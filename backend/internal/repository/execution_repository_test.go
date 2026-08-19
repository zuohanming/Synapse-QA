package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
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
		WithArgs("ui", "pending", true, "admin", "{1}").
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "headless", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at", "project_id"}).
			AddRow(1, "ui", "pending", true, "admin", []byte(`[1]`), []byte(`{}`), nil, nil, now, now, nil))
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

func TestExecutionRepositoryStatisticsScopedAggregatesInDatabase(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	now := time.Date(2026, 8, 19, 15, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	trendStart := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	currentStart := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	previousStart := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	trendEnd := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	columns := []string{
		"total_runs", "total_cases", "passed_cases", "failed_cases", "failed_runs", "running_runs",
		"current_runs", "current_cases", "current_passed", "previous_runs", "previous_cases", "previous_passed",
		"bucket", "trend_runs", "trend_cases", "trend_passed",
	}
	rows := sqlmock.NewRows(columns)
	for index := 0; index < 14; index++ {
		rows.AddRow(int64(3), int64(30), int64(24), int64(6), int64(1), int64(1),
			int64(2), int64(20), int64(16), int64(1), int64(10), int64(8),
			trendStart.AddDate(0, 0, index), int64(1), int64(2), int64(1))
	}
	mock.ExpectQuery("(?s)with scoped_runs.*project_members").
		WithArgs(int64(7), false, trendStart, currentStart, previousStart, trendEnd).
		WillReturnRows(rows)

	result, err := repo.StatisticsScoped(context.Background(), 7, false, now)
	if err != nil {
		t.Fatalf("StatisticsScoped returned error: %v", err)
	}
	if result.TotalRuns != 3 || result.TotalCases != 30 || result.PassedCases != 24 || result.FailedRuns != 1 {
		t.Fatalf("unexpected totals: %+v", result)
	}
	if result.PassRate != 80 || result.RunChange != 100 || result.CaseChange != 100 || result.PassRateChange != 0 {
		t.Fatalf("unexpected rates: %+v", result)
	}
	if len(result.Trend) != 14 || result.Trend[0].Date != "08-06" || result.Trend[0].PassRate != 50 {
		t.Fatalf("unexpected trend: %+v", result.Trend)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionRepositoryCreateRunWithProject(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	now := time.Now()
	projectID := int64(9)
	mock.ExpectQuery("insert into execution_runs").
		WithArgs("ui", "pending", true, "member", "{1}", projectID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "headless", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at", "project_id"}).
			AddRow(1, "ui", "pending", true, "member", []byte(`[1]`), []byte(`{}`), nil, nil, now, now, projectID))
	run, err := repo.CreateRunWithProject(context.Background(), model.ExecutionRunRequest{RunType: "ui", CaseIDs: []int64{1}}, "member", &projectID)
	if err != nil {
		t.Fatalf("CreateRunWithProject returned error: %v", err)
	}
	if run.ProjectID == nil || *run.ProjectID != projectID {
		t.Fatalf("unexpected project id: %+v", run.ProjectID)
	}
}

func TestExecutionRepositoryResolveRunProject(t *testing.T) {
	cases := []struct {
		name       string
		projectID  any
		caseCount  int64
		validCount int64
		projects   int64
		authorized bool
		want       int64
		wantErr    string
	}{
		{name: "same project member", projectID: int64(9), caseCount: 2, validCount: 2, projects: 1, authorized: true, want: 9},
		{name: "cross project", projectID: int64(9), caseCount: 2, validCount: 2, projects: 2, authorized: true, wantErr: "同一项目"},
		{name: "not member", projectID: int64(9), caseCount: 2, validCount: 2, projects: 1, authorized: false, wantErr: "无权访问"},
		{name: "missing case", projectID: nil, caseCount: 2, validCount: 1, projects: 1, authorized: true, wantErr: "用例不存在"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			repo, mock, closeFn := newExecutionRepoMock(t)
			defer closeFn()
			mock.ExpectQuery("select min\\(p.id\\)").WithArgs("{1,2}", false, int64(7)).
				WillReturnRows(sqlmock.NewRows([]string{"project_id", "case_count", "valid_case_count", "project_count", "authorized"}).
					AddRow(item.projectID, item.caseCount, item.validCount, item.projects, item.authorized))
			got, err := repo.ResolveRunProject(context.Background(), []int64{1, 2}, 7, false)
			if item.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), item.wantErr) {
					t.Fatalf("expected error containing %q, got %v", item.wantErr, err)
				}
			} else if err != nil || got != item.want {
				t.Fatalf("resolved project=%d err=%v, want project=%d", got, err, item.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExecutionRepositoryScopedRunsIncludeProjectFilter(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	now := time.Now()
	mock.ExpectQuery("select count").WithArgs(int64(7), false).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("select er.id, er.run_type, er.status").WithArgs(int64(7), false, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "headless", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at", "project_id"}).
			AddRow(1, "ui", "running", true, "member", []byte(`[1]`), []byte(`{}`), nil, nil, now, now, int64(9)))
	items, total, err := repo.ListRunsScoped(context.Background(), 7, false, model.ExecutionRunFilter{}, 1, 20)
	if err != nil || total != 1 || len(items) != 1 || items[0].ProjectID == nil || *items[0].ProjectID != 9 {
		t.Fatalf("unexpected scoped list: total=%d items=%+v err=%v", total, items, err)
	}
	mock.ExpectQuery("select count").WithArgs(int64(8), false).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("select er.id, er.run_type, er.status").WithArgs(int64(8), false, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "headless", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at", "project_id"}))
	hidden, total, err := repo.ListRunsScoped(context.Background(), 8, false, model.ExecutionRunFilter{}, 1, 20)
	if err != nil || total != 0 || len(hidden) != 0 {
		t.Fatalf("non-member should not see runs: total=%d items=%d err=%v", total, len(hidden), err)
	}

	mock.ExpectQuery("select er.id, er.run_type, er.status").WithArgs(int64(99), int64(1), true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "headless", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at", "project_id"}).
			AddRow(99, "ui", "completed", true, "old", []byte(`[1]`), []byte(`{}`), nil, nil, now, now, nil))
	historical, err := repo.GetRunScoped(context.Background(), 1, true, 99)
	if err != nil || historical.ProjectID != nil {
		t.Fatalf("admin should see historical NULL project run: %+v err=%v", historical, err)
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "headless", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at", "project_id"}).
			AddRow(1, "ui", "pending", true, "admin", []byte(`{91,50,53,44,50,52,93}`), []byte(`{}`), nil, nil, now, now, nil))
	run, err := repo.GetRun(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if len(run.CaseIDs) != 2 || run.CaseIDs[0] != 25 || run.CaseIDs[1] != 24 {
		t.Fatalf("expected legacy case ids [25 24], got %v", run.CaseIDs)
	}

	mock.ExpectQuery("select count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("select id, run_type, status").WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "headless", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at", "project_id"}).
		AddRow(1, "ui", "pending", true, "admin", []byte(`[1]`), []byte(`{}`), nil, nil, now, now, nil))
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

func TestExecutionRepositoryCountActiveTasksByExecutor(t *testing.T) {
	repo, mock, closeFn := newExecutionRepoMock(t)
	defer closeFn()
	mock.ExpectQuery("select count").WithArgs("exec-1").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	count, err := repo.CountActiveTasksByExecutor(context.Background(), "exec-1")
	if err != nil || count != 3 {
		t.Fatalf("unexpected active task count: count=%d err=%v", count, err)
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "run_type", "status", "headless", "triggered_by", "case_ids", "summary", "started_at", "finished_at", "created_at", "updated_at", "project_id"}).
			AddRow(1, "ui", "running", true, "admin", []byte(`[1]`), []byte(`{}`), nil, nil, now, now, nil))
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
