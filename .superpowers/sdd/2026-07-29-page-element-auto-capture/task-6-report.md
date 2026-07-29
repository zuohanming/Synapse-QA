# Task 6 交接报告：执行器有头浏览器元素拾取最终安全收口

## 完成范围

本轮按最终复审意见统一收口执行器 picker、平台回调、后端启动 ACK、候选幂等、SSRF、GUI/FastAPI 生命周期和测试报告，不扩展 Task 6 之外的产品功能。

### 页面绑定与 picker

- 每次会话由 Python 生成 256-bit 随机 nonce；nonce 只进入闭包控制器，不写入页面全局对象。页面侧无法取得 picker controller，也不能提交 selector。
- binding 同时校验 nonce、当前 page、frame、capture origin、元素 owner frame，并从 `ElementHandle` 重新提取 DOM 快照与网页 payload 比对。
- pick 使用单飞锁、每秒速率限制和待上报容量限制；工具条排除基于 `composedPath()`，每个同源 frame 只安装一个控制器，模式、Alt、Esc 与 dispose 向全部同源 frame 广播，跨域 frame 不安装控制器。
- 截图先滚动元素并读取 `bounding_box()`，再用 page screenshot 的 `clip` 与 Playwright `mask` 完成；不向页面 DOM 插入遮罩。password、tel、身份标识输入框会遮罩；Chromium 无法稳定投影子 frame mask 时保守遮罩 iframe 视口。截图串行化，异常和删除失败不会覆盖主错误，残留文件进入后续清理重试。

### 脱敏与网络边界

- Task 5/6 共用 `capture_security.py`：URL 对外只保留 origin + path，移除 userinfo、query 和 fragment；递归检测 camelCase 敏感键、路径内嵌 token/JWT、常见 secret 与高熵值。
- DOM 文本使用可见 `innerText`；平台 payload 在模型边界再次强制 URL 脱敏和敏感扫描。
- 导航 URL 及 BrowserContext 的每个请求都重新解析 DNS 并校验；默认拒绝 loopback、private、link-local、metadata 地址，只有显式 origin 与 private-host 双白名单才能用于本地 headed 测试。顶层跨 origin 重定向被拦截。
- 平台 HTTP 客户端不跟随重定向，不会把 executor/session 凭据带到重定向目标；响应体上限为 1 MiB，HTTP error response 会关闭。

### 启动 ACK 与候选幂等

- 已 ACK 的 start 只接受普通 heartbeat 或相同 receipt 的重试；receipt hash 保留到命令保留期结束。重试在同一 token/context/session 状态下返回成功，不轮换 token。
- 候选请求新增必填 UUID `clientCaptureId`。数据库列、历史回填和 `(session_id, client_capture_id)` 唯一索引已补齐；相同 client ID 重试返回原候选，既不重复插入，也不重复增加候选计数，即使会话已达到容量上限也可安全重试。
- 事务路径在真实 PostgreSQL 中验证 ACK hash 保留、错误 receipt/过期租约/错误状态拒绝、回滚与断连语义。

### 生命周期与可观测性

- 会话终止先清空 token/receipt，再以嵌套 `finally` 尽力执行 picker dispose、page、context、browser lease 和临时文件清理；任一层异常不阻断下一层。
- poller 的 start/stop 受锁保护并等待线程确认退出；FastAPI lifespan 停止两个后台客户端后 `await` manager close。GUI worker 只向队列投递事件，Tk 更新均由主线程消费；全局 401 终止活动采集并进入统一认证失效流程。
- `/health` 仅返回 capture active/mode，不暴露 URL、页面标题、token 或 receipt。
- Windows 临时文件安全依赖操作系统临时目录继承的当前用户 ACL；实现与测试明确不把 POSIX `chmod` 描述为 Windows DACL 配置。

## TDD 与验证记录

- RED：共享敏感扫描模块不存在时，新增反例测试失败；候选表缺少 `client_capture_id` 时，迁移测试失败。
- 执行器全量：`cd executor; python -m pytest -q` → `131 passed, 2 skipped`。
- 执行器静态编译：`cd executor; python -m compileall -q app gui.py` → 通过。
- 后端全量：`cd backend; go test ./... -count=1` → 全部通过。
- 后端静态检查：`cd backend; go vet ./...` → 通过。
- 真实 Edge picker：`RUN_HEADED_PICKER_INTEGRATION=1` 运行 `test_headed_edge_picker_on_local_static_html` → `1 passed`。覆盖工具条、operate/Alt/Esc、Shadow DOM、同源/跨域 iframe、屏幕外元素、真实遮罩像素、无 DOM overlay、并发截图锁、删除失败后页面存活和 dispose。
- 真实 Edge manager：运行 `test_headed_manager_uses_explicit_local_allowlist_and_blocks_metadata` → `1 passed`。覆盖显式本地 allowlist、URL 脱敏和 metadata 请求阻断。
- 真实 PostgreSQL：`TestHeartbeatAndAckStartPostgreSQLLeaseAndExpiry`、`TestRepositoryStateAuthorizationAndCleanup`、`TestCandidateRetryIsIdempotent` 和 `TestElementCaptureMigrationUpgradesAndIsIdempotentOnPostgreSQL` 全部通过；临时实例已停止并清理。
- 工作区检查：`git diff --check` → 通过。

默认跳过的两个测试均为显式 opt-in 的本机 headed Edge 集成测试；它们已在本轮单独启用并通过。

## 残余边界

- 平台当前没有截图上传 API，截图仅短暂存在于执行器本地并在上报成功或会话结束时删除。
- Chrome 与 Edge 共用同一 Playwright channel 白名单和实现路径；本轮真实浏览器验证使用本机 installed Edge，Chrome 由参数矩阵和 fake Playwright 回归覆盖。
- Windows 没有额外创建自定义 DACL；部署安全依赖系统临时目录的当前用户 ACL。如未来允许自定义共享临时目录，应另行增加 Windows ACL 配置与验证。
