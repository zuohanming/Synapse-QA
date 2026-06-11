import { useMemo, useRef, useState } from "react";
import { DataTable, PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { configService } from "../services/configService.js";
import { uiAutomationService } from "../services/uiAutomationService.js";
import { formatTime, pageItems } from "../utils/formatters.js";

const listSectionMap = {
  页面步骤: uiAutomationService.steps,
  测试用例: uiAutomationService.cases,
  全局变量: uiAutomationService.variables
};

const initialFilters = {
  id: "",
  pageName: "",
  pageURL: "",
  product: "",
  module: ""
};

const initialStepFilters = {
  id: "",
  stepName: "",
  product: "",
  module: "",
  page: "",
  status: ""
};

const emptyPageForm = {
  name: "",
  category: "",
  method: "",
  locator: "",
  value: "WEB",
  description: "",
  status: "active"
};

const emptyStepForm = {
  name: "",
  category: "",
  method: "",
  locator: "",
  description: "",
  status: "active"
};

const emptyElementForm = {
  name: "",
  type1: "xpath",
  locator1: "",
  index1: "",
  type2: "",
  locator2: "",
  index2: "",
  type3: "",
  locator3: "",
  index3: "",
  aiPrompt: "",
  waitTime: ""
};

export function UIAutomationPage({ activePath }) {
  const section = activePath[1];

  if (section === "页面元素") {
    return <PageElementsWorkspace />;
  }

  if (section === "页面步骤") {
    return <PageStepsPage />;
  }

  const resource = listSectionMap[section] || uiAutomationService.elements;
  const { data, loading, error } = useAsyncData(() => resource.list({ page: 1, pageSize: 20 }), [section]);

  return (
    <>
      <PageHeader title={section} description="界面自动化资产管理" />
      <StateBlock loading={loading} error={error}>
        <DataTable
          rows={pageItems(data)}
          columns={[
            { key: "id", title: "ID" },
            { key: "name", title: "名称" },
            { key: "category", title: "分类" },
            { key: "method", title: "模块/方法" },
            { key: "locator", title: "定位/地址" },
            { key: "status", title: "状态" },
            { key: "updatedAt", title: "更新时间", render: (row) => formatTime(row.updatedAt) }
          ]}
        />
      </StateBlock>
    </>
  );
}

function PageStepsPage() {
  const [form, setForm] = useState(initialStepFilters);
  const [filters, setFilters] = useState(initialStepFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [selectedIds, setSelectedIds] = useState([]);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [modal, setModal] = useState(null);
  const [workingStep, setWorkingStep] = useState(null);

  const { data, loading, error, reload } = useAsyncData(
    () =>
      uiAutomationService.steps.list({
        id: filters.id,
        pageName: filters.stepName,
        product: filters.product,
        module: filters.module,
        pageUrl: filters.page,
        page,
        pageSize
      }),
    [filters.id, filters.stepName, filters.product, filters.module, filters.page, page, pageSize]
  );

  const sourceRows = pageItems(data);
  const rows = filters.status ? sourceRows.filter((row) => row.status === filters.status) : sourceRows;
  const total = filters.status ? rows.length : data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const allSelected = rows.length > 0 && rows.every((row) => selectedIds.includes(row.id));

  const productOptions = useMemo(() => uniqueOptions(sourceRows, "category"), [sourceRows]);
  const moduleOptions = useMemo(() => uniqueOptions(sourceRows, "method"), [sourceRows]);
  const pageOptions = useMemo(() => uniqueOptions(sourceRows, "locator"), [sourceRows]);

  function handleSearch(event) {
    event.preventDefault();
    setSelectedIds([]);
    setPage(1);
    setFilters({ ...form });
  }

  function handleReset() {
    setForm(initialStepFilters);
    setFilters(initialStepFilters);
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

  async function handleBulkDelete() {
    if (!selectedIds.length) {
      setNotice("请先选择需要删除的页面步骤。");
      return;
    }
    if (!window.confirm(`确认删除已选中的 ${selectedIds.length} 个页面步骤吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await Promise.all(selectedIds.map((id) => uiAutomationService.steps.remove(id)));
      setSelectedIds([]);
      await reload();
      setNotice("已删除选中的页面步骤。");
    } catch (err) {
      setNotice(err.message || "批量删除失败");
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteRow(row) {
    if (!window.confirm(`确认删除页面步骤“${row.name}”吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await uiAutomationService.steps.remove(row.id);
      setSelectedIds((current) => current.filter((id) => id !== row.id));
      await reload();
      setNotice("页面步骤已删除。");
    } catch (err) {
      setNotice(err.message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  async function handleCopyRow(row) {
    setBusy(true);
    setNotice("");
    try {
      await uiAutomationService.steps.create({
        name: `${row.name || "步骤"}_copy`,
        category: row.category || "",
        method: row.method || "",
        locator: row.locator || "",
        action: row.action || "",
        value: row.value || "",
        description: row.description || "",
        status: row.status || "active"
      });
      await reload();
      setNotice("页面步骤已复制。");
    } catch (err) {
      setNotice(err.message || "复制失败");
    } finally {
      setBusy(false);
    }
  }

  if (workingStep) {
    return <StepWorkbench step={workingStep} onBack={() => setWorkingStep(null)} />;
  }

  return (
    <div className="section-stack">
      <PageHeader title="页面步骤" description="调试页面步骤" />

      <section className="resource-panel">
        <div className="panel-header">
          <strong>调试页面步骤</strong>
        </div>

        <form className="filter-grid filter-grid-steps" onSubmit={handleSearch}>
          <label className="form-field">
            <span>ID</span>
            <input className="text-input" value={form.id} placeholder="请输入步骤ID" onChange={(event) => setForm({ ...form, id: event.target.value })} />
          </label>
          <label className="form-field">
            <span>步骤名称</span>
            <input className="text-input" value={form.stepName} placeholder="请输入步骤名称" onChange={(event) => setForm({ ...form, stepName: event.target.value })} />
          </label>
          <label className="form-field">
            <span>项目/产品</span>
            <select className="text-input" value={form.product} onChange={(event) => setForm({ ...form, product: event.target.value })}>
              <option value="">请选择产品</option>
              {productOptions.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>模块名称</span>
            <select className="text-input" value={form.module} onChange={(event) => setForm({ ...form, module: event.target.value })}>
              <option value="">请选择产品</option>
              {moduleOptions.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>所属页面</span>
            <select className="text-input" value={form.page} onChange={(event) => setForm({ ...form, page: event.target.value })}>
              <option value="">请选择所属页面</option>
              {pageOptions.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>状态</span>
            <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              <option value="">请选择步骤状态</option>
              <option value="active">通过</option>
              <option value="disabled">失败</option>
            </select>
          </label>
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
                    <th>项目/产品</th>
                    <th>模块名称</th>
                    <th>所属页面</th>
                    <th>步骤名称</th>
                    <th>预估步骤顺序</th>
                    <th>状态</th>
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
                        <td>{row.category || "-"}</td>
                        <td>{row.method || "-"}</td>
                        <td>{row.locator || "-"}</td>
                        <td>{row.name || "-"}</td>
                        <td className="cell-ellipsis cell-wide" title={row.description || ""}>
                          {row.description || "-"}
                        </td>
                        <td>
                          <span className={row.status === "disabled" ? "status-badge status-failed" : "status-badge status-passed"}>
                            {row.status === "disabled" ? "失败" : "通过"}
                          </span>
                        </td>
                        <td>
                          <div className="action-links">
                            <button className="link-button" type="button">
                              调试
                            </button>
                            <button className="link-button" onClick={() => setWorkingStep(row)} type="button">
                              步骤
                            </button>
                            <details className="more-menu">
                              <summary>更多</summary>
                              <div className="more-menu-panel">
                                <button className="link-button" disabled={busy} onClick={() => handleCopyRow(row)} type="button">
                                  复制
                                </button>
                                <button className="link-button danger-link" disabled={busy} onClick={() => handleDeleteRow(row)} type="button">
                                  删除
                                </button>
                              </div>
                            </details>
                          </div>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan="9">暂无页面步骤数据</td>
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
        <StepModal
          busy={busy}
          modal={modal}
          productOptions={productOptions}
          moduleOptions={moduleOptions}
          pageOptions={pageOptions}
          onClose={() => setModal(null)}
          onSubmit={async (payload, mode) => {
            setBusy(true);
            setNotice("");
            try {
              if (mode === "edit") {
                await uiAutomationService.steps.update(modal.row.id, payload);
                setNotice("页面步骤已更新。");
              } else {
                await uiAutomationService.steps.create(payload);
                setNotice("页面步骤已创建。");
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

function StepModal({ busy, modal, productOptions, moduleOptions, pageOptions, onClose, onSubmit }) {
  const source = modal.row;
  const [form, setForm] = useState(
    source
      ? {
          name: source.name || "",
          category: source.category || "",
          method: source.method || "",
          locator: source.locator || "",
          description: source.description || "",
          status: source.status || "active"
        }
      : emptyStepForm
  );
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    if (!form.category.trim() || !form.method.trim() || !form.locator.trim() || !form.name.trim()) {
      setError("项目/产品、模块名称、所属页面和步骤名称不能为空。");
      return;
    }
    await onSubmit(
      {
        name: form.name.trim(),
        category: form.category.trim(),
        method: form.method.trim(),
        locator: form.locator.trim(),
        action: "",
        value: "",
        description: form.description.trim(),
        status: form.status || "active"
      },
      modal.mode
    );
  }

  return (
    <div className="modal-backdrop">
      <form className="modal-card modal-card-small step-modal-card" onSubmit={handleSubmit}>
        <div className="modal-header">
          <strong>{modal.mode === "edit" ? "编辑" : "新增"}</strong>
          <button className="modal-close" onClick={onClose} type="button">
            ×
          </button>
        </div>
        <div className="modal-form">
          <label className="form-field form-field-inline required-field">
            <span>项目/产品</span>
            <select className="text-input" value={form.category} onChange={(event) => setForm({ ...form, category: event.target.value })}>
              <option value="">请选择项目名称</option>
              {ensureOptions(productOptions, form.category).map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field form-field-inline required-field">
            <span>模块名称</span>
            <select className="text-input" value={form.method} onChange={(event) => setForm({ ...form, method: event.target.value })}>
              <option value="">请选择测试模块</option>
              {ensureOptions(moduleOptions, form.method).map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field form-field-inline required-field">
            <span>所属页面</span>
            <select className="text-input" value={form.locator} onChange={(event) => setForm({ ...form, locator: event.target.value })}>
              <option value="">请选择步骤所属页面</option>
              {ensureOptions(pageOptions, form.locator).map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field form-field-inline required-field">
            <span>步骤名称</span>
            <input className="text-input" placeholder="请输入页面步骤名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </label>
          <label className="form-field form-field-inline">
            <span>状态</span>
            <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              <option value="active">通过</option>
              <option value="disabled">失败</option>
            </select>
          </label>
          <label className="form-field field-span-2">
            <span>预估步骤顺序</span>
            <textarea className="text-area" rows="3" placeholder="例如：-> 设置 -> 点击 -> 结果" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
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

function StepWorkbench({ step, onBack }) {
  const [nodes, setNodes] = useState([]);
  const [nodeSeq, setNodeSeq] = useState(1);
  const [selectedNode, setSelectedNode] = useState(null);
  const [draggingItem, setDraggingItem] = useState(null);
  const lastDropAt = useRef(0);
  const selected = selectedNode;
  const palette = [
    ["元素操作", "#10b981"],
    ["断言操作", "#2548b8"],
    ["SQL操作", "#d97706"],
    ["自定义变量", "#2548b8"],
    ["条件判断", "#64748b"],
    ["python代码", "#60a5fa"]
  ];

  const createNode = (item, event) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const nextNode = {
      id: nodeSeq,
      type: item.label,
      color: item.color,
      title: item.label,
      locator: "拖入后配置节点定位",
      x: Math.max(24, event.clientX - rect.left + event.currentTarget.scrollLeft - 48),
      y: Math.max(24, event.clientY - rect.top + event.currentTarget.scrollTop - 24)
    };

    setNodeSeq((value) => value + 1);
    setNodes((current) => [...current, nextNode]);
    setSelectedNode(nextNode);
  };

  const handleDragStart = (event, item) => {
    setDraggingItem(item);
    event.dataTransfer.setData("application/json", JSON.stringify(item));
    event.dataTransfer.effectAllowed = "copy";
  };

  const handleDrop = (event) => {
    event.preventDefault();
    const raw = event.dataTransfer.getData("application/json");
    if (!raw) return;
    lastDropAt.current = Date.now();
    createNode(JSON.parse(raw), event);
    setDraggingItem(null);
  };

  const handleCanvasMouseUp = (event) => {
    if (!draggingItem || Date.now() - lastDropAt.current < 100) return;
    createNode(draggingItem, event);
    setDraggingItem(null);
  };

  return (
    <section className="step-workbench">
      <div className="step-workbench-header">
        <div>
          <h2>页面步骤工作台 / {step.id} / {step.name || "-"}</h2>
          <p>编排页面步骤、维护节点配置，并查看最近一次调试结果</p>
        </div>
        <div className="action-row">
          <button className="icon-text-button compact-button" type="button">
            美化画布
          </button>
          <button className="primary-button compact-button" type="button">
            保存画布
          </button>
          <button className="success-button compact-button" type="button">
            调试
          </button>
          <button className="icon-text-button compact-button" onClick={onBack} type="button">
            返回
          </button>
        </div>
      </div>

      <div className="step-workbench-grid">
        <aside className="step-palette">
          <strong>操作面板</strong>
          <p>拖入节点类型</p>
          <div className="palette-list">
            {palette.map(([label, color]) => (
              <button
                className="palette-item"
                draggable
                key={label}
                onDragEnd={() => setDraggingItem(null)}
                onDragStart={(event) => handleDragStart(event, { label, color })}
                onMouseDown={() => setDraggingItem({ label, color })}
                type="button"
              >
                <span style={{ background: color }} />
                {label}
              </button>
            ))}
          </div>
        </aside>

        <main className="flow-panel">
          <div className="flow-panel-header">
            <div>
              <strong>流程画布</strong>
              <p>从左侧拖入节点，连接执行顺序后保存画布</p>
            </div>
            <div className="flow-stats">
              <span>节点 {nodes.length}</span>
              <span>连线 {Math.max(0, nodes.length - 1)}</span>
              <span>步骤 {nodes.length}</span>
            </div>
          </div>
          <div className={nodes.length ? "flow-canvas" : "flow-canvas empty"} onDragOver={(event) => event.preventDefault()} onDrop={handleDrop} onMouseUp={handleCanvasMouseUp}>
            {nodes.length ? (
              <>
                <div className="free-node-layer">
                  {nodes.map((node) => (
                    <button
                      className={selected?.id === node.id ? "flow-node active" : "flow-node"}
                      key={node.id}
                      onClick={() => setSelectedNode(node)}
                      style={{ borderLeftColor: node.color, left: node.x, top: node.y }}
                      type="button"
                    >
                      <span>{node.type}</span>
                      <strong>{node.title}</strong>
                      <small>{node.locator}</small>
                      <i />
                    </button>
                  ))}
                </div>
                <div className="minimap" aria-hidden="true">
                  {nodes.map((node) => (
                    <span key={node.id} />
                  ))}
                </div>
              </>
            ) : (
              <div className="empty-canvas">
                <strong>空白画布</strong>
                <p>从左侧拖拽组件到这里开始编排页面步骤</p>
              </div>
            )}
          </div>
        </main>

        <aside className="node-detail">
          <strong>节点详情</strong>
          <p>选择画布节点后维护配置</p>
          <div className="detail-tabs">
            <button className="active" type="button">
              节点配置
            </button>
            <button type="button">调试结果</button>
          </div>
          {selected ? (
            <div className="node-config">
              <label className="form-field">
                <span>节点名称</span>
                <input className="text-input" readOnly value={selected.title} />
              </label>
              <label className="form-field">
                <span>节点类型</span>
                <input className="text-input" readOnly value={selected.type} />
              </label>
              <label className="form-field">
                <span>定位表达式</span>
                <textarea className="text-area" readOnly rows="4" value={selected.locator} />
              </label>
            </div>
          ) : (
            <div className="empty-detail">
              <strong>暂未选择节点</strong>
              <p>在左侧画布中点击一个节点后，这里会显示元素、操作和断言配置。</p>
            </div>
          )}
        </aside>
      </div>
    </section>
  );
}

function PageElementsWorkspace() {
  const [form, setForm] = useState(initialFilters);
  const [filters, setFilters] = useState(initialFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [selectedIds, setSelectedIds] = useState([]);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [selectedPage, setSelectedPage] = useState(null);
  const [pageModal, setPageModal] = useState(null);

  const { data, loading, error, reload } = useAsyncData(
    () => uiAutomationService.elements.list({ ...filters, page, pageSize }),
    [filters.id, filters.pageName, filters.pageURL, filters.product, filters.module, page, pageSize]
  );

  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const allSelected = rows.length > 0 && rows.every((row) => selectedIds.includes(row.id));

  const productOptions = useMemo(() => uniqueOptions(rows, "category"), [rows]);
  const moduleOptions = useMemo(() => uniqueOptions(rows, "method"), [rows]);

  async function handleSearch(event) {
    event.preventDefault();
    setSelectedIds([]);
    setPage(1);
    setFilters({ ...form });
  }

  function handleReset() {
    setForm(initialFilters);
    setFilters(initialFilters);
    setPage(1);
    setPageSize(20);
    setSelectedIds([]);
    setSelectedPage(null);
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

  async function handleBulkDelete() {
    if (!selectedIds.length) {
      setNotice("请先选择需要删除的页面对象。");
      return;
    }
    if (!window.confirm(`确认删除已选中的 ${selectedIds.length} 个页面对象吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await Promise.all(selectedIds.map((id) => uiAutomationService.elements.remove(id)));
      setSelectedIds([]);
      if (selectedPage && selectedIds.includes(selectedPage.id)) {
        setSelectedPage(null);
      }
      await reload();
      setNotice("已删除选中的页面对象。");
    } catch (err) {
      setNotice(err.message || "批量删除失败");
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteRow(row) {
    if (!window.confirm(`确认删除页面“${row.name}”吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await uiAutomationService.elements.remove(row.id);
      setSelectedIds((current) => current.filter((id) => id !== row.id));
      if (selectedPage?.id === row.id) {
        setSelectedPage(null);
      }
      await reload();
      setNotice("页面对象已删除。");
    } catch (err) {
      setNotice(err.message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  async function handleCopyRow(row) {
    setBusy(true);
    setNotice("");
    try {
      await uiAutomationService.elements.create({
        name: `${row.name || "页面"}_copy`,
        category: row.category || "",
        method: row.method || "",
        locator: row.locator || "",
        value: row.value || "WEB",
        description: row.description || "",
        status: row.status || "active"
      });
      await reload();
      setNotice("页面对象已复制。");
    } catch (err) {
      setNotice(err.message || "复制失败");
    } finally {
      setBusy(false);
    }
  }

  if (selectedPage) {
    return <PageElementPanel key={selectedPage.id} pageRow={selectedPage} onBack={() => setSelectedPage(null)} />;
  }

  return (
    <div className="section-stack">
      <PageHeader title="页面元素" description="还原页面对象列表，并支持继续维护页面下的元素资产。" />

      <section className="resource-panel">
        <div className="panel-header">
          <strong>UI元素页面对象</strong>
        </div>

        <form className="filter-grid" onSubmit={handleSearch}>
          <label className="form-field">
            <span>ID</span>
            <input className="text-input" value={form.id} placeholder="请输入页面ID" onChange={(event) => setForm({ ...form, id: event.target.value })} />
          </label>
          <label className="form-field">
            <span>页面名称</span>
            <input className="text-input" value={form.pageName} placeholder="请输入页面名称" onChange={(event) => setForm({ ...form, pageName: event.target.value })} />
          </label>
          <label className="form-field">
            <span>页面地址</span>
            <input className="text-input" value={form.pageURL} placeholder="请输入页面地址" onChange={(event) => setForm({ ...form, pageURL: event.target.value })} />
          </label>
          <label className="form-field">
            <span>项目/产品</span>
            <select className="text-input" value={form.product} onChange={(event) => setForm({ ...form, product: event.target.value })}>
              <option value="">请选择产品</option>
              {productOptions.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>模块名称</span>
            <select className="text-input" value={form.module} onChange={(event) => setForm({ ...form, module: event.target.value })}>
              <option value="">请选择模块</option>
              {moduleOptions.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
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
            <button className="primary-button compact-button" onClick={() => setPageModal({ mode: "create", row: null })} type="button">
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
                    <th>项目/产品</th>
                    <th>模块名称</th>
                    <th>页面名称</th>
                    <th>页面地址</th>
                    <th>端类型</th>
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
                        <td>{row.category || "-"}</td>
                        <td>{row.method || "-"}</td>
                        <td>{row.name || "-"}</td>
                        <td className="cell-ellipsis" title={row.locator || ""}>
                          {row.locator || "-"}
                        </td>
                        <td>
                          <span className="table-badge">{row.value || "WEB"}</span>
                        </td>
                        <td>
                          <div className="action-links">
                            <button className="link-button" onClick={() => setPageModal({ mode: "edit", row })} type="button">
                              编辑
                            </button>
                            <button className="link-button" onClick={() => setSelectedPage(row)} type="button">
                              添加元素
                            </button>
                            <details className="more-menu">
                              <summary>更多</summary>
                              <div className="more-menu-panel">
                                <button className="link-button" disabled={busy} onClick={() => handleCopyRow(row)} type="button">
                                  复制
                                </button>
                                <button className="link-button danger-link" disabled={busy} onClick={() => handleDeleteRow(row)} type="button">
                                  删除
                                </button>
                              </div>
                            </details>
                          </div>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan="8">暂无页面对象数据</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            <PaginationBar page={page} pageSize={pageSize} total={total} totalPages={totalPages} onPageChange={setPage} onPageSizeChange={(value) => {
              setPage(1);
              setPageSize(value);
            }} />
          </TablePanel>
        </StateBlock>
      </section>

      {pageModal ? (
        <PageObjectModal
          busy={busy}
          modal={pageModal}
          pageRows={rows}
          onClose={() => setPageModal(null)}
          onSubmit={async (payload, mode) => {
            setBusy(true);
            setNotice("");
            try {
              if (mode === "edit") {
                await uiAutomationService.elements.update(pageModal.row.id, payload);
                setNotice("页面对象已更新。");
              } else {
                await uiAutomationService.elements.create(payload);
                setNotice("页面对象已创建。");
              }
              setPageModal(null);
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

function PageElementPanel({ pageRow, onBack }) {
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [elementPage, setElementPage] = useState(1);
  const [elementPageSize, setElementPageSize] = useState(20);
  const [selectedIds, setSelectedIds] = useState([]);
  const [modal, setModal] = useState(null);

  const { data, loading, error, reload } = useAsyncData(
    () => uiAutomationService.pageElements.list({ pageId: pageRow.id, page: elementPage, pageSize: elementPageSize }),
    [pageRow.id, elementPage, elementPageSize]
  );

  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / elementPageSize));
  const allSelected = rows.length > 0 && rows.every((row) => selectedIds.includes(row.id));

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

  async function handleBulkDelete() {
    if (!selectedIds.length) {
      setNotice("请先选择需要删除的元素。");
      return;
    }
    if (!window.confirm(`确认删除已选中的 ${selectedIds.length} 个元素吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await Promise.all(selectedIds.map((id) => uiAutomationService.pageElements.remove(id)));
      setSelectedIds([]);
      await reload();
      setNotice("已删除选中的页面元素。");
    } catch (err) {
      setNotice(err.message || "批量删除失败");
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete(row) {
    if (!window.confirm(`确认删除元素“${row.name}”吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await uiAutomationService.pageElements.remove(row.id);
      await reload();
      setNotice("页面元素已删除。");
    } catch (err) {
      setNotice(err.message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="resource-panel page-element-config">
      <div className="page-config-header">
        <div>
          <h2>页面元素配置 / {pageRow.id} / {pageRow.name || "-"}</h2>
          <p className="panel-subtitle">维护页面元素定位、操作配置和批量导入数据</p>
        </div>
        <button className="icon-text-button compact-button" onClick={onBack} type="button">
          返回
        </button>
      </div>

      <div className="page-config-toolbar">
        <span />
        <div className="action-row">
          <button className="primary-button compact-button" type="button">
            下载模板
          </button>
          <button className="primary-button compact-button" type="button">
            点击上传
          </button>
          <button className="primary-button compact-button" onClick={() => setModal({ mode: "create", row: null })} type="button">
            单个新增
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
                  <th>元素名称</th>
                  <th>等待时间(秒)</th>
                  <th>类型-1</th>
                  <th>定位-1</th>
                  <th>下标-1</th>
                  <th>类型-2</th>
                  <th>定位-2</th>
                  <th>下标-2</th>
                  <th>类型-3</th>
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
                      <td>{row.name || "-"}</td>
                      <td>{row.waitTime || "-"}</td>
                      <td>{row.type1 || "-"}</td>
                      <td className="cell-ellipsis" title={row.locator1 || ""}>
                        {row.locator1 || "-"}
                      </td>
                      <td>{row.index1 || "-"}</td>
                      <td>{row.type2 || "-"}</td>
                      <td className="cell-ellipsis" title={row.locator2 || ""}>
                        {row.locator2 || "-"}
                      </td>
                      <td>{row.index2 || "-"}</td>
                      <td>{row.type3 || "-"}</td>
                      <td>
                        <div className="action-links">
                          <button className="link-button" type="button">
                            调试
                          </button>
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
                    <td colSpan="12">暂无数据</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <PaginationBar
            page={elementPage}
            pageSize={elementPageSize}
            total={total}
            totalPages={totalPages}
            onPageChange={setElementPage}
            onPageSizeChange={(value) => {
              setElementPage(1);
              setElementPageSize(value);
            }}
          />
        </TablePanel>
      </StateBlock>

      {modal ? (
        <PageElementModal
          busy={busy}
          modal={modal}
          pageRow={pageRow}
          onClose={() => setModal(null)}
          onSubmit={async (payload, mode) => {
            setBusy(true);
            setNotice("");
            try {
              if (mode === "edit") {
                await uiAutomationService.pageElements.update(modal.row.id, payload);
                setNotice("页面元素已更新。");
              } else {
                await uiAutomationService.pageElements.create(payload);
                setNotice("页面元素已创建。");
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
    </section>
  );
}

function PageObjectModal({ busy, modal, pageRows, onClose, onSubmit }) {
  const source = modal.row;
  const [form, setForm] = useState(
    source
      ? {
          name: source.name || "",
          category: source.category || "",
          method: source.method || "",
          locator: source.locator || "",
          value: source.value || "WEB",
          description: source.description || "",
          status: source.status || "active"
        }
      : emptyPageForm
  );
  const [error, setError] = useState("");
  const { data: productsData, loading: loadingProducts } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const productOptions = useMemo(() => {
    const items = pageItems(productsData);
    const options = items.map((item) => ({
      label: `${item.projectName}/${item.name}`,
      value: `${item.projectName}/${item.name}`,
      uiType: item.uiType || "WEB"
    }));
    if (form.category && !options.some((item) => item.value === form.category)) {
      return [{ label: form.category, value: form.category, uiType: form.value || "WEB" }, ...options];
    }
    return options;
  }, [form.category, form.value, productsData]);
  const moduleOptions = useMemo(() => {
    const options = uniqueOptions(
      (pageRows || []).filter((row) => !form.category || row.category === form.category),
      "method"
    );
    if (form.method && !options.includes(form.method)) {
      return [form.method, ...options];
    }
    return options;
  }, [form.category, form.method, pageRows]);

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    if (!form.name.trim() || !form.category.trim() || !form.method.trim() || !form.locator.trim()) {
      setError("项目/产品、模块名称、页面名称和页面地址不能为空。");
      return;
    }
    await onSubmit(
      {
        ...form,
        name: form.name.trim(),
        category: form.category.trim(),
        method: form.method.trim(),
        locator: form.locator.trim(),
        value: form.value.trim() || "WEB",
        description: form.description.trim(),
        status: form.status
      },
      modal.mode
    );
  }

  return (
    <div className="modal-backdrop">
      <form className="modal-card" onSubmit={handleSubmit}>
        <div className="modal-header">
          <strong>{modal.mode === "edit" ? "编辑" : "新增"}</strong>
          <button className="modal-close" onClick={onClose} type="button">
            ×
          </button>
        </div>
        <div className="modal-form">
          <label className="form-field form-field-inline required-field">
            <span>项目/产品</span>
            <select
              className="text-input"
              disabled={loadingProducts}
              value={form.category}
              onChange={(event) => {
                const selected = productOptions.find((item) => item.value === event.target.value);
                setForm({
                  ...form,
                  category: event.target.value,
                  method: "",
                  value: selected?.uiType || form.value
                });
              }}
            >
              <option value="">{loadingProducts ? "加载项目中" : "请选择项目名称"}</option>
              {productOptions.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field form-field-inline required-field">
            <span>模块名称</span>
            <select className="text-input" value={form.method} onChange={(event) => setForm({ ...form, method: event.target.value })}>
              <option value="">{form.category ? "请选择测试模块" : "请先选择项目名称"}</option>
              {moduleOptions.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field form-field-inline required-field">
            <span>页面名称</span>
            <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </label>
          <label className="form-field form-field-inline required-field">
            <span>页面地址</span>
            <input className="text-input" placeholder="请输入页面地址" value={form.locator} onChange={(event) => setForm({ ...form, locator: event.target.value })} />
          </label>
          <label className="form-field form-field-inline">
            <span>端类型</span>
            <select className="text-input" value={form.value} onChange={(event) => setForm({ ...form, value: event.target.value })}>
              <option value="WEB">WEB</option>
              <option value="APP">APP</option>
              <option value="H5">H5</option>
            </select>
          </label>
          <label className="form-field form-field-inline">
            <span>状态</span>
            <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              <option value="active">启用</option>
              <option value="disabled">停用</option>
            </select>
          </label>
          <label className="form-field field-span-2">
            <span>说明</span>
            <textarea className="text-area" rows="3" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
          </label>
        </div>
        {error ? <div className="form-error">{error}</div> : null}
        <div className="modal-actions">
          <button className="icon-text-button compact-button" onClick={onClose} type="button">
            取消
          </button>
          <button className="primary-button compact-button" disabled={busy} type="submit">
            {busy ? "保存中" : "保存"}
          </button>
        </div>
      </form>
    </div>
  );
}

function PageElementModal({ busy, modal, pageRow, onClose, onSubmit }) {
  const source = modal.row;
  const [form, setForm] = useState(
    source
      ? {
          pageId: pageRow.id,
          name: source.name || "",
          type1: source.type1 || "xpath",
          locator1: source.locator1 || "",
          index1: source.index1 || "",
          type2: source.type2 || "",
          locator2: source.locator2 || "",
          index2: source.index2 || "",
          type3: source.type3 || "",
          locator3: source.locator3 || "",
          index3: source.index3 || "",
          aiPrompt: source.aiPrompt || "",
          waitTime: source.waitTime || ""
        }
      : { ...emptyElementForm, pageId: pageRow.id }
  );
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    if (!form.name.trim() || !form.type1.trim() || !form.locator1.trim()) {
      setError("元素名称、类型-1 和定位-1 不能为空。");
      return;
    }
    await onSubmit(
      {
        ...form,
        pageId: pageRow.id,
        name: form.name.trim(),
        type1: form.type1.trim(),
        locator1: form.locator1.trim(),
        index1: form.index1.trim(),
        type2: form.type2.trim(),
        locator2: form.locator2.trim(),
        index2: form.index2.trim(),
        type3: form.type3.trim(),
        locator3: form.locator3.trim(),
        index3: form.index3.trim(),
        aiPrompt: form.aiPrompt.trim(),
        waitTime: form.waitTime.trim()
      },
      modal.mode
    );
  }

  return (
    <div className="modal-backdrop">
      <form className="modal-card modal-card-element" onSubmit={handleSubmit}>
        <div className="modal-header">
          <strong>{modal.mode === "edit" ? "编辑页面元素" : "新增页面元素"}</strong>
          <button className="modal-close" onClick={onClose} type="button">
            ×
          </button>
        </div>
        <div className="modal-grid element-modal-form">
          <label className="form-field required-field">
            <span>元素名称</span>
            <input className="text-input" placeholder="请输入元素名称" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </label>
          <label className="form-field">
            <span>等待时间(秒)</span>
            <input className="text-input" placeholder="请输入等待时间" value={form.waitTime} onChange={(event) => setForm({ ...form, waitTime: event.target.value })} />
          </label>
          <label className="form-field required-field">
            <span>类型-1</span>
            <select className="text-input" value={form.type1} onChange={(event) => setForm({ ...form, type1: event.target.value })}>
              <option value="">请选择元素表达式类型</option>
              <option value="xpath">xpath</option>
              <option value="css">css</option>
              <option value="id">id</option>
              <option value="name">name</option>
              <option value="text">text</option>
            </select>
          </label>
          <label className="form-field required-field">
            <span>定位-1</span>
            <input className="text-input" placeholder="请输入元素表达式" value={form.locator1} onChange={(event) => setForm({ ...form, locator1: event.target.value })} />
          </label>
          <label className="form-field">
            <span>索引-1</span>
            <input className="text-input" placeholder="请输入元素下标，从1开始数" value={form.index1} onChange={(event) => setForm({ ...form, index1: event.target.value })} />
          </label>
          <label className="form-field">
            <span>类型-2</span>
            <select className="text-input" value={form.type2} onChange={(event) => setForm({ ...form, type2: event.target.value })}>
              <option value="">请选择类型-2的元素表达式类型</option>
              <option value="xpath">xpath</option>
              <option value="css">css</option>
              <option value="id">id</option>
              <option value="name">name</option>
              <option value="text">text</option>
            </select>
          </label>
          <label className="form-field">
            <span>定位-2</span>
            <input className="text-input" placeholder="请输入定位-2的元素表达式" value={form.locator2} onChange={(event) => setForm({ ...form, locator2: event.target.value })} />
          </label>
          <label className="form-field">
            <span>索引-2</span>
            <input className="text-input" placeholder="请输入元素下标，从1开始数" value={form.index2} onChange={(event) => setForm({ ...form, index2: event.target.value })} />
          </label>
          <label className="form-field">
            <span>类型-3</span>
            <select className="text-input" value={form.type3} onChange={(event) => setForm({ ...form, type3: event.target.value })}>
              <option value="">请选择类型-3的元素表达式类型</option>
              <option value="xpath">xpath</option>
              <option value="css">css</option>
              <option value="id">id</option>
              <option value="name">name</option>
              <option value="text">text</option>
            </select>
          </label>
          <label className="form-field">
            <span>定位-3</span>
            <input className="text-input" placeholder="请输入定位-3的元素表达式" value={form.locator3} onChange={(event) => setForm({ ...form, locator3: event.target.value })} />
          </label>
          <label className="form-field">
            <span>索引-3</span>
            <input className="text-input" placeholder="请输入元素下标，从1开始数" value={form.index3} onChange={(event) => setForm({ ...form, index3: event.target.value })} />
          </label>
          <label className="form-field field-span-2">
            <span>AI 提示词</span>
            <textarea className="text-area" rows="3" placeholder="请输入AI辅助定位，用于查找元素的提示词" value={form.aiPrompt} onChange={(event) => setForm({ ...form, aiPrompt: event.target.value })} />
          </label>
        </div>
        {error ? <div className="form-error">{error}</div> : null}
        <div className="modal-actions">
          <button className="icon-text-button compact-button" onClick={onClose} type="button">
            取消
          </button>
          <button className="primary-button compact-button" disabled={busy} type="submit">
            {busy ? "保存中" : "保存"}
          </button>
        </div>
      </form>
    </div>
  );
}

function uniqueOptions(rows, key) {
  return [...new Set(rows.map((row) => row[key]).filter(Boolean))];
}

function ensureOptions(options, current) {
  if (!current || options.includes(current)) {
    return options;
  }
  return [current, ...options];
}
