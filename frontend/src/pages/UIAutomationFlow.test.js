import { describe, expect, it } from "vitest";

import { buildDebugActions } from "./UIAutomationPage.js";

describe("画布调试动作", () => {
  it("生成条件节点的真/假分支", () => {
    const nodes = [
      { id: 1, type: "自定义变量", tag: "set_variable", values: { variable_name: "count", variable_value: "2" } },
      { id: 2, type: "条件判断", tag: "condition", values: { left_value: "${count}", operator: "equals", right_value: "2" } },
      { id: 3, type: "Python代码", tag: "python_code", values: { python_code: "result['ok'] = True" } },
      { id: 4, type: "断言操作", tag: "assert_variable", values: { left_value: "${count}", operator: "notEquals", right_value: "2" } }
    ];
    const connections = [
      { from: 1, to: 2 },
      { from: 2, to: 3, branch: "true" },
      { from: 2, to: 4, branch: "false" }
    ];

    const actions = buildDebugActions(nodes, connections);

    expect(actions[0]).toMatchObject({ action: "setVariable", next: "2" });
    expect(actions[1]).toMatchObject({ action: "condition", trueNext: "3", falseNext: "4" });
    expect(actions[2]).toMatchObject({ action: "pythonCode" });
    expect(actions[3]).toMatchObject({ action: "assertVariable" });
  });

  it("生成扩展元素断言动作", () => {
    const nodes = [
      { id: 1, type: "断言操作", tag: "assert_attribute_contains", values: { locating: "#submit", attribute_name: "class", expected: "active" } },
      { id: 2, type: "断言操作", tag: "assert_count", values: { locating: ".item", expected_count: "3" } },
      { id: 3, type: "断言操作", tag: "assert_regex", values: { left_value: "${order_id}", pattern: "^SN-" } }
    ];

    const actions = buildDebugActions(nodes, [{ from: 1, to: 2 }, { from: 2, to: 3 }]);

    expect(actions[0]).toMatchObject({ action: "assertAttribute", attribute: "class", operator: "contains" });
    expect(actions[1]).toMatchObject({ action: "assertCount", count: "3" });
    expect(actions[2]).toMatchObject({ action: "assertRegex", pattern: "^SN-" });
  });
});
