import { Check, Code2, Copy, KeyRound, LayoutTemplate, Plus, Shield, Type, UserRoundCog, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { DataTable, PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { downloadOperationLogs, systemService } from "../services/systemService.js";
import { formatTime, pageItems } from "../utils/formatters.js";
import { applyAppearance, readAppearance } from "../utils/appearance.js";

export function SystemPage({ activePath }) {
  const section = activePath[1];
  const loaders = {
    系统概览: () => systemService.overview(),
    系统参数: () => Promise.resolve(null),
    通知配置: () => Promise.resolve(null),
    配置管理: () => Promise.all([systemService.menus(), systemService.dictionaries()]),
    外观设置: () => Promise.resolve(null),
    用户管理: () => Promise.resolve(null),
    角色管理: () => Promise.resolve(null),
    操作日志: () => Promise.resolve(null)
  };
  const { data, loading, error } = useAsyncData(loaders[section] || loaders.配置管理, [section]);

  if (section === "系统参数") {
    return <SystemSettingsPage initialGroup="execution" />;
  }
  if (section === "系统概览") {
    return <SystemOverviewPage data={data} loading={loading} error={error} />;
  }
  if (section === "通知配置") {
    return <SystemSettingsPage initialGroup="notification" notificationOnly />;
  }
  if (section === "外观设置") {
    return <AppearanceSettings />;
  }

  if (section === "用户管理") {
    return <UserManagement />;
  }
  if (section === "角色管理" || section === "角色与权限") {
    return <RoleManagement />;
  }
  if (section === "操作日志") {
    return <OperationLogPage />;
  }
  return (
    <div className="section-stack">
      <PageHeader title={section} description="查看菜单与数据字典配置" />
      <StateBlock loading={loading} error={error}><ConfigOverview data={data} /></StateBlock>
    </div>
  );
}

function SystemOverviewPage({ data, loading, error }) {
  const trend = data?.runTrend || [];
  const maxRuns = Math.max(1, ...trend.map((item) => Number(item.total)));
  const health = data?.totalExecutors ? Math.round(Number(data.onlineExecutors) / Number(data.totalExecutors) * 100) : 0;
  return <div className="section-stack system-overview-page">
    <PageHeader title="系统概览" description="查看平台容量、执行活跃度和最近管理动作" />
    <StateBlock loading={loading} error={error}>
      <section className="system-vital-strip">
        <div className="system-health-score"><span>运行态势</span><strong>{health}<small>%</small></strong><p>{data?.onlineExecutors || 0} / {data?.totalExecutors || 0} 个执行器在线</p></div>
        <div className="system-vital-metrics">
          <article><span>今日执行</span><strong>{data?.runsToday || 0}</strong><small>跨 UI 与接口自动化</small></article>
          <article><span>今日失败</span><strong>{data?.failuresToday || 0}</strong><small>需要关注的执行批次</small></article>
          <article><span>活跃账号</span><strong>{data?.activeUsers || 0}</strong><small>共 {data?.users || 0} 个账号</small></article>
          <article><span>权限角色</span><strong>{data?.roles || 0}</strong><small>当前系统角色</small></article>
        </div>
      </section>
      <div className="system-overview-grid">
        <section className="resource-panel system-run-pulse"><header><div><strong>近 7 日执行脉冲</strong><span>总执行量与失败量</span></div><b>7 DAYS</b></header><div className="run-pulse-chart">{trend.map((item) => <div key={item.date}><span className="pulse-column"><i style={{ height: `${Math.max(4, Number(item.total) / maxRuns * 100)}%` }} /><em style={{ height: `${Math.max(0, Number(item.failed) / maxRuns * 100)}%` }} /></span><strong>{item.total}</strong><small>{item.date}</small></div>)}</div></section>
        <section className="resource-panel system-recent-audit"><header><div><strong>最近管理动作</strong><span>最新 6 条操作审计</span></div></header>{(data?.recentLogs || []).map((item) => <article key={item.id}><span>{(item.actor || "?").slice(0, 1)}</span><div><strong>{item.action}</strong><small>{item.actor} · {item.target}</small></div><time>{formatTime(item.createdAt)}</time></article>)}</section>
      </div>
    </StateBlock>
  </div>;
}

function OperationLogPage() {
  const [filters, setFilters] = useState({ actor: "", action: "", keyword: "", dateFrom: "", dateTo: "" });
  const [query, setQuery] = useState(filters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [notice, setNotice] = useState("");
  const { data, loading, error } = useAsyncData(() => systemService.logs({ ...query, page, pageSize }), [query.actor, query.action, query.keyword, query.dateFrom, query.dateTo, page, pageSize]);
  const total = Number(data?.total || 0);
  async function exportLogs() {
    try {
      await downloadOperationLogs(query);
      setNotice("操作日志已导出。");
    } catch (requestError) {
      setNotice(requestError.message);
    }
  }
  return <div className="section-stack system-audit-page">
    <PageHeader title="操作日志" description="按人员、动作和时间追踪平台内的关键管理行为" />
    <section className="resource-panel">
      <form className="filter-grid audit-filter-grid" onSubmit={(event) => { event.preventDefault(); setPage(1); setQuery(filters); }}>
        <label className="form-field"><span>操作人</span><input className="text-input" value={filters.actor} onChange={(event) => setFilters({ ...filters, actor: event.target.value })} /></label>
        <label className="form-field"><span>动作</span><input className="text-input" value={filters.action} onChange={(event) => setFilters({ ...filters, action: event.target.value })} /></label>
        <label className="form-field"><span>目标</span><input className="text-input" value={filters.keyword} onChange={(event) => setFilters({ ...filters, keyword: event.target.value })} /></label>
        <label className="form-field"><span>开始日期</span><input className="text-input" type="date" value={filters.dateFrom} onChange={(event) => setFilters({ ...filters, dateFrom: event.target.value })} /></label>
        <label className="form-field"><span>结束日期</span><input className="text-input" type="date" value={filters.dateTo} onChange={(event) => setFilters({ ...filters, dateTo: event.target.value })} /></label>
        <div className="toolbar-row"><button className="primary-button compact-button" type="submit">查询</button><button className="icon-text-button compact-button" onClick={() => { const empty = { actor: "", action: "", keyword: "", dateFrom: "", dateTo: "" }; setFilters(empty); setQuery(empty); setPage(1); }} type="button">重置</button></div>
      </form>
      <div className="list-actions"><span className="muted-text">共 {total} 条审计记录</span><button className="icon-text-button compact-button" onClick={exportLogs} type="button">导出 CSV</button></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <StateBlock loading={loading} error={error}><TablePanel><DataTable columns={logColumns} rows={pageItems(data)} emptyText="暂无操作日志" /><PaginationBar page={page} pageSize={pageSize} total={total} totalPages={Math.max(1, Math.ceil(total / pageSize))} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} /></TablePanel></StateBlock>
    </section>
  </div>;
}

function UserManagement() {
  const [filters, setFilters] = useState({ id: "", nickname: "", account: "" });
  const [query, setQuery] = useState(filters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [modal, setModal] = useState(null);
  const [temporaryPassword, setTemporaryPassword] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const { data, loading, error, reload } = useAsyncData(() => systemService.users({ ...query, page, pageSize }), [query.id, query.nickname, query.account, page, pageSize]);
  const { data: rolesData } = useAsyncData(() => systemService.roles(), []);
  const roles = (rolesData || []).filter((role) => role.status === "active");
  const rows = pageItems(data);
  const total = Number(data?.total || 0);

  async function saveUser(payload) {
    setBusy(true);
    try {
      if (modal.mode === "edit") {
        await systemService.updateUser(modal.row.id, payload);
        setNotice("用户信息已更新，角色或状态变化会使旧登录立即失效。");
      } else {
        const result = await systemService.createUser(payload);
        setTemporaryPassword(result.temporaryPassword);
        setNotice("用户已创建。");
      }
      setModal(null);
      await reload();
    } finally {
      setBusy(false);
    }
  }

  async function toggleStatus(row) {
    if (!window.confirm(`确认${row.status === "active" ? "停用" : "启用"}用户“${row.username}”吗？`)) return;
    setBusy(true);
    try {
      await systemService.updateUser(row.id, { status: row.status === "active" ? "disabled" : "active" });
      await reload();
      setNotice(row.status === "active" ? "用户已停用，现有登录已失效。" : "用户已启用。");
    } catch (requestError) {
      setNotice(requestError.message || "更新用户状态失败");
    } finally {
      setBusy(false);
    }
  }

  async function resetPassword(row) {
    if (!window.confirm(`确认重置用户“${row.username}”的密码吗？其现有登录会立即失效。`)) return;
    setBusy(true);
    try {
      const result = await systemService.resetUserPassword(row.id);
      setTemporaryPassword(result.temporaryPassword);
    } catch (requestError) {
      setNotice(requestError.message || "重置密码失败");
    } finally {
      setBusy(false);
    }
  }

  async function unlock(row) {
    setBusy(true);
    try {
      await systemService.unlockUser(row.id);
      await reload();
      setNotice(`用户“${row.username}”已解锁。`);
    } catch (requestError) {
      setNotice(requestError.message || "解锁用户失败");
    } finally {
      setBusy(false);
    }
  }

  const columns = [
    { key: "identity", title: "用户", render: (row) => <div className="system-user-identity"><span>{(row.displayName || row.username).slice(0, 1)}</span><div><strong>{row.displayName}</strong><small>{row.username}</small></div></div> },
    { key: "roleName", title: "系统角色", render: (row) => row.roleName || "未分配" },
    { key: "email", title: "邮箱", render: (row) => row.email || "—" },
    { key: "status", title: "状态", render: (row) => row.lockedUntil && new Date(row.lockedUntil) > new Date() ? <span className="status-pill">已锁定</span> : <span className={`status-pill ${row.status === "active" ? "success" : ""}`}>{row.status === "active" ? "已启用" : "已停用"}</span> },
    { key: "lastLoginAt", title: "最近登录", render: (row) => row.lastLoginAt ? formatTime(row.lastLoginAt) : "从未登录" },
    { key: "actions", title: "操作", render: (row) => <div className="action-row"><button className="link-button" onClick={() => setModal({ mode: "edit", row })} type="button">编辑</button><button className="link-button" onClick={() => resetPassword(row)} type="button">重置密码</button>{row.lockedUntil && new Date(row.lockedUntil) > new Date() ? <button className="link-button" onClick={() => unlock(row)} type="button">解锁</button> : null}<button className={row.status === "active" ? "danger-link" : "link-button"} disabled={row.roleCode === "admin" && row.status === "active"} onClick={() => toggleStatus(row)} title={row.roleCode === "admin" ? "超级管理员不能在此停用" : ""} type="button">{row.status === "active" ? "停用" : "启用"}</button></div> }
  ];

  return <div className="section-stack system-admin-page">
    <PageHeader title="用户管理" description="维护平台身份、系统角色和账号状态" />
    <section className="resource-panel">
      <form className="filter-grid system-user-filter" onSubmit={(event) => { event.preventDefault(); setPage(1); setQuery(filters); }}>
        <label className="form-field"><span>ID</span><input className="text-input" value={filters.id} onChange={(event) => setFilters({ ...filters, id: event.target.value })} /></label>
        <label className="form-field"><span>昵称</span><input className="text-input" value={filters.nickname} onChange={(event) => setFilters({ ...filters, nickname: event.target.value })} /></label>
        <label className="form-field"><span>账号</span><input className="text-input" value={filters.account} onChange={(event) => setFilters({ ...filters, account: event.target.value })} /></label>
        <div className="toolbar-row"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={() => { const empty = { id: "", nickname: "", account: "" }; setFilters(empty); setQuery(empty); setPage(1); }} type="button">重置</button></div>
      </form>
      <div className="list-actions"><span className="muted-text">共 {total} 个用户</span><button className="primary-button compact-button" onClick={() => setModal({ mode: "create", row: null })} type="button"><Plus size={14} />新增用户</button></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <StateBlock loading={loading} error={error}><TablePanel><DataTable columns={columns} rows={rows} emptyText="暂无用户" /><PaginationBar page={page} pageSize={pageSize} total={total} totalPages={Math.max(1, Math.ceil(total / pageSize))} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} /></TablePanel></StateBlock>
    </section>
    {modal ? <UserModal busy={busy} modal={modal} roles={roles} onClose={() => setModal(null)} onSubmit={saveUser} /> : null}
    {temporaryPassword ? <SecretResult title="一次性临时密码" value={temporaryPassword} onClose={() => setTemporaryPassword("")} /> : null}
  </div>;
}

function UserModal({ busy, modal, roles, onClose, onSubmit }) {
  const row = modal.row;
  const [form, setForm] = useState({ username: row?.username || "", displayName: row?.displayName || "", email: row?.email || "", roleId: row?.roleId || "", status: row?.status || "active" });
  const [error, setError] = useState("");
  async function submit(event) {
    event.preventDefault();
    setError("");
    try {
      const payload = { displayName: form.displayName.trim(), email: form.email.trim(), roleId: Number(form.roleId) };
      if (modal.mode === "create") payload.username = form.username.trim();
      else payload.status = form.status;
      await onSubmit(payload);
    } catch (requestError) {
      setError(requestError.message || "保存用户失败");
    }
  }
  return <div className="modal-backdrop"><form className="modal-card modal-card-small system-user-modal" onSubmit={submit}>
    <div className="modal-header"><div><strong>{modal.mode === "edit" ? "编辑用户" : "新增用户"}</strong><span>{modal.mode === "edit" ? "角色或状态变更后旧登录立即失效" : "系统将生成一次性临时密码"}</span></div><button className="modal-close" onClick={onClose} type="button"><X size={16} /></button></div>
    <div className="modal-form">
      <label className="form-field"><span>* 登录账号</span><input required disabled={modal.mode === "edit"} className="text-input" value={form.username} onChange={(event) => setForm({ ...form, username: event.target.value })} /></label>
      <label className="form-field"><span>* 用户昵称</span><input required className="text-input" value={form.displayName} onChange={(event) => setForm({ ...form, displayName: event.target.value })} /></label>
      <label className="form-field"><span>邮箱</span><input className="text-input" type="email" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} /></label>
      <label className="form-field"><span>* 系统角色</span><select required className="text-input" disabled={row?.roleCode === "admin"} value={form.roleId} onChange={(event) => setForm({ ...form, roleId: event.target.value })}><option value="">请选择角色</option>{roles.map((role) => <option key={role.id} value={role.id}>{role.name}</option>)}</select></label>
      {modal.mode === "edit" ? <label className="form-field"><span>账号状态</span><select className="text-input" disabled={row?.roleCode === "admin"} value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}><option value="active">启用</option><option value="disabled">停用</option></select></label> : null}
    </div>
    {error ? <div className="form-error modal-error">{error}</div> : null}
    <div className="modal-actions"><button className="icon-text-button compact-button" onClick={onClose} type="button">取消</button><button className="primary-button compact-button" disabled={busy} type="submit">{busy ? "保存中" : modal.mode === "edit" ? "保存修改" : "创建用户"}</button></div>
  </form></div>;
}

function SecretResult({ title, value, onClose }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    await navigator.clipboard.writeText(value);
    setCopied(true);
  }
  return <div className="modal-backdrop"><section className="modal-card modal-card-small system-secret-modal"><div className="modal-header"><div><strong>{title}</strong><span>关闭后将无法再次查看</span></div></div><div className="system-secret-value"><KeyRound size={18} /><code>{value}</code><button onClick={copy} type="button"><Copy size={14} />{copied ? "已复制" : "复制"}</button></div><div className="inline-notice">用户首次登录后必须修改此密码。</div><div className="modal-actions"><button className="primary-button compact-button" onClick={onClose} type="button">我已保存</button></div></section></div>;
}

function RoleManagement() {
  const [selectedId, setSelectedId] = useState(0);
  const [draft, setDraft] = useState(null);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const { data: rolesData, loading, error, reload } = useAsyncData(() => systemService.roles(), []);
  const { data: permissionsData } = useAsyncData(() => systemService.permissions(), []);
  const roles = rolesData || [];
  const permissions = permissionsData || [];
  const selected = roles.find((role) => role.id === selectedId) || roles[0];
  const form = draft || selected;
  const grouped = useMemo(() => groupPermissions(permissions), [permissions]);

  function edit(next) {
    setDraft({ ...form, ...next, permissions: next.permissions || form.permissions || [] });
  }
  function togglePermission(code) {
    const current = new Set(form.permissions || []);
    if (current.has(code)) {
      current.delete(code);
      if (code === "menu.system.read") [...current].filter((item) => item.startsWith("system.")).forEach((item) => current.delete(item));
      if (code === "menu.api_automation.read") [...current].filter((item) => item.startsWith("api.")).forEach((item) => current.delete(item));
    } else {
      current.add(code);
      if (code.startsWith("system.")) current.add("menu.system.read");
      if (code.startsWith("api.")) current.add("menu.api_automation.read");
    }
    edit({ permissions: [...current] });
  }
  async function save() {
    if (!form) return;
    setBusy(true);
    try {
      const payload = { name: form.name, description: form.description, status: form.status || "active", permissions: form.permissions || [] };
      if (form.id) await systemService.updateRole(form.id, payload);
      else await systemService.createRole(payload);
      setDraft(null);
      await reload();
      setNotice("角色权限已保存，关联用户需重新登录。");
    } catch (requestError) {
      setNotice(requestError.message || "保存角色失败");
    } finally {
      setBusy(false);
    }
  }
  function startCreate(source = null) {
    const next = { id: 0, name: source ? `${source.name} 副本` : "", description: source?.description || "", status: "active", permissions: [...(source?.permissions || [])] };
    setSelectedId(0);
    setDraft(next);
  }

  return <div className="section-stack system-admin-page">
    <PageHeader title="角色与权限" description="用菜单和操作权限定义平台能力边界，项目范围仍由项目成员关系控制" />
    {notice ? <div className="inline-notice">{notice}</div> : null}
    <StateBlock loading={loading} error={error}>
      <div className="role-permission-workbench">
        <aside className="role-rail">
          <header><div><strong>系统角色</strong><span>{roles.length} 个角色</span></div><button aria-label="新增角色" onClick={() => startCreate()} type="button"><Plus size={15} /></button></header>
          <div>{roles.map((role) => <button className={selected?.id === role.id && !draft ? "active" : ""} key={role.id} onClick={() => { setSelectedId(role.id); setDraft(null); }} type="button"><span><Shield size={15} /></span><div><strong>{role.name}</strong><small>{role.builtIn ? "内置角色" : "自定义角色"} · {role.permissions?.length || 0} 项权限</small></div><i className={role.status === "active" ? "online" : ""} /></button>)}</div>
        </aside>
        <main className="permission-editor">
          {form ? <>
            <header><div><span>{form.id ? form.code : "NEW ROLE"}</span><input aria-label="角色名称" className="permission-role-name" disabled={form.code === "admin"} value={form.name} onChange={(event) => edit({ name: event.target.value })} /></div><div><button className="icon-text-button compact-button" disabled={!form.id} onClick={() => startCreate(form)} type="button"><Copy size={14} />复制角色</button><button className="primary-button compact-button" disabled={busy || form.code === "admin" || !form.name.trim()} onClick={save} type="button">{busy ? "保存中" : "保存权限"}</button></div></header>
            <div className="permission-role-meta"><label><span>角色说明</span><input className="text-input" disabled={form.code === "admin"} value={form.description} onChange={(event) => edit({ description: event.target.value })} /></label><label><span>状态</span><select className="text-input" disabled={form.builtIn || form.code === "admin"} value={form.status || "active"} onChange={(event) => edit({ status: event.target.value })}><option value="active">启用</option><option value="disabled">停用</option></select></label></div>
            {form.code === "admin" ? <div className="inline-notice">超级管理员始终拥有全部权限，不允许修改。</div> : null}
            <div className="permission-groups">{grouped.map((group) => {
              const selectedCount = group.items.filter((item) => (form.permissions || []).includes(item.code)).length;
              return <section key={group.key}><header><div><strong>{group.label}</strong><span>{selectedCount} / {group.items.length}</span></div><button disabled={form.code === "admin"} onClick={() => {
                const current = new Set(form.permissions || []);
                const allSelected = selectedCount === group.items.length;
                group.items.forEach((item) => allSelected ? current.delete(item.code) : current.add(item.code));
                edit({ permissions: [...current] });
              }} type="button">{selectedCount === group.items.length ? "取消全选" : "全选"}</button></header><div>{group.items.map((permission) => <label key={permission.code}><input checked={form.code === "admin" || (form.permissions || []).includes(permission.code)} disabled={form.code === "admin"} onChange={() => togglePermission(permission.code)} type="checkbox" /><span><strong>{permission.name}</strong><small>{permission.code}</small></span></label>)}</div></section>;
            })}</div>
          </> : <div className="system-empty"><UserRoundCog size={26} /><strong>选择一个角色</strong></div>}
        </main>
      </div>
    </StateBlock>
  </div>;
}

function groupPermissions(items) {
  const labels = { menu: "菜单访问", system: "系统管理", api: "接口自动化" };
  const map = new Map();
  items.forEach((item) => {
    const key = item.code.split(".")[0];
    if (!map.has(key)) map.set(key, { key, label: labels[key] || "业务权限", items: [] });
    map.get(key).items.push(item);
  });
  return [...map.values()];
}

const settingSchemas = {
  execution: {
    title: "执行策略",
    description: "控制接口用例批量执行、并发调度和执行器离线判定",
    fields: [
      ["defaultConcurrency", "默认并发数", "未指定并发时使用", 1, 100],
      ["maxConcurrency", "最大并发数", "单批次允许的并发上限", 1, 100],
      ["batchSize", "批次用例数", "一次可选择的最大用例数", 1, 5000],
      ["executorOfflineSeconds", "离线判定（秒）", "超过该时间未心跳则不可调度", 15, 600]
    ]
  },
  security: {
    title: "安全策略",
    description: "控制会话、密码强度和连续登录失败保护",
    fields: [
      ["sessionHours", "会话有效期（小时）", "新登录凭证的最长有效时间", 1, 720],
      ["passwordMinLength", "密码最短长度", "新建及修改密码时生效", 8, 32],
      ["loginFailureLimit", "失败锁定阈值", "连续输错密码达到此次数后锁定", 3, 20],
      ["lockMinutes", "锁定时长（分钟）", "管理员也可在用户管理中提前解锁", 1, 1440]
    ]
  },
  notification: {
    title: "通知默认策略",
    description: "作为所有用户的默认订阅；系统告警与安全告警始终开启",
    fields: [
      ["executionSuccess", "执行成功", "测试批次执行成功时通知"],
      ["executionFailure", "执行失败", "测试批次执行失败时通知"],
      ["executorOffline", "执行器离线", "执行器断连或停止时通知"],
      ["systemAlert", "系统告警", "平台关键异常，固定开启", true],
      ["securityAlert", "安全告警", "账号与权限风险，固定开启", true]
    ]
  }
};

function SystemSettingsPage({ initialGroup, notificationOnly = false }) {
  const [groupKey, setGroupKey] = useState(initialGroup);
  const [groups, setGroups] = useState([]);
  const [draft, setDraft] = useState({});
  const [history, setHistory] = useState([]);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const { loading, error, reload } = useAsyncData(async () => {
    const items = await systemService.settings();
    setGroups(items || []);
    return items;
  }, []);
  const current = groups.find((item) => item.groupKey === groupKey);
  const schema = settingSchemas[groupKey];

  useEffect(() => {
    setDraft(current?.value ? { ...current.value } : {});
  }, [current?.revision, groupKey]);

  useEffect(() => {
    systemService.settingHistory(groupKey).then(setHistory).catch(() => setHistory([]));
  }, [groupKey, current?.revision]);

  async function save() {
    setBusy(true);
    setNotice("");
    try {
      await systemService.updateSettings(groupKey, { revision: current.revision, value: draft, changeSummary: `更新${schema.title}` });
      await reload();
      setNotice("配置已保存，并已对后续业务操作生效。");
    } catch (requestError) {
      setNotice(requestError.message || "保存配置失败");
    } finally {
      setBusy(false);
    }
  }

  async function rollback(targetRevision) {
    if (!window.confirm(`确认回滚到 V${targetRevision} 吗？当前配置会先保留到历史记录。`)) return;
    setBusy(true);
    try {
      await systemService.rollbackSettings(groupKey, { revision: current.revision, targetRevision });
      await reload();
      setNotice(`已回滚到 V${targetRevision}。`);
    } catch (requestError) {
      setNotice(requestError.message || "回滚失败");
    } finally {
      setBusy(false);
    }
  }

  const dirty = current && JSON.stringify(current.value) !== JSON.stringify(draft);
  return <div className="section-stack system-settings-page">
    <PageHeader title={notificationOnly ? "通知配置" : "系统参数"} description={notificationOnly ? "管理全平台默认通知订阅策略" : "集中维护会实际影响平台运行的关键参数"} />
    <StateBlock loading={loading} error={error}>
      <div className="system-settings-workbench">
        {!notificationOnly ? <aside className="settings-group-rail">
          {["execution", "security"].map((key) => <button className={groupKey === key ? "active" : ""} key={key} onClick={() => setGroupKey(key)} type="button"><strong>{settingSchemas[key].title}</strong><span>{settingSchemas[key].description}</span></button>)}
        </aside> : null}
        <main className="settings-editor">
          <header><div><span>配置组</span><h2>{schema.title}</h2><p>{schema.description}</p></div><div className="settings-version"><span>当前版本</span><strong>V{current?.revision || 1}</strong></div></header>
          {notice ? <div className="inline-notice">{notice}</div> : null}
          <div className="settings-field-list">
            {schema.fields.map(([key, label, description, minOrLocked, max]) => {
              const isBoolean = typeof draft[key] === "boolean" || typeof minOrLocked === "boolean";
              const locked = isBoolean && minOrLocked === true;
              return <label className="settings-field-row" key={key}><div><strong>{label}</strong><span>{description}</span></div>{isBoolean
                ? <input checked={Boolean(draft[key])} disabled={locked} onChange={(event) => setDraft({ ...draft, [key]: event.target.checked })} type="checkbox" />
                : <input className="text-input" max={max} min={minOrLocked} onChange={(event) => setDraft({ ...draft, [key]: Number(event.target.value) })} type="number" value={draft[key] ?? ""} />}</label>;
            })}
          </div>
          <footer><span>{dirty ? "有尚未保存的修改" : "配置已同步"}</span><button className="primary-button compact-button" disabled={!dirty || busy} onClick={save} type="button">{busy ? "保存中…" : "保存配置"}</button></footer>
          <section className="settings-history"><header><strong>版本记录</strong><span>保存和回滚均记录操作人及时间</span></header>{history.length ? history.map((item) => <article key={item.id}><div><strong>V{item.revision}</strong><span>{item.changeSummary || "配置快照"} · {item.createdBy || "系统"}</span></div><time>{formatTime(item.createdAt)}</time><button disabled={busy} onClick={() => rollback(item.revision)} type="button">回滚</button></article>) : <p>暂无历史版本</p>}</section>
        </main>
      </div>
    </StateBlock>
  </div>;
}

function AppearanceSettings() {
  const [appearance, setAppearance] = useState(() => readAppearance());

  function update(key, value) {
    setAppearance((current) => applyAppearance({ ...current, [key]: value }));
  }

  return <div className="section-stack appearance-page">
    <PageHeader title="外观设置" description="选择平台主题、字体和内容密度，修改后立即生效" />
    <section className="resource-panel appearance-section">
      <div className="appearance-section-heading"><LayoutTemplate size={18} /><div><strong>界面主题</strong><span>选择工作台的整体视觉风格</span></div></div>
      <div className="theme-choice-grid">
        {[
          ["blueprint", "蓝图网格", "专业蓝白、高信息密度的默认工作台"],
          ["midnight", "午夜作战", "深色低眩光，适合长时间执行与监控"],
          ["precision", "精密实验室", "冷静青绿，强调验证、质量与状态"],
          ["executive", "管理驾驶舱", "沉稳靛蓝，强化指标与决策层级"],
          ["industrial", "工业信号", "石墨与琥珀，突出告警和执行动作"]
        ].map(([value, label, description]) => (
          <button aria-label={`${label}：${description}`} className={appearance.theme === value ? "theme-choice active" : "theme-choice"} key={value} onClick={() => update("theme", value)} type="button">
            <span className={`theme-preview theme-preview-${value}`}><i /><b /><em /></span><strong>{label}</strong><small>{description}</small>{appearance.theme === value ? <Check size={16} /> : null}
          </button>
        ))}
      </div>
    </section>
    <div className="appearance-settings-grid">
      <section className="resource-panel appearance-section">
        <div className="appearance-section-heading"><Type size={18} /><div><strong>界面字体</strong><span>用于导航、表单和正文</span></div></div>
        <select className="text-input" onChange={(event) => update("uiFont", event.target.value)} value={appearance.uiFont}><option value="system">系统默认</option><option value="yahei">微软雅黑</option><option value="source">思源黑体</option></select>
        <div className="font-sample">Synapse QA · 自动化测试工作台 · Aa 123</div>
      </section>
      <section className="resource-panel appearance-section">
        <div className="appearance-section-heading"><Code2 size={18} /><div><strong>代码字体</strong><span>用于日志、SQL 和 Python 编辑器</span></div></div>
        <select className="text-input" onChange={(event) => update("codeFont", event.target.value)} value={appearance.codeFont}><option value="consolas">Consolas</option><option value="cascadiacode">Cascadia Code</option><option value="jetbrains">JetBrains Mono</option></select>
        <code className="code-font-sample">assert response.status == 200</code>
      </section>
    </div>
    <section className="resource-panel appearance-section">
      <div className="appearance-section-heading"><Type size={18} /><div><strong>字体大小</strong><span>分别调整界面、数据表格和代码日志的文字尺寸</span></div></div>
      <div className="font-size-setting-list">
        {[
          ["uiFontSize", "界面文字", "导航、按钮、表单与正文", "界面 Aa 123"],
          ["tableFontSize", "表格文字", "列表表头、数据单元格与结构化参数", "字段名称 · status · 200"],
          ["codeFontSize", "代码与日志", "JSON、SQL、Python 和执行日志", "const status = 200;"]
        ].map(([key, label, description, sample]) => <article className={`font-size-setting font-size-setting-${key}`} key={key}>
          <div><strong>{label}</strong><span>{description}</span><samp>{sample}</samp></div>
          <div className="font-size-options">{[["small", "小"], ["standard", "标准"], ["large", "大"]].map(([value, optionLabel]) => <button aria-label={`${label}：${optionLabel}`} className={appearance[key] === value ? "active" : ""} key={value} onClick={() => update(key, value)} type="button">{optionLabel}</button>)}</div>
        </article>)}
      </div>
    </section>
    <section className="resource-panel appearance-section">
      <div className="appearance-section-heading"><LayoutTemplate size={18} /><div><strong>显示密度</strong><span>控制表格、表单和工作区的间距</span></div></div>
      <div className="density-options">{[["compact", "紧凑"], ["standard", "标准"], ["comfortable", "宽松"]].map(([value, label]) => <button className={appearance.density === value ? "active" : ""} key={value} onClick={() => update("density", value)} type="button">{label}</button>)}</div>
    </section>
  </div>;
}

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
