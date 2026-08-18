import { describe, expect, it } from "vitest";
import { isValidPath, pathFromHash, pathToHash } from "./routeState.js";

describe("性能测试独立 URL 路由（SPEC §8.1）", () => {
  it("pathToHash 将性能测试路径映射为英文 slug", () => {
    expect(pathToHash(["性能测试", "压测方案"])).toBe("#/performance/plans");
    expect(pathToHash(["性能测试", "压测方案", "123"])).toBe("#/performance/plans/123");
    expect(pathToHash(["性能测试", "压测方案", "123", "edit"])).toBe("#/performance/plans/123/edit");
    expect(pathToHash(["性能测试", "压测方案", "new"])).toBe("#/performance/plans/new");
    expect(pathToHash(["性能测试", "测试报告"])).toBe("#/performance/runs");
    expect(pathToHash(["性能测试", "测试报告", "456"])).toBe("#/performance/runs/456");
  });

  it("pathFromHash 将英文 slug 还原为内部 activePath", () => {
    expect(pathFromHash("#/performance/plans")).toEqual(["性能测试", "压测方案"]);
    expect(pathFromHash("#/performance/plans/123")).toEqual(["性能测试", "压测方案", "123"]);
    expect(pathFromHash("#/performance/plans/123/edit")).toEqual(["性能测试", "压测方案", "123", "edit"]);
    expect(pathFromHash("#/performance/runs/456")).toEqual(["性能测试", "测试报告", "456"]);
    expect(pathFromHash("#/performance/schedules")).toEqual(["性能测试", "定时规则"]);
  });

  it("路径往返一致（刷新可恢复）", () => {
    const paths = [
      ["性能测试", "压测方案"],
      ["性能测试", "压测方案", "123"],
      ["性能测试", "压测方案", "123", "edit"],
      ["性能测试", "测试报告", "456"],
      ["首页", "项目概览"]
    ];
    paths.forEach((path) => {
      expect(pathFromHash(pathToHash(path))).toEqual(path);
    });
  });

  it("isValidPath 校验深层子路径", () => {
    expect(isValidPath(["性能测试", "压测方案"])).toBe(true);
    expect(isValidPath(["性能测试", "压测方案", "123"])).toBe(true);
    expect(isValidPath(["性能测试", "压测方案", "123", "edit"])).toBe(true);
    expect(isValidPath(["性能测试", "压测方案", "new"])).toBe(true);
    expect(isValidPath(["性能测试", "测试报告", "456"])).toBe(true);
    expect(isValidPath(["性能测试", "测试报告", "456", "edit"])).toBe(false);
    expect(isValidPath(["性能测试", "压测方案", "abc"])).toBe(false);
  });

  it("非法 slug 回退 null", () => {
    expect(pathFromHash("#/performance/unknown/1")).toBeNull();
    expect(pathFromHash("#/performance/plans/abc/edit")).toBeNull();
  });
});
