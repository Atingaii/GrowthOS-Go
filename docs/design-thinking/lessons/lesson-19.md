# 第 19 节第一性原理设计手记：让一个聚合在失败、并发与坏数据面前仍然只有一种解释

本文记录的不是“怎样写一个 Repository”的语法教程，而是为什么当前 Lottery 持久化边界只能先长成 `Create + FindByID`，以及每一个看似普通的 SQL 选择背后在防什么失败。

结论只适用于 2026-08-29、实现 `50ac811` 与证据加固 `2c420c9` 的代码时间切片：Lottery 领域对象和两张 MySQL 表已经存在；本节已经实现 application-owned 的窄端口、MySQL `Create` / `FindByID` adapter、恢复构造、稳定错误分类、精确 `SELECT + INSERT` 授权，以及真实 MySQL 8.4.11 与 Compose 验证。Repository 还没有接入 HTTP handler、抽奖算法或运行时业务用例；更新、删除、列表、幂等创建、缓存、审计和生产可观测性也都没有发生。

## 1. 决策命题与时间切片：我们不是在抽象 SQL，而是在定义“一个 Strategy 何时真实存在”

### 1.1 真正需要决定的事情

第 18 节只证明了数据库能保存 Strategy 根行和 Award 子行，也暴露了数据库无法独立证明的跨行/Unicode 事实。此时直接让未来 handler 自己写 SQL，会出现四个没有统一答案的问题：

1. 根行已经写入、第三个 Award 失败时，Strategy 算存在还是不存在；
2. 根查询之后有并发写入、子查询看见新 Award 时，返回的是哪个时间点的 Strategy；
3. 数据库行能被扫描但不能构造成领域对象时，是“忽略坏行”“修一修再返回”还是停止服务；
4. 连接断在 `COMMIT` 附近时，调用方能否把失败当成“肯定没写入”并重试。

所以本节命题是：

> 在不发明更新、删除、列表和在线接口语义的前提下，怎样建立一个由 application 消费者拥有的最小端口，使合法 Strategy 只能整体创建，读取只能返回同一数据库快照中的完整聚合，存储坏数据必须失败关闭，SQL/driver 细节不能泄露到不可信输出，同时对重复、取消、锁等待、死锁和提交结果未知给出不说谎的分类？

Repository 的价值不在于消灭 SQL。它负责把两种不同世界对齐：

- 领域世界中，Strategy 是一个有至少一个 Award、总权重不溢出、名称规范、AwardID 不重复的完整对象；
- 关系世界中，Strategy 是两张表里的多行，任意一行都可能缺失、陈旧、越权写入或在读取期间变化。

### 1.2 本节已经进入实现的时间切片

- `StrategyCreator.Create(ctx, strategy)`；
- `StrategyReader.FindByID(ctx, id)`；
- 端口定义在 `internal/lottery/application`，MySQL 实现在 `internal/lottery/adapter/mysqlrepo`；
- 根行与所有 Award 在同一写事务中插入；
- 子项按领域规范化后的 AwardID 稳定顺序写入；
- 根表和子表用两个参数化查询读取，并共享一个 `REPEATABLE READ`、read-only 事务；
- 事务结束后使用 `RestoreAward` / `RestoreStrategy` 重建领域对象；
- not found、already exists、stored invalid、retryable、commit outcome unknown、dependency failure 等稳定错误类；
- MySQL 1205/1213 只标记可重试，不在 adapter 内自动重试；
- 应用身份只拥有两张业务表的精确 `SELECT, INSERT`，没有 UPDATE、DELETE、DDL 或 `schema_migrations` 权限；
- unit、sqlmock 控制流、真实 MySQL 8.4.11 集成与 Compose smoke 证据。

### 1.3 尚未发生的时间切片

- API composition root 没有实例化或注入该 Repository；
- `/api/lottery` 不存在；
- 没有抽奖算法消费 `StrategyReader`；
- 没有运营创建 Strategy 的 application use case；
- 没有 Update/Save/Upsert/Delete/List；
- 没有草稿、发布、版本、乐观锁、审计或 outbox；
- 没有请求级幂等键和提交未知后的自动对账；
- 没有 Repository metrics、trace span、慢查询告警或生产 SLO；
- 没有读副本、分库、缓存或批量导入。

“代码可被未来用例调用”与“运行时已经提供业务功能”是两个事实，不能混写。

### 1.4 可重放的推导主链

```text
领域把 Strategy 定义为聚合
→ 一次持久化不能留下只有根或部分 Award
→ Create 必须使用一个事务
→ 创建语义不允许覆盖既有身份
→ 根 INSERT 1062 才能分类为 StrategyAlreadyExists
→ 子 INSERT 的 1062 不能冒充根已存在
→ 输入 Award 已有领域规范顺序
→ 按 AwardID 稳定写入，减少并发锁顺序差异并提高可复现性

聚合跨两张表
→ 读取至少需要组合根和子项
→ 两次普通 autocommit SELECT 可能看见两个时间点
→ 两个查询必须共享一次一致性快照
→ MySQL REPEATABLE READ 在首个 consistent read 建立快照
→ 根查询与子查询置于同一个 read-only RR 事务
→ 查询结果仍不可信
→ 事务结束后用 Restore 构造器 fail closed

COMMIT 是数据库耐久性边界
→ driver 返回错误不等于服务端一定未提交
→ 写提交错误不能归为普通 dependency failure
→ 返回 CommitOutcomeUnknown
→ 禁止 adapter 盲目重试
```

## 2. 不可争辩的事实与约束：先分清代码事实、平台事实和愿望

### 2.1 当前代码事实

| 事实 | 当前能推出什么 | 不能推出什么 |
| --- | --- | --- |
| application 只定义 `StrategyCreator` / `StrategyReader` | 当前消费者只承诺 Create 与 FindByID | 永久不需要 Update/List |
| interface 与 MySQL adapter 分包 | application 不依赖 `sqlx`/MySQL error | 已实现跨数据库可移植性 |
| `Create` 开一个事务插根和全部 Award | 正常返回成功前，写入是聚合级提交 | 所有外部副作用也在该事务内 |
| `Create` 再调用 `RestoreStrategy` | 零值/被错误组装的值不能越过边界 | 任意未来领域规则都会自动被数据库表达 |
| 根 INSERT 1062 单独映射 already exists | 该 StrategyID 已被数据库唯一性拒绝 | 重试一定安全或既有数据内容相同 |
| 子 INSERT 的 1062 走 generic failure | adapter 不谎称“根 Strategy 已存在” | 子 1062 的根因已被自动修复 |
| `FindByID` 使用 RR + read-only transaction | 两个普通 SELECT 在一个 InnoDB consistent snapshot 内 | 读的是调用瞬间绝对最新状态 |
| 根先查、Award 后查且 `ORDER BY award_id` | 首次一致性读建立时间点；子项顺序可复现 | 物理存储顺序或业务展示顺序等于 AwardID |
| hydration 在读事务提交之后执行 | 数据库连接尽早释放，领域校验不占事务 | 坏数据在 SQL 阶段就被数据库拒绝 |
| `RepositoryError.Error()` 只渲染语义类，`Unwrap()` 保留 cause | 非信任输出可稳定脱敏，可信诊断仍能 `errors.Is/As` | 调用者把 cause 打到公开日志仍然安全 |
| 1205/1213 映射为 retryable | 上层能识别瞬时事务冲突候选 | adapter 已经重试，或每个 retryable 都应该重试 |
| 写 Commit 的非取消错误映射 outcome unknown | 不把未知状态说成确定回滚 | 已完成自动对账/幂等 |
| Repository 借用 `*sqlx.DB` 而不 Close | 连接池所有权留在 composition root | 当前运行时已经正确注入与关闭 |
| grants 是两表 `SELECT, INSERT` 精确集合 | adapter 所需语句最小可执行 | 应用身份有更新、删除、版本表或 DDL 能力 |

### 2.2 业务事实

- Strategy 是 Lottery 配置聚合，不是营销 Activity，也不是一次 Draw；
- Award 属于 Strategy，AwardID 的身份范围在 Strategy 内；
- 一个可用 Strategy 必须至少含一个 Award；
- Create 的普通含义是“一个新身份第一次成为持久事实”，不是“把期望状态同步到数据库”；
- `reward` 和 `no_reward` 都是正常候选，不是成功/异常二分；
- 当前没有编辑、删除、分页、搜索、草稿、发布或审批需求；
- 当前没有证据表明调用者可以把重复 Create 当成幂等成功；
- 当前没有外部消息、Benefit 发放或 DrawResult 需要与配置创建原子提交。

### 2.3 MySQL 与 Go 平台事实

- InnoDB 普通 SELECT 在 `REPEATABLE READ` 中使用一致性非锁定读；同一事务的第一次一致性读建立快照，后续一致性读复用它；
- `READ COMMITTED` 为每次一致性读建立新快照，因此不能天然保证两查询看到同一聚合时间点；
- locking read 会引入锁等待、死锁和对写者的阻塞，它解决“我要基于读取继续写”的问题，不是只读配置加载的默认答案；
- `SERIALIZABLE` 提供更强并发限制，但会扩大锁与吞吐成本；当前读取没有需要串行化的业务副作用；
- `database/sql` 的 `Tx` 绑定单一连接；事务内不应绕过 `Tx` 再直接调用 `DB`；
- `context` 可以取消等待中的数据库操作，但网络断开、driver 行为和服务端事务状态之间仍存在不可消除的边界；
- `Commit` 返回错误时，不能一般化地断言事务一定没有提交；
- MySQL 1062 只描述某个唯一约束冲突；业务分类取决于哪条语句、哪个身份语义触发它；
- MySQL 1205 与 1213 分别代表锁等待超时与死锁类失败；整个事务可能需要由完整用例边界重新执行；
- Go 的 interface 由消费者所需行为定义更容易保持窄小；接口位置不是“基础设施文件应该放哪里”的审美问题，而是依赖方向问题。

### 2.4 仍待验证的假设

1. 单个 Strategy 的 Award 数量足够小，可以整体读入内存并逐行 INSERT；
2. MySQL 是单一权威写主库，根和子查询不会被路由到不同副本；
3. 当前复合主键既能按 StrategyID 过滤，也能满足 AwardID 排序；
4. Strategy 创建量和冲突率不足以要求批量 INSERT 或专门的重试协调器；
5. 上层一定会给 Repository 传有界 context；代码接受无 deadline 的 `Background`，并没有强制这一点；
6. driver/MySQL 8.4.11 对 read-only RR transaction 的实现与当前真实测试环境一致；
7. 数据库坏行是异常恢复事件，不是合法“草稿”状态；
8. 精确 grants 的运维成本低于通配权限带来的长期攻击面；
9. 领域的 AwardID 排序仍是未来算法所需的规范顺序；
10. 提交结果未知目前可以交给上层人工/显式对账，而不需要本节发明幂等协议。

### 2.5 本节禁止越界的说法

- 不得说“实现了通用 CRUD Repository”；
- 不得说“Repository 已接入线上 API”；
- 不得说“REPEATABLE READ 总能读到最新数据”；
- 不得说“read-only 事务本身就是数据库权限控制”；
- 不得说“所有 1062 都是 Strategy 已存在”；
- 不得说“`ErrRepositoryRetryable` 会自动重试”；
- 不得说“context 取消证明数据库一定没有提交”；
- 不得说“Commit 返回错误证明写入失败”；
- 不得说“sqlmock 证明了 MySQL 隔离级别语义”；
- 不得说“EXPLAIN 的一次结果证明生产数据规模下永远使用相同计划”；
- 不得说“真实 MySQL 集成测试证明生产 TLS、复制、故障切换和性能”；
- 不得说“错误脱敏后可以无条件打印 wrapped cause”。

## 3. 为什么现在做、为什么不多做：只跨越已经出现的业务缝隙

### 3.1 为什么 schema 之后必须出现 Repository

领域对象在内存中合法、表结构在 MySQL 中合法，仍不等于两者之间的映射合法。第 18 节留下的物理缝隙包括：空根行、跨行总权重溢出、SQL 接受而 Go 拒绝的 Unicode、读期间并发变化，以及父子部分写入。

如果下一节算法直接接收 `*sqlx.DB`，算法就同时负责概率与一致性；如果 handler 自己拼 SQL，HTTP 层就会决定坏数据是否忽略；如果每个用例各写一套 mapper，错误语义和事务边界会漂移。Repository 因而在这个时点出现：不是为了“分层好看”，而是已经有一个无法由领域或 schema 单独承担的翻译责任。

### 3.2 为什么恰好是 Create + FindByID

`Create` 回答最小写入问题：如何让一个新聚合第一次完整成为事实。`FindByID` 回答最小读取问题：如何为后续算法按明确身份加载一个完整配置。两者形成可验证闭环：合法对象可以保存，再从持久化快照恢复为等价对象。

没有实现其他方法，是因为它们不是语法变体，而是新业务命题：

- Update 需要冲突检测、聚合版本、替换/增删 Award 的原子协议；
- Upsert 需要回答同 ID 不同内容究竟是覆盖、拒绝还是幂等；
- Delete 需要审计、引用、历史 Draw 和 Benefit 语义；
- List 需要筛选、分页、排序、投影和容量上限；
- Save 会故意模糊 Create/Update 的不同失败语义。

“先把方法补齐以后总会用到”会把尚未决策的产品语义伪装成基础设施便利。

### 3.3 为什么 interface 放在 application

端口描述的是用例要什么，不是 MySQL 能做什么。放在 application 有三项具体收益：

1. application 只依赖 domain 类型和 `context`，不依赖 `sqlx`、driver error 或表结构；
2. 一个未来用例只拿到 Reader 或 Creator，不会因持有全能 CRUD 接口而越权；
3. adapter 通过编译期断言证明满足消费者契约，替换实现不需要让 domain 知道数据库。

把 interface 放到 adapter 会让消费者依赖基础设施；放到 domain 会把数据库用例能力误写成领域概念；只注入具体 `*mysqlrepo.Repository` 则把 application 与 MySQL 绑定。当前窄端口是依赖倒置的最小表达，不是为“以后换数据库”提前支付一切可移植成本。

### 3.4 为什么不引入 ORM

当前真正困难的部分是事务、快照、错误号、commit uncertainty 和坏数据重建；ORM 不会消除这些问题，只可能把它们藏进默认值。四条固定 SQL 很短，`sqlx` 能提供参数绑定和结构扫描，又保留 SQL/EXPLAIN 可审查性。

这不是“ORM 永远不好”。当查询投影大量增长、动态筛选成为主需求、团队维护手写 mapper 的缺陷率上升时，生成式 query 工具或 ORM 都应重新比较。当前没有数据支持先引入 entity lifecycle、lazy loading、自动 upsert 或隐式事务。

### 3.5 为什么暂不接入 handler 与算法

Repository 自己可以被独立证伪。若同时接 API，HTTP 状态、JSON 大整数、认证和 request deadline 会混入验收；若同时接算法，随机源、公平性和无偏采样会掩盖持久化问题。

本节先固定“输入/输出是完整 Strategy，以及失败有哪些诚实分类”。下一节算法才能假定自己拿到的是一个合法快照，而不是在随机路径里补数据库校验。

### 3.6 为什么权限必须在本节同步扩大

第 18 节 app 身份只有 SELECT；本节出现真实 INSERT，因此继续只读会让代码在宽权测试身份下通过、Compose 运行身份却失败。授权按实际语句从 `SELECT` 增长为 `SELECT, INSERT`，仍不给 UPDATE/DELETE/DDL/版本表访问。

代码能力与账号能力必须同节演进，否则“最小权限”就只是一句文档，而不是能执行的部署契约。

## 4. 从第一性维度推导需求：事实 → 风险 → 需求 → 机制 → 证据

### 4.1 事实源：数据库行不是领域对象

**事实：** MySQL 保存列值，领域构造器定义 Strategy/Award 完整语义；高权限脚本、历史版本、临时约束变化或未来 bug 都可能留下当前领域拒绝的行。

**风险：** mapper 直接填充私有对象、静默 trim 名称、跳过坏 Award 或把空子集当正常 Strategy，会把一个数据库异常伪装成可抽奖配置。

**需求：** 每次越过 persistence→domain 边界都必须重新验证；恢复不能修改权威存储事实；任何一行非法都不返回部分聚合。

**机制：** `RestoreAward` 检查 canonical name、ID、Weight、Outcome；`RestoreStrategy` 检查根、至少一子、重复 ID、总权重溢出并防御性复制/排序。Repository 将任意失败包装为 `ErrStoredStrategyInvalid`，且返回零值 Strategy。

**证据：** unit 测试拒绝非规范 NBSP 名称；真实 MySQL 集成构造空 Awards、根名称非规范、Award 控制字符和总权重溢出四类坏快照并断言 fail closed。

**证据边界：** 这不能发现所有语义污染，例如运营把两个合法名称填反；构造器只能验证明确规则，不能推断业务意图。

### 4.2 写故障域：聚合不能部分存在

**事实：** 一次 Create 需要一条根 INSERT 和 N 条 Award INSERT。

**风险：** autocommit 下根成功、子失败会留下空或不完整 Strategy；后续 Find 可能把它判为损坏，恢复人员又无法知道原始意图。

**需求：** Create 成功时全部持久；任何 commit 前失败时全部回滚。

**机制：** 单一 `sql.Tx` 包含根与全部子 INSERT；defer rollback 兜底；每条语句检查 `RowsAffected == 1`；prepared child statement 关闭成功后才尝试 commit；commit 前再次检查 context。

**证据：** 真实 MySQL 测试临时增加只拒绝指定测试 Strategy/Award 的 CHECK，使根 INSERT 成功、子 INSERT 失败，随后验证根/子计数都为零并删除测试约束。

**证据边界：** 该注入证明 commit 前失败的原子回滚，不证明主机在 fsync、网络 ACK 或 commit 返回附近崩溃时的所有结果；那正是 outcome unknown 的来源。

### 4.3 创建身份：重复不是“成功过一次”的充分证据

**事实：** 根表 StrategyID 是聚合身份，子表唯一性是 `(strategy_id, award_id)`。

**风险：** 把任意 1062 映射 already exists，会把坏 mapper、触发器/约束漂移或子身份冲突谎报为“这个 Strategy 已存在”；先 SELECT EXISTS 再 INSERT 则有 TOCTOU 竞争窗口。

**需求：** 以数据库唯一约束裁决并发，只在根 INSERT 的 1062 声明 `ErrStrategyAlreadyExists`；子 INSERT 1062 保持 generic failure。

**机制：** 根 INSERT 使用专用 `classifyRootInsertError`；Award 写入使用通用分类；没有预查询，没有把 duplicate 当幂等成功。

**证据：** 顺序重复 Create 返回 already exists；两个 goroutine 并发创建同一 Strategy，真实 MySQL 下恰好一个成功、一个 duplicate，最终只有一个根和完整子集；unit 测试证明通用 1062 不会被映射为根 already exists。

**证据边界：** 这不证明重复请求的业务幂等性。第一次提交结果未知后再次 Create 得到 duplicate，也不能自动证明已有聚合与请求内容相同。

### 4.4 稳定顺序：不是为了美观，而是压缩并发状态空间

**事实：** domain Strategy 已按 AwardID 规范化 Awards；数据库复合主键也以 `(strategy_id, award_id)` 排序。

**风险：** 相同聚合若按调用者 slice 的偶然顺序写入，会让 SQL trace、失败位置和并发锁获取顺序不稳定，增加死锁分析与复现成本。

**需求：** 相同领域状态产生相同子写入顺序；读取也产生规范化顺序。

**机制：** Create 再用 `RestoreStrategy` 得到规范顺序，依次执行 prepared INSERT；Find 子查询显式 `ORDER BY award_id`，领域恢复仍再次排序防御。

**证据：** 领域测试与 round-trip 比较验证顺序独立于输入；EXPLAIN 证明当前 MySQL 8.4.11 下子查询走 PRIMARY 的 `strategy_id` 左前缀且无 filesort。

**证据边界：** 稳定顺序只能降低某些锁顺序差异，不能消灭所有死锁；它也不是运营展示顺序、概率顺序或物理行顺序。

### 4.5 读取时间：两条 SQL 必须回答同一个时间点

**事实：** 聚合根和 Award 分表保存；一次读取需要组合两个结果集。

**风险：** 若两条查询各自 autocommit，根可能来自修改前，Award 来自修改后，形成数据库从未以完整聚合存在过的混合状态。

**需求：** 两条普通 SELECT 使用同一一致性快照，不为了只读加载阻塞写者。

**机制：** `FindByID` 显式开启 `sql.LevelRepeatableRead`、`ReadOnly: true` 的事务；先读根以建立一致性快照，再读 Award；只用事务 handle 查询；commit 后才 hydrate。

**证据：** 真实 MySQL 测试在 reader 读取旧根后，由另一事务更新根并新增 Award并提交；旧 reader 的第二个查询仍只看到旧 Award，新 `FindByID` 则看到新根与两个 Award。sqlmock 测试另外约束公开 `FindByID` 的 Begin→根 query→子 query→Commit 顺序。

**证据边界：** 为了确定性插入并发写，真实 snapshot 测试直接调用了与公开方法共用的查询 helper；它证明 MySQL RR 与这些查询的组合语义。sqlmock 只证明公开控制流，不能验证 TxOptions 或 InnoDB MVCC。两者互补，但仍不等于生产读副本/故障切换行为。

### 4.6 隔离级别：强度不是越高越好

**事实：** 当前 Find 只读一个配置聚合，不基于它在同一事务写入。

**风险：** `READ COMMITTED` 太弱会让两个查询使用不同快照；locking read/`SERIALIZABLE` 太强会让普通读取持锁、阻塞编辑并增加死锁/尾延迟；`READ UNCOMMITTED` 可见未提交数据。

**需求：** 保证同一聚合快照，同时保持非锁定读。

**机制：** 选择 read-only `REPEATABLE READ`，而不是依赖 server 默认隔离级别；不加 `FOR UPDATE`/`FOR SHARE`。

**证据：** 真实并发 snapshot 探针与 MySQL 官方 consistent-read 语义一致。

**证据边界：** 如果未来读取后要执行基于当前版本的更新，或业务要求绝对最新/线性一致，本结论失效，需要版本 CAS、锁定读或其他一致性协议，而不是继续复用只读 Find。

### 4.7 时间预算与取消：调用者拥有耐心，adapter 传播而不发明

**事实：** SQL 可能等待连接、锁、网络或结果行；HTTP/任务调用者最了解总预算。

**风险：** 使用 `context.Background()` 或 adapter 内固定长重试会越过请求预算；只在事务开始前检查 context 又不能取消中途锁等待。

**需求：** 每个 Begin/Exec/Prepare/Query 都使用同一个 caller context；commit 前检查已经发生的取消；标准取消错误保持 `context.Canceled` / `DeadlineExceeded` 可识别。

**机制：** 所有数据库调用使用 Context 版本；nil context 被稳定拒绝；通用分类先识别 wrapped cancellation；真实测试用 gap lock 让 Award INSERT 等待，2 秒 deadline 后验证回滚且无残留。

**证据：** 预取消 Find/Create 与 in-flight blocked Create 都返回标准 context 错误；共享 pool 之后仍可 Ping。

**证据边界：** context 不是提交结果证明；若 driver 已发送 COMMIT 而响应丢失，之后 context 恰好取消也不能把任意 driver 错误降格为 canceled。代码只在事务错误与 context error 匹配或为 `sql.ErrTxDone` 时返回取消，否则写提交仍是 outcome unknown。

### 4.8 提交语义：宁可承认未知，也不制造错误确定性

**事实：** Commit 是客户端请求与服务端耐久状态之间的分布式边界；连接可能在服务端提交后、客户端收到响应前断开。

**风险：** 将所有 commit error 包为 ordinary failure 会诱导调用者盲重试，可能得到 duplicate，或在未来副作用场景造成重复处理；将所有“context 已取消”都当确定回滚也会掩盖已提交可能。

**需求：** 写 commit 非确定取消错误必须单独表达 `ErrCommitOutcomeUnknown`；保留 driver cause 给可信诊断，但公开文本不能泄露细节。

**机制：** `classifyWriteCommitError` 的默认分支是 outcome unknown；`RepositoryError` 支持 `errors.Is` 语义类与 wrapped cause；没有自动重试。

**证据：** sqlmock 让公开 Create 的 Commit 返回 driver error，断言 outcome unknown、cause 可追踪、渲染文本只有安全类；unit 测试覆盖“context 同时取消但 driver 返回不匹配错误”仍保持 unknown。

**证据边界：** mock 能稳定触发分支，却不能制造真实 TCP 半断、mysqld 崩溃和 redo durability 的每种时序；生产仍需带身份的对账流程。

### 4.9 错误与隐私：给调用者语义，不给攻击者拓扑

**事实：** driver error 可能包含表名、constraint、SQL、地址或实现细节；调用者又需要区分 not found、duplicate、corruption、retryable 和 unknown outcome。

**风险：** 原样返回会泄露；全部转成一个字符串则使恢复、指标和策略无法区分；仅用字符串比较又脆弱。

**需求：** 稳定错误类可 `errors.Is`；原因可在可信边界 `errors.As/Unwrap`；`Error()` 只能渲染审核过的类；未知/零值错误 fail closed。

**机制：** application 定义 `RepositoryError` 和八类 sentinel；unknown class、zero value、typed nil 都表现为 `ErrRepositoryFailure`。

**证据：** unit 测试检查 class/cause 双重可追踪、字符串不含 driver detail，以及未知类/零值/typed nil 的 fail-closed 行为。

**证据边界：** 类型设计不能阻止调用者显式 `%+v`、递归 Unwrap 或把 cause 作为日志字段；日志策略和遥测脱敏仍需在 composition/use-case 层落地。

### 4.10 权限：让“当前代码需要什么”成为可执行 allowlist

**事实：** Repository 当前只发出两表 SELECT 和 INSERT；Migration 使用独立身份。

**风险：** schema wildcard 或 CRUD 权限让 SQL 注入/未来 bug 可更新、删除、篡改版本表；只追加 GRANT 会让旧 volume 保留历史扩权。

**需求：** 每次启动先撤销旧直接权限，再只授当前语句集；验证完整 grants 而不是 contains；排除 mandatory role 隐式扩权。

**机制：** socket-only grant job 对 app 身份执行 `REVOKE ALL`，授两表 `SELECT, INSERT`，精确比较排序后的 `SHOW GRANTS` 并要求 `@@GLOBAL.mandatory_roles` 为空；API 仍不能 UPDATE/DELETE/schema_migrations。

**证据：** schema integration 和 Compose smoke 都验证正向 SELECT/INSERT、负向 UPDATE/DELETE/版本表访问及 exact grants；复用 volume 后 reconciliation 仍通过。

**证据边界：** 这证明 fixed Compose topology 的直接权限集合，不证明生产 IAM/proxy/role/备份账号、运行中人工 GRANT 或数据库主机权限。

### 4.11 发布、可逆性、可观测性与学习成本

**事实：** schema latest 仍为 2，Repository 是代码/权限能力增长，不需要新表；本项目要求章节可逐步学习和独立验证。

**风险：** 代码先发布但权限没扩会运行失败；权限先给得过宽会扩大攻击面；把 Repository、API、算法、缓存同时发布会无法定位哪层破坏聚合语义。

**需求：** Migration clean→grant reconcile→API gate 保持不变；权限只增 INSERT；Repository 在独立章节完成真实 DB 验证；失败至少有稳定 semantic class 可供后续指标使用。

**机制：** Compose 镜像/labels 切到 lesson 19，grant job 成功后 API 才启动；分离 unit/mock/MySQL/Compose 证据。

**证据：** 当前 MySQL 8.4.11 isolated integration 与默认/独立 Compose smoke 已通过。

**证据边界：** 还没有 Repository 调用指标、事务时长、pool wait、错误分类计数、trace 或告警；本节只是提供可观测语义原料，不能声称生产可观测性完成。

## 5. 备选方案矩阵：被排除的都是能做出来的真实方案

### 5.1 端口形态与所有权

| 方案 | 优点 | 当前风险/代价 | 当前结论 | 重评触发器 |
| --- | --- | --- | --- | --- |
| application-owned `Creator` + `Reader` | 消费者接口最小，依赖不指向 MySQL | 方法多时可能产生多个小 interface | **选择** | 用例组合证明接口切分造成大量重复适配 |
| domain 层 `StrategyRepository` | DDD 项目常见，名字统一 | 把用例能力/持久化关注带入纯领域；易长成 CRUD | 不选 | Repository 行为本身成为明确领域概念且无基础设施词汇 |
| adapter 层定义 interface | 与实现靠近 | application 反向依赖 adapter | 不选 | 仅 adapter 内部测试 seam，不作为应用端口 |
| 直接注入 `*mysqlrepo.Repository` | 最少类型 | 锁死 MySQL/sqlx、授予消费者所有方法 | 不选 | composition root 内部构造可以具体，但不穿越 application 边界 |
| 一个 CRUD mega-interface | 看似统一、mock 方便 | 未用方法、过权、Update/Save 语义被提前发明 | 不选 | 不存在；应由真实消费者组合小接口，而非预建大全 |

### 5.2 SQL 工具

| 方案 | 优点 | 当前代价 | 当前结论 | 重评触发器 |
| --- | --- | --- | --- | --- |
| `database/sql` + `sqlx` 手写 SQL | SQL、事务、EXPLAIN、error number 都显式 | mapper/列名需人工维护 | **选择** | 查询数量和投影显著增长、人工漂移成为主要缺陷源 |
| 全功能 ORM | entity mapping、动态查询生态 | 隐式 upsert/事务/lifecycle，难表达 commit uncertainty 和窄 SQL | 当前不选 | 真实复杂查询/关联维护收益超过默认行为审计成本 |
| sqlc/代码生成 query | 编译期查询类型，SQL仍显式 | 增加生成链、配置、生成物审查 | 暂不选 | 稳定 query 数量上升、scan 重复明显、团队接受生成供应链 |
| 自建通用 DAO/反射 mapper | 可以统一样板 | 重造 ORM，错误/类型边界更不透明 | 不选 | 无合理触发器；优先成熟工具 |

### 5.3 Create 语义

| 方案 | 并发与失败 | 业务含义 | 当前结论 |
| --- | --- | --- | --- |
| `INSERT` only，由唯一约束裁决 | 并发确定；duplicate 明确 | 第一次创建新身份 | **选择** |
| `SELECT EXISTS` 后 INSERT | 有 TOCTOU，仍需处理 1062 | 没有额外价值 | 不选 |
| `INSERT ... ON DUPLICATE KEY UPDATE` | 抹平 duplicate，可能无意覆盖 | upsert | 不选，未定义更新/幂等 |
| `REPLACE INTO` | 实质 delete+insert，可触发 FK/时间变化 | 破坏性替换 | 禁止作为 Create |
| 先删子再重建整聚合 | 可表达 replace | 中途/并发/版本语义复杂 | 留给未来显式 Update |

### 5.4 子项写入

| 方案 | 成本 | 完整性/诊断 | 当前结论 | 重评触发器 |
| --- | --- | --- | --- | --- |
| 事务内 prepare 一次、按 ID 逐行 Exec | N 次执行；代码直观 | 失败定位明确、稳定锁顺序 | **选择** | Awards p95 足够大且 profiling 显示 round-trip 主导 |
| 单条动态 multi-values INSERT | 少 round-trip | placeholder/包大小动态、错误定位变粗 | 暂不选 | 有明确上限和批量性能证据 |
| 批量 loader/临时表 | 高吞吐 | 权限、资源、恢复复杂 | 不选 | 大规模离线导入成为独立用例 |
| 并发 goroutine 插子项 | 潜在吞吐 | 同一 Tx/连接不能获得预期并行，顺序与错误更复杂 | 不选 | 不适合当前事务模型 |

### 5.5 读取形状：两查询、JOIN 或 JSON 聚合

| 方案 | round-trip | 正确性与维护 | 当前结论 | 重评触发器 |
| --- | --- | --- | --- | --- |
| 两查询 + 单 RR snapshot | 2 queries | 根/子映射清楚、无根字段重复、空子显式 | **选择** | 网络 RTT 成为瓶颈或需要跨更多子集合 |
| 一条 LEFT JOIN | 1 query | 根列为每个 Award 重复；空子/scan/dedupe 边界复杂 | 当前不选 | RTT 高且聚合行数小，benchmark 证明收益明显 |
| MySQL JSON_ARRAYAGG | 1 query/1 row | JSON 类型/顺序/大值/解析错误进入 DB adapter | 当前不选 | 数据库投影 API 明确且有跨语言收益 |
| 先查 Award 再推断根 | 少一次？ | 无 Award 无法区分 not found 与损坏空根；根名丢失 | 不合法 |
| N+1 每个 Award 再查详情 | 多 round-trip | 当前数据都在一表，没有理由 | 不选 |

两查询不是“数据库规范化必然要求”，而是当前数据量、可读性和一致性事务之间的平衡。它的正确性依赖“同一 Tx + RR”，不能只复制 SQL 而丢掉事务。

### 5.6 读取隔离方案

| 方案 | 一致性 | 对写者/成本 | 当前结论 | 重评触发器 |
| --- | --- | --- | --- | --- |
| 两次 autocommit | 两个时间点 | 最低事务成本 | 不选，可能混合聚合 | 聚合变成单行 |
| READ COMMITTED transaction | 每条一致性读新快照 | 非锁定 | 不选，两查询仍可漂移 | 改成单查询或允许最终一致投影 |
| read-only REPEATABLE READ | 同一首读快照 | 非锁定，持有短事务 | **选择** | 单查询、读副本协议或业务一致性改变 |
| RR + `FOR SHARE/UPDATE` | 可锁定最新行 | 阻塞写者、死锁/尾延迟 | 不选 | 读取后同事务条件写入 |
| SERIALIZABLE | 最强串行化倾向 | 并发/锁成本最大 | 不选 | 有必须串行化的业务不变量且版本/CAS不足 |
| READ UNCOMMITTED | 可能脏读 | 低约束 | 禁止用于配置聚合 | 无 |

### 5.7 Hydration 时机

| 方案 | 连接占用 | 语义 | 当前结论 |
| --- | --- | --- | --- |
| 事务内扫描并完整构造后 commit | 坏数据早发现 | CPU/domain 校验延长 Tx | 可行但当前不选 |
| 事务内只扫描，commit 后 Restore | 更早归还连接；快照行已复制到 Go | commit 成功后才做领域校验 | **选择** |
| 不用 Restore，直接映射字段 | 最快 | 绕过不变量 | 禁止 |
| 自动 trim/删坏行 | 表面可用 | 隐式改写事实且不可审计 | 禁止 |

### 5.8 重试位置

| 方案 | 优点 | 风险 | 当前结论 |
| --- | --- | --- | --- |
| adapter 内遇 1205/1213 循环 | 调用者简单 | 不知道总预算/副作用/退避；可能掩盖热点 | 不选 |
| 返回 retryable，由 use case 重放完整事务 | 可结合预算、次数、指标和副作用 | 上层需要显式策略 | **选择当前边界** |
| 所有 repository failure 都重试 | “看起来更可靠” | 权限/schema/corruption 永久失败被放大 | 禁止 |
| commit unknown 自动重试 | 可能自愈网络抖动 | 重复/歧义 | 禁止，先对账 |

### 5.9 权限方案

| 方案 | 便利 | 风险 | 当前结论 |
| --- | --- | --- | --- |
| app 使用 root/migrator | 无权限障碍 | 任意数据/DDL破坏 | 禁止 |
| schema wildcard SELECT/INSERT | 新表自动可用 | 未来表无审查自动暴露 | 不选 |
| 两表 CRUD | 未来 Update方便 | 当前多余破坏能力 | 不选 |
| 两表 exact SELECT+INSERT | 正好覆盖当前 SQL | 每次能力变化要更新 allowlist | **选择** |
| app 只 SELECT | 最小读取 | Create 无法执行 | 第18节结论，本节已失效 |

## 6. 不变量与信任边界：任何实现替换都必须保住的性质

### 6.1 Repository 不变量

1. `Create(nil, ...)` 和未配置 Repository 不得 panic；
2. `Create` 必须在发 SQL 前重新验证 Strategy；
3. Create 成功意味着一个根和输入的全部 Award 已在同一事务提交；
4. commit 前任何错误都不得留下该测试聚合的部分行；
5. duplicate Create 不得覆盖既有聚合；
6. 只有根 INSERT 1062 可以分类为 `ErrStrategyAlreadyExists`；
7. 每条 INSERT 必须影响恰好一行；
8. 同一 Strategy 内容产生稳定 Award 写入顺序；
9. `FindByID(0)` 不访问数据库并返回领域 ID 错误；
10. 根不存在与根存在但 Awards 为空必须严格区分；
11. Find 的根/子查询必须使用同一事务快照；
12. Find 不得使用 locking read 干扰正常写者；
13. 任一存储行违反当前领域规则时，不返回部分或修复后的 Strategy；
14. 返回的 Strategy/Awards 不与 scan slice 共享可变所有权；
15. 公开错误字符串不能包含 SQL、driver message、DSN、constraint 或 Secret；
16. trusted code 仍可通过 `errors.Is/As` 看到语义类和 cause；
17. Repository 不关闭借来的共享 pool；
18. adapter 不自动重试 1205/1213，更不能重试 outcome unknown。

### 6.2 数据信任边界

```text
调用者构造的 domain.Strategy
  └─ 仍可能是零值/错误复制 → RestoreStrategy 再验证
       └─ 参数化 SQL → MySQL schema 局部约束
            └─ 持久化行不是可信领域对象
                 └─ scan 到 storage DTO
                      └─ RestoreAward + RestoreStrategy
                           └─ 完整合法聚合，或零值 + stable error
```

domain 类型私有字段能减少非法构造，但 Go 零值始终存在；数据库 CHECK 能拒绝局部坏值，但不能保证至少一子和总和；两层验证是责任互补，不是重复浪费。

### 6.3 身份边界

- StrategyID 是根身份；
- AwardID 只有与 StrategyID 组合才是持久化身份；
- Repository API 接收完整 Strategy 或 StrategyID，不暴露“全局 Award lookup”；
- duplicate 只说明身份冲突，不证明内容等价；
- future event/API 若只携带 AwardID，会跨越当前身份边界并丢失语义。

### 6.4 代码与依赖信任边界

| 边界 | 允许 | 必须拒绝/隔离 |
| --- | --- | --- |
| application→port | domain 类型、context、稳定错误类 | SQL、driver error number、`sqlx.DB` |
| adapter→MySQL | 固定 SQL、绑定参数、显式 TxOptions | 用户提供 SQL/表名、multi-statements |
| persistence DTO→domain | exact value + Restore | unsafe field fill、trim/normalize repair |
| Repository→caller | 完整 Strategy 或错误 | partial aggregate、raw DB message |
| composition root→Repository | 借用已配置 pool | Repository 自行创建/关闭全局 pool |
| test migrator→schema | 隔离库内受控 DDL/清理 | 默认 Compose 数据、用户其他容器/volume |

### 6.5 权限信任边界

| 身份 | 当前必要能力 | 明确禁止 |
| --- | --- | --- |
| application | 两表 SELECT、INSERT | UPDATE、DELETE、DDL、schema_migrations |
| migrator/verification | schema 演进与受控测试修复 | 在线请求 |
| grant job | 短时 root socket 权限收敛 | TCP 网络、长期驻留、业务行操作 |
| Repository package | 业务表固定 SQL | 权限管理、Migration、Secret读取 |

read-only TxOptions 是事务意图，不是权限边界；真正的破坏面仍由 MySQL grants 约束。

## 7. 资源所有权、并发与时间预算：谁创建，谁释放，谁有资格重试

### 7.1 连接池所有权

- `mysqlstore.Open` 创建并配置 `*sqlx.DB` pool；
- application composition root 应拥有它、监控它并在进程退出时 Close；
- `mysqlrepo.New` 只借用 handle，不 Ping、不改 pool 参数、不 Close；
- 同一个 pool 可被同进程其他 adapter 共享；Repository 操作结束后仍可 Ping 是所有权未被误夺的证据；
- nil handle 在构造时被拒绝，zero Repository 的方法也 fail closed。

若 Repository 自己 Close pool，一个用例完成就可能破坏其他模块；若 Repository 自己 Open pool，每个实例会产生独立连接预算、Secret 生命周期和不可见资源泄漏。

### 7.2 写事务所有权与生命周期

```text
caller context
  → BeginTx
  → root INSERT
  → Prepare child INSERT
  → Award[sorted 0..N) INSERT
  → statement Close
  → context pre-commit check
  → Commit
  → success / outcome unknown
```

- `Create` 创建并独占 Tx；
- defer rollback 只作任何未提交路径的兜底；commit 成功后的 rollback 返回值故意忽略；
- prepared statement 属于该 Tx，方法内关闭，不缓存到 Repository；
- caller 不接管 Tx，也不能在中途插入外部 SQL；
- 任何 child failure 后不继续尝试剩余 Award。

### 7.3 读事务所有权与生命周期

```text
caller context
  → BeginTx(RR, read-only)
  → root SELECT  [建立 consistent snapshot]
  → award SELECT [复用 snapshot]
  → rows fully scan + close
  → context pre-commit check
  → Commit
  → Restore aggregate outside Tx
```

扫描结果在 commit 前完整复制到 Go 值；领域恢复不再访问数据库，因此可在释放事务连接后执行。若将来 Restore 变得昂贵，应单独衡量 CPU/内存预算，不能为了“快”绕过校验。

### 7.4 并发创建

同一 StrategyID 的并发 Create 不用进程内 mutex：数据库唯一约束是跨实例共享裁判。理想结果是一个成功、另一个在根 INSERT 得到 1062。进程锁既无法跨实例，又会把 identity consistency 错误地绑定到单进程。

不同 StrategyID 的创建可并发。稳定 AwardID 顺序减少同一索引上的锁序变化，但 InnoDB 仍可能因其他事务、外键、gap lock 或 future index 产生 1205/1213。

### 7.5 时间预算的层级

1. use case/request context 应给出最外层总 deadline；
2. Repository 所有语句继承该 context，不私自延长；
3. driver connect/read/write timeout 是网络安全网，不替代业务 deadline；
4. MySQL lock wait/deadlock可能早于或晚于 context 触发；
5. 上层若重试，必须在同一个总预算内包含 backoff 和下一次完整事务；
6. commit unknown 不进入普通 retry budget，而进入对账/人工恢复预算。

当前代码没有强制 context 必须带 deadline，这是一个刻意保留给调用者、但必须在 API composition 时收口的缺口。

### 7.6 容量复杂度

- Create：1 begin + 1 root exec + 1 prepare + N child exec + 1 commit，数据库工作和内存近似 O(N)；
- Find：1 begin + 2 queries + 1 commit，返回/恢复内存 O(N)；
- 不是 N+1 查询，但写入仍有 N 次 Exec round-trip；
- pool 中每个进行中的 Tx 占用一个连接；慢锁等待会造成 pool saturation；
- 当前没有 Awards 数量的产品上限、payload 上限或 transaction duration SLO。

## 8. 失败模型与恢复语义：每一种失败都要回答“事实可能已经到哪一步”

### 8.1 输入在 SQL 前非法

zero Strategy、非法名称或非法 Award 由 Restore 拒绝；数据库没有被访问。调用者应修正请求/程序错误，不能重试依赖。

### 8.2 Begin 失败

没有事务，也没有本次写入。可能是 pool/context/网络/权限；通用错误分类保留取消，1205/1213 是 retryable，其余 dependency failure。当前没有按 stage 细分 begin/query/scan。

### 8.3 根 INSERT 1062

本次事务没有创建新根；返回 StrategyAlreadyExists。恢复动作不是直接当成功，而是由上层决定：拒绝重复、读取对比，或进入提交未知对账。

### 8.4 根 INSERT 成功，子 INSERT 失败

事务未提交，defer rollback；返回对应错误。子 1062、CHECK、FK、权限或 schema drift 都不被误报为根 already exists。真实故障注入证明当前 MySQL 路径不会留下根。

### 8.5 child statement Prepare/Close 或 RowsAffected 异常

都在 commit 前返回并回滚。Close error 虽少见，也不能假定已执行语句可安全提交；RowsAffected 非 1 表明 SQL/driver/触发器语义偏离预期，fail closed 而不是“差不多成功”。

### 8.6 调用前取消

Begin/Exec/Query 返回标准 context error；没有可靠证据表明 SQL 已开始，因此测试还验证无测试行残留。业务层可把它当请求终止，不应记录为数据库 corruption。

### 8.7 事务中锁等待后 deadline

真实测试用 gap lock 阻塞 child INSERT，deadline 后返回 `DeadlineExceeded`，随后验证根/子均为零。这证明当前 driver/context/rollback 组合在该时序下成立；不应推广为所有 commit 附近时序。

### 8.8 1205 锁等待超时与 1213 死锁

两者标为 `ErrRepositoryRetryable`，含义是“完整事务可能在上层策略允许时重放”，不是“当前语句继续即可”。adapter 不知道：

- 请求是否还剩预算；
- use case 是否已有事务外副作用；
- 允许几次、何种 jitter/backoff；
- 热点是否应该熔断而不是持续加压；
- 是否需要更新 retry metrics/request ID。

所以 adapter 只分类，不重试。未来上层必须重建整个 Create 输入并从 Begin 开始，不能从失败 Award 继续。

### 8.9 写 Commit 返回错误

若事务错误与已发生的 context error 明确匹配，或 `database/sql` 返回 `sql.ErrTxDone` 且 context 已取消，返回标准取消；否则返回 `ErrCommitOutcomeUnknown`。

恢复原则：

1. 不盲重试；
2. 使用相同 StrategyID 查询权威库；
3. 若存在，比较完整聚合是否等于原请求；
4. 若不存在，也要考虑复制延迟/故障切换时间点后再按策略处理；
5. 若存在不同内容，升级为身份冲突/人工调查；
6. 记录 request/strategy identity，但不要记录 Secret 或未审查 driver message。

当前只有 FindByID 可作为对账原语，没有自动 resolution use case，因此不能声称已解决幂等。

### 8.10 Find 根不存在

只有根查询 `sql.ErrNoRows` 映射 `ErrStrategyNotFound`。不查询 Awards 来“猜”根，也不返回空 Strategy。

### 8.11 根存在但 Award 为空或坏数据

这是 `ErrStoredStrategyInvalid`，不是 not found。自动补默认 no_reward、忽略坏行、trim 或删除都会篡改事实。运行时合理默认是 fail closed，恢复人员通过受控数据审计/修复流程处理。

### 8.12 读取期间有并发更新

旧 reader 返回首个 consistent read 时间点的完整旧聚合；新 reader 在 writer commit 后返回新聚合。它不等待“绝对最新”，也不会把旧根与新 Awards 混合。若业务未来要求读己之写/线性一致，必须重新定义路由和版本，不应把 RR 误当承诺。

### 8.13 读事务 Commit 失败

read-only 事务没有本节业务写效果，因此非取消错误走 operation failure，而非 write outcome unknown；方法不会返回已经扫描的 Strategy。这样调用者不会把“读事务结束失败”当成可信成功。

### 8.14 旧版本、DDL 与 schema drift

缺表、缺列、列类型变化、权限漂移、事务期间 incompatible DDL 都走 repository failure；本节没有自动 Migration 或兼容 fallback。API 启动 gate 保证正常 Compose 先到 clean latest，但运行中/外部环境漂移仍需监控。

### 8.15 恢复人员常见错误

- 将 duplicate 一律改成 success；
- commit unknown 后立即无限重试；
- 给 app root/DDL 权限“先恢复服务”；
- 删掉坏 Award 让 Find 成功，却不保留审计；
- 把 RR 改成 RC 只为少写 Tx；
- 加 `FOR UPDATE` 解决读取不一致，结果阻塞所有配置写；
- 修改历史 Migration 或关闭约束；
- 清空默认 Compose volume 来重跑测试；
- 为减少错误率在 adapter 内无限重试 1205/1213。

## 9. 安全与隐私推导：权限、SQL、错误和资源耗尽是一条链

### 9.1 最小权限来自语句集合

当前四条业务 SQL 只需 SELECT/INSERT 两张表。grant reconciliation 先撤权再精确授予，避免 reused volume 积累历史权限；negative probe 明确拒绝 UPDATE/DELETE/schema_migrations。

未来新增 Update 时必须同节增加权限、事务和并发协议，不能今天预授。未来删除功能也不能因“账号已经有 DELETE”而倒逼产品语义。

### 9.2 参数绑定与注入面

StrategyID、name、weight、outcome 全部使用 `?` 参数；调用者不能提供 SQL、列名、ORDER BY 或表名。Repository 不启用 multi-statements。测试中的动态 CHECK DDL只由受控迁移身份构造固定前缀+base36 constraint 和数值，不属于运行时输入面。

参数化 SQL 不能防止合法但恶意的大对象、Unicode 视觉欺骗或业务授权缺失；这些属于不同防线。

### 9.3 错误输出与 cause 生命周期

公开边界只应输出 stable class 和 request ID；可信结构化日志可在受控环境记录 error number/stage，但仍不应直接打印 DSN、SQL 参数或 Secret。`Unwrap` 是诊断能力，不是公开序列化许可。

特别是 corrupt snapshot 的具体名称可能是运营内容；API 不应把非法值回显给匿名调用者。

### 9.4 Secret、TLS 与身份

Repository 不读环境变量/Secret，不构造 DSN；这些由 `mysqlstore` 和 composition root 管理。真实 isolated 本地测试可使用 disabled TLS，但这不构成生产传输安全结论。生产连接必须由环境基线决定 TLS CA、凭据轮换和最小身份。

### 9.5 read-only 事务不是访问控制

`ReadOnly: true` 表达驱动/数据库事务意图并减少误写机会，但真正限制 UPDATE/DELETE 的是 app grants。若 driver/server 忽略 read-only hint，权限仍应挡住破坏性 SQL；若 grants 漂移，TxOptions 也不能成为完整安全边界。

### 9.6 资源耗尽与容量攻击

Find 会把全部 Awards 载入内存，Create 会在一个事务内执行 N 次 INSERT。当前没有产品级 N 上限；一旦 API 接入，恶意/误操作的大聚合可能占满连接、内存、redo 和锁等待预算。必须在 use case/HTTP 层建立 payload、Award 数量和 deadline 上限，并用数据决定批处理策略。

### 9.7 Unicode 与输出安全

Restore 保留合法 Unicode 原值，不做 normalization。名称未来进入 HTML/日志/CSV 时仍需上下文转义、防公式注入或同形字符提示；Repository 的“保真”不能被误写为“内容安全”。

### 9.8 供应链安全

运行时新增依赖仍是既有 `sqlx` 与 MySQL driver；`go-sqlmock` 只用于测试控制流。版本进入 `go.mod/go.sum` 能提供可复现解析，但不是漏洞扫描、SBOM、签名或 provenance。MySQL 8.4.11 固定 tag 也不是 digest pin。生产交付仍需依赖审计和镜像来源策略。

## 10. 证据设计：绿色测试分别想推翻什么，又证明不了什么

### 10.1 证据矩阵

| 证据 | 试图证伪的错误主张 | 当前能证明 | 明确不能证明 |
| --- | --- | --- | --- |
| application error unit | safe class 会丢 cause，或 cause 会泄露到字符串 | `errors.Is` class/cause 与安全渲染并存；零值 fail closed | 调用方日志一定安全 |
| Restore unit/domain tests | storage DTO 可以绕过名称/聚合规则 | 非规范数据被拒绝，排序/总和重算 | 所有业务语义污染可检测 |
| sqlmock Find 顺序 | 公开方法可能在 Tx 外查询或先 commit | Begin→root→awards→Commit 的控制流 | MySQL RR、read-only TxOptions、真实扫描性能 |
| sqlmock commit failure | Create 会把 commit driver error 当普通失败 | 公开 Create 返回 outcome unknown，保留 cause且脱敏 | 真实网络半断/服务端耐久结果 |
| real TxOptions probe | 代码写了 RR/read-only 但 driver/server 没执行 | 共用 helper 得到 REPEATABLE-READ；写探针被 server 1792 拒绝并由固定 driver 映射为 bad connection；无行残留 | 公开 Find 内部的任意语句级暂停点；跨 driver 行为 |
| 真实 round-trip | Unicode、特殊 SQL 字符、MaxUint64 或 scoped AwardID 映射错误 | 固定 MySQL 8.4.11 上无损 Create/Find | JSON/前端大整数、其他 MySQL 版本 |
| 顺序 duplicate | Create 会覆盖/误报 | 根 1062→already exists，行数不变 | duplicate 是幂等成功 |
| 并发 duplicate | 进程内预查能替代唯一约束 | 两个真实并发 Create 恰好一成一冲突 | 高并发吞吐/所有死锁分布 |
| scoped CHECK rollback 注入 | 根成功、子失败会残留空聚合 | commit 前 child failure 后根/子均为零 | crash/commit ACK 丢失时序 |
| 预取消 + gap-lock deadline | context 只在开始前有用，或取消后残留根 | 预取消与 in-flight lock wait 可取消并回滚 | commit 已发送后的确定结果 |
| real 1205 gap-lock probe | MySQL error number 只在合成单测成立 | server 端 1s lock wait 返回 1205，公开 Create→retryable，整笔 Tx 回滚 | 真实 1213 deadlock 分布；上层重试正确性 |
| 四类 corrupt snapshots | 读路径会忽略/修复坏数据或返回部分对象 | 空子、坏根名、控制字符、总和溢出都 fail closed | 所有可能 corruption 类别 |
| RR snapshot interleave | 两查询可能组合两个提交时间点 | 同一真实 RR Tx 的旧根/旧子一致，新 Find见新状态 | public 方法内精确暂停；跨副本一致性 |
| EXPLAIN root/awards | 查询会全表扫描或额外 filesort | 当前 schema/统计下 root PRIMARY const、child PRIMARY 左前缀且无 filesort | 数据量增长、统计漂移后计划永远不变 |
| exact grants + negative probes | app 仍有历史 CRUD/版本表权限 | 当前 isolated/Compose 身份只有两表 SELECT+INSERT，mandatory roles 空 | 外部 IAM/运行中人工扩权 |
| Compose smoke | 镜像、migration、grants、启动 gate 未协同 | 当前本地 Compose 拓扑可重复启动并通过边界检查 | 业务 Repository 已被 HTTP 调用；生产平台正确 |
| pool Ping after operations | Repository 误关共享 pool | 当前操作后 pool 仍可用 | 长期连接泄漏/生产 pool tuning |

### 10.2 为什么需要 mock 与真实 MySQL 两类证据

mock 擅长稳定制造 commit error 和约束调用顺序，但不实现 InnoDB；真实 MySQL 擅长证明 error number、MVCC、锁等待、rollback、uint64 和执行计划，但很难可重复制造“服务端已 commit、客户端正好丢 ACK”的纳秒时序。

因此二者不是互相替代：

- 用 mock 证明应用控制流走到哪个分类分支；
- 用真实 MySQL 证明数据库语义和 SQL 形状；
- 用代码评审/官方语义承认仍无法实验穷举的 commit uncertainty；
- 用未来 fault-injection/staging 补生产拓扑证据。

### 10.3 负向证据同样重要

本节还应通过代码范围审查确认：

- 没有 UPDATE/DELETE/UPSERT SQL；
- 没有 `FOR UPDATE`/`FOR SHARE`；
- 没有 adapter retry loop；
- 没有 Repository Close shared pool；
- 没有把 raw driver error 当公开字符串；
- 没有 HTTP route 或算法声称消费 Repository；
- 没有 schema wildcard grant；
- 没有为了测试删除默认 Compose data volume。

### 10.4 当前证据的总边界

真实 MySQL 8.4.11 isolated integration 和 Compose 已通过，说明当前代码、schema、直接 grants 与本地拓扑在该固定范围内相容。它不能证明：生产数据规模、跨 AZ 延迟、读副本、连接故障切换、TLS、在线 DDL、持续权限审计、自动对账、幂等、业务 API、算法公平或端到端 SLO。

文档门禁和章节最终提交属于文档交付流程；本手记不把尚未执行的门禁结果提前写成事实。

## 11. 被刻意推迟的能力：没有证据就不增加永久复杂度

| 推迟项 | 为什么现在不做 | 当前风险 | 明确重评触发器 | 未来归属 |
| --- | --- | --- | --- | --- |
| Update/Save | 缺版本、冲突和替换语义 | 暂不能运营编辑 | 第一个编辑用例获批 | application/domain/schema |
| Upsert | duplicate 不等于幂等 | 客户端需处理 already exists | 明确 request identity + 内容等价协议 | use case/API |
| Delete | 缺历史引用/审计 | 不能清理配置 | 生命周期、合规或归档需求 | product/domain |
| List/search/page | 缺查询与排序事实 | 管理界面暂不可浏览 | 运营页面真实需求与数据规模 | read model/API |
| aggregate version/CAS | Create 无更新冲突 | 未来更新不可安全实现 | Update 章节 | schema/Repository |
| bulk INSERT | Award 规模未知 | 大 N 可能慢 | p95 N、RTT、transaction timing 达阈值 | adapter |
| adapter 自动 retry | 不知道总预算/副作用 | 上层需显式处理 retryable | 完整 use case 与 retry policy | application/service |
| commit unknown 自动对账 | 尚无调用协议/request record | 需人工/显式处理未知 | API/任务真实创建流 | application/runbook |
| read replica | 无拓扑/lag 契约 | 全走 primary | 读负载、容灾需求 | infrastructure |
| Repository metrics/tracing | 还未接运行时 | 生产诊断缺口 | composition/API 接入 | observability |
| cache | 无热点/失效版本 | DB 读取成本未优化 | p95、QPS、命中模型证据 | Redis adapter |
| ORM/sqlc | 当前四条 SQL 可审查 | 手写漂移需 code review | query 数量/缺陷率上升 | tooling |
| data repair tool | corruption 只做 fail closed | 恢复依赖人工 SQL | 第一例真实 corruption/导入流程 | ops/admin |
| audit/outbox | Create 无外部事件 | 无变更历史 | 发布/通知/合规需求 | domain/infrastructure |
| Award count upper bound | 无产品数据 | 大聚合可耗尽资源 | HTTP 创建前必须决策 | product/application |
| production TLS/failover test | 本节本地 isolated | 环境差异未知 | staging/上线准备 | platform |

## 12. 需求未提但架构师会主动检查的点

### 12.1 容量总账：整聚合模型的隐藏上限

当前每次 Find 读取全部 Awards，每次 Create 持有事务直到 N 条子 INSERT 完成。需要在 API 前采集/约束：

- Strategy 总量、平均/p95/max Awards；
- name 平均字节数与 utf8mb4 最坏空间；
- Create QPS、Find QPS、热点 Strategy 比例；
- transaction duration、lock wait、pool wait；
- redo/binlog/备份增长；
- MaxAllowedPacket 与批量策略；
- 恶意或误配置的最大 payload。

没有这些数，不应声称“可支撑高并发”或“无需批量”。

### 12.2 索引不是看 SQL 猜出来就结束

当前根 PK 对等值查询为 const，子复合 PK 的左前缀服务 `WHERE strategy_id`，后续 AwardID 顺序不 filesort。这是良好起点，但需监测：

- stats/版本升级后执行计划变化；
- 单 Strategy 巨大时扫描行数；
- 未来 List 按 name/status/updated_at 查询产生新索引需求；
- 额外索引的写放大、buffer pool 和 DDL 成本。

不要因未来“可能按名称搜”今天就加索引，也不要把一次 EXPLAIN 当永久证明。

### 12.3 发布顺序和兼容窗口

本节 schema 不变，权限从 SELECT 扩到 SELECT+INSERT，是向前兼容增长。正常 Compose 仍是 migrate→grants→API；但生产滚动要回答：

1. 旧实例在新 grants 下仍能运行吗；
2. 新实例在旧 grants 下是否会发 Create 并失败；
3. grant reconcile 的 REVOKE→GRANT 短窗口会不会影响在途旧实例；
4. 多实例时是切新 role/account，还是允许短时超集再收敛；
5. rollback 到旧代码后额外 INSERT 是否应撤销。

本地 one-shot gate 不能直接复制为生产零停机授权方案。

### 12.4 Commit unknown 的运维剧本

这是最容易在面试和生产中被忽略的点。未来 runbook 至少要定义：

- 用哪个 request ID/StrategyID 查；
- 查询 primary 还是可能 lag 的 replica；
- 比较哪些字段、顺序如何 canonicalize；
- 等待多久才判定未提交；
- 已存在但内容不同如何升级；
- 谁能用 migrator/管理员身份调查 raw cause；
- 何时允许人工重放；
- 事件/审计如何记录最终 resolution。

只返回 `ErrCommitOutcomeUnknown` 是诚实起点，不是恢复闭环终点。

### 12.5 数据损坏隔离与修复

fail closed 会保护抽奖正确性，但也可能让业务不可用。需要预先决定：

- 是否对 corrupt Strategy 单独下线，而不是拖垮全部 Lottery；
- 管理端如何显示“配置不可用”而不泄露坏值；
- 修复前是否备份原行并记录操作者/原因；
- 修复工具是否复用 Restore 验证；
- 是否定期扫描所有 Strategy 做 offline integrity check；
- 如何防止高权限 CSV/脚本绕过领域校验。

### 12.6 可观测缺口清单

当前只有错误类型，没有生产仪表。接入运行时前至少设计：

- `lottery_repository_operation_duration`，按 operation/result 分类但不带 StrategyID 高基数标签；
- pool in-use/wait count/wait duration；
- not_found、already_exists、stored_invalid、retryable、outcome_unknown 计数；
- MySQL 1205/1213 与 context cancellation 分开；
- query/transaction trace span，不记录名称/SQL参数/DSN；
- corrupt 与 outcome unknown 的告警阈值；
- EXPLAIN/slow log 的受控采集；
- retry attempt 由上层记录，不能在 adapter 内隐身。

### 12.7 读副本与一致性路由

如果未来根查询和子查询被中间件拆到不同副本，单 `Tx` 的承诺可能被破坏；如果 Find 走 lagging replica，Create 后立即 Find 可能 not found。Repository 当前借用一个 pool/Tx，隐含“同一数据库连接指向一个一致 InnoDB server”的前提。引入读写分离必须定义 read-your-write、replica lag、fallback primary 和事务路由，不能只换 DSN。

### 12.8 Schema evolution 与旧代码

新列若 NOT NULL 无默认，会破坏旧 INSERT；重命名/类型变化会破坏 scan；改变 Outcome/名称规则会让历史行被新 Restore 判坏。未来 Migration 必须走 expand/contract：先兼容读写，再回填/验证，最后收敛；并明确旧构件的 rollback 窗口。

### 12.9 事务隔离与 DDL

一致性读并不覆盖所有 DDL 变化。生产 online ALTER 可能造成 table definition changed、metadata lock 或连接错误。Repository 会 fail closed 为 dependency failure；发布 runbook 需要避免在不兼容 DDL 窗口让旧/新代码混跑。

### 12.10 成本模型

当前选择的成本很具体：

- 每个 Create 有 N 次 child Exec；
- 每个 Find 有两个查询与一个短 Tx；
- 每个 aggregate hydration 有 O(N) allocations/validation；
- 精确 grants 每次 schema/SQL 能力变化要维护脚本和 smoke；
- 真实 MySQL 测试比 unit 慢，需隔离容器和清理；
- stable errors 和 unknown outcome 增加上层分支，但避免更昂贵的数据歧义。

优化必须指向被测成本，不能因为“事务/两查询看起来重”就牺牲一致性。

### 12.11 供应链与可复现

- `go.mod/go.sum` 固定 sqlx、driver、sqlmock 解析版本；升级要重跑 error number、TxOptions、cancel/commit 分类；
- MySQL 8.4.11 与生产 minor 版本要比较；
- Docker tag 最终应考虑 digest、SBOM、签名与 CVE 响应；
- 测试 helper 的临时 CHECK 必须使用隔离 schema、唯一 constraint 名并在前后验证清理；
- 不能把本地测试生成物、临时 container/volume 当成项目依赖留下。

### 12.12 长期兼容与大整数

Repository 证明 Go↔MySQL `uint64` round-trip，不证明 API/JavaScript 无损。未来 HTTP 必须决定 ID/weight 使用字符串还是受限数字；否则 MaxUint64 在前端会丢精度。这个问题属于 API 契约，不能在 Repository 里把类型偷偷降为 int64。

### 12.13 业务授权与数据库授权是两层

MySQL app 身份能 INSERT，不代表任意 HTTP 用户都能创建 Strategy。未来 use case 必须验证操作者身份、租户/组织、审批与审计。数据库最小权限限制“服务进程能做什么”，业务授权限制“当前主体被允许做什么”。

### 12.14 配置发布与不可变快照

本节只有 Create，没有“已发布/草稿”状态。未来 Draw 若总是 Find 当前 Strategy，配置 Update 会改变后续概率；历史 Draw 又需要知道当时版本。可能演进为 immutable StrategyVersion 或 aggregate version + Draw snapshot，但必须由发布/审计事实驱动，不能让本节 Create 暗中承担版本功能。

### 12.15 对账不能只看“有这一 ID”

提交未知后发现同 ID，只能证明某个聚合存在；还需比较 name、完整 Awards、weight、outcome 和 canonical 顺序。否则一个较早/并发创建的不同配置会被误认作本请求成功。未来可考虑内容 hash，但 hash 算法、字段版本和碰撞/规范化必须正式定义。

### 12.16 测试隔离与清理

真实集成测试会写业务行、临时加 CHECK；双重环境变量授权是为了阻止误指默认库。fixture ID 精确列出、事务清理、约束存在性复查，比广域 `DELETE`/drop database/Docker prune 安全。任何失败后都应只清理本测试明确创建的容器、constraint、rows 和临时文件。

### 12.17 简历与面试表达边界

可以诚实表达：设计窄 application port、事务性聚合 Create、单 RR snapshot Find、领域 fail-closed hydration、commit outcome unknown/error classification、真实 MySQL 并发/取消/回滚与最小权限验证。

不能表达：完成线上抽奖 API、千万 QPS、读写分离、自动容灾、分布式事务、零数据丢失、缓存优化或完整幂等，因为这些证据都不存在。

## 13. 假设与风险账本：每个“目前可以”都要有失效信号

| ID | 假设/风险 | 当前证据或控制 | 失效影响 | 观察信号 | 下一次复核/动作 |
| --- | --- | --- | --- | --- | --- |
| R19-01 | 单 Strategy Awards 足够小 | 整聚合测试、无生产数据 | 内存/Tx/RTT放大 | p95/max N、Tx duration | API 创建前先定上限 |
| R19-02 | 两查询 RTT 可接受 | 本地集成通过 | Find 尾延迟高 | query/Tx p95、跨 AZ RTT | 有运行数据后比较 JOIN |
| R19-03 | 逐行 child INSERT 可接受 | 稳定顺序、当前小 fixture | 大 N 写慢/占连接 | Create duration、N、redo | 达阈值 benchmark multi-values |
| R19-04 | 复合 PK 持续满足查询 | 当前 EXPLAIN PRIMARY/no filesort | 扫描/排序退化 | rows examined、plan diff | 数据量/版本升级时复跑 |
| R19-05 | MySQL 8.4 RR 语义与生产一致 | 8.4.11 真实 snapshot | 混合快照或环境差异 | server version/config | staging 每次 MySQL 升级 |
| R19-06 | 单 pool Tx 不会跨副本 | 当前单 MySQL | root/child/读己之写不一致 | 引入 proxy/replica | 拓扑变更前 ADR |
| R19-07 | read-only TxOptions 被 driver/server正确处理 | 共用 helper 的真实 `@@transaction_isolation=REPEATABLE-READ`；写探针触发 1792→driver bad connection且无行 | 意图提示失效 | driver/MySQL upgrade | 升级集成测试；grants仍作硬边界 |
| R19-08 | Create 身份由调用者生成且冲突低 | unique/concurrent test | duplicate 增多/可用性下降 | already_exists rate | ID 生成用例出现时复核 |
| R19-09 | duplicate 不需要幂等成功 | 当前 Create 契约 | 客户端重试体验差 | 重复请求/网络故障 | API 设计 request identity |
| R19-10 | commit unknown 可由上层处理 | 独立错误类；暂无流程 | 数据状态长期不明 | outcome_unknown count | runtime 接 Create 前补 runbook/use case |
| R19-11 | 1205/1213 由上层重试更安全 | 真实 1205 + 合成 1213 分类；adapter 无 retry | 调用方遗漏重试 | retryable error rate | 首个用例定义预算/退避；补真实 deadlock 场景 |
| R19-12 | stable AwardID order 是 canonical | domain 排序、SQL ORDER BY | future display/version含义冲突 | 运营排序需求 | 新增 position 前 ADR |
| R19-13 | 数据坏行是异常而非草稿 | domain/schema 契约 | 合法草稿被判不可用 | 产品提出分步编辑 | 建 draft/staging 模型 |
| R19-14 | fail closed 可接受 | 四类 corrupt test | 单配置故障影响业务 | stored_invalid count | API 接入时设计隔离/告警 |
| R19-15 | exact grants 运维可承受 | reconcile/smoke | 发布失败或人工绕过 | grant job failures/drift | 每个 SQL 能力章节复核 |
| R19-16 | mandatory roles 为空 | grant job 断言 | 隐式扩权/启动失败 | server config change | 启用 role 前重建权限模型 |
| R19-17 | application 无 UPDATE/DELETE需求 | 当前 SQL 范围 | 开发者绕权/功能受阻 | 新用例 PR | 同节设计事务+扩权 |
| R19-18 | raw cause 只进入可信诊断 | safe Error string | SQL/数据/拓扑泄露 | log review/扫描 | runtime logging 接入前审计 |
| R19-19 | caller 会传有界 context | 集成使用 deadline | 无界锁等待/池耗尽 | missing deadline、long Tx | HTTP/use case composition 强制 |
| R19-20 | Repository 不拥有 pool | 类型注释、Ping test | 连接泄漏或误关闭 | pool stats/shutdown errors | composition 接入时验证 Close |
| R19-21 | schema v2 与 adapter 长期兼容 | fixed SQL+真实 test | deploy 后缺列/权限失败 | migration/version mismatch | 每个 Migration 跑兼容矩阵 |
| R19-22 | 无 trigger/旁路写改变语义 | 当前 schema 可审查 | RowsAffected/数据漂移 | SHOW CREATE/schema diff | staging/权限持续审计 |
| R19-23 | sqlmock 只作控制流证据 | 文档明确边界 | 团队误信 mock 等于 DB | 测试评审 | 保留真实 MySQL gate |
| R19-24 | 当前测试清理足够精确 | unique IDs/constraint复查 | 污染共享数据 | leftover row/constraint | 每次集成失败后审计 exact targets |
| R19-25 | Repository 尚未运行时接流量 | `rg` 无外部消费者 | 文档夸大/死代码漂移 | composition PR | 第21节接入并补端到端证据 |
| R19-26 | MySQL primary 是权威对账源 | 当前单实例 | replica lag误判未提交 | 拓扑变化 | outcome unknown runbook 指定 primary |
| R19-27 | 名称保真优先于搜索折叠 | Restore exact + binary schema | 搜索/反欺骗体验不足 | 国际化/搜索需求 | 建独立搜索投影 |
| R19-28 | 依赖版本当前可接受 | go.mod/go.sum、测试通过 | driver transaction/error行为变更 | Dependabot/CVE/upgrade | 每次升级重跑真实取消/快照 |
| R19-29 | 当前没有生产 SLO证据 | 只记录缺口 | 简历/架构过度承诺 | 性能宣称 | load test 前禁止吞吐结论 |
| R19-30 | Create 不伴随外部事件 | 当前无 outbox/handler | 未来双写不一致 | 发布/通知需求 | 新用例先设计 outbox/事务边界 |

## 14. 未来演进问题：下一节可以建立在边界上，但不能跳过新命题

### 14.1 第 20 节加权算法

- 算法接收完整 `domain.Strategy` 还是直接依赖 `StrategyReader`；
- 如何保证 `[0,totalWeight)` 在 MaxUint64 边界无偏；
- 随机源如何注入和测试；
- canonical AwardID 顺序是否只用于确定性遍历，不成为隐藏概率；
- `no_reward` 怎样作为正常 Award 返回；
- Repository failure 与 draw outcome 如何严格分开。

### 14.2 第 21 节 API/composition

- 谁创建 MySQL pool、Repository、use case 并按反序关闭；
- request deadline 多长，如何小于 server shutdown/driver timeout；
- StrategyID/Weight 如何无损 JSON 表达；
- not found/duplicate/corrupt/retryable/outcome unknown 映射什么公开错误；
- Create 是否真的对外开放，认证/业务授权/幂等键是什么；
- outcome unknown 如何给客户端可执行且不误导的响应；
- 如何增加低基数 metrics、trace 与安全日志；
- 端到端测试如何证明 app 身份而非 migrator 被使用。

### 14.3 第 23 节规则与配置演进

- Strategy 是可变聚合还是 immutable version；
- draft/published/archived 如何建模；
- Update 是整聚合替换还是命令式增删 Award；
- optimistic version/CAS 放哪一层；
- 历史 Draw 如何指向当时配置；
- 谁能编辑、审批、发布，审计如何持久。

### 14.4 第 24 节缓存

- cache key 是否含 Strategy version；
- 缓存完整聚合还是只读投影；
- corrupt cache 与 corrupt DB 怎样区分；
- MySQL Find 是回源还是权威每次读；
- TTL/主动失效/双删分别会产生什么窗口；
- Redis 失败是否回源，回源风暴如何限制；
- 没有 version 的当前 Strategy 为什么不适合直接缓存更新。

### 14.5 更远的数据库问题

- 读写分离后的 read-your-write 与 outcome unknown 对账；
- 多租户后 ID/权限/索引是否要加入 tenant_id；
- 大 Strategy 是否仍是一个同步事务；
- bulk import 如何复用领域验证并保留逐项错误；
- backup restore 后 schema/version/grants 如何重验；
- 数据修复如何审批、审计和回滚；
- online DDL 与旧/new adapter 的 expand-contract；
- 若分库后物理 FK 消失，用什么巡检/事件协议保持完整性。

### 14.6 必须重新决策的明确触发器

出现任一条件时，当前结论不应被当作惯例继续复制：

1. 单 Strategy Award 数量或 Find/Create p95 超出目标；
2. 新增 Update/Delete/List/搜索/草稿/发布用例；
3. 引入读副本、代理路由、分库或跨区域；
4. outcome unknown 在真实环境出现；
5. stored invalid 在真实数据出现；
6. 1205/1213 比率持续升高；
7. MySQL/driver/sqlx 版本升级；
8. mandatory roles 或生产 IAM 模型启用；
9. API 开放 Create 给外部调用者；
10. Strategy 创建需要同时发事件/审计/Benefit；
11. EXPLAIN/slow query/pool wait 显示当前 SQL 形状成为瓶颈；
12. 产品要求“同 ID 重放是幂等成功”。

## 15. 可追溯证据

### 15.1 Application 端口与错误

- [Repository 窄端口](../../../internal/lottery/application/repository.go)
- [稳定错误分类](../../../internal/lottery/application/repository_error.go)
- [错误类与 cause 脱敏测试](../../../internal/lottery/application/repository_error_test.go)

### 15.2 MySQL adapter

- [Create、FindByID、事务与分类实现](../../../internal/lottery/adapter/mysqlrepo/repository.go)
- [边界与错误分类单元测试](../../../internal/lottery/adapter/mysqlrepo/repository_test.go)
- [公开事务控制流与 Commit 未知测试](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)
- [真实 MySQL Repository 集成测试](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)

### 15.3 Domain hydration 与 schema/权限

- [Strategy 的 New/Restore 边界](../../../internal/lottery/domain/strategy.go)
- [Award 的 New/Restore 边界](../../../internal/lottery/domain/award.go)
- [Lottery schema 集成与权限负向测试](../../../migrations/lottery_schema_integration_test.go)
- [应用授权收敛脚本](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)
- [Compose 拓扑](../../../deploy/compose/compose.yaml)
- [Compose smoke](../../../scripts/compose-smoke.sh)
- [MySQL 集成双重授权入口](../../../Makefile)

### 15.4 前置决策与上一时间切片

- [第 17 节领域对象课程](../../course/part-03/lesson-17-lottery-domain-objects.md)
- [第 18 节关系结构课程](../../course/part-03/lesson-18-lottery-schema.md)
- [第 18 节设计手记](lesson-18.md)
- [ADR-0010：MySQL Migration 边界](../../decisions/ADR-0010-mysql-migration-boundaries.md)
- [ADR-0013：Lottery 领域模型](../../decisions/ADR-0013-lottery-domain-model.md)
- [ADR-0014：Lottery 持久化结构](../../decisions/ADR-0014-lottery-persistence-schema.md)
- [ADR-0015：Compose 授权收敛](../../decisions/ADR-0015-compose-schema-grant-reconciliation.md)

### 15.5 本节学习、契约与验收

- [第 19 节课程](../../course/part-03/lesson-19-lottery-repository.md)
- [第 19 节 API 记录](../../api/lessons/lesson-19.md)
- [第 19 节 QA](../../qa/lessons/lesson-19.md)
- [第 19 节面试问答](../../interview/lessons/lesson-19.md)
- [ADR-0016：Lottery Repository 事务与错误边界](../../decisions/ADR-0016-lottery-repository-boundaries.md)

### 15.6 官方语义依据

- [Go：Executing transactions](https://go.dev/doc/database/execute-transactions)
- [Go：Canceling in-progress operations](https://go.dev/doc/database/cancel-operations)
- [Go `database/sql`](https://pkg.go.dev/database/sql)
- [Go Code Review Comments：Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)
- [go-sql-driver/mysql `MySQLError`](https://pkg.go.dev/github.com/go-sql-driver/mysql#MySQLError)
- [MySQL 8.4：Consistent Nonlocking Reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)
- [MySQL 8.4：Transaction Isolation Levels](https://dev.mysql.com/doc/refman/8.4/en/innodb-transaction-isolation-levels.html)
- [MySQL 8.4：START TRANSACTION、COMMIT 与 ROLLBACK](https://dev.mysql.com/doc/refman/8.4/en/commit.html)
- [MySQL 8.4：Handling Deadlocks](https://dev.mysql.com/doc/refman/8.4/en/innodb-deadlocks-handling.html)
- [MySQL 8.4：GRANT](https://dev.mysql.com/doc/refman/8.4/en/grant.html)
- [MySQL 8.4 Server Error Reference](https://dev.mysql.com/doc/mysql-errors/8.4/en/server-error-reference.html)

### 15.7 证据总边界

当前证据能支持的最强结论是：在固定 MySQL 8.4.11、当前 schema、Go driver/sqlx 版本与本地 Compose 权限拓扑内，合法 Strategy 可以以聚合事务创建，并从一个 read-only RR snapshot 完整恢复；duplicate、commit unknown、retryable、corrupt data 和普通 dependency failure 不会被同一个模糊错误掩盖；应用身份的直接数据库能力被收敛到这四条 SQL 所需的两表 SELECT+INSERT。

它仍然不能证明 Repository 已被运行时业务使用，不能证明高并发性能、生产容灾、读副本一致性、自动重试/对账、幂等、更新语义、抽奖公平、API 安全或端到端业务成功。真正像架构师思考，最后一步不是把未知写成“后续优化”，而是把它们变成带触发器、负责人和证据要求的下一次决策。
