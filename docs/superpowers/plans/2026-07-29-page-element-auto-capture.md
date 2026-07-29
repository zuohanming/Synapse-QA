# 页面元素自动采集实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有页面元素配置中实现由执行器驱动的有头 Chromium 元素拾取、候选审核、原子入库、版本历史和回滚。

**Architecture:** 平台后端持久化采集会话、候选和元素版本，并通过执行器回调接收候选；执行器为采集会话创建独立 Playwright 浏览器上下文，注入拾取工具条并生成定位器；React 页面负责启动会话、轮询状态、审核候选和批量保存。第一期使用短轮询而非新增 WebSocket 基础设施，候选自增 ID 作为增量游标。

**Tech Stack:** Go 1.x、Gin、PostgreSQL、Python 3、FastAPI、Playwright、React 18、Vitest。

## Global Constraints

- 第一阶段只支持有头 Chromium，Chrome 与 Edge 使用 Playwright channel。
- 一次会话只绑定一个页面对象、一个执行器、一个浏览器上下文及创建用户。
- 同一执行器最多一个活动采集会话。
- 会话 30 分钟无操作失效，执行器断连只允许原机在 60 秒内恢复。
- 单会话最多 500 个候选，达到 400 个告警；一次批量保存最多 200 个。
- 候选必须审核后入库；无可靠唯一定位器、未命名元素和未处理冲突禁止保存。
- 批量保存必须全量预检并在单个 PostgreSQL 事务中完成。
- 不持久化 Cookie、Local Storage、输入值或候选截图；缩略图到期清理。
- 后端代码变更后自动重启。
- 每个代码、配置或文档变更同步更新 `docs/daily-updates/2026-07-29.md`；用户可见变化同步更新 `CHANGELOG.md`。

---

## 文件结构

### 后端新增文件

- `backend/internal/model/element_capture.go`：会话、候选、版本、批量保存 DTO。
- `backend/internal/repository/element_capture_repository.go`：采集领域事务与查询。
- `backend/internal/repository/element_capture_repository_test.go`：SQL 事务和状态变更测试。
- `backend/internal/service/element_capture_service.go`：会话状态机、令牌、候选门禁和业务规则。
- `backend/internal/service/element_capture_service_test.go`：状态、容量、权限外业务规则测试。
- `backend/internal/controller/element_capture_controller.go`：平台 API 与执行器回调入口。
- `backend/internal/controller/element_capture_controller_test.go`：HTTP 参数和响应测试。

### 执行器新增文件

- `executor/app/models/capture.py`：采集请求、状态和候选模型。
- `executor/app/services/capture_session_manager.py`：单会话互斥、浏览器生命周期和模式切换。
- `executor/app/services/element_picker.py`：注入拾取脚本、iframe/Shadow DOM 命中和截图。
- `executor/app/services/locator_generator.py`：名称、定位器、唯一性、指纹和评分。
- `executor/tests/test_capture_session_manager.py`
- `executor/tests/test_locator_generator.py`

### 前端新增文件

- `frontend/src/components/ElementCaptureLauncher.js`：启动采集弹窗。
- `frontend/src/components/ElementCaptureDrawer.js`：候选审核抽屉。
- `frontend/src/components/ElementCaptureDrawer.test.js`
- `frontend/src/services/elementCaptureService.js`
- `frontend/src/services/elementCaptureService.test.js`

### 修改文件

- `backend/cmd/api/bootstrap.go`
- `backend/cmd/api/api_migration.go`
- `backend/cmd/api/main.go`
- `backend/internal/model/ui.go`
- `backend/internal/repository/automation_repository.go`
- `backend/internal/router/router.go`
- `backend/internal/service/automation_service.go`
- `executor/app/api/routes.py`
- `executor/app/core/config.py`
- `executor/gui.py`
- `frontend/src/pages/UIAutomationPage.js`
- `frontend/src/pages/UIAutomationPage.test.js`
- `frontend/src/styles/appearance.css`
- `docs/PRD.md`
- `docs/api-page-element-capture.md`
- `docs/daily-updates/2026-07-29.md`
- `CHANGELOG.md`

---

### Task 1: 数据模型、迁移与权限目录

**Files:**
- Create: `backend/internal/model/element_capture.go`
- Modify: `backend/cmd/api/bootstrap.go`
- Modify: `backend/cmd/api/api_migration.go`
- Modify: `backend/internal/model/ui.go`
- Test: `backend/cmd/api/api_migration_test.go`

**Interfaces:**
- Produces: `ElementCaptureSession`、`ElementCaptureCandidate`、`PageElementVersion`、`CaptureSessionCreateRequest`、`CandidateBatchSaveRequest`。
- Produces permissions: `ui.element.read`、`ui.element.capture`、`ui.element.manage`。

- [ ] **Step 1: 写权限目录失败测试**

```go
func TestPermissionSeedsContainElementCapturePermissions(t *testing.T) {
	codes := permissionSeedCodes()
	for _, code := range []string{"ui.element.read", "ui.element.capture", "ui.element.manage"} {
		if !slices.Contains(codes, code) {
			t.Fatalf("missing permission %s", code)
		}
	}
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend; go test ./cmd/api -run TestPermissionSeedsContainElementCapturePermissions -count=1`

Expected: FAIL，提示缺少采集权限。

- [ ] **Step 3: 定义模型和数据库结构**

```go
type ElementCaptureSession struct {
	ID string `json:"id"`
	PageID int64 `json:"pageId"`
	ExecutorID string `json:"executorId"`
	Status string `json:"status"`
	Mode string `json:"mode"`
	CurrentURL string `json:"currentUrl"`
	CandidateCount int `json:"candidateCount"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type CandidateBatchSaveRequest struct {
	SessionID string `json:"sessionId"`
	Items []CandidateSaveItem `json:"items"`
}
```

在 `bootstrap.go` 增加：

- `element_capture_sessions`
- `element_capture_candidates`
- `page_element_versions`
- `page_elements` 的 fingerprint、capture_source、capture_url、tag_name、accessible_name、quality_score、captured_by、captured_at、last_verified_at、verification_status、current_version 字段
- 活动会话执行器唯一索引
- 会话、候选到期查询索引

- [ ] **Step 4: 增加权限种子并运行迁移测试**

Run: `cd backend; go test ./cmd/api -run TestPermissionSeedsContainElementCapturePermissions -count=1`

Expected: PASS。

- [ ] **Step 5: 运行后端全量测试**

Run: `cd backend; go test ./...`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add backend/cmd/api/bootstrap.go backend/cmd/api/api_migration.go backend/cmd/api/api_migration_test.go backend/internal/model/element_capture.go backend/internal/model/ui.go
git commit -m "feat: add element capture data model"
```

---

### Task 2: 会话状态机与一次性令牌

**Files:**
- Create: `backend/internal/repository/element_capture_repository.go`
- Create: `backend/internal/repository/element_capture_repository_test.go`
- Create: `backend/internal/service/element_capture_service.go`
- Create: `backend/internal/service/element_capture_service_test.go`
- Modify: `backend/cmd/api/main.go`

**Interfaces:**
- Consumes: Task 1 模型和数据表。
- Produces:
  - `CreateSession(ctx, actor, req) (ElementCaptureSession, error)`
  - `GetSession(ctx, userID, sessionID) (ElementCaptureSessionDetail, error)`
  - `SetMode(ctx, actor, sessionID, mode) error`
  - `StopSession(ctx, actor, sessionID) error`
  - `Heartbeat(ctx, sessionID, executorID, token, currentURL) error`
  - `ExpireSessions(ctx, now) error`

- [ ] **Step 1: 写会话状态失败测试**

```go
func TestCreateSessionRejectsBusyExecutor(t *testing.T) {
	repo := &fakeCaptureRepo{activeExecutor: "exec-1"}
	service := NewElementCaptureService(repo, fakeExecutorReader{online: true}, []byte("secret"))
	_, err := service.CreateSession(context.Background(), "admin", model.CaptureSessionCreateRequest{
		PageID: 8, ExecutorID: "exec-1", URL: "https://example.test", BrowserChannel: "chrome",
	})
	if err == nil || err.Error() != "执行器正在采集页面元素" {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

同时覆盖：离线执行器、非 UI 执行器、非法 URL、模式仅允许 `pick/operate`、30 分钟到期、60 秒中断恢复、错误执行器不能恢复。

- [ ] **Step 2: 运行服务测试并确认失败**

Run: `cd backend; go test ./internal/service -run ElementCapture -count=1`

Expected: FAIL，提示服务或方法未定义。

- [ ] **Step 3: 实现 Repository 原子状态更新**

使用条件更新：

```sql
update element_capture_sessions
set status='active', last_heartbeat_at=now(), current_url=$3, updated_at=now()
where id=$1 and executor_id=$2 and status in ('starting','active','interrupted')
returning id
```

令牌只保存 SHA-256 摘要，明文仅在创建响应中返回一次。

- [ ] **Step 4: 实现最小状态机**

```go
const (
	CaptureStarting = "starting"
	CaptureActive = "active"
	CaptureInterrupted = "interrupted"
	CaptureCompleted = "completed"
	CaptureExpired = "expired"
	CaptureFailed = "failed"
)
```

启动时校验执行器状态和 UI 能力；活动会话冲突转换为用户可读错误；停止和过期都生成待执行器领取的关闭指令。

- [ ] **Step 5: 运行服务与 Repository 测试**

Run: `cd backend; go test ./internal/service ./internal/repository -run ElementCapture -count=1`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add backend/internal/repository/element_capture_repository.go backend/internal/repository/element_capture_repository_test.go backend/internal/service/element_capture_service.go backend/internal/service/element_capture_service_test.go backend/cmd/api/main.go
git commit -m "feat: add element capture session state machine"
```

---

### Task 3: 候选接收、质量门禁与原子入库

**Files:**
- Modify: `backend/internal/repository/element_capture_repository.go`
- Modify: `backend/internal/service/element_capture_service.go`
- Modify: `backend/internal/model/element_capture.go`
- Test: `backend/internal/service/element_capture_service_test.go`
- Test: `backend/internal/repository/element_capture_repository_test.go`

**Interfaces:**
- Produces:
  - `AddCandidate(ctx, executorID, token, req) (ElementCaptureCandidate, error)`
  - `ListCandidates(ctx, userID, sessionID, afterID, limit) ([]ElementCaptureCandidate, error)`
  - `UpdateCandidate(ctx, actor, sessionID, candidateID, req) error`
  - `BatchSave(ctx, actor, req) (BatchSaveResult, error)`

- [ ] **Step 1: 写质量和容量失败测试**

```go
func TestBatchSaveRejectsUnreliableCandidate(t *testing.T) {
	service := newCaptureServiceWithCandidates(candidateFixture{
		Name: "未命名元素", Locators: nil, QualityScore: 20,
	})
	_, err := service.BatchSave(context.Background(), "admin", model.CandidateBatchSaveRequest{
		SessionID: "session-1", Items: []model.CandidateSaveItem{{CandidateID: 1, Resolution: "create"}},
	})
	if err == nil || !strings.Contains(err.Error(), "候选项 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

覆盖：501 个候选拒绝、201 个保存项拒绝、重名、非唯一定位器、未处理冲突、跨页面候选、无管理权限提交。

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend; go test ./internal/service -run 'Candidate|BatchSave' -count=1`

Expected: FAIL。

- [ ] **Step 3: 实现元素指纹重复匹配**

指纹字段由执行器提供，后端只接受 64 位小写十六进制 SHA-256；重复查询限定 `page_id` 且排除软删除元素。候选保存时返回 `duplicateElementId` 和 `conflictStatus`。

- [ ] **Step 4: 实现全量预检**

```go
func validateCandidateForSave(candidate model.ElementCaptureCandidate, item model.CandidateSaveItem) []model.FieldIssue
```

返回候选 ID、字段和中文原因；存在任意问题时 Repository 事务不得开始。

- [ ] **Step 5: 实现单事务新增、更新和版本写入**

事务顺序：

1. `select ... for update` 锁定会话和所有候选；
2. 写入或更新 `page_elements`；
3. 为每个正式元素写入 `page_element_versions`；
4. 标记候选 `saved/ignored`；
5. 写入一条批量操作审计；
6. commit。

- [ ] **Step 6: 模拟版本写入失败并验证整批回滚**

Run: `cd backend; go test ./internal/repository -run 'BatchSave.*Rollback' -count=1`

Expected: PASS，元素表和候选状态都未部分更新。

- [ ] **Step 7: 提交**

```bash
git add backend/internal/model/element_capture.go backend/internal/repository/element_capture_repository.go backend/internal/repository/element_capture_repository_test.go backend/internal/service/element_capture_service.go backend/internal/service/element_capture_service_test.go
git commit -m "feat: add captured element review transaction"
```

---

### Task 4: 平台 HTTP API、权限和版本回滚

**Files:**
- Create: `backend/internal/controller/element_capture_controller.go`
- Create: `backend/internal/controller/element_capture_controller_test.go`
- Modify: `backend/internal/router/router.go`
- Modify: `backend/internal/service/element_capture_service.go`
- Modify: `backend/internal/repository/element_capture_repository.go`

**Interfaces:**
- Platform API:
  - `POST /api/ui/page-elements/capture-sessions`
  - `GET /api/ui/page-elements/capture-sessions/:id`
  - `PATCH /api/ui/page-elements/capture-sessions/:id/mode`
  - `POST /api/ui/page-elements/capture-sessions/:id/stop`
  - `GET /api/ui/page-elements/capture-sessions/:id/candidates`
  - `PATCH /api/ui/page-elements/capture-sessions/:id/candidates/:candidateId`
  - `POST /api/ui/page-elements/capture-sessions/:id/save`
  - `GET /api/ui/page-elements/:id/versions`
  - `POST /api/ui/page-elements/:id/versions/:version/rollback`
- Executor API:
  - `POST /api/executor/element-capture/:id/heartbeat`
  - `POST /api/executor/element-capture/:id/candidates`
  - `POST /api/executor/element-capture/:id/fail`
  - `GET /api/executor/element-capture/commands?executorId=...`

- [ ] **Step 1: 写权限与 HTTP 失败测试**

```go
func TestCaptureSessionRequiresCapturePermission(t *testing.T) {
	router := captureTestRouter(model.Claims{Permissions: []string{"ui.element.read"}})
	response := performJSON(router, http.MethodPost, "/api/ui/page-elements/capture-sessions", validSessionBody)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}
```

覆盖采集、查看、入库、回滚四类权限和执行器错误令牌返回 401。

- [ ] **Step 2: 运行 Controller 测试并确认失败**

Run: `cd backend; go test ./internal/controller -run ElementCapture -count=1`

Expected: FAIL。

- [ ] **Step 3: 实现 Controller 和路由**

平台接口使用现有 JWT claims；执行器接口使用会话一次性令牌与执行器 ID 双重校验。候选缩略图接口增加 `Cache-Control: no-store`。

- [ ] **Step 4: 实现版本读取和回滚**

回滚不复用旧版本号；将目标快照写为新的 `current_version + 1`，`source=rollback`，同时更新正式元素。

- [ ] **Step 5: 运行 Controller 和后端全量测试**

Run: `cd backend; go test ./...`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add backend/internal/controller/element_capture_controller.go backend/internal/controller/element_capture_controller_test.go backend/internal/router/router.go backend/internal/service/element_capture_service.go backend/internal/repository/element_capture_repository.go
git commit -m "feat: expose element capture and version APIs"
```

---

### Task 5: 执行器定位器生成器

**Files:**
- Create: `executor/app/models/capture.py`
- Create: `executor/app/services/locator_generator.py`
- Create: `executor/tests/test_locator_generator.py`

**Interfaces:**
- Produces:

```python
def build_candidate(snapshot: ElementSnapshot) -> CaptureCandidate: ...
def is_dynamic_token(value: str) -> bool: ...
def score_locator(strategy: str, unique: bool, depth: int) -> int: ...
```

- [ ] **Step 1: 写定位器优先级失败测试**

```python
def test_prefers_test_id_over_id_and_css():
    snapshot = ElementSnapshot(
        tag="button",
        attributes={"data-testid": "save-order", "id": "btn_981273"},
        accessible_name="保存",
        visible_text="保存",
    )
    candidate = build_candidate(snapshot)
    assert candidate.locators[0].type == "testid"
    assert candidate.locators[0].value == "save-order"
```

覆盖 label 命名、role、稳定 CSS、文本、XPath、动态 ID/class 过滤、敏感值过滤、最多三组、唯一性失败降分和 SHA-256 指纹稳定。

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd executor; python -m pytest tests/test_locator_generator.py -q`

Expected: FAIL，模块不存在。

- [ ] **Step 3: 实现纯函数生成器**

生成器不直接访问 Playwright；浏览器层提供每个候选定位器的匹配数。评分基线：testid 95、静态 id 90、role 85、label 82、稳定 CSS 70、文本 60、XPath 40；不唯一时减 50，包含动态 token 直接丢弃。

- [ ] **Step 4: 运行测试**

Run: `cd executor; python -m pytest tests/test_locator_generator.py -q`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add executor/app/models/capture.py executor/app/services/locator_generator.py executor/tests/test_locator_generator.py
git commit -m "feat: generate stable element locators"
```

---

### Task 6: 执行器有头浏览器拾取会话

**Files:**
- Create: `executor/app/services/capture_session_manager.py`
- Create: `executor/app/services/element_picker.py`
- Create: `executor/tests/test_capture_session_manager.py`
- Modify: `executor/app/api/routes.py`
- Modify: `executor/app/core/config.py`
- Modify: `executor/gui.py`

**Interfaces:**
- Consumes: Task 5 `build_candidate`。
- Produces:

```python
class CaptureSessionManager:
    async def start(self, command: CaptureStartCommand) -> CaptureState: ...
    async def set_mode(self, session_id: str, mode: CaptureMode) -> None: ...
    async def stop(self, session_id: str, reason: str) -> None: ...
    async def heartbeat(self) -> None: ...
```

- [ ] **Step 1: 写单会话互斥和清理失败测试**

```python
@pytest.mark.asyncio
async def test_rejects_second_capture_session():
    manager = CaptureSessionManager(browser_factory=fake_browser_factory)
    await manager.start(command("session-1"))
    with pytest.raises(ValueError, match="已有页面元素采集会话"):
        await manager.start(command("session-2"))
```

覆盖 stop 关闭 context、令牌失效关闭、普通测试任务不被计入采集互斥、Chrome/Edge channel、无头模式被拒绝。

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd executor; python -m pytest tests/test_capture_session_manager.py -q`

Expected: FAIL。

- [ ] **Step 3: 实现工具条和拾取脚本**

脚本必须：

- 使用 Shadow Root 隔离工具条样式；
- `pick` 模式阻止 click 默认行为和冒泡；
- `operate` 模式不拦截页面；
- Alt 临时反转模式，Esc 清除高亮；
- 递归处理 open Shadow DOM；
- 同源 iframe 注入相同脚本；
- 跨域 iframe 显示“不支持跨域 iframe”；
- 只把 DOM 特征传回 Python，不传输入框 value。

- [ ] **Step 4: 实现局部截图与敏感遮罩**

截图前在密码、tel、包含身份证模式的输入框上覆盖纯色遮罩；截图以临时文件保存并在上传成功、停止或过期时删除。

- [ ] **Step 5: 接入执行器命令轮询和 GUI 状态**

执行器连接平台后每 2 秒领取一次采集命令；活动时 GUI 状态显示“正在采集 · 页面名称”，停止后恢复在线。

- [ ] **Step 6: 运行执行器全量测试**

Run: `cd executor; python -m pytest -q`

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add executor/app/models/capture.py executor/app/services/capture_session_manager.py executor/app/services/element_picker.py executor/app/api/routes.py executor/app/core/config.py executor/gui.py executor/tests/test_capture_session_manager.py
git commit -m "feat: add headed browser element picker"
```

---

### Task 7: 前端采集服务与候选审核组件

**Files:**
- Create: `frontend/src/services/elementCaptureService.js`
- Create: `frontend/src/services/elementCaptureService.test.js`
- Create: `frontend/src/components/ElementCaptureLauncher.js`
- Create: `frontend/src/components/ElementCaptureDrawer.js`
- Create: `frontend/src/components/ElementCaptureDrawer.test.js`
- Modify: `frontend/src/styles/appearance.css`

**Interfaces:**
- Consumes: Task 4 平台 API。
- Produces:
  - `ElementCaptureLauncher({ pageRow, executors, onStarted, onClose })`
  - `ElementCaptureDrawer({ session, onClose, onSaved })`

- [ ] **Step 1: 写服务请求失败测试**

```javascript
it("按游标增量查询候选", async () => {
  fetch.mockResolvedValue(jsonResponse({ data: [] }));
  await elementCaptureService.candidates("session-1", { afterId: 12, limit: 100 });
  expect(fetch).toHaveBeenCalledWith(
    expect.stringContaining("/capture-sessions/session-1/candidates?afterId=12&limit=100"),
    expect.any(Object)
  );
});
```

- [ ] **Step 2: 写审核门禁失败测试**

```javascript
it("存在未处理冲突时禁用批量保存", async () => {
  render(<ElementCaptureDrawer session={session} />);
  expect(await screen.findByText("发现重复元素")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存选中元素" })).toBeDisabled();
});
```

覆盖未命名、非唯一定位器、200 上限、冲突三种决策、编辑名称、模式切换、停止和断连提示。

- [ ] **Step 3: 运行测试并确认失败**

Run: `cd frontend; npm test -- --run src/services/elementCaptureService.test.js src/components/ElementCaptureDrawer.test.js`

Expected: FAIL。

- [ ] **Step 4: 实现服务和启动弹窗**

执行器列表只展示在线、支持 UI 且非采集中的项。URL 使用 `new URL()` 校验，仅允许 `http:` 与 `https:`。浏览器通道只提供 Chrome、Edge。

- [ ] **Step 5: 实现审核抽屉**

抽屉包含会话状态条、倒计时、模式切换、质量筛选、虚拟滚动候选表、定位器编辑、冲突决策和批量保存栏。缩略图使用 `loading="lazy"`，错误时显示文字占位。

- [ ] **Step 6: 运行组件测试和构建**

Run: `cd frontend; npm test -- --run src/services/elementCaptureService.test.js src/components/ElementCaptureDrawer.test.js; npm run build`

Expected: PASS；构建退出码 0。

- [ ] **Step 7: 提交**

```bash
git add frontend/src/services/elementCaptureService.js frontend/src/services/elementCaptureService.test.js frontend/src/components/ElementCaptureLauncher.js frontend/src/components/ElementCaptureDrawer.js frontend/src/components/ElementCaptureDrawer.test.js frontend/src/styles/appearance.css
git commit -m "feat: add element capture review drawer"
```

---

### Task 8: 页面元素工作台集成与版本界面

**Files:**
- Modify: `frontend/src/pages/UIAutomationPage.js`
- Modify: `frontend/src/pages/UIAutomationPage.test.js`
- Modify: `frontend/src/styles/appearance.css`
- Modify: `frontend/src/services/elementCaptureService.js`

**Interfaces:**
- Consumes: Task 7 组件。
- Produces: 页面元素配置中的“自动采集”入口、未处理离开保护、采集元数据区和版本回滚界面。

- [ ] **Step 1: 写页面集成失败测试**

```javascript
it("从当前页面对象启动自动采集并打开审核抽屉", async () => {
  renderPageElementPanel(pageFixture);
  fireEvent.click(screen.getByRole("button", { name: "自动采集" }));
  expect(screen.getByText(`采集页面：${pageFixture.name}`)).toBeInTheDocument();
});
```

覆盖手工新增仍可用、离开未处理候选确认、版本列表和回滚确认。

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd frontend; npm test -- --run src/pages/UIAutomationPage.test.js`

Expected: FAIL。

- [ ] **Step 3: 集成启动与审核组件**

`PageElementPanel` 只持有 `captureSession` 和弹窗开关；候选状态留在 `ElementCaptureDrawer`，避免继续扩大页面文件内的状态耦合。

- [ ] **Step 4: 增加采集元数据与版本抽屉**

元素详情展示来源、URL、标签、可访问名称、评分和最近验证。回滚按钮调用版本 API，成功后刷新元素列表和版本列表。

- [ ] **Step 5: 运行前端全量测试与构建**

Run: `cd frontend; npm test -- --run; npm run build`

Expected: 所有测试通过；构建退出码 0。

- [ ] **Step 6: 提交**

```bash
git add frontend/src/pages/UIAutomationPage.js frontend/src/pages/UIAutomationPage.test.js frontend/src/styles/appearance.css frontend/src/services/elementCaptureService.js
git commit -m "feat: integrate automatic element capture workspace"
```

---

### Task 9: 清理任务、文档与端到端验收

**Files:**
- Modify: `backend/cmd/api/main.go`
- Modify: `docs/PRD.md`
- Create: `docs/api-page-element-capture.md`
- Modify: `docs/daily-updates/2026-07-29.md`
- Modify: `CHANGELOG.md`
- Test: `frontend/e2e/element-capture.spec.js`（若目录不存在则创建）

**Interfaces:**
- Consumes: Tasks 1–8 全部能力。
- Produces: 到期清理循环、接口说明和真实采集验收证据。

- [ ] **Step 1: 写到期清理失败测试**

```go
func TestExpireSessionsDeletesTemporaryScreenshots(t *testing.T) {
	repo := newCaptureRepoWithExpiredSession("session-1", []string{"candidate-1.png"})
	cleaner := NewCaptureCleaner(repo, fakeFileRemover{})
	if err := cleaner.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	assertRemoved(t, "candidate-1.png")
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend; go test ./internal/service -run CaptureCleaner -count=1`

Expected: FAIL。

- [ ] **Step 3: 实现每分钟清理循环**

应用启动时创建可取消 goroutine，每分钟执行一次：过期会话状态更新、候选缩略图删除、已终止会话令牌摘要失效。应用关闭时停止 ticker。

- [ ] **Step 4: 编写接口和用户文档**

`docs/api-page-element-capture.md` 必须列出全部平台/执行器接口、权限、状态机、错误码、请求响应示例和安全限制；`docs/PRD.md` 更新页面元素章节。

- [ ] **Step 5: 自动重启后端并验证健康**

Run:

```powershell
$api=Get-Process synapse-api -ErrorAction SilentlyContinue | Select-Object -First 1
if($api){Stop-Process -Id $api.Id -Force}
Start-Process powershell -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File','F:\Synapse QA\backend\start-api.ps1' -WorkingDirectory 'F:\Synapse QA\backend' -WindowStyle Hidden
```

轮询 `http://127.0.0.1:8080/api/health`，Expected: `ok`。

- [ ] **Step 6: 执行全量自动化验证**

Run:

```powershell
Set-Location 'F:\Synapse QA\backend'; go test ./...
Set-Location 'F:\Synapse QA\executor'; python -m pytest -q
Set-Location 'F:\Synapse QA\frontend'; npm test -- --run; npm run build
```

Expected: 所有命令退出码 0。

- [ ] **Step 7: 执行真实 Chromium 端到端验收**

验收脚本使用本地固定测试页，覆盖：

1. 启动采集；
2. 切换操作/拾取模式；
3. 拾取普通按钮、同源 iframe 元素和 open Shadow DOM 元素；
4. 生成候选、编辑名称并解决一个重复冲突；
5. 批量保存；
6. 查看版本并回滚；
7. 确认临时截图清理；
8. 确认操作日志包含启动、保存和回滚。

- [ ] **Step 8: 更新每日记录和变更日志**

记录变更内容、影响范围、每条验证命令及真实 E2E 结果；`CHANGELOG.md` 的 `Unreleased` 增加页面元素自动采集功能。

- [ ] **Step 9: 最终提交**

```bash
git add backend/cmd/api/main.go frontend/e2e/element-capture.spec.js docs/PRD.md docs/api-page-element-capture.md docs/daily-updates/2026-07-29.md CHANGELOG.md
git commit -m "test: verify automatic page element capture"
```

---

## 自检结果

- 规格覆盖：会话、权限、浏览器边界、双模式、定位器、命名、候选、截图、容量、重复、事务、版本、错误恢复、前端交互和 E2E 均有对应任务。
- 依赖顺序：数据模型 → 状态机 → 候选事务 → HTTP API → 定位器 → 浏览器拾取 → 前端组件 → 页面集成 → 清理与 E2E。
- 类型一致：平台统一使用 `sessionID`、`candidateID`、`resolution`；执行器统一使用 `CaptureSessionManager` 和 `CaptureCandidate`。
- 范围控制：第一期不包含全 DOM 扫描、完整录制、Firefox/WebKit、跨执行器恢复、跨域 iframe、closed Shadow DOM、Canvas 和远程视频流。
