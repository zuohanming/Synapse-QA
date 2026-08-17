package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	runErr        bool
}

func (f *controllerFakePerfRepo) ListPlans(ctx context.Context, filter model.PerfTestPlanFilter, page, pageSize int) ([]model.PerfTestPlan, int64, error) {
	if f.listErr {
		return nil, 0, errControllerFake
	}
	return []model.PerfTestPlan{{ID: 1, ProductID: 1, Name: "登录接口压测", LoadMode: "constant", Priority: "P1", Status: "active"}}, 1, nil
}

func (f *controllerFakePerfRepo) GetPlan(ctx context.Context, id int64) (model.PerfTestPlan, error) {
	if f.getErr {
		return model.PerfTestPlan{}, errControllerFake
	}
	return model.PerfTestPlan{ID: id, ProductID: 1, Name: "登录接口压测"}, nil
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

func (f *controllerFakePerfRepo) CreateRun(ctx context.Context, planID int64, actor string) (int64, error) {
	if f.runErr {
		return 0, errControllerFake
	}
	return 100, nil
}

func perfRouter(authenticated bool) *gin.Engine {
	return perfRouterWithRepo(authenticated, &controllerFakePerfRepo{productExists: true})
}

func perfRouterWithRepo(authenticated bool, repo *controllerFakePerfRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctl := NewPerformanceController(service.NewPerformanceService(repo, nil))
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
	r.GET("/perf/runs", ctl.ListRuns)
	r.GET("/perf/runs/:id", ctl.GetRun)
	return r
}

func validPerfPlanBody() []byte {
	body, _ := json.Marshal(model.PerfTestPlanRequest{
		ProductID: 1,
		Name:      "登录接口压测",
		TargetURL: "https://example.com/login",
		Method:    "GET",
		LoadMode:  "constant",
		VUs:       10,
		Duration:  "60s",
		Priority:  "P1",
		Status:    "active",
		Headers:   json.RawMessage(`{}`),
		Stages:    json.RawMessage(`[]`),
	})
	return body
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
	req := httptest.NewRequest(http.MethodPost, "/perf/plans/1/run", nil)
	rec := httptest.NewRecorder()
	perfRouter(true).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
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
	cases := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodPost, "/perf/plans", body},
		{http.MethodPatch, "/perf/plans/1", body},
		{http.MethodDelete, "/perf/plans/1", nil},
		{http.MethodPost, "/perf/plans/1/run", nil},
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
		{http.MethodPost, "/perf/plans/bad/run", nil},
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
		{http.MethodPost, "/perf/plans/1/run", nil, perfRouterWithRepo(true, &controllerFakePerfRepo{getErr: true}), http.StatusBadRequest},
		{http.MethodPost, "/perf/plans/1/run", nil, perfRouterWithRepo(true, &controllerFakePerfRepo{runErr: true}), http.StatusBadRequest},
		{http.MethodGet, "/perf/runs", nil, perfRouterWithRepo(true, &controllerFakePerfRepo{productExists: true, listRunsErr: true}), http.StatusBadRequest},
		{http.MethodGet, "/perf/runs/1", nil, perfRouterWithRepo(true, &controllerFakePerfRepo{getRunErr: true}), http.StatusNotFound},
		{http.MethodPost, "/perf/plans", body, perfRouterWithRepo(true, &controllerFakePerfRepo{productExists: true, listErr: true}), http.StatusCreated},
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
