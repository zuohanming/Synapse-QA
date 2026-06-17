# Changelog

## Unreleased

- 新增界面自动化测试用例专用后端模块。
- 新增 `/api/test-cases` REST API，覆盖 CRUD、详情、参数化数据集、导入和导出。
- 新增 `test_cases`、`test_case_steps`、`test_case_datasets` 数据库表和初始化逻辑。
- 新增测试用例 Service、Controller、Repository 测试。
- 前端“界面自动化 / 测试用例”切换到专用 API。
- 前端新增测试用例筛选、分页、新增、编辑、删除、批量删除、导入、导出、详情和参数化数据 UI。
- 前端新增 Vitest 组件测试、Playwright 关键 E2E 和覆盖率统计配置。
- 新增 `SPEC.md`、`tasks/test-case-module.md`、`docs/api-test-case.md`、`PR_DESCRIPTION.md`、`SUMMARY.md`。
