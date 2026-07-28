import { DataTable } from "../components/DataTable.js";
import { PageHeader } from "../components/PageHeader.js";
import { ResourceListPage } from "../components/ResourceListPage.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { systemService } from "../services/systemService.js";
import { formatTime, pageItems } from "../utils/formatters.js";
import { applyAppearance, readAppearance } from "../utils/appearance.js";

export function SystemPage({ activePath }) {
  const section = activePath[1];
  const loaders = {
    配置管理: () => Promise.all([systemService.menus(), systemService.dictionaries()]),
    外观设置: () => Promise.resolve(null),
    用户管理: () => systemService.users({ page: 1, pageSize: 20 }),
    角色管理: () => systemService.roles(),
    操作日志: () => systemService.logs()
  };
  const { data, loading, error } = useAsyncData(loaders[section] || loaders.配置管理, [section]);

  if (section === "外观设置") {
    return <AppearanceSettings />;
  }

  if (section === "用户管理") {
    return <ResourceListPage title={section} description="维护平台用户与访问状态" panelTitle="用户列表" rows={pageItems(data)} columns={userColumns} loading={loading} error={error} />;
  }
  if (section === "角色管理") {
    return <ResourceListPage title={section} description="查看平台角色及权限说明" panelTitle="角色列表" rows={data || []} columns={roleColumns} loading={loading} error={error} />;
  }
  if (section === "操作日志") {
    return <ResourceListPage title={section} description="追踪平台内的关键操作记录" panelTitle="操作日志列表" rows={data || []} columns={logColumns} loading={loading} error={error} />;
  }
  return (
    <div className="section-stack">
      <PageHeader title={section} description="查看菜单与数据字典配置" />
      <StateBlock loading={loading} error={error}><ConfigOverview data={data} /></StateBlock>
    </div>
  );
}

function AppearanceSettings() {
  const [appearance, setAppearance] = useState(() => readAppearance());

  function update(key, value) {
    setAppearance((current) => applyAppearance({ ...current, [key]: value }));
  }

  return <div className="section-stack appearance-page">
    <PageHeader title="外观设置" description="选择平台主题、字体和内容密度，修改后立即生效" />
    <section className="resource-panel appearance-section">
      <div className="appearance-section-heading"><LayoutTemplate size={18} /><div><strong>界面主题</strong><span>选择工作台的整体视觉风格</span></div></div>
      <div className="theme-choice-grid">
        <button className={appearance.theme === "blue" ? "theme-choice active" : "theme-choice"} onClick={() => update("theme", "blue")} type="button">
          <span className="theme-preview theme-preview-blue"><i /><b /><em /></span><strong>科技蓝</strong><small>当前企业工作台风格</small>{appearance.theme === "blue" ? <Check size={16} /> : null}
        </button>
        <button className={appearance.theme === "codex" ? "theme-choice active" : "theme-choice"} onClick={() => update("theme", "codex")} type="button">
          <span className="theme-preview theme-preview-codex"><i /><b /><em /></span><strong>Codex 浅色</strong><small>暖灰、黑白与轻量边框</small>{appearance.theme === "codex" ? <Check size={16} /> : null}
        </button>
        <button className={appearance.theme === "wechat" ? "theme-choice active" : "theme-choice"} onClick={() => update("theme", "wechat")} type="button">
          <span className="theme-preview theme-preview-wechat"><i /><b /><em /></span><strong>微信清新</strong><small>柔和灰白与自然绿色强调</small>{appearance.theme === "wechat" ? <Check size={16} /> : null}
        </button>
        <button className={appearance.theme === "kimi" ? "theme-choice active" : "theme-choice"} onClick={() => update("theme", "kimi")} type="button">
          <span className="theme-preview theme-preview-kimi"><i /><b /><em /></span><strong>Kimi 月紫</strong><small>冷白、月紫与轻盈渐变层次</small>{appearance.theme === "kimi" ? <Check size={16} /> : null}
        </button>
      </div>
    </section>
    <div className="appearance-settings-grid">
      <section className="resource-panel appearance-section">
        <div className="appearance-section-heading"><Type size={18} /><div><strong>界面字体</strong><span>用于导航、表单和正文</span></div></div>
        <select className="text-input" onChange={(event) => update("uiFont", event.target.value)} value={appearance.uiFont}><option value="system">系统默认</option><option value="yahei">微软雅黑</option><option value="source">思源黑体</option></select>
        <div className="font-sample">Synapse QA · 自动化测试工作台 · Aa 123</div>
      </section>
      <section className="resource-panel appearance-section">
        <div className="appearance-section-heading"><Code2 size={18} /><div><strong>代码字体</strong><span>用于日志、SQL 和 Python 编辑器</span></div></div>
        <select className="text-input" onChange={(event) => update("codeFont", event.target.value)} value={appearance.codeFont}><option value="consolas">Consolas</option><option value="cascadiacode">Cascadia Code</option><option value="jetbrains">JetBrains Mono</option></select>
        <code className="code-font-sample">assert response.status == 200</code>
      </section>
    </div>
    <section className="resource-panel appearance-section">
      <div className="appearance-section-heading"><Type size={18} /><div><strong>字体大小</strong><span>分别调整界面、数据表格和代码日志的文字尺寸</span></div></div>
      <div className="font-size-setting-list">
        {[
          ["uiFontSize", "界面文字", "导航、按钮、表单与正文", "界面 Aa 123"],
          ["tableFontSize", "表格文字", "列表表头、数据单元格与结构化参数", "字段名称 · status · 200"],
          ["codeFontSize", "代码与日志", "JSON、SQL、Python 和执行日志", "const status = 200;"]
        ].map(([key, label, description, sample]) => <article className={`font-size-setting font-size-setting-${key}`} key={key}>
          <div><strong>{label}</strong><span>{description}</span><samp>{sample}</samp></div>
          <div className="font-size-options">{[["small", "小"], ["standard", "标准"], ["large", "大"]].map(([value, optionLabel]) => <button aria-label={`${label}：${optionLabel}`} className={appearance[key] === value ? "active" : ""} key={value} onClick={() => update(key, value)} type="button">{optionLabel}</button>)}</div>
        </article>)}
      </div>
    </section>
    <section className="resource-panel appearance-section">
      <div className="appearance-section-heading"><LayoutTemplate size={18} /><div><strong>显示密度</strong><span>控制表格、表单和工作区的间距</span></div></div>
      <div className="density-options">{[["compact", "紧凑"], ["standard", "标准"], ["comfortable", "宽松"]].map(([value, label]) => <button className={appearance.density === value ? "active" : ""} key={value} onClick={() => update("density", value)} type="button">{label}</button>)}</div>
    </section>
  </div>;
}

const userColumns = [
  { key: "id", title: "ID" },
  { key: "displayName", title: "昵称" },
  { key: "username", title: "账号" },
  { key: "roleName", title: "角色" },
  { key: "email", title: "邮箱" },
  { key: "status", title: "状态" },
  { key: "createdAt", title: "创建时间", render: (row) => formatTime(row.createdAt) }
];

const roleColumns = [
  { key: "id", title: "ID" },
  { key: "name", title: "角色名称" },
  { key: "description", title: "角色描述" },
  { key: "createdAt", title: "创建时间", render: (row) => formatTime(row.createdAt) },
  { key: "updatedAt", title: "更新时间", render: (row) => formatTime(row.updatedAt) }
];

const logColumns = [
  { key: "id", title: "ID" },
  { key: "actor", title: "操作人" },
  { key: "action", title: "动作" },
  { key: "target", title: "对象" },
  { key: "createdAt", title: "时间", render: (row) => formatTime(row.createdAt) }
];

function ConfigOverview({ data }) {
  const [menus = [], dicts = []] = data || [];
  return (
    <div className="split-grid">
      <section className="resource-panel"><div className="panel-header"><strong>菜单列表</strong></div><DataTable rows={menus} columns={[{ key: "id", title: "ID" }, { key: "title", title: "菜单" }, { key: "code", title: "编码" }]} /></section>
      <section className="resource-panel"><div className="panel-header"><strong>数据字典列表</strong></div><DataTable rows={dicts} columns={[{ key: "id", title: "ID" }, { key: "name", title: "字典" }, { key: "value", title: "值" }]} /></section>
    </div>
  );
}
import { Check, Code2, LayoutTemplate, Type } from "lucide-react";
import { useState } from "react";
