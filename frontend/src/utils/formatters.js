export function formatTime(value) {
  if (!value) return "-";
  return new Date(value).toLocaleString("zh-CN", { hour12: false });
}

export function pageItems(result) {
  return result?.items || [];
}
