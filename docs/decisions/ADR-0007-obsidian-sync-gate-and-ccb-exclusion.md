# ADR-0007：Obsidian 最终同步门禁与 .ccb 排除

- **状态：** 已接受
- **日期：** 2026-08-12
- **负责人：** GrowthOS 维护者

## 背景

双代理协作协议（[ADR-0004](ADR-0004-collaboration-protocol.md)）要求 Claude 每个任务提交带证据的交付报告，但未规定文档镜像到个人 Obsidian Vault 的强制时点，导致最终镜像可能漏做或晚做。同时，CCB 多代理协作在仓库根生成的 `.ccb/` 运行时目录（含大量第三方 Markdown 与插件上下文链接）未被 `cmd/doccheck` 跳过，原工作区 `make verify` 因此持续报 `.ccb` 断链误报，掩盖了真实质量信号。

## 评估过的方案

1. **仅在 `.gitignore` 排除 `.ccb`**：能隐藏误报，但 doccheck 仍会扫描、`make verify` 在原工作区仍可能失败，属于"仅靠偶然通过"。
2. **仅让 doccheck 跳过 `.ccb`**：能消除误报，但 `.ccb/` 仍可能被误提交，且无回归测试保证不复发。
3. **两者都做，并补回归测试**（采用）：`.gitignore` 排除根 `.ccb/`，`doccheck` 明确跳过 `.ccb` 并补回归测试，把"是否在含 `.ccb/` 的工作区运行 `make verify` 通过"固化为可验证事实。
4. **同步门禁仅写在协作协议正文**：够用但不进完成定义/贡献/QA 索引，易被跳过；需要同步写入各权威入口保持一致。

## 决策

- `.gitignore` 增加 `/.ccb/`，排除仓库根 CCB 运行时目录；不删除也不纳入 Git。
- `cmd/doccheck` 链接检查将 `.ccb` 列入跳过目录，并新增回归测试验证 `.ccb` 下断链不被报告、非 `.ccb` 断链仍被报告。
- 协作协议新增"Obsidian 最终同步门禁"：每个任务**无论是否修改** `README.md`/`docs`/QA，都必须在提交交付报告前无条件执行 `make docs-sync VAULT=/mnt/e/TencentGo/growthOS`，不允许跳过；最终同步是交付前最后一步镜像，之后不改 `README.md`/`docs/`；同步命令、结果与一致性证据写入交付/QA 记录；Codex 缺此证据不得验收通过。
- 同步门禁同步写入 `docs/standards/obsidian-sync.md`、`CONTRIBUTING.md`、`docs/standards/definition-of-done.md` 与 `docs/README.md` 导航，保证一致且易发现。
- 本决策是对 ADR-0004 协作协议的演进，不重写其历史结论；ADR-0004 保持已接受。

## 影响

- 原工作区（含 `.ccb/`）的 `make verify` 不再因 `.ccb` 误报失败。
- Claude 每个任务在交付前必须执行并记录最终 Obsidian 同步，Codex 验收门禁可据此把关。
- Vault 仍是个人镜像，只读单向，个人笔记不回写仓库（语义不变）。
- `.ccb/` 不进入 Git、不被删除，CCB 运行时状态保留。
