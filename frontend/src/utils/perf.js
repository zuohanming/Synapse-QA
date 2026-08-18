// 性能测试模块共享元数据与纯函数。
// 字段命名与 SPEC v2.5 契约一致：scenarioType / loadConfig / environment / thresholds 数组 / planSnapshot 等。

// 平台并发安全上限（前端预览用默认值，实际以平台配置为准）。
export const PLATFORM_VUS_LIMIT = 500;

export function validateVusLimit(value, label = "VU") {
  const number = Number(value);
  if (!Number.isInteger(number) || number <= 0) return `${label} 必须是正整数。`;
  return number > PLATFORM_VUS_LIMIT ? `${label} 不能超过平台上限 ${PLATFORM_VUS_LIMIT}。` : "";
}

// 6 大场景类型（SPEC §2 / §8.2）。
export const SCENARIO_TYPES = [
  {
    key: "baseline",
    label: "基准测试",
    description: "校验脚本与环境可用性，获取裸响应基线",
    purpose: "单用户低并发、短时压测",
    durationHint: "1~2 分钟",
    riskLevel: "低",
    riskTone: "neutral",
    executor: "constant-vus"
  },
  {
    key: "ramp",
    label: "梯度压力测试",
    description: "摸查性能拐点与最大承载能力",
    purpose: "阶梯式加压，逐步逼近上限",
    durationHint: "视阶段数量",
    riskLevel: "中",
    riskTone: "warning",
    executor: "ramping-vus"
  },
  {
    key: "peak",
    label: "峰值负载测试",
    description: "模拟线上峰值，验收生产 SLA",
    purpose: "峰值 1.5x 并发长时间稳压",
    durationHint: "爬坡 + 保持（通常 30 分钟以上）",
    riskLevel: "高",
    riskTone: "danger",
    executor: "ramping-vus"
  },
  {
    key: "stress",
    label: "极限压力测试",
    description: "摸底极限性能瓶颈",
    purpose: "持续加压至暴跌/超时/报错",
    durationHint: "视步长与最大 VU",
    riskLevel: "高",
    riskTone: "danger",
    executor: "ramping-vus"
  },
  {
    key: "soak",
    label: "稳定性/耐久测试",
    description: "校验长时间运行可靠性",
    purpose: "固定中高并发、长时稳压",
    durationHint: "10 分钟以上",
    riskLevel: "中",
    riskTone: "warning",
    executor: "constant-vus"
  },
  {
    key: "mixed",
    label: "混合场景压测",
    description: "贴近真实流量，消除压测失真",
    purpose: "参数随机化 + 思考时间 + 冷热混合",
    durationHint: "视接口组合",
    riskLevel: "中",
    riskTone: "warning",
    executor: "scenarios",
    p1: true
  }
];

// 测试环境（SPEC §3.1：P0 仅作风险标签，多环境切换放 P2）。
export const ENVIRONMENTS = {
  test: { label: "测试", tone: "neutral" },
  staging: { label: "预发", tone: "warning" },
  production: { label: "生产", tone: "danger" }
};

// 方案状态（保留字段）。
export const PLAN_STATUS = {
  draft: { label: "草稿", tone: "neutral" },
  active: { label: "启用", tone: "success" },
  disabled: { label: "停用", tone: "warning" }
};

// 执行记录状态：中间态 + 5 种终态（SPEC §3.3）。
export const RUN_STATUS = {
  pending: { label: "待执行", tone: "neutral", running: true },
  queued: { label: "排队中", tone: "neutral", running: true },
  dispatching: { label: "调度中", tone: "warning", running: true },
  dispatched: { label: "已下发", tone: "warning", running: true },
  running: { label: "执行中", tone: "warning", running: true },
  stopping: { label: "停止中", tone: "warning", running: true },
  completed: { label: "通过", tone: "success", terminal: true },
  threshold_failed: { label: "性能未达标", tone: "danger", terminal: true, failed: true },
  execution_failed: { label: "执行异常", tone: "danger", terminal: true, failed: true },
  timed_out: { label: "超时", tone: "danger", terminal: true, failed: true },
  canceled: { label: "已取消", tone: "neutral", terminal: true }
};

export const RUNNING_STATUSES = ["pending", "queued", "dispatching", "dispatched", "running", "stopping"];

// 失败阶段（SPEC §3.2）。
export const FAILURE_STAGES = {
  dispatch: "调度下发",
  startup: "启动",
  script_generation: "脚本生成",
  k6_runtime: "k6 运行时",
  callback: "回调",
  timeout: "超时",
  executor_offline: "执行器离线",
  cancel: "取消"
};

// 阈值指标选项（SPEC §3.1 平台模型）。
export const THRESHOLD_METRICS = [
  { value: "http_req_duration", label: "HTTP 请求时长", unit: "ms", aggregations: ["avg", "min", "max", "p(50)", "p(90)", "p(95)", "p(99)"] },
  { value: "http_req_failed", label: "HTTP 失败率", unit: "", aggregations: ["rate"] },
  { value: "http_reqs", label: "HTTP 请求数", unit: "", aggregations: ["count", "rate"] },
  { value: "http_req_waiting", label: "等待时间", unit: "ms", aggregations: ["avg", "p(90)", "p(95)", "p(99)"] },
  { value: "http_req_blocked", label: "阻塞时间", unit: "ms", aggregations: ["avg", "p(90)", "p(95)"] },
  { value: "http_req_connecting", label: "建连时间", unit: "ms", aggregations: ["avg", "p(90)", "p(95)"] },
  { value: "http_req_tls_handshaking", label: "TLS 握手", unit: "ms", aggregations: ["avg", "p(90)", "p(95)"] },
  { value: "http_req_receiving", label: "接收时间", unit: "ms", aggregations: ["avg", "p(90)", "p(95)"] },
  { value: "http_req_sending", label: "发送时间", unit: "ms", aggregations: ["avg", "p(90)", "p(95)"] },
  { value: "iterations", label: "迭代次数", unit: "", aggregations: ["count", "rate"] },
  { value: "iteration_duration", label: "迭代时长", unit: "ms", aggregations: ["avg", "p(90)", "p(95)", "p(99)"] }
];

export const THRESHOLD_OPERATORS = [
  { value: "<", label: "< 小于" },
  { value: "<=", label: "≤ 不大于" },
  { value: ">", label: "> 大于" },
  { value: ">=", label: "≥ 不小于" }
];

// 阈值模板（SPEC §8.2）。
export const THRESHOLD_TEMPLATES = {
  loose: {
    label: "宽松",
    thresholds: [
      { metric: "http_req_duration", aggregation: "p(95)", operator: "<", value: 800, unit: "ms" },
      { metric: "http_req_failed", aggregation: "rate", operator: "<", value: 0.05, unit: "" }
    ]
  },
  sla: {
    label: "常规 SLA",
    thresholds: [
      { metric: "http_req_duration", aggregation: "p(95)", operator: "<", value: 500, unit: "ms" },
      { metric: "http_req_duration", aggregation: "p(99)", operator: "<", value: 1000, unit: "ms" },
      { metric: "http_req_failed", aggregation: "rate", operator: "<", value: 0.01, unit: "" }
    ]
  },
  strict: {
    label: "严格",
    thresholds: [
      { metric: "http_req_duration", aggregation: "p(95)", operator: "<", value: 300, unit: "ms" },
      { metric: "http_req_duration", aggregation: "p(99)", operator: "<", value: 600, unit: "ms" },
      { metric: "http_req_failed", aggregation: "rate", operator: "<", value: 0.005, unit: "" }
    ]
  },
  custom: { label: "自定义", thresholds: [] }
};

export function scenarioMeta(key) {
  return SCENARIO_TYPES.find((item) => item.key === key) || null;
}

export function scenarioLabel(key) {
  return scenarioMeta(key)?.label || key || "-";
}

// 各场景默认负载配置（SPEC §2.1）。
export function defaultLoadConfig(scenarioType) {
  switch (scenarioType) {
    case "baseline":
      return { vus: 3, duration: "2m" };
    case "ramp":
      return { stages: [{ duration: "1m", target: 10 }, { duration: "1m", target: 50 }] };
    case "peak":
      return { peakVus: 150, rampDuration: "2m", holdDuration: "30m", rampDownDuration: "2m" };
    case "stress":
      return { startVus: 10, stepVus: 10, stepDuration: "1m", maxVus: 500 };
    case "soak":
      return { vus: 100, duration: "30m" };
    case "mixed":
      return { vus: 10, duration: "10m", thinkTime: "0.5s", scenarios: [] };
    default:
      return { vus: 3, duration: "2m" };
  }
}

// 解析 k6 时长字符串（如 "2m"、"30s"、"1h"、"500ms"）为秒。
export function parseDurationToSeconds(value) {
  if (value == null) return NaN;
  const text = String(value).trim();
  if (!text) return NaN;
  const match = /^(\d+(?:\.\d+)?)\s*(ms|s|m|h)$/.exec(text);
  if (!match) return NaN;
  const num = Number(match[1]);
  const factors = { ms: 0.001, s: 1, m: 60, h: 3600 };
  return num * factors[match[2]];
}

export function isValidK6Duration(value) {
  return Number.isFinite(parseDurationToSeconds(value));
}

export function normalizeHeadersObject(value) {
  if (value === undefined || value === "") return {};
  if (typeof value !== "object" || Array.isArray(value)) return null;
  return Object.fromEntries(Object.entries(value).map(([key, item]) => [String(key), String(item ?? "")]));
}

export function validateMixedConfig(config) {
  const cfg = config || {};
  const vusError = validateVusLimit(cfg.vus, "并发数 VU");
  if (vusError) return vusError;
  if (!isValidK6Duration(cfg.duration)) return "混合场景时长格式不正确。";
  if (!isValidK6Duration(cfg.thinkTime)) return "思考时间格式不正确。";
  const scenarios = Array.isArray(cfg.scenarios) ? cfg.scenarios : [];
  if (scenarios.length < 1 || scenarios.length > 50) return "混合场景接口数量必须为 1~50 条。";
  for (const item of scenarios) {
    if (!String(item?.name || "").trim() || !String(item?.url || "").trim() || !isAllowedRequestUrl(item.url) || !item.method || !Number.isFinite(Number(item.weight)) || Number(item.weight) <= 0) return "混合场景接口的名称、URL、方法和正权重不能为空。";
    if (normalizeHeadersObject(item.headers) === null) return "混合场景接口 Headers 必须是 JSON object。";
    if (item.body != null && typeof item.body !== "string") return "混合场景接口 Body 必须是字符串。";
  }
  if (Math.abs(scenarios.reduce((sum, item) => sum + Number(item.weight), 0) - 1) > 1e-6) return "混合场景接口权重合计必须等于 1。";
  return "";
}

export function validateMixedTarget(config, targetUrl, environmentId = null) {
  const scenarios = Array.isArray(config?.scenarios) ? config.scenarios : [];
  const target = String(targetUrl || "").trim();
  const hasRelative = scenarios.some((item) => !/^https?:\/\//i.test(String(item?.url || "").trim()));
  const hasEnvironment = environmentId !== null && environmentId !== undefined && String(environmentId).trim() !== "";
  const relativeTarget = /^(?:\/|[^\s:/?#]+(?:[/?#].*)?)$/.test(target);
  if (target && !/^https?:\/\//i.test(target) && !(hasEnvironment && relativeTarget)) return "混合场景目标必须是相对路径或合法 HTTP(S) 地址。";
  if (hasRelative && !hasEnvironment && !/^https?:\/\//i.test(target)) return "混合场景包含相对 URL 且未选择环境时，目标 Base URL 必须是合法 HTTP(S) 地址。";
  return "";
}

function isAllowedRequestUrl(value) {
  const url = String(value || "").trim();
  return /^(https?:\/\/|\/)/i.test(url) || /^[^\s:/?#]+(?:[/?#].*)?$/.test(url);
}

export function formatCompareRate(value) {
  if (value == null || !Number.isFinite(Number(value))) return "不可比较";
  return `${(Number(value) * 100).toFixed(2).replace(/\.00$/, "")}%`;
}

export function filterTrendPoints(items) {
  return (Array.isArray(items) ? items : []).filter((item) => item && item.runId != null);
}

export function environmentTargetPreview(environment, targetUrl) {
  const base = environment?.baseUrl || "";
  const target = String(targetUrl || "").trim();
  if (!base) return target || "-";
  try {
    const baseUrl = new URL(`${base.replace(/\/$/, "")}/`);
    if (/^https?:\/\//i.test(target)) {
      const absolute = new URL(target);
      absolute.protocol = baseUrl.protocol;
      absolute.host = baseUrl.host;
      return absolute.toString();
    }
    if (target.startsWith("/")) return new URL(target, baseUrl.origin).toString();
    return new URL(target, baseUrl).toString();
  } catch {
    return target || base;
  }
}

export function isProductionEnvironment(environment) {
  return environment?.deployEnv === "production" || environment?.environment === "production";
}

export function formatDurationSeconds(seconds) {
  if (!Number.isFinite(seconds) || seconds <= 0) return "0 秒";
  if (seconds < 60) return `${Math.round(seconds)} 秒`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分 ${Math.round(seconds % 60)} 秒`;
  return `${Math.floor(seconds / 3600)} 小时 ${Math.floor((seconds % 3600) / 60)} 分`;
}

// 负载预览（SPEC §8.2 右侧实时预览）。
export function computeLoadPreview(scenarioType, loadConfig) {
  const cfg = loadConfig || {};
  const empty = {
    totalDurationSeconds: 0,
    totalDurationText: "0 秒",
    maxVus: 0,
    estimatedMin: 0,
    estimatedMax: 0,
    overSafeLimit: false,
    safeLimitVus: PLATFORM_VUS_LIMIT,
    timeline: [],
    note: ""
  };
  if (!scenarioType) return empty;

  if (scenarioType === "mixed") {
    const scenarios = Array.isArray(cfg.scenarios) ? cfg.scenarios : [];
    const seconds = parseDurationToSeconds(cfg.duration) || 0;
    const vus = Number(cfg.vus || 0);
    const weighted = scenarios.reduce((sum, item) => sum + Math.max(0, Number(item?.weight || 0)), 0);
    const preview = buildPreview(vus, seconds, [{ label: "混合流量", seconds, vus }]);
    return { ...preview, scenarios: scenarios.map((item, index) => ({ name: item.name || `接口 ${index + 1}`, weight: Number(item.weight || 0), color: ["#49d6c8", "#f2b35b", "#8e9cff", "#ff6b8a"][index % 4] })), note: `P1 混合流量按接口权重随机选择${weighted ? `（当前合计 ${(weighted * 100).toFixed(2)}%）` : ""}。` };
  }

  if (scenarioType === "baseline" || scenarioType === "soak") {
    const vus = Number(cfg.vus || 0);
    const seconds = parseDurationToSeconds(cfg.duration) || 0;
    return buildPreview(vus, seconds, [{ label: "固定并发", seconds, vus }]);
  }

  if (scenarioType === "ramp") {
    const stages = Array.isArray(cfg.stages) ? cfg.stages : [];
    const seconds = stages.reduce((sum, s) => sum + (parseDurationToSeconds(s?.duration) || 0), 0);
    const maxVus = stages.reduce((max, s) => Math.max(max, Number(s?.target || 0)), 0);
    const timeline = stages.map((s, index) => ({ label: `阶段 ${index + 1}`, seconds: parseDurationToSeconds(s?.duration) || 0, vus: Number(s?.target || 0) }));
    return buildPreview(maxVus, seconds, timeline);
  }

  if (scenarioType === "peak") {
    const maxVus = Number(cfg.peakVus || 0);
    const ramp = parseDurationToSeconds(cfg.rampDuration) || 0;
    const hold = parseDurationToSeconds(cfg.holdDuration) || 0;
    const down = parseDurationToSeconds(cfg.rampDownDuration) || 0;
    const timeline = [
      { label: "爬坡", seconds: ramp, vus: maxVus },
      { label: "保持", seconds: hold, vus: maxVus },
      { label: "降压", seconds: down, vus: 0 }
    ];
    return buildPreview(maxVus, ramp + hold + down, timeline);
  }

  if (scenarioType === "stress") {
    const startVus = Number(cfg.startVus || 0);
    const stepVus = Number(cfg.stepVus || 0);
    const stepDuration = parseDurationToSeconds(cfg.stepDuration) || 0;
    const maxVus = Number(cfg.maxVus || 0);
    const steps = stepVus > 0 ? Math.max(1, Math.floor((maxVus - startVus) / stepVus) + 1) : 1;
    const seconds = steps * stepDuration;
    const timeline = Array.from({ length: steps }, (_, index) => ({
      label: `第 ${index + 1} 阶`,
      seconds: stepDuration,
      vus: Math.min(maxVus, startVus + index * stepVus)
    }));
    return buildPreview(maxVus, seconds, timeline);
  }

  return empty;
}

function buildPreview(maxVus, totalSeconds, timeline, note) {
  // 预计请求量：假设平均响应时间 200ms（乐观）~ 1s（悲观），每 VU 串行迭代 1~5 req/s。
  const optimistic = Math.round(maxVus * totalSeconds * 5);
  const pessimistic = Math.round(maxVus * totalSeconds * 1);
  return {
    totalDurationSeconds: totalSeconds,
    totalDurationText: formatDurationSeconds(totalSeconds),
    maxVus,
    estimatedMin: pessimistic,
    estimatedMax: optimistic,
    overSafeLimit: maxVus > PLATFORM_VUS_LIMIT,
    safeLimitVus: PLATFORM_VUS_LIMIT,
    timeline,
    note: note || "预计请求量按平均响应时间 200ms~1s 估算，实际取决于被测接口响应速度。"
  };
}

// 阈值表达式渲染（结构化 → "http_req_duration p(95)<500"，k6 原生无空格）。
export function thresholdExpression(t) {
  if (!t?.metric) return "";
  const agg = t.aggregation || "value";
  return `${t.metric} ${agg}${t.operator}${t.value}${t.unit || ""}`;
}

// 结构化阈值数组 → k6 thresholds 对象（SPEC §3.1）。
export function renderK6Thresholds(thresholds) {
  const grouped = {};
  (Array.isArray(thresholds) ? thresholds : []).forEach((t) => {
    if (!t?.metric || !t?.operator || t?.value == null) return;
    const expr = k6ThresholdExpression(t);
    if (!expr) return;
    const item = { threshold: expr, abortOnFail: Boolean(t.abortOnFail) };
    if (t.abortOnFail && t.delayAbortEval) item.delayAbortEval = t.delayAbortEval;
    (grouped[t.metric] = grouped[t.metric] || []).push(item);
  });
  return grouped;
}

export function k6ThresholdExpression(t) {
  if (!t?.aggregation || !t?.operator || t?.value == null) return "";
  return `${t.aggregation}${t.operator}${t.value}`;
}

export function mergeSeries(series, incoming) {
  const base = series && typeof series === "object" ? series : { version: 1, points: [] };
  const points = Array.isArray(base.points) ? base.points : [];
  const additions = Array.isArray(incoming) ? incoming : Array.isArray(incoming?.points) ? incoming.points : incoming ? [incoming] : [];
  const bySequence = new Map(points.filter((point) => point?.sequence != null).map((point) => [point.sequence, point]));
  additions.forEach((point) => {
    if (point?.sequence != null) bySequence.set(point.sequence, point);
  });
  const sorted = [...bySequence.values()].sort((a, b) => a.sequence - b.sequence);
  const nextPoints = sorted.length > 3000 ? [sorted[0], ...sorted.slice(-2999)] : sorted;
  return { ...base, ...(incoming?.version ? incoming : {}), points: nextPoints, lastSequence: nextPoints.length ? nextPoints.at(-1).sequence : (base.lastSequence ?? -1) };
}

export function normalizeSeries(series) {
  return mergeSeries({ version: 1, intervalMs: series?.intervalMs, points: [] }, series || {});
}

// 统一正式 k6 summary；兼容早期把自定义 output 直接保存为 summary 的历史记录。
export function normalizePerfSummary(summary) {
  const source = summary?.summary?.metrics && !summary?.metrics ? summary.summary : summary;
  if (!source || typeof source !== "object") return { metrics: {}, options: {}, state: {} };
  const sourceMetrics = source.metrics || {};
  const hasK6Values = Object.values(sourceMetrics).some((item) => item?.values && typeof item.values === "object");
  if (hasK6Values) return source;

  const metrics = {};
  Object.entries(sourceMetrics).forEach(([metric, item]) => {
    if (!item || typeof item !== "object") return;
    const values = { ...(item.values || {}) };
    if (item.count != null) values.count = item.count;
    if (item.rate != null) values.rate = item.rate;
    if (item.avg_duration_ms != null) values.avg = item.avg_duration_ms;
    if (item.p95_duration_ms != null) values["p(95)"] = item.p95_duration_ms;
    if (item.p99_duration_ms != null) values["p(99)"] = item.p99_duration_ms;
    if (item.error_rate != null) values.rate = item.error_rate;
    metrics[metric] = { ...item, values };
  });
  (Array.isArray(source.thresholds) ? source.thresholds : []).forEach((item) => {
    if (!item?.metric || !item.threshold) return;
    const metric = (metrics[item.metric] = metrics[item.metric] || { values: {} });
    metric.thresholds = { ...(metric.thresholds || {}), [item.threshold]: { ok: Boolean(item.ok) } };
  });
  return { ...source, metrics, options: source.options || {}, state: source.state || {} };
}

// 从 k6 summary 解析阈值结果（SPEC §9 第 2 屏）。
export function extractThresholdResults(summary) {
  const metrics = normalizePerfSummary(summary).metrics || {};
  const results = [];
  Object.entries(metrics).forEach(([metric, item]) => {
    const thresholds = item?.thresholds || {};
    const values = item?.values || {};
    Object.entries(thresholds).forEach(([expression, th]) => {
      const match = /^(.+?)(<=|>=|<|>)(.+)$/.exec(expression);
      const aggregation = match ? match[1] : expression;
      results.push({
        metric,
        aggregation,
        expression,
        operator: match ? match[2] : "",
        target: match ? match[3] : "",
        actual: values[aggregation],
        passed: Boolean(th?.ok)
      });
    });
  });
  return results;
}

// 从 summary 读取指标值（P99 等仅存 summary，SPEC §3.2）。
export function metricValueFromSummary(summary, metric, aggregation) {
  return normalizePerfSummary(summary)?.metrics?.[metric]?.values?.[aggregation];
}

// 报告指标占位：空值显示 "--"，不显示 null/NaN。
export function numOrDash(value) {
  if (value === null || value === undefined || value === "") return "--";
  const num = Number(value);
  return Number.isNaN(num) ? "--" : num;
}

export function renderMs(value) {
  const num = numOrDash(value);
  return num === "--" ? "--" : `${num} ms`;
}

export function renderPercent(value) {
  const num = numOrDash(value);
  return num === "--" ? "--" : `${(Number(num) <= 1 ? Number(num) * 100 : Number(num)).toFixed(2).replace(/\.00$/, "")}%%`.replace("%%", "%");
}

export function renderRps(value) {
  const num = numOrDash(value);
  return num === "--" ? "--" : `${num}`;
}

export function planStatusLabel(status) {
  return PLAN_STATUS[status]?.label || status || "-";
}

export function planStatusTone(status) {
  return PLAN_STATUS[status]?.tone || "neutral";
}

export function runStatusLabel(status) {
  return RUN_STATUS[status]?.label || status || "-";
}

export function runStatusTone(status) {
  return RUN_STATUS[status]?.tone || "neutral";
}

export function runStatusRunning(status) {
  return Boolean(RUN_STATUS[status]?.running);
}

export function environmentLabel(environment) {
  return ENVIRONMENTS[environment]?.label || environment || "-";
}

export function environmentTone(environment) {
  return ENVIRONMENTS[environment]?.tone || "neutral";
}

export function failureStageLabel(stage) {
  return FAILURE_STAGES[stage] || stage || "-";
}

export function priorityTone(priority) {
  return { P0: "danger", P1: "danger", P2: "warning", P3: "neutral" }[priority] || "neutral";
}

// 最终 URL 展示脱敏（SPEC §8.2：敏感值脱敏，变量引用保留）。
export function maskSensitiveUrl(url) {
  if (!url) return "-";
  const sensitiveKey = /token|key|secret|password|passwd|authorization|auth|cookie|signature/i;
  const mask = (text) => text.replace(/([?&])([^=&#]*?)(=)([^&#]*)/g, (match, sep, key, eq, value) => {
    const masked = !/^\{\{.*\}\}$/.test(value) && sensitiveKey.test(key) ? "***" : value;
    return sep + key + eq + masked;
  });
  try {
    const parsed = new URL(url);
    parsed.searchParams.forEach((value, key) => {
      if (!/^\{\{.*\}\}$/.test(value) && sensitiveKey.test(key)) parsed.searchParams.set(key, "***");
    });
    return parsed.toString();
  } catch {
    return mask(url);
  }
}

// 读取方案字段时兼容旧占位字段（loadMode/vus/duration/stages → scenarioType/loadConfig）。
export function normalizeScenarioType(plan) {
  if (plan?.scenarioType) return plan.scenarioType;
  if (plan?.loadMode === "ramping") return "ramp";
  if (plan?.loadMode === "constant") {
    // 旧模型无法区分 baseline/soak，按并发与时长粗略归为 baseline，长时长归 soak。
    const seconds = parseDurationToSeconds(plan?.duration);
    if (Number(plan?.vus) >= 50 || (seconds && seconds >= 600)) return "soak";
    return "baseline";
  }
  return "baseline";
}

export function normalizeLoadConfig(plan) {
  if (plan?.loadConfig && Object.keys(plan.loadConfig).length) return plan.loadConfig;
  const scenarioType = normalizeScenarioType(plan);
  if (scenarioType === "ramp" && Array.isArray(plan?.stages)) return { stages: plan.stages };
  return defaultLoadConfig(scenarioType);
}

export function normalizeThresholds(plan) {
  if (Array.isArray(plan?.thresholds)) return plan.thresholds;
  // 旧模型 thresholds 是 JSON 对象（k6 风格），P0 无法精确还原结构化，返回空数组。
  return [];
}
