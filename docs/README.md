# GrowthOS-Go 文档

这里是项目事实的入口。代码回答“系统现在如何运行”，文档回答“为什么这样运行、边界是什么、如何证明它正确”。两者必须在同一次变更中更新。

## 导航

| 主题 | 权威文档 | 作用 |
| --- | --- | --- |
| 产品定位 | [产品简述](product/product-brief.md) | 目标用户、问题、范围和成功信号 |
| 课程实施 | [96 节路线](course/README.md) | 章节顺序、当前进度和演进约束 |
| 当前工程 | [仓库地图](architecture/repository-map.md) | 目录边界与当前阶段能力 |
| 前端工程 | [前端架构](frontend/frontend-architecture.md) | 四类工作台、路由、运行方式和 UI 基线 |
| 架构决策 | [ADR 索引](decisions/README.md) | 稳定决策、取舍和后果 |
| 质量证据 | [QA 索引](qa/README.md) | 测试策略、验收记录和已知风险 |
| 完成标准 | [Definition of Done](standards/definition-of-done.md) | 何时可以称为完成 |
| 文档治理 | [文档治理规范](standards/documentation-governance.md) | 文档归属和漂移控制 |
| Obsidian 同步 | [双向同步说明](standards/obsidian-sync.md) | 项目文档与 Obsidian Vault 的同步规则 |

## 当前基线

- 当前完成：第 1 节和第 14 节前端整体框架。
- 当前代码：Go 产品 API 尚未开始；`web/` 已有可运行的 React/Vite/Tailwind UI 框架，页面当前使用 Mock 数据。
- 当前架构承诺：Go 优先、渐进式演进、数据库按需求迁移。
- 当前明确未做：业务 API、数据库表、Redis、MQ、微服务和 Java 实现；前端真实 API 联调留给第 15 节。

完整状态以 [课程状态台账](course/status.csv) 为准。

source edit
source conflict
