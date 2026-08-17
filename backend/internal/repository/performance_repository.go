package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

// PerformanceRepository 负责性能测试方案与执行记录的数据读写。
type PerformanceRepository struct {
	db *sql.DB
}

func NewPerformanceRepository(db *sql.DB) *PerformanceRepository {
	return &PerformanceRepository{db: db}
}

// perfRunColumns 是执行记录查询的统一列清单，scanPerfTestRun 的顺序与之对应。
const perfRunColumns = `run.id, run.plan_id, coalesce(plan.name, ''), run.scenario_type, run.status, run.triggered_by,
	run.plan_snapshot, run.config_hash, run.executor_id, run.executor_name, run.k6_version, run.environment,
	run.requested_at, run.dispatched_at, run.dispatch_deadline_at, run.start_deadline_at, run.expected_finish_at,
	run.script_hash, run.generator_version, run.task_id, run.callback_token_hash, run.idempotency_key,
	run.exit_code, run.duration_ms, run.total_requests, run.avg_duration_ms, run.p95_duration_ms, run.error_rate, run.rps,
	run.error_message, run.failure_stage, run.diagnostic_output, run.needs_attention,
	run.summary, run.series, run.started_at, run.finished_at, run.created_at, run.updated_at`

const perfRunJoin = `from perf_test_runs run
	left join perf_test_plans plan on plan.id = run.plan_id`

// perfRunUpdateColumns 是 UpdateRunStatus 允许白名单更新的列，顺序固定保证 SQL 稳定。
var perfRunUpdateColumns = []string{
	"task_id", "callback_token_hash", "executor_id", "executor_name",
	"script_hash", "generator_version", "k6_version",
	"dispatch_deadline_at", "start_deadline_at", "dispatched_at", "expected_finish_at",
	"started_at", "finished_at",
}

func (r *PerformanceRepository) ListPlans(ctx context.Context, filter model.PerfTestPlanFilter, page, pageSize int) ([]model.PerfTestPlan, int64, error) {
	where := []string{"p.deleted_at is null"}
	args := []any{}
	if filter.ID != "" {
		id, err := strconv.ParseInt(filter.ID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("p.id = $%d", len(args)))
	}
	if filter.Name != "" {
		args = append(args, "%"+strings.ToLower(filter.Name)+"%")
		where = append(where, fmt.Sprintf("lower(p.name) like $%d", len(args)))
	}
	if filter.ProductID != "" {
		id, err := strconv.ParseInt(filter.ProductID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("p.product_id = $%d", len(args)))
	}
	if filter.ScenarioType != "" {
		args = append(args, filter.ScenarioType)
		where = append(where, fmt.Sprintf("p.scenario_type = $%d", len(args)))
	}
	if filter.Environment != "" {
		args = append(args, filter.Environment)
		where = append(where, fmt.Sprintf("p.environment = $%d", len(args)))
	}
	if filter.Priority != "" {
		args = append(args, filter.Priority)
		where = append(where, fmt.Sprintf("p.priority = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("p.status = $%d", len(args)))
	}
	if filter.Owner != "" {
		args = append(args, "%"+strings.ToLower(filter.Owner)+"%")
		where = append(where, fmt.Sprintf("lower(p.owner) like $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from perf_test_plans p where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select p.id, p.product_id, coalesce(pr.name, ''), p.name, p.target_url, p.method, p.headers, p.body,
		       p.scenario_type, p.load_config, p.environment, p.thresholds, p.status, p.priority, p.owner,
		       p.tags, p.description, p.created_by, p.created_at, p.updated_at
		from perf_test_plans p
		left join products pr on pr.id = p.product_id
		where `+whereSQL+`
		order by p.id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.PerfTestPlan{}
	for rows.Next() {
		item, err := scanPerfTestPlan(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *PerformanceRepository) GetPlan(ctx context.Context, id int64) (model.PerfTestPlan, error) {
	row := r.db.QueryRowContext(ctx, `
		select p.id, p.product_id, coalesce(pr.name, ''), p.name, p.target_url, p.method, p.headers, p.body,
		       p.scenario_type, p.load_config, p.environment, p.thresholds, p.status, p.priority, p.owner,
		       p.tags, p.description, p.created_by, p.created_at, p.updated_at
		from perf_test_plans p
		left join products pr on pr.id = p.product_id
		where p.id = $1 and p.deleted_at is null`, id)
	return scanPerfTestPlan(row)
}

func (r *PerformanceRepository) CreatePlan(ctx context.Context, req model.PerfTestPlanRequest, actor string) (int64, error) {
	thresholds, err := json.Marshal(req.Thresholds)
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.db.QueryRowContext(ctx, `
		insert into perf_test_plans(product_id, name, target_url, method, headers, body, scenario_type, load_config, environment, thresholds, status, priority, owner, tags, description, created_by)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		returning id`,
		req.ProductID, req.Name, req.TargetURL, req.Method, req.Headers, req.Body, req.ScenarioType, req.LoadConfig, req.Environment, thresholds, req.Status, req.Priority, req.Owner, req.Tags, req.Description, actor).Scan(&id)
	return id, err
}

func (r *PerformanceRepository) UpdatePlan(ctx context.Context, id int64, req model.PerfTestPlanRequest) (int64, error) {
	thresholds, err := json.Marshal(req.Thresholds)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, `
		update perf_test_plans set product_id = $1, name = $2, target_url = $3, method = $4, headers = $5,
			body = $6, scenario_type = $7, load_config = $8, environment = $9, thresholds = $10,
			status = $11, priority = $12, owner = $13, tags = $14, description = $15, updated_at = now()
		where id = $16 and deleted_at is null`,
		req.ProductID, req.Name, req.TargetURL, req.Method, req.Headers, req.Body, req.ScenarioType, req.LoadConfig, req.Environment, thresholds, req.Status, req.Priority, req.Owner, req.Tags, req.Description, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PerformanceRepository) DeletePlan(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update perf_test_plans set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PerformanceRepository) ExistsProduct(ctx context.Context, id int64) bool {
	var exists bool
	_ = r.db.QueryRowContext(ctx, `select exists(select 1 from products where id = $1 and deleted_at is null and status = 'active')`, id).Scan(&exists)
	return exists
}

func (r *PerformanceRepository) ListRuns(ctx context.Context, filter model.PerfTestRunFilter, page, pageSize int) ([]model.PerfTestRun, int64, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if filter.ID != "" {
		id, err := strconv.ParseInt(filter.ID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("run.id = $%d", len(args)))
	}
	if filter.PlanID != "" {
		id, err := strconv.ParseInt(filter.PlanID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("run.plan_id = $%d", len(args)))
	}
	if filter.ScenarioType != "" {
		args = append(args, filter.ScenarioType)
		where = append(where, fmt.Sprintf("run.scenario_type = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("run.status = $%d", len(args)))
	}
	if filter.Environment != "" {
		args = append(args, filter.Environment)
		where = append(where, fmt.Sprintf("run.environment = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from perf_test_runs run where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select `+perfRunColumns+`
		`+perfRunJoin+`
		where `+whereSQL+`
		order by run.id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.PerfTestRun{}
	for rows.Next() {
		item, err := scanPerfTestRun(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *PerformanceRepository) GetRun(ctx context.Context, id int64) (model.PerfTestRun, error) {
	row := r.db.QueryRowContext(ctx, `select `+perfRunColumns+` `+perfRunJoin+` where run.id = $1`, id)
	return scanPerfTestRun(row)
}

func (r *PerformanceRepository) GetRunByTaskID(ctx context.Context, taskID string) (model.PerfTestRun, error) {
	row := r.db.QueryRowContext(ctx, `select `+perfRunColumns+` `+perfRunJoin+` where run.task_id = $1`, taskID)
	return scanPerfTestRun(row)
}

func (r *PerformanceRepository) GetRunByIdempotencyKey(ctx context.Context, triggeredBy, key string) (model.PerfTestRun, error) {
	row := r.db.QueryRowContext(ctx, `select `+perfRunColumns+` `+perfRunJoin+` where run.triggered_by = $1 and run.idempotency_key = $2`, triggeredBy, key)
	return scanPerfTestRun(row)
}

func (r *PerformanceRepository) CreateRun(ctx context.Context, planID int64, scenarioType, environment, configHash, idempotencyKey, triggeredBy string, planSnapshot json.RawMessage, requestedAt, expectedFinishAt time.Time) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		insert into perf_test_runs(plan_id, scenario_type, plan_snapshot, config_hash, environment, idempotency_key, triggered_by, requested_at, expected_finish_at, status)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')
		returning id`,
		planID, scenarioType, planSnapshot, configHash, environment, idempotencyKey, triggeredBy, requestedAt, expectedFinishAt).Scan(&id)
	return id, err
}

// MarkNeedsAttention 标记记录需要人工关注，但不改变当前状态（SPEC §5.4）。
func (r *PerformanceRepository) MarkNeedsAttention(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update perf_test_runs set needs_attention = true, updated_at = now() where id = $1`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateRunStatus 条件更新运行状态，from 支持逗号分隔多值；rows 为 0 表示竞态（非法转移）。
func (r *PerformanceRepository) UpdateRunStatus(ctx context.Context, id int64, from, to string, extra map[string]any) (int64, error) {
	sets := []string{"status = $2", "updated_at = now()"}
	args := []any{id, to}
	for _, col := range perfRunUpdateColumns {
		val, ok := extra[col]
		if !ok {
			continue
		}
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	query := fmt.Sprintf("update perf_test_runs set %s where id = $1 and status = any(string_to_array($%d, ','))",
		strings.Join(sets, ", "), len(args)+1)
	args = append(args, from)
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateRunResult 条件更新终态并回填指标，同时在同一事务中撤销回调凭据（SPEC §4.2）。
func (r *PerformanceRepository) UpdateRunResult(ctx context.Context, id int64, from []string, to string, result model.PerfRunResult) (int64, error) {
	avg := nullableFloat(result.AvgDurationMs)
	p95 := nullableFloat(result.P95DurationMs)
	errorRate := nullableFloat(result.ErrorRate)
	rps := nullableFloat(result.RPS)
	var exitCode any
	if result.ExitCode != nil {
		exitCode = *result.ExitCode
	}
	var durationMs any
	if result.DurationMs != nil {
		durationMs = *result.DurationMs
	}
	res, err := r.db.ExecContext(ctx, `
		update perf_test_runs set status = $2, total_requests = $3, avg_duration_ms = $4, p95_duration_ms = $5,
			error_rate = $6, rps = $7, summary = $8, exit_code = $9, duration_ms = $10,
			error_message = $11, failure_stage = $12, diagnostic_output = $13, needs_attention = $14,
			script_hash = coalesce(nullif($15, ''), script_hash),
			generator_version = coalesce(nullif($16, ''), generator_version),
			k6_version = coalesce(nullif($17, ''), k6_version),
			finished_at = now(), callback_token_hash = '', updated_at = now()
		where id = $1 and status = any(string_to_array($18, ','))`,
		id, to, result.TotalRequests, avg, p95, errorRate, rps, result.Summary, exitCode, durationMs,
		result.ErrorMessage, result.FailureStage, result.DiagnosticOutput, result.NeedsAttention,
		result.ScriptHash, result.GeneratorVersion, result.K6Version,
		strings.Join(from, ","))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

type perfTestScanner interface {
	Scan(dest ...any) error
}

func scanPerfTestPlan(scanner perfTestScanner) (model.PerfTestPlan, error) {
	var item model.PerfTestPlan
	var thresholdsRaw []byte
	err := scanner.Scan(&item.ID, &item.ProductID, &item.ProductName, &item.Name, &item.TargetURL, &item.Method,
		&item.Headers, &item.Body, &item.ScenarioType, &item.LoadConfig, &item.Environment,
		&thresholdsRaw, &item.Status, &item.Priority, &item.Owner,
		&item.Tags, &item.Description, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return model.PerfTestPlan{}, err
	}
	if len(item.Headers) == 0 {
		item.Headers = json.RawMessage(`{}`)
	}
	if len(item.LoadConfig) == 0 {
		item.LoadConfig = json.RawMessage(`{}`)
	}
	if len(thresholdsRaw) > 0 {
		_ = json.Unmarshal(thresholdsRaw, &item.Thresholds)
	}
	if item.Thresholds == nil {
		item.Thresholds = []model.PerfThreshold{}
	}
	return item, nil
}

func scanPerfTestRun(scanner perfTestScanner) (model.PerfTestRun, error) {
	var item model.PerfTestRun
	var exitCode sql.NullInt64
	var durationMs sql.NullInt64
	var avg, p95, errorRate, rps sql.NullFloat64
	var requestedAt, dispatchedAt, dispatchDeadlineAt, startDeadlineAt, expectedFinishAt, startedAt, finishedAt sql.NullTime
	var executorID, taskID, idempotencyKey sql.NullString
	var errorMessage sql.NullString
	var seriesRaw []byte
	err := scanner.Scan(&item.ID, &item.PlanID, &item.PlanName, &item.ScenarioType, &item.Status, &item.TriggeredBy,
		&item.PlanSnapshot, &item.ConfigHash, &executorID, &item.ExecutorName, &item.K6Version, &item.Environment,
		&requestedAt, &dispatchedAt, &dispatchDeadlineAt, &startDeadlineAt, &expectedFinishAt,
		&item.ScriptHash, &item.GeneratorVersion, &taskID, &item.CallbackTokenHash, &idempotencyKey,
		&exitCode, &durationMs, &item.TotalRequests, &avg, &p95, &errorRate, &rps,
		&errorMessage, &item.FailureStage, &item.DiagnosticOutput, &item.NeedsAttention,
		&item.Summary, &seriesRaw, &startedAt, &finishedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return model.PerfTestRun{}, err
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		item.ExitCode = &code
	}
	if durationMs.Valid {
		ms := int(durationMs.Int64)
		item.DurationMs = &ms
	}
	if avg.Valid {
		item.AvgDurationMs = &avg.Float64
	}
	if p95.Valid {
		item.P95DurationMs = &p95.Float64
	}
	if errorRate.Valid {
		item.ErrorRate = &errorRate.Float64
	}
	if rps.Valid {
		item.RPS = &rps.Float64
	}
	if requestedAt.Valid {
		item.RequestedAt = &requestedAt.Time
	}
	if dispatchedAt.Valid {
		item.DispatchedAt = &dispatchedAt.Time
	}
	if dispatchDeadlineAt.Valid {
		item.DispatchDeadlineAt = &dispatchDeadlineAt.Time
	}
	if startDeadlineAt.Valid {
		item.StartDeadlineAt = &startDeadlineAt.Time
	}
	if expectedFinishAt.Valid {
		item.ExpectedFinishAt = &expectedFinishAt.Time
	}
	if startedAt.Valid {
		item.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		item.FinishedAt = &finishedAt.Time
	}
	if executorID.Valid {
		item.ExecutorID = executorID.String
	}
	if taskID.Valid {
		item.TaskID = taskID.String
	}
	if idempotencyKey.Valid {
		item.IdempotencyKey = idempotencyKey.String
	}
	if errorMessage.Valid {
		item.ErrorMessage = errorMessage.String
	}
	if len(item.Summary) == 0 {
		item.Summary = json.RawMessage(`{}`)
	}
	if len(seriesRaw) > 0 {
		item.Series = json.RawMessage(seriesRaw)
	}
	return item, nil
}
