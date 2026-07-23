# PR 描述：完整实现界面自动化测试用例模块

## 背景

原测试用例功能复用通用 `ui_assets` 资源模型，无法表达用例优先级、参数化数据、步骤关联、详情和导入导出。本次改造在现有项目根目录内接入专用测试用例模块。

## 主要变更

- 新增测试用例 Model、Repository、Service、Controller。
- 新增 `/api/test-cases` REST API：
  - 分页查询
  - 详情
  - 新增
  - 编辑
  - 删除
  - 参数化数据集 CRUD
  - JSON 导入
  - JSON 导出
- 新增数据库表：
  - `test_cases`
  - `test_case_steps`
  - `test_case_datasets`
- 前端“界面自动化 / 测试用例”接入专用 API。
- 前端实现列表、筛选、分页、表单弹窗、详情面板、批量删除、导入导出和参数化数据维护。
- 新增后端 Service、Controller、Repository 测试。
- 新增前端 Vitest 组件测试、Playwright 关键 E2E 和覆盖率统计。
- 新增 `SPEC.md`、`tasks/test-case-module.md`、`docs/api-test-case.md`、`CHANGELOG.md`、`SUMMARY.md`。

## 验证结果

```bash
cd backend
go test ./...
go vet ./...
go build ./cmd/api

cd frontend
npm run build
npm test
npm run test:coverage
npm run e2e
```

以上命令均已通过。

## 覆盖率说明

test-case 新增后端函数级覆盖率均不低于 85%。当前 Go 包级总覆盖率为 25.3%，原因是 Go 以包为单位统计覆盖率，而相关包中包含大量既有非 test-case 代码。

前端覆盖率结果：

- Statements：99.09%
- Branches：91.9%
- Functions：97.89%
- Lines：99.02%

Playwright 关键 E2E：1 passed。

## 兼容性

- 保留旧 `/api/ui/cases` 兼容路由。
- 新增 `/api/test-cases` 作为正式测试用例模块 API。
