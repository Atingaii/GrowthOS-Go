# GrowthOS-Go 配置参考

**状态：** 第 32 节 Identity 双池运行时、Session HTTP、独立 provision/maintenance 配置与 Compose 安全注入已进入实现候选；最终冻结验收待完成

**更新日期：** 2026-09-02

**来源章节：** [第 12 节：配置、日志与错误体系](course/part-02/lesson-12-config-logging-errors.md)、[第 13 节：接入 MySQL 与 Migration](course/part-02/lesson-13-mysql-migrations.md)、[第 15 节：前后端第一次联调](course/part-02/lesson-15-first-fullstack-integration.md)、[第 16 节：Docker Compose 开发环境](course/part-02/lesson-16-docker-compose-development.md)、[第 18 节：第一次正式业务建表](course/part-03/lesson-18-lottery-schema.md)、[第 19 节：实现仓储层](course/part-03/lesson-19-lottery-repository.md)、[第 21 节：开放第一个 Lottery API](course/part-03/lesson-21-lottery-api.md)、[第 24 节：第一次 Redis 业务缓存](course/part-03/lesson-24-redis-strategy-cache.md)、[第 32 节产品基线](product/identity-session-authentication-v1.md)、[ADR-0028](decisions/ADR-0028-identity-session-authentication.md)

本页记录 `growth-api`、`growth-migrate`、`growth-identity-provision`、`growth-identity-maintenance`、Vite 开发/预览进程、显式 MySQL 集成测试与 Compose 开发栈真正读取的配置边界。Go 侧只有 `internal/platform/appconfig` 读取各进程声明的环境和明确指向的密码文件；HTTP、Lottery、业务 MySQL、Identity MySQL、Redis、Migration、provision/maintenance 和业务包只接收已经校验的类型化值。前端代理配置由运行 Vite 的 Node.js 进程独立读取，不经过 Go `appconfig`，也不会暴露给浏览器代码；Compose 再负责把本地文件秘密、容器网络地址、development ephemeral feature、可选 Strategy 投影缓存、`growthos_app` 两表只读权限、`growthos_identity` 三表精确权限与 `growthos_identity_provisioner` 单表 INSERT-only 权限装配起来。

## 1. 加载规则

```text
代码内公开、非秘密默认值
          ↓ 被显式覆盖
进程环境变量 GROWTHOS_*
```

- 项目自有环境变量统一使用 `GROWTHOS_` 前缀；
- 非秘密变量未设置时使用默认值；已设置但为空/全空白时失败，不静默回退；
- MySQL business API、Identity runtime 与 Migration 三个密码都没有默认值，各自必须在直接变量与对应 `_FILE` 变量中恰好选择一个；Redis 密码也没有默认值，但只在 Strategy 缓存启用时要求二选一。密码允许任意非空 bytes，最多 1024 bytes；文件来源只去除末尾 CR/LF，不裁剪其他空白；
- 枚举严格小写；duration 使用 Go 语法，例如 `500ms`、`5s`、`2m`；
- 配置错误尽量聚合，失败返回零值，只报告变量名与约束，不回显原值；
- 启动时一次加载，不支持热更新；
- `configs/growth-api.env.example` 只列公开值，不自动加载，也不给密码写占位赋值。

`appconfig.Default()`、`DefaultMigration()`、`DefaultIdentityProvisioner()` 和 `DefaultIdentityMaintenance()` 都只表示公开默认值，不是可直接启动的完整配置；各自数据库 Password 有意为空。对应入口必须调用 `Load`、`LoadMigration`、`LoadIdentityProvisioner` 或 `LoadIdentityMaintenance`。`Load` 同时验证两套 runtime credential/pool 与浏览器安全边界；`growth-api` 随后分别打开 business/Identity pool，组合 Identity Repository 与 session routes，并把两套 MySQL 纳入同一个并发 readiness 边界。两个 one-shot loader 只读取共同部署连接参数和本进程声明的 credential/budget，不会因无关 HTTP、Lottery、Redis、Argon2、CSRF 或其他身份变量而改变所有权。

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
| `GROWTHOS_COMPOSE_WEB_PORT` | `8088` | 规范十进制 1～65535，不得前导零且不得为 HTTP 默认端口 80；同时形成 Web 发布端口与 exact public origin |
| `GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID` | `local-v1` | `[A-Za-z0-9_-]{1,16}`；仅在明确轮换 active CSRF key 时同步改变，不含 key material |
| `deploy/compose/secrets/mysql_root_password` | 首次由脚本随机生成 | 挂载给 MySQL 和 `mysql-grants`；前者用于官方入口初始化，后者只经 Unix socket 精确收敛应用权限 |
| `deploy/compose/secrets/mysql_app_password` | 首次由脚本随机生成 | 挂载给 MySQL 与 API；API 通过 `GROWTHOS_MYSQL_PASSWORD_FILE` 读取 |
| `deploy/compose/secrets/mysql_migration_password` | 首次由脚本随机生成 | 挂载给 MySQL 与一次性 Migrator；MySQL health 也使用该身份，Migrator 通过 `GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE` 读取 |
| `deploy/compose/secrets/mysql_identity_password` | 首次由脚本随机生成 | 挂载给 MySQL、`mysql-grants` 与 API；API 通过 `GROWTHOS_IDENTITY_MYSQL_PASSWORD_FILE` 读取，授权作业用同一秘密证明账号凭据可用 |
| `deploy/compose/secrets/mysql_identity_provisioner_password` | 首次由脚本随机生成 | 挂载给 MySQL、`mysql-grants` 与 operations-only provisioner；provisioner 通过 `GROWTHOS_IDENTITY_PROVISIONER_MYSQL_PASSWORD_FILE` 读取 |
| `deploy/compose/secrets/redis_password` | 首次由脚本随机生成 | 只挂载给 Redis 与 API；Redis 用它建立 `growthos_api` ACL，API 通过 `GROWTHOS_REDIS_PASSWORD_FILE` 读取 |
| `deploy/compose/secrets/identity_throttle_hmac_key` | 首次由脚本生成 32 个原始随机 bytes | 只挂载给 API；通过 `GROWTHOS_IDENTITY_THROTTLE_HMAC_KEY_FILE` 为登录 throttle subject 做不可逆派生，不能与 CSRF key 复用 |
| `deploy/compose/secrets/identity_csrf_active_key` | 首次由脚本生成另一份 32 个原始随机 bytes | 只挂载给 API；通过 `GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_FILE` 签名 session-bound CSRF token，与 active key id 一起轮换 |

`make compose-up` 会先运行 Secret 生成器。八个文件完整时只校验并复用，全部不存在时一次生成。为让真实开发 volume 线性升级，生成器额外接受三个精确状态：旧四份凭据完整时新增 Identity runtime/provisioner 两份数据库密码和两份协议密钥；过渡期五份数据库/缓存凭据完整时新增 provisioner 密码和两份协议密钥；已有 Lesson 32 七件 runtime 集合时只新增 provisioner 密码。所有路径都会先校验且绝不改写旧值；其他任意 partial 集合都失败。如果 `growthos_mysql_data` 已存在而整套 Secret 全失，脚本也拒绝生成新值，避免持久化数据库与凭据静默失配。本地目录权限为 `0700`，文件为 `0444`；后者兼容 Docker Desktop 文件挂载给非 root 容器，真正的容器可见范围仍由逐服务只读挂载限制。它们是被 Git 和 Docker build context 排除的本地开发文件，不是加密 Secret Manager，也不支持热轮换。

生成器以 `.generate.lock` 原子目录串行化整套检查与发布，避免两个首次启动互相覆盖。可处理的退出会自动清锁；若断电或 `SIGKILL` 遗留 stale lock，必须先证明没有生成进程，并确认它是本 Secret 目录内空的、非 symlink 目录，才可用精确 `rmdir` 移除。锁不是允许删除或重生八份 Secret 的理由。

Compose 把 `GROWTHOS_IDENTITY_PUBLIC_ORIGIN` 绑定到同一 `GROWTHOS_COMPOSE_WEB_PORT`，默认即 `http://127.0.0.1:8088`；改变 Web 端口时不需要再维护第二个 origin。该值表达浏览器实际可见的 exact origin，不是 API 容器的 `http://api:8080`。API 是两份协议密钥的唯一消费者；MySQL、Migrator、授权作业、Redis 与 Web 均不挂载它们。现有 API `/health` 仍负责进程存活，缺少/损坏 key 会让配置加载在监听前失败，不能通过在 healthcheck 里打印或读取 key 来“补验收”。

当前本地 Nginx 拓扑没有建立可信代理 CIDR 语义：Identity guard 只读取 API socket 的 `RemoteAddr`，明确忽略 `X-Forwarded-For`。因此经 Web 代理进入的浏览器登录会把 source throttle 聚合到 Web 容器 peer IP；它能防止客户端伪造来源头，却不是生产级逐客户端来源限流证据。正式代理信任、header 清洗和客户端地址恢复必须在独立后续小节设计与验收。

Compose 另有 `growthos_mysql_socket` named volume，只传递 MySQL Unix socket，不承载数据库事实。启动顺序为 `mysql → migrate → mysql-grants → api`；Redis 独立启动，API 不把 Redis healthy 作为启动或 readiness 前提。`mysql-grants` 不读取 `GROWTHOS_*` 环境变量，不加入任何网络，只以 UID 999 从只读 root/Identity runtime/provisioner Secret 和只读 socket 执行固定 allowlist。它会先用 `CREATE USER IF NOT EXISTS` 为旧 volume 补建 `growthos_identity` 与 `growthos_identity_provisioner`，再撤销旧 direct grants 后精确重授：`growthos_app` 只读两张 Lottery 表；`growthos_identity` 读取 workforce account、仅更新其 `updated_at` 列，并可对 session/throttle 执行 SELECT/INSERT/UPDATE/DELETE；`growthos_identity_provisioner` 只可向 workforce account `INSERT`。列级 `UPDATE` 是 MySQL 8.4.11 执行 `SELECT ... FOR UPDATE` 所需的最小权限，不允许改写 login、credential、status 或 epoch。作业要求三份 `SHOW GRANTS` 与 allowlist 完全一致、两份 Identity Secret 各自能通过固定能力探针，并断言 `@@GLOBAL.mandatory_roles` 为空；否则失败关闭且 API/operations 不得继续。它不修改已存在账号的密码，不是通用 DBA 管理入口，也不能执行调用方提供的 SQL。

Redis 只加入内部 `cache` 网络，API 是唯一业务消费者。Compose Redis ACL 先 `resetkeys`、`resetchannels`、`-@all`，再只允许 `PING`、`GETRANGE`、`SET`、`DEL`，并把 key 限制在 `growthos:development:lottery:strategy:projection:v1:*`；因此它不能扫描 key、订阅 channel、修改服务配置或访问前缀外数据。开发实例设置 `48mb` 与 `allkeys-lru`，关闭持久化，`/data` 是 tmpfs：它只是可丢弃加速器，不是事实库。

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
| `GROWTHOS_LOTTERY_EPHEMERAL_SELECTION_ENABLED` | `false` | 只接受小写 `true` / `false`；staging/production 必须为 false | 仅 API |
| `GROWTHOS_LOTTERY_SELECTION_TIMEOUT` | `3s` | `> 0` 且 `<= 30s`，并满足下述跨层预算 | 仅 API |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED` | `false` | 只接受小写 `true` / `false`；启用时要求 Redis 密码并校验缓存预算 | 仅 API |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_TTL` | `5m` | `>= 1s` 且 `<= 5m`；实际写入 TTL 再减去最多 10% jitter | 仅 API |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_LOOKUP_TIMEOUT` | `75ms` | `>= 1ms` 且 `<= 1s` | 仅 API |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_WRITE_TIMEOUT` | `75ms` | `>= 1ms` 且 `<= 1s` | 仅 API |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_FILL_TIMEOUT` | `2s` | `>= 1ms` 且 `<= 30s` | 仅 API |

`LoadMigration` 有意忽略所有 HTTP、Lottery 与 Redis 变量。即使宿主环境存在非法 HTTP 地址、feature flag、selection/cache timeout 或 Redis 配置，也不能阻止独立迁移命令运行。

`GROWTHOS_LOTTERY_EPHEMERAL_SELECTION_ENABLED=true` 只表示在 development/test 注册临时 route，不是认证、授权或生产发布开关。staging/production 设置为 true 会使配置加载失败；默认 false 时 route 不注册。仓库 Compose 为学习与验收显式使用 development + true，不能复制为正式部署策略。

## 5. Lottery Strategy 缓存与 Redis 配置

Strategy 缓存是 `StrategyReader` 的可选 cache-aside 装饰器：MySQL 始终是权威来源，Redis miss、超时、协议错误、内容损坏或写失败都不能改变既有 Lottery 业务错误语义。缓存关闭时 API 不创建 Redis client，也不要求 Redis Secret；缓存启用时，Redis 不执行启动 `PING`，也不参与 `/ready`。默认 `GROWTHOS_REDIS_MIN_IDLE_CONNS=0` 时首次真实命令才建连；显式设为正数会允许 go-redis 后台预建连接，但预建失败不会由 client 构造同步返回或阻断 API 启动。Redis 宕机时请求在有界缓存预算后回源 MySQL；MySQL 宕机时只有已经存在的合法缓存投影能继续服务，cold miss 仍按原有 unavailable 语义失败。

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_REDIS_ADDRESS` | `127.0.0.1:6379` | 非空 host 的合法 `host:port`，端口 1～65535 | Redis TCP endpoint |
| `GROWTHOS_REDIS_USERNAME` | `growthos_api` | `[A-Za-z0-9][A-Za-z0-9._-]{0,63}` | 最小权限 ACL 用户 |
| `GROWTHOS_REDIS_PASSWORD` | **无，启用时二选一** | 任意非空值，最多 1024 bytes；与 `_FILE` 互斥且不回显 | 直接 Secret |
| `GROWTHOS_REDIS_PASSWORD_FILE` | **无，启用时二选一** | 非空文件路径；有界读取；错误不回显路径或内容 | 文件 Secret |
| `GROWTHOS_REDIS_DATABASE` | `0` | 只能为 `0` | runtime ACL 不授权 `SELECT`；非 0 会在配置阶段失败，避免出现“本地校验通过、连接初始化失败”的假合法配置 |
| `GROWTHOS_REDIS_TLS_MODE` | `disabled` | `disabled` / `verify_identity`；staging/production 启用缓存时必须后者 | TLS 策略 |
| `GROWTHOS_REDIS_TLS_CA_FILE` | 未设置 | 可选非空路径；仅 `verify_identity` 可设置 | 扩展系统根 CA，保持主机名验证 |
| `GROWTHOS_REDIS_DIAL_TIMEOUT` | `250ms` | `> 0` 且 `<= 5s` | 新连接预算 |
| `GROWTHOS_REDIS_READ_TIMEOUT` | `75ms` | `> 0` 且 `<= 5s` | 单连接读取预算上限 |
| `GROWTHOS_REDIS_WRITE_TIMEOUT` | `75ms` | `> 0` 且 `<= 5s` | 单连接写入预算上限 |
| `GROWTHOS_REDIS_POOL_TIMEOUT` | `100ms` | `> 0` 且 `<= 5s` | 等待连接池预算 |
| `GROWTHOS_REDIS_POOL_SIZE` | `10` | 整数 1～100 | 最大连接数 |
| `GROWTHOS_REDIS_MIN_IDLE_CONNS` | `0` | 整数 0～100，且不大于 pool size | 预留空闲连接数；默认不预热 |
| `GROWTHOS_REDIS_CONN_MAX_LIFETIME` | `15m` | `> 0` 且 `<= 24h` | 单连接最大寿命 |
| `GROWTHOS_REDIS_CONN_MAX_IDLE_TIME` | `5m` | `> 0` 且 `<= 1h` | 单连接最大空闲时间 |

跨组件缓存预算必须满足：

```text
lookup timeout + fill timeout + write timeout
  <= GROWTHOS_LOTTERY_SELECTION_TIMEOUT
```

`LookupTimeout`、`WriteTimeout` 是策略层 deadline；底层 dial/read/write/pool 设置是客户端每个阶段的硬上限，两层共同避免 Redis 把 3 秒业务预算耗尽。默认 `75ms + 2s + 75ms <= 3s`。缓存 value 是版本化 JSON v1、最多 2 MiB、最多 1000 个 Award，完整 uint64 用规范十进制 string；读取用 `GETRANGE 0 2097152` 检出超限值，坏值只精确 `DEL` 当前 key 后回源，不使用 `KEYS`、`SCAN` 或 `FLUSHALL`。不存在的 Strategy 不做 negative cache，以免当前无精确失效机制时把后续新建事实遮蔽。

## 6. MySQL 共享连接配置

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_MYSQL_ADDRESS` | `127.0.0.1:3306` | 非空 host 的合法 `host:port`，端口 1～65535 | TCP endpoint |
| `GROWTHOS_MYSQL_DATABASE` | `growthos` | `[a-z][a-z0-9_]{0,63}` | 目标 schema |
| `GROWTHOS_MYSQL_TLS_MODE` | `disabled` | `disabled` / `verify_identity` | TLS 策略 |
| `GROWTHOS_MYSQL_TLS_CA_FILE` | 未设置 | 可选非空路径；`disabled` 时禁止设置；文件须可读、为有效 PEM 且不超过 1 MiB | 为系统根证书池增加 CA |
| `GROWTHOS_MYSQL_CONNECT_TIMEOUT` | `3s` | `> 0` 且 `<= 30s` | 单次连接 timeout |
| `GROWTHOS_MYSQL_WRITE_TIMEOUT` | `5s` | `> 0` 且 `<= 5m` | 单连接网络写 timeout |

staging/production 必须使用 `verify_identity`。该模式验证证书链与 `ADDRESS` 中的主机名，TLS 最低 1.2；可选 CA 是扩展系统根，不是关闭验证。CA 路径或解析失败只报告稳定阶段，不回显路径或证书内容。development/test 允许显式 disabled。

## 7. Business API 数据库与连接池配置

`appconfig.Load` 同时要求 business API 与 Identity runtime 密码，但两者进入两个独立的类型化 `MySQLConfig`，不得拼成共享 DSN 或共享 pool。它不读取 Migrator 账号、密码或执行 timeout。

第 21 节 Compose 运行时的 `growthos_app` 只可对 `lottery_strategy` 和 `lottery_strategy_award` 执行 `SELECT`；它不能 INSERT、UPDATE、DELETE、执行 DDL 或读写 `schema_migrations`。这是由当前产品只调用 `StrategyReader.FindByID` 推导的部署层最小权限，不是 `appconfig` 可配置的授权列表。第 19 节 Repository Create 仍由可丢弃 schema 上的隔离测试 writer 验证，不要求长期运行身份保留 INSERT。未来增加写用例或新对象时必须重新审核，不能预授予 schema wildcard DML。

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

GROWTHOS_LOTTERY_SELECTION_TIMEOUT + 1s
  <= GROWTHOS_MYSQL_READ_TIMEOUT

GROWTHOS_LOTTERY_SELECTION_TIMEOUT + 1s
  <= GROWTHOS_HTTP_WRITE_TIMEOUT
```

readiness 的 1 秒为失败写入 503 envelope 的固定响应预算；selection 的两个 1 秒分别给依赖返回和 HTTP 响应留余量。Compose 当前是 selection 3s、MySQL read 5s、HTTP write 10s。selection timeout 是 cooperative deadline：Repository 会观察 context，但同步无 context 的本地 selector 不能被强制抢占，因此领域另以每个 Strategy 最多 1000 个 Award 限制不可取消区段。连接池设置在首次 Ping 前完成；首次 Ping 失败时池被关闭，API 不监听。

## 8. Identity MySQL 独立运行身份与连接池配置

Identity 与 business runtime 复用第 6 节的 endpoint、database、TLS、connect/write timeout，因为它们连接同一个受控 MySQL 部署；除此之外，账号、Secret、read/ping timeout 与 pool/lifetime 都独立加载。`Config.IdentityMySQL` 是独立连接所有权边界，production composition 据此打开、拥有并逆序关闭第二个 pool；它拒绝 pool 指针别名及两个 wrapper 共享同一个底层 `sql.DB`。`Config.MySQL` 的 `growthos_app` 不会传给 Identity Repository，也不会反向复用 `growthos_identity` 查询 Lottery/Marketing。每账户最多 5 个 active session 是领域/事务不变量，不是把数据库 pool 设置为 5。

当前 Compose 最小权限精确为：

- `identity_workforce_account`：`SELECT, UPDATE (updated_at)`；列级 `UPDATE` 只服务 MySQL 8.4.11 的账户锁读，公开 API runtime 不拥有 login/credential/status/epoch 写权限；
- `identity_session`：`SELECT, INSERT, UPDATE, DELETE`，用于签发、resolve/touch、撤销、并发上限与有界清理；
- `identity_authentication_throttle`：`SELECT, INSERT, UPDATE, DELETE`，用于登录名/source 两维 admission reservation 与结果收口；
- 明确拒绝 `schema_migrations`、Lottery、Marketing、Governance 等非 Identity 表，以及 DDL/GRANT；反向的 `growthos_app` 也被拒绝读取三张 Identity 表。

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_IDENTITY_MYSQL_USER` | `growthos_identity` | 1～32 个有效可打印 Unicode 字符；无控制字符与首尾空白 | 独立 Identity runtime 账号 |
| `GROWTHOS_IDENTITY_MYSQL_PASSWORD` | **无，二选一** | 任意非空值，最多 1024 bytes；与 `_FILE` 互斥且不回显 | Identity 直接 Secret |
| `GROWTHOS_IDENTITY_MYSQL_PASSWORD_FILE` | **无，二选一** | 非空文件路径；有界读取，只去除末尾 CR/LF；错误不回显路径或内容 | Identity 文件 Secret |
| `GROWTHOS_IDENTITY_MYSQL_READ_TIMEOUT` | `5s` | `> 0` 且 `<= 5m` | Identity 单连接网络读 timeout |
| `GROWTHOS_IDENTITY_MYSQL_PING_TIMEOUT` | `3s` | `> 0` 且 `<= 30s`，并满足下述 HTTP 响应预算 | Identity pool 启动/readiness Ping 预算 |
| `GROWTHOS_IDENTITY_MYSQL_MAX_OPEN_CONNS` | `10` | 整数 1～100 | Identity pool 最大打开连接 |
| `GROWTHOS_IDENTITY_MYSQL_MAX_IDLE_CONNS` | `10` | 整数 0～100，且不大于 Identity max open | Identity pool 最大空闲连接 |
| `GROWTHOS_IDENTITY_MYSQL_CONN_MAX_LIFETIME` | `3m` | `> 0` 且 `<= 1h` | Identity 单连接最大复用寿命 |
| `GROWTHOS_IDENTITY_MYSQL_CONN_MAX_IDLE_TIME` | `1m` | `> 0` 且 `<= 30m` | Identity 单连接最大空闲时间 |

Identity readiness 预算独立校验：

```text
GROWTHOS_IDENTITY_MYSQL_PING_TIMEOUT + 1s
  <= GROWTHOS_HTTP_WRITE_TIMEOUT
```

`cmd/growth-api` 已把 Identity Repository、密码校验、双维 throttle、session 签发/解析/撤销、Cookie/CSRF/Origin guard 与三个 session HTTP endpoint 组合进生产进程；`/ready` 并发探测 business 与 Identity 两套凭据，并以两者 timeout 的较大值作为外层预算。readiness 只证明连接可用，不证明表级 grants；授权仍由 `mysql-grants` 的 allowlist 与真实 1142/1143 拒绝矩阵独立验收。

### 8.1 Identity 浏览器安全与执行预算

Cookie mode 不接受独立开关：development/test 从 numeric-loopback HTTP origin 派生开发 Cookie，staging/production 从 HTTPS origin 派生 `Secure` 的 `__Host-` Cookie。origin 必须是规范、无 credentials/path/query/fragment 的 exact origin；默认端口显式写法、大小写非规范 host 等模糊形式会失败关闭。

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_IDENTITY_PUBLIC_ORIGIN` | **无，必填** | development/test 为 exact HTTP numeric-loopback origin；staging/production 为 exact HTTPS origin | Origin、Fetch Metadata、Cookie mode 的共同边界 |
| `GROWTHOS_IDENTITY_THROTTLE_HMAC_KEY` / `_FILE` | **无，二选一** | 恰好 32 个非全零 bytes；文件须是普通文件；与所有 CSRF key material 不同 | 私密派生登录名/source throttle subject |
| `GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_ID` | **无，必填** | `[A-Za-z0-9_-]{1,16}` | 标记当前 CSRF 签名 key revision，不是秘密 |
| `GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY` / `_FILE` | **无，二选一** | 恰好 32 个非全零 bytes；文件须是普通文件；不能复用 throttle key | session-bound CSRF token 签名 |
| `GROWTHOS_IDENTITY_ARGON2_MAX_CONCURRENT` | `2` | 整数 1～4 | 每进程同时执行 Argon2id 的硬上限 |
| `GROWTHOS_IDENTITY_ARGON2_ACQUIRE_TIMEOUT` | `250ms` | `1ms`～`1s` | 等待 Argon2 admission slot 的预算 |
| `GROWTHOS_IDENTITY_HTTP_HANDLER_TIMEOUT` | `3s` | `> 0` 且 `<= 30s`，并满足两条预算关系 | session HTTP handler 总 deadline |

```text
GROWTHOS_IDENTITY_ARGON2_ACQUIRE_TIMEOUT + 1s
  <= GROWTHOS_IDENTITY_HTTP_HANDLER_TIMEOUT

GROWTHOS_IDENTITY_HTTP_HANDLER_TIMEOUT + 1s
  <= GROWTHOS_HTTP_WRITE_TIMEOUT
```

Compose 固定 `2 / 250ms / 3s`，配合 HTTP write `10s` 留出明确余量。它没有配置 previous CSRF key；本地 active key 轮换后允许既有 CSRF token 失效。需要平滑轮换的正式环境必须把 previous key id、独立 key file 与绝对 accept-until 作为同一组受控变更，不能长期保留 previous key。

### 8.2 Identity provisioner 独立配置

`LoadIdentityProvisioner` 复用第 6 节 endpoint、database、TLS、connect/write timeout，但只读取 provisioner 自己的数据库身份与预算。它不读取 API 的 business/Identity runtime password、HTTP、Lottery、Redis、Migration、CSRF、throttle key 或 pool 参数。composition root 只打开一个最大 open/idle 均为 1 的短命连接；公开命令固定为 `growth-identity-provision create`，enrollment password 只能经调用方拥有的 `--password-file` 进入，不属于环境配置。

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_IDENTITY_PROVISIONER_MYSQL_USER` | `growthos_identity_provisioner` | 1～32 个有效可打印 Unicode 字符；无控制字符与首尾空白 | workforce account INSERT-only 身份 |
| `GROWTHOS_IDENTITY_PROVISIONER_MYSQL_PASSWORD` / `_FILE` | **无，二选一** | 非空，最多 1024 bytes；文件有界读取；两来源互斥且错误不回显 | provisioner 独立数据库 Secret |
| `GROWTHOS_IDENTITY_PROVISIONER_MYSQL_READ_TIMEOUT` | `5s` | `> 0` 且 `<= 5m` | 单连接读取预算 |
| `GROWTHOS_IDENTITY_PROVISIONER_MYSQL_PING_TIMEOUT` | `3s` | `> 0` 且 `<= 30s` | one-shot 首次 Ping 预算 |
| `GROWTHOS_IDENTITY_PROVISIONER_OPERATION_TIMEOUT` | `3s` | `1s`～`30s`，并在 read/write 内各预留 1s | 唯一一次 create 的总操作预算 |

Compose 的 `identity-provision` 只在 `operations` profile 出现，固定 UID 65532、read-only rootfs、`cap_drop: ALL`、`no-new-privileges`、无 published port，并只静态挂载 provisioner 数据库 Secret。host wrapper 在调用前把 caller-owned `0600`、单 hard-link、非 symlink regular password file 复制到私有临时快照，运行时额外挂载该快照，结束时按精确路径覆写并 unlink；这只降低普通残留风险，不能宣称在 SSD/CoW/快照介质上物理不可恢复。

### 8.3 Identity maintenance 独立配置

`LoadIdentityMaintenance` 同样复用共同 MySQL 连接策略，但有意复用 `GROWTHOS_IDENTITY_MYSQL_USER` 与其 password source，因为清理的 Session/throttle DELETE 已属于 runtime allowlist。它只额外读取下列三个专用预算，不读取 HTTP、Lottery、Redis、Argon2、CSRF/throttle key、provisioner/Migration credential 或任何 caller cutoff/batch/loop/retry 参数。composition root 固定单连接、单次 `run`、无自动重试。

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_READ_TIMEOUT` | `5s` | `> 0` 且 `<= 5m` | maintenance 单连接读取预算 |
| `GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_PING_TIMEOUT` | `3s` | `> 0` 且 `<= 30s` | one-shot 首次 Ping 预算 |
| `GROWTHOS_IDENTITY_MAINTENANCE_OPERATION_TIMEOUT` | `3s` | `1s`～`30s`，并在 read/write 内各预留 1s | 一次固定两阶段清理的总预算 |

`identity-maintenance` 也只在 `operations` profile 出现，使用固定 `growth-identity-maintenance run`、UID 65532、read-only rootfs、无端口，只挂载 runtime Identity Secret。代码在一次 clock snapshot 下计算 Session 7d 与 throttle 24h cutoff，Session/throttle 各最多删除 250 行、总计最多 500；第一阶段失败或结果未知时不进入第二阶段，第一阶段已提交而第二阶段失败时如实返回部分进度，不用“全局事务”伪装回滚。

## 9. Migration 专属配置

`appconfig.LoadMigration` 只要求 Migration 密码。它复用 environment/log、endpoint、database、TLS、connect/write timeout，但不读取 business API 或 Identity runtime 的 user/password/read/ping/pool，也不读取 HTTP 配置。

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

## 10. 无秘密示例

以下与 `configs/growth-api.env.example` 同口径，只展示公开值：

```dotenv
GROWTHOS_ENVIRONMENT=development
GROWTHOS_HTTP_ADDRESS=:8080
GROWTHOS_HTTP_SHUTDOWN_TIMEOUT=5s
GROWTHOS_HTTP_READ_HEADER_TIMEOUT=5s
GROWTHOS_HTTP_READ_TIMEOUT=15s
GROWTHOS_HTTP_WRITE_TIMEOUT=30s
GROWTHOS_HTTP_IDLE_TIMEOUT=60s
GROWTHOS_LOTTERY_EPHEMERAL_SELECTION_ENABLED=false
GROWTHOS_LOTTERY_SELECTION_TIMEOUT=3s
GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED=false
GROWTHOS_LOTTERY_STRATEGY_CACHE_TTL=5m
GROWTHOS_LOTTERY_STRATEGY_CACHE_LOOKUP_TIMEOUT=75ms
GROWTHOS_LOTTERY_STRATEGY_CACHE_WRITE_TIMEOUT=75ms
GROWTHOS_LOTTERY_STRATEGY_CACHE_FILL_TIMEOUT=2s
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

GROWTHOS_IDENTITY_MYSQL_USER=growthos_identity
GROWTHOS_IDENTITY_MYSQL_READ_TIMEOUT=5s
GROWTHOS_IDENTITY_MYSQL_PING_TIMEOUT=3s
GROWTHOS_IDENTITY_MYSQL_MAX_OPEN_CONNS=10
GROWTHOS_IDENTITY_MYSQL_MAX_IDLE_CONNS=10
GROWTHOS_IDENTITY_MYSQL_CONN_MAX_LIFETIME=3m
GROWTHOS_IDENTITY_MYSQL_CONN_MAX_IDLE_TIME=1m

GROWTHOS_IDENTITY_PUBLIC_ORIGIN=http://127.0.0.1:5173
GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_ID=local-v1
GROWTHOS_IDENTITY_ARGON2_MAX_CONCURRENT=2
GROWTHOS_IDENTITY_ARGON2_ACQUIRE_TIMEOUT=250ms
GROWTHOS_IDENTITY_HTTP_HANDLER_TIMEOUT=3s

GROWTHOS_REDIS_ADDRESS=127.0.0.1:6379
GROWTHOS_REDIS_USERNAME=growthos_api
GROWTHOS_REDIS_DATABASE=0
GROWTHOS_REDIS_TLS_MODE=disabled
GROWTHOS_REDIS_DIAL_TIMEOUT=250ms
GROWTHOS_REDIS_READ_TIMEOUT=75ms
GROWTHOS_REDIS_WRITE_TIMEOUT=75ms
GROWTHOS_REDIS_POOL_TIMEOUT=100ms
GROWTHOS_REDIS_POOL_SIZE=10
GROWTHOS_REDIS_MIN_IDLE_CONNS=0
GROWTHOS_REDIS_CONN_MAX_LIFETIME=15m
GROWTHOS_REDIS_CONN_MAX_IDLE_TIME=5m

GROWTHOS_MYSQL_MIGRATION_USER=growthos_migrator
GROWTHOS_MYSQL_MIGRATION_READ_TIMEOUT=35s
GROWTHOS_MYSQL_MIGRATION_STATEMENT_TIMEOUT=30s
GROWTHOS_MYSQL_MIGRATION_LOCK_TIMEOUT=40s
```

直接密码和 `_FILE` 路径都不出现在通用赋值示例中；操作者必须分别为 business API、Identity runtime 和 Migration 按运行方式选择一种 MySQL 密码来源，并为 Identity throttle/active CSRF 各选择一份互不相同的 32-byte Secret 来源。缓存关闭时不需要 Redis 密码；缓存启用时也必须选择一个 Redis 密码来源。Compose 已在服务级环境中指向 `/run/secrets/...`，手工运行则应由受控进程环境或本机未跟踪普通文件注入。Secret 注入完成后分别运行：

```bash
make api-run
make db-status
make db-migrate
```

不要把 DSN 作为替代变量提交，也不要在 shell 命令、截图或 QA 文档中展开密码。

## 11. 失败与秘密语义

以下情况在连接/监听前失败：

- Vite 代理目标不是无凭据、无非根路径、无 query/fragment 的 HTTP(S) origin，或 `PORT` 非法；
- 必需密码的直接来源与文件来源同时存在或同时缺失，文件路径为空、不可读，或解析后的密码为空/超过 1024 bytes；缓存关闭时 Redis 密码可缺省，但若任一 Redis 密码来源被显式提供仍必须满足互斥与内容约束；
- Identity public origin 缺失或不是当前环境允许的 canonical exact origin；throttle/active CSRF key 缺失、不是普通文件、不是 32 个非全零 bytes、来源冲突或发生 key reuse；active key id 非法；Argon2 admission/handler timeout 越界或破坏执行/HTTP 响应余量；
- Lottery flag 不是精确小写布尔值、selection/cache timeout 或 TTL 越界、缓存三段预算超过 selection timeout、破坏 MySQL read/HTTP write 余量，或 staging/production 尝试启用 ephemeral route；
- Redis 地址、ACL 用户、TLS/CA、pool、连接寿命或 timeout 非法；staging/production 启用缓存却未使用 `verify_identity`；
- MySQL 地址缺少 host/port，数据库名或 business/Identity/Migration 用户名不合法；
- TLS 枚举、环境 TLS 组合或 CA 组合不合法；
- duration/整数无法解析、越界，business/Identity 各自 idle pool 大于 open pool，或任一 Ping 预算破坏 HTTP 响应余量；
- provisioner/maintenance 专用 read、ping、operation timeout 越界或没有在共享 MySQL read/write timeout 内各保留 1 秒 cancellation/cleanup 预算；provisioner 独立 password 缺失/冲突，或任一 one-shot 意外读取其他进程变量；
- API 的 MySQL opener、TLS、connector 或首次 MySQL Ping 失败；Redis client 创建不做启动 Ping；
- Migration source/状态不安全。

配置错误只列变量名和允许范围，不含原值。`Config`、`MySQLConfig`、`RedisConfig`、`MigrationConfig`、`MigrationMySQLConfig`、`IdentityProvisionerConfig` 与 `IdentityMaintenanceConfig` 及其含密子配置，在 `String`、`GoString`、`slog.LogValuer` 和 JSON 边界返回整体脱敏文本；MySQL/Redis adapter 的含密码 Config 也遵守同一规则。

整体脱敏是最后防线，不是记录配置对象的许可。日志仍应只选择 environment、component、operation、版本号等非秘密字段。驱动错误可能包含账号、主机、SQL 或拓扑，API/Migration 入口不能直接格式化它。

## 12. 配置所有权

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
          ┌────────────┼────────────┐          │
          │            │            │          │
       logging   business MySQL  Identity MySQL│
                       │            │          │
               mysqlstore.Open  mysqlstore.Open│
                       │            │          │
                    sqlx.DB      sqlx.DB   mysqlstore.OpenMigration
                       │            │          │
            Lottery Repository  Identity Repository  dbmigration.Runner
                       │            │
          direct/cache Reader   login/resolve/revoke
                       │            │
                  Lottery HTTP   session HTTP
                       └──────┬─────┘
                              │
                    dual MySQL readiness
                              │
                         httpserver
```

两套 `sqlx.DB` 的创建顺序是 business → Identity → optional Redis，任何失败与正常停机都按 Redis → Identity → business 逆序释放。session route 注册失败也遵守同一所有权规则。双库 readiness 同时发起两个有界 `PingContext`，任一失败会取消 sibling 但仍等待其返回，不遗留未拥有的探针 goroutine。

operations-only 进程不加入上述长期对象图：

```text
LoadIdentityProvisioner -> one short-lived pool(max 1) -> INSERT-only account create -> close
LoadIdentityMaintenance -> one short-lived pool(max 1) -> fixed bounded cleanup      -> close
```

两个 host wrapper 先让 `mysql-grants` 收敛，再以最长 180 秒的 exact state 轮询等待 one-shot prerequisite；只有唯一合法 container 的 `exited:0` 算成功。`created:0`、`running:0`、`restarting:0` 继续等待，非零退出、歧义 ID、意外状态、inspect 失败或超时都失败关闭。该规则来自提交 `af4245e` 对真实 `docker compose up --wait` 误判已完成 `mysql-grants` 的修复，不能退化成只等 `healthy`。

- `growth-api` 不读取 Migration Secret，也不执行 DDL；
- `growth-migrate` 不读取 HTTP、business API 或 Identity runtime Secret/池参数；
- `growth-migrate` 也不读取 Lottery feature/timeout、Strategy 缓存或 Redis 配置；
- Compose `mysql-grants` 不读取 business API/Migration Secret，不使用 TCP 或容器网络；它只读取 root、Identity runtime 与 provisioner Secret，在 Migration 完成后经共享 Unix socket 把 `growthos_app`、`growthos_identity`、`growthos_identity_provisioner` 分别收敛到两表只读、三表 runtime 与单表 INSERT-only allowlist，并在 credential、grant 或 mandatory role 证明失败时关闭；
- `growth-identity-provision` 不读取 runtime/Migration credential，不能 readback/upsert；`growth-identity-maintenance` 只复用 runtime Identity credential 与固定预算，不读取 provisioner/enrollment password、协议密钥或 caller cleanup policy；
- Vite 不读取 Go API/Migration Secret，浏览器也不读取代理目标；
- `mysqlstore` 不读取环境变量，只接收类型化配置；
- `redisstore` 不读取环境变量，只接收类型化、整体脱敏配置；client 创建不执行 `PING`，连接池由 API 在缓存启用时拥有并在停机时显式关闭；
- `strategycache` 只依赖最小 `GETRANGE` / `SET` / `DEL` Store 与权威 `StrategyReader`，不解析 Redis 地址、密码或 TLS；
- `dbmigration` 不拥有凭据，只接收已经打开且所有权明确的专用连接；
- `/ready` 只接收最小 `PingContext` 接口，不解析数据库配置。

## 13. 隔离 MySQL 集成测试变量

`make test-integration-mysql` 的 `GROWTHOS_TEST_MYSQL_*` 只属于测试 harness，不由 `appconfig` 读取，也不能用于产品启动。第 19 节除 API/Migration 两套地址、库名、账号和密码外，还要求两个精确开关：

```text
GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES=lesson-19-isolated-schema
GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES=lesson-19-isolated-repository
```

它们不是安全授权令牌，而是阻止操作者误把 destructive schema/fixture 测试指向普通开发库的第二道显式确认。测试仍会校验 API/Migrator 指向同一 schema、身份不同，并以事务或精确 ID/约束名清理。凭据只通过当前进程环境注入，不写入 QA、shell history 示例或仓库文件。

## 14. 变更规则

新增或修改配置必须同步：

1. 类型化结构、默认值、校验、聚合错误和不泄漏测试；
2. `configs/growth-api.env.example`（Secret 仍不得赋值）；
3. 本参考、课程正文和相关 runbook；
4. 部署资产、真实集成变量和 QA 冒烟；
5. 跨组件 timeout/容量关系；
6. 若改变安全、兼容或长期运维约束，新增或替代 ADR。

当前没有配置热更新、远程配置中心、加密 Secret Manager 或秘密热轮换。第 16 节的本地生成器只解决可复现开发装配；授权作业当前只解决 `growthos_app` 两表只读、`growthos_identity` 三表精确 runtime 能力与 `growthos_identity_provisioner` 单表 INSERT-only 的 Compose 收敛，不是通用 IAM/DBA 平台。第 24 节 Redis 配置只解决一个可重建 Strategy 投影的有界加速器，不提供通用缓存平台。第 76 节若加入 Nacos，它也只能成为新的输入适配器，不能绕过本页类型、校验、账号隔离和秘密边界。

## 15. 第 32 节当前证据边界

- 两个 disposable provision Compose 环境已经通过并完成精确清理；official maintenance fixture 已通过 `2/1/3` 后 `0/0/0` 收敛、active Session fingerprint 不变和零 fixture 残留；
- development loopback 的真实浏览器已完成 Nginx → Go → MySQL login、reload/current、logout、MySQL outage unknown/unavailable 与恢复重核；该证据没有直接读取 HttpOnly Cookie/storage，也不证明 Set-Cookie wire 属性；
- Argon2id 当前开发机 baseline 为 serial `26.638354ms/op`、parallel capacity=2 `14.179475ms/op`，单/双 profile 19/38 MiB；它不构成 production 容量结论；
- 原始 Session HTTP wire、独立 MySQL 最终矩阵、staging/production TLS/可信代理和全仓冻结门禁仍为 `PENDING`；第 33～35 节权限执行与投影不属于本配置切片。

第 24 节的配置决策与真实证据见[课程](course/part-03/lesson-24-redis-strategy-cache.md)、[API](api/lessons/lesson-24.md)、[QA](qa/lessons/lesson-24.md)、[设计手记](design-thinking/lessons/lesson-24.md)、[面试问答](interview/lessons/lesson-24.md)、[Redis 运维手册](runbooks/redis-strategy-cache.md)和 [ADR-0020](decisions/ADR-0020-lottery-strategy-cache-aside.md)。
