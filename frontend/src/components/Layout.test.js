import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("../hooks/useAuth.js", () => ({
  useAuth: () => ({ user: { displayName: "管理员" }, logout: vi.fn() })
}));

import { Layout } from "./Layout.js";

describe("一级和二级菜单折叠", () => {
  afterEach(cleanup);

  it("一级菜单默认收起，点击后展开并允许选择二级菜单", () => {
    const onNavigate = vi.fn();
    const { container } = render(
      <Layout activePath={["首页", "项目概览"]} onNavigate={onNavigate}><div>内容区</div></Layout>
    );
    const homeMenu = screen.getByRole("button", { name: "首页" });
    const children = container.querySelector(".menu-group .menu-children");

    expect(homeMenu).toHaveAttribute("aria-expanded", "false");
    expect(children).not.toHaveClass("open");

    fireEvent.click(homeMenu);
    expect(homeMenu).toHaveAttribute("aria-expanded", "true");
    expect(children).toHaveClass("open");

    fireEvent.click(screen.getByRole("button", { name: "项目概览" }));
    expect(onNavigate).toHaveBeenCalledWith(["首页", "项目概览"]);

    fireEvent.click(homeMenu);
    expect(homeMenu).toHaveAttribute("aria-expanded", "false");
    expect(children).not.toHaveClass("open");
  });
});
