import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { DataTable } from "./DataTable.js";

describe("DataTable 容器适配模式", () => {
  afterEach(cleanup);

  it("启用 fitContainer 时使用百分比列宽且不设置表格最小宽度", () => {
    const { container } = render(
      <DataTable
        columns={[
          { key: "name", title: "名称", width: "20%" },
          { key: "description", title: "说明", width: "80%" }
        ]}
        fitContainer
        rows={[{ id: "row-1", name: "接口 A", description: "接口说明" }]}
      />
    );

    const wrapper = container.querySelector(".table-wrap");
    const table = container.querySelector("table");
    const columns = container.querySelectorAll("col");

    expect(wrapper).toHaveClass("table-wrap-fit");
    expect(table).toHaveAttribute("data-fit-container", "true");
    expect(table.style.minWidth).toBe("");
    expect(columns).toHaveLength(2);
    expect(columns[0]).toHaveStyle({ width: "20%" });
    expect(columns[1]).toHaveStyle({ width: "80%" });
  });
});
