import { useState } from "react";
import { PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { configService } from "../services/configService.js";
import { uiAutomationService } from "../services/uiAutomationService.js";
import { formatTime, pageItems } from "../utils/formatters.js";

const initialCaseFilters = {
  id: "",
  name: "",
  productId: "",
  moduleId: "",
  pageId: "",
  priority: "",
  status: ""
};

const emptyCaseForm = {
  productId: "",
  moduleId: "",
  pageId: "",
  name: "",
  caseType: "ui",
  priority: "P2",
  status: "draft",
  owner: "",
  tags: "",
  description: "",
  preconditions: "",
  expectedResult: "",
  dataEnabled: false,
  stepIds: []
};
export function TestCasesPage() {
  const [form, setForm] = useState(initialCaseFilters);
  const [filters, setFilters] = useState(initialCaseFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [selectedIds, setSelectedIds] = useState([]);
  const [modal, setModal] = useState(null);
  const [detail, setDetail] = useState(null);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const { data, loading, error, reload } = useAsyncData(
    () => uiAutomationService.cases.list({ ...filters, page, pageSize }),
    [filters.id, filters.name, filters.productId, filters.moduleId, filters.pageId, filters.priority, filters.status, page, pageSize]
  );
  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const allSelected = rows.length > 0 && rows.every((row) => selectedIds.includes(row.id));

  const { data: productsData, loading: loadingProducts } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const productOptions = pageItems(productsData);
  const { data: modulesData, loading: loadingModules } = useAsyncData(
    () => (form.productId ? configService.productModules.list({ productId: form.productId, page: 1, pageSize: 200 }) : Promise.resolve({ items: [] })),
    [form.productId]
  );
  const moduleOptions = pageItems(modulesData);
  const selectedProduct = productOptions.find((item) => String(item.id) === String(form.productId));
  const selectedModule = moduleOptions.find((item) => String(item.id) === String(form.moduleId));
  const { data: pagesData, loading: loadingPages } = useAsyncData(
    () =>
      selectedProduct
        ? uiAutomationService.elements.list({
            product: `${selectedProduct.projectName}/${selectedProduct.name}`,
            module: selectedModule?.name || "",
            page: 1,
            pageSize: 200
          })
        : Promise.resolve({ items: [] }),
    [selectedProduct?.id, selectedModule?.id]
  );
  const pageOptions = pageItems(pagesData);

  function submitSearch(event) {
    event.preventDefault();
    setFilters({ ...form });
    setSelectedIds([]);
    setPage(1);
  }

  function resetSearch() {
    setForm(initialCaseFilters);
    setFilters(initialCaseFilters);
    setSelectedIds([]);
    setPage(1);
  }

  function toggleSelectAll() {
    setSelectedIds(allSelected ? [] : rows.map((row) => row.id));
  }

  function toggleSelectOne(id) {
    setSelectedIds((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  }

  async function deleteRows(ids) {
    if (!ids.length) {
      setNotice("请先选择需要删除的测试用例。");
      return;
    }
    if (!window.confirm(`确认删除 ${ids.length} 条测试用例吗？`)) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await Promise.all(ids.map((id) => uiAutomationService.cases.remove(id)));
      setSelectedIds([]);
      setDetail(null);
      await reload();
      setNotice("测试用例已删除。");
    } catch (err) {
      setNotice(err.message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  async function openDetail(row) {
    setBusy(true);
    setNotice("");
    try {
      const result = await uiAutomationService.cases.get(row.id);
      setDetail(result);
    } catch (err) {
      setNotice(err.message || "读取详情失败");
    } finally {
      setBusy(false);
    }
  }

  async function exportCases() {
    setBusy(true);
    setNotice("");
    try {
      const result = await uiAutomationService.cases.export(filters);
      const blob = new Blob([JSON.stringify(result.items || [], null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = "test-cases.json";
      link.click();
      URL.revokeObjectURL(url);
      setNotice("测试用例已导出。");
    } catch (err) {
      setNotice(err.message || "导出失败");
    } finally {
      setBusy(false);
    }
  }

  async function importCases(event) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) {
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      const text = await file.text();
      const parsed = JSON.parse(text);
      const items = Array.isArray(parsed) ? parsed : parsed.items;
      if (!Array.isArray(items)) {
        throw new Error("导入文件必须是数组或包含 items 数组");
      }
      const result = await uiAutomationService.cases.import(items);
      await reload();
      setNotice(`已导入 ${result.count || items.length} 条测试用例。`);
    } catch (err) {
      setNotice(err.message || "导入失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="section-stack">
      <PageHeader title="测试用例" description="管理界面自动化测试用例、关联步骤和参数化数据" />
      <section className="resource-panel">
        <div className="panel-header">
          <strong>测试用例管理</strong>
          <div className="action-row">
            <label className="icon-text-button compact-button file-action">
              导入
              <input accept="application/json" onChange={importCases} type="file" />
            </label>
            <button className="icon-text-button compact-button" disabled={busy} onClick={exportCases} type="button">
              导出
            </button>
            <button className="primary-button compact-button" onClick={() => setModal({ mode: "create", row: null })} type="button">
              新增
            </button>
            <button className="danger-button compact-button" disabled={busy} onClick={() => deleteRows(selectedIds)} type="button">
              批量删除
            </button>
          </div>
        </div>

        <form className="filter-grid filter-grid-cases" onSubmit={submitSearch}>
          <label className="form-field">
            <span>ID</span>
            <input className="text-input" value={form.id} onChange={(event) => setForm({ ...form, id: event.target.value })} placeholder="请输入用例ID" />
          </label>
          <label className="form-field">
            <span>用例名称</span>
            <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="请输入用例名称" />
          </label>
          <label className="form-field">
            <span>项目/产品</span>
            <select className="text-input" disabled={loadingProducts} value={form.productId} onChange={(event) => setForm({ ...form, productId: event.target.value, moduleId: "", pageId: "" })}>
              <option value="">{loadingProducts ? "加载产品中" : "请选择产品"}</option>
              {productOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.projectName}/{item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>模块名称</span>
            <select className="text-input" disabled={!form.productId || loadingModules} value={form.moduleId} onChange={(event) => setForm({ ...form, moduleId: event.target.value, pageId: "" })}>
              <option value="">{form.productId ? "请选择模块" : "请先选择产品"}</option>
              {moduleOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>所属页面</span>
            <select className="text-input" disabled={!form.productId || loadingPages} value={form.pageId} onChange={(event) => setForm({ ...form, pageId: event.target.value })}>
              <option value="">{form.productId ? "请选择页面" : "请先选择产品"}</option>
              {pageOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>优先级</span>
            <select className="text-input" value={form.priority} onChange={(event) => setForm({ ...form, priority: event.target.value })}>
              <option value="">全部</option>
              <option value="P0">P0</option>
              <option value="P1">P1</option>
              <option value="P2">P2</option>
              <option value="P3">P3</option>
            </select>
          </label>
          <label className="form-field">
            <span>状态</span>
            <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              <option value="">全部</option>
              <option value="draft">草稿</option>
              <option value="active">启用</option>
              <option value="disabled">停用</option>
            </select>
          </label>
          <div className="filter-actions">
            <button className="primary-button compact-button" type="submit">
              搜索
            </button>
            <button className="icon-text-button compact-button" onClick={resetSearch} type="button">
              重置
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
                    <th className="checkbox-cell">
                      <input checked={allSelected} onChange={toggleSelectAll} type="checkbox" />
                    </th>
                    <th>ID</th>
                    <th>项目/产品</th>
                    <th>模块</th>
                    <th>所属页面</th>
                    <th>用例名称</th>
                    <th>优先级</th>
                    <th>状态</th>
                    <th>负责人</th>
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
                        <td>{row.productName || "-"}</td>
                        <td>{row.moduleName || "-"}</td>
                        <td>{row.pageName || "-"}</td>
                        <td>{row.name}</td>
                        <td>
                          <span className="status-pill neutral">{row.priority}</span>
                        </td>
                        <td>
                          <span className={`status-pill ${row.status === "active" ? "success" : row.status === "disabled" ? "danger" : "warning"}`}>{caseStatusLabel(row.status)}</span>
                        </td>
                        <td>{row.owner || "-"}</td>
                        <td>{formatTime(row.updatedAt)}</td>
                        <td>
                          <div className="action-links">
                            <button className="link-button" onClick={() => openDetail(row)} type="button">
                              详情
                            </button>
                            <button className="link-button" onClick={() => setModal({ mode: "edit", row })} type="button">
                              编辑
                            </button>
                            <button className="link-button danger-link" disabled={busy} onClick={() => deleteRows([row.id])} type="button">
                              删除
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan="11">暂无数据</td>
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

      {detail ? <TestCaseDetailPanel detail={detail} onClose={() => setDetail(null)} onRefresh={() => openDetail(detail)} /> : null}
      {modal ? (
        <TestCaseModal
          busy={busy}
          modal={modal}
          onClose={() => setModal(null)}
          onSubmit={async (payload, mode) => {
            setBusy(true);
            setNotice("");
            try {
              if (mode === "edit") {
                await uiAutomationService.cases.update(modal.row.id, payload);
                setNotice("测试用例已更新。");
              } else {
                await uiAutomationService.cases.create(payload);
                setNotice("测试用例已创建。");
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

function TestCaseModal({ busy, modal, onClose, onSubmit }) {
  const source = modal.row;
  const [form, setForm] = useState(
    source
      ? {
          productId: String(source.productId || ""),
          moduleId: source.moduleId ? String(source.moduleId) : "",
          pageId: source.pageId ? String(source.pageId) : "",
          name: source.name || "",
          caseType: source.caseType || "ui",
          priority: source.priority || "P2",
          status: source.status || "draft",
          owner: source.owner || "",
          tags: source.tags || "",
          description: source.description || "",
          preconditions: source.preconditions || "",
          expectedResult: source.expectedResult || "",
          dataEnabled: Boolean(source.dataEnabled),
          stepIds: source.steps?.map((item) => item.stepId) || []
        }
      : emptyCaseForm
  );
  const [error, setError] = useState("");
  const { data: productsData, loading: loadingProducts } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const productOptions = pageItems(productsData);
  const selectedProduct = productOptions.find((item) => String(item.id) === String(form.productId));
  const { data: modulesData, loading: loadingModules } = useAsyncData(
    () => (form.productId ? configService.productModules.list({ productId: form.productId, page: 1, pageSize: 200 }) : Promise.resolve({ items: [] })),
    [form.productId]
  );
  const moduleOptions = pageItems(modulesData);
  const selectedModule = moduleOptions.find((item) => String(item.id) === String(form.moduleId));
  const { data: pagesData, loading: loadingPages } = useAsyncData(
    () =>
      selectedProduct
        ? uiAutomationService.elements.list({
            product: `${selectedProduct.projectName}/${selectedProduct.name}`,
            module: selectedModule?.name || "",
            page: 1,
            pageSize: 200
          })
        : Promise.resolve({ items: [] }),
    [selectedProduct?.id, selectedModule?.id]
  );
  const pageOptions = pageItems(pagesData);
  const selectedPage = pageOptions.find((item) => String(item.id) === String(form.pageId));
  const { data: stepsData, loading: loadingSteps } = useAsyncData(
    () =>
      selectedProduct
        ? uiAutomationService.steps.list({
            product: `${selectedProduct.projectName}/${selectedProduct.name}`,
            module: selectedModule?.name || "",
            pageUrl: selectedPage?.name || "",
            page: 1,
            pageSize: 200
          })
        : Promise.resolve({ items: [] }),
    [selectedProduct?.id, selectedModule?.id, selectedPage?.id]
  );
  const stepOptions = pageItems(stepsData);

  function toggleStep(id) {
    setForm((current) => ({
      ...current,
      stepIds: current.stepIds.includes(id) ? current.stepIds.filter((item) => item !== id) : [...current.stepIds, id]
    }));
  }

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    if (!form.productId || !form.name.trim()) {
      setError("项目/产品和用例名称不能为空。");
      return;
    }
    await onSubmit(
      {
        ...form,
        productId: Number(form.productId),
        moduleId: Number(form.moduleId || 0),
        pageId: Number(form.pageId || 0),
        name: form.name.trim(),
        owner: form.owner.trim(),
        tags: form.tags.trim(),
        description: form.description.trim(),
        preconditions: form.preconditions.trim(),
        expectedResult: form.expectedResult.trim(),
        stepIds: form.stepIds.map(Number)
      },
      modal.mode
    );
  }

  return (
    <div className="modal-backdrop">
      <form className="modal-card modal-card-wide" onSubmit={handleSubmit}>
        <div className="modal-header">
          <strong>{modal.mode === "edit" ? "编辑测试用例" : "新增测试用例"}</strong>
          <button className="modal-close" onClick={onClose} type="button">
            ×
          </button>
        </div>
        <div className="modal-grid case-modal-form">
          <label className="form-field required-field">
            <span>项目/产品</span>
            <select className="text-input" disabled={loadingProducts} value={form.productId} onChange={(event) => setForm({ ...form, productId: event.target.value, moduleId: "", pageId: "", stepIds: [] })}>
              <option value="">{loadingProducts ? "加载产品中" : "请选择产品"}</option>
              {productOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.projectName}/{item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>模块名称</span>
            <select className="text-input" disabled={!form.productId || loadingModules} value={form.moduleId} onChange={(event) => setForm({ ...form, moduleId: event.target.value, pageId: "", stepIds: [] })}>
              <option value="">{form.productId ? "请选择模块" : "请先选择产品"}</option>
              {moduleOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>所属页面</span>
            <select className="text-input" disabled={!form.productId || loadingPages} value={form.pageId} onChange={(event) => setForm({ ...form, pageId: event.target.value, stepIds: [] })}>
              <option value="">{form.productId ? "请选择页面" : "请先选择产品"}</option>
              {pageOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field required-field">
            <span>用例名称</span>
            <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="请输入用例名称" />
          </label>
          <label className="form-field">
            <span>用例类型</span>
            <select className="text-input" value={form.caseType} onChange={(event) => setForm({ ...form, caseType: event.target.value })}>
              <option value="ui">UI</option>
              <option value="api">API</option>
              <option value="unit">单元</option>
              <option value="mixed">混合</option>
            </select>
          </label>
          <label className="form-field">
            <span>优先级</span>
            <select className="text-input" value={form.priority} onChange={(event) => setForm({ ...form, priority: event.target.value })}>
              <option value="P0">P0</option>
              <option value="P1">P1</option>
              <option value="P2">P2</option>
              <option value="P3">P3</option>
            </select>
          </label>
          <label className="form-field">
            <span>状态</span>
            <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              <option value="draft">草稿</option>
              <option value="active">启用</option>
              <option value="disabled">停用</option>
            </select>
          </label>
          <label className="form-field">
            <span>负责人</span>
            <input className="text-input" value={form.owner} onChange={(event) => setForm({ ...form, owner: event.target.value })} placeholder="请输入负责人" />
          </label>
          <label className="form-field">
            <span>标签</span>
            <input className="text-input" value={form.tags} onChange={(event) => setForm({ ...form, tags: event.target.value })} placeholder="smoke,login" />
          </label>
          <label className="form-field checkbox-line">
            <input checked={form.dataEnabled} onChange={(event) => setForm({ ...form, dataEnabled: event.target.checked })} type="checkbox" />
            <span>启用参数化</span>
          </label>
          <label className="form-field field-span-2">
            <span>前置条件</span>
            <textarea className="text-area" rows="3" value={form.preconditions} onChange={(event) => setForm({ ...form, preconditions: event.target.value })} />
          </label>
          <label className="form-field field-span-2">
            <span>预期结果</span>
            <textarea className="text-area" rows="3" value={form.expectedResult} onChange={(event) => setForm({ ...form, expectedResult: event.target.value })} />
          </label>
          <label className="form-field field-span-2">
            <span>描述</span>
            <textarea className="text-area" rows="3" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
          </label>
          <div className="form-field field-span-2">
            <span>关联步骤</span>
            <div className="check-list">
              {loadingSteps ? <span className="muted-text">加载步骤中</span> : null}
              {!loadingSteps && !stepOptions.length ? <span className="muted-text">暂无可关联步骤</span> : null}
              {stepOptions.map((item) => (
                <label key={item.id} className="check-item">
                  <input checked={form.stepIds.includes(item.id)} onChange={() => toggleStep(item.id)} type="checkbox" />
                  <span>{item.name}</span>
                </label>
              ))}
            </div>
          </div>
        </div>
        {error ? <div className="form-error">{error}</div> : null}
        <div className="modal-actions">
          <button className="icon-text-button compact-button" onClick={onClose} type="button">
            取消
          </button>
          <button className="primary-button compact-button" disabled={busy} type="submit">
            {busy ? "保存中" : "提交"}
          </button>
        </div>
      </form>
    </div>
  );
}

function TestCaseDetailPanel({ detail, onClose, onRefresh }) {
  const [datasetForm, setDatasetForm] = useState({ name: "", variables: "{\n  \n}", enabled: true });
  const [notice, setNotice] = useState("");

  async function createDataset(event) {
    event.preventDefault();
    setNotice("");
    try {
      await uiAutomationService.cases.datasets.create(detail.id, {
        name: datasetForm.name,
        variables: JSON.parse(datasetForm.variables || "{}"),
        enabled: datasetForm.enabled
      });
      setDatasetForm({ name: "", variables: "{\n  \n}", enabled: true });
      await onRefresh();
    } catch (err) {
      setNotice(err.message || "保存参数化数据失败");
    }
  }

  async function removeDataset(row) {
    if (!window.confirm(`确认删除数据集“${row.name}”吗？`)) {
      return;
    }
    setNotice("");
    try {
      await uiAutomationService.cases.datasets.remove(detail.id, row.id);
      await onRefresh();
    } catch (err) {
      setNotice(err.message || "删除参数化数据失败");
    }
  }

  return (
    <section className="resource-panel detail-panel">
      <div className="panel-header">
        <strong>测试用例详情 / {detail.id} / {detail.name}</strong>
        <button className="icon-text-button compact-button" onClick={onClose} type="button">
          关闭
        </button>
      </div>
      <div className="detail-grid">
        <span>项目/产品：{detail.productName || "-"}</span>
        <span>模块：{detail.moduleName || "-"}</span>
        <span>页面：{detail.pageName || "-"}</span>
        <span>优先级：{detail.priority}</span>
        <span>状态：{caseStatusLabel(detail.status)}</span>
        <span>负责人：{detail.owner || "-"}</span>
      </div>
      <div className="detail-block">
        <strong>关联步骤</strong>
        {(detail.steps || []).length ? (
          <ol className="step-list">
            {detail.steps.map((item) => (
              <li key={item.id}>{item.stepName || item.stepId}</li>
            ))}
          </ol>
        ) : (
          <span className="muted-text">暂无关联步骤</span>
        )}
      </div>
      <div className="detail-block">
        <strong>参数化数据</strong>
        {notice ? <div className="inline-notice">{notice}</div> : null}
        <form className="dataset-form" onSubmit={createDataset}>
          <input className="text-input" value={datasetForm.name} onChange={(event) => setDatasetForm({ ...datasetForm, name: event.target.value })} placeholder="数据集名称" />
          <textarea className="text-area" rows="3" value={datasetForm.variables} onChange={(event) => setDatasetForm({ ...datasetForm, variables: event.target.value })} placeholder='{"username":"admin"}' />
          <label className="checkbox-line">
            <input checked={datasetForm.enabled} onChange={(event) => setDatasetForm({ ...datasetForm, enabled: event.target.checked })} type="checkbox" />
            <span>启用</span>
          </label>
          <button className="primary-button compact-button" type="submit">
            新增数据集
          </button>
        </form>
        <div className="dataset-list">
          {(detail.datasets || []).map((item) => (
            <div className="dataset-row" key={item.id}>
              <span>{item.name}</span>
              <code>{JSON.stringify(item.variables)}</code>
              <button className="link-button danger-link" onClick={() => removeDataset(item)} type="button">
                删除
              </button>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function caseStatusLabel(status) {
  return { draft: "草稿", active: "启用", disabled: "停用" }[status] || status || "-";
}


