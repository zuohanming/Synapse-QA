import { useState } from "react";
import { ChevronDown, ChevronRight, LogOut, Home, Monitor, Settings, Users, Layers, FileCode, Play, Zap, Shield, Database, Menu, PanelLeftClose, Bell, CircleHelp, ClipboardCheck, BarChart3 } from "lucide-react";
import { menuData } from "../config/appConfig.js";
import { useAuth } from "../hooks/useAuth.js";

const menuIcons = {
  "首页": <Home size={18} />,
  "界面自动化": <Monitor size={18} />,
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
  "配置管理": <Settings size={16} />,
  "用户管理": <Users size={16} />,
  "角色管理": <Shield size={16} />,
  "操作日志": <FileCode size={16} />
};

export function Layout({ activePath, onNavigate, children }) {
  const { user, logout } = useAuth();
  const [sidebarOpen, setSidebarOpen] = useState(false);

  function navigate(path) {
    onNavigate(path);
    setSidebarOpen(false);
  }

  return (
    <div className={sidebarOpen ? "app-shell sidebar-open" : "app-shell"}>
      <button className="sidebar-backdrop" aria-label="关闭导航" onClick={() => setSidebarOpen(false)} type="button" />
      <aside className="sidebar" aria-label="主导航">
        <div className="brand">
          <div className="brand-mark" aria-hidden="true"><span /></div>
          <div className="brand-copy"><strong>Synapse QA</strong><small>QUALITY CONSOLE</small></div>
        </div>
        <nav className="menu-tree">
          {menuData.map((group) => (
            <div className="menu-group" key={group.title}>
              <button
                className={activePath[0] === group.title ? "menu-button active" : "menu-button"}
                onClick={() => navigate([group.title, group.children[0]])}
              >
                {menuIcons[group.title]}
                <span>{group.title}</span>
                <ChevronDown className="menu-chevron" size={14} />
              </button>
              <div className="menu-children">
                {group.children.map((child) => (
                  <button
                    className={activePath[1] === child ? "menu-child active" : "menu-child"}
                    key={child}
                    onClick={() => navigate([group.title, child])}
                  >
                    {menuIcons[child]}
                    {child}
                  </button>
                ))}
              </div>
            </div>
          ))}
        </nav>
        <div className="sidebar-footer"><PanelLeftClose size={16} /><span>测试工作台</span></div>
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
            <button className="topbar-icon notification-button" aria-label="通知" type="button"><Bell size={18} /><i /></button>
            <div className="user-profile"><span className="user-avatar">{(user?.displayName || user?.username || "A").slice(0, 1).toUpperCase()}</span><span>{user?.displayName || user?.username}</span></div>
            <button className="topbar-icon" aria-label="退出登录" title="退出登录" onClick={logout} type="button"><LogOut size={17} /></button>
          </div>
        </header>
        <section className="workspace">{children}</section>
      </main>
    </div>
  );
}
