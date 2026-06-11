import { DataTable } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { systemService } from "../services/systemService.js";
import { formatTime, pageItems } from "../utils/formatters.js";

export function SystemPage({ activePath }) {
  const section = activePath[1];
  const loaders = {
    配置管理: () => Promise.all([systemService.menus(), systemService.dictionaries()]),
    用户管理: () => systemService.users({ page: 1, pageSize: 20 }),
    角色管理: () => systemService.roles(),
    操作日志: () => systemService.logs()
  };
  const { data, loading, error } = useAsyncData(loaders[section] || loaders.配置管理, [section]);

  return (
    <>
      <PageHeader title={section} description="系统管理数据维护" />
      <StateBlock loading={loading} error={error}>
        {section === "用户管理" ? <UsersTable result={data} /> : null}
        {section === "角色管理" ? <RolesTable rows={data || []} /> : null}
        {section === "操作日志" ? <LogsTable rows={data || []} /> : null}
        {section === "配置管理" ? <ConfigOverview data={data} /> : null}
      </StateBlock>
    </>
  );
}

function UsersTable({ result }) {
  return (
    <DataTable
      rows={pageItems(result)}
      columns={[
        { key: "id", title: "ID" },
        { key: "displayName", title: "昵称" },
        { key: "username", title: "账号" },
        { key: "roleName", title: "角色" },
        { key: "email", title: "邮箱" },
        { key: "status", title: "状态" },
        { key: "createdAt", title: "创建时间", render: (row) => formatTime(row.createdAt) }
      ]}
    />
  );
}

function RolesTable({ rows }) {
  return (
    <DataTable
      rows={rows}
      columns={[
        { key: "id", title: "ID" },
        { key: "name", title: "角色名称" },
        { key: "description", title: "角色描述" },
        { key: "createdAt", title: "创建时间", render: (row) => formatTime(row.createdAt) },
        { key: "updatedAt", title: "更新时间", render: (row) => formatTime(row.updatedAt) }
      ]}
    />
  );
}

function LogsTable({ rows }) {
  return (
    <DataTable
      rows={rows}
      columns={[
        { key: "id", title: "ID" },
        { key: "actor", title: "操作人" },
        { key: "action", title: "动作" },
        { key: "target", title: "对象" },
        { key: "createdAt", title: "时间", render: (row) => formatTime(row.createdAt) }
      ]}
    />
  );
}

function ConfigOverview({ data }) {
  const [menus = [], dicts = []] = data || [];
  return (
    <div className="split-grid">
      <DataTable rows={menus} columns={[{ key: "id", title: "ID" }, { key: "title", title: "菜单" }, { key: "code", title: "编码" }]} />
      <DataTable rows={dicts} columns={[{ key: "id", title: "ID" }, { key: "name", title: "字典" }, { key: "value", title: "值" }]} />
    </div>
  );
}
