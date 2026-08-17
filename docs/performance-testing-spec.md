# 性能测试真实需求 SPEC

> 版本：v2.5 · 底层压测引擎：Grafana k6 · 状态：已评审
> v2.0 以 6 大场景为核心；v2.1 修正调度/超时/取消/k6 检测；v2.2 补快照/状态机/时序留存/前端架构；v2.3 补任务关联、幂等、掉线恢复与 NDJSON 兜底；v2.4 分离任务标识与回调凭据、修正外部提交竞态和幂等语义，收口 P0/P1 能力边界；v2.5 固化 dispatching 重试粘性绑定、stopping 兜底、执行器 submit 幂等与取消墓碑、回调重试。

## 1. 背景与目标

- 当前 `perf_test_plans` / `perf_test_runs` 已支持方案 CRUD 与报告数据建模，但触发执行为占位，未发生真实压测。
- 执行器（Python + FastAPI）无并发 HTTP / 压测能力，底层压测引擎选定 **Grafana k6**（独立二进制 + 子进程 + handleSummary 写 JSON）。
- 目标：按 6 大场景配置并真实执行压测，实时观察指标，得到聚合报告与阈值判定，历史报告完整还原执行现场，并支持基线回归对比。

## 2. 核心设计：6 大测试场景

性能测试方案以「场景类型（scenario_type）」为一等分类，每个场景有明确的测试目的、负载施加方式、K6 映射、关键参数与核心观察指标。

| # | 场景 | 类型 | 核心作用 | 负载方式 | K6 映射 | 关键参数 | 核心观察指标 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 基准测试 | `baseline` | 校验脚本/环境可用性，获取裸响应基线 | 单用户低并发、短时 | constant-vus | vus(1~5)、duration(1~2m) | 响应时间基线、错误率 |
| 2 | 梯度压力测试 | `ramp` | 摸查性能拐点、最大承载能力 | 阶梯式加压 | ramping-vus | stages（阶梯递增） | QPS 上限、P95 拐点、报错临界点 |
| 3 | 峰值负载测试 | `peak` | 模拟线上峰值、验收生产 SLA | 峰值 1.5x 并发长时间稳压 | ramping-vus（爬坡+平台） | peakVus、rampDuration、holdDuration | 响应时间、错误率、资源全部达标 |
| 4 | 极限压力测试 | `stress` | 摸底极限性能瓶颈 | 持续加压至暴跌/超时/报错 | ramping-vus（持续递增） | startVus、stepVus、stepDuration、maxVus | 崩溃点、超时点、短板定位 |
| 5 | 稳定性/耐久测试 | `soak` | 校验长时间运行可靠性 | 固定中高并发、10min+ 长时稳压 | constant-vus（长时） | vus(中高)、duration(10m~数小时) | 请求侧长时间趋势（资源泄漏需外部监控） |
| 6 | 真实场景混合压测 | `mixed` | 贴近真实流量、消除压测失真 | 参数随机化 + 思考时间 + 冷热混合 | scenarios（多接口+权重） | scenarios、thinkTime、randomizeParams | 综合指标、失真消除 |

> baseline 同时承担「冒烟」与「裸基线采集」；P0 以「可用性 + 裸响应基线」为主，基线回归对比放 P2。

### 2.1 场景参数结构（`load_config` JSON）

各场景负载参数结构差异较大，统一由 `load_config jsonb` 承载，前端按场景类型渲染对应表单（普通用户不手写 JSON）：

```jsonc
// baseline 基准测试
{ "vus": 3, "duration": "2m" }
// ramp 梯度压力测试
{ "stages": [ { "duration": "1m", "target": 10 }, { "duration": "1m", "target": 50 } ] }
// peak 峰值负载测试
{ "peakVus": 150, "rampDuration": "2m", "holdDuration": "30m", "rampDownDuration": "2m" }
// stress 极限压力测试
{ "startVus": 10, "stepVus": 10, "stepDuration": "1m", "maxVus": 500 }
// soak 稳定性/耐久测试
{ "vus": 100, "duration": "30m" }
// mixed 真实场景混合压测
{ "scenarios": [ { "name": "首页", "weight": 0.6, "method": "GET", "url": "/" },
                 { "name": "下单", "weight": 0.4, "method": "POST", "url": "/order", "body": "..." } ],
  "thinkTime": "0.5s", "randomizeParams": true, "coldHotMix": true }
```

**mixed 的 `weight` 语义（P1 精确定义）**：`weight` 表示业务流在**每次迭代中被随机选择的概率**，各接口 weight 之和必须等于 1；执行器生成加权随机分派函数。若需独立控制各接口吞吐量，则不使用 weight，改为为每个 scenario 配置独立 executor 与 rate/VU。

### 2.2 场景 → K6 脚本映射

| 场景 | k6 executor | options 渲染 |
| --- | --- | --- |
| baseline / soak | `constant-vus` | `{ vus, duration }` |
| ramp / peak / stress | `ramping-vus` | `{ stages }`（阶梯/爬坡+平台/持续递增） |
| mixed | `scenarios` + `sleep` 思考时间 + 参数随机化 | `{ scenarios: {...} }` |

## 3. 数据模型设计

### 3.1 压测方案（`perf_test_plans`，v2 目标态）

以 `scenario_type` 取代 `load_mode`，负载参数收敛到 `load_config`：

```sql
alter table perf_test_plans add column if not exists scenario_type text not null default 'baseline';
alter table perf_test_plans add column if not exists load_config jsonb not null default '{}'::jsonb;
-- P0 环境：枚举，仅作风险标签（多环境切换放 P2）
alter table perf_test_plans add column if not exists environment text not null default 'test';  -- test/staging/production
```

**阈值存储改为平台模型**（不再直接存 k6 thresholds 字符串，便于校验/回显/基线比较/迁移）：

```jsonc
// perf_test_plans.thresholds（平台结构化数组）
[
  { "metric": "http_req_duration", "aggregation": "p(95)", "operator": "<", "value": 500, "unit": "ms", "abortOnFail": false, "delayAbortEval": "10s" },
  { "metric": "http_req_failed",   "aggregation": "rate",  "operator": "<", "value": 0.01, "unit": "",    "abortOnFail": false }
]
// 执行器渲染为 k6 thresholds：
// { http_req_duration: [ { threshold: "p(95)<500", abortOnFail: false } ],
//   http_req_failed:   [ { threshold: "rate<0.01", abortOnFail: false } ] }
```

`abortOnFail` 字段 P0 允许持久化但强制规范化为 `false`，前端不展示「自动停止」开关；P1 再开放配置并生成 k6 `abortOnFail` / `delayAbortEval`。

保留字段：`name`、`product_id`、`target_url`、`method`、`headers`、`body`、`status`、`priority`、`owner`、`tags`、`description`。

### 3.2 执行记录（`perf_test_runs`，P0 增量）

```sql
-- 执行快照与元数据（历史报告还原“当时压了什么”）
alter table perf_test_runs add column if not exists scenario_type text not null default 'baseline';
alter table perf_test_runs add column if not exists plan_snapshot jsonb not null default '{}'::jsonb;  -- 未解析模板快照（见 §3.4 脱敏规则）
alter table perf_test_runs add column if not exists config_hash text not null default '';
alter table perf_test_runs add column if not exists executor_id text references executors(executor_id);  -- text，与 executors.executor_id 一致
alter table perf_test_runs add column if not exists executor_name text not null default '';
alter table perf_test_runs add column if not exists k6_version text not null default '';
alter table perf_test_runs add column if not exists environment text not null default 'test';
alter table perf_test_runs add column if not exists requested_at timestamptz;      -- 用户触发时间
alter table perf_test_runs add column if not exists dispatched_at timestamptz;    -- 成功提交给执行器的时间
alter table perf_test_runs add column if not exists dispatch_deadline_at timestamptz; -- 外部提交确认截止时间
alter table perf_test_runs add column if not exists start_deadline_at timestamptz;    -- 已接收任务的启动截止时间
alter table perf_test_runs add column if not exists expected_finish_at timestamptz;   -- 按负载配置计算的最晚完成时间
alter table perf_test_runs add column if not exists script_hash text not null default '';  -- 生成脚本哈希（替代脚本文本，见 §3.4）
alter table perf_test_runs add column if not exists generator_version text not null default '';
-- 任务关联与幂等（评审补全）
alter table perf_test_runs add column if not exists task_id text;                 -- 执行器侧任务标识，仅用于定位
alter table perf_test_runs add column if not exists callback_token_hash text not null default ''; -- 回调 Bearer Token 的 SHA-256 哈希（token 高强度随机，无盐）
alter table perf_test_runs add column if not exists idempotency_key text;         -- 幂等键（见 §5.3）
-- 结果与诊断
alter table perf_test_runs add column if not exists exit_code int;
alter table perf_test_runs add column if not exists duration_ms int;
alter table perf_test_runs add column if not exists error_message text;
alter table perf_test_runs add column if not exists failure_stage text not null default '';
alter table perf_test_runs add column if not exists diagnostic_output text not null default ''; -- 脱敏后最后 32 KB 输出
alter table perf_test_runs add column if not exists needs_attention boolean not null default false;
alter table perf_test_runs add column if not exists series jsonb;                 -- 降采样时序（P1，见 §7.2）
```

- **唯一约束**：
  ```sql
  create unique index if not exists uq_perf_runs_task_id on perf_test_runs(task_id);
  create unique index if not exists uq_perf_runs_idem on perf_test_runs(triggered_by, idempotency_key);
  -- 同一方案同一时间仅一个活动任务（DB 级约束，非先查后插）
  create unique index if not exists uq_perf_plan_active on perf_test_runs(plan_id) where status in ('pending','queued','dispatching','dispatched','running','stopping');
  ```
- **回调鉴权**：`task_id` 仅用于定位任务；下发执行器时另发送高强度随机 `callbackToken`，执行器回调使用 `Authorization: Bearer <callbackToken>`，后端仅存 `callback_token_hash`。进入终态后 Token 立即失效。
- 指标列（summary 的物化缓存）：`total_requests`、`avg_duration_ms`、`p95_duration_ms`、`error_rate`、`rps`；P99 仅存 summary。
- **快照写入时机**：触发时按方案当前配置生成 `plan_snapshot` + `config_hash`；执行器渲染脱敏脚本后，在启动回调中上报 `script_hash` + `generator_version` + `k6_version`。报告页读快照而非方案。
- `failure_stage` 枚为：`dispatch`、`startup`、`script_generation`、`k6_runtime`、`callback`、`timeout`、`executor_offline`、`cancel`。被测请求的 HTTP 错误/超时归入 k6 指标与阈值判定，不单独作为平台 `execution_failed`；只有执行链路本身异常才记 `execution_failed + failure_stage`。

### 3.3 状态机（固化，含 dispatching / dispatched）

**中间态**：`pending`（创建完成、尚未进入调度）→ `queued`（等待合适执行器或槽位）→ `dispatching`（已预留 task_id，正向执行器提交）→ `dispatched`（执行器已确认接收）→ `running`（执行器确认 k6 已启动）→ `stopping`（停止中）。

**终态**：`completed`（成功且阈值全通过）、`threshold_failed`（完成但性能未达标）、`execution_failed`（k6/网络/脚本/执行器异常）、`timed_out`（超时）、`canceled`（取消，含部分结果）。

**合法转移表**（所有流转用条件更新 `where status = 预期状态`；非法转移返回 409 Conflict）：

| 当前 | 可转移至 | 触发 |
| --- | --- | --- |
| pending | queued | 进入调度队列 |
| pending | canceled | 用户取消 |
| queued | dispatching | 选中执行器，持久化 task_id / 回调凭据后开始提交 |
| queued | execution_failed | 无可用执行器且重试耗尽 |
| queued | canceled | 用户取消 |
| dispatching | dispatched | 执行器确认接收 |
| dispatching | running | 执行器快速启动，启动回调早于提交响应 |
| dispatching | queued | 确认执行器未接收，清空 task_id/executor_id 后重新调度（重试需粘性绑定，见下） |
| dispatching | execution_failed | 重试耗尽且确认未执行 |
| dispatching | canceled | 用户取消 |
| dispatched | running | 执行器确认 k6 已启动 |
| dispatched | execution_failed | 执行器拒绝、确认未启动或启动超时 |
| dispatched | canceled | 用户取消 |
| running | stopping | 用户请求取消 |
| running | completed | 正常完成 + 阈值全通过 |
| running | threshold_failed | 正常完成 + 阈值未达标 |
| running | execution_failed | k6/网络/脚本异常 |
| running | timed_out | 超时 |
| stopping | canceled | k6 终止完成（含部分结果） |
| stopping | execution_failed | 宽限期确认执行器不可达，无法完成终止 |

前端统一「失败」色系，但明确展示失败类型（阈值不达标 / 执行异常 / 超时 / 取消）。

外部 HTTP 提交与本地数据库无法纳入同一事务：必须先持久化 `task_id` / `callback_token_hash` / `executor_id` 并进入 `dispatching`，再调用执行器。执行器创建任务接口以 `task_id` 幂等；提交结果不确定时先查询同 `task_id`，禁止换新 task_id 盲目重投。`dispatching` 阶段重试必须**粘性绑定同一 executor_id + 同一 task_id**（避免同一 task_id 被投到不同执行器产生多份任务）；仅当确认执行器未接收后才清空 task_id/executor_id 并回退 queued 重新调度。`dispatching` 超过 `dispatch_deadline_at` 或 `dispatched` 超过 `start_deadline_at` 时必须主动查询执行器，不允许无期挂起。

**乱序与取消补偿**：
- 启动回调先到使 `dispatching → running` 后，提交 HTTP 成功响应再到时，后端读到已是 `running` 即视为提交成功，不返回 409，不回退状态。
- 用户在 `dispatching` 中取消时，`canceled` 作为取消墓碑；无论外部 POST 后续成功还是查询发现任务已存在，后端都必须立即向同 `task_id` 发送补偿取消，不得忽略为孤儿压测。
- 执行器收到已取消 task_id 的创建/启动请求时幂等返回已取消，不再启动 k6。

### 3.4 快照与敏感数据规则（评审补全）

- 数据库仅存**变量引用**（如 `{{secret.api_token}}`），**绝不存解析后的值**。
- `plan_snapshot` 保存**未解析模板**；不设 `script_snapshot`，仅存 `script_hash + generator_version`（**不存运行时渲染脚本**）。
- 敏感值可出现在 URL Query、Cookie、Body、Basic Auth、mixed 各接口请求体，均走统一脱敏。
- 运行时 secret 通过环境变量或临时数据文件注入；临时文件权限限制为当前执行器进程可读，任务结束立即清理。
- 日志、错误信息、summary、NDJSON 统一走脱敏器。

### 3.5 数据库迁移发布顺序（评审补全）

不在一次启动迁移内「加列 → 回填 → 删列」：

1. 新增**可空**新列；
2. 回填旧数据（旧 `load_mode/vus/duration/stages` 语义生成 `scenario_type/load_config`）；
3. 校验回填数量与 JSON 合法性；
4. 新代码切换到新字段；
5. 再加 NOT NULL / 默认值 / CHECK / 索引；
6. **下一版本**删除旧列（滚动发布时不在同版本删旧列）。

## 4. 执行链路设计（K6 集成，P0 核心）

### 4.1 调度原则：机制复用、数据载体不复用

复用底层原语（`pickExecutor`/`submitToExecutor`/`cancelExecutorTask`/回调鉴权模式），不复用通用 `execution_runs` 的用例粒度数据表与 pass/fail 聚合。

### 4.2 链路

```
用户触发（带 idempotency_key）→ 幂等校验 → 生成快照 + 创建 perf_test_runs(pending, scenario_type)
  → pending → queued，调度器从 queued 任务中 pickExecutor（校验 k6 检测项）
  → 生成 task_id/callbackToken，先持久化 task_id/callback_token_hash/executor_id → 状态 dispatching
  → submitToExecutor 幂等下发 { taskId, type:"perf", payload, callbackUrl, callbackToken }
  → 执行器确认接收 → 记 dispatched_at → 状态 dispatched
  → 执行器 perf_runner 渲染 k6 脚本 → subprocess 调 k6 run → 回调状态 running + script_hash/generator_version/k6_version
  → 回调独立端点 /api/perf/tasks/:taskId/callback（幂等）
  → 后端条件更新：回填指标 + summary + 阈值判定 + 终态
```

- 回调端点：`POST /api/perf/tasks/:taskId/callback`，必须携带 Bearer callback token；`task_id` 仅定位任务，不作鉴权凭据。
- 启动回调可从 `dispatching` 或 `dispatched` 条件更新为 `running`，兼容「执行器已启动、提交 HTTP 响应尚未返回」的时序。
- 终态更新与 callback token 撤销在同一数据库事务中完成；若终态响应丢失导致执行器重试，已撤销 Token 返回 **410 Gone**，执行器将 410 视为「后端已终结」并停止重试。其他 Token 无效返回 401/403。
- 阈值判定：thresholds 任一 `ok=false` → `threshold_failed`；全通过 → `completed`。

### 4.3 执行器改动（Python）

| 文件 | 改动 |
| --- | --- |
| `app/models/task.py` | `TaskType` 加 `perf = "perf"` |
| `app/core/config.py` | `supported_types` 加 `"perf"`；perf 专用并发槽位（默认 1） |
| `app/runners/perf_runner.py`（新增） | 渲染 k6 脚本 → `subprocess.Popen` 调 k6（`--out json` 本地聚合兜底）→ 解析 handleSummary → 返回结果 |
| `app/services/task_manager.py` | `_runners` 注册 perf runner + `_execute` 分派分支；`submit` 对已存在 task_id 幂等返回现有 TaskView（不抛错）；新增「已取消 task_id 墓碑」（带 TTL），`cancel` 即使任务不存在也记墓碑返回已取消，后续创建/启动请求命中墓碑幂等返回已取消 |
| `app/api/routes.py` | `/tasks` 创建/取消对幂等命中返回 200（非 400/404） |
| `app/services/callback_client.py` | 回调带 `Authorization: Bearer <callbackToken>`；终态前指数退避重试；收 410/401/403 停止重试 |

### 4.4 k6 检测链路（P0 必做）

- `heartbeat_client._checks()` 加 `shutil.which("k6")` 探测，版本号随心跳/回调上报为 `k6_version`。
- 注册/心跳上报 `supportedTypes` 含 `perf` 时校验 k6 可用，否则不上报。
- `pickExecutor` 调度时校验执行器 checks 含 k6，不可用则拒绝并回报「执行器未安装 k6」。

## 5. 超时 / 取消 / 并发 / 可靠性设计（P0 必做）

### 5.1 超时与取消

- **超时**：perf runner 按 `load_config` duration 动态计算上限（duration × 系数 + 缓冲），超时 → `timed_out`。
- **取消分两段（评审补全）**：
  - **优雅停止**：发送优雅中断 → 等待宽限期 → k6 正常收尾并 `handleSummary` 落盘 → 读 summary 得到完整/部分结果；
  - **强制终止**：宽限期内未退出 → `Popen.terminate()`（Windows 上即强杀语义）→ `kill()`；
  - 无论哪条路径，**取消后用执行器本地已聚合的 NDJSON 采样数据兜底生成部分结果**（见 §7.1），状态 `stopping → canceled` + 部分指标。
- Windows 上 `terminate()` 不保证 k6 优雅收尾，故部分结果来源必须是**本地采样聚合**，而非依赖 handleSummary。

### 5.2 并发

- 单执行器同时最多 1 个 perf 任务（专用并发槽位 = 1）；满时任务保持 `queued`。
- 同一方案同一时间仅 1 个活动任务，由 DB 部分唯一索引 `uq_perf_plan_active` 强制（§3.2），不依赖服务层先查后插。
- `queued` 长期排队（槽位满）触发队列等待超时告警（不自动转终态，由用户取消或等待）；P0 至少提供「排队中」可见性与告警，不承诺自动超时。

### 5.3 幂等（评审补全）

- `perf_test_runs.idempotency_key` + `unique(triggered_by, idempotency_key)`。
- key 采用 UUID 且**永久唯一，不复用**，与数据库唯一索引语义一致。
- 「同请求」以 `triggered_by + idempotency_key + plan_id + config_hash` 判定；`config_hash` 对触发时的规范化 `plan_snapshot` 计算（JSON key 排序、统一数值/时长表达后 SHA-256）。同 key 同请求 → 返回原 run（**统一 HTTP 200**）；同 key 不同 plan/config → **409**。
- `triggered_by` 为现有 NOT NULL 字段；run 不提供物理删除，因此幂等墓碑与历史报告同生命周期保留。
- 前端在用户点击「确认执行」时生成 key，并在本地保留至收到成功/409；页面刷新或请求超时后复用原 key，用户明确发起新一次执行时才生成新 key。
- 并发同 key 到达时：捕获唯一索引冲突后重查判定（同请求返 200、不同 config 返 409），不直接抛 500。

### 5.4 执行器掉线与后端重启（评审补全：绝不自动重投）

- **running/stopping 任务永远不得自动重投**（压测可能仍在执行器上运行，自动重投会对目标施压双倍流量）。
- 心跳短暂中断：保留 `running` 并标注「执行器连接中断」（前端显示，不误判失败）。
- 经宽限期后：查询执行器任务状态；确认进程不存在 → 置 `execution_failed`；无法确认 → 保留原任务状态并置 `needs_attention=true`，禁止重新执行同方案，等待人工确认。
- 后端重启恢复：扫描 `dispatching/dispatched/running/stopping`。`dispatching` 按 `dispatch_deadline_at`、`dispatched` 按 `start_deadline_at`、运行任务按 `expected_finish_at` 并结合执行器任务查询恢复；`stopping` 重发补偿取消，宽限期确认执行器不可达则置 `execution_failed`，否则本地兜底置 `canceled` + `needs_attention=true`。`expected_finish_at` 由后端按规范化 `load_config` 计算总时长后加 timeout buffer；无法确认的置 `needs_attention=true`。

### 5.5 回调可靠性

- 回调同时校验 task_id + callback token；同一非终态事件重复回调幂等返回 200；终态后重试按 §4.2 返回 410；迟到/乱序回调用条件更新拒绝。执行器对终态前回调做指数退避重试，收 410/401/403 停止重试。
- 取消后到场的完成回调不得改写状态（条件更新拦截）；`stopping` 期间到达的完成回调条件更新拒绝并返回 409，执行器据 409 停止重试。
- 回调体大小上限：超限时只回传指标列 + 精简 summary（裁剪非必要 metrics 明细）；P0 不承诺另行上传完整 summary，不引入尚未定义的压缩/分片协议。
- 含 secret 的运行时脚本/注入文件在任务结束后立即清理；已脱敏的 summary/NDJSON/诊断文件按配置 TTL 清理，默认 24 小时。

## 6. 安全与权限设计（P0 必做）

- **目标安全**：perf 目标不做 loopback/link-local 限制（内网被测服务），但需权限门禁 + 目标归属校验 + 并发硬上限（vus/peakVus/maxVus 平台级上限）。
- **凭证保护**：见 §3.4（变量引用 + 运行时注入 + 统一脱敏）。
- **k6 脚本模板转义**：headers/body 拼入 JS 前转义与边界校验。
- **权限三档**：`perf.plan.read` / `perf.plan.manage` / `perf.plan.execute`；一级菜单 `menu.perf_test.read`；执行/管理路由挂 `RequirePermission`，未授权看不到执行/取消按钮。

## 7. 监控设计

### 7.1 采样采集（P0，评审补全：解决取消部分结果来源）

- **P0 即纳入最低限度 NDJSON 采集，但不做实时前端曲线**：执行器以 `--out json` 采集，本地持续聚合为短周期采样，作为「取消/中断后部分结果」的数据来源。
- 执行器按固定周期将聚合快照原子写入临时文件；正常取消或 k6 子进程强杀时可使用最近一份完整快照。如执行器主进程/主机崩溃且未产生任何完整快照，允许部分指标为空，报告必须标注「部分数据不可用」，不伪造为 0。
- P1 在此基础上前端订阅 SSE 做实时曲线。

### 7.2 时序留存（P1）

- 执行中推送原始或短周期聚合 → 后端 1~5s 降采样 → 结束后存 `perf_test_runs.series jsonb`（每项 ≤1000~3000 点，超限再降采样）。
- 报告趋势图读 `series`，而非仅聚合卡片。

### 7.3 资源监控边界（避免误导）

- k6 仅请求侧指标；内存泄漏、Goroutine 泄漏、CPU 饱和、GC 抖动、连接池耗尽等需接 Prometheus/Grafana。
- 平台仅提供请求侧证据，报告明确标注「资源侧指标需外部监控」，不给出“已定位瓶颈”的误导结论。

## 8. 前端信息架构与页面设计

### 8.1 信息架构与路由（评审补全）

```
性能测试
├── 压测方案（/performance/plans、/performance/plans/:id/edit、/performance/plans/:id）
├── 执行监控（/performance/runs?status=running，P0 可并入报告列表，提供“仅看进行中”入口）
└── 测试报告（/performance/runs、/performance/runs/:id）
```

- **明确 URL 路由**（hash 路由）：编辑页、详情页有独立 URL，刷新后恢复当前页面，不依赖弹窗内存状态。
- **P0 与 P1 页面能力分离**：P0 执行详情只展示状态、聚合指标、阈值、日志；实时指标与 SSE 状态在 P1 才上线，避免阶段一范围膨胀。
- 长时间任务补「离开页面仍继续执行」提示，允许从通知中心返回运行详情。

### 8.2 新建/编辑方案页（四区块）

1. **基本信息**：方案名称、项目/产品、测试环境（测试/预发/生产，P0 仅作风险标签）、场景类型、负责人、标签、描述。场景类型用**场景卡片**（各卡片显示适用场景、预计时长、风险等级）。
2. **请求配置**：URL、Method；Params / Headers / Body 分 Tab；Body JSON 格式化与语法校验；敏感 Header 用 `{{secret.api_token}}` 变量引用；「发送一次测试请求」冒烟按钮（见 §8.5）；显示最终 URL（敏感值脱敏）。JSON 编辑放「高级模式」。
3. **负载配置**：按场景渲染控件（baseline: VU/持续时间；ramp: 阶段表格；peak: 爬坡/峰值 VU/保持/降压；stress: 起始 VU/步长/每阶时长/最大 VU；soak: VU/持续时间；mixed: 接口列表/权重/思考时间），右侧实时负载预览（总预计时长、最大并发、预计请求量、时间轴、是否超安全限制）。
4. **阈值设置**：结构化表格（指标 / 聚合方式 / 运算符 / 阈值），模板（宽松/常规 SLA/严格/自定义），底部「查看生成的 k6 配置」。P1 再增加「自动停止」和延迟评估配置。

### 8.3 触发执行交互

- 不点按钮立即执行，先弹**执行确认面板**（目标域名与环境、场景类型、最大并发、预计时长、阈值、执行器、是否生产、是否有同方案正在执行）。
- 生产环境或高负载二次确认（输入方案名称）。
- 触发成功直接跳转执行详情。
- **P0 取消批量执行**，仅单方案执行。

### 8.4 运行中页面（P1）

- 顶部：状态、运行时长、执行器、当前 VU、当前 RPS、当前 P95、错误率、停止按钮、SSE 连接状态与重连提示。
- 图表：RPS / P50-P99 延迟 / 错误率 / 活跃 VU 曲线，共用时间轴，ramp/stress 叠加负载阶段背景。
- 下方：阈值实时状态、HTTP 状态码分布、错误 Top N、当前阶段与剩余时间。P1 「执行日志」仅表示当前 SSE 连接收到的脱敏事件流，刷新后只恢复 P0 持久化的最后 32 KB `diagnostic_output`，不保证恢复完整实时日志。
- SSE 断开显示「监控连接已断开，压测可能仍在执行」，不误判失败。

### 8.5 「发送一次测试请求」后端范围（评审补全）

- **通过所选执行器发送**（后端能访问不代表压测节点能访问），与正式压测同一网络位置与变量解析。请求使用 `type:"perf" + payload.mode:"smoke"`，不创建正式 `perf_test_runs`。
- smoke 任务占用 perf 专用槽位，禁止携带并发/持续时间参数，只发送一次请求。
- 超时限制；响应体大小限制 + 脱敏展示。
- 冒烟成功**非**执行压测的硬性必要条件，仅作前置建议。
- smoke 结果回传：不建 `perf_test_runs`、无 callback token，结果参照现有 `StartDebug` 的 taskID 轮询模式回传前端（复用 `execution_service.go` 调试链路），不新增独立回调端点。
- 新增对应 API（执行器侧 debug 端点，复用 `perf` 任务调试链路），验收见 §12。

## 9. 报告设计

- **列表**：执行记录（方案、场景类型、状态、触发人、环境、起止时间、耗时、关键指标），提供「仅看进行中」入口。
- **详情四屏**：
  1. **结论**：通过/性能未达标/执行异常（图标+文字）；场景、环境、目标、执行时间、配置版本；P95、P99、错误率、RPS、总请求数。
  2. **阈值结果**：逐条表达式、实际值、目标值、差距、通过/失败。
  3. **趋势与分布**：吞吐/延迟/错误率趋势、延迟直方图、状态码与错误分布（读 §7.2 series）。
  4. **诊断信息**：执行器与 k6 版本、退出码、失败阶段、错误信息、脱敏后最后 32 KB 输出（`diagnostic_output`）、原始 summary 折叠面板、方案执行快照（`plan_snapshot`）。P0 不承诺持久化完整日志。
- **状态展示**：终态区分五种，前端统一「失败」色系但明确失败类型。
- **通知（P0）**：执行完成/失败经通知中心触达；劣化告警放 P2。

## 10. 基线对比设计（P2）

- 将「通过」的执行 summary 存为基线（plan + scenario_type）；详情展示「本次 vs 基线」差值、变化率、劣化标红。
- 同方案多次执行趋势对比归 P2。

## 11. 边界与非目标

- 统一走 k6，仅 HTTP 协议级压测；资源侧结论依赖外部监控。
- mixed、实时前端曲线、abortOnFail 自动中止、基线对比不随 P0 落地。
- **分布式压测砍掉（长期愿景）**；定时触发、多环境目标切换归 P2。
- 不做 k6 Cloud 托管。

## 12. 验收标准

### P0（可靠性基础 + 5 单接口场景真实执行 + 结构化配置 + 聚合报告）

- 6 大场景类型创建方案，前端按场景渲染表单，普通用户无需手写 JSON 完成 5 种 P0 场景配置。
- baseline/ramp/peak/stress/soak 真实跑通 k6，报告展示真实指标。
- **快照**：触发时落 `plan_snapshot` + `config_hash`；执行器启动时回填 `script_hash` + `generator_version` + `k6_version`；改方案后旧报告仍还原旧配置。
- **状态机**：含 dispatching/dispatched 的中间态与 5 种终态；报告明确展示失败类型；非法转移 409。
- **任务关联与鉴权**：`executor_id text` 类型正确；`task_id` 唯一且仅用于定位；回调 Bearer Token 与 task_id 分离，数据库只存哈希；终态后 Token 失效。
- **无回调竞态**：下发前已持久化 task_id/callback_token_hash/executor_id；执行器以 task_id 幂等创建；快速启动回调可从 dispatching 转 running。
- **幂等**：重复点击不建多条（`unique(triggered_by,idempotency_key)`）；key 永久唯一且不复用；同 key 不同 config 409；同方案活动任务唯一。
- **可靠性**：running/stopping 不自动重投；后端重启按 expected_finish_at + 执行器查询恢复；回调幂等；取消后完成回调不改写状态。
- **取消部分结果**：取消后展示本地采样聚合的部分数据（P0 已采集 NDJSON 兜底）。
- 阈值不达标 → `threshold_failed` 并指出失败阈值；脚本/网络异常 → `execution_failed`。
- 未装 k6 执行器被识别拒绝；长时场景不被 300s 误杀；单执行器仅 1 个 perf 任务。
- P0 前端不展示自动停止，后端将 `abortOnFail` 规范化为 `false`。
- `perf.plan.execute` 权限约束；后端 go build/vet/test、执行器 perf runner 单测、前端 build/test 通过。

### P1（mixed + 实时监控 + 时序留存 + abortOnFail）

- mixed 按 weight（概率语义，总和=1）分派；实时曲线随执行更新；SSE 断线重连不误判失败；页面刷新恢复状态与订阅。
- 结束后存降采样 `series`（每项 ≤1000~3000 点），报告趋势图读 series。
- 平台支持的阈值项均可配置 `abortOnFail` 自动中止，并支持 `delayAbortEval`；错误率是默认模板中的常用项。

### P2（基线/趋势/导出/定时/多环境）

- 基线回归对比；同方案趋势；报告导出；定时触发；多环境目标切换（environment_id + base URL 解析）。

### 前端通用验收

- 切换场景后只显示对应参数，提示不兼容配置清除；非法 URL/时长/VU/阶段顺序/阈值提交前字段级提示；负载配置变化后预览实时更新。
- 执行前确认页完整展示环境/目标/最大并发/时长；生产二次确认；取消有确认提示并展示部分数据。
- 所有图表有单位、图例、空态、无障碍文本；未授权看不到执行/取消按钮。

## 13. 分阶段实施建议

1. **阶段一（P0）**：数据模型（scenario_type/load_config/快照/task_id/幂等/状态机/唯一索引，按 §3.5 迁移顺序）+ 执行器 perf runner + NDJSON 本地采样兜底 + k6 检测 + 超时/取消/并发/可靠性 + 后端独立调度/回填/阈值判定 + 权限三档 + 通知 + 前端方案编辑页（场景卡片 + 结构化负载/阈值）+ 执行确认页 + 报告详情页（结论/阈值/诊断三屏）+ 冒烟测试请求。
2. **阶段二（P1）**：mixed 多接口 + 实时监控（SSE + 运行中页面）+ 时序留存 + 报告趋势图 + abortOnFail。
3. **阶段三（P2）**：基线对比 + 趋势 + 导出 + 定时触发 + 多环境切换。
