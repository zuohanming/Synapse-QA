package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestElementCaptureLeaseAndExpiryUseRealPostgreSQLWithoutBusyConnection(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = localPostgresDSN(t)
	}
	if dsn == "" {
		t.Skip("未配置 TEST_DATABASE_URL 或 backend/.env.local 数据库连接")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("element_capture_lease_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(ctx, `create schema `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.ExecContext(context.Background(), `drop schema if exists `+schema+` cascade`) }()
	scoped, err := leaseTestDSN(dsn, schema)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		`create table element_capture_sessions(id text primary key,executor_id text not null,token_hash text not null default '',status text not null,updated_at timestamptz not null default now(),expires_at timestamptz not null)`,
		`create table element_capture_commands(id bigserial primary key,session_id text not null,executor_id text not null,command_type text not null,payload jsonb not null,status text not null,expires_at timestamptz not null,lease_until timestamptz,attempts int not null default 0,lease_receipt_hash text not null default '',acked_at timestamptz,created_at timestamptz not null default now(),claimed_at timestamptz)`,
	} {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	_, err = db.ExecContext(ctx, `insert into element_capture_sessions(id,executor_id,status,expires_at) values('start','exec-1','starting',$1),('expired','exec-1','active',$2)`, now.Add(time.Hour), now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `insert into element_capture_commands(session_id,executor_id,command_type,payload,status,expires_at) values('start','exec-1','start','{"sessionId":"start","type":"start","token":"legacy-secret"}','queued',$1)`, now.Add(time.Hour))
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
	if err = repo.ExpireSessions(ctx, now); err != nil {
		t.Fatalf("ExpireSessions: %v", err)
	}
	var commands int
	if err = db.QueryRowContext(ctx, `select count(*) from element_capture_commands where session_id='expired' and command_type='expire'`).Scan(&commands); err != nil || commands != 1 {
		t.Fatalf("expired commands=%d err=%v", commands, err)
	}
}

func localPostgresDSN(t *testing.T) string {
	raw, err := os.ReadFile("../../.env.local")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && key == "DATABASE_URL" {
			return strings.Trim(strings.TrimSpace(value), "\"'")
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
