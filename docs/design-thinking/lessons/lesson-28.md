# 第 28 节设计手记：先证明配置值得执行，再讨论怎样执行

> 第 27 节出现了第一个真实的多出口路由：`premium_override` 与 `baseline_default` 都是合法成功边，并且可以指向不同或相同的 Strategy target。第 28 节不把这个需求夸大成“通用规则平台”，而是解决一个更基础的问题：**怎样把已经批准的 Lottery 路由语义保存成一份不可变、可界定资源消耗、可被数据库局部约束、也能在读取后重新证明合法的完整拓扑。**
>
> 本手记记录 2026-08-30 的 Lesson 28 实现切片。规范性边界以 [Lottery Strategy Routing Graph 基线 v1](../../product/lottery-strategy-routing-graph-v1.md) 与 [ADR-0024](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md) 为准；实际命令、长期 Compose 与 pending 边界见[第 28 节 QA](../../qa/lessons/lesson-28.md)。本文详细展开架构师为什么做出这些选择、反例是什么、残余风险在哪里。它不声称 graph evaluator、Activity 发布、公开 API、runtime composition、认证授权或前端已经存在，也不预写尚未完成的 final-freeze 结果。

---

## 1. 从第一性问题开始：数据库不是本节的起点

### 1.1 表面需求

表面上，课程到了“规则树第一次数据库升级”，似乎只要设计几张表：

```text
rule_tree
rule_node
rule_edge
```

如果从表名开始，很容易立即讨论：

- adjacency list 还是 nested set；
- JSON 还是关系表；
- Redis 要不要缓存；
- 是否接一个规则引擎；
- 运营后台怎样拖拽节点。

这些都不是当前最先需要回答的问题。数据库只能保存我们已经能够准确命名的事实；如果业务语言还没有收敛，schema 只会把猜测固化。

### 1.2 当前已有的不可忽略事实

第 27 节已经用代码和测试提供了最小真实证据：

```text
confirmed premium
  -> premium_override
  -> Strategy target

confirmed standard
  -> baseline_default
  -> Strategy target
```

同时还证明：

- 两个 branch 可以指向不同 target；
- 两个 branch 也可以在迁移期汇聚到同一 target；
- branch 解释“为什么到达”，不能从 target 反推；
- unknown/unsupported/failure 不能命中 default；
- Route 与 Participation eligibility、authorization、selection 不同。

所以本节不是凭空发明树，而是把一个已经存在的、有向、多出口、允许合流的控制结构变成权威配置。

### 1.3 真正问题

真正问题可以写成：

> 给定一组来自受控业务词汇的 node/edge，系统怎样保证它们构成一份身份明确、结构完整、资源有界、不可原地覆盖、读取一致且恢复时失败关闭的 Lottery Strategy 路由快照？

这个问题包含六个维度：

1. **身份：** 什么唯一标识一份内容；
2. **语言：** v1 允许表达哪些东西；
3. **完整性：** 什么拓扑才算合法；
4. **持久化：** 数据库能证明什么，不能证明什么；
5. **恢复：** 历史行怎样重新成为可信领域值；
6. **边界：** 什么必须留给执行、发布和授权章节。

## 2. 本节成功的定义

### 2.1 正向定义

一份 graph 只有经过以下完整链路，才可成为未来执行器的候选输入：

```text
approved v1 vocabulary
  -> complete domain construction
  -> topology + budget validation
  -> immutable/canonical graph value
  -> create-only transaction
  -> database local constraints
  -> bounded consistent read
  -> strict domain restore
  -> validated immutable graph value
```

### 2.2 负向定义

本节成功不等于：

- 有 node/edge rows 就能执行；
- target Strategy 存在就已经发布；
- revision 名字像 `v1` 就是内容版本；
- transaction 成功就证明无环；
- FK 存在就证明 rooted DAG；
- graph table 可写就完成权限系统；
- domain test 通过就证明 MySQL driver 可以扫描全部 `uint64`；
- test-only identity 可写就应该给线上 API 扩权。

这组负向定义很重要。架构事故经常不是“完全没有控制”，而是把一项有限控制夸大成全链路保证。

## 3. 先划所有权：哪些事实属于谁

### 3.1 所有权矩阵

| 概念 | 权威所有者 | Graph 保存什么 | Graph 不能宣布什么 |
| --- | --- | --- | --- |
| 会员等级 | 外部会员 authority | 不保存主体事实；只使用稳定 rule code | 谁是 premium、事实是否新鲜 |
| tier -> Strategy 路由 | Lottery | decision、branch、default、target 拓扑 | 外部会员生命周期 |
| Strategy 内容 | Lottery Strategy aggregate | terminal 的 StrategyID 引用 | Strategy 已发布或内容不可变 |
| Participation 资格 | Participation | 不保存 | eligible/reject/次数/风险 |
| Activity 生命周期 | Marketing，第 30 节 | 本节不保存 | 哪份 revision active/published |
| 身份与权限 | Governance，第 31～35 节 | 本节不保存 | 谁能创建、查看、批准、使用 |
| 图执行 path | 第 29 节 | 本节不保存运行输入/结果 | 当前请求走了哪条边 |

### 3.2 为什么 Graph 在 Lottery bounded context

terminal 是 `StrategyID`，rule 的决定结果也是 Lottery Strategy target，所以 aggregate 属于 Lottery。

如果放到 Participation：

- Participation 将开始拥有 Lottery target；
- eligibility 与 routing 混成一个结果；
- 风险拒绝和会员 route failure 更难区分。

如果放到 Governance：

- role/permission 可能和会员 tier 混淆；
- “能访问 graph”与“graph 如何路由”混成一个模型。

如果做成 shared `internal/rules`：

- 第一个业务规则尚未证明跨上下文通用性；
- future context 被迫接受 Lottery vocabulary；
- 很快出现 `map[string]any` 和自由 operator。

### 3.3 为什么 Strategy target 只是引用

graph terminal 只保存非零 `StrategyID`，不复制 Strategy name、Awards、weight 或 outcome。原因是：

- Strategy aggregate 是另一条一致性边界；
- graph revision 不应因为 Strategy 显示名变化而复制写模型；
- 恢复 graph 不等于加载 Strategy；
- 第 30 节仍要决定 Activity 发布时怎样绑定不可变 Strategy 内容。

当前 FK 只能证明写入时父 Strategy row 存在。它不能证明 target 已发布、未来不会改义，或适用于某 Activity。这是明确保留的产品缺口。

## 4. 为什么叫 `StrategyRoutingGraph`，而不是 `RuleTree`

### 4.1 名称是一项边界控制

`StrategyRoutingGraph` 每个词都有限制作用：

- `Strategy`：禁止路由到 URL、脚本、队列、跨上下文 command；
- `Routing`：不承担资格、授权、库存或发奖；
- `Graph`：允许多个父节点共享后继；
- 没有 `Generic` / `Engine` / `Platform`：不承诺任意规则。

### 4.2 严格 tree 的隐藏成本

严格 tree 要求每个非 root 节点只有一个父节点。第 27 节允许两个 branch 汇聚到同一 target：

```text
premium  ----+
              +--> Strategy 100
baseline ----+
```

若坚持 tree，只能复制两个 terminal：

```text
premium  --> terminal-20 -> Strategy 100
baseline --> terminal-30 -> Strategy 100
```

复制会导致：

- 同一业务 target 出现两个拓扑 identity；
- 后续 target 变更需要同步修改；
- path/storage 比较出现无业务意义差异；
- 数据量和审计噪声增加。

所以真实数学结构是 rooted DAG，而“规则树”只保留为课程演进语言。

### 4.3 为什么还不叫工作流

工作流通常需要：

- long-running state；
- delay/retry/compensation；
- side effects；
- parallel/join；
- durable execution cursor；
- human approval task。

当前 graph 只有纯路由拓扑，没有执行状态或副作用。把它叫工作流会引入错误期待。

## 5. Aggregate 边界：为什么 graph 必须完整构造

### 5.1 aggregate 要保护的核心不变量

单个 node 合法，不代表 graph 合法；单个 edge 合法，也不代表 graph 合法。

例如：

- 两个 node 都合法，但 root 指向不存在 ID；
- 每条 edge 端点都存在，但整体形成 cycle；
- 每个 decision 有一条合法 edge，但缺另一 branch；
- 两个独立 DAG 各自合法，但只有一个从 root 可达；
- 每条路径局部合法，但最长深度为 17。

因此 aggregate boundary 必须覆盖：

```text
identity + schema + root + all nodes + all edges + derived depth
```

### 5.2 为什么没有 public setter

如果允许：

```go
graph.AddNode(...)
graph.RemoveEdge(...)
graph.SetRoot(...)
```

调用方会在多个中间状态之间移动：

- root 暂时 dangling；
- decision 暂时缺 branch；
- 删除 edge 后出现 unreachable；
- target 替换时 revision 内容原地变化。

本节选择一次传入完整集合，构造成功后 immutable。新的内容必须新 Revision。

### 5.3 immutable 的精确定义

这里的不可变表示：

- fields 私有；
- 没有公开 mutation method；
- 输入 slice 被复制；
- 输出 slice 是防御性副本；
- 同一 `(GraphID, Revision)` 不提供更新操作；
- restore 后仍形成同样的领域值。

它不表示：

- 物理内存有防篡改签名；
- DBA 无法执行旁路 SQL；
- Revision 是内容 hash；
- Strategy target 的业务内容永远不变。

## 6. 身份设计：GraphID、Revision、SchemaVersion 三者各回答一个问题

### 6.1 GraphID：哪个配置家族

GraphID 是 nonzero `uint64`。它回答：

> 这是哪一条长期演进的 Strategy routing configuration？

同一个 GraphID 可以有多个 Revision。本节不定义 active revision，也不定义谁分配 ID。

### 6.2 Revision：家族中的哪份不可变内容

Revision grammar：

```text
^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$
```

设计目标：

- ASCII byte 上限可同时在 Go/MySQL 准确表达；
- `ascii_bin` 保持大小写敏感；
- 无 trim 让调用错误显式暴露；
- 避免控制字符、换行和宽字符混淆；
- 可容纳 `v1`、Git-like token、时间序列或受控语义名。

Revision 本身不要求 hash，因为当前没有内容寻址、签名和跨环境 promotion 的真实需求。但 create-only PK 让成功写入后的 token/content 绑定可成立。

### 6.3 SchemaVersion：用哪个解释器读取

SchemaVersion 回答：

> 这份 node/edge 数据服从哪套封闭结构协议？

它不回答“内容是否更新”。v1 程序遇到 v2 必须失败关闭，不能尽力把未知 node kind 当 terminal。

### 6.4 反例：只用一个 `version`

如果表中只有：

```text
version = 5
```

无法判断它是：

- 第五版内容；
- schema 第五版；
- Migration v5；
- Strategy 第五版；
- Activity 发布序号。

名称模糊会在日志、API 和恢复逻辑中扩散。三类身份分离是为了让兼容决策可审查。

## 7. v1 语言为什么刻意封闭

### 7.1 节点 discriminated union

v1 node kind：

```text
decision(rule_code, strategy_id = NULL)
strategy_target(rule_code = NULL, strategy_id > 0)
```

decision rule code 只能是：

```text
lottery.membership_tier.route_strategy
```

### 7.2 Edge vocabulary

每个 decision 精确拥有：

```text
premium_override  + is_default=false
baseline_default  + is_default=true
```

Edge 不保存 condition expression。branch code 的含义由代码拥有的 v1 protocol 解释。

### 7.3 为什么不提前加入 operator registry

operator registry 看起来只多一个 interface，但立刻引出：

- 输入类型系统；
- nullable/unknown 三值逻辑；
- 数字与字符串比较；
- 时间和时区；
- operator version；
- timeout/cancellation；
- side-effect prohibition；
- unknown operator recovery；
- schema compatibility；
- metrics cardinality；
- 运营审批。

当前只有一个真实 rule。用具体 token 更诚实，也为第 29 节建立可对照的 semantic oracle。

### 7.4 为什么 default 仍然不是 catch-all

`baseline_default` 是结构上的 required default edge，却只承接 confirmed `standard`。

必须分开：

| 情况 | 是否进入 baseline |
| --- | --- |
| confirmed standard | 是 |
| confirmed premium | 否，走 override |
| unknown/unsupported tier | 否，输入非法 |
| fact missing/stale/future/corrupt | 否，无法决定 |
| provider unavailable | 否，技术失败 |
| caller canceled | 否，停止工作 |

如果把 default 当 catch-all，可用性表面提高，却可能把未知高价值人群或坏 payload 静默放进普通奖池。失败关闭是有意的业务安全选择。

## 8. 图合法性的形式化拆解

### 8.1 局部合法性

令：

```text
G = (V, E, r)
```

局部条件包括：

- 每个 `v in V` 有合法 ID 和 node union；
- 每个 `e in E` 有合法 source、target、branch、default；
- source/target 均在 V；
- source 是 decision；
- terminal 出度 0；
- decision 出度精确为 2；
- branch set 精确为批准集合。

### 8.2 全局合法性

全局条件包括：

- `r in V` 且 `kind(r) = decision`；
- 对所有 `v in V`，存在从 r 到 v 的路径；
- G 无有向环；
- 所有 maximal path 终止于 target；
- longest root-to-terminal edge count <= 16；
- `|V| <= 128`，`|E| <= 256`。

### 8.3 为什么每项都不能省

| 省略条件 | 可能接受的坏状态 |
| --- | --- |
| root type | 从 terminal 开始，图永不做决定 |
| all reachable | revision 中藏有不被执行但可误导审计的子图 |
| acyclic | 执行器无限循环或耗尽预算 |
| terminal no outgoing | target 同时又继续路由，语义冲突 |
| exact branches | unknown branch 或缺 default 造成不确定 |
| node/edge budget | graph bomb 消耗内存/CPU |
| depth budget | 节点不多但请求栈/执行步数过深 |

## 9. 校验管线为什么按这个顺序

当前 validator 的高层顺序是：

```text
identity
  -> schema
  -> root scalar
  -> node/edge count budgets
  -> canonical nodes + node union + uniqueness
  -> root existence/type
  -> canonical edges + edge union + endpoint/branch uniqueness
  -> per-node exact outgoing shape
  -> DFS cycle/depth/reachability
  -> derived depth
```

### 9.1 为什么预算在 map 分配前

如果传入百万节点，再先构造 `nodeByID`，即使最终返回 limit error，也已经产生内存放大。长度是 O(1) 可知的第一道资源门，应该尽早拒绝。

### 9.2 为什么 node map 在 edge 检查前

edge 完整性依赖 endpoint identity。先建立去重后的 `nodeByID`，才能准确区分：

- source missing；
- target missing；
- source kind 非 decision；
- terminal outgoing。

### 9.3 为什么 outgoing shape 在 DFS 前

如果 decision 缺边，DFS 可能把它视为深度 0 terminal，错误接受隐式终止。先验证 node kind 对应的出度/branch set，再计算拓扑。

### 9.4 为什么 reachability 在 DFS 后比较 count

从 root 出发的 DFS 状态 map 同时记录实际访问闭包。遍历结束后：

```text
len(state) == len(nodes)
```

才能证明全可达。这样无需另一遍图扫描。

## 10. 三色 DFS：共享后继不是环

### 10.1 两种“再次遇到”语义不同

```text
visiting: 当前递归路径尚未退出
visited:  该节点及其全部后继已经验证完成
```

再次遇到 `visiting`：形成 back edge，必有 cycle。

再次遇到 `visited`：形成 cross/forward convergence，可合法复用。

### 10.2 只有 `seen` 为什么错误

假设：

```text
root
  premium  -> A -> T
  baseline -> B -> T
```

从 A 访问 T 后，把 T 放进 `seen`。随后 B 再访问 T。若代码将“seen”直接解释为 cycle，就拒绝合法 DAG。

三色法保留“正在当前栈中”与“已完整完成”的区别。

### 10.3 为什么要 memoize 后缀深度

每个节点到 terminal 的最长剩余路径只由其后继决定，与从哪个父节点到达无关：

```text
suffixDepth(n) = max(suffixDepth(child) + 1)
```

共享后继完成后缓存该值，其他父节点可直接复用。这样验证复杂度保持 O(V+E)，而不是按所有 root-to-terminal path 枚举。

### 10.4 递归安全边界

递归在当前 128 nodes / depth 16 的硬上限下可控。但代码实际在完成 DFS 后才验证 derived depth；为什么仍不会递归 128 层以上？nodes 总数先被限制为 128，因此调用栈绝对有界。

若未来 node budget 大幅扩大，应重新评估 iterative DFS，不能机械复用当前实现。

## 11. 深度预算：不是节点数的别名

### 11.1 精确定义

深度是 root 到任一 terminal 的最长 edge 数：

```text
root only                  = 0（但 v1 root decision 必须有边，所以不能成为合法完整图）
root -> terminal           = 1
root -> decision -> target = 2
```

### 11.2 为什么取最长而不是最短

执行最坏资源消耗由最长路径决定。最短路径正常，不代表另一 branch 不会走 17 步。

### 11.3 shared successor shallow-first 反例

一个共享节点可能先通过浅路径被访问，又通过深路径引用。正确 memo 值是共享节点到 terminal 的后缀深度；父路径各自加一。

若错误缓存“第一次从 root 到达该节点的距离”，深路径可能被低估。专门的测试 fixture 应让浅路径先访问共享节点，以防测试顺序偶然掩盖 bug。

### 11.4 为什么 schema budget 不等于执行 budget

depth <= 16 只是拓扑静态上限。第 29 节仍要定义：

- max steps；
- time budget；
- cancellation；
- operator invocation count；
- path allocation；
- failure at a specific step。

静态 DAG 无环不等于每个 operator 都快速、纯净或可用。

## 12. canonicalization 与不可变性

### 12.1 为什么输入可以乱序

数据可能来自：

- Go constructor；
- MySQL `ORDER BY`；
- 测试 fixture；
- 未来 import tool。

aggregate 不应把调用方排序当隐藏前置条件，所以构造器复制并排序。

### 12.2 canonical order

```text
nodes: node_id ascending
edges: from_node_id, branch_code, to_node_id, is_default
```

这让：

- 同内容的 SQL 写入顺序稳定；
- 输出比较稳定；
- map iteration 不影响历史；
- unit test 能精确冻结语义。

### 12.3 防御性复制为什么是并发前提

如果 `Nodes()` 返回内部 slice，调用方可以：

```go
nodes := graph.Nodes()
nodes[0] = anotherNode
```

随后另一个 goroutine 读取 graph 就发生数据竞争或 invariant 漂移。

返回副本让 graph 自身保持只读。race test 证明当前 accessors 不共享可写 request state，但不替代调用方依赖对象的线程安全审查。

### 12.4 derived depth 也要重新验证

graph 保存 derived `depth`，`Validate()` 不只重算 topology，还检查保存值是否一致。这样同包测试或未来 unsafe restoration 若伪造 field，不会把错误缓存值当真。

## 13. 持久化候选方案矩阵

| 方案 | 优点 | 主要风险 | 结论 |
| --- | --- | --- | --- |
| 继续只在 Go 中传 policy | 最简单，无 Migration | revision/content 无权威绑定，无法恢复 | 不足 |
| graph header 单行 JSON | 一次读写，结构灵活 | FK/CHECK/identity 不可见，易原地覆盖 | 拒绝 |
| 通用 RuleNode + expression JSON | 表面扩展快 | 未定义 DSL、类型、安全、预算、兼容 | 拒绝 |
| 严格 tree 三表 | 概念直观 | 复制合法共享 target/后继 | 拒绝 |
| DAG 三表 + 双向 root FK | root 由 DB 保证 | InnoDB 立即 FK 导致插入环 | 拒绝反向 FK |
| `node.is_root` | 无双向 FK | root identity 分散，难证明至少一个 | 不采用 |
| nullable root 后 UPDATE | 可插入双向 FK | create-only revision 经历可变/不完整状态 | 拒绝 |
| 关闭 FK checks | 任意顺序可插 | 不补验、特权扩大、完整性虚假 | 拒绝 |
| bounded immutable rooted DAG + 三表 + logical root | 与需求匹配，局部约束清晰 | root 残余风险需双重验证 | 采用 |

方案评估的原则不是“哪个技术最现代”，而是：哪个方案在当前证据下用最少新语义满足完整性，并把不能保证的部分明确暴露。

## 14. 为什么不是单列 JSON

### 14.1 JSON 的真实优点

JSON 不是天然坏设计。它有：

- 一次 insert/select；
- payload 可整体 hash；
- schema 演进灵活；
- 不需要多表 join；
- 对 document store 友好。

### 14.2 当前选择拒绝 JSON 的原因

本节特别需要证明：

- node scoped identity；
- graph/revision ownership；
- source/target existence；
- target Strategy reference；
- node union shape；
- exact branch/default pairing。

这些恰好是关系数据库擅长表达的局部事实。JSON 会把它们全部推迟到 application parse 后，旁路 SQL 也难以构造精确约束反例。

### 14.3 “反正 domain 还要 Validate”为什么不成立

domain validation 与 DB constraint 防御不同入口：

```text
normal application write -> domain validation
privileged SQL / old bug / manual import -> database constraints + restore validation
```

必须由 domain 检查全图，不代表 DB 应放弃局部 FK/CHECK。反过来，有 DB constraint 也不代表 restore 可以省略全局校验。

### 14.4 何时应重新考虑 JSON

如果未来出现：

- graph 必须作为签名文档跨环境 promotion；
- 所有读取永远整图；
- node/edge 查询不再重要；
- schema kind 数量巨大且关系约束收益很低；
- document database 成为权威存储；

可以新增 ADR 评估 canonical JSON + content hash，但不能在 v1 原表中静默混用两套真相。

## 15. 三表 schema：每个约束证明什么

### 15.1 graph header

关键物理属性：

- composite PK `(graph_id, revision)`；
- `BIGINT UNSIGNED` 对齐 Go `uint64`；
- revision `ascii_bin`；
- schema exact 1 CHECK；
- root positive CHECK；
- `DATETIME(6)` diagnostic timestamp。

它证明 identity 唯一和标量形状，不证明 root node 存在。

### 15.2 node

关键物理属性：

- composite PK `(graph_id, revision, node_id)`；
- composite FK -> graph；
- nullable rule/strategy discriminated union CHECK；
- Strategy FK；
- target index。

它证明每个 row 属于 graph snapshot、target row 在写入时存在、union 局部合法，不证明每个 decision 的 edge 完整。

### 15.3 edge

关键物理属性：

- PK `(graph_id, revision, from_node_id, branch_code)`；
- composite source FK；
- composite target FK；
- target lookup index；
- exact branch/default CHECK。

它证明 edge 端点位于同一 scoped revision，并防止一个 source 同 branch 重复，不证明 cycle、reachability 或 decision 是否恰好有两条边。

### 15.4 为什么 enum-like token 用 binary semantics

稳定 code 是 protocol identity，不是面向人的自然语言。大小写折叠会让：

```text
premium_override
Premium_Override
```

在某些 collation 下被视作相同或比较行为不一致。ASCII + binary 使 Go literal 与 MySQL identity 对齐。

## 16. root 反向 FK：最值得讲清楚的残余风险

### 16.1 循环引用

如果同时定义：

```text
node.(graph_id, revision) -> graph
graph.(graph_id, revision, root_node_id) -> node
```

没有合法的第一条 INSERT。

### 16.2 transaction 不会延迟 FK

常见误解：

> “反正两条语句在同一个事务里，commit 时数据就完整了。”

InnoDB 在语句执行时检查 FK，不支持 PostgreSQL 风格 deferred constraint。`NO ACTION` 也等同立即 `RESTRICT`。因此 transaction 原子性不能解决插入顺序循环。

### 16.3 为什么不用 `foreign_key_checks=0`

关闭检查的问题：

- 需要更高权限；
- 会话中其他写入也失去保护；
- 重新开启不会扫描并补验已写坏数据；
- 正常 Repository 路径依赖绕过安全控制；
- 最小权限验收无法成立。

### 16.4 为什么不用 nullable root + UPDATE

该方案让 graph row 经历：

```text
header(root=NULL) -> nodes -> UPDATE header(root=id)
```

虽然 transaction 外不可见中间状态，但物理 schema 永久允许 NULL，且 Repository 需要 UPDATE；这与 create-only 完整 snapshot 的模型冲突，也让旁路写更容易留下半成品。

### 16.5 为什么不用 `node.is_root`

它可以避免反向 FK，却会：

- 将 aggregate root identity 分散到 child row；
- 最多一个 root 可逼近，至少一个 root仍难以用普通约束证明；
- 必须读取全部 nodes 后才能知道 header identity；
- root 改动看起来像 child row mutation。

### 16.6 最终控制链

本节采用：

```text
domain root validation before SQL
  -> create-only header first
  -> nodes reference header
  -> edges reference nodes
  -> atomic commit
  -> every read strict root validation
  -> minimal write identity
```

残余风险是：特权账号绕过 Repository 可以写合法 header、合法 child rows，却让 header.root 指向不存在或 terminal node。真实 corruption fixture 必须证明 reader 返回 stored-invalid，不自动猜 root。

## 17. 数据库与领域的完整性分工

| 不变量 | DB 能可靠证明 | Domain Create/Restore 必须证明 |
| --- | --- | --- |
| nonzero IDs | unsigned + CHECK | 再验证 Go value |
| revision identity | length/regexp/ascii_bin | exact grammar、无 trim |
| graph identity unique | composite PK | duplicate 语义是 conflict |
| node scope | composite FK | 查询必须带完整 identity |
| edge endpoint scope | 两个 composite FK | adjacency 与 node kind |
| Strategy row existence | nullable target FK | nonzero terminal shape |
| node union | CHECK | constructor/restore closed kind |
| branch/default | CHECK + scoped PK | 每 decision exact branch set |
| root existence/type | 无反向 FK | 必须验证 |
| all reachable | 不能用单行约束 | 必须验证 |
| acyclic | 不能用普通 FK/CHECK | 必须验证 |
| max aggregate size/depth | 不能按 aggregate CHECK | bounded read + validator |
| publication/access | 本节无状态 | 本节不得声称 |

关键思维是：完整性不是“放数据库”或“放代码”的二选一，而是按约束表达能力分配。

## 18. 为什么 Migration 分成 000003、000004、000005

### 18.1 依赖顺序天然是三步

```text
graph
  <- node
       <- edge
```

一个 DDL 一个版本让每个历史步骤可独立 checksum、定位和审计。

### 18.2 不写 down migration 的边界

项目当前采用 forward-only immutable migration。共享后不能修改旧 SQL；撤销业务代码也不能删除历史 migration。

若未来 schema 要退役，应追加新 migration 做迁移/归档，而不是改写 000003～000005。

### 18.3 Atomic DDL 不等于三条 migration 原子

MySQL atomic DDL 保护单条支持的 DDL 语句及其字典更新，不表示三份 migration 是一个跨版本事务。Migration runner 的 clean/dirty/version protocol 仍负责历史推进。

### 18.4 为什么 Migration latest 是 5，而 graph schema 是 1

```text
Migration v5: repository schema history progressed to fifth DDL
Graph schema v1: node/edge content follows first interpretation protocol
```

二者数字可能偶然接近，语义完全不同。

## 19. Ports：把能力表面压到最小

### 19.1 为什么分 Creator/Reader

单一大 interface 会诱导：

```go
Save
Update
Delete
List
FindLatest
Publish
Evaluate
```

其中每项都需要新的产品语义。拆分窄端口让未来 use case 只获得必需能力，也便于最小权限数据库 identity。

### 19.2 为什么 Create 接收完整 aggregate

如果端口暴露：

```go
CreateHeader
AddNode
AddEdge
```

application 必须管理半图和重试，aggregate invariant 跨越多个调用。完整 graph value 让“能传入 Repository”本身已经意味着 domain construction 成功。

### 19.3 为什么 Find 使用 identity value object

`FindByIdentity(ctx, graphID, revision string)` 会让每个 adapter 重复校验。`StrategyRoutingGraphIdentity` 将 nonzero ID 与 exact revision grammar 收敛在 domain。

它仍不是 authorization scope，也不包含 tenant/principal。

### 19.4 为什么 graph adapter 不合并进既有 Strategy Repository

两者表面都属于 Lottery/MySQL，但：

- aggregate 不同；
- lifecycle 不同；
- permission surface 不同；
- runtime composition 状态不同；
- graph 当前未装配，Strategy 已被现有 API 消费。

分离后，长期 `growthos_app` 可以继续无 graph 权限，而测试专用 identity 独立验收 adapter。

## 20. Create 路径与失败模型

### 20.1 写前 Validate 必须零 SQL

非法 graph 代表 caller/domain contract violation。若先 Begin 或 insert header 再发现拓扑坏：

- 浪费连接与锁；
- 错误分类被 SQL constraint 偶然决定；
- DB 只知道局部形状，可能暂时接受全局坏图。

所以 `graph.Validate()` 在 transaction 前完成。

### 20.2 写入顺序

```text
header -> canonical nodes -> canonical edges
```

它匹配 FK 依赖。Node/edge statement 重用降低重复 prepare，同时每一行检查 `RowsAffected == 1`。

### 20.3 为什么不是 bulk insert

在最多 128/256 的规模下，逐行 prepared statement 的优点是：

- 失败行位置更可定位；
- 参数映射清晰；
- sqlmock 易验证 canonical order；
- 不拼接动态 SQL；
- 当前容量下 round trips 仍有界。

成本是语句次数更多。若真实性能证据显示 create 成为瓶颈，可以评估 chunked multi-row insert，但必须保持行数校验、参数安全和事务语义。

### 20.4 duplicate identity

header insert 的 MySQL 1062 映射 `AlreadyExists`。只有 root insert 的 identity collision 能安全做该映射；child duplicate/constraint failure 说明实现或存储异常，不应假装 graph 已存在。

### 20.5 child failure rollback

测试通过临时 child constraint 或 driver error 让 node/edge insert 失败。正确结果：

- Create 返回稳定 failure；
- header、nodes、edges 对外均为 0 行；
- 临时 constraint 被清理；
- pool 仍可用。

这验证“transaction 是 aggregate 原子写入边界”，而不是只验证 Go 返回了 error。

### 20.6 commit outcome unknown

commit 报错可能发生在：

- server commit 前；
- server 已提交、response 丢失；
- connection 中断导致 client 不知道结果。

因此不能返回 generic retryable 并自动重试。稳定类 `ErrCommitOutcomeUnknown` 提醒调用者先按 identity read-back。

## 21. Read snapshot：三次查询必须属于同一观察时刻

### 21.1 为什么不是普通 SELECT

一次完整 restore 至少读：

1. header；
2. nodes；
3. edges。

如果观察不同时间，即使每行都合法，组合结果可能不属于任何真实 snapshot。

### 21.2 Repeatable Read

`sql.LevelRepeatableRead` 让同一 transaction 内一致读取同一个数据库 snapshot。它是当前 MySQL 行为与项目 repository 约定的显式表达，不依赖 server default 恰好正确。

### 21.3 ReadOnly

`ReadOnly: true`：

- 告诉 driver/server 这是只读事务；
- 防止未来 refactor 在 Find 中偷偷写 repair marker；
- 可用真实 server 写入反例验收。

sqlmock 不能完整证明 TxOptions 抵达 server，所以 unit source/AST guard 与真实 MySQL probe互补。

### 21.4 unknown schema 提前失败

Header 一旦显示 schema != 1，reader 不应继续查询 nodes/edges：

- 避免用 v1 projection 误扫未来表意；
- 减少不必要 I/O；
- 让错误明确为 stored-invalid/schema unsupported。

### 21.5 commit read transaction

读取完成后显式 commit，而不是只靠 rollback defer，能暴露 transaction end failure。随后才进行纯领域 restore。

## 22. Bounded read：多读一行而不是截断

### 22.1 `LIMIT N+1`

```text
nodes LIMIT 129
edges LIMIT 257
```

如果长度大于 domain max，返回 stored-invalid。

### 22.2 为什么不 `COUNT(*)` 再 SELECT

额外 count 会：

- 增加查询；
- 仍需要同 snapshot；
- count 与实际 scan 重复工作；
- 不能替代 row mapping error。

`N+1` 在已知小上限下直接给出超限证据。

### 22.3 为什么不能只保留前 N 行

截断意味着：

```text
persisted revision content != restored revision content
```

这破坏不可变 revision 的可解释性。超限必须整体失败，而不是最佳努力。

## 23. 严格恢复：数据库行不是领域对象

### 23.1 两阶段恢复

```text
SQL row scan
  -> stored row structs with nullable fields
  -> strict union decoding
  -> domain node/edge constructors
  -> complete RestoreStrategyRoutingGraph
```

### 23.2 为什么 nullable union 要显式检查

合法 decision：

```text
rule_code.Valid = true
strategy_id.Valid = false
```

合法 target：

```text
rule_code.Valid = false
strategy_id.Valid = true
```

即使 DDL 有 CHECK，reader 仍显式验证，因为：

- 历史 schema 可能漂移；
- 特权维护可能绕过；
- 测试 double 可以返回坏行；
- domain 不应相信 adapter 已正确 discriminated。

### 23.3 default marker 只接受 0/1

Go `bool` scan 可能让 driver coercion 掩盖 2 等坏值。先扫 `uint8`，再精确映射 0/1，使 stored corruption 明确失败。

### 23.4 MaxUint64

MySQL `BIGINT UNSIGNED` 覆盖 `0..18446744073709551615`。GraphID、NodeID 和 StrategyID 的 Go 类型都是 `uint64`。

nullable target 必须使用 `sql.Null[uint64]`，真实 scan 测试要让 max 值穿过 driver/sqlx，而不是只在内存构造 wrapper。

### 23.5 自动修复为什么危险

如果 restore 自动：

- 删除 unreachable；
- 断开 cycle；
- 补 baseline；
- 猜 root；
- trim revision；

那么同一数据库 revision 在不同程序版本中会恢复成不同内容。审计、回放和 incident diagnosis 全部失真。坏 graph 应隔离和调查，不应在 read path 中静默修复。

## 24. Repository error taxonomy

### 24.1 需要区分的状态

| class | 含义 | 是否能自动重试 |
| --- | --- | --- |
| invalid argument | caller contract 错 | 否 |
| not configured | composition 错 | 否 |
| graph not found | identity 不存在 | 取决于用例，不是存储故障 |
| already exists | create-only 冲突 | 不能覆盖；可 read-back |
| stored graph invalid | 行存在但无法信任 | 否，应调查 |
| retryable | deadlock/lock timeout 等 | 上层需预算与幂等判断 |
| commit outcome unknown | 可能已提交 | 先 read-back，不能盲重试 |
| repository failure | SQL/permission/schema/scan 等 | 不承诺重试有效 |
| context canceled/deadline | caller 生命周期结束 | 保留 caller 语义 |

### 24.2 为什么错误文本低披露

Repository raw error 可能包含：

- SQL 片段；
- 表/列名；
- connection detail；
- constraint 名；
- graph identity 或业务 revision。

稳定 `Error()` 只输出 class。受信诊断仍可检查 cause，但未来 transport 不能原样序列化。

### 24.3 stored-invalid 为什么不是 not-found

将 corruption 映射 not-found 会隐藏数据完整性事件，并可能诱导创建同 identity。二者必须分开。

## 25. 权限与威胁边界

### 25.1 资产

本节保护的资产包括：

- graph revision/content 绑定；
- Strategy target 引用；
- branch/default 路由意图；
- Migration history；
- 长期 runtime 的最小数据库权限；
- 未来 executor 的可信输入边界。

### 25.2 攻击/故障主体

- 编程错误的内部 caller；
- 错误配置的 adapter；
- 过度授权的数据库账号；
- 手工运维 SQL；
- 历史 bug 留下的坏 row；
- 恶意超大 fixture；
- 未来版本产生的 unknown schema/token；
- 试图枚举 graph 的未授权用户（后续权限章节处理）。

### 25.3 主要威胁与当前控制

| 威胁 | 当前控制 | 剩余风险 |
| --- | --- | --- |
| 同 revision 原地覆盖 | composite PK、无 Update/Upsert | 特权 DBA 可直接改行 |
| 半图可见 | single transaction | commit unknown 需 read-back |
| graph bomb | 128/256/16、LIMIT N+1 | 管理 API rate limit 尚无 |
| cycle/unreachable | Create + Restore validation | validator bug |
| root dangling | 双重 domain validation | DB 无反向 FK，旁路可造坏行 |
| unknown schema 被误读 | exact v1 fail closed | schema v2 升级协议尚无 |
| target 引用不存在 | FK | target 发布/语义版本尚无 |
| runtime 账号扩权 | graph adapter 不装配；长期账号无 graph grants | 后续组合必须重新评审 |
| 测试账号越权 | 隔离 schema、精确三表 SELECT/INSERT、negative probes | 环境配置错误需验收门 |

### 25.4 测试专用 graph identity

真实 MySQL adapter 验收不能用 migrator，因为 migrator 的权限太大，无法证明 Repository 的最小权限需求。

也不能复用 API identity，因为：

- 会给当前 runtime 暗中扩 graph 写权限；
- 测试通过后无法判断是必要权限还是 inherited privilege；
- 与“adapter 未装配”的架构事实冲突。

因此测试 identity 必须：

- 指向 disposable schema；
- 与 migrator/API user 不同；
- exact grants 只有 graph/header/node/edge 的 `SELECT, INSERT`；
- `UPDATE/DELETE` graph 失败；
- old Strategy/Award 与 schema_migrations 读写失败；
- mandatory roles 为空。

Strategy fixtures 由 verification/migration identity seed，graph identity 只引用它们。

### 25.5 长期 `growthos_app`

当前 production-like runtime 没有 graph consumer，所以 `growthos_app` 继续不获得 graph 三表权限。

Migration 创建表不等于 runtime 账号需要访问表。权限应由已装配 use case 推导，而不是由 schema existence 推导。

## 26. 性能与容量推导

### 26.1 Domain validation complexity

在 canonical sort 后：

```text
sort nodes: O(V log V)
sort edges: O(E log E)
maps/shape: O(V + E)
DFS:        O(V + E)
```

V <= 128、E <= 256，使绝对成本有界。

### 26.2 Create I/O model

当前大致为：

```text
1 begin
1 header insert
V node executions
E edge executions
1 commit
```

最坏语句数量在预算内仍可能超过 300。当前选择优先清晰与证据；没有真实创建吞吐需求前不做 bulk optimization。

### 26.3 Find I/O model

```text
1 begin snapshot
1 header select
1 bounded node select
1 bounded edge select
1 commit
1 O(V+E) strict restore after canonical sort
```

查询均使用完整 composite identity，header 应是 const PK lookup，child rows 按 PRIMARY key range/order 获取。EXPLAIN 验收防止无意 filesort/全表扫描，但小 fixture 的 plan 不是生产容量证明。

### 26.4 为什么没有 Redis cache

缓存会立即要求：

- key 是 GraphID 还是 GraphID+Revision；
- schema version 如何编码；
- corrupt cache 怎样处理；
- publish/active revision 失效；
- negative cache；
- cache stampede；
- MySQL 与 cache 真相边界。

当前 graph 未执行、规模有界，也无读取 QPS 证据。先建立正确权威读取，再测瓶颈。

### 26.5 为什么不发布 QPS/P99

Unit test、race test、EXPLAIN 和单个 disposable MySQL round-trip 只能验证功能/形状，不是容量压测。缺少：

- graph 数量分布；
- create/read ratio；
- concurrent workload；
- network latency；
- buffer pool 状态；
- production hardware；
- SLO。

所以本节不能给出伪精确性能结论。

## 27. 可观测性设计：先定义低基数语义

### 27.1 当前可稳定观测的操作

未来 adapter metrics 可以按：

```text
operation = create | find
outcome   = success | not_found | conflict | stored_invalid |
            retryable | commit_unknown | canceled | failure
```

### 27.2 不能做普通 metric label 的字段

- GraphID；
- Revision；
- NodeID；
- StrategyID；
- SQL/constraint；
- 完整 rule path。

这些高基数或敏感字段会造成时序爆炸，也可能泄露业务配置。

### 27.3 日志最小建议

受信日志可以包含：

- request correlation ID；
- operation；
- stable error class；
- elapsed bucket；
- node/edge count bucket；
- schema version；
- 是否 commit outcome unknown。

Graph identity 若确需 incident correlation，应通过受控结构化字段、访问控制和保留期评审，不进入公开 error 文本。

### 27.4 stored-invalid 告警

`stored_invalid` 不是普通用户输入错误。它意味着：

- 特权旁路写入；
- schema/check 漂移；
- adapter restore bug；
- 未知未来 schema 被旧实例读取。

未来应有低频高优先级告警，并附内部 graph identity correlation，但本节不声称已接 metrics/dashboard。

### 27.5 commit unknown 告警

commit unknown 需要：

- 记录 operation correlation；
- 上层停止盲重试；
- read-back identity；
- 区分“已存在且内容一致”与“identity 被其他内容占用”。

当前没有 command/idempotency workflow，所以只冻结错误语义。

## 28. 测试设计：每类测试证伪一个错误信念

### 28.1 Domain constructor tests

证伪：

> “只要 node/edge 各自合法，graph 就合法。”

覆盖 root、duplicate、dangling、branch set、terminal outgoing、unreachable、cycle、budget。

### 28.2 Convergence tests

证伪：

> “再次访问节点就是环。”

两 branch 同 terminal、多 decision 共享后继都应合法。

### 28.3 Shared-successor depth test

证伪：

> “第一次访问距离可以代表最长深度。”

fixture 让浅路径先遍历共享节点，再通过更深父路径引用，最终 depth 必须取最长。

### 28.4 Defensive-copy/race tests

证伪：

> “字段私有就自动不可变。”

slice 底层数组仍可共享，必须修改输入/输出副本并并发读取验证。

### 28.5 Fuzz

证伪：

> “手写反例覆盖了所有拓扑组合。”

Fuzzer 真正把 bytes 映射进 node/edge topology，检查不 panic、不死循环、成功必 Validate、失败返回零 graph。

### 28.6 Migration tests

证伪：

> “SQL 文件看起来对，真实 MySQL 就会按预期约束。”

应在 disposable MySQL 验证 exact v5、列/collation、constraint、FK 列顺序、case sensitivity、cross revision、rollback、root 无反向 FK。

### 28.7 Adapter unit tests

证伪：

> “domain 和 DB 有校验，所以 adapter 只是机械 SQL。”

需要验证 transaction order、RowsAffected、duplicate mapping、commit unknown、read TxOptions callpoint、unknown schema short-circuit、N+1 limit、strict restore、MaxUint scan 与无 mutation SQL surface。

### 28.8 Real repository integration

证伪：

> “sqlmock 通过就证明 driver、privileges、isolation 和 unsigned scan 正确。”

真实测试设计覆盖：

- distinct identities and exact grants；
- round-trip/new revision/duplicate；
- max uint；
- concurrent create；
- child failure rollback；
- dangling logical root fail closed；
- cancellation；
- server-side repeatable-read/read-only；
- EXPLAIN；
- fixture cleanup。

实际执行结果属于 QA 记录，设计文档只冻结为什么需要这些证据。

### 28.9 Negative architecture diff

证伪：

> “新增 schema 时顺便接进 API 不会有影响。”

必须审查：

- `cmd` 无 graph composition；
- existing HTTP route/DTO 无变化；
- Web 无 graph 页面；
- Participation 无 import；
- 没有 evaluator/Activity/permission；
- runtime identity 无 graph grant。

该负证不要求 Compose 文件逐字不变：本节确实将 API/Migration build version 与 image tag 推进到 `lesson-28`，并把 migration/smoke/隔离 acceptance 的 schema checkpoint 推进到 v5。负证要求的是没有新增 graph runtime consumer、公开 endpoint、service/network/secret 拓扑或前端 surface。

## 29. 反事实推演：如果选择另一条路，会发生什么

### 29.1 反事实一：直接让第 27 节 policy revision 当主键

事故路径：

1. caller A 用 `revision=v1` 配 targets 100/200；
2. caller B 也用 `revision=v1` 配 100/300；
3. decision log 只记 `v1`；
4. 事后无法恢复当时完整内容。

GraphID+Revision+create-only storage 解决的是权威绑定，而不是只换一个类型名。

### 29.2 反事实二：JSON 一列且允许 UPDATE

事故路径：

1. Activity 历史只引用 graph ID/revision；
2. 运营修改 JSON target；
3. 同 revision 的历史 replay 得到新结果；
4. 审计证据失真。

create-only 比存储格式更关键；三表再增加局部完整性。

### 29.3 反事实三：`foreign_key_checks=0` 建双向 FK

事故路径：

1. Repository 临时关闭检查；
2. 中间 child insert 失败；
3. cleanup 或会话复用不正确；
4. 其他写入绕过 FK；
5. 重新开启不补验；
6. 文档却声称“有 FK 所以完整”。

这比公开 root FK 缺口更危险，因为它制造虚假安全感。

### 29.4 反事实四：reader 自动删掉不可达节点

事故路径：

1. DB 中存在额外 premium branch 子图；
2. 当前 root 未连到它；
3. reader 静默忽略；
4. 管理工具看到 rows，executor 看不到；
5. 同 revision 出现两种真相。

严格失败让 corruption 可见。

### 29.5 反事实五：给 `growthos_app` 直接授予 graph 写权限

事故路径：

1. 当前 API 没有 graph use case，但账号已有 INSERT；
2. 某个 SQL injection/错误 handler 获得额外破坏面；
3. 未来权限章节前已经可以写 graph；
4. 无法证明谁通过什么 command 创建。

Migration existence 不应自动扩 runtime privilege。

### 29.6 反事实六：本节顺便做 evaluator

事故路径：

1. evaluator 接受尚未冻结的 generic facts；
2. schema 添加 expression/operator；
3. unknown/default/cancellation 语义临时决定；
4. 持久格式先于测试 oracle；
5. 第 27 节 concrete semantics 被新 DSL 改义。

第 29 节独立实现能用本节 validated graph 与第 27 节 router 做等价检查。

## 30. 可演进性：哪些变化需要新 revision，哪些需要新 schema

### 30.1 新 graph Revision

如果 v1 language 不变，仅：

- target Strategy 变化；
- topology 在 v1 node/branch 内变化；
- root/node identity 重新组织；

应创建同 GraphID 的新 Revision。

### 30.2 新 graph schema version

如果引入：

- 新 node kind；
- 新 rule/operator；
- 新 branch vocabulary；
- expression/value type；
- terminal 引用 Strategy revision 而不只是 ID；
- 不同预算/解释协议且破坏兼容；

需要 schema v2 和新 ADR/恢复器，不能让 v1 reader 猜。

### 30.3 新 Migration

物理表/索引/约束变化必须追加 migration v6+。Graph schema v2 不一定等于 Migration v2 或 v6，两条版本轴独立。

### 30.4 Activity 发布

第 30 节需要决定：

- Activity 怎样引用 GraphID+Revision；
- target Strategy 是否也要 immutable revision；
- draft/approved/published/retired；
- 发布时重新 Validate/Find；
- rollback 是切换引用还是改旧内容；
- concurrent publish 冲突。

Graph header 不提前加入这些状态。

### 30.5 权限

第 31～35 节统一模型需要回答：

- Principal；
- resource/action；
- role/permission；
- object/data scope；
- session；
- server-side enforcement；
- frontend projection；
- overreach/E2E。

Graph aggregate 不保存 role 或 menu visibility。权限管理引用 graph resource，不拥有 graph topology。

## 31. 风险账本

| 风险 | 当前决定 | 何时重评 |
| --- | --- | --- |
| root 无反向 FK | 接受并双重校验 | 出现旁路坏 root 或需要更强 DB proof |
| target 只引用 StrategyID | 接受 | Strategy 获得 immutable business revision |
| 逐行 insert | 接受可读性 | 实测 create latency/throughput 不满足 SLO |
| 每次 Find 都 restore | 接受正确性 | profiling 证明校验成为热点 |
| 128/256/16 | 作为安全预算 | 真实配置接近上限 |
| revision 非 content hash | 接受 create-only token | 需要跨环境 promotion、签名、防篡改 |
| exact membership rule only | 接受最小语言 | 新真实 rule 产品批准 |
| 无 delete/archive | 接受不可变历史 | retention/法规/容量提出要求 |
| graph adapter 未装配 | 符合本节 stop line | 第 29/30 节形成真实 consumer |
| runtime 无 graph grants | 最小权限 | composition 与 server authorization 已完成 |
| 无 cache | 没有性能证据 | read workload 和 DB profile 触发 |

风险账本不是待办垃圾桶。每项都要写明当前为何可接受、哪类新证据触发重决策。

## 32. 架构师检查清单

### 32.1 需求与语言

- [ ] 是否有真实多出口/共享后继需求，而不是为了学技术造图？
- [ ] rule、branch、default、terminal 的业务含义是否逐字冻结？
- [ ] unknown/failure 是否明确不能命中 default？
- [ ] 图是否只属于 Lottery Strategy routing？
- [ ] 是否拒绝了无证据的 generic DSL/operator？

### 32.2 Identity 与版本

- [ ] GraphID、Revision、SchemaVersion 是否各自回答不同问题？
- [ ] Revision grammar 是否在 Go/MySQL 对齐？
- [ ] collation 是否大小写敏感？
- [ ] 是否避免 trim/normalize 改写 identity？
- [ ] 是否明确 Revision 不是天然 hash？

### 32.3 Domain invariants

- [ ] root 非零、存在、且是 decision？
- [ ] nodes/edges 是否在 map 分配前受限？
- [ ] node union 是否封闭？
- [ ] 每 decision exact branches/default？
- [ ] terminal 无 outgoing？
- [ ] dangling、duplicate、unreachable 是否拒绝？
- [ ] 三色 DFS 是否区分 visiting/visited？
- [ ] shared successor 是否有正例？
- [ ] longest depth 是否有 shallow-first 反例？
- [ ] input/output collections 是否防御性复制？

### 32.4 Persistence

- [ ] 每条 Migration 是否 forward-only、历史不改？
- [ ] graph/node/edge composite scope 是否完整？
- [ ] Strategy target FK 的保证是否没有被夸大？
- [ ] root 无反向 FK 的原因/风险是否记录？
- [ ] 是否拒绝 `foreign_key_checks=0`？
- [ ] Create 是否完整验证后才有 SQL？
- [ ] 是否一个事务 header -> nodes -> edges？
- [ ] every RowsAffected 是否检查？
- [ ] duplicate 与 child failure 是否分开分类？
- [ ] commit unknown 是否阻止盲重试？

### 32.5 Read/Restore

- [ ] 查询是否使用完整 GraphID+Revision？
- [ ] 是否 repeatable-read + read-only？
- [ ] unknown schema 是否在 child query 前失败？
- [ ] 是否 LIMIT N+1 检测超限？
- [ ] SQL columns 是否显式，不用 `SELECT *`？
- [ ] nullable uint64 是否覆盖真实 max scan？
- [ ] snapshot 结束后是否 strict restore？
- [ ] restore 是否绝不自动修复？
- [ ] corruption 是否返回零 graph + stable stored-invalid？

### 32.6 Security

- [ ] migrator、test graph identity、API identity 是否不同？
- [ ] graph test identity 是否 exact 三表 SELECT/INSERT？
- [ ] UPDATE/DELETE 与旧表/schema_migrations 是否 negative tested？
- [ ] mandatory roles 是否为空？
- [ ] 长期 `growthos_app` 是否仍无 graph grants？
- [ ] 错误文本是否不泄露 SQL/DSN/拓扑？
- [ ] 是否没有把 membership tier 当 role？

### 32.7 Performance/observability

- [ ] O(V+E) 验证与 sort 成本是否在硬预算内？
- [ ] query plan 是否按完整 PK scope？
- [ ] 是否没有在无证据时引入 Redis？
- [ ] 是否没有用 unit/integration test 冒充生产 SLO？
- [ ] metrics labels 是否低基数？
- [ ] stored-invalid/commit-unknown 是否有未来告警语义？

### 32.8 Stop line

- [ ] 没有 evaluator/traversal？
- [ ] 没有 publish/Activity 状态？
- [ ] 没有 HTTP/runtime composition？
- [ ] 没有 UI？
- [ ] 没有 session/RBAC？
- [ ] 没有 Redis/RabbitMQ/PG graph path？
- [ ] 没有正式 Draw/Benefit？

## 33. 本节当前代码证据索引

### 33.1 领域

- [Graph aggregate 与 validator](../../../internal/lottery/domain/strategy_routing_graph.go)
- [Graph error taxonomy](../../../internal/lottery/domain/strategy_routing_graph_errors.go)
- [Graph unit tests](../../../internal/lottery/domain/strategy_routing_graph_test.go)
- [Topology fuzz](../../../internal/lottery/domain/strategy_routing_graph_fuzz_test.go)

### 33.2 Application 与 adapter

- [Repository ports](../../../internal/lottery/application/repository.go)
- [Repository error classes](../../../internal/lottery/application/repository_error.go)
- [Port shape test](../../../internal/lottery/application/strategy_routing_graph_repository_test.go)
- [MySQL graph repository](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository.go)
- [Adapter unit tests](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_test.go)
- [真实 MySQL 验收设计](../../../internal/lottery/adapter/mysqlrepo/strategy_routing_graph_repository_integration_test.go)

### 33.3 Schema

- [000003 graph header](../../../migrations/sql/000003_create_lottery_strategy_routing_graph.up.sql)
- [000004 nodes](../../../migrations/sql/000004_create_lottery_strategy_routing_node.up.sql)
- [000005 edges](../../../migrations/sql/000005_create_lottery_strategy_routing_edge.up.sql)
- [Schema integration](../../../migrations/strategy_routing_graph_schema_integration_test.go)

### 33.4 规范

- [Graph 产品基线](../../product/lottery-strategy-routing-graph-v1.md)
- [ADR-0024](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)
- [第 28 节 API 零变化记录](../../api/lessons/lesson-28.md)
- [第 28 节 QA：已执行证据与 final-freeze pending](../../qa/lessons/lesson-28.md)
- [第 28 节面试问答](../../interview/lessons/lesson-28.md)

## 34. 可准确声称与禁止声称

### 34.1 可以准确声称

> Lesson 28 为 Lottery 会员 Strategy 路由建立了封闭 schema v1 的有界 immutable rooted DAG，用 GraphID+Revision 形成 create-only 内容身份；三表提供 scoped PK/FK/CHECK，domain 在 Create/Restore 两端验证唯一 decision root、exact branches、全可达、无环、共享后继、terminal 终止及 128/256/16 预算；未装配 MySQL adapter 用单事务写入、只读 RR snapshot、N+1 bounded read、严格 nullable union/MaxUint64 恢复和低披露错误阻止半图与损坏数据进入未来执行层。

### 34.2 不能声称

- 通用规则平台/DSL 已完成；
- graph 已被执行或发布；
- Activity 已引用 revision；
- Strategy target 是 immutable published version；
- 当前公开 API 可创建/查询 graph；
- 前端有规则管理页面；
- session/RBAC/data scope 已存在；
- 长期 runtime 账号拥有 graph 权限；
- MySQL FK 单独证明 rooted DAG；
- 测试已经证明生产性能或可用性。

## 35. 下一节的输入契约

第 29 节 executor 应只接受：

- `Validate()` 成功的 immutable graph；
- exact schema v1；
- 受控、类型化的会员事实；
- 受控 clock/cancellation/预算；
- 第 27 节 concrete router 作为语义 oracle。

它不应接受：

- 裸 SQL rows；
- 未知 schema；
- `map[string]any`；
- 客户端提交的 membership tier；
- 自动修复后的 partial graph；
- “latest revision”隐式查询；
- Activity/publish 状态的临时替代。

第 29 节的核心问题是执行正确性；第 30 节是发布生命周期；第 31～35 节是统一权限。保持线性演进，才能让每一层都拥有自己的失败模型和验收证据。

## 36. 最终第一性结论

本节最重要的架构结论有十条：

1. **持久化之前先有真实业务语言。** 第 27 节的 concrete branch 是 v1 schema 的来源。
2. **名称限制野心。** `StrategyRoutingGraph` 比 `RuleEngine` 更准确地限定所有权。
3. **aggregate 完整性跨越所有 node/edge。** 单行合法不能推出图合法。
4. **DAG 与 tree 的差别来自真实合流。** 三色 DFS 必须把共享后继和环分开。
5. **资源预算也是安全契约。** 128/256/16 先于无界分配和递归。
6. **数据库与领域各证明自己擅长的事实。** FK/CHECK 不取代拓扑校验，拓扑校验也不取代 FK。
7. **事务原子性不等于 deferred FK。** root 无反向 FK 是诚实记录的权衡，不用关闭检查制造虚假完整性。
8. **恢复是重新建立信任。** SQL scan 成功不等于领域对象合法，strict restore 不自动修复。
9. **权限由已装配能力推导。** 测试专用 graph identity 的精确 grants 不能扩张长期 `growthos_app`。
10. **schema、执行、发布、授权必须分节演进。** 一次只解决一个已经有证据的问题，才能让 Git 历史真正成为可学习的架构推导过程。

一个成熟的架构设计，不是把所有未来技术一次装进系统，而是清楚知道：现在可以证明什么、数据库能保证什么、还有什么残余风险、哪条新证据才值得触发下一次演进。
