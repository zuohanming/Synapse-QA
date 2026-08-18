import { describe, expect, it } from "vitest";
import { buildSmokeRequest, serializeForm, validateForm } from "./PerfPlanEditPage.js";

function mixedForm(environmentId, targetUrl, scenarioUrl = "orders") {
  return { productId: "1", name: "混合方案", scenarioType: "mixed", environmentId, targetUrl, loadConfig: { vus: 2, duration: "1m", thinkTime: "1s", scenarios: [{ name: "订单", weight: 1, method: "GET", url: scenarioUrl, headers: {}, body: "" }] }, thresholds: [] };
}

describe("mixed 方案 payload", () => {
  it("保留 targetUrl，不因 mixed 序列化而清空", () => {
    const payload = serializeForm({
      productId: "1", name: "订单混合", scenarioType: "mixed", targetUrl: "https://api.example.com", params: [], method: "GET", headers: [], body: "",
      environmentId: "7", loadConfig: { vus: 10, duration: "1m", thinkTime: "1s", scenarios: [{ name: "订单", weight: 1, method: "GET", url: "/orders", headers: {}, body: "" }] },
      thresholds: [], environment: "test", status: "draft", priority: "P1", owner: "", tags: "", description: ""
    });
    expect(payload.targetUrl).toBe("https://api.example.com");
    expect(payload.environmentId).toBe(7);
  });

  it("冒烟请求明确使用 smoke mode", () => {
    expect(buildSmokeRequest({ targetUrl: "/orders", method: "GET", headers: {}, body: "", executorId: "exec-1" })).toMatchObject({ mode: "smoke", executorId: "exec-1" });
  });

  it("真实表单校验覆盖环境与 mixed target 组合", () => {
    expect(validateForm(mixedForm("7", ""))).toBe("");
    expect(validateForm(mixedForm("7", "api"))).toBe("");
    expect(validateForm(mixedForm("", ""))).toContain("Base URL");
    expect(validateForm(mixedForm("7", "", "ftp://example.com"))).toContain("接口");
  });
});
