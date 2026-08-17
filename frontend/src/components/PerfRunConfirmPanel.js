import { useEffect, useState } from "react";
import { AlertTriangle, Play, X } from "lucide-react";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { performanceService } from "../services/performanceService.js";
import {
  computeLoadPreview,
  environmentLabel,
  environmentTone,
  maskSensitiveUrl,
  PLATFORM_VUS_LIMIT,
  RUNNING_STATUSES,
  scenarioLabel,
  thresholdExpression
} from "../utils/perf.js";

// 执行确认面板（SPEC §8.3）：触发前二次确认，生产/高负载需输入方案名称。
export function PerfRunConfirmPanel({ plan, onClose, onRun }) {
  const [executor, setExecutor] = useState("");
  const [step, setStep] = useState("confirm");
  const [confirmName, setConfirmName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const preview = computeLoadPreview(plan?.scenarioType, plan?.loadConfig);
  const isProduction = plan?.environment === "production";
  const highLoad = preview.overSafeLimit;
  const needSecondConfirm = isProduction || highLoad;

  const { data: executorsData, loading: loadingExecutors } = useAsyncData(() => performanceService.executors(), []);
  const executors = Array.isArray(executorsData) ? executorsData : [];
  const perfExecutors = executors.filter(
    (item) => item.status === "online" && (!Array.isArray(item.supportedTypes) || item.supportedTypes.includes("perf"))
  );
  const candidateExecutors = perfExecutors.length ? perfExecutors : executors.filter((item) => item.status === "online");

  const { data: runsData } = useAsyncData(
    () => (plan?.id ? performanceService.runs.list({ planId: plan.id, page: 1, pageSize: 100 }) : Promise.resolve({ items: [] })),
    [plan?.id]
  );
  const activeRuns = (Array.isArray(runsData?.items) ? runsData.items : []).filter((row) => RUNNING_STATUSES.includes(row.status));

  useEffect(() => {
    if (!executor && candidateExecutors.length) {
      setExecutor(String(candidateExecutors[0].executorId || candidateExecutors[0].id));
    }
  }, [candidateExecutors, executor]);

  function generateIdempotencyKey() {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") return crypto.randomUUID();
    return `perf-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }

  // 幂等键本地保留，页面刷新/超时后复用（SPEC §5.3）。
  function idempotencyStorageKey() {
    return `perf.idem.${plan.id}`;
  }

  function getOrCreateIdempotencyKey() {
    const storageKey = idempotencyStorageKey();
    try {
      const stored = localStorage.getItem(storageKey);
      if (stored) return stored;
    } catch {
      // 忽略本地存储不可用。
    }
    const fresh = generateIdempotencyKey();
    try {
      localStorage.setItem(storageKey, fresh);
    } catch {
      // 忽略本地存储不可用。
    }
    return fresh;
  }

  async function submit() {
    if (needSecondConfirm && step !== "second") {
      setStep("second");
      setError("");
      return;
    }
    if (step === "second" && confirmName.trim() !== String(plan?.name || "").trim()) {
      setError("输入与方案名称不一致，请重新输入以确认执行。");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await performanceService.plans.run(plan.id, getOrCreateIdempotencyKey());
      // 已收到成功响应：清除幂等键，下一次为全新执行。
      try {
        localStorage.removeItem(idempotencyStorageKey());
      } catch {
        // 忽略本地存储不可用。
      }
      onRun(result?.id ?? result?.runId);
    } catch (err) {
      // 失败/超时保留幂等键，刷新后复用原 key 重试（SPEC §5.3）。
      setError(err.status === 409 ? "方案配置已变化或存在冲突，请返回方案页重新发起执行。" : err.message || "触发执行失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-backdrop">
      <section className="modal-card modal-card-small perf-confirm" aria-label="执行确认">
        <div className="modal-header">
          <strong>执行确认</strong>
          <button className="modal-close" onClick={onClose} type="button"><X size={18} /></button>
        </div>

        <div className="perf-confirm-body">
          <div className="perf-confirm-plan">
            <span className="perf-confirm-kicker">PLAN / RUN</span>
            <strong>{plan?.name || "-"}</strong>
            <small>#{plan?.id} · {scenarioLabel(plan?.scenarioType)}</small>
          </div>

          <dl className="perf-confirm-grid">
            <div><dt>目标域名</dt><dd className="perf-url" title={plan?.targetUrl || ""}>{maskSensitiveUrl(plan?.targetUrl)}</dd></div>
            <div>
              <dt>测试环境</dt>
              <dd><span className={`status-pill ${environmentTone(plan?.environment)}`}>{environmentLabel(plan?.environment)}</span></dd>
            </div>
            <div><dt>最大并发</dt><dd>{preview.maxVus > 0 ? `${preview.maxVus} VU` : "--"}</dd></div>
            <div><dt>预计时长</dt><dd>{preview.totalDurationText}</dd></div>
            <div><dt>执行器</dt><dd>
              <select className="text-input" disabled={loadingExecutors} value={executor} onChange={(event) => setExecutor(event.target.value)}>
                <option value="">{loadingExecutors ? "加载执行器中" : "请选择执行器"}</option>
                {candidateExecutors.map((item) => (
                  <option key={item.executorId || item.id} value={String(item.executorId || item.id)}>
                    {item.name || item.executorId || item.id}
                  </option>
                ))}
              </select>
            </dd></div>
          </dl>

          {(plan?.thresholds || []).length ? (
            <div className="perf-confirm-thresholds">
              <span>阈值</span>
              <ul>{(plan.thresholds || []).map((item, index) => <li key={index}><code>{thresholdExpression(item)}</code></li>)}</ul>
            </div>
          ) : null}

          {activeRuns.length ? (
            <div className="perf-confirm-warn"><AlertTriangle size={15} /><span>该方案已有 {activeRuns.length} 个任务执行中，同方案同时仅允许 1 个活动任务。</span></div>
          ) : null}

          {needSecondConfirm ? (
            <div className="perf-confirm-warn perf-confirm-danger">
              <AlertTriangle size={15} />
              <span>{isProduction ? "目标为生产环境" : `最大并发 ${preview.maxVus} 超过平台安全限制（${PLATFORM_VUS_LIMIT}）`}，执行可能影响线上服务，请谨慎确认。</span>
            </div>
          ) : null}

          {step === "second" ? (
            <label className="form-field">
              <span>请输入方案名称以确认执行</span>
              <input className="text-input" value={confirmName} onChange={(event) => setConfirmName(event.target.value)} placeholder={plan?.name || ""} />
            </label>
          ) : null}

          {error ? <div className="form-error">{error}</div> : null}
        </div>

        <div className="modal-actions">
          <button className="icon-text-button compact-button" disabled={busy} onClick={onClose} type="button">取消</button>
          <button className="success-button compact-button" disabled={busy || !executor} onClick={submit} type="button">
            <Play size={14} />{busy ? "触发中" : step === "second" ? "确认并执行" : "确认执行"}
          </button>
        </div>
      </section>
    </div>
  );
}
