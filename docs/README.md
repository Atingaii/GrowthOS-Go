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
| 当前工程 | [仓库地图](architecture/repository-map.md) | 目录边界与当前阶段能力 |
| 前端工程 | [前端架构](frontend/frontend-architecture.md) | 四类工作台、路由、运行方式和 UI 基线 |
| API 契约 | [API 文档](api/README.md) | 按章节查看前端 API 新增、调整和联调状态 |
| 架构决策 | [ADR 索引](decisions/README.md) | 稳定决策、取舍和后果 |
| 质量证据 | [QA 索引](qa/README.md) | 测试策略、验收记录和已知风险 |
| 完成标准 | [Definition of Done](standards/definition-of-done.md) | 何时可以称为完成 |
| 文档治理 | [文档治理规范](standards/documentation-governance.md) | 文档归属和漂移控制 |
| 双代理协作 | [协作协议](standards/collaboration-protocol.md) | 仅 CCB 模式下激活的 Codex 规划/验收、Claude 实现/测试闭环规则 |
| Obsidian 同步 | [同步说明](standards/obsidian-sync.md) | 根 README/docs 单向镜像到个人 Vault；含每任务最终同步门禁（见协作协议） |

## 当前基线

- 当前完成：第 1～8 节和第 14 节前端整体框架。
- 当前代码：Go 产品 API 尚未开始；`web/` 已有可运行的 React/Vite/Tailwind UI 框架，页面当前使用 Mock 数据。
- 当前架构承诺：Go 优先、渐进式演进、数据库按需求迁移。
- 当前明确未做：业务 API、数据库表、Redis、MQ、微服务和 Java 实现；前端真实 API 联调留给第 15 节。

完整状态以 [课程状态台账](course/status.csv) 为准。

source edit
source conflict
