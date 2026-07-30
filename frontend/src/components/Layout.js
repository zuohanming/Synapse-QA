import { useState } from "react";
import { Bell, Braces, ChevronDown, ChevronRight, Gauge, LogOut, Home, KeyRound, Monitor, Settings, Users, Layers, FileCode, Play, Zap, Shield, Database, Menu, CircleHelp, ClipboardCheck, BarChart3 } from "lucide-react";
import { menuData } from "../config/appConfig.js";
import { useAuth } from "../hooks/useAuth.js";
import { NotificationCenter } from "./NotificationCenter.js";
import { TableOverflowTooltip } from "./TableOverflowTooltip.js";

const menuIcons = {
  "首页": <Home size={18} />,
  "界面自动化": <Monitor size={18} />,
  "接口自动化": <Braces size={18} />,
  "接口管理": <Gauge size={16} />,
  "请求头管理": <KeyRound size={16} />,
  "测试配置": <Settings size={18} />,
  "系统管理": <Users size={18} />,
  "项目概览": <Layers size={16} />,
  "页面元素": <FileCode size={16} />,
  "页面步骤": <Play size={16} />,
  "测试用例": <FileCode size={16} />,
  "全局变量": <Zap size={16} />,
  "项目配置": <Settings size={16} />,
  "项目产品": <Layers size={16} />,
  "测试对象": <Database size={16} />,
  "执行器配置": <Play size={16} />,
  "执行中心": <ClipboardCheck size={18} />,
  "执行记录": <Play size={16} />,
  "测试报告": <BarChart3 size={16} />,
  "系统概览": <Gauge size={16} />,
  "系统参数": <Settings size={16} />,
  "通知配置": <Bell size={16} />,
  "外观设置": <Settings size={16} />,
  "用户管理": <Users size={16} />,
  "角色与权限": <Shield size={16} />,
  "操作日志": <FileCode size={16} />
};

const groupPermissions = {
  "首页": "menu.home.read",
  "界面自动化": "menu.ui_automation.read",
  "接口自动化": "menu.api_automation.read",
  "测试配置": "menu.test_config.read",
  "执行中心": "menu.execution.read",
  "系统管理": "menu.system.read"
};

const systemChildPermissions = {
  "系统概览": "system.overview.read",
  "系统参数": "system.settings.read",
  "通知配置": "system.settings.read",
  "外观设置": "system.appearance.read",
  "用户管理": "system.user.read",
  "角色与权限": "system.role.read",
  "操作日志": "system.audit.read"
};

export function Layout({ activePath, onNavigate, children }) {
  const { user, logout } = useAuth();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [expandedGroup, setExpandedGroup] = useState("");

  function navigate(path) {
    onNavigate(path);
    setSidebarOpen(false);
  }

  function toggleGroup(title) {
    setExpandedGroup((current) => current === title ? "" : title);
  }
  const granted = new Set(user?.permissions || []);
  const isAdmin = user?.roleCode === "admin";
  const visibleMenus = menuData
    .filter((group) => isAdmin || granted.has(groupPermissions[group.title]))
    .map((group) => ({
      ...group,
      children: group.children.filter((child) => group.title !== "系统管理" || isAdmin || granted.has(systemChildPermissions[child]))
    }))
    .filter((group) => group.children.length > 0);

  return (
    <div className={sidebarOpen ? "app-shell sidebar-open" : "app-shell"}>
      <a className="skip-link" href="#main-workspace">跳到主要内容</a>
      <button className="sidebar-backdrop" aria-label="关闭导航" onClick={() => setSidebarOpen(false)} type="button" />
      <aside className="sidebar" aria-label="主导航">
        <div className="brand">
          <div className="brand-mark" aria-hidden="true"><span /></div>
          <div className="brand-copy"><strong>Synapse QA</strong><small>QUALITY CONSOLE</small></div>
        </div>
        <nav className="menu-tree">
          {visibleMenus.map((group) => {
            const expanded = expandedGroup === group.title;
            return <div className="menu-group" key={group.title}>
              <button
                className={`${activePath[0] === group.title ? "menu-button active" : "menu-button"}${expanded ? " expanded" : ""}`}
                onClick={() => toggleGroup(group.title)}
                aria-expanded={expanded}
              >
                {menuIcons[group.title]}
                <span>{group.title}</span>
                <ChevronDown className="menu-chevron" size={14} />
              </button>
              <div className={`menu-children${expanded ? " open" : ""}`} aria-hidden={!expanded}>
                <div className="menu-children-inner">{group.children.map((child) => (
                  <button
                    className={activePath[1] === child ? "menu-child active" : "menu-child"}
                    key={child}
                    onClick={() => navigate([group.title, child])}
                    tabIndex={expanded ? 0 : -1}
                  >
                    {menuIcons[child]}
                    {child}
                  </button>
                ))}</div>
              </div>
            </div>
          })}
        </nav>
        <div className="sidebar-footer"><Zap size={16} /><span>测试工作台</span></div>
      </aside>
      <main className="main">
        <header className="topbar">
          <div className="topbar-leading">
            <button className="mobile-menu-button" aria-label="打开导航" onClick={() => setSidebarOpen(true)} type="button"><Menu size={19} /></button>
            <div className="breadcrumb">
            {menuIcons[activePath[0]]}
            {activePath[0]} <ChevronRight size={14} /> {activePath[1]}
            </div>
          </div>
          <div className="topbar-actions">
            <button className="topbar-icon" aria-label="帮助" type="button"><CircleHelp size={18} /></button>
            <NotificationCenter />
            <div className="user-profile"><span className="user-avatar">{(user?.displayName || user?.username || "A").slice(0, 1).toUpperCase()}</span><span>{user?.displayName || user?.username}</span></div>
            <button className="topbar-icon" aria-label="退出登录" title="退出登录" onClick={logout} type="button"><LogOut size={17} /></button>
          </div>
        </header>
        <section className="workspace" id="main-workspace" tabIndex={-1}>{children}</section>
      </main>
      <TableOverflowTooltip />
    </div>
  );
}
