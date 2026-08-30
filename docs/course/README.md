# GrowthOS-Go 课程路线

本课程以需求推动系统演进，共 12 部分、101 节。第 1～3 部分保持已经形成的八节节奏；第 4 部分因第 22 节共享工作台暴露出的公共访问控制需求扩展为 13 节，第 5～12 部分继续每部分 8 节。当前只实施 Go 版本；Java 版本不属于本路线的交付范围，只在 Go 版完成并稳定后另行规划。

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
| 4 | 25～37 | 规则系统、公共访问控制与营销活动 | 规则链、规则树、Activity、公共权限模型、会话认证、前后端授权、越权验收、运营后台与阶段复盘 |
| 5 | 38～45 | 活动账户、订单与库存 | SKU、账户流水、订单、MySQL 库存、Redis Lua 与闭环 |
| 6 | 46～53 | MQ、最终一致性与补偿 | RocketMQ、Outbox、可靠投递、奖励快照与异常后台 |
| 7 | 54～61 | 积分、优惠券、返利与权益中心 | Reward、Ledger、Coupon、Rebate 与权益重构 |
| 8 | 62～69 | Growth Feed 与用户行为 | FeedItem、React Feed、Cursor、Behavior Event、画像 |
| 9 | 70～77 | Feed 推荐、实验与增长分析 | Candidate、Filtering、Ranking、A/B、ClickHouse 与闭环 |
| 10 | 78～85 | 模块化单体到分布式 | 服务拆分、gRPC、Nacos、Sentinel-Go、Trace、分片与 OpenSearch |
| 11 | 86～93 | AI MCP Gateway | JSON-RPC、MCP 生命周期、Gateway、动态 Tool、权限审计 |
| 12 | 94～101 | AI Agent、可观测、压测与上线 | LLM、Agent、审批、Guardrail、OTel、压测与 Kubernetes |

完整标题和实施状态以 [status.csv](status.csv) 为唯一状态源，结构性调整及编号迁移记录在[路线修订记录](route-revisions.md)。完成章节必须同时登记课程正文和 [QA 证据](../qa/README.md)，否则 `make doc-check` 失败。第 13 节起，每节还要交付独立的[第一性原理设计手记](../design-thinking/README.md)与[面试问答](../interview/README.md)；第 1～12、14 节按新增规范回填后，再把这两项纳入自动完成门禁。逐节学习时可按[课程分支检查点](branch-checkpoints.md)切换和比较实现分支；分支已存在不等于章节已经验收完成。

## 当前进度

当前已完成第 1～25 节，共 25 节；第二部分 M0 与第三部分 M1 均已验收。第三部分已经完成最小 Lottery 领域对象、第一组业务表、Strategy Create/FindByID 仓储、无偏加权 Award 选择、development/test 专用 ephemeral Lottery API、真实 React 消费者、规则事实所有权停止线，以及第一个以 MySQL 为事实源的 Redis Strategy 读取投影。第四部分由第 25 节开始：Participation 已拥有第一条基于权威注册事实、显式 freshness 与含边界 cutoff 的可执行资格规则，但尚未接入事实 adapter、公开 API、Lottery 或真实主体。完成数与证据路径以 [status.csv](status.csv) 为准。

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
- [第 21 节：开放第一个 Lottery API](part-03/lesson-21-lottery-api.md) 已完成并验收；配套 [API](../api/lessons/lesson-21.md)、[QA](../qa/lessons/lesson-21.md)、[设计手记](../design-thinking/lessons/lesson-21.md)、[面试问答](../interview/lessons/lesson-21.md)和 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md) 已登记。
- [第 22 节：实现第一个真实 React 抽奖页](part-03/lesson-22-react-lottery-page.md) 已完成并验收；配套 [API](../api/lessons/lesson-22.md)、[QA](../qa/lessons/lesson-22.md)、[设计手记](../design-thinking/lessons/lesson-22.md)和 [面试问答](../interview/lessons/lesson-22.md)已登记。
- [第 23 节：需求升级抽奖策略需要规则](part-03/lesson-23-lottery-strategy-rule-requirements.md) 已完成并验收；配套[需求基线](../product/lottery-rule-requirements-v1.md)、[API](../api/lessons/lesson-23.md)、[QA](../qa/lessons/lesson-23.md)、[设计手记](../design-thinking/lessons/lesson-23.md)、[面试问答](../interview/lessons/lesson-23.md)和 [ADR-0019](../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)已登记。
- [第 24 节：第一次 Redis 缓存](part-03/lesson-24-redis-strategy-cache.md) 已完成并验收；配套 [API](../api/lessons/lesson-24.md)、[QA](../qa/lessons/lesson-24.md)、[设计手记](../design-thinking/lessons/lesson-24.md)、[面试问答](../interview/lessons/lesson-24.md)、[运维手册](../runbooks/redis-strategy-cache.md)和 [ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)已登记。
- [第 25 节：需求升级——不是所有用户都能抽](part-04/lesson-25-user-eligibility.md) 已完成并验收；配套[规则基线](../product/new-user-eligibility-v1.md)、[API](../api/lessons/lesson-25.md)、[QA](../qa/lessons/lesson-25.md)、[设计手记](../design-thinking/lessons/lesson-25.md)、[面试问答](../interview/lessons/lesson-25.md)和 [ADR-0021](../decisions/ADR-0021-participation-new-user-eligibility.md)已登记。
- 第 11～23 节依次建立 Go/MySQL/React/Compose 基线、Lottery 领域/仓储/选择/API/React 纵向链和规则所有权停止线；第 24 节只在 application-owned `StrategyReader` 外增加 Lottery cache-aside decorator。MySQL 仍是唯一事实源，Redis value 经严格 v1 codec 与领域恢复；2 MiB/1000 Award、TTL≤5m+jitter、同 key fill、poison 修复、fail-open、最小 ACL 与低基数观测分别由适合它们的单元/边界测试或隔离 Compose 证据覆盖。服务器实际剩余 TTL、真实 2 MiB Redis value 和 Redis/MySQL 同时停止未被冒充为已执行场景，精确边界见第 24 节 QA。
- 当前真实 Lottery API 及其 React 消费者仍只产生并展示不持久化的 ephemeral selection；`reward` 只是奖励候选，`no_reward` 是正常候选结果。Redis 不缓存资格、权限、库存、随机选择或 Draw/Result，也不进入 API readiness。第 25 节资格代码仍是未装配的 Participation domain/application 内核；正式 Draw/Result、认证、对象级授权、幂等、完整资格组合与 Lottery 前置门控、库存、积分扣减和发奖均未实现；INV-03 仍未满足。
- M0 健康探针基线与 M1 Strategy 缓存本地基线均已形成。M1 三组 50 RPS×10s 均 500/500 成功：warm-cache MySQL prepared execute 为 0，cache-disabled 与 Redis-down 均为 1000；它只证明当前本机的命中/直连/fail-open 路径，不外推为业务 SLO、生产容量或通用缓存收益。第 45、61、77、85、93、101 节继续形成后续里程碑。
- 下一节是第 26 节“责任链实现前置规则”：先增加第二条由真实需求驱动的 Participation 前置判断，再从两个具体消费者反推最小线性短路协议，不提前制造通用规则平台。第 31～35 节仍按“公共模型 → 会话 → 服务端强制 → 前端感知 → 越权验收”演进，并在第 36 节首个真实运营后台复用。权限能力服务于用户端、运营端、MCP 和 Agent，而不是为每套界面各建一套角色开关。
- Go 完整版本结束后，才以稳定 Specification 为输入另行制定 Java 第二轮计划。

## 课程演进规则

1. 不为保持路线图编号美观而重写已发生的 Migration 历史。
2. 每个阶段保留当时合理方案及其失效条件，避免把终态倒灌到早期章节。
3. 新技术必须对应已观察或已论证的问题；技术清单不是验收清单。
4. 课程正文描述学习路径，第一性原理手记保留开放推导，ADR 固化稳定决策，QA 描述验证证据，章节 API 记录前端调用契约，面试文档负责准确口述与追问；六者不能互相替代。
5. 架构图和 ER 图都标注版本；最终完整图只在第 101 节基于实际实现生成。

## 前后端与数据库节奏

前端不是最终阶段补上的展示层。React + TypeScript 在第 14 节初始化，第 15 节完成首次 API 联调；抽奖页、运营后台、活动详情、积分/优惠券中心、Growth Feed、数据大盘、MCP 控制台和 AI Operator 依次在对应业务章节交付。

数据库同样不是预制终态。第 13 节先建立连接、账号隔离、readiness 与前向 Migration 机制；第 18 节才以 `000001` / `000002` 分别创建 `lottery_strategy`、`lottery_strategy_award`，当前 latest 为 2。拆成两个版本使每个版本只包含一条 MySQL DDL，并让第二张表失败时能明确报告 version 2 dirty；API 只有在全部迁移和精确授权成功后才启动。后续每一次字段、索引、新表、冗余快照或宽表变化，必须记录新业务需求、现有设计的不足、Migration、查询/并发影响和 QA 证据。已执行的 Migration 只追加，不回写历史。

## 可部署里程碑

| 里程碑 | 章节 | 可验证交付 |
| --- | --- | --- |
| M0 | 16 | Go + React + MySQL + Redis 开发环境与健康页 |
| M1 | 24 | Lottery 策略读取与临时选择性能基线 |
| M2 | 45 | 营销活动抽奖 MVP |
| M3 | 61 | 积分、优惠券、返利与权益中心 |
| M4 | 77 | Feed、行为、实验与分析反馈闭环 |
| M5 | 85 | 分布式 Growth Platform 与运营查询 |
| M6 | 93 | 可治理的 AI MCP Gateway |
| M7 | 101 | AI Agent、可观测、压测和 Kubernetes 上线 |
