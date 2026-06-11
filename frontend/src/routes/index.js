import { DashboardPage } from "../pages/DashboardPage.js";
import { SystemPage } from "../pages/SystemPage.js";
import { ConfigPage } from "../pages/ConfigPage.js";
import { UIAutomationPage } from "../pages/UIAutomationPage.js";

export const routes = [
  {
    match: (path) => path[0] === "首页",
    Component: DashboardPage
  },
  {
    match: (path) => path[0] === "系统管理",
    Component: SystemPage
  },
  {
    match: (path) => path[0] === "测试配置",
    Component: ConfigPage
  },
  {
    match: (path) => path[0] === "界面自动化",
    Component: UIAutomationPage
  }
];
