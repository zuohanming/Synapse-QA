import { useMemo, useState } from "react";
import { AlignLeft, ArrowLeft, Braces, ChevronDown, ChevronRight, Clipboard, FileJson, FlaskConical, KeyRound, Minimize2, PanelLeftClose, PanelLeftOpen, Play, Plus, RefreshCw, Save, Upload } from "lucide-react";
import { DataTable, PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { apiAutomationService } from "../services/apiAutomationService.js";
import { configService } from "../services/configService.js";
import { pageItems } from "../utils/formatters.js";

const emptyForm = { name: "", productId: "", moduleId: "", path: "", method: "GET", protocol: "HTTP", endpointType: "WEB", lifecycleStatus: "draft", timeoutSeconds: 30, followRedirects: true, configuration: {}, revision: 0 };
const initialFilters = { keyword: "", projectId: "", productId: "", moduleId: "", method: "", lifecycleStatus: "", deletionState: "active" };

export function APIAutomationPage({ activePath }) {
  const section = activePath[1] || "接口管理";
  if (section === "接口管理") return <InterfaceManagementPage />;
  if (section === "请求头管理") return <RequestHeaderManagementPage />;
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
  const [resourceCollapsed, setResourceCollapsed] = useState(false);
  const [expandedProjects, setExpandedProjects] = useState([]);
  const [curlModal, setCurlModal] = useState(false);
  const [curlCommand, setCurlCommand] = useState("");
  const [curlError, setCurlError] = useState("");
  const { data, loading, error, reload } = useAsyncData(() => {
    const { deletionState, ...query } = applied;
    return apiAutomationService.interfaces.list({
      ...query,
      includeDeleted: deletionState === "all",
      deletedOnly: deletionState === "deleted",
      page,
      pageSize
    });
  }, [applied, page, pageSize]);
  const { data: projectsData } = useAsyncData(() => configService.projects.list({ page: 1, pageSize: 200 }), []);
  const { data: productsData } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const products = useMemo(() => pageItems(productsData).map((item) => ({ value: String(item.id), label: `${item.projectName}/${item.name}`, projectId: String(item.projectId) })), [productsData]);
  const projects = useMemo(() => pageItems(projectsData), [projectsData]);
  const selectedProductId = form.productId || filters.productId;
  const { data: modulesData } = useAsyncData(() => selectedProductId ? configService.productModules.list({ productId: selectedProductId, page: 1, pageSize: 200 }) : Promise.resolve({ items: [] }), [selectedProductId]);
  const modules = pageItems(modulesData);
  const pageRows = pageItems(data);
  const total = Number(data?.total || 0);
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  function toggleProject(projectId) {
    const value = String(projectId);
    setExpandedProjects((current) => current.includes(value)
      ? current.filter((id) => id !== value)
      : [...current, value]);
  }

  function openCreate() { setEditing(null); setForm(emptyForm); setNotice(""); setModal(true); }
  function openEdit(row) { setEditing(row); setForm({ name: row.name, productId: String(row.productId), moduleId: row.moduleId ? String(row.moduleId) : "", path: row.path, method: row.method, protocol: row.protocol, endpointType: row.endpointType, lifecycleStatus: row.lifecycleStatus, timeoutSeconds: row.timeoutSeconds, followRedirects: row.followRedirects, configuration: row.configuration || {}, revision: row.revision }); setModal(true); }
  async function save(event) {
    event.preventDefault();
    if (!form.productId || !form.name.trim() || !form.path.trim()) { setNotice("项目/产品、接口名称和 URL 路径不能为空。"); return; }
    setBusy(true);
    try {
      const payload = { ...form, productId: Number(form.productId), moduleId: Number(form.moduleId || 0) };
      if (editing) await apiAutomationService.interfaces.update(editing.id, payload);
      else await apiAutomationService.interfaces.create(payload);
      await reload(); setModal(false); setNotice(editing ? "接口已更新。" : "接口已新增。");
    } catch (err) { setNotice(err.message || "保存接口失败"); } finally { setBusy(false); }
  }
  async function removeRows(ids) {
    if (!ids.length || !window.confirm(`确认删除选中的 ${ids.length} 个接口吗？`)) return;
    setBusy(true);
    try {
      const result = ids.length === 1
        ? (await apiAutomationService.interfaces.remove(ids[0]), { succeeded: ids, failed: [] })
        : await apiAutomationService.interfaces.batchDelete(ids);
      setSelected([]);
      await reload();
      setNotice(`删除完成：成功 ${result.succeeded?.length || 0} 条，失败 ${result.failed?.length || 0} 条。`);
    }
    catch (err) { setNotice(err.message || "删除接口失败"); } finally { setBusy(false); }
  }
  async function batchStatus(status) {
    if (!selected.length) return;
    setBusy(true);
    try {
      const result = await apiAutomationService.interfaces.batchStatus(selected, status);
      setSelected([]);
      await reload();
      setNotice(`批量操作完成：成功 ${result.succeeded?.length || 0} 条，失败 ${result.failed?.length || 0} 条。`);
    } catch (err) { setNotice(err.message || "批量更新接口状态失败"); } finally { setBusy(false); }
  }
  async function restoreRow(id) {
    setBusy(true);
    try { await apiAutomationService.interfaces.restore(id); await reload(); setNotice("接口已恢复。"); }
    catch (err) { setNotice(err.message || "恢复接口失败"); } finally { setBusy(false); }
  }
  async function batchMoveToCurrentProduct() {
    if (!selected.length || !filters.productId) {
      setNotice("请先在筛选区选择目标产品，再勾选需要移动的接口。");
      return;
    }
    setBusy(true);
    try {
      const result = await apiAutomationService.interfaces.batchMove(selected, Number(filters.productId), Number(filters.moduleId || 0));
      setSelected([]);
      await reload();
      setNotice(`批量移动完成：成功 ${result.succeeded?.length || 0} 条，失败 ${result.failed?.length || 0} 条。`);
    } catch (err) { setNotice(err.message || "批量移动接口失败"); } finally { setBusy(false); }
  }
  function openCurlImport() {
    setCurlCommand("");
    setCurlError("");
    setCurlModal(true);
  }
  async function importCurl(event) {
    event.preventDefault();
    const command = curlCommand.trim();
    if (!command) {
      setCurlError("请粘贴需要导入的 cURL 命令。");
      return;
    }
    if (!/^curl(?:\s|$)/i.test(command)) {
      setCurlError("命令需要以 curl 开头，请检查后重试。");
      return;
    }
    setBusy(true); setNotice(""); setCurlError("");
    try {
      const parsed = await apiAutomationService.curl.parse(command);
      setEditing(null);
      setForm({
        ...emptyForm,
        productId: filters.productId,
        method: parsed.method,
        path: parsed.url,
        protocol: parsed.protocol || "HTTP",
        configuration: {
          headers: JSON.stringify(parsed.headers || {}, null, 2),
          params: "{}",
          body: parsed.body || ""
        }
      });
      setCurlModal(false);
      setModal(true);
      setNotice(parsed.maskedHeaders?.length ? `已识别敏感请求头：${parsed.maskedHeaders.join("、")}` : "cURL 已解析，请补充接口归属和名称。");
    } catch (error) {
      setCurlError(error.message || "解析失败，请检查命令格式和转义字符。");
    } finally {
      setBusy(false);
    }
  }

  const columns = [
    { key: "select", width: "3%", title: <input checked={pageRows.length > 0 && pageRows.every((row) => selected.includes(row.id))} onChange={(event) => setSelected(event.target.checked ? pageRows.map((row) => row.id) : [])} type="checkbox" />, render: (row) => <input checked={selected.includes(row.id)} onChange={() => setSelected((current) => current.includes(row.id) ? current.filter((id) => id !== row.id) : [...current, row.id])} type="checkbox" /> },
    { key: "id", width: "4%", title: "ID" },
    { key: "productName", width: "12%", title: "项目/产品", render: (row) => `${row.projectName}/${row.productName}` },
    { key: "moduleName", width: "9%", title: "模块名称", render: (row) => row.moduleName || "未分组" },
    { key: "name", width: "11%", title: "接口名称" },
    { key: "path", width: "21%", title: "方法 / 路径", render: (row) => <span><span className={`api-tag method-${row.method?.toLowerCase()}`}>{row.method}</span> <code>{row.path}</code></span> },
    { key: "endpointType", width: "7%", title: "端类型", render: (row) => <span className="api-tag endpoint">{row.endpointType}</span> },
    { key: "lifecycleStatus", width: "8%", title: "接口状态", render: (row) => <span className={`status-badge ${row.lifecycleStatus === "active" && !row.deletedAt ? "status-passed" : ""}`}>{row.deletedAt ? "已删除" : lifecycleLabel(row.lifecycleStatus)}</span> },
    { key: "lastDebugStatus", width: "8%", title: "最近调试", render: (row) => row.lastDebugStatus ? <span className={`status-badge ${row.lastDebugStatus === "success" ? "status-passed" : "status-failed"}`}>{row.lastDebugStatus === "success" ? "通过" : "失败"}</span> : "未调试" },
    { key: "updatedBy", width: "10%", title: "最近修改", render: (row) => <span>{row.updatedBy}<br /><small>{new Date(row.updatedAt).toLocaleString()}</small></span> },
    { key: "operations", width: "7%", title: "操作", render: (row) => row.deletedAt
      ? <div className="action-links"><button className="link-button" disabled={busy} onClick={() => restoreRow(row.id)} type="button">恢复</button></div>
      : <div className="action-links"><button className="link-button" onClick={() => setDetailRow(row)} type="button">调试</button><button className="link-button" onClick={() => openEdit(row)} type="button">编辑</button><button className="link-button danger-link" onClick={() => removeRows([row.id])} type="button">删除</button></div> }
  ];

  if (detailRow) {
    return <InterfaceDetailWorkspace row={detailRow} projectId={String(detailRow.projectId)} onBack={() => { setDetailRow(null); reload(); }} />;
  }

  return <div className="section-stack api-interface-page">
    <PageHeader title="接口管理" description="收集、维护和调试接口定义" />
    <div className={`api-interface-browser ${resourceCollapsed ? "resource-collapsed" : ""}`}>
    <aside className={`resource-panel api-resource-tree ${resourceCollapsed ? "collapsed" : ""}`}>
      <div className="api-resource-tree-header">
        {!resourceCollapsed ? <strong>接口资源</strong> : null}
        <button aria-label={resourceCollapsed ? "展开接口资源" : "收起接口资源"} className="resource-collapse-button" onClick={() => setResourceCollapsed((value) => !value)} title={resourceCollapsed ? "展开接口资源" : "收起接口资源"} type="button">{resourceCollapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}</button>
      </div>
      {!resourceCollapsed ? <>
        <button className={!filters.projectId ? "active" : ""} onClick={() => { setFilters(initialFilters); setApplied(initialFilters); setPage(1); }} type="button">全部项目</button>
        {projects.map((project) => {
          const projectId = String(project.id);
          const collapsed = !expandedProjects.includes(projectId);
          return <div className="api-resource-project" key={project.id}>
            <div className="api-resource-project-row">
              <button aria-label={collapsed ? `展开${project.name}` : `收起${project.name}`} className="project-toggle-button" onClick={() => toggleProject(projectId)} type="button">{collapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}</button>
              <button className={filters.projectId === projectId && !filters.productId ? "active" : ""} onClick={() => { const next = { ...initialFilters, projectId }; setExpandedProjects((current) => current.includes(projectId) ? current : [...current, projectId]); setFilters(next); setApplied(next); setPage(1); }} type="button">{project.name}</button>
            </div>
            {!collapsed ? products.filter((product) => product.projectId === projectId).map((product) => <button className={`child ${filters.productId === product.value ? "active" : ""}`} key={product.value} onClick={() => { const next = { ...initialFilters, projectId, productId: product.value }; setFilters(next); setApplied(next); setPage(1); }} type="button">{product.label.split("/").pop()}</button>) : null}
          </div>;
        })}
      </> : null}
    </aside>
    <section className="resource-panel api-interface-list">
      <div className="panel-header"><strong>接口信息收集</strong></div>
      <form className="api-filter-grid api-interface-filter" onSubmit={(event) => { event.preventDefault(); setApplied(filters); setPage(1); }}>
        <FilterInput label="名称或路径" value={filters.keyword} onChange={(value) => setFilters({ ...filters, keyword: value })} />
        <FilterSelect label="项目/产品" value={filters.productId} onChange={(value) => setFilters({ ...filters, productId: value, projectId: products.find((item) => item.value === value)?.projectId || "", moduleId: "" })} options={products} />
        <FilterSelect label="模块名称" value={filters.moduleId} onChange={(value) => setFilters({ ...filters, moduleId: value })} options={modules.map((item) => ({ value: String(item.id), label: item.name }))} />
        <FilterSelect label="方法" value={filters.method} onChange={(value) => setFilters({ ...filters, method: value })} options={["GET", "POST", "PUT", "DELETE", "PATCH"].map(option)} />
        <FilterSelect label="接口状态" value={filters.lifecycleStatus} onChange={(value) => setFilters({ ...filters, lifecycleStatus: value })} options={[{ value: "draft", label: "草稿" }, { value: "active", label: "启用" }, { value: "disabled", label: "停用" }, { value: "deprecated", label: "已废弃" }]} />
        <FilterSelect label="数据范围" value={filters.deletionState} onChange={(value) => setFilters({ ...filters, deletionState: value })} options={[{ value: "active", label: "正常数据" }, { value: "deleted", label: "回收站" }, { value: "all", label: "全部数据" }]} />
        <div className="api-filter-actions"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={() => { setFilters(initialFilters); setApplied(initialFilters); }} type="button">重置</button></div>
      </form>
      <div className="api-list-toolbar"><div className="api-tabs"><button className="active" type="button">接口定义</button></div><div><button className="primary-button compact-button" onClick={openCreate} type="button"><Plus size={14} />新增</button><button className="icon-text-button compact-button" disabled={busy} onClick={openCurlImport} type="button"><Upload size={14} />导入 cURL</button><button className="icon-text-button compact-button" disabled={!selected.length || !filters.productId || busy} onClick={batchMoveToCurrentProduct} type="button">移动到当前产品</button><button className="icon-text-button compact-button" disabled={!selected.length || busy} onClick={() => batchStatus("active")} type="button">批量启用</button><button className="icon-text-button compact-button" disabled={!selected.length || busy} onClick={() => batchStatus("disabled")} type="button">批量停用</button><button className="icon-text-button compact-button" disabled={!selected.length || busy} onClick={() => batchStatus("deprecated")} type="button">批量废弃</button><button className="danger-button compact-button" disabled={!selected.length || busy} onClick={() => removeRows(selected)} type="button">批量删除</button><button className="icon-text-button compact-button" onClick={reload} type="button"><RefreshCw size={14} /></button></div></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <StateBlock loading={loading} error={error}><TablePanel><DataTable columns={columns} rows={pageRows} fitContainer emptyText="暂无接口数据" /><PaginationBar page={page} pageSize={pageSize} total={total} totalPages={totalPages} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} /></TablePanel></StateBlock>
    </section>
    </div>
    {curlModal ? <CurlImportModal busy={busy} command={curlCommand} error={curlError} onChange={(value) => { setCurlCommand(value); if (curlError) setCurlError(""); }} onClose={() => setCurlModal(false)} onSubmit={importCurl} /> : null}
    {modal ? <InterfaceModal form={form} setForm={setForm} products={products} modules={modules} editing={editing} busy={busy} onClose={() => setModal(false)} onSubmit={save} /> : null}
  </div>;
}

const detailSections = [
  ["variables", "临时变量", "保存到当前接口，调试时优先级最高"],
  ["auth", "认证", "配置 Bearer、Basic 或 API Key"],
  ["headers", "请求头", "维护接口请求 Headers"],
  ["params", "参数", "维护查询参数"],
  ["body", "请求体", "维护 Body 配置"],
  ["jsonpath", "后置 JSONPath 提取", "提取响应 JSON 到缓存"],
  ["regex", "后置正则提取", "通过正则提取响应内容"],
  ["script", "后置脚本", "编写自定义响应处理脚本"],
  ["assertions", "结构化断言配置", "维护响应断言规则"],
  ["versions", "版本记录", "查看差异并回滚历史版本"]
];

const responseSections = [
  ["body", "响应体"],
  ["headers", "响应头"],
  ["logs", "执行日志"],
  ["variables", "提取结果"],
  ["assertions", "断言"],
  ["preview", "请求预览"],
  ["history", "历史"]
];

function InterfaceDetailWorkspace({ row, projectId, onBack }) {
  const saved = row.configuration || {};
  const [historyFilters, setHistoryFilters] = useState({ status: "", executorId: "", dateFrom: "", dateTo: "" });
  const { data: testObjectsData } = useAsyncData(
    () => configService.testObjects.list({ productId: row.productId, page: 1, pageSize: 200 }),
    [row.productId]
  );
  const testObjects = pageItems(testObjectsData);
  const { data: historyData, reload: reloadHistory } = useAsyncData(
    () => apiAutomationService.debug.history(row.id, {
      status: historyFilters.status,
      executorId: historyFilters.executorId,
      dateFrom: historyFilters.dateFrom ? new Date(historyFilters.dateFrom).toISOString() : "",
      dateTo: historyFilters.dateTo ? new Date(historyFilters.dateTo).toISOString() : "",
      limit: 100
    }),
    [row.id, historyFilters]
  );
  const debugHistory = Array.isArray(historyData) ? historyData : [];
  const { data: versionsData, reload: reloadVersions } = useAsyncData(
    () => apiAutomationService.versions.list(row.id),
    [row.id]
  );
  const versions = Array.isArray(versionsData) ? versionsData : [];
  const { data: defaultHeadersData } = useAsyncData(
    () => projectId ? apiAutomationService.requestHeaders.list({ projectId }) : Promise.resolve({ items: [] }),
    [projectId]
  );
  const defaultHeaders = useMemo(() => Object.fromEntries(
    pageItems(defaultHeadersData)
      .filter((item) => item.enabled && item.name)
      .map((item) => [item.name, item.value])
  ), [defaultHeadersData]);
  const [active, setActive] = useState("headers");
  const [responseActive, setResponseActive] = useState("body");
  const [config, setConfig] = useState({
    auth: JSON.stringify(saved.auth || { type: "none" }, null, 2),
    headers: saved.headers || "{\n  \"Content-Type\": \"application/json\"\n}",
    params: saved.params || "{\n  \n}",
    body: saved.body || "{\n  \n}",
    bodyType: saved.bodyType || "json",
    jsonpath: saved.jsonpath || "[]",
    regex: saved.regex || "[]",
    script: saved.script || "",
    assertions: saved.assertions || "[]"
  });
  const [parameterRows, setParameterRows] = useState(() => parseParameterRows(saved, row.path));
  const [method, setMethod] = useState(row.method || "GET");
  const [url, setUrl] = useState(row.path || "");
  const [currentRevision, setCurrentRevision] = useState(row.revision || 1);
  const [result, setResult] = useState(null);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [testObjectId, setTestObjectId] = useState("");
  const [preview, setPreview] = useState(null);
  const [temporaryVariableRows, setTemporaryVariableRows] = useState(() => parseTemporaryVariableRows(saved));
  const [savedTemporaryVariablesSignature, setSavedTemporaryVariablesSignature] = useState(() => stableConfigurationValue(parseTemporaryVariableRows(saved)));
  const [temporaryFiles, setTemporaryFiles] = useState(saved.temporaryFiles || []);
  const [savedSignature, setSavedSignature] = useState(() => editorConfigurationSignature(
    {
      auth: JSON.stringify(saved.auth || { type: "none" }, null, 2),
      headers: saved.headers || "{\n  \"Content-Type\": \"application/json\"\n}",
      body: saved.body || "{\n  \n}",
      bodyType: saved.bodyType || "json",
      jsonpath: saved.jsonpath || "[]",
      regex: saved.regex || "[]",
      script: saved.script || "",
      assertions: saved.assertions || "[]"
    },
    parseParameterRows(saved),
    saved.temporaryFiles || [],
    parseTemporaryVariableRows(saved)
  ));
  const [debugEvents, setDebugEvents] = useState([]);
  const [activeTaskId, setActiveTaskId] = useState("");
  const [historyDetail, setHistoryDetail] = useState(null);
  const [versionDiff, setVersionDiff] = useState(null);
  const currentSignature = editorConfigurationSignature(config, parameterRows, temporaryFiles, temporaryVariableRows);
  const configurationDirty = currentSignature !== savedSignature;
  const temporaryVariablesDirty = stableConfigurationValue(temporaryVariableRows) !== savedTemporaryVariablesSignature;

  function syncParametersFromUrl(nextUrl) {
    try {
      const parsed = new URL(nextUrl, "http://synapse.local");
      const incoming = new Map(parsed.searchParams.entries());
      setParameterRows((current) => {
        const retained = current.filter((item) => item.source !== "url" || incoming.has(item.key));
        const existingKeys = new Set(retained.map((item) => item.key));
        const next = retained.map((item) => incoming.has(item.key) ? { ...item, value: incoming.get(item.key), source: "url" } : item);
        for (const [key, value] of incoming) {
          if (!existingKeys.has(key)) next.push({ key, value, description: "", enabled: true, source: "url" });
        }
        return JSON.stringify(next) === JSON.stringify(current) ? current : next;
      });
    } catch {
      // URL 尚未输入完整时不打断编辑。
    }
  }

  function updateParametersAndUrl(nextRows) {
    setUrl((currentUrl) => syncUrlFromParameterRows(currentUrl, parameterRows, nextRows));
    setParameterRows(nextRows);
  }

  function copyBodyToTemporaryVariables() {
    const incomingRows = temporaryRowsFromRequestBody(config.body);
    const incomingNames = new Set(incomingRows.map((row) => row.key));
    setTemporaryVariableRows((current) => {
      const next = current.filter((row, index) => !incomingNames.has(row.key.trim()) || current.findIndex((item) => item.key.trim() === row.key.trim()) === index);
      incomingRows.forEach((incoming) => {
        const index = next.findIndex((row) => row.key.trim() === incoming.key);
        if (index >= 0) next[index] = { ...next[index], ...incoming, description: next[index].description || incoming.description };
        else next.push(incoming);
      });
      return next;
    });
    return incomingRows.length;
  }

  function configurationPayload() {
    const enabledParams = Object.fromEntries(parameterRows
      .filter((item) => item.enabled !== false && item.key.trim())
      .map((item) => [item.key.trim(), item.value]));
    return {
      ...saved,
      ...config,
      auth: JSON.parse(config.auth || "{\"type\":\"none\"}"),
      params: JSON.stringify(enabledParams, null, 2),
      paramsMeta: parameterRows,
      temporaryVariables: temporaryVariableRows,
      temporaryFiles
    };
  }

  async function uploadTemporaryFile(event) {
    const file = event.target.files?.[0];
    if (!file) return;
    setBusy(true); setNotice("");
    try {
      const uploaded = await apiAutomationService.tempFiles.upload(row.projectId, file);
      setTemporaryFiles((current) => [...current, uploaded]);
      setNotice(`临时文件 ${uploaded.originalName} 已上传，1 小时后自动过期。`);
    } catch (error) {
      setNotice(error.message || "上传临时文件失败");
    } finally {
      event.target.value = "";
      setBusy(false);
    }
  }

  async function removeTemporaryFile(file) {
    setBusy(true); setNotice("");
    try {
      await apiAutomationService.tempFiles.remove(file.id);
      setTemporaryFiles((current) => current.filter((item) => item.id !== file.id));
      setNotice("临时文件已删除。");
    } catch (error) {
      setNotice(error.message || "删除临时文件失败");
    } finally {
      setBusy(false);
    }
  }

  async function saveConfiguration() {
    setBusy(true); setNotice("");
    const signatureToSave = currentSignature;
    try {
      const payload = configurationPayload();
      const duplicateParam = parameterRows.find((item, index) => item.key.trim() && parameterRows.some((other, otherIndex) => otherIndex !== index && other.key.trim() === item.key.trim()));
      if (duplicateParam) throw new Error(`参数名 ${duplicateParam.key.trim()} 重复`);
      buildTemporaryVariables(temporaryVariableRows);
      for (const [key, label] of [["headers", "请求头"], ["jsonpath", "JSONPath 提取"], ["regex", "正则提取"], ["assertions", "断言"]]) {
        try {
          JSON.parse(config[key] || (["jsonpath", "regex", "assertions"].includes(key) ? "[]" : "{}"));
        } catch {
          throw new Error(`${label}配置不是合法 JSON`);
        }
      }
      await apiAutomationService.interfaces.saveConfiguration(row.id, { configuration: payload, revision: currentRevision });
      const refreshed = await apiAutomationService.interfaces.get(row.id);
      if (!isConfigurationPersisted(refreshed.configuration || {}, payload)) {
        throw new Error("保存后回读校验失败，请刷新后重试");
      }
      setCurrentRevision(refreshed.revision || currentRevision + 1);
      setSavedSignature(signatureToSave);
      setSavedTemporaryVariablesSignature(stableConfigurationValue(temporaryVariableRows));
      await reloadVersions();
      setNotice("接口配置已保存。");
    } catch (error) { setNotice(error.message || "保存接口配置失败"); } finally { setBusy(false); }
  }

  async function execute() {
    setBusy(true); setNotice("");
    const streamController = new AbortController();
    try {
      setDebugEvents([]);
      const run = await apiAutomationService.debug.start(row.id, previewPayload());
      setActiveTaskId(run.taskId);
      setNotice(`任务已下发至执行器 ${run.executorId}。`);
      let sequence = 0;
      let terminalEventReceived = false;
      const consumeEvents = async () => {
        while (!streamController.signal.aborted && !terminalEventReceived) {
          try {
            await apiAutomationService.debug.stream(run.taskId, sequence, (event) => {
              if (event.sequence <= sequence) return;
              sequence = event.sequence;
              terminalEventReceived = ["success", "failed", "canceled"].includes(event.status);
              setDebugEvents((current) => [...current, event]);
            }, streamController.signal);
          } catch (error) {
            if (error.name === "AbortError") return;
          }
          if (!terminalEventReceived && !streamController.signal.aborted) {
            await new Promise((resolve) => setTimeout(resolve, 300));
          }
        }
      };
      const eventStream = consumeEvents();
      const deadline = Date.now() + (Number(row.timeoutSeconds || 30) + 15) * 1000;
      while (Date.now() < deadline) {
        const current = await apiAutomationService.debug.get(run.taskId);
        if (["success", "failed", "canceled"].includes(current.status)) {
          streamController.abort();
          await eventStream;
          const taskResult = current.result || {};
          let response = {};
          try { response = JSON.parse(taskResult.output || "{}"); } catch { response = { body: taskResult.output || taskResult.error || "" }; }
          setResult({
            status: response.statusCode ?? "-",
            duration: response.durationMs || 0,
            ok: current.status === "success",
            headers: response.headers || {},
            body: response.body || taskResult.error || current.errorMessage || "",
            request: current.request,
            variables: response.extractedVariables || {},
            assertions: response.assertions || [],
            extractorErrors: response.extractorErrors || []
          });
          setResponseActive("body");
          await reloadHistory();
          setNotice(current.status === "success" ? "执行器调试完成。" : current.errorMessage || taskResult.error || "执行器调试失败。");
          setActiveTaskId("");
          return;
        }
        await new Promise((resolve) => setTimeout(resolve, 500));
      }
      throw new Error("等待执行器结果超时");
    } catch (error) {
      setResult({ status: "-", duration: 0, ok: false, headers: {}, body: error.message, request: { url, method } });
      setNotice(error.message || "执行器调试失败");
    } finally { streamController.abort(); setActiveTaskId(""); setBusy(false); }
  }

  async function cancelDebug() {
    if (!activeTaskId) return;
    try {
      await apiAutomationService.debug.cancel(activeTaskId);
      setNotice("正在取消执行器任务。");
    } catch (error) {
      setNotice(error.message || "取消调试失败");
    }
  }

  async function openHistoryDetail(id) {
    setBusy(true); setNotice("");
    try {
      setHistoryDetail(await apiAutomationService.debug.historyDetail(id));
    } catch (error) {
      setNotice(error.message || "读取调试历史失败");
    } finally {
      setBusy(false);
    }
  }

  async function compareVersion(version, targetVersion) {
    setBusy(true); setNotice("");
    try {
      setVersionDiff(await apiAutomationService.versions.diff(row.id, version, targetVersion));
    } catch (error) {
      setNotice(error.message || "读取版本差异失败");
    } finally {
      setBusy(false);
    }
  }

  async function restoreVersion(version) {
    if (!window.confirm(`确认回滚至 V${version} 吗？回滚会生成一个新版本。`)) return;
    setBusy(true); setNotice("");
    try {
      const restored = await apiAutomationService.versions.restore(row.id, version);
      await reloadVersions();
      setNotice(`已回滚并生成 V${restored.version}。返回列表后可加载最新配置。`);
    } catch (error) {
      setNotice(error.message || "回滚接口版本失败");
    } finally {
      setBusy(false);
    }
  }

  async function previewRequest() {
    setBusy(true); setNotice("");
    try {
      const response = await apiAutomationService.interfaces.preview(row.id, previewPayload());
      setPreview(response);
      setNotice("最终请求已构建，敏感请求头已脱敏。");
    } catch (error) {
      setNotice(error.message || "构建最终请求失败");
    } finally {
      setBusy(false);
    }
  }

  function previewPayload() {
    return {
      testObjectId: Number(testObjectId || 0),
      snapshot: {
        productId: row.productId, moduleId: row.moduleId, name: row.name, method, path: url,
        protocol: row.protocol, endpointType: row.endpointType, lifecycleStatus: row.lifecycleStatus,
        timeoutSeconds: row.timeoutSeconds, followRedirects: row.followRedirects,
        configuration: configurationPayload(), revision: row.revision
      },
      temporaryVariables: buildTemporaryVariables(temporaryVariableRows),
      temporaryHeaders: {}
    };
  }

  async function exportCurl() {
    setBusy(true); setNotice("");
    try {
      const response = await apiAutomationService.interfaces.exportCurl(row.id, previewPayload());
      await navigator.clipboard?.writeText(response.curl);
      setNotice("脱敏 cURL 已复制到剪贴板。");
    } catch (error) {
      setNotice(error.message || "导出 cURL 失败");
    } finally {
      setBusy(false);
    }
  }

  return <div className="api-detail-page api-debugger-page">
    <header className="api-detail-header"><div><span>接口配置工作台 / #{row.id}</span><h2>{row.name}</h2><p>维护请求配置、发送请求并查看响应结果</p></div><div><button className="primary-button compact-button" disabled={busy} onClick={() => { previewRequest(); setResponseActive("preview"); }} type="button"><Braces size={14} />预览请求</button><button className="icon-text-button compact-button" disabled={busy} onClick={exportCurl} type="button"><Clipboard size={14} />复制 cURL</button><button className="icon-text-button compact-button" onClick={onBack} type="button"><ArrowLeft size={14} />返回</button></div></header>
    {notice ? <div className="inline-notice">{notice}</div> : null}
    <div className="api-detail-layout">
      <div className="api-debugger-request-bar"><select aria-label="请求方法" className="text-input" value={method} onChange={(event) => setMethod(event.target.value)}>{["GET", "POST", "PUT", "DELETE", "PATCH"].map((item) => <option key={item}>{item}</option>)}</select><input aria-label="请求 URL" className="text-input api-debugger-url" value={url} onChange={(event) => { const nextUrl = event.target.value; setUrl(nextUrl); syncParametersFromUrl(nextUrl); }} /><select aria-label="测试环境" className="text-input" value={testObjectId} onChange={(event) => setTestObjectId(event.target.value)}><option value="">选择测试环境</option>{testObjects.map((item) => <option key={item.id} value={item.id}>{item.envName}</option>)}</select><span className={`api-save-state ${configurationDirty ? "dirty" : ""}`}>{configurationDirty ? "有未保存修改" : "配置已保存"}</span><button aria-label="保存配置" className="icon-text-button compact-button" disabled={busy || !configurationDirty} onClick={saveConfiguration} type="button"><Save size={14} />保存</button>{activeTaskId ? <button className="danger-button compact-button" onClick={cancelDebug} type="button">取消</button> : <button aria-label="执行接口" className="success-button api-send-button" disabled={busy} onClick={execute} type="button"><Play size={14} />{busy ? "发送中" : "发送"}</button>}</div>
      <div className="api-debugger-columns">
        <main className="api-detail-editor api-request-pane">
          <nav className="api-request-tabs">{detailSections.map(([key, title]) => <button className={active === key ? "active" : ""} key={key} onClick={() => setActive(key)} type="button">{title}{key === "variables" && temporaryVariableRows.length ? <small>{temporaryVariableRows.filter((item) => item.enabled !== false && item.key.trim()).length}</small> : null}{key === "params" && parameterRows.length ? <small>{parameterRows.filter((item) => item.enabled !== false && item.key.trim()).length}</small> : null}{key === "body" && config.bodyType !== "none" && config.body.trim() ? <i className="api-tab-status" aria-label="请求体已配置" /> : null}</button>)}</nav>
          <div className="api-request-pane-content">
            {active === "body" || active === "variables" ? null : <div className="api-editor-tip">{active === "headers" ? `已加载 ${Object.keys(defaultHeaders).length} 个项目默认请求头；接口内同名请求头优先。` : "配置修改后请点击顶部保存；发送时使用当前编辑内容。"}</div>}
            {active === "body" && config.bodyType === "form_data" ? <div className="api-temp-files"><label className="icon-text-button compact-button">上传临时文件<input disabled={busy} hidden onChange={uploadTemporaryFile} type="file" /></label>{temporaryFiles.map((file) => <span key={file.id}>{file.originalName}（{Math.ceil(file.sizeBytes / 1024)} KB）<button className="link-button danger-link" onClick={() => removeTemporaryFile(file)} type="button">删除</button></span>)}</div> : null}
            {active === "variables" ? <TemporaryVariableEditor rows={temporaryVariableRows} dirty={temporaryVariablesDirty} saving={busy} onChange={setTemporaryVariableRows} onSave={saveConfiguration} /> : active === "body" ? <RequestBodyEditor bodyType={config.bodyType} value={config.body} onBodyTypeChange={(bodyType) => setConfig({ ...config, bodyType })} onChange={(body) => setConfig({ ...config, body })} onCopyToVariables={copyBodyToTemporaryVariables} /> : active === "params" ? <ParameterTableEditor rows={parameterRows} onChange={updateParametersAndUrl} /> : active === "jsonpath" || active === "regex" ? <ExtractorRuleEditor type={active} value={config[active]} dirty={configurationDirty} saving={busy} onChange={(value) => setConfig({ ...config, [active]: value })} onSave={saveConfiguration} /> : active === "assertions" ? <AssertionRuleEditor value={config.assertions} onChange={(value) => setConfig({ ...config, assertions: value })} /> : active === "versions" ? <VersionPanel versions={versions} diff={versionDiff} busy={busy} onCompare={compareVersion} onRestore={restoreVersion} /> : <textarea className="api-config-editor" spellCheck="false" value={config[active]} onChange={(event) => setConfig({ ...config, [active]: event.target.value })} />}
          </div>
        </main>
        <aside className="api-result-panel api-response-pane">
          <div className="api-response-toolbar"><nav>{responseSections.map(([key, title]) => <button className={responseActive === key ? "active" : ""} key={key} onClick={() => setResponseActive(key)} type="button">{title}{key === "headers" && result ? <small>{Object.keys(result.headers || {}).length}</small> : null}</button>)}</nav><div className="api-response-metrics"><span>{result ? `${(result.duration / 1000).toFixed(2)}s` : "--"}</span><strong className={result?.ok ? "success" : result ? "danger" : ""}>{result?.status ?? "---"}</strong></div></div>
          <div className="api-response-content">
            {responseActive === "body" ? <DebuggerResponseView label="响应体" value={prettyResponseBody(result?.body || "")} empty="发送请求后在此查看响应体" /> : null}
            {responseActive === "headers" ? <DebuggerResponseView label="响应头" value={result ? JSON.stringify(result.headers, null, 2) : ""} empty="发送请求后在此查看响应头" /> : null}
            {responseActive === "logs" ? <DebuggerResponseView label="执行日志" value={debugEvents.map((event) => `[${event.progress}%] ${event.message}`).join("\n")} empty="发送请求后在此查看实时执行日志" /> : null}
            {responseActive === "variables" ? <DebuggerResponseView label="提取结果" value={result ? JSON.stringify({ variables: result.variables || {}, errors: result.extractorErrors || [] }, null, 2) : ""} empty="执行提取规则后在此查看结果" /> : null}
            {responseActive === "assertions" ? <DebuggerResponseView label="断言结果" value={result ? JSON.stringify(result.assertions || [], null, 2) : ""} empty="执行断言后在此查看结果" /> : null}
            {responseActive === "preview" ? <DebuggerResponseView label="最终请求" value={preview || result?.request ? JSON.stringify(preview || result?.request, null, 2) : ""} empty="点击预览请求后在此查看最终请求" /> : null}
            {responseActive === "history" ? <div className="api-response-history"><div className="api-history-filters"><select value={historyFilters.status} onChange={(event) => setHistoryFilters({ ...historyFilters, status: event.target.value })}><option value="">全部状态</option><option value="success">成功</option><option value="failed">失败</option><option value="canceled">已取消</option></select><input placeholder="执行器 ID" value={historyFilters.executorId} onChange={(event) => setHistoryFilters({ ...historyFilters, executorId: event.target.value })} /><input type="datetime-local" value={historyFilters.dateFrom} onChange={(event) => setHistoryFilters({ ...historyFilters, dateFrom: event.target.value })} /><input type="datetime-local" value={historyFilters.dateTo} onChange={(event) => setHistoryFilters({ ...historyFilters, dateTo: event.target.value })} /></div><div className="api-debug-history">{debugHistory.map((item) => <button key={item.id} onClick={() => openHistoryDetail(item.id)} type="button"><b className={item.status === "success" ? "success" : item.status === "failed" ? "danger" : ""}>{item.status}</b><small>{new Date(item.createdAt).toLocaleString()}</small><code>{item.executorId}</code></button>)}</div>{historyDetail ? <DebuggerResponseView label={`历史详情 #${historyDetail.id}`} value={JSON.stringify(historyDetail, null, 2)} /> : null}</div> : null}
          </div>
        </aside>
      </div>
    </div>
  </div>;
}

const emptyHeaderForm = { projectId: "", name: "", value: "", description: "", enabled: true, sensitive: false };

function RequestHeaderManagementPage() {
  const [filters, setFilters] = useState({ projectId: "", keyword: "" });
  const [applied, setApplied] = useState({ projectId: "", keyword: "" });
  const [form, setForm] = useState(emptyHeaderForm);
  const [editing, setEditing] = useState(null);
  const [modal, setModal] = useState(false);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const { data: projectsData } = useAsyncData(() => configService.projects.list({ page: 1, pageSize: 200 }), []);
  const projects = useMemo(() => pageItems(projectsData).map((item) => ({ value: String(item.id), label: item.name })), [projectsData]);
  const { data, loading, error, reload } = useAsyncData(
    () => applied.projectId ? apiAutomationService.requestHeaders.list({ projectId: applied.projectId, keyword: applied.keyword }) : Promise.resolve({ items: [] }),
    [applied.projectId, applied.keyword]
  );
  const rows = pageItems(data);

  function openCreate() {
    setEditing(null);
    setForm({ ...emptyHeaderForm, projectId: applied.projectId });
    setNotice("");
    setModal(true);
  }

  function openEdit(row) {
    setEditing(row);
    setForm({ projectId: String(row.projectId), name: row.name, value: row.value, description: row.description, enabled: row.enabled, sensitive: row.sensitive });
    setNotice("");
    setModal(true);
  }

  async function save(event) {
    event.preventDefault();
    if (!form.projectId || !form.name.trim()) {
      setNotice("项目和请求头名称不能为空。");
      return;
    }
    const duplicate = rows.some((item) => item.id !== editing?.id && String(item.projectId) === form.projectId && item.name.toLowerCase() === form.name.trim().toLowerCase());
    if (duplicate) {
      setNotice("该项目已存在同名请求头。");
      return;
    }
    const payload = { ...form, projectId: Number(form.projectId) };
    setBusy(true);
    try {
      if (editing) await apiAutomationService.requestHeaders.update(editing.id, payload);
      else await apiAutomationService.requestHeaders.create(payload);
      await reload();
      setModal(false);
      setNotice(editing ? "请求头已更新。" : "请求头已新增。");
    } catch (err) {
      setNotice(err.message || "保存请求头失败");
    } finally {
      setBusy(false);
    }
  }

  async function remove(row) {
    if (!window.confirm(`确认删除请求头 ${row.name} 吗？`)) return;
    setBusy(true);
    try {
      await apiAutomationService.requestHeaders.remove(row.id);
      await reload();
      setNotice("请求头已删除。");
    } catch (err) {
      setNotice(err.message || "删除请求头失败");
    } finally {
      setBusy(false);
    }
  }

  const columns = [
    { key: "id", title: "ID" },
    { key: "projectName", title: "所属项目" },
    { key: "name", title: "请求头名称", render: (row) => <code>{row.name}</code> },
    { key: "value", title: "默认值", render: (row) => <code>{row.value || "-"}</code> },
    { key: "description", title: "说明", render: (row) => row.description || "-" },
    { key: "enabled", title: "状态", render: (row) => <span className={`status-badge ${row.enabled ? "status-passed" : "status-failed"}`}>{row.enabled ? "启用" : "停用"}</span> },
    { key: "operations", title: "操作", render: (row) => <div className="action-links"><button className="link-button" onClick={() => openEdit(row)} type="button">编辑</button><button className="link-button danger-link" onClick={() => remove(row)} type="button">删除</button></div> }
  ];

  return <div className="section-stack api-interface-page">
    <PageHeader title="请求头管理" description="按项目维护接口执行时自动携带的默认请求头" />
    <section className="resource-panel">
      <form className="api-filter-grid api-header-filter" onSubmit={(event) => { event.preventDefault(); setApplied(filters); }}>
        <FilterSelect label="所属项目" value={filters.projectId} onChange={(value) => setFilters({ ...filters, projectId: value })} options={projects} />
        <FilterInput label="请求头名称" value={filters.keyword} onChange={(value) => setFilters({ ...filters, keyword: value })} />
        <div className="api-filter-actions"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={() => { setFilters({ projectId: "", keyword: "" }); setApplied({ projectId: "", keyword: "" }); }} type="button">重置</button></div>
      </form>
      <div className="api-list-toolbar"><div><strong>项目默认请求头</strong></div><div><button className="primary-button compact-button" onClick={openCreate} type="button"><Plus size={14} />新增请求头</button><button className="icon-text-button compact-button" onClick={reload} type="button"><RefreshCw size={14} /></button></div></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <StateBlock loading={loading} error={error}><TablePanel><DataTable columns={columns} rows={rows} emptyText="暂无项目默认请求头" /></TablePanel></StateBlock>
    </section>
    {modal ? <div className="modal-backdrop"><form className="modal-card api-interface-modal" onSubmit={save}><div className="modal-header"><strong>{editing ? "编辑请求头" : "新增请求头"}</strong><button className="modal-close" onClick={() => setModal(false)} type="button">×</button></div><div className="api-interface-form">
      <FilterSelect required label="所属项目" value={form.projectId} onChange={(value) => setForm({ ...form, projectId: value })} options={projects} />
      <FilterInput required label="请求头名称" value={form.name} onChange={(value) => setForm({ ...form, name: value })} placeholder="例如 Authorization" />
      <FilterInput label="默认值" value={form.value} onChange={(value) => setForm({ ...form, value })} placeholder="例如 Bearer ${token}" />
      <FilterInput label="说明" value={form.description} onChange={(value) => setForm({ ...form, description: value })} />
      <FilterSelect label="状态" value={form.enabled ? "true" : "false"} onChange={(value) => setForm({ ...form, enabled: value === "true" })} options={[{ value: "true", label: "启用" }, { value: "false", label: "停用" }]} />
      <FilterSelect label="敏感信息" value={form.sensitive ? "true" : "false"} onChange={(value) => setForm({ ...form, sensitive: value === "true" })} options={[{ value: "false", label: "普通" }, { value: "true", label: "敏感" }]} />
    </div><div className="modal-actions"><button className="icon-text-button compact-button" onClick={() => setModal(false)} type="button">取消</button><button className="primary-button compact-button" disabled={busy} type="submit">{busy ? "保存中" : "保存"}</button></div></form></div> : null}
  </div>;
}

function parseParameterRows(configuration, requestUrl = "") {
  let rows;
  if (Array.isArray(configuration.paramsMeta)) {
    rows = configuration.paramsMeta
      .filter((item) => item && (item.key || item.value || item.description))
      .map((item) => ({
        key: String(item.key || ""),
        value: String(item.value ?? ""),
        description: String(item.description || ""),
        enabled: item.enabled !== false,
        source: item.source === "url" ? "url" : "manual"
      }));
  } else {
    try {
      const source = typeof configuration.params === "string" ? JSON.parse(configuration.params || "{}") : configuration.params || {};
      rows = Object.entries(source).map(([key, value]) => ({ key, value: String(value ?? ""), description: "", enabled: true }));
    } catch {
      rows = [];
    }
  }
  try {
    const parsed = new URL(requestUrl, "http://synapse.local");
    const incoming = new Map(parsed.searchParams.entries());
    const existingKeys = new Set(rows.map((item) => item.key));
    rows = rows.map((item) => incoming.has(item.key) ? { ...item, value: incoming.get(item.key), source: "url" } : item);
    for (const [key, value] of incoming) {
      if (!existingKeys.has(key)) rows.push({ key, value, description: "", enabled: true, source: "url" });
    }
  } catch {
    // 兼容尚未形成有效 URL 的历史数据。
  }
  return rows;
}

export function parseTemporaryVariableRows(configuration) {
  if (Array.isArray(configuration.temporaryVariables)) {
    return configuration.temporaryVariables
      .filter((item) => item && (item.key || item.value || item.description))
      .map((item) => ({
        key: String(item.key || ""),
        type: ["string", "number", "boolean", "json"].includes(item.type) ? item.type : "string",
        value: String(item.value ?? ""),
        description: String(item.description || ""),
        enabled: item.enabled !== false
      }));
  }
  if (configuration.temporaryVariables && typeof configuration.temporaryVariables === "object") {
    return temporaryRowsFromRequestBody(JSON.stringify(configuration.temporaryVariables));
  }
  return [];
}

function syncUrlFromParameterRows(currentUrl, previousRows, nextRows) {
  const hashIndex = currentUrl.indexOf("#");
  const hash = hashIndex >= 0 ? currentUrl.slice(hashIndex) : "";
  const withoutHash = hashIndex >= 0 ? currentUrl.slice(0, hashIndex) : currentUrl;
  const queryIndex = withoutHash.indexOf("?");
  const base = queryIndex >= 0 ? withoutHash.slice(0, queryIndex) : withoutHash;
  const search = new URLSearchParams(queryIndex >= 0 ? withoutHash.slice(queryIndex + 1) : "");
  for (const item of previousRows) {
    const key = item.key.trim();
    if (key) search.delete(key);
  }
  for (const item of nextRows) {
    const key = item.key.trim();
    if (key && item.enabled !== false) search.set(key, item.value);
  }
  const query = search.toString().replace(/%24%7B/gi, "${").replace(/%7D/gi, "}");
  return `${base}${query ? `?${query}` : ""}${hash}`;
}

function isConfigurationPersisted(actual, expected) {
  const keys = ["headers", "params", "paramsMeta", "body", "bodyType", "jsonpath", "regex", "script", "assertions", "temporaryVariables", "temporaryFiles"];
  return keys.every((key) => stableConfigurationValue(actual[key] ?? null) === stableConfigurationValue(expected[key] ?? null));
}

function editorConfigurationSignature(config, parameterRows, temporaryFiles, temporaryVariables) {
  const { params: _legacyParams, ...editableConfig } = config;
  return stableConfigurationValue({ config: editableConfig, parameterRows, temporaryFiles, temporaryVariables });
}

function stableConfigurationValue(value) {
  if (Array.isArray(value)) return `[${value.map(stableConfigurationValue).join(",")}]`;
  if (value && typeof value === "object") {
    return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableConfigurationValue(value[key])}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

function temporaryRowsFromRequestBody(body) {
  let parsed;
  try { parsed = JSON.parse(body || "{}"); } catch { throw new Error("请求体不是合法 JSON，无法复制"); }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") throw new Error("请求体必须是 JSON 对象");
  const rows = Object.entries(parsed).map(([key, value]) => {
    if (typeof value === "number") return { key, type: "number", value: String(value), description: "来自请求体", enabled: true };
    if (typeof value === "boolean") return { key, type: "boolean", value: String(value), description: "来自请求体", enabled: true };
    if (value === null || typeof value === "object") return { key, type: "json", value: JSON.stringify(value), description: "来自请求体", enabled: true };
    return { key, type: "string", value: String(value), description: "来自请求体", enabled: true };
  });
  if (!rows.length) throw new Error("请求体中没有可复制的字段");
  return rows;
}

function buildTemporaryVariables(rows) {
  const result = {};
  const names = new Set();
  rows.filter((row) => row.enabled !== false && row.key.trim()).forEach((row) => {
    const name = row.key.trim();
    if (names.has(name)) throw new Error(`临时变量名不能重复：${name}`);
    names.add(name);
    if (row.type === "number") {
      if (!row.value.trim() || Number.isNaN(Number(row.value))) throw new Error(`临时变量 ${name} 不是有效数字`);
      result[name] = Number(row.value);
    } else if (row.type === "boolean") {
      result[name] = row.value === "true";
    } else if (row.type === "json") {
      try { result[name] = JSON.parse(row.value); } catch { throw new Error(`临时变量 ${name} 不是合法 JSON`); }
    } else {
      result[name] = row.value;
    }
  });
  return result;
}

function TemporaryVariableEditor({ rows, dirty, saving, onChange, onSave }) {
  const updateRow = (index, key, value) => {
    if (index === rows.length) {
      onChange([...rows, { key: "", type: "string", value: "", description: "", enabled: true, [key]: value }]);
      return;
    }
    const updated = { ...rows[index], [key]: value };
    if (!updated.key.trim() && !updated.value.trim() && !updated.description.trim()) {
      onChange(rows.filter((_, itemIndex) => itemIndex !== index));
      return;
    }
    onChange(rows.map((row, itemIndex) => itemIndex === index ? updated : row));
  };
  const duplicateKeys = new Set(rows
    .map((row) => row.key.trim())
    .filter((key, _index, values) => key && values.indexOf(key) !== values.lastIndexOf(key)));
  const visibleRows = [...rows, { key: "", type: "string", value: "", description: "", enabled: true, placeholder: true }];
  const enabledCount = rows.filter((row) => row.enabled !== false && row.key.trim()).length;

  return <section className="api-variable-editor">
    <header className="api-variable-toolbar">
      <div><strong>接口调试变量</strong><span>保存到当前接口，用于预览、发送和 cURL 导出，优先级最高</span></div>
      <div className="api-variable-toolbar-actions"><span className={`api-save-state ${dirty ? "dirty" : ""}`}>{dirty ? "有未保存修改" : "已保存"}</span><button aria-label="保存临时变量" className="icon-text-button compact-button" disabled={saving || !dirty} onClick={onSave} type="button"><Save size={13} />{saving ? "保存中" : "保存"}</button>{rows.length ? <button className="link-button danger-link" onClick={() => onChange([])} type="button">清空变量</button> : null}</div>
    </header>
    <div className="api-variable-table" role="table" aria-label="临时变量列表">
      <div className="api-variable-table-head" role="row"><span>启用</span><span>变量名</span><span>类型</span><span>值</span><span>说明</span><span>操作</span></div>
      {visibleRows.map((row, index) => <div className={`api-variable-row ${row.placeholder ? "placeholder" : ""} ${duplicateKeys.has(row.key.trim()) ? "invalid" : ""}`} role="row" key={index}>
        <input aria-label={`启用临时变量 ${index + 1}`} checked={row.enabled !== false} disabled={row.placeholder} onChange={(event) => updateRow(index, "enabled", event.target.checked)} type="checkbox" />
        <input aria-label={`临时变量名 ${index + 1}`} onChange={(event) => updateRow(index, "key", event.target.value)} placeholder={row.placeholder ? "变量名" : ""} value={row.key} />
        <select aria-label={`临时变量类型 ${index + 1}`} onChange={(event) => updateRow(index, "type", event.target.value)} value={row.type || "string"}><option value="string">String</option><option value="number">Number</option><option value="boolean">Boolean</option><option value="json">JSON</option></select>
        {row.type === "boolean"
          ? <select aria-label={`临时变量值 ${index + 1}`} onChange={(event) => updateRow(index, "value", event.target.value)} value={row.value || "true"}><option value="true">true</option><option value="false">false</option></select>
          : <input aria-label={`临时变量值 ${index + 1}`} onChange={(event) => updateRow(index, "value", event.target.value)} placeholder={row.type === "json" ? "{\"key\":\"value\"}" : row.placeholder ? "变量值" : ""} value={row.value} />}
        <input aria-label={`临时变量说明 ${index + 1}`} onChange={(event) => updateRow(index, "description", event.target.value)} placeholder={row.placeholder ? "可选说明" : ""} value={row.description} />
        {row.placeholder ? <span /> : <button className="link-button danger-link" onClick={() => onChange(rows.filter((_, itemIndex) => itemIndex !== index))} type="button">删除</button>}
      </div>)}
    </div>
    <footer><span>共 {rows.length} 个变量，已启用 {enabledCount} 个</span>{duplicateKeys.size ? <strong>变量名不能重复</strong> : <span>引用格式：{"${variable_name}"}</span>}</footer>
  </section>;
}

function RequestBodyEditor({ bodyType, value, onBodyTypeChange, onChange, onCopyToVariables }) {
  const [scrollTop, setScrollTop] = useState(0);
  const [validation, setValidation] = useState(null);
  const content = String(value || "");
  const lineCount = Math.max(1, content.split("\n").length);
  const isJson = bodyType === "json";

  function transformJson(compact) {
    try {
      const parsed = JSON.parse(content || "{}");
      onChange(JSON.stringify(parsed, null, compact ? 0 : 2));
      setValidation({ type: "success", text: compact ? "JSON 已压缩" : "JSON 格式正确" });
    } catch {
      setValidation({ type: "error", text: "JSON 格式错误，请检查后重试" });
    }
  }

  function copyToVariables() {
    try {
      const count = onCopyToVariables();
      setValidation({ type: "success", text: `已复制 ${count} 个字段到临时变量` });
    } catch (error) {
      setValidation({ type: "error", text: error.message });
    }
  }

  function handleKeyDown(event) {
    if (event.key !== "Tab") return;
    event.preventDefault();
    const input = event.currentTarget;
    const start = input.selectionStart;
    const nextValue = `${content.slice(0, start)}  ${content.slice(input.selectionEnd)}`;
    onChange(nextValue);
    requestAnimationFrame(() => {
      input.selectionStart = start + 2;
      input.selectionEnd = start + 2;
    });
  }

  return <section className="api-body-editor">
    <header className="api-body-toolbar">
      <label>
        <span>数据类型</span>
        <select aria-label="请求体数据类型" value={bodyType} onChange={(event) => { onBodyTypeChange(event.target.value); setValidation(null); }}>
          <option value="none">无</option>
          <option value="json">JSON</option>
          <option value="form_data">form-data</option>
          <option value="urlencoded">x-www-form-urlencoded</option>
          <option value="raw">Raw 文本</option>
        </select>
      </label>
      <div className="api-body-tools">
        {validation ? <span className={validation.type}>{validation.text}</span> : <span>{lineCount} 行</span>}
        <button aria-label="复制到临时变量" disabled={!isJson} onClick={copyToVariables} title="将 JSON 顶层字段复制到临时变量" type="button"><Clipboard size={14} />复制到临时变量</button>
        <button aria-label="格式化 JSON" disabled={!isJson} onClick={() => transformJson(false)} title="格式化 JSON" type="button"><AlignLeft size={14} />格式化</button>
        <button aria-label="压缩 JSON" disabled={!isJson} onClick={() => transformJson(true)} title="压缩 JSON" type="button"><Minimize2 size={14} />压缩</button>
      </div>
    </header>
    {bodyType === "none"
      ? <div className="api-body-empty"><Braces size={24} /><strong>当前请求不发送 Body</strong><span>选择 JSON、表单或 Raw 文本后开始编辑</span></div>
      : <div className={`api-code-editor ${validation?.type === "error" ? "invalid" : ""}`}>
          <div className="api-code-gutter" aria-hidden="true"><div style={{ transform: `translateY(-${scrollTop}px)` }}>{Array.from({ length: lineCount }, (_, index) => <span key={index}>{index + 1}</span>)}</div></div>
          <textarea aria-label="请求体内容" onChange={(event) => { onChange(event.target.value); setValidation(null); }} onKeyDown={handleKeyDown} onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)} placeholder={isJson ? "{\n  \"key\": \"value\"\n}" : "请输入请求体内容"} spellCheck="false" value={content} />
        </div>}
  </section>;
}

function ParameterTableEditor({ rows, onChange }) {
  const updateRow = (index, key, value) => {
    if (index === rows.length) {
      onChange([...rows, { key: "", value: "", description: "", enabled: true, [key]: value }]);
      return;
    }
    const updated = { ...rows[index], [key]: value };
    if (!updated.key.trim() && !updated.value.trim() && !updated.description.trim()) {
      onChange(rows.filter((_, itemIndex) => itemIndex !== index));
      return;
    }
    onChange(rows.map((row, itemIndex) => itemIndex === index ? updated : row));
  };
  const duplicateKeys = new Set(rows
    .map((row) => row.key.trim())
    .filter((key, _index, values) => key && values.indexOf(key) !== values.lastIndexOf(key)));
  const visibleRows = [...rows, { key: "", value: "", description: "", enabled: true, placeholder: true }];
  return <div className="api-parameter-editor">
    <div className="api-parameter-toolbar"><div><strong>查询参数</strong><span>启用的参数会自动拼接到请求 URL</span></div>{rows.length ? <button className="link-button danger-link" onClick={() => onChange([])} type="button">清空参数</button> : null}</div>
    <div className="api-parameter-table">
      <div className="api-parameter-table-head"><span>启用</span><span>参数名</span><span>参数值</span><span>说明</span><span></span></div>
      {visibleRows.map((row, index) => {
        const duplicate = duplicateKeys.has(row.key.trim());
        return <div className={`api-parameter-row ${row.enabled === false ? "disabled" : ""} ${row.placeholder ? "placeholder" : ""}`} key={index}>
          <label><input aria-label={`启用参数 ${index + 1}`} checked={row.enabled !== false} disabled={row.placeholder} onChange={(event) => updateRow(index, "enabled", event.target.checked)} type="checkbox" /></label>
          <div><input aria-label={`参数名 ${index + 1}`} className={duplicate ? "invalid" : ""} onChange={(event) => updateRow(index, "key", event.target.value)} placeholder={row.placeholder ? "键" : "参数名"} spellCheck="false" value={row.key} />{duplicate ? <small>参数名重复</small> : null}</div>
          <input aria-label={`参数值 ${index + 1}`} onChange={(event) => updateRow(index, "value", event.target.value)} placeholder={row.placeholder ? "值，支持 ${变量名}" : "参数值"} spellCheck="false" value={row.value} />
          <input aria-label={`参数说明 ${index + 1}`} onChange={(event) => updateRow(index, "description", event.target.value)} placeholder="可选说明" value={row.description} />
          {row.placeholder ? <span></span> : <button aria-label={`删除参数 ${row.key || index + 1}`} onClick={() => onChange(rows.filter((_, itemIndex) => itemIndex !== index))} type="button">−</button>}
        </div>;
      })}
    </div>
    <div className="api-parameter-footer"><span>共 {rows.length} 个参数，已启用 {rows.filter((item) => item.enabled !== false && item.key.trim()).length} 个</span><span>输入最后一行可继续添加</span></div>
  </div>;
}

function ExtractorRuleEditor({ type, value, dirty, saving, onChange, onSave }) {
  const rules = parseRuleArray(value);
  const update = (next) => onChange(JSON.stringify(next, null, 2));
  const add = () => update([...rules, { name: "", expression: type === "jsonpath" ? "$." : "", required: true, enabled: true, defaultValue: "", sensitive: false }]);
  if (type === "jsonpath") {
    const duplicate = (index) => update([...rules.slice(0, index + 1), { ...rules[index], name: rules[index].name ? `${rules[index].name}_copy` : "" }, ...rules.slice(index + 1)]);
    return <div className="api-rule-editor api-jsonpath-editor">
      <div className="api-rule-toolbar">
        <div><strong>响应字段映射</strong><span>从响应 JSON 中提取字段，供后续请求和断言引用</span></div>
        <div className="api-rule-toolbar-actions"><span className={`api-save-state ${dirty ? "dirty" : ""}`}>{dirty ? "有未保存修改" : "已保存"}</span><button aria-label="保存 JSONPath 提取" className="icon-text-button compact-button" disabled={saving || !dirty} onClick={onSave} type="button"><Save size={13} />{saving ? "保存中" : "保存"}</button><button className="primary-button compact-button" onClick={add} type="button"><Plus size={13} />新增提取规则</button></div>
      </div>
      <div className="api-jsonpath-guide"><Braces size={16} /><span>支持属性和数组下标，例如 <code>$.data.token</code>、<code>$.items[0].id</code>、<code>$['access-token']</code></span></div>
      {rules.length ? <div className="api-jsonpath-rule-list">{rules.map((rule, index) => {
        const pathError = validateJsonPath(rule.expression);
        const nameError = validateVariableName(rule.name, rules, index);
        return <article className={`api-jsonpath-rule-card ${rule.enabled === false ? "disabled" : ""}`} key={index}>
          <header><div><span className="api-rule-index">{String(index + 1).padStart(2, "0")}</span><div><strong>{rule.name || "未命名变量"}</strong><small>{rule.name ? `后续可使用 \${${rule.name}}` : "填写变量名后生成引用方式"}</small></div></div><div><button className="link-button" onClick={() => duplicate(index)} type="button">复制</button><button className="link-button danger-link" onClick={() => update(rules.filter((_, itemIndex) => itemIndex !== index))} type="button">删除</button></div></header>
          <div className="api-jsonpath-primary-fields">
            <label><span>变量名 <b>*</b></span><input className={`text-input ${nameError ? "invalid" : ""}`} onChange={(event) => updateRule(rules, index, "name", event.target.value, update)} placeholder="例如 access_token" value={rule.name || ""} />{nameError ? <small className="field-error">{nameError}</small> : <small>仅支持字母、数字和下划线，不能以数字开头</small>}</label>
            <label><span>JSONPath <b>*</b></span><div className={`api-jsonpath-expression ${pathError ? "invalid" : ""}`}><Braces size={14} /><input onChange={(event) => updateRule(rules, index, "expression", event.target.value, update)} placeholder="$.data.token" spellCheck="false" value={rule.expression || ""} /></div>{pathError ? <small className="field-error">{pathError}</small> : <small>路径必须从响应根节点 $ 开始</small>}</label>
          </div>
          <div className="api-jsonpath-secondary-fields">
            <label><span>提取失败时的默认值</span><input className="text-input" disabled={rule.required !== false} onChange={(event) => updateRule(rules, index, "defaultValue", event.target.value, update)} placeholder={rule.required !== false ? "必填规则不使用默认值" : "可选"} value={rule.defaultValue ?? ""} /></label>
            <div className="api-jsonpath-switches"><label><input checked={rule.required !== false} onChange={(event) => updateRule(rules, index, "required", event.target.checked, update)} type="checkbox" /><span><strong>必须提取</strong><small>失败时终止本次调试</small></span></label><label><input checked={rule.sensitive === true} onChange={(event) => updateRule(rules, index, "sensitive", event.target.checked, update)} type="checkbox" /><span><strong>敏感变量</strong><small>日志中隐藏实际值</small></span></label><label><input checked={rule.enabled !== false} onChange={(event) => updateRule(rules, index, "enabled", event.target.checked, update)} type="checkbox" /><span><strong>启用规则</strong><small>参与接口调试执行</small></span></label></div>
          </div>
        </article>;
      })}</div> : <div className="api-rule-empty api-jsonpath-empty"><span><Braces size={20} /></span><strong>还没有提取规则</strong><p>新增规则，将响应字段保存为后续步骤可引用的变量。</p><button className="primary-button compact-button" onClick={add} type="button"><Plus size={13} />新增第一条规则</button></div>}
    </div>;
  }
  return <div className="api-rule-editor"><div className="api-rule-toolbar"><span>{type === "jsonpath" ? "JSONPath" : "正则"}提取规则</span><button className="primary-button compact-button" onClick={add} type="button"><Plus size={13} />新增规则</button></div>{rules.length ? rules.map((rule, index) => <div className="api-rule-row" key={index}>
    <input className="text-input" onChange={(event) => updateRule(rules, index, "name", event.target.value, update)} placeholder="变量名称" value={rule.name || ""} />
    <input className="text-input rule-expression" onChange={(event) => updateRule(rules, index, "expression", event.target.value, update)} placeholder={type === "jsonpath" ? "$.data.id" : "token=(.+)"} value={rule.expression || ""} />
    <label><input checked={rule.required !== false} onChange={(event) => updateRule(rules, index, "required", event.target.checked, update)} type="checkbox" />必填</label>
    <label><input checked={rule.sensitive === true} onChange={(event) => updateRule(rules, index, "sensitive", event.target.checked, update)} type="checkbox" />敏感</label>
    <label><input checked={rule.enabled !== false} onChange={(event) => updateRule(rules, index, "enabled", event.target.checked, update)} type="checkbox" />启用</label>
    <button className="link-button danger-link" onClick={() => update(rules.filter((_, itemIndex) => itemIndex !== index))} type="button">删除</button>
  </div>) : <div className="api-rule-empty">暂无提取规则</div>}</div>;
}

function validateJsonPath(expression) {
  const value = String(expression || "").trim();
  if (!value) return "请输入 JSONPath";
  if (!value.startsWith("$")) return "JSONPath 必须以 $ 开头";
  if (value === "$") return "";
  const tokens = value.slice(1).match(/(?:\.[A-Za-z_][A-Za-z0-9_-]*|\[\d+\]|\['[^']+'\]|\["[^"]+"\])/g) || [];
  return `$${tokens.join("")}` === value ? "" : "当前支持属性访问、数组下标和方括号属性";
}

function validateVariableName(name, rules, index) {
  const value = String(name || "").trim();
  if (!value) return "请输入变量名";
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(value)) return "变量名格式不正确";
  if (rules.some((rule, itemIndex) => itemIndex !== index && String(rule.name || "").trim() === value)) return "变量名不能重复";
  return "";
}

function AssertionRuleEditor({ value, onChange }) {
  const rules = parseRuleArray(value);
  const update = (next) => onChange(JSON.stringify(next, null, 2));
  const add = () => update([...rules, { type: "status", expression: "", operator: "equals", expected: "200", description: "", enabled: true }]);
  return <div className="api-rule-editor"><div className="api-rule-toolbar"><span>所有启用断言都会执行</span><button className="primary-button compact-button" onClick={add} type="button"><Plus size={13} />新增断言</button></div>{rules.length ? rules.map((rule, index) => <div className="api-rule-row assertion" key={index}>
    <select className="text-input" onChange={(event) => updateRule(rules, index, "type", event.target.value, update)} value={rule.type || "body"}><option value="status">状态码</option><option value="duration">响应时间</option><option value="header">响应头</option><option value="jsonpath">JSONPath</option><option value="regex">正则</option><option value="body">响应体</option><option value="variable">提取变量</option></select>
    <input className="text-input" onChange={(event) => updateRule(rules, index, "expression", event.target.value, update)} placeholder="表达式/变量名" value={rule.expression || ""} />
    <select className="text-input" onChange={(event) => updateRule(rules, index, "operator", event.target.value, update)} value={rule.operator || "equals"}>{["equals", "not_equals", "contains", "not_contains", "exists", "not_exists", "matches", "gt", "gte", "lt", "lte"].map((item) => <option key={item}>{item}</option>)}</select>
    <input className="text-input" disabled={["exists", "not_exists"].includes(rule.operator)} onChange={(event) => updateRule(rules, index, "expected", event.target.value, update)} placeholder="预期值" value={rule.expected ?? ""} />
    <input className="text-input" onChange={(event) => updateRule(rules, index, "description", event.target.value, update)} placeholder="说明" value={rule.description || ""} />
    <label><input checked={rule.enabled !== false} onChange={(event) => updateRule(rules, index, "enabled", event.target.checked, update)} type="checkbox" />启用</label>
    <button className="link-button danger-link" onClick={() => update(rules.filter((_, itemIndex) => itemIndex !== index))} type="button">删除</button>
  </div>) : <div className="api-rule-empty">暂无断言规则</div>}</div>;
}

function VersionPanel({ versions, diff, busy, onCompare, onRestore }) {
  return <div className="api-version-panel"><div className="api-version-list">{versions.map((item, index) => <article key={item.id}><div><strong>V{item.version}</strong><span>{item.changeSummary}</span><small>{item.createdBy} · {new Date(item.createdAt).toLocaleString()}</small></div><div>{versions[index + 1] ? <button className="link-button" disabled={busy} onClick={() => onCompare(versions[index + 1].version, item.version)} type="button">与上一版对比</button> : null}<button className="link-button" disabled={busy || index === 0} onClick={() => onRestore(item.version)} type="button">回滚</button></div></article>)}</div>{diff ? <pre className="api-version-diff">{JSON.stringify(diff.changes, null, 2)}</pre> : null}</div>;
}

function parseRuleArray(value) {
  if (Array.isArray(value)) return value;
  try {
    const parsed = JSON.parse(value || "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function updateRule(rules, index, key, value, update) {
  update(rules.map((rule, itemIndex) => itemIndex === index ? { ...rule, [key]: value } : rule));
}

function prettyResponseBody(value) {
  if (!value) return "";
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

function DebuggerResponseView({ label, value, empty = "暂无数据" }) {
  return <section className="api-debugger-response-view">
    <header><div><strong>{label}</strong><span>{value ? `${value.length} 字符` : "等待数据"}</span></div><button disabled={!value} onClick={() => navigator.clipboard?.writeText(value)} type="button"><Clipboard size={12} />复制</button></header>
    {value ? <pre>{value}</pre> : <div className="api-debugger-response-empty"><Braces size={22} /><span>{empty}</span></div>}
  </section>;
}

function ResultBlock({ label, value }) {
  return <section className="api-result-block"><div><strong>{label}</strong><button onClick={() => navigator.clipboard?.writeText(value)} type="button"><Clipboard size={12} />复制</button></div><pre>{value || "执行接口后显示结果"}</pre></section>;
}

function CurlImportModal({ busy, command, error, onChange, onClose, onSubmit }) {
  return <div className="modal-backdrop">
    <form className="modal-card api-curl-import-modal" onSubmit={onSubmit}>
      <div className="modal-header">
        <div className="api-curl-modal-title"><span><Clipboard size={17} /></span><div><strong>导入 cURL</strong><small>解析请求并生成可编辑的接口定义</small></div></div>
        <button aria-label="关闭" className="modal-close" disabled={busy} onClick={onClose} type="button">×</button>
      </div>
      <div className="api-curl-import-body">
        <div className="api-curl-command-label"><label htmlFor="curl-command">cURL 命令</label><span>{command.length} 字符</span></div>
        <div className={`api-curl-editor ${error ? "has-error" : ""}`}>
          <div aria-hidden="true" className="api-curl-editor-gutter">›</div>
          <textarea autoFocus id="curl-command" onChange={(event) => onChange(event.target.value)} placeholder={"curl --request POST 'https://api.example.com/users' \\\n  --header 'Content-Type: application/json' \\\n  --data '{\"name\":\"Synapse\"}'"} spellCheck="false" value={command} />
        </div>
        {error ? <div className="api-curl-error" role="alert">{error}</div> : null}
        <div className="api-curl-import-hint">
          <Braces size={16} />
          <p><strong>导入后仍可修改</strong><span>方法、URL、请求头和请求体会自动填入新增接口表单；鉴权信息将按敏感字段规则处理。</span></p>
        </div>
      </div>
      <div className="modal-actions"><button className="secondary-button" disabled={busy} onClick={onClose} type="button">取消</button><button className="primary-button" disabled={busy || !command.trim()} type="submit">{busy ? "正在解析…" : "解析并继续"}</button></div>
    </form>
  </div>;
}

function InterfaceModal({ form, setForm, products, modules, editing, busy, onClose, onSubmit }) {
  return <div className="modal-backdrop"><form className="modal-card api-interface-modal" onSubmit={onSubmit}><div className="modal-header"><strong>{editing ? "编辑接口" : "新增接口"}</strong><button className="modal-close" onClick={onClose} type="button">×</button></div><div className="api-interface-form">
    <FilterSelect required label="项目/产品" value={form.productId} onChange={(value) => setForm({ ...form, productId: value, moduleId: "" })} options={products} />
    <FilterSelect label="模块名称" value={form.moduleId} onChange={(value) => setForm({ ...form, moduleId: value })} options={modules.map((item) => ({ value: String(item.id), label: item.name }))} />
    <FilterInput required label="接口名称" value={form.name} onChange={(value) => setForm({ ...form, name: value })} />
    <FilterInput required label="URL 路径" value={form.path} onChange={(value) => setForm({ ...form, path: value })} placeholder="/reimbursements" />
    <FilterSelect required label="Method" value={form.method} onChange={(value) => setForm({ ...form, method: value })} options={["GET", "POST", "PUT", "DELETE", "PATCH"].map(option)} />
    <FilterSelect required label="协议" value={form.protocol} onChange={(value) => setForm({ ...form, protocol: value })} options={["HTTP", "HTTPS"].map(option)} />
    <FilterSelect label="端类型" value={form.endpointType} onChange={(value) => setForm({ ...form, endpointType: value })} options={["WEB", "APP"].map(option)} />
    <FilterSelect label="接口状态" value={form.lifecycleStatus} onChange={(value) => setForm({ ...form, lifecycleStatus: value })} options={[{ value: "draft", label: "草稿" }, { value: "active", label: "启用" }, { value: "disabled", label: "停用" }, { value: "deprecated", label: "已废弃" }]} />
  </div><div className="modal-actions"><button className="icon-text-button compact-button" onClick={onClose} type="button">取消</button><button className="primary-button compact-button" disabled={busy} type="submit">{busy ? "提交中" : "提交"}</button></div></form></div>;
}

function FilterInput({ label, value, onChange, placeholder, required }) { return <label className="form-field"><span>{required ? "* " : ""}{label}</span><input className="text-input" value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder || `请输入${label}`} /></label>; }
function FilterSelect({ label, value, onChange, options, required }) { return <label className="form-field"><span>{required ? "* " : ""}{label}</span><select className="text-input" value={value} onChange={(event) => onChange(event.target.value)}><option value="">请选择{label}</option>{options.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>; }
function option(value) { return { value, label: value }; }
function lifecycleLabel(value) { return ({ draft: "草稿", active: "启用", disabled: "停用", deprecated: "已废弃" })[value] || value; }

const placeholderMeta = {
  测试用例: [FlaskConical, "API CASES", "还没有接口测试用例", "从已维护的接口创建测试用例。"],
  全局变量: [FileJson, "VARIABLES", "还没有接口全局变量", "变量可通过 ${variable_name} 在请求中引用。"],
  请求头管理: [KeyRound, "HEADERS", "还没有公共请求头", "创建请求头模板后，可在多个接口和用例中复用。"]
};
function APISectionPlaceholder({ section }) {
  const [Icon, kicker, title, text] = placeholderMeta[section] || placeholderMeta["测试用例"];
  return <div className="section-stack api-automation-page"><PageHeader title={section} description="接口自动化资产管理" /><section className="resource-panel api-resource-panel"><div className="api-section-marker"><Icon size={18} /><span>{kicker}</span></div><div className="api-empty-state"><span><Icon size={24} /></span><strong>{title}</strong><p>{text}</p><button className="primary-button compact-button" type="button">新增{section === "测试用例" ? "用例" : section.replace("管理", "")}</button></div></section></div>;
}
