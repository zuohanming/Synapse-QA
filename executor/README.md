# Synapse QA Executor

Python 执行器只负责执行任务，不负责用户、权限、资产管理、报告归档或数据库写入。

## 职责

- 接收后端下发的执行任务。
- 根据任务类型调用对应 Runner。
- 管理本地任务生命周期。
- 采集日志、错误、耗时和执行结果。
- 回调后端上报任务状态。

## 启动

带界面的执行器：

```powershell
cd executor
python -m venv .venv
.\.venv\Scripts\pip install -r requirements.txt
.\.venv\Scripts\python gui.py
```

无界面服务入口：

```powershell
cd executor
python -m venv .venv
.\.venv\Scripts\pip install -r requirements.txt
.\.venv\Scripts\python main.py
```

默认地址：

```text
http://127.0.0.1:8090
```

## 编译 exe

```powershell
cd executor
.\build-exe.ps1
```

编译产物：

```text
dist/synapse-executor/synapse-executor.exe
```

`synapse-executor.exe` 会打开桌面界面，并在界面内启动执行器服务。界面包含服务状态、健康检查、平台地址、本地地址和运行日志。

## 平台心跳

执行器启动后会先注册，再按固定间隔向平台发送心跳。平台地址和令牌通过环境变量配置：

```text
PLATFORM_BASE_URL=http://127.0.0.1:8080
EXECUTOR_SHARED_TOKEN=请粘贴平台生成的 Token
EXECUTOR_HEARTBEAT_INTERVAL_SECONDS=10
```

Token 在平台「测试配置 / 执行器配置」中生成。生成后复制环境变量配置到执行器运行环境，再重启执行器生效。可参考 `.env.example`。

心跳会上报执行器能力、任务负载和依赖自检：

```json
{
  "executorId": "local-python-executor",
  "status": "online",
  "runningTasks": 1,
  "queuedTasks": 0,
  "supportedTypes": ["noop", "script", "api", "ui", "unit"],
  "checks": {
    "pytest": true,
    "playwright": true,
    "browser": true
  }
}
```

平台超过 30 秒未收到心跳会展示为 `suspect`，超过 60 秒展示为 `offline`。

## 接口

```text
GET  /health
POST /tasks
GET  /tasks/{task_id}
POST /tasks/{task_id}/cancel
GET  /tasks
```

## 示例任务

### 脚本任务

```json
{
  "taskId": "demo-script-1",
  "type": "script",
  "payload": {
    "command": ["python", "-c", "print('hello executor')"],
    "timeoutSeconds": 10
  },
  "callbackUrl": "http://127.0.0.1:8080/api/executor/callback"
}
```

### Pytest 单元测试任务

```json
{
  "taskId": "demo-unit-1",
  "type": "unit",
  "payload": {
    "cwd": "F:/Synapse QA",
    "paths": ["executor/tests"],
    "args": ["-q"],
    "timeoutSeconds": 60
  }
}
```

### Playwright UI 测试任务

首次使用 Playwright 前需要安装浏览器：

```powershell
cd executor
.\.venv\Scripts\python -m playwright install chromium
```

```json
{
  "taskId": "demo-ui-1",
  "type": "ui",
  "payload": {
    "browser": "chromium",
    "headless": true,
    "url": "http://127.0.0.1:4173",
    "actions": [
      {"action": "assertText", "text": "Synapse"},
      {"action": "screenshot", "name": "home.png"}
    ],
    "timeoutSeconds": 30
  }
}
```
