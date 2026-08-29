# 第 18 节 QA：首个 Lottery 业务 schema、Migration 与精确授权

- **对应章节：** [第一次正式业务建表](../../course/part-03/lesson-18-lottery-schema.md)
- **API 记录：** [第 18 节 API 边界](../../api/lessons/lesson-18.md)
- **设计推导：** [第 18 节第一性原理设计手记](../../design-thinking/lessons/lesson-18.md)
- **面试复盘：** [第 18 节面试问答](../../interview/lessons/lesson-18.md)
- **长期决策：** [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)、[ADR-0015](../../decisions/ADR-0015-compose-schema-grant-reconciliation.md)
- **分支：** `codex/lesson-18-lottery-schema`
- **日期：** 2026-08-29

> 本记录把“可复现命令、已实际完成的 schema/Compose 证据”和“文档合并后仍由章节收尾复跑的全仓门禁”分开。不能因为测试源码存在或普通 `go test` 显示 skip 就宣称真实 MySQL 联调通过；本节最终 schema 代码已在独立 MySQL 与两套 Compose 环境复跑，提交信息和文档完成后的全仓输出仍由章节收尾回填到分支检查点。

## 1. 验收边界

本节只证明：

1. 两条前向 Migration 可以在 MySQL 8.4 上建立目标结构；
2. 数据库能执行本节明确声明的列、键、外键和 CHECK；
3. MySQL 与 Go driver 能无损保存本节需要的数值和 Unicode 边界；
4. Migration 的成功、重复执行、dirty 和 latest version 语义保持 fail closed；
5. API 与 Migrator 身份分离，应用账号被收敛到两张表的精确 SELECT；
6. 复用 Compose volume 时不依赖 init 脚本重跑，授权 reconciliation 会撤销遗留权限；
7. Compose 启动门要求 Migration 和授权任务都成功；
8. 原有 liveness/readiness/HTTP/前端构建没有回归。

本节不证明：

- Repository 能加载或写入 Strategy；
- 数据库永远存在至少一个 Award；
- 跨行 Weight 总和不会溢出；
- SQL `name_basic` 等价于完整 Go 名称规则；
- 加权随机算法公平；
- 一次 Draw 有唯一最终结果；
- Lottery API、React 真实抽奖链路或 Redis 缓存已经完成；
- 生产规模 DDL 锁时延、复制延迟、备份恢复或容量 SLO。

## 2. 变更清单与职责

| 文件/入口 | 验收职责 |
| --- | --- |
| [`000001_create_lottery_strategy.up.sql`](../../../migrations/sql/000001_create_lottery_strategy.up.sql) | Strategy 父表、正 ID、名称基础约束、行时间戳 |
| [`000002_create_lottery_strategy_award.up.sql`](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql) | Award 子表、复合主键、RESTRICT 外键、Weight/Outcome 约束 |
| [`embed_test.go`](../../../migrations/embed_test.go) | 初始迁移 checksum 与一文件一语句 |
| [`lottery_schema_integration_test.go`](../../../migrations/lottery_schema_integration_test.go) | 真实 MySQL 结构、边界值、负向约束和权限 |
| [`10-create-growthos-users.sh`](../../../deploy/compose/mysql/init/10-create-growthos-users.sh) | 新 volume 首次创建 app/migrator 身份，不授予 app wildcard |
| [`reconcile-growthos-app-grants.sh`](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh) | 复用 volume 的完整撤权、精确授权、grant/mandatory role 验证 |
| [`compose.yaml`](../../../deploy/compose/compose.yaml) | mysql→migrate→mysql-grants→api 生命周期与 socket 隔离 |
| [`compose-smoke.sh`](../../../scripts/compose-smoke.sh) | live schema version、约束标签、完整 grant、端口和 HTTP 契约 |
| [`Makefile`](../../../Makefile) | 显式破坏性集成 opt-in、Compose migration/grants 入口 |

## 3. Schema 契约验收

### 3.1 目标结构

`lottery_strategy`：

| 项目 | 精确期望 |
| --- | --- |
| Engine | InnoDB |
| Table collation | `utf8mb4_0900_bin` |
| PK | `strategy_id` |
| ID type | `BIGINT UNSIGNED NOT NULL` |
| Name | `VARCHAR(128) ... utf8mb4_0900_bin NOT NULL` |
| Checks | ID > 0；`name_basic` 非空且默认 TRIM 后不变 |
| Timestamps | `DATETIME(6)`，created default；updated default + ON UPDATE |

`lottery_strategy_award`：

| 项目 | 精确期望 |
| --- | --- |
| Engine/collation | InnoDB / `utf8mb4_0900_bin` |
| PK | `(strategy_id, award_id)` |
| FK | `strategy_id` 引用父表；DELETE/UPDATE RESTRICT |
| IDs/Weight | `BIGINT UNSIGNED`；AwardID/Weight > 0 |
| Outcome | `VARCHAR(16) ascii_bin`；只允许 `reward` / `no_reward` |
| Name | `VARCHAR(128)` + `name_basic` |
| Timestamps | Award 行自己的 `DATETIME(6)` 元数据 |

### 3.2 明确不存在的结构

验收时也要查“没有什么”：

- 没有 `AUTO_INCREMENT`；
- 没有 `total_weight`；
- 没有 `status/version/position/deleted_at`；
- 没有 Activity、User、Inventory、Benefit、DrawResult 列/表；
- 没有猜测性的 `name/outcome/updated_at` 二级索引；
- 没有 CASCADE 外键动作；
- 没有旧的 `*_name_canonical` 约束标签。

## 4. Migration 历史验收

### 4.1 静态不可变门禁

复现：

```bash
go test -count=1 ./migrations -run TestInitialLotteryMigrationsRemainImmutable
```

真实通过标准：

- v1 SHA-256 为 `2816792cd7ebaaf70c986c56f89f8207d1cac599deee5d1342d20af7768dcefc`；
- v2 SHA-256 为 `396aa84751e30f66fa7751bf79e389050e16fd1faf54dec0e7836b953efe60e4`；
- 每个文件恰有一个 `;`；
- 命令 exit 0 且没有 skip。

checksum 只能发现字节变化，不能证明 SQL 在所有 MySQL 版本语义相同，也不是发布签名。真实 DDL 仍需集成测试。

### 4.2 干净迁移路径

在一次性 MySQL/schema 上，期望状态序列：

```text
status → uninitialized, latest=2
up     → applied, version=2
status → clean, version=2, latest=2, dirty=false
up     → no_change, version=2
```

Compose live 环境的精确版本查询：

```bash
docker compose --project-name growthos \
  --file deploy/compose/compose.yaml exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_migration_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_migrator \
      --database=growthos --batch --silent --skip-column-names \
      --execute="SELECT CONCAT(version, CHAR(58), dirty) FROM schema_migrations"
  '
```

精确期望为：

```text
2:0
```

### 4.3 dirty 故障路径

故障注入必须只在一次性 schema：预先创建与 v2 冲突的 `lottery_strategy_award`，再执行 `up`。期望：

- v1 已创建并作为已完成历史保留；
- v2 失败；
- 版本状态为 `2:1`；
- 随后的 `status/up` 均 fail closed；
- 不能自动 `force` 或假装两条 DDL 已事务回滚。

本场景验证“一条 DDL 一个版本”的诊断价值，不是推荐恢复手工删除表。恢复必须先用 `SHOW CREATE TABLE`、版本表、日志和发布构件核实真实副作用。

## 5. 真实 MySQL 集成测试

### 5.1 安全前提

该目标会执行 DDL 和负向 DML，必须满足：

- 使用专门创建、可整体丢弃的 schema/container；
- API 与 Migrator 地址、数据库名相同；
- 用户名不同；
- 明确设置精确 opt-in 值；
- 不把真实密码写入 shell history、文档或 Git；
- 测试后按精确容器/schema 名清理，不运行广域 prune。

### 5.2 复现入口

用调用环境的 Secret manager 或临时 shell 注入值：

```bash
export GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES=lesson-18-isolated-schema
export GROWTHOS_TEST_MYSQL_API_ADDRESS=127.0.0.1:<isolated-port>
export GROWTHOS_TEST_MYSQL_API_DATABASE=<disposable-schema>
export GROWTHOS_TEST_MYSQL_API_USER=<restricted-user>
export GROWTHOS_TEST_MYSQL_API_PASSWORD=<temporary-secret>
export GROWTHOS_TEST_MYSQL_MIGRATION_ADDRESS=127.0.0.1:<isolated-port>
export GROWTHOS_TEST_MYSQL_MIGRATION_DATABASE=<disposable-schema>
export GROWTHOS_TEST_MYSQL_MIGRATION_USER=<migration-user>
export GROWTHOS_TEST_MYSQL_MIGRATION_PASSWORD=<temporary-secret>

make test-integration-mysql
```

不要把尖括号占位原样执行。真实通过必须看到这三个包 exit 0，且测试名 `TestLotterySchemaMySQLIntegration` 实际 RUN/PASS 而不是 skip：

```text
./internal/infrastructure/mysql
./internal/infrastructure/migration
./migrations
```

如果缺少 opt-in 或任一变量，Make 目标必须非零失败；这与普通 `go test ./...` 在没有外部环境时允许 skip 是不同语义。

### 5.3 正向边界

| 探针 | 预期 |
| --- | --- |
| 128 个中文字符的 Strategy/Award 名称 | 写入和读回一致 |
| `strategy_id=math.MaxUint64` | 用 Go `uint64` 无损读回父 ID |
| `award_id=math.MaxUint64` | 复合主键中的子 ID 无损读回 |
| `weight=math.MaxUint64` | Weight 用 Go `uint64` 无损读回 |
| `reward` / `no_reward` | 均可写入 |
| `e\u0301` 与 `é` | binary collation 下各自保持原值；精确比较不被折叠 |
| 同一 AwardID 出现在不同 Strategy | 允许，符合 scoped identity |

### 5.4 负向边界与精确 MySQL error

| 操作 | 期望 error number | 证明内容 |
| --- | ---: | --- |
| StrategyID=0 | 3819 | 父 ID CHECK enforced |
| 名称仅 U+0020 空格 | 3819 | `name_basic` enforced |
| 名称以 U+0020 开头 | 3819 | `name_basic` 拒绝 leading space |
| 名称以 U+0020 结尾 | 3819 | `name_basic` 拒绝 trailing space |
| AwardID=0 | 3819 | 子 ID CHECK enforced |
| Weight=0 | 3819 | 正权重 CHECK enforced |
| Outcome=`REWARD` | 3819 | ascii_bin + 封闭值域、大小写敏感 |
| 不存在父 Strategy 的 Award | 1452 | 外键拒绝孤儿 |
| 同一 `(strategy_id, award_id)` 重复 | 1062 | 复合主键唯一 |
| 删除仍有 Award 的 Strategy | 1451 | RESTRICT 生效 |
| 129 个中文字符 | 1406 | strict SQL mode 下 VARCHAR(128) 不截断 |
| 应用账号写业务表 | 1142 | 第 18 节无写权限 |
| 应用账号读/写 `schema_migrations` | 1142 | 版本事实隔离 |

所有数据层探针在 Migrator 事务中执行并回滚。应用写权限负向探针也显式开启事务并回滚：如果错误配置意外放开写权限，测试仍不会把探针行留在数据库。

## 6. 名称契约的诚实验收

数据库 constraint 名为：

```text
chk_lottery_strategy_name_basic
chk_lottery_strategy_award_name_basic
```

`basic` 的验收边界：

| 输入 | SQL 本节是否保证拒绝 | Go 领域是否拒绝 |
| --- | --- | --- |
| 空字符串 | 是（NOT NULL/CHECK） | 是 |
| 只有普通 U+0020 空格 | 是 | 是 |
| 首尾普通 U+0020 | 是 | 会 trim 后规范化 |
| 首尾 Tab/换行/其他 Unicode 空白 | **本节 SQL 不作完整保证** | Go `TrimSpace`/控制字符规则处理 |
| 内部控制字符 | **本节 SQL 不作完整保证** | Go 拒绝 |
| 非法 UTF-8 字节序列 | 由连接/字符集处理，但不等价于领域构造证据 | Go 显式拒绝 |
| Unicode NFC/NFD 同一可见形式 | binary collation 可区分 | Go 当前不做 normalization |

因此，验收不得写“数据库完整实现了第 17 节名称不变量”。第 19 节必须增加坏行重建测试：任一 Award 或 Strategy 不能通过领域构造器时，Repository 返回数据损坏/不变量错误，不能自动清洗后继续。

## 7. 数据库无法独立保证的聚合规则

### 7.1 至少一个 Award

外键只保证“每个 Award 有父 Strategy”，不保证“每个 Strategy 有 Award”。以下状态在物理表上可存在：

```sql
INSERT INTO lottery_strategy(strategy_id, name) VALUES (1, 'empty');
-- 没有子行
```

这在领域上不是合法 Strategy。第 19 节需要：

- 读取时构造器 fail closed；
- 写完整聚合时用事务避免对外可见部分状态；
- 若未来允许草稿空策略，必须新增显式状态而不是悄悄放宽领域。

### 7.2 总权重不溢出

每一行 `weight > 0` 且在 `BIGINT UNSIGNED` 范围内，不代表 `SUM(weight)` 能装进 Go `uint64`。当前没有跨行 CHECK；第 19 节加载必须让 `NewStrategy` 逐项检查溢出，写入也应先验证完整聚合。

### 7.3 行时间戳不是聚合版本

更新 Award 只刷新该 Award 的 `updated_at`，不会自动刷新父 Strategy。验收只能把时间戳称为行元数据，不能把 `MAX(children.updated_at)`、父表 `updated_at` 或微秒值擅自当乐观锁/缓存版本。

## 8. Compose 生命周期与授权验收

### 8.1 配置静态检查

```bash
make compose-config
```

精确期望：Compose 配置可解析，并包含：

```text
mysql healthy
  → migrate service_completed_successfully
  → mysql-grants service_completed_successfully
  → api
```

还应核对：

- `mysql-grants.network_mode: none`；
- 仅 root Secret、只读 `mysql_socket`、只读固定脚本和空 `/var/lib/mysql` bind；
- read-only rootfs、drop ALL、no-new-privileges；
- API/Migrator 不挂 root Secret；
- 只有 MySQL 服务挂 `mysql_data`；
- MySQL/API/Redis/Migrate/Grants 不发布宿主机端口。

### 8.2 启动与 smoke

```bash
make compose-up
make compose-smoke
```

smoke 的成功输出必须逐项包括：

- mysql/api/redis/web running + healthy；
- migrate/mysql-grants exited successfully；
- migrate/api image 标识 lesson-18；
- schema clean `2:0`；
- 两个 `name_basic` 且没有旧 canonical 标签；
- app 能读两业务表；
- app grant 完整集合与 allowlist 相等；
- app 无 `schema_migrations` 访问；
- `/health`、`/ready`、SPA 与 unknown `/api` 404/request ID 契约通过；
- 端口隔离通过。

### 8.3 精确授权内容

用应用身份读取：

```sql
SHOW GRANTS FOR CURRENT_USER;
```

排序后的直接 grant 必须恰好为：

```text
GRANT SELECT ON `growthos`.`lottery_strategy` TO `growthos_app`@`%`
GRANT SELECT ON `growthos`.`lottery_strategy_award` TO `growthos_app`@`%`
GRANT USAGE ON *.* TO `growthos_app`@`%`
```

仅比较直接 grant 仍可能漏掉 MySQL 全局 `mandatory_roles` 产生的隐式有效权限。因此 reconciliation 还必须验证：

```sql
SELECT @@GLOBAL.mandatory_roles;
```

结果为空；非空就失败关闭。这里没有宣称已经穷举 MySQL 所有可能的动态权限/代理配置，固定 Compose 基线通过 init、完整 direct grant 与 mandatory role 三层共同收敛。

### 8.4 复用 volume 场景

验证重点不是重置数据。步骤是：

1. 保留既有 `${project}_mysql_data`；
2. 运行 v1/v2 Migration；
3. 运行 `mysql-grants`；
4. 检查旧 schema wildcard/DML 已消失；
5. 确认业务表数据未被删除；
6. API 只在 reconciliation exit 0 后启动。

init 脚本是否修改不能作为这条升级路径的证据，因为已有 volume 不会重跑 init。

## 9. 安全负向验收

| 风险 | 负向断言 |
| --- | --- |
| API 篡改版本 | SELECT/UPDATE `schema_migrations` 均 1142 |
| API 提前写业务 | INSERT `lottery_strategy` 1142 |
| 旧通配权限残留 | 完整 SHOW GRANTS 相等比较失败 |
| 强制角色隐式扩权 | `@@GLOBAL.mandatory_roles` 非空则 grants job 失败 |
| root 走网络 | grants `network_mode:none` 且使用 `--protocol=socket` |
| root Secret 扩散 | 仅 grants/MySQL 必要边界挂载；API/Migrator 无 root Secret |
| 授权任务访问数据文件 | `/var/lib/mysql` 是显式空只读 bind，不是 `mysql_data` |
| 授权失败仍接流量 | API depends_on grants successful completion |
| 写探针污染数据 | 应用探针事务总是 rollback |

## 10. 全仓回归门禁

最终分支 tip 应依次执行：

```bash
make fmt-check
go vet ./...
go test -count=1 ./...
go test -race ./...
make doc-check
make web-test
make web-typecheck
make web-build
make compose-config
make compose-smoke
```

或使用聚合入口：

```bash
make verify
```

其中 `make verify` 不自动替代显式真实 MySQL schema 测试和 live Compose smoke；三类证据应分别记录。`web-build` 生成的 `web/dist` 若仅用于本次验证，应在完成检查后按精确路径恢复性清理，不删除 `node_modules` 等可复用依赖。

## 11. 已执行事实与文档完成后的最终门禁

最终 schema、constraint 与 mandatory-role 加固代码上已经观察到：

- 在独立 MySQL 8.4.11、独立 loopback 端口和 tmpfs 数据目录上，v1/v2 首次 applied、status clean v2、重复 up no_change；最终显式 `make test-integration-mysql` 对三个目标包通过，包含三个 MaxUint64 与 leading/trailing U+0020 探针；
- 单独故障容器验证 v2 冲突后 `2:1 dirty`，父表 v1 保留；
- 独立 tmpfs MySQL 最终测试容器已按精确名称删除；
- 保留默认 Compose `mysql_data` 的升级路径已把 live schema 推进到 v2，将应用账号收敛为两表 SELECT，并在最终 constraint/mandatory-role 加固后通过 `make compose-smoke`；
- 独立 Compose project `growthosl18verify` 使用全新 named volumes：首次启动 + smoke 通过，重复启动 + smoke 再次通过，证明 init 路径和 reconciliation 幂等路径；随后通过该 project 的 `down --volumes` 精确清理；
- grant 客户端早期产生的匿名 volume `7d7ab52982dfc50b4daf2a93a09712231b8dcbf0642e54c63b8c48e0ee89608a` 已先确认无容器引用，再精确删除；未运行广域 prune。

上述结果已覆盖最终 schema、`name_basic` 标签、空只读 `/var/lib/mysql` 覆盖、direct grant allowlist 与 mandatory roles 失败关闭；不要再描述成“这些加固尚未复跑”。文档、索引和全局当前态合并后，主代理仍需在最终源码 tip 上复跑 `make verify`、`go test -race ./...` 与 `make doc-check`，以证明文档完整性和全仓没有被后续编辑破坏。

## 12. 清理与保留

### 12.1 应清理

- 仅为本节创建的一次性 MySQL 测试容器；
- dirty 故障注入容器；
- 授权客户端意外产生且已确认无容器引用的匿名 volume；
- shell/smoke 临时目录；
- 仅为验证生成的 `web/dist`。

清理前必须先按精确 ID/名称确认归属和引用；禁止 `docker system prune`、`docker volume prune`、通配删除或删除用户原有资源。

### 12.2 应保留

- `growthos_mysql_data`：现有开发数据；
- `growthos_mysql_socket`：当前 Compose 拓扑需要的命名 volume；
- 当前 lesson-18 API/Migrate 镜像和正常运行容器；
- Compose Secret 文件；
- `node_modules`、Go module cache 等可复用依赖；
- 源码、Migration、QA 和所有可交付文档。

## 13. 偏离检查

| 课程承诺 | 实际检查 | 判定原则 |
| --- | --- | --- |
| 只建两张业务表 | schema/迁移文件列表 | 不出现其他业务表 |
| 不做 Repository | `internal/lottery` 与 SQL 调用检查 | 无 repository/adapter/业务查询 |
| 不做算法 | Lottery 包检查 | 无随机源或选择逻辑 |
| 不做业务 API | route/API 测试 | 未知 route 仍 404 |
| 不改真实前端 | `/lottery` 数据源检查 | 仍为 Mock |
| 不接 Redis 业务 | API 配置/依赖检查 | 无 Redis client/key/readiness |
| 数据库只守可表达子集 | constraint/文档检查 | 不把 name_basic、FK 夸大为完整聚合合法性 |
| 应用最小权限 | SHOW GRANTS + mandatory_roles + 1142 | 只有两表 SELECT，隐式强制角色为空 |

## 14. 剩余风险

1. Repository 尚未出现，坏行 fail-closed 只是下一节必须实现的要求；
2. 真实数据量为零或很小，当前没有索引/查询性能证据；
3. 没有生产规模在线 DDL、备份恢复或副本验证；
4. `SHOW GRANTS` 文本 allowlist 与 fixed MySQL 版本耦合；
5. root reconciliation 不是事务，中间失败会暂时撤销应用权限；
6. binary collation 不提供语言学搜索、Unicode normalization 或反欺骗；
7. 行时间戳可能被后续开发者误当聚合版本；
8. 空 Strategy 与总权重溢出仍可通过绕过 Repository 的多行写形成；
9. 没有定义生产数据库身份和部署 job 的等价实现；
10. 本节没有业务写路径，所以权限扩展策略仍需第 19 节用真实 SQL证明。

## 15. 验收结论模板

章节最终收尾时只在真实复跑后填写：

```text
实现提交：<sha>
文档提交：<sha>
最终检查点：<sha>
真实 MySQL：PASS / FAIL（MySQL 版本、独立 schema、无 skip）
Compose smoke：PASS / FAIL
全仓 verify/race：PASS / FAIL
清理：<精确目标与结果>
```

任何失败必须保留失败阶段、修复和复跑证据，不能只保留最后一行绿色输出。
