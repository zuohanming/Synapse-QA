import { useEffect, useState } from "react";
import { Download, RefreshCw } from "lucide-react";
import { PaginationBar, TablePanel } from "../components/DataTable.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { executionService } from "../services/executionService.js";
import { formatTime, pageItems } from "../utils/formatters.js";

const initialFilters = { id: "", runType: "", status: "" };

export function ExecutionPage({ activePath }) {
  const reportMode = activePath[1] === "测试报告";
  const [form, setForm] = useState(initialFilters);
  const [filters, setFilters] = useState(initialFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [detail, setDetail] = useState(null);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const { data, loading, error, reload } = useAsyncData(
    () => executionService.list({ ...filters, page, pageSize }),
    [filters.id, filters.runType, filters.status, page, pageSize]
  );
  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const runningStatuses = new Set(["pending", "queued", "running"]);
  const hasRunningRows = rows.some((row) => runningStatuses.has(row.status));
  const detailIsRunning = detail && runningStatuses.has(detail.status);

  useEffect(() => {
    if (!hasRunningRows && !detailIsRunning) return undefined;
    const timer = window.setTimeout(async () => {
      await reload({ silent: true });
      if (detailIsRunning) {
        try {
          setDetail(await executionService.get(detail.id));
        } catch {
          // 列表轮询仍继续，详情读取失败由下一轮重试。
        }
      }
    }, 2000);
    return () => window.clearTimeout(timer);
  }, [hasRunningRows, detail?.id, detail?.status, detailIsRunning, reload]);

  async function openDetail(row) {
    setBusy(true);
    setNotice("");
    try {
      setDetail(await executionService.get(row.id));
    } catch (err) {
      setNotice(err.message || "读取执行结果失败");
    } finally {
      setBusy(false);
    }
  }

  async function cancelRun(row) {
    if (!window.confirm(`确认取消执行批次 #${row.id} 吗？`)) return;
    setBusy(true);
    try {
      await executionService.cancel(row.id);
      await reload();
      setNotice(`执行批次 #${row.id} 已取消。`);
    } catch (err) {
      setNotice(err.message || "取消失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="section-stack">
      <section className="resource-panel">
        <div className="panel-header"><strong>{reportMode ? "测试报告" : "执行记录"}</strong></div>
        <form className="filter-grid" onSubmit={(event) => { event.preventDefault(); setFilters({ ...form }); setPage(1); }}>
          <label className="form-field"><span>批次 ID</span><input className="text-input" value={form.id} placeholder="请输入批次 ID" onChange={(event) => setForm({ ...form, id: event.target.value })} /></label>
          <label className="form-field"><span>执行类型</span><select className="text-input" value={form.runType} onChange={(event) => setForm({ ...form, runType: event.target.value })}><option value="">全部</option><option value="ui">UI</option><option value="api">API</option><option value="unit">单元测试</option><option value="noop">流程验证</option></select></label>
          <label className="form-field"><span>状态</span><select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}><option value="">全部</option><option value="running">执行中</option><option value="completed">已通过</option><option value="failed">失败</option><option value="canceled">已取消</option></select></label>
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
          <div className="toolbar-row"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={() => { setForm(initialFilters); setFilters(initialFilters); setPage(1); }} type="button">重置</button></div>
        </form>
        <div className="list-actions"><div /><button className="icon-text-button compact-button" onClick={reload} type="button"><RefreshCw size={14} />刷新</button></div>
        {notice ? <div className="inline-notice">{notice}</div> : null}
        <StateBlock loading={loading} error={error}>
          <TablePanel>
            <div className="table-wrap"><table className="data-table"><thead><tr><th>批次 ID</th><th>类型</th><th>状态</th><th>执行进度</th><th>用例数</th><th>通过</th><th>失败</th><th>触发人</th><th>开始时间</th><th>操作</th></tr></thead><tbody>
              {rows.length ? rows.map((row) => { const summary = row.summary || {}; return <tr key={row.id}><td>{row.id}</td><td>{runTypeLabel(row.runType)}</td><td><span className={`status-badge ${row.status === "completed" ? "status-passed" : row.status === "failed" ? "status-failed" : ""}`}>{runStatusLabel(row.status)}</span></td><td><ExecutionProgress summary={summary} totalFallback={row.caseIds?.length} /></td><td>{summary.total ?? row.caseIds?.length ?? 0}</td><td>{summary.passed ?? "-"}</td><td>{summary.failed ?? "-"}</td><td>{row.triggeredBy || "-"}</td><td>{formatTime(row.startedAt || row.createdAt)}</td><td><div className="action-links"><button className="link-button" disabled={busy} onClick={() => openDetail(row)} type="button">{reportMode ? "查看报告" : "详情"}</button>{row.status === "running" ? <button className="link-button danger-link" disabled={busy} onClick={() => cancelRun(row)} type="button">取消</button> : null}</div></td></tr>; }) : <tr><td colSpan="10">暂无执行记录</td></tr>}
            </tbody></table></div>
            <PaginationBar page={page} pageSize={pageSize} total={total} totalPages={totalPages} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} />
          </TablePanel>
        </StateBlock>
      </section>
      {detail ? <ExecutionReport detail={detail} onClose={() => setDetail(null)} /> : null}
    </div>
  );
}

function ExecutionReport({ detail, onClose }) {
  const summary = detail.summary || {};
  return <div className="modal-backdrop"><section className="modal-card modal-card-wide execution-report"><div className="modal-header"><strong>测试报告 #{detail.id}</strong><button className="modal-close" onClick={onClose} type="button">×</button></div><div className="report-summary"><ReportMetric label="用例总数" value={summary.total ?? detail.tasks?.length ?? 0} /><ReportMetric label="通过" value={summary.passed ?? 0} tone="success" /><ReportMetric label="失败" value={summary.failed ?? 0} tone="danger" /><ReportMetric label="跳过" value={summary.skipped ?? 0} /></div><div className="detail-grid"><div><span>执行类型</span><strong>{runTypeLabel(detail.runType)}</strong></div><div><span>执行状态</span><strong>{runStatusLabel(detail.status)}</strong></div><div><span>触发人</span><strong>{detail.triggeredBy || "-"}</strong></div><div><span>开始时间</span><strong>{formatTime(detail.startedAt)}</strong></div><div><span>结束时间</span><strong>{formatTime(detail.finishedAt)}</strong></div><div><span>批次 ID</span><strong>{detail.id}</strong></div></div><div className="report-task-list"><strong>任务明细</strong><div className="table-wrap"><table className="data-table"><thead><tr><th>用例 ID</th><th>执行器</th><th>状态</th><th>输出</th><th>错误</th></tr></thead><tbody>{detail.tasks?.length ? detail.tasks.map((task) => <tr key={task.id}><td>{task.caseId}</td><td>{task.executorId}</td><td>{runStatusLabel(task.status)}</td><td className="cell-ellipsis" title={task.result?.output || ""}>{task.result?.output || "-"}</td><td className="cell-ellipsis" title={task.result?.error || ""}>{task.result?.error || "-"}</td></tr>) : <tr><td colSpan="5">暂无任务明细</td></tr>}</tbody></table></div></div><div className="modal-actions"><button className="icon-text-button compact-button" onClick={() => downloadReport(detail)} type="button"><Download size={14} />导出报告</button><button className="primary-button compact-button" onClick={onClose} type="button">关闭</button></div></section></div>;
}

function ReportMetric({ label, value, tone = "" }) { return <div className={`report-metric ${tone}`}><span>{label}</span><strong>{value}</strong></div>; }
function ExecutionProgress({ summary = {}, totalFallback = 0 }) {
  const total = Number(summary.total ?? totalFallback ?? 0);
  const completed = Number(summary.passed || 0) + Number(summary.failed || 0) + Number(summary.skipped || 0);
  const percent = total > 0 ? Math.min(100, Math.round((completed / total) * 100)) : 0;
  return <div className="execution-progress" title={`已完成 ${completed}/${total}`}><div className="execution-progress-track"><span style={{ width: `${percent}%` }} /></div><small>{percent}%</small></div>;
}
function runTypeLabel(value) { return { ui: "UI", api: "API", unit: "单元测试", script: "脚本", noop: "流程验证" }[value] || value || "-"; }
function runStatusLabel(value) { return { pending: "等待中", queued: "排队中", running: "执行中", success: "通过", completed: "已通过", failed: "失败", canceled: "已取消" }[value] || value || "-"; }
function escapeHTML(value) { return String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]); }
function downloadReport(detail) { const summary = detail.summary || {}; const rows = (detail.tasks || []).map((task) => `<tr><td>${escapeHTML(task.caseId)}</td><td>${escapeHTML(task.executorId)}</td><td>${escapeHTML(runStatusLabel(task.status))}</td><td>${escapeHTML(task.result?.output)}</td><td>${escapeHTML(task.result?.error)}</td></tr>`).join(""); const html = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>测试报告 #${detail.id}</title><style>body{font:14px system-ui;margin:40px;color:#1d2129}h1{font-size:24px}.summary{display:flex;gap:32px;margin:24px 0}.summary strong{display:block;font-size:28px}table{width:100%;border-collapse:collapse}th,td{padding:10px;border:1px solid #ddd;text-align:left}th{background:#f5f6f7}</style><h1>Synapse QA 测试报告 #${detail.id}</h1><p>状态：${escapeHTML(runStatusLabel(detail.status))}　类型：${escapeHTML(runTypeLabel(detail.runType))}　触发人：${escapeHTML(detail.triggeredBy)}</p><div class="summary"><div>总数<strong>${summary.total ?? detail.tasks?.length ?? 0}</strong></div><div>通过<strong>${summary.passed ?? 0}</strong></div><div>失败<strong>${summary.failed ?? 0}</strong></div><div>跳过<strong>${summary.skipped ?? 0}</strong></div></div><table><thead><tr><th>用例 ID</th><th>执行器</th><th>状态</th><th>输出</th><th>错误</th></tr></thead><tbody>${rows}</tbody></table></html>`; const url = URL.createObjectURL(new Blob([html], { type: "text/html;charset=utf-8" })); const link = document.createElement("a"); link.href = url; link.download = `synapse-report-${detail.id}.html`; link.click(); URL.revokeObjectURL(url); }
