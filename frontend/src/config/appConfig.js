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
    title: "测试配置",
    children: ["项目配置", "项目产品", "测试对象", "执行器配置"]
  },
  {
    title: "执行中心",
    children: ["执行记录", "测试报告"]
  },
  {
    title: "系统管理",
    children: ["配置管理", "用户管理", "角色管理", "操作日志"]
  }
];
