# 第 32 节面试问答：真实 Session 认证

- **课程正文：** [第 32 节课程](../../course/part-04/lesson-32-real-session-authentication.md)
- **API 契约：** [第 32 节 API](../../api/lessons/lesson-32.md)
- **QA：** [第 32 节 QA](../../qa/lessons/lesson-32.md)
- **设计手记：** [第 32 节设计手记](../../design-thinking/lessons/lesson-32.md)
- **运行手册：** [Identity Session 运维手册](../../runbooks/identity-session-operations.md)
- **证据更新日期：** 2026-09-03
- **面经检索日期：** 2026-09-02

> 证据分层：项目行为只以当前代码、测试、产品基线和 ADR 为准；密码学、HTTP 和安全结论只用 RFC、OWASP、NIST、W3C、Go/MySQL 等一手资料校准；牛客只证明“求职者或题型文章出现过相近追问”，不代表公司官方题库、逐字原题，也不证明帖子回答正确。

## 1. 先准备两段项目自述

### 1.1 30 秒版本

> 第 31 节只有纯授权模型，没有可信 Principal 来源，所以第 32 节建立了独立 Identity 上下文：本地 workforce password 用有界 Argon2id 验证，MySQL 作为 account、Session 和双维 throttle 唯一 authority；成功登录签发 32-byte opaque HttpOnly Cookie，库内只存 SHA-256 digest，并用 status、epoch、revoke、idle/absolute expiry 恢复 human Principal。unsafe 请求再校验 exact Origin、Fetch Metadata 和 session-bound HMAC CSRF。API、provisioner、migrator、maintenance 使用最小数据库身份；浏览器严格处理 201/200/204、超时和提交不确定。本节只完成认证，不声称 RBAC、前端权限裁剪或越权 E2E 已完成。

### 1.2 90 秒版本

> 我先把目标从“做登录页”改成“只有服务器已确认的 credential 与 Session 才能构造 trusted human Principal”。密码采用 Argon2id `m=19456KiB,t=2,p=1`，strict PHC parser先限制坏存量参数；unknown用户走dummy work，外围有MySQL login/source双维reservation和进程级默认2并发闸门。每次成功登录都生成32-byte随机opaque token，浏览器通过HttpOnly、SameSite Strict Cookie持有，MySQL只存SHA-256 digest。Session同时检查account enabled、captured epoch、revoke、15分钟idle和8小时absolute；60秒touch减少写放大，每账号最多5个，合法replacement优先，再按`last_seen_at, issued_at, session_ref`确定性淘汰。POST/DELETE要求exact Origin，logout再用绑定Session digest的HMAC CSRF。数据库身份分权，maintenance固定one-shot、session/throttle各250行且不自动重试commit unknown。第 32 节真实认证链已按 development DoD 完成；实际证据已有focused Go/fuzz、独立MySQL、HEAD `9fc4e06` 的development增强 Compose/Session wire、前端、浏览器核心旅程与最终代码/文档门禁；raw 429、TE/Trailer、2049-byte body、exact Cookie/clear-Cookie、header单值矩阵与invalid-Host JSON 421均已实跑。仍PENDING的是raw Content-Length absent/zero/mismatch proxy变体、真实commit-unknown fault、production TLS/可信代理，以及浏览器storage/console与更广设备/辅助技术。

## 2. 事实账本与停止线

| 可以说 | 不能说 |
| --- | --- |
| 已按 development DoD 完成真实认证链，focused Go、MySQL 与 development 增强 HTTP wire 已实跑 | 已经完整生产验收 |
| Session成功只返回 trusted human Principal | 登录成功即获得业务权限 |
| browser adapter/UI 单元、类型、构建与核心旅程已通过 | 浏览器直接读取 HttpOnly store 或完整 CSRF/storage/device 矩阵已通过 |
| Argon数据是 Apple M2 Pro 本地 baseline | 这是生产 SLO/吞吐承诺 |
| provision、maintenance 与完整 disposable Compose 已通过并精确清理 | 任意生产规模或 production TLS 顺带通过 |
| 浏览器核心旅程、development HTTP Cookie tuple 与数据库中断恢复已通过 | 浏览器直接读取 HttpOnly store、production Cookie 或完整越权E2E已通过 |
| Auth boundary 只包 `/login`、`/session` | `/admin` 等页面已经受认证/RBAC保护 |

## 3. 精准面试问答

### Q1：这一节解决了什么，为什么不能直接说“权限系统完成了”？

**回答：** 本节只建立认证链：credential 验证后创建 server-side Session，后续从有效 Session 恢复 trusted human Principal。授权还需要服务端加载 Resource fact、RoleBinding 和 exact Policy，调用第 31 节 evaluator并强制结果；前端权限投影和越权 E2E 也未发生。

**追问：** 为什么按章节拆？认证是授权的可信输入；服务端强制是安全边界；前端裁剪只是体验；负向 E2E 才证明组合没有漏口。顺序混在一起会让浏览器 Role或菜单误成为事实源。

**项目证据：** [Identity application boundary](../../../internal/identity/application/doc.go)、[第 32 节 route stop-line](../../../web/src/routes/appRouter.tsx)。

**依据：** `项目事实` ADR-0028；`官方技术` [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) 与 [Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html) Cheat Sheets。

### Q2：认证、授权、Session 和数据库账号有什么区别？

**回答：** 认证证明请求者是谁；授权判断已认证主体能否对资源执行动作；Session是跨请求承载认证状态的bearer+服务端记录；数据库账号限制某个进程能执行哪些SQL。`growthos_identity`不是产品管理员，`PrincipalID`也不是MySQL user。

**追问：** 为什么这种区分重要？因为四者的authority、撤销、审计和故障恢复不同。把它们压成一个`isAdmin`会造成confused deputy和权限漂移。

**项目证据：** [ADR-0028](../../decisions/ADR-0028-identity-session-authentication.md)、[MySQL grant reconciliation](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)。

**依据：** `官方技术` OWASP Authentication/Authorization Cheat Sheets；`牛客线索`文末“登录后鉴权流程”记录。

### Q3：为什么新建 Identity bounded context，而不是放进 Governance？

**回答：** Governance拥有Principal/Resource/Action/Policy判定，但不应理解password、Cookie、Session或CSRF。Identity拥有credential与Session生命周期，只有验证成功才映射Governance human Principal。这样未来OIDC只替换Identity adapter，不改业务授权模型。

**追问：** platform可以做什么？提供通用配置、MySQL、HTTP和日志机制，但不能决定account status、credential version或epoch。

**项目证据：** [domain](../../../internal/identity/domain)、[application package contract](../../../internal/identity/application/doc.go)、[HTTP adapter contract](../../../internal/identity/adapter/httpapi/doc.go)。

**依据：** `项目事实` 产品基线与ADR-0028。

### Q4：为什么先做本地 workforce provider，不直接上OIDC？

**回答：** 当前需要离线、可重复的本地Compose学习链，且没有现成企业IdP、测试tenant和account mapping。OIDC会新增redirect/callback、issuer、JWKS、state/nonce和provider outage。当前把credential verification做成可替换端口，Account映射到稳定Principal；未来再加OIDC adapter。

**追问：** 有没有过度投资本地密码？有，所以范围限制为workforce、无公众注册、无密码重置/MFA，账号只通过受控one-shot provision。

**项目证据：** [Identity product baseline](../../product/identity-session-authentication-v1.md)、[verifier port](../../../internal/identity/application/ports.go)。

**依据：** `官方技术` [OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html)用于说明未来协议复杂度；当前选型是项目约束。

### Q5：为什么密码不能直接存SHA-256？

**回答：** 人类密码低熵，快速hash让离线字典攻击很便宜；salt只防预计算与相同密码相同digest，不增加足够的单次猜测成本。密码应使用专门的慢/内存硬KDF，当前选择Argon2id。

**追问：** 那为什么Session token可以SHA-256？token是服务器生成的256-bit随机值，已有高熵；SHA-256这里只做不可逆定长lookup digest，不承担拉高猜测成本。

**项目证据：** [passwordhash](../../../internal/identity/adapter/passwordhash/passwordhash.go)、[Session digest](../../../internal/identity/domain/digest.go)。

**依据：** `官方技术` [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)、[RFC 9106](https://www.rfc-editor.org/rfc/rfc9106.html)。

### Q6：Argon2id参数怎么选，为什么是19MiB、t=2、p=1？

**回答：** 参数来自当前OWASP方向和本项目资源约束，再用目标机器benchmark校准。v1固定`m=19456KiB,t=2,p=1`、16-byte salt、32-byte output；不是永久最优值。升级必须同时改产品基线、ADR、parser、benchmark和迁移策略。

**追问：** 本机数据怎样解释？Apple M2 Pro 10次serial约`26.638354ms/op`，parallel capacity=2约`14.179475ms/op`，profile 19/38MiB；只能做回归baseline，不能直接成为production SLO。

**项目证据：** [password config](../../../internal/identity/adapter/passwordhash/config.go)、提交`71553fe`的benchmark记录。

**依据：** `官方技术` OWASP Password Storage、RFC 9106、[Go x/crypto/argon2](https://pkg.go.dev/golang.org/x/crypto/argon2)。

### Q7：为什么自描述PHC envelope仍必须设hard limit？

**回答：** envelope参数来自数据库，而坏行或越权写入可能把memory/iterations设成巨大值。如果parse后直接hash，数据库内容会驱动资源DoS。实现先验证algorithm/version、字段唯一性、base64、salt/output与旧profile硬边界，再决定是否执行。

**追问：** 为什么允许旧profile？支持逐步升级；成功验证旧profile只给内部`rehash_required`，当前登录事务不偷偷改credential，避免把认证与写密码生命周期耦合。

**项目证据：** [PHC parser](../../../internal/identity/adapter/passwordhash/phc.go)、[fuzz tests](../../../internal/identity/adapter/passwordhash/fuzz_test.go)。

**依据：** `官方技术` RFC 9106和OWASP Password Storage。

### Q8：unknown user为什么也做一次Argon2？能否保证恒定时间？

**回答：** 如果unknown立即返回、known才hash，会产生明显枚举分支。实现用服务端固定dummy envelope让unknown、wrong和disabled具有相似主工作与同一401。不能承诺网络端严格constant-time，因为DB cache、调度、GC和网络仍有差异。

**追问：** 还需要什么？统一错误、双维限速、日志低披露和监控；dummy work不能单独消除所有侧信道。

**项目证据：** [login orchestration](../../../internal/identity/application/login.go)、`TestLoginUnknownWrongAndDisabledAreIndistinguishable`。

**依据：** `官方技术` OWASP Authentication Cheat Sheet。

### Q9：为什么Argon2还需要进程级semaphore？

**回答：** 单次19MiB有界，100个并发仍可能耗尽内存。默认容量2、允许1～4，等待默认250ms；满载后返回503，不无限排队。它是单实例资源闸门，不替代跨实例持久throttle。

**追问：** 为什么返回503而不是429？429表示持久登录策略阻断；semaphore饱和是技术容量不可用。区分有助于监控和恢复，也避免把资源故障计成密码失败。

**真实缺陷：** `FuzzWorkGateCapacityAndCancellation` 找到 available slot 与 1ms timer 同时 ready 时旧 `select` 会随机误报 503。`5af29e2` 先走 nonblocking slot fast-path，只在满槽时启动 timer，并加入 `(capacity=2,occupied=1,cancel=false)` seed；修复后 count=10、race、10 秒 fuzz 625,627 次执行 PASS。这是准入时序 bug，不是 credential 绕过。

**项目证据：** [password gate](../../../internal/identity/adapter/passwordhash/passwordhash.go)、[gate tests](../../../internal/identity/adapter/passwordhash/gate_test.go)。

**依据：** `项目事实` ADR-0028；`官方技术` Go context/semaphore相关并发原则。

### Q10：为什么选择server-side Session而不是JWT？

**回答：** 当前需要即时logout、account epoch撤权、disabled立即失效、每账号五会话和确定性淘汰。JWT若要这些能力仍需服务器查epoch/denylist，“无状态”优势消失，却增加claims过期、signing key rollover和客户端存储复杂度。

**追问：** JWT什么时候合适？跨服务、短期、能接受到期前不可撤销，或有成熟token service和key治理时。选型取决于撤权一致性，不是流行度。

**项目证据：** [Session domain](../../../internal/identity/domain/session.go)、[ADR alternative](../../decisions/ADR-0028-identity-session-authentication.md)。

**依据：** `官方技术` [OWASP Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)；`牛客线索`文末Session/Cookie/JWT相关记录。

### Q11：为什么不把Session放现有Redis？

**回答：** 当前Redis是Lottery可重建缓存，允许eviction、临时volume和fail-open；Session是不可凭空重建、必须可靠撤销的安全事实。miss或重启不能改变认证结果，Lottery ACL也不该看到Session。v1用MySQL作唯一authority。

**追问：** 代价是什么？current多一次DB访问、availability耦合和touch写放大；用独立pool、60秒touch window和readiness控制。真实profile出现瓶颈后才设计cache一致性。

**项目证据：** [MySQL repository](../../../internal/identity/adapter/mysqlrepo)、[ADR Redis rejection](../../decisions/ADR-0028-identity-session-authentication.md)。

**依据：** `项目事实`现有Redis故障语义；`官方技术` OWASP Session Management。

### Q12：opaque Session token怎样设计？

**回答：** 每次登录用`crypto/rand`生成32 bytes，编码为43字符无填充base64url。token不含account、role、时间或PII；数据库只存SHA-256 digest。随机源失败无fallback，COMMIT确认前不发Cookie。

**追问：** 为什么不用UUID/request ID？它们未必提供所需不可预测熵，还可能有结构。安全token需要CSPRNG，而不是唯一即可。

**项目证据：** [Session services](../../../internal/identity/application/session_output.go)、[digest type](../../../internal/identity/domain/digest.go)。

**依据：** `官方技术` [Go crypto/rand](https://pkg.go.dev/crypto/rand)、OWASP Session Management。

### Q13：如何防Session fixation？

**回答：** 成功登录永远签发全新token，不接受客户端提供Session ID，也不沿用旧Cookie。已有合法Cookie最多作为replacement hint，在事务中撤销旧Session；重复、坏或另一环境Cookie直接拒绝。

**追问：** 为什么不能静默忽略坏Cookie？多credential source会产生优先级歧义，也可能掩盖攻击或部署混淆。严格失败让wire语义唯一。

**项目证据：** [login HTTP tests](../../../internal/identity/adapter/httpapi/session_test.go)、[Session repository replacement](../../../internal/identity/adapter/mysqlrepo/session.go)。

**依据：** `官方技术` OWASP Session Management中Session ID regeneration原则。

### Q14：idle expiry、absolute expiry和touch window怎样配合？

**回答：** idle 15分钟限制无人活动；absolute 8小时限制一次认证的总生命；`now >=`任一expiry即失效。touch最多每60秒写一次，只延长idle且不能越过absolute，以减少锁/写放大。

**追问：** 为什么不每请求续期？安全收益有限，却增加行锁、binlog和并发touch/revoke竞争。conditional update保证revoke不会被续活覆盖。

**项目证据：** [Session domain](../../../internal/identity/domain/session.go)、[resolve/touch repository](../../../internal/identity/adapter/mysqlrepo/session.go)。

**依据：** `官方技术` OWASP Session Management。

### Q15：每账号最多五个Session如何保证并发下不超限？

**回答：** 签发事务`SELECT ... FOR UPDATE`锁account，重新检查status/version/epoch，再锁当前epoch的active Session；若已超过五个就fail closed。合法同account replacement hint先按`security_response`撤销；移除hint后若仍恰有五个，才按`last_seen_at, issued_at, session_ref`撤销唯一确定性最旧值，然后insert/commit。不是事务外count后insert。

**追问：** replacement hint与容量谁先？合法旧Cookie替换先处理，避免明明替换当前设备却额外踢另一设备；伪造hint不能掩盖存量异常。

**项目证据：** [Session repository tests](../../../internal/identity/adapter/mysqlrepo/session_test.go)中的eviction/replacement/overflow用例。

**依据：** `官方技术` MySQL 8.4 transaction/locking documentation；具体排序是项目决定。

### Q16：AuthenticationEpoch解决什么问题？

**回答：** Session创建时捕获account的non-zero epoch；resolve时必须exact match。安全响应、密码变更或禁用可单调递增epoch，让全部旧Session立即失效，无需逐行扫描。

**追问：** 为什么还要单Session revoke？epoch是账户级大锤；用户普通logout只应撤销当前设备。两种撤权粒度不同。

**项目证据：** [account domain](../../../internal/identity/domain/account.go)、[Session resolve](../../../internal/identity/application/resolve.go)。

**依据：** `项目事实` ADR-0028；`官方技术` OWASP Session Management的re-authentication/invalidating sessions原则。

### Q17：为什么只允许token digest collision重试三次？

**回答：** unique digest碰撞有明确安全重试方式：丢弃candidate、重新生成token，且设3次硬上限。deadlock/network/commit error可能已经产生副作用，重跑会多发Session或重复淘汰，不应统称transient retry。

**追问：** 三次耗尽意味着什么？返回技术不可用并告警；实际概率极低，连续碰撞更可能是随机源、schema或测试注入异常。

**项目证据：** `TestLoginRetriesOnlyDigestCollisionWithFreshCandidate`及[login application](../../../internal/identity/application/login.go)。

**依据：** `官方技术` Go crypto/rand；重试边界为项目决策。

### Q18：什么是COMMIT outcome unknown，为什么不能自动重试？

**回答：** `Commit()`网络错误不证明rollback，数据库可能已提交。自动重试可能重复签发、撤销或删除。签发时返回503且不交付bearer；撤销时清浏览器Cookie但返回`session_revocation_indeterminate`；maintenance第一阶段不确定时停止第二阶段。

**追问：** orphan Session怎么办？客户端未拿到raw token，无法使用；它可能短期占容量，最终absolute expiry和有界maintenance收敛。不能为了清它而猜提交结果。

**项目证据：** [commit receipt](../../../internal/identity/application/commit_receipt.go)、[repository error classification](../../../internal/identity/adapter/mysqlrepo/repository.go)。

**依据：** `官方技术` [Go database/sql](https://pkg.go.dev/database/sql) transaction语义；项目明确把unknown建模。

### Q19：登录限速为什么同时按login和source？

**回答：** 只按login容易让攻击者锁死受害者；只按source会被分布式绕过且误伤NAT。当前login阈值5、source30、窗口15分钟、backoff 30秒到15分钟。成功只reset login，不让一个正确账号清空source攻击历史。

**追问：** source为什么不直接存IP？使用专用HMAC digest降低数据库泄漏后的低熵反查和PII扩散。

**项目证据：** [throttle domain](../../../internal/identity/domain/throttle.go)、[HMAC digester](../../../internal/identity/adapter/throttledigest/digester.go)。

**依据：** `官方技术` OWASP Authentication Cheat Sheet登录throttling原则。

### Q20：如何关闭“多个实例在Argon2前同时通过”的race？

**回答：** 两条throttle row按固定`(dimension,digest)`顺序加锁，同一事务检查`failure_count+inflight_count`并同时reservation；释放事务后才hash。receipt携带admission epoch，只能finalize一次。进程崩溃遗留reservation最多3秒，回收时推进epoch使旧receipt失效。

**追问：** 为什么backoff后只放一个probe？若failure count达到阈值后永远要求低于阈值，backoff结束也没有成功机会，等价永久锁死。单probe提供受控恢复。

**项目证据：** [admission application](../../../internal/identity/application/admission.go)、[MySQL throttle tests](../../../internal/identity/adapter/mysqlrepo/throttle_test.go)。

**依据：** `项目并发设计`；`官方技术` MySQL row locking/transaction语义。

### Q21：为什么source只信RemoteAddr，不信X-Forwarded-For？

**回答：** 未配置可信代理边界时，forwarding header可由客户端伪造。当前只用直连socket peer，拒绝header覆盖。代价是在Nginx后Go可能只看到代理地址，多客户端共享source budget。

**追问：** production怎么做？定义可信proxy allowlist、hop数量、header清洗和直接访问阻断，再由唯一adapter恢复client source；不能简单取逗号列表第一项。

**项目证据：** [request guard](../../../internal/identity/adapter/requestguard/guard.go)及`TestTrustedSourceUsesOnlyConnectedSocket`。

**依据：** `官方技术` OWASP关于代理/header信任的一般原则；具体拓扑是项目事实。

### Q22：HttpOnly、Secure、SameSite分别解决什么？

**回答：** HttpOnly阻止普通JS直接读取bearer；Secure要求只经HTTPS发送；SameSite控制跨站请求携带Cookie。它们都不是“万能安全”：HttpOnly不能阻止XSS借浏览器发请求，SameSite不能替代CSRF token/Origin，Secure也不验证应用授权。

**追问：** 为什么开发和生产不同名？HTTP loopback开发用`growthos_dev_session`非Secure；staging/prod用`__Host-growthos_session`+Secure。分名防止跨环境credential混淆。

**项目证据：** [Cookie policy](../../../internal/identity/adapter/sessioncookie/cookie.go)。

**依据：** `官方技术` [RFC 6265](https://www.rfc-editor.org/rfc/rfc6265.html)、[MDN Set-Cookie](https://developer.mozilla.org/docs/Web/HTTP/Headers/Set-Cookie)、OWASP Session Management。

### Q23：有SameSite Strict，为什么仍需要CSRF？

**回答：** SameSite是浏览器策略，存在同站子域、导航/兼容性和非浏览器客户端边界。unsafe请求还要求exact Origin；logout再要求绑定Session的CSRF。多层防线让任一浏览器机制变化不直接成为单点。

**追问：** CSRF token为什么不也设HttpOnly？应用JS需要把它放入`X-CSRF-Token`；真正不能读的是bearer Cookie。token不进localStorage/URL/Cookie，只在当前组件内存。

**项目证据：** [CSRF adapter](../../../internal/identity/adapter/csrf/csrf.go)、[session API client](../../../web/src/api/sessionApi.ts)。

**依据：** `官方技术` [OWASP CSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)。

### Q24：session-bound CSRF token是什么结构，如何轮换？

**回答：** `v1.<key-id>.<nonce>.<mac>`；nonce/MAC各32 bytes，HMAC-SHA-256绑定key-id、Session digest和nonce。active key签发，active及最多一个previous验证；previous窗口不超过8小时absolute Session寿命，key ID为1～16个受限字符。

**追问：** 为什么不复用throttle HMAC key？不同协议、轮换和泄漏域必须隔离。复用会让一个用途的读取/轮换扩大到另一用途。

**项目证据：** [CSRF implementation](../../../internal/identity/adapter/csrf/csrf.go)、[config](../../../internal/platform/appconfig/identity.go)。

**依据：** `官方技术` [Go crypto/hmac](https://pkg.go.dev/crypto/hmac)、OWASP CSRF Prevention。

### Q25：Origin与Sec-Fetch-Site如何校验？为什么login也要？

**回答：** POST/DELETE要求恰好一个Origin，与canonical public origin精确相等；Sec-Fetch-Site可缺失，存在时必须恰好一个`same-origin`。不回退Host、Referer或forwarded header。login也校验以防login CSRF。

**追问：** 为什么允许Sec-Fetch-Site缺失？受控非浏览器客户端可能不发送；exact Origin仍是必须条件。header存在时严格验证，不能用缺失绕过Origin。

**项目证据：** [request guard tests](../../../internal/identity/adapter/requestguard/guard_test.go)、[HTTP Session handler](../../../internal/identity/adapter/httpapi/session.go)。

**依据：** `官方技术` [W3C Fetch Metadata](https://www.w3.org/TR/fetch-metadata/)、OWASP CSRF Prevention。

### Q26：为什么HTTP decoder这么严格，连`application/json; charset=utf-8`都拒绝？

**回答：** 认证边界只需要一个表示。duplicate JSON keys、trailing value、Content-Length/TE、Content-Type参数可能被代理/框架不同解释。固定exact JSON、known length、无TE/trailer、唯一fields、有效UTF-8/surrogate，减少request smuggling和parser differential。

**追问：** 代价是什么？非浏览器客户端必须完全按contract构造，兼容性较低。但这是窄认证API，机械一致性优先。

**项目证据：** `TestLoginStrictRequestVocabularyHasNoApplicationSideEffects`、`FuzzDecodeLoginRequestStrict`于[httpapi tests](../../../internal/identity/adapter/httpapi/session_test.go)。

**依据：** `项目威胁模型`；通用HTTP语义以标准/框架文档校准。

### Q27：Session API的精确契约是什么？

**回答：** 同一路径`/api/v1/session`：POST exact `{login_name,password}`→201；GET bodyless→200；DELETE bodyless+Origin+Cookie+CSRF→confirmed 204。成功snapshot只有authenticated、human Principal id、idle/absolute expiry和CSRF；没有Role/Scope/capability。

**追问：** 为什么不设计`/login`和`/logout`？资源式Session API让创建、读取、删除语义集中；更重要的是唯一path/method矩阵易做strict handler和测试。URI命名不是主要安全机制。

**项目证据：** [API文档](../../api/lessons/lesson-32.md)、[route registration](../../../internal/identity/adapter/httpapi/session.go)。

**依据：** `项目事实`，不是外部REST教条。

### Q28：公开错误为什么低披露但仍分401/403/429/503？

**回答：** 同一类可枚举credential状态统一，例如unknown/wrong/disabled都401；但客户端下一步不同：Origin/CSRF 403应刷新/修复来源，persistent throttle 429应等待，dependency/gate/unknown 503不能冒充密码错，revocation indeterminate还要避免“已退出”假象。

**追问：** 内部还要分吗？要，以稳定result class和request ID供告警/排障，但不能记录password、Cookie、digest、CSRF、DSN或底层cause。

**项目证据：** [HTTP error mapping](../../../internal/identity/adapter/httpapi/session.go)、[application errors](../../../internal/identity/application/errors.go)。

**依据：** `官方技术` OWASP Authentication与[Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)。

### Q29：前端transport为什么要运行时strict decoder？

**回答：** TypeScript类型编译后消失，网络响应是unknown。adapter要求exact 201/200/204、exact snapshot keys、`human`、canonical ID、真实RFC3339、可见ASCII CSRF；额外/缺失字段或状态错配归contract error，避免旧后端/代理漂移进入UI。

**追问：** 错误envelope也完全exact吗？当前只严格要求必要结构和request-id一致，不拒绝额外错误字段；不能夸大成exact-key error parser。

**项目证据：** [sessionApi](../../../web/src/api/sessionApi.ts)、[httpClient](../../../web/src/api/httpClient.ts)。

**依据：** `项目事实`；RFC3339格式由相关Internet规范定义。

### Q30：为什么前端请求same-origin、no-store、5秒取消且不自动重试？

**回答：** same-origin path/mode/credentials避免bearer跨origin；no-store减少敏感响应缓存；redirect=error防credential随跳转；Session timeout只允许100～5000ms；caller abort和timeout分别分类。不自动重试是因为POST/DELETE可能有提交不确定，GET自动重试也会掩盖outage。

**追问：** 502/503/504 HTML怎么办？非JSON gateway响应归`gateway`；规范JSON 503仍按HTTP error envelope处理，便于UI区分代理故障和应用低披露错误。

**项目证据：** [shared executeRequest](../../../web/src/api/httpClient.ts)、strict transport 96用例提交`267bdff`。

**依据：** `官方技术` Fetch API语义；具体5秒/无重试为项目决策。

### Q31：前端怎样避免把技术故障当未登录？

**回答：** mount先进入checking并GET current；只有401 `unauthenticated`进入anonymous。network/gateway/timeout/503/contract进入unavailable并允许显式重新核查。否则DB故障会显示登录表单，用户反复提交password且系统可能制造更多不确定Session。

**追问：** logout失败怎么办？confirmed/已失效清snapshot；ordinary failure保留snapshot和CSRF供显式重试；commit unknown清浏览器侧状态但显示“服务端撤销未确认”。

**项目证据：** [useSessionBoundary](../../../web/src/layouts/useSessionBoundary.ts)、[AuthLayout tests](../../../web/src/layouts/AuthLayout.test.tsx)。

**实际证据：** 真实浏览器经Nginx→Go→MySQL完成登录到`/session`、刷新恢复、logout到`/login`且刷新仍匿名；有效Session时停止MySQL，刷新保留`/session`并显示“暂时无法确认登录状态”，不泄漏Principal；恢复MySQL并点击重试后Principal恢复，最后再次logout。

**证据边界：** 浏览器运行没有窥探内部Cookie store，不能靠它单独证明Cookie wire。独立 HEAD `9fc4e06` HTTP gate 已证明development的host-only、HttpOnly、SameSite Strict、Path `/`、非Secure issue tuple和exact clear tuple；production的Secure `__Host-` Cookie仍必须由真实TLS环境证明。

**依据：** `项目事实`，与fail-closed原则一致。

### Q32：密码和CSRF在浏览器中真的“不会留存”吗？

**回答：** 精确说法是password不进React state、URL、storage或日志，发起一次请求后清空input；但它必须短暂成为JS string和JSON body，无法可靠zeroize，DevTools也可能观察。CSRF不渲染、不持久化，却在当前组件state中供DELETE使用。raw Session bearer由HttpOnly Cookie持有，普通JS不可读。

**追问：** 为什么强调措辞？安全面试重视可证明边界；“完全不在内存”是无法兑现的承诺，会掩盖XSS/APM/DevTools风险。

**项目证据：** [LoginPage](../../../web/src/pages/auth/LoginPage.tsx)、[session boundary](../../../web/src/layouts/useSessionBoundary.ts)。

**依据：** `官方技术` OWASP Session/CSRF与浏览器Cookie语义。

### Q33：为什么Identity需要独立MySQL账号和pool？

**回答：** `growthos_app`不应读password envelope或Session digest，migrator也不应被HTTP runtime复用。`growthos_identity`只获得account必要读/受控updated_at与Session/throttle DML；business反向不可读。独立pool/credential让SQL injection或代码缺陷受数据库第二道约束，并可独立轮换/撤销。

**追问：** 只配置不同用户名够吗？不够。composition还拒绝credential alias和同底层pool；grant acceptance必须实际执行allow/deny SQL。

**项目证据：** [growth-api Identity composition](../../../cmd/growth-api/identity.go)、[appconfig Identity MySQL](../../../internal/platform/appconfig/identity_mysql.go)。

**依据：** `官方技术` MySQL 8.4 account management/least privilege；`项目事实` grant allowlist。

### Q34：readiness和health为什么分开？

**回答：** health表示进程活着；readiness表示是否能安全接流量。business或Identity任一required MySQL pool失败，ready false；Identity Session请求503。Argon gate暂时满、一次错误密码或单用户throttle不是全局dependency故障，不应该摘除整个实例。

**追问：** 能否Identity down时退回mock/Header Principal？不能，这会把availability优化变成认证绕过。Redis也不是fallback authority。

**项目证据：** [growth-api Identity wiring](../../../cmd/growth-api/identity.go)、[runtime tests](../../../cmd/growth-api/identity_test.go)。

**依据：** `项目故障模型`；健康/就绪探针是通用运维模式。

### Q35：provisioner为什么只有INSERT，没有SELECT/readback？

**回答：** bootstrap只需要创建一条enabled、version/epoch=1的account。数据库账号只有workforce表INSERT；CLI没有role/status/hash/timestamp/update/delete/upsert。duplicate失败，commit unknown不重试。没有SELECT可证明此进程不能枚举credential或把“查到了”误当创建成功。

**追问：** 如何确认结果？由另一个获批准的读取身份或真实登录链核查，不给provisioner扩权。bootstrap循环不能靠公开API绕开第33节授权。

**项目证据：** [provision command grammar](../../../cmd/growth-identity-provision/command.go)、[MySQL provisioner](../../../internal/identity/adapter/mysqlprovisioner/provisioner.go)。

**依据：** `官方技术` MySQL least privilege；`项目事实`两轮disposable Compose acceptance。

### Q36：为什么enrollment password用文件，而不是flag/env？

**回答：** flag易进shell history/process list，env易进Compose materialization和诊断。wrapper要求caller-owned regular non-symlink、0600、hard-link count 1，复制进0700临时目录、只读挂载0444 snapshot，结束后覆写/unlink/rmdir；caller原文件不删。

**追问：** 覆写后能否保证物理擦除？不能，SSD/COW/filesystem缓存不保证。能证明的是应用层暴露最小、精确unlink和无容器残留。本轮专用E2E私人password file确实先覆写再unlink并删除父目录，但仍只属于应用层清理证据。

**真实缺陷延伸：** 长驻provision曾发现`docker compose up --wait`会把快速成功的`mysql-grants exited:0`误判失败。`af4245e`改为180秒有界状态轮询，只有唯一container exact `exited:0`成功，ambiguous/非零/意外状态/超时全部fail closed。面试中应说明这是运行证据推动设计修正，而不是“忽略Compose错误”。

**项目证据：** [password file reader](../../../cmd/growth-identity-provision/password_file.go)、[Compose wrapper](../../../scripts/compose-identity-provision.sh)。

**依据：** `项目安全边界`；通用Secret原则来自容器/操作系统文档。

### Q37：maintenance为什么固定250+250，而不是一个500行共享池？

**回答：** Session和throttle各250且不互借，防一个大表饿死另一表，也保持两个独立事务小而可预测。Session先清理；失败/commit unknown不启动throttle。Session已提交而throttle失败是明确部分进度，总计仍不超过500。

**追问：** Session eligibility是什么？只有`absolute_expires_at <= observed-7d`或`revoked_at <= observed-7d`；idle过期本身不够。throttle按已编码24h retention的`row_expires_at <= observed`且无inflight，不再额外减一天。

**项目证据：** [maintenance application](../../../internal/identity/application/maintenance.go)、[repository](../../../internal/identity/adapter/mysqlrepo/maintenance.go)。

**依据：** `项目retention/锁预算决策`；数据库事务原则以MySQL文档校准。

### Q38：为什么maintenance没有caller cutoff、batch、dry-run、loop和自动retry？

**回答：** 这些参数会把窄运维动作变成通用删除平台，并允许扩大锁/删除范围。一个server clock snapshot冻结边界；runtime固定一条连接、一次attempt；operation 1～30秒，在read/write内留1秒取消/清理。删除时重新检查eligibility。

**追问：** 没dry-run如何安全？小批次、受控fixture、事务内条件DELETE、bounded count和第二次收敛证据。若真实运营需要预览，应另设计不泄漏digest的只读观测面。

**项目证据：** [maintenance config](../../../internal/platform/appconfig/identity_maintenance.go)、[runtime](../../../cmd/growth-identity-maintenance/production.go)。

**实际证据：** 官方 disposable project `growthosl2465e15560c550fd33fc6901bf` 首次删除Session/throttle/total `2/1/3`，第二次精确`0/0/0`；active Session fingerprint不变、fixture residue `0:0:0`，其他功能/失败/性能/grant断言均PASS。disposable containers/volumes/networks/5 images/builder/state/secrets精确清理，保留可复用`growthos/identity-maintenance:lesson-32` image。

**依据：** `项目第一性原则与实际Compose证据`；该PASS不是生产backlog或调度SLO。

### Q39：Secret和key轮换最容易犯什么错？

**回答：** 把CSRF与throttle key复用、只删一个Compose Secret后重生成、DB密码和volume账号失配、所有服务挂载全量Secret、active切换前未部署previous、轮换后立刻销毁旧key。不同协议必须独立key和消费矩阵。

**追问：** throttle HMAC key为何更难轮换？旧行用旧key摘要，换key后无法定位同一subject；需dual-digest或等待retention的明确过渡，不能像CSRF active/previous那样想当然。

**项目证据：** [Compose secrets generator](../../../scripts/generate-compose-secrets.sh)、[operations runbook](../../runbooks/identity-session-operations.md)。

**依据：** `官方技术` [Docker Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/)；具体轮换顺序为项目决策。

### Q40：你如何证明实现是“真实推进”，而不是只有单元测试？

**回答：** 证据分层：纯Go/TS测试证明不变量；真实MySQL证明DDL、collation、lock、grants与driver；Compose证明image/user/mount/network/Secret；HTTP证明Nginx→Go→MySQL wire；browser证明Cookie jar、JS可见性、刷新/退出/交互；TLS证明Secure `__Host-`和`verify_identity`。每层不能互相代替。

**追问：** 当前诚实状态？Identity 普通/race/shuffle×10、appconfig与四个binary count=10、九个fuzz target PASS；HEAD `4149576` 的独立MySQL 8.4.11 gate 19s/exit 0，终态`14:0`、Identity`0:0:0`。HEAD `9fc4e06` 的官方 development 增强 Compose gate 在 project `growthosl24f6a5acf4d242695ad3e2df19` exit 0、无可信总耗时；除了完整Lottery/cache/performance门禁，还证明Session 201→200→replacement→204→replay、Cookie/CSRF/Origin/Fetch、同形401、五会话、MySQL 503/recovery、raw login/source 429、TE/Trailer、2049-byte body、错误零Set-Cookie、失效态exact clear-Cookie、安全header单值矩阵和invalid-Host JSON 421。清理前`disabled:2:10:31`，fixture cleanup`10:31:1`，三表及Docker/temp residue全零，长期资源不变且健康。浏览器另在1719×862、390×844、1280×720完成核心旅程与keyboard/focus/aria/reduced-motion核查，最终代码/文档门禁也已通过。仍PENDING的是raw Content-Length absent/zero/mismatch proxy变体、真实issue/revoke commit-unknown fault、browser storage/console、更广设备/辅助技术和production TLS/可信代理；L33～L35也没有因此完成。

**追问：** 验收脚本自己失败怎么办？保留完整链而不是只展示最后绿灯。`8a5e0ce`上的工作树核心轮 302s PASS，但该commit本身未含Session gate，不能当冻结provenance；首个已提交增强 gate `903fd9f` 在 project `growthosl24c1bf7ce29e5efa417fae6932` 的Session前置门禁都PASS，随后因BSD awk把`index`当内建名而exit 2，完成清理但无可信总耗时。`51b52e0`修复后，project `growthosl240da11b08420700da0d07428f` 又在第二次重复backend build取Docker Hub OAuth token时遇到`EOF`，未进入Session且外部residue为零。`9fc4e06`把四个backend target合并为一次Bake，共享builder只执行一次，才在新project完整PASS。三个失败层分别是证据provenance、脚本可移植性和外部构建依赖，不能误报成产品Session失败，也不能靠盲目retry掩盖重复工作。

**项目证据：** [第32节QA](../../qa/lessons/lesson-32.md)。

**依据：** `项目证据治理`；牛客题型只用于准备表达，不作为通过证明。

## 4. 高频追问速答

### 4.1 “为什么401不区分用户不存在和密码错误？”

防账户枚举；两者以及disabled使用相同公开code/message，unknown仍走dummy Argon。内部只用低披露result class区分。

### 4.2 “CSRF token泄漏了怎么办？”

它仍需匹配受害Session digest且请求带HttpOnly Cookie和exact Origin；泄漏仍是事故，应轮换key/结束Session并查XSS。CSRF不是bearer Cookie替代品。

### 4.3 “Cookie被盗怎么办？”

bearer被盗即可重放，所以要HTTPS、HttpOnly、SameSite、短idle/absolute、epoch/revoke、日志禁敏和终端安全；不能声称digest-only能保护浏览器侧盗窃。

### 4.4 “为何GET current不要求Origin/CSRF？”

它是读取并受no-store保护，不执行用户选择的unsafe业务动作；仍严格Cookie/body/query/header，并且touch失败不产生Principal。跨站读取还受浏览器同源与CORP等约束，但XSS另论。

### 4.5 “为何logout已失效返回401而不是204幂等？”

当前contract让204只表示服务器确认本次Session撤销；inactive用401并清Cookie，保留证据含义。调用者UI可以把401当“已结束”，但transport不能伪造成confirmed revoke。

### 4.6 “为何普通logout失败保留前端snapshot？”

因为Cookie可能仍有效，保留内存CSRF允许用户显式重试；自动清除会把“UI看起来退出”错当安全结果。commit unknown例外：服务器要求清Cookie并显示不确定警告。

### 4.7 “为什么error envelope还允许extra fields？”

当前共享decoder只要求`error.code/message/request_id`且header/body request ID一致；成功Session才exact-key。面试应说清当前实现，不要把期望冒充事实；若要收紧错误shape，应单独评估其他API兼容性。

### 4.8 “为什么maintenance复用runtime Identity账号？”

它只删除Session/throttle历史，正是runtime已有DML authority；另建账号能更窄但增加Secret/grant/轮换成本。当前通过固定command/config/one pool/one attempt约束，不使用provisioner、migrator或root。

## 5. 官方技术依据（只支撑通用结论）

| 主题 | 一手资料 | 用途 |
| --- | --- | --- |
| Argon2 | [RFC 9106](https://www.rfc-editor.org/rfc/rfc9106.html) | algorithm/version与安全背景 |
| 密码存储 | [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) | 专用KDF、参数校准与升级方向 |
| 认证器 | [NIST SP 800-63B](https://pages.nist.gov/800-63-4/sp800-63b.html) | password/authenticator生命周期原则 |
| 登录安全 | [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) | 统一错误、throttling、re-authentication |
| Session | [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) | token entropy、Cookie、fixation、expiry/revoke |
| Cookie | [RFC 6265](https://www.rfc-editor.org/rfc/rfc6265.html)、[MDN Set-Cookie](https://developer.mozilla.org/docs/Web/HTTP/Headers/Set-Cookie) | Cookie属性与浏览器语义 |
| CSRF | [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html) | token、Origin、SameSite的分层作用 |
| Fetch Metadata | [W3C Fetch Metadata](https://www.w3.org/TR/fetch-metadata/) | `Sec-Fetch-Site`语义 |
| 日志 | [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html) | 敏感数据最小化 |
| 随机/HMAC/digest | [Go crypto/rand](https://pkg.go.dev/crypto/rand)、[hmac](https://pkg.go.dev/crypto/hmac)、[sha256](https://pkg.go.dev/crypto/sha256) | 实现primitive语义 |
| 取消/事务 | [Go context](https://pkg.go.dev/context)、[database/sql](https://pkg.go.dev/database/sql) | timeout/cancel/transaction边界 |
| Secret挂载 | [Docker Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/) | 服务级Secret消费方式 |

这些资料不决定GrowthOS的五会话、15m/8h、250+250、错误code或数据库表名；那些仍是产品基线、ADR、代码和测试共同冻结的项目事实。

## 6. 牛客真实题型线索（不作为技术规范）

以下页面已于2026-09-02核验。个人记录只能说明相近话题确实出现在求职者复盘中；汇总/文章只说明常见出题方向。帖子中的答案、标题公司归属和题目措辞均不提升为项目事实。

| 类型 | 页面 | 可用于准备的追问方向 | 不可推导 |
| --- | --- | --- | --- |
| 个人面经 | [“阿里SLS一面凉经”](https://www.nowcoder.com/discuss/463105566483783680) | Cookie/Session与token、账号认证时效性 | 公司官方题库、帖子答案正确 |
| 个人面经 | [“字节电商后端日常实习一面凉经”](https://www.nowcoder.com/discuss/656587984497606656) | Session/Cookie、密码为什么/如何hash、Argon2/bcrypt延伸 | 特定团队统一标准 |
| 个人前端面经 | [“科班小前端的大厂面经”](https://www.nowcoder.com/discuss/353156245500665856) | 登录之后的鉴权流程 | 本项目应使用某种token存储 |
| 个人总结 | [“秋招总结面经〖算法〗〖前端〗〖测试〗”](https://www.nowcoder.com/discuss/353157451287568384) | XSS/CSRF与安全测试维度 | CSRF或XSS技术结论 |
| 题目汇总 | [“接口测试常考面试题”](https://www.nowcoder.com/discuss/920769103625719808) | 登录链路、非法/过期/退出、弱网与重复请求测试 | 真实公司逐字面经 |
| 个人面经 | [“小厂一面：30分钟速通，拿下一血！”](https://www.nowcoder.com/discuss/701450908437061632) | JWT/token存储取舍 | JWT一定优于Session |
| 技术/面试文章 | [“阿里前端预测面试会考这个：SameSite”](https://www.nowcoder.com/discuss/384709) | Cookie字段、SameSite追问 | 阿里官方题目或完整CSRF方案 |
| 面试风格文章 | [“用户Token到底该存哪？”](https://www.nowcoder.com/discuss/865918550328803328) | HttpOnly/localStorage/CSRF权衡 | 文章建议即权威规范 |

### 6.1 如何把线索转成自己的回答

不要背“Session安全、JWT无状态”这类口号。用项目证据回答：

1. 先说需求：即时撤权、epoch、五会话、浏览器同源；
2. 再说选择：MySQL authority + opaque HttpOnly Cookie；
3. 展开威胁：fixation、CSRF、XSS、枚举、hash DoS、commit unknown；
4. 给精确机制和数字；
5. 说出代价、替代方案与重评条件；
6. 最后主动交代已执行证据和PENDING，不伪造生产结论。

## 7. 面试中的常见失分点

- 把authentication与authorization混成“登录即有权限”；
- 说“密码加盐SHA-256就安全”或“Session digest也要Argon2”；
- 把dummy hash夸成网络绝对恒定时间；
- 只会背JWT/Session优缺点，不结合即时撤权与当前Redis语义；
- 说HttpOnly能防XSS、SameSite能完全防CSRF；
- 信任任意`X-Forwarded-For`，没有代理边界；
- 把`Commit`错误当已rollback并自动retry；
- 认为204带JSON也无所谓；
- 声称password已从JS内存物理清零；
- 把MySQL runtime账号当产品Role；
- 给provisioner `SELECT/UPDATE`只为操作方便；
- maintenance用caller指定`DELETE ... LIMIT N`却没有retention/affected-row/retry设计；
- 用单元测试PASS冒充真实MySQL、Compose、browser或TLS；
- 不敢承认PENDING，反而做无法证明的“生产级”承诺。

## 8. 最后的60秒收束

> 这套实现最重要的不是用了Argon2或HttpOnly，而是认证链每一层都有唯一authority和失败边界：password hash参数与并发有界；Session bearer随机、digest-only、可撤销且双到期；throttle在hash前原子reservation；Origin、Fetch Metadata、SameSite和session-bound CSRF分层；数据库身份分离；提交不确定不盲重试；浏览器区分匿名与不可用。证据也分层，第 32 节已按 development DoD 完成并通过focused Go/fuzz、独立MySQL、HEAD `9fc4e06` development增强 Compose/Session wire、前端、provision、maintenance、浏览器核心旅程和最终代码/文档门禁；raw 429、TE/Trailer、2049-byte body、exact Cookie/clear-Cookie、安全header单值矩阵与invalid-Host JSON 421均已实跑。仍PENDING的是raw Content-Length特定proxy变体、真实commit-unknown fault、production TLS/可信代理，以及browser storage/console与更广设备/辅助技术。最后我把trusted Principal作为本节终点，服务端RBAC、前端capability投影和完整越权E2E继续按33到35节推进。
