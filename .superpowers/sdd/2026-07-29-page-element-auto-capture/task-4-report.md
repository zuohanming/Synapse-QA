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
