#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB_NAME="synapse_qa"
API_PORT="8080"
WEB_PORT="4173"
EXECUTOR_PORT="8090"

: "${PGPASSWORD:?请先设置 PGPASSWORD，例如 export PGPASSWORD=your_password}"
export PGPASSWORD

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

to_windows_path() {
  if command_exists wslpath; then
    wslpath -w "$1"
  elif command_exists cygpath; then
    cygpath -w "$1"
  else
    printf '%s' "$1"
  fi
}

health_ok() {
  curl -fsS "$1" >/dev/null 2>&1
}

ensure_database() {
  if ! command_exists psql || ! command_exists createdb; then
    if command_exists powershell.exe; then
      echo "当前 shell 未找到 psql/createdb，改用 start.ps1 启动。"
      exec powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(to_windows_path "$ROOT_DIR/start.ps1")"
    fi
    echo "未找到 psql/createdb，请先安装 PostgreSQL 客户端并确保已加入 PATH。"
    exit 1
  fi

  db_exists="$(psql -h 127.0.0.1 -U postgres -d postgres -tAc "select 1 from pg_database where datname='${DB_NAME}'" 2>/dev/null | tr -d '[:space:]')"
  if [ "$db_exists" != "1" ]; then
    createdb -h 127.0.0.1 -U postgres "$DB_NAME"
  fi
}

start_api() {
  if health_ok "http://127.0.0.1:${API_PORT}/api/health"; then
    return
  fi

  ensure_database

  (
    cd "$ROOT_DIR/backend"
    export DATABASE_URL="postgres://postgres:${PGPASSWORD}@127.0.0.1:5432/${DB_NAME}?sslmode=disable"
    export API_ADDR="127.0.0.1:${API_PORT}"
    if ! command_exists go; then
      echo "未找到 go，无法构建后端。"
      exit 1
    fi
    go build -o synapse-api ./cmd/api
    ./synapse-api
  ) >"$ROOT_DIR/backend/api.log" 2>&1 &
}

start_web() {
  if health_ok "http://127.0.0.1:${WEB_PORT}/"; then
    return
  fi

  if ! command_exists npm; then
    echo "未找到 npm，无法启动 React 前端。"
    exit 1
  fi

  (
    cd "$ROOT_DIR/frontend"
    if [ ! -d "node_modules" ]; then
      npm install
    fi
    npm run dev -- --host 127.0.0.1 --port "$WEB_PORT"
  ) >"$ROOT_DIR/web.log" 2>&1 &
}

start_executor() {
  if health_ok "http://127.0.0.1:${EXECUTOR_PORT}/health"; then
    return
  fi

  if ! command_exists python3 && ! command_exists python; then
    echo "未找到 python3/python，无法启动执行器。"
    exit 1
  fi

  (
    cd "$ROOT_DIR/executor"
    py="python"
    if command_exists python3; then
      py="python3"
    fi
    if [ ! -f ".venv/bin/python" ] && [ ! -f ".venv/Scripts/python.exe" ]; then
      "$py" -m venv .venv
    fi
    if [ -f ".venv/bin/python" ]; then
      venv_py=".venv/bin/python"
    else
      venv_py=".venv/Scripts/python.exe"
    fi
    "$venv_py" -m pip install -r requirements.txt
    "$venv_py" main.py
  ) >"$ROOT_DIR/executor.log" 2>&1 &
}

start_api
start_web
start_executor

sleep 5

if ! health_ok "http://127.0.0.1:${API_PORT}/api/health"; then
  echo "API 启动失败，请查看 backend/api.log。"
  exit 1
fi

if ! health_ok "http://127.0.0.1:${WEB_PORT}/"; then
  echo "Web 启动失败，请查看 web.log。"
  exit 1
fi

if ! health_ok "http://127.0.0.1:${EXECUTOR_PORT}/health"; then
  echo "执行器启动失败，请查看 executor.log。"
  exit 1
fi

echo "API: http://127.0.0.1:${API_PORT}"
echo "Web: http://127.0.0.1:${WEB_PORT}"
echo "Executor: http://127.0.0.1:${EXECUTOR_PORT}"
echo "默认账号：admin / admin123"
