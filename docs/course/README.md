# GrowthOS-Go 课程路线

本课程以需求推动系统演进，共 12 部分、96 节，每部分 8 节。当前只实施 Go 版本；Java 版本不属于本路线的交付范围，只在 Go 版完成并稳定后另行规划。

## 学习闭环

每一节按适用程度采用同一交付闭环：

```text
业务诉求 -> 当前问题 -> 方案分析 -> 领域建模 -> 数据库变化
-> Migration -> Go 工程调整 -> 实现 -> 测试 -> 验证
-> QA 证据 -> Git Commit -> 复盘 -> 下一节问题
```

分析章节不会为了填模板虚构代码或数据库变化。实现章节也不能省略决策、测试和文档。

## 十二部分

| 部分 | 章节 | 主题 | 阶段出口 |
| --- | --- | --- | --- |
| 1 | 1～8 | 产品需求与系统分析 | 产品边界、用户旅程、领域 v1、NFR 与系统 V0 |
| 2 | 9～16 | Go + React 从零搭建 | Go、Gin、MySQL、Migration、React 联调与 Compose |
| 3 | 17～24 | 从两张表开始做抽奖 | Strategy、Award、Lottery API、React 抽奖页与 Redis |
| 4 | 25～32 | 规则系统与营销活动 | 规则链、规则树、Activity、运营后台与阶段复盘 |
| 5 | 33～40 | 活动账户、订单与库存 | SKU、账户流水、订单、MySQL 库存、Redis Lua 与闭环 |
| 6 | 41～48 | MQ、最终一致性与补偿 | RocketMQ、Outbox、可靠投递、奖励快照与异常后台 |
| 7 | 49～56 | 积分、优惠券、返利与权益中心 | Reward、Ledger、Coupon、Rebate 与权益重构 |
| 8 | 57～64 | Growth Feed 与用户行为 | FeedItem、React Feed、Cursor、Behavior Event、画像 |
| 9 | 65～72 | Feed 推荐、实验与增长分析 | Candidate、Filtering、Ranking、A/B、ClickHouse 与闭环 |
| 10 | 73～80 | 模块化单体到分布式 | 服务拆分、gRPC、Nacos、Sentinel-Go、Trace、分片与 OpenSearch |
| 11 | 81～88 | AI MCP Gateway | JSON-RPC、MCP 生命周期、Gateway、动态 Tool、权限审计 |
| 12 | 89～96 | AI Agent、可观测、压测与上线 | LLM、Agent、审批、Guardrail、OTel、压测与 Kubernetes |

完整标题和实施状态以 [status.csv](status.csv) 为唯一状态源。完成章节必须同时登记课程正文和 [QA 证据](../qa/README.md)，否则 `make doc-check` 失败。第 13 节起，每节还要交付独立的[第一性原理设计手记](../design-thinking/README.md)与[面试问答](../interview/README.md)；第 1～12、14 节按新增规范回填后，再把这两项纳入自动完成门禁。逐节学习时可按[课程分支检查点](branch-checkpoints.md)切换和比较实现分支；分支已存在不等于章节已经验收完成。

## 当前进度

当前已完成第 1～21 节，共 21 节；第二部分和 M0 工程里程碑均已验收，第三部分已经完成最小 Lottery 领域对象、第一组业务表、Strategy Create/FindByID 仓储、无偏加权 Award 选择，以及 development/test 专用的首个 ephemeral Lottery API。

- [第 1 节：为什么要做 AI 原生大营销增长平台](part-01/lesson-01-why-ai-native-growth-platform.md) 已完成。
- [第 2 节：梳理完整用户增长旅程](part-01/lesson-02-user-growth-journey.md) 已完成。
- [第 3 节：设计运营人员工作流](part-01/lesson-03-operator-workflow.md) 已完成。
- [第 4 节：设计 AI Operator 工作流](part-01/lesson-04-ai-operator-workflow.md) 已完成。
- [第 5 节：事件风暴第一次领域分析](part-01/lesson-05-first-event-storm.md) 已完成。
- [第 6 节：第一次划分限界上下文](part-01/lesson-06-first-bounded-contexts.md) 已完成。
- [第 7 节：确定非功能需求](part-01/lesson-07-non-functional-requirements.md) 已完成。
- [第 8 节：画 V0 系统设计](part-01/lesson-08-v0-system-design.md) 已完成。
- [第 9 节：创建 GrowthOS-Go 仓库](part-02/lesson-09-create-growthos-go-repository.md) 已完成。
- [第 10 节：为什么第一版不直接微服务](part-02/lesson-10-modular-monolith-first.md) 已完成。
- [第 11 节：使用 Gin 初始化 Go Web 服务](part-02/lesson-11-gin-http-service.md) 已完成。
- [第 12 节：配置、日志与错误码体系](part-02/lesson-12-config-logging-errors.md) 已完成并验收。
- [第 13 节：接入 MySQL 与 Migration](part-02/lesson-13-mysql-migrations.md) 已完成并验收。
- [第 14 节：React TypeScript 前端工程初始化](part-02/lesson-14-react-frontend-framework.md) 已完成。
- [第 15 节：前后端第一次联调](part-02/lesson-15-first-fullstack-integration.md) 已完成并验收。
- [第 16 节：Docker Compose 开发环境](part-02/lesson-16-docker-compose-development.md) 已完成并验收。
- [第 17 节：最简单随机抽奖需要什么对象](part-03/lesson-17-lottery-domain-objects.md) 已完成并验收。
- [第 18 节：第一次正式业务建表](part-03/lesson-18-lottery-schema.md) 已完成并验收。
- [第 19 节：实现仓储层](part-03/lesson-19-lottery-repository.md) 已完成并验收。
- [第 20 节：实现最简单概率抽奖](part-03/lesson-20-lottery-weighted-selection.md) 已完成并验收。
- [第 21 节：开放第一个 Lottery API](part-03/lesson-21-lottery-api.md) 已完成并验收；配套 [API](../api/lessons/lesson-21.md)、[QA](../qa/lessons/lesson-21.md)、[设计手记](../design-thinking/lessons/lesson-21.md)、[面试问答](../interview/lessons/lesson-21.md)和 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md) 已登记；下一节是第 22 节接入真实 React 抽奖页。
- 第 11 节建立最小 Go HTTP 进程和无依赖 `GET /health`；第 12 节集中配置、`slog`、`request_id` 和统一错误 envelope；第 13 节增加 MySQL 启动连接、`GET /ready` 与独立 Migration 命令；第 15 节让 React 系统状态页真实消费两个探针；第 16 节把 Web、API、一次性 Migration、MySQL 与隔离的 Redis 占位装配为仅暴露同源 Web 入口的可复现开发栈；第 17 节用纯 Go 领域对象定义 Lottery Strategy/Award；第 18 节创建两张业务表并收敛启动授权链；第 19 节以窄端口、父子写事务、只读 RR 快照和恢复校验实现 Strategy 仓储；第 20 节用 bounded crypto source 与减法桶实现完整 uint64 范围的加权 Award 选择；第 21 节以 `EphemeralSelectionService` 和 HTTP adapter 组合出默认关闭、仅 development/test 可启用的 `POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections`。
- 当前已有一个真实后端 Lottery API，但它只产生不持久化的 ephemeral selection；正式 Draw/Result、认证、对象级授权、幂等、参与资格、库存、发奖与 Redis 业务缓存均未实现。运行身份只对两张业务表拥有 `SELECT`，不能 INSERT、UPDATE、DELETE 或访问 `schema_migrations`；需要写 fixture 的历史 Repository 集成测试使用隔离测试身份。React `/lottery` 仍使用客户端 `Math.random()` Mock，只有系统状态页完成真实前端联调。不能把临时选择、64 个请求且最大并行 16 的 acceptance 或探针负载外推为在线抽奖闭环、64 并发/64 RPS 或业务 SLO，INV-03 仍未满足。
- 第 16 节已经形成 M0：正式健康探针负载在本机 Docker Desktop 上以 100 RPS 持续 5 分钟完成 30,000/30,000 次请求，P99 为 4.1495 ms；该结果只证明当前工程探针和本地栈，不外推为业务 SLO。第 24、40、56、72、80、88、96 节继续形成后续里程碑。
- Go 完整版本结束后，才以稳定 Specification 为输入另行制定 Java 第二轮计划。

## 课程演进规则

1. 不为保持路线图编号美观而重写已发生的 Migration 历史。
2. 每个阶段保留当时合理方案及其失效条件，避免把终态倒灌到早期章节。
3. 新技术必须对应已观察或已论证的问题；技术清单不是验收清单。
4. 课程正文描述学习路径，第一性原理手记保留开放推导，ADR 固化稳定决策，QA 描述验证证据，章节 API 记录前端调用契约，面试文档负责准确口述与追问；六者不能互相替代。
5. 架构图和 ER 图都标注版本；最终完整图只在第 96 节基于实际实现生成。

## 前后端与数据库节奏

前端不是最终阶段补上的展示层。React + TypeScript 在第 14 节初始化，第 15 节完成首次 API 联调；抽奖页、运营后台、活动详情、积分/优惠券中心、Growth Feed、数据大盘、MCP 控制台和 AI Operator 依次在对应业务章节交付。

数据库同样不是预制终态。第 13 节先建立连接、账号隔离、readiness 与前向 Migration 机制；第 18 节才以 `000001` / `000002` 分别创建 `lottery_strategy`、`lottery_strategy_award`，当前 latest 为 2。拆成两个版本使每个版本只包含一条 MySQL DDL，并让第二张表失败时能明确报告 version 2 dirty；API 只有在全部迁移和精确授权成功后才启动。后续每一次字段、索引、新表、冗余快照或宽表变化，必须记录新业务需求、现有设计的不足、Migration、查询/并发影响和 QA 证据。已执行的 Migration 只追加，不回写历史。

## 可部署里程碑

| 里程碑 | 章节 | 可验证交付 |
| --- | --- | --- |
| M0 | 16 | Go + React + MySQL + Redis 开发环境与健康页 |
| M1 | 24 | 在线抽奖 MVP |
| M2 | 40 | 营销活动抽奖 MVP |
| M3 | 56 | 积分、优惠券、返利与权益中心 |
| M4 | 72 | Feed、行为、实验与分析反馈闭环 |
| M5 | 80 | 分布式 Growth Platform 与运营查询 |
| M6 | 88 | 可治理的 AI MCP Gateway |
| M7 | 96 | AI Agent、可观测、压测和 Kubernetes 上线 |
