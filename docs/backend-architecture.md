# 自动化测试平台后端架构规划

## 1. 架构结论

后端主服务采用 Go，自动化执行生态保留 Python，AI 能力作为独立能力层接入平台。

```text
平台控制面：Go
任务调度：Go
执行器 Agent：Go
自动化执行脚本：Python / Pytest / Playwright / Selenium / Requests
AI 能力层：Go 编排 + Python 辅助处理 + 大模型服务
```

推荐整体形态：

```text
Go 平台后端
  |-- 业务 API
  |-- 权限与项目上下文
  |-- 任务调度
  |-- 执行器管理
  |-- 报告中心
  |-- AI 编排

Go 执行器 Agent
  |-- 拉取任务
  |-- 启动 Python 执行插件
  |-- 上传日志、截图、报告

Python 执行生态
  |-- Pytest
  |-- Playwright
  |-- Selenium
  |-- Requests
  |-- Allure

AI 能力层
  |-- 自动生成测试用例
  |-- 测试报告诊断
  |-- 失败聚类
  |-- 日志摘要
  |-- 影响范围分析
  |-- 智能补充断言
```

## 2. 核心原则

- Go 负责平台稳定性、并发、调度、权限和数据一致性。
- Python 负责测试执行生态、报告处理和部分数据加工。
- AI 不直接改写生产数据，默认生成草稿、建议或诊断结论，由用户确认后入库。
- 所有 AI 结果必须可追溯：输入来源、模型、提示词版本、生成时间、操作者都要记录。
- AI 能力先做辅助决策，不做不可回滚的自动决策。

## 3. 总体架构

```text
Web 前端
  |
  | REST API / SSE / WebSocket
  v
Go 平台后端
  |
  |-- 用户与权限
  |-- 项目与环境配置
  |-- 自动化资产管理
  |-- 任务调度
  |-- 执行器中心
  |-- 报告中心
  |-- 文件中心
  |-- 消息通知
  |-- AI 编排中心
  |
  | PostgreSQL / Redis / 文件存储 / pgvector
  v
Go 执行器 Agent
  |
  |-- Python 执行插件
  |-- 日志回传
  |-- 截图与报告上传
```

## 4. 技术栈建议

### 4.1 后端主服务

```text
语言：Go
Web 框架：Gin
ORM：GORM
数据库：PostgreSQL
缓存与队列：Redis
异步任务：Asynq
定时任务：robfig/cron
实时推送：SSE 优先，必要时 WebSocket
认证：JWT
权限：RBAC + 项目数据权限
日志：zap
配置：Viper
API 文档：OpenAPI / Swagger
```

### 4.2 AI 相关组件

```text
AI 编排：Go 后端模块
文本处理：Go 为主，复杂文件解析可用 Python worker
向量检索：PostgreSQL + pgvector
模型接入：通过统一 Model Provider 接口
异步生成：Redis + Asynq
结果推送：SSE
```

模型接入不要写死某一家供应商，需要抽象为：

```text
ModelProvider
  |-- Chat
  |-- Embedding
  |-- Rerank 可选
```

这样后续可以切换 OpenAI、私有模型或其他兼容 OpenAI API 的模型服务。

### 4.3 PostgreSQL 使用建议

PostgreSQL 作为平台主数据库，同时承担早期 AI 检索所需的向量存储能力。

建议启用扩展：

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS vector;
```

推荐用途：

- 业务数据：用户、项目、环境、用例、执行记录、报告。
- JSON 数据：步骤、断言、AI 输出、诊断证据可用 `jsonb` 存储。
- 全文检索：日志摘要、用例标题、接口描述可用 PostgreSQL 全文检索。
- 向量检索：历史用例、接口定义、失败诊断摘要可用 `pgvector`。

Go 侧建议：

```text
驱动：pgx
ORM：GORM PostgreSQL driver
迁移：golang-migrate 或 goose
主键：UUID 或 bigserial，优先统一一种
时间字段：timestamptz
JSON 字段：jsonb
```

本地开发连接串示例：

```text
postgres://postgres:postgres@127.0.0.1:5432/synapse_qa?sslmode=disable
```

## 5. 服务边界

### 5.1 Go 平台后端

负责：

- 前端 API。
- 用户、角色、权限和项目数据隔离。
- 自动化资产管理。
- 执行任务编排和状态流转。
- 执行器注册、心跳和容量管理。
- 报告聚合、趋势统计和通知。
- AI 请求编排、上下文组装、结果落库。

不负责：

- 不直接执行用户测试脚本。
- 不直接运行不可信代码。
- 不让 AI 绕过权限访问数据。

### 5.2 Go 执行器 Agent

负责：

- 注册到平台。
- 上报心跳和资源指标。
- 拉取任务。
- 下载脚本、数据和配置。
- 调用 Python 执行插件。
- 回传日志、截图、报告和执行结果。

### 5.3 Python 执行插件

负责：

- 执行 Pytest、Playwright、Selenium、Requests 等测试。
- 生成 Allure 原始数据。
- 处理截图、日志、测试附件。
- 按平台协议输出结构化结果。

### 5.4 AI 编排中心

负责：

- 接收 AI 任务请求。
- 校验用户权限和项目上下文。
- 收集测试资产、接口定义、执行日志、报告、缺陷历史等上下文。
- 调用模型生成结果。
- 对结果做结构化解析和规则校验。
- 生成草稿、建议、诊断报告或复测计划。
- 记录 AI 调用审计。

不负责：

- 不直接绕过业务流程创建正式用例。
- 不直接删除、覆盖用户资产。
- 不自动执行高风险操作。

## 6. AI 功能规划

### 6.1 自动生成测试用例

输入来源：

- 需求描述。
- 接口文档。
- Swagger / OpenAPI。
- 页面元素。
- 历史用例。
- 缺陷记录。
- 产品模块信息。

输出内容：

- 用例标题。
- 前置条件。
- 测试步骤。
- 测试数据。
- 预期结果。
- 优先级。
- 标签。
- 适用环境。
- 自动化类型建议：UI、API、单元、混合。

落地方式：

```text
AI 生成用例草稿
  -> 用户预览
  -> 用户编辑
  -> 用户确认入库
  -> 进入正式用例管理
```

初期不要让 AI 直接生成正式用例，必须经过人工确认。

### 6.2 自动生成接口测试

输入来源：

- Swagger / OpenAPI。
- Postman Collection。
- HAR 文件。
- 接口历史请求日志。
- 项目环境变量。
- 请求头配置。

输出内容：

- 正常场景。
- 异常场景。
- 边界值场景。
- 参数组合。
- 断言建议。
- 参数关联建议。
- Mock 数据建议。

特别注意：

- AI 生成请求参数时不能使用真实敏感数据。
- Token、密码、手机号、身份证等字段必须脱敏。
- 参数关联建议必须由用户确认。

### 6.3 自动生成 UI 自动化步骤

输入来源：

- 页面元素库。
- 页面结构。
- 历史步骤模板。
- 用户自然语言描述。

输出内容：

- 页面步骤。
- 元素选择建议。
- 等待策略。
- 断言步骤。
- 截图点。
- 失败恢复建议。

落地方式：

```text
自然语言描述
  -> AI 生成步骤草稿
  -> 映射页面元素
  -> 用户确认
  -> 保存为步骤模板或用例步骤
```

### 6.4 测试报告诊断

输入来源：

- 执行结果。
- 失败日志。
- 断言结果。
- 请求/响应日志。
- 截图。
- 历史失败记录。
- 环境变更记录。
- 执行器状态。

输出内容：

- 失败摘要。
- 可能根因。
- 影响范围。
- 复测建议。
- 责任归属建议。
- 是否疑似环境问题。
- 是否疑似数据问题。
- 是否疑似脚本问题。
- 是否疑似产品缺陷。

输出示例：

```text
结论：更可能是测试数据问题
证据：
1. 失败集中在会员等级字段
2. 最近三次失败都发生在预发环境
3. 相同用例在测试环境通过
建议：
1. 刷新预发会员基础数据
2. 重跑订单链路失败集合
3. 若仍失败，再转产品缺陷
```

### 6.5 失败聚类

目标：

- 将大量失败用例按相似根因归组。
- 减少人工逐条查看失败日志的成本。

聚类维度：

- 错误信息。
- 堆栈摘要。
- 接口路径。
- 断言字段。
- 页面元素。
- 执行环境。
- 执行器。
- 最近代码或配置变更。

输出内容：

- 失败簇名称。
- 涉及用例数。
- 代表性日志。
- 可能根因。
- 建议负责人。
- 建议复测范围。

### 6.6 智能补充断言

输入来源：

- 接口响应样例。
- 历史断言。
- 接口定义。
- 业务字段字典。

输出内容：

- 状态码断言。
- 业务码断言。
- 字段存在性断言。
- 字段类型断言。
- 关键业务字段断言。
- 响应时间断言。

原则：

- AI 只能生成断言建议。
- 启用断言必须由用户确认。
- 高风险断言需要标记原因。

### 6.7 智能复测计划

输入来源：

- 失败用例。
- 失败聚类结果。
- 代码变更模块。
- 缺陷修复记录。
- 环境变更记录。

输出内容：

- 最小复测集合。
- 推荐执行顺序。
- 是否需要全量回归。
- 推荐执行器。
- 预计耗时。

## 7. AI 数据流

```text
用户发起 AI 操作
  |
  v
Go 后端校验权限
  |
  v
AI 编排中心收集上下文
  |
  |-- 项目数据
  |-- 用例数据
  |-- 接口定义
  |-- 执行日志
  |-- 报告附件
  |-- 历史缺陷
  |
  v
脱敏与裁剪
  |
  v
构造提示词
  |
  v
调用模型服务
  |
  v
结构化解析
  |
  v
规则校验
  |
  v
保存 AI 结果草稿
  |
  v
前端展示给用户确认
```

## 8. AI 上下文管理

需要建设统一的上下文接口：

```text
ProjectContext
CaseContext
ApiContext
ExecutionContext
ReportContext
FileContext
UserPermissionContext
```

上下文组装规则：

- 只允许读取用户有权限的项目数据。
- 优先使用结构化数据，不直接喂大段原始日志。
- 日志需要先摘要、裁剪、脱敏。
- 大文件先解析成结构化摘要，再进入模型上下文。
- 对提示词长度做硬限制。

## 9. AI 安全与审计

必须记录：

```text
ai_tasks
ai_task_inputs
ai_task_outputs
ai_prompt_versions
ai_model_calls
ai_user_confirmations
```

每次 AI 调用记录：

- 用户 ID。
- 项目 ID。
- 功能类型。
- 输入数据摘要。
- 使用的模型。
- 提示词版本。
- Token 消耗。
- 输出结果。
- 用户是否采纳。
- 采纳后生成的业务对象 ID。

敏感数据处理：

- 密码、Token、密钥、Cookie 默认不进入模型。
- 手机号、身份证、邮箱、银行卡默认脱敏。
- 请求头中的 Authorization 默认脱敏。
- 环境变量中的加密变量不进入模型。

## 10. AI 数据模型初稿

```text
ai_tasks
  id
  project_id
  user_id
  task_type
  status
  source_type
  source_id
  model_provider
  model_name
  prompt_version
  input_summary
  output_summary
  error_message
  created_at
  updated_at

ai_generated_cases
  id
  ai_task_id
  project_id
  case_type
  title
  priority
  tags
  preconditions
  steps_json
  expected_results
  status
  confirmed_by
  confirmed_at

ai_report_diagnoses
  id
  ai_task_id
  execution_run_id
  conclusion
  root_cause_type
  evidence_json
  suggestion_json
  confidence
  confirmed_by
  confirmed_at

ai_failure_clusters
  id
  ai_task_id
  execution_run_id
  cluster_name
  root_cause_type
  case_count
  representative_log
  suggestion

ai_prompt_versions
  id
  name
  version
  template
  enabled
  created_at
```

## 11. 核心业务模块

### 11.1 用户与权限

- 用户管理。
- 角色管理。
- 菜单权限。
- 按钮权限。
- API 权限。
- 项目成员权限。
- 数据权限。

### 11.2 项目配置

- 项目管理。
- 产品管理。
- 模块管理。
- 环境管理。
- 数据库配置。
- Redis 配置。
- MQ 配置。
- 第三方服务配置。
- 通知组管理。

### 11.3 自动化资产

UI 自动化：

- 页面管理。
- 元素管理。
- 步骤管理。
- 用例管理。
- 场景管理。
- 数据驱动。

API 自动化：

- 接口管理。
- Swagger/Postman/HAR 导入。
- 请求头管理。
- 参数关联。
- 断言配置。
- Mock 服务。

单元自动化：

- 仓库绑定。
- 用例同步。
- 标签管理。
- 参数化管理。
- 工具文件。
- 测试文件。

### 11.4 执行与报告

- 手动执行。
- 定时执行。
- 批量执行。
- 混合任务。
- 失败重跑。
- 任务取消。
- 超时控制。
- 执行日志。
- 断言结果。
- 截图查看。
- 趋势分析。
- 成功率统计。
- 缺陷分析。

### 11.5 AI 能力中心

- 用例生成。
- 接口测试生成。
- UI 步骤生成。
- 报告诊断。
- 失败聚类。
- 日志摘要。
- 智能断言建议。
- 智能复测计划。
- AI 调用审计。

## 12. 任务执行链路

```text
1. 用户在前端点击执行
2. Go 后端创建 execution_run
3. 后端拆分 execution_task
4. 任务进入 Redis 队列
5. 执行器拉取任务
6. 执行器下载用例、配置、文件和脚本
7. 执行器启动 Python 插件
8. Python 插件执行测试
9. 执行器实时上传日志和状态
10. 执行器上传报告、截图、附件
11. 后端聚合结果
12. AI 可选触发报告诊断
13. 前端实时刷新执行状态和诊断结果
14. 通知模块发送结果通知
```

## 13. API 设计原则

- REST API 为主。
- 执行状态和 AI 生成进度使用 SSE。
- 写操作记录操作日志。
- AI 生成类接口必须返回任务 ID。
- AI 任务异步执行，前端通过任务 ID 查询或订阅进度。
- 所有 AI 接口必须校验项目权限。

示例：

```text
POST   /api/projects/{projectId}/ai/cases/generate
GET    /api/ai/tasks/{taskId}
GET    /api/ai/tasks/{taskId}/stream
POST   /api/ai/generated-cases/{id}/confirm
POST   /api/executions/{runId}/ai/diagnose
GET    /api/executions/{runId}/ai/diagnosis
POST   /api/executions/{runId}/ai/failure-clusters
POST   /api/projects/{projectId}/api-definitions/{apiId}/ai/assertions
```

## 14. 目录结构建议

```text
backend/
  cmd/
    api/
    worker/
    scheduler/
  internal/
    app/
    config/
    middleware/
    modules/
      auth/
      project/
      ui_auto/
      api_auto/
      unit_auto/
      execution/
      executor/
      report/
      file/
      notification/
      system/
      ai/
        application/
        domain/
        infrastructure/
        prompts/
    pkg/
      response/
      errors/
      logger/
      storage/
      queue/
      modelprovider/
      sanitizer/
  migrations/
  docs/
  go.mod

agent/
  cmd/
    agent/
  internal/
    executor/
    heartbeat/
    runner/
    uploader/
  go.mod

plugins/
  python/
    pytest_runner/
    playwright_runner/
    report_parser/
    log_summarizer/
```

## 15. 迭代计划

### 阶段 1：后端骨架

目标：

- Go API 服务可启动。
- PostgreSQL、Redis 配置完成。
- 用户登录、项目管理、环境管理基础接口可用。
- OpenAPI 文档可访问。

验收：

- 本地可启动 API 服务。
- 登录接口返回 JWT。
- 项目列表、新增、编辑、删除可用。
- 数据库迁移可重复执行。

### 阶段 2：执行模型

目标：

- 建立 execution_runs、execution_tasks、execution_logs。
- 支持手动创建执行任务。
- 支持任务状态流转。

验收：

- 前端可发起一次模拟执行。
- 后端可生成执行记录和任务记录。
- 日志可以按 runId 查询。

### 阶段 3：执行器 Agent

目标：

- Agent 可注册。
- Agent 可上报心跳。
- Agent 可拉取任务。
- Agent 可回传日志和结果。

验收：

- 后台能看到执行器在线。
- 停止 Agent 后能识别离线。
- Agent 能执行一个模拟任务并回传完成状态。

### 阶段 4：Python 执行插件

目标：

- 支持 Pytest 执行。
- 支持 Playwright 执行。
- 支持上传截图、日志、报告。

验收：

- 可以通过平台触发一个真实 Pytest 用例。
- 可以查看执行日志和结果。
- 可以下载或查看报告附件。

### 阶段 5：报告中心与定时任务

目标：

- 支持 Cron 策略。
- 支持执行趋势。
- 支持成功率统计。
- 支持失败分析。

验收：

- 定时任务能自动触发。
- 报告中心能展示趋势图。
- 失败用例可按模块、环境、负责人筛选。

### 阶段 6：AI MVP

目标：

- 支持基于接口定义生成 API 测试用例草稿。
- 支持基于执行报告生成失败诊断。
- 支持 AI 任务异步执行和进度查询。
- 支持 AI 结果人工确认后入库。

验收：

- 用户选择一个接口后，可以生成测试用例草稿。
- 用户可以编辑并确认 AI 生成用例。
- 执行失败后，可以生成诊断结论。
- AI 调用记录可审计。

### 阶段 7：AI 增强

目标：

- 失败聚类。
- 智能断言建议。
- 智能复测计划。
- 历史用例检索增强。

验收：

- 一次失败执行可自动归类主要失败簇。
- 可基于响应样例生成断言建议。
- 可给出最小复测集合。

## 16. 一期建议范围

一期不要覆盖所有菜单，先聚焦：

```text
用户登录
项目管理
环境管理
执行器管理
手动执行
执行记录
执行日志
报告附件
AI 报告诊断 MVP
AI 接口用例生成 MVP
```

AI 功能建议一期只做两个：

```text
1. 从接口定义生成 API 测试用例草稿
2. 从失败报告生成诊断结论
```

原因：

- 这两个功能价值最直观。
- 输入数据结构相对清晰。
- 容易做人工确认。
- 能快速验证 AI 能否真正减少测试人员工作量。

## 17. 风险与控制

### 17.1 AI 结果不稳定

控制方式：

- AI 只生成草稿。
- 必须人工确认。
- 提示词版本化。
- 结果结构化校验。

### 17.2 敏感数据泄露

控制方式：

- 统一脱敏层。
- 加密变量禁止进入模型。
- AI 输入输出审计。
- 按项目权限组装上下文。

### 17.3 成本失控

控制方式：

- AI 调用异步排队。
- 单项目限流。
- 单用户限流。
- 上下文长度限制。
- 记录 Token 消耗。

### 17.4 用户过度信任 AI

控制方式：

- 页面明确显示 AI 置信度和证据。
- 高风险结论必须标记“需人工确认”。
- 不允许 AI 自动关闭缺陷或自动放行发布。

## 18. 下一步

建议下一步先细化三份设计：

```text
1. execution_run / execution_task 状态机
2. 执行器注册、心跳、拉取任务协议
3. AI 用例生成与报告诊断的数据结构
```

完成这三份设计后，再初始化 Go 后端工程。
