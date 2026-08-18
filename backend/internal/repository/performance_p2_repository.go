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

var _ interface {
	ListPerfEnvironments(context.Context, int64) ([]model.PerfEnvironment, error)
	GetPerfEnvironment(context.Context, int64) (model.PerfEnvironment, error)
	CreatePlanWithEnvironment(context.Context, model.PerfTestPlanRequest, string) (int64, error)
	UpdatePlanWithEnvironment(context.Context, int64, model.PerfTestPlanRequest) (int64, error)
	GetPerfBaseline(context.Context, int64, string, string, *int64) (model.PerfBaseline, error)
	GetPerfBaselineByRun(context.Context, int64) (model.PerfBaseline, error)
	UpsertPerfBaseline(context.Context, model.PerfBaseline) (model.PerfBaseline, error)
	DeletePerfBaseline(context.Context, int64) error
	ListPerfTrendRuns(context.Context, int64, string, string, *int64, int) ([]model.PerfTestRun, error)
	GetActivePerfRun(context.Context, int64) (model.PerfTestRun, error)
	ListPerfDegradationCandidates(context.Context, int) ([]model.PerfTestRun, error)
	MarkPerfDegradationChecked(context.Context, int64) (int64, error)
	ListPerfSchedules(context.Context, model.PerfScheduleFilter, int, int) ([]model.PerfSchedule, int64, error)
	GetPerfSchedule(context.Context, int64) (model.PerfSchedule, error)
	CreatePerfSchedule(context.Context, model.PerfSchedule) (model.PerfSchedule, error)
	UpdatePerfSchedule(context.Context, model.PerfSchedule) (int64, error)
	SetPerfScheduleEnabled(context.Context, int64, bool, *time.Time, string) (int64, error)
	SoftDeletePerfSchedule(context.Context, int64, string) (int64, error)
	ClaimPerfSchedules(context.Context, string, time.Time, int) ([]model.PerfSchedule, error)
	FinishPerfSchedule(context.Context, int64, string, *time.Time, *time.Time, string, *int64, string) (int64, error)
	SetPerfScheduleError(context.Context, int64, string, string) (int64, error)
} = (*PerformanceRepository)(nil)

func (r *PerformanceRepository) ListPerfEnvironments(ctx context.Context, productID int64) ([]model.PerfEnvironment, error) {
	rows, err := r.db.QueryContext(ctx, `select id, product_id, env_name, target, deploy_env, query_enabled, write_enabled
		from test_objects where product_id=$1 and deleted_at is null order by id`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.PerfEnvironment{}
	for rows.Next() {
		var item model.PerfEnvironment
		if err := rows.Scan(&item.EnvironmentID, &item.ProductID, &item.EnvName, &item.BaseURL, &item.DeployEnv, &item.QueryEnabled, &item.WriteEnabled); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PerformanceRepository) GetPerfEnvironment(ctx context.Context, id int64) (model.PerfEnvironment, error) {
	var item model.PerfEnvironment
	err := r.db.QueryRowContext(ctx, `select id, product_id, env_name, target, deploy_env, query_enabled, write_enabled
		from test_objects where id=$1 and deleted_at is null`, id).Scan(&item.EnvironmentID, &item.ProductID, &item.EnvName, &item.BaseURL, &item.DeployEnv, &item.QueryEnabled, &item.WriteEnabled)
	return item, err
}

func (r *PerformanceRepository) CreatePlanWithEnvironment(ctx context.Context, req model.PerfTestPlanRequest, actor string) (int64, error) {
	thresholds, err := json.Marshal(req.Thresholds)
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.db.QueryRowContext(ctx, `insert into perf_test_plans(product_id, name, target_url, method, headers, body, scenario_type, load_config, environment, environment_id, thresholds, status, priority, owner, tags, description, created_by)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) returning id`,
		req.ProductID, req.Name, req.TargetURL, req.Method, req.Headers, req.Body, req.ScenarioType, req.LoadConfig, req.Environment, req.EnvironmentID, thresholds, req.Status, req.Priority, req.Owner, req.Tags, req.Description, actor).Scan(&id)
	return id, err
}

func (r *PerformanceRepository) UpdatePlanWithEnvironment(ctx context.Context, id int64, req model.PerfTestPlanRequest) (int64, error) {
	thresholds, err := json.Marshal(req.Thresholds)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, `update perf_test_plans set product_id=$1,name=$2,target_url=$3,method=$4,headers=$5,body=$6,
		scenario_type=$7,load_config=$8,environment=$9,environment_id=$10,thresholds=$11,status=$12,priority=$13,owner=$14,tags=$15,description=$16,updated_at=now()
		where id=$17 and deleted_at is null`, req.ProductID, req.Name, req.TargetURL, req.Method, req.Headers, req.Body, req.ScenarioType, req.LoadConfig, req.Environment, req.EnvironmentID, thresholds, req.Status, req.Priority, req.Owner, req.Tags, req.Description, id)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	return count, nil
}

func (r *PerformanceRepository) CreateRunWithEnvironment(ctx context.Context, planID int64, scenarioType, environment string, environmentID *int64, configHash, idempotencyKey, triggeredBy string, snapshot json.RawMessage, requestedAt, expectedFinishAt time.Time) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `insert into perf_test_runs(plan_id,scenario_type,plan_snapshot,config_hash,environment,environment_id,idempotency_key,triggered_by,requested_at,expected_finish_at,status)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'pending') returning id`, planID, scenarioType, snapshot, configHash, environment, environmentID, idempotencyKey, triggeredBy, requestedAt, expectedFinishAt).Scan(&id)
	return id, err
}

func (r *PerformanceRepository) GetPerfBaseline(ctx context.Context, planID int64, scenarioType, environment string, environmentID *int64) (model.PerfBaseline, error) {
	var item model.PerfBaseline
	err := r.db.QueryRowContext(ctx, `select id,plan_id,scenario_type,environment_id,environment,run_id,set_by,created_at,updated_at
		from perf_test_baselines where plan_id=$1 and scenario_type=$2 and environment_id is not distinct from $3
		and ($3 is not null or environment=$4)`, planID, scenarioType, environmentID, environment).Scan(&item.ID, &item.PlanID, &item.ScenarioType, &item.EnvironmentID, &item.Environment, &item.RunID, &item.SetBy, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *PerformanceRepository) GetPerfBaselineByRun(ctx context.Context, runID int64) (model.PerfBaseline, error) {
	var item model.PerfBaseline
	err := r.db.QueryRowContext(ctx, `select id,plan_id,scenario_type,environment_id,environment,run_id,set_by,created_at,updated_at from perf_test_baselines where run_id=$1`, runID).Scan(&item.ID, &item.PlanID, &item.ScenarioType, &item.EnvironmentID, &item.Environment, &item.RunID, &item.SetBy, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *PerformanceRepository) UpsertPerfBaseline(ctx context.Context, baseline model.PerfBaseline) (model.PerfBaseline, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.PerfBaseline{}, err
	}
	defer tx.Rollback()
	var planID int64
	if err := tx.QueryRowContext(ctx, `select id from perf_test_plans where id=$1 and deleted_at is null for update`, baseline.PlanID).Scan(&planID); err != nil {
		return model.PerfBaseline{}, err
	}
	var existing model.PerfBaseline
	err = tx.QueryRowContext(ctx, `select id,plan_id,scenario_type,environment_id,environment,run_id,set_by,created_at,updated_at from perf_test_baselines where run_id=$1`, baseline.RunID).Scan(&existing.ID, &existing.PlanID, &existing.ScenarioType, &existing.EnvironmentID, &existing.Environment, &existing.RunID, &existing.SetBy, &existing.CreatedAt, &existing.UpdatedAt)
	if err == nil {
		return existing, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return model.PerfBaseline{}, err
	}
	if _, err := tx.ExecContext(ctx, `delete from perf_test_baselines where plan_id=$1 and scenario_type=$2 and environment_id is not distinct from $3 and ($3 is not null or environment=$4)`, baseline.PlanID, baseline.ScenarioType, baseline.EnvironmentID, baseline.Environment); err != nil {
		return model.PerfBaseline{}, err
	}
	if err := tx.QueryRowContext(ctx, `insert into perf_test_baselines(plan_id,scenario_type,environment_id,environment,run_id,set_by) values($1,$2,$3,$4,$5,$6)
		returning id,plan_id,scenario_type,environment_id,environment,run_id,set_by,created_at,updated_at`, baseline.PlanID, baseline.ScenarioType, baseline.EnvironmentID, baseline.Environment, baseline.RunID, baseline.SetBy).Scan(&baseline.ID, &baseline.PlanID, &baseline.ScenarioType, &baseline.EnvironmentID, &baseline.Environment, &baseline.RunID, &baseline.SetBy, &baseline.CreatedAt, &baseline.UpdatedAt); err != nil {
		return model.PerfBaseline{}, err
	}
	return baseline, tx.Commit()
}

func (r *PerformanceRepository) DeletePerfBaseline(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `delete from perf_test_baselines where id=$1`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PerformanceRepository) ListPerfTrendRuns(ctx context.Context, planID int64, scenarioType, environment string, environmentID *int64, limit int) ([]model.PerfTestRun, error) {
	rows, err := r.db.QueryContext(ctx, `select `+perfRunColumns+` `+perfRunJoin+`
		where run.plan_id=$1 and run.scenario_type=$2 and run.environment_id is not distinct from $3 and ($3 is not null or run.environment=$4)
		and run.status in ('completed','threshold_failed') order by run.finished_at desc nulls last, run.id desc limit $5`, planID, scenarioType, environmentID, environment, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.PerfTestRun{}
	for rows.Next() {
		item, err := scanPerfTestRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PerformanceRepository) GetActivePerfRun(ctx context.Context, planID int64) (model.PerfTestRun, error) {
	row := r.db.QueryRowContext(ctx, `select `+perfRunColumns+` `+perfRunJoin+`
		where run.plan_id=$1
		and run.status in ('pending','queued','dispatching','dispatched','running','stopping') order by run.id desc limit 1`, planID)
	return scanPerfTestRun(row)
}

func (r *PerformanceRepository) ListPerfDegradationCandidates(ctx context.Context, limit int) ([]model.PerfTestRun, error) {
	rows, err := r.db.QueryContext(ctx, `select `+perfRunColumns+` `+perfRunJoin+`
		where run.status in ('completed','threshold_failed')
		and run.degradation_checked_at is null
		and exists (select 1 from perf_test_baselines b where b.plan_id=run.plan_id and b.scenario_type=run.scenario_type
			and b.run_id <> run.id and b.environment_id is not distinct from run.environment_id
			and (b.environment_id is not null or b.environment=run.environment))
		and not exists (select 1 from notifications n join users u on u.id=n.user_id
			where u.username=run.triggered_by and n.type='perf.degradation' and n.target_type='perf_run' and n.target_id=run.id::text)
		order by run.finished_at asc nulls first, run.id asc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.PerfTestRun{}
	for rows.Next() {
		item, err := scanPerfTestRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PerformanceRepository) MarkPerfDegradationChecked(ctx context.Context, runID int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update perf_test_runs set degradation_checked_at=now(),updated_at=now() where id=$1 and degradation_checked_at is null`, runID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PerformanceRepository) ListPerfSchedules(ctx context.Context, filter model.PerfScheduleFilter, page, pageSize int) ([]model.PerfSchedule, int64, error) {
	where := []string{"s.deleted_at is null"}
	args := []any{}
	if filter.PlanID != "" {
		id, err := strconv.ParseInt(filter.PlanID, 10, 64)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, id)
		where = append(where, fmt.Sprintf("s.plan_id=$%d", len(args)))
	}
	if filter.Enabled != nil {
		args = append(args, *filter.Enabled)
		where = append(where, fmt.Sprintf("s.enabled=$%d", len(args)))
	}
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from perf_test_schedules s where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `select s.id,s.name,s.plan_id,coalesce(p.name,''),s.environment_id,coalesce(e.env_name,''),s.cron_expression,s.timezone,s.enabled,s.next_run_at,s.last_scheduled_for,s.last_triggered_at,s.last_run_id,s.last_result,s.last_error,s.created_by,s.updated_by,s.created_at,s.updated_at,s.deleted_at
		from perf_test_schedules s left join perf_test_plans p on p.id=s.plan_id left join test_objects e on e.id=s.environment_id where `+whereSQL+` order by s.id desc limit $`+strconv.Itoa(len(queryArgs)-1)+` offset $`+strconv.Itoa(len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.PerfSchedule{}
	for rows.Next() {
		item, err := scanPerfSchedule(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *PerformanceRepository) GetPerfSchedule(ctx context.Context, id int64) (model.PerfSchedule, error) {
	row := r.db.QueryRowContext(ctx, `select s.id,s.name,s.plan_id,coalesce(p.name,''),s.environment_id,coalesce(e.env_name,''),s.cron_expression,s.timezone,s.enabled,s.next_run_at,s.last_scheduled_for,s.last_triggered_at,s.last_run_id,s.last_result,s.last_error,s.created_by,s.updated_by,s.created_at,s.updated_at,s.deleted_at
		from perf_test_schedules s left join perf_test_plans p on p.id=s.plan_id left join test_objects e on e.id=s.environment_id where s.id=$1 and s.deleted_at is null`, id)
	return scanPerfSchedule(row)
}

func (r *PerformanceRepository) CreatePerfSchedule(ctx context.Context, item model.PerfSchedule) (model.PerfSchedule, error) {
	row := r.db.QueryRowContext(ctx, `insert into perf_test_schedules(name,plan_id,environment_id,cron_expression,timezone,enabled,next_run_at,created_by,updated_by)
		values($1,$2,$3,$4,$5,$6,$7,$8,$8) returning id`, item.Name, item.PlanID, item.EnvironmentID, item.CronExpression, item.Timezone, item.Enabled, item.NextRunAt, item.CreatedBy)
	var id int64
	if err := row.Scan(&id); err != nil {
		return model.PerfSchedule{}, err
	}
	return r.GetPerfSchedule(ctx, id)
}

func (r *PerformanceRepository) UpdatePerfSchedule(ctx context.Context, item model.PerfSchedule) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update perf_test_schedules set name=$1,plan_id=$2,environment_id=$3,cron_expression=$4,timezone=$5,next_run_at=$6,last_error=$7,updated_by=$8,updated_at=now()
		where id=$9 and deleted_at is null and (claim_until is null or claim_until < now())`, item.Name, item.PlanID, item.EnvironmentID, item.CronExpression, item.Timezone, item.NextRunAt, item.LastError, item.UpdatedBy, item.ID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PerformanceRepository) SetPerfScheduleEnabled(ctx context.Context, id int64, enabled bool, next *time.Time, actor string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update perf_test_schedules set enabled=$1,next_run_at=$2,claim_token='',claim_owner='',claim_until=null,updated_by=$3,updated_at=now()
		where id=$4 and deleted_at is null and (claim_until is null or claim_until < now())`, enabled, next, actor, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PerformanceRepository) SoftDeletePerfSchedule(ctx context.Context, id int64, actor string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update perf_test_schedules set deleted_at=now(),enabled=false,next_run_at=null,claim_token='',claim_owner='',claim_until=null,updated_by=$1,updated_at=now() where id=$2 and deleted_at is null`, actor, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PerformanceRepository) ClaimPerfSchedules(ctx context.Context, owner string, now time.Time, limit int) ([]model.PerfSchedule, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `select id from perf_test_schedules where enabled and deleted_at is null and next_run_at is not null and next_run_at <= $1 and (claim_until is null or claim_until < $1) order by next_run_at,id for update skip locked limit $2`, now, limit)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	items := []model.PerfSchedule{}
	for _, id := range ids {
		var token string
		if err := tx.QueryRowContext(ctx, `update perf_test_schedules set claim_token=md5(random()::text||clock_timestamp()::text||id::text),claim_owner=$1,claim_until=$2,updated_at=now() where id=$3 returning claim_token`, owner, now.Add(2*time.Minute), id).Scan(&token); err != nil {
			return nil, err
		}
		item, err := scanPerfSchedule(tx.QueryRowContext(ctx, `select s.id,s.name,s.plan_id,coalesce(p.name,''),s.environment_id,coalesce(e.env_name,''),s.cron_expression,s.timezone,s.enabled,s.next_run_at,s.last_scheduled_for,s.last_triggered_at,s.last_run_id,s.last_result,s.last_error,s.created_by,s.updated_by,s.created_at,s.updated_at,s.deleted_at from perf_test_schedules s left join perf_test_plans p on p.id=s.plan_id left join test_objects e on e.id=s.environment_id where s.id=$1`, id))
		if err != nil {
			return nil, err
		}
		item.ClaimToken = token
		items = append(items, item)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PerformanceRepository) FinishPerfSchedule(ctx context.Context, id int64, token string, scheduledFor, next *time.Time, result string, runID *int64, lastError string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `update perf_test_schedules set enabled=case when $3='blocked' then false else enabled end,last_scheduled_for=$1,next_run_at=$2,last_result=$3,last_run_id=$4,last_triggered_at=now(),last_error=$5,claim_token='',claim_owner='',claim_until=null,updated_at=now() where id=$6 and claim_token=$7`, scheduledFor, next, result, runID, lastError, id, token)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *PerformanceRepository) SetPerfScheduleError(ctx context.Context, id int64, token, message string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `update perf_test_schedules set last_result='error',last_error=$1,last_triggered_at=now(),updated_at=now() where id=$2 and claim_token=$3`, message, id, token)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanPerfSchedule(scanner interface{ Scan(...any) error }) (model.PerfSchedule, error) {
	var item model.PerfSchedule
	var envID, lastRunID sql.NullInt64
	var next, scheduled, triggered, deleted sql.NullTime
	if err := scanner.Scan(&item.ID, &item.Name, &item.PlanID, &item.PlanName, &envID, &item.EnvironmentName, &item.CronExpression, &item.Timezone, &item.Enabled, &next, &scheduled, &triggered, &lastRunID, &item.LastResult, &item.LastError, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &deleted); err != nil {
		return model.PerfSchedule{}, err
	}
	if envID.Valid {
		value := envID.Int64
		item.EnvironmentID = &value
	}
	if lastRunID.Valid {
		value := lastRunID.Int64
		item.LastRunID = &value
	}
	if next.Valid {
		item.NextRunAt = &next.Time
	}
	if scheduled.Valid {
		item.LastScheduledFor = &scheduled.Time
	}
	if triggered.Valid {
		item.LastTriggeredAt = &triggered.Time
	}
	if deleted.Valid {
		item.DeletedAt = &deleted.Time
	}
	return item, nil
}
