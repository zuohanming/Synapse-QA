import { AlertTriangle, Bell, CheckCheck, CircleCheck, Info, Settings, Trash2, X } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { API_BASE, TOKEN_KEY } from "../config/appConfig.js";
import { notificationService } from "../services/notificationService.js";

const categories = [["", "全部"], ["unread", "未读"], ["execution", "执行任务"], ["executor", "执行器"], ["system", "系统"]];

export function NotificationCenter() {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState([]);
  const [count, setCount] = useState(0);
  const [category, setCategory] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [preferences, setPreferences] = useState(null);

  const reload = useCallback(async () => {
    const [list, unread] = await Promise.all([
      notificationService.list({ page: 1, pageSize: 50, unread: category === "unread" ? true : undefined, category: category === "unread" ? "" : category }),
      notificationService.unreadCount()
    ]);
    setItems(list.items || []);
    setCount(unread.count || 0);
  }, [category]);

  useEffect(() => { reload().catch(() => {}); }, [reload]);
  useEffect(() => {
    const controller = new AbortController();
    let retry;
    async function connect() {
      try {
        const response = await fetch(`${API_BASE}/notifications/stream`, { headers: { Authorization: `Bearer ${localStorage.getItem(TOKEN_KEY) || ""}` }, signal: controller.signal });
        if (!response.ok || !response.body) return;
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          if (buffer.includes("\n\n")) { buffer = buffer.slice(buffer.lastIndexOf("\n\n") + 2); reload().catch(() => {}); }
        }
      } catch { if (!controller.signal.aborted) retry = window.setTimeout(connect, 3000); }
    }
    connect();
    return () => { controller.abort(); window.clearTimeout(retry); };
  }, [reload]);

  async function openPreferences() { setSettingsOpen(true); setPreferences(await notificationService.preferences()); }
  async function togglePreference(key) { const next = { ...preferences, [key]: !preferences[key] }; setPreferences(next); await notificationService.updatePreferences(next); }
  async function markAll() { await notificationService.markAllRead(); await reload(); }
  async function removeItem(event, id) { event.stopPropagation(); await notificationService.remove(id); await reload(); }
  async function selectItem(item) { if (!item.isRead) await notificationService.markRead(item.id); if (item.targetUrl) window.location.hash = item.targetUrl.replace(/^#/, ""); setOpen(false); await reload(); }

  return <>
    <button className="topbar-icon notification-button" aria-label="通知" onClick={() => setOpen(true)} type="button"><Bell size={18} />{count ? <span className="notification-count">{count > 99 ? "99+" : count}</span> : null}</button>
    {open ? <><button aria-label="关闭通知" className="notification-drawer-backdrop" onClick={() => setOpen(false)} type="button" /><aside aria-label="通知中心" className="notification-drawer">
      <div className="notification-header"><div><strong>通知</strong><span>{count} 条未读</span></div><div><button aria-label="通知设置" onClick={openPreferences} type="button"><Settings size={17} /></button><button aria-label="关闭" onClick={() => setOpen(false)} type="button"><X size={18} /></button></div></div>
      {settingsOpen ? <NotificationPreferences preferences={preferences} onBack={() => setSettingsOpen(false)} onToggle={togglePreference} /> : <>
        <div className="notification-toolbar"><div>{categories.map(([value, label]) => <button className={category === value ? "active" : ""} key={label} onClick={() => setCategory(value)} type="button">{label}</button>)}</div><button disabled={!count} onClick={markAll} type="button"><CheckCheck size={14} />全部已读</button></div>
        <div className="notification-list">{items.length ? items.map((item) => <button className={item.isRead ? "notification-item" : "notification-item unread"} key={item.id} onClick={() => selectItem(item)} type="button"><NotificationIcon level={item.level} /><span className="notification-copy"><strong>{item.title}</strong><span>{item.content}</span><small>{relativeTime(item.createdAt)}</small></span><span className="notification-actions"><i /> <span aria-label="删除通知" onClick={(event) => removeItem(event, item.id)} role="button"><Trash2 size={14} /></span></span></button>) : <div className="notification-empty"><Bell size={25} /><strong>暂无通知</strong><span>新的执行结果和系统事件会显示在这里</span></div>}</div>
      </>}
    </aside></> : null}
  </>;
}

function NotificationIcon({ level }) { const Icon = level === "error" ? AlertTriangle : level === "success" ? CircleCheck : Info; return <span className={`notification-type ${level || "info"}`}><Icon size={17} /></span>; }
function NotificationPreferences({ preferences, onBack, onToggle }) { if (!preferences) return <div className="notification-empty">正在加载设置</div>; const rows=[["executionSuccess","执行成功"],["executionFailure","执行失败"],["executorAlert","执行器告警"],["systemNotice","系统通知"]]; return <div className="notification-preferences"><button onClick={onBack} type="button">← 返回通知</button><strong>通知偏好</strong><span>控制希望在站内接收的通知类型</span>{rows.map(([key,label])=><label key={key}><span>{label}</span><input checked={preferences[key]} onChange={()=>onToggle(key)} type="checkbox" /></label>)}</div>; }
function relativeTime(value) { const seconds=Math.max(0,Math.floor((Date.now()-new Date(value).getTime())/1000));if(seconds<60)return "刚刚";if(seconds<3600)return `${Math.floor(seconds/60)} 分钟前`;if(seconds<86400)return `${Math.floor(seconds/3600)} 小时前`;return `${Math.floor(seconds/86400)} 天前`; }
