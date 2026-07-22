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
		returning id, run_type, status, headless, triggered_by, case_ids, summary, started_at, finished_at, created_at, updated_at
	`, req.RunType, "pending", req.Headless == nil || *req.Headless, triggeredBy, formatInt64Array(req.CaseIDs))
	return scanExecutionRun(row)
}

func formatInt64Array(values []int64) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = strconv.FormatInt(value, 10)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// GetRun 查询执行批次基础信息。
func (r *ExecutionRepository) GetRun(ctx context.Context, id int64) (model.ExecutionRun, error) {
	row := r.db.QueryRowContext(ctx, `
		select id, run_type, status, headless, triggered_by, case_ids, summary, started_at, finished_at, created_at, updated_at
		from execution_runs
		where id = $1
	`, id)
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
		select id, run_type, status, headless, triggered_by, case_ids, summary, started_at, finished_at, created_at, updated_at
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

type executionRunScanner interface {
	Scan(dest ...any) error
}

func scanExecutionRun(scanner executionRunScanner) (model.ExecutionRun, error) {
	var item model.ExecutionRun
	var caseIDsRaw []byte
	var startedAt, finishedAt sql.NullTime
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
