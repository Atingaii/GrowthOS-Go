# 仓库地图

**状态：** 当前
**更新日期：** 2026-08-30

本文件描述当前仓库，而不是第 101 节的目标仓库。目录随需求演进，改动目录职责时必须同步更新本文件。

第 9～24 节依次形成 Gin/MySQL/React/Compose 工程基线及 Lottery 领域、两表、仓储、选择、ephemeral API/React 和 Redis Strategy 投影。第 25～26 节在 Participation 中建立新用户/风险资格与固定短路链；第 27 节在 Lottery 中以具体会员路由证明线性 chain 的分支局限；第 28 节再增加 Lottery-owned bounded immutable rooted DAG、三个前向 Migration、聚合级端口和未装配的 graph MySQL adapter；第 29 节现已以封闭 typed dispatch 对 exact graph revision 做有界、确定的单路径求值。当前已有内部 graph evaluator，但仍没有 graph 发布/Activity 绑定、runtime 装配、生产 fact adapter、正式 Draw/Result、登录认证、RBAC/对象级授权、在线资格门控、库存或发奖；graph Repository/evaluator 与 Participation 代码均没有 HTTP 或 composition-root 装配，其他工作台业务数据仍为明确 Mock/本地状态。

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
| `internal/lottery/domain` | Strategy/Award、WeightedSelector、会员 concrete routing 与 schema v1 `StrategyRoutingGraph`；graph 以 `(GraphID, Revision)` 标识 bounded immutable rooted DAG，限制 128 nodes、256 edges、16 edges depth，并规范排序、防御性复制、严格验证 root/可达/环/branch/default/terminal。第 29 节追加 `1..16` step budget、immutable decision/path 与迭代 single-path evaluator，共享 package-private 会员 tier-to-branch oracle，按 exact branch 求值并在所有失败上返回 zero decision | 第 17、20、27～29 节 |
| `internal/lottery/application` | `StrategyCreator` / `StrategyReader`、会员 fact reader/router、graph-specific Create/Find 窄端口，以及未装配的 `StrategyRoutingGraphEvaluationService`。求值服务按 exact identity 只读一次 graph，在 worst-depth 准入后只取一次 Clock 和一次会员 fact；child deadline 协作约束 context-aware reader/检查点，Clock 则必须是有界本地调用，并在依赖/执行边界明确 caller/internal/provider 错误优先级与低披露语义；不依赖传输或 SQL adapter | 第 19、21、27～29 节 |
| `internal/lottery/adapter/mysqlrepo` | 两个彼此独立的手写 SQL Repository：Strategy 父子聚合和 StrategyRoutingGraph header/nodes/edges 均事务 Create、只读 RR snapshot Find；graph reader 使用 129/257 sentinel 有界读取并在恢复时重验完整领域不变量；adapter 不拥有共享 pool，graph adapter 未装配 | 第 19、21、28 节 |
| `internal/lottery/adapter/strategycache` | Lottery-owned StrategyReader cache-aside 装饰器：版本化投影、完整 uint64 string、2 MiB/1000 Award 边界、TTL jitter、同 key fill 合并、坏值精确删除与 fail-open；不拥有 Redis client | 第 24 节 |
| `internal/lottery/adapter/randomsource` | 以 `crypto/rand.Int` 实现均匀 `[0,upper)` 的生产随机适配器；支持完整 uint64、稳定错误与并发共享，不负责权重映射或 Draw 审计 | 第 20 节 |
| `internal/lottery/adapter/httpapi` | 注册默认关闭的 ephemeral selection route，校验规范 uint64 path、demo header/query/body framing，映射最小 string DTO、稳定错误、timeout、no-store 与 Request ID | 第 21 节 |
| `internal/participation/domain` | 持久化/传输无关的 ParticipantRef、注册/风险事实快照、具体新用户/风险准入 policy 与 decision、RuleSetRevision、稳定 reason 和纯 evaluator | 第 25～26 节 |
| `internal/participation/application` | consumer-owned registration/risk fact ports、受控 Clock、standalone 新用户用例，以及固定“新用户→风险”短路链；验证 shared as-of、freshness、取消、单类错误与 trace，不依赖 adapter/HTTP/Lottery | 第 25～26 节 |
| `internal/*` | Lottery、Participation 之外仍预留的私有领域与基础设施边界；占位不代表实现 | 随对应领域章节引入 |
| `pkg` | 可被外部导入的少量稳定 Go 包 | 仅在确有跨模块公共契约时 |
| `configs/growth-api.env.example` | 不自动加载且不给密码赋值的 API/Migration 公开环境变量示例 | 第 12～13 节 |
| `migrations/embed.go` | 编译期嵌入 Migration 说明与 `sql/` source；迁移字节通过 hash 测试防止已发布历史被静默改写 | 第 13、18、28 节 |
| `migrations/sql` | 严格命名的前向 `.up.sql`；`000001` / `000002` 建 Strategy/Award，`000003` / `000004` / `000005` 建 routing graph/node/edge，当前源码 latest 为 5 | 第 13 节机制，第 18、28 节业务结构 |
| `migrations/lottery_schema_integration_test.go` | 只在显式授权的隔离 schema 上，以 legacy writer 测试身份验证旧两表结构、Repository 所需 SELECT/INSERT、负向权限与回滚清理；不定义当前运行账号权限 | 第 18～19、28 节回归 |
| `migrations/strategy_routing_graph_schema_integration_test.go` | 在同一可丢弃 schema 上验证 graph 三表精确列/FK/CHECK/collation、latest 5、跨 revision/大小写/空白边界、显式回滚和 legacy writer 测试身份对 graph 表零权限 | 第 28 节 |
| `scripts/generate-compose-secrets.sh` | 完整 Secret 集合生成/验证；部分集合与“旧 MySQL volume + 缺凭据”状态 fail closed，阻止静默错配 | 第 16 节 |
| `scripts/compose-smoke.sh` | 四常驻/两 one-shot 状态、源码 latest 5、长期 app 仅旧两表 SELECT 且 graph 三表拒绝、Redis 网络/Secret/ACL/内存策略、探针/ephemeral 404、HTTP 契约与端口隔离；对 MySQL/业务事实只读 | 第 16、18～19、21、24、28 节 |
| `scripts/compose-lottery-api-acceptance.sh` | 以随机 project/secret/volume/image 创建一次性真实纵向环境，验证 latest 5、长期 app 对 graph 三表拒绝，以及 Lottery/cache success/failure、ACL、poison、自愈、故障恢复、M1 三基线、数据指纹与所有权清理 | 第 21、24、28 节回归 |
| `scripts/lesson28-mysql-acceptance.sh` | 以随机 label/name、回环动态端口、tmpfs 数据和任务 Secret 启动一次性 MySQL 8.4.11，分别授予 legacy writer 旧两表与 graph repository 新三表 `SELECT, INSERT`，运行六组 Integration，并核对临时零残留和长期 `growthos` Docker 快照不变 | 第 28 节 |
| `deploy/compose` | Web/API/MySQL/Redis 四个常驻服务，Migrate/mysql-grants 两个 one-shot，三张隔离网络、MySQL data/socket named volume、API/Redis 文件秘密、ephemeral/cache 配置和仅回环 Web 端口；第 28 节镜像/授权门升级到 v5 且不装配 graph adapter | 第 16、18～19、21、24、28 节 |
| `deploy/compose/mysql/grants` | 只经 MySQL Unix socket、`network_mode: none` 运行的应用授权收敛脚本；只允许旧两张 Lottery 表 `SELECT`，显式拒绝 graph 三表且 mandatory role 非空时失败关闭 | 第 18～19、21、28 节回归 |
| `deploy/docker` | API/Migrator/Web/Redis 构建边界、受限 Go 编译并行/内存、非 root 运行入口、Redis 最小 ACL/48 MiB allkeys-lru，以及限制 Host/framing/size/timeout/request ID 的 Nginx 同源网关 | 第 16、21、24 节 |
| `web` | 统一 React 用户端、运营端、MCP 与 AI Operator 框架；系统状态与 ephemeral Lottery 已真实联调，其余业务页面为带边界说明的 Mock/本地交互 | 第 14～15、22 节 |
| `web/src/api` | 只访问同源路径的 HTTP client、六类失败语义、bodyless POST、运行时 decoder，以及 system/lottery API adapters | 第 15、22 节 |
| `web/src/components/layout/WorkspaceShell.tsx` | 四类工作台共享的桌面侧栏、移动抽屉、顶栏、搜索、通知样例、主题、全宽和内容几何；不承担认证或授权判断 | 第 22 节 |
| `web/src/pages/user/lottery` | `/lottery` 页面与 selection Hook：规范 StrategyID、显式状态机、pending 抑制、取消和旧响应隔离；不产生浏览器随机结果 | 第 22 节 |
| `web/src/pages/system/status` | `/system/status` 页面、并行探针 hook、取消/竞态控制和组件测试 | 第 15 节 |
| `web/vite.config.ts` | dev/preview 的 `127.0.0.1` 严格端口和 `/health`、`/ready`、`/api` 精确同源代理 | 第 15 节 |
| `docs` | 产品、架构、决策、QA、第一性原理设计推导、面试问答和课程事实；第 28 节增加 Strategy routing graph/latest 5/隔离 MySQL 证据，第 29 节增加 closed evaluator、预算/取消/错误优先级、运维停止线与求职可口述证据 | 全程 |
| `docs/design-thinking` | 按章节保存事实到机制的推导、备选方案、失败模型、风险账本与重决策条件 | 第 13 节起，历史章节回填 |
| `docs/interview` | 按章节保存可口述问答、追问、项目证据、选型边界与分级外部来源 | 第 13 节起，历史章节回填 |
| `docs/runbooks` | MySQL Migration、本地 Compose、Redis Strategy 缓存与未装配 graph evaluator 的运行/故障停止条件、恢复和数据清理纪律；含第 28 节 disposable MySQL/长期 v5 验收及第 29 节 exact identity、1/1/1 调用与 timeout/cancel 核查 | 第 13、16、24、28～29 节 |

## 当前依赖规则

1. `cmd/*` 只做装配和进程生命周期管理。
2. `internal/<domain>` 拥有自己的领域模型、应用用例和端口。
3. 领域模块不得直接依赖另一个模块的数据库实现。
4. `internal/infrastructure` 提供跨领域的数据库连接、Migration、HTTP 等平台能力；`internal/<domain>/adapter` 实现领域端口的技术适配器，两者都不拥有领域规则。
5. `pkg` 不是杂物目录；不稳定或仅仓库内部使用的代码留在 `internal`。
6. 当前 `.gitkeep` 只表示计划边界，不代表能力已经实现。
7. 浏览器 API 适配只接受同源路径；开发代理目标由仅 Vite 进程读取的 `GROWTHOS_WEB_API_PROXY_TARGET` 配置，默认 `http://127.0.0.1:8080`，不向浏览器暴露后端 origin。
8. `internal/lottery/domain` 不导入 Gin、SQL/sqlx、Redis、`crypto/rand` 或 JSON/DB tag；MySQL adapter 只能通过 `RestoreAward` / `RestoreStrategy` / `RestoreStrategyRoutingGraph` 重建合法聚合，不能 trim、补边或静默修复存量事实；randomsource adapter 依赖 domain-owned 窄端口，依赖方向不能反转。
9. application 层端口按消费者能力拆分；ephemeral selection 只依赖 `StrategyReader` 与 Award selector，graph persistence 只暴露 Create/FindByIdentity；两类端口不得合成 CRUD 大接口，adapter 也不反向进入 domain/application。
10. Lottery HTTP DTO 不复用 domain struct；公开 ID 使用规范十进制 string，weight、Strategy 名称与内部错误不进入 selection 响应。
11. 业务规则跟随权威事实所有者和业务阶段，不建立跨 Activity、Participation、Lottery、Benefit 与 Governance 的万能 `common/rules`；通用执行原语只有在至少两个真实消费者证明同语义后，才由消费方反推。
12. Redis 是 adapter 依赖而不是领域事实：`strategycache` 装饰 application-owned `StrategyReader`，MySQL reader 仍是权威来源；HTTP/application/domain 不获得 Redis 命令或 key 能力。
13. `internal/participation/domain` 不导入其他项目包；application 只可依赖本上下文 domain。用户目录拥有注册原始事实，Participation 只通过 consumer-owned `RegistrationFactReader` 获取受控快照；在真实 provider、会话主体和 Activity 出现前，不得越过边界读用户表或把 `ParticipantRef` 当成已认证 Principal。
14. `StrategyRoutingGraph` 只拥有 Lottery 的 rule/branch/default/Strategy target 拓扑；它不是跨 Participation/Governance/Benefit 的通用规则引擎。第 29 节 evaluator 只消费已验证 graph aggregate 和 typed 会员 fact，不得引入裸 SQL row、`map[string]any`、registry/DSL 或跨上下文事实袋。Graph MySQL adapter/evaluator 均尚未装配到 `cmd/growth-api`，存在代码和测试不等于长期 runtime 获得 graph 表权限。

第 11 节的 `/health` 仍是无外部依赖的进程 liveness，只证明 Gin 路由和 handler 能响应。第 13 节的 API 在监听前必须打开并 Ping MySQL，运行中 `/ready` 每次用有界 Ping 表示数据库 readiness；依赖故障时 `/ready` 为 503 而 `/health` 仍可为 200。两者都不证明业务数据正确、Migration 最新或 SLO 达标。

第 12 节保持 `request_id` 与未来 OpenTelemetry `trace_id` 分离；fault 平台层不导入 Gin/HTTP，只有 HTTP adapter 决定 status 和公开 error envelope。配置与隐私规则见[配置参考](../configuration.md)，长期边界见[ADR-0009](../decisions/ADR-0009-runtime-boundaries.md)。

第 13 节保持 API 与 Migration 身份和进程分离：`growth-api` 使用受限 pool 且不执行 DDL，`growth-migrate` 使用专用单连接且只提供前向 `up/status`。第 18 节 source 到 latest 2；第 28 节只追加 `000003 graph -> 000004 node -> 000005 edge`，当前源码 latest 为 5。默认长期 Compose 已在同一 MySQL container 与原 named resources 上从 `2:0` 前向到 `5:0`，旧两表指纹不变、新三表为空，并通过 smoke；隔离 Lottery/cache acceptance 也在 v5 重跑通过且完整清理。第 19 节隔离 legacy writer 验证旧两表 `SELECT, INSERT`；第 28 节隔离 graph repository 身份只验证新三表 `SELECT, INSERT`。长期 `growthos_app` 仍精确只有旧两表 `SELECT`，对 graph 三表的真实读取为 MySQL 1142，对写操作、DDL 和 `schema_migrations` 也无权限。边界见 [ADR-0010](../decisions/ADR-0010-mysql-migration-boundaries.md)、[ADR-0016](../decisions/ADR-0016-lottery-repository-boundaries.md)、[ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)和 [ADR-0024](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)，操作步骤见 [MySQL Migration 运维手册](../runbooks/mysql-migrations.md)。

第 15 节的系统状态页通过 Vite dev/preview 同源代理真实消费 `GET /health` 与 `GET /ready`，统一 client 做运行时 JSON 契约检查，并由状态 hook 管理并行请求、取消和过期结果。Go 统一错误响应明确携带 `Cache-Control: no-store`。正常、数据库不可用和 API 离线场景已做真实浏览器关联验收，但这些证据不代表吞吐、长期可用性或生产 SLO 已验证。前端工具链要求 Node.js `>=22.22.2`、pnpm `10.13.1`，质量门包含 `test`、`typecheck` 和 `build`。

第 24 节的 Compose 拓扑仍只发布 `127.0.0.1:8088` 的 Nginx Web 入口。Web/API 位于 `edge`，API/MySQL/Migrator 位于内部 `data`，只有 API/Redis 位于 Docker-internal `cache`；启动门仍为 `mysql healthy → migrate exited 0 → mysql-grants exited 0 → API`，Redis 不成为 API 启动或 readiness authority。第 28 节不装配 graph Repository，只把当前源码迁移目标提升到 latest 5，并让授权/smoke 明确证明 runtime 对新三表零权限。`mysql-grants` 使用 MySQL 官方客户端镜像、非 root UID 999、只读根文件系统和共享 socket，`network_mode: none`，精确撤销旧授权后仅授予旧两张业务表 `SELECT`，并要求 `@@GLOBAL.mandatory_roles` 为空；它不挂入 data 网络。Redis 默认用户关闭，业务 ACL 允许无 key 的 `PING`，并只允许对版本化前缀执行 `GETRANGE/SET/DEL`；实例固定 48 MiB、`allkeys-lru`、无持久化。API、Migrator、Web、Redis 使用非 root、只读根文件系统、去除 capabilities 与 `no-new-privileges`，MySQL 官方镜像则保留初始化阶段 root 和可写数据目录这一明确例外。Nginx 动态解析 API 容器地址，统一回写 `X-Request-ID`，并在 API location 约束本地 Host、请求 framing、16 KiB 上限和 timeout。长期边界见 [ADR-0012](../decisions/ADR-0012-compose-development-topology.md)、[ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)、[ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)和 [ADR-0024](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)，操作步骤见 [Docker Compose](../runbooks/local-compose.md)与 [Redis 缓存运维手册](../runbooks/redis-strategy-cache.md)。

第 17～24 节形成 Strategy/Award、事务仓储、无偏选择、ephemeral API/React 和 cache-aside；第 25～26 节另在 Participation 中验证新用户/风险资格，但没有装配进 Lottery；第 27 节以会员快照把 confirmed premium/standard 映射到显式 override/default target。第 28 节把这份 Lottery 拓扑提升为 schema v1 rooted DAG：create/restore 都要求唯一显式 decision root、全可达、无环、terminal 无出边、每个 decision 精确 premium/default 两边，并限制 128/256/16；三表只表达数据库可靠承担的复合 scope、引用、局部枚举与唯一性，root/可达/环/深度仍由完整恢复后的领域校验承担。Repository create-only 绑定 `(GraphID, Revision)` 与内容，FindByIdentity 使用只读 RR 快照。第 29 节在该 aggregate 之上实现了内部 closed evaluator：只读 exact revision 一次，先做 `Depth() <= maxSteps` 最坏路径准入，再以一次 Clock 和一次会员 fact 完成确定的 exact-branch 迭代遍历；每步受 `1..16` hard stop，context-aware reader 和 evaluator 检查点受 positive maxDuration/caller cancellation 协作约束，Clock 必须是有界本地调用。任一失败只返回 zero decision/path。Graph Repository/evaluator 都仍未发布、未绑定 Activity、未装配进现有 ephemeral selection。整条运行链仍没有 DrawID、结果持久化、认证、授权、幂等、完整资格组合、库存或发奖，INV-03 尚未满足。边界见 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)、[ADR-0019](../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)、[ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)、[ADR-0021](../decisions/ADR-0021-participation-new-user-eligibility.md)、[ADR-0023](../decisions/ADR-0023-membership-strategy-routing-boundary.md)、[ADR-0024](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)和 [ADR-0025](../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)。

第 29 节最终候选已实际通过 `make verify` 与全仓 `go test -race -count=1 ./...`，前端 19/19 个 test files、152/152 个 tests、typecheck 和 build 也通过。Lottery domain/application 的 atomic coverage 分别为 93.6%/88.3%，合并口径 92.1%；独立 10 秒 evaluator fuzz 通过 2,899,250 次执行，新发现 1 个 interesting input（总数 43）。第 28 节 MySQL 8.4.11 disposable acceptance 也以六组 Integration 重跑通过，只能证明 schema/Repository 上游回归，不能证明 evaluator 已真实装配或达成业务 SLO。这些数字只属于本次候选，不是对未来提交或生产能力的预写。

第一版运行时采用 [ADR-0007](../decisions/ADR-0007-modular-monolith-first.md) 确定的模块化单体：一个 Go 产品进程可以装配多个领域模块，但共享进程和数据库实例不改变事实所有权。服务拆分必须等待第 78 节起出现的负载、发布、故障域、合规或团队证据。

## 有意延迟的决定

- Gin 在第 11 节作为 Go HTTP 基线接入；gRPC + Protobuf 是后续服务间 RPC 基线，到第 80 节再按拆分需求接入。
- 第 12 节已把监听地址、HTTP timeout、日志级别和格式纳入显式配置，并建立请求关联与统一错误。
- MySQL 连接、`sqlx` pool 与前向 Migration 机制已在第 13 节接入，第 18～19 节建立首组 Lottery 表与 Strategy Repository；第 28 节增加 create-only graph/node/edge 三表和 graph Repository。两类读取都用只读 RR 快照；graph 更新、删除、发布/active revision 与精准缓存失效仍等待真实用例。
- React、TypeScript、Vite、Tailwind CSS、Lucide、Recharts 和 Zustand 在第 14 节接入；第 15 节接通系统探针，第 22 节接通 Lottery ephemeral selection 并收敛共享工作台壳层。其余领域仍等待真实 API，不以 Mock 或本地交互冒充完成。
- 第 16 节已引入 Compose 本地开发环境，第 24 节接入第一个业务 Redis 消费者和最小 ACL；它仍是单机开发拓扑：没有镜像 digest 固定、内部 TLS、生产资源配额、Secret Manager、Redis HA 或生产容量证明。
- 第 19～29 节逐步建立 Strategy 仓储、无偏选择、受限 ephemeral API/React、规则所有权、Redis 读取投影、Participation 两节点资格、具体会员路由、持久化 routing graph 和内部 closed evaluator。内部 evaluator 的存在不等于发布或在线执行：第 30 节才引入 Activity 与 exact revision 绑定；正式 Draw/Result、幂等、库存与发奖仍必须由后续真实问题驱动，runtime graph grant、运行写权限和缓存失效总线不会因“常见架构”被提前加入。
- 登录认证、RBAC、租户/对象级数据范围、前后端授权强制与审计拒绝路径尚未实现；当前工作台导航分区不是权限边界。第 25 节已形成首个具体资格切片，第 26～30 节继续以真实规则、决策机制与 Activity 等受保护对象演进；第 31～35 节再以公共模型、真实会话、服务端强制、前端感知和越权验收逐步建立统一访问控制，第 36 节首个真实运营后台复用它。
- 服务拆分、RPC 和注册中心延迟至第 78 节以后。
- 最终目录图和 ER 图延迟至第 101 节复盘。
