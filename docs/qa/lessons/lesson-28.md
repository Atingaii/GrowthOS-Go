# 第 28 节 QA：Strategy 路由图首次持久化验收

- **课程主题：** 规则树第一次数据库升级
- **需求基线：** [Lottery Strategy Routing Graph 基线 v1](../../product/lottery-strategy-routing-graph-v1.md)
- **架构决策：** [ADR-0024](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)
- **上一节：** [第 27 节 QA](lesson-27.md)
- **课程正文：** [第 28 节课程](../../course/part-04/lesson-28-rule-tree-schema.md)
- **API 记录：** [第 28 节 API 记录](../../api/lessons/lesson-28.md)
- **设计手记：** [第 28 节设计手记](../../design-thinking/lessons/lesson-28.md)
- **面试问答：** [第 28 节面试问答](../../interview/lessons/lesson-28.md)
- **基准提交：** `809d436`（第 27 节已验收 tip）
- **已知实现与真实依赖验收序列：** `f27ce17`、`2786d96`、`17a6c54`、`ac89423`、`d53b2ec`、`4d9b074`、`e053527`、`8db8c3c`、`4b79d1d`、`97bb783`、`d7deafa`、`2d2c7c2`、`f6b537d`、`ebe0b70`
- **学习文档与当前态收口序列：** `3b3886b`、`5c757a9`、`be19827`、`54f4769`、`e4531bf`、`4057f5d`
- **验收日期：** 2026-08-30，Asia/Shanghai
- **当前记录状态：** graph domain、窄 Repository port、Migration 000003～000005、未装配的 MySQL graph adapter、单元/集成测试与一次性 MySQL 8.4.11 验收脚本已经形成。mysqlrepo 普通/race/20 轮 shuffle/vet、disposable MySQL 全链路、长期 `growthos` 原地 `2:0 -> 5:0`、重复 migration/status、Compose smoke、隔离 Lottery/cache acceptance、最终 `make verify`、全仓 race、独立 fuzz、atomic coverage、前端 test/typecheck/build、章节 diff/停止线/线性历史和任务自有构建产物清理均已实际通过。远端与累计分支只在冻结提交产生后核对，最终 SHA 以同名远端实际引用为准，不在会改变自身 SHA 的文档提交中预写

> 本节验收的是“一份 Lottery 专属、有界、不可变的 Strategy 路由 rooted DAG 能被完整保存并严格恢复”。它不是通用规则引擎，不执行图，不发布 revision，不把图绑定到 Activity，也没有新增公开 API、UI、会话或权限系统。测试专用 graph repository 账号具备精确三表 `SELECT, INSERT`，不等于长期运行 API 账号已经获得同样权限。

## 1. 证据状态词汇

为避免把测试文件存在、某次实现阶段通过和最终分支冻结混为一谈，本文只使用以下四种状态：

| 状态 | 精确定义 |
| --- | --- |
| **ACTUAL-PASS** | 命令已经在本机真实执行并以 exit 0 完成；若依赖 MySQL，则测试没有 skip |
| **CODE-EVIDENCE** | 实现和对应测试已经落盘，但本文不把它冒充最终 accepted-tip 的独立复跑 |
| **FINAL-CANDIDATE-PASS** | 完整文档与索引已经收口，命令已在候选内容上真实 exit 0；冻结提交只承载相同内容与证据 |
| **OUT-OF-SCOPE** | 本节刻意没有交付，不能通过测试旧路径来伪装已经完成 |

一次 `go test` exit 0 只有在目标测试实际运行时才算证据。MySQL 集成测试依赖显式授权 token 与连接变量；缺少变量时会 `Skip`，所以普通无环境的全仓测试不能替代本节 disposable MySQL 验收。

## 2. 本节必须成立的能力

### 2.1 领域与拓扑

- aggregate identity 是非零 `(GraphID, Revision)`；GraphID、NodeID 与 StrategyID 保留完整 `uint64` 范围；
- revision 精确遵循 `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`，使用 ASCII binary identity，不 trim，不进行大小写折叠；
- `schema_version = 1` 是唯一可恢复版本，zero、future 或 unknown schema 失败关闭；
- v1 节点封闭为 `decision` 与 `strategy_target`；不接受任意表达式、脚本、URL、handler 或 `map[string]any`；
- decision 只接受 `lottery.membership_tier.route_strategy`；terminal 只携带非零 StrategyID；两个 union variant 不能混装；
- 每个 decision 精确包含 `premium_override` 非默认边和 `baseline_default` 默认边；default 不能吞 unknown、依赖失败或取消；
- graph 有显式单一 root，root 必须存在且为 decision；所有节点从 root 可达；
- edge 两端必须存在、source 必须为 decision、terminal 不得有出边；
- graph 必须无环；多个 branch 可以汇聚同一个 terminal，多个 decision 可以共享合法后继，因此领域名是 rooted DAG 而不是强行称为 tree；
- 节点上限 128、边上限 256、最长 root-to-terminal edge depth 上限 16；边界 16 合法，17 失败；
- 构造和恢复都 canonicalize node/edge order，并对输入、输出 slice 做防御性复制；
- 任意失败只返回零 `StrategyRoutingGraph`，不能返回半个可执行对象。

### 2.2 持久化

- Migration inventory 精确结束于 v5，000003/000004/000005 分别创建 graph、node、edge 三张 InnoDB 表；
- 已发布 migration 以 checksum 冻结，每个 migration 只有一条 DDL statement，不能改写历史；
- `(graph_id, revision)` 是 graph 主键；node/edge identity 都携带完整 graph/revision scope；
- node -> graph、terminal -> Strategy、edge -> source/target node 使用 `RESTRICT` 外键；
- graph header 的 `root_node_id` 刻意没有反向 FK，因为 MySQL/InnoDB 不提供 deferred FK，正反引用会造成没有合法第一条 INSERT 的环；
- 数据库负责局部 CHECK/FK/UNIQUE，domain Create/Restore 负责 root、全可达、无环、完整 branch set 和资源预算；任何一层都不能冒充另一层；
- 写入是 create-only：header -> canonical nodes -> canonical edges 同一事务；不提供 Update、Upsert、Replace、Delete 或 partial mutation；
- 相同 GraphID 的新内容必须使用新 revision；相同 `(GraphID, Revision)` 冲突，不暗中覆盖；
- 每一行都检查 `RowsAffected == 1`，child failure 必须使 header/nodes/edges 全部 rollback；
- 读取使用完整 identity、显式列、`LIMIT 129/257`，在同一 `REPEATABLE READ` + read-only snapshot 中取得 header/nodes/edges；
- snapshot 结束后才做 strict Restore；未知 schema 在读取 children 前快速失败；坏数据不能被 trim、删孤儿、断环、补 default 或猜 root 后继续；
- commit error 的写入结果可能未知，不能盲目把重试当幂等成功。

### 2.3 权限与运行边界

- migration identity 与业务 identity 必须不同；
- disposable legacy API identity 只保留既有 `lottery_strategy` / `lottery_strategy_award` 的精确 `SELECT, INSERT`；
- disposable graph repository identity 只拥有三张 routing graph 表的精确 `SELECT, INSERT`，并且与 migration identity、legacy API identity 都不同；
- graph identity 不能读旧 Strategy 聚合表和 `schema_migrations`，不能写旧表，不能在 graph 表执行 UPDATE/DELETE；
- mandatory roles 必须为空，防止角色间接扩大 `SHOW GRANTS` 看不到的预期 allowlist；
- 本节 graph adapter 刻意未进入 runtime composition；长期 API 身份没有因为“测试需要写图”自动获得 graph 权限；
- schema 合法、create 成功和 repository 可读都不等于调用者有权查看、编辑、发布或执行 graph。

## 3. 本节明确不验收

- 第 29 节 graph evaluator、operator registry、多步执行 path、步数/时间预算；
- 通用 `Rule` / `RuleTree` / `Engine` / DSL / DMN / OPA / 脚本运行时；
- draft、publish、retire、rollback、active revision、灰度或审批流；
- 第 30 节 Activity -> graph revision 绑定及目标 Strategy 发布态校验；
- Membership provider、Principal 映射、Participation eligibility、正式 Draw/Result、库存和发奖；
- HTTP/MCP/Agent route、request/response DTO、错误码或新增公开 endpoint；
- React 页面、菜单、按权限裁剪的路由/操作或浏览器 E2E；
- 第 31～35 节 session、Principal、RBAC/ABAC、tenant/data scope 与越权验收；
- Redis graph cache、RabbitMQ 发布事件、PostgreSQL 复制或双写；
- 生产 graph 管理账号、生产写入工作流、备份恢复演练、容量/SLO 或线上发布资格。

旧 Lottery API、前端或 Compose smoke 即使通过，也只能证明旧路径没有明显回归，不能证明本节 graph 已被运行时消费。

## 4. 需求到证据覆盖矩阵

| 验收面 | 主要代码/测试证据 | 当前状态 | 不能推出 |
| --- | --- | --- | --- |
| rooted DAG 与封闭 node/edge union | `strategy_routing_graph.go` / `strategy_routing_graph_test.go` | CODE-EVIDENCE | 已有通用规则语言 |
| 合流不误判 cycle | convergence、shared successor、longest-depth tests | CODE-EVIDENCE | 任意图算法都已验证 |
| 128/256/16 预算 | limit/depth boundary tests，读取 `LIMIT 129/257` | CODE-EVIDENCE | 线上容量或 latency SLO |
| graph value 不可变 | input/output slice mutation、64-worker read tests | CODE-EVIDENCE | 真实 adapter 的所有调用都无 race |
| damaged topology fuzz target | `FuzzRestoreStrategyRoutingGraphTopologyNeverPanicsOrLoops` | FINAL-CANDIDATE-PASS；10 秒、1,693,489 execs | 对所有输入的穷举证明 |
| 窄 application port | `StrategyRoutingGraphCreator` / `Reader` 反射测试 | CODE-EVIDENCE | 已有管理 use case 或 runtime composition |
| 三表 migration v5 | embed checksum/inventory tests + schema integration | ACTUAL-PASS（真实 MySQL） | 数据库单独证明 rooted DAG |
| graph Create transaction | sqlmock 顺序/RowsAffected/rollback/commit tests | ACTUAL-PASS（mysqlrepo 普通/race/shuffle/vet） | 网络断开时 commit 一定失败 |
| graph Restore | bounded explicit query、unknown schema、bad union/topology tests | ACTUAL-PASS | 数据库坏内容会被自动修复 |
| `uint64` 完整扫描 | sqlmock `[]byte("18446744073709551615")` + MySQL round trip | ACTUAL-PASS | 所有第三方 driver 都有相同 scan 行为 |
| RR read-only snapshot | call-site AST、helper options、MySQL isolation/write rejection | ACTUAL-PASS | 多服务形成跨 authority 原子快照 |
| create-only 冲突 | duplicate + two-worker concurrent create | ACTUAL-PASS | create 可以盲目重试或等同幂等 |
| child failure 原子回滚 | unit rollback + MySQL 临时 CHECK probe | ACTUAL-PASS | 所有未知数据库故障均可自动重试 |
| exact graph grants | `SHOW GRANTS` allowlist + mandatory roles + negative probes | ACTUAL-PASS | 生产 RBAC/租户授权已完成 |
| 查询计划 | real MySQL `EXPLAIN` | ACTUAL-PASS | 数据增长后计划永不变化 |
| disposable 环境隔离 | exact container label/name、tmpfs、快照与 cleanup | ACTUAL-PASS | 其他共享/生产环境已经迁移到 v5 |
| 长期 Compose 前向迁移 | 同一 MySQL container/named resources、`2:0 -> 5:0`、旧表 fingerprint、新表空态 | ACTUAL-PASS | 任意旧 volume 会自动升级或生产迁移零风险 |
| 长期运行身份最小权限 | exact grants、空 mandatory roles、graph `SELECT`/`INSERT` 拒绝与 smoke | ACTUAL-PASS | 产品层认证、RBAC 或对象级授权 |
| 隔离 Lottery/cache 回归 | v5 acceptance、故障恢复、三组 M1 与所有权清理 | ACTUAL-PASS | graph 已被 HTTP/runtime 消费或 M1 是生产 SLO |
| 全仓/前端/最终文档门禁 | `make verify`、全仓 race、frontend、doccheck | FINAL-CANDIDATE-PASS | 获得生产 SLO 或图已进入 runtime |

## 5. 按提交核查真实演进

| 顺序 | 提交 | 演进焦点 |
| ---: | --- | --- |
| 1 | `f27ce17` | 先冻结 graph 的产品语言、身份、不变量和停止线 |
| 2 | `2786d96` | ADR 决定有界不可变 rooted DAG、三表和 create-only 边界 |
| 3 | `17a6c54` | 实现 graph/node/edge value、拓扑校验、预算与防御性复制 |
| 4 | `ac89423` | 校正合法分支合流语义，防止把 DAG 错写成严格树 |
| 5 | `d53b2ec` | 增加 aggregate-scoped Creator/Reader port 与稳定错误分类 |
| 6 | `4d9b074` | 增加 000003～000005 及 MySQL schema 集成证据 |
| 7 | `e053527` | 对齐实际 FK shape，诚实记录 root 反向 FK 缺失 |
| 8 | `8db8c3c` | 实现未装配 graph MySQL adapter 与 sqlmock contract |
| 9 | `4b79d1d` | 加固 MaxUint scan、RR read-only call-site 与 scope 文档证据 |
| 10 | `97bb783` | 补充面试问答，不改变运行时 |
| 11 | `d7deafa` | 增加真实 MySQL graph repository integration |
| 12 | `2d2c7c2` | 将一次性 MySQL 8.4.11 验收和精确清理自动化 |
| 13 | `f6b537d` | 将 Compose build/schema checkpoint 前向推进到 lesson 28 / v5，不新增 graph runtime consumer |
| 14 | `ebe0b70` | 在长期 smoke 与隔离 Lottery acceptance 中固定运行身份 graph 表拒绝 |
| 15 | `3b3886b` | 交付课程与零公开 HTTP 变化的 API 记录 |
| 16 | `5c757a9` | 交付第一性原理设计手记与技术权衡 |
| 17 | `be19827` | 登记实现阶段、真实 MySQL 与 Compose 验收证据及待冻结项 |
| 18 | `54f4769` | 对齐 README、产品边界与 Repository 当前态 |
| 19 | `e4531bf` | 对齐本地 Compose 与 Migration 运维手册 |
| 20 | `4057f5d` | 登记课程、API、QA、设计与面试索引及章节状态 |

可复核线性切片：

```bash
git log --reverse --oneline 809d436..ebe0b70
git diff --stat 809d436..ebe0b70
```

提交数量不是验收目标。真正的顺序是先回答“为什么持久化、保存什么、谁拥有”，再建立领域不变量，之后才落 Migration、port、adapter 和真实依赖证据；没有从一个空泛通用引擎倒推业务需求。

## 6. Domain 结构验收

### 6.1 identity、revision 与 schema

| 输入 | 预期 |
| --- | --- |
| GraphID 0 | `ErrStrategyRoutingGraphIdentityInvalid` + zero value |
| revision 空、首字符标点、空白、slash、非 ASCII、129 bytes | identity invalid，不 trim |
| revision 128 bytes 边界 | 合法 |
| `route-v1` 与 `Route-v1` | 两个大小写敏感 identity |
| schema 0 或 2 | `ErrStrategyRoutingGraphSchemaUnsupported` |
| schema 1 | 进入完整 shape/topology 校验 |

Revision 不是 content hash。create-only composite key 只保证成功创建以后该 identity 不被原地覆盖；它不证明两个数据库、两个环境或创建前的调用方天然对同一 token 使用同一内容。

### 6.2 node/edge union

`decision` 必须有 rule code、没有 StrategyID；`strategy_target` 必须没有 rule code、有非零 StrategyID。Repository 用 `sql.NullString` 与 `sql.Null[uint64]` 保留 NULL 和 zero 的差别，再交给 Restore，不能用 Go zero 值把损坏的 SQL union 合法化。

Edge 的 `is_default` 是持久证据：

```text
premium_override -> is_default false
baseline_default -> is_default true
```

恢复时不重算并覆盖数据库 marker，而是核对 marker 与 branch 是否一致。这样旁路坏行会被拒绝，而不会在内存中悄悄修好。

### 6.3 topology 与算法边界

领域测试覆盖 duplicate node、duplicate branch、dangling source/target、terminal source、missing/non-decision root、unreachable node、self/two-node cycle、缺失 branch、错误 default、超 node/edge budget，以及最长路径深度 16/17 边界。

共享后继的正确判定需要区分“当前递归栈中的灰色节点”和“已经完成的黑色节点”。第二个父节点再次访问已完成 successor 是合法合流；再次进入当前 path 才是 cycle。另有 shallow-first fixture 证明深度计算不能因为节点首次被较短路径访问就缓存错误结果。

## 7. Migration 与数据库完整性验收

### 7.1 实际 v5 schema 证据

真实 MySQL gate 已验证：

- migration status 为 `clean`，`version = 5`，`latest = 5`；
- graph/node/edge 三表是 InnoDB，关键 token 列是 `ascii/ascii_bin`；
- 4 个命名 `RESTRICT` FK 的 9 个 ordered reference column 精确匹配设计；
- `root_node_id` 没有反向 FK，这是已批准边界，不是漏写后仍声称数据库完整；
- case-distinct revision 可以共存；trailing whitespace、非法 grammar、zero ID、future schema 由 CHECK 拒绝；
- missing graph/node/Strategy、cross-revision edge 由 FK 拒绝；
- duplicate identity/branch 由主键拒绝；
- parent graph、node、Strategy 被引用时不能删除；
- schema fixture transaction 显式 rollback，随后在事务外核对 Strategy/header/node/edge 均为零残留。

### 7.2 为什么 root 由 Restore 再验证

下面的插入环在 InnoDB 中无法靠“最后 commit 时完整”解决：

```text
node -> graph FK
graph.root_node_id -> node FK
```

两边都会逐语句立即检查，没有 deferred constraint，因此不存在合法第一条 INSERT。本节保留 node -> graph 和 edge -> node 的正向 FK，不使用 `foreign_key_checks=0`，同时用以下链路补齐 aggregate 完整性：

```text
完整 domain graph
  -> Create 前 Validate
  -> graph/nodes/edges 单事务写入
  -> 数据库局部 FK/CHECK/UNIQUE
  -> 每次读取有界
  -> Restore 再验证 root 与完整 topology
  -> 只返回合法 immutable aggregate
```

真实 integration 特意用 migration identity 写入“header 的 root=999、实际 node 集合无 999”的局部合法坏 fixture。graph repository 可以扫描这些行，但最终必须返回 `ErrStoredStrategyRoutingGraphInvalid` 和 zero graph。这是对“数据库能保证什么/不能保证什么”的直接反证。

## 8. Repository 写路径验收

### 8.1 Create 顺序与事务

sqlmock evidence 冻结了：

1. invalid graph 在任何 SQL 前失败；
2. `BeginTxx`；
3. 插入 header；
4. 按 canonical NodeID 插入 nodes；
5. 按 canonical `(from, branch)` 插入 edges；
6. 每行核对 `RowsAffected == 1`；
7. cancellation 可见时不 commit；
8. commit 成功才返回 nil。

header/node/edge 任一 RowsAffected 异常或 child driver error 都 rollback。duplicate header 的 MySQL 1062 映射为 `ErrStrategyRoutingGraphAlreadyExists`，安全对外文案不泄露 SQL/driver detail。commit failure 映射为 `ErrCommitOutcomeUnknown`，因为服务器可能已经 durable，不能无条件自动重放。

### 8.2 真实并发与回滚

MySQL integration 对同一 immutable identity 同时发起两个 Create，实际结果精确为：

```text
1 success + 1 ErrStrategyRoutingGraphAlreadyExists
```

随后核对只有 1 header、3 nodes、2 edges，没有“双成功”或半图。

child rollback probe 临时为 routing node 加入只针对测试 graph/node 的 CHECK，使 header 与前序 node 写入后在后续 child 失败。Repository 返回稳定 storage failure；事务结束后以 migration identity 查询，header/node/edge 均为 0。probe constraint 在 cleanup 中删除，并再次查询 information_schema 确认零残留。

## 9. Repository 读路径验收

### 9.1 一致快照与有界读取

Find 使用：

```text
sql.LevelRepeatableRead
ReadOnly: true
```

单元测试一方面核对 helper 的 exact options，另一方面通过 Go AST/源码调用点证明 `FindByIdentity` 的 `BeginTxx` 实际传入该 helper；这是因为当前 sqlmock 版本不能直接断言 `TxOptions`。

真实 MySQL probe 再证明服务端看到 `REPEATABLE-READ`，并在该事务内尝试 INSERT。服务器 read-only 错误 1792 经 driver 的 `rejectReadOnly` 映射为 `driver.ErrBadConn`，写入后零行。两份证据组合起来证明“生产 Find 调用点使用该 options”与“MySQL 实际执行该 options”各自成立；不能只拿 helper 测试冒充 call-site，也不能只开一笔旁路事务冒充 Repository。

header、nodes、edges 都用完整 `(graph_id, revision)` 谓词。nodes `LIMIT 129`、edges `LIMIT 257` 允许读到“上限 + 1”后明确报 corrupt；不能用 `LIMIT 128/256` 截断后把超限数据库误认为合法图。

### 9.2 strict Restore 与错误边界

| 场景 | public class | 返回值 |
| --- | --- | --- |
| identity 不存在 | `ErrStrategyRoutingGraphNotFound` | zero graph |
| header schema 未知 | `ErrStoredStrategyRoutingGraphInvalid`，children 不再查询 | zero graph |
| node/edge 超限 | `ErrStoredStrategyRoutingGraphInvalid` | zero graph |
| NULL union、unknown kind/branch、错误 default marker | `ErrStoredStrategyRoutingGraphInvalid` | zero graph |
| missing root、dangling/unreachable/cycle | `ErrStoredStrategyRoutingGraphInvalid` | zero graph |
| caller canceled | `context.Canceled` 可识别 | zero graph / no write |
| lock wait/deadlock | repository retryable class | zero graph / rollback |
| read commit failure | repository storage/commit class | zero graph |

Read transaction 在 strict domain restore 前 commit。这样 database/sql 资源生命周期不会被领域 CPU 校验无谓拉长；即使 restore 发现坏内容，也只产生 zero aggregate + error，不会让坏内容成为可执行对象。

### 9.3 `MaxUint64` 证据

`BIGINT UNSIGNED` 超过 signed int64 后，scan 路径最容易只在“小 ID”测试中被遗漏。本节有两层证据：

- sqlmock 将 `18446744073709551615` 作为真实 driver 常见的 `[]byte` 值交给 `sql.Null[uint64]`，不是只手工构造一个已经合法的 Null；
- disposable MySQL 用 `math.MaxUint64` 作为 GraphID、terminal NodeID 和 StrategyID 完整 Create/Find round trip，并逐字段比较恢复 aggregate。

它证明当前 MySQL driver/sqlx 路径保留完整无符号范围；不应把数据库 ID 偷换成 `int64` 或先 scan signed 再转换。

### 9.4 查询计划

真实 MySQL 8.4.11 `EXPLAIN` 已实际验证：

- header lookup 为 `const` + `PRIMARY`；
- node lookup 使用 `PRIMARY`；
- edge lookup使用 `PRIMARY`；
- node/edge canonical `ORDER BY` 均没有 `Using filesort`。

这是当前 schema、当前 SQL、当前 fixture 下的计划证据，不是永久性能保证。数据分布、MySQL 版本或 query 变化后仍需重新验收。

## 10. 授权与负向验收

### 10.1 三种身份

| 身份 | 正向权限 | 明确禁止 |
| --- | --- | --- |
| migration | disposable schema 的 Migration/fixture 管理 | 不能与业务账号复用 |
| legacy API test identity | old Strategy 两表 `SELECT, INSERT` | DDL、UPDATE、DELETE、`schema_migrations`；精确 grant 中也不包含 graph 三表 |
| graph repository test identity | graph/node/edge 三表 `SELECT, INSERT` | old Strategy 两表、`schema_migrations`、graph UPDATE/DELETE、DDL；不能复用 API/migration user |

graph repository 在测试中需要 terminal Strategy 父行，但 graph identity 本身不能读取或写入 `lottery_strategy`。父行由 migration fixture identity seed，这避免为了准备外键目标而扩大 graph repository 权限。

`SHOW GRANTS FOR CURRENT_USER` 必须与精确 allowlist逐项相等，且 `@@GLOBAL.mandatory_roles` 为空。只测试“某条 SELECT 成功”不足以证明最小权限，因为账号可能同时拥有全库权限。

### 10.2 负向 SQL

真实 graph identity 已执行并得到 MySQL 1142 的负向面包括：

- 读取 `lottery_strategy`、`lottery_strategy_award`、`schema_migrations`；
- UPDATE 三张 graph 表；
- DELETE 三张 graph 表；
- INSERT 两张旧 Strategy 表；
- INSERT/UPDATE `schema_migrations`。

这些是数据库权限边界，不是产品层 authorization。谁能在管理 API 中创建、查看或发布某个 graph，要等会话、RBAC 与 scope 章节由服务端强制；前端隐藏按钮不能替代这些 SQL/HTTP 负向证据。

## 11. Disposable MySQL 8.4.11 全链路证据

### 11.1 实际执行命令

```bash
./scripts/lesson28-mysql-acceptance.sh
```

- **状态：** ACTUAL-PASS，exit 0；
- **依赖：** `mysql:8.4.11`；
- **执行方式：** 随机精确容器名/label、loopback 动态端口、MySQL data tmpfs、随机密码文件、独立 database 与三种 identity；
- **Migration：** 先由 `growth-migrate up` 应用 embedded migrations，再运行集成套件；最终 clean exact v5；
- **并行策略：** `-count=1 -p=1 -run 'Integration$'`，避免同一 disposable schema 上的破坏性集成测试并行；
- **输出结论：** MySQL pools、migration runner、旧 Lottery schema、routing graph schema、旧 Strategy repository、graph repository 六个 integration tests 全部 PASS，没有以 skip 代替通过。

脚本实际运行的 package/test：

| Package | Test | 结论 |
| --- | --- | --- |
| `internal/infrastructure/mysql` | `TestMySQLPoolsIntegration` | PASS |
| `internal/infrastructure/migration` | `TestRunnerMySQLIntegration` | PASS |
| `migrations` | `TestLotterySchemaMySQLIntegration` | PASS |
| `migrations` | `TestStrategyRoutingGraphSchemaMySQLIntegration` | PASS |
| `internal/lottery/adapter/mysqlrepo` | `TestRepositoryMySQLIntegration` | PASS |
| `internal/lottery/adapter/mysqlrepo` | `TestStrategyRoutingGraphRepositoryMySQLIntegration` | PASS |

### 11.2 它曾真实发现三个问题

这条 gate 不是“写完一次就绿”的装饰。首次真实执行依次暴露并推动修正：

1. **BSD `sed` 可移植性：** macOS 默认 BSD sed 与 GNU-only 写法不一致；端口解析改成 POSIX 基本正则可接受的 `[0-9][0-9]*`，不要求用户另装 GNU sed。
2. **Docker `--rm` 异步清理：** `docker stop` 返回时 auto-remove 可能还未完成；cleanup 增加有界轮询，并在退出前同时检查 exact container name 与 exact unique label 为零。
3. **edge query `Using filesort`：** 真实 `EXPLAIN` 发现排序与主键顺序不一致；查询 canonical order 调整为匹配 `(graph_id, revision, from_node_id, branch_code)`，复跑后 PRIMARY 且无 filesort。

因此验收记录保留“先失败、修正、再通过”的工程轨迹，不把第一版脚本描述成天然正确。

## 12. mysqlrepo 实现阶段门禁

以下四条已实际执行并通过：

```bash
go test -count=1 ./internal/lottery/adapter/mysqlrepo
go test -race -count=1 ./internal/lottery/adapter/mysqlrepo
go test -shuffle=on -count=20 ./internal/lottery/adapter/mysqlrepo
go vet ./internal/lottery/adapter/mysqlrepo
```

- **状态：** ACTUAL-PASS；
- **race：** 无数据竞争报告；
- **shuffle：** 20 轮通过，当前 package tests 不依赖固定执行顺序；
- **vet：** package 静态检查通过；
- **边界：** 这不是全仓 race/vet，也不是最终 accepted-tip clean-worktree gate。mysqlrepo 命令会编译依赖 package，但不会执行依赖 package 的 `_test.go`。

## 13. 长期 Compose 与隔离 Lottery/cache 实际回归

### 13.1 长期 `growthos` 原地前向迁移

本节不是只在 disposable database 证明 v5。对用户已经运行的默认 `growthos` Compose 栈，先记录资源与数据快照，再实际执行：

```bash
make compose-up
make compose-migrate
make compose-status
make compose-smoke
```

实际结果：

- `make compose-up` exit 0；同一个 MySQL container ID、既有 named volume 与既有 networks 前后 identity 不变；
- `schema_migrations` 从 clean `2:0` 前向到 clean `5:0`；旧 `lottery_strategy` / `lottery_strategy_award` 的 `0:0:empty` fingerprint 前后相同；
- 新 `lottery_strategy_routing_graph` / `lottery_strategy_routing_node` / `lottery_strategy_routing_edge` 行数为 `0:0:0`；
- 长期 `growthos_app` 的 direct grants 仍精确为 `USAGE` + 旧两表 `SELECT`，`@@GLOBAL.mandatory_roles` 为空；它读取每张 graph 表都真实收到 MySQL 1142，smoke 的零行 graph `INSERT` 权限探针也被拒绝；
- 已到 v5 后再次执行 `make compose-migrate` exit 0，结果为 `no_change` / latest 5；`make compose-status` exit 0，结果为 clean v5；
- `make compose-smoke` exit 0，覆盖 lesson-28 image/version、v5、运行身份 graph denial、既有 HTTP/Redis/network/secret/port 契约。

这些事实只属于本次本地长期实例。保留同一 container/volume/network 且旧表 fingerprint 不变，不能推出任意环境会自动迁移，也不能替代生产备份、审批、演练与容量评估。

### 13.2 隔离 Lottery/cache acceptance 与本轮 M1

随后实际执行：

```bash
make compose-lottery-api-acceptance
```

- **状态：** ACTUAL-PASS，exit 0；
- **schema/权限：** 隔离栈为 clean v5；运行身份仍只有旧两表 `SELECT`，graph 表读取与零行写入探针被拒绝；
- **旧路径回归：** 既有 Lottery endpoint、Redis ACL/poison 修复、Redis/MySQL stop/recovery、并发与业务表全列 fingerprint 均通过；
- **边界：** graph adapter 仍未装配，整条 acceptance 没有创建、读取或执行 graph。

本轮同一台本机、同一脚本的三组 50 RPS × 10 秒 M1 数据为：

| 场景 | scheduled / completed / success | errors / unexpected / dropped | actual RPS | p50 | p95 | p99 | max | 路径证据 |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| warm-cache | `500 / 500 / 500` | `0 / 0 / 0` | 50.08126457430281 | 2.131833 ms | 4.74375 ms | 7.435958 ms | 11.71225 ms | MySQL executes `0`；cache hits `500` |
| direct MySQL | `500 / 500 / 500` | `0 / 0 / 0` | 50.080440034118105 | 3.166208 ms | 4.623333 ms | 6.879708 ms | 14.423667 ms | MySQL executes `1000`；source loads `500`；cache events `0` |
| Redis down | `500 / 500 / 500` | `0 / 0 / 0` | 50.07592365322605 | 3.184375 ms | 5.987083 ms | 8.678667 ms | 11.255209 ms | MySQL executes `1000`；source loads/fill leaders `500/500`；joined `0`；read-error logs `1` |

计数与路径证据证明本次短窗口没有 transport/status/completion failure，并分别走了 warm hit、禁用缓存直读和 Redis 故障回源。瞬时 RPS/延迟只用于本机回归，不能写成生产吞吐、P99、容量或 SLO，也不能证明 Lesson 28 graph 的执行性能。

验收结束后，脚本按精确 ownership 清除了本次随机 Compose project 的 containers、volumes、networks、task image tags、buildx builder/state、Secret 与 response files/directories；长期默认 `growthos` containers/volumes/networks 前后 identity 不变。可复用基础镜像与共享构建缓存保留。

## 14. Final freeze 实际矩阵

以下命令均在完整文档和索引收口后的候选内容上真实执行。测试不是因为提交动作才成立；冻结提交只把已经通过的同一工作树内容固化进历史。

| 验收项 | 实际命令/证据 | 结果 |
| --- | --- | --- |
| 独立文档检查 | `go run ./cmd/doccheck` | ACTUAL-PASS，`documentation checks passed` |
| 完整仓库门禁 | `make verify` | ACTUAL-PASS，约 14.14 秒；Go vet/test、doccheck 与全部前端门禁均通过 |
| 全仓 race | `go test -race -count=1 ./...` | ACTUAL-PASS，约 15.05 秒，无 race 报告 |
| graph fuzz 窗口 | `go test ./internal/lottery/domain -run='^$' -fuzz='^FuzzRestoreStrategyRoutingGraphTopologyNeverPanicsOrLoops$' -fuzztime=10s` | ACTUAL-PASS；baseline `238/238`，1,693,489 execs，5 个 new interesting（total 243） |
| atomic coverage | 六个 Lottery package，`-covermode=atomic` + `go tool cover -func` | ACTUAL-PASS；总覆盖率 89.9% |
| frontend test | `make verify` 内 Vitest | ACTUAL-PASS；19/19 files、152/152 tests |
| frontend typecheck | `tsc --noEmit` | ACTUAL-PASS，无错误 |
| frontend build | Vite v8.0.3 | ACTUAL-PASS；2,462 modules，约 245 ms |
| 章节 diff | `git diff --check 809d436..HEAD` | ACTUAL-PASS |
| runtime stop-line diff | `cmd/growth-api`、`web`、HTTP adapter、Participation、configs、`go.mod/go.sum` 精确路径 | ACTUAL-PASS，`809d436..HEAD` 无变化 |
| 线性历史 | `merge-base --is-ancestor` + `rev-list --merges` | ACTUAL-PASS；基于 `809d436`，候选 20 个线性、零 merge 提交 |
| main/cumulative 前置核对 | 本地与远端 ref | ACTUAL-PASS；main 仍为 `3ec52a2`，累计分支在冻结前仍为第 27 节 `809d436` |
| task-owned artifact cleanup | coverage temp、fuzz corpus、`web/dist` | ACTUAL-PASS；均不存在，工作树重新干净 |

冻结提交 push 后还会执行同名远端 SHA 相等、累计分支 fast-forward 与 main 不变的外部 ref 核对。最终 commit SHA 不能安全写入它自身：一旦修改文档写入 SHA，SHA 就会再次变化，因此以 Git 远端引用和本节最终交接记录共同作为证据。

### 14.1 当前文档草稿检查

本 QA、并行课程/API/设计文档已经落盘后实际执行：

```bash
go run ./cmd/doccheck
git diff --check 809d436..HEAD
git diff --check
test -z "$(git diff --no-index --check /dev/null \
  docs/qa/lessons/lesson-28.md 2>&1 || true)"
```

- **实际：** 四条检查均 exit 0；doccheck 输出 `documentation checks passed`；
- **为什么新增文件需要第四条：** 普通 `git diff --check` 不检查 untracked 文件；`git diff --no-index` 对“文件内容不同”本身会返回 1，因此外层只断言其 `--check` 没有输出 whitespace error；
- **边界：** 这是文档形成阶段的检查；完整索引收口后已经再次执行本节 14 的全量门禁，不能用这里的早期结果替代最终矩阵。

## 15. 停止线与精确负向断言

出现以下任一情况，本节应判不通过：

- 把课程标题中的“树”直接实现成只允许单父节点，拒绝合法 DAG 合流；
- 把任何再次访问节点都判成 cycle，导致 shared successor 误报；
- 不计算 longest path，因 shallow-first 缓存而接受 depth 17；
- unknown schema/kind/rule/branch 被当成 v1 或 default；
- revision 被 trim、case-fold，或允许 Unicode/空白后仍称其为 canonical identity；
- decision 携带 StrategyID、terminal 携带 rule code，或 NULL 被 scan 成合法 zero union；
- default edge 吞 unknown tier、not-found、stale、provider error 或 cancellation；
- 只依赖 CHECK/FK，就宣称数据库证明了 root、全可达、无环和完整 branch set；
- 使用 `foreign_key_checks=0` 伪装循环 root FK 可行；
- 读取只取前 128/256 行后忽略超限，或自动删除坏行继续；
- 查询不带完整 `(GraphID, Revision)` scope；
- Create 出现 Update/Upsert/Replace/partial mutation，或 duplicate 被当成功覆盖；
- child 失败后留下 header 或部分 nodes/edges；
- commit error 被无条件分类为“未写入”并自动盲重试；
- GraphID/NodeID/StrategyID 通过 `int64` 中转丢失 `MaxUint64`；
- Find 的三次读取不在同一 RR read-only snapshot；
- graph repository identity 获得旧表、schema_migrations、UPDATE/DELETE 或全库权限；
- 为 integration 方便复用 migration/legacy API identity；
- 把测试专用 graph grant 描述成生产 RBAC 已完成；
- 把 graph adapter 接入当前 runtime、旧 API 或 UI，却仍宣称本节无执行/管理 surface；
- 新增通用 engine/DSL、Activity publish、认证授权、Redis graph cache 或正式 Draw；
- 只跑无环境的 `go test ./...`，集成测试全部 skip，却声称真实 MySQL 已验收；
- 最终 `make verify`、全仓 race、fuzz、前端门禁或远端冻结尚未执行就预写 exit 0。

## 16. 剩余风险与下一责任

| 风险 | 当前影响 | 下一触发器 / 责任 |
| --- | --- | --- |
| header root 没有 FK | 特权旁路 SQL 可制造 dangling/non-decision root | 继续最小权限 + 每次 Restore；若形状升级，重新评估 schema |
| graph identity 不是内容 hash | 跨环境同 token 未天然绑定同内容 | 发布 registry/内容摘要需求明确后再设计 |
| Strategy FK 只证明写入时存在 | 不证明 target 已发布、版本稳定或与 Activity 兼容 | 第 30 节 Activity 发布绑定 |
| graph 尚不能执行 | 没有 runtime decision/path | 第 29 节 concrete evaluator，继续以第 27 节 router 为 oracle |
| 没有 active revision | 数据库里存在不等于线上启用 | 第 30 节生命周期/发布语义 |
| graph adapter 未装配 | 当前 API 不会读写新表 | 未来有受权管理 use case 后显式 composition |
| 测试账号不是生产授权 | 无法说明哪个真人/服务可查看或编辑 | 第 31～35 节 session、RBAC、scope、E2E |
| 每次 Restore 为 O(V+E) | 上限内可控，但尚无生产 latency evidence | 有真实读流量后 benchmark/observability |
| 查询计划会随数据/版本变化 | 当前 EXPLAIN 不能永久担保 | schema/query/MySQL 升级时重跑 |
| 没有生产备份恢复演练 | 不具备灾备声明 | 上线前 runbook 与恢复演练章节 |
| 本地长期 Compose 已迁移 | 只证明本次默认实例从 v2 原地到 v5 | 其他环境仍按独立快照、备份、审批与回滚预案发布 |

## 17. 清理与环境影响

### 17.1 已实际完成的 disposable cleanup

- MySQL 容器使用唯一随机 name 和 `com.growthos.acceptance.lesson28=<unique>` label；结束后 exact name 查询为 0、exact label 容器查询为 0；
- Docker `--rm` 的异步删除经过有界轮询，不以 `docker stop` 返回冒充容器已经消失；
- MySQL data 使用 container tmpfs，没有创建 named volume；
- secret directory 只包含本次生成的 root/migration/API/graph password 与 client config；逐个文件删除后 `rmdir`，最终目录查询为 0；
- graph/Strategy fixture、临时 rollback CHECK 与 migration runner probe table 都由测试精确清理并验证零残留；
- 验收前后长期 `growthos` Compose container、volume、network 快照完全相同，脚本没有停止、迁移或修改长期栈；
- 已存在的 `mysql:8.4.11` image 作为后续可复用依赖保留，没有把共享镜像误当一次性 artifact 删除；
- 共享 Go build/test cache 可供后续章节复用，未删除；仓库内没有保留容器日志、密码、coverage、browser trace 或下载图片。
- 隔离 Lottery/cache acceptance 创建的随机 project containers/volumes/networks、task image tags、buildx builder/state、Secret 与 response 文件/目录也按精确 ownership 清除；默认长期 `growthos` 资源保留且 identity 不变。

### 17.2 Final freeze artifact 实际清理

- atomic coverage 使用唯一 `mktemp -d` 目录；`coverage.out` 已逐个 `unlink`，目录已精确 `rmdir`，二者均验证不存在；
- fuzz 没有在仓库内生成 `testdata/fuzz` corpus、`*.test`、`*.prof`、`coverage.out` 或 `fuzz-*`；
- `make verify` 前 `web/dist` 不存在，之后只生成 `index.html` 与 `assets/` 下 5 个带 hash 的 JS/CSS 文件；根代理先用 `find` 枚举，再逐文件 `unlink`、逐目录 `rmdir`，最终验证 `web/dist` 不存在；
- `web/node_modules`、四个 Compose secret 文件、Go build cache、`mysql:8.4.11` 与共享 buildx 基础镜像是既有或可复用依赖，均保留；既有 Vitest `results.json` 被测试刷新，但不是本任务新建，因没有可信前值而未删除；
- 清理后 `git status --short` 为空；`git status --short --ignored` 只显示上述既有 secrets 与 `web/node_modules/`。

本轮没有递归删除用户 cache、长期 Docker volume、依赖镜像、凭证或项目源文件。

## 18. 文档同步状态

- 当前 shell 未配置 `VAULT`；
- 因此没有执行 `make docs-sync VAULT=...`，也不声称个人 Obsidian/外部知识库已经同步；
- Git 仓库内文档是当前唯一可验证事实源；
- 本文落盘后的独立 doccheck 结果应与最终章节索引收口后的 doccheck 区分，后者仍须在 accepted-tip 候选上复跑。

## 19. 当前验收清单

- [x] 产品基线与 ADR 冻结 rooted DAG、三表、create-only 和停止线；
- [x] graph/node/edge 领域模型、资源预算与 strict Restore 已实现；
- [x] aggregate-scoped Creator/Reader port 与稳定错误类已实现；
- [x] Migration 000003～000005 及不可变 checksum/inventory 证据已实现；
- [x] 未装配 MySQL graph adapter 与 bounded explicit queries 已实现；
- [x] mysqlrepo 普通、race、20 轮 shuffle、vet 已实际 exit 0；
- [x] disposable MySQL 8.4.11 gate 已实际 exit 0，六个 integration tests 均 PASS；
- [x] real schema clean exact v5、FK/CHECK/rollback/case/grammar 已验证；
- [x] graph round trip、新 revision、duplicate、concurrent create、MaxUint 与 missing 已验证；
- [x] child failure 0/0/0 rollback 与 dangling logical root fail-closed 已验证；
- [x] RR read-only、write rejection、PRIMARY/no-filesort EXPLAIN 已验证；
- [x] graph identity exact grants、mandatory role 和 forbidden SQL 已验证；
- [x] exact container/label/temp cleanup 与长期 growthos 快照不变已验证；
- [x] 长期 `growthos` 已在同一 MySQL container/named resources 上从 `2:0` 前向到 `5:0`，旧表 `0:0:empty` fingerprint 不变且新三表 `0:0:0`；
- [x] 重复 `make compose-migrate` 为 `no_change` latest 5，`make compose-status` 为 clean v5，`make compose-smoke` exit 0；
- [x] 长期 `growthos_app` graph `SELECT` 真实 1142，零行 graph `INSERT` 探针也被拒绝；
- [x] v5 隔离 Lottery/cache acceptance exit 0，本轮三组 M1 数据与精确 project/artifact cleanup 已记录；
- [x] 已如实记录 BSD sed、async `--rm`、edge filesort 三次真实失败与修正；
- [x] 当前无 `VAULT`，未伪造外部文档同步；
- [x] 本 QA 落盘后当前工作树 doccheck 与 tracked/untracked whitespace diff-check 已通过；
- [x] 完整文档/索引收口后的最终 doccheck；
- [x] final candidate `make verify` 与全仓 race；
- [x] final graph fuzz、atomic coverage 与前端门禁；
- [x] `809d436..HEAD` diff/stop-line/线性历史/clean-worktree；
- [ ] 冻结提交 push 后同名远端 SHA、累计分支 fast-forward 和 main 不变复核。

最终 accepted tip 只以根代理提交并推送后的同名远端实际 SHA 为准。本文不虚构尚未生成的 SHA，也不把 disposable test identity 的成功外推为生产权限系统已经完成。
