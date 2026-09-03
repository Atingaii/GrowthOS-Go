# 非功能需求基线 v1

**状态：** v1 目标基线；第 1～32 节开发范围已验收；第 32 节 MySQL、完整 development Compose/Session wire、浏览器核心旅程、Go 普通/race、Web 与 doccheck 门禁均通过，业务 SLO 与 production/扩展验收尚未完成

**更新日期：** 2026-09-03

**来源章节：** [第 7 节：确定非功能需求](../course/part-01/lesson-07-non-functional-requirements.md)及第 23～29 节增量证据；第 30 节以[Activity Publication 绑定基线](activity-publication-binding-v1.md)登记并验收 exact Strategy snapshot、immutable publication、CAS、rollback、resolve gate、Lottery ACL、commit unknown 对账与 v11 边界；第 31 节冻结纯 Governance evaluator 边界；第 32 节以[真实 Session 认证基线](identity-session-authentication-v1.md)登记并验收 Identity 安全与运维开发基线。

## 1. 使用规则

本文件是 GrowthOS 的非功能需求台账。它记录目标、测量口径和未来证据，不是性能宣传页。

1. `目标值` 表示需要在对应里程碑验证的候选 SLO；
2. `实测值` 只能引用带环境和脚本版本的 QA/压测报告；
3. 没有实现或没有测量时写“未测量”，禁止填入估算结果；
4. 需求变化时保留修改原因，不能为了让测试通过而静默降低目标；
5. 外部支付、交易、券、消息和 LLM 的指标与 GrowthOS 自身指标分开统计。

## 2. 当前状态

| 项目 | 当前事实 |
| --- | --- |
| Go 运行时基线 | 第 11～16 节已验收 Gin 进程、`GET /health`、MySQL `GET /ready`、类型化配置、文件秘密、结构化日志、请求关联与统一错误；Compose 只发布同源 Web 入口；第 17～21 节新增并装配 Lottery 领域、两表、Repository、选择器、生产随机源与受限 ephemeral API；第 24 节在权威 Reader 外装配可选 Redis cache-aside，Redis 不参与启动或 readiness。第 32 节已在同一 API 进程装配 Session HTTP，但业务与 Identity 使用独立 credential/pool，`/ready` 同时检查两个必需 MySQL pool |
| 数据库与历史证据 | 源码 Migration latest 为 14：第 30 节十张业务表及 v11 真实 MySQL/Compose、checksum 与八表 1142 权限负证继续作为历史证据；第 32 节由 `000012`～`000014` 新增 `identity_workforce_account`、`identity_session`、`identity_authentication_throttle`，当前共十三张应用表。`growthos_app` 保持旧两表 `SELECT`；`growthos_identity` 只获账户必要读取/受控 `updated_at` 与 Session/throttle DML；`growthos_identity_provisioner` 只有账户 `INSERT`；`growthos_migrator` 承担 DDL。独立 disposable MySQL 8.4.11 官方门禁与第 32 节开发范围完整门禁已实际通过 |
| Identity 认证（开发范围已验收） | MySQL 是 workforce account、Session 与双维 throttle 的唯一 authority；Argon2id、opaque token digest、status/epoch/revoke/idle/absolute expiry、session-bound CSRF、Origin/Cookie 边界已实现。Redis 不是 Session authority 或 fallback；认证输出只含 trusted human Principal，不含 Role、Scope、Permission 或 authorization result |
| Lottery 领域 | Strategy/Award、Repository、WeightedSelector、CryptoSource、会员路由、exact graph/evaluator 与 create-only Strategy snapshot 已形成；真实 MySQL 已验证 snapshot exact RR、并发创建、事务回滚和隔离最小 writer。graph/snapshot service 与 adapter 均未装配，不存在 latest fallback |
| Marketing Activity | draft/published/retired、immutable publication history、state-version CAS、追加式 rollback、retire、一次 Clock 的 `[start,end)` resolve gate、Lottery/Approval verifier ports与 commit receipt 三态对账已形成。真实 MySQL 已验证 publish/replace/rollback/retire、RR、CAS、half-write 回滚；没有真实 Governance provider、API/UI、runtime composition、权限或业务 SLO 实测 |
| Participation 资格 | 第 25 节已验证权威注册事实快照、版本化含边界 cutoff、主体/未来时间/freshness 检查，以及 eligible/ineligible 与 not-found/stale/unavailable/invalid/cancelled 的语义分离；第 26 节再增加 source-owned `passed/blocked` 风险快照与具体准入 policy，并用固定 `new-user -> risk` 计划在任何 reader 前捕获一次服务端 logical as-of。只有 confirmed eligible 才继续；拒绝、技术失败和 caller cancellation 均短路，技术失败返回零 aggregate。当前没有生产事实 adapter、持久化、缓存、HTTP/React、Activity、真实 Principal、Lottery/composition 装配或资格性能实测 |
| Strategy 缓存 | MySQL 始终是权威来源；Redis 只保存可重建读取投影，不保存 not-found、一次选择或最终结果。缓存 miss/错误/写失败 fail-open，staging/production 启用时强制身份验证 TLS；Compose ACL 允许无 key 的 `PING`，并只允许对版本化 key 前缀执行 `GETRANGE/SET/DEL`，48 MiB `allkeys-lru`、无持久化 |
| 规则决策质量 | 第 25～29 节内核证据保持不变。第 30 节已验证 exact candidate、一次 Clock、`[start,end)`、CAS conflict、append-only rollback provenance、terminal snapshot 闭合集、half-write rollback 与所有失败 zero-result；源码为 COMMIT 应答未知提供 exact receipt/observation 的 committed/not_committed/indeterminate 三态。授权拒绝、完整资格、持久化审计与 runtime 编排仍待后续章节验证 |
| 求值内核质量门 | 第 29 节最终候选上，Lottery domain/application atomic coverage 分别为 93.6%/88.3%、合并 92.1%；全仓普通与 `-race` 测试通过；独立 10 秒 evaluator fuzz 通过 2,899,250 execs，新发现 1 个 interesting input（总数 43）。这些数据只是内核回归证据，不是业务覆盖率、生产并发安全、穷举证明或 SLO |
| React | 第 14 节框架与第 15 节系统状态页已完成；系统探针在宿主开发模式经 Vite、Compose 模式经 Nginx 同源代理真实读取 Go API。第 22 节让 `/lottery` 真实消费 ephemeral API；第 32 节已新增并完成开发验收的独立 `AuthLayout`、同源 Session transport、`/login` 与 `/session`，并区分 checking/anonymous/authenticated/unavailable。其他业务、Admin、MCP 与 Agent 工作台仍未由权限裁剪，部分仍是显式 Mock/本地状态；要求 Node.js `>=22.22.2`、pnpm `10.13.1` |
| 前端质量门 | Vitest、TypeScript typecheck、Vite build 与格式检查已纳入验证；第 32 节 23 个文件、250 个测试和桌面/移动/认证态浏览器 Design QA 已实际通过。该证据只覆盖 Session transport 与认证 UX；第 34 节按 capability 裁剪导航/路由/操作和第 35 节跨角色越权 E2E 仍未实现 |
| 性能实测 | M0 `/health` 100 RPS×5min：30,000/30,000 成功、P99 4.1495ms；`/ready` 20 RPS×30s：600/600 成功、P99 6.841375ms。M1 在同一本地 Docker Desktop 上对 ephemeral selection 各跑 50 RPS×10s：warm-cache、cache-disabled、Redis-down 均 500/500 成功且零 error/unexpected/dropped；三组 P99 分别 5.202ms、9.747167ms、8.222959ms。均为单机短窗口开发基线，不是业务 SLO或生产容量 |
| 可用性实测 | M0 两组与 M1 三组共五个短窗口内，错误、异常状态和丢弃均为 0；没有长稳、跨主机、灾备或生产可用性证据 |
| 恢复演练 | 已演练 MySQL、API、Redis 单点停止与恢复；第 24 节进一步证明 Redis down 时 cold read 回源、MySQL down 时 warm hit 可用而 cold miss 失败、两者恢复后无需重启 API 可重新填充。未验证宿主机故障、Redis/MySQL HA 或数据灾难恢复 |
| 安全与故障演练 | 第 30 节长期 runtime 旧两表 `SELECT`、其余八表 1142、一次性 writer、Compose v5→v11 与清理证据继续保留。第 32 节另验证分离 Identity/provisioner 权限、one-shot maintenance、浏览器登录/恢复/退出与依赖不可用呈现；完整 development Compose/Session wire 与最终代码/文档门禁已实际通过。raw Content-Length 特定代理变体、真实 wire COMMIT 丢应答、production TLS/可信代理和更广设备/AT 仍未完成；它也不等于第 33 节服务端 RBAC、第 34 节前端裁剪、第 35 节越权验收或生产渗透测试 |

第 13～29 节已有证据口径继续按各自 QA 保留。第 30 节真实证据覆盖当时源码 latest 11、一次性 MySQL 8.4.11、长期 Compose v5→v11、八表权限负证、独立 Lottery acceptance、清理和 `make verify`；这些历史证据不因源码前进到 latest 14 而被改写。第 32 节开发验收只证明认证链，不可外推为业务/Admin/MCP/Agent 已授权。

### M0 工程探针实测

| 路径 | 负载窗口 | 成功 / 计划 | 错误 / 异常状态 / 丢弃 | P50 | P95 | P99 | 最大值 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `/health`（经 Nginx） | 100 RPS × 5 min | 30,000 / 30,000 | 0 / 0 / 0 | 1.084208 ms | 2.744875 ms | 4.1495 ms | 18.116291 ms | 通过本节 P99 ≤ 100 ms 工程门槛 |
| `/ready`（经 Nginx + MySQL Ping） | 20 RPS × 30 s | 600 / 600 | 0 / 0 / 0 | 4.08525 ms | 5.935083 ms | 6.841375 ms | 8.570541 ms | 记录基线；未设置独立 P99 门槛 |

环境、原始 JSON、资源水位、故障注入与未覆盖项见[第 16 节 QA](../qa/lessons/lesson-16.md)。

### M1 Strategy 缓存本地实测

| 场景 | 负载窗口 | 成功 / 计划 | 错误 / 异常状态 / 丢弃 | 实际 RPS | P50 / P95 / P99 / 最大值 | 权威读取与缓存证据 |
| --- | --- | ---: | ---: | ---: | --- | --- |
| warm-cache | 50 RPS × 10s | 500 / 500 | 0 / 0 / 0 | 50.086182 | 1.730167 / 4.129458 / 5.202 / 7.387375 ms | MySQL execute `0`；cache hit `500` |
| cache disabled / direct-MySQL | 50 RPS × 10s | 500 / 500 | 0 / 0 / 0 | 50.076986 | 2.390709 / 5.382042 / 9.747167 / 25.003417 ms | MySQL execute `1000`；source load `500`；cache event `0` |
| Redis down | 50 RPS × 10s | 500 / 500 | 0 / 0 / 0 | 50.050506 | 2.602833 / 5.777708 / 8.222959 / 10.596541 ms | MySQL execute `1000`；source load/fill leader `500/500`；joined `0`；限流 read-error log `1` |

MySQL 计数来自 Performance Schema 的 prepared statement `statement/com/Execute`，因为当前 Go driver 会把带参数的两条 Strategy SELECT 作为 prepared execute；脚本同时核对运行身份仍为精确 SELECT-only，查询 fingerprint 未变。三组在同一个 2026-08-30 本地开发环境顺序执行，不能作为跨版本显著性检验、云环境容量结论或 SLO；完整汇总口径与证据边界见[第 24 节 QA](../qa/lessons/lesson-24.md)。

## 3. 服务等级候选目标

| 能力 | 峰值容量目标 | P99 目标 | 可用性目标 | 当前实测 |
| --- | ---: | ---: | ---: | --- |
| Feed 读取 | 50,000 RPS / 5min | ≤ 200ms | 99.90% | 未测量 |
| Behavior 事件接收 | 100,000 RPS / 5min | ≤ 100ms | 99.90% | 未测量 |
| 活动参与 | 10,000 RPS / 5min | ≤ 300ms | 99.95% | 未测量 |
| 抽奖决策 | 10,000 RPS / 5min | ≤ 150ms | 99.95% | 未测量 |
| 权益发放受理 | 5,000 RPS / 5min | ≤ 300ms | 99.95% | 未测量 |
| 运营查询/写入 | 300 RPS / 5min | 查询 ≤ 500ms；写入 ≤ 800ms | 99.95% | 未测量 |
| 分析查询 | 500 RPS / 5min | ≤ 2s | 99.90% | 未测量 |
| MCP Tool 网关 | 500 RPS / 5min | 网关开销 ≤ 100ms | 99.50% | 未测量 |

统计窗口、有效请求定义、外部耗时排除项和阶段性门槛见课程正文。后续修改本表时必须同步对应 QA 证据。

## 4. 关键业务不变量

| 编号 | 不变量 | 未来证据 |
| --- | --- | --- |
| INV-01 | 同一业务请求最多形成一份有效参与订单 | 并发重复请求测试、唯一约束验证 |
| INV-02 | 一次参与最多消耗一次额度 | 余额/流水对账和重试测试 |
| INV-03 | 一次抽奖只能有一个最终结果 | 超时、崩溃恢复和结果查询测试 |
| INV-04 | 权益变化全部关联唯一业务流水 | 账实对账、重复消息测试 |
| INV-05 | 库存领取总量不超过可分配库存 | 高并发库存测试和最终对账 |
| INV-06 | 活动和审批并发修改不会静默覆盖 | 版本冲突测试 |
| INV-07 | 高风险操作不能绕过权限与审批 | 越权、审批失效和审计测试 |
| INV-08 | 模型文本不能直接确认业务成功 | Tool 结果校验和失败注入测试 |
| INV-09 | 每次高风险写操作必须形成完整且不可由普通用户覆盖的审计记录 | 审计完整性对账、故障注入测试 |
| INV-10 | raw password、raw Session token、CSRF 派生密钥不得落库、进入日志或前端持久层 | 数据指纹、日志负证、浏览器存储检查与故障注入 |
| INV-11 | Session 只有在 account enabled、authentication epoch 匹配、未撤销且 idle/absolute expiry 均有效时才能恢复 Principal | 时钟边界、并发撤销、epoch bump、touch 与真实 MySQL 测试 |
| INV-12 | 认证成功不能绕过服务端 Policy；没有 exact Resource facts 与 allow decision 时业务操作必须 default deny | 第 33 节服务端负证与第 35 节跨角色/对象/租户 E2E |

## 5. 最终一致性目标

| 链路 | 目标 | 观测方式 | 当前实测 |
| --- | ---: | --- | --- |
| Redis 库存 → MySQL | ≤ 5s | 库存版本延迟、账实差异 | 未测量 |
| 异步权益发放 | 95% ≤ 3s，99.9% ≤ 5min | 创建到到账的分位数 | 未测量 |
| Behavior → 实时画像 | ≤ 10s | 事件时间与画像水位 | 未测量 |
| Behavior → 漏斗分析 | ≤ 5min | 数据水位与查询更新时间 | 未测量 |
| 事实 → OpenSearch 投影 | ≤ 60s | 版本和更新时间差 | 未测量 |
| 外部转化事件 → 分析链 | 接收后 ≤ 60s | 来源时间、接收时间和入链时间 | 未测量 |

## 6. 恢复目标

| 数据/能力 | RPO | RTO | 当前证据 |
| --- | ---: | ---: | --- |
| 单实例故障下已确认核心事实 | 0 | ≤ 5min | 无 |
| 核心数据库灾难恢复 | ≤ 5min | ≤ 60min | 无 |
| 活动与配置数据 | ≤ 5min | ≤ 60min | 无 |
| 已可靠接收的行为数据 | ≤ 15min | ≤ 4h | 无 |
| 可重建的搜索、画像和分析投影 | 可重建 | ≤ 8h | 无 |

## 7. 降级优先级

```text
优先保护：参与事实、抽奖结果、权益流水、库存、审批和审计
可以降级：个性化排序、实时画像、分析查询、搜索和 AI
不能降级：Session 权威解析、权限校验、幂等校验、金额/额度正确性和事实验证
```

GrowthOS 的人工运营能力不能依赖 LLM 才能工作；核心交易不能依赖分析链实时可用；外部权益不可用时必须保持处理中或失败状态，不能显示假成功。Identity MySQL 不可用时不能从 Redis、旧 UI snapshot 或客户端字段恢复 Principal；前端必须区分“匿名”和“暂时无法确认身份”。

## 8. 可观测性验收清单

- [ ] HTTP、RPC、MQ、任务、MCP 和 Agent 链路可关联 `trace_id`；
- [ ] 核心写操作可按业务请求号查询最终状态；
- [ ] Dashboard 能看到 QPS、错误率、P95/P99 和资源水位；
- [ ] 库存差异、消息积压、补偿和外部依赖有业务告警；
- [ ] 错误、结果未知和高风险操作保留完整 Trace；
- [ ] 日志、事件与 Prompt 不包含未脱敏秘密和完整个人数据；
- [ ] 认证指标只使用低基数结果/阶段标签，禁止记录 login name、raw token、password、CSRF、digest 或可反推 source 的值；
- [ ] 告警关联值班手册、影响范围和恢复验证。

## 9. AI 与审批目标

| 控制项 | 候选目标 | 当前实测 |
| --- | --- | --- |
| 审批有效期 | 默认 ≤ 24h；版本、参数、预算或风险变化立即失效 | 未测量 |
| MCP Tool 网关开销 | P99 ≤ 100ms，不含 Tool、LLM 和人工等待 | 未测量 |
| AI 任务检查点恢复 | 执行器恢复后 ≤ 15min 恢复可继续任务 | 未测量 |
| 核心写 Tool 结果未知核对 | ≤ 5min 自动查询或升级人工 | 未测量 |
| 高风险写操作审计完整性 | 100% | 未测量 |

单个 Tool 的业务超时、重试和执行预算必须在接口出现时单独登记，不能用网关开销代替端到端耗时。

## 10. 阶段证据台账

| 里程碑 | 计划证据 | 状态 |
| --- | --- | --- |
| M0 · 第 16 节 | 健康接口负载、连接池与 Compose 稳定性 | 已验证；仅为本机短时工程基线，详见第 16 节 QA |
| 第 17 节领域前置 | Strategy/Award 构造、边界、溢出、所有权与确定性单元测试 | 已验证纯内存配置对象；没有 Draw/Result，INV-03 和业务 SLO 尚未验证 |
| 第 18 节持久化前置 | 两表 DDL、约束、latest 2/dirty、应用只读权限与 Compose 授权启动门 | 已验证 Schema/权限，不含 Repository、业务写入、抽奖执行或业务 SLO |
| 第 19 节仓储前置 | 父子原子 Create、RR/read-only FindByID、恢复校验、并发/取消、1205、执行计划与精确权限 | 已在隔离 MySQL 8.4.11 验证；尚无算法、HTTP 业务负载、Draw/Result 或业务 SLO |
| 第 20 节算法前置 | 精确整数桶、完整 `uint64`、crypto rejection、错误边界、并发路径和本机微基准 | 已验证纯内存 selector 与随机 adapter；不含 HTTP、SQL、Draw/Result、幂等重试或业务 SLO |
| 第 21 节临时 API 前置 | feature gate、最小 DTO、两表 SELECT-only、Nginx→Go→MySQL→CryptoSource、网关故障、数据指纹与有界并发 | 已在隔离 Compose 验证 64 个请求、最大并行 16；没有定速 RPS/延迟分位、正式 Draw、认证、幂等、React 联调或业务 SLO |
| 第 22 节 React 消费者前置 | 同源 bodyless POST、运行时 DTO 解码、pending 抑制、取消与旧响应隔离、桌面/移动工作台、真实浏览器与前端质量门 | 已验证 ephemeral API 的真实可见消费者；没有正式 Draw、认证、RBAC、自动重试、持久化结果或业务 SLO，其余工作台仍是显式 Mock/本地状态 |
| 第 23 节规则边界前置 | 32 条需求、事实/决定所有权、失败分类、版本与解释约束、精确文档白名单及相对第 22 节的 runtime negative diff | 已验证需求与停止线；没有 Rule/责任链/规则树、资格、权限、Draw 或运行时性能证据 |
| M1 · 第 24 节 | Lottery Strategy 读取投影 cache-aside、最小 ACL、poison/依赖故障恢复与三组 50 RPS×10s source-load 基线 | 已在单次本地隔离 Compose 验证；三组均 500/500 成功，warm-cache MySQL execute=0，另两组=1000；不是正式 Draw、业务 SLO、长稳或生产容量 |
| 第 25 节资格前置 | RegistrationFactSnapshot、含边界 cutoff、freshness、一次 Clock、稳定决定/安全错误、取消竞态、并发与架构停止线 | 已在 domain/application 单元、fuzz、race 和全仓 Go 测试验证；没有 adapter、HTTP、Compose、真实身份、Lottery 门控或业务 SLO |
| 第 26 节资格链前置 | RiskScreeningFactSnapshot、具体风险准入、shared as-of、固定 `new-user -> risk` 顺序、后序零调用短路、零 aggregate 技术失败、事实读取 wrapper 的单一公开错误 class、最小有序 trace 与架构停止线 | 已在 Participation domain/application 单元、fuzz、race、并发和顺序扰动测试验证内核；没有 adapter、HTTP、Compose、真实身份、Lottery/composition 门控、持久化审计、浏览器 E2E 或业务 SLO |
| 第 27 节会员路由前置 | 封闭会员快照、一次 as-of、premium override、standard explicit default、一跳 path、unknown/技术失败不吞入 default 与 concrete router 语义 oracle | 已在 Lottery domain/application 单元、fuzz、race、并发和架构停止线验证；没有生产 fact adapter、持久化、HTTP/React、运行时装配、Activity、权限或业务 SLO |
| 第 28 节路由图持久化前置 | Lottery-owned bounded immutable rooted DAG、schema v1、latest 5 三表、create-only revision、事务 Create、RR Find、严格恢复、精确测试身份和 disposable MySQL 清理边界 | 已在单元/race/fuzz 与一次性 MySQL 8.4.11 六组 Integration 验证；任务资源零残留、长期 Docker 快照不变。该节仅交付持久化输入边界；当前 evaluator 由第 29 节提供，仍无 Activity/发布、公开 API/UI、runtime composition、认证/RBAC 或业务 SLO |
| 第 29 节路由图求值前置 | exact immutable graph、closed membership dispatch、one graph/Clock/fact、iterative exact-branch path、worst-depth + actual step hard stop、child deadline/cancellation priority、immutable evidence 和 zero-decision | 已在 domain/application 单元、64-worker、架构、全仓 race、atomic coverage 和 10 秒 fuzz 验证；第 28 节 MySQL 8.4.11 六组 Integration 上游回归重跑通过。没有 Activity/发布、公开 API/UI、runtime composition、生产 fact adapter、认证/RBAC、Strategy selection/Draw 或业务 SLO |
| 第 30 节 Activity publication 前置 | exact Strategy snapshot、immutable publication、CAS、rollback provenance、retire、一次 Clock 的 `[start,end)` gate、Lottery terminal manifest ACL、latest 11 与八表零权限；源码另提供 commit unknown receipt 对账 | 已通过源码、真实 MySQL 8.4.11、长期 Compose v11、独立 Lottery acceptance、清理与全仓质量门验证；所有 service/Repository/ACL 未装配，API/UI、Governance provider、认证/RBAC、Draw 与业务 SLO 均未实现 |
| 第 31 节访问控制模型前置 | exact capability、Role ceiling、Scope、immutable Policy revision、default-deny evaluator 与威胁边界 | 已验收纯 domain/application evaluator；没有 credential、Policy repository、服务端 enforcement、HTTP DTO 或 React projection |
| 第 32 节真实 Session 认证 | Identity account/Session/throttle、Argon2id、独立数据库身份/pool、Session HTTP、`/login`/`/session`、provision/maintenance 与浏览器认证旅程 | 已完成（开发范围）；独立 MySQL、完整 development Compose/Session wire、浏览器核心旅程、Go 普通/race、Web 与 doccheck 均通过。raw Content-Length 代理矩阵、真实 wire COMMIT acknowledgement-loss、production TLS、可信代理与更广设备/AT 仍待验收，不构成生产就绪；第 33～35 节尚未实现 |
| M2 · 第 45 节 | 活动参与、库存、锁和幂等报告 | 待验证 |
| M3 · 第 61 节 | 积分、优惠券、返利与权益闭环报告 | 待验证 |
| M4 · 第 77 节 | Feed、事件接收与分析水位报告 | 待验证 |
| M5 · 第 85 节 | 分布式峰值和分片依据 | 待验证 |
| M6 · 第 93 节 | MCP 权限、审计和网关开销 | 待验证 |
| M7 · 第 101 节 | 12 小时稳定性、灾备和故障演练报告 | 待验证 |

第 24 节缓存证据可追溯到[课程](../course/part-03/lesson-24-redis-strategy-cache.md)、[API](../api/lessons/lesson-24.md)、[QA](../qa/lessons/lesson-24.md)、[设计手记](../design-thinking/lessons/lesson-24.md)、[面试问答](../interview/lessons/lesson-24.md)、[运维手册](../runbooks/redis-strategy-cache.md)与 [ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)。

第 25 节资格证据可追溯到[规则基线](new-user-eligibility-v1.md)、[课程](../course/part-04/lesson-25-user-eligibility.md)、[API 零变化记录](../api/lessons/lesson-25.md)、[QA](../qa/lessons/lesson-25.md)、[设计手记](../design-thinking/lessons/lesson-25.md)、[面试问答](../interview/lessons/lesson-25.md)与 [ADR-0021](../decisions/ADR-0021-participation-new-user-eligibility.md)。

第 26 节资格链证据可追溯到[规则链基线](participation-prerequisite-chain-v1.md)、[课程](../course/part-04/lesson-26-responsibility-chain.md)、[API 零变化记录](../api/lessons/lesson-26.md)、[QA](../qa/lessons/lesson-26.md)、[设计手记](../design-thinking/lessons/lesson-26.md)、[面试问答](../interview/lessons/lesson-26.md)与 [ADR-0022](../decisions/ADR-0022-participation-prerequisite-chain.md)。该证据只覆盖 Participation domain/application 内核，不是在线资格门控、跨 authority 原子快照、公开 API 或浏览器 E2E 证据。

第 27 节会员路由证据可追溯到[产品基线](membership-strategy-routing-v1.md)、[课程](../course/part-04/lesson-27-responsibility-chain-limits.md)、[API 零变化记录](../api/lessons/lesson-27.md)、[QA](../qa/lessons/lesson-27.md)、[设计手记](../design-thinking/lessons/lesson-27.md)、[面试问答](../interview/lessons/lesson-27.md)与 [ADR-0023](../decisions/ADR-0023-membership-strategy-routing-boundary.md)。第 28 节持久化证据可追溯到[Strategy Routing Graph 基线](lottery-strategy-routing-graph-v1.md)、[课程](../course/part-04/lesson-28-rule-tree-schema.md)、[API 零变化记录](../api/lessons/lesson-28.md)、[QA](../qa/lessons/lesson-28.md)、[设计手记](../design-thinking/lessons/lesson-28.md)、[面试问答](../interview/lessons/lesson-28.md)与 [ADR-0024](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)。第 29 节求值证据可追溯到[产品基线](lottery-strategy-routing-evaluation-v1.md)、[课程](../course/part-04/lesson-29-rule-decision-engine.md)、[API 零变化记录](../api/lessons/lesson-29.md)、[QA](../qa/lessons/lesson-29.md)、[设计手记](../design-thinking/lessons/lesson-29.md)、[面试问答](../interview/lessons/lesson-29.md)、[运维手册](../runbooks/strategy-routing-graph-evaluation.md)与 [ADR-0025](../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)。第 29 节是内部 evaluation 证据，不是发布/Activity/权限/runtime/浏览器 E2E 证据。

第 30 节证据可追溯到[产品基线](activity-publication-binding-v1.md)、[课程](../course/part-04/lesson-30-strategy-vs-activity.md)、[API 零变化记录](../api/lessons/lesson-30.md)、[QA](../qa/lessons/lesson-30.md)、[设计手记](../design-thinking/lessons/lesson-30.md)、[面试问答](../interview/lessons/lesson-30.md)、[运维手册](../runbooks/activity-publication.md)与 [ADR-0026](../decisions/ADR-0026-activity-publication-binding.md)。这些链接证明未装配发布内核与 v11 工程验收，不是业务运行或权限系统证据。

第 32 节已验收实现可追溯到[产品基线](identity-session-authentication-v1.md)、[课程](../course/part-04/lesson-32-real-session-authentication.md)、[API](../api/lessons/lesson-32.md)、[QA](../qa/lessons/lesson-32.md)、[设计手记](../design-thinking/lessons/lesson-32.md)、[面试问答](../interview/lessons/lesson-32.md)、[运维手册](../runbooks/identity-session-operations.md)、[浏览器设计 QA](../../design-qa.md)与 [ADR-0028](../decisions/ADR-0028-identity-session-authentication.md)。其 MySQL、完整 development Compose/Session wire、浏览器核心旅程及代码/文档门禁已经闭环；raw framing、真实 wire COMMIT acknowledgement-loss、production TLS/可信代理与浏览器扩展矩阵继续保留为后续证据，不由开发验收外推。

## 11. 变更触发条件

出现以下情况时必须重新评审本基线：

- 真实业务规模、请求比例或数据保留要求发生变化；
- 外部支付、券或 LLM 依赖改变端到端体验；
- 压测证明当前目标不合理或成本不可接受；
- 发生数据丢失、重复发放、越权或长时间恢复事故；
- 合规要求改变审计、隐私、加密或数据驻留边界；
- 上下文被拆为独立服务，统计口径随之变化。

目标变更需要写清业务原因、影响和新验证计划；具体技术方案形成长期约束时新增 ADR。
