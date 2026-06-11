from app.models.task import TaskCreate, TaskResult
from app.runners.base import Runner


class NoopRunner(Runner):
    def run(self, task: TaskCreate) -> TaskResult:
        return TaskResult(exit_code=0, output="noop task completed")
