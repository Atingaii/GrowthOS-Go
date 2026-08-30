# 第 28 节面试题：有界不可变路由图、三表持久化与严格恢复

本文面向“规则树/决策图、关系模型与 JSON、MySQL 外键和 CHECK、事务、索引、DDD Repository、幂等与故障恢复”类追问。第 28 节把第 27 节已经存在的 `premium_override` / `baseline_default` / Strategy target 词汇保存为 Lottery-owned、create-only 的 `StrategyRoutingGraph`：它是有唯一入口、允许共享后继、禁止环和不可达节点的有界 rooted DAG，而不是任意表达式平台。

能力边界必须先说清：本节已经有领域 aggregate、consumer-owned Creator/Reader port、未装配进运行时的 MySQL adapter、三条前向 Migration，以及 domain、fuzz、repository unit、schema integration 和专用最小权限 identity 的 repository integration 证据。它没有第 29 节图执行器或多步运行 path，没有第 30 节 Activity 发布/审批/回滚，也没有管理 API、UI、真实会员 provider、session、Principal、RBAC、数据范围或浏览器 E2E。数据库里“存在且结构合法”不等于“已发布、可执行、调用者有权使用”。

## 60 秒项目自述

> 第 27 节用一个具体会员等级 router 证明了线性责任链不能诚实表达多个成功出口；第 28 节只解决下一件真实事情：怎样把这份路由拓扑保存、恢复，并确保坏数据不能变成可信领域对象。我建立了以 `(GraphID, Revision)` 标识的不可变 rooted DAG，schema v1 只允许 membership decision 与 Strategy terminal；每个 decision 必须有 premium override 和 baseline default 两条边，default 仍只代表 confirmed standard，不吞 unknown 或依赖故障。
>
> 完整校验同时覆盖唯一 decision root、节点/分支唯一、端点存在、terminal 无出边、每个 decision 分支完备、全可达、无环，以及 128 nodes、256 edges、最长路径 16 的预算。算法用 DFS 三色状态区分“回到当前递归栈的环”和“再次到达已完成共享后继”，并缓存每个节点到 terminal 的最长后缀深度；输入和访问器都做防御复制，node/edge 顺序 canonical，避免持久化依赖调用方 slice 或 map 顺序。
>
> 存储没有把整图塞进 JSON，而是拆成 graph、node、edge 三张 InnoDB 表：数据库负责 composite identity、局部字段形状、前向引用和 Strategy target 存在性，领域在 create 前和 restore 后负责跨行全图不变量。create 在一个事务中按 header、nodes、edges 写入且不提供 update/upsert；read 在只读 repeatable-read snapshot 中有界读取，再严格 restore。header 的 root 没建反向 FK，因为 InnoDB 外键立即检查会与 node→graph 形成无法开始的插入环。这个 adapter 尚未运行时装配；图执行、发布、API 和权限都明确留在后续章节。

## 来源与可信度

- **项目事实：** 以 [Lottery Strategy Routing Graph 产品基线 v1](../../product/lottery-strategy-routing-graph-v1.md)、[ADR-0024](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)、[领域 aggregate](../../../internal/lottery/domain/strategy_routing_graph.go)、[Repository ports](../../../internal/lottery/application/repository.go)、[MySQL adapter](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)和 Migration/测试为准。产品与 ADR 决定语义，代码说明当前实现，测试只说明实际覆盖过的行为。
- **MySQL 8.4 官方主来源：** [FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)、[CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)、[Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)、[Consistent Nonlocking Reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)、[How MySQL Uses Indexes](https://dev.mysql.com/doc/refman/8.4/en/mysql-indexes.html)与 [JSON Data Type](https://dev.mysql.com/doc/refman/8.4/en/json.html)。
- **Go 官方主来源：** [Executing transactions](https://go.dev/doc/database/execute-transactions)、[`database/sql`](https://pkg.go.dev/database/sql)、[Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)、[语言规范：Slice types](https://go.dev/ref/spec#Slice_types)、[Fuzzing](https://go.dev/doc/security/fuzz/)与 [Race Detector](https://go.dev/doc/articles/race_detector)。
- **模型术语校准：** OMG 官方 [DMN 1.4 规范页](https://www.omg.org/spec/DMN/1.4)用于区分“持久化一个受限决策拓扑”和“实现完整决策模型/命中策略/引擎”；GrowthOS 本节没有实现 DMN。
- 外部链接已于 **2026-08-30** 复核可访问；只把上述官方页面用于其直接支持的语义，不从二手文章推导 MySQL 或 Go 行为。

## 牛客面经真实性未独立核验声明

下列页面是牛客社区用户的个人复盘，只用于观察可能出现的追问形态。本文没有独立核验发帖者身份、公司、岗位、轮次、题目原句、录音或录用结果；它们不是公司官方题库，也不是技术依据。帖子附带的解释可能错误，本文不采信帖子答案，技术回答仍以官方资料、项目契约和可执行代码为准。

- [字节后端个人复盘](https://www.nowcoder.com/discuss/565102127207440384)与 [26 秋招个人小结](https://www.nowcoder.com/discuss/838483750089453568)：用户自述被追问是否了解“正规规则引擎/规则引擎”；只用于准备“图是否等于引擎”的追问。
- [快手 Java 后端一面个人复盘](https://www.nowcoder.com/discuss/731843046232395776)：用户自述被问“为什么不用外键”；帖子中的回答不作为本项目取舍依据。
- [字节电商后端个人复盘](https://www.nowcoder.com/discuss/353158184137334784)：用户自述被问事务隔离/ACID、B+Tree/最左匹配、业务幂等、第三范式与 BCNF。
- [追一/携程/华为个人复盘](https://www.nowcoder.com/discuss/353154582975029248)：用户自述被问联合索引、项目为什么用 JSON/还有什么保存方式、数据库范式。
- [百度后端个人复盘](https://www.nowcoder.com/discuss/353159613296091136)：用户自述项目追问中出现“为什么用 JSON schema”。
- [ByteHouse 数据库实习个人复盘](https://www.nowcoder.com/discuss/572859889337335808)：用户自述被深挖数据库项目、redo/commit、B+Tree 与 LSM、JSON parser 等，说明项目中的存储选择通常会被继续追到实现与故障边界。
- [美团后端暑期实习个人复盘](https://www.nowcoder.com/discuss/634838974485254144)：用户自述被问是否了解 DDD。未找到可可靠表明面试官原句是“为什么定义 Repository port”的个人复盘，因此本文的 Repository 追问是根据项目代码自然延伸，不能伪称真实原题。

---

## 1. 第 28 节到底解决了什么，为什么不是直接做执行器？

- **直接回答：** 它解决“合法路由拓扑怎样拥有稳定身份、被原子保存并被严格恢复”。第 27 节已经给出 rule/branch/default/target 业务词汇，但一跳内存 policy 没有 node/edge identity、跨 revision 引用、损坏恢复或资源预算。若同时做执行，会把存储格式、事实读取、遍历、取消和运行 path 的失败混在一个切片里，无法判断哪层错。
- **追问：** “没有执行器，这张图有什么业务价值？”
  - **追问回答：** 它交付的是可供后续执行器消费的可信配置边界，而不是线上用户价值闭环。价值在于先冻结“什么数据有资格被执行”；这不允许我把第 29 节能力提前写进简历。
- **权衡：** 分节增加一次集成工作，但让 schema、恢复语义和执行语义可分别审查、回退与测试；一次做全会更快看到 demo，却更难证明坏数据不会被执行。
- **代码 / 测试证据：** [Graph 产品范围](../../product/lottery-strategy-routing-graph-v1.md)、[ADR 停止线](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)、[领域 aggregate](../../../internal/lottery/domain/strategy_routing_graph.go)。
- **来源：** 项目产品/ADR 是本题的直接需求来源；OMG [DMN 1.4 规范页](https://www.omg.org/spec/DMN/1.4)列出其正式规范与机器可读模型，反证“有 node/edge 表”不能自动称为完整引擎。

## 2. 为什么叫 rooted DAG，而不是“规则树”？

- **直接回答：** v1 有一个显式 root，边有方向且禁止环，但两个或多个父节点可以共享同一后继，甚至 premium/default 两条边可以汇聚到同一 terminal。严格树要求除 root 外每个节点只有一个父节点，会排除合法共享，因此精确术语是 rooted directed acyclic graph。
- **追问：** “共享 terminal 是否会丢掉为什么命中？”
  - **追问回答：** 不会。branch identity 留在 edge 上；最终 Strategy target 相同也不能反推出走的是 premium 还是 baseline。第 28 节只保存结构，第 29 节若形成 path，必须记录实际经过的 edge，而不是只记录 terminal。
- **权衡：** DAG 支持复用、避免复制后继，但校验不能再用“节点被访问第二次就是环”的简化规则；严格树算法更简单，却会错误拒绝业务允许的合流。
- **代码 / 测试证据：** [`StrategyRoutingGraph` 与 DFS](../../../internal/lottery/domain/strategy_routing_graph.go)、[共享后继/合流测试](../../../internal/lottery/domain/strategy_routing_graph_test.go)。
- **来源：** [产品基线的 rooted DAG 不变量](../../product/lottery-strategy-routing-graph-v1.md)是直接契约；牛客的[规则引擎追问复盘](https://www.nowcoder.com/discuss/565102127207440384)只说明这类名词边界会被问，真实性未独立核验。

## 3. DFS 怎样既发现环，又不把共享后继误判成环？

- **直接回答：** 每个节点有 `unvisited / visiting / visited` 三色状态。进入递归栈标记 `visiting`；若边再次到达 `visiting` 节点，才是回边/环；到达 `visited` 节点表示该后缀已经完整验证，可以复用缓存深度。用一个全局 `seen bool` 会把第二个合法父节点到共享 terminal 误判为 cycle。
- **追问：** “为什么不使用 Kahn 拓扑排序？”
  - **追问回答：** Kahn 同样正确；当前 DFS 同一次遍历还能从显式 root 统计可达节点并计算最长后缀深度，且规模硬上限很小。契约冻结结果，不冻结算法，若 profile 或可读性证据改变可替换。
- **权衡：** 递归 DFS 代码紧凑，但要有深度预算防止恶意深链；Kahn 需要入度与队列，避免递归却还需单独从 root 证明全可达和计算最长路径。
- **代码 / 测试证据：** [`strategyRoutingGraphDepth`](../../../internal/lottery/domain/strategy_routing_graph.go)、[cycle 与 shared-successor 反例](../../../internal/lottery/domain/strategy_routing_graph_test.go)、[拓扑 fuzz](../../../internal/lottery/domain/strategy_routing_graph_fuzz_test.go)。
- **来源：** [产品基线 6.4](../../product/lottery-strategy-routing-graph-v1.md)明确共享已完成节点合法、当前递归路径重复非法；Go 官方 [Fuzzing](https://go.dev/doc/security/fuzz/)说明 coverage-guided fuzz 用于探索人工容易遗漏的边界，但不构成穷举证明。

## 4. 图的 depth 怎样定义，合流时怎样计算？

- **直接回答：** depth 是从 root 到任一 terminal 的最长路径所含 edge 数；root 本身是 0，一 decision 直达 terminal 是 1。DFS 对每个节点缓存“到 terminal 的最长后缀”，父节点取所有 child depth 的最大值加 1。共享后继只计算一次，不把不同路径的前缀错误累加到缓存里。
- **追问：** “为什么不是节点数，或者最短路径？”
  - **追问回答：** 后续执行成本按走过的边/step 更直观，且产品边界已将一跳定义为 1；最短路径不能限制另一条更深分支。第 28 节的 16 只是配置安全预算，不是第 29 节执行超时或 SLO。
- **权衡：** 最长路径能覆盖最坏拓扑成本，但会拒绝只有少数深分支的图；放宽上限要评估恢复/执行风险并升级契约，不能静默改常量。
- **代码 / 测试证据：** [`Depth` 与最长后缀 memo](../../../internal/lottery/domain/strategy_routing_graph.go)、[depth 16/17 边界测试](../../../internal/lottery/domain/strategy_routing_graph_test.go)。
- **来源：** [产品基线 6.5](../../product/lottery-strategy-routing-graph-v1.md)给出精确定义与边界；此题是项目算法契约，不借外部帖子作技术依据。

## 5. 为什么设置 128 nodes、256 edges、depth 16 三种上限？

- **直接回答：** 三个维度控制不同放大面：node 数决定对象与索引空间，edge 数决定 adjacency 和校验工作，depth 决定递归栈与未来单路径步数。只限 depth 仍可能有宽图，只限 nodes 仍可能有大量边；恢复器还用 `LIMIT max+1` 区分“恰好上限”和“存储超限”。
- **追问：** “v1 每个 decision 只有两条边，256 是否冗余？”
  - **追问回答：** closed shape 会形成更紧的组合上限，但显式 edge 总预算仍是持久化/恢复协议，能在未来 schema 演进或坏库数据进入完整分配前先失败。它不是宣称 256 条边都能在当前其他不变量下组成合法图。
- **权衡：** 硬上限牺牲任意规模配置，换来可预测恢复成本和安全失败；真实需求超过时应新 ADR、新 schema/兼容测试，而不是从数据库读到多少就处理多少。
- **代码 / 测试证据：** [三项常量与早期检查](../../../internal/lottery/domain/strategy_routing_graph.go)、[repository `LIMIT + 1`](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[超限测试](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** [产品风险矩阵](../../product/lottery-strategy-routing-graph-v1.md)是上限的批准来源；Go [Fuzzing](https://go.dev/doc/security/fuzz/)只支持边界探索方法，不提供这些业务数值。

## 6. schema v1 为什么只有一个 rule、两个 branch 和两种 node kind？

- **直接回答：** 因为真实需求目前只有第 27 节 membership tier 路由。decision 精确绑定 `lottery.membership_tier.route_strategy`，terminal 只持有非零 StrategyID；每个 decision 精确有 `premium_override(false)` 与 `baseline_default(true)`。没有 expression、script、HTTP、inventory、authorization 或 `map[string]any`，未知 kind/rule/branch/schema 一律失败关闭。
- **追问：** “这不就是写死 if-else，为什么还建图？”
  - **追问回答：** 图解决的是身份、拓扑、共享后继、版本化保存与恢复，不是假装已经通用化规则语言。若当前只有一个 decision，图仍提供 durable boundary；若永远没有第二种真实 rule，多层重复 decision 可能没有业务收益，不能为展示技术而制造它。
- **权衡：** 封闭 schema 扩展需要代码、Migration 和兼容决策，但旧程序不会“尽力执行”未知配置；开放 JSON DSL 看似灵活，却同时引入类型、沙箱、发布、预算和故障治理债务。
- **代码 / 测试证据：** [closed node/edge constructors](../../../internal/lottery/domain/strategy_routing_graph.go)、[未知 schema/kind/rule/branch 失败测试](../../../internal/lottery/domain/strategy_routing_graph_test.go)。
- **来源：** [产品基线 4](../../product/lottery-strategy-routing-graph-v1.md)与 [ADR 方案比较](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)直接批准该范围；牛客两篇[规则引擎](https://www.nowcoder.com/discuss/838483750089453568)[追问复盘](https://www.nowcoder.com/discuss/565102127207440384)只用于准备“为什么还不是引擎”，真实性未独立核验。

## 7. 为什么不用 graph header 一行 JSON 保存整张图？

- **直接回答：** JSON 能方便整体读写，但会把 node/edge scoped identity、source/target FK、terminal→Strategy FK、node union、branch/default 局部约束和精确损坏 fixture 都藏进应用 payload。三表并没有取代全图 validator，而是让数据库可靠证明它擅长的局部关系，领域继续证明跨行拓扑。
- **追问：** “MySQL 有 JSON schema 或 generated column，不也能约束和索引吗？”
  - **追问回答：** MySQL JSON 列本身不能直接建立普通索引，需要抽取 scalar 到 generated column/表达式索引；即便抽出部分字段，也很难把任意数量 edge 的两端和 Strategy 行做成清晰外键。JSON schema 校验也不会自动证明全可达、无环和最长深度。
- **权衡：** 三表需要多次查询/插入和 restore 组装，但当前规模有界并换来可审计关系完整性；若未来主要需求变成原样归档、极少局部查询，可重新评估同时保存 canonical document，但不能删除当前不变量证据。
- **代码 / 测试证据：** [三条 Migration](../../../migrations/sql)、[schema 集成测试](../../../migrations/strategy_routing_graph_schema_integration_test.go)、[严格 restore](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)。
- **来源：** MySQL 8.4 [JSON Data Type](https://dev.mysql.com/doc/refman/8.4/en/json.html)直接说明 JSON 列不被直接索引、可通过抽取 scalar 建 generated-column index；牛客[华为个人复盘](https://www.nowcoder.com/discuss/353154582975029248)和[百度个人复盘](https://www.nowcoder.com/discuss/353159613296091136)只表明“为什么 JSON/还有什么方案”是常见项目追问，真实性未独立核验。

## 8. 三表是否就是“满足第三范式”，为什么不把范式当唯一理由？

- **直接回答：** 设计先从不变量和访问模式出发，不是为了贴 3NF 标签。graph header、node、edge 分别有不同 identity 与约束生命周期，拆表避免在 header 重复 node/edge 组；但是否满足某个范式不能自动证明索引合适、事务正确、拓扑无环或系统可用。
- **追问：** “什么时候可以反范式化？”
  - **追问回答：** 需要真实 profile 证明 join/组装是瓶颈，并先定义同步、校验、回填、读旧值和故障修复协议。例如增加受控 canonical snapshot 用于读取可以讨论，但它只能是派生/校验数据，不能让两份来源都可独立修改。
- **权衡：** 规范化降低重复与局部不一致，代价是组装查询；反范式化降低特定读路径成本，代价是写放大和一致性协议。当前硬上限小、没有性能证据，因此不预加冗余 JSON/Redis。
- **代码 / 测试证据：** [三表语义](../../product/lottery-strategy-routing-graph-v1.md)、[explicit bounded selects](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[SQL 形状测试](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** MySQL [How MySQL Uses Indexes](https://dev.mysql.com/doc/refman/8.4/en/mysql-indexes.html)直接支持按查询与索引评估，未声称范式自动带来性能；牛客[字节个人复盘](https://www.nowcoder.com/discuss/353158184137334784)与[华为个人复盘](https://www.nowcoder.com/discuss/353154582975029248)自述出现范式追问，真实性未独立核验。

## 9. GraphID、Revision、schema version 和 Migration version 有什么区别？

- **直接回答：** GraphID 标识一个 Lottery 路由配置家族；Revision 与 GraphID 组成一份不可变内容的 lookup identity；schema version 说明 node/edge 字段与验证协议，目前只接受 1；Migration version 说明数据库 DDL 历史当前走到第几步。它们都不是 Strategy version、Activity version、会员 fact revision 或应用版本。
- **追问：** “为什么 Revision 不直接当 schema version？”
  - **追问回答：** 内容变化和解释格式变化是两个正交轴。同一 schema 可有许多业务 revision；未来 schema v2 也可能恢复某个业务家族的新 revision。合并后旧 reader 无法判断是内容没见过还是格式根本不理解。
- **权衡：** 多个 version 字段增加心智成本，却能独立处理内容身份、兼容与 DDL 演进；减少字段会把不同升级风险压成一个含糊字符串。
- **代码 / 测试证据：** [`StrategyRoutingGraphIdentity` 与 schema type](../../../internal/lottery/domain/strategy_routing_graph.go)、[Migration 状态/清单测试](../../../migrations/embed_test.go)。
- **来源：** [产品基线 5](../../product/lottery-strategy-routing-graph-v1.md)是直接术语契约；MySQL [Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)只解释 DDL 原子性，不把 Migration history 等同业务 revision。

## 10. Revision grammar 为什么是 1～128 ASCII bytes、大小写敏感且不 trim？

- **直接回答：** 持久 identity 需要跨 Go/MySQL/日志/迁移稳定且无 Unicode 规范化、空白折叠或 collation 等价歧义，因此用 `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`，数据库列使用 ASCII binary collation。`V1` 和 `v1` 是不同 token；`"v1 "` 不是自动修成 `"v1"`，而是非法输入。
- **追问：** “为什么不是内容哈希？”
  - **追问回答：** 当前 revision 是受控 correlation token。内容寻址还需冻结 canonical serialization、哈希算法、字段升级和碰撞处理；哈希也不证明审批、发布或 target 可用。没有这些协议就不能宣称 content-addressed。
- **权衡：** ASCII grammar 限制人类可读 Unicode 命名，但换来字节级身份稳定；内容哈希可增强去重/完整性，却会形成长期序列化兼容契约，本节无证据承担。
- **代码 / 测试证据：** [`validateStrategyRoutingGraphRevision`](../../../internal/lottery/domain/strategy_routing_graph.go)、[revision 边界测试](../../../internal/lottery/domain/strategy_routing_graph_test.go)、[binary collation 集成断言](../../../migrations/strategy_routing_graph_schema_integration_test.go)。
- **来源：** [产品 revision 契约](../../product/lottery-strategy-routing-graph-v1.md)与 [ADR](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)是直接来源；MySQL [How MySQL Uses Indexes](https://dev.mysql.com/doc/refman/8.4/en/mysql-indexes.html)说明字符集/collation 不匹配会影响比较与索引使用。

## 11. Go value receiver 已经复制 struct，为什么还要 defensive copy slice？

- **直接回答：** struct 复制只复制 slice descriptor；多个 slice 仍可能共享底层数组。构造时复制并排序输入，`Nodes()` / `Edges()` / `OutgoingEdges()` 返回新 slice，才能防止调用方在 create 后修改原输入或返回值而改变 aggregate 内部内容。
- **追问：** “节点元素如果以后加 map/slice 字段，当前浅复制还够吗？”
  - **追问回答：** 不够。当前 node/edge 只有标量值，所以元素复制安全；未来若加入引用字段，必须在构造/访问边界深复制或改成真正不可变值，并升级测试，不能沿用今天的结论。
- **权衡：** defensive copy 有 O(n) 分配成本，但上限 128/256 且配置以正确性为先；直接暴露内部 slice 更快，却破坏 revision 不可变、并发读和持久化确定性。
- **代码 / 测试证据：** [`Nodes` / `Edges` / `OutgoingEdges`](../../../internal/lottery/domain/strategy_routing_graph.go)、[输入与返回 slice 变异测试](../../../internal/lottery/domain/strategy_routing_graph_test.go)。
- **来源：** Go 规范 [Slice types](https://go.dev/ref/spec#Slice_types)明确 slice 与其他 slice 可能共享 underlying array；项目测试证明当前标量元素边界。

## 12. 为什么同时在 New、Validate、Restore 和数据库约束中重复校验？

- **直接回答：** 这些入口面对不同信任级别。`New` 阻止应用创建坏 aggregate；`Validate` 让 repository 在零值或同包伪造状态上写前失败；数据库约束阻止绕过当前代码的局部坏引用/坏行；`Restore` 假设存储可能损坏或来自旧 writer，重新证明全图不变量。它们是纵深防御，不是四份同义代码。
- **追问：** “数据库已有 FK/CHECK，Restore 是否多余？”
  - **追问回答：** 多余不了。FK/CHECK 不能证明每个 decision 恰有两条边、唯一 root 指向 decision、全可达、无环、最长深度和 aggregate 行数上限；特权写、旧数据或缺失反向 root FK 也可能产生可读坏行。
- **权衡：** 重复验证增加 CPU 和测试维护，但图有硬上限，成本可控；相信任一单层会把该层的表达限制或绕过途径升级成运行风险。
- **代码 / 测试证据：** [`New`/`Validate`/`Restore`](../../../internal/lottery/domain/strategy_routing_graph.go)、[row 约束 Migration](../../../migrations/sql)、[repository corrupt-row tests](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** MySQL [CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)说明 CHECK 按单行表达式求值且不允许 subquery，直接支持“不能证明跨行拓扑”的边界；牛客[快手外键追问](https://www.nowcoder.com/discuss/731843046232395776)仅作为题型来源。

## 13. 为什么 graph header 的 `root_node_id` 不建到 node 的反向外键？

- **直接回答：** node 已通过 `(graph_id, revision)` FK 依赖 graph header；若 header 再通过 `(graph_id, revision, root_node_id)` 依赖 node，就形成插入环。InnoDB 不做 deferred FK：插 header 时 root node 不存在，插 node 时 header 不存在，没有合法第一步，即使二者最终都在同一事务也不行。
- **追问：** “先插 header 的 NULL root，插 node，再 UPDATE root 呢？”
  - **追问回答：** 会把 create-only immutable revision 变成中间可变状态，并需要 nullable/UPDATE 权限和额外故障恢复。当前选择 header→nodes→edges 单事务，写前/读后领域校验 root，明确记录缺少反向 FK 的残余风险。
- **权衡：** 少一个 FK 允许特权绕过 repository 制造 header 指向坏 root；换来可执行的 create-only schema。该残余风险由最小权限、单事务、写前验证和严格 restore 共同缓解，而不是被文档隐藏。
- **代码 / 测试证据：** [graph Migration](../../../migrations/sql/000003_create_lottery_strategy_routing_graph.up.sql)、[node Migration](../../../migrations/sql/000004_create_lottery_strategy_routing_node.up.sql)、[无反向 FK 与 header-first 集成测试](../../../migrations/strategy_routing_graph_schema_integration_test.go)。
- **来源：** MySQL 8.4 [FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)明确 InnoDB 的 `NO ACTION` 等价立即 `RESTRICT`，且 child 无匹配 parent 的 INSERT/UPDATE 被拒绝；牛客[快手个人复盘](https://www.nowcoder.com/discuss/731843046232395776)只提供“为什么不用/用外键”的追问形态。

## 14. 为什么不能临时 `SET foreign_key_checks = 0` 解决循环引用？

- **直接回答：** 因为重新开启 `foreign_key_checks` 不会扫描并补验禁用期间插入的行。它把“无法表示的插入顺序”变成可永久遗留的不一致，不是 deferred constraint。应用账号也不应获得这种绕过能力。
- **追问：** “那用 trigger/stored procedure 做全图校验呢？”
  - **追问回答：** trigger 仍不自然证明全图 cycle/reachability/depth，还会把核心业务算法藏进数据库并扩大权限、迁移和调试面。当前共享领域 validator 已覆盖所有构造/恢复入口，数据库只承担能清晰表达的局部不变量。
- **权衡：** 应用校验无法防住数据库超级用户，但逻辑可版本化、单测和复用；数据库程序能靠近数据，却形成另一套发布、权限和测试体系。当前威胁模型选择前者并保留最小 DB 约束。
- **代码 / 测试证据：** [ADR 的 rejected alternatives](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)、[完整领域 validator](../../../internal/lottery/domain/strategy_routing_graph.go)、[权限/约束集成测试](../../../migrations/strategy_routing_graph_schema_integration_test.go)。
- **来源：** MySQL [FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)明确重新启用 `foreign_key_checks` 不触发数据扫描，因此这是直接官方边界。

## 15. MySQL CHECK 到底能保证什么，`NULL` 为什么要特别小心？

- **直接回答：** CHECK 表达式为 `TRUE` 或 `UNKNOWN` 都通过，只有 `FALSE` 失败，因此不能写一个会让非法 NULL 落成 UNKNOWN 的含糊条件。node 表先用 kind+nullable 字段的完整互斥表达式限定 decision/terminal union，Go restore 还用 `sql.NullString` / `sql.Null[uint64]` 区分数据库 NULL 与零值，再走领域构造器。
- **追问：** “能否在 CHECK 中查询 edge 表，保证每个 decision 两条边？”
  - **追问回答：** 不能。MySQL CHECK 不允许 subquery；即使某数据库支持更强表达，跨多行增删时的过渡状态和并发语义仍需谨慎设计。当前由完整 aggregate validator 证明。
- **权衡：** 严格 row CHECK 提早拒绝大量坏行，但不能因此复制完整 graph 算法进 SQL；只在 Go 校验则更统一，却放弃数据库可可靠承担的局部防线。
- **代码 / 测试证据：** [node/edge CHECK Migration](../../../migrations/sql)、[`restoreStrategyRoutingNodeUnion`](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[NULL/default corrupt tests](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** MySQL 8.4 [CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)直接说明 `TRUE/UNKNOWN` 通过、`FALSE` 违规，并明确 subquery 不允许。

## 16. 为什么所有 node/edge 外键都带 `(graph_id, revision)` scope？

- **直接回答：** NodeID 只在一个 immutable revision 内唯一。若 edge 只引用 `node_id`，同号节点可跨 graph/revision 串线；composite source/target FK 让边的两端必须属于本行声明的同一个 `(GraphID, Revision)`。node→graph FK 也让子行不能脱离 header。
- **追问：** “既然 edge row 自己有 graph/revision，应用层拼接时检查不就够了？”
  - **追问回答：** 应用检查应该保留，但数据库能精确表达这个关系，就没有理由把所有写入路径都假设为当前 adapter。FK 把局部 scope 完整性放到数据边界，restore 再验证防御旧数据和残余风险。
- **权衡：** composite key/FK 让索引和 SQL 更长、写检查更多，换来跨 revision 污染在数据库层立即失败；全局 NodeID 可缩短键，却引入独立 ID 分配服务且仍不能表达 revision 所有权。
- **代码 / 测试证据：** [node/edge Migration](../../../migrations/sql)、[跨 revision FK 失败测试](../../../migrations/strategy_routing_graph_schema_integration_test.go)。
- **来源：** MySQL [FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)说明对应列需相似类型，且引用/被引用 key 需要合适索引；项目 schema 决定完整 scope 的列顺序。

## 17. terminal→Strategy FK 能证明什么，不能证明什么？

- **直接回答：** 它能证明写入 terminal 时该 `StrategyID` 父行存在，并通过 `RESTRICT` 阻止某些被引用 Strategy 删除；它不能证明 Strategy 已发布、内容不可变、有独立业务 version、适用于该 Activity，或未来语义永不变化。
- **追问：** “那这个 FK 价值是否很小？”
  - **追问回答：** 不是。它排除了最基础的悬空 target，显著缩小坏状态空间；只是不能越权替代第 30 节发布绑定和 Strategy version 设计。精确说明局部保证比“有 FK 所以引用完整”更可信。
- **权衡：** FK 带来写检查、删除限制和同库耦合，但当前 Strategy 与 graph 同属 Lottery/MySQL 边界，收益明确；若未来分库，必须用新的发布验证、outbox/同步事实和补偿机制替代，不能静默丢掉保证。
- **代码 / 测试证据：** [node terminal FK](../../../migrations/sql/000004_create_lottery_strategy_routing_node.up.sql)、[缺失 Strategy/RESTRICT 测试](../../../migrations/strategy_routing_graph_schema_integration_test.go)。
- **来源：** MySQL [FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)只承诺关系存在与 referential action；[产品风险矩阵](../../product/lottery-strategy-routing-graph-v1.md)明确列出 published/versioned 仍未解决。

## 18. composite index 的列顺序怎样服务实际查询？

- **直接回答：** 主读取都先用完整 `(graph_id, revision)` 找 header，再按同一前缀读 nodes/edges；node PK `(graph_id, revision, node_id)` 和 edge PK `(graph_id, revision, from_node_id, branch_code)`正好覆盖左前缀过滤与 canonical 顺序。edge 另有 `(graph_id, revision, to_node_id)` index 支持 target FK/反向引用检查，不靠 `SELECT *`。
- **追问：** “为什么不为每个字段都建索引？”
  - **追问回答：** 索引要由真实查询和约束驱动；额外索引增加 insert、空间和维护成本。当前没有 list/latest/按 branch 全库搜索 API，也没有 profile 证明需要更多 secondary index。
- **权衡：** composite key 较宽但保留完整 scope 与读序；单列索引更小，却不能支持当前完整 identity 或避免跨 revision 混合。未来新增查询先用 `EXPLAIN`/真实负载验证，而不是猜。
- **代码 / 测试证据：** [三表 Migration](../../../migrations/sql)、[显式 ORDER BY 与 bounded query](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[SQL shape test](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** MySQL [How MySQL Uses Indexes](https://dev.mysql.com/doc/refman/8.4/en/mysql-indexes.html)明确多列索引可使用 leftmost prefix；牛客[字节](https://www.nowcoder.com/discuss/353158184137334784)与[携程/华为](https://www.nowcoder.com/discuss/353154582975029248)个人复盘自述出现最左匹配/联合索引追问，真实性未独立核验。

## 19. 为什么 Repository 是 create-only，而不是 `Save` / `Upsert`？

- **直接回答：** `(GraphID, Revision)` 表示不可变 snapshot；同键第二次 create 是稳定 conflict，即使 payload 相同也不暗中成功，更不能覆盖。新内容必须新 Revision。宽泛 `Save` 隐藏 insert/update 语义，upsert 会让已被引用的 revision 原地改义，破坏回放和审计解释。
- **追问：** “运营改错一条 edge，不能 UPDATE 是否太重？”
  - **追问回答：** 当前没有运营发布流程；正确演进是新 revision，后续由审批/发布选择 active reference。数据库坏数据修复属于受控迁移/运维事件，不能伪装成业务 Repository 更新方法。
- **权衡：** create-only 增加 revision 数量和未来清理策略，换来稳定历史与并发语义；原地更新节省存储，却要求乐观锁、读者一致性、发布原子性和审计协议，本节没有这些证据。
- **代码 / 测试证据：** [窄 Creator/Reader ports](../../../internal/lottery/application/repository.go)、[adapter 只有 INSERT/SELECT](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[duplicate conflict/no mutation tests](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** [产品 create-only 契约](../../product/lottery-strategy-routing-graph-v1.md)与 [ADR](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)直接决定语义；不从 ORM 或面经帖子反推该边界。

## 20. create 事务为什么按 header→nodes→edges，且所有 SQL 必须走同一个 `Tx`？

- **直接回答：** node FK 依赖 header，edge 的 source/target FK 依赖 nodes，所以顺序由即时约束决定。adapter 先 `Validate`，再 `BeginTxx`，通过同一 `Tx` 插 header、canonical nodes、canonical edges、逐行检查 affected row，最后 commit；任一步失败 deferred rollback，不能留下可读半图。
- **追问：** “事务中偶尔用 `db.ExecContext` 是否也会自动加入？”
  - **追问回答：** 不会。`sql.DB` 可能选择另一连接并在事务外执行，形成不一致视图或半写。事务期间的 query/exec/prepare 都必须调用 `Tx` 方法。
- **权衡：** 一次事务持有连接和锁更久，但图规模硬上限小并需要 aggregate 原子性；拆成多个事务提高局部吞吐，却会暴露只有 header 或缺 edge 的中间 revision。
- **代码 / 测试证据：** [`Create`](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[canonical write/rollback/affected-row tests](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** Go 官方 [Executing transactions](https://go.dev/doc/database/execute-transactions)明确事务操作应通过 `sql.Tx`，并警告事务内调用非事务 `sql.DB` 方法会产生不一致或死锁风险。

## 21. “用了事务”是否就意味着 create 幂等？

- **直接回答：** 不意味着。事务提供原子提交/回滚，不提供请求去重。相同 `(GraphID, Revision)` 的第二次 create 返回 already-exists；本节没有 idempotency key，也不把 duplicate 自动当成功，因为无法仅凭键判断 payload 与第一次相同或调用者意图相同。
- **追问：** “Commit 返回网络错误，直接重试不就行？”
  - **追问回答：** commit 可能已经在服务端 durable，只是响应丢失，所以 adapter 分类为 `ErrCommitOutcomeUnknown`。调用方应按 identity 查询并比较受控 canonical 内容，再决定前次是否成功；盲重试只能得到 conflict，若错误地生成同 revision 不同内容更危险。
- **权衡：** 显式 unknown outcome 增加调用方 reconciliation 流程，却避免把不确定提交误报失败或重复写；自动重试简单，但无法安全跨越 commit 边界。
- **代码 / 测试证据：** [`ErrCommitOutcomeUnknown`](../../../internal/lottery/application/repository_error.go)、[write commit 分类](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[commit failure test](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** Go [`database/sql`](https://pkg.go.dev/database/sql)规定 transaction 必须以 Commit/Rollback 结束，且 Commit 失败后 Tx 结果不可当有效；牛客[字节个人复盘](https://www.nowcoder.com/discuss/353158184137334784)自述出现业务幂等追问，真实性未独立核验，帖子答案不采用。

## 22. read 为什么需要只读 repeatable-read transaction？

- **直接回答：** header、nodes、edges 是三次查询；若各自看到不同提交时点，可能拼出数据库从未同时存在的 snapshot。adapter 用 read-only `sql.LevelRepeatableRead` transaction，让普通 consistent reads 共享首个 consistent read 建立的 snapshot，读完 commit，再在事务外调用严格 domain restore。
- **追问：** “create-only 后内容不改，单次查询是否就够？”
  - **追问回答：** create-only 降低已完成 revision 被修改的风险，但特权运维、迁移、异常数据与并发首次写入仍是边界；而且三表 aggregate 的读取协议本身应明确一致快照。它仍不是跨数据库或跨 Strategy 业务版本的全局 snapshot。
- **权衡：** read transaction 占用连接并依赖隔离级别支持，换来三次 SELECT 的一致视图；单条 join 可以减少往返，但会重复 header/node 数据、仍需限制行数和还原两类集合，未必更清楚。
- **代码 / 测试证据：** [`FindByIdentity` 与 `readSnapshotOptions`](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[snapshot read SQL expectations](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** MySQL 8.4 [Consistent Nonlocking Reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)明确 REPEATABLE READ 中同一事务的 consistent reads 使用首个 consistent read 建立的 snapshot，READ COMMITTED 则每次读新 snapshot。

## 23. 为什么读取用 `LIMIT 129/257`，而不是读满上限后直接截断？

- **直接回答：** `max+1` 是检测超限的哨兵：129th node 或 257th edge 存在就说明整个 stored aggregate 非法，返回 `ErrStoredStrategyRoutingGraphInvalid`。截断会把存储内容静默改义，可能恰好丢掉环、不可达节点或另一条分支，然后错误返回“合法子图”。
- **追问：** “为什么不先 `COUNT(*)` 再读取？”
  - **追问回答：** 会增加查询，并且若不在同一 snapshot 可能与后续结果不一致；`LIMIT max+1` 在固定上限下同时控制传输/分配并给出是否超限。当前不宣称它在所有执行计划上最优，只是边界确定。
- **权衡：** 多读一行是常数成本，换来区分 exact max 与 overflow；无 LIMIT 让坏库可放大内存，直接 max LIMIT 则掩盖损坏。
- **代码 / 测试证据：** [bounded node/edge selects](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[129/257 fail-closed tests](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** [产品恢复契约](../../product/lottery-strategy-routing-graph-v1.md)是直接来源；MySQL [How MySQL Uses Indexes](https://dev.mysql.com/doc/refman/8.4/en/mysql-indexes.html)用于校准索引访问，不替项目决定安全上限。

## 24. 为什么 Repository interface 放 application 包，而且拆成 Creator/Reader？

- **直接回答：** port 由用它的 Lottery application 定义，adapter 返回 concrete `*StrategyRoutingGraphRepository` 并用 compile-time assertion 证明实现。拆成 `Create` 与 `FindByIdentity` 两个窄能力，未来用例只依赖需要的方向，不得到 update/delete/list/latest/publish/execution 权限。
- **追问：** “为什么不定义一个通用 `Repository[T]` 做 CRUD？”
  - **追问回答：** CRUD 会抹掉 create-only immutable revision、严格 identity lookup 和领域错误语义；泛型减少样板，却把并不存在的 Update/Delete/Save 能力放进公共契约。接口应从真实消费者长出来，不为 mock 预造。
- **权衡：** 窄接口数量更多，组合时可能注入两个 port；换来最小能力、清楚 use-case 依赖和可独立替换。当前 adapter 未运行时装配，所以不能宣称线上已经使用这些 port。
- **代码 / 测试证据：** [application ports](../../../internal/lottery/application/repository.go)、[adapter compile-time assertions](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[port shape test](../../../internal/lottery/application/strategy_routing_graph_repository_test.go)。
- **来源：** Go 官方 [Code Review Comments：Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)明确接口通常属于使用者包，不应在实现侧只为 mock 预定义；牛客[美团 DDD 个人复盘](https://www.nowcoder.com/discuss/634838974485254144)只说明 DDD 会被问，真实性未独立核验，并未声称其原题涉及本项目 Repository。

## 25. Repository error 为什么分 not-found、conflict、stored-invalid、retryable 和 commit-unknown？

- **直接回答：** 它们驱动不同恢复动作：not-found 是身份不存在；already-exists 是 create 冲突；stored-invalid 必须 fail closed 并告警，不能执行/自动修；retryable 只覆盖死锁、lock wait、bad connection/network 等瞬态操作；write commit unknown 要先 reconciliation；generic failure 不承诺重试有效。公共 `Error()` 只渲染稳定 class，受信代码仍可通过 `errors.Is/As` 和 `Unwrap` 检查 cause。
- **追问：** “为什么 duplicate child row 不也分类为 graph already exists？”
  - **追问回答：** aggregate identity conflict 只在 header primary key insert 上成立；child duplicate 可能表示 schema/driver/数据异常，不能冒充“整个 graph 已存在”。adapter 只把 root/header MySQL 1062 映射成 graph conflict。
- **权衡：** 精细分类增加测试与调用方分支，但防止盲重试和错误成功；直接返回 driver error 简单，却泄露 SQL/表信息并把恢复策略耦合 MySQL error number。
- **代码 / 测试证据：** [stable error classes](../../../internal/lottery/application/repository_error.go)、[adapter classifiers](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[safe rendering/classification tests](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)。
- **来源：** Go 官方 [`errors`](https://pkg.go.dev/errors)定义 wrapping、`Is` 与 `As` 的检查机制；Go [`database/sql`](https://pkg.go.dev/database/sql)界定 transaction/commit 生命周期。具体错误分类是项目契约，不从帖子答案照搬。

## 26. Migration 的“atomic DDL”是否表示 000003～000005 在一个事务里一起提交？

- **直接回答：** 不表示。MySQL 8.4 atomic DDL 说的是一个受支持 DDL statement 的 data dictionary、storage engine 与 binlog 变化要么整体提交要么整体回滚；它不是 transactional DDL，DDL 会隐式结束当前事务，不能把多条 CREATE TABLE 当一个普通 DML transaction。项目因此一条前向 Migration 只承载一条 DDL，并用版本/dirty 状态逐步恢复。
- **追问：** “为什么不把三张表一个 Migration 一次 CREATE？”
  - **追问回答：** 那会制造多 statement 原子性的错误假设，也让失败定位/历史校验变差。header、node、edge 分三条单 DDL Migration，顺序与 FK 依赖清楚；历史一旦共享只能向前追加，不能改写 checksum。
- **权衡：** 三个版本增加迁移步骤，却准确匹配 MySQL 原子边界和依赖顺序；一个大脚本更短，但中途失败的状态与恢复协议更复杂。
- **代码 / 测试证据：** [000003](../../../migrations/sql/000003_create_lottery_strategy_routing_graph.up.sql)、[000004](../../../migrations/sql/000004_create_lottery_strategy_routing_node.up.sql)、[000005](../../../migrations/sql/000005_create_lottery_strategy_routing_edge.up.sql)、[embed/checksum/单 DDL 测试](../../../migrations/embed_test.go)。
- **来源：** MySQL 8.4 [Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)明确“atomic DDL is not transactional DDL”，DDL 隐式结束事务且不能与其他语句组成同一事务。

## 27. 当前 MySQL adapter 为什么“已实现但未装配”，又为什么单设 rule-graph 数据库 identity？

- **直接回答：** adapter 作为基础设施实现可独立单测和真实 MySQL round-trip，但本节没有创建/管理 use case、HTTP route 或可信权限模型，所以仍不接入 production composition。在线 API identity `growthos_app` 不获三张新表权限；隔离集成另用不复用 API/migrator 的 rule-graph identity，只授予三张 graph 表的 `SELECT, INSERT`，不授予 Strategy/schema_migrations 读取或 graph `UPDATE/DELETE`。
- **追问：** “terminal 有 Strategy FK，专用 identity 连 Strategy 都不能读，怎样写入？”
  - **追问回答：** FK 检查由数据库执行，不要求 repository identity 读取父表；集成 fixture 由高权限验证 identity 预置 Strategy，随后专用 identity 真实 create/find round-trip。该测试还核验 exact grants、forbidden surfaces、concurrent create 一胜一冲突、rollback、坏 logical root、REPEATABLE-READ/READ ONLY 与 EXPLAIN。它证明 adapter 在隔离环境下可用，不等于已经被在线 API 调用。
- **权衡：** 专用 identity 增加凭据、grant 与运维配置，却把未装配 repository 的能力限制在所需三表和 create/read；复用 migrator 或在线 API identity 更省配置，但扩大 schema/业务数据攻击面。未装配则保持本节无纵向 API 闭环的诚实边界。
- **代码 / 测试证据：** [adapter 注释与构造器](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)、[专用 identity MySQL integration](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_integration_test.go)、[在线 API identity exact grants/deny](../../../migrations/strategy_routing_graph_schema_integration_test.go)、[Migration README](../../../migrations/README.md)。
- **来源：** [产品架构停止线](../../product/lottery-strategy-routing-graph-v1.md)直接规定未装配 adapter 和不扩大的运行面；MySQL [FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)也说明创建 FK 需要对 parent 的 `REFERENCES` 权限，支持区分 migration 与最小应用身份。

## 28. 怎样测试 graph，不把几个 happy path 冒充正确性证明？

- **直接回答：** domain 表驱动测试覆盖 legal one-hop、两边合流、多个父共享后继、乱序 canonicalization、zero/unknown schema、坏 revision、duplicate node/branch、dangling、root、terminal 出边、缺分支/错 default、self/two/multi-node cycle、unreachable、128/256/16 边界和防御复制；fuzz 从合法骨架出发变异 topology，目标是永不 panic/死循环且成功结果始终 Validate。repository 测试覆盖零 SQL fail-early、写序/rollback/affected rows、duplicate、commit unknown、read snapshot、limits、NULL union/default 损坏、取消和错误分类；MySQL 集成再验证真实 DDL/FK/CHECK/collation/权限与 rollback。
- **追问：** “`go test -race` 和 fuzz 通过能证明线程安全/所有图都正确吗？”
  - **追问回答：** 不能。race detector 只发现实际运行路径上的 race；seed corpus 和限时 fuzz 都不是穷举。它们与精确反例、MySQL 集成、架构 negative check 互补，且本节没有生产负载、QPS、P99 或故障演练证据。
- **权衡：** 大量边界测试会锁定重要错误语义并增加维护成本；应锁业务不变量与外部可观察行为，不锁无关实现细节。SQL mock 能精确检查顺序，但不能替代真实 MySQL，集成测试也不能替代生产容量。
- **代码 / 测试证据：** [domain tests](../../../internal/lottery/domain/strategy_routing_graph_test.go)、[fuzz target](../../../internal/lottery/domain/strategy_routing_graph_fuzz_test.go)、[repository unit tests](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)、[MySQL repository integration](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_integration_test.go)、[MySQL schema integration](../../../migrations/strategy_routing_graph_schema_integration_test.go)。
- **来源：** Go 官方 [Fuzzing](https://go.dev/doc/security/fuzz/)说明 coverage-guided fuzz 可触达人工遗漏边界；[Race Detector](https://go.dev/doc/articles/race_detector)明确只发现运行时实际发生、被执行路径上的 race。

## 29. 什么时候才应该升级 schema 或引入真正规则引擎？

- **直接回答：** 新 node kind/operator、第三种真实 fact、可组合表达式、类型系统、冲突消解、未知值语义会触发 schema v2 评估；而运营无代码编辑、审批、模拟、灰度/回滚、bundle 分发、沙箱、执行预算、审计和独立故障域同时出现，才有规则引擎证据。仅仅“把 JSON 放数据库再 for-loop”不是成熟引擎。
- **追问：** “当前多个 decision 都是同一 membership rule，是否过度设计？”
  - **追问回答：** canonical 需求用一层就够；DAG 能表达合流和后续有证据的拓扑，但不应为了显示深度去堆重复 decision。若真实语义长期只有一次 membership tier 判断，应保持浅图；第 29 节还必须用第 27 节 concrete router fixture 做等价 oracle。
- **权衡：** 受限 schema 让变更走代码审查和发布，灵活性低但故障面清楚；通用引擎提高运营效率，却要求治理一门语言和运行平台。是否升级由真实变更频率、角色和故障成本决定。
- **代码 / 测试证据：** [schema v1 closed vocabulary](../../../internal/lottery/domain/strategy_routing_graph.go)、[ADR 重新评估触发器](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)、[第 29 节停止线](../../product/lottery-strategy-routing-graph-v1.md)。
- **来源：** OMG [DMN 1.4 规范页](https://www.omg.org/spec/DMN/1.4)提供正式规范及机器可读模型入口；牛客[规则引擎个人复盘](https://www.nowcoder.com/discuss/838483750089453568)只用于题型校准，真实性未独立核验。

## 30. 面试时怎样准确描述第 28 节完成度？

- **直接回答：** 可以说“为 Lottery 会员 Strategy 路由实现了有界不可变 rooted DAG 与 create-only 三表持久化；在 create/restore 双入口验证 root、分支完备、引用、全可达、无环和 128/256/16 预算；MySQL adapter 用单事务写入、repeatable-read 有界恢复和稳定错误分类，但尚未运行时装配”。不能说“自研通用规则引擎”“已发布在线规则”“已完成多步执行”“已做管理后台/权限/E2E”。
- **追问：** “简历如何量化亮点？”
  - **追问回答：** 可量化契约是三表、两类 node、两条精确 branch、128/256/16 边界、`LIMIT max+1`、header/nodes/edges 固定写序，以及 domain/fuzz/repository/MySQL 集成测试；不能编造线上 QPS、P99、命中率、节省成本或事故收益。
- **权衡：** 诚实的层级完成度不如“大而全引擎”吸睛，但能经受代码、Git、DDL 和故障追问；后续章节实际完成执行/发布/权限后再升级表述。
- **代码 / 测试证据：** [产品当前能力](../../product/lottery-strategy-routing-graph-v1.md)、[ADR 决策总结](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)、[领域代码](../../../internal/lottery/domain/strategy_routing_graph.go)、[MySQL adapter](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)。
- **来源：** Go [Race Detector](https://go.dev/doc/articles/race_detector)与 MySQL 官方资料共同提醒工具/约束各有证明边界；本题的完成度以仓库实际代码和验收记录为准，不以面经帖子或计划文档冒充实现。

## 常见错误表述纠正

| 错误表述 | 准确纠正 |
| --- | --- |
| “第 28 节实现了通用规则树/规则引擎。” | 只实现 schema v1 的 membership routing rooted DAG 与持久化；无 DSL、operator registry 或 executor。 |
| “这是树，所以每个节点只有一个父节点。” | 它是 rooted DAG；共享后继和两边合流合法，只有回到当前 DFS 路径才是环。 |
| “default 就是任何未知情况兜底。” | `baseline_default` 只对应 confirmed standard；unknown、unsupported、事实/依赖错误仍失败关闭，且本节尚未执行。 |
| “有 FK/CHECK，所以数据库证明了整张图合法。” | 数据库证明局部行形状与引用；root type、两分支完备、全可达、无环、深度/总数仍由领域 restore 证明。 |
| “同一事务会把 InnoDB FK 延迟到 commit。” | InnoDB `NO ACTION` 等价立即 `RESTRICT`；这正是 header root 不建反向 FK 的原因。 |
| “关掉 `foreign_key_checks`，再打开就会补验。” | MySQL 官方明确重新打开不会扫描并验证绕过期间的数据。 |
| “CHECK 为 NULL 时就是失败。” | CHECK 的 `UNKNOWN` 也通过；必须显式设计 NULL/union 条件并在 restore 再校验。 |
| “JSON 一行一定更快、更灵活。” | 未有 benchmark；JSON 会弱化当前需要的 scoped FK、Strategy 引用、局部约束与损坏 fixture，JSON 列也不直接建立普通索引。 |
| “事务等于幂等，commit 报错直接重试。” | 事务不提供请求去重；write commit error 可能 outcome unknown，需按 identity 查询并比较，而不是盲重试。 |
| “Revision 是内容哈希/发布时间/Git SHA。” | 它只是受控、大小写敏感的 bounded correlation token；没有 canonical hash 或发布语义。 |
| “Strategy FK 证明 target 已发布且不可变。” | 只证明写入时父行存在并约束部分删除；发布/业务 version 在第 30 节之后处理。 |
| “adapter 文件或专用 identity integration 存在，就表示 API 已在线使用。” | adapter 当前未 runtime composition；在线 API identity 未获新表权限。专用最小权限 identity 只形成隔离 round-trip 证据，无管理 API、UI 或真实流量。 |
| “已经有权限系统保护图配置。” | 第 28 节没有 session、Principal、RBAC、data scope 或浏览器 E2E；第 31～35 节才建立。 |
| “fuzz/race 通过证明所有输入和生产并发安全。” | fuzz 不是穷举；race 只覆盖实际执行路径；没有生产容量与 SLO 证据。 |

## 能力停止线

### 可以准确说

- 已把一个真实 membership Strategy 路由词汇建模为 Lottery-owned、唯一 root、允许共享后继的有界不可变 DAG；
- 已对 identity、schema、node union、精确 branch/default、root、dangling、duplicate、terminal、全可达、cycle 与最长深度做统一校验；
- 已实现 nodes/edges 的 defensive copy、canonical order、binary node lookup 和最长后缀 depth memo；
- 已用 graph/node/edge 三张 InnoDB 表表达 composite scope、前向 FK、Strategy target FK、row CHECK 与 create-only identity；
- 已明确省略 root 反向 FK 的 InnoDB 即时检查原因及残余风险；
- 已实现未装配 MySQL adapter：合法 graph 零歧义单事务写入，按 identity 的只读一致快照、有界读取与严格恢复；
- 已区分 not-found、already-exists、stored-invalid、retryable、write commit outcome unknown 与 generic failure；
- 已用 domain/fuzz/repository unit、MySQL schema integration 与专用 identity repository integration 覆盖 happy path、反例与边界；最终命令证据以本节 QA 记录为准。

### 不能说

- 已实现第 29 节 graph executor、operator dispatch、多步运行 path、step/time/cancellation budget；
- graph 已审批、发布、active、被 Activity 引用或支持灰度/回滚；
- 已接入真实会员 provider、Strategy runtime load/selection、Draw/Result、库存或权益发放；
- 已新增 HTTP/管理 API、runtime composition、运营 UI、Redis、RabbitMQ 或 PG；
- 已完成 session、Principal、RBAC/ABAC、tenant/data scope、前端权限裁剪或浏览器 E2E；
- Migration identity 的 schema 测试等于应用身份已经有新表权限或生产 adapter 已启用；
- FK/CHECK 已证明全部图不变量，或者 Strategy FK 已证明业务发布有效；
- 单测、sqlmock、fuzz、race 或 disposable MySQL integration 已证明生产 QPS、P95/P99、SLO、安全性或线上收益。

## 复习清单

- [ ] 能在 60 秒内说清第 27 节具体 router 与第 28 节持久化拓扑的关系；
- [ ] 能解释 rooted DAG 与严格 tree 的差别，以及 shared successor 为什么不是 cycle；
- [ ] 能手画 DFS 三色状态、reachable count 和 longest-suffix depth memo；
- [ ] 能说清 depth 16 是 edge 数且 exact 16 合法、17 非法；
- [ ] 能分别解释 128 nodes、256 edges、depth 16 和 `LIMIT max+1`；
- [ ] 能解释为什么 closed schema 不等于通用规则引擎；
- [ ] 能比较三表、单 JSON、反范式化及重新评估触发器；
- [ ] 能区分 GraphID、Revision、schema version、Migration version、Strategy/Activity/fact version；
- [ ] 能解释 revision ASCII grammar、binary collation、case-sensitive 和 no-trim；
- [ ] 能引用 Go slice 共享底层数组说明 defensive copy；
- [ ] 能画出数据库局部约束与领域全图校验的责任矩阵；
- [ ] 能解释 root 反向 FK 的插入环、InnoDB 即时检查和为什么不关 `foreign_key_checks`；
- [ ] 能解释 CHECK 的 TRUE/UNKNOWN/FALSE 语义与 subquery 限制；
- [ ] 能说明 composite FK 如何阻止 cross-graph/cross-revision edge；
- [ ] 能说明 Strategy FK 的价值和“未证明 published/versioned”的边界；
- [ ] 能用 leftmost prefix 解释当前 PK/index 顺序，而不是背诵“索引越多越好”；
- [ ] 能说明 create-only 为什么拒绝 Save/Upsert/原地 UPDATE；
- [ ] 能画出 Validate → Begin → header → nodes → edges → affected rows → Commit；
- [ ] 能区分事务原子性、请求幂等和 commit outcome unknown；
- [ ] 能解释 repeatable-read snapshot 为什么用于三次读取；
- [ ] 能解释 consumer-owned Creator/Reader port，而不把 DDD 说成万能模板；
- [ ] 能按恢复动作区分 repository error class，且不泄露 driver/SQL 文本；
- [ ] 能区分 single-statement atomic DDL 与 multi-statement transactional DDL；
- [ ] 能区分 adapter 已实现但未装配、在线 API identity 未扩权与专用最小权限 integration identity；
- [ ] 能明确第 29 执行、第 30 发布、第 31～35 权限/E2E 都尚未实现；
- [ ] 能把牛客链接只当个人复盘的追问形态，不当公司官方题库或技术答案。
