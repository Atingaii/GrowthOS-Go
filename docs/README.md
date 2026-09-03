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
| 访问控制基线 | [统一访问控制模型与威胁边界 v1](product/access-control-model-threat-boundary-v1.md) | Principal、exact capability、Role/Scope/Policy、拒绝语义与后续信任停止线 |
| 会话认证基线 | [Identity 与真实会话认证 v1](product/identity-session-authentication-v1.md) | workforce credential、server-side session、Cookie/CSRF、MySQL 权威与授权停止线 |
| 课程实施 | [101 节路线](course/README.md) | 章节顺序、当前进度和演进约束 |
| 路线修订 | [课程路线修订记录](course/route-revisions.md) | 结构性新增、编号迁移、历史保护与安排理由 |
| 学习分支 | [课程分支检查点](course/branch-checkpoints.md) | 按章节切换、比较和核查实现分支 |
| 运行配置 | [配置参考](configuration.md) | `GROWTHOS_` 环境变量、默认值、校验与秘密边界 |
| 数据库运维 | [MySQL Migration 运维手册](runbooks/mysql-migrations.md) | 身份隔离、前向发布、故障停止条件与清理 |
| 本地开发栈 | [Docker Compose 运维手册](runbooks/local-compose.md) | Secret 生成、启停、M0 验收、故障定位与数据重置边界 |
| Strategy 缓存运维 | [Redis Strategy Cache 运维手册](runbooks/redis-strategy-cache.md) | 缓存开关、精确 key、ACL、故障降级、恢复与安全清理 |
| 路由图求值运维 | [Strategy Routing Graph 求值验收与故障分诊](runbooks/strategy-routing-graph-evaluation.md) | 内部验收、预算/取消优先级、低披露分诊与未来装配停止条件 |
| Activity 发布运维 | [Activity Publication 运维与验收](runbooks/activity-publication.md) | exact 发布、回滚、CAS、resolve gate 与尚未装配边界 |
| 访问模型审查 | [访问控制模型审查手册](runbooks/access-control-model-review.md) | catalog/role/scope/deny 变更、离线紧急撤权分析、证据与回滚；不是线上事故手册 |
| 会话认证运维 | [Identity Session 运维手册](runbooks/identity-session-operations.md) | 账号创建、会话清理、Cookie/CSRF 验收、故障恢复与秘密清理边界 |
| 当前工程 | [仓库地图](architecture/repository-map.md) | 目录边界与当前阶段能力 |
| 前端工程 | [前端架构](frontend/frontend-architecture.md) | 四类工作台、路由、运行方式和 UI 基线 |
| API 契约 | [API 文档](api/README.md) | 按章节查看前端 API 新增、调整和联调状态 |
| 架构决策 | [ADR 索引](decisions/README.md) | 稳定决策、取舍和后果 |
| 质量证据 | [QA 索引](qa/README.md) | 测试策略、验收记录和已知风险 |
| 设计推导 | [第一性原理设计手记](design-thinking/README.md) | 按章节保留事实、方案权衡、失败模型、风险账本与重决策条件 |
| 面试复盘 | [面试问答索引](interview/README.md) | 按章节把设计、追问、证据与选型边界整理为可口述问答 |
| 视觉验收 | [当前设计 QA](../design-qa.md) | 第 32 节登录/当前会话页面与参考站点、桌面/移动和交互状态的对照证据 |
| 完成标准 | [Definition of Done](standards/definition-of-done.md) | 何时可以称为完成 |
| 文档治理 | [文档治理规范](standards/documentation-governance.md) | 文档归属和漂移控制 |
| Obsidian 同步 | [同步说明](standards/obsidian-sync.md) | 根 README/docs 单向镜像到个人 Vault |

## 当前基线

- 当前完成：第 1～32 节，共 32 节。第 32 节“真实会话认证”已经验收：Identity 现在拥有本地 workforce account、Argon2id credential verification、MySQL server-side session、双维度 throttle、session-bound CSRF 与 `POST/GET/DELETE /api/v1/session`；React 新增真实 `/login` 与 `/session` 会话体验。该完成声明只覆盖认证 development DoD，不包含第 33～35 节授权闭环或生产环境认证。
- 当前代码与验收：源码 Migration latest 为 `14`，历史 `000001`～`000011` 与第 30 节 v5→v11 的全部证据保持不变；`000012`～`000014` 追加 `identity_workforce_account`、`identity_session`、`identity_authentication_throttle`，总计十张既有业务表加三张 Identity 表。API 保持 `growthos_app` 业务 pool 与 `growthos_identity` 认证 pool 分离；受控账号创建再使用只可向 account 表 `INSERT` 的 `growthos_identity_provisioner`，DDL 仍只属于 migrator。独立 disposable MySQL 8.4.11、development HTTP wire、浏览器核心旅程及最终 Go/race/Web/doccheck 门禁已实际通过；未覆盖的生产 TLS/可信代理、完整 raw framing、真实 wire COMMIT 丢应答和更广设备/辅助技术矩阵继续以该节 QA 为准。
- 当前架构承诺：Go 优先、渐进式演进、数据库按需求迁移。
- 当前明确未做：生产注册/风险/会员 fact adapter、在线资格/会员路由/Activity resolve 门控的运行时编排、Activity 管理 API/UI、真实 Governance 审批提供方、正式 Lottery Draw/Result、服务端 RBAC enforcement 与 Policy/assignment repository、Resource tenant/owner/object facts 装配、前端 capability 投影、完整浏览器越权端到端验收、多租户隔离、幂等、库存、发奖、Strategy 更新/精准失效、MQ、微服务和 Java 实现。登录成功只建立 trusted human Principal；`/admin/*`、`/mcp/*`、`/agent/*` 与现有业务 route 仍未因第 32 节自动受保护，导航也尚未按权限裁剪。

第 24 节完整证据链：[ADR-0020](decisions/ADR-0020-lottery-strategy-cache-aside.md)、[课程正文](course/part-03/lesson-24-redis-strategy-cache.md)、[API 零变化记录](api/lessons/lesson-24.md)、[QA](qa/lessons/lesson-24.md)、[第一性原理手记](design-thinking/lessons/lesson-24.md)、[面试问答](interview/lessons/lesson-24.md)和[运维手册](runbooks/redis-strategy-cache.md)。第 21 节后端边界仍由 [ADR-0018](decisions/ADR-0018-ephemeral-lottery-selection-api.md)约束；缓存没有把 ephemeral selection 升级成正式 Draw。

第 25 节完整证据链：[新用户资格规则基线](product/new-user-eligibility-v1.md)、[ADR-0021](decisions/ADR-0021-participation-new-user-eligibility.md)、[课程正文](course/part-04/lesson-25-user-eligibility.md)、[API 零变化记录](api/lessons/lesson-25.md)、[QA](qa/lessons/lesson-25.md)、[第一性原理手记](design-thinking/lessons/lesson-25.md)和[面试问答](interview/lessons/lesson-25.md)。该证据只证明 Participation 内核的确定性与失败语义，不证明真实身份、外部事实适配、公开资格 API 或 Lottery 前置门控。

第 26 节完整证据链：[前置资格链基线](product/participation-prerequisite-chain-v1.md)、[ADR-0022](decisions/ADR-0022-participation-prerequisite-chain.md)、[课程正文](course/part-04/lesson-26-responsibility-chain.md)、[API 零变化记录](api/lessons/lesson-26.md)、[QA](qa/lessons/lesson-26.md)、[第一性原理手记](design-thinking/lessons/lesson-26.md)和[面试问答](interview/lessons/lesson-26.md)。该证据只证明固定 Participation gate chain 的顺序、短路、时间和失败语义，不证明线上抽奖已受资格保护。

第 27 节完整证据链：[会员策略路由基线](product/membership-strategy-routing-v1.md)、[ADR-0023](decisions/ADR-0023-membership-strategy-routing-boundary.md)、[课程正文](course/part-04/lesson-27-responsibility-chain-limits.md)、[API 零变化记录](api/lessons/lesson-27.md)、[QA](qa/lessons/lesson-27.md)、[第一性原理手记](design-thinking/lessons/lesson-27.md)和[面试问答](interview/lessons/lesson-27.md)。该证据只证明 Lottery 内部的具体会员路由、显式默认分支、路径与失败语义，不证明已有会员事实 adapter、公开 API、运行时装配、Activity、权限控制或浏览器端到端链路。

第 28 节完整证据链：[Strategy Routing Graph 基线](product/lottery-strategy-routing-graph-v1.md)、[ADR-0024](decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)、[课程正文](course/part-04/lesson-28-rule-tree-schema.md)、[API 零变化记录](api/lessons/lesson-28.md)、[QA](qa/lessons/lesson-28.md)、[第一性原理手记](design-thinking/lessons/lesson-28.md)和[面试问答](interview/lessons/lesson-28.md)。该证据只证明 Lottery-owned graph 的结构、不可变 identity、latest 5 schema、Repository 事务/恢复语义和隔离 MySQL 8.4.11 权限，不证明图已经执行、发布、绑定 Activity、进入运行时或获得任何 UI/权限能力。

第 29 节完整证据链：[Strategy Routing Graph 求值基线](product/lottery-strategy-routing-evaluation-v1.md)、[ADR-0025](decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)、[课程正文](course/part-04/lesson-29-rule-decision-engine.md)、[API 零变化记录](api/lessons/lesson-29.md)、[QA](qa/lessons/lesson-29.md)、[第一性原理手记](design-thinking/lessons/lesson-29.md)、[面试问答](interview/lessons/lesson-29.md)和[运维/验收手册](runbooks/strategy-routing-graph-evaluation.md)。该证据只证明 exact graph 的封闭确定求值、一次 fact/as-of、完整 path、step/time/cancel 预算、固定错误优先级与 zero-decision 协议，不证明 graph 已发布、绑定 Activity、进入 runtime/API/UI、获得真实会员事实或通过认证授权。

第 30 节完整证据链：[Activity Publication 绑定基线](product/activity-publication-binding-v1.md)、[ADR-0026](decisions/ADR-0026-activity-publication-binding.md)、[课程正文](course/part-04/lesson-30-strategy-vs-activity.md)、[API 零变化记录](api/lessons/lesson-30.md)、[QA](qa/lessons/lesson-30.md)、[第一性原理手记](design-thinking/lessons/lesson-30.md)、[面试问答](interview/lessons/lesson-30.md)和[运维/验收手册](runbooks/activity-publication.md)。它证明 exact snapshot/publication/CAS/rollback/gate/ACL 与 v11 工程边界；本节源码另提供 commit-outcome-unknown receipt 三态对账。两者都不证明运行时装配、真实审批、API/UI、访问控制或正式 Draw。

第 31 节完整证据链：[访问控制模型基线](product/access-control-model-threat-boundary-v1.md)、[ADR-0027](decisions/ADR-0027-governance-access-control-model.md)、[课程正文](course/part-04/lesson-31-access-control-model-threat-boundary.md)、[API 零变化记录](api/lessons/lesson-31.md)、[QA](qa/lessons/lesson-31.md)、[第一性原理手记](design-thinking/lessons/lesson-31.md)、[面试问答](interview/lessons/lesson-31.md)和[模型审查手册](runbooks/access-control-model-review.md)。它证明纯 Policy language/evaluator 的 exact、不可变、默认拒绝与停止线，不证明 caller 已认证、Resource facts 已可信加载、API 已强制或 UI 已按权限裁剪。

第 32 节已验收证据链：[Identity 会话基线](product/identity-session-authentication-v1.md)、[ADR-0028](decisions/ADR-0028-identity-session-authentication.md)、[课程正文](course/part-04/lesson-32-real-session-authentication.md)、[API 契约](api/lessons/lesson-32.md)、[QA](qa/lessons/lesson-32.md)、[第一性原理手记](design-thinking/lessons/lesson-32.md)、[面试问答](interview/lessons/lesson-32.md)、[运维手册](runbooks/identity-session-operations.md)与[当前设计 QA](../design-qa.md)。认证绝不等于第 33 节授权、第 34 节前端裁剪或第 35 节越权验收，生产环境与未执行故障矩阵也没有被 development PASS 替代。

完整状态以 [课程状态台账](course/status.csv) 为准。
