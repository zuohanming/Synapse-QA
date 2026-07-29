# Task 4 交接报告：平台 HTTP API、权限和版本回滚

## 状态

已完成并提交：`feat: expose element capture and version APIs`。

## 实现摘要

- 新增 `ElementCaptureController`，为平台提供会话创建、查询、模式切换、停止、候选分页与审核、批量保存、版本读取和版本回滚接口；使用现有 JWT claims 与 `ui.element.capture/read/manage/rollback` 权限中间件。
- 新增执行器心跳、候选接收、失败回调和命令查询路由。心跳、候选和失败回调将执行器 ID 与一次性会话令牌传入服务；无效令牌映射为 401。
- 候选列表、候选更新、批量保存和执行器候选接收响应写入 `Cache-Control: no-store`。参数错误、资源不存在、状态冲突及 `CandidateIssuesError` 均沿用项目 `APIResponse` JSON 结构。
- 新增版本查询与回滚仓储逻辑。回滚在一个事务中锁定正式元素，读取目标快照，更新正式元素元数据，生成 `current_version + 1` 的新版本并写入回滚审计；新快照标记 `source=rollback`，不复用旧版本号。

## TDD 与验证

- RED：`cd backend; go test ./internal/controller -run ElementCapture -count=1` 初始因 `ElementCaptureCommand` 与 `NewElementCaptureController` 未定义而构建失败。
- GREEN：同一 controller 聚焦命令通过，覆盖采集权限、查看权限、回滚权限、候选 no-store、无效执行器令牌和 `CandidateIssuesError` 统一冲突响应。
- 回滚 RED：`cd backend; go test ./internal/repository -run RollbackWritesNextVersion -count=1` 初始因 `RollbackVersion` 未定义而构建失败。
- 回滚 GREEN：同一命令通过，SQL mock 断言元素锁、目标快照、递增版本写入、回滚审计和 Commit 处于同一事务。
- 最终验证：`cd backend; go test ./... -count=1`、`cd backend; go vet ./...`、`git diff --check`。

## 范围

未扩展至 Task 5+ 的执行器浏览器拾取、定位器生成或前端实现。

## Fix round 1：授权、命令与错误边界

- 资源授权：当前 `ui_assets` 和 `page_elements` 没有 `project_id` 或 `product_id`，因此按确认的最小安全模型在会话/候选和元素页面上使用 `created_by` owner 约束，`roles.code='admin'` 依照现有统一规则放行。查询、更新和批量保存事务锁均重复该条件；未来页面引入项目归属后再迁移为 `project_members` 授权。
- 执行器认证：所有执行器入口在 JSON 解析前统一读取 `X-Executor-ID` 和 `Authorization: Bearer <session-token>`，通过会话令牌 SHA-256 摘要与执行器 ID 双校验。认证错误使用 `model.ErrUnauthorized`，不再按中文错误字符串判断。
- 命令：创建、模式切换和停止写入线程安全的进程内命令队列；命令读取必须携带已验证的 executor/session/token，按会话消费并删除已领取命令。
- 错误与缓存：新增结构化领域错误，已知参数、资源和状态错误分别映射 400/404/409，未知错误统一返回不泄露内部详情的 500。候选路由在权限中间件之前设置 `Cache-Control: no-store`，控制器也在入口重复设置。显式 `limit=0` 返回 400，未传 `limit` 才采用默认值。
- 回滚：事务锁定有权限的页面元素，目标快照以 map 方式复制并仅更新 `id`、递增 `version` 和 `source=rollback`，保留定位器的 `score`/`unique` 与其它当前字段。
- 验证：controller、repository/service 聚焦、后端全量测试和 vet 均已通过；最终提交前将再次执行 diff-check。

本轮修复已提交：`fix: secure element capture APIs`。

## Fix round 2：持久化命令与认证解环

- 命令表 `element_capture_commands` 通过幂等启动迁移创建；start、set_mode、stop 和 expire 与相应会话状态变化使用一个事务提交，轮询以 `FOR UPDATE SKIP LOCKED` 原子领取，过期命令标记失效并对已领取/失效命令执行 24 小时留存清理。
- 命令轮询不再要求尚未领取 start 命令的执行器知晓会话令牌。它使用现有 `executors.executor_token`/共享令牌机制的 `X-Executor-Token` 和 executor ID；start payload 只投递一次会话令牌，心跳、候选和失败回调继续使用该令牌。
- 页面资源的真实类型经 Bootstrap、UI 路由和创建调用核查为 `page`；`page_element` 仅代表元素列表资产，未被误作页面。创建会话先检查页面 owner/admin，创建事务内再锁定并复查，避免 TOCTOU。
- 候选项路由在 AuthMiddleware 之前安装 no-store 中间件，因此 JWT 401 响应同样禁止缓存；候选容量、并发、名称唯一和终态失败均映射为类型化领域错误。

## Fix round 3：无密令牌租约交付

- `element_capture_commands.payload` 只保存非敏感命令字段。领取 start 租约时才在同一事务内生成一次性会话令牌、写入其 hash 并将明文仅放入响应 DTO；重领会轮换 hash，旧令牌立即失效。
- 命令迁移增加 `lease_until`、`attempts` 和 `acked_at`；领取将命令置为短暂 leased，ACK 将其置为 acked，未确认的 stop/expire 会在租约到期后重投。后台调度器定时调用过期清理，不依赖轮询请求。
- 新增 `POST /api/executor/element-capture/commands/:id/ack`，使用现有执行器长期令牌和 executor ID，按命令归属原子确认。页面 owner/admin 校验兼容 `page` 与历史 `page_element` 两种页面资产。

## Fix round 4：PostgreSQL 结果集与历史数据加固

- 领取命令和会话到期处理都会先读取、检查并关闭 `UPDATE ... RETURNING` 的全部结果，再继续执行后续更新/插入，避免 pgx 在同一事务连接上报告 `conn busy`。真实 PostgreSQL 隔离 schema 测试覆盖 start 领取与超时会话命令写入。
- 迁移删除旧 JSONB payload 中的 `token`，将历史 pending/claimed 命令迁移为 queued 或 expired 并清除过期 lease；后台 scheduler 独立执行命令清理。领取响应提供随机 lease receipt，数据库仅保存其 hash，普通 ACK 不能确认 start 命令。

## Fix round 5：原子启动确认

- 首次 heartbeat 以 `commandReceipt` 绑定当前 leased start 命令；`HeartbeatAndAckStart` 在一个 PostgreSQL 事务中验证回执、更新会话状态并 ACK start，任何一步失败都会回滚。已 ACK 的 start 允许后续 heartbeat 不重复携带回执。
- 真实 PostgreSQL 聚焦测试已非跳过运行并通过，覆盖 start 领取、会话 token hash 写入、原子 heartbeat ACK 和过期会话命令入队；临时 schema 在 scoped 连接关闭后删除，并检查残留计数为 0。连接凭据仅注入测试进程，未写入仓库或报告。

## 升级接管收尾：关闭命令状态机

### 根因与修复

- 第 5 轮实现只在最新 start 命令为 `leased` 时检查回执，`queued`、`expired` 和无命令行仍会继续更新会话；`acked` 路径也未拒绝多余回执。现改为封闭状态机：仅 `leased + 有效 receipt` 执行会话更新与 ACK，或 `acked + 空 receipt` 执行后续 heartbeat；其余状态统一返回 `model.ErrConflict`，错误回执、租约过期和数据库错误均由事务回滚。
- receipt SHA-256 摘要使用 `subtle.ConstantTimeCompare` 比较；普通 ACK 同时要求非 start、leased、租约及命令有效期均有效。移除废弃 `AckStartCommand` 接口、实现和 fake。
- `ClaimCommands`、`expireSessionsAndEnqueue`、批量读取候选等 `Rows` 路径补齐 defer/成功前显式 Close，保证扫描、解码和 `Rows.Err` 早退均关闭结果集。真实 PostgreSQL 还暴露并修复了 `CleanupCommands` 的时间参数推断问题，统一显式转换为 `timestamptz`。
- 真实迁移 fixture 预建最小 roles/users/executors/ui_assets/session/candidate 依赖和 Fix2 版 commands，插入携带顶层 token 的 pending/claimed 历史命令，连续运行两轮 `app.migrate`；断言 token 清除、pending/claimed 状态迁移、租约/receipt 列、旧 pending 索引删除、新 claim 索引及无明文。

### RED

- `TestElementCaptureRepositoryHeartbeatAndAckStartStateMachine`：`queued command`、`expired command`、`missing command`、`acked follow-up rejects receipt` 首次运行均因实现继续调用 session update 而失败。
- `TestElementCaptureMigrationUpgradesAndIsIdempotentOnPostgreSQL`：旧 fixture 首次真实运行在第 1 轮第 19 条报 `relation "executors" does not exist`（SQLSTATE 42P01）。
- `TestElementCaptureRepositoryStateAuthorizationAndCleanupOnPostgreSQL`：首次真实运行在 `CleanupCommands` 报 `timestamp with time zone < interval`（SQLSTATE 42883），证明 sqlmock 未覆盖 PostgreSQL 参数类型推断。

### GREEN

- `TestElementCaptureRepositoryHeartbeatAndAckStartStateMachine`
- `TestElementCaptureRepositoryClosesCommandRowsOnDecodeFailure`
- `TestElementCaptureRepositoryClosesExpiredSessionRowsOnScanFailure`
- `TestElementCaptureRepositoryAuthorizesCommandWithSuppliedFallbackWhenSettingIsAbsent`
- `TestElementCaptureRepositoryCleanupEnforcesAttemptCapAndRetention`
- `TestElementCaptureRepositoryAckCommandMatrix`
- `TestElementCaptureRepositoryPageAccessibleOwnerDenyAndAdminMatrix`
- `TestElementCaptureRepositoryRollbackVersionFailureRollsBack`
- `TestElementCaptureMigrationUpgradesAndIsIdempotentOnPostgreSQL`（真实 PostgreSQL，两轮 migrate）
- `TestElementCaptureLeaseAndExpiryUseRealPostgreSQLWithoutBusyConnection`（真实 PostgreSQL）
- `TestElementCaptureRepositoryStateAuthorizationAndCleanupOnPostgreSQL`（真实 PostgreSQL）

最终验证：`cd backend; go test ./... -count=1`、`cd backend; go vet ./...`、`git diff --check`。真实 PostgreSQL 凭据仅注入测试进程；迁移和仓储测试 cleanup 均断言临时 schema 残留计数为 0。
