export const APPEARANCE_KEY = "synapse_qa_appearance";

export const defaultAppearance = {
  theme: "blue",
  uiFont: "system",
  codeFont: "consolas",
  density: "standard"
};

const uiFonts = {
  system: 'Inter, "PingFang SC", "Microsoft YaHei", system-ui, sans-serif',
  yahei: '"Microsoft YaHei UI", "Microsoft YaHei", sans-serif',
  source: '"Noto Sans SC", "Source Han Sans SC", "PingFang SC", sans-serif'
};

const codeFonts = {
  consolas: 'Consolas, "SFMono-Regular", monospace',
  cascadiacode: '"Cascadia Code", Consolas, monospace',
  jetbrains: '"JetBrains Mono", Consolas, monospace'
};

export function readAppearance() {
  try {
    return { ...defaultAppearance, ...JSON.parse(localStorage.getItem(APPEARANCE_KEY) || "{}") };
  } catch {
    return { ...defaultAppearance };
  }
}

export function applyAppearance(value) {
  const appearance = { ...defaultAppearance, ...value };
  const root = document.documentElement;
  root.dataset.theme = appearance.theme;
  root.dataset.density = appearance.density;
  root.style.setProperty("--app-font", uiFonts[appearance.uiFont] || uiFonts.system);
  root.style.setProperty("--code-font", codeFonts[appearance.codeFont] || codeFonts.consolas);
  localStorage.setItem(APPEARANCE_KEY, JSON.stringify(appearance));
  return appearance;
}

export function applyStoredAppearance() {
  return applyAppearance(readAppearance());
}
