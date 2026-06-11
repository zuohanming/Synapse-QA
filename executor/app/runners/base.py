from abc import ABC, abstractmethod

from app.models.task import TaskCreate, TaskResult


class Runner(ABC):
    @abstractmethod
    def run(self, task: TaskCreate) -> TaskResult:
        raise NotImplementedError
