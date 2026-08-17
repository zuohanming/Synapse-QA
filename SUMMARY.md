# 性能测试模块实现摘要

## 已完成

- 在根目录 `F:\Synapse QA` 内完成开发，未使用独立 worktree。
- 新增性能测试后端分层：
  - `backend/internal/model/performance.go`
  - `backend/internal/repository/performance_repository.go`
  - `backend/internal/service/performance_service.go`
  - `backend/internal/controller/performance_controller.go`
- 接入 `backend/cmd/api/main.go`、`backend/cmd/api/bootstrap.go`、`backend/internal/router/router.go`、`backend/cmd/api/api_migration.go`（新增权限码 `menu.perf_test.read`）。
- 新增数据库表：
  - `perf_test_plans`（压测方案，软删除，unique(product_id,name)）
  - `perf_test_runs`（执行记录/指标报告）
- 新增 REST API：
  - 压测方案 CRUD（分页、详情、创建、更新、软删除）
  - 触发执行（占位，创建待执行记录）
  - 执行记录列表与详情
- 前端新增「性能测试」一级菜单（二级：压测方案、测试报告）：
  - `frontend/src/pages/PerformancePage.js`（二级菜单分发器）
  - `frontend/src/pages/PerfPlansPage.js`（压测方案：列表/筛选/表单弹窗/详情/触发执行）
  - `frontend/src/pages/PerfRunsPage.js`（测试报告：执行记录列表/指标详情抽屉）
  - `frontend/src/services/performanceService.js`
- 接入菜单/路由/样式：`appConfig.js`、`Layout.js`、`routes/index.js`、`global.css`
- 新增后端测试：`performance_service_test.go`、`performance_controller_test.go`

## 当前验证

已执行并通过：

```bash
cd backend
go build ./...
go vet ./...
go test ./...

cd frontend
npm run build
npm run test
```

结果：

- 后端全部包测试通过，新增 service 12 个、controller 8 个测试函数通过。
- 前端 build 通过（仅既有 chunk size 提示），`npm run test` 85 个用例全部通过。

## 压测引擎选型

- 底层压测引擎选定 Grafana k6，集成方式为「独立 k6 二进制 + 子进程 + handleSummary 写 JSON」（不使用 Go 库内嵌，官方未提供稳定 runner API）。
- 方案字段对齐 k6 选项（vus/duration/stages/thresholds），报告指标对齐 k6 summary metrics（http_reqs count、http_req_duration avg/p95、http_req_failed rate、rps）。
- 真实压测执行闭环（执行器接入 k6）为后续独立项。

## 后续建议

- 执行器新增 `perf` 任务类型与 k6 runner（subprocess 调 k6，解析 handleSummary JSON 回填指标与 summary）。
- 后端扩展压测调度与类型校验（`normalizeExecutionRunRequest` 放行 perf、`buildTaskPayload` 生成压测 payload）。
- 如需实时压测曲线，可复用 API 调试的事件回调 + SSE 推送链路。
