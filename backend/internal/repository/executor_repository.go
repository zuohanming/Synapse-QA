package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"synapseqa/backend/internal/model"
)

// ExecutorRepository 只负责执行器注册和心跳状态的数据读写。
type ExecutorRepository struct {
	db *sql.DB
}

func NewExecutorRepository(db *sql.DB) *ExecutorRepository {
	return &ExecutorRepository{db: db}
}

func (r *ExecutorRepository) UpsertRegister(ctx context.Context, req model.ExecutorRegisterRequest) (model.ExecutorView, error) {
	checksJSON := []byte(`{}`)
	supportedTypes := strings.Join(req.SupportedTypes, ",")
	row := r.db.QueryRowContext(ctx, `
		insert into executors(executor_id, name, endpoint, status, version, max_workers, running_tasks, queued_tasks, supported_types, checks, last_heartbeat_at)
		values($1, $2, $3, 'registered', $4, $5, 0, 0, $6, $7, now())
		on conflict(executor_id) do update set
			name = excluded.name,
			endpoint = excluded.endpoint,
			status = 'registered',
			version = excluded.version,
			max_workers = excluded.max_workers,
			supported_types = excluded.supported_types,
			checks = excluded.checks,
			last_heartbeat_at = now(),
			updated_at = now()
		returning executor_id, name, endpoint, status, version, max_workers, running_tasks, queued_tasks, supported_types, checks, last_heartbeat_at, updated_at, created_at
	`, req.ExecutorID, req.Name, req.Endpoint, req.Version, req.MaxWorkers, supportedTypes, checksJSON)
	return scanExecutor(row)
}

func (r *ExecutorRepository) UpsertHeartbeat(ctx context.Context, req model.ExecutorHeartbeatRequest) (model.ExecutorView, error) {
	checksJSON, err := json.Marshal(req.Checks)
	if err != nil {
		return model.ExecutorView{}, err
	}
	supportedTypes := strings.Join(req.SupportedTypes, ",")
	row := r.db.QueryRowContext(ctx, `
		insert into executors(executor_id, name, endpoint, status, version, max_workers, running_tasks, queued_tasks, supported_types, checks, last_heartbeat_at)
		values($1, $1, '', $2, $3, $4, $5, $6, $7, $8, now())
		on conflict(executor_id) do update set
			status = excluded.status,
			version = excluded.version,
			max_workers = excluded.max_workers,
			running_tasks = excluded.running_tasks,
			queued_tasks = excluded.queued_tasks,
			supported_types = excluded.supported_types,
			checks = excluded.checks,
			last_heartbeat_at = now(),
			updated_at = now()
		returning executor_id, name, endpoint, status, version, max_workers, running_tasks, queued_tasks, supported_types, checks, last_heartbeat_at, updated_at, created_at
	`, req.ExecutorID, req.Status, req.Version, req.MaxWorkers, req.RunningTasks, req.QueuedTasks, supportedTypes, checksJSON)
	return scanExecutor(row)
}

func (r *ExecutorRepository) List(ctx context.Context) ([]model.ExecutorView, error) {
	rows, err := r.db.QueryContext(ctx, `
		select executor_id, name, endpoint, status, version, max_workers, running_tasks, queued_tasks, supported_types, checks, last_heartbeat_at, updated_at, created_at
		from executors
		order by last_heartbeat_at desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.ExecutorView
	for rows.Next() {
		item, err := scanExecutor(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ExecutorRepository) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `select value from platform_settings where key = $1`, key).Scan(&value)
	return value, err
}

type executorScanner interface {
	Scan(dest ...any) error
}

func scanExecutor(scanner executorScanner) (model.ExecutorView, error) {
	var item model.ExecutorView
	var supportedTypes string
	var checksRaw []byte
	if err := scanner.Scan(
		&item.ExecutorID,
		&item.Name,
		&item.Endpoint,
		&item.Status,
		&item.Version,
		&item.MaxWorkers,
		&item.RunningTasks,
		&item.QueuedTasks,
		&supportedTypes,
		&checksRaw,
		&item.LastHeartbeatAt,
		&item.UpdatedAt,
		&item.CreatedAt,
	); err != nil {
		return model.ExecutorView{}, err
	}
	if supportedTypes != "" {
		item.SupportedTypes = strings.Split(supportedTypes, ",")
	}
	if len(checksRaw) > 0 {
		_ = json.Unmarshal(checksRaw, &item.Checks)
	}
	if item.Checks == nil {
		item.Checks = map[string]bool{}
	}
	return item, nil
}
