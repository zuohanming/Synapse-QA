package main

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

func (a *app) migrateLegacyAPIData(ctx context.Context) error {
	if err := a.seedAPIPermissions(ctx); err != nil {
		return err
	}
	if err := a.migrateLegacyInterfaces(ctx); err != nil {
		return err
	}
	return a.migrateLegacyProjectHeaders(ctx)
}

func (a *app) seedAPIPermissions(ctx context.Context) error {
	permissions := [][]string{
		{"menu.home.read", "访问首页"},
		{"menu.ui_automation.read", "访问界面自动化"},
		{"menu.api_automation.read", "访问接口自动化"},
		{"menu.test_config.read", "访问测试配置"},
		{"menu.execution.read", "访问执行中心"},
		{"menu.system.read", "访问系统管理"},
		{"system.overview.read", "查看系统概览"},
		{"system.user.read", "查看用户"},
		{"system.user.manage", "维护用户"},
		{"system.role.read", "查看角色权限"},
		{"system.role.manage", "维护角色权限"},
		{"system.settings.read", "查看系统参数"},
		{"system.settings.manage", "维护系统参数"},
		{"system.notification.manage", "维护通知配置"},
		{"system.audit.read", "查看操作日志"},
		{"system.audit.export", "导出操作日志"},
		{"system.appearance.read", "查看个人外观"},
		{"system.appearance.manage", "维护个人外观"},
		{"api.interface.read", "查看接口"},
		{"api.interface.write", "维护接口"},
		{"api.interface.debug", "调试接口"},
		{"api.interface.delete", "删除接口"},
		{"api.project_header.manage", "维护项目请求头"},
	}
	for _, permission := range permissions {
		if _, err := a.db.ExecContext(ctx, `insert into permissions(code,name) values($1,$2) on conflict(code) do update set name=excluded.name`, permission[0], permission[1]); err != nil {
			return err
		}
	}
	if _, err := a.db.ExecContext(ctx, `
		insert into role_permissions(role_id,permission_id)
		select r.id,p.id from roles r cross join permissions p
		where r.code='admin'
		on conflict do nothing
	`); err != nil {
		return err
	}
	if _, err := a.db.ExecContext(ctx, `
		insert into role_permissions(role_id,permission_id)
		select r.id,p.id from roles r cross join permissions p
		where r.code in ('qa_lead','automation_engineer')
		  and (p.code in ('menu.home.read','menu.ui_automation.read','menu.api_automation.read',
		    'menu.test_config.read','menu.execution.read','menu.system.read','system.appearance.read','system.appearance.manage')
		    or p.code like 'api.%')
		on conflict do nothing
	`); err != nil {
		return err
	}
	if _, err := a.db.ExecContext(ctx, `
		insert into role_permissions(role_id,permission_id)
		select r.id,p.id from roles r cross join permissions p
		where r.code='viewer' and p.code in ('menu.home.read','menu.ui_automation.read',
		  'menu.api_automation.read','menu.execution.read','menu.system.read','system.appearance.read','system.appearance.manage',
		  'api.interface.read')
		on conflict do nothing
	`); err != nil {
		return err
	}
	_, err := a.db.ExecContext(ctx, `
		insert into project_members(user_id,project_id,role_id)
		select u.id,p.id,r.id from users u join roles r on r.code='admin' cross join projects p
		where u.username='admin'
		on conflict(user_id,project_id) do update set role_id=excluded.role_id
	`)
	return err
}

func (a *app) migrateLegacyInterfaces(ctx context.Context) error {
	rows, err := a.db.QueryContext(ctx, `
		select id,name,category,method,locator,action,value,description,status,created_by
		from ui_assets u
		where asset_type='api_interface' and deleted_at is null
		and not exists(select 1 from api_legacy_migrations m where m.source_type='interface' and m.source_id=u.id)
		order by id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name, category, method, locator, moduleName, protocol, description, status, actor string
		if err := rows.Scan(&id, &name, &category, &method, &locator, &moduleName, &protocol, &description, &status, &actor); err != nil {
			return err
		}
		productID, parseErr := strconv.ParseInt(strings.TrimSpace(category), 10, 64)
		if parseErr != nil {
			a.recordLegacyMigration(ctx, "interface", id, 0, "error", "无法解析产品 ID")
			continue
		}
		var moduleID int64
		_ = a.db.QueryRowContext(ctx, `select id from product_modules where product_id=$1 and name=$2 and deleted_at is null limit 1`, productID, moduleName).Scan(&moduleID)
		endpointType := "WEB"
		configuration := json.RawMessage(`{}`)
		var meta map[string]any
		if json.Unmarshal([]byte(description), &meta) == nil {
			if value, ok := meta["endpointType"].(string); ok {
				endpointType = value
			}
			configuration, _ = json.Marshal(meta)
		} else if strings.TrimSpace(description) != "" {
			endpointType = description
		}
		lifecycle := "draft"
		if status == "active" {
			lifecycle = "active"
		} else if status == "disabled" {
			lifecycle = "disabled"
		}
		if actor == "" {
			actor = "migration"
		}
		if protocol == "" {
			protocol = "HTTP"
		}
		normalized := normalizeLegacyPath(locator)
		tx, err := a.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		var targetID int64
		err = tx.QueryRowContext(ctx, `
			insert into api_interfaces(product_id,module_id,name,method,path,normalized_path,protocol,endpoint_type,lifecycle_status,configuration,created_by,updated_by)
			values($1,nullif($2,0),$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)
			on conflict do nothing returning id
		`, productID, moduleID, name, strings.ToUpper(method), locator, normalized, strings.ToUpper(protocol), strings.ToUpper(endpointType), lifecycle, configuration, actor).Scan(&targetID)
		if err != nil {
			tx.Rollback()
			a.recordLegacyMigration(ctx, "interface", id, 0, "conflict", "方法和路径重复或关联数据无效")
			continue
		}
		snapshot, _ := json.Marshal(map[string]any{"legacyId": id, "name": name, "productId": productID, "moduleId": moduleID, "method": strings.ToUpper(method), "path": locator, "configuration": meta})
		if _, err = tx.ExecContext(ctx, `insert into api_interface_versions(interface_id,version,snapshot,change_summary,created_by) values($1,1,$2,'旧数据迁移',$3)`, targetID, snapshot, actor); err != nil {
			tx.Rollback()
			return err
		}
		if _, err = tx.ExecContext(ctx, `insert into api_legacy_migrations(source_type,source_id,target_id,status) values('interface',$1,$2,'success')`, id, targetID); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (a *app) migrateLegacyProjectHeaders(ctx context.Context) error {
	rows, err := a.db.QueryContext(ctx, `
		select id,name,category,value,description,status,created_by
		from ui_assets u
		where asset_type='api_request_header' and deleted_at is null
		and not exists(select 1 from api_legacy_migrations m where m.source_type='project_header' and m.source_id=u.id)
		order by id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name, category, value, description, status, actor string
		if err := rows.Scan(&id, &name, &category, &value, &description, &status, &actor); err != nil {
			return err
		}
		projectID, parseErr := strconv.ParseInt(strings.TrimSpace(category), 10, 64)
		if parseErr != nil || strings.TrimSpace(name) == "" {
			a.recordLegacyMigration(ctx, "project_header", id, 0, "error", "无法解析项目或请求头名称")
			continue
		}
		if actor == "" {
			actor = "migration"
		}
		var targetID int64
		err := a.db.QueryRowContext(ctx, `
			insert into api_project_headers(project_id,header_name,header_name_normalized,header_value,description,enabled,sensitive,created_by,updated_by)
			values($1,$2,$3,$4,$5,$6,$7,$8,$8)
			on conflict do nothing returning id
		`, projectID, strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(name)), value, description, status == "active", isSensitiveHeader(name), actor).Scan(&targetID)
		if err != nil {
			a.recordLegacyMigration(ctx, "project_header", id, 0, "conflict", "同项目请求头重复或项目无效")
			continue
		}
		a.recordLegacyMigration(ctx, "project_header", id, targetID, "success", "")
	}
	return rows.Err()
}

func (a *app) recordLegacyMigration(ctx context.Context, sourceType string, sourceID, targetID int64, status, message string) {
	_, _ = a.db.ExecContext(ctx, `insert into api_legacy_migrations(source_type,source_id,target_id,status,message) values($1,$2,nullif($3,0),$4,$5) on conflict(source_type,source_id) do update set target_id=excluded.target_id,status=excluded.status,message=excluded.message`, sourceType, sourceID, targetID, status, message)
}

func normalizeLegacyPath(value string) string {
	value = strings.TrimSpace(strings.Split(value, "?")[0])
	if value == "" {
		return "/"
	}
	if strings.HasSuffix(value, "/") && value != "/" {
		value = strings.TrimSuffix(value, "/")
	}
	return value
}

func isSensitiveHeader(name string) bool {
	value := strings.ToLower(name)
	for _, keyword := range []string{"authorization", "cookie", "token", "secret", "password", "api-key"} {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}
