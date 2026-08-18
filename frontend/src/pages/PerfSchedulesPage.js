import { useState } from "react";
import { Pencil, Plus, RefreshCw, Trash2, X } from "lucide-react";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { useAuth } from "../hooks/useAuth.js";
import { performanceService } from "../services/performanceService.js";
import { pageItems } from "../utils/formatters.js";
import { environmentLabel } from "../utils/perf.js";
import { pathToHash } from "../utils/routeState.js";

export function PerfSchedulesPage() {
  const { user } = useAuth();
  const permissions = new Set(user?.permissions || []);
  const isAdmin = user?.roleCode === "admin";
  const canManage = isAdmin || permissions.has("perf.plan.manage");
  const canExecute = isAdmin || permissions.has("perf.plan.execute");
  const [filters, setFilters] = useState({ planId: "", enabled: "" });
  const [page, setPage] = useState(1);
  const [notice, setNotice] = useState("");
  const [editing, setEditing] = useState(null);
  const { data, loading, error, reload } = useAsyncData(() => performanceService.schedules.list({ ...filters, page, pageSize: 20 }), [filters.planId, filters.enabled, page]);
  const { data: plansData } = useAsyncData(() => performanceService.plans.list({ page: 1, pageSize: 200 }), []);
  const plans = pageItems(plansData);
  const rows = Array.isArray(data) ? data : (data?.items || []);

  async function action(fn, message) {
    try { await fn(); setNotice(message); await reload(); } catch (err) { setNotice(err.status === 409 ? "规则存在执行冲突，当前操作未完成。" : (err.message || "操作失败")); }
  }
  function goRun(id) { if (id) window.location.hash = pathToHash(["性能测试", "测试报告", String(id)]); }

  return <div className="section-stack perf-schedules-page">
    <div className="page-header"><div><h1>定时规则</h1><p>按 Cron 触发性能测试，下一次执行时间由后端计算。</p></div><div className="action-row"><button className="icon-text-button compact-button" onClick={() => reload()} type="button"><RefreshCw size={14} />刷新</button>{canManage && canExecute ? <button className="primary-button compact-button" onClick={() => setEditing({})} type="button"><Plus size={14} />新建规则</button> : null}</div></div>
    <section className="resource-panel"><div className="perf-filter-row"><select className="text-input" value={filters.planId} onChange={(e) => { setPage(1); setFilters({ ...filters, planId: e.target.value }); }}><option value="">全部方案</option>{plans.map((plan) => <option key={plan.id} value={plan.id}>{plan.name}</option>)}</select><select className="text-input" value={filters.enabled} onChange={(e) => { setPage(1); setFilters({ ...filters, enabled: e.target.value }); }}><option value="">全部状态</option><option value="true">已启用</option><option value="false">已停用</option></select></div></section>
    {notice ? <div className="inline-notice">{notice}</div> : null}
    <StateBlock loading={loading} error={error}><section className="resource-panel"><div className="table-wrap"><table className="data-table"><thead><tr><th>名称</th><th>方案</th><th>环境</th><th>Cron</th><th>时区</th><th>下次执行</th><th>上次结果</th><th>状态</th><th>操作</th></tr></thead><tbody>{rows.length ? rows.map((row) => <tr key={row.id}><td>{row.name}</td><td>{row.planName || row.planId || "-"}</td><td>{row.environmentName || "跟随方案"}</td><td><code>{row.cronExpression}</code></td><td>{row.timezone}</td><td>{row.nextRunAt || "--"}</td><td>{row.lastRunId ? <button className="link-button" onClick={() => goRun(row.lastRunId)} type="button">{row.lastResult || `Run #${row.lastRunId}`}</button> : (row.lastResult || row.lastError || "--")}<small>{row.lastTriggeredAt || ""}</small></td><td><span className={`status-pill ${row.enabled ? "success" : "neutral"}`}>{row.enabled ? "已启用" : "已停用"}</span>{row.lastResult && ["blocked", "error", "skipped_active_run"].includes(row.lastResult) ? <small className="schedule-status-note">{scheduleStatus(row.lastResult)}</small> : null}</td><td><div className="action-row">{canManage && canExecute ? <><button className="link-button" onClick={() => setEditing(row)} type="button"><Pencil size={13} />编辑</button><button className="link-button" onClick={() => action(() => row.enabled ? performanceService.schedules.disable(row.id) : performanceService.schedules.enable(row.id), row.enabled ? "规则已停用" : "规则已启用")} type="button">{row.enabled ? "停用" : "启用"}</button><button className="link-button danger-text" onClick={() => { if (window.confirm(`确认删除规则“${row.name}”吗？`)) action(() => performanceService.schedules.remove(row.id), "规则已删除"); }} type="button"><Trash2 size={13} />删除</button></> : null}</div></td></tr>) : <tr><td colSpan="9" className="muted-text">暂无定时规则</td></tr>}</tbody></table></div><div className="pagination-row"><button disabled={page <= 1} onClick={() => setPage(page - 1)} type="button">上一页</button><span>第 {page} 页</span><button disabled={rows.length < 20} onClick={() => setPage(page + 1)} type="button">下一页</button></div></section></StateBlock>
    {editing ? <ScheduleEditor value={editing} plans={plans} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); await reload(); }} /> : null}
  </div>;
}

export function serializeScheduleCreate(form) { return { name: form.name, planId: form.planId ? Number(form.planId) : null, environmentId: form.environmentId ? Number(form.environmentId) : null, cronExpression: form.cronExpression, timezone: form.timezone, enabled: Boolean(form.enabled) }; }
export function serializeSchedulePatch(form) { return { name: form.name, planId: form.planId ? Number(form.planId) : null, environmentId: form.environmentId ? Number(form.environmentId) : null, cronExpression: form.cronExpression, timezone: form.timezone }; }

function ScheduleEditor({ value, plans, onClose, onSaved }) {
  const [form, setForm] = useState({ name: value.name || "", planId: value.planId || "", environmentId: value.environmentId || "", cronExpression: value.cronExpression || "0 0 * * *", timezone: value.timezone || "Asia/Shanghai", enabled: value.enabled !== false });
  const [error, setError] = useState("");
  const save = async () => {
    if (!form.name.trim()) return setError("规则名称不能为空。");
    if (form.cronExpression.trim().split(/\s+/).length !== 5) return setError("Cron 必须是 5 个字段。");
    try { const payload = value.id ? serializeSchedulePatch(form) : serializeScheduleCreate(form); if (value.id) await performanceService.schedules.update(value.id, payload); else await performanceService.schedules.create(payload); await onSaved(); } catch (err) { setError(err.message || "保存失败"); }
  };
  return <div className="modal-backdrop"><section className="modal-card modal-card-small"><div className="modal-header"><strong>{value.id ? "编辑定时规则" : "新建定时规则"}</strong><button className="modal-close" onClick={onClose} type="button"><X size={18} /></button></div><div className="modal-grid"><label className="form-field"><span>名称</span><input className="text-input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label><label className="form-field"><span>方案</span><select className="text-input" value={form.planId} onChange={(e) => setForm({ ...form, planId: e.target.value })}><option value="">请选择方案</option>{plans.map((plan) => <option key={plan.id} value={plan.id}>{plan.name}</option>)}</select></label><label className="form-field"><span>环境</span><input className="text-input" value={form.environmentId} onChange={(e) => setForm({ ...form, environmentId: e.target.value })} placeholder="留空跟随方案" /></label><label className="form-field"><span>Cron（5字段）</span><input className="text-input" value={form.cronExpression} onChange={(e) => setForm({ ...form, cronExpression: e.target.value })} /><small>分 时 日 月 周；下次执行时间由后端返回。</small></label><label className="form-field"><span>IANA 时区</span><input className="text-input" value={form.timezone} onChange={(e) => setForm({ ...form, timezone: e.target.value })} placeholder="Asia/Shanghai" /></label></div>{error ? <div className="form-error">{error}</div> : null}<div className="modal-actions"><button className="icon-text-button compact-button" onClick={onClose} type="button">取消</button><button className="primary-button compact-button" onClick={save} type="button">保存</button></div></section></div>;
}

function scheduleStatus(value) { return { blocked: "已阻止", error: "执行错误", skipped_active_run: "已有活动任务，已跳过" }[value] || value; }
