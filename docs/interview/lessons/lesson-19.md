# 第 19 节面试问答：Lottery Repository、事务快照与错误语义

本文只描述实现提交 `50ac811` 与证据加固提交 `2c420c9` 已经进入代码的能力：面向 Strategy 聚合的窄接口、MySQL Create 与 FindByID、父子表事务、REPEATABLE READ 只读快照、领域恢复、稳定错误分类、Context 取消、精确表级 SELECT/INSERT 权限，以及单元、sqlmock 和真实 MySQL 分层验证。它没有把 Repository 接入 HTTP 路由，也没有实现 Update、Save、Upsert、Delete、List、加权随机、库存/权益发放、Redis 缓存或前端抽奖页。

## 60 秒项目自述

第 19 节把上一节的两张 Lottery 表变成了一个真正按聚合工作的 MySQL Repository。我没有先做通用 CRUD，而是在 application 包定义消费者拥有的 StrategyCreator 和 StrategyReader 两个窄端口，MySQL adapter 返回具体 Repository。Create 先用领域恢复规则重新校验完整 Strategy，再在一个事务里插入根和全部 Award；Award 按稳定 ID 顺序写入，任一子项失败就回滚，重复根身份只在根 INSERT 的 1062 上分类为 already exists。FindByID 在一个显式 REPEATABLE READ、read-only 事务内先读根再读按 AwardID 排序的子项，保证两次查询来自同一 MVCC 快照；事务结束后再用 RestoreAward/RestoreStrategy 精确重建，空奖项、非规范 Unicode 名称或总权重溢出都会 fail closed。错误对外只暴露稳定语义，1205/1213 标成可重试但 adapter 不自动重试，写 COMMIT 的驱动错误单独标成 outcome unknown，避免盲目重放。验证上，sqlmock 只锁定调用顺序和提交失败分支，真实 MySQL 才验证 MVCC、约束、并发创建、取消、权限与 EXPLAIN。这个切片完成的是持久化端口，不是在线抽奖接口。

## 来源说明

- **项目事实：** 以实现 `50ac811` 和证据加固 `2c420c9` 的 Go 代码、测试、Migration 与授权脚本为准；命令是否实际通过、在哪个 MySQL 版本运行，以[第 19 节 QA](../../qa/lessons/lesson-19.md)记录为准。本文不能反向替代 QA 验收。
- **官方事实：** 主要使用 Go 官方 database/sql 文档、Go Code Review Comments、go-sql-driver/mysql v1.10.0 源码和 MySQL 8.4 Reference Manual。官方资料说明机制，不替项目证明某个测试已经执行。
- **面经启发：** 牛客链接是候选人自述，只用于说明“项目深挖、幂等、事务隔离、MVCC、Context、索引、两表原子性”等追问方向确实出现在复盘中。公司、部门、轮次与措辞均按作者自述，本文没有独立核验面试原题。
- 这些链接在 2026-08-29 做过可访问性与正文检查，本章引用的 10 篇牛客复盘当次均可见正文；动态页面后续可访问性仍可能变化。本文只做短句归纳，技术答案全部回到官方资料和项目证据。

本章主要题型参考包括：[字节社招作者自述：项目、接口幂等与事务](https://www.nowcoder.com/discuss/353157357217718272)、[字节实习作者自述：MVCC、MySQL 锁与项目深挖](https://www.nowcoder.com/discuss/353159669701091328)、[字节商业化作者自述：事务隔离与 B+ 树](https://www.nowcoder.com/discuss/353159293174226944)、[字节 Go 实习作者自述：隔离级别与 Context](https://www.nowcoder.com/discuss/353156142509531136)、[美团作者自述：项目深挖与 MySQL 隔离级别](https://www.nowcoder.com/discuss/471056135512936448)、[美团到家作者自述：事务、幻读与锁](https://www.nowcoder.com/discuss/614236533700194304)、[阿里系作者自述：两表原子性、索引与乐观锁](https://www.nowcoder.com/discuss/523595426050666496)、[阿里作者自述：提交过程、MVCC 与隔离性](https://www.nowcoder.com/discuss/353156645666627584)、[多家公司 Go 实习作者自述：InnoDB、隔离与锁](https://www.nowcoder.com/discuss/353155683119996928)、[Go 面经作者自述：Context 与数据库索引](https://www.nowcoder.com/discuss/353154773744558080)。

## 1. 第 19 节到底解决了什么问题，为什么不直接做抽奖 API？

- **直接回答：** 它解决“一个合法 Strategy 聚合怎样可靠地进入 MySQL、又怎样完整地恢复出来”。Repository 是领域与存储之间的防腐边界：写入要保持根和全部 Award 原子，读取要得到同一快照，并把数据库行重新送回领域不变量。HTTP 身份、请求 DTO、幂等键、鉴权和状态码属于下一层；加权选择、库存与发奖又是其他用例。先把这些混在一个 handler 里会让事务、错误和业务边界无法独立验证。
- **追问：** 没有 HTTP，Create 算真实业务能力吗？
  - **追问回答：** 它是可调用、可集成测试的持久化能力，但还不是用户可访问的线上功能。面试时应说“完成 Repository adapter”，不能说“上线创建抽奖策略接口”。当前没有 composition root 把它注入 server，也没有 Lottery 路由。
- **项目证据：** [Repository 实现](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[application 端口](../../../internal/lottery/application/repository.go)、[当前 HTTP 路由](../../../internal/infrastructure/httpapi/router.go)。
- **选型边界：** 很小的内部脚本可以直接使用 database/sql，不一定都要 Repository；一旦需要保护聚合不变量、替换存储或稳定错误语义，这个边界才值回复杂度。
- **来源：** 面经启发来自[字节社招作者关于项目细节和方案取舍的复盘](https://www.nowcoder.com/discuss/353157357217718272)；项目事实来自上述代码。

## 2. Repository 和 DAO 有什么区别？这只是改了名字吗？

- **直接回答：** 不是名字区别，而是抽象对象不同。DAO 往往围绕表行提供增删改查；本节 Repository 围绕 Strategy 聚合提供 Create 与 FindByID。调用方交付的是完整领域对象，adapter 自己负责两张表、事务、排序、错误映射和恢复，不把 storedStrategy/storedAward 或 SQL 细节泄漏给 application。这样“至少一个 Award、AwardID 作用域、总权重不溢出”等规则仍以聚合为单位成立。
- **追问：** 如果运营后台只想修改一个 Award，Repository 会不会很笨重？
  - **追问回答：** 当前没有这个用例，所以不能先暴露表级 UpdateAward。未来若局部修改是合法业务动作，应先定义并发版本、完整聚合校验、审计和发布语义，再决定是聚合命令还是专用写模型；不能为了少写 SQL 绕过根。
- **项目证据：** [聚合端口只收发 Strategy](../../../internal/lottery/application/repository.go)、[两表 adapter 与私有 stored row](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[Strategy 聚合规则](../../../internal/lottery/domain/strategy.go)。
- **选型边界：** 查询报表、批处理或纯 ETL 不一定适合领域 Repository，可以使用专用 query/DAO；不要把 Repository 强行覆盖所有数据访问。
- **来源：** 面经启发来自[阿里系作者记录的“两张表如何保证原子性”项目追问](https://www.nowcoder.com/discuss/523595426050666496)；官方事务事实见[Go 执行事务指南](https://go.dev/doc/database/execute-transactions)。

## 3. 为什么拆成 StrategyCreator 和 StrategyReader 两个窄接口，而不是一个 Repository 大接口？

- **直接回答：** 接口由消费者需要的能力定义，而不是把实现未来可能有的方法全部公开。写用例只依赖 Create，读用例只依赖 FindByID；它们不会被迫获得 Update、Delete、List 或事务控制权。窄接口降低偶然耦合，也使未来读写 adapter 分离、测试替身和权限审查更直接。MySQL 实现用编译期断言证明同时满足两个端口。
- **追问：** 为什么不为每个函数都定义接口，岂不是更“解耦”？
  - **追问回答：** 接口仍应来自真实消费者，而不是为抽象而抽象。两个端口表达当前两个用例方向；如果调用方直接需要一个具体类型且没有替换点，额外接口没有价值。关键是最小可用契约，不是接口数量。
- **项目证据：** [消费者侧两个端口](../../../internal/lottery/application/repository.go)、[adapter 编译期契约](../../../internal/lottery/adapter/mysqlrepo/repository.go)。
- **选型边界：** 当多个方法必须共享一个业务原子协议时，可以形成更大的用例接口；但不能把 CRUD 全家桶当作默认起点。
- **来源：** 官方事实见[Go Code Review Comments：接口通常属于使用方](https://go.dev/wiki/CodeReviewComments#interfaces)；项目事实见上述端口。

## 4. 为什么 mysqlrepo.New 返回具体指针，而不是返回接口？

- **直接回答：** 构造器属于实现包，返回具体 Repository 让实现未来增加方法而不破坏调用方；需要抽象的 application 在自己的边界用 StrategyCreator/StrategyReader 接收它。这样接口所有权仍在消费者侧，也避免 adapter 为“方便 mock”自造大接口。New 只检查数据库句柄非空，不偷偷 Ping、迁移或接管生命周期。
- **追问：** 单元测试怎么替换实现？
  - **追问回答：** 用例层可以接收自己定义的窄接口并提供小型 fake；adapter 本身则通过真实公开方法、sqlmock 和 MySQL 集成测试验证。没有必要让构造器返回接口才能测试。
- **项目证据：** [New 与具体 Repository](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[application 接口](../../../internal/lottery/application/repository.go)、[公开路径 sqlmock 测试](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)。
- **选型边界：** 插件注册或运行时工厂确实可能需要返回统一接口；当前是静态 composition，具体返回更简单。
- **来源：** 官方事实见[Go 接口评审建议](https://go.dev/wiki/CodeReviewComments#interfaces)；项目事实见上述实现。

## 5. 为什么只有 Create，没有 Save、Update 或 Upsert？Create 是幂等的吗？

- **直接回答：** 当前稳定用例只有“以显式身份创建一个完整聚合”。Save 会把创建还是覆盖藏起来，Upsert 会把重复请求、配置冲突甚至事故误写成成功，Update 又需要版本、丢失更新与发布协议。Create 因此是 create-only：同一 StrategyID 已存在就返回 ErrStrategyAlreadyExists，不把重复身份当幂等成功。它本身不是幂等接口。
- **追问：** 相同内容重放，为什么不能直接返回成功？
  - **追问回答：** 因为 Repository 没有请求身份，只看到资源身份；相同 ID 可能是网络重试，也可能是调用者错误或并发竞争。要支持业务幂等，应在更高层引入 idempotency key、请求摘要和可查询结果，再明确同 key 不同 payload 的冲突语义。
- **项目证据：** [Create 注释与重复分类](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[重复与并发创建集成测试](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[无 Update/Upsert 的端口](../../../internal/lottery/application/repository.go)。
- **选型边界：** 事件投影、可重复同步或声明式配置可能适合幂等 Upsert；面向受审计业务资源的首次创建默认应区分重复。
- **来源：** 面经启发来自[字节社招作者记录的“项目接口如何保证幂等”](https://www.nowcoder.com/discuss/353157357217718272)；项目选择见上述代码。

## 6. 为什么 Create 必须把父表和所有 Award 放在一个事务里？

- **直接回答：** Strategy 的合法状态要求根与至少一个合法 Award 同时存在。先提交父再逐个提交子，任一子写失败都会留下领域上不可恢复的空或半聚合；先写子又会被外键拒绝。Create 因此 BeginTxx 后写根、复用子项 statement 写完全部 Award，检查每次影响一行，最后只 Commit 一次；任何中途错误由 deferred Rollback 收口。
- **追问：** 外键和 CHECK 已经在数据库，为什么事务还必要？
  - **追问回答：** 外键只保证子有父，CHECK 只约束单行，都不保证“父和全部预期子一起可见”。事务负责跨语句原子性，领域负责跨行不变量，两者不能互相替代。
- **项目证据：** [Create 事务流程](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[临时精确 CHECK 触发子项失败并验证父回滚](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[两表 DDL](../../../migrations/sql/README.md)。
- **选型边界：** 跨数据库或跨服务聚合无法直接依赖单库 ACID，需要 outbox、Saga、补偿和对账；当前两表同属一个 InnoDB schema，单事务最直接。
- **来源：** 官方事实见[Go 事务流程](https://go.dev/doc/database/execute-transactions)和[MySQL COMMIT/ROLLBACK](https://dev.mysql.com/doc/refman/8.4/en/commit.html)；面经启发见[阿里系作者的两表原子性追问](https://www.nowcoder.com/discuss/523595426050666496)。

## 7. 为什么不先 SELECT EXISTS，再决定 INSERT？

- **直接回答：** “先查不存在再插入”存在 TOCTOU：两个并发事务都可能看到不存在，然后同时写。唯一键才是并发仲裁点；本节直接 INSERT 根，让 MySQL 主键决定唯一胜者，再把根 INSERT 的 1062 映射成 already exists。这样少一次往返，也不把预检查误当互斥锁。
- **追问：** 预检查是不是完全没用？
  - **追问回答：** 它可以改善某些交互提示或提前发现明显冲突，但不能替代唯一约束。即使做预检查，仍必须正确处理最终 INSERT 的 1062。
- **项目证据：** [直接根 INSERT](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[两个 goroutine 同时 Create 恰好一成一败](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[Strategy 主键](../../../migrations/sql/000001_create_lottery_strategy.up.sql)。
- **选型边界：** 需要序列化复杂条件时可能使用锁、版本或可串行化事务；单一身份唯一性优先交给唯一索引。
- **来源：** 面经启发来自[字节实习作者记录的并发与 MySQL 锁追问](https://www.nowcoder.com/discuss/353159669701091328)；官方错误号见[MySQL 8.4 ER_DUP_ENTRY 1062](https://dev.mysql.com/doc/mysql-errors/8.4/en/server-error-reference.html)。

## 8. 为什么 Award 要按稳定顺序写入，并只 Prepare 一次子项 INSERT？

- **直接回答：** Strategy 在恢复时按 AwardID 排成规范顺序，Create 重新校验后按这个顺序写。统一迭代顺序消除了调用方切片顺序带来的执行差异，让测试与故障定位可复现；在当前 create-only 路径，它主要是确定性约束，不声称已经测得死锁率下降。子项 SQL 在事务内 Prepare 一次、循环复用并在 Commit 前关闭，避免每个 Award 重复准备；每次 RowsAffected 必须等于 1，异常结果 fail closed。
- **追问：** 为什么不用一条 multi-row INSERT？
  - **追问回答：** 多值 INSERT 可减少往返，Award 很多时可能更合适；当前聚合小、逐项错误位置清楚、参数数量稳定，单 Prepare 循环更易读。未来优化要基于 award 数量和基准，并处理 packet 大小及整句失败语义。
- **项目证据：** [Strategy 规范排序](../../../internal/lottery/domain/strategy.go)、[Prepare/循环/RowsAffected 检查](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[SQL 顺序与 statement close 测试](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)。
- **选型边界：** 未来增加会争用相同记录的写路径时，统一锁序可能降低某类死锁，但不能替代死锁重试；批量很大时应评估批次、多值写或导入协议。
- **来源：** 官方建议包括[事务应短小并准备处理死锁](https://dev.mysql.com/doc/refman/8.4/en/innodb-deadlocks-handling.html)；项目事实见上述实现。

## 9. 如何防 SQL 注入，并证明 Unicode、特殊字符和 uint64 没被破坏？

- **直接回答：** 所有数据值都通过问号占位符作为参数传给 database/sql；SQL 标识符是源码中的固定表列名，没有把名称或 ID 用字符串拼进 SQL。真实 MySQL round-trip 使用包含单引号、问号、类似注释片段和分解 Unicode 的名称，并验证 MaxUint64 的 StrategyID、AwardID、Weight 原样恢复。这同时检查 driver 绑定、scan 与 schema 映射，不只是看 SQL 字符串。
- **追问：** 使用 PreparedStatement 就能解决所有输入安全问题吗？
  - **追问回答：** 不能。参数化主要防值进入 SQL 语法；仍需领域长度、控制字符、Unicode 规范、权限和输出编码。动态表名/排序字段也不能靠值占位符，必须做固定 allowlist。
- **项目证据：** [固定 SQL 与参数绑定](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[特殊名称和 MaxUint64 真实回环](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[领域名称校验](../../../internal/lottery/domain/name.go)。
- **选型边界：** PostgreSQL 等 driver 的 placeholder 语法不同；迁移数据库时应改 adapter，不能让 application 感知。
- **来源：** 官方事实见[Go：避免 SQL 注入](https://go.dev/doc/database/sql-injection)；项目事实见上述集成测试。

## 10. FindByID 为什么用两次查询，而不是一条 JOIN？

- **直接回答：** 根和子项是 1:N。JOIN 会为每个 Award 重复 Strategy 列；若要识别“根存在但无 Award”，LEFT JOIN 还要处理 NULL 子行，INNER JOIN 则会直接丢失这个根。两条查询先得到唯一根，再得到按 AwardID 排序的子项，扫描模型和错误分类更直接。额外一次往返由同一事务承担，换来清晰的聚合重建。当前 Award 数量小，没有证据表明 JOIN 更快。
- **追问：** 两次查询会不会读到不同版本？
  - **追问回答：** 如果直接各用 DB 查询会有风险，所以两次查询都通过同一个 REPEATABLE READ transaction 执行；第一条一致性读建立的快照被第二条复用。
- **项目证据：** [两条查询和私有 row model](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[完整聚合回环测试](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[公开路径顺序测试](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)。
- **选型边界：** 高延迟网络、子项极少且投影简单时 JOIN 可能更优；应以真实基准、返回体与执行计划决定，而不是把“两查询”当教条。
- **来源：** 面经启发来自[阿里作者关于 MySQL JOIN、查询过程和 MVCC 的复盘](https://www.nowcoder.com/discuss/353156645666627584)；官方事务连接事实见[database/sql](https://pkg.go.dev/database/sql)。

## 11. 为什么读取也要开事务，而且必须是同一个事务？

- **直接回答：** FindByID 要把两张表恢复成一个时点的 Strategy。database/sql 的 Tx 绑定单连接；MySQL InnoDB 在 REPEATABLE READ 下，让同一事务内的普通一致性 SELECT 共用第一次读建立的快照。因此根查询与 Award 查询必须都走 tx，不能中间调用 DB。否则并发事务恰好在两次读之间提交时，当前查询顺序可能组合出“旧根+新 Award”的从未整体读取状态。
- **追问：** 单条 SELECT 本身不是原子的吗？
  - **追问回答：** 每条 SELECT 各自有一致视图，但聚合读取跨两条语句；问题正发生在语句之间。事务把两次读的观察边界合并为一个快照。
- **项目证据：** [FindByID 的单事务](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[旧读者保持旧根/旧 Award，新 Find 看到新根/新增 Award](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[sqlmock 顺序断言](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)。
- **选型边界：** 如果聚合被存成单行文档或由单条查询一次性返回，可能无需显式多语句快照；跨语句一致性仍需重新论证。
- **来源：** 官方事实见[database/sql：Tx 绑定单连接](https://pkg.go.dev/database/sql)和[MySQL 一致性非锁定读](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)；面经启发见[阿里作者的提交、隔离与 MVCC 追问](https://www.nowcoder.com/discuss/353156645666627584)。

## 12. 为什么选 REPEATABLE READ，而不是 READ COMMITTED、SERIALIZABLE 或 FOR UPDATE？

- **直接回答：** READ COMMITTED 每次一致性读建立新快照，根与子查询之间仍可能看到并发提交；REPEATABLE READ 恰好满足“同一读事务内保持一个快照”。SERIALIZABLE 更强但会引入不必要的锁/冲突；FOR UPDATE/FOR SHARE 是锁定读，会阻塞发布者，而当前只是读取不可变配置快照，不做 read-modify-write。显式 TxOptions 避免依赖环境默认隔离级别。
- **追问：** REPEATABLE READ 能保证所有并发正确性吗？
  - **追问回答：** 不能。它只保证本读流程的普通一致性 SELECT 共享快照；写写冲突、跨服务约束、DDL 变化和业务发布版本仍有各自协议。项目也没有声称串行化。
- **项目证据：** [显式 LevelRepeatableRead 与 ReadOnly 共用 helper](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[真实隔离级别、只读写拒绝与并发快照场景](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)。
- **选型边界：** 单条查询、允许近实时拼接或报表更看重新鲜度时 READ COMMITTED 可合理；真正 read-modify-write 可能需要锁或乐观版本；金融级跨条件串行约束才考虑 SERIALIZABLE。
- **来源：** 官方事实见[MySQL 8.4 隔离级别](https://dev.mysql.com/doc/refman/8.4/en/innodb-transaction-isolation-levels.html)和[一致性读快照](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)；面经启发见[字节商业化作者的隔离实现追问](https://www.nowcoder.com/discuss/353159293174226944)与[美团作者的隔离级别题型](https://www.nowcoder.com/discuss/471056135512936448)。

## 13. ReadOnly: true 只是注释吗？它能替代数据库权限吗？

- **直接回答：** 不是注释。go-sql-driver/mysql v1.10.0 会先设置隔离级别，再把只读选项映射为 START TRANSACTION READ ONLY；MySQL 会禁止该事务修改或锁定其他事务可见的普通表，并可能应用只读优化。但它只是这一笔事务的访问模式，不是账号授权，也不能替代 GRANT；临时表还有例外。
- **追问：** 既然应用账号有 INSERT，读方法会不会误写？
  - **追问回答：** ReadOnly 给 FindByID 增加一层运行时护栏；更重要的是代码只执行固定 SELECT，接口也不暴露事务。账号 INSERT 是 Create 的最小需要，不能把整个账号设为只读。
- **项目证据：** [FindByID TxOptions](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[精确 SELECT/INSERT 授权](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)。
- **选型边界：** driver 或数据库不支持相同 TxOptions 映射时必须验证实际行为；不要跨数据库假设 ReadOnly 语义完全一致。
- **来源：** 官方事实见[MySQL READ ONLY 事务](https://dev.mysql.com/doc/refman/8.4/en/commit.html)和[go-sql-driver/mysql v1.10.0 BeginTx 实现](https://github.com/go-sql-driver/mysql/blob/v1.10.0/connection.go)；项目事实见上述代码。

## 14. 为什么 SQL 还要显式 ORDER BY award_id？EXPLAIN 验证了什么？

- **直接回答：** 没有 ORDER BY，SQL 结果顺序不属于契约，即使当前 InnoDB 主键布局看起来有序也不能依赖。查询明确按 AwardID 排序，与领域规范顺序一致。真实 MySQL EXPLAIN 检查根查询以 PRIMARY 做 const access，子查询使用复合主键的 strategy_id 左前缀、key_len 为 8，且没有 Using filesort；这证明当前 schema 与查询形状匹配。
- **追问：** EXPLAIN 通过是不是就代表生产性能好？
  - **追问回答：** 不是。它只证明测试数据和版本下的访问路径，没有证明大基数、buffer pool、锁等待、网络、P95/P99 或生产统计信息。性能结论还要真实数据分布、基准和观测。
- **项目证据：** [ORDER BY 查询](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[EXPLAIN 断言](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[Award 复合主键](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql)。
- **选型边界：** 若未来排序契约改为运营优先级，应新增显式列与索引；不能把 AwardID 偶然当展示顺序。
- **来源：** 面经启发来自[美团作者的 B+ 树与隔离追问](https://www.nowcoder.com/discuss/471056135512936448)；官方事实见[MySQL EXPLAIN 输出](https://dev.mysql.com/doc/refman/8.4/en/explain-output.html)。

## 15. 为什么领域同时需要 New 和 Restore？读取时直接调用 New 不行吗？

- **直接回答：** New 面向外部新输入，会用 TrimSpace 把名称规范化后再建立对象；Restore 面向已经持久化的权威快照，只接受本来就规范的数据，绝不静默 trim 或修复。如果存储里有首尾 NBSP、全角空格或其他需要变化才能合法的值，调用 New 会把坏事实洗成好对象，掩盖数据漂移；Restore 必须拒绝。规范但字节不同的 Unicode，例如分解形式 e + combining acute，则原样保留，不擅自 normalization。
- **追问：** 写入 Create 为什么还要 RestoreStrategy，参数不是已经是领域对象吗？
  - **追问回答：** Go 值可能来自零值、旧代码、反射/反序列化或未来包内构造路径；adapter 在信任边界重新验证完整快照，成本很小。它不是深拷贝所有世界状态，而是确保准备写入的聚合满足当前不变量并得到稳定 Award 顺序。
- **项目证据：** [New/Restore Strategy](../../../internal/lottery/domain/strategy.go)、[New/Restore Award](../../../internal/lottery/domain/award.go)、[Unicode 恢复测试](../../../internal/lottery/domain/award_test.go)、[Create 写前复验](../../../internal/lottery/adapter/mysqlrepo/repository.go)。
- **选型边界：** 若存储迁移明确要修复历史值，应写可审计的一次性迁移并统计影响，而不是让每次读取悄悄改值；事件溯源还需要按事件版本升级的独立策略。
- **来源：** 项目场景题；项目事实来自上述领域代码，数据库快照背景见[MySQL 一致性读](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)。

## 16. 数据库行能读出来，为什么还可能返回 stored strategy invalid？

- **直接回答：** 数据库约束只覆盖单行子集，不能保证父至少有一个 Award、跨行总权重不溢出，也不能完整复制 Go 的 Unicode/控制字符规则。FindByID 先把快照读完并结束事务，再逐个 RestoreAward、最后 RestoreStrategy；任何规则失败都统一返回 ErrStoredStrategyInvalid，并返回零 Strategy，绝不交付部分聚合。真实测试由迁移身份插入四类坏快照：空 Award、NBSP 根名称、控制字符 Award 名称、总权重溢出。
- **追问：** 为什么不把具体哪一行坏了直接返回给 API？
  - **追问回答：** 对外稳定语义应是“存储聚合不可用”，避免泄露原始内容、SQL 或驱动细节；受信任日志/指标可以通过保留的 cause、StrategyID 和审查过的标签定位。用户请求不能继续抽奖，也不能拿半份数据降级。
- **项目证据：** [restoreStrategy fail closed](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[四类坏快照集成测试](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[领域不变量](../../../internal/lottery/domain/strategy.go)。
- **选型边界：** 报表可选择逐行隔离坏记录并标注质量，但决策型抽奖配置必须整体有效；不能把分析系统的容错策略搬到在线决策。
- **来源：** 项目场景题；官方快照机制见[MySQL 8.4 一致性非锁定读](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)。

## 17. RepositoryError 为什么既隐藏底层消息，又保留 cause？会不会自相矛盾？

- **直接回答：** Error 方法只渲染审查过的语义类，例如 not found、retryable、outcome unknown，不把 SQL、账号、表名或驱动 message 带到用户边界；同时 Unwrap 保留原 cause，使受信任代码能用 errors.Is/errors.As 做诊断、指标或错误号审计。未知 class、零值和 typed nil 都 fail closed 为通用 storage failure。
- **追问：** 既然能 Unwrap，日志是不是自动安全？
  - **追问回答：** 不是。调用者若主动展开 error chain 或记录底层 MySQLError，仍可能暴露敏感细节。安全渲染只保护默认 Error 字符串；日志策略还要分信任域、脱敏和访问控制，HTTP 层只能映射稳定 class。
- **项目证据：** [错误类型与白名单](../../../internal/lottery/application/repository_error.go)、[安全文本与 cause 测试](../../../internal/lottery/application/repository_error_test.go)、[集成测试的安全字符串断言](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)。
- **选型边界：** 内部批处理可以返回更丰富结构化诊断，但不能把 driver message 当稳定 API；需要跨进程传输时应定义显式 error code，而不是序列化 Go error chain。
- **来源：** 项目场景题；driver 可检查的错误结构见[go-sql-driver/mysql MySQLError](https://pkg.go.dev/github.com/go-sql-driver/mysql#MySQLError)。

## 18. not found、非法参数、坏数据和数据库故障为什么要分开？

- **直接回答：** 它们要求不同处置。StrategyID 为零是领域调用错误；找不到根是稳定业务结果 ErrStrategyNotFound；根存在但不能恢复是数据完整性事故 ErrStoredStrategyInvalid；权限、schema、scan 或未知 driver 错误是 ErrRepositoryFailure。若都返回一个 error，调用方可能把事故误报 404，或把用户输入问题误重试成数据库风暴。
- **追问：** sql.ErrNoRows 为什么只在根查询映射 not found？
  - **追问回答：** 根是聚合存在性的判据。子查询返回零行不是 sql.ErrNoRows，而是空切片；随后 RestoreStrategy 把它判为 stored invalid，因为数据库有根但领域聚合不完整。两种状态不能混为同一个 404。
- **项目证据：** [FindByID 分类流程](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[错误类定义](../../../internal/lottery/application/repository_error.go)、[missing 与 corrupt 集成断言](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)。
- **选型边界：** 如果未来引入 draft Strategy，零 Award 可能成为合法草稿状态；那要显式建模状态与查询契约，而不是简单把 invalid 改成 not found。
- **来源：** 项目场景题；Go 行查询错误语义见[database/sql](https://pkg.go.dev/database/sql)。

## 19. 为什么只有根 INSERT 的 1062 才映射 already exists？

- **直接回答：** 在 Create 语义中，根主键冲突准确表示 Strategy 身份已存在，所以根 INSERT 的 MySQLError 1062 映射 ErrStrategyAlreadyExists。Award 的 1062 不一定是调用者重复创建：领域已拒绝同 Strategy 内重复 AwardID，子项冲突更可能意味着 schema 漂移、触发器、异常历史行或 adapter bug，因此按通用存储失败处理，不能伪装成“策略已存在”。
- **追问：** 只看错误号够吗，要不要解析 key 名？
  - **追问回答：** 当前分类点已经被语句位置限定为根 INSERT，表只有预期身份唯一性，因此无需解析不稳定 message。若根表以后新增其他 unique constraint，就应结合约束名/业务语义重审，不能继续把所有根 1062 都说成 ID 重复。
- **项目证据：** [classifyRootInsertError](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[1062 作用域单元测试](../../../internal/lottery/adapter/mysqlrepo/repository_test.go)、[重复/并发创建集成测试](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)。
- **选型边界：** PostgreSQL 等数据库错误码与约束信息不同；语义类保持在 application，具体解析属于各 adapter。
- **来源：** 官方事实见[MySQL 8.4 ER_DUP_ENTRY 1062](https://dev.mysql.com/doc/mysql-errors/8.4/en/server-error-reference.html)和[MySQLError Number 字段](https://pkg.go.dev/github.com/go-sql-driver/mysql#MySQLError)；面经启发见[字节社招作者的接口幂等追问](https://www.nowcoder.com/discuss/353157357217718272)。

## 20. 1205 与 1213 为什么标成 retryable，却不在 Repository 内自动重试？

- **直接回答：** 1205 是 lock wait timeout，1213 是 deadlock victim；二者常可通过重放完整事务成功，所以 adapter 映射为 ErrRepositoryRetryable。但“可重试”不是“立即无限重试”：上层才知道请求 deadline、退避/jitter、重试预算、操作幂等性和是否需要告警。Repository 内部自动循环会隐藏延迟、放大争用，并让测试与容量不可控。
- **追问：** Create 收到 retryable 后一定可以重试吗？
  - **追问回答：** 只能在确认失败发生于 Commit 结果明确之前、并重放整个事务时评估重试；不能只重放某条子 INSERT。MySQL 默认下 1205 只回滚超时语句，但本 Repository 收到错误后立即退出并由 deferred Rollback 回滚整笔事务，所以上层只能在方法返回后重放完整 Create。若错误属于 ErrCommitOutcomeUnknown，则绝不能盲重试，应先按业务身份查证。
- **项目证据：** [1205/1213 分类且无 retry loop](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[1213 等分类单元测试](../../../internal/lottery/adapter/mysqlrepo/repository_test.go)、[真实 1205、整笔回滚与 Context 阻塞取消场景](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)。
- **选型边界：** 基础设施层若有统一、可观测、有限次数且只覆盖幂等读/明确回滚事务的重试器，可以集中实现；本节 adapter 不拥有这些策略。
- **来源：** 官方事实见[MySQL 1205/1213 错误参考](https://dev.mysql.com/doc/mysql-errors/8.4/en/server-error-reference.html)和[死锁处理建议](https://dev.mysql.com/doc/refman/8.4/en/innodb-deadlocks-handling.html)；面经启发见[阿里系作者关于死锁与业务超时的项目深挖](https://www.nowcoder.com/discuss/523595426050666496)。

## 21. 什么是 commit outcome unknown？为什么 Commit 返回错误不能直接说回滚了？

- **直接回答：** 写事务发送 COMMIT 后，服务器可能已经持久化，但客户端在收到确认前断网；此时 driver 只能返回错误，调用方无法从该响应判断是否生效。go-sql-driver/mysql v1.10.0 的 Commit 本质上执行 COMMIT 并返回结果，因此本节把非明确取消的写提交错误分类为 ErrCommitOutcomeUnknown，而不是 repository failure 或 retryable。盲目重放可能产生重复副作用。
- **追问：** Commit 前检查 ctx.Err 能消除这个窗口吗？
  - **追问回答：** 不能。它只避免已知取消后主动发送 Commit；检查与网络提交之间仍有竞态。若 transaction error 与 context error 或 sql.ErrTxDone 明确对应，返回取消；若同时出现不相关 driver commit error，仍优先 unknown outcome，避免把不确定写错说成取消。
- **项目证据：** [写/读 Commit 分类](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[竞态分类单元测试](../../../internal/lottery/adapter/mysqlrepo/repository_test.go)、[公开 Create 的 commit failure sqlmock 测试](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)。
- **选型边界：** 当前只分类不解决查证；未来 HTTP 创建需用业务 ID/idempotency key、状态查询或事务结果表来收敛不确定性。真正 exactly-once 不能靠一次 Commit 调用承诺。
- **来源：** 官方事实见[MySQL COMMIT 使变更永久](https://dev.mysql.com/doc/refman/8.4/en/commit.html)、[database/sql Tx.Commit](https://pkg.go.dev/database/sql)和[go-sql-driver/mysql v1.10.0 Commit 源码](https://github.com/go-sql-driver/mysql/blob/v1.10.0/transaction.go)；“响应丢失导致结果未知”是基于请求/确认边界的工程推断，项目通过可控 driver failure 验证分类，不声称做过真实断网故障注入。

## 22. Context 在 Repository 中如何传播？取消就等于数据库绝不会写入吗？

- **直接回答：** Create/FindByID 从 BeginTxx 到 ExecContext、QueryxContext、GetContext、PrepareContext 都传入调用方 ctx，并在 Commit 前再检查 ctx.Err。真实测试覆盖预取消读写和一个被 gap lock 阻塞到 deadline 的 Create，随后验证没有残留父子行。Context 能终止不再需要的工作并约束等待，但不能倒转已经成功提交的副作用，也不能消除前述 Commit 确认竞态。
- **追问：** 为什么禁止 nil Context，不自动换 Background？
  - **追问回答：** 静默换 Background 会丢失 deadline、追踪与取消，可能让锁等待无限延长。nil 是调用契约错误，立即返回 ErrRepositoryInvalidArgument；真正的后台任务应由调用者显式传 context.Background 或有界派生 context。
- **项目证据：** [Context 校验与全链路传递](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[nil/零 receiver 单元测试](../../../internal/lottery/adapter/mysqlrepo/repository_test.go)、[预取消与在途取消集成测试](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)。
- **选型边界：** 长任务可能需要与请求生命周期解耦的 durable job，而不是简单延长 HTTP context；仍要有独立 deadline、取消与状态机。
- **来源：** 官方事实见[Go：取消进行中的数据库操作](https://go.dev/doc/database/cancel-operations)；面经启发见[字节 Go 实习作者的 Context 题型](https://www.nowcoder.com/discuss/353156142509531136)和[Go 面经作者的 Context 题型](https://www.nowcoder.com/discuss/353154773744558080)。

## 23. Repository 为什么不 Close 数据库？连接池和事务连接分别归谁管？

- **直接回答：** sqlx.DB 包装的 sql.DB 是并发安全的共享连接池，应由 application composition root 创建、配置并在进程退出时关闭。Repository 只是借用它；如果某个 adapter 方法 Close，会影响其他模块。单个 Tx 会绑定池中的一条连接，Commit/Rollback 后归还；每条失败路径都 defer Rollback，读出的 Rows 与 prepared statement 也显式关闭。集成测试最后 Ping 共享池，验证操作后仍可用。
- **追问：** New 为什么不 Ping？
  - **追问回答：** 构造与连通性是不同责任。启动流程已有基础设施层的 Open/Ping/readiness；若 New 隐式联网，会让单元测试、依赖装配和错误时序难以控制。Repository 只验证句柄配置，不宣称数据库健康。
- **项目证据：** [借用池的类型注释与 New](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[Rows/statement/transaction 收口](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[最终 Ping 集成断言](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[MySQL pool 构造](../../../internal/infrastructure/mysql/open.go)。
- **选型边界：** 一次性 CLI 可以在 composition root defer Close；Repository 仍不应猜测自己是否独占池。专用连接级状态则应显式使用 Conn/Tx 并控制释放。
- **来源：** 官方事实见[database/sql：DB 并发安全、自带池，Tx 绑定单连接](https://pkg.go.dev/database/sql)和[Go 事务指南](https://go.dev/doc/database/execute-transactions)。

## 24. 为什么应用账号现在精确拥有两表 SELECT、INSERT，却没有 UPDATE、DELETE 或 schema_migrations 权限？

- **直接回答：** 权限必须跟已实现 SQL 一一对应：FindByID 需要两表 SELECT，Create 需要两表 INSERT；本节没有更新、删除、DDL 或迁移状态访问，因此都不授予。reconciliation 先撤销旧 grant，再精确比较排序后的 SHOW GRANTS，并要求 mandatory_roles 为空。真实 schema 集成测试正向验证两表 SELECT/INSERT，负向验证两表 UPDATE/DELETE 和 schema_migrations 的 SELECT/INSERT/UPDATE 都返回 1142。
- **追问：** 既然根/子写入靠事务，为什么不给应用 REFERENCES 或 ALTER？
  - **追问回答：** 外键已经由 migrator 建好，普通 INSERT 只需 INSERT；应用不应改变约束或 schema。迁移身份与运行身份分离，能缩小 bug 或注入后的破坏面。
- **项目证据：** [授权 reconciliation](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)、[精确 grant 与 1142 正负向测试](../../../migrations/lottery_schema_integration_test.go)、[Repository 固定 SQL](../../../internal/lottery/adapter/mysqlrepo/repository.go)。
- **选型边界：** 第一次实现 Update/Delete 时应按已审查语句扩权并补负向测试；最小权限是随能力演进的 allowlist，不是永久不变。云角色/代理身份还要重新评估继承权限。
- **来源：** 官方事实见[MySQL 8.4 GRANT](https://dev.mysql.com/doc/refman/8.4/en/grant.html)；项目事实见上述脚本和测试。

## 25. sqlmock 和真实 MySQL 测试各自证明什么？为什么两者都要？

- **直接回答：** sqlmock 适合低成本锁定 public control flow：FindByID 是 Begin→根查询→Award 查询→Commit，Create 的 driver Commit error 被分类为 outcome unknown，statement 也被关闭。它模拟 driver，不执行 InnoDB，所以不能证明 REPEATABLE READ、ReadOnly、外键、错误号、Unicode/uint64、EXPLAIN 或真实取消。真实 MySQL 集成测试负责这些引擎/driver/权限事实；普通单元测试则覆盖纯分类和领域规则。
- **追问：** sqlmock 的 Find 测试能证明 TxOptions 真被 MySQL 接受吗？
  - **追问回答：** 不能。当前 sqlmock expectation 只观察 Begin 与 SQL 顺序，不校验 MySQL 对隔离/只读选项的翻译。该结论由生产代码、固定 driver 源码与真实快照场景共同校准，不能把 mock 说成数据库证据。
- **项目证据：** [sqlmock 事务顺序与 commit error](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)、[真实 MySQL Repository 测试](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[纯分类测试](../../../internal/lottery/adapter/mysqlrepo/repository_test.go)。
- **选型边界：** SQL 很简单、真实测试足够快时可以减少 mock；复杂错误注入难以稳定在真库复现时 mock 仍有价值。两者不是互相替代。
- **来源：** 维护者资料见[go-sqlmock：模拟 sql/driver、无需真实连接](https://github.com/DATA-DOG/go-sqlmock/blob/v1.5.2/README.md)；官方数据库行为见[MySQL 隔离级别](https://dev.mysql.com/doc/refman/8.4/en/innodb-transaction-isolation-levels.html)。

## 26. 真实 MySQL 集成测试怎样避免误伤开发数据，又覆盖了哪些故障域？

- **直接回答：** 测试要求 schema-change 和 repository-write 两个精确 opt-in，要求 app/migrator 是不同账号且指向同一隔离 schema；缺任一条件就 skip/fail。fixture 使用动态 StrategyID，清理只删除明确 ID，临时 CHECK 约束也使用动态名字、只拒绝一个精确父子组合，结束后核对行和约束都不存在。覆盖回环、scoped AwardID、MaxUint64、重复/并发 Create、子失败回滚、预取消、在途锁等待取消、四类坏快照、RR 快照与 EXPLAIN。
- **追问：** 为什么故障注入要由 migrator 身份安装 CHECK？
  - **追问回答：** app 按最小权限没有 ALTER，这正是安全边界。测试需要在父 INSERT 成功后让指定子 INSERT 失败，临时精确 CHECK 能稳定制造这个点，又不改变其他记录；安装与删除必须由隔离环境的迁移/验证身份完成。
- **项目证据：** [双 opt-in Make 入口](../../../Makefile)、[集成授权门和精确清理](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[身份与权限集成测试](../../../migrations/lottery_schema_integration_test.go)。
- **选型边界：** 本地隔离 MySQL 证明功能语义，不证明生产数据量、复制、故障转移、磁盘满或网络分区；这些要在专门 staging/chaos 环境测试，不能在共享库随意注入。
- **来源：** 项目场景题；官方事务/连接行为见[Go 执行事务](https://go.dev/doc/database/execute-transactions)，死锁与短事务建议见[MySQL 8.4 deadlock handling](https://dev.mysql.com/doc/refman/8.4/en/innodb-deadlocks-handling.html)。

## 27. 为什么第 19 节不把 Strategy 放进 Redis 缓存？

- **直接回答：** 当前目标是先建立权威 MySQL Repository 和正确性基线，且没有真实吞吐、延迟或热点证据。此时加缓存会立即引入 key/version、失效顺序、陈旧策略、穿透/击穿、坏快照传播和发布一致性问题；更危险的是抽奖决策可能长期使用旧权重。先让直接数据库读取可验证，后续用观测数据证明瓶颈，再为不可变配置版本设计 cache-aside 或发布后预热。
- **追问：** Strategy 读多写少，不是天然适合缓存吗？
  - **追问回答：** 很可能适合，但“适合”不等于可以无版本直接缓存。至少要定义聚合版本、TTL/主动失效、更新后的可见性、故障降级和坏对象不入缓存；本节连 Update/发布用例都没有，无法正确回答失效。
- **项目证据：** [当前 Repository 只依赖 MySQL](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[端口没有 cache/update 语义](../../../internal/lottery/application/repository.go)、[Redis 容器存在但 API 不依赖](../../../deploy/compose/compose.yaml)、[配置文档明确 API 当前不读 Redis](../../configuration.md)。
- **选型边界：** 若压测证明 MySQL 是读热点，Strategy 又按不可变版本发布，Redis/进程内缓存都可评估；强一致实时更新场景则可能宁可直接读主库或使用版本校验。
- **来源：** 面经启发来自[字节实习作者记录的 Redis/MySQL 一致性追问](https://www.nowcoder.com/discuss/353159669701091328)和[阿里系作者的 MySQL/Redis 一致性项目深挖](https://www.nowcoder.com/discuss/523595426050666496)；当前不缓存是项目选择，不是声称 Redis 不合适。

## 28. 这一节最终能写进简历什么，哪些能力仍然不存在？

- **直接回答：** 可以准确写“基于 Go database/sql/sqlx 与 MySQL 8.4 实现 Strategy 聚合 Repository；用单事务保证父子原子写，以 REPEATABLE READ 只读快照完成两查询一致恢复；设计稳定错误分类、Context 取消、精确 SELECT/INSERT 权限，并用 sqlmock 与真实 MySQL 验证并发、回滚、坏数据和执行计划”。不能写“上线抽奖 API、高并发抽奖算法、库存防超卖、Redis 缓存、幂等创建接口、配置更新发布或权益发放”，因为这些都没有进入当前 composition。
- **追问：** 面试官问 QPS、P99、命中率或生产事故怎么办？
  - **追问回答：** 直接说明本节只有本地/隔离 MySQL 功能证据，没有生产流量和 SLO；可以讲将如何压测、观测和演练，但必须把计划与实绩分开。诚信边界本身比虚构数字更能体现工程判断。
- **项目证据：** [Repository 全部实现](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[真实 MySQL 场景](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)、[当前 Lottery application 仅有两个端口](../../../internal/lottery/application/repository.go)、[当前 HTTP 路由未装配 Lottery](../../../internal/infrastructure/httpapi/router.go)。
- **选型边界：** 后续章节真正实现算法、HTTP、缓存或更新后，简历可以增量更新；不得把 roadmap 倒灌进本节提交。
- **来源：** 面经启发来自[字节社招作者关于项目要讲细节和方案取舍的复盘](https://www.nowcoder.com/discuss/353157357217718272)与[美团作者记录的项目深挖](https://www.nowcoder.com/discuss/471056135512936448)；项目事实见上述代码与[第 19 节 QA](../../qa/lessons/lesson-19.md)。

## 不能夸大的事实

- 第 19 节只有 Repository adapter，没有 Lottery HTTP/GRPC endpoint，也没有把它装配进 server。
- Create 接收显式 StrategyID；没有实现 ID 生成、请求 DTO、认证、授权或审计。
- Create 是 create-only 且非幂等；duplicate error 不等于请求幂等已经完成。
- 单库事务保证当前两表原子写，不等于跨库、跨服务或分布式事务。
- 稳定 AwardID 顺序只是降低非确定性，不能承诺永不死锁。
- ErrRepositoryRetryable 只是分类，不代表 adapter 已自动重试或所有重试都安全。
- ErrCommitOutcomeUnknown 只把风险说准确；项目尚未实现查证、幂等键或 exactly-once。
- Context 取消能中断等待，不保证撤销已经提交的副作用。
- REPEATABLE READ 证明当前两次普通 SELECT 的一致快照，不等于 SERIALIZABLE。
- ReadOnly 是事务访问模式，不是账号权限，也不是跨 driver 的普遍同义保证。
- 两次查询的快照测试复用生产查询 helper；它没有在 public FindByID 内部注入任意时点 hook。
- sqlmock 证明调用协议与可控错误分支，不证明 InnoDB、MySQL 错误号、权限或 TxOptions 的真实执行。
- 真实 MySQL 小数据 EXPLAIN 不等于生产 P95/P99、容量、复制或锁竞争结论。
- 安全 Error 字符串不代表展开 cause 的日志天然脱敏。
- 精确 SELECT/INSERT grant 是当前 Compose 基线；启用其他角色、代理或云 IAM 后必须重审有效权限。
- 没有 Update、Save、Upsert、Delete、List、分页、乐观锁或配置发布版本。
- 没有加权随机算法、公平性统计、库存、Benefit 发放、MQ、Redis Lottery 缓存或前端抽奖页。
- 牛客面经是作者自述和题型启发，不是公司官方题库，也不是技术事实依据。

## 复习清单

- [ ] 60 秒内讲清窄端口、父子事务、RR 快照、恢复与错误边界。
- [ ] 能画出 application port → mysqlrepo adapter → sqlx.DB → 两张 InnoDB 表的依赖方向。
- [ ] 能解释 Repository 与表级 DAO、Create 与 Save/Upsert 的区别。
- [ ] 能口述 Create 的 Begin、根 INSERT、单 Prepare、稳定 Award 顺序、Commit/Rollback。
- [ ] 能解释为什么不做 SELECT EXISTS，以及并发 Create 为什么只有一个成功。
- [ ] 能画出根查询、并发 writer、Award 查询在 RR 与 RC 下的可见性差异。
- [ ] 能说明 read-only、非锁定 SELECT、FOR UPDATE、SERIALIZABLE 的边界。
- [ ] 能定位 ORDER BY 与 EXPLAIN 的 PRIMARY/key_len/no filesort 证据。
- [ ] 能说明 New 会规范化新输入，而 Restore 不修复持久化事实。
- [ ] 能列出四类坏快照，并说明为什么不返回部分聚合。
- [ ] 能把 not found、already exists、stored invalid、retryable、unknown outcome、failure 分开。
- [ ] 能解释 1062 为什么只在根 INSERT 映射重复，1205/1213 为什么不自动重试。
- [ ] 能用“请求可能已提交、确认丢失”解释 commit outcome unknown。
- [ ] 能说明 Context 取消的收益与不能撤销已提交写的限制。
- [ ] 能解释 pool 归 composition root、Tx 绑定单连接、Repository 不 Close。
- [ ] 能列出应用账号正向 SELECT/INSERT 与负向 UPDATE/DELETE/schema_migrations 权限。
- [ ] 能区分领域单元测试、错误分类测试、sqlmock、真实 MySQL、Compose smoke 各自证据。
- [ ] 能准确回答为何现在不加 Redis，以及版本/失效协议出现后何时重评。
- [ ] 面试前对照[第 19 节 QA](../../qa/lessons/lesson-19.md)，只陈述实际通过的环境和命令。
- [ ] 最后复述“没有 HTTP、算法、更新、缓存、库存或发奖”，避免把未来章节说成当前实绩。
