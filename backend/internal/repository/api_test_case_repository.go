package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"synapseqa/backend/internal/model"
)

func (r *APIAutomationRepository) ListAPIGlobalVariables(ctx context.Context, userID int64, filter model.APIGlobalVariableFilter) ([]model.APIGlobalVariable, error) {
	rows, err := r.db.QueryContext(ctx, `
		select v.id,v.scope_type,v.project_id,v.product_id,coalesce(p.name,''),coalesce(pr.name,''),
		       v.env_name,v.var_name,v.value_type,v.var_value,v.description,v.enabled,v.sensitive,v.revision,
		       v.created_by,v.updated_by,v.created_at,v.updated_at,v.deleted_at
		from api_global_variables v
		left join projects p on p.id=v.project_id
		left join products pr on pr.id=v.product_id
		where v.deleted_at is null
		  and ($2='' or v.scope_type=$2)
		  and ($3='' or v.project_id=$3::bigint)
		  and ($4='' or v.product_id=$4::bigint)
		  and ($5='' or v.env_name=$5)
		  and ($6='' or v.var_name ilike '%'||$6||'%' or v.description ilike '%'||$6||'%')
		  and (v.scope_type='system' or exists(
		    select 1 from users u left join roles ro on ro.id=u.role_id
		    where u.id=$1 and u.deleted_at is null and (ro.code='admin' or exists(
		      select 1 from project_members pm where pm.user_id=u.id and pm.project_id=v.project_id
		    ))
		  ))
		order by v.scope_type,v.project_id,v.product_id,v.env_name,v.var_name_normalized
	`, userID, filter.ScopeType, filter.ProjectID, filter.ProductID, filter.EnvName, filter.Keyword)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.APIGlobalVariable{}
	for rows.Next() {
		var item model.APIGlobalVariable
		if err := rows.Scan(&item.ID, &item.ScopeType, &item.ProjectID, &item.ProductID, &item.ProjectName, &item.ProductName,
			&item.EnvName, &item.Name, &item.ValueType, &item.Value, &item.Description, &item.Enabled, &item.Sensitive,
			&item.Revision, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APIAutomationRepository) GetAPIGlobalVariable(ctx context.Context, id int64) (model.APIGlobalVariable, error) {
	var item model.APIGlobalVariable
	err := r.db.QueryRowContext(ctx, `
		select v.id,v.scope_type,v.project_id,v.product_id,coalesce(p.name,''),coalesce(pr.name,''),
		       v.env_name,v.var_name,v.value_type,v.var_value,v.description,v.enabled,v.sensitive,v.revision,
		       v.created_by,v.updated_by,v.created_at,v.updated_at,v.deleted_at
		from api_global_variables v
		left join projects p on p.id=v.project_id left join products pr on pr.id=v.product_id
		where v.id=$1 and v.deleted_at is null
	`, id).Scan(&item.ID, &item.ScopeType, &item.ProjectID, &item.ProductID, &item.ProjectName, &item.ProductName,
		&item.EnvName, &item.Name, &item.ValueType, &item.Value, &item.Description, &item.Enabled, &item.Sensitive,
		&item.Revision, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	return item, err
}

func (r *APIAutomationRepository) CreateAPIGlobalVariable(ctx context.Context, req model.APIGlobalVariableRequest, value, actor string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		insert into api_global_variables(scope_type,project_id,product_id,env_name,var_name,var_name_normalized,
		  value_type,var_value,description,enabled,sensitive,created_by,updated_by)
		values($1,nullif($2,0),nullif($3,0),$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
		returning id
	`, req.ScopeType, req.ProjectID, req.ProductID, req.EnvName, req.Name, strings.ToLower(req.Name),
		req.ValueType, value, req.Description, req.Enabled == nil || *req.Enabled, req.Sensitive, actor).Scan(&id)
	return id, err
}

func (r *APIAutomationRepository) UpdateAPIGlobalVariable(ctx context.Context, id int64, req model.APIGlobalVariableRequest, value, actor string) (int64, error) {
	var revision int64
	err := r.db.QueryRowContext(ctx, `
		update api_global_variables set scope_type=$2,project_id=nullif($3,0),product_id=nullif($4,0),env_name=$5,
		  var_name=$6,var_name_normalized=$7,value_type=$8,var_value=$9,description=$10,
		  enabled=$11,sensitive=$12,updated_by=$13,updated_at=now(),revision=revision+1
		where id=$1 and deleted_at is null and revision=$14 returning revision
	`, id, req.ScopeType, req.ProjectID, req.ProductID, req.EnvName, req.Name, strings.ToLower(req.Name),
		req.ValueType, value, req.Description, req.Enabled == nil || *req.Enabled, req.Sensitive, actor, req.Revision).Scan(&revision)
	return revision, err
}

func (r *APIAutomationRepository) DeleteAPIGlobalVariable(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `update api_global_variables set deleted_at=now(),updated_at=now() where id=$1 and deleted_at is null`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APIAutomationRepository) ListAPITestCases(ctx context.Context, userID int64, filter model.APITestCaseFilter, page, pageSize int) ([]model.APITestCase, int64, error) {
	where := []string{"c.deleted_at is null", `(exists(select 1 from users u left join roles ro on ro.id=u.role_id where u.id=$1 and u.deleted_at is null and (ro.code='admin' or exists(select 1 from project_members pm where pm.user_id=u.id and pm.project_id=c.project_id))))`}
	args := []any{userID}
	add := func(value, clause string) {
		if value != "" {
			args = append(args, value)
			where = append(where, fmt.Sprintf(clause, len(args)))
		}
	}
	add(filter.ProjectID, "c.project_id=$%d::bigint")
	add(filter.ProductID, "c.product_id=$%d::bigint")
	add(filter.ModuleID, "c.module_id=$%d::bigint")
	add(filter.Keyword, "(c.name ilike '%%'||$%[1]d||'%%' or cast(c.id as text)=$%[1]d)")
	add(filter.Status, "c.status=$%d")
	add(filter.Priority, "c.priority=$%d")
	add(filter.Owner, "c.owner=$%d")
	var total int64
	if err := r.db.QueryRowContext(ctx, `select count(*) from api_test_cases c where `+strings.Join(where, " and "), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	query := `
		select c.id,c.project_id,p.name,c.product_id,pr.name,coalesce(c.module_id,0),coalesce(m.name,''),
		  c.name,c.priority,c.status,c.owner,array_to_json(c.tags),c.draft,c.revision,c.current_version,
		  c.last_run_status,c.last_run_at,c.created_by,c.updated_by,c.created_at,c.updated_at,c.deleted_at
		from api_test_cases c join projects p on p.id=c.project_id join products pr on pr.id=c.product_id
		left join product_modules m on m.id=c.module_id
		where ` + strings.Join(where, " and ") + fmt.Sprintf(" order by c.updated_at desc limit $%d offset $%d", len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.APITestCase{}
	for rows.Next() {
		item, err := scanAPITestCase(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

type apiTestCaseScanner interface {
	Scan(...any) error
}

func scanAPITestCase(scanner apiTestCaseScanner) (model.APITestCase, error) {
	var item model.APITestCase
	var tags []byte
	err := scanner.Scan(&item.ID, &item.ProjectID, &item.ProjectName, &item.ProductID, &item.ProductName,
		&item.ModuleID, &item.ModuleName, &item.Name, &item.Priority, &item.Status, &item.Owner, &tags,
		&item.Draft, &item.Revision, &item.CurrentVersion, &item.LastRunStatus, &item.LastRunAt,
		&item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err == nil {
		_ = json.Unmarshal(tags, &item.Tags)
	}
	return item, err
}

func (r *APIAutomationRepository) GetAPITestCase(ctx context.Context, userID, id int64) (model.APITestCase, error) {
	row := r.db.QueryRowContext(ctx, `
		select c.id,c.project_id,p.name,c.product_id,pr.name,coalesce(c.module_id,0),coalesce(m.name,''),
		  c.name,c.priority,c.status,c.owner,array_to_json(c.tags),c.draft,c.revision,c.current_version,
		  c.last_run_status,c.last_run_at,c.created_by,c.updated_by,c.created_at,c.updated_at,c.deleted_at
		from api_test_cases c join projects p on p.id=c.project_id join products pr on pr.id=c.product_id
		left join product_modules m on m.id=c.module_id
		where c.id=$2 and c.deleted_at is null and exists(
		  select 1 from users u left join roles ro on ro.id=u.role_id where u.id=$1 and u.deleted_at is null
		  and (ro.code='admin' or exists(select 1 from project_members pm where pm.user_id=u.id and pm.project_id=c.project_id))
		)
	`, userID, id)
	return scanAPITestCase(row)
}

func (r *APIAutomationRepository) CreateAPITestCase(ctx context.Context, req model.APITestCaseRequest, actor string) (int64, error) {
	tags, _ := json.Marshal(req.Tags)
	var id int64
	err := r.db.QueryRowContext(ctx, `
		insert into api_test_cases(project_id,product_id,module_id,name,priority,status,owner,tags,draft,created_by,updated_by)
		values($1,$2,nullif($3,0),$4,$5,$6,$7,array(select jsonb_array_elements_text($8::jsonb)),$9,$10,$10)
		returning id
	`, req.ProjectID, req.ProductID, req.ModuleID, req.Name, req.Priority, req.Status, req.Owner, tags, req.Draft, actor).Scan(&id)
	return id, err
}

func (r *APIAutomationRepository) UpdateAPITestCase(ctx context.Context, id int64, req model.APITestCaseRequest, actor string) (int64, error) {
	tags, _ := json.Marshal(req.Tags)
	var revision int64
	err := r.db.QueryRowContext(ctx, `
		update api_test_cases set project_id=$2,product_id=$3,module_id=nullif($4,0),name=$5,priority=$6,status=$7,
		  owner=$8,tags=array(select jsonb_array_elements_text($9::jsonb)),draft=$10,updated_by=$11,
		  updated_at=now(),revision=revision+1
		where id=$1 and deleted_at is null and revision=$12 returning revision
	`, id, req.ProjectID, req.ProductID, req.ModuleID, req.Name, req.Priority, req.Status, req.Owner,
		tags, req.Draft, actor, req.Revision).Scan(&revision)
	return revision, err
}

func (r *APIAutomationRepository) DeleteAPITestCase(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `update api_test_cases set deleted_at=now(),updated_at=now() where id=$1 and deleted_at is null`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APIAutomationRepository) PublishAPITestCase(ctx context.Context, id int64, revision int64, actor, summary string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var version int
	var snapshot json.RawMessage
	err = tx.QueryRowContext(ctx, `
		update api_test_cases set current_version=current_version+1,status='active',revision=revision+1,
		  updated_by=$3,updated_at=now()
		where id=$1 and revision=$2 and deleted_at is null
		returning current_version,draft
	`, id, revision, actor).Scan(&version, &snapshot)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
		insert into api_test_case_versions(case_id,version,snapshot,change_summary,created_by)
		values($1,$2,$3,$4,$5)
	`, id, version, snapshot, summary, actor); err != nil {
		return 0, err
	}
	return version, tx.Commit()
}

func (r *APIAutomationRepository) ListAPITestCaseVersions(ctx context.Context, caseID int64) ([]model.APITestCaseVersion, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id,case_id,version,snapshot,change_summary,created_by,created_at
		from api_test_case_versions where case_id=$1 order by version desc
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.APITestCaseVersion{}
	for rows.Next() {
		var item model.APITestCaseVersion
		if err := rows.Scan(&item.ID, &item.CaseID, &item.Version, &item.Snapshot, &item.ChangeSummary, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APIAutomationRepository) GetAPITestCaseVersion(ctx context.Context, caseID int64, version int) (model.APITestCaseVersion, error) {
	var item model.APITestCaseVersion
	err := r.db.QueryRowContext(ctx, `
		select id,case_id,version,snapshot,change_summary,created_by,created_at
		from api_test_case_versions where case_id=$1 and version=$2
	`, caseID, version).Scan(&item.ID, &item.CaseID, &item.Version, &item.Snapshot, &item.ChangeSummary, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (r *APIAutomationRepository) TestObjectByEnvironment(ctx context.Context, productID int64, envName string) (model.TestObject, error) {
	var item model.TestObject
	err := r.db.QueryRowContext(ctx, `
		select id,product_id,env_name,target,deploy_env,auto_type,owner,query_enabled,write_enabled,created_at,updated_at
		from test_objects where product_id=$1 and env_name=$2 and deleted_at is null order by id limit 1
	`, productID, envName).Scan(&item.ID, &item.ProductID, &item.EnvName, &item.Target, &item.DeployEnv,
		&item.AutoType, &item.Owner, &item.QueryEnabled, &item.WriteEnabled, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *APIAutomationRepository) ResolveAPIGlobalVariables(ctx context.Context, projectID, productID int64, envName string) ([]model.APIGlobalVariable, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id,scope_type,project_id,product_id,'','',env_name,var_name,value_type,var_value,description,
		  enabled,sensitive,revision,created_by,updated_by,created_at,updated_at,deleted_at
		from api_global_variables
		where deleted_at is null and enabled=true and (env_name='' or env_name=$3)
		  and (scope_type='system' or (scope_type='project' and project_id=$1) or (scope_type='product' and product_id=$2))
		order by case scope_type when 'system' then 0 when 'project' then 1 else 2 end,
		  case when env_name='' then 0 else 1 end,id
	`, projectID, productID, envName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.APIGlobalVariable{}
	for rows.Next() {
		var item model.APIGlobalVariable
		if err := rows.Scan(&item.ID, &item.ScopeType, &item.ProjectID, &item.ProductID, &item.ProjectName, &item.ProductName,
			&item.EnvName, &item.Name, &item.ValueType, &item.Value, &item.Description, &item.Enabled, &item.Sensitive,
			&item.Revision, &item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *APIAutomationRepository) CreateAPITestRunBatch(ctx context.Context, batch model.APITestRunBatch, instances []model.APITestRunInstance) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		insert into api_test_run_batches(batch_id,project_id,env_name,status,total_instances,queued_instances,
		  options,triggered_by,started_at)
		values($1,$2,$3,'queued',$4,$4,$5,$6,now())
	`, batch.BatchID, batch.ProjectID, batch.EnvName, len(instances), batch.Options, batch.TriggeredBy)
	if err != nil {
		return err
	}
	for _, item := range instances {
		if _, err = tx.ExecContext(ctx, `
			insert into api_test_run_instances(batch_id,task_id,case_id,case_version,dataset_index,status,snapshot)
			values($1,$2,$3,$4,$5,'queued',$6)
		`, batch.BatchID, item.TaskID, item.CaseID, item.CaseVersion, item.DatasetIndex, item.Snapshot); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *APIAutomationRepository) MarkAPITestRunDispatched(ctx context.Context, taskID, executorID string) error {
	_, err := r.db.ExecContext(ctx, `
		update api_test_run_instances set status='running',executor_id=$2,started_at=now(),updated_at=now()
		where task_id=$1 and status='queued'
	`, taskID, executorID)
	if err == nil {
		_, err = r.db.ExecContext(ctx, `
			update api_test_run_batches b set status='running',running_instances=x.running_instances,
			  queued_instances=x.queued_instances,updated_at=now()
			from (select batch_id,count(*) filter(where status='running') running_instances,
			  count(*) filter(where status='queued') queued_instances from api_test_run_instances
			  where batch_id=(select batch_id from api_test_run_instances where task_id=$1) group by batch_id) x
			where b.batch_id=x.batch_id
		`, taskID)
	}
	return err
}

func (r *APIAutomationRepository) CompleteAPITestRunInstance(ctx context.Context, taskID, status string, result json.RawMessage, errorMessage string) (string, error) {
	var batchID string
	err := r.db.QueryRowContext(ctx, `
		update api_test_run_instances set status=$2,result=$3,error_message=$4,finished_at=now(),updated_at=now()
		where task_id=$1 returning batch_id
	`, taskID, status, result, errorMessage).Scan(&batchID)
	if err != nil {
		return "", err
	}
	_, err = r.db.ExecContext(ctx, `
		update api_test_run_batches b set
		  status=case when x.active=0 then case when x.failed>0 then 'failed' when x.canceled>0 then 'canceled' else 'success' end else 'running' end,
		  queued_instances=x.queued,running_instances=x.running,passed_instances=x.passed,
		  failed_instances=x.failed,canceled_instances=x.canceled,
		  finished_at=case when x.active=0 then now() else null end,updated_at=now()
		from (select batch_id,count(*) filter(where status='queued') queued,count(*) filter(where status='running') running,
		  count(*) filter(where status='success') passed,count(*) filter(where status='failed') failed,
		  count(*) filter(where status='canceled') canceled,count(*) filter(where status in ('queued','running')) active
		  from api_test_run_instances where batch_id=$1 group by batch_id) x
		where b.batch_id=x.batch_id
	`, batchID)
	return batchID, err
}

func (r *APIAutomationRepository) GetAPITestRunBatch(ctx context.Context, userID int64, batchID string) (model.APITestRunBatch, error) {
	var item model.APITestRunBatch
	err := r.db.QueryRowContext(ctx, `
		select b.id,b.batch_id,b.project_id,b.env_name,b.status,b.total_instances,b.queued_instances,
		  b.running_instances,b.passed_instances,b.failed_instances,b.canceled_instances,b.options,
		  b.triggered_by,b.started_at,b.finished_at,b.created_at,b.updated_at
		from api_test_run_batches b where b.batch_id=$2 and exists(
		  select 1 from users u left join roles ro on ro.id=u.role_id where u.id=$1 and
		  (ro.code='admin' or exists(select 1 from project_members pm where pm.user_id=u.id and pm.project_id=b.project_id))
		)
	`, userID, batchID).Scan(&item.ID, &item.BatchID, &item.ProjectID, &item.EnvName, &item.Status,
		&item.TotalInstances, &item.QueuedInstances, &item.RunningInstances, &item.PassedInstances,
		&item.FailedInstances, &item.CanceledInstances, &item.Options, &item.TriggeredBy,
		&item.StartedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *APIAutomationRepository) GetAPITestRunBatchInternal(ctx context.Context, batchID string) (model.APITestRunBatch, error) {
	var item model.APITestRunBatch
	err := r.db.QueryRowContext(ctx, `
		select id,batch_id,project_id,env_name,status,total_instances,queued_instances,
		  running_instances,passed_instances,failed_instances,canceled_instances,options,
		  triggered_by,started_at,finished_at,created_at,updated_at
		from api_test_run_batches where batch_id=$1
	`, batchID).Scan(&item.ID, &item.BatchID, &item.ProjectID, &item.EnvName, &item.Status,
		&item.TotalInstances, &item.QueuedInstances, &item.RunningInstances, &item.PassedInstances,
		&item.FailedInstances, &item.CanceledInstances, &item.Options, &item.TriggeredBy,
		&item.StartedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *APIAutomationRepository) ListAPITestRunInstances(ctx context.Context, batchID string) ([]model.APITestRunInstance, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id,batch_id,task_id,case_id,case_version,dataset_index,coalesce(executor_id,''),status,
		  snapshot,result,error_message,started_at,finished_at,created_at,updated_at
		from api_test_run_instances where batch_id=$1 order by id
	`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.APITestRunInstance{}
	for rows.Next() {
		var item model.APITestRunInstance
		if err := rows.Scan(&item.ID, &item.BatchID, &item.TaskID, &item.CaseID, &item.CaseVersion,
			&item.DatasetIndex, &item.ExecutorID, &item.Status, &item.Snapshot, &item.Result,
			&item.ErrorMessage, &item.StartedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
