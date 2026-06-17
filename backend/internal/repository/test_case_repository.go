package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"synapseqa/backend/internal/model"
)

type TestCaseRepository struct {
	db *sql.DB
}

func NewTestCaseRepository(db *sql.DB) *TestCaseRepository {
	return &TestCaseRepository{db: db}
}

func (r *TestCaseRepository) List(ctx context.Context, filter model.TestCaseFilter, page, pageSize int) ([]model.TestCase, int64, error) {
	where := []string{"tc.deleted_at is null"}
	args := []any{}
	if filter.ID != "" {
		id, err := strconv.ParseInt(filter.ID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("tc.id = $%d", len(args)))
	}
	if filter.Name != "" {
		args = append(args, "%"+strings.ToLower(filter.Name)+"%")
		where = append(where, fmt.Sprintf("lower(tc.name) like $%d", len(args)))
	}
	if filter.ProductID != "" {
		id, err := strconv.ParseInt(filter.ProductID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("tc.product_id = $%d", len(args)))
	}
	if filter.ModuleID != "" {
		id, err := strconv.ParseInt(filter.ModuleID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("tc.module_id = $%d", len(args)))
	}
	if filter.CaseType != "" {
		args = append(args, filter.CaseType)
		where = append(where, fmt.Sprintf("tc.case_type = $%d", len(args)))
	}
	if filter.Priority != "" {
		args = append(args, filter.Priority)
		where = append(where, fmt.Sprintf("tc.priority = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("tc.status = $%d", len(args)))
	}
	if filter.Owner != "" {
		args = append(args, "%"+strings.ToLower(filter.Owner)+"%")
		where = append(where, fmt.Sprintf("lower(tc.owner) like $%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from test_cases tc where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		select tc.id, tc.product_id, coalesce(pr.name || '/' || p.name, ''), coalesce(tc.module_id, 0), coalesce(pm.name, ''),
		       coalesce(tc.page_id, 0), coalesce(pg.name, ''), tc.name, tc.case_type, tc.priority, tc.status, tc.owner,
		       tc.tags, tc.description, tc.preconditions, tc.expected_result, tc.data_enabled, tc.created_by, tc.created_at, tc.updated_at
		from test_cases tc
		left join products p on p.id = tc.product_id
		left join projects pr on pr.id = p.project_id
		left join product_modules pm on pm.id = tc.module_id
		left join ui_assets pg on pg.id = tc.page_id
		where `+whereSQL+`
		order by tc.id desc
		limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.TestCase{}
	for rows.Next() {
		item, err := scanTestCase(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *TestCaseRepository) Get(ctx context.Context, id int64) (model.TestCaseDetail, error) {
	row := r.db.QueryRowContext(ctx, `
		select tc.id, tc.product_id, coalesce(pr.name || '/' || p.name, ''), coalesce(tc.module_id, 0), coalesce(pm.name, ''),
		       coalesce(tc.page_id, 0), coalesce(pg.name, ''), tc.name, tc.case_type, tc.priority, tc.status, tc.owner,
		       tc.tags, tc.description, tc.preconditions, tc.expected_result, tc.data_enabled, tc.created_by, tc.created_at, tc.updated_at
		from test_cases tc
		left join products p on p.id = tc.product_id
		left join projects pr on pr.id = p.project_id
		left join product_modules pm on pm.id = tc.module_id
		left join ui_assets pg on pg.id = tc.page_id
		where tc.id = $1 and tc.deleted_at is null`, id)
	base, err := scanTestCase(row)
	if err != nil {
		return model.TestCaseDetail{}, err
	}
	steps, err := r.ListSteps(ctx, id)
	if err != nil {
		return model.TestCaseDetail{}, err
	}
	datasets, err := r.ListDatasets(ctx, id)
	if err != nil {
		return model.TestCaseDetail{}, err
	}
	return model.TestCaseDetail{TestCase: base, Steps: steps, Datasets: datasets}, nil
}

func (r *TestCaseRepository) Create(ctx context.Context, req model.TestCaseRequest, actor string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, `
		insert into test_cases(product_id, module_id, page_id, name, case_type, priority, status, owner, tags, description, preconditions, expected_result, data_enabled, created_by)
		values($1, nullif($2, 0), nullif($3, 0), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		returning id`, req.ProductID, req.ModuleID, req.PageID, req.Name, req.CaseType, req.Priority, req.Status, req.Owner, req.Tags, req.Description, req.Preconditions, req.ExpectedResult, req.DataEnabled, actor).Scan(&id); err != nil {
		return 0, err
	}
	if err := replaceSteps(ctx, tx, id, req.StepIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (r *TestCaseRepository) Update(ctx context.Context, id int64, req model.TestCaseRequest) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		update test_cases set product_id = $1, module_id = nullif($2, 0), page_id = nullif($3, 0), name = $4,
			case_type = $5, priority = $6, status = $7, owner = $8, tags = $9, description = $10,
			preconditions = $11, expected_result = $12, data_enabled = $13, updated_at = now()
		where id = $14 and deleted_at is null`, req.ProductID, req.ModuleID, req.PageID, req.Name, req.CaseType, req.Priority, req.Status, req.Owner, req.Tags, req.Description, req.Preconditions, req.ExpectedResult, req.DataEnabled, id)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return rows, err
	}
	if err := replaceSteps(ctx, tx, id, req.StepIDs); err != nil {
		return 0, err
	}
	return rows, tx.Commit()
}

func (r *TestCaseRepository) Delete(ctx context.Context, id int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update test_cases set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null`, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *TestCaseRepository) ExistsProduct(ctx context.Context, id int64) bool {
	var exists bool
	_ = r.db.QueryRowContext(ctx, `select exists(select 1 from products where id = $1 and deleted_at is null)`, id).Scan(&exists)
	return exists
}

func (r *TestCaseRepository) CountMissingSteps(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	rows, err := r.db.QueryContext(ctx, `select id from ui_assets where asset_type = 'page_step' and deleted_at is null`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	found := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		found[id] = true
	}
	var missing int64
	for _, id := range ids {
		if !found[id] {
			missing++
		}
	}
	return missing, rows.Err()
}

func (r *TestCaseRepository) ListSteps(ctx context.Context, caseID int64) ([]model.TestCaseStep, error) {
	rows, err := r.db.QueryContext(ctx, `
		select tcs.id, tcs.case_id, tcs.step_id, coalesce(s.name, ''), tcs.sort_order, tcs.note, tcs.created_at
		from test_case_steps tcs
		left join ui_assets s on s.id = tcs.step_id
		where tcs.case_id = $1
		order by tcs.sort_order asc, tcs.id asc`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.TestCaseStep{}
	for rows.Next() {
		var item model.TestCaseStep
		if err := rows.Scan(&item.ID, &item.CaseID, &item.StepID, &item.StepName, &item.SortOrder, &item.Note, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TestCaseRepository) ListDatasets(ctx context.Context, caseID int64) ([]model.TestCaseDataset, error) {
	rows, err := r.db.QueryContext(ctx, `select id, case_id, name, variables, enabled, created_at, updated_at from test_case_datasets where case_id = $1 order by id desc`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.TestCaseDataset{}
	for rows.Next() {
		var item model.TestCaseDataset
		if err := rows.Scan(&item.ID, &item.CaseID, &item.Name, &item.Variables, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TestCaseRepository) CreateDataset(ctx context.Context, caseID int64, req model.TestCaseDatasetRequest) error {
	_, err := r.db.ExecContext(ctx, `insert into test_case_datasets(case_id, name, variables, enabled) values($1, $2, $3, $4)`, caseID, req.Name, req.Variables, req.Enabled)
	return err
}

func (r *TestCaseRepository) UpdateDataset(ctx context.Context, caseID, datasetID int64, req model.TestCaseDatasetRequest) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update test_case_datasets set name = $1, variables = $2, enabled = $3, updated_at = now() where id = $4 and case_id = $5`, req.Name, req.Variables, req.Enabled, datasetID, caseID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *TestCaseRepository) DeleteDataset(ctx context.Context, caseID, datasetID int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `delete from test_case_datasets where id = $1 and case_id = $2`, datasetID, caseID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

type testCaseScanner interface {
	Scan(dest ...any) error
}

func scanTestCase(scanner testCaseScanner) (model.TestCase, error) {
	var item model.TestCase
	err := scanner.Scan(&item.ID, &item.ProductID, &item.ProductName, &item.ModuleID, &item.ModuleName, &item.PageID, &item.PageName, &item.Name, &item.CaseType, &item.Priority, &item.Status, &item.Owner, &item.Tags, &item.Description, &item.Preconditions, &item.ExpectedResult, &item.DataEnabled, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func replaceSteps(ctx context.Context, tx *sql.Tx, caseID int64, stepIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `delete from test_case_steps where case_id = $1`, caseID); err != nil {
		return err
	}
	for index, stepID := range stepIDs {
		if _, err := tx.ExecContext(ctx, `insert into test_case_steps(case_id, step_id, sort_order) values($1, $2, $3)`, caseID, stepID, index+1); err != nil {
			return err
		}
	}
	return nil
}
