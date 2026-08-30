# Lottery Strategy Routing Graph 基线 v1

- **状态：** 第 28 节已批准实现基线
- **日期：** 2026-08-30
- **所有者：** Lottery bounded context
- **适用范围：** `StrategyRoutingGraph` 的领域结构、consumer-owned Repository port、未装配的 MySQL graph repository adapter、三表持久化、create-only 写入与恢复校验
- **不适用范围：** 图执行、规则发布、Activity、公开 API、会员 provider/HTTP/runtime adapter、composition 装配、UI、认证、授权与正式 Draw

## 1. 为什么现在需要把一跳路由保存成图

第 27 节已经用具体代码证明一条线性资格链无法准确表达两个合法成功出口：

```text
confirmed premium
  -> premium_override
  -> premium Strategy target

confirmed standard
  -> baseline_default
  -> baseline Strategy target
```

当时 `MembershipStrategyRoutingPolicy` 是调用方传入的不可变值。它足以验证分支语义，却留下一个已经被记录的缺口：revision 只是一个有界关联 token，同一个字符串仍可能被不同调用方配成不同 targets。只要配置还没有进入权威 registry，就不能通过 `(policy revision)` 唯一恢复当时的完整路由内容，也无法在数据库边界证明 root、edge、terminal 与 target 引用完整。

第 28 节只补上这一层事实：Lottery 可以把一份完整、合法、不可变的 Strategy 路由拓扑以 `(GraphID, Revision)` 保存并恢复；任何由数据库恢复的内容都必须重新通过同一领域校验。它不执行图，也不把数据库记录自动称为“已发布规则”。

## 2. 本节新增的产品能力

本节形成一个 Lottery 专属的 `StrategyRoutingGraph`，它具备以下能力：

1. 用非零 `GraphID` 标识一条长期路由配置家族；
2. 用独立、规范的 `Revision` 标识该家族的一份不可变内容；
3. 用 `schema_version = 1` 声明持久化内容的解释协议；
4. 保存一个显式 root、一个或多个 decision node、一个或多个 `strategy_target` terminal node，以及有向 edge；
5. 允许多个 branch 指向相同 terminal，也允许多个 decision 共享合法后继，所以结构是 rooted DAG，不谎称必须是数学意义上的树；
6. 在创建和恢复时验证唯一 root、全可达、无环、无悬空、terminal 无出边、decision 分支完备以及资源上限；
7. 用 create-only 语义把同一 `(GraphID, Revision)` 永久绑定到同一份内容，拒绝原地更新或覆盖。

“保存成功”只表示这份配置在 v1 schema 下结构合法、目标引用在写入时存在。它不表示图已经发布、正在被 Activity 使用、可以执行、调用者有权限、用户满足资格，或已经形成正式抽奖结果。

## 3. 领域名称与所有权

### 3.1 为什么叫 `StrategyRoutingGraph`

这个名字同时限定了三个边界：

- `Strategy`：terminal 只引用 Lottery `StrategyID`，不路由任意 URL、handler、脚本或跨上下文对象；
- `Routing`：图只描述“选择哪个 Strategy target”，不承担 Participation eligibility、Authorization、Inventory 或 Benefit delivery；
- `Graph`：允许共享后继和分支合流，不能用“tree”掩盖多父节点事实。

课程仍沿用“规则树第一次数据库升级”作为学习标题，因为这是从线性 chain 进入显式 node/edge 的第一次升级；生产领域语言以真实结构 `StrategyRoutingGraph` 为准。

### 3.2 所有权矩阵

| 概念 | 权威所有者 | `StrategyRoutingGraph` 可以保存 | 不得保存或宣布 |
| --- | --- | --- | --- |
| 会员等级事实 | 外部会员 authority | 不保存主体事实；decision 只引用稳定 rule code | tier 快照、姓名、订单、角色、风控画像 |
| 会员到 Strategy 的路由决定 | Lottery | rule、branch、default 与 target 拓扑 | 外部 authority 的写模型 |
| Strategy 聚合 | Lottery Strategy | terminal 的非零 `StrategyID` 引用 | Strategy 内容副本、虚构 Strategy version |
| Participation 资格 | Participation | 不保存 | eligible/reject、次数、风险决定 |
| Activity 生命周期与绑定 | Marketing（第 30 节） | 本节不保存 | draft/published/retired、活动时间窗、启用 revision |
| 访问控制 | Governance（第 31～35 节） | 本节不保存 | principal、role、permission、scope、menu visibility |
| 图执行与 path | 第 29 节 Lottery application/domain | 本节只提供已验证拓扑 | 当前请求事实、运行 path、执行耗时或结果 |

会员等级不是角色，route 成功不是 eligibility，schema 合法也不是 authorization。

## 4. v1 的精确拓扑语言

### 4.1 节点种类是封闭集合

`schema_version = 1` 只接受两种节点：

| kind | 必要内容 | 禁止内容 | 出边语义 |
| --- | --- | --- | --- |
| `decision` | 稳定 `rule_code` | `StrategyID` target | 必须按 v1 rule 形成完整确定分支 |
| `strategy_target` | 非零 `StrategyID` | rule code | terminal，出度必须为 0 |

v1 没有 expression、script、HTTP call、subgraph、delay、parallel、random、inventory、authorization 或任意 `map[string]any` 节点。新增 kind 必须升级 schema 并提供新恢复器，不能让旧代码把未知 kind 当 decision 或 terminal。

### 4.2 v1 只认识第 27 节的真实 rule

每个 `decision` 的 rule code 必须精确为：

```text
lottery.membership_tier.route_strategy
```

它必须且只能拥有以下两条出边：

| branch code | `is_default` | v1 产品意义 |
| --- | --- | --- |
| `premium_override` | `false` | confirmed `premium` 的显式 override |
| `baseline_default` | `true` | confirmed `standard` 的批准基线边 |

两条边可以指向不同 target，也可以在受控迁移/合流场景中指向同一后继。branch 身份仍不可丢失，因为“为什么到达这里”不能从最终 target 反推。

v1 edge 不保存任意 condition、优先级、表达式或输入值 bag。`premium_override` / `baseline_default` 的解释属于稳定 rule 协议，不由数据库中的自由文本重新定义。这样可以持久化真实拓扑，而不会偷偷交付一个没有安全边界的 DSL。

### 4.3 default 仍不吞 unknown

`baseline_default` 是图结构上的 required default edge，但它不是技术故障 fallback，也不是所有未识别输入的 catch-all。

- confirmed `standard` 可以按第 27 节产品语义选择它；
- unknown/unsupported tier 必须在 rule 输入校验中失败关闭；
- fact not found、stale、future、corrupt 或 provider unavailable 不能进入图；
- caller cancellation 不能换路到 default；
- 第 28 节不执行图，因此不声称上述运行语义已经由持久化层交付。

第 29 节执行器必须用第 27 节 concrete router 的 fixture 作为语义 oracle，不能把“没有命中 edge”重新解释为自动 default。

## 5. 身份、revision 与 schema version

### 5.1 `GraphID`

`StrategyRoutingGraphID` 是非零 `uint64`，在 MySQL 中映射为 `BIGINT UNSIGNED`。它标识一条可随 revision 演进的 Lottery 路由配置家族，不是 Activity ID、Strategy ID、数据库自增行号或租户 ID。

本节不定义 ID 生成服务，也不使用 `AUTO_INCREMENT`。调用方必须提供非零 ID；何时分配、谁有权创建由后续管理用例和权限章节决定。

### 5.2 `Revision`

`StrategyRoutingGraphRevision` 是与 GraphID 组合使用的内容身份 token：

```text
^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$
```

因此它：

- 长度为 1～128 ASCII bytes；
- 首字符只能是 ASCII 字母或数字；
- 后续只允许 ASCII 字母、数字、`.`、`_`、`:`、`-`；
- 大小写敏感；
- 不接受首尾空白，也不在构造器中自动 trim；
- 不复用第 27 节允许可打印 Unicode 的 policy revision 契约。

revision 可以由受控发布流程选择，但本节不规定它必须是内容哈希、时间戳或 Git SHA。唯一能成立的保证是：一次成功 create 后，数据库中的 `(GraphID, Revision)` 只对应这一份内容；同键重试必须得到“已存在/冲突”，不能覆盖原内容。

### 5.3 `schema_version`

`StrategyRoutingGraphSchemaVersionV1 = 1` 只表示 node/edge 字段和验证协议的格式版本。它不是：

- graph content revision；
- Strategy version；
- Activity version；
- Migration version；
- 应用版本；
- 会员事实 revision。

`NewStrategyRoutingGraph` 创建 v1；`RestoreStrategyRoutingGraph` 显式接收数据库中的 schema version，并对零值、未知值或未来值失败关闭。旧程序不能“尽力读取”新 schema。

### 5.4 `NodeID`

`StrategyRoutingNodeID` 是非零 `uint64`，唯一范围为单个 `(GraphID, Revision)`。同一个 node number 可以出现在不同 graph/revision 中；edge 必须携带完整 graph/revision scope，不能只凭 node ID 跨图连接。

## 6. rooted DAG 的完整不变量

一份合法 v1 graph 必须同时满足以下条件；只满足其中一部分不能创建或恢复 aggregate。

### 6.1 身份和内容

1. GraphID 非零；
2. revision 满足精确 ASCII grammar；
3. schema version 精确为 1；
4. node 集合非空且不超过 128；由于 root 必须是 decision 且路径必须到 terminal，合法 v1 图实际至少有 2 个 node；
5. edge 不超过 256；由于每个 decision 恰有两条边，合法 v1 图实际至少有 2 条 edge；
6. 每个 node ID 非零且在 graph 内唯一；
7. edge 的 `(from, branch)` 在同一 graph 内唯一；
8. 不接受重复 node 或重复 edge 后“最后一个覆盖前一个”。

### 6.2 root 与可达性

1. graph header 恰好指定一个非零 root node ID；
2. root 必须存在于本 revision 的 node 集合；
3. root 必须是 `decision`；
4. 从 root 出发必须能到达每一个 node；
5. 因此不允许孤儿节点、未被使用的 terminal 或隐藏的第二个子图。

“唯一 root”由 graph aggregate 的单值 root identity 表达，不靠“找唯一入度为 0 的节点”猜测。全可达和无环成立后，任何非 root 节点都必须处于 root 的实际后继闭包中。

### 6.3 edge 引用、方向与节点形状

1. 每条 edge 的 source 与 target 都必须存在于同一 `(GraphID, Revision)`；
2. 不允许跨 graph 或跨 revision edge；
3. edge source 必须是 `decision`；
4. `strategy_target` 出度必须为 0；
5. decision 必须精确拥有 `premium_override` 与 `baseline_default` 两条出边；
6. 每个 decision 恰好一条 `is_default = true`，且只能是 `baseline_default`；
7. `premium_override` 必须是非 default；
8. terminal 必须携带非零 StrategyID，decision 不得携带 target；
9. decision 必须携带精确 v1 rule code，terminal 不得携带 rule code。

数据库的行级 CHECK、UNIQUE 与 FK 会尽量拒绝局部坏形状，但“每个 decision 恰有两条边”“全图全可达”仍需要领域校验，不能用一句“数据库有约束”冒充已经证明。

### 6.4 无环和终止

1. 从 root 开始的有向遍历不得再次遇到当前递归路径中的节点；已经完成的节点可以被其他合法父节点共享；
2. 禁止 self-loop；
3. 禁止两节点或多节点环；
4. 共享后继合法，不得因第二条父边误报 cycle；
5. 有限 DAG 中每条路径最终必须到达 `strategy_target`；
6. 决策节点不能成为没有出边的隐式 reject/unknown terminal。

实现可以使用 DFS colour、Kahn topological sort 或等价有界算法；对外契约只关心确定结果与稳定错误边界，不冻结内部算法。

### 6.5 深度、节点和边预算

| 预算 | v1 上限 | 精确定义 |
| --- | --- | --- |
| nodes | 128 | 一个 revision 中 decision + target 的总数 |
| edges | 256 | 一个 revision 中所有有向 edge 总数的安全硬上限；exact v1 两分支与至少一个 terminal 还会形成更紧的可达组合上限 |
| depth | 16 | root 到任一 terminal 的最长路径所含 edge 数 |

root 自身深度为 0；一个 decision 直接指向 terminal 的图深度为 1。深度 16 合法，17 失败。上限在分配大 map/slice 或递归深入前尽早检查，以降低损坏数据库行或恶意 fixture 造成的 CPU/内存放大。

这些是 schema v1 的安全预算，不是线上 QPS、延迟 SLO 或第 29 节执行步数/时间预算。修改上限可能改变兼容性，必须通过新决策与测试，不能只改一个常量。

## 7. 最小持久化形状

v1 使用三张 Lottery-owned InnoDB 表：

```text
lottery_strategy_routing_graph
  PK (graph_id, revision)
  schema_version
  root_node_id          -- aggregate scalar; no reverse FK to node table
  created_at

lottery_strategy_routing_node
  PK (graph_id, revision, node_id)
  FK (graph_id, revision) -> graph
  node_kind
  rule_code             -- decision only
  strategy_id           -- strategy_target only
  created_at

lottery_strategy_routing_edge
  PK/unique scoped by graph revision and source branch
  FK (graph_id, revision) -> graph
  FK source -> node
  FK target -> node
  branch_code
  is_default
  created_at
```

精确 SQL 列、约束和索引以第 28 节不可变 Migration 为准；本产品基线冻结的是语义，不鼓励 Repository 依赖列序或 `SELECT *`。

### 7.1 为什么不是 graph 行一个 JSON

JSON 单列可以一次读取，但会让以下事实失去数据库可见性：

- node/edge scoped identity；
- source/target 外键；
- terminal Strategy 引用；
- node kind 与字段互斥；
- branch 唯一性；
- 精确索引和损坏 fixture。

应用层仍要做全图校验，不代表数据库应该放弃能可靠表达的局部约束。三表让“数据库能证明什么”和“恢复器必须证明什么”可以被分别测试。

### 7.2 root 反向 FK 为什么刻意不建

直觉上可以建立两条引用：

```text
node.(graph_id, revision) -> graph.(graph_id, revision)
graph.(graph_id, revision, root_node_id) -> node.(graph_id, revision, node_id)
```

但这会产生插入环：node 要求 graph 已存在，graph 又要求 root node 已存在。InnoDB 外键按语句/行立即检查，不支持把该循环引用延迟到事务 commit；`NO ACTION` 对 InnoDB 等同于立即 `RESTRICT`。因此即使把两次 INSERT 放在一个事务里，也没有合法的第一条 INSERT。

v1 的选择是：

- 保留 graph header 中非空 `root_node_id`，因为 root 是 aggregate identity 的一部分；
- 保留 node 到 graph、edge 到两端 node 的正向 composite FK；edge 的完整 scope 随两端 node 一并受约束；
- 不创建 header 到 node 的反向 FK；
- create 前在领域内验证 root 存在且为 decision；
- 按 `graph -> nodes -> edges` 在一个事务中写入；
- commit 前不对外暴露未完成 aggregate；
- 每次恢复都重新验证 root、全可达、无环和全部图不变量。

不得通过 `SET foreign_key_checks = 0` 绕过问题。MySQL 重新打开该变量不会自动扫描并补验绕过期间写入的数据，因此它不是完整性方案。

替代方案 `node.is_root` 可以避免反向引用，但会把 aggregate header 的 root identity 分散到子行，并仍需要领域层证明“至少一个 root”；nullable root 后 UPDATE 会把 create-only revision 变成分阶段可变状态；先插占位 node 会制造无业务意义记录。这些方案在 v1 都不采用。

## 8. 数据库与领域分别保证什么

| 不变量 | MySQL 局部约束 | 创建/恢复领域校验 |
| --- | --- | --- |
| GraphID/NodeID 非零 | unsigned + CHECK | 再验证 |
| revision grammar | binary collation、长度及可表达的 CHECK | 精确 ASCII grammar |
| `(GraphID, Revision)` 唯一 | composite PK | create-only 冲突语义 |
| node 属于 graph revision | composite FK | 再验证 scope |
| edge 两端存在 | composite FK | 再验证并建立 adjacency |
| target Strategy 存在 | terminal target FK/受控引用约束 | 非零与 node shape |
| node kind 与字段互斥 | row CHECK | 构造器/restore 再验证 |
| branch/default 字面值 | CHECK/UNIQUE 可覆盖局部集合 | 每个 decision 完整分支集合 |
| root 存在且为 decision | 无反向 FK | 必须验证 |
| 无环、全可达、最大深度 | 单行约束做不到 | 必须验证 |
| node/edge 总数上限 | 表本身不按 aggregate 计数 | 读取有界 + 必须验证 |
| published / Activity 可用 | 本节无该状态 | 本节不宣称 |

Strategy 外键只能证明写入时有该 ID 的父行，不能证明 Strategy 已发布、内容不可变、具有独立业务 version，或未来删除/更新策略已经完整。第 30 节 Activity 发布边界必须继续补足版本化引用。

## 9. create-only 写入契约

一次 create 必须遵守以下顺序：

1. 从完全构造的 graph value 开始，先在内存中执行完整 `Validate()`；
2. graph 非法时零 SQL；
3. 开启一个数据库事务；
4. 插入 graph header；
5. 按 canonical node ID 顺序插入全部 nodes；
6. 按 canonical edge order 插入全部 edges；
7. 检查期望写入行数；
8. 事务内或 commit 前保持同一完整内容，不执行局部修补；
9. commit 成功后才宣布 create 成功；
10. 任一步失败 rollback，不能留下可读取的半图。

Repository 不提供 `Update`、`Upsert`、`Replace`、`Save`、原地改 target 或删 edge 方法。相同 GraphID 的新内容必须使用新 Revision；相同 `(GraphID, Revision)` 再次 create 返回稳定冲突/已存在错误，即使 payload 恰好相同也不能暗中覆盖。

本节没有幂等请求 key 或发布命令，因此 create 冲突不自动等于“前一次一定成功”。调用方若遇到提交结果不确定，只能通过 `(GraphID, Revision)` 查询并比较受控内容；不能再次生成同 revision 的不同图。

## 10. 恢复与损坏数据处理

恢复不是“把三张表 scan 到公开 struct”即可。一次 Find/Restore 至少需要：

1. 使用完整 `(GraphID, Revision)` 查询 header；
2. 未找到返回稳定 not-found，不返回零 aggregate 加 nil error；
3. 读取 header 的显式 schema version；
4. 对未知 schema 立即失败关闭，不降级成 v1；
5. 有界读取 nodes，超过 128 时停止并返回 corrupt/invalid；
6. 有界读取 edges，超过 256 时停止并返回 corrupt/invalid；
7. 严格映射 node kind、rule、branch、default 和 target；
8. 拒绝 NULL/未知枚举、重复 node/edge 或 graph scope mismatch；
9. 调用 `RestoreStrategyRoutingGraph` 重新执行完整不变量；
10. 只有恢复后的 immutable aggregate 合法时才返回成功。

恢复不得：

- 删除不可达节点后继续；
- 选择第一个 root 猜测；
- 断开形成环的 edge；
- 为缺失 default 自动补边；
- 将未知 node kind 当 terminal；
- 将未知 rule/branch 当成 v1 的 default；
- trim 或重写 revision；
- 忽略多余行以凑上限；
- 加载 Strategy 内容后掩盖坏 graph。

任何坏行都表示无法形成可信 graph，返回稳定、低披露的恢复失败；原始 SQL、表内容、完整拓扑或连接信息不能进入公共错误文本。

## 11. 不可变性与调用方防御

合法 graph 创建后：

- graph/node/edge 字段不通过公开 setter 修改；
- nodes/edges 访问器返回副本，而不是内部 slice/map；
- 返回顺序 canonical，使测试、比较和持久化不依赖 Go map iteration；
- `Validate()` 对零值和被同包损坏的 value 仍失败；
- concurrent read 不共享请求级可变状态；
- graph 本身不持有数据库连接、context、Clock、事实 reader 或执行缓存。

不可变指“同一 `(GraphID, Revision)` 的领域内容不能原地改写”，不表示 Go value 在物理内存上拥有防篡改签名，也不表示数据库管理员无法绕过应用权限。最小数据库账号、审计和后续管理授权仍是独立控制。

## 12. 验收矩阵

### 12.1 合法图

- 第 27 节一 decision、两 target、两 edge 图可创建、保存、恢复；
- 两条 branch 可以汇聚到同一 terminal；
- 多个 decision 可以共享后继而不会被误判为 cycle；
- node/edge 输入乱序时恢复结果仍 canonical；
- depth 1 与 depth 16 合法；
- node/edge count 在构造完整 adjacency 前先受 128/256 安全硬上限约束；v1 exact 两分支和 terminal 形状可能形成更紧的实际组合上限；
- create 后调用方修改输入/返回 slice 不改变 aggregate。

### 12.2 非法结构

- zero GraphID/NodeID、坏 revision、schema 0/unknown 拒绝；
- root 缺失、root 指向 terminal、悬空 edge、跨 scope edge 拒绝；
- duplicate node、duplicate source branch、duplicate default 拒绝；
- decision 缺 premium/default 任一边或多出第三边拒绝；
- default 字面值与 `is_default` 不一致拒绝；
- terminal 带出边或 decision 带 target 拒绝；
- self-loop、双节点环、多节点环拒绝；
- 不可达 decision/terminal 拒绝；
- 129 nodes、257 edges 或 depth 17 拒绝；
- 未知 node kind/rule/branch 不被 default 吞掉。

### 12.3 持久化与事务

- 三条前向 Migration 分别创建 graph/node/edge 表；
- node -> graph 与 edge -> source/target node 的 composite FK 拒绝悬空 scope 或节点；
- graph root 列无反向 FK，并有测试/文档证明原因；
- write order 为 header、nodes、edges，错误时整事务 rollback；
- 同 `(GraphID, Revision)` 第二次 create 稳定冲突，不发生 UPDATE；
- 恢复时超过预算及坏 row fail closed；
- MySQL round-trip 与 domain fixture 等价；
- Migration 历史不可改写，新增变更只能前向追加。

### 12.4 架构停止线

- 不新增 graph executor、operator registry、generic `Rule` / `Engine`；
- 不把 StrategyRoutingGraph 放入 Participation、Governance 或 Marketing；
- 不新增 Activity/status/published/active revision 字段；
- 不新增 HTTP route、DTO、runtime config、Compose 服务或 React 页面；
- 不接 Redis、RabbitMQ 或 PG；
- 除本节未装配的 MySQL graph repository adapter 外，不新增会员 provider、HTTP/runtime adapter 或 composition 装配，也不实现 UI 权限或浏览器 E2E；
- 现有会员 concrete router、Strategy repository、selector 与 ephemeral API 行为不变。

## 13. 威胁、性能、隐私与可观测边界

| 风险 | 本节控制 | 尚未解决 |
| --- | --- | --- |
| 超大图耗尽内存/CPU | 128/256/16 硬上限；读取有界；验证前限量 | 线上管理 API rate limit |
| 环或不可达节点制造执行异常 | create/restore 均验证 | 第 29 节执行预算 |
| revision 被原地覆盖 | composite PK + create-only | 发布审批、操作者授权 |
| root 反向引用无 FK | 写前完整验证、单事务、恢复再验证 | 绕过 Repository 的特权写入 |
| 未知 schema 被旧程序误读 | exact v1 gate，未知失败关闭 | schema upgrade/migration protocol |
| 图泄露会员策略 | 不保存主体或事实 payload；无公开 API | 后续运营查询的数据范围与审计 |
| 高基数 label | GraphID/revision/node/StrategyID 不进入普通 metric label | 第 29/30 节正式 observability |
| target 被删除或改义 | FK 可阻止部分破坏 | Strategy 业务 version/发布语义 |

本节性能验收只证明验证算法在硬上限内终止、repository 采用有限行数和固定次数级别查询；不发布生产 QPS、P95/P99 或数据库容量 SLO。日志只记录稳定 operation/error class 与 request correlation；不得打印整图、revision 对应的业务内容、SQL、凭证或会员派生信息。

## 14. 本节停止线

### 本节现在做

- Lottery-owned `StrategyRoutingGraph` immutable aggregate；
- GraphID + Revision + schema v1；
- decision / strategy_target 与现有 rule/branches/default；
- 唯一 root、全可达、无环、无悬空、terminal 无出边；
- 128 nodes、256 edges、depth 16；
- graph/node/edge 三表；
- create-only transaction 与严格 restore validation；
- domain/unit/fuzz/race、Migration/MySQL integration 与 architecture negative evidence。

### 第 29 节再做

- 只执行已经恢复并验证的 graph；
- 受控事实输入与 operator dispatch；
- 确定 traversal、多步 path；
- step/depth/time/cancellation budget；
- unknown operator、事实失败和执行失败语义；
- 与第 27 节 concrete router 的等价 fixture。

### 第 30 节再做

- Activity lifecycle；
- draft/approve/publish/retire/rollback 的真实边界；
- Activity 对不可变 graph revision 与 Strategy 配置的引用；
- published revision 的并发与历史解释。

### 第 31～35 节再做

- Principal、role、permission、resource、action 与 data scope；
- 真实 session；
- 服务端 RBAC 强制；
- 前端导航/路由/操作能力投影；
- 越权与浏览器端到端验收。

## 15. 当前可以与不能宣称的能力

可以准确表述：

> 为 Lottery 会员 Strategy 路由建立有界不可变 rooted DAG，以 GraphID/Revision/schema v1、decision/strategy_target、显式 branch/default 和 Strategy 引用形成三表持久化；在 create 与 restore 两个入口验证唯一 root、全可达、无环、无悬空、terminal 终止以及 128/256/16 资源边界，并用 create-only 事务绑定 revision 内容。

不能表述：

- 已实现通用规则树平台、规则引擎、DSL 或 DMN；
- graph 已发布或被 Activity 使用；
- 已执行多步决策或生成持久 path；
- 已接入真实会员 adapter；
- 已有管理 API、运营页面、审批、灰度或回滚；
- 已实现认证、RBAC、数据范围或前端权限；
- 已形成正式 Draw/Result、库存预占或权益发放；
- MySQL 外键已经证明全部图不变量；
- 单元/集成测试等于生产性能与可用性证据。

## 16. 相关基线

- [Lottery 会员等级 Strategy 路由基线 v1](membership-strategy-routing-v1.md)
- [Lottery 业务规则需求基线 v1](lottery-rule-requirements-v1.md)
- [ADR-0019：Lottery 规则所有权与评估边界](../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [ADR-0023：会员等级 Strategy 路由边界](../decisions/ADR-0023-membership-strategy-routing-boundary.md)
- [ADR-0024：Lottery Strategy Routing Graph 持久化](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)
- [MySQL 8.4：FOREIGN KEY Constraint Differences](https://dev.mysql.com/doc/refman/8.4/en/ansi-diff-foreign-keys.html)
- [MySQL 8.4：FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)
- [MySQL 8.4：CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)
- [MySQL 8.4：Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)
