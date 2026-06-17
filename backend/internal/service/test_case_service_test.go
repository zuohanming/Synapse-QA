package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/repository"
)

type fakeTestCaseRepo struct {
	productExists bool
	missingSteps  int64
	createID      int64
	listErr       error
	getErr        error
	createErr     error
	updateErr     error
	deleteErr     error
	datasetErr    error
	updateRows    int64
	deleteRows    int64
	datasetRows   int64
}

type fakeOperationLogger struct {
	calls int
}

func (f *fakeOperationLogger) LogOperation(ctx context.Context, actor, action, target string) error {
	f.calls++
	return nil
}

func (f *fakeTestCaseRepo) List(ctx context.Context, filter model.TestCaseFilter, page, pageSize int) ([]model.TestCase, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return []model.TestCase{{ID: 1, Name: "登录成功"}}, 1, nil
}

func (f *fakeTestCaseRepo) Get(ctx context.Context, id int64) (model.TestCaseDetail, error) {
	if f.getErr != nil {
		return model.TestCaseDetail{}, f.getErr
	}
	if id == 0 {
		return model.TestCaseDetail{}, errors.New("missing")
	}
	return model.TestCaseDetail{TestCase: model.TestCase{ID: id, Name: "登录成功"}}, nil
}

func (f *fakeTestCaseRepo) Create(ctx context.Context, req model.TestCaseRequest, actor string) (int64, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	if f.createID == 0 {
		f.createID = 1
	}
	return f.createID, nil
}

func (f *fakeTestCaseRepo) Update(ctx context.Context, id int64, req model.TestCaseRequest) (int64, error) {
	if f.updateErr != nil {
		return 0, f.updateErr
	}
	return f.updateRows, nil
}

func (f *fakeTestCaseRepo) Delete(ctx context.Context, id int64) (int64, error) {
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	return f.deleteRows, nil
}

func (f *fakeTestCaseRepo) ExistsProduct(ctx context.Context, id int64) bool {
	return f.productExists
}

func (f *fakeTestCaseRepo) CountMissingSteps(ctx context.Context, ids []int64) (int64, error) {
	return f.missingSteps, nil
}

func (f *fakeTestCaseRepo) ListDatasets(ctx context.Context, caseID int64) ([]model.TestCaseDataset, error) {
	return []model.TestCaseDataset{{ID: 1, CaseID: caseID, Name: "默认数据", Variables: json.RawMessage(`{"name":"admin"}`), Enabled: true}}, nil
}

func (f *fakeTestCaseRepo) CreateDataset(ctx context.Context, caseID int64, req model.TestCaseDatasetRequest) error {
	if f.datasetErr != nil {
		return f.datasetErr
	}
	return nil
}

func (f *fakeTestCaseRepo) UpdateDataset(ctx context.Context, caseID, datasetID int64, req model.TestCaseDatasetRequest) (int64, error) {
	if f.datasetErr != nil {
		return 0, f.datasetErr
	}
	return f.datasetRows, nil
}

func (f *fakeTestCaseRepo) DeleteDataset(ctx context.Context, caseID, datasetID int64) (int64, error) {
	if f.datasetErr != nil {
		return 0, f.datasetErr
	}
	return f.datasetRows, nil
}

func validTestCaseRequest() model.TestCaseRequest {
	return model.TestCaseRequest{
		ProductID:      1,
		Name:           "登录成功",
		CaseType:       "ui",
		Priority:       "P1",
		Status:         "active",
		Owner:          "admin",
		ExpectedResult: "进入首页",
		StepIDs:        []int64{1, 1, 2},
	}
}

func TestTestCaseServiceCreateSuccess(t *testing.T) {
	repo := &fakeTestCaseRepo{productExists: true, createID: 9}
	logger := &fakeOperationLogger{}
	svc := NewTestCaseServiceWithLogger(repo, logger)
	id, err := svc.Create(context.Background(), "admin", validTestCaseRequest())
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if id != 9 {
		t.Fatalf("expected id 9, got %d", id)
	}
	if logger.calls != 1 {
		t.Fatalf("expected one log call, got %d", logger.calls)
	}
}

func TestNewTestCaseServiceWithSystemRepository(t *testing.T) {
	svc := NewTestCaseService(&fakeTestCaseRepo{}, repository.NewSystemRepository(nil))
	if svc.systemRepo == nil {
		t.Fatal("expected non-nil operation logger")
	}
}

func TestTestCaseServiceListAndGet(t *testing.T) {
	svc := NewTestCaseService(&fakeTestCaseRepo{productExists: true}, nil)
	result, err := svc.List(context.Background(), model.TestCaseFilter{Name: " 登录 "}, 0, 500)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if result.Total != 1 || result.Page != 1 || result.PageSize != 100 {
		t.Fatalf("unexpected page result: %+v", result)
	}
	detail, err := svc.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if detail.ID != 1 {
		t.Fatalf("expected detail id 1, got %d", detail.ID)
	}
	if _, err := svc.Get(context.Background(), 0); err == nil {
		t.Fatal("expected invalid id error")
	}
	svc = NewTestCaseService(&fakeTestCaseRepo{getErr: errors.New("missing")}, nil)
	if _, err := svc.Get(context.Background(), 2); err == nil {
		t.Fatal("expected get missing error")
	}
}

func TestTestCaseServiceRejectsInvalidEnums(t *testing.T) {
	repo := &fakeTestCaseRepo{productExists: true}
	svc := NewTestCaseService(repo, nil)
	req := validTestCaseRequest()
	req.Priority = "P9"
	if _, err := svc.Create(context.Background(), "admin", req); err == nil {
		t.Fatal("expected invalid priority error")
	}
	req = validTestCaseRequest()
	req.CaseType = "browser"
	if _, err := svc.Create(context.Background(), "admin", req); err == nil {
		t.Fatal("expected invalid case type error")
	}
	req = validTestCaseRequest()
	req.Status = "archived"
	if _, err := svc.Create(context.Background(), "admin", req); err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestTestCaseServiceRejectsRequiredFieldsAndLength(t *testing.T) {
	svc := NewTestCaseService(&fakeTestCaseRepo{productExists: true}, nil)
	req := validTestCaseRequest()
	req.ProductID = 0
	if _, err := svc.Create(context.Background(), "admin", req); err == nil {
		t.Fatal("expected product required error")
	}
	req = validTestCaseRequest()
	req.Name = ""
	if _, err := svc.Create(context.Background(), "admin", req); err == nil {
		t.Fatal("expected name required error")
	}
	req = validTestCaseRequest()
	req.Name = strings.Repeat("测", 121)
	if _, err := svc.Create(context.Background(), "admin", req); err == nil {
		t.Fatal("expected name length error")
	}
}

func TestTestCaseServiceRejectsMissingProduct(t *testing.T) {
	svc := NewTestCaseService(&fakeTestCaseRepo{productExists: false}, nil)
	if _, err := svc.Create(context.Background(), "admin", validTestCaseRequest()); err == nil {
		t.Fatal("expected missing product error")
	}
}

func TestTestCaseServiceRejectsMissingSteps(t *testing.T) {
	svc := NewTestCaseService(&fakeTestCaseRepo{productExists: true, missingSteps: 1}, nil)
	if _, err := svc.Create(context.Background(), "admin", validTestCaseRequest()); err == nil {
		t.Fatal("expected missing steps error")
	}
}

func TestTestCaseServiceUpdateAndDeleteNotFound(t *testing.T) {
	svc := NewTestCaseService(&fakeTestCaseRepo{productExists: true, updateRows: 0, deleteRows: 0}, nil)
	if err := svc.Update(context.Background(), "admin", 1, validTestCaseRequest()); err == nil {
		t.Fatal("expected update not found error")
	}
	if err := svc.Delete(context.Background(), "admin", 1); err == nil {
		t.Fatal("expected delete not found error")
	}
}

func TestTestCaseServiceRepositoryErrors(t *testing.T) {
	req := validTestCaseRequest()
	if _, err := NewTestCaseService(&fakeTestCaseRepo{productExists: true, createErr: errors.New("duplicate")}, nil).Create(context.Background(), "admin", req); err == nil {
		t.Fatal("expected create repository error")
	}
	if err := NewTestCaseService(&fakeTestCaseRepo{productExists: true, updateErr: errors.New("duplicate")}, nil).Update(context.Background(), "admin", 1, req); err == nil {
		t.Fatal("expected update repository error")
	}
	if err := NewTestCaseService(&fakeTestCaseRepo{deleteErr: errors.New("db")}, nil).Delete(context.Background(), "admin", 1); err == nil {
		t.Fatal("expected delete repository error")
	}
}

func TestTestCaseServiceUpdateAndDeleteSuccess(t *testing.T) {
	svc := NewTestCaseService(&fakeTestCaseRepo{productExists: true, updateRows: 1, deleteRows: 1}, nil)
	if err := svc.Update(context.Background(), "admin", 1, validTestCaseRequest()); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if err := svc.Delete(context.Background(), "admin", 1); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := svc.Update(context.Background(), "admin", 0, validTestCaseRequest()); err == nil {
		t.Fatal("expected invalid update id error")
	}
	if err := svc.Delete(context.Background(), "admin", 0); err == nil {
		t.Fatal("expected invalid delete id error")
	}
}

func TestTestCaseServiceImportAndExport(t *testing.T) {
	repo := &fakeTestCaseRepo{productExists: true, createID: 9}
	logger := &fakeOperationLogger{}
	svc := NewTestCaseServiceWithLogger(repo, logger)
	count, err := svc.Import(context.Background(), "admin", model.TestCaseImportRequest{Items: []model.TestCaseRequest{validTestCaseRequest()}})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected imported count 1, got %d", count)
	}
	items, err := svc.Export(context.Background(), model.TestCaseFilter{Name: " 登录 "})
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}
	if len(items) != 1 || items[0].Name != "登录成功" {
		t.Fatalf("unexpected export items: %+v", items)
	}
	if logger.calls < 2 {
		t.Fatalf("expected import logs, got %d", logger.calls)
	}
}

func TestTestCaseServiceImportAndExportFailures(t *testing.T) {
	svc := NewTestCaseService(&fakeTestCaseRepo{productExists: true}, nil)
	if _, err := svc.Import(context.Background(), "admin", model.TestCaseImportRequest{}); err == nil {
		t.Fatal("expected empty import error")
	}
	items := make([]model.TestCaseRequest, 201)
	for i := range items {
		items[i] = validTestCaseRequest()
	}
	if _, err := svc.Import(context.Background(), "admin", model.TestCaseImportRequest{Items: items}); err == nil {
		t.Fatal("expected import size error")
	}
	bad := validTestCaseRequest()
	bad.Name = ""
	if _, err := svc.Import(context.Background(), "admin", model.TestCaseImportRequest{Items: []model.TestCaseRequest{bad}}); err == nil {
		t.Fatal("expected import validation error")
	}
	if _, err := NewTestCaseService(&fakeTestCaseRepo{listErr: errors.New("db")}, nil).Export(context.Background(), model.TestCaseFilter{}); err == nil {
		t.Fatal("expected export list error")
	}
}

func TestTestCaseServiceDatasetValidation(t *testing.T) {
	svc := NewTestCaseService(&fakeTestCaseRepo{productExists: true, datasetRows: 1}, nil)
	items, err := svc.ListDatasets(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListDatasets returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one dataset, got %d", len(items))
	}
	if err := svc.CreateDataset(context.Background(), "admin", 1, model.TestCaseDatasetRequest{Name: "默认", Variables: json.RawMessage(`{"user":"admin"}`)}); err != nil {
		t.Fatalf("CreateDataset returned error: %v", err)
	}
	if err := svc.UpdateDataset(context.Background(), "admin", 1, 1, model.TestCaseDatasetRequest{Name: "默认", Variables: json.RawMessage(`{"user":"test"}`)}); err != nil {
		t.Fatalf("UpdateDataset returned error: %v", err)
	}
	if err := svc.DeleteDataset(context.Background(), "admin", 1, 1); err != nil {
		t.Fatalf("DeleteDataset returned error: %v", err)
	}
	if _, err := svc.ListDatasets(context.Background(), 0); err == nil {
		t.Fatal("expected invalid case id error")
	}
	if err := svc.CreateDataset(context.Background(), "admin", 0, model.TestCaseDatasetRequest{Name: "默认", Variables: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected invalid create dataset case id error")
	}
	if err := svc.UpdateDataset(context.Background(), "admin", 0, 1, model.TestCaseDatasetRequest{Name: "默认", Variables: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected invalid update dataset id error")
	}
	if err := svc.DeleteDataset(context.Background(), "admin", 0, 1); err == nil {
		t.Fatal("expected invalid delete dataset id error")
	}
	if err := NewTestCaseService(&fakeTestCaseRepo{datasetRows: 0}, nil).UpdateDataset(context.Background(), "admin", 1, 1, model.TestCaseDatasetRequest{Name: "默认", Variables: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected update dataset not found error")
	}
	if err := NewTestCaseService(&fakeTestCaseRepo{datasetRows: 0}, nil).DeleteDataset(context.Background(), "admin", 1, 1); err == nil {
		t.Fatal("expected delete dataset not found error")
	}
	if err := NewTestCaseService(&fakeTestCaseRepo{datasetErr: errors.New("db")}, nil).CreateDataset(context.Background(), "admin", 1, model.TestCaseDatasetRequest{Name: "默认", Variables: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected create dataset repository error")
	}
	if err := NewTestCaseService(&fakeTestCaseRepo{datasetErr: errors.New("db")}, nil).UpdateDataset(context.Background(), "admin", 1, 1, model.TestCaseDatasetRequest{Name: "默认", Variables: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected update dataset repository error")
	}
	if err := NewTestCaseService(&fakeTestCaseRepo{datasetErr: errors.New("db")}, nil).DeleteDataset(context.Background(), "admin", 1, 1); err == nil {
		t.Fatal("expected delete dataset repository error")
	}
	if err := svc.CreateDataset(context.Background(), "admin", 1, model.TestCaseDatasetRequest{Name: "", Variables: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected empty dataset name error")
	}
	if err := svc.CreateDataset(context.Background(), "admin", 1, model.TestCaseDatasetRequest{Name: "数组", Variables: json.RawMessage(`[1,2]`)}); err == nil {
		t.Fatal("expected json object error")
	}
}
