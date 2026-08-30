# 第 28 节：规则树第一次数据库升级——先保存一张可信的有界图

> 第 27 节已经证明：会员路由不是一条只会“继续/停止”的资格责任链，而是一个拥有两个合法成功出口的 Lottery 决定。本节不急着执行更多规则，也不把“一张图”包装成通用规则平台；我们先把已经出现的真实拓扑升级为 Lottery-owned、create-only、可严格恢复的 `StrategyRoutingGraph`，让 `(GraphID, Revision)` 第一次能够唯一指向一份完整持久内容。

- **起点：** 第 27 节已验收 tip `809d436`
- **学习分支：** `codex/lesson-28-rule-tree-schema`
- **产品规格提交：** `f27ce17`
- **架构决策提交：** `2786d96`
- **domain 提交：** `17a6c54`
- **Repository ports 提交：** `d53b2ec`
- **Migration 提交：** `4d9b074`
- **MySQL adapter 提交：** `8db8c3c`
- **恢复证据加固提交：** `4b79d1d`
- **真实 MySQL 集成提交：** `d7deafa`
- **一次性验收脚本提交：** `2d2c7c2`
- **长期 Compose v5 检查点提交：** `f6b537d`
- **运行身份 graph 拒绝提交：** `ebe0b70`
- **产品规格：** [Lottery Strategy Routing Graph 基线 v1](../../product/lottery-strategy-routing-graph-v1.md)
- **架构决策：** [ADR-0024](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)
- **API 记录：** [第 28 节 API](../../api/lessons/lesson-28.md)
- **QA：** [第 28 节 QA](../../qa/lessons/lesson-28.md) 独立区分已执行证据与 final-freeze pending，本文不重复预写终验结论
- **设计手记：** [第 28 节第一性原理手记](../../design-thinking/lessons/lesson-28.md)
- **面试问答：** [第 28 节面试问答](../../interview/lessons/lesson-28.md)

## 1. 这一节解决的不是“怎么跑规则”，而是“什么配置值得被跑”

第 27 节的 concrete router 已经冻结如下业务语义：

```text
confirmed premium
  -> premium_override
  -> premium Strategy target

confirmed standard
  -> baseline_default
  -> baseline Strategy target
```

当时的 `MembershipStrategyRoutingPolicy` 是调用方传入的不可变 Go value。它能证明路由语义，却不能回答：

- revision 对应的完整 targets 到底保存在哪里；
- 同一 revision 会不会被另一调用方重新解释；
- root、node、edge 与 Strategy target 是否形成一份完整配置；
- 数据库中的历史内容能否被确定地恢复；
- 旁路 SQL 或损坏数据是否会直接进入未来执行器。

所以本节的第一性问题是：

> 在还没有图执行、Activity 发布和权限管理之前，怎样把一份 Lottery Strategy 路由拓扑保存为有界、不可变、可验证、可恢复的权威快照？

本节答案是：

```text
closed v1 topology
  -> domain validates complete rooted DAG
  -> create-only Repository port
  -> one MySQL transaction: header -> nodes -> edges
  -> bounded repeatable-read snapshot
  -> strict domain Restore
  -> immutable graph value
```

顺序非常重要。系统先证明配置合法，再让未来执行器消费；不是让执行器一边走，一边猜怎样修复数据库。

## 2. 为什么生产名字是 Graph，而课程标题仍然说“规则树”

课程标题记录的是学习演进：这是项目第一次从线性 chain 进入显式 node/edge schema。

生产模型叫 `StrategyRoutingGraph`，原因是本节允许合法合流：

```text
                     +-> baseline branch --+
root decision -------+                     +-> one shared target
                     +-> premium branch ---+
```

更复杂的合法结构还可以让两个 decision 共享同一个后继。只要没有环、所有节点从 root 可达、每条路径最终到达 terminal，它就是 rooted DAG；强行叫严格 tree 会制造两个问题：

1. 为共享 terminal 复制节点，导致身份和引用漂移；
2. 把“每个非 root 只有一个父节点”错误提升为业务不变量。

`StrategyRoutingGraph` 这个名字还限制了所有权：

- `Strategy`：terminal 只能引用 Lottery `StrategyID`；
- `Routing`：图只回答“走哪个 Strategy”；
- `Graph`：允许共享后继，但不允许 cycle。

它不是跨上下文的 `RuleTree`、`DecisionEngine`、工作流或脚本平台。

## 3. 本节交付的完整切片

### 3.1 domain

[strategy_routing_graph.go](../../../internal/lottery/domain/strategy_routing_graph.go) 新增：

- `StrategyRoutingGraphID`；
- `StrategyRoutingGraphRevision`；
- `StrategyRoutingGraphIdentity`；
- `StrategyRoutingGraphSchemaVersionV1`；
- `StrategyRoutingNodeID`；
- 封闭 node kind：`decision | strategy_target`；
- 封闭 edge branch：`premium_override | baseline_default`；
- immutable `StrategyRoutingGraph`；
- `NewStrategyRoutingGraph` 与 `RestoreStrategyRoutingGraph`；
- root、拓扑、资源预算和 canonical order 校验；
- 防御性集合访问和并发只读能力。

### 3.2 application ports

[repository.go](../../../internal/lottery/application/repository.go) 新增两个窄端口：

```go
type StrategyRoutingGraphCreator interface {
    Create(ctx context.Context, graph domain.StrategyRoutingGraph) error
}

type StrategyRoutingGraphReader interface {
    FindByIdentity(
        ctx context.Context,
        identity domain.StrategyRoutingGraphIdentity,
    ) (domain.StrategyRoutingGraph, error)
}
```

端口没有 `Update`、`Upsert`、`Delete`、`List`、`FindLatest`、`Publish`、`Evaluate` 或局部 node/edge 修改。

### 3.3 MySQL schema

三条只向前追加的 Migration：

1. `000003`：graph header；
2. `000004`：scoped nodes；
3. `000005`：scoped edges。

Migration latest 因此从 2 演进到 5。一个 DDL 一个版本，让失败位置、历史 checksum 和恢复路径都保持可定位。

### 3.4 未装配 adapter

[strategy_routing_graph_repository.go](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go) 实现：

- create 前 domain validation；
- header、canonical nodes、canonical edges 的单事务写入；
- 同 identity 冲突映射；
- 完整 `(GraphID, Revision)` 查询；
- repeatable-read + read-only snapshot；
- nodes `LIMIT 129`、edges `LIMIT 257` 的超限探针；
- nullable union 与 `uint64` 严格扫描；
- commit 后 strict restore；
- 低披露 Repository error class。

“未装配”是能力边界：当前 `cmd/growth-api` 没有构造这个 adapter，现有 endpoint 也没有调用它。

## 4. 身份三件套不能混成一个 version

### 4.1 GraphID

`StrategyRoutingGraphID` 是非零 `uint64`，标识一条长期路由配置家族。

它不是：

- Activity ID；
- Strategy ID；
- tenant ID；
- 自增数据库行号；
- 当前 active revision。

本节没有 ID 分配服务，调用方必须提供非零值。

### 4.2 Revision

Revision 与 GraphID 组合形成 immutable snapshot identity：

```text
^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$
```

它是 1～128 bytes、大小写敏感的 ASCII token。构造器不 trim，也不接受 Unicode 或首尾空白。

Revision 不是天然内容哈希。真正成立的保证是：一次 `Create` 成功后，数据库 composite primary key 与 create-only contract 让同一 `(GraphID, Revision)` 不能被覆盖。

### 4.3 SchemaVersion

`schema_version = 1` 表示“如何解释 node/edge 内容”，不是 graph revision、Strategy version、Migration version 或应用版本。

`NewStrategyRoutingGraph` 只创建当前 v1；`RestoreStrategyRoutingGraph` 必须显式读取持久化 schema，任何 0、未知或未来值都失败关闭。

## 5. v1 拒绝万能表达式

v1 node 只有两种：

| kind | 必填 | 必须为空 | 出边 |
| --- | --- | --- | --- |
| `decision` | `lottery.membership_tier.route_strategy` | Strategy target | 精确两条 |
| `strategy_target` | 非零 StrategyID | rule code | 0 条 |

每个 decision 恰有：

| branch | `is_default` | 语义 |
| --- | --- | --- |
| `premium_override` | `false` | confirmed premium |
| `baseline_default` | `true` | confirmed standard |

本节没有 condition JSON、operator string、priority、script、HTTP call 或 `map[string]any` fact bag。数据库只持久化已经批准的稳定 rule/branch vocabulary，不让自由文本偷偷成为运行时语言。

`baseline_default` 也不是 unknown fallback。unknown/unsupported tier、缺失或陈旧 fact、provider failure 和 caller cancellation 都不能靠 default 获得 Strategy。

## 6. 一张合法 graph 必须同时满足哪些不变量

### 6.1 身份与资源上限

- GraphID 非零；
- revision 满足 exact ASCII grammar；
- schema version 精确为 1；
- root NodeID 非零；
- nodes 非空且最多 128；
- edges 最多 256；
- root 到任一 terminal 的最长路径最多 16 条 edge。

预算在构造大 map、建立完整 adjacency 或递归深入前检查。这样损坏数据库或恶意 fixture 不能先让进程无界分配，再得到“超限”错误。

### 6.2 节点与边局部形状

- NodeID 在一个 graph revision 内非零且唯一；
- decision 只有 rule code，没有 StrategyID；
- terminal 只有非零 StrategyID，没有 rule code；
- edge 两端均存在；
- edge source 必须是 decision；
- terminal 没有出边；
- 同一 source 的 branch 唯一；
- baseline/default 与 premium/non-default 标记逐字一致。

### 6.3 root、可达性、无环与终止

- header 指定唯一 root identity；
- root 必须存在并是 decision；
- 每个 node 都必须从 root 可达；
- 不允许 self-loop、两节点环或更长 cycle；
- 允许多个父节点共享一个已经完成的后继；
- 每条有限路径最终到达 `strategy_target`；
- longest depth 不能超过 16。

只检查“没有环”还不够：一个与 root 分离的合法小 DAG 仍然是隐藏配置。只检查“所有边端点存在”也不够：一个完整环的每条 edge 都可以有合法端点。

## 7. DFS 三色法怎样区分环和共享后继

实现使用三个状态：

```text
unvisited -> visiting -> visited
```

对节点 `n`：

1. 第一次进入时标记 `visiting`；
2. 递归访问全部 outgoing successor；
3. 计算 `n` 到 terminal 的最长剩余深度；
4. 完成后标记 `visited` 并缓存深度。

再次遇到节点时：

- 若状态是 `visiting`，说明它仍在当前递归栈中，形成 cycle；
- 若状态是 `visited`，说明另一条合法父边汇聚到已经完整验证的后继，直接复用缓存深度。

这正是共享后继与环的差别：

```text
legal convergence                 illegal cycle

A -> C <- B                       A -> B -> C
                                    ^       |
                                    +-------+
```

若只使用一个 `seen` 集合，第二条父边到 `C` 会被误报为环。若完全不记忆完成节点，共享子图会被重复遍历，最坏工作量可能随路径数放大。

## 8. 深度为什么必须取最长 root-to-terminal path

深度定义为 edge 数：

- root 本身深度 0；
- decision 直接指向 terminal，深度 1；
- 上限 16 合法；
- 17 失败。

对于一个 decision：

```text
depth(node) = max(depth(child) + 1)
```

不能使用第一次到达某节点时的 root distance 来替代“该节点到 terminal 的最长剩余深度”。共享后继可能先从浅路径被访问，再从深路径被访问；缓存的是节点自身后缀深度，它与父路径无关，最后由每个父节点逐层加一，才能得到全图最长路径。

测试中的“shared successor shallow-first”反例专门防止把 DFS 遍历顺序误当拓扑深度。

## 9. canonical order 是可重复性协议

调用方输入的 node/edge slice 可以乱序。构造时：

- nodes 按 NodeID 升序；
- edges 按 source、branch、destination 排序；
- `Nodes()` / `Edges()` 返回防御性副本；
- `OutgoingEdges()` 返回 canonical branch order 的副本；
- `Node()` 对已排序 node 做二分查找。

这带来四个好处：

1. MySQL 写入顺序不依赖 Go map iteration；
2. 同一内容的测试比较稳定；
3. 出错时定位到 canonical index；
4. 调用方修改输入或返回 slice 不会改写 aggregate。

canonical 不等于 content hash。Revision 仍是调用方提供的相关 token；本节没有自动摘要或签名。

## 10. 为什么使用 graph/node/edge 三表，而不是一个 JSON

一个 graph header 中存整份 JSON 的优点是真实的：一次读、一列版本化、写代码少。但它会让数据库看不见：

- scoped NodeID；
- node -> graph ownership；
- edge source/target 引用；
- terminal -> Strategy 引用；
- decision/target 字段互斥；
- source branch uniqueness；
- 可精确注入和验证的损坏 fixture。

应用层无论如何要做全图校验，不代表数据库应该放弃它能准确表达的局部约束。

三表的职责是：

```text
MySQL proves local row/reference facts
domain proves cross-row/topology facts
```

这不是重复，而是两个不同故障入口的纵深防御：正常写入前的内存对象可能坏，特权 SQL 或历史数据也可能绕过正常构造器。

## 11. 三张表的精确职责

### 11.1 graph header：Migration 000003

```text
lottery_strategy_routing_graph
  PK (graph_id, revision)
  schema_version = 1
  root_node_id > 0
  created_at DATETIME(6)
```

Revision 使用 `CHARACTER SET ascii COLLATE ascii_bin`，保持 byte-bound、大小写敏感身份。`created_at` 是行诊断时间，不是发布时间、业务 revision 或乐观锁。

### 11.2 nodes：Migration 000004

```text
lottery_strategy_routing_node
  PK (graph_id, revision, node_id)
  FK (graph_id, revision) -> graph
  FK strategy_id -> lottery_strategy
  CHECK exact decision/target union
```

`strategy_id` 可空是 discriminated union 的物理需要：decision 为 NULL，terminal 非 NULL。Strategy FK 证明 target 行在写入时存在，但不证明 Strategy 已发布、内容不可变或适合某 Activity。

### 11.3 edges：Migration 000005

```text
lottery_strategy_routing_edge
  PK (graph_id, revision, from_node_id, branch_code)
  FK scoped source -> node
  FK scoped target -> node
  CHECK exact branch/default pairing
```

source 与 target FK 都携带 graph/revision，因此数据库可以拒绝跨 revision edge。主键顺序支持按完整 identity 恢复；单独的 target 索引则支撑目标端复合 FK 与反向定位，而不是本节恢复查询的排序依据。

## 12. 为什么 header 的 root 没有反向 FK

直觉设计是：

```text
node -> graph
graph.root -> node
```

但这在 InnoDB 形成不可插入的循环：

- 先插 graph：root node 还不存在；
- 先插 node：parent graph 还不存在。

把两条 INSERT 放在同一个 transaction 也无济于事。事务提供原子可见性与 rollback，不会把 InnoDB FK 检查推迟到 commit；`NO ACTION` 也不是 deferred constraint。

本节明确选择：

1. header 保留非空 root identity；
2. node -> graph 与 edge -> nodes 的正向 FK 保留；
3. 不建 header -> root node 反向 FK；
4. create 前 domain 验证 root；
5. 单事务按 header -> nodes -> edges 写入；
6. 每次读后 strict restore 再验证 root。

没有使用：

- `foreign_key_checks=0`；
- nullable root 后 UPDATE；
- 占位 root node；
- `node.is_root` 代替 header identity；
- trigger 模拟全图校验。

这是公开记录的残余风险，不是隐藏缺陷：拥有高权限的旁路写者仍能制造 dangling root，而所有正常 reader 必须失败关闭。

## 13. Repository ports 为什么拆成 Creator 与 Reader

将 create 和 find 分成两个 consumer-owned interface，可以让未来用例只依赖所需能力，也避免一个“大 Repository”自然长出未批准操作。

`Creator` 的语义是：

- 接收完整、已构造 graph；
- create-only；
- duplicate identity 是冲突；
- 不提供局部写入。

`Reader` 的语义是：

- 按完整、已验证 identity 恢复一个 immutable snapshot；
- 不承诺 latest、list、publish 或 Strategy load；
- 找不到返回稳定 not-found；
- 坏存储返回 stable stored-invalid，而不是 partial graph。

Graph port 与既有 Strategy Repository 分离，因为它们是不同 aggregate、不同生命周期和不同最小权限表面。把 graph methods 塞进现有 `Repository` 会让 API runtime 在组合时意外获得 graph 写权限。

## 14. Create transaction 的真实协议

`Create` 执行：

```text
validate ctx / configured adapter / complete graph
  -> Begin transaction
  -> INSERT header
  -> prepare node INSERT
  -> INSERT canonical nodes, check every RowsAffected == 1
  -> prepare edge INSERT
  -> INSERT canonical edges, check every RowsAffected == 1
  -> caller cancellation check
  -> COMMIT
```

任一 child insert、statement close、RowsAffected 或 commit 前检查失败，deferred rollback 尝试清理事务。

duplicate header 的 MySQL 1062 映射为 `ErrStrategyRoutingGraphAlreadyExists`。其他 child FK/CHECK/permission/driver failure 不被误叫 duplicate。

commit 返回错误是特殊状态：server 可能已经提交，也可能没有。因此 adapter 返回 `ErrCommitOutcomeUnknown`，不能盲目把同一 revision 的另一个 payload再写一次。调用方以后要按 identity 查询并比较受控内容。

本节没有网络 command/idempotency key，所以不能把 duplicate 自动宣布为“前一次请求成功”。

## 15. Find 为什么必须使用一个只读一致性快照

图分三张表。如果分别在普通 autocommit 查询中读取：

```text
read header at T1
read nodes  at T2
read edges  at T3
```

即使当前正常写路径 create-only，旁路维护、未来 schema 操作或错误权限都可能让 reader 观察混合时刻。

本节使用：

```go
&sql.TxOptions{
    Isolation: sql.LevelRepeatableRead,
    ReadOnly:  true,
}
```

同一 snapshot 内按 header、nodes、edges 顺序读取。header 不存在时返回 not-found；unknown schema 在查询 child rows 前就失败关闭。

只读不是注释：真实 MySQL 验收会读取 `@@transaction_isolation`，并用事务内写入反例确认 read-only 选项抵达 server。

## 16. 为什么查询多读一行

domain 上限为：

```text
nodes <= 128
edges <= 256
```

Repository 查询使用：

```text
LIMIT 129
LIMIT 257
```

如果只 `LIMIT 128`，数据库里有 129 行时，reader 会看到前 128 行，然后可能把多余行静默忽略。多读一行能区分“刚好达到预算”和“实际超限”。

超限不能截断修复，必须返回 `ErrStoredStrategyRoutingGraphInvalid`。否则 revision 在数据库中的真实内容与 application 看到的内容不同。

## 17. strict restore 为什么放在 snapshot commit 之后

读取阶段只做 transport/storage 级工作：

- 显式列查询，不依赖 `SELECT *`；
- 扫描 `uint64` 与 nullable union；
- 检查行数预算；
- 结束 read-only transaction。

之后才调用：

```text
RestoreStrategyRoutingNode
RestoreStrategyRoutingEdge
RestoreStrategyRoutingGraph
```

恢复器拒绝：

- unknown schema/kind/rule/branch；
- NULL/非 NULL union 错配；
- 非 0/1 default marker；
- missing/terminal root；
- dangling edge；
- duplicate、缺 branch、多 branch；
- terminal outgoing；
- unreachable、cycle、depth 超限。

它绝不：

- trim revision；
- 选择第一个 root；
- 删除孤儿；
- 断开环；
- 自动补 default；
- 忽略多余行；
- 将 unknown token 当 baseline。

先结束 snapshot 再构造 immutable aggregate，也让数据库连接持有时间保持在有界扫描阶段，不把纯 CPU 拓扑校验放在 transaction 生命周期内。

## 18. nullable unsigned `BIGINT` 为什么需要真实扫描证据

Go 的 `uint64` 最大值大于有符号 `int64` 最大值。terminal `strategy_id` 又是 nullable union 列。

因此 adapter 使用：

```go
sql.Null[uint64]
```

而不是 `sql.NullInt64` 或先扫 string 再无界转换。测试不仅构造一个 `Null[uint64]` 值，还必须让 `18446744073709551615` 真正穿过 sqlx/driver scan 和 strict restore，证明 GraphID、NodeID 与 StrategyID 的 MySQL `BIGINT UNSIGNED` 到 Go `uint64` 映射没有符号截断。

## 19. 错误分类为什么必须低披露

graph repository 新增稳定类：

- `ErrStrategyRoutingGraphNotFound`；
- `ErrStrategyRoutingGraphAlreadyExists`；
- `ErrStoredStrategyRoutingGraphInvalid`。

并复用：

- invalid argument / not configured；
- retryable transaction；
- commit outcome unknown；
- generic repository failure；
- caller context error。

公开 `Error()` 只渲染审核过的 class，不打印 SQL、DSN、table content 或完整 graph。受信诊断代码仍可通过 error chain 检查 cause。

Stored-invalid 与 not-found 不能合并：前者说明 identity 存在但内容不可信，需要运维调查；后者只是没有该 snapshot。Stored-invalid 也不能回退成空 graph或旧 revision。

## 20. 数据库权限：测试 identity 与线上 runtime 必须严格区分

本节存在两个容易被混淆的事实：

1. 为了真实验收未装配 adapter，需要一个**测试专用、隔离 schema 的 graph repository identity**；
2. 当前长期运行的 `growthos_app` 并没有消费 graph repository，因此**不应获得 graph 表权限**。

测试专用 identity 必须：

- 与 migrator identity 不同；
- 与 API identity 不同；
- 只对三张 graph 表拥有 `SELECT, INSERT`；
- 不能 `UPDATE` / `DELETE` graph rows；
- 不能读写 `lottery_strategy`、`lottery_strategy_award`；
- 不能读写 `schema_migrations`；
- 不通过 mandatory role 获得额外权限。

target Strategy fixture 由 migrator/verification identity 预置，因为 graph repository 本身没有创建 Strategy 的职责。

长期 `growthos_app` 继续只有当前 runtime 已需的最小旧表权限，并显式不能访问 graph 三表。测试账号证明 adapter 的最小权限可行，不是给线上账号扩权的理由。

## 21. 测试矩阵怎样对应失败模型

### 21.1 domain 合法结构

- 单 decision、两个 target；
- 两 branch 汇聚同一 terminal；
- 多 decision 共享后继；
- 输入乱序后 canonical；
- 最长路径深度；
- depth 16 正边界；
- defensive copy；
- concurrent read。

### 21.2 domain 非法结构

- zero GraphID/NodeID；
- bad revision、unknown schema；
- unknown/mixed node union；
- unknown branch/default mismatch；
- duplicate node/branch；
- missing/terminal root；
- dangling source/target；
- missing/extra branch；
- terminal outgoing；
- unreachable node；
- self/two-node/multi-node cycle；
- nodes 129、edges 257、depth 17。

### 21.3 fuzz 与 race

拓扑 fuzz 将任意 bytes 投影成 node/edge 组合，目标是：

- 不 panic；
- 不无限递归；
- 成功返回的 graph 必须再次 Validate；
- 失败不得返回非零 aggregate。

race 测试证明 immutable graph 的并发读取不产生共享写竞争。它不证明数据库吞吐或 executor 并发。

### 21.4 Migration schema

真实 MySQL schema 验收应证明：

- clean exact v5；
- 三表列、类型、collation、CHECK、PK/index；
- 4 个 named RESTRICT FK 与精确复合列顺序；
- root 列刻意没有反向 FK；
- cross-revision edge 被拒绝；
- revision 大小写可共存、尾空白被拒绝；
- graph/node/edge 局部坏行被约束拒绝；
- transaction rollback 后库外观察不到 fixture；
- 长期 API test identity 无 graph 表权限。

### 21.5 adapter unit

- invalid graph 零 SQL；
- canonical header -> nodes -> edges；
- every RowsAffected；
- duplicate root mapping；
- child failure rollback；
- commit unknown；
- full identity snapshot read；
- unknown schema 在 child query 前失败；
- `LIMIT + 1` 超限；
- stored corrupt strict restore；
- nullable MaxUint64 scan；
- cancellation/driver classification；
- SQL 无 UPDATE/UPSERT/DELETE/LIST/LATEST。

### 21.6 adapter 真实 MySQL

测试设计覆盖：

- exact isolated grants 与 forbidden probes；
- schema v5；
- create/find round-trip；
- same GraphID/new Revision；
- duplicate identity；
- MaxUint64 identity/node/target；
- concurrent create 恰好一成功一冲突；
- child constraint failure 整体 rollback；
- missing identity；
- 特权 fixture 制造 dangling logical root，Repository fail closed；
- pre-cancel 无残留；
- read-only repeatable-read 抵达 server；
- scoped primary-key lookup plan；
- fixture 与临时 constraint 清理。

这些是真实验收应覆盖的矩阵；最终实际命令和结果只记录在本节 QA，不在课程文档中预写结论。

## 22. 建议验证命令

定向测试：

```bash
go test -count=1 ./internal/lottery/domain
go test -count=1 ./internal/lottery/application
go test -count=1 ./internal/lottery/adapter/mysqlrepo
go test -race -count=1 ./internal/lottery/...
go test -shuffle=on -count=20 \
  ./internal/lottery/domain \
  ./internal/lottery/application \
  ./internal/lottery/adapter/mysqlrepo
go vet ./internal/lottery/...
```

定向 fuzz：

```bash
go test ./internal/lottery/domain \
  -run='^$' \
  -fuzz='^FuzzRestoreStrategyRoutingGraphTopologyNeverPanicsOrLoops$' \
  -fuzztime=10s
```

真实 MySQL 测试必须使用 disposable schema 和显式双授权 token；环境变量与实际通过结果以终验阶段的第 28 节 QA 为准，不能把默认 skip 当通过。

全仓门禁：

```bash
make verify
```

范围负证：

```bash
git diff --name-only 809d436..HEAD
git diff 809d436..HEAD -- \
  cmd web configs \
  internal/participation
```

Migration、真实依赖验收脚本、Compose 构建版本检查点与未装配 MySQL graph adapter 是本节批准变化；公开 handler、runtime composition、Compose service/network/secret 拓扑、React 与权限系统应保持零扩张。不能把 `deploy/compose/compose.yaml` 的 lesson-28 image/version 更新误报为“Compose 文件零变化”。

## 23. 本节没有实现什么

- 没有 graph evaluator 或 traversal use case；
- 没有 operator registry、DSL、DMN、OPA 或脚本；
- 没有当前请求 fact 输入或多步 decision path；
- 没有 draft/publish/approve/retire/rollback/active revision；
- 没有 Activity 聚合或 Activity -> graph revision 绑定；
- 没有会员 authority adapter；
- 没有 runtime composition；
- 没有公开 HTTP/MCP/Agent API；
- 没有 UI、菜单、路由或按钮；
- 没有 session、Principal、RBAC/ABAC、tenant/data scope；
- 没有 Redis graph cache、RabbitMQ event 或 PostgreSQL projection；
- 没有正式 Draw、库存或权益；
- 没有线上 QPS、P95/P99 或浏览器 E2E 证据。

## 24. 为什么不在本节顺便做发布和权限

Schema 合法回答的是“这份图的结构能否被可信解释”。发布回答的是“哪份 revision 在什么 Activity 中生效”。授权回答的是“哪个 Principal 能创建、查看、批准或使用它”。

三个问题拥有不同状态机：

```text
structural validity != publication state != access right
```

将 `status=published`、`owner_role` 或 `tenant_id` 直接塞进 graph header，看似少建表，实际上会让 Lottery graph aggregate 同时拥有 Marketing 生命周期与 Governance 决定。本节刻意保持图纯净，为第 30～35 节留下真实的边界演进。

## 25. 可准确写进简历的边界

可以表述：

> 为 Lottery 会员 Strategy 路由建立有界不可变 rooted DAG，使用 GraphID/Revision/schema v1 和 graph/node/edge 三表绑定完整拓扑；在 Create 与 Restore 两端执行唯一 root、全可达、无环、共享后继、terminal 终止及 128/256/16 预算校验，并以 create-only transaction、repeatable-read read-only snapshot、严格恢复和隔离最小权限 MySQL 验收防止半图与坏数据进入后续执行层。

不能表述：

- 实现了通用规则引擎；
- graph 已被线上 API 执行；
- 已有运营发布与 Activity；
- 已完成登录、RBAC 或多租户隔离；
- 已有规则管理前端；
- 所有图不变量都由 FK 保证；
- 集成测试就是生产 SLO。

## 26. 第 29 节为什么自然出现

第 28 节已经形成一个可信输入：

```text
validated immutable graph
  + exact v1 rule vocabulary
  + bounded topology
  + strict restored snapshot
```

下一节才有资格回答执行问题：

- 怎样把受控会员事实交给 exact operator；
- 怎样从 root 选择唯一 branch；
- 怎样输出多步 path；
- 怎样限制 step、depth、time 与 cancellation；
- unknown fact/operator 怎样失败关闭；
- 怎样用第 27 节 concrete router 做语义等价 oracle。

第 29 节不能接受未经 `Validate`/`Restore` 的裸 node/edge rows，也不能为了通用性临时加入自由 expression。执行器应消费本节已经证明的 graph，而不是重新发明存储修复规则。

## 27. 本节复盘

本节最重要的学习点不是“三张表怎么建”，而是完整性责任如何分层：

1. 第 27 节的真实业务分支提供 schema 词汇；
2. 领域模型只表达 Lottery Strategy 路由，不预造万能规则；
3. rooted DAG 允许合流，三色 DFS 区分共享后继与环；
4. 128/256/16 把输入资源消耗变成契约；
5. canonical immutable value 让比较、写入和并发读取稳定；
6. 三表把数据库可证明的局部引用显式化；
7. root 反向 FK 缺失被诚实记录，并由写前/读后校验补强；
8. create-only transaction 绑定 revision/content；
9. bounded read-only snapshot 与 strict restore 不信任历史行；
10. 测试专用 graph identity 与线上 `growthos_app` 权限边界绝不混淆；
11. schema、执行、发布与授权按照真实问题顺序分节演进。

渐进式架构的价值，不是每一节都让界面多一个功能，而是每一节只增加一项可以被证据支持、也能被下一节安全消费的能力。
