import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { performanceService } from "../services/performanceService.js";
import { formatTime, pageItems } from "../utils/formatters.js";

const initialRunFilters = { id: "", planId: "", status: "" };

export function PerfRunsPage() {
  const [form, setForm] = useState(initialRunFilters);
  const [filters, setFilters] = useState(initialRunFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [detail, setDetail] = useState(null);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const { data, loading, error, reload } = useAsyncData(
    () => performanceService.runs.list({ ...filters, page, pageSize }),
    [filters.id, filters.planId, filters.status, page, pageSize]
  );
  const rows = pageItems(data);
  const total = data?.total || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const { data: plansData, loading: loadingPlans } = useAsyncData(() => performanceService.plans.list({ page: 1, pageSize: 200 }), []);
  const planOptions = pageItems(plansData);

  function submitSearch(event) {
    event.preventDefault();
    setFilters({ ...form });
    setPage(1);
  }

  function resetSearch() {
    setForm(initialRunFilters);
    setFilters(initialRunFilters);
    setPage(1);
  }

  async function openDetail(row) {
    setBusy(true);
    setNotice("");
    try {
      setDetail(await performanceService.runs.get(row.id));
    } catch (err) {
      setNotice(err.message || "读取执行记录失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="section-stack">
      <PageHeader title="测试报告" description="查看性能压测执行记录与关键指标" />
      <section className="resource-panel">
        <div className="panel-header">
          <strong>性能测试报告</strong>
        </div>

        <form className="filter-grid" onSubmit={submitSearch}>
          <label className="form-field">
            <span>ID</span>
            <input className="text-input" value={form.id} onChange={(event) => setForm({ ...form, id: event.target.value })} placeholder="请输入执行记录 ID" />
          </label>
          <label className="form-field">
            <span>方案</span>
            <select className="text-input" disabled={loadingPlans} value={form.planId} onChange={(event) => setForm({ ...form, planId: event.target.value })}>
              <option value="">{loadingPlans ? "加载方案中" : "全部方案"}</option>
              {planOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>状态</span>
            <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              <option value="">全部</option>
              <option value="pending">待执行</option>
              <option value="running">执行中</option>
              <option value="completed">通过</option>
              <option value="failed">失败</option>
              <option value="canceled">取消</option>
            </select>
          </label>
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
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
          <button className="icon-text-button compact-button" onClick={() => reload()} type="button">
            <RefreshCw size={14} />刷新
          </button>
        </div>

        {notice ? <div className="inline-notice">{notice}</div> : null}

        <StateBlock loading={loading} error={error}>
          <TablePanel>
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>方案名称</th>
                    <th>状态</th>
                    <th>触发人</th>
                    <th>总请求数</th>
                    <th>平均耗时</th>
                    <th>P95 耗时</th>
                    <th>错误率</th>
                    <th>RPS</th>
                    <th>开始时间</th>
                    <th>结束时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.length ? (
                    rows.map((row) => (
                      <tr key={row.id}>
                        <td>{row.id}</td>
                        <td>{row.planName || "-"}</td>
                        <td>
                          <span className={`status-pill ${runStatusTone(row.status)}`}>{runStatusLabel(row.status)}</span>
                        </td>
                        <td>{row.triggeredBy || "-"}</td>
                        <td>{numOrDash(row.totalRequests)}</td>
                        <td>{renderMs(row.avgDurationMs)}</td>
                        <td>{renderMs(row.p95DurationMs)}</td>
                        <td>{renderPercent(row.errorRate)}</td>
                        <td>{numOrDash(row.rps)}</td>
                        <td>{formatTime(row.startedAt)}</td>
                        <td>{formatTime(row.finishedAt)}</td>
                        <td>
                          <div className="action-links">
                            <button className="link-button" disabled={busy} onClick={() => openDetail(row)} type="button">
                              详情
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan="12">暂无执行记录</td>
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

      {detail ? <RunDetailPanel detail={detail} onClose={() => setDetail(null)} /> : null}
    </div>
  );
}

function RunDetailPanel({ detail, onClose }) {
  return (
    <div className="modal-backdrop">
      <section className="modal-card modal-card-wide perf-run-report" aria-label={`执行记录详情 #${detail.id}`}>
        <div className="modal-header">
          <strong>执行记录详情 / #{detail.id}</strong>
          <button className="modal-close" onClick={onClose} type="button">
            ×
          </button>
        </div>
        <div className="perf-run-body">
          <div className="perf-run-status-row">
            <span>方案：{detail.planName || detail.planId || "-"}</span>
            <span className={`status-pill ${runStatusTone(detail.status)}`}>{runStatusLabel(detail.status)}</span>
          </div>
          <div className="perf-metric-grid">
            <PerfMetric label="总请求数" value={numOrDash(detail.totalRequests)} />
            <PerfMetric label="平均耗时" value={renderMs(detail.avgDurationMs)} />
            <PerfMetric label="P95 耗时" value={renderMs(detail.p95DurationMs)} />
            <PerfMetric label="错误率" value={renderPercent(detail.errorRate)} />
            <PerfMetric label="RPS" value={numOrDash(detail.rps)} />
          </div>
          <div className="detail-grid">
            <span>方案名称：{detail.planName || "-"}</span>
            <span>触发人：{detail.triggeredBy || "-"}</span>
            <span>退出码：{detail.exitCode ?? "-"}</span>
            <span>开始时间：{formatTime(detail.startedAt)}</span>
            <span>结束时间：{formatTime(detail.finishedAt)}</span>
            <span>记录 ID：{detail.id}</span>
          </div>
          <div className="detail-block">
            <strong>汇总数据（summary）</strong>
            <pre className="perf-json">{formatJson(detail.summary)}</pre>
          </div>
        </div>
        <div className="modal-actions">
          <button className="primary-button compact-button" onClick={onClose} type="button">
            关闭
          </button>
        </div>
      </section>
    </div>
  );
}

function PerfMetric({ label, value }) {
  return (
    <div className="perf-metric-card">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
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

function numOrDash(value) {
  if (value === null || value === undefined || value === "") return "--";
  const num = Number(value);
  return Number.isNaN(num) ? "--" : num;
}

function renderMs(value) {
  const num = numOrDash(value);
  return num === "--" ? "--" : `${num} ms`;
}

function renderPercent(value) {
  const num = numOrDash(value);
  return num === "--" ? "--" : `${num}%`;
}

function runStatusLabel(status) {
  return { pending: "待执行", running: "执行中", completed: "通过", failed: "失败", canceled: "取消" }[status] || status || "-";
}

function runStatusTone(status) {
  return { pending: "neutral", running: "warning", completed: "success", failed: "danger", canceled: "neutral" }[status] || "neutral";
}
