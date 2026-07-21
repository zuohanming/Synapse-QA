package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"synapseqa/backend/internal/model"
)

func newExecutionRepoMock(t *testing.T) (*ExecutionRepository, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	return NewExecutionRepository(db), mock, func() { _ = db.Close() }
}

func TestExecutionRepositoryCreateRun(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	now := time.Now()
	mock.ExpectQuery("insert into execution_runs").
		WithArgs("ui", "pending", "admin", []byte(`[1]`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at"}).
			AddRow(1, "ui", "pending", "admin", []byte(`[1]`), []byte(`{}`), nil, nil, now, now))
	run, err := repo.CreateRun(context.Background(), model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}}, "admin")
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if run.ID != 1 || run.Status != "pending" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryGetAndListRuns(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	now := time.Now()
	mock.ExpectQuery("select id, run_type, status").WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at"}).
			AddRow(1, "ui", "pending", "admin", []byte(`[1,2]`), []byte(`{}`), nil, nil, now, now))
	run, err := repo.GetRun(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if len(run.CaseIDs) != 2 {
		t.Fatalf("expected 2 case ids, got %d", len(run.CaseIDs))
	}

	mock.ExpectQuery("select count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("select id, run_type, status").WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at"}).
		AddRow(1, "ui", "pending", "admin", []byte(`[1]`), []byte(`{}`), nil, nil, now, now))
	items, total, err := repo.ListRuns(context.Background(), model.ExecutionRunFilter{}, 1, 20)
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("unexpected list result: total=%d items=%d", total, len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryCreateAndGetTask(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	now := time.Now()
	payload := []byte(`{"caseId":1}`)
	mock.ExpectQuery("insert into execution_tasks").WithArgs(int64(1), "task-1", int64(2), "exec-1", "ui", payload, "http://cb").
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_id", "task_id", "case_id", "executor_id", "task_type", "payload", "callback_url", "status", "result", "started_at", "finished_at", "created_at", "updated_at"}).
			AddRow(1, 1, "task-1", 2, "exec-1", "ui", payload, "http://cb", "queued", []byte(`{}`), nil, nil, now, now))
	task, err := repo.CreateTask(context.Background(), 1, "task-1", 2, "exec-1", "ui", "http://cb", payload)
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if task.TaskID != "task-1" {
		t.Fatalf("unexpected task id: %s", task.TaskID)
	}

	mock.ExpectQuery("select id, run_id, task_id").WithArgs("task-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_id", "task_id", "case_id", "executor_id", "task_type", "payload", "callback_url", "status", "result", "started_at", "finished_at", "created_at", "updated_at"}).
			AddRow(1, 1, "task-1", 2, "exec-1", "ui", payload, "http://cb", "queued", []byte(`{}`), nil, nil, now, now))
	found, err := repo.GetTaskByTaskID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetTaskByTaskID returned error: %v", err)
	}
	if found.ID != task.ID {
		t.Fatalf("unexpected task")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryListTasksByRun(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	now := time.Now()
	mock.ExpectQuery("select id, run_id, task_id").WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_id", "task_id", "case_id", "executor_id", "task_type", "payload", "callback_url", "status", "result", "started_at", "finished_at", "created_at", "updated_at"}).
			AddRow(1, 1, "task-1", 2, "exec-1", "ui", []byte(`{}`), "", "queued", []byte(`{}`), nil, nil, now, now).
			AddRow(2, 1, "task-2", 3, "exec-1", "ui", []byte(`{}`), "", "queued", []byte(`{}`), nil, nil, now, now))
	items, err := repo.ListTasksByRun(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListTasksByRun returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryUpdateTaskStatusAndResult(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	mock.ExpectExec("update execution_tasks").WithArgs("running", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateTaskStatus(context.Background(), 1, "running"); err != nil {
		t.Fatalf("UpdateTaskStatus returned error: %v", err)
	}
	result := []byte(`{"exitCode":0}`)
	mock.ExpectExec(regexp.QuoteMeta("update execution_tasks set result = $1, updated_at = now() where id = $2")).WithArgs(result, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateTaskResult(context.Background(), 1, result); err != nil {
		t.Fatalf("UpdateTaskResult returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryLogs(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	mock.ExpectExec("insert into execution_logs").WithArgs(int64(1), "info", "started").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.CreateLog(context.Background(), 1, "info", "started"); err != nil {
		t.Fatalf("CreateLog returned error: %v", err)
	}
	now := time.Now()
	mock.ExpectQuery("select id, task_id, level, message, created_at").WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "level", "message", "created_at"}).
			AddRow(1, 1, "info", "started", now))
	logs, err := repo.ListLogs(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListLogs returned error: %v", err)
	}
	if len(logs) != 1 || logs[0].Message != "started" {
		t.Fatalf("unexpected logs: %+v", logs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryUpdateRunStatus(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	summary := []byte(`{"total":1,"passed":1}`)
	mock.ExpectExec("update execution_runs").WithArgs("completed", summary, sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateRunStatus(context.Background(), 1, "completed", summary); err != nil {
		t.Fatalf("UpdateRunStatus returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryListRunsFilters(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	now := time.Now()
	mock.ExpectQuery("select count").WithArgs("ui", "running").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("select id, run_type, status").WithArgs("ui", "running", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at"}).
			AddRow(1, "ui", "running", "admin", []byte(`[1]`), []byte(`{}`), nil, nil, now, now))
	_, _, err := repo.ListRuns(context.Background(), model.ExecutionRunFilter{RunType: "ui", Status: "running"}, 1, 20)
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryTaskListFilters(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	now := time.Now()
	mock.ExpectQuery("select count").WithArgs(int64(1), "exec-1", "success").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("select id, run_id, task_id").WithArgs(int64(1), "exec-1", "success", 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_id", "task_id", "case_id", "executor_id", "task_type", "payload", "callback_url", "status", "result", "started_at", "finished_at", "created_at", "updated_at"}).
			AddRow(1, 1, "task-1", 2, "exec-1", "ui", []byte(`{}`), "", "success", []byte(`{}`), nil, nil, now, now))
	_, _, err := repo.ListTasks(context.Background(), model.ExecutionTaskFilter{RunID: "1", ExecutorID: "exec-1", Status: "success"}, 1, 10)
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryInvalidIDs(t *testing.T) {
	repo, _, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	if _, _, err := repo.ListRuns(context.Background(), model.ExecutionRunFilter{ID: "bad"}, 1, 20); err == nil {
		t.Fatal("expected invalid run id error")
	}
	if _, _, err := repo.ListTasks(context.Background(), model.ExecutionTaskFilter{RunID: "bad"}, 1, 20); err == nil {
		t.Fatal("expected invalid task run id error")
	}
}

var _ json.RawMessage
