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
| 课程实施 | [96 节路线](course/README.md) | 章节顺序、当前进度和演进约束 |
| 学习分支 | [课程分支检查点](course/branch-checkpoints.md) | 按章节切换、比较和核查实现分支 |
| 运行配置 | [配置参考](configuration.md) | `GROWTHOS_` 环境变量、默认值、校验与秘密边界 |
| 数据库运维 | [MySQL Migration 运维手册](runbooks/mysql-migrations.md) | 身份隔离、前向发布、故障停止条件与清理 |
| 本地开发栈 | [Docker Compose 运维手册](runbooks/local-compose.md) | Secret 生成、启停、M0 验收、故障定位与数据重置边界 |
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

- 当前完成：第 1～19 节，共 19 节；第二阶段 M0 已验收，第三阶段已完成最小 Lottery 领域建模、第一组业务表与 Strategy Create/FindByID 仓储，下一节是第 20 节实现最简单概率抽奖。
- 当前代码：已有可运行的 Gin 产品进程、类型化配置、`slog`、`request_id`、统一错误、`GET /health`、MySQL `GET /ready`、有界 `sqlx` 连接池和独立前向 Migration 命令；`000001` / `000002` 创建 `lottery_strategy` 与 `lottery_strategy_award`，latest 为 2；`internal/lottery/domain` 提供持久化无关的 Strategy/Award 聚合，`internal/lottery/application` 提供两个窄仓储端口，`internal/lottery/adapter/mysqlrepo` 用父子写事务和只读 RR 快照实现它们。Compose 以 `mysql → migrate → mysql-grants → api` 装配，四个常驻服务健康、两个 one-shot 成功退出；授权作业只经 Unix socket 且无网络，应用身份只可对两张业务表 `SELECT, INSERT`，不能 UPDATE、DELETE 或访问 `schema_migrations`，精确 grants 不匹配或 mandatory role 非空都会失败关闭。
- 当前架构承诺：Go 优先、渐进式演进、数据库按需求迁移。
- 当前明确未做：概率抽奖算法、Lottery 业务 API、真实 Lottery 前端、Strategy 更新/删除/Upsert、Redis 业务缓存、MQ、微服务和 Java 实现；Repository 也尚未装配进产品 HTTP 进程。Compose 中的 Redis 仍是隔离且易失的环境占位，不在 API readiness 中，系统状态之外的业务域尚未真实联调，INV-03 也尚无 Draw/Result 实现可供验证。两表行级 `updated_at` 不是聚合版本；数据库的 `*_name_basic` 只覆盖空串与首尾 ASCII 空格，完整聚合合法性由 Repository 恢复时重新校验。

完整状态以 [课程状态台账](course/status.csv) 为准。
