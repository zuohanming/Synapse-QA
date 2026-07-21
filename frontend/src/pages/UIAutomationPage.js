import { useMemo, useRef, useState } from "react";
import { useEffect } from "react";
import { TestCasesPage } from "./TestCasesPage.js";
import { DataTable, PaginationBar, TablePanel } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { ResourceListPage } from "../components/ResourceListPage.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { configService } from "../services/configService.js";
import { uiAutomationService } from "../services/uiAutomationService.js";
import { formatTime, pageItems } from "../utils/formatters.js";
import { clearPageState, persistPageState, readPageState } from "../utils/routeState.js";

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
  action: "",
  value: "",
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

const operationGroups = [
  {
    title: "WEB 浏览器操作",
    color: "#2563eb",
    items: [
      { tag: "w_wait_for_timeout", name: "强制等待", params: ["_time"] },
      { tag: "w_goto", name: "打开URL", params: ["url"] },
      { tag: "w_screenshot", name: "整个页面截图", params: ["path"] },
      { tag: "w_alert", name: "设置弹窗不予处理", params: [] },
      { tag: "w_get_cookie", name: "获取cookie", params: [] },
      { tag: "w_set_cookie", name: "设置cookie", params: ["storage_state"] },
      { tag: "w_clear_cookies", name: "清除所有cookie", params: [] },
      { tag: "w_clear_storage", name: "清除本地存储和会话存储", params: [] }
    ]
  },
  {
    title: "WEB 元素操作",
    color: "#10b981",
    items: [
      { tag: "w_click", name: "元素单击", params: ["locating"] },
      { tag: "w_dblclick", name: "元素双击", params: ["locating"] },
      { tag: "w_force_click", name: "强制单击", params: ["locating"] },
      { tag: "w_input", name: "元素输入", params: ["locating", "input_value"] },
      { tag: "w_hover", name: "鼠标悬停", params: ["locating"] },
      { tag: "w_get_text", name: "获取元素文本", params: ["locating", "set_cache_key"] },
      { tag: "w_clear_input", name: "元素清空再输入", params: ["locating", "input_value"] },
      { tag: "w_many_click", name: "多元素循环单击", params: ["locating"] },
      { tag: "w_upload_files", name: "拖拽文件上传", params: ["locating", "file_path"] },
      { tag: "w_click_upload_files", name: "点击并选择文件上传", params: ["locating", "file_path"] },
      { tag: "w_download", name: "下载文件", params: ["locating", "file_key"] },
      { tag: "w_element_wheel", name: "滚动到元素位置", params: ["locating"] },
      { tag: "w_right_click", name: "元素右键点击", params: ["locating"] },
      { tag: "w_time_click", name: "循环点击N秒", params: ["locating", "n"] },
      { tag: "w_drag_up_pixel", name: "往上拖动N个像素", params: ["locating", "n"] },
      { tag: "w_drag_down_pixel", name: "往下拖动N个像素", params: ["locating", "n"] },
      { tag: "w_drag_left_pixel", name: "往左拖动N个像素", params: ["locating", "n"] },
      { tag: "w_drag_right_pixel", name: "往右拖动N个像素", params: ["locating", "n"] },
      { tag: "w_ele_screenshot", name: "元素截图", params: ["locating", "path"] },
      { tag: "w_drag_to", name: "拖动A元素到达B", params: ["locating1", "locating2"] }
    ]
  },
  {
    title: "WEB 输入设备",
    color: "#7c3aed",
    items: [
      { tag: "w_keys", name: "模拟按键", params: ["keyboard"] },
      { tag: "w_wheel", name: "鼠标滚动", params: ["y"] },
      { tag: "w_mouse_click", name: "鼠标点击坐标", params: ["x", "y"] },
      { tag: "w_mouse_center", name: "鼠标移动到中间", params: [] },
      { tag: "w_keyboard_type_text", name: "模拟人工输入文字", params: ["text"] },
      { tag: "w_keyboard_insert_text", name: "直接输入文字", params: ["text"] },
      { tag: "w_keyboard_delete_text", name: "删除光标左侧字符", params: ["count"] }
    ]
  },
  {
    title: "WEB 页面操作",
    color: "#0f766e",
    items: [
      { tag: "w_switch_tabs", name: "切换页签", params: ["individual"] },
      { tag: "w_close_current_tab", name: "关闭当前页签", params: [] },
      { tag: "w_open_new_tab_and_switch", name: "点击并打开新页签", params: ["locating"] },
      { tag: "w_refresh", name: "刷新页面", params: [] },
      { tag: "w_go_back", name: "返回上一页", params: [] },
      { tag: "w_go_forward", name: "前进到下一页", params: [] }
    ]
  },
  {
    title: "WEB 定制开发",
    color: "#ea580c",
    items: [
      { tag: "w_demo", name: "项目自定义方法", params: ["locating", "input_value"] },
      { tag: "w_is_click", name: "元素在页面则点击", params: ["locating"] }
    ]
  },
  {
    title: "安卓 应用操作",
    color: "#16a34a",
    items: [
      { tag: "a_start_app", name: "启动应用", params: ["package_name"] },
      { tag: "a_close_app", name: "关闭应用", params: ["package_name"] },
      { tag: "a_clear_app", name: "清除app数据", params: ["package_name"] },
      { tag: "a_app_stop_all", name: "停止所有app", params: [] },
      { tag: "a_app_stop_appoint", name: "停止除指定app外所有app", params: ["package_name"] }
    ]
  },
  {
    title: "安卓 元素操作",
    color: "#0891b2",
    items: [
      { tag: "a_click", name: "元素单击", params: ["locating"] },
      { tag: "a_double_click", name: "元素双击", params: ["locating"] },
      { tag: "a_input", name: "单击输入", params: ["locating", "text"] },
      { tag: "a_set_text", name: "设置文本", params: ["locating", "text"] },
      { tag: "a_click_coord", name: "坐标单击", params: ["locating", "x", "y"] },
      { tag: "a_double_click_coord", name: "坐标双击", params: ["x", "y"] },
      { tag: "a_long_click", name: "长按元素", params: ["locating", "time_"] },
      { tag: "a_clear_text", name: "清空输入框", params: ["locating"] },
      { tag: "a_get_text", name: "获取元素文本", params: ["locating", "set_cache_key"] },
      { tag: "a_element_screenshot", name: "元素截图", params: ["locating", "file_name"] },
      { tag: "a_pinch_in", name: "元素缩小", params: ["locating"] },
      { tag: "a_pinch_out", name: "元素放大", params: ["locating"] },
      { tag: "a_wait", name: "等待元素出现", params: ["locating", "time_"] },
      { tag: "a_wait_gone", name: "等待元素消失", params: ["locating", "time_"] },
      { tag: "a_drag_to_ele", name: "拖动A元素到达B元素上", params: ["locating", "locating2"] },
      { tag: "a_drag_to_coord", name: "拖动元素到坐标上", params: ["locating", "x", "y"] },
      { tag: "a_swipe_right", name: "元素内向右滑动", params: ["locating"] },
      { tag: "a_swipe_left", name: "元素内向左滑动", params: ["locating"] },
      { tag: "a_swipe_up", name: "元素内向上滑动", params: ["locating"] },
      { tag: "a_swipe_ele", name: "元素内向下滑动", params: ["locating"] },
      { tag: "a_get_center", name: "提取元素坐标", params: ["locating", "x_key", "y_key"] }
    ]
  },
  {
    title: "安卓 设备操作",
    color: "#475569",
    items: [
      { tag: "a_sleep", name: "强制等待", params: ["_time"] },
      { tag: "a_screen_on", name: "打开屏幕", params: [] },
      { tag: "a_screen_off", name: "关闭屏幕", params: [] },
      { tag: "a_get_window_size", name: "提取屏幕尺寸", params: [] },
      { tag: "a_push", name: "推送文件到设备", params: ["file_path", "catalogue"] },
      { tag: "a_pull", name: "提取文件", params: ["file_path", "catalogue"] },
      { tag: "a_unlock", name: "解锁屏幕", params: [] },
      { tag: "a_press_home", name: "按home键", params: [] },
      { tag: "a_press_back", name: "按back键", params: [] },
      { tag: "a_press_left", name: "按left键", params: [] },
      { tag: "a_press_right", name: "按right键", params: [] },
      { tag: "a_press_up", name: "按up键", params: [] },
      { tag: "a_press_down", name: "按down键", params: [] },
      { tag: "a_press_center", name: "按center键", params: [] },
      { tag: "a_press_menu", name: "按menu键", params: [] },
      { tag: "a_press_search", name: "按search键", params: [] },
      { tag: "a_press_enter", name: "按enter键", params: [] },
      { tag: "a_press_delete", name: "按delete键", params: [] },
      { tag: "a_press_recent", name: "按recent键", params: [] },
      { tag: "a_press_volume_up", name: "按volume_up键", params: [] },
      { tag: "a_press_volume_down", name: "按volume_down键", params: [] },
      { tag: "a_press_volume_mute", name: "按volume_mute键", params: [] },
      { tag: "a_press_camera", name: "按camera键", params: [] },
      { tag: "a_press_power", name: "按power键", params: [] }
    ]
  },
  {
    title: "安卓 页面操作",
    color: "#db2777",
    items: [
      { tag: "a_swipe_down", name: "下滑", params: [] },
      { tag: "a_swipe", name: "坐标滑动", params: ["sx", "sy", "ex", "ey"] },
      { tag: "a_drag", name: "坐标拖动", params: ["sx", "sy", "ex", "ey"] },
      { tag: "a_open_quick_settings", name: "打开快速通知", params: [] },
      { tag: "a_screenshot", name: "屏幕截图", params: ["file_name"] },
      { tag: "a_set_orientation_natural", name: "设置为natural", params: [] },
      { tag: "a_set_orientation_left", name: "设置为left", params: [] },
      { tag: "a_set_orientation_right", name: "设置为right", params: [] },
      { tag: "a_set_orientation_upsidedown", name: "设置为upsidedown", params: [] },
      { tag: "a_freeze_rotation", name: "冻结旋转", params: [] },
      { tag: "a_freeze_rotation_false", name: "取消冻结旋转", params: [] },
      { tag: "a_dump_hierarchy", name: "获取转储内容", params: [] },
      { tag: "a_open_notification", name: "打开通知", params: [] }
    ]
  }
];

const visibleOperationGroups = operationGroups.filter(
  (group) => group.title !== "WEB 定制开发" && !group.title.startsWith("安卓 ")
);

const operationParamLabels = {
  _time: "等待时间",
  locating: "选择元素",
  locating1: "选择元素 A",
  locating2: "选择元素 B",
  input_value: "输入内容",
  url: "URL 地址",
  path: "保存路径",
  file_path: "文件路径",
  file_name: "文件名称",
  storage_state: "Cookie 数据",
  set_cache_key: "缓存变量名",
  file_key: "文件变量名",
  keyboard: "按键",
  text: "文本内容",
  count: "数量",
  package_name: "应用包名",
  catalogue: "目录",
  time_: "等待时间",
  n: "数值",
  individual: "页签序号",
  x: "横坐标 X",
  y: "纵坐标 Y",
  sx: "起点 X",
  sy: "起点 Y",
  ex: "终点 X",
  ey: "终点 Y",
  x_key: "X 坐标变量名",
  y_key: "Y 坐标变量名"
};

function operationGroupLabel(title = "") {
  return title.startsWith("WEB ") ? title.slice(4) : title.replace(/^安卓 /, "安卓·");
}

const stepNodeTypes = [
  { label: "元素操作", color: "#10b981" },
  { label: "断言操作", color: "#2548b8" },
  { label: "SQL操作", color: "#d97706" },
  { label: "自定义变量", color: "#2548b8" },
  { label: "条件判断", color: "#64748b" },
  { label: "python代码", color: "#60a5fa" }
];

export function UIAutomationPage({ activePath }) {
  const section = activePath[1];

  if (section === "页面元素") {
    return <PageElementsWorkspace />;
  }

  if (section === "页面步骤") {
    return <PageStepsPage />;
  }

  if (section === "测试用例") {
    return <TestCasesPage />;
  }

  const resource = listSectionMap[section] || uiAutomationService.elements;
  const { data, loading, error } = useAsyncData(() => resource.list({ page: 1, pageSize: 20 }), [section]);

  return (
    <ResourceListPage
      title={section}
      description="界面自动化资产管理"
      panelTitle={`${section}列表`}
      rows={pageItems(data)}
      loading={loading}
      error={error}
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
  const [workingStep, setWorkingStep] = useState(() => readPageState("ui.steps.workingStep", null));

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

  const { data: productsData, loading: loadingProducts } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const stepProductOptions = useMemo(
    () =>
      pageItems(productsData).map((item) => ({
        label: `${item.projectName}/${item.name}`,
        value: `${item.projectName}/${item.name}`,
        productId: item.id
      })),
    [productsData]
  );
  const selectedFilterProduct = stepProductOptions.find((item) => item.value === form.product);
  const { data: modulesData, loading: loadingModules } = useAsyncData(
    () => (selectedFilterProduct?.productId ? configService.productModules.list({ productId: selectedFilterProduct.productId, page: 1, pageSize: 200 }) : Promise.resolve({ items: [] })),
    [selectedFilterProduct?.productId]
  );
  const moduleOptions = useMemo(() => uniqueOptions(pageItems(modulesData), "name"), [modulesData]);
  const { data: pagesData, loading: loadingPages } = useAsyncData(
    () => uiAutomationService.elements.list({ product: form.product, module: form.module, page: 1, pageSize: 200 }),
    [form.product, form.module]
  );
  const pageOptions = useMemo(() => uniqueOptions(pageItems(pagesData), "name"), [pagesData]);

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
    return (
      <StepWorkbench
        step={workingStep}
        onBack={() => {
          clearPageState("ui.steps.workingStep");
          setWorkingStep(null);
        }}
      />
    );
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
            <select
              className="text-input"
              disabled={loadingProducts}
              value={form.product}
              onChange={(event) => setForm({ ...form, product: event.target.value, module: "", page: "" })}
            >
              <option value="">{loadingProducts ? "加载产品中" : "请选择产品"}</option>
              {stepProductOptions.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>模块名称</span>
            <select
              className="text-input"
              disabled={!form.product || loadingModules}
              value={form.module}
              onChange={(event) => setForm({ ...form, module: event.target.value, page: "" })}
            >
              <option value="">{form.product ? (loadingModules ? "加载模块中" : "请选择模块") : "请先选择产品"}</option>
              {moduleOptions.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>所属页面</span>
            <select className="text-input" disabled={!form.product || !form.module || loadingPages} value={form.page} onChange={(event) => setForm({ ...form, page: event.target.value })}>
              <option value="">{form.product && form.module ? (loadingPages ? "加载页面中" : "请选择所属页面") : "请先选择模块"}</option>
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
                            <button
                              className="link-button"
                              onClick={() => {
                                persistPageState("ui.steps.workingStep", row);
                                setWorkingStep(row);
                              }}
                              type="button"
                            >
                              调试
                            </button>
                            <button className="link-button" onClick={() => setModal({ mode: "edit", row })} type="button">
                              编辑
                            </button>
                            <button
                              className="link-button"
                              onClick={() => {
                                persistPageState("ui.steps.workingStep", row);
                                setWorkingStep(row);
                              }}
                              type="button"
                            >
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

function StepModal({ busy, modal, onClose, onSubmit }) {
  const source = modal.row;
  const [form, setForm] = useState(
    source
      ? {
          name: source.name || "",
          category: source.category || "",
          method: source.method || "",
          locator: source.locator || "",
          action: source.action || "",
          value: source.value || "",
          description: source.description || "",
          status: source.status || "active"
        }
      : emptyStepForm
  );
  const [error, setError] = useState("");
  const { data: productsData, loading: loadingProducts } = useAsyncData(() => configService.products.list({ page: 1, pageSize: 200 }), []);
  const productOptions = useMemo(() => {
    const options = pageItems(productsData).map((item) => ({
      label: `${item.projectName}/${item.name}`,
      value: `${item.projectName}/${item.name}`,
      productId: item.id
    }));
    if (form.category && !options.some((item) => item.value === form.category)) {
      return [{ label: form.category, value: form.category, productId: null }, ...options];
    }
    return options;
  }, [form.category, productsData]);
  const selectedProduct = productOptions.find((item) => item.value === form.category);
  const { data: modulesData, loading: loadingModules } = useAsyncData(
    () => (selectedProduct?.productId ? configService.productModules.list({ productId: selectedProduct.productId, page: 1, pageSize: 200 }) : Promise.resolve({ items: [] })),
    [selectedProduct?.productId]
  );
  const moduleOptions = useMemo(() => {
    const options = uniqueOptions(pageItems(modulesData), "name");
    if (form.method && !options.includes(form.method)) {
      return [form.method, ...options];
    }
    return options;
  }, [form.method, modulesData]);
  const { data: pagesData, loading: loadingPages } = useAsyncData(
    () => uiAutomationService.elements.list({ product: form.category, module: form.method, page: 1, pageSize: 200 }),
    [form.category, form.method]
  );
  const pageOptions = useMemo(() => {
    const options = uniqueOptions(pageItems(pagesData), "name");
    if (form.locator && !options.includes(form.locator)) {
      return [form.locator, ...options];
    }
    return options;
  }, [form.locator, pagesData]);

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
        action: form.action,
        value: form.value.trim(),
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
            <select
              className="text-input"
              disabled={loadingProducts}
              value={form.category}
              onChange={(event) => setForm({ ...form, category: event.target.value, method: "", locator: "" })}
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
            <select
              className="text-input"
              disabled={!form.category || loadingModules}
              value={form.method}
              onChange={(event) => setForm({ ...form, method: event.target.value, locator: "" })}
            >
              <option value="">{form.category ? (loadingModules ? "加载模块中" : "请选择测试模块") : "请先选择项目名称"}</option>
              {moduleOptions.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label className="form-field form-field-inline required-field">
            <span>所属页面</span>
            <select className="text-input" disabled={!form.category || !form.method || loadingPages} value={form.locator} onChange={(event) => setForm({ ...form, locator: event.target.value })}>
              <option value="">{form.category && form.method ? (loadingPages ? "加载页面中" : "请选择步骤所属页面") : "请先选择模块"}</option>
              {pageOptions.map((item) => (
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
  const [connections, setConnections] = useState([]);
  const [nodeSeq, setNodeSeq] = useState(1);
  const [selectedNode, setSelectedNode] = useState(null);
  const [draggingItem, setDraggingItem] = useState(null);
  const [draggingNode, setDraggingNode] = useState(null);
  const [panningCanvas, setPanningCanvas] = useState(null);
  const [zoom, setZoom] = useState(1);
  const [configError, setConfigError] = useState("");
  const [operationMenuOpen, setOperationMenuOpen] = useState(false);
  const [activeOperationGroup, setActiveOperationGroup] = useState(visibleOperationGroups[0].title);
  const [pageElementOptions, setPageElementOptions] = useState([]);
  const [pageElementsLoading, setPageElementsLoading] = useState(false);
  const [toast, setToast] = useState("");
  const [connectingFrom, setConnectingFrom] = useState(null);
  const canvasRef = useRef(null);
  const operationPickerRef = useRef(null);
  const zoomRef = useRef(1);
  const lastDropAt = useRef(0);
  const selected = selectedNode ? nodes.find((node) => node.id === selectedNode.id) || selectedNode : null;
  const canvasSize = { width: 1200, height: 720 };
  const operationOptions = operationGroups.flatMap((group) => group.items.map((item) => ({ ...item, group: group.title })));
  const activeGroup = visibleOperationGroups.find((group) => group.title === activeOperationGroup) || visibleOperationGroups[0];
  const operationDisplay = selected?.tag
    ? `${operationGroupLabel(selected.operationGroup)} / ${selected.operationName}`
    : "请选择元素操作";
  const connectionCount = connections.length;
  const connectedNodeIds = new Set(connections.flatMap((connection) => [connection.from, connection.to]));
  const unconnectedCount = nodes.filter((node) => !connectedNodeIds.has(node.id)).length;

  useEffect(() => {
    const closeOperationMenu = (event) => {
      if (!operationPickerRef.current?.contains(event.target)) setOperationMenuOpen(false);
    };
    document.addEventListener("mousedown", closeOperationMenu);
    return () => document.removeEventListener("mousedown", closeOperationMenu);
  }, []);

  useEffect(() => {
    let active = true;
    async function loadPageElements() {
      setPageElementsLoading(true);
      try {
        const pages = await uiAutomationService.elements.list({ pageName: step.locator || "", page: 1, pageSize: 100 });
        const pageRows = pageItems(pages);
        const pageRow = pageRows.find((item) => item.name === step.locator) || pageRows[0];
        if (!pageRow) {
          if (active) setPageElementOptions([]);
          return;
        }
        const elements = await uiAutomationService.pageElements.list({ pageId: pageRow.id, page: 1, pageSize: 200 });
        if (active) {
          setPageElementOptions(
            pageItems(elements)
              .map((item) => ({ id: item.id, name: item.name, locator: item.locator1 || item.locator2 || item.locator3 || "" }))
              .filter((item) => item.locator)
          );
        }
      } catch {
        if (active) setPageElementOptions([]);
      } finally {
        if (active) setPageElementsLoading(false);
      }
    }
    loadPageElements();
    return () => {
      active = false;
    };
  }, [step.locator]);

  useEffect(() => {
    if (!toast) return undefined;
    const timer = window.setTimeout(() => setToast(""), 2200);
    return () => window.clearTimeout(timer);
  }, [toast]);

  const createNode = (item, event) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const nextNode = {
      id: nodeSeq,
      type: item.label,
      color: item.color,
      title: item.label,
      tag: "",
      operationName: "",
      operationGroup: "",
      params: [],
      values: {},
      locator: "未配置",
      saved: false,
      x: Math.max(24, (event.clientX - rect.left + event.currentTarget.scrollLeft) / zoomRef.current - 78),
      y: Math.max(24, (event.clientY - rect.top + event.currentTarget.scrollTop) / zoomRef.current - 24)
    };

    const previousNode = nodes[nodes.length - 1];
    setNodeSeq((value) => value + 1);
    setNodes((current) => [...current, nextNode]);
    if (previousNode) {
      setConnections((current) => [...current, { from: previousNode.id, to: nextNode.id }]);
    }
    setSelectedNode(nextNode);
  };

  const applyZoom = (nextZoom, anchor) => {
    const canvas = canvasRef.current;
    if (!canvas) {
      setZoom(nextZoom);
      return;
    }

    const rect = canvas.getBoundingClientRect();
    const offsetX = anchor?.x ?? rect.width / 2;
    const offsetY = anchor?.y ?? rect.height / 2;
    const currentZoom = zoomRef.current;
    const logicalX = (canvas.scrollLeft + offsetX) / currentZoom;
    const logicalY = (canvas.scrollTop + offsetY) / currentZoom;

    zoomRef.current = nextZoom;
    setZoom(nextZoom);
    requestAnimationFrame(() => {
      canvas.scrollLeft = logicalX * nextZoom - offsetX;
      canvas.scrollTop = logicalY * nextZoom - offsetY;
    });
  };

  const changeZoom = (direction, anchor) => {
    const nextZoom = Math.max(0.2, Math.min(3, Math.round((zoomRef.current + direction * 0.1) * 10) / 10));
    if (nextZoom !== zoomRef.current) {
      applyZoom(nextZoom, anchor);
    }
  };

  const resetZoom = () => {
    if (zoomRef.current !== 1) {
      applyZoom(1);
    }
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
    if (panningCanvas) {
      setPanningCanvas(null);
      return;
    }
    if (draggingNode) {
      setDraggingNode(null);
      return;
    }
    if (!draggingItem || Date.now() - lastDropAt.current < 100) return;
    createNode(draggingItem, event);
    setDraggingItem(null);
  };

  const handleNodeMouseDown = (event, node) => {
    event.preventDefault();
    event.stopPropagation();
    setSelectedNode(node);
    setDraggingNode({
      id: node.id,
      startClientX: event.clientX,
      startClientY: event.clientY,
      startX: node.x,
      startY: node.y
    });
  };

  const handleCanvasMouseMove = (event) => {
    if (panningCanvas) {
      const canvas = canvasRef.current;
      if (!canvas) return;
      canvas.scrollLeft = panningCanvas.scrollLeft - (event.clientX - panningCanvas.clientX);
      canvas.scrollTop = panningCanvas.scrollTop - (event.clientY - panningCanvas.clientY);
      return;
    }
    if (!draggingNode) return;
    const nextX = draggingNode.startX + (event.clientX - draggingNode.startClientX) / zoomRef.current;
    const nextY = draggingNode.startY + (event.clientY - draggingNode.startClientY) / zoomRef.current;
    setNodes((current) =>
      current.map((node) =>
        node.id === draggingNode.id
          ? {
              ...node,
              x: Math.max(0, Math.min(canvasSize.width - 156, nextX)),
              y: Math.max(0, Math.min(canvasSize.height - 64, nextY))
            }
          : node
      )
    );
  };

  const handleCanvasMouseDown = (event) => {
    if (event.button !== 0 || draggingItem || event.target.closest(".flow-node") || event.target.closest(".zoom-indicator")) {
      return;
    }
    const canvas = canvasRef.current;
    if (!canvas) return;
    setPanningCanvas({
      clientX: event.clientX,
      clientY: event.clientY,
      scrollLeft: canvas.scrollLeft,
      scrollTop: canvas.scrollTop
    });
  };

  const startConnection = (event, node) => {
    event.preventDefault();
    event.stopPropagation();
    setConnectingFrom(node.id);
    setSelectedNode(node);
    setToast("请选择目标节点");
  };

  const completeConnection = (event, node) => {
    event.preventDefault();
    event.stopPropagation();
    if (!connectingFrom) return;
    if (connectingFrom === node.id) {
      setToast("不能连接当前节点");
      return;
    }
    const remainingConnections = connections.filter((connection) => connection.from !== connectingFrom && connection.to !== node.id);
    let cursor = node.id;
    const visited = new Set();
    while (cursor && !visited.has(cursor)) {
      if (cursor === connectingFrom) {
        setToast("不能形成循环连接");
        return;
      }
      visited.add(cursor);
      cursor = remainingConnections.find((connection) => connection.from === cursor)?.to;
    }
    setConnections([...remainingConnections, { from: connectingFrom, to: node.id }]);
    setConnectingFrom(null);
    setSelectedNode(node);
    setToast("连接成功");
  };

  const deleteSelectedNode = () => {
    if (!selected) return;
    setNodes((current) => current.filter((node) => node.id !== selected.id));
    setConnections((current) => current.filter((connection) => connection.from !== selected.id && connection.to !== selected.id));
    if (connectingFrom === selected.id) setConnectingFrom(null);
    setSelectedNode(null);
    setConfigError("");
    setToast("节点已删除");
  };

  const updateSelectedNode = (patch) => {
    if (!selected) return;
    setNodes((current) => current.map((node) => (node.id === selected.id ? { ...node, ...patch } : node)));
    setSelectedNode((current) => (current ? { ...current, ...patch } : current));
  };

  const selectOperation = (tag) => {
    setConfigError("");
    setOperationMenuOpen(false);
    const item = operationOptions.find((option) => option.tag === tag);
    if (!item) {
      updateSelectedNode({
        operationGroup: "",
        operationName: "",
        title: selected.type,
        tag: "",
        params: [],
        values: {},
        locator: "未配置",
        saved: false
      });
      return;
    }
    const values = item.params.reduce((result, param) => ({ ...result, [param]: selected?.values?.[param] || "" }), {});
    updateSelectedNode({
      operationGroup: item.group,
      operationName: item.name,
      title: item.name,
      tag: item.tag,
      params: item.params,
      values,
      locator: item.params.includes("locating") ? values.locating || "请配置元素定位" : item.params.join(", ") || "无需参数",
      saved: false
    });
  };

  const toggleOperationMenu = () => {
    if (!operationMenuOpen) {
      const selectedGroupVisible = visibleOperationGroups.some((group) => group.title === selected?.operationGroup);
      setActiveOperationGroup(selectedGroupVisible ? selected.operationGroup : visibleOperationGroups[0].title);
    }
    setOperationMenuOpen((open) => !open);
  };

  const updateOperationParam = (param, value) => {
    setConfigError("");
    updateSelectedNode({ values: { ...selected.values, [param]: value }, saved: false });
  };

  const saveNodeConfig = () => {
    if (!selected.tag) {
      setConfigError("请选择元素操作");
      return;
    }
    const missingParam = selected.params.find((param) => !String(selected.values?.[param] || "").trim());
    if (missingParam) {
      setConfigError(`请输入${operationParamLabels[missingParam] || missingParam}`);
      return;
    }
    setConfigError("");
    updateSelectedNode({
      saved: true,
      locator: selected.params.includes("locating") ? selected.values.locating : selected.params.length ? "参数已配置" : "无需参数"
    });
    setToast("保存成功");
  };

  const handleWheel = (event) => {
    event.preventDefault();
    const rect = event.currentTarget.getBoundingClientRect();
    changeZoom(event.deltaY > 0 ? -1 : 1, {
      x: event.clientX - rect.left,
      y: event.clientY - rect.top
    });
  };

  return (
    <section className="step-workbench">
      <div className="step-workbench-header">
        <div className="toolbar-title">
          <strong>页面步骤工作台 / {step.id} / {step.name || "-"}</strong>
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
          <div className="toolbar-title">
            <strong>操作面板</strong>
          </div>
          <div className="palette-list">
            {stepNodeTypes.map((item) => (
              <button
                className="palette-item node-type-item"
                draggable
                key={item.label}
                onDragEnd={() => setDraggingItem(null)}
                onDragStart={(event) => handleDragStart(event, item)}
                onMouseDown={() => setDraggingItem(item)}
                type="button"
              >
                <span style={{ background: item.color }} />
                <strong>{item.label}</strong>
              </button>
            ))}
          </div>
        </aside>

        <main className="flow-panel">
          <div className="flow-panel-header">
            <div className="toolbar-title">
              <strong>流程画布</strong>
              {connectingFrom ? <small className="connection-mode-tip">请选择目标节点左侧连接点</small> : null}
            </div>
            <div className="flow-stats">
              <span className="warning-stat">未保存 {nodes.filter((node) => !node.saved).length}</span>
              <span className="warning-stat">未连接 {unconnectedCount}</span>
              <span>节点 {nodes.length}</span>
              <span>连线 {connectionCount}</span>
              <span>步骤 {nodes.length}</span>
            </div>
          </div>
          <div
            className={`${nodes.length ? "flow-canvas" : "flow-canvas empty"}${panningCanvas ? " is-panning" : ""}`}
            onDragOver={(event) => event.preventDefault()}
            onDrop={handleDrop}
            onMouseDown={handleCanvasMouseDown}
            onMouseLeave={() => {
              setDraggingNode(null);
              setPanningCanvas(null);
            }}
            onMouseMove={handleCanvasMouseMove}
            onMouseUp={handleCanvasMouseUp}
            onWheel={handleWheel}
            ref={canvasRef}
          >
            <div className="zoom-indicator">
              <button aria-label="缩小画布" onClick={() => changeZoom(-1)} type="button">
                -
              </button>
              <strong>{Math.round(zoom * 100)}%</strong>
              <button aria-label="放大画布" onClick={() => changeZoom(1)} type="button">
                +
              </button>
              <button onClick={resetZoom} type="button">
                100%
              </button>
            </div>
            {nodes.length ? (
              <>
                <div className="canvas-content" style={{ height: canvasSize.height * zoom, width: canvasSize.width * zoom }}>
                  <div className="free-node-layer" style={{ height: canvasSize.height, transform: `scale(${zoom})`, width: canvasSize.width }}>
                    <svg className="flow-connections" height={canvasSize.height} width={canvasSize.width} aria-hidden="true">
                      <defs>
                        <marker id="flow-arrow" markerHeight="7" markerWidth="8" orient="auto" refX="7" refY="3.5">
                          <path d="M0,0 L8,3.5 L0,7 Z" />
                        </marker>
                      </defs>
                      {connections.map((connection) => {
                        const node = nodes.find((item) => item.id === connection.from);
                        const nextNode = nodes.find((item) => item.id === connection.to);
                        if (!node || !nextNode) return null;
                        const startX = node.x + 156;
                        const startY = node.y + 32;
                        const endX = nextNode.x;
                        const endY = nextNode.y + 32;
                        const curve = Math.max(55, Math.abs(endX - startX) / 2);
                        return (
                          <path
                            className="flow-connection-path"
                            d={`M ${startX} ${startY} C ${startX + curve} ${startY}, ${endX - curve} ${endY}, ${endX} ${endY}`}
                            key={`${connection.from}-${connection.to}`}
                            markerEnd="url(#flow-arrow)"
                          />
                        );
                      })}
                    </svg>
                    {nodes.map((node) => (
                      <div
                        className={`${selected?.id === node.id ? "flow-node active" : "flow-node"}${connectingFrom === node.id ? " is-connecting" : ""}`}
                        key={node.id}
                        onMouseDown={(event) => handleNodeMouseDown(event, node)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter" || event.key === " ") setSelectedNode(node);
                        }}
                        role="button"
                        style={{ borderLeftColor: node.color, left: node.x, top: node.y }}
                        tabIndex="0"
                      >
                        <button aria-label={`连接到${node.title}`} className="node-port input-port" onClick={(event) => completeConnection(event, node)} onMouseDown={(event) => event.stopPropagation()} type="button" />
                        <span>{node.type}</span>
                        <strong>{node.title}</strong>
                        <small>{node.tag || "未配置"}</small>
                        <em>{node.saved ? "已配置" : "未保存"}</em>
                        <button aria-label={`从${node.title}开始连接`} className="node-port output-port" onClick={(event) => startConnection(event, node)} onMouseDown={(event) => event.stopPropagation()} type="button" />
                      </div>
                    ))}
                  </div>
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
          <div className="node-detail-title">
            <div>
              <strong>节点详情</strong>
              <p>{selected?.type || "选择画布节点后维护配置"}</p>
            </div>
            {selected ? <span>{selected.type}</span> : null}
          </div>
          <div className="detail-tabs">
            <button className="active" type="button">
              节点配置
            </button>
            <button type="button">调试结果</button>
          </div>
          {selected ? (
            <div className="node-config">
              <div className="recent-element-box">
                <strong>最近测试的元素信息</strong>
                <div>暂无元素信息</div>
              </div>
              <div className="node-section-title">节点详情</div>
              <div className="form-field required-field">
                <span>元素操作</span>
                <div className="operation-cascader" ref={operationPickerRef}>
                  <button
                    aria-expanded={operationMenuOpen}
                    aria-haspopup="listbox"
                    className={operationMenuOpen ? "operation-cascader-trigger is-open" : "operation-cascader-trigger"}
                    onClick={toggleOperationMenu}
                    type="button"
                  >
                    <span className={selected.tag ? "" : "placeholder"}>{operationDisplay}</span>
                    <i>⌄</i>
                  </button>
                  {operationMenuOpen ? (
                    <div className="operation-cascader-menu">
                      <div className="operation-group-list" role="listbox" aria-label="操作分类">
                        {visibleOperationGroups.map((group) => (
                          <button
                            aria-selected={activeGroup.title === group.title}
                            className={activeGroup.title === group.title ? "active" : ""}
                            key={group.title}
                            onClick={() => setActiveOperationGroup(group.title)}
                            onMouseEnter={() => setActiveOperationGroup(group.title)}
                            type="button"
                          >
                            <span>{operationGroupLabel(group.title)}</span><i>›</i>
                          </button>
                        ))}
                      </div>
                      <div className="operation-item-list" role="listbox" aria-label={operationGroupLabel(activeGroup.title)}>
                        {activeGroup.items.map((item) => (
                          <button
                            aria-selected={selected.tag === item.tag}
                            className={selected.tag === item.tag ? "active" : ""}
                            key={item.tag}
                            onClick={() => selectOperation(item.tag)}
                            type="button"
                          >
                            {item.name}
                          </button>
                        ))}
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>
              {selected.params.map((param) => (
                <label className="form-field required-field" key={param}>
                  <span>{operationParamLabels[param] || param}</span>
                  {param.startsWith("locating") ? (
                    <select
                      className="text-input"
                      disabled={pageElementsLoading || pageElementOptions.length === 0}
                      value={selected.values?.[param] || ""}
                      onChange={(event) => updateOperationParam(param, event.target.value)}
                    >
                      <option value="">
                        {pageElementsLoading ? "正在加载页面元素" : pageElementOptions.length ? "请选择页面元素" : "暂无可选页面元素"}
                      </option>
                      {pageElementOptions.map((element) => (
                        <option key={`${param}-${element.id}`} value={element.locator}>
                          {element.name}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <textarea
                      className="text-area node-param-input"
                      placeholder={`请输入${operationParamLabels[param] || param}`}
                      rows="3"
                      value={selected.values?.[param] || ""}
                      onChange={(event) => updateOperationParam(param, event.target.value)}
                    />
                  )}
                </label>
              ))}
              <label className="form-field">
                <span>备注</span>
                <textarea
                  className="text-area"
                  rows="3"
                  value={selected.remark || ""}
                  onChange={(event) => updateSelectedNode({ remark: event.target.value })}
                />
              </label>
              {configError ? <div className="form-error">{configError}</div> : null}
              <div className="node-config-actions">
                <button className="danger-button compact-button" onClick={deleteSelectedNode} type="button">
                  删除节点
                </button>
                <button className="primary-button compact-button" onClick={saveNodeConfig} type="button">
                  保存配置
                </button>
              </div>
            </div>
          ) : (
            <div className="empty-detail">
              <strong>暂未选择节点</strong>
              <p>在左侧画布中点击一个节点后，这里会显示元素、操作和断言配置。</p>
            </div>
          )}
        </aside>
      </div>
      {toast ? <div className="success-toast" role="status">{toast}</div> : null}
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
  const [selectedPage, setSelectedPage] = useState(() => readPageState("ui.elements.selectedPage", null));
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
    return (
      <PageElementPanel
        key={selectedPage.id}
        pageRow={selectedPage}
        onBack={() => {
          clearPageState("ui.elements.selectedPage");
          setSelectedPage(null);
        }}
      />
    );
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
                            <button
                              className="link-button"
                              onClick={() => {
                                persistPageState("ui.elements.selectedPage", row);
                                setSelectedPage(row);
                              }}
                              type="button"
                            >
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
    <div className="section-stack">
      <PageHeader title="页面元素配置" description={`页面对象：${pageRow.id} / ${pageRow.name || "-"}`} />
      <section className="resource-panel">
        <div className="panel-header">
          <strong>元素列表</strong>
        </div>

        <div className="list-actions">
          <button className="icon-text-button compact-button" onClick={onBack} type="button">
            返回
          </button>
          <div className="action-row">
            <button className="icon-text-button compact-button" type="button">下载模板</button>
            <button className="icon-text-button compact-button" type="button">点击上传</button>
            <button className="primary-button compact-button" onClick={() => setModal({ mode: "create", row: null })} type="button">新增</button>
            <button className="danger-button compact-button" disabled={busy} onClick={handleBulkDelete} type="button">批量删除</button>
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
      </section>

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
    </div>
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
      productId: item.id,
      uiType: item.uiType || "WEB"
    }));
    if (form.category && !options.some((item) => item.value === form.category)) {
      return [{ label: form.category, value: form.category, productId: null, uiType: form.value || "WEB" }, ...options];
    }
    return options;
  }, [form.category, form.value, productsData]);
  const selectedProduct = productOptions.find((item) => item.value === form.category);
  const { data: modulesData, loading: loadingModules } = useAsyncData(
    () => (selectedProduct?.productId ? configService.productModules.list({ productId: selectedProduct.productId, page: 1, pageSize: 200 }) : Promise.resolve({ items: [] })),
    [selectedProduct?.productId]
  );
  const moduleOptions = useMemo(() => {
    const moduleRows = pageItems(modulesData);
    const options = moduleRows.length
      ? uniqueOptions(moduleRows, "name")
      : uniqueOptions(
          (pageRows || []).filter((row) => !form.category || row.category === form.category),
          "method"
        );
    if (form.method && !options.includes(form.method)) {
      return [form.method, ...options];
    }
    return options;
  }, [form.category, form.method, modulesData, pageRows]);

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
            <select className="text-input" disabled={!form.category || loadingModules} value={form.method} onChange={(event) => setForm({ ...form, method: event.target.value })}>
              <option value="">{form.category ? (loadingModules ? "加载模块中" : "请选择测试模块") : "请先选择项目名称"}</option>
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
