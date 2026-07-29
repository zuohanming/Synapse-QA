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
	mock.ExpectQuery(regexp.QuoteMeta("select page_id from element_capture_sessions where id=$1 and status in ('active','completed') for update")).
		WithArgs("session-1").WillReturnRows(sqlmock.NewRows([]string{"page_id"}).AddRow(8))
	mock.ExpectQuery(regexp.QuoteMeta("select id from element_capture_candidates where session_id=$1 for update")).
		WithArgs("session-1").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("insert into page_elements").
		WithArgs(8, "submit", "testid", "submit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "https://example.test", "button", "Submit", 95.0, "admin").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(22))
	mock.ExpectExec("insert into page_element_versions").
		WithArgs(22, 1, sqlmock.AnyArg(), "采集候选项审核入库", "admin").
		WillReturnError(errors.New("version write failed"))
	mock.ExpectRollback()

	_, err = NewElementCaptureRepository(db).SaveCandidates(context.Background(), "admin", model.CandidateBatchSaveRequest{
		SessionID: "session-1", Items: []model.CandidateSaveItem{{CandidateID: 1, Resolution: "create"}},
	}, []model.ElementCaptureCandidate{{
		ID: 1, Name: "submit", Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CaptureURL: "https://example.test", TagName: "button", AccessibleName: "Submit", QualityScore: 95,
		Locators: []byte(`[{"type":"testid","value":"submit","score":95,"unique":true}]`),
	}})
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
