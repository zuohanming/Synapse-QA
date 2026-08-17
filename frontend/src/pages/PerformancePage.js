import { PerfPlansPage } from "./PerfPlansPage.js";
import { PerfRunsPage } from "./PerfRunsPage.js";

export function PerformancePage({ activePath }) {
  const section = activePath[1];
  if (section === "压测方案") return <PerfPlansPage />;
  if (section === "测试报告") return <PerfRunsPage />;
  return <PerfPlansPage />;
}
