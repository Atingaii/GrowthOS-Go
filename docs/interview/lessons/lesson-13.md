# 第 13 节面试问答：MySQL、sqlx、Migration 与健康检查

本章只描述第 13 节已经建立的基础设施边界。本节尚未加入业务表和真实业务 Migration；对应验证状态以 [第 13 节 QA](../../qa/lessons/lesson-13.md) 为准。

## 60 秒项目自述

这一节我为 GrowthOS 建立了 MySQL 8.4 的生产化接入边界。API 使用 `database/sql` 管理的有界连接池，再由 sqlx 提供结构体扫描等轻量能力；启动监听前执行有截止时间的 Ping。运行期保留原有 `/health` 作为进程存活检查，新增 `/ready` 用有界 MySQL Ping 决定是否接收数据库流量。数据库账号按职责拆成受限 API 身份和具备 DDL 权限的 Migration 身份，只有后者的专用单连接允许 `multiStatements`。Migration 使用嵌入二进制的 forward-only SQL、MySQL 会话锁和 dirty/version mismatch 的失败关闭策略，并分别约束单条 SQL 的 `statement 30s < read 35s` 与取锁阶段的 `read 35s < lock-acquire 40s`。本节交付的是连接、迁移和运维骨架，还没有声称完成业务表设计或生产容量调优。

## 来源说明

- `面经启发` 只说明候选人复盘中出现过相近题型，不把帖子改写成经独立验证的公司原题。
- `社区讨论` 只用于补充追问角度，不用于单独证明技术事实。
- `官方事实` 用 Go、MySQL、Redis、PostgreSQL 或库维护者资料校准答案。
- `项目事实` 由当前仓库实现、测试、ADR、Runbook 和 QA 支持。
- dirty、version mismatch、readiness 超时层级等题没有找到可靠的完全同题面经，因此明确按项目场景题组织，不虚构出处。

## 1. 为什么当前系统选择 MySQL 作为事实库？

- **直接回答：** GrowthOS 当前需要关系约束、事务和可审计的版本化 schema，开发环境也已经具备 MySQL 8.4，所以先选一个关系型事实库把边界做深。这个选择不是“MySQL 普遍比 PostgreSQL 或 Redis 快”，而是当前需求、团队认知和迁移成本下的阶段决策。
- **追问：** 如果已有 MongoDB 或 Redis 数据，怎样证明迁移到 MySQL 后一条不丢？
  - **追问回答：** 先定义源记录数、主键集合、关键字段校验和与业务不变量；双写或停写窗口内迁移，按分片对账，再灰度切读。迁移成功不能只靠脚本零报错，还要保留可重复执行、差异报告和回退计划。
- **项目证据：** [MySQL 与 Migration ADR](../../decisions/ADR-0010-mysql-migration-boundaries.md)、[第 13 节课程正文](../../course/part-02/lesson-13-mysql-migrations.md)。
- **选型边界：** 出现明确的 PostgreSQL 专属能力、Redis 主存储需求或跨区域一致性模型时，应重新做 ADR，而不是让本节选择永久化。
- **来源：** `面经启发` [牛客用户自述的腾讯音乐 Go 社招复盘，包含数据库迁移原因与正确性追问](https://www.nowcoder.com/discuss/647227163053240320)；`官方事实` [MySQL 事务控制](https://dev.mysql.com/doc/refman/8.4/en/commit.html)。

## 2. 为什么选 sqlx，而不是 GORM？

- **直接回答：** sqlx 是 `database/sql` 的薄扩展，保留底层接口和显式 SQL，主要增加结构体扫描、命名参数、`Get/Select`。GORM 是包含关联、Hook、预加载和 AutoMigrate 的完整 ORM。当前项目更看重 SQL 可见性、Migration 可审查性和学习边界，所以选 sqlx；这不是说 GORM 不好或不能写原生 SQL。
- **追问：** GORM 也支持 Raw SQL，为什么仍不选？
  - **追问回答：** 选择看默认抽象和团队约束，不只看“能不能”。如果主体仍依赖模型约定、Hook 和 AutoMigrate，schema 与查询行为会分散在 ORM 配置中；本项目希望版本 SQL 是唯一显式入口。若以后大量简单关联 CRUD 的交付效率成为主要矛盾，可以重新评估。
- **项目证据：** [MySQL 打开与 sqlx 包装](../../../internal/infrastructure/mysql/open.go)、[嵌入 Migration](../../../migrations/embed.go)、[Migration 规则](../../../migrations/README.md)。
- **选型边界：** sqlx 不自动解决 N+1、动态查询组合和重复 DAO；业务复杂后仍需 repository 规范、查询构建器或重新评估 ORM。
- **来源：** `社区讨论` [牛客 GORM 与 SQLX 对比题](https://www.nowcoder.com/discuss/757654087407058944)、[掘金 sqlx/GORM 讨论](https://juejin.cn/post/7316451458840231971)；`官方事实` [sqlx README](https://github.com/jmoiron/sqlx)、[GORM 功能总览](https://gorm.io/docs/)、[GORM AutoMigrate](https://gorm.io/docs/migration.html)。

## 3. `sql.DB` 是一条连接还是连接池？sqlx 是否另建一套池？

- **直接回答：** `sql.DB` 是可被多个 goroutine 并发使用的长生命周期句柄，内部管理连接池；查询借出连接，完成后归还。sqlx 只是包装和扩展标准库，没有再维护一套池。只有需要同一数据库会话时才取 `DB.Conn(ctx)`，并在完成后关闭连接句柄归还资源。
- **追问：** `sql.DB.Close` 应该在每次请求结束时调用吗？
  - **追问回答：** 不应该。`sql.DB` 设计为跨请求共享，频繁关闭会失去池化价值。它应在进程优雅停止或构造失败清理时关闭；每次请求应关闭的是 `Rows`、事务或专用 `Conn`。
- **项目证据：** [API pool 构造](../../../internal/infrastructure/mysql/open.go)、[Migration 专用连接](../../../internal/infrastructure/migration/runner.go)。
- **选型边界：** 带会话状态、命名锁或事务的操作不能假设池会自动复用同一物理连接，必须显式使用事务或专用 `Conn`。
- **来源：** `面经启发` [牛客用户自述“蚂蚁面试”中的连接池原理题](https://www.nowcoder.com/discuss/736581477898387456)、[腾讯 WXG 复盘中的通用连接池设计题](https://www.nowcoder.com/discuss/353158302706114560)；`官方事实` [Go 连接管理](https://go.dev/doc/database/manage-connections)、[`database/sql`](https://pkg.go.dev/database/sql)。

## 4. `MaxOpenConns` 应该怎样定，而不是拍脑袋写 10 或 100？

- **直接回答：** 先用数据库总连接预算反推每个应用副本上限，为 DBA、Migration、监控和故障切换预留空间；再用峰值请求率乘数据库操作耗时估算并发需求。上线后联合观察 `DBStats.InUse`、`WaitCount`、`WaitDuration` 与 MySQL CPU、锁等待、慢查询。增大池只会减少应用侧排队，也可能把过载直接推给数据库。
- **追问：** `WaitCount` 很高时是否应该立刻扩池？
  - **追问回答：** 不一定。如果 MySQL 仍有容量且等待主要来自池上限，可以小步扩池；如果数据库 CPU、I/O 或锁已经饱和，扩池会增加竞争，应先优化 SQL、事务长度、限流或实例容量。
- **项目证据：** [有界默认值与校验](../../../internal/platform/appconfig/config.go)、[真实 DBStats 断言](../../../internal/infrastructure/mysql/mysql_integration_test.go)。
- **选型边界：** 当前 `10` 是小而有界的默认值，不是生产压测结论；副本数、查询延迟或数据库规格变化后必须重算。
- **来源：** `面经启发` [掘金用户自述“5 秒内建立多少个 MySQL 连接”场景](https://juejin.cn/post/6934953431954096141)、[牛客连接池原理题复盘](https://www.nowcoder.com/discuss/736581477898387456)；`官方事实` [Go MaxOpen 与池等待](https://go.dev/doc/database/manage-connections)、[`DBStats`](https://pkg.go.dev/database/sql)。

## 5. `MaxIdleConns`、`ConnMaxIdleTime`、`ConnMaxLifetime` 分别解决什么？

- **直接回答：** MaxIdle 保留可复用连接，减少突发期重连；MaxIdleTime 在流量回落后释放闲置连接；MaxLifetime 定期轮换老连接，避免长期绑定某个后端。`MaxIdleConns` 不应超过 `MaxOpenConns`。本项目的 10/10、1 分钟、3 分钟只是当前基线。
- **追问：** MaxIdle 等于 MaxOpen 会不会永远保留全部连接？
  - **追问回答：** 不会，因为还有 MaxIdleTime 和 MaxLifetime；但突发后短时间会保留较多空闲连接。是否合理要结合重连成本、服务副本数和数据库连接预算判断。
- **项目证据：** [连接池配置](../../../internal/platform/appconfig/config.go)、[池参数安装](../../../internal/infrastructure/mysql/open.go)、[连接池集成测试](../../../internal/infrastructure/mysql/mysql_integration_test.go)。
- **选型边界：** 经过代理、负载均衡或云数据库时，Lifetime 还应低于外部连接回收边界并结合实际抖动策略；不能只照抄本地默认值。
- **来源：** `社区讨论` [掘金 MySQL 连接池与超时实验](https://juejin.cn/post/6844904087427776519)；`官方事实` [Go 连接池参数](https://go.dev/doc/database/manage-connections)、[`database/sql.DBStats`](https://pkg.go.dev/database/sql)。

## 6. 为什么 `OpenDB` 成功后还必须 `PingContext`？

- **直接回答：** 打开句柄通常只完成配置和 connector 初始化，不保证已经连到数据库。`PingContext` 会验证通信，并在需要时建立连接。项目在 HTTP 监听前做有界 Ping，失败就关闭部分创建的池并退出。但 Ping 只证明连接、认证和基本通信，不证明业务表存在或账号拥有全部 DML 权限。
- **追问：** 为什么不能用无超时的 `Ping`？
  - **追问回答：** 无界 Ping 可能让启动或探针无限挂住。应从进程或请求 context 派生更短 deadline，使取消能传播，并确保外层仍有时间记录错误或写出稳定响应。
- **项目证据：** [有界首次 Ping](../../../internal/infrastructure/mysql/open.go)、[API 数据库装配](../../../cmd/growth-api/database.go)、[监听前初始化](../../../cmd/growth-api/main.go)。
- **选型边界：** Ping 不能替代 schema version 检查、权限负向测试或业务查询；更严格的启动门禁应作为独立检查设计。
- **来源：** `官方事实` [`PingContext` 定义和有界示例](https://pkg.go.dev/database/sql)。

## 7. `/health` 和 `/ready` 为什么必须分开？

- **直接回答：** liveness 表示进程是否仍能工作，失败可能触发重启；readiness 表示实例是否应接收流量，失败应摘流但不必杀进程。数据库短暂抖动时，让 `/health` 保持 200、`/ready` 返回 503，可以避免所有实例同时重启形成级联故障。
- **追问：** 数据库持续不可用时 Pod 是否应该重启？
  - **追问回答：** 通常不应仅因数据库不可用而重启应用，因为重启不能修复外部依赖，还会放大负载。应由 readiness 摘流并持续探测；只有进程自身死锁或无法推进时才由 liveness 重启。
- **项目证据：** [健康检查](../../../internal/infrastructure/httpapi/health.go)、[就绪检查](../../../internal/infrastructure/httpapi/readiness.go)、[路由装配](../../../internal/infrastructure/httpapi/router.go)、[API 契约说明](../../api/lessons/lesson-13.md)。
- **选型边界：** 只有部署平台真的把两个路径配置为相应探针时，应用内语义才会转化为摘流和重启行为。
- **来源：** `官方事实` [Kubernetes liveness、readiness、startup probe](https://kubernetes.io/docs/concepts/workloads/pods/probes/)、[Go 服务中有界 PingContext 示例](https://pkg.go.dev/database/sql)。

## 8. 为什么 API 账号和 Migration 账号必须分开？

- **直接回答：** API 只获得业务需要的 SELECT/INSERT/UPDATE/DELETE；Migration 才获得 CREATE/ALTER/DROP/INDEX 及维护版本表所需权限。这样即使 API 出现注入或凭据泄漏，也不能直接改 schema，并且两套凭据可以独立轮换、审计和限制来源主机。
- **追问：** API 启动 Ping 成功是否说明最小权限配置正确？
  - **追问回答：** 不说明。Ping 只验证连接和基本通信；必须用真实 DML 正向测试和 DDL 负向测试证明 API 权限，用 Migration 集成测试证明迁移账号权限，部署后再用 `SHOW GRANTS` 审计。
- **项目证据：** [两套配置模型](../../../internal/platform/appconfig/config.go)、[API 连接装配](../../../cmd/growth-api/database.go)、[Migration 连接装配](../../../cmd/growth-migrate/production.go)、[Migration Runbook](../../runbooks/mysql-migrations.md)。
- **选型边界：** 权限集合应随实际业务表和操作逐项增加；本节尚无业务表，不能把示例账号说成最终生产 GRANT 清单。
- **来源：** `官方事实` [MySQL 两阶段访问控制](https://dev.mysql.com/doc/refman/8.4/en/access-control.html)、[GRANT 的数据库、表和列级范围](https://dev.mysql.com/doc/refman/8.4/en/grant.html)。

## 9. 为什么 API 关闭 `multiStatements`，Migration 却开启？

- **直接回答：** MySQL 驱动默认关闭多语句。API 的一次调用应只执行一条明确语句，关闭它能缩小意外分号批处理的影响范围，但不能替代占位符。Migration 文件可能合法包含多条 DDL，因此只在专用账号和专用连接边界开启。
- **追问：** 关闭 `multiStatements` 是否就彻底没有 SQL 注入？
  - **追问回答：** 不是。单条语句内仍可因字符串拼接产生注入；核心防线是固定 SQL 结构和参数绑定。Migration SQL 必须来自受审查、随构件发布的文件，不能拼接用户输入。
- **项目证据：** [驱动配置边界](../../../internal/infrastructure/mysql/config.go)、[API 与 Migration 打开路径](../../../internal/infrastructure/mysql/open.go)、[配置测试](../../../internal/infrastructure/mysql/config_test.go)。
- **选型边界：** 如果未来 Migration 工具改为逐句解析执行，可以重新评估是否仍需开启；不能为了方便批量 API 请求而全局打开。
- **来源：** `官方事实` [go-sql-driver multiStatements 说明](https://github.com/go-sql-driver/mysql)、[MySQL 参数化预处理](https://dev.mysql.com/doc/refman/8.4/en/sql-prepared-statements.html)。

## 10. Migration 为什么要取得专用 `sql.Conn`，而不是随便使用连接池？

- **直接回答：** golang-migrate 的 MySQL 锁使用 `GET_LOCK`，而 MySQL 命名锁属于数据库会话，事务提交不会释放，只有显式 RELEASE 或会话结束才释放。因此锁、版本检查、DDL 和解锁必须绑定同一连接。项目把 Migration 池限制为 1，再通过 `db.Conn(ctx)` 和 `WithConnection` 明确固定会话。
- **追问：** 进程崩溃后锁会不会永久遗留？
  - **追问回答：** 会话终止时 MySQL 会释放命名锁，不会永久遗留；但已执行的 DDL 可能保留，版本表也可能是 dirty，所以另一个实例取得锁后仍必须先检查版本和 dirty，不能盲目继续。
- **项目证据：** [Migration 单连接池](../../../internal/infrastructure/mysql/open.go)、[专用连接与 migrate engine](../../../internal/infrastructure/migration/runner.go)。
- **选型边界：** `GET_LOCK` 只协调同一 MySQL server 上使用相同锁名的参与者；多主或绕过该工具的 DDL 需要额外发布协调。
- **来源：** `官方事实` [Go 专用连接语义](https://go.dev/doc/database/manage-connections)、[MySQL GET_LOCK 生命周期](https://dev.mysql.com/doc/refman/8.4/en/locking-functions.html)、[golang-migrate 数据库锁说明](https://github.com/golang-migrate/migrate/blob/master/FAQ.md)。

## 11. 为什么项目采用 forward-only，而且不向产品命令暴露 down、drop、force？

- **直接回答：** 项目只发布 `up/status`，Migration SQL 随二进制嵌入；失败修复优先提交新的前向迁移。库本身确实提供 Down、Drop、Force，但这些操作可能丢数据或只修改版本标记，因此不能成为普通发布动作。相应代价是 schema 变更要采用 expand/contract，并让前后应用版本保持一段兼容。
- **追问：** 应用版本需要紧急回滚时怎么办？
  - **追问回答：** 回滚应用不等于回滚 schema。先用向后兼容的扩展式变更发布新 schema，再切应用；旧字段和旧读写路径延迟到确认无旧版本流量后才清理。事故中的 Force 只能由维护者核查真实 schema 后按 Runbook 执行。
- **项目证据：** [Migration 命令](../../../cmd/growth-migrate/main.go)、[嵌入式 forward-only 资源](../../../migrations/embed.go)、[Migration 规则](../../../migrations/README.md)、[事故处理](../../runbooks/mysql-migrations.md)。
- **选型边界：** forward-only 是本项目的发布策略，不是 golang-migrate 的强制要求；一次性测试库和经批准的灾难恢复流程可以有不同工具边界。
- **来源：** `官方事实` [golang-migrate Down、Drop、Force 语义](https://pkg.go.dev/github.com/golang-migrate/migrate/v4)、[MySQL DDL 隐式提交](https://dev.mysql.com/doc/refman/8.4/en/implicit-commit.html)。

## 12. MySQL 8.4 支持 atomic DDL，为什么一个 Migration 文件仍可能部分成功？

- **直接回答：** atomic DDL 只保证单条受支持 DDL 的数据字典、存储引擎和 binlog 变化一起成功或回滚；官方明确说明它不是 transactional DDL。DDL 会隐式结束当前事务，所以文件里第一条 CREATE 成功、第二条 ALTER 失败时，第一条通常不会随整个文件回滚。
- **追问：** `BEGIN; CREATE TABLE a(...); ALTER TABLE b ...; ROLLBACK;` 能否撤销 `a`？
  - **追问回答：** 不能依赖 ROLLBACK 撤销。`CREATE TABLE` 等 DDL 会触发隐式提交；atomic DDL 主要保证这“一条 DDL”在崩溃时不留下数据字典、引擎和 binlog 相互矛盾的中间状态。
- **项目证据：** [课程中的 DDL 边界](../../course/part-02/lesson-13-mysql-migrations.md)、[Migration Runbook](../../runbooks/mysql-migrations.md)、[dirty 处理](../../../internal/infrastructure/migration/runner.go)。
- **选型边界：** 迁移文件应尽量小、可观察、可重试；不能用一个大文件加 `BEGIN` 假装获得 PostgreSQL 式 transactional DDL。
- **来源：** `官方事实` [MySQL 8.4 Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)、[隐式提交语句](https://dev.mysql.com/doc/refman/8.4/en/implicit-commit.html)。

## 13. dirty 状态是什么意思，为什么不能自动 Force？

- **直接回答：** golang-migrate 在执行每个版本前先写 dirty，成功后才清除；执行失败时 dirty 保留，阻止继续叠加迁移。恢复时必须检查该版本哪些 DDL 或 DML 已经落地，修复 schema 或数据，再把版本设置到与真实状态一致。Force 只改版本标记，不会回滚或补齐 SQL。
- **追问：** 直接删除 `schema_migrations` 记录有什么问题？
  - **追问回答：** 它会抹掉失败证据，却不改变已经落地的对象；下一次可能重复 CREATE、跳过必要 ALTER，或把更严重的漂移伪装成未初始化。正确动作是先调查、备份和对账，再走受控修复。
- **项目证据：** [Runner dirty 状态机](../../../internal/infrastructure/migration/runner.go)、[稳定错误分类](../../../internal/infrastructure/migration/error.go)、[状态机测试](../../../internal/infrastructure/migration/runner_test.go)、[恢复 Runbook](../../runbooks/mysql-migrations.md)。
- **选型边界：** 产品命令不提供 Force；确需使用时属于维护操作，必须保留审批、真实状态证据和后续修复 Migration。
- **来源：** `官方事实` [golang-migrate dirty FAQ](https://github.com/golang-migrate/migrate/blob/master/FAQ.md)、[官方 dirty 恢复说明](https://github.com/golang-migrate/migrate/blob/master/GETTING_STARTED.md)。

## 14. dirty 和 version mismatch 有什么区别？

- **直接回答：** dirty 表示某个已知版本在执行中失败，需要确认部分副作用；version mismatch 是项目额外的失败关闭策略，表示数据库报告的干净版本不在当前嵌入历史中或数据库领先当前二进制。常见原因是部署旧构件、使用错误分支或人工修改版本。正确动作是找到匹配历史的构件并核查，而不是自动降级或 Force。
- **追问：** Migration 版本号不连续是否一定 mismatch？
  - **追问回答：** 不一定。项目判断的是当前数据库版本是否属于嵌入版本集合，不要求数字连续；但数据库处在一个源码不存在的版本，或领先于当前最新版本，就必须拒绝继续。
- **项目证据：** [版本集合与状态判断](../../../internal/infrastructure/migration/runner.go)、[version mismatch 错误](../../../internal/infrastructure/migration/error.go)、[未知和超前版本测试](../../../internal/infrastructure/migration/runner_test.go)。
- **选型边界：** mismatch 是 GrowthOS 的稳定错误语义，不应描述成 golang-migrate 内置的同名状态；底层库提供的是 version、dirty 和找不到 source version 的错误。
- **来源：** `官方事实` [golang-migrate Version API](https://pkg.go.dev/github.com/golang-migrate/migrate/v4)、[migrate 对不存在 source version 的处理](https://github.com/golang-migrate/migrate/blob/master/migrate.go)。

## 15. statement、read、lock-acquire timeout 为什么不能都设成 30 秒？

- **直接回答：** statement timeout 限制单条迁移语句；`readTimeout` 是 MySQL 驱动单连接 I/O 读取截止；`golang-migrate` 的 lock timeout 只限制获取数据库锁，不包住后续 DDL 或解锁。当前默认约束分成两条：`statement 30s + 5s <= read 35s`，避免驱动先抢走单条 SQL 的截止；取锁阶段再要求 `read 35s + 5s <= lock-acquire 40s`，避免外层先返回而取锁 I/O 仍在后台等待。
- **追问：** 若 `readTimeout=5s`、`StatementTimeout=30s` 会怎样？
  - **追问回答：** 大约 5 秒时驱动读超时先失败，30 秒语句预算永远无法兑现，错误阶段也会偏离预期。这是配置层 P2 边界；最小修复是使用独立 Migration read timeout，并在 appconfig、MySQL adapter 和 Runner 入口强制 `read >= statement + margin`。
- **项目证据：** [跨层 timeout 校验](../../../internal/platform/appconfig/config.go)、[MySQL adapter 校验](../../../internal/infrastructure/mysql/config.go)、[Runner 校验](../../../internal/infrastructure/migration/runner.go)、[readiness 有界 context](../../../internal/infrastructure/httpapi/readiness.go)。
- **选型边界：** 5 秒只是当前传播余量，不是行业常数；锁取得后 `LockTimeout` 已停止计时，因此不能把 40 秒说成整次迁移上限。调整单层时必须分别复核 SQL 执行与取锁两个阶段，并让 readiness Ping 加响应预算后不超过 HTTP write timeout。
- **来源：** `社区讨论` [掘金 readTimeout 实验](https://juejin.cn/post/6844904087427776519)；`官方事实` [go-sql-driver 连接和 I/O timeout](https://github.com/go-sql-driver/mysql)、[golang-migrate MySQL statement timeout](https://github.com/golang-migrate/migrate/blob/master/database/mysql/README.md)。

## 16. Redis 已经安装，为什么本节不把它作为主存储或 readiness 依赖？

- **直接回答：** Redis 官方定位覆盖缓存、数据库和消息代理，也支持 RDB/AOF，不能简单说它“不持久”。项目当前更适合把它用于可重建或时效性强的数据，例如缓存、限流和短期幂等状态；关系约束和业务事实仍在 MySQL。只有请求没有 Redis 就无法正确服务时，Redis 才应成为 readiness 的硬依赖。
- **追问：** Redis 有持久化，为什么仍不默认替换 MySQL？
  - **追问回答：** 是否作为事实库取决于数据模型、一致性、查询、恢复目标和团队运维能力，不只看有没有落盘。当前领域模型以关系事务为主，引入第二事实源会增加双写与恢复复杂度，没有明确收益。
- **项目证据：** [当前 MySQL 决策边界](../../decisions/ADR-0010-mysql-migration-boundaries.md)、[当前 readiness 只检查 MySQL](../../../internal/infrastructure/httpapi/readiness.go)。
- **选型边界：** 以后某项能力把 Redis 设为强依赖时，需要单独定义降级、持久化、缓存一致性和探针策略，不能沿用“缓存丢了可重建”的假设。
- **来源：** `面经启发` [Go 社招复盘中的 Redis 场景、持久化和连接池题](https://www.nowcoder.com/discuss/353158063463014400)、[跟谁学 Go 面经中的 MySQL/Redis 追问](https://www.nowcoder.com/discuss/353156172658188288)；`官方事实` [Redis 官方定位](https://redis.io/docs/latest/get-started/)、[Redis 内存与持久化权衡](https://redis.io/docs/latest/develop/get-started/faq/)。

## 17. 什么条件下应重新评估 PostgreSQL，而不是继续 MySQL？

- **直接回答：** PostgreSQL 是同级替代方案，不是自动升级。一个明确差异是 PostgreSQL 支持大多数表和 catalog DDL 在事务中回滚，而 MySQL atomic DDL 不提供多条 DDL 的事务原子性。如果业务确实需要 PostgreSQL 专属能力或这种迁移语义，且收益覆盖方言、运维和迁移重写成本，再正式立 ADR。
- **追问：** 已经使用 `database/sql` 和 sqlx，切 PostgreSQL 是否只换驱动？
  - **追问回答：** 不是。连接接口较容易复用，但 placeholder、SQL 方言、DDL、Migration 锁、权限模型、TLS 参数和集成测试都要重做；显式 SQL 的可见性让工作可枚举，不代表 SQL 可移植。
- **项目证据：** [MySQL 专属连接实现](../../../internal/infrastructure/mysql/config.go)、[MySQL Migration adapter](../../../internal/infrastructure/migration/runner.go)、[Migration SQL 规则](../../../migrations/sql/README.md)。
- **选型边界：** “Docker 里已经装了 PostgreSQL”不是生产选型依据；必须先有可量化需求、迁移成本和退出方案。
- **来源：** `面经启发` [Go 社招复盘中同时追问 MySQL/PG](https://www.nowcoder.com/discuss/353158063463014400)；`官方事实` [PostgreSQL 官方 Wiki 的 transactional DDL 示例](https://wiki.postgresql.org/wiki/Transactional_DDL_in_PostgreSQL%3A_A_Competitive_Analysis)、[PostgreSQL BEGIN](https://www.postgresql.org/docs/current/sql-begin.html)、[MySQL Atomic DDL 的非事务边界](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)。

## 18. 如何防止 MySQL 密码、DSN 和弱 TLS 配置泄漏到日志？

- **直接回答：** 密码放进配置结构体后不会自动脱敏，`fmt.Printf("%+v", cfg)` 或 `slog.Any("config", cfg)` 都可能直接打印。项目只记录白名单字段和稳定阶段码，不记录 DSN、原始驱动错误或完整配置；配置的字符串表示固定为 redacted。staging/production 只允许验证服务端身份的 TLS，不接受 `skip-verify` 或可回退明文的 `preferred`。
- **追问：** TLS 已加密但不校验主机名，是否足够？
  - **追问回答：** 不够。加密只能防被动窃听，不验证证书链和目标身份仍可能遭遇中间人。自定义 CA 应扩展可信根，同时保留主机名校验；开发环境的 disabled 不能复制到 staging/production。
- **项目证据：** [配置脱敏与环境约束](../../../internal/platform/appconfig/config.go)、[TLS 与 DSN 构造](../../../internal/infrastructure/mysql/config.go)、[安全阶段错误](../../../internal/infrastructure/mysql/error.go)、[环境变量示例](../../../configs/growth-api.env.example)。
- **选型边界：** 稳定错误码有利于防泄漏，但会损失现场细节；生产诊断应通过受控指标、错误类别和数据库侧日志补充，而不是重新打印密码或 DSN。
- **来源：** `官方事实` [go-sql-driver TLS 模式及对 skip-verify/preferred 的警告](https://github.com/go-sql-driver/mysql)、[MySQL 访问控制](https://dev.mysql.com/doc/refman/8.4/en/access-control.html)。

## 不能夸大的事实

- 本节建立的是 MySQL 连接、Migration、权限、探针和运维骨架，`migrations/sql` 还没有真实业务 `.up.sql`，不能说“业务表已经完成”。
- 连接池默认值是有界起点，没有生产 QPS、容量或压测数据，不能说“10 个连接最优”。
- 选择 MySQL 是当前项目决策，不是对 MySQL、PostgreSQL、Redis 的通用性能排名。
- sqlx 保留显式 SQL，但不代表它在所有查询中都比 GORM 快，也不代表 GORM 不能写 Raw SQL。
- Ping 成功不证明业务 schema、全部权限或查询性能正常。
- `/health` 与 `/ready` 的应用语义只有在部署平台正确配置探针后才会产生重启和摘流效果。
- MySQL atomic DDL 不是多语句 transactional DDL；一个 Migration 文件仍可能部分落地。
- forward-only 是 GrowthOS 的产品发布策略；golang-migrate 官方同时支持 down、drop 和 force。
- dirty 不能自动修复，Force 也不是回滚。
- version mismatch 是项目增加的稳定失败语义，不是 golang-migrate 的同名内置状态。
- Redis 可以持久化，也可以在适当模型下作为数据库；本项目只是暂未把它设为事实库。
- PostgreSQL 是可行替代方案，但不是替换驱动即可无成本迁移。
- 集成测试可能依赖 Docker 和凭据；是否真实运行、是否跳过必须以 [QA 记录](../../qa/lessons/lesson-13.md) 为准。
- 面经链接是用户自述，只用于题型启发，不能宣称公司或原题已经独立核验。

## 复习清单

- [ ] 在 60 秒内讲清连接池、账号隔离、探针、Migration 和本节无业务表边界。
- [ ] 能画出 `HTTP -> sqlx.DB -> database/sql pool -> MySQL`，并解释 sqlx 不另建池。
- [ ] 能从数据库连接总预算和 `DBStats` 解释 MaxOpen，而不是背默认值。
- [ ] 能说明 `/health` 失败与 `/ready` 失败对编排平台的不同影响。
- [ ] 能列出 API 与 Migration 账号各自需要和不需要的权限。
- [ ] 能解释为什么 `multiStatements` 只在 Migration 专用连接开启。
- [ ] 能分别画出 SQL 阶段的 `statement 30s -> read 35s` 与取锁阶段的 `read 35s -> lock-acquire 40s`，并复现 5s read 抢先失败的问题。
- [ ] 能用“两条 DDL，第二条失败”说明 atomic DDL、隐式提交和 dirty。
- [ ] 能区分 dirty、version mismatch、cancelled 和 no change。
- [ ] 能说明 forward-only 的收益、代价和 expand/contract 回滚策略。
- [ ] 能指出 Redis、PostgreSQL 何时进入架构，而不是因为本机已安装就引入。
- [ ] 能在仓库中快速定位 `open.go`、`runner.go`、`readiness.go`、ADR、Runbook 和 QA。
- [ ] 回答前检查“不能夸大的事实”，不把设计、单元测试或待验证项说成生产实绩。
