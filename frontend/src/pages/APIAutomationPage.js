import { useMemo, useState } from "react";
import { ArrowLeft, Braces, Clipboard, FileJson, FlaskConical, KeyRound, Play, Plus, RefreshCw, Save, Upload } from "lucide-react";
import { DataTable, PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { apiAutomationService } from "../services/apiAutomationService.js";
import { configService } from "../services/configService.js";
import { pageItems } from "../utils/formatters.js";

const emptyForm = { name: "", productId: "", moduleId: "", path: "", method: "GET", protocol: "HTTP", endpointType: "WEB", lifecycleStatus: "draft", timeoutSeconds: 30, followRedirects: true, configuration: {}, revision: 0 };
const initialFilters = { keyword: "", projectId: "", productId: "", moduleId: "", method: "", lifecycleStatus: "" };

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
  const { data, loading, error, reload } = useAsyncData(() => apiAutomationService.interfaces.list({ ...applied, page, pageSize }), [applied, page, pageSize]);
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
    try { await Promise.all(ids.map((id) => apiAutomationService.interfaces.remove(id))); setSelected([]); await reload(); setNotice("接口已删除。"); }
    catch (err) { setNotice(err.message || "删除接口失败"); } finally { setBusy(false); }
  }
  async function importCurl() {
    const command = window.prompt("请粘贴 cURL 命令");
    if (!command) return;
    setBusy(true); setNotice("");
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
      setModal(true);
      setNotice(parsed.maskedHeaders?.length ? `已识别敏感请求头：${parsed.maskedHeaders.join("、")}` : "cURL 已解析，请补充接口归属和名称。");
    } catch (error) {
      setNotice(error.message || "解析 cURL 失败");
    } finally {
      setBusy(false);
    }
  }

  const columns = [
    { key: "select", title: <input checked={pageRows.length > 0 && pageRows.every((row) => selected.includes(row.id))} onChange={(event) => setSelected(event.target.checked ? pageRows.map((row) => row.id) : [])} type="checkbox" />, render: (row) => <input checked={selected.includes(row.id)} onChange={() => setSelected((current) => current.includes(row.id) ? current.filter((id) => id !== row.id) : [...current, row.id])} type="checkbox" /> },
    { key: "id", title: "ID" },
    { key: "productName", title: "项目/产品", render: (row) => `${row.projectName}/${row.productName}` },
    { key: "moduleName", title: "模块名称", render: (row) => row.moduleName || "未分组" },
    { key: "name", title: "接口名称" },
    { key: "path", title: "方法 / 路径", render: (row) => <span><span className={`api-tag method-${row.method?.toLowerCase()}`}>{row.method}</span> <code>{row.path}</code></span> },
    { key: "endpointType", title: "端类型", render: (row) => <span className="api-tag endpoint">{row.endpointType}</span> },
    { key: "lifecycleStatus", title: "接口状态", render: (row) => <span className={`status-badge ${row.lifecycleStatus === "active" ? "status-passed" : ""}`}>{lifecycleLabel(row.lifecycleStatus)}</span> },
    { key: "lastDebugStatus", title: "最近调试", render: (row) => row.lastDebugStatus ? <span className={`status-badge ${row.lastDebugStatus === "success" ? "status-passed" : "status-failed"}`}>{row.lastDebugStatus === "success" ? "通过" : "失败"}</span> : "未调试" },
    { key: "updatedBy", title: "最近修改", render: (row) => <span>{row.updatedBy}<br /><small>{new Date(row.updatedAt).toLocaleString()}</small></span> },
    { key: "operations", title: "操作", render: (row) => <div className="action-links"><button className="link-button" onClick={() => setDetailRow(row)} type="button">调试</button><button className="link-button" onClick={() => openEdit(row)} type="button">编辑</button><button className="link-button danger-link" onClick={() => removeRows([row.id])} type="button">删除</button></div> }
  ];

  if (detailRow) {
    return <InterfaceDetailWorkspace row={detailRow} projectId={String(detailRow.projectId)} onBack={() => { setDetailRow(null); reload(); }} />;
  }

  return <div className="section-stack api-interface-page">
    <PageHeader title="接口管理" description="收集、维护和调试接口定义" />
    <div className="api-interface-browser">
    <aside className="resource-panel api-resource-tree"><strong>接口资源</strong><button className={!filters.projectId ? "active" : ""} onClick={() => { setFilters(initialFilters); setApplied(initialFilters); setPage(1); }} type="button">全部项目</button>{projects.map((project) => <div key={project.id}><button className={filters.projectId === String(project.id) && !filters.productId ? "active" : ""} onClick={() => { const next = { ...initialFilters, projectId: String(project.id) }; setFilters(next); setApplied(next); setPage(1); }} type="button">{project.name}</button>{products.filter((product) => product.projectId === String(project.id)).map((product) => <button className={`child ${filters.productId === product.value ? "active" : ""}`} key={product.value} onClick={() => { const next = { ...initialFilters, projectId: String(project.id), productId: product.value }; setFilters(next); setApplied(next); setPage(1); }} type="button">{product.label.split("/").pop()}</button>)}</div>)}</aside>
    <section className="resource-panel api-interface-list">
      <div className="panel-header"><strong>接口信息收集</strong></div>
      <form className="api-filter-grid" onSubmit={(event) => { event.preventDefault(); setApplied(filters); setPage(1); }}>
        <FilterInput label="名称或路径" value={filters.keyword} onChange={(value) => setFilters({ ...filters, keyword: value })} />
        <FilterSelect label="项目/产品" value={filters.productId} onChange={(value) => setFilters({ ...filters, productId: value, projectId: products.find((item) => item.value === value)?.projectId || "", moduleId: "" })} options={products} />
        <FilterSelect label="模块名称" value={filters.moduleId} onChange={(value) => setFilters({ ...filters, moduleId: value })} options={modules.map((item) => ({ value: String(item.id), label: item.name }))} />
        <FilterSelect label="方法" value={filters.method} onChange={(value) => setFilters({ ...filters, method: value })} options={["GET", "POST", "PUT", "DELETE", "PATCH"].map(option)} />
        <FilterSelect label="接口状态" value={filters.lifecycleStatus} onChange={(value) => setFilters({ ...filters, lifecycleStatus: value })} options={[{ value: "draft", label: "草稿" }, { value: "active", label: "启用" }, { value: "disabled", label: "停用" }, { value: "deprecated", label: "已废弃" }]} />
        <div className="api-filter-actions"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={() => { setFilters(initialFilters); setApplied(initialFilters); }} type="button">重置</button></div>
      </form>
      <div className="api-list-toolbar"><div className="api-tabs"><button className="active" type="button">接口定义</button></div><div><button className="primary-button compact-button" onClick={openCreate} type="button"><Plus size={14} />新增</button><button className="icon-text-button compact-button" disabled={busy} onClick={importCurl} type="button"><Upload size={14} />导入 cURL</button><button className="danger-button compact-button" disabled={!selected.length || busy} onClick={() => removeRows(selected)} type="button">批量删除</button><button className="icon-text-button compact-button" onClick={reload} type="button"><RefreshCw size={14} /></button></div></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <StateBlock loading={loading} error={error}><TablePanel><DataTable columns={columns} rows={pageRows} emptyText="暂无接口数据" /><PaginationBar page={page} pageSize={pageSize} total={total} totalPages={totalPages} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} /></TablePanel></StateBlock>
    </section>
    </div>
    {modal ? <InterfaceModal form={form} setForm={setForm} products={products} modules={modules} editing={editing} busy={busy} onClose={() => setModal(false)} onSubmit={save} /> : null}
  </div>;
}

const detailSections = [
  ["variables", "临时变量", "仅用于本次预览与调试，优先级最高"],
  ["auth", "认证", "配置 Bearer、Basic 或 API Key"],
  ["headers", "请求头", "维护接口请求 Headers"],
  ["params", "参数", "维护查询参数"],
  ["body", "请求体", "维护 Body 配置"],
  ["jsonpath", "后置 JSONPath 提取", "提取响应 JSON 到缓存"],
  ["regex", "后置正则提取", "通过正则提取响应内容"],
  ["script", "后置脚本", "编写自定义响应处理脚本"],
  ["assertions", "结构化断言配置", "维护响应断言规则"]
];

function InterfaceDetailWorkspace({ row, projectId, onBack }) {
  const saved = row.configuration || {};
  const { data: testObjectsData } = useAsyncData(
    () => configService.testObjects.list({ productId: row.productId, page: 1, pageSize: 200 }),
    [row.productId]
  );
  const testObjects = pageItems(testObjectsData);
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
  const [method, setMethod] = useState(row.method || "GET");
  const [url, setUrl] = useState(row.path || "");
  const [result, setResult] = useState(null);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [testObjectId, setTestObjectId] = useState("");
  const [preview, setPreview] = useState(null);
  const [temporaryVariables, setTemporaryVariables] = useState("{\n  \n}");
  const [temporaryFiles, setTemporaryFiles] = useState(saved.temporaryFiles || []);
  const [debugEvents, setDebugEvents] = useState([]);
  const [activeTaskId, setActiveTaskId] = useState("");

  function configurationPayload() {
    return {
      ...saved,
      ...config,
      auth: JSON.parse(config.auth || "{\"type\":\"none\"}"),
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
    try {
      await apiAutomationService.interfaces.update(row.id, { productId: row.productId, moduleId: row.moduleId, name: row.name, method, path: url, protocol: row.protocol, endpointType: row.endpointType, lifecycleStatus: row.lifecycleStatus, timeoutSeconds: row.timeoutSeconds, followRedirects: row.followRedirects, configuration: configurationPayload(), revision: row.revision });
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
            request: current.request
          });
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
      temporaryVariables: JSON.parse(temporaryVariables || "{}"),
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

  return <div className="api-detail-page">
    <header className="api-detail-header"><div><span>接口配置工作台 / #{row.id}</span><h2>{row.name}</h2><p>维护请求配置、预览最终请求和查看最近一次响应结果</p></div><div><button className="primary-button compact-button" disabled={busy} onClick={previewRequest} type="button"><Braces size={14} />预览请求</button><button className="icon-text-button compact-button" disabled={busy} onClick={exportCurl} type="button"><Clipboard size={14} />复制 cURL</button>{activeTaskId ? <button className="danger-button compact-button" onClick={cancelDebug} type="button">取消</button> : <button className="success-button compact-button" disabled={busy} onClick={execute} type="button"><Play size={14} />执行</button>}<button className="icon-text-button compact-button" onClick={onBack} type="button"><ArrowLeft size={14} />返回</button></div></header>
    {notice ? <div className="inline-notice">{notice}</div> : null}
    <div className="api-detail-layout">
      <aside className="api-detail-nav"><div><strong>配置项</strong><span>按执行链路维护接口配置</span></div>{detailSections.map(([key, title, description]) => <button className={active === key ? "active" : ""} key={key} onClick={() => setActive(key)} type="button"><strong>{title}</strong><span>{description}</span></button>)}</aside>
      <main className="api-detail-editor">
        <div className="api-request-line"><select className="text-input" value={method} onChange={(event) => setMethod(event.target.value)}>{["GET", "POST", "PUT", "DELETE", "PATCH"].map((item) => <option key={item}>{item}</option>)}</select><input className="text-input" value={url} onChange={(event) => setUrl(event.target.value)} /><select aria-label="测试环境" className="text-input" value={testObjectId} onChange={(event) => setTestObjectId(event.target.value)}><option value="">选择测试环境</option>{testObjects.map((item) => <option key={item.id} value={item.id}>{item.envName}</option>)}</select><button className="primary-button compact-button" disabled={busy} onClick={saveConfiguration} type="button"><Save size={14} />保存</button></div>
        <div className="api-editor-heading"><strong>{detailSections.find(([key]) => key === active)?.[1]}</strong><span>{detailSections.find(([key]) => key === active)?.[2]}</span></div>
        <div className="api-editor-tip">{active === "headers" ? `已加载 ${Object.keys(defaultHeaders).length} 个项目默认请求头；接口内同名请求头优先。` : active === "variables" ? "临时变量不会保存到接口，覆盖同名的环境级和项目级全局变量。" : "请输入合法 JSON；后置脚本支持普通文本配置，保存后用于执行阶段处理。"}</div>
        {active === "body" ? <label className="form-field"><span>Body 类型</span><select className="text-input" value={config.bodyType} onChange={(event) => setConfig({ ...config, bodyType: event.target.value })}><option value="none">无</option><option value="json">JSON</option><option value="form_data">form-data</option><option value="urlencoded">x-www-form-urlencoded</option><option value="raw">Raw 文本</option></select></label> : null}
        {active === "body" && config.bodyType === "form_data" ? <div className="api-temp-files"><label className="icon-text-button compact-button">上传临时文件<input disabled={busy} hidden onChange={uploadTemporaryFile} type="file" /></label>{temporaryFiles.map((file) => <span key={file.id}>{file.originalName}（{Math.ceil(file.sizeBytes / 1024)} KB）<button className="link-button danger-link" onClick={() => removeTemporaryFile(file)} type="button">删除</button></span>)}</div> : null}
        <textarea className="api-config-editor" spellCheck="false" value={active === "variables" ? temporaryVariables : config[active]} onChange={(event) => active === "variables" ? setTemporaryVariables(event.target.value) : setConfig({ ...config, [active]: event.target.value })} />
      </main>
      <aside className="api-result-panel">
        <div className="api-result-heading"><div><strong>调用结果</strong><span>执行后固定展示最近一次响应</span></div><button className="success-button compact-button" disabled={busy} onClick={execute} type="button"><Play size={14} />{busy ? "执行中" : "执行"}</button></div>
        <div className="api-result-summary"><div><span>状态码</span><strong>{result?.status ?? "-"}</strong></div><div><span>响应时间</span><strong>{result ? `${(result.duration / 1000).toFixed(2)} 秒` : "-"}</strong></div><div><span>执行状态</span><strong className={result?.ok ? "success" : result ? "danger" : ""}>{result ? result.ok ? "调用完成" : "调用失败" : "等待执行"}</strong></div></div>
        <div className="api-result-tabs"><span className="active">响应</span><span>请求</span><span>缓存</span></div>
        {debugEvents.length ? <ResultBlock label="实时执行日志" value={debugEvents.map((event) => `[${event.progress}%] ${event.message}`).join("\n")} /> : null}
        {preview ? <ResultBlock label="最终请求预览" value={JSON.stringify(preview, null, 2)} /> : null}
        <ResultBlock label="响应头" value={result ? JSON.stringify(result.headers, null, 2) : ""} />
        <ResultBlock label="响应体" value={result?.body || ""} />
        {result ? <ResultBlock label="实际请求" value={JSON.stringify(result.request, null, 2)} /> : null}
      </aside>
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

function ResultBlock({ label, value }) {
  return <section className="api-result-block"><div><strong>{label}</strong><button onClick={() => navigator.clipboard?.writeText(value)} type="button"><Clipboard size={12} />复制</button></div><pre>{value || "执行接口后显示结果"}</pre></section>;
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
