# 仓库地图

**状态：** 当前
**更新日期：** 2026-08-31

本文件描述当前仓库，而不是第 101 节的目标仓库。目录随需求演进，改动目录职责时必须同步更新本文件。

第 9～31 节依次形成工程基线、Lottery/Marketing/Participation 切片，以及 Governance-owned 访问控制语言。第 31 节新增的 `internal/governance/domain` 只包含 canonical Principal/Resource/Action、exact Permission、固定 Role ceiling、四种 ScopeKind、allow/deny RoleBinding、immutable Policy 和纯 Decision evaluator；没有 session、transport、persistence、middleware、UI 或业务 use-case enforcement。第 25～31 节新内核都未进入现有 HTTP/composition root；长期 `growthos_app` 权限也没有扩宽。

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
| `internal/lottery/domain` | Strategy/Award、WeightedSelector、会员 concrete routing、schema v1 `StrategyRoutingGraph`、封闭 evaluator，以及按 `(StrategyID, Revision)` 标识的 create-only Strategy snapshot。snapshot 固化名称与 Award 内容，不把旧 mutable Strategy 行或 cache projection 当作发布证据 | 第 17、20、27～30 节 |
| `internal/lottery/application` | 既有 Strategy/graph/evaluator 窄端口与新增 exact Strategy snapshot Creator/Reader；所有端口由消费者能力定义，不提供 latest 猜测或 CRUD 大接口，不依赖传输/SQL adapter | 第 19、21、27～30 节 |
| `internal/lottery/adapter/mysqlrepo` | Strategy、StrategyRoutingGraph 与 StrategySnapshot 三个彼此独立的手写 SQL Repository；完整聚合事务创建、RR exact 读取、严格恢复、错误分类与 shared pool 非所有权。graph/snapshot adapter 均未装配 | 第 19、21、28、30 节 |
| `internal/lottery/adapter/strategycache` | Lottery-owned StrategyReader cache-aside 装饰器：版本化投影、完整 uint64 string、2 MiB/1000 Award 边界、TTL jitter、同 key fill 合并、坏值精确删除与 fail-open；不拥有 Redis client | 第 24 节 |
| `internal/lottery/adapter/randomsource` | 以 `crypto/rand.Int` 实现均匀 `[0,upper)` 的生产随机适配器；支持完整 uint64、稳定错误与并发共享，不负责权重映射或 Draw 审计 | 第 20 节 |
| `internal/lottery/adapter/httpapi` | 注册默认关闭的 ephemeral selection route，校验规范 uint64 path、demo header/query/body framing，映射最小 string DTO、稳定错误、timeout、no-store 与 Request ID | 第 21 节 |
| `internal/participation/domain` | 持久化/传输无关的 ParticipantRef、注册/风险事实快照、具体新用户/风险准入 policy 与 decision、RuleSetRevision、稳定 reason 和纯 evaluator | 第 25～26 节 |
| `internal/participation/application` | consumer-owned registration/risk fact ports、受控 Clock、standalone 新用户用例，以及固定“新用户→风险”短路链；验证 shared as-of、freshness、取消、单类错误与 trace，不依赖 adapter/HTTP/Lottery | 第 25～26 节 |
| `internal/governance/domain` | 统一且封闭的访问策略语言：Principal、Resource kind/type/facts、exact Permission、五种 Role template ceiling、system/tenant/owned/resource Scope、allow/deny binding、immutable Policy revision、default deny/deny precedence 与有界 evidence；构造只验证 shape，当前无 runtime consumer | 第 31 节 |
| `internal/marketing/domain` | Marketing-owned Activity aggregate、draft/published/retired 状态机、immutable publication version、exact graph/snapshot manifest、rollback provenance、CAS plan 与 `[start,end)` resolve gate；不拥有 Lottery 内容或 Governance 审批事实 | 第 30 节 |
| `internal/marketing/application` | CreateDraft/Publish/Rollback/Retire/Resolve 用例与 narrow ports；一次 Clock、positive maxDuration、exact candidate、低披露失败。COMMIT 应答丢失时通过 `ActivityCommitReceiptFromError` 取受信 receipt，以 `ObserveCurrentActivity` / `ObserveActivityRoot` 和 `ReconcileActivityCommit` 形成 committed/not_committed/indeterminate 三态，不建议盲重放 | 第 30 节 |
| `internal/marketing/adapter/mysqlrepo` | Activity、publication、manifest 的 Marketing 内部事务/RR/CAS Repository；publication history 追加且 rollback 复制 exact source，adapter 不跨上下文读 Lottery 表、不拥有共享 pool | 第 30 节 |
| `internal/marketing/adapter/lotteryconfig` | Marketing consumer-owned Lottery ACL：exact 读取 graph 与每个 Strategy snapshot，要求 terminal Strategy revision 集与 publication manifest 精确相等；不猜 latest、不做跨上下文 SQL join | 第 30 节 |
| `internal/*` | Lottery、Participation 之外仍预留的私有领域与基础设施边界；占位不代表实现 | 随对应领域章节引入 |
| `pkg` | 可被外部导入的少量稳定 Go 包 | 仅在确有跨模块公共契约时 |
| `configs/growth-api.env.example` | 不自动加载且不给密码赋值的 API/Migration 公开环境变量示例 | 第 12～13 节 |
| `migrations/embed.go` | 编译期嵌入 Migration 说明与 `sql/` source；迁移字节通过 hash 测试防止已发布历史被静默改写 | 第 13、18、28、30 节 |
| `migrations/sql` | 严格命名的前向 `.up.sql`；`000001`～`000005` 保留旧五表，`000006` / `000007` 新增 Strategy snapshot 两表，`000008`～`000010` 新增 Activity publication 三表，`000011` 追加 Marketing 内部 active-publication 反向外键；源码 latest 11、总计十张业务表 | 第 13 节机制，第 18、28、30 节业务结构 |
| `migrations/lottery_schema_integration_test.go` | 只在显式授权的隔离 schema 上，以 legacy writer 测试身份验证旧两表结构、Repository 所需 SELECT/INSERT、负向权限与回滚清理；不定义当前运行账号权限 | 第 18～19、28 节回归 |
| `migrations/strategy_routing_graph_schema_integration_test.go` | 在同一可丢弃 schema 上验证 graph 三表精确列/FK/CHECK/collation、latest 5、跨 revision/大小写/空白边界、显式回滚和 legacy writer 测试身份对 graph 表零权限 | 第 28 节 |
| `migrations/activity_publication_schema_integration_test.go` | 真实 MySQL 8.4.11 验证 v5 七行 FK 基线到 v11 的旧表结构/数据哈希保持、五张新表/6 个 RESTRICT FK/20 个 CHECK/binary collation、Marketing→Lottery 零 FK、dirty/restore、repeat no_change 与隔离权限 | 第 30 节 |
| `scripts/generate-compose-secrets.sh` | 完整 Secret 集合生成/验证；部分集合与“旧 MySQL volume + 缺凭据”状态 fail closed，阻止静默错配 | 第 16 节 |
| `scripts/compose-smoke.sh` | 已在长期 v11 栈验证十表、旧两表 SELECT-only、八表拒绝、Redis 网络/Secret/ACL/内存策略、探针/HTTP 契约与端口隔离；对 MySQL/业务事实保持只读 | 第 16、18～19、21、24、28、30 节 |
| `scripts/compose-lottery-api-acceptance.sh` | 以随机 project/secret/volume/image 在 v11 schema 回归 Lottery/cache/fault/M1 与八表拒绝，并完成 project/卷/网络/镜像/builder/Secret/响应精确清理 | 第 21、24、28、30 节回归 |
| `scripts/lesson28-mysql-acceptance.sh` | 以随机 label/name、回环动态端口、tmpfs 数据和任务 Secret 启动一次性 MySQL 8.4.11，分别授予 legacy writer 旧两表与 graph repository 新三表 `SELECT, INSERT`，运行六组 Integration，并核对临时零残留和长期 `growthos` Docker 快照不变 | 第 28 节 |
| `scripts/lesson30-mysql-acceptance.sh` | 以确认口令、随机 label/name、tmpfs MySQL 8.4.11 与隔离身份验证真实 v5 七行 FK 基线→v11、五张新表、snapshot/Marketing Repository、最小权限、dirty/restore 与精确清理 | 第 30 节 |
| `deploy/compose` | Web/API/MySQL/Redis 四常驻与 Migrate/mysql-grants 两 one-shot；长期 v5→v11 原地升级已保持 MySQL/Redis/Web/网络/卷 identity，旧表零行/checksum 不变、新表为空，且不装配 graph/snapshot/Marketing adapter | 第 16、18～19、21、24、28、30 节 |
| `deploy/compose/mysql/grants` | 只经 MySQL Unix socket、`network_mode: none` 收敛应用授权；长期 `growthos_app` 只允许旧两表 `SELECT`，并已真实验证 graph/snapshot/Marketing 八表 1142 拒绝及 mandatory role 为空 | 第 18～19、21、28、30 节 |
| `deploy/docker` | API/Migrator/Web/Redis 构建边界、受限 Go 编译并行/内存、非 root 运行入口、Redis 最小 ACL/48 MiB allkeys-lru，以及限制 Host/framing/size/timeout/request ID 的 Nginx 同源网关 | 第 16、21、24 节 |
| `web` | 统一 React 用户端、运营端、MCP 与 AI Operator 框架；系统状态与 ephemeral Lottery 已真实联调，其余业务页面为带边界说明的 Mock/本地交互 | 第 14～15、22 节 |
| `web/src/api` | 只访问同源路径的 HTTP client、六类失败语义、bodyless POST、运行时 decoder，以及 system/lottery API adapters | 第 15、22 节 |
| `web/src/components/layout/WorkspaceShell.tsx` | 四类工作台共享的桌面侧栏、移动抽屉、顶栏、搜索、通知样例、主题、全宽和内容几何；不承担认证或授权判断 | 第 22 节 |
| `web/src/pages/user/lottery` | `/lottery` 页面与 selection Hook：规范 StrategyID、显式状态机、pending 抑制、取消和旧响应隔离；不产生浏览器随机结果 | 第 22 节 |
| `web/src/pages/system/status` | `/system/status` 页面、并行探针 hook、取消/竞态控制和组件测试 | 第 15 节 |
| `web/vite.config.ts` | dev/preview 的 `127.0.0.1` 严格端口和 `/health`、`/ready`、`/api` 精确同源代理 | 第 15 节 |
| `docs` | 产品、架构、决策、QA、第一性原理设计推导、面试问答和课程事实；第 31 节增加访问模型基线、零 API 契约、威胁矩阵与信任停止线 | 全程 |
| `docs/design-thinking` | 按章节保存事实到机制的推导、备选方案、失败模型、风险账本与重决策条件 | 第 13 节起，历史章节回填 |
| `docs/interview` | 按章节保存可口述问答、追问、项目证据、选型边界与分级外部来源 | 第 13 节起，历史章节回填 |
| `docs/runbooks` | MySQL/Compose/Redis/graph/Activity 的运维验收，以及第 31 节 access catalog/role/scope/deny 模型变更、离线撤权分析和证据纪律 | 第 13、16、24、28～31 节 |

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
15. Marketing publication 与 Lottery graph/snapshot 不建立跨 bounded-context FK 或 SQL join。Marketing 只保存 exact refs，`lotteryconfig` ACL 通过 Lottery application-owned readers 复核闭合集；任一缺失、坏快照、集合不等或依赖故障均失败关闭，不能回退 latest。
16. Activity publication 只追加；publish/rollback/retire 必须以 `state_version` CAS 拒绝并发覆盖。Governance 拥有审批决定，Marketing 只消费绑定 exact candidate 的 verifier evidence；第 30 节没有装配真实 Governance provider，也没有授权任何浏览器或 runtime 调用。
17. Governance domain 只允许经评审的纯标准库依赖；其他 production Go 文件在第 31 节不得导入它或未来子包。Principal/Resource 构造不是信任证明，Permission 不直接绑定 Principal，unknown capability/scope 和缺失 tenant/owner 均 fail closed；第 33 节只能在可信事实与明确 enforcement point 完成后修改停止线。

第 11 节的 `/health` 仍是无外部依赖的进程 liveness，只证明 Gin 路由和 handler 能响应。第 13 节的 API 在监听前必须打开并 Ping MySQL，运行中 `/ready` 每次用有界 Ping 表示数据库 readiness；依赖故障时 `/ready` 为 503 而 `/health` 仍可为 200。两者都不证明业务数据正确、Migration 最新或 SLO 达标。

第 12 节保持 `request_id` 与未来 OpenTelemetry `trace_id` 分离；fault 平台层不导入 Gin/HTTP，只有 HTTP adapter 决定 status 和公开 error envelope。配置与隐私规则见[配置参考](../configuration.md)，长期边界见[ADR-0009](../decisions/ADR-0009-runtime-boundaries.md)。

第 13 节保持 API 与 Migration 身份和进程分离。第 30 节把 source latest 追加到 11：旧 `000001`～`000005` 不回写，`000006`～`000010` 新增五表，`000011` 只补 Marketing 内部 FK。真实一次性 MySQL 与长期 Compose 都已验证 v5→v11；长期 `growthos_app` 仍精确只有旧两表 `SELECT`，对其余八表均真实 1142 拒绝。边界见 [ADR-0010](../decisions/ADR-0010-mysql-migration-boundaries.md)、[ADR-0024](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)与 [ADR-0026](../decisions/ADR-0026-activity-publication-binding.md)，操作步骤见 [MySQL Migration 运维手册](../runbooks/mysql-migrations.md)。

第 15 节的系统状态页通过 Vite dev/preview 同源代理真实消费 `GET /health` 与 `GET /ready`，统一 client 做运行时 JSON 契约检查，并由状态 hook 管理并行请求、取消和过期结果。Go 统一错误响应明确携带 `Cache-Control: no-store`。正常、数据库不可用和 API 离线场景已做真实浏览器关联验收，但这些证据不代表吞吐、长期可用性或生产 SLO 已验证。前端工具链要求 Node.js `>=22.22.2`、pnpm `10.13.1`，质量门包含 `test`、`typecheck` 和 `build`。

第 24 节的 Compose 拓扑仍只发布 `127.0.0.1:8088` 的 Nginx Web 入口，启动门仍为 `mysql healthy → migrate exited 0 → mysql-grants exited 0 → API`，Redis 不成为 API 启动或 readiness authority。第 30 节没有装配任何新 service/adapter，也没有扩宽 `growthos_app`；长期栈 v11 status/smoke 已证明仅旧两表 `SELECT`、其余八表拒绝和 mandatory role 为空。长期边界见 [ADR-0012](../decisions/ADR-0012-compose-development-topology.md)、[ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)与 [ADR-0026](../decisions/ADR-0026-activity-publication-binding.md)。

第 17～29 节形成 Strategy/Award、事务仓储、ephemeral API/React、cache-aside、资格/路由和 exact graph evaluator。第 30 节由 Lottery create-only snapshot 固化 Strategy 内容，由 Marketing Activity publication 保存 exact graph + closed terminal snapshot manifest；publish/rollback/retire 使用 CAS，rollback 追加版本并保留 provenance，resolve 在一次 Clock 下执行生命周期与 `[start,end)` gate。两上下文之间没有 FK，Lottery ACL 只走 exact readers 并 fail closed；commit acknowledgement 丢失则以 application-owned receipt 与 exact observation 三态对账。以上代码仍未装配进现有 ephemeral selection，运行链没有 DrawID、结果持久化、认证、授权、幂等、完整资格组合、库存或发奖，INV-03 尚未满足。

第 31 节在这些真实受保护对象出现后才建立公共授权词典。它能对 exact Principal/Resource/Action 在 exact Policy revision 下形成 confirmed allow/deny 或 zero Decision + error，matching deny 确定覆盖 allow，并保留 BindingID/RoleID/Effect/ScopeKind/Permission evidence。架构门禁仍使 runtime import 为零，所以现有 ephemeral route 仍无主体和授权；第 32/33 节之前不能把构造出的值对象当认证或服务端事实。

第 29 节保留其普通/race/fuzz/coverage 与 disposable MySQL 证据。第 30 节最终源码/文档候选又实际通过 `make verify`、真实 disposable MySQL 8.4.11、长期 Compose status/smoke 与独立 Lottery acceptance；这证明当前未装配内核和工程边界，不证明业务 SLO、运行时 Activity API 或生产容量。

第一版运行时采用 [ADR-0007](../decisions/ADR-0007-modular-monolith-first.md) 确定的模块化单体：一个 Go 产品进程可以装配多个领域模块，但共享进程和数据库实例不改变事实所有权。服务拆分必须等待第 78 节起出现的负载、发布、故障域、合规或团队证据。

## 有意延迟的决定

- Gin 在第 11 节作为 Go HTTP 基线接入；gRPC + Protobuf 是后续服务间 RPC 基线，到第 80 节再按拆分需求接入。
- 第 12 节已把监听地址、HTTP timeout、日志级别和格式纳入显式配置，并建立请求关联与统一错误。
- MySQL 连接、`sqlx` pool 与前向 Migration 机制已在第 13 节接入，第 18～19 节建立首组 Lottery 表与 Strategy Repository；第 28 节增加 create-only graph/node/edge 三表和 graph Repository。两类读取都用只读 RR 快照；graph 更新、删除、发布/active revision 与精准缓存失效仍等待真实用例。
- React、TypeScript、Vite、Tailwind CSS、Lucide、Recharts 和 Zustand 在第 14 节接入；第 15 节接通系统探针，第 22 节接通 Lottery ephemeral selection 并收敛共享工作台壳层。其余领域仍等待真实 API，不以 Mock 或本地交互冒充完成。
- 第 16 节已引入 Compose 本地开发环境，第 24 节接入第一个业务 Redis 消费者和最小 ACL；它仍是单机开发拓扑：没有镜像 digest 固定、内部 TLS、生产资源配额、Secret Manager、Redis HA 或生产容量证明。
- 第 19～29 节逐步建立 Strategy 仓储、无偏选择、ephemeral API/React、规则所有权、Redis 投影、Participation 资格、会员路由、routing graph 和 closed evaluator；第 30 节已验收 exact Strategy snapshot 与 Activity publication。以上新增内核仍未在线执行；正式 Draw/Result、幂等、库存与发奖必须由后续真实问题驱动，runtime 新表 grant、运行写权限和缓存失效总线不会被提前加入。
- 公共访问控制策略模型已在第 31 节实现，但登录认证、真实会话、Policy/assignment repository、租户/对象事实装配、服务端强制、前端权限投影和审计拒绝路径仍未实现；当前工作台导航不是权限边界。第 32～35 节按会话、服务端强制、前端感知和越权验收继续闭环，第 36 节首个真实运营后台再复用它。
- 服务拆分、RPC 和注册中心延迟至第 78 节以后。
- 最终目录图和 ER 图延迟至第 101 节复盘。
