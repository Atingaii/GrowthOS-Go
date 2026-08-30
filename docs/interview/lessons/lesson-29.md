# 第 29 节面试题：封闭类型化路由图求值、确定性证据与取消预算

本文面向“规则/决策引擎、DAG 执行、事实快照、Go `context`、超时归因、错误边界、不可变值、并发、race 与 fuzz”类项目深挖。第 29 节只把第 28 节已经严格构造或恢复的 `StrategyRoutingGraph` 执行成一条可信 Lottery 路由：一次读取 exact graph、一份权威会员事实、一个逻辑时刻，沿唯一实际 path 到达一个 Strategy target。

能力边界必须先说清：本节已经有 Lottery domain evaluator、未装配的 application service、共享会员事实/分支语义、step 与 duration budget、caller/internal/provider 错误优先级、完整或零决定协议，以及 unit、并发、race、fuzz 和架构停止线测试。它仍没有 Activity 发布/active revision、runtime wiring、HTTP/API、真实会员 adapter、Strategy 加载、加权随机、正式 Draw/Result、session、Principal、RBAC、UI 或浏览器 E2E。求值成功不等于图已发布、用户有资格、调用者获授权或奖品已抽中。

## 60 秒项目自述

> 第 28 节只解决不可变 rooted DAG 的保存和严格恢复，第 29 节才冻结执行语义。我没有把课程标题里的“引擎”扩成通用规则平台，而是实现 Lottery-owned、schema-v1 的封闭类型化 evaluator：调用方显式给出 `(GraphID, Revision)`，application 只读一次 graph，复核 identity/aggregate 和最坏路径预算，再捕获一次业务时刻、读取一次权威会员事实，所有 decision 复用同一 snapshot。
>
> Domain 从显式 root 迭代执行实际单路径。会员事实先通过第 27 节共用的 typed branch oracle 得到 `premium_override` 或 `baseline_default`，再精确匹配同名 edge；不依赖 edge 顺序，也绝不把 unknown、缺边或依赖错误落入 default。每走一条 decision edge 计一个 step，路径按顺序记录 node、rule、branch、reason 和 destination，到 terminal 只返回 Strategy identity，不加载 Strategy、不随机选择。
>
> 运行控制分为静态图预算、服务最坏深度准入、实际 step guard 和横跨整个 use case 的 cooperative deadline。graph/fact reader 必须观察 context；既有 `Clock.Now()` 没有 context，因此必须是有界本地调用，阻塞时只能在返回后发现超时。caller deadline 早于或等于内部 deadline 时归 caller；只有内部更早才用 `WithDeadlineCause` 安装私有 cause。在依赖返回/执行边界，错误优先级固定为 caller、内部 budget、provider、非法 value；入参与配置仍先按 fail-fast 顺序校验。所有失败只返回 zero decision/path。结果和 path 防御复制，交错异构的 64 并发调用、取消边界、低披露错误、race、fuzz 与“不得装配到 runtime/HTTP/UI”的架构测试共同锁定当前边界。

## 来源、访问日期与可信度

- **项目事实：** 以 [产品基线](../../product/lottery-strategy-routing-evaluation-v1.md)、[ADR-0025](../../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)、[domain evaluator](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[application service](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[application error](../../../internal/lottery/application/strategy_routing_graph_evaluation_error.go)及相邻测试为准。产品/ADR 决定批准语义；代码说明当前实现；测试只证明被执行到的行为，不自动证明生产可用。
- **Go 官方主来源：** [`context` package](https://pkg.go.dev/context)、Go 官方博客 [Go Concurrency Patterns: Context](https://go.dev/blog/context)、[`errors` package](https://pkg.go.dev/errors)、[`time` 的 monotonic clock 说明](https://pkg.go.dev/time#hdr-Monotonic_Clocks)、[Go Fuzzing](https://go.dev/doc/security/fuzz/)与 [Data Race Detector](https://go.dev/doc/articles/race_detector)。本文的取消、cause、错误匹配、时间、fuzz 和 race 技术结论优先以这些一手资料为准。
- 上述官方链接与下列社区页面均在 **2026-08-30** 检索或复核。本文主要转述，不复制长段原文；社区页的可访问性差异见下一节说明。

## 真实面经题型参考与未核验声明

下列内容是社区用户自述或整理，只用于观察“面试官可能怎样继续追问”，不是公司官方题库，也不是技术结论来源。本文没有独立核验作者身份、公司/岗位、面试轮次、录音、题目原句或录用结果；帖子答案即使存在也不直接采信。

- 牛客[快手日常实习个人复盘](https://www.nowcoder.com/discuss/723657746536464384)自述被问规则引擎流程、执行期额外取资源以及规则冲突；用于准备“事实何时读取、冲突如何处理、为何当前没有通用命中策略”。
- 牛客[26 秋招个人小结](https://www.nowcoder.com/discuss/838483750089453568)自述项目后被追问是否了解规则引擎；用于准备“为何这个 evaluator 还不能冒充通用引擎”。
- 牛客[早期 Golang 个人面经](https://www.nowcoder.com/discuss/353154773744558080)自述被问 `context` 用途、map 顺序、主协程等待与并发；用于准备取消传播、顺序确定性和并发边界。
- 牛客[社招五年 Go 个人复盘](https://www.nowcoder.com/discuss/882048490270904320)自述被问项目角色、难点和技术选型；用于准备从需求、不变量、备选方案和触发重评条件回答，而不是罗列名词。
- 牛客[深信服 Golang 全流程个人复盘](https://www.nowcoder.com/discuss/414023383497678848)自述项目设计被深挖，并出现“怎样保证线程安全/方案优缺点”；用于准备 stateless evaluator、依赖并发契约与 race 证据。
- 掘金[进阶版 Golang 面试题整理](https://juejin.cn/post/7178051992063836216)包含“`Job` 超时提前返回”和 `context` 等题型；掘金[腾讯社招 Golang 经验分享](https://juejin.cn/post/6844904131132391438)也列出 `context` 用途与项目架构追问。二者用于题型补充，不用于证明标准库细节。
- 掘金[《上来就三道算法、23 年腾娱互动、Golang 后端面试复盘（已挂）……》](https://juejin.cn/post/7198544446283235384)强调项目难点、选型和“为什么”；掘金[百度 Golang 实习面经整理](https://juejin.cn/post/6844904200351154189)出现任务依赖排序与判环题型。它们分别用于准备方案权衡和 DAG 追问；页面可能是转载/整理，原始归属未核验。
- **访问受阻说明：** 快手与 26 秋招两篇牛客正文可直接读取；部分旧牛客页面深度抓取只返回页面壳，深信服页面的一次直接抓取超时；部分掘金正文抓取也只返回页面壳，但搜索索引可见本文引用的题型片段。因此本文只采用检索结果中可复核的题型，不补写不可见原文，不声称逐字核对了全部帖子。
- 本轮还检索了知乎相关结果，但没有找到稳定可访问、与本节直接相关且能识别为个人真实复盘的页面，因此不为凑来源而引用，也不把聚合答案伪称真实面经。

---

## 1. 第 29 节到底解决了什么？

- **直接回答：** 它解决“给定一份明确、合法、不可变 graph revision 和一份权威会员事实，怎样在有限资源内得到唯一 Strategy target 与完整 path 证据”。第 28 节证明 graph 可被信任，本节证明它怎样执行；二者不能互相替代。
- **追问：** “为什么不直接接入抽奖接口？”
  - **追问回答：** 因为线上入口还需要 Activity 发布绑定、真实会话、服务端授权、Participation、Strategy snapshot、随机选择、幂等、库存与正式结果。提前接入会把“配置执行正确”与“谁可以在何时抽什么”混成一个不可审查用例。
- **权衡：** 分层让故障定位、测试和演进边界清晰，代价是当前只有内部能力、没有用户闭环。
- **代码 / 测试证据：** [产品停止线](../../product/lottery-strategy-routing-evaluation-v1.md)、[application service 注释与依赖](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[runtime/HTTP/UI 禁装配测试](../../../internal/lottery/application/membership_routing_architecture_test.go)。

## 2. 为什么标题叫“决策引擎”，实现却刻意不叫通用 `RuleEngine`？

- **直接回答：** 当前只有一个 Lottery-owned operator、一个 typed fact、两条 closed branch。证据只支持“受限图求值器”，不支持跨上下文 fact bag、动态 operator registry、冲突消解、DSL、脚本、安全沙箱或运营编辑器。名称不能替代能力证明。
- **追问：** “写一个接口和 registry 不是更符合开闭原则吗？”
  - **追问回答：** 只有一个实现时，registry 会先制造重复注册、缺插件、输入类型、依赖声明、版本兼容和生命周期问题，却没有第二个真实消费者验证抽象。未来出现第二种 Lottery rule，再由两个具体用例反推 closed union、visitor 或 registry。
- **权衡：** closed dispatch 扩展要改代码并发布，但未知 code 必然失败关闭；动态 registry 表面灵活，却扩大运行兼容与安全面。
- **代码 / 测试证据：** [ADR 方案四/五/九](../../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)、[封闭 dispatch](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[通用引擎/泛型/map fact bag 禁止测试](../../../internal/lottery/application/membership_routing_architecture_test.go)。
- **面经关联：** 牛客[快手复盘](https://www.nowcoder.com/discuss/723657746536464384)提供“规则引擎流程与冲突”的追问形态，但不能反向证明本项目必须实现完整引擎。

## 3. 既然 tier 到 target 可以写成 `if/else`，为什么还要执行 graph？

- **直接回答：** `if/else` 可以实现一跳业务语义，却绕过 graph 的 root、node/edge identity、revision、合流、多步 path 与预算；那会让第 28 节保存的拓扑成为无效摆设。本节复用 `if/else` 只做 typed `fact -> branch` oracle，然后必须沿 graph 的 exact edge 走到 terminal。
- **追问：** “是不是为了展示 DAG 而过度设计？”
  - **追问回答：** 若业务永远只有一跳，确实应重新评估 graph 的收益；当前章节的真实目标是把已经批准并持久化的有界 topology 变成可解释执行。文档明确承认 v1 多步仍重复同一个会员 operator，并未宣称多条件能力。
- **权衡：** graph 增加模型和验证成本，换来 immutable revision、可审计 path 与未来受控拓扑演进；直接分支更简单，但不能诚实消费已存在的 graph。
- **代码 / 测试证据：** [typed branch helper](../../../internal/lottery/domain/membership_routing.go)、[graph evaluator](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[与第 27 节 concrete router 等价测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。

## 4. 为什么必须传 exact `(GraphID, Revision)`，不提供 latest/active？

- **直接回答：** exact identity 让一次 evaluation 的 topology 可定位、可重新加载、不会因并发发布而漂移。它只解决图版本定位；没有历史 fact reader 和持久 decision 时仍不能完整重放。`latest` 是存储排序概念，`active` 是 Activity 发布决策；两者都不是 evaluator 应猜的业务输入。
- **追问：** “revision 不存在时 fallback 前一版能提高可用性吗？”
  - **追问回答：** 会静默改变调用者指定的语义，甚至执行已退役配置。当前 not-found 直接零决定；第 30 节由发布模型明确批准 active binding 与回滚，而不是 Repository 随机兜底。
- **权衡：** exact identity 让调用方承担正确选择 revision 的责任，换来确定性与清楚归责；latest 入口方便，却把排序、发布和回滚揉进执行层。
- **代码 / 测试证据：** [application exact lookup](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[wrong identity/not-found 测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。

## 5. 为什么依赖顺序是 graph → Clock → fact，并且各至多一次？

- **直接回答：** 先读 graph 可以在 missing/corrupt/超预算配置上避免读取会员派生事实；图合法后捕获唯一业务时刻，再读取一次 fact，用同一时刻做 future/freshness 判断。多步 path 全部复用同一 graph、时刻和 fact，不产生节点间事实漂移或 provider N+1。
- **追问：** “先并行读 graph 和 fact 不是延迟更低吗？”
  - **追问回答：** 当前没有生产延迟证据，提前并行会在 graph 根本不可执行时仍访问用户事实，增加隐私、成本、取消和错误竞争面。若未来 profile 证明需要并行，必须先重新定义 snapshot、一致性和错误优先级。
- **权衡：** 串行多一个依赖时延，换来最小数据访问和确定错误顺序；并行潜在更快，但更难归因且会做无效敏感读取。
- **代码 / 测试证据：** [application orchestration](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[依赖顺序/调用次数测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。

## 6. “同一份 snapshot”是否意味着分布式强一致或完全可重放？

- **直接回答：** 不意味着。它只保证单次进程内求值使用一个 exact immutable graph、一份返回的 fact snapshot 和一个 canonical evaluated-at。它没有 graph 与会员 authority 的跨系统事务，也没有把 decision 持久化为审计记录；provider 是否能按 fact revision 历史回读也未证明。
- **追问：** “那为什么还记录 fact source/revision？”
  - **追问回答：** provenance 能解释本次决定基于哪份受控事实，并为未来审计/重放协议提供必要输入；它是必要但不充分条件，不能夸大为已经实现事件溯源。
- **权衡：** 最小 provenance 降低数据泄露并保持值对象轻量，代价是尚不能离线完整重建外部事实。
- **代码 / 测试证据：** [decision 字段与访问器](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[产品输出边界](../../product/lottery-strategy-routing-evaluation-v1.md)。

## 7. 第 27 节 concrete router 与第 29 节 evaluator 怎样防止语义漂移？

- **直接回答：** 两条路径共享 package-private `evaluateMembershipRoutingBranch`：confirmed premium 得到 `premium_override`，confirmed standard 得到 `baseline_default`，unknown/future 等失败关闭。Graph evaluator 不再复制另一份 tier switch；测试还把 concrete router 作为 oracle 对 target、branch、reason、fact provenance 与时刻做等价比较。
- **追问：** “为什么 helper 不导出成通用 Rule 接口？”
  - **追问回答：** 它只属于 Lottery membership 的具体语言，导出会给外部包造成稳定扩展承诺，并诱导把不同上下文条件塞进同一接口。package-private helper 足以消除当前重复。
- **权衡：** 窄 helper 复用精确但不可插件化；通用接口扩展表面容易，却会过早冻结错误抽象。
- **代码 / 测试证据：** [共享 branch evaluator](../../../internal/lottery/domain/membership_routing.go)、[branch 单测](../../../internal/lottery/domain/membership_routing_branch_test.go)、[oracle 等价测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。

## 8. `baseline_default` 为什么不能兜底 unknown、缺边或 provider 错误？

- **直接回答：** default 是图结构中的显式业务分支，只代表一份已确认的 standard tier；它不是异常恢复策略。unknown 表示事实不可解释，缺边表示图/执行 invariant 破坏，provider error 表示没有可信事实，三者都必须零决定。
- **追问：** “为了可用性，异常时走 standard 奖池风险不是更低吗？”
  - **追问回答：** 这仍是未经批准的业务决定，可能越权发奖、掩盖数据故障并污染审计。若业务真正需要降级，必须明确降级条件、目标、监控、期限和回滚，并作为新规则/发布策略建模。
- **权衡：** fail closed 会牺牲故障期成功率，换来不把未知伪装成合法 standard；隐式 fallback 看似可用，实则改变业务含义。
- **代码 / 测试证据：** [exact branch match 与 `matches == 1`](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[invalid/fuzz 测试](../../../internal/lottery/domain/strategy_routing_evaluation_fuzz_test.go)。

## 9. DAG 为什么只执行一条 path，不并行所有分支？

- **直接回答：** DAG 描述可能路径，不表示一次请求要遍历所有路径。每个 decision 对一份 fact 只选一个 exact branch，执行器立即进入该 destination；共享后继表示不同可能路径能汇聚，不表示本次同时走过多个父分支。
- **追问：** “那规则冲突如何解决？”
  - **追问回答：** v1 根本没有多个同时命中的 rule、priority、first-hit/all-hit 或结果合并，所以没有冲突消解协议。若未来引入多 operator 或并行命中，必须先定义 hit policy 与事实所有权，不能借 DAG 名称假装已有。
- **权衡：** 单路径确定、易预算、易解释；并行分支能支持组合决策，却需要冲突、取消、聚合和部分失败语义。
- **代码 / 测试证据：** [迭代 single-path loop](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[shared successor/converged decision 测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。
- **面经关联：** 牛客[快手复盘](https://www.nowcoder.com/discuss/723657746536464384)中的“规则冲突”用于提醒先声明本项目没有该能力，再说明何时才需设计。

## 10. 为什么用迭代循环而不是递归？

- **直接回答：** 单路径执行天然可以用 `currentNodeID`、`path` 和 `visited` 表达。迭代让“decision 前检查 context/step、append edge、到 terminal 终止”的检查点集中，partial path 所有权清楚，也不依赖调用栈。
- **追问：** “depth 只有 16，递归也不会栈溢出吧？”
  - **追问回答：** 对，拒绝递归不是因为当前会栈溢出，而是因为迭代更容易审计预算与取消。若递归实现同样清楚且测试不变量一致，也可以重新评估；不要伪造性能理由。
- **权衡：** 迭代稍显命令式，换来显式状态与边界；递归贴近图定义，但检查点和 partial result 更分散。
- **代码 / 测试证据：** [domain evaluator loop](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[深度 16 与取消边界测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。

## 11. 一个 step 怎样精确定义？为什么 terminal 不额外计数？

- **直接回答：** 一个 step 是“在 decision 上成功求得 branch，并沿唯一 edge 移动到 destination”。成本动作是 decision+edge traversal；到达 terminal 只是读取已选节点 target，因此不再加一步。`decision -> terminal` 的 path 长度就是 1。
- **追问：** “graph read、fact read 为什么不算 step？”
  - **追问回答：** 它们受整体 duration/cancel budget，但不是拓扑遍历单位。把 I/O 和 edge 混进同一个计数会让 `maxSteps` 无法解释；依赖成本应由 duration、调用次数和未来观测指标约束。
- **权衡：** 单一 edge 单位易验证但不代表每步耗时相同；若未来异构 operator 成本差异大，需要独立 cost model，而不是偷偷改变 step 定义。
- **代码 / 测试证据：** [`StrategyRoutingGraphStepBudget` 与 loop guard](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[1/16 边界测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。

## 12. 为什么既做 `graph.Depth() <= maxSteps` 准入，又保留实际 path step guard？

- **直接回答：** 最坏深度准入保证同一 revision/配置不会因会员 tier 不同出现“短路径成功、长路径执行一半失败”的事实相关可用性差异；loop guard 是运行时最后防线，防止未来 validator、伪造 aggregate 或执行回归绕过准入。
- **追问：** “只检查实际路径不是能接受更多请求吗？”
  - **追问回答：** 会让 service 对同一配置的可执行承诺依赖用户事实，而且可能读完敏感事实、走出半条 path 才失败。当前产品选择 revision 级全有或全无；如果未来确有局部执行需求，应重新定义发布准入和用户公平性。
- **权衡：** worst-case admission 可能拒绝本次恰好很短的 path，换来一致、可预测的 revision 可用性；actual-only 提高局部成功率，却引入事实相关失败。
- **代码 / 测试证据：** [application 事实读取前 depth gate](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[worst-case admission 测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)、[domain 双重 guard](../../../internal/lottery/domain/strategy_routing_evaluation.go)。

## 13. 第 28 节已有 depth 16，为什么第 29 节还要运行预算？

- **直接回答：** 静态预算回答“graph 是否允许存在/恢复”，运行预算回答“这个 service configuration 是否承诺执行”。合法 depth 16 graph 可以被 `maxSteps=8` 的较小服务拒绝；反之 `maxSteps=16` 也不能让非法 depth 17 graph 通过 Restore。
- **追问：** “为什么 maxSteps 不能为 0 表示 unlimited？”
  - **追问回答：** zero 值在 Go 中容易由漏配产生；把它解释成 unlimited 会让配置缺失静默扩大权限和资源面。v1 只接受显式 `1..16`。
- **权衡：** 双预算多一层配置与错误分类，换来存储安全边界和运行承诺解耦；单一常量更简单但无法按部署收紧。
- **代码 / 测试证据：** [budget constructor/Validate](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[invalid budget 测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。

## 14. `maxDuration` 是性能 SLO 或硬超时吗？

- **直接回答：** 都不是。它的 timer window 横跨 graph read、Clock、fact read 和 traversal，但只对观察 `context` 的依赖与本地检查点形成 cooperative cancellation。`MembershipRoutingClock.Now()` 没有 context 参数，当前必须是有界本地调用；若它阻塞，只能在返回后发现 timeout。Go 也不能强制抢占一个不合作的同步 provider 或已经开始的纯函数。当前 operator 常量规模、最多 16 step，只是把 evaluator 本地不可取消段限制得很小。
- **追问：** “设置 100ms 就能宣称 P99 小于 100ms 吗？”
  - **追问回答：** 不能。deadline 只是停止意图，不是延迟分布、成功率或生产负载证据；测试 timeout 值更不能冒充线上 SLO。
- **权衡：** cooperative context 与 Go I/O 生态兼容、成本低，代价是不合作代码可能晚返回；硬隔离需要进程/请求边界、资源配额和更复杂故障协议。
- **代码 / 测试证据：** [child deadline orchestration](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[blocking reader timeout 测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。
- **来源：** Go 官方 [`context` 文档](https://pkg.go.dev/context)将 `CancelFunc` 定义为通知工作放弃，且明确它不会等待工作停止；[官方 Context 博客](https://go.dev/blog/context)说明依赖通过 `Done` 信号尽快退出。

## 15. 为什么内部 duration 用 `time.Now()`，业务 evaluated-at 却用注入 Clock？

- **直接回答：** 二者所有权不同。`time.Now().Add(maxDuration)` 是本进程的运行安全 deadline，保留 monotonic reading 以抵抗 wall-clock 调整；注入的 `MembershipRoutingClock` 是可测试、可解释的业务 as-of，用于 fact future/freshness 并最终进入 decision，需规范为 UTC 且剥离 monotonic 部分。
- **追问：** “全部用 fake Clock 测试不是更可控吗？”
  - **追问回答：** 若把业务 fake time 直接驱动标准库 context timer，需要额外 scheduler/timer abstraction，容易让 production deadline 与测试模型分叉。当前用 channel 协调阻塞点、用真实短 deadline 验证端到端；业务时刻仍完全可控。
- **权衡：** 两个时钟概念增加解释成本，换来“业务事实时间”和“运行耗时”不混淆；共用一个 clock 简单，却可能丢 monotonic 保障或把测试时钟泄漏到运维预算。
- **代码 / 测试证据：** [application service](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[共享事实时刻 helper](../../../internal/lottery/application/membership_routing.go)。
- **来源：** Go 官方 [`time` monotonic clocks](https://pkg.go.dev/time#hdr-Monotonic_Clocks)说明 `time.Now` 同时含 wall/monotonic reading，`Add` 保留两者，`UTC`/`Round(0)` 会去掉 monotonic reading。

## 16. caller deadline 与内部 deadline 相同，为什么显式归 caller？

- **直接回答：** 这是项目为稳定错误归因冻结的策略：若 caller deadline 早于或等于 internal deadline，就只派生 `WithCancel(callerCtx)`；只有 internal 严格更早才安装私有 deadline cause。这样 equal 时不会让两个 timer 的调度先后随机决定错误类别。
- **追问：** “标准库不是自动取更早 deadline 吗？”
  - **追问回答：** 它能传播较早 deadline，但本项目还要区分“谁拥有相同截止时刻”。equal ownership 是业务错误契约，不是对标准库实现细节的猜测，因此显式比较。
- **权衡：** 多一个 helper 与测试，换来 deterministic attribution；直接 `WithTimeoutCause` 更短，但 equal deadline 的对外分类可能受先取消者影响。
- **代码 / 测试证据：** [`strategyRoutingGraphEvaluationContext`](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[earlier/equal caller ownership 测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。
- **来源：** [`context.WithDeadline`](https://pkg.go.dev/context#WithDeadline)说明 child 在 deadline、显式 cancel 或 parent cancel 中先发生者关闭；[`context.Cause`](https://pkg.go.dev/context#Cause)说明第一次取消设置 cause。equal 归 caller 是项目在其上增加的确定性政策。

## 17. 为什么用 `WithDeadlineCause`，又为什么 cleanup 不会伪装成 timeout？

- **直接回答：** 内部 deadline 到期时，私有 cause 能把“本 service budget 用尽”与普通 `context.DeadlineExceeded` 区分。提前调用返回的 `CancelFunc` 会真实取消 child，使 `Err()`/`Cause()` 表现为普通 canceled；它只是不设置传入的 deadline cause，同时释放 timer/父子引用。分类 helper 仅在 `context.Cause(evaluationCtx)` 等于私有 sentinel 时映射 `TimedOut`，显式 cleanup 形成的 cancel 不会被当成内部超时。
- **追问：** “直接看 `evaluationCtx.Err()==DeadlineExceeded` 不够吗？”
  - **追问回答：** 不够，因为 caller deadline 也会让 child 的 `Err()` 为 deadline exceeded，provider 还可能返回同一 sentinel。必须先看 caller，再看 child cause，才能保持责任边界。
- **权衡：** cause-aware 分类更精确，但需要 Go 1.21+ API 和更多测试；只看 `Err` 简单却会混淆三种 timeout。
- **代码 / 测试证据：** [私有 cause 与分类 helper](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[internal timeout/cleanup 测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。
- **来源：** [`WithDeadlineCause`](https://pkg.go.dev/context#WithDeadlineCause)明确 deadline 到期设置给定 cause，而返回的 `CancelFunc` 不设置该 cause；[`Cause`](https://pkg.go.dev/context#Cause)返回第一次取消原因。

## 18. caller、内部 budget 和 provider error 同时发生时，优先级是什么？

- **直接回答：** 固定为 caller cancellation/deadline > internal evaluation timeout > dependency error > dependency value 合法性。每个 dependency 返回后、遍历边界和最终成功前都重查 context；任何高优先级状态出现，低优先级 value/error 丢弃并返回 zero decision。
- **追问：** “为什么 provider error 不优先，毕竟它先返回？”
  - **追问回答：** 返回时刻与 goroutine 调度不等于责任归属。若 caller 已撤销请求，调用者应观察自己的 cancel；若 caller 仍活但 service budget 已用尽，应观察内部超时。只有两个 context 都活着，provider error 才是稳定解释。
- **权衡：** 显式排序增加边界检查，换来监控与重试策略稳定；first-observed 更简单，但同一故障可能随调度变类。
- **代码 / 测试证据：** [context error classifier](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[graph/Clock/fact 取消竞争测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。

## 19. provider 返回 `context.DeadlineExceeded`，为什么不一定是 caller 超时？

- **直接回答：** 先检查 caller 和 evaluation child。如果二者都存活，provider 的 wrapped `DeadlineExceeded` 只表示它自己的 operation/transport budget，graph reader 映射 evaluation failure，membership reader 映射 fact unavailable；不能伪装成 caller cancel 或内部 evaluation timeout。
- **追问：** “是否应该自动 retry？”
  - **追问回答：** 本节没有 retry budget、幂等/负载、退避和剩余 deadline 协议。自动 retry 可能放大尾延迟并跨过事实时刻；当前失败关闭，由未来受控上层决定是否重试整次 evaluation。
- **权衡：** 精确保留 provider ownership 让恢复策略保守，代价是调用方要理解稳定 class；把所有 deadline 合并方便，却导致错误重试和监控失真。
- **代码 / 测试证据：** [read error classifiers](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[live graph/fact provider deadline 测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。

## 20. dependency 同时返回 value 和 error 时为什么 error 胜出？

- **直接回答：** 非 nil error 是 provider 对该 operation 不可信的明确声明；同时返回的 graph/fact 可能是 partial、stale 或 diagnostic payload。继续使用会让失败路径偷偷产出 target，因此 value 完全不可观察。
- **追问：** “能否 Validate value 后继续？”
  - **追问回答：** `Validate` 只能证明对象内部形状，不能撤销 provider 对读取一致性/完整性的失败声明。若某 port 真有“stale value + warning”协议，应使用显式 result type 和批准语义，不复用 Go 的 `(value,error)` 模糊表达。
- **权衡：** error-wins 牺牲某些潜在 stale fallback，换来可信边界；best-effort 可能提高成功率，却缺少来源与有效期保证。
- **代码 / 测试证据：** [application return-order](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[graph/fact error-wins 测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。

## 21. 为什么所有失败必须返回 zero decision，而不是 partial path？

- **直接回答：** target/path 是可被下游误当成合法路由的高价值结果。step 超限、取消或坏 branch 时返回“前 N 步成功”会让调用方绕过最终确认、泄露内部 topology，并使 retry 从中间继续的语义不明。只有到 terminal、最后 context 检查和 `Confirmed()` 都通过才返回结果。
- **追问：** “排障不需要 partial path 吗？”
  - **追问回答：** 可以在受控诊断层记录低披露 trace/event，但不能放进公共业务返回值；本节还没有持久审计与对象级访问控制，所以先不保存高基数 path 失败详情。
- **权衡：** zero-only 降低现场可见细节，换来不可误用与隐私最小化；partial result 有利调试，却扩大 API、授权和生命周期问题。
- **代码 / 测试证据：** [domain 所有错误返回零值](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[zero decision assertion 与取消测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)、[fuzz invariant](../../../internal/lottery/domain/strategy_routing_evaluation_fuzz_test.go)。

## 22. 成功 path 为什么记录 rule、branch、reason、from/to，而不只记录 target？

- **直接回答：** 两条 branch 可以汇聚到同一 terminal/Strategy；只存 target 无法解释 premium 还是 standard 路径。`from/to` 证明实际 edge，rule/branch 提供机器语义，reason 与第 27 节稳定解释一致；ordered path 能验证连续性和最终 terminal。
- **追问：** “为什么不把完整 graph 或原始 tier 一起返回？”
  - **追问回答：** 完整 graph 含未走分支且数据量更大，原始 tier/subject 属于会员派生信息。最小 path 已足够解释路由；更多披露需要审计用途、授权和 retention 设计。
- **权衡：** 最小证据不能单独还原所有配置，但显著降低泄露和耦合；全快照便于离线分析，却扩大存储与访问控制面。
- **代码 / 测试证据：** [`StrategyRoutingGraphPathStep`](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[shared successor branch evidence 测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。

## 23. decision 怎样做到“不可变”，`Confirmed()` 又能证明到什么程度？

- **直接回答：** 字段不导出，domain 包外只能通过只读访问器获取；`Path()` 每次返回 defensive copy。`Confirmed()` 检查 identity/schema/root/terminal/target、fact provenance、canonical UTC、budget、path 连续性、无重复节点和 branch/reason 配对。当前 production 构造路径只有 evaluator 会基于具体 graph 形成完整结果；同包测试仍可伪造字段，所以 `Confirmed()` 和 graph 对照测试不能省略。
- **追问：** “`Confirmed()` 能否独立证明 path 真存在于原 graph？”
  - **追问回答：** 不能完全独立，因为 decision 不携带整张 graph；它证明值对象内部一致，具体 edge/terminal target 与 graph 的对应由 evaluator 构造过程保证。未来若跨进程反序列化 decision，需要签名、重新加载 graph 校验或专门 Restore 协议。
- **权衡：** 不携带 graph 保持结果最小，代价是离开构造边界后不能独立重验拓扑；嵌入 graph 会增加复制、泄露和版本耦合。
- **代码 / 测试证据：** [decision/Confirmed/Path](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[forgery 与 defensive-copy 测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。

## 24. graph 已在 Create/Restore 校验，application/domain 为什么还要再 `Validate()`？

- **直接回答：** port 契约可能被测试 stub、新 adapter、未来重构或同包伪造破坏；执行是一条更高风险边界，必须在读取事实前重新确认 exact identity、schema、derived depth 和全图 invariant。不能把“通常来自可信 repository”当运行证明。
- **追问：** “重复 DFS 会不会影响性能？”
  - **追问回答：** 图有 128 nodes/256 edges/depth16 的硬上限，当前没有 profile 表明验证是瓶颈。若未来需要缓存 validated aggregate，应连同 immutable identity、发布失效和缓存污染威胁一起设计，不能直接删验证。
- **权衡：** 重复校验增加有界 CPU，换来 adapter 边界防御；完全信任上游更快，却让一个坏实现直接产出业务 target。
- **代码 / 测试证据：** [application Validate/depth gate](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[domain evaluator Validate](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[invalid aggregate 测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。

## 25. 已验证无环，执行时为什么还维护 `visited`？

- **直接回答：** 它是便宜的 defense-in-depth：如果未来 validator 回归、同包伪造或 graph/value 内部不一致导致 path 重访，执行器立即失败，而不是依赖 step budget 才停止。它也使 decision 的 no-repeat invariant 显式可审计。
- **追问：** “visited 是否会把 DAG 的共享后继误判为环？”
  - **追问回答：** 不会，因为一次 evaluation 只走一条 path；同一路径第二次到达同一 node 才意味着 cycle/revisit。第 28 节全图校验中的三色 DFS 则允许不同父路径共享已完成后继，两处语义不同。
- **权衡：** 每次求值多一个至多 17 个 entry 的 map，换来执行层止损；删除它代码更短，但对上游 invariant 完全单点依赖。
- **代码 / 测试证据：** [visited guard](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[graph/decision forgery 测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。
- **面经关联：** 掘金[百度 Golang 面经整理](https://juejin.cn/post/6844904200351154189)出现任务依赖排序/判环题型；这里只借题型，项目技术答案仍来自自身 graph 契约。

## 26. 错误包装为什么提供 `Is` 和显式 `Cause()`，却故意不提供 `Unwrap()`？

- **直接回答：** 普通调用者通过 `errors.Is` 只能看到一个 reviewed、低披露稳定 class；可信诊断代码必须显式 `errors.As` 到 `StrategyRoutingGraphEvaluationError` 后调用 `Cause()`。没有 `Unwrap()`，所以内部 SQL、node、provider 或 private deadline cause 不会沿标准 error chain 意外匹配或进入通用日志。
- **追问：** “这会不会违背 Go 惯用 wrapping？”
  - **追问回答：** 这是有意的信任边界，不是忘记 `%w`。标准库允许 error 类型自定义浅层 `Is`；项目用它暴露语义 class，同时通过专用方法保留受控诊断。代价是通用 telemetry 不会自动遍历 cause，必须有受信 adapter。
- **权衡：** 不透明 wrapper 减少泄露和错误匹配，增加专用诊断代码；开放 `Unwrap` 更方便，却可能让内部 `DeadlineExceeded` 被外层误判成 caller timeout。
- **代码 / 测试证据：** [application error type](../../../internal/lottery/application/strategy_routing_graph_evaluation_error.go)、[低披露/单 class 测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。
- **来源：** Go 官方 [`errors.Is`](https://pkg.go.dev/errors#Is)说明相等或自定义 `Is(error) bool` 可定义匹配，并建议 `Is` 只做浅比较。

## 27. 为什么 service 是只读却仍要做 64 并发测试？需要加锁吗？

- **直接回答：** service configuration 只读，请求级 graph/fact/path/visited/context 是局部变量，正确设计不需要 service 全局锁。Domain 64-worker 证明同一 immutable 输入可重复；application 64-worker 交错两组 subject、identity、tier、branch 与 target，并按 identity/subject 分别计数，证明这些被测请求结果不串；`-race` 再观察实际执行路径上的数据竞争。
- **追问：** “service 无锁是否保证 injected reader 也安全？”
  - **追问回答：** 不保证。并发调用契约要求被共享的 graph reader、fact reader 和 Clock 自身支持并发，或由 composition 提供隔离/串行 wrapper。本节只证明 evaluator/service 不主动共享可变请求状态。
- **权衡：** stateless service 易横向扩展且无需锁竞争，代价是把依赖线程安全作为清晰端口要求；在 service 外层粗锁可保护不安全 adapter，却会串行化所有请求并隐藏根因。
- **代码 / 测试证据：** [domain 64 并发测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)、[application 64 并发测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。
- **来源：** [`context` 官方文档](https://pkg.go.dev/context)说明同一 Context 可供多个 goroutine 安全使用，但它不自动让 Context 中携带的值或任意依赖对象并发安全。

## 28. `go test -race` 能证明线程安全吗？

- **直接回答：** 不能做数学证明。Race detector 只能发现实际运行路径发生的数据竞争；未覆盖路径、未触发调度或业务级原子性错误仍可能存在。它应与 stateless 设计审查、64 并发 fixture、普通测试和真实负载验收组合使用。
- **追问：** “race 通过后为什么还要并发断言？”
  - **追问回答：** 无 data race 不等于结果正确，例如加锁后所有请求仍可能错误复用同一 path。并发单测检查业务隔离与确定性，race 检查内存访问竞争，关注点不同。
- **权衡：** race 会显著增加运行开销但能发现难复现缺陷；只跑普通测试更快，却缺少并发内存访问证据。
- **代码 / 测试证据：** [domain 并发测试](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)、[application 交错请求并发测试](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。
- **来源：** Go 官方 [Data Race Detector](https://go.dev/doc/articles/race_detector)明确它只发现运行时实际发生的 race，覆盖不足时应让 `-race` binary 承受更真实 workload。

## 29. fuzz 测了什么，没测什么？

- **直接回答：** Fuzz target 生成 depth 1..16 的有界 graph，变换 tier、budget、future fact 和 graph 内部 mutation，断言不 panic、不死循环、错误只返回 zero decision，成功 path/target 始终在预算内。seed 还锁定 depth16、budget15/16、unknown 与多种伪造边界。
- **追问：** “跑了很多 execs 是否证明算法正确？”
  - **追问回答：** 不能。Coverage-guided fuzz 擅长发现人工遗漏输入，但不是状态空间穷举；当前 fuzz 也没有真实 Repository、timer 调度、跨系统故障或任意 topology generator。语义 oracle、表驱动边界和架构测试仍不可替代。
- **权衡：** fuzz 扩大异常输入覆盖，代价是可重复时间与 corpus 管理；只写例子更快，却容易遗漏组合边界。
- **代码 / 测试证据：** [evaluation fuzz target](../../../internal/lottery/domain/strategy_routing_evaluation_fuzz_test.go)、[deterministic unit tests](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)。
- **来源：** Go 官方 [Fuzzing](https://go.dev/doc/security/fuzz/)说明其使用 coverage guidance 变异输入、失败输入进入回归 corpus；官方也建议 target 快速、确定、每次调用不保留全局状态。

## 30. 超时测试怎样避免把 `time.Sleep` 当同步？

- **直接回答：** blocking reader 先关闭 `started` channel，测试确认依赖确实进入阻塞后才等待结果；reader 监听传入 context 的 `Done` 后返回。这样业务先后关系由 channel 建立，不靠猜“睡多少毫秒足够”。真实 deadline 仍由 timer 触发，并有独立 watchdog 防止测试永久挂死。
- **追问：** “为什么不全部使用过去 deadline？”
  - **追问回答：** helper 的 caller/internal/equal 归因可用过去的 absolute deadline 无等待地验证；端到端 service 测试还要证明 child context 真传给阻塞 dependency 并能解除阻塞，因此保留短真实 timer。
- **权衡：** channel 协调的测试更确定但 fixture 较多；裸 `Sleep` 简短却在慢 CI 上易 flaky，在快机器上也可能没有命中目标状态。
- **代码 / 测试证据：** [timeout 与 deadline ownership tests](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。

## 31. 为什么 evaluator 到 terminal 后不加载 Strategy 或调用随机选择器？

- **直接回答：** terminal 只表达非零 `StrategyID` 路由目标。Strategy 是否存在/发布、其 Award snapshot、weighted selection、随机源、Draw identity、幂等和库存都是另一组失败与一致性边界；塞进 evaluator 会让 graph 错误和抽奖错误互相污染。
- **追问：** “只返回 ID 会不会多一次 I/O？”
  - **追问回答：** 可能，但当前没有 production profile；清楚的 use-case composition 比提前融合更重要。第 30 节先冻结 Activity 对 exact graph/Strategy 的发布引用，之后才能决定是否同一快照读取或缓存。
- **权衡：** 分离增加上层编排与潜在 I/O，换来单一职责、独立重试/幂等和诚实结果语义；一次完成更像 demo 闭环，却难以定义 commit 边界。
- **代码 / 测试证据：** [decision 只含 target identity](../../../internal/lottery/domain/strategy_routing_evaluation.go)、[ADR 方案七](../../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)、[runtime 禁装配测试](../../../internal/lottery/application/membership_routing_architecture_test.go)。

## 32. 为什么本节没有 cache、RabbitMQ、Redis 或新 Migration？

- **直接回答：** graph 已由第 28 节 Repository 提供 immutable aggregate；第 29 节只新增同步求值语义，没有新的持久状态、事件所有权或已测性能瓶颈。提前缓存会新增 key、失效、污染和一致性协议，消息化则改变同步结果与取消语义。
- **追问：** “immutable revision 不是天然适合缓存吗？”
  - **追问回答：** 是潜在候选，但仍需定义 cache key 包含 exact identity、negative cache、corrupt entry、容量/淘汰、权限和发布绑定；本节连 runtime 都未装配，没有证据选择 Redis 或进程内 cache。
- **权衡：** 不缓存保持真实来源和故障边界简单，可能承担重复读取；缓存可降读延迟，却增加另一份可信状态。
- **代码 / 测试证据：** [产品架构停止线](../../product/lottery-strategy-routing-evaluation-v1.md)、[架构扫描](../../../internal/lottery/application/membership_routing_architecture_test.go)。

## 33. route success 与权限/资格是什么关系？

- **直接回答：** 没有蕴含关系。MembershipSubjectRef 是外部会员事实查询引用，不是当前 session Principal；tier 是业务 fact，不是 role；route success 只说明 graph 对该 fact 选择了 Strategy target。认证、RBAC、对象 scope 和 Participation eligibility 都尚未执行。
- **追问：** “premium 用户是否天然有 premium 页面/管理权限？”
  - **追问回答：** 不能。会员等级和访问控制属于不同 bounded context；把 tier 当 role 会造成横向越权。第 31～35 节分别建立公共访问模型、真实 session、服务端 RBAC、前端能力投影和越权/E2E。
- **权衡：** 正交建模增加显式映射与编排，换来业务路由不污染安全边界；复用一个字段简单，却把营销标签升级成身份凭证。
- **代码 / 测试证据：** [产品与访问控制章节边界](../../product/lottery-strategy-routing-evaluation-v1.md)、[decision 明确不含 subject/principal](../../../internal/lottery/domain/strategy_routing_evaluation.go)。

## 34. 如何观测 evaluator，而不泄露高基数或会员信息？

- **直接回答：** 当前可安全规划的是低基数 outcome/error class、阶段与聚合 duration；GraphID/revision、NodeID、StrategyID、fact revision、subject、tier、SQL/endpoint 不应做普通 metric label。需要 path 诊断时进入受控 trace/audit，配套采样、脱敏、retention 和对象级授权。
- **追问：** “没有 GraphID label 怎样排查单图问题？”
  - **追问回答：** metric 用于全局趋势，不承担单对象取证；可用受控日志/trace correlation 查 exact identity，但访问范围和保留期必须显式。不能为方便 dashboard 制造 cardinality 与隐私风险。
- **权衡：** 低基数指标稳定便宜但单案信息少；高基数 label 查询直观，却可能拖垮时序库并泄露业务标识。
- **代码 / 测试证据：** [低披露 Error](../../../internal/lottery/application/strategy_routing_graph_evaluation_error.go)、[产品威胁/可观测边界](../../product/lottery-strategy-routing-evaluation-v1.md)。

## 35. 什么时候才应该引入 operator registry、DSL 或正式规则平台？

- **直接回答：** 至少出现第二/第三种真实 Lottery operator，并能回答输入类型、事实 authority、版本兼容、命中/冲突策略、执行依赖、资源预算、发布审批、回滚、审计和安全沙箱；同时有变更频率/交付瓶颈证明代码发布已不合适。仅因“if/else 多”或简历想写引擎不够。
- **追问：** “Drools/DMN/OPA 怎么选？”
  - **追问回答：** 先确定问题类别：业务决策表、授权策略还是复杂推理，比较语义模型、类型系统、可解释性、部署/runtime、治理和团队能力；本节没有完成该需求识别，所以不能伪造产品选型结论。
- **权衡：** 平台化可提升受控配置效率，但会新增语言、兼容、治理和攻击面；closed evaluator 发布频繁但语义最小、类型和失败边界清楚。
- **代码 / 测试证据：** [ADR 重新评估触发条件](../../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)、[architecture generic-engine stop line](../../../internal/lottery/application/membership_routing_architecture_test.go)。
- **面经关联：** 牛客[规则引擎追问](https://www.nowcoder.com/discuss/723657746536464384)与[项目追问](https://www.nowcoder.com/discuss/838483750089453568)、掘金[技术选型复盘](https://juejin.cn/post/7198544446283235384)只说明面试会追“流程、冲突、为什么选”，不能代替本项目触发条件。

## 36. 面试官给一个故障场景，应该怎样现场推演？

**场景：** graph reader 返回 graph+error；返回期间 caller 被取消；同时内部 deadline 也可能到期。问最终返回什么、后续调用几次。

- **建议回答顺序：** 先说 invariant，再说优先级，再说副作用：
  1. 依赖返回后先查 `callerCtx.Err()`；若已取消，原样返回 caller error；
  2. zero decision/path；graph value 与 provider error 都不可观察；
  3. Clock 和 fact 零调用，不 fallback revision、不 retry；
  4. 若 caller 仍活再查 child；child 到期则返回稳定 internal timeout；
  5. 两个 context 都活才分类 reader error；即使 graph `Validate()` 能过也不能继续。
- **继续深挖：** 若 caller 与 internal deadline 完全相同，构造 helper 预先把 ownership 给 caller；若 provider 自己返回 wrapped `DeadlineExceeded` 而两个 context 都活，则是 provider failure，不是 caller/internal timeout。
- **代码 / 测试证据：** [application ordering](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)、[error-wins/cancellation/deadline tests](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)。

## 37. 面试时怎样准确描述第 29 节的测试证据？

- **直接回答：** 可以说测试覆盖 concrete-router oracle、edge 顺序无关、共享后继/多步、depth/step 1 与 16、exact identity、调用顺序与次数、typed nil、graph/fact error-wins、future/stale、caller/internal/provider timeout 归因、最终返回前取消、zero-decision、defensive copy、64 并发、fuzz 和架构禁装配；再说明 race/fuzz 只能覆盖实际执行与生成到的路径。
- **追问：** “单测通过能否说明生产性能/稳定性？”
  - **追问回答：** 不能。没有真实 provider、runtime、流量、故障注入、容量测试或生产 SLO；测试证明 contract implementation，不证明线上 P99/QPS、可用性和安全渗透。
- **权衡：** 诚实列证据边界不会显得“少”，反而避免把工具输出夸大为生产事实。
- **代码 / 测试证据：** [domain tests](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)、[application tests](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)、[fuzz](../../../internal/lottery/domain/strategy_routing_evaluation_fuzz_test.go)、[architecture tests](../../../internal/lottery/application/membership_routing_architecture_test.go)。

## 常见错误表述纠正

| 不准确表述 | 更准确说法 |
| --- | --- |
| “我实现了通用规则引擎/规则平台” | “我实现了 Lottery-owned、schema-v1、单 operator 的封闭类型化路由图 evaluator，并明确 registry/DSL 的触发条件。” |
| “DAG 会并行跑所有规则” | “DAG 表示可能路径；一次 membership evaluation 只沿一个 exact branch 形成 ordered path。” |
| “default 能兜底 unknown 和异常” | “default 只代表 confirmed standard；unknown、provider error、缺边一律失败关闭。” |
| “用了 context 就能硬中断任何调用” | “deadline 是 cooperative；依赖必须观察 context，本地同步 operator 不能被强制中断。” |
| “内部 timeout 就是 `context.DeadlineExceeded`” | “内部 budget 用私有 cause 映射稳定 class，故意不通过 `errors.Is` 暴露 `DeadlineExceeded`。” |
| “父子 deadline 相同标准库会稳定判 caller” | “equal ownership 是本项目显式比较后冻结的政策，不依赖 timer 竞态。” |
| “race 通过证明线程安全” | “race 只发现实际执行路径发生的 data race；还需设计审查与业务并发断言。” |
| “fuzz 跑很多次证明算法正确” | “fuzz 扩展异常组合覆盖，不是穷举证明，也不覆盖未建模的真实依赖。” |
| “同一 snapshot 就能完整重放” | “本次调用内固定 graph/fact/time；尚无跨 authority 事务、历史 fact reader 或持久审计。” |
| “route 到 Strategy 就完成抽奖” | “只返回 StrategyID 与 path；没有 Strategy load、随机、Draw、库存或发奖。” |
| “会员等级就是角色/权限” | “tier 是业务事实，不是 Principal/role；访问控制留在第 31～35 节。” |
| “服务已上线” | “domain/application 内核和测试已实现，但没有 runtime/API/Compose 装配。” |

## 简历与面试能力停止线

### 可以准确说

- 设计并实现 Lottery 专属、封闭类型化的不可变 DAG 单路径 evaluator；
- 以 exact graph revision、单一权威 fact snapshot 和 canonical evaluated-at 保证一次调用内的确定语义；
- 复用第 27 节 typed branch oracle，避免 concrete router 与 graph evaluator 漂移；
- 以最坏 graph depth 准入 + 实际 step guard + cooperative duration/caller cancellation 形成多层预算；
- 显式设计 caller/internal/provider error 优先级和私有 timeout cause；
- 采用完整或零决定、最小 path evidence、防御复制和低披露 error wrapper；
- 用 unit、并发、race、fuzz 与架构扫描覆盖顺序、边界、取消、不可变性和停止线。

### 不能说

- 已实现 Drools/DMN/OPA 等同能力、动态 registry、表达式 DSL、脚本沙箱或跨上下文规则平台；
- graph 已经 publish/active，或已与 Activity、Strategy version 原子绑定；
- 已接入线上 HTTP/API、Compose runtime、真实会员系统、Redis/RabbitMQ 或管理 UI；
- 已完成身份认证、服务端 RBAC、前端权限投影或越权 E2E；
- route success 代表 eligibility、authorization、Award selection、正式 Draw/Result 或权益发放；
- `maxDuration` 是生产 P99/SLO，race/fuzz/unit 是生产容量与安全证明；
- 当前 decision 已持久审计、可跨系统完全重放，或错误 cause 可直接公开给普通调用者。

## 模拟深挖清单

面试前应能不看文档依次回答：

1. 为什么第 28 节可信保存与第 29 节可信执行必须拆开？
2. 为什么 one operator 不足以证明 registry/DSL 抽象？
3. graph、业务 Clock、fact 为什么按该顺序且各一次？
4. exact revision、active revision、schema version 分别属于谁？
5. default 为什么不是异常 fallback？
6. shared successor 与单路径执行是否冲突？
7. step、depth、duration、SLO 各自回答什么问题？
8. 为什么最坏 depth admission 比 actual-only 更公平/可预测？
9. business clock 与 monotonic operation deadline 为什么分开？
10. caller/internal/provider timeout 怎样稳定归因？
11. `WithDeadlineCause` 的 cause 与返回 CancelFunc 有什么区别？
12. 为什么 error wrapper 不 `Unwrap`，又怎样保留受信诊断？
13. `Confirmed()` 能证明什么、不能证明什么？
14. graph 已 Validate 为什么执行仍有 visited 与 step guard？
15. unit、race、fuzz、64 并发各自覆盖什么盲区？
16. 为什么 Strategy load/random/Draw 和 Activity/RBAC 必须留到后续？
17. 出现第二种 operator 时，哪些事实才足以触发架构升级？
18. 怎样用一句话说明“已实现但未上线”，避免简历夸大？

如果这些问题只能背答案、不能回到产品基线、ADR、代码和反例，就还没有真正掌握第 29 节。
