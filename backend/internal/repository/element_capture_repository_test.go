package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
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
