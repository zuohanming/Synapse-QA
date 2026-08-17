package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"synapseqa/backend/internal/model"
)

type fakePerformanceRepo struct {
	productExists bool
	createPlanID  int64
	createRunID   int64
	listErr       error
	getErr        error
	createErr     error
	updateErr     error
	deleteErr     error
	updateRows    int64
	deleteRows    int64
	runErr        error
	getRunErr     error
	listRunsErr   error
}

func (f *fakePerformanceRepo) ListPlans(ctx context.Context, filter model.PerfTestPlanFilter, page, pageSize int) ([]model.PerfTestPlan, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return []model.PerfTestPlan{{ID: 1, Name: "登录接口压测"}}, 1, nil
}

func (f *fakePerformanceRepo) GetPlan(ctx context.Context, id int64) (model.PerfTestPlan, error) {
	if f.getErr != nil {
		return model.PerfTestPlan{}, f.getErr
	}
	if id == 0 {
		return model.PerfTestPlan{}, errors.New("missing")
	}
	return model.PerfTestPlan{ID: id, Name: "登录接口压测"}, nil
}

func (f *fakePerformanceRepo) CreatePlan(ctx context.Context, req model.PerfTestPlanRequest, actor string) (int64, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	if f.createPlanID == 0 {
		f.createPlanID = 1
	}
	return f.createPlanID, nil
}

func (f *fakePerformanceRepo) UpdatePlan(ctx context.Context, id int64, req model.PerfTestPlanRequest) (int64, error) {
	if f.updateErr != nil {
		return 0, f.updateErr
	}
	return f.updateRows, nil
}

func (f *fakePerformanceRepo) DeletePlan(ctx context.Context, id int64) (int64, error) {
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	return f.deleteRows, nil
}

func (f *fakePerformanceRepo) ExistsProduct(ctx context.Context, id int64) bool {
	return f.productExists
}

func (f *fakePerformanceRepo) ListRuns(ctx context.Context, filter model.PerfTestRunFilter, page, pageSize int) ([]model.PerfTestRun, int64, error) {
	if f.listRunsErr != nil {
		return nil, 0, f.listRunsErr
	}
	return []model.PerfTestRun{{ID: 1, PlanID: 1, Status: "pending"}}, 1, nil
}

func (f *fakePerformanceRepo) GetRun(ctx context.Context, id int64) (model.PerfTestRun, error) {
	if f.getRunErr != nil {
		return model.PerfTestRun{}, f.getRunErr
	}
	if id == 0 {
		return model.PerfTestRun{}, errors.New("missing")
	}
	return model.PerfTestRun{ID: id, PlanID: 1, Status: "pending"}, nil
}

func (f *fakePerformanceRepo) CreateRun(ctx context.Context, planID int64, actor string) (int64, error) {
	if f.runErr != nil {
		return 0, f.runErr
	}
	if f.createRunID == 0 {
		f.createRunID = 100
	}
	return f.createRunID, nil
}

func validPerfPlanRequest() model.PerfTestPlanRequest {
	return model.PerfTestPlanRequest{
		ProductID:  1,
		Name:       "登录接口压测",
		TargetURL:  "https://example.com/login",
		Method:     "GET",
		LoadMode:   "constant",
		VUs:        10,
		Duration:   "60s",
		Priority:   "P1",
		Status:     "active",
		Owner:      "admin",
		Headers:    json.RawMessage(`{}`),
		Thresholds: json.RawMessage(`{}`),
		Stages:     json.RawMessage(`[]`),
	}
}

func TestPerformanceServiceCreateSuccess(t *testing.T) {
	repo := &fakePerformanceRepo{productExists: true, createPlanID: 9}
	logger := &fakeOperationLogger{}
	svc := NewPerformanceServiceWithLogger(repo, logger)
	id, err := svc.CreatePlan(context.Background(), "admin", validPerfPlanRequest())
	if err != nil {
		t.Fatalf("CreatePlan returned error: %v", err)
	}
	if id != 9 {
		t.Fatalf("expected id 9, got %d", id)
	}
	if logger.calls != 1 {
		t.Fatalf("expected one log call, got %d", logger.calls)
	}
}

func TestPerformanceServiceRejectsNameMissing(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil)
	req := validPerfPlanRequest()
	req.Name = ""
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected name required error")
	}
	req = validPerfPlanRequest()
	req.Name = strings.Repeat("测", 121)
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected name length error")
	}
}

func TestPerformanceServiceRejectsInvalidEnums(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil)
	req := validPerfPlanRequest()
	req.Method = "OPTIONS"
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected invalid method error")
	}
	req = validPerfPlanRequest()
	req.LoadMode = "spike"
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected invalid load mode error")
	}
	req = validPerfPlanRequest()
	req.Priority = "P9"
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected invalid priority error")
	}
	req = validPerfPlanRequest()
	req.Status = "archived"
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestPerformanceServiceRejectsConstantMissingVUsOrDuration(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil)
	req := validPerfPlanRequest()
	req.VUs = 0
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected vus required error")
	}
	req = validPerfPlanRequest()
	req.Duration = ""
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected duration required error")
	}
}

func TestPerformanceServiceRejectsRampingMissingStages(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil)
	req := validPerfPlanRequest()
	req.LoadMode = "ramping"
	req.Stages = json.RawMessage(`[]`)
	if _, err := svc.CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected ramping stages error")
	}
}

func TestPerformanceServiceRejectsMissingProduct(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: false}, nil)
	if _, err := svc.CreatePlan(context.Background(), "admin", validPerfPlanRequest()); err == nil {
		t.Fatal("expected missing product error")
	}
}

func TestPerformanceServiceUpdateNotFound(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true, updateRows: 0}, nil)
	if err := svc.UpdatePlan(context.Background(), "admin", 1, validPerfPlanRequest()); err == nil {
		t.Fatal("expected update not found error")
	}
}

func TestPerformanceServiceRepositoryErrors(t *testing.T) {
	req := validPerfPlanRequest()
	if _, err := NewPerformanceService(&fakePerformanceRepo{productExists: true, createErr: errors.New("duplicate")}, nil).CreatePlan(context.Background(), "admin", req); err == nil {
		t.Fatal("expected create repository error")
	}
	if err := NewPerformanceService(&fakePerformanceRepo{productExists: true, updateErr: errors.New("duplicate")}, nil).UpdatePlan(context.Background(), "admin", 1, req); err == nil {
		t.Fatal("expected update repository error")
	}
	if err := NewPerformanceService(&fakePerformanceRepo{deleteErr: errors.New("db")}, nil).DeletePlan(context.Background(), "admin", 1); err == nil {
		t.Fatal("expected delete repository error")
	}
	if _, err := NewPerformanceService(&fakePerformanceRepo{productExists: true, runErr: errors.New("db")}, nil).RunPlan(context.Background(), "admin", 1); err == nil {
		t.Fatal("expected run repository error")
	}
}

func TestPerformanceServiceLogCalls(t *testing.T) {
	repo := &fakePerformanceRepo{productExists: true, createPlanID: 9, updateRows: 1, deleteRows: 1, createRunID: 100}
	logger := &fakeOperationLogger{}
	svc := NewPerformanceServiceWithLogger(repo, logger)
	if _, err := svc.CreatePlan(context.Background(), "admin", validPerfPlanRequest()); err != nil {
		t.Fatalf("CreatePlan returned error: %v", err)
	}
	if err := svc.UpdatePlan(context.Background(), "admin", 1, validPerfPlanRequest()); err != nil {
		t.Fatalf("UpdatePlan returned error: %v", err)
	}
	if err := svc.DeletePlan(context.Background(), "admin", 1); err != nil {
		t.Fatalf("DeletePlan returned error: %v", err)
	}
	if _, err := svc.RunPlan(context.Background(), "admin", 1); err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}
	if logger.calls != 4 {
		t.Fatalf("expected 4 log calls, got %d", logger.calls)
	}
}

func TestPerformanceServiceListAndGet(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{productExists: true}, nil)
	result, err := svc.ListPlans(context.Background(), model.PerfTestPlanFilter{Name: " 压测 "}, 0, 500)
	if err != nil {
		t.Fatalf("ListPlans returned error: %v", err)
	}
	if result.Total != 1 || result.Page != 1 || result.PageSize != 100 {
		t.Fatalf("unexpected page result: %+v", result)
	}
	plan, err := svc.GetPlan(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPlan returned error: %v", err)
	}
	if plan.ID != 1 {
		t.Fatalf("expected plan id 1, got %d", plan.ID)
	}
	if _, err := svc.GetPlan(context.Background(), 0); err == nil {
		t.Fatal("expected invalid plan id error")
	}
	runResult, err := svc.ListRuns(context.Background(), model.PerfTestRunFilter{PlanID: "1"}, 1, 20)
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if runResult.Total != 1 {
		t.Fatalf("expected one run, got %d", runResult.Total)
	}
	run, err := svc.GetRun(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if run.ID != 1 {
		t.Fatalf("expected run id 1, got %d", run.ID)
	}
	if _, err := svc.GetRun(context.Background(), 0); err == nil {
		t.Fatal("expected invalid run id error")
	}
}

func TestPerformanceServiceRunPlanMissingPlan(t *testing.T) {
	svc := NewPerformanceService(&fakePerformanceRepo{getErr: errors.New("missing")}, nil)
	if _, err := svc.RunPlan(context.Background(), "admin", 1); err == nil {
		t.Fatal("expected run missing plan error")
	}
}
