const defaultLoopSteps = [
  {
    key: "goal",
    title: "目标",
    description: "明确本轮要解决的问题和验收结果。",
    checklist: ["确认要实现什么", "确认不做什么", "定义可验证标准"]
  },
  {
    key: "scope",
    title: "边界",
    description: "控制改动范围，避免无关重构和扩展。",
    checklist: ["只改相关文件", "不处理历史无关问题", "标记风险点"]
  },
  {
    key: "plan",
    title: "方案",
    description: "给出最小可行实现路径。",
    checklist: ["列出涉及模块", "说明关键取舍", "确定验证方式"]
  },
  {
    key: "implement",
    title: "实现",
    description: "按局部修改完成代码落地。",
    checklist: ["遵循现有风格", "删除自身引入的冗余", "保持提交范围清晰"]
  },
  {
    key: "verify",
    title: "验证",
    description: "使用自动化构建和浏览器检查闭环。",
    checklist: ["前端 npm run build", "后端 go build ./cmd/api", "关键 UI 路径浏览器验证"]
  },
  {
    key: "record",
    title: "记录",
    description: "沉淀修改内容、验证结果和待办项。",
    checklist: ["更新 docs 修改记录", "记录风险和后续事项", "提交前检查 git diff"]
  },
  {
    key: "commit",
    title: "提交",
    description: "以一次完整闭环为单位提交代码。",
    checklist: ["git status 确认范围", "提交清晰 message", "按需推送远端"]
  }
];

export function LoopEngineering({ steps = defaultLoopSteps, activeKey = "goal", title = "Loop Engineering", description = "目标澄清、局部实现、验证记录和提交归档的工程闭环。" }) {
  const activeStep = steps.find((step) => step.key === activeKey) || steps[0];

  return (
    <section className="loop-engineering">
      <div className="loop-engineering-header">
        <div>
          <strong>{title}</strong>
          <p>{description}</p>
        </div>
        <span>{activeStep?.title || "-"}</span>
      </div>

      <div className="loop-engineering-steps">
        {steps.map((step, index) => (
          <article className={step.key === activeKey ? "loop-step active" : "loop-step"} key={step.key}>
            <div className="loop-step-index">{index + 1}</div>
            <div>
              <strong>{step.title}</strong>
              <p>{step.description}</p>
              <ul>
                {step.checklist.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}

export { defaultLoopSteps };
