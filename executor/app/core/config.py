import os


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


class Settings:
    app_name = "Synapse QA Executor"
    executor_id = os.getenv("EXECUTOR_ID", "local-python-executor")
    executor_name = os.getenv("EXECUTOR_NAME", "本地 Python 执行器")
    executor_version = os.getenv("EXECUTOR_VERSION", "1.0.0")
    executor_endpoint = os.getenv("EXECUTOR_ENDPOINT", "http://127.0.0.1:8090")
    executor_shared_token = _environment_value("EXECUTOR_SHARED_TOKEN", "synapse-local-executor-token")
    platform_base_url = os.getenv("PLATFORM_BASE_URL", "http://127.0.0.1:8080")
    heartbeat_interval_seconds = int(os.getenv("EXECUTOR_HEARTBEAT_INTERVAL_SECONDS", "10"))
    max_workers = int(os.getenv("EXECUTOR_MAX_WORKERS", "2"))
    default_timeout_seconds = int(os.getenv("EXECUTOR_DEFAULT_TIMEOUT_SECONDS", "300"))
    artifacts_dir = os.getenv("EXECUTOR_ARTIFACTS_DIR", "artifacts")
    supported_types = ["noop", "script", "api", "ui", "unit"]


settings = Settings()
