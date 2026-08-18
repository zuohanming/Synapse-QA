import { useState } from "react";
import { Search, X } from "lucide-react";
import { StateBlock } from "./StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { apiAutomationService } from "../services/apiAutomationService.js";
import { pageItems } from "../utils/formatters.js";

const SUPPORTED_METHODS = ["GET", "POST", "PUT", "DELETE", "PATCH"];

export function PerfInterfacePicker({ productId, multiple = false, maxSelection, onClose, onConfirm }) {
  const [draft, setDraft] = useState({ keyword: "", method: "" });
  const [applied, setApplied] = useState(draft);
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState([]);
  const [detailError, setDetailError] = useState("");
  const pageSize = 10;
  const { data, loading, error } = useAsyncData(() => apiAutomationService.interfaces.list({ productId, keyword: applied.keyword, method: applied.method, page, pageSize }), [productId, applied, page]);
  const rows = pageItems(data);
  const total = Number(data?.total || rows.length);
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  function search(event) { event.preventDefault(); setPage(1); setApplied({ ...draft }); }
  const selectionLimit = multiple && Number.isFinite(maxSelection) ? Math.max(0, maxSelection) : Infinity;
  function toggle(row) { if (!isSupported(row.method)) return; setSelected((current) => { if (!multiple) return [row.id]; if (current.includes(row.id)) return current.filter((id) => id !== row.id); return current.length >= selectionLimit ? current : [...current, row.id]; }); }
  async function confirm() {
    setDetailError("");
    try { const details = await Promise.all(selected.map((id) => apiAutomationService.interfaces.get(id))); await onConfirm(details); } catch (err) { setDetailError(err.message || "读取接口配置失败"); }
  }
  const remaining = Number.isFinite(selectionLimit) ? Math.max(0, selectionLimit - selected.length) : null;
  return <div className="modal-backdrop"><section aria-label="从接口管理添加" className="modal-card modal-card-wide perf-interface-picker"><div className="modal-header"><div><strong>从接口管理添加</strong><small>导入后为独立快照，后续编辑不会联动接口管理。</small></div><button aria-label="关闭接口选择" className="modal-close" onClick={onClose} type="button"><X size={18} /></button></div><form className="perf-picker-toolbar" onSubmit={search}><label className="form-field"><span>关键词</span><input className="text-input" value={draft.keyword} onChange={(event) => setDraft({ ...draft, keyword: event.target.value })} placeholder="接口名称或路径" /></label><label className="form-field"><span>方法</span><select className="text-input" value={draft.method} onChange={(event) => setDraft({ ...draft, method: event.target.value })}><option value="">全部方法</option>{[...SUPPORTED_METHODS, "HEAD", "OPTIONS"].map((method) => <option key={method} value={method}>{method}{isSupported(method) ? "" : "（压测不支持）"}</option>)}</select></label><button className="primary-button compact-button" type="submit"><Search size={14} />搜索</button></form><StateBlock loading={loading} error={error}><div className="perf-picker-count">已选 {selected.length} 个{multiple ? `，本次最多还能选择 ${remaining} 个` : "（单选）"}</div>{detailError ? <div className="form-error" role="alert">{detailError}，可以调整选择后重试。</div> : null}<div className="table-wrap"><table className="data-table perf-picker-table"><thead><tr><th>{multiple ? "选择" : ""}</th><th>接口</th><th>方法</th><th>路径</th></tr></thead><tbody>{rows.length ? rows.map((row) => { const supported = isSupported(row.method); const checked = selected.includes(row.id); const limited = multiple && !checked && selected.length >= selectionLimit; return <tr className={!supported ? "is-disabled" : ""} key={row.id}><td><input aria-label={`选择 ${row.name || row.path}`} checked={checked} disabled={!supported || limited} onChange={() => toggle(row)} type={multiple ? "checkbox" : "radio"} /></td><td title={row.name || "未命名接口"}>{row.name || "未命名接口"}</td><td>{row.method || "--"}{!supported ? <small className="perf-picker-unsupported">压测不支持</small> : null}</td><td title={row.path || ""}><code>{row.path || "--"}</code></td></tr>; }) : <tr><td className="table-empty-cell" colSpan="4">暂无可导入接口</td></tr>}</tbody></table></div><div className="perf-picker-pagination"><button disabled={page <= 1} onClick={() => setPage(page - 1)} type="button">上一页</button><span>第 {page} / {totalPages} 页 · 共 {total} 条</span><button disabled={page >= totalPages} onClick={() => setPage(page + 1)} type="button">下一页</button></div></StateBlock><div className="modal-actions"><button className="icon-text-button compact-button" onClick={onClose} type="button">取消</button><button className="primary-button compact-button" disabled={!selected.length} onClick={confirm} type="button">导入已选</button></div></section></div>;
}

export function isSupportedPerfMethod(method) { return SUPPORTED_METHODS.includes(String(method || "").toUpperCase()); }
function isSupported(method) { return isSupportedPerfMethod(method); }
