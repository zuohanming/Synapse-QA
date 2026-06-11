# Synapse QA

自动化测试平台工程，当前采用：

- 后端：Go + Gin，按 `Router -> Controller -> Service -> Repository -> Model` 分层。
- 前端：React + Vite，源码位于 `frontend/`。
- 执行器：Python + FastAPI，源码位于 `executor/`。
- 数据库：PostgreSQL。

## 一键启动

```powershell
.\start.ps1
```

或：

```bash
bash ./start.sh
```

启动脚本会：

- 确认 PostgreSQL 数据库 `synapse_qa` 存在。
- 构建并启动 Gin 后端 API。
- 安装前端依赖并启动 React 开发服务。
- 安装执行器依赖并启动 Python 执行器。

## 地址

```text
前端：http://127.0.0.1:4173/
后端：http://127.0.0.1:8080/
执行器：http://127.0.0.1:8090/
```

## 默认账号

```text
用户名：admin
密码：admin123
```

## 目录

```text
backend/   Go Gin 后端
frontend/  React 前端
executor/  Python 执行器
docs/      架构和分层文档
```
