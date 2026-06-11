import { ChevronRight, LogOut } from "lucide-react";
import { menuData } from "../config/appConfig.js";
import { useAuth } from "../hooks/useAuth.js";

export function Layout({ activePath, onNavigate, children }) {
  const { user, logout } = useAuth();

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark" />
          <strong>Synapse QA</strong>
        </div>
        <nav className="menu-tree">
          {menuData.map((group) => (
            <div className="menu-group" key={group.title}>
              <button className={activePath[0] === group.title ? "menu-button active" : "menu-button"} onClick={() => onNavigate([group.title, group.children[0]])}>
                {group.title}
              </button>
              <div className="menu-children">
                {group.children.map((child) => (
                  <button className={activePath[1] === child ? "menu-child active" : "menu-child"} key={child} onClick={() => onNavigate([group.title, child])}>
                    {child}
                  </button>
                ))}
              </div>
            </div>
          ))}
        </nav>
      </aside>
      <main className="main">
        <header className="topbar">
          <div className="breadcrumb">
            {activePath[0]} <ChevronRight size={14} /> {activePath[1]}
          </div>
          <div className="topbar-actions">
            <span>{user?.displayName || user?.username}</span>
            <button className="icon-text-button" onClick={logout} type="button">
              <LogOut size={16} />
              退出
            </button>
          </div>
        </header>
        <section className="workspace">{children}</section>
      </main>
    </div>
  );
}
