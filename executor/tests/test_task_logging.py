import logging
from datetime import datetime
from unittest.mock import Mock, patch

from app.models.task import TaskCreate, TaskResult, TaskType, TaskView
from app.services.callback_client import notify_callback
from app.services.task_manager import TaskManager


def test_task_manager_logs_lifecycle_and_progress(caplog):
    manager = TaskManager()
    manager._runners[TaskType.ui] = Mock()
    manager._runners[TaskType.ui].run.side_effect = lambda task, progress: (
        progress("[开始] 打开 URL"),
        progress("[开始] 打开 URL\n[完成] 打开 URL"),
        TaskResult(exitCode=0, output="[完成] 打开 URL"),
    )[-1]

    with caplog.at_level(logging.INFO):
        view = manager.submit(TaskCreate(taskId="log-task", type="ui", payload={}))
        manager._futures[view.task_id].result(timeout=2)

    messages = [record.getMessage() for record in caplog.records]
    assert any("收到任务" in message for message in messages)
    assert any("任务已进入队列" in message for message in messages)
    assert any("任务开始执行" in message for message in messages)
    assert any("[开始] 打开 URL" in message for message in messages)
    assert any("[完成] 打开 URL" in message for message in messages)
    assert any("任务执行成功" in message for message in messages)
    assert any("任务无需回调" in message for message in messages)


def test_callback_logs_success_and_failure(caplog):
    task = TaskView(
        taskId="callback-task",
        type="ui",
        status="success",
        payload={},
        callbackUrl="http://backend/callback",
        createdAt=datetime.now(),
    )

    with patch("urllib.request.urlopen") as urlopen, caplog.at_level(logging.INFO):
        urlopen.return_value.close.return_value = None
        notify_callback(task)
        urlopen.side_effect = OSError("连接失败")
        notify_callback(task)

    messages = [record.getMessage() for record in caplog.records]
    assert any("任务回调成功" in message for message in messages)
    assert any("任务回调失败" in message and "连接失败" in message for message in messages)


def test_task_manager_rejects_when_queue_is_full():
    manager = TaskManager()
    for index in range(max(1, manager._executor._max_workers * 2)):
        manager._tasks[f"active-{index}"] = TaskView(
            taskId=f"active-{index}",
            type="noop",
            status="queued",
            payload={},
            createdAt=datetime.now(),
        )

    try:
        manager.submit(TaskCreate(taskId="overflow", type="noop", payload={}))
        assert False, "队列已满时应拒绝任务"
    except ValueError as error:
        assert "队列已满" in str(error)
