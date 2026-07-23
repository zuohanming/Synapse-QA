# 当前任务

## 任务名称

完整实现界面自动化下的测试用例模块。

## 用户要求

在项目根目录 `F:\Synapse QA` 内完成测试用例模块前后端全栈开发，不创建独立 worktree 或独立模块。

## 实现范围

- 后端 Model、Repository、Service、Controller、Router、DTO。
- 后端 CRUD、参数化数据集、导入导出、权限和字段验证。
- 前端测试用例列表、筛选、分页、表单、详情、批量删除、导入导出和参数化数据维护。
- 后端测试、前端构建验证、文档更新。

## 验收标准

- 后端 test-case 相关测试通过。
- 前端构建通过。
- 前端组件测试和关键 E2E 通过。
- 前后端 test-case 相关覆盖率达到 85%。
- 无 lint、格式化和构建错误。
- 代码符合 `SKILL.md`。

## 当前进展

- 后端实现已完成并通过 `go test ./...`、`go vet ./...`、`go build ./cmd/api`。
- 前端测试用例页面已接入 `/api/test-cases` 并通过 `npm run build`、`npm test`、`npm run test:coverage`、`npm run e2e`。
- test-case 新增后端函数级覆盖率已达到 85% 以上。
- 前端覆盖率：Statements 99.09%、Branches 91.9%、Functions 97.89%、Lines 99.02%。
- Playwright 关键 E2E 已通过：1 passed。
- 后端包级总覆盖率为 25.3%，原因是同包包含大量既有非 test-case 代码；test-case 新增函数级覆盖率均不低于 85%。
