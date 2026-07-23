import urllib.error
from unittest.mock import Mock, patch

from app.services.heartbeat_client import HeartbeatClient
from gui import ExecutorGui


def test_heartbeat_can_restart_after_stop():
    client = HeartbeatClient(lambda: {"queuedTasks": 0, "runningTasks": 0})
    client._stop_event.set()
    loop = Mock()
    client._loop = loop

    client.start()
    client._thread.join(timeout=1)

    assert not client._stop_event.is_set()
    loop.assert_called_once()


def test_heartbeat_auth_failure_stops_and_notifies():
    callback = Mock()
    client = HeartbeatClient(lambda: {"queuedTasks": 0, "runningTasks": 0}, callback)
    error = urllib.error.HTTPError("http://platform/api/executors/heartbeat", 401, "Unauthorized", {}, None)

    with patch("urllib.request.urlopen", side_effect=error):
        status = client._post("/api/executors/heartbeat", {"executorId": "executor-a"})

    assert status == 401
    assert client._stop_event.is_set()
    callback.assert_called_once()


def test_authenticate_returns_invalid_token_message():
    client = HeartbeatClient(lambda: {"queuedTasks": 0, "runningTasks": 0})
    with patch.object(client, "_post", return_value=401):
        success, message = client.authenticate("invalid")

    assert not success
    assert message == "Token 无效或已失效"


def test_login_extracts_token_from_copied_environment_config():
    copied = "EXECUTOR_ID=local-python-executor\nEXECUTOR_SHARED_TOKEN=executor_token_123"
    assert ExecutorGui._extract_token(copied) == "executor_token_123"


def test_report_stopped_sends_offline_heartbeat():
    client = HeartbeatClient(lambda: {"queuedTasks": 0, "runningTasks": 0})
    post = Mock(return_value=200)
    client._post = post

    client.report_stopped()

    payload = post.call_args.args[1]
    assert payload["status"] == "offline"
