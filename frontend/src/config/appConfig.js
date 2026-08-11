export const API_BASE = import.meta.env.VITE_API_BASE || "http://127.0.0.1:8080/api";

export const TOKEN_KEY = "synapse_qa_token";

export const menuData = [
  {
    title: "首页",
    children: ["项目概览"]
  },
  {
    title: "界面自动化",
    children: ["页面元素", "页面步骤", "测试用例", "全局变量"]
  },
  {
    title: "接口自动化",
    children: ["接口管理", "测试用例", "全局变量", "请求头管理"]
  },
  {
    title: "测试配置",
    children: ["项目配置", "项目产品", "测试对象", "执行器配置"]
  },
  {
    title: "执行中心",
    children: ["执行记录", "测试报告"]
  },
  {
    title: "AI 智能",
    children: ["AI 助手", "智能断言", "用例生成", "失败分析", "AI 设置"]
  },
  {
    title: "系统管理",
    children: ["系统概览", "用户管理", "角色与权限", "系统参数", "通知配置", "操作日志", "外观设置"]
  }
];
