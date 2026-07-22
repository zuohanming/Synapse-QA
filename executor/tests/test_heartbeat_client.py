from unittest.mock import Mock

from app.services.heartbeat_client import HeartbeatClient


def test_heartbeat_can_restart_after_stop():
    client = HeartbeatClient(lambda: {"queuedTasks": 0, "runningTasks": 0})
    client._stop_event.set()
    loop = Mock()
    client._loop = loop

    client.start()
    client._thread.join(timeout=1)

    assert not client._stop_event.is_set()
    loop.assert_called_once()
