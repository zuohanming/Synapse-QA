# 界面自动化测试用例模块任务

## 目标

在项目根目录 `F:\Synapse QA` 内完整实现界面自动化下的测试用例模块，覆盖后端 API、前端页面、测试、验证和文档。

## 范围

- 后端：Model、Repository、Service、Controller、Router、DTO、数据库初始化。
- 后端能力：CRUD、参数化数据集、导入导出、权限校验、字段验证。
- 前端：测试用例列表页、表单页/弹窗、详情区域、API 调用、状态管理、表单验证。
- 前端能力：搜索、筛选、批量操作、导入导出 UI。
- 测试：后端单元测试和 HTTP 集成测试，前端组件测试，关键 E2E 验证。
- 文档：`SPEC.md`、`docs/api-test-case.md`、`PR_DESCRIPTION.md`、`CHANGELOG.md`、`SUMMARY.md`。

## 工作区约束

- 所有正式实现必须在根目录 `F:\Synapse QA` 内完成。
- 不创建独立 git worktree。
- 不创建脱离当前项目结构的独立模块。
- 后端接入现有 `backend/internal/...` 分层。
- 前端接入现有 `frontend/src/...` 分层。

## 验收标准

- 后端所有 test-case 相关测试通过。
- 前端组件测试和关键 E2E 通过。
- 前后端 test-case 相关覆盖率达到 85%。
- 无 lint、格式化和构建错误。
- 代码符合 `SKILL.md`。
