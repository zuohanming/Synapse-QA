package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/controller"
	"synapseqa/backend/internal/model"
)

type executionRouteServiceStub struct{}

func (executionRouteServiceStub) ListRunsScoped(context.Context, model.Claims, model.ExecutionRunFilter, int, int) (model.PageResult, error) {
	return model.PageResult{Items: []model.ExecutionRun{}, Total: 0}, nil
}
func (executionRouteServiceStub) StatisticsScoped(context.Context, model.Claims) (model.ExecutionStatistics, error) {
	return model.ExecutionStatistics{Trend: []model.ExecutionTrendPoint{}}, nil
}
func (executionRouteServiceStub) GetRunScoped(context.Context, model.Claims, int64) (model.ExecutionRunDetail, error) {
	return model.ExecutionRunDetail{}, nil
}
func (executionRouteServiceStub) CreateRunScoped(context.Context, model.Claims, model.ExecutionRunRequest) (model.ExecutionRunDetail, error) {
	return model.ExecutionRunDetail{}, nil
}
func (executionRouteServiceStub) StartDebug(context.Context, string, model.ExecutionDebugRequest) (model.ExecutionDebugStart, error) {
	return model.ExecutionDebugStart{}, nil
}
func (executionRouteServiceStub) GetDebugTask(context.Context, string, string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (executionRouteServiceStub) CancelRunScoped(context.Context, model.Claims, int64) error {
	return nil
}
func (executionRouteServiceStub) HandleCallback(context.Context, model.ExecutionCallbackRequest) error {
	return nil
}
func (executionRouteServiceStub) ListLogsScoped(context.Context, model.Claims, int64) ([]model.ExecutionLog, error) {
	return nil, nil
}

func executionRouteTestEngine(permissions []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("", func(c *gin.Context) {
		c.Set("claims", model.Claims{UserID: 7, Username: "member", RoleCode: "viewer", Permissions: permissions})
		c.Next()
	})
	registerExecutionRoutes(group, Dependencies{ExecutionController: controller.NewExecutionController(executionRouteServiceStub{})})
	return engine
}

func TestExecutionRoutesRequireReadPermission(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/executions"},
		{http.MethodGet, "/executions/statistics"},
		{http.MethodPost, "/executions"},
		{http.MethodPost, "/executions/debug"},
		{http.MethodGet, "/executions/debug/task-1"},
		{http.MethodGet, "/executions/1"},
		{http.MethodPost, "/executions/1/cancel"},
		{http.MethodGet, "/executions/tasks/1/logs"},
	}
	engine := executionRouteTestEngine(nil)
	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s returned %d, want %d", route.method, route.path, rec.Code, http.StatusForbidden)
		}
	}
}

func TestExecutionRoutesAllowReadPermission(t *testing.T) {
	engine := executionRouteTestEngine([]string{"menu.execution.read"})
	req := httptest.NewRequest(http.MethodGet, "/executions", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read permission returned %d: %s", rec.Code, rec.Body.String())
	}
}
