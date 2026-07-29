package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestElementCaptureMigrationUpgradesAndIsIdempotentOnPostgreSQL(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过真实 PostgreSQL 迁移测试")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("打开 PostgreSQL：%v", err)
	}
	schema := ""
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if schema != "" {
			if _, err := adminDB.ExecContext(cleanupCtx, `drop schema if exists `+schema+` cascade`); err != nil {
				t.Errorf("清理临时 schema %s：%v", schema, err)
			}
			var count int
			if err := adminDB.QueryRowContext(cleanupCtx, `select count(*) from pg_namespace where nspname=$1`, schema).Scan(&count); err != nil || count != 0 {
				t.Errorf("临时 schema 清理残留：count=%d err=%v", count, err)
			}
		}
		if err := adminDB.Close(); err != nil {
			t.Errorf("关闭 PostgreSQL 管理连接：%v", err)
		}
	})
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("连接 PostgreSQL：%v", err)
	}

	schema = fmt.Sprintf("element_capture_migration_%d", time.Now().UnixNano())
	if !regexp.MustCompile(`^element_capture_migration_[0-9]+$`).MatchString(schema) {
		t.Fatalf("临时 schema 名称无效：%q", schema)
	}
	if _, err := adminDB.ExecContext(ctx, `create schema `+schema); err != nil {
		t.Fatalf("创建临时 schema：%v", err)
	}

	scopedDSN, err := postgresDSNWithSearchPath(dsn, schema)
	if err != nil {
		t.Fatalf("设置迁移 search_path：%v", err)
	}
	db, err := sql.Open("pgx", scopedDSN)
	if err != nil {
		t.Fatalf("打开临时 schema 连接：%v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭临时 schema 连接：%v", err)
		}
	}()
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("连接临时 schema：%v", err)
	}

	legacyDDL := []string{
		`create table roles (id bigserial primary key)`,
		`create table users (id bigserial primary key)`,
		`create table executors (executor_id text primary key)`,
		`create table ui_assets (id bigserial primary key)`,
		`create table page_elements (
			id bigserial primary key,
			page_id bigint not null references ui_assets(id),
			name text not null,
			fingerprint text not null default '',
			deleted_at timestamptz
		)`,
		`create unique index uq_page_elements_active_fingerprint on page_elements(page_id,fingerprint) where deleted_at is null and fingerprint <> ''`,
		`create table element_capture_sessions (
			id text primary key,
			page_id bigint not null references ui_assets(id),
			executor_id text not null references executors(executor_id),
			browser_context_id text not null default '',
			created_by text not null,
			status text not null default 'active',
			mode text not null default 'pick',
			current_url text not null default '',
			candidate_count int not null default 0,
			expires_at timestamptz not null,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)`,
		`create table element_capture_candidates (
			id text primary key,
			cursor_id bigserial unique,
			session_id text not null references element_capture_sessions(id) on delete cascade,
			fingerprint text not null default '',
			tag_name text not null default '',
			accessible_name text not null default '',
			locators jsonb not null default '[]'::jsonb,
			quality_score double precision not null default 0,
			status text not null default 'pending',
			expires_at timestamptz not null
		)`,
		`create unique index uq_element_capture_candidates_cursor_id on element_capture_candidates(cursor_id)`,
		`create table element_capture_commands (
			id bigserial primary key,
			session_id text not null references element_capture_sessions(id) on delete cascade,
			executor_id text not null references executors(executor_id),
			command_type text not null,
			payload jsonb not null default '{}'::jsonb,
			status text not null default 'pending',
			expires_at timestamptz not null,
			claimed_at timestamptz,
			created_at timestamptz not null default now()
		)`,
		`create index idx_element_capture_commands_pending on element_capture_commands(executor_id,id) where status='pending'`,
		`insert into roles(id) values(1)`,
		`insert into users(id) values(1)`,
		`insert into executors(executor_id) values('legacy-executor')`,
		`insert into ui_assets(id) values(8)`,
		`insert into element_capture_sessions(id,page_id,executor_id,created_by,status,expires_at)
		 values
		 ('legacy-pending',8,'legacy-executor','owner','completed',now()+interval '1 hour'),
		 ('legacy-claimed',8,'legacy-executor','owner','completed',now()+interval '1 hour'),
		 ('legacy-expired',8,'legacy-executor','owner','completed',now()-interval '1 hour')`,
		`insert into element_capture_commands(session_id,executor_id,command_type,payload,status,expires_at,claimed_at)
		 values
		 ('legacy-pending','legacy-executor','start','{"sessionId":"legacy-pending","type":"start","token":"plain-pending"}','pending',now()+interval '1 hour',null),
		 ('legacy-claimed','legacy-executor','start','{"sessionId":"legacy-claimed","type":"start","token":"plain-claimed"}','claimed',now()+interval '1 hour',now()),
		 ('legacy-expired','legacy-executor','start','{"sessionId":"legacy-expired","type":"start","token":"plain-expired"}','claimed',now()-interval '1 hour',now()-interval '2 hours')`,
		`insert into element_capture_candidates(id,session_id,expires_at) values('legacy-candidate','legacy-pending',now()+interval '1 hour')`,
	}
	for _, statement := range legacyDDL {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("建立旧版结构失败：%v", err)
		}
	}

	for round := 1; round <= 2; round++ {
		if err := (&app{db: db}).migrate(ctx); err != nil {
			t.Fatalf("第 %d 轮 migrate 失败：%v", round, err)
		}
	}

	var cursorID int64
	var clientCaptureID string
	if err := db.QueryRowContext(ctx, `select cursor_id,client_capture_id from element_capture_candidates where id='legacy-candidate'`).Scan(&cursorID, &clientCaptureID); err != nil {
		t.Fatalf("读取升级后的候选游标：%v", err)
	}
	if cursorID <= 0 || clientCaptureID != "legacy-candidate" {
		t.Fatalf("旧候选幂等字段回填错误：cursor=%d clientCaptureId=%q", cursorID, clientCaptureID)
	}

	wantCommandStatuses := map[string]string{
		"legacy-pending": "queued",
		"legacy-claimed": "queued",
		"legacy-expired": "expired",
	}
	for sessionID, wantStatus := range wantCommandStatuses {
		var status string
		var payloadHasToken bool
		var payloadText, receiptHash string
		if err := db.QueryRowContext(ctx, `
			select status,payload ? 'token',payload::text,lease_receipt_hash
			from element_capture_commands where session_id=$1
		`, sessionID).Scan(&status, &payloadHasToken, &payloadText, &receiptHash); err != nil {
			t.Fatalf("读取迁移命令 %s：%v", sessionID, err)
		}
		if status != wantStatus {
			t.Errorf("命令 %s 状态=%q，期望 %q", sessionID, status, wantStatus)
		}
		if payloadHasToken || strings.Contains(payloadText, "plain-") || receiptHash != "" {
			t.Errorf("命令 %s 仍包含明文或遗留回执：payload=%s receiptHash=%q", sessionID, payloadText, receiptHash)
		}
	}

	var receiptColumnCount int
	if err := db.QueryRowContext(ctx, `
		select count(*) from information_schema.columns
		where table_schema=current_schema() and table_name='element_capture_commands'
		  and column_name in ('lease_until','attempts','acked_at','lease_receipt_hash')
	`).Scan(&receiptColumnCount); err != nil {
		t.Fatalf("检查命令租约列：%v", err)
	}
	if receiptColumnCount != 4 {
		t.Fatalf("命令租约列数量=%d，期望 4", receiptColumnCount)
	}

	var oldPendingIndexCount int
	if err := db.QueryRowContext(ctx, `
		select count(*) from pg_indexes
		where schemaname=current_schema() and indexname='idx_element_capture_commands_pending'
	`).Scan(&oldPendingIndexCount); err != nil {
		t.Fatalf("检查旧 pending 索引：%v", err)
	}
	if oldPendingIndexCount != 0 {
		t.Fatalf("旧 pending 索引仍存在：%d", oldPendingIndexCount)
	}

	var claimIndexCount int
	var claimIndexDefinition string
	if err := db.QueryRowContext(ctx, `
		select count(*),coalesce(max(indexdef),'') from pg_indexes
		where schemaname=current_schema() and indexname='idx_element_capture_commands_claim'
	`).Scan(&claimIndexCount, &claimIndexDefinition); err != nil {
		t.Fatalf("检查 claim 索引：%v", err)
	}
	if claimIndexCount != 1 || !strings.Contains(claimIndexDefinition, "queued") || !strings.Contains(claimIndexDefinition, "leased") {
		t.Fatalf("claim 索引状态错误：count=%d definition=%q", claimIndexCount, claimIndexDefinition)
	}

	var legacyFingerprintIndexCount int
	if err := db.QueryRowContext(ctx, `
		select count(*) from pg_class i
		join pg_namespace n on n.oid=i.relnamespace
		where n.nspname=current_schema() and i.relkind='i' and i.relname='uq_page_elements_active_fingerprint'
	`).Scan(&legacyFingerprintIndexCount); err != nil {
		t.Fatalf("检查旧 fingerprint 索引：%v", err)
	}
	if legacyFingerprintIndexCount != 0 {
		t.Fatalf("旧 fingerprint 唯一索引仍存在：%d", legacyFingerprintIndexCount)
	}

	var automaticCursorConstraintCount int
	if err := db.QueryRowContext(ctx, `
		select count(*) from pg_constraint c
		join pg_class r on r.oid=c.conrelid
		join pg_namespace n on n.oid=r.relnamespace
		where n.nspname=current_schema() and r.relname='element_capture_candidates'
		  and c.conname='element_capture_candidates_cursor_id_key'
	`).Scan(&automaticCursorConstraintCount); err != nil {
		t.Fatalf("检查旧 cursor 自动约束：%v", err)
	}
	if automaticCursorConstraintCount != 0 {
		t.Fatalf("旧 cursor 自动唯一约束仍存在：%d", automaticCursorConstraintCount)
	}

	var namedCursorIndexCount int
	var namedCursorIndexUnique bool
	if err := db.QueryRowContext(ctx, `
		select count(*),coalesce(bool_and(x.indisunique),false)
		from pg_index x
		join pg_class i on i.oid=x.indexrelid
		join pg_namespace n on n.oid=i.relnamespace
		where n.nspname=current_schema() and i.relname='uq_element_capture_candidates_cursor_id'
	`).Scan(&namedCursorIndexCount, &namedCursorIndexUnique); err != nil {
		t.Fatalf("检查命名 cursor 唯一索引：%v", err)
	}
	if namedCursorIndexCount != 1 || !namedCursorIndexUnique {
		t.Fatalf("命名 cursor 唯一索引状态错误：count=%d unique=%v", namedCursorIndexCount, namedCursorIndexUnique)
	}

	for _, indexName := range []string{
		"uq_page_elements_active_name",
		"uq_element_capture_candidates_cursor_id",
		"uq_element_capture_candidates_session_client_capture",
		"uq_element_capture_sessions_active_executor",
		"idx_element_capture_commands_claim",
	} {
		var count int
		if err := db.QueryRowContext(ctx, `select count(*) from pg_indexes where schemaname=current_schema() and indexname=$1`, indexName).Scan(&count); err != nil {
			t.Fatalf("检查索引 %s：%v", indexName, err)
		}
		if count != 1 {
			t.Fatalf("索引 %s 应且仅应存在一次，实际 %d", indexName, count)
		}
	}

	if _, err := db.ExecContext(ctx, `insert into page_elements(page_id,name,fingerprint) values(8,'Submit','same-fingerprint')`); err != nil {
		t.Fatalf("插入首个活动元素：%v", err)
	}
	if _, err := db.ExecContext(ctx, `insert into page_elements(page_id,name,fingerprint) values(8,'submit','other-fingerprint')`); err == nil {
		t.Fatal("同页活动名称唯一索引未生效")
	}
	if _, err := db.ExecContext(ctx, `insert into page_elements(page_id,name,fingerprint) values(8,'Copy','same-fingerprint')`); err != nil {
		t.Fatalf("显式 create 所需的重复 fingerprint 能力被错误阻止：%v", err)
	}
}

func postgresDSNWithSearchPath(dsn, schema string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
