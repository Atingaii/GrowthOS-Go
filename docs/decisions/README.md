# 架构决策记录（ADR）

ADR 记录影响多个模块、需要长期维护或难以低成本撤销的决定。状态使用 `拟议`、`已接受`、`已废弃` 或 `已被 ADR-XXXX 替代`。

## 索引

| ADR | 状态 | 决定 |
| --- | --- | --- |
| [ADR-0001](ADR-0001-evolutionary-delivery.md) | 已接受 | 用需求驱动数据库、领域和架构演进 |
| [ADR-0002](ADR-0002-go-first-delivery.md) | 已接受 | 第一阶段只实现完整 Go 版本 |
| [ADR-0003](ADR-0003-documentation-as-deliverable.md) | 已接受 | 文档与 QA 证据进入完成标准和 CI |
| [ADR-0005](ADR-0005-domestic-mainstream-technology-baseline.md) | 已接受 | 国内常用 Go、前端与中间件技术基线 |

新 ADR 从 [模板](template.md) 创建。已接受的 ADR 不直接重写结论；新事实需要新 ADR，并将旧记录标记为被替代。
