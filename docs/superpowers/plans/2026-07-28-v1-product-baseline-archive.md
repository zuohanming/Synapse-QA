# Synapse QA V1 产品基线与归档实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 Synapse QA V1 日常权威产品基线，并将封版时内容固化为可追溯的 V1 归档快照。

**Architecture:** 使用“权威基线 + 冻结归档”双层文档结构。`docs/product/` 按需求、产品架构和功能状态分责维护，`docs/archive/v1/` 保存封版副本和版本说明，原 `docs/PRD.md` 保留为兼容入口。

**Tech Stack:** Markdown、PowerShell、Git

## Global Constraints

- 只修改产品文档、归档文档、每日更新和 `CHANGELOG.md`，不修改业务代码、接口、数据库或运行配置。
- API 测试用例、API 全局变量和 API Runner 正式批次链路在完成统一端到端验收前必须标记为“待验收”。
- 功能状态只能使用“已交付”“待验收”“部分交付”“V1 非范围”。
- `docs/archive/v1/` 封版后不静默改写正文；事实错误通过归档 `README.md` 勘误。
- 保留 `docs/PRD.md` 路径，避免已有引用失效。
- 每次文档变更同步追加 `docs/daily-updates/2026-07-28.md`，不得覆盖既有记录。
- 面向使用者的 V1 产品基线和归档体系变化同步记录在 `CHANGELOG.md` 的 `Unreleased` 章节。
- 不清理或提交工作区中与本计划无关的既有改动。

---

## 文件职责

| 文件 | 操作 | 单一职责 |
| --- | --- | --- |
| `docs/product/requirements-v1.md` | 新建 | V1 产品承诺、功能需求、业务规则、非功能要求和验收标准 |
| `docs/product/architecture-v1.md` | 新建 | V1 信息架构、能力分层、核心流程、对象关系和系统边界 |
| `docs/product/feature-status-v1.md` | 新建 | V1 功能逐项状态、证据和待验收条件 |
| `docs/archive/v1/README.md` | 新建 | 归档元数据、范围、口径、文件索引、已知限制和勘误规则 |
| `docs/archive/v1/requirements.md` | 新建 | `requirements-v1.md` 的封版副本 |
| `docs/archive/v1/architecture.md` | 新建 | `architecture-v1.md` 的封版副本 |
| `docs/archive/v1/feature-status.md` | 新建 | `feature-status-v1.md` 的封版副本 |
| `docs/PRD.md` | 修改 | 旧路径兼容入口和迁移说明 |
| `docs/daily-updates/2026-07-28.md` | 修改 | 本次各阶段变更、影响范围和验证结果 |
| `CHANGELOG.md` | 修改 | `Unreleased` 中记录 V1 产品基线和归档体系 |

---

### Task 1: 建立 V1 需求权威基线

**Files:**
- Create: `docs/product/requirements-v1.md`
- Modify: `docs/daily-updates/2026-07-28.md`

**Interfaces:**
- Consumes: `docs/PRD.md`、`README.md`、`executor/README.md`、`docs/superpowers/specs/2026-07-28-v1-product-baseline-archive-design.md`
- Produces: 后续架构文档和功能状态表共同引用的 V1 范围、术语和验收口径

- [ ] **Step 1: 提取现有 PRD 章节和明确边界**

Run:

```powershell
rg -n '^#{1,4} ' docs/PRD.md
rg -n '当前限制|后续需求|非功能需求|验收基线|当前边界|尚未|占位' docs/PRD.md
```

Expected:

- 输出覆盖产品概述、角色、功能需求、业务规则、非功能需求、限制和验收基线。
- API 测试用例和 API 全局变量的旧“占位”描述被识别为需要按新口径改写的内容。

- [ ] **Step 2: 创建需求基线**

创建 `docs/product/requirements-v1.md`，按以下固定结构编写：

```markdown
# Synapse QA V1 产品需求基线

## 1. 文档信息
## 2. 产品定位
## 3. 产品目标与非目标
## 4. 目标用户与职责
## 5. V1 产品范围
### 5.1 登录与认证
### 5.2 首页
### 5.3 界面自动化
### 5.4 接口自动化
### 5.5 测试配置
### 5.6 执行中心
### 5.7 测试报告
### 5.8 通知中心
### 5.9 系统管理
## 6. 核心业务规则
## 7. 非功能需求
## 8. 已知限制
## 9. V1 验收标准
## 10. 术语
```

写作要求：

- 将原 PRD 已确认需求重组到以上章节，不新增未经证据支持的功能。
- API 测试用例、API 全局变量、API Runner 批次链路描述为 V1 范围内的“待验收能力”。
- 将浏览器单接口调试、执行器单接口调试、API 用例执行和批量回归明确区分。
- 在“非目标”中明确不承诺云端 SaaS 多租户、移动原生应用、AI 自动生成用例和无限制自定义代码沙箱。
- 验收条目使用可观察结果，不使用“可用”“完善”“正常”等无条件描述。

- [ ] **Step 3: 执行需求文档静态检查**

Run:

```powershell
rg -n 'TBD|TODO|待定|基本完成|大致可用|后续完善' docs/product/requirements-v1.md
rg -n '^## ' docs/product/requirements-v1.md
git diff --check -- docs/product/requirements-v1.md
```

Expected:

- 第一条命令无匹配。
- 第二条命令完整输出步骤2规定的10个二级章节。
- `git diff --check` 无输出并返回成功。

- [ ] **Step 4: 追加每日更新**

在 `docs/daily-updates/2026-07-28.md` 追加“V1 产品需求基线”记录，必须包含：

```markdown
## V1 产品需求基线

- 变更内容：建立 V1 产品需求权威基线，统一产品定位、用户、功能范围、业务规则、非功能要求、已知限制和验收标准。
- 影响范围：产品、研发、测试、实施与运维后续使用的 V1 需求口径；未修改运行时代码。
- 验证结果：已完成章节完整性、模糊状态词和 Markdown 差异检查。
```

- [ ] **Step 5: 提交需求基线**

```powershell
git add -- docs/product/requirements-v1.md docs/daily-updates/2026-07-28.md
git diff --cached --check
git commit -m "docs: 建立 V1 产品需求基线"
```

---

### Task 2: 建立 V1 产品架构基线

**Files:**
- Create: `docs/product/architecture-v1.md`
- Modify: `docs/daily-updates/2026-07-28.md`

**Interfaces:**
- Consumes: `docs/product/requirements-v1.md` 中的产品范围和术语；`docs/backend-architecture.md`、`README.md`、`executor/README.md`
- Produces: 功能状态表和归档说明引用的信息架构、对象关系、核心流程与系统边界

- [ ] **Step 1: 核对实际菜单、路由和系统边界**

Run:

```powershell
rg -n '界面自动化|接口自动化|测试配置|执行中心|系统管理' frontend/src/components/Layout.js frontend/src/App.js
rg -n '\.(GET|POST|PUT|PATCH|DELETE)\(' backend/internal/router/router.go
rg -n '^## |^### ' docs/backend-architecture.md executor/README.md
```

Expected:

- 能识别 UI 自动化、API 自动化、配置、执行、报告和系统管理入口。
- 能识别平台 API 与执行器注册、心跳、任务下发和结果回调边界。

- [ ] **Step 2: 创建产品架构文档**

创建 `docs/product/architecture-v1.md`，使用以下固定结构：

```markdown
# Synapse QA V1 产品架构

## 1. 架构目标
## 2. 产品能力分层
## 3. 信息架构
## 4. 核心业务对象
## 5. UI 自动化业务闭环
## 6. API 自动化业务闭环
## 7. 执行调度与结果流
## 8. 系统组成与职责边界
## 9. 关键业务规则
## 10. 安全与可追溯边界
## 11. V1 架构限制
```

文档必须包含以下 Mermaid 图：

1. Web、平台 API、PostgreSQL、执行器和被测目标的系统上下文。
2. 项目、产品、模块、环境、测试资产、用例、批次、任务和报告的对象关系。
3. UI 自动化主流程。
4. API 自动化主流程，并标出待验收环节。

规则描述必须与需求基线一致：

```text
用例临时变量 > 环境级变量 > 项目级变量
接口自定义请求头 > 项目默认请求头
```

- [ ] **Step 3: 检查架构完整性和 Mermaid 围栏**

Run:

```powershell
$content = Get-Content -Raw -Encoding UTF8 docs/product/architecture-v1.md
$mermaidStarts = ([regex]::Matches($content, '```mermaid')).Count
$allFences = ([regex]::Matches($content, '```')).Count
if ($mermaidStarts -ne 4) { throw "Mermaid 图数量应为 4，实际为 $mermaidStarts" }
if (($allFences % 2) -ne 0) { throw "Markdown 代码围栏未闭合" }
rg -n '用例临时变量 > 环境级变量 > 项目级变量|接口自定义请求头 > 项目默认请求头' docs/product/architecture-v1.md
git diff --check -- docs/product/architecture-v1.md
```

Expected:

- PowerShell 脚本成功结束。
- 两条优先级规则均有匹配。
- `git diff --check` 无输出。

- [ ] **Step 4: 追加每日更新并提交**

追加：

```markdown
## V1 产品架构基线

- 变更内容：建立 V1 产品架构权威基线，固化能力分层、信息架构、核心对象、UI/API 业务闭环、执行结果流和系统边界。
- 影响范围：产品架构评审、需求拆解和后续版本规划；未修改运行时代码。
- 验证结果：已核对实际菜单、后端路由和执行器职责，并通过 Mermaid 围栏、关键规则和 Markdown 差异检查。
```

然后执行：

```powershell
git add -- docs/product/architecture-v1.md docs/daily-updates/2026-07-28.md
git diff --cached --check
git commit -m "docs: 建立 V1 产品架构基线"
```

---

### Task 3: 建立 V1 功能状态矩阵

**Files:**
- Create: `docs/product/feature-status-v1.md`
- Modify: `docs/daily-updates/2026-07-28.md`

**Interfaces:**
- Consumes: `docs/product/requirements-v1.md`、`docs/product/architecture-v1.md`、`docs/PRD.md`、`docs/daily-updates/2026-07-28.md`、当前前后端路由和模型
- Produces: 唯一的 V1 功能交付状态表和 API 自动化待验收清单

- [ ] **Step 1: 收集实现与验证证据**

Run:

```powershell
rg -n '验证结果：|go test|npm test|npm run build|Playwright|端到端' docs/daily-updates/2026-07-28.md
rg -n '/global-variables|/test-cases|/test-runs|/debug' backend/internal/router/router.go
rg -n 'APIGlobalVariable|APITestCase|APITestRun' backend/internal/model/api_test_case.go
rg -n 'APITestCasesPage|APIGlobalVariablesPage' frontend/src/App.js frontend/src/components/Layout.js
```

Expected:

- 能定位已交付能力的验证记录。
- 能定位 API 测试用例、变量、执行批次的开发证据，但不能仅凭路由或模型将其认定为“已交付”。

- [ ] **Step 2: 创建功能状态矩阵**

创建 `docs/product/feature-status-v1.md`，包含：

```markdown
# Synapse QA V1 功能状态

## 1. 状态定义
## 2. 状态判定规则
## 3. V1 功能矩阵
## 4. API 自动化待验收清单
## 5. V1 非范围
## 6. 状态维护规则
```

“V1 功能矩阵”至少覆盖：

- 登录认证
- 用户与角色权限
- 首页概览
- 项目、产品、模块、测试对象
- 页面与元素
- UI 步骤画布
- UI 测试用例
- UI 全局变量
- 接口定义和版本
- 项目默认请求头
- 单接口预览与调试
- API 全局变量
- API 测试用例和版本
- API 参数化数据集
- API Runner 正式批次执行
- 执行器管理
- 执行记录和实时日志
- 测试报告
- 通知中心
- 系统参数、审计和外观

矩阵每行使用以下列：

```text
产品域 | 功能 | V1 状态 | 范围说明 | 验证证据或待验收条件
```

API 自动化待验收清单必须逐项给出可执行条件，包括：

- 创建并发布包含多个接口步骤和多个数据集的用例；
- 通过执行器展开为对应数量的执行实例；
- 验证变量优先级和请求头优先级；
- 验证 JSONPath/正则提取、断言和失败策略；
- 验证进度、取消、回调和批次状态汇总；
- 验证报告保存每个数据集、步骤、请求和断言结果；
- 完成后端、执行器、前端和端到端测试。

- [ ] **Step 3: 检查状态词闭集**

Run:

```powershell
$allowed = @('已交付', '待验收', '部分交付', 'V1 非范围')
$rows = Get-Content -Encoding UTF8 docs/product/feature-status-v1.md |
  Where-Object { $_ -match '^\|' -and $_ -notmatch '产品域|---' }
$invalid = foreach ($row in $rows) {
  $cells = $row.Split('|') | ForEach-Object { $_.Trim() }
  if ($cells.Count -ge 5 -and $cells[3] -and $cells[3] -notin $allowed) { $row }
}
if ($invalid) { $invalid; throw '发现未定义的功能状态' }
rg -n '基本完成|大致可用|后续完善' docs/product/feature-status-v1.md
```

Expected:

- PowerShell 状态检查成功。
- 最后一条 `rg` 无匹配。

- [ ] **Step 4: 追加每日更新并提交**

追加：

```markdown
## V1 功能状态矩阵

- 变更内容：建立 V1 功能状态矩阵，以四级状态区分已交付、待验收、部分交付和 V1 非范围，并明确 API 自动化端到端验收条件。
- 影响范围：版本范围判定、验收跟踪和后续版本规划；未修改运行时代码。
- 验证结果：已核对代码与每日验证证据，并通过状态闭集和模糊状态词检查。
```

然后执行：

```powershell
git add -- docs/product/feature-status-v1.md docs/daily-updates/2026-07-28.md
git diff --cached --check
git commit -m "docs: 建立 V1 功能状态矩阵"
```

---

### Task 4: 建立旧 PRD 兼容入口和变更说明

**Files:**
- Modify: `docs/PRD.md`
- Modify: `CHANGELOG.md`
- Modify: `docs/daily-updates/2026-07-28.md`

**Interfaces:**
- Consumes: 三份 `docs/product/` 权威文档
- Produces: 旧链接使用者可发现新基线的稳定入口

- [ ] **Step 1: 将旧 PRD 调整为兼容入口**

保留 `docs/PRD.md` 文件，在文件开头标题后加入以下迁移说明：

```markdown
> **V1 权威基线已拆分归档**
>
> - [V1 产品需求基线](product/requirements-v1.md)
> - [V1 产品架构](product/architecture-v1.md)
> - [V1 功能状态](product/feature-status-v1.md)
>
> 本文件保留用于兼容历史链接。涉及功能是否正式交付时，以“V1 功能状态”为准。
```

保留原有正文作为历史汇总，不删除章节；将文档状态改为“历史实现汇总 / 兼容入口”。

- [ ] **Step 2: 更新 Changelog**

在 `CHANGELOG.md` 的 `## Unreleased` 下新增一条：

```markdown
- 建立 V1 产品需求、产品架构和功能状态权威基线，并新增不可静默改写的 V1 归档机制；API 自动化扩展在完整端到端验证前统一标记为待验收。
```

- [ ] **Step 3: 检查兼容链接**

Run:

```powershell
$targets = @(
  'docs/product/requirements-v1.md',
  'docs/product/architecture-v1.md',
  'docs/product/feature-status-v1.md'
)
$missing = $targets | Where-Object { -not (Test-Path -LiteralPath $_) }
if ($missing) { $missing; throw 'PRD 兼容入口存在缺失目标' }
rg -n 'V1 产品需求基线|V1 产品架构|V1 功能状态' docs/PRD.md
git diff --check -- docs/PRD.md CHANGELOG.md
```

Expected:

- 三个目标文件均存在。
- `docs/PRD.md` 中三条链接均有匹配。
- `git diff --check` 无输出。

- [ ] **Step 4: 追加每日更新并提交**

追加：

```markdown
## V1 文档兼容入口

- 变更内容：将原 PRD 保留为历史兼容入口，链接到新的需求、架构和状态权威基线，并在变更日志记录 V1 归档体系。
- 影响范围：历史文档链接和 V1 产品资料导航；未修改运行时代码。
- 验证结果：三个权威文档链接目标均存在，Markdown 差异检查通过。
```

然后执行：

```powershell
git add -- docs/PRD.md CHANGELOG.md docs/daily-updates/2026-07-28.md
git diff --cached --check
git commit -m "docs: 增加 V1 产品基线入口"
```

---

### Task 5: 固化 V1 归档快照

**Files:**
- Create: `docs/archive/v1/README.md`
- Create: `docs/archive/v1/requirements.md`
- Create: `docs/archive/v1/architecture.md`
- Create: `docs/archive/v1/feature-status.md`
- Modify: `docs/daily-updates/2026-07-28.md`

**Interfaces:**
- Consumes: 三份已完成的 `docs/product/` 权威文档
- Produces: 内容冻结且可独立阅读的 V1 归档包

- [ ] **Step 1: 复制封版内容**

Run:

```powershell
New-Item -ItemType Directory -Force -Path 'docs/archive/v1' | Out-Null
Copy-Item -LiteralPath 'docs/product/requirements-v1.md' -Destination 'docs/archive/v1/requirements.md'
Copy-Item -LiteralPath 'docs/product/architecture-v1.md' -Destination 'docs/archive/v1/architecture.md'
Copy-Item -LiteralPath 'docs/product/feature-status-v1.md' -Destination 'docs/archive/v1/feature-status.md'
```

Expected:

- `docs/archive/v1/` 下生成三份内容副本。
- 不删除或覆盖 `docs/product/` 中的权威文件。

- [ ] **Step 2: 创建归档说明**

创建 `docs/archive/v1/README.md`，包含以下固定结构：

```markdown
# Synapse QA V1 产品归档

## 版本信息
## 归档范围
## 文件索引
## 功能状态口径
## V1 已知限制
## API 自动化待验收声明
## 归档维护规则
## 勘误记录
## 后续版本
```

具体要求：

- 版本号写为 `V1.0`。
- 归档日期写为 `2026-07-28`。
- 文件索引使用相对链接指向三份归档文档。
- “勘误记录”初始内容写为“归档时无勘误”。
- “后续版本”明确新功能进入 V1.1 或 V2，不回填为 V1 原始承诺。

- [ ] **Step 3: 验证归档副本一致**

Run:

```powershell
$pairs = @(
  @('docs/product/requirements-v1.md', 'docs/archive/v1/requirements.md'),
  @('docs/product/architecture-v1.md', 'docs/archive/v1/architecture.md'),
  @('docs/product/feature-status-v1.md', 'docs/archive/v1/feature-status.md')
)
foreach ($pair in $pairs) {
  $sourceHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $pair[0]).Hash
  $archiveHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $pair[1]).Hash
  if ($sourceHash -ne $archiveHash) {
    throw "归档副本不一致：$($pair[0]) -> $($pair[1])"
  }
}
rg -n 'V1.0|2026-07-28|归档时无勘误|待验收' docs/archive/v1/README.md
git diff --check -- docs/archive/v1
```

Expected:

- 三组 SHA256 哈希完全一致。
- 归档版本、日期、勘误和待验收声明均有匹配。
- `git diff --check` 无输出。

- [ ] **Step 4: 追加每日更新并提交**

追加：

```markdown
## V1 产品资料归档

- 变更内容：将 V1 产品需求、产品架构和功能状态固化到归档目录，并新增归档元数据、已知限制、待验收声明和勘误规则。
- 影响范围：V1 历史版本追溯和后续 V1.1/V2 版本规划；未修改运行时代码。
- 验证结果：三份归档副本与权威基线 SHA256 完全一致，归档说明关键字段和 Markdown 差异检查通过。
```

然后执行：

```powershell
git add -- docs/archive/v1 docs/daily-updates/2026-07-28.md
git diff --cached --check
git commit -m "docs: 固化 V1 产品资料归档"
```

---

### Task 6: 执行 V1 文档终验

**Files:**
- Modify: `docs/daily-updates/2026-07-28.md`

**Interfaces:**
- Consumes: 全部 V1 权威文档、归档文档、兼容入口和变更日志
- Produces: 可复查的最终验收证据

- [ ] **Step 1: 检查必需文件**

Run:

```powershell
$required = @(
  'docs/product/requirements-v1.md',
  'docs/product/architecture-v1.md',
  'docs/product/feature-status-v1.md',
  'docs/archive/v1/README.md',
  'docs/archive/v1/requirements.md',
  'docs/archive/v1/architecture.md',
  'docs/archive/v1/feature-status.md',
  'docs/PRD.md',
  'CHANGELOG.md'
)
$missing = $required | Where-Object { -not (Test-Path -LiteralPath $_) }
if ($missing) { $missing; throw 'V1 归档文件不完整' }
```

Expected: 命令无错误结束。

- [ ] **Step 2: 检查模糊状态和未决占位符**

Run:

```powershell
$targets = @('docs/product', 'docs/archive/v1')
$matches = rg -n 'TBD|TODO|待定|基本完成|大致可用|后续完善' $targets
if ($LASTEXITCODE -eq 0) { $matches; throw '发现占位符或模糊状态词' }
if ($LASTEXITCODE -gt 1) { throw 'rg 扫描失败' }
```

Expected: 命令无匹配并成功结束。

- [ ] **Step 3: 复核待验收能力未被误标**

Run:

```powershell
$status = Get-Content -Raw -Encoding UTF8 docs/product/feature-status-v1.md
$requiredPending = @('API 全局变量', 'API 测试用例', 'API Runner')
foreach ($name in $requiredPending) {
  if ($status -notmatch "(?m)^\|[^\r\n]*$([regex]::Escape($name))[^\r\n]*\|\s*待验收\s*\|") {
    throw "$name 未明确标记为待验收"
  }
}
```

Expected: 三项均明确标记为“待验收”。

- [ ] **Step 4: 复核归档哈希和 Markdown 差异**

Run:

```powershell
$pairs = @(
  @('docs/product/requirements-v1.md', 'docs/archive/v1/requirements.md'),
  @('docs/product/architecture-v1.md', 'docs/archive/v1/architecture.md'),
  @('docs/product/feature-status-v1.md', 'docs/archive/v1/feature-status.md')
)
foreach ($pair in $pairs) {
  if ((Get-FileHash -Algorithm SHA256 -LiteralPath $pair[0]).Hash -ne
      (Get-FileHash -Algorithm SHA256 -LiteralPath $pair[1]).Hash) {
    throw "归档副本不一致：$($pair[1])"
  }
}
git diff --check
```

Expected:

- 三组哈希一致。
- `git diff --check` 无输出。

- [ ] **Step 5: 记录最终验证并提交**

在当日更新的“V1 产品资料归档”记录中，将验证结果补充为：

```markdown
- 验证结果：必需文件完整；占位符和模糊状态词扫描无匹配；API 全局变量、API 测试用例和 API Runner 均明确标记为待验收；三份归档副本与权威基线 SHA256 完全一致；`git diff --check` 通过。
```

然后执行：

```powershell
git add -- docs/daily-updates/2026-07-28.md
git diff --cached --check
git commit -m "docs: 完成 V1 产品归档验收"
git status --short
```

Expected:

- 提交成功。
- `git status --short` 仍可显示任务开始前已有的无关改动，但不得出现本计划遗漏的 V1 文档改动。
