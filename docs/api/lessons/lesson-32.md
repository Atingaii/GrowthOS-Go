# 第 32 节 API 记录：真实 Session HTTP 与浏览器 transport

- **课程主题：** 真实会话认证
- **产品基线：** [GrowthOS Identity 与真实会话认证基线 v1](../../product/identity-session-authentication-v1.md)
- **架构决策：** [ADR-0028](../../decisions/ADR-0028-identity-session-authentication.md)
- **运行手册：** [Identity Session 运维手册](../../runbooks/identity-session-operations.md)
- **记录日期：** 2026-09-02
- **实现状态：** 已实现并完成第 32 节 development 验收；生产 TLS、可信代理来源与第 35 节跨角色越权 E2E 仍不在本节证明范围

> 本文以当前实现为准，区分“源码 contract”和“已执行证据”。产品基线与 ADR 中早期使用的
> `{login,password}` 是设计期简写；已实现且唯一允许的 HTTP 字段名是
> `{login_name,password}`。当前 Nginx → Go → MySQL development wire、一次性 MySQL 8.4.11 与核心浏览器旅程已经实际通过；这些结果仍不是 staging/production TLS、可信代理 client IP、真实 COMMIT 应答丢失或完整越权防护证明。

## 1. 本节新增的唯一公开 surface

Identity adapter 只在同一路径注册三个方法：

| 方法 | 路径 | 成功状态 | 用途 |
| --- | --- | ---: | --- |
| `POST` | `/api/v1/session` | `201 Created` | 用 login name 与 password 创建全新 server-side session |
| `GET` | `/api/v1/session` | `200 OK` | 解析并按需 touch 当前 session，返回 trusted human Principal |
| `DELETE` | `/api/v1/session` | `204 No Content` | 校验 Origin、Cookie 与 session-bound CSRF 后撤销当前 session |

没有复数 `/sessions`、`/login`、`/logout`、refresh-token endpoint、公开注册、密码重置、
账号增删改、role/capability endpoint，也没有任何公开 provision 或 maintenance API。
账号创建和历史清理只能通过运行手册中的受控 one-shot 进程完成。

实现锚点：

- [Session HTTP adapter](../../../internal/identity/adapter/httpapi/session.go)
- [Cookie policy](../../../internal/identity/adapter/sessioncookie/cookie.go)
- [Origin/source guard](../../../internal/identity/adapter/requestguard/guard.go)
- [浏览器 Session adapter](../../../web/src/api/sessionApi.ts)
- [浏览器共享 HTTP transport](../../../web/src/api/httpClient.ts)

## 2. 三个方法共同的不变量

### 2.1 路径、query 与 caller 身份

- path 必须精确为 `/api/v1/session`；浏览器 adapter 只接受以 `/` 开头、非 `//`、不含反斜线的同源绝对路径；
- 三个方法都拒绝非空 query，也拒绝只有 `?` 的 `ForceQuery`；认证秘密不得进入 URL；
- 三个方法都拒绝 `Authorization`、`X-Account-ID`、`X-Principal-ID`、`X-Role`、
  `X-Permission`、`X-Scope` 或 `X-Tenant-ID`；
- account、Principal、Role、Scope、tenant 和 Permission 不能由客户端 header/body 自报；
- handler 默认总预算 `3s`，配置不得大于 `30s`；超时、取消和依赖不确定均不能产生 trusted Principal。

### 2.2 响应安全 header

handler 在成功与失败路径都设置：

```text
Cache-Control: no-store
Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'
Referrer-Policy: no-referrer
Permissions-Policy: camera=(), geolocation=(), microphone=()
Cross-Origin-Resource-Policy: same-origin
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
```

这些 header 只约束当前 Session 响应，不等价于完整站点 CSP、CORS、XSS 或点击劫持验收。

### 2.3 公开错误 envelope

Go adapter 的稳定错误形状是：

```json
{
  "error": {
    "code": "authentication_failed",
    "message": "authentication failed",
    "request_id": "server-request-id"
  }
}
```

`code`、`message` 和 `request_id` 都必须为非空字符串。浏览器 transport 在响应同时含
`X-Request-ID` 时要求 header 与 body 的 `request_id` 精确一致，否则归类为 contract failure；
没有该 header 时使用 body 的 request ID。当前浏览器错误 decoder 要求上述必需结构，但并未拒绝额外错误字段；
不能把它描述为 error envelope 的 exact-key decoder。成功 Session envelope 才是 exact-key decoder。

## 3. `POST /api/v1/session`

### 3.1 请求 contract

最小合法请求：

```http
POST /api/v1/session HTTP/1.1
Content-Type: application/json
Origin: http://127.0.0.1:8088

{"login_name":"alice.ops","password":"caller-owned secret"}
```

body 必须是 exact object：

```json
{
  "login_name": "alice.ops",
  "password": "caller-owned secret"
}
```

服务端逐项强制：

- 恰好一个 `Content-Type`，值精确为 `application/json`；参数、大小写变体、重复 header 或其他 media type 都失败；
- 恰好一个 `Origin`，值与配置的 canonical public origin 精确一致；
- `Sec-Fetch-Site` 可以缺失；存在时必须恰好一个且精确为 `same-origin`；
- 不使用 `Host`、`Referer`、`Forwarded` 或 `X-Forwarded-*` 作为 Origin fallback；
- source 只取当前 TCP peer 的 `RemoteAddr`，并忽略所有 forwarding header；
- `Content-Length` 必须已知且为 `1..2048`，不得使用 `Transfer-Encoding` 或 trailer；实际读取 bytes 必须与长度精确一致；
- body 必须是有效 UTF-8、单一 JSON object、无 trailing value；字段必须恰好各出现一次，值必须都是 string；
- unknown、missing、duplicate field，以及 JSON 中未成对 UTF-16 surrogate 都返回 `400 invalid_request`；
- `login_name` 精确匹配 `[a-z][a-z0-9._-]{2,63}`，不 trim、case-fold 或 Unicode normalize；
- 登录 password 为 `1..128` 个 Unicode code point 且 UTF-8 最多 `512` bytes，不 trim、normalize 或截断。

浏览器调用参数采用 TypeScript camelCase：

```ts
createSession({ loginName: "alice.ops", password: "..." })
```

adapter 在发起 fetch 前验证 caller object 的 exact keys、login grammar、password code point/byte 边界，
然后只序列化 `{login_name,password}`。序列化失败或 caller 输入多/少字段时归为本地 contract failure，fetch 不会发生。

### 3.2 可选旧 Cookie 与 fixation 边界

POST 可以不带 Session Cookie。若浏览器已有当前环境的一个 canonical Session Cookie，它只作为服务端撤销旧会话的 replacement hint；
登录成功仍签发全新随机 token，绝不沿用调用者给出的值。重复、损坏、非 canonical 或另一环境模式的 Session Cookie 返回
`400 invalid_request`，而不是静默忽略。

每次成功签发使用 32 个随机 bytes；浏览器只收到 base64url 无填充 bearer，MySQL 只持久化其 SHA-256 digest。
创建事务明确 COMMIT 前不发 `Set-Cookie`。随机源失败、Cookie/CSRF 构造失败、deadline 或 COMMIT outcome unknown 都失败关闭。

### 3.3 成功响应

状态必须精确为 `201`，包含环境匹配的 `Set-Cookie` 和以下 exact JSON：

```json
{
  "data": {
    "authenticated": true,
    "principal": {
      "kind": "human",
      "id": "alice-operator"
    },
    "idle_expires_at": "2026-09-02T10:15:30.123456Z",
    "absolute_expires_at": "2026-09-02T17:45:30.123456Z",
    "csrf_token": "v1.active-key.nonce.mac"
  }
}
```

时间使用 RFC3339Nano。DTO 不包含 account ID、login name、session token/digest、credential version、
authentication epoch、Role、Scope、Permission、tenant、Policy 或 authorization Decision。

### 3.4 失败语义

- unknown login、wrong password、disabled account、stale credential/epoch 等低披露认证失败统一为
  `401 authentication_failed`，message 固定为 `authentication failed`；unknown account 仍走 dummy Argon2id；
- persistent login/source throttle 返回 `429 authentication_throttled`；
- Argon2 admission 饱和、Identity MySQL/entropy/CSRF/Cookie 依赖失败、取消、deadline 或提交不确定返回
  `503 authentication_unavailable`；
- 任何 `503` 登录都不得携带可用 Session Cookie；请求不能自动重试，因为提交是否发生可能未知；
- 未分类缺陷返回 `500 internal_error`，不向客户端回显内部 cause。

实现的持久 throttle 是 login/source 双维 15 分钟窗口：login 阈值 5、source 阈值 30，
退避从 30 秒到 15 分钟；Argon2 process-wide 默认并发 2、admission 等待默认 250ms。
这些内部参数不进入 HTTP DTO。

## 4. `GET /api/v1/session`

### 4.1 请求 contract

```http
GET /api/v1/session HTTP/1.1
Cookie: growthos_dev_session=...
```

- 不允许 query；
- 不允许 body、非零 `Content-Length`、`Transfer-Encoding` 或 trailer；
- 不要求 Origin、Fetch Metadata 或 CSRF；GET 只读取/按 60 秒 touch window 延长 idle，绝不延长 absolute expiry；
- 必须有且只有一个当前环境的 canonical Session Cookie；缺失、重复、损坏或另一环境的 cookie 都视为 unauthenticated。

### 4.2 成功与失败

- 有效 Session 返回精确 `200` 与第 3.3 节同形 envelope，并签发当前 Session 新的 CSRF token；
- missing、invalid、expired、revoked、disabled account 或 epoch mismatch 返回
  `401 unauthenticated`、message `authentication required`，同时清理浏览器 Cookie；
- Identity MySQL/CSRF/touch/deadline 等技术失败返回 `503 authentication_unavailable`，不得降级成匿名或旧 Principal；
- repository 返回不可识别的结果时返回 `500 internal_error`。

GET 成功只证明本请求得到一个 trusted human Principal；它没有做业务授权。

## 5. `DELETE /api/v1/session`

### 5.1 请求 contract

```http
DELETE /api/v1/session HTTP/1.1
Origin: http://127.0.0.1:8088
Cookie: growthos_dev_session=...
X-CSRF-Token: v1.active-key.nonce.mac
```

- 不允许 query、body、payload framing 或 trailer；
- 必须满足与 POST 相同的 exact Origin 规则；`Sec-Fetch-Site` 存在时只能为单个 `same-origin`；
- 必须有且只有一个当前环境 canonical Cookie；
- 必须恰好有一个非空 `X-CSRF-Token`；token 必须由接受的 key 签名并绑定当前 Session digest；
- missing、malformed、wrong-session、wrong-key、bad-MAC 或过期 previous-key token 对外统一为
  `403 request_origin_rejected`，不泄露具体原因。

CSRF 失败时服务器不撤销 Session；Origin/CSRF 的 `403` 路径也不会顺便清 Cookie。

### 5.2 成功、失效与不确定撤销

- 只有数据库确认当前 Session 已撤销才返回 `204`；响应没有 JSON、没有任何 body，并发送清 Cookie；
- missing/invalid/inactive Session 返回 `401 unauthenticated` 并清 Cookie；
- revoke COMMIT outcome unknown 返回 `503 session_revocation_indeterminate`，message 为
  `session revocation could not be confirmed`，同时清 Cookie，且调用方不能盲目自动重试；
- 其他认证依赖不可用返回 `503 authentication_unavailable`；只有 unauthenticated 与明确
  revocation-indeterminate 分支保证先清 Cookie，普通 unavailable 不能被文档误写为一定清除；
- 未分类缺陷返回 `500 internal_error`。

浏览器 adapter 对成功 logout 额外要求状态精确为 `204`、无 `Content-Type`、无
`Transfer-Encoding`、`Content-Length` 缺失或精确为 `0`，并且实际读取为零 bytes；否则归 contract failure。

## 6. Cookie 与 CSRF wire boundary

### 6.1 Cookie

| 环境 | Public origin | 名称 | `Secure` | 共同属性 |
| --- | --- | --- | --- | --- |
| development/test 本地模式 | exact HTTP loopback origin | `growthos_dev_session` | false | host-only、`Path=/`、`HttpOnly`、`SameSite=Strict` |
| staging/production | exact HTTPS origin | `__Host-growthos_session` | true | host-only、`Path=/`、`HttpOnly`、`SameSite=Strict` |

raw token 固定 32 bytes，Cookie value 是 43 字符 canonical base64url 无填充值；全零值被拒绝。
`Expires`/`Max-Age` 不越过 8 小时 absolute expiry。清 Cookie 使用相同 name/path/security/SameSite/HttpOnly tuple，
空值、过去 `Expires` 和 `Max-Age=-1`。另一环境的 cookie name 不被兼容读取。

### 6.2 CSRF

CSRF token wire 形如：

```text
v1.<key-id>.<nonce>.<mac>
```

它使用独立 32-byte HMAC key，绑定当前 Session digest；active key 负责签发，active 加至多一个 previous key 负责验证，
previous 接受窗口不得超过 8 小时。token 由 JSON 响应交给内存中的浏览器代码，并只经
`X-CSRF-Token` 回传；不能放 URL、Cookie、日志、localStorage 或 sessionStorage。

## 7. 完整公开错误表

| HTTP | `code` | 固定 public message | 代表性条件 |
| ---: | --- | --- | --- |
| 400 | `invalid_request` | `invalid request` | query、forbidden identity header、body/framing、login grammar、Cookie shape |
| 401 | `authentication_failed` | `authentication failed` | POST unknown/wrong/disabled/stale credential |
| 401 | `unauthenticated` | `authentication required` | GET/DELETE missing、invalid 或 inactive Session |
| 403 | `request_origin_rejected` | `request origin rejected` | unsafe Origin/Fetch Metadata/CSRF 拒绝 |
| 415 | `unsupported_media_type` | `unsupported media type` | POST 非 exact `application/json` |
| 429 | `authentication_throttled` | `authentication throttled` | persistent admission policy 拒绝登录 |
| 503 | `authentication_unavailable` | `authentication temporarily unavailable` | Identity dependency、timeout、capacity 或普通 commit unknown |
| 503 | `session_revocation_indeterminate` | `session revocation could not be confirmed` | logout revoke outcome unknown |
| 500 | `internal_error` | `internal server error` | 未分类实现缺陷/不可信返回值 |

公开响应不区分账号是否存在、具体 session 失效原因、CSRF 子原因、SQL/Argon2/entropy 错误、host、账号或 Secret path。
结构化失败日志只记录稳定的 `operation`、`result_class` 和 `request_id`。

## 8. 浏览器 transport contract

公开 API：

```ts
type SessionSnapshot = {
  authenticated: true;
  principal: { kind: "human"; id: string };
  idleExpiresAt: string;
  absoluteExpiresAt: string;
  csrfToken: string;
};

createSession({ loginName, password }, options?)
readCurrentSession(options?)
revokeCurrentSession(csrfToken, options?)
```

每个请求固定：

```text
credentials = same-origin
mode        = same-origin
cache       = no-store
redirect    = error
retry       = none
```

Session adapter 默认 timeout `5000ms`，只接受 `100..5000ms` 的 safe integer；caller 可以传
`AbortSignal`。前端不会自行注入 `Origin` 或 Cookie：浏览器按同源规则管理这两个 forbidden/credential header；
DELETE 只显式加入 `X-CSRF-Token`，没有 body 或 payload framing header。

成功 decoder 要求：

- outer object 只有 `data`；data 只有五个规定字段；Principal 只有 `kind/id`；
- `authenticated` 精确为 true，kind 精确为 `human`；
- Principal ID 满足第 31 节 canonical grammar；
- 两个时间是日历上真实、带时区、允许 1～9 位小数秒的 RFC3339；
- CSRF 是非空 visible ASCII 且最多 512 bytes；
- 状态分别精确为 `201`、`200`、`204`，其他即使是 2xx 也归 contract failure。

`ApiClientError.kind` 分类：

| kind | 语义 |
| --- | --- |
| `http` | 后端返回非 2xx 且公开 JSON error envelope 可验证 |
| `gateway` | 外层或非本仓库代理返回非 JSON 的 `502/503/504`；当前仓库 Nginx 自身的 `502/504` 使用关联 request ID 的 JSON error contract |
| `contract` | 本地输入/配置非法，或成功/错误响应不符合上述 contract |
| `timeout` | adapter 自己的有界 timer 触发取消 |
| `cancelled` | caller signal 或 request controller 已取消 |
| `network` | fetch/连接/redirect 等未分类 transport failure |

transport 不自动重试。调用者尤其不能对 POST 或 revoke-indeterminate DELETE 做透明重放。

## 9. 停止线：认证不是权限系统

第 32 节只完成：

```text
untrusted login_name + password
                -> verified server-side session
                -> trusted human Principal
```

本节明确没有完成：

- 第 33 节的服务端 RBAC enforcement、可信 Resource fact 加载、401/403/404 低披露策略；
- 第 34 节的 capability snapshot 与导航、路由、页面、字段、按钮裁剪；
- 第 35 节的 direct API、跨角色、跨对象、跨 tenant、浏览器越权 E2E；
- Role/Policy/Binding 管理 UI 或公开 API；
- 生产代理 client-IP allowlist、生产 TLS/`__Host-` 浏览器证明；
- MFA、SSO、OIDC、Passkey、消费者身份或 service/agent credential。

因此当前业务 endpoint 仍不能仅因 Session route 存在而宣称已受保护；前端隐藏也不能代替服务端授权。
当前 Nginx → API 拓扑中 source 是连接 peer，通常会聚合为代理容器地址；在显式代理信任模型落地前，
不能宣称已经得到生产级逐客户端 IP throttle。

## 10. 证据台账

`ACTUAL-PASS` 表示列出的命令或真实入口已经执行；它只证明该行范围。当前最终 development Compose 运行来自冻结前代码 tip `9fc4e06`，project 为 `growthosl24f6a5acf4d242695ad3e2df19`，退出码为 0；运行没有保留可信总时长，因此本文不补造耗时。

| 证据 | 当前状态 | 冻结前必须保留的材料 |
| --- | --- | --- |
| Go Session handler、Cookie、Origin/CSRF、application/repository contract | `ACTUAL-PASS` | focused、repeat、shuffle、race、fuzz、全仓普通/race、vet 与 diff-check 已通过 |
| TypeScript Session adapter 与共享 transport contract | `ACTUAL-PASS` | 23 个 Vitest 文件、250 个测试、typecheck、format 与 production build 已通过 |
| clean MySQL 8.4.11 Migration 12～14 与 exact grants | `ACTUAL-PASS`（`4149576` 历史基线） | 一次性 MySQL 8.4.11 运行 19 秒、exit 0；schema/Repository/allow-deny/mandatory-role/清理均通过，Identity 终态 `0:0:0` |
| Compose one-shot provision 与最小权限 | `ACTUAL-PASS`（`9fc4e06`） | INSERT-only provisioner 创建唯一 HTTP credential；runtime/provisioner/migrator 能力与禁止项均经真实 MySQL 验证 |
| Nginx → Go → MySQL login/current/logout/replacement/replay | `ACTUAL-PASS`（`9fc4e06`） | 201→200→replacement→204→replay、五会话上限、同形 401、MySQL unavailable/recovery、exact Set/Clear-Cookie 均通过 |
| throttle 与 wire 安全矩阵 | `ACTUAL-PASS`（`9fc4e06`） | 登录第 6 次和分布式来源第 31 次分别精确 429；TE/Trailer、2049-byte body、错误无 Cookie、canonical headers 与 invalid-Host JSON 421 均通过 |
| issue/revoke COMMIT outcome unknown | 确定性测试通过；真实 wire 故障注入 `PENDING` | 当前只证明 application/repository 的 outcome 分类与禁止盲重试，不冒充真实网络断点 |
| maintenance session/throttle fixture | `ACTUAL-PASS`（`9fc4e06`） | 首轮只删 2 个 eligible Session 和 1 个 expired throttle，第二轮收敛为 0，active Session 与业务表保持不变 |
| 浏览器 login/current/reload/unavailable/recovery/logout | `ACTUAL-PASS`（核心旅程） | 1719×862、390×844、1280×720，以及 keyboard/focus/ARIA/reduced-motion 已核查；直接 storage/console 与更广设备/辅助技术认证仍待后续 |
| raw `Content-Length` absent/0/mismatch 的全部代理变体 | `PENDING` | handler/race/fuzz 已覆盖相邻边界；本轮 raw proxy 只实际发送已记录的 framing/size 场景 |
| staging HTTPS `__Host-` Cookie | `PENDING` | 真实 TLS origin 与浏览器 Cookie 属性 |
| production proxy client address | `PENDING` | proxy allowlist、header 清洗、真实 client-IP 测试 |

本轮 HTTP fixture cleanup 为 `10:31:1`，外部复核 project containers/volumes/networks/images/builder/tempdirs 均为 0；长期 `growthos` 资源身份未改变且保持 healthy。表中仍为 `PENDING` 的项目是明确延期的生产或更强故障/设备证明，不得被本节 PASS 吞并，也不阻碍按本节 development Definition of Done 冻结。

## 11. 来源分层

### 11.1 官方技术资料

以下资料用于解释实现选择，不是 GrowthOS 的运行验收证据：

1. [RFC 6265: HTTP State Management Mechanism](https://www.rfc-editor.org/rfc/rfc6265.html)：Cookie 的基本语义。
2. [MDN Set-Cookie](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie)：Cookie 属性与 `__Host-` 工程参考。
3. [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)：低披露认证错误与账号枚举防线。
4. [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)：opaque bearer、fixation、expiry 与 revoke。
5. [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)：HMAC token、Origin、SameSite 的组合边界。
6. [W3C Fetch Metadata Request Headers](https://www.w3.org/TR/fetch-metadata/)：`Sec-Fetch-*` 的请求上下文信号。

### 11.2 牛客/面试题型线索

面试题型只用于训练表达，不能改变 wire contract：为什么不用 JWT/Redis session、如何防 fixation 与账号枚举、
SameSite 为什么不能单独替代 CSRF、COMMIT unknown 为什么不能重试、为什么 UI 隐藏不是授权。
本文没有把任何牛客帖子当技术权威，也不伪造“真实面经”原题。已核验的帖子 URL、访问日期、可复述主题与证据限制
记录在[第 32 节面试问答](../../interview/lessons/lesson-32.md)；技术结论仍以本节实现、测试以及上面的官方资料为准。
