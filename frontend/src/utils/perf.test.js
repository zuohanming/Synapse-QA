import { describe, expect, it } from "vitest";
import {
  computeLoadPreview,
  defaultLoadConfig,
  extractThresholdResults,
  maskSensitiveUrl,
  normalizeLoadConfig,
  normalizeScenarioType,
  normalizeThresholds,
  numOrDash,
  parseDurationToSeconds,
  renderK6Thresholds,
  renderMs,
  renderPercent,
  runStatusLabel,
  runStatusTone,
  thresholdExpression
} from "./perf.js";

describe("parseDurationToSeconds", () => {
  it("解析 ms/s/m/h 单位", () => {
    expect(parseDurationToSeconds("500ms")).toBe(0.5);
    expect(parseDurationToSeconds("30s")).toBe(30);
    expect(parseDurationToSeconds("2m")).toBe(120);
    expect(parseDurationToSeconds("1h")).toBe(3600);
  });

  it("非法值返回 NaN", () => {
    expect(Number.isNaN(parseDurationToSeconds("abc"))).toBe(true);
    expect(Number.isNaN(parseDurationToSeconds(""))).toBe(true);
    expect(Number.isNaN(parseDurationToSeconds(null))).toBe(true);
  });
});

describe("defaultLoadConfig", () => {
  it("各场景返回符合 SPEC §2.1 的结构", () => {
    expect(defaultLoadConfig("baseline")).toEqual({ vus: 3, duration: "2m" });
    expect(defaultLoadConfig("ramp").stages.length).toBeGreaterThan(0);
    expect(defaultLoadConfig("peak")).toHaveProperty("peakVus");
    expect(defaultLoadConfig("stress")).toHaveProperty("maxVus");
    expect(defaultLoadConfig("soak")).toHaveProperty("duration");
    expect(defaultLoadConfig("mixed")).toHaveProperty("scenarios");
  });
});

describe("computeLoadPreview", () => {
  it("baseline 计算总时长与最大并发", () => {
    const preview = computeLoadPreview("baseline", { vus: 3, duration: "2m" });
    expect(preview.maxVus).toBe(3);
    expect(preview.totalDurationSeconds).toBe(120);
  });

  it("ramp 取阶段目标最大值作为最大并发", () => {
    const preview = computeLoadPreview("ramp", { stages: [{ duration: "1m", target: 10 }, { duration: "1m", target: 50 }] });
    expect(preview.maxVus).toBe(50);
    expect(preview.totalDurationSeconds).toBe(120);
    expect(preview.timeline.length).toBe(2);
  });

  it("peak 计算爬坡+保持+降压总时长", () => {
    const preview = computeLoadPreview("peak", { peakVus: 150, rampDuration: "2m", holdDuration: "30m", rampDownDuration: "2m" });
    expect(preview.maxVus).toBe(150);
    expect(preview.totalDurationSeconds).toBe(34 * 60);
  });

  it("超过平台安全上限时标记 overSafeLimit", () => {
    const preview = computeLoadPreview("stress", { startVus: 10, stepVus: 10, stepDuration: "1m", maxVus: 600 });
    expect(preview.overSafeLimit).toBe(true);
  });

  it("mixed 场景返回 P1 占位提示", () => {
    const preview = computeLoadPreview("mixed", {});
    expect(preview.maxVus).toBe(0);
    expect(preview.note).toContain("P1");
  });
});

describe("thresholdExpression 与 renderK6Thresholds", () => {
  it("渲染阈值表达式", () => {
    expect(thresholdExpression({ metric: "http_req_duration", aggregation: "p(95)", operator: "<", value: 500, unit: "ms" })).toBe("http_req_duration p(95)<500ms");
    expect(thresholdExpression({ metric: "http_req_failed", aggregation: "rate", operator: "<", value: 0.01, unit: "" })).toBe("http_req_failed rate<0.01");
  });

  it("分组渲染 k6 thresholds 对象", () => {
    const result = renderK6Thresholds([
      { metric: "http_req_duration", aggregation: "p(95)", operator: "<", value: 500, unit: "ms" },
      { metric: "http_req_failed", aggregation: "rate", operator: "<", value: 0.01, unit: "" }
    ]);
    expect(result.http_req_duration[0].threshold).toBe("http_req_duration p(95)<500ms");
    expect(result.http_req_failed[0].threshold).toBe("http_req_failed rate<0.01");
    expect(result.http_req_duration[0].abortOnFail).toBe(false);
  });
});

describe("extractThresholdResults", () => {
  it("从 k6 summary 解析阈值结果", () => {
    const summary = {
      metrics: {
        http_req_duration: {
          values: { "p(95)": 456.7 },
          thresholds: { "p(95)<500": { ok: true } }
        }
      }
    };
    const results = extractThresholdResults(summary);
    expect(results).toHaveLength(1);
    expect(results[0].expression).toBe("p(95)<500");
    expect(results[0].actual).toBe(456.7);
    expect(results[0].passed).toBe(true);
  });

  it("空 summary 返回空数组", () => {
    expect(extractThresholdResults(null)).toEqual([]);
  });
});

describe("指标占位与脱敏", () => {
  it("numOrDash 空值显示 --", () => {
    expect(numOrDash(null)).toBe("--");
    expect(numOrDash(undefined)).toBe("--");
    expect(numOrDash("")).toBe("--");
    expect(numOrDash(0)).toBe(0);
  });

  it("renderMs / renderPercent 带单位", () => {
    expect(renderMs(null)).toBe("--");
    expect(renderMs(123)).toBe("123 ms");
    expect(renderPercent(null)).toBe("--");
    expect(renderPercent(0.5)).toBe("0.5%");
  });

  it("maskSensitiveUrl 脱敏敏感 query，保留变量引用", () => {
    const masked = maskSensitiveUrl("https://example.com/api?token=abc123&name=zhangsan");
    expect(masked).not.toContain("abc123");
    expect(masked).toContain("name=zhangsan");
    const withRef = maskSensitiveUrl("https://example.com/api?key={{secret.api_token}}");
    expect(withRef).toContain("{{secret.api_token}}");
  });
});

describe("旧字段兼容", () => {
  it("normalizeScenarioType 将旧 loadMode 映射到场景", () => {
    expect(normalizeScenarioType({ scenarioType: "peak" })).toBe("peak");
    expect(normalizeScenarioType({ loadMode: "ramping" })).toBe("ramp");
    expect(normalizeScenarioType({ loadMode: "constant", vus: 3, duration: "2m" })).toBe("baseline");
  });

  it("normalizeLoadConfig 优先使用 loadConfig，旧 stages 回退", () => {
    expect(normalizeLoadConfig({ loadConfig: { vus: 5, duration: "3m" } })).toEqual({ vus: 5, duration: "3m" });
    expect(normalizeLoadConfig({ loadMode: "ramping", stages: [{ duration: "1m", target: 10 }] }).stages).toHaveLength(1);
  });

  it("normalizeThresholds 仅接受数组", () => {
    expect(normalizeThresholds({ thresholds: [{ metric: "http_req_duration" }] })).toHaveLength(1);
    expect(normalizeThresholds({ thresholds: { "p(95)<500": {} } })).toEqual([]);
  });
});

describe("状态映射", () => {
  it("终态失败类型使用 danger 色系", () => {
    expect(runStatusTone("threshold_failed")).toBe("danger");
    expect(runStatusTone("execution_failed")).toBe("danger");
    expect(runStatusTone("timed_out")).toBe("danger");
    expect(runStatusTone("completed")).toBe("success");
    expect(runStatusTone("canceled")).toBe("neutral");
  });

  it("明确展示失败类型", () => {
    expect(runStatusLabel("threshold_failed")).toBe("性能未达标");
    expect(runStatusLabel("execution_failed")).toBe("执行异常");
    expect(runStatusLabel("timed_out")).toBe("超时");
  });
});
