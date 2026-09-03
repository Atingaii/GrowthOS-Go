# 服务端 RBAC 强制执行基线 v1

**状态：** 第 33 节规范基线，代码与验收仍按本文分阶段落地

**来源：** 第 31 节统一访问控制模型、第 32 节真实会话认证

**首个受保护用例：** `POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections`

## 1. 为什么现在才把权限接到真实接口

第 31 节只回答“一个完整 Policy 如何确定性地形成 allow 或 deny”，第 32 节只回答“一个浏览器 Cookie 如何恢复成可信 human Principal”。两节都刻意没有保护任何业务动作，因为真实授权还缺少四个条件：

1. 必须由资源所属模块提供服务端事实，不能相信浏览器提交的 owner、tenant、role 或 scope；
2. 必须存在可追溯的 Policy revision 和人员 RoleBinding 权威源，不能在 composition root 临时硬编码管理员；
3. 必须把授权点放在业务副作用之前，并使缺件、超时、损坏和审计失败全部 fail closed；
4. 必须定义匿名、禁止、资源不存在和技术故障的公开披露，而不是把内部 reason 原样返回。

因此第 33 节不是新增一套与业务无关的“鉴权演示接口”，而是把现有唯一已装配业务纵切真正接入：

```text
opaque Session Cookie
  -> Identity authority 恢复 VerifiedSession
  -> composition edge 映射 Governance human Principal
  -> Lottery 加载 canonical Strategy snapshot
  -> Governance 读取一个 exact immutable Policy revision
  -> Policy.Evaluate
  -> durable authorization audit
  -> 只有 audited allow 才执行 WeightedSelector
```

这条链只保护 development/test 专用的 ephemeral selection。它不会把该接口升级成正式 Draw，不会创建参与记录、扣次数、预占库存或发放权益。

## 2. 用户、威胁与成功标准

### 2.1 本节服务的人

| 人员 | 需要完成的工作 | 本节默认能力上限 |
| --- | --- | --- |
| 平台管理员 | 在受控环境验证平台配置与故障恢复 | 可以模拟 Lottery Strategy |
| Lottery 设计者 | 对自己获分配范围内的 Strategy 做临时模拟 | 可以模拟，但是否实际允许取决于 RoleBinding scope |
| 营销运营 | 读取 Strategy/Graph 以设计活动 | 默认不能执行随机模拟 |
| 安全审计员 | 只读 Policy、Audit 与业务配置 | 默认不能执行随机模拟 |
| Growth Member | 面向消费者的成员身份占位 | 没有 workforce 运营能力 |

Role 只是经过代码评审的能力上限。任何真实人员必须出现在被激活的 exact Policy revision 的 RoleBinding 中，才可能获得权限。

### 2.2 主要攻击路径

| 攻击 | 失败关闭位置 | 不能采用的做法 |
| --- | --- | --- |
| 匿名直接调用 API | Identity Cookie 解析与 Session authority | 只在 React 隐藏按钮 |
| 伪造 Principal/Role/Scope header | HTTP 边界拒绝或忽略，Principal 只来自 VerifiedSession | 信任 `X-User-ID` / `X-Role` |
| 替换 Strategy ID | exact Resource scope + default deny | 只检查“已登录” |
| 读权限升级成模拟权限 | exact `simulate` Action | 把 POST 粗略映射为 `read` |
| 跨 tenant/owner | 缺少权威事实时 scope 不匹配 | 从 query/body/header 补 tenant/owner |
| Policy 半发布或混 revision | 单事务发布 + RR snapshot exact load | 查询 `MAX(revision)` 或逐表 latest |
| 撤权后继续使用旧缓存 | 每请求读取 active Policy，不缓存 | 把 Policy 放 Redis fail-open cache |
| 审计失败仍执行业务 | allow audit 必须先提交 | 异步 fire-and-forget allow audit |
| deny 泄露对象存在性 | deny 与 not-found 使用同一 404 envelope | 返回内部 `no_binding` / `scope_mismatch` |
| 技术错误被当成正常 deny/allow | zero Decision + 503 或保守 404 | catch-all 后继续执行 selector |

### 2.3 完成标准

第 33 节只有同时满足以下条件才能声明完成：

- 当前 Session Cookie 能在受保护路由恢复成可信 human Principal；
- 伪造浏览器 Principal、Role、Scope、tenant 或 owner 不能改变授权结果；
- Policy、Role、Permission 和 RoleBinding 的 exact revision 已持久化，并由独立运行身份读取；
- active pointer 与新 revision 原子发布，运行时不查询 latest；
- Lottery Strategy 只加载一次，授权位于加载成功与随机选择之间；
- confirmed allow 的完整最小 Decision evidence 已持久化后，selector 才能被调用；
- confirmed deny、no binding、no permission、scope mismatch 与 explicit deny 均不调用 selector；
- 匿名为 401，对象级禁止与不存在同形 404，授权技术故障为 503；
- Governance MySQL 不可用、Policy 损坏或 allow audit 结果未知时 fail closed；
- 独立 MySQL、HTTP/Compose、grants、race、fuzz 与全仓门禁取得实际证据。

## 3. 本节新增的 exact capability

现有接口每次会产生一个新的随机 ephemeral selection。它不是只读 DTO，因此不能复用：

```text
object:lottery.strategy:read
```

第 33 节新增：

```text
object:lottery.strategy:simulate
```

`simulate` 表达“对一个已存在 Strategy 执行非持久化试运行”。它与未来正式 Draw/participate 完全不同。当前角色能力上限变化如下：

| Role | `object:lottery.strategy:simulate` |
| --- | --- |
| `platform_administrator` | 有 |
| `lottery_designer` | 有 |
| `marketing_operator` | 无 |
| `security_auditor` | 无 |
| `growth_member` | 无 |

即使角色模板有该 capability，也仍需 matching allow RoleBinding。matching deny 始终覆盖 allow。

## 4. 三类事实必须由三个所有者提供

### 4.1 Identity：只证明“是谁”

Identity 从 Cookie 读取 raw bearer，查询 MySQL Session 与 workforce account，返回不可伪造构造的 `VerifiedSession`。Governance integration 只消费其中的 `PrincipalID`，映射为：

```text
PrincipalKindHuman + exact PrincipalID
```

它不能使用 AccountID、LoginName、HTTP Session DTO 或浏览器 header 代替 Principal。raw token 在解析边界之外不可见，也不得进入日志、Policy 或 Audit。

### 4.2 Lottery：只证明“资源是什么”

`EphemeralSelectionService` 通过自己的 `StrategyReader` 读取一个 canonical Strategy aggregate。当前 Strategy schema 只有 ID、name 和 awards，没有 tenant 或 owner；因此第 33 节构造的 Resource 是：

```text
kind      = object
type      = lottery.strategy
id        = canonical decimal StrategyID
tenant    = absent
owner     = absent
```

“缺失”是当前权威 schema 的事实，不是 wildcard。system scope 与 tenant 为空的 exact resource scope可以匹配；tenant/owned scope 必须失败。本节不得宣称已经完成多租户数据隔离。

### 4.3 Governance：只证明“能不能做”

Governance 负责：

- 读取一个完整、不可变、激活的 Policy revision；
- 构造 server-owned AuditContext；
- 调用纯 evaluator；
- 同步持久化允许或拒绝的最小 Decision evidence；
- 向业务返回闭合的 allowed / denied / unavailable 分类。

Governance 不读取 Lottery 表，不解析 Cookie，不执行随机选择，也不把 Decision 当作可重复使用的 bearer。

## 5. Policy authority 与发布模型

### 5.1 为什么不能只存 `user_role`

如果人员 assignment 是一张可变 `user_role` 表，而审计只记录 `PolicyID + revision`，历史决定无法重放：同一个 revision 下的人员、scope 或 role capability 会在原地变化。第 31 节的 immutable Policy 也会在持久化边界失真。

因此一次 Policy revision 必须完整保存：

- Policy identity 与 schema version；
- 该 revision 实际启用的 Role 集；
- 每个 Role 的 exact Permission 子集；
- 每条 Principal + Role + Scope + effect RoleBinding。

代码仍拥有封闭 ResourceType、Action、RoleID 和每个角色的 capability ceiling；数据库 revision 只能收窄，不能越过代码模板增权。

### 5.2 规范化表

计划以前向 Migration 逐表建立：

1. `governance_policy_revision`：immutable header、schema version、canonical content digest；
2. `governance_policy_role`：该 revision 启用的 closed RoleID；
3. `governance_policy_role_permission`：每个 Role 的 exact capability 子集；
4. `governance_policy_role_binding`：Principal、Role、Scope、effect；
5. `governance_active_policy`：固定 runtime slot 指向一个 exact revision，并携带 CAS state version；
6. `governance_authorization_audit`：一次 confirmed Decision 的 header；
7. `governance_authorization_audit_match`：有界、规范排序的 match evidence。

每个 Migration 只含一条 DDL。历史 Migration 不回写，结构修正继续增加更高版本。

### 5.3 单事务 append + activate

operations-only publisher 执行：

1. 严格解析离线 Policy 文件；
2. 用领域构造器重建并验证完整 Policy；
3. 计算 canonical content digest；
4. 在一个事务中插入 header、roles、permissions、bindings；
5. 用 expected active state version 对固定 slot 做 CAS；
6. commit 后才报告成功。

同一 `(PolicyID, revision)` 永不更新或删除。重复发布只有 canonical digest 完全相同时才可报告 already-exists；内容冲突必须失败。COMMIT outcome unknown 不盲目重试，调用方按 exact identity 读取并核对 digest 后再决定。

运行时每请求用一个 read-only repeatable-read snapshot 读取 active pointer、exact header、roles、permissions 和最多 1024 条 bindings。它禁止：

- `MAX(revision)` / `ORDER BY revision DESC LIMIT 1`；
- 缺行后回退上一版；
- 部分表来自不同 revision；
- 未知 action/role/scope 被忽略；
- Redis 或进程缓存成为授权真相。

### 5.4 数据库身份隔离

| 身份 | 允许 | 明确拒绝 |
| --- | --- | --- |
| `growthos_governance` | Policy/active 表 SELECT；audit/match INSERT | Policy 更新、业务表、Identity 表、DDL |
| `growthos_governance_publisher` | Policy revision 子表 INSERT；active slot SELECT/INSERT/UPDATE | audit 伪造、历史 revision UPDATE/DELETE、业务/Identity 表、DDL |
| `growthos_migrator` | Migration 所需 DDL | 运行流量 |
| `growthos_app` | 原有 Lottery 只读边界 | Governance 与 Identity 表 |
| `growthos_identity` | Identity account/session/throttle 必要 DML | Governance 与业务表 |

grant reconciler 必须对 fresh volume 和 existing volume 都执行 `REVOKE ALL` 后精确授权；不能把初始化脚本只运行一次当成持续隔离。

## 6. 请求认证与 CSRF 边界

受保护路由只接受配置模式对应的唯一 Session Cookie。Identity-owned authenticator 负责：

1. 拒绝重复同名 Cookie、development/production Cookie 混用与非法编码；
2. 调用 `ResolveService.Resolve`；
3. 将 missing、expired、revoked、epoch mismatch 和 disabled account 统一为 unauthenticated；
4. 将 Identity dependency failure 归类为 authentication unavailable；
5. 返回 `VerifiedSession`，绝不返回 raw bearer。

该 POST 已有 exact custom `X-GrowthOS-Demo-Mode: ephemeral-selection`，该 header 不是 CORS safelisted；同源入口也不开放跨源 CORS。第 33 节仍需重新执行 exact Origin / Fetch Metadata 检查，并保持 Session Cookie 的 SameSite 策略。由于接口只在 development/test 开启且无持久业务副作用，本切片允许把“exact Origin + same-origin Fetch Metadata + non-safelisted Demo header + no CORS + SameSite”作为 CSRF 边界，不额外改变当前 React 请求签名。

这是一条有意限定的例外，不是通用 unsafe Cookie API 模板。后续 Activity publish、Policy change、正式 Draw 等真实写操作必须使用第 32 节已经建立的 session-bound CSRF token，并单独验收。

浏览器提交的下列 header 不会成为权威事实：

```text
X-Principal-ID
X-Account-ID
X-Role
X-Scope
X-Tenant-ID
X-Owner-ID
```

验收必须证明添加或替换这些 header 不会改变决定。

## 7. 强制点为什么在 Repository 与 Selector 之间

当前 use case 的关键顺序必须变成：

```text
validate input/context
  -> FindByID exactly once
  -> verify returned Strategy.ID
  -> map canonical Resource facts
  -> authorize simulate
  -> require confirmed audit outcome
  -> WeightedSelector.Select exactly once
  -> validate selected Award belongs to same Strategy snapshot
```

这样同时满足：

- 不从浏览器构造 tenant/owner；
- 授权与 selector 使用同一个 immutable Strategy snapshot；
- deny/unavailable 时随机源调用次数为零；
- 不为授权再读一次 Strategy，避免同请求读到两个版本；
- 未来资源增加 tenant/owner 时仍由 Lottery adapter 演进。

Lottery application 只拥有 consumer-side 窄端口，例如“当前 actor 是否可以 simulate 这个已加载 Strategy”。它不需要理解 Policy 表、Role、Cookie 或 HTTP 状态码。Governance integration adapter 负责把该端口映射到统一模型。

## 8. AuditContext 与持久证据

### 8.1 reference 来源

- evaluation reference：服务端密码学随机生成的 canonical lowercase reference；
- correlation reference：对经过平台验证的 Request ID 加 domain separation 后取 SHA-256，并编码成 lowercase reference；
- evaluated-at：一个 server clock snapshot，UTC、microsecond 精度。

不能直接把任意 Request ID 强转为 AuditReference，因为 HTTP request ID 的 grammar 未必等同 Governance grammar。

### 8.2 audit header

每条 confirmed Decision 至少保存：

- evaluation/correlation reference 和 evaluated/recorded 时间；
- exact PolicyID/revision；
- Principal kind/id；
- Resource kind/type/id、可选 tenant/owner；
- exact Action；
- allow/deny outcome 与内部 reason；
- match count。

match 子表按 canonical order 保存 binding ID、role ID、effect、scope kind 与 exact permission。它不保存 Cookie、password、token、CSRF、IP 原文、完整 Policy JSON、Strategy name/awards 或用户可见错误文字。

### 8.3 audit 失败语义

- allow：audit header + matches 必须同事务提交；失败或 COMMIT outcome unknown 返回 503，selector 为零；
- deny：仍先尝试同事务 append；成功后对外 404；若 sink 失败，为避免把“对象存在 + audit 故障”变成新的存在性 oracle，仍返回同形 404，同时输出无敏感值、低基数的 emergency structured log/metric；
- evaluator/policy 技术失败：没有伪造 Decision；对外 503，并记录低基数 operational event，sink 不可用时只能依靠外部日志告警。

因此“所有 allow 都有 durable audit”是安全不变量；“所有 deny 都有 durable audit”是正常态目标，但数据库全面故障时不能通过改变拒绝结果来实现。

## 9. 公开 HTTP 语义与低披露

| 场景 | HTTP | code | 资源/策略读取 | selector |
| --- | ---: | --- | --- | ---: |
| missing/invalid/expired/revoked Session | 401 | `unauthenticated` | 均不读 | 0 |
| Identity authority unavailable | 503 | `authentication_unavailable` | 均不读 | 0 |
| invalid canonical Strategy ID | 400 | `invalid_strategy_id` | 不读 | 0 |
| Strategy 不存在 | 404 | `lottery_strategy_not_found` | Strategy 读一次 | 0 |
| confirmed authorization deny | 404 | `lottery_strategy_not_found` | Strategy/Policy 各读一次快照 | 0 |
| Policy 缺失/损坏/超时 | 503 | `authorization_unavailable` | Strategy 已读，Policy 尝试 | 0 |
| allow audit 未确认 | 503 | `authorization_unavailable` | 已读 | 0 |
| confirmed audited allow | 200 | — | 已读 | 1 |

对象级 deny 不返回 403，是为了不向已认证但无权调用者确认 Strategy 是否存在。内部 reason、Role、binding、scope、Policy revision 和 audit reference 都不进入错误响应。

该选择只针对当前 exact-object endpoint。未来集合级动作或“不涉及对象存在性”的请求可以使用 403，但必须在对应章节单独定义。

## 10. 并发、撤权与 TOCTOU

Policy publish 在单事务中创建完整 revision 并 CAS active pointer。每个授权请求在一个 RR snapshot 中读取 active pointer 和完整 revision，因此不会看到半个 Policy 或混合版本。

请求开始后 active pointer 可能切换。本节接受一个明确边界：

- 当前 ephemeral operation 没有持久业务写；
- 已开始的请求可以按其 audit 中记录的 exact revision 完成；
- 后续请求在无 Policy cache 的前提下读取新 active revision；
- 紧急立即阻断仍可通过 Session revoke/account epoch 与停止 route 达成。

这不等于普遍解决 TOCTOU。未来 Activity publish、Policy change、库存或正式 Draw 必须在业务事务/CAS 中同时核验资源 version、状态与必要的授权水位，不能把本节非持久模拟的边界照搬。

## 11. 配置、readiness 与故障

`growth-api` 将拥有三套不能别名的 MySQL pool：

```text
business MySQL   -> Lottery facts
identity MySQL   -> Session/authentication facts
governance MySQL -> Policy/assignment/audit facts
```

三者都是本路由所需 authority；任一 required pool 缺失、别名、constructor 失败或 ping 失败，启动/readiness 必须失败关闭。Redis 仍只是 Lottery Strategy 可丢弃缓存：Redis 故障可以回源 business MySQL，但不能提供 Principal、Policy、RoleBinding 或 Audit fallback。

运行时配置需要独立 Governance DSN/Secret、pool limits、ping timeout 和 active slot。Secret 只通过文件注入；示例文件只含占位，不提交真实密码。

## 12. 可观测性与隐私

低基数 metric 至少区分：

- `authorization_total{outcome=allow|deny|error,action=simulate,resource_type=lottery.strategy}`；
- `authorization_duration_seconds`；
- `authorization_audit_failure_total{decision=allow|deny}`；
- `governance_policy_load_failure_total{class=missing|corrupt|timeout|dependency}`。

日志允许包含 request ID、canonical StrategyID、稳定错误 class；不得包含 raw Cookie/token/password/CSRF、完整 Policy、全部 bindings、Strategy awards 或客户端可控的大文本。PrincipalID 只进入受保护 audit，不默认进入普通请求日志。

第 33 节不把 metric/log backend、OpenTelemetry 或 SIEM 集成冒充已完成；只建立事件语义、结构化日志和数据库 audit authority。

## 13. 逐阶段实现计划

1. 冻结本文与 ADR，明确 capability、披露、audit、CSRF 和停止线；
2. 扩展 `simulate` capability 与角色 ceiling，完善 60+ 组合矩阵；
3. 新增 forward-only schema、inventory、checksum 与 grants；
4. 实现 Policy publisher 和 exact active snapshot repository；
5. 实现 Governance application enforcer 与 durable audit sink；
6. 抽取 Identity request authenticator，完成 VerifiedSession → Principal 映射；
7. 在 Lottery repository 与 selector 之间接入 required authorizer；
8. 将第三 pool、readiness、Compose Secret 与 route 装配到 production composition root；
9. 执行 unit/integration/race/fuzz、真实 MySQL 与 HTTP/Compose 负向验收；
10. 完成课程、API、QA、第一性原理设计手记、面试问答、Runbook 与全局索引；
11. 只有所有真实证据通过后才冻结第 33 节分支和累计分支。

每一步使用独立小提交并推送；后一步只建立在前一步已验收 tip 上。

## 14. 明确不在第 33 节完成

- React capability store 或 `/api/v1/capabilities`；
- 按 Role/Permission 裁剪导航、路由、页面、字段、按钮；
- 浏览器 direct URL、跨角色、跨对象、重放和完整辅助技术矩阵；
- Policy/RoleBinding 自助管理后台、审批流、SoD 或 JIT access；
- tenant/owner 的权威资源字段与多租户隔离声明；
- 正式 Lottery Draw/Result、幂等、资格、库存与 Benefit 发放；
- OPA、Casbin、OpenFGA、Zanzibar、RLS、独立授权服务或 Policy cache；
- production TLS、可信代理和多区域撤权 SLA。

第 34 节才把服务端提供的最小 capability projection 用于前端体验；第 35 节才形成完整越权与浏览器端到端验收。第 33 节结束时只能说“当前 development-only Lottery simulate route 已被服务端 RBAC 强制保护”。

## 15. 不变量

1. Identity Session 不携带 Role、Scope、Permission 或 Policy snapshot；
2. 浏览器不能声明可信 Principal、tenant、owner、Role 或 Scope；
3. 同一 PolicyID/revision 的内容不可变且可由 canonical digest 复核；
4. active pointer 不会指向半发布、缺行或未经领域校验的 revision；
5. runtime 不拥有 Policy 写权限，publisher 不拥有 audit 伪造权限；
6. unknown capability、role、scope、effect 或损坏 Policy 绝不被忽略后继续；
7. tenant/owner 事实缺失时相关 scope 必须不匹配；
8. deny 覆盖 allow；无 binding、无 permission、scope mismatch 均 deny；
9. allow audit 未确认时 selector 永远不执行；
10. denied 与 unavailable 路径中 selector 调用次数为零；
11. 对象级 forbidden 与 not-found 使用同一公开 404；
12. Redis、前端状态和日志不能恢复授权真相；
13. 当前 `simulate` 永远不等于正式 Draw、参与成功或权益到账；
14. L33 证据不能外推为全站、前端或 production 权限系统完成。
