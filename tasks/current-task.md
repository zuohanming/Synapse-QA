# 当前任务

## 任务名称

补充根目录开发约束。

## 用户要求

用户明确要求：不应该创建独立模块，所有任务都应该在根目录下进行，并写入项目规范。

## 实现范围

- 更新 `SKILL.md`，增加工作区约束。
- 更新 `STATE.md`，记录该项目决策。
- 更新 `tasks/current-task.md`，记录当前任务。
- 不修改业务代码。
- 不提交 Git，除非用户明确要求。

## 隐含假设

- 文档使用 UTF-8 编码和简体中文。
- 内容以当前项目实际结构为准。
- “根目录”指当前主项目目录 `F:\Synapse QA`。

## 验收标准

- `SKILL.md` 明确禁止普通任务创建独立 worktree、独立仓库或孤立模块。
- `SKILL.md` 明确所有正式任务必须在 `F:\Synapse QA` 根目录内完成。
- `STATE.md` 记录该项目决策。
- `tasks/current-task.md` 记录本次规范补充任务。

## 验证方式

```bash
Test-Path SKILL.md
Test-Path STATE.md
Test-Path tasks/current-task.md
git status --short
```
