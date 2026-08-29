# 第 19 节：实现 Lottery Repository

**状态：** 实现完成，最终章节门禁与提交信息以 QA/分支检查点为准

**日期：** 2026-08-29

**阶段：** 从两张表开始做抽奖

**本节把第 17 节的 `Strategy` 聚合与第 18 节的两张 MySQL 表连接起来：应用层只声明 `Create` / `FindByID` 两个窄端口，MySQL adapter 负责父子事务写入、同一快照聚合读取、领域重建、稳定错误分类和上下文取消。它没有新增 HTTP API、没有实现加权随机算法，也没有 Update、Delete、Save、Upsert、缓存或在线管理后台。**

## 1. 本节真正解决的问题

第 18 节之后，系统同时拥有两类彼此独立的事实：

- Go 内存中的 `domain.Strategy` / `domain.Award` 能保证完整领域不变量；
- MySQL 中的 `lottery_strategy` / `lottery_strategy_award` 能持久保存父子行，并用键、外键和 CHECK 守住一部分结构约束。

但“对象存在”和“表存在”还不等于“聚合可以安全持久化”。中间至少缺少这些决策：

1. 应用真正需要什么持久化能力，是否应该一开始就定义完整 CRUD；
2. Strategy 根和多个 Award 子行如何做到全有或全无；
3. 两个并发创建相同 Strategy 时，谁决定胜负；
4. 两次 SELECT 如何避免读到不属于同一时刻的父子组合；
5. SQL 能接受、但领域不接受的旧数据应该被修复、忽略还是拒绝；
6. deadlock、lock wait timeout、重复键、not found 和连接错误是否应当被调用方同等处理；
7. `Commit` 返回网络错误时，数据究竟有没有落库；
8. 谁拥有 `*sqlx.DB` 连接池，Repository 是否有权关闭它；
9. 应用数据库账号需要新增哪些权限，又必须继续拒绝哪些权限；
10. 单元测试、SQL mock 与真实 MySQL 各自能证明什么。

Repository 的价值不只是把 SQL 放进一个 struct。它是领域对象与持久化事实之间的翻译边界，也是事务、取消、错误和数据损坏语义第一次集中落地的地方。

## 2. 本节之前与之后的能力边界

### 2.1 开始前

- Lottery 领域对象是不可变值对象/聚合，构造后 Awards 按 AwardID 规范排序；
- 两张 InnoDB 表已经存在，Award 使用 `(strategy_id, award_id)` 复合主键；
- 应用账号只有两张业务表的 SELECT；
- API 进程只用 MySQL 做 readiness Ping；
- `/lottery` 页面仍使用 Mock；
- 没有任何业务 SQL、Repository port 或 row mapper。

### 2.2 完成本节后

- 应用层拥有 consumer-owned 的 `StrategyCreator` 与 `StrategyReader` 两个最小端口；
- MySQL adapter 实现 `Create` 与 `FindByID`；
- `Create` 在一个事务内写入父行和全部子行，任一步失败即回滚；
- `FindByID` 在一个 read-only、Repeatable Read 事务中完成两次查询，再从同一快照重建完整聚合；
- 持久化行必须通过 `RestoreAward` / `RestoreStrategy`，脏快照 fail closed；
- Repository 错误拥有可由 `errors.Is` 检查的稳定语义类，同时保留内部 cause；
- 应用账号的精确权限升级为两张业务表各自的 SELECT + INSERT；
- 真实 MySQL 集成测试覆盖原子性、并发创建、取消、一致快照、损坏数据、完整 `uint64` 和执行计划。

### 2.3 完成后仍然没有

- `POST /api/lottery/**`、`GET /api/lottery/**` 或其他 Lottery HTTP route；
- 把 Repository 装配进 `growth-api` 的业务 handler/use case；
- 请求/响应 DTO、HTTP status 与 Repository error 的公开映射；
- 加权随机选择、随机源、draw use case 或最终结果；
- Strategy Update、Delete、Save、Upsert、List 或分页；
- 乐观锁、版本化发布、审计、软删除或配置历史；
- Redis key、TTL、缓存回源、失效或双写；
- Activity、Participation、库存、Benefit 与 DrawResult。

所以本节的“Repository 已实现”指代码边界和数据库行为已经成立，不表示线上用户已经能够创建 Strategy 或发起抽奖。

## 3. 先从使用者需要的端口出发

端口定义在 [`internal/lottery/application/repository.go`](../../../internal/lottery/application/repository.go)：

```go
type StrategyCreator interface {
    Create(ctx context.Context, strategy domain.Strategy) error
}

type StrategyReader interface {
    FindByID(ctx context.Context, id domain.StrategyID) (domain.Strategy, error)
}
```

这里有两个有意的设计点。

### 3.1 接口由 consumer 拥有

应用层是持久化能力的使用者，因此接口放在 application，而不是放在 MySQL adapter 中。未来 use case 可以只依赖自己需要的行为；MySQL 只是当前实现，测试内存仓库或其他存储也只能实现相同语义，不能反过来要求应用接受驱动细节。

依赖方向为：

```text
Lottery application
    │ 只知道 StrategyCreator / StrategyReader
    ▼
Lottery domain

MySQL adapter
    ├─ 依赖 application 端口并提供实现
    ├─ 依赖 domain 做重建
    └─ 依赖 sqlx / go-sql-driver 做 I/O
```

领域包不知道 SQL，应用端口也不暴露 `sql.Tx`、row struct、MySQL error number 或表名。

### 3.2 为什么拆成两个窄接口

调用方可能只读 Strategy，也可能只创建 Strategy。让它们都依赖一个 `StrategyRepository` 大接口，会产生几类没有收益的耦合：

- 只读用例被迫依赖写能力；
- fake 必须实现用不到的方法；
- 接口一旦加入 Update/Delete/List，所有使用者都被动变化；
- 权限审查不容易从端口看出最小能力。

窄接口还能在编译期表达最小依赖：某个函数只收 `StrategyReader`，就没有直接调用 Delete 的能力。

### 3.3 为什么现在不定义完整 CRUD

当前只有两个已被设计和验证的动作：创建一份完整配置，以及按 ID 加载一份完整配置。以下名字看似常见，实际都带着尚未回答的业务语义：

| 方法 | 隐含问题 | 本节结论 |
| --- | --- | --- |
| `Update` | 全量还是局部、并发覆盖如何发现、已发布策略能否改 | 不定义 |
| `Save` | insert 与 update 如何区分、并发语义是否被隐藏 | 不定义 |
| `Upsert` | 重试是否覆盖旧配置、子行如何替换、历史如何审计 | 不定义 |
| `Delete` | 硬删/软删/归档、外键 RESTRICT、已引用策略怎么办 | 不定义 |
| `List` | 过滤、排序、分页、管理视图与聚合大小 | 不定义 |

接口不是对数据表可做操作的镜像，而是当前业务真正需要的协议。

## 4. MySQL adapter 的职责与所有权

实现位于 [`internal/lottery/adapter/mysqlrepo`](../../../internal/lottery/adapter/mysqlrepo)。`Repository` 只保存一个借用的 `*sqlx.DB`：

```go
type Repository struct {
    database *sqlx.DB
}
```

`*sqlx.DB` 代表并发安全的连接池，不是一条独占连接。它通常由 application composition root 创建，并被 readiness、多个 Repository 或其他基础设施共同使用。因此：

- `mysqlrepo.New` 只校验并保存 pool；
- Repository 不提供 `Close`；
- pool 的创建者负责在进程关闭时统一 Close；
- 一次 Repository 调用只临时借用事务/连接，结束后归还池；
- nil pool、nil context 和零值 Repository 都返回明确错误，而不是 panic。

如果 Repository 自己关闭共享 pool，一个局部组件的生命周期就会破坏其他调用者。所有权必须从资源创建点明确，而不能由“谁手里有指针”来推断。

## 5. `Create`：把一个聚合当作一个提交单位

### 5.1 执行顺序

`Create` 的实际控制流如下：

```text
校验 ctx / repository
    ↓
RestoreStrategy 再验证完整聚合
    ↓
BEGIN
    ↓
INSERT lottery_strategy
    ↓ RowsAffected 必须为 1
Prepare 一次 Award INSERT
    ↓
按规范 AwardID 顺序逐个 INSERT
    ↓ 每次 RowsAffected 必须为 1
Close prepared statement
    ↓
提交前检查 ctx.Err()
    ↓
COMMIT
```

任一步在 Commit 前失败，deferred Rollback 都会结束事务；成功 Commit 后再调用 Rollback 只会得到 `sql.ErrTxDone`，其结果被安全忽略。

### 5.2 为什么写入前还要重验 Strategy

`Strategy` 的字段不可导出，正常业务代码只能通过领域构造器得到合法值。但 adapter 仍调用 `RestoreStrategy(strategy.ID(), strategy.Name(), strategy.Awards())`：

- 零值 `domain.Strategy{}` 会被拒绝；
- adapter 不把“这是 typed value”误当成“永远合法”；
- 领域规则仍是持久化入口的最后一道业务门；
- 未来领域结构演进时，adapter 不会绕过新增不变量。

这不是在 Repository 复制验证规则，而是复用领域唯一事实源。

### 5.3 为什么父子必须同一事务

Strategy 的合法状态要求至少一个 Award。若父行和子行分别自动提交，以下失败就会留下领域不可加载的半成品：

```text
父 INSERT 成功并提交
    ↓
第 2 个 Award INSERT 违反约束/超时
    ↓
数据库留下 root + 不完整 awards
```

同一事务把持久化原子边界与聚合边界对齐：

- 父 insert 失败：没有子行；
- 任一子 insert 失败：父行和已写子行一并回滚；
- statement close 或 affected rows 异常：不提交；
- context 在提交前取消：不提交；
- 只有 Commit 成功返回，调用方才能确认整份配置已落库。

真实集成测试临时添加一个只命中特定测试 ID 的 CHECK，让子行必然失败，然后验证父/子行数都为零。这比只 mock `Rollback` 更接近要证明的数据库事实。

### 5.4 为什么不先 `SELECT EXISTS`

“先查不存在，再插入”不能消除并发竞争：

```text
请求 A: SELECT -> 不存在
请求 B: SELECT -> 不存在
请求 A: INSERT -> 成功
请求 B: INSERT -> 冲突
```

最终仲裁者仍是主键。额外 SELECT 只增加一次往返，还容易制造“查过就不会冲突”的错觉。因此 `Create` 直接 INSERT，让 `lottery_strategy.PRIMARY` 在原子操作中裁决身份唯一性。

并发集成测试同时发起两个相同 ID 的 `Create`，要求恰好一个成功、一个 `ErrStrategyAlreadyExists`，最终只有一个完整聚合。

### 5.5 为什么只把根表 1062 归类为 AlreadyExists

根 INSERT 的 MySQL 1062 明确表示 Strategy ID 已存在，因此转换为 `ErrStrategyAlreadyExists`。

Award INSERT 的 1062 不做相同转换。对一个刚刚成功插入的新父行、且已通过领域去重并按 ID 排序的 Strategy，子表重复键不是“这个 Strategy 已经存在”的可靠证据；它更可能表示 schema 漂移、未知约束或实现 bug。把所有 1062 都翻译成 AlreadyExists 会掩盖真正故障，因此子写入的 1062 fail closed 为通用 Repository failure。

### 5.6 稳定顺序、prepare once 与 affected rows

`Strategy.Awards()` 返回 AwardID 升序的防御性副本。Repository 按该顺序写入，而不是继承调用者原始 slice 顺序。稳定顺序的收益包括：

- 并发事务访问多个键时更容易形成一致的锁获取顺序；
- 日志、trace 与测试更可重复；
- 输入 slice 顺序不会偷偷变成持久化业务语义。

Award INSERT 在事务中 Prepare 一次，再对每个 Award 执行；这避免应用层为每一行重复建立 statement 对象。它不承诺某个驱动一定使用服务端 prepared statement，也不把 prepare 当作性能结论，真实收益仍应由规模与 profile 验证。

每次普通 INSERT 都要求 `RowsAffected() == 1`。如果驱动无法报告或出现非预期行数，事务不会被当作成功提交。这里不用多值批量 INSERT，因为当前优先级是逐行故障定位、明确 affected-row 契约和简单事务证明；Award 数量出现真实规模瓶颈时再测量批量方案。

## 6. 失败不只是一个 `error`

应用层定义稳定语义错误，adapter 把驱动/SQL cause 包装为 `RepositoryError`。外部渲染只显示审核过的错误类，可信代码仍可通过 `errors.Is` / `errors.As` 检查类别与 cause。

| 语义错误 | 表示什么 | 是否暗示可重试 |
| --- | --- | --- |
| `ErrRepositoryInvalidArgument` | Repository 调用契约错误，例如 nil context | 否，应修代码 |
| `ErrRepositoryNotConfigured` | adapter 没有可用 pool | 否，应修装配 |
| `ErrStrategyNotFound` | 根查询没有该 ID | 否；这是业务查无结果 |
| `ErrStrategyAlreadyExists` | `Create` 根身份已存在/竞争失败 | 否；不是自动幂等成功 |
| `ErrStoredStrategyInvalid` | 行可读，但无法恢复一个合法聚合 | 否；应告警、隔离和修数据 |
| `ErrRepositoryRetryable` | 当前识别的 1205/1213 临时事务失败 | 只表示可能重试，不负责重试 |
| `ErrCommitOutcomeUnknown` | 写 Commit 返回错误，服务端可能已提交 | 绝不能盲重试 Create |
| `ErrRepositoryFailure` | 权限、schema、scan、连接或未分类存储失败 | 不承诺重试有效 |

安全字符串不会包含 SQL、表名、约束名、host 或驱动消息；cause 仍被保留给日志/诊断边界。当前这些错误是 Go application contract，不是 HTTP status 或公开 JSON error code。

### 6.1 1205 与 1213 为什么只标记、不自动重试

MySQL 1205 是 lock wait timeout，1213 是 deadlock victim。它们通常具有临时性，但 Repository 不知道：

- 上层 request 还剩多少 deadline；
- 整个 use case 除数据库外是否已产生其他副作用；
- 应使用何种退避、抖动和最大次数；
- 创建动作是否有业务幂等键；
- 当前错误是在普通 SQL 阶段还是 Commit 结果不明阶段。

因此 adapter 只返回 `ErrRepositoryRetryable`，由未来 use case 在掌握完整语义时决定是否重试。没有策略的内部紧循环重试会放大数据库争用。

### 6.2 Commit outcome unknown 是独立状态

写事务执行 `COMMIT` 后，客户端可能在收到服务端确认前断线：

```text
客户端发送 COMMIT
    ↓
MySQL 已持久化
    ↓
网络断开，driver 返回 error
```

此时返回普通 failure 并盲目重试 `Create`，第二次很可能得到 duplicate；更一般的写操作甚至可能重复副作用。所以非明确取消语义的写 Commit error 被分类为 `ErrCommitOutcomeUnknown`。

这不是说事务一定提交，也不是说一定没提交；它要求未来调用方通过幂等键、状态查询或对账协议解决不确定性。`Create` 当前不是幂等接口，AlreadyExists 也不能自动解释为“上次请求成功”。

## 7. Context：取消必须贯穿数据库边界

所有 begin、exec、prepare 和 query 都使用同一个调用方 `context.Context`。Repository 不在内部创建一个脱离 request 的背景 context，也不吞掉 `context.Canceled` / `context.DeadlineExceeded`。

写入在 Commit 前额外检查 `ctx.Err()`，减少“前序 SQL 已结束、调用方已取消、仍继续提交”的窗口。Commit 本身不接收 context，因此还必须结合 driver 返回错误判断：

- context 未取消但 Commit 失败：写结果未知；
- context 已取消且事务报告匹配取消或 `sql.ErrTxDone`：返回标准 context error；
- context 已取消但 driver 返回一个无关 Commit 故障：不能用取消掩盖结果未知。

真实 MySQL 测试既覆盖预取消，也用 gap lock 阻塞 Award INSERT，等待 deadline 后验证事务没有留下父子行。它证明取消不是只在函数入口检查一次。

## 8. `FindByID`：读取的是聚合快照，不是两份独立行集

### 8.1 执行顺序

```text
校验 ctx / repository / StrategyID
    ↓
BEGIN READ ONLY, REPEATABLE READ
    ↓
SELECT strategy root WHERE strategy_id = ?
    ├─ no row -> ErrStrategyNotFound
    ▼
SELECT awards WHERE strategy_id = ? ORDER BY award_id
    ↓
提交只读事务，释放连接
    ↓
逐行 RestoreAward
    ↓
RestoreStrategy
    ├─ 合法 -> 返回完整聚合
    └─ 非法 -> ErrStoredStrategyInvalid，不返回半成品
```

领域重建放在事务结束之后：数据库快照已经完整复制到本地 row values，无需继续占用事务连接来执行纯 CPU 校验。

### 8.2 为什么是一个 read-only Repeatable Read 事务

两次独立 SELECT 在 autocommit 或 Read Committed 下可能观察到不同提交点：

```text
读取旧 root
    ↓ 另一个事务修改 root 并新增 Award，随后提交
读取新 awards
    ↓
拼出数据库中从未同时存在过的组合
```

当前使用单个 `sql.LevelRepeatableRead`、`ReadOnly: true` 的事务。第一次 consistent read 建立快照，第二次 consistent read 继续观察同一快照。读取不使用 `FOR UPDATE`，因为配置加载不需要阻塞正常写者。

真实 MySQL 测试在 reader 读完旧 root 后，由另一个事务修改 root 并新增 Award；旧 reader 随后仍读到旧 awards，而一个新的 `FindByID` 能看到新 root + 新 awards。这证明的是实际 InnoDB 快照行为，不只是 mock 调用顺序。

`ReadOnly: true` 表达该事务不应写入的意图，也让数据库/driver 有机会执行相应约束或优化；它不是权限替代品。应用账号仍然必须通过精确 GRANT 限制能力。

### 8.3 为什么使用两次查询而不是 JOIN

评估过两种基本形状：

| 形状 | 优点 | 代价 |
| --- | --- | --- |
| root JOIN awards | 一次 round trip | 每个 Award 重复 root 列；空子集和 not found 区分更绕；mapper 易把一行误当聚合 |
| root 查询 + awards 查询 | 形状直接；not found 明确；父子扫描简单 | 多一次 round trip；必须用事务避免撕裂读取 |

本节选择两次查询，并用同一快照解决一致性。它更贴近“先确认聚合根，再加载其实体集合”的模型，也能把“没有根”与“有根但零 Award 的损坏快照”区分开。

如果未来一份 Strategy 有极高读取 QPS、网络 RTT 成为已测瓶颈，可以重新评估 JOIN、JSON aggregation 或缓存；当前没有证据值得牺牲清晰性。

### 8.4 `ORDER BY award_id` 不是装饰

SQL 表没有隐含稳定顺序。Award 查询显式 `ORDER BY award_id`：

- 结果映射和诊断可重复；
- 与领域的规范 AwardID 顺序一致；
- 复合主键 `(strategy_id, award_id)` 同时支持过滤和顺序；
- EXPLAIN 门禁验证使用 PRIMARY 左前缀，且没有 filesort。

`RestoreStrategy` 仍会排序自己的防御性副本，因此领域正确性不依赖 MySQL 恰好返回某个顺序；SQL order 是 adapter 的确定性和执行计划契约，不是用偶然行序替代领域规则。

## 9. 从 row 恢复领域对象：失败关闭，不做静默修复

第 18 节已经承认数据库 CHECK 只能覆盖完整名称规则的子集。绕过应用的历史脚本或高权限运维仍可能写出：

- root 存在但没有 Award；
- 名称包含数据库允许、Go 领域拒绝的 Unicode 首尾空白；
- Award 名称包含控制字符；
- 单行 Weight 都合法，但总和溢出 `uint64`；
- 未来 schema/代码版本不兼容的 Outcome。

row mapper 不直接填充领域私有字段，也不调用会 trim 输入的 `New*` 来“修好”事实，而是调用：

```text
stored Award rows -> RestoreAward (要求已经 canonical)
                  -> RestoreStrategy (重新检查跨行不变量)
```

任何一项失败都返回 `ErrStoredStrategyInvalid` 和零值 Strategy，不跳过坏 Award、不返回部分聚合、不 trim 后继续，也不在读取路径自动改库。

这样做的理由是：

- 静默 trim 会让内存事实与数据库原值不同；
- 跳过坏子行会改变概率分布；
- 自动修库需要审计、权限、并发和恢复协议；
- 一个无效配置不应进入下一节抽奖算法。

fail closed 会降低坏数据时的可用性，但保护了抽奖正确性。后续应通过告警、离线诊断和受审计修复恢复服务，而不是把损坏配置解释成合法配置。

## 10. 查询与类型映射

Repository row struct 继续用 `uint64` 接收 `BIGINT UNSIGNED`：

```text
strategy_id -> uint64
award_id    -> uint64
weight      -> uint64
name        -> string
outcome     -> string -> domain.AwardOutcome -> Restore validation
```

真实测试覆盖 `math.MaxUint64` 的 StrategyID、AwardID 和 Weight，以及 128 个中文字符。它防止 adapter 中某个临时 `int64`、`int` 或不安全字符串转换重新缩小第 18 节已经建立的存储范围。

SQL 参数始终使用占位符，不拼接 Strategy 名称或 Award 名称；包含单引号、问号、SQL 注释样式文本和分解 Unicode 的 round trip 测试证明值被当作数据处理。测试专用动态约束名只由受控的 base36 字符生成，不来自业务输入。

## 11. 最小权限随真实 SQL 精确升级

本节实际 SQL 只有：

```text
lottery_strategy       : SELECT, INSERT
lottery_strategy_award : SELECT, INSERT
```

所以 `growthos_app` 的 allowlist 从第 18 节 SELECT-only 精确升级为两张表各自的 SELECT + INSERT。仍然没有：

- UPDATE 或 DELETE；
- CREATE、ALTER、DROP、INDEX、REFERENCES；
- `schema_migrations` 的 SELECT/INSERT/UPDATE；
- schema wildcard DML；
- root/Migrator Secret；
- mandatory role 隐式扩权。

Compose 的 `mysql-grants` one-shot 仍先撤销旧 direct grants，再重建并精确比较完整 `SHOW GRANTS`。这对复用 volume 同样生效，不能通过手工给账号加 UPDATE 来绕过本节边界。

“Repository 代码暂时没有调用 UPDATE”不等于账号可以提前拥有 UPDATE。权限根据已经实现并测试的 SQL 增长，未来新增真正的修改用例时再同步端口、事务、grant、负向测试和 ADR。

## 12. 没有缓存是一项明确决策

Strategy 看起来像适合缓存的读多写少配置，但本节没有直接接入 Redis，原因不是忘记，而是缓存协议尚无输入：

- key 是否包含 Strategy version；
- Create 后如何失效或预热；
- miss、空值和损坏快照缓存多久；
- 多实例如何传播失效；
- DB/Redis 双写失败如何恢复；
- 旧配置能否继续用于进行中的 Activity；
- TTL 与业务发布时效如何对应。

先让权威 MySQL Repository 的读写、一致性和错误语义成立，才能在第 24 节根据实际 API 与访问模式设计 cache-aside。现在加入缓存只会把未知语义复制到第二个状态系统。

## 13. 测试金字塔：每一层证明不同命题

### 13.1 领域/adapter 单元测试

普通 Go 测试覆盖：

- nil pool、nil context、零值 receiver；
- MySQL 1205、1213、1062 的分类边界；
- context 取消/超时保留为标准 sentinel；
- write/read Commit 错误的不同分类；
- 安全错误字符串与 cause 链；
- 非 canonical row、空 Awards、溢出等重建失败；
- public `Create` / `FindByID` 的事务调用顺序。

SQL mock 能证明“代码按 Begin → Query/Exec → Commit 的次序调用”，但不能证明 InnoDB MVCC、外键、权限、锁等待或真实 driver 扫描行为。

### 13.2 真实 MySQL 集成测试

[`repository_integration_test.go`](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go) 使用独立 application/Migrator 身份和一次性 schema，覆盖：

- 特殊字符、分解 Unicode 和完整聚合 round trip；
- AwardID 在不同 Strategy 中可以重复；
- 完整 `uint64` 上限；
- 顺序/并发 duplicate create；
- 子行失败导致父子全部回滚；
- 预取消和被 gap lock 阻塞后的 deadline；
- server 端真实 1205 lock-wait-timeout 被分类为 retryable，Repository 返回后整笔事务已回滚；
- 空 Award、Unicode 空白、控制字符、总权重溢出等损坏快照；
- 生产共用的 TxOptions helper 在真实 MySQL 中得到 `REPEATABLE-READ`，read-only 写探针被拒绝；
- 使用同一 TxOptions/query helper 的 Repeatable Read 旧快照与新快照；
- 主键访问与无 filesort 执行计划；
- Repository 完成后共享 pool 仍可 Ping。

Migration 集成测试还验证应用身份恰好拥有两表 SELECT + INSERT，并负向验证 UPDATE、DELETE 和 `schema_migrations` 访问被 MySQL 1142 拒绝。

### 13.3 为什么集成测试有双重 opt-in

该门禁会执行 Migration、写入测试行，并由 Migrator 临时添加/删除测试约束。为了防止误指向共享开发库或用户数据，`make test-integration-mysql` 要求：

```text
GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES=lesson-19-isolated-schema
GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES=lesson-19-isolated-repository
```

并要求 application 与 migration 两组连接指向同一个隔离 schema、使用不同账号。不要把这两个 opt-in 设置在长期全局 shell profile，更不要让测试连接指向生产或个人复用数据库。

## 14. 可执行验证

### 14.1 快速本地门禁

在仓库根目录运行：

```bash
go test ./...
go test -race ./...
go vet ./...
make doc-check
```

### 14.2 Compose 真实启动与权限 smoke

```bash
make compose-up
make compose-smoke
make compose-status
```

应观察到：

- migrate clean version 2；
- `mysql-grants` 成功收敛 SELECT + INSERT allowlist；
- API/Web 健康路径未回归；
- 应用账号不能 UPDATE/DELETE 或访问版本表；
- 未知 `/api/**` 仍返回既有统一 404，而不是 Lottery 业务响应。

### 14.3 隔离 MySQL Repository 验证

先准备一个可丢弃的 MySQL 8.4 schema，以及不同的应用/Migrator 账号，再显式提供：

```bash
export GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES=lesson-19-isolated-schema
export GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES=lesson-19-isolated-repository
export GROWTHOS_TEST_MYSQL_API_ADDRESS=127.0.0.1:3306
export GROWTHOS_TEST_MYSQL_API_DATABASE=growthos_l19_disposable
export GROWTHOS_TEST_MYSQL_API_USER=growthos_app_test
export GROWTHOS_TEST_MYSQL_API_PASSWORD='use-a-disposable-secret'
export GROWTHOS_TEST_MYSQL_MIGRATION_ADDRESS=127.0.0.1:3306
export GROWTHOS_TEST_MYSQL_MIGRATION_DATABASE=growthos_l19_disposable
export GROWTHOS_TEST_MYSQL_MIGRATION_USER=growthos_migrator_test
export GROWTHOS_TEST_MYSQL_MIGRATION_PASSWORD='use-a-different-disposable-secret'
make test-integration-mysql
```

示例只说明变量形状，不应复制示例 secret 到共享环境。测试完成后应删除专用容器/schema/账号等一次性资源；不要删除项目长期 Docker volume 来代替有目标的清理。

## 15. 代码阅读路线

建议按以下顺序阅读与单步调试：

1. [`application/repository.go`](../../../internal/lottery/application/repository.go)：先看调用者真正依赖的能力；
2. [`application/repository_error.go`](../../../internal/lottery/application/repository_error.go)：理解稳定错误类与内部 cause 的分离；
3. [`domain/award.go`](../../../internal/lottery/domain/award.go) 与 [`domain/strategy.go`](../../../internal/lottery/domain/strategy.go)：比较 `New*` 与 `Restore*`；
4. [`adapter/mysqlrepo/repository.go`](../../../internal/lottery/adapter/mysqlrepo/repository.go)：跟踪 Create/FindByID 的 happy path 和每个 early return；
5. [`repository_transaction_test.go`](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)：观察 public 方法的事务顺序与 Commit unknown；
6. [`repository_integration_test.go`](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)：观察真实并发、锁、MVCC、损坏数据与计划；
7. [`lottery_schema_integration_test.go`](../../../migrations/lottery_schema_integration_test.go)：观察权限和 schema 如何共同约束 adapter。

阅读每个 error return 时，可以问三个问题：数据库可能处于什么状态、调用方可以安全做什么、日志边界需要保留什么证据。这比只记住 SQL 文本更接近 Repository 设计的核心。

## 16. 容易产生的错误理解

### 16.1 “Repository 就是 DAO”

本节 Repository 以完整 Strategy 聚合为输入/输出，并让事务边界与聚合一致；它不是给每张表生成一套行级 CRUD。row mapper 和 SQL 是 adapter 内部细节。

### 16.2 “Repeatable Read 能修复脏数据”

隔离级别只保证本次两次读取来自一致快照，不保证快照本身符合领域规则。是否可用仍由 Restore 检查。

### 16.3 “AlreadyExists 等于幂等成功”

相同 ID 不代表相同 payload，也不能证明前一次结果属于当前请求。当前 `Create` 明确 create-only，duplicate 是冲突语义。

### 16.4 “Retryable 表示 Repository 会重试”

它只描述故障类别。实际重试次数、退避和幂等必须由掌握完整 use case 的上层决定。

### 16.5 “ReadOnly 事务等于数据库账号只读”

事务选项约束一次事务意图，GRANT 约束账号能力，两者作用层不同。本节账号因为 Create 需要 INSERT，但仍没有 UPDATE/DELETE。

### 16.6 “有 adapter 就已经有 API”

当前 `growth-api` 没有 Lottery handler/use case 装配。Repository 测试能直接调用 adapter，不代表远程客户端能访问它。

## 17. 本节设计的撤销与演进边界

当前决策不是永久真理。出现以下证据时应重新评估：

- Strategy 的 Award 数量大到逐行 INSERT 或整聚合读取成为已测瓶颈；
- 出现更新/发布并发，需要 version/CAS 和历史策略；
- API QPS/RTT 证明两查询的往返成本不可接受；
- 出现读副本，需要定义 read-after-create 和复制延迟；
- 出现缓存，需要 key/version/失效/损坏数据协议；
- Commit unknown 需要面向调用方的幂等创建与状态查询；
- 数据修复流程需要独立 quarantine/repair 工具；
- 多个应用进程需要不同数据库身份；
- 执行计划或数据分布证明需要新索引。

重新评估必须基于实际用例、测量和失败语义，不是因为某个框架默认提供更多方法。

## 18. 下一节交接：把纯算法放在持久化之后

第 20 节将实现最小加权选择算法。它可以直接消费本节返回的合法 `domain.Strategy`，不需要知道 SQL、事务、MySQL error 或 `sqlx.DB`。

交接时已成立的前置事实：

1. `Strategy.TotalWeight()` 已检查不溢出；
2. Awards 非空、Weight 全为正；
3. Awards 按 AwardID 规范排序；
4. `reward` / `no_reward` 都是正常候选结果；
5. `FindByID` 不会把损坏快照交给算法；
6. 同一次加载得到一个一致父子快照。

第 20 节仍应保持纯计算边界，重点决定随机源注入、区间 `[0,totalWeight)`、累积权重边界、确定性测试和无偏性；不要把数据库查询、HTTP、Redis 或权益发放塞进选择函数。到第 21 节出现真实 Draw use case 与 HTTP 契约时，再装配 Reader、算法和 transport。

## 19. 本节小结

本节完成的不是“写了四条 SQL”，而是一组彼此一致的边界：

- 应用只依赖 Create/FindByID；
- 写事务与 Strategy 聚合原子边界一致；
- 读事务把两次查询固定在同一个一致快照；
- 领域 Restore 对持久化快照再次裁决；
- 稳定错误类区分冲突、损坏、临时失败和结果未知；
- context 取消贯穿阻塞数据库操作；
- pool 生命周期留给创建者；
- SELECT + INSERT 精确权限与真实 SQL 一致；
- mock 证明控制流，真实 MySQL 证明数据库事实；
- HTTP、算法、更新和缓存继续保持未实现。

这使下一节可以放心讨论“如何选择一个 Award”，而不必让概率算法同时承担数据一致性与存储修复责任。

## 20. 关联资料

- [第 19 节 API 记录](../../api/lessons/lesson-19.md)
- [第 19 节 QA](../../qa/lessons/lesson-19.md)
- [第 19 节第一性原理设计手记](../../design-thinking/lessons/lesson-19.md)
- [第 19 节面试问答](../../interview/lessons/lesson-19.md)
- [ADR-0016：Lottery Repository 边界与事务语义](../../decisions/ADR-0016-lottery-repository-boundaries.md)
- [第 18 节：第一次正式业务建表](lesson-18-lottery-schema.md)

## 参考

- [Go：Executing transactions](https://go.dev/doc/database/execute-transactions)
- [Go：Canceling in-progress database operations](https://go.dev/doc/database/cancel-operations)
- [Go `database/sql`](https://pkg.go.dev/database/sql)
- [Go Code Review Comments：Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)
- [MySQL 8.4：Consistent Nonlocking Reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)
- [MySQL 8.4：Transaction Isolation Levels](https://dev.mysql.com/doc/refman/8.4/en/innodb-transaction-isolation-levels.html)
- [MySQL 8.4：COMMIT](https://dev.mysql.com/doc/refman/8.4/en/commit.html)
- [MySQL 8.4：Deadlocks](https://dev.mysql.com/doc/refman/8.4/en/innodb-deadlocks-handling.html)
- [MySQL 8.4：GRANT](https://dev.mysql.com/doc/refman/8.4/en/grant.html)
