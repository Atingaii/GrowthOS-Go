# 产品简述：GrowthOS-Go

**状态：** v11 基线
**更新日期：** 2026-08-30
**来源章节：** [第 1 节](../course/part-01/lesson-01-why-ai-native-growth-platform.md)、[第 2 节](../course/part-01/lesson-02-user-growth-journey.md)、[第 3 节](../course/part-01/lesson-03-operator-workflow.md)、[第 4 节](../course/part-01/lesson-04-ai-operator-workflow.md)、[第 5 节](../course/part-01/lesson-05-first-event-storm.md)、[第 6 节](../course/part-01/lesson-06-first-bounded-contexts.md)、[第 7 节](../course/part-01/lesson-07-non-functional-requirements.md)、[第 17 节](../course/part-03/lesson-17-lottery-domain-objects.md)、[第 18 节](../course/part-03/lesson-18-lottery-schema.md)、[第 19 节](../course/part-03/lesson-19-lottery-repository.md)、[第 20 节](../course/part-03/lesson-20-lottery-weighted-selection.md)、[第 21 节](../course/part-03/lesson-21-lottery-api.md)、[第 22 节](../course/part-03/lesson-22-react-lottery-page.md)

## 一句话定位

GrowthOS-Go 是面向电商、出行、金融、内容和游戏等高流量业务的 AI 原生营销增长平台：统一承载营销活动、奖励权益、个性化触达、行为反馈与运营自动化。

## 要解决的问题

业务团队通常能快速做出一次活动，却难以持续复用策略、权益、流量和数据能力。烟囱式活动进一步造成规则重复、权益口径不一、用户触达割裂、效果无法归因，以及高风险运营操作缺少审计。

GrowthOS-Go 要把一次性项目沉淀成三层能力：

1. **Business：** 营销活动、抽奖、积分、优惠券和用户奖励。
2. **Growth：** Feed、行为采集、画像、实验和分析形成反馈闭环。
3. **AI Native：** MCP Gateway 将受控业务能力开放为工具，Agent 辅助查询、规划和执行。

## 目标用户

| 角色 | 核心任务 | 当前痛点 |
| --- | --- | --- |
| 消费者 | 发现并参与相关活动，领取和使用权益 | 触达重复、规则不透明、权益割裂 |
| 运营人员 | 创建活动、配置人群与策略、观察效果 | 依赖研发、配置分散、试错周期长 |
| 数据分析人员 | 还原漏斗、实验效果与用户路径 | 事件口径不一、链路不可关联 |
| 平台研发与 SRE | 提供稳定、可扩展、可诊断的平台 | 重复建设、热点流量、一致性与排障困难 |
| 审批与风控人员 | 控制高风险运营动作 | 权限、审批和审计链不完整 |

## 第一阶段范围

- 只建设 Go 完整版本。
- 以 101 节演进式课程推进，先业务、后增长、再 AI Native；新增的五节公共访问控制能力位于真实运营后台之前，而不是在尚无受保护业务对象时提前搭建。
- 数据库、领域模型和部署拓扑只在当前需求需要时演进。
- Go 版完成并形成稳定业务 Specification 后，Java 版仅作为后续独立计划重新评估和规划。

## 当前非目标

- 不在项目初始化时创建最终 ER 图或全部业务表。
- 不在流量和团队边界尚未证明前拆分全部微服务。
- Go 后端技术基线确定为 Gin、gRPC + Protobuf、sqlx + 手写 SQL、go-redis、RocketMQ、Nacos、Sentinel-Go 和 OpenTelemetry；组件仍按需求逐步接入。
- 前端技术基线确定为 React、TypeScript、Vite、Tailwind CSS、Lucide、Recharts 与 Zustand；第 14 节完成统一 UI 框架，后续章节接入真实业务。
- 不把 AI 直接连接数据库，也不允许 Agent 绕过业务权限执行写操作。
- 不在 Go 第一阶段维护 Java 镜像实现。

## 当前产品实现切片

第 17 节把 Lottery 最小业务语言落成纯 Go 领域对象：`Strategy` 管理至少一个 `Award`，候选使用正整数相对权重，并显式区分 `reward` 与 `no_reward`。第 18 节用 `000001` / `000002` 分别创建 `lottery_strategy` 与 `lottery_strategy_award`，保存 Strategy 根行和隶属于它的 Award 候选；当前产品 Migration latest 为 2。第 19 节再以两个窄 application 端口和 MySQL adapter 实现 Strategy Create/FindByID：完整父子聚合原子写入，一次读取来自同一个只读 RR 快照，恢复时重新验证领域不变量。第 20 节在领域层增加 `WeightedSelector` 与 consumer-owned `BoundedRandomSource`，多候选从均匀 `[0,totalWeight)` 整数位置映射到规范排序的 Award，生产 adapter 使用 `crypto/rand.Int` 并覆盖完整 `uint64` 范围；单候选不消耗随机源，`no_reward` 仍是正常选择结果。第 21 节以 `EphemeralSelectionService` 组合只读 Repository 与 Selector，并通过 `POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections` 暴露第一次真实纵向调用；完整 uint64 identity 以十进制 string 传输，路由默认关闭且只允许 development/test 显式打开。第 22 节增加同源 bodyless POST transport、严格 Lottery runtime decoder 与 React 请求状态 Hook，使 `/lottery` 真实消费该临时接口，并在共享工作台壳层中区分 pending、`reward`、`no_reward`、服务端拒绝、网络故障、超时、取消与契约漂移。

数据库只承担它能可靠表达的子集：正 ID/权重、Strategy 内 AwardID 唯一、封闭 outcome、引用完整性以及基础名称形态。`*_name_basic` 只拒绝空串和首尾 ASCII U+0020 空格，不等价于领域层完整名称契约；外键不能保证至少一个 Award，单行约束也不能验证跨行总权重是否溢出。两表的 `updated_at` 是行级元数据，不是聚合版本，Award 更新不会自动推进 Strategy 时间戳。

当前 Compose 运行身份仅可对两张业务表 `SELECT`，不能 INSERT、UPDATE、DELETE 或访问 `schema_migrations`；需要创建 fixture 的 Repository 集成测试使用可丢弃 schema 中的隔离 writer 身份。GrowthOS 已有可验证的领域、持久化结构、内部 Repository、加权选择器、一个真实后端业务路由及其 React 消费者，但还没有正式 Draw API、认证、RBAC、持久化结果或 Redis 业务缓存。除系统状态页和 `/lottery` 外，其他用户、运营、MCP 与 Agent 页面仍是明确标注的 Mock 快照或浏览器本地交互。

尤其，ephemeral API 返回的 Award 仍是一次瞬时计算结果，不是带 DrawID、幂等键和持久化状态的一次用户抽奖最终事实；`reward` 也不表示积分或优惠券已经进入 Benefit 发放生命周期。当前没有认证、对象级授权、Participation 资格、Draw/Result、库存或发奖实现，因此“一次抽奖只能有一个最终结果”仍是待后续章节验证的业务不变量。调用超时后直接重试会形成一次新的临时选择，不能被描述为安全的业务重试。

第 21 节服务端事实、契约和取舍分别见[课程正文](../course/part-03/lesson-21-lottery-api.md)、[API](../api/lessons/lesson-21.md)、[QA](../qa/lessons/lesson-21.md)、[第一性原理手记](../design-thinking/lessons/lesson-21.md)、[面试问答](../interview/lessons/lesson-21.md)和 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)；第 22 节 React 消费与工作台设计见对应的[课程正文](../course/part-03/lesson-22-react-lottery-page.md)、[API](../api/lessons/lesson-22.md)、[QA](../qa/lessons/lesson-22.md)、[第一性原理手记](../design-thinking/lessons/lesson-22.md)和[面试问答](../interview/lessons/lesson-22.md)。隔离 acceptance 的 64 个多 Award 请求最多并行 16 个，只证明当前链路并发返回配置内结果，且调用前后两张 Lottery 业务表 fingerprint 不变；访问日志、连接统计等技术副作用仍可能发生，因此这不是“系统零副作用”、64 并发、64 RPS 或业务性能结果。浏览器 UI 验收同样不能替代业务负载证据。

## 成功信号

最终成功不是“技术栈全部用到”，而是以下能力可以被证据验证：

- 用户从 Feed 触达、参与、获得权益到转化的链路可运行、可追踪。
- 运营能复用活动和策略能力，并通过实验与漏斗结果继续优化。
- 核心账户与权益操作具备幂等、流水、审计和故障恢复能力。
- 平台能用压测数据解释扩展决策，而非预先堆叠分布式组件。
- AI Agent 通过受控工具完成可授权任务，高风险动作具备审批和审计。

这些信号已经在[非功能需求基线 v1](non-functional-requirements-v1.md)中转为候选 SLO、业务不变量和阶段验证计划。当前已有第一组 Go 业务领域对象、两张持久化表、Strategy Repository、经过边界验证的选择器、ephemeral HTTP 纵向链与真实 React 消费者；但页面展示的仍是不可恢复的临时候选选择，不是正式 Draw/Result 或奖励到账事实，抽奖、参与、权益等业务 SLO 仍全部“未测量”。算法微基准、64 请求/最大并行 16 的 acceptance、浏览器 UI 验收和 M0 工程探针分别回答不同局部问题，均不能外推为正式 Draw 能力、端到端业务延迟或生产吞吐。

完整消费者主线、异常恢复和术语定义见[用户增长旅程 v1](user-growth-journey-v1.md)。

完整运营角色、配置、审批、发布、止损和复盘流程见[运营人员工作流 v1](operator-workflow-v1.md)。

AI 查询、计划、Tool 调用、审批、失败和审计边界见[AI Operator 工作流 v1](ai-operator-workflow-v1.md)。

命令、领域事件、查询、策略、失败和补偿的统一分析见[领域事件地图 v1](domain-event-map-v1.md)。

营销活动、抽奖、参与、权益、Feed、行为分析、治理和 AI 运营的职责与事实所有权见[限界上下文地图 v1](bounded-context-map-v1.md)。

容量、延迟、可用性、一致性、恢复、安全与降级目标见[非功能需求基线 v1](non-functional-requirements-v1.md)。
