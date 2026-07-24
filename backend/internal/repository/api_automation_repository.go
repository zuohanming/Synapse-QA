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

func (r *APIAutomationRepository) ListInterfaces(ctx context.Context, userID int64, filter model.APIInterfaceFilter, page, pageSize int) ([]model.APIInterface, int64, error) {
	where := []string{"i.deleted_at is null", "p.deleted_at is null", "pr.deleted_at is null", `(exists(select 1 from users u join roles ro on ro.id=u.role_id where u.id=$1 and ro.code='admin') or exists(select 1 from project_members pm where pm.user_id=$1 and pm.project_id=pr.id))`}
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
			i.last_debug_at, i.created_by, i.updated_by, i.created_at, i.updated_at
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
		if err := rows.Scan(&item.ID, &item.ProductID, &item.ProjectID, &item.ProjectName, &item.ProductName, &item.ModuleID, &item.ModuleName,
			&item.Name, &item.Method, &item.Path, &item.Protocol, &item.EndpointType, &item.LifecycleStatus, &item.TimeoutSeconds,
			&item.FollowRedirects, &item.Configuration, &item.Revision, &item.LastDebugStatus, &duration, &debugAt,
			&item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if duration.Valid {
			item.LastDebugDurationMS = &duration.Int64
		}
		if debugAt.Valid {
			item.LastDebugAt = &debugAt.Time
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
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
