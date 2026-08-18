import os
import sys


def _ensure_playwright_browsers_path() -> None:
    """修正 PyInstaller 打包后 Playwright 找不到浏览器的问题。

    Playwright 在 frozen（PyInstaller/Nuitka）环境下会把 PLAYWRIGHT_BROWSERS_PATH
    默认置为 "0"，从而到驱动目录下的 .local-browsers 查找浏览器；而浏览器实际安装在
    用户级 ms-playwright 缓存中，导致启动浏览器报
    「Looks like Playwright was just installed or updated」。
    这里在 frozen 且未显式指定路径时，提前指向用户级缓存，避免该报错。
    """
    if not getattr(sys, "frozen", False):
        return
    if os.environ.get("PLAYWRIGHT_BROWSERS_PATH"):
        return
    if os.name == "nt":
        base = os.environ.get("LOCALAPPDATA") or os.path.join(os.path.expanduser("~"), "AppData", "Local")
    else:
        base = os.environ.get("XDG_CACHE_HOME") or os.path.join(os.path.expanduser("~"), ".cache")
    os.environ["PLAYWRIGHT_BROWSERS_PATH"] = os.path.join(base, "ms-playwright")


_ensure_playwright_browsers_path()


def _environment_value(name: str, default: str) -> str:
    value = os.getenv(name)
    if value:
        return value
    if os.name == "nt":
        try:
            import winreg

            with winreg.OpenKey(winreg.HKEY_CURRENT_USER, "Environment") as key:
                stored, _ = winreg.QueryValueEx(key, name)
                if stored:
                    return str(stored)
        except OSError:
            pass
    return default


def _csv_values(name: str) -> tuple[str, ...]:
    return tuple(item.strip() for item in _environment_value(name, "").split(",") if item.strip())


def save_executor_token(token: str) -> None:
    os.environ["EXECUTOR_SHARED_TOKEN"] = token
    settings.executor_shared_token = token
    if os.name == "nt":
        import winreg

        with winreg.CreateKey(winreg.HKEY_CURRENT_USER, "Environment") as key:
            winreg.SetValueEx(key, "EXECUTOR_SHARED_TOKEN", 0, winreg.REG_SZ, token)


class Settings:
    app_name = "Synapse QA Executor"
    executor_id = os.getenv("EXECUTOR_ID", "local-python-executor")
    executor_name = os.getenv("EXECUTOR_NAME", "本地 Python 执行器")
    executor_version = os.getenv("EXECUTOR_VERSION", "1.0.0")
    executor_endpoint = _environment_value("EXECUTOR_ENDPOINT", "http://127.0.0.1:8090")
    executor_shared_token = _environment_value("EXECUTOR_SHARED_TOKEN", "synapse-local-executor-token")
    platform_base_url = _environment_value("PLATFORM_BASE_URL", "http://127.0.0.1:8080")
    heartbeat_interval_seconds = int(os.getenv("EXECUTOR_HEARTBEAT_INTERVAL_SECONDS", "10"))
    capture_command_poll_interval_seconds = 2
    capture_allowed_origins = _csv_values("CAPTURE_ALLOWED_ORIGINS")
    capture_allowed_browser_channels = frozenset({"chrome", "msedge"})
    capture_pending_max = int(os.getenv("CAPTURE_PENDING_MAX", "100"))
    capture_rate_limit_per_second = int(os.getenv("CAPTURE_RATE_LIMIT_PER_SECOND", "5"))
    max_workers = int(os.getenv("EXECUTOR_MAX_WORKERS", "2"))
    default_timeout_seconds = int(os.getenv("EXECUTOR_DEFAULT_TIMEOUT_SECONDS", "300"))
    artifacts_dir = os.getenv("EXECUTOR_ARTIFACTS_DIR", "artifacts")
    supported_types = ["noop", "script", "api", "api_case", "ui", "unit", "perf"]
    # 性能测试专用并发槽位：单执行器同时最多 1 个 perf 任务（SPEC §5.2）。
    perf_max_concurrent = int(os.getenv("EXECUTOR_PERF_MAX_CONCURRENT", "1"))
    # 性能测试超时 = load_config 总时长 × 系数 + 缓冲（SPEC §5.1），不固定 300s。
    perf_timeout_coefficient = float(os.getenv("EXECUTOR_PERF_TIMEOUT_COEFFICIENT", "1.5"))
    perf_timeout_buffer_seconds = int(os.getenv("EXECUTOR_PERF_TIMEOUT_BUFFER_SECONDS", "60"))
    # 优雅停止宽限期：发中断信号后等待 k6 收尾的时间。
    perf_grace_period_seconds = int(os.getenv("EXECUTOR_PERF_GRACE_PERIOD_SECONDS", "10"))
    # 本地 NDJSON 采样聚合快照的写入周期。
    perf_snapshot_interval_seconds = float(os.getenv("EXECUTOR_PERF_SNAPSHOT_INTERVAL_SECONDS", "5"))
    # NDJSON 延迟/失败窗口的最大采样点数，避免长任务内存无限增长。
    perf_ndjson_window_points = int(os.getenv("EXECUTOR_PERF_NDJSON_WINDOW_POINTS", "5000"))
    # 已取消 task_id 墓碑的存活时间（幂等返回已取消）。
    perf_cancel_tombstone_ttl_seconds = int(os.getenv("EXECUTOR_PERF_CANCEL_TOMBSTONE_TTL_SECONDS", "3600"))
    perf_task_ttl_seconds = int(os.getenv("EXECUTOR_PERF_TASK_TTL_SECONDS", "600"))
    perf_task_max_retained = int(os.getenv("EXECUTOR_PERF_TASK_MAX_RETAINED", "1000"))
    # 终态回调重试策略（SPEC §5.5）。
    callback_max_attempts = int(os.getenv("EXECUTOR_CALLBACK_MAX_ATTEMPTS", "5"))
    callback_retry_base_delay_seconds = float(os.getenv("EXECUTOR_CALLBACK_RETRY_BASE_DELAY_SECONDS", "1"))


settings = Settings()
