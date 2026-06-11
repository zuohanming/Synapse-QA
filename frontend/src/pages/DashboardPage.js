import { PageHeader } from "../components/PageHeader.js";
import { StateBlock } from "../components/StateBlock.js";
import { useAsyncData } from "../hooks/useAsyncData.js";
import { systemService } from "../services/systemService.js";

export function DashboardPage() {
  const { data, loading, error } = useAsyncData(() => systemService.overview(), []);
  const cards = [
    ["用户数量", data?.users],
    ["角色数量", data?.roles],
    ["项目数量", data?.projects],
    ["产品数量", data?.products],
    ["测试对象", data?.testObjects],
    ["UI 资产", data?.uiAssets],
    ["页面元素", data?.pageElements]
  ];

  return (
    <>
      <PageHeader title="项目概览" description="当前自动化平台资源总览" />
      <StateBlock loading={loading} error={error}>
        <div className="metric-grid">
          {cards.map(([label, value]) => (
            <div className="metric-card" key={label}>
              <span>{label}</span>
              <strong>{value ?? 0}</strong>
            </div>
          ))}
        </div>
      </StateBlock>
    </>
  );
}
