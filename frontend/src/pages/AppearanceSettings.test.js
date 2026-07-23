import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { APPEARANCE_KEY, applyAppearance, defaultAppearance } from "../utils/appearance.js";
import { SystemPage } from "./SystemPage.js";

describe("外观设置", () => {
  afterEach(() => {
    cleanup();
    applyAppearance(defaultAppearance);
    localStorage.removeItem(APPEARANCE_KEY);
  });

  it("即时切换 Codex 主题、字体和密度并持久化", async () => {
    render(<SystemPage activePath={["系统管理", "外观设置"]} />);

    fireEvent.click(screen.getByRole("button", { name: /Codex 浅色/ }));
    fireEvent.change(screen.getAllByRole("combobox")[0], { target: { value: "yahei" } });
    fireEvent.click(screen.getByRole("button", { name: "紧凑" }));

    expect(document.documentElement.dataset.theme).toBe("codex");
    expect(document.documentElement.dataset.density).toBe("compact");
    expect(document.documentElement.style.getPropertyValue("--app-font")).toContain("Microsoft YaHei UI");
    expect(JSON.parse(localStorage.getItem(APPEARANCE_KEY))).toMatchObject({ theme: "codex", uiFont: "yahei", density: "compact" });
  });

  it("支持微信和 Kimi 外观", () => {
    render(<SystemPage activePath={["系统管理", "外观设置"]} />);

    fireEvent.click(screen.getByRole("button", { name: /微信清新/ }));
    expect(document.documentElement.dataset.theme).toBe("wechat");

    fireEvent.click(screen.getByRole("button", { name: /Kimi 月紫/ }));
    expect(document.documentElement.dataset.theme).toBe("kimi");
    expect(JSON.parse(localStorage.getItem(APPEARANCE_KEY))).toMatchObject({ theme: "kimi" });
  });
});
