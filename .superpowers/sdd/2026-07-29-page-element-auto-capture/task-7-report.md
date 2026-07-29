# Task 7 首轮报告：前端采集服务与候选审核抽屉

## 实现前契约映射

### 平台 API 与前端方法

| 前端方法 | HTTP | 请求 | 成功数据 | 前端约束 |
| --- | --- | --- | --- | --- |
| `create(body, { signal })` | `POST /api/ui/page-elements/capture-sessions` | `pageId`、`executorId`、`browserChannel`、`mode`、`url` | `{ session, token }` | 只返回 `session`，丢弃一次性 `token` |
| `get(id, { signal })` | `GET /api/ui/page-elements/capture-sessions/:id` | 无 | 会话详情 | 不渲染 `currentUrl` 查询参数、`browserContextId` |
| `mode(id, mode, { signal })` | `PATCH .../:id/mode` | `{ mode }` | message | 仅发送 `pick/operate` |
| `stop(id, { signal })` | `POST .../:id/stop` | 无 | message | 停止不等于保存 |
| `candidates(id, { afterId, limit, signal })` | `GET .../:id/candidates` | query；默认 `afterId=0, limit=100` | 候选数组 | limit 限制到 1–200，AbortSignal 原样传递 |
| `update(id, candidateId, body, { signal })` | `PATCH .../:id/candidates/:candidateId` | 可编辑字段 | message | 名称/冲突决策乐观更新，失败回滚 |
| `save(id, items, { signal })` | `POST .../:id/save` | `{ items }` | `{ savedCandidateIds, ignoredCandidateIds }` | 每批 1–200；409 `data.issues` 保持结构化 |
| `versions(elementId, { signal })` | `GET /api/ui/page-elements/:id/versions` | 无 | 版本数组 | 本任务只提供服务，不接 Task 8 页面 |
| `rollback(elementId, version, { signal })` | `POST .../:id/versions/:version/rollback` | 无 | 版本详情 | 本任务只提供服务 |

真实候选 DTO 使用 `cursorId` 作为 UI/API 的 `candidateId`；`id` 是内部字符串业务主键。定位器为最多三条 `{ type, value, score, unique }`。冲突保存项为 `{ candidateId, resolution, targetElementId? }`。

### API 差异记录

- 当前 `httpClient` 只保留错误 message/status，无法读取 Controller 的 409 `{ data: { issues } }`；Task 7 将最小扩展为 `error.data = payload.data`，继续复用统一鉴权与 401 行为。
- 当前候选响应只暴露单个 `duplicateElementId`，后端批量预检却支持同指纹多目标。Drawer 会兼容未来的 `conflictTargetIds` / `duplicateElementIds`，并完整实现多目标选择与 `targetElementId` 保存协议；在现有 DTO 下只能呈现单个目标。此处不越界修改 Task 4 后端。
- 当前后端 `PATCH mode` 路由要求 `ui.element.manage`，与设计中采集权限的描述不同；前端按真实 API 调用并展示失败，不绕过权限。

## UI 状态机

### Launcher

`idle → submitting → started`；失败回到 `idle` 并保留执行器与 channel。只有 `status=online` 且 `supportedTypes` 含 `ui` 或 `browser` 的执行器可选。URL 在进入 UI 与提交时都收敛为 HTTP(S) `origin + pathname`，不保存/显示 query、fragment 或 credentials。

### Drawer

会话状态沿用后端 `starting → active ↔ interrupted → completed | expired | failed`。`starting/active/interrupted` 使用递归短轮询；正常间隔 2 秒，网络失败按 2/4/8/16/30 秒退避，保留已有候选；终态、停止、切换 session、关闭和卸载会 Abort 并停止调度。每轮携带 generation/session 检查，旧响应不可写入新会话。

候选以 `cursorId` 去重并升序合并。审核状态分离为：

- 服务端基线：候选名称、定位器、冲突状态；
- 乐观草稿：名称、`update/ignore/create` 与目标元素；
- 选择集：最多 200；
- 字段 issues：本地门禁、PATCH 失败或后端 409；
- dirty 集：名称/冲突/目标修改及未保存选择，用于关闭确认。

保存成功后移除后端返回的 saved/ignored 候选并调用 `onSaved`；409 聚焦第一条问题候选。停止会话只改变会话状态，不清除草稿或选择。

## 紧凑设计 token 自检

- 颜色：完全复用 `--panel/#fff`、`--bg`、`--text`、`--muted`、`--line`、`--line-strong`、`--primary`、`--primary-soft`、`--success`、`--warning`、`--danger`；不建立新主题。
- 字体：界面沿用 `--app-font`，locator/分数使用 `--code-font`；不引入字体依赖。
- 布局：Launcher 是窄启动对话框；Drawer 固定右侧，宽 `min(960px, 96vw)`，头部/质量轨道/独立滚动列表/粘性操作栏四段。
- 唯一签名：质量轨道使用一条横向连续分段按钮展示“全部 / 可保存 / 待命名 / 定位不可靠 / 冲突”，文字、数量、颜色三重表达并可直接过滤。
- 反模板自检：删除独立 KPI 卡片、装饰渐变和重复容器；候选使用带左侧质量刻度的紧凑审核行。所有新增选择器限定 `element-capture-*`。
- 可访问性：真实 button/input、`aria-pressed`/`aria-checked`、标题关联、focus trap、可见 `:focus-visible`、Escape、焦点返回和 reduced-motion；768px 以下转为全宽且操作栏保持可见。

## RED / GREEN

### RED

- 首次运行 `npm test -- --run src/services/elementCaptureService.test.js src/components/ElementCaptureLauncher.test.js src/components/ElementCaptureDrawer.test.js` 返回退出码 1。
- 三个 suite 均因对应 production module 尚不存在而失败：`elementCaptureService.js`、`ElementCaptureLauncher.js`、`ElementCaptureDrawer.js` 无法解析；0 项测试误通过。
- 测试先行定义了真实 API URL/method/body/Signal/409、Launcher 过滤和生命周期、Drawer 合并/旧响应/轮询/质量/冲突/保存/焦点/敏感字段契约。

### GREEN

- `elementCaptureService.test.js`：4 项通过。
- `ElementCaptureLauncher.test.js`：5 项通过。
- `ElementCaptureDrawer.test.js`：12 项通过，覆盖 cursor 去重、session generation、真实 timer 轮询/退避/终态、质量筛选、400/500、保存门禁、三种冲突与 target、PATCH 回滚、409 聚焦、保存成功、mode/stop、未保存确认、焦点和敏感字段隔离。
- 三文件聚焦：`3 passed / 21 tests passed`。
- 前端全量：`13 passed / 64 tests passed`。
- Vite production build：1613 modules transformed，退出码 0。
- `git diff --check`：退出码 0，仅提示工作区 LF 将按 Git 配置转为 CRLF。

## 残余风险

- Task 4 当前候选响应只提供单个 `duplicateElementId`，没有把锁后 `ExistingFingerprints` 多目标集合暴露给前端。Drawer 已兼容 `conflictTargetIds` / `duplicateElementIds` 并实现完整多目标交互，但真实环境在后端 DTO 扩展前无法让用户看到多个可选目标；后端仍会以 409 `targetElementId` issue 阻止不明确更新。
- Task 4 当前模式切换路由要求 `ui.element.manage`，与设计中 `ui.element.capture` 可操作模式的权限描述不一致。前端遵循真实 API，权限不足会回滚 UI 并显示错误。
- 本任务按边界未集成 `UIAutomationPage`；实际页面入口、组件联动和浏览器端 E2E 属于 Task 8。
- Vite build 保留仓库既有的单 chunk 超过 500 kB 提示；本任务没有引入新依赖或改动打包策略。
