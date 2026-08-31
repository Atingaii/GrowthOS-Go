# 产品简述：GrowthOS-Go

**状态：** v17 基线
**更新日期：** 2026-08-31
**来源章节：** [第 1 节](../course/part-01/lesson-01-why-ai-native-growth-platform.md)、[第 2 节](../course/part-01/lesson-02-user-growth-journey.md)、[第 3 节](../course/part-01/lesson-03-operator-workflow.md)、[第 4 节](../course/part-01/lesson-04-ai-operator-workflow.md)、[第 5 节](../course/part-01/lesson-05-first-event-storm.md)、[第 6 节](../course/part-01/lesson-06-first-bounded-contexts.md)、[第 7 节](../course/part-01/lesson-07-non-functional-requirements.md)、[第 17 节](../course/part-03/lesson-17-lottery-domain-objects.md)、[第 18 节](../course/part-03/lesson-18-lottery-schema.md)、[第 19 节](../course/part-03/lesson-19-lottery-repository.md)、[第 20 节](../course/part-03/lesson-20-lottery-weighted-selection.md)、[第 21 节](../course/part-03/lesson-21-lottery-api.md)、[第 22 节](../course/part-03/lesson-22-react-lottery-page.md)、[第 23 节](../course/part-03/lesson-23-lottery-strategy-rule-requirements.md)、[第 24 节](../course/part-03/lesson-24-redis-strategy-cache.md)、[第 25 节](../course/part-04/lesson-25-user-eligibility.md)、[第 26 节](../course/part-04/lesson-26-responsibility-chain.md)、[第 27 节](../course/part-04/lesson-27-responsibility-chain-limits.md)、[第 28 节](../course/part-04/lesson-28-rule-tree-schema.md)、[第 29 节](../course/part-04/lesson-29-rule-decision-engine.md)、[第 30 节](../course/part-04/lesson-30-strategy-vs-activity.md)、[第 31 节](../course/part-04/lesson-31-access-control-model-threat-boundary.md)

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

第 17 节把 Lottery 最小业务语言落成纯 Go 领域对象：`Strategy` 管理至少一个 `Award`，候选使用正整数相对权重，并显式区分 `reward` 与 `no_reward`。第 18 节用 `000001` / `000002` 分别创建 `lottery_strategy` 与 `lottery_strategy_award`，保存 Strategy 根行和隶属于它的 Award 候选；该节验收时 Migration latest 为 2。第 19 节再以两个窄 application 端口和 MySQL adapter 实现 Strategy Create/FindByID：完整父子聚合原子写入，一次读取来自同一个只读 RR 快照，恢复时重新验证领域不变量。第 20 节在领域层增加 `WeightedSelector` 与 consumer-owned `BoundedRandomSource`，多候选从均匀 `[0,totalWeight)` 整数位置映射到规范排序的 Award，生产 adapter 使用 `crypto/rand.Int` 并覆盖完整 `uint64` 范围；单候选不消耗随机源，`no_reward` 仍是正常选择结果。第 21 节以 `EphemeralSelectionService` 组合只读 Repository 与 Selector，并通过 `POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections` 暴露第一次真实纵向调用；完整 uint64 identity 以十进制 string 传输，路由默认关闭且只允许 development/test 显式打开。第 22 节增加同源 bodyless POST transport、严格 Lottery runtime decoder 与 React 请求状态 Hook，使 `/lottery` 真实消费该临时接口，并在共享工作台壳层中区分 pending、`reward`、`no_reward`、服务端拒绝、网络故障、超时、取消与契约漂移。

第 23 节用“活动有效、新用户且有次数、风险允许、按会员等级路由、奖励可分配、仍可能 `no_reward`”这一复合需求检查现有模型，确认业务口中的“抽奖规则”跨越多个决定所有者。Activity 发布态与时间窗决定归 Marketing，用户资格、次数和参与风控决定归 Participation，Strategy 路由、终端选择与正式 Draw/Result 归 Lottery，库存可分配、权益交付与补偿归 Benefit（含内部库存子能力），操作者授权决定归 Governance 的统一访问控制能力；外部会员、风险等系统只是原始事实提供方。业务资格拒绝、合法 `no_reward`、技术失败/结果未知与授权拒绝必须保持不同语义。

第 23 节没有新增 `Rule`、`RuleEngine`、通用上下文、Migration、API、Redis 或前端判断。当时只有一个真实终端选择消费者，提前把规则字段塞进 `Strategy` 会让两表 Repository 只能恢复残缺聚合；第 25 节因此只先建立首个具体资格事实契约和规则，第 26 节在第二条真实风险准入规则出现后，才由两个实际消费者共同需要的顺序、继续、拒绝、错误、取消和 trace 语义反推最小线性协议。完整需求见 [Lottery 业务规则需求基线 v1](lottery-rule-requirements-v1.md)，长期停止线见 [ADR-0019](../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)。

第 24 节只加速 Strategy 读取投影：`strategycache.Reader` 装饰 application-owned `StrategyReader`，MySQL 始终是事实源；Redis 保存版本化 JSON v1，不保存用户资格、一次选择、Draw/Result 或 not-found。命中时恢复并重新校验聚合；miss、超时、协议错误和 poison value 在有界预算内回源。成功回源后才 best-effort 写 Redis，SET 失败直接返回已经取得的 source 结果；坏值只删除精确 key，TTL 不超过 5 分钟并带最多 10% jitter。同 key cold fill 合并避免击穿；不同 key 的 fill 不共享执行锁，只在 flight map 登记/移除时短暂共享记账 mutex。Compose 只允许 API/Redis 进入 internal cache 网络；业务 ACL 允许无 key 的 `PING`，并只允许对版本化前缀执行 `GETRANGE/SET/DEL`，默认用户和扫描/管理/channel 命令全部关闭。长期边界见 [ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)。

第 25 节首次把 Participation 的一条具体资格规则落成可执行代码。外部用户目录仍拥有账户注册原始事实；Participation 定义消费方拥有的 `RegistrationFactReader` 端口，以接收带主体引用、注册时刻、观察时刻、来源和修订的受控快照，但本节没有生产 adapter。application 用一次注入的服务端时刻校验主体匹配、未来时间和最大陈旧时间，domain 再按版本化 policy 的含边界 cutoff 形成稳定 `eligible` / `ineligible` 决定。not-found、stale、unavailable、损坏和取消均表示没有形成可信业务决定，不能被伪装成“老用户”；决定不显式建模 ParticipantRef、registered-at 等直接 PII 或用户文案，并保留可能高基数的 revision/source/evaluated-at 供受控追溯。source/revision 不含邮箱、手机号等 PII 仍是未来 adapter contract，必须由 contract test 强制；这些字段也不能直接用作普通指标 label。长期边界见 [ADR-0021](../decisions/ADR-0021-participation-new-user-eligibility.md)和[新用户资格规则基线 v1](new-user-eligibility-v1.md)。

第 26 节增加第二条具体 Participation 规则，但仍不复制风控模型。受控风险事实提供方拥有最小 `passed/blocked` screening disposition、source-owned `assessed_at`、source 和 revision；Participation 只拥有固定规则 `participation.risk.screening_admission` 对当前参与场景的准入决定。`passed` 使该节点继续，`blocked` 形成确定业务拒绝；not-found、stale、future、主体不符、损坏、provider unavailable 和未分类读取失败都返回零决定加安全 typed error，不能默认放行或冒充业务拒绝。

两个具体节点共同证明了一个 Participation 专用的固定前置资格链：组合器在任何 reader 前从受控服务端 `Clock` 捕获一次 canonical logical as-of，先懒读取并判断新用户，只有 confirmed eligible 才访问更敏感的风险 reader。新用户拒绝、技术失败或 caller cancellation 都使 risk reader 保持零调用；只有两节点都 confirmed eligible 才形成 `all_prerequisites_satisfied`。确定结果携带独立 `RuleSetRevision`、同一 evaluated-at 和只包含实际执行节点的有序最小 trace；trace 不含 ParticipantRef、原始风险特征、阈值、provider payload 或 raw error，返回的 steps slice 也不能改写内部证据。fact revision 只用于受控追溯，仍禁止进入普通 metrics label。

第 26 节同时收紧事实读取错误边界：普通 `Error()` / `errors.Is` 只暴露一个审核过的 application class，底层 provider cause 通过显式 `Cause()` 留给受信诊断，避免一个错误同时命中互斥语义分类。`Cause()` 不是可直接打印原始错误的许可；未来 observer 仍需字段白名单、脱敏、访问控制和保留期。固定计划使用包内最小 step，没有导出通用 `Rule` / `RuleEngine`、priority、泛型事实袋、规则树或 DSL。

本节有意不新增用户/风险表、Migration、Redis 资格缓存、外部 provider adapter、HTTP/React 合约、Activity、Lottery 调用或 composition-root 装配。`ParticipantRef` 是受控查询引用，不是已认证 Principal；共享 as-of 也不是两个外部 authority 的原子事务快照。当前代码只证明 Participation domain/application 内核的固定顺序、真实短路、失败分类和最小执行证据，不能被描述为在线抽奖已经完成资格门控，也没有对应 Compose 或浏览器 E2E。长期边界见 [ADR-0022](../decisions/ADR-0022-participation-prerequisite-chain.md)和[Participation 前置资格链基线 v1](participation-prerequisite-chain-v1.md)。

第 27 节回到 Lottery，用已确认的会员等级做第一个具体 Strategy 路由。consumer-owned `MembershipTierFactReader` 提供封闭 `standard/premium` 快照，application 在一次受控 as-of 下校验主体、未来时间、freshness 与取消；confirmed premium 进入 `premium_override`，confirmed standard 进入显式 `baseline_default`，unknown、损坏、缺失、过期或 provider failure 都不得伪装成默认命中。决定携带 defensive-copy 一跳 path，并在返回成功前检查 branch/reason/path 一致性。它证明固定 gate chain 无法表达分支选择，但 policy revision 在该节仍只是 bounded token，同名 token 还不能唯一恢复完整路由内容。

第 28 节只解决这个已经出现的持久化缺口。Lottery-owned `StrategyRoutingGraph` 用 `(GraphID, Revision)` 标识 create-only schema v1，显式 root、`decision` / `strategy_target` node 与 `premium_override` / `baseline_default` edge 形成允许共享后继的 rooted DAG。构造和恢复必须证明唯一显式 decision root、全可达、无环、terminal 无出边、每个 decision 精确两条 approved branch、default 映射一致，并在分配/遍历前限制 128 nodes、256 edges、16 edges depth。未知 schema、node kind、rule 或 branch 一律失败关闭；graph 不拥有 Participation、Governance、Benefit 或 Activity 的事实。

`000003` / `000004` / `000005` 依次创建 graph header、node、edge，使当前源码 Migration latest 提升为 5。复合主外键把 node/edge 限制在同一 graph revision，CHECK/UNIQUE 保护局部枚举、branch/default 与 target shape；header 的 root 不建立反向 FK，因为 InnoDB 立即检查会与 node -> graph FK 形成没有合法首次 INSERT 的循环。root/全可达/环/深度等跨行事实仍由完整恢复后的领域验证负责，数据库不会被描述成完整 graph validator。

application 在第 28 节只新增 `StrategyRoutingGraphCreator.Create` 与 `StrategyRoutingGraphReader.FindByIdentity`，独立 MySQL adapter 在一个事务中按 header -> canonical nodes -> canonical edges 创建完整 revision，在 read-only `REPEATABLE READ` snapshot 中以 129/257 sentinel 有界加载并严格恢复。Adapter 尚未装配进 `growth-api`，也没有 Update/Delete、active/published revision、Activity 绑定、HTTP/React 或权限 UI。一次性 MySQL 8.4.11 已验证 latest 5、graph round-trip/并发/回滚/RR/EXPLAIN、完整 uint64 与坏存量失败关闭：legacy writer 只获旧两表 `SELECT, INSERT`，graph repository 身份只获新三表 `SELECT, INSERT`；测试资源零残留且长期 Docker 快照不变。默认长期 Compose 也已原地从 `2:0` 前向到 `5:0`，旧表指纹不变、新三表为空，runtime graph SELECT 真实 1142，长期 smoke 与隔离 v5 Lottery/cache acceptance 均通过并完成所有权校验清理。

第 29 节在不扩大存储和 runtime 边界的前提下补上内部执行语义。`StrategyRoutingGraphEvaluationService` 必须按调用方给定的 exact `(GraphID, Revision)` 只读 graph 一次，复核 identity/`Validate()` 并在事实读取前要求 graph 最坏 depth 不超过服务端 `maxSteps`。准入后只从受控 Clock 取一次 canonical UTC `evaluated_at`、只读一次权威会员 fact，并让整条 path 复用同一 snapshot；graph/Clock/fact 的正常调用上限因而为 `1/1/1`。第 27 节 concrete router 与新 evaluator 共享 package-private tier-to-branch oracle，v1 仍只认 `lottery.membership_tier.route_strategy`，不是通用 `RuleEngine`、registry 或 DSL。

Domain evaluator 从显式 root 开始迭代走唯一实际 path，按 selected branch 精确查找一条 edge，不使用 `edges[0]`、数据库顺序或“未命中就 default”。成功决定只携带 exact graph/schema/root/terminal/Strategy target、fact provenance、单一 evaluated-at 与 defensive-copy ordered path；不携带 subject、raw tier、session、Strategy/Award 内容或 Draw。`maxSteps` 只允许 `1..16`，loop 仍保留 actual hard stop；positive `maxDuration` 的 child timer 横跨 graph read、Clock、fact read 和 traversal，caller 更早或相同 deadline 始终拥有优先权。它是 cooperative budget：graph/fact reader 必须观察 context，既有 `Clock.Now()` 则必须保持为有界本地调用，阻塞时只能在返回后观察超时。在依赖返回与执行边界上，caller cancellation > internal timeout > live provider error 的分类顺序被显式固定；任一失败只返回 zero decision/path，内部 timeout 也不通过 `errors.Is` 伪装成 caller `context.DeadlineExceeded`。

这份 evaluator 仍只是未发布、未装配的 domain/application 内核：没有 latest/active fallback、Activity 绑定、长期 graph grant、生产会员 adapter、HTTP/UI/auth、Strategy load/selection 或持久 Draw。最终候选的实际证据为：Lottery domain/application atomic coverage 93.6%/88.3%、合并 92.1%；全仓普通与 race 测试通过；独立 10 秒 evaluator fuzz 执行 2,899,250 次、新发现 1 个 interesting input（总数 43）；前端 19/19 files、152/152 tests、typecheck/build 通过。第 28 节 MySQL 8.4.11 六组 Integration 也作为 schema/Repository 上游回归重跑通过，不能据此声称 evaluator 已连接真实 MySQL/fact provider 或达成业务 SLO。

数据库只承担它能可靠表达的子集。旧两表保护正 ID/权重、Strategy 内 AwardID 唯一、封闭 outcome、引用完整性与基础名称形态；它们仍不能保证至少一个 Award或跨行总权重不溢出。新三表保护 graph revision scope、node/edge 引用、局部 discriminated shape、branch 唯一与 default 字面值，却不能证明 root 类型、全可达、无环、深度或 decision 完整出边。两表的 `updated_at` 仍只是行级元数据；graph revision 则是 create-only 内容身份，不是 Activity version、schema version、应用版本或会员 fact revision。

当前 Compose MySQL 运行身份仅可对旧两张业务表 `SELECT`，不能访问新三张 graph 表，也不能 INSERT、UPDATE、DELETE 或访问 `schema_migrations`；需要创建 fixture 的两个 Repository 集成测试分别使用可丢弃 schema 中、互不扩权的 legacy writer 与 graph writer。GrowthOS 已有可验证的 Lottery 领域、五张持久化表、两个内部 Repository、Strategy Redis 读取缓存、加权选择器、一个真实 ephemeral HTTP/React 消费链、尚未运行时装配的 Participation 两节点资格链、graph Repository 与 closed routing evaluator；但还没有 graph 发布/Activity 绑定、runtime/API 求值链、正式 Draw API、真实事实 adapter、认证、RBAC 或持久化结果。除系统状态页和 `/lottery` 外，其他工作台仍是明确 Mock 快照或浏览器本地交互。

尤其，ephemeral API 返回的 Award 仍是一次瞬时计算结果，不是带 DrawID、幂等键和持久化状态的一次用户抽奖最终事实；`reward` 也不表示积分或优惠券已经进入 Benefit 发放生命周期。持久化 graph 只是合法配置，不表示已发布、已绑定 Activity 或已被在线 runtime 选中；第 29 节内部 evaluation 成功也只表示对显式指定 revision 形成一份可信 Strategy route，不表示调用者有权、用户有资格或抽奖已完成。当前没有认证、对象级授权、Activity/次数等完整 Participation 前置条件，也没有把资格链、graph Repository 或 evaluator 装配进 Lottery 运行链；Draw/Result、库存或发奖同样尚未实现，因此“一次抽奖只能有一个最终结果”仍是待验证的不变量。

第 21～23 节服务端、React 和规则边界分别见各自课程、QA、设计手记与 ADR。第 24 节缓存事实与取舍见[课程正文](../course/part-03/lesson-24-redis-strategy-cache.md)、[API](../api/lessons/lesson-24.md)、[QA](../qa/lessons/lesson-24.md)、[第一性原理手记](../design-thinking/lessons/lesson-24.md)、[面试问答](../interview/lessons/lesson-24.md)和 [ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)。隔离 acceptance 验证了缓存/直连/Redis-down 三组 50 RPS×10s 均 500/500 成功，warm-cache MySQL prepared execute 为 0，另两组为 1000；这只证明当次本地链路的 source-load 和短窗口延迟，不是生产容量、正式 Draw SLO 或通用缓存收益。调用前后两张 Lottery 业务表 fingerprint 不变也不等于“系统零副作用”。

第 25 节资格事实与错误边界见[课程正文](../course/part-04/lesson-25-user-eligibility.md)、[API 零变化记录](../api/lessons/lesson-25.md)、[QA](../qa/lessons/lesson-25.md)、[第一性原理手记](../design-thinking/lessons/lesson-25.md)、[面试问答](../interview/lessons/lesson-25.md)和 [ADR-0021](../decisions/ADR-0021-participation-new-user-eligibility.md)。这些证据来自 domain/application 单元、fuzz、并发、取消和架构停止线测试；本节没有 Compose、真实外部目录或浏览器验收，不能外推为在线资格链已经交付。

第 26 节第二条风险规则与固定前置资格链见[规则链基线](participation-prerequisite-chain-v1.md)、[课程正文](../course/part-04/lesson-26-responsibility-chain.md)、[API 零变化记录](../api/lessons/lesson-26.md)、[QA](../qa/lessons/lesson-26.md)、[第一性原理手记](../design-thinking/lessons/lesson-26.md)、[面试问答](../interview/lessons/lesson-26.md)和 [ADR-0022](../decisions/ADR-0022-participation-prerequisite-chain.md)。这些证据验证 Participation 内核中的 source-owned risk snapshot、一次 shared as-of、固定 `new-user -> risk` 顺序、后序零调用短路、零 aggregate 技术失败、两个事实读取 wrapper 各自的单一公开错误 class 与最小 trace；没有真实 fact adapter、公开 API、Lottery/composition 装配、Compose 或浏览器 E2E，不能据此声称线上请求已经受资格保护。

第 27 节 concrete 会员路由见[产品基线](membership-strategy-routing-v1.md)、[课程正文](../course/part-04/lesson-27-responsibility-chain-limits.md)、[API 零变化记录](../api/lessons/lesson-27.md)、[QA](../qa/lessons/lesson-27.md)、[第一性原理手记](../design-thinking/lessons/lesson-27.md)、[面试问答](../interview/lessons/lesson-27.md)和 [ADR-0023](../decisions/ADR-0023-membership-strategy-routing-boundary.md)。第 28 节持久化见[Strategy Routing Graph 基线](lottery-strategy-routing-graph-v1.md)、[课程正文](../course/part-04/lesson-28-rule-tree-schema.md)、[API 零变化记录](../api/lessons/lesson-28.md)、[QA](../qa/lessons/lesson-28.md)、[第一性原理手记](../design-thinking/lessons/lesson-28.md)、[面试问答](../interview/lessons/lesson-28.md)和 [ADR-0024](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)。前者是执行语义 oracle，后者是 topology/schema/repository 输入边界。第 29 节 closed evaluator 见[求值产品基线](lottery-strategy-routing-evaluation-v1.md)、[课程正文](../course/part-04/lesson-29-rule-decision-engine.md)、[API 零变化记录](../api/lessons/lesson-29.md)、[QA](../qa/lessons/lesson-29.md)、[第一性原理手记](../design-thinking/lessons/lesson-29.md)、[面试问答](../interview/lessons/lesson-29.md)、[运维手册](../runbooks/strategy-routing-graph-evaluation.md)和 [ADR-0025](../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)；这三节连起了内部语义/持久化/求值，但仍不证明已发布、已装配或具备 Activity/权限/浏览器 E2E。

第 30 节把可执行 Strategy 与可运营 Activity 分开：Lottery create-only snapshot 固化 exact Strategy/Award 内容，Marketing 以 immutable publication、`state_version` CAS、追加式 rollback、retire、一次 Clock 的时间窗 resolve 和 Lottery ACL 管理 Activity；commit acknowledgement 丢失通过受信 receipt 与 exact read-back 形成三态对账。第 31 节再为这些高风险管理动作建立 Governance-owned 的访问语言：16 个 exact kind/type/action capability、5 个固定角色模板上限、`system/tenant/owned/resource` 四种 scope、allow/deny RoleBinding、不可变 Policy identity/revision、default deny、deny precedence 和有界 Decision evidence 均已成为纯 Go 可执行模型。

第 31 节仍没有 credential/session、Policy repository、assignment、HTTP enforcement、401/403/404 映射、审计落库、React capability projection 或越权 E2E。Principal/Resource 构造只验证 shape，不能证明 caller 或 tenant/owner facts 可信；现有 endpoint 不导入 Governance domain。完整边界见[访问控制模型基线](access-control-model-threat-boundary-v1.md)、[ADR-0027](../decisions/ADR-0027-governance-access-control-model.md)、[课程](../course/part-04/lesson-31-access-control-model-threat-boundary.md)、[QA](../qa/lessons/lesson-31.md)和[模型审查手册](../runbooks/access-control-model-review.md)。

## 成功信号

最终成功不是“技术栈全部用到”，而是以下能力可以被证据验证：

- 用户从 Feed 触达、参与、获得权益到转化的链路可运行、可追踪。
- 运营能复用活动和策略能力，并通过实验与漏斗结果继续优化。
- 核心账户与权益操作具备幂等、流水、审计和故障恢复能力。
- 平台能用压测数据解释扩展决策，而非预先堆叠分布式组件。
- AI Agent 通过受控工具完成可授权任务，高风险动作具备审批和审计。

这些信号已经在[非功能需求基线 v1](non-functional-requirements-v1.md)中转为候选 SLO、业务不变量和阶段验证计划。当前已有 Go Lottery/Participation/Marketing 领域切片、十张业务表、可丢弃读取缓存、ephemeral HTTP/React 纵向链、未装配的资格链、routing graph evaluator、Activity publication 和 Governance Policy evaluator；但页面仍只展示不可恢复的临时候选选择，授权内核也没有 runtime consumer。局部算法、MySQL、race/fuzz、领域内核和 UI 证据均不能外推为正式 Draw、在线资格/授权、端到端业务 SLO 或生产吞吐。

完整消费者主线、异常恢复和术语定义见[用户增长旅程 v1](user-growth-journey-v1.md)。

完整运营角色、配置、审批、发布、止损和复盘流程见[运营人员工作流 v1](operator-workflow-v1.md)。

AI 查询、计划、Tool 调用、审批、失败和审计边界见[AI Operator 工作流 v1](ai-operator-workflow-v1.md)。

命令、领域事件、查询、策略、失败和补偿的统一分析见[领域事件地图 v1](domain-event-map-v1.md)。

营销活动、抽奖、参与、权益、Feed、行为分析、治理和 AI 运营的职责与事实所有权见[限界上下文地图 v1](bounded-context-map-v1.md)。

容量、延迟、可用性、一致性、恢复、安全与降级目标见[非功能需求基线 v1](non-functional-requirements-v1.md)。
