import { menuData } from "../config/appConfig.js";

const ROUTE_STATE_KEY = "synapse_qa_route_state";
const PAGE_STATE_PREFIX = "synapse_qa_page_state:";
const ROUTE_TTL = 24 * 60 * 60 * 1000;

const defaultPath = ["首页", "项目概览"];

export function getDefaultPath() {
  return defaultPath;
}

export function isValidPath(path) {
  if (!Array.isArray(path) || path.length < 2) return false;
  const group = menuData.find((item) => item.title === path[0]);
  return Boolean(group?.children.includes(path[1]));
}

export function pathToHash(path) {
  if (!isValidPath(path)) return "#/";
  return `#/${path.map((item) => encodeURIComponent(item)).join("/")}`;
}

export function pathFromHash(hash = window.location.hash) {
  const raw = hash.replace(/^#\/?/, "");
  if (!raw) return null;
  const path = raw
    .split("/")
    .filter(Boolean)
    .map((item) => decodeURIComponent(item));
  return isValidPath(path) ? path : null;
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
