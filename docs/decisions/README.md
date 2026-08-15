# 架构决策记录（ADR）

ADR 记录影响多个模块、需要长期维护或难以低成本撤销的决定。状态使用 `拟议`、`已接受`、`已废弃` 或 `已被 ADR-XXXX 替代`。

## 索引

| ADR | 状态 | 决定 |
| --- | --- | --- |
| [ADR-0001](ADR-0001-evolutionary-delivery.md) | 已接受 | 用需求驱动数据库、领域和架构演进 |
| [ADR-0002](ADR-0002-go-first-delivery.md) | 已接受 | 第一阶段只实现完整 Go 版本 |
| [ADR-0003](ADR-0003-documentation-as-deliverable.md) | 已接受 | 文档与 QA 证据进入完成标准和 CI |
| [ADR-0004](ADR-0004-collaboration-protocol.md) | 已接受 | Codex 规划/验收、Claude 实现/测试的双代理协作协议 |
| [ADR-0005](ADR-0005-domestic-mainstream-technology-baseline.md) | 已接受 | 国内常用 Go、前端与中间件技术基线 |
| [ADR-0006](ADR-0006-frontend-framework-baseline.md) | 已接受 | 统一 React 前端框架与设计包复刻基线 |
| [ADR-0007](ADR-0007-obsidian-sync-gate-and-ccb-exclusion.md) | 已接受 | Obsidian 最终同步门禁与 `.ccb` Git/doccheck 排除 |
| [ADR-0008](ADR-0008-collaboration-activation-boundary.md) | 已接受 | 双代理协作协议仅在 CCB 模式下激活的边界 |

新 ADR 从 [模板](template.md) 创建。已接受的 ADR 不直接重写结论；新事实需要新 ADR，并将旧记录标记为被替代。
