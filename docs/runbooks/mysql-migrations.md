# MySQL Migration 运维手册

**适用范围：** GrowthOS-Go 第 13 节及之后的 MySQL 前向 Migration

**当前边界：** 生产迁移集为空；首个真实 `000001_*.up.sql` 在第 18 节加入

## 1. 目的

本手册说明如何安全检查和执行 GrowthOS MySQL Migration，以及遇到 dirty、版本漂移、取消或连接问题时何时必须停止。它不是 MySQL 管理员权限说明，也不授权操作者绕过审批执行 `force`、`drop` 或任意 SQL。

长期设计依据见 [ADR-0010](../decisions/ADR-0010-mysql-migration-boundaries.md)，配置键和值域见[配置参考](../configuration.md)。

## 2. 角色与权限

| 角色 | 账号 | 权限边界 |
| --- | --- | --- |
| API 进程 | `growthos_app`（可覆盖） | 仅后续业务需要的 DML/查询权限，不授予 DDL |
| Migration 进程 | `growthos_migrator`（可覆盖） | 仅目标 schema 的审核 DDL、版本记录和必要 DML |
| DBA/管理员 | 部署环境管理的独立身份 | 创建 schema/账号、授权、备份和事故恢复；不进入应用配置 |

不要让 API 使用 Migrator 密码“临时解决权限问题”，也不要让 Migration 借用 API 密码。管理员应通过环境自己的 Secret 与账号管理流程创建身份，再用 `SHOW GRANTS` 核对；仓库不提供真实密码。

一个最小权限方向示例（host 范围必须替换为环境实际受控来源，账号应由安全流程预先创建）：

```sql
GRANT SELECT, INSERT, UPDATE, DELETE
  ON growthos.* TO 'growthos_app'@'<api-host>';

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, INDEX, REFERENCES
  ON growthos.* TO 'growthos_migrator'@'<migration-host>';
```

这只是起点，不是复制即用的生产授权。后续 Migration 使用 VIEW、TRIGGER、EVENT 或其他对象时，应按已审核文件增加最小权限，禁止直接授予全局 `ALL PRIVILEGES`。

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

第 13 节迁移集为空。不要为了让命令“看起来执行过”而添加空 `000001`；`no_migrations` 是当前正确结果。

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
| `no_migrations` | 当前第 13 节正常；无需生成版本表 |
| `uninitialized` | 已有首个迁移，确认目标库为空或符合初始条件后继续 |
| `pending` | 核对当前/最新版本与发布内容后继续 |
| `clean` | 若已是最新，无需重复变更；`up` 应为 `no_change` |

任何命令失败、dirty、version mismatch、未知状态或日志环境不符都必须停止。

### 5.2 执行

```bash
make db-migrate
```

成功结果只能是：

- `no_migrations`：没有真实文件；
- `no_change`：已经最新；
- `applied`：应用了待执行版本。

命令退出 0 之后再次检查：

```bash
make db-status
```

有真实迁移时应达到 `clean` 且数据库版本等于二进制嵌入的 latest。当前空迁移集仍为 `no_migrations`。

### 5.3 部署 API

Migration 成功并核对后再部署 API。`growth-api` 自身不会运行 DDL，只会用 API 身份打开有界连接池并 Ping。应用回滚前必须确认旧版本与新 schema 兼容；二进制回滚不会自动回滚数据库。

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

普通 `make test` 可能因缺少环境变量而 skip MySQL 集成测试。强制联调使用：

```bash
make test-integration-mysql
```

该目标在进入 Go 测试前逐项要求 API 与 Migration 的 address/database/user/password 八个变量，缺失即失败且只输出变量名。可选 TLS 变量沿用各测试前缀。

集成测试验证：

- 两个身份都能连接并查询；
- API DDL 被 MySQL 权限拒绝；
- Migrator 单连接可执行隔离多语句 DDL；
- UTC/`utf8mb4` 会话不变量；
- 临时迁移首次 applied、再次 no_change、最终 clean；
- 临时表和独立版本表清理。

测试只可以连接专用测试 schema，禁止对共享 staging/production 运行会创建/删除隔离表的测试。

## 9. 日志与审计

允许记录：

- service、environment、version；
- component、operation；
- `no_migrations/no_change/applied` 或安全 status；
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
