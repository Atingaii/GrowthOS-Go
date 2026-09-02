# GrowthOS Identity 与真实会话认证基线 v1

> **状态：第 32 节实现候选；核心源码与部分真实证据已完成，最终冻结验收尚未完成。**
>
> 本文冻结第 32 节“真实会话认证”的产品语义、信任边界、安全不变量和验收口径。当前分支已经实现 Identity 三表、领域/应用/适配器、双连接池、Session HTTP、浏览器会话体验与两个 operations-only one-shot；但原始 HTTP wire 全矩阵、独立 MySQL 最终矩阵、staging/production TLS 和全仓冻结门禁仍未完成。下文分别标记“源码事实”“已执行证据”与 `PENDING`，不得由其中一层外推另一层。

- **章节：** 第 32 节“真实会话认证”
- **上游：** 第 31 节 Governance 访问控制模型与威胁边界
- **下游：** 第 33 节服务端 RBAC 强制、第 34 节前端权限投影、第 35 节越权与浏览器端到端验收
- **设计日期：** 2026-09-01
- **实现校准日期：** 2026-09-02
- **主要消费者：** GrowthOS workforce Web、后续可信服务端授权层
- **非消费者：** 当前 ephemeral Lottery route、外部消费者身份主数据、数据库基础设施账号

## 1. 为什么第 32 节现在必须出现

第 31 节已经能回答：

> 一个**已经可信确认**的 Principal，是否能在某个 exact Policy revision 下，对一个服务端确认的 Resource 执行 exact Action？

但当前系统无法证明请求中的 caller 就是这个 Principal。浏览器给出 `principal_id`、角色字符串、用户名或隐藏字段，都只是攻击者可修改的数据；成功构造 `governance.Principal` 也只证明值的形状合法。

因此，第 32 节只解决一条新的可信转换：

```text
untrusted login identifier + password
                    |
                    v
Identity credential verification
                    |
                    v
verified server-side session
                    |
                    v
trusted Governance Principal
```

它必须真实解决以下问题：

1. 密码不能明文保存，也不能使用快速通用 hash；
2. 登录成功后，浏览器不能自己声明 session identity；
3. session token 泄漏后等价于暂时获得该身份，必须高熵、可撤销、可过期且不落日志；
4. Cookie 会被浏览器自动携带，必须单独抵抗 CSRF；
5. 登录不能接受攻击者预先固定的 session ID；
6. 注销、账号禁用和凭据轮换必须真正让旧 session 失效；
7. 并发登录、并发注销、依赖失败和 COMMIT 应答未知必须有确定语义；
8. 登录成功只形成可信 Principal，不自动形成任何 Role、Scope、Permission 或 allow Decision。

## 2. 第一性原则与决定所有者

### 2.1 五个彼此独立的问题

| 问题 | 决定所有者 | 成功结果 | 不能冒充它的信号 |
| --- | --- | --- | --- |
| credential 是否匹配一个有效 workforce account | Identity | verified account | 用户名存在、前端显示头像 |
| 当前 Cookie 是否指向有效 server-side session | Identity | trusted Principal | Cookie 语法合法、token 可解码 |
| Principal 是否能执行资源动作 | Governance / 第 33 节 enforcement | allow / deny / technical error | 登录成功、账号状态 enabled |
| Activity 是否可发布或参与 | Marketing / Participation | 各自业务决定 | session 有效、RBAC allow |
| MySQL 身份能执行哪些 SQL | Infrastructure ACL | SQL capability | 产品 Principal 或 Role |

认证、会话、授权、审批和业务规则可能分别拒绝同一次操作，但证据、恢复方式和对外披露完全不同，不能压成一个 `allowed bool`。

### 2.2 Identity 是独立上下文

第 32 节新增独立 `Identity` 上下文，建议代码边界为：

```text
internal/identity/domain
internal/identity/application
internal/identity/adapter/passwordhash
internal/identity/adapter/mysqlrepo
internal/identity/adapter/httpapi
```

Identity 拥有本地 workforce authentication provider 的以下事实：

- 本地 workforce account 的登录标识、状态与 credential envelope；
- account authentication epoch；
- server-side session 的签发、到期与撤销状态；
- credential 验证和 session 认证决定。

Identity 明确不拥有：

- Governance Role、Permission、Scope、Policy 或 RoleBinding；
- Marketing、Lottery 等业务 Resource facts；
- 消费者用户、会员或企业组织目录的完整生命周期；
- MySQL/Redis 基础设施账号的产品身份含义。

`internal/governance/domain` 继续拥有 canonical `Principal` 类型。Identity 只有在 credential 与 session 均已验证后，才把受控 account 映射为 `PrincipalKindHuman + PrincipalID`。客户端永远不能直接提供这个映射结果。

### 2.3 可替换的本地 workforce identity provider

GrowthOS 长期可以接入企业 IAM、OIDC 或 SSO，但当前仓库没有可依赖的外部 IdP。为了形成可运行、可学习且可攻击验证的完整会话链，第 32 节采用一个**本地、可替换、只服务 workforce 的 identity provider**。

这不是把消费者主数据或企业组织目录复制进 GrowthOS：

- 不开放公众注册；
- 不实现消费者账号；
- 不实现组织同步；
- 不把本地 Role 当企业 IAM Role；
- account provisioning 只通过受控 bootstrap/运维入口，不通过公开 Web API；
- 未来外部 provider 只需替换 credential verification 与 account mapping adapter，不改变 session 和 Governance Principal 契约。

## 3. 本节交付与停止线

### 3.1 本节必须交付

- local workforce account 与 credential 的严格模型；
- Argon2id password envelope 与验证器；
- MySQL account/session 唯一事实源；
- 高熵 opaque session token、digest lookup 和固定到期；
- server-side session resolve 到 trusted Principal；
- session-bound、HMAC-signed CSRF token；
- Cookie、Origin 和 Fetch Metadata 防线；
- 登录、查询当前会话和注销的最小 HTTP 契约；
- 账号 epoch、显式 revoke、到期和并发会话上限；
- 双维度登录限速与有界 Argon2 并发；
- 统一低披露错误、日志白名单和故障语义；
- 单元、race、fuzz、真实 MySQL、HTTP、Compose 与最小浏览器会话证据。

### 3.2 第 33～35 节停止线

第 32 节明确不交付：

- Policy/RoleBinding repository 或动态角色管理；
- Gin 全局授权 middleware、业务 handler decorator 或 use-case enforcement；
- 服务端加载 Resource tenant/owner/ID 并调用 `Policy.Evaluate`；
- 401/403/404 的对象存在性低披露授权策略；
- durable authorization audit sink；
- 服务端 capability projection；
- React 按 capability 裁剪导航、路由、页面、字段或按钮；
- 跨角色、跨对象、跨 tenant、直接 URL/API 的完整越权 E2E；
- “已有 session，所以现有 endpoint 已受 RBAC 保护”的任何声明。

对应后续边界：

| 章节 | 唯一新增核心能力 | 第 32 节不得提前实现 |
| --- | --- | --- |
| 33 | trusted Principal + Resource facts + exact Policy 的服务端强制 | 把登录成功当 authorization allow |
| 34 | 服务端最小 capability snapshot 驱动前端体验投影 | 把 Role/Policy 全量放进 session DTO |
| 35 | anonymous/expired/cross-role/cross-object/direct API/browser 负向闭环 | 用第 32 节登录测试冒充完整越权验收 |

### 3.3 其他非目标

- 公开注册、邀请、密码修改、忘记密码和账号恢复；
- MFA、Passkey、OIDC、OAuth、SAML 或企业目录同步；
- service/agent credential、delegation 或 impersonation；
- JWT、refresh token 或客户端持有的授权声明；
- Redis session、session cache 或跨区域会话复制；
- 风险画像、设备指纹或 IP 强绑定；
- 滑动过期、remember-me 或无限续期；
- 完整安全运营平台和生产渗透测试结论。

## 4. 统一语言与核心值

### 4.1 WorkforceAccount

`WorkforceAccount` 是本地 provider 中可被认证的人类操作者记录，至少包含：

| 字段 | 含义 | 不变量 |
| --- | --- | --- |
| `AccountID` | Identity 内稳定 account identity | 非零、canonical、不可由 login name 推导 |
| `LoginName` | 人类输入的登录标识 | 唯一、规范 grammar，不进入 Principal |
| `PrincipalID` | 成功认证后映射的 Governance human Principal ID | canonical，不能由请求体覆盖 |
| `PasswordEnvelope` | 严格版本化 Argon2id envelope | 不含明文，可完整校验参数 |
| `Status` | `enabled` / `disabled` | unknown 状态失败关闭 |
| `AuthenticationEpoch` | 撤销全部旧 session 的单调非零版本 | session 创建时保存 exact epoch |
| `CreatedAt/UpdatedAt` | UTC microsecond 行元数据 | 不是授权 revision |

`LoginName` v1 采用明确、可机械核查的 canonical grammar；实现不得静默 trim、lowercase 或 Unicode normalize。具体长度和字符集由 ADR 固化并由构造器、数据库 binary collation 与 HTTP decoder共同验证。

账号表不保存 Role、tenant、owner、会员 tier 或 capability。一个 account 映射哪个 Principal，与这个 Principal 当前能做什么，是两个不同事实。

### 4.2 PasswordCredential

密码只在以下极短边界出现：

```text
HTTPS request body -> bounded decoder -> Argon2id verifier -> discard
```

禁止：

- 存储明文或可逆密文；
- 使用 MD5、SHA-1、SHA-256 等快速 hash 直接存密码；
- trim、大小写转换或 Unicode normalization；
- 写日志、trace、metrics label、panic、SQL error 或测试快照；
- 把 password 放进 URL、query、header、Cookie 或前端持久存储；
- 把基础设施 MySQL password 当 workforce credential。

HTTP 层必须在执行昂贵 hash 前限制 body、login name 和 password 的 byte size。过长输入按统一无凭据回显错误拒绝，避免用超大输入放大 CPU/内存成本。

### 4.3 VerifiedSession

一个有效 session 至少绑定：

- server-generated `SessionID`；
- `AccountID`；
- exact `PrincipalID`；
- 创建时的 `AuthenticationEpoch`；
- `IssuedAt`；
- `IdleExpiresAt`；
- `AbsoluteExpiresAt`；
- optional `RevokedAt` 与封闭 revoke reason；
- session token 的 SHA-256 digest。

有效性条件是以下条件全部成立：

```text
token digest exact match
AND account exists
AND account.status == enabled
AND session.epoch == account.authentication_epoch
AND revoked_at IS NULL
AND now < idle_expires_at
AND now < absolute_expires_at
```

任一条件缺失、未知、损坏或依赖失败都不能产生 trusted Principal。

## 5. 密码哈希基线

### 5.1 固定 Argon2id profile

第 32 节 v1 profile 固定为：

| 参数 | 固定值 |
| --- | ---: |
| algorithm | Argon2id |
| envelope version | `v=19` |
| memory | `m=19456 KiB` |
| iterations | `t=2` |
| parallelism | `p=1` |
| salt | 16 random bytes |
| output | 32 bytes |

这是第 32 节实现目标，不是已执行性能证据。实现前仍须通过 ADR-0028 记录选择依据，并在目标开发/Compose 环境实测单次和有界并发延迟、RSS 与取消边界；若实测证明该 profile 无法满足安全或资源预算，必须修改设计基线和 ADR，不能只在代码里静默降低参数。

### 5.2 严格 envelope

建议采用自描述 envelope：

```text
$argon2id$v=19$m=19456,t=2,p=1$<base64-salt>$<base64-output>
```

恢复与验证必须检查：

- 精确 algorithm 和 version；
- 参数键无缺失、重复、unknown 或溢出；
- 参数精确等于当前允许 profile，或属于 ADR 明确允许的旧 profile；
- salt 精确 16 bytes；
- output 精确 32 bytes；
- Base64 使用固定 alphabet/padding 规则；
- envelope 总长度有上限；
- malformed/unsupported envelope 不执行攻击者指定的无界参数；
- 比较使用 constant-time primitive。

### 5.3 unknown account 与枚举抗性

unknown login name、wrong password、disabled account 和 epoch 异常对外统一为：

```text
authentication_failed
```

unknown account 仍执行一次由服务端持有、参数合法的 dummy Argon2id envelope 验证，避免明显的“未查到用户就立即返回”时序差异。该措施不能证明网络层完全无侧信道，因此还必须结合统一响应结构、双维度限速和观测告警。

### 5.4 rehash 与 credential 生命周期

本节不开放密码修改，但 verifier 必须能返回内部 `rehash_required` 信号，为未来 profile 升级保留受控路径。登录响应不得暴露这个信号，也不得在同一个请求中未经事务设计就自动改写 credential。

## 6. MySQL 是唯一 account/session 真相

### 6.1 为什么不使用 Redis

现有 Redis 只拥有 Lottery Strategy 可重建读取投影，并使用独立 keyspace/ACL、fail-open 语义。认证 session 是不可凭空重建且必须可靠撤销的安全事实：

- Redis miss 不能 fail-open；
- cache eviction 不能被解释成正常注销；
- 不能复用 Lottery Redis identity 或命令 ACL；
- 同时维护 MySQL account 与 Redis session 会立即引入双写和撤权一致性问题。

因此 v1 使用 MySQL 作为 account、credential 与 session 的唯一权威来源，不增加 Redis session。

### 6.2 预期表边界

v1 精确新增三个 Identity-owned 表：

```text
identity_workforce_account
identity_session
identity_authentication_throttle
```

数据库应保护：

- account/login/principal 的唯一 identity；
- binary/canonical 字符串比较；
- 封闭 account status；
- non-zero authentication epoch；
- token digest 唯一；
- session → account 同上下文外键；
- issued/idle/absolute/revoked 时间的局部顺序；
- session/account 查询与 expiry cleanup 所需索引。
- login/source 两种 throttle dimension 的 HMAC digest、窗口、失败计数与 blocked-until；
- throttle key 唯一性、时间顺序与有界 cleanup 索引。

数据库不能独立证明：

- password envelope 的全部密码学合法性；
- `now` 是否已经越过 expiry；
- Cookie、Origin 或 CSRF 是否可信；
- account 对业务资源是否有权限；
- COMMIT 应答丢失后客户端是否收到 token。

完整对象必须由 Repository 严格恢复并重新验证，不能把坏存量返回成半合法 session。

### 6.3 独立 `growthos_identity` 运行身份

Identity runtime 使用独立 MySQL 账号 `growthos_identity`，不得借用：

- `growthos_app`；
- `growthos_migrator`；
- root；
- 集成测试 writer；
- Redis 业务 identity。

`growthos_identity` 只获得 Identity 表为完成以下用例所需的精确权限：

- 按 login name 读取 account credential/status/epoch；
- 按 token digest 读取 session 与关联 account；
- 创建 session；
- 更新 idle expiry；
- 撤销 session；
- 在签发事务中锁定 account 并撤销超额最旧 session；
- 执行有界、可审计的过期/已撤销 session 清理。
- 在昂贵 Argon2 之前读取并原子更新双维度 throttle 行。

它不得读取 Lottery、Marketing、Participation、Governance Policy 表或 `schema_migrations`，也不得执行 DDL、GRANT、任意 schema 扫描或跨上下文 join。

bootstrap/provisioning 使用另一个一次性最小权限身份；不能为省事让公开 API 获得 account credential 写权限。

## 7. Session token 设计

### 7.1 Opaque token

每次成功登录由 `crypto/rand` 生成精确 32 random bytes，使用无 padding Base64URL 编码后放入 Cookie。token：

- 不含 AccountID、PrincipalID、Role、时间或任何 PII；
- 不能由数据库 sequence、UUID time、用户名或旧 token 推导；
- 只在创建响应和后续 Cookie 请求边界短暂出现；
- 不写入 JSON、日志、trace、metrics、数据库或错误。

数据库只保存：

```text
SHA-256(raw 32-byte token)
```

SHA-256 在这里用于高熵随机 token 的 lookup digest，不用于密码哈希。Repository 以固定长度 digest exact lookup，并用数据库唯一约束防止极小概率碰撞。若插入碰撞，生成器只能有界重试；超过预算必须失败，不能覆盖既有 session。

### 7.2 Session fixation

登录请求可能已经携带攻击者预置 Cookie。成功登录时必须：

1. 不采用、不升级、不复用 incoming token；
2. 总是生成全新 32-byte token 和 SessionID；
3. 若 incoming token 对应旧 session，可以按策略撤销，但撤销失败不能让新 token 退化为旧 token；
4. 响应只写入新 Cookie；
5. 测试必须证明攻击者给出的 token 不会成为新 session digest。

### 7.3 固定过期

v1 同时使用：

- **idle timeout：15 分钟**；
- **absolute lifetime：8 小时**。

规则：

- `now >= idle_expires_at` 即失效；
- `now >= absolute_expires_at` 即失效；
- authenticated request 可以把 idle expiry 延长到 `min(now+15m, absolute_expires_at)`；
- absolute expiry 永不滑动；
- safe session introspection 是否刷新 idle 必须在 ADR 中固定，v1 建议所有成功 authenticated request 统一刷新，避免页面轮询制造不同语义；
- 失败、CSRF 拒绝、anonymous 请求和 technical error 不刷新；
- 更新失败时本次请求不产生 trusted Principal，避免把数据库不可写伪装成会话正常。

客户端 Cookie expiry 不得晚于服务端 absolute expiry。服务端始终是权威判断，浏览器删除 Cookie 不能替代服务端 revoke。

## 8. 并发会话、epoch 与撤销

### 8.1 每账户最多五个 active session

v1 允许同一 account 在多个受信浏览器使用，但最多存在 **5 个 active session**。

签发顺序固定为同一 MySQL 事务：

```text
SELECT account ... FOR UPDATE
  -> 复核 enabled + exact authentication epoch
  -> 查询该 account 未撤销且未过期 session，规范排序
  -> 若新建后超过 5，按 issued_at ASC, session_id ASC 撤销最旧 session
  -> INSERT 新 session
  -> COMMIT
```

锁 account 根行使同一 account 的并发登录串行化，保证两个同时登录不会各自看到“当前只有 4 个”并最终留下 6 个。不同 account 不共享全局业务锁。

“active”由事务捕获的一次服务端 `now` 计算；规范排序不得依赖数据库未指定行顺序。旧 session 被撤销而不是直接删除，以便短期诊断和确定返回语义。

### 8.2 当前注销与显式 revoke

`DELETE /api/v1/session` 只撤销当前 session：

- 必须先验证 session、Origin/Fetch Metadata 和 session-bound CSRF；
- revoke 使用条件更新，重复执行不能恢复 session；
- 成功、已撤销或已过期时都清除浏览器 Cookie；
- 不能接受 body 中的任意 SessionID 作为删除目标。

本节不公开“列出全部 session”“注销其他设备”或管理员 revoke API，但 Repository 与领域原因必须能表达：

- user logout；
- concurrency limit；
- account epoch changed；
- account disabled；
- security response。

### 8.3 AuthenticationEpoch

account 的 `AuthenticationEpoch` 是撤销所有旧 session 的单调版本。session 签发时保存 exact epoch；resolve 时要求 session epoch 与 account 当前 epoch 相等。

未来发生密码变更、账号恢复或安全事件时，可以原子递增 epoch，使所有旧 session 在下一次 resolve 时失效，而不需要逐行同步更新。epoch：

- 不是 Role/Policy revision；
- 不是时间戳；
- 不由客户端提供；
- 不允许回退或复用；
- 溢出必须阻断操作并告警，不能回到 1。

## 9. Session-bound HMAC CSRF

### 9.1 为什么 SameSite 不够

Cookie 会由浏览器自动附带。`SameSite=Strict` 能缩小常见 cross-site 请求，但不能替代独立 CSRF token、Origin 校验和 XSS 防护，也不能覆盖所有代理、浏览器兼容和 same-site sibling origin 风险。

第 32 节采用 session-bound、HMAC-signed token：

```text
v1.<key_id>.<csrf_nonce_base64url>.<mac_base64url>

mac = HMAC-SHA-256(
  server_csrf_key,
  length_prefixed("growthos-csrf-v1", key_id, session_token_digest, csrf_nonce)
)
```

- `csrf_nonce` 使用独立 32 random bytes；
- `key_id` 只用于选择受控 keyring 条目，不是 secret；
- token 通过 session response body 返回给同源 JavaScript，不放入 session Cookie；
- 前端在 unsafe request 的 `X-CSRF-Token` header 中回传；
- MAC 与当前 session token digest 绑定，另一个 session 的 token 不能重放；
- 服务端严格解析版本、段数、Base64URL 和长度，并 constant-time 比较 MAC；
- session revoke/expiry 后，即使 CSRF token 未过期也无效；
- HMAC key 只从受控 Secret 来源加载，不进入数据库、前端包、日志或 Git。

CSRF key 的 key ID、轮换窗口和多 key 验证方式必须由 ADR-0028 固化。缺失、空值、重复配置或不支持的 key version 应使 Identity HTTP 能力启动失败，而不是生成临时 key 导致重启后全部 token 随机失效。

### 9.2 Origin 与 Fetch Metadata

所有 Cookie-authenticated unsafe method，以及登录 POST 本身，必须同时执行：

1. `Origin` 与服务端配置的 exact public origin 匹配；
2. 若 `Sec-Fetch-Site` 存在，只允许 `same-origin`；
3. 明确拒绝 `cross-site`、`same-site` 和 malformed value；
4. 除登录外，校验 session-bound `X-CSRF-Token`；
5. 不用 Host、Referer、CORS allow-all 或自定义 header 单独替代上述组合。

Fetch Metadata 是附加信号，不是唯一防线；非浏览器客户端可能没有这些 header。是否允许无 `Sec-Fetch-Site` 但 Origin/CSRF 均合法的客户端，由 ADR 明确。缺少 Origin 的 Cookie unsafe request v1 默认失败关闭。

## 10. Cookie 基线与环境分界

### 10.1 共同属性

session Cookie 必须：

- `HttpOnly`；
- `SameSite=Strict`；
- `Path=/`；
- host-only，不设置 `Domain`；
- expiry 不晚于服务端 absolute expiry；
- 不被 JavaScript、localStorage、sessionStorage、IndexedDB 或 Zustand 持久化读取；
- 所有 session 响应使用 `Cache-Control: no-store`。

清除 Cookie 时必须使用相同 name/path/domain/secure 属性，并设置过期 `Expires` 与非正 `Max-Age`，避免只改值而保留旧 Cookie。

### 10.2 Development/test

当前本地 Compose 通过 loopback HTTP 访问，`Secure` Cookie 无法在该链路正常往返。因此 development/test 使用独立名称，例如：

```text
growthos_dev_session
```

属性为 host-only、HttpOnly、SameSite=Strict、Path=/，允许 `Secure=false`，但必须满足：

- 仅在 `development` / `test` 环境；
- 仅接受明确 loopback public origin；
- 不得监听或发布到非 loopback host 后继续使用不安全 Cookie；
- 配置和启动测试证明 production 不能选择这个名称或模式。

### 10.3 Staging/production

staging/production 固定使用：

```text
__Host-growthos_session
```

并强制：

- `Secure=true`；
- `HttpOnly=true`；
- `SameSite=Strict`；
- `Path=/`；
- 无 `Domain`；
- exact HTTPS public origin；
- 非 TLS/错误 origin 配置直接启动失败。

`Secure` 由受控环境配置决定，不能相信任意客户端 `X-Forwarded-Proto`。生产 TLS termination 与可信代理边界尚未部署完成时，只能说配置门禁存在，不能声称生产链路已验收。

## 11. 双维度登录限速与资源预算

### 11.1 为什么必须双维度

只按 login name 限速，攻击者可以轮换用户名做 password spraying；只按来源限速，攻击者可以轮换 IP/代理集中撞一个高价值账号。因此至少需要：

- **account/login-name 维度**：限制单一目标 credential 的失败尝试；
- **来源维度**：限制同一受信来源对多个 login name 的尝试。

两者都必须在昂贵 Argon2 之前形成有界 admission，但不能通过不同错误暴露 account 是否存在。

### 11.2 来源可信边界

当前 Gin 明确不信任 forwarding header。第 32 节不能直接采用客户端 `X-Forwarded-For`；来源 key 必须来自：

- 经过精确可信代理 allowlist 校验后的 client address；或
- 当前 loopback Compose edge 注入且 Go 端能证明来自该 edge 的受控值。

在可信代理边界完成前，来源维度只能被描述为本地拓扑控制，不能宣传为生产抗分布式 credential stuffing。

### 11.3 失败计数与 Argon2 前 admission reservation

“只在失败后增加计数”与“在 Argon2 前严格限制多实例并发”不能只靠一次无锁读取同时成立。若 20 个请求都读到 login failure count 为 4，它们会一起进入昂贵 hash，之后再各自记录失败，瞬时资源预算和“前 5 次”语义都已经被突破。v1 不为此新增第四张 attempt ledger，也不把数据库事务跨越 Argon2 持有；而是在同一 throttle 行内保存有界的短租约聚合状态：

- `inflight_count` 表示该 dimension 已获准、尚未完成的认证计算数；
- `admission_epoch` 在一批过期 reservation 被回收时单调递增，使旧 receipt 不能误减新一批计数；
- `inflight_expires_at` 是当前批次 reservation 的最晚截止时间，只能被新的有效 admission 向后推进；
- admission 在一个事务内按 `(dimension, subject_digest)` 规范顺序锁定 login/source 两行；阈值前只有 `failure_count + inflight_count` 低于各自阈值才同时增加两个 `inflight_count`；
- failure count 已到阈值时，active backoff 内全部拒绝；backoff 到期且 observation window 尚未重置时，每个 dimension 只允许一个 `inflight_count == 0` 的 probe reservation。若仍要求 `failure_count + inflight_count < threshold`，阈值后的 count 永远不可能下降到可 admission，系统会变成永久账户锁死；
- blocked 或 capacity-exhausted 请求不增加 failure count，也不执行 Argon2；
- credential 失败以同一个不可由 HTTP 伪造的 admission receipt 原子完成两行：减少 inflight 并增加 failure count/计算 backoff；成功减少两行 inflight，只重置 login failure 状态；取消、hash capacity 或前置依赖失败执行 neutral release，不增加 failure count；
- finalization outcome unknown 返回技术失败且不盲重试；残留 reservation 最迟在其 deadline 后由下一次 admission 回收，属于短时 fail-closed，而不是永久锁死；
- application 对一个 receipt 最多 finalize 一次；Repository 以 exact epoch、positive inflight 与两行固定锁序失败关闭，旧 epoch receipt 只能得到 stale/no-op 结果，不能扣减新 reservation。

reservation deadline 取 `min(request deadline, admitted_at + 3s)`，必须晚于 Argon semaphore 等待与一次固定 profile 验证的受控预算，且不得被客户端延长。失败 probe 增加 failure count 并从 30 秒继续指数 backoff；成功 probe 仍遵守“只重置 login、不重置 source”的规则，neutral release 不改变 failure count 或 backoff。该设计保留“只累计实际 credential failure”的产品语义，又不在 hash 期间占用数据库连接或行锁；代价是进程崩溃后会在最多一个短租约窗口内保守拒绝一部分请求。

### 11.4 Argon2 并发预算

固定 profile 每次至少消耗约 19 MiB memory，HTTP worker 数不能直接等于允许的 hash 并发数。实现必须提供：

- process-wide bounded verification semaphore；
- positive queue/wait budget；
- caller cancellation；
- admission rejected/timeout 的低基数指标；
- 不因 unknown account 绕过预算；
- 不持有数据库事务或 account row lock 执行 Argon2。

限速窗口、次数、退避和 Argon2 最大并发的精确数值必须由 ADR 与本机/Compose 实测校准；本文不虚构已测阈值。没有精确阈值实现与证据时，第 32 节不能宣称在线猜测攻击已受控。

## 12. 最小 HTTP 契约

所有路径使用同源相对 URL、JSON、`Cache-Control: no-store`、统一 error envelope 与 `X-Request-ID`。响应不得重定向到任意客户端 URL。

### 12.1 `POST /api/v1/session`

用途：验证 credential 并创建全新 session。

请求：

```json
{
  "login_name": "operator-1",
  "password": "<never logged>"
}
```

要求：

- 精确 `Content-Type: application/json`；
- 单个有界 JSON object；
- unknown/duplicate/trailing fields 或 trailing JSON 拒绝；
- 不接受 query credential、Basic Auth 或客户端 session ID；
- 先执行 body/origin/fetch-metadata/limit admission，再查 account 和 Argon2；
- 成功 `201 Created`，设置新 Cookie 并返回最小 session DTO；
- credential 错误统一 `401 authentication_failed`；
- throttled 返回统一 `429 authentication_throttled`，不披露 account；
- dependency/entropy/commit indeterminate 不得设置 Cookie。

成功 DTO 候选：

```json
{
  "data": {
    "authenticated": true,
    "principal": {
      "kind": "human",
      "id": "operator-1"
    },
    "idle_expires_at": "2026-09-01T00:15:00Z",
    "absolute_expires_at": "2026-09-01T08:00:00Z",
    "csrf_token": "v1.<nonce>.<mac>"
  }
}
```

DTO 不含 AccountID、password envelope、session token、digest、Role、Permission、Scope、Policy revision 或内部失败原因。

### 12.2 `GET /api/v1/session`

用途：解析 Cookie、验证 server-side session 并返回当前最小认证状态。

- 有效时 `200 OK`，返回与登录成功相同的最小 Principal/expiry/CSRF 数据；
- 缺失、随机、到期、撤销、epoch mismatch 或 disabled account 统一 `401 unauthenticated`；
- 坏存量或 MySQL unavailable 是 `503 authentication_unavailable`，不能伪装成正常 anonymous；
- invalid/expired Cookie 响应同时清除 Cookie；
- 不返回 Role/capability，也不因此保护任何现有业务 endpoint。

### 12.3 `DELETE /api/v1/session`

用途：撤销当前 session。

- 必须验证 exact Origin、Fetch Metadata、有效 Cookie 和 session-bound CSRF header；
- 不接收 request body 或目标 SessionID；
- 已确认 revoke 后返回 `204 No Content` 并清 Cookie；
- session 已到期/撤销时保持低披露、清 Cookie，具体 204/401 由 ADR 固化；
- revoke COMMIT outcome unknown 时返回 `503 session_revocation_indeterminate`、清本地 Cookie，但明确不能证明服务端 token 已撤销；
- 不记录 raw Cookie/CSRF token。

### 12.4 错误语义

| 类别 | HTTP 候选 | 对外 code | 内部必须区分 |
| --- | ---: | --- | --- |
| 请求结构非法 | 400 | `invalid_request` | body/framing/field/origin parser |
| credential 不成立 | 401 | `authentication_failed` | unknown/wrong/disabled 仅受信诊断可区分 |
| session 不成立 | 401 | `unauthenticated` | absent/invalid/expired/revoked/epoch |
| CSRF/Origin 拒绝 | 403 | `request_origin_rejected` | origin/fetch/token class |
| 双维度限速 | 429 | `authentication_throttled` | 不披露命中哪个 account bucket |
| 依赖或资源预算失败 | 503 | `authentication_unavailable` | MySQL/entropy/hash budget/cancel |
| revoke 结果未知 | 503 | `session_revocation_indeterminate` | commit outcome unknown |
| 未分类内部错误 | 500 | `internal_error` | 受控 cause，不对外直出 |

本表是产品契约候选；最终 fault code、status 和 body shape 必须在实现 ADR/API 文档中精确冻结。

## 13. COMMIT outcome unknown 与部分失败

### 13.1 创建 session 的 COMMIT 应答未知

如果 INSERT/COMMIT 可能已成功，但客户端没有收到确认：

- 不发送 `Set-Cookie`；
- 返回低披露 503；
- 不自动用同一 token 重放；
- 数据库可能存在一个客户端从未收到 raw token 的 orphan session；
- 因 token 从未披露，该行不能被正常使用，但仍占容量并等待 expiry/cleanup；
- observer 只记录 operation、outcome_unknown、AccountID 的受控 opaque correlation，不记录 token/digest/password。

不得把“客户端没收到 Cookie”推断为事务一定未提交。

### 13.2 revoke 的 COMMIT 应答未知

revoke 结果未知比创建更危险：浏览器 Cookie 可以被清除，但攻击者持有的副本可能仍有效。因此：

- 清除当前浏览器 Cookie，减少继续使用；
- 对外返回结果未知，不显示“已安全退出”；
- 允许使用同一 session token 的条件 revoke 安全重试；
- Runbook 指导在持续未知时递增 account epoch 或执行受控安全 revoke；
- 在服务器确认前不能宣称 token 已失效。

### 13.3 其他失败优先级

推荐分类优先级：

```text
caller cancellation
  > internal operation timeout
  > rate/admission rejection
  > live dependency failure
  > credential/session negative decision
```

具体优先级需由 application contract test 固化，避免同一竞态被随机映射成 401、429 或 503。

## 14. 日志、指标与隐私

### 14.1 永不记录

- password 或 password envelope；
- raw session token、Cookie header、Set-Cookie value；
- CSRF nonce、token、MAC 或 HMAC key；
-完整 login request body；
- SQL、DSN、数据库密码或 secret 文件路径；
- RoleBinding、Policy 全量或业务敏感对象。

### 14.2 允许的白名单字段

- stable operation；
- result class：success / negative / throttled / unavailable / indeterminate；
- request/correlation ID；
- duration bucket；
- environment 与 service；
- 低基数 failure stage；
- 经评审的 opaque AccountID/SessionID 关联值，仅限受保护安全日志。

普通 metrics label 不使用 login name、PrincipalID、AccountID、SessionID、来源 IP、token digest 或 error cause，避免高基数和二次泄露。

### 14.3 登录审计边界

第 32 节可以形成 authentication lifecycle 安全事件，但不冒充第 33 节 authorization audit：

- credential accepted/rejected；
- session issued/resolved/expired/revoked；
- concurrency eviction；
- epoch mismatch；
- CSRF/origin rejection；
- throttling 与 commit outcome unknown。

这些事件的持久 sink、保留期、访问控制和跨服务 trace 仍需后续运维/审计能力；当前 `slog` 输出不能被描述为不可篡改审计账本。

## 15. 容量、清理与恢复策略

### 15.1 有界容量

| 边界 | v1 规则 |
| --- | --- |
| active sessions/account | 最多 5 |
| raw token | 精确 32 bytes |
| token digest | SHA-256，精确 32 bytes |
| CSRF nonce | 精确 32 bytes |
| Argon2 salt/output | 16 / 32 bytes |
| idle/absolute lifetime | 15m / 8h |
| HTTP body/login/password/envelope | 必须有显式 byte 上限，精确值由 ADR 固化 |
| hash concurrency | 必须有正上限，按实测校准 |
| repository rows per account read | 以 5 active + 有界 sentinel/历史查询限制 |

### 15.2 历史 session 清理

撤销和过期行不能无限增长。实现必须提供可恢复的清理协议：

- 只删除 `absolute_expires_at` 或 `revoked_at` 早于受控 retention cutoff 的行；
- 以主键/时间索引分页、小事务删除；
- 每批有硬上限和 cancellation；
- 不使用无条件 `DELETE FROM identity_session`；
- 不因 cleanup 故障放宽 resolve；
- 保留期和执行方式由 ADR/Runbook 固化并在真实 MySQL 验证。

在清理命令、索引与验收完成前，不能宣称 session 存储容量闭环已经完成。

### 15.3 密钥恢复

CSRF HMAC key 丢失会使既有 CSRF token 无法验证，但不应使 session Cookie 变成有效 CSRF 证明。恢复顺序必须：

1. 停止 unsafe mutation 或 fail closed；
2. 恢复受控 key version，或按批准流程轮换；
3. 必要时递增 account epoch / 全局安全 epoch 使旧 session 失效；
4. 验证新登录、旧 session、logout 和错误日志；
5. 不把 key 写进仓库或普通配置输出。

## 16. 威胁矩阵

| 威胁 | 典型攻击 | v1 控制 | 剩余风险 / 后续 |
| --- | --- | --- | --- |
| 凭据库泄漏 | 离线撞库 | Argon2id 固定 profile、独立随机 salt、无明文 | 密码强度/泄漏口令检查与轮换流程后续完善 |
| 在线猜测 | password spraying / brute force | account + 来源双维度限速、hash semaphore、统一错误 | 分布式来源和生产 edge 尚未验收 |
| 账号枚举 | 比较状态、body、时延 | dummy hash、统一 401、统一 response shape | 网络噪声不能证明完全恒时 |
| Hash 参数炸弹 | 恶意坏 envelope 指定超大 m/t/p | 严格 parser、固定 allowlist、长度上限 | DBA 越权仍需基础设施控制 |
| Session 猜测 | 穷举可预测 ID | 32-byte CSPRNG opaque token | entropy source 故障必须 fail closed |
| 数据库 session 泄漏 | 直接拿表冒充用户 | 只存 SHA-256 digest | 在线进程内存或 Cookie 泄漏仍可劫持 |
| Session fixation | 攻击者先设置 Cookie | 登录总是生成新 token，不升级 incoming token | 子域 Cookie 注入由 host-only/`__Host-` 缩小 |
| Session replay | 窃取 Cookie 后重复请求 | absolute/idle expiry、revoke、epoch、TLS/Secure | v1 不做设备绑定或 replay nonce |
| CSRF | 恶意站点借 victim Cookie 发 mutation | Strict Cookie、exact Origin、Fetch Metadata、session-bound HMAC token | XSS 可在同源执行动作，需独立 XSS 防线 |
| CSRF 跨 session 重放 | 用 A 的 CSRF token 配 B Cookie | MAC 绑定 session digest | HMAC key 泄漏影响全部 session |
| Login CSRF | 强迫 victim 登录攻击者账号 | login POST 也校验 Origin/Fetch Metadata | 非浏览器客户端策略需 ADR 固化 |
| 垂直越权 | 登录后直接调用 admin API | 本节明确不宣称授权；L33 必须强制 Policy | L33 前现有 endpoint 仍无 RBAC |
| Session 携带旧 Role | 撤权后仍沿用缓存 capability | session 不保存 Role/Permission/Scope | L33 Policy repository/revision 尚未实现 |
| 并发登录越过上限 | 两事务都看到 4 个 session | 锁 account 后规范撤销最旧、再插入 | DB lock timeout 需要稳定错误 |
| Logout/resolve 竞态 | revoke 同时另一个请求解析 | 条件更新、事务/时刻规则、不得复活 | 已开始业务副作用的请求由 L33 use case 处理 |
| 账号禁用后旧 session | 已有 Cookie 继续访问 | 每次 resolve 检查 status + epoch | 高流量下需要后续性能证据 |
| COMMIT 应答未知 | UI 显示错误但 DB 已提交 | 明确 indeterminate、创建不发 Cookie、revoke 可重试/epoch | orphan row 由 cleanup 收敛 |
| Token/密码日志泄漏 | access/error log 打印 header/body | 字段白名单、Nginx 只记 path、测试 secret sentinel | 第三方库/代理仍需持续审计 |
| 不安全开发 Cookie 上线 | production 仍 `Secure=false` | 环境分型、production 启动强制 `__Host-` + Secure | TLS/可信代理真实部署后再验收 |
| Redis eviction | session 被逐出或 fail-open | v1 session 完全不使用 Redis | 未来引入 Redis 必须单独 ADR |
| SQL 身份越权 | API 账号读取所有业务表 | 独立 `growthos_identity` 精确 grants | MySQL root/migrator 仍属基础设施威胁 |

## 17. 验收矩阵

以下全部是待执行门禁；设计文档完成不代表项目已通过。

### 17.1 Domain 与密码哈希

| 场景 | 必须证明 | 证据类型 |
| --- | --- | --- |
| canonical account/login/principal | 非法、空、超长、未知状态返回零值 | table/unit/fuzz |
| Argon2id round trip | 固定 m/t/p、16B salt、32B output 可验证 | unit + known envelope |
| unique salt | 相同密码产生不同 envelope | deterministic entropy seam test |
| strict envelope | duplicate/unknown/overflow/huge/malformed 参数在 hash 前拒绝 | unit + fuzz |
| constant-time output compare | 使用审核过的 constant-time primitive | code review + test boundary |
| no normalization | 密码 byte sequence 不 trim/lowercase/normalize | Unicode/boundary tests |
| dummy verification | unknown account 仍走合法固定 profile | application spy/contract test |
| rehash signal | 旧允许 profile 只产生内部信号 | unit |

### 17.2 Session application

| 场景 | 必须证明 |
| --- | --- |
| 登录成功 | account enabled、密码正确才创建 exact epoch session |
| 错误 credential | unknown/wrong/disabled 对外同类且不创建 session |
| fixation | incoming Cookie/token 永不成为新 session token |
| entropy failure | zero session、zero Cookie、稳定 unavailable |
| digest collision | 有界重试，不覆盖旧 session |
| idle boundary | `now == idle_expires_at` 失效 |
| absolute boundary | `now == absolute_expires_at` 失效且不滑动 |
| revoke | 已撤销 session 永不再产生 Principal |
| epoch | account epoch 改变后全部旧 session 失效 |
| disabled | 既有 session 下一次 resolve 失效 |
| concurrency cap | 64 个并发登录后 active session 永远不超过 5 |
| eviction order | 同时刻按 SessionID tie-break 确定撤销最旧 |
| logout/resolve race | 无数据竞争、无 session 复活、结果符合固定优先级 |
| dependency/cancel | technical failure 不映射为 valid anonymous/Principal |
| defensive evidence | 返回 DTO 不暴露可变内部 session/secret |

### 17.3 CSRF、Cookie 与 HTTP

| 场景 | 预期 |
| --- | --- |
| valid same-origin login | 201、新 Cookie、最小 session DTO |
| malformed/duplicate JSON | 400、无 hash/DB side effect |
| unknown/wrong/disabled | 相同 401 code/body shape |
| cross-site/same-site login | Origin/Fetch Metadata 拒绝，防 login CSRF |
| valid current session | 200、Principal/expiry/CSRF，无 Role/capability |
| random/expired/revoked Cookie | 401 + 清 Cookie |
| MySQL failure | 503，不伪装 401 |
| missing/wrong/cross-session CSRF | 403、零 revoke |
| valid logout | 204、DB 已确认 revoke、清 Cookie |
| revoke outcome unknown | 503 + 清 Cookie，不宣称服务器已撤销 |
| development Cookie | host-only/HttpOnly/Strict，loopback 环境才允许无 Secure |
| production Cookie config | `__Host-`/Secure/HttpOnly/Strict/Path=/，错误配置启动失败 |
| cache headers | login/current/logout/error 均 `no-store` |
| secret sentinel | access/application/Nginx log 均不出现 password/token/CSRF |

### 17.4 真实 MySQL 与权限

必须在一次性 MySQL 8.4 环境验证：

- clean latest Migration、重复 no-change 和 dirty stop；
- account/session round trip 与严格坏存量恢复；
- token digest unique、同上下文 FK、CHECK、binary collation 和索引；
- account row lock 下并发签发最多 5 active session；
- logout、epoch、disabled、expiry 与 cleanup；
- deadlock/lock timeout/cancel/COMMIT outcome unknown 分类；
- `growthos_identity` 只执行审核 SQL；
- 对 Lottery、Marketing、Governance、Participation 与 `schema_migrations` 的真实权限拒绝；
- `growthos_app` 不可读取 Identity credential/session 表；
- bootstrap identity 不被 API runtime 复用；
- 测试 schema、账号和临时 Secret 精确清理。

### 17.5 Compose 与最小浏览器链

必须经真实 Nginx → Go → MySQL 验证：

1. 登录成功并由浏览器保存 Cookie；
2. JavaScript 无法读取 HttpOnly session token；
3. current session 返回 trusted Principal 与新的 session-bound CSRF token；
4. cross-origin、same-site sibling、缺失/错误/跨 session CSRF 被拒绝；
5. logout 后旧 Cookie 直接请求也无法恢复 session；
6. 预置 Cookie 无法固定登录后的 token；
7. idle/absolute/epoch/disabled 场景失败关闭；
8. MySQL unavailable 返回 technical failure，不显示假登录/假注销；
9. 现有 `/health`、`/ready` 和 ephemeral Lottery 行为保持原边界；
10. 页面不根据 Role/capability 裁剪导航，避免越过第 34 节。

该浏览器链只证明认证/session/CSRF，不证明第 35 节完整跨角色和对象越权 E2E。

### 17.6 全仓门禁

- `go test -count=1 ./...`；
- focused 与全仓 `go test -race`；
- envelope/token/parser fuzz；
- focused coverage，并解释覆盖范围；
- `go vet ./...` 与格式检查；
- Web unit/component test、typecheck、production build；
- `go run ./cmd/doccheck` 与 `make verify`；
- `git diff --check`；
- 第 32 节路径白名单和第 31 节已验收语义回归；
- 分支从第 31 节冻结 tip 线性创建，逐个小提交推送；
- `main` 不变，累计分支只在第 32 节冻结后 fast-forward；
- coverage、`web/dist`、临时 schema/账号/Secret/浏览器 profile 全部精确清理。

### 17.7 当前实现与证据台账（2026-09-02）

以下“已实现”只表示当前候选分支存在可追溯源码；“实际通过”只覆盖紧邻描述的执行范围：

| 范围 | 当前状态 | 可宣称的证据边界 |
| --- | --- | --- |
| Identity schema 与持久化 | 已实现 | `000012`～`000014` 依次建立 `identity_workforce_account`、`identity_session`、`identity_authentication_throttle`，当前 Migration latest 为 14；独立 disposable MySQL 最终迁移/Repository/授权全矩阵仍 `PENDING` |
| API runtime | 已实现 | `growthos_app` 与 `growthos_identity` 使用独立 credential/pool；Session route、Argon2id、双维 throttle、opaque token、CSRF/Origin/Cookie 与双 pool readiness 已装配 |
| 账号 provisioning | 已实现且两轮 disposable Compose 实际通过 | `growth-identity-provision` 使用独立 `growthos_identity_provisioner` 的 workforce-account `INSERT`-only 权限；调用方密码文件经私有快照传入，命令不 readback、不 upsert |
| 历史 maintenance | 已实现且 official disposable fixture 实际通过 | `growth-identity-maintenance run` 复用 runtime Identity credential、固定一个 clock snapshot 与 Session/throttle 各 250 行预算；实测 `2/1/3` 后第二轮精确 `0/0/0`，active Session fingerprint 不变，fixture 零残留 |
| Argon2id 资源基线 | 本地实际通过 | Apple M2 Pro 上 serial `26.638354ms/op`、parallel capacity=2 `14.179475ms/op`，单 profile 19 MiB、双 profile 最大 38 MiB；这不是 production p99、容器限额或 DoS 容量结论 |
| 浏览器核心旅程 | development loopback 实际通过 | 真实 Nginx → Go → MySQL 完成 login、reload/current、logout；MySQL 中断时保持 unknown/unavailable 而不伪装 anonymous，恢复后可重新核查同一 Principal；该证据没有直接读取浏览器 HttpOnly Cookie，也不替代 wire 属性、旧 bearer replay或 TLS 验收 |
| 原始 Session HTTP wire | `PENDING` | 仍须冻结 201→200→204→401、严格 framing/header/body、Cookie tuple、旧 bearer replay、429/503 与日志 sentinel 全矩阵 |
| staging/production | `PENDING` | 配置已强制 HTTPS、MySQL `verify_identity` 与 Secure `__Host-` Cookie；真实 TLS、可信代理 client IP 与浏览器属性尚未验收 |
| 后续权限系统 | 不属于第 32 节 | 第 33 节服务端 RBAC、第 34 节 capability 驱动 UI 裁剪、第 35 节越权 E2E 均未实现 |
| 最终冻结 | `PENDING` | 全仓 Go/race/vet/fuzz/repeat、Web、doccheck、Compose、diff、远端 ref、累计学习分支与临时材料清理仍须由最终门禁统一确认 |

真实长驻账号 provision 还暴露了一个有价值的部署缺陷：`docker compose up --wait` 会把已经快速成功并进入 `exited:0` 的 `mysql-grants` 当作等待失败。提交 `af4245e` 把 provision/maintenance wrapper 改为最长 180 秒的显式状态轮询：唯一合法 container 的 `exited:0` 才成功，`created:0`、`running:0`、`restarting:0` 继续等待，非零退出、歧义 ID、意外状态或超时全部失败关闭。该修复证明“one-shot 成功完成”与“长期 service healthy”必须采用不同验收语义。

Argon2 fuzz 还发现一条资源准入时序缺陷：当 gate 有可用 slot、同时 1ms timer 也 ready 时，原 `select` 可能随机选择 timeout 并误报 503 unavailable；这不是 credential 绕过或权限漏洞，但会把可服务请求错误降级。提交 `5af29e2` 在 context 预检查后先执行 nonblocking available fast-path，只在槽位确实已满时启动 timer，并把 `(capacity=2, occupied=1, cancel=false)` 加入 fuzz seed。修复后 passwordhash `count=10`、race 与 10 秒 fuzz（625,627 次执行）通过；其他 Identity fuzz targets 与全仓最终重跑仍属于冻结门禁。

## 18. ADR-0028 已具体化的编码参数

ADR-0028 已把实现不得默默决定的项目收敛为：

1. LoginName 精确 ASCII grammar、password/login/body/envelope byte/rune 上限；
2. bootstrap enrollment 最小长度与不在 login transaction 自动 rehash 的边界；
3. Argon2 profile、最大并发 2 和 250ms admission wait；
4. 持久双维限速的 15m window、5/30 次阈值、30s～15m 指数退避，以及同表 3s 上限的 Argon 前 reservation；
5. active + optional previous CSRF keyring、32-byte key、key-id grammar 与 8h 兼容上限；
6. 缺少 Fetch Metadata 时仍必须满足 exact Origin/CSRF；header 存在时只接受 `same-origin`；
7. current-session GET 以 60s window 刷新 idle，写失败返回 503 且不产生 Principal；
8. session 7d retention、throttle 24h retention、单批最多 500 行的 maintenance ownership；
9. 已失效 logout 返回 401 并清 Cookie，只有 confirmed revoke 返回 204；
10. session create COMMIT error 永不发送 Cookie，不因 reconciliation read 把本次响应升级为成功；
11. development 仅 loopback origin 可关闭 Secure，staging/production 强制 HTTPS + `__Host-`；
12. 本地 AccountID 到 PrincipalID 由服务端稳定映射，未来企业 IAM 另立映射切片。

这些参数已经进入第 32 节实现候选，但“进入源码”不等于所有环境均已执行。当前 Argon2 开发机 benchmark、两轮 provision、official maintenance fixture 与 development 浏览器核心旅程已有上述范围内证据；独立 MySQL 最终矩阵、原始 HTTP wire、生产代理来源、TLS Cookie 和全仓冻结仍必须真实验收。若改变 MySQL 唯一事实、opaque token、server-side revoke、session-bound CSRF、固定绝对过期、五会话上限、独立 Identity/DB identity 或第 33～35 节停止线，必须同步修改产品基线与 ADR 并解释原因。

## 19. 本节完成后能与不能宣称什么

只有全部实现和实际验收完成后，才可以宣称：

- GrowthOS 有一个可替换的本地 workforce identity provider；
- 真实 password credential 可以换取 MySQL-backed server-side session；
- browser 通过有环境边界的安全 Cookie 携带 opaque token；
- session 能按 idle/absolute/epoch/revoke/concurrency policy 失败关闭；
- unsafe session mutation 受到 Origin、Fetch Metadata 与 session-bound HMAC CSRF 保护；
- 一个有效 session 可以产生 trusted human Principal。

即使完成也不能宣称：

- 业务 API 已受 RBAC 保护；
- 登录用户拥有任何 Role、Scope 或 Permission；
- 不同人员已经看到不同导航或按钮；
- 跨对象/跨 tenant 越权已完成浏览器验收；
- 企业 SSO、MFA、消费者身份或生产 TLS 已上线；
- 全系统通过渗透测试或满足某项合规认证。

## 20. 官方资料与使用边界

以下资料用于校准设计，不构成 GrowthOS 的已验收证据：

1. [RFC 9106: Argon2 Memory-Hard Function](https://www.rfc-editor.org/rfc/rfc9106.html)：Argon2id 算法、参数和安全分析；GrowthOS 仍需按自身环境实测。
2. [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)：Argon2id、salt、work factor 与升级原则。
3. [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html)：密码认证、在线猜测限制和认证生命周期的当前指导。
4. [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)：账号枚举、认证响应和敏感账号边界。
5. [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)：opaque session ID、entropy、Cookie、固定攻击、到期与撤销。
6. [RFC 6265: HTTP State Management Mechanism](https://www.rfc-editor.org/rfc/rfc6265.html)：Cookie、Secure 与 HttpOnly 的标准语义；SameSite/前缀的当前浏览器行为还需结合最新规范与实测。
7. [MDN Set-Cookie](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie)：Cookie 属性与 `__Host-` 前缀的浏览器工程参考。
8. [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)：synchronizer/HMAC token、Origin 与 SameSite 的组合防线。
9. [W3C Fetch Metadata Request Headers](https://www.w3.org/TR/fetch-metadata/)：`Sec-Fetch-*` 请求上下文信号；当前文档属于持续演进规范，不能单独作为认证依据。
10. [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)：认证事件、敏感字段排除和日志保护。
11. [Go `crypto/rand`](https://pkg.go.dev/crypto/rand)、[`crypto/sha256`](https://pkg.go.dev/crypto/sha256)、[`crypto/hmac`](https://pkg.go.dev/crypto/hmac)：token entropy、digest 与 MAC 的标准库实现边界。
12. [Go `golang.org/x/crypto/argon2`](https://pkg.go.dev/golang.org/x/crypto/argon2)：Go Argon2id 实现 API；依赖版本必须由 `go.mod/go.sum` 锁定并回归验证。

## 21. 下一节输入

第 32 节最终输出给第 33 节的唯一可信身份输入应近似：

```text
AuthenticatedRequestContext {
  Principal: governance.Principal{kind: human, id: ...},
  SessionID: opaque server correlation only,
  AuthenticatedAt / Session expiry,
  Request correlation
}
```

它不包含角色或授权结果。第 33 节必须由受保护 service layer 加载 exact Resource facts 与 Policy，再调用第 31 节 `Policy.Evaluate`：

```text
verified session -> trusted Principal
trusted Principal + server Resource + exact Action + exact Policy
  -> allow / deny / technical error
```

这条依赖顺序是本章最重要的停止线：**认证回答“你是谁”，授权才回答“你能做什么”。**
