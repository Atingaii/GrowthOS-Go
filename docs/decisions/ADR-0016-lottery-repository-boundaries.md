# ADR-0016：Lottery Repository 边界与事务语义

- **状态：** 已接受
- **日期：** 2026-08-29
- **负责人：** GrowthOS 维护者

## 背景

[ADR-0013](ADR-0013-lottery-domain-model.md) 已把 Lottery 最小配置定义为 `Strategy` 聚合：Strategy 至少拥有一个 Award，AwardID 在 Strategy 内唯一，Weight 为正且总和不能溢出，名称与 Outcome 受领域规则约束。[ADR-0014](ADR-0014-lottery-persistence-schema.md) 将该聚合映射为 `lottery_strategy` 和 `lottery_strategy_award` 两张 MySQL 8.4 表；[ADR-0015](ADR-0015-compose-schema-grant-reconciliation.md) 建立了可对复用 volume 生效的精确授权收敛。

第 19 节需要在不引入 HTTP、算法、缓存和策略编辑语义的前提下，建立领域与 MySQL 之间的持久化边界。核心难题不是 SQL 语法，而是：

- application 应依赖表级 CRUD，还是依赖聚合级能力；
- 根行和多个 Award 行怎样共享一个写入原子边界；
- 并发创建同一 Strategy 是否先查后写、upsert 或由唯一键裁决；
- 两次读取如何保证来自同一个父子快照；
- 持久化数据违反完整领域不变量时如何处理；
- not found、duplicate、暂时性事务失败、普通依赖失败和 Commit 结果未知如何区分；
- context 取消如何穿过阻塞 SQL 与 Commit 边界；
- Repository 是否拥有共享连接池；
- 应用账号应增加哪些最小权限；
- mock 与真实 MySQL 各自承担什么验证责任。

本 ADR 固化第 19 节已实现的 Repository 边界。它不决定未来 HTTP DTO/status、Draw 算法、Strategy 更新/删除、缓存或权益发放。

## 已知事实与约束

1. 当前聚合只有 `Strategy` 根与其不可脱离的 Awards；一份合法 Strategy 至少一个 Award。
2. Award 的持久化主键是 `(strategy_id, award_id)`，根主键是 `strategy_id`。
3. Strategy/Award ID 与 Weight 支持完整正 `uint64`；adapter 不能收窄到 `int64`。
4. 数据库 CHECK 不能独立保证 Unicode 完整名称契约、非空 Award 集合或总权重不溢出。
5. 应用运行身份与 Migrator 身份已隔离；grant reconciliation 会删除 allowlist 外的 direct grants。
6. `*sqlx.DB` 是可被多个组件共享的并发连接池，而不是 Repository 的单次独占连接。
7. 当前只有创建完整 Strategy 与按 ID 恢复完整 Strategy 的需求。
8. 当前没有更新、删除、分页、发布版本、幂等键、审计、历史回放或缓存协议。
9. MySQL InnoDB 基线为 8.4.11；默认隔离虽然是 Repeatable Read，但代码不能依赖环境默认值。
10. `database/sql` Commit 返回错误时，客户端不能普遍推断服务端一定已回滚。
11. 取消来自调用方 context；adapter 不应脱离 deadline 继续阻塞数据库操作。
12. Repository 还没有装配进 HTTP handler；进程内 port 与网络 API 是不同兼容边界。

## 目标

1. 让应用层只依赖当前用例所需的最小聚合级能力；
2. 保证创建 Strategy 的根和全部 Awards 原子持久化；
3. 让并发 duplicate 由数据库唯一键确定性仲裁；
4. 保证一次 FindByID 的根和 Awards 来自同一个提交快照；
5. 对 SQL 可读但领域非法的数据 fail closed；
6. 提供稳定、可检查、不会把 SQL/driver 文本直接渲染给外层的错误语义；
7. 保留 context 取消与内部 cause，避免错误分类吞掉关键诊断；
8. 明确写 Commit 结果未知与普通可重试错误的区别；
9. 连接池生命周期仍由 composition root 管理；
10. 数据库权限恰好覆盖已实现 SQL；
11. 用真实 MySQL 验证 MVCC、约束、锁、权限、类型和执行计划。

## 非目标

- 不定义 table-generic DAO/ORM 基类；
- 不实现 Update、Delete、Save、Upsert、List 或分页；
- 不实现 HTTP route、DTO、status/error code 或鉴权；
- 不实现加权随机算法、DrawResult、幂等或权益发放；
- 不引入 Redis、读副本、CQRS、事件溯源或 Outbox；
- 不在读取路径自动修复损坏数据；
- 不承诺一次 Repository 调用具有跨数据库/消息的 exactly-once；
- 不把 Compose 的本地权限机制直接定义为生产部署方案。

## 候选方案矩阵

### 1. Port 形状与所有者

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| adapter 包定义 `Repository` 大接口，包含 CRUD/List | 常见、方法集中 | provider 决定 consumer 依赖；提前承诺未知语义；fake 负担大 | 不采用 |
| domain 包定义持久化接口 | 聚合旁边易发现 | 领域层被持久化用例概念污染；不是所有领域行为都需存储 | 不采用 |
| application 定义一个含 Create/Find 的接口 | consumer-owned；方法仍少 | 只读/只写调用方仍依赖另一半能力 | 可行但不采用 |
| application 分别定义 `StrategyCreator` / `StrategyReader` | consumer-owned；最小能力；易做接口隔离和测试 | 实现类型同时满足两个接口，需要更多小类型名 | **采用** |

### 2. Repository 操作粒度

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| Strategy/Award 表各自 DAO | 行操作简单、ORM 友好 | 调用方必须自己拼事务；可产生半聚合；领域边界丢失 | 不采用 |
| generic `Save(any)` | 表面通用 | 类型/语义弱；insert/update/幂等隐藏；难做权限审查 | 不采用 |
| 聚合级 `Create(Strategy)` / `FindByID` | 事务和一致性与聚合对齐；输入输出保持领域语言 | adapter 需要多行映射与领域重建 | **采用** |

### 3. 创建语义与并发唯一性

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| 父/子各自 autocommit | 实现最短 | 子写失败会留下不完整聚合 | 不采用 |
| 一个事务写父与全部子 | 聚合全有或全无；失败边界清楚 | 事务时间随 Award 数增长 | **采用** |
| `SELECT EXISTS` 后 INSERT | 可提前返回自定义冲突 | 存在 TOCTOU；多一次往返；最终仍靠唯一键 | 不采用 |
| `INSERT ... ON DUPLICATE KEY UPDATE` | 可合并创建/更新 | 偷偷引入 Upsert；payload/历史/并发语义未定义 | 不采用 |
| `INSERT IGNORE` | 重试看似方便 | 会吞掉约束问题和 payload 差异，affected rows 语义变弱 | 不采用 |
| 直接 INSERT，由根主键 1062 仲裁 | 原子、确定、少一次查询 | 调用方需处理 AlreadyExists | **采用** |

### 4. Award 写入方式

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| 每个 Award 单独 Prepare/Exec | 直观 | 重复建立 statement 对象，顺序可能继承输入偶然性 | 不采用 |
| 单条动态多值 INSERT | round trip 少 | 动态 SQL/参数上限；逐行失败定位弱；当前无规模证据 | 暂不采用 |
| 事务内 Prepare once，按 AwardID 稳定逐行 Exec | 控制流清楚；确定顺序；每行 affected rows 可验证 | Award 很多时 round trip 增长 | **采用** |

### 5. 读取形状

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| 一条 INNER JOIN | 一次查询 | 根列重复；空 Awards 会被伪装成 not found；mapper 易混淆行与聚合 | 不采用 |
| 一条 LEFT JOIN | 可区分有根无子 | nullable 子列与重复根映射复杂；仍重复传输根列 | 可行但不采用 |
| 两次无事务 SELECT | 代码简单、扫描清楚 | 两次可能来自不同提交点，产生撕裂聚合 | 不采用 |
| 同一事务先 root、再 `ORDER BY award_id` 读取 Awards | not found 清楚；扫描直接；完整快照可证明 | 多一次查询，需要正确隔离和事务结束 | **采用** |

### 6. 读取隔离

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| 依赖 MySQL 默认隔离 | 配置少 | 环境变化会改变契约，代码意图不可见 | 不采用 |
| Read Committed | 每条语句看到较新数据 | 两次查询可看到不同提交点 | 不采用 |
| Repeatable Read consistent reads | 同一事务两查询共享快照；不阻塞正常 writer | 可能读到事务开始后的旧版本；持有 snapshot 有成本 | **采用** |
| `SELECT ... FOR UPDATE` | 可锁定并准备修改 | 读取配置会阻塞 writer；本节不修改 | 不采用 |
| Serializable | 最强隔离直觉 | 不必要锁/冲突，吞吐成本高 | 不采用 |

### 7. 持久化重建

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| row 直接赋值领域字段 | 快、少校验 | 绕过不变量；领域字段本来也不导出 | 不采用 |
| 使用 `New*` 并 trim/normalize | 可能提高可用性 | 静默改变持久化事实；概率配置可能被悄悄修改 | 不采用 |
| 忽略坏 Award，返回剩余候选 | 页面可能继续工作 | 改变概率分布；返回从未配置的部分聚合 | 禁止 |
| `RestoreAward` + `RestoreStrategy`，任一失败则整个读取失败 | 领域唯一裁决；不修复、不返回半聚合 | 坏数据导致该 Strategy 不可用，需要运维修复 | **采用** |

### 8. 错误边界

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| 原样返回 driver/MySQL error | 诊断直接 | 上层耦合 driver；错误文本泄露 SQL/schema/host | 不采用 |
| 只返回一个 `ErrDatabase` | 公共面简单 | 无法区分 not found、duplicate、损坏、retryable、unknown | 不采用 |
| 稳定语义类 + wrapped cause + 安全 `Error()` | 上层可 `errors.Is`；可信边界保留诊断；渲染安全 | 需要维护分类表与测试 | **采用** |

### 9. 连接池所有权

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| 每个 Repository 自建/自关 pool | 局部封装完整 | 连接池爆炸；配置重复；readiness 与 shutdown 不统一 | 不采用 |
| Repository 接收共享 pool 并提供 Close | 可由局部关闭 | 所有权含糊；一个 adapter 可破坏其他使用者 | 不采用 |
| composition root 创建/关闭 pool，Repository 只借用 | 生命周期唯一；多组件共享；shutdown 明确 | 调用方必须管理资源 | **采用** |

### 10. 数据库权限

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| schema 级 ALL/DML | 后续开发少改 grant | API 可改 migration/未来表；越权面大 | 不采用 |
| 两表 SELECT only | 读取够用 | 无法执行已实现 Create | 不采用 |
| 两表 SELECT + INSERT | 恰好覆盖 Find/Create；不能改删 | 每新增写语义都需同步变更 | **采用** |
| 提前加入 UPDATE/DELETE | 未来方便 | 当前无端口/事务/审计语义，违反最小权限 | 不采用 |

## 决策

### 1. 应用端口

1. 在 `internal/lottery/application` 定义 consumer-owned 的 `StrategyCreator` 和 `StrategyReader`。
2. `StrategyCreator` 只含 `Create(context.Context, domain.Strategy) error`。
3. `StrategyReader` 只含 `FindByID(context.Context, domain.StrategyID) (domain.Strategy, error)`。
4. 不定义 Update、Delete、Save、Upsert、List、分页或事务泄漏接口。
5. MySQL `Repository` 在编译期断言同时实现两个端口。

### 2. Adapter 与 pool

1. MySQL adapter 位于 `internal/lottery/adapter/mysqlrepo`。
2. `New` 接收并借用 `*sqlx.DB`；nil pool 返回 `ErrRepositoryNotConfigured`。
3. Repository 没有 `Close`，不得关闭共享 pool；创建者在 composition root 统一管理生命周期。
4. nil context、零值 receiver 和零 ID 失败关闭，不 panic。

### 3. Create 事务

1. 写入前通过 `RestoreStrategy` 重新验证传入聚合。
2. 在一个事务中先 INSERT `lottery_strategy`，再 INSERT 全部 `lottery_strategy_award`。
3. 不执行 `SELECT EXISTS`；根主键直接仲裁并发身份。
4. 根 INSERT 的 1062 映射为 `ErrStrategyAlreadyExists`；其他位置的 1062 不冒充根冲突。
5. Awards 使用领域给出的 AwardID 规范顺序。
6. Award INSERT 在事务内 Prepare 一次、逐行执行；每次 INSERT 的 affected rows 必须恰好为 1。
7. 任一 Commit 前错误通过 rollback 使父子全无；提交前检查 context。
8. adapter 不自动重试任何 SQL/事务。
9. 1205/1213 在普通操作阶段映射为 `ErrRepositoryRetryable`。
10. 非明确取消的写 Commit error 映射为 `ErrCommitOutcomeUnknown`，保留原 cause；调用方不得盲重试。

### 4. FindByID 一致读取

1. 显式开启 `Isolation: sql.LevelRepeatableRead`、`ReadOnly: true` 的单个事务。
2. 先按主键查询 root；`sql.ErrNoRows` 映射为 `ErrStrategyNotFound`。
3. 再按 `strategy_id` 查询 Awards，并显式 `ORDER BY award_id`。
4. 两次查询不使用 locking read；一致性来自 InnoDB consistent snapshot。
5. 完成 row 扫描并提交只读事务后，再执行纯内存领域重建，缩短事务占用。
6. 先逐行 `RestoreAward`，再 `RestoreStrategy`；任一不变量失败返回 `ErrStoredStrategyInvalid` 和零值 Strategy。
7. 不 trim、不跳过坏行、不返回部分聚合、不在读取路径自动修库。

### 5. 错误 contract

Repository application contract 固定以下语义类：

| 类 | Adapter 判定边界 |
| --- | --- |
| `ErrRepositoryInvalidArgument` | nil context 等调用契约错误 |
| `ErrRepositoryNotConfigured` | nil/零值 pool |
| `ErrStrategyNotFound` | root SELECT 的 `sql.ErrNoRows` |
| `ErrStrategyAlreadyExists` | root INSERT 的 MySQL 1062 |
| `ErrStoredStrategyInvalid` | row 可读但 Restore 失败 |
| `ErrRepositoryRetryable` | 普通操作识别到 MySQL 1205/1213 |
| `ErrCommitOutcomeUnknown` | 无法证明未提交的 write Commit error |
| `ErrRepositoryFailure` | 权限、schema、scan、连接与其他未分类故障 |

`RepositoryError.Error()` 只渲染语义类；`Unwrap` 保留 cause 给可信日志/诊断。context canceled/deadline 尽量保留标准 sentinel。该 contract 不等于 HTTP contract，transport 必须另行显式映射。

### 6. 权限

`growthos_app` 的完整 direct-grant allowlist 更新为：

```text
USAGE ON *.*
SELECT, INSERT ON growthos.lottery_strategy
SELECT, INSERT ON growthos.lottery_strategy_award
```

继续要求 `@@GLOBAL.mandatory_roles` 为空；不授予 UPDATE、DELETE、DDL 或 `schema_migrations` 访问。Compose reconciliation、smoke 和真实权限集成测试必须同步验证完整正/负权限。

## 关键控制流

### Create

```text
domain.Strategy
    │ RestoreStrategy 再验证
    ▼
BEGIN
    ├─ INSERT root ── 1062(root) ──> AlreadyExists
    ├─ Prepare Award INSERT once
    ├─ INSERT Award 1
    ├─ INSERT Award 2 ...
    ├─ affected rows == 1 for every INSERT
    └─ COMMIT
         ├─ success -> durable aggregate confirmed
         └─ driver error without proven cancellation -> outcome unknown
```

### FindByID

```text
BEGIN READ ONLY / REPEATABLE READ
    ├─ SELECT root by PRIMARY
    ├─ SELECT Awards by PRIMARY left prefix ORDER BY award_id
    └─ COMMIT
         ↓
RestoreAward[] -> RestoreStrategy
    ├─ valid -> complete aggregate
    └─ invalid -> stored-invalid, no partial return
```

## 必须保持的实现约束

1. Create 的父、子 SQL 不得移出同一个 transaction handle。
2. 不得在 duplicate create 前添加一个被误认为互斥保证的 SELECT。
3. 不得把 `ErrStrategyAlreadyExists` 改成隐式幂等成功。
4. 不得把所有 MySQL 1062 一律映射成 Strategy already exists。
5. 不得在 Repository 内无界或无退避重试 1205/1213。
6. 不得把不确定的 write Commit error 降级成普通 retryable。
7. FindByID 的两个查询不得拆成两个独立 autocommit 快照。
8. 不得依赖 MySQL 环境默认隔离；事务选项必须显式。
9. Award 查询必须有确定性 `ORDER BY award_id`。
10. 不得绕过 Restore、静默 trim、忽略坏 Award 或返回部分聚合。
11. adapter 不能把 `uint64` 临时扫描/转换为 `int64`。
12. SQL 值必须参数化；业务字符串不得拼接进 SQL。
13. Repository 不得 Close 借入的 pool。
14. 错误公开渲染不得包含 driver/SQL/host/Secret。
15. 新增 SQL 动词前必须同时更新 port/use case、grant allowlist、负向权限测试和 ADR。

## 影响

### 正面影响

- application 依赖当前业务能力而不是 MySQL/CRUD 形状；
- 写事务和聚合边界对齐，不留下 root-only 或 partial-awards 半成品；
- 并发创建由数据库主键原子仲裁，没有 TOCTOU 假保证；
- 两查询读取保持清晰，同时通过 Repeatable Read 得到一致父子快照；
- 持久化损坏不会进入后续抽奖算法或改变概率分布；
- error caller 能区分冲突、损坏、临时故障和结果未知；
- context、cause 与安全公开字符串各自保留正确职责；
- shared pool 生命周期明确，不被局部 adapter 破坏；
- 权限与实际 SQL 精确相等，不为未来 CRUD 预授权；
- 真实 MySQL 测试使 MVCC、锁、约束、unsigned 类型和计划证据可复现。

### 成本

- FindByID 比单 JOIN 多一次数据库 round trip；
- 每个 Award 单独 Exec，数量很大时写入往返会增长；
- Repeatable Read transaction 在调用期间持有 snapshot/连接；
- 错误分类、Commit unknown 与 context race 需要专门测试和维护；
- fail closed 会让损坏 Strategy 不可用，需要独立运维修复；
- 每次增加 Repository 行为都必须同步精确 grants；
- consumer-owned 小接口数量可能增多，需要 composition root 清晰装配。

### 风险

- 未来误把 AlreadyExists 当幂等成功，会丢失 payload/请求身份差异；
- 上层若对 `ErrCommitOutcomeUnknown` 直接重试，可能重复或混淆写结果；
- 长事务或调用方不设 deadline 会占用 pool 和旧版本；
- SQL mock 通过可能制造“已证明数据库语义”的错觉；
- 行时间戳不是聚合 version，不能支撑未来无丢失更新；
- 应用账号虽无 UPDATE/DELETE，但 INSERT 仍能创建业务事实，HTTP 暴露前必须补鉴权/审计；
- 当前 MySQL 8.4 计划断言可能在版本升级后需要重新校准，而不是盲目删除。

## 失败与恢复语义

| 失败点 | 已确认状态 | 调用方安全动作 |
| --- | --- | --- |
| 领域重验失败 | 未 Begin/未写 | 修正调用代码/输入，不重试同值 |
| root duplicate 1062 | 当前 ID 已存在 | 返回冲突；若需幂等必须另查请求身份/payload |
| Award INSERT/statement/affected rows 失败 | deferred rollback；未确认提交 | 记录 cause；按错误类决定，不假装创建成功 |
| 1205/1213（Commit 前） | 当前事务失败/需回滚 | 上层在 deadline、退避、幂等允许时才重试 |
| context canceled/deadline（Commit 前） | 不继续提交，rollback | 结束请求；必要时查询业务状态 |
| write Commit driver error | 可能已提交，也可能未提交 | 标记 outcome unknown；查询/对账，禁止盲重试 |
| root SELECT no rows | 没有该聚合根 | 返回 not found；不查询成空合法 Strategy |
| Awards/Restore 失败 | 不返回部分聚合 | 告警、隔离、用受控工具诊断/修复 |
| read Commit/连接故障 | 没有业务写副作用 | 返回 repository failure/retryable/context；由上层决定读重试 |
| grant/schema drift | SQL 被拒绝或失败 | 阻断启动/操作，修 migration/grants；不扩大 API 权限绕过 |

Repository 不负责自动数据修复，也不把恢复策略藏在 adapter 内部。Commit unknown 的完整解决通常需要未来的 idempotency/operation record/read-after-write 协议。

## 验证策略

### 单元与 mock

- 构造/receiver/context 边界；
- stable error class、safe string 与 wrapped cause；
- 1213/1062 分类与模拟 COMMIT error；
- read/write Commit 分类和取消竞态；
- Restore fail closed；
- public Create/FindByID 的 Begin/SQL/Commit 顺序。

### 真实 MySQL 8.4

- 父子 round trip、特殊字符与完整 `uint64`；
- Strategy-scoped AwardID；
- sequential/concurrent duplicate create；
- 子约束故障后的父子全回滚；
- 预取消、gap-lock context deadline，以及服务端真实 1205 后整笔回滚；
- 空 Awards、非 canonical 名称、控制字符、总权重溢出；
- 生产 `TxOptions` 在服务端体现为 `REPEATABLE-READ`，只读写探针被拒且零残留；
- 使用同一生产 `TxOptions` 与 query helper 的旧快照，以及事务结束后的公开新快照；
- root PRIMARY const lookup、Award PRIMARY left-prefix lookup、无 filesort；
- shared pool 调用后仍可用；
- exact SELECT+INSERT grants，以及 UPDATE/DELETE/version-table 拒绝。

SQL mock 只证明应用控制流，不替代真实 MySQL 对 MVCC、权限、锁和 driver 的证明。

## 演进与撤销方式

本决策可以按新增需求前向演进，但不得偷偷扩大现有方法含义：

- 出现编辑需求时新增独立 ADR，定义 version/CAS、全量/局部替换、发布与审计；不要把 `Create` 改名 `Save` 后隐式 Upsert。
- 出现删除需求时先定义引用、归档、历史和恢复；再新增端口与权限。
- 出现性能证据时可把 Award 写改成批量、读改成 JOIN/聚合，但必须保留原子性、一致快照和 fail-closed，并用 benchmark/EXPLAIN 验证。
- 出现 Redis 时先定义权威源、key/version、失效、miss 与损坏快照，不修改 Repository 错误含义来迁就缓存。
- 出现读副本时定义复制延迟和 read-after-create；不能继续声称强一致可见性。
- 出现创建幂等时新增 request identity/operation record；不要把 duplicate 一律改成成功。
- 更换 MySQL adapter 时，新 adapter 必须满足同一 application port 与语义测试，而不是让 application 接受新存储细节。

## 重评触发器

以下任一证据出现时重新评估本 ADR：

1. 单个 Strategy 的 Award 数量让逐行 INSERT 达到已测性能瓶颈；
2. 两查询 RTT 在真实负载中成为显著延迟来源；
3. 出现 Strategy 更新、删除、发布版本、草稿/审批或并发编辑；
4. 业务需要 read-after-write、读副本或跨地域一致性；
5. `ErrCommitOutcomeUnknown` 需要面向用户的自动恢复；
6. 引入 Redis/cache-aside、CDC 或事件驱动配置分发；
7. Strategy 聚合过大，整聚合加载不再合适；
8. 需要按名称/状态/Activity 查询，而不再只有主键读取；
9. 生产发现新的 MySQL transient error，需要纳入可重试分类；
10. MySQL/driver 升级改变 transaction option、SHOW GRANTS 或执行计划行为；
11. 多个进程需要拆分 read/write 数据库身份；
12. 数据损坏事件要求正式 quarantine/repair 工作流。

## 验收证据

- [Application ports](../../internal/lottery/application/repository.go)
- [Repository errors](../../internal/lottery/application/repository_error.go)
- [MySQL adapter](../../internal/lottery/adapter/mysqlrepo/repository.go)
- [事务控制流测试](../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)
- [真实 MySQL Repository 测试](../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)
- [真实 schema/权限测试](../../migrations/lottery_schema_integration_test.go)
- [Compose grant reconciliation](../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)
- [第 19 节课程](../course/part-03/lesson-19-lottery-repository.md)
- [第 19 节 QA](../qa/lessons/lesson-19.md)

## 参考

- [Go：Executing transactions](https://go.dev/doc/database/execute-transactions)
- [Go：Canceling in-progress database operations](https://go.dev/doc/database/cancel-operations)
- [Go `database/sql`](https://pkg.go.dev/database/sql)
- [Go Code Review Comments：Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)
- [go-sql-driver/mysql](https://pkg.go.dev/github.com/go-sql-driver/mysql)
- [MySQL 8.4：Consistent Nonlocking Reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)
- [MySQL 8.4：Transaction Isolation Levels](https://dev.mysql.com/doc/refman/8.4/en/innodb-transaction-isolation-levels.html)
- [MySQL 8.4：COMMIT](https://dev.mysql.com/doc/refman/8.4/en/commit.html)
- [MySQL 8.4：Deadlocks](https://dev.mysql.com/doc/refman/8.4/en/innodb-deadlocks-handling.html)
- [MySQL 8.4：GRANT](https://dev.mysql.com/doc/refman/8.4/en/grant.html)
