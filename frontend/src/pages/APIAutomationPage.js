import { useMemo, useState } from "react";
import { ArrowLeft, Braces, Clipboard, FileJson, FlaskConical, KeyRound, Play, Plus, RefreshCw, Save, Upload } from "lucide-react";
import { DataTable, PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { apiAutomationService } from "../services/apiAutomationService.js";
import { configService } from "../services/configService.js";
import { pageItems } from "../utils/formatters.js";

const emptyForm = { name: "", category: "", action: "", locator: "", method: "GET", value: "HTTP", description: "WEB", status: "active" };
const initialFilters = { id: "", name: "", url: "", product: "", module: "", endpointType: "", method: "", protocol: "", status: "" };

export function APIAutomationPage({ activePath }) {
  const section = activePath[1] || "接口管理";
  if (section === "接口管理") return <InterfaceManagementPage />;
  return <APISectionPlaceholder section={section} />;
}

function InterfaceManagementPage() {
  const [form, setForm] = useState(emptyForm);
  const [filters, setFilters] = useState(initialFilters);
  const [applied, setApplied] = useState(initialFilters);
  const [selected, setSelected] = useState([]);
  const [modal, setModal] = useState(false);
  const [detailRow, setDetailRow] = useState(null);
  const [editing, setEditing] = useState(null);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const { data, loading, error, reload } = useAsyncData(() => apiAutomationService.interfaces.list({ page: 1, pageSize: 100 }), []);
  const { data: productsData } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const products = useMemo(() => pageItems(productsData).map((item) => ({ value: String(item.id), label: `${item.projectName}/${item.name}` })), [productsData]);
  const selectedProductId = form.category || filters.product;
  const { data: modulesData } = useAsyncData(() => selectedProductId ? configService.productModules.list({ productId: selectedProductId, page: 1, pageSize: 200 }) : Promise.resolve({ items: [] }), [selectedProductId]);
  const modules = pageItems(modulesData);
  const sourceRows = pageItems(data);
  const rows = sourceRows.filter((row) =>
    (!applied.id || String(row.id) === applied.id) &&
    (!applied.name || row.name.toLowerCase().includes(applied.name.toLowerCase())) &&
    (!applied.url || row.locator.toLowerCase().includes(applied.url.toLowerCase())) &&
    (!applied.product || row.category === applied.product) &&
    (!applied.module || row.action === applied.module) &&
    (!applied.endpointType || interfaceMeta(row).endpointType === applied.endpointType) &&
    (!applied.method || row.method === applied.method) &&
    (!applied.protocol || row.value === applied.protocol) &&
    (!applied.status || row.status === applied.status)
  );
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
  const pageRows = rows.slice((page - 1) * pageSize, page * pageSize);

  function openCreate() { setEditing(null); setForm(emptyForm); setNotice(""); setModal(true); }
  function openEdit(row) { setEditing(row); setForm({ name: row.name, category: row.category, action: row.action, locator: row.locator, method: row.method, value: row.value, description: row.description, status: row.status }); setModal(true); }
  async function save(event) {
    event.preventDefault();
    if (!form.category || !form.action || !form.name.trim() || !form.locator.trim()) { setNotice("项目/产品、模块、接口名称和 URL 路径不能为空。"); return; }
    setBusy(true);
    try {
      if (editing) await apiAutomationService.interfaces.update(editing.id, form);
      else await apiAutomationService.interfaces.create(form);
      await reload(); setModal(false); setNotice(editing ? "接口已更新。" : "接口已新增。");
    } catch (err) { setNotice(err.message || "保存接口失败"); } finally { setBusy(false); }
  }
  async function removeRows(ids) {
    if (!ids.length || !window.confirm(`确认删除选中的 ${ids.length} 个接口吗？`)) return;
    setBusy(true);
    try { await Promise.all(ids.map((id) => apiAutomationService.interfaces.remove(id))); setSelected([]); await reload(); setNotice("接口已删除。"); }
    catch (err) { setNotice(err.message || "删除接口失败"); } finally { setBusy(false); }
  }

  const columns = [
    { key: "select", title: <input checked={pageRows.length > 0 && pageRows.every((row) => selected.includes(row.id))} onChange={(event) => setSelected(event.target.checked ? pageRows.map((row) => row.id) : [])} type="checkbox" />, render: (row) => <input checked={selected.includes(row.id)} onChange={() => setSelected((current) => current.includes(row.id) ? current.filter((id) => id !== row.id) : [...current, row.id])} type="checkbox" /> },
    { key: "id", title: "ID" },
    { key: "category", title: "项目/产品", render: (row) => products.find((item) => item.value === row.category)?.label || row.category },
    { key: "action", title: "模块名称" },
    { key: "name", title: "接口名称" },
    { key: "locator", title: "URL", render: (row) => <code>{row.locator}</code> },
    { key: "description", title: "端类型", render: (row) => <span className="api-tag endpoint">{interfaceMeta(row).endpointType}</span> },
    { key: "method", title: "方法", render: (row) => <span className={`api-tag method-${row.method?.toLowerCase()}`}>{row.method}</span> },
    { key: "value", title: "协议", render: (row) => <span className="api-tag protocol">{row.value}</span> },
    { key: "status", title: "状态", render: (row) => <span className={`status-badge ${row.status === "active" ? "status-passed" : "status-failed"}`}>{row.status === "active" ? "通过" : "失败"}</span> },
    { key: "operations", title: "操作", render: (row) => <div className="action-links"><button className="link-button" onClick={() => setDetailRow(row)} type="button">执行</button><button className="link-button" onClick={() => setDetailRow(row)} type="button">详情</button><button className="link-button danger-link" onClick={() => removeRows([row.id])} type="button">删除</button></div> }
  ];

  if (detailRow) return <InterfaceDetailWorkspace row={detailRow} onBack={() => { setDetailRow(null); reload(); }} />;

  return <div className="section-stack api-interface-page">
    <PageHeader title="接口管理" description="收集、维护和调试接口定义" />
    <section className="resource-panel">
      <div className="panel-header"><strong>接口信息收集</strong></div>
      <form className="api-filter-grid" onSubmit={(event) => { event.preventDefault(); setApplied(filters); setPage(1); }}>
        <FilterInput label="ID" value={filters.id} onChange={(value) => setFilters({ ...filters, id: value })} />
        <FilterInput label="接口名称" value={filters.name} onChange={(value) => setFilters({ ...filters, name: value })} />
        <FilterInput label="URL" value={filters.url} onChange={(value) => setFilters({ ...filters, url: value })} />
        <FilterSelect label="项目/产品" value={filters.product} onChange={(value) => setFilters({ ...filters, product: value, module: "" })} options={products} />
        <FilterSelect label="模块名称" value={filters.module} onChange={(value) => setFilters({ ...filters, module: value })} options={modules.map((item) => ({ value: item.name, label: item.name }))} />
        <FilterSelect label="端类型" value={filters.endpointType} onChange={(value) => setFilters({ ...filters, endpointType: value })} options={["WEB", "APP"].map(option)} />
        <FilterSelect label="方法" value={filters.method} onChange={(value) => setFilters({ ...filters, method: value })} options={["GET", "POST", "PUT", "DELETE", "PATCH"].map(option)} />
        <FilterSelect label="协议" value={filters.protocol} onChange={(value) => setFilters({ ...filters, protocol: value })} options={["HTTP", "HTTPS"].map(option)} />
        <FilterSelect label="状态" value={filters.status} onChange={(value) => setFilters({ ...filters, status: value })} options={[{ value: "active", label: "通过" }, { value: "disabled", label: "失败" }]} />
        <div className="api-filter-actions"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={() => { setFilters(initialFilters); setApplied(initialFilters); }} type="button">重置</button></div>
      </form>
      <div className="api-list-toolbar"><div className="api-tabs"><button type="button">批量生成</button><button className="active" type="button">调试接口</button></div><div><button className="primary-button compact-button" onClick={openCreate} type="button"><Plus size={14} />新增</button><button className="icon-text-button compact-button" type="button"><Upload size={14} />导入</button><button className="success-button compact-button" disabled={!selected.length} type="button">批量执行</button><button className="danger-button compact-button" disabled={!selected.length || busy} onClick={() => removeRows(selected)} type="button">批量删除</button><button className="icon-text-button compact-button" onClick={reload} type="button"><RefreshCw size={14} /></button></div></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <StateBlock loading={loading} error={error}><TablePanel><DataTable columns={columns} rows={pageRows} emptyText="暂无接口数据" /><PaginationBar page={page} pageSize={pageSize} total={rows.length} totalPages={totalPages} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} /></TablePanel></StateBlock>
    </section>
    {modal ? <InterfaceModal form={form} setForm={setForm} products={products} modules={modules} editing={editing} busy={busy} onClose={() => setModal(false)} onSubmit={save} /> : null}
  </div>;
}

const detailSections = [
  ["headers", "请求头", "维护接口请求 Headers"],
  ["params", "参数", "维护查询参数"],
  ["body", "请求体", "维护 Body 配置"],
  ["jsonpath", "后置 JSONPath 提取", "提取响应 JSON 到缓存"],
  ["regex", "后置正则提取", "通过正则提取响应内容"],
  ["script", "后置脚本", "编写自定义响应处理脚本"],
  ["assertions", "结构化断言配置", "维护响应断言规则"]
];

function InterfaceDetailWorkspace({ row, onBack }) {
  const saved = interfaceMeta(row);
  const [active, setActive] = useState("headers");
  const [config, setConfig] = useState({
    headers: saved.headers || "{\n  \"Content-Type\": \"application/json\"\n}",
    params: saved.params || "{\n  \n}",
    body: saved.body || "{\n  \n}",
    jsonpath: saved.jsonpath || "[]",
    regex: saved.regex || "[]",
    script: saved.script || "",
    assertions: saved.assertions || "[]"
  });
  const [method, setMethod] = useState(row.method || "GET");
  const [url, setUrl] = useState(row.locator || "");
  const [result, setResult] = useState(null);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  async function saveConfiguration() {
    setBusy(true); setNotice("");
    try {
      await apiAutomationService.interfaces.update(row.id, { ...row, method, locator: url, description: JSON.stringify({ ...saved, endpointType: saved.endpointType, ...config }) });
      setNotice("接口配置已保存。");
    } catch (error) { setNotice(error.message || "保存接口配置失败"); } finally { setBusy(false); }
  }

  async function execute() {
    setBusy(true); setNotice("");
    const started = performance.now();
    try {
      const headers = JSON.parse(config.headers || "{}");
      const params = JSON.parse(config.params || "{}");
      const target = new URL(url, window.location.origin);
      Object.entries(params).forEach(([key, value]) => target.searchParams.set(key, String(value)));
      const options = { method, headers };
      if (!["GET", "HEAD"].includes(method)) options.body = config.body || undefined;
      const response = await fetch(target, options);
      const body = await response.text();
      setResult({ status: response.status, duration: performance.now() - started, ok: response.ok, headers: Object.fromEntries(response.headers.entries()), body, request: { url: target.toString(), method, headers, body: options.body || "" } });
    } catch (error) {
      setResult({ status: "-", duration: performance.now() - started, ok: false, headers: {}, body: error.message, request: { url, method } });
    } finally { setBusy(false); }
  }

  return <div className="api-detail-page">
    <header className="api-detail-header"><div><span>接口配置工作台 / #{row.id}</span><h2>{row.name}</h2><p>维护请求配置、后置处理、断言和最近一次响应结果</p></div><div><button className="success-button compact-button" disabled={busy} onClick={execute} type="button"><Play size={14} />执行</button><button className="icon-text-button compact-button" onClick={onBack} type="button"><ArrowLeft size={14} />返回</button></div></header>
    {notice ? <div className="inline-notice">{notice}</div> : null}
    <div className="api-detail-layout">
      <aside className="api-detail-nav"><div><strong>配置项</strong><span>按执行链路维护接口配置</span></div>{detailSections.map(([key, title, description]) => <button className={active === key ? "active" : ""} key={key} onClick={() => setActive(key)} type="button"><strong>{title}</strong><span>{description}</span></button>)}</aside>
      <main className="api-detail-editor">
        <div className="api-request-line"><select className="text-input" value={method} onChange={(event) => setMethod(event.target.value)}>{["GET", "POST", "PUT", "DELETE", "PATCH"].map((item) => <option key={item}>{item}</option>)}</select><input className="text-input" value={url} onChange={(event) => setUrl(event.target.value)} /><button className="primary-button compact-button" disabled={busy} onClick={saveConfiguration} type="button"><Save size={14} />保存</button></div>
        <div className="api-editor-heading"><strong>{detailSections.find(([key]) => key === active)?.[1]}</strong><span>{detailSections.find(([key]) => key === active)?.[2]}</span></div>
        <div className="api-editor-tip">请输入合法 JSON；后置脚本支持普通文本配置，保存后用于执行阶段处理。</div>
        <textarea className="api-config-editor" spellCheck="false" value={config[active]} onChange={(event) => setConfig({ ...config, [active]: event.target.value })} />
      </main>
      <aside className="api-result-panel">
        <div className="api-result-heading"><div><strong>调用结果</strong><span>执行后固定展示最近一次响应</span></div><button className="success-button compact-button" disabled={busy} onClick={execute} type="button"><Play size={14} />{busy ? "执行中" : "执行"}</button></div>
        <div className="api-result-summary"><div><span>状态码</span><strong>{result?.status ?? "-"}</strong></div><div><span>响应时间</span><strong>{result ? `${(result.duration / 1000).toFixed(2)} 秒` : "-"}</strong></div><div><span>执行状态</span><strong className={result?.ok ? "success" : result ? "danger" : ""}>{result ? result.ok ? "调用完成" : "调用失败" : "等待执行"}</strong></div></div>
        <div className="api-result-tabs"><span className="active">响应</span><span>请求</span><span>缓存</span></div>
        <ResultBlock label="响应头" value={result ? JSON.stringify(result.headers, null, 2) : ""} />
        <ResultBlock label="响应体" value={result?.body || ""} />
        {result ? <ResultBlock label="实际请求" value={JSON.stringify(result.request, null, 2)} /> : null}
      </aside>
    </div>
  </div>;
}

function ResultBlock({ label, value }) {
  return <section className="api-result-block"><div><strong>{label}</strong><button onClick={() => navigator.clipboard?.writeText(value)} type="button"><Clipboard size={12} />复制</button></div><pre>{value || "执行接口后显示结果"}</pre></section>;
}

function interfaceMeta(row) {
  try {
    const parsed = JSON.parse(row.description || "");
    return parsed && typeof parsed === "object" ? { endpointType: "WEB", ...parsed } : { endpointType: "WEB" };
  } catch {
    return { endpointType: row.description || "WEB" };
  }
}

function InterfaceModal({ form, setForm, products, modules, editing, busy, onClose, onSubmit }) {
  return <div className="modal-backdrop"><form className="modal-card api-interface-modal" onSubmit={onSubmit}><div className="modal-header"><strong>{editing ? "编辑接口" : "新增接口"}</strong><button className="modal-close" onClick={onClose} type="button">×</button></div><div className="api-interface-form">
    <FilterSelect required label="项目/产品" value={form.category} onChange={(value) => setForm({ ...form, category: value, action: "" })} options={products} />
    <FilterSelect required label="模块名称" value={form.action} onChange={(value) => setForm({ ...form, action: value })} options={modules.map((item) => ({ value: item.name, label: item.name }))} />
    <FilterInput required label="接口名称" value={form.name} onChange={(value) => setForm({ ...form, name: value })} />
    <FilterInput required label="URL 路径" value={form.locator} onChange={(value) => setForm({ ...form, locator: value })} placeholder="/reimbursements" />
    <FilterSelect required label="Method" value={form.method} onChange={(value) => setForm({ ...form, method: value })} options={["GET", "POST", "PUT", "DELETE", "PATCH"].map(option)} />
    <FilterSelect required label="协议" value={form.value} onChange={(value) => setForm({ ...form, value })} options={["HTTP", "HTTPS"].map(option)} />
    <FilterSelect label="端类型" value={form.description} onChange={(value) => setForm({ ...form, description: value })} options={["WEB", "APP"].map(option)} />
  </div><div className="modal-actions"><button className="icon-text-button compact-button" onClick={onClose} type="button">取消</button><button className="primary-button compact-button" disabled={busy} type="submit">{busy ? "提交中" : "提交"}</button></div></form></div>;
}

function FilterInput({ label, value, onChange, placeholder, required }) { return <label className="form-field"><span>{required ? "* " : ""}{label}</span><input className="text-input" value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder || `请输入${label}`} /></label>; }
function FilterSelect({ label, value, onChange, options, required }) { return <label className="form-field"><span>{required ? "* " : ""}{label}</span><select className="text-input" value={value} onChange={(event) => onChange(event.target.value)}><option value="">请选择{label}</option>{options.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>; }
function option(value) { return { value, label: value }; }

const placeholderMeta = {
  测试用例: [FlaskConical, "API CASES", "还没有接口测试用例", "从已维护的接口创建测试用例。"],
  全局变量: [FileJson, "VARIABLES", "还没有接口全局变量", "变量可通过 ${variable_name} 在请求中引用。"],
  请求头管理: [KeyRound, "HEADERS", "还没有公共请求头", "创建请求头模板后，可在多个接口和用例中复用。"]
};
function APISectionPlaceholder({ section }) {
  const [Icon, kicker, title, text] = placeholderMeta[section] || placeholderMeta["测试用例"];
  return <div className="section-stack api-automation-page"><PageHeader title={section} description="接口自动化资产管理" /><section className="resource-panel api-resource-panel"><div className="api-section-marker"><Icon size={18} /><span>{kicker}</span></div><div className="api-empty-state"><span><Icon size={24} /></span><strong>{title}</strong><p>{text}</p><button className="primary-button compact-button" type="button">新增{section === "测试用例" ? "用例" : section.replace("管理", "")}</button></div></section></div>;
}
