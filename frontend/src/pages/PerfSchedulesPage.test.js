import { describe, expect, it } from "vitest";
import { serializeScheduleCreate, serializeSchedulePatch } from "./PerfSchedulesPage.js";

describe("定时规则 payload", () => {
  it("create/edit 使用 numeric plan/environment ID 并携带 enabled", () => {
    expect(serializeScheduleCreate({ name: "nightly", planId: "12", environmentId: "8", cronExpression: "0 1 * * *", timezone: "Asia/Shanghai", enabled: true })).toEqual({ name: "nightly", planId: 12, environmentId: 8, cronExpression: "0 1 * * *", timezone: "Asia/Shanghai", enabled: true });
    const patch = serializeSchedulePatch({ name: "follow", planId: "12", environmentId: "", cronExpression: "0 1 * * *", timezone: "Asia/Shanghai", enabled: false });
    expect(patch.environmentId).toBe(null);
    expect(patch).not.toHaveProperty("enabled");
  });
});
