package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"regexp"
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
		`create table page_elements (
			id bigserial primary key,
			page_id bigint not null,
			name text not null,
			fingerprint text not null default '',
			deleted_at timestamptz
		)`,
		`create unique index uq_page_elements_active_fingerprint on page_elements(page_id,fingerprint) where deleted_at is null and fingerprint <> ''`,
		`create table element_capture_sessions (
			id text primary key,
			executor_id text not null,
			status text not null
		)`,
		`create table element_capture_candidates (
			id text primary key,
			cursor_id bigserial unique,
			session_id text not null,
			expires_at timestamptz not null
		)`,
		`create unique index uq_element_capture_candidates_cursor_id on element_capture_candidates(cursor_id)`,
		`insert into element_capture_sessions(id,executor_id,status) values('legacy-session','legacy-executor','completed')`,
		`insert into element_capture_candidates(id,session_id,expires_at) values('legacy-candidate','legacy-session',now()+interval '1 hour')`,
	}
	for _, statement := range legacyDDL {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("建立旧版结构失败：%v", err)
		}
	}

	for round := 1; round <= 2; round++ {
		for index, statement := range elementCaptureMigrationStatements() {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				t.Fatalf("第 %d 轮迁移第 %d 条失败：%v", round, index+1, err)
			}
		}
	}

	var cursorID int64
	if err := db.QueryRowContext(ctx, `select cursor_id from element_capture_candidates where id='legacy-candidate'`).Scan(&cursorID); err != nil {
		t.Fatalf("读取升级后的候选游标：%v", err)
	}
	if cursorID <= 0 {
		t.Fatalf("旧候选未回填正数游标：%d", cursorID)
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
		"uq_element_capture_sessions_active_executor",
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
