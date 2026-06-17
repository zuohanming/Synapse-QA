# Synapse QA 项目规范

## 技术栈

- 前端：React + Vite，源码位于 `frontend/src`。
- 后端：Go，API 入口位于 `backend/cmd/api`。
- 数据库：PostgreSQL。
- 执行器：独立 `executor` 模块。

## 编码风格

- 优先保持现有文件风格，不做无关格式化和重构。
- 前端页面级业务逻辑放在 `frontend/src/pages`。
- 前端公共组件放在 `frontend/src/components`。
- 前端请求封装放在 `frontend/src/services`。
- 前端路由、状态恢复等通用工具放在 `frontend/src/utils`。
- 后端按 `controller`、`service`、`repository`、`model`、`router` 分层。
- 数据校验优先放在 service 层，controller 层只做参数解析和响应组装。
- 单次需求只修改必要文件，避免顺手处理无关问题。

## 架构原则

- 前后端功能以稳定接口为边界，先保证 API 行为明确，再接入页面。
- 列表、分页、弹窗、表单等 UI 能力优先复用公共组件。
- 数据库结构变更必须同步初始化逻辑，并说明兼容性影响。
- 核心 CRUD 与 AI 能力保持服务边界清晰，避免直接耦合。
- 页面刷新、筛选、分页、动态路由等状态应尽量可恢复。

## 工作区约束

- 所有开发任务必须在项目根目录 `F:\Synapse QA` 内进行。
- 禁止为普通功能开发创建独立 git worktree、独立仓库或脱离主项目的孤立模块。
- 新功能必须接入当前项目既有目录结构；后端遵循 `backend/internal/...` 分层，前端遵循 `frontend/src/...` 分层。
- 如需求提到不存在的目录结构，应先对照当前项目结构调整实现路径，并在任务文档中记录原因。
- 只有用户明确要求隔离实验或临时验证时，才允许创建独立目录；该目录不得作为正式实现位置。

## 测试与验证命令

前端构建：

```bash
cd frontend
npm run build
```

后端测试：

```bash
cd backend
go test ./...
```

后端编译：

```bash
cd backend
go build ./cmd/api
```

后端静态检查：

```bash
cd backend
go vet ./...
```

查看变更范围：

```bash
git status --short
git diff --stat
```

## Loop Engineering 模式

每轮开发按以下闭环推进：

1. 目标：明确本轮做什么、不做什么、验收标准是什么。
2. 边界：确认只修改与目标直接相关的文件。
3. 方案：列出实现路径、数据结构、接口和风险点。
4. 实现：按最小可行范围落地。
5. 验证：运行构建、测试或浏览器关键路径验证。
6. 记录：必要时更新 `STATE.md`、`tasks/current-task.md` 或变更文档。
7. 提交：确认 diff 范围后再提交或推送。

## 提交前检查

- UI 改动必须运行前端构建，必要时浏览器验证关键路径。
- 后端接口改动必须运行 `go test ./...` 和 `go build ./cmd/api`。
- 数据库改动必须说明表结构、初始化逻辑和兼容性。
- 提交前必须确认工作区只包含本轮相关变更。
