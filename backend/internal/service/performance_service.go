package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"synapseqa/backend/internal/model"
)

// ErrPerfConfigConflict 表示同幂等键但触发配置不同（SPEC §5.3，返回 409）。
var ErrPerfConfigConflict = errors.New("同幂等键但配置不同")

var (
	perfSensitiveTextPattern  = regexp.MustCompile(`(?i)(["']?(?:authorization|cookie|password|passwd|token|access[_-]?token|refresh[_-]?token|client[_-]?secret|secret|api[_-]?key)["']?\s*[:=]\s*(?:(?:bearer|basic)\s+)?)[^"'\s,;}]+`)
	perfSensitiveQueryPattern = regexp.MustCompile(`(?i)([?&](?:authorization|cookie|password|passwd|token|access[_-]?token|refresh[_-]?token|client[_-]?secret|secret|api[_-]?key)=)[^&#\s]+`)
	perfDurationPattern       = regexp.MustCompile(`^(?:\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h))+$`)
	perfRandRead              = rand.Read
)

// 性能测试调度与超时常量（SPEC §5.1）。
const (
	perfDispatchDeadline = 60 * time.Second  // dispatching 提交确认截止
	perfStartDeadline    = 120 * time.Second // dispatched 启动确认截止
	perfTimeoutBuffer    = 120 * time.Second // 超时判定附加缓冲
	perfDefaultDuration  = 600 * time.Second // 无法解析负载时长的保守默认
	perfStoppingGrace    = 30 * time.Second  // stopping 宽限期
	perfRecoverInterval  = 10 * time.Second  // 恢复扫描周期
	perfMaxVUs           = 500
)

type PerformanceRepository interface {
	ListPlans(ctx context.Context, filter model.PerfTestPlanFilter, page, pageSize int) ([]model.PerfTestPlan, int64, error)
	GetPlan(ctx context.Context, id int64) (model.PerfTestPlan, error)
	CreatePlan(ctx context.Context, req model.PerfTestPlanRequest, actor string) (int64, error)
	UpdatePlan(ctx context.Context, id int64, req model.PerfTestPlanRequest) (int64, error)
	DeletePlan(ctx context.Context, id int64) (int64, error)
	ExistsProduct(ctx context.Context, id int64) bool
	ListRuns(ctx context.Context, filter model.PerfTestRunFilter, page, pageSize int) ([]model.PerfTestRun, int64, error)
	GetRun(ctx context.Context, id int64) (model.PerfTestRun, error)
	GetRunByTaskID(ctx context.Context, taskID string) (model.PerfTestRun, error)
	GetRunByIdempotencyKey(ctx context.Context, triggeredBy, key string) (model.PerfTestRun, error)
	CreateRun(ctx context.Context, planID int64, scenarioType, environment, configHash, idempotencyKey, triggeredBy string, planSnapshot json.RawMessage, requestedAt, expectedFinishAt time.Time) (int64, error)
	UpdateRunStatus(ctx context.Context, id int64, from, to string, extra map[string]any) (int64, error)
	UpdateRunResult(ctx context.Context, id int64, from []string, to string, result model.PerfRunResult) (int64, error)
	MarkNeedsAttention(ctx context.Context, id int64) (int64, error)
}

type PerformanceService struct {
	performanceRepo PerformanceRepository
	executorRepo    ExecutorRepository
	systemRepo      OperationLogger
	callbackBase    string
	httpClient      *http.Client
	wakeScheduler   chan struct{}
	dispatchMu      sync.Mutex
	notifier        interface {
		Create(context.Context, model.NotificationCreate) error
	}
	notifiedRuns sync.Map
	smokeMu      sync.Mutex
	smokeTasks   map[string]string // taskID -> executorID（冒烟任务定位）
	events       *PerfEventHub
}

func NewPerformanceService(performanceRepo PerformanceRepository, executorRepo ExecutorRepository, systemRepo OperationLogger, callbackBase string) *PerformanceService {
	svc := &PerformanceService{
		performanceRepo: performanceRepo,
		executorRepo:    executorRepo,
		callbackBase:    strings.TrimRight(callbackBase, "/"),
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		wakeScheduler:   make(chan struct{}, 1),
		smokeTasks:      make(map[string]string),
		events:          NewPerfEventHub(),
	}
	if systemRepo != nil {
		svc.systemRepo = systemRepo
	}
	return svc
}

func NewPerformanceServiceWithLogger(performanceRepo PerformanceRepository, logger OperationLogger) *PerformanceService {
	return &PerformanceService{
		performanceRepo: performanceRepo,
		systemRepo:      logger,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		wakeScheduler:   make(chan struct{}, 1),
		smokeTasks:      make(map[string]string),
		events:          NewPerfEventHub(),
	}
}

func (s *PerformanceService) SetNotifier(notifier interface {
	Create(context.Context, model.NotificationCreate) error
}) {
	s.notifier = notifier
}

func (s *PerformanceService) ListPlans(ctx context.Context, filter model.PerfTestPlanFilter, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	filter.ID = strings.TrimSpace(filter.ID)
	filter.Name = strings.TrimSpace(filter.Name)
	filter.ProductID = strings.TrimSpace(filter.ProductID)
	filter.ScenarioType = strings.TrimSpace(filter.ScenarioType)
	filter.Environment = strings.TrimSpace(filter.Environment)
	filter.Priority = strings.TrimSpace(filter.Priority)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Owner = strings.TrimSpace(filter.Owner)
	items, total, err := s.performanceRepo.ListPlans(ctx, filter, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *PerformanceService) GetPlan(ctx context.Context, id int64) (model.PerfTestPlan, error) {
	if id <= 0 {
		return model.PerfTestPlan{}, errors.New("性能测试方案 ID 无效")
	}
	item, err := s.performanceRepo.GetPlan(ctx, id)
	if err != nil {
		return model.PerfTestPlan{}, errors.New("性能测试方案不存在")
	}
	return item, nil
}

func (s *PerformanceService) CreatePlan(ctx context.Context, actor string, req model.PerfTestPlanRequest) (int64, error) {
	req, err := s.normalizeRequest(ctx, req)
	if err != nil {
		return 0, err
	}
	if err := s.SetPlanEnvironment(ctx, &req); err != nil {
		return 0, err
	}
	var id int64
	if repo, ok := s.performanceRepo.(interface {
		CreatePlanWithEnvironment(context.Context, model.PerfTestPlanRequest, string) (int64, error)
	}); ok {
		id, err = repo.CreatePlanWithEnvironment(ctx, req, actor)
	} else {
		id, err = s.performanceRepo.CreatePlan(ctx, req, actor)
	}
	if err != nil {
		return 0, errors.New("新增性能测试方案失败，名称可能已存在")
	}
	s.log(ctx, actor, "新增性能测试方案", req.Name)
	return id, nil
}

func (s *PerformanceService) UpdatePlan(ctx context.Context, actor string, id int64, req model.PerfTestPlanRequest) error {
	if id <= 0 {
		return errors.New("性能测试方案 ID 无效")
	}
	req, err := s.normalizeRequest(ctx, req)
	if err != nil {
		return err
	}
	if err := s.SetPlanEnvironment(ctx, &req); err != nil {
		return err
	}
	var rows int64
	if repo, ok := s.performanceRepo.(interface {
		UpdatePlanWithEnvironment(context.Context, int64, model.PerfTestPlanRequest) (int64, error)
	}); ok {
		rows, err = repo.UpdatePlanWithEnvironment(ctx, id, req)
	} else {
		rows, err = s.performanceRepo.UpdatePlan(ctx, id, req)
	}
	if err != nil {
		return errors.New("更新性能测试方案失败，名称可能已存在")
	}
	if rows == 0 {
		return errors.New("性能测试方案不存在")
	}
	s.log(ctx, actor, "更新性能测试方案", fmt.Sprintf("%d:%s", id, req.Name))
	return nil
}

func (s *PerformanceService) DeletePlan(ctx context.Context, actor string, id int64) error {
	if id <= 0 {
		return errors.New("性能测试方案 ID 无效")
	}
	rows, err := s.performanceRepo.DeletePlan(ctx, id)
	if err != nil {
		return errors.New("删除性能测试方案失败")
	}
	if rows == 0 {
		return errors.New("性能测试方案不存在")
	}
	s.log(ctx, actor, "删除性能测试方案", strconv.FormatInt(id, 10))
	return nil
}

// RunPlan 触发执行（幂等，SPEC §5.3）。reused 为 true 表示命中已有执行记录，应返回 200。
func (s *PerformanceService) RunPlan(ctx context.Context, actor, idempotencyKey string, planID int64) (model.PerfTestRun, bool, error) {
	return s.runPlanWithEnvironment(ctx, actor, idempotencyKey, planID, nil)
}

func (s *PerformanceService) RunPlanWithEnvironment(ctx context.Context, actor, idempotencyKey string, planID int64, environmentID *int64) (model.PerfTestRun, bool, error) {
	return s.runPlanWithEnvironment(ctx, actor, idempotencyKey, planID, environmentID)
}

func (s *PerformanceService) runPlanWithEnvironment(ctx context.Context, actor, idempotencyKey string, planID int64, overrideEnvironmentID *int64) (model.PerfTestRun, bool, error) {
	if planID <= 0 {
		return model.PerfTestRun{}, false, errors.New("性能测试方案 ID 无效")
	}
	plan, err := s.performanceRepo.GetPlan(ctx, planID)
	if err != nil {
		return model.PerfTestRun{}, false, errors.New("性能测试方案不存在")
	}
	if plan.Status != "active" {
		return model.PerfTestRun{}, false, errors.New("方案必须处于 active 状态才能执行")
	}
	originalPlan := plan
	resolvedPlan := plan
	if overrideEnvironmentID != nil {
		if *overrideEnvironmentID <= 0 {
			return model.PerfTestRun{}, false, errors.New("环境 ID 无效")
		}
		resolved, resolveErr := s.resolvePerfEnvironment(ctx, resolvedPlan.ProductID, overrideEnvironmentID)
		if resolveErr != nil {
			return model.PerfTestRun{}, false, resolveErr
		}
		resolvedPlan.EnvironmentID = &resolved.EnvironmentID
		resolvedPlan.Environment = resolved.EnvName
		resolvedPlan.EnvironmentName = resolved.EnvName
		resolvedPlan.EnvironmentDeployEnv = resolved.DeployEnv
		resolvedPlan.EnvironmentBaseURL = resolved.BaseURL
	}
	if resolvedPlan.EnvironmentID != nil {
		resolved, resolveErr := s.resolvePerfEnvironment(ctx, resolvedPlan.ProductID, resolvedPlan.EnvironmentID)
		if resolveErr != nil {
			return model.PerfTestRun{}, false, resolveErr
		}
		resolvedPlan.EnvironmentID = &resolved.EnvironmentID
		resolvedPlan.EnvironmentName = resolved.EnvName
		resolvedPlan.Environment = resolved.EnvName
		resolvedPlan.EnvironmentDeployEnv = resolved.DeployEnv
		resolvedPlan.EnvironmentBaseURL = resolved.BaseURL
	}
	if resolvedPlan.EnvironmentID != nil {
		if err := resolvePerfPlanURLs(&resolvedPlan); err != nil {
			return model.PerfTestRun{}, false, err
		}
	}
	snapshot, configHash, err := buildPlanSnapshotAndHashForRun(originalPlan, resolvedPlan)
	if err != nil {
		return model.PerfTestRun{}, false, errors.New("生成执行快照失败")
	}
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return model.PerfTestRun{}, false, errors.New("幂等键不能为空")
	}
	if existing, err := s.performanceRepo.GetRunByIdempotencyKey(ctx, actor, key); err == nil {
		if existing.ConfigHash == configHash {
			existing, err = s.enqueuePendingRun(ctx, existing)
			if err != nil {
				return model.PerfTestRun{}, true, err
			}
			return existing, true, nil
		}
		return existing, false, ErrPerfConfigConflict
	}
	requestedAt := time.Now()
	var runID int64
	if repo, ok := s.performanceRepo.(interface {
		CreateRunWithEnvironment(context.Context, int64, string, string, *int64, string, string, string, json.RawMessage, time.Time, time.Time) (int64, error)
	}); ok {
		runID, err = repo.CreateRunWithEnvironment(ctx, planID, resolvedPlan.ScenarioType, resolvedPlan.Environment, resolvedPlan.EnvironmentID, configHash, key, actor, snapshot, requestedAt, computeExpectedFinishAt(resolvedPlan, requestedAt))
	} else {
		runID, err = s.performanceRepo.CreateRun(ctx, planID, resolvedPlan.ScenarioType, resolvedPlan.Environment, configHash, key, actor, snapshot, requestedAt, computeExpectedFinishAt(resolvedPlan, requestedAt))
	}
	if err != nil {
		// 唯一索引冲突（并发同 key 或同方案活动任务），重查幂等判定。
		if existing, getErr := s.performanceRepo.GetRunByIdempotencyKey(ctx, actor, key); getErr == nil {
			if existing.ConfigHash == configHash {
				existing, enqueueErr := s.enqueuePendingRun(ctx, existing)
				if enqueueErr != nil {
					return model.PerfTestRun{}, true, enqueueErr
				}
				return existing, true, nil
			}
			return existing, false, ErrPerfConfigConflict
		}
		return model.PerfTestRun{}, false, errors.New("触发性能测试失败，同一方案可能已有活动执行")
	}
	run, err := s.performanceRepo.GetRun(ctx, runID)
	if err != nil {
		return model.PerfTestRun{}, false, errors.New("读取执行记录失败")
	}
	run, err = s.enqueuePendingRun(ctx, run)
	if err != nil {
		return model.PerfTestRun{}, false, err
	}
	s.log(ctx, actor, "触发性能测试", strconv.FormatInt(planID, 10))
	s.wake()
	return run, false, nil
}

// enqueuePendingRun 推进 pending→queued，条件更新保证并发触发只入队一次。
func (s *PerformanceService) enqueuePendingRun(ctx context.Context, run model.PerfTestRun) (model.PerfTestRun, error) {
	if run.Status != model.PerfRunPending {
		return run, nil
	}
	rows, err := s.performanceRepo.UpdateRunStatus(ctx, run.ID, model.PerfRunPending, model.PerfRunQueued, nil)
	if err != nil {
		return model.PerfTestRun{}, errors.New("加入性能测试队列失败")
	}
	if rows > 0 {
		run.Status = model.PerfRunQueued
		return run, nil
	}
	latest, err := s.performanceRepo.GetRun(ctx, run.ID)
	if err != nil {
		return model.PerfTestRun{}, errors.New("确认性能测试队列状态失败")
	}
	if latest.Status == model.PerfRunPending {
		return model.PerfTestRun{}, errors.New("加入性能测试队列失败")
	}
	return latest, nil
}

// CancelRun 取消执行（SPEC §5.1）。pending/queued 直接取消，运行态转为 stopping 并通知执行器。
func (s *PerformanceService) CancelRun(ctx context.Context, actor string, id int64) error {
	run, err := s.performanceRepo.GetRun(ctx, id)
	if err != nil {
		return errors.New("执行记录不存在")
	}
	switch run.Status {
	case model.PerfRunPending, model.PerfRunQueued:
		result := s.partialTerminalResult(ctx, run, model.PerfFailureCancel, "任务已取消")
		if err := s.persistTerminal(ctx, run, []string{run.Status}, model.PerfRunCanceled, result); err != nil {
			return errors.New("取消执行失败")
		}
	case model.PerfRunDispatching, model.PerfRunDispatched, model.PerfRunRunning:
		rows, err := s.performanceRepo.UpdateRunStatus(ctx, id, run.Status, model.PerfRunStopping, nil)
		if err != nil {
			return errors.New("取消执行失败")
		}
		if rows == 0 {
			return errors.New("执行状态已变化，取消失败")
		}
		if run.TaskID != "" && s.executorRepo != nil {
			_ = s.cancelExecutorTask(ctx, run.ExecutorID, run.TaskID)
		}
	case model.PerfRunStopping:
		return nil
	default:
		return errors.New("当前执行已结束，无法取消")
	}
	s.log(ctx, actor, "取消性能测试", strconv.FormatInt(id, 10))
	return nil
}

func (s *PerformanceService) ListRuns(ctx context.Context, filter model.PerfTestRunFilter, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	filter.ID = strings.TrimSpace(filter.ID)
	filter.PlanID = strings.TrimSpace(filter.PlanID)
	filter.ScenarioType = strings.TrimSpace(filter.ScenarioType)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Environment = strings.TrimSpace(filter.Environment)
	items, total, err := s.performanceRepo.ListRuns(ctx, filter, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *PerformanceService) GetRun(ctx context.Context, id int64) (model.PerfTestRun, error) {
	if id <= 0 {
		return model.PerfTestRun{}, errors.New("执行记录 ID 无效")
	}
	item, err := s.performanceRepo.GetRun(ctx, id)
	if err != nil {
		return model.PerfTestRun{}, errors.New("执行记录不存在")
	}
	hydrateRunEnvironmentFromSnapshot(&item)
	if !isFinalRunStatus(item.Status) && s.events != nil {
		if series, ok := s.events.Snapshot(item.ID); ok {
			encoded, _ := json.Marshal(series)
			item.Series = encoded
		}
	}
	return item, nil
}

// HandleCallback 处理执行器回调（SPEC §4.2），task_id 仅定位，凭据为 Bearer callback token。
func (s *PerformanceService) HandleCallback(ctx context.Context, taskID, token string, req model.PerfCallbackRequest) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("taskId 不能为空")
	}
	run, err := s.performanceRepo.GetRunByTaskID(ctx, taskID)
	if err != nil {
		return errors.New("执行记录不存在")
	}
	if isFinalRunStatus(run.Status) {
		return errors.New("执行已终结")
	}
	if run.CallbackTokenHash == "" || sha256Hex(token) != run.CallbackTokenHash {
		return errors.New("回调凭据无效")
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	switch status {
	case "running":
		if run.Status == model.PerfRunRunning {
			return nil
		}
		if run.Status == model.PerfRunStopping {
			return errors.New("回调状态冲突")
		}
		startedAt := time.Now()
		rows, err := s.performanceRepo.UpdateRunStatus(ctx, run.ID, model.PerfRunDispatching+","+model.PerfRunDispatched, model.PerfRunRunning, map[string]any{
			"script_hash":        req.ScriptHash,
			"generator_version":  req.GeneratorVersion,
			"k6_version":         req.K6Version,
			"started_at":         startedAt,
			"expected_finish_at": s.expectedFinishAtForRun(run, startedAt),
		})
		if err != nil {
			return errors.New("更新执行状态失败")
		}
		if rows == 0 {
			return errors.New("回调状态冲突")
		}
		return nil
	case "completed", "threshold_failed", "execution_failed", "timed_out", "canceled":
		if run.Status == model.PerfRunStopping && status != "canceled" {
			return errors.New("回调状态冲突")
		}
		summary := sanitizePerfJSON(req.Summary)
		seriesRaw := normalizePerfSeries(req.Series)
		if len(req.Series) == 0 && s.events != nil {
			if current, ok := s.events.Snapshot(run.ID); ok {
				seriesRaw, _ = json.Marshal(current)
			}
		}
		finalStatus := resolveFinalStatus(status, summary)
		if status == "execution_failed" || status == "timed_out" || status == "canceled" {
			seriesRaw = forcePerfSeriesPartial(seriesRaw)
		}
		failureStage := req.FailureStage
		if failureStage == "" {
			failureStage = defaultFailureStage(status)
		}
		result := model.PerfRunResult{
			ScriptHash:       sanitizePerfText(req.ScriptHash),
			GeneratorVersion: sanitizePerfText(req.GeneratorVersion),
			K6Version:        sanitizePerfText(req.K6Version),
			TotalRequests:    req.TotalRequests,
			AvgDurationMs:    req.AvgDurationMs,
			P95DurationMs:    req.P95DurationMs,
			P99DurationMs:    firstFloat(req.P99DurationMs, perfP99FromSummary(summary)),
			ErrorRate:        req.ErrorRate,
			RPS:              req.RPS,
			Summary:          summary,
			ExitCode:         req.ExitCode,
			DurationMs:       req.DurationMs,
			ErrorMessage:     sanitizePerfText(req.ErrorMessage),
			FailureStage:     failureStage,
			DiagnosticOutput: sanitizePerfText(req.DiagnosticOutput),
			NeedsAttention:   req.NeedsAttention,
			Series:           seriesRaw,
		}
		rows, err := s.performanceRepo.UpdateRunResult(ctx, run.ID, []string{model.PerfRunDispatching, model.PerfRunDispatched, model.PerfRunRunning, model.PerfRunStopping}, finalStatus, result)
		if err != nil {
			return errors.New("更新执行结果失败")
		}
		if rows == 0 {
			return errors.New("回调状态冲突")
		}
		s.notifyRunFinished(ctx, run, finalStatus)
		if s.events != nil {
			var series model.PerfSeries
			_ = json.Unmarshal(result.Series, &series)
			s.events.MarkTerminal(run.ID, finalStatus, series)
		}
		return nil
	default:
		return errors.New("回调状态无效")
	}
}

// HandleSampleEvent 接收执行器窗口采样，旧 sequence 幂等忽略，不落库阻塞执行器。
func (s *PerformanceService) HandleSampleEvent(ctx context.Context, taskID, token string, event model.PerfSampleEvent) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("taskId 不能为空")
	}
	run, err := s.performanceRepo.GetRunByTaskID(ctx, taskID)
	if err != nil {
		return errors.New("执行记录不存在")
	}
	if isFinalRunStatus(run.Status) {
		return errors.New("执行已终结")
	}
	if run.CallbackTokenHash == "" || sha256Hex(token) != run.CallbackTokenHash {
		return errors.New("回调凭据无效")
	}
	if event.Sequence <= 0 {
		return errors.New("采样序号无效")
	}
	event.TaskID = taskID
	if event.Type == "" {
		event.Type = "sample"
	}
	if event.Status == "" {
		event.Status = model.PerfRunRunning
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.WindowMs < 1000 || event.WindowMs > 5000 {
		event.WindowMs = 2000
	}
	if event.StatusCodes == nil {
		event.StatusCodes = map[string]int{}
	}
	if event.ErrorTopN == nil {
		event.ErrorTopN = []model.PerfErrorTop{}
	}
	if event.Thresholds == nil {
		event.Thresholds = []model.PerfSampleThreshold{}
	}
	event.Message = sanitizePerfText(event.Message)
	if encoded, marshalErr := json.Marshal(event); marshalErr == nil {
		var safeEvent model.PerfSampleEvent
		if json.Unmarshal(sanitizePerfJSON(encoded), &safeEvent) == nil {
			event = safeEvent
		}
	}
	if encoded, marshalErr := json.Marshal(event); marshalErr != nil || len(encoded) > 64*1024 {
		return errors.New("采样事件过大")
	}
	if s.events != nil {
		s.events.Publish(run.ID, event)
	}
	return nil
}

func (s *PerformanceService) SubscribeRunEvents(ctx context.Context, id, after int64) (<-chan PerfStreamMessage, func(), error) {
	if id <= 0 {
		return nil, nil, errors.New("执行记录 ID 无效")
	}
	run, err := s.performanceRepo.GetRun(ctx, id)
	if err != nil {
		return nil, nil, errors.New("执行记录不存在")
	}
	if isFinalRunStatus(run.Status) && s.events != nil {
		var series model.PerfSeries
		_ = json.Unmarshal(normalizePerfSeries(run.Series), &series)
		s.events.MarkTerminal(id, run.Status, series)
	}
	channel, cancel := s.events.Subscribe(id, after)
	return channel, cancel, nil
}

// StartScheduler 启动性能测试队列调度与中间态恢复循环。
func (s *PerformanceService) StartScheduler(ctx context.Context) {
	go func() {
		dispatchTicker := time.NewTicker(time.Second)
		recoverTicker := time.NewTicker(perfRecoverInterval)
		scheduleTicker := time.NewTicker(10 * time.Second)
		defer dispatchTicker.Stop()
		defer recoverTicker.Stop()
		defer scheduleTicker.Stop()
		// 启动时立即恢复一次，处理进程重启遗留的中间态任务（SPEC §5.4）。
		s.recover(ctx)
		s.dispatch(ctx)
		s.ProcessPerfSchedules(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-dispatchTicker.C:
				s.dispatch(ctx)
			case <-recoverTicker.C:
				s.recover(ctx)
			case <-scheduleTicker.C:
				s.ProcessPerfSchedules(ctx)
			case <-s.wakeScheduler:
				s.dispatch(ctx)
			}
		}
	}()
	s.wake()
}

func (s *PerformanceService) dispatch(ctx context.Context) {
	if err := s.DispatchPending(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("性能测试队列调度失败：%v", err)
	}
}

func (s *PerformanceService) recover(ctx context.Context) {
	if s.events != nil {
		s.events.CleanupExpired(time.Now())
	}
	if err := s.RecoverStale(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("性能测试恢复扫描失败：%v", err)
	}
	s.RecoverPerfDegradation(ctx)
}

func (s *PerformanceService) wake() {
	select {
	case s.wakeScheduler <- struct{}{}:
	default:
	}
}

// DispatchPending 扫描 pending/queued 任务，先推进 pending 再下发。
func (s *PerformanceService) DispatchPending(ctx context.Context) error {
	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	pendingRuns, _, err := s.performanceRepo.ListRuns(ctx, model.PerfTestRunFilter{Status: model.PerfRunPending}, 1, 50)
	if err != nil {
		return err
	}
	for _, run := range pendingRuns {
		if run.ID == 0 {
			continue
		}
		if _, err := s.enqueuePendingRun(ctx, run); err != nil {
			continue
		}
	}
	runs, _, err := s.performanceRepo.ListRuns(ctx, model.PerfTestRunFilter{Status: model.PerfRunQueued}, 1, 50)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.ID == 0 {
			continue
		}
		if err := s.dispatchRun(ctx, run); err != nil {
			// 执行器暂不可用或无 k6，保留 queued，下一轮继续。
			continue
		}
	}
	return nil
}

func (s *PerformanceService) dispatchRun(ctx context.Context, run model.PerfTestRun) error {
	fullRun, err := s.performanceRepo.GetRun(ctx, run.ID)
	if err != nil {
		return err
	}
	fullRun.Status = run.Status
	run = fullRun
	executor, err := s.pickExecutor(ctx)
	if err != nil {
		return err
	}
	taskID := generateTaskID()
	callbackToken, err := generateCallbackToken()
	if err != nil {
		return errors.New("生成性能测试回调凭据失败")
	}
	callbackURL := fmt.Sprintf("%s/api/perf/tasks/%s/callback", s.callbackBase, taskID)
	eventURL := fmt.Sprintf("%s/api/perf/tasks/%s/events/callback", s.callbackBase, taskID)
	sampleIntervalMs := perfSampleIntervalMs(run)
	now := time.Now()
	rows, err := s.performanceRepo.UpdateRunStatus(ctx, run.ID, model.PerfRunQueued, model.PerfRunDispatching, map[string]any{
		"task_id":              taskID,
		"callback_token_hash":  sha256Hex(callbackToken),
		"executor_id":          executor.ExecutorID,
		"executor_name":        executor.Name,
		"dispatch_deadline_at": now.Add(perfDispatchDeadline),
		"start_deadline_at":    now.Add(perfStartDeadline),
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		// 竞态：已被其他调度循环处理。
		return nil
	}
	payload, err := buildPerfPayload(run)
	if err != nil {
		return errors.New("构建执行器负载失败")
	}
	if err := s.submitPerfTask(ctx, executor, taskID, payload, callbackURL, eventURL, callbackToken, sampleIntervalMs); err != nil {
		// 提交失败：保持 dispatching 并依赖超时恢复重试（粘性绑定同一 task_id/executor_id）。
		return err
	}
	_, _ = s.performanceRepo.UpdateRunStatus(ctx, run.ID, model.PerfRunDispatching, model.PerfRunDispatched, map[string]any{
		"dispatched_at": time.Now(),
	})
	return nil
}

// RecoverStale 扫描并恢复中间态任务（SPEC §5.4）：只查询、只补偿取消，绝不自动重投。
func (s *PerformanceService) RecoverStale(ctx context.Context) error {
	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	for _, status := range []string{model.PerfRunDispatching, model.PerfRunDispatched, model.PerfRunRunning, model.PerfRunStopping} {
		runs, _, err := s.performanceRepo.ListRuns(ctx, model.PerfTestRunFilter{Status: status}, 1, 100)
		if err != nil {
			return err
		}
		for _, run := range runs {
			if run.ID == 0 {
				continue
			}
			s.recoverRun(ctx, run)
		}
	}
	return nil
}

func (s *PerformanceService) recoverRun(ctx context.Context, run model.PerfTestRun) {
	now := time.Now()
	switch run.Status {
	case model.PerfRunDispatching:
		if run.DispatchDeadlineAt == nil || now.Before(*run.DispatchDeadlineAt) {
			return
		}
		task, err := s.queryExecutorTask(ctx, run.ExecutorID, run.TaskID)
		if err != nil {
			s.markExecutionFailed(ctx, run.ID, model.PerfFailureExecutorOffline, "提交确认超时，执行器不可达")
			return
		}
		switch taskStatus(task) {
		case "running":
			// 执行器已快速启动，启动回调丢失，直接推进到 running。
			_, _ = s.performanceRepo.UpdateRunStatus(ctx, run.ID, model.PerfRunDispatching, model.PerfRunRunning, nil)
		case "success", "failed", "canceled":
			s.recoverExecutorTerminal(ctx, run, task)
		case "queued":
			// 执行器已接收任务，推进到 dispatched。
			_, _ = s.performanceRepo.UpdateRunStatus(ctx, run.ID, model.PerfRunDispatching, model.PerfRunDispatched, map[string]any{"dispatched_at": time.Now()})
		default:
			s.markExecutionFailed(ctx, run.ID, model.PerfFailureDispatch, "执行器未接收任务")
		}
	case model.PerfRunDispatched:
		if run.StartDeadlineAt == nil || now.Before(*run.StartDeadlineAt) {
			return
		}
		task, err := s.queryExecutorTask(ctx, run.ExecutorID, run.TaskID)
		if err != nil {
			s.markExecutionFailed(ctx, run.ID, model.PerfFailureExecutorOffline, "启动确认超时，执行器不可达")
			return
		}
		if taskStatus(task) == "running" {
			_, _ = s.performanceRepo.UpdateRunStatus(ctx, run.ID, model.PerfRunDispatched, model.PerfRunRunning, nil)
		} else if taskStatus(task) == "success" || taskStatus(task) == "failed" || taskStatus(task) == "canceled" {
			s.recoverExecutorTerminal(ctx, run, task)
		} else {
			s.markExecutionFailed(ctx, run.ID, model.PerfFailureStartup, "执行器未启动 k6")
		}
	case model.PerfRunRunning:
		if run.ExpectedFinishAt == nil || now.Before(*run.ExpectedFinishAt) {
			return
		}
		task, err := s.queryExecutorTask(ctx, run.ExecutorID, run.TaskID)
		if err != nil {
			s.markNeedsAttention(ctx, run.ID)
			return
		}
		switch taskStatus(task) {
		case "success", "failed", "canceled":
			s.recoverExecutorTerminal(ctx, run, task)
		case "":
			s.markExecutionFailed(ctx, run.ID, model.PerfFailureTimeout, "执行超时且执行器进程已结束")
		default:
			// 仍在运行但已超时，无法确认是否卡死 → 人工关注。
			s.markNeedsAttention(ctx, run.ID)
		}
	case model.PerfRunStopping:
		if now.Sub(run.UpdatedAt) < perfStoppingGrace {
			return
		}
		if run.TaskID != "" {
			_ = s.cancelExecutorTask(ctx, run.ExecutorID, run.TaskID)
		}
		task, err := s.queryExecutorTask(ctx, run.ExecutorID, run.TaskID)
		if err != nil {
			s.markExecutionFailed(ctx, run.ID, model.PerfFailureExecutorOffline, "取消确认时执行器不可达")
			return
		}
		if taskStatus(task) == "canceled" || taskStatus(task) == "success" || taskStatus(task) == "failed" {
			// 执行器已结束，取消意图优先，保存已采集的部分结果并撤销回调凭据。
			result := perfResultFromExecutorTask(task)
			result.Series = forcePerfSeriesPartial(result.Series)
			_ = s.persistTerminal(ctx, run, []string{model.PerfRunStopping}, model.PerfRunCanceled, result)
		} else {
			s.markNeedsAttention(ctx, run.ID)
		}
	}
}

func (s *PerformanceService) recoverExecutorTerminal(ctx context.Context, run model.PerfTestRun, task map[string]any) {
	result := perfResultFromExecutorTask(task)
	status := taskTerminalPerfStatus(task, result)
	if status == model.PerfRunCanceled || status == model.PerfRunExecutionFailed || status == model.PerfRunTimedOut {
		result.Series = forcePerfSeriesPartial(result.Series)
	}
	_ = s.persistTerminal(ctx, run, []string{run.Status}, status, result)
}

func taskTerminalPerfStatus(task map[string]any, result model.PerfRunResult) string {
	resultMap, _ := task["result"].(map[string]any)
	outputText, _ := resultMap["output"].(string)
	var output map[string]any
	_ = json.Unmarshal([]byte(outputText), &output)
	status, _ := output["terminal_status"].(string)
	switch status {
	case model.PerfRunCompleted, model.PerfRunThresholdFailed, model.PerfRunExecutionFailed, model.PerfRunTimedOut, model.PerfRunCanceled:
		if status == model.PerfRunCompleted {
			return resolveFinalStatus(status, result.Summary)
		}
		return status
	}
	switch taskStatus(task) {
	case "canceled":
		return model.PerfRunCanceled
	case "success":
		return resolveFinalStatus(model.PerfRunCompleted, result.Summary)
	default:
		return model.PerfRunExecutionFailed
	}
}

func perfResultFromExecutorTask(task map[string]any) model.PerfRunResult {
	resultMap, _ := task["result"].(map[string]any)
	outputText, _ := resultMap["output"].(string)
	var output map[string]any
	_ = json.Unmarshal([]byte(outputText), &output)
	metrics, _ := output["metrics"].(map[string]any)
	summary := output["summary"]
	if summary == nil {
		summary = _summaryFromPerfOutput(output)
	}
	summaryRaw, _ := json.Marshal(summary)
	seriesRaw, _ := json.Marshal(output["series"])
	return model.PerfRunResult{
		ScriptHash:       sanitizePerfText(stringValue(output, "script_hash")),
		GeneratorVersion: sanitizePerfText(stringValue(output, "generator_version")),
		K6Version:        sanitizePerfText(stringValue(output, "k6_version")),
		TotalRequests:    intValue(metrics, "total_requests"),
		AvgDurationMs:    floatValuePtr(metrics, "avg_duration_ms"),
		P95DurationMs:    floatValuePtr(metrics, "p95_duration_ms"),
		P99DurationMs:    firstFloat(floatValuePtr(metrics, "p99_duration_ms"), perfP99FromSummary(summaryRaw)),
		ErrorRate:        floatValuePtr(metrics, "error_rate"),
		RPS:              floatValuePtr(metrics, "rps"),
		Summary:          sanitizePerfJSON(summaryRaw),
		Series:           normalizePerfSeries(seriesRaw),
		ExitCode:         intPtrFromAny(resultMap["exitCode"]),
		ErrorMessage:     sanitizePerfText(stringValue(resultMap, "error")),
		FailureStage:     sanitizePerfText(stringValue(output, "failure_stage")),
		DiagnosticOutput: sanitizePerfText(stringValue(output, "diagnostic")),
		NeedsAttention:   boolValue(output, "needs_attention"),
	}
}

func _summaryFromPerfOutput(output map[string]any) map[string]any {
	return map[string]any{"metrics": output["metrics"], "options": map[string]any{}, "state": map[string]any{}}
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func intValue(values map[string]any, key string) int {
	value, ok := values[key]
	if !ok {
		return 0
	}
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	}
	return 0
}

func intPtrFromAny(value any) *int {
	if value == nil {
		return nil
	}
	result := intValue(map[string]any{"value": value}, "value")
	return &result
}

func floatValuePtr(values map[string]any, key string) *float64 {
	value, ok := values[key]
	if !ok || value == nil {
		return nil
	}
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case int:
		number = float64(typed)
	default:
		return nil
	}
	return &number
}

func boolValue(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func (s *PerformanceService) markExecutionFailed(ctx context.Context, id int64, failureStage, message string) {
	run, err := s.performanceRepo.GetRun(ctx, id)
	if err != nil {
		return
	}
	result := s.partialTerminalResult(ctx, run, failureStage, message)
	_ = s.persistTerminal(ctx, run, []string{model.PerfRunDispatching, model.PerfRunDispatched, model.PerfRunRunning, model.PerfRunStopping}, model.PerfRunExecutionFailed, result)
}

func (s *PerformanceService) partialTerminalResult(ctx context.Context, run model.PerfTestRun, failureStage, message string) model.PerfRunResult {
	seriesRaw := run.Series
	if s.events != nil {
		if series, ok := s.events.Snapshot(run.ID); ok {
			seriesRaw, _ = json.Marshal(series)
		}
	}
	return model.PerfRunResult{
		Summary:          json.RawMessage(`{}`),
		FailureStage:     failureStage,
		ErrorMessage:     sanitizePerfText(message),
		DiagnosticOutput: sanitizePerfText(message),
		Series:           forcePerfSeriesPartial(normalizePerfSeries(seriesRaw)),
	}
}

func (s *PerformanceService) persistTerminal(ctx context.Context, run model.PerfTestRun, from []string, status string, result model.PerfRunResult) error {
	rows, err := s.performanceRepo.UpdateRunResult(ctx, run.ID, from, status, result)
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("执行状态已变化")
	}
	s.notifyRunFinished(ctx, run, status)
	if s.events != nil {
		var series model.PerfSeries
		_ = json.Unmarshal(normalizePerfSeries(result.Series), &series)
		s.events.MarkTerminal(run.ID, status, series)
	}
	return nil
}

func (s *PerformanceService) markNeedsAttention(ctx context.Context, id int64) {
	_, _ = s.performanceRepo.MarkNeedsAttention(ctx, id)
}

// queryExecutorTask 查询执行器侧任务状态，仅用于恢复判定，不触发状态回写。
func (s *PerformanceService) queryExecutorTask(ctx context.Context, executorID, taskID string) (map[string]any, error) {
	if s.executorRepo == nil || executorID == "" || taskID == "" {
		return nil, errors.New("任务信息不完整")
	}
	executor, err := s.executorRepo.GetByID(ctx, executorID)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(executor.Endpoint, "/") + "/tasks/" + taskID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("任务不存在")
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("执行器返回 %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func taskStatus(task map[string]any) string {
	status, _ := task["status"].(string)
	return strings.ToLower(strings.TrimSpace(status))
}

// pickExecutor 选择在线、支持 perf 且 k6 检测通过的执行器（SPEC §4.4）。
func (s *PerformanceService) pickExecutor(ctx context.Context) (model.ExecutorView, error) {
	if s.executorRepo == nil {
		return model.ExecutorView{}, errors.New("执行器仓储未初始化")
	}
	executors, err := s.executorRepo.List(ctx)
	if err != nil {
		return model.ExecutorView{}, errors.New("查询执行器失败")
	}
	var candidates []model.ExecutorView
	for _, item := range executors {
		if item.Status != "online" {
			continue
		}
		if !containsString(item.SupportedTypes, "perf") {
			continue
		}
		if !item.Checks["k6"] {
			continue
		}
		candidates = append(candidates, item)
	}
	if len(candidates) == 0 {
		return model.ExecutorView{}, errors.New("没有可用的在线性能测试执行器（未安装 k6 或离线）")
	}
	selected := candidates[0]
	for _, item := range candidates[1:] {
		if item.RunningTasks < selected.RunningTasks {
			selected = item
		}
	}
	return selected, nil
}

func (s *PerformanceService) submitPerfTask(ctx context.Context, executor model.ExecutorView, taskID string, payload map[string]any, callbackURL, eventURL, callbackToken string, sampleIntervalMs int) error {
	body := map[string]any{
		"taskId":           taskID,
		"type":             "perf",
		"payload":          payload,
		"callbackUrl":      callbackURL,
		"eventUrl":         eventURL,
		"callbackToken":    callbackToken,
		"sampleIntervalMs": sampleIntervalMs,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := strings.TrimRight(executor.Endpoint, "/") + "/tasks"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("执行器返回 %d", resp.StatusCode)
	}
	return nil
}

func (s *PerformanceService) cancelExecutorTask(ctx context.Context, executorID, taskID string) error {
	executor, err := s.executorRepo.GetByID(ctx, executorID)
	if err != nil {
		return err
	}
	url := strings.TrimRight(executor.Endpoint, "/") + "/tasks/" + taskID + "/cancel"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("执行器返回 %d", resp.StatusCode)
	}
	return nil
}

// StartSmoke 下发一次冒烟测试请求（SPEC §8.5），不创建 perf_test_runs、无回调凭据。
func (s *PerformanceService) StartSmoke(ctx context.Context, actor string, req model.PerfSmokeRequest) (string, error) {
	req.TargetURL = strings.TrimSpace(req.TargetURL)
	req.ExecutorID = strings.TrimSpace(req.ExecutorID)
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	if req.TargetURL == "" {
		return "", errors.New("目标地址不能为空")
	}
	if req.ExecutorID == "" {
		return "", errors.New("执行器不能为空")
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	executor, err := s.executorRepo.GetByID(ctx, req.ExecutorID)
	if err != nil {
		return "", errors.New("执行器不存在")
	}
	if executor.Status != "online" {
		return "", errors.New("执行器不在线")
	}
	if !containsString(executor.SupportedTypes, "perf") {
		return "", errors.New("执行器不支持性能测试")
	}
	taskID := generateTaskID()
	payload := map[string]any{
		"mode":    "smoke",
		"target":  req.TargetURL,
		"method":  req.Method,
		"headers": req.Headers,
		"body":    req.Body,
	}
	if err := s.submitSmokeTask(ctx, executor, taskID, payload); err != nil {
		return "", errors.New("下发冒烟测试失败")
	}
	s.recordSmoke(taskID, executor.ExecutorID)
	s.log(ctx, actor, "性能测试冒烟请求", req.TargetURL)
	return taskID, nil
}

// GetSmoke 轮询冒烟测试任务结果。
func (s *PerformanceService) GetSmoke(ctx context.Context, taskID string) (map[string]any, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("冒烟任务 ID 无效")
	}
	executorID, ok := s.lookupSmoke(taskID)
	if !ok {
		return nil, errors.New("冒烟任务不存在")
	}
	result, err := s.queryExecutorTask(ctx, executorID, taskID)
	if err != nil {
		return nil, errors.New("查询冒烟任务失败")
	}
	return result, nil
}

func (s *PerformanceService) submitSmokeTask(ctx context.Context, executor model.ExecutorView, taskID string, payload map[string]any) error {
	body := map[string]any{
		"taskId":  taskID,
		"type":    "perf",
		"payload": payload,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := strings.TrimRight(executor.Endpoint, "/") + "/tasks"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("执行器返回 %d", resp.StatusCode)
	}
	return nil
}

func (s *PerformanceService) recordSmoke(taskID, executorID string) {
	s.smokeMu.Lock()
	defer s.smokeMu.Unlock()
	s.smokeTasks[taskID] = executorID
}

func (s *PerformanceService) lookupSmoke(taskID string) (string, bool) {
	s.smokeMu.Lock()
	defer s.smokeMu.Unlock()
	executorID, ok := s.smokeTasks[taskID]
	return executorID, ok
}

func (s *PerformanceService) notifyRunFinished(ctx context.Context, run model.PerfTestRun, status string) {
	if s.notifier != nil {
		if _, loaded := s.notifiedRuns.Load(run.ID); !loaded {
			level, title, notificationType := "success", "性能测试执行完成", "perf.completed"
			if status == model.PerfRunThresholdFailed {
				level, title, notificationType = "warning", "性能测试未达标", "perf.threshold_failed"
			} else if status == model.PerfRunExecutionFailed || status == model.PerfRunTimedOut || status == model.PerfRunCanceled {
				level, title, notificationType = "error", "性能测试执行失败", "perf.failed"
			}
			if err := s.notifier.Create(ctx, model.NotificationCreate{
				Username:   run.TriggeredBy,
				Type:       notificationType,
				Level:      level,
				Title:      title,
				Content:    fmt.Sprintf("性能测试方案 #%d 执行结束：%s", run.PlanID, status),
				TargetType: "perf_run",
				TargetID:   strconv.FormatInt(run.ID, 10),
				TargetURL:  "#/性能测试/测试报告",
			}); err == nil {
				s.notifiedRuns.Store(run.ID, true)
			}
		}
	}
	if status == model.PerfRunCompleted || status == model.PerfRunThresholdFailed {
		s.notifyPerfDegradation(ctx, run.ID)
	}
}

func (s *PerformanceService) normalizeRequest(ctx context.Context, req model.PerfTestPlanRequest) (model.PerfTestPlanRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.TargetURL = strings.TrimSpace(req.TargetURL)
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.ScenarioType = strings.TrimSpace(req.ScenarioType)
	req.Environment = strings.TrimSpace(req.Environment)
	req.Priority = strings.TrimSpace(req.Priority)
	req.Status = strings.TrimSpace(req.Status)
	req.Owner = strings.TrimSpace(req.Owner)
	req.Tags = strings.TrimSpace(req.Tags)
	req.Description = strings.TrimSpace(req.Description)
	if req.Method == "" {
		req.Method = "GET"
	}
	if req.ScenarioType == "" {
		req.ScenarioType = "baseline"
	}
	if req.Environment == "" {
		req.Environment = "test"
	}
	if req.Priority == "" {
		req.Priority = "P2"
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.ProductID <= 0 || req.Name == "" {
		return req, errors.New("产品名称和方案名称不能为空")
	}
	if len([]rune(req.Name)) > 120 {
		return req, errors.New("方案名称不能超过 120 个字符")
	}
	if !allowed(req.Method, "GET", "POST", "PUT", "DELETE", "PATCH") {
		return req, errors.New("请求方法无效")
	}
	if !allowed(req.ScenarioType, "baseline", "ramp", "peak", "stress", "soak", "mixed") {
		return req, errors.New("场景类型无效")
	}
	if req.EnvironmentID == nil && !allowed(req.Environment, "test", "staging", "production") {
		return req, errors.New("测试环境无效")
	}
	if !allowed(req.Priority, "P0", "P1", "P2", "P3") {
		return req, errors.New("优先级无效")
	}
	if !allowed(req.Status, "draft", "active", "disabled") {
		return req, errors.New("状态无效")
	}
	if !s.performanceRepo.ExistsProduct(ctx, req.ProductID) {
		return req, errors.New("产品不存在")
	}
	if len(req.Headers) == 0 {
		req.Headers = json.RawMessage(`{}`)
	}
	if !isJSONObject(req.Headers) {
		return req, errors.New("请求头必须是 JSON 对象")
	}
	if len(req.LoadConfig) == 0 {
		req.LoadConfig = json.RawMessage(`{}`)
	}
	if req.ScenarioType == "mixed" {
		if err := normalizeMixedLoadConfig(&req.LoadConfig); err != nil {
			return req, err
		}
	}
	if err := validateLoadConfig(req.ScenarioType, req.LoadConfig); err != nil {
		return req, err
	}
	if req.ScenarioType == "mixed" && req.EnvironmentID == nil {
		if err := validateMixedTarget(req.TargetURL, req.LoadConfig); err != nil {
			return req, err
		}
	}
	if req.ScenarioType == "mixed" {
		if err := validateMixedAbsoluteURLs(req.LoadConfig); err != nil {
			return req, err
		}
	}
	if err := normalizeThresholds(&req); err != nil {
		return req, err
	}
	return req, nil
}

func validateMixedAbsoluteURLs(config json.RawMessage) error {
	var value map[string]any
	if err := json.Unmarshal(config, &value); err != nil {
		return errors.New("混合场景负载配置无效")
	}
	items, _ := value["scenarios"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		text, _ := item["url"].(string)
		parsed, err := url.Parse(strings.TrimSpace(text))
		if err == nil && parsed.IsAbs() && !isAbsoluteHTTPURL(text) {
			return errors.New("mixed scenario URL 只允许 HTTP(S) 地址")
		}
	}
	return nil
}

func validateLoadConfig(scenarioType string, config json.RawMessage) error {
	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		return errors.New("负载配置必须是 JSON 对象")
	}
	switch scenarioType {
	case "baseline", "soak":
		if !hasPositiveNumber(cfg, "vus") || !hasNonEmptyString(cfg, "duration") {
			return errors.New("该场景需要设置并发数和时长")
		}
		if !hasVUsWithinLimit(cfg, "vus") {
			return errors.New("vus 不能超过 500")
		}
	case "ramp":
		stages, ok := cfg["stages"].([]any)
		if !ok || len(stages) == 0 {
			return errors.New("梯度压力场景需要配置爬坡阶段")
		}
		for _, raw := range stages {
			stage, ok := raw.(map[string]any)
			if !ok || !hasStageTargetWithinLimit(stage, "target") {
				return errors.New("梯度阶段并发不能超过 500")
			}
		}
	case "peak":
		if !hasPositiveNumber(cfg, "peakVus") || !hasNonEmptyString(cfg, "rampDuration") || !hasNonEmptyString(cfg, "holdDuration") {
			return errors.New("峰值负载场景需要设置峰值并发、爬坡时长和保持时长")
		}
		if !hasVUsWithinLimit(cfg, "peakVus") {
			return errors.New("peakVus 不能超过 500")
		}
	case "stress":
		if !hasPositiveNumber(cfg, "startVus") || !hasPositiveNumber(cfg, "stepVus") || !hasNonEmptyString(cfg, "stepDuration") || !hasPositiveNumber(cfg, "maxVus") {
			return errors.New("极限压力场景需要设置起始并发、步长、每阶时长和最大并发")
		}
		if !hasVUsWithinLimit(cfg, "startVus") || !hasVUsWithinLimit(cfg, "maxVus") {
			return errors.New("stress 并发不能超过 500")
		}
	case "mixed":
		if !hasPositiveNumber(cfg, "vus") || !hasPositiveDuration(cfg, "duration") || !hasPositiveDuration(cfg, "thinkTime") {
			return errors.New("混合场景需要设置正数 vus、duration 和 thinkTime")
		}
		if !hasVUsWithinLimit(cfg, "vus") {
			return errors.New("vus 不能超过 500")
		}
		scenarios, ok := cfg["scenarios"].([]any)
		if !ok || len(scenarios) < 1 || len(scenarios) > 50 {
			return errors.New("混合场景需要配置 1~50 个接口场景")
		}
		seen := map[string]bool{}
		totalWeight := 0.0
		for _, raw := range scenarios {
			item, ok := raw.(map[string]any)
			if !ok {
				return errors.New("混合场景接口配置无效")
			}
			name, _ := item["name"].(string)
			name = strings.TrimSpace(name)
			url, _ := item["url"].(string)
			method, _ := item["method"].(string)
			if rawHeaders, exists := item["headers"]; exists && rawHeaders != nil {
				if _, ok := rawHeaders.(map[string]any); !ok {
					return errors.New("混合场景 headers 必须是对象")
				}
			}
			if rawBody, exists := item["body"]; exists && !isAllowedMixedBody(rawBody) {
				return errors.New("混合场景 body 类型无效")
			}
			weight, ok := numberValue(item["weight"])
			if name == "" || seen[name] || !ok || weight <= 0 || strings.TrimSpace(url) == "" || !allowed(strings.ToUpper(method), "GET", "POST", "PUT", "DELETE", "PATCH") {
				return errors.New("混合场景名称、权重和 URL 必须有效且名称唯一")
			}
			seen[name] = true
			totalWeight += weight
		}
		if math.Abs(totalWeight-1) > 1e-6 {
			return errors.New("混合场景权重总和必须为 1")
		}
	}
	return nil
}

func normalizeThresholds(req *model.PerfTestPlanRequest) error {
	for index := range req.Thresholds {
		item := &req.Thresholds[index]
		item.Metric = strings.TrimSpace(item.Metric)
		item.Aggregation = strings.TrimSpace(item.Aggregation)
		item.Operator = strings.TrimSpace(item.Operator)
		item.DelayAbortEval = strings.TrimSpace(item.DelayAbortEval)
		if item.Metric == "" {
			return errors.New("阈值指标不能为空")
		}
		if item.Aggregation == "" {
			return errors.New("阈值聚合方式不能为空")
		}
		if !allowed(item.Operator, "<", "<=", ">", ">=") {
			return errors.New("阈值运算符无效")
		}
		if !allowedThreshold(item.Metric, item.Aggregation) {
			return errors.New("阈值指标或聚合方式无效")
		}
		if !item.AbortOnFail {
			item.DelayAbortEval = ""
		} else if item.DelayAbortEval != "" {
			if _, err := perfParseDuration(item.DelayAbortEval); err != nil {
				return errors.New("delayAbortEval 必须是合法 k6 duration")
			}
		}
	}
	return nil
}

func normalizeMixedLoadConfig(raw *json.RawMessage) error {
	var cfg map[string]any
	if err := json.Unmarshal(*raw, &cfg); err != nil {
		return errors.New("负载配置必须是 JSON 对象")
	}
	// P1 不实现随机参数/冷热流量扩展，明确关闭而不是静默执行。
	cfg["randomizeParams"] = false
	cfg["coldHotMix"] = false
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return errors.New("混合场景配置无效")
	}
	*raw = encoded
	return nil
}

func hasPositiveDuration(cfg map[string]any, key string) bool {
	value, ok := cfg[key].(string)
	if !ok {
		return false
	}
	parsed, err := perfParseDuration(value)
	return err == nil && parsed > 0
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case json.Number:
		result, err := typed.Float64()
		return result, err == nil
	default:
		return 0, false
	}
}

func isAllowedMixedBody(value any) bool {
	switch value.(type) {
	case nil, string, bool, float64, []any, map[string]any:
		return true
	default:
		return false
	}
}

func allowedThreshold(metric, aggregation string) bool {
	allowedAggregations := map[string]map[string]bool{
		"http_req_duration":        {"avg": true, "min": true, "max": true, "p(50)": true, "p(90)": true, "p(95)": true, "p(99)": true},
		"http_req_failed":          {"rate": true},
		"http_reqs":                {"count": true, "rate": true},
		"http_req_waiting":         {"avg": true, "p(90)": true, "p(95)": true, "p(99)": true},
		"http_req_blocked":         {"avg": true, "p(90)": true, "p(95)": true},
		"http_req_connecting":      {"avg": true, "p(90)": true, "p(95)": true},
		"http_req_tls_handshaking": {"avg": true, "p(90)": true, "p(95)": true},
		"http_req_receiving":       {"avg": true, "p(90)": true, "p(95)": true},
		"http_req_sending":         {"avg": true, "p(90)": true, "p(95)": true},
		"iterations":               {"count": true, "rate": true},
		"iteration_duration":       {"avg": true, "p(90)": true, "p(95)": true, "p(99)": true},
	}
	return allowedAggregations[metric][aggregation]
}

func buildPlanSnapshotAndHash(plan model.PerfTestPlan) (json.RawMessage, string, error) {
	return buildPlanSnapshotAndHashForRun(plan, plan)
}

func buildPlanSnapshotAndHashForRun(original, resolved model.PerfTestPlan) (json.RawMessage, string, error) {
	headers, err := canonicalRaw(original.Headers)
	if err != nil {
		return nil, "", err
	}
	originalLoadConfig, err := canonicalRaw(original.LoadConfig)
	if err != nil {
		return nil, "", err
	}
	resolvedLoadConfig, err := canonicalRaw(resolved.LoadConfig)
	if err != nil {
		return nil, "", err
	}
	snapshot := map[string]any{
		"planId":               original.ID,
		"name":                 original.Name,
		"targetUrl":            original.TargetURL,
		"method":               original.Method,
		"headers":              headers,
		"body":                 original.Body,
		"scenarioType":         original.ScenarioType,
		"loadConfig":           originalLoadConfig,
		"environment":          resolved.Environment,
		"environmentId":        resolved.EnvironmentID,
		"environmentName":      resolved.EnvironmentName,
		"environmentBaseUrl":   resolved.EnvironmentBaseURL,
		"environmentDeployEnv": resolved.EnvironmentDeployEnv,
		"resolvedTargetUrl":    resolved.TargetURL,
		"resolvedLoadConfig":   resolvedLoadConfig,
		"thresholds":           original.Thresholds,
	}
	canonical, err := json.Marshal(snapshot) // map key 自动排序
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]), nil
}

// buildPerfPayload 从执行快照展开执行器期望的 snake_case 负载字段（SPEC §4.2）。
// 执行器 perf_runner.generate_script 读取 scenario_type/load_config/target/method/headers/body/thresholds。
func buildPerfPayload(run model.PerfTestRun) (map[string]any, error) {
	snapshot := map[string]any{}
	if len(run.PlanSnapshot) > 0 {
		if err := json.Unmarshal(run.PlanSnapshot, &snapshot); err != nil {
			return nil, err
		}
	}
	scenarioType := run.ScenarioType
	if value, ok := snapshot["scenarioType"].(string); ok && value != "" {
		scenarioType = value
	}
	target := snapshot["resolvedTargetUrl"]
	if target == nil {
		target = snapshot["targetUrl"]
	}
	loadConfig := snapshot["resolvedLoadConfig"]
	if loadConfig == nil {
		loadConfig = snapshot["loadConfig"]
	}
	return map[string]any{
		"runId":                run.ID,
		"planId":               run.PlanID,
		"environment":          run.Environment,
		"scenario_type":        scenarioType,
		"target":               target,
		"method":               snapshot["method"],
		"headers":              snapshot["headers"],
		"body":                 snapshot["body"],
		"load_config":          loadConfig,
		"environment_id":       snapshot["environmentId"],
		"environment_name":     snapshot["environmentName"],
		"environment_base_url": snapshot["environmentBaseUrl"],
		"thresholds":           snapshot["thresholds"],
	}, nil
}

func canonicalRaw(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// computeExpectedFinishAt 按场景负载计算最晚完成时间（SPEC §5.1）。
func computeExpectedFinishAt(plan model.PerfTestPlan, requestedAt time.Time) time.Time {
	return requestedAt.Add(perfTotalDuration(plan.ScenarioType, plan.LoadConfig) + perfTimeoutBuffer)
}

func (s *PerformanceService) expectedFinishAtForRun(run model.PerfTestRun, startedAt time.Time) time.Time {
	return startedAt.Add(perfDurationForRun(run) + perfTimeoutBuffer)
}

func perfDurationForRun(run model.PerfTestRun) time.Duration {
	plan := model.PerfTestPlan{ScenarioType: run.ScenarioType}
	var snapshot struct {
		ScenarioType       string          `json:"scenarioType"`
		LoadConfig         json.RawMessage `json:"loadConfig"`
		ResolvedLoadConfig json.RawMessage `json:"resolvedLoadConfig"`
	}
	if json.Unmarshal(run.PlanSnapshot, &snapshot) == nil {
		if snapshot.ScenarioType != "" {
			plan.ScenarioType = snapshot.ScenarioType
		}
		plan.LoadConfig = snapshot.ResolvedLoadConfig
		if len(plan.LoadConfig) == 0 {
			plan.LoadConfig = snapshot.LoadConfig
		}
	}
	return perfTotalDuration(plan.ScenarioType, plan.LoadConfig)
}

func perfSampleIntervalMs(run model.PerfTestRun) int {
	durationMs := float64(perfDurationForRun(run).Milliseconds())
	interval := int(math.Ceil(durationMs/3000.0)) * 1000
	if interval < 1000 {
		return 1000
	}
	if interval > 5000 {
		return 5000
	}
	return interval
}

// perfTotalDuration 解析 load_config 计算压测总时长，无法解析时用保守默认。
func perfTotalDuration(scenarioType string, config json.RawMessage) time.Duration {
	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		return perfDefaultDuration
	}
	switch scenarioType {
	case "baseline", "soak":
		if value, ok := cfg["duration"].(string); ok {
			if parsed, err := perfParseDuration(value); err == nil {
				return parsed
			}
		}
	case "ramp":
		return sumStagesDuration(cfg)
	case "stress":
		return stressDuration(cfg)
	case "peak":
		if stages, ok := cfg["stages"].([]any); ok && len(stages) > 0 {
			if total := sumStageList(stages); total > 0 {
				return total
			}
		}
		return sumDurationFields(cfg, "rampDuration", "holdDuration", "rampDownDuration")
	case "mixed":
		if value, ok := cfg["duration"].(string); ok {
			if parsed, err := perfParseDuration(value); err == nil {
				return parsed
			}
		}
		return perfDefaultDuration
	}
	return perfDefaultDuration
}

func sumStagesDuration(cfg map[string]any) time.Duration {
	stages, ok := cfg["stages"].([]any)
	if !ok || len(stages) == 0 {
		return perfDefaultDuration
	}
	return sumStageList(stages)
}

func stressDuration(cfg map[string]any) time.Duration {
	start, startOK := numberValue(cfg["startVus"])
	step, stepOK := numberValue(cfg["stepVus"])
	max, maxOK := numberValue(cfg["maxVus"])
	duration, durationOK := cfg["stepDuration"].(string)
	if !startOK || !stepOK || !maxOK || !durationOK || step <= 0 || max < start {
		return perfDefaultDuration
	}
	count := 1 + int(math.Ceil((max-start)/step))
	parsed, err := perfParseDuration(duration)
	if err != nil || parsed <= 0 {
		return perfDefaultDuration
	}
	return time.Duration(count) * parsed
}

func sumStageList(stages []any) time.Duration {
	var total time.Duration
	for _, raw := range stages {
		stage, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value, ok := stage["duration"].(string)
		if !ok {
			continue
		}
		if parsed, err := perfParseDuration(value); err == nil {
			total += parsed
		}
	}
	if total <= 0 {
		return perfDefaultDuration
	}
	return total
}

func sumDurationFields(cfg map[string]any, keys ...string) time.Duration {
	var total time.Duration
	for _, key := range keys {
		value, ok := cfg[key].(string)
		if !ok {
			continue
		}
		if parsed, err := perfParseDuration(value); err == nil {
			total += parsed
		}
	}
	if total <= 0 {
		return perfDefaultDuration
	}
	return total
}

func perfParseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" || !perfDurationPattern.MatchString(value) {
		return 0, errors.New("duration 为空")
	}
	return time.ParseDuration(value)
}

func resolveFinalStatus(status string, summary json.RawMessage) string {
	switch status {
	case model.PerfRunThresholdFailed:
		return model.PerfRunThresholdFailed
	case model.PerfRunExecutionFailed:
		return model.PerfRunExecutionFailed
	case model.PerfRunTimedOut:
		return model.PerfRunTimedOut
	case model.PerfRunCanceled:
		return model.PerfRunCanceled
	case model.PerfRunCompleted:
		if thresholdsFailed(summary) {
			return model.PerfRunThresholdFailed
		}
		return model.PerfRunCompleted
	default:
		return status
	}
}

func thresholdsFailed(summary json.RawMessage) bool {
	if len(summary) == 0 {
		return false
	}
	var root map[string]any
	if err := json.Unmarshal(summary, &root); err != nil {
		return false
	}
	thresholds, ok := root["thresholds"]
	if !ok {
		return false
	}
	return containsOKFalse(thresholds)
}

func containsOKFalse(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if okVal, exists := v["ok"]; exists {
			if okBool, isBool := okVal.(bool); isBool && !okBool {
				return true
			}
		}
		for _, item := range v {
			if containsOKFalse(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if containsOKFalse(item) {
				return true
			}
		}
	}
	return false
}

func isFinalRunStatus(status string) bool {
	switch status {
	case model.PerfRunCompleted, model.PerfRunThresholdFailed, model.PerfRunExecutionFailed, model.PerfRunTimedOut, model.PerfRunCanceled:
		return true
	}
	return false
}

func defaultFailureStage(status string) string {
	switch status {
	case "timed_out":
		return model.PerfFailureTimeout
	case "canceled":
		return model.PerfFailureCancel
	case "execution_failed":
		return model.PerfFailureK6Runtime
	default:
		return ""
	}
}

func hasPositiveNumber(cfg map[string]any, key string) bool {
	value, ok := cfg[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case float64:
		return v > 0
	case int:
		return v > 0
	case json.Number:
		n, err := v.Float64()
		return err == nil && n > 0
	}
	return false
}

func hasVUsWithinLimit(cfg map[string]any, key string) bool {
	value, ok := numberValue(cfg[key])
	return ok && value > 0 && value <= perfMaxVUs && math.Trunc(value) == value
}

func hasStageTargetWithinLimit(cfg map[string]any, key string) bool {
	value, ok := numberValue(cfg[key])
	return ok && value >= 0 && value <= perfMaxVUs && math.Trunc(value) == value
}

func validateMixedTarget(target string, config json.RawMessage) error {
	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		return errors.New("负载配置必须是 JSON 对象")
	}
	scenarios, _ := cfg["scenarios"].([]any)
	needsTarget := false
	for _, raw := range scenarios {
		item, _ := raw.(map[string]any)
		value, _ := item["url"].(string)
		if parsed, parseErr := url.Parse(strings.TrimSpace(value)); parseErr == nil && parsed.IsAbs() && !isAbsoluteHTTPURL(value) {
			return errors.New("mixed scenario URL 只允许 HTTP(S) 地址")
		}
		if !isAbsoluteHTTPURL(value) {
			needsTarget = true
			break
		}
	}
	if needsTarget && !isAbsoluteHTTPURL(target) {
		return errors.New("mixed 含相对 URL 时必须提供合法 HTTP(S) target")
	}
	return nil
}

func isAbsoluteHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func hasNonEmptyString(cfg map[string]any, key string) bool {
	value, ok := cfg[key]
	if !ok {
		return false
	}
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func generateCallbackToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := perfRandRead(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sanitizePerfText(value string) string {
	text := perfSensitiveQueryPattern.ReplaceAllString(value, `${1}***`)
	return perfSensitiveTextPattern.ReplaceAllString(text, `${1}***`)
}

func sanitizePerfJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return json.RawMessage(`{}`)
	}
	encoded, err := json.Marshal(sanitizePerfValue(value))
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func normalizePerfSeries(raw json.RawMessage) json.RawMessage {
	series := model.PerfSeries{Version: 1, Points: []model.PerfSampleEvent{}, StatusCodes: map[string]int{}, ErrorTopN: []model.PerfErrorTop{}, Thresholds: []model.PerfSampleThreshold{}}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &series)
	}
	if series.Version == 0 {
		series.Version = 1
	}
	if len(series.Points) > 3000 {
		series.Points = downsamplePerfPoints(series.Points, 3000)
	}
	if series.StatusCodes == nil {
		series.StatusCodes = map[string]int{}
	}
	if series.ErrorTopN == nil {
		series.ErrorTopN = []model.PerfErrorTop{}
	}
	if series.Thresholds == nil {
		series.Thresholds = []model.PerfSampleThreshold{}
	}
	encoded, err := json.Marshal(series)
	if err != nil {
		return json.RawMessage(`{"version":1,"points":[],"statusCodes":{},"errorTopN":[],"thresholds":[]}`)
	}
	return sanitizePerfJSON(encoded)
}

func forcePerfSeriesPartial(raw json.RawMessage) json.RawMessage {
	var series model.PerfSeries
	_ = json.Unmarshal(normalizePerfSeries(raw), &series)
	series.Partial = true
	encoded, _ := json.Marshal(series)
	return sanitizePerfJSON(encoded)
}

func sanitizePerfValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			lower := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "password") || strings.Contains(lower, "passwd") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "apikey") {
				result[key] = "***"
			} else {
				result[key] = sanitizePerfValue(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = sanitizePerfValue(item)
		}
		return result
	case string:
		text := sanitizePerfText(typed)
		trimmed := strings.TrimSpace(text)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var nested any
			if json.Unmarshal([]byte(trimmed), &nested) == nil {
				if encoded, err := json.Marshal(sanitizePerfValue(nested)); err == nil {
					return string(encoded)
				}
			}
		}
		return text
	default:
		return value
	}
}

func isJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var obj map[string]any
	return json.Unmarshal(raw, &obj) == nil
}

func (s *PerformanceService) log(ctx context.Context, actor, action, target string) {
	if s.systemRepo != nil {
		_ = s.systemRepo.LogOperation(ctx, actor, action, target)
	}
}
