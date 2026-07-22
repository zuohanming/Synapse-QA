package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
)

type fakeExecutionService struct {
	listErr     bool
	getErr      bool
	getNotFound bool
	createErr   bool
	cancelErr   bool
	callbackErr bool
	logsErr     bool
	detail      model.ExecutionRunDetail
	listResult  model.PageResult
}

func (f *fakeExecutionService) ListRuns(ctx context.Context, filter model.ExecutionRunFilter, page, pageSize int) (model.PageResult, error) {
	if f.listErr {
		return model.PageResult{}, errControllerFake
	}
	return f.listResult, nil
}

func (f *fakeExecutionService) GetRun(ctx context.Context, id int64) (model.ExecutionRunDetail, error) {
	if f.getNotFound {
		return model.ExecutionRunDetail{}, errors.New("执行批次不存在")
	}
	if f.getErr {
		return model.ExecutionRunDetail{}, errControllerFake
	}
	return f.detail, nil
}

func (f *fakeExecutionService) CreateRun(ctx context.Context, actor string, req model.ExecutionRunRequest) (model.ExecutionRunDetail, error) {
	if f.createErr {
		return model.ExecutionRunDetail{}, errControllerFake
	}
	return f.detail, nil
}

func (f *fakeExecutionService) StartDebug(ctx context.Context, actor string, req model.ExecutionDebugRequest) (model.ExecutionDebugStart, error) {
	return model.ExecutionDebugStart{TaskID: "debug-1", ExecutorID: "executor-1", Status: "queued"}, nil
}

func (f *fakeExecutionService) GetDebugTask(ctx context.Context, executorID, taskID string) (map[string]any, error) {
	return map[string]any{"taskId": taskID, "status": "success"}, nil
}

func (f *fakeExecutionService) CancelRun(ctx context.Context, actor string, id int64) error {
	if f.cancelErr {
		return errControllerFake
	}
	return nil
}

func (f *fakeExecutionService) HandleCallback(ctx context.Context, req model.ExecutionCallbackRequest) error {
	if f.callbackErr {
		return errControllerFake
	}
	return nil
}

func (f *fakeExecutionService) ListLogs(ctx context.Context, taskID int64) ([]model.ExecutionLog, error) {
	if f.logsErr {
		return nil, errControllerFake
	}
	return []model.ExecutionLog{{ID: 1, TaskID: taskID, Level: "info", Message: "log"}}, nil
}

func executionRouter(authenticated bool, svc ExecutionService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctl := NewExecutionController(svc)
	r := gin.New()
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Set("claims", model.Claims{UserID: 1, Username: "admin"})
			c.Next()
		})
	}
	r.GET("/executions", ctl.List)
	r.POST("/executions", ctl.Create)
	r.GET("/executions/:id", ctl.Get)
	r.POST("/executions/:id/cancel", ctl.Cancel)
	r.POST("/executions/tasks/:taskId/callback", ctl.Callback)
	r.GET("/executions/tasks/:taskId/logs", ctl.ListTaskLogs)
	return r
}

func defaultExecutionService() *fakeExecutionService {
	return &fakeExecutionService{
		listResult: model.PageResult{Items: []model.ExecutionRun{{ID: 1}}, Total: 1, Page: 1, PageSize: 20},
		detail: model.ExecutionRunDetail{
			ExecutionRun: model.ExecutionRun{ID: 1, RunType: "ui", Status: "running"},
			Tasks:        []model.ExecutionTask{{ID: 1, TaskID: "task-1", Status: "success"}},
		},
	}
}

func TestExecutionControllerList(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/executions?page=1&pageSize=20", nil)
	rec := httptest.NewRecorder()
	executionRouter(true, defaultExecutionService()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestExecutionControllerCreateRequiresAuth(t *testing.T) {
	body, _ := json.Marshal(model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}})
	req := httptest.NewRequest(http.MethodPost, "/executions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	executionRouter(false, defaultExecutionService()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestExecutionControllerCRUD(t *testing.T) {
	body, _ := json.Marshal(model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}})
	cases := []struct {
		method string
		path   string
		body   []byte
		code   int
	}{
		{http.MethodGet, "/executions/1", nil, http.StatusOK},
		{http.MethodPost, "/executions", body, http.StatusCreated},
		{http.MethodPost, "/executions/1/cancel", nil, http.StatusOK},
		{http.MethodPost, "/executions/tasks/1/callback", []byte(`{"taskId":"task-1","status":"success"}`), http.StatusOK},
		{http.MethodGet, "/executions/tasks/1/logs", nil, http.StatusOK},
	}
	r := executionRouter(true, defaultExecutionService())
	for _, item := range cases {
		req := httptest.NewRequest(item.method, item.path, bytes.NewReader(item.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != item.code {
			t.Fatalf("%s %s expected %d, got %d: %s", item.method, item.path, item.code, rec.Code, rec.Body.String())
		}
	}
}

func TestExecutionControllerInvalidInputs(t *testing.T) {
	r := executionRouter(true, defaultExecutionService())
	cases := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodGet, "/executions/bad", nil},
		{http.MethodPost, "/executions/bad/cancel", nil},
		{http.MethodGet, "/executions/tasks/bad/logs", nil},
		{http.MethodPost, "/executions", []byte(`{`)},
		{http.MethodPost, "/executions/tasks/task-1/callback", []byte(`{`)},
	}
	for _, item := range cases {
		req := httptest.NewRequest(item.method, item.path, bytes.NewReader(item.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s expected 400, got %d", item.method, item.path, rec.Code)
		}
	}
}

func TestExecutionControllerNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/executions/99", nil)
	rec := httptest.NewRecorder()
	executionRouter(true, &fakeExecutionService{getNotFound: true}).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestExecutionControllerServiceFailures(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   []byte
		svc    *fakeExecutionService
		code   int
	}{
		{http.MethodGet, "/executions", nil, &fakeExecutionService{listErr: true}, http.StatusBadRequest},
		{http.MethodGet, "/executions/1", nil, &fakeExecutionService{getErr: true}, http.StatusBadRequest},
		{http.MethodPost, "/executions", []byte(`{"runType":"ui","caseIds":[1]}`), &fakeExecutionService{createErr: true}, http.StatusBadRequest},
		{http.MethodPost, "/executions/1/cancel", nil, &fakeExecutionService{cancelErr: true}, http.StatusBadRequest},
		{http.MethodPost, "/executions/tasks/1/callback", []byte(`{"taskId":"task-1","status":"success"}`), &fakeExecutionService{callbackErr: true}, http.StatusBadRequest},
		{http.MethodGet, "/executions/tasks/1/logs", nil, &fakeExecutionService{logsErr: true}, http.StatusBadRequest},
	}
	for _, item := range cases {
		req := httptest.NewRequest(item.method, item.path, bytes.NewReader(item.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		executionRouter(true, item.svc).ServeHTTP(rec, req)
		if rec.Code != item.code {
			t.Fatalf("%s %s expected %d, got %d", item.method, item.path, item.code, rec.Code)
		}
	}
}

func TestExecutionControllerAuthFailures(t *testing.T) {
	body, _ := json.Marshal(model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}})
	cases := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodGet, "/executions", nil},
		{http.MethodPost, "/executions", body},
		{http.MethodGet, "/executions/1", nil},
		{http.MethodPost, "/executions/1/cancel", nil},
		{http.MethodGet, "/executions/tasks/1/logs", nil},
	}
	r := executionRouter(false, defaultExecutionService())
	for _, item := range cases {
		req := httptest.NewRequest(item.method, item.path, bytes.NewReader(item.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s expected 401, got %d", item.method, item.path, rec.Code)
		}
	}
}
