package repository

import (
	"context"
	"database/sql"
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

func (r *SystemRepository) FindUserByUsername(ctx context.Context, username string) (model.User, string, error) {
	var u model.User
	var passwordHash string
	err := r.db.QueryRowContext(ctx, `
		select u.id, u.username, u.password_hash, u.display_name, u.email, u.status, coalesce(r.name, ''), u.mcp_api_key, u.last_login_ip, u.last_login_at, u.created_at
		from users u left join roles r on r.id = u.role_id
		where u.username = $1 and u.deleted_at is null
	`, username).Scan(&u.ID, &u.Username, &passwordHash, &u.DisplayName, &u.Email, &u.Status, &u.RoleName, &u.MCPAPIKey, &u.LastLoginIP, &u.LastLoginAt, &u.CreatedAt)
	return u, passwordHash, err
}

func (r *SystemRepository) CreateUser(ctx context.Context, req model.RegisterRequest, passwordHash string, apiKey string) error {
	_, err := r.db.ExecContext(ctx, `
		insert into users(username, password_hash, display_name, email, mcp_api_key)
		values($1, $2, $3, $4, $5)
	`, req.Username, passwordHash, req.DisplayName, req.Email, apiKey)
	return err
}

func (r *SystemRepository) RecordLogin(ctx context.Context, userID int64, ip string) error {
	_, err := r.db.ExecContext(ctx, `update users set last_login_ip = $1, last_login_at = now() where id = $2`, ip, userID)
	return err
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
		select u.id, u.username, u.display_name, u.email, u.status, coalesce(r.name, ''), u.mcp_api_key, u.last_login_ip, u.last_login_at, u.created_at
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
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Status, &u.RoleName, &u.MCPAPIKey, &u.LastLoginIP, &u.LastLoginAt, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *SystemRepository) GetUserEditState(ctx context.Context, id int64) (status, displayName, email string, roleID sql.NullInt64, username string, err error) {
	err = r.db.QueryRowContext(ctx, `select status, display_name, email, role_id, username from users where id = $1 and deleted_at is null`, id).Scan(&status, &displayName, &email, &roleID, &username)
	return
}

func (r *SystemRepository) UpdateUser(ctx context.Context, id int64, status, displayName, email string, roleID any) (int64, error) {
	result, err := r.db.ExecContext(ctx, `update users set status = $1, display_name = $2, email = $3, role_id = $4 where id = $5 and deleted_at is null`, status, displayName, email, roleID, id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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
	rows, err := r.db.QueryContext(ctx, `select id, name, code, description, created_at, updated_at from roles where deleted_at is null order by id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []model.Role
	for rows.Next() {
		var role model.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Code, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *SystemRepository) CreateRole(ctx context.Context, req model.RoleRequest) error {
	code := "role_" + fmt.Sprintf("%x", time.Now().UnixNano())
	_, err := r.db.ExecContext(ctx, `insert into roles(name, code, description) values($1, $2, $3)`, req.Name, code, req.Description)
	return err
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
	_, err := r.db.ExecContext(ctx, `update roles set name = $1, description = $2, updated_at = now() where id = $3`, req.Name, req.Description, id)
	return err
}

func (r *SystemRepository) DeleteRole(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `update roles set deleted_at = now(), updated_at = now() where id = $1`, id)
	return err
}
