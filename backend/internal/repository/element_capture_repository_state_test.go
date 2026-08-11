package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"synapseqa/backend/internal/model"
)

func captureReceiptHash(receipt string) string {
	hash := sha256.Sum256([]byte(receipt))
	return hex.EncodeToString(hash[:])
}

func TestElementCaptureRepositoryHeartbeatAndAckStartStateMachine(t *testing.T) {
	const (
		sessionID      = "session-1"
		executorID     = "exec-1"
		tokenHash      = "token-hash"
		browserContext = "context-1"
		currentURL     = "https://example.test"
		validReceipt   = "valid-receipt"
		invalidReceipt = "wrong-receipt"
	)

	tests := []struct {
		name       string
		status     string
		receipt    string
		commandRow bool
		ackRows    int64
		dbErr      error
		wantOK     bool
		wantErr    error
	}{
		{name: "leased success", status: "leased", receipt: validReceipt, commandRow: true, ackRows: 1, wantOK: true},
		{name: "wrong receipt", status: "leased", receipt: invalidReceipt, commandRow: true, wantErr: model.ErrConflict},
		{name: "expired lease rolls back heartbeat", status: "leased", receipt: validReceipt, commandRow: true, ackRows: 0, wantErr: model.ErrConflict},
		{name: "queued command", status: "queued", receipt: validReceipt, commandRow: true, wantErr: model.ErrConflict},
		{name: "expired command", status: "expired", receipt: validReceipt, commandRow: true, wantErr: model.ErrConflict},
		{name: "missing command", receipt: validReceipt, wantErr: model.ErrConflict},
		{name: "acked follow-up without receipt", status: "acked", commandRow: true, wantOK: true},
		{name: "acked response-loss retry accepts same receipt", status: "acked", receipt: validReceipt, commandRow: true, wantOK: true},
		{name: "acked retry rejects different receipt", status: "acked", receipt: invalidReceipt, commandRow: true, wantErr: model.ErrConflict},
		{name: "database error rolls back", dbErr: errors.New("command read failed"), wantErr: errors.New("command read failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			mock.ExpectBegin()
			commandQuery := mock.ExpectQuery(`select status,lease_receipt_hash from element_capture_commands`).
				WithArgs(sessionID, executorID)
			switch {
			case tt.dbErr != nil:
				commandQuery.WillReturnError(tt.dbErr)
			case tt.commandRow:
				commandQuery.WillReturnRows(sqlmock.NewRows([]string{"status", "lease_receipt_hash"}).
					AddRow(tt.status, captureReceiptHash(validReceipt)))
			default:
				commandQuery.WillReturnRows(sqlmock.NewRows([]string{"status", "lease_receipt_hash"}))
			}

			shouldUpdateSession := tt.dbErr == nil && tt.commandRow &&
				((tt.status == "leased" && tt.receipt == validReceipt) ||
					(tt.status == "acked" && (tt.receipt == "" || tt.receipt == validReceipt)))
			if shouldUpdateSession {
				mock.ExpectQuery(`update element_capture_sessions set status='active'`).
					WithArgs(sessionID, executorID, browserContext, currentURL, tokenHash).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(sessionID))
			}
			if shouldUpdateSession && tt.status == "leased" {
				mock.ExpectExec(`update element_capture_commands set status='acked'`).
					WithArgs(sessionID, executorID, captureReceiptHash(validReceipt)).
					WillReturnResult(sqlmock.NewResult(0, tt.ackRows))
			}
			if tt.wantOK {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}

			ok, gotErr := NewElementCaptureRepository(db).HeartbeatAndAckStart(
				context.Background(), sessionID, executorID, tokenHash, browserContext, currentURL, tt.receipt,
			)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v，期望 %v，err=%v", ok, tt.wantOK, gotErr)
			}
			if tt.wantErr == nil && gotErr != nil {
				t.Fatalf("返回意外错误：%v", gotErr)
			}
			if tt.wantErr != nil {
				if tt.dbErr != nil {
					if gotErr == nil || gotErr.Error() != tt.dbErr.Error() {
						t.Fatalf("错误=%v，期望 %v", gotErr, tt.dbErr)
					}
				} else if !errors.Is(gotErr, tt.wantErr) {
					t.Fatalf("错误=%v，期望类型 %v", gotErr, tt.wantErr)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestElementCaptureRepositoryClosesCommandRowsOnDecodeFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`update element_capture_commands set status='expired'`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`delete from element_capture_commands`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`for update skip locked`).
		WithArgs("exec-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "session_id", "command_type", "payload"}).
			AddRow(7, "session-1", "start", []byte(`{`)).
			AddRow(8, "session-2", "stop", []byte(`{"sessionId":"session-2","type":"stop"}`))).
		RowsWillBeClosed()
	mock.ExpectRollback()

	_, err = NewElementCaptureRepository(db).ClaimCommands(context.Background(), "exec-1", 1)
	if err == nil {
		t.Fatal("畸形命令 payload 应返回错误")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryClosesExpiredSessionRowsOnScanFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`update element_capture_sessions set status='expired'`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "executor_id"}).
			AddRow("session-1", nil).
			AddRow("session-2", "exec-1")).
		RowsWillBeClosed()
	mock.ExpectRollback()

	err = NewElementCaptureRepository(db).ExpireSessions(context.Background(), sql.NullTime{}.Time)
	if err == nil {
		t.Fatal("无法扫描 executor_id 时应返回错误")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryAuthorizesCommandWithSuppliedFallbackWhenSettingIsAbsent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`executor_token=''[\s\S]*coalesce\(nullif\(\(select value from platform_settings where key='executor_shared_token'\),''\),\$3\)`).
		WithArgs("exec-1", "fallback-token", "fallback-token").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	allowed, err := NewElementCaptureRepository(db).AuthorizeCommandExecutor(
		context.Background(), "exec-1", "fallback-token", "fallback-token",
	)
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryCleanupEnforcesAttemptCapAndRetention(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(`update element_capture_commands set status='expired',lease_until=null,lease_receipt_hash=''[\s\S]*attempts >= 5 and lease_until <= \$1::timestamptz`).
		WithArgs(now).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`delete from element_capture_commands where status in \('acked','expired'\)[\s\S]*created_at < \(\$1::timestamptz - interval '24 hours'\)`).
		WithArgs(now).
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := NewElementCaptureRepository(db).CleanupCommands(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestElementCaptureRepositoryAckCommandMatrix(t *testing.T) {
	tests := []struct {
		name    string
		rows    int64
		dbErr   error
		wantAck bool
	}{
		{name: "leased valid receipt", rows: 1, wantAck: true},
		{name: "wrong receipt", rows: 0},
		{name: "expired lease", rows: 0},
		{name: "queued command", rows: 0},
		{name: "start command", rows: 0},
		{name: "database error", dbErr: errors.New("ack failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			expectation := mock.ExpectExec(`update element_capture_commands set status='acked'[\s\S]*command_type<>'start'[\s\S]*status='leased'[\s\S]*lease_until>now\(\)[\s\S]*lease_receipt_hash=\$3`).
				WithArgs(int64(7), "exec-1", captureReceiptHash("receipt"))
			if tt.dbErr != nil {
				expectation.WillReturnError(tt.dbErr)
			} else {
				expectation.WillReturnResult(sqlmock.NewResult(0, tt.rows))
			}

			acked, gotErr := NewElementCaptureRepository(db).AckCommand(context.Background(), "exec-1", 7, "receipt")
			if acked != tt.wantAck {
				t.Fatalf("acked=%v，期望 %v", acked, tt.wantAck)
			}
			if tt.dbErr == nil && gotErr != nil {
				t.Fatalf("返回意外错误：%v", gotErr)
			}
			if tt.dbErr != nil && (gotErr == nil || gotErr.Error() != tt.dbErr.Error()) {
				t.Fatalf("错误=%v，期望 %v", gotErr, tt.dbErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestElementCaptureRepositoryPageAccessibleOwnerDenyAndAdminMatrix(t *testing.T) {
	tests := []struct {
		name    string
		actor   string
		allowed bool
	}{
		{name: "owner allow", actor: "owner", allowed: true},
		{name: "other owner deny", actor: "other", allowed: false},
		{name: "admin allow", actor: "admin", allowed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			mock.ExpectQuery(`from ui_assets p[\s\S]*p\.created_by=\$2[\s\S]*ro\.code='admin'`).
				WithArgs(int64(8), tt.actor).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tt.allowed))
			allowed, err := NewElementCaptureRepository(db).PageAccessible(context.Background(), 8, tt.actor)
			if err != nil || allowed != tt.allowed {
				t.Fatalf("allowed=%v，期望 %v，err=%v", allowed, tt.allowed, err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestElementCaptureRepositoryRollbackVersionFailureRollsBack(t *testing.T) {
	versionFailure := errors.New("version write failed")
	auditFailure := errors.New("audit write failed")
	tests := []struct {
		name       string
		versionErr error
		auditErr   error
		wantErr    error
	}{
		{name: "version failure", versionErr: versionFailure, wantErr: versionFailure},
		{name: "audit failure", auditErr: auditFailure, wantErr: auditFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			expectRollbackVersionPrelude(mock)
			versionInsert := mock.ExpectQuery(`insert into page_element_versions`).
				WithArgs(int64(22), 4, sqlmock.AnyArg(), "回滚至版本 1", "admin")
			if tt.versionErr != nil {
				versionInsert.WillReturnError(tt.versionErr)
			} else {
				versionInsert.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(12, time.Now()))
				mock.ExpectExec(`insert into operation_logs`).
					WithArgs("admin", "22:1").
					WillReturnError(tt.auditErr)
			}
			mock.ExpectRollback()

			_, gotErr := NewElementCaptureRepository(db).RollbackVersion(context.Background(), "admin", 22, 1)
			if gotErr == nil || gotErr.Error() != tt.wantErr.Error() {
				t.Fatalf("错误=%v，期望 %v", gotErr, tt.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func expectRollbackVersionPrelude(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectQuery(`select e\.current_version from page_elements e join ui_assets p[\s\S]*for update of e,p`).
		WithArgs(int64(22), "admin").
		WillReturnRows(sqlmock.NewRows([]string{"current_version"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta("select snapshot from page_element_versions where page_element_id=$1 and version=$2")).
		WithArgs(int64(22), 1).
		WillReturnRows(sqlmock.NewRows([]string{"snapshot"}).AddRow([]byte(
			`{"id":22,"version":1,"name":"submit","fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","captureUrl":"https://example.test","tagName":"button","accessibleName":"Submit","locators":[{"type":"testid","value":"submit","index":"","score":95,"unique":true}],"qualityScore":95}`,
		)))
	mock.ExpectExec(`update page_elements set name=\$1`).
		WithArgs(
			"submit", "testid", "submit", "", "", "", "", "", "", "",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"rollback", "https://example.test", "button", "Submit", 95.0, "admin", 4, int64(22),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
}
