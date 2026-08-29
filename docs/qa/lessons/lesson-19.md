# 第 19 节 QA：Lottery Strategy 仓储、事务快照与最小写权限

- **对应章节：** [实现仓储层](../../course/part-03/lesson-19-lottery-repository.md)
- **API 记录：** [第 19 节 API 边界](../../api/lessons/lesson-19.md)
- **设计推导：** [第 19 节第一性原理设计手记](../../design-thinking/lessons/lesson-19.md)
- **面试复盘：** [第 19 节面试问答](../../interview/lessons/lesson-19.md)
- **长期决策：** [ADR-0016](../../decisions/ADR-0016-lottery-repository-boundaries.md)
- **分支：** `codex/lesson-19-lottery-repository`
- **实现提交：** `50ac811`（`feat: add lottery strategy repository`）
- **证据加固提交：** `2c420c9`（`test: strengthen repository transaction evidence`）
- **文档内容提交：** `09556d8`（`docs: complete lesson nineteen learning evidence`）
- **日期：** 2026-08-29

> 本记录区分“测试源码覆盖了什么”“哪条命令真的在本机执行过”和“这些证据仍然不能证明什么”。第 19 节没有 Lottery HTTP API、抽奖算法、更新/删除、Redis 缓存或生产部署，不能把仓储可读写外推为在线抽奖闭环。

## 1. 验收范围

本节验收以下事实：

1. application 层只定义调用方所需的 `StrategyCreator.Create` 与 `StrategyReader.FindByID` 两个窄端口；
2. MySQL adapter 能把一个合法 Strategy 及其全部 Awards 原子写入两张表；
3. `FindByID` 在同一个只读 `REPEATABLE READ` 事务内按“根行 → Award 行”读取一个一致快照；
4. 从数据库恢复对象时不静默 trim/修复，任何违反领域不变量的快照都失败关闭；
5. 重复、暂态事务故障、找不到、坏数据、普通依赖故障和写提交结果未知具有不同且可用 `errors.Is` 判断的语义；
6. 应用账号精确拥有两张业务表的 `SELECT, INSERT`，没有 UPDATE、DELETE、DDL 或 `schema_migrations` 权限；
7. 单元测试、SQL 控制流测试、真实 MySQL 8.4.11 集成测试与默认 Compose smoke 形成分层证据；
8. 测试创建的临时容器与独立 Compose 资源被精确清理，默认开发数据卷保留。

本节不验收：

- HTTP handler、DTO、状态码或浏览器业务调用；
- 随机选择的正确性、公平性或安全随机源；
- Strategy 更新、删除、Upsert、版本发布或审计；
- Create 的幂等键、请求去重或“提交结果未知”后的业务查询协议；
- Redis 缓存、击穿/穿透/失效或缓存一致性；
- 生产 MySQL 拓扑、复制、备份恢复、TLS、跨地域时延或真实业务容量。

## 2. 验收环境

| 维度 | 实际环境 |
| --- | --- |
| 宿主机 | macOS，Apple Silicon |
| 日期/时区 | 2026-08-29，Asia/Shanghai |
| Go module | `github.com/Atingaii/GrowthOS-Go`，Go `1.26.6` |
| 数据库驱动 | `go-sql-driver/mysql v1.10.0` |
| SQL helper | `sqlx v1.4.0` |
| SQL 控制流替身 | `go-sqlmock v1.5.2`，只用于顺序与错误路径，不代替真实 MySQL |
| 独立数据库 | 官方 `mysql:8.4.11`，临时容器、临时数据目录、宿主回环随机端口 |
| 默认开发栈 | Docker Compose project `growthos`；MySQL/API/Redis/Web 健康，Migration 与授权作业退出 0 |
| 默认持久化资源 | `growthos_mysql_data`、`growthos_mysql_socket` 均保留 |

隔离集成测试要求两个显式授权开关：

```text
GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES=lesson-19-isolated-schema
GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES=lesson-19-isolated-repository
```

缺少任一个值时，`make test-integration-mysql` 必须拒绝执行，而不是在未知数据库中尝试清表、加约束或写测试数据。

## 3. 证据矩阵

| 风险命题 | 主要证据 | 已证伪的反例 | 仍不能证明 |
| --- | --- | --- | --- |
| 聚合可能只写入父行 | 真实 MySQL 子行故障注入与事务回滚 | 临时 CHECK 让目标 Award 插入失败后，父/子行计数均为 0 | 主机掉电、存储损坏时的恢复 |
| 两次读取可能来自不同版本 | 使用生产 `readSnapshotOptions` 与同一组 query helper 的 RR 确定性交错测试 | 读取根后并发更新根并增加 Award；旧事务仍见旧根/旧集合，提交后的公开读取见新根/新集合 | 尚未在公开 `FindByID` 内注入暂停点；多副本读、代理路由和复制延迟 |
| SQL mock 绿色但 SQL 在 MySQL 不成立 | 独立 MySQL 8.4.11 全集成 | uint64 上限、CHECK、权限、锁等待、执行计划都在真实引擎执行 | 所有 MySQL 小版本和参数组合 |
| 数据库行可绕过完整领域规则 | Migrator 注入坏行后 `FindByID` | 空 Award、NBSP 首空白、控制字符、总权重溢出均返回 stored-invalid | 离线修复流程是否完善 |
| 重复创建被误当成功 | 串行与并发 duplicate 测试 | 并发同 ID 恰好一个成功，其余 AlreadyExists | HTTP 层幂等协议 |
| 暂态错误被 adapter 盲重试 | 真实 1205、真实 context deadline 与 1213 分类单测 | 正常操作阶段的 1205/1213 只标为 retryable，adapter 内不重试；context 超时仍回滚整笔事务 | 真实 1213 与上层重试预算、退避是否正确 |
| COMMIT 错误被误判为回滚 | 公开 `Create` 的模拟 driver COMMIT error 控制流测试 | 模拟提交错误返回 commit-outcome-unknown 且保留 cause | 真实断网点是否已提交；这正是未知语义 |
| 应用权限随旧 volume 漂移 | grants reconciliation、SHOW GRANTS 精确比较、负向 SQL | UPDATE、DELETE、`schema_migrations` 读写均得到 1142 | 生产 mandatory role/云数据库权限体系 |
| 查询退化为扫描/排序 | 真实 `EXPLAIN` 断言 | 根表和子表查询都使用 PRIMARY，子查询无 filesort | 未来数据分布和新查询形状 |

## 4. 普通与竞态测试

最终章节收尾执行：

```bash
go test ./...
go test -race ./...
go vet ./...
```

实际结果：全仓所有 Go package 普通测试通过；全仓所有 Go package race 测试通过；`go vet ./...` 退出 0。没有把缺少集成环境而 skip 的普通测试当成真实 MySQL 证据，真实引擎结果单独记录在第 6 节。

覆盖重点包括：

- `RestoreAward` / `RestoreStrategy` 接受已经规范的原始 Unicode 值，但拒绝需要 trim 的 ASCII、NBSP `U+00A0`、全角空格 `U+3000` 和控制字符；
- Strategy 仍执行正 ID、至少一个 Award、同策略 AwardID 唯一、正权重、Outcome 封闭、总权重不溢出与 AwardID 规范排序；
- Repository 的 nil pool、nil context、零 ID 与零值 Strategy 均失败关闭；
- `RepositoryError` 的公开字符串只呈现稳定语义类，可信代码仍能通过 `errors.Is` / `errors.As` 查看 cause；
- `sqlmock` 锁定公开方法的事务顺序和 COMMIT 错误路径；
- `-race` 检查并发测试自身与 Repository 调用没有被 Go race detector 捕获的数据竞争。

一次加固过程中，旧单测仍把“context 已取消 + 任意 driver commit error”断言成 `context.Canceled`。这会掩盖独立驱动故障。最终规则收窄为：只有提交错误本身匹配 context 错误或 `sql.ErrTxDone` 时才归类为取消；其他写提交错误为 `ErrCommitOutcomeUnknown`，其他读提交错误为普通仓储故障。修正测试后相关普通与 race 测试通过。

## 5. SQL 控制流测试的边界

[`repository_transaction_test.go`](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go) 使用 `sqlmock` 验证：

1. `FindByID` 公开入口只开始一次事务；
2. 先查询根行，再查询 `ORDER BY award_id` 的 Award 行；
3. 两次查询成功后才 COMMIT；
4. `Create` 接收逆序的两个 Award，先插父，只 prepare 一次子语句并按 AwardID 规范顺序执行两次；
5. `WillBeClosed` 验证 statement 最终关闭，生产源码显式在 COMMIT 前关闭；
6. driver 在 COMMIT 返回普通错误时，公开结果为 `ErrCommitOutcomeUnknown`，同时 cause 仍可被可信代码检查。

`WillBeClosed` 本身不证明 Close 与 COMMIT 的先后，只证明最终关闭；先关闭再提交的顺序还需要和生产控制流源码一起审查。替身也不能验证 `sql.TxOptions` 在 MySQL 的实际隔离行为，更不能验证 InnoDB 锁、错误号、权限、collation 或执行计划。`go-sqlmock v1.5.2` 不会把 MySQL 的 RR/MVCC 语义变成真实事实；这些命题由下一层（本章第 6 节）的真实引擎测试承担。

## 6. 独立 MySQL 8.4.11 集成命令与结果

最终成功证据使用精确命名的临时容器 `growthos-lesson19-final-evidence2-20260829`：

1. 在 tmpfs 数据目录启动 `mysql:8.4.11`；
2. 只发布宿主机 `127.0.0.1` 的随机端口；
3. 通过 TCP root 探针等待正式 server ready，避免把初始化阶段 socket 误判为就绪；
4. 创建独立 `growthos_migrator` / `growthos_app` 身份；
5. 用 Migrator 执行嵌入 Migration 到 clean latest 2；
6. 只向应用身份授予两张表 `SELECT, INSERT`；
7. 带双显式开关运行 `make test-integration-mysql`；
8. 无论成功失败均由 trap 删除这个精确容器。

实际结果：

```text
TestMySQLPoolsIntegration             PASS
TestRunnerMySQLIntegration            PASS
TestLotterySchemaMySQLIntegration     PASS
TestRepositoryMySQLIntegration        PASS (3.21s)
```

四个 package 串行 `-p=1` 执行且使用 `-count=1`，避免缓存结果和跨 package 同时修改隔离 schema。测试结束后再次检查，临时容器已不存在。

证据加固的第一次复验容器是 `growthos-lesson19-final-evidence-20260829`。只读事务内写入探针最初期待直接观察 MySQL 1792，但项目 DSN 固定启用 `RejectReadOnly=true`，`go-sql-driver/mysql` 会把服务端 1792 映射为 `driver.ErrBadConn`；这次执行因此失败，并暴露了测试对 driver 映射的错误假设。断言随后改为验证固定 driver 的公开错误、事务外零残留，并再次用上述 `evidence2` 容器完整通过。失败过程被保留在本 QA 中，不能删除后只展示绿色结果。

## 7. Create 原子性与重复语义

本节分层测试覆盖：

- 含引号、问号、SQL 注释形状和组合 Unicode 的名称通过占位参数原样往返；
- 同一个 AwardID 可在两个不同 Strategy 中存在，符合复合主键身份；
- `math.MaxUint64` StrategyID、AwardID 与 Weight 可写入并扫描回 `uint64`；
- SQL 控制流测试以两个逆序 Award 调用 `Create`，断言同一次 prepare 下按 AwardID 规范顺序执行两个 INSERT；源码与领域归一化共同维持这个顺序；
- 同一 ID 的第二次 Create 返回 `ErrStrategyAlreadyExists`，不是幂等成功；
- 并发创建同一 ID 时恰好一个成功，其余为 AlreadyExists；
- 临时、只命中特定测试 Strategy/Award 的 CHECK 让子插入失败，事务回滚后父子计数都为 0；
- 测试用 CHECK 名称由本轮 ID 派生，只限制本轮探针行，结束时按精确名称移除；
- context 在 Begin 前取消不会留下行；
- 真实 gap lock 配合单连接应用池和会话级 `innodb_lock_wait_timeout=1` 产生 MySQL 1205；公开 `Create` 返回 Retryable，错误链保留 1205，父子行整体回滚且 adapter 没有内部重试；
- 另一条真实 gap lock 探针由约 2 秒 context deadline 中止，释放锁后父子同样无残留，用来区分数据库暂态错误与调用方取消。

只有根表 INSERT 的 1062 被分类为 AlreadyExists。子表 1062 代表领域校验、表结构或事务内数据出现不一致，不能伪装成“Strategy 已存在”，因此落入普通仓储故障。

## 8. FindByID 快照与恢复语义

读取路径在同一个 `sql.LevelRepeatableRead + ReadOnly` 事务中：

```text
BEGIN read-only RR
  SELECT strategy root
  SELECT awards WHERE strategy_id = ? ORDER BY award_id
COMMIT
RestoreAward × N → RestoreStrategy
```

确定性交错测试使用生产 Repository 相同的 `readSnapshotOptions` 和两个 query helper：Reader 先读旧根，Migrator 随后更新根并插入新 Award，Reader 再读 Award。旧事务看到“旧根 + 旧 Award 集”；事务提交后的公开 `FindByID` 看到“新根 + 新 Award 集”。它验证了同一生产事务选项与查询 helper 在当前 MySQL 8.4.11/InnoDB 中的快照行为，但没有在公开 `FindByID` 内增加只为测试服务的暂停 hook；因此证据边界不是“公开入口内部已被任意位置确定性交错”。

真实引擎测试还在使用生产 `readSnapshotOptions` 开始的事务内查询 `@@transaction_isolation`，得到 `REPEATABLE-READ`，随后尝试 INSERT。MySQL 以 1792 拒绝只读事务写入；由于项目固定 `RejectReadOnly=true`，driver 对调用方呈现 `driver.ErrBadConn`。事务外再查确认探针行数为 0。这条证据同时锁定了生产 TxOptions、当前 driver 映射和“没有写入落库”，而不是由 sqlmock 推断只读语义。

Migrator 还会精确注入以下物理上可存在、领域上非法的快照：

- 根存在但没有 Award；
- Strategy 名称含首个 NBSP；
- Award 名称含控制字符；
- 两个合法单行 Weight 的总和溢出 `uint64`。

公开读取全部返回 `ErrStoredStrategyInvalid`，不会返回半聚合、静默 trim、丢弃坏 Award 或重算成另一个事实。不存在根行则单独返回 `ErrStrategyNotFound`。

## 9. 错误分类与取消

当前稳定语义类如下：

| 类别 | 典型来源 | 调用方含义 |
| --- | --- | --- |
| invalid argument / not configured | nil context、nil pool | 编程或装配错误，不应重试 |
| not found | 根查询 `sql.ErrNoRows` | 指定 Strategy 不存在 |
| already exists | 根 INSERT 1062 | Create 身份冲突 |
| stored invalid | Restore 失败 | 权威行不构成合法聚合，失败关闭并告警 |
| retryable | 真实 MySQL 1205；合成 MySQL 1213 分类器单测 | 仅限正常操作阶段；上层在满足业务幂等和预算时才可重放完整事务 |
| commit outcome unknown | 写 COMMIT 响应错误且无法证明取消 | 先按业务身份查询/对账，禁止盲重试 |
| repository failure | SQL、扫描、权限、schema、驱动等其他错误 | 默认不可假设重试有效 |

公开 `Error()` 不包含 SQL、表名、约束名或 driver message；错误链保留 cause 供可信日志、指标与故障定位使用。当前还没有统一业务 service 记录这些类别的指标，这属于已知可观测性缺口。

## 10. 最小权限验证

应用账号在本节的精确 allowlist 为：

```text
GRANT USAGE ON *.*
GRANT SELECT, INSERT ON growthos.lottery_strategy
GRANT SELECT, INSERT ON growthos.lottery_strategy_award
```

真实 schema 集成测试验证排序后的 `SHOW GRANTS` 完全相等，并要求 `@@GLOBAL.mandatory_roles` 为空。它直接执行正向父子 INSERT、SELECT 与 ROLLBACK，并直接验证：

- 两张表 UPDATE：1142；
- 两张表 DELETE：1142；
- `schema_migrations` SELECT：1142；
- `schema_migrations` INSERT/UPDATE：1142；
- 应用身份不执行 Migration 或 DDL。

默认 Compose smoke 同样直接比较精确 grants 与空 mandatory roles；它的直接负向 SQL 探针是根表 UPDATE、根表 DELETE 与 `schema_migrations` SELECT。子表和 migration 表的其余拒绝结论来自“精确 grants 没有其他授权”，完整的逐语句 1142 证据由隔离 schema 集成测试承担。两套证据互补，但不能表述成每一套都执行了全部负向 SQL。

只检查“没有某一条 grant 文本”是不够的：mandatory role 可能提供有效权限，旧 volume 也可能保留通配授权。因此授权作业先撤销旧权限、再建立精确白名单并比较最终事实。

## 11. 查询计划证据

真实 MySQL 对生产 SQL 形状执行 `EXPLAIN`：

- 根查询 `WHERE strategy_id = ?` 使用 `PRIMARY`，访问类型为常量主键查找；
- Award 查询 `WHERE strategy_id = ? ORDER BY award_id` 使用复合 `PRIMARY(strategy_id, award_id)` 的最左前缀；
- 子查询不出现 filesort，主键天然提供 AwardID 顺序。

这只证明当前 schema、SQL 和测试数据下优化器选择符合预期。它不构成未来容量结论；新增过滤、分页、软删除、版本列或排序需求后必须重新以真实分布验证。

## 12. 默认 Compose 回归

在保留既有 `growthos_mysql_data` 与 `growthos_mysql_socket` 的前提下执行：

```bash
make compose-up
make compose-smoke
```

实际 smoke 全部通过：

- MySQL、API、Redis、Web 四个常驻服务健康；
- `migrate`、`mysql-grants` 两个 one-shot 成功退出；
- API/Migrate 镜像标识为 lesson 19；
- Migration clean latest 2；
- 第 18 节名称约束仍匹配不可变 schema；
- 两表精确 `SELECT+INSERT` 且无 UPDATE/DELETE/migration-table access；
- `/health`、`/ready`、SPA 与统一未知 API route 契约通过；
- 只有 Web 发布 `127.0.0.1:8088`。

Repository 当前尚未装配进 `growth-api` handler，所以 smoke 不会也不应通过 HTTP 创建或读取 Strategy。

## 13. 清理记录

已删除本节专用、可抛弃资源：

- `growthos-lesson19-final-hardening-20260829` 临时 MySQL 容器；
- `growthos-lesson19-final-evidence-20260829` 首次证据复验容器；
- `growthos-lesson19-final-evidence2-20260829` 最终成功复验容器；
- 上述容器的 tmpfs 数据均随各自精确 trap 清理消失；
- 先前复验使用的独立 Compose project 资源已按精确 project 名删除；
- 两次 `make verify` 前都确认 `web/dist` 不存在；各自新生成的目录已整体移到本机废纸篓中的 `GrowthOS-Go-lesson19-web-dist-20260829-1846` 与 `GrowthOS-Go-lesson19-web-dist-final-20260829-1903`，均可恢复；没有删除任何源码或依赖。

明确保留：

- 默认 `growthos` Compose 开发栈仍在运行，便于继续第 20 节；
- `growthos_mysql_data` 与 `growthos_mysql_socket` named volumes 未删除；
- `deploy/compose/secrets` 中既有、被 Git 忽略的本地开发凭据未改动；
- Go module 与前端依赖属于可复用项目依赖，不作为临时缓存清理。

章节最终 `make verify`、`go test -race -count=1 ./...`、全仓 shellcheck 与 `make compose-config` 已完成：Go vet/test、`make doc-check`、前端 4 个测试文件共 34 个用例、TypeScript typecheck、Vite production build、race、脚本静态检查和 Compose 配置解析均通过。随后重新执行 `make compose-up` 与 `make compose-smoke`，默认栈完整通过。Vite 报告主 bundle 超过 500 kB 的非阻断 warning；该容量信号保留为未来按真实页面边界拆包的输入，不通过调整 warning 阈值伪装解决。

本机没有配置课程文档同步所需的 `VAULT` 环境变量，因此本节没有执行 Obsidian 同步，也不把 Git 内文档存在误报成外部知识库已同步；Git 仓库中的版本仍是本节唯一已验证的文档事实源。

## 14. 剩余风险

1. 写 COMMIT 的真实网络断点没有可确定答案，代码只能准确返回“结果未知”；业务层仍需设计查询/对账和幂等协议。
2. 真实 1205 已验证；1213 仍只有合成错误分类证据，上层也还没有退避、抖动、截止时间与最大次数策略。
3. 当前 Create 不支持更新；如果未来可修改 Strategy，需要版本、并发控制、读缓存失效和审计设计，不能直接授予 UPDATE。
4. RR 快照保证一次加载内部一致，不保证读到全局最新版本，也不解决读副本延迟。
5. 两次查询适合当前小聚合；Award 数量上限尚未由产品定义，超大集合会增加内存、事务时间和锁/连接占用。
6. 错误 cause 尚未由业务服务转为分类指标和结构化日志；不能仅凭安全公开字符串运营系统。
7. 应用本地开发账号的 host 为 `%` 但服务网络隔离；生产环境必须使用受控来源、TLS 和环境自己的角色策略，不能照抄 Compose。

## 15. 验收结论

第 19 节实现提交 `50ac811`、证据加固提交 `2c420c9` 与文档内容提交 `09556d8` 已推送到 `origin/codex/lesson-19-lottery-repository`。实现层、真实 MySQL 8.4.11、最小权限和默认 Compose 证据通过；文档链接、全仓普通/race/vet、前端既有门禁与最终分支 tip 由本节收尾提交再次核验并登记在[课程分支检查点](../../course/branch-checkpoints.md)。

结论限定为：**GrowthOS 已拥有一个可验证的 Lottery Strategy Create/FindByID 仓储边界；还没有一次可在线调用的抽奖。**
