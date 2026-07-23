import { useEffect, useState } from "react";
import { Activity, AlertTriangle, ArrowDownRight, ArrowUpRight, CheckCircle2, ChevronDown, Clock3, Download, FileText, Layers3, RefreshCw, Server, ShieldCheck, X } from "lucide-react";
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
  const { data: statistics, loading: statisticsLoading, error: statisticsError, reload: reloadStatistics } = useAsyncData(
    () => reportMode ? loadExecutionStatistics() : Promise.resolve(null),
    [reportMode]
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
    <div className={`section-stack execution-page ${reportMode ? "execution-report-page" : ""}`}>
      {reportMode ? <ReportStatistics data={statistics} loading={statisticsLoading} error={statisticsError} onRetry={reloadStatistics} /> : null}
      <section className="resource-panel">
        <div className="panel-header"><strong>{reportMode ? "测试报告" : "执行记录"}</strong></div>
        <form className="filter-grid execution-filter-grid" onSubmit={(event) => { event.preventDefault(); setFilters({ ...form }); setPage(1); }}>
          <label className="form-field"><span>批次 ID</span><input className="text-input" value={form.id} placeholder="请输入批次 ID" onChange={(event) => setForm({ ...form, id: event.target.value })} /></label>
          <label className="form-field"><span>执行类型</span><select className="text-input" value={form.runType} onChange={(event) => setForm({ ...form, runType: event.target.value })}><option value="">全部</option><option value="ui">UI</option><option value="api">API</option><option value="unit">单元测试</option><option value="noop">流程验证</option></select></label>
          <label className="form-field"><span>状态</span><select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}><option value="">全部</option><option value="running">执行中</option><option value="completed">已通过</option><option value="failed">失败</option><option value="canceled">已取消</option></select></label>
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
          <div className="toolbar-row"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={() => { setForm(initialFilters); setFilters(initialFilters); setPage(1); }} type="button">重置</button></div>
        </form>
        <div className="list-actions"><div /><button className="icon-text-button compact-button" onClick={() => { reload(); reloadStatistics(); }} type="button"><RefreshCw size={14} />刷新</button></div>
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

async function loadExecutionStatistics() {
  try {
    const result = await executionService.statistics();
    if (!result) throw new Error("统计接口未返回数据");
    return result;
  } catch {
    const runs = [];
    let page = 1;
    let total = 0;
    do {
      const result = await executionService.list({ page, pageSize: 100 });
      const items = pageItems(result);
      runs.push(...items);
      total = Number(result?.total || 0);
      page += 1;
      if (!items.length) break;
    } while (runs.length < total);
    return buildClientStatistics(runs);
  }
}

function buildClientStatistics(runs) {
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const trendStart = new Date(today);
  trendStart.setDate(today.getDate() - 13);
  const currentStart = new Date(today);
  currentStart.setDate(today.getDate() - 6);
  const previousStart = new Date(currentStart);
  previousStart.setDate(currentStart.getDate() - 7);
  const trend = Array.from({ length: 14 }, (_, index) => {
    const date = new Date(trendStart);
    date.setDate(trendStart.getDate() + index);
    return { date: `${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`, runs: 0, cases: 0, passed: 0, passRate: 0 };
  });
  const result = { totalRuns: runs.length, totalCases: 0, passedCases: 0, failedCases: 0, failedRuns: 0, runningRuns: 0, passRate: 0, runChange: 0, caseChange: 0, passRateChange: 0, trend };
  const periods = { current: { runs: 0, cases: 0, passed: 0 }, previous: { runs: 0, cases: 0, passed: 0 } };
  runs.forEach((run) => {
    const summary = run.summary || {};
    const cases = Number(summary.total ?? run.caseIds?.length ?? 0);
    const passed = Number(summary.passed || 0);
    result.totalCases += cases;
    result.passedCases += passed;
    result.failedCases += Number(summary.failed || 0);
    if (run.status === "failed") result.failedRuns += 1;
    if (["pending", "queued", "running"].includes(run.status)) result.runningRuns += 1;
    const created = new Date(run.createdAt || run.startedAt);
    const target = created >= currentStart ? periods.current : created >= previousStart ? periods.previous : null;
    if (target) {
      target.runs += 1;
      target.cases += cases;
      target.passed += passed;
    }
    const day = new Date(created.getFullYear(), created.getMonth(), created.getDate());
    const index = Math.round((day - trendStart) / 86400000);
    if (index >= 0 && index < trend.length) {
      trend[index].runs += 1;
      trend[index].cases += cases;
      trend[index].passed += passed;
    }
  });
  const percentOf = (value, total) => total ? value * 100 / total : 0;
  const changeOf = (current, previous) => previous ? (current - previous) * 100 / previous : current ? 100 : 0;
  result.passRate = percentOf(result.passedCases, result.totalCases);
  result.runChange = changeOf(periods.current.runs, periods.previous.runs);
  result.caseChange = changeOf(periods.current.cases, periods.previous.cases);
  result.passRateChange = percentOf(periods.current.passed, periods.current.cases) - percentOf(periods.previous.passed, periods.previous.cases);
  trend.forEach((point) => { point.passRate = percentOf(point.passed, point.cases); });
  return result;
}

function ReportStatistics({ data, loading, error, onRetry }) {
  if (loading) {
    return <section className="report-statistics report-statistics-loading" aria-label="报告统计加载中"><div /><div /><div /></section>;
  }
  if (error || !data) {
    return <section className="report-statistics-error"><Activity size={20} /><div><strong>统计数据暂时不可用</strong><span>{error || "未获取到统计结果"}</span></div><button className="icon-text-button compact-button" onClick={onRetry} type="button">重新加载</button></section>;
  }
  const trend = data.trend || [];
  const maxCases = Math.max(1, ...trend.map((point) => Number(point.cases || 0)));
  return (
    <section className="report-statistics" aria-label="全部批次统计">
      <div className="report-statistics-lead">
        <div className="report-statistics-heading">
          <span><Activity size={14} /> QUALITY PULSE · 全部批次</span>
          <strong>{Number(data.passRate || 0).toFixed(1)}<small>%</small></strong>
          <p>累计用例通过率</p>
        </div>
        <TrendValue value={data.passRateChange} suffix=" 个百分点" label="较前 7 天" />
      </div>
      <div className="report-statistics-metrics">
        <StatisticsMetric icon={<Layers3 size={16} />} label="累计执行批次" value={data.totalRuns} change={data.runChange} />
        <StatisticsMetric icon={<ShieldCheck size={16} />} label="累计执行用例" value={data.totalCases} change={data.caseChange} />
        <StatisticsMetric icon={<AlertTriangle size={16} />} label="失败批次" value={data.failedRuns} note={`${data.runningRuns || 0} 个批次执行中`} danger />
      </div>
      <div className="report-pulse-chart">
        <div className="report-pulse-title"><div><strong>近 14 天质量脉冲</strong><span>柱高代表用例量，绿色覆盖代表通过率</span></div><span>{data.passedCases || 0} 通过 / {data.failedCases || 0} 失败</span></div>
        <div className="report-pulse-bars">
          {trend.map((point) => {
            const height = Math.max(point.cases ? 12 : 3, Number(point.cases || 0) / maxCases * 100);
            return <div className="report-pulse-column" key={point.date} title={`${point.date} · ${point.runs} 批次 · ${point.cases} 用例 · 通过率 ${Number(point.passRate || 0).toFixed(1)}%`}><div className="report-pulse-track" style={{ height: `${height}%` }}><i style={{ height: `${Number(point.passRate || 0)}%` }} /></div><span>{point.date}</span></div>;
          })}
        </div>
      </div>
    </section>
  );
}

function StatisticsMetric({ icon, label, value, change, note, danger = false }) {
  return <div className={`report-statistics-metric ${danger ? "danger" : ""}`}><span className="report-statistics-icon">{icon}</span><div><span>{label}</span><strong>{Number(value || 0).toLocaleString()}</strong>{note ? <small>{note}</small> : <TrendValue value={change} label="较前 7 天" />}</div></div>;
}

function TrendValue({ value = 0, suffix = "%", label }) {
  const numeric = Number(value || 0);
  const positive = numeric >= 0;
  return <span className={`report-trend ${positive ? "up" : "down"}`}>{positive ? <ArrowUpRight size={12} /> : <ArrowDownRight size={12} />}{Math.abs(numeric).toFixed(1)}{suffix} <small>{label}</small></span>;
}

function ExecutionReport({ detail, onClose }) {
  const summary = detail.summary || {};
  const tasks = detail.tasks || [];
  const total = Number(summary.total ?? tasks.length);
  const passed = Number(summary.passed ?? tasks.filter((task) => task.status === "success").length);
  const failed = Number(summary.failed ?? tasks.filter((task) => task.status === "failed").length);
  const skipped = Number(summary.skipped ?? tasks.filter((task) => task.status === "skipped").length);
  const passRate = total ? Math.round((passed / total) * 100) : 0;
  const failedTasks = tasks.filter((task) => task.status === "failed");
  const [expandedTaskId, setExpandedTaskId] = useState(failedTasks[0]?.id ?? null);

  return (
    <div className="modal-backdrop report-backdrop">
      <section className="modal-card execution-report" aria-label={`测试报告 #${detail.id}`}>
        <header className="report-hero">
          <div className="report-hero-copy">
            <span className="report-eyebrow"><FileText size={14} /> SYNAPSE QA · 自动化测试报告</span>
            <h2>执行批次 #{detail.id}</h2>
            <p>{runTypeLabel(detail.runType)} 测试 · 由 {detail.triggeredBy || "系统"} 触发 · {formatTime(detail.startedAt || detail.createdAt)}</p>
          </div>
          <div className="report-hero-actions">
            <span className={`report-status report-status-${detail.status}`}>{runStatusLabel(detail.status)}</span>
            <button className="report-icon-button" onClick={onClose} aria-label="关闭报告" type="button"><X size={18} /></button>
          </div>
        </header>

        <div className="report-scroll">
          <section className="report-overview">
            <div className="report-score-card">
              <div className="report-donut" style={{ "--score": `${passRate * 3.6}deg` }}><div><strong>{passRate}%</strong><span>通过率</span></div></div>
              <div className="report-conclusion">
                <span>质量结论</span>
                <strong>{failed > 0 ? "存在失败用例，需要关注" : total > 0 ? "本次执行全部通过" : "暂无可分析用例"}</strong>
                <p>{failed > 0 ? `${failed} 条用例失败，建议优先检查错误日志与执行环境。` : "当前批次未发现阻断问题。"}</p>
              </div>
            </div>
            <div className="report-metric-grid">
              <ReportMetric label="用例总数" value={total} icon={<FileText size={16} />} />
              <ReportMetric label="通过" value={passed} tone="success" icon={<CheckCircle2 size={16} />} />
              <ReportMetric label="失败" value={failed} tone="danger" icon={<AlertTriangle size={16} />} />
              <ReportMetric label="执行耗时" value={formatDuration(detail.startedAt, detail.finishedAt)} icon={<Clock3 size={16} />} compact />
            </div>
          </section>

          <section className="report-section report-distribution">
            <div className="report-section-heading"><div><strong>结果分布</strong><span>本批次各状态用例占比</span></div><span>{total} 条用例</span></div>
            <div className="report-segments" aria-label="测试结果分布">
              {passed > 0 ? <span className="passed" style={{ width: `${(passed / total) * 100}%` }} /> : null}
              {failed > 0 ? <span className="failed" style={{ width: `${(failed / total) * 100}%` }} /> : null}
              {skipped > 0 ? <span className="skipped" style={{ width: `${(skipped / total) * 100}%` }} /> : null}
            </div>
            <div className="report-legend"><span><i className="passed" />通过 {passed}</span><span><i className="failed" />失败 {failed}</span><span><i className="skipped" />跳过 {skipped}</span></div>
          </section>

          {failedTasks.length ? (
            <section className="report-section report-failure-focus">
              <div className="report-section-heading"><div><strong><AlertTriangle size={16} /> 失败定位</strong><span>优先展示阻断本次质量结论的错误</span></div><span>{failedTasks.length} 项</span></div>
              <div className="report-failure-grid">
                {failedTasks.slice(0, 3).map((task) => <article key={task.id}><span>用例 #{task.caseId}</span><strong>{task.result?.error || "执行失败，未返回错误详情"}</strong><small>执行器 #{task.executorId || "-"}</small></article>)}
              </div>
            </section>
          ) : null}

          <section className="report-section">
            <div className="report-section-heading"><div><strong>执行信息</strong><span>批次环境与时间信息</span></div></div>
            <div className="report-meta-grid">
              <div><span>执行类型</span><strong>{runTypeLabel(detail.runType)}</strong></div>
              <div><span>触发人</span><strong>{detail.triggeredBy || "-"}</strong></div>
              <div><span>开始时间</span><strong>{formatTime(detail.startedAt)}</strong></div>
              <div><span>结束时间</span><strong>{formatTime(detail.finishedAt)}</strong></div>
              <div><span>执行耗时</span><strong>{formatDuration(detail.startedAt, detail.finishedAt)}</strong></div>
              <div><span>批次 ID</span><strong>#{detail.id}</strong></div>
            </div>
          </section>

          <section className="report-section report-task-list">
            <div className="report-section-heading"><div><strong>用例执行明细</strong><span>点击用例查看完整输出与错误日志</span></div><span>{tasks.length} 项</span></div>
            <div className="report-task-cards">
              {tasks.length ? tasks.map((task) => {
                const expanded = expandedTaskId === task.id;
                return (
                  <article className={`report-task-card ${expanded ? "expanded" : ""}`} key={task.id}>
                    <button onClick={() => setExpandedTaskId(expanded ? null : task.id)} type="button">
                      <span className={`report-task-state ${task.status}`}><TaskStatusIcon status={task.status} /></span>
                      <span className="report-task-title"><strong>用例 #{task.caseId}</strong><small><Server size={12} /> 执行器 #{task.executorId || "-"} · {formatDuration(task.startedAt, task.finishedAt)}</small></span>
                      <span className={`report-task-label ${task.status}`}>{runStatusLabel(task.status)}</span>
                      <ChevronDown className="report-task-chevron" size={16} />
                    </button>
                    {expanded ? <div className="report-task-detail"><LogBlock label="执行输出" value={task.result?.output} /><LogBlock label="错误信息" value={task.result?.error} danger /></div> : null}
                  </article>
                );
              }) : <div className="report-empty">暂无任务明细</div>}
            </div>
          </section>
        </div>
        <footer className="report-footer">
          <span>报告生成于 {formatTime(new Date().toISOString())}</span>
          <div><button className="icon-text-button compact-button" onClick={() => downloadRichReport(detail)} type="button"><Download size={14} />导出 HTML</button><button className="primary-button compact-button" onClick={onClose} type="button">完成</button></div>
        </footer>
      </section>
    </div>
  );
}

function ReportMetric({ label, value, tone = "", icon, compact = false }) {
  return <div className={`report-metric ${tone} ${compact ? "compact" : ""}`}><span className="report-metric-icon">{icon}</span><div><span>{label}</span><strong>{value}</strong></div></div>;
}
function TaskStatusIcon({ status }) { return status === "success" ? <CheckCircle2 size={17} /> : status === "failed" ? <AlertTriangle size={17} /> : <Clock3 size={17} />; }
function LogBlock({ label, value, danger = false }) { return <div className={danger ? "danger" : ""}><span>{label}</span><pre>{value || "暂无内容"}</pre></div>; }
function formatDuration(start, end) {
  if (!start || !end) return "-";
  const duration = Math.max(0, new Date(end).getTime() - new Date(start).getTime());
  if (!Number.isFinite(duration)) return "-";
  if (duration < 1000) return `${duration} ms`;
  if (duration < 60000) return `${(duration / 1000).toFixed(duration < 10000 ? 1 : 0)} 秒`;
  const minutes = Math.floor(duration / 60000);
  return `${minutes} 分 ${Math.round((duration % 60000) / 1000)} 秒`;
}

function LegacyExecutionReport({ detail, onClose }) {
  const summary = detail.summary || {};
  return <div className="modal-backdrop"><section className="modal-card modal-card-wide execution-report"><div className="modal-header"><strong>测试报告 #{detail.id}</strong><button className="modal-close" onClick={onClose} type="button">×</button></div><div className="report-summary"><ReportMetric label="用例总数" value={summary.total ?? detail.tasks?.length ?? 0} /><ReportMetric label="通过" value={summary.passed ?? 0} tone="success" /><ReportMetric label="失败" value={summary.failed ?? 0} tone="danger" /><ReportMetric label="跳过" value={summary.skipped ?? 0} /></div><div className="detail-grid"><div><span>执行类型</span><strong>{runTypeLabel(detail.runType)}</strong></div><div><span>执行状态</span><strong>{runStatusLabel(detail.status)}</strong></div><div><span>触发人</span><strong>{detail.triggeredBy || "-"}</strong></div><div><span>开始时间</span><strong>{formatTime(detail.startedAt)}</strong></div><div><span>结束时间</span><strong>{formatTime(detail.finishedAt)}</strong></div><div><span>批次 ID</span><strong>{detail.id}</strong></div></div><div className="report-task-list"><strong>任务明细</strong><div className="table-wrap"><table className="data-table"><thead><tr><th>用例 ID</th><th>执行器</th><th>状态</th><th>输出</th><th>错误</th></tr></thead><tbody>{detail.tasks?.length ? detail.tasks.map((task) => <tr key={task.id}><td>{task.caseId}</td><td>{task.executorId}</td><td>{runStatusLabel(task.status)}</td><td className="cell-ellipsis" title={task.result?.output || ""}>{task.result?.output || "-"}</td><td className="cell-ellipsis" title={task.result?.error || ""}>{task.result?.error || "-"}</td></tr>) : <tr><td colSpan="5">暂无任务明细</td></tr>}</tbody></table></div></div><div className="modal-actions"><button className="icon-text-button compact-button" onClick={() => downloadReport(detail)} type="button"><Download size={14} />导出报告</button><button className="primary-button compact-button" onClick={onClose} type="button">关闭</button></div></section></div>;
}

function LegacyReportMetric({ label, value, tone = "" }) { return <div className={`report-metric ${tone}`}><span>{label}</span><strong>{value}</strong></div>; }
function ExecutionProgress({ summary = {}, totalFallback = 0 }) {
  const total = Number(summary.total ?? totalFallback ?? 0);
  const completed = Number(summary.passed || 0) + Number(summary.failed || 0) + Number(summary.skipped || 0);
  const percent = total > 0 ? Math.min(100, Math.round((completed / total) * 100)) : 0;
  return <div className="execution-progress" title={`已完成 ${completed}/${total}`}><div className="execution-progress-track"><span style={{ width: `${percent}%` }} /></div><small>{percent}%</small></div>;
}
function runTypeLabel(value) { return { ui: "UI", api: "API", unit: "单元测试", script: "脚本", noop: "流程验证" }[value] || value || "-"; }
function runStatusLabel(value) { return { pending: "等待中", queued: "排队中", running: "执行中", success: "通过", completed: "已通过", failed: "失败", canceled: "已取消" }[value] || value || "-"; }
function escapeHTML(value) { return String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]); }
function downloadRichReport(detail) {
  const summary = detail.summary || {};
  const tasks = detail.tasks || [];
  const total = Number(summary.total ?? tasks.length);
  const passed = Number(summary.passed ?? tasks.filter((task) => task.status === "success").length);
  const failed = Number(summary.failed ?? tasks.filter((task) => task.status === "failed").length);
  const skipped = Number(summary.skipped ?? tasks.filter((task) => task.status === "skipped").length);
  const rate = total ? Math.round((passed / total) * 100) : 0;
  const cards = tasks.map((task) => `<article class="task ${task.status === "failed" ? "bad" : ""}"><header><div><b>用例 #${escapeHTML(task.caseId)}</b><small>执行器 #${escapeHTML(task.executorId || "-")} · ${escapeHTML(formatDuration(task.startedAt, task.finishedAt))}</small></div><em class="${escapeHTML(task.status)}">${escapeHTML(runStatusLabel(task.status))}</em></header><div class="logs"><div><label>执行输出</label><pre>${escapeHTML(task.result?.output || "暂无内容")}</pre></div><div><label>错误信息</label><pre>${escapeHTML(task.result?.error || "暂无内容")}</pre></div></div></article>`).join("");
  const html = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>测试报告 #${escapeHTML(detail.id)}</title><style>
  :root{--ink:#172033;--muted:#667085;--line:#e5e9ef;--bg:#f4f6f9;--green:#1d9b6c;--red:#dc4c4c;--amber:#d78a32}*{box-sizing:border-box}body{margin:0;color:var(--ink);background:var(--bg);font:14px Inter,"PingFang SC","Microsoft YaHei",system-ui,sans-serif}.page{width:min(1060px,calc(100% - 32px));margin:32px auto}.hero{display:flex;justify-content:space-between;gap:24px;padding:34px 38px;color:#fff;background:linear-gradient(130deg,#172033,#263653);border-radius:17px 17px 0 0}.hero small{color:#aeb9ca;letter-spacing:.1em}.hero h1{margin:10px 0 7px;font-size:28px}.hero p{margin:0;color:#b9c3d1}.badge{align-self:flex-start;padding:6px 11px;color:#bbf7d0;background:#22c55e22;border:1px solid #86efac30;border-radius:99px}.content{display:grid;gap:17px;padding:24px;background:#fff;border:1px solid var(--line);border-top:0;border-radius:0 0 17px 17px}.overview{display:grid;grid-template-columns:1.1fr 1fr;gap:13px}.score{display:flex;align-items:center;gap:27px;padding:23px;background:#fbfcfd;border:1px solid var(--line);border-radius:11px}.ring{display:grid;width:118px;height:118px;flex:none;place-items:center;background:conic-gradient(var(--green) ${rate * 3.6}deg,#e8ecf1 0);border-radius:50%}.ring div{display:grid;width:90px;height:90px;place-content:center;text-align:center;background:#fff;border-radius:50%}.ring b{font-size:27px}.ring small{color:var(--muted)}.verdict b{display:block;margin:7px 0;font-size:18px}.verdict p{margin:0;color:var(--muted);line-height:1.6}.metrics{display:grid;grid-template-columns:1fr 1fr;gap:9px}.metric{display:grid;align-content:center;padding:17px;background:#fbfcfd;border:1px solid var(--line);border-radius:9px}.metric span{color:var(--muted);font-size:11px}.metric b{margin-top:5px;font-size:23px}.green b{color:var(--green)}.red b{color:var(--red)}.section{padding:19px;border:1px solid var(--line);border-radius:11px}.section h2{margin:0 0 4px;font-size:15px}.section>p{margin:0 0 15px;color:var(--muted);font-size:11px}.bar{display:flex;overflow:hidden;height:9px;background:#edf0f4;border-radius:99px}.bar i:nth-child(1){background:var(--green)}.bar i:nth-child(2){background:var(--red)}.bar i:nth-child(3){background:var(--amber)}.legend{display:flex;gap:22px;margin-top:10px;color:var(--muted);font-size:11px}.meta{display:grid;grid-template-columns:repeat(3,1fr);overflow:hidden;border:1px solid var(--line);border-radius:8px}.meta div{display:grid;gap:5px;padding:12px;border-right:1px solid var(--line);border-bottom:1px solid var(--line)}.meta div:nth-child(3n){border-right:0}.meta div:nth-last-child(-n+3){border-bottom:0}.meta span,.task small,label{color:var(--muted);font-size:10px}.tasks{display:grid;gap:8px}.task{overflow:hidden;border:1px solid var(--line);border-radius:8px}.task.bad{border-color:#efcaca}.task header{display:flex;justify-content:space-between;align-items:center;padding:13px}.task header div{display:grid;gap:5px}.task em{padding:4px 8px;background:#eef1f5;border-radius:99px;font-size:10px;font-style:normal}.task em.success{color:var(--green);background:#e9f6f1}.task em.failed{color:var(--red);background:#fceeee}.logs{display:grid;grid-template-columns:1fr 1fr;gap:1px;background:var(--line);border-top:1px solid var(--line)}.logs div{min-width:0;padding:12px;background:#fafbfc}pre{overflow:auto;margin:7px 0 0;font:10px/1.55 Consolas,monospace;white-space:pre-wrap;word-break:break-word}.foot{color:var(--muted);font-size:10px;text-align:center}@media(max-width:720px){.overview{grid-template-columns:1fr}.meta,.logs{grid-template-columns:1fr}.hero{padding:25px}.content{padding:13px}}@media print{body{background:#fff}.page{width:100%;margin:0}.hero,.content{border-radius:0}.task{break-inside:avoid}}
  </style></head><body><main class="page"><header class="hero"><div><small>SYNAPSE QA · 自动化测试报告</small><h1>执行批次 #${escapeHTML(detail.id)}</h1><p>${escapeHTML(runTypeLabel(detail.runType))} 测试 · ${escapeHTML(detail.triggeredBy || "系统")} · ${escapeHTML(formatTime(detail.startedAt || detail.createdAt))}</p></div><span class="badge">${escapeHTML(runStatusLabel(detail.status))}</span></header><div class="content">
  <section class="overview"><div class="score"><div class="ring"><div><b>${rate}%</b><small>通过率</small></div></div><div class="verdict"><span>质量结论</span><b>${failed ? "存在失败用例，需要关注" : "本次执行全部通过"}</b><p>${failed ? `${failed} 条用例失败，请优先检查错误日志。` : "当前批次未发现阻断问题。"}</p></div></div><div class="metrics"><div class="metric"><span>用例总数</span><b>${total}</b></div><div class="metric green"><span>通过</span><b>${passed}</b></div><div class="metric red"><span>失败</span><b>${failed}</b></div><div class="metric"><span>执行耗时</span><b>${escapeHTML(formatDuration(detail.startedAt, detail.finishedAt))}</b></div></div></section>
  <section class="section"><h2>结果分布</h2><p>本批次各状态用例占比</p><div class="bar"><i style="width:${total ? passed / total * 100 : 0}%"></i><i style="width:${total ? failed / total * 100 : 0}%"></i><i style="width:${total ? skipped / total * 100 : 0}%"></i></div><div class="legend"><span>通过 ${passed}</span><span>失败 ${failed}</span><span>跳过 ${skipped}</span></div></section>
  <section class="section"><h2>执行信息</h2><p>批次环境与时间信息</p><div class="meta"><div><span>执行类型</span><b>${escapeHTML(runTypeLabel(detail.runType))}</b></div><div><span>触发人</span><b>${escapeHTML(detail.triggeredBy || "-")}</b></div><div><span>批次 ID</span><b>#${escapeHTML(detail.id)}</b></div><div><span>开始时间</span><b>${escapeHTML(formatTime(detail.startedAt))}</b></div><div><span>结束时间</span><b>${escapeHTML(formatTime(detail.finishedAt))}</b></div><div><span>执行耗时</span><b>${escapeHTML(formatDuration(detail.startedAt, detail.finishedAt))}</b></div></div></section>
  <section class="section"><h2>用例执行明细</h2><p>共 ${tasks.length} 项，包含完整输出与错误日志</p><div class="tasks">${cards || "<p>暂无任务明细</p>"}</div></section><footer class="foot">报告由 Synapse QA 生成 · ${escapeHTML(formatTime(new Date().toISOString()))}</footer></div></main></body></html>`;
  const url = URL.createObjectURL(new Blob([html], { type: "text/html;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = `synapse-report-${detail.id}.html`;
  link.click();
  URL.revokeObjectURL(url);
}
function downloadReport(detail) { const summary = detail.summary || {}; const rows = (detail.tasks || []).map((task) => `<tr><td>${escapeHTML(task.caseId)}</td><td>${escapeHTML(task.executorId)}</td><td>${escapeHTML(runStatusLabel(task.status))}</td><td>${escapeHTML(task.result?.output)}</td><td>${escapeHTML(task.result?.error)}</td></tr>`).join(""); const html = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>测试报告 #${detail.id}</title><style>body{font:14px system-ui;margin:40px;color:#1d2129}h1{font-size:24px}.summary{display:flex;gap:32px;margin:24px 0}.summary strong{display:block;font-size:28px}table{width:100%;border-collapse:collapse}th,td{padding:10px;border:1px solid #ddd;text-align:left}th{background:#f5f6f7}</style><h1>Synapse QA 测试报告 #${detail.id}</h1><p>状态：${escapeHTML(runStatusLabel(detail.status))}　类型：${escapeHTML(runTypeLabel(detail.runType))}　触发人：${escapeHTML(detail.triggeredBy)}</p><div class="summary"><div>总数<strong>${summary.total ?? detail.tasks?.length ?? 0}</strong></div><div>通过<strong>${summary.passed ?? 0}</strong></div><div>失败<strong>${summary.failed ?? 0}</strong></div><div>跳过<strong>${summary.skipped ?? 0}</strong></div></div><table><thead><tr><th>用例 ID</th><th>执行器</th><th>状态</th><th>输出</th><th>错误</th></tr></thead><tbody>${rows}</tbody></table></html>`; const url = URL.createObjectURL(new Blob([html], { type: "text/html;charset=utf-8" })); const link = document.createElement("a"); link.href = url; link.download = `synapse-report-${detail.id}.html`; link.click(); URL.revokeObjectURL(url); }
