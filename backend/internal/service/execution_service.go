package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	CountActiveTasksByExecutor(ctx context.Context, executorID string) (int, error)
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
	wakeScheduler chan struct{}
	dispatchMu    sync.Mutex
}

const executionDispatchBatchSize = 20

func NewExecutionService(executionRepo ExecutionRepository, executorRepo ExecutorRepository, testCaseRepo TestCaseReader, systemRepo OperationLogger, callbackBase string) *ExecutionService {
	return &ExecutionService{
		executionRepo: executionRepo,
		executorRepo:  executorRepo,
		testCaseRepo:  testCaseRepo,
		systemRepo:    systemRepo,
		callbackBase:  strings.TrimRight(callbackBase, "/"),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		wakeScheduler: make(chan struct{}, 1),
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

	if err := s.executionRepo.StartRun(ctx, run.ID); err != nil {
		return model.ExecutionRunDetail{}, errors.New("启动执行批次失败")
	}
	_run, err := s.executionRepo.GetRun(ctx, run.ID)
	if err != nil {
		return model.ExecutionRunDetail{}, errors.New("读取执行批次失败")
	}
	run = _run

	if err := s.dispatchRun(ctx, run); err != nil {
		return model.ExecutionRunDetail{}, err
	}
	s.wake()

	_ = s.systemRepo.LogOperation(ctx, actor, "创建执行批次", fmt.Sprintf("run=%d cases=%d", run.ID, len(req.CaseIDs)))
	return s.GetRun(ctx, run.ID)
}

// StartScheduler 启动持久化执行队列调度器，API 重启后会继续处理运行中的批次。
func (s *ExecutionService) StartScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-s.wakeScheduler:
			}
			if err := s.DispatchPending(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("执行队列调度失败：%v", err)
			}
		}
	}()
	s.wake()
}

func (s *ExecutionService) wake() {
	select {
	case s.wakeScheduler <- struct{}{}:
	default:
	}
}

// DispatchPending 扫描运行中批次，并按执行器容量分批下发。
func (s *ExecutionService) DispatchPending(ctx context.Context) error {
	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	runs, _, err := s.executionRepo.ListRuns(ctx, model.ExecutionRunFilter{Status: "running"}, 1, 100)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.ID == 0 {
			continue
		}
		if err := s.dispatchRun(ctx, run); err != nil {
			// 执行器暂时不可用时保留批次，下一轮继续调度。
			continue
		}
	}
	return nil
}

func (s *ExecutionService) dispatchRun(ctx context.Context, run model.ExecutionRun) error {
	tasks, err := s.executionRepo.ListTasksByRun(ctx, run.ID)
	if err != nil {
		return err
	}
	dispatched := make(map[int64]bool, len(tasks))
	for _, task := range tasks {
		dispatched[task.CaseID] = true
	}
	remaining := make([]int64, 0)
	for _, caseID := range run.CaseIDs {
		if !dispatched[caseID] {
			remaining = append(remaining, caseID)
		}
	}
	if len(remaining) == 0 {
		return s.aggregateRunStatus(ctx, run.ID)
	}
	executor, err := s.pickExecutor(ctx, run.RunType)
	if err != nil {
		return err
	}
	active, err := s.executionRepo.CountActiveTasksByExecutor(ctx, executor.ExecutorID)
	if err != nil {
		return err
	}
	maxWorkers := executor.MaxWorkers
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	queueLimit := maxWorkers * 2
	slots := queueLimit - active
	if slots <= 0 {
		return nil
	}
	if slots > executionDispatchBatchSize {
		slots = executionDispatchBatchSize
	}
	if slots > len(remaining) {
		slots = len(remaining)
	}
	for _, caseID := range remaining[:slots] {
		headless := run.Headless
		task, err := s.dispatchCaseTask(ctx, run.ID, caseID, executor, run.RunType, &headless)
		if err != nil {
			_ = s.recordDispatchFailure(ctx, run, caseID, executor, task, err)
		}
	}
	return s.aggregateRunStatus(ctx, run.ID)
}

func (s *ExecutionService) recordDispatchFailure(ctx context.Context, run model.ExecutionRun, caseID int64, executor model.ExecutorView, task model.ExecutionTask, dispatchErr error) error {
	if task.ID == 0 {
		taskID := generateTaskID()
		callbackURL := fmt.Sprintf("%s/api/executions/tasks/%s/callback", s.callbackBase, taskID)
		var err error
		task, err = s.executionRepo.CreateTask(ctx, run.ID, taskID, caseID, executor.ExecutorID, run.RunType, callbackURL, json.RawMessage(`{}`))
		if err != nil {
			return err
		}
	}
	result, _ := json.Marshal(model.ExecutionTaskResult{ExitCode: 1, Error: dispatchErr.Error()})
	_ = s.executionRepo.UpdateTaskResult(ctx, task.ID, result)
	return s.executionRepo.UpdateTaskStatus(ctx, task.ID, "failed")
}

// StartDebug 将页面步骤画布作为一次轻量 UI 任务下发，不写入正式执行批次。
func (s *ExecutionService) StartDebug(ctx context.Context, actor string, req model.ExecutionDebugRequest) (model.ExecutionDebugStart, error) {
	if len(req.Actions) == 0 {
		return model.ExecutionDebugStart{}, errors.New("调试画布不能为空")
	}
	executor, err := s.pickExecutor(ctx, "ui")
	if err != nil {
		return model.ExecutionDebugStart{}, err
	}
	taskID := generateTaskID()
	payload := map[string]any{
		"url":            strings.TrimSpace(req.URL),
		"actions":        req.Actions,
		"timeoutSeconds": req.TimeoutSeconds,
		"headless":       true,
	}
	if req.Headless != nil {
		payload["headless"] = *req.Headless
	}
	if req.TimeoutSeconds <= 0 {
		payload["timeoutSeconds"] = 30
	}
	if err := s.submitToExecutor(ctx, executor.Endpoint, taskID, "ui", payload, ""); err != nil {
		return model.ExecutionDebugStart{}, errors.New("下发调试任务失败")
	}
	_ = s.systemRepo.LogOperation(ctx, actor, "调试页面步骤", taskID)
	return model.ExecutionDebugStart{TaskID: taskID, ExecutorID: executor.ExecutorID, Status: "queued"}, nil
}

// GetDebugTask 从执行器读取轻量调试任务的实时状态。
func (s *ExecutionService) GetDebugTask(ctx context.Context, executorID, taskID string) (map[string]any, error) {
	if strings.TrimSpace(executorID) == "" || strings.TrimSpace(taskID) == "" {
		return nil, errors.New("调试任务参数无效")
	}
	executor, err := s.executorRepo.GetByID(ctx, executorID)
	if err != nil {
		return nil, errors.New("执行器不存在")
	}
	url := strings.TrimRight(executor.Endpoint, "/") + "/tasks/" + taskID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New("创建调试查询请求失败")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, errors.New("查询调试任务失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("调试任务不存在")
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("执行器返回 %d", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.New("解析调试结果失败")
	}
	return result, nil
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
	_ = s.executionRepo.UpdateRunStatus(ctx, id, "canceled", summaryJSON(model.ExecutionSummary{Total: int64(len(run.CaseIDs)), Skipped: int64(len(run.CaseIDs))}))
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
	s.wake()
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
func (s *ExecutionService) dispatchCaseTask(ctx context.Context, runID int64, caseID int64, executor model.ExecutorView, runType string, headless *bool) (model.ExecutionTask, error) {
	caseDetail, err := s.testCaseRepo.Get(ctx, caseID)
	if err != nil {
		return model.ExecutionTask{}, errors.New("读取用例失败")
	}
	if caseDetail.Status == "disabled" || caseDetail.Status == "deleted" {
		return model.ExecutionTask{}, errors.New("停用或已删除的用例不能执行")
	}

	payload, err := s.buildTaskPayload(caseDetail, runType)
	if err != nil {
		return model.ExecutionTask{}, err
	}
	if runType == "ui" {
		payload["headless"] = true
		if headless != nil {
			payload["headless"] = *headless
		}
	}
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
		return task, errors.New("下发任务到执行器失败")
	}
	return task, nil
}

// buildTaskPayload 根据用例详情和执行类型生成执行器 payload。
func (s *ExecutionService) buildTaskPayload(caseDetail model.TestCaseDetail, runType string) (map[string]any, error) {
	if runType == "api" {
		return map[string]any{
			"caseId":   caseDetail.ID,
			"caseName": caseDetail.Name,
			"url":      caseDetail.Preconditions,
		}, nil
	}
	// 默认 UI 类型：优先解析步骤画布，兼容旧版扁平 action 字段。
	actions := []map[string]any{}
	previousTerminals := []string{}
	for stepIndex, step := range caseDetail.Steps {
		flowActions, roots, terminals, parsed, err := buildCanvasActions(step.Description, fmt.Sprintf("step-%d", stepIndex+1))
		if err != nil {
			return nil, fmt.Errorf("步骤“%s”画布无效: %w", step.StepName, err)
		}
		if parsed {
			if len(previousTerminals) > 0 {
				for _, terminalID := range previousTerminals {
					for _, action := range actions {
						if action["nodeId"] == terminalID {
							action["next"] = roots[0]
						}
					}
				}
			}
			actions = append(actions, flowActions...)
			previousTerminals = terminals
			continue
		}
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
	if len(actions) == 0 {
		return nil, errors.New("测试用例没有可执行步骤")
	}
	payload := map[string]any{
		"caseId":   caseDetail.ID,
		"caseName": caseDetail.Name,
		"url":      caseDetail.Preconditions,
		"actions":  actions,
	}
	if caseDetail.DataEnabled {
		datasets := []map[string]any{}
		for _, dataset := range caseDetail.Datasets {
			if !dataset.Enabled {
				continue
			}
			variables := map[string]any{}
			if err := json.Unmarshal(dataset.Variables, &variables); err != nil {
				return nil, fmt.Errorf("数据集“%s”变量格式无效", dataset.Name)
			}
			datasets = append(datasets, map[string]any{"name": dataset.Name, "variables": variables})
		}
		if len(datasets) == 0 {
			return nil, errors.New("测试用例已启用参数化，但没有启用的数据集")
		}
		payload["datasets"] = datasets
	}
	return payload, nil
}

type canvasFlow struct {
	Schema      string             `json:"schema"`
	Nodes       []canvasNode       `json:"nodes"`
	Connections []canvasConnection `json:"connections"`
}

type canvasNode struct {
	ID            int64          `json:"id"`
	Tag           string         `json:"tag"`
	OperationName string         `json:"operationName"`
	Values        map[string]any `json:"values"`
}

type canvasConnection struct {
	From   int64  `json:"from"`
	To     int64  `json:"to"`
	Branch string `json:"branch"`
}

func buildCanvasActions(description, prefix string) ([]map[string]any, []string, []string, bool, error) {
	if !strings.Contains(description, "synapse-flow-v1") {
		return nil, nil, nil, false, nil
	}
	var flow canvasFlow
	if err := json.Unmarshal([]byte(description), &flow); err != nil {
		return nil, nil, nil, true, errors.New("无法解析画布数据")
	}
	if flow.Schema != "synapse-flow-v1" || len(flow.Nodes) == 0 {
		return nil, nil, nil, true, errors.New("画布没有节点")
	}
	incoming := map[int64]bool{}
	outgoing := map[int64][]canvasConnection{}
	for _, connection := range flow.Connections {
		incoming[connection.To] = true
		outgoing[connection.From] = append(outgoing[connection.From], connection)
	}
	roots := []string{}
	terminals := []string{}
	actions := make([]map[string]any, 0, len(flow.Nodes))
	for _, node := range flow.Nodes {
		nodeID := fmt.Sprintf("%s-%d", prefix, node.ID)
		if !incoming[node.ID] {
			roots = append(roots, nodeID)
		}
		if len(outgoing[node.ID]) == 0 {
			terminals = append(terminals, nodeID)
		}
		action, err := canvasNodeAction(node)
		if err != nil {
			return nil, nil, nil, true, err
		}
		action["nodeId"] = nodeID
		for _, connection := range outgoing[node.ID] {
			target := fmt.Sprintf("%s-%d", prefix, connection.To)
			switch connection.Branch {
			case "true":
				action["trueNext"] = target
			case "false":
				action["falseNext"] = target
			default:
				action["next"] = target
			}
		}
		if node.Tag == "condition" && (action["trueNext"] == nil || action["falseNext"] == nil) {
			return nil, nil, nil, true, errors.New("条件节点必须连接真、假分支")
		}
		actions = append(actions, action)
	}
	if len(roots) != 1 {
		return nil, nil, nil, true, errors.New("画布必须只有一个起始节点")
	}
	return actions, roots, terminals, true, nil
}

func canvasNodeAction(node canvasNode) (map[string]any, error) {
	value := func(key string) any { return node.Values[key] }
	action := map[string]any{"label": node.OperationName}
	switch node.Tag {
	case "w_wait_for_timeout":
		action["action"], action["value"] = "waitForTimeout", value("_time")
	case "w_goto":
		action["action"], action["url"] = "goto", value("url")
	case "w_screenshot":
		action["action"], action["name"] = "screenshot", value("path")
	case "w_click", "w_force_click":
		action["action"], action["selector"] = "click", value("locating")
	case "w_dblclick":
		action["action"], action["selector"] = "dblclick", value("locating")
	case "w_input", "w_clear_input":
		action["action"], action["selector"], action["value"] = "fill", value("locating"), value("input_value")
	case "w_hover":
		action["action"], action["selector"] = "hover", value("locating")
	case "assert_text":
		action["action"], action["selector"], action["text"] = "assertText", value("locating"), value("expected")
	case "assert_text_equals":
		action["action"], action["selector"], action["text"] = "assertTextEquals", value("locating"), value("expected")
	case "assert_title":
		action["action"], action["text"] = "assertTitle", value("expected")
	case "assert_title_equals":
		action["action"], action["text"] = "assertTitleEquals", value("expected")
	case "assert_url":
		action["action"], action["text"] = "assertURL", value("expected")
	case "assert_element_exists":
		action["action"], action["selector"] = "assertElementExists", value("locating")
	case "assert_visible", "assert_hidden", "assert_enabled", "assert_disabled", "assert_checked", "assert_unchecked":
		actions := map[string]string{"assert_visible": "assertVisible", "assert_hidden": "assertHidden", "assert_enabled": "assertEnabled", "assert_disabled": "assertDisabled", "assert_checked": "assertChecked", "assert_unchecked": "assertUnchecked"}
		action["action"], action["selector"] = actions[node.Tag], value("locating")
	case "assert_value_equals":
		action["action"], action["selector"], action["text"] = "assertValueEquals", value("locating"), value("expected")
	case "assert_attribute_equals", "assert_attribute_contains":
		action["action"], action["selector"], action["attribute"], action["text"] = "assertAttribute", value("locating"), value("attribute_name"), value("expected")
		if node.Tag == "assert_attribute_contains" {
			action["operator"] = "contains"
		} else {
			action["operator"] = "equals"
		}
	case "assert_count":
		action["action"], action["selector"], action["count"] = "assertCount", value("locating"), value("expected_count")
	case "assert_variable":
		action["action"], action["left"], action["operator"], action["right"] = "assertVariable", value("left_value"), value("operator"), value("right_value")
	case "assert_variable_exists":
		action["action"], action["name"] = "assertVariableExists", value("variable_name")
	case "assert_regex":
		action["action"], action["value"], action["pattern"] = "assertRegex", value("left_value"), value("pattern")
	case "set_variable":
		action["action"], action["name"], action["value"] = "setVariable", value("variable_name"), value("variable_value")
	case "condition":
		action["action"], action["left"], action["operator"], action["right"] = "condition", value("left_value"), value("operator"), value("right_value")
	case "sql_query":
		action["action"], action["connectionString"], action["query"], action["resultVariable"] = "sqlQuery", value("connection_string"), value("sql"), value("result_variable")
	case "python_code":
		action["action"], action["code"] = "pythonCode", value("python_code")
	default:
		return nil, fmt.Errorf("不支持的节点操作: %s", node.Tag)
	}
	return action, nil
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
	run, err := s.executionRepo.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	tasks, err := s.executionRepo.ListTasksByRun(ctx, runID)
	if err != nil {
		return err
	}
	total := int64(len(run.CaseIDs))
	var passed, failed, skipped, active int64
	allFinished := len(tasks) == len(run.CaseIDs)
	for _, task := range tasks {
		switch task.Status {
		case "success":
			passed++
		case "failed":
			failed++
		case "canceled":
			skipped++
		default:
			active++
			allFinished = false
		}
	}
	waiting := total - int64(len(tasks))
	if !allFinished {
		return s.executionRepo.UpdateRunStatus(ctx, runID, "running", summaryJSON(model.ExecutionSummary{Total: total, Passed: passed, Failed: failed, Skipped: skipped, Waiting: waiting, Active: active}))
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
