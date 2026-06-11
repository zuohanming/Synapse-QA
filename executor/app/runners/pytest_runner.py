import subprocess
import sys
from pathlib import Path

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


class PytestRunner(Runner):
    """运行 Pytest 测试任务，负责收集标准输出和退出码。"""

    def run(self, task: TaskCreate) -> TaskResult:
        payload = task.payload
        paths = payload.get("paths") or []
        args = payload.get("args") or []
        cwd = Path(payload.get("cwd") or Path.cwd()).resolve()
        timeout = int(payload.get("timeoutSeconds") or settings.default_timeout_seconds)

        if not isinstance(paths, list):
            raise ValueError("unit 任务的 payload.paths 必须是数组")
        if not isinstance(args, list):
            raise ValueError("unit 任务的 payload.args 必须是数组")
        if not cwd.exists():
            raise ValueError(f"unit 任务工作目录不存在: {cwd}")

        command = [sys.executable, "-m", "pytest", *[str(item) for item in paths], *[str(item) for item in args]]
        completed = subprocess.run(
            command,
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        error = completed.stderr if completed.returncode != 0 else ""
        return TaskResult(exit_code=completed.returncode, output=completed.stdout, error=error)
