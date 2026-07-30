import { useMemo, useState } from "react";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { DataTable, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { apiAutomationService } from "../services/apiAutomationService.js";
import { configService } from "../services/configService.js";
import { pageItems } from "../utils/formatters.js";

const emptyForm = { scopeType: "product", projectId: "", productId: "", envName: "", name: "", valueType: "string", value: "", description: "", enabled: true, sensitive: false, revision: 0 };

export function APIGlobalVariablesPage() {
  const [filters, setFilters] = useState({ keyword: "", scopeType: "", projectId: "", productId: "", envName: "" });
  const [modal, setModal] = useState(false);
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [notice, setNotice] = useState("");
  const { data: projectsData } = useAsyncData(() => configService.projects.list({ page: 1, pageSize: 200 }), []);
  const { data: productsData } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 500 }), []);
  const projects = pageItems(projectsData);
  const products = pageItems(productsData);
  const { data, reload } = useAsyncData(() => apiAutomationService.globalVariables.list(filters), [filters]);
  const rows = pageItems(data);
  const scopedProducts = useMemo(() => products.filter((item) => !form.projectId || String(item.projectId) === String(form.projectId)), [products, form.projectId]);

  function openCreate() {
    setEditing(null);
    setForm(emptyForm);
    setModal(true);
  }

  function openEdit(row) {
    setEditing(row);
    setForm({
      scopeType: row.scopeType, projectId: row.projectId || "", productId: row.productId || "",
      envName: row.envName || "", name: row.name, valueType: row.valueType, value: row.value,
      description: row.description || "", enabled: row.enabled, sensitive: row.sensitive, revision: row.revision
    });
    setModal(true);
  }

  async function save(event) {
    event.preventDefault();
    setNotice("");
    try {
      const body = { ...form, projectId: Number(form.projectId || 0), productId: Number(form.productId || 0) };
      if (editing) await apiAutomationService.globalVariables.update(editing.id, body);
      else await apiAutomationService.globalVariables.create(body);
      setModal(false);
      setNotice(editing ? "全局变量已更新。" : "全局变量已创建。");
      await reload();
    } catch (error) { setNotice(error.message || "保存全局变量失败"); }
  }

  async function remove(row) {
    if (!window.confirm(`确认删除变量 ${row.name} 吗？`)) return;
    try {
      await apiAutomationService.globalVariables.remove(row.id);
      setNotice("全局变量已删除。");
      await reload();
    } catch (error) { setNotice(error.message || "删除全局变量失败"); }
  }

  const columns = [
    { key: "name", title: "变量名", render: (row) => <button className="link-button" onClick={() => openEdit(row)} type="button">{row.name}</button> },
    { key: "scope", title: "作用域", render: (row) => scopeLabel(row) },
    { key: "envName", title: "环境", render: (row) => row.envName || "全部环境" },
    { key: "valueType", title: "类型", render: (row) => <span className="api-variable-type">{row.valueType}</span> },
    { key: "value", title: "变量值", render: (row) => <code className={row.sensitive ? "masked" : ""}>{row.value}</code> },
    { key: "description", title: "说明" },
    { key: "status", title: "状态", render: (row) => <span className={`status-pill ${row.enabled ? "success" : ""}`}>{row.enabled ? "启用" : "停用"}</span> },
    { key: "actions", title: "操作", render: (row) => <div className="api-row-actions"><button className="link-button" onClick={() => openEdit(row)} type="button">编辑</button><button aria-label={`删除变量 ${row.name}`} className="link-button danger-link" onClick={() => remove(row)} type="button"><Trash2 size={13} /></button></div> }
  ];

  return <div className="section-stack api-automation-page">
    <PageHeader title="接口全局变量" description="维护系统、项目和产品范围变量，执行批次启动时冻结变量快照" />
    <section className="resource-panel api-resource-panel">
      <div className="api-variable-filter">
        <input className="text-input" placeholder="搜索变量名或说明" value={filters.keyword} onChange={(event) => setFilters({ ...filters, keyword: event.target.value })} />
        <select className="text-input" value={filters.scopeType} onChange={(event) => setFilters({ ...filters, scopeType: event.target.value })}><option value="">全部作用域</option><option value="system">系统</option><option value="project">项目</option><option value="product">产品</option></select>
        <select className="text-input" value={filters.projectId} onChange={(event) => setFilters({ ...filters, projectId: event.target.value, productId: "" })}><option value="">全部项目</option>{projects.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
        <select className="text-input" value={filters.productId} onChange={(event) => setFilters({ ...filters, productId: event.target.value })}><option value="">全部产品</option>{products.filter((item) => !filters.projectId || String(item.projectId) === String(filters.projectId)).map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
        <button className="primary-button compact-button" onClick={openCreate} type="button"><Plus size={14} />新增变量</button>
      </div>
      <div className="api-global-variable-guide"><KeyRound size={16} /><div><strong>覆盖顺序</strong><span>步骤临时变量 → 提取变量 → 数据集 → 用例变量 → 接口变量 → 产品变量 → 项目变量 → 系统变量</span></div></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <TablePanel><DataTable columns={columns} rows={rows} emptyText="暂无接口全局变量" /></TablePanel>
    </section>
    {modal ? <div className="modal-backdrop"><form className="modal-card api-interface-modal" onSubmit={save}><div className="modal-header"><strong>{editing ? "编辑全局变量" : "新增全局变量"}</strong><button className="modal-close" onClick={() => setModal(false)} type="button">×</button></div><div className="api-interface-form">
      <label className="form-field"><span>* 作用域</span><select className="text-input" value={form.scopeType} onChange={(event) => setForm({ ...form, scopeType: event.target.value, projectId: event.target.value === "system" ? "" : form.projectId, productId: event.target.value === "product" ? form.productId : "" })}><option value="system">系统</option><option value="project">项目</option><option value="product">产品</option></select></label>
      {form.scopeType !== "system" ? <label className="form-field"><span>* 项目</span><select required className="text-input" value={form.projectId} onChange={(event) => setForm({ ...form, projectId: event.target.value, productId: "" })}><option value="">请选择项目</option>{projects.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label> : null}
      {form.scopeType === "product" ? <label className="form-field"><span>* 产品</span><select required className="text-input" value={form.productId} onChange={(event) => setForm({ ...form, productId: event.target.value })}><option value="">请选择产品</option>{scopedProducts.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label> : null}
      {form.scopeType !== "system" ? <label className="form-field"><span>环境名称</span><input className="text-input" placeholder="留空表示全部环境" value={form.envName} onChange={(event) => setForm({ ...form, envName: event.target.value })} /></label> : null}
      <label className="form-field"><span>* 变量名</span><input required className="text-input" placeholder="例如 access_token" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></label>
      <label className="form-field"><span>* 类型</span><select className="text-input" value={form.valueType} onChange={(event) => setForm({ ...form, valueType: event.target.value, sensitive: event.target.value === "secret" || form.sensitive })}>{["string", "number", "boolean", "json", "secret"].map((item) => <option key={item}>{item}</option>)}</select></label>
      <label className="form-field"><span>* 变量值</span><input required className="text-input" type={form.valueType === "secret" && form.value !== "******" ? "password" : "text"} value={form.value} onChange={(event) => setForm({ ...form, value: event.target.value })} /></label>
      <label className="form-field"><span>说明</span><input className="text-input" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} /></label>
      <label className="api-case-enabled"><input checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} type="checkbox" />启用变量</label>
      <label className="api-case-enabled"><input checked={form.sensitive || form.valueType === "secret"} disabled={form.valueType === "secret"} onChange={(event) => setForm({ ...form, sensitive: event.target.checked })} type="checkbox" />敏感变量（日志与报告脱敏）</label>
    </div><div className="modal-actions"><button className="icon-text-button compact-button" onClick={() => setModal(false)} type="button">取消</button><button className="primary-button compact-button" type="submit">保存变量</button></div></form></div> : null}
  </div>;
}

function scopeLabel(row) {
  if (row.scopeType === "system") return "系统";
  if (row.scopeType === "project") return `项目 / ${row.projectName}`;
  return `产品 / ${row.projectName} / ${row.productName}`;
}

