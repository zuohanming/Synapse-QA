import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { DataTable } from "./DataTable.js";

describe("DataTable 容器适配模式", () => {
  afterEach(cleanup);

  it("默认模式为百分比列宽使用默认像素最小宽度", () => {
    const { container } = render(
      <DataTable
        columns={[{ key: "name", title: "名称", width: "20%" }]}
        rows={[{ id: "row-1", name: "接口 A" }]}
      />
    );

    const table = container.querySelector("table");
    const column = container.querySelector("col");

    expect(table).toHaveStyle({ minWidth: "160px" });
    expect(column).toHaveStyle({ width: "20%" });
  });

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

  it("operations 列使用操作单元格且不启用文本溢出气泡", () => {
    const { container } = render(
      <DataTable
        columns={[{
          key: "operations",
          title: "操作",
          render: () => <button type="button">编辑</button>
        }]}
        rows={[{ id: "row-1" }]}
      />
    );

    const content = container.querySelector("tbody .table-cell-content");

    expect(content).toHaveClass("table-cell-actions");
    expect(content).not.toHaveAttribute("data-overflow-tooltip");
    expect(content).not.toHaveAttribute("tabindex");
  });
});
