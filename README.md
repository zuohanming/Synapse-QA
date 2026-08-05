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

## 文档

| 文档 | 说明 |
| --- | --- |
| [产品需求总纲（PRD）](docs/PRD.md) | 产品定位、信息架构、全局规则、验收基线 |
| [模块文档索引](docs/modules/README.md) | 按二级菜单拆分的 25 份业务模块文档 |
| [AI 全流程自动化设计方案](docs/AI全流程自动化设计方案.md) | AI 自然语言驱动测试全流程的专项方案 |
| [技术文档](docs/technical/) | 后端架构规划、API 设计、分层说明等 |

## 目录

```text
backend/   Go Gin 后端
frontend/  React 前端
executor/  Python 执行器
docs/      产品与模块文档、技术文档、每日更新
```
