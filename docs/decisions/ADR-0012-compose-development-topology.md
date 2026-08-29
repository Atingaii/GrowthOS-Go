# ADR-0012：Docker Compose 本地开发拓扑与隔离边界

- **状态：** 已接受
- **日期：** 2026-08-29
- **负责人：** GrowthOS 维护者

## 背景

第 11～15 节已经分别建立 Go HTTP 进程、结构化日志、MySQL 双身份与前向 Migration、React/Vite 页面框架，以及浏览器对 `/health`、`/ready` 的第一次真实消费。操作者仍需手工协调数据库、Migration、API 和前端进程，环境差异、端口冲突、Secret 注入和启动顺序没有统一机器契约。

用户本机 Docker Desktop 已有 MySQL、Redis、RabbitMQ、PostgreSQL 等个人容器和 volume。本节不能以“开发环境初始化”为由复用、停止、改密或删除这些资源；也不能因为本机已经安装某种中间件，就让它在没有业务语义时成为 API 强依赖。

需要决定：

1. 哪些进程进入 Compose，以及是否共享容器；
2. 浏览器、Web、API、Migration、MySQL、Redis 之间的最小可达关系；
3. 哪一个端口可以发布到宿主机；
4. 如何表达 MySQL ready、Migration 完成和 API 存活；
5. Secret 用环境变量还是文件注入，首次 volume 与密码集合如何保持一致；
6. Redis 是立即接入业务，还是只作为隔离环境能力存在；
7. Web 使用 Vite 运行时还是生产静态服务器；
8. 容器失败后自动重启还是保留现场；
9. 如何建立安全日志、request ID 关联、故障演练和固定性能基线；
10. 哪些开发便利不能被解释为生产方案。

## 决策驱动

方案必须同时满足：

- 不修改用户已有容器、端口、volume 或数据库；
- 一个命令可以得到同一套本地拓扑和可检查的启动条件；
- 浏览器继续使用第 15 节同源 path，不编译内部 service name；
- 宿主机暴露面最小，数据库和缓存不发布端口；
- API 进程与 DDL 生命周期、账号和 Secret 分离；
- liveness 与 readiness 语义不因容器 healthcheck 合并；
- API/MySQL/Redis 故障可以分别定位，正确组件不会被连带判死；
- Secret 不进入 Git、构建上下文、镜像层、普通环境变量或日志；
- 本地持久数据和可丢缓存有明确不同生命周期；
- 安全加固不妨碍非 root 镜像实际运行；
- 正常与故障路径都能自动验收，而不只检查首页一次 200；
- 方案为学习服务，复杂度必须由当前风险支撑。

## 评估过的方案

### 1. 继续维护多条 `docker run` 和宿主机启动命令

| 优点 | 代价 / 风险 |
| --- | --- |
| 每个组件命令直观 | 网络、volume、Secret、health 与顺序散落在 shell/history |
| 可以只启动一个组件 | 不同操作者容易获得不同端口、镜像和权限 |
| 无需 Compose 文件 | 清理目标不明确，容易误操作用户已有容器 |

**结论：不采用为主流程。** 单组件诊断仍可使用 Docker CLI，但完整开发拓扑由 Compose 作为声明式边界。

### 2. 一个容器运行 Nginx、Go、MySQL、Redis 和 Migration

| 优点 | 代价 / 风险 |
| --- | --- |
| 表面上“一次启动” | 多进程监督、日志、信号、用户和写目录混杂 |
| 无容器间 DNS | 任一进程失败难以表达，镜像巨大且职责不清 |
| 不需要网络设计 | Migration 一次性生命周期与常驻流量进程耦合 |

**结论：不采用。** 以生命周期和权限边界拆为五个服务，不以技术数量炫耀“微服务”。

### 3. 所有服务加入一张默认网络

| 优点 | 代价 / 风险 |
| --- | --- |
| Compose 最短 | Web 可直连 MySQL/Redis，Migration 可进入 edge |
| service name 全部互通 | 无法从拓扑看出当前真实依赖，未来误用成本低 |

**结论：不采用。** 使用 `edge`、internal `data`、internal `cache` 三张网络。Web/API 共享 edge；API/Migration/MySQL 共享 data；Redis 单独位于 cache。

### 4. 为 Web、API、MySQL、Redis 都发布宿主机端口

| 优点 | 代价 / 风险 |
| --- | --- |
| 可直接用宿主机工具访问每个组件 | 抢占用户已有 3306/6379，扩大局域网/宿主机暴露面 |
| 调试简单 | 浏览器可能绕过同源入口，环境拓扑进入前端配置 |

**结论：不采用。** 只发布 `127.0.0.1:${GROWTHOS_COMPOSE_WEB_PORT:-8088}:8080`。数据库深度诊断通过项目作用域内的受控容器/命令完成，不以永久端口换便利。

### 5. Compose Web 运行 `vite dev` 或 `vite preview`

| 方案 | 优点 | 当前问题 | 结论 |
| --- | --- | --- | --- |
| `vite dev` | HMR、前端迭代快 | 含构建工具和源码，不代表静态部署；需要 bind mount/watch | 宿主机开发保留，Compose 不采用 |
| `vite preview` | 可预览 build | 官方定位是本地预览，不是生产服务器；代理/日志/health 控制有限 | 不采用 |
| 多阶段 Node build + Nginx runtime | 运行层只含静态产物；可控制同源代理、缓存、日志和 health | 修改代码需 rebuild；增加 Nginx 配置 | 采用 |

### 6. Nginx 静态 upstream 与 Docker DNS 动态解析

| 方案 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- |
| 启动时静态解析 `api:8080` | 配置短 | API 缺失可能让 Nginx 启动失败；recreate 后可能固定旧 IP | 不采用 |
| Docker resolver + 变量 `proxy_pass` | API IP 变化后重解析；API 离线不阻止 SPA 启动 | 依赖 Docker 内置 DNS 语义；生产 discovery 不可照搬 | 采用 |

Web 不声明对 API 的 `depends_on`，Web health 使用本地 `/container-health`。API 离线时静态页面仍可用，代理请求产生带网关 request ID 的 gateway failure。

### 7. API healthcheck 使用 `/ready` 或 `/health`

| 方案 | 问题 | 结论 |
| --- | --- | --- |
| API health 请求 `/ready` | MySQL 故障把活着的 Go 进程判 unhealthy，可能形成无意义重启 | 不采用 |
| API health 请求 `/health`，流量检查另用 `/ready` | 保留进程存活/依赖就绪的可诊断组合 | 采用 |

MySQL health 也不能只检查进程/TCP；当前以 `growthos_app` 对目标 schema 执行真实 `SELECT 1`，使启动 gate 同时验证 Secret、身份和最小查询权限。

### 8. API 自动迁移、手工迁移与 one-shot Migration

| 方案 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- |
| API 启动自动迁移 | 步骤少 | DDL 与流量耦合，多实例竞争，高权限进入 API | 不采用 |
| 操作者每次手工记得先迁移 | 灵活 | 顺序不可审计，容易漏做 | 不作为 Compose 主路径 |
| `migrate` one-shot + `service_completed_successfully` | 生命周期、权限、失败 gate 清晰 | 启动多一步，失败需显式排查 | 采用 |

当前空 Migration 集返回 `no_migrations`、退出 0；不为演示制造空 `000001`。

### 9. 一套 root 账号、双账号或更多账号

| 方案 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- |
| API/Migration 都用 root | 配置最少 | API bug 获得全局 DDL/管理能力 | 不采用 |
| API/Migration 共用 schema owner | 两套 Secret 变一套 | 在线进程仍有 DDL；审计无法区分 | 不采用 |
| root 只初始化；API DML；Migrator schema DDL | 最小权限和生命周期对应 | 首次初始化与密码管理更复杂 | 采用 |

本地 API 授予 `SELECT/INSERT/UPDATE/DELETE`；Migrator 在目标 schema 额外授予 `CREATE/ALTER/DROP/INDEX/REFERENCES`。不授予全局 `ALL PRIVILEGES`。

### 10. 直接环境变量密码与文件 Secret

| 方案 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- |
| 明文写入 Compose environment | 简单 | 进入配置、inspect、进程环境和调试输出 |
| `.env` 插值 | 方便覆盖 | 仍进入 container environment；文件易误提交 |
| Compose file secret + 应用 `_FILE` | 按服务挂载、避开 image/env，兼容未来文件 Secret | 宿主机文件仍是明文；需要权限/生命周期处理 | 采用 |

应用要求直接值和 `_FILE` 恰好一个、读取有界、只 trim 结尾换行，并用稳定错误隐藏路径/内容。Compose 本地 Secret 不被误称为加密的生产 Secret manager。

### 11. Secret 按文件独立生成或按集合生成

| 方案 | 风险 | 结论 |
| --- | --- | --- |
| 缺哪个补哪个 | 可能产生跨批次身份；已有 MySQL volume 的账号密码不会同步 | 不采用 |
| 每次覆盖四个 | 直接使已有 volume 无法认证 | 不采用 |
| 0 个时先完整生成/验证再逐文件发布、4 个时验证、1～3 个时失败；volume 存在且 0 个时失败 | 四次发布不是文件系统事务；中断会留下部分集合，但下次运行必定失败而不会静默补齐 | 采用 |

宿主机 Secret 目录为 `0700`，文件为 `0444`。文件级可读是 Docker Desktop file secret 以 root-owned bind mount 进入非 root 容器的兼容选择；每个服务仍只挂载声明的文件，目录遍历由宿主机用户权限限制。

### 12. Redis 立即接入并持久化，还是隔离且可丢

| 方案 | 优点 | 当前问题 | 结论 |
| --- | --- | --- | --- |
| API 立即接入 Redis，并加入 readiness | 简历技术点更多 | 没有真实 key/TTL/一致性需求；增加伪依赖 | 不采用 |
| Redis 独立运行，启用 volume/AOF | 重启保留数据 | 当前无恢复目标，形成没有消费者的持久状态承诺 | 不采用 |
| Redis 只在 internal cache 网络，ACL 保护，RDB/AOF 关闭，`/data` tmpfs | 可验证镜像/Secret/health，故障不影响 API，边界诚实 | 当前不能宣称业务缓存已实现 | 采用 |

### 13. 自动 restart 与显式失败

| 方案 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- |
| `always/unless-stopped` | 临时故障后可能自愈 | crash loop 覆盖首次失败和退出码，日志滚动 |
| dependency `restart: true` | 显式 Compose restart 可传播 | 用重启掩盖数据库连接池恢复；错误耦合生命周期 |
| `restart: "no"` | 现场、状态和 exit code 稳定 | 需要操作者显式恢复 | 本地开发采用 |

运行期 MySQL 故障不触发 API restart；数据库恢复后，Go 连接池必须自行恢复 readiness。

### 14. 完整工具链运行镜像与多阶段非 root 镜像

| 方案 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- |
| Go/Node builder 直接作为 runtime | 容器内调试方便 | 编译器、包管理器、源码和缓存扩大镜像/攻击面 |
| 多阶段构建，只复制二进制/静态资产 | 运行边界小、build/runtime 清晰 | 调试需单独工具容器 | 采用 |

Go API/Migrate 使用 UID/GID 65532，Nginx 使用 101，Redis 使用 999；应用服务 read-only、drop ALL capabilities、no-new-privileges，并为必要写路径提供有界 tmpfs。MySQL 因官方初始化和数据写入保留可写 named volume，不强行套用相同限制。

### 15. 网关直接透传 request ID 或统一回写

| 方案 | 风险 | 结论 |
| --- | --- | --- |
| 只透传 upstream ID | API-down 502 没有可关联 ID；静态请求也缺关联 |
| Nginx 无条件再加一个 ID | 可能产生两个冲突 `X-Request-ID` |
| map upstream ID/`$request_id`，隐藏 upstream 同名头，再统一 `add_header ... always` | 配置稍多；能保证单一最终 ID并覆盖 gateway | 采用 |

access log 使用同一最终 ID。Go 响应关联到 Go 日志，502/静态响应关联到 Nginx 日志。

### 16. 日志保留完整 request target 或严格脱敏

| 方案 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- |
| access/error log 记录完整 URI、Referer | 诊断信息多 | query、授权码、过滤条件和来源可能泄密 |
| access log 只记 `$uri`，不记 Referer；error log 仅 `crit` | 降低敏感数据面，仍保留 status/upstream/耗时/ID | 请求级 Nginx 原始错误细节减少 | 采用 |

MySQL driver 同理使用 `NopLogger`，避免原始 cause 绕过 GrowthOS 的安全结构化日志。深度诊断转到受控 DBA/主机工具。

### 17. 临时手工 curl 与固定 M0 门禁

| 方案 | 风险 | 结论 |
| --- | --- |
| 首页 curl 一次 200 | 无法检查依赖、错误契约、端口和持续行为 | 不足 |
| 可调参数负载命令 | 适合开发，但可以把 5 分钟门禁覆盖为 1 秒仍声称完成 | 保留为辅助 |
| smoke + recipe 中不可覆盖的 M0 参数 | 时间成本更高，但验收定义稳定 | 采用 |

M0 固定 `/health` 100 RPS×5 分钟、32 workers、2 秒 timeout、P99≤100ms；`/ready` 20 RPS×30 秒、32 workers、2 秒 timeout。它是章节本地门槛，不是生产容量模型。

## 决策

1. 本地开发拓扑由 `deploy/compose/compose.yaml` 声明，Compose project 默认名为 `growthos`，不设置全局 `container_name`，避免与用户已有容器命名冲突。
2. 定义 `web`、`api`、`migrate`、`mysql`、`redis` 五个服务；每个容器保持单一主要生命周期，Migration 为 one-shot。
3. 只发布 Web：`127.0.0.1:${GROWTHOS_COMPOSE_WEB_PORT:-8088}:8080`。API 只 `expose` 8080，MySQL/Redis/Migrate 不发布宿主机端口。
4. 定义 `edge`、internal `data`、internal `cache` 三张网络；成员严格按当前必要通信配置。Redis 当前是 cache 网络唯一成员。
5. Web 使用 Node/pnpm 多阶段构建 React，再以非 root Nginx 提供静态资产；Compose 不运行 Vite dev/preview。
6. Nginx 保持浏览器同源 path：精确代理 `/health`、`/ready`，代理 `/api` 命名空间，不 rewrite；其他页面回退 SPA。
7. Nginx 通过 Docker resolver `127.0.0.11` 和变量 upstream 动态解析 `api`，API 缺失不阻止 Web 启动，API recreate 不要求 Web restart。
8. Web health 使用独立 `/container-health` 204，不访问 API；Web 不依赖 API 启动。
9. MySQL health 使用 `growthos_app` + app Secret 在目标 schema 执行 `SELECT 1`，不使用只证明 daemon 响应的弱检查。
10. `migrate` 等待 MySQL healthy；API 等待 MySQL healthy 和 Migration successful completion。依赖条件只作为启动 gate，不被解释为运行期自动恢复。
11. API Compose health 使用 `/health` liveness，不使用 MySQL `/ready`。数据库中断时 API 应保持 healthy，而 `/ready` 安全返回 503。
12. 所有服务 `restart: "no"`，不配置 dependency `restart: true`。MySQL 恢复后 API pool 必须在不重启进程的情况下恢复 readiness。
13. MySQL 使用唯一 named volume `mysql_data`；普通 `compose-down` 保留它。删除 volume 只通过带精确确认词的 `compose-reset`，且操作者必须先解析 project 和 volume label。
14. MySQL root Secret 只用于首次官方初始化；`growthos_app` 仅 schema DML，`growthos_migrator` 仅目标 schema 的 DML + 审核 DDL。API 与 Migration 永不使用 root 或互相借用密码。
15. 四个本地 Secret 作为完整集合管理：无文件时先在私有临时目录完整生成/验证再逐文件发布，全有时验证，部分存在时失败；已有 `${project}_mysql_data` 但全套 Secret 缺失时失败。生成器不覆盖现有值，也不把四次文件移动误称为单一文件系统事务。
16. Secret 目录 `0700`、文件 `0444`，并从 Git 和 Docker build context 排除。每个服务只挂载所需 Secret。
17. Go API/Migration 分别支持互斥直接密码变量与 `_FILE` 变量；文件读取有界、只去除结尾换行、错误不泄漏路径或内容。Compose 只使用 `_FILE`。
18. Redis 使用独立 Secret 生成 ACL，位于 internal cache 网络，不接入 API、不加入 readiness，关闭 RDB/AOF，以 64 MiB tmpfs 提供可丢数据目录。
19. Go/Node 使用多阶段构建和固定版本 tag；Go 依赖执行 download + verify，前端使用 frozen lockfile。最终 runtime 不携带编译工具和源码。
20. API/Migrate/Web/Redis 以非 root 运行，root filesystem read-only、drop ALL capabilities、no-new-privileges、`init: true`，必要写入只进入有界 tmpfs。MySQL 按官方数据生命周期单独处理。
21. Compose json-file log 设置 10 MiB×3 轮转。Nginx access log 只记录 path、最终 request ID、status/upstream status、bytes 和耗时，不记录 query/Referer；Nginx error log 仅保留 critical。
22. Nginx 对 upstream `X-Request-ID` 先隐藏后统一回写：有 upstream 时使用 API ID，无 upstream/静态/gateway 时使用 Nginx ID，且 access log 使用同一 ID。
23. MySQL driver 使用 `NopLogger`，防止 driver 原始连接错误直接写非结构化 stderr；应用日志继续只报告稳定 stage。
24. Compose stop grace 分别为 Web/API 10s、MySQL/Redis 30s、Migration 50s，以覆盖各自进程内部 shutdown/lock 预算并留余量。
25. `make compose-smoke` 检查服务/exit 状态、HTTP/错误/request ID 和端口隔离；`make compose-m0` 固定运行 smoke 与两段不可覆盖负载。
26. 任何 Compose 操作必须以明确 project/file 为作用域；不使用 `docker system prune`、`docker volume prune` 或无目标批量删除清理本节资源。

## 验收证据

最终固定门禁：

| 目标 | 计数 | 错误 | 延迟/速率 |
| --- | --- | --- | --- |
| `/health` | scheduled/completed/success `30000/30000/30000` | errors/unexpected/dropped `0/0/0` | P50 `1.084208ms`、P95 `2.744875ms`、P99 `4.1495ms`、max `18.116291ms`、实际 `100.0027 RPS`；100ms 门槛通过 |
| `/ready` | `600/600/600` | `0/0/0` | P50 `4.08525ms`、P95 `5.935083ms`、P99 `6.841375ms`、max `8.570541ms`、实际 `20.0276841 RPS` |

32 workers readiness 复测与最终 smoke 后，Web/API/MySQL/Redis 瞬时内存约 `5.535/6.664/438/23.41 MiB`，Docker 配额 `1.924 GiB`；这些值会随 allocator/cache 波动，不是 limit 或峰值。API-down 502 仍返回唯一 `X-Request-ID` 并与安全 access log 关联；query/Referer marker 未出现在任何 Compose 日志。证据和环境限制见[第 16 节 QA](../qa/lessons/lesson-16.md)。

## 影响

### 正面影响

- 开发者可以从统一 Make 入口得到可重复的服务、账号、网络和健康拓扑；
- 用户已有 MySQL/Redis 等资源不被端口、名字或数据生命周期碰触；
- 浏览器同源契约从 Vite 平滑迁移到接近部署的 Nginx 静态入口；
- API、MySQL、Web、Redis 和 Migration 的故障可以分别观察，不会被一个“全局健康”布尔值掩盖；
- API 没有 DDL 权限，Migration 失败能在流量进程启动前阻断；
- Secret 不进入镜像或普通环境变量，且旧 volume/新密码错配会 fail closed；
- API recreate、MySQL restart 的恢复能力可以真实演练，而非依赖连锁重启；
- Redis 的存在不被夸大为已实现缓存，后续仍可由真实用例决定；
- 固定 M0 给后续章节提供可比较的本地回归基线；
- 502 也有网关 ID，正常链路与失败链路都有一致关联入口。

### 成本与风险

- 维护三份 Dockerfile/入口配置、Compose、Secret generator、smoke 和负载工具；
- Node/Vite HMR 与 Compose 静态验证是两条开发路径，代码更新需选择正确入口；
- 三张本地 bridge 网络提高了文件长度和排障学习成本；
- file Secret 在宿主机仍是明文，`0444` 选择依赖上层 `0700` 目录和本机账户模型；
- MySQL 初始化脚本只在空 volume 执行，账号/Secret 轮换不能靠编辑文件完成；
- `restart: no` 要求操作者显式处理失败，但这是保留学习现场的有意代价；
- Redis 无持久化意味着任何数据重建即丢失；在接入业务前这是正确边界，接入后必须重新决策；
- `NopLogger` 和 Nginx `crit` error log 减少了通用日志原始细节，深度诊断需要受控工具；
- Docker DNS resolver 地址和行为是当前 runtime 特定实现，不能直接移植到 Kubernetes/生产网关；
- M0 只覆盖无业务 payload 的探针，极低延迟不能外推为业务 API 性能。

## 数据与撤销

普通撤销：

```bash
make compose-down
```

它移除本 Compose project 的容器和网络，保留 `mysql_data`。下一次 up 复用数据库账号和数据，所以必须保留同一 Secret 集合。

完全数据重置是破坏性操作：

```bash
make compose-reset CONFIRM=reset-growthos-data
```

执行前必须用 Compose label 解析准确 volume，确认它属于目标 project，备份需要保留的数据，并确认不是用户已有的外部 MySQL volume。不得通过改 project 名、通配符、prune 或 Docker Desktop 批量删除替代。

若只回退本 ADR 的应用/Nginx 改动，必须保持以下不可变契约：

- API 与 Migration 身份和进程仍分离；
- 浏览器仍使用同源 path；
- `/health` 与 `/ready` 语义不合并；
- 已初始化 MySQL volume 不得配一套随机新 Secret；
- 用户现有 Docker 资源不受影响；
- 数据删除必须显式、精确和可审计。

## 生产差距

该决策只适用于单机本地 development。它没有决定：生产 TLS/域名/认证网关、Secret manager、MySQL HA/备份/复制、Redis 业务模型、资源限制和调度、集群网络策略、多实例 discovery、镜像 digest/SBOM/签名、集中可观测性、滚动发布或正式容量 SLO。

尤其：本地账号 host `%`、MySQL TLS disabled、loopback HTTP、file Secret、`restart: no` 和 Docker DNS resolver 都不能未经重新评估复制到 staging/production。

## 重新评估触发条件

出现以下任一证据时，应修订本 ADR 或新增决策：

- API 出现第一个真实 Redis 用例，需要定义 key、TTL、失效、一致性、容量和降级；
- Redis 状态从可丢 cache 变为必须恢复的数据；
- 多个本地服务需要访问 Redis，cache 网络成员发生变化；
- 出现真实业务 Migration，one-shot 时长/锁超出当前 50 秒停止预算；
- MySQL 用户初始化或密码轮换需要在保留 volume 时自动化；
- 团队需要容器内 HMR、remote development 或多架构镜像；
- 生产平台使用 Kubernetes、Nomad、Swarm 或托管服务，需要新的 discovery/secret/probe 模型；
- `/ready` 新增依赖或成本明显上升；
- M0 出现回归、主机差异显著，或需要业务 payload/数据规模容量测试；
- Nginx access/error 日志不足以诊断真实事故，需在不泄漏敏感数据的前提下引入结构化网关日志/trace；
- 需要将 Web/API 暴露给非 loopback 客户端；
- 单机 MySQL volume 已不能满足备份、恢复、HA 或数据规模要求。

## 相关文档

- [第 16 节课程](../course/part-02/lesson-16-docker-compose-development.md)
- [第 16 节 API 记录](../api/lessons/lesson-16.md)
- [本地 Compose 运维手册](../runbooks/local-compose.md)
- [ADR-0010：MySQL 与 Migration 边界](ADR-0010-mysql-migration-boundaries.md)
- [ADR-0011：前端同源代理边界](ADR-0011-same-origin-frontend-integration.md)

## 官方参考

- [Docker Compose 启停顺序](https://docs.docker.com/compose/how-tos/startup-order/)
- [Docker Compose Secret](https://docs.docker.com/compose/how-tos/use-secrets/)
- [Docker 多阶段构建](https://docs.docker.com/build/building/multi-stage/)
- [Docker 构建最佳实践](https://docs.docker.com/build/building/best-practices/)
- [Docker bridge 网络](https://docs.docker.com/engine/network/drivers/bridge/)
- [Compose services 参考](https://docs.docker.com/reference/compose-file/services/)
- [Compose volumes 参考](https://docs.docker.com/reference/compose-file/volumes/)
- [`docker compose down`](https://docs.docker.com/reference/cli/docker/compose/down/)
- [MySQL 官方镜像](https://hub.docker.com/_/mysql)
- [Redis 官方镜像](https://hub.docker.com/_/redis)
- [NGINX resolver](https://nginx.org/en/docs/http/ngx_http_core_module.html#resolver)
