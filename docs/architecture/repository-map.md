# 仓库地图

**状态：** 当前
**更新日期：** 2026-08-30

本文件描述当前仓库，而不是第 101 节的目标仓库。目录随需求演进，改动目录职责时必须同步更新本文件。

第 9 节已验收仓库工程基线。第 11～12 节落实 Gin 进程、配置、结构化日志、请求关联和错误适配；第 13 节加入 MySQL 连接池、独立 Migration 命令、数据库 readiness 与真实 MySQL 8.4 验收；第 14～15 节完成 React 框架和首个真实前后端系统探针切片；第 16 节把这些能力装配为隔离的 Compose M0 开发栈；第 17～18 节落地 Strategy/Award 聚合与两张业务表；第 19 节以 application 窄端口和 MySQL Strategy Repository 闭合持久化边界；第 20 节新增 consumer-owned bounded random port、加权 Selector 与 crypto adapter；第 21 节把只读 Repository、Selector 和专用 HTTP adapter 装配为 development/test ephemeral selection API；第 22 节通过 Lottery API adapter、运行时 decoder 与请求状态 Hook 形成真实 React 消费者，并用共享 `WorkspaceShell` 收敛四类工作台；第 23 节只新增规则需求、ADR 和学习证据；第 24 节以 Lottery-owned cache-aside 装饰器、窄 Redis client 与最小 ACL 加速 Strategy 读取投影。当前仍没有正式 Draw/Result、登录认证、RBAC/对象级授权、幂等、库存或发奖；除系统探针与 Lottery selection 外，工作台业务数据仍为明确 Mock/本地状态。

| 路径 | 当前职责 | 引入产品代码的章节 |
| --- | --- | --- |
| `cmd/doccheck` | 文档完整性与课程证据检查器 | 项目基线 |
| `cmd/docsync` | 项目 README/docs 到 Obsidian Vault 的单向镜像工具 | 项目基线 |
| `cmd/growth-api` | Go 产品进程入口：版本、信号、共享 MySQL pool、可选 Redis pool、Gin/HTTP、Lottery selection/cache 用例装配与资源关闭 | 第 11～13、21、24 节 |
| `cmd/growth-migrate` | 独立前向迁移入口，只暴露 `up/status`，装配 Migrator 账号、嵌入 source 与 Runner | 第 13 节 |
| `cmd/healthload` | 标准库定速 HTTP 负载器；输出单行 JSON，并以错误、异常状态、丢弃请求或 P99 越界作为失败 | 第 16 节 |
| `internal/platform/appconfig` | `GROWTHOS_` API/Migration 独立配置、直接/文件秘密二选一、公开默认值、秘密脱敏、Lottery/cache feature gate、Redis TLS/pool 与跨组件 timeout 校验 | 第 12～13、16、21、24 节 |
| `internal/platform/logging` | 标准库 `slog` logger 构造、级别、格式与基础字段 | 第 12 节 |
| `internal/platform/fault` | 与传输无关的 fault kind、稳定 code、公开消息与内部 cause；包含 unavailable 语义 | 第 12～13 节 |
| `internal/infrastructure/httpapi` | Gin router、`/health`、MySQL `/ready`、请求中间件、错误映射、统一错误 `no-store` 缓存策略与 HTTP 契约测试 | 第 11～15 节 |
| `internal/infrastructure/httpserver` | 标准库 HTTP Server 运行、配置化 timeout、context 取消、优雅关闭与脱敏 ErrorLog 桥接 | 第 11～12 节 |
| `internal/infrastructure/mysql` | 安全 driver Config、TLS、API `sqlx` pool、Migration 单连接、首次 Ping、稳定错误 stage 与不绕过 JSON 边界的 driver logger | 第 13、16 节 |
| `internal/infrastructure/redisstore` | 不在构造时探测依赖的有界 RESP2 client；只暴露 `GETRANGE`、`SET`、`DEL` 与 `Close`，拥有 TLS、连接池、超时、错误脱敏和坏连接驱逐 | 第 24 节 |
| `internal/infrastructure/migration` | 嵌入 source 校验、前向 `up/status`、dirty/version/cancelled 状态机与资源所有权 | 第 13 节 |
| `internal/lottery/domain` | 持久化/传输无关的 Strategy 聚合、Award 候选、正整数相对权重、显式 reward/no_reward、名称契约，以及 bounded random port 与减法桶 WeightedSelector | 第 17、20 节 |
| `internal/lottery/application` | 调用方拥有的 `StrategyCreator` / `StrategyReader` 窄端口、仓储错误语义，以及组合快照读取与 Award 选择的 `EphemeralSelectionService`；不依赖传输或 SQL adapter | 第 19、21 节 |
| `internal/lottery/adapter/mysqlrepo` | 手写 SQL 的 Strategy Repository：父子原子 Create、只读 RR 快照 FindByID、最多 1000 个 Award 的失败关闭边界、领域恢复、context 和 driver 错误分类；不拥有共享 pool | 第 19、21 节 |
| `internal/lottery/adapter/strategycache` | Lottery-owned StrategyReader cache-aside 装饰器：版本化投影、完整 uint64 string、2 MiB/1000 Award 边界、TTL jitter、同 key fill 合并、坏值精确删除与 fail-open；不拥有 Redis client | 第 24 节 |
| `internal/lottery/adapter/randomsource` | 以 `crypto/rand.Int` 实现均匀 `[0,upper)` 的生产随机适配器；支持完整 uint64、稳定错误与并发共享，不负责权重映射或 Draw 审计 | 第 20 节 |
| `internal/lottery/adapter/httpapi` | 注册默认关闭的 ephemeral selection route，校验规范 uint64 path、demo header/query/body framing，映射最小 string DTO、稳定错误、timeout、no-store 与 Request ID | 第 21 节 |
| `internal/*` | Lottery 之外仍预留的私有领域与基础设施边界；占位不代表实现 | 随对应领域章节引入 |
| `pkg` | 可被外部导入的少量稳定 Go 包 | 仅在确有跨模块公共契约时 |
| `configs/growth-api.env.example` | 不自动加载且不给密码赋值的 API/Migration 公开环境变量示例 | 第 12～13 节 |
| `migrations/embed.go` | 编译期嵌入 Migration 说明与 `sql/` source；迁移字节通过 hash 测试防止已发布历史被静默改写 | 第 13、18 节 |
| `migrations/sql` | 严格命名的前向 `.up.sql`；`000001` 建 `lottery_strategy`，`000002` 建 `lottery_strategy_award`，当前 latest 为 2 | 第 13 节机制，第 18 节业务结构 |
| `migrations/lottery_schema_integration_test.go` | 只在双显式授权的隔离 schema 上，以专用 writer 测试身份验证两表结构、Repository 所需 SELECT/INSERT、负向权限与回滚清理；不定义当前运行账号权限 | 第 18～19 节 |
| `scripts/generate-compose-secrets.sh` | 完整 Secret 集合生成/验证；部分集合与“旧 MySQL volume + 缺凭据”状态 fail closed，阻止静默错配 | 第 16 节 |
| `scripts/compose-smoke.sh` | 四常驻/两 one-shot 状态、latest 2、两表 SELECT-only、Redis 网络/Secret/ACL/内存策略、探针/ephemeral 404、HTTP 契约与端口隔离；对 MySQL/业务事实只读，仅写入并精确清理一个有 TTL 的 Redis ACL 探针 key | 第 16、18～19、21、24 节 |
| `scripts/compose-lottery-api-acceptance.sh` | 以随机 project/secret/volume/image 创建一次性真实纵向环境，验证 Lottery/cache success/failure、ACL、poison、自愈、Redis/MySQL/网关故障恢复、M1 三基线、数据指纹与所有权校验清理 | 第 21、24 节 |
| `deploy/compose` | Web/API/MySQL/Redis 四个常驻服务，Migrate/mysql-grants 两个 one-shot，三张隔离网络、MySQL data/socket named volume、API/Redis 文件秘密、ephemeral/cache 配置和仅回环 Web 端口 | 第 16、18～19、21、24 节 |
| `deploy/compose/mysql/grants` | 只经 MySQL Unix socket、`network_mode: none` 运行的应用授权收敛脚本；当前只允许两张 Lottery 表 `SELECT`，mandatory role 非空时失败关闭 | 第 18～19、21 节 |
| `deploy/docker` | API/Migrator/Web/Redis 构建边界、受限 Go 编译并行/内存、非 root 运行入口、Redis 最小 ACL/48 MiB allkeys-lru，以及限制 Host/framing/size/timeout/request ID 的 Nginx 同源网关 | 第 16、21、24 节 |
| `web` | 统一 React 用户端、运营端、MCP 与 AI Operator 框架；系统状态与 ephemeral Lottery 已真实联调，其余业务页面为带边界说明的 Mock/本地交互 | 第 14～15、22 节 |
| `web/src/api` | 只访问同源路径的 HTTP client、六类失败语义、bodyless POST、运行时 decoder，以及 system/lottery API adapters | 第 15、22 节 |
| `web/src/components/layout/WorkspaceShell.tsx` | 四类工作台共享的桌面侧栏、移动抽屉、顶栏、搜索、通知样例、主题、全宽和内容几何；不承担认证或授权判断 | 第 22 节 |
| `web/src/pages/user/lottery` | `/lottery` 页面与 selection Hook：规范 StrategyID、显式状态机、pending 抑制、取消和旧响应隔离；不产生浏览器随机结果 | 第 22 节 |
| `web/src/pages/system/status` | `/system/status` 页面、并行探针 hook、取消/竞态控制和组件测试 | 第 15 节 |
| `web/vite.config.ts` | dev/preview 的 `127.0.0.1` 严格端口和 `/health`、`/ready`、`/api` 精确同源代理 | 第 15 节 |
| `docs` | 产品、架构、决策、QA、第一性原理设计推导、面试问答和课程事实；第 24 节增加 Redis Strategy 缓存 ADR 与运维证据 | 全程 |
| `docs/design-thinking` | 按章节保存事实到机制的推导、备选方案、失败模型、风险账本与重决策条件 | 第 13 节起，历史章节回填 |
| `docs/interview` | 按章节保存可口述问答、追问、项目证据、选型边界与分级外部来源 | 第 13 节起，历史章节回填 |
| `docs/runbooks` | MySQL Migration、本地 Compose 与 Redis Strategy 缓存的运行、发布、故障停止条件、恢复和数据清理纪律 | 第 13、16、24 节 |

## 当前依赖规则

1. `cmd/*` 只做装配和进程生命周期管理。
2. `internal/<domain>` 拥有自己的领域模型、应用用例和端口。
3. 领域模块不得直接依赖另一个模块的数据库实现。
4. `internal/infrastructure` 提供跨领域的数据库连接、Migration、HTTP 等平台能力；`internal/<domain>/adapter` 实现领域端口的技术适配器，两者都不拥有领域规则。
5. `pkg` 不是杂物目录；不稳定或仅仓库内部使用的代码留在 `internal`。
6. 当前 `.gitkeep` 只表示计划边界，不代表能力已经实现。
7. 浏览器 API 适配只接受同源路径；开发代理目标由仅 Vite 进程读取的 `GROWTHOS_WEB_API_PROXY_TARGET` 配置，默认 `http://127.0.0.1:8080`，不向浏览器暴露后端 origin。
8. `internal/lottery/domain` 不导入 Gin、SQL/sqlx、Redis、`crypto/rand` 或 JSON/DB tag；MySQL adapter 只能通过 `RestoreAward` / `RestoreStrategy` 重建合法聚合，不能静默修复存量事实；randomsource adapter 依赖 domain-owned 窄端口，依赖方向不能反转。
9. application 层端口按消费者能力拆分；当前没有 CRUD 大接口，ephemeral selection 只依赖 `StrategyReader` 与 Award selector，不获得写能力，adapter 也不反向进入 domain/application。
10. Lottery HTTP DTO 不复用 domain struct；公开 ID 使用规范十进制 string，weight、Strategy 名称与内部错误不进入 selection 响应。
11. 业务规则跟随权威事实所有者和业务阶段，不建立跨 Activity、Participation、Lottery、Benefit 与 Governance 的万能 `common/rules`；通用执行原语只有在至少两个真实消费者证明同语义后，才由消费方反推。
12. Redis 是 adapter 依赖而不是领域事实：`strategycache` 装饰 application-owned `StrategyReader`，MySQL reader 仍是权威来源；HTTP/application/domain 不获得 Redis 命令或 key 能力。

第 11 节的 `/health` 仍是无外部依赖的进程 liveness，只证明 Gin 路由和 handler 能响应。第 13 节的 API 在监听前必须打开并 Ping MySQL，运行中 `/ready` 每次用有界 Ping 表示数据库 readiness；依赖故障时 `/ready` 为 503 而 `/health` 仍可为 200。两者都不证明业务数据正确、Migration 最新或 SLO 达标。

第 12 节保持 `request_id` 与未来 OpenTelemetry `trace_id` 分离；fault 平台层不导入 Gin/HTTP，只有 HTTP adapter 决定 status 和公开 error envelope。配置与隐私规则见[配置参考](../configuration.md)，长期边界见[ADR-0009](../decisions/ADR-0009-runtime-boundaries.md)。

第 13 节保持 API 与 Migration 身份和进程分离：`growth-api` 使用受限 pool 且不执行 DDL，`growth-migrate` 使用专用单连接且只提供前向 `up/status`。第 18 节产品 source 已到 latest 2；两个版本各包含一条 `CREATE TABLE`，已应用环境应 `clean` 且 `version=latest=2`，重复 `up` 为 `no_change`。第 19 节隔离 Repository writer 测试曾验证两表 `SELECT, INSERT`；第 21 节依据实际运行用例把长期 Compose 应用身份进一步收敛为两表 `SELECT`，不能 INSERT、UPDATE、DELETE、执行 DDL 或读写 `schema_migrations`。边界见 [ADR-0010](../decisions/ADR-0010-mysql-migration-boundaries.md)、[ADR-0016](../decisions/ADR-0016-lottery-repository-boundaries.md)和 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)，操作步骤见 [MySQL Migration 运维手册](../runbooks/mysql-migrations.md)。

第 15 节的系统状态页通过 Vite dev/preview 同源代理真实消费 `GET /health` 与 `GET /ready`，统一 client 做运行时 JSON 契约检查，并由状态 hook 管理并行请求、取消和过期结果。Go 统一错误响应明确携带 `Cache-Control: no-store`。正常、数据库不可用和 API 离线场景已做真实浏览器关联验收，但这些证据不代表吞吐、长期可用性或生产 SLO 已验证。前端工具链要求 Node.js `>=22.22.2`、pnpm `10.13.1`，质量门包含 `test`、`typecheck` 和 `build`。

第 24 节的 Compose 拓扑仍只发布 `127.0.0.1:8088` 的 Nginx Web 入口。Web/API 位于 `edge`，API/MySQL/Migrator 位于内部 `data`，只有 API/Redis 位于 Docker-internal `cache`；启动门仍为 `mysql healthy → migrate exited 0 → mysql-grants exited 0 → API`，Redis 不成为 API 启动或 readiness authority。`mysql-grants` 使用 MySQL 官方客户端镜像、非 root UID 999、只读根文件系统和共享 socket，`network_mode: none`，精确撤销旧授权后仅授予两张业务表 `SELECT`，并要求 `@@GLOBAL.mandatory_roles` 为空；它不挂入 data 网络。Redis 默认用户关闭，业务 ACL 允许无 key 的 `PING`，并只允许对版本化前缀执行 `GETRANGE/SET/DEL`；实例固定 48 MiB、`allkeys-lru`、无持久化。API、Migrator、Web、Redis 使用非 root、只读根文件系统、去除 capabilities 与 `no-new-privileges`，MySQL 官方镜像则保留初始化阶段 root 和可写数据目录这一明确例外。Nginx 动态解析 API 容器地址，统一回写 `X-Request-ID`，并在 API location 约束本地 Host、请求 framing、16 KiB 上限和 timeout；非法 Host 的 421 是 server-level 非 JSON，HTTP parser 更早拒绝的不支持 Transfer-Encoding 也不承诺 JSON。长期边界见 [ADR-0012](../decisions/ADR-0012-compose-development-topology.md)、[ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)和 [ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)，操作步骤见 [Docker Compose](../runbooks/local-compose.md)与 [Redis 缓存运维手册](../runbooks/redis-strategy-cache.md)。

第 17 节的 `Strategy` 拥有至少一个 `Award`，在构造时检查正 ID、名称、正权重、封闭 Outcome、AwardID 唯一和总权重溢出，并对候选 slice 做防御性复制和 AwardID 规范排序。第 18 节的两张表保护可由单行、主外键表达的子集。第 19 节 Repository 在写前重新验证调用方聚合，在读后以 `RestoreAward` / `RestoreStrategy` 原样恢复；Create 只写一次完整聚合，FindByID 在单一 read-only RR 快照中读取。第 20 节的 `WeightedSelector` 对多候选请求 `[0,totalWeight)` 均匀位置并用减法桶映射，单候选直接返回，`no_reward` 是成功 Award；生产 `CryptoSource` 支持完整 uint64 并拒绝 modulo bias。第 21 节的 `EphemeralSelectionService` 与 HTTP adapter 只读取快照并返回一次临时选择，完整 uint64 ID 以 decimal string 传输；路由默认关闭且 staging/production 不能开启。第 22 节 React adapter 保持这组 DTO 和失败语义，页面不再由浏览器随机决定 Award，也不自动重试。第 23 节保持全部代码契约不变，只明确前置业务拒绝、授权拒绝、资源不可用、技术失败/未知与合法 `no_reward` 不能混用。第 24 节在 MySQL Reader 外包一层可选 cache-aside：命中时恢复同一合法聚合，miss/坏值/Redis 故障回源，成功回源后 best-effort 写入；not-found 不缓存，TTL 最多 5 分钟并带最多 10% jitter。整条链仍没有 DrawID、结果持久化、认证、授权、幂等、库存或发奖，INV-03 尚未满足；Redis 缓存命中不能冒充规则决策已完成。边界见 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)、[规则需求基线](../product/lottery-rule-requirements-v1.md)、[ADR-0019](../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)和 [ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)。

第一版运行时采用 [ADR-0007](../decisions/ADR-0007-modular-monolith-first.md) 确定的模块化单体：一个 Go 产品进程可以装配多个领域模块，但共享进程和数据库实例不改变事实所有权。服务拆分必须等待第 78 节起出现的负载、发布、故障域、合规或团队证据。

## 有意延迟的决定

- Gin 在第 11 节作为 Go HTTP 基线接入；gRPC + Protobuf 是后续服务间 RPC 基线，到第 80 节再按拆分需求接入。
- 第 12 节已把监听地址、HTTP timeout、日志级别和格式纳入显式配置，并建立请求关联与统一错误。
- MySQL 连接、`sqlx` pool 与前向 Migration 机制已在第 13 节接入，第 18 节建立首组 Lottery 表，第 19 节已交付 Create/FindByID 手写 SQL、事务与真实引擎验证；第 24 节只缓存不可变读取投影。更新、删除、版本发布及其精确失效协议仍等待真实用例。
- React、TypeScript、Vite、Tailwind CSS、Lucide、Recharts 和 Zustand 在第 14 节接入；第 15 节接通系统探针，第 22 节接通 Lottery ephemeral selection 并收敛共享工作台壳层。其余领域仍等待真实 API，不以 Mock 或本地交互冒充完成。
- 第 16 节已引入 Compose 本地开发环境，第 24 节接入第一个业务 Redis 消费者和最小 ACL；它仍是单机开发拓扑：没有镜像 digest 固定、内部 TLS、生产资源配额、Secret Manager、Redis HA 或生产容量证明。
- 第 19 节已建立 Lottery Strategy 窄仓储，第 20 节已建立无偏加权 Award 选择，第 21 节已建立两表 `SELECT` 运行权限和受限 ephemeral API，第 22 节已交付真实 ephemeral React 页面，第 23 节已建立规则事实所有权与演进停止线，第 24 节已交付可重建 Strategy 读取投影缓存。正式资格、Draw/Result、幂等、库存与发奖必须由后续真实问题驱动，写权限和缓存失效总线不会因“常见架构”被提前加入。
- 登录认证、RBAC、租户/对象级数据范围、前后端授权强制与审计拒绝路径尚未实现；当前工作台导航分区不是权限边界。第 25～30 节先形成资格规则、决策引擎与 Activity 等受保护对象，第 31～35 节再以公共模型、真实会话、服务端强制、前端感知和越权验收逐步建立统一访问控制，第 36 节首个真实运营后台复用它。
- 服务拆分、RPC 和注册中心延迟至第 78 节以后。
- 最终目录图和 ER 图延迟至第 101 节复盘。
