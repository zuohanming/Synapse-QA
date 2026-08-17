import { useState } from "react";
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
  scenarioLabel
} from "../utils/perf.js";
import { pathToHash } from "../utils/routeState.js";

// 报告详情页（SPEC §9 四屏：P0 实现结论 / 阈值结果 / 诊断，趋势与分布为 P1 占位）。
export function PerfRunDetailPage({ runId }) {
  const { user } = useAuth();
  const granted = new Set(user?.permissions || []);
  const isAdmin = user?.roleCode === "admin";
  const canExecute = isAdmin || granted.has("perf.plan.execute");

  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const { data: run, loading, error, reload } = useAsyncData(() => performanceService.runs.get(runId), [runId]);

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

  if (loading || error) {
    return <StateBlock loading={loading} error={error} />;
  }

  const conclusion = conclusionFor(run.status);
  const p99 = metricValueFromSummary(run.summary, "http_req_duration", "p(99)");
  const thresholdResults = extractThresholdResults(run.summary);
  const targetUrl = run.planSnapshot?.targetUrl;

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

      {notice ? <div className="inline-notice">{notice}</div> : null}

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

      {/* 第 3 屏：趋势与分布（P1 占位） */}
      <section className="resource-panel">
        <div className="panel-header"><strong>趋势与分布</strong></div>
        <div className="perf-p1-placeholder">趋势曲线 / 延迟直方图 / 状态码与错误分布依赖降采样 series 数据，P1 上线实时监控后开放。</div>
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
