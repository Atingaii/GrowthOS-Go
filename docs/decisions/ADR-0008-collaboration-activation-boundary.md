# ADR-0008：双代理协作协议的激活边界

- **状态：** 已接受
- **日期：** 2026-08-12
- **负责人：** GrowthOS 维护者

## 背景

双代理协作协议（[ADR-0004](ADR-0004-collaboration-protocol.md) 及其演进 ADR-0007）规定了 Codex 与 Claude 的职责分工、任务包、验收与 Obsidian 同步门禁。但仓库根级 `AGENTS.md`/`CLAUDE.md` 会被**任意一次单独运行** Codex 或 Claude 时读取——包括非 CCB 会话、无 CCB 项目上下文、或仅有一个代理在工作的场景。若协议始终生效，单代理会被错误要求虚构另一代理、进行双代理委派与验收，违背"代理按宿主自身规则工作"的预期。

## 评估过的方案

1. **协议始终生效**：单代理被要求虚构另一代理并完成双代理闭环，错误且难以满足。
2. **删除双代理协议**：丢失 CCB 协作的职责与验收价值。
3. **明确激活边界**（采用）：协议仅在 CCB 管理且 Codex/Claude 作为同一 CCB 项目协作代理时激活；单代理/非 CCB/无 CCB 上下文时不激活，按宿主规则工作。

## 决策

- 在唯一权威正文 `docs/standards/collaboration-protocol.md` 增加"激活边界"章节：明确激活条件（CCB 管理 + Codex/Claude 同一 CCB 项目）、不激活条件（单 Codex/单 Claude/非 CCB/无 CCB 项目上下文）及对应的单代理行为。
- CCB 识别以项目存在并被 CCB 管理的上下文为准（可引用 `.ccb/ccb.config` 或 CCB 运行时），但 `.ccb/` 仍必须排除 Git（见 ADR-0007）。
- 根级入口 `AGENTS.md`/`CLAUDE.md`、`CONTRIBUTING.md`、`docs/README.md` 导航与 `docs/standards/obsidian-sync.md` 门禁同步注明激活边界，保持单一权威正文、避免漂移。

## 影响

- 单代理运行时不再被要求虚构另一代理或进行双代理验收。
- CCB 模式下双代理职责、任务包、交付、验收与同步门禁照常生效。
- 规则更准确反映"CCB 协作是项目的一种运行模式，而非默认状态"。
