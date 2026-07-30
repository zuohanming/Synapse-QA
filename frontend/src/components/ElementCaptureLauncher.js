import { useEffect, useMemo, useRef, useState } from "react";

import { elementCaptureService } from "../services/elementCaptureService.js";

const focusableSelector = [
  "button:not([disabled])",
  "a[href]",
  "input:not([disabled])",
  "select:not([disabled])",
  "[tabindex]:not([tabindex='-1'])"
].join(",");

function safeURLInput(value = "") {
  const withoutQuery = String(value).split(/[?#]/, 1)[0].trim();
  try {
    const parsed = new URL(withoutQuery);
    if (!["http:", "https:"].includes(parsed.protocol)) return withoutQuery;
    parsed.username = "";
    parsed.password = "";
    return `${parsed.origin}${parsed.pathname}`;
  } catch {
    return withoutQuery.replace(/^([a-z]+:\/\/)[^/@]*@/i, "$1");
  }
}

function validTargetURL(value) {
  try {
    return ["http:", "https:"].includes(new URL(value).protocol);
  } catch {
    return false;
  }
}

function supportsBrowserCapture(executor) {
  const types = Array.isArray(executor?.supportedTypes) ? executor.supportedTypes : [];
  return executor?.status === "online" && types.includes("ui");
}

function publicSession(session, pageRow, executor) {
  const {
    id,
    pageId,
    executorId,
    browserChannel,
    status,
    mode,
    candidateCount,
    lastHeartbeatAt,
    interruptedAt,
    recoveryExpiresAt,
    expiresAt
  } = session || {};
  return {
    id,
    pageId: pageId || pageRow?.id,
    pageName: pageRow?.name || "",
    executorId: executorId || executor?.executorId,
    executorName: executor?.name || executor?.executorId || "",
    browserChannel,
    status,
    mode,
    candidateCount,
    lastHeartbeatAt,
    interruptedAt,
    recoveryExpiresAt,
    expiresAt
  };
}

export function ElementCaptureLauncher({
  pageRow,
  executors = [],
  onStarted = () => {},
  onClose = () => {}
}) {
  const availableExecutors = useMemo(
    () => executors.filter(supportsBrowserCapture),
    [executors]
  );
  const [executorId, setExecutorId] = useState(() => (
    executors.find(supportsBrowserCapture)?.executorId || ""
  ));
  const [browserChannel, setBrowserChannel] = useState("chrome");
  const [targetURL, setTargetURL] = useState(() => safeURLInput(pageRow?.locator || ""));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const dialogRef = useRef(null);
  const closeRef = useRef(null);
  const submittingRef = useRef(false);
  const requestRef = useRef(null);
  const previousFocusRef = useRef(typeof document === "undefined" ? null : document.activeElement);

  useEffect(() => {
    if (!availableExecutors.some((item) => item.executorId === executorId)) {
      setExecutorId(availableExecutors[0]?.executorId || "");
    }
  }, [availableExecutors, executorId]);

  useEffect(() => {
    closeRef.current?.focus();
    return () => requestRef.current?.abort();
  }, []);

  function restoreFocus() {
    const previous = previousFocusRef.current;
    if (previous && typeof previous.focus === "function" && previous.isConnected) {
      previous.focus();
    }
  }

  function requestClose() {
    requestRef.current?.abort();
    restoreFocus();
    onClose();
  }

  function handleDialogKeyDown(event) {
    if (event.key === "Escape") {
      event.preventDefault();
      requestClose();
      return;
    }
    if (event.key !== "Tab") return;
    const focusable = [...(dialogRef.current?.querySelectorAll(focusableSelector) || [])];
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  async function handleSubmit(event) {
    event.preventDefault();
    if (submittingRef.current) return;
    const safeTarget = safeURLInput(targetURL);
    if (!executorId || !validTargetURL(safeTarget)) {
      setError(!executorId ? "请选择可用执行器。" : "目标 URL 必须是有效的 HTTP(S) 地址。");
      return;
    }

    submittingRef.current = true;
    setBusy(true);
    setError("");
    const controller = new AbortController();
    requestRef.current?.abort();
    requestRef.current = controller;
    try {
      const started = await elementCaptureService.create({
        pageId: pageRow?.id,
        executorId,
        browserChannel,
        mode: "pick",
        url: safeTarget
      }, { signal: controller.signal });
      const executor = availableExecutors.find((item) => item.executorId === executorId);
      onStarted(publicSession(started, pageRow, executor));
    } catch (submitError) {
      if (submitError?.name !== "AbortError") {
        setError(submitError?.message || "启动页面元素采集失败");
      }
    } finally {
      if (requestRef.current === controller) requestRef.current = null;
      submittingRef.current = false;
      setBusy(false);
    }
  }

  return (
    <div className="element-capture-launcher-backdrop">
      <section
        aria-labelledby="element-capture-launcher-title"
        aria-modal="true"
        className="element-capture-launcher"
        onKeyDown={handleDialogKeyDown}
        ref={dialogRef}
        role="dialog"
      >
        <header className="element-capture-launcher-header">
          <div>
            <span>HEADED CAPTURE</span>
            <h2 id="element-capture-launcher-title">启动页面元素采集</h2>
          </div>
          <button
            aria-label="关闭启动弹窗"
            className="element-capture-icon-button"
            onClick={requestClose}
            ref={closeRef}
            type="button"
          >
            ×
          </button>
        </header>

        <form className="element-capture-launcher-body" onSubmit={handleSubmit}>
          <div className="element-capture-launcher-page">
            <span>页面对象</span>
            <strong>{pageRow?.name || `页面 #${pageRow?.id || "-"}`}</strong>
            <code>{safeURLInput(targetURL) || "尚未配置目标 URL"}</code>
          </div>

          <label className="element-capture-field">
            <span>目标 URL（仅保留 origin/path）</span>
            <input
              aria-label="目标 URL"
              onChange={(event) => setTargetURL(safeURLInput(event.target.value))}
              placeholder="https://example.com/path"
              type="url"
              value={targetURL}
            />
          </label>

          <fieldset className="element-capture-fieldset">
            <legend>在线执行器</legend>
            {availableExecutors.length ? (
              <div className="element-capture-executor-list">
                {availableExecutors.map((executor) => (
                  <button
                    aria-pressed={executorId === executor.executorId}
                    className={executorId === executor.executorId ? "is-active" : ""}
                    key={executor.executorId}
                    onClick={() => setExecutorId(executor.executorId)}
                    type="button"
                  >
                    <span className="element-capture-online-dot" aria-hidden="true" />
                    <strong>{executor.name || executor.executorId}</strong>
                    <small>{executor.executorId} · UI</small>
                  </button>
                ))}
              </div>
            ) : (
              <div className="element-capture-empty-action">
                <strong>没有可用于有头采集的在线执行器。</strong>
                <span>启动支持 UI 能力的执行器后再试。</span>
                <a href="/config">前往执行器配置</a>
              </div>
            )}
          </fieldset>

          <fieldset className="element-capture-fieldset">
            <legend>浏览器 channel</legend>
            <div className="element-capture-segmented" role="group" aria-label="浏览器 channel">
              <button
                aria-pressed={browserChannel === "chrome"}
                className={browserChannel === "chrome" ? "is-active" : ""}
                onClick={() => setBrowserChannel("chrome")}
                type="button"
              >
                Google Chrome
              </button>
              <button
                aria-pressed={browserChannel === "msedge"}
                className={browserChannel === "msedge" ? "is-active" : ""}
                onClick={() => setBrowserChannel("msedge")}
                type="button"
              >
                Microsoft Edge
              </button>
            </div>
          </fieldset>

          <p className="element-capture-headed-note">
            将在所选执行器上打开可见浏览器窗口。启动后可在拾取与操作模式间切换。
          </p>

          {error ? <div className="element-capture-error" role="alert">{error}</div> : null}

          <footer className="element-capture-launcher-actions">
            <button className="element-capture-secondary-button" onClick={requestClose} type="button">
              取消
            </button>
            <button
              className="element-capture-primary-button"
              disabled={busy || !availableExecutors.length || !validTargetURL(targetURL)}
              type="submit"
            >
              {busy ? "正在启动…" : "启动有头采集"}
            </button>
          </footer>
        </form>
      </section>
    </div>
  );
}
