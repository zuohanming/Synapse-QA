import { useState } from "react";
import { ChevronLeft, Play, Trash2 } from "lucide-react";
import { PerfRunConfirmPanel } from "../components/PerfRunConfirmPanel.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { useAuth } from "../hooks/useAuth.js";
import { performanceService } from "../services/performanceService.js";
import { formatTime } from "../utils/formatters.js";
import {
  computeLoadPreview,
  environmentLabel,
  environmentTone,
  maskSensitiveUrl,
  normalizeLoadConfig,
  normalizeScenarioType,
  normalizeThresholds,
  planStatusLabel,
  planStatusTone,
  priorityTone,
  renderK6Thresholds,
  scenarioLabel,
  thresholdExpression
} from "../utils/perf.js";
import { pathToHash } from "../utils/routeState.js";

// 方案详情页（SPEC §8.1：/performance/plans/:id）。
export function PerfPlanDetailPage({ planId }) {
  const { user } = useAuth();
  const granted = new Set(user?.permissions || []);
  const isAdmin = user?.roleCode === "admin";
  const canManage = isAdmin || granted.has("perf.plan.manage");
  const canExecute = isAdmin || granted.has("perf.plan.execute");

  const [confirm, setConfirm] = useState(false);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const { data: plan, loading, error, reload } = useAsyncData(() => performanceService.plans.get(planId), [planId]);

  function go(path) {
    window.location.hash = pathToHash(path);
  }

  async function removePlan() {
    if (!plan) return;
    if (!window.confirm(`确认删除压测方案“${plan.name}”吗？`)) return;
    setBusy(true);
    setNotice("");
    try {
      await performanceService.plans.remove(plan.id);
      go(["性能测试", "压测方案"]);
    } catch (err) {
      setNotice(err.message || "删除失败");
      setBusy(false);
    }
  }

  if (loading || error) {
    return <StateBlock loading={loading} error={error} />;
  }

  const scenarioType = normalizeScenarioType(plan);
  const loadConfig = normalizeLoadConfig(plan);
  const thresholds = normalizeThresholds(plan);
  const preview = computeLoadPreview(scenarioType, loadConfig);

  return (
    <div className="section-stack">
      <div className="perf-edit-header">
        <button className="icon-text-button compact-button" onClick={() => go(["性能测试", "压测方案"])} type="button">
          <ChevronLeft size={15} />返回列表
        </button>
        <div className="perf-edit-title">
          <strong>压测方案 / #{plan.id} / {plan.name}</strong>
          <span>{scenarioLabel(scenarioType)} · {environmentLabel(plan.environment)}</span>
        </div>
        <div className="action-row">
          {canManage ? <button className="icon-text-button compact-button" onClick={() => go(["性能测试", "压测方案", String(plan.id), "edit"])} type="button">编辑</button> : null}
          {canExecute ? <button className="success-button compact-button" onClick={() => setConfirm(true)} type="button"><Play size={14} />触发执行</button> : null}
          {canManage ? <button className="danger-button compact-button" disabled={busy} onClick={removePlan} type="button"><Trash2 size={14} />删除</button> : null}
        </div>
      </div>

      {notice ? <div className="inline-notice">{notice}</div> : null}

      <section className="resource-panel">
        <div className="panel-header"><strong>基本信息</strong></div>
        <div className="detail-grid">
          <span>项目/产品：{plan.productName || "-"}</span>
          <span>测试环境：<span className={`status-pill ${environmentTone(plan.environment)}`}>{environmentLabel(plan.environment)}</span></span>
          <span>场景类型：{scenarioLabel(scenarioType)}</span>
          <span>优先级：<span className={`status-pill ${priorityTone(plan.priority)}`}>{plan.priority}</span></span>
          <span>状态：<span className={`status-pill ${planStatusTone(plan.status)}`}>{planStatusLabel(plan.status)}</span></span>
          <span>负责人：{plan.owner || "-"}</span>
          <span>标签：{plan.tags || "-"}</span>
          <span>创建人：{plan.createdBy || "-"}</span>
          <span>更新时间：{formatTime(plan.updatedAt)}</span>
        </div>
        {plan.description ? <div className="detail-block"><strong>描述</strong><span>{plan.description}</span></div> : null}
      </section>

      <section className="resource-panel">
        <div className="panel-header"><strong>请求配置</strong></div>
        <div className="detail-grid">
          <span>目标接口：<code className="perf-url">{maskSensitiveUrl(plan.targetUrl)}</code></span>
          <span>请求方法：{plan.method || "-"}</span>
        </div>
        <div className="detail-block">
          <strong>请求头</strong>
          <pre className="perf-json">{formatJson(plan.headers)}</pre>
        </div>
        <div className="detail-block">
          <strong>请求体</strong>
          <pre className="perf-json">{plan.body || "暂无"}</pre>
        </div>
      </section>

      <section className="resource-panel">
        <div className="panel-header"><strong>负载配置</strong></div>
        <div className="perf-detail-load">
          <div className="detail-grid perf-detail-load-stats">
            <span>总预计时长：{preview.totalDurationText}</span>
            <span>最大并发：{preview.maxVus > 0 ? `${preview.maxVus} VU` : "--"}</span>
            <span>预计请求量：{preview.estimatedMin > 0 ? `${preview.estimatedMin.toLocaleString()} ~ ${preview.estimatedMax.toLocaleString()}` : "--"}</span>
            <span className={preview.overSafeLimit ? "perf-danger" : ""}>安全限制：{preview.overSafeLimit ? `超过上限 ${preview.safeLimitVus} VU` : `≤ ${preview.safeLimitVus} VU`}</span>
          </div>
          <pre className="perf-json">{JSON.stringify(loadConfig, null, 2)}</pre>
        </div>
      </section>

      <section className="resource-panel">
        <div className="panel-header"><strong>阈值设置</strong></div>
        {thresholds.length ? (
          <div className="perf-threshold-list">
            {thresholds.map((item, index) => (
              <div className="perf-threshold-item" key={index}><code>{thresholdExpression(item)}</code><span>{item.aggregation === "rate" ? "" : item.unit}</span></div>
            ))}
          </div>
        ) : <span className="muted-text">暂未设置阈值</span>}
        <details className="perf-advanced">
          <summary>查看生成的 k6 配置</summary>
          <pre className="perf-json">{JSON.stringify(renderK6Thresholds(thresholds), null, 2)}</pre>
        </details>
      </section>

      {confirm ? <PerfRunConfirmPanel plan={plan} onClose={() => setConfirm(false)} onRun={(runId) => { setConfirm(false); if (runId) go(["性能测试", "测试报告", String(runId)]); }} /> : null}
    </div>
  );
}

function formatJson(value) {
  if (value == null || value === "") return "暂无";
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}
