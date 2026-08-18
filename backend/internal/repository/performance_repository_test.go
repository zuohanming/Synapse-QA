package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPerformanceRepositoryListProjectionOmitsLargeFields(t *testing.T) {
	for _, column := range []string{"summary", "series", "plan_snapshot", "diagnostic_output"} {
		if strings.Contains(perfRunListColumns, column) {
			t.Fatalf("list projection must omit %s", column)
		}
	}
}

func TestPerformanceRepositoryScanPerfTestRunNullableColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("select " + perfRunColumns + " " + perfRunJoin + " where run.id = $1")).
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "plan_id", "plan_name", "scenario_type", "status", "triggered_by",
		"plan_snapshot", "config_hash", "executor_id", "executor_name", "k6_version", "environment",
		"environment_id", "environment_name", "environment_base_url", "environment_deploy_env", "requested_at", "dispatched_at", "dispatch_deadline_at", "start_deadline_at", "expected_finish_at",
		"script_hash", "generator_version", "task_id", "callback_token_hash", "idempotency_key",
		"exit_code", "duration_ms", "total_requests", "avg_duration_ms", "p95_duration_ms", "p99_duration_ms", "error_rate", "rps",
		"error_message", "failure_stage", "diagnostic_output", "needs_attention", "summary", "series",
		"started_at", "finished_at", "created_at", "updated_at", "degradation_checked_at",
	}).AddRow(
		int64(1), int64(2), "登录压测", "baseline", "pending", "admin",
		[]byte(`{"targetUrl":"http://127.0.0.1"}`), "hash", nil, "", "", "test",
		nil, "", "", "", nil, nil, nil, nil, nil,
		"", "", nil, "callback-hash", "idem-1",
		nil, nil, 0, nil, nil, 42.5, nil, nil,
		nil, "", "", false, []byte(`{}`), nil,
		nil, nil, now, now, nil,
	))

	run, err := NewPerformanceRepository(db).GetRun(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRun returned error for nullable error_message: %v", err)
	}
	if run.ID != 1 || run.PlanID != 2 || run.ErrorMessage != "" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if run.ExecutorID != "" || run.ExitCode != nil || run.AvgDurationMs != nil || run.Series != nil {
		t.Fatalf("nullable fields were not normalized: %+v", run)
	}
	if run.P99DurationMs == nil || *run.P99DurationMs != 42.5 {
		t.Fatalf("p99 was not scanned: %+v", run.P99DurationMs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetActivePerfRunUsesPlanOnlyAndNoEnvironmentArguments(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	query := "select " + perfRunColumns + " " + perfRunJoin + `
		where run.plan_id=$1
		and run.status in ('pending','queued','dispatching','dispatched','running','stopping') order by run.id desc limit 1`
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)
	_, err = NewPerformanceRepository(db).GetActivePerfRun(context.Background(), 9)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetActivePerfRun error = %v, want sql.ErrNoRows", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
