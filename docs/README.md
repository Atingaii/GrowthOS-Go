# GrowthOS-Go 文档

这里是项目事实的入口。代码回答“系统现在如何运行”，文档回答“为什么这样运行、边界是什么、如何证明它正确”。两者必须在同一次变更中更新。

## 导航

| 主题 | 权威文档 | 作用 |
| --- | --- | --- |
| 产品定位 | [产品简述](product/product-brief.md) | 目标用户、问题、范围和成功信号 |
| 用户旅程 | [用户增长旅程 v1](product/user-growth-journey-v1.md) | 从触达、参与、权益到再次触达的完整体验 |
| 运营流程 | [运营人员工作流 v1](product/operator-workflow-v1.md) | 从目标、配置、审批到发布、止损和复盘 |
| AI 运营 | [AI Operator 工作流 v1](product/ai-operator-workflow-v1.md) | 自然语言、结构化计划、Tool、审批与审计边界 |
| 领域分析 | [领域事件地图 v1](product/domain-event-map-v1.md) | 命令、事件、查询、策略、失败和补偿的统一基线 |
| 领域边界 | [限界上下文地图 v1](product/bounded-context-map-v1.md) | 业务语言、职责、事实所有权和协作关系 |
| 质量目标 | [非功能需求基线 v1](product/non-functional-requirements-v1.md) | 容量、延迟、一致性、恢复、安全和降级目标 |
| 系统设计 | [GrowthOS 系统设计 V0](product/system-design-v0.md) | 产品架构、系统上下文、用例、领域关系与近期运行形态 |
| 课程实施 | [101 节路线](course/README.md) | 章节顺序、当前进度和演进约束 |
| 路线修订 | [课程路线修订记录](course/route-revisions.md) | 结构性新增、编号迁移、历史保护与安排理由 |
| 学习分支 | [课程分支检查点](course/branch-checkpoints.md) | 按章节切换、比较和核查实现分支 |
| 运行配置 | [配置参考](configuration.md) | `GROWTHOS_` 环境变量、默认值、校验与秘密边界 |
| 数据库运维 | [MySQL Migration 运维手册](runbooks/mysql-migrations.md) | 身份隔离、前向发布、故障停止条件与清理 |
| 本地开发栈 | [Docker Compose 运维手册](runbooks/local-compose.md) | Secret 生成、启停、M0 验收、故障定位与数据重置边界 |
| Strategy 缓存运维 | [Redis Strategy Cache 运维手册](runbooks/redis-strategy-cache.md) | 缓存开关、精确 key、ACL、故障降级、恢复与安全清理 |
| 当前工程 | [仓库地图](architecture/repository-map.md) | 目录边界与当前阶段能力 |
| 前端工程 | [前端架构](frontend/frontend-architecture.md) | 四类工作台、路由、运行方式和 UI 基线 |
| API 契约 | [API 文档](api/README.md) | 按章节查看前端 API 新增、调整和联调状态 |
| 架构决策 | [ADR 索引](decisions/README.md) | 稳定决策、取舍和后果 |
| 质量证据 | [QA 索引](qa/README.md) | 测试策略、验收记录和已知风险 |
| 设计推导 | [第一性原理设计手记](design-thinking/README.md) | 按章节保留事实、方案权衡、失败模型、风险账本与重决策条件 |
| 面试复盘 | [面试问答索引](interview/README.md) | 按章节把设计、追问、证据与选型边界整理为可口述问答 |
| 完成标准 | [Definition of Done](standards/definition-of-done.md) | 何时可以称为完成 |
| 文档治理 | [文档治理规范](standards/documentation-governance.md) | 文档归属和漂移控制 |
| Obsidian 同步 | [同步说明](standards/obsidian-sync.md) | 根 README/docs 单向镜像到个人 Vault |

## 当前基线

- 当前完成：第 1～27 节，共 27 节；第二阶段 M0 与第三阶段 M1 均已验收。第三阶段已完成 Lottery 领域、两表、仓储、无偏选择、ephemeral API/React、规则所有权与 Redis Strategy 投影。第 25～26 节在 Participation 中依次交付新用户资格、风险准入与固定有序短路链；第 27 节在 Lottery 中用封闭会员等级、显式 premium/standard 分支和一跳 path 交付首个具体策略路由，证明线性 gate chain 不足以承担分支选择。第四阶段仍在进行中，下一步第 28 节首次为规则树升级数据库；公共访问控制仍按第 31～35 节推进。
- 当前代码：已有可运行的 Gin 产品进程、类型化配置、`slog`、`request_id`、统一错误、`GET /health`、MySQL `GET /ready`、有界 `sqlx`/Redis pool 和独立前向 Migration 命令；`000001` / `000002` 创建两张 Lottery 表，latest 为 2。`StrategyReader` 可按开关被 cache-aside 装饰。Participation 现有 consumer-owned `RegistrationFactReader`、`RiskScreeningFactReader`、具体 policy/decision 与 `EligibilityPrerequisiteChain`。Lottery 新增 consumer-owned `MembershipTierFactReader`、具体 routing policy/decision/path 与 application service；它只证明完整 fact/policy snapshot 和同一 as-of 下的确定性路由，policy revision 尚不是内容 registry 或哈希唯一性证明。上述资格与会员能力均没有生产 adapter、表/缓存、公开 HTTP API 或 composition-root/运行时装配；现有 ephemeral Lottery 契约与 React 消费者不变且尚无资格或会员路由门控。Compose 与 Redis ACL/readiness 边界均未改变。
- 当前架构承诺：Go 优先、渐进式演进、数据库按需求迁移。
- 当前明确未做：生产注册/风险/会员 fact adapter、在线资格与会员路由门控、会员路由公开 API、正式 Lottery Draw/Result、登录认证、RBAC/对象级授权、前端权限投影、浏览器端到端验收、幂等、规则树/Activity、库存、发奖、Strategy 更新/精准失效、MQ、微服务和 Java 实现。React `/lottery` 只展示不可恢复的 ephemeral Award 选择；其余工作台业务仍为明确 Mock/本地交互。Redis 不缓存资格、会员事实、权限、库存、随机选择或最终结果；M1 本机证据不是业务 SLO。INV-03 仍无最终结果事实可供验证。

第 24 节完整证据链：[ADR-0020](decisions/ADR-0020-lottery-strategy-cache-aside.md)、[课程正文](course/part-03/lesson-24-redis-strategy-cache.md)、[API 零变化记录](api/lessons/lesson-24.md)、[QA](qa/lessons/lesson-24.md)、[第一性原理手记](design-thinking/lessons/lesson-24.md)、[面试问答](interview/lessons/lesson-24.md)和[运维手册](runbooks/redis-strategy-cache.md)。第 21 节后端边界仍由 [ADR-0018](decisions/ADR-0018-ephemeral-lottery-selection-api.md)约束；缓存没有把 ephemeral selection 升级成正式 Draw。

第 25 节完整证据链：[新用户资格规则基线](product/new-user-eligibility-v1.md)、[ADR-0021](decisions/ADR-0021-participation-new-user-eligibility.md)、[课程正文](course/part-04/lesson-25-user-eligibility.md)、[API 零变化记录](api/lessons/lesson-25.md)、[QA](qa/lessons/lesson-25.md)、[第一性原理手记](design-thinking/lessons/lesson-25.md)和[面试问答](interview/lessons/lesson-25.md)。该证据只证明 Participation 内核的确定性与失败语义，不证明真实身份、外部事实适配、公开资格 API 或 Lottery 前置门控。

第 26 节完整证据链：[前置资格链基线](product/participation-prerequisite-chain-v1.md)、[ADR-0022](decisions/ADR-0022-participation-prerequisite-chain.md)、[课程正文](course/part-04/lesson-26-responsibility-chain.md)、[API 零变化记录](api/lessons/lesson-26.md)、[QA](qa/lessons/lesson-26.md)、[第一性原理手记](design-thinking/lessons/lesson-26.md)和[面试问答](interview/lessons/lesson-26.md)。该证据只证明固定 Participation gate chain 的顺序、短路、时间和失败语义，不证明线上抽奖已受资格保护。

第 27 节完整证据链：[会员策略路由基线](product/membership-strategy-routing-v1.md)、[ADR-0023](decisions/ADR-0023-membership-strategy-routing-boundary.md)、[课程正文](course/part-04/lesson-27-responsibility-chain-limits.md)、[API 零变化记录](api/lessons/lesson-27.md)、[QA](qa/lessons/lesson-27.md)、[第一性原理手记](design-thinking/lessons/lesson-27.md)和[面试问答](interview/lessons/lesson-27.md)。该证据只证明 Lottery 内部的具体会员路由、显式默认分支、路径与失败语义，不证明已有会员事实 adapter、公开 API、运行时装配、Activity、权限控制或浏览器端到端链路。下一步第 28 节才首次为规则树升级数据库结构。

完整状态以 [课程状态台账](course/status.csv) 为准。
