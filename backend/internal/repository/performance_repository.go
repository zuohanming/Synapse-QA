package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"synapseqa/backend/internal/model"
)

// PerformanceRepository 负责性能测试方案与执行记录的数据读写。
type PerformanceRepository struct {
	db *sql.DB
}

func NewPerformanceRepository(db *sql.DB) *PerformanceRepository {
	return &PerformanceRepository{db: db}
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
	if filter.LoadMode != "" {
		args = append(args, filter.LoadMode)
		where = append(where, fmt.Sprintf("p.load_mode = $%d", len(args)))
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
		       p.load_mode, p.vus, p.duration, p.stages, p.thresholds, p.status, p.priority, p.owner,
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
		       p.load_mode, p.vus, p.duration, p.stages, p.thresholds, p.status, p.priority, p.owner,
		       p.tags, p.description, p.created_by, p.created_at, p.updated_at
		from perf_test_plans p
		left join products pr on pr.id = p.product_id
		where p.id = $1 and p.deleted_at is null`, id)
	return scanPerfTestPlan(row)
}

func (r *PerformanceRepository) CreatePlan(ctx context.Context, req model.PerfTestPlanRequest, actor string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		insert into perf_test_plans(product_id, name, target_url, method, headers, body, load_mode, vus, duration, stages, thresholds, status, priority, owner, tags, description, created_by)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		returning id`,
		req.ProductID, req.Name, req.TargetURL, req.Method, req.Headers, req.Body, req.LoadMode, req.VUs, req.Duration, req.Stages, req.Thresholds, req.Status, req.Priority, req.Owner, req.Tags, req.Description, actor).Scan(&id)
	return id, err
}

func (r *PerformanceRepository) UpdatePlan(ctx context.Context, id int64, req model.PerfTestPlanRequest) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		update perf_test_plans set product_id = $1, name = $2, target_url = $3, method = $4, headers = $5,
			body = $6, load_mode = $7, vus = $8, duration = $9, stages = $10, thresholds = $11,
			status = $12, priority = $13, owner = $14, tags = $15, description = $16, updated_at = now()
		where id = $17 and deleted_at is null`,
		req.ProductID, req.Name, req.TargetURL, req.Method, req.Headers, req.Body, req.LoadMode, req.VUs, req.Duration, req.Stages, req.Thresholds, req.Status, req.Priority, req.Owner, req.Tags, req.Description, id)
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
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("run.status = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from perf_test_runs run where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select run.id, run.plan_id, coalesce(plan.name, ''), run.status, run.triggered_by, run.exit_code, run.total_requests,
		       run.avg_duration_ms, run.p95_duration_ms, run.error_rate, run.rps, run.summary,
		       run.started_at, run.finished_at, run.created_at, run.updated_at
		from perf_test_runs run
		left join perf_test_plans plan on plan.id = run.plan_id
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
	row := r.db.QueryRowContext(ctx, `
		select run.id, run.plan_id, coalesce(plan.name, ''), run.status, run.triggered_by, run.exit_code, run.total_requests,
		       run.avg_duration_ms, run.p95_duration_ms, run.error_rate, run.rps, run.summary,
		       run.started_at, run.finished_at, run.created_at, run.updated_at
		from perf_test_runs run
		left join perf_test_plans plan on plan.id = run.plan_id
		where run.id = $1`, id)
	return scanPerfTestRun(row)
}

func (r *PerformanceRepository) CreateRun(ctx context.Context, planID int64, actor string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		insert into perf_test_runs(plan_id, status, triggered_by)
		values($1, 'pending', $2)
		returning id`, planID, actor).Scan(&id)
	return id, err
}

type perfTestScanner interface {
	Scan(dest ...any) error
}

func scanPerfTestPlan(scanner perfTestScanner) (model.PerfTestPlan, error) {
	var item model.PerfTestPlan
	err := scanner.Scan(&item.ID, &item.ProductID, &item.ProductName, &item.Name, &item.TargetURL, &item.Method,
		&item.Headers, &item.Body, &item.LoadMode, &item.VUs, &item.Duration,
		&item.Stages, &item.Thresholds, &item.Status, &item.Priority, &item.Owner,
		&item.Tags, &item.Description, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return model.PerfTestPlan{}, err
	}
	if len(item.Headers) == 0 {
		item.Headers = json.RawMessage(`{}`)
	}
	if len(item.Stages) == 0 {
		item.Stages = json.RawMessage(`[]`)
	}
	if len(item.Thresholds) == 0 {
		item.Thresholds = json.RawMessage(`{}`)
	}
	return item, nil
}

func scanPerfTestRun(scanner perfTestScanner) (model.PerfTestRun, error) {
	var item model.PerfTestRun
	var exitCode sql.NullInt64
	var avg, p95, errorRate, rps sql.NullFloat64
	var startedAt, finishedAt sql.NullTime
	err := scanner.Scan(&item.ID, &item.PlanID, &item.PlanName, &item.Status, &item.TriggeredBy,
		&exitCode, &item.TotalRequests, &avg, &p95, &errorRate, &rps, &item.Summary,
		&startedAt, &finishedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return model.PerfTestRun{}, err
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		item.ExitCode = &code
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
	if startedAt.Valid {
		item.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		item.FinishedAt = &finishedAt.Time
	}
	if len(item.Summary) == 0 {
		item.Summary = json.RawMessage(`{}`)
	}
	return item, nil
}
