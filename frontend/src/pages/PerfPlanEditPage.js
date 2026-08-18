import { useEffect, useState } from "react";
import { Check, ChevronLeft, Play, Plus, Trash2 } from "lucide-react";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { useAuth } from "../hooks/useAuth.js";
import { configService } from "../services/configService.js";
import { performanceService } from "../services/performanceService.js";
import { pageItems } from "../utils/formatters.js";
import {
  ENVIRONMENTS,
  PLATFORM_VUS_LIMIT,
  SCENARIO_TYPES,
  THRESHOLD_METRICS,
  THRESHOLD_OPERATORS,
  THRESHOLD_TEMPLATES,
  computeLoadPreview,
  defaultLoadConfig,
  maskSensitiveUrl,
  normalizeLoadConfig,
  normalizeScenarioType,
  normalizeThresholds,
  parseDurationToSeconds,
  isValidK6Duration,
  normalizeHeadersObject,
  validateMixedConfig,
  validateMixedTarget,
  validateVusLimit,
  renderK6Thresholds,
  thresholdExpression
} from "../utils/perf.js";
import { pathToHash } from "../utils/routeState.js";

// 方案编辑页（SPEC §8.2）：基本信息 / 请求配置 / 负载配置 / 阈值设置四区块。
export function PerfPlanEditPage({ planId }) {
  const { user } = useAuth();
  const granted = new Set(user?.permissions || []);
  const isAdmin = user?.roleCode === "admin";
  const canManage = isAdmin || granted.has("perf.plan.manage");

  const [form, setForm] = useState(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const { data: planData, loading, error: loadError } = useAsyncData(
    () => (planId ? performanceService.plans.get(planId) : Promise.resolve(null)),
    [planId]
  );
  const { data: productsData, loading: loadingProducts } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const productOptions = pageItems(productsData);
  const { data: environmentsData, loading: loadingEnvironments } = useAsyncData(
    () => (form?.productId ? performanceService.environments(form.productId) : Promise.resolve({ items: [] })),
    [form?.productId]
  );
  const environmentOptions = Array.isArray(environmentsData) ? environmentsData : (environmentsData?.items || []);

  // 首次加载完成后初始化表单（planId 为空时为新建空表单）。
  useEffect(() => {
    if (form !== null || loading || loadError) return;
    setForm(planId ? planToForm(planData) : emptyForm());
  }, [form, loading, loadError, planId, planData]);

  if (!canManage) {
    return <StateBlock loading={false} error="无权限：需要 perf.plan.manage 权限才能编辑压测方案。" />;
  }
  if (loadError) {
    return <StateBlock loading={false} error={loadError} />;
  }
  if (loading || form === null) {
    return <StateBlock loading />;
  }

  const current = form;

  function update(partial) {
    setForm((prev) => ({ ...prev, ...partial }));
  }

  function updateProduct(productId) {
    update({ productId, environmentId: "", environmentName: "", environmentBaseUrl: "", environmentDeployEnv: "" });
  }

  function updateLoadConfig(next) {
    update({ loadConfig: next });
  }

  function changeScenarioType(nextType) {
    if (nextType === current.scenarioType) return;
    if (!window.confirm(`切换场景类型将清除当前负载配置并重置为「${scenarioLabelFor(nextType)}」默认值，是否继续？`)) return;
    update({ scenarioType: nextType, loadConfig: defaultLoadConfig(nextType) });
  }

  async function handleSave() {
    setError("");
    const validationError = validateForm(current);
    if (validationError) {
      setError(validationError);
      return;
    }
    setBusy(true);
    try {
      const payload = serializeForm(current);
      if (planId) {
        await performanceService.plans.update(planId, payload);
        setNotice("压测方案已更新。");
      } else {
        const created = await performanceService.plans.create(payload);
        if (created?.id) {
          window.location.hash = pathToHash(["性能测试", "压测方案", String(created.id)]);
        }
      }
    } catch (err) {
      setError(err.message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="section-stack perf-edit-page">
      <div className="perf-edit-header">
        <button className="icon-text-button compact-button" onClick={() => { window.location.hash = pathToHash(["性能测试", "压测方案"]); }} type="button">
          <ChevronLeft size={15} />返回列表
        </button>
        <div className="perf-edit-title">
          <strong>{planId ? `编辑压测方案 / #${planId}` : "新增压测方案"}</strong>
          <span>按 6 大场景配置负载与阈值，普通用户无需手写 JSON</span>
        </div>
        <div className="action-row">
          <button className="primary-button compact-button" disabled={busy} onClick={handleSave} type="button">{busy ? "保存中" : "保存方案"}</button>
        </div>
      </div>

      {notice ? <div className="inline-notice">{notice}</div> : null}

      <section className="resource-panel">
        <div className="panel-header"><strong>基本信息</strong></div>
        <div className="modal-grid perf-section-grid">
          <label className="form-field required-field">
            <span>方案名称</span>
            <input className="text-input" value={current.name} onChange={(event) => update({ name: event.target.value })} placeholder="例如：登录接口峰值压测" />
          </label>
          <label className="form-field required-field">
            <span>项目/产品</span>
            <select className="text-input" disabled={loadingProducts} value={current.productId} onChange={(event) => updateProduct(event.target.value)}>
              <option value="">{loadingProducts ? "加载产品中" : "请选择产品"}</option>
              {productOptions.map((item) => <option key={item.id} value={item.id}>{item.projectName}/{item.name}</option>)}
            </select>
          </label>
          <label className="form-field">
            <span>测试环境</span>
            <select className="text-input" value={current.environment} onChange={(event) => update({ environment: event.target.value })}>
              {Object.entries(ENVIRONMENTS).map(([key, item]) => <option key={key} value={key}>{item.label}</option>)}
            </select>
          </label>
          <label className="form-field">
            <span>运行环境</span>
            <select className="text-input" disabled={!current.productId || loadingEnvironments} value={current.environmentId || ""} onChange={(event) => { const selected = environmentOptions.find((item) => String(item.environmentId) === event.target.value); update({ environmentId: event.target.value, environmentName: selected?.envName || "", environmentBaseUrl: selected?.baseUrl || "", environmentDeployEnv: selected?.deployEnv || "" }); }}>
              <option value="">{loadingEnvironments ? "加载环境中" : "跟随旧环境配置"}</option>
              {environmentOptions.map((item) => <option key={item.environmentId} value={item.environmentId}>{item.envName} · {maskSensitiveUrl(item.baseUrl || "-")}</option>)}
            </select>
          </label>
          <label className="form-field">
            <span>负责人</span>
            <input className="text-input" value={current.owner} onChange={(event) => update({ owner: event.target.value })} placeholder="请输入负责人" />
          </label>
          <label className="form-field">
            <span>标签</span>
            <input className="text-input" value={current.tags} onChange={(event) => update({ tags: event.target.value })} placeholder="smoke,login" />
          </label>
          <label className="form-field">
            <span>优先级</span>
            <select className="text-input" value={current.priority} onChange={(event) => update({ priority: event.target.value })}>
              <option value="P0">P0</option>
              <option value="P1">P1</option>
              <option value="P2">P2</option>
              <option value="P3">P3</option>
            </select>
          </label>
          <label className="form-field field-span-2">
            <span>描述</span>
            <textarea className="text-area" rows="2" value={current.description} onChange={(event) => update({ description: event.target.value })} />
          </label>
          <div className="form-field field-span-2">
            <span>场景类型</span>
            <ScenarioCards value={current.scenarioType} onChange={changeScenarioType} />
          </div>
        </div>
      </section>

      <RequestConfigSection form={current} update={update} />

      <LoadConfigSection scenarioType={current.scenarioType} loadConfig={current.loadConfig} onChange={updateLoadConfig} />

      <ThresholdSection thresholds={current.thresholds} onChange={(next) => update({ thresholds: next })} />

      {error ? <div className="form-error perf-edit-error">{error}</div> : null}
    </div>
  );
}

function emptyForm() {
  return {
    productId: "",
    environmentId: "",
    environmentName: "",
    environmentBaseUrl: "",
    environmentDeployEnv: "",
    name: "",
    environment: "test",
    scenarioType: "baseline",
    owner: "",
    tags: "",
    description: "",
    targetUrl: "",
    method: "GET",
    params: [],
    headers: [],
    body: "",
    loadConfig: defaultLoadConfig("baseline"),
    thresholds: [],
    status: "draft",
    priority: "P2"
  };
}

function planToForm(plan) {
  const scenarioType = normalizeScenarioType(plan);
  return {
    productId: plan.productId ? String(plan.productId) : "",
    environmentId: plan.environmentId ? String(plan.environmentId) : "",
    environmentName: plan.environmentName || "",
    environmentBaseUrl: plan.environmentBaseUrl || "",
    environmentDeployEnv: plan.environmentDeployEnv || plan.deployEnv || "",
    name: plan.name || "",
    environment: plan.environment || "test",
    scenarioType,
    owner: plan.owner || "",
    tags: plan.tags || "",
    description: plan.description || "",
    targetUrl: plan.targetUrl || "",
    method: plan.method || "GET",
    params: parseParams(plan.targetUrl),
    headers: objectToKv(plan.headers),
    body: plan.body || "",
    loadConfig: normalizeLoadConfig(plan),
    thresholds: normalizeThresholds(plan).map((item) => ({ abortOnFail: false, delayAbortEval: "", ...item })),
    status: plan.status || "draft",
    priority: plan.priority || "P2"
  };
}

export function serializeForm(form) {
  return {
    productId: Number(form.productId),
    name: form.name.trim(),
    targetUrl: form.scenarioType === "mixed" ? form.targetUrl.trim() : buildFinalUrl(form.targetUrl, form.params),
    method: form.method,
    headers: headersToObject(form.headers),
    body: form.body,
    scenarioType: form.scenarioType,
    loadConfig: form.loadConfig,
    environment: form.environment,
    environmentId: form.environmentId ? Number(form.environmentId) : null,
    thresholds: (form.thresholds || []).map((item) => ({
      metric: item.metric,
      aggregation: item.aggregation,
      operator: item.operator,
      value: Number(item.value),
      unit: item.unit || "",
      abortOnFail: Boolean(item.abortOnFail),
      ...(item.abortOnFail && item.delayAbortEval ? { delayAbortEval: item.delayAbortEval } : {})
    })),
    status: form.status,
    priority: form.priority,
    owner: form.owner.trim(),
    tags: form.tags.trim(),
    description: form.description.trim()
  };
}

export function buildSmokeRequest({ targetUrl, method, headers, body, executorId }) {
  return { mode: "smoke", targetUrl, method, headers, body, executorId: String(executorId) };
}

export function validateForm(form) {
  if (!form.productId) return "请选择项目/产品。";
  if (!form.name.trim()) return "方案名称不能为空。";
  if ([...form.name.trim()].length > 120) return "方案名称不能超过 120 个字符。";
  if (form.scenarioType !== "mixed") {
    if (!form.targetUrl.trim()) return "目标 URL 不能为空。";
    const isAbsoluteTarget = /^https?:\/\//i.test(form.targetUrl.trim());
    const isRelativeTarget = /^(?:\/|[^\s:/?#]+(?:[/?#].*)?)$/.test(form.targetUrl.trim());
    if (!isAbsoluteTarget && !(form.environmentId && isRelativeTarget)) return "目标 URL 必须是合法 HTTP(S) 地址，或在选择环境后填写相对路径。";
    const bodyError = validateBody(form.body);
    if (bodyError) return bodyError;
  }
  const loadError = validateLoadConfig(form.scenarioType, form.loadConfig);
  if (loadError) return loadError;
  if (form.scenarioType === "mixed") {
    const targetError = validateMixedTarget(form.loadConfig, form.targetUrl, form.environmentId);
    if (targetError) return targetError;
  }
  return validateThresholds(form.thresholds);
}

function validateBody(body) {
  const text = String(body || "").trim();
  if (!text) return "";
  try {
    JSON.parse(text);
    return "";
  } catch {
    return "请求体不是有效的 JSON，请修正后再保存。";
  }
}

function validateLoadConfig(scenarioType, cfg) {
  const cfg2 = cfg || {};
  const num = (v) => Number(v);
  switch (scenarioType) {
    case "baseline":
    case "soak":
      if (validateVusLimit(cfg2.vus, "并发数 VU")) return validateVusLimit(cfg2.vus, "并发数 VU");
      if (Number.isNaN(parseDurationToSeconds(cfg2.duration))) return "时长格式不正确（如 2m、30s、1h）。";
      break;
    case "ramp": {
      const stages = Array.isArray(cfg2.stages) ? cfg2.stages : [];
      if (!stages.length) return "爬坡模式至少需要配置一个阶段。";
      for (let index = 0; index < stages.length; index += 1) {
        const stage = stages[index] || {};
        if (Number.isNaN(parseDurationToSeconds(stage.duration))) return `第 ${index + 1} 个阶段时长格式不正确。`;
        if (!Number.isInteger(num(stage.target)) || num(stage.target) <= 0) return `第 ${index + 1} 个阶段目标并发不合法。`;
        if (num(stage.target) > PLATFORM_VUS_LIMIT) return `第 ${index + 1} 个阶段目标并发不能超过 ${PLATFORM_VUS_LIMIT}。`;
        if (index > 0 && num(stage.target) < num(stages[index - 1].target)) return "阶段目标并发应按顺序递增。";
      }
      break;
    }
    case "peak":
      if (validateVusLimit(cfg2.peakVus, "峰值并发 VU")) return validateVusLimit(cfg2.peakVus, "峰值并发 VU");
      if (["rampDuration", "holdDuration", "rampDownDuration"].some((key) => Number.isNaN(parseDurationToSeconds(cfg2[key])))) return "爬坡/保持/降压时长格式不正确。";
      break;
    case "stress":
      if (!Number.isInteger(num(cfg2.startVus)) || num(cfg2.startVus) < 0) return "起始 VU 不合法。";
      if (!Number.isInteger(num(cfg2.stepVus)) || num(cfg2.stepVus) <= 0) return "步长 VU 不合法。";
      if (Number.isNaN(parseDurationToSeconds(cfg2.stepDuration))) return "每阶时长格式不正确。";
      if (!Number.isInteger(num(cfg2.maxVus)) || num(cfg2.maxVus) <= num(cfg2.startVus)) return "最大 VU 必须大于起始 VU。";
      if (num(cfg2.maxVus) > PLATFORM_VUS_LIMIT) return `最大 VU 不能超过 ${PLATFORM_VUS_LIMIT}。`;
      break;
    case "mixed":
      return validateMixedConfig(cfg2);
    default:
      return "未知场景类型。";
  }
  return "";
}

function validateThresholds(list) {
  for (let index = 0; index < list.length; index += 1) {
    const item = list[index];
    if (!item.metric) return `第 ${index + 1} 条阈值请选择指标。`;
    if (!item.aggregation) return `第 ${index + 1} 条阈值请选择聚合方式。`;
    if (!item.operator) return `第 ${index + 1} 条阈值请选择运算符。`;
    if (item.value == null || item.value === "" || Number.isNaN(Number(item.value))) return `第 ${index + 1} 条阈值请填写合法数值。`;
    if (item.abortOnFail && item.delayAbortEval && !isValidK6Duration(item.delayAbortEval)) return `第 ${index + 1} 条阈值的延迟评估时间格式不正确。`;
  }
  return "";
}

function scenarioLabelFor(key) {
  return SCENARIO_TYPES.find((item) => item.key === key)?.label || key;
}

function parseParams(url) {
  try {
    return Array.from(new URL(url).searchParams.entries()).map(([key, value]) => ({ key, value }));
  } catch {
    return [];
  }
}

function objectToKv(obj) {
  if (!obj || typeof obj !== "object" || Array.isArray(obj)) return [];
  return Object.entries(obj).map(([key, value]) => ({ key, value: String(value ?? "") }));
}

function headersToObject(list) {
  const result = {};
  (list || []).forEach((item) => {
    if (item.key && item.key.trim()) result[item.key.trim()] = item.value;
  });
  return result;
}

function buildFinalUrl(targetUrl, params) {
  const list = (params || []).filter((item) => item.key && item.key.trim());
  if (!list.length) return targetUrl.trim();
  try {
    const url = new URL(targetUrl.trim());
    list.forEach((item) => url.searchParams.set(item.key.trim(), item.value));
    return url.toString();
  } catch {
    const sep = targetUrl.includes("?") ? "&" : "?";
    return `${targetUrl.trim()}${sep}${list.map((item) => `${encodeURIComponent(item.key.trim())}=${encodeURIComponent(item.value)}`).join("&")}`;
  }
}

// 场景卡片（SPEC §8.2：卡片展示适用场景、预计时长、风险等级）。
function ScenarioCards({ value, onChange }) {
  return (
    <div className="perf-scenario-grid">
      {SCENARIO_TYPES.map((item) => (
        <button type="button" key={item.key} className={`perf-scenario-card${value === item.key ? " active" : ""}`} onClick={() => onChange(item.key)}>
          <div className="perf-scenario-card-head">
            <strong>{item.label}</strong>
            <span className={`status-pill ${item.riskTone}`}>{item.riskLevel}风险</span>
            {item.p1 ? <span className="perf-p1-tag">P1</span> : null}
          </div>
          <p>{item.description}</p>
          <small>{item.purpose} · 预计 {item.durationHint}</small>
        </button>
      ))}
    </div>
  );
}

// 请求配置区块（URL/Method + Params/Headers/Body Tab + 冒烟 + 最终 URL 脱敏）。
function RequestConfigSection({ form, update }) {
  const [tab, setTab] = useState("headers");
  const [advanced, setAdvanced] = useState(false);
  const [smoking, setSmoking] = useState(false);
  const [smokeResult, setSmokeResult] = useState(null);
  const [smokeExecutor, setSmokeExecutor] = useState("");

  const { data: executorsData, loading: loadingExecutors } = useAsyncData(() => performanceService.executors(), []);
  const executors = Array.isArray(executorsData) ? executorsData.filter((item) => item.status === "online") : [];

  const finalUrl = buildFinalUrl(form.targetUrl, form.params);

  function formatBody() {
    const text = String(form.body || "").trim();
    if (!text) return;
    try {
      update({ body: JSON.stringify(JSON.parse(text), null, 2) });
    } catch {
      // 非法 JSON 保持原样，由保存校验提示。
    }
  }

  async function runSmoke() {
    const executorId = smokeExecutor || (executors[0]?.executorId || executors[0]?.id);
    if (!executorId) {
      setSmokeResult({ status: "failed", result: { error: "暂无可用的在线执行器" } });
      return;
    }
    setSmoking(true);
    setSmokeResult(null);
    try {
      const task = await performanceService.smoke.start(buildSmokeRequest({
        targetUrl: finalUrl,
        method: form.method,
        headers: headersToObject(form.headers),
        body: form.body,
        executorId: String(executorId)
      }));
      const taskId = task?.taskId || task?.id;
      if (!taskId) throw new Error("冒烟请求未返回任务 ID");
      const deadline = Date.now() + 30000;
      let result = task;
      while (Date.now() < deadline) {
        result = await performanceService.smoke.status(taskId);
        setSmokeResult(result);
        if (["success", "failed", "canceled"].includes(result?.status)) break;
        await new Promise((resolve) => window.setTimeout(resolve, 500));
      }
      if (!["success", "failed", "canceled"].includes(result?.status)) {
        try { await performanceService.smoke.cancel(taskId); } catch { /* 取消接口不可用时，明确提示任务可能继续 */ }
        setSmokeResult({ status: "timeout", result: { error: "冒烟请求超时，已请求取消；任务可能仍在继续。" } });
      }
    } catch (err) {
      setSmokeResult({ status: "failed", result: { error: err.message || "冒烟请求失败" } });
    } finally {
      setSmoking(false);
    }
  }

  return (
    <section className="resource-panel">
      <div className="panel-header">
        <strong>请求配置</strong>
        <span className="perf-smoke-actions">
          <select className="text-input perf-smoke-executor" disabled={loadingExecutors} value={smokeExecutor} onChange={(event) => setSmokeExecutor(event.target.value)}>
            <option value="">{loadingExecutors ? "加载执行器中" : "选择冒烟执行器"}</option>
            {executors.map((item) => <option key={item.executorId || item.id} value={String(item.executorId || item.id)}>{item.name || item.executorId || item.id}</option>)}
          </select>
          <button className="icon-text-button compact-button" disabled={smoking || !finalUrl} onClick={runSmoke} type="button">
            <Play size={14} />{smoking ? "发送中" : "发送一次测试请求"}
          </button>
        </span>
      </div>

      <div className="perf-request-row">
        <label className="form-field perf-method-field">
          <span>Method</span>
          <select className="text-input" value={form.method} onChange={(event) => update({ method: event.target.value })}>
            {["GET", "POST", "PUT", "DELETE", "PATCH"].map((item) => <option key={item} value={item}>{item}</option>)}
          </select>
        </label>
        <label className="form-field required-field">
          <span>URL</span>
          <input className="text-input" value={form.targetUrl} onChange={(event) => update({ targetUrl: event.target.value, params: parseParams(event.target.value) })} placeholder="https://example.com/api/login" />
        </label>
      </div>

      <div className="perf-final-url">最终 URL：<code>{maskSensitiveUrl(finalUrl)}</code></div>

      <div className="perf-tabs" role="tablist">
        {[["headers", "Headers"], ["params", "Params"], ["body", "Body"]].map(([key, label]) => (
          <button type="button" key={key} className={tab === key ? "active" : ""} onClick={() => setTab(key)}>{label}</button>
        ))}
      </div>

      {tab === "headers" ? <KvEditor rows={form.headers} onChange={(rows) => update({ headers: rows })} keyLabel="Header 名称" valueLabel="值" valuePlaceholder="如 {{secret.api_token}}（敏感值用变量引用）" /> : null}
      {tab === "params" ? <KvEditor rows={form.params} onChange={(rows) => update({ params: rows })} keyLabel="参数名" valueLabel="值" valuePlaceholder="查询参数值" /> : null}
      {tab === "body" ? (
        <div className="perf-body-editor">
          <div className="perf-body-toolbar">
            <span>Body 为 JSON，提交前会校验语法</span>
            <button className="icon-text-button compact-button" onClick={formatBody} type="button">格式化 JSON</button>
          </div>
          <textarea className="text-area perf-code-area" rows="8" value={form.body} onChange={(event) => update({ body: event.target.value })} placeholder='{"username":"admin"}' spellCheck={false} />
        </div>
      ) : null}

      <details className="perf-advanced" open={advanced} onToggle={(event) => setAdvanced(event.target.open)}>
        <summary>高级模式（JSON 编辑）</summary>
        <div className="perf-advanced-grid">
          <label className="form-field">
            <span>Headers JSON</span>
            <textarea className="text-area perf-code-area" rows="4" value={JSON.stringify(headersToObject(form.headers), null, 2)} onChange={(event) => { try { update({ headers: objectToKv(JSON.parse(event.target.value)) }); } catch { /* 忽略中间态 */ } }} spellCheck={false} />
          </label>
          <label className="form-field">
            <span>Body JSON</span>
            <textarea className="text-area perf-code-area" rows="4" value={form.body} onChange={(event) => update({ body: event.target.value })} spellCheck={false} />
          </label>
        </div>
      </details>

      {smokeResult ? <SmokeResult result={smokeResult} /> : null}
    </section>
  );
}

function KvEditor({ rows, onChange, keyLabel, valueLabel, valuePlaceholder }) {
  function updateRow(index, partial) {
    const next = (rows || []).map((item, i) => (i === index ? { ...item, ...partial } : item));
    onChange(next);
  }
  function addRow() {
    onChange([...(rows || []), { key: "", value: "" }]);
  }
  function removeRow(index) {
    onChange((rows || []).filter((_, i) => i !== index));
  }
  return (
    <div className="perf-kv">
      {(rows || []).map((item, index) => (
        <div className="perf-kv-row" key={index}>
          <input className="text-input" value={item.key} onChange={(event) => updateRow(index, { key: event.target.value })} placeholder={keyLabel} />
          <input className="text-input" value={item.value} onChange={(event) => updateRow(index, { value: event.target.value })} placeholder={valuePlaceholder} />
          <button className="icon-text-button compact-button" onClick={() => removeRow(index)} type="button" aria-label="删除"><Trash2 size={14} /></button>
        </div>
      ))}
      <button className="icon-text-button compact-button" onClick={addRow} type="button"><Plus size={14} />添加一行</button>
    </div>
  );
}

function SmokeResult({ result }) {
  const ok = result?.status === "success";
  const timeout = result?.status === "timeout";
  const body = result?.result?.body ?? result?.result?.responseBody ?? result?.result?.error ?? "";
  return (
    <div className={`perf-smoke-result ${ok ? "ok" : "fail"}`}>
      <span className="perf-smoke-status">{ok ? <Check size={14} /> : timeout ? "⚠" : "✕"}{ok ? "冒烟成功" : timeout ? "冒烟超时" : "冒烟失败"}</span>
      {result?.result?.statusCode != null ? <span>状态码 {result.result.statusCode}</span> : null}
      {result?.result?.durationMs != null ? <span>耗时 {result.result.durationMs} ms</span> : null}
      <pre className="perf-json">{body ? String(body).slice(0, 2000) : "无响应体"}</pre>
    </div>
  );
}

// 负载配置区块：按场景渲染控件 + 右侧实时预览（SPEC §8.2）。
function LoadConfigSection({ scenarioType, loadConfig, onChange }) {
  const preview = computeLoadPreview(scenarioType, loadConfig);
  const cfg = loadConfig || {};

  function patch(partial) {
    onChange({ ...cfg, ...partial });
  }

  return (
    <section className="resource-panel">
      <div className="panel-header"><strong>负载配置</strong></div>
      <div className="perf-load-layout">
        <div className="perf-load-editor">
          {scenarioType === "baseline" || scenarioType === "soak" ? (
            <div className="perf-load-fields">
              <NumberField label="并发数 VU" value={cfg.vus} onChange={(value) => patch({ vus: Number(value) })} />
              <label className="form-field"><span>持续时间</span><input className="text-input" value={cfg.duration || ""} onChange={(event) => patch({ duration: event.target.value })} placeholder="2m / 30s / 1h" /></label>
            </div>
          ) : null}

          {scenarioType === "ramp" ? <StageTable stages={cfg.stages || []} onChange={(stages) => patch({ stages })} /> : null}

          {scenarioType === "peak" ? (
            <div className="perf-load-fields">
              <NumberField label="峰值 VU" value={cfg.peakVus} onChange={(value) => patch({ peakVus: Number(value) })} />
              <label className="form-field"><span>爬坡时长</span><input className="text-input" value={cfg.rampDuration || ""} onChange={(event) => patch({ rampDuration: event.target.value })} placeholder="2m" /></label>
              <label className="form-field"><span>保持时长</span><input className="text-input" value={cfg.holdDuration || ""} onChange={(event) => patch({ holdDuration: event.target.value })} placeholder="30m" /></label>
              <label className="form-field"><span>降压时长</span><input className="text-input" value={cfg.rampDownDuration || ""} onChange={(event) => patch({ rampDownDuration: event.target.value })} placeholder="2m" /></label>
            </div>
          ) : null}

          {scenarioType === "stress" ? (
            <div className="perf-load-fields">
              <NumberField label="起始 VU" value={cfg.startVus} onChange={(value) => patch({ startVus: Number(value) })} />
              <NumberField label="步长 VU" value={cfg.stepVus} onChange={(value) => patch({ stepVus: Number(value) })} />
              <label className="form-field"><span>每阶时长</span><input className="text-input" value={cfg.stepDuration || ""} onChange={(event) => patch({ stepDuration: event.target.value })} placeholder="1m" /></label>
              <NumberField label="最大 VU" value={cfg.maxVus} onChange={(value) => patch({ maxVus: Number(value) })} />
            </div>
          ) : null}

          {scenarioType === "mixed" ? (
            <MixedEditor config={cfg} onChange={patch} />
          ) : null}
        </div>

        <LoadPreview preview={preview} />
      </div>
    </section>
  );
}

function NumberField({ label, value, onChange }) {
  return (
    <label className="form-field">
      <span>{label}</span>
      <input className="text-input" type="number" min="0" value={value ?? ""} onChange={(event) => onChange(event.target.value)} />
    </label>
  );
}

function StageTable({ stages, onChange }) {
  function updateRow(index, partial) {
    onChange(stages.map((item, i) => (i === index ? { ...item, ...partial } : item)));
  }
  function addRow() {
    const last = stages.length ? stages[stages.length - 1] : { target: 0 };
    onChange([...stages, { duration: "1m", target: Number(last.target || 0) + 10 }]);
  }
  function removeRow(index) {
    onChange(stages.filter((_, i) => i !== index));
  }
  return (
    <div className="perf-stages">
      <div className="perf-stages-head"><span>阶段</span><span>时长</span><span>目标并发</span><span /></div>
      {stages.map((stage, index) => (
        <div className="perf-stages-row" key={index}>
          <span>#{index + 1}</span>
          <input className="text-input" value={stage.duration || ""} onChange={(event) => updateRow(index, { duration: event.target.value })} placeholder="1m" />
          <input className="text-input" type="number" min="1" value={stage.target ?? ""} onChange={(event) => updateRow(index, { target: Number(event.target.value) })} />
          <button className="icon-text-button compact-button" onClick={() => removeRow(index)} type="button" aria-label="删除阶段"><Trash2 size={14} /></button>
        </div>
      ))}
      <button className="icon-text-button compact-button" onClick={addRow} type="button"><Plus size={14} />添加阶段</button>
    </div>
  );
}

function LoadPreview({ preview }) {
  return (
    <aside className="perf-load-preview" aria-label="负载预览">
      <div className="perf-load-preview-title">负载预览</div>
      <dl className="perf-load-preview-stats">
        <div><dt>总预计时长</dt><dd>{preview.totalDurationText}</dd></div>
        <div><dt>最大并发</dt><dd>{preview.maxVus > 0 ? `${preview.maxVus} VU` : "--"}</dd></div>
        <div><dt>预计请求量</dt><dd>{preview.estimatedMin > 0 ? `${preview.estimatedMin.toLocaleString()} ~ ${preview.estimatedMax.toLocaleString()}` : "--"}</dd></div>
        <div className={preview.overSafeLimit ? "danger" : ""}><dt>安全限制</dt><dd>{preview.overSafeLimit ? `超过上限 ${preview.safeLimitVus} VU` : `≤ ${preview.safeLimitVus} VU`}</dd></div>
      </dl>
      {preview.timeline.length ? (
        <div className="perf-timeline">
          {preview.timeline.map((item, index) => (
            <div className="perf-timeline-row" key={index} title={`${item.label} · ${item.vus} VU`}>
              <span className="perf-timeline-label">{item.label}</span>
              <span className="perf-timeline-bar" style={{ width: `${Math.max(4, (item.seconds / Math.max(1, preview.totalDurationSeconds)) * 100)}%`, height: `${Math.max(4, (item.vus / Math.max(1, preview.maxVus)) * 22)}px` }} />
            </div>
          ))}
        </div>
      ) : null}
      {preview.note ? <p className="perf-load-preview-note">{preview.note}</p> : null}
    </aside>
  );
}

function MixedEditor({ config, onChange }) {
  const scenarios = Array.isArray(config.scenarios) ? config.scenarios : [];
  const update = (index, partial) => onChange({ scenarios: scenarios.map((item, i) => i === index ? { ...item, ...partial } : item) });
  const totalWeight = scenarios.reduce((sum, item) => sum + Number(item.weight || 0), 0);
  return <div className="perf-mixed-editor">
    <div className="perf-load-fields">
      <NumberField label="全局 VU" value={config.vus} onChange={(value) => onChange({ vus: Number(value) })} />
      <label className="form-field"><span>持续时间</span><input className="text-input" value={config.duration || ""} onChange={(e) => onChange({ duration: e.target.value })} placeholder="10m" /></label>
      <label className="form-field"><span>思考时间</span><input className="text-input" value={config.thinkTime || ""} onChange={(e) => onChange({ thinkTime: e.target.value })} placeholder="0.5s" /></label>
    </div>
    <div className={`perf-mixed-summary${Math.abs(totalWeight - 1) <= 1e-6 ? " valid" : ""}`}>接口 {scenarios.length}/50 · 权重合计 <strong>{totalWeight.toFixed(6)}</strong>（必须为 1）</div>
    {scenarios.map((item, index) => <div className="perf-mixed-row" key={index}>
      <span className="perf-mixed-index">{index + 1}</span><input className="text-input" aria-label={`混合接口名称 ${index + 1}`} placeholder="接口名称" value={item.name || ""} onChange={(e) => update(index, { name: e.target.value })} />
      <input className="text-input" type="number" min="0.000001" max="1" step="0.000001" aria-label={`混合接口权重 ${index + 1}`} value={item.weight ?? ""} onChange={(e) => update(index, { weight: Number(e.target.value) })} />
      <select className="text-input" value={item.method || "GET"} onChange={(e) => update(index, { method: e.target.value })}>{["GET", "POST", "PUT", "DELETE", "PATCH"].map((m) => <option key={m}>{m}</option>)}</select>
      <input className="text-input perf-mixed-url" aria-label={`混合接口 URL ${index + 1}`} placeholder="https://example.com/api" value={item.url || ""} onChange={(e) => update(index, { url: e.target.value })} />
      <button className="icon-text-button compact-button" type="button" onClick={() => onChange({ scenarios: scenarios.filter((_, i) => i !== index) })} aria-label="删除接口"><Trash2 size={14} /></button>
      <details className="perf-mixed-advanced"><summary>Headers / Body</summary><div className="perf-mixed-advanced-grid"><label className="form-field"><span>Headers JSON</span><textarea className="text-area" rows="2" value={typeof item.headers === "string" ? item.headers : JSON.stringify(item.headers || {}, null, 2)} onChange={(e) => { try { update(index, { headers: normalizeHeadersObject(JSON.parse(e.target.value)) }); } catch { update(index, { headers: e.target.value }); } }} /></label><label className="form-field"><span>Body JSON</span><textarea className="text-area" rows="2" value={item.body || ""} onChange={(e) => update(index, { body: e.target.value })} /></label></div></details>
    </div>)}
    <button className="icon-text-button compact-button" type="button" disabled={scenarios.length >= 50} onClick={() => onChange({ scenarios: [...scenarios, { name: "新接口", weight: 0.1, method: "GET", url: "", headers: {}, body: "" }] })}><Plus size={14} />添加接口</button>
  </div>;
}

// 阈值设置区块（SPEC §8.2：结构化表格 + 模板 + k6 配置预览）。
function ThresholdSection({ thresholds, onChange }) {
  const list = Array.isArray(thresholds) ? thresholds : [];
  const [showK6, setShowK6] = useState(false);

  function updateRow(index, partial) {
    onChange(list.map((item, i) => (i === index ? { ...item, ...partial } : item)));
  }
  function addRow() {
    const first = THRESHOLD_METRICS[0];
    onChange([...list, { metric: first.value, aggregation: first.aggregations[2] || first.aggregations[0], operator: "<", value: 500, unit: first.unit }]);
  }
  function removeRow(index) {
    onChange(list.filter((_, i) => i !== index));
  }
  function applyTemplate(key) {
    const template = THRESHOLD_TEMPLATES[key];
    if (!template) return;
    onChange(template.thresholds.map((item) => ({ ...item })));
  }

  return (
    <section className="resource-panel">
      <div className="panel-header">
        <strong>阈值设置</strong>
        <div className="perf-threshold-templates">
          {Object.entries(THRESHOLD_TEMPLATES).map(([key, item]) => (
            <button type="button" key={key} className="icon-text-button compact-button" onClick={() => applyTemplate(key)}>{item.label}</button>
          ))}
        </div>
      </div>

      <div className="perf-threshold-table">
        <div className="perf-threshold-head"><span>指标</span><span>聚合方式</span><span>运算符</span><span>阈值</span><span>自动停止</span><span /></div>
        {list.map((item, index) => (
          <ThresholdRow key={index} item={item} index={index} onChange={(partial) => updateRow(index, partial)} onRemove={() => removeRow(index)} />
        ))}
        {!list.length ? <div className="perf-threshold-empty">暂未设置阈值，可通过上方模板快速应用，或逐条添加。</div> : null}
        <button className="icon-text-button compact-button" onClick={addRow} type="button"><Plus size={14} />添加阈值</button>
      </div>

      <details className="perf-advanced" open={showK6} onToggle={(event) => setShowK6(event.target.open)}>
        <summary>查看生成的 k6 配置</summary>
        <pre className="perf-json">{JSON.stringify(renderK6Thresholds(list), null, 2)}</pre>
      </details>
      <p className="perf-threshold-note">启用自动停止后，阈值持续失败时将提前结束压测。</p>
    </section>
  );
}

function ThresholdRow({ item, onChange, onRemove }) {
  const metric = THRESHOLD_METRICS.find((m) => m.value === item.metric);
  const aggregations = metric?.aggregations || ["avg", "p(95)", "p(99)", "rate"];
  const expression = thresholdExpression(item);
  return (
    <div className="perf-threshold-row">
      <select className="text-input" value={item.metric || ""} onChange={(event) => {
        const next = THRESHOLD_METRICS.find((m) => m.value === event.target.value);
        onChange({ metric: next.value, aggregation: next.aggregations[2] || next.aggregations[0], unit: next.unit });
      }}>
        {THRESHOLD_METRICS.map((m) => <option key={m.value} value={m.value}>{m.label}</option>)}
      </select>
      <select className="text-input" value={item.aggregation || ""} onChange={(event) => onChange({ aggregation: event.target.value })}>
        {aggregations.map((a) => <option key={a} value={a}>{a}</option>)}
      </select>
      <select className="text-input" value={item.operator || ""} onChange={(event) => onChange({ operator: event.target.value })}>
        {THRESHOLD_OPERATORS.map((o) => <option key={o.value} value={o.value}>{o.value}</option>)}
      </select>
      <div className="perf-threshold-value">
        <input className="text-input" type="number" step="any" value={item.value ?? ""} onChange={(event) => onChange({ value: event.target.value })} />
        {item.unit ? <span>{item.unit}</span> : null}
      </div>
      <label className="perf-abort-toggle"><input type="checkbox" checked={Boolean(item.abortOnFail)} onChange={(event) => onChange({ abortOnFail: event.target.checked, ...(event.target.checked ? {} : { delayAbortEval: "" }) })} />停止</label>
      <div className="perf-threshold-op">
        <code title={expression}>{expression || "-"}</code>
        {item.abortOnFail ? <input className="text-input perf-delay-input" aria-label="延迟评估" placeholder="如 10s" value={item.delayAbortEval || ""} onChange={(event) => onChange({ delayAbortEval: event.target.value })} /> : null}
        <button className="icon-text-button compact-button" onClick={onRemove} type="button" aria-label="删除阈值"><Trash2 size={14} /></button>
      </div>
    </div>
  );
}
