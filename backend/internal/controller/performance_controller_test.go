package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type controllerFakePerfRepo struct {
	productExists bool
	listErr       bool
	getErr        bool
	updateRows    int64
	deleteRows    int64
	deleteMissing bool
	listRunsErr   bool
	getRunErr     bool

	plan           model.PerfTestPlan
	idempotentRun  model.PerfTestRun
	idempotentErr  error
	runByTaskID    model.PerfTestRun
	runByTaskErr   error
	updateStatusR  int64
	updateResultR  int64
	updateResultTo string
	updateResult   model.PerfRunResult
}

func (f *controllerFakePerfRepo) ListPlans(ctx context.Context, filter model.PerfTestPlanFilter, page, pageSize int) ([]model.PerfTestPlan, int64, error) {
	if f.listErr {
		return nil, 0, errControllerFake
	}
	return []model.PerfTestPlan{{ID: 1, ProductID: 1, Name: "登录接口压测", ScenarioType: "baseline", Priority: "P1", Status: "active"}}, 1, nil
}

func (f *controllerFakePerfRepo) GetPlan(ctx context.Context, id int64) (model.PerfTestPlan, error) {
	if f.getErr {
		return model.PerfTestPlan{}, errControllerFake
	}
	if f.plan.ID == 0 {
		f.plan = model.PerfTestPlan{
			ID: id, Name: "登录接口压测", TargetURL: "https://example.com/login", Method: "GET",
			ScenarioType: "baseline", Environment: "test", LoadConfig: json.RawMessage(`{"vus":3,"duration":"2m"}`),
		}
	}
	return f.plan, nil
}

func (f *controllerFakePerfRepo) CreatePlan(ctx context.Context, req model.PerfTestPlanRequest, actor string) (int64, error) {
	return 1, nil
}

func (f *controllerFakePerfRepo) UpdatePlan(ctx context.Context, id int64, req model.PerfTestPlanRequest) (int64, error) {
	if f.updateRows != 0 {
		return f.updateRows, nil
	}
	return 1, nil
}

func (f *controllerFakePerfRepo) DeletePlan(ctx context.Context, id int64) (int64, error) {
	if f.deleteMissing {
		return 0, nil
	}
	if f.deleteRows != 0 {
		return f.deleteRows, nil
	}
	return 1, nil
}

func (f *controllerFakePerfRepo) ExistsProduct(ctx context.Context, id int64) bool {
	return f.productExists
}

func (f *controllerFakePerfRepo) ListRuns(ctx context.Context, filter model.PerfTestRunFilter, page, pageSize int) ([]model.PerfTestRun, int64, error) {
	if f.listRunsErr {
		return nil, 0, errControllerFake
	}
	return []model.PerfTestRun{{ID: 1, PlanID: 1, PlanName: "登录接口压测", Status: "pending"}}, 1, nil
}

func (f *controllerFakePerfRepo) GetRun(ctx context.Context, id int64) (model.PerfTestRun, error) {
	if f.getRunErr {
		return model.PerfTestRun{}, errControllerFake
	}
	return model.PerfTestRun{ID: id, PlanID: 1, Status: "pending"}, nil
}

func (f *controllerFakePerfRepo) GetRunByTaskID(ctx context.Context, taskID string) (model.PerfTestRun, error) {
	if f.runByTaskErr != nil {
		return model.PerfTestRun{}, f.runByTaskErr
	}
	if f.runByTaskID.ID == 0 {
		f.runByTaskID = model.PerfTestRun{ID: 1, PlanID: 1, Status: "dispatching"}
	}
	return f.runByTaskID, nil
}

func (f *controllerFakePerfRepo) GetRunByIdempotencyKey(ctx context.Context, triggeredBy, key string) (model.PerfTestRun, error) {
	if f.idempotentErr != nil {
		return model.PerfTestRun{}, f.idempotentErr
	}
	if f.idempotentRun.ID == 0 {
		return model.PerfTestRun{}, errControllerFake
	}
	return f.idempotentRun, nil
}

func (f *controllerFakePerfRepo) CreateRun(ctx context.Context, planID int64, scenarioType, environment, configHash, idempotencyKey, triggeredBy string, planSnapshot json.RawMessage, requestedAt, expectedFinishAt time.Time) (int64, error) {
	return 100, nil
}

func (f *controllerFakePerfRepo) MarkNeedsAttention(ctx context.Context, id int64) (int64, error) {
	return 1, nil
}

func (f *controllerFakePerfRepo) UpdateRunStatus(ctx context.Context, id int64, from, to string, extra map[string]any) (int64, error) {
	return f.updateStatusR, nil
}

func (f *controllerFakePerfRepo) UpdateRunResult(ctx context.Context, id int64, from []string, to string, result model.PerfRunResult) (int64, error) {
	f.updateResultTo = to
	f.updateResult = result
	return f.updateResultR, nil
}

func perfRouter(authenticated bool) *gin.Engine {
	return perfRouterWithRepo(authenticated, &controllerFakePerfRepo{productExists: true})
}

func perfRouterWithRepo(authenticated bool, repo *controllerFakePerfRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctl := NewPerformanceController(service.NewPerformanceService(repo, nil, nil, "http://localhost"))
	r := gin.New()
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Set("claims", model.Claims{UserID: 1, Username: "admin"})
			c.Next()
		})
	}
	r.GET("/perf/plans", ctl.ListPlans)
	r.GET("/perf/plans/:id", ctl.GetPlan)
	r.POST("/perf/plans", ctl.CreatePlan)
	r.PATCH("/perf/plans/:id", ctl.UpdatePlan)
	r.DELETE("/perf/plans/:id", ctl.DeletePlan)
	r.POST("/perf/plans/:id/run", ctl.RunPlan)
	r.POST("/perf/runs/:id/cancel", ctl.CancelRun)
	r.GET("/perf/runs", ctl.ListRuns)
	r.GET("/perf/runs/:id", ctl.GetRun)
	r.POST("/perf/tasks/:taskId/callback", ctl.Callback)
	return r
}

func validPerfPlanBody() []byte {
	body, _ := json.Marshal(model.PerfTestPlanRequest{
		ProductID:    1,
		Name:         "登录接口压测",
		TargetURL:    "https://example.com/login",
		Method:       "GET",
		ScenarioType: "baseline",
		LoadConfig:   json.RawMessage(`{"vus":3,"duration":"2m"}`),
		Environment:  "test",
		Priority:     "P1",
		Status:       "active",
		Headers:      json.RawMessage(`{}`),
		Thresholds:   []model.PerfThreshold{{Metric: "http_req_duration", Aggregation: "p(95)", Operator: "<", Value: 500}},
	})
	return body
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TestPerformanceControllerListPlans(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/perf/plans?page=1&pageSize=20", nil)
	rec := httptest.NewRecorder()
	perfRouter(true).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPerformanceControllerCreateRequiresAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/perf/plans", bytes.NewReader(validPerfPlanBody()))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	perfRouter(false).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPerformanceControllerPlanCRUD(t *testing.T) {
	body := validPerfPlanBody()
	cases := []struct {
		method string
		path   string
		body   []byte
		code   int
	}{
		{http.MethodGet, "/perf/plans/1", nil, http.StatusOK},
		{http.MethodPost, "/perf/plans", body, http.StatusCreated},
		{http.MethodPatch, "/perf/plans/1", body, http.StatusOK},
		{http.MethodDelete, "/perf/plans/1", nil, http.StatusOK},
	}
	r := perfRouter(true)
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

func TestPerformanceControllerRunPlan(t *testing.T) {
	body, _ := json.Marshal(model.PerfRunRequest{IdempotencyKey: "key-1"})
	req := httptest.NewRequest(http.MethodPost, "/perf/plans/1/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	perfRouterWithRepo(true, &controllerFakePerfRepo{productExists: true, updateStatusR: 1}).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPerformanceControllerCancelRun(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/perf/runs/1/cancel", nil)
	rec := httptest.NewRecorder()
	perfRouterWithRepo(true, &controllerFakePerfRepo{productExists: true, updateStatusR: 1}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPerformanceControllerRuns(t *testing.T) {
	cases := []struct {
		method string
		path   string
		code   int
	}{
		{http.MethodGet, "/perf/runs?page=1&pageSize=20", http.StatusOK},
		{http.MethodGet, "/perf/runs/1", http.StatusOK},
	}
	r := perfRouter(true)
	for _, item := range cases {
		req := httptest.NewRequest(item.method, item.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != item.code {
			t.Fatalf("%s %s expected %d, got %d: %s", item.method, item.path, item.code, rec.Code, rec.Body.String())
		}
	}
}

func TestPerformanceControllerAuthFailures(t *testing.T) {
	body := validPerfPlanBody()
	runBody, _ := json.Marshal(model.PerfRunRequest{IdempotencyKey: "key-1"})
	cases := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodPost, "/perf/plans", body},
		{http.MethodPatch, "/perf/plans/1", body},
		{http.MethodDelete, "/perf/plans/1", nil},
		{http.MethodPost, "/perf/plans/1/run", runBody},
		{http.MethodPost, "/perf/runs/1/cancel", nil},
	}
	r := perfRouter(false)
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

func TestPerformanceControllerInvalidInputs(t *testing.T) {
	r := perfRouter(true)
	cases := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodGet, "/perf/plans/bad", nil},
		{http.MethodPatch, "/perf/plans/bad", []byte(`{}`)},
		{http.MethodDelete, "/perf/plans/bad", nil},
		{http.MethodPost, "/perf/plans/bad/run", []byte(`{}`)},
		{http.MethodPost, "/perf/runs/bad/cancel", nil},
		{http.MethodGet, "/perf/runs/bad", nil},
		{http.MethodPost, "/perf/plans", []byte(`{`)},
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

func TestPerformanceControllerCallbackUnauthorized(t *testing.T) {
	repo := &controllerFakePerfRepo{productExists: true}
	repo.runByTaskID = model.PerfTestRun{ID: 1, PlanID: 1, Status: "running", CallbackTokenHash: tokenHash("correct")}
	r := perfRouterWithRepo(true, repo)
	req := httptest.NewRequest(http.MethodPost, "/perf/tasks/task-1/callback", bytes.NewReader([]byte(`{"status":"running"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPerformanceControllerCallbackGone(t *testing.T) {
	repo := &controllerFakePerfRepo{productExists: true}
	repo.runByTaskID = model.PerfTestRun{ID: 1, PlanID: 1, Status: "completed", CallbackTokenHash: tokenHash("secret")}
	r := perfRouterWithRepo(true, repo)
	req := httptest.NewRequest(http.MethodPost, "/perf/tasks/task-1/callback", bytes.NewReader([]byte(`{"status":"completed"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPerformanceControllerCallbackSuccess(t *testing.T) {
	repo := &controllerFakePerfRepo{productExists: true, updateStatusR: 1}
	repo.runByTaskID = model.PerfTestRun{ID: 1, PlanID: 1, Status: "dispatching", CallbackTokenHash: tokenHash("secret")}
	r := perfRouterWithRepo(true, repo)
	req := httptest.NewRequest(http.MethodPost, "/perf/tasks/task-1/callback", bytes.NewReader([]byte(`{"status":"running","scriptHash":"abc"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPerformanceControllerCallbackPerfPayload(t *testing.T) {
	repo := &controllerFakePerfRepo{productExists: true, updateResultR: 1}
	repo.runByTaskID = model.PerfTestRun{ID: 1, PlanID: 1, Status: model.PerfRunRunning, CallbackTokenHash: tokenHash("secret")}
	body := []byte(`{"status":"completed","generatorVersion":"perf-1.0.0","k6Version":"v1.0.0","exitCode":0,"durationMs":3456,"totalRequests":3571,"avgDurationMs":3,"p95DurationMs":5,"errorRate":0,"rps":100,"summary":{"thresholds":[{"ok":true}],"metrics":{"total_requests":3571}},"diagnosticOutput":"ok"}`)
	req := httptest.NewRequest(http.MethodPost, "/perf/tasks/task-1/callback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	perfRouterWithRepo(true, repo).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if repo.updateResultTo != model.PerfRunCompleted || repo.updateResult.TotalRequests != 3571 || repo.updateResult.P95DurationMs == nil || *repo.updateResult.P95DurationMs != 5 {
		t.Fatalf("perf callback fields were not bound: status=%s result=%+v", repo.updateResultTo, repo.updateResult)
	}
}

func TestPerformanceControllerBusinessValidationFailure(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/perf/plans", bytes.NewReader(validPerfPlanBody()))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	perfRouterWithRepo(true, &controllerFakePerfRepo{productExists: false}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPerformanceControllerServiceFailures(t *testing.T) {
	body := validPerfPlanBody()
	cases := []struct {
		method string
		path   string
		body   []byte
		router *gin.Engine
		code   int
	}{
		{http.MethodGet, "/perf/plans", nil, perfRouterWithRepo(true, &controllerFakePerfRepo{productExists: true, listErr: true}), http.StatusBadRequest},
		{http.MethodGet, "/perf/plans/1", nil, perfRouterWithRepo(true, &controllerFakePerfRepo{productExists: true, getErr: true}), http.StatusNotFound},
		{http.MethodDelete, "/perf/plans/1", nil, perfRouterWithRepo(true, &controllerFakePerfRepo{deleteMissing: true}), http.StatusBadRequest},
		{http.MethodGet, "/perf/runs", nil, perfRouterWithRepo(true, &controllerFakePerfRepo{productExists: true, listRunsErr: true}), http.StatusBadRequest},
		{http.MethodGet, "/perf/runs/1", nil, perfRouterWithRepo(true, &controllerFakePerfRepo{getRunErr: true}), http.StatusNotFound},
		{http.MethodPost, "/perf/plans", body, perfRouterWithRepo(true, &controllerFakePerfRepo{productExists: true}), http.StatusCreated},
	}
	for _, item := range cases {
		req := httptest.NewRequest(item.method, item.path, bytes.NewReader(item.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		item.router.ServeHTTP(rec, req)
		if rec.Code != item.code {
			t.Fatalf("%s %s expected %d, got %d: %s", item.method, item.path, item.code, rec.Code, rec.Body.String())
		}
	}
}

type controllerFakeExecutorRepo struct {
	executors []model.ExecutorView
}

func (f *controllerFakeExecutorRepo) List(ctx context.Context) ([]model.ExecutorView, error) {
	return f.executors, nil
}

func (f *controllerFakeExecutorRepo) GetByID(ctx context.Context, executorID string) (model.ExecutorView, error) {
	for _, item := range f.executors {
		if item.ExecutorID == executorID {
			return item, nil
		}
	}
	return model.ExecutorView{}, errControllerFake
}

func perfRouterWithExecutor(authenticated bool, repo *controllerFakePerfRepo, execRepo *controllerFakeExecutorRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctl := NewPerformanceController(service.NewPerformanceService(repo, execRepo, nil, "http://localhost"))
	r := gin.New()
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Set("claims", model.Claims{UserID: 1, Username: "admin"})
			c.Next()
		})
	}
	r.POST("/perf/smoke", ctl.StartSmoke)
	r.GET("/perf/smoke/:taskId", ctl.GetSmoke)
	return r
}

func TestPerformanceControllerStartSmokeRequiresAuth(t *testing.T) {
	body, _ := json.Marshal(model.PerfSmokeRequest{ExecutorID: "exec-1", TargetURL: "https://example.com"})
	req := httptest.NewRequest(http.MethodPost, "/perf/smoke", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	perfRouterWithExecutor(false, &controllerFakePerfRepo{}, &controllerFakeExecutorRepo{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPerformanceControllerStartSmoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	execRepo := &controllerFakeExecutorRepo{executors: []model.ExecutorView{{
		ExecutorID: "exec-1", Endpoint: server.URL, Status: "online", SupportedTypes: []string{"perf"},
	}}}
	body, _ := json.Marshal(model.PerfSmokeRequest{ExecutorID: "exec-1", TargetURL: "https://example.com", Method: "GET"})
	req := httptest.NewRequest(http.MethodPost, "/perf/smoke", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	perfRouterWithExecutor(true, &controllerFakePerfRepo{}, execRepo).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}
