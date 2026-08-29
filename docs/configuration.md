# GrowthOS-Go 配置参考

**状态：** 第 13 节已完成并验收

**更新日期：** 2026-08-29

**来源章节：** [第 12 节：配置、日志与错误体系](course/part-02/lesson-12-config-logging-errors.md)、[第 13 节：接入 MySQL 与 Migration](course/part-02/lesson-13-mysql-migrations.md)

本页记录 `growth-api` 与 `growth-migrate` 真正读取的配置边界。只有 `internal/platform/appconfig` 读取进程环境；HTTP、数据库、Migration 和业务包只接收已经校验的类型化值。

## 1. 加载规则

```text
代码内公开、非秘密默认值
          ↓ 被显式覆盖
进程环境变量 GROWTHOS_*
```

- 项目自有环境变量统一使用 `GROWTHOS_` 前缀；
- 非秘密变量未设置时使用默认值；已设置但为空/全空白时失败，不静默回退；
- 两个密码没有默认值，必须显式注入。密码允许任意非空 bytes，最多 1024 bytes；
- 枚举严格小写；duration 使用 Go 语法，例如 `500ms`、`5s`、`2m`；
- 配置错误尽量聚合，失败返回零值，只报告变量名与约束，不回显原值；
- 启动时一次加载，不支持热更新；
- `configs/growth-api.env.example` 只列公开值，不自动加载，也不给密码写占位赋值。

`appconfig.Default()` 和 `DefaultMigration()` 只表示公开默认值，不是可直接启动的完整配置；其中 Password 有意为空。生产入口必须调用 `Load` 或 `LoadMigration`。

## 2. 通用进程配置

API 与 Migration 都读取 environment/log；只有 API 读取 HTTP 配置。

| 环境变量 | 默认值 | 允许值 / 校验 | 进程 |
| --- | --- | --- | --- |
| `GROWTHOS_ENVIRONMENT` | `development` | `development` / `test` / `staging` / `production` | 两者 |
| `GROWTHOS_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` | 两者 |
| `GROWTHOS_LOG_FORMAT` | `json` | `json` / `text` | 两者 |
| `GROWTHOS_HTTP_ADDRESS` | `:8080` | 合法 `host:port`，端口 1～65535；HTTP 允许空 host | 仅 API |
| `GROWTHOS_HTTP_SHUTDOWN_TIMEOUT` | `5s` | `> 0` 且 `<= 2m` | 仅 API |
| `GROWTHOS_HTTP_READ_HEADER_TIMEOUT` | `5s` | `> 0` 且 `<= 30s` | 仅 API |
| `GROWTHOS_HTTP_READ_TIMEOUT` | `15s` | `> 0` 且 `<= 5m` | 仅 API |
| `GROWTHOS_HTTP_WRITE_TIMEOUT` | `30s` | `> 0` 且 `<= 10m` | 仅 API |
| `GROWTHOS_HTTP_IDLE_TIMEOUT` | `60s` | `> 0` 且 `<= 10m` | 仅 API |

`LoadMigration` 有意忽略所有 HTTP 变量。即使宿主环境存在非法 HTTP 地址或 timeout，也不能阻止独立迁移命令运行。

## 3. MySQL 共享连接配置

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_MYSQL_ADDRESS` | `127.0.0.1:3306` | 非空 host 的合法 `host:port`，端口 1～65535 | TCP endpoint |
| `GROWTHOS_MYSQL_DATABASE` | `growthos` | `[a-z][a-z0-9_]{0,63}` | 目标 schema |
| `GROWTHOS_MYSQL_TLS_MODE` | `disabled` | `disabled` / `verify_identity` | TLS 策略 |
| `GROWTHOS_MYSQL_TLS_CA_FILE` | 未设置 | 可选非空路径；`disabled` 时禁止设置；文件须可读、为有效 PEM 且不超过 1 MiB | 为系统根证书池增加 CA |
| `GROWTHOS_MYSQL_CONNECT_TIMEOUT` | `3s` | `> 0` 且 `<= 30s` | 单次连接 timeout |
| `GROWTHOS_MYSQL_WRITE_TIMEOUT` | `5s` | `> 0` 且 `<= 5m` | 单连接网络写 timeout |

staging/production 必须使用 `verify_identity`。该模式验证证书链与 `ADDRESS` 中的主机名，TLS 最低 1.2；可选 CA 是扩展系统根，不是关闭验证。CA 路径或解析失败只报告稳定阶段，不回显路径或证书内容。development/test 允许显式 disabled。

## 4. API 数据库与连接池配置

`appconfig.Load` 只要求 API 密码，不读取 Migrator 账号、密码或执行 timeout。

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_MYSQL_USER` | `growthos_app` | 1～32 个有效可打印 Unicode 字符；无控制字符与首尾空白 | 最小权限 API 账号 |
| `GROWTHOS_MYSQL_PASSWORD` | **无，必填** | 任意非空值，最多 1024 bytes；不回显 | API Secret |
| `GROWTHOS_MYSQL_READ_TIMEOUT` | `5s` | `> 0` 且 `<= 5m` | API 单连接网络读 timeout |
| `GROWTHOS_MYSQL_PING_TIMEOUT` | `3s` | `> 0` 且 `<= 30s` | 启动与 `/ready` Ping |
| `GROWTHOS_MYSQL_MAX_OPEN_CONNS` | `10` | 整数 1～100 | 最大打开连接 |
| `GROWTHOS_MYSQL_MAX_IDLE_CONNS` | `10` | 整数 0～100，且 `<= MAX_OPEN_CONNS` | 最大空闲连接；0 表示不保留 |
| `GROWTHOS_MYSQL_CONN_MAX_LIFETIME` | `3m` | `> 0` 且 `<= 1h` | 单连接最大复用寿命 |
| `GROWTHOS_MYSQL_CONN_MAX_IDLE_TIME` | `1m` | `> 0` 且 `<= 30m` | 单连接最大空闲时间 |

跨组件还必须满足：

```text
GROWTHOS_MYSQL_PING_TIMEOUT + 1s
  <= GROWTHOS_HTTP_WRITE_TIMEOUT
```

1 秒是 readiness 失败写入 503 envelope 的固定响应预算。连接池设置在首次 Ping 前完成；首次 Ping 失败时池被关闭，API 不监听。

## 5. Migration 专属配置

`appconfig.LoadMigration` 只要求 Migration 密码。它复用 environment/log、endpoint、database、TLS、connect/write timeout，但不读取 API user/password/read/ping/pool 或 HTTP 配置。

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_MYSQL_MIGRATION_USER` | `growthos_migrator` | 1～32 个有效可打印 Unicode 字符；无控制字符与首尾空白 | 独立 DDL 账号 |
| `GROWTHOS_MYSQL_MIGRATION_PASSWORD` | **无，必填** | 任意非空值，最多 1024 bytes；不回显 | Migrator Secret |
| `GROWTHOS_MYSQL_MIGRATION_READ_TIMEOUT` | `35s` | `> 0` 且 `<= 10m30s` | Migration 专用网络读 timeout |
| `GROWTHOS_MYSQL_MIGRATION_STATEMENT_TIMEOUT` | `30s` | `> 0` 且 `<= 10m` | 单条 Migration SQL timeout |
| `GROWTHOS_MYSQL_MIGRATION_LOCK_TIMEOUT` | `40s` | `>= 11s` 且 `<= 11m` | 获取数据库锁的 timeout；不限制整个 Migration 或解锁 |

三个 timeout 必须保持独立且满足：

```text
statement + 5s <= migration read
migration read + 5s <= lock
```

默认分成两条关系：SQL 执行阶段满足 `statement 30s + 5s <= read 35s`，获取锁阶段满足 `read 35s + 5s <= lock-acquire 40s`。Migration read 不能复用 API 的 `GROWTHOS_MYSQL_READ_TIMEOUT=5s`。lock 至少 11 秒，是为了覆盖上游 MySQL adapter 固定 10 秒的 `GET_LOCK` 服务端等待，避免外层获取锁等待先返回。它不承诺限制 `RELEASE_LOCK`、整个 Migration 或全部清理时间。

## 6. 无秘密示例

以下与 `configs/growth-api.env.example` 同口径，只展示公开值：

```dotenv
GROWTHOS_ENVIRONMENT=development
GROWTHOS_HTTP_ADDRESS=:8080
GROWTHOS_HTTP_SHUTDOWN_TIMEOUT=5s
GROWTHOS_HTTP_READ_HEADER_TIMEOUT=5s
GROWTHOS_HTTP_READ_TIMEOUT=15s
GROWTHOS_HTTP_WRITE_TIMEOUT=30s
GROWTHOS_HTTP_IDLE_TIMEOUT=60s
GROWTHOS_LOG_LEVEL=info
GROWTHOS_LOG_FORMAT=json

GROWTHOS_MYSQL_ADDRESS=127.0.0.1:3306
GROWTHOS_MYSQL_DATABASE=growthos
GROWTHOS_MYSQL_TLS_MODE=disabled
GROWTHOS_MYSQL_CONNECT_TIMEOUT=3s
GROWTHOS_MYSQL_READ_TIMEOUT=5s
GROWTHOS_MYSQL_WRITE_TIMEOUT=5s
GROWTHOS_MYSQL_USER=growthos_app
GROWTHOS_MYSQL_PING_TIMEOUT=3s
GROWTHOS_MYSQL_MAX_OPEN_CONNS=10
GROWTHOS_MYSQL_MAX_IDLE_CONNS=10
GROWTHOS_MYSQL_CONN_MAX_LIFETIME=3m
GROWTHOS_MYSQL_CONN_MAX_IDLE_TIME=1m

GROWTHOS_MYSQL_MIGRATION_USER=growthos_migrator
GROWTHOS_MYSQL_MIGRATION_READ_TIMEOUT=35s
GROWTHOS_MYSQL_MIGRATION_STATEMENT_TIMEOUT=30s
GROWTHOS_MYSQL_MIGRATION_LOCK_TIMEOUT=40s
```

`GROWTHOS_MYSQL_PASSWORD` 和 `GROWTHOS_MYSQL_MIGRATION_PASSWORD` 不出现在赋值示例中。Secret 注入完成后分别运行：

```bash
make api-run
make db-status
make db-migrate
```

不要把 DSN 作为替代变量提交，也不要在 shell 命令、截图或 QA 文档中展开密码。

## 7. 失败与秘密语义

以下情况在连接/监听前失败：

- 必填密码缺失、为空或超过 1024 bytes；
- MySQL 地址缺少 host/port，数据库名或用户名不合法；
- TLS 枚举、环境 TLS 组合或 CA 组合不合法；
- duration/整数无法解析、越界或破坏 timeout/池关系；
- API opener、TLS、connector 或首次 Ping 失败；
- Migration source/状态不安全。

配置错误只列变量名和允许范围，不含原值。`Config`、`MySQLConfig`、`MigrationConfig` 与 `MigrationMySQLConfig` 在 `String`、`GoString`、`slog.LogValuer` 和 JSON 边界返回整体脱敏文本；数据库 adapter 的含密码 Config 也遵守同一规则。

整体脱敏是最后防线，不是记录配置对象的许可。日志仍应只选择 environment、component、operation、版本号等非秘密字段。驱动错误可能包含账号、主机、SQL 或拓扑，API/Migration 入口不能直接格式化它。

## 8. 配置所有权

```text
                    os.LookupEnv
                         │
               internal/platform/appconfig
                  ┌──────┴────────┐
                  │               │
                Load        LoadMigration
                  │               │
        growth-api Config   growth-migrate Config
          ┌───────┼──────┐        │
          │       │      │        │
       logging httpserver mysqlstore.Open
                          │        │
                       sqlx.DB  mysqlstore.OpenMigration
                                      │
                              dbmigration.Runner
```

- `growth-api` 不读取 Migration Secret，也不执行 DDL；
- `growth-migrate` 不读取 HTTP 或 API Secret/池参数；
- `mysqlstore` 不读取环境变量，只接收类型化配置；
- `dbmigration` 不拥有凭据，只接收已经打开且所有权明确的专用连接；
- `/ready` 只接收最小 `PingContext` 接口，不解析数据库配置。

## 9. 变更规则

新增或修改配置必须同步：

1. 类型化结构、默认值、校验、聚合错误和不泄漏测试；
2. `configs/growth-api.env.example`（Secret 仍不得赋值）；
3. 本参考、课程正文和相关 runbook；
4. 部署资产、真实集成变量和 QA 冒烟；
5. 跨组件 timeout/容量关系；
6. 若改变安全、兼容或长期运维约束，新增或替代 ADR。

当前没有配置热更新、远程配置中心或仓库内 Secret 管理。第 76 节若加入 Nacos，它只能成为新的输入适配器，不能绕过本页类型、校验、账号隔离和秘密边界。
