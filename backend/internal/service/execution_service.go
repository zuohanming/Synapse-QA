package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

type ExecutionRepository interface {
	CreateRun(ctx context.Context, req model.ExecutionRunRequest, triggeredBy string) (model.ExecutionRun, error)
	GetRun(ctx context.Context, id int64) (model.ExecutionRun, error)
	ListRuns(ctx context.Context, filter model.ExecutionRunFilter, page, pageSize int) ([]model.ExecutionRun, int64, error)
	UpdateRunStatus(ctx context.Context, id int64, status string, summary json.RawMessage) error
	StartRun(ctx context.Context, id int64) error
	CreateTask(ctx context.Context, runID int64, taskID string, caseID int64, executorID, taskType, callbackURL string, payload json.RawMessage) (model.ExecutionTask, error)
	GetTaskByTaskID(ctx context.Context, taskID string) (model.ExecutionTask, error)
	GetTask(ctx context.Context, id int64) (model.ExecutionTask, error)
	ListTasksByRun(ctx context.Context, runID int64) ([]model.ExecutionTask, error)
	UpdateTaskStatus(ctx context.Context, id int64, status string) error
	UpdateTaskResult(ctx context.Context, id int64, result json.RawMessage) error
	CreateLog(ctx context.Context, taskID int64, level, message string) error
	ListLogs(ctx context.Context, taskID int64) ([]model.ExecutionLog, error)
}

type ExecutorRepository interface {
	List(ctx context.Context) ([]model.ExecutorView, error)
	GetByID(ctx context.Context, executorID string) (model.ExecutorView, error)
}

type TestCaseReader interface {
	Get(ctx context.Context, id int64) (model.TestCaseDetail, error)
}

// ExecutionService 负责执行批次创建、执行器调度和回调处理。
type ExecutionService struct {
	executionRepo ExecutionRepository
	executorRepo  ExecutorRepository
	testCaseRepo  TestCaseReader
	systemRepo    OperationLogger
	callbackBase  string
	httpClient    *http.Client
}

func NewExecutionService(executionRepo ExecutionRepository, executorRepo ExecutorRepository, testCaseRepo TestCaseReader, systemRepo OperationLogger, callbackBase string) *ExecutionService {
	return &ExecutionService{
		executionRepo: executionRepo,
		executorRepo:  executorRepo,
		testCaseRepo:  testCaseRepo,
		systemRepo:    systemRepo,
		callbackBase:  strings.TrimRight(callbackBase, "/"),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateRun 创建执行批次并下发任务。
func (s *ExecutionService) CreateRun(ctx context.Context, actor string, req model.ExecutionRunRequest) (model.ExecutionRunDetail, error) {
	req, err := normalizeExecutionRunRequest(req)
	if err != nil {
		return model.ExecutionRunDetail{}, err
	}

	run, err := s.executionRepo.CreateRun(ctx, req, actor)
	if err != nil {
		return model.ExecutionRunDetail{}, errors.New("创建执行批次失败")
	}

	executor, err := s.pickExecutor(ctx, req.RunType)
	if err != nil {
		return model.ExecutionRunDetail{}, err
	}

	if err := s.executionRepo.StartRun(ctx, run.ID); err != nil {
		return model.ExecutionRunDetail{}, errors.New("启动执行批次失败")
	}
	_run, err := s.executionRepo.GetRun(ctx, run.ID)
	if err != nil {
		return model.ExecutionRunDetail{}, errors.New("读取执行批次失败")
	}
	run = _run

	var tasks []model.ExecutionTask
	for _, caseID := range req.CaseIDs {
		task, err := s.dispatchCaseTask(ctx, run.ID, caseID, executor, req.RunType)
		if err != nil {
			_ = s.systemRepo.LogOperation(ctx, actor, "创建执行任务失败", fmt.Sprintf("run=%d case=%d err=%s", run.ID, caseID, err.Error()))
			continue
		}
		tasks = append(tasks, task)
	}

	if len(tasks) == 0 && len(req.CaseIDs) > 0 {
		_ = s.executionRepo.UpdateRunStatus(ctx, run.ID, "failed", summaryJSON(model.ExecutionSummary{Total: int64(len(req.CaseIDs))}))
	}

	_ = s.systemRepo.LogOperation(ctx, actor, "创建执行批次", fmt.Sprintf("run=%d cases=%d", run.ID, len(req.CaseIDs)))
	return s.GetRun(ctx, run.ID)
}

// GetRun 查询执行批次详情，包含任务列表。
func (s *ExecutionService) GetRun(ctx context.Context, id int64) (model.ExecutionRunDetail, error) {
	run, err := s.executionRepo.GetRun(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ExecutionRunDetail{}, errors.New("执行批次不存在")
		}
		return model.ExecutionRunDetail{}, errors.New("查询执行批次失败")
	}
	tasks, err := s.executionRepo.ListTasksByRun(ctx, id)
	if err != nil {
		return model.ExecutionRunDetail{}, errors.New("查询任务列表失败")
	}
	return model.ExecutionRunDetail{ExecutionRun: run, Tasks: tasks}, nil
}

// ListRuns 分页查询执行批次。
func (s *ExecutionService) ListRuns(ctx context.Context, filter model.ExecutionRunFilter, page, pageSize int) (model.PageResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	items, total, err := s.executionRepo.ListRuns(ctx, filter, page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

// CancelRun 取消执行批次。
func (s *ExecutionService) CancelRun(ctx context.Context, actor string, id int64) error {
	run, err := s.executionRepo.GetRun(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("执行批次不存在")
		}
		return errors.New("查询执行批次失败")
	}
	if run.Status == "completed" || run.Status == "failed" || run.Status == "canceled" {
		return errors.New("当前批次已结束")
	}

	tasks, err := s.executionRepo.ListTasksByRun(ctx, id)
	if err != nil {
		return errors.New("查询任务列表失败")
	}
	for _, task := range tasks {
		if task.Status == "queued" || task.Status == "running" {
			_ = s.executionRepo.UpdateTaskStatus(ctx, task.ID, "canceled")
			// 向执行器发送取消请求，失败不影响本地状态。
			_ = s.cancelExecutorTask(ctx, task)
		}
	}
	_ = s.executionRepo.UpdateRunStatus(ctx, id, "canceled", summaryJSON(model.ExecutionSummary{Total: int64(len(tasks))}))
	_ = s.systemRepo.LogOperation(ctx, actor, "取消执行批次", strconv.FormatInt(id, 10))
	return nil
}

// HandleCallback 处理执行器回调，更新任务状态和批次聚合状态。
func (s *ExecutionService) HandleCallback(ctx context.Context, req model.ExecutionCallbackRequest) error {
	if req.TaskID == "" {
		return errors.New("taskId 不能为空")
	}
	task, err := s.executionRepo.GetTaskByTaskID(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("任务不存在")
		}
		return errors.New("查询任务失败")
	}
	if task.Status == "success" || task.Status == "failed" || task.Status == "canceled" {
		return nil
	}

	status := strings.ToLower(req.Status)
	if status == "success" {
		status = "success"
	} else if status == "failed" {
		status = "failed"
	} else if status == "canceled" {
		status = "canceled"
	} else {
		status = "failed"
	}
	if err := s.executionRepo.UpdateTaskStatus(ctx, task.ID, status); err != nil {
		return errors.New("更新任务状态失败")
	}
	if err := s.executionRepo.UpdateTaskResult(ctx, task.ID, req.Result); err != nil {
		return errors.New("更新任务结果失败")
	}
	if err := s.aggregateRunStatus(ctx, task.RunID); err != nil {
		return errors.New("聚合批次状态失败")
	}
	return nil
}

// ListLogs 查询任务日志。
func (s *ExecutionService) ListLogs(ctx context.Context, taskID int64) ([]model.ExecutionLog, error) {
	_, err := s.executionRepo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("任务不存在")
		}
		return nil, errors.New("查询任务失败")
	}
	return s.executionRepo.ListLogs(ctx, taskID)
}

// pickExecutor 选择在线且支持指定类型的执行器。
func (s *ExecutionService) pickExecutor(ctx context.Context, taskType string) (model.ExecutorView, error) {
	executors, err := s.executorRepo.List(ctx)
	if err != nil {
		return model.ExecutorView{}, errors.New("查询执行器失败")
	}
	var candidates []model.ExecutorView
	for _, item := range executors {
		if item.Status != "online" {
			continue
		}
		supported := false
		for _, t := range item.SupportedTypes {
			if t == taskType {
				supported = true
				break
			}
		}
		if !supported {
			continue
		}
		candidates = append(candidates, item)
	}
	if len(candidates) == 0 {
		return model.ExecutorView{}, errors.New("没有可用的在线执行器")
	}
	// 选择运行任务数最少的执行器。
	selected := candidates[0]
	for _, item := range candidates[1:] {
		if item.RunningTasks < selected.RunningTasks {
			selected = item
		}
	}
	return selected, nil
}

// dispatchCaseTask 为单个用例生成并下发任务。
func (s *ExecutionService) dispatchCaseTask(ctx context.Context, runID int64, caseID int64, executor model.ExecutorView, runType string) (model.ExecutionTask, error) {
	caseDetail, err := s.testCaseRepo.Get(ctx, caseID)
	if err != nil {
		return model.ExecutionTask{}, errors.New("读取用例失败")
	}
	if caseDetail.Status == "deleted" {
		return model.ExecutionTask{}, errors.New("用例已删除")
	}

	payload := s.buildTaskPayload(caseDetail, runType)
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return model.ExecutionTask{}, errors.New("生成任务 payload 失败")
	}

	taskID := generateTaskID()
	callbackURL := fmt.Sprintf("%s/api/executions/tasks/%s/callback", s.callbackBase, taskID)
	task, err := s.executionRepo.CreateTask(ctx, runID, taskID, caseID, executor.ExecutorID, runType, callbackURL, payloadJSON)
	if err != nil {
		return model.ExecutionTask{}, errors.New("保存任务失败")
	}

	if err := s.submitToExecutor(ctx, executor.Endpoint, taskID, runType, payload, callbackURL); err != nil {
		return model.ExecutionTask{}, errors.New("下发任务到执行器失败")
	}
	return task, nil
}

// buildTaskPayload 根据用例详情和执行类型生成执行器 payload。
func (s *ExecutionService) buildTaskPayload(caseDetail model.TestCaseDetail, runType string) map[string]any {
	if runType == "api" {
		return map[string]any{
			"caseId":   caseDetail.ID,
			"caseName": caseDetail.Name,
			"url":      caseDetail.Preconditions,
		}
	}
	// 默认 UI 类型：将用例步骤映射为 Playwright action 序列。
	actions := []map[string]any{}
	for _, step := range caseDetail.Steps {
		action := normalizePlaywrightAction(step.Action)
		if action == "" {
			continue
		}
		item := map[string]any{"action": action}
		if step.Locator != "" {
			item["selector"] = step.Locator
		}
		if step.Value != "" {
			if action == "assertText" || action == "assertTitle" {
				item["text"] = step.Value
			} else if action == "goto" {
				item["url"] = step.Value
			} else {
				item["value"] = step.Value
			}
		}
		actions = append(actions, item)
	}
	return map[string]any{
		"caseId":   caseDetail.ID,
		"caseName": caseDetail.Name,
		"url":      caseDetail.Preconditions,
		"actions":  actions,
	}
}

func normalizePlaywrightAction(action string) string {
	switch strings.TrimSpace(action) {
	case "goto", "click", "fill", "press", "waitForSelector", "assertText", "assertTitle", "screenshot":
		return strings.TrimSpace(action)
	case "w_click", "w_force_click", "w_is_click":
		return "click"
	case "w_input", "w_clear_input":
		return "fill"
	case "w_open":
		return "goto"
	case "w_wait_element":
		return "waitForSelector"
	case "w_assert_text":
		return "assertText"
	default:
		return ""
	}
}

// submitToExecutor 向执行器提交任务。
func (s *ExecutionService) submitToExecutor(ctx context.Context, endpoint, taskID, taskType string, payload map[string]any, callbackURL string) error {
	body := map[string]any{
		"taskId":      taskID,
		"type":        taskType,
		"payload":     payload,
		"callbackUrl": callbackURL,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := strings.TrimRight(endpoint, "/") + "/tasks"
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
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("执行器返回 %d", resp.StatusCode)
	}
	return nil
}

// cancelExecutorTask 向执行器发送取消请求。
func (s *ExecutionService) cancelExecutorTask(ctx context.Context, task model.ExecutionTask) error {
	executor, err := s.executorRepo.GetByID(ctx, task.ExecutorID)
	if err != nil {
		return err
	}
	url := strings.TrimRight(executor.Endpoint, "/") + "/tasks/" + task.TaskID + "/cancel"
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

// aggregateRunStatus 聚合任务状态，更新执行批次状态。
func (s *ExecutionService) aggregateRunStatus(ctx context.Context, runID int64) error {
	tasks, err := s.executionRepo.ListTasksByRun(ctx, runID)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return nil
	}
	var total, passed, failed, skipped int64
	allFinished := true
	for _, task := range tasks {
		total++
		switch task.Status {
		case "success":
			passed++
		case "failed":
			failed++
		case "canceled":
			skipped++
		default:
			allFinished = false
		}
	}
	if !allFinished {
		return nil
	}
	status := "completed"
	if failed > 0 {
		status = "failed"
	} else if skipped > 0 && passed == 0 {
		status = "canceled"
	}
	return s.executionRepo.UpdateRunStatus(ctx, runID, status, summaryJSON(model.ExecutionSummary{Total: total, Passed: passed, Failed: failed, Skipped: skipped}))
}

func normalizeExecutionRunRequest(req model.ExecutionRunRequest) (model.ExecutionRunRequest, error) {
	req.RunType = strings.TrimSpace(strings.ToLower(req.RunType))
	if req.RunType == "" {
		req.RunType = "ui"
	}
	if req.RunType != "ui" && req.RunType != "api" && req.RunType != "unit" && req.RunType != "script" && req.RunType != "noop" {
		return req, errors.New("执行类型无效")
	}
	if len(req.CaseIDs) == 0 {
		return req, errors.New("至少选择一个用例")
	}
	return req, nil
}

func summaryJSON(summary model.ExecutionSummary) json.RawMessage {
	data, _ := json.Marshal(summary)
	return data
}

func generateTaskID() string {
	return fmt.Sprintf("task-%d-%d", time.Now().UnixNano(), time.Now().UnixMicro())
}
