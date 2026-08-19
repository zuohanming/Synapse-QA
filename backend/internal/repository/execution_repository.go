package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

// ExecutionRepository 负责执行批次、任务和日志的数据库读写。
type ExecutionRepository struct {
	db *sql.DB
}

func NewExecutionRepository(db *sql.DB) *ExecutionRepository {
	return &ExecutionRepository{db: db}
}

// CreateRun 创建执行批次。
func (r *ExecutionRepository) CreateRun(ctx context.Context, req model.ExecutionRunRequest, triggeredBy string) (model.ExecutionRun, error) {
	row := r.db.QueryRowContext(ctx, `
		insert into execution_runs(run_type, status, headless, triggered_by, case_ids)
		values($1, $2, $3, $4, $5)
		returning id, run_type, status, headless, triggered_by, case_ids, summary, started_at, finished_at, created_at, updated_at, project_id
	`, req.RunType, "pending", req.Headless == nil || *req.Headless, triggeredBy, formatInt64Array(req.CaseIDs))
	return scanExecutionRun(row)
}

// CreateRunWithProject 创建已固化项目归属的执行批次。
func (r *ExecutionRepository) CreateRunWithProject(ctx context.Context, req model.ExecutionRunRequest, triggeredBy string, projectID *int64) (model.ExecutionRun, error) {
	row := r.db.QueryRowContext(ctx, `
		insert into execution_runs(run_type, status, headless, triggered_by, case_ids, project_id)
		values($1, $2, $3, $4, $5, $6)
		returning id, run_type, status, headless, triggered_by, case_ids, summary, started_at, finished_at, created_at, updated_at, project_id
	`, req.RunType, "pending", req.Headless == nil || *req.Headless, triggeredBy, formatInt64Array(req.CaseIDs), nullableInt64(projectID))
	return scanExecutionRun(row)
}

// ResolveRunProject 校验执行批次中的全部用例，并解析唯一且可访问的项目。
func (r *ExecutionRepository) ResolveRunProject(ctx context.Context, caseIDs []int64, userID int64, admin bool) (int64, error) {
	var projectID sql.NullInt64
	var caseCount, validCaseCount, projectCount int64
	var authorized bool
	err := r.db.QueryRowContext(ctx, `
		select min(p.id), count(*)::bigint, count(p.id)::bigint, count(distinct p.id)::bigint,
		       coalesce(bool_and($2 or exists(
			select 1 from project_members pm
			where pm.user_id = $3 and pm.project_id = p.id
		       )), false)
		from unnest($1::bigint[]) case_ref(case_id)
		left join test_cases tc on tc.id = case_ref.case_id and tc.deleted_at is null
		left join products product on product.id = tc.product_id and product.deleted_at is null
		left join projects p on p.id = product.project_id and p.deleted_at is null and p.status = 'active'
	`, formatInt64Array(caseIDs), admin, userID).Scan(&projectID, &caseCount, &validCaseCount, &projectCount, &authorized)
	if err != nil {
		return 0, err
	}
	if caseCount != int64(len(caseIDs)) || validCaseCount != caseCount || !projectID.Valid {
		return 0, errors.New("测试用例不存在或所属项目无效")
	}
	if projectCount != 1 {
		return 0, errors.New("执行批次中的用例必须属于同一项目")
	}
	if !authorized {
		return 0, errors.New("无权访问该项目")
	}
	return projectID.Int64, nil
}

func formatInt64Array(values []int64) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = strconv.FormatInt(value, 10)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

// GetRun 查询执行批次基础信息。
func (r *ExecutionRepository) GetRun(ctx context.Context, id int64) (model.ExecutionRun, error) {
	row := r.db.QueryRowContext(ctx, `
		select id, run_type, status, headless, triggered_by, case_ids, summary, started_at, finished_at, created_at, updated_at, project_id
		from execution_runs
		where id = $1
	`, id)
	return scanExecutionRun(row)
}

// GetRunScoped 查询用户可见的执行批次；管理员包含历史 project_id 为 NULL 的批次。
func (r *ExecutionRepository) GetRunScoped(ctx context.Context, userID int64, admin bool, id int64) (model.ExecutionRun, error) {
	row := r.db.QueryRowContext(ctx, `
		select er.id, er.run_type, er.status, er.headless, er.triggered_by, er.case_ids, er.summary,
		       er.started_at, er.finished_at, er.created_at, er.updated_at, er.project_id
		from execution_runs er
		where er.id = $1
		  and ($3 or (er.project_id is not null and exists(
			select 1 from project_members pm
			where pm.user_id = $2 and pm.project_id = er.project_id
		  )))
	`, id, userID, admin)
	return scanExecutionRun(row)
}

// ListRuns 分页查询执行批次。
func (r *ExecutionRepository) ListRuns(ctx context.Context, filter model.ExecutionRunFilter, page, pageSize int) ([]model.ExecutionRun, int64, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if filter.ID != "" {
		id, err := strconv.ParseInt(filter.ID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("id = $%d", len(args)))
	}
	if filter.RunType != "" {
		args = append(args, filter.RunType)
		where = append(where, fmt.Sprintf("run_type = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from execution_runs where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select id, run_type, status, headless, triggered_by, case_ids, summary, started_at, finished_at, created_at, updated_at, project_id
		from execution_runs
		where `+whereSQL+`
		order by id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)),
		queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []model.ExecutionRun{}
	for rows.Next() {
		item, err := scanExecutionRun(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// ListRunsScoped 按项目成员关系查询用户可见的执行批次。
func (r *ExecutionRepository) ListRunsScoped(ctx context.Context, userID int64, admin bool, filter model.ExecutionRunFilter, page, pageSize int) ([]model.ExecutionRun, int64, error) {
	where := []string{`($2 or (er.project_id is not null and exists(
		select 1 from project_members pm where pm.user_id = $1 and pm.project_id = er.project_id
	)))`}
	args := []any{userID, admin}
	if filter.ID != "" {
		id, err := strconv.ParseInt(filter.ID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("er.id = $%d", len(args)))
	}
	if filter.RunType != "" {
		args = append(args, filter.RunType)
		where = append(where, fmt.Sprintf("er.run_type = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("er.status = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from execution_runs er where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select er.id, er.run_type, er.status, er.headless, er.triggered_by, er.case_ids, er.summary,
		       er.started_at, er.finished_at, er.created_at, er.updated_at, er.project_id
		from execution_runs er
		where `+whereSQL+`
		order by er.id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.ExecutionRun{}
	for rows.Next() {
		item, err := scanExecutionRun(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// StatisticsScoped 在数据库内完成执行统计聚合，避免将全部执行批次加载到应用层。
func (r *ExecutionRepository) StatisticsScoped(ctx context.Context, userID int64, admin bool, now time.Time) (model.ExecutionStatistics, error) {
	now = now.UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	trendStart := dayStart.AddDate(0, 0, -13)
	currentStart := dayStart.AddDate(0, 0, -6)
	previousStart := currentStart.AddDate(0, 0, -7)
	trendEnd := dayStart.AddDate(0, 0, 1)

	rows, err := r.db.QueryContext(ctx, `
		with scoped_runs as (
			select er.created_at, er.status,
				case when coalesce(er.summary->>'total', '') ~ '^[0-9]+$' then (er.summary->>'total')::bigint else 0 end as total_cases,
				case when coalesce(er.summary->>'passed', '') ~ '^[0-9]+$' then (er.summary->>'passed')::bigint else 0 end as passed_cases,
				case when coalesce(er.summary->>'failed', '') ~ '^[0-9]+$' then (er.summary->>'failed')::bigint else 0 end as failed_cases
			from execution_runs er
			where ($2::boolean or (er.project_id is not null and exists(
				select 1 from project_members pm
				where pm.user_id = $1 and pm.project_id = er.project_id
			)))
		), totals as (
			select
				count(*)::bigint as total_runs,
				coalesce(sum(total_cases), 0)::bigint as total_cases,
				coalesce(sum(passed_cases), 0)::bigint as passed_cases,
				coalesce(sum(failed_cases), 0)::bigint as failed_cases,
				coalesce(sum(case when status = 'failed' then 1 else 0 end), 0)::bigint as failed_runs,
				coalesce(sum(case when status in ('pending', 'queued', 'running') then 1 else 0 end), 0)::bigint as running_runs,
				coalesce(sum(case when created_at >= $4 then 1 else 0 end), 0)::bigint as current_runs,
				coalesce(sum(case when created_at >= $4 then total_cases else 0 end), 0)::bigint as current_cases,
				coalesce(sum(case when created_at >= $4 then passed_cases else 0 end), 0)::bigint as current_passed,
				coalesce(sum(case when created_at >= $5 and created_at < $4 then 1 else 0 end), 0)::bigint as previous_runs,
				coalesce(sum(case when created_at >= $5 and created_at < $4 then total_cases else 0 end), 0)::bigint as previous_cases,
				coalesce(sum(case when created_at >= $5 and created_at < $4 then passed_cases else 0 end), 0)::bigint as previous_passed
			from scoped_runs
		), trend as (
			select date_trunc('day', created_at at time zone 'UTC') at time zone 'UTC' as bucket,
				count(*)::bigint as runs,
				coalesce(sum(total_cases), 0)::bigint as cases,
				coalesce(sum(passed_cases), 0)::bigint as passed
			from scoped_runs
			where created_at >= $3 and created_at < $6
			group by 1
		)
		select totals.total_runs, totals.total_cases, totals.passed_cases, totals.failed_cases,
			totals.failed_runs, totals.running_runs, totals.current_runs, totals.current_cases,
			totals.current_passed, totals.previous_runs, totals.previous_cases, totals.previous_passed,
			days.bucket, coalesce(trend.runs, 0)::bigint, coalesce(trend.cases, 0)::bigint,
			coalesce(trend.passed, 0)::bigint
		from totals
		cross join generate_series($3::timestamptz, $6::timestamptz - interval '1 day', interval '1 day') days(bucket)
		left join trend on trend.bucket = days.bucket
		order by days.bucket
	`, userID, admin, trendStart, currentStart, previousStart, trendEnd)
	if err != nil {
		return model.ExecutionStatistics{}, err
	}
	defer rows.Close()

	result := model.ExecutionStatistics{Trend: make([]model.ExecutionTrendPoint, 14)}
	for index := range result.Trend {
		result.Trend[index].Date = trendStart.AddDate(0, 0, index).Format("01-02")
	}
	var (
		currentRuns, previousRuns, currentCases, previousCases, currentPassed, previousPassed int64
	)
	trendIndex := 0
	for rows.Next() {
		var (
			bucket                             time.Time
			trendRuns, trendCases, trendPassed int64
		)
		if err := rows.Scan(
			&result.TotalRuns, &result.TotalCases, &result.PassedCases, &result.FailedCases,
			&result.FailedRuns, &result.RunningRuns, &currentRuns, &currentCases,
			&currentPassed, &previousRuns, &previousCases, &previousPassed,
			&bucket, &trendRuns, &trendCases, &trendPassed,
		); err != nil {
			return model.ExecutionStatistics{}, err
		}
		if trendIndex < len(result.Trend) {
			result.Trend[trendIndex] = model.ExecutionTrendPoint{
				Date:     bucket.UTC().Format("01-02"),
				Runs:     trendRuns,
				Cases:    trendCases,
				PassRate: executionPercent(trendPassed, trendCases),
			}
		}
		trendIndex++
	}
	if err := rows.Err(); err != nil {
		return model.ExecutionStatistics{}, err
	}
	result.PassRate = executionPercent(result.PassedCases, result.TotalCases)
	result.RunChange = executionChangeRate(currentRuns, previousRuns)
	result.CaseChange = executionChangeRate(currentCases, previousCases)
	result.PassRateChange = executionPercent(currentPassed, currentCases) - executionPercent(previousPassed, previousCases)
	return result, nil
}

func executionPercent(value, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}

func executionChangeRate(current, previous int64) float64 {
	if previous == 0 {
		if current > 0 {
			return 100
		}
		return 0
	}
	return float64(current-previous) * 100 / float64(previous)
}

// UpdateRunStatus 更新执行批次状态和摘要。
func (r *ExecutionRepository) UpdateRunStatus(ctx context.Context, id int64, status string, summary json.RawMessage) error {
	var finishedAt *time.Time
	if status == "completed" || status == "failed" || status == "canceled" {
		now := time.Now()
		finishedAt = &now
	}
	_, err := r.db.ExecContext(ctx, `
		update execution_runs
		set status = $1, summary = $2, finished_at = coalesce($3, finished_at), updated_at = now()
		where id = $4
	`, status, summary, finishedAt, id)
	return err
}

// UpdateRunStatusScoped 只更新当前用户仍可见的执行批次。
func (r *ExecutionRepository) UpdateRunStatusScoped(ctx context.Context, userID int64, admin bool, id int64, status string, summary json.RawMessage) error {
	var finishedAt *time.Time
	if status == "completed" || status == "failed" || status == "canceled" {
		now := time.Now()
		finishedAt = &now
	}
	result, err := r.db.ExecContext(ctx, `
		update execution_runs
		set status = $1, summary = $2, finished_at = coalesce($3, finished_at), updated_at = now()
		where id = $4
		  and ($6 or (project_id is not null and exists(
			select 1 from project_members pm
			where pm.user_id = $5 and pm.project_id = execution_runs.project_id
		  )))
	`, status, summary, finishedAt, id, userID, admin)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// StartRun 将批次状态改为 running 并记录开始时间。
func (r *ExecutionRepository) StartRun(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `
		update execution_runs
		set status = 'running', started_at = coalesce(started_at, $1), updated_at = now()
		where id = $2
	`, now, id)
	return err
}

// CreateTask 创建执行任务。
func (r *ExecutionRepository) CreateTask(ctx context.Context, runID int64, taskID string, caseID int64, executorID, taskType, callbackURL string, payload json.RawMessage) (model.ExecutionTask, error) {
	row := r.db.QueryRowContext(ctx, `
		insert into execution_tasks(run_id, task_id, case_id, executor_id, task_type, payload, callback_url, status)
		values($1, $2, $3, $4, $5, $6, $7, 'queued')
		returning id, run_id, task_id, case_id, executor_id, task_type, payload, callback_url, status, result, started_at, finished_at, created_at, updated_at
	`, runID, taskID, caseID, executorID, taskType, payload, callbackURL)
	return scanExecutionTask(row)
}

// GetTaskByTaskID 通过执行器侧任务 ID 查询任务。
func (r *ExecutionRepository) GetTaskByTaskID(ctx context.Context, taskID string) (model.ExecutionTask, error) {
	row := r.db.QueryRowContext(ctx, `
		select id, run_id, task_id, case_id, executor_id, task_type, payload, callback_url, status, result, started_at, finished_at, created_at, updated_at
		from execution_tasks
		where task_id = $1
	`, taskID)
	return scanExecutionTask(row)
}

// GetTask 通过数据库主键查询任务。
func (r *ExecutionRepository) GetTask(ctx context.Context, id int64) (model.ExecutionTask, error) {
	row := r.db.QueryRowContext(ctx, `
		select id, run_id, task_id, case_id, executor_id, task_type, payload, callback_url, status, result, started_at, finished_at, created_at, updated_at
		from execution_tasks
		where id = $1
	`, id)
	return scanExecutionTask(row)
}

// ListTasksByRun 查询某个执行批次下的所有任务。
func (r *ExecutionRepository) ListTasksByRun(ctx context.Context, runID int64) ([]model.ExecutionTask, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id, run_id, task_id, case_id, executor_id, task_type, payload, callback_url, status, result, started_at, finished_at, created_at, updated_at
		from execution_tasks
		where run_id = $1
		order by id asc
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []model.ExecutionTask{}
	for rows.Next() {
		item, err := scanExecutionTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// CountActiveTasksByExecutor 返回平台已下发但尚未结束的任务数量。
func (r *ExecutionRepository) CountActiveTasksByExecutor(ctx context.Context, executorID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		select count(*) from execution_tasks
		where executor_id = $1 and status in ('queued', 'running')
	`, executorID).Scan(&count)
	return count, err
}

// ListTasks 分页查询任务。
func (r *ExecutionRepository) ListTasks(ctx context.Context, filter model.ExecutionTaskFilter, page, pageSize int) ([]model.ExecutionTask, int64, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if filter.RunID != "" {
		runID, err := strconv.ParseInt(filter.RunID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, runID)
		where = append(where, fmt.Sprintf("run_id = $%d", len(args)))
	}
	if filter.ExecutorID != "" {
		args = append(args, filter.ExecutorID)
		where = append(where, fmt.Sprintf("executor_id = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from execution_tasks where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select id, run_id, task_id, case_id, executor_id, task_type, payload, callback_url, status, result, started_at, finished_at, created_at, updated_at
		from execution_tasks
		where `+whereSQL+`
		order by id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)),
		queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []model.ExecutionTask{}
	for rows.Next() {
		item, err := scanExecutionTask(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// UpdateTaskStatus 更新任务状态。
func (r *ExecutionRepository) UpdateTaskStatus(ctx context.Context, id int64, status string) error {
	var startedAt, finishedAt *time.Time
	now := time.Now()
	if status == "running" {
		startedAt = &now
	}
	if status == "success" || status == "failed" || status == "canceled" {
		finishedAt = &now
	}
	_, err := r.db.ExecContext(ctx, `
		update execution_tasks
		set status = $1,
		    started_at = coalesce($2, started_at),
		    finished_at = coalesce($3, finished_at),
		    updated_at = now()
		where id = $4
	`, status, startedAt, finishedAt, id)
	return err
}

// UpdateTaskResult 更新任务结果。
func (r *ExecutionRepository) UpdateTaskResult(ctx context.Context, id int64, result json.RawMessage) error {
	_, err := r.db.ExecContext(ctx, `
		update execution_tasks
		set result = $1, updated_at = now()
		where id = $2
	`, result, id)
	return err
}

// CreateLog 创建执行日志。
func (r *ExecutionRepository) CreateLog(ctx context.Context, taskID int64, level, message string) error {
	_, err := r.db.ExecContext(ctx, `
		insert into execution_logs(task_id, level, message)
		values($1, $2, $3)
	`, taskID, level, message)
	return err
}

// ListLogs 查询任务日志。
func (r *ExecutionRepository) ListLogs(ctx context.Context, taskID int64) ([]model.ExecutionLog, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id, task_id, level, message, created_at
		from execution_logs
		where task_id = $1
		order by id asc
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []model.ExecutionLog{}
	for rows.Next() {
		var item model.ExecutionLog
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Level, &item.Message, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetTaskRunIDScoped 校验任务所属批次对当前用户可见，历史 NULL 仅管理员可见。
func (r *ExecutionRepository) GetTaskRunIDScoped(ctx context.Context, userID int64, admin bool, taskID int64) (int64, error) {
	var runID int64
	err := r.db.QueryRowContext(ctx, `
		select t.run_id
		from execution_tasks t
		join execution_runs er on er.id = t.run_id
		where t.id = $1
		  and ($3 or (er.project_id is not null and exists(
			select 1 from project_members pm
			where pm.user_id = $2 and pm.project_id = er.project_id
		  )))
	`, taskID, userID, admin).Scan(&runID)
	return runID, err
}

// ListLogsScoped 只返回可见执行批次下的任务日志。
func (r *ExecutionRepository) ListLogsScoped(ctx context.Context, userID int64, admin bool, taskID int64) ([]model.ExecutionLog, error) {
	rows, err := r.db.QueryContext(ctx, `
		select l.id, l.task_id, l.level, l.message, l.created_at
		from execution_logs l
		join execution_tasks t on t.id = l.task_id
		join execution_runs er on er.id = t.run_id
		where l.task_id = $1
		  and ($3 or (er.project_id is not null and exists(
			select 1 from project_members pm
			where pm.user_id = $2 and pm.project_id = er.project_id
		  )))
		order by l.id asc
	`, taskID, userID, admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.ExecutionLog{}
	for rows.Next() {
		var item model.ExecutionLog
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Level, &item.Message, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type executionRunScanner interface {
	Scan(dest ...any) error
}

func scanExecutionRun(scanner executionRunScanner) (model.ExecutionRun, error) {
	var item model.ExecutionRun
	var caseIDsRaw []byte
	var startedAt, finishedAt sql.NullTime
	var projectID sql.NullInt64
	err := scanner.Scan(
		&item.ID,
		&item.RunType,
		&item.Status,
		&item.Headless,
		&item.TriggeredBy,
		&caseIDsRaw,
		&item.Summary,
		&startedAt,
		&finishedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
		&projectID,
	)
	if err != nil {
		return model.ExecutionRun{}, err
	}
	if startedAt.Valid {
		item.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		item.FinishedAt = &finishedAt.Time
	}
	if projectID.Valid {
		item.ProjectID = &projectID.Int64
	}
	if len(caseIDsRaw) > 0 {
		if err := json.Unmarshal(caseIDsRaw, &item.CaseIDs); err != nil {
			arrayText := strings.TrimSpace(string(caseIDsRaw))
			arrayText = strings.TrimPrefix(strings.TrimSuffix(arrayText, "}"), "{")
			if arrayText != "" {
				for _, value := range strings.Split(arrayText, ",") {
					id, parseErr := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
					if parseErr == nil {
						item.CaseIDs = append(item.CaseIDs, id)
					}
				}
			}
		}
		// 兼容旧版本将 JSON 字节误写入 bigint[] 的历史数据。
		if len(item.CaseIDs) >= 2 && item.CaseIDs[0] == '[' && item.CaseIDs[len(item.CaseIDs)-1] == ']' {
			legacyJSON := make([]byte, len(item.CaseIDs))
			for index, value := range item.CaseIDs {
				legacyJSON[index] = byte(value)
			}
			var decoded []int64
			if json.Unmarshal(legacyJSON, &decoded) == nil {
				item.CaseIDs = decoded
			}
		}
	}
	if item.Summary == nil {
		item.Summary = []byte("{}")
	}
	return item, nil
}

type executionTaskScanner interface {
	Scan(dest ...any) error
}

func scanExecutionTask(scanner executionTaskScanner) (model.ExecutionTask, error) {
	var item model.ExecutionTask
	var startedAt, finishedAt sql.NullTime
	err := scanner.Scan(
		&item.ID,
		&item.RunID,
		&item.TaskID,
		&item.CaseID,
		&item.ExecutorID,
		&item.TaskType,
		&item.Payload,
		&item.CallbackURL,
		&item.Status,
		&item.Result,
		&startedAt,
		&finishedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return model.ExecutionTask{}, err
	}
	if startedAt.Valid {
		item.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		item.FinishedAt = &finishedAt.Time
	}
	if item.Payload == nil {
		item.Payload = []byte("{}")
	}
	if item.Result == nil {
		item.Result = []byte("{}")
	}
	return item, nil
}
