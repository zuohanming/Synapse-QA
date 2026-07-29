package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"synapseqa/backend/internal/model"
)

func TestElementCaptureRepositoryHeartbeatUsesConditionalStateTransition(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("update element_capture_sessions").
		WithArgs("session-1", "exec-1", "context-1", "https://example.test", "token-hash").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("session-1"))

	updated, err := NewElementCaptureRepository(db).Heartbeat(context.Background(), "session-1", "exec-1", "token-hash", "context-1", "https://example.test")
	if err != nil || !updated {
		t.Fatalf("Heartbeat returned updated=%v err=%v", updated, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryBatchSaveRollsBackWhenVersionWriteFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("select page_id,status from element_capture_sessions where id=$1 and status in ('active','completed') for update")).
		WithArgs("session-1").WillReturnRows(sqlmock.NewRows([]string{"page_id", "status"}).AddRow(8, "active"))
	mock.ExpectQuery(regexp.QuoteMeta("select id from ui_assets where id=$1 and deleted_at is null for update")).
		WithArgs(8).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectQuery("from element_capture_candidates c left join page_elements p").
		WithArgs("session-1").WillReturnRows(sqlmock.NewRows([]string{"id", "cursor_id", "session_id", "name", "fingerprint", "capture_url", "tag_name", "accessible_name", "locators", "quality_score", "duplicate_element_id", "page_id", "conflict_status", "conflict_resolution", "status", "expires_at"}).AddRow("candidate-1", 1, "session-1", "submit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "https://example.test", "button", "Submit", []byte(`[{"type":"testid","value":"submit","score":95,"unique":true}]`), 95, 0, 0, "", "", "pending", time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta("select id,lower(name),fingerprint from page_elements where page_id=$1 and deleted_at is null for update")).
		WithArgs(8).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "fingerprint"}))
	mock.ExpectQuery("insert into page_elements").
		WithArgs(8, "submit", "testid", "submit", "", "", "", "", "", "", "", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "https://example.test", "button", "Submit", 95.0, "admin").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(22))
	mock.ExpectExec("insert into page_element_versions").
		WithArgs(22, 1, sqlmock.AnyArg(), "采集候选项审核入库", "admin").
		WillReturnError(errors.New("version write failed"))
	mock.ExpectRollback()

	_, err = NewElementCaptureRepository(db).SaveCandidates(context.Background(), "admin", model.CandidateBatchSaveRequest{
		SessionID: "session-1", Items: []model.CandidateSaveItem{{CandidateID: 1, Resolution: "create"}},
	}, func(model.CaptureBatchData) error { return nil })
	if err == nil || err.Error() != "version write failed" {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryExpiresTimedOutAndInterruptedSessions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("update element_capture_sessions\n\t\tset status = 'expired', updated_at = now()")).
		WithArgs(now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("update element_capture_sessions").
		WithArgs(now, now.Add(captureRecoveryWindow), now.Add(-captureRecoveryWindow)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("update element_capture_sessions").
		WithArgs(now).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := NewElementCaptureRepository(db).ExpireSessions(context.Background(), now); err != nil {
		t.Fatalf("ExpireSessions returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
