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

type APIAutomationRepository struct {
	db *sql.DB
}

func NewAPIAutomationRepository(db *sql.DB) *APIAutomationRepository {
	return &APIAutomationRepository{db: db}
}

func (r *APIAutomationRepository) CanAccessProject(ctx context.Context, userID, projectID int64) bool {
	var allowed bool
	_ = r.db.QueryRowContext(ctx, `
		select exists(
			select 1 from users u left join roles ro on ro.id = u.role_id
			where u.id = $1 and u.deleted_at is null
			and (ro.code = 'admin' or exists(
				select 1 from project_members pm where pm.user_id = u.id and pm.project_id = $2
			))
		)
	`, userID, projectID).Scan(&allowed)
	return allowed
}

func (r *APIAutomationRepository) ProductProject(ctx context.Context, productID int64) (int64, error) {
	var projectID int64
	err := r.db.QueryRowContext(ctx, `select project_id from products where id = $1 and deleted_at is null`, productID).Scan(&projectID)
	return projectID, err
}

func (r *APIAutomationRepository) TestObject(ctx context.Context, id, productID int64) (model.TestObject, error) {
	var item model.TestObject
	err := r.db.QueryRowContext(ctx, `
		select id, product_id, env_name, target, deploy_env, auto_type, owner,
		       query_enabled, write_enabled, created_at, updated_at
		from test_objects
		where id = $1 and product_id = $2 and deleted_at is null
	`, id, productID).Scan(
		&item.ID, &item.ProductID, &item.EnvName, &item.Target, &item.DeployEnv,
		&item.AutoType, &item.Owner, &item.QueryEnabled, &item.WriteEnabled,
		&item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func (r *APIAutomationRepository) GlobalVariables(ctx context.Context, productID int64, envName string) ([]model.UIAsset, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id, asset_type, name, category, method, locator, action, value,
		       description, status, created_by, created_at, updated_at
		from ui_assets
		where asset_type = 'global_variable'
		  and deleted_at is null
		  and status = 'active'
		  and action = $1
		  and (method = 'project' or (method = 'environment' and locator = $2))
		order by case when method = 'project' then 0 else 1 end, id
	`, strconv.FormatInt(productID, 10), envName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.UIAsset
	for rows.Next() {
		var item model.UIAsset
		if err := rows.Scan(
			&item.ID, &item.AssetType, &item.Name, &item.Category, &item.Method,
			&item.Locator, &item.Action, &item.Value, &item.Description, &item.Status,
			&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APIAutomationRepository) CreateTempFile(ctx context.Context, item model.APITempFile, ownerUserID int64, storedPath string) error {
	_, err := r.db.ExecContext(ctx, `
		insert into api_temp_files(id, project_id, owner_user_id, original_name, stored_path, mime_type, size_bytes, sha256, expires_at)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, item.ID, item.ProjectID, ownerUserID, item.OriginalName, storedPath, item.MIMEType, item.SizeBytes, item.SHA256, item.ExpiresAt)
	return err
}

func (r *APIAutomationRepository) DeleteTempFile(ctx context.Context, id string, userID int64) (string, int64, error) {
	var path string
	var projectID int64
	err := r.db.QueryRowContext(ctx, `
		update api_temp_files
		set deleted_at = now()
		where id = $1 and owner_user_id = $2 and deleted_at is null and expires_at > now()
		returning stored_path, project_id
	`, id, userID).Scan(&path, &projectID)
	return path, projectID, err
}

func (r *APIAutomationRepository) ExpiredTempFiles(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		update api_temp_files set deleted_at = now()
		where deleted_at is null and expires_at <= now()
		returning stored_path
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

func (r *APIAutomationRepository) CreateDebugRun(ctx context.Context, taskID string, interfaceID, projectID int64, executorID, actor string, request json.RawMessage) (model.APIDebugRun, error) {
	row := r.db.QueryRowContext(ctx, `
		insert into api_debug_runs(task_id, interface_id, project_id, executor_id, request_snapshot, triggered_by)
		values($1, $2, $3, $4, $5, $6)
		returning id, task_id, interface_id, project_id, executor_id, status, request_snapshot,
		          result, error_message, triggered_by, started_at, finished_at, created_at, updated_at
	`, taskID, interfaceID, projectID, executorID, request, actor)
	return scanAPIDebugRun(row)
}

func (r *APIAutomationRepository) GetDebugRun(ctx context.Context, taskID string) (model.APIDebugRun, error) {
	row := r.db.QueryRowContext(ctx, `
		select id, task_id, interface_id, project_id, executor_id, status, request_snapshot,
		       result, error_message, triggered_by, started_at, finished_at, created_at, updated_at
		from api_debug_runs where task_id = $1
	`, taskID)
	return scanAPIDebugRun(row)
}

func (r *APIAutomationRepository) UpdateDebugRun(ctx context.Context, taskID, status string, result json.RawMessage, errorMessage string) error {
	_, err := r.db.ExecContext(ctx, `
		update api_debug_runs
		set status = $2,
		    result = $3,
		    error_message = $4,
		    started_at = case when $2 = 'running' then coalesce(started_at, now()) else started_at end,
		    finished_at = case when $2 in ('success','failed','canceled') then coalesce(finished_at, now()) else finished_at end,
		    updated_at = now()
		where task_id = $1
	`, taskID, status, result, errorMessage)
	return err
}

func (r *APIAutomationRepository) UpdateInterfaceDebugSummary(ctx context.Context, interfaceID int64, status string, durationMS int64) error {
	_, err := r.db.ExecContext(ctx, `
		update api_interfaces
		set last_debug_status = $2, last_debug_duration_ms = $3, last_debug_at = now()
		where id = $1
	`, interfaceID, status, durationMS)
	return err
}

func (r *APIAutomationRepository) CreateDebugEvent(ctx context.Context, event model.APIDebugEvent) error {
	_, err := r.db.ExecContext(ctx, `
		insert into api_debug_events(task_id, sequence, event_type, stage, status, message, progress, data)
		values($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict(task_id, sequence) do nothing
	`, event.TaskID, event.Sequence, event.Type, event.Stage, event.Status, event.Message, event.Progress, event.Data)
	return err
}

func (r *APIAutomationRepository) ListDebugEvents(ctx context.Context, taskID string, after int) ([]model.APIDebugEvent, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id, task_id, sequence, event_type, stage, status, message, progress, data, created_at
		from api_debug_events
		where task_id = $1 and sequence > $2
		order by sequence
	`, taskID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.APIDebugEvent
	for rows.Next() {
		var item model.APIDebugEvent
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Sequence, &item.Type, &item.Stage, &item.Status, &item.Message, &item.Progress, &item.Data, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanAPIDebugRun(scanner interface{ Scan(...any) error }) (model.APIDebugRun, error) {
	var item model.APIDebugRun
	err := scanner.Scan(
		&item.ID, &item.TaskID, &item.InterfaceID, &item.ProjectID, &item.ExecutorID,
		&item.Status, &item.Request, &item.Result, &item.ErrorMessage, &item.TriggeredBy,
		&item.StartedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func (r *APIAutomationRepository) ListDebugRuns(ctx context.Context, interfaceID int64, filter model.APIDebugRunFilter) ([]model.APIDebugRun, error) {
	where := []string{"interface_id = $1"}
	args := []any{interfaceID}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.ExecutorID != "" {
		args = append(args, filter.ExecutorID)
		where = append(where, fmt.Sprintf("executor_id = $%d", len(args)))
	}
	if filter.DateFrom != nil {
		args = append(args, *filter.DateFrom)
		where = append(where, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if filter.DateTo != nil {
		args = append(args, *filter.DateTo)
		where = append(where, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 20
	}
	args = append(args, filter.Limit)
	rows, err := r.db.QueryContext(ctx, `
		select id, task_id, interface_id, project_id, executor_id, status, request_snapshot,
		       result, error_message, triggered_by, started_at, finished_at, created_at, updated_at
		from api_debug_runs
		where `+strings.Join(where, " and ")+`
		order by id desc
		limit $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.APIDebugRun
	for rows.Next() {
		item, err := scanAPIDebugRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APIAutomationRepository) GetDebugRunByID(ctx context.Context, id int64) (model.APIDebugRun, error) {
	row := r.db.QueryRowContext(ctx, `
		select id, task_id, interface_id, project_id, executor_id, status, request_snapshot,
		       result, error_message, triggered_by, started_at, finished_at, created_at, updated_at
		from api_debug_runs where id = $1
	`, id)
	return scanAPIDebugRun(row)
}

func (r *APIAutomationRepository) ReplaceDebugAssertions(ctx context.Context, runID int64, items []model.APIDebugAssertion) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `delete from api_debug_assertions where debug_run_id = $1`, runID); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `
			insert into api_debug_assertions(
				debug_run_id, assertion_index, assertion_type, expression, operator,
				expected_value, actual_value, passed, error_message
			) values($1,$2,$3,$4,$5,$6,$7,$8,$9)
		`, runID, item.Index, item.Type, item.Expression, item.Operator, item.Expected, item.Actual, item.Passed, item.ErrorMessage); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *APIAutomationRepository) ListDebugAssertions(ctx context.Context, runID int64) ([]model.APIDebugAssertion, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id, debug_run_id, assertion_index, assertion_type, expression, operator,
		       expected_value, actual_value, passed, error_message, created_at
		from api_debug_assertions
		where debug_run_id = $1
		order by assertion_index
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.APIDebugAssertion
	for rows.Next() {
		var item model.APIDebugAssertion
		if err := rows.Scan(
			&item.ID, &item.DebugRunID, &item.Index, &item.Type, &item.Expression,
			&item.Operator, &item.Expected, &item.Actual, &item.Passed,
			&item.ErrorMessage, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APIAutomationRepository) ListInterfaces(ctx context.Context, userID int64, filter model.APIInterfaceFilter, page, pageSize int) ([]model.APIInterface, int64, error) {
	where := []string{"p.deleted_at is null", "pr.deleted_at is null", `(exists(select 1 from users u join roles ro on ro.id=u.role_id where u.id=$1 and ro.code='admin') or exists(select 1 from project_members pm where pm.user_id=$1 and pm.project_id=pr.id))`}
	if filter.DeletedOnly {
		where = append(where, "i.deleted_at is not null")
	} else if !filter.IncludeDeleted {
		where = append(where, "i.deleted_at is null")
	}
	args := []any{userID}
	addID := func(column, value string) error {
		if value == "" {
			return nil
		}
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("%s = $%d", column, len(args)))
		return nil
	}
	if err := addID("pr.id", filter.ProjectID); err != nil {
		return nil, 0, err
	}
	if err := addID("p.id", filter.ProductID); err != nil {
		return nil, 0, err
	}
	if err := addID("i.module_id", filter.ModuleID); err != nil {
		return nil, 0, err
	}
	if filter.Keyword != "" {
		args = append(args, "%"+strings.ToLower(filter.Keyword)+"%")
		where = append(where, fmt.Sprintf("(lower(i.name) like $%d or lower(i.path) like $%d)", len(args), len(args)))
	}
	if filter.Method != "" {
		args = append(args, strings.ToUpper(filter.Method))
		where = append(where, fmt.Sprintf("i.method = $%d", len(args)))
	}
	if filter.LifecycleStatus != "" {
		args = append(args, filter.LifecycleStatus)
		where = append(where, fmt.Sprintf("i.lifecycle_status = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `select count(*) from api_interfaces i join products p on p.id=i.product_id join projects pr on pr.id=p.project_id where `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select i.id, i.product_id, pr.id, pr.name, p.name, coalesce(i.module_id,0), coalesce(pm.name,''),
			i.name, i.method, i.path, i.protocol, i.endpoint_type, i.lifecycle_status, i.timeout_seconds,
			i.follow_redirects, i.configuration, i.revision, i.last_debug_status, i.last_debug_duration_ms,
			i.last_debug_at, i.created_by, i.updated_by, i.created_at, i.updated_at, i.deleted_at
		from api_interfaces i
		join products p on p.id=i.product_id
		join projects pr on pr.id=p.project_id
		left join product_modules pm on pm.id=i.module_id
		where `+whereSQL+`
		order by i.updated_at desc, i.id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.APIInterface{}
	for rows.Next() {
		var item model.APIInterface
		var duration sql.NullInt64
		var debugAt sql.NullTime
		var deletedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.ProductID, &item.ProjectID, &item.ProjectName, &item.ProductName, &item.ModuleID, &item.ModuleName,
			&item.Name, &item.Method, &item.Path, &item.Protocol, &item.EndpointType, &item.LifecycleStatus, &item.TimeoutSeconds,
			&item.FollowRedirects, &item.Configuration, &item.Revision, &item.LastDebugStatus, &duration, &debugAt,
			&item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &deletedAt); err != nil {
			return nil, 0, err
		}
		if duration.Valid {
			item.LastDebugDurationMS = &duration.Int64
		}
		if debugAt.Valid {
			item.LastDebugAt = &debugAt.Time
		}
		if deletedAt.Valid {
			item.DeletedAt = &deletedAt.Time
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *APIAutomationRepository) GetInterfaceIncludingDeleted(ctx context.Context, userID, id int64) (model.APIInterface, error) {
	items, _, err := r.ListInterfaces(ctx, userID, model.APIInterfaceFilter{IncludeDeleted: true}, 1, 10000)
	if err != nil {
		return model.APIInterface{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return model.APIInterface{}, sql.ErrNoRows
}

func (r *APIAutomationRepository) GetInterface(ctx context.Context, userID, id int64) (model.APIInterface, error) {
	items, _, err := r.ListInterfaces(ctx, userID, model.APIInterfaceFilter{}, 1, 10000)
	if err != nil {
		return model.APIInterface{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return model.APIInterface{}, sql.ErrNoRows
}

func (r *APIAutomationRepository) CreateInterface(ctx context.Context, req model.APIInterfaceRequest, normalizedPath, actor string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	follow := true
	if req.FollowRedirects != nil {
		follow = *req.FollowRedirects
	}
	err = tx.QueryRowContext(ctx, `
		insert into api_interfaces(product_id,module_id,name,method,path,normalized_path,protocol,endpoint_type,lifecycle_status,timeout_seconds,follow_redirects,configuration,created_by,updated_by)
		values($1,nullif($2,0),$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13) returning id
	`, req.ProductID, req.ModuleID, req.Name, req.Method, req.Path, normalizedPath, req.Protocol, req.EndpointType, req.LifecycleStatus, req.TimeoutSeconds, follow, req.Configuration, actor).Scan(&id)
	if err != nil {
		return 0, err
	}
	snapshot, _ := json.Marshal(req)
	if _, err = tx.ExecContext(ctx, `insert into api_interface_versions(interface_id,version,snapshot,change_summary,created_by) values($1,1,$2,'创建接口',$3)`, id, snapshot, actor); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (r *APIAutomationRepository) UpdateInterface(ctx context.Context, id int64, req model.APIInterfaceRequest, normalizedPath, actor string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	follow := true
	if req.FollowRedirects != nil {
		follow = *req.FollowRedirects
	}
	var version int
	err = tx.QueryRowContext(ctx, `
		update api_interfaces set product_id=$1,module_id=nullif($2,0),name=$3,method=$4,path=$5,normalized_path=$6,
			protocol=$7,endpoint_type=$8,lifecycle_status=$9,timeout_seconds=$10,follow_redirects=$11,configuration=$12,
			current_version=current_version+1,revision=revision+1,updated_by=$13,updated_at=now()
		where id=$14 and revision=$15 and deleted_at is null returning current_version
	`, req.ProductID, req.ModuleID, req.Name, req.Method, req.Path, normalizedPath, req.Protocol, req.EndpointType, req.LifecycleStatus,
		req.TimeoutSeconds, follow, req.Configuration, actor, id, req.Revision).Scan(&version)
	if err != nil {
		return 0, err
	}
	snapshot, _ := json.Marshal(req)
	if _, err = tx.ExecContext(ctx, `insert into api_interface_versions(interface_id,version,snapshot,change_summary,created_by) values($1,$2,$3,'更新接口',$4)`, id, version, snapshot, actor); err != nil {
		return 0, err
	}
	return int64(version), tx.Commit()
}

func (r *APIAutomationRepository) DeleteInterface(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update api_interfaces set deleted_at=now(),updated_at=now() where id=$1 and deleted_at is null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *APIAutomationRepository) RestoreInterface(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update api_interfaces set deleted_at=null,updated_at=now() where id=$1 and deleted_at is not null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *APIAutomationRepository) ListInterfaceVersions(ctx context.Context, interfaceID int64) ([]model.APIInterfaceVersion, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id, interface_id, version, snapshot, change_summary, created_by, created_at
		from api_interface_versions
		where interface_id = $1
		order by version desc
	`, interfaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.APIInterfaceVersion
	for rows.Next() {
		var item model.APIInterfaceVersion
		if err := rows.Scan(&item.ID, &item.InterfaceID, &item.Version, &item.Snapshot, &item.ChangeSummary, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APIAutomationRepository) GetInterfaceVersion(ctx context.Context, interfaceID int64, version int) (model.APIInterfaceVersion, error) {
	var item model.APIInterfaceVersion
	err := r.db.QueryRowContext(ctx, `
		select id, interface_id, version, snapshot, change_summary, created_by, created_at
		from api_interface_versions
		where interface_id = $1 and version = $2
	`, interfaceID, version).Scan(
		&item.ID, &item.InterfaceID, &item.Version, &item.Snapshot,
		&item.ChangeSummary, &item.CreatedBy, &item.CreatedAt,
	)
	return item, err
}

func (r *APIAutomationRepository) RestoreInterfaceVersion(ctx context.Context, id int64, req model.APIInterfaceRequest, normalizedPath, actor string, sourceVersion int) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	follow := req.FollowRedirects == nil || *req.FollowRedirects
	var version int
	err = tx.QueryRowContext(ctx, `
		update api_interfaces set product_id=$1,module_id=nullif($2,0),name=$3,method=$4,path=$5,normalized_path=$6,
			protocol=$7,endpoint_type=$8,lifecycle_status=$9,timeout_seconds=$10,follow_redirects=$11,configuration=$12,
			current_version=current_version+1,revision=revision+1,updated_by=$13,updated_at=now()
		where id=$14 and revision=$15 and deleted_at is null returning current_version
	`, req.ProductID, req.ModuleID, req.Name, req.Method, req.Path, normalizedPath, req.Protocol, req.EndpointType,
		req.LifecycleStatus, req.TimeoutSeconds, follow, req.Configuration, actor, id, req.Revision).Scan(&version)
	if err != nil {
		return 0, err
	}
	snapshot, _ := json.Marshal(req)
	summary := fmt.Sprintf("回滚至 V%d", sourceVersion)
	if _, err = tx.ExecContext(ctx, `
		insert into api_interface_versions(interface_id,version,snapshot,change_summary,created_by)
		values($1,$2,$3,$4,$5)
	`, id, version, snapshot, summary, actor); err != nil {
		return 0, err
	}
	return version, tx.Commit()
}

func (r *APIAutomationRepository) ListProjectHeaders(ctx context.Context, projectID int64, keyword string) ([]model.APIProjectHeader, error) {
	args := []any{projectID}
	where := "h.project_id=$1 and h.deleted_at is null"
	if keyword != "" {
		args = append(args, "%"+strings.ToLower(keyword)+"%")
		where += fmt.Sprintf(" and lower(h.header_name) like $%d", len(args))
	}
	rows, err := r.db.QueryContext(ctx, `
		select h.id,h.project_id,p.name,h.header_name,h.header_value,h.description,h.enabled,h.sensitive,h.created_by,h.updated_by,h.created_at,h.updated_at
		from api_project_headers h join projects p on p.id=h.project_id
		where `+where+` order by h.header_name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.APIProjectHeader{}
	for rows.Next() {
		var item model.APIProjectHeader
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.ProjectName, &item.Name, &item.Value, &item.Description, &item.Enabled, &item.Sensitive, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APIAutomationRepository) HeaderProject(ctx context.Context, id int64) (int64, error) {
	var projectID int64
	err := r.db.QueryRowContext(ctx, `select project_id from api_project_headers where id=$1 and deleted_at is null`, id).Scan(&projectID)
	return projectID, err
}

func (r *APIAutomationRepository) CreateProjectHeader(ctx context.Context, req model.APIProjectHeaderRequest, actor string) error {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	_, err := r.db.ExecContext(ctx, `insert into api_project_headers(project_id,header_name,header_name_normalized,header_value,description,enabled,sensitive,created_by,updated_by) values($1,$2,$3,$4,$5,$6,$7,$8,$8)`,
		req.ProjectID, req.Name, strings.ToLower(req.Name), req.Value, req.Description, enabled, req.Sensitive, actor)
	return err
}

func (r *APIAutomationRepository) UpdateProjectHeader(ctx context.Context, id int64, req model.APIProjectHeaderRequest, actor string) (int64, error) {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	result, err := r.db.ExecContext(ctx, `update api_project_headers set project_id=$1,header_name=$2,header_name_normalized=$3,header_value=$4,description=$5,enabled=$6,sensitive=$7,updated_by=$8,updated_at=now() where id=$9 and deleted_at is null`,
		req.ProjectID, req.Name, strings.ToLower(req.Name), req.Value, req.Description, enabled, req.Sensitive, actor, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *APIAutomationRepository) DeleteProjectHeader(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update api_project_headers set deleted_at=now(),updated_at=now() where id=$1 and deleted_at is null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
