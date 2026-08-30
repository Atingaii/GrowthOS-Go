# MySQL Migration 运维手册

**适用范围：** GrowthOS-Go 第 13 节及之后的 MySQL 前向 Migration

**当前边界：** 产品迁移 latest 为 2；`000001` / `000002` 分别创建 `lottery_strategy` / `lottery_strategy_award`

## 1. 目的

本手册说明如何安全检查和执行 GrowthOS MySQL Migration，以及遇到 dirty、版本漂移、取消或连接问题时何时必须停止。它不是 MySQL 管理员权限说明，也不授权操作者绕过审批执行 `force`、`drop` 或任意 SQL。

长期设计依据见 [ADR-0010](../decisions/ADR-0010-mysql-migration-boundaries.md)；第 21 节运行时最小权限依据见 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)，配置键和值域见[配置参考](../configuration.md)。

## 2. 角色与权限

| 角色 | 账号 | 权限边界 |
| --- | --- | --- |
| API 进程 | `growthos_app`（可覆盖） | 第 21 节当前运行链只允许两张 Lottery 业务表 `SELECT`；无 INSERT、UPDATE、DELETE、DDL 或 `schema_migrations` 权限 |
| Migration 进程 | `growthos_migrator`（可覆盖） | 仅目标 schema 的审核 DDL、版本记录和必要 DML |
| DBA/管理员 | 部署环境管理的独立身份 | 创建 schema/账号、授权、备份和事故恢复；不进入应用配置 |

不要让 API 使用 Migrator 密码“临时解决权限问题”，也不要让 Migration 借用 API 密码。管理员应通过环境自己的 Secret 与账号管理流程创建身份，再用 `SHOW GRANTS` 核对；仓库不提供真实密码。

第 21 节当前运行应用权限 allowlist 如下（host 范围必须替换为环境实际受控来源，账号应由安全流程预先创建）：

```sql
GRANT SELECT
  ON growthos.lottery_strategy TO 'growthos_app'@'<api-host>';

GRANT SELECT
  ON growthos.lottery_strategy_award TO 'growthos_app'@'<api-host>';

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, INDEX, REFERENCES
  ON growthos.* TO 'growthos_migrator'@'<migration-host>';
```

这不是复制即用的生产授权：生产 host、管理员身份、mandatory role 与撤权流程必须由环境安全设计决定。Repository 代码仍保留 Create/FindByID，但当前 composition root 的 HTTP 用例只依赖 `StrategyReader.FindByID`，所以长期 runtime 不需要 INSERT。第 19 节 Create 由可丢弃 schema 中的隔离 writer 测试身份验证；测试需要写权限不代表产品进程也应拥有写权限。后续增加新写用例、SQL、VIEW、TRIGGER、EVENT 或其他对象时，应从已审核用例重新计算 API/Migrator 最小权限，禁止直接授予应用或 Migrator 全局 `ALL PRIVILEGES`。

Compose 使用 Migration 后一次性 `mysql-grants` 作业收敛这个 allowlist：作业不加入网络，只通过只读 `growthos_mysql_socket` 连接，先撤销应用旧授权，再精确授予两张表 `SELECT`，最后比较排序后的 `SHOW GRANTS`。它还要求 `@@GLOBAL.mandatory_roles` 为空，防止角色在 `SHOW GRANTS` 表面 allowlist 之外隐式扩权；任一断言失败都会阻止 API 启动。其他环境若启用 mandatory role，必须把有效权限纳入独立安全评审，不能直接删掉断言后照搬 Compose 作业。

第 21 节权限变化的上下文与证据见[课程](../course/part-03/lesson-21-lottery-api.md)、[API](../api/lessons/lesson-21.md)、[QA](../qa/lessons/lesson-21.md)、[设计手记](../design-thinking/lessons/lesson-21.md)和[面试问答](../interview/lessons/lesson-21.md)。

## 3. 必要配置

共享非秘密配置：

```text
GROWTHOS_ENVIRONMENT
GROWTHOS_LOG_LEVEL
GROWTHOS_LOG_FORMAT
GROWTHOS_MYSQL_ADDRESS
GROWTHOS_MYSQL_DATABASE
GROWTHOS_MYSQL_TLS_MODE
GROWTHOS_MYSQL_TLS_CA_FILE            # verify_identity 时可选
GROWTHOS_MYSQL_CONNECT_TIMEOUT
GROWTHOS_MYSQL_WRITE_TIMEOUT
```

Migration 专属配置：

```text
GROWTHOS_MYSQL_MIGRATION_USER
GROWTHOS_MYSQL_MIGRATION_PASSWORD     # 必填 Secret
GROWTHOS_MYSQL_MIGRATION_READ_TIMEOUT
GROWTHOS_MYSQL_MIGRATION_STATEMENT_TIMEOUT
GROWTHOS_MYSQL_MIGRATION_LOCK_TIMEOUT
```

`LoadMigration` 不读取 HTTP、API 用户/密码、API Ping 或 API pool 参数。不要在文档、工单、聊天或命令输出中粘贴变量值。优先使用 Secret manager、容器 Secret 或不进入 shell history 的受控注入方式。

staging/production 必须使用 `GROWTHOS_MYSQL_TLS_MODE=verify_identity`。自定义 CA 文件只能在该模式使用；文件必须可读、为有效 PEM 且不超过 1 MiB，证书必须覆盖 `GROWTHOS_MYSQL_ADDRESS` 中的 host。

## 4. 变更文件预检

当前目录规则：

```text
migrations/sql/NNNNNN_description.up.sql
```

发布前逐项核对：

- [ ] 六位版本号非零、递增且不重复；
- [ ] 描述为小写字母开头，只含小写字母、数字、下划线；
- [ ] 没有 `.down.sql`；
- [ ] 已共享的旧文件没有被修改；
- [ ] SQL 没有密码、Token、真实个人数据或环境专属地址；
- [ ] 已评估 MySQL 隐式提交、元数据锁、表大小、磁盘与复制影响；
- [ ] 新旧应用版本在发布窗口内都能兼容 schema；
- [ ] 已在与生产版本一致的影子库/测试库演练；
- [ ] 已准备备份、恢复负责人和停止条件。

当前不可变清单：

| 版本 | 文件 | 单条 DDL | 结果 |
| ---: | --- | --- | --- |
| 1 | `000001_create_lottery_strategy.up.sql` | `CREATE TABLE lottery_strategy` | Strategy 聚合根行、正 ID、基础名称与行时间戳 |
| 2 | `000002_create_lottery_strategy_award.up.sql` | `CREATE TABLE lottery_strategy_award` | 复合主键、正 ID/权重、封闭 outcome、父表 `RESTRICT` 外键与行时间戳 |

每个版本只有一条 MySQL DDL，因为 MySQL atomic DDL 的原子边界是一条语句，不是任意多语句文件。若版本 2 失败，版本 1 可以已经完整存在，而版本表明确记录 version 2 dirty；这比把两个 `CREATE TABLE` 塞入一个看似整体、实际并不跨语句原子的文件更容易恢复。已共享文件只追加不回写，并由嵌入字节 hash 测试保护；任何改动都应新增更高版本。

## 5. 标准发布流程

### 5.1 发布前

1. 确认目标环境、数据库名和构建版本，禁止靠终端历史猜测；
2. 核对变更审批、备份与恢复方案；
3. 用部署 Secret 注入 Migrator 配置；
4. 先执行状态检查：

```bash
make db-status
```

允许继续的状态：

| 状态 | 操作 |
| --- | --- |
| `uninitialized` | 已有首个迁移，确认目标库为空或符合初始条件后继续 |
| `pending` | 核对当前/最新版本与发布内容后继续 |
| `clean` | 当前已部署环境仍必须确认 `version=latest=2`；重复 `up` 应为 `no_change` |

任何命令失败、dirty、version mismatch、未知状态或日志环境不符都必须停止。

### 5.2 执行

```bash
make db-migrate
```

当前成功结果只能是：

- `no_change`：已经 latest 2；
- `applied`：应用了一个或两个待执行版本。

命令退出 0 之后再次检查：

```bash
make db-status
```

应达到 `clean` 且 `version=latest=2`。如果数据库版本高于二进制 latest、dirty、或 latest 不是预期的 2，停止发布并核对构建 SHA 与目标库。

### 5.3 部署 API

Migration 成功并核对后再收敛应用授权，授权通过后才部署 API。`growth-api` 自身不会运行 DDL 或授权，只会用 API 身份打开有界连接池并 Ping。Compose 由依赖链自动执行 `migrate → mysql-grants → api`；其他环境必须提供等价、可审计且失败关闭的发布步骤。应用回滚前必须确认旧版本与新 schema、权限 allowlist 都兼容；二进制回滚不会自动回滚数据库或恢复旧授权。

### 5.4 Schema 约束的责任边界

第 18 节数据库约束不是完整领域模型的替代品：

- `chk_*_name_basic` 只校验非空且没有首尾 ASCII U+0020 空格；它不等价于 Go 名称构造器对 Unicode 空白/控制字符的完整处理；
- 外键只能阻止孤儿 Award 与父 Strategy 被误删，不能保证每个 Strategy 至少有一个 Award；
- 单行 `weight > 0` 不能证明同一 Strategy 跨行权重总和未溢出 `uint64`；
- `lottery_strategy.updated_at` 与 Award 行的 `updated_at` 都只是各自行元数据。更新 Award 不会触碰根行，因此两者都不能充当聚合版本、ETag 或缓存失效水位。

第 19 节 Repository 已按稳定顺序读取根和 Award，并通过 `RestoreAward` / `RestoreStrategy` 原样重建聚合；非法存量数据失败关闭，而不是 trim、跳过坏行或返回半合法对象。Create 在一个事务中写完整父子聚合，但当前产品进程没有调用 Create，也没有 INSERT；该写路径只由隔离 writer 测试身份验证，不应在运维层给 runtime “补回”写权限。

### 5.5 Repository 事务故障的操作边界

- MySQL 1205（lock wait timeout）和 1213（deadlock）只被标记为 retryable；adapter 不自动重试。上层只有在请求幂等、仍有 deadline 预算并采用有界退避时才能重试。
- 根 INSERT 的 1062 表示 Strategy 身份已存在；子 INSERT 的 1062 不等价于根冲突，应按数据/代码不一致调查。
- 写 COMMIT 返回普通驱动错误时，系统返回“commit outcome unknown”。此时数据库可能已经提交，禁止直接重新 Create；先按 StrategyID 查询权威事实、结合请求/审计信息对账，再由用例决定是否补偿。
- 读取使用 read-only RR 快照，保证单次根/子聚合内部一致，不保证绝对最新。不得为了“刷新”在运维层擅自改隔离级别或加锁定读。
- stored-invalid 表示物理行无法构成合法领域聚合，应隔离调用、保留诊断证据并由受控修复流程处理；不要在 SQL 控制台直接 trim 或删除坏行来让告警消失。

## 6. timeout 预算

默认值：

```text
statement 30s + 5s <= migration read 35s + 5s <= lock 40s
```

- 不要把 API `GROWTHOS_MYSQL_READ_TIMEOUT=5s` 当作 Migration read；
- statement/read/lock 使用三个独立变量；
- 相邻 timeout 至少保留 5 秒余量，让 SQL/网络层先返回；
- lock 最小 11 秒，因为上游 MySQL adapter 的 `GET_LOCK` 有固定 10 秒服务端等待；
- 延长 timeout 之前先确认 SQL、锁等待和表规模，不要用更长等待掩盖阻塞。

`lock` 只限制获取数据库锁，不限制 `RELEASE_LOCK`、整个 Migration 或总清理时间。锁获取 timeout 或收到 SIGTERM 时，命令请求 graceful stop 并等待当前 Migration 边界；若执行中的 `up` 被取消，该 Runner 不可复用。产品命令无论如何都会关闭 Runner，新尝试必须使用新进程并先执行 `status`。

## 7. 故障处置

### 7.1 配置、TLS 或连接失败

症状：命令非零退出，安全日志只显示 configuration/open/ping 等阶段。

处理：

1. 不要求应用打印完整驱动错误或 DSN；
2. 从部署系统核对变量是否存在、TLS host/CA 是否匹配、账号 host 范围是否正确；
3. 使用管理员受控工具独立验证网络、证书和 `SHOW GRANTS`；
4. 修复后重新运行 `status`，不要直接重复 `up`。

### 7.2 `migration_dirty`

dirty 表示某个版本执行开始后没有完整成功。后续 Migration 必须停止。

1. 立即停止重复发布和自动重试；
2. 保存构建版本、目标环境、稳定 stage、发生时间与变更审批号；不要复制密码/DSN/SQL 参数；
3. 由 DBA 与变更作者检查实际 schema、MySQL 日志和备份；
4. 决定恢复备份、人工完成剩余步骤或编写纠正 Migration；
5. 如果必须修改版本标记，只能使用审批后的外部管理流程，产品命令不提供 `force`；
6. 恢复后用同一构建先 `status`，确认 clean/pending，再决定是否 `up`。

不要直接删除 `schema_migrations`，不要改写已经共享的 `.up.sql`，不要用 API 账号修表。

### 7.3 `migration_version_mismatch`

数据库报告的 clean 版本不在当前二进制的嵌入历史中，常见原因是连错库、部署了旧二进制或历史被改写。

1. 停止 Migration 和 API 发布；
2. 核对环境、数据库、构建 SHA 和分支；
3. 查找包含该版本的正确发布产物；
4. 若历史被改写，按事故处理，不把当前文件改成迎合数据库版本；
5. 只有确认同一条历史后才能继续。

### 7.4 `migration_cancelled`

执行中的 `up` 被取消后，Runner 已进入 terminal；调用前已取消或 `status` 被取消本身不会将 Runner 标为 terminal。运维上仍一律使用新进程复核：

1. 等待当前进程完成关闭；
2. 不在同一 Runner 上再次调用 `up/status`；
3. 启动新进程，先 `status`；
4. 若状态 dirty，按 dirty 流程处理；否则评估是否重试。

### 7.5 close failure

Migration 执行成功但连接/source 关闭失败时，命令仍非零退出。不要仅凭 `applied` 日志宣布发布成功；用新进程运行 `status` 并检查资源/网络状态后再决策。

## 8. 真实集成测试

普通 `make test` 可能因缺少环境变量而 skip 第 19 节隔离 writer MySQL 集成测试。强制联调使用：

```bash
export GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES=lesson-19-isolated-schema
export GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES=lesson-19-isolated-repository
make test-integration-mysql
```

该目标必须连接专用、可丢弃且事先授权改变结构与写入 Repository fixture 的 schema。两个 opt-in 必须同时为精确值；它在进入 Go 测试前还逐项要求名为 `GROWTHOS_TEST_MYSQL_API_*` 的**隔离 writer 测试身份**与 Migration 身份的 address/database/user/password 八个变量，缺失即失败且只输出变量名。变量中的 `API` 是历史测试命名，不表示当前长期 `growthos_app` 应有写权限。测试会确认两个身份指向同一 address/database 且用户名不同。可选 TLS 变量沿用各测试前缀。不要把真实密码写入示例、shell history 或 QA。

集成测试验证：

- Migrator 应用嵌入迁移后状态为 clean latest 2；
- 两张表使用 InnoDB 与 `utf8mb4_0900_bin`，正 ID/权重、复合主键、外键 `RESTRICT`、封闭 outcome 和名称长度/基础形态约束有效；
- 128 个中文字符和 `uint64` 最大值可往返，组合/分解 Unicode 在二进制 collation 下保持不同；
- 隔离 writer 测试身份可在事务中 INSERT/SELECT 两张业务表并回滚，但 UPDATE、DELETE 和 `schema_migrations` 读写被 MySQL 1142 拒绝；
- 该隔离 writer 测试身份的 SHOW GRANTS 精确等于两表 `SELECT, INSERT` 加全局 USAGE，mandatory role 为空；这不是当前 runtime grant；
- Repository 对 Unicode/SQL 特殊字符和 `uint64` 上限往返，串行/并发重复、父子回滚、取消、坏快照与连接池复用均符合语义；
- 同一 RR 事务在并发修改中读取旧根/旧 Award 集，后续公开读取看到新根/新 Award 集；
- 生产根/子查询的真实 EXPLAIN 使用主键，Award 排序不出现 filesort；
- Migrator 单连接可执行隔离多语句 DDL；
- UTC/`utf8mb4` 会话不变量；
- 临时迁移首次 applied、再次 no_change、最终 clean；
- schema 探针数据在事务中回滚；Repository fixture、定向临时 CHECK、通用临时表和独立版本表完成精确清理。

测试只可以连接专用测试 schema，禁止对共享开发 volume、staging 或 production 运行。显式 opt-in 只是防误操作的一道门，不替代目标地址、数据库归属和清理核对。

## 9. 日志与审计

允许记录：

- service、environment、version；
- component、operation；
- `no_change/applied` 或安全 status；
- migration version/latest；
- 稳定失败 stage、开始/结束时间和外部审批号。

禁止记录：密码、DSN、完整连接 Config、SQL 内容/参数、CA 路径或内容、驱动原始错误、数据库内网 host。需要深度诊断时使用受控 DBA 工具和受限存储，不放宽应用通用日志。

## 10. 清理与交接

每次演练或验收结束：

- 删除任务专用临时二进制、响应和日志目录；
- 确认测试隔离表与隔离版本表已经删除；
- 不删除用户原有容器、Volume、数据库、账号或可复用依赖；
- 记录实际 MySQL 版本、构建 SHA、状态前后值、命令退出码和清理结果；
- 不在工单或 QA 文档粘贴 Secret。

## 11. 官方参考

- [MySQL 8.4 访问控制](https://dev.mysql.com/doc/refman/8.4/en/access-control.html)
- [MySQL 8.4 加密连接](https://dev.mysql.com/doc/refman/8.4/en/using-encrypted-connections.html)
- [Go `database/sql` 连接池](https://pkg.go.dev/database/sql)
- [`go-sql-driver/mysql` v1.10.0 连接池与 timeout](https://github.com/go-sql-driver/mysql/tree/v1.10.0#connection-pool-and-timeouts)
- [`golang-migrate` v4.19.1 dirty/lock FAQ](https://github.com/golang-migrate/migrate/blob/v4.19.1/FAQ.md)
