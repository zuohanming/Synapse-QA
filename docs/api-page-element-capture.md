# 页面元素自动采集 API

## 概述与状态机

平台创建采集会话并下发至在线 UI 执行器。执行器启动独立有头 Chromium 上下文，将用户拾取的元素作为候选回传；审核后原子保存并写入版本快照。

状态机：`starting → active ↔ interrupted → completed | expired | failed`。仅 `active` 与 `completed` 可保存；`interrupted` 等待原执行器在恢复窗口内恢复。

## 平台接口

| 方法与路径 | 权限 | 用途 |
| --- | --- | --- |
| `POST /api/ui/page-elements/capture-sessions` | `ui.element.capture` | 创建会话 |
| `GET /api/ui/page-elements/capture-sessions/:id` | `ui.element.read` | 查询会话 |
| `PATCH /api/ui/page-elements/capture-sessions/:id/mode` | `ui.element.manage` | 切换 `pick` / `operate` |
| `POST /api/ui/page-elements/capture-sessions/:id/stop` | `ui.element.capture` | 停止会话 |
| `GET /api/ui/page-elements/capture-sessions/:id/candidates` | `ui.element.read` | 按 `afterId`、`limit` 增量读取候选 |
| `PATCH /api/ui/page-elements/capture-sessions/:id/candidates/:candidateId` | `ui.element.manage` | 更新候选审核字段 |
| `POST /api/ui/page-elements/capture-sessions/:id/save` | `ui.element.manage` | 原子保存 1–200 项 |
| `GET /api/ui/page-elements/:id/versions` | `ui.element.read` | 查询元素版本 |
| `POST /api/ui/page-elements/:id/versions/:version/rollback` | `ui.element.rollback` | 以新版本回滚 |

创建请求示例：

```json
{"pageId":12,"executorId":"executor-ui-01","browserChannel":"chrome","mode":"pick","url":"https://example.test/login"}
```

保存请求示例：

```json
{"items":[{"candidateId":101,"resolution":"create"},{"candidateId":102,"resolution":"update","targetElementId":33},{"candidateId":103,"resolution":"ignore"}]}
```

HTTP `409` 的 `data.issues` 包含 `candidateId`、`field`、`message`。前端必须保留失败候选，不得把缺失或部分保存结果视为成功。

## 执行器接口

| 方法与路径 | 鉴权 | 用途 |
| --- | --- | --- |
| `GET /api/executor/element-capture/commands` | `X-Executor-ID` + `X-Executor-Token` | 租约领取命令 |
| `POST /api/executor/element-capture/commands/:id/ack` | 长期令牌 + receipt | 确认非启动命令 |
| `POST /api/executor/element-capture/:id/heartbeat` | `X-Executor-ID` + 会话 Bearer Token | 首次绑定、心跳、启动 ACK |
| `POST /api/executor/element-capture/:id/candidates` | 会话令牌 | 幂等回传候选 |
| `POST /api/executor/element-capture/:id/fail` | 会话令牌 | 报告失败 |

启动令牌仅在租约领取响应出现，数据库只保存摘要。候选用 `clientCaptureId` 做数据库级幂等；401 后执行器必须关闭对应浏览器上下文。

## 安全、容量与清理

- 仅允许在线且声明 `ui` 能力的执行器，浏览器 channel 为 Chrome 或 Edge。
- URL 仅允许 HTTP(S)，需执行 origin 白名单和 DNS/私网限制；不得记录查询参数、凭据或令牌。
- DOM 快照采用 allowlist，密码、Token、Cookie 等敏感值不得进入候选、日志和截图。
- 单会话最多 500 个候选，达到 400 个时预警；单次保存最多 200 个。
- 定位器须同时满足 `unique=true` 且质量分不低于 70；`ignore` 跳过名称和定位器门禁。
- 同指纹存在多个目标时，`update` 必须显式选择 `targetElementId`。
- 每分钟清理到期候选、过期命令和终态会话令牌摘要。截图只存于执行器临时目录，由会话关闭流程清理。

## 错误码与重试

- `400` 参数或边界无效；`401` 凭据无效；`403` 权限不足；`404` 资源不存在；`409` 状态、并发或审核门禁冲突。
- 网络错误和 `5xx` 可按 2/4/8/16/30 秒退避；401/403/404 和业务错误不得无限重试。

## 已知发布限制

真实浏览器链路仍有安全审查项：可信用户事件证明、DNS 校验与 Chromium 实际连接绑定、跨域/动态 iframe 截图遮罩，以及后台线程确定性退出。在关闭前不应发布到不受信任网络或生产环境。
