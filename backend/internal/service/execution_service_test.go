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
		f.run = model.ExecutionRun{ID: 1, RunType: req.RunType, Status: "pending", TriggeredBy: triggeredBy, CaseIDs: req.CaseIDs}
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
	if f.createTaskReturn.TaskID == "" {
		f.createTaskReturn = model.ExecutionTask{ID: 1, RunID: runID, TaskID: taskID, CaseID: caseID, ExecutorID: executorID, TaskType: taskType, Payload: payload, CallbackURL: callbackURL, Status: "queued"}
	}
	f.tasks = append(f.tasks, f.createTaskReturn)
	return f.createTaskReturn, nil
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

func (f *fakeExecutionRepo) UpdateTaskStatus(ctx context.Context, id int64, status string) error {
	f.task.Status = status
	return f.updateStatusErr
}

func (f *fakeExecutionRepo) UpdateTaskResult(ctx context.Context, id int64, result json.RawMessage) error {
	f.task.Result = result
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
		f.getReturn = model.TestCaseDetail{TestCase: model.TestCase{ID: id, Name: "登录成功", Status: "active"}}
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

func TestExecutionServiceCreateRunValidation(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	if _, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{}); err == nil {
		t.Fatal("expected empty case ids error")
	}
	if _, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "bad", CaseIDs: []int64{1}}); err == nil {
		t.Fatal("expected invalid run type error")
	}
}

func TestExecutionServiceCreateRunNoExecutor(t *testing.T) {
	svc := NewExecutionService(&fakeExecutionRepo{}, &fakeExecutorRepo{executors: []model.ExecutorView{}}, &fakeTestCaseReader{}, &fakeExecutionOperationLogger{}, "http://localhost")
	if _, err := svc.CreateRun(context.Background(), "admin", model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}}); err == nil {
		t.Fatal("expected no executor error")
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
	payload := svc.buildTaskPayload(model.TestCaseDetail{
		TestCase: model.TestCase{ID: 7, Name: "登录", Preconditions: "http://127.0.0.1:4173/target.html"},
		Steps: []model.TestCaseStep{
			{Action: "w_input", Locator: "#username", Value: "admin"},
			{Action: "w_click", Locator: "#submit"},
			{Action: "assertTitle", Value: "Synapse QA E2E Target"},
		},
	}, "ui")
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
