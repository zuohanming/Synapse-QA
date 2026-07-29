# Task 6 交接报告：执行器有头浏览器元素拾取

## 实现前架构与协议检查

### 已确认边界

- Task 4 命令由 `GET /api/executor/element-capture/commands?executorId=...` 领取，使用长期 `X-Executor-Token`；普通命令由 `POST /commands/:id/ack` 携带 lease receipt 确认。
- `start` 不能调用普通 ACK。首次会话 heartbeat 必须携带 start receipt，由平台在同一事务内激活会话并确认 start；成功后后续 heartbeat 不再携带 receipt。
- 回调使用 `X-Executor-ID` 与 `Authorization: Bearer <session token>`。HTTP 401 立即清理本地会话并通知 GUI 认证失效；409 清理冲突会话；网络失败保留会话、receipt 和待上报候选，等待下一轮重试。
- 命令轮询在独立常驻 asyncio 事件循环中每 2 秒运行，Playwright 对象始终由同一线程/事件循环使用；FastAPI startup/shutdown 只启动一个 poller，并在 shutdown 的同一事件循环中关闭会话。
- 采集使用独立 headed Chromium browser/context/page，只允许 `chrome`、`msedge` channel，不接受 `executable_path`；它不进入 `TaskManager`，因此不占普通任务槽位。

### 组件职责

- `capture_platform_client.py`：封装命令领取、heartbeat、候选、失败与普通 ACK 的 HTTP 契约，并将响应分类为成功、401、409、其它 HTTP 错误和可重试网络错误；不记录 token、receipt 或响应正文。
- `capture_session_manager.py`：持有唯一采集会话和 Playwright 生命周期；重复同 session 的 start 幂等，另一 session 明确冲突；管理首次 receipt、模式、待上报候选、临时截图与彻底清理。
- `element_picker.py`：提供 Shadow Root 工具条/高亮脚本、DOM allowlist 校验、可信 locator 匹配复核、前后敏感扫描、局部截图遮罩和随机临时文件。网页侧传入的 CSS/XPath/selector 一律拒绝，定位器只由 Python 根据结构化特征生成。
- `routes.py`：组装 manager/client/poller，绑定 FastAPI 生命周期，并在 `/health` 仅暴露 active/mode/pageTitle 等非敏感状态。
- `gui.py`：把 poller 线程的状态通过 `root.after(0, ...)` 切回 Tk 主线程；采集时显示“正在采集 · 页面名”，停止后恢复服务状态，认证失效返回登录页。

### Task 5 残余风险补偿

进度账本明确记录 Task 5 最终复审未清洁，残余项包括敏感值处理、外部 selector 信任、动态 token、指纹稳定性和平台载荷校验。因此 Task 6 不把 `build_candidate` 视为唯一安全边界：

1. binding 入站只接受固定字段和固定属性 allowlist；出现 `value`、storage、cookie、outerHTML、authorization 或 selector 类字段时整条候选丢弃。
2. 入站结构化值在调用 Task 5 前扫描 JWT、Bearer、常见 API key、会话/令牌键及高熵值。
3. Python 生成候选后，仅对 `platform_payload()` 做第二次递归敏感扫描；失败只记录不含原值的安全错误。
4. locator match count 由可信 Playwright frame 对 Python 生成的定位器重新计算，不信任网页上报的 selector 或唯一性。

### 清理与可验证完成标准

- stop、expire、401、409、启动失败和应用 shutdown 均按 page → context → browser → Playwright 顺序尽力关闭，取消会话任务、清空 token/receipt/队列并删除截图和会话临时目录；部分初始化失败也走同一清理路径。
- 截图只调用目标 `ElementHandle.screenshot`；截图前用独立 Shadow Root 遮罩 password、tel 和身份标识输入框，finally 中移除遮罩。截图绝不进入平台 payload，成功上报即删除；网络失败仅在内存队列中保留到重试。
- fake Playwright 单测覆盖单会话、幂等、channel/headed、启动回滚、命令 ACK/receipt/401/409/网络重试、picker 模式与键盘行为、Shadow/iframe、allowlist、双扫描、遮罩、清理、GUI 回调及 poller 生命周期；另以本地静态 HTML 尝试运行 installed Edge 的 headed 集成测试。

## TDD 记录

- 基线：`cd executor; python -m pytest -q` → `55 passed`。
- RED（首层）：`cd executor; python -m pytest tests/test_capture_session_manager.py tests/test_element_picker.py -q` 在收集阶段失败，明确缺少 `CaptureCommand`、`CaptureMode` 与 `CaptureStartCommand`，符合新功能尚未实现的预期。
- RED（第二层）：补最小协议模型后，同一命令明确因缺少 `capture_platform_client` 与 `element_picker` 模块失败。
- RED（契约补强）：真实 Edge 首次运行证明 init script 在 DOM 根节点就绪前错误标记安装、handle binding 错传两个参数、同源 iframe 未显式递归安装以及 iframe 焦点下 Esc 只清当前 document；对应失败逐项保留并修复。补充单测还先证明重复 start 未接纳轮换 token/receipt、启动失败回调 401 未通知 GUI。
- GREEN（fake/应用装配）：`cd executor; python -m pytest tests/test_capture_session_manager.py tests/test_element_picker.py -q` → `45 passed, 1 skipped`；skip 仅为默认不弹出本机 headed Edge，核心生命周期测试无 skip。
- GREEN（真实浏览器）：`$env:RUN_HEADED_PICKER_INTEGRATION='1'; python -m pytest tests/test_element_picker.py::test_headed_edge_picker_on_local_static_html -q -rs` → `1 passed`。使用本机 `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`，覆盖真实点击拦截、operate、Alt、Esc、open Shadow DOM、同源 iframe、跨域 frame 主动退出、候选生成、身份证值遮罩且 value 不进入 payload、局部截图与删除。
- 回归：`cd executor; python -m pytest -q` → `100 passed, 1 skipped`；`python -m compileall -q app` 与 `git diff --check` 通过。

## 实现结果

- 平台客户端严格区分长期 executor token 与一次性 session token。首次 heartbeat 携带 start receipt 并由 Task 4 原子 ACK；后续清空 receipt。普通 mode/stop/expire 成功后才 ACK，网络失败保留会话/租约等待重投。
- 重复 start 同 session 不重复启动 browser，但会接纳租约重投时轮换的新 token/receipt；另一 session 明确拒绝。headless、非 `chrome`/`msedge` 和携带 URL userinfo 的命令在 launch 前拒绝。
- stop、expire、401、409、启动失败与 shutdown 会关闭 page/context/browser/Playwright，清空 token/receipt/候选并删除会话临时目录。普通 `TaskManager` 统计和容量不受采集会话影响。
- picker 不接收网页 selector；只从固定 allowlist 生成 `ElementSnapshot`，由 Task 5 生成 locator 后再通过当前可信 frame 复算 match count。截图绝不进入平台 payload，本地路径也不回传。
- FastAPI 使用 lifespan 统一启动/关闭 heartbeat 与唯一 poller；GUI 状态和认证失效均通过 `root.after(0, ...)` 回到 Tk 主线程。

## 聚焦测试名称

### `test_capture_session_manager.py`

- `test_single_capture_session_is_idempotent_for_same_start_and_rejects_another_session`
- `test_capture_only_launches_allowed_headed_channels[chrome/msedge]`
- `test_capture_rejects_arbitrary_channel_and_headless_before_launch`
- `test_startup_failure_rolls_back_all_initialized_resources`
- `test_every_terminal_reason_closes_page_context_browser_and_clears_credentials`
- `test_capture_session_does_not_change_normal_task_capacity`
- `test_start_receipt_is_sent_once_then_followup_heartbeat_omits_it`
- `test_mode_stop_and_expire_ack_only_after_local_operation_succeeds`
- `test_network_error_keeps_session_and_start_receipt_for_next_cycle`
- `test_released_start_redelivery_rotates_credentials_without_relaunching_browser`
- `test_unauthorized_and_conflict_heartbeat_close_local_session`
- `test_start_failure_callback_401_notifies_gui_auth_failure_without_leaking_resources`
- `test_command_poller_waits_exactly_two_seconds_and_closes_manager_on_shutdown`
- `test_platform_client_uses_long_token_for_claim_and_session_bearer_for_callbacks`
- `test_gui_capture_status_is_dispatched_to_tk_main_thread`
- `test_fastapi_lifecycle_starts_one_capture_poller_and_health_is_non_sensitive`
- `test_gui_registration_connects_thread_safe_capture_status_and_shared_auth_failure`

### `test_element_picker.py`

- `test_picker_installs_structured_binding_and_supports_mode_clear_and_dispose`
- `test_dom_binding_rejects_every_non_allowlisted_or_sensitive_field`
- `test_dom_binding_keeps_only_fixed_safe_attribute_allowlist`
- `test_sensitive_scan_rejects_candidate_before_task5_without_logging_raw_value`
- `test_second_sensitive_scan_rejects_compromised_task5_output`
- `test_match_counts_are_recomputed_through_trusted_frame_locators`
- `test_local_screenshot_masks_sensitive_inputs_and_is_deleted_after_report`
- `test_screenshot_failure_removes_mask_and_partial_temp_file`
- `test_recursive_sensitive_scanner_handles_keys_and_values_without_false_positive`
- `test_headed_edge_picker_on_local_static_html`

## 残余风险

- Task 5 最终复审账本仍为 `NOT CLEAN`。Task 6 的双扫描、网页 selector 拒绝和平台 payload 门禁阻止其残余问题直接泄露敏感数据，但动态 token 分类、指纹稳定性和定位器质量的根因仍属于 Task 5，不能由本任务宣称消除。
- 当前平台没有截图上传 API，截图仅在本地临时存在并在候选成功上报后删除；因此平台候选暂时没有缩略图预览。
- headed 集成已在本机 Edge 验证；Chrome channel 由同一白名单/API 路径和 fake 矩阵覆盖，但本机没有发现 Chrome 可执行文件，未做第二套真实浏览器运行。
