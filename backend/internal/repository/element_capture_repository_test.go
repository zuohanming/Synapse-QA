package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"synapseqa/backend/internal/model"
)

type tokenFreePayload struct{}

func (tokenFreePayload) Match(value driver.Value) bool {
	bytes, ok := value.([]byte)
	return ok && !strings.Contains(string(bytes), "token") && !strings.Contains(string(bytes), "one-time")
}

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

func TestElementCaptureRepositoryPageExistsAcceptsPageAndLegacyPageElementAssets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(`from ui_assets where id = \$1 and asset_type in \('page','page_element'\) and deleted_at is null`).WithArgs(int64(8)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	exists, err := NewElementCaptureRepository(db).PageExists(context.Background(), 8)
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryPageAccessibleUsesOwnerOrAdminForRealPageAssets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`from ui_assets p[\s\S]*asset_type in \('page','page_element'\)[\s\S]*p\.created_by=\$2[\s\S]*ro\.code='admin'`).WithArgs(int64(8), "owner").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	allowed, err := NewElementCaptureRepository(db).PageAccessible(context.Background(), 8, "owner")
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryClaimsLeasedStartAndReturnsOnlyInMemoryToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`update element_capture_commands set status='expired'`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`delete from element_capture_commands`).WillReturnResult(sqlmock.NewResult(0, 0))
	payload := []byte(`{"sessionId":"session-1","type":"start"}`)
	mock.ExpectQuery(`for update skip locked[\s\S]*status='leased'`).WithArgs("exec-1", 50).WillReturnRows(sqlmock.NewRows([]string{"id", "session_id", "command_type", "payload"}).AddRow(7, "session-1", "start", payload))
	mock.ExpectExec(`update element_capture_sessions set token_hash=\$1`).WithArgs(sqlmock.AnyArg(), "session-1", "exec-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	items, err := NewElementCaptureRepository(db).ClaimCommands(context.Background(), "exec-1", 50)
	if err != nil || len(items) != 1 || items[0].ID != 7 || items[0].Token == "" || string(payload) != `{"sessionId":"session-1","type":"start"}` {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryAcknowledgesOnlyLeasedCommandOfExecutor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec(`update element_capture_commands set status='acked'`).WithArgs(int64(7), "exec-1").WillReturnResult(sqlmock.NewResult(0, 0))
	acked, err := NewElementCaptureRepository(db).AckCommand(context.Background(), "exec-1", 7)
	if err != nil || acked {
		t.Fatalf("acked=%v err=%v", acked, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryRotatesStartTokenWhenLeaseIsRetried(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for range 2 {
		mock.ExpectBegin()
		mock.ExpectExec(`update element_capture_commands set status='expired'`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`delete from element_capture_commands`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`for update skip locked[\s\S]*status='leased'`).WithArgs("exec-1", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "session_id", "command_type", "payload"}).AddRow(7, "session-1", "start", []byte(`{"sessionId":"session-1","type":"start"}`)))
		mock.ExpectExec(`update element_capture_sessions set token_hash=\$1`).WithArgs(sqlmock.AnyArg(), "session-1", "exec-1").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}
	first, err := NewElementCaptureRepository(db).ClaimCommands(context.Background(), "exec-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewElementCaptureRepository(db).ClaimCommands(context.Background(), "exec-1", 1)
	if err != nil || first[0].Token == "" || second[0].Token == "" || first[0].Token == second[0].Token {
		t.Fatalf("tokens=%q/%q err=%v", first[0].Token, second[0].Token, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryCreatesSessionAndStartCommandAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`select p\.id from ui_assets p[\s\S]*asset_type in \('page','page_element'\)[\s\S]*for update`).WithArgs(int64(8), "owner").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectExec(`insert into element_capture_sessions`).WithArgs("session-1", int64(8), "exec-1", "", "chrome", "owner", "starting", "pick", "https://example.test", "", now, now.Add(30*time.Minute)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`insert into element_capture_commands`).WithArgs("session-1", "exec-1", "start", tokenFreePayload{}, now.Add(30*time.Minute)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	session := model.ElementCaptureSession{ID: "session-1", PageID: 8, ExecutorID: "exec-1", BrowserChannel: "chrome", CreatedBy: "owner", Status: "starting", Mode: "pick", CurrentURL: "https://example.test", LastHeartbeatAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	err = NewElementCaptureRepository(db).CreateSessionWithStartCommand(context.Background(), "owner", session, model.ElementCaptureCommand{SessionID: "session-1", Type: "start", Token: "one-time"})
	if err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
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
	mock.ExpectQuery(`select page_id,status from element_capture_sessions[\s\S]*created_by=\$2[\s\S]*for update`).
		WithArgs("session-1", "admin").WillReturnRows(sqlmock.NewRows([]string{"page_id", "status"}).AddRow(8, "active"))
	mock.ExpectQuery(regexp.QuoteMeta("select id from ui_assets where id=$1 and deleted_at is null for update")).
		WithArgs(8).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(8))
	mock.ExpectQuery("from element_capture_candidates c left join page_elements p").
		WithArgs("session-1").WillReturnRows(sqlmock.NewRows([]string{"id", "cursor_id", "session_id", "name", "fingerprint", "capture_url", "tag_name", "accessible_name", "locators", "quality_score", "duplicate_element_id", "page_id", "conflict_status", "conflict_resolution", "status", "expires_at"}).AddRow("candidate-1", 1, "session-1", "submit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "https://example.test", "button", "Submit", []byte(`[{"type":"testid","value":"submit","score":95,"unique":true}]`), 95, 0, 0, "", "", "pending", time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta("select id,lower(name),fingerprint from page_elements where page_id=$1 and deleted_at is null for update")).
		WithArgs(8).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "fingerprint"}))
	mock.ExpectQuery(`insert into page_elements[\s\S]*captured_at,current_version\)[\s\S]*now\(\),1\) returning id`).
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
	mock.ExpectBegin()
	mock.ExpectQuery(`update element_capture_sessions set status='expired'[\s\S]*returning id,executor_id`).
		WithArgs(now).WillReturnRows(sqlmock.NewRows([]string{"id", "executor_id"}).AddRow("session-1", "exec-1"))
	mock.ExpectExec(`insert into element_capture_commands`).WithArgs("session-1", "exec-1", "expire", sqlmock.AnyArg(), now.Add(30*time.Minute)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("update element_capture_sessions").
		WithArgs(now, now.Add(captureRecoveryWindow), now.Add(-captureRecoveryWindow)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`update element_capture_sessions set status='expired'[\s\S]*recovery_expires_at <= \$1[\s\S]*returning id,executor_id`).
		WithArgs(now).WillReturnRows(sqlmock.NewRows([]string{"id", "executor_id"}))
	mock.ExpectCommit()

	if err := NewElementCaptureRepository(db).ExpireSessions(context.Background(), now); err != nil {
		t.Fatalf("ExpireSessions returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryRollbackWritesNextVersionInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`select e\.current_version from page_elements e join ui_assets p[\s\S]*p\.created_by=\$2[\s\S]*for update of e,p`).
		WithArgs(int64(22), "admin").WillReturnRows(sqlmock.NewRows([]string{"current_version"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta("select snapshot from page_element_versions where page_element_id=$1 and version=$2")).
		WithArgs(int64(22), 1).WillReturnRows(sqlmock.NewRows([]string{"snapshot"}).AddRow([]byte(`{"id":22,"version":1,"name":"submit","fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","captureUrl":"https://example.test","tagName":"button","accessibleName":"Submit","locators":[{"type":"testid","value":"submit","index":"","score":95,"unique":true}],"qualityScore":95}`)))
	mock.ExpectExec("update page_elements set name=\\$1").
		WithArgs("submit", "testid", "submit", "", "", "", "", "", "", "", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "rollback", "https://example.test", "button", "Submit", 95.0, "admin", 4, int64(22)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("insert into page_element_versions").
		WithArgs(int64(22), 4, sqlmock.AnyArg(), "回滚至版本 1", "admin").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(12, time.Now()))
	mock.ExpectExec("insert into operation_logs").WithArgs("admin", "22:1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	version, err := NewElementCaptureRepository(db).RollbackVersion(context.Background(), "admin", 22, 1)
	if err != nil || version.Version != 4 || version.PageElementID != 22 {
		t.Fatalf("version=%+v err=%v", version, err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(version.Snapshot, &snapshot); err != nil || snapshot["id"].(float64) != 22 || snapshot["version"].(float64) != 4 || snapshot["source"] != "rollback" {
		t.Fatalf("snapshot=%s err=%v", version.Snapshot, err)
	}
	locator := snapshot["locators"].([]any)[0].(map[string]any)
	if locator["score"].(float64) != 95 || locator["unique"] != true {
		t.Fatalf("locator=%+v", locator)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryListVersionsRejectsInvisiblePage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`select exists\([\s\S]*page_elements e join ui_assets p[\s\S]*asset_type='page'`).WithArgs(int64(22), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	_, err = NewElementCaptureRepository(db).ListVersions(context.Background(), 7, 22)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryAuthorizesExecutorBySessionAndTokenHash(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`select exists\(select 1 from element_capture_sessions where id=\$1 and executor_id=\$2 and token_hash=\$3\)`).WithArgs("session-1", "exec-1", "hash").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	allowed, err := NewElementCaptureRepository(db).AuthorizeExecutor(context.Background(), "session-1", "exec-1", "hash")
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
