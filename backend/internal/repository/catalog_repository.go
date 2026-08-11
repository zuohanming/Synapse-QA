package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

type CatalogRepository struct {
	db *sql.DB
}

func NewCatalogRepository(db *sql.DB) *CatalogRepository {
	return &CatalogRepository{db: db}
}

func (r *CatalogRepository) ListMenus(ctx context.Context) ([]model.Menu, error) {
	rows, err := r.db.QueryContext(ctx, `select id, parent_id, title, code, sort_order from system_menus order by sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var menus []model.Menu
	for rows.Next() {
		var item model.Menu
		var parentID sql.NullInt64
		if err := rows.Scan(&item.ID, &parentID, &item.Title, &item.Code, &item.SortOrder); err != nil {
			return nil, err
		}
		if parentID.Valid {
			value := parentID.Int64
			item.ParentID = &value
		}
		menus = append(menus, item)
	}
	return menus, rows.Err()
}

func (r *CatalogRepository) ListDictionaries(ctx context.Context) ([]model.Dictionary, error) {
	rows, err := r.db.QueryContext(ctx, `select id, name, code, item_key, item_value, enabled from dictionaries order by code, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dicts []model.Dictionary
	for rows.Next() {
		var item model.Dictionary
		if err := rows.Scan(&item.ID, &item.Name, &item.Code, &item.Key, &item.Value, &item.Enabled); err != nil {
			return nil, err
		}
		dicts = append(dicts, item)
	}
	return dicts, rows.Err()
}

func (r *CatalogRepository) ListOperationLogs(ctx context.Context) ([]model.OperationLog, error) {
	rows, err := r.db.QueryContext(ctx, `select id, actor, action, target, ip, created_at from operation_logs order by id desc limit 30`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []model.OperationLog
	for rows.Next() {
		var item model.OperationLog
		if err := rows.Scan(&item.ID, &item.Actor, &item.Action, &item.Target, &item.IP, &item.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, item)
	}
	return logs, rows.Err()
}

func (r *CatalogRepository) ListProjects(ctx context.Context, idFilter, name string, page, pageSize int) ([]model.Project, int64, error) {
	where := []string{"deleted_at is null"}
	args := []any{}
	if idFilter != "" {
		id, err := strconv.ParseInt(idFilter, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("id = $%d", len(args)))
	}
	if name != "" {
		args = append(args, "%"+strings.ToLower(name)+"%")
		where = append(where, fmt.Sprintf("lower(name) like $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	total, err := r.count(ctx, "projects", whereSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select id, name, status, created_at, updated_at
		from projects
		where `+whereSQL+`
		order by id asc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []model.Project
	for rows.Next() {
		var item model.Project
		if err := rows.Scan(&item.ID, &item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *CatalogRepository) CreateProject(ctx context.Context, req model.ProjectRequest) error {
	_, err := r.db.ExecContext(ctx, `insert into projects(name, status) values($1, $2)`, req.Name, req.Status)
	return err
}

func (r *CatalogRepository) UpdateProject(ctx context.Context, id int64, req model.ProjectRequest) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update projects set name = $1, status = $2, updated_at = now() where id = $3 and deleted_at is null`, req.Name, req.Status, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *CatalogRepository) DeleteProject(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update projects set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *CatalogRepository) ListProducts(ctx context.Context, idFilter, name string, page, pageSize int) ([]model.Product, int64, error) {
	where := []string{"p.deleted_at is null", "pr.deleted_at is null"}
	args := []any{}
	if idFilter != "" {
		id, err := strconv.ParseInt(idFilter, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("p.id = $%d", len(args)))
	}
	if name != "" {
		args = append(args, "%"+strings.ToLower(name)+"%")
		where = append(where, fmt.Sprintf("lower(p.name) like $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from products p join projects pr on pr.id = p.project_id where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select p.id, p.project_id, pr.name, p.code, p.name, p.owner, p.status, p.ui_type, p.api_type, p.created_at, p.updated_at
		from products p join projects pr on pr.id = p.project_id
		where `+whereSQL+`
		order by p.id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []model.Product
	for rows.Next() {
		var item model.Product
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.ProjectName, &item.Code, &item.Name, &item.Owner, &item.Status, &item.UIType, &item.APIType, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *CatalogRepository) CreateProduct(ctx context.Context, req model.ProductRequest) error {
	_, err := r.db.ExecContext(ctx, `insert into products(project_id, code, name, owner, status, ui_type, api_type) values($1, $2, $3, $4, $5, $6, $7)`, req.ProjectID, req.Code, req.Name, req.Owner, req.Status, req.UIType, req.APIType)
	return err
}

func (r *CatalogRepository) UpdateProduct(ctx context.Context, id int64, req model.ProductRequest) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update products set project_id = $1, name = $2, owner = $3, status = $4, ui_type = $5, api_type = $6, updated_at = now() where id = $7 and deleted_at is null`, req.ProjectID, req.Name, req.Owner, req.Status, req.UIType, req.APIType, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *CatalogRepository) DeleteProduct(ctx context.Context, id int64) (int64, error) {
	var used bool
	if err := r.db.QueryRowContext(ctx, `select exists(select 1 from product_modules where product_id=$1 and deleted_at is null) or exists(select 1 from test_objects where product_id=$1 and deleted_at is null) or exists(select 1 from test_cases where product_id=$1 and deleted_at is null)`, id).Scan(&used); err != nil || used {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, `update products set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *CatalogRepository) ProductStats(ctx context.Context, id int64) (model.ProductStats, error) {
	var item model.ProductStats
	err := r.db.QueryRowContext(ctx, `select
	(select count(*) from product_modules where product_id=$1 and deleted_at is null),
	(select count(*) from test_objects where product_id=$1 and deleted_at is null),
	(select count(*) from ui_assets where product_id=$1 and asset_type='page' and deleted_at is null),
	(select count(*) from api_interfaces where product_id=$1 and deleted_at is null),
	(select count(*) from test_cases where product_id=$1 and deleted_at is null),
	(select count(*) from api_test_cases where product_id=$1 and deleted_at is null)`, id).Scan(&item.Modules, &item.Environments, &item.Pages, &item.Interfaces, &item.UICases, &item.APICases)
	return item, err
}

func (r *CatalogRepository) CopyProduct(ctx context.Context, id int64, req model.ProductCopyRequest) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var projectID int64
	var owner, uiType, apiType string
	if err = tx.QueryRowContext(ctx, `select project_id,owner,ui_type,api_type from products where id=$1 and deleted_at is null`, id).Scan(&projectID, &owner, &uiType, &apiType); err != nil {
		return err
	}
	var newID int64
	if err = tx.QueryRowContext(ctx, `insert into products(project_id,code,name,owner,status,ui_type,api_type) values($1,$2,$3,$4,'active',$5,$6) returning id`, projectID, req.Code, req.Name, owner, uiType, apiType).Scan(&newID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `insert into product_modules(product_id,name,level1,level2) select $1,name,level1,level2 from product_modules where product_id=$2 and deleted_at is null`, newID, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *CatalogRepository) ListProductModules(ctx context.Context, productID int64, level1, level2, name string, page, pageSize int) ([]model.ProductModule, int64, error) {
	where := []string{"deleted_at is null", "product_id = $1"}
	args := []any{productID}
	if level1 != "" {
		args = append(args, level1)
		where = append(where, fmt.Sprintf("level1 = $%d", len(args)))
	}
	if level2 != "" {
		args = append(args, level2)
		where = append(where, fmt.Sprintf("level2 = $%d", len(args)))
	}
	if name != "" {
		args = append(args, "%"+strings.ToLower(name)+"%")
		where = append(where, fmt.Sprintf("lower(name) like $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from product_modules where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select id, product_id, name, level1, level2, created_at, updated_at
		from product_modules
		where `+whereSQL+`
		order by id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []model.ProductModule
	for rows.Next() {
		var item model.ProductModule
		if err := rows.Scan(&item.ID, &item.ProductID, &item.Name, &item.Level1, &item.Level2, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *CatalogRepository) CreateProductModule(ctx context.Context, req model.ProductModuleRequest) error {
	_, err := r.db.ExecContext(ctx, `insert into product_modules(product_id, name, level1, level2) values($1, $2, $3, $4)`, req.ProductID, req.Name, req.Level1, req.Level2)
	return err
}

func (r *CatalogRepository) UpdateProductModule(ctx context.Context, id int64, req model.ProductModuleRequest) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update product_modules set name = $1, level1 = $2, level2 = $3, updated_at = now() where id = $4 and product_id = $5 and deleted_at is null`, req.Name, req.Level1, req.Level2, id, req.ProductID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *CatalogRepository) DeleteProductModule(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update product_modules set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *CatalogRepository) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `select value from platform_settings where key = $1`, key).Scan(&value)
	return value, err
}

func (r *CatalogRepository) GetSettingWithUpdatedAt(ctx context.Context, key string) (string, time.Time, error) {
	var value string
	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx, `select value, updated_at from platform_settings where key = $1`, key).Scan(&value, &updatedAt)
	return value, updatedAt, err
}

func (r *CatalogRepository) UpsertSetting(ctx context.Context, key, value string) (time.Time, error) {
	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx, `
		insert into platform_settings(key, value)
		values($1, $2)
		on conflict(key) do update set value = excluded.value, updated_at = now()
		returning updated_at
	`, key, value).Scan(&updatedAt)
	return updatedAt, err
}

func (r *CatalogRepository) count(ctx context.Context, table, whereSQL string, args ...any) (int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx, "select count(*) from "+table+" where "+whereSQL, args...).Scan(&total)
	return total, err
}
