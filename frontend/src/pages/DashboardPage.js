import { useEffect, useMemo, useState } from "react";
import { AlertCircle, ArrowRight, CheckCircle2, ClipboardList, FileBarChart, Plus, RefreshCw, Settings2, Timer, XCircle } from "lucide-react";
import { DataTable, TablePanel } from "../components/DataTable.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { useAuth } from "../hooks/useAuth.js";
import { dashboardService } from "../services/dashboardService.js";
import { pathFromHash, pathToHash } from "../utils/routeState.js";

const RANGE_OPTIONS = [["24h", "最近 24 小时"], ["7d", "最近 7 天"], ["30d", "最近 30 天"]];
const TYPE_LABELS = { ui: "UI", api: "API", perf: "性能" };
const STATUS_LABELS = { success: "成功", failed: "失败", running: "运行中", canceled: "已取消", timeout: "超时", unknown: "未知" };

export function DashboardPage() {
  const { user } = useAuth();
  const isAdmin = user?.roleCode === "admin";
  const permissions = new Set(user?.permissions || []);
  const [projectId, setProjectId] = useState("");
  const [range, setRange] = useState("7d");
  const [refreshing, setRefreshing] = useState(false);
  const query = useMemo(() => ({ ...(projectId ? { projectId } : {}), range }), [projectId, range]);
  const { data, loading, error, reload } = useAsyncData(() => dashboardService.overview(query), [projectId, range]);
  const hasRunning = hasRunningExecutions(data);
  const canViewExecutions = isAdmin || permissions.has("menu.execution.read");

  useEffect(() => {
    if (!hasRunning) return undefined;
    const timer = window.setInterval(() => reload({ silent: true }), 30000);
    return () => window.clearInterval(timer);
  }, [hasRunning, reload]);

  async function refresh() {
    setRefreshing(true);
    try { await reload({ silent: true }); } finally { setRefreshing(false); }
  }

  const partialMessage = getPartialMessage(data);
  const projects = data?.projects || [];
  const recent = (data?.executions?.recent || []).map((item) => ({ ...item, dashboardKey: getDashboardItemKey(item) }));
  const counts = data?.executions?.counts || {};
  const total = counts.total;
  const successRate = total > 0 ? `${((Number(counts.success || 0) / total) * 100).toFixed(1)}%` : "—";

  return (
    <div className="dashboard-page">
      <header className="dashboard-page-header"><div><h1>项目概览</h1><p>查看近期测试执行、失败任务和待处理事项</p></div></header>
      <div className="dashboard-toolbar">
        <label className="dashboard-filter"><span>项目</span><select className="text-input" aria-label="项目筛选" value={projectId} onChange={(event) => setProjectId(event.target.value)}><option value="">全部项目</option>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label>
        <label className="dashboard-filter"><span>时间范围</span><select className="text-input" aria-label="时间范围" value={range} onChange={(event) => setRange(event.target.value)}>{RANGE_OPTIONS.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
        <button className="icon-text-button compact-button" disabled={loading || refreshing} onClick={refresh} type="button"><RefreshCw className={refreshing ? "dashboard-spin" : ""} size={14} />{refreshing ? "刷新中" : "刷新"}</button>
        <span className="dashboard-updated">{data?.generatedAt ? `更新于 ${formatRelativeTime(data.generatedAt)}` : "等待数据"}</span>
      </div>
      {loading && !data ? <DashboardSkeleton /> : error && !data ? <StateBlock loading={false} error={error}><span /></StateBlock> : <>
        {partialMessage ? <div className="dashboard-partial" role="status"><AlertCircle size={15} />{partialMessage}<button className="link-button" onClick={refresh} type="button">重新加载</button></div> : null}
        <AttentionSection data={data} canViewExecutions={canViewExecutions} />
        <section className="dashboard-metric-grid" aria-label="执行概览指标">
          <MetricCard icon={CheckCircle2} label="成功率" value={successRate} helper={total > 0 ? `最近 ${rangeLabel(range)} · ${total} 次执行` : "暂无执行数据"} tone="success" interactive={canViewExecutions} onClick={() => navigateTo(["执行中心", "执行记录"])} />
          <MetricCard icon={XCircle} label="失败执行" value={counts.failed ?? "—"} helper={`最近 ${rangeLabel(range)}`} tone="danger" interactive={canViewExecutions} onClick={() => navigateTo(["执行中心", "执行记录"])} />
          <MetricCard icon={Timer} label="运行中" value={counts.running ?? "—"} helper="当前执行任务" tone="running" interactive={canViewExecutions} onClick={() => navigateTo(["执行中心", "执行记录"])} />
        </section>
        <div className="dashboard-main-grid">
          <RecentExecutions rows={recent} loading={false} error={error} onRetry={refresh} canViewExecutions={canViewExecutions} />
          <QuickActions permissions={permissions} isAdmin={isAdmin} />
        </div>
      </>}
    </div>
  );
}

function AttentionSection({ data, canViewExecutions }) {
  const items = (data?.attention?.items || []).slice(0, 3);
  return <section className="dashboard-attention resource-panel"><div className="dashboard-section-heading"><div><strong>待处理事项</strong><span>需要优先关注的测试问题</span></div></div>{items.length ? <div className="attention-list">{items.map((item) => <div className="attention-item" key={getDashboardItemKey(item)}><span className="attention-icon"><XCircle size={15} /></span><div className="attention-copy"><strong>{item.title || "执行失败"}</strong><span>{item.description || `${item.projectName || "未知项目"} · ${formatRelativeTime(item.createdAt)}`}</span></div><button className="link-button" onClick={() => navigateToTarget(item.targetUrl, getExecutionFallbackPath(item, canViewExecutions), canViewExecutions)} type="button">查看</button></div>)}</div> : <div className="dashboard-normal-state"><CheckCircle2 size={16} /><div><strong>当前没有待处理事项</strong><span>最近测试执行状态正常</span></div></div>}</section>;
}

function MetricCard({ icon: Icon, label, value, helper, tone, interactive, onClick }) {
  const content = <><span className="dashboard-metric-icon"><Icon size={17} /></span><span className="dashboard-metric-label">{label}</span><strong>{value}</strong><small>{helper}</small></>;
  return interactive ? <button className={`dashboard-metric-card ${tone}`} onClick={onClick} type="button">{content}</button> : <div className={`dashboard-metric-card ${tone}`}>{content}</div>;
}

function RecentExecutions({ rows, loading, error, onRetry, canViewExecutions }) {
  const columns = [
    { key: "title", title: "执行名称", width: "24%", render: (row) => <strong className="dashboard-execution-title">{row.title || "未命名执行"}</strong> },
    { key: "projectName", title: "项目", width: "16%", render: (row) => row.projectName || "—" },
    { key: "type", title: "类型", width: "12%", render: (row) => <span className="dashboard-type-tag">{TYPE_LABELS[row.type] || row.type || "—"}</span> },
    { key: "status", title: "状态", width: "14%", render: (row) => <StatusBadge status={row.status} /> },
    { key: "startedAt", title: "开始时间", width: "15%", render: (row) => formatRelativeTime(row.startedAt || row.createdAt) },
    { key: "durationMs", title: "耗时", width: "11%", render: (row) => formatDuration(row.durationMs) },
    { key: "actions", title: "操作", width: "8%", render: (row) => <button className="link-button" onClick={() => navigateToTarget(row.targetUrl, getExecutionFallbackPath(row, canViewExecutions), canViewExecutions)} type="button">详情</button> }
  ];
  return <section aria-label="最近执行" className="dashboard-recent resource-panel"><div className="dashboard-section-heading"><div><strong>最近执行</strong><span>最近 5 条 UI、API 和性能测试执行记录</span></div>{canViewExecutions ? <button className="link-button" onClick={() => navigateTo(["执行中心", "执行记录"])} type="button">查看全部 <ArrowRight size={13} /></button> : null}</div>{error ? <div className="dashboard-inline-error">最近执行暂时无法加载<button className="link-button" onClick={onRetry} type="button">重试</button></div> : <><div className="dashboard-table-desktop"><TablePanel compact><DataTable columns={columns} rows={rows.slice(0, 5)} rowKey="dashboardKey" emptyText="暂无执行记录" fitContainer /></TablePanel></div><div className="dashboard-execution-cards">{rows.length ? rows.slice(0, 5).map((row) => <button className="dashboard-execution-card" key={row.dashboardKey} onClick={() => navigateToTarget(row.targetUrl, getExecutionFallbackPath(row, canViewExecutions), canViewExecutions)} type="button"><strong>{row.title || "未命名执行"}</strong><span>{row.projectName || "—"} · {TYPE_LABELS[row.type] || row.type || "—"}</span><small><StatusBadge status={row.status} /> {formatRelativeTime(row.startedAt || row.createdAt)} · {formatDuration(row.durationMs)}</small></button>) : <div className="dashboard-mobile-empty">暂无执行记录</div>}</div></>}</section>;
}

function QuickActions({ permissions, isAdmin }) {
  const can = (permission) => isAdmin || permissions.has(permission);
  const actions = [
    can("perf.plan.manage") ? ["创建压测方案", Plus, ["性能测试", "压测方案", "new"]] : null,
    can("menu.execution.read") ? ["查看执行记录", ClipboardList, ["执行中心", "执行记录"]] : null,
    can("menu.api_automation.read") ? ["管理接口资产", Settings2, ["接口自动化", "接口管理"]] : null,
    can("menu.execution.read") ? ["查看质量报告", FileBarChart, ["执行中心", "测试报告"]] : null
  ].filter(Boolean);
  return <section className="dashboard-quick resource-panel"><div className="dashboard-section-heading"><div><strong>快捷操作</strong><span>快速进入常用工作</span></div></div><div className="dashboard-action-grid">{actions.map(([label, Icon, path]) => <button className="icon-text-button" key={label} onClick={() => navigateTo(path)} type="button"><Icon size={15} />{label}</button>)}</div>{!actions.length ? <div className="dashboard-empty-small">暂无可用快捷操作</div> : null}</section>;
}

function StatusBadge({ status }) { return <span className={`dashboard-status ${status || "unknown"}`}><span />{STATUS_LABELS[status] || "未知"}</span>; }

function DashboardSkeleton() { return <div className="dashboard-skeleton" aria-label="正在加载项目概览"><div className="skeleton-block dashboard-skeleton-attention" /><div className="dashboard-skeleton-metrics"><div className="skeleton-block" /><div className="skeleton-block" /><div className="skeleton-block" /></div><div className="skeleton-block dashboard-skeleton-table" /></div>; }

export function getPartialMessage(data) {
  const sourceStates = data?.executions?.sourceStates || {};
  const states = Object.values(sourceStates);
  const hasSourceError = states.some((value) => value === "error" || value?.state === "error" || value?.status === "error");
  const onlyForbidden = states.length > 0 && states.every((value) => value === "forbidden" || value?.state === "forbidden" || value?.status === "forbidden");
  if (hasSourceError || (data?.executions?.state === "partial" && !onlyForbidden)) return "部分执行数据暂时无法加载，当前结果可能不完整。";
  return "";
}

export function getDashboardItemKey(item = {}) {
  return item.uid || `${item.type || item.source}:${item.id}`;
}

export function hasRunningExecutions(data) {
  return Number(data?.executions?.counts?.running || 0) > 0;
}

export function navigateTo(path) { window.location.hash = pathToHash(path); }

export function navigateToTarget(targetUrl, fallback, canViewExecutions = true) {
  const safePath = typeof targetUrl === "string" && targetUrl.startsWith("#/") ? pathFromHash(targetUrl) : null;
  navigateTo(safePath && (canViewExecutions || safePath[0] !== "执行中心") ? safePath : fallback);
}

function getExecutionFallbackPath(item, canViewExecutions) {
  const type = item?.type || item?.source;
  if (type === "api") return ["接口自动化", "接口管理"];
  if (type === "ui") return ["界面自动化", "测试用例"];
  if (type === "perf") return canViewExecutions ? ["性能测试", "测试报告"] : ["性能测试", "压测方案"];
  return canViewExecutions ? ["执行中心", "执行记录"] : ["首页", "项目概览"];
}

function rangeLabel(range) { return RANGE_OPTIONS.find(([value]) => value === range)?.[1]?.replace("最近 ", "") || "7 天"; }
function formatDuration(durationMs) { if (durationMs === null || durationMs === undefined) return "—"; if (durationMs < 1000) return `${durationMs} ms`; return `${Math.floor(durationMs / 60000)} 分 ${Math.floor((durationMs % 60000) / 1000)} 秒`; }
function formatRelativeTime(value) { if (!value) return "—"; const time = new Date(value).getTime(); if (Number.isNaN(time)) return "—"; const minutes = Math.max(0, Math.floor((Date.now() - time) / 60000)); if (minutes < 1) return "刚刚"; if (minutes < 60) return `${minutes} 分钟前`; const hours = Math.floor(minutes / 60); if (hours < 24) return `${hours} 小时前`; return `${Math.floor(hours / 24)} 天前`; }
