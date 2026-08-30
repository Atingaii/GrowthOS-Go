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
| [ADR-0012](ADR-0012-compose-development-topology.md) | 已接受 | 本地 Compose 仅发布同源 Web 入口，以隔离网络、一次性迁移和文件秘密装配开发栈 |
| [ADR-0013](ADR-0013-lottery-domain-model.md) | 已接受 | 以 Strategy 聚合管理持久化无关的 Lottery 候选，采用整数相对权重、显式结果语义和 AwardID 规范顺序 |
| [ADR-0014](ADR-0014-lottery-persistence-schema.md) | 已接受 | 以两条单 DDL 前向 Migration 建立 Lottery Strategy/Award 最小持久化结构与约束边界 |
| [ADR-0015](ADR-0015-compose-schema-grant-reconciliation.md) | 已接受 | Migration 后经无网络 Unix socket 作业精确收敛 Compose 应用身份的数据权限 |
| [ADR-0016](ADR-0016-lottery-repository-boundaries.md) | 已接受 | 以窄端口、聚合写事务、只读 RR 快照和语义错误实现 Lottery Strategy 仓储 |
| [ADR-0017](ADR-0017-lottery-weighted-selection.md) | 已接受 | 以 consumer-owned bounded source、减法桶和 crypto adapter 实现无偏加权 Award 选择 |
| [ADR-0018](ADR-0018-ephemeral-lottery-selection-api.md) | 已接受 | 以默认关闭的 development/test ephemeral route 组合只读快照与加权选择，并明确拒绝把临时结果冒充正式 Draw |
| [ADR-0019](ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md) | 已接受 | 按事实所有者和业务阶段拆分 Lottery 规则，保持终端选择纯度，并以真实证据驱动规则链、规则树与访问控制演进 |
| [ADR-0020](ADR-0020-lottery-strategy-cache-aside.md) | 已接受 | 以 MySQL 为唯一事实源、Redis 为可丢弃严格版本化投影，为 Lottery Strategy 读取建立 fail-open cache-aside 边界 |
| [ADR-0021](ADR-0021-participation-new-user-eligibility.md) | 已接受 | 以权威注册事实、具体决定与诚实失败语义建立首个 Participation 新用户资格切片，不提前伪造身份、事实表或通用规则引擎 |

新 ADR 从 [模板](template.md) 创建。已接受的 ADR 不直接重写结论；新事实需要新 ADR，并将旧记录标记为被替代。

课程从 96 节扩展为 101 节只调整尚未实施能力的学习顺序，不改变既有 ADR 的结论、生效时间或其历史章节引用；涉及长期访问控制边界的新决定将在对应实现章节另建 ADR。
