import subprocess

from app.core.config import settings
from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


class ScriptRunner(Runner):
    """执行本地命令脚本，适合承载 Python 执行器的真实任务入口。"""

    def run(self, task: TaskCreate) -> TaskResult:
        # payload.command 必须是命令数组，避免字符串拼接带来的 shell 注入风险。
        command = task.payload.get("command")
        if not isinstance(command, list) or not command:
            raise ValueError("script 任务必须提供 payload.command 数组")

        timeout = int(task.payload.get("timeoutSeconds") or settings.default_timeout_seconds)
        completed = subprocess.run(
            [str(item) for item in command],
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        status_error = completed.stderr if completed.returncode != 0 else ""
        return TaskResult(exit_code=completed.returncode, output=completed.stdout, error=status_error)
