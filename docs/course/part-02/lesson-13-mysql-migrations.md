# 第 13 节：接入 MySQL 与 Migration

**状态：** 已完成并验收

**日期：** 2026-08-29

**阶段：** Go + React 从零搭建

**本节只建立 MySQL 连接、运行账号隔离、连接池、readiness 与前向 Migration 机制；不创建业务表，也不让 API 进程自动执行 DDL。首个 `000001_*.up.sql` 保留给第 18 节的真实业务建表。**

## 1. 为什么现在接入数据库

第 12 节已经解决配置、日志、请求关联和错误响应，但 `growth-api` 仍没有持久化依赖。直接开始写抽奖仓储会把五类问题混在第一个业务需求里：

1. API 运行账号和 DDL 账号是否应该拥有相同权限；
2. 连接池、首次连接与运行中依赖探测分别是什么语义；
3. 数据库结构如何随章节只追加、不回写历史；
4. Migration 失败、dirty、版本不匹配和取消后能否安全继续；
5. 密码、DSN、SQL 和驱动原始错误如何避免进入日志。

第 13 节先把这些基础设施边界单独实现。这样第 18 节创建第一批业务表、第 19 节实现仓储时，可以专注于领域事实和 SQL，而不再临时发明数据库生命周期。

## 2. 本节范围与非目标

### 2.1 交付范围

- 以 MySQL 8.4 为当前服务端基线；
- 使用 `go-sql-driver/mysql` v1.10.0、`sqlx` v1.4.0 和 `golang-migrate/migrate` v4.19.1；
- 在 `appconfig` 中建立 API 与 Migration 两套独立配置，密码必须由外部注入；
- API 使用受限账号和有界 `sqlx.DB` 连接池，启动前执行有 timeout 的 `PingContext`；
- 保留 `GET /health` 作为进程 liveness，新增 `GET /ready` 作为数据库 readiness；
- 新增独立 `growth-migrate` 命令，只暴露 `up` 和 `status`；
- Migration 从编译期嵌入的 `migrations/sql` 读取严格命名的 `.up.sql`；
- 对 dirty、未知/超前版本、取消和资源关闭使用稳定、安全的失败语义；
- 提供显式 `make test-integration-mysql`，让真实 MySQL 验收不能被普通测试的 skip 掩盖；
- 提供配置参考、ADR、API 记录、QA 记录和运维手册。

### 2.2 明确不做

- 不创建 `strategy`、`strategy_award` 或其他业务表；
- 不添加占位 `000001` Migration；
- 不在 `growth-api` 启动时自动迁移；
- 不暴露 `down`、`drop`、`force`、跳转任意版本或任意 SQL 命令；
- 不实现业务 Repository、事务用例或手写业务 SQL；
- 不接入 Redis、MQ、读写分离、分库分表或数据库代理；
- 不把 `/ready` 当作业务正确性、数据一致性或性能 SLO 证明；
- 不在仓库、示例、日志或 API 响应中保存密码、DSN、CA 内容或真实内网拓扑。

## 3. 技术方案选择

### 3.1 数据访问

| 方案 | 优点 | 当前问题 | 结论 |
| --- | --- | --- | --- |
| ORM 自动建表 | 上手快 | 结构变化原因不清晰，容易把模型反射当作 Migration 历史 | 不采用 |
| 只用 `database/sql` | 依赖最少 | 后续结构体扫描、命名参数与常用查询需要重复样板 | 可行但不作为主线 |
| `sqlx` + 手写 SQL | 保留 SQL 可见性，同时提供结构体扫描与标准库超集接口 | 需要团队维护 SQL 和映射 | 采用 |

`sqlx` 不接管连接池；底层仍是 Go `database/sql`。池大小、空闲连接、连接寿命和 Ping 都在同一个标准生命周期中管理。

### 3.2 Migration

| 方案 | 当前取舍 |
| --- | --- |
| 应用启动自动建表 | 发布与流量启动耦合，多个实例可能竞争 DDL；不采用 |
| 手写版本表与 SQL 执行器 | 可以完全定制，但锁、dirty、版本和 source 生命周期需要重复实现；不采用 |
| `golang-migrate` 库 + 项目受限外壳 | 复用成熟 adapter，只向维护者暴露项目允许的前向命令；采用 |

上游库支持的能力不等于产品命令应该全部开放。GrowthOS 的外壳故意只保留 `up` 和 `status`，把危险恢复操作留在经过审批的人工事故流程中。

## 4. 运行架构

### 4.1 API 启动与关闭

```text
GROWTHOS_* API config
        │
        ▼
appconfig.Load
        │
        ▼
mysqlstore.Open
  ├─ 构造 driver Config（不拼接可打印 DSN）
  ├─ 设置连接池边界
  └─ 有界 PingContext
        │ 成功
        ▼
Gin router + /health + /ready
        │
        ▼
HTTP Server
        │ 停止
        └─ 关闭数据库池
```

数据库配置、connector 或首次 Ping 失败时，API 在监听端口前非零退出。日志只说明稳定阶段 `database startup failed`，不直接格式化驱动错误。停止时 HTTP 生命周期结束后关闭池；关闭失败也让进程失败关闭，而不是伪装成成功。

### 4.2 liveness 与 readiness

```text
GET /health  ──> Gin handler 可响应，不访问 MySQL
GET /ready   ──> 有界 DB Ping ──成功──> 200 ready
                              └─失败──> 503 dependency_unavailable
```

`/health` 只回答“进程是否活着”，因此数据库中断时仍可以返回 200，供平台区分进程崩溃与依赖故障。`/ready` 回答“当前是否可以接收依赖数据库的流量”，失败时使用第 12 节统一 error envelope，且不暴露驱动错误。

### 4.3 独立迁移进程

```text
GROWTHOS_* migration config
        │
        ▼
appconfig.LoadMigration
        │（忽略 HTTP 与 API 账号配置）
        ▼
mysqlstore.OpenMigration
  ├─ 迁移专用账号
  ├─ 单连接池
  └─ 只在此边界开启 multiStatements
        │
        ▼
dbmigration.Runner
  ├─ 扫描并校验嵌入的 .up.sql
  ├─ status
  └─ up
```

`growth-migrate` 与 `growth-api` 是不同命令、不同配置和不同连接池。迁移配置不会因为非法 HTTP 地址、缺失 API 密码或错误 API 池参数而失败；API 配置也不会要求迁移密码。

## 5. 两个数据库身份

| 身份 | 默认用户名 | 允许用途 | 明确禁止 |
| --- | --- | --- | --- |
| API | `growthos_app` | 后续业务 DML、查询与事务 | DDL、版本表维护、任意多语句执行 |
| Migrator | `growthos_migrator` | 审核后的前向 DDL、Migration 版本记录 | 承担在线请求、作为 API 兜底账号 |

MySQL 会在每条语句执行时再次检查账号权限，因此“代码里不调用 DDL”不能替代数据库授权。API 连接器关闭 `multiStatements` 和多种弱化认证/文件访问选项；Migration 因一个版本文件可能包含多条语句，只在专用单连接边界开启 `multiStatements`。

两个密码都没有默认值，最多 1024 bytes，可以包含任意非空内容。错误只返回变量名和约束。含密码的配置类型还实现了脱敏的字符串、结构化日志和 JSON 表示，避免一次调试格式化意外输出整个配置；正确做法仍是只选择必要的非秘密字段记录。

## 6. 连接与会话不变量

连接器使用结构化 driver Config，而不是先拼装一条可被日志复制的 DSN。当前会话固定：

- TCP 地址必须包含非空 host 与 1～65535 端口；
- 数据库名使用严格小写标识符；
- 字符集为 `utf8mb4`；
- 会话时区为 `+00:00`，Go 时间位置为 UTC，并启用 `parseTime`；
- staging/production 必须使用 `verify_identity`；
- 可选 CA 只扩展系统根证书池，TLS disabled 时禁止配置 CA；文件必须可读、为有效 PEM 且不超过 1 MiB；
- TLS 最低版本为 1.2，并校验证书链与目标主机名；
- API 与 Migration 都拒绝只读连接，但不会在日志中公开真实拓扑或凭据。

本地 development/test 可以显式使用 `disabled`，这只是开发便利，不应复制到 staging/production。

## 7. API 连接池与 timeout

当前公开默认值：

| 配置 | 默认值 | 约束/意图 |
| --- | ---: | --- |
| connect timeout | `3s` | 单次建连上限，最大 `30s` |
| read / write timeout | `5s` / `5s` | 单连接网络 I/O 上限，各最大 `5m` |
| startup/readiness Ping | `3s` | 最大 `30s` |
| max open / idle | `10` / `10` | open 为 1～100；idle 为 0～100 且不超过 open |
| connection max lifetime | `3m` | 最大 `1h` |
| connection max idle time | `1m` | 最大 `30m` |

`database/sql` 的默认最大打开连接数是无限，不能直接用于服务。这里先给出小而有界的本地默认值，后续根据真实并发、MySQL `max_connections`、实例数和 `DBStats` 再调整，而不是因为“10 看起来小”就放大。

readiness 还保留固定响应预算：

```text
MySQL Ping 3s + HTTP JSON 响应预算 1s <= HTTP write timeout 30s
```

配置加载器会拒绝破坏该关系的组合，防止 Ping 刚超时、HTTP 写截止也同时到达，最终连稳定 503 都写不出去。

## 8. Migration timeout 层级

默认 Migration timeout 有明确先后顺序：

```text
SQL 执行：statement 30s + 5s <= network read 35s
获取锁：network read 35s + 5s <= lock acquisition 40s
```

三层不是同一个 timeout 的三个名字：

- `statement` 限制单个 SQL 执行；
- migration `read` 是专用连接的网络读截止，不能复用 API 的 `5s`；
- `lock` 只限制获取数据库锁的等待时间，不限制整个 Migration 或释放锁。

相邻 timeout 保留 5 秒余量，但两条关系分别服务不同阶段：执行 SQL 时让 statement timeout 先于驱动网络读截止；获取锁时让网络读截止先于外层 lock-acquire timeout。上游 MySQL adapter 的 `GET_LOCK` 固定使用 10 秒服务端等待，所以锁获取 timeout 不能小于 11 秒；默认选择 40 秒，避免外层获取锁等待先返回而遗留后台锁 goroutine。这个预算不承诺约束 `RELEASE_LOCK`、整个 Migration 或全部清理时间。配置层、MySQL adapter 和 Migration Runner 都重复验证关键关系，防止绕过任一入口。

## 9. 前向 Migration 规则

当前 `migrations/sql` **没有真实 `.up.sql` 文件**，所以：

- `growth-migrate status` 返回 `no_migrations`；
- `growth-migrate up` 返回 `no_migrations`；
- 不连接数据库版本表，也不创建虚假的 `schema_migrations`；
- 第 18 节才加入 `000001_*.up.sql` 并第一次产生真实结构变化。

未来文件必须匹配：

```text
000001_create_strategy_and_award.up.sql
000002_add_example_index.up.sql
```

约束包括六位非零递增版本、稳定小写描述、只允许 `.up.sql`、版本不重复。目录中出现 `.down.sql` 或非法文件名时，在执行 SQL 前失败。已经进入共享分支的 Migration 不得修改；修正通过新版本前向追加。

MySQL DDL 可能发生隐式提交，不能假设一个 Migration 文件可以像普通事务一样完全回滚。每个文件应小、可审查、可在影子库演练，并提前评估锁表与兼容窗口。

## 10. 状态与失败语义

### 10.1 `status` 成功状态

| 状态 | 含义 |
| --- | --- |
| `no_migrations` | 二进制没有嵌入真实 Migration；当前第 13 节即为此状态 |
| `uninitialized` | 已有迁移文件，但数据库还没有版本 |
| `pending` | 数据库版本属于当前历史，但落后于最新嵌入版本 |
| `clean` | 数据库版本与当前嵌入历史一致且非 dirty |

如果数据库报告的 clean 版本不属于当前嵌入历史，或者比当前二进制更新，返回 `migration_version_mismatch`，不能误报为 clean。

### 10.2 `up` 成功状态

| 状态 | 含义 |
| --- | --- |
| `no_migrations` | 没有真实迁移文件 |
| `no_change` | 已经处于最新版本 |
| `applied` | 至少应用了一个待执行版本 |

### 10.3 必须人工处理的失败

- `migration_dirty`：执行中断并留下 dirty 标记；不能继续叠加新版本；
- `migration_version_mismatch`：数据库与当前二进制不是同一条历史；
- `migration_cancelled`：信号/上下文取消；执行中的 `Up` 被取消后，该 Runner 进入 terminal，必须关闭并新建，不能复用；调用前已经取消或 `Status` 被取消不改变 Runner 可复用状态；
- source/config/open/apply/status/close：分别表示安全阶段，不输出驱动 cause、SQL 或 DSN。

Runner 在 Migration engine 已开始执行后的失败或取消路径会重新检查 dirty，dirty 优先于普通 apply/cancelled 报告。产品命令不提供 `force`；处理流程见[MySQL Migration 运维手册](../../runbooks/mysql-migrations.md)。

## 11. 命令边界

```bash
make db-status
make db-migrate
```

等价于：

```bash
go run ./cmd/growth-migrate status
go run ./cmd/growth-migrate up
```

命令必须恰好接收一个严格小写参数。`down`、`drop`、`force`、大写或额外参数返回 usage failure，并且在读取环境、创建 logger 或连接数据库之前停止。

迁移命令记录 `service`、`environment`、`version`、`component=migration`、`operation`、安全结果和版本号；失败只记录阶段消息。密码、DSN、SQL、CA 路径和驱动原始错误都不进入通用日志。

## 12. 测试与验收策略

| 风险 | 自动化保护 |
| --- | --- |
| API/Migrator 配置串用 | 两个独立 loader、独立类型和命令装配测试 |
| 密码因 `%v`、`%#v`、slog 或 JSON 泄露 | 配置类型脱敏边界和哨兵测试 |
| TLS 或地址弱校验 | host/端口、环境 TLS、CA 与证书配置测试 |
| 池没有上限或 Ping 无截止 | pool 归一化、DBStats 与有界 context 测试 |
| `/health` 错误依赖数据库 | liveness/readiness 分离契约测试 |
| `/ready` 泄露驱动错误 | 503 稳定 envelope、request ID 与日志隐私测试 |
| 非法/重复/down Migration 被执行 | 内存 FS source 扫描测试 |
| dirty、版本漂移、取消被误报成功 | Runner 状态机、错误分类和 terminal 测试 |
| 资源所有权不清导致 double close/leak | 构造失败、成功、取消和关闭路径测试 |
| 普通测试 skip 被误认为联调通过 | `make test-integration-mysql` 缺变量即失败的显式目标 |

普通 `go test ./...` 在未提供真实 MySQL 环境变量时允许集成测试 skip，从而保持单元门禁可复现；它不能作为数据库联调证据。只有显式集成目标、实际容器环境和记录的输出共同构成 MySQL 验收。

## 13. 数据库变化

**没有业务结构变化。**

本节加入连接和 Migration 机制，但 `migrations/sql` 只有说明文件。没有 `strategy`、`award`、用户、活动或任何占位业务表，也不会为了测试产品命令而占用 `000001`。真实集成测试使用隔离的临时表/版本表并负责清理，不形成产品 schema 历史。

## 14. API 变化

- `GET /health` body 与第 11～12 节保持不变，仍不访问数据库；
- 新增 `GET /ready`：数据库 Ping 成功返回 200 `ready`；失败返回 503 `dependency_unavailable`；
- 两个端点都返回 `Cache-Control: no-store` 和 `X-Request-ID`；
- 没有新增业务资源 API，React 仍未调用真实后端。

精确契约见[第 13 节 API 记录](../../api/lessons/lesson-13.md)。

## 15. QA 与学习分支

本节分支为 `codex/lesson-13-mysql-migrations`，直接基于第 12 节最终验收 tip。实现提交 `b3f5aa7` 与交叉审查加固提交 `b734463` 均已推送至同名远端分支；完整章节以最终文档提交后的分支 tip 为准。

真实 Docker 验收使用 `mysql:8.4` 容器，服务端实际为 MySQL 8.4.11。显式集成目标非 skip 通过，并验证两个账号、API DDL 1142 拒绝、Migrator 多语句/Runner、空生产迁移集、readiness 故障切换和精确清理。完整命令、结果与剩余风险见[第 13 节 QA](../../qa/lessons/lesson-13.md)。

## 16. 官方资料

- [MySQL 8.4 Reference Manual](https://dev.mysql.com/doc/refman/8.4/en/)
- [MySQL 访问控制与账号管理](https://dev.mysql.com/doc/refman/8.4/en/access-control.html)
- [MySQL 加密连接与 VERIFY_IDENTITY](https://dev.mysql.com/doc/refman/8.4/en/connection-options.html)
- [Go `database/sql`：连接池与 Ping API](https://pkg.go.dev/database/sql)
- [`go-sql-driver/mysql` v1.10.0](https://github.com/go-sql-driver/mysql/releases/tag/v1.10.0)
- [`sqlx` v1.4.0](https://github.com/jmoiron/sqlx/releases/tag/v1.4.0)
- [`golang-migrate` v4.19.1](https://github.com/golang-migrate/migrate/releases/tag/v4.19.1)
- [`golang-migrate` v4.19.1 MySQL adapter 源码](https://github.com/golang-migrate/migrate/blob/v4.19.1/database/mysql/mysql.go)
- [`golang-migrate` v4.19.1 dirty 与数据库锁 FAQ](https://github.com/golang-migrate/migrate/blob/v4.19.1/FAQ.md)

## 17. 本节复盘

第 13 节的重点不是“程序能连上本机 3306”，而是把数据库当作一个需要身份、生命周期、版本和故障语义的基础设施边界：在线流量不拥有 DDL 权限，迁移不借用 API 账号；liveness 不因依赖故障误杀进程，readiness 不在数据库不可用时接收流量；Migration 只向前追加，危险恢复不会因为上游库支持就自动暴露。

当前仍然没有业务表，这是刻意保留的课程节奏。下一步第 14 节的前端框架已经存在；第 15 节将让 React 首次调用后端健康/readiness 契约，第 16 节再把本地基础设施固化为 Compose。真正的 `000001` 要等第 18 节根据抽奖对象和查询需求创建。
