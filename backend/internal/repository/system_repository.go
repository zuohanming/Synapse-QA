package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

// SystemRepository 只负责系统管理相关数据读写，不包含业务判断。
type SystemRepository struct {
	db *sql.DB
}

func NewSystemRepository(db *sql.DB) *SystemRepository {
	return &SystemRepository{db: db}
}

func (r *SystemRepository) Count(ctx context.Context, table string) int64 {
	var value int64
	_ = r.db.QueryRowContext(ctx, fmt.Sprintf("select count(*) from %s where deleted_at is null", table)).Scan(&value)
	return value
}

func (r *SystemRepository) SystemOverview(ctx context.Context, offlineSeconds int) (model.SystemOverview, error) {
	var result model.SystemOverview
	if err := r.db.QueryRowContext(ctx, `
		select
		  (select count(*) from users where deleted_at is null),
		  (select count(*) from users where deleted_at is null and status='active'),
		  (select count(*) from roles where deleted_at is null),
		  (select count(*) from executors),
		  (select count(*) from executors where last_heartbeat_at >= now()-make_interval(secs => $1)),
		  (select count(*) from execution_runs where created_at >= current_date)
		    +(select count(*) from api_test_run_batches where created_at >= current_date),
		  (select count(*) from execution_runs where created_at >= current_date and status='failed')
		    +(select count(*) from api_test_run_batches where created_at >= current_date and status='failed')
	`, offlineSeconds).Scan(&result.Users, &result.ActiveUsers, &result.Roles, &result.TotalExecutors, &result.OnlineExecutors, &result.RunsToday, &result.FailuresToday); err != nil {
		return result, err
	}
	logs, _, err := r.ListOperationLogs(ctx, model.OperationLogFilter{Page: 1, PageSize: 6})
	if err != nil {
		return result, err
	}
	result.RecentLogs = logs
	rows, err := r.db.QueryContext(ctx, systemRunTrendQuery())
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.RunTrend = []model.SystemRunTrendItem{}
	for rows.Next() {
		var item model.SystemRunTrendItem
		if err := rows.Scan(&item.Date, &item.Total, &item.Failed); err != nil {
			return result, err
		}
		result.RunTrend = append(result.RunTrend, item)
	}
	return result, rows.Err()
}

func systemRunTrendQuery() string {
	return `
		with days as (
		  select generate_series(current_date-6,current_date,'1 day')::date as run_date
		), runs as (
		  select created_at::date as run_date,status from execution_runs where created_at >= current_date-6
		  union all
		  select created_at::date as run_date,status from api_test_run_batches where created_at >= current_date-6
		)
		select to_char(days.run_date,'MM-DD'),count(runs.run_date),count(*) filter(where runs.status='failed')
		from days left join runs on runs.run_date=days.run_date
		group by days.run_date order by days.run_date
	`
}

func (r *SystemRepository) ListOperationLogs(ctx context.Context, filter model.OperationLogFilter) ([]model.OperationLog, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(value, clause string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		args = append(args, strings.TrimSpace(value))
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	add(filter.Actor, "actor ilike '%%' || $%d || '%%'")
	add(filter.Action, "action ilike '%%' || $%d || '%%'")
	add(filter.Keyword, "(target ilike '%%' || $%d || '%%')")
	add(filter.DateFrom, "created_at >= $%d::date")
	add(filter.DateTo, "created_at < $%d::date + interval '1 day'")
	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "select count(*) from operation_logs where "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := r.db.QueryContext(ctx, `
		select id,actor,action,target,ip,created_at from operation_logs where `+whereSQL+`
		order by id desc limit $`+fmt.Sprint(len(args)-1)+` offset $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.OperationLog{}
	for rows.Next() {
		var item model.OperationLog
		if err := rows.Scan(&item.ID, &item.Actor, &item.Action, &item.Target, &item.IP, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *SystemRepository) FindUserByUsername(ctx context.Context, username string) (model.User, string, error) {
	var u model.User
	var passwordHash string
	err := r.db.QueryRowContext(ctx, `
		select u.id, u.username, u.password_hash, u.display_name, u.email, u.status, coalesce(r.name, ''), coalesce(r.code, ''), u.role_id,
		  u.must_change_password,u.mcp_api_key, u.last_login_ip, u.last_login_at, u.locked_until, u.created_at
		from users u left join roles r on r.id = u.role_id
		where u.username = $1 and u.deleted_at is null
	`, username).Scan(&u.ID, &u.Username, &passwordHash, &u.DisplayName, &u.Email, &u.Status, &u.RoleName, &u.RoleCode,
		&u.RoleID, &u.MustChangePassword, &u.MCPAPIKey, &u.LastLoginIP, &u.LastLoginAt, &u.LockedUntil, &u.CreatedAt)
	return u, passwordHash, err
}

func (r *SystemRepository) CreateUser(ctx context.Context, req model.RegisterRequest, passwordHash string, apiKey string) error {
	_, err := r.db.ExecContext(ctx, `
		insert into users(username, password_hash, display_name, email, mcp_api_key)
		values($1, $2, $3, $4, $5)
	`, req.Username, passwordHash, req.DisplayName, req.Email, apiKey)
	return err
}

func (r *SystemRepository) CreateManagedUser(ctx context.Context, req model.UserCreateRequest, passwordHash, apiKey string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		insert into users(username,password_hash,display_name,email,status,role_id,mcp_api_key,must_change_password)
		select $1,$2,$3,$4,'active',r.id,$5,true from roles r
		where r.id=$6 and r.deleted_at is null and r.status='active'
		returning id
	`, req.Username, passwordHash, req.DisplayName, req.Email, apiKey, req.RoleID).Scan(&id)
	return id, err
}

func (r *SystemRepository) ResetUserPassword(ctx context.Context, id int64, passwordHash string) (string, int64, error) {
	var username string
	err := r.db.QueryRowContext(ctx, `
		update users set password_hash=$2,must_change_password=true,auth_version=auth_version+1
		where id=$1 and deleted_at is null returning username
	`, id, passwordHash).Scan(&username)
	if err != nil {
		return "", 0, err
	}
	return username, 1, nil
}

func (r *SystemRepository) ChangePassword(ctx context.Context, userID int64, passwordHash string) (int64, error) {
	var authVersion int64
	err := r.db.QueryRowContext(ctx, `
		update users set password_hash=$2,must_change_password=false,auth_version=auth_version+1
		where id=$1 and deleted_at is null and status='active' returning auth_version
	`, userID, passwordHash).Scan(&authVersion)
	return authVersion, err
}

func (r *SystemRepository) RecordLogin(ctx context.Context, userID int64, ip string) error {
	_, err := r.db.ExecContext(ctx, `update users set last_login_ip=$1,last_login_at=now(),failed_login_count=0,locked_until=null where id=$2`, ip, userID)
	return err
}

func (r *SystemRepository) RecordLoginFailure(ctx context.Context, username string, limit, lockMinutes int) error {
	_, err := r.db.ExecContext(ctx, `
		update users
		set failed_login_count=failed_login_count+1,
		    locked_until=case when failed_login_count+1 >= $2 then now()+make_interval(mins => $3) else locked_until end
		where username=$1 and deleted_at is null
	`, username, limit, lockMinutes)
	return err
}

func (r *SystemRepository) UnlockUser(ctx context.Context, id int64) (string, error) {
	var username string
	err := r.db.QueryRowContext(ctx, `
		update users set failed_login_count=0,locked_until=null
		where id=$1 and deleted_at is null returning username
	`, id).Scan(&username)
	return username, err
}

func (r *SystemRepository) LogOperation(ctx context.Context, actor, action, target string) error {
	_, err := r.db.ExecContext(ctx, `insert into operation_logs(actor, action, target) values($1, $2, $3)`, actor, action, target)
	return err
}

func (r *SystemRepository) ListUsers(ctx context.Context, id, nickname, account string, page, pageSize int) ([]model.User, int64, error) {
	where := []string{"u.deleted_at is null"}
	args := []any{}
	if id != "" {
		args = append(args, id)
		where = append(where, fmt.Sprintf("cast(u.id as text) like '%%' || $%d || '%%'", len(args)))
	}
	if nickname != "" {
		args = append(args, nickname)
		where = append(where, fmt.Sprintf("u.display_name ilike '%%' || $%d || '%%'", len(args)))
	}
	if account != "" {
		args = append(args, account)
		where = append(where, fmt.Sprintf("u.username ilike '%%' || $%d || '%%'", len(args)))
	}

	whereSQL := strings.Join(where, " and ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `select count(*) from users u where `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, `
		select u.id, u.username, u.display_name, u.email, u.status, coalesce(r.name, ''), coalesce(r.code, ''), u.role_id,
		  u.must_change_password,u.mcp_api_key, u.last_login_ip, u.last_login_at, u.locked_until, u.created_at
		from users u left join roles r on r.id = u.role_id
		where `+whereSQL+`
		order by u.id desc
		limit $`+fmt.Sprint(len(args)-1)+` offset $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Status, &u.RoleName, &u.RoleCode,
			&u.RoleID, &u.MustChangePassword, &u.MCPAPIKey, &u.LastLoginIP, &u.LastLoginAt, &u.LockedUntil, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *SystemRepository) UserAccess(ctx context.Context, userID int64) (string, string, int64, []string, error) {
	var status, roleCode string
	var authVersion int64
	if err := r.db.QueryRowContext(ctx, `
		select u.status,coalesce(ro.code,''),u.auth_version from users u
		left join roles ro on ro.id=u.role_id and ro.deleted_at is null and ro.status='active'
		where u.id=$1 and u.deleted_at is null
	`, userID).Scan(&status, &roleCode, &authVersion); err != nil {
		return "", "", 0, nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		select p.code from role_permissions rp join permissions p on p.id=rp.permission_id
		join roles ro on ro.id=rp.role_id
		join users u on u.role_id=ro.id
		where u.id=$1 and u.deleted_at is null and ro.deleted_at is null and ro.status='active'
		order by p.code
	`, userID)
	if err != nil {
		return "", "", 0, nil, err
	}
	defer rows.Close()
	permissions := []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return "", "", 0, nil, err
		}
		permissions = append(permissions, code)
	}
	return status, roleCode, authVersion, permissions, rows.Err()
}

func (r *SystemRepository) GetUserEditState(ctx context.Context, id int64) (status, displayName, email string, roleID sql.NullInt64, username string, err error) {
	err = r.db.QueryRowContext(ctx, `select status, display_name, email, role_id, username from users where id = $1 and deleted_at is null`, id).Scan(&status, &displayName, &email, &roleID, &username)
	return
}

func (r *SystemRepository) UpdateUser(ctx context.Context, id int64, status, displayName, email string, roleID any) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		update users set status=$1,display_name=$2,email=$3,role_id=$4,
		  auth_version=case when status<>$1 or role_id is distinct from $4 then auth_version+1 else auth_version end
		where id=$5 and deleted_at is null
	`, status, displayName, email, roleID, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *SystemRepository) IsOnlyActiveAdmin(ctx context.Context, userID int64) bool {
	var only bool
	_ = r.db.QueryRowContext(ctx, `
		select exists(
		  select 1 from users target join roles target_role on target_role.id=target.role_id
		  where target.id=$1 and target.deleted_at is null and target.status='active' and target_role.code='admin'
		    and (select count(*) from users u join roles r on r.id=u.role_id
		      where u.deleted_at is null and u.status='active' and r.code='admin' and r.deleted_at is null)=1
		)
	`, userID).Scan(&only)
	return only
}

func (r *SystemRepository) DeleteUser(ctx context.Context, id int64) (string, int64, error) {
	var username string
	if err := r.db.QueryRowContext(ctx, `select username from users where id = $1 and deleted_at is null`, id).Scan(&username); err != nil {
		return "", 0, err
	}
	result, err := r.db.ExecContext(ctx, `update users set deleted_at = now() where id = $1 and deleted_at is null`, id)
	if err != nil {
		return "", 0, err
	}
	rows, err := result.RowsAffected()
	return username, rows, err
}

func (r *SystemRepository) ListRoles(ctx context.Context) ([]model.Role, error) {
	rows, err := r.db.QueryContext(ctx, `
		select r.id,r.name,r.code,r.description,r.built_in,r.status,r.created_at,r.updated_at,
		  coalesce(string_agg(p.code,',' order by p.code),'')
		from roles r left join role_permissions rp on rp.role_id=r.id
		left join permissions p on p.id=rp.permission_id
		where r.deleted_at is null group by r.id order by r.id desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []model.Role
	for rows.Next() {
		var role model.Role
		var permissions string
		if err := rows.Scan(&role.ID, &role.Name, &role.Code, &role.Description, &role.BuiltIn, &role.Status,
			&role.CreatedAt, &role.UpdatedAt, &permissions); err != nil {
			return nil, err
		}
		if permissions != "" {
			role.Permissions = strings.Split(permissions, ",")
		} else {
			role.Permissions = []string{}
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *SystemRepository) CreateRole(ctx context.Context, req model.RoleRequest) error {
	code := "role_" + fmt.Sprintf("%x", time.Now().UnixNano())
	var id int64
	if err := r.db.QueryRowContext(ctx, `insert into roles(name,code,description,status) values($1,$2,$3,$4) returning id`, req.Name, code, req.Description, req.Status).Scan(&id); err != nil {
		return err
	}
	return r.ReplaceRolePermissions(ctx, id, req.Permissions)
}

func (r *SystemRepository) FindRoleCodeName(ctx context.Context, id int64) (code, name string, err error) {
	err = r.db.QueryRowContext(ctx, `select code, name from roles where id = $1 and deleted_at is null`, id).Scan(&code, &name)
	return
}

func (r *SystemRepository) CountRoleUsers(ctx context.Context, id int64) int64 {
	var count int64
	_ = r.db.QueryRowContext(ctx, `select count(*) from users where role_id = $1 and deleted_at is null`, id).Scan(&count)
	return count
}

func (r *SystemRepository) UpdateRole(ctx context.Context, id int64, req model.RoleRequest) error {
	_, err := r.db.ExecContext(ctx, `update roles set name=$1,description=$2,status=$3,updated_at=now() where id=$4 and code<>'admin'`, req.Name, req.Description, req.Status, id)
	if err != nil {
		return err
	}
	return r.ReplaceRolePermissions(ctx, id, req.Permissions)
}

func (r *SystemRepository) DeleteRole(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `update roles set deleted_at = now(), updated_at = now() where id = $1`, id)
	return err
}

func (r *SystemRepository) ListPermissions(ctx context.Context) ([]model.Permission, error) {
	rows, err := r.db.QueryContext(ctx, `select id,code,name,description from permissions order by code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.Permission{}
	for rows.Next() {
		var item model.Permission
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Description); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SystemRepository) ReplaceRolePermissions(ctx context.Context, roleID int64, codes []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `delete from role_permissions where role_id=$1`, roleID); err != nil {
		return err
	}
	for _, code := range codes {
		if _, err = tx.ExecContext(ctx, `
			insert into role_permissions(role_id,permission_id)
			select $1,id from permissions where code=$2 on conflict do nothing
		`, roleID, code); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `update users set auth_version=auth_version+1 where role_id=$1 and deleted_at is null`, roleID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SystemRepository) ListSystemSettingGroups(ctx context.Context) ([]model.SystemSettingGroup, error) {
	rows, err := r.db.QueryContext(ctx, `
		select group_key,value,revision,updated_by,created_at,updated_at
		from system_setting_groups order by group_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.SystemSettingGroup{}
	for rows.Next() {
		var item model.SystemSettingGroup
		if err := rows.Scan(&item.GroupKey, &item.Value, &item.Revision, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SystemRepository) GetSystemSettingGroup(ctx context.Context, groupKey string) (model.SystemSettingGroup, error) {
	var item model.SystemSettingGroup
	err := r.db.QueryRowContext(ctx, `
		select group_key,value,revision,updated_by,created_at,updated_at
		from system_setting_groups where group_key=$1
	`, groupKey).Scan(&item.GroupKey, &item.Value, &item.Revision, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *SystemRepository) UpdateSystemSettingGroup(ctx context.Context, groupKey string, value json.RawMessage, revision int64, actor, summary string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var currentValue json.RawMessage
	if err = tx.QueryRowContext(ctx, `
		select value from system_setting_groups where group_key=$1 and revision=$2 for update
	`, groupKey, revision).Scan(&currentValue); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
		insert into system_setting_history(group_key,revision,value,change_summary,created_by)
		values($1,$2,$3,'变更前快照',$4) on conflict(group_key,revision) do nothing
	`, groupKey, revision, currentValue, actor); err != nil {
		return 0, err
	}
	var nextRevision int64
	err = tx.QueryRowContext(ctx, `
		update system_setting_groups set value=$2,revision=revision+1,updated_by=$3,updated_at=now()
		where group_key=$1 and revision=$4 returning revision
	`, groupKey, value, actor, revision).Scan(&nextRevision)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
		insert into system_setting_history(group_key,revision,value,change_summary,created_by)
		values($1,$2,$3,$4,$5)
	`, groupKey, nextRevision, value, summary, actor); err != nil {
		return 0, err
	}
	return nextRevision, tx.Commit()
}

func (r *SystemRepository) ListSystemSettingHistory(ctx context.Context, groupKey string) ([]model.SystemSettingHistory, error) {
	rows, err := r.db.QueryContext(ctx, `
		select id,group_key,revision,value,change_summary,created_by,created_at
		from system_setting_history where group_key=$1 order by revision desc limit 50
	`, groupKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.SystemSettingHistory{}
	for rows.Next() {
		var item model.SystemSettingHistory
		if err := rows.Scan(&item.ID, &item.GroupKey, &item.Revision, &item.Value, &item.ChangeSummary, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SystemRepository) SystemSettingHistory(ctx context.Context, groupKey string, revision int64) (json.RawMessage, error) {
	var value json.RawMessage
	err := r.db.QueryRowContext(ctx, `
		select value from system_setting_history where group_key=$1 and revision=$2
	`, groupKey, revision).Scan(&value)
	return value, err
}
