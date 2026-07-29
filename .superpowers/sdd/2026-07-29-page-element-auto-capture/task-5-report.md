# Task 5 交接报告：执行器定位器生成器

## 实现内容

- 新增纯函数 `build_candidate`、`is_dynamic_token` 与 `score_locator`，不依赖 Playwright、网络或浏览器状态。
- 新增与平台候选 JSON 兼容的 Pydantic 模型；`platform_payload()` 仅输出平台接收接口所需字段及定位器的 `type`、`value`、`score`、`unique`。
- 定位器按 testid、稳定 data 属性/静态 id、role、label、稳定 CSS、文本和 XPath 的固定优先级去重并截断为三组；匹配数量来自 `ElementSnapshot.locator_matches`。
- 动态 ID/class、UUID、时间戳、长数字或哈希随机片段会被过滤；包含 password、token、secret、cookie、authorization 等敏感字段或值的内容不会进入候选、定位器或指纹。
- 指纹基于规范化、排序后的非敏感稳定 DOM 特征生成 64 位小写 SHA-256；属性顺序及敏感值变化不会改变结果。

## TDD 与验证

- RED：`cd executor; python -m pytest tests/test_locator_generator.py -q` 初始因 `app.models.capture` 不存在而在收集阶段失败。
- GREEN：同一聚焦测试通过 9 项，覆盖优先级、动态/敏感过滤（包括嵌入 CSS 表达式的动态 class）、三组上限、去重、非唯一降分、深度扣分、指纹稳定性和未命名占位。
- 回归：`cd executor; python -m pytest -q` 通过，结果为 29 passed。

## 范围

本任务仅实现快照到候选的确定性转换；浏览器拾取、定位器实际匹配统计和候选回传由后续 Task 6 负责。
