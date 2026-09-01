# ADR-0028：由 Identity 上下文拥有本地可替换的真实会话认证

- 状态：已接受（仅接受架构决策；实现尚未完成，须通过第 32 节验收）
- 日期：2026-09-01
- 关联章节：第 32 节“真实会话认证”
- 前置决策：[ADR-0027：由 Governance 拥有统一、默认拒绝的访问控制模型](ADR-0027-governance-access-control-model.md)

## 背景

第 31 节只建立了 Governance 的 `Principal`、Role、Scope、Policy 和 Decision 模型。`Principal` 构造成功只能证明值形状合法，不能证明请求者已经登录。当前前端 mock user、角色切换、Header、Cookie 字符串和数据库连接账号都不能成为可信身份。

GrowthOS 需要先形成一条独立、可撤销、可过期、可审计的认证链：credential 经权威验证后创建 server-side session；后续请求只从已验证 session 恢复可信 Principal。第 33 节才使用该 Principal 强制 RBAC，第 34 节才投影前端能力，第 35 节才宣称完整越权与浏览器 E2E 验收。

现有 Redis 只服务 Lottery 可丢弃缓存，ACL 只覆盖 Strategy projection，且使用临时数据卷、`allkeys-lru` 和 fail-open 语义。它不满足会话事实的持久性、不可驱逐性和故障关闭要求。现有 `growthos_app` 连接也只应访问业务表，不能顺势获得 credential/session 权限。

## 驱动因素

1. 浏览器声明不得直接构造可信 Principal；
2. 登录、解析、续期、退出、全量撤权必须具有明确 authority 和失败语义；
3. session 必须支持 idle/absolute expiry、单会话撤销、账户级 epoch 撤销和并发上限；
4. 密码验证的 CPU/内存成本必须有上限，未知账户不能形成明显枚举旁路；
5. Cookie 认证必须同时处理 fixation、CSRF、跨站请求和低披露错误；
6. MySQL 权限、连接池和 readiness 必须按 Identity 边界隔离；
7. 本地 workforce 登录要能被未来 OIDC workforce IdP 替换，而不改 Governance 或业务用例；
8. Redis、JWT、前端状态和日志都不能成为 session authority；
9. Migration、灰度、故障回滚和 commit outcome unknown 必须在编码前有确定处理；
10. 本节不能偷跑授权、能力投影或“全部安全完成”的结论。

## 决定

### 1. 独立 Identity bounded context

认证属于 `internal/identity`，不属于 Governance、业务上下文或 `internal/platform`：

- domain 拥有 WorkforceAccount、LocalCredential、Session 和 AuthenticationThrottle 的不变量；
- application 拥有 authenticate、create/resolve/revoke session、revoke all 和 credential rehash 用例；
- adapter 实现 MySQL repository、Argon2id、随机 token 与 HTTP Cookie；未来 OIDC 必须另立切片；
- composition root 装配依赖、路由、独立连接池和生命周期；
- infrastructure 只可承载通用 HTTP context plumbing，不可验证密码或决定账户状态。

Identity application 输出不可伪造的 `VerifiedSession`，由受限构造路径携带 server-derived human Principal。Principal ID 来自稳定、opaque AccountID；login name、email、provider subject、role、tenant 和 scope 都不是 Principal ID，也不进入客户端可写输入。

本节的本地 workforce provider 是可替换的认证边界，不是永久身份协议。`CredentialAuthenticator` 端口返回已验证的 AccountID 和 credential version。未来 OIDC adapter 必须在独立章节定义 issuer/subject 映射与协议校验；第 32 节不预建一张没有 consumer 的 external identity 表。业务模块和 Governance 不感知本地密码或未来 OIDC 差异，未经显式绑定的外部 subject 永不自动创建账户。

### 2. MySQL 是 account/session 唯一事实源

MySQL 权威保存：

- `identity_workforce_account`：AccountID、规范化 login lookup、Argon2id envelope、credential version、状态、单调 `authentication_epoch` 与审计时间；
- `identity_session`：非秘密 SessionRef、token digest、AccountID、captured epoch、issued/last-seen/idle/absolute expiry、revoked time 和原因；
- `identity_authentication_throttle`：`login|source` dimension、HMAC subject digest、window、failure count、blocked-until 与更新时间。

Migration 只做前向、可重复验证的 additive DDL；迁移器仍是唯一 DDL 身份。原始 password、原始 session token、CSRF token 和可重放 credential 不得进入数据库、Migration、fixture、日志或错误。

Redis 不保存 session、epoch、credential、CSRF authority 或撤权事实，也不作为 MySQL 故障时的 fallback。Redis 重启、驱逐或关闭不得改变已登录会话的权威结果。若未来需要 session read cache，必须另立 ADR，证明撤权一致性、命名空间、ACL、持久性和故障模式。

### 3. 独立 `growthos_identity` 最小权限连接

API 使用独立 MySQL runtime account `growthos_identity` 和独立 pool/DSN：

- 只对三张 Identity 表授予完成已声明用例所需的精确 SELECT/INSERT/UPDATE/DELETE；
- 不授予 DDL、GRANT、`schema_migrations`、Lottery/Marketing/Governance 表或其他 schema 权限；
- `growthos_app` 反向不得读取 credential、token digest 或 session；
- migrator 执行 DDL，grant reconciliation job 在 migration 后精确收敛 grants；
- secret 只从互斥 env/file source 加载，错误和配置摘要必须脱敏。

Identity pool 复用现有 UTC、TLS、bounded pool 和健康检查约束，但不与业务 pool 共享连接身份。连接池容量由数据库预算配置；“最多 5”指每个账户最多五个并发有效会话，不是把 pool 固定为五条连接。

### 4. Argon2id credential envelope

本地密码使用版本化 Argon2id envelope。v1 参数精确为 `m=19456 KiB`、`t=2`、`p=1`、16-byte random salt 和 32-byte output，与产品基线冻结的 OWASP 当前最低配置一致；编码必须包含 envelope version、algorithm、参数、salt 和 digest。验证前严格解析并检查 hard min/max，拒绝可诱发异常资源消耗的数据库值。该参数不是永恒最佳值，只有基准和资源预算支持时才通过同步修改产品基线与 ADR 升级。

密码 adapter 必须：

- 使用 constant-time digest compare；
- 对未知、disabled 和错误密码执行同类 dummy Argon2id 工作，并返回同一个公开错误；
- 在成功验证时只返回内部 `rehash_required` 信号；第 32 节登录事务不自动改写 credential，后续受控 provisioning/credential lifecycle 用 CAS 完成升级；
- 不修改输入字符串、不记录长度以外的敏感特征，并尽快释放中间字节；
- 把 `x/crypto` 作为直接、锁定的依赖，不自制 KDF。

Argon2id 外围设置 process-wide bounded semaphore。v1 默认最大并发 2、允许配置 1～4，默认等待预算 250ms，且等待预算必须小于 Identity request timeout。队列满或等待超时返回低披露 `503 authentication_unavailable`，不无限排队、不冒充 credential 失败或持久 throttle；参数和并发度必须在目标容器内用 benchmark 与内存上限复核后才能变更。

### 5. Opaque session token 与 digest-only persistence

每次成功登录都用 `crypto/rand` 生成至少 256-bit 随机 token，base64url 无填充后只写入 HttpOnly Cookie。随机源失败必须 fail closed，禁止 request ID、时间戳、UUID 或伪随机 fallback。创建登录时总是生成新 token，绝不接受或延续调用者提供的 session，从而阻止 fixation。

MySQL 只保存 `SHA-256(raw token)` 的定长 digest，并建立唯一索引。高熵 token 使无 key digest 可安全查找；raw token 只在当前请求内存中存在，COMMIT 被确认前不得写入响应。日志、trace、metrics、panic、SQL 参数诊断和错误都不得包含 Cookie 或 digest。

Cookie 约束为 HttpOnly、Path `/`、SameSite=Strict、无 Domain；production/staging 必须 Secure 并使用 `__Host-growthos_session`。本地 loopback HTTP 使用不同的 development cookie name，配置加载必须拒绝在非 development 环境关闭 Secure，也必须拒绝把 development 非安全 Cookie 发布到非 loopback origin。删除 Cookie 必须复用完全相同的 name/path/security/samesite 属性并设置过期。

### 6. idle/absolute expiry、epoch 与每账户最多五个会话

默认 idle TTL 15 分钟、absolute TTL 8 小时，且 `idle < absolute`；两者均由有界配置加载并使用注入 Clock。session 在 `now >= idle_expires_at` 或 `now >= absolute_expires_at` 时失效，边界不留一微秒宽限。

解析 session 时以 MySQL 的 account state、captured epoch、revocation 和双 expiry 共同判断，任何不确定均 fail closed。为避免每请求写库，`last_seen_at` 使用最多 60 秒的受控 touch window；conditional update 只延长 idle expiry，不得延长 absolute expiry。并发 resolve/touch/revoke 必须通过条件 UPDATE 保证撤销不会被“续活”覆盖。

每账户最多五个当前 epoch 的未撤销、未过期 session。成功 credential verification 后，创建事务必须：

1. `SELECT ... FOR UPDATE` 锁定 account；
2. 重新检查 account state、credential version 和 epoch；
3. 清理/忽略已失效记录并计算有效会话；
4. 若已有五个，按 `last_seen_at, issued_at, session_id` 的确定顺序撤销最旧会话；
5. 插入捕获当前 epoch 的新 session，再 COMMIT。

因此并发登录在提交点仍最多五个；不能用“先 count 后 insert”的非事务检查。单会话 logout 设置 revoked time；未来 logout-all/password reset/security response 在锁定 account 后递增 `authentication_epoch`，旧 epoch 会话立即失效。epoch 溢出必须拒绝操作并告警，不得回绕；第 32 节公开 HTTP 不提前暴露这些未来管理操作。

### 7. COMMIT outcome unknown

只有 COMMIT 明确成功才能发送 Set-Cookie。若驱动返回网络错误导致提交结果未知：

- 不把请求宣称为成功，不自动用同一 token 重放完整事务；
- 返回 503 且绝不发送 Cookie，即使随后观察到行存在也不把本次响应升级为成功；
- 未下发 raw token 的孤儿 session 对客户端不可用，但仍占容量，按 absolute expiry 与有界 cleanup 收敛；
- observer 仅记录无秘密的 operation/outcome-unknown/session-ref，不记录 token digest；
- 对撤权类操作 outcome unknown 清理浏览器 Cookie、返回 503，并允许同一 token 的条件 revoke 重试或由 Runbook 递增 epoch；
- `database/sql` 的 COMMIT error 不能被解释为 rollback 已确认。

这一路径必须有故障注入测试，不能把 `database/sql` 的 COMMIT error 等同于 rollback 已发生。

### 8. Session-bound CSRF、Origin 与 Fetch Metadata

同源 Cookie 认证的 unsafe 请求采用三层防护：

1. 服务端返回 `v1 + key-id + random nonce + HMAC-SHA-256(dedicated-key, key-id || session-token-digest || nonce)` 的 session-bound CSRF token；
2. 浏览器把 token 仅保存在内存并通过 `X-CSRF-Token` 回传，服务端 constant-time 校验并重新确认 session；
3. `POST/PUT/PATCH/DELETE` 必须有与配置 public origin 精确一致的 Origin；若 `Sec-Fetch-Site` 存在，只接受 `same-origin`，明确拒绝 `cross-site`/`same-site`。

CSRF HMAC 使用独立 keyring，不复用 session token、password 或 rate-limit key；v1 允许一个 active key 与至多一个 previous key，key-id 必须互异，previous key 验证窗口不得超过 8 小时 absolute lifetime。CSRF token 不放 localStorage、不作为 Cookie、不出现在 URL。CORS 保持默认关闭，unsafe body 只接受受限 `application/json`。

匿名 login 没有 session-bound token，因此要求 exact Origin、Fetch Metadata（若存在）、JSON media type、严格 body schema 和限速。缺少 Fetch Metadata 不能绕过 Origin；缺少或错误 Origin 的浏览器 unsafe 请求拒绝。XSS 不属于 CSRF 能解决的问题，仍由 CSP、编码和依赖治理处理。

### 9. 限速与账户枚举防护

登录限速以 MySQL `identity_authentication_throttle` 为多实例一致真相，同时按受信来源和 HMAC 后的规范化 login key 建行；raw IP/login 不入表、不进入 metric label。限速发生在 Argon2 前，但 unknown 与 known account 使用相同 key 与公开响应。

v1 使用 15 分钟 observation window：login dimension 前 5 次失败可进入密码验证，source dimension 前 30 次失败可进入密码验证；越过阈值后从 30 秒开始指数退避，按后续实际失败翻倍并封顶 15 分钟。blocked 请求不执行 Argon2，也不增加计数；窗口无新失败 15 分钟后重置。事务行锁/原子 upsert 保证多实例不会各自放宽预算。成功仅重置 login dimension，不清除 source dimension，避免用已知正确账号冲洗来源预算。该策略不是持久账户锁死：它有明确上限和自动恢复，且错误统一为 `429 authentication_throttled`。

process-wide Argon2 semaphore 是第二层资源预算，不替代持久双维 throttle。Redis 仍不获得 session 或 limiter authority。来源必须来自当前受信 socket/proxy 边界；未建立 production proxy allowlist 前，只能宣称 loopback Compose 拓扑已验收。

### 9.1 v1 有界参数

编码不得再自行发明边界：

- `LoginName` 精确匹配 `[a-z][a-z0-9._-]{2,63}`，不 trim、case-fold 或 Unicode normalize；
- `AccountID` 与 `PrincipalID` 使用第 31 节 canonical identifier 约束，二者由服务端映射，客户端不能提交；
- 登录 password 接受 1～128 Unicode code point 且 UTF-8 编码最多 512 bytes，不 trim、不截断；bootstrap enrollment 额外要求至少 12 code point；
- JSON body 最多 2048 bytes，envelope 最多 256 bytes，login source canonical bytes 最多 128；
- Identity handler 总预算默认 3s，Argon semaphore 等待默认 250ms；
- persistent throttle 使用 15m window、login 5 次、source 30 次、30s 起始/15m 封顶指数退避；
- session touch window 为 60s，只有距离上次权威 touch 已满 60s 时才写 `last_seen_at/idle_expires_at`；
- inactive throttle 行保留 24h；expired/revoked session 行保留 7d，由一次最多 500 行的 Identity maintenance operation 清理；
- active CSRF key 与 optional previous key 的 ID 匹配 `[A-Za-z0-9_-]{1,16}`，每把 key 精确 32 bytes；previous key 最长验证 8h；
- 缺少 `Sec-Fetch-Site` 时，只要 exact Origin 和必要 CSRF 均有效即可服务受控非浏览器客户端；header 存在时必须精确为 `same-origin`；
- `GET /api/v1/session` 可以按 60s touch window 刷新 idle；touch 失败产生 zero trusted Principal + 503；
- `DELETE /api/v1/session` 遇到缺失、过期或已撤销 session 返回统一 401 并清 Cookie；只有已确认 revoke 才返回 204。

### 10. HTTP 契约

Identity HTTP adapter 暴露版本化同源 API：

- `POST /api/v1/session`：严格 JSON `{login,password}`，创建 session，`201` + Set-Cookie + 最小 session DTO；
- `GET /api/v1/session`：解析当前 Cookie，`200` 返回 Principal kind/id 与 expiry，不返回 role/scope/permission；
- `DELETE /api/v1/session`：要求 session、CSRF 和 origin，撤销当前 session，`204` 并清 Cookie；

DTO 使用 exact-field decoder，拒绝 unknown/duplicate/trailing JSON、错误 Content-Type、超限 body、重复同名 Cookie 和多种 credential source。API 不接受 Principal/AccountID/role/tenant/scope header 或 body 字段。所有响应 `Cache-Control: no-store`，不 redirect，保持现有 request ID 与稳定 JSON error envelope。

公开错误映射：malformed 为 400，缺失/无效 credential 或 session 为统一 401，Origin/CSRF 为低披露 403，不支持 media type 为 415，持久 throttle 为 429，Argon semaphore/依赖不可用/无法判定提交为 503，未知缺陷为 500。账户状态冲突或 stale credential version 在外部仍按 401；不得向客户端区分 unknown login、wrong password、disabled account、revoked、epoch mismatch 或具体 CSRF 原因。

### 11. Runtime readiness 与故障语义

Identity 是认证路由的 required dependency。进程启动时必须验证 config、identity pool 和所有 constructor；`/ready` 同时探测业务 MySQL 与 Identity 最小权限连接。Identity MySQL 不可用时 readiness 为 false、登录/解析 fail closed 为 503，禁止退回 mock user、Header Principal、匿名 allow 或 Redis session。

`/health` 仍只表示进程存活。Argon2 semaphore 饱和、单次 rate limit 和无效登录不令 readiness 失败。Redis 可丢弃 cache 的故障语义保持原样，不能影响 Identity 判断。

## 备选方案与否决理由

### A. JWT access token

拒绝。当前模块化单体需要即时 logout、epoch 撤权、五会话上限和低成本轮换；JWT 会把过期 claims、denylist、浏览器存储和 key rollover 复杂度提前引入，最终仍需要 server-side authority。

### B. Redis 作为 session store

拒绝。现有 Redis 是可驱逐、无持久化、Lottery 专属 ACL 的 fail-open cache；扩大 key pattern 会破坏隔离，驱逐也不能等同安全 logout。

### C. 复用 `growthos_app` pool

拒绝。业务查询不应接触 password envelope 或 session digest；独立连接身份能以数据库 grants 证明最小权限，并允许单独撤销和轮换。

### D. 现在直接接入外部 OIDC/SaaS IdP

暂不采用。当前本地开发、离线验收和账户生命周期尚未形成；立即外置会引入 redirect、callback、provider availability 和测试账户依赖。可替换端口与 issuer/subject 映射保留迁移路径。

### E. 进程内 session 或前端/localStorage token

拒绝。进程内状态无法跨重启并发共享；脚本可读 token 扩大 XSS 后果。HttpOnly opaque Cookie + MySQL authority 更符合当前拓扑。

### F. bcrypt/PBKDF2 或无界 Argon2id

拒绝。选择可调内存成本的 Argon2id；无界参数和无界并发会把密码验证变成 DoS 放大器。未来算法替换通过 envelope version 和 verifier registry 演进。

### G. 仅 SameSite 或 double-submit Cookie 防 CSRF

拒绝。SameSite 不是完整 origin policy，独立可读 Cookie 也没有自然绑定权威 session。session-bound HMAC、exact Origin 和 Fetch Metadata 提供可验证的纵深防御。

### H. 持久账户锁死

拒绝。它会成为可远程触发的账户拒绝服务并泄露账户状态。采用短期多维限速、资源 semaphore、统一响应和安全告警。

## 后果与风险

正面结果是认证事实、授权模型、业务数据和缓存 authority 被清晰分离；session 可即时撤销、可解释过期，并能在数据库 grants 层证明隔离；未来切换 workforce IdP 不改变业务 Principal contract。

代价是新增独立 pool、secret/key rotation、Argon2 内存预算、touch write、清理任务和 commit reconciliation。MySQL 成为认证可用性的关键依赖；每请求 authority lookup 增加延迟。可在真实指标证明需要后增加安全缓存，但不能预先牺牲撤权一致性。

主要风险包括登录计时枚举、Argon2 资源耗尽、Cookie 属性漂移、CSRF key 轮换错误、session touch/revoke race、epoch 回绕、COMMIT 结果未知和 grants 过宽。每项都必须有负向或故障注入验收，不能仅靠代码评审。

## Migration、上线与回滚

1. 先添加 forward-only Identity tables、索引、外键、约束和 migration checksum 测试；
2. 创建 `growthos_identity` runtime account，并由 grant reconciliation 精确收敛/证明拒绝；
3. 添加独立 config、secret files、CSRF keyring 和 identity pool，环境示例不得放真实秘密；
4. 部署 repository/application/HTTP adapter，认证路由先独立可验收，不保护现有业务路由；
5. 通过一次性、读取秘密输入的 provisioning 命令创建首个 workforce account；仓库不提交明文密码或可复用 hash；
6. 完成 MySQL、HTTP、Compose、浏览器和日志验收后才把第 32 节标记完成；
7. 第 33 节再逐个把业务用例接到 VerifiedSession + Governance enforcement。

应用回滚使用上一镜像，保留 additive tables 和数据，不执行破坏性 down migration。可关闭新认证路由并撤销 `growthos_identity` 凭证；已创建 session 因尚未接入第 33 节业务 enforcement 不会变成授权旁路。若 token/CSRF secret 事故，轮换 key、递增账户 epoch 或批量撤销 session；若 schema 错误，通过新的 forward repair migration 修复。

## 安全不变量

1. 第 32 节只有 verified local workforce credential 能创建 session；未来 external IdP assertion 必须经独立 ADR 与 adapter 验收；
2. 只有有效 MySQL session 能产生 VerifiedSession/Principal；
3. raw password/token/CSRF 不持久化、不记录、不进入 URL；
4. Redis、前端 store、Header 和业务 DB account 不能恢复可信 Principal；
5. revoke、epoch mismatch、idle/absolute expiry 和技术不确定均 fail closed；
6. 每账户在事务提交点最多五个有效 session；
7. COMMIT 未确认前不得下发 Cookie；
8. unsafe Cookie 请求必须通过 CSRF 与 origin policy；
9. session DTO 不包含 role、scope、permission 或 authorization Decision；
10. 第 32 节完成不等于任何业务路由已经授权。

## 验收

- domain/application：状态、双 expiry 边界、epoch、单/全量 revoke、Clock、defensive copy 和错误分类；
- Argon2：envelope malformed/参数上下限、dummy hash、rehash CAS、semaphore 超时、benchmark 和容器内存峰值；
- token/Cookie：随机源失败、digest-only、唯一冲突、fixation、重复 Cookie、production Secure/`__Host-` 和精确删除；
- concurrency：并发登录始终最多五个，resolve/touch/revoke race 不复活 session，`go test -race` 通过；
- commit fault injection：已提交、未提交、无法判定三条路径均不盲重放；任何 COMMIT error 都不下发 token；
- CSRF：valid、wrong session/epoch/key、跨站 Origin、Fetch Metadata、缺失 header、key rotation 和 constant-time compare；
- rate limit：MySQL login/source row、阈值/指数退避/自动恢复、多实例并发、无高基数敏感 label、unknown/known account 统一公开错误；
- real MySQL：clean migration、second-up no change、schema/index/FK、round trip、expiry/revoke/epoch、并发事务；
- grants：`growthos_identity` 只允许声明 DML，拒绝 DDL/业务表/`schema_migrations`；`growthos_app` 拒绝 Identity 表；
- HTTP/Compose：真实 Cookie jar 完成 login/current/csrf/logout/replay，错误 envelope/no-store/request ID 正确；
- Redis 隔离：Redis restart/eviction/offline 不改变 session authority，且不存在 session key；
- readiness：Identity pool down 时 ready=false、auth=503、health 仍表示存活，无任何 mock/fallback；
- 泄密检查：响应、结构化日志、Nginx access log、数据库和错误中均无 password/raw Cookie/token/CSRF；
- 全仓执行普通测试、race、vet、format、doccheck、Web verify、Compose acceptance 和 `git diff --check`。

## 章节停止线

- 第 32 节只交付 credential → authoritative session → trusted Principal；
- 第 33 节才在服务端加载可信 Resource facts 并强制 Governance RBAC；
- 第 34 节才由服务端 capability projection 裁剪前端导航、路由和操作；
- 第 35 节才覆盖 direct API、跨账户/角色/对象、Cookie/CSRF 和浏览器端到端越权矩阵；
- 在第 33～35 节验收完成前，不得声称“权限系统已完成”或以 UI 隐藏替代服务端安全。
