# 架构决策记录（ADR）

ADR 记录影响多个模块、需要长期维护或难以低成本撤销的决定。状态使用 `拟议`、`已接受`、`已废弃` 或 `已被 ADR-XXXX 替代`。

## 索引

| ADR | 状态 | 决定 |
| --- | --- | --- |
| [ADR-0001](ADR-0001-evolutionary-delivery.md) | 已接受 | 用需求驱动数据库、领域和架构演进 |
| [ADR-0002](ADR-0002-go-first-delivery.md) | 已接受 | 第一阶段只实现完整 Go 版本 |
| [ADR-0003](ADR-0003-documentation-as-deliverable.md) | 已接受 | 文档与 QA 证据进入完成标准和 CI |
| [ADR-0005](ADR-0005-domestic-mainstream-technology-baseline.md) | 已接受 | 国内常用 Go、前端与中间件技术基线 |
| [ADR-0006](ADR-0006-frontend-framework-baseline.md) | 已接受 | 统一 React 前端框架与设计包复刻基线 |
| [ADR-0007](ADR-0007-modular-monolith-first.md) | 已接受 | 第一版使用模块化单体，以运行证据驱动服务拆分 |
| [ADR-0008](ADR-0008-supported-go-toolchain-baseline.md) | 已接受 | 最低工具链升级到受支持且可复现的 Go 1.26.6，并接受 Gin v1.12.0 |
| [ADR-0009](ADR-0009-runtime-boundaries.md) | 已接受 | 显式配置、slog、请求关联和 fault 到 HTTP 的运行时边界 |
| [ADR-0010](ADR-0010-mysql-migration-boundaries.md) | 已接受 | MySQL 连接、账号隔离、readiness 与前向 Migration 边界 |
| [ADR-0011](ADR-0011-same-origin-frontend-integration.md) | 已接受 | 浏览器通过受约束的同源代理消费系统探针，并在运行时验证契约与失败语义 |

新 ADR 从 [模板](template.md) 创建。已接受的 ADR 不直接重写结论；新事实需要新 ADR，并将旧记录标记为被替代。
