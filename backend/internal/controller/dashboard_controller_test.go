package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/service"
)

type dashboardControllerFakeRepo struct {
	projects []model.DashboardProject
}

func (f *dashboardControllerFakeRepo) ListProjects(context.Context, int64, bool, *int64) ([]model.DashboardProject, error) {
	return f.projects, nil
}

func (f *dashboardControllerFakeRepo) ListUIExecutions(context.Context, int64, bool, *int64, time.Time, time.Time) (model.DashboardSourceData, error) {
	return model.DashboardSourceData{
		Counts: model.DashboardExecutionCounts{Total: 1, Success: 1},
		Recent: []model.DashboardExecutionRecord{{Type: "ui", ID: "1", Status: "completed", CreatedAt: time.Now().UTC()}},
	}, nil
}

func (f *dashboardControllerFakeRepo) ListAPIExecutions(context.Context, int64, bool, *int64, time.Time, time.Time) (model.DashboardSourceData, error) {
	return model.DashboardSourceData{}, nil
}

func (f *dashboardControllerFakeRepo) ListPerfExecutions(context.Context, int64, bool, *int64, time.Time, time.Time) (model.DashboardSourceData, error) {
	return model.DashboardSourceData{}, nil
}

func dashboardControllerRouter(repo service.DashboardRepository, claims model.Claims) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("claims", claims)
		c.Next()
	})
	router.GET("/api/dashboard/overview", NewDashboardController(service.NewDashboardService(repo)).Overview)
	return router
}

func TestDashboardControllerOverviewContract(t *testing.T) {
	repo := &dashboardControllerFakeRepo{projects: []model.DashboardProject{{ID: 1, Name: "项目"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/overview?range=24h", nil)
	rec := httptest.NewRecorder()
	dashboardControllerRouter(repo, model.Claims{UserID: 1, RoleCode: "admin"}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data  model.DashboardOverview `json:"data"`
		Error string                  `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if response.Error != "" || response.Data.Filter.Range != "24h" || response.Data.Filter.ProjectID != nil {
		t.Fatalf("unexpected overview contract: %+v", response)
	}
	if response.Data.GeneratedAt.IsZero() || response.Data.Executions.Counts.Total != 1 {
		t.Fatalf("overview data is incomplete: %+v", response.Data)
	}
	if response.Data.Executions.Recent[0].Status != "success" {
		t.Fatalf("status normalization missing from controller response: %+v", response.Data.Executions.Recent)
	}
}

func TestDashboardControllerRejectsInvalidRange(t *testing.T) {
	repo := &dashboardControllerFakeRepo{projects: []model.DashboardProject{{ID: 1, Name: "项目"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/overview?range=90d", nil)
	rec := httptest.NewRecorder()
	dashboardControllerRouter(repo, model.Claims{UserID: 1, RoleCode: "admin"}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid range, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDashboardControllerDefaultsRangeTo7d(t *testing.T) {
	repo := &dashboardControllerFakeRepo{projects: []model.DashboardProject{{ID: 1, Name: "项目"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/overview", nil)
	rec := httptest.NewRecorder()
	dashboardControllerRouter(repo, model.Claims{UserID: 1, RoleCode: "admin"}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for omitted range, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data model.DashboardOverview `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if response.Data.Filter.Range != "7d" {
		t.Fatalf("omitted range should default to 7d, got %q", response.Data.Filter.Range)
	}
}

func TestDashboardControllerReturns404ForUnauthorizedProject(t *testing.T) {
	repo := &dashboardControllerFakeRepo{projects: []model.DashboardProject{{ID: 1, Name: "项目"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/overview?projectId=2&range=24h", nil)
	rec := httptest.NewRecorder()
	dashboardControllerRouter(repo, model.Claims{UserID: 7, RoleCode: "viewer"}).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unauthorized project, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDashboardControllerRejectsInvalidProjectID(t *testing.T) {
	repo := &dashboardControllerFakeRepo{projects: []model.DashboardProject{{ID: 1, Name: "项目"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/overview?projectId=nope&range=24h", nil)
	rec := httptest.NewRecorder()
	dashboardControllerRouter(repo, model.Claims{UserID: 1, RoleCode: "admin"}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid project ID, got %d: %s", rec.Code, rec.Body.String())
	}
}
