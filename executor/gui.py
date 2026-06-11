import logging
import queue
import sys
import threading
import tkinter as tk
import urllib.error
import urllib.request
import webbrowser
from pathlib import Path
from tkinter import ttk
from urllib.parse import urlparse


class NullWriter:
    def write(self, value: str) -> int:
        return len(value)

    def flush(self) -> None:
        return None

    def isatty(self) -> bool:
        return False


if sys.stdout is None:
    sys.stdout = NullWriter()
if sys.stderr is None:
    sys.stderr = NullWriter()


import uvicorn

from app.api.routes import create_app
from app.core.config import settings


BG = "#eef4fb"
PANEL = "#ffffff"
PANEL_SOFT = "#f8fbff"
BORDER = "#cfdced"
TEXT = "#0f1f33"
MUTED = "#64748b"
PRIMARY = "#1d4ed8"
PRIMARY_DARK = "#143a9a"
SUCCESS = "#0f9f6e"
WARNING = "#d97706"
DANGER = "#dc2626"
LOG_BG = "#0d1726"
LOG_TEXT = "#dbeafe"


class QueueLogHandler(logging.Handler):
    def __init__(self, log_queue: queue.Queue[str]) -> None:
        super().__init__()
        self.log_queue = log_queue

    def emit(self, record: logging.LogRecord) -> None:
        self.log_queue.put(self.format(record))


class ExecutorGui:
    def __init__(self) -> None:
        self.root = tk.Tk()
        self.root.title("Synapse QA 执行器")
        self.root.geometry("1080x680")
        self.root.minsize(980, 620)
        self.root.configure(bg=BG)
        self.root.option_add("*Font", ("Microsoft YaHei UI", 10))
        self.root.option_add("*TCombobox*Listbox.font", ("Microsoft YaHei UI", 10))

        self.server: uvicorn.Server | None = None
        self.server_thread: threading.Thread | None = None
        self.log_queue: queue.Queue[str] = queue.Queue()

        self.status_var = tk.StringVar(value="未启动")
        self.health_var = tk.StringVar(value="等待启动")
        self.endpoint_var = tk.StringVar(value=settings.executor_endpoint)
        self.platform_var = tk.StringVar(value=settings.platform_base_url)
        self.executor_var = tk.StringVar(value=settings.executor_id)
        self.workers_var = tk.StringVar(value=str(settings.max_workers))
        self.version_var = tk.StringVar(value=getattr(settings, "executor_version", "1.0.0"))
        self.support_var = tk.StringVar(value=" / ".join(settings.supported_types))

        self.status_badge: tk.Label | None = None
        self.health_badge: tk.Label | None = None
        self.start_button: ttk.Button | None = None
        self.stop_button: ttk.Button | None = None
        self.log_text: tk.Text | None = None

        self._setup_logging()
        self._setup_style()
        self._build_ui()
        self.root.protocol("WM_DELETE_WINDOW", self.close)
        self.root.after(300, self._drain_logs)
        self.root.after(1000, self._refresh_health)
        self.start_server()

    def _setup_logging(self) -> None:
        formatter = logging.Formatter("%(asctime)s %(levelname)s %(name)s - %(message)s", "%H:%M:%S")
        root_logger = logging.getLogger()
        root_logger.setLevel(logging.INFO)
        root_logger.handlers.clear()

        gui_handler = QueueLogHandler(self.log_queue)
        gui_handler.setFormatter(formatter)
        root_logger.addHandler(gui_handler)

        file_handler = logging.FileHandler(Path.cwd() / "executor-gui.log", encoding="utf-8")
        file_handler.setFormatter(formatter)
        root_logger.addHandler(file_handler)

    def _setup_style(self) -> None:
        style = ttk.Style()
        style.theme_use("clam")
        style.configure("TButton", padding=(14, 8), font=("Microsoft YaHei UI", 10))
        style.configure("Primary.TButton", foreground="#ffffff", background=PRIMARY, bordercolor=PRIMARY)
        style.map("Primary.TButton", background=[("active", PRIMARY_DARK), ("disabled", "#94a3b8")])
        style.configure("Danger.TButton", foreground="#ffffff", background=DANGER, bordercolor=DANGER)
        style.map("Danger.TButton", background=[("active", "#b91c1c"), ("disabled", "#fca5a5")])
        style.configure("Soft.TButton", foreground=TEXT, background="#edf4ff", bordercolor="#bdd2f3")
        style.map("Soft.TButton", background=[("active", "#dbeafe")])

    def _build_ui(self) -> None:
        self.root.columnconfigure(0, weight=1)
        self.root.rowconfigure(1, weight=1)

        header = tk.Frame(self.root, bg=PANEL, highlightthickness=1, highlightbackground=BORDER)
        header.grid(row=0, column=0, sticky="ew", padx=14, pady=(14, 10))
        header.columnconfigure(0, weight=1)

        title_block = tk.Frame(header, bg=PANEL)
        title_block.grid(row=0, column=0, sticky="w", padx=18, pady=16)
        tk.Label(title_block, text="Synapse QA 执行器", bg=PANEL, fg=TEXT, font=("Microsoft YaHei UI", 20, "bold")).grid(row=0, column=0, sticky="w")
        tk.Label(title_block, text="本地任务执行、平台心跳上报、运行状态监控", bg=PANEL, fg=MUTED).grid(row=1, column=0, sticky="w", pady=(4, 0))

        actions = tk.Frame(header, bg=PANEL)
        actions.grid(row=0, column=1, sticky="e", padx=18, pady=16)
        self.start_button = ttk.Button(actions, text="启动服务", style="Primary.TButton", command=self.start_server)
        self.start_button.grid(row=0, column=0, padx=(0, 8))
        self.stop_button = ttk.Button(actions, text="停止服务", style="Danger.TButton", command=self.stop_server)
        self.stop_button.grid(row=0, column=1, padx=(0, 8))
        ttk.Button(actions, text="打开地址", style="Soft.TButton", command=lambda: webbrowser.open(settings.executor_endpoint)).grid(row=0, column=2)

        content = tk.Frame(self.root, bg=BG)
        content.grid(row=1, column=0, sticky="nsew", padx=14, pady=(0, 14))
        content.columnconfigure(0, weight=3)
        content.columnconfigure(1, weight=2)
        content.rowconfigure(1, weight=1)

        status_panel = self._panel(content)
        status_panel.grid(row=0, column=0, sticky="ew", padx=(0, 10), pady=(0, 10))
        status_panel.columnconfigure(0, weight=1)
        status_panel.columnconfigure(1, weight=1)
        status_panel.columnconfigure(2, weight=1)
        self._metric_card(status_panel, 0, "服务状态", self.status_var, "执行器 API 服务")
        self._metric_card(status_panel, 1, "健康检查", self.health_var, "本地 /health 探测")
        self._metric_card(status_panel, 2, "最大并发", self.workers_var, "任务并行执行数")

        info_panel = self._panel(content)
        info_panel.grid(row=0, column=1, sticky="ew", pady=(0, 10))
        info_panel.columnconfigure(1, weight=1)
        self._section_title(info_panel, "连接信息")
        self._info_row(info_panel, 1, "执行器 ID", self.executor_var)
        self._info_row(info_panel, 2, "本地地址", self.endpoint_var)
        self._info_row(info_panel, 3, "平台地址", self.platform_var)
        self._info_row(info_panel, 4, "版本", self.version_var)

        log_panel = self._panel(content)
        log_panel.grid(row=1, column=0, sticky="nsew", padx=(0, 10))
        log_panel.columnconfigure(0, weight=1)
        log_panel.rowconfigure(1, weight=1)
        self._section_title(log_panel, "运行日志")
        self.log_text = tk.Text(
            log_panel,
            height=20,
            wrap="word",
            state="disabled",
            bg=LOG_BG,
            fg=LOG_TEXT,
            insertbackground=LOG_TEXT,
            relief="flat",
            padx=12,
            pady=10,
            font=("Consolas", 10),
        )
        self.log_text.grid(row=1, column=0, sticky="nsew", padx=14, pady=(0, 14))
        scrollbar = ttk.Scrollbar(log_panel, command=self.log_text.yview)
        scrollbar.grid(row=1, column=1, sticky="ns", pady=(0, 14), padx=(0, 14))
        self.log_text.configure(yscrollcommand=scrollbar.set)

        capability_panel = self._panel(content)
        capability_panel.grid(row=1, column=1, sticky="nsew")
        capability_panel.columnconfigure(0, weight=1)
        self._section_title(capability_panel, "执行能力")
        self._capability_item(capability_panel, 1, "支持任务", self.support_var.get())
        self._capability_item(capability_panel, 2, "默认超时", f"{settings.default_timeout_seconds} 秒")
        self._capability_item(capability_panel, 3, "产物目录", settings.artifacts_dir)
        self._capability_item(capability_panel, 4, "心跳间隔", f"{settings.heartbeat_interval_seconds} 秒")

        hint = tk.Label(
            capability_panel,
            text="关闭窗口会停止本地执行器服务。运行异常时可查看 executor-gui.log。",
            bg=PANEL,
            fg=MUTED,
            wraplength=360,
            justify="left",
        )
        hint.grid(row=5, column=0, sticky="ew", padx=14, pady=(16, 14))

        self._apply_state_badges()

    def _panel(self, parent: tk.Widget) -> tk.Frame:
        return tk.Frame(parent, bg=PANEL, highlightthickness=1, highlightbackground=BORDER)

    def _section_title(self, parent: tk.Frame, text: str) -> None:
        tk.Label(parent, text=text, bg=PANEL, fg=TEXT, font=("Microsoft YaHei UI", 12, "bold")).grid(
            row=0, column=0, sticky="w", padx=14, pady=(14, 10)
        )

    def _metric_card(self, parent: tk.Frame, column: int, label: str, value: tk.StringVar, desc: str) -> None:
        card = tk.Frame(parent, bg=PANEL_SOFT, highlightthickness=1, highlightbackground="#dbe7f5")
        card.grid(row=0, column=column, sticky="ew", padx=(14 if column == 0 else 0, 14), pady=14)
        card.columnconfigure(0, weight=1)
        tk.Label(card, text=label, bg=PANEL_SOFT, fg=MUTED).grid(row=0, column=0, sticky="w", padx=12, pady=(10, 0))
        badge = tk.Label(card, textvariable=value, bg="#e0ecff", fg=PRIMARY, font=("Microsoft YaHei UI", 15, "bold"), padx=10, pady=4)
        badge.grid(row=1, column=0, sticky="w", padx=12, pady=(6, 4))
        tk.Label(card, text=desc, bg=PANEL_SOFT, fg=MUTED).grid(row=2, column=0, sticky="w", padx=12, pady=(0, 10))
        if label == "服务状态":
            self.status_badge = badge
        elif label == "健康检查":
            self.health_badge = badge

    def _info_row(self, parent: tk.Frame, row: int, label: str, value: tk.StringVar) -> None:
        tk.Label(parent, text=label, bg=PANEL, fg=MUTED).grid(row=row, column=0, sticky="w", padx=14, pady=7)
        tk.Label(parent, textvariable=value, bg=PANEL, fg=TEXT, font=("Microsoft YaHei UI", 10, "bold")).grid(
            row=row, column=1, sticky="ew", padx=(8, 14), pady=7
        )

    def _capability_item(self, parent: tk.Frame, row: int, label: str, value: str) -> None:
        item = tk.Frame(parent, bg=PANEL_SOFT, highlightthickness=1, highlightbackground="#e2e8f0")
        item.grid(row=row, column=0, sticky="ew", padx=14, pady=(0, 10))
        item.columnconfigure(0, weight=1)
        tk.Label(item, text=label, bg=PANEL_SOFT, fg=MUTED).grid(row=0, column=0, sticky="w", padx=12, pady=(8, 0))
        tk.Label(item, text=value, bg=PANEL_SOFT, fg=TEXT, wraplength=340, justify="left").grid(row=1, column=0, sticky="w", padx=12, pady=(3, 8))

    def _server_host_port(self) -> tuple[str, int]:
        parsed = urlparse(settings.executor_endpoint)
        host = parsed.hostname or "127.0.0.1"
        port = parsed.port or 8090
        return host, port

    def _set_status(self, value: str) -> None:
        self.status_var.set(value)
        self._apply_state_badges()

    def _set_health(self, value: str) -> None:
        self.health_var.set(value)
        self._apply_state_badges()

    def _apply_state_badges(self) -> None:
        status = self.status_var.get()
        health = self.health_var.get()
        self._configure_badge(self.status_badge, status)
        self._configure_badge(self.health_badge, health)
        if self.start_button and self.stop_button:
            running = self.server_thread is not None and self.server_thread.is_alive()
            self.start_button.state(["disabled"] if running else ["!disabled"])
            self.stop_button.state(["!disabled"] if running else ["disabled"])

    def _configure_badge(self, badge: tk.Label | None, value: str) -> None:
        if badge is None:
            return
        if value in {"运行中", "正常"}:
            badge.configure(bg="#dff8ed", fg=SUCCESS)
        elif value in {"启动中", "停止中", "等待启动"}:
            badge.configure(bg="#fff3d6", fg=WARNING)
        elif value in {"未启动", "已停止", "不可达"} or value.startswith("异常"):
            badge.configure(bg="#fee2e2", fg=DANGER)
        else:
            badge.configure(bg="#e0ecff", fg=PRIMARY)

    def start_server(self) -> None:
        if self.server_thread and self.server_thread.is_alive():
            self._append_log("服务已经在运行。")
            return
        self._set_status("启动中")
        self._set_health("等待启动")
        host, port = self._server_host_port()
        app = create_app()
        config = uvicorn.Config(app, host=host, port=port, reload=False, log_level="info", access_log=True)
        self.server = uvicorn.Server(config)
        self.server_thread = threading.Thread(target=self._run_server, name="synapse-executor-server", daemon=True)
        self.server_thread.start()
        self._append_log(f"正在启动执行器服务：{settings.executor_endpoint}")

    def _run_server(self) -> None:
        try:
            if self.server:
                self.server.run()
        except Exception:
            logging.exception("执行器服务启动失败")
        finally:
            self.root.after(0, lambda: self._set_status("已停止"))

    def stop_server(self) -> None:
        if not self.server:
            self._set_status("已停止")
            return
        self._set_status("停止中")
        self.server.should_exit = True
        self._append_log("正在停止执行器服务。")

    def _refresh_health(self) -> None:
        try:
            with urllib.request.urlopen(settings.executor_endpoint.rstrip("/") + "/health", timeout=1.5) as response:
                if response.status == 200:
                    self._set_status("运行中")
                    self._set_health("正常")
                else:
                    self._set_health(f"异常 HTTP {response.status}")
        except (urllib.error.URLError, TimeoutError, OSError):
            if self.status_var.get() not in {"启动中", "停止中"}:
                self._set_health("不可达")
        self.root.after(3000, self._refresh_health)

    def _append_log(self, message: str) -> None:
        self.log_queue.put(message)

    def _drain_logs(self) -> None:
        if self.log_text is None:
            return
        while True:
            try:
                message = self.log_queue.get_nowait()
            except queue.Empty:
                break
            self.log_text.configure(state="normal")
            self.log_text.insert("end", message + "\n")
            self.log_text.see("end")
            self.log_text.configure(state="disabled")
        self.root.after(300, self._drain_logs)

    def close(self) -> None:
        self.stop_server()
        self.root.after(500, self.root.destroy)

    def run(self) -> None:
        self.root.mainloop()


if __name__ == "__main__":
    ExecutorGui().run()
