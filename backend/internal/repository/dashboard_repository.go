package repository

import (
	"context"
	"database/sql"
	"time"

	"synapseqa/backend/internal/model"
)

const (
	dashboardStatusRunning  = `'active', 'pending', 'queued', 'dispatching', 'dispatched', 'running', 'stopping'`
	dashboardStatusSuccess  = `'completed', 'success'`
	dashboardStatusFailed   = `'failed', 'execution_failed', 'threshold_failed', 'timed_out'`
	dashboardStatusCanceled = `'canceled'`
)

// DashboardRepository 只读取首页所需的项目和三个执行来源的聚合数据。
type DashboardRepository struct {
	db *sql.DB
}

func NewDashboardRepository(db *sql.DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

func (r *DashboardRepository) ListProjects(ctx context.Context, userID int64, admin bool, projectID *int64) ([]model.DashboardProject, error) {
	rows, err := r.db.QueryContext(ctx, `
		select p.id, p.name
		from projects p
		where p.deleted_at is null and p.status = 'active'
		  and ($2 or exists(
			select 1 from project_members pm
			where pm.user_id = $1 and pm.project_id = p.id
		  ))
		  and ($3::bigint is null or p.id = $3)
		order by p.id asc
	`, userID, admin, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := make([]model.DashboardProject, 0)
	for rows.Next() {
		var item model.DashboardProject
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		projects = append(projects, item)
	}
	return projects, rows.Err()
}

func (r *DashboardRepository) ListUIExecutions(ctx context.Context, userID int64, admin bool, projectID *int64, from, to time.Time) (model.DashboardSourceData, error) {
	counts, err := r.queryDashboardCounts(ctx, `
		with scoped_runs as (
			select er.id, er.status
			from execution_runs er
			join lateral unnest(er.case_ids) case_ref(case_id) on true
			join test_cases tc on tc.id = case_ref.case_id and tc.deleted_at is null
			join products product on product.id = tc.product_id and product.deleted_at is null
			join projects p on p.id = product.project_id and p.deleted_at is null and p.status = 'active'
			where er.run_type = 'ui'
			  and er.created_at >= $3 and er.created_at < $4
			  and not exists(
				select 1
				from unnest(er.case_ids) all_case_ref(case_id)
				left join test_cases all_tc on all_tc.id = all_case_ref.case_id and all_tc.deleted_at is null
				left join products all_product on all_product.id = all_tc.product_id and all_product.deleted_at is null
				left join projects all_project on all_project.id = all_product.project_id
				  and all_project.deleted_at is null and all_project.status = 'active'
				where all_project.id is null or all_project.id <> p.id
			  )
			  and ($2 or exists(
				select 1 from project_members pm
				where pm.user_id = $1 and pm.project_id = p.id
			  ))
			  and ($5::bigint is null or p.id = $5)
			group by er.id, er.status
			having count(distinct p.id) = 1
		)
		select count(*) as total,
		       coalesce(sum(case when lower(trim(status)) in (`+dashboardStatusSuccess+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(status)) in (`+dashboardStatusFailed+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(status)) in (`+dashboardStatusRunning+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(status)) in (`+dashboardStatusCanceled+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(status)) not in (`+dashboardStatusSuccess+`, `+dashboardStatusFailed+`, `+dashboardStatusRunning+`, `+dashboardStatusCanceled+`) then 1 else 0 end), 0)
		from scoped_runs
	`, userID, admin, from, to, projectID)
	if err != nil {
		return model.DashboardSourceData{}, err
	}
	recent, err := r.queryDashboardRecords(ctx, "ui", `
		with scoped_runs as (
			select er.id::text as id, er.status, min(p.id) as project_id, min(p.name) as project_name,
			       coalesce(string_agg(tc.name, '、' order by tc.name), 'UI 执行') as title,
			       er.created_at, er.started_at, er.finished_at, null::bigint as duration_ms
			from execution_runs er
			join lateral unnest(er.case_ids) case_ref(case_id) on true
			join test_cases tc on tc.id = case_ref.case_id and tc.deleted_at is null
			join products product on product.id = tc.product_id and product.deleted_at is null
			join projects p on p.id = product.project_id and p.deleted_at is null and p.status = 'active'
			where er.run_type = 'ui'
			  and er.created_at >= $3 and er.created_at < $4
			  and not exists(
				select 1
				from unnest(er.case_ids) all_case_ref(case_id)
				left join test_cases all_tc on all_tc.id = all_case_ref.case_id and all_tc.deleted_at is null
				left join products all_product on all_product.id = all_tc.product_id and all_product.deleted_at is null
				left join projects all_project on all_project.id = all_product.project_id
				  and all_project.deleted_at is null and all_project.status = 'active'
				where all_project.id is null or all_project.id <> p.id
			  )
			  and ($2 or exists(
				select 1 from project_members pm
				where pm.user_id = $1 and pm.project_id = p.id
			  ))
			  and ($5::bigint is null or p.id = $5)
			group by er.id, er.status, er.created_at, er.started_at, er.finished_at
			having count(distinct p.id) = 1
		)
		select id, status, project_id, project_name, title, created_at, started_at, finished_at, duration_ms
		from scoped_runs
		order by created_at desc, id::bigint desc
		limit 5
	`, userID, admin, from, to, projectID)
	if err != nil {
		return model.DashboardSourceData{}, err
	}
	failed, err := r.queryDashboardRecords(ctx, "ui", `
		with scoped_runs as (
			select er.id::text as id, er.status, min(p.id) as project_id, min(p.name) as project_name,
			       coalesce(string_agg(tc.name, '、' order by tc.name), 'UI 执行') as title,
			       er.created_at, er.started_at, er.finished_at, null::bigint as duration_ms
			from execution_runs er
			join lateral unnest(er.case_ids) case_ref(case_id) on true
			join test_cases tc on tc.id = case_ref.case_id and tc.deleted_at is null
			join products product on product.id = tc.product_id and product.deleted_at is null
			join projects p on p.id = product.project_id and p.deleted_at is null and p.status = 'active'
			where er.run_type = 'ui'
			  and er.created_at >= $3 and er.created_at < $4
			  and lower(trim(er.status)) in (`+dashboardStatusFailed+`)
			  and not exists(
				select 1
				from unnest(er.case_ids) all_case_ref(case_id)
				left join test_cases all_tc on all_tc.id = all_case_ref.case_id and all_tc.deleted_at is null
				left join products all_product on all_product.id = all_tc.product_id and all_product.deleted_at is null
				left join projects all_project on all_project.id = all_product.project_id
				  and all_project.deleted_at is null and all_project.status = 'active'
				where all_project.id is null or all_project.id <> p.id
			  )
			  and ($2 or exists(
				select 1 from project_members pm
				where pm.user_id = $1 and pm.project_id = p.id
			  ))
			  and ($5::bigint is null or p.id = $5)
			group by er.id, er.status, er.created_at, er.started_at, er.finished_at
			having count(distinct p.id) = 1
		)
		select id, status, project_id, project_name, title, created_at, started_at, finished_at, duration_ms
		from scoped_runs
		order by created_at desc, id::bigint desc
		limit 3
	`, userID, admin, from, to, projectID)
	if err != nil {
		return model.DashboardSourceData{}, err
	}
	return model.DashboardSourceData{Counts: counts, Recent: recent, Failed: failed}, nil
}

func (r *DashboardRepository) ListAPIExecutions(ctx context.Context, userID int64, admin bool, projectID *int64, from, to time.Time) (model.DashboardSourceData, error) {
	counts, err := r.queryDashboardCounts(ctx, `
		select count(*) as total,
		       coalesce(sum(case when lower(trim(b.status)) in (`+dashboardStatusSuccess+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(b.status)) in (`+dashboardStatusFailed+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(b.status)) in (`+dashboardStatusRunning+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(b.status)) in (`+dashboardStatusCanceled+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(b.status)) not in (`+dashboardStatusSuccess+`, `+dashboardStatusFailed+`, `+dashboardStatusRunning+`, `+dashboardStatusCanceled+`) then 1 else 0 end), 0)
		from api_test_run_batches b
		join projects p on p.id = b.project_id and p.deleted_at is null and p.status = 'active'
		where b.created_at >= $3 and b.created_at < $4
		  and ($2 or exists(
			select 1 from project_members pm
			where pm.user_id = $1 and pm.project_id = p.id
		  ))
		  and ($5::bigint is null or b.project_id = $5)
	`, userID, admin, from, to, projectID)
	if err != nil {
		return model.DashboardSourceData{}, err
	}
	recent, err := r.queryDashboardRecords(ctx, "api", `
		select b.batch_id, b.status, b.project_id, p.name,
		       '接口测试批次 ' || b.batch_id,
		       b.created_at, b.started_at, b.finished_at, null::bigint
		from api_test_run_batches b
		join projects p on p.id = b.project_id and p.deleted_at is null and p.status = 'active'
		where b.created_at >= $3 and b.created_at < $4
		  and ($2 or exists(
			select 1 from project_members pm
			where pm.user_id = $1 and pm.project_id = p.id
		  ))
		  and ($5::bigint is null or b.project_id = $5)
		order by b.created_at desc, b.id desc
		limit 5
	`, userID, admin, from, to, projectID)
	if err != nil {
		return model.DashboardSourceData{}, err
	}
	failed, err := r.queryDashboardRecords(ctx, "api", `
		select b.batch_id, b.status, b.project_id, p.name,
		       '接口测试批次 ' || b.batch_id,
		       b.created_at, b.started_at, b.finished_at, null::bigint
		from api_test_run_batches b
		join projects p on p.id = b.project_id and p.deleted_at is null and p.status = 'active'
		where b.created_at >= $3 and b.created_at < $4
		  and lower(trim(b.status)) in (`+dashboardStatusFailed+`)
		  and ($2 or exists(
			select 1 from project_members pm
			where pm.user_id = $1 and pm.project_id = p.id
		  ))
		  and ($5::bigint is null or b.project_id = $5)
		order by b.created_at desc, b.id desc
		limit 3
	`, userID, admin, from, to, projectID)
	if err != nil {
		return model.DashboardSourceData{}, err
	}
	return model.DashboardSourceData{Counts: counts, Recent: recent, Failed: failed}, nil
}

func (r *DashboardRepository) ListPerfExecutions(ctx context.Context, userID int64, admin bool, projectID *int64, from, to time.Time) (model.DashboardSourceData, error) {
	counts, err := r.queryDashboardCounts(ctx, `
		select count(*) as total,
		       coalesce(sum(case when lower(trim(run.status)) in (`+dashboardStatusSuccess+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(run.status)) in (`+dashboardStatusFailed+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(run.status)) in (`+dashboardStatusRunning+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(run.status)) in (`+dashboardStatusCanceled+`) then 1 else 0 end), 0),
		       coalesce(sum(case when lower(trim(run.status)) not in (`+dashboardStatusSuccess+`, `+dashboardStatusFailed+`, `+dashboardStatusRunning+`, `+dashboardStatusCanceled+`) then 1 else 0 end), 0)
		from perf_test_runs run
		join perf_test_plans plan on plan.id = run.plan_id and plan.deleted_at is null
		join products product on product.id = plan.product_id and product.deleted_at is null
		join projects p on p.id = product.project_id and p.deleted_at is null and p.status = 'active'
		where run.created_at >= $3 and run.created_at < $4
		  and ($2 or exists(
			select 1 from project_members pm
			where pm.user_id = $1 and pm.project_id = p.id
		  ))
		  and ($5::bigint is null or product.project_id = $5)
	`, userID, admin, from, to, projectID)
	if err != nil {
		return model.DashboardSourceData{}, err
	}
	recent, err := r.queryDashboardRecords(ctx, "perf", `
		select run.id::text, run.status, product.project_id, p.name,
		       coalesce(plan.name, '性能测试执行'),
		       run.created_at, run.started_at, run.finished_at, run.duration_ms
		from perf_test_runs run
		join perf_test_plans plan on plan.id = run.plan_id and plan.deleted_at is null
		join products product on product.id = plan.product_id and product.deleted_at is null
		join projects p on p.id = product.project_id and p.deleted_at is null and p.status = 'active'
		where run.created_at >= $3 and run.created_at < $4
		  and ($2 or exists(
			select 1 from project_members pm
			where pm.user_id = $1 and pm.project_id = p.id
		  ))
		  and ($5::bigint is null or product.project_id = $5)
		order by run.created_at desc, run.id desc
		limit 5
	`, userID, admin, from, to, projectID)
	if err != nil {
		return model.DashboardSourceData{}, err
	}
	failed, err := r.queryDashboardRecords(ctx, "perf", `
		select run.id::text, run.status, product.project_id, p.name,
		       coalesce(plan.name, '性能测试执行'),
		       run.created_at, run.started_at, run.finished_at, run.duration_ms
		from perf_test_runs run
		join perf_test_plans plan on plan.id = run.plan_id and plan.deleted_at is null
		join products product on product.id = plan.product_id and product.deleted_at is null
		join projects p on p.id = product.project_id and p.deleted_at is null and p.status = 'active'
		where run.created_at >= $3 and run.created_at < $4
		  and lower(trim(run.status)) in (`+dashboardStatusFailed+`)
		  and ($2 or exists(
			select 1 from project_members pm
			where pm.user_id = $1 and pm.project_id = p.id
		  ))
		  and ($5::bigint is null or product.project_id = $5)
		order by run.created_at desc, run.id desc
		limit 3
	`, userID, admin, from, to, projectID)
	if err != nil {
		return model.DashboardSourceData{}, err
	}
	return model.DashboardSourceData{Counts: counts, Recent: recent, Failed: failed}, nil
}

func (r *DashboardRepository) queryDashboardCounts(ctx context.Context, query string, args ...any) (model.DashboardExecutionCounts, error) {
	var counts model.DashboardExecutionCounts
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&counts.Total, &counts.Success, &counts.Failed, &counts.Running, &counts.Canceled, &counts.Unknown,
	)
	return counts, err
}

func (r *DashboardRepository) queryDashboardRecords(ctx context.Context, source, query string, args ...any) ([]model.DashboardExecutionRecord, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.DashboardExecutionRecord, 0)
	for rows.Next() {
		var item model.DashboardExecutionRecord
		var startedAt, finishedAt sql.NullTime
		var duration sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Status, &item.ProjectID, &item.ProjectName, &item.Title,
			&item.CreatedAt, &startedAt, &finishedAt, &duration); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			item.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			item.FinishedAt = &finishedAt.Time
		}
		if duration.Valid {
			item.DurationMS = &duration.Int64
		}
		item.Type = source
		items = append(items, item)
	}
	return items, rows.Err()
}
