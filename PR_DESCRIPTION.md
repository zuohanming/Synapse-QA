# PR 描述：完整实现测试用例模块

## 背景

当前测试用例复用通用 `ui_assets` 资源模型，无法表达测试用例专属字段、参数化数据集和步骤关联。本 PR 新增独立测试用例模块，并保留原有 `/api/ui/cases` 兼容路由。

## 主要变更

- 新增测试用例 Model、Repository、Service、Controller。
- 新增 `/api/test-cases` REST API。
- 新增参数化数据集接口。
- 新增数据库表：
  - `test_cases`
  - `test_case_steps`
  - `test_case_datasets`
- 新增 Service 单元测试、Controller HTTP 测试、Repository sqlmock 测试。
- 新增 `SPEC.md`、`docs/api-test-case.md`、`CHANGELOG.md`、`SUMMARY.md`。

## 验证结果

```bash
go build ./cmd/api
go test ./...
go vet ./...
gofmt -l ...
```

以上命令均已通过。

## 覆盖率说明

test-case 新增函数的函数级覆盖率已基本达到或超过 85%。当前 Go 包级覆盖率仍低，因为 `controller`、`service`、`repository` 包中包含大量既有模块代码，未纳入本次测试用例模块测试范围。

## 兼容性

- 不删除旧的 `/api/ui/cases`。
- 新增 `/api/test-cases` 作为正式测试用例模块 API。

## 已知约束

原始停止条件要求“只修改 `src/test-case/`”，但当前仓库没有 `src/test-case/` 结构。为接入现有 Go 应用，本 PR 按 `backend/internal/...` 分层实现。
