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

from app.api.routes import configure_gui_callbacks, create_app, heartbeat_client
from app.core.config import save_executor_token, settings


BG = "#f7f7f5"
PANEL = "#ffffff"
PANEL_SOFT = "#f4f4f2"
BORDER = "#deded9"
TEXT = "#20201d"
MUTED = "#73736e"
PRIMARY = "#20201d"
PRIMARY_DARK = "#080807"
SUCCESS = "#16865c"
WARNING = "#b66a13"
DANGER = "#c13c35"
LOG_BG = "#fbfbfa"
LOG_TEXT = "#292926"
SIDEBAR = "#efefec"
SIDEBAR_MUTED = "#6f6f69"


def resource_path(relative_path: str) -> Path:
    base_path = Path(getattr(sys, "_MEIPASS", Path(__file__).resolve().parent))
    return base_path / relative_path


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
        self.root.geometry("1180x740")
        self.root.minsize(1020, 640)
        self.root.configure(bg=BG)
        self._window_icon: tk.PhotoImage | None = None
        icon_png_path = resource_path("assets/executor-icon.png")
        if icon_png_path.exists():
            self._window_icon = tk.PhotoImage(file=str(icon_png_path))
            self.root.iconphoto(True, self._window_icon)
        icon_path = resource_path("assets/executor-icon.ico")
        if icon_path.exists():
            self.root.iconbitmap(default=str(icon_path))
        self.root.option_add("*Font", ("Microsoft YaHei UI", 10))
        self.root.option_add("*TCombobox*Listbox.font", ("Microsoft YaHei UI", 10))

        self.server: uvicorn.Server | None = None
        self.server_thread: threading.Thread | None = None
        self.log_queue: queue.Queue[str] = queue.Queue()
        self.auth_result_queue: queue.Queue[tuple[bool, str]] = queue.Queue()

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
        self.login_token_var = tk.StringVar(value="")
        self.login_message_var = tk.StringVar(value="请输入执行器专属 Token")
        self.login_button: ttk.Button | None = None
        self.authenticated = False
        self.capture_active = False

        self._setup_logging()
        self._setup_style()
        self._build_login_ui()
        self.root.protocol("WM_DELETE_WINDOW", self.close)

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
        style.configure("Soft.TButton", foreground=TEXT, background="#ffffff", bordercolor=BORDER)
        style.map("Soft.TButton", background=[("active", "#eeeeea")])

    def _build_ui(self) -> None:
        for child in self.root.winfo_children():
            child.destroy()
        self.root.columnconfigure(0, weight=0)
        self.root.columnconfigure(1, weight=1)
        self.root.rowconfigure(0, weight=1)
        self.root.rowconfigure(1, weight=0)

        sidebar = tk.Frame(self.root, width=220, bg=SIDEBAR, highlightthickness=1, highlightbackground=BORDER)
        sidebar.grid(row=0, column=0, sticky="nsw")
        sidebar.grid_propagate(False)
        sidebar.columnconfigure(0, weight=1)
        brand = tk.Frame(sidebar, bg=SIDEBAR)
        brand.grid(row=0, column=0, sticky="ew", padx=18, pady=(22, 28))
        tk.Label(brand, text=">_", bg=SIDEBAR, fg=TEXT, font=("Consolas", 17, "bold")).grid(row=0, column=0, sticky="w")
        tk.Label(brand, text="Synapse Executor", bg=SIDEBAR, fg=TEXT, font=("Segoe UI", 12, "bold")).grid(row=0, column=1, sticky="w", padx=(9, 0))

        nav = tk.Frame(sidebar, bg=SIDEBAR)
        nav.grid(row=1, column=0, sticky="ew", padx=10)
        self._sidebar_item(nav, 0, "●", "运行概览", True)
        self._sidebar_item(nav, 1, "≡", "任务日志")
        self._sidebar_item(nav, 2, "⚙", "执行配置")

        identity = tk.Frame(sidebar, bg=PANEL, highlightthickness=1, highlightbackground=BORDER)
        identity.grid(row=3, column=0, sticky="sew", padx=10, pady=12)
        sidebar.rowconfigure(2, weight=1)
        tk.Label(identity, text="CONNECTED EXECUTOR", bg=PANEL, fg=SIDEBAR_MUTED, font=("Segoe UI", 8, "bold")).grid(row=0, column=0, sticky="w", padx=12, pady=(11, 3))
        tk.Label(identity, textvariable=self.executor_var, bg=PANEL, fg=TEXT, font=("Consolas", 9)).grid(row=1, column=0, sticky="w", padx=12)
        tk.Label(identity, textvariable=self.platform_var, bg=PANEL, fg=SIDEBAR_MUTED, font=("Consolas", 8)).grid(row=2, column=0, sticky="w", padx=12, pady=(3, 11))

        workspace = tk.Frame(self.root, bg=BG)
        workspace.grid(row=0, column=1, sticky="nsew")
        workspace.columnconfigure(0, weight=1)
        workspace.rowconfigure(2, weight=1)

        header = tk.Frame(workspace, bg=BG)
        header.grid(row=0, column=0, sticky="ew", padx=26, pady=(22, 18))
        header.columnconfigure(0, weight=1)
        title_block = tk.Frame(header, bg=BG)
        title_block.grid(row=0, column=0, sticky="w")
        tk.Label(title_block, text="执行器", bg=BG, fg=TEXT, font=("Microsoft YaHei UI", 20, "bold")).grid(row=0, column=0, sticky="w")
        tk.Label(title_block, text="本地任务运行与连接状态", bg=BG, fg=MUTED).grid(row=1, column=0, sticky="w", pady=(3, 0))
        actions = tk.Frame(header, bg=BG)
        actions.grid(row=0, column=1, sticky="e")
        self.start_button = ttk.Button(actions, text="启动服务", style="Primary.TButton", command=self.start_server)
        self.start_button.grid(row=0, column=0, padx=(0, 8))
        self.stop_button = ttk.Button(actions, text="停止", style="Soft.TButton", command=self.stop_server)
        self.stop_button.grid(row=0, column=1, padx=(0, 8))
        ttk.Button(actions, text="打开地址", style="Soft.TButton", command=lambda: webbrowser.open(settings.executor_endpoint)).grid(row=0, column=2)

        status_panel = self._panel(workspace)
        status_panel.grid(row=1, column=0, sticky="ew", padx=26, pady=(0, 14))
        status_panel.columnconfigure(0, weight=1)
        status_panel.columnconfigure(1, weight=1)
        status_panel.columnconfigure(2, weight=1)
        self._metric_card(status_panel, 0, "服务状态", self.status_var, "执行器 API 服务")
        self._metric_card(status_panel, 1, "健康检查", self.health_var, "本地 /health 探测")
        self._metric_card(status_panel, 2, "最大并发", self.workers_var, "任务并行执行数")

        content = tk.Frame(workspace, bg=BG)
        content.grid(row=2, column=0, sticky="nsew", padx=26, pady=(0, 24))
        content.columnconfigure(0, weight=5)
        content.columnconfigure(1, weight=2)
        content.rowconfigure(0, weight=1)
        log_panel = self._panel(content)
        log_panel.grid(row=0, column=0, sticky="nsew", padx=(0, 14))
        log_panel.columnconfigure(0, weight=1)
        log_panel.rowconfigure(1, weight=1)
        self._section_title(log_panel, "任务终端")
        self.log_text = tk.Text(
            log_panel,
            height=20,
            wrap="word",
            state="disabled",
            bg=LOG_BG,
            fg=LOG_TEXT,
            insertbackground=LOG_TEXT,
            selectbackground="#deded9",
            selectforeground=TEXT,
            relief="flat",
            padx=14,
            pady=13,
            font=("Consolas", 10),
        )
        self.log_text.grid(row=1, column=0, sticky="nsew", padx=12, pady=(0, 12))
        scrollbar = ttk.Scrollbar(log_panel, command=self.log_text.yview)
        scrollbar.grid(row=1, column=1, sticky="ns", pady=(0, 12), padx=(0, 12))
        self.log_text.configure(yscrollcommand=scrollbar.set)
        capability_panel = self._panel(content)
        capability_panel.grid(row=0, column=1, sticky="nsew")
        capability_panel.columnconfigure(0, weight=1)
        self._section_title(capability_panel, "运行环境")
        self._capability_item(capability_panel, 1, "执行器 ID", self.executor_var.get())
        self._capability_item(capability_panel, 2, "本地地址", self.endpoint_var.get())
        self._capability_item(capability_panel, 3, "支持任务", self.support_var.get())
        self._capability_item(capability_panel, 4, "默认超时", f"{settings.default_timeout_seconds} 秒")
        self._capability_item(capability_panel, 5, "版本", self.version_var.get())

        self._apply_state_badges()

    def _sidebar_item(self, parent: tk.Frame, row: int, icon: str, text: str, active: bool = False) -> None:
        background = PANEL if active else SIDEBAR
        item = tk.Frame(parent, bg=background, highlightthickness=1 if active else 0, highlightbackground=BORDER)
        item.grid(row=row, column=0, sticky="ew", pady=2)
        tk.Label(item, text=icon, width=3, bg=background, fg=TEXT if active else SIDEBAR_MUTED, font=("Consolas", 10)).grid(row=0, column=0, padx=(7, 0), pady=9)
        tk.Label(item, text=text, bg=background, fg=TEXT if active else SIDEBAR_MUTED, font=("Microsoft YaHei UI", 9, "bold" if active else "normal")).grid(row=0, column=1, sticky="w", padx=(4, 10))

    def _build_login_ui(self, message: str = "请输入执行器专属 Token") -> None:
        self.authenticated = False
        self.login_message_var.set(message)
        for child in self.root.winfo_children():
            child.destroy()
        self.root.columnconfigure(0, weight=1)
        self.root.rowconfigure(1, weight=0)
        self.root.rowconfigure(0, weight=1)

        shell = tk.Frame(self.root, bg=BG)
        shell.grid(row=0, column=0, sticky="nsew")
        shell.columnconfigure(0, weight=1)
        shell.rowconfigure(0, weight=1)
        card = tk.Frame(shell, bg=PANEL, width=440, height=390, highlightthickness=1, highlightbackground=BORDER)
        card.grid(row=0, column=0)
        card.grid_propagate(False)
        card.columnconfigure(0, weight=1)

        mark = tk.Label(card, text=">_", width=3, height=1, bg=PRIMARY, fg="#ffffff", font=("Consolas", 16, "bold"))
        mark.grid(row=0, column=0, pady=(38, 18))
        tk.Label(card, text="连接执行器", bg=PANEL, fg=TEXT, font=("Microsoft YaHei UI", 20, "bold")).grid(row=1, column=0)
        tk.Label(card, text=f"执行器：{settings.executor_id}", bg=PANEL, fg=MUTED).grid(row=2, column=0, pady=(7, 24))

        field = tk.Frame(card, bg=PANEL)
        field.grid(row=3, column=0, sticky="ew", padx=48)
        field.columnconfigure(0, weight=1)
        tk.Label(field, text="专属 Token", bg=PANEL, fg=TEXT, font=("Microsoft YaHei UI", 10, "bold")).grid(row=0, column=0, sticky="w", pady=(0, 7))
        token_entry = ttk.Entry(field, textvariable=self.login_token_var, show="●", font=("Consolas", 11))
        token_entry.grid(row=1, column=0, sticky="ew", ipady=6)
        token_entry.bind("<Return>", lambda _event: self.connect_platform())
        token_entry.focus_set()

        self.login_button = ttk.Button(card, text="连接平台", style="Primary.TButton", command=self.connect_platform)
        self.login_button.grid(row=4, column=0, sticky="ew", padx=48, pady=(18, 10))
        message_label = tk.Label(card, textvariable=self.login_message_var, bg=PANEL, fg=MUTED, wraplength=350)
        message_label.grid(row=5, column=0, padx=48)

    def connect_platform(self) -> None:
        token = self._extract_token(self.login_token_var.get())
        if not token:
            self.login_message_var.set("Token 不能为空")
            return
        if self.login_button:
            self.login_button.state(["disabled"])
        self.login_message_var.set("正在验证 Token 并连接平台…")
        threading.Thread(target=self._authenticate, args=(token,), name="executor-login", daemon=True).start()
        self.root.after(100, self._poll_auth_result)

    @staticmethod
    def _extract_token(value: str) -> str:
        text = value.strip()
        for line in text.splitlines():
            if line.strip().startswith("EXECUTOR_SHARED_TOKEN="):
                return line.split("=", 1)[1].strip()
        return text

    def _authenticate(self, token: str) -> None:
        try:
            success, message = heartbeat_client.authenticate(token)
            if success:
                save_executor_token(token)
            self.auth_result_queue.put((success, message))
        except Exception as error:
            logging.exception("执行器连接平台失败")
            self.auth_result_queue.put((False, f"连接失败：{error}"))

    def _poll_auth_result(self) -> None:
        try:
            success, message = self.auth_result_queue.get_nowait()
        except queue.Empty:
            if self.login_button and "disabled" in self.login_button.state():
                self.root.after(100, self._poll_auth_result)
            return
        if success:
            self._show_main()
        else:
            self._login_failed(message)

    def _login_failed(self, message: str) -> None:
        self.login_message_var.set(message)
        if self.login_button:
            self.login_button.state(["!disabled"])

    def _show_main(self) -> None:
        self.authenticated = True
        configure_gui_callbacks(
            self.queue_capture_status,
            lambda: self.root.after(0, self._auth_expired),
        )
        self._build_ui()
        self.authenticated = True
        self.root.after(300, self._drain_logs)
        self.root.after(1000, self._refresh_health)
        self.start_server()

    def _auth_expired(self) -> None:
        if not self.authenticated:
            return
        self.authenticated = False
        self.stop_server()
        self.login_token_var.set("")
        self._build_login_ui("Token 已失效，请输入新的专属 Token 重新连接")

    def _panel(self, parent: tk.Widget) -> tk.Frame:
        return tk.Frame(parent, bg=PANEL, highlightthickness=1, highlightbackground=BORDER)

    def _section_title(self, parent: tk.Frame, text: str) -> None:
        tk.Label(parent, text=text, bg=PANEL, fg=TEXT, font=("Microsoft YaHei UI", 12, "bold")).grid(
            row=0, column=0, sticky="w", padx=14, pady=(14, 10)
        )

    def _metric_card(self, parent: tk.Frame, column: int, label: str, value: tk.StringVar, desc: str) -> None:
        card = tk.Frame(parent, bg=PANEL, highlightthickness=0)
        card.grid(row=0, column=column, sticky="ew", padx=(14 if column == 0 else 0, 14), pady=14)
        card.columnconfigure(0, weight=1)
        tk.Label(card, text=label, bg=PANEL_SOFT, fg=MUTED).grid(row=0, column=0, sticky="w", padx=12, pady=(10, 0))
        badge = tk.Label(card, textvariable=value, bg=PANEL_SOFT, fg=TEXT, font=("Microsoft YaHei UI", 15, "bold"), padx=10, pady=4)
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
        item = tk.Frame(parent, bg=PANEL_SOFT, highlightthickness=0)
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

    def queue_capture_status(self, active: bool, page_title: str) -> None:
        """供采集轮询线程调用，把 Tk 状态更新切回主线程。"""
        self.root.after(0, self._apply_capture_status, active, page_title)

    def _apply_capture_status(self, active: bool, page_title: str) -> None:
        self.capture_active = active
        if active:
            self._set_status(f"正在采集 · {page_title or '页面元素'}")
        elif self.server_thread is not None and self.server_thread.is_alive():
            self._set_status("运行中")
        else:
            self._set_status("已停止")

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
        if value in {"运行中", "正常"} or value.startswith("正在采集 · "):
            badge.configure(bg="#e6f4ed", fg=SUCCESS)
        elif value in {"启动中", "停止中", "等待启动"}:
            badge.configure(bg="#f7efe3", fg=WARNING)
        elif value in {"未启动", "已停止", "不可达"} or value.startswith("异常"):
            badge.configure(bg="#f8e9e7", fg=DANGER)
        else:
            badge.configure(bg=PANEL_SOFT, fg=TEXT)

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
        heartbeat_client.report_stopped()
        self.server.should_exit = True
        self._append_log("正在停止执行器服务。")

    def _refresh_health(self) -> None:
        if not self.authenticated:
            return
        try:
            with urllib.request.urlopen(settings.executor_endpoint.rstrip("/") + "/health", timeout=1.5) as response:
                if response.status == 200:
                    if not self.capture_active:
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
        if self.log_text is None or not self.authenticated:
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
