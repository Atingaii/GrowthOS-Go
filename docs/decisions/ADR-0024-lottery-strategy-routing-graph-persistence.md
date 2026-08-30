# ADR-0024：以有界不可变 rooted DAG 持久化 Lottery Strategy 路由

- **状态：** 已接受
- **日期：** 2026-08-30
- **负责人：** GrowthOS 架构组
- **适用范围：** 第 28 节“规则树第一次数据库升级”
- **需求基线：** [Lottery Strategy Routing Graph 基线 v1](../product/lottery-strategy-routing-graph-v1.md)
- **替代关系：** 不替代 ADR-0014、ADR-0019 或 ADR-0023；在其 Strategy、规则所有权和具体会员路由边界内新增持久化 graph aggregate

## 背景

ADR-0023 已用具体会员路由证明线性资格链的边界：confirmed `premium` 选择 `premium_override`，confirmed `standard` 选择 `baseline_default`，两条成功边各自到达一个 Lottery Strategy target；unknown、unsupported 或事实依赖失败都不能命中 default。

第 27 节刻意没有数据库。`MembershipStrategyRoutingPolicy` 的 revision 只是调用方携带的 bounded token，不是 registry 约束的内容身份。只要同一个 revision 字符串仍能与不同 targets 组合，就无法可靠地通过 identity 恢复历史配置，也无法在加载阶段证明：

- root 是否存在且唯一；
- edge 是否引用本 graph revision 的 node；
- terminal 是否真的终止；
- 是否有环、不可达节点或过深路径；
- 每个 decision 的 premium/default 是否完整且确定；
- schema 是否被当前程序理解；
- 同一 revision 是否被原地覆盖。

第 28 节出现的真实新需求不是“执行任意规则”，而是把已经存在的 branch/default/target 拓扑保存成一份可恢复、可验证、不可变的 Lottery 配置。执行、发布、Activity 和权限仍需要各自后续证据。

## 决策驱动

1. graph 必须使用 Lottery 领域语言，不能吞并 Participation、Governance、Inventory 或 Marketing；
2. 持久结构必须来自第 27 节现有 rule/branch/default/target，而不是从通用引擎类图倒推；
3. revision 必须在权威持久化边界绑定唯一内容，不能继续只是任意调用方约定；
4. schema version、content revision、Strategy identity、Activity version 和应用版本必须分离；
5. 任何由数据库恢复的内容都必须通过领域校验，不能把“SQL 可 scan”视为合法 graph；
6. 图必须有显式、确定、有限的结构上限，损坏配置不能制造无界遍历；
7. 数据库应承担可可靠表达的局部约束，但不能伪造跨行 graph invariant；
8. MySQL/InnoDB 外键立即检查，不支持把 graph header 与 root node 的循环引用延迟到 commit；
9. 写入要么形成完整 revision，要么完全回滚；不能先暴露半图再补边；
10. 第 28 节不能因为有 node/edge 表就宣称已有执行器、发布流或运营平台。

## 评估过的方案

### 方案一：继续只保留 concrete policy value，不持久化

| 优点 | 代价 / 风险 |
| --- | --- |
| 代码最少；第 27 节语义已测试 | 相同 revision 可绑定不同 targets |
| 没有 schema/migration 成本 | 无法按稳定身份恢复完整配置 |
| 不会过早抽象 | 新增多 decision/shared successor 时继续堆 nested switch |

**结论：拒绝作为第 28 节终态。** Concrete router 继续保留为语义 oracle，但已经出现了 revision/content binding 与持久恢复需求。

### 方案二：把整张图作为 JSON 存在 graph 单行

| 优点 | 代价 / 风险 |
| --- | --- |
| 一次 INSERT/SELECT；root 与子结构天然同一 payload | node/edge scoped identity 和引用完整性对数据库不可见 |
| schema 可在应用层灵活演进 | branch/default/target shape 只能靠解析后发现 |
| 避免三表 join | 大 JSON 更新易变成原地覆盖，坏 fixture 和索引边界不透明 |

**结论：拒绝。** 应用层无论如何都要验证全图，但不能因此放弃 MySQL 对局部 FK、CHECK、UNIQUE 和 target reference 的可靠保护。

### 方案三：采用通用 `RuleNode` + JSON condition/DSL

| 优点 | 代价 / 风险 |
| --- | --- |
| 表面支持任意规则、运营配置与未来扩展 | 未定义 operator、类型系统、沙箱、预算、兼容、审批和错误语义 |
| node schema 看似稳定 | `map[string]any` 会混入会员画像、权限、库存与跨上下文事实 |
| 可以直接为第 29 节执行 | schema 先于真实 rule，未知表达式可能在生产被“尽力执行” |

**结论：拒绝。** v1 只接受现有会员 rule、两种 node kind 和两条 branch；新增规则必须有新业务证据和兼容决定。

### 方案四：把结构建成严格 tree，禁止共享后继

| 优点 | 代价 / 风险 |
| --- | --- |
| 每个非 root 节点只有一个父节点，概念简单 | 第 27 节已证明两个 branch 可以合法汇聚到同一 target |
| path 可由 parent 回溯 | 共享 terminal 必须复制 node，identity 和引用漂移 |
| cycle 检查略简单 | “规则树”课程名反过来扭曲真实拓扑 |

**结论：拒绝。** 实际结构是 rooted DAG。允许共享后继，但仍禁止环、孤儿和跨 revision edge。

### 方案五：三表 adjacency model + graph header 对 root node 建反向 FK

候选引用如下：

```text
node.(graph_id, revision)
  -> graph.(graph_id, revision)

graph.(graph_id, revision, root_node_id)
  -> node.(graph_id, revision, node_id)
```

| 优点 | 代价 / 风险 |
| --- | --- |
| root existence 由数据库 FK 明确表达 | graph INSERT 要求 root node 已存在 |
| node ownership 也由 FK 表达 | node INSERT 又要求 graph 已存在，形成插入环 |
| 绕过 Repository 也能拒绝 dangling root | InnoDB 外键立即检查，事务不能把检查延迟到 commit |

**结论：拒绝反向 FK。** 同一事务不会创造合法插入顺序；`NO ACTION` 在 InnoDB 中仍是立即 `RESTRICT`。

### 方案六：node 行保存 `is_root`，header 不保存 root identity

| 优点 | 代价 / 风险 |
| --- | --- |
| 不存在 header -> node 反向引用 | root identity 从 aggregate header 分散到子行 |
| 可以用条件唯一索引逼近“最多一个 root” | MySQL 单行/唯一约束仍不能证明“至少一个 root” |
| parent-first 写入顺序直接成立 | restore 要扫描子行后才知道 aggregate root |

**结论：不采用。** v1 把 root 视为 aggregate 的必要单值身份，保留在 graph header；完整性缺口由写前和恢复后领域校验显式承担。

### 方案七：root nullable，先插 graph/node 再 UPDATE root

| 优点 | 代价 / 风险 |
| --- | --- |
| 最终可以建立反向 FK | graph revision 在事务中经历“不完整 -> 修改”状态 |
| 写入顺序可实现 | 需要 nullable root，数据库可长期保存未完成 header |
| commit 前理论上可补齐 | 与 create-only/无 Update 语义冲突，失败恢复更复杂 |

**结论：拒绝。** 不用分阶段可变状态换取一个不能覆盖其他 graph invariant 的 FK。

### 方案八：关闭 `foreign_key_checks` 写入循环引用

| 优点 | 代价 / 风险 |
| --- | --- |
| 任意插入顺序都能通过 | 正常写路径临时关闭安全约束，影响会话中其他写入 |
| 可以保留双向 FK 定义 | 重新开启不会回溯验证关闭期间写入的坏数据 |
| 实现看似直接 | 需要额外特权，破坏最小权限和集成测试可信度 |

**结论：拒绝。** `foreign_key_checks=0` 适合受控导入/维护场景，不是在线 Repository 的完整性协议。

### 方案九：Lottery 专属有界 immutable rooted DAG + 三表 + 逻辑 root 引用

| 优点 | 成本 / 风险 |
| --- | --- |
| 真实表达共享 terminal 与未来共享后继 | graph header 的 root 没有反向 FK |
| 三表保留局部 FK/CHECK/UNIQUE | 全可达、无环、深度必须由领域验证 |
| create-only 绑定 revision/content | 新内容必须新 revision，不能原地编辑 |
| exact schema v1 拒绝未知结构 | 当前只能保存会员 Strategy 路由 |

**结论：采用。** 它把数据库能证明的事实与领域必须证明的事实分开，复杂度与当前需求相称。

## 决策

### 1. 建立 Lottery-owned `StrategyRoutingGraph`

生产领域类型使用 `StrategyRoutingGraph`，而不是跨上下文 `RuleTree` 或 `DecisionEngine`。它只描述 Lottery Strategy 路由拓扑。

graph identity 由以下字段组成：

- 非零 `StrategyRoutingGraphID`（`uint64`）；
- `StrategyRoutingGraphRevision`，满足 `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`；
- `StrategyRoutingGraphSchemaVersionV1 = 1`；
- 非零 root node ID；
- 一组 scoped immutable nodes；
- 一组 scoped immutable directed edges。

GraphID 标识配置家族，Revision 标识该家族的一份不可变内容，schema version 标识解释协议。三者不能互相替代。

### 2. v1 是有界 rooted DAG，不是万能树

v1 node kind 只有：

- `decision`：携带稳定 rule code；
- `strategy_target`：携带非零 StrategyID，且没有出边。

每个 decision 的 rule code 必须为 `lottery.membership_tier.route_strategy`，并且恰有：

- `premium_override`，`is_default = false`；
- `baseline_default`，`is_default = true`。

edge 不携带 expression、priority、script 或自由条件。多个 branch 可以指向同一 target，多个 decision 可以共享后继，因此模型允许 DAG 合流；但不能跨 graph/revision 引用。

### 3. 完整图不变量在 Create 和 Restore 两次执行

合法 graph 必须同时满足：

1. GraphID、Revision、schema、root 合法；
2. node 集合非空且不超过 128，edge 不超过 256；exact v1 decision/terminal 形状会自然要求至少两个 node 和两条 edge；
3. node ID 非零且 scoped unique；
4. root 存在且是 decision；
5. edge 两端存在且 source 为 decision；
6. terminal 没有出边；
7. decision 的 exact rule/branches/default 完整；
8. 全部 nodes 从 root 可达；
9. 图无 self-loop 或任意 cycle；
10. root 到任一 terminal 的最长 edge 数不超过 16；
11. 每条有限路径终止于 `strategy_target`；
12. 输入/返回 collection 不暴露内部可变状态。

root 深度为 0，单个 decision 直接到 terminal 的深度为 1。共享后继不能被误判为 cycle。

`NewStrategyRoutingGraph` 固定创建 schema v1；`RestoreStrategyRoutingGraph` 必须显式接收持久化 schema，并对未知版本失败关闭。Restore 不是降低标准的“数据库构造器”。

### 4. 采用 graph/node/edge 三张 InnoDB 表

物理模型分为：

1. graph header：`(graph_id, revision)` 主键、schema version、root node ID、行创建时间；
2. node：scoped composite key、node kind、互斥的 rule code / target Strategy ID；
3. edge：scoped source/target、branch code、default 标志与唯一性边界。

NodeID 与 GraphID 均使用 `BIGINT UNSIGNED` 对齐完整 `uint64`；revision 使用 ASCII/binary 语义和 128 bytes 上限；enum-like token 使用大小写敏感比较。Migration 继续采用只向前追加、一条 DDL 一个版本的历史，因此三表按依赖顺序分别创建。

graph/node/edge 表不增加 status、published_at、active_revision、ActivityID、tenant、owner principal、审批人、表达式 JSON、执行 path 或更新版本。这些概念没有在本节形成真实生命周期。

### 5. root 是 header 标量，但不建 header -> node FK

node 通过 composite FK 指向 graph；edge 分别通过 composite FK 指向同一 graph revision 内的 source/target node，由节点引用传递保证 graph scope。graph header 的 `root_node_id` 保持 `NOT NULL` / 非零局部约束，但不建立到 node 的反向 FK。

原因不是忽略完整性，而是 MySQL/InnoDB 没有 deferred foreign-key checking：

- 先插 graph 会因 root node 不存在而失败；
- 先插 root node 会因 graph 不存在而失败；
- 包在同一事务中也不会延迟到 commit；
- `NO ACTION` 不提供延迟语义；
- 关闭 foreign key checks 又无法在重启后自动补验。

因此 root existence/type 由 graph aggregate 在写入前验证，并在每次恢复后再次验证。Repository 必须单事务按 header、nodes、edges 写入；事务提交前外部连接看不到完整 revision。拥有绕过 Repository 的数据库写权限仍可能制造坏 root，这属于最小权限、运维审计和 restore fail-closed 共同控制的残余风险，不能在文档中隐藏。

### 6. 同一 `(GraphID, Revision)` create-only

Repository 只提供 create 与 find/restore 语义：

- create 前验证完整 aggregate；
- 一个事务写 header/nodes/edges；
- 任一步失败整体 rollback；
- composite PK 冲突映射为稳定 already-exists/conflict；
- 不提供 Update、Upsert、Replace 或同 revision 改 target/edge；
- 相同 GraphID 的新内容必须使用新 Revision；
- created-at 只是行诊断时间，不是 graph revision、发布时间或乐观锁。

本节不把 create 自动重试称为幂等。若 commit 结果不确定，调用方需要用 identity 查询并比较已恢复内容；不能以相同 revision 写入不同 payload。

### 7. 数据库局部约束与领域全局校验并存

数据库负责它能准确表达的局部事实：

- composite primary/unique key；
- unsigned/nonzero/长度/枚举类 CHECK；
- node -> graph、edge -> source/target node 的 composite FK；
- decision/target 字段互斥；
- source branch/default 的局部唯一性；
- 可建立时的 terminal StrategyID 引用。

领域 Create/Restore 负责跨行和拓扑事实：

- root 存在且为 decision；
- 恰当数量与 exact branch set；
- 全可达；
- 无环；
- terminal 无出边；
- node/edge/depth 上限；
- unknown schema/rule/kind/branch 失败关闭。

数据库约束不能替代领域校验；领域校验也不是删除可表达 FK/CHECK 的理由。

### 8. 默认边保留第 27 节失败关闭语义

`baseline_default` 结构上是 required default，但业务上只承接 confirmed `standard`。unknown/unsupported tier、缺失/过期/未来/损坏 fact、provider error 或 caller cancellation都不能 fallback。

第 28 节没有 evaluator，所以此处冻结的是未来执行兼容条件，不宣称数据库能判断 tier。第 29 节必须使用现有 concrete router 做等价测试。

### 9. 严格恢复，不做自动修复

Repository 从三表读取后必须：

- 先检查 header schema；
- 有界读取最多 128 nodes / 256 edges，并能检测超限；
- 严格映射所有 token；
- 用 `RestoreStrategyRoutingGraph` 执行完整校验；
- 只在 immutable aggregate 合法时返回。

它不得删除孤儿、打断环、补 default、猜 root、trim revision、忽略未知 token 或只读取前 N 行。自动修复会让数据库坏内容与运行行为脱钩，破坏 revision 的可解释性。

### 10. 第 28 节停止线

本节明确不实现或修改：

- graph traversal/evaluator/operator registry/多步 path；
- 通用 `Rule`、`RuleTree`、`Engine`、DSL、DMN、OPA 或脚本；
- draft/publish/retire/rollback/active revision；
- Activity 聚合或 Activity -> graph/Strategy 引用；
- 会员 authority adapter、身份映射或在线门控；
- HTTP/MCP/Agent route、DTO、runtime config、Compose 服务或 React；
- session、Principal、RBAC/ABAC、tenant/data scope 或前端权限裁剪；
- Redis cache、RabbitMQ event、PG dual-write；
- Strategy load/WeightedSelector/随机数、正式 Draw/Result、库存和 Benefit；
- 持久 decision trace、安全审计 UI 或浏览器 E2E。

现有第 27 节 router、Strategy Repository/cache/selector、ephemeral API 和前端行为保持不变。

## 影响

### 正面影响

- GraphID/Revision 终于由权威 create-only 存储绑定完整内容，而不是靠调用方自律；
- rooted DAG 准确表达共享 terminal/后继，不被课程中的“树”一词限制；
- exact v1 schema 让未知 kind/rule/branch 明确失败，不会自动 fallback；
- 三表把 node/edge identity、局部引用和坏数据测试变得可见；
- Create 与 Restore 对称验证，防止旁路 SQL 数据被直接信任；
- 128/256/16 限制把拓扑验证的资源消耗变成可审查边界；
- create-only 为第 30 节 Activity 引用历史 revision 提供稳定输入；
- 第 29 节可以只处理已验证 graph，不必同时发明存储修复语义。

### 成本与限制

- 三表恢复需要多行映射和完整 adjacency validation；
- header root 没有数据库反向 FK，特权旁路写入可能制造 dangling/terminal root；
- 每次 restore 都要做 O(V+E) 级别校验，不能将 scan 成功当合法；
- 新内容必须新 revision，运营编辑/草稿尚无就地保存体验；
- v1 只接受现有 membership rule，不能配置第三种条件或任意表达式；
- target FK/非零引用不证明 Strategy 已发布或内容不可变；
- graph schema 合法不表示有权创建、查看或使用；
- 没有 Activity publisher，数据库中的 revision 仍不等于线上 active 配置；
- 没有 executor，无法给出多步 path、执行耗时或运行结果。

### 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| 绕过 Repository 写入坏 root | 最小数据库账号、无反向更新能力、每次 restore 完整验证、集成损坏 fixture |
| revision 大小写/空白漂移 | ASCII binary grammar；拒绝而非 trim |
| graph bomb | 读取和构造前限制 nodes 128 / edges 256 / depth 16 |
| shared successor 被误判 cycle | cycle 检测区分 recursion stack 与已完成节点；合法 DAG fixture |
| unknown schema 被旧程序误读 | Restore exact-version gate，失败关闭 |
| default 吞掉 unknown | 固定 rule/branch协议；第 29 节对照 concrete router |
| create 只写半图 | 一个 InnoDB transaction，错误 rollback，外部只见 commit 后状态 |
| target 存在但不适用 | 本节不宣称 published/versioned；第 30 节发布绑定再验证 |
| 三表 join 变慢 | 当前硬上限小；先测真实查询，不预建 Redis/冗余 JSON |

## 数据库完整性边界复盘

### 为什么一个事务不等于 deferred constraint

事务提供原子可见性与 rollback，不改变 InnoDB 外键检查时机。循环引用的两条 INSERT 都会在各自语句执行时检查，所以“最后 commit 时两边都有”不是 MySQL 可接受的证明。这个区别必须进入设计文档和面试表达，否则很容易写出永远无法插入的 schema。

### 为什么没有 root FK 仍然可以诚实交付

我们没有宣称数据库单独保证 rooted DAG。实际保证链是：

```text
complete immutable domain graph
  -> Validate root/topology/budgets
  -> one create-only transaction
  -> database local FK/CHECK/UNIQUE
  -> every read bounded + Restore full validation
  -> only validated aggregate reaches future executor
```

缺失的反向 FK 被明确记录为残余风险，并由写前/读后两个校验点和数据库权限共同缓解。比使用 `foreign_key_checks=0` 后声称“外键完整”更可验证。

### 为什么不使用 trigger 补完整性

trigger 也无法自然完成全图 cycle/reachability/depth 校验，还会把业务算法藏进数据库、增加写顺序与 Migration 调试难度。当前领域 validator 已经是所有入口共享的明确契约，因此不为一个 root 反向引用引入 trigger/stored procedure。

## 撤销与演进

第 28 节没有公开 API、Activity 绑定或 production executor。若尚无外部消费者，撤销代码组合可以停止使用 Repository，但已共享 Migration 不能回写或删除，只能追加新版本归档/迁移。

线性演进路径为：

1. 第 27 节 concrete router 固定会员路由产品语义；
2. 第 28 节 immutable rooted DAG 与三表绑定 revision/content；
3. 第 29 节只执行已验证 graph，加入 traversal/path/预算/取消；
4. 第 30 节 Activity publisher 只引用经过批准的不可变 revision；
5. 第 31～35 节统一认证授权保护 graph/Activity 管理与查询；
6. 第 36 节首个运营后台消费服务端能力，而不拥有规则真相。

未来若需要第三种 rule/operator、运营草稿、内容哈希、签名、批量导入或 schema v2，必须新增 ADR 和兼容/迁移矩阵，不改义复用 v1 token。

## 重新评估触发条件

出现以下任一证据时新增决定：

- graph 数量/规模接近 128/256/16 上限；
- 真实拓扑永远没有共享后继，或反之大量共享导致 aggregate 边界变化；
- 新增渠道、地域、用户段、时间窗或复合 operator；
- 需要非开发人员草稿、审批、模拟、发布、灰度或回滚；
- 需要内容寻址 hash、签名、防篡改或跨环境 promotion；
- Strategy 获得独立业务 version，terminal 需要引用 immutable Strategy revision；
- Activity 需要原子绑定 graph 与 Strategy 快照；
- restore 校验或三表读取成为可测性能瓶颈；
- 线上出现 dangling root、绕过 Repository 写入或 schema 漂移事故；
- 需要归档、删除、保留期或法规审计；
- 需要多租户 graph 与对象级 data scope；
- 考虑外部 DMN/规则平台或另一数据库。

## 验收证据

第 28 节必须能够证明：

1. domain 接受合法 single-decision graph、converging targets 和 shared successor DAG；
2. zero/bad identity、unknown schema/kind/rule/branch、缺边、多边、坏 default 均失败；
3. root missing/terminal、dangling edge、unreachable node、cycle、terminal outgoing 均失败；
4. nodes 128、edges 256、depth 16 的安全硬边界有反例测试，且对能同时满足 v1 exact branches、terminal 和全可达约束的边界组合提供正例；
5. 输入/返回 collection 不可修改内部 aggregate，并发读取通过 race；
6. fuzz 对任意 node/edge 组合不 panic、不越界、不无限遍历；
7. 三条 Migration 按 graph/node/edge 顺序建立 schema 与局部约束；
8. root header 没有反向 FK，integration test 和文档明确验证该有意边界；
9. Repository 写前 Validate、单事务 create-only、冲突稳定映射、读后 Restore；
10. 真实 MySQL round-trip 与 corruption fixture 证明坏数据不进入 domain；
11. 除未装配的 MySQL graph repository adapter 外，未新增 executor、publish/Activity、会员 provider/HTTP/runtime adapter、composition、API/UI、权限或浏览器 E2E；
12. 第 27 节 concrete route 和现有 Strategy/selector/ephemeral 行为不变。

## 相关资料

- [Lottery Strategy Routing Graph 基线 v1](../product/lottery-strategy-routing-graph-v1.md)
- [Lottery 会员等级 Strategy 路由基线 v1](../product/membership-strategy-routing-v1.md)
- [ADR-0014：Lottery Strategy/Award 的首个持久化结构](ADR-0014-lottery-persistence-schema.md)
- [ADR-0019：Lottery 规则所有权与评估边界](ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [ADR-0023：会员等级 Strategy 路由边界](ADR-0023-membership-strategy-routing-boundary.md)
- [MySQL 8.4：FOREIGN KEY Constraint Differences](https://dev.mysql.com/doc/refman/8.4/en/ansi-diff-foreign-keys.html)
- [MySQL 8.4：FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)
- [MySQL 8.4：CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)
- [MySQL 8.4：Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)
