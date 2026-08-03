import { DashboardPage } from "../pages/DashboardPage.js";
import { SystemPage } from "../pages/SystemPage.js";
import { ConfigPage } from "../pages/ConfigPage.js";
import { UIAutomationPage } from "../pages/UIAutomationPage.js";
import { ExecutionPage } from "../pages/ExecutionPage.js";
import { APIAutomationPage } from "../pages/APIAutomationPage.js";
import { DataFactoryPage } from "../pages/DataFactoryPage.js";
import { AIPage } from "../pages/AIPage.js";

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
  },
  {
    match: (path) => path[0] === "接口自动化",
    Component: APIAutomationPage
  },
  {
    match: (path) => path[0] === "数据工厂",
    Component: DataFactoryPage
  },
  {
    match: (path) => path[0] === "AI 智能",
    Component: AIPage
  },
  {
    match: (path) => path[0] === "执行中心",
    Component: ExecutionPage
  }
];
