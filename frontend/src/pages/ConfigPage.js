import { Copy, KeyRound, RefreshCw } from "lucide-react";
import { useState } from "react";
import { DataTable, PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { configService } from "../services/configService.js";
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

  const resource = section === "测试对象" ? configService.testObjects : configService.projects;
  const { data, loading, error } = useAsyncData(() => resource.list({ page: 1, pageSize: 20 }), [section]);

  return (
    <>
      <PageHeader title={section} description="测试配置资产管理" />
      <StateBlock loading={loading} error={error}>
        <DataTable rows={pageItems(data)} columns={columnsFor(section)} />
      </StateBlock>
    </>
  );
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
          <div>
            <h2>产品模块配置 / {product.id} / {product.name}</h2>
            <p className="panel-subtitle">维护当前产品下的模块结构和模块名称</p>
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
            <p>按层级快速定位模块</p>
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

function ExecutorConfigPage() {
  const { data, loading, error, reload } = useAsyncData(() => configService.executorToken.get(), []);
  const [generated, setGenerated] = useState(null);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const token = generated?.token || "";
  const envText = token ? `EXECUTOR_SHARED_TOKEN=${token}` : "EXECUTOR_SHARED_TOKEN=请先生成 Token";

  async function handleGenerate() {
    setBusy(true);
    setNotice("");
    try {
      const result = await configService.executorToken.generate();
      setGenerated(result);
      await reload();
      setNotice("Token 已生成，请及时配置到执行器环境变量。");
    } catch (err) {
      setNotice(err.message);
    } finally {
      setBusy(false);
    }
  }

  async function handleCopy() {
    if (!token) {
      setNotice("请先生成 Token");
      return;
    }
    await navigator.clipboard.writeText(envText);
    setNotice("已复制环境变量配置");
  }

  return (
    <>
      <PageHeader title="执行器配置" description="维护平台与执行器之间的连接凭据" />
      <StateBlock loading={loading} error={error}>
        <div className="settings-panel">
          <div className="settings-row">
            <div>
              <div className="settings-title">
                <KeyRound size={18} />
                执行器共享 Token
              </div>
              <p>当前 Token：{generated?.maskedToken || data?.maskedToken || "未生成"}</p>
              <p>更新时间：{formatTime(generated?.updatedAt || data?.updatedAt)}</p>
            </div>
            <div className="settings-actions">
              <button className="icon-text-button" disabled={busy} onClick={handleGenerate} type="button">
                <RefreshCw size={16} />
                {busy ? "生成中" : "生成 Token"}
              </button>
              <button className="icon-text-button" onClick={handleCopy} type="button">
                <Copy size={16} />
                复制配置
              </button>
            </div>
          </div>
          <pre className="token-output">{envText}</pre>
          {notice ? <div className="inline-notice">{notice}</div> : null}
        </div>
      </StateBlock>
    </>
  );
}
