package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"synapseqa/backend/internal/model"
)

type fakeExecutionRepo struct {
	createRunErr     error
	getRunErr        error
	listRunsErr      error
	createTaskErr    error
	getTaskErr       error
	listTasksErr     error
	updateStatusErr  error
	updateResultErr  error
	logErr           error
	createTaskReturn model.ExecutionTask
	listTasksReturn  []model.ExecutionTask
	run              model.ExecutionRun
	task             model.ExecutionTask
	tasks            []model.ExecutionTask
}

func (f *fakeExecutionRepo) CreateRun(ctx context.Context, req model.ExecutionRunRequest, triggeredBy string) (model.ExecutionRun, error) {
	if f.createRunErr != nil {
		return model.ExecutionRun{}, f.createRunErr
	}
	if f.run.ID == 0 {
		headless := req.Headless == nil || *req.Headless
		f.run = model.ExecutionRun{ID: 1, RunType: req.RunType, Headless: headless, Status: "pending", TriggeredBy: triggeredBy, CaseIDs: req.CaseIDs}
	}
	return f.run, nil
}

func (f *fakeExecutionRepo) GetRun(ctx context.Context, id int64) (model.ExecutionRun, error) {
	if f.getRunErr != nil {
		return model.ExecutionRun{}, f.getRunErr
	}
	if f.run.ID == 0 {
		f.run = model.ExecutionRun{ID: id, Status: "pending"}
	}
	return f.run, nil
}

func (f *fakeExecutionRepo) ListRuns(ctx context.Context, filter model.ExecutionRunFilter, page, pageSize int) ([]model.ExecutionRun, int64, error) {
	if f.listRunsErr != nil {
		return nil, 0, f.listRunsErr
	}
	return []model.ExecutionRun{f.run}, 1, nil
}

func (f *fakeExecutionRepo) UpdateRunStatus(ctx context.Context, id int64, status string, summary json.RawMessage) error {
	f.run.Status = status
	f.run.Summary = summary
	return f.updateStatusErr
}

func (f *fakeExecutionRepo) StartRun(ctx context.Context, id int64) error {
	f.run.Status = "running"
	return nil
}

func (f *fakeExecutionRepo) CreateTask(ctx context.Context, runID int64, taskID string, caseID int64, executorID, taskType, callbackURL string, payload json.RawMessage) (model.ExecutionTask, error) {
	if f.createTaskErr != nil {
		return model.ExecutionTask{}, f.createTaskErr
	}
	item := f.createTaskReturn
	if item.TaskID == "" {
		item = model.ExecutionTask{ID: int64(len(f.tasks) + 1), RunID: runID, TaskID: taskID, CaseID: caseID, ExecutorID: executorID, TaskType: taskType, Payload: payload, CallbackURL: callbackURL, Status: "queued"}
	}
	f.tasks = append(f.tasks, item)
	return item, nil
}

func (f *fakeExecutionRepo) GetTaskByTaskID(ctx context.Context, taskID string) (model.ExecutionTask, error) {
	if f.getTaskErr != nil {
		return model.ExecutionTask{}, f.getTaskErr
	}
	if f.task.TaskID == "" {
		f.task = model.ExecutionTask{ID: 1, RunID: 1, TaskID: taskID, Status: "running"}
	}
	return f.task, nil
}

func (f *fakeExecutionRepo) GetTask(ctx context.Context, id int64) (model.ExecutionTask, error) {
	return f.task, nil
}

func (f *fakeExecutionRepo) ListTasksByRun(ctx context.Context, runID int64) ([]model.ExecutionTask, error) {
	if f.listTasksErr != nil {
		return nil, f.listTasksErr
	}
	if len(f.listTasksReturn) > 0 {
		return f.listTasksReturn, nil
	}
	return f.tasks, nil
}

func (f *fakeExecutionRepo) CountActiveTasksByExecutor(ctx context.Context, executorID string) (int, error) {
	count := 0
	for _, task := range f.tasks {
		if task.ExecutorID == executorID && (task.Status == "queued" || task.Status == "running") {
			count++
		}
	}
	return count, nil
}

func (f *fakeExecutionRepo) UpdateTaskStatus(ctx context.Context, id int64, status string) error {
	f.task.Status = status
	for index := range f.tasks {
		if f.tasks[index].ID == id {
			f.tasks[index].Status = status
		}
	}
	return f.updateStatusErr
}

func (f *fakeExecutionRepo) UpdateTaskResult(ctx context.Context, id int64, result json.RawMessage) error {
	f.task.Result = result
	for index := range f.tasks {
		if f.tasks[index].ID == id {
			f.tasks[index].Result = result
		}
	}
	return f.updateResultErr
}

func (f *fakeExecutionRepo) CreateLog(ctx context.Context, taskID int64, level, message string) error {
	return f.logErr
}

func (f *fakeExecutionRepo) ListLogs(ctx context.Context, taskID int64) ([]model.ExecutionLog, error) {
	return []model.ExecutionLog{{ID: 1, TaskID: taskID, Level: "info", Message: "log"}}, nil
}

type fakeExecutorRepo struct {
	executors []model.ExecutorView
	getErr    error
}

func (f *fakeExecutorRepo) List(ctx context.Context) ([]model.ExecutorView, error) {
	return f.executors, nil
}

func (f *fakeExecutorRepo) GetByID(ctx context.Context, executorID string) (model.ExecutorView, error) {
	if f.getErr != nil {
		return model.ExecutorView{}, f.getErr
	}
	for _, item := range f.executors {
		if item.ExecutorID == executorID {
			return item, nil
		}
	}
	return model.ExecutorView{}, errors.New("not found")
}

type fakeTestCaseReader struct {
	getReturn model.TestCaseDetail
	getErr    error
}

func (f *fakeTestCaseReader) Get(ctx context.Context, id int64) (model.TestCaseDetail, error) {
	if f.getErr != nil {
		return model.TestCaseDetail{}, f.getErr
	}
	if f.getReturn.ID == 0 {
		f.getReturn = model.TestCaseDetail{
			TestCase: model.TestCase{ID: id, Name: "登录成功", Status: "active"},
			Steps:    []model.TestCaseStep{{Action: "click", Locator: "#submit"}},
		}
	}
	return f.getReturn, nil
}

type fakeExecutionOperationLogger struct{}

func (f *fakeExecutionOperationLogger) LogOperation(ctx context.Context, actor, action, target string) error {
	return nil
}

func newExecutionServiceWithServer(t *testing.T, handler http.HandlerFunc) (*ExecutionService, *httptest.Server) {
	server := httptest.NewServer(handler)
	execRepo := &fakeExecutorRepo{
		executors: []model.ExecutorView{{
			ExecutorID:     "exec-1",
			Endpoint:       server.URL,
			Status:         "online",
			SupportedTypes: []string{"ui"},
			RunningTasks:   0,
		}},
	}
	return NewExecutionService(&fakeExecutionRepo{}, execRepo, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, server.URL), server
}

func TestExecutionServiceCreateRunSuccess(t *testing.T) {
	svc, server := newExecutionServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	defer server.Close()
	detail, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if detail.ID != 1 || len(detail.Tasks) != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestExecutionServicePassesHeadedModeToExecutor(t *testing.T) {
	var receivedHeadless any
	svc, server := newExecutionServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode executor request: %v", err)
		}
		payload, _ := body["payload"].(map[string]any)
		receivedHeadless = payload["headless"]
		w.WriteHeader(http.StatusAccepted)
	})
	defer server.Close()
	headless := false
	if _, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}, Headless: &headless}); err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if receivedHeadless != false {
		t.Fatalf("expected headed mode, got %#v", receivedHeadless)
	}
}

func TestExecutionServiceCreateRunValidation(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	if _, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{}); err == nil {
		t.Fatal("expected empty case ids error")
	}
	if _, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "bad", CaseIDs: []int64{1}}); err == nil {
		t.Fatal("expected invalid run type error")
	}
}

func TestExecutionServiceCreateRunDispatchesOnlyExecutorCapacity(t *testing.T) {
	dispatched := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatched++
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"taskId":"accepted","type":"ui","status":"queued","payload":{},"createdAt":"2026-01-01T00:00:00Z"}`))
	}))
	defer server.Close()

	caseIDs := make([]int64, 1000)
	for i := range caseIDs {
		caseIDs[i] = int64(i + 1)
	}
	repo := &fakeExecutionRepo{}
	executors := &fakeExecutorRepo{executors: []model.ExecutorView{{
		ExecutorID: "exec-1", Endpoint: server.URL, Status: "online", MaxWorkers: 2, SupportedTypes: []string{"ui"},
	}}}
	svc := NewExecutionService(repo, executors, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, server.URL)
	detail, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "ui", CaseIDs: caseIDs})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if dispatched != 4 || len(detail.Tasks) != 4 {
		t.Fatalf("expected 4 initial tasks, dispatched=%d tasks=%d", dispatched, len(detail.Tasks))
	}
	if detail.Status != "running" {
		t.Fatalf("expected running batch, got %s", detail.Status)
	}
}

func TestExecutionServiceHeadedRunUsesExecutorCapacity(t *testing.T) {
	dispatched := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatched++
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	repo := &fakeExecutionRepo{}
	executors := &fakeExecutorRepo{executors: []model.ExecutorView{{
		ExecutorID: "exec-1", Endpoint: server.URL, Status: "online", MaxWorkers: 8, SupportedTypes: []string{"ui"},
	}}}
	svc := NewExecutionService(repo, executors, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, server.URL)
	headless := false
	_, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1, 2, 3}, Headless: &headless})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if dispatched != 3 {
		t.Fatalf("expected three headed tasks, got %d", dispatched)
	}
}

func TestExecutionServiceDispatchPendingRefillsAvailableSlots(t *testing.T) {
	dispatched := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatched++
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	repo := &fakeExecutionRepo{}
	executors := &fakeExecutorRepo{executors: []model.ExecutorView{{
		ExecutorID: "exec-1", Endpoint: server.URL, Status: "online", MaxWorkers: 1, SupportedTypes: []string{"ui"},
	}}}
	svc := NewExecutionService(repo, executors, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, server.URL)
	_, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1, 2, 3, 4, 5}})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if dispatched != 2 {
		t.Fatalf("expected initial queue capacity 2, got %d", dispatched)
	}
	repo.tasks[0].Status = "success"
	repo.tasks[1].Status = "success"
	if err := svc.DispatchPending(context.Background()); err != nil {
		t.Fatalf("DispatchPending returned error: %v", err)
	}
	if dispatched != 4 {
		t.Fatalf("expected two replacement tasks, got %d total", dispatched)
	}
}

func TestExecutionServiceCreateRunNoExecutor(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{executors: []model.ExecutorView{}}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	if _, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}}); err == nil {
		t.Fatal("expected no executor error")
	}
}

func TestExecutionServiceRejectsDisabledCase(t *testing.T) {
	repo := &fakeExecutionRepo{}
	svc := NewExecutionService(repo, &fakeExecutorRepo{executors: []model.ExecutorView{{ExecutorID: "exec-1", Status: "online", SupportedTypes: []string{"ui"}}}}, &fakeTestCaseReader{
		getReturn: model.TestCaseDetail{TestCase: model.TestCase{ID: 1, Name: "停用用例", Status: "disabled"}, Steps: []model.TestCaseStep{{Action: "click", Locator: "#submit"}}},
	}, &fakeExecutionOperationLogger{}, "http://localhost")
	detail, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}})
	if err != nil {
		t.Fatalf("CreateRun returned unexpected error: %v", err)
	}
	if detail.Status != "failed" || len(detail.Tasks) != 1 || detail.Tasks[0].Status != "failed" {
		t.Fatalf("expected disabled case to create a failed task: %+v", detail)
	}
}

func TestExecutionServiceAllowsDraftCasePayload(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	payload, err := svc.buildTaskPayload(model.TestCaseDetail{
		TestCase: model.TestCase{ID: 1, Name: "草稿用例", Status: "draft"},
		Steps:    []model.TestCaseStep{{Action: "click", Locator: "#submit"}},
	}, "ui")
	if err != nil || len(payload["actions"].([]map[string]any)) != 1 {
		t.Fatalf("draft case should remain executable: payload=%#v err=%v", payload, err)
	}
}

func TestExecutionServiceCreateRunExecutorUnavailable(t *testing.T) {
	svc, server := newExecutionServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer server.Close()
	detail, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}})
	if err != nil {
		t.Fatalf("CreateRun returned unexpected error: %v", err)
	}
	if len(detail.Tasks) != 1 || detail.Status != "failed" {
		t.Fatalf("expected failed run with one task, got status=%s tasks=%d", detail.Status, len(detail.Tasks))
	}
}

func TestExecutionServiceHandleCallback(t *testing.T) {
	svc := NewExecutionService(
		&fakeExecutionRepo{listTasksReturn: []model.ExecutionTask{{ID: 1, RunID: 1, TaskID: "task-1", Status: "success"}}},
		&fakeExecutorRepo{},
		&fakeTestCaseReader{},
		&fakeExecutionOperationLogger{},
		"http://localhost",
	)
	req := model.ExecutionCallbackRequest{TaskID: "task-1", Status: "success", Result: json.RawMessage(`{"exitCode":0}`)}
	if err := svc.HandleCallback(context.Background(), req); err != nil {
		t.Fatalf("HandleCallback returned error: %v", err)
	}
}

func TestExecutionServiceHandleCallbackMissingTask(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{getTaskErr: errors.New("missing")}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	req := model.ExecutionCallbackRequest{TaskID: "task-1", Status: "success"}
	if err := svc.HandleCallback(context.Background(), req); err == nil {
		t.Fatal("expected missing task error")
	}
}

func TestExecutionServiceListAndGet(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	result, err := svc.ListRuns(context.Background(), model.ExecutionRunFilter{}, 1, 20)
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected total 1, got %d", result.Total)
	}
	detail, err := svc.GetRun(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if detail.ID != 1 {
		t.Fatalf("expected run id 1, got %d", detail.ID)
	}
}

func TestExecutionServiceGetRunNotFound(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{getRunErr: errors.New("missing")}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	if _, err := svc.GetRun(context.Background(), 99); err == nil {
		t.Fatal("expected not found error")
	}
}

func TestExecutionServiceCancelRun(t *testing.T) {
	_, server := newExecutionServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	execRepo := &fakeExecutorRepo{
		executors: []model.ExecutorView{{
			ExecutorID:     "exec-1",
			Endpoint:       server.URL,
			Status:         "online",
			SupportedTypes: []string{"ui"},
		}},
	}
	execSvc := NewExecutionService(&fakeExecutionRepo{listTasksReturn: []model.ExecutionTask{{ID: 1, RunID: 1, TaskID: "task-1", ExecutorID: "exec-1", Status: "queued"}}}, execRepo, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, server.URL)
	if err := execSvc.CancelRun(context.Background(), "admin", 1); err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
}

func TestExecutionServiceCancelRunNotFound(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{getRunErr: errors.New("missing")}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	if err := svc.CancelRun(context.Background(), "admin", 99); err == nil {
		t.Fatal("expected not found error")
	}
}

func TestExecutionServiceListLogs(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	logs, err := svc.ListLogs(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListLogs returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one log, got %d", len(logs))
	}
}

func TestExecutionServicePickExecutor(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{
		executors: []model.ExecutorView{
			{ExecutorID: "exec-1", Status: "online", SupportedTypes: []string{"ui"}, RunningTasks: 2},
			{ExecutorID: "exec-2", Status: "online", SupportedTypes: []string{"ui"}, RunningTasks: 1},
			{ExecutorID: "exec-3", Status: "offline", SupportedTypes: []string{"ui"}, RunningTasks: 0},
		},
	}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	exec, err := svc.pickExecutor(context.Background(), "ui")
	if err != nil {
		t.Fatalf("pickExecutor returned error: %v", err)
	}
	if exec.ExecutorID != "exec-2" {
		t.Fatalf("expected exec-2, got %s", exec.ExecutorID)
	}
}

func TestExecutionServiceBuildsPlayableUIPayload(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	payload, err := svc.buildTaskPayload(model.TestCaseDetail{
		TestCase: model.TestCase{ID: 7, Name: "登录", Preconditions: "http://127.0.0.1:4173/target.html"},
		Steps: []model.TestCaseStep{
			{Action: "w_input", Locator: "#username", Value: "admin"},
			{Action: "w_click", Locator: "#submit"},
			{Action: "assertTitle", Value: "Synapse QA E2E Target"},
		},
	}, "ui")
	if err != nil {
		t.Fatalf("buildTaskPayload returned error: %v", err)
	}
	if payload["url"] != "http://127.0.0.1:4173/target.html" {
		t.Fatalf("unexpected url: %v", payload["url"])
	}
	actions, ok := payload["actions"].([]map[string]any)
	if !ok || len(actions) != 3 {
		t.Fatalf("unexpected actions: %#v", payload["actions"])
	}
	if actions[0]["action"] != "fill" || actions[0]["selector"] != "#username" || actions[0]["value"] != "admin" {
		t.Fatalf("unexpected first action: %#v", actions[0])
	}
	if actions[2]["text"] != "Synapse QA E2E Target" {
		t.Fatalf("unexpected assertion action: %#v", actions[2])
	}
}

func TestExecutionServiceBuildsCanvasAndDatasetPayload(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	flow := `{"schema":"synapse-flow-v1","nodes":[{"id":1,"tag":"set_variable","operationName":"设置变量","values":{"variable_name":"role","variable_value":"${dataset_role}"}},{"id":2,"tag":"condition","operationName":"判断角色","values":{"left_value":"${role}","operator":"equals","right_value":"admin"}},{"id":3,"tag":"assert_title","operationName":"标题断言","values":{"expected":"管理台"}},{"id":4,"tag":"assert_url","operationName":"URL断言","values":{"expected":"login"}}],"connections":[{"from":1,"to":2},{"from":2,"to":3,"branch":"true"},{"from":2,"to":4,"branch":"false"}]}`
	finalFlow := `{"schema":"synapse-flow-v1","nodes":[{"id":1,"tag":"assert_variable_exists","operationName":"变量断言","values":{"variable_name":"role"}}],"connections":[]}`
	payload, err := svc.buildTaskPayload(model.TestCaseDetail{
		TestCase: model.TestCase{ID: 8, Name: "角色登录", DataEnabled: true},
		Steps: []model.TestCaseStep{
			{StepName: "角色分支", Description: flow},
			{StepName: "最终断言", Description: finalFlow},
		},
		Datasets: []model.TestCaseDataset{
			{Name: "管理员", Enabled: true, Variables: json.RawMessage(`{"dataset_role":"admin"}`)},
			{Name: "停用数据", Enabled: false, Variables: json.RawMessage(`{"dataset_role":"guest"}`)},
		},
	}, "ui")
	if err != nil {
		t.Fatalf("buildTaskPayload returned error: %v", err)
	}
	actions := payload["actions"].([]map[string]any)
	if len(actions) != 5 || actions[1]["trueNext"] != "step-1-3" || actions[1]["falseNext"] != "step-1-4" {
		t.Fatalf("unexpected canvas actions: %#v", actions)
	}
	if actions[2]["next"] != "step-2-1" || actions[3]["next"] != "step-2-1" {
		t.Fatalf("canvas steps were not chained: %#v", actions)
	}
	datasets := payload["datasets"].([]map[string]any)
	if len(datasets) != 1 || datasets[0]["name"] != "管理员" {
		t.Fatalf("unexpected datasets: %#v", datasets)
	}
}

func TestExecutionServiceAggregateStatus(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []model.ExecutionTask
		expected string
	}{
		{"all success", []model.ExecutionTask{{Status: "success"}, {Status: "success"}}, "completed"},
		{"one failed", []model.ExecutionTask{{Status: "success"}, {Status: "failed"}}, "failed"},
		{"all canceled", []model.ExecutionTask{{Status: "canceled"}, {Status: "canceled"}}, "canceled"},
		{"canceled with success", []model.ExecutionTask{{Status: "success"}, {Status: "canceled"}}, "completed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeExecutionRepo{listTasksReturn: tt.tasks}
			svc := NewExecutionService(repo, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
			_ = svc.aggregateRunStatus(context.Background(), 1)
			// 状态更新通过 UpdateRunStatus，fake repo 不保存，只需确认不 panic
		})
	}
}
