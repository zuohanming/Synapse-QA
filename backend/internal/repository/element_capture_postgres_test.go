package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"synapseqa/backend/internal/model"
)

func TestElementCaptureLeaseAndExpiryUseRealPostgreSQLWithoutBusyConnection(t *testing.T) {
	db, ctx := capturePostgresTestDB(t, "element_capture_lease")
	for _, statement := range []string{
		`create table element_capture_sessions(id text primary key,executor_id text not null,token_hash text not null default '',status text not null,browser_context_id text not null default '',current_url text not null default '',last_heartbeat_at timestamptz not null default now(),interrupted_at timestamptz,recovery_expires_at timestamptz,updated_at timestamptz not null default now(),expires_at timestamptz not null)`,
		`create table element_capture_commands(id bigserial primary key,session_id text not null,executor_id text not null,command_type text not null,payload jsonb not null,status text not null,expires_at timestamptz not null,lease_until timestamptz,attempts int not null default 0,lease_receipt_hash text not null default '',acked_at timestamptz,created_at timestamptz not null default now(),claimed_at timestamptz)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	_, err := db.ExecContext(ctx, `insert into element_capture_sessions(id,executor_id,status,expires_at) values('start','exec-1','starting',$1),('expired','exec-1','active',$2)`, now.Add(time.Hour), now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `insert into element_capture_commands(session_id,executor_id,command_type,payload,status,expires_at) values('start','exec-1','start','{"sessionId":"start","type":"start"}','queued',$1)`, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewElementCaptureRepository(db)
	items, err := repo.ClaimCommands(ctx, "exec-1", 1)
	if err != nil || len(items) != 1 || items[0].Token == "" || items[0].Receipt == "" {
		t.Fatalf("claim=%+v err=%v", items, err)
	}
	var payload string
	var hash string
	if err = db.QueryRowContext(ctx, `select payload::text from element_capture_commands where id=$1`, items[0].ID).Scan(&payload); err != nil || strings.Contains(payload, "token") {
		t.Fatalf("payload=%s err=%v", payload, err)
	}
	if err = db.QueryRowContext(ctx, `select token_hash from element_capture_sessions where id='start'`).Scan(&hash); err != nil || hash == "" {
		t.Fatalf("hash=%q err=%v", hash, err)
	}
	tokenHash := hash
	if ok, err := repo.HeartbeatAndAckStart(ctx, "start", "exec-1", tokenHash, "ctx-1", "https://example.test", items[0].Receipt); err != nil || !ok {
		t.Fatalf("atomic heartbeat ack=%v err=%v", ok, err)
	}
	var commandStatus string
	if err = db.QueryRowContext(ctx, `select status from element_capture_commands where id=$1`, items[0].ID).Scan(&commandStatus); err != nil || commandStatus != "acked" {
		t.Fatalf("start status=%s err=%v", commandStatus, err)
	}
	if err = repo.ExpireSessions(ctx, now); err != nil {
		t.Fatalf("ExpireSessions: %v", err)
	}
	var commands int
	if err = db.QueryRowContext(ctx, `select count(*) from element_capture_commands where session_id='expired' and command_type='expire'`).Scan(&commands); err != nil || commands != 1 {
		t.Fatalf("expired commands=%d err=%v", commands, err)
	}
}

func TestElementCaptureRepositoryStateAuthorizationAndCleanupOnPostgreSQL(t *testing.T) {
	db, ctx := capturePostgresTestDB(t, "element_capture_repository")
	for _, statement := range []string{
		`create table roles(id bigserial primary key,code text not null)`,
		`create table users(id bigserial primary key,username text not null,role_id bigint references roles(id),deleted_at timestamptz)`,
		`create table executors(executor_id text primary key,executor_token text not null default '')`,
		`create table platform_settings(key text primary key,value text not null)`,
		`create table ui_assets(id bigserial primary key,asset_type text not null,created_by text not null,deleted_at timestamptz)`,
		`create table element_capture_sessions(id text primary key,executor_id text not null,token_hash text not null default '',status text not null,browser_context_id text not null default '',current_url text not null default '',last_heartbeat_at timestamptz not null default now(),interrupted_at timestamptz,recovery_expires_at timestamptz,updated_at timestamptz not null default now(),expires_at timestamptz not null)`,
		`create table element_capture_commands(id bigserial primary key,session_id text not null,executor_id text not null,command_type text not null,payload jsonb not null default '{}'::jsonb,status text not null,expires_at timestamptz not null,lease_until timestamptz,attempts int not null default 0,lease_receipt_hash text not null default '',acked_at timestamptz,created_at timestamptz not null default now(),claimed_at timestamptz)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		insert into roles(id,code) values(1,'admin');
		insert into users(username,role_id) values('admin',1);
		insert into executors(executor_id) values('exec-1'),('exec-fallback');
		insert into ui_assets(id,asset_type,created_by) values(8,'page','owner')
	`); err != nil {
		t.Fatal(err)
	}

	repo := NewElementCaptureRepository(db)
	for actor, want := range map[string]bool{"owner": true, "other": false, "admin": true} {
		allowed, err := repo.PageAccessible(ctx, 8, actor)
		if err != nil || allowed != want {
			t.Fatalf("PageAccessible actor=%s allowed=%v want=%v err=%v", actor, allowed, want, err)
		}
	}
	allowed, err := repo.AuthorizeCommandExecutor(ctx, "exec-fallback", "fallback-token", "fallback-token")
	if err != nil || !allowed {
		t.Fatalf("fallback authorization allowed=%v err=%v", allowed, err)
	}

	now := time.Now().UTC()
	sessionIDs := []string{"leased", "wrong-receipt", "expired-lease", "queued", "missing-command", "acked", "db-error"}
	for _, sessionID := range sessionIDs {
		if _, err := db.ExecContext(ctx, `insert into element_capture_sessions(id,executor_id,token_hash,status,expires_at) values($1,'exec-1','token-hash','starting',$2)`, sessionID, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range []struct {
		sessionID string
		status    string
		receipt   string
		lease     time.Time
	}{
		{"leased", "leased", "leased-receipt", now.Add(time.Minute)},
		{"wrong-receipt", "leased", "right-receipt", now.Add(time.Minute)},
		{"expired-lease", "leased", "expired-receipt", now.Add(-time.Minute)},
		{"queued", "queued", "", now.Add(time.Minute)},
		{"acked", "acked", "", now.Add(time.Minute)},
		{"db-error", "leased", "db-error-receipt", now.Add(time.Minute)},
	} {
		if _, err := db.ExecContext(ctx, `insert into element_capture_commands(session_id,executor_id,command_type,status,expires_at,lease_until,lease_receipt_hash) values($1,'exec-1','start',$2,$3,$4,$5)`,
			command.sessionID, command.status, now.Add(time.Hour), command.lease, captureReceiptHash(command.receipt)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		create function reject_db_error_ack() returns trigger language plpgsql as $$
		begin
			if old.session_id='db-error' and new.status='acked' then
				raise exception 'forced ack failure';
			end if;
			return new;
		end $$;
		create trigger reject_db_error_ack before update on element_capture_commands
		for each row execute function reject_db_error_ack()
	`); err != nil {
		t.Fatal(err)
	}

	if ok, err := repo.HeartbeatAndAckStart(ctx, "leased", "exec-1", "token-hash", "ctx-1", "https://example.test", "leased-receipt"); err != nil || !ok {
		t.Fatalf("leased success ok=%v err=%v", ok, err)
	}
	assertCaptureSessionAndCommandState(t, ctx, db, "leased", "active", "acked")

	for _, failure := range []struct {
		name        string
		sessionID   string
		receipt     string
		wantCommand string
	}{
		{"wrong receipt", "wrong-receipt", "wrong", "leased"},
		{"expired lease", "expired-lease", "expired-receipt", "leased"},
		{"queued", "queued", "queued-receipt", "queued"},
		{"missing command", "missing-command", "missing-receipt", ""},
	} {
		t.Run(failure.name, func(t *testing.T) {
			ok, err := repo.HeartbeatAndAckStart(ctx, failure.sessionID, "exec-1", "token-hash", "ctx-1", "https://example.test", failure.receipt)
			if ok || !errors.Is(err, model.ErrConflict) {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
			assertCaptureSessionAndCommandState(t, ctx, db, failure.sessionID, "starting", failure.wantCommand)
		})
	}

	if ok, err := repo.HeartbeatAndAckStart(ctx, "acked", "exec-1", "token-hash", "ctx-1", "https://example.test", ""); err != nil || !ok {
		t.Fatalf("acked follow-up ok=%v err=%v", ok, err)
	}
	assertCaptureSessionAndCommandState(t, ctx, db, "acked", "active", "acked")

	if ok, err := repo.HeartbeatAndAckStart(ctx, "db-error", "exec-1", "token-hash", "ctx-1", "https://example.test", "db-error-receipt"); err == nil || ok {
		t.Fatalf("database error ok=%v err=%v", ok, err)
	}
	assertCaptureSessionAndCommandState(t, ctx, db, "db-error", "starting", "leased")

	ackCases := []struct {
		sessionID string
		kind      string
		status    string
		lease     time.Time
		receipt   string
		want      bool
	}{
		{"ack-valid", "stop", "leased", now.Add(time.Minute), "ack-receipt", true},
		{"ack-wrong", "stop", "leased", now.Add(time.Minute), "stored-receipt", false},
		{"ack-expired", "stop", "leased", now.Add(-time.Minute), "ack-receipt", false},
		{"ack-queued", "stop", "queued", now.Add(time.Minute), "ack-receipt", false},
		{"ack-start", "start", "leased", now.Add(time.Minute), "ack-receipt", false},
	}
	for _, ackCase := range ackCases {
		var commandID int64
		storedReceipt := ackCase.receipt
		suppliedReceipt := ackCase.receipt
		if ackCase.sessionID == "ack-wrong" {
			suppliedReceipt = "wrong-receipt"
		}
		if err := db.QueryRowContext(ctx, `insert into element_capture_commands(session_id,executor_id,command_type,status,expires_at,lease_until,lease_receipt_hash) values($1,'exec-1',$2,$3,$4,$5,$6) returning id`,
			ackCase.sessionID, ackCase.kind, ackCase.status, now.Add(time.Hour), ackCase.lease, captureReceiptHash(storedReceipt)).Scan(&commandID); err != nil {
			t.Fatal(err)
		}
		acked, err := repo.AckCommand(ctx, "exec-1", commandID, suppliedReceipt)
		if err != nil || acked != ackCase.want {
			t.Fatalf("AckCommand %s acked=%v want=%v err=%v", ackCase.sessionID, acked, ackCase.want, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
		insert into element_capture_commands(session_id,executor_id,command_type,status,expires_at,lease_until,attempts,created_at)
		values
		('cleanup-expiry','exec-1','stop','queued',$1,$2,0,$3),
		('cleanup-cap','exec-1','stop','leased',$4,$2,5,$3),
		('cleanup-under-cap','exec-1','stop','leased',$4,$2,4,$3),
		('cleanup-old-acked','exec-1','stop','acked',$4,$2,1,$5),
		('cleanup-old-expired','exec-1','stop','expired',$4,$2,1,$5)
	`, now.Add(-time.Minute), now.Add(-time.Minute), now, now.Add(time.Hour), now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.CleanupCommands(ctx, now); err != nil {
		t.Fatal(err)
	}
	var expiredCount, underCapCount, retainedOldCount int
	if err := db.QueryRowContext(ctx, `select count(*) from element_capture_commands where session_id in ('cleanup-expiry','cleanup-cap') and status='expired'`).Scan(&expiredCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from element_capture_commands where session_id='cleanup-under-cap' and status='leased'`).Scan(&underCapCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from element_capture_commands where session_id in ('cleanup-old-acked','cleanup-old-expired')`).Scan(&retainedOldCount); err != nil {
		t.Fatal(err)
	}
	if expiredCount != 2 || underCapCount != 1 || retainedOldCount != 0 {
		t.Fatalf("cleanup expired=%d underCap=%d retainedOld=%d", expiredCount, underCapCount, retainedOldCount)
	}
}

func assertCaptureSessionAndCommandState(t *testing.T, ctx context.Context, db *sql.DB, sessionID, wantSession, wantCommand string) {
	t.Helper()
	var sessionStatus string
	if err := db.QueryRowContext(ctx, `select status from element_capture_sessions where id=$1`, sessionID).Scan(&sessionStatus); err != nil {
		t.Fatal(err)
	}
	if sessionStatus != wantSession {
		t.Fatalf("session %s status=%s want=%s", sessionID, sessionStatus, wantSession)
	}
	if wantCommand == "" {
		return
	}
	var commandStatus string
	if err := db.QueryRowContext(ctx, `select status from element_capture_commands where session_id=$1 and command_type='start'`, sessionID).Scan(&commandStatus); err != nil {
		t.Fatal(err)
	}
	if commandStatus != wantCommand {
		t.Fatalf("command %s status=%s want=%s", sessionID, commandStatus, wantCommand)
	}
}

func capturePostgresTestDB(t *testing.T, prefix string) (*sql.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = localPostgresDSN(t)
	}
	if dsn == "" {
		t.Skip("未配置 TEST_DATABASE_URL 或 backend/.env.local 数据库连接")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	if _, err = admin.ExecContext(ctx, `create schema `+schema); err != nil {
		t.Fatal(err)
	}
	scoped, err := leaseTestDSN(dsn, schema)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", scoped)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭 scoped PostgreSQL：%v", err)
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(cleanupCtx, `drop schema if exists `+schema+` cascade`); err != nil {
			t.Errorf("清理 schema：%v", err)
		}
		var count int
		if err := admin.QueryRowContext(cleanupCtx, `select count(*) from pg_namespace where nspname=$1`, schema).Scan(&count); err != nil || count != 0 {
			t.Errorf("schema 清理残留 count=%d err=%v", count, err)
		}
		if err := admin.Close(); err != nil {
			t.Errorf("关闭 PostgreSQL 管理连接：%v", err)
		}
	})
	db.SetMaxOpenConns(1)
	return db, ctx
}

func localPostgresDSN(t *testing.T) string {
	for _, path := range []string{"../../.env.local", "../../../.env.local", `F:\Synapse QA\.env.local`} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && key == "DATABASE_URL" {
				return strings.Trim(strings.TrimSpace(value), "\"'")
			}
		}
	}
	return ""
}

func leaseTestDSN(dsn, schema string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
