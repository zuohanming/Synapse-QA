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

type controllerFakeTestCaseRepo struct {
	productExists bool
	listErr       bool
	getErr        bool
	updateRows    int64
	deleteRows    int64
	deleteMissing bool
	datasetRows   int64
	datasetErr    bool
}

func (f *controllerFakeTestCaseRepo) List(ctx context.Context, filter model.TestCaseFilter, page, pageSize int) ([]model.TestCase, int64, error) {
	if f.listErr {
		return nil, 0, errControllerFake
	}
	return []model.TestCase{{ID: 1, ProductID: 1, Name: "登录成功", CaseType: "ui", Priority: "P1", Status: "active"}}, 1, nil
}

func (f *controllerFakeTestCaseRepo) Get(ctx context.Context, id int64) (model.TestCaseDetail, error) {
	if f.getErr {
		return model.TestCaseDetail{}, errControllerFake
	}
	return model.TestCaseDetail{TestCase: model.TestCase{ID: id, ProductID: 1, Name: "登录成功"}}, nil
}

func (f *controllerFakeTestCaseRepo) Create(ctx context.Context, req model.TestCaseRequest, actor string) (int64, error) {
	return 1, nil
}

func (f *controllerFakeTestCaseRepo) Update(ctx context.Context, id int64, req model.TestCaseRequest) (int64, error) {
	if f.updateRows != 0 {
		return f.updateRows, nil
	}
	return 1, nil
}

func (f *controllerFakeTestCaseRepo) Delete(ctx context.Context, id int64) (int64, error) {
	if f.deleteMissing {
		return 0, nil
	}
	if f.deleteRows != 0 {
		return f.deleteRows, nil
	}
	return 1, nil
}

func (f *controllerFakeTestCaseRepo) ExistsProduct(ctx context.Context, id int64) bool {
	return f.productExists
}

func (f *controllerFakeTestCaseRepo) CountMissingSteps(ctx context.Context, ids []int64) (int64, error) {
	return 0, nil
}

func (f *controllerFakeTestCaseRepo) ListDatasets(ctx context.Context, caseID int64) ([]model.TestCaseDataset, error) {
	if f.datasetErr {
		return nil, errControllerFake
	}
	return []model.TestCaseDataset{}, nil
}

func (f *controllerFakeTestCaseRepo) CreateDataset(ctx context.Context, caseID int64, req model.TestCaseDatasetRequest) error {
	if f.datasetErr {
		return errControllerFake
	}
	return nil
}

func (f *controllerFakeTestCaseRepo) UpdateDataset(ctx context.Context, caseID, datasetID int64, req model.TestCaseDatasetRequest) (int64, error) {
	if f.datasetErr {
		return 0, errControllerFake
	}
	if f.datasetRows != 0 {
		return f.datasetRows, nil
	}
	return 1, nil
}

func (f *controllerFakeTestCaseRepo) DeleteDataset(ctx context.Context, caseID, datasetID int64) (int64, error) {
	if f.datasetErr {
		return 0, errControllerFake
	}
	if f.datasetRows != 0 {
		return f.datasetRows, nil
	}
	return 1, nil
}

var errControllerFake = bytes.ErrTooLarge

func testCaseRouter(authenticated bool) *gin.Engine {
	return testCaseRouterWithRepo(authenticated, &controllerFakeTestCaseRepo{productExists: true})
}

func testCaseRouterWithRepo(authenticated bool, repo *controllerFakeTestCaseRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctl := NewTestCaseController(service.NewTestCaseService(repo, nil))
	r := gin.New()
	if authenticated {
		r.Use(func(c *gin.Context) {
			c.Set("claims", model.Claims{UserID: 1, Username: "admin"})
			c.Next()
		})
	}
	r.GET("/test-cases", ctl.List)
	r.GET("/test-cases/:id", ctl.Get)
	r.POST("/test-cases", ctl.Create)
	r.PATCH("/test-cases/:id", ctl.Update)
	r.DELETE("/test-cases/:id", ctl.Delete)
	r.GET("/test-cases/:id/datasets", ctl.ListDatasets)
	r.POST("/test-cases/:id/datasets", ctl.CreateDataset)
	r.PATCH("/test-cases/:id/datasets/:datasetId", ctl.UpdateDataset)
	r.DELETE("/test-cases/:id/datasets/:datasetId", ctl.DeleteDataset)
	return r
}

func TestTestCaseControllerList(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test-cases?page=1&pageSize=20", nil)
	rec := httptest.NewRecorder()
	testCaseRouter(true).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTestCaseControllerCreateRequiresAuth(t *testing.T) {
	body, _ := json.Marshal(model.TestCaseRequest{ProductID: 1, Name: "登录成功"})
	req := httptest.NewRequest(http.MethodPost, "/test-cases", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	testCaseRouter(false).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestTestCaseControllerCRUD(t *testing.T) {
	body, _ := json.Marshal(model.TestCaseRequest{ProductID: 1, Name: "登录成功", CaseType: "ui", Priority: "P1", Status: "active"})
	cases := []struct {
		method string
		path   string
		body   []byte
		code   int
	}{
		{http.MethodGet, "/test-cases/1", nil, http.StatusOK},
		{http.MethodPost, "/test-cases", body, http.StatusCreated},
		{http.MethodPatch, "/test-cases/1", body, http.StatusOK},
		{http.MethodDelete, "/test-cases/1", nil, http.StatusOK},
	}
	r := testCaseRouter(true)
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

func TestTestCaseControllerCreateDataset(t *testing.T) {
	body, _ := json.Marshal(model.TestCaseDatasetRequest{Name: "默认数据", Variables: json.RawMessage(`{"user":"admin"}`), Enabled: true})
	req := httptest.NewRequest(http.MethodPost, "/test-cases/1/datasets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	testCaseRouter(true).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTestCaseControllerDatasetCRUD(t *testing.T) {
	body, _ := json.Marshal(model.TestCaseDatasetRequest{Name: "默认数据", Variables: json.RawMessage(`{"user":"admin"}`), Enabled: true})
	cases := []struct {
		method string
		path   string
		body   []byte
		code   int
	}{
		{http.MethodGet, "/test-cases/1/datasets", nil, http.StatusOK},
		{http.MethodPatch, "/test-cases/1/datasets/1", body, http.StatusOK},
		{http.MethodDelete, "/test-cases/1/datasets/1", nil, http.StatusOK},
	}
	r := testCaseRouter(true)
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

func TestTestCaseControllerInvalidInputs(t *testing.T) {
	r := testCaseRouter(true)
	cases := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodGet, "/test-cases/bad", nil},
		{http.MethodPatch, "/test-cases/bad", []byte(`{}`)},
		{http.MethodDelete, "/test-cases/bad", nil},
		{http.MethodGet, "/test-cases/bad/datasets", nil},
		{http.MethodPatch, "/test-cases/1/datasets/bad", []byte(`{}`)},
		{http.MethodDelete, "/test-cases/1/datasets/bad", nil},
		{http.MethodPost, "/test-cases", []byte(`{`)},
		{http.MethodPost, "/test-cases/1/datasets", []byte(`{`)},
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

func TestTestCaseControllerAuthFailures(t *testing.T) {
	body, _ := json.Marshal(model.TestCaseRequest{ProductID: 1, Name: "登录成功"})
	datasetBody, _ := json.Marshal(model.TestCaseDatasetRequest{Name: "默认", Variables: json.RawMessage(`{}`)})
	cases := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodPost, "/test-cases", body},
		{http.MethodPatch, "/test-cases/1", body},
		{http.MethodDelete, "/test-cases/1", nil},
		{http.MethodPost, "/test-cases/1/datasets", datasetBody},
		{http.MethodPatch, "/test-cases/1/datasets/1", datasetBody},
		{http.MethodDelete, "/test-cases/1/datasets/1", nil},
	}
	r := testCaseRouter(false)
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

func TestTestCaseControllerBusinessValidationFailure(t *testing.T) {
	body, _ := json.Marshal(model.TestCaseRequest{ProductID: 99, Name: "登录成功"})
	req := httptest.NewRequest(http.MethodPost, "/test-cases", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	testCaseRouterWithRepo(true, &controllerFakeTestCaseRepo{productExists: false}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTestCaseControllerServiceFailures(t *testing.T) {
	body, _ := json.Marshal(model.TestCaseRequest{ProductID: 1, Name: "登录成功", CaseType: "ui", Priority: "P1", Status: "active"})
	cases := []struct {
		method string
		path   string
		body   []byte
		router *gin.Engine
		code   int
	}{
		{http.MethodGet, "/test-cases", nil, testCaseRouterWithRepo(true, &controllerFakeTestCaseRepo{productExists: true, listErr: true}), http.StatusBadRequest},
		{http.MethodGet, "/test-cases/1", nil, testCaseRouterWithRepo(true, &controllerFakeTestCaseRepo{productExists: true, getErr: true}), http.StatusNotFound},
		{http.MethodPatch, "/test-cases/1", body, testCaseRouterWithRepo(true, &controllerFakeTestCaseRepo{productExists: false}), http.StatusBadRequest},
		{http.MethodDelete, "/test-cases/1", nil, testCaseRouterWithRepo(true, &controllerFakeTestCaseRepo{deleteMissing: true}), http.StatusBadRequest},
		{http.MethodDelete, "/test-cases/1", nil, testCaseRouterWithRepo(true, &controllerFakeTestCaseRepo{deleteRows: -1}), http.StatusOK},
		{http.MethodGet, "/test-cases/1/datasets", nil, testCaseRouterWithRepo(true, &controllerFakeTestCaseRepo{datasetErr: true}), http.StatusBadRequest},
		{http.MethodPost, "/test-cases/1/datasets", []byte(`{"name":"x","variables":{}}`), testCaseRouterWithRepo(true, &controllerFakeTestCaseRepo{datasetErr: true}), http.StatusBadRequest},
		{http.MethodPatch, "/test-cases/1/datasets/1", []byte(`{"name":"x","variables":[]}`), testCaseRouter(true), http.StatusBadRequest},
		{http.MethodDelete, "/test-cases/1/datasets/1", nil, testCaseRouterWithRepo(true, &controllerFakeTestCaseRepo{datasetErr: true}), http.StatusBadRequest},
	}
	for _, item := range cases {
		req := httptest.NewRequest(item.method, item.path, bytes.NewReader(item.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		item.router.ServeHTTP(rec, req)
		if rec.Code != item.code {
			t.Fatalf("%s %s expected %d, got %d", item.method, item.path, item.code, rec.Code)
		}
	}
}
