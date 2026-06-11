# Synapse QA Backend

## 执行器心跳

后端提供执行器注册和心跳接口：

```text
POST /api/executors/register
POST /api/executors/heartbeat
GET  /api/executors
```

执行器调用注册和心跳接口时需要携带：

```text
X-Executor-Token: synapse-local-executor-token
```

生产环境请通过平台「测试配置 / 执行器配置」生成共享令牌，再将生成结果配置到执行器环境变量：

```text
EXECUTOR_SHARED_TOKEN=平台生成的 Token
```

平台按 `lastHeartbeatAt` 判断状态：30 秒以上为 `suspect`，60 秒以上为 `offline`。

后端采用 Go + Gin，并遵循 `Router -> Controller -> Service -> Repository -> Model` 分层。

## 启动

```powershell
.\start-api.ps1
```

或：

```powershell
go run ./cmd/api
```

默认地址：

```text
http://127.0.0.1:8080
```

## 环境变量

```text
DATABASE_URL=postgres://postgres:<password>@127.0.0.1:5432/synapse_qa?sslmode=disable
API_ADDR=127.0.0.1:8080
JWT_SECRET=synapse-local-dev-secret
```

## 结构

```text
cmd/api/                 启动入口和数据库迁移
internal/router/         路由注册
internal/controller/     HTTP 控制器
internal/service/        业务逻辑
internal/repository/     数据访问
internal/model/          请求、响应和领域结构
```
