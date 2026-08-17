import { useState } from "react";
import { Plus, RefreshCw } from "lucide-react";
import { PerfRunConfirmPanel } from "../components/PerfRunConfirmPanel.js";
import { PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { useAuth } from "../hooks/useAuth.js";
import { configService } from "../services/configService.js";
import { performanceService } from "../services/performanceService.js";
import { formatTime, pageItems } from "../utils/formatters.js";
import { ENVIRONMENTS, SCENARIO_TYPES, environmentLabel, environmentTone, planStatusLabel, planStatusTone, priorityTone, scenarioLabel } from "../utils/perf.js";
import { pathToHash } from "../utils/routeState.js";

const initialFilters = { id: "", name: "", productId: "", scenarioType: "", environment: "", priority: "", status: "" };

export function PerfPlansPage() {
  const { user } = useAuth();
  const granted = new Set(user?.permissions || []);
  const isAdmin = user?.roleCode === "admin";
  const canManage = isAdmin || granted.has("perf.plan.manage");
  const canExecute = isAdmin || granted.has("perf.plan.execute");

  const [form, setForm] = useState(initialFilters);
  const [filters, setFilters] = useState(initialFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [confirmPlan, setConfirmPlan] = useState(null);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const { data, loading, error, reload } = useAsyncData(
    () => performanceService.plans.list({ ...filters, page, pageSize }),
    [filters.id, filters.name, filters.productId, filters.scenarioType, filters.environment, filters.priority, filters.status, page, pageSize]
  );
  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const { data: productsData, loading: loadingProducts } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const productOptions = pageItems(productsData);

  function go(path) {
    window.location.hash = pathToHash(path);
  }

  function submitSearch(event) {
    event.preventDefault();
    setFilters({ ...form });
    setPage(1);
  }

  function resetSearch() {
    setForm(initialFilters);
    setFilters(initialFilters);
    setPage(1);
  }

  async function removeRow(row) {
    if (!window.confirm(`确认删除压测方案“${row.name}”吗？`)) return;
    setBusy(true);
    setNotice("");
    try {
      await performanceService.plans.remove(row.id);
      await reload();
      setNotice("压测方案已删除。");
    } catch (err) {
      setNotice(err.message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  function onRun(runId) {
    setConfirmPlan(null);
    if (runId) go(["性能测试", "测试报告", String(runId)]);
  }

  return (
    <div className="section-stack">
      <PageHeader title="压测方案" description="按 6 大场景配置并触发性能压测" />
      <section className="resource-panel">
        <div className="panel-header">
          <strong>压测方案管理</strong>
        </div>

        <form className="filter-grid" onSubmit={submitSearch}>
          <label className="form-field">
            <span>ID</span>
            <input className="text-input" value={form.id} onChange={(event) => setForm({ ...form, id: event.target.value })} placeholder="请输入方案 ID" />
          </label>
          <label className="form-field">
            <span>方案名称</span>
            <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="请输入方案名称" />
          </label>
          <label className="form-field">
            <span>项目/产品</span>
            <select className="text-input" disabled={loadingProducts} value={form.productId} onChange={(event) => setForm({ ...form, productId: event.target.value })}>
              <option value="">{loadingProducts ? "加载产品中" : "请选择产品"}</option>
              {productOptions.map((item) => (
                <option key={item.id} value={item.id}>{item.projectName}/{item.name}</option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>场景类型</span>
            <select className="text-input" value={form.scenarioType} onChange={(event) => setForm({ ...form, scenarioType: event.target.value })}>
              <option value="">全部</option>
              {SCENARIO_TYPES.map((item) => <option key={item.key} value={item.key}>{item.label}</option>)}
            </select>
          </label>
          <label className="form-field">
            <span>测试环境</span>
            <select className="text-input" value={form.environment} onChange={(event) => setForm({ ...form, environment: event.target.value })}>
              <option value="">全部</option>
              {Object.entries(ENVIRONMENTS).map(([key, item]) => <option key={key} value={key}>{item.label}</option>)}
            </select>
          </label>
          <label className="form-field">
            <span>优先级</span>
            <select className="text-input" value={form.priority} onChange={(event) => setForm({ ...form, priority: event.target.value })}>
              <option value="">全部</option>
              <option value="P0">P0</option>
              <option value="P1">P1</option>
              <option value="P2">P2</option>
              <option value="P3">P3</option>
            </select>
          </label>
          <label className="form-field">
            <span>状态</span>
            <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              <option value="">全部</option>
              <option value="draft">草稿</option>
              <option value="active">启用</option>
              <option value="disabled">停用</option>
            </select>
          </label>
          <div className="filter-actions">
            <button className="primary-button compact-button" type="submit">搜索</button>
            <button className="icon-text-button compact-button" onClick={resetSearch} type="button">重置</button>
          </div>
        </form>

        <div className="list-actions">
          <div />
          <div className="action-row">
            <button className="icon-text-button compact-button" onClick={() => reload()} type="button"><RefreshCw size={14} />刷新</button>
            {canManage ? <button className="primary-button compact-button" onClick={() => go(["性能测试", "压测方案", "new"])} type="button"><Plus size={14} />新增方案</button> : null}
          </div>
        </div>

        {notice ? <div className="inline-notice">{notice}</div> : null}

        <StateBlock loading={loading} error={error}>
          <TablePanel>
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>方案名称</th>
                    <th>项目/产品</th>
                    <th>场景类型</th>
                    <th>环境</th>
                    <th>优先级</th>
                    <th>状态</th>
                    <th>负责人</th>
                    <th>更新时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.length ? (
                    rows.map((row) => (
                      <tr key={row.id}>
                        <td>{row.id}</td>
                        <td>{row.name}</td>
                        <td>{row.productName || "-"}</td>
                        <td>{scenarioLabel(row.scenarioType)}</td>
                        <td><span className={`status-pill ${environmentTone(row.environment)}`}>{environmentLabel(row.environment)}</span></td>
                        <td><span className={`status-pill ${priorityTone(row.priority)}`}>{row.priority}</span></td>
                        <td><span className={`status-pill ${planStatusTone(row.status)}`}>{planStatusLabel(row.status)}</span></td>
                        <td>{row.owner || "-"}</td>
                        <td>{formatTime(row.updatedAt)}</td>
                        <td>
                          <div className="action-links">
                            <button className="link-button" onClick={() => go(["性能测试", "压测方案", String(row.id)])} type="button">详情</button>
                            {canManage ? <button className="link-button" onClick={() => go(["性能测试", "压测方案", String(row.id), "edit"])} type="button">编辑</button> : null}
                            {canExecute ? <button className="link-button" disabled={busy} onClick={() => setConfirmPlan(row)} type="button">触发</button> : null}
                            {canManage ? <button className="link-button danger-link" disabled={busy} onClick={() => removeRow(row)} type="button">删除</button> : null}
                          </div>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr><td colSpan="10">暂无数据</td></tr>
                  )}
                </tbody>
              </table>
            </div>
            <PaginationBar page={page} pageSize={pageSize} total={total} totalPages={totalPages} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} />
          </TablePanel>
        </StateBlock>
      </section>

      {confirmPlan ? <PerfRunConfirmPanel plan={confirmPlan} onClose={() => setConfirmPlan(null)} onRun={onRun} /> : null}
    </div>
  );
}
