import { useState } from "react";
import { Play, RefreshCw } from "lucide-react";
import { PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { configService } from "../services/configService.js";
import { performanceService } from "../services/performanceService.js";
import { formatTime, pageItems } from "../utils/formatters.js";
import { pathToHash } from "../utils/routeState.js";

const initialPlanFilters = {
  id: "",
  name: "",
  productId: "",
  loadMode: "",
  priority: "",
  status: ""
};

const emptyPlanForm = {
  productId: "",
  name: "",
  targetUrl: "",
  method: "GET",
  headers: "",
  body: "",
  loadMode: "constant",
  vus: "10",
  duration: "60s",
  stages: "",
  thresholds: "",
  status: "draft",
  priority: "P2",
  owner: "",
  tags: "",
  description: ""
};

export function PerfPlansPage() {
  const [form, setForm] = useState(initialPlanFilters);
  const [filters, setFilters] = useState(initialPlanFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [selectedIds, setSelectedIds] = useState([]);
  const [modal, setModal] = useState(null);
  const [detail, setDetail] = useState(null);
  const [notice, setNotice] = useState("");
  const [toast, setToast] = useState("");
  const [busy, setBusy] = useState(false);

  const { data, loading, error, reload } = useAsyncData(
    () => performanceService.plans.list({ ...filters, page, pageSize }),
    [filters.id, filters.name, filters.productId, filters.loadMode, filters.priority, filters.status, page, pageSize]
  );
  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const allSelected = rows.length > 0 && rows.every((row) => selectedIds.includes(row.id));

  const { data: productsData, loading: loadingProducts } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const productOptions = pageItems(productsData);

  function submitSearch(event) {
    event.preventDefault();
    setFilters({ ...form });
    setSelectedIds([]);
    setPage(1);
  }

  function resetSearch() {
    setForm(initialPlanFilters);
    setFilters(initialPlanFilters);
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
      setNotice("请先选择需要删除的压测方案。");
      return;
    }
    if (!window.confirm(`确认删除 ${ids.length} 个压测方案吗？`)) return;
    setBusy(true);
    setNotice("");
    try {
      await Promise.all(ids.map((id) => performanceService.plans.remove(id)));
      setSelectedIds([]);
      setDetail(null);
      await reload();
      setNotice("压测方案已删除。");
    } catch (err) {
      setNotice(err.message || "删除失败");
    } finally {
      setBusy(false);
    }
  }

  async function runSelected() {
    if (!selectedIds.length) {
      setNotice("请先选择需要触发的压测方案。");
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      await Promise.all(selectedIds.map((id) => performanceService.plans.run(id)));
      setToast(`已触发 ${selectedIds.length} 个压测方案执行`);
      window.setTimeout(() => setToast(""), 3600);
    } catch (err) {
      setNotice(err.message || "触发执行失败");
    } finally {
      setBusy(false);
    }
  }

  async function runPlan(row) {
    setBusy(true);
    setNotice("");
    try {
      await performanceService.plans.run(row.id);
      setToast(`压测方案“${row.name || row.id}”已触发执行`);
      window.setTimeout(() => setToast(""), 3600);
    } catch (err) {
      setNotice(err.message || "触发执行失败");
    } finally {
      setBusy(false);
    }
  }

  async function openDetail(row) {
    setBusy(true);
    setNotice("");
    try {
      setDetail(await performanceService.plans.get(row.id));
    } catch (err) {
      setNotice(err.message || "读取详情失败");
    } finally {
      setBusy(false);
    }
  }

  async function openEdit(row) {
    setBusy(true);
    setNotice("");
    try {
      const result = await performanceService.plans.get(row.id);
      setModal({ mode: "edit", row: result });
    } catch (err) {
      setNotice(err.message || "读取方案数据失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="section-stack">
      {toast ? (
        <div className="perf-toast" role="status">
          <span>{toast}</span>
          <button className="link-button" onClick={() => { window.location.hash = pathToHash(["性能测试", "测试报告"]); }} type="button">
            查看报告
          </button>
        </div>
      ) : null}
      <PageHeader title="压测方案" description="维护性能压测方案、负载配置与执行阈值" />
      <section className="resource-panel">
        <div className="panel-header">
          <strong>压测方案管理</strong>
        </div>

        <form className="filter-grid" onSubmit={submitSearch}>
          <label className="form-field">
            <span>ID</span>
            <input className="text-input" value={form.id} onChange={(event) => setForm({ ...form, id: event.target.value })} placeholder="请输入方案 ID" />
          </label>
          <label className="form-field">
            <span>方案名称</span>
            <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="请输入方案名称" />
          </label>
          <label className="form-field">
            <span>项目/产品</span>
            <select className="text-input" disabled={loadingProducts} value={form.productId} onChange={(event) => setForm({ ...form, productId: event.target.value })}>
              <option value="">{loadingProducts ? "加载产品中" : "请选择产品"}</option>
              {productOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.projectName}/{item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>负载模式</span>
            <select className="text-input" value={form.loadMode} onChange={(event) => setForm({ ...form, loadMode: event.target.value })}>
              <option value="">全部</option>
              <option value="constant">固定并发</option>
              <option value="ramping">爬坡</option>
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

        <div className="list-actions">
          <div />
          <div className="action-row">
            <button className="icon-text-button compact-button" onClick={() => reload()} type="button">
              <RefreshCw size={14} />刷新
            </button>
            <button className="success-button compact-button" disabled={busy || !selectedIds.length} onClick={runSelected} type="button">
              <Play size={14} />触发执行
            </button>
            <button className="primary-button compact-button" onClick={() => setModal({ mode: "create", row: null })} type="button">
              新增
            </button>
            <button className="danger-button compact-button" disabled={busy} onClick={() => deleteRows(selectedIds)} type="button">
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
                    <th>方案名称</th>
                    <th>项目/产品</th>
                    <th>目标接口</th>
                    <th>负载模式</th>
                    <th>并发数</th>
                    <th>时长</th>
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
                        <td>{row.name}</td>
                        <td>{row.productName || "-"}</td>
                        <td className="cell-ellipsis" title={row.targetUrl || ""}>{row.targetUrl || "-"}</td>
                        <td>{loadModeLabel(row.loadMode)}</td>
                        <td>{row.vus ?? "-"}</td>
                        <td>{row.duration || "-"}</td>
                        <td>
                          <span className={`status-pill ${planPriorityTone(row.priority)}`}>{row.priority}</span>
                        </td>
                        <td>
                          <span className={`status-pill ${planStatusTone(row.status)}`}>{planStatusLabel(row.status)}</span>
                        </td>
                        <td>{row.owner || "-"}</td>
                        <td>{formatTime(row.updatedAt)}</td>
                        <td>
                          <div className="action-links">
                            <button className="link-button" disabled={busy} onClick={() => runPlan(row)} type="button">
                              触发
                            </button>
                            <button className="link-button" onClick={() => openEdit(row)} type="button">
                              编辑
                            </button>
                            <button className="link-button" onClick={() => openDetail(row)} type="button">
                              详情
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
                      <td colSpan="13">暂无数据</td>
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

      {detail ? <PlanDetailPanel detail={detail} onClose={() => setDetail(null)} /> : null}
      {modal ? (
        <PlanModal
          busy={busy}
          modal={modal}
          onClose={() => setModal(null)}
          onSubmit={async (payload, mode) => {
            setBusy(true);
            setNotice("");
            try {
              if (mode === "edit") {
                await performanceService.plans.update(modal.row.id, payload);
                setNotice("压测方案已更新。");
              } else {
                await performanceService.plans.create(payload);
                setNotice("压测方案已创建。");
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

function planToForm(source) {
  return {
    productId: source.productId ? String(source.productId) : "",
    name: source.name || "",
    targetUrl: source.targetUrl || "",
    method: source.method || "GET",
    headers: typeof source.headers === "string" ? source.headers : source.headers ? JSON.stringify(source.headers, null, 2) : "",
    body: source.body || "",
    loadMode: source.loadMode || "constant",
    vus: source.vus != null ? String(source.vus) : "10",
    duration: source.duration || "60s",
    stages: source.stages?.length ? JSON.stringify(source.stages, null, 2) : "",
    thresholds: typeof source.thresholds === "string" ? source.thresholds : source.thresholds ? JSON.stringify(source.thresholds, null, 2) : "",
    status: source.status || "draft",
    priority: source.priority || "P2",
    owner: source.owner || "",
    tags: source.tags || "",
    description: source.description || ""
  };
}

function PlanModal({ busy, modal, onClose, onSubmit }) {
  const source = modal.row;
  const [form, setForm] = useState(source ? planToForm(source) : emptyPlanForm);
  const [error, setError] = useState("");
  const { data: productsData, loading: loadingProducts } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const productOptions = pageItems(productsData);

  async function handleSubmit(event) {
    event.preventDefault();
    setError("");
    if (!form.productId || !form.name.trim() || !form.targetUrl.trim()) {
      setError("项目/产品、方案名称和目标接口不能为空。");
      return;
    }
    let headers = {};
    let thresholds = {};
    let stages = [];
    try {
      headers = JSON.parse(form.headers.trim() || "{}");
    } catch {
      setError("请求头不是有效的 JSON。");
      return;
    }
    try {
      thresholds = JSON.parse(form.thresholds.trim() || "{}");
    } catch {
      setError("阈值不是有效的 JSON。");
      return;
    }
    if (form.loadMode === "ramping") {
      try {
        stages = JSON.parse(form.stages.trim() || "[]");
        if (!Array.isArray(stages)) throw new Error();
      } catch {
        setError("爬坡阶段不是有效的 JSON 数组。");
        return;
      }
    }
    await onSubmit(
      {
        productId: Number(form.productId),
        name: form.name.trim(),
        targetUrl: form.targetUrl.trim(),
        method: form.method,
        headers,
        body: form.body,
        loadMode: form.loadMode,
        vus: Number(form.vus || 0),
        duration: form.duration.trim(),
        stages,
        thresholds,
        status: form.status,
        priority: form.priority,
        owner: form.owner.trim(),
        tags: form.tags.trim(),
        description: form.description.trim()
      },
      modal.mode
    );
  }

  return (
    <div className="modal-backdrop">
      <form className="modal-card modal-card-wide" onSubmit={handleSubmit}>
        <div className="modal-header">
          <strong>{modal.mode === "edit" ? "编辑压测方案" : "新增压测方案"}</strong>
          <button className="modal-close" onClick={onClose} type="button">
            ×
          </button>
        </div>
        <div className="modal-grid perf-plan-form">
          <label className="form-field required-field">
            <span>项目/产品</span>
            <select className="text-input" disabled={loadingProducts} value={form.productId} onChange={(event) => setForm({ ...form, productId: event.target.value })}>
              <option value="">{loadingProducts ? "加载产品中" : "请选择产品"}</option>
              {productOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.projectName}/{item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field required-field">
            <span>方案名称</span>
            <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="请输入方案名称" />
          </label>
          <label className="form-field required-field">
            <span>目标 URL</span>
            <input className="text-input" value={form.targetUrl} onChange={(event) => setForm({ ...form, targetUrl: event.target.value })} placeholder="https://example.com/api" />
          </label>
          <label className="form-field">
            <span>请求方法</span>
            <select className="text-input" value={form.method} onChange={(event) => setForm({ ...form, method: event.target.value })}>
              <option value="GET">GET</option>
              <option value="POST">POST</option>
              <option value="PUT">PUT</option>
              <option value="DELETE">DELETE</option>
              <option value="PATCH">PATCH</option>
            </select>
          </label>
          <label className="form-field field-span-2">
            <span>请求头（JSON）</span>
            <textarea className="text-area" rows="3" value={form.headers} onChange={(event) => setForm({ ...form, headers: event.target.value })} placeholder='{"Content-Type":"application/json"}' />
          </label>
          <label className="form-field field-span-2">
            <span>请求体</span>
            <textarea className="text-area" rows="3" value={form.body} onChange={(event) => setForm({ ...form, body: event.target.value })} placeholder='{"username":"admin"}' />
          </label>
          <div className="form-field field-span-2 perf-load-config">
            <span>负载配置</span>
            <div className="perf-mode-toggle">
              <button type="button" className={form.loadMode === "constant" ? "active" : ""} onClick={() => setForm({ ...form, loadMode: "constant" })}>
                固定并发 constant
              </button>
              <button type="button" className={form.loadMode === "ramping" ? "active" : ""} onClick={() => setForm({ ...form, loadMode: "ramping" })}>
                爬坡 ramping
              </button>
            </div>
            {form.loadMode === "constant" ? (
              <div className="perf-load-fields">
                <label className="form-field">
                  <span>并发数 VU</span>
                  <input className="text-input" type="number" min="1" value={form.vus} onChange={(event) => setForm({ ...form, vus: event.target.value })} placeholder="10" />
                </label>
                <label className="form-field">
                  <span>时长</span>
                  <input className="text-input" value={form.duration} onChange={(event) => setForm({ ...form, duration: event.target.value })} placeholder="60s" />
                </label>
              </div>
            ) : (
              <label className="form-field">
                <span>爬坡阶段（JSON 数组）</span>
                <textarea className="text-area" rows="3" value={form.stages} onChange={(event) => setForm({ ...form, stages: event.target.value })} placeholder='[{"duration":"30s","target":20}]' />
              </label>
            )}
          </div>
          <label className="form-field field-span-2">
            <span>阈值（JSON）</span>
            <textarea className="text-area" rows="2" value={form.thresholds} onChange={(event) => setForm({ ...form, thresholds: event.target.value })} placeholder='{"http_req_duration":["p(95)<500"]}' />
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
            <span>优先级</span>
            <select className="text-input" value={form.priority} onChange={(event) => setForm({ ...form, priority: event.target.value })}>
              <option value="P0">P0</option>
              <option value="P1">P1</option>
              <option value="P2">P2</option>
              <option value="P3">P3</option>
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
          <label className="form-field field-span-2">
            <span>描述</span>
            <textarea className="text-area" rows="3" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
          </label>
        </div>
        {error ? <div className="form-error modal-error">{error}</div> : null}
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

function PlanDetailPanel({ detail, onClose }) {
  return (
    <section className="resource-panel detail-panel">
      <div className="panel-header">
        <strong>压测方案详情 / {detail.id} / {detail.name}</strong>
        <button className="icon-text-button compact-button" onClick={onClose} type="button">
          关闭
        </button>
      </div>
      <div className="detail-grid">
        <span>项目/产品：{detail.productName || "-"}</span>
        <span>目标接口：{detail.targetUrl || "-"}</span>
        <span>请求方法：{detail.method || "-"}</span>
        <span>负载模式：{loadModeLabel(detail.loadMode)}</span>
        <span>并发数：{detail.vus ?? "-"}</span>
        <span>时长：{detail.duration || "-"}</span>
        <span>优先级：{detail.priority || "-"}</span>
        <span>状态：{planStatusLabel(detail.status)}</span>
        <span>负责人：{detail.owner || "-"}</span>
        <span>标签：{detail.tags || "-"}</span>
        <span>创建人：{detail.createdBy || "-"}</span>
        <span>更新时间：{formatTime(detail.updatedAt)}</span>
      </div>
      <div className="detail-block">
        <strong>负载配置</strong>
        {detail.loadMode === "ramping" ? <pre className="perf-json">{formatJson(detail.stages)}</pre> : <span>并发 {detail.vus ?? "-"} · 时长 {detail.duration || "-"}</span>}
      </div>
      <div className="detail-block">
        <strong>阈值</strong>
        <pre className="perf-json">{formatJson(detail.thresholds)}</pre>
      </div>
      <div className="detail-block">
        <strong>请求头</strong>
        <pre className="perf-json">{formatJson(detail.headers)}</pre>
      </div>
      {detail.description ? (
        <div className="detail-block">
          <strong>描述</strong>
          <span>{detail.description}</span>
        </div>
      ) : null}
    </section>
  );
}

function formatJson(value) {
  if (value == null || value === "") return "暂无";
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function loadModeLabel(value) {
  return { constant: "固定并发", ramping: "爬坡" }[value] || value || "-";
}

function planStatusLabel(status) {
  return { draft: "草稿", active: "启用", disabled: "停用" }[status] || status || "-";
}

function planStatusTone(status) {
  return { draft: "neutral", active: "success", disabled: "warning" }[status] || "neutral";
}

function planPriorityTone(priority) {
  return { P0: "danger", P1: "danger", P2: "warning", P3: "neutral" }[priority] || "neutral";
}
