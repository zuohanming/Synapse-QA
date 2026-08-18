import { useState } from "react";
import { CalendarClock, Pencil, Plus, RefreshCw, Trash2, X } from "lucide-react";
import { PaginationBar, TablePanel } from "../components/DataTable.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { useAuth } from "../hooks/useAuth.js";
import { performanceService } from "../services/performanceService.js";
import { pageItems } from "../utils/formatters.js";
import { pathToHash } from "../utils/routeState.js";

const PAGE_SIZE_DEFAULT = 20;
const RESULT_LABELS = { triggered: "已触发", skipped_active_run: "已有活动任务，已跳过", blocked: "已阻止", error: "执行错误" };

export function PerfSchedulesPage() {
  const { user } = useAuth();
  const permissions = new Set(user?.permissions || []);
  const isAdmin = user?.roleCode === "admin";
  const canOperate = (isAdmin || permissions.has("perf.plan.manage")) && (isAdmin || permissions.has("perf.plan.execute"));
  const [draftFilters, setDraftFilters] = useState({ planId: "", enabled: "" });
  const [filters, setFilters] = useState({ planId: "", enabled: "" });
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(PAGE_SIZE_DEFAULT);
  const [notice, setNotice] = useState("");
  const [editing, setEditing] = useState(null);
  const { data, loading, error, reload } = useAsyncData(() => performanceService.schedules.list({ ...filters, page, pageSize }), [filters.planId, filters.enabled, page, pageSize]);
  const { data: plansData } = useAsyncData(() => performanceService.plans.list({ page: 1, pageSize: 200 }), []);
  const plans = pageItems(plansData);
  const rows = Array.isArray(data) ? data : (data?.items || []);
  const total = Array.isArray(data) ? rows.length : Number(data?.total ?? rows.length);
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const hasFilter = Boolean(filters.planId || filters.enabled);

  function submitSearch(event) { event.preventDefault(); setPage(1); setFilters({ ...draftFilters }); }
  function resetSearch() { setDraftFilters({ planId: "", enabled: "" }); setFilters({ planId: "", enabled: "" }); setPage(1); }
  async function action(fn, message) { try { await fn(); setNotice(message); await reload(); } catch (err) { setNotice(err.status === 409 ? "规则存在执行冲突，当前操作未完成。" : (err.message || "操作失败")); } }
  function goRun(id) { if (id) window.location.hash = pathToHash(["性能测试", "测试报告", String(id)]); }

  return <div className="section-stack perf-schedules-page">
    <section className="resource-panel">
      <div className="panel-header"><strong>定时规则管理</strong></div>
      <form className="filter-grid" onSubmit={submitSearch}>
        <label className="form-field"><span>方案</span><select className="text-input" value={draftFilters.planId} onChange={(event) => setDraftFilters({ ...draftFilters, planId: event.target.value })}><option value="">全部方案</option>{plans.map((plan) => <option key={plan.id} value={plan.id}>{plan.name}</option>)}</select></label>
        <label className="form-field"><span>状态</span><select className="text-input" value={draftFilters.enabled} onChange={(event) => setDraftFilters({ ...draftFilters, enabled: event.target.value })}><option value="">全部</option><option value="true">已启用</option><option value="false">已停用</option></select></label>
        <div className="filter-actions"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={resetSearch} type="button">重置</button></div>
      </form>
      <div className="list-actions"><div /> <div className="action-row"><button aria-label="刷新定时规则" className="icon-text-button compact-button" onClick={() => reload()} type="button"><RefreshCw size={14} />刷新</button>{canOperate ? <button aria-label="新建定时规则" className="primary-button compact-button" onClick={() => setEditing({})} type="button"><Plus size={14} />新增规则</button> : null}</div></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <StateBlock loading={loading} error={error}><TablePanel>{rows.length ? <div className="table-wrap"><table className="data-table perf-schedule-table"><thead><tr><th>规则</th><th>方案</th><th>环境</th><th>Cron</th><th>时区</th><th>下次执行</th><th>上次结果</th><th>状态</th><th>操作</th></tr></thead><tbody>{rows.map((row) => <ScheduleRow key={row.id} row={row} canOperate={canOperate} onEdit={() => setEditing(row)} onToggle={() => action(() => row.enabled ? performanceService.schedules.disable(row.id) : performanceService.schedules.enable(row.id), row.enabled ? "规则已停用" : "规则已启用")} onDelete={() => { if (window.confirm(`确认删除规则“${row.name}”吗？`)) action(() => performanceService.schedules.remove(row.id), "规则已删除"); }} onRun={goRun} />)}</tbody></table></div> : <ScheduleEmpty filtered={hasFilter} canOperate={canOperate} onReset={resetSearch} onCreate={() => setEditing({})} />} <PaginationBar page={page} pageSize={pageSize} total={total} totalPages={totalPages} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} /></TablePanel></StateBlock>
    </section>
    {editing ? <ScheduleEditor value={editing} plans={plans} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); await reload(); }} /> : null}
  </div>;
}

function ScheduleRow({ row, canOperate, onEdit, onToggle, onDelete, onRun }) {
  const result = row.lastResult;
  return <tr><td><div className="perf-schedule-name" title={row.name}><strong>{row.name}</strong><small>#{row.id}</small></div></td><td title={row.planName || row.planId}>{row.planName || row.planId || "-"}</td><td title={row.environmentName || "跟随方案"}>{row.environmentName || "跟随方案"}</td><td><code className="perf-schedule-cron" title={row.cronExpression}>{row.cronExpression}</code></td><td>{row.timezone || "--"}</td><td>{row.nextRunAt || "--"}</td><td>{row.lastRunId ? <button aria-label={`查看运行 ${row.lastRunId}`} className="link-button" onClick={() => onRun(row.lastRunId)} type="button">{RESULT_LABELS[result] || result || `Run #${row.lastRunId}`}</button> : <span className={result ? "perf-schedule-result-text" : "muted-text"}>{RESULT_LABELS[result] || result || row.lastError || "暂无记录"}</span>}{row.lastTriggeredAt ? <small className="perf-schedule-last-time">{row.lastTriggeredAt}</small> : null}</td><td><span className={`status-pill ${row.enabled ? "success" : "neutral"}`}>{row.enabled ? "已启用" : "已停用"}</span>{result && RESULT_LABELS[result] ? <small className={`perf-schedule-result-badge result-${result}`}>{RESULT_LABELS[result]}</small> : null}</td><td>{canOperate ? <div className="perf-schedule-actions"><button aria-label={`编辑 ${row.name}`} className="link-button" onClick={onEdit} type="button"><Pencil size={13} />编辑</button><button aria-label={`${row.enabled ? "停用" : "启用"} ${row.name}`} className="link-button" onClick={onToggle} type="button">{row.enabled ? "停用" : "启用"}</button><button aria-label={`删除 ${row.name}`} className="link-button danger-text" onClick={onDelete} type="button"><Trash2 size={13} />删除</button></div> : <span className="muted-text">—</span>}</td></tr>;
}

function ScheduleEmpty({ filtered, canOperate, onReset, onCreate }) { return <div className="perf-schedule-empty"><CalendarClock size={32} /><strong>{filtered ? "没有匹配的定时规则" : "暂无定时规则"}</strong><p>{filtered ? "请调整筛选条件后重试。" : "创建一条规则，让性能测试按计划自动触发。"}</p>{filtered ? <button className="icon-text-button compact-button" onClick={onReset} type="button">重置筛选</button> : canOperate ? <button className="primary-button compact-button" onClick={onCreate} type="button"><Plus size={14} />新增规则</button> : null}</div>; }

export function serializeScheduleCreate(form) { return { name: form.name, planId: form.planId ? Number(form.planId) : null, environmentId: form.environmentId ? Number(form.environmentId) : null, cronExpression: form.cronExpression, timezone: form.timezone, enabled: Boolean(form.enabled) }; }
export function serializeSchedulePatch(form) { return { name: form.name, planId: form.planId ? Number(form.planId) : null, environmentId: form.environmentId ? Number(form.environmentId) : null, cronExpression: form.cronExpression, timezone: form.timezone }; }

function ScheduleEditor({ value, plans, onClose, onSaved }) {
  const [form, setForm] = useState({ name: value.name || "", planId: value.planId || "", environmentId: value.environmentId || "", cronExpression: value.cronExpression || "0 0 * * *", timezone: value.timezone || "Asia/Shanghai", enabled: value.enabled !== false });
  const [formError, setFormError] = useState("");
  async function save() { if (!form.name.trim()) return setFormError("规则名称不能为空。"); if (!form.planId) return setFormError("请选择方案。"); if (form.cronExpression.trim().split(/\s+/).length !== 5) return setFormError("Cron 必须是 5 个字段，例如：0 1 * * *。"); try { const payload = value.id ? serializeSchedulePatch(form) : serializeScheduleCreate(form); if (value.id) await performanceService.schedules.update(value.id, payload); else await performanceService.schedules.create(payload); await onSaved(); } catch (err) { setFormError(err.message || "保存失败"); } }
  return <div className="modal-backdrop"><section aria-label={value.id ? "编辑定时规则" : "新建定时规则"} className="modal-card modal-card-small perf-schedule-editor"><div className="modal-header"><strong>{value.id ? "编辑定时规则" : "新建定时规则"}</strong><button aria-label="关闭" className="modal-close" onClick={onClose} type="button"><X size={18} /></button></div><div className="modal-grid"><label className="form-field"><span>规则名称</span><input autoFocus className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="例如：工作日夜间压测" /></label><label className="form-field"><span>方案</span><select className="text-input" value={form.planId} onChange={(event) => setForm({ ...form, planId: event.target.value })}><option value="">请选择方案</option>{plans.map((plan) => <option key={plan.id} value={plan.id}>{plan.name}</option>)}</select></label><label className="form-field"><span>环境</span><input className="text-input" value={form.environmentId} onChange={(event) => setForm({ ...form, environmentId: event.target.value })} placeholder="留空跟随方案环境" /></label><label className="form-field"><span>Cron（5 字段）</span><input className="text-input perf-schedule-cron-input" value={form.cronExpression} onChange={(event) => setForm({ ...form, cronExpression: event.target.value })} placeholder="0 1 * * *" /><small>格式：分 时 日 月 周；下次执行时间由后端返回。</small></label><label className="form-field"><span>IANA 时区</span><input className="text-input" value={form.timezone} onChange={(event) => setForm({ ...form, timezone: event.target.value })} placeholder="Asia/Shanghai" /></label><label className="perf-schedule-enabled"><input checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} type="checkbox" />保存后启用规则</label></div>{formError ? <div className="form-error" role="alert">{formError}</div> : null}<div className="modal-actions"><button className="icon-text-button compact-button" onClick={onClose} type="button">取消</button><button className="primary-button compact-button" onClick={save} type="button">{value.id ? "保存修改" : "创建规则"}</button></div></section></div>;
}
