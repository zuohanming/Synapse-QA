package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"synapseqa/backend/internal/model"
)

func newTestCaseRepoMock(t *testing.T) (*TestCaseRepository, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	return NewTestCaseRepository(db), mock, func() { _ = db.Close() }
}

func TestTestCaseRepositoryList(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("select count(*) from test_cases tc where tc.deleted_at is null and tc.id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("select tc.id").
		WithArgs(int64(1), 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "product_id", "product_name", "module_id", "module_name", "page_id", "page_name", "name", "case_type", "priority", "status", "owner", "tags", "description", "preconditions", "expected_result", "data_enabled", "created_by", "created_at", "updated_at"}).
			AddRow(1, 2, "项目/产品", 3, "模块", 4, "页面", "登录成功", "ui", "P1", "active", "admin", "smoke", "desc", "pre", "ok", true, "admin", now, now))
	items, total, err := repo.List(context.Background(), model.TestCaseFilter{ID: "1"}, 1, 20)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Name != "登录成功" {
		t.Fatalf("unexpected list result: total=%d items=%+v", total, items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryListAllFilters(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	now := time.Now()
	mock.ExpectQuery("select count").
		WithArgs(int64(2), "%登录%", int64(3), int64(4), "ui", "P1", "active", "%admin%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("select tc.id").
		WithArgs(int64(2), "%登录%", int64(3), int64(4), "ui", "P1", "active", "%admin%", 10, 10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "product_id", "product_name", "module_id", "module_name", "page_id", "page_name", "name", "case_type", "priority", "status", "owner", "tags", "description", "preconditions", "expected_result", "data_enabled", "created_by", "created_at", "updated_at"}).
			AddRow(2, 3, "项目/产品", 4, "模块", 5, "页面", "登录", "ui", "P1", "active", "admin", "", "", "", "", false, "admin", now, now))
	_, _, err := repo.List(context.Background(), model.TestCaseFilter{
		ID: "2", Name: "登录", ProductID: "3", ModuleID: "4", CaseType: "ui", Priority: "P1", Status: "active", Owner: "admin",
	}, 2, 10)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryListRejectsInvalidIDs(t *testing.T) {
	repo, _, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	if _, _, err := repo.List(context.Background(), model.TestCaseFilter{ID: "bad"}, 1, 20); err == nil {
		t.Fatal("expected invalid id error")
	}
	if _, _, err := repo.List(context.Background(), model.TestCaseFilter{ProductID: "bad"}, 1, 20); err == nil {
		t.Fatal("expected invalid product id error")
	}
	if _, _, err := repo.List(context.Background(), model.TestCaseFilter{ModuleID: "bad"}, 1, 20); err == nil {
		t.Fatal("expected invalid module id error")
	}
}

func TestTestCaseRepositoryCreateWithSteps(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectBegin()
	mock.ExpectQuery("insert into test_cases").WithArgs(int64(1), int64(2), int64(3), "登录成功", "ui", "P1", "active", "admin", "smoke", "desc", "pre", "ok", true, "admin").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectExec(regexp.QuoteMeta("delete from test_case_steps where case_id = $1")).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("insert into test_case_steps").WithArgs(int64(9), int64(10), 1).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("insert into test_case_steps").WithArgs(int64(9), int64(11), 2).WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()
	id, err := repo.Create(context.Background(), model.TestCaseRequest{
		ProductID: 1, ModuleID: 2, PageID: 3, Name: "登录成功", CaseType: "ui", Priority: "P1", Status: "active",
		Owner: "admin", Tags: "smoke", Description: "desc", Preconditions: "pre", ExpectedResult: "ok", DataEnabled: true, StepIDs: []int64{10, 11},
	}, "admin")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if id != 9 {
		t.Fatalf("expected id 9, got %d", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryGetDetail(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	now := time.Now()
	caseRows := sqlmock.NewRows([]string{"id", "product_id", "product_name", "module_id", "module_name", "page_id", "page_name", "name", "case_type", "priority", "status", "owner", "tags", "description", "preconditions", "expected_result", "data_enabled", "created_by", "created_at", "updated_at"}).
		AddRow(1, 2, "项目/产品", 3, "模块", 4, "页面", "登录成功", "ui", "P1", "active", "admin", "smoke", "desc", "pre", "ok", true, "admin", now, now)
	mock.ExpectQuery("select tc.id").WithArgs(int64(1)).WillReturnRows(caseRows)
	mock.ExpectQuery("select tcs.id").WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "case_id", "step_id", "step_name", "sort_order", "note", "created_at"}).
			AddRow(1, 1, 10, "输入账号", 1, "", now))
	mock.ExpectQuery("select id, case_id, name, variables, enabled, created_at, updated_at from test_case_datasets").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "case_id", "name", "variables", "enabled", "created_at", "updated_at"}).
			AddRow(1, 1, "默认", []byte(`{"user":"admin"}`), true, now, now))
	detail, err := repo.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if detail.ID != 1 || len(detail.Steps) != 1 || len(detail.Datasets) != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryGetPropagatesStepAndDatasetErrors(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	now := time.Now()
	caseRows := sqlmock.NewRows([]string{"id", "product_id", "product_name", "module_id", "module_name", "page_id", "page_name", "name", "case_type", "priority", "status", "owner", "tags", "description", "preconditions", "expected_result", "data_enabled", "created_by", "created_at", "updated_at"}).
		AddRow(1, 2, "项目/产品", 0, "", 0, "", "登录成功", "ui", "P1", "active", "admin", "", "", "", "", false, "admin", now, now)
	mock.ExpectQuery("select tc.id").WithArgs(int64(1)).WillReturnRows(caseRows)
	mock.ExpectQuery("select tcs.id").WithArgs(int64(1)).WillReturnError(sql.ErrConnDone)
	if _, err := repo.Get(context.Background(), 1); err == nil {
		t.Fatal("expected step query error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	repo, mock, closeFn = newTestCaseRepoMock(t)
	defer closeFn()
	caseRows = sqlmock.NewRows([]string{"id", "product_id", "product_name", "module_id", "module_name", "page_id", "page_name", "name", "case_type", "priority", "status", "owner", "tags", "description", "preconditions", "expected_result", "data_enabled", "created_by", "created_at", "updated_at"}).
		AddRow(1, 2, "项目/产品", 0, "", 0, "", "登录成功", "ui", "P1", "active", "admin", "", "", "", "", false, "admin", now, now)
	mock.ExpectQuery("select tc.id").WithArgs(int64(1)).WillReturnRows(caseRows)
	mock.ExpectQuery("select tcs.id").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "case_id", "step_id", "step_name", "sort_order", "note", "created_at"}))
	mock.ExpectQuery("select id, case_id, name, variables, enabled, created_at, updated_at from test_case_datasets").WithArgs(int64(1)).WillReturnError(sql.ErrConnDone)
	if _, err := repo.Get(context.Background(), 1); err == nil {
		t.Fatal("expected dataset query error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryUpdateDeleteAndDatasets(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectBegin()
	mock.ExpectExec("update test_cases set").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("delete from test_case_steps where case_id = $1")).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	rows, err := repo.Update(context.Background(), 1, model.TestCaseRequest{ProductID: 1, Name: "登录成功", CaseType: "ui", Priority: "P1", Status: "active"})
	if err != nil || rows != 1 {
		t.Fatalf("Update rows=%d err=%v", rows, err)
	}
	mock.ExpectExec("update test_cases set deleted_at").WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	rows, err = repo.Delete(context.Background(), 1)
	if err != nil || rows != 1 {
		t.Fatalf("Delete rows=%d err=%v", rows, err)
	}
	mock.ExpectExec("insert into test_case_datasets").WithArgs(int64(1), "默认", []byte(`{"a":1}`), true).WillReturnResult(sqlmock.NewResult(1, 1))
	err = repo.CreateDataset(context.Background(), 1, model.TestCaseDatasetRequest{Name: "默认", Variables: []byte(`{"a":1}`), Enabled: true})
	if err != nil {
		t.Fatalf("CreateDataset err=%v", err)
	}
	mock.ExpectExec("update test_case_datasets set").WithArgs("默认", []byte(`{"a":2}`), true, int64(2), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	rows, err = repo.UpdateDataset(context.Background(), 1, 2, model.TestCaseDatasetRequest{Name: "默认", Variables: []byte(`{"a":2}`), Enabled: true})
	if err != nil || rows != 1 {
		t.Fatalf("UpdateDataset rows=%d err=%v", rows, err)
	}
	mock.ExpectExec("delete from test_case_datasets").WithArgs(int64(2), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	rows, err = repo.DeleteDataset(context.Background(), 1, 2)
	if err != nil || rows != 1 {
		t.Fatalf("DeleteDataset rows=%d err=%v", rows, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryUpdateDeleteErrors(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectBegin()
	mock.ExpectExec("update test_cases set").WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()
	if _, err := repo.Update(context.Background(), 1, model.TestCaseRequest{ProductID: 1, Name: "登录", CaseType: "ui", Priority: "P1", Status: "active"}); err == nil {
		t.Fatal("expected update exec error")
	}
	mock.ExpectExec("update test_cases set deleted_at").WithArgs(int64(1)).WillReturnError(sql.ErrConnDone)
	if _, err := repo.Delete(context.Background(), 1); err == nil {
		t.Fatal("expected delete exec error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	repo, mock, closeFn = newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectBegin()
	mock.ExpectExec("update test_cases set").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	rows, err := repo.Update(context.Background(), 1, model.TestCaseRequest{ProductID: 1, Name: "登录", CaseType: "ui", Priority: "P1", Status: "active"})
	if err != nil || rows != 0 {
		t.Fatalf("expected zero rows without error, rows=%d err=%v", rows, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	repo, mock, closeFn = newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectBegin()
	mock.ExpectExec("update test_cases set").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("delete from test_case_steps where case_id = $1")).WithArgs(int64(1)).WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()
	if _, err := repo.Update(context.Background(), 1, model.TestCaseRequest{ProductID: 1, Name: "登录", CaseType: "ui", Priority: "P1", Status: "active"}); err == nil {
		t.Fatal("expected update replace steps error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryDatasetErrors(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectQuery("select id, case_id, name, variables, enabled, created_at, updated_at from test_case_datasets").
		WithArgs(int64(1)).
		WillReturnError(sql.ErrConnDone)
	if _, err := repo.ListDatasets(context.Background(), 1); err == nil {
		t.Fatal("expected list datasets error")
	}
	mock.ExpectExec("insert into test_case_datasets").WithArgs(int64(1), "默认", []byte(`{}`), true).WillReturnError(sql.ErrConnDone)
	if err := repo.CreateDataset(context.Background(), 1, model.TestCaseDatasetRequest{Name: "默认", Variables: []byte(`{}`), Enabled: true}); err == nil {
		t.Fatal("expected create dataset error")
	}
	mock.ExpectExec("update test_case_datasets set").WithArgs("默认", []byte(`{}`), true, int64(2), int64(1)).WillReturnError(sql.ErrConnDone)
	if _, err := repo.UpdateDataset(context.Background(), 1, 2, model.TestCaseDatasetRequest{Name: "默认", Variables: []byte(`{}`), Enabled: true}); err == nil {
		t.Fatal("expected update dataset error")
	}
	mock.ExpectExec("delete from test_case_datasets").WithArgs(int64(2), int64(1)).WillReturnError(sql.ErrConnDone)
	if _, err := repo.DeleteDataset(context.Background(), 1, 2); err == nil {
		t.Fatal("expected delete dataset error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryCreateRollbackOnStepError(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectBegin()
	mock.ExpectQuery("insert into test_cases").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(regexp.QuoteMeta("delete from test_case_steps where case_id = $1")).WithArgs(int64(1)).WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()
	if _, err := repo.Create(context.Background(), model.TestCaseRequest{ProductID: 1, Name: "登录", CaseType: "ui", Priority: "P1", Status: "active"}, "admin"); err == nil {
		t.Fatal("expected create step error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	repo, mock, closeFn = newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectBegin()
	mock.ExpectQuery("insert into test_cases").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(regexp.QuoteMeta("delete from test_case_steps where case_id = $1")).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("insert into test_case_steps").WithArgs(int64(1), int64(10), 1).WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()
	if _, err := repo.Create(context.Background(), model.TestCaseRequest{ProductID: 1, Name: "登录", CaseType: "ui", Priority: "P1", Status: "active", StepIDs: []int64{10}}, "admin"); err == nil {
		t.Fatal("expected create insert step error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryCreateInsertError(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectBegin()
	mock.ExpectQuery("insert into test_cases").WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()
	if _, err := repo.Create(context.Background(), model.TestCaseRequest{ProductID: 1, Name: "登录", CaseType: "ui", Priority: "P1", Status: "active"}, "admin"); err == nil {
		t.Fatal("expected create insert error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTestCaseRepositoryExistsAndMissingSteps(t *testing.T) {
	repo, mock, closeFn := newTestCaseRepoMock(t)
	defer closeFn()
	mock.ExpectQuery("select exists").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	if !repo.ExistsProduct(context.Background(), 1) {
		t.Fatal("expected product exists")
	}
	missing, err := repo.CountMissingSteps(context.Background(), nil)
	if err != nil || missing != 0 {
		t.Fatalf("expected no missing for empty ids, missing=%d err=%v", missing, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta("select id from ui_assets where asset_type = 'page_step' and deleted_at is null")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(3))
	missing, err = repo.CountMissingSteps(context.Background(), []int64{1, 2, 3})
	if err != nil {
		t.Fatalf("CountMissingSteps err=%v", err)
	}
	if missing != 1 {
		t.Fatalf("expected one missing step, got %d", missing)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

var _ *sql.DB
