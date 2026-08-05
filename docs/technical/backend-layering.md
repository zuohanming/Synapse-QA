# 后端分层规范

后端采用 Gin，并遵循 `Router -> Controller -> Service -> Repository -> Model` 分层。

## 分层职责

```text
cmd/api/main.go
  应用装配入口：连接数据库、执行迁移、注入依赖、启动 HTTP 服务。

cmd/api/bootstrap.go
  启动期数据库迁移和本地种子数据，不承载请求处理逻辑。

internal/router/
  只负责注册路由和挂载中间件，不写业务逻辑。

internal/controller/
  只负责 HTTP 入参、鉴权上下文、状态码和统一 JSON 响应。

internal/service/
  负责业务规则，例如登录校验、token 签发、参数默认值、删除约束、操作日志。

internal/repository/
  负责数据库读写，禁止夹带 HTTP 和复杂业务判断。

internal/model/
  放请求、响应和领域数据结构。
```

## 迁移状态

旧的 `legacy_handlers.go` 已删除，所有 API 均已迁移到分层结构：

- 认证：登录、注册、当前用户
- 系统管理：概览、用户、角色、菜单、字典、操作日志
- 测试配置：项目、产品、测试对象
- UI 自动化：页面资产、步骤、用例、全局变量、页面元素

## 开发约束

- 新接口必须先定义 Model，再写 Repository、Service、Controller，最后在 Router 注册。
- Controller 不允许直接使用 `database/sql`。
- Repository 不允许引用 Gin 或 `net/http`。
- Service 不允许读取 `gin.Context`，只接收普通参数和 `context.Context`。
- `cmd/api/main.go` 不承载业务逻辑，只做依赖注入和启动。
- `cmd/api/bootstrap.go` 只做迁移和种子数据，不新增业务接口。
