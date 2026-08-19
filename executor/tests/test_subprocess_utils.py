import json
import os
import subprocess
from types import SimpleNamespace
from unittest.mock import Mock

from app.core import subprocess_utils
from app.models.task import TaskCreate, TaskResult, TaskType
from app.runners.perf_runner import PerfRunner
from app.runners.playwright_runner import PlaywrightRunner
from app.runners.pytest_runner import PytestRunner
from app.runners.script_runner import ScriptRunner
from app.services.heartbeat_client import HeartbeatClient


def test_hidden_subprocess_kwargs_is_empty_on_non_windows(monkeypatch):
    monkeypatch.setattr(subprocess_utils, "os", SimpleNamespace(name="posix", environ=os.environ))
    assert subprocess_utils.hidden_subprocess_kwargs(123) == {}


def test_hidden_subprocess_kwargs_combines_no_window_and_startupinfo_on_windows(monkeypatch):
    class FakeStartupInfo:
        def __init__(self):
            self.dwFlags = 0
            self.wShowWindow = 99

    monkeypatch.setattr(subprocess_utils, "os", SimpleNamespace(name="nt", environ=os.environ))
    monkeypatch.setattr(subprocess_utils.subprocess, "CREATE_NO_WINDOW", 0x08000000, raising=False)
    monkeypatch.setattr(subprocess_utils.subprocess, "STARTUPINFO", FakeStartupInfo, raising=False)
    monkeypatch.setattr(subprocess_utils.subprocess, "STARTF_USESHOWWINDOW", 1, raising=False)
    monkeypatch.setattr(subprocess_utils.subprocess, "SW_HIDE", 0, raising=False)

    kwargs = subprocess_utils.hidden_subprocess_kwargs(0x00000200)

    assert kwargs["creationflags"] == 0x00000200 | 0x08000000
    assert isinstance(kwargs["startupinfo"], FakeStartupInfo)
    assert kwargs["startupinfo"].dwFlags == 1
    assert kwargs["startupinfo"].wShowWindow == 0


def test_find_k6_executable_prefers_path(monkeypatch, tmp_path):
    path_k6 = tmp_path / "path-k6.exe"
    path_k6.write_bytes(b"")
    fallback_k6 = tmp_path / "Program Files" / "k6" / "k6.exe"
    fallback_k6.parent.mkdir(parents=True)
    fallback_k6.write_bytes(b"")
    monkeypatch.setattr(subprocess_utils, "os", SimpleNamespace(name="nt", environ=os.environ))
    monkeypatch.setattr(subprocess_utils.shutil, "which", lambda _name: str(path_k6))
    monkeypatch.setenv("ProgramFiles", str(fallback_k6.parents[1]))

    assert subprocess_utils.find_k6_executable() == str(path_k6)


def test_find_k6_executable_uses_program_files_fallback(monkeypatch, tmp_path):
    program_files = tmp_path / "Program Files"
    k6_path = program_files / "k6" / "k6.exe"
    k6_path.parent.mkdir(parents=True)
    k6_path.write_bytes(b"")
    monkeypatch.setattr(subprocess_utils, "os", SimpleNamespace(name="nt", environ=os.environ))
    monkeypatch.setattr(subprocess_utils.shutil, "which", lambda _name: None)
    monkeypatch.setenv("ProgramFiles", str(program_files))
    monkeypatch.setenv("ProgramFiles(x86)", str(tmp_path / "missing-x86"))
    monkeypatch.setenv("LOCALAPPDATA", str(tmp_path / "missing-local"))
    monkeypatch.setenv("ChocolateyInstall", str(tmp_path / "missing-choco"))
    monkeypatch.setenv("USERPROFILE", str(tmp_path / "missing-user"))

    assert subprocess_utils.find_k6_executable() == str(k6_path)


def test_find_k6_executable_returns_none_when_candidates_are_missing(monkeypatch, tmp_path):
    monkeypatch.setattr(subprocess_utils, "os", SimpleNamespace(name="nt", environ=os.environ))
    monkeypatch.setattr(subprocess_utils.shutil, "which", lambda _name: None)
    for name in ("ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA", "ChocolateyInstall", "USERPROFILE"):
        monkeypatch.setenv(name, str(tmp_path / name))

    assert subprocess_utils.find_k6_executable() is None


def test_find_k6_executable_non_windows_keeps_path_semantics(monkeypatch, tmp_path):
    path_k6 = tmp_path / "k6"
    path_k6.write_bytes(b"")
    monkeypatch.setattr(subprocess_utils, "os", SimpleNamespace(name="posix", environ=os.environ))
    monkeypatch.setattr(subprocess_utils.shutil, "which", lambda _name: str(path_k6))

    assert subprocess_utils.find_k6_executable() == str(path_k6)


def test_heartbeat_reports_k6_and_preserves_perf_when_locator_finds_it(monkeypatch):
    monkeypatch.setattr("app.services.heartbeat_client.find_k6_executable", lambda: "C:/Program Files/k6/k6.exe")
    client = HeartbeatClient(lambda: {"queuedTasks": 0, "runningTasks": 0})

    assert client._checks()["k6"] is True
    assert "perf" in client._supported_types()


def test_heartbeat_k6_version_uses_unified_hidden_flags(monkeypatch):
    completed = Mock(stdout="k6 v2.2.0\n")
    run = Mock(return_value=completed)
    hidden = Mock(return_value={"creationflags": 7, "startupinfo": "hidden"})
    monkeypatch.setattr("app.services.heartbeat_client.find_k6_executable", lambda: "k6.exe")
    monkeypatch.setattr("app.services.heartbeat_client.subprocess.run", run)
    monkeypatch.setattr("app.services.heartbeat_client.hidden_subprocess_kwargs", hidden)

    assert HeartbeatClient._k6_version() == "k6 v2.2.0"
    hidden.assert_called_once_with()
    assert run.call_args.args == (["k6.exe", "version"],)
    assert run.call_args.kwargs["creationflags"] == 7
    assert run.call_args.kwargs["startupinfo"] == "hidden"


def test_perf_version_uses_unified_hidden_flags(monkeypatch):
    completed = Mock(stdout="k6 v2.2.0\n")
    run = Mock(return_value=completed)
    hidden = Mock(return_value={"creationflags": 9, "startupinfo": "hidden"})
    monkeypatch.setattr("app.runners.perf_runner.subprocess.run", run)
    monkeypatch.setattr("app.runners.perf_runner.hidden_subprocess_kwargs", hidden)

    assert PerfRunner._k6_version("k6.exe") == "k6 v2.2.0"
    hidden.assert_called_once_with()
    assert run.call_args.args == (["k6.exe", "version"],)
    assert run.call_args.kwargs["creationflags"] == 9
    assert run.call_args.kwargs["startupinfo"] == "hidden"


def test_perf_runner_uses_locator_path_for_version_and_execution(monkeypatch):
    path = "C:/Program Files/k6/k6.exe"
    version = Mock(return_value="k6 v2.2.0")
    run_k6 = Mock(return_value=TaskResult(exitCode=0, output="{}"))
    monkeypatch.setattr("app.runners.perf_runner.find_k6_executable", lambda: path)
    monkeypatch.setattr(PerfRunner, "_k6_version", staticmethod(version))
    monkeypatch.setattr(PerfRunner, "_run_k6", run_k6)

    result = PerfRunner().run(TaskCreate(type=TaskType.perf, payload={"scenario_type": "baseline", "load_config": {"vus": 1, "duration": "1s"}, "target": "http://example.test"}))

    assert result.exit_code == 0
    version.assert_called_once_with(path)
    assert run_k6.call_args.args[0] == path


def test_perf_popen_preserves_process_group_and_adds_hidden_flags(monkeypatch, tmp_path):
    captured = {}

    class FakeProcess:
        returncode = 0

        def wait(self, timeout=None):
            return None

        def poll(self):
            return self.returncode

    class FakeSampler:
        def __init__(self, *_args, **_kwargs):
            pass

        def start(self):
            pass

        def stop(self):
            pass

        def series(self, partial):
            return {"version": 1, "points": [], "statusCodes": {}, "errorTopN": [], "thresholds": [], "partial": partial}

    def fake_popen(*args, **kwargs):
        captured["args"] = args
        captured["kwargs"] = kwargs
        return FakeProcess()

    monkeypatch.setattr("app.runners.perf_runner.subprocess.Popen", fake_popen)
    monkeypatch.setattr("app.runners.perf_runner.NdjsonSampler", FakeSampler)
    PerfRunner()._run_k6("k6.exe", tmp_path, {}, "baseline", 5, None, None)

    if os.name == "nt":
        expected_group = getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
        expected_hidden = getattr(subprocess, "CREATE_NO_WINDOW", 0)
        assert captured["kwargs"]["creationflags"] & expected_group == expected_group
        assert captured["kwargs"]["creationflags"] & expected_hidden == expected_hidden
        assert captured["kwargs"]["startupinfo"].wShowWindow == getattr(subprocess, "SW_HIDE", 0)
    else:
        assert "creationflags" not in captured["kwargs"]
        assert "startupinfo" not in captured["kwargs"]


def test_script_runner_uses_unified_hidden_flags(monkeypatch):
    completed = Mock(returncode=0, stdout="ok", stderr="")
    run = Mock(return_value=completed)
    hidden = Mock(return_value={"creationflags": 11})
    monkeypatch.setattr("app.runners.script_runner.subprocess.run", run)
    monkeypatch.setattr("app.runners.script_runner.hidden_subprocess_kwargs", hidden)

    ScriptRunner().run(TaskCreate(type=TaskType.script, payload={"command": ["python", "-V"]}))

    hidden.assert_called_once_with()
    assert run.call_args.kwargs["creationflags"] == 11


def test_pytest_runner_uses_unified_hidden_flags(monkeypatch, tmp_path):
    completed = Mock(returncode=0, stdout="", stderr="")
    run = Mock(return_value=completed)
    hidden = Mock(return_value={"creationflags": 12})
    monkeypatch.setattr("app.runners.pytest_runner.subprocess.run", run)
    monkeypatch.setattr("app.runners.pytest_runner.hidden_subprocess_kwargs", hidden)

    PytestRunner().run(TaskCreate(type=TaskType.unit, payload={"cwd": str(tmp_path)}))

    hidden.assert_called_once_with()
    assert run.call_args.kwargs["creationflags"] == 12


def test_playwright_python_node_uses_unified_hidden_flags(monkeypatch, tmp_path):
    completed = Mock(returncode=0, stdout=json.dumps({"variables": {}, "result": {}}), stderr="")
    run = Mock(return_value=completed)
    hidden = Mock(return_value={"creationflags": 13})
    monkeypatch.setattr("app.runners.playwright_runner.subprocess.run", run)
    monkeypatch.setattr("app.runners.playwright_runner.hidden_subprocess_kwargs", hidden)

    PlaywrightRunner()._run_python({"code": "result['ok'] = True"}, {})

    hidden.assert_called_once_with()
    assert run.call_args.kwargs["creationflags"] == 13
