import { PerfPlanDetailPage } from "./PerfPlanDetailPage.js";
import { PerfPlanEditPage } from "./PerfPlanEditPage.js";
import { PerfPlansPage } from "./PerfPlansPage.js";
import { PerfRunDetailPage } from "./PerfRunDetailPage.js";
import { PerfRunsPage } from "./PerfRunsPage.js";
import { PerfSchedulesPage } from "./PerfSchedulesPage.js";

// 性能测试模块分发器：按 hash 路由的深层路径分发（SPEC §8.1）。
// 内部 activePath 形如 ["性能测试", "压测方案", id?, action?]。
export function PerformancePage({ activePath }) {
  const section = activePath[1];

  if (section === "压测方案") {
    const third = activePath[2];
    if (third === "new") return <PerfPlanEditPage key="new" planId={null} />;
    if (third && activePath[3] === "edit") return <PerfPlanEditPage key={third} planId={third} />;
    if (third) return <PerfPlanDetailPage key={third} planId={third} />;
    return <PerfPlansPage />;
  }

  if (section === "测试报告") {
    const third = activePath[2];
    if (third) return <PerfRunDetailPage key={third} runId={third} />;
    return <PerfRunsPage />;
  }

  if (section === "定时规则") return <PerfSchedulesPage />;

  return <PerfPlansPage />;
}
