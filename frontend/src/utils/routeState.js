import { menuData } from "../config/appConfig.js";

const ROUTE_STATE_KEY = "synapse_qa_route_state";
const PAGE_STATE_PREFIX = "synapse_qa_page_state:";
const ROUTE_TTL = 24 * 60 * 60 * 1000;

const defaultPath = ["首页", "项目概览"];

// 性能测试模块使用英文 slug 的独立 URL，支持深层子路径（详情/编辑）。
const PERF_GROUP = "性能测试";
const PERF_URL_PREFIX = "performance";
const PERF_SLUGS = {
  压测方案: "plans",
  测试报告: "runs",
  定时规则: "schedules"
};

export function getDefaultPath() {
  return defaultPath;
}

export function isValidPath(path) {
  if (!Array.isArray(path) || path.length < 2) return false;
  const group = menuData.find((item) => item.title === path[0]);
  if (!group?.children.includes(path[1])) return false;
  if (path[0] !== PERF_GROUP) return path.length === 2;
  // 性能测试：压测方案 [/new | /:id | /:id/edit]，测试报告 [/ | /:id]
  if (path[1] === "压测方案") {
    if (path.length === 2) return true;
    if (path.length === 3) return path[2] === "new" || /^\d+$/.test(path[2]);
    if (path.length === 4) return /^\d+$/.test(path[2]) && path[3] === "edit";
    return false;
  }
  if (path[1] === "测试报告") {
    if (path.length === 2) return true;
    return path.length === 3 && /^\d+$/.test(path[2]);
  }
  if (path[1] === "定时规则") {
    if (path.length === 2) return true;
    return path.length === 3 && /^\d+$/.test(path[2]);
  }
  return false;
}

export function pathToHash(path) {
  if (!isValidPath(path)) return "#/";
  if (path[0] === PERF_GROUP) {
    const slug = PERF_SLUGS[path[1]];
    const rest = path.slice(2).map((item) => encodeURIComponent(item)).join("/");
    return rest ? `#/${PERF_URL_PREFIX}/${slug}/${rest}` : `#/${PERF_URL_PREFIX}/${slug}`;
  }
  return `#/${path.map((item) => encodeURIComponent(item)).join("/")}`;
}

export function pathFromHash(hash = window.location.hash) {
  const raw = hash.replace(/^#\/?/, "");
  if (!raw) return null;
  const segments = raw
    .split("/")
    .filter(Boolean)
    .map((item) => decodeURIComponent(item));
  if (segments[0] === PERF_URL_PREFIX) {
    const child = Object.keys(PERF_SLUGS).find((key) => PERF_SLUGS[key] === segments[1]);
    if (!child) return null;
    const path = [PERF_GROUP, child, ...segments.slice(2)];
    return isValidPath(path) ? path : null;
  }
  return isValidPath(segments) ? segments : null;
}

export function readRouteState() {
  const hashPath = pathFromHash();
  if (hashPath) return hashPath;
  if (window.location.hash.replace(/^#\/?/, "")) {
    return defaultPath;
  }
  try {
    const state = JSON.parse(localStorage.getItem(ROUTE_STATE_KEY) || "null");
    if (!state || Date.now() - state.updatedAt > ROUTE_TTL || !isValidPath(state.activePath)) {
      localStorage.removeItem(ROUTE_STATE_KEY);
      return defaultPath;
    }
    return state.activePath;
  } catch {
    localStorage.removeItem(ROUTE_STATE_KEY);
    return defaultPath;
  }
}

export function persistRouteState(activePath) {
  if (!isValidPath(activePath)) return;
  localStorage.setItem(
    ROUTE_STATE_KEY,
    JSON.stringify({
      activePath,
      search: window.location.search,
      scroll: { x: window.scrollX, y: window.scrollY },
      updatedAt: Date.now()
    })
  );
  const nextHash = pathToHash(activePath);
  if (window.location.hash !== nextHash) {
    window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}${nextHash}`);
  }
}

export function readScrollState() {
  try {
    const state = JSON.parse(localStorage.getItem(ROUTE_STATE_KEY) || "null");
    return state?.scroll || { x: 0, y: 0 };
  } catch {
    return { x: 0, y: 0 };
  }
}

export function cleanupRouteState() {
  const now = Date.now();
  try {
    const state = JSON.parse(localStorage.getItem(ROUTE_STATE_KEY) || "null");
    if (state && now - state.updatedAt > ROUTE_TTL) {
      localStorage.removeItem(ROUTE_STATE_KEY);
    }
  } catch {
    localStorage.removeItem(ROUTE_STATE_KEY);
  }

  Object.keys(localStorage)
    .filter((key) => key.startsWith(PAGE_STATE_PREFIX))
    .forEach((key) => {
      try {
        const state = JSON.parse(localStorage.getItem(key) || "null");
        if (!state || now - state.updatedAt > ROUTE_TTL) {
          localStorage.removeItem(key);
        }
      } catch {
        localStorage.removeItem(key);
      }
    });
}

export function readPageState(key, fallback = null) {
  try {
    const state = JSON.parse(localStorage.getItem(`${PAGE_STATE_PREFIX}${key}`) || "null");
    if (!state || Date.now() - state.updatedAt > ROUTE_TTL) {
      localStorage.removeItem(`${PAGE_STATE_PREFIX}${key}`);
      return fallback;
    }
    return state.value ?? fallback;
  } catch {
    localStorage.removeItem(`${PAGE_STATE_PREFIX}${key}`);
    return fallback;
  }
}

export function persistPageState(key, value) {
  localStorage.setItem(`${PAGE_STATE_PREFIX}${key}`, JSON.stringify({ value, updatedAt: Date.now() }));
}

export function clearPageState(key) {
  localStorage.removeItem(`${PAGE_STATE_PREFIX}${key}`);
}
