# 第 16 节：Docker Compose 开发环境

**状态：** 已完成并验收

**日期：** 2026-08-29

**阶段：** Go + React 从零搭建

**本节把第 11～15 节已经存在的 Go API、独立 Migration、React 静态产物、MySQL 与一个尚未接入业务的 Redis 组织成可重复启动的本地 Compose 拓扑；它不是生产部署模板，也没有新增任何业务 API。**

## 1. 这一节真正要解决什么

到第 15 节为止，浏览器已经能通过 Vite 的开发代理访问 Go，但操作者仍要分别准备 MySQL、注入两套数据库密码、启动 Migration、启动 API、启动 Vite，并自行判断顺序是否正确。每个进程单独可运行，不等于整个开发环境可重复交付。

本节的目标不是简单写一份 `compose.yaml`，而是把这些隐含约定变成机器可检查的系统边界：

1. 哪些进程属于独立生命周期；
2. 谁可以连接谁，谁可以被宿主机访问；
3. 首次启动时账号、数据目录和 Migration 按什么顺序形成；
4. liveness、readiness 与容器 health 分别证明什么；
5. Secret 如何进入进程而不出现在镜像、Compose 环境变量或日志；
6. API、MySQL、Redis 任一故障时，其他组件应保持什么状态；
7. “本机能访问一次”怎样升级为固定的冒烟与 M0 负载门禁；
8. 如何停止这套环境而不误删用户 Docker Desktop 中已有的 MySQL、Redis、RabbitMQ 或 PostgreSQL 数据。

最终得到的是一个学习用的最小部署单元：结构足够接近真实系统，边界又足够明确，不把尚未发生的业务复杂度伪装成已经完成。

## 2. 本节范围与非目标

### 2.1 交付范围

- Compose 中定义 `web`、`api`、`migrate`、`mysql`、`redis` 五个服务；
- 使用 `edge`、`data`、`cache` 三张用户自定义网络表达最小通信关系；
- 只把 Nginx `web` 的 8080 映射到宿主机 loopback，默认地址为 `127.0.0.1:8088`；
- 使用多阶段 Dockerfile 构建 Go 的两个二进制和 React 静态产物；
- 由 Nginx 提供 SPA 静态资源，并同源代理 `/health`、`/ready` 与 `/api`；
- 使用 Docker 内置 DNS 动态解析 `api`，允许 API 容器重建后无需重启 Nginx；
- 以 MySQL 受限 API 账号执行真实 `SELECT 1` healthcheck；
- 让 `migrate` 成为必须成功退出的一次性服务，再启动 `api`；
- 保持 API `/health` 为 liveness、`/ready` 为 MySQL readiness；
- 为 MySQL root、API、Migrator 与 Redis 生成一套完整的本地文件 Secret；
- Go API 与 Migration 同时支持互斥的直接密码变量和 `_FILE` 密码变量；
- MySQL 首次初始化创建两个 schema 级最小权限账号；
- Redis 使用独立密码和 ACL，只进入隔离 `cache` 网络，当前不接入 API、不持久化；
- 应用容器使用非 root 用户、只读根文件系统、移除 Linux capabilities、`no-new-privileges` 和有界 tmpfs；
- Nginx access/error log 不记录请求 query string 与 Referer；网关隐藏 upstream 同名响应头后统一回写 API request ID 或 Nginx request ID，让 502 也可关联；
- 禁用 MySQL driver 自带的原始 stderr logger，继续使用 GrowthOS 的安全结构化日志边界；
- 提供 Compose 生命周期、冒烟、状态、Migration 与固定 M0 负载门禁的 Make 入口。

### 2.2 明确不做

- 不新增活动、策略、奖品、积分、Feed、Agent 或 MCP 业务接口；
- 不创建业务表；当前空 Migration 集成功退出仍是正确结果；
- 不让 API 使用 Redis，也不把 Redis 加入 API readiness；
- 不把 Redis 数据当作可恢复状态；本节明确关闭 RDB 与 AOF；
- 不复用或修改用户 Docker Desktop 中已经存在的 MySQL、Redis、RabbitMQ、PostgreSQL 容器；
- 不发布 MySQL、Redis 或 API 的宿主机端口；
- 不启用宽泛 CORS；浏览器仍只访问同源路径；
- 不提供 TLS、公网入口、认证网关、容器编排集群、自动扩缩容、备份或灾难恢复；
- 不把本机 M0 结果解释为生产容量、SLA 或横向扩展结论；
- 不通过自动重启隐藏启动失败；本节所有服务的 restart policy 都是 `no`。

## 3. 五个服务不是五个“技术名词”

| 服务 | 进程/镜像职责 | 正常生命周期 | 宿主机端口 | 数据 |
| --- | --- | --- | --- | --- |
| `web` | Nginx 提供 React 静态资源并反向代理 Go | 常驻 | `127.0.0.1:8088 -> 8080` | 无持久数据 |
| `api` | `growth-api`，处理 liveness、readiness 和未来业务 API | 常驻 | 不发布，仅在 `edge` 暴露 8080 | 通过受限账号访问 MySQL |
| `migrate` | `growth-migrate up` | 一次性，成功后退出 0 | 不发布 | 通过 Migrator 账号访问 MySQL |
| `mysql` | MySQL 8.4.11 | 常驻 | 不发布 | `mysql_data` named volume |
| `redis` | Redis 7.4.11 + 本地 ACL 配置 | 常驻但当前独立 | 不发布 | `/data` 为 tmpfs，不持久 |

拆分的判断依据是生命周期和权限，而不是“每种技术一个容器”：Migration 必须在流量进程前完成并退出；API 必须长期运行但不能持有 DDL 权限；Nginx 即使 API 离线也应继续提供页面；Redis 当前只是环境能力，不应凭技术清单强行耦合到 Go。

## 4. 三张网络表达可达关系

```text
host browser
    │
    │ 127.0.0.1:8088（唯一 published port）
    ▼
 web ─────────────── edge ─────────────── api
                                             │
                                             │ data（internal）
                                             ▼
                    migrate ── data ── mysql

 redis ───────────── cache（internal；当前无其他成员）
```

服务与网络的实际成员关系：

| 网络 | 成员 | 用途 |
| --- | --- | --- |
| `edge` | `web`、`api` | 同源反向代理访问 Go |
| `data` | `api`、`migrate`、`mysql` | 数据库访问；标记为 `internal` |
| `cache` | `redis` | 预留缓存边界；标记为 `internal`，当前故意没有调用方 |

这不是防火墙或零信任体系，但比所有服务共享默认网络更诚实：`web` 没有理由直连 MySQL，`migrate` 没有理由进入 edge，Redis 在业务语义出现前也没有理由出现在 API 的网络命名空间。

`expose: 8080` 只描述容器网络内的 API 端口，不等于发布到宿主机。真正对宿主机开放的是 `ports`，本节只在 `web` 使用，而且显式绑定 `127.0.0.1`。因此不会抢占用户已有 MySQL 的 3306，也不会让局域网默认访问本地开发环境。

## 5. 构建镜像：编译工具不等于运行依赖

### 5.1 Go：一个构建阶段，两个运行目标

`deploy/docker/Dockerfile.backend` 的 builder 使用固定 Go/Alpine 基线，先复制 `go.mod`、`go.sum`，执行 `go mod download && go mod verify`，再复制源码并构建：

```text
builder
  ├─ /out/growth-api
  └─ /out/growth-migrate
          │
          ├─ target api     -> Alpine runtime + growth-api
          └─ target migrate -> Alpine runtime + growth-migrate
```

构建设置 `CGO_ENABLED=0`、目标 OS/架构、`-trimpath`、去除符号，并注入 `lesson-16` 版本标签。最终运行层只保留二进制、CA 根证书和时区数据，不携带 Go 编译器、源码或模块缓存；两个 target 复用 builder 与统一的 UID/GID `65532`，同时保持不同入口命令。

### 5.2 Web：Node 只负责构建，Nginx 才是运行时

`deploy/docker/Dockerfile.web` 在 Node 阶段固定 pnpm 版本，使用 `pnpm install --frozen-lockfile` 后执行生产构建。最终层只把 `/src/dist` 复制到 Nginx：

```text
Node + pnpm + TypeScript + Vite
              │ pnpm run build
              ▼
        immutable static assets
              │ COPY --from=builder
              ▼
       Nginx non-root runtime
```

Vite dev server 适合 HMR，本节 Compose 入口要验证的是接近部署的静态资源与反向代理语义，因此不在运行容器中启动 `vite dev` 或 `vite preview`。源代码修改不会自动热更新；需要 HMR 时仍使用第 15 节宿主机开发流程，需要验证 Compose 产物时重新 build/up。

### 5.3 Redis：只生成运行期配置

Redis image 以固定官方版本为基础，入口脚本从 `/run/secrets/redis_password` 读取 64 位小写十六进制 Secret，在只读根文件系统下把 ACL 和配置写入 `/tmp/growthos-redis`。当前显式设置：

```text
save ""
appendonly no
```

`/data` 是 64 MiB tmpfs。因此容器重建后 Redis 数据消失是设计事实，不是待修复 bug。只有当真实业务定义缓存键、失效策略、丢失后果与恢复目标后，才有依据决定是否持久化或接入 API。

## 6. Nginx：同一个浏览器 origin，两类独立健康事实

### 6.1 静态资源和代理路径

浏览器仍访问同一个 origin：

| 路径 | Nginx 行为 |
| --- | --- |
| `/assets/...` | 只读取构建产物，带 immutable 长缓存 |
| `/index.html` | 返回 SPA shell，`no-store` |
| 其他页面路径 | `try_files` 回退到 `/index.html` |
| `/health`、`/ready` | 原路径代理到 `http://api:8080` |
| `/api`、`/api/...` | 原路径代理到 `http://api:8080` |
| `/container-health` | Nginx 自身返回 204，不访问 API |

这里没有 path rewrite，也没有 CORS。React 的 `fetch("/health")` 在 Vite 与 Compose 中保持同一个调用契约，只是本地代理的实现者从 Vite 变为 Nginx。

### 6.2 为什么需要动态 Docker DNS

如果 Nginx 在启动时把 `api` 只解析一次并固定为旧容器 IP，API 被 recreate 后，Nginx 可能继续访问失效地址。配置使用 Docker 内置 resolver `127.0.0.11`，把 upstream 放在变量中，并设置短期 DNS 有效期。服务名仍然稳定，地址可以随容器替换重新解析。

这一选择还有一个故障隔离价值：API 不存在时，Nginx 仍能启动并提供 SPA，而代理请求返回 gateway failure。页面可以如实显示“代理无法取得 API 响应”，而不是整个前端容器因为启动期域名解析失败退出。

### 6.3 为什么 Web health 不访问 API

`web` 的 Compose healthcheck 只请求 `/container-health`。它证明 Nginx 正在监听并能生成响应，不证明 Go 或 MySQL 健康。API 离线时：

- `web` 仍应 healthy；
- `/` 和静态资源仍可访问；
- `/health`、`/ready` 通过代理返回 gateway failure；
- 系统状态页仍能解释离线状态。

如果 Web health 代理到 `/health`，API 故障会同时把 Web 标为 unhealthy，失去故障定位信息，也可能让平台不必要地重启正确工作的静态服务。

## 7. 启动图：顺序只解决启动，不解决运行期恢复

```text
mysql container starts
        │
        │ authenticated SELECT 1 as growthos_app
        ▼
mysql becomes healthy
        │
        ▼
migrate runs growth-migrate up
        │ exit 0
        ▼
api opens pool, performs startup Ping, starts HTTP
        │
        │ GET /health returns 200
        ▼
api becomes healthy

web and redis may start independently
```

`depends_on` 使用两种条件：

- `migrate` 等待 `mysql: service_healthy`；
- `api` 等待 `mysql: service_healthy` 和 `migrate: service_completed_successfully`。

`web` 不依赖 `api`，`redis` 也不被任何业务服务依赖。Compose 条件只控制本次创建顺序，不会在运行期持续编排恢复，也不会因为 MySQL 后来重启而自动重启 API。本节故意不配置 dependency `restart: true`：Go 的 `database/sql` 连接池应该在数据库恢复后建立新连接，故障演练必须验证 `/ready` 能在不重启 API 的情况下恢复。

所有服务的 restart policy 为 `no`。开发阶段让错误稳定可见，比自动重启形成 crash loop、滚动日志和不确定现场更适合学习与诊断。

## 8. liveness、readiness 和 Compose health 不能混为一个词

| 检查 | 实际动作 | 成功能证明 | 失败不一定证明 |
| --- | --- | --- | --- |
| MySQL health | 以 `growthos_app` 连接目标 schema 并执行 `SELECT 1` | MySQL 已接受真实应用身份和最小查询 | Migration 已执行、业务表正确、性能达标 |
| API health | 容器内 `GET /health` | Go HTTP 进程可响应 | MySQL 当前可用 |
| API readiness | 经 Nginx/直接 `GET /ready`，有界 MySQL Ping | API 当前能连接 MySQL | 业务查询、数据一致性、Redis 或整个集群正常 |
| Web health | Nginx 本地 `/container-health` | 静态入口进程可响应 | API、MySQL 正常 |
| Redis health | 带密码 `redis-cli ping` | Redis 接受受保护连接 | API 已使用缓存、缓存数据可恢复 |
| Migrate 完成 | 一次性命令退出 0 | 当前内嵌 Migration 集成功处理 | 未来所有 Migration 都安全、业务 schema 已存在 |

API 容器 health 必须用 `/health`，不能用 `/ready`。数据库运行中断时 `/health=200`、`/ready=503` 才是有价值的故障组合：进程不应被当作死掉，流量也不应继续被错误接收。

MySQL healthcheck 不使用只检查 daemon 响应的弱探测，而是以受限 API 账号执行真实 SQL。这同时验证数据库启动、账号初始化、密码集合、目标 schema 和最小查询权限。

## 9. 账号：把“代码不会做 DDL”升级成数据库拒绝 DDL

MySQL 官方镜像只在全新数据目录初始化时执行 `deploy/compose/mysql/init/10-create-growthos-users.sh`。脚本读取文件 Secret，创建：

| 身份 | schema 权限 | 使用方 |
| --- | --- | --- |
| `growthos_app` | `SELECT, INSERT, UPDATE, DELETE` | MySQL health、`growth-api` |
| `growthos_migrator` | 上述 DML + `CREATE, ALTER, DROP, INDEX, REFERENCES` | `growth-migrate` |

root 密码只供 MySQL 官方入口完成首次初始化，不进入 API 或 Migration。Migrator 也不进入 API。账号 host 在本地镜像中使用 `%` 以适配容器地址变化，但服务只连接 `data` internal network；这不是生产账号来源策略，生产必须按真实网络和身份系统进一步收紧。

因为初始化脚本只在空 volume 执行，修改 Secret 文件不会自动轮换已存在账号的密码。这个事实直接影响后面的 Secret 生成器和 reset 操作。

## 10. `_FILE`：文件注入不是字符串拼接技巧

API 和 Migration 分别接受：

```text
GROWTHOS_MYSQL_PASSWORD
GROWTHOS_MYSQL_PASSWORD_FILE

GROWTHOS_MYSQL_MIGRATION_PASSWORD
GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE
```

每个进程对自己的密码要求“恰好一个”：直接值和 `_FILE` 同时存在会失败，两个都不存在也会失败。文件读取有大小上限，最多接收 1024 bytes 的密码，只移除文件结尾的 CR/LF，不会把密码首尾空格悄悄 trim 掉。失败只报告稳定变量名和约束，不打印文件路径、内容或密码。

Compose 选择 `_FILE`，把 Secret 以 `/run/secrets/<name>` 文件形式按服务授权：

| Secret | 可以读取的服务 |
| --- | --- |
| `mysql_root_password` | `mysql` |
| `mysql_app_password` | `mysql`、`api` |
| `mysql_migration_password` | `mysql`、`migrate` |
| `redis_password` | `redis` |

Compose 本地 file secret 不是加密 Secret manager。它的价值是避免密码出现在镜像层、Compose environment 和普通进程环境，并限制每个服务的挂载集合；宿主机文件本身仍需保护。

## 11. 为什么 Secret 必须作为一个集合生成

`scripts/generate-compose-secrets.sh` 维护四个 64 位小写十六进制随机值，并执行以下 fail-closed 规则：

1. 目录不存在时创建，宿主机目录设为 `0700`；
2. 四个文件都不存在时，先在私有临时目录完整生成并验证，再逐文件发布；四次移动不是单一文件系统事务，发布中断形成部分集合时下次运行会 fail closed；
3. 四个文件都存在时只验证，不覆盖；
4. 只存在 1～3 个文件时拒绝补齐，避免身份集合来自不同批次；
5. 文件不是可读普通文件、格式不是 64 位小写十六进制时拒绝；
6. 默认 MySQL volume 已存在、整套 Secret 却缺失时拒绝生成，因为新密码不会匹配 volume 内的账号；
7. Secret 文件设为 `0444`，用于兼容 Docker Desktop 把 file secret 以 root-owned 只读文件挂入非 root 容器；真正的宿主机隔离仍依赖上层 `0700` 目录；
8. Secret 目录被 `.gitignore` 和 `.dockerignore` 排除，不能进入提交和构建上下文。

这里 `0444` 不是说 Secret 可以随意共享。没有目录遍历权限的其他宿主机用户无法到达文件，Compose 又只向声明了该 Secret 的服务挂载它。若本机权限模型或共享目录策略不同，应重新评估，而不是盲目复制权限数值。

## 12. 运行时加固与它没有解决的问题

`api`、`migrate`、`web`、`redis` 当前共同采用：

- 固定非 root UID/GID；
- `read_only: true`；
- `cap_drop: [ALL]`；
- `security_opt: no-new-privileges:true`；
- `init: true` 处理信号和僵尸子进程；
- 有界 `/tmp` tmpfs；
- `restart: "no"`；
- JSON file log rotation，单文件 10 MiB、最多 3 个。

Redis 额外使用 64 MiB `/data` tmpfs。MySQL 是明确例外：官方初始化流程和 `/var/lib/mysql` 持久写入需要可写文件系统与 named volume，本节没有假装所有镜像都能套同一个 hardened 模板。

这些配置降低误写和容器逃逸面，但不等于沙箱、机密计算或生产合规：容器仍共享宿主机内核，镜像仍需漏洞扫描，Docker daemon 权限仍然高，网络策略也只是本地 bridge 隔离。

## 13. 日志：关联请求，但不把排障变成数据泄漏

Nginx access log 只记录：远端地址、关联 request ID、时间、method、规范化 `$uri`、协议、status、响应 bytes、总耗时和 upstream 耗时。它故意不记录 `$request_uri` 的 query string，也不记录 Referer，因为 query、授权回调、过滤条件和页面来源可能包含 token 或个人信息。

关联 ID 的选择是：

```text
API response X-Request-ID 存在 -> 使用 API request ID
否则                         -> 使用 Nginx 自己的 request ID
```

Nginx 会隐藏 upstream 原始的同名响应头，再把上述最终 ID 统一写为一个 `X-Request-ID`。因此 Go 正常/错误响应仍使用 Go ID，API-down 的 502 也会获得 Nginx ID，并与安全 access log 相同；不会因为 upstream 和 gateway 各写一次而产生重复/冲突 header。这让同一条代理请求可以从 Nginx access log 关联到 Go 结构化访问日志，或在没有 upstream 时关联到网关失败。它仍不是分布式 trace，也不应该被当作不可伪造身份。

Nginx 内置 error log 的请求级报错可能附带原始 request target 和 Referer，而格式不可像 access log 一样定制。当前只把 error log 保留在 `crit`，可诊断的 upstream status/耗时进入脱敏 access log。验收向 query 和 Referer 注入唯一 marker 后检索全部 Compose 日志，marker 均未出现；这证明当前演练路径没有泄漏，不替代后续业务字段的持续日志审计。

Go MySQL driver 默认可能把内部连接错误直接写入 stderr，绕过 GrowthOS 的 JSON、安全 stage 与脱敏边界。本节在每个 driver Config 上使用 `NopLogger`，让 readiness 和生命周期日志只暴露稳定操作事实。代价是通用容器日志不再包含 driver 原始细节；深度数据库诊断应使用受控 DBA 工具，而不是把 DSN、拓扑或 cause 放宽到应用日志。

## 14. 停止预算要覆盖进程内部预算

| 服务 | Compose `stop_grace_period` | 依据 |
| --- | ---: | --- |
| `web` | 10s | Nginx 使用 `SIGQUIT` 优雅退出 |
| `api` | 10s | Go HTTP shutdown 默认 5s，给信号传播和池关闭留余量 |
| `migrate` | 50s | 覆盖默认 40s 锁获取预算并留清理余量 |
| `mysql` | 30s | 给数据库刷盘与正常停止留时间 |
| `redis` | 30s | 给 Redis 正常停止留时间，虽当前无持久数据 |

grace period 不是“操作一定能在该时间完成”的保证。到期后 Docker 可以发送 SIGKILL；尤其 Migration 执行 DDL 时，停止前仍应先读状态和日志，不能把容器停止当作数据库事务回滚。

## 15. Make 入口按意图分层

| 命令 | 意图 |
| --- | --- |
| `make compose-secrets` | 创建或验证完整 Secret 集合 |
| `make compose-config` | 先验证 Secret，再让 Compose 校验模型 |
| `make compose-build` | 构建 API、Migration、Web、Redis 四个本地 image target |
| `make compose-up` | build、后台启动并等待健康，最多 180s |
| `make compose-down` | 停止并移除本项目容器/网络，保留 MySQL volume |
| `make compose-reset CONFIRM=reset-growthos-data` | 显式删除本项目 named volume；破坏性操作 |
| `make compose-ps` | 查看服务、一次性退出状态和 health |
| `make compose-logs` | 查看最后 200 行本项目日志 |
| `make compose-migrate` | 以临时容器运行 `growth-migrate up` |
| `make compose-status` | 以临时容器运行 `growth-migrate status` |
| `make compose-smoke` | 检查服务状态、HTTP 契约和唯一 loopback 端口 |
| `make compose-load-health` | 可调参数的 `/health` 局部负载工具 |
| `make compose-load-ready` | 可调参数的 `/ready` 局部负载工具 |
| `make compose-verify` | 代码质量门禁 + Compose 启动 + smoke，不运行 5 分钟 M0 |
| `make compose-m0` | 固定的本节完整冒烟与负载门禁 |

日常调试可以用可调 load target 缩短时间；正式 M0 刻意在 recipe 中固定参数，避免 `HEALTHLOAD_DURATION=1s make compose-m0` 仍被误记成完成 5 分钟验收。

## 16. M0 固定门禁如何解释

`cmd/healthload` 是一个有界、本仓库可测试的 HTTP 负载探针。它按固定速率调度请求、限制 worker 和连接、为每次请求设置 timeout、只读取最多 4 KiB body，并输出单行 JSON 汇总。以下任一情况都非零退出：

- transport/read/close error 非零；
- 出现非预期 HTTP status；
- completed 不等于 scheduled；
- worker 队列出现 dropped request；
- 配置了 P99 上限且实际 P99 超过上限。

`make compose-m0` 的不可覆盖门禁是：

```text
/health: 100 requests/s × 5 minutes = 30,000 scheduled requests
         32 workers, per-request timeout 2s, P99 <= 100ms

/ready:   20 requests/s × 30 seconds = 600 scheduled requests
          32 workers, per-request timeout 2s
```

先运行 Compose up 和 smoke，再运行这两段负载。`/health` 不访问数据库，适合观察 HTTP/Nginx/API 基线；`/ready` 每次 Ping MySQL，故采用更低且更短的压力，避免把探针本身变成不必要的数据库负载。

最终固定门禁结果：

| 目标 | scheduled / completed / success | errors / unexpected / dropped | 延迟与速率 |
| --- | --- | --- | --- |
| `/health` | `30000 / 30000 / 30000` | `0 / 0 / 0` | P50 `1.084208ms`、P95 `2.744875ms`、P99 `4.1495ms`、max `18.116291ms`、实际 `100.0027 RPS`；100ms P99 门槛通过 |
| `/ready` | `600 / 600 / 600` | `0 / 0 / 0` | P50 `4.08525ms`、P95 `5.935083ms`、P99 `6.841375ms`、max `8.570541ms`、实际 `20.0276841 RPS` |

32 workers readiness 复测与最终 smoke 后，Docker Desktop 瞬时快照约为：Web `5.535 MiB`、API `6.664 MiB`、MySQL `438 MiB`、Redis `23.41 MiB`，Docker 配额 `1.924 GiB`；一次性 Migration 已退出，不在快照中。allocator/cache 会让相邻取样变化，这不是峰值、资源 limit、内存泄漏证明或生产 sizing。原始命令、环境和完整输出见[第 16 节 QA](../../qa/lessons/lesson-16.md)。

M0 只回答“当前开发机、当前镜像、当前单实例拓扑是否越过这个学习门槛”。它没有并发业务 SQL、真实 payload、认证、数据量、网络抖动或多实例，不能用于生产容量规划。

## 17. 按代码阅读的学习顺序

建议不要从 200 行 Compose 文件逐字背诵，而是沿因果关系阅读：

### 第一步：先看外部入口

打开 `deploy/compose/compose.yaml`，只寻找 `ports`。确认只有 `web` 一处，并理解 `127.0.0.1` 和 `0.0.0.0` 的暴露差异；再查看三个 network 的成员。

### 第二步：沿浏览器请求向内走

打开 `deploy/docker/nginx.conf`：

1. `/` 如何回退到 SPA；
2. `/health`、`/ready`、`/api` 如何保持原路径；
3. `resolver 127.0.0.11` 和变量 upstream 为什么成对出现；
4. `/container-health` 为什么不代理；
5. access log 为什么使用 `$uri` 而不是 `$request_uri`。

### 第三步：区分进程与依赖健康

回到 Compose，对照 MySQL health、API health、Redis health、Web health 和 migrate exit condition。逐项说出“成功证明什么”和“不能证明什么”。

### 第四步：看权限和 Secret 的形成

依次阅读：

1. `scripts/generate-compose-secrets.sh`；
2. `deploy/compose/secrets/README.md`；
3. `deploy/compose/mysql/init/10-create-growthos-users.sh`；
4. `internal/platform/appconfig/config.go` 中的 `_FILE` 读取；
5. Compose 各服务的 `secrets` 列表。

重点不是密码格式，而是“首次 volume 中的账号状态”和“宿主机 Secret 集合”必须一致。

### 第五步：看镜像边界

分别阅读三个 Dockerfile，回答 builder 有什么、runtime 有什么、最终容器以谁运行、哪里可写。再对照 `.dockerignore` 确认 Secret、Git、IDE、前端依赖和产物没有进入 context。

### 第六步：看验证入口

阅读 `scripts/compose-smoke.sh` 与 `cmd/healthload`，区分一次契约冒烟和持续速率门禁。最后用本节 Runbook 执行正常启动与故障演练。

## 18. 实操步骤

安全前置与排障细节见[本地 Compose 运维手册](../../runbooks/local-compose.md)。正常路径为：

```bash
make compose-config
make compose-up
make compose-ps
make compose-smoke
```

访问：

```text
http://127.0.0.1:8088/system/status
```

检查 Migration：

```bash
make compose-status
make compose-migrate
```

当前没有真实 `.up.sql`，所以空集合的安全状态仍应是 `no_migrations`，不能为了“看见 applied”添加空版本。

完成普通代码和容器验证：

```bash
make compose-verify
```

只有准备进行完整固定门禁时才运行：

```bash
make compose-m0
```

正常停止并保留数据库：

```bash
make compose-down
```

## 19. 必做故障演练

### 19.1 MySQL 停止与恢复

预期：

```text
MySQL stop
  ├─ API container health remains healthy through /health
  ├─ browser /health remains 200
  └─ browser /ready becomes safe 503 dependency_unavailable

MySQL start and becomes healthy
  └─ /ready recovers without recreating/restarting API
```

如果 API 被自动重启，说明 liveness/readiness 或 restart 策略发生了错误耦合；如果 MySQL 恢复后必须人工重启 API，说明连接池恢复能力需要调查。

### 19.2 API 停止与重建

预期：Web 仍 healthy，SPA 仍可打开，代理端点出现 gateway failure；重建 API 后，Nginx 通过 Docker DNS 重新解析新地址，不需要重启 Web。

### 19.3 Redis 停止

预期：API、MySQL readiness、Web 与 Migrate 状态都不受影响。若 `/ready` 因 Redis 停止失败，说明代码或网络已经越过了“尚未业务接入”的真实边界。

### 19.4 Migration 失败

在受控测试条件下使一次性 Migration 非零退出，预期 API 不启动。不要为演练修改共享 Migration 历史或在用户数据库执行破坏性 SQL。

### 19.5 Secret 集合不完整

在任务专用临时目录测试生成脚本，预期只存在部分文件时失败；不要删除实际 Secret 或已有 MySQL volume 来制造现场。真实环境缺 Secret 时应恢复原集合，不能直接生成新密码假装修复。

完整命令、浏览器正常/降级/离线状态、端口与清理证据由[第 16 节 QA](../../qa/lessons/lesson-16.md)记录。

## 20. 常见错误及其根因

| 错法 | 为什么危险/误导 | 正确边界 |
| --- | --- | --- |
| 给 MySQL、Redis、API 都写 `ports` | 与用户服务冲突并扩大攻击面 | 只发布 loopback Web |
| 容器内用 `localhost` 访问另一个服务 | `localhost` 指向当前容器自己 | 使用同网络 service name |
| 只写短语法 `depends_on` | 只保证创建顺序，不等待可用 | MySQL `service_healthy`、Migration `service_completed_successfully` |
| 用 `/ready` 做 API liveness | DB 抖动会把健康进程判死 | API health 使用 `/health` |
| Web health 代理 API | API 故障连带掩盖静态入口 | 独立 `/container-health` |
| 在 API 启动时自动迁移 | DDL 与流量生命周期耦合，多实例竞争 | one-shot `migrate` |
| API 和 Migrator 共用 root | 代码 bug 可直接变成 schema 破坏 | schema 级双账号最小权限 |
| 把密码写入 environment 或 image | 易被 inspect、日志、layer 暴露 | Compose file secret + `_FILE` |
| Secret 丢失后直接重新生成 | 新密码与已有 volume 账号不一致 | volume guard，恢复原集合或显式数据重置 |
| 把所有服务放一个网络 | 无关组件获得不必要可达性 | edge/data/cache 三网络 |
| 因为安装了 Redis 就立即接入 | 没有键、TTL、一致性和失效需求 | 先隔离运行，等真实用例 |
| Redis 开持久化“更完整” | 当前没有恢复语义，只增加陈旧数据和运维承诺 | RDB/AOF 关闭，tmpfs 明示可丢 |
| Nginx 写死启动时 API IP | recreate 后 upstream 可能失效 | Docker DNS 动态解析 service name |
| Nginx 记录完整 URI/Referer | query 和来源可能包含敏感信息 | 规范化 path + 关联 ID |
| `restart: always` 掩盖错误 | crash loop 破坏现场且增加不确定性 | 开发阶段 `restart: no` |
| 把本地 M0 当压测结论 | 缺少业务流量、数据和生产环境变量 | 只作为章节回归门槛 |
| 用 `docker system prune` 清理 | 可能删除用户其他项目资源 | 只操作明确 Compose project |

## 21. 自己动手的练习

### 练习 A：画出最小可达矩阵

不看 Compose，先写出五个服务两两之间是否需要连接，再与实际三张网络比较。任何“以后可能用”都不能算当前必要边。

### 练习 B：解释一次数据库故障

用自己的话区分以下四个事实：API 进程存活、API 可接数据库流量、MySQL 容器 healthy、Migration 已完成。再预测停止 MySQL 时每个事实如何变化。

### 练习 C：证明最小权限

在专用测试环境验证 API 账号能 `SELECT 1`，但 DDL 被 MySQL 拒绝；Migrator 能执行审核 DDL。不要对共享或生产 schema 做实验。

### 练习 D：解释完整 Secret 集合

假设 MySQL volume 已初始化、四个 Secret 全部丢失，回答为什么重新生成四个随机值仍无法让 API 登录，以及什么信息才可能安全恢复。

### 练习 E：验证动态 DNS

记录 API 容器 ID，重建 API，再确认容器 ID 已变化、Web 未重启、代理恢复。不要依赖旧 IP 或手工修改 `/etc/hosts`。

### 练习 F：比较两个负载目标

说明 `/health` 100 RPS 与 `/ready` 20 RPS 的不同成本，并解释为什么 readiness P99 不应在缺少数据库负载模型时照搬 liveness 的 100ms 阈值。

## 22. 验收清单

- [ ] `make verify` 通过；
- [ ] `make compose-config` 不输出 Secret 内容并成功校验；
- [ ] 五个服务都被创建，`migrate` 成功退出 0；
- [ ] MySQL、API、Redis、Web health 状态符合设计；
- [ ] 只有 Web 发布 `127.0.0.1:<port>`，API/MySQL/Redis/Migrate 无宿主机端口；
- [ ] `/`、`/health`、`/ready` 和未知 `/api/...` 契约通过 smoke；
- [ ] 未知 API route 的 header/body request ID 一致；
- [ ] Web 在 API 离线时仍 healthy 且能提供 SPA；
- [ ] MySQL 离线时 `/health=200`、`/ready=503`；
- [ ] MySQL 恢复后 API 不重启即可恢复 readiness；
- [ ] Redis 离线不影响 API readiness；
- [ ] API 账号无 DDL 权限，Migrator 只有 schema 级审核权限；
- [ ] Secret 未进入 Git、image context、Compose environment 或文档输出；
- [ ] 应用服务以非 root 运行并启用只读根文件系统/capability 限制；
- [ ] 正常 Go 响应与 API-down 502 都只有一个可关联 `X-Request-ID`；
- [ ] query/Referer 唯一 marker 未出现在任何 Compose 日志中；
- [ ] 固定 M0 门禁按原始参数执行并将单行 JSON 结果记录到 QA；
- [ ] 任务临时文件已清理，用户原有容器、volume、数据库和镜像未被删除。

## 23. 生产差距

本节是本地开发拓扑，不是把 Compose 文件复制到服务器即可上线。至少仍缺：

- TLS 终止、可信域名、认证、授权、CSRF 与公网 probe 暴露策略；
- 外部 Secret manager、审计、轮换与恢复流程；
- MySQL 高可用、备份、恢复演练、复制、磁盘容量和加密；
- 生产账号 host/source 限制与管理员 provisioning；
- Redis 的真实业务键、TTL、一致性、容量、淘汰与恢复语义；
- 资源 requests/limits、编排调度、自动恢复、滚动发布和多实例策略；
- 镜像 digest、SBOM、签名、漏洞扫描、补丁与多架构发布；
- 集中日志、指标、trace、告警、SLO 和数据保留政策；
- 业务负载模型、数据规模、容量测试和故障注入；
- 正式 Migration 审批、备份、影子库和回滚/纠正流程。

后续可以在这个可重复环境上继续建立 CI，但不能用“Compose 已启动”跳过上述生产设计。课程的直接下一节进入最简单随机抽奖的领域对象分析，不在本节提前混入 CI 或业务实现。

## 24. 本节实现提交

| commit | 学习主题 |
| --- | --- |
| `e746a6f` | API/Migration 的互斥 `_FILE` Secret 消费边界 |
| `52c3add` | MySQL driver `NopLogger` 与安全日志边界 |
| `7aa6c9e` | 五服务 Compose 拓扑、镜像、Nginx、Secret generator、smoke 与 M0 工具 |

建议按表中顺序逐个 `git show <commit>`：先看应用进程如何消费 Secret，再看驱动日志为何收口，最后看部署拓扑如何组合这些已有边界。文档提交由本节最终交付记录补充，不把尚未生成的 SHA 写成事实。

## 25. 关键文件

| 文件 | 责任 |
| --- | --- |
| `deploy/compose/compose.yaml` | 服务、依赖、网络、Secret、health、volume 与安全边界 |
| `deploy/docker/Dockerfile.backend` | Go 多阶段构建及 API/Migrate target |
| `deploy/docker/Dockerfile.web` | 前端冻结构建和非 root Nginx runtime |
| `deploy/docker/Dockerfile.redis` | 固定 Redis runtime 与受控入口 |
| `deploy/docker/nginx.conf` | SPA、同源代理、动态 DNS、health 与脱敏日志 |
| `deploy/docker/redis-entrypoint.sh` | Redis Secret 校验、ACL 和无持久化配置 |
| `deploy/compose/mysql/init/10-create-growthos-users.sh` | 首次 volume 的双账号最小授权 |
| `deploy/compose/secrets/README.md` | 本地 Secret 权限与生命周期说明 |
| `scripts/generate-compose-secrets.sh` | 完整集合生成、验证、partial/volume guard |
| `scripts/compose-smoke.sh` | 服务、HTTP 契约、request ID 与端口冒烟 |
| `cmd/healthload` | 有界固定速率负载和 JSON 门禁报告 |
| `internal/platform/appconfig/config.go` | API/Migration `_FILE` 密码消费契约 |
| `internal/infrastructure/mysql/config.go` | 安全 MySQL driver 配置与 `NopLogger` |
| `Makefile` | 可审计的 Compose 生命周期与 M0 入口 |

架构决策见 [ADR-0012](../../decisions/ADR-0012-compose-development-topology.md)，部署消费契约见[第 16 节 API 记录](../../api/lessons/lesson-16.md)，安全操作见[本地 Compose 运维手册](../../runbooks/local-compose.md)，最终证据见[第 16 节 QA](../../qa/lessons/lesson-16.md)，开放推导见[第一性原理设计手记](../../design-thinking/lessons/lesson-16.md)，求职复盘见[面试问答](../../interview/lessons/lesson-16.md)。

## 26. 官方参考

- [Docker Compose 启停顺序与依赖条件](https://docs.docker.com/compose/how-tos/startup-order/)
- [Docker Compose Secret](https://docs.docker.com/compose/how-tos/use-secrets/)
- [Docker 多阶段构建](https://docs.docker.com/build/building/multi-stage/)
- [Docker 用户自定义 bridge 网络](https://docs.docker.com/engine/network/drivers/bridge/)
- [Compose service 属性：health、read_only、tmpfs、stop_grace_period](https://docs.docker.com/reference/compose-file/services/)
- [Compose named volume](https://docs.docker.com/reference/compose-file/volumes/)
- [MySQL 官方镜像初始化约定](https://hub.docker.com/_/mysql)
- [Redis 官方镜像](https://hub.docker.com/_/redis)
