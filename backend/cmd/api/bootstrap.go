package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// app 保存启动期需要的共享依赖；业务请求已迁移到 internal 分层。
type app struct {
	db        *sql.DB
	jwtSecret []byte
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// migrate 只负责数据库结构初始化，业务逻辑不写在这里。
func (a *app) migrate(ctx context.Context) error {
	statements := []string{
		`create table if not exists roles (
			id bigserial primary key,
			name text not null unique,
			code text not null unique,
			description text not null default '',
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			created_at timestamptz not null default now()
		)`,
		`create table if not exists users (
			id bigserial primary key,
			username text not null unique,
			password_hash text not null,
			display_name text not null,
			email text not null default '',
			status text not null default 'active',
			mcp_api_key text not null default '',
			last_login_ip text not null default '',
			last_login_at timestamptz,
			deleted_at timestamptz,
			role_id bigint references roles(id),
			created_at timestamptz not null default now()
		)`,
		`create table if not exists system_menus (
			id bigserial primary key,
			parent_id bigint,
			title text not null,
			code text not null unique,
			sort_order int not null default 0,
			created_at timestamptz not null default now()
		)`,
		`create table if not exists dictionaries (
			id bigserial primary key,
			name text not null,
			code text not null,
			item_key text not null,
			item_value text not null,
			enabled boolean not null default true,
			created_at timestamptz not null default now(),
			unique(code, item_key)
		)`,
		`create table if not exists operation_logs (
			id bigserial primary key,
			actor text not null,
			action text not null,
			target text not null,
			ip text not null default '',
			created_at timestamptz not null default now()
		)`,
		`create table if not exists ui_assets (
			id bigserial primary key,
			asset_type text not null,
			name text not null,
			category text not null default '',
			method text not null default '',
			locator text not null default '',
			action text not null default '',
			value text not null default '',
			description text not null default '',
			status text not null default 'active',
			created_by text not null default '',
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			created_at timestamptz not null default now()
		)`,
		`create table if not exists page_elements (
			id bigserial primary key,
			page_id bigint not null references ui_assets(id),
			name text not null,
			type1 text not null,
			locator1 text not null,
			index1 text not null default '',
			type2 text not null default '',
			locator2 text not null default '',
			index2 text not null default '',
			type3 text not null default '',
			locator3 text not null default '',
			index3 text not null default '',
			ai_prompt text not null default '',
			wait_time text not null default '',
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			created_at timestamptz not null default now()
		)`,
		`create table if not exists projects (
			id bigserial primary key,
			name text not null unique,
			status text not null default 'active',
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			created_at timestamptz not null default now()
		)`,
		`create table if not exists products (
			id bigserial primary key,
			project_id bigint not null references projects(id),
			name text not null,
			ui_type text not null default 'WEB',
			api_type text not null default 'WEB',
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			created_at timestamptz not null default now(),
			unique(project_id, name)
		)`,
		`create table if not exists product_modules (
			id bigserial primary key,
			product_id bigint not null references products(id),
			name text not null,
			level1 text not null default '',
			level2 text not null default '',
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			created_at timestamptz not null default now(),
			unique(product_id, level1, level2, name)
		)`,
		`create table if not exists test_objects (
			id bigserial primary key,
			product_id bigint not null references products(id),
			env_name text not null,
			target text not null,
			deploy_env text not null default '生产环境',
			auto_type text not null default '界面自动化',
			owner text not null default '',
			query_enabled boolean not null default true,
			write_enabled boolean not null default false,
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			created_at timestamptz not null default now(),
			unique(product_id, env_name)
		)`,
		`create table if not exists test_cases (
			id bigserial primary key,
			product_id bigint not null references products(id),
			module_id bigint references product_modules(id),
			page_id bigint references ui_assets(id),
			name text not null,
			case_type text not null default 'ui',
			priority text not null default 'P2',
			status text not null default 'draft',
			owner text not null default '',
			tags text not null default '',
			description text not null default '',
			preconditions text not null default '',
			expected_result text not null default '',
			data_enabled boolean not null default false,
			created_by text not null default '',
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			created_at timestamptz not null default now(),
			unique(product_id, name)
		)`,
		`create table if not exists test_case_steps (
			id bigserial primary key,
			case_id bigint not null references test_cases(id),
			step_id bigint not null references ui_assets(id),
			sort_order int not null default 1,
			note text not null default '',
			created_at timestamptz not null default now(),
			unique(case_id, step_id)
		)`,
		`create table if not exists test_case_datasets (
			id bigserial primary key,
			case_id bigint not null references test_cases(id),
			name text not null,
			variables jsonb not null default '{}'::jsonb,
			enabled boolean not null default true,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now(),
			unique(case_id, name)
		)`,
		`create table if not exists executors (
			executor_id text primary key,
			name text not null,
			endpoint text not null default '',
			status text not null default 'registered',
			version text not null default '',
			max_workers int not null default 1,
			running_tasks int not null default 0,
			queued_tasks int not null default 0,
			supported_types text not null default '',
			checks jsonb not null default '{}'::jsonb,
			last_heartbeat_at timestamptz not null default now(),
			updated_at timestamptz not null default now(),
			created_at timestamptz not null default now()
		)`,
		`create table if not exists element_capture_sessions (
			id text primary key,
			page_id bigint not null references ui_assets(id),
			executor_id text not null references executors(executor_id),
			browser_context_id text not null,
			created_by text not null,
			status text not null default 'active',
			mode text not null,
			current_url text not null default '',
			candidate_count int not null default 0,
			expires_at timestamptz not null,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)`,
		`create table if not exists element_capture_candidates (
			id text primary key,
			cursor_id bigserial unique,
			session_id text not null references element_capture_sessions(id) on delete cascade,
			fingerprint text not null,
			tag_name text not null default '',
			accessible_name text not null default '',
			locators jsonb not null default '[]'::jsonb,
			quality_score double precision not null default 0,
			status text not null default 'pending',
			expires_at timestamptz not null,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)`,
		`create table if not exists page_element_versions (
			id bigserial primary key,
			page_element_id bigint not null references page_elements(id),
			version int not null,
			snapshot jsonb not null,
			change_summary text not null default '',
			created_by text not null,
			created_at timestamptz not null default now(),
			unique(page_element_id, version)
		)`,
		`create table if not exists execution_runs (
			id bigserial primary key,
			run_type text not null default 'ui',
			status text not null default 'pending',
			headless boolean not null default true,
			triggered_by text not null default '',
			case_ids bigint[] not null default '{}',
			summary jsonb not null default '{}'::jsonb,
			started_at timestamptz,
			finished_at timestamptz,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)`,
		`create table if not exists execution_tasks (
			id bigserial primary key,
			run_id bigint not null references execution_runs(id),
			task_id text not null unique,
			case_id bigint references test_cases(id),
			executor_id text not null default '',
			task_type text not null default 'ui',
			payload jsonb not null default '{}'::jsonb,
			callback_url text not null default '',
			status text not null default 'queued',
			result jsonb not null default '{}'::jsonb,
			started_at timestamptz,
			finished_at timestamptz,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)`,
		`create table if not exists execution_logs (
			id bigserial primary key,
			task_id bigint not null references execution_tasks(id),
			level text not null default 'info',
			message text not null default '',
			created_at timestamptz not null default now()
		)`,
		`create table if not exists platform_settings (
			key text primary key,
			value text not null,
			updated_at timestamptz not null default now(),
			created_at timestamptz not null default now()
		)`,
		`create table if not exists notifications (
			id bigserial primary key,
			user_id bigint not null references users(id),
			type text not null,
			level text not null default 'info',
			title text not null,
			content text not null default '',
			target_type text not null default '',
			target_id text not null default '',
			target_url text not null default '',
			is_read boolean not null default false,
			read_at timestamptz,
			created_at timestamptz not null default now()
		)`,
		`create table if not exists notification_preferences (
			user_id bigint primary key references users(id),
			execution_success boolean not null default true,
			execution_failure boolean not null default true,
			executor_alert boolean not null default true,
			system_notice boolean not null default true,
			updated_at timestamptz not null default now()
		)`,
		`create table if not exists system_setting_groups (
			group_key text primary key,
			value jsonb not null default '{}'::jsonb,
			revision bigint not null default 1,
			updated_by text not null default '',
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)`,
		`create table if not exists system_setting_history (
			id bigserial primary key,
			group_key text not null,
			revision bigint not null,
			value jsonb not null,
			change_summary text not null default '',
			created_by text not null default '',
			created_at timestamptz not null default now(),
			unique(group_key,revision)
		)`,
		`create index if not exists idx_execution_tasks_run_id on execution_tasks(run_id)`,
		`create index if not exists idx_execution_tasks_task_id on execution_tasks(task_id)`,
		`create index if not exists idx_execution_logs_task_id on execution_logs(task_id)`,
		`create index if not exists idx_execution_runs_status on execution_runs(status)`,
		`create index if not exists idx_notifications_user_created on notifications(user_id, created_at desc)`,
		`alter table execution_runs add column if not exists headless boolean not null default true`,
		`alter table executors add column if not exists executor_token text not null default ''`,
		`alter table roles add column if not exists updated_at timestamptz not null default now()`,
		`alter table roles add column if not exists deleted_at timestamptz`,
		`alter table roles add column if not exists built_in boolean not null default false`,
		`alter table roles add column if not exists status text not null default 'active'`,
		`alter table users add column if not exists auth_version bigint not null default 1`,
		`alter table users add column if not exists must_change_password boolean not null default false`,
		`alter table users add column if not exists mcp_api_key text not null default ''`,
		`alter table users add column if not exists last_login_ip text not null default ''`,
		`alter table users add column if not exists last_login_at timestamptz`,
		`alter table users add column if not exists failed_login_count integer not null default 0`,
		`alter table users add column if not exists locked_until timestamptz`,
		`alter table users add column if not exists deleted_at timestamptz`,
		`alter table notification_preferences add column if not exists use_system_defaults boolean not null default true`,
		`alter table ui_assets add column if not exists deleted_at timestamptz`,
		`alter table page_elements add column if not exists deleted_at timestamptz`,
		`alter table page_elements add column if not exists fingerprint text not null default ''`,
		`alter table page_elements add column if not exists capture_source text not null default ''`,
		`alter table page_elements add column if not exists capture_url text not null default ''`,
		`alter table page_elements add column if not exists tag_name text not null default ''`,
		`alter table page_elements add column if not exists accessible_name text not null default ''`,
		`alter table page_elements add column if not exists quality_score double precision not null default 0`,
		`alter table page_elements add column if not exists captured_by text not null default ''`,
		`alter table page_elements add column if not exists captured_at timestamptz`,
		`alter table page_elements add column if not exists last_verified_at timestamptz`,
		`alter table page_elements add column if not exists verification_status text not null default ''`,
		`alter table page_elements add column if not exists current_version int not null default 1`,
		`create index if not exists idx_element_capture_sessions_expires_at on element_capture_sessions(expires_at)`,
		`create index if not exists idx_element_capture_candidates_expires_at on element_capture_candidates(expires_at)`,
		`alter table projects add column if not exists status text not null default 'active'`,
		`alter table projects add column if not exists updated_at timestamptz not null default now()`,
		`alter table projects add column if not exists deleted_at timestamptz`,
		`alter table products add column if not exists ui_type text not null default 'WEB'`,
		`alter table products add column if not exists api_type text not null default 'WEB'`,
		`alter table products add column if not exists updated_at timestamptz not null default now()`,
		`alter table products add column if not exists deleted_at timestamptz`,
		`alter table product_modules add column if not exists level1 text not null default ''`,
		`alter table product_modules add column if not exists level2 text not null default ''`,
		`alter table product_modules add column if not exists updated_at timestamptz not null default now()`,
		`alter table product_modules add column if not exists deleted_at timestamptz`,
		`alter table test_objects add column if not exists deploy_env text not null default '生产环境'`,
		`alter table test_objects add column if not exists auto_type text not null default '界面自动化'`,
		`alter table test_objects add column if not exists owner text not null default ''`,
		`alter table test_objects add column if not exists query_enabled boolean not null default true`,
		`alter table test_objects add column if not exists write_enabled boolean not null default false`,
		`alter table test_objects add column if not exists updated_at timestamptz not null default now()`,
		`alter table test_objects add column if not exists deleted_at timestamptz`,
		`alter table test_cases add column if not exists module_id bigint references product_modules(id)`,
		`alter table test_cases add column if not exists page_id bigint references ui_assets(id)`,
		`alter table test_cases add column if not exists case_type text not null default 'ui'`,
		`alter table test_cases add column if not exists priority text not null default 'P2'`,
		`alter table test_cases add column if not exists status text not null default 'draft'`,
		`alter table test_cases add column if not exists owner text not null default ''`,
		`alter table test_cases add column if not exists tags text not null default ''`,
		`alter table test_cases add column if not exists preconditions text not null default ''`,
		`alter table test_cases add column if not exists expected_result text not null default ''`,
		`alter table test_cases add column if not exists data_enabled boolean not null default false`,
		`alter table test_cases add column if not exists created_by text not null default ''`,
		`alter table test_cases add column if not exists updated_at timestamptz not null default now()`,
		`alter table test_cases add column if not exists deleted_at timestamptz`,
		`create unique index if not exists uq_product_modules_id_product on product_modules(id, product_id)`,
		`create table if not exists permissions (
			id bigserial primary key,
			code text not null unique,
			name text not null,
			description text not null default '',
			created_at timestamptz not null default now()
		)`,
		`create table if not exists role_permissions (
			role_id bigint not null references roles(id),
			permission_id bigint not null references permissions(id),
			created_at timestamptz not null default now(),
			primary key(role_id, permission_id)
		)`,
		`create table if not exists project_members (
			user_id bigint not null references users(id),
			project_id bigint not null references projects(id),
			role_id bigint not null references roles(id),
			created_at timestamptz not null default now(),
			primary key(user_id, project_id)
		)`,
		`create table if not exists api_interfaces (
			id bigserial primary key,
			product_id bigint not null references products(id),
			module_id bigint,
			name text not null,
			method text not null,
			path text not null,
			normalized_path text not null,
			protocol text not null default 'HTTP',
			endpoint_type text not null default 'WEB',
			lifecycle_status text not null default 'draft',
			timeout_seconds int not null default 30 check(timeout_seconds between 1 and 300),
			follow_redirects boolean not null default true,
			configuration jsonb not null default '{}'::jsonb,
			current_version int not null default 1,
			revision bigint not null default 1,
			last_debug_status text not null default '',
			last_debug_duration_ms bigint,
			last_debug_at timestamptz,
			created_by text not null default '',
			updated_by text not null default '',
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			foreign key(module_id, product_id) references product_modules(id, product_id)
		)`,
		`create unique index if not exists uq_api_interfaces_active_path on api_interfaces(product_id, method, normalized_path) where deleted_at is null`,
		`create table if not exists api_interface_versions (
			id bigserial primary key,
			interface_id bigint not null references api_interfaces(id),
			version int not null,
			snapshot jsonb not null,
			change_summary text not null default '',
			created_by text not null,
			created_at timestamptz not null default now(),
			unique(interface_id, version)
		)`,
		`create table if not exists api_project_headers (
			id bigserial primary key,
			project_id bigint not null references projects(id),
			header_name text not null,
			header_name_normalized text not null,
			header_value text not null default '',
			description text not null default '',
			enabled boolean not null default true,
			sensitive boolean not null default false,
			created_by text not null default '',
			updated_by text not null default '',
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now(),
			deleted_at timestamptz
		)`,
		`create unique index if not exists uq_api_project_headers_active_name on api_project_headers(project_id, header_name_normalized) where deleted_at is null`,
		`create table if not exists api_global_variables (
			id bigserial primary key,
			scope_type text not null check(scope_type in ('system','project','product')),
			project_id bigint references projects(id),
			product_id bigint references products(id),
			env_name text not null default '',
			var_name text not null,
			var_name_normalized text not null,
			value_type text not null default 'string' check(value_type in ('string','number','boolean','json','secret')),
			var_value text not null default '',
			description text not null default '',
			enabled boolean not null default true,
			sensitive boolean not null default false,
			revision bigint not null default 1,
			created_by text not null default '',
			updated_by text not null default '',
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now(),
			deleted_at timestamptz
		)`,
		`create unique index if not exists uq_api_global_variables_active_name
			on api_global_variables(scope_type, coalesce(project_id,0), coalesce(product_id,0), env_name, var_name_normalized)
			where deleted_at is null`,
		`create index if not exists idx_api_global_variables_scope on api_global_variables(scope_type, project_id, product_id, env_name) where deleted_at is null`,
		`create table if not exists api_test_cases (
			id bigserial primary key,
			project_id bigint not null references projects(id),
			product_id bigint not null references products(id),
			module_id bigint references product_modules(id),
			name text not null,
			priority text not null default 'P2',
			status text not null default 'draft',
			owner text not null default '',
			tags text[] not null default '{}',
			draft jsonb not null default '{"steps":[],"datasets":[],"variables":[]}'::jsonb,
			revision bigint not null default 1,
			current_version int not null default 0,
			last_run_status text not null default '',
			last_run_at timestamptz,
			created_by text not null default '',
			updated_by text not null default '',
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now(),
			deleted_at timestamptz,
			foreign key(module_id, product_id) references product_modules(id, product_id)
		)`,
		`create unique index if not exists uq_api_test_cases_active_name on api_test_cases(project_id, product_id, lower(name)) where deleted_at is null`,
		`create index if not exists idx_api_test_cases_filter on api_test_cases(project_id, product_id, status, updated_at desc) where deleted_at is null`,
		`create table if not exists api_test_case_versions (
			id bigserial primary key,
			case_id bigint not null references api_test_cases(id),
			version int not null,
			snapshot jsonb not null,
			change_summary text not null default '',
			created_by text not null,
			created_at timestamptz not null default now(),
			unique(case_id, version)
		)`,
		`create table if not exists api_test_run_batches (
			id bigserial primary key,
			batch_id text not null unique,
			project_id bigint not null references projects(id),
			env_name text not null,
			status text not null default 'queued',
			total_instances int not null default 0,
			queued_instances int not null default 0,
			running_instances int not null default 0,
			passed_instances int not null default 0,
			failed_instances int not null default 0,
			canceled_instances int not null default 0,
			options jsonb not null default '{}'::jsonb,
			triggered_by text not null,
			started_at timestamptz,
			finished_at timestamptz,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)`,
		`create table if not exists api_test_run_instances (
			id bigserial primary key,
			batch_id text not null references api_test_run_batches(batch_id) on delete cascade,
			task_id text not null unique,
			case_id bigint not null references api_test_cases(id),
			case_version int not null,
			dataset_index int not null default 0,
			executor_id text,
			status text not null default 'queued',
			snapshot jsonb not null,
			result jsonb not null default '{}'::jsonb,
			error_message text not null default '',
			started_at timestamptz,
			finished_at timestamptz,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)`,
		`create index if not exists idx_api_test_run_instances_batch on api_test_run_instances(batch_id, status, id)`,
		`create table if not exists api_legacy_migrations (
			source_type text not null,
			source_id bigint not null,
			target_id bigint,
			status text not null,
			message text not null default '',
			created_at timestamptz not null default now(),
			primary key(source_type, source_id)
		)`,
		`create table if not exists api_temp_files (
			id text primary key,
			project_id bigint not null references projects(id),
			owner_user_id bigint not null references users(id),
			original_name text not null,
			stored_path text not null,
			mime_type text not null default 'application/octet-stream',
			size_bytes bigint not null check(size_bytes >= 0),
			sha256 text not null,
			expires_at timestamptz not null,
			created_at timestamptz not null default now(),
			deleted_at timestamptz
		)`,
		`create index if not exists idx_api_temp_files_expiry on api_temp_files(expires_at) where deleted_at is null`,
		`create table if not exists api_debug_runs (
			id bigserial primary key,
			task_id text not null unique,
			interface_id bigint not null references api_interfaces(id),
			project_id bigint not null references projects(id),
			executor_id text not null references executors(executor_id),
			status text not null default 'queued',
			request_snapshot jsonb not null,
			result jsonb not null default '{}'::jsonb,
			error_message text not null default '',
			triggered_by text not null,
			started_at timestamptz,
			finished_at timestamptz,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		)`,
		`create index if not exists idx_api_debug_runs_interface on api_debug_runs(interface_id, id desc)`,
		`create table if not exists api_debug_events (
			id bigserial primary key,
			task_id text not null references api_debug_runs(task_id) on delete cascade,
			sequence int not null,
			event_type text not null,
			stage text not null,
			status text not null,
			message text not null,
			progress int not null default 0,
			data jsonb not null default '{}'::jsonb,
			created_at timestamptz not null default now(),
			unique(task_id, sequence)
		)`,
		`create table if not exists api_debug_assertions (
			id bigserial primary key,
			debug_run_id bigint not null references api_debug_runs(id) on delete cascade,
			assertion_index int not null,
			assertion_type text not null,
			expression text not null default '',
			operator text not null,
			expected_value text not null default '',
			actual_value text not null default '',
			passed boolean not null,
			error_message text not null default '',
			created_at timestamptz not null default now(),
			unique(debug_run_id, assertion_index)
		)`,
	}
	statements = append(statements, elementCaptureMigrationStatements()...)
	for _, statement := range statements {
		if _, err := a.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func elementCaptureMigrationStatements() []string {
	return []string{
		`alter table element_capture_sessions add column if not exists token_hash text not null default ''`,
		`alter table element_capture_sessions add column if not exists browser_channel text not null default ''`,
		`alter table element_capture_sessions add column if not exists last_heartbeat_at timestamptz not null default now()`,
		`alter table element_capture_sessions add column if not exists interrupted_at timestamptz`,
		`alter table element_capture_sessions add column if not exists recovery_expires_at timestamptz`,
		`alter table element_capture_candidates add column if not exists name text not null default ''`,
		`alter table element_capture_candidates add column if not exists capture_url text not null default ''`,
		`alter table element_capture_candidates add column if not exists duplicate_element_id bigint references page_elements(id)`,
		`alter table element_capture_candidates add column if not exists conflict_status text not null default ''`,
		`alter table element_capture_candidates add column if not exists conflict_resolution text not null default ''`,
		`alter table element_capture_candidates add column if not exists cursor_id bigserial`,
		`drop index if exists uq_element_capture_sessions_active_executor`,
		`create unique index if not exists uq_element_capture_sessions_active_executor on element_capture_sessions(executor_id) where status in ('starting', 'active', 'interrupted')`,
	}
}

// seed 保证本地开发环境具备默认账号和基础数据。
func (a *app) seed(ctx context.Context) error {
	var roleID int64
	if err := a.db.QueryRowContext(ctx, `
		insert into roles(name, code, description, built_in)
		values('管理员', 'admin', '系统内置管理员', true)
		on conflict(code) do update set name = excluded.name, description = excluded.description, built_in=true, status='active', deleted_at = null, updated_at = now()
		returning id
	`).Scan(&roleID); err != nil {
		return err
	}
	if _, err := a.db.ExecContext(ctx, `
		insert into users(username, password_hash, display_name, email, status, role_id, mcp_api_key)
		values('admin', $1, '系统管理员', 'admin@synapse.local', 'active', $2, $3)
		on conflict(username) do update set display_name = excluded.display_name, status = 'active', role_id = excluded.role_id, mcp_api_key = coalesce(nullif(users.mcp_api_key, ''), excluded.mcp_api_key), deleted_at = null
	`, hashPassword("admin123"), roleID, generateAPIKey("admin")); err != nil {
		return err
	}

	roles := [][]string{
		{"测试负责人", "qa_lead", "负责测试计划和质量门禁"},
		{"自动化工程师", "automation_engineer", "维护自动化资产和执行任务"},
		{"只读访客", "viewer", "查看报告和统计数据"},
	}
	for _, row := range roles {
		if _, err := a.db.ExecContext(ctx, `insert into roles(name, code, description,built_in) values($1, $2, $3,true) on conflict(code) do update set name = excluded.name, description = excluded.description, built_in=true, status='active', deleted_at = null, updated_at = now()`, row[0], row[1], row[2]); err != nil {
			return err
		}
	}

	menus := []struct {
		title string
		code  string
		sort  int
	}{
		{"首页", "home", 1},
		{"界面自动化", "ui_automation", 2},
		{"测试配置", "test_config", 3},
		{"系统管理", "system", 4},
	}
	for _, menu := range menus {
		if _, err := a.db.ExecContext(ctx, `insert into system_menus(title, code, sort_order) values($1, $2, $3) on conflict(code) do update set title = excluded.title, sort_order = excluded.sort_order`, menu.title, menu.code, menu.sort); err != nil {
			return err
		}
	}

	dicts := [][]any{
		{"环境类型", "deploy_env", "prod", "生产环境", true},
		{"环境类型", "deploy_env", "staging", "预发环境", true},
		{"自动化类型", "auto_type", "ui", "界面自动化", true},
		{"自动化类型", "auto_type", "api", "接口自动化", true},
	}
	for _, row := range dicts {
		if _, err := a.db.ExecContext(ctx, `insert into dictionaries(name, code, item_key, item_value, enabled) values($1, $2, $3, $4, $5) on conflict(code, item_key) do update set name = excluded.name, item_value = excluded.item_value, enabled = excluded.enabled`, row...); err != nil {
			return err
		}
	}

	var projectID int64
	if err := a.db.QueryRowContext(ctx, `insert into projects(name, status) values('Synapse QA', 'active') on conflict(name) do update set status = 'active', deleted_at = null, updated_at = now() returning id`).Scan(&projectID); err != nil {
		return err
	}
	var productID int64
	if err := a.db.QueryRowContext(ctx, `insert into products(project_id, name, ui_type, api_type) values($1, 'Web 管理端', 'WEB', 'WEB') on conflict(project_id, name) do update set ui_type = excluded.ui_type, api_type = excluded.api_type, deleted_at = null, updated_at = now() returning id`, projectID).Scan(&productID); err != nil {
		return err
	}

	// 添加更多产品数据
	productNames := []string{"电商平台", "移动端应用", "后台管理系统", "支付网关", "数据分析平台",
		"CRM系统", "OA办公", "即时通讯", "云存储", "AI助手",
		"物流系统", "医疗系统", "教育平台", "金融服务", "社交平台",
		"游戏平台", "视频网站", "音乐应用", "购物商城", "智能硬件"}
	for _, name := range productNames {
		var exists bool
		if err := a.db.QueryRowContext(ctx, `select exists(select 1 from products where project_id = $1 and name = $2)`, projectID, name).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := a.db.ExecContext(ctx, `insert into products(project_id, name, ui_type, api_type) values($1, $2, 'WEB', 'WEB')`, projectID, name); err != nil {
				return err
			}
		}
	}

	// 添加模块数据
	a.db.ExecContext(ctx, `insert into product_modules(product_id, name) values($1, '登录模块') on conflict(product_id, name) do nothing`, productID)
	var moduleID int64
	a.db.QueryRowContext(ctx, `select id from product_modules where product_id = $1 and name = '登录模块' limit 1`, productID).Scan(&moduleID)
	moduleNames := []string{"首页", "用户管理", "权限管理", "订单管理", "商品管理",
		"数据报表", "系统设置", "日志管理", "通知中心", "帮助中心",
		"营销活动", "会员管理", "支付管理", "库存管理", "物流配送",
		"客服系统", "评论管理", "收藏夹", "购物车", "搜索模块"}
	for _, name := range moduleNames {
		var exists bool
		if err := a.db.QueryRowContext(ctx, `select exists(select 1 from product_modules where product_id = $1 and name = $2)`, productID, name).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := a.db.ExecContext(ctx, `insert into product_modules(product_id, name) values($1, $2)`, productID, name); err != nil {
				return err
			}
		}
	}

	if _, err := a.db.ExecContext(ctx, `insert into test_objects(product_id, env_name, target, deploy_env, auto_type, owner, query_enabled, write_enabled) values($1, '生产环境', 'https://qa.example.com', '生产环境', '界面自动化', 'admin', true, false) on conflict(product_id, env_name) do update set target = excluded.target, deploy_env = excluded.deploy_env, auto_type = excluded.auto_type, owner = excluded.owner, deleted_at = null, updated_at = now()`, productID); err != nil {
		return err
	}

	// 添加测试对象数据
	testEnvNames := []string{"预发环境", "测试环境", "开发环境", "UAT环境", "SIT环境",
		"性能测试", "安全测试", "兼容性测试", "压力测试", "回归测试",
		"自动化测试", "冒烟测试", "单元测试", "集成测试", "系统测试",
		"验收测试", "探索测试", "A/B测试", "灰度测试", "Beta测试"}
	for _, envName := range testEnvNames {
		var exists bool
		if err := a.db.QueryRowContext(ctx, `select exists(select 1 from test_objects where product_id = $1 and env_name = $2)`, productID, envName).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			target := "https://" + strings.ToLower(strings.ReplaceAll(envName, " ", "-")) + ".example.com"
			if _, err := a.db.ExecContext(ctx, `insert into test_objects(product_id, env_name, target, deploy_env, auto_type, owner, query_enabled, write_enabled) values($1, $2, $3, '生产环境', '界面自动化', 'admin', true, false)`, productID, envName, target); err != nil {
				return err
			}
		}
	}

	// 添加页面元素数据
	for i := 1; i <= 20; i++ {
		name := fmt.Sprintf("测试页面 %02d", i)
		category := "Synapse QA/电商平台"
		var exists bool
		if err := a.db.QueryRowContext(ctx, `select exists(select 1 from ui_assets where name = $1 and asset_type = $2 and deleted_at is null)`, name, "page").Scan(&exists); err != nil {
			return err
		}
		if !exists {
			locator := fmt.Sprintf("https://example.com/page/%d", i)
			description := fmt.Sprintf("这是第%d个测试页面，用于演示", i)
			if _, err := a.db.ExecContext(ctx, `insert into ui_assets(asset_type, name, category, method, locator, description, status) values($1, $2, $3, '登录模块', $4, $5, 'active')`, "page", name, category, locator, description); err != nil {
				return err
			}
		}
	}

	// 添加页面步骤数据
	for i := 1; i <= 20; i++ {
		name := fmt.Sprintf("测试步骤 %02d", i)
		category := "Synapse QA/电商平台"
		var exists bool
		if err := a.db.QueryRowContext(ctx, `select exists(select 1 from ui_assets where name = $1 and asset_type = $2 and deleted_at is null)`, name, "step").Scan(&exists); err != nil {
			return err
		}
		if !exists {
			status := "active"
			if i%3 == 0 {
				status = "disabled"
			}
			description := fmt.Sprintf("这是第%d个测试步骤，用于演示", i)
			if _, err := a.db.ExecContext(ctx, `insert into ui_assets(asset_type, name, category, method, locator, description, status) values($1, $2, $3, '登录模块', 'https://example.com/page/1', $4, $5)`, "step", name, category, description, status); err != nil {
				return err
			}
		}
	}

	// 查询一个页面ID用于测试用例
	var pageID int64
	if err := a.db.QueryRowContext(ctx, `select id from ui_assets where asset_type = 'page' and deleted_at is null limit 1`).Scan(&pageID); err != nil {
		pageID = 0
	}

	// 添加测试用例数据
	testCaseNames := []string{"登录功能测试", "注册功能测试", "忘记密码测试", "首页浏览测试", "商品搜索测试",
		"购物车测试", "支付流程测试", "订单提交测试", "个人中心测试", "收藏功能测试",
		"评论功能测试", "优惠券测试", "会员权益测试", "退款申请测试", "物流查询测试",
		"客服咨询测试", "消息通知测试", "设置修改测试", "退出登录测试", "权限验证测试"}
	priorities := []string{"P0", "P1", "P2", "P3"}
	statuses := []string{"draft", "ready", "in_progress", "completed"}
	for i, name := range testCaseNames {
		var exists bool
		if err := a.db.QueryRowContext(ctx, `select exists(select 1 from test_cases where product_id = $1 and name = $2)`, productID, name).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			priority := priorities[i%len(priorities)]
			status := statuses[i%len(statuses)]
			if _, err := a.db.ExecContext(ctx, `insert into test_cases(product_id, name, case_type, priority, status, description, owner) values($1, $2, 'ui', $3, $4, '这是一条自动生成的测试用例，用于演示系统功能', 'admin')`, productID, name, priority, status); err != nil {
				return err
			}
		}
	}

	if _, err := a.db.ExecContext(ctx, `
		insert into platform_settings(key, value)
		values('executor_shared_token', $1)
		on conflict(key) do nothing
	`, env("EXECUTOR_SHARED_TOKEN", "synapse-local-executor-token")); err != nil {
		return err
	}
	settingGroups := [][]string{
		{"execution", `{"defaultConcurrency":5,"maxConcurrency":100,"batchSize":1000,"executorOfflineSeconds":45}`},
		{"security", `{"sessionHours":24,"passwordMinLength":8,"loginFailureLimit":5,"lockMinutes":15}`},
		{"notification", `{"executionSuccess":true,"executionFailure":true,"executorOffline":true,"systemAlert":true,"securityAlert":true}`},
	}
	for _, setting := range settingGroups {
		if _, err := a.db.ExecContext(ctx, `
			insert into system_setting_groups(group_key,value,updated_by)
			values($1,$2::jsonb,'system') on conflict(group_key) do nothing
		`, setting[0], setting[1]); err != nil {
			return err
		}
	}
	return nil
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte("synapse:" + password))
	return hex.EncodeToString(sum[:])
}

func generateAPIKey(seed string) string {
	sum := sha256.Sum256([]byte(seed + ":" + strconv.FormatInt(time.Now().UnixNano(), 10)))
	return "mango_" + hex.EncodeToString(sum[:])[:24]
}
