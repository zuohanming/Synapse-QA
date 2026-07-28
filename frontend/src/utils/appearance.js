export const APPEARANCE_KEY = "synapse_qa_appearance";

export const defaultAppearance = {
  theme: "blueprint",
  uiFont: "system",
  codeFont: "consolas",
  uiFontSize: "standard",
  tableFontSize: "standard",
  codeFontSize: "standard",
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

const fontSizes = {
  ui: { small: "12px", standard: "14px", large: "16px" },
  table: { small: "10px", standard: "12px", large: "14px" },
  code: { small: "10px", standard: "12px", large: "14px" }
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
  root.dataset.uiFontSize = appearance.uiFontSize;
  root.dataset.tableFontSize = appearance.tableFontSize;
  root.dataset.codeFontSize = appearance.codeFontSize;
  root.style.setProperty("--app-font", uiFonts[appearance.uiFont] || uiFonts.system);
  root.style.setProperty("--code-font", codeFonts[appearance.codeFont] || codeFonts.consolas);
  root.style.setProperty("--ui-font-size", fontSizes.ui[appearance.uiFontSize] || fontSizes.ui.standard);
  root.style.setProperty("--table-font-size", fontSizes.table[appearance.tableFontSize] || fontSizes.table.standard);
  root.style.setProperty("--code-font-size", fontSizes.code[appearance.codeFontSize] || fontSizes.code.standard);
  localStorage.setItem(APPEARANCE_KEY, JSON.stringify(appearance));
  return appearance;
}

export function applyStoredAppearance() {
  return applyAppearance(readAppearance());
}
