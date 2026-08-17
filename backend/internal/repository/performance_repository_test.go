package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPerformanceRepositoryScanPerfTestRunNullableColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("select "+perfRunColumns+" "+perfRunJoin+" where run.id = $1")).
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{
			"id", "plan_id", "plan_name", "scenario_type", "status", "triggered_by",
			"plan_snapshot", "config_hash", "executor_id", "executor_name", "k6_version", "environment",
			"requested_at", "dispatched_at", "dispatch_deadline_at", "start_deadline_at", "expected_finish_at",
			"script_hash", "generator_version", "task_id", "callback_token_hash", "idempotency_key",
			"exit_code", "duration_ms", "total_requests", "avg_duration_ms", "p95_duration_ms", "error_rate", "rps",
			"error_message", "failure_stage", "diagnostic_output", "needs_attention", "summary", "series",
			"started_at", "finished_at", "created_at", "updated_at",
		}).AddRow(
			int64(1), int64(2), "登录压测", "baseline", "pending", "admin",
			[]byte(`{"targetUrl":"http://127.0.0.1"}`), "hash", nil, "", "", "test",
			nil, nil, nil, nil, nil,
			"", "", nil, "callback-hash", "idem-1",
			nil, nil, 0, nil, nil, nil, nil,
			nil, "", "", false, []byte(`{}`), nil,
			nil, nil, now, now,
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
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
