import { useEffect, useMemo, useRef, useState } from "react";

import { elementCaptureService } from "../services/elementCaptureService.js";

const terminalStatuses = new Set(["completed", "expired", "failed"]);
const focusableSelector = [
  "button:not([disabled])",
  "a[href]",
  "input:not([disabled])",
  "select:not([disabled])",
  "[tabindex]:not([tabindex='-1'])"
].join(",");

const statusLabels = {
  starting: "浏览器启动中",
  active: "采集中",
  interrupted: "执行器连接中断，正在等待原机恢复",
  completed: "会话已停止，候选仍需保存或忽略。",
  expired: "会话已过期，已接收候选仍可审核。",
  failed: "浏览器采集失败，请检查执行器后重新启动。"
};

const qualitySegments = [
  { key: "all", label: "全部" },
  { key: "ready", label: "可保存" },
  { key: "unnamed", label: "待命名" },
  { key: "unreliable", label: "定位不可靠" },
  { key: "conflict", label: "冲突" }
];

function parseLocators(raw) {
  if (Array.isArray(raw)) return raw.slice(0, 3);
  try {
    const parsed = JSON.parse(raw || "[]");
    return Array.isArray(parsed) ? parsed.slice(0, 3) : [];
  } catch {
    return [];
  }
}

function uniqueTargetIDs(candidate) {
  const source = candidate?.conflictTargetIds
    || candidate?.duplicateElementIds
    || candidate?.conflictTargets?.map((item) => item.id)
    || [];
  const values = [...source];
  if (candidate?.duplicateElementId) values.push(candidate.duplicateElementId);
  return [...new Set(values.map(Number).filter((value) => value > 0))].sort((left, right) => left - right);
}

function safeCandidate(candidate) {
  const cursorId = Number(candidate?.cursorId || candidate?.candidateId || 0);
  const conflictTargetIds = uniqueTargetIDs(candidate);
  const conflictResolution = candidate?.conflictResolution || "";
  const defaultTarget = Number(candidate?.targetElementId || 0)
    || (conflictResolution === "update" && conflictTargetIds.length === 1 ? conflictTargetIds[0] : 0);
  return {
    cursorId,
    name: String(candidate?.name || ""),
    serverName: String(candidate?.serverName ?? candidate?.name ?? ""),
    tagName: String(candidate?.tagName || ""),
    accessibleName: String(candidate?.accessibleName || ""),
    locators: parseLocators(candidate?.locators),
    qualityScore: Number(candidate?.qualityScore || 0),
    duplicateElementId: Number(candidate?.duplicateElementId || 0),
    conflictTargetIds,
    conflictStatus: String(candidate?.conflictStatus || ""),
    conflictResolution,
    serverConflictResolution: String(candidate?.serverConflictResolution ?? conflictResolution),
    targetElementId: defaultTarget,
    status: String(candidate?.status || "pending"),
    candidateCount: Number(candidate?.candidateCount || 0),
    warning: String(candidate?.warning || "")
  };
}

export function mergeCaptureCandidates(current, incoming) {
  const merged = new Map(
    (current || []).map((item) => {
      const safe = safeCandidate(item);
      return [safe.cursorId, safe];
    })
  );
  (incoming || []).forEach((item) => {
    const safe = safeCandidate(item);
    if (safe.cursorId > 0 && safe.status === "pending") merged.set(safe.cursorId, safe);
  });
  return [...merged.values()].sort((left, right) => left.cursorId - right.cursorId);
}

export function getPollDelay(failureCount) {
  return Math.min(30000, 2000 * (2 ** Math.max(0, Number(failureCount) || 0)));
}

function isUnnamed(candidate) {
  const name = candidate?.name?.trim();
  return !name || name === "未命名元素";
}

function hasReliableLocator(candidate) {
  return (candidate?.locators || []).some((locator) => (
    locator?.unique === true && Number(locator?.score) >= 70
  ));
}

function hasInvalidUpdateTarget(candidate) {
  if (candidate?.conflictResolution !== "update") return false;
  const targets = uniqueTargetIDs(candidate);
  const selected = Number(candidate?.targetElementId || 0);
  if (targets.length > 1 && !selected) return true;
  if (selected && !targets.includes(selected)) return true;
  return targets.length === 0;
}

function qualityKey(candidate) {
  if (isUnnamed(candidate)) return "unnamed";
  if (!hasReliableLocator(candidate)) return "unreliable";
  if (
    candidate?.conflictStatus === "duplicate"
    && (!candidate?.conflictResolution || hasInvalidUpdateTarget(candidate))
  ) return "conflict";
  return "ready";
}

export function getSaveBlockers(candidates, selectedIDs) {
  const selected = (candidates || []).filter((item) => selectedIDs?.has(item.cursorId));
  if (!selected.length) return ["请选择需要保存的候选"];
  const blockers = [];
  if (selected.length > 200) blockers.push("一次最多保存 200 个候选");
  const unnamed = selected.filter(isUnnamed).length;
  const unreliable = selected.filter((item) => !hasReliableLocator(item)).length;
  const unresolved = selected.filter((item) => (
    item.conflictStatus === "duplicate" && !item.conflictResolution
  )).length;
  const invalidTargets = selected.filter(hasInvalidUpdateTarget).length;
  if (unnamed) blockers.push(`${unnamed} 个候选尚未命名`);
  if (unreliable) blockers.push(`${unreliable} 个候选缺少评分不低于 70 的唯一定位器`);
  if (unresolved) blockers.push(`${unresolved} 个候选尚未处理冲突`);
  if (invalidTargets) blockers.push(`${invalidTargets} 个更新候选需要选择目标元素`);
  return blockers;
}

function publicSession(session) {
  return {
    id: session?.id,
    pageId: session?.pageId,
    pageName: session?.pageName || "",
    executorId: session?.executorId || "",
    executorName: session?.executorName || "",
    browserChannel: session?.browserChannel || "",
    status: session?.status || "starting",
    mode: session?.mode || "pick",
    candidateCount: Number(session?.candidateCount || 0),
    interruptedAt: session?.interruptedAt,
    recoveryExpiresAt: session?.recoveryExpiresAt,
    expiresAt: session?.expiresAt
  };
}

function buildSaveItem(candidate) {
  const resolution = candidate.conflictStatus === "duplicate"
    ? candidate.conflictResolution
    : "create";
  const item = { candidateId: candidate.cursorId, resolution };
  if (resolution === "update" && candidate.targetElementId) {
    item.targetElementId = Number(candidate.targetElementId);
  }
  return item;
}

export function ElementCaptureDrawer({
  session,
  onClose = () => {},
  onSaved = () => {}
}) {
  const [sessionState, setSessionState] = useState(() => publicSession(session));
  const [candidates, setCandidates] = useState([]);
  const [selectedIDs, setSelectedIDs] = useState(() => new Set());
  const [filter, setFilter] = useState("all");
  const [loading, setLoading] = useState(true);
  const [disconnected, setDisconnected] = useState(session?.status === "interrupted");
  const [pollNotice, setPollNotice] = useState("");
  const [actionError, setActionError] = useState("");
  const [candidateIssues, setCandidateIssues] = useState({});
  const [saving, setSaving] = useState(false);
  const dialogRef = useRef(null);
  const closeRef = useRef(null);
  const previousFocusRef = useRef(typeof document === "undefined" ? null : document.activeElement);
  const generationRef = useRef(0);
  const cursorRef = useRef(0);
  const timerRef = useRef(null);
  const pollAbortRef = useRef(null);
  const actionControllersRef = useRef(new Set());
  const dirtyRef = useRef(new Set());
  const rowRefs = useRef(new Map());

  function abortAllRequests() {
    if (timerRef.current) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    pollAbortRef.current?.abort();
    pollAbortRef.current = null;
    actionControllersRef.current.forEach((controller) => controller.abort());
    actionControllersRef.current.clear();
  }

  useEffect(() => {
    closeRef.current?.focus();
  }, []);

  useEffect(() => {
    const sessionID = session?.id;
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    abortAllRequests();
    cursorRef.current = 0;
    dirtyRef.current.clear();
    setSessionState(publicSession(session));
    setCandidates([]);
    setSelectedIDs(new Set());
    setCandidateIssues({});
    setFilter("all");
    setLoading(true);
    setDisconnected(session?.status === "interrupted");
    setPollNotice("");
    let stopped = false;
    let failures = 0;

    const schedule = (delay) => {
      if (stopped || generationRef.current !== generation) return;
      timerRef.current = window.setTimeout(poll, delay);
    };

    const poll = async () => {
      if (!sessionID || stopped || generationRef.current !== generation) return;
      const controller = new AbortController();
      pollAbortRef.current?.abort();
      pollAbortRef.current = controller;
      const afterID = cursorRef.current;
      const [detailResult, candidateResult] = await Promise.allSettled([
        elementCaptureService.get(sessionID, { signal: controller.signal }),
        elementCaptureService.candidates(sessionID, {
          afterId: afterID,
          limit: 100,
          signal: controller.signal
        })
      ]);
      if (stopped || controller.signal.aborted || generationRef.current !== generation) return;

      const rejected = [detailResult, candidateResult].find((result) => result.status === "rejected");
      if (rejected) {
        if (rejected.reason?.name !== "AbortError") {
          failures += 1;
          setDisconnected(true);
          setPollNotice(`暂时无法连接采集服务，${getPollDelay(failures) / 1000} 秒后重试；已有候选不会清空。`);
          setLoading(false);
          schedule(getPollDelay(failures));
        }
        return;
      }

      failures = 0;
      const detail = publicSession({ ...session, ...detailResult.value });
      const incoming = Array.isArray(candidateResult.value) ? candidateResult.value : [];
      setSessionState(detail);
      setDisconnected(detail.status === "interrupted");
      setPollNotice("");
      setLoading(false);
      setCandidates((current) => {
        const existing = new Map(current.map((item) => [item.cursorId, item]));
        const merged = mergeCaptureCandidates(current, incoming).map((item) => {
          const previous = existing.get(item.cursorId);
          if (!previous || !dirtyRef.current.has(item.cursorId)) return item;
          return {
            ...item,
            name: previous.name,
            serverName: previous.serverName,
            conflictResolution: previous.conflictResolution,
            serverConflictResolution: previous.serverConflictResolution,
            targetElementId: previous.targetElementId
          };
        });
        cursorRef.current = Math.max(
          cursorRef.current,
          ...merged.map((item) => item.cursorId),
          0
        );
        const reportedCount = Math.max(
          detail.candidateCount,
          ...incoming.map((item) => Number(item?.candidateCount || 0)),
          0
        );
        if (reportedCount !== detail.candidateCount) {
          setSessionState((currentSession) => ({ ...currentSession, candidateCount: reportedCount }));
        }
        return merged;
      });
      if (!terminalStatuses.has(detail.status)) schedule(getPollDelay(0));
    };

    poll();
    return () => {
      stopped = true;
      if (timerRef.current) window.clearTimeout(timerRef.current);
      pollAbortRef.current?.abort();
    };
  }, [session?.id]);

  useEffect(() => () => abortAllRequests(), []);

  const counts = useMemo(() => {
    const result = { all: candidates.length, ready: 0, unnamed: 0, unreliable: 0, conflict: 0 };
    candidates.forEach((item) => {
      result[qualityKey(item)] += 1;
    });
    return result;
  }, [candidates]);

  const filteredCandidates = useMemo(
    () => candidates.filter((item) => filter === "all" || qualityKey(item) === filter),
    [candidates, filter]
  );
  const saveBlockers = useMemo(
    () => getSaveBlockers(candidates, selectedIDs),
    [candidates, selectedIDs]
  );
  const canSave = saveBlockers.length === 0 && !saving;
  const selectedVisible = filteredCandidates.length > 0
    && filteredCandidates.every((item) => selectedIDs.has(item.cursorId));
  const capacityCount = Math.max(sessionState.candidateCount, candidates.length);

  function restoreFocus() {
    const previous = previousFocusRef.current;
    if (previous && typeof previous.focus === "function" && previous.isConnected) {
      previous.focus();
    }
  }

  function hasUnsavedChanges() {
    return selectedIDs.size > 0 || dirtyRef.current.size > 0;
  }

  function requestClose() {
    if (hasUnsavedChanges() && !window.confirm("仍有未保存的选择或审核修改，确认关闭吗？")) return;
    abortAllRequests();
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

  async function runAction(operation) {
    const controller = new AbortController();
    const generation = generationRef.current;
    actionControllersRef.current.add(controller);
    try {
      const result = await operation(controller.signal);
      if (generationRef.current !== generation) {
        throw new DOMException("会话已切换", "AbortError");
      }
      return result;
    } finally {
      actionControllersRef.current.delete(controller);
    }
  }

  function updateLocalCandidate(cursorId, updater) {
    setCandidates((current) => current.map((item) => (
      item.cursorId === cursorId ? updater(item) : item
    )));
  }

  function setIssue(cursorId, message) {
    setCandidateIssues((current) => ({
      ...current,
      [cursorId]: message ? [{ field: "candidate", message }] : []
    }));
  }

  async function commitName(cursorId) {
    const candidate = candidates.find((item) => item.cursorId === cursorId);
    if (!candidate || candidate.name === candidate.serverName) return;
    const attemptedName = candidate.name;
    dirtyRef.current.add(cursorId);
    setIssue(cursorId, "");
    try {
      await runAction((signal) => elementCaptureService.update(
        sessionState.id,
        cursorId,
        { name: attemptedName },
        { signal }
      ));
      updateLocalCandidate(cursorId, (item) => ({ ...item, serverName: attemptedName }));
    } catch (error) {
      if (error?.name === "AbortError") return;
      updateLocalCandidate(cursorId, (item) => ({ ...item, name: item.serverName }));
      setIssue(cursorId, error?.message || "候选名称更新失败");
    }
  }

  async function setConflictResolution(cursorId, resolution) {
    const candidate = candidates.find((item) => item.cursorId === cursorId);
    if (!candidate || candidate.conflictResolution === resolution) return;
    const previousResolution = candidate.serverConflictResolution;
    dirtyRef.current.add(cursorId);
    setIssue(cursorId, "");
    updateLocalCandidate(cursorId, (item) => ({
      ...item,
      conflictResolution: resolution,
      targetElementId: resolution === "update"
        ? (item.targetElementId || (item.conflictTargetIds.length === 1 ? item.conflictTargetIds[0] : 0))
        : 0
    }));
    try {
      await runAction((signal) => elementCaptureService.update(
        sessionState.id,
        cursorId,
        { conflictResolution: resolution },
        { signal }
      ));
      updateLocalCandidate(cursorId, (item) => ({
        ...item,
        serverConflictResolution: resolution
      }));
    } catch (error) {
      if (error?.name === "AbortError") return;
      updateLocalCandidate(cursorId, (item) => ({
        ...item,
        conflictResolution: previousResolution,
        targetElementId: previousResolution === "update" ? item.targetElementId : 0
      }));
      setIssue(cursorId, error?.message || "冲突处理方式更新失败");
    }
  }

  function setTargetElement(cursorId, targetElementId) {
    dirtyRef.current.add(cursorId);
    setIssue(cursorId, "");
    updateLocalCandidate(cursorId, (item) => ({
      ...item,
      targetElementId: Number(targetElementId || 0)
    }));
  }

  function toggleCandidate(cursorId) {
    setSelectedIDs((current) => {
      const next = new Set(current);
      if (next.has(cursorId)) next.delete(cursorId);
      else next.add(cursorId);
      return next;
    });
  }

  function toggleVisibleCandidates() {
    setSelectedIDs((current) => {
      const next = new Set(current);
      filteredCandidates.forEach((item) => {
        if (selectedVisible) next.delete(item.cursorId);
        else next.add(item.cursorId);
      });
      return next;
    });
  }

  async function changeMode(mode) {
    if (sessionState.mode === mode) return;
    const previous = sessionState.mode;
    setActionError("");
    setSessionState((current) => ({ ...current, mode }));
    try {
      await runAction((signal) => elementCaptureService.mode(sessionState.id, mode, { signal }));
    } catch (error) {
      if (error?.name === "AbortError") return;
      setSessionState((current) => ({ ...current, mode: previous }));
      setActionError(error?.message || "采集模式切换失败");
    }
  }

  async function stopSession() {
    setActionError("");
    try {
      await runAction((signal) => elementCaptureService.stop(sessionState.id, { signal }));
      if (timerRef.current) window.clearTimeout(timerRef.current);
      pollAbortRef.current?.abort();
      setSessionState((current) => ({ ...current, status: "completed" }));
      setDisconnected(false);
    } catch (error) {
      if (error?.name !== "AbortError") {
        setActionError(error?.message || "停止采集失败");
      }
    }
  }

  async function saveSelected() {
    if (!canSave) return;
    const selected = candidates.filter((item) => selectedIDs.has(item.cursorId));
    setSaving(true);
    setActionError("");
    setCandidateIssues({});
    try {
      const result = await runAction((signal) => elementCaptureService.save(
        sessionState.id,
        selected.map(buildSaveItem),
        { signal }
      ));
      const processed = new Set([
        ...(result?.savedCandidateIds || []),
        ...(result?.ignoredCandidateIds || [])
      ].map(Number));
      if (!processed.size) selected.forEach((item) => processed.add(item.cursorId));
      setCandidates((current) => current.filter((item) => !processed.has(item.cursorId)));
      setSelectedIDs((current) => new Set([...current].filter((id) => !processed.has(id))));
      processed.forEach((id) => dirtyRef.current.delete(id));
      onSaved(result);
    } catch (error) {
      if (error?.name === "AbortError") return;
      if (error?.status === 409 && Array.isArray(error?.issues)) {
        const mapped = {};
        error.issues.forEach((issue) => {
          const id = Number(issue.candidateId || 0);
          if (!mapped[id]) mapped[id] = [];
          mapped[id].push(issue);
        });
        setCandidateIssues(mapped);
        const generalIssues = error.issues.filter((issue) => !issue.candidateId);
        if (generalIssues.length) {
          setActionError(generalIssues.map((issue) => issue.message).join("；"));
        }
        const firstID = Number(error.issues.find((issue) => issue.candidateId)?.candidateId || 0);
        window.setTimeout(() => rowRefs.current.get(firstID)?.focus(), 0);
      } else {
        setActionError(error?.message || "批量保存失败");
      }
    } finally {
      setSaving(false);
    }
  }

  const statusText = disconnected
    ? "执行器连接中断，已有候选仍可审核；连接恢复后将继续接收。"
    : statusLabels[sessionState.status] || sessionState.status;

  return (
    <div className="element-capture-drawer-backdrop">
      <aside
        aria-labelledby="element-capture-drawer-title"
        aria-modal="true"
        className="element-capture-drawer"
        onKeyDown={handleDialogKeyDown}
        ref={dialogRef}
        role="dialog"
      >
        <header className="element-capture-drawer-header">
          <div className="element-capture-drawer-heading">
            <span>CAPTURE REVIEW</span>
            <h2 id="element-capture-drawer-title">候选元素审核</h2>
            <p>
              {sessionState.pageName || `页面 #${sessionState.pageId || "-"}`}
              <b aria-hidden="true">·</b>
              {sessionState.executorName || sessionState.executorId || "未知执行器"}
              {sessionState.browserChannel ? <code>{sessionState.browserChannel}</code> : null}
            </p>
          </div>
          <div className="element-capture-drawer-header-actions">
            <div className="element-capture-segmented" role="group" aria-label="采集模式">
              <button
                aria-pressed={sessionState.mode === "pick"}
                className={sessionState.mode === "pick" ? "is-active" : ""}
                disabled={terminalStatuses.has(sessionState.status)}
                onClick={() => changeMode("pick")}
                type="button"
              >
                拾取模式
              </button>
              <button
                aria-pressed={sessionState.mode === "operate"}
                className={sessionState.mode === "operate" ? "is-active" : ""}
                disabled={terminalStatuses.has(sessionState.status)}
                onClick={() => changeMode("operate")}
                type="button"
              >
                操作模式
              </button>
            </div>
            <button
              className="element-capture-stop-button"
              disabled={terminalStatuses.has(sessionState.status)}
              onClick={stopSession}
              type="button"
            >
              停止采集
            </button>
            <button
              aria-label="关闭候选审核"
              className="element-capture-icon-button"
              onClick={requestClose}
              ref={closeRef}
              type="button"
            >
              ×
            </button>
          </div>
        </header>

        <div
          className={`element-capture-session-status ${disconnected ? "is-disconnected" : ""}`}
          role="status"
        >
          <span className="element-capture-status-dot" aria-hidden="true" />
          <strong>{statusText}</strong>
          {pollNotice ? <small>{pollNotice}</small> : null}
        </div>

        <nav className="element-capture-quality-track" aria-label="候选质量筛选">
          {qualitySegments.map((segment) => (
            <button
              aria-label={`${segment.label} ${counts[segment.key]}`}
              aria-pressed={filter === segment.key}
              className={`is-${segment.key} ${filter === segment.key ? "is-active" : ""}`}
              key={segment.key}
              onClick={() => setFilter(segment.key)}
              type="button"
            >
              <span>{segment.label}</span>
              <strong>{counts[segment.key]}</strong>
            </button>
          ))}
        </nav>

        {capacityCount >= 400 ? (
          <div className={`element-capture-capacity ${capacityCount >= 500 ? "is-limit" : ""}`} role="alert">
            {capacityCount >= 500
              ? `候选已达到 500 个上限，执行器将暂停拾取。请先审核并保存。`
              : `候选已达到 ${capacityCount} 个，接近 500 个上限。建议先分批审核。`}
          </div>
        ) : null}
        {actionError ? <div className="element-capture-error" role="alert">{actionError}</div> : null}

        <div className="element-capture-list-toolbar">
          <label>
            <input
              aria-checked={selectedVisible}
              checked={selectedVisible}
              disabled={!filteredCandidates.length}
              onChange={toggleVisibleCandidates}
              type="checkbox"
            />
            选择当前筛选候选
          </label>
          <span>已选择 {selectedIDs.size} / 200</span>
        </div>

        <div className="element-capture-candidate-list">
          {loading ? (
            <div className="element-capture-state">
              <strong>正在连接采集会话…</strong>
              <span>候选到达后会自动出现在这里。</span>
            </div>
          ) : filteredCandidates.length ? (
            filteredCandidates.map((candidate) => {
              const issues = candidateIssues[candidate.cursorId] || [];
              const category = qualityKey(candidate);
              return (
                <article
                  aria-label={`候选 ${candidate.name || candidate.cursorId}`}
                  className={`element-capture-candidate is-${category}`}
                  data-testid={`element-capture-candidate-${candidate.cursorId}`}
                  key={candidate.cursorId}
                  ref={(node) => {
                    if (node) rowRefs.current.set(candidate.cursorId, node);
                    else rowRefs.current.delete(candidate.cursorId);
                  }}
                  tabIndex={-1}
                >
                  <div className="element-capture-candidate-main">
                    <input
                      aria-label={`选择 ${candidate.name || `候选 ${candidate.cursorId}`}`}
                      checked={selectedIDs.has(candidate.cursorId)}
                      onChange={() => toggleCandidate(candidate.cursorId)}
                      type="checkbox"
                    />
                    <div className="element-capture-candidate-identity">
                      <label>
                        <span>候选名称</span>
                        <input
                          aria-label={`候选名称 ${candidate.cursorId}`}
                          onBlur={() => commitName(candidate.cursorId)}
                          onChange={(event) => {
                            dirtyRef.current.add(candidate.cursorId);
                            updateLocalCandidate(candidate.cursorId, (item) => ({
                              ...item,
                              name: event.target.value
                            }));
                          }}
                          onKeyDown={(event) => {
                            if (event.key === "Enter") event.currentTarget.blur();
                          }}
                          value={candidate.name}
                        />
                      </label>
                      <p>
                        <code>#{candidate.cursorId}</code>
                        <span>{candidate.tagName || "element"}</span>
                        {candidate.accessibleName ? <span>{candidate.accessibleName}</span> : null}
                      </p>
                    </div>
                    <span className={`element-capture-quality-label is-${category}`}>
                      {qualitySegments.find((segment) => segment.key === category)?.label}
                    </span>
                  </div>

                  <div className="element-capture-locators" aria-label={`候选 ${candidate.cursorId} 定位器`}>
                    {candidate.locators.length ? candidate.locators.map((locator, index) => (
                      <div key={`${locator.type}-${locator.value}-${index}`}>
                        <span>{locator.type || "locator"}</span>
                        <code>{locator.value || "-"}</code>
                        <strong>{Number(locator.score || 0)}</strong>
                        <em className={locator.unique ? "is-unique" : "is-not-unique"}>
                          {locator.unique ? "唯一" : "非唯一"}
                        </em>
                      </div>
                    )) : (
                      <p>没有可用定位器。请重新拾取语义更稳定的元素。</p>
                    )}
                  </div>

                  {candidate.conflictStatus === "duplicate" ? (
                    <div className="element-capture-conflict">
                      <div>
                        <strong>发现重复元素</strong>
                        <span>选择本次候选如何处理；系统不会静默覆盖。</span>
                      </div>
                      <div className="element-capture-conflict-actions" role="group" aria-label={`候选 ${candidate.cursorId} 冲突处理`}>
                        <button
                          aria-pressed={candidate.conflictResolution === "update"}
                          className={candidate.conflictResolution === "update" ? "is-active" : ""}
                          onClick={() => setConflictResolution(candidate.cursorId, "update")}
                          type="button"
                        >
                          更新已有元素
                        </button>
                        <button
                          aria-pressed={candidate.conflictResolution === "ignore"}
                          className={candidate.conflictResolution === "ignore" ? "is-active" : ""}
                          onClick={() => setConflictResolution(candidate.cursorId, "ignore")}
                          type="button"
                        >
                          忽略候选
                        </button>
                        <button
                          aria-pressed={candidate.conflictResolution === "create"}
                          className={candidate.conflictResolution === "create" ? "is-active" : ""}
                          onClick={() => setConflictResolution(candidate.cursorId, "create")}
                          type="button"
                        >
                          另存为新元素
                        </button>
                      </div>
                      {candidate.conflictResolution === "update" && candidate.conflictTargetIds.length > 1 ? (
                        <label className="element-capture-target-select">
                          <span>更新目标</span>
                          <select
                            aria-label="更新目标"
                            onChange={(event) => setTargetElement(candidate.cursorId, event.target.value)}
                            value={candidate.targetElementId || ""}
                          >
                            <option value="">请选择目标元素</option>
                            {candidate.conflictTargetIds.map((targetID) => (
                              <option key={targetID} value={targetID}>元素 #{targetID}</option>
                            ))}
                          </select>
                        </label>
                      ) : null}
                    </div>
                  ) : null}

                  {isUnnamed(candidate) ? (
                    <p className="element-capture-guidance">先补充可辨识且页面内唯一的名称。</p>
                  ) : !hasReliableLocator(candidate) ? (
                    <p className="element-capture-guidance">请重新拾取，至少需要同一条“唯一且评分 ≥ 70”的定位器。</p>
                  ) : null}

                  {issues.length ? (
                    <ul className="element-capture-issues" aria-label={`候选 ${candidate.cursorId} 问题`}>
                      {issues.map((issue, index) => (
                        <li key={`${issue.field || "candidate"}-${index}`}>{issue.message}</li>
                      ))}
                    </ul>
                  ) : null}
                </article>
              );
            })
          ) : (
            <div className="element-capture-state">
              <strong>{candidates.length ? "当前质量筛选下没有候选" : "尚未收到候选"}</strong>
              <span>
                {terminalStatuses.has(sessionState.status)
                  ? "会话已结束；可关闭审核，或切换筛选查看已有候选。"
                  : "在浏览器中切换到拾取模式，然后选择页面元素。"}
              </span>
              {candidates.length ? (
                <button onClick={() => setFilter("all")} type="button">查看全部候选</button>
              ) : null}
            </div>
          )}
        </div>

        <footer className="element-capture-savebar">
          <div>
            <strong>批量入库</strong>
            <span>{saveBlockers.length ? saveBlockers.join("；") : `将原子保存 ${selectedIDs.size} 个候选`}</span>
          </div>
          <button
            className="element-capture-primary-button"
            disabled={!canSave}
            onClick={saveSelected}
            type="button"
          >
            {saving ? "正在保存…" : "保存选中元素"}
          </button>
        </footer>
      </aside>
    </div>
  );
}
