import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { appendMixedScenarios, buildSmokeRequest, mapApiInterfaceToRequest, mapApiInterfaceToScenario, normalizeSmokeResult, serializeForm, shouldShowSingleInterfacePicker, SmokeResult, validateForm } from "./PerfPlanEditPage.js";

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

  it("映射接口配置并处理 mixed 限额、重名和不支持方法", () => {
    const item = { id: 1, name: "订单", method: "POST", path: "/orders", configuration: { params: '{"page":"1"}', headers: { Authorization: "Bearer visible" }, body: { id: 1 } } };
    expect(mapApiInterfaceToRequest(item)).toMatchObject({ method: "POST", targetUrl: "/orders", body: '{"id":1}' });
    expect(mapApiInterfaceToScenario(item).headers).toEqual({ Authorization: "Bearer visible" });
    const result = appendMixedScenarios([{ name: "订单", weight: 1 }], [item, { ...item, id: 2, method: "GET" }, { id: 3, name: "不支持", method: "HEAD", path: "/head" }], 2);
    expect(result).toHaveLength(2);
    expect(result[1].name).toBe("订单 (2)");
  });

  it("mixed 不显示普通场景单选入口", () => {
    expect(shouldShowSingleInterfacePicker("mixed")).toBe(false);
    expect(shouldShowSingleInterfacePicker("baseline")).toBe(true);
  });

  it("标准化结构化结果并保留 0 和空字符串", () => {
    expect(normalizeSmokeResult({ status: "success", result: { statusCode: 200, durationMs: 0, body: "", bodySize: 0 } })).toMatchObject({ statusCode: 200, durationMs: 0, body: "", bodySize: 0, hasBody: false });
  });

  it("兼容 JSON output、普通文本 output、旧字段和错误元数据", () => {
    expect(normalizeSmokeResult({ status: "success", result: { output: '{"body":"ok","headers":{"x-id":"1"},"isBinary":true}' } })).toMatchObject({ body: "ok", isBinary: true });
    expect(normalizeSmokeResult({ status: "failed", result: { output: "plain text", errorMessage: "请求失败" } })).toMatchObject({ body: "plain text", error: "请求失败" });
    expect(normalizeSmokeResult({ status: "failed", responseBody: "legacy", errorMessage: "顶层失败" })).toMatchObject({ body: "legacy", error: "顶层失败" });
  });

  it("展示错误、截断/二进制提示和格式化响应体", () => {
    render(<SmokeResult result={{ status: "failed", result: { statusCode: 500, durationMs: 12, contentType: "application/json", bodySize: 18, body: '{"ok":false}', headers: { "x-request-id": "1" }, error: "服务异常", truncated: true, isBinary: true } }} />);
    expect(screen.getByText("HTTP 500")).toBeInTheDocument();
    expect(screen.getByText("服务异常", { exact: false })).toBeInTheDocument();
    expect(screen.getByText("响应体已截断，仅展示已接收内容。")).toBeInTheDocument();
    expect(screen.getByText("响应体为二进制内容，不进行文本格式化。")).toBeInTheDocument();
    expect(screen.getByText(/"ok": false/)).toBeInTheDocument();
  });
});
