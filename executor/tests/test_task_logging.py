import logging
import json
import time
from datetime import datetime
from concurrent.futures import Future
from threading import Event
from unittest.mock import Mock, patch

from fastapi.testclient import TestClient

from app.api.routes import create_app
from app.core.config import settings
from app.models.task import TaskCreate, TaskResult, TaskStatus, TaskType, TaskView
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


def test_running_api_task_can_be_canceled():
    manager = TaskManager()
    manager._runners[TaskType.api] = Mock()

    def wait_for_cancel(_task, _progress, canceled):
        deadline = time.time() + 2
        while time.time() < deadline:
            if canceled():
                raise InterruptedError("任务已取消")
            time.sleep(0.01)
        return TaskResult(exitCode=0)

    manager._runners[TaskType.api].run.side_effect = wait_for_cancel
    view = manager.submit(TaskCreate(taskId="cancel-api", type="api", payload={"url": "https://example.com"}))
    deadline = time.time() + 1
    while manager.get(view.task_id).status != "running" and time.time() < deadline:
        time.sleep(0.01)
    canceled = manager.cancel(view.task_id)
    manager._futures[view.task_id].result(timeout=2)

    assert canceled.status == "canceled"
    assert manager.get(view.task_id).status == "canceled"


def test_task_endpoints_never_expose_callback_token():
    with TestClient(create_app()) as client:
        response = client.post("/tasks", json={"taskId": "safe-task-view", "type": "noop", "payload": {}, "callbackToken": "secret-token"})
        assert response.status_code in (200, 202)
        assert "callbackToken" not in response.json()
        detail = client.get("/tasks/safe-task-view")
        assert detail.status_code == 200
        assert "callbackToken" not in detail.json()
        listing = client.get("/tasks")
        assert all("callbackToken" not in item for item in listing.json())


def test_task_manager_prunes_terminal_tasks(monkeypatch):
    manager = TaskManager()
    monkeypatch.setattr(settings, "perf_task_ttl_seconds", 0)
    task = TaskView(taskId="expired-task", type="noop", status="success", payload={}, createdAt=datetime.now(), finishedAt=datetime.now())
    manager._tasks[task.task_id] = task
    assert manager.get(task.task_id) is None


def test_queued_cancel_blocks_runner_even_when_future_cancel_returns_false():
    manager = TaskManager()
    task = TaskView(taskId="queued-race", type=TaskType.noop, status=TaskStatus.queued, payload={}, createdAt=datetime.now())
    event = manager._cancel_events.setdefault(task.task_id, Event())
    future = Future()
    future.set_running_or_notify_cancel()
    manager._tasks[task.task_id] = task
    manager._futures[task.task_id] = future
    canceled = manager.cancel(task.task_id)
    assert canceled.status == TaskStatus.canceled
    assert event.is_set()
    assert manager._mark_running(task.task_id) is False


def test_perf_terminal_callback_is_bounded_and_keeps_point_endpoints():
    points = [{"sequence": index, "p95": 1, "statusCodes": {"200": 1}, "errorTopN": [{"message": "x"}], "thresholds": [{"metric": "x"}]} for index in range(3000)]
    task = TaskView(
        taskId="large-perf-callback",
        type=TaskType.perf,
        status=TaskStatus.success,
        payload={},
        callbackUrl="http://backend/callback",
        callbackToken="secret-token",
        result=TaskResult(exitCode=0, output=json.dumps({"terminal_status": "completed", "summary": {"metrics": {"http_reqs": {"values": {"count": 3000}}}, "large": "x" * (5 * 1024 * 1024)}, "diagnostic": "x" * 200000, "series": {"points": points}})),
        createdAt=datetime.now(),
    )
    with patch("urllib.request.urlopen") as urlopen:
        urlopen.return_value.close.return_value = None
        notify_callback(task)
    assert urlopen.call_count == 1
    data = urlopen.call_args.args[0].data
    assert len(data) <= 4 * 1024 * 1024
    payload = json.loads(data)
    bounded_points = payload["series"]["points"]
    assert bounded_points[0]["sequence"] == 0
    assert bounded_points[-1]["sequence"] == 2999
    assert all("statusCodes" not in point and "errorTopN" not in point and "thresholds" not in point for point in bounded_points)
