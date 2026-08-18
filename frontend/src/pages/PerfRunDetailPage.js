import { useEffect, useRef, useState } from "react";
import { AlertTriangle, CheckCircle2, ChevronLeft, Clock3, RefreshCw, XCircle } from "lucide-react";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { useAuth } from "../hooks/useAuth.js";
import { performanceService } from "../services/performanceService.js";
import { formatTime } from "../utils/formatters.js";
import {
  environmentLabel,
  environmentTone,
  extractThresholdResults,
  failureStageLabel,
  maskSensitiveUrl,
  metricValueFromSummary,
  numOrDash,
  renderMs,
  renderPercent,
  runStatusLabel,
  runStatusRunning,
  runStatusTone,
  scenarioLabel,
  mergeSeries,
  normalizeSeries
} from "../utils/perf.js";
import { pathToHash } from "../utils/routeState.js";

// 报告详情页（SPEC §9 四屏：结论、阈值结果、趋势分布与诊断信息）。
export function PerfRunDetailPage({ runId }) {
  const { user } = useAuth();
  const granted = new Set(user?.permissions || []);
  const isAdmin = user?.roleCode === "admin";
  const canExecute = isAdmin || granted.has("perf.plan.execute");

  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [series, setSeries] = useState(() => normalizeSeries(null));
  const [streamState, setStreamState] = useState("连接中");
  const seriesRef = useRef(series);
  const streamRef = useRef(null);
  const reconnectRef = useRef(null);
  const terminalRef = useRef(false);
  const terminalReloadRef = useRef(false);
  const { data: run, loading, error, reload } = useAsyncData(() => performanceService.runs.get(runId), [runId]);
  const terminal = Boolean(run && ["completed", "threshold_failed"].includes(run.status));
  const canManage = isAdmin || granted.has("perf.plan.manage");
  const { data: baselineData, reload: reloadBaseline } = useAsyncData(() => terminal ? performanceService.runs.baseline.get(runId).catch(() => null) : Promise.resolve(null), [runId, terminal]);
  const { data: compareData, reload: reloadCompare } = useAsyncData(() => terminal ? performanceService.runs.compare(runId).catch(() => null) : Promise.resolve(null), [runId, terminal]);
  const { data: trendData } = useAsyncData(() => terminal ? performanceService.runs.trend(runId, 20).catch(() => ({ items: [] })) : Promise.resolve({ items: [] }), [runId, terminal]);
  const [exporting, setExporting] = useState("");
  const baselineExists = Boolean(baselineData?.exists && baselineData.baseline);
  const baselineIsCurrentRun = Boolean(baselineData?.isCurrentRun);

  useEffect(() => { if (run) { const next = normalizeSeries(run.series); seriesRef.current = next; setSeries(next); } }, [run?.id, run?.series?.lastSequence]);
  useEffect(() => {
    if (!run || !runStatusRunning(run.status)) return undefined;
    terminalRef.current = false;
    terminalReloadRef.current = false;
    let stopped = false;
    let delay = 500;
    const connect = () => {
      if (stopped) return;
      setStreamState("连接中");
      const controller = performanceService.runs.stream(run.id, seriesRef.current.lastSequence, {
        "perf.sample": (sample) => { setStreamState("已连接"); delay = 500; setSeries((current) => { const next = mergeSeries(current, sample); seriesRef.current = next; return next; }); },
        "perf.reset": (payload) => { const next = normalizeSeries(payload?.series || payload); seriesRef.current = next; setSeries(next); },
        "perf.terminal": async () => { terminalRef.current = true; window.clearTimeout(reconnectRef.current); streamRef.current?.cancel(); setStreamState("已结束"); if (!terminalReloadRef.current) { terminalReloadRef.current = true; await reload(); } }
      });
      streamRef.current = controller;
      controller.promise.catch(() => {}).finally(() => {
        if (stopped || controller.signal.aborted || terminalRef.current) return;
        setStreamState("监控连接已断开，压测可能仍在执行");
        reconnectRef.current = window.setTimeout(connect, delay);
        delay = Math.min(delay * 2, 10000);
      });
    };
    connect();
    return () => { stopped = true; streamRef.current?.cancel(); window.clearTimeout(reconnectRef.current); };
  }, [run?.id, run?.status]);

  function go(path) {
    window.location.hash = pathToHash(path);
  }

  async function cancelRun() {
    if (!run) return;
    if (!window.confirm(`确认取消执行记录 #${run.id} 吗？`)) return;
    setBusy(true);
    setNotice("");
    try {
      await performanceService.runs.cancel(run.id);
      await reload();
      setNotice("已请求取消执行。");
    } catch (err) {
      setNotice(err.message || "取消失败");
    } finally {
      setBusy(false);
    }
  }

  async function setBaseline(remove = false) {
    if (!remove && run.status === "threshold_failed") return;
    if (!window.confirm(remove ? "确认取消当前基线吗？" : (baselineExists ? "确认替换当前基线吗？" : "确认将本次执行设为基线吗？"))) return;
    try { await (remove ? performanceService.runs.baseline.remove(run.id) : performanceService.runs.baseline.set(run.id)); await reloadBaseline(); await reloadCompare(); } catch (err) { setNotice(err.message || "基线操作失败"); }
  }

  async function download(format) {
    setExporting(format);
    try { const result = await performanceService.runs.export(run.id, format); const url = URL.createObjectURL(result.blob); const link = document.createElement("a"); link.href = url; link.download = result.filename; link.click(); URL.revokeObjectURL(url); } catch (err) { setNotice(err.message || "导出失败"); } finally { setExporting(""); }
  }

  if (loading || error) {
    return <StateBlock loading={loading} error={error} />;
  }

  const conclusion = conclusionFor(run.status);
  const p99 = run.p99DurationMs ?? metricValueFromSummary(run.summary, "http_req_duration", "p(99)");
  const thresholdResults = extractThresholdResults(run.summary);
  const targetUrl = run.planSnapshot?.targetUrl;
  const latest = series.points.at(-1) || {};
  const displayStatus = latest.status || run.status;

  return (
    <div className="section-stack perf-run-detail">
      <div className="perf-edit-header">
        <button className="icon-text-button compact-button" onClick={() => go(["性能测试", "测试报告"])} type="button">
          <ChevronLeft size={15} />返回列表
        </button>
        <div className="perf-edit-title">
          <strong>执行记录 / #{run.id} / {run.planName || "-"}</strong>
          <span>{scenarioLabel(run.scenarioType)} · {environmentLabel(run.environment)}</span>
        </div>
        <div className="action-row">
          <button className="icon-text-button compact-button" onClick={() => reload()} type="button"><RefreshCw size={14} />刷新</button>
          {canExecute && runStatusRunning(run.status) ? <button className="danger-button compact-button" disabled={busy} onClick={cancelRun} type="button">取消执行</button> : null}
        </div>
      </div>

      {run.environmentName || run.environmentBaseUrl || run.planSnapshot?.resolvedTargetUrl ? <div className="perf-live-meta"><span>环境：{run.environmentName || environmentLabel(run.environment)}</span>{run.environmentBaseUrl ? <span>Base URL：{maskSensitiveUrl(run.environmentBaseUrl)}</span> : null}<span>最终目标：{maskSensitiveUrl(run.planSnapshot?.resolvedTargetUrl || targetUrl)}</span></div> : null}

      {notice ? <div className="inline-notice">{notice}</div> : null}

      {runStatusRunning(run.status) ? <section className="resource-panel perf-live-panel">
        <div className="panel-header"><strong>实时监控</strong><span className={`status-pill ${runStatusTone(displayStatus)}`}>{runStatusLabel(displayStatus)}</span><span className="perf-stream-state">{streamState}</span></div>
        <div className="perf-live-meta"><span>运行时长：{formatDuration(run)}</span><span>执行器：{run.executorName || run.executorId || "-"}</span><span>阶段：{latest.stage || "--"}</span><span>剩余：{formatRemaining(latest.remainingMs)}</span></div>
        <div className="perf-metric-grid"><PerfMetric label="当前 VU" value={numOrDash(latest.vus)} /><PerfMetric label="当前 RPS" value={numOrDash(latest.rps)} /><PerfMetric label="当前 P95" value={renderMs(latest.p95)} /><PerfMetric label="错误率" value={formatRate(latest.errorRate)} /></div>
      </section> : null}

      {/* 第 1 屏：结论 */}
      <section className="resource-panel perf-conclusion">
        <div className="perf-conclusion-lead">
          <span className={`perf-conclusion-icon ${conclusion.tone}`}>{conclusion.icon}</span>
          <div>
            <span className="perf-conclusion-kicker">PERFORMANCE CONCLUSION</span>
            <strong>{conclusion.title}</strong>
            <p>{conclusion.desc}</p>
          </div>
        </div>
        <div className="perf-conclusion-meta">
          <span>场景：{scenarioLabel(run.scenarioType)}</span>
          <span>环境：<span className={`status-pill ${environmentTone(run.environment)}`}>{environmentLabel(run.environment)}</span></span>
          <span>目标：<code className="perf-url">{maskSensitiveUrl(targetUrl)}</code></span>
          <span>执行时间：{formatTime(run.startedAt || run.requestedAt || run.createdAt)}</span>
          <span>配置版本：{run.configHash ? run.configHash.slice(0, 12) : "-"}</span>
        </div>
        <div className="perf-metric-grid">
          <PerfMetric label="P95 耗时" value={renderMs(run.p95DurationMs)} />
          <PerfMetric label="P99 耗时" value={renderMs(p99)} />
          <PerfMetric label="错误率" value={renderPercent(run.errorRate)} />
          <PerfMetric label="RPS" value={numOrDash(run.rps)} />
          <PerfMetric label="总请求数" value={numOrDash(run.totalRequests)} />
        </div>
      </section>

      {/* 第 2 屏：阈值结果 */}
      <section className="resource-panel">
        <div className="panel-header"><strong>阈值结果</strong></div>
        {thresholdResults.length ? (
          <div className="table-wrap">
            <table className="data-table perf-threshold-results">
              <thead><tr><th>表达式</th><th>实际值</th><th>目标值</th><th>差距</th><th>结果</th></tr></thead>
              <tbody>
                {thresholdResults.map((item, index) => (
                  <tr key={index}>
                    <td><code>{item.expression}</code></td>
                    <td>{formatThresholdValue(item.actual)}</td>
                    <td>{item.target}</td>
                    <td>{formatGap(item)}</td>
                    <td><span className={`status-pill ${item.passed ? "success" : "danger"}`}>{item.passed ? "通过" : "失败"}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <span className="muted-text">{run.status === "completed" || run.status === "threshold_failed" ? "未返回阈值明细" : "执行未产生阈值结果"}</span>
        )}
      </section>

      {terminal ? <section className="resource-panel perf-p2-panel"><div className="panel-header"><strong>基线与比较</strong>{baselineIsCurrentRun ? <span className="status-pill success">当前基线</span> : null}<div className="action-row"><button className="icon-text-button compact-button" disabled={exporting === "csv"} onClick={() => download("csv")} type="button">{exporting === "csv" ? "导出中" : "导出 CSV"}</button><button className="icon-text-button compact-button" disabled={exporting === "json"} onClick={() => download("json")} type="button">{exporting === "json" ? "导出中" : "导出 JSON"}</button>{canManage && run.status === "completed" && run.p95DurationMs != null && p99 != null && run.errorRate != null && run.rps != null ? <button className="primary-button compact-button" onClick={() => setBaseline(false)} type="button">{baselineExists ? "设为基线（替换）" : "设为基线"}</button> : null}{canManage && baselineIsCurrentRun ? <button className="icon-text-button compact-button" onClick={() => setBaseline(true)} type="button">取消基线</button> : null}</div></div><CompareCard data={compareData} /></section> : null}

      {terminal ? <section className="resource-panel"><div className="panel-header"><strong>同方案趋势</strong></div><TrendTable data={trendData} /></section> : null}

      {/* 第 3 屏：趋势与分布 */}
      <section className="resource-panel">
        <div className="panel-header"><strong>趋势与分布</strong></div>
        {series.points.length ? <SeriesCharts series={series} run={run} /> : <div className="perf-empty-state">暂无时序数据。报告不会用零值代替。</div>}
      </section>

      {/* 第 4 屏：诊断信息 */}
      <section className="resource-panel">
        <div className="panel-header"><strong>诊断信息</strong></div>
        <div className="detail-grid">
          <span>执行器：{run.executorName || run.executorId || "-"}</span>
          <span>k6 版本：{run.k6Version || "-"}</span>
          <span>退出码：{run.exitCode ?? "-"}</span>
          <span>失败阶段：{failureStageLabel(run.failureStage)}</span>
          <span>任务 ID：{run.taskId || "-"}</span>
          <span>执行耗时：{formatDuration(run)}</span>
        </div>
        {run.errorMessage ? (
          <div className="detail-block">
            <strong>错误信息</strong>
            <pre className="perf-json perf-log">{run.errorMessage}</pre>
          </div>
        ) : null}
        <div className="detail-block">
          <strong>执行输出（脱敏，最后 32 KB）</strong>
          <pre className="perf-json perf-log">{run.diagnosticOutput || "暂无输出"}</pre>
        </div>
        <details className="perf-advanced">
          <summary>原始 summary</summary>
          <pre className="perf-json">{JSON.stringify(run.summary || {}, null, 2)}</pre>
        </details>
        <details className="perf-advanced">
          <summary>方案执行快照（plan_snapshot）</summary>
          <pre className="perf-json">{JSON.stringify(run.planSnapshot || {}, null, 2)}</pre>
        </details>
      </section>
    </div>
  );
}

function CompareCard({ data }) {
  if (!data) return <div className="perf-empty-state">暂无可比较基线。</div>;
  if (data.comparable === false) return <div className="perf-empty-state">当前基线不可比较：{data.reason || "指标维度或数据不完整"}。</div>;
  const metrics = data.metrics || {};
  const rows = [["P95", metrics.p95DurationMs], ["P99", metrics.p99DurationMs], ["错误率", metrics.errorRate], ["RPS", metrics.rps]];
  return <div className="perf-compare-grid">{rows.map(([label, item]) => { const value = item?.current ?? item; const comparable = item?.comparable !== false && item?.baseline != null && value != null; const degraded = item?.degraded; return <div className={`perf-compare-card ${degraded ? "degraded" : comparable ? "improved" : ""}`} key={label}><span>{label}</span><strong>{value == null ? "--" : label === "错误率" ? `${(Number(value) * 100).toFixed(2)}%` : value}</strong><small>{!comparable ? (item?.reason || "基线值缺失，无法比较") : `${item.delta == null ? "--" : item.delta} · ${item.changeRate == null ? "--" : `${(Number(item.changeRate) * 100).toFixed(2)}%`}`}</small></div>; })}</div>;
}

function TrendTable({ data }) {
  const items = Array.isArray(data) ? data : (data?.items || []);
  if (!items.length) return <div className="perf-empty-state">暂无同方案趋势数据。</div>;
  const [metric, setMetric] = useState("p95DurationMs");
  const values = items.map((item) => Number(item[metric])).filter(Number.isFinite);
  const max = Math.max(...values, 1);
  const path = items.map((item, index) => { const value = Number(item[metric]); const x = items.length === 1 ? 50 : index / (items.length - 1) * 100; const y = Number.isFinite(value) ? 42 - value / max * 36 : 42; return `${index && Number.isFinite(Number(items[index - 1][metric])) ? "L" : "M"}${x.toFixed(2)},${y.toFixed(2)}`; }).join(" ");
  return <div><div className="perf-trend-tabs">{[["p95DurationMs", "P95"], ["p99DurationMs", "P99"], ["errorRate", "错误率"], ["rps", "RPS"]].map(([key, label]) => <button className={metric === key ? "active" : ""} key={key} onClick={() => setMetric(key)} type="button">{label}</button>)}</div><svg className="perf-sparkline" viewBox="0 0 100 46" role="img" aria-label="同方案趋势"><path d="M0,42H100" className="perf-chart-axis" /><path d={path} fill="none" stroke="#49d6c8" strokeWidth="1.7" vectorEffect="non-scaling-stroke" /></svg><div className="table-wrap"><table className="data-table"><thead><tr><th>时间</th><th>状态</th><th>环境</th><th>P95</th><th>P99</th><th>错误率</th><th>RPS</th></tr></thead><tbody>{items.map((item, index) => <tr key={`${item.runId || item.id}-${index}`} className={item.degraded ? "perf-row-degraded" : ""}><td>{item.finishedAt || "--"}</td><td>{item.status || "--"}{item.isBaseline ? <span className="status-pill success">基线</span> : null}</td><td>{data?.environmentName || data?.environment || item.environmentName || item.environment || "--"}</td><td>{item.p95DurationMs ?? "--"}</td><td>{item.p99DurationMs ?? "--"}</td><td>{item.errorRate == null ? "--" : `${(Number(item.errorRate) * 100).toFixed(2)}%`}</td><td>{item.rps ?? "--"}</td></tr>)}</tbody></table></div></div>;
}

function formatRemaining(value) {
  if (value == null) return "--";
  return value < 60000 ? `${Math.max(0, Math.round(value / 1000))} 秒` : `${Math.round(value / 60000)} 分钟`;
}

function formatRate(value) {
  return renderPercent(value);
}

function SeriesCharts({ series, run }) {
  const points = series.points || [];
  const lines = [
    ["RPS", "rps", "#49d6c8", "req/s"],
    ["延迟", ["p50", "p90", "p95", "p99"], "#f2b35b", "ms"],
    ["错误率", "errorRate", "#ff6b8a", "%"],
    ["活跃 VU", "vus", "#8e9cff", "VU"]
  ];
  return <div className="perf-series-wrap">
    {lines.map(([label, keys, color, unit]) => <div className="perf-series-card" key={label}>
      <div className="perf-series-title"><strong>{label}</strong><span>{unit}</span></div>
      <Sparkline points={points} keys={keys} color={color} label={`${label}趋势图`} />
      <div className="perf-series-legend">{(Array.isArray(keys) ? keys : [keys]).map((key, i) => <span key={key}><i style={{ background: Array.isArray(keys) ? ["#e8c36a", "#f39b61", "#ff6b8a", "#c278ff"][i] : color }} />{key === "errorRate" ? "错误率" : key.toUpperCase()}</span>)}</div>
    </div>)}
    <Distribution title="HTTP 状态码分布" values={points.at(-1)?.statusCodes || series.statusCodes} empty="暂无状态码统计" />
    <Distribution title="错误 Top N" values={points.at(-1)?.errorTopN || series.errorTopN} empty="暂无错误" />
    <ThresholdDistribution values={points.at(-1)?.thresholds || series.thresholds} />
    <div className="perf-live-messages"><strong>实时消息（脱敏）</strong>{points.slice(-8).reverse().map((point) => point.message ? <div key={point.sequence}>{new Date(point.timestamp).toLocaleTimeString()} · {String(point.message).slice(0, 300)}</div> : null)}</div>
  </div>;
}

function Sparkline({ points, keys, color, label }) {
  const fields = Array.isArray(keys) ? keys : [keys];
  const values = points.flatMap((point) => fields.map((key) => finiteMetric(point[key], key)).filter((value) => value !== null));
  if (!values.length) return <div className="perf-chart-empty">暂无数据</div>;
  const max = Math.max(...values, 1); const min = Math.min(...values, 0); const range = max - min || 1;
  const paths = fields.map((key, index) => points.map((point, i) => { const value = finiteMetric(point[key], key); const x = points.length === 1 ? 50 : (i / (points.length - 1)) * 100; return value === null ? `M${x.toFixed(2)},42` : `${i && finiteMetric(points[i - 1][key], key) !== null ? "L" : "M"}${x.toFixed(2)},${(42 - ((value - min) / range) * 36).toFixed(2)}`; }).join(" "));
  return <svg className="perf-sparkline" viewBox="0 0 100 46" role="img" aria-label={label}><path d="M0,42H100" className="perf-chart-axis" />{paths.map((path, index) => <path key={index} d={path} fill="none" stroke={Array.isArray(keys) ? ["#e8c36a", "#f39b61", "#ff6b8a", "#c278ff"][index] : color} strokeWidth="1.7" vectorEffect="non-scaling-stroke" />)}</svg>;
}

function finiteMetric(value, key) {
  if (value === null || value === undefined || value === "") return null;
  const number = Number(value);
  if (!Number.isFinite(number)) return null;
  return key === "errorRate" ? number * 100 : number;
}

function Distribution({ title, values, empty }) {
  const entries = Array.isArray(values) ? values.map((item) => [item.label || item.message || item.error || "未知", item.count ?? item.value ?? 0]) : Object.entries(values || {});
  return <div className="perf-distribution"><strong>{title}</strong>{entries.length ? entries.slice(0, 8).map(([key, value]) => <div className="perf-dist-row" key={key}><span>{key}</span><b>{value}</b></div>) : <span className="muted-text">{empty}</span>}</div>;
}

function ThresholdDistribution({ values }) {
  const list = Array.isArray(values) ? values : [];
  return <div className="perf-distribution"><strong>实时阈值</strong>{list.length ? list.map((item, index) => <div className="perf-dist-row" key={`${item.metric || "threshold"}-${item.expression || ""}-${index}`}><span>{item.metric || "指标"} {item.expression || ""}</span><b>{item.actual == null ? "--" : item.actual} · {thresholdStatusLabel(item.status)}</b></div>) : <span className="muted-text">暂无阈值状态</span>}</div>;
}

function thresholdStatusLabel(status) {
  return { passed: "通过", failed: "失败", ok: "通过", breach: "失败", running: "评估中" }[status] || status || "未知";
}

function PerfMetric({ label, value }) {
  return (
    <div className="perf-metric-card">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function conclusionFor(status) {
  switch (status) {
    case "completed":
      return { icon: <CheckCircle2 size={26} />, title: "通过", tone: "success", desc: "压测正常完成，全部阈值通过" };
    case "threshold_failed":
      return { icon: <AlertTriangle size={26} />, title: "性能未达标", tone: "danger", desc: "压测完成但存在未通过阈值，请查看阈值结果" };
    case "execution_failed":
      return { icon: <XCircle size={26} />, title: "执行异常", tone: "danger", desc: "执行链路异常，未得到完整结果" };
    case "timed_out":
      return { icon: <Clock3 size={26} />, title: "超时", tone: "danger", desc: "超过预计完成时间被判定超时" };
    case "canceled":
      return { icon: <XCircle size={26} />, title: "已取消", tone: "neutral", desc: "任务被手动取消，保留部分结果" };
    default:
      return { icon: <Clock3 size={26} />, title: runStatusLabel(status), tone: "warning", desc: "任务仍在执行中" };
  }
}

function formatThresholdValue(value) {
  if (value == null) return "--";
  if (typeof value === "number") return value < 100 ? Number(value.toFixed(2)) : Math.round(value);
  return String(value);
}

function formatGap(item) {
  const actual = Number(item.actual);
  const target = Number(item.target);
  if (Number.isNaN(actual) || Number.isNaN(target)) return "--";
  const gap = actual - target;
  const sign = gap > 0 ? "+" : "";
  return `${sign}${Number(gap.toFixed(2))}`;
}

function formatDuration(run) {
  if (!run.startedAt) return "--";
  const start = new Date(run.startedAt).getTime();
  const end = run.finishedAt ? new Date(run.finishedAt).getTime() : Date.now();
  const duration = Math.max(0, end - start);
  if (duration < 1000) return `${duration} ms`;
  if (duration < 60000) return `${(duration / 1000).toFixed(duration < 10000 ? 1 : 0)} 秒`;
  const minutes = Math.floor(duration / 60000);
  return `${minutes} 分 ${Math.round((duration % 60000) / 1000)} 秒`;
}
