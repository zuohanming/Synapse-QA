import os
import shutil
import subprocess
from pathlib import Path


def hidden_subprocess_kwargs(creationflags: int = 0) -> dict:
    """返回跨平台的隐藏子进程参数；非 Windows 返回空字典。"""
    if os.name != "nt":
        return {}

    flags = int(creationflags or 0)
    no_window = int(getattr(subprocess, "CREATE_NO_WINDOW", 0) or 0)
    if no_window:
        flags |= no_window

    kwargs = {"creationflags": flags} if flags else {}
    startup_info_type = getattr(subprocess, "STARTUPINFO", None)
    if startup_info_type is not None:
        startup_info = startup_info_type()
        startup_info.dwFlags |= int(getattr(subprocess, "STARTF_USESHOWWINDOW", 0) or 0)
        startup_info.wShowWindow = int(getattr(subprocess, "SW_HIDE", 0) or 0)
        kwargs["startupinfo"] = startup_info
    return kwargs


def find_k6_executable() -> str | None:
    """定位 k6；优先遵循 PATH，Windows 桌面环境再检查常见安装目录。"""
    path_value = shutil.which("k6")
    if path_value and Path(path_value).is_file():
        return path_value
    if os.name != "nt":
        return None

    environment = os.environ
    candidates = [
        _under(environment.get("ProgramFiles"), "k6", "k6.exe"),
        _under(environment.get("ProgramFiles(x86)"), "k6", "k6.exe"),
        _under(environment.get("LOCALAPPDATA"), "Microsoft", "WinGet", "Links", "k6.exe"),
        _under(environment.get("ChocolateyInstall"), "bin", "k6.exe"),
        _under(environment.get("USERPROFILE"), "scoop", "shims", "k6.exe"),
        _under(environment.get("USERPROFILE"), "scoop", "apps", "k6", "current", "k6.exe"),
    ]
    for candidate in candidates:
        if candidate and Path(candidate).is_file():
            return candidate
    return None


def _under(root: str | None, *parts: str) -> str | None:
    if not root:
        return None
    return str(Path(root).joinpath(*parts))
