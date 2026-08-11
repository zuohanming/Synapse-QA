package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"synapseqa/backend/internal/model"
)

type AutomationRepository struct {
	db *sql.DB
}

func NewAutomationRepository(db *sql.DB) *AutomationRepository {
	return &AutomationRepository{db: db}
}

func (r *AutomationRepository) ListTestObjects(ctx context.Context, idFilter, envName, productID string, page, pageSize int) ([]model.TestObject, int64, error) {
	where := []string{"t.deleted_at is null", "p.deleted_at is null", "pr.deleted_at is null"}
	args := []any{}
	if idFilter != "" {
		id, err := strconv.ParseInt(idFilter, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("t.id = $%d", len(args)))
	}
	if envName != "" {
		args = append(args, "%"+strings.ToLower(envName)+"%")
		where = append(where, fmt.Sprintf("lower(t.env_name) like $%d", len(args)))
	}
	if productID != "" {
		id, err := strconv.ParseInt(productID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("t.product_id = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `select count(*) from test_objects t join products p on p.id = t.product_id join projects pr on pr.id = p.project_id where `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select t.id, t.product_id, pr.name || '/' || p.name, t.env_name, t.target, t.deploy_env, t.auto_type, t.owner, t.query_enabled, t.write_enabled, t.created_at, t.updated_at
		from test_objects t join products p on p.id = t.product_id join projects pr on pr.id = p.project_id
		where `+whereSQL+`
		order by t.id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []model.TestObject
	for rows.Next() {
		var item model.TestObject
		if err := rows.Scan(&item.ID, &item.ProductID, &item.ProductName, &item.EnvName, &item.Target, &item.DeployEnv, &item.AutoType, &item.Owner, &item.QueryEnabled, &item.WriteEnabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *AutomationRepository) CreateTestObject(ctx context.Context, req model.TestObjectRequest) error {
	_, err := r.db.ExecContext(ctx, `insert into test_objects(product_id, env_name, target, deploy_env, auto_type, owner, query_enabled, write_enabled) select id,$2,$3,$4,$5,$6,$7,$8 from products where id=$1 and deleted_at is null and status='active'`, req.ProductID, req.EnvName, req.Target, req.DeployEnv, req.AutoType, req.Owner, req.QueryEnabled, req.WriteEnabled)
	return err
}

func (r *AutomationRepository) UpdateTestObject(ctx context.Context, id int64, req model.TestObjectRequest) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update test_objects set product_id = $1, env_name = $2, target = $3, deploy_env = $4, auto_type = $5, owner = $6, query_enabled = $7, write_enabled = $8, updated_at = now() where id = $9 and deleted_at is null`, req.ProductID, req.EnvName, req.Target, req.DeployEnv, req.AutoType, req.Owner, req.QueryEnabled, req.WriteEnabled, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *AutomationRepository) DeleteTestObject(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update test_objects set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *AutomationRepository) ListUIAssets(ctx context.Context, assetType string, filter model.UIAssetFilter, page, pageSize int) ([]model.UIAsset, int64, error) {
	where := []string{"asset_type = $1", "deleted_at is null"}
	args := []any{assetType}
	if filter.Keyword != "" {
		args = append(args, "%"+strings.ToLower(filter.Keyword)+"%")
		where = append(where, fmt.Sprintf("(lower(name) like $%d or lower(category) like $%d or lower(method) like $%d or lower(locator) like $%d or lower(description) like $%d)", len(args), len(args), len(args), len(args), len(args)))
	}
	if filter.ID != "" {
		id, err := strconv.ParseInt(filter.ID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("id = $%d", len(args)))
	}
	if filter.PageName != "" {
		args = append(args, "%"+strings.ToLower(filter.PageName)+"%")
		where = append(where, fmt.Sprintf("lower(name) like $%d", len(args)))
	}
	if filter.PageURL != "" {
		args = append(args, "%"+strings.ToLower(filter.PageURL)+"%")
		where = append(where, fmt.Sprintf("lower(locator) like $%d", len(args)))
	}
	if filter.Product != "" {
		args = append(args, filter.Product)
		where = append(where, fmt.Sprintf("category = $%d", len(args)))
	}
	if filter.Module != "" {
		args = append(args, filter.Module)
		where = append(where, fmt.Sprintf("method = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from ui_assets where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select id, asset_type, name, category, method, locator, action, value, description, status, created_by, created_at, updated_at
		from ui_assets
		where `+whereSQL+`
		order by id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []model.UIAsset
	for rows.Next() {
		var item model.UIAsset
		if err := rows.Scan(&item.ID, &item.AssetType, &item.Name, &item.Category, &item.Method, &item.Locator, &item.Action, &item.Value, &item.Description, &item.Status, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *AutomationRepository) CreateUIAsset(ctx context.Context, assetType string, req model.UIAssetRequest, actor string) error {
	_, err := r.db.ExecContext(ctx, `insert into ui_assets(asset_type, name, category, method, locator, action, value, description, status, created_by) values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, assetType, req.Name, req.Category, req.Method, req.Locator, req.Action, req.Value, req.Description, req.Status, actor)
	return err
}

func (r *AutomationRepository) UpdateUIAsset(ctx context.Context, id int64, assetType string, req model.UIAssetRequest) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update ui_assets set name = $1, category = $2, method = $3, locator = $4, action = $5, value = $6, description = $7, status = $8, updated_at = now() where id = $9 and asset_type = $10 and deleted_at is null`, req.Name, req.Category, req.Method, req.Locator, req.Action, req.Value, req.Description, req.Status, id, assetType)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *AutomationRepository) DeleteUIAsset(ctx context.Context, id int64, assetType string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update ui_assets set deleted_at = now(), updated_at = now() where id = $1 and asset_type = $2 and deleted_at is null`, id, assetType)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *AutomationRepository) ListPageElements(ctx context.Context, pageID int64, page, pageSize int) ([]model.PageElement, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `select count(*) from page_elements where page_id = $1 and deleted_at is null`, pageID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		select id, page_id, name, type1, locator1, index1, type2, locator2, index2, type3, locator3, index3, ai_prompt, wait_time,
			fingerprint, capture_source, capture_url, tag_name, accessible_name, quality_score, captured_by, captured_at, last_verified_at, verification_status, current_version,
			created_at, updated_at
		from page_elements
		where page_id = $1 and deleted_at is null
		order by id desc
		limit $2 offset $3
	`, pageID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []model.PageElement
	for rows.Next() {
		var item model.PageElement
		if err := rows.Scan(&item.ID, &item.PageID, &item.Name, &item.Type1, &item.Locator1, &item.Index1, &item.Type2, &item.Locator2, &item.Index2, &item.Type3, &item.Locator3, &item.Index3, &item.AIPrompt, &item.WaitTime, &item.Fingerprint, &item.CaptureSource, &item.CaptureURL, &item.TagName, &item.AccessibleName, &item.QualityScore, &item.CapturedBy, &item.CapturedAt, &item.LastVerifiedAt, &item.VerificationStatus, &item.CurrentVersion, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *AutomationRepository) CreatePageElement(ctx context.Context, req model.PageElementRequest) error {
	_, err := r.db.ExecContext(ctx, `insert into page_elements(page_id, name, type1, locator1, index1, type2, locator2, index2, type3, locator3, index3, ai_prompt, wait_time) values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`, req.PageID, req.Name, req.Type1, req.Locator1, req.Index1, req.Type2, req.Locator2, req.Index2, req.Type3, req.Locator3, req.Index3, req.AIPrompt, req.WaitTime)
	return err
}

func (r *AutomationRepository) UpdatePageElement(ctx context.Context, id int64, req model.PageElementRequest) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update page_elements set page_id = $1, name = $2, type1 = $3, locator1 = $4, index1 = $5, type2 = $6, locator2 = $7, index2 = $8, type3 = $9, locator3 = $10, index3 = $11, ai_prompt = $12, wait_time = $13, updated_at = now() where id = $14 and deleted_at is null`, req.PageID, req.Name, req.Type1, req.Locator1, req.Index1, req.Type2, req.Locator2, req.Index2, req.Type3, req.Locator3, req.Index3, req.AIPrompt, req.WaitTime, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *AutomationRepository) DeletePageElement(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update page_elements set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
