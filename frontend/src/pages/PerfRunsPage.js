import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { useAuth } from "../hooks/useAuth.js";
import { performanceService } from "../services/performanceService.js";
import { formatTime, pageItems } from "../utils/formatters.js";
import {
  ENVIRONMENTS,
  RUNNING_STATUSES,
  SCENARIO_TYPES,
  environmentLabel,
  environmentTone,
  renderMs,
  renderPercent,
  runStatusLabel,
  runStatusRunning,
  runStatusTone,
  scenarioLabel
} from "../utils/perf.js";
import { pathToHash } from "../utils/routeState.js";

const initialFilters = { id: "", planId: "", scenarioType: "", environment: "", status: "" };

export function PerfRunsPage() {
  const { user } = useAuth();
  const granted = new Set(user?.permissions || []);
  const isAdmin = user?.roleCode === "admin";
  const canExecute = isAdmin || granted.has("perf.plan.execute");

  const [form, setForm] = useState(initialFilters);
  const [filters, setFilters] = useState(initialFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [activeOnly, setActiveOnly] = useState(false);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const { data, loading, error, reload } = useAsyncData(
    () => performanceService.runs.list(activeOnly ? { page: 1, pageSize: 200 } : { ...filters, page, pageSize }),
    [activeOnly, filters.id, filters.planId, filters.scenarioType, filters.environment, filters.status, page, pageSize]
  );
  const allRows = pageItems(data);
  const filteredRows = activeOnly ? allRows.filter((row) => RUNNING_STATUSES.includes(row.status)) : allRows;
  const total = activeOnly ? filteredRows.length : data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const rows = activeOnly ? filteredRows.slice((page - 1) * pageSize, page * pageSize) : filteredRows;

  const { data: plansData, loading: loadingPlans } = useAsyncData(() => performanceService.plans.list({ page: 1, pageSize: 200 }), []);
  const planOptions = pageItems(plansData);

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

  function toggleActiveOnly() {
    setActiveOnly((current) => !current);
    setPage(1);
  }

  async function cancelRun(row) {
    if (!window.confirm(`确认取消执行记录 #${row.id} 吗？取消后仍会保留部分结果。`)) return;
    setBusy(true);
    setNotice("");
    try {
      await performanceService.runs.cancel(row.id);
      await reload();
      setNotice(`执行记录 #${row.id} 已请求取消。`);
    } catch (err) {
      setNotice(err.message || "取消失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="section-stack">
      <PageHeader title="测试报告" description="查看性能压测执行记录与关键指标" />
      <section className="resource-panel">
        <div className="panel-header">
          <strong>性能测试报告</strong>
        </div>

        <form className="filter-grid" onSubmit={submitSearch}>
          <label className="form-field">
            <span>ID</span>
            <input className="text-input" value={form.id} onChange={(event) => setForm({ ...form, id: event.target.value })} placeholder="请输入执行记录 ID" />
          </label>
          <label className="form-field">
            <span>方案</span>
            <select className="text-input" disabled={loadingPlans} value={form.planId} onChange={(event) => setForm({ ...form, planId: event.target.value })}>
              <option value="">{loadingPlans ? "加载方案中" : "全部方案"}</option>
              {planOptions.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
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
            <span>环境</span>
            <select className="text-input" value={form.environment} onChange={(event) => setForm({ ...form, environment: event.target.value })}>
              <option value="">全部</option>
              {Object.entries(ENVIRONMENTS).map(([key, item]) => <option key={key} value={key}>{item.label}</option>)}
            </select>
          </label>
          <label className="form-field">
            <span>状态</span>
            <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              <option value="">全部</option>
              <option value="pending">待执行</option>
              <option value="queued">排队中</option>
              <option value="dispatching">调度中</option>
              <option value="dispatched">已下发</option>
              <option value="running">执行中</option>
              <option value="stopping">停止中</option>
              <option value="completed">通过</option>
              <option value="threshold_failed">性能未达标</option>
              <option value="execution_failed">执行异常</option>
              <option value="timed_out">超时</option>
              <option value="canceled">已取消</option>
            </select>
          </label>
          <div className="filter-actions">
            <button className="primary-button compact-button" type="submit">搜索</button>
            <button className="icon-text-button compact-button" onClick={resetSearch} type="button">重置</button>
          </div>
        </form>

        <div className="list-actions">
          <div className="action-row">
            <button className={`icon-text-button compact-button${activeOnly ? " perf-active-toggle" : ""}`} onClick={toggleActiveOnly} type="button">
              {activeOnly ? "显示全部" : "仅看进行中"}
            </button>
            <button className="icon-text-button compact-button" onClick={() => reload()} type="button"><RefreshCw size={14} />刷新</button>
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
                    <th>场景类型</th>
                    <th>状态</th>
                    <th>触发人</th>
                    <th>环境</th>
                    <th>RPS</th>
                    <th>P95 耗时</th>
                    <th>错误率</th>
                    <th>开始时间</th>
                    <th>耗时</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.length ? (
                    rows.map((row) => (
                      <tr key={row.id}>
                        <td>{row.id}</td>
                        <td>{row.planName || "-"}</td>
                        <td>{scenarioLabel(row.scenarioType)}</td>
                        <td><span className={`status-pill ${runStatusTone(row.status)}`}>{runStatusLabel(row.status)}</span></td>
                        <td>{row.triggeredBy || "-"}</td>
                        <td><span className={`status-pill ${environmentTone(row.environment)}`}>{environmentLabel(row.environment)}</span></td>
                        <td>{row.rps != null ? Number(row.rps).toFixed(1) : "--"}</td>
                        <td>{renderMs(row.p95DurationMs)}</td>
                        <td>{renderPercent(row.errorRate)}</td>
                        <td>{formatTime(row.startedAt || row.createdAt)}</td>
                        <td>{formatRunDuration(row)}</td>
                        <td>
                          <div className="action-links">
                            <button className="link-button" onClick={() => go(["性能测试", "测试报告", String(row.id)])} type="button">详情</button>
                            {canExecute && runStatusRunning(row.status) ? <button className="link-button danger-link" disabled={busy} onClick={() => cancelRun(row)} type="button">取消</button> : null}
                          </div>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr><td colSpan="12">暂无执行记录</td></tr>
                  )}
                </tbody>
              </table>
            </div>
            <PaginationBar page={page} pageSize={pageSize} total={total} totalPages={totalPages} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} />
          </TablePanel>
        </StateBlock>
      </section>
    </div>
  );
}

function formatRunDuration(row) {
  if (!row.startedAt) return "--";
  const start = new Date(row.startedAt).getTime();
  const end = row.finishedAt ? new Date(row.finishedAt).getTime() : Date.now();
  const duration = Math.max(0, end - start);
  if (duration < 1000) return `${duration} ms`;
  if (duration < 60000) return `${(duration / 1000).toFixed(duration < 10000 ? 1 : 0)} 秒`;
  const minutes = Math.floor(duration / 60000);
  return `${minutes} 分 ${Math.round((duration % 60000) / 1000)} 秒`;
}
