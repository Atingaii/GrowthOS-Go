# MySQL Migration 运维手册

**适用范围：** GrowthOS-Go 第 13 节及之后的 MySQL 前向 Migration

**当前边界：** 产品源码 Migration latest 为 14；旧 `000001`～`000011` 字节历史保留，`000012`～`000014` 依次新增 workforce account、Session 与 authentication throttle 三张 Identity 表，当前共十三张业务/Identity 表。第 28 节 v5、第 30 节 disposable/长期 v5→v11 仍是各自时间切片的历史证据；第 32 节独立 MySQL 8.4.11 schema/Repository/runtime-grant 与最终代码/文档门禁已实际通过，生产 TLS 与部署三账号完整轮换仍为 `PENDING`。

## 1. 目的

本手册说明如何安全检查和执行 GrowthOS MySQL Migration，以及遇到 dirty、版本漂移、取消或连接问题时何时必须停止。它不是 MySQL 管理员权限说明，也不授权操作者绕过审批执行 `force`、`drop` 或任意 SQL。

长期设计依据见 [ADR-0010](../decisions/ADR-0010-mysql-migration-boundaries.md)；第 21 节运行时最小权限依据见 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)，第 28 节 graph 持久化边界见 [ADR-0024](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)，第 30 节 snapshot/Activity 边界见 [ADR-0026](../decisions/ADR-0026-activity-publication-binding.md)，第 32 节 Identity 边界见 [ADR-0028](../decisions/ADR-0028-identity-session-authentication.md)，配置键和值域见[配置参考](../configuration.md)。

## 2. 角色与权限

| 角色 | 账号 | 权限边界 |
| --- | --- | --- |
| API 进程 | `growthos_app`（可覆盖） | 当前运行链只允许旧两张 Lottery 业务表 `SELECT`；对 graph 三表、snapshot 两表、Marketing 三表共八张未装配表零权限，也无 INSERT、UPDATE、DELETE、DDL 或 `schema_migrations` 权限 |
| Identity runtime / maintenance | `growthos_identity`（可覆盖） | workforce account `SELECT` 与 `UPDATE(updated_at)`；Session/throttle `SELECT, INSERT, UPDATE, DELETE`；拒绝 credential/status/epoch 写入、业务表、`schema_migrations`、DDL/GRANT。maintenance 只复用此既有 DELETE 能力，不新增权限 |
| Identity provisioner | `growthos_identity_provisioner`（可覆盖） | 只可向 workforce account `INSERT`；不能 readback、UPDATE、DELETE、upsert，不能访问 Session/throttle、业务或 Migration 表 |
| Migration 进程 | `growthos_migrator`（可覆盖） | 仅目标 schema 的审核 DDL、版本记录和必要 DML |
| legacy Repository 集成测试 | 任务专用隔离 writer | 仅旧 Strategy/Award 两表 `SELECT, INSERT`；不等于 API 运行身份 |
| graph Repository 集成测试 | 任务专用隔离 graph writer | 仅新 graph/node/edge 三表 `SELECT, INSERT`；不等于 API 运行身份，且 graph adapter 尚未装配 |
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

这不是复制即用的生产授权：生产 host、管理员身份、mandatory role 与撤权流程必须由环境安全设计决定。当前 HTTP 用例只读取旧 Strategy/Award；graph、Strategy snapshot 与 Marketing Activity publication 的 domain/application/Repository/ACL 全部未装配。所以长期 runtime 既不需要 INSERT，也不需要读取另外八表。隔离测试写身份只用于可丢弃 schema，不能据此扩宽产品进程。后续增加运行时写用例时，应从已审核用例重新计算最小权限，禁止直接授予应用或 Migrator 全局 `ALL PRIVILEGES`。

Compose 使用 Migration 后一次性 `mysql-grants` 作业收敛当前 allowlist：作业不加入网络，只通过只读 `growthos_mysql_socket` 连接，先撤销三个目标账号的旧 direct grants，再精确授予 business 两表只读、Identity runtime 三表能力和 provisioner 单表 INSERT，最后比较排序后的 `SHOW GRANTS`、执行 credential/capability probe，并确认 `@@GLOBAL.mandatory_roles` 为空。长期 v11 栈对 graph/snapshot/Marketing 八表的 1142 拒绝仍是第 30 节历史证据；第 32 节当前 smoke/operations 不能被反向写成当时已经存在 Identity 表或 provisioner。

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
| 3 | `000003_create_lottery_strategy_routing_graph.up.sql` | `CREATE TABLE lottery_strategy_routing_graph` | `(graph_id, revision)` header、schema version 与逻辑 root；revision 使用 ASCII binary 语义 |
| 4 | `000004_create_lottery_strategy_routing_node.up.sql` | `CREATE TABLE lottery_strategy_routing_node` | decision/strategy_target 互斥节点、同 revision 复合 scope 与 Strategy target `RESTRICT` 引用 |
| 5 | `000005_create_lottery_strategy_routing_edge.up.sql` | `CREATE TABLE lottery_strategy_routing_edge` | 同 revision 两端复合外键、source-scoped branch 唯一性与 default 映射 |
| 6 | `000006_create_lottery_strategy_snapshot.up.sql` | `CREATE TABLE lottery_strategy_snapshot` | `(strategy_id, revision)` create-only snapshot header、binary revision 与名称快照 |
| 7 | `000007_create_lottery_strategy_snapshot_award.up.sql` | `CREATE TABLE lottery_strategy_snapshot_award` | exact Strategy revision 内 Award 内容、正权重与父 snapshot `RESTRICT` 外键 |
| 8 | `000008_create_marketing_activity.up.sql` | `CREATE TABLE marketing_activity` | Marketing Activity root、draft/published/retired、`state_version` 与 nullable active publication identity |
| 9 | `000009_create_marketing_activity_publication.up.sql` | `CREATE TABLE marketing_activity_publication` | immutable numeric publication version、exact graph ref、`[starts_at, ends_at)` 与 rollback provenance |
| 10 | `000010_create_marketing_activity_publication_strategy.up.sql` | `CREATE TABLE marketing_activity_publication_strategy` | publication 内 exact Strategy snapshot manifest；只建 Marketing 内部 FK，不跨 Lottery 建 FK |
| 11 | `000011_add_marketing_activity_active_publication_fk.up.sql` | `ALTER TABLE marketing_activity` | 追加 Activity active publication 反向复合 FK；没有新增第六张表 |
| 12 | `000012_create_identity_workforce_account.up.sql` | `CREATE TABLE identity_workforce_account` | binary canonical account/login/Principal、Argon2id envelope、enabled/disabled、credential version 与 authentication epoch |
| 13 | `000013_create_identity_session.up.sql` | `CREATE TABLE identity_session` | digest-only opaque Session、issue/revoke operation refs、双 expiry/epoch/revocation、容量与有界 cleanup 索引、account `RESTRICT` FK |
| 14 | `000014_create_identity_authentication_throttle.up.sql` | `CREATE TABLE identity_authentication_throttle` | login/source HMAC digest 聚合、failure/inflight/admission epoch、lease/backoff/window 与有界 cleanup 索引 |

每个版本只有一条 MySQL DDL，因为 MySQL atomic DDL 的原子边界是一条语句，不是任意多语句文件。若某版本失败，之前版本可以已经完整存在，而版本表明确记录当前 dirty。第 30 节必须在旧五表之后按 `snapshot -> snapshot_award -> activity -> publication -> publication_strategy -> active FK` 前向执行。已共享文件只追加不回写，并由嵌入字节 hash 测试保护；任何改动都应新增更高版本。

第 32 节再按 `workforce account -> session -> authentication throttle` 前向执行。三张表都使用 InnoDB、ASCII binary identifier 语义和数据库可表达的局部 CHECK/UNIQUE/FK；完整 password envelope、Session 生命周期、五会话上限与 throttle admission 仍由 Identity domain/application/Repository 校验。Migration 不写默认账号、明文 password、可复用 hash、raw Session/CSRF token 或 fixture。

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
| `clean` | 使用当前构建时仍必须确认 `version=latest=14`；重复 `up` 应为 `no_change` |

任何命令失败、dirty、version mismatch、未知状态或日志环境不符都必须停止。

### 5.2 执行

```bash
make db-migrate
```

当前成功结果只能是：

- `no_change`：已经 latest 14；
- `applied`：应用了一个或多个待执行版本。

命令退出 0 之后再次检查：

```bash
make db-status
```

应达到 `clean` 且 `version=latest=14`。如果数据库版本高于二进制 latest、dirty、或 latest 不是预期的 14，停止发布并核对构建与目标库。已经运行的 volume 不会因源码更新自动升级；只有实际 `status -> up -> status` 完成，才可记录该实例达到 v14。第 30 节长期 `growthos` 在保持 MySQL/Redis/Web/网络/卷 identity 的前提下从 v5 原地升级 v11 并通过当时的 status、权限负证与 smoke，这是保留的历史时间切片，不自动证明今天的 volume 已到 v14；每个环境仍必须独立执行和记录 v11→v14 及新的 Identity grant 结果。

### 5.3 部署 API

Migration 成功并核对后再收敛三个目标账号的授权，授权通过后才部署 API 或运行 Identity operations。`growth-api` 自身不会运行 DDL 或授权，只会用 business/Identity 两个独立 runtime 身份打开有界连接池并 Ping；provisioner 与 maintenance 各自只打开一个短命连接。Compose 由依赖链自动执行 `migrate → mysql-grants → api`，operations wrapper 也先证明 `mysql-grants` 精确 `exited:0`；其他环境必须提供等价、可审计且失败关闭的发布步骤。应用回滚前必须确认旧版本与新 schema、权限 allowlist 都兼容；二进制回滚不会自动回滚数据库或恢复旧授权。

### 5.4 Schema 约束的责任边界

第 18 节数据库约束不是完整领域模型的替代品：

- `chk_*_name_basic` 只校验非空且没有首尾 ASCII U+0020 空格；它不等价于 Go 名称构造器对 Unicode 空白/控制字符的完整处理；
- 外键只能阻止孤儿 Award 与父 Strategy 被误删，不能保证每个 Strategy 至少有一个 Award；
- 单行 `weight > 0` 不能证明同一 Strategy 跨行权重总和未溢出 `uint64`；
- `lottery_strategy.updated_at` 与 Award 行的 `updated_at` 都只是各自行元数据。更新 Award 不会触碰根行，因此两者都不能充当聚合版本、ETag 或缓存失效水位。

第 19 节 Repository 已按稳定顺序读取根和 Award，并通过 `RestoreAward` / `RestoreStrategy` 原样重建聚合；非法存量数据失败关闭，而不是 trim、跳过坏行或返回半合法对象。Create 在一个事务中写完整父子聚合，但当前产品进程没有调用 Create，也没有 INSERT；该写路径只由隔离 writer 测试身份验证，不应在运维层给 runtime “补回”写权限。

第 28 节 graph 三表同样只承担数据库可靠表达的局部事实：node/edge 复合外键收紧同 revision scope，root/可达/环/深度仍由完整领域恢复验证。

第 30 节 Strategy snapshot 两表同样 create-only；Marketing 三表只拥有 Activity/publication 生命周期。publication 与 Lottery graph/snapshot 不建跨 bounded-context FK，只保存 exact refs，由 Lottery ACL 与 resolve fail-closed 补上语义闭合。Marketing 内部 FK 保证 publication/manifest/active identity 的局部引用，领域仍负责 exact terminal 集、不可变版本、rollback source、时间窗和状态机。发布、回滚和退役的 `state_version` CAS 冲突不能被运维层改成覆盖写或盲目重试；历史 publication 禁止原地 UPDATE/DELETE。

第 32 节 Identity 三表同样不是完整认证协议的替代品：数据库只表达 binary identifier、唯一 login/Principal、非零 digest/epoch、局部时间与 revocation/throttle shape，以及 Session→account 的 `RESTRICT` 引用。Argon2 PHC 严格解析、unknown/disabled dummy work、session-bound CSRF、Origin、每账户五会话事务、touch/revoke race、双维 admission lease 与公开错误仍由应用代码负责。`growthos_identity` 对 workforce account 的 `UPDATE(updated_at)` 是 MySQL 8.4.11 锁读所需列级能力，不授权它修改 login、credential、status 或 epoch；创建账号只能经 INSERT-only provisioner。

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

### 8.1 第 32 节当前实现与执行证据

当前已验收源码已经把 Migration latest 推进到 14，并将 Identity runtime、INSERT-only provisioner 与 maintenance 的 SQL 能力写入 Compose grant reconciliation/smoke。两个 disposable provision Compose、official maintenance、development browser 与 HTTP core wire 各自已有证据；本节只登记独立数据库门禁，不用其他层代替 schema/grant。

长驻 provision 实跑曾因 `docker compose up --wait` 把已成功完成的 `mysql-grants` `exited:0` 当作等待失败而停止。提交 `af4245e` 已让 provision/maintenance wrapper 最长 180 秒轮询唯一 container 的 exact state：只接受 `exited:0`，等待 `created:0`/`running:0`/`restarting:0`，对非零退出、歧义、意外状态、inspect 失败和超时关闭。该事实只证明 prerequisite 判定已修复，不把一次成功升级成所有数据库场景均通过。

第 32 节现提供显式 opt-in 的独立门禁：

```bash
GROWTHOS_LESSON32_MYSQL_ACCEPTANCE=run-disposable-mysql-8.4.11 \
  make lesson32-mysql-acceptance
```

脚本使用随机 name/精确 label、随机 loopback port、tmpfs 数据目录与私有 root/Migrator/runtime Secret。它先运行 `TestIdentitySchemaMySQLIntegration`：fresh v14、second-up、v11→v14、旧业务结构/数据 fingerprint、Identity schema/index/FK/CHECK/binary semantics、dirty fail-closed；测试结束后确认数据库为 0 表。随后运行 migration immutability/inventory，用真实 `growth-migrate up/status` 恢复 v14，授予 runtime workforce `SELECT + UPDATE(updated_at)` 和 Session/throttle DML，再执行 `TestRepositoryMySQL84Acceptance` 的 credential、并发 fencing、锁等待取消、Session lifecycle、maintenance 与 grant deny。

本轮从 HEAD `4149576` 执行，容器 `growthos-lesson32-mysql-e4e83e6c1b0e7036f42e65f9`、label `com.growthos.acceptance.lesson32=run-e4e83e6c1b0e7036f42e65f9`，`VERSION()=8.4.11`，19 秒 exit 0。真实 migrator 报 `applied/14`，status 为 `clean/14/latest=14`；终态 `schema_migrations=14:0`、workforce/session/throttle `0:0:0`、`identity_l32_forbidden=0`。runtime 正向 lock-read/updated_at 成功，credential/login update、account insert/delete、migration/Lottery/Marketing read 与 DDL 被拒绝；helper 还严格比较 direct grants 与空 mandatory role。

脚本对随机 container ID/name/label 做精确清理，Secret 按长度覆写后 unlink 并删私有目录；SSD、CoW、快照与控制器重映射意味着这不是物理不可恢复保证。外部复核 name/label/temp 均为 0，长期 `growthos` containers/volumes/networks 前后快照一致。该门禁只创建 Migrator/runtime 两个测试账号；不能据此单独宣称 production host/TLS、长期 business/provisioner 身份轮换或 raw COMMIT outcome-unknown 网络注入已通过。

### 8.2 第 28/30 节历史与通用隔离入口

普通 `make test` 会在缺少显式 opt-in 时 skip 真实 MySQL 集成测试。第 28 节推荐使用自包含的一次性门禁：

```bash
make lesson28-mysql-acceptance
```

该目标启动随机名称/label、动态回环端口、tmpfs 数据目录的一次性 `mysql:8.4.11`，生成仅存在任务临时目录的 Secret，创建彼此隔离的 Migrator、legacy writer 与 graph repository 身份，应用 latest 5 后运行六组 `Integration` 测试。当前第 28 节验收已真实 exit 0：六组全部通过，任务容器与 Secret 目录零残留，长期 `growthos` containers/volumes/networks 前后快照一致。若镜像原本不存在，下载的 `mysql:8.4.11` 会作为可复用依赖保留，而不是作为“临时垃圾”删除。

第 30 节提供并已执行独立门禁：

```bash
GROWTHOS_LESSON30_MYSQL_ACCEPTANCE=run-disposable-mysql-8.4.11 \
  make lesson30-mysql-acceptance
```

该门禁已在一次性 MySQL 8.4.11 上通过：先建立含 7 条非空 FK 行的真实 v5 基线，再前向到 v11；旧五表结构与数据哈希不变，重复执行为 `no_change`，dirty/restore 可恢复。新五表共验证 6 个 `RESTRICT` FK、20 个 CHECK 与 binary collation，Marketing→Lottery 跨上下文 FK 数为 0；隔离 snapshot/Marketing writer 保持最小权限，越界操作真实返回 1142。Repository 路径覆盖 snapshot 并发/回滚和 Activity publish/replace/rollback/retire、RR、CAS 与 half-write rollback，任务资源完成精确清理。

需要接入外部**专用可丢弃 schema**时，强制联调使用：

```bash
export GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES=lesson-19-isolated-schema
export GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES=lesson-19-isolated-repository
export GROWTHOS_TEST_MYSQL_ALLOW_RULE_GRAPH_WRITES=lesson-28-isolated-rule-graph
make test-integration-mysql
```

该目标必须连接专用、可丢弃且事先授权改变结构与写入 fixture 的 schema。三个 opt-in 必须同时为精确值；进入 Go 测试前还逐项要求 Migration 身份、历史命名为 `GROWTHOS_TEST_MYSQL_API_*` 的 legacy writer 身份，以及 `GROWTHOS_TEST_MYSQL_RULE_GRAPH_*` graph repository 身份的 address/database/user/password。三个身份必须指向同一隔离 schema 且用户名互异。变量中的 `API` 不表示当前长期 `growthos_app` 应有写权限；可选 TLS 变量沿用各前缀。不要把真实密码写入示例、shell history 或 QA。

以下清单描述第 28 节历史 v5 门禁已经验证的范围，不覆盖 v11：

- Migrator 应用嵌入迁移后状态为 clean latest 5；
- 两张表使用 InnoDB 与 `utf8mb4_0900_bin`，正 ID/权重、复合主键、外键 `RESTRICT`、封闭 outcome 和名称长度/基础形态约束有效；
- 128 个中文字符和 `uint64` 最大值可往返，组合/分解 Unicode 在二进制 collation 下保持不同；
- 隔离 writer 测试身份可在事务中 INSERT/SELECT 两张业务表并回滚，但 UPDATE、DELETE 和 `schema_migrations` 读写被 MySQL 1142 拒绝；
- 该隔离 writer 测试身份的 SHOW GRANTS 精确等于两表 `SELECT, INSERT` 加全局 USAGE，mandatory role 为空；这不是当前 runtime grant；
- Repository 对 Unicode/SQL 特殊字符和 `uint64` 上限往返，串行/并发重复、父子回滚、取消、坏快照与连接池复用均符合语义；
- 同一 RR 事务在并发修改中读取旧根/旧 Award 集，后续公开读取看到新根/新 Award 集；
- 生产根/子查询的真实 EXPLAIN 使用主键，Award 排序不出现 filesort；
- graph 三表精确列、binary revision、四组 FK（其中 graph scope 与 edge 两端为复合 FK）、局部 CHECK/UNIQUE、同 revision scope、大小写 revision 共存和尾空白拒绝符合 DDL；header root 无反向 FK 且由领域恢复验证；
- legacy writer 的精确权限仍只有旧两表 `SELECT, INSERT`，读取或写入 graph 三表被拒绝；graph repository 身份的精确权限只覆盖新三表 `SELECT, INSERT`，不能访问旧两表、`schema_migrations`、UPDATE 或 DELETE；
- graph Repository 覆盖 create/round-trip、新 revision、重复、完整 `uint64`/nullable scan、并发 create、子行约束回滚、not-found、坏存量、取消、真实 read-only RR 和 EXPLAIN；
- Migrator 单连接可执行隔离多语句 DDL；
- UTC/`utf8mb4` 会话不变量；
- 临时迁移首次 applied、再次 no_change、最终 clean；
- schema 探针数据在事务中回滚；legacy/graph Repository fixture、定向临时 CHECK、通用临时表和独立版本表完成精确清理。

第 30 节 v11 门禁已经证明：旧五表结构/数据哈希前后不变；五张新表及 `000011` FK 精确生效；Marketing 只存在内部 FK；publication/manifest 不跨上下文引用 Lottery FK；隔离 writer 遵守最小权限；失败路径和任务资源完成精确清理。长期应用身份的旧两表 `SELECT` 与八表拒绝另由长期 Compose status/smoke 实证。

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
- `lesson28-mysql-acceptance` 只可停止同时匹配本次精确 container ID、随机 name 与 label 的容器，并只删除已解析的任务 Secret 文件/目录；不得按前缀、通配符或全局 prune 扩大清理范围；
- `lesson30-mysql-acceptance` 同样只能清理本次解析出的精确 container ID/name/label 和任务 Secret 目录；不得误删长期 `growthos` 或其他验收任务资源；
- `lesson32-mysql-acceptance` 同样只清理由本轮精确 container ID/name/label 共同证明归属的容器与私有临时目录；Secret 先按已知长度覆写再 unlink，但 SSD、CoW、快照和控制器重映射下不能宣称物理不可恢复。本轮外部 name/label/temp 复核为零，长期 `growthos` containers/volumes/networks 前后快照一致；
- 不删除用户原有容器、Volume、数据库、账号或可复用依赖；
- 记录实际 MySQL 版本、构建 SHA、状态前后值、命令退出码和清理结果；
- 不在工单或 QA 文档粘贴 Secret。

## 11. 官方参考

- [MySQL 8.4 访问控制](https://dev.mysql.com/doc/refman/8.4/en/access-control.html)
- [MySQL 8.4 加密连接](https://dev.mysql.com/doc/refman/8.4/en/using-encrypted-connections.html)
- [Go `database/sql` 连接池](https://pkg.go.dev/database/sql)
- [`go-sql-driver/mysql` v1.10.0 连接池与 timeout](https://github.com/go-sql-driver/mysql/tree/v1.10.0#connection-pool-and-timeouts)
- [`golang-migrate` v4.19.1 dirty/lock FAQ](https://github.com/golang-migrate/migrate/blob/v4.19.1/FAQ.md)
