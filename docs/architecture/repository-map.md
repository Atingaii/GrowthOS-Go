# 仓库地图

**状态：** 当前
**更新日期：** 2026-09-02

本文件描述当前仓库，而不是第 101 节的目标仓库。目录随需求演进，改动目录职责时必须同步更新本文件。

第 9～31 节依次形成工程基线、Lottery/Marketing/Participation 切片，以及 Governance-owned 访问控制语言。第 32 节候选首次装配独立 `internal/identity`：credential verification、MySQL server-side Session、Cookie/CSRF 和三方法 HTTP route 已进入 `growth-api`，业务与 Identity pool/账号互不别名。认证成功只形成 trusted human Principal；Governance Policy 尚未被 runtime 调用，业务/Admin/MCP/Agent 没有第 33 节服务端 RBAC，第 34 节权限投影和第 35 节越权 E2E 也未完成。

| 路径 | 当前职责 | 引入产品代码的章节 |
| --- | --- | --- |
| `cmd/doccheck` | 文档完整性与课程证据检查器 | 项目基线 |
| `cmd/docsync` | 项目 README/docs 到 Obsidian Vault 的单向镜像工具 | 项目基线 |
| `cmd/growth-api` | Go 产品进程入口：版本、信号、互不别名的 business/Identity MySQL pool、可选 Redis pool、双 authority readiness、Gin/HTTP、Lottery selection/cache 与 Identity Session 装配、资源关闭 | 第 11～13、21、24、32 节 |
| `cmd/growth-migrate` | 独立前向迁移入口，只暴露 `up/status`，装配 Migrator 账号、嵌入 source 与 Runner | 第 13 节 |
| `cmd/growth-identity-provision` | operations-only 本地 workforce account create 命令；只从受限文件读取 enrollment password，经独立 provisioner 身份执行 create-only INSERT | 第 32 节 |
| `cmd/growth-identity-maintenance` | operations-only 有界 Session/throttle 历史清理命令；固定 cutoff/batch/单轮语义，不开放 HTTP 或任意 SQL | 第 32 节 |
| `cmd/healthload` | 标准库定速 HTTP 负载器；输出单行 JSON，并以错误、异常状态、丢弃请求或 P99 越界作为失败 | 第 16 节 |
| `internal/platform/appconfig` | `GROWTHOS_` API/Migration/Identity operations 配置、直接/文件秘密二选一、脱敏、business/Identity/provisioner 身份分离、Cookie origin/CSRF/throttle key 与跨 pool/timeout 约束 | 第 12～13、16、21、24、32 节 |
| `internal/platform/logging` | 标准库 `slog` logger 构造、级别、格式与基础字段 | 第 12 节 |
| `internal/platform/fault` | 与传输无关的 fault kind、稳定 code、公开消息与内部 cause；包含 unavailable 语义 | 第 12～13 节 |
| `internal/infrastructure/httpapi` | Gin router、`/health`、双 MySQL authority `/ready`、请求中间件、错误映射、统一错误 `no-store` 缓存策略与 HTTP 契约测试 | 第 11～15、32 节 |
| `internal/infrastructure/httpserver` | 标准库 HTTP Server 运行、配置化 timeout、context 取消、优雅关闭与脱敏 ErrorLog 桥接 | 第 11～12 节 |
| `internal/infrastructure/mysql` | 安全 driver Config、TLS、按身份独立拥有的 API `sqlx` pool、Migration/operations 连接、首次 Ping、稳定错误 stage 与不绕过 JSON 边界的 driver logger | 第 13、16、32 节 |
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
| `internal/identity/domain` | WorkforceAccount、LocalCredential metadata、Session、AuthenticationThrottle、digest 与时间不变量；不包含 Gin/SQL/Cookie、Role/Scope/Permission 或业务资源 | 第 32 节 |
| `internal/identity/application` | login、resolve current、revoke current 与有界 maintenance 用例；consumer-owned repository/password/entropy/Clock ports，固定失败优先级与 commit outcome 语义 | 第 32 节 |
| `internal/identity/adapter/mysqlrepo` | account/session/throttle 的手写 SQL repository：事务化 admission/login/revoke、RR/锁读、digest-only lookup、epoch/status/session-cap 失效与有界清理 | 第 32 节 |
| `internal/identity/adapter/passwordhash` | 固定 profile 的严格 Argon2id envelope、constant-time compare、unknown-account dummy work 与 process-wide 有界 admission | 第 32 节 |
| `internal/identity/adapter/httpapi` | 精确注册 `POST/GET/DELETE /api/v1/session`；有界 JSON/Cookie/CSRF/Origin 输入、低披露错误、`no-store` 与最小 Session DTO | 第 32 节 |
| `internal/identity/adapter/{csrf,requestguard,sessioncookie,throttledigest,mysqlprovisioner}` | Session-bound CSRF、同源/Fetch Metadata guard、环境化安全 Cookie、HMAC throttle subject 与 create-only 账号 provision adapter | 第 32 节 |
| `internal/governance/domain` | 统一且封闭的访问策略语言：Principal、Resource kind/type/facts、exact Permission、五种 Role template ceiling、system/tenant/owned/resource Scope、allow/deny binding、immutable Policy revision、default deny/deny precedence 与有界 evidence；构造只验证 shape，当前无 runtime consumer | 第 31 节 |
| `internal/marketing/domain` | Marketing-owned Activity aggregate、draft/published/retired 状态机、immutable publication version、exact graph/snapshot manifest、rollback provenance、CAS plan 与 `[start,end)` resolve gate；不拥有 Lottery 内容或 Governance 审批事实 | 第 30 节 |
| `internal/marketing/application` | CreateDraft/Publish/Rollback/Retire/Resolve 用例与 narrow ports；一次 Clock、positive maxDuration、exact candidate、低披露失败。COMMIT 应答丢失时通过 `ActivityCommitReceiptFromError` 取受信 receipt，以 `ObserveCurrentActivity` / `ObserveActivityRoot` 和 `ReconcileActivityCommit` 形成 committed/not_committed/indeterminate 三态，不建议盲重放 | 第 30 节 |
| `internal/marketing/adapter/mysqlrepo` | Activity、publication、manifest 的 Marketing 内部事务/RR/CAS Repository；publication history 追加且 rollback 复制 exact source，adapter 不跨上下文读 Lottery 表、不拥有共享 pool | 第 30 节 |
| `internal/marketing/adapter/lotteryconfig` | Marketing consumer-owned Lottery ACL：exact 读取 graph 与每个 Strategy snapshot，要求 terminal Strategy revision 集与 publication manifest 精确相等；不猜 latest、不做跨上下文 SQL join | 第 30 节 |
| `internal/*` | Lottery、Participation 之外仍预留的私有领域与基础设施边界；占位不代表实现 | 随对应领域章节引入 |
| `pkg` | 可被外部导入的少量稳定 Go 包 | 仅在确有跨模块公共契约时 |
| `configs/growth-api.env.example`、`configs/growth-identity-*.env.example` | 不自动加载且不给秘密赋值的 API、provision、maintenance 公开配置示例 | 第 12～13、32 节 |
| `migrations/embed.go` | 编译期嵌入 Migration 说明与 `sql/` source；迁移字节通过 hash 测试防止已发布历史被静默改写 | 第 13、18、28、30 节 |
| `migrations/sql` | 严格命名的前向 `.up.sql`；历史 `000001`～`000011` 保留十张业务表，`000012`～`000014` 追加 workforce account、server-side Session 与 authentication throttle；源码 latest 14、总计十三张领域表 | 第 13 节机制，第 18、28、30、32 节业务结构 |
| `migrations/lottery_schema_integration_test.go` | 只在显式授权的隔离 schema 上，以 legacy writer 测试身份验证旧两表结构、Repository 所需 SELECT/INSERT、负向权限与回滚清理；不定义当前运行账号权限 | 第 18～19、28 节回归 |
| `migrations/strategy_routing_graph_schema_integration_test.go` | 在同一可丢弃 schema 上验证 graph 三表精确列/FK/CHECK/collation、latest 5、跨 revision/大小写/空白边界、显式回滚和 legacy writer 测试身份对 graph 表零权限 | 第 28 节 |
| `migrations/activity_publication_schema_integration_test.go` | 真实 MySQL 8.4.11 验证 v5 七行 FK 基线到 v11 的旧表结构/数据哈希保持、五张新表/6 个 RESTRICT FK/20 个 CHECK/binary collation、Marketing→Lottery 零 FK、dirty/restore、repeat no_change 与隔离权限 | 第 30 节 |
| `migrations/identity_schema_integration_test.go` | 在显式可丢弃 schema 验证 latest 14、三张 Identity 表的列/索引/FK/CHECK/collation、坏行拒绝、旧 v11 事实保持与精确测试权限 | 第 32 节 |
| `scripts/generate-compose-secrets.sh` | 完整 Secret 集合生成/验证；除既有凭据外生成彼此不同的 Identity runtime/provisioner password、throttle HMAC 与 CSRF key，部分集合和旧 volume 缺凭据均失败关闭 | 第 16、32 节 |
| `scripts/compose-smoke.sh` | 长期栈只读核对 latest 14、十三表、business/Identity/provisioner 精确 grant、Redis ACL、双 readiness、Session 匿名边界与端口隔离 | 第 16、18～19、21、24、28、30、32 节 |
| `scripts/compose-lottery-api-acceptance.sh` | 随机 project/secret/volume/image 中回归 Lottery/cache/fault/M1，并扩展 Identity schema/grant/provision/session HTTP/maintenance/故障与精确清理；第 32 节 development HTTP wire 官方门禁已通过，精确范围仍以 QA 记录为准 | 第 21、24、28、30、32 节回归 |
| `scripts/compose-identity-provision.sh` | 校验调用方私密 password snapshot，以 operations profile 运行单个 provisioner，接受 one-shot `exited:0` 并精确清理容器/快照 | 第 32 节 |
| `scripts/compose-identity-maintenance.sh` | 以 operations profile 运行固定有界 maintenance，接受 one-shot `exited:0`，不把任意 cutoff/batch 暴露给值班命令 | 第 32 节 |
| `scripts/lesson28-mysql-acceptance.sh` | 以随机 label/name、回环动态端口、tmpfs 数据和任务 Secret 启动一次性 MySQL 8.4.11，分别授予 legacy writer 旧两表与 graph repository 新三表 `SELECT, INSERT`，运行六组 Integration，并核对临时零残留和长期 `growthos` Docker 快照不变 | 第 28 节 |
| `scripts/lesson30-mysql-acceptance.sh` | 以确认口令、随机 label/name、tmpfs MySQL 8.4.11 与隔离身份验证真实 v5 七行 FK 基线→v11、五张新表、snapshot/Marketing Repository、最小权限、dirty/restore 与精确清理 | 第 30 节 |
| `scripts/lesson32-mysql-acceptance.sh` | 以确认口令、随机 name/label/回环端口和 tmpfs MySQL 8.4.11 验证 v11→v14、Identity schema/Repository/锁与会话上限、runtime grant allow/deny、migration inventory 和精确清理 | 第 32 节 |
| `deploy/compose` | Web/API/MySQL/Redis 四常驻，Migrate/mysql-grants 两启动 one-shot，并增加 profile-scoped Identity provision/maintenance；latest 14 与双 pool/secret/grant 候选不装配 graph/snapshot/Marketing | 第 16、18～19、21、24、28、30、32 节 |
| `deploy/compose/mysql/grants` | 只经 MySQL Unix socket、`network_mode: none` 精确收敛：`growthos_app` 两张业务表 SELECT，`growthos_identity` 三张 Identity 表所需 DML，`growthos_identity_provisioner` 仅 account INSERT；mandatory role 为空 | 第 18～19、21、28、30、32 节 |
| `deploy/docker` | API/Migrator/Web/Redis 与 Identity operations 构建边界、受限 Go 编译、非 root/只读运行，以及限制 Host/framing/size/timeout/request ID 的 Nginx 同源网关 | 第 16、21、24、32 节 |
| `web` | 统一 React 工作台，加上公共登录/当前会话体验；系统状态、ephemeral Lottery 与 Session 已有真实 API 消费，其余业务页面仍为有边界说明的 Mock/本地交互 | 第 14～15、22、32 节 |
| `web/src/api` | 只访问同源路径的 HTTP client、运行时 decoder、system/lottery adapters，以及严格 `sessionApi` create/current/revoke transport；Cookie 交给浏览器，CSRF 只由调用方显式传给 DELETE | 第 15、22、32 节 |
| `web/src/components/layout/WorkspaceShell.tsx` | 四类工作台共享的桌面侧栏、移动抽屉、顶栏、搜索、通知样例、主题、全宽和内容几何；不承担认证或授权判断 | 第 22 节 |
| `web/src/layouts/AuthLayout.tsx`、`web/src/layouts/useSessionBoundary.ts` | `/login` 与 `/session` 的公共壳层和 `checking/anonymous/authenticated/unavailable` 状态；使用 generation/abort guard，不保护其他业务 route | 第 32 节 |
| `web/src/pages/auth` | 真实登录、current session、不可用重试与注销页面；不展示或缓存 Role/Scope/Permission，也不构成 RBAC UI | 第 32 节 |
| `web/src/pages/user/lottery` | `/lottery` 页面与 selection Hook：规范 StrategyID、显式状态机、pending 抑制、取消和旧响应隔离；不产生浏览器随机结果 | 第 22 节 |
| `web/src/pages/system/status` | `/system/status` 页面、并行探针 hook、取消/竞态控制和组件测试 | 第 15 节 |
| `web/vite.config.ts` | dev/preview 的 `127.0.0.1` 严格端口和 `/health`、`/ready`、`/api` 精确同源代理 | 第 15 节 |
| `docs` | 产品、架构、决策、QA、第一性原理设计推导、面试问答和课程事实；第 32 节增加 Identity 基线、真实 Session API、运维手册与设计验收入口 | 全程 |
| `docs/design-thinking` | 按章节保存事实到机制的推导、备选方案、失败模型、风险账本与重决策条件 | 第 13 节起，历史章节回填 |
| `docs/interview` | 按章节保存可口述问答、追问、项目证据、选型边界与分级外部来源 | 第 13 节起，历史章节回填 |
| `docs/runbooks` | MySQL/Compose/Redis/graph/Activity 运维验收、访问模型审查，以及 Identity provision/session/maintenance/故障与秘密清理操作 | 第 13、16、24、28～32 节 |

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
18. Identity 只把 server-derived account/session 映射为 human Principal；HTTP 请求不得提交 Principal/Role/Scope/Permission。Session token 只以 digest 落库并由 HttpOnly Cookie 携带，CSRF 只驻留调用方内存；Identity 不读取 Policy 或业务资源。登录成功不能解锁业务/Admin/MCP/Agent，直到第 33 节服务端 enforcement 实际装配。

第 11 节的 `/health` 仍是无外部依赖的进程 liveness，只证明 Gin 路由和 handler 能响应。第 32 节候选要求 API 在监听前分别打开并 Ping business 与 Identity pool，运行中 `/ready` 在同一预算内检查两个 authority；任一失败时 `/ready` 为 503 而 `/health` 仍可为 200。响应不披露是哪一池失败，两者也都不证明业务数据正确、Migration 最新或 SLO 达标。

第 12 节保持 `request_id` 与未来 OpenTelemetry `trace_id` 分离；fault 平台层不导入 Gin/HTTP，只有 HTTP adapter 决定 status 和公开 error envelope。配置与隐私规则见[配置参考](../configuration.md)，长期边界见[ADR-0009](../decisions/ADR-0009-runtime-boundaries.md)。

第 13 节保持 API 与 Migration 身份和进程分离。第 30 节 v5→v11 的一次性 MySQL、长期 Compose 与 `growthos_app` 八表拒绝证据继续作为历史基线；第 32 节不回写历史，只追加 `000012` account、`000013` Session 与 `000014` throttle，把 source latest 提升到 14。API 使用分离的 `growthos_app`/`growthos_identity` pool，provisioner 使用只可 account INSERT 的第三个运行身份，migrator 继续独占 DDL。边界见 [ADR-0010](../decisions/ADR-0010-mysql-migration-boundaries.md)、[ADR-0026](../decisions/ADR-0026-activity-publication-binding.md)与 [ADR-0028](../decisions/ADR-0028-identity-session-authentication.md)，操作步骤见 [MySQL Migration 运维手册](../runbooks/mysql-migrations.md)和 [Identity Session 运维手册](../runbooks/identity-session-operations.md)。

第 15 节的系统状态页继续通过同源代理消费探针；第 32 节 `sessionApi` 同样只接受同源绝对路径，并为 create/current/revoke 分别固定请求形态与运行时 DTO。`AuthLayout` 管理检查、匿名、已认证和依赖不可用状态，generation/abort guard 阻止旧 current 结果在注销后复活。`/login` 与 `/session` 的认证导航不是业务 route guard；前端工具链要求 Node.js `>=22.22.2`、pnpm `10.13.1`，质量门包含 `test`、`typecheck` 和 `build`。

Compose 默认只发布 `127.0.0.1:8088` 的 Nginx Web 入口；`GROWTHOS_COMPOSE_WEB_PORT` 可调整端口，但地址始终只绑定回环。启动门仍为 `mysql healthy → migrate exited 0 → mysql-grants exited 0 → API`，Redis 不成为 API 启动、readiness 或 Session authority。第 32 节只增加 Identity Secrets/grants、双 pool 和 profile-scoped provision/maintenance one-shot；这些 operations 容器无公开端口且不常驻。长期边界见 [ADR-0012](../decisions/ADR-0012-compose-development-topology.md)、[ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)与 [ADR-0028](../decisions/ADR-0028-identity-session-authentication.md)。

第 17～30 节形成 Strategy/Award、ephemeral API/React、cache-aside、资格/路由、exact graph/snapshot 与 Activity publication，但新业务内核仍未进入现有 ephemeral selection。第 32 节只在同一进程旁路装配 Session 认证，不把 trusted Principal 注入 Lottery/Marketing use case，也不改变 DrawID、结果持久化、幂等、完整资格、库存或发奖缺口；INV-03 尚未满足。

第 31 节公共授权词典能对 exact Principal/Resource/Action 形成 confirmed allow/deny 或 zero Decision + error。第 32 节现在只建立 credential → server-side Session → trusted human Principal，但 Identity 仍不导入 Governance evaluator，现有 ephemeral route 也不消费该 Principal。第 33 节必须由服务端加载 Resource facts 并强制 Policy；在此之前，Session、隐藏菜单和构造出的 Resource 都不是 allow 证据。

第 29 节保留其普通/race/fuzz/coverage 与 disposable MySQL 证据。第 30 节最终源码/文档候选又实际通过 `make verify`、真实 disposable MySQL 8.4.11、长期 Compose status/smoke 与独立 Lottery acceptance；这证明当前未装配内核和工程边界，不证明业务 SLO、运行时 Activity API 或生产容量。

第一版运行时采用 [ADR-0007](../decisions/ADR-0007-modular-monolith-first.md) 确定的模块化单体：一个 Go 产品进程可以装配多个领域模块，但共享进程和数据库实例不改变事实所有权。服务拆分必须等待第 78 节起出现的负载、发布、故障域、合规或团队证据。

## 有意延迟的决定

- Gin 在第 11 节作为 Go HTTP 基线接入；gRPC + Protobuf 是后续服务间 RPC 基线，到第 80 节再按拆分需求接入。
- 第 12 节已把监听地址、HTTP timeout、日志级别和格式纳入显式配置，并建立请求关联与统一错误。
- MySQL 连接、`sqlx` pool 与前向 Migration 机制已在第 13 节接入，第 18～19 节建立首组 Lottery 表与 Strategy Repository；第 28 节增加 create-only graph/node/edge 三表和 graph Repository。两类读取都用只读 RR 快照；graph 更新、删除、发布/active revision 与精准缓存失效仍等待真实用例。
- React、TypeScript、Vite、Tailwind CSS、Lucide、Recharts 和 Zustand 在第 14 节接入；第 15 节接通系统探针，第 22 节接通 Lottery ephemeral selection，第 32 节再接通 `/login` 与 `/session`。业务工作台仍等待真实 API/RBAC，不以认证页、Mock 或本地交互冒充权限完成。
- 第 16 节已引入 Compose 本地开发环境，第 24 节接入第一个业务 Redis 消费者和最小 ACL；它仍是单机开发拓扑：没有镜像 digest 固定、内部 TLS、生产资源配额、Secret Manager、Redis HA 或生产容量证明。
- 第 19～29 节逐步建立 Strategy 仓储、无偏选择、ephemeral API/React、规则所有权、Redis 投影、Participation 资格、会员路由、routing graph 和 closed evaluator；第 30 节已验收 exact Strategy snapshot 与 Activity publication。以上新增内核仍未在线执行；正式 Draw/Result、幂等、库存与发奖必须由后续真实问题驱动，runtime 新表 grant、运行写权限和缓存失效总线不会被提前加入。
- 公共访问控制策略模型已在第 31 节实现；第 32 节候选再实现登录认证与真实会话。Policy/assignment repository、租户/对象事实装配、服务端强制、前端 capability 投影、拒绝审计与完整越权 E2E 仍未实现；第 33～35 节继续闭环，第 36 节首个真实运营后台再复用它。
- 服务拆分、RPC 和注册中心延迟至第 78 节以后。
- 最终目录图和 ER 图延迟至第 101 节复盘。
