import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { systemService } from "../services/systemService.js";
import { Box, Braces, CheckCircle2, FolderKanban, MousePointer2, Package, ShieldCheck, UsersRound } from "lucide-react";

export function DashboardPage() {
  const { data, loading, error } = useAsyncData(() => systemService.overview(), []);
  const cards = [
    ["测试项目", data?.projects, FolderKanban, "项目空间"],
    ["产品模块", data?.products, Package, "业务覆盖"],
    ["测试对象", data?.testObjects, Box, "执行目标"],
    ["页面元素", data?.pageElements, MousePointer2, "自动化资产"],
    ["UI 资产", data?.uiAssets, Braces, "可复用资源"],
    ["平台用户", data?.users, UsersRound, "协作成员"],
    ["权限角色", data?.roles, ShieldCheck, "访问控制"]
  ];

  return (
    <>
      <PageHeader title="项目概览" description="查看自动化测试平台的资产规模与运行准备情况" />
      <StateBlock loading={loading} error={error}>
        <div className="dashboard-layout">
          <section className="overview-banner">
            <div><span className="overview-kicker"><CheckCircle2 size={14} /> 工作台已就绪</span><h2>质量资产，一览即明</h2><p>从项目配置到自动化执行，在一个工作台中完成闭环管理。</p></div>
            <div className="overview-score"><span>资产总量</span><strong>{cards.reduce((sum, [, value]) => sum + Number(value || 0), 0)}</strong><small>项已纳入平台</small></div>
          </section>
          <div className="metric-grid">
            {cards.map(([label, value, Icon, helper], index) => (
              <article className="metric-card" key={label}>
                <div className="metric-card-top"><span className="metric-icon"><Icon size={18} /></span><span className="metric-index">0{index + 1}</span></div>
                <strong>{value ?? 0}</strong><span className="metric-label">{label}</span><small>{helper}</small>
              </article>
            ))}
          </div>
        </div>
      </StateBlock>
    </>
  );
}
