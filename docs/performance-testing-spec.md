# 性能测试真实需求 SPEC

> 版本：v2.1（真实需求，已按评审修订） · 底层压测引擎：Grafana k6 · 状态：已评审
> 前序：v1 已完成「方案管理 + 报告框架」占位版（触发执行仅落待执行记录）。v2 以 6 大测试场景为核心；v2.1 已修正调度模型、超时/取消、k6 检测链路等评审问题。

## 1. 背景与目标

- 当前 `perf_test_plans` / `perf_test_runs` 已支持方案 CRUD 与报告数据建模，但触发执行为占位，未发生真实压测。
- 执行器（Python + FastAPI）无并发 HTTP / 压测能力，底层压测引擎选定 **Grafana k6**（独立二进制 + 子进程 + handleSummary 写 JSON）。
- 目标：让测试人员按 **6 大场景** 配置并真实执行压测，实时观察指标，得到聚合报告与阈值判定，并支持基线回归对比。

## 2. 核心设计：6 大测试场景

性能测试方案以「场景类型（scenario_type）」为一等分类，每个场景有明确的测试目的、负载施加方式、K6 映射、关键参数与核心观察指标。

| # | 场景 | 类型 | 核心作用 | 负载方式 | K6 映射 | 关键参数 | 核心观察指标 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 基准测试 | `baseline` | 校验脚本/环境可用性，获取裸响应基线 | 单用户低并发、短时 | constant-vus | vus(1~5)、duration(1~2m) | 响应时间基线、错误率 |
| 2 | 梯度压力测试 | `ramp` | 摸查性能拐点、最大承载能力 | 阶梯式加压 | ramping-vus | stages（阶梯递增） | QPS 上限、P95 拐点、报错临界点 |
| 3 | 峰值负载测试 | `peak` | 模拟线上峰值、验收生产 SLA | 峰值 1.5x 并发长时间稳压 | ramping-vus（爬坡+平台） | peakVus、rampDuration、holdDuration | 响应时间、错误率、资源全部达标 |
| 4 | 极限压力测试 | `stress` | 摸底极限性能瓶颈 | 持续加压至暴跌/超时/报错 | ramping-vus（持续递增） | startVus、stepVus、stepDuration、maxVus | 崩溃点、超时点、短板定位 |
| 5 | 稳定性/耐久测试 | `soak` | 校验长时间运行可靠性 | 固定中高并发、10min+ 长时稳压 | constant-vus（长时） | vus(中高)、duration(10m~数小时) | 内存/Goroutine 泄漏、连接不释放、GC 抖动 |
| 6 | 真实场景混合压测 | `mixed` | 贴近真实流量、消除压测失真 | 参数随机化 + 思考时间 + 冷热混合 | scenarios（多接口+权重） | scenarios、thinkTime、randomizeParams | 综合指标、失真消除 |

> baseline 场景同时承担「冒烟（脚本/环境可用性）」与「裸基线采集」两个目的；P0 以「可用性 + 裸响应基线」为主，真正的基线回归对比放 P2（见 §9）。

### 2.1 场景参数结构（`load_config` JSON）

各场景负载参数结构差异较大，统一由 `load_config jsonb` 承载，前端按场景类型渲染对应表单：

```jsonc
// baseline 基准测试
{ "vus": 3, "duration": "2m" }

// ramp 梯度压力测试
{ "stages": [
    { "duration": "1m", "target": 10 },
    { "duration": "1m", "target": 50 },
    { "duration": "1m", "target": 100 }
] }

// peak 峰值负载测试
{ "peakVus": 150, "rampDuration": "2m", "holdDuration": "30m" }

// stress 极限压力测试
{ "startVus": 10, "stepVus": 10, "stepDuration": "1m", "maxVus": 500 }

// soak 稳定性/耐久测试
{ "vus": 100, "duration": "30m" }

// mixed 真实场景混合压测
{ "scenarios": [ { "name": "首页", "weight": 0.6, "method": "GET", "url": "/" },
                 { "name": "下单", "weight": 0.4, "method": "POST", "url": "/order", "body": "..." } ],
  "thinkTime": "0.5s", "randomizeParams": true, "coldHotMix": true }
```

### 2.2 场景 → K6 脚本映射

执行时由 `scenario_type` + `load_config` 渲染 k6 `options`：

| 场景 | k6 executor | options 渲染 |
| --- | --- | --- |
| baseline / soak | `constant-vus` | `{ vus, duration }` |
| ramp / peak / stress | `ramping-vus` | `{ stages }`（由参数生成阶梯/爬坡+平台/持续递增） |
| mixed | `scenarios`（多接口按 weight 分派）+ `sleep` 思考时间 + 参数随机化 | `{ scenarios: {...} }` |

## 3. 数据模型设计

### 3.1 压测方案（`perf_test_plans`，v2 目标态）

以 `scenario_type` 取代 `load_mode`，负载参数收敛到 `load_config`，**迁移后删除旧列**（避免新旧双写与两套校验）：

```sql
alter table perf_test_plans
  add column if not exists scenario_type text not null default 'baseline';
alter table perf_test_plans
  add column if not exists load_config jsonb not null default '{}'::jsonb;
-- 数据迁移：按旧 load_mode/vus/duration/stages 语义生成 scenario_type + load_config
-- 迁移完成后：
alter table perf_test_plans drop column if exists load_mode;
alter table perf_test_plans drop column if exists vus;
alter table perf_test_plans drop column if exists duration;
alter table perf_test_plans drop column if exists stages;
```

对应删除 `performance_service.go` 中旧的 constant/ramping 校验分支（`normalizeRequest`），只保留 `scenario_type` + `load_config` 一条链路。

保留字段：`name`、`product_id`、`target_url`、`method`、`headers`、`body`、`thresholds`、`status`、`priority`、`owner`、`tags`、`description`。

### 3.2 执行记录（`perf_test_runs`，P0 增量）

```sql
alter table perf_test_runs
  add column if not exists scenario_type text not null default 'baseline';
alter table perf_test_runs
  add column if not exists exit_code int;
alter table perf_test_runs
  add column if not exists duration_ms int;
alter table perf_test_runs
  add column if not exists error_message text;
```

配套代码改动（评审补全，实施时一并处理）：
- `model.PerfTestRun` 增加 `ScenarioType string`、`ExitCode *int`、`DurationMs *int`、`ErrorMessage string` 字段（`json` tag 对应 `scenarioType/exitCode/durationMs/errorMessage`）。
- `repository.PerformanceRepository`：`scanPerfTestRun`（`performance_repository.go:240-277`）与 `CreateRun`（`:206-213`，当前只插 plan_id/status/triggered_by）覆盖上述新列；新增 `UpdateRunResult`（回填指标 + summary + exit_code + duration_ms + error_message + 终态）。
- 指标列（v1 已建，语义为 summary 的物化缓存，便于列表页展示/筛选，不重复解析大 jsonb）：`total_requests`、`avg_duration_ms`、`p95_duration_ms`、`error_rate`、`rps`。
- **P99 仅存于 `summary jsonb`，不单独加列**（报告页从 summary 解析）。

### 3.3 P1/P2 增量

- 实时指标（P1）：不落库主表，走「事件回调 + SSE」直推；`perf_test_samples` 仅为备选，P1 不建表。
- 基线（P2）：`perf_test_baselines`（plan_id、scenario_type、name、summary jsonb、created_at）；执行记录加 `baseline_id`。

## 4. 执行链路设计（K6 集成，P0 核心）

### 4.1 调度原则：机制复用、数据载体不复用

通用 `execution_runs` 调度是**用例粒度 + pass/fail 计数**模型（`case_ids bigint[]`、`execution_tasks.case_id` 外键、`dispatchCaseTask` 依赖用例详情、`aggregateRunStatus` 只统计 pass/fail），与 perf「单次运行 = 一个 k6 进程 + 指标型结果」不兼容。**不复用其数据表与聚合逻辑**，仅复用底层原语：

| 复用原语 | 位置 | 用途 |
| --- | --- | --- |
| `pickExecutor` | `execution_service.go:397` | 选在线且 `SupportedTypes` 含 `perf` 的执行器 |
| `submitToExecutor` | `execution_service.go:707` | 下发任务（独立 payload + 独立回调端点） |
| `cancelExecutorTask` | `execution_service.go:737` | 取消/停止 |
| 回调鉴权模式 | `router.go:43` | task_id 一次性凭据鉴权 |

### 4.2 链路

```
用户触发（选择方案）→ 后端创建 perf_test_runs(status=running, scenario_type)
  → pickExecutor 选在线支持 perf 的执行器（校验 k6 检测项，见 §4.4）
  → submitToExecutor 下发 { taskId, type:"perf", payload:{scenario_type,load_config,target,method,headers,body,thresholds}, callbackUrl }
  → 执行器 perf_runner 渲染 k6 脚本 → subprocess 调 k6 run → handleSummary 写 JSON
  → 读回 summary → 回调独立端点 /api/perf/tasks/:taskId/callback
  → 后端解析 k6 summary 回填 perf_test_runs 指标 + 阈值判定 + 汇总终态
  → 前端报告页展示
```

- 回调端点独立：`POST /api/perf/tasks/:taskId/callback`（鉴权沿用 task_id 一次性凭据模式）。
- 阈值判定：`summary.metrics` 中 thresholds 任一 `ok=false` → 状态 `failed`，否则 `completed`。

### 4.3 执行器改动（Python）

| 文件 | 改动 |
| --- | --- |
| `app/models/task.py` | `TaskType` 加 `perf = "perf"` |
| `app/core/config.py` | `supported_types` 加 `"perf"`；新增 perf 专用并发槽位（默认 1） |
| `app/runners/perf_runner.py`（新增） | 渲染 k6 脚本 → `subprocess.Popen` 调 k6 → 解析 handleSummary JSON → 返回结果（含进度/事件/取消支持） |
| `app/services/task_manager.py` | `_runners` 注册 perf runner + `_execute` 加分派分支 |

### 4.4 k6 检测链路（评审补全，P0 必做）

- `heartbeat_client._checks()`（`heartbeat_client.py:148-153`）增加 k6 探测：`shutil.which("k6")` 或版本检测。
- 注册/心跳上报 `supportedTypes` 含 `perf` 时，必须校验 k6 可用，否则不上报 `perf` 或标记 checks 失败。
- 后端 `pickExecutor` 调度时校验执行器 checks 含 k6 可用，不可用则拒绝并回报「执行器未安装 k6」。
- 打包版执行器需携带 k6 二进制，或文档说明安装方式；`requirements.txt` 无需新增 Python 依赖（k6 为独立二进制）。

### 4.5 结果回填

- `http_reqs.count`→`total_requests`、`http_req_duration.avg/p(95)`→耗时、`http_req_failed.rate`→`error_rate`、`http_reqs.rate`→`rps`；完整 summary 存 jsonb；`exit_code`、`duration_ms`、`error_message` 一并落库。
- k6 异常退出（脚本错误/超时/被杀）时，回填 `error_message` + 部分结果兜底，状态 `failed`。

## 5. 超时 / 取消 / 并发设计（评审补全，P0 必做）

- **超时**：perf runner 不用统一 `default_timeout_seconds=300`（`pytest_runner.py:18` 等），按 `load_config` 的 duration 动态计算超时上限（如 duration × 系数 + 缓冲），soak/peak 长时场景不再被 300s 误杀。
- **取消**：`TaskManager.cancel` 对 perf 任务改为「取消信号 → `Popen.terminate()`（宽限后 `kill()`）强杀 k6 子进程」，而非仅置标志；**取消时仍让 handleSummary 落盘已采集的部分结果**并回填，状态 `canceled` + 部分指标。
- **并发上限**：单执行器同时最多 1 个 perf 任务（perf 专用并发槽位 = 1），避免多个 k6 抢 CPU/带宽导致指标失真；队列满时任务保留待调度。

## 6. 安全与权限设计（评审补全，P0 必做）

- **目标安全**：perf 目标不做 loopback/link-local 地址限制（区别于 `ApiRunner._validate_target`，被测服务常为内网地址），但需：权限门禁 + 目标归属校验 + 并发硬上限（`load_config` 中 vus/peakVus/maxVus 设平台级上限）。
- **凭证保护**：headers 中的 token 等敏感值安全透传到 k6 脚本（运行时注入，不落明文日志/脚本文件；`summary` 与回传载荷脱敏）。
- **k6 脚本模板转义**：headers/body 拼入 JS 脚本前做转义与边界校验，防止脚本语法错误或注入。
- **权限三档**（新增权限码，实施时入 `permissionSeeds`）：
  - `perf.plan.read`：查看方案与报告
  - `perf.plan.manage`：创建/编辑/删除方案
  - `perf.plan.execute`：触发执行、取消
  - 一级菜单沿用已有 `menu.perf_test.read`；执行/管理路由挂 `RequirePermission`（对比现有 system 路由做法），压测属高危操作，不再只挂 `authed`。

## 7. 实时监控设计（P1）

- k6 `--out json` 实时 NDJSON → 执行器解析 → 事件回调推送到后端 → 前端执行中绘制 QPS / 响应时间 / 错误率 / 活跃 VU 曲线。
- 数据通道：**新建 perf 事件回调 + SSE 端点**（现有 API 调试 SSE 依赖 `api_debug_events`/`api_debug_runs` 表，perf 无对应表，需 P1 前明确，或直接复用通知中心做进度推送）。监控数据不落库，执行结束以最终 summary 为准。

## 8. 报告与通知设计

- **列表**：执行记录（方案、场景类型、状态、触发人、起止时间、耗时、关键指标）。
- **详情**：场景类型标签、聚合指标卡（总请求数、平均/P95/P99、错误率、RPS）、阈值判定（逐条通过/失败）、延迟分布与 QPS 趋势图、完整 summary JSON。
- **按场景的指标关注**：baseline 看响应时间基线与错误率；ramp 看 QPS 上限与 P95 拐点；peak 看 SLA 达标；stress 看崩溃点与短板；soak 看长时趋势与资源泄漏；mixed 看综合指标。
- **状态**：pending / running / completed / failed / canceled；failed 展示 error_message 与失败阈值。
- **通知（P0）**：执行完成/失败经通知中心触达（perf 独立调度后需自己接 notifier，复用现有通知能力）；指标劣化告警放 P2。

## 9. 基线对比设计（P2）

- 将某次「通过」的执行 summary 保存为基线（按 plan + scenario_type）。
- 新执行详情展示「本次 vs 基线」：各指标差值、变化率、劣化标红（如 P95 升幅超阈值）。
- 同方案多次执行趋势对比亦归入 P2。

## 10. 边界与非目标

- 统一走 k6，不自研压测引擎；仅 HTTP 协议级压测（区别于界面自动化）。
- mixed（多接口 scenarios）、实时监控、被测服务保护（`abortOnFail` 自动中止）、基线对比，不随 P0 落地，按阶段推进。
- **分布式压测（多执行器协同施压）砍掉，列为长期愿景**（多 k6 输出聚合复杂度极高，当前价值/成本比低，YAGNI）。
- 定时/计划触发、多环境变量（dev/staging/prod 目标切换）归入 P2。
- 不做 k6 Cloud 托管，仅本地二进制执行。

## 11. 验收标准

### P0（6 场景建模 + 5 个单接口场景真实执行）

- 平台支持以 6 大场景类型创建方案，前端按场景类型渲染对应负载表单。
- baseline / ramp / peak / stress / soak 五种场景可真实跑通 k6 压测，报告展示对应真实指标。
- 阈值不达标时执行记录状态为「失败」并指出失败阈值。
- 未安装 k6 的执行器被正确识别并拒绝（k6 检测链路生效），给出可理解提示。
- soak/peak 长时场景不被 300s 超时误杀；取消能强杀 k6 子进程并落盘部分结果。
- 单执行器同时仅 1 个 perf 任务，不相互抢资源。
- 触发执行受 `perf.plan.execute` 权限约束，非授权用户 403。
- 后端 `go build/vet/test` 通过；执行器 perf runner 单测通过；前端 build/test 通过。

### P1（mixed 场景 + 实时监控 + 被测服务保护）

- mixed 场景按多接口权重施压，支持思考时间与参数随机化。
- 执行中实时曲线随压测更新。
- 错误率阈值支持 `abortOnFail` 自动中止。

### P2（基线对比 + 趋势 + 导出 + 定时/多环境）

- 基线保存与回归对比展示指标差值；同方案多次执行趋势；报告导出；定时触发；多环境目标切换。

## 12. 分阶段实施建议

1. **阶段一（P0）**：数据模型迁移（scenario_type + load_config + 删旧列）+ 执行器 perf runner + k6 检测链路 + 超时/取消/并发设计 + 后端独立调度/回填/阈值判定 + 权限三档 + 完成/失败通知 + 报告页真实指标展示（5 个单接口场景）。
2. **阶段二（P1）**：mixed 多接口场景 + 实时监控（事件回调 + SSE + 曲线）+ 报告图表 + abortOnFail 保护。
3. **阶段三（P2）**：基线对比 + 同方案趋势 + 报告导出 + 定时触发 + 多环境目标切换。
