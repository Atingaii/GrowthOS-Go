# GrowthOS-Go 配置参考

**状态：** 第 16 节已完成并验收

**更新日期：** 2026-08-29

**来源章节：** [第 12 节：配置、日志与错误体系](course/part-02/lesson-12-config-logging-errors.md)、[第 13 节：接入 MySQL 与 Migration](course/part-02/lesson-13-mysql-migrations.md)、[第 15 节：前后端第一次联调](course/part-02/lesson-15-first-fullstack-integration.md)、[第 16 节：Docker Compose 开发环境](course/part-02/lesson-16-docker-compose-development.md)

本页记录 `growth-api`、`growth-migrate`、Vite 开发/预览进程与 Compose 开发栈真正读取的配置边界。Go 侧只有 `internal/platform/appconfig` 读取进程环境和明确指向的密码文件；HTTP、数据库、Migration 和业务包只接收已经校验的类型化值。前端代理配置由运行 Vite 的 Node.js 进程独立读取，不经过 Go `appconfig`，也不会暴露给浏览器代码；Compose 再负责把本地文件秘密和容器网络地址装配给对应进程。

## 1. 加载规则

```text
代码内公开、非秘密默认值
          ↓ 被显式覆盖
进程环境变量 GROWTHOS_*
```

- 项目自有环境变量统一使用 `GROWTHOS_` 前缀；
- 非秘密变量未设置时使用默认值；已设置但为空/全空白时失败，不静默回退；
- 两个密码没有默认值；每个进程必须在直接变量与对应 `_FILE` 变量中恰好选择一个。密码允许任意非空 bytes，最多 1024 bytes；文件来源只去除末尾 CR/LF，不裁剪其他空白；
- 枚举严格小写；duration 使用 Go 语法，例如 `500ms`、`5s`、`2m`；
- 配置错误尽量聚合，失败返回零值，只报告变量名与约束，不回显原值；
- 启动时一次加载，不支持热更新；
- `configs/growth-api.env.example` 只列公开值，不自动加载，也不给密码写占位赋值。

`appconfig.Default()` 和 `DefaultMigration()` 只表示公开默认值，不是可直接启动的完整配置；其中 Password 有意为空。生产入口必须调用 `Load` 或 `LoadMigration`。

## 2. Vite 开发与预览代理配置

第 15 节的系统状态页只请求浏览器当前 origin 下的 `/health` 与 `/ready`；Vite 的 Node.js 进程负责把这些路径代理到 Go API。运行前端工具链需要 Node.js `>=22.22.2` 与 pnpm `10.13.1`。

| 进程环境变量 | 默认值 | 允许值 / 校验 | 读取者 |
| --- | --- | --- | --- |
| `GROWTHOS_WEB_API_PROXY_TARGET` | `http://127.0.0.1:8080` | 仅 `http` / `https` origin；禁止 username、password、非根路径、query 与 fragment | Vite Node.js 进程 |
| `PORT` | dev `5173`；preview `4173` | 十进制安全整数，范围 1～65535 | Vite Node.js 进程 |

`GROWTHOS_WEB_API_PROXY_TARGET` 刻意不使用 `VITE_` 前缀：它由 `vite.config.ts` 通过带 `GROWTHOS_WEB_` 前缀过滤的 `loadEnv` 读取，只供服务端代理使用，不应进入浏览器 `import.meta.env`。Go 的 `internal/platform/appconfig` 也不读取该变量。有效值必须只表达一个 origin，例如 `https://api.example.com`；`https://user:pass@example.com`、`https://api.example.com/base`、`https://api.example.com?x=1` 与带 fragment 的值都会在 Vite 启动前失败。代理目标中禁止凭据，但这也不代表可以把任何 API Secret 放入其他前端环境变量。

开发服务器默认监听 `127.0.0.1:5173`，生产构建预览默认监听 `127.0.0.1:4173`；两者都可由合法的进程变量 `PORT` 覆盖，并启用 `strictPort`，端口占用时直接失败而不是静默换端口。两种模式使用同一代理边界，只代理精确的 `/health`、`/ready`，以及 `/api` 或 `/api/...`；相似前缀不会被代理。浏览器因此只看到同源路径，不拥有上游 origin 的配置权。

这套 Vite 配置是本地开发与构建预览适配器，不是生产反向代理方案。第 16 节的 Compose 路径使用 Nginx 作为本地容器入口，并继续保持“浏览器同源、上游地址只存在于服务端代理、API/MySQL/Redis 不发布宿主机端口”的边界。

## 3. Compose 开发栈装配输入

Compose 文件位于 `deploy/compose/compose.yaml`。仓库级 Make 目标默认使用项目名 `growthos`，只把 Web 绑定到回环地址；下面的值属于本地装配层，不会进入浏览器 bundle：

| 输入 | 默认值 | 约束 / 用途 |
| --- | --- | --- |
| Make 变量 `COMPOSE_PROJECT` | `growthos` | Compose 资源命名空间；smoke 只接受字母、数字、点、下划线和连字符 |
| `GROWTHOS_COMPOSE_WEB_PORT` | `8088` | 1～65535；仅形成 `127.0.0.1:<port>` 的 Web 发布端口 |
| `deploy/compose/secrets/mysql_root_password` | 首次由脚本随机生成 | 只挂载给 MySQL，用于官方入口初始化和管理健康边界 |
| `deploy/compose/secrets/mysql_app_password` | 首次由脚本随机生成 | 挂载给 MySQL 与 API；API 通过 `GROWTHOS_MYSQL_PASSWORD_FILE` 读取 |
| `deploy/compose/secrets/mysql_migration_password` | 首次由脚本随机生成 | 挂载给 MySQL 与一次性 Migrator；后者通过 `GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE` 读取 |
| `deploy/compose/secrets/redis_password` | 首次由脚本随机生成 | 只挂载给 Redis；当前 API 不读取 Redis 配置 |

`make compose-up` 会先运行 Secret 生成器。四个文件必须“全部存在或全部不存在”：完整集合会复用；部分集合直接失败；如果 `growthos_mysql_data` 已存在而完整 Secret 集缺失，脚本拒绝生成新值，避免持久化数据库与凭据静默失配。本地目录权限为 `0700`，文件为 `0444`；后者兼容 Docker Desktop 文件挂载给非 root 容器，真正的容器可见范围仍由逐服务只读挂载限制。它们是被 Git 和 Docker build context 排除的本地开发文件，不是加密 Secret Manager，也不支持热轮换。

## 4. 通用进程配置

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

## 5. MySQL 共享连接配置

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_MYSQL_ADDRESS` | `127.0.0.1:3306` | 非空 host 的合法 `host:port`，端口 1～65535 | TCP endpoint |
| `GROWTHOS_MYSQL_DATABASE` | `growthos` | `[a-z][a-z0-9_]{0,63}` | 目标 schema |
| `GROWTHOS_MYSQL_TLS_MODE` | `disabled` | `disabled` / `verify_identity` | TLS 策略 |
| `GROWTHOS_MYSQL_TLS_CA_FILE` | 未设置 | 可选非空路径；`disabled` 时禁止设置；文件须可读、为有效 PEM 且不超过 1 MiB | 为系统根证书池增加 CA |
| `GROWTHOS_MYSQL_CONNECT_TIMEOUT` | `3s` | `> 0` 且 `<= 30s` | 单次连接 timeout |
| `GROWTHOS_MYSQL_WRITE_TIMEOUT` | `5s` | `> 0` 且 `<= 5m` | 单连接网络写 timeout |

staging/production 必须使用 `verify_identity`。该模式验证证书链与 `ADDRESS` 中的主机名，TLS 最低 1.2；可选 CA 是扩展系统根，不是关闭验证。CA 路径或解析失败只报告稳定阶段，不回显路径或证书内容。development/test 允许显式 disabled。

## 6. API 数据库与连接池配置

`appconfig.Load` 只要求 API 密码，不读取 Migrator 账号、密码或执行 timeout。

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_MYSQL_USER` | `growthos_app` | 1～32 个有效可打印 Unicode 字符；无控制字符与首尾空白 | 最小权限 API 账号 |
| `GROWTHOS_MYSQL_PASSWORD` | **无，二选一** | 任意非空值，最多 1024 bytes；与 `_FILE` 互斥且不回显 | API 直接 Secret |
| `GROWTHOS_MYSQL_PASSWORD_FILE` | **无，二选一** | 非空文件路径；读取上限有界，只去除末尾 CR/LF；错误不回显路径或内容 | API 文件 Secret |
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

## 7. Migration 专属配置

`appconfig.LoadMigration` 只要求 Migration 密码。它复用 environment/log、endpoint、database、TLS、connect/write timeout，但不读取 API user/password/read/ping/pool 或 HTTP 配置。

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_MYSQL_MIGRATION_USER` | `growthos_migrator` | 1～32 个有效可打印 Unicode 字符；无控制字符与首尾空白 | 独立 DDL 账号 |
| `GROWTHOS_MYSQL_MIGRATION_PASSWORD` | **无，二选一** | 任意非空值，最多 1024 bytes；与 `_FILE` 互斥且不回显 | Migrator 直接 Secret |
| `GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE` | **无，二选一** | 非空文件路径；读取上限有界，只去除末尾 CR/LF；错误不回显路径或内容 | Migrator 文件 Secret |
| `GROWTHOS_MYSQL_MIGRATION_READ_TIMEOUT` | `35s` | `> 0` 且 `<= 10m30s` | Migration 专用网络读 timeout |
| `GROWTHOS_MYSQL_MIGRATION_STATEMENT_TIMEOUT` | `30s` | `> 0` 且 `<= 10m` | 单条 Migration SQL timeout |
| `GROWTHOS_MYSQL_MIGRATION_LOCK_TIMEOUT` | `40s` | `>= 11s` 且 `<= 11m` | 获取数据库锁的 timeout；不限制整个 Migration 或解锁 |

三个 timeout 必须保持独立且满足：

```text
statement + 5s <= migration read
migration read + 5s <= lock
```

默认分成两条关系：SQL 执行阶段满足 `statement 30s + 5s <= read 35s`，获取锁阶段满足 `read 35s + 5s <= lock-acquire 40s`。Migration read 不能复用 API 的 `GROWTHOS_MYSQL_READ_TIMEOUT=5s`。lock 至少 11 秒，是为了覆盖上游 MySQL adapter 固定 10 秒的 `GET_LOCK` 服务端等待，避免外层获取锁等待先返回。它不承诺限制 `RELEASE_LOCK`、整个 Migration 或全部清理时间。

## 8. 无秘密示例

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

直接密码和 `_FILE` 路径都不出现在通用赋值示例中；操作者按运行方式选择一种来源。Compose 已在服务级环境中指向 `/run/secrets/...`，手工运行则应由受控进程环境或本机未跟踪文件注入。Secret 注入完成后分别运行：

```bash
make api-run
make db-status
make db-migrate
```

不要把 DSN 作为替代变量提交，也不要在 shell 命令、截图或 QA 文档中展开密码。

## 9. 失败与秘密语义

以下情况在连接/监听前失败：

- Vite 代理目标不是无凭据、无非根路径、无 query/fragment 的 HTTP(S) origin，或 `PORT` 非法；
- 密码的直接来源与文件来源同时存在或同时缺失，文件路径为空、不可读，或解析后的密码为空/超过 1024 bytes；
- MySQL 地址缺少 host/port，数据库名或用户名不合法；
- TLS 枚举、环境 TLS 组合或 CA 组合不合法；
- duration/整数无法解析、越界或破坏 timeout/池关系；
- API opener、TLS、connector 或首次 Ping 失败；
- Migration source/状态不安全。

配置错误只列变量名和允许范围，不含原值。`Config`、`MySQLConfig`、`MigrationConfig` 与 `MigrationMySQLConfig` 在 `String`、`GoString`、`slog.LogValuer` 和 JSON 边界返回整体脱敏文本；数据库 adapter 的含密码 Config 也遵守同一规则。

整体脱敏是最后防线，不是记录配置对象的许可。日志仍应只选择 environment、component、operation、版本号等非秘密字段。驱动错误可能包含账号、主机、SQL 或拓扑，API/Migration 入口不能直接格式化它。

## 10. 配置所有权

Vite 与 Go 使用两个互不越权的配置入口：

```text
GROWTHOS_WEB_API_PROXY_TARGET / PORT
                 │
          Vite Node.js 进程
                 │
       dev / preview 同源代理
                 │
       浏览器只请求同源路径
```

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
- Vite 不读取 Go API/Migration Secret，浏览器也不读取代理目标；
- `mysqlstore` 不读取环境变量，只接收类型化配置；
- `dbmigration` 不拥有凭据，只接收已经打开且所有权明确的专用连接；
- `/ready` 只接收最小 `PingContext` 接口，不解析数据库配置。

## 11. 变更规则

新增或修改配置必须同步：

1. 类型化结构、默认值、校验、聚合错误和不泄漏测试；
2. `configs/growth-api.env.example`（Secret 仍不得赋值）；
3. 本参考、课程正文和相关 runbook；
4. 部署资产、真实集成变量和 QA 冒烟；
5. 跨组件 timeout/容量关系；
6. 若改变安全、兼容或长期运维约束，新增或替代 ADR。

当前没有配置热更新、远程配置中心、加密 Secret Manager 或秘密热轮换。第 16 节的本地生成器只解决可复现开发装配；第 76 节若加入 Nacos，它也只能成为新的输入适配器，不能绕过本页类型、校验、账号隔离和秘密边界。
