import { DataTable } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { ResourceListPage } from "../components/ResourceListPage.js";
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

  if (section === "用户管理") {
    return <ResourceListPage title={section} description="维护平台用户与访问状态" panelTitle="用户列表" rows={pageItems(data)} columns={userColumns} loading={loading} error={error} />;
  }
  if (section === "角色管理") {
    return <ResourceListPage title={section} description="查看平台角色及权限说明" panelTitle="角色列表" rows={data || []} columns={roleColumns} loading={loading} error={error} />;
  }
  if (section === "操作日志") {
    return <ResourceListPage title={section} description="追踪平台内的关键操作记录" panelTitle="操作日志列表" rows={data || []} columns={logColumns} loading={loading} error={error} />;
  }
  return (
    <div className="section-stack">
      <PageHeader title={section} description="查看菜单与数据字典配置" />
      <StateBlock loading={loading} error={error}><ConfigOverview data={data} /></StateBlock>
    </div>
  );
}

const userColumns = [
  { key: "id", title: "ID" },
  { key: "displayName", title: "昵称" },
  { key: "username", title: "账号" },
  { key: "roleName", title: "角色" },
  { key: "email", title: "邮箱" },
  { key: "status", title: "状态" },
  { key: "createdAt", title: "创建时间", render: (row) => formatTime(row.createdAt) }
];

const roleColumns = [
  { key: "id", title: "ID" },
  { key: "name", title: "角色名称" },
  { key: "description", title: "角色描述" },
  { key: "createdAt", title: "创建时间", render: (row) => formatTime(row.createdAt) },
  { key: "updatedAt", title: "更新时间", render: (row) => formatTime(row.updatedAt) }
];

const logColumns = [
  { key: "id", title: "ID" },
  { key: "actor", title: "操作人" },
  { key: "action", title: "动作" },
  { key: "target", title: "对象" },
  { key: "createdAt", title: "时间", render: (row) => formatTime(row.createdAt) }
];

function ConfigOverview({ data }) {
  const [menus = [], dicts = []] = data || [];
  return (
    <div className="split-grid">
      <section className="resource-panel"><div className="panel-header"><strong>菜单列表</strong></div><DataTable rows={menus} columns={[{ key: "id", title: "ID" }, { key: "title", title: "菜单" }, { key: "code", title: "编码" }]} /></section>
      <section className="resource-panel"><div className="panel-header"><strong>数据字典列表</strong></div><DataTable rows={dicts} columns={[{ key: "id", title: "ID" }, { key: "name", title: "字典" }, { key: "value", title: "值" }]} /></section>
    </div>
  );
}
