import { Activity, Copy, KeyRound, Plus, RefreshCw, Server, X } from "lucide-react";
import { useEffect, useState } from "react";
import { DataTable, PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { ResourceListPage } from "../components/ResourceListPage.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { configService } from "../services/configService.js";
import { executionService } from "../services/executionService.js";
import { formatTime, pageItems } from "../utils/formatters.js";
import { clearPageState, persistPageState, readPageState } from "../utils/routeState.js";

export function ConfigPage({ activePath }) {
  const section = activePath[1];
  if (section === "执行器配置") {
    return <ExecutorConfigPage />;
  }
  if (section === "项目配置") {
    return <ProjectConfigPage />;
  }
  if (section === "项目产品") {
    return <ProductConfigPage />;
  }
  if (section === "测试对象") {
    return <TestObjectConfigPage />;
  }

  const resource = configService.projects;
  const { data, loading, error } = useAsyncData(() => resource.list({ page: 1, pageSize: 20 }), [section]);

  return <ResourceListPage title={section} description="测试配置资产管理" panelTitle={`${section}列表`} rows={pageItems(data)} columns={columnsFor(section)} loading={loading} error={error} />;
}

function TestObjectConfigPage() {
  const [filters, setFilters] = useState({ id: "", envName: "", productId: "" });
  const [query, setQuery] = useState(filters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [modal, setModal] = useState(null);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const { data: projectsData } = useAsyncData(() => configService.projects.list({ page: 1, pageSize: 200 }), []);
  const { data: productsData } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 500 }), []);
  const projects = pageItems(projectsData);
  const products = pageItems(productsData);
  const { data, loading, error, reload } = useAsyncData(
    () => configService.testObjects.list({ ...query, page, pageSize }),
    [query.id, query.envName, query.productId, page, pageSize]
  );
  const rows = pageItems(data);
  const total = Number(data?.total || 0);

  async function remove(row) {
    if (!window.confirm(`确认删除测试环境“${row.envName}”吗？`)) return;
    setBusy(true);
    setNotice("");
    try {
      await configService.testObjects.remove(row.id);
      await reload();
      setNotice("测试对象已删除。");
    } catch (requestError) {
      setNotice(requestError.message || "删除测试对象失败");
    } finally {
      setBusy(false);
    }
  }

  const columns = [
    { key: "id", title: "ID" },
    { key: "productName", title: "产品" },
    { key: "envName", title: "环境名称" },
    { key: "target", title: "目标地址" },
    { key: "autoType", title: "自动化类型" },
    { key: "permission", title: "权限", render: (row) => `${row.queryEnabled ? "查询" : ""}${row.queryEnabled && row.writeEnabled ? " / " : ""}${row.writeEnabled ? "写入" : ""}` || "无" },
    { key: "owner", title: "负责人" },
    { key: "actions", title: "操作", render: (row) => <div className="action-row"><button className="link-button" onClick={() => setModal({ mode: "edit", row })} type="button">编辑</button><button className="danger-link" disabled={busy} onClick={() => remove(row)} type="button">删除</button></div> }
  ];

  return <div className="section-stack">
    <PageHeader title="测试对象" description="维护各产品在不同环境中的访问地址，供接口调试和测试用例执行选择" />
    <section className="resource-panel">
      <div className="panel-header"><strong>测试对象列表</strong></div>
      <form className="filter-grid" onSubmit={(event) => { event.preventDefault(); setPage(1); setQuery(filters); }}>
        <label className="form-field"><span>ID</span><input className="text-input" placeholder="测试对象 ID" value={filters.id} onChange={(event) => setFilters({ ...filters, id: event.target.value })} /></label>
        <label className="form-field"><span>环境名称</span><input className="text-input" placeholder="例如：测试环境" value={filters.envName} onChange={(event) => setFilters({ ...filters, envName: event.target.value })} /></label>
        <label className="form-field"><span>产品</span><select className="text-input" value={filters.productId} onChange={(event) => setFilters({ ...filters, productId: event.target.value })}><option value="">全部产品</option>{products.map((item) => <option key={item.id} value={item.id}>{item.projectName ? `${item.projectName} / ` : ""}{item.name}</option>)}</select></label>
        <div className="toolbar-row"><button className="primary-button compact-button" type="submit">搜索</button><button className="icon-text-button compact-button" onClick={() => { const empty = { id: "", envName: "", productId: "" }; setFilters(empty); setQuery(empty); setPage(1); }} type="button">重置</button></div>
      </form>
      <div className="list-actions"><div><span className="muted-text">共 {total} 个环境</span></div><button className="primary-button compact-button" onClick={() => setModal({ mode: "create", row: null })} type="button"><Plus size={14} />新增测试对象</button></div>
      {notice ? <div className="inline-notice">{notice}</div> : null}
      <StateBlock loading={loading} error={error}><TablePanel><DataTable columns={columns} rows={rows} emptyText="暂无测试对象" /><PaginationBar page={page} pageSize={pageSize} total={total} totalPages={Math.max(1, Math.ceil(total / pageSize))} onPageChange={setPage} onPageSizeChange={(value) => { setPage(1); setPageSize(value); }} /></TablePanel></StateBlock>
    </section>
    {modal ? <TestObjectModal busy={busy} modal={modal} products={products} projects={projects} onClose={() => setModal(null)} onSubmit={async (payload) => {
      setBusy(true);
      setNotice("");
      try {
        if (modal.mode === "edit") await configService.testObjects.update(modal.row.id, payload);
        else await configService.testObjects.create(payload);
        setModal(null);
        await reload();
        setNotice(modal.mode === "edit" ? "测试对象已更新。" : "测试对象已创建。");
      } finally {
        setBusy(false);
      }
    }} /> : null}
  </div>;
}

function TestObjectModal({ busy, modal, products, projects, onClose, onSubmit }) {
  const source = modal.row;
  const sourceProduct = products.find((item) => String(item.id) === String(source?.productId));
  const [form, setForm] = useState({
    projectId: sourceProduct?.projectId || "",
    productId: source?.productId || "",
    envName: source?.envName || "",
    target: source?.target || "",
    deployEnv: source?.deployEnv || "测试环境",
    autoType: source?.autoType || "接口自动化",
    owner: source?.owner || "",
    queryEnabled: source?.queryEnabled ?? true,
    writeEnabled: source?.writeEnabled ?? true
  });
  const [formError, setFormError] = useState("");
  const availableProducts = products.filter((item) => !form.projectId || String(item.projectId) === String(form.projectId));

  async function submit(event) {
    event.preventDefault();
    setFormError("");
    try {
      await onSubmit({
        productId: Number(form.productId), envName: form.envName.trim(), target: form.target.trim(),
        deployEnv: form.deployEnv, autoType: form.autoType, owner: form.owner.trim(),
        queryEnabled: form.queryEnabled, writeEnabled: form.writeEnabled
      });
    } catch (requestError) {
      setFormError(requestError.message || "保存测试对象失败");
    }
  }

  return <div className="modal-backdrop"><form className="modal-card modal-card-small" onSubmit={submit}>
    <div className="modal-header"><strong>{modal.mode === "edit" ? "编辑测试对象" : "新增测试对象"}</strong><button className="modal-close" onClick={onClose} type="button"><X size={17} /></button></div>
    <div className="modal-form">
      <label className="form-field"><span>* 所属项目</span><select required className="text-input" value={form.projectId} onChange={(event) => setForm({ ...form, projectId: event.target.value, productId: "" })}><option value="">请选择项目</option>{projects.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="form-field"><span>* 所属产品</span><select required className="text-input" value={form.productId} onChange={(event) => setForm({ ...form, productId: event.target.value })}><option value="">请选择产品</option>{availableProducts.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="form-field"><span>* 环境名称</span><input required className="text-input" placeholder="例如：测试环境" value={form.envName} onChange={(event) => setForm({ ...form, envName: event.target.value })} /></label>
      <label className="form-field"><span>* 目标地址</span><input required className="text-input" placeholder="https://api-test.example.com" value={form.target} onChange={(event) => setForm({ ...form, target: event.target.value })} /></label>
      <label className="form-field"><span>部署环境</span><select className="text-input" value={form.deployEnv} onChange={(event) => setForm({ ...form, deployEnv: event.target.value })}>{["开发环境", "测试环境", "预发布环境", "生产环境"].map((item) => <option key={item}>{item}</option>)}</select></label>
      <label className="form-field"><span>自动化类型</span><select className="text-input" value={form.autoType} onChange={(event) => setForm({ ...form, autoType: event.target.value })}><option>接口自动化</option><option>界面自动化</option><option>通用</option></select></label>
      <label className="form-field"><span>* 负责人</span><input required className="text-input" value={form.owner} onChange={(event) => setForm({ ...form, owner: event.target.value })} /></label>
      <div className="form-field"><span>执行权限</span><label><input checked={form.queryEnabled} onChange={(event) => setForm({ ...form, queryEnabled: event.target.checked })} type="checkbox" /> 允许查询请求</label><label><input checked={form.writeEnabled} onChange={(event) => setForm({ ...form, writeEnabled: event.target.checked })} type="checkbox" /> 允许写入请求</label></div>
    </div>
    {formError ? <div className="form-error modal-error">{formError}</div> : null}
    <div className="modal-actions"><button className="icon-text-button compact-button" onClick={onClose} type="button">取消</button><button className="primary-button compact-button" disabled={busy} type="submit">{busy ? "保存中" : "保存"}</button></div>
  </form></div>;
}

function columnsFor(section) {
  if (section === "项目产品") {
    return [
      { key: "id", title: "ID" },
      { key: "projectName", title: "项目" },
      { key: "name", title: "产品" },
      { key: "uiType", title: "UI 端" },
      { key: "apiType", title: "API 端" },
      { key: "updatedAt", title: "更新时间", render: (row) => formatTime(row.updatedAt) }
    ];
  }
  if (section === "测试对象") {
    return [
      { key: "id", title: "ID" },
      { key: "productName", title: "项目/产品" },
      { key: "envName", title: "环境" },
      { key: "target", title: "测试对象" },
      { key: "owner", title: "负责人" }
    ];
  }
  return [
    { key: "id", title: "ID" },
    { key: "name", title: "项目名称" },
    { key: "status", title: "状态" },
    { key: "updatedAt", title: "更新时间", render: (row) => formatTime(row.updatedAt) }
  ];
}

function ProjectConfigPage() {
  const [form, setForm] = useState({ id: "", name: "" });
  const [filters, setFilters] = useState({ id: "", name: "" });
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [modal, setModal] = useState(null);
  const [selectedIds, setSelectedIds] = useState([]);

  const { data, loading, error, reload } = useAsyncData(
    () => configService.projects.list({ ...filters, page, pageSize }),
    [filters.id, filters.name, page, pageSize]
  );

  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const allSelected = rows.length > 0 && rows.every((row) => selectedIds.includes(row.id));

  function handleSearch(event) {
    event.preventDefault();
    setSelectedIds([]);
    setPage(1);
    setFilters({ ...form });
  }

  function handleReset() {
    setForm({ id: "", name: "" });
    setFilters({ id: "", name: "" });
    setPage(1);
    setPageSize(20);
    setSelectedIds([]);
    setNotice("");
  }

  function toggleSelectAll() {
    if (allSelected) {
      setSelectedIds([]);
      return;
    }
    setSelectedIds(rows.map((row) => row.id));
  }

  function toggleSelectOne(id) {
    setSelectedIds((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  }

  async function handleDelete(row) {
    if (!window.confirm(`确认删除项目“${row.name}”吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await configService.projects.remove(row.id);
      setSelectedIds((current) => current.filter((id) => id !== row.id));
      await reload();
      setNotice("项目已删除。");
    } catch (err) {
      setNotice(err.message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  async function handleBulkDelete() {
    if (!selectedIds.length) {
      setNotice("请先选择需要删除的项目。");
      return;
    }
    if (!window.confirm(`确认删除已选中的 ${selectedIds.length} 个项目吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await Promise.all(selectedIds.map((id) => configService.projects.remove(id)));
      setSelectedIds([]);
      await reload();
      setNotice("已删除选中的项目。");
    } catch (err) {
      setNotice(err.message || "批量删除失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="section-stack">
      <PageHeader title="项目配置" description="维护测试项目基础信息" />
      <section className="resource-panel">
        <div className="panel-header">
          <strong>项目列表</strong>
        </div>

        <form className="filter-grid" onSubmit={handleSearch}>
          <label className="form-field">
            <span>ID</span>
            <input className="text-input" placeholder="请输入项目ID" value={form.id} onChange={(event) => setForm({ ...form, id: event.target.value })} />
          </label>
          <label className="form-field">
            <span>项目名称</span>
            <input className="text-input" placeholder="请输入项目名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </label>
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
          <div className="toolbar-row">
            <button className="primary-button compact-button" type="submit">
              搜索
            </button>
            <button className="icon-text-button compact-button" onClick={handleReset} type="button">
              重置
            </button>
          </div>
        </form>

        <div className="list-actions">
          <div />
          <div className="action-row">
            <button className="primary-button compact-button" onClick={() => setModal({ mode: "create", row: null })} type="button">
              新增
            </button>
            <button className="danger-button compact-button" disabled={busy} onClick={handleBulkDelete} type="button">
              批量删除
            </button>
          </div>
        </div>

        {notice ? <div className="inline-notice">{notice}</div> : null}

        <StateBlock loading={loading} error={error}>
          <TablePanel>
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th className="checkbox-cell">
                      <input checked={allSelected} onChange={toggleSelectAll} type="checkbox" />
                    </th>
                    <th>ID</th>
                    <th>项目名称</th>
                    <th>状态</th>
                    <th>更新时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.length ? (
                    rows.map((row) => (
                      <tr key={row.id}>
                        <td className="checkbox-cell">
                          <input checked={selectedIds.includes(row.id)} onChange={() => toggleSelectOne(row.id)} type="checkbox" />
                        </td>
                        <td>{row.id}</td>
                        <td>{row.name}</td>
                        <td>{row.status === "active" ? "启用" : "停用"}</td>
                        <td>{formatTime(row.updatedAt)}</td>
                        <td>
                          <div className="action-links">
                            <button className="link-button" onClick={() => setModal({ mode: "edit", row })} type="button">
                              编辑
                            </button>
                            <button className="link-button danger-link" disabled={busy} onClick={() => handleDelete(row)} type="button">
                              删除
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan="6">暂无项目数据</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            <PaginationBar
              page={page}
              pageSize={pageSize}
              total={total}
              totalPages={totalPages}
              onPageChange={setPage}
              onPageSizeChange={(value) => {
                setPage(1);
                setPageSize(value);
              }}
            />
          </TablePanel>
        </StateBlock>
      </section>

      {modal ? (
        <ProjectModal
          busy={busy}
          modal={modal}
          onClose={() => setModal(null)}
          onSubmit={async (payload, mode) => {
            setBusy(true);
            setNotice("");
            try {
              if (mode === "edit") {
                await configService.projects.update(modal.row.id, payload);
                setNotice("项目已更新。");
              } else {
                await configService.projects.create(payload);
                setNotice("项目已创建。");
              }
              setModal(null);
              await reload();
            } catch (err) {
              setNotice(err.message || "保存失败");
            } finally {
              setBusy(false);
            }
          }}
        />
      ) : null}
    </div>
  );
}

function ProjectModal({ busy, modal, onClose, onSubmit }) {
  const source = modal.row;
  const [form, setForm] = useState({
    name: source?.name || "",
    status: source?.status || "active"
  });
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    if (!form.name.trim()) {
      setError("项目名称不能为空。");
      return;
    }
    await onSubmit(
      {
        name: form.name.trim(),
        status: form.status
      },
      modal.mode
    );
  }

  return (
    <div className="modal-backdrop">
      <form className="modal-card modal-card-small" onSubmit={handleSubmit}>
        <div className="modal-header">
          <strong>{modal.mode === "edit" ? "编辑项目" : "新增项目"}</strong>
          <button className="modal-close" onClick={onClose} type="button">
            ×
          </button>
        </div>
        <div className="modal-form">
          <label className="form-field form-field-inline required-field">
            <span>项目名称</span>
            <input className="text-input" placeholder="请输入项目名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </label>
          <label className="form-field form-field-inline">
            <span>状态</span>
            <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              <option value="active">启用</option>
              <option value="disabled">停用</option>
            </select>
          </label>
        </div>
        {error ? <div className="form-error modal-error">{error}</div> : null}
        <div className="modal-actions">
          <button className="icon-text-button compact-button" onClick={onClose} type="button">
            取消
          </button>
          <button className="primary-button compact-button" disabled={busy} type="submit">
            {busy ? "提交中" : "提交"}
          </button>
        </div>
      </form>
    </div>
  );
}

function ProductConfigPage() {
  const [form, setForm] = useState({ id: "", name: "" });
  const [filters, setFilters] = useState({ id: "", name: "" });
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [modal, setModal] = useState(null);
  const [selectedIds, setSelectedIds] = useState([]);
  const [projects, setProjects] = useState([]);
  const [selectedProduct, setSelectedProduct] = useState(() => readPageState("config.product.selectedProduct", null));

  const { data, loading, error, reload } = useAsyncData(
    () => configService.products.list({ ...filters, page, pageSize }),
    [filters.id, filters.name, page, pageSize]
  );

  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const allSelected = rows.length > 0 && rows.every((row) => selectedIds.includes(row.id));

  async function loadProjects() {
    try {
      const result = await configService.projects.list({ page: 1, pageSize: 200 });
      setProjects(pageItems(result) || []);
    } catch {
      setProjects([]);
    }
  }

  function handleSearch(event) {
    event.preventDefault();
    setSelectedIds([]);
    setPage(1);
    setFilters({ ...form });
  }

  function handleReset() {
    setForm({ id: "", name: "" });
    setFilters({ id: "", name: "" });
    setPage(1);
    setPageSize(20);
    setSelectedIds([]);
    setNotice("");
  }

  function toggleSelectAll() {
    if (allSelected) {
      setSelectedIds([]);
      return;
    }
    setSelectedIds(rows.map((row) => row.id));
  }

  function toggleSelectOne(id) {
    setSelectedIds((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  }

  async function handleDelete(row) {
    if (!window.confirm(`确认删除产品"${row.name}"吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await configService.products.remove(row.id);
      setSelectedIds((current) => current.filter((id) => id !== row.id));
      await reload();
      setNotice("产品已删除。");
    } catch (err) {
      setNotice(err.message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  async function handleBulkDelete() {
    if (!selectedIds.length) {
      setNotice("请先选择需要删除的产品。");
      return;
    }
    if (!window.confirm(`确认删除已选中的 ${selectedIds.length} 个产品吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await Promise.all(selectedIds.map((id) => configService.products.remove(id)));
      setSelectedIds([]);
      await reload();
      setNotice("已删除选中的产品。");
    } catch (err) {
      setNotice(err.message || "批量删除失败");
    } finally {
      setBusy(false);
    }
  }

  if (selectedProduct) {
    return (
      <ProductModulePage
        product={selectedProduct}
        onBack={() => {
          clearPageState("config.product.selectedProduct");
          setSelectedProduct(null);
        }}
      />
    );
  }

  return (
    <div className="section-stack">
      <PageHeader title="项目产品" description="维护测试项目下的产品信息" />
      <section className="resource-panel">
        <div className="panel-header">
          <strong>产品列表</strong>
        </div>

        <form className="filter-grid" onSubmit={handleSearch}>
          <label className="form-field">
            <span>ID</span>
            <input className="text-input" placeholder="请输入产品ID" value={form.id} onChange={(event) => setForm({ ...form, id: event.target.value })} />
          </label>
          <label className="form-field">
            <span>产品名称</span>
            <input className="text-input" placeholder="请输入产品名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </label>
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
          <div className="toolbar-row">
            <button className="primary-button compact-button" type="submit">
              搜索
            </button>
            <button className="icon-text-button compact-button" onClick={handleReset} type="button">
              重置
            </button>
          </div>
        </form>

        <div className="list-actions">
          <div />
          <div className="action-row">
            <button className="primary-button compact-button" onClick={() => { loadProjects(); setModal({ mode: "create", row: null }); }} type="button">
              新增
            </button>
            <button className="danger-button compact-button" disabled={busy} onClick={handleBulkDelete} type="button">
              批量删除
            </button>
          </div>
        </div>

        {notice ? <div className="inline-notice">{notice}</div> : null}

        <StateBlock loading={loading} error={error}>
          <TablePanel>
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th className="checkbox-cell">
                      <input checked={allSelected} onChange={toggleSelectAll} type="checkbox" />
                    </th>
                    <th>ID</th>
                    <th>项目</th>
                    <th>产品名称</th>
                    <th>UI 端</th>
                    <th>API 端</th>
                    <th>更新时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.length ? (
                    rows.map((row) => (
                      <tr key={row.id}>
                        <td className="checkbox-cell">
                          <input checked={selectedIds.includes(row.id)} onChange={() => toggleSelectOne(row.id)} type="checkbox" />
                        </td>
                        <td>{row.id}</td>
                        <td>{row.projectName || "-"}</td>
                        <td>{row.name}</td>
                        <td>
                          <span className="table-badge">{row.uiType || "WEB"}</span>
                        </td>
                        <td>
                          <span className="table-badge">{row.apiType || "WEB"}</span>
                        </td>
                        <td>{formatTime(row.updatedAt)}</td>
                        <td>
                          <div className="action-links">
                            <button
                              className="link-button"
                              onClick={() => {
                                persistPageState("config.product.selectedProduct", row);
                                setSelectedProduct(row);
                              }}
                              type="button"
                            >
                              模块
                            </button>
                            <button className="link-button" onClick={() => { loadProjects(); setModal({ mode: "edit", row }); }} type="button">
                              编辑
                            </button>
                            <button className="link-button danger-link" disabled={busy} onClick={() => handleDelete(row)} type="button">
                              删除
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan="8">暂无产品数据</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            <PaginationBar
              page={page}
              pageSize={pageSize}
              total={total}
              totalPages={totalPages}
              onPageChange={setPage}
              onPageSizeChange={(value) => {
                setPage(1);
                setPageSize(value);
              }}
            />
          </TablePanel>
        </StateBlock>
      </section>

      {modal ? (
        <ProductModal
          busy={busy}
          modal={modal}
          projects={projects}
          onClose={() => setModal(null)}
          onSubmit={async (payload, mode) => {
            setBusy(true);
            setNotice("");
            try {
              if (mode === "edit") {
                await configService.products.update(modal.row.id, payload);
                setNotice("产品已更新。");
              } else {
                await configService.products.create(payload);
                setNotice("产品已创建。");
              }
              setModal(null);
              await reload();
            } catch (err) {
              setNotice(err.message || "保存失败");
            } finally {
              setBusy(false);
            }
          }}
        />
      ) : null}
    </div>
  );
}

function ProductModal({ busy, modal, projects, onClose, onSubmit }) {
  const source = modal.row;
  const [form, setForm] = useState({
    projectId: source?.projectId || "",
    name: source?.name || "",
    uiType: source?.uiType || "WEB",
    apiType: source?.apiType || "WEB"
  });
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    if (!form.projectId || !form.name.trim()) {
      setError("所属项目和产品名称不能为空。");
      return;
    }
    await onSubmit(
      {
        projectId: Number(form.projectId),
        name: form.name.trim(),
        uiType: form.uiType,
        apiType: form.apiType
      },
      modal.mode
    );
  }

  const projectOptions = projects.map((p) => ({ id: p.id, name: p.name }));
  if (source?.projectId && !projectOptions.find((p) => p.id === source.projectId)) {
    projectOptions.push({ id: source.projectId, name: source.projectName || `项目 ${source.projectId}` });
  }

  return (
    <div className="modal-backdrop">
      <form className="modal-card modal-card-small" onSubmit={handleSubmit}>
        <div className="modal-header">
          <strong>{modal.mode === "edit" ? "编辑产品" : "新增产品"}</strong>
          <button className="modal-close" onClick={onClose} type="button">
            ×
          </button>
        </div>
        <div className="modal-form">
          <label className="form-field form-field-inline required-field">
            <span>所属项目</span>
            <select className="text-input" value={form.projectId} onChange={(event) => setForm({ ...form, projectId: event.target.value })}>
              <option value="">请选择项目</option>
              {projectOptions.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field form-field-inline required-field">
            <span>产品名称</span>
            <input className="text-input" placeholder="请输入产品名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </label>
          <label className="form-field form-field-inline">
            <span>UI 端</span>
            <select className="text-input" value={form.uiType} onChange={(event) => setForm({ ...form, uiType: event.target.value })}>
              <option value="WEB">WEB</option>
              <option value="APP">APP</option>
              <option value="H5">H5</option>
            </select>
          </label>
          <label className="form-field form-field-inline">
            <span>API 端</span>
            <select className="text-input" value={form.apiType} onChange={(event) => setForm({ ...form, apiType: event.target.value })}>
              <option value="WEB">WEB</option>
              <option value="APP">APP</option>
              <option value="H5">H5</option>
            </select>
          </label>
        </div>
        {error ? <div className="form-error modal-error">{error}</div> : null}
        <div className="modal-actions">
          <button className="icon-text-button compact-button" onClick={onClose} type="button">
            取消
          </button>
          <button className="primary-button compact-button" disabled={busy} type="submit">
            {busy ? "提交中" : "提交"}
          </button>
        </div>
      </form>
    </div>
  );
}

function ProductModulePage({ product, onBack }) {
  const [form, setForm] = useState({ level1: "", level2: "", name: "" });
  const [filters, setFilters] = useState({ level1: "", level2: "", name: "" });
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [modal, setModal] = useState(null);

  const { data, loading, error, reload } = useAsyncData(
    () => configService.productModules.list({ productId: product.id, ...filters, page, pageSize }),
    [product.id, filters.level1, filters.level2, filters.name, page, pageSize]
  );

  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const allRows = pageItems(data);
  const level1Options = uniqueValues(allRows, "level1");
  const level2Options = uniqueValues(allRows, "level2");

  function handleSearch(event) {
    event.preventDefault();
    setPage(1);
    setFilters({ ...form });
  }

  function handleReset() {
    setForm({ level1: "", level2: "", name: "" });
    setFilters({ level1: "", level2: "", name: "" });
    setPage(1);
    setPageSize(20);
    setNotice("");
  }

  async function handleDelete(row) {
    if (!window.confirm(`确认删除模块“${row.name}”吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await configService.productModules.remove(row.id);
      await reload();
      setNotice("模块已删除。");
    } catch (err) {
      setNotice(err.message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="section-stack">
      <section className="resource-panel product-module-page">
        <div className="page-config-header">
          <div className="toolbar-title">
            <strong>产品模块配置 / {product.id} / {product.name}</strong>
          </div>
          <div className="action-row">
            <button className="primary-button compact-button" onClick={() => setModal({ mode: "create", row: null })} type="button">
              增加
            </button>
            <button className="icon-text-button compact-button" onClick={onBack} type="button">
              返回
            </button>
          </div>
        </div>

        <div className="module-config-layout">
          <aside className="module-nav-panel">
            <div className="module-nav-title">
              <strong>模块导航</strong>
              <span>{total}</span>
            </div>
            <input className="text-input" placeholder="搜索模块" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
            <div className="module-chip-row">
              <span>一级 {level1Options.length}</span>
              <span>二级 {level2Options.length}</span>
              <span>模块 {total}</span>
            </div>
            <div className="module-tree">
              <button className="module-tree-item active" onClick={() => { setForm({ level1: "", level2: "", name: "" }); setFilters({ level1: "", level2: "", name: "" }); }} type="button">
                全部模块 ({total})
              </button>
              <button className="module-tree-item" type="button">
                未分组 ({rows.filter((row) => !row.level1 && !row.level2).length})
              </button>
            </div>
          </aside>

          <main className="module-table-panel">
            <div className="module-panel-header">
              <div>
                <strong>全部模块</strong>
                <p>筛选、编辑和删除当前产品下的模块</p>
              </div>
              <button className="icon-text-button compact-button" onClick={handleReset} type="button">
                重置筛选
              </button>
            </div>

            <form className="module-filter-grid" onSubmit={handleSearch}>
              <label className="form-field">
                <span>一级模块</span>
                <select className="text-input" value={form.level1} onChange={(event) => setForm({ ...form, level1: event.target.value })}>
                  <option value="">全部一级模块</option>
                  {level1Options.map((item) => (
                    <option key={item} value={item}>
                      {item}
                    </option>
                  ))}
                </select>
              </label>
              <label className="form-field">
                <span>二级模块</span>
                <select className="text-input" value={form.level2} onChange={(event) => setForm({ ...form, level2: event.target.value })}>
                  <option value="">全部二级模块</option>
                  {level2Options.map((item) => (
                    <option key={item} value={item}>
                      {item}
                    </option>
                  ))}
                </select>
              </label>
              <label className="form-field">
                <span>模块名称</span>
                <input className="text-input" placeholder="搜索实际模块" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
              </label>
              <div className="toolbar-row">
                <button className="primary-button compact-button" type="submit">
                  搜索
                </button>
              </div>
            </form>

            {notice ? <div className="inline-notice">{notice}</div> : null}

            <StateBlock loading={loading} error={error}>
              <TablePanel>
                <div className="table-wrap">
                  <table className="data-table">
                    <thead>
                      <tr>
                        <th>序号</th>
                        <th>三级模块（模块名称）</th>
                        <th>一级模块</th>
                        <th>二级模块</th>
                        <th>创建时间</th>
                        <th>更新时间</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {rows.length ? (
                        rows.map((row) => (
                          <tr key={row.id}>
                            <td>{row.id}</td>
                            <td>{row.name}</td>
                            <td>{row.level1 || "-"}</td>
                            <td>{row.level2 || "-"}</td>
                            <td>{formatTime(row.createdAt)}</td>
                            <td>{formatTime(row.updatedAt)}</td>
                            <td>
                              <div className="action-links">
                                <button className="link-button" onClick={() => setModal({ mode: "edit", row })} type="button">
                                  编辑
                                </button>
                                <button className="link-button danger-link" disabled={busy} onClick={() => handleDelete(row)} type="button">
                                  删除
                                </button>
                              </div>
                            </td>
                          </tr>
                        ))
                      ) : (
                        <tr>
                          <td colSpan="7">暂无模块数据</td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
                <PaginationBar
                  page={page}
                  pageSize={pageSize}
                  total={total}
                  totalPages={totalPages}
                  onPageChange={setPage}
                  onPageSizeChange={(value) => {
                    setPage(1);
                    setPageSize(value);
                  }}
                />
              </TablePanel>
            </StateBlock>
          </main>
        </div>
      </section>

      {modal ? (
        <ProductModuleModal
          busy={busy}
          modal={modal}
          onClose={() => setModal(null)}
          onSubmit={async (payload, mode) => {
            setBusy(true);
            setNotice("");
            try {
              if (mode === "edit") {
                await configService.productModules.update(modal.row.id, { ...payload, productId: product.id });
                setNotice("模块已更新。");
              } else {
                await configService.productModules.create({ ...payload, productId: product.id });
                setNotice("模块已创建。");
              }
              setModal(null);
              await reload();
            } catch (err) {
              setNotice(err.message || "保存失败");
            } finally {
              setBusy(false);
            }
          }}
        />
      ) : null}
    </div>
  );
}

function ProductModuleModal({ busy, modal, onClose, onSubmit }) {
  const source = modal.row;
  const [form, setForm] = useState({
    name: source?.name || "",
    level1: source?.level1 || "",
    level2: source?.level2 || ""
  });
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    if (!form.name.trim()) {
      setError("模块名称不能为空。");
      return;
    }
    await onSubmit(
      {
        name: form.name.trim(),
        level1: form.level1.trim(),
        level2: form.level2.trim()
      },
      modal.mode
    );
  }

  return (
    <div className="modal-backdrop">
      <form className="modal-card modal-card-small" onSubmit={handleSubmit}>
        <div className="modal-header">
          <strong>{modal.mode === "edit" ? "编辑模块" : "新增模块"}</strong>
          <button className="modal-close" onClick={onClose} type="button">
            ×
          </button>
        </div>
        <div className="modal-form">
          <label className="form-field form-field-inline">
            <span>一级模块</span>
            <input className="text-input" placeholder="请输入一级模块" value={form.level1} onChange={(event) => setForm({ ...form, level1: event.target.value })} />
          </label>
          <label className="form-field form-field-inline">
            <span>二级模块</span>
            <input className="text-input" placeholder="请输入二级模块" value={form.level2} onChange={(event) => setForm({ ...form, level2: event.target.value })} />
          </label>
          <label className="form-field form-field-inline required-field">
            <span>模块名称</span>
            <input className="text-input" placeholder="请输入模块名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </label>
        </div>
        {error ? <div className="form-error modal-error">{error}</div> : null}
        <div className="modal-actions">
          <button className="icon-text-button compact-button" onClick={onClose} type="button">
            取消
          </button>
          <button className="primary-button compact-button" disabled={busy} type="submit">
            {busy ? "提交中" : "提交"}
          </button>
        </div>
      </form>
    </div>
  );
}

function uniqueValues(rows, key) {
  return [...new Set(rows.map((row) => row[key]).filter(Boolean))];
}

function executorStatusMeta(status) {
  if (status === "online") return { className: "is-online", label: "在线" };
  if (status === "pending" || status === "registered") return { className: "is-pending", label: "待连接" };
  if (status === "suspect") return { className: "is-pending", label: "连接异常" };
  return { className: "is-offline", label: "离线" };
}

function ExecutorConfigPage() {
  const {
    data: executors,
    loading: executorsLoading,
    error: executorsError,
    reload: reloadExecutors
  } = useAsyncData(() => executionService.executors(), []);
  const [generatedTokens, setGeneratedTokens] = useState({});
  const [busyExecutorId, setBusyExecutorId] = useState("");
  const [notice, setNotice] = useState("");
  const [createModal, setCreateModal] = useState(false);
  const [createForm, setCreateForm] = useState({ executorId: "", name: "" });

  const executorRows = Array.isArray(executors) ? executors : [];
  const onlineCount = executorRows.filter((item) => item.status === "online").length;

  useEffect(() => {
    const timer = window.setInterval(() => reloadExecutors({ silent: true }), 10000);
    return () => window.clearInterval(timer);
  }, [reloadExecutors]);

  async function handleGenerate(executorId) {
    setBusyExecutorId(executorId);
    setNotice("");
    try {
      const result = await executionService.generateExecutorToken(executorId);
      setGeneratedTokens((current) => ({ ...current, [executorId]: result.token }));
      setNotice(`已为执行器 ${executorId} 生成专属 Token，旧 Token 已失效。`);
    } catch (err) {
      setNotice(err.message);
    } finally {
      setBusyExecutorId("");
    }
  }

  async function handleCopy(executorId) {
    const token = generatedTokens[executorId];
    await navigator.clipboard.writeText(`EXECUTOR_ID=${executorId}\nEXECUTOR_SHARED_TOKEN=${token}`);
    setNotice(`已复制执行器 ${executorId} 的专属配置。`);
  }

  async function handleCreate(event) {
    event.preventDefault();
    setBusyExecutorId("__create__");
    setNotice("");
    try {
      const result = await executionService.createExecutor({ executorId: createForm.executorId.trim(), name: createForm.name.trim() });
      setGeneratedTokens((current) => ({ ...current, [result.executorId]: result.token }));
      await reloadExecutors();
      setCreateModal(false);
      setCreateForm({ executorId: "", name: "" });
      setNotice(`执行器 ${result.executorId} 已创建，请复制专属 Token 完成连接。`);
    } catch (err) {
      setNotice(err.message);
    } finally {
      setBusyExecutorId("");
    }
  }

  return (
    <>
      <PageHeader title="执行器配置" description="每个执行器使用独立凭据，状态和权限互不影响" />
      <section className="resource-panel executor-status-panel">
        <div className="panel-header">
          <div>
            <div className="settings-title"><Activity size={18} />执行器运行状态</div>
            <p className="panel-description">在线 {onlineCount} / 共 {executorRows.length} 个，状态每 10 秒自动更新</p>
          </div>
          <div className="settings-actions">
            <button className="icon-text-button compact-button" onClick={() => setCreateModal(true)} type="button"><Plus size={15} />新增执行器</button>
            <button className="icon-text-button compact-button" disabled={executorsLoading} onClick={() => reloadExecutors()} type="button"><RefreshCw className={executorsLoading ? "spin-icon" : ""} size={15} />刷新状态</button>
          </div>
        </div>
        <StateBlock loading={executorsLoading} error={executorsError}>
          {executorRows.length ? <div className="executor-card-grid">
            {executorRows.map((item) => {
              const statusMeta = executorStatusMeta(item.status);
              return <article className={`executor-card ${statusMeta.className}`} key={item.executorId}>
                <div className="executor-card-head">
                  <div className="executor-name-cell"><span className="executor-icon"><Server size={17} /></span><div><strong>{item.name || item.executorId}</strong><small>{item.executorId}</small></div></div>
                  <span className={`executor-state ${statusMeta.className}`}><i />{statusMeta.label}</span>
                </div>
                <div className="executor-load-grid">
                  <div><span>运行中</span><strong>{item.runningTasks || 0}</strong></div>
                  <div><span>排队中</span><strong>{item.queuedTasks || 0}</strong></div>
                  <div><span>最大并发</span><strong>{item.maxWorkers || 1}</strong></div>
                </div>
                <div className="executor-card-detail">
                  <div><span>服务地址</span><code className="executor-endpoint">{item.endpoint || "-"}</code></div>
                  <div><span>最后心跳</span><strong>{formatTime(item.lastHeartbeatAt)}</strong></div>
                </div>
                <div className="executor-card-footer">
                  <span>支持类型</span>
                  <div className="executor-capabilities">{(item.supportedTypes || []).map((type) => <span key={type}>{type.toUpperCase()}</span>)}</div>
                </div>
                <div className="executor-token-zone">
                  <div className="executor-token-actions">
                    <span><KeyRound size={14} />专属 Token</span>
                    <button className="link-button" disabled={Boolean(busyExecutorId)} onClick={() => handleGenerate(item.executorId)} type="button">
                      {busyExecutorId === item.executorId ? "生成中" : generatedTokens[item.executorId] ? "重新生成" : "生成 Token"}
                    </button>
                  </div>
                  {generatedTokens[item.executorId] ? <div className="executor-token-result"><code>{generatedTokens[item.executorId]}</code><button aria-label={`复制 ${item.executorId} 配置`} onClick={() => handleCopy(item.executorId)} type="button"><Copy size={14} /></button></div> : <small>Token 仅在生成后显示一次，请立即复制到对应执行器。</small>}
                </div>
              </article>;
            })}
          </div> : <div className="executor-empty">暂无已注册执行器，请启动执行器并确认 Token 配置正确。</div>}
        </StateBlock>
        {notice ? <div className="inline-notice">{notice}</div> : null}
      </section>
      {createModal ? <div className="modal-backdrop">
        <section aria-label="新增执行器" className="modal-card executor-create-modal">
          <div className="modal-header"><div><strong>新增执行器</strong><p>创建独立身份并生成一对一连接 Token</p></div><button aria-label="关闭新增执行器" className="modal-close" onClick={() => setCreateModal(false)} type="button"><X size={17} /></button></div>
          <form onSubmit={handleCreate}>
            <label className="form-field"><span>执行器 ID</span><input className="text-input" onChange={(event) => setCreateForm((current) => ({ ...current, executorId: event.target.value }))} placeholder="例如 executor-beijing-01" required value={createForm.executorId} /></label>
            <label className="form-field"><span>执行器名称</span><input className="text-input" onChange={(event) => setCreateForm((current) => ({ ...current, name: event.target.value }))} placeholder="例如 北京 UI 执行器" required value={createForm.name} /></label>
            <div className="executor-create-hint"><KeyRound size={15} /><span>创建后 Token 仅显示一次，需要粘贴到对应执行器登录页。</span></div>
            <div className="modal-actions"><button className="icon-text-button compact-button" onClick={() => setCreateModal(false)} type="button">取消</button><button className="primary-button compact-button" disabled={busyExecutorId === "__create__"} type="submit">{busyExecutorId === "__create__" ? "创建中" : "创建并生成 Token"}</button></div>
          </form>
        </section>
      </div> : null}
    </>
  );
}
