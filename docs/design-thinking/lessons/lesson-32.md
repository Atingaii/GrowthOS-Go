# 第 32 节设计手记：从第一性原则推导真实 Session 认证

- **课程正文：** [第 32 节课程](../../course/part-04/lesson-32-real-session-authentication.md)
- **API 契约：** [第 32 节 API](../../api/lessons/lesson-32.md)
- **QA：** [第 32 节 QA](../../qa/lessons/lesson-32.md)
- **面试问答：** [第 32 节面试问答](../../interview/lessons/lesson-32.md)
- **运行手册：** [Identity Session 运维手册](../../runbooks/identity-session-operations.md)
- **决策：** [ADR-0028](../../decisions/ADR-0028-identity-session-authentication.md)
- **记录更新日期：** 2026-09-03

> 这不是“最后代码长什么样”的倒序说明，而是一个设计者如何从资产、信任、失败和证据逐步收敛到当前实现。源码存在不等于真实环境已验收；当前 focused Go、独立 MySQL 8.4.11、HEAD `9fc4e06` 的完整 development Compose/Session wire、maintenance 与浏览器核心旅程已有实际证据。仍保持 `PENDING` 的是 raw Content-Length absent/zero/mismatch 的 proxy 变体、真实 issue/revoke COMMIT outcome-unknown fault、staging/production TLS与可信代理，以及浏览器 storage/console、更广设备/辅助技术和最终冻结门禁。

## 1. 真正的问题不是“做一个登录页”

登录页只收集输入。系统真正需要证明的是：

```text
此请求持有的 bearer
  确实由本系统在一次已确认的 credential verification 后签发，
  仍未撤销，未越过 idle/absolute expiry，
  对应 account 仍 enabled，captured epoch 仍匹配，
  因而可以恢复一个 server-derived human Principal。
```

其中任何一步未知，都不能产生 trusted Principal。这个结论决定了：

- 认证事实不能由 React store、Header、JWT payload 或 mock user 自报；
- Session authority 必须有明确持久化、并发和撤销语义；
- 密码验证、Cookie、CSRF、Origin、限速、数据库权限和故障处理是一条链，而不是互不相干的“安全功能”；
- 登录成功仍没有回答“该 Principal 能否发布 Activity”，授权必须留给后续章节。

## 2. 第一性原则：先列不可消失的事实

### 2.1 资产

| 资产 | 泄漏后果 | 篡改后果 | 不可用后果 |
| --- | --- | --- | --- |
| password | 账户接管、跨站复用风险 | 伪造登录 | 用户无法认证 |
| raw Session token | 当前会话接管 | 以 bearer 冒充 | 当前设备掉线 |
| password envelope | 离线猜测 | 拒绝服务或后门 hash | 本地登录失败 |
| Session/token digest rows | 行为关联、撤权事实暴露 | 绕过到期/撤销 | 所有 current 请求失败 |
| throttle key/state | login/IP 关联或绕限速 | 放大 hash、锁死账户 | 登录被保守拒绝 |
| CSRF key/token | 伪造 unsafe 请求 | 退出等操作被滥用 | unsafe 操作失败 |
| Principal mapping | 身份错配 | confused deputy | 授权链无法建立 |

### 2.2 信任边界

浏览器控制 URL、headers、JSON、Cookie bytes 和时序；它不控制 account 状态、Principal mapping、服务端时钟、数据库提交结果或 Policy。MySQL 是存储 authority，但坏行仍需领域恢复校验；数据库类型正确不等于认证事实合法。反向代理能改变网络 peer 的含义；没有可信代理规则时，`RemoteAddr` 只能代表当前直连 socket。

### 2.3 必须保留的安全性质

1. **不可伪造：** 只有受控构造路径能输出 trusted Principal；
2. **可撤销：** 单 Session 和 account epoch 都能使 bearer 失效；
3. **有界：** body、字符串、hash 参数、hash 并发、事务、会话数、重试和 cleanup 都有上限；
4. **低披露：** 外部错误和日志不区分可枚举内部状态；
5. **失败关闭：** dependency/timeout/corrupt/unknown 不退回 anonymous allow 或旧 snapshot；
6. **最小权限：** API、provisioner、migrator、maintenance 用不同能力；
7. **可证明：** 单元测试、真实 MySQL、Compose、HTTP、浏览器和 TLS 证据不能互相替代。

## 3. 为什么 Identity 必须是独立 bounded context

### 3.1 不放在 Governance

Governance 负责“已知主体对资源能否执行动作”，不应理解 password、Cookie 或 Session token。否则 Policy evaluator 会同时承担密码学、HTTP 和存储生命周期，失去纯函数性质，也无法在未来用 OIDC 替换认证来源。

### 3.2 不放在业务模块

Marketing、Lottery、MCP 和 Agent 都会消费主体。如果每个模块自己解析 Cookie，就会复制过期、撤销、CSRF 和错误语义，并出现“同一个 bearer 在不同模块得到不同身份”的分裂事实。

### 3.3 不放在 platform

platform 可以提供通用 config、HTTP、MySQL 和日志机制，却不应决定 workforce account status、credential version 或 authentication epoch。这些是产品身份语义，不是基础设施细节。

因此 [Identity domain](../../../internal/identity/domain)、[application](../../../internal/identity/application)和 adapters 形成独立上下文；application 的输出才可以映射为第 31 节的 human Principal。

## 4. 为什么先做本地 workforce provider，而不是直接 OIDC

当前真实约束是本地开发、离线 Compose 和可重复学习分支。直接接入第三方 IdP 会新增 redirect/callback、issuer、JWKS、state/nonce、测试租户和 provider outage，却没有现成组织目录可依赖。

本地 provider 的最小模型只包含 account、login lookup、password envelope、status、credential version、epoch 和 Principal mapping。它不复制消费者用户、组织结构或企业 Role，也不开放自助注册。未来 OIDC adapter 可以替换 credential verification 与 issuer/subject mapping，但仍把结果交给同一 Session service；Governance 与业务 use case 不需要知道密码是否存在。

代价是当前必须自己承担密码生命周期和 bootstrap 运维。为控制范围，本节只提供 INSERT-only enrollment，不实现修改、重置或恢复；这些能力不能通过给 provisioner 增加 UPDATE 临时凑出。

## 5. 从人类密码低熵推导 Argon2id

### 5.1 为什么不能“加盐 SHA-256”

盐解决相同密码得到相同 digest 与预计算表问题，却不增加单次猜测成本。攻击者拿到 envelope 后可以并行高速试探。密码 KDF 必须故意消耗时间和内存，并允许随硬件重新校准。

### 5.2 为什么当前选择 Argon2id

当前 [passwordhash adapter](../../../internal/identity/adapter/passwordhash)采用 RFC 9106 定义的 Argon2id 与版本化 PHC envelope。v1 固定 `m=19456 KiB, t=2, p=1`、salt 16、output 32 bytes；这与当前 OWASP 最低建议方向一致，并通过 `x/crypto/argon2` 避免自制 KDF。

bcrypt 仍比快速 hash 安全，但其工作因子主要调节 CPU、密码长度/编码语义也不同；PBKDF2 在合规场景有价值，但当前没有 FIPS 约束。选择 Argon2id 不是“新所以更好”，而是当前 threat/resource trade-off。

### 5.3 为什么必须先解析上限再 hash

自描述 envelope 的参数来自数据库，而数据库可能损坏或被越权写入。若直接相信 `m`/`t`，一个坏值就能要求数 GiB 内存或极长 CPU，形成存储驱动 DoS。因此 parser 先验证 algorithm/version、字段、base64、salt/output 和 hard range；失败不执行 attacker-selected work。

### 5.4 为什么有 dummy work

unknown account 若立即返回，known account 才做 Argon2，会产生明显时间分支。[login tests](../../../internal/identity/application/login_test.go)要求 unknown、wrong 和 disabled 的公开结果相同，并让 unknown 走固定 dummy envelope。

这只是降低易见差异，不能承诺网络端完全 constant-time：数据库 cache、调度、GC 和链路抖动都可能不同。正确表述是“统一主要密码工作与公开语义”，而不是“彻底消除所有侧信道”。

### 5.5 为什么还要 semaphore

单次 hash 有界不代表并发总内存有界。攻击者可以同时提交大量请求，所以进程使用默认容量 2、允许 1～4 的 semaphore，默认等待 250ms。满载时 503，避免无界 goroutine 排队占内存。

Apple M2 Pro 的 10 次 baseline：serial `26.638354ms/op`、parallel capacity=2 `14.179475ms/op`；profile memory 分别 19/38 MiB，进程 max RSS `107,823,104` bytes。这只能证明本机当前版本的回归点，不能直接外推 production p99 或吞吐。

这里还有一条由 fuzz 反推实现顺序的真实教训：旧代码把 slot receive 与 timer channel 放在同一个 `select`；capacity=2、occupied=1 时若 1ms timer 同时 ready，调度可以随机返回 timeout，制造错误 503。`5af29e2` 改为先检查 context，再 nonblocking 获取现有 slot，只有满槽才创建 timer，并把 `(2,1,false)` 变成 seed。修复后 passwordhash count=10/race 与 10 秒 WorkGate fuzz（625,627 executions）通过。这是资源准入时序缺陷，不是安全绕过。

## 6. 从“需要即时撤权”推导 server-side Session

当前需求包含单会话 logout、account epoch 撤权、每账户最多五个 Session、disabled 立即失效和 deterministic eviction。若使用自包含 JWT，服务器仍需要 denylist/epoch lookup 才能即时撤权；此时“无状态”已经消失，却多了 claims 过期、signing key rollover 和浏览器 token 存储复杂度。

MySQL 已经是 account authority，因此 v1 让 [MySQL repository](../../../internal/identity/adapter/mysqlrepo)同时保存 Session authority。每次 current 需要数据库，但得到统一、强一致的状态语义。若未来 profiling 证明读放大不可接受，再评估 cache；不能在没有撤权一致性设计时先放进 Redis。

## 7. 为什么当前 Redis 明确不能存 Session

现有 Redis 服务 Lottery 可重建 projection，允许 eviction、临时 volume 和 fail-open cache。Session 恰好相反：miss 不可当匿名 allow，eviction 不能当正常 logout，Redis restart 不能改变撤销事实，Lottery ACL 也不能看到 bearer。

选择 MySQL 的代价是 current latency、数据库可用性耦合和 touch 写入。当前通过独立 pool、60 秒 touch window、有界查询和 readiness 管理；不是通过第二份 authority 双写。

## 8. 从 bearer 泄漏风险推导 opaque token + digest-only

### 8.1 token 为什么是纯随机

[Session token](../../../internal/identity/domain/session.go)是 32 random bytes，不编码 account、role、时间或 PII。request ID、时间戳、sequence 和 UUID 都不能作为 fallback。随机源失败直接失败关闭。

### 8.2 为什么数据库存 SHA-256 digest

随机 token 有 256-bit 熵，SHA-256 在这里是 lookup digest，不是密码 KDF。数据库泄漏时不能直接拿行值作为 Cookie；固定 32-byte digest 便于唯一索引和 exact lookup。原始 token 只存在于请求/响应内存与 HttpOnly Cookie。

### 8.3 为什么提交前不能发 Cookie

若先发 bearer 再发现事务失败，浏览器会得到无法解析或状态不明的 credential。application 只在 COMMIT 明确成功后交付 token；HTTP 层随后构造 Set-Cookie。若提交已成功但后续 CSRF/Cookie 构造失败，客户端仍不得得到可用 bearer，但数据库可能留下孤儿 Session，后续由到期与 maintenance 收敛。

### 8.4 为什么碰撞只重试 token candidate

token digest unique collision 的因果明确：换一个随机 candidate 即可，且最多三次。deadlock、timeout、network 或 COMMIT error 的结果可能未知，重跑完整事务会多发 Session或错误淘汰其他设备，不能归入同一个“retryable DB error”。

## 9. 从长期 bearer 风险推导双到期与 touch

只有 idle expiry 会被持续访问无限延长；只有 absolute expiry 会让短暂离开设备仍保持很久。两者同时存在：idle 15 分钟限制无人操作窗口，absolute 8 小时限制一次认证的最长生命。

每请求写 `last_seen_at` 会放大行锁与 binlog，因此只在 60 秒 touch window 后更新。touch 使用条件 UPDATE，不能把被并发 revoke 的 Session 续活，也不能越过 absolute。边界采用 `now >= expiry` 即失效，避免相等时刻的隐性宽限。

## 10. 从多设备与安全响应推导五会话上限和 epoch

每 account 最多五个当前 epoch 的有效 Session。新登录事务先锁 account、重新验证 credential/epoch，再读取并锁定最多六个 active Session；若已经超过五个，立即 fail closed，不能让 replacement hint 把坏存量伪装成可修复状态。合法、同 account 且仍 active 的 replacement hint 优先按 `security_response` 撤销；移除 hint 后若仍恰有五个，才按 `last_seen_at, issued_at, session_ref` 撤销唯一确定性最旧值，然后 insert/commit。这个顺序既避免“替换本设备却误踢另一设备”，也保证并发登录不能通过事务外 count 同时越界。

单会话 revoke 适合用户退出；`AuthenticationEpoch` 适合密码泄漏、禁用或安全响应时让全部旧 Session 失效。Session 捕获创建时 exact epoch；resolve 时不匹配立即失败。epoch 必须 non-zero、单调且不回绕。

当前公开 API 没有 logout-all。保留领域能力不等于未经产品/运维设计就暴露 endpoint。

## 11. 从昂贵 hash 前置攻击推导双维持久 throttle

### 11.1 为什么不是只按账户

只按账户会让攻击者锁死受害者；只按 IP 会被代理/NAT 汇聚，也可被分布式来源绕过。当前同时按规范 login 和可信 source 建维度：login 阈值 5，source 30；成功只清 login，不让一个正确账号冲洗来源预算。

### 11.2 为什么 key 要 HMAC

直接存 login/IP 会扩散 PII，也可从低熵值反查。普通 hash 对字典化 login/IP 仍可枚举；专用 HMAC key 让数据库单独泄漏时难以离线恢复原值。domain separation 和长度前缀避免不同维度/拼接歧义。

### 11.3 为什么必须 reservation

多实例如果都在 hash 前只读 `failure_count=4`，它们会同时通过并超发。当前在一笔短事务中按固定 key 顺序锁两行，检查 `failure+inflight`，并同时增加 reservation；然后释放连接再做 Argon2。receipt 携带 exact epoch，只能 finalize 一次。

### 11.4 为什么需要 lease 与 probe

进程可能在 hash 后、finalize 前崩溃。reservation 最晚 3 秒过期；回收时前进 epoch，旧 receipt 不能扣新批次。阈值达到后 backoff 到期必须允许一个 probe，否则 failure count 永远不降低、账户成为永久锁死。

代价是崩溃后短时间保守拒绝；这是比超发昂贵 hash 更符合资源边界的选择。

### 11.5 代理现实

[request guard](../../../internal/identity/adapter/requestguard/guard.go)只信 socket peer，拒绝任意 forwarding header。这关闭了客户端伪造 IP 的简单漏洞，但 Nginx 拓扑下 Go 看到的是代理 peer，多客户端可能共享 source budget。生产启用真实 client IP 前必须设计可信 proxy allowlist、header 清洗和 hop 规则；不能直接“开始信 X-Forwarded-For”。

## 12. 从 Cookie 自动携带推导 CSRF 三层防线

### 12.1 SameSite 不是完整答案

SameSite Strict 是浏览器策略，受导航语义、兼容性和同站子域影响；它也不验证请求来自预期 canonical origin。因此 unsafe Session 请求还校验 Origin 和 Fetch Metadata。

### 12.2 为什么 login 也校验 Origin

匿名 login 没有既有 Session CSRF token，却仍可能遭 login CSRF：攻击者让受害者浏览器登录攻击者账户，后续行为写入错误身份。POST 因此要求 exact Origin、严格 JSON/Content-Type 和 throttle。

### 12.3 为什么 logout token 要绑定 Session

一个全局 token 被窃取后可作用于其他 Session；double-submit Cookie 又会让 token 进入另一 Cookie。当前 [CSRF adapter](../../../internal/identity/adapter/csrf/csrf.go)用独立 key对 `key-id + session digest + nonce` 做 HMAC-SHA-256，生成 `v1.<id>.<nonce>.<mac>`。wrong-session token 即使格式正确也失败。

### 12.4 active/previous 为什么只有两把

轮换需要短暂兼容在途页面，但无限 keyring 会扩大验证面和遗忘旧 key 风险。active 签发，active + optional previous 验证；previous 接受时间不超过 Session absolute 8 小时。key ID 有 1～16 字符封闭 grammar，CSRF key与 throttle key必须不同。

### 12.5 XSS 边界

HttpOnly 保护 raw Session bearer 不被普通 JS 读取；CSRF token 必须由应用 JS 放入 header，因此存在组件内存。XSS 仍可能借用户浏览器发同源请求，CSRF 不能解决 XSS。需要 CSP、输出编码、依赖治理和第 35 节浏览器安全验收。

## 13. 从跨环境混淆推导两种 Cookie name

development exact HTTP loopback 无法使用 Secure `__Host-` Cookie，所以使用 `growthos_dev_session`；staging/production HTTPS 使用 `__Host-growthos_session`。两者均 host-only、Path `/`、HttpOnly、SameSite Strict。

不同名称避免把本地非 Secure bearer 携入部署环境，也让配置能拒绝 alternate cookie。迁移便利性变差，但“同时接受新旧名”会扩大 credential source 并制造优先级歧义。

## 14. 从解析器差异推导严格 HTTP grammar

宽松 JSON、代理和框架可能对 duplicate keys、Content-Length/TE、Content-Type 参数和 surrogate 采用不同解释。安全边界需要让客户端、Nginx、Go handler 和 application 看到同一请求。

[Session HTTP adapter](../../../internal/identity/adapter/httpapi/session.go)因此固定：

- 一条 `/api/v1/session`、POST/GET/DELETE 三方法；
- 三者无 query、无伪造 Principal/Role/Scope/Tenant/Authorization header；
- POST exact `Content-Type: application/json`、known `Content-Length 1..2048`、无 TE/trailer；
- body exact `{login_name,password}`、唯一 fields、单一 JSON、有效 UTF-8 与 surrogate；
- GET/DELETE bodyless；DELETE exact Origin、Cookie、单一非空 CSRF；
- 201/200 返回最小 snapshot，204 完全无 body；
- 所有响应 no-store，并设置专用 Session 响应的 CSP/frame/referrer/permissions/nosniff 边界。

严格会增加非浏览器客户端接入成本，但 contract 可机械测试，避免“看起来一样”的多种表示进入认证核心。

## 15. 从枚举风险推导低披露错误

对外错误必须足够让客户端决定下一步，却不能泄露 account/CSRF/session 内部状态：

| 类别 | 对外语义 | 内部仍需区分 |
| --- | --- | --- |
| login credential 失败 | `401 authentication_failed` | unknown/wrong/disabled/stale |
| current/logout 无 Session | `401 unauthenticated` | missing/malformed/expired/revoked/epoch |
| Origin/CSRF | `403 request_origin_rejected` | origin/fetch/key/mac/session binding |
| persistent throttle | `429 authentication_throttled` | login/source/block expiry |
| dependency/gate/unknown | `503 authentication_unavailable` | DB/random/deadline/capacity/commit |
| revoke commit unknown | `503 session_revocation_indeterminate` | 必须阻止“已安全退出”假象 |

日志也不能把低披露重新打破。当前只允许 operation、result class、request ID；维护成功可记录两个 bounded count。

## 16. COMMIT outcome unknown：最重要的失败思维

`database/sql` 的 `Commit()` 返回网络错误时，服务器可能已经提交，也可能没有。应用层没有证据选择其中一个。

### 16.1 签发

不返回成功、不发 raw token、不自动重跑。如果行实际存在，它没有交付 bearer，是孤儿 Session，会占短期容量并最终过期/清理。这比重试后签发两次更可控。

### 16.2 撤销

浏览器 Cookie 被清理，返回 `session_revocation_indeterminate`。这降低当前设备继续使用 bearer 的机会，却不能声称服务端行已撤销；用户界面必须诚实提示，运维可通过 account epoch 做批准后的安全响应。

### 16.3 maintenance

Session 阶段 commit unknown 时停止，不开始 throttle；Session 已确认提交、throttle 后续失败时明确报告部分进度。把两个表放一个巨大事务可以获得全或无，却扩大锁时间和失败域；当前选择两个独立、小事务并让结果语义显式。

## 17. 从最小权限推导三类数据库进程

复用 `growthos_app` 最方便，却会让业务 SQL 读取 password envelope/session digest；复用 migrator 更危险，会把 DDL/GRANT 带进 HTTP runtime。当前分离：

- API/maintenance：`growthos_identity`；
- provision：`growthos_identity_provisioner`，workforce 仅 INSERT；
- migration/grant reconciliation：专用受控进程；
- business：`growthos_app`，反向不可读 Identity。

独立 pool 不只是配置字段不同；composition root 还拒绝同 username/credential 和同底层 pool alias。这样权限泄漏可被 MySQL deny 行为证伪，而不是只靠代码约定。

代价是 Secret、连接和 readiness 更复杂。安全边界越重要，越值得用基础设施能力作为第二道约束。

## 18. 为什么 provision 是 INSERT-only one-shot

本节需要可登录账号，却不具备公开注册、密码恢复和管理员授权系统。如果把 bootstrap 暴露成 HTTP，就需要先解决“谁能创建管理员”的循环依赖。

[provision CLI](../../../cmd/growth-identity-provision)只有 exact `create` + account/login/principal/password-file。status 固定 enabled，credential/auth epoch 固定 1，envelope由进程生成，时间来自可信 clock。数据库账号不能 SELECT，所以成功后也不 readback；duplicate 不是幂等成功，commit unknown 不重试。

### 18.1 为什么 password file 比 flag/env 更合适

flag 易进 shell history/process list，env 易进入进程诊断/Compose materialization。caller-owned 0600 regular file 让秘密有可审查的文件边界。wrapper 创建私有 snapshot，挂载后明确清理；caller 原文件保留，由调用者负责。

覆写临时文件不能在 SSD/COW 上证明物理擦除，所以正确保证是“最小化暴露、unlink、无应用层残留”，不是“密码已不可恢复”。

### 18.2 真实运行为什么推翻了 `compose --wait` 的直觉

长驻 provision 实际运行时，`mysql-grants` 已快速成功并成为 `exited:0`，但 `docker compose up --wait` 仍将它判成等待失败。`--wait` 的“服务保持 running/healthy”模型与“成功后退出”的 prerequisite 语义不一致；继续把工具退出码当业务事实会错误阻断正确流程。

提交 `af4245e` 没有忽略错误，而是为 provision/maintenance wrapper 建立专用状态机：最多180秒轮询唯一container，`created/running/restarting`继续等，只有exact `exited:0`成功；ambiguous identity、非零退出、意外state、inspect失败或超时都fail closed。这是“先定义成功证据，再选择工具原语”的真实例子。

## 19. 为什么 maintenance 不做通用清理框架

Session/throttle 历史会增长，但开放 table/cutoff/batch/loop 给操作者会把一次维护命令变成数据删除平台。第一性原则是：只删除已经由产品 retention 判定无须保留、且在 DELETE 时仍满足条件的行。

[MaintenanceOperation](../../../internal/identity/application/maintenance.go)从一个 server clock snapshot 固定：

- Session：`absolute_expires_at <= observed-7d` 或 `revoked_at <= observed-7d`，250；idle expiry 本身不够；
- throttle：`row_expires_at <= observed` 且无 inflight，250；24h retention 已编码进 row，不再额外减一天；
- 总上限 500，预算不互借。

[maintenance runtime](../../../cmd/growth-identity-maintenance/production.go)固定一条连接、一次 attempt、无重试。专用 config 只读取必要 env，operation 1～30s，并在 MySQL read/write deadline 内预留 1s 取消/清理。错误返回 zero config/result，所有格式边界脱敏。

没有 dry-run 是一个有意取舍：查询候选与真实 DELETE 之间会漂移，dry-run 也不能证明稍后会删同一集合。当前用小 batch、事务内重新校验、真实 fixture 与低披露 count 建证据。若运营确有预览需求，应设计只读、不可泄漏 digest 的独立观测接口，而不是复用删除 SQL。

### 19.1 真实 fixture 如何证伪误删与不收敛

官方 disposable project `growthosl2465e15560c550fd33fc6901bf` 第一次运行删除Session/throttle/total `2/1/3`，第二次必须精确`0/0/0`。仅看“命令exit 0”无法证明正确，因此同时比较active Session fingerprint不变、fixture residue `0:0:0`，并执行其他功能、失败、性能和grant断言。

清理也属于验收：本次disposable containers、volumes、networks、5个临时images、builder/state/secrets全部精确移除，只保留可复用`growthos/identity-maintenance:lesson-32` image。这个PASS证明当前fixture和装配，不代表任意生产backlog、锁等待或调度频率。

## 20. 从浏览器不可信推导 strict transport

[共享 httpClient](../../../web/src/api/httpClient.ts)集中同源 path、credentials、no-store、redirect error、timeout/abort、error envelope 和 gateway 语义，避免 Session adapter 复制一套稍有不同的安全逻辑。[Session API](../../../web/src/api/sessionApi.ts)再收窄到 5 秒上限、201/200/204 和 exact snapshot。

### 20.1 为什么严格解码成功响应

TypeScript interface 在运行时不存在。若代理、旧后端或被篡改响应返回 `kind=service`、假时间或额外 capability，直接 cast 会把不受支持状态带入 UI。decoder要求 exact keys、`authenticated=true`、human、canonical ID、真实 RFC3339 和可见 ASCII CSRF。

### 20.2 为什么 204 连 header/body 都严格

logout 的成功语义必须唯一。带 JSON、Content-Type、Transfer-Encoding、非零 Content-Length 或任意 bytes 的 204 都可能是代理/后端 contract drift；adapter归为 contract failure，不猜测。

### 20.3 为什么不自动重试

GET 理论上可重试，但统一自动重试会掩盖 outage，并可能延迟真实错误；POST/DELETE 又有提交不确定和重复副作用。当前调用者只能显式 retry current 或 logout，保持用户动作与证据一致。

### 20.4 password/CSRF 的诚实描述

[LoginPage](../../../web/src/pages/auth/LoginPage.tsx)不把 password放 React state，请求发起后清 input；不进 URL/storage/log。它仍必须作为 JS string 序列化并可被 DevTools Network 观察，无法可靠 zeroize。CSRF 不渲染、不持久化，但保留在 [session boundary](../../../web/src/layouts/useSessionBoundary.ts)组件内存供 DELETE 使用。

## 21. UI 状态为什么不能压成 logged-in boolean

至少需要：

```text
checking
anonymous
authenticated(snapshot)
unavailable(error)
```

以及 login `idle/submitting/error`、logout `idle/logging-out/error`。current 401 才是匿名；network/503 表示“无法确认”，不能显示登录表单并诱导重复登录。logout ordinary failure 保留 snapshot；confirmed/已失效清除；revocation-indeterminate 清浏览器侧状态但明确不证明服务器撤销。

AbortController 与 generation 用来抛弃卸载或新一代请求后的迟到响应。不过前端竞态测试仍应持续覆盖 retry/login/logout 交错，不能因为有一个 counter 就宣称所有并发状态都正确。

当前 Auth boundary 只包 `/login`、`/session`；系统状态和错误页保持公开，业务/Admin/MCP/Agent 页面不在本节改造。这条停止线避免用登录 UI 偷跑第 33/34 节。

## 22. readiness 为什么与 health 分开

`/health` 回答进程是否存活；`/ready` 回答是否应接流量。Identity MySQL down 时进程仍能响应 health，却不能安全认证，所以 readiness 必须 false。Argon semaphore 短时饱和、一个错误密码或某个用户被 throttle 则不是全局依赖故障，不应让实例摘除。

business 与 Identity pool 并发 probe，任一 required authority 失败就 not ready。不能为了“可用性”在 Identity 故障时恢复 mock user、信任 header、使用旧 frontend snapshot 或把 Redis 当 Session fallback。

## 23. Secret 与轮换如何从消费边界推导

每个进程只能挂载它需要的 Secret：API 收 business/Identity/Redis password 与 throttle/CSRF key；provision只收 provisioner DB password + one-shot enrollment snapshot；maintenance只收 runtime Identity password；Web 不收任何 Secret。

CSRF active/previous 支持受控重叠；throttle HMAC key 不可随意轮换，因为旧行将无法定位，需先设计 dual-digest/retention 过渡。MySQL credential 轮换需要数据库账号、Secret 文件和消费者重启的顺序证据。Session raw token不使用全局 signing key，无需 bearer signing-key rollover，但仍依赖数据库和 Cookie 生命周期。

具体步骤见 [运维手册](../../runbooks/identity-session-operations.md)。任何 partial Secret set、reuse、权限异常或 staging/prod TLS 缺失都应停止。

## 24. 失败模式与操作者动作

| 失败 | 系统行为 | 操作者动作 | 禁止动作 |
| --- | --- | --- | --- |
| Argon gate saturated | login 503，无 Cookie | 看资源/攻击指标，校准并发 | 把它伪装成错误密码、无限扩容 |
| Identity DB down | current/login 503，ready false | 恢复 DB/TLS/grant | header/mock/Redis fallback |
| bad stored envelope | 低披露认证失败/不可用，无 Principal | 隔离坏行、走受控 credential lifecycle | 自动降参或直接 compare |
| issue commit unknown | 503，无 bearer | 保留 request ID，等待到期/受控核查 | 自动重放登录 |
| logout commit unknown | clear + indeterminate 503 | 告知用户，必要时 epoch 安全响应 | 宣称安全退出 |
| provision duplicate | 失败，无 readback | 用另一个授权身份核查 | upsert/授予 SELECT |
| maintenance first stage unknown | 停止 second stage | 核查低披露结果，等批准再跑 | 内部自动 retry |
| grant drift | readiness/acceptance 失败 | reconciliation 到 exact allowlist | 给 `*.*` 临时权限 |
| proxy topology changed | source 语义失真 | 重做 trusted proxy threat model | 直接信 caller header |

## 25. 为什么不是其他方案

### 25.1 前端 localStorage token

脚本可读 bearer 扩大 XSS 后果；刷新/多 tab 更方便不是足够理由。HttpOnly Cookie把 bearer读取权留给浏览器网络栈，代价是必须认真做 CSRF。

### 25.2 进程内 Session

实现简单，但实例重启即丢、扩容不共享、无法形成可靠撤权事实。它只适合单进程 demo，不符合当前 Compose/API 边界。

### 25.3 JWT

适合无需即时中心撤权或跨服务验证的场景。当前每账户五 Session、epoch、即时 logout仍会引入 state；提前 JWT只增加 key/claim/denylist 复杂度。

### 25.4 Redis authority

当前 Redis 的 durability/eviction/ACL/failure mode与认证事实冲突。未来可以新建安全专用 Redis，但必须先证明持久性与 MySQL一致性，而不是复用 Lottery cache。

### 25.5 数据库保存 raw token

查询更直接，却让 DB read 泄漏即可直接重放。digest-only用极小实现成本缩小爆炸半径。

### 25.6 只按 IP 限速

NAT/代理误伤，分布式攻击绕过；只按 login又容易账户锁死。双维仍有代理汇聚 trade-off，但更平衡，且保持明确恢复上限。

### 25.7 无状态 double-submit CSRF

实现更通用，但 token可能进入 Cookie且不自然绑定服务器 Session。当前已有 Session digest，HMAC绑定能明确防止跨 Session复用。

### 25.8 一个万能数据库账号

减少 Secret和pool，却让一次 SQL injection/代码缺陷越过业务、credential、DDL边界。独立身份的运维成本换取可验证最小权限。

### 25.9 公共 account admin API

没有第 33 节授权强制前无法安全暴露。one-shot provision用操作面解决 bootstrap循环，代价是账号生命周期功能暂缺。

### 25.10 cron 无限 cleanup

自动化能控制增长，但在 retention、锁和失败证据未完成前会持续扩大错误。先固定 one-shot + 250/250，真实 backlog出现后再决定调度频率。

## 26. 关键 trade-off 账本

| 决定 | 获得 | 付出 | 重评证据 |
| --- | --- | --- | --- |
| MySQL Session | 即时一致撤权、单一 authority | 每次 resolve DB latency | profile/availability 显示瓶颈 |
| Argon2id 19MiB | 提高离线猜测成本 | 内存/CPU和并发限制 | 目标容器 benchmark |
| strict JSON/header | parser一致、攻击面窄 | 客户端兼容性低 | 有真实受控客户端需求 |
| SameSite Strict | 强跨站默认 | 某些跨站入口不可用 | 产品需要 federated flow |
| source=socket peer | 不信伪造 forwarding header | 代理后共享 budget | 明确 proxy allowlist/topology |
| five Sessions | 控制遗忘设备 | 第六次会踢最旧设备 | 用户行为/客服数据 |
| INSERT-only provision | bootstrap最小权限 | 无查询/更新便利 | account admin授权已落地 |
| maintenance 250+250 | 小事务、互不饥饿 | backlog可能多轮 | 真实 backlog/锁等待 |
| no automatic retry | 避免未知副作用 | 用户需显式重试 | 引入 operation ledger/idempotency |

## 27. 测试策略为什么必须分层

### 27.1 纯测试能证明什么

domain/application/adapters 的 table、race、fuzz、failure injection能证明边界、状态机、事务调用和低披露分类；它们不能证明 MySQL 8.4 真实隔离级别、Nginx header、浏览器 Cookie jar或 TLS。

### 27.2 真实 MySQL 能证明什么

Migration、constraint、collation、lock、affected rows、grants和 driver行为；它不能证明浏览器不把 token放 storage，或 UI诚实呈现 indeterminate。

### 27.3 Compose/HTTP/browser/TLS 各自证明什么

- Compose：image、user、mount、network、service依赖、真实 MySQL/Redis/Nginx装配；
- HTTP：wire grammar、status/header/Cookie/重放与故障；
- browser：导航/刷新/交互、故障状态、focus/语义与响应式呈现；Cookie wire 属性仍由 HTTP gate 证明，浏览器脚本不能读取 HttpOnly bearer 来制造证据；
- TLS：Secure `__Host-` 与 MySQL `verify_identity` 的部署事实。

当前证据分为互不替代的几层：Identity 普通/race/shuffle×10 与九个 fuzz target 均通过；HEAD `4149576` 的独立 MySQL 8.4.11 gate 在 19 秒内验证 schema、真实 migrator、Repository/runtime grants，终态 `14:0`、Identity `0:0:0`；浏览器在 1719 × 862、390 × 844 与 1280 × 720 三种状态/视口完成 login/current/logout、reload、MySQL outage/recovery、键盘顺序、focus、aria/live status 与 reduced-motion 核查。当前全仓 Go 普通 23.2 秒、race 25.8 秒、vet 与 fmt-check 也已通过，但冻结 tip 后仍需重跑最终组合门禁。

HTTP 证据本身又经历了四个可区分的状态。start HEAD `8a5e0ce`、code baseline `5af29e2` 的工作树核心 gate 在 project `growthosl24d2103fd496568ceac960d315` 运行 302 秒 exit 0，证明 201→200→replacement→204→replay、development Cookie 必需属性、CSRF/Origin/Fetch、同形 401、五会话与 MySQL 503/recovery；但 `8a5e0ce` 提交自身尚未包含该 Session gate，所以它不是冻结 provenance。

首个已提交增强 gate `903fd9f` 在 project `growthosl24c1bf7ce29e5efa417fae6932` 让 Session 之前的 Compose/Lottery/cache/performance/grant/maintenance 全部通过，却因 macOS BSD awk 把循环变量 `index` 视为内建名而 exit 2，尚未进入 Session 断言；它完成清理，但没有可信总耗时。在此前已统一 security-header owner 的基础上，`51b52e0` 随后修复 awk、invalid-Host JSON 421 和 exact Cookie 断言；project `growthosl240da11b08420700da0d07428f` 的复跑又在第二次相同 backend image build 获取 Docker Hub OAuth token 时遇到 `EOF`，仍未进入 Session，外部 residue 为零。失败层不同，结论也必须不同：前者是脚本可移植性，后者是构建外部依赖，都不是 Session 正负行为结果。

`9fc4e06` 因而没有只“再试一次网络”，而是消除四次相同 backend build：API、migrate、provision、maintenance 合并进一次 Compose Bake，共享 Go builder 实际只执行一次，其余 target 命中 cache。project `growthosl24f6a5acf4d242695ad3e2df19` 最终 exit 0；没有可信总耗时，不能补算。它完整通过 Lottery/cache/performance 等既有门禁，并在 Session wire 上证明 raw login/source 429、TE/Trailer、2049-byte 普通 body、逐类错误零 Set-Cookie、失效态 exact clear-Cookie、每个状态 exact 单值安全 header，以及非法 Host 的 correlated JSON 421。第一性原则不是“失败后增加 retry”，而是先区分产品、验收程序、构建拓扑和外部 registry 的失败域，再移除可避免的重复工作。

三条清理证据也不可混写：早先 browser E2E 删除 2 Session、2 throttle、1 account；历史核心 HTTP 工作树删除 `10:3:1`；HEAD `9fc4e06` 增强轮在终态 `disabled:2:10:31` 后删除 `10:31:1`。增强轮随后确认三表与 disposable Docker/temp residue 全零，长期 `growthos` 不变且健康。私人 password file 先覆写再 unlink，父目录删除；这里仍只声称应用层清理完成，不把 SSD/COW 上的覆写宣传成物理不可恢复。

增强门禁现在已经关闭 raw 429、TE/Trailer、2049-byte body、clear-Cookie 与 header owner 的旧证据缺口。仍未关闭的是 raw Content-Length absent/zero/mismatch 的全部 proxy 变体和真实 issue/revoke COMMIT outcome-unknown 网络断提交；浏览器 storage/console、更广设备/辅助技术、staging/production TLS、可信代理与最终冻结门禁也仍 `PENDING`。详见 [QA 证据账本](../../qa/lessons/lesson-32.md)。

## 28. 真实架构师如何变更这一系统

### 28.1 新增 MFA

先定义 threat和恢复，再增加 challenge transaction、attempt限制、factor enrollment/revocation；不要在 Session DTO加一个 caller-controlled `mfa=true`。

### 28.2 接 OIDC

冻结 issuer/audience/redirect/state/nonce、JWKS缓存与 outage语义；建立 external subject到本地 Account/Principal的显式绑定；不自动创建或继承 IdP Role。

### 28.3 增加 logout-all

使用 account epoch 的可信写路径，明确谁能调用、成功/commit-unknown语义、当前 Cookie清理、audit和 UI提示；不要逐行循环 revoke。

### 28.4 增加 Session 管理页

设计最小设备/时间投影，禁止返回 raw token/digest；明确 revoke other和当前 Session的CSRF/授权；解决对象存在性披露。

### 28.5 引入缓存

先测 resolve热点，再设计 cache key、TTL、epoch/revoke invalidation、DB outage和stale上限；任何 miss/error都不能变成 allow。

### 28.6 自动 maintenance

先让 one-shot fixture、backlog/lock指标和失败告警稳定，再用外部 scheduler触发同一固定命令；调度层不能获得 cutoff/batch或数据库额外权限。

## 29. 本节停止线为何必须保留

当前 [route table](../../../web/src/routes/appRouter.tsx)明确让 AuthLayout只包 `/login` 与 `/session`。这不是忘记保护其他页面，而是防止认证 UI 被误解为 authorization enforcement。

第 33 节必须在服务端把 trusted Principal、server-loaded Resource和exact Policy组合；第 34 节只消费服务端 capability投影改善体验；第 35 节用直接 API/URL和跨对象攻击证明客户端裁剪不可替代服务端 deny。顺序交换会让前端 Role或Session成为事实源。

## 30. 尚未提及但必须持续追问的点

1. Nginx access log是否可能记录 query、Cookie或敏感 headers？
2. APM/trace自动 capture request body/header是否关闭？
3. 浏览器密码管理器与 DevTools截图的运维规范是什么？
4. MySQL backup、binlog与只读副本如何保护 password envelope/digest？
5. 服务器时钟跳变对 expiry、lease和retention的影响如何监控？
6. provision caller原始 password file如何由人安全销毁？
7. account disabled/epoch变化的管理入口与双人审批何时设计？
8. CSRF previous key窗口结束后如何证明旧 key已从所有实例移除？
9. throttle HMAC key丢失/轮换时旧行如何收敛而不解封攻击？
10. maintenance持续满批是否表示攻击、时钟错误或retention设计失真？
11. 多可用区后MySQL commit uncertainty和延迟预算是否仍合适？
12. 第 33 节授权审计如何关联 request ID而不复制 credential？

这些问题不要求在本节全部实现，但必须记录触发条件，避免系统以“功能已经能登录”为终点。

## 31. 候选人可复述的设计模板

> 我先把认证目标限定为从不可伪造的服务器证据恢复 human Principal，而不是做登录页面。然后盘点 password、bearer、Session state、throttle和Principal mapping的资产与authority。人类密码用严格、自描述且有资源硬上限的Argon2id，unknown用户走dummy work，外围用持久双维reservation和进程semaphore限制放大。Session使用32-byte随机opaque Cookie，MySQL只存SHA-256 digest，并以status、epoch、revoke、idle和absolute共同判断；COMMIT未知绝不当rollback或盲重试。Cookie设HttpOnly/SameSite，unsafe请求再校验exact Origin、Fetch Metadata和绑定Session digest的HMAC CSRF。API、provisioner、migrator和maintenance使用最小数据库身份；账号创建与历史清理都是固定one-shot。浏览器严格解码201/200/204，区分匿名与技术不可判定，不持久化bearer或CSRF。最后把认证停止在trusted Principal：RBAC强制、前端capability投影和越权E2E按后续章节继续，并为每层保留不可互相替代的真实证据。

## 32. 结论

第 32 节的核心不是技术名词数量，而是每个选择都能回答：资产是什么、事实由谁拥有、输入哪里不可信、资源如何有界、失败是否可判定、最小权限能否被数据库和进程边界证明、真实证据来自哪一层，以及下一节必须在哪条停止线上继续。

当前实现面已经覆盖完整认证候选链，focused Go、独立 MySQL、HEAD `9fc4e06` 的完整 development Compose/Session wire、maintenance 与浏览器核心旅程均已有实证；raw 429、TE/Trailer、2049-byte body、exact Cookie/clear-Cookie、安全 header 单值矩阵和 invalid-Host JSON 421 已不再是待项。尚未完成的是 raw Content-Length absent/zero/mismatch 的全部 proxy 变体、真实 commit-unknown fault、浏览器 storage/console 与更广设备/辅助技术、staging/production TLS与可信代理，以及最终冻结门禁。L33服务端RBAC、L34前端capability投影和L35完整越权E2E仍未实现。设计成熟度的一部分正是既升级已有真实证据，也拒绝用它替剩余待项提前背书。
