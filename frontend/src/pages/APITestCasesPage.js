import { useEffect, useMemo, useState } from "react";
import { ArrowDown, ArrowLeft, ArrowUp, CheckCircle2, Copy, GitBranch, Play, Plus, Save, Trash2, X } from "lucide-react";
import { DataTable, PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { apiAutomationService } from "../services/apiAutomationService.js";
import { configService } from "../services/configService.js";
import { pageItems } from "../utils/formatters.js";

const emptyDraft = { steps: [], datasets: [], variables: [], dataSchema: [] };
const emptyForm = { projectId: "", productId: "", moduleId: "", name: "", priority: "P2", status: "draft", owner: "", tags: [] };

export function APITestCasesPage() {
  const [filters, setFilters] = useState({ keyword: "", projectId: "", productId: "", status: "" });
  const [applied, setApplied] = useState(filters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [detailId, setDetailId] = useState(0);
  const [modal, setModal] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [notice, setNotice] = useState("");
  const { data: projectsData } = useAsyncData(() => configService.projects.list({ page: 1, pageSize: 200 }), []);
  const { data: productsData } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 500 }), []);
  const projects = pageItems(projectsData);
  const products = pageItems(productsData);
  const { data, loading, reload } = useAsyncData(
    () => apiAutomationService.testCases.list({ ...applied, page, pageSize }),
    [applied, page, pageSize]
  );
  const rows = pageItems(data);
  const total = Number(data?.total || 0);
  const projectProducts = products.filter((item) => !form.projectId || String(item.projectId) === String(form.projectId));

  async function createCase(event) {
    event.preventDefault();
    setNotice("");
    try {
      const result = await apiAutomationService.testCases.create({
        ...form,
        projectId: Number(form.projectId),
        productId: Number(form.productId),
        moduleId: Number(form.moduleId || 0),
        tags: form.tags,
        draft: emptyDraft
      });
      setModal(false);
      setForm(emptyForm);
      await reload();
      setDetailId(result.id);
    } catch (error) {
      setNotice(error.message || "创建接口测试用例失败");
    }
  }

  if (detailId) return <APITestCaseEditor id={detailId} onBack={() => { setDetailId(0); reload(); }} />;

  const columns = [
    { key: "id", title: "ID" },
    { key: "name", title: "用例名称", render: (row) => <button className="link-button" onClick={() => setDetailId(row.id)} type="button">{row.name}</button> },
    { key: "scope", title: "项目 / 产品", render: (row) => <span>{row.projectName} / {row.productName}</span> },
    { key: "steps", title: "步骤", render: (row) => `${safeDraft(row.draft).steps.length} 步` },
    { key: "priority", title: "优先级", render: (row) => <span className="api-case-priority">{row.priority}</span> },
    { key: "version", title: "发布版本", render: (row) => row.currentVersion ? `V${row.currentVersion}` : "未发布" },
    { key: "status", title: "状态", render: (row) => <span className={`status-pill ${row.status === "active" ? "success" : ""}`}>{caseStatus(row.status)}</span> },
    { key: "owner", title: "负责人" },
    { key: "updatedAt", title: "更新时间", render: (row) => new Date(row.updatedAt).toLocaleString() },
    { key: "actions", title: "操作", render: (row) => <button className="link-button" onClick={() => setDetailId(row.id)} type="button">编排</button> }
  ];

  return <div className="section-stack api-automation-page">
    <PageHeader title="接口测试用例" description="编排多接口业务链路，发布不可变版本并交给执行器运行" />
    <section className="resource-panel api-resource-panel">
      <form className="filter-grid api-case-filter" onSubmit={(event) => { event.preventDefault(); setPage(1); setApplied(filters); }}>
        <label><span>名称或 ID</span><input className="text-input" placeholder="搜索用例" value={filters.keyword} onChange={(event) => setFilters({ ...filters, keyword: event.target.value })} /></label>
        <label><span>项目</span><select className="text-input" value={filters.projectId} onChange={(event) => setFilters({ ...filters, projectId: event.target.value, productId: "" })}><option value="">全部项目</option>{projects.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
        <label><span>产品</span><select className="text-input" value={filters.productId} onChange={(event) => setFilters({ ...filters, productId: event.target.value })}><option value="">全部产品</option>{products.filter((item) => !filters.projectId || String(item.projectId) === String(filters.projectId)).map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
        <label><span>状态</span><select className="text-input" value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}><option value="">全部状态</option><option value="draft">草稿</option><option value="active">已发布</option><option value="disabled">停用</option><option value="deprecated">已废弃</option></select></label>
        <div className="api-filter-actions"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={() => { setFilters({ keyword: "", projectId: "", productId: "", status: "" }); setApplied({ keyword: "", projectId: "", productId: "", status: "" }); }} type="button">重置</button></div>
      </form>
      <div className="api-list-toolbar"><div><strong>用例资产</strong><span>共 {total} 条</span></div><button className="primary-button compact-button" onClick={() => setModal(true)} type="button"><Plus size={14} />新增用例</button></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <TablePanel><DataTable columns={columns} rows={loading ? [] : rows} emptyText={loading ? "正在加载" : "暂无接口测试用例"} /><PaginationBar page={page} pageSize={pageSize} total={total} totalPages={Math.ceil(total / pageSize)} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} /></TablePanel>
    </section>
    {modal ? <div className="modal-backdrop"><form className="modal-card api-interface-modal" onSubmit={createCase}><div className="modal-header"><strong>新增接口测试用例</strong><button className="modal-close" onClick={() => setModal(false)} type="button">×</button></div><div className="api-interface-form">
      <label className="form-field"><span>* 所属项目</span><select required className="text-input" value={form.projectId} onChange={(event) => setForm({ ...form, projectId: event.target.value, productId: "" })}><option value="">请选择项目</option>{projects.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="form-field"><span>* 所属产品</span><select required className="text-input" value={form.productId} onChange={(event) => setForm({ ...form, productId: event.target.value })}><option value="">请选择产品</option>{projectProducts.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="form-field"><span>* 用例名称</span><input required className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></label>
      <label className="form-field"><span>优先级</span><select className="text-input" value={form.priority} onChange={(event) => setForm({ ...form, priority: event.target.value })}>{["P0", "P1", "P2", "P3"].map((item) => <option key={item}>{item}</option>)}</select></label>
      <label className="form-field"><span>负责人</span><input className="text-input" value={form.owner} onChange={(event) => setForm({ ...form, owner: event.target.value })} /></label>
    </div><div className="modal-actions"><button className="icon-text-button compact-button" onClick={() => setModal(false)} type="button">取消</button><button className="primary-button compact-button" type="submit">创建并编排</button></div></form></div> : null}
  </div>;
}

function APITestCaseEditor({ id, onBack }) {
  const [item, setItem] = useState(null);
  const [draft, setDraft] = useState(emptyDraft);
  const [selected, setSelected] = useState(0);
  const [interfaceId, setInterfaceId] = useState("");
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState("");
  const [validation, setValidation] = useState(null);
  const [runOpen, setRunOpen] = useState(false);
  const { data: interfacesData } = useAsyncData(() => item ? apiAutomationService.interfaces.list({ projectId: item.projectId, page: 1, pageSize: 500 }) : Promise.resolve({ items: [] }), [item?.projectId]);
  const interfaces = pageItems(interfacesData);

  async function load() {
    const result = await apiAutomationService.testCases.get(id);
    setItem(result);
    setDraft(safeDraft(result.draft));
    setDirty(false);
  }
  useEffect(() => { load().catch((error) => setNotice(error.message)); }, [id]);

  const selectedStep = draft.steps[selected];
  const updateDraft = (next) => { setDraft(next); setDirty(true); setValidation(null); };
  const updateStep = (key, value) => updateDraft({ ...draft, steps: draft.steps.map((step, index) => index === selected ? { ...step, [key]: value } : step) });

  async function addStep() {
    if (!interfaceId) return;
    try {
      const api = interfaces.find((entry) => String(entry.id) === String(interfaceId));
      const versions = await apiAutomationService.versions.list(interfaceId);
      const version = versions[0]?.version;
      if (!version) throw new Error("接口尚无可用版本");
      const count = draft.steps.length + 1;
      const step = { id: crypto.randomUUID(), name: api.name, key: `step_${count}`, interfaceId: Number(interfaceId), interfaceVersion: version, enabled: true, phase: "main", condition: "", failurePolicy: "stop", overrides: {} };
      updateDraft({ ...draft, steps: [...draft.steps, step] });
      setSelected(draft.steps.length);
      setInterfaceId("");
    } catch (error) { setNotice(error.message || "添加接口步骤失败"); }
  }

  async function saveDraft(silent = false) {
    if (!item || !dirty || saving) return;
    setSaving(true);
    if (!silent) setNotice("");
    try {
      const result = await apiAutomationService.testCases.update(id, {
        projectId: item.projectId, productId: item.productId, moduleId: item.moduleId,
        name: item.name, priority: item.priority, status: item.status, owner: item.owner,
        tags: item.tags || [], draft, revision: item.revision
      });
      setItem({ ...item, revision: result.revision });
      setDirty(false);
      if (!silent) setNotice("草稿已保存。");
    } catch (error) { setNotice(error.message || "保存草稿失败"); } finally { setSaving(false); }
  }

  useEffect(() => {
    if (!dirty || saving) return undefined;
    const timer = setTimeout(() => saveDraft(true), 3000);
    return () => clearTimeout(timer);
  }, [dirty, draft, item?.revision]);

  async function validate() {
    if (dirty) await saveDraft(true);
    try {
      const result = await apiAutomationService.testCases.validate(id);
      setValidation(result);
      setNotice(result.valid ? "发布检查通过。" : `发布检查失败：${result.errors.join("；")}`);
    } catch (error) { setNotice(error.message || "发布检查失败"); }
  }

  async function publish() {
    if (dirty) {
      setNotice("请等待草稿保存完成后再发布。");
      return;
    }
    try {
      const result = await apiAutomationService.testCases.publish(id, { revision: item.revision, changeSummary: "发布接口测试用例" });
      setNotice(`已发布 V${result.version}。`);
      await load();
    } catch (error) { setNotice(error.message || "发布失败"); }
  }

  function moveStep(direction) {
    const target = selected + direction;
    if (target < 0 || target >= draft.steps.length) return;
    const steps = [...draft.steps];
    [steps[selected], steps[target]] = [steps[target], steps[selected]];
    setSelected(target);
    updateDraft({ ...draft, steps });
  }

  if (!item) return <div className="screen-center">{notice || "正在加载用例"}</div>;
  return <div className="api-case-editor-page">
    <header className="api-case-editor-header"><div><button className="icon-text-button compact-button" onClick={onBack} type="button"><ArrowLeft size={14} />返回用例列表</button><span className="page-eyebrow">接口测试用例 / #{item.id}</span><h1>{item.name}</h1></div><div><span aria-live="polite" className={`api-save-state ${dirty ? "dirty" : ""}`} role="status">{saving ? "保存中" : dirty ? "有未保存修改" : "草稿已保存"}</span><button className="icon-text-button compact-button" disabled={!dirty || saving} onClick={() => saveDraft(false)} type="button"><Save size={14} />保存草稿</button><button className="icon-text-button compact-button" onClick={validate} type="button"><CheckCircle2 size={14} />发布检查</button><button className="primary-button compact-button" disabled={dirty || saving} onClick={publish} type="button">发布版本</button><button className="success-button compact-button" disabled={!item.currentVersion || dirty || saving} onClick={() => setRunOpen(true)} type="button"><Play size={14} />执行</button></div></header>
    {notice ? <div aria-live="polite" className="inline-notice" role="status">{notice}</div> : null}
    <div className="api-case-workbench">
      <aside className="api-case-resource-pane"><header><strong>接口资源</strong><span>从当前项目选择接口</span></header><select className="text-input" value={interfaceId} onChange={(event) => setInterfaceId(event.target.value)}><option value="">选择接口</option>{interfaces.map((entry) => <option key={entry.id} value={entry.id}>{entry.moduleName ? `${entry.moduleName} / ` : ""}{entry.name}</option>)}</select><button className="primary-button compact-button" disabled={!interfaceId} onClick={addStep} type="button"><Plus size={13} />添加为步骤</button><div className="api-case-scope-card"><GitBranch size={16} /><strong>变量流</strong><span>前序提取变量自动进入后续步骤，发布时检查引用顺序。</span></div></aside>
      <main className="api-case-sequence"><header><div><strong>调用链</strong><span>{draft.steps.length} 个步骤 · 实例内顺序执行</span></div><div><button disabled={selected <= 0} onClick={() => moveStep(-1)} type="button"><ArrowUp size={13} /></button><button disabled={selected >= draft.steps.length - 1} onClick={() => moveStep(1)} type="button"><ArrowDown size={13} /></button></div></header>
        <div className="api-case-step-list">{draft.steps.length ? draft.steps.map((step, index) => <button className={`api-case-step ${selected === index ? "active" : ""} ${step.enabled === false ? "disabled" : ""}`} key={step.id || index} onClick={() => setSelected(index)} type="button"><span>{String(index + 1).padStart(2, "0")}</span><div><strong>{step.name || "未命名步骤"}</strong><small>{step.key} · 接口 #{step.interfaceId} V{step.interfaceVersion}</small></div><i>{step.phase === "cleanup" ? "清理" : step.condition ? "条件" : "主流程"}</i></button>) : <div className="api-case-empty"><GitBranch size={28} /><strong>调用链还没有步骤</strong><span>从左侧选择接口并添加。</span></div>}</div>
      </main>
      <aside className="api-case-inspector">{selectedStep ? <><header><div><strong>步骤配置</strong><span>仅保存与接口版本不同的覆盖项</span></div><button aria-label="删除步骤" className="danger-link" onClick={() => { updateDraft({ ...draft, steps: draft.steps.filter((_, index) => index !== selected) }); setSelected(Math.max(0, selected - 1)); }} type="button"><Trash2 size={14} /></button></header>
        <label><span>显示名称</span><input className="text-input" value={selectedStep.name} onChange={(event) => updateStep("name", event.target.value)} /></label>
        <label><span>步骤标识</span><input className="text-input" value={selectedStep.key} onChange={(event) => updateStep("key", event.target.value)} /></label>
        <div className="api-case-inline-fields"><label><span>阶段</span><select className="text-input" value={selectedStep.phase || "main"} onChange={(event) => updateStep("phase", event.target.value)}><option value="setup">前置</option><option value="main">主流程</option><option value="cleanup">清理</option></select></label><label><span>失败策略</span><select className="text-input" value={selectedStep.failurePolicy || "stop"} onChange={(event) => updateStep("failurePolicy", event.target.value)}><option value="stop">终止用例</option><option value="continue">继续执行</option><option value="always">始终执行</option></select></label></div>
        <label><span>执行条件</span><input className="text-input" placeholder='例如 ${user_type} == "vip"' value={selectedStep.condition || ""} onChange={(event) => updateStep("condition", event.target.value)} /></label>
        <label><span>配置覆盖（JSON）</span><textarea className="api-case-overrides" spellCheck="false" value={JSON.stringify(selectedStep.overrides || {}, null, 2)} onChange={(event) => { try { updateStep("overrides", JSON.parse(event.target.value || "{}")); } catch { /* 编辑未完成时保留原值。 */ } }} /></label>
        <label className="api-case-enabled"><input checked={selectedStep.enabled !== false} onChange={(event) => updateStep("enabled", event.target.checked)} type="checkbox" />启用此步骤</label>
      </> : <div className="api-case-empty"><Copy size={24} /><strong>选择一个步骤</strong><span>在这里配置条件、失败策略和覆盖项。</span></div>}</aside>
    </div>
    {validation ? <div className={`api-case-validation ${validation.valid ? "success" : "danger"}`}><strong>{validation.valid ? "发布检查通过" : "存在阻断问题"}</strong>{validation.errors.map((error) => <span key={error}>{error}</span>)}{validation.warnings.map((warning) => <span key={warning}>{warning}</span>)}</div> : null}
    {runOpen ? <APITestRunDialog item={item} onClose={() => setRunOpen(false)} /> : null}
  </div>;
}

function APITestRunDialog({ item, onClose }) {
  const [form, setForm] = useState({ envName: "", concurrency: 5, retryFailedOnce: false, remark: "" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [run, setRun] = useState(null);
  const { data: environmentsData } = useAsyncData(
    () => configService.testObjects.list({ productId: item.productId, page: 1, pageSize: 200 }),
    [item.productId]
  );
  const environments = pageItems(environmentsData);

  useEffect(() => {
    if (!form.envName && environments.length) {
      setForm((current) => ({ ...current, envName: environments[0].envName }));
    }
  }, [environments.length]);

  useEffect(() => {
    if (!run?.batchId || ["success", "failed", "canceled"].includes(run.status)) return undefined;
    const timer = setInterval(async () => {
      try {
        const result = await apiAutomationService.testRuns.get(run.batchId);
        setRun(result.batch);
      } catch (requestError) {
        setError(requestError.message || "读取执行进度失败");
      }
    }, 1500);
    return () => clearInterval(timer);
  }, [run?.batchId, run?.status]);

  async function start(event) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      const result = await apiAutomationService.testRuns.start({
        caseIds: [item.id],
        envName: form.envName,
        concurrency: Number(form.concurrency),
        retryFailedOnce: form.retryFailedOnce,
        remark: form.remark,
        temporaryVariables: {}
      });
      setRun(result);
    } catch (requestError) {
      setError(requestError.message || "启动执行失败");
    } finally {
      setSubmitting(false);
    }
  }

  const completed = Number(run?.passedInstances || 0) + Number(run?.failedInstances || 0) + Number(run?.canceledInstances || 0);
  const progress = run?.totalInstances ? Math.round(completed * 100 / run.totalInstances) : 0;

  return <div className="modal-backdrop api-run-backdrop" role="presentation">
    <div aria-modal="true" className="modal-card api-run-dialog" role="dialog">
      <div className="modal-header"><div><strong>执行接口测试用例</strong><span>{item.name} · 发布版本 V{item.currentVersion}</span></div><button aria-label="关闭执行弹窗" className="modal-close" onClick={onClose} type="button"><X size={16} /></button></div>
      {!run ? <form onSubmit={start}>
        <div className="api-run-form">
          <label className="form-field"><span>* 测试环境</span><select required className="text-input" value={form.envName} onChange={(event) => setForm({ ...form, envName: event.target.value })}><option value="">请选择测试环境</option>{environments.map((entry) => <option key={entry.id} value={entry.envName}>{entry.envName} · {entry.target}</option>)}</select></label>
          <label className="form-field"><span>并发实例数</span><input className="text-input" max="100" min="1" type="number" value={form.concurrency} onChange={(event) => setForm({ ...form, concurrency: event.target.value })} /></label>
          <label className="form-field api-run-wide"><span>执行备注</span><input className="text-input" placeholder="可选，用于区分本次执行" value={form.remark} onChange={(event) => setForm({ ...form, remark: event.target.value })} /></label>
          <label className="api-run-option api-run-wide"><input checked={form.retryFailedOnce} onChange={(event) => setForm({ ...form, retryFailedOnce: event.target.checked })} type="checkbox" /><span><strong>失败自动重试一次</strong><small>仅重试失败的数据实例，不重复已通过实例</small></span></label>
        </div>
        {error ? <div className="inline-notice danger">{error}</div> : null}
        <div className="modal-actions"><button className="icon-text-button compact-button" onClick={onClose} type="button">取消</button><button className="success-button compact-button" disabled={submitting || !form.envName} type="submit"><Play size={14} />{submitting ? "正在启动" : "开始执行"}</button></div>
      </form> : <div className="api-run-progress">
        <div className="api-run-progress-title"><div><span className={`status-pill ${run.status === "success" ? "success" : ""}`}>{runStatus(run.status)}</span><strong>{progress}%</strong></div><small>批次 {run.batchId}</small></div>
        <div className="api-run-progress-track"><i style={{ width: `${progress}%` }} /></div>
        <div className="api-run-metrics"><div><strong>{run.totalInstances}</strong><span>总实例</span></div><div><strong>{run.runningInstances}</strong><span>执行中</span></div><div><strong>{run.passedInstances}</strong><span>通过</span></div><div><strong>{run.failedInstances}</strong><span>失败</span></div><div><strong>{run.queuedInstances}</strong><span>排队</span></div></div>
        {error ? <div className="inline-notice danger">{error}</div> : null}
        <div className="modal-actions"><button className="primary-button compact-button" onClick={onClose} type="button">关闭</button></div>
      </div>}
    </div>
  </div>;
}

function safeDraft(value) {
  if (!value) return { ...emptyDraft };
  if (typeof value === "object") return { ...emptyDraft, ...value, steps: value.steps || [], datasets: value.datasets || [], variables: value.variables || [] };
  try { return { ...emptyDraft, ...JSON.parse(value) }; } catch { return { ...emptyDraft }; }
}

function caseStatus(value) {
  return ({ draft: "草稿", active: "已发布", disabled: "停用", deprecated: "已废弃" })[value] || value;
}

function runStatus(value) {
  return ({ queued: "排队中", running: "执行中", success: "已通过", failed: "失败", canceled: "已取消" })[value] || value;
}
