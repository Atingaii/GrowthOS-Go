# 第 32 节：真实会话认证

> 本节承接第 31 节的纯 Governance 访问控制模型，在独立 Identity bounded context 中建立“本地 workforce credential → MySQL 权威 Session → trusted human Principal”的真实认证链。它没有把登录成功解释成授权成功；服务端 RBAC、前端 capability 投影和完整越权 E2E 仍分别属于第 33、34、35 节。

## 1. 为什么本节必须位于授权强制之前

第 31 节已经能表达 `Principal + Resource + Action`，却故意没有回答“请求里的 Principal 从哪里来”。如果直接相信浏览器发送的用户 ID、Role 或 Scope，再精确的 Policy evaluator 也只是对伪造输入做精确计算。

因此第 32 节只解决一个前置问题：

```text
人类输入 credential
  -> 有界、低披露的密码验证
  -> MySQL 原子签发 server-side session
  -> HttpOnly opaque Cookie
  -> 后续请求解析、续期、撤销
  -> server-derived human Principal
```

这条链的末端是“已认证主体”，不是“允许执行业务动作”。业务资源、RoleBinding、Policy revision 和 authorization Decision 都没有进入 Session DTO。

## 2. 学习目标、事实层次与停止线

完成本节后，应当能够解释：

1. 为什么认证、授权、数据库账号和业务账号必须分开；
2. 为什么本地 workforce provider 可以被未来 OIDC 替换，而 Session 与 Governance 不必重写；
3. 为什么密码需要 Argon2id、自描述 envelope、资源上限和进程级并发闸门；
4. 为什么当前 Redis 不能成为 Session authority；
5. 为什么 opaque bearer 只进 HttpOnly Cookie，而数据库只存 SHA-256 lookup digest；
6. idle、absolute、touch、单会话 revoke、account epoch 和五会话上限如何组合；
7. Origin、Fetch Metadata、SameSite 和 session-bound CSRF 各自解决什么问题；
8. 双维 throttle 为什么必须在 Argon2 之前原子 reservation；
9. COMMIT outcome unknown 为什么不能当 rollback，也不能自动重试；
10. provision、runtime 和 maintenance 为什么需要不同命令与数据库权限；
11. 浏览器 adapter 为什么要严格验证成功 envelope、状态码、超时与无正文 `204`；
12. 哪些证据已经真实执行，哪些仍为 `PENDING`。

本文采用三层事实：

| 层次 | 含义 |
| --- | --- |
| `IMPLEMENTED-SURFACE` | 源码与测试文件已经存在，可逐行审查；不等于命令已运行 |
| `ACTUAL-PASS` | 有本次或已给出的精确命令、范围和结果记录 |
| `PENDING` | 最终声明所需，但尚未取得真实执行证据 |

本节的硬停止线是：没有第 33 节的服务端 Policy 强制，就不能说业务 API 已受 RBAC 保护；没有第 35 节的负向浏览器/API 验收，就不能说越权闭环完成。

## 3. 先划出四种身份

| 身份 | 作用 | 权威来源 | 不能替代 |
| --- | --- | --- | --- |
| workforce account | 被登录的人类操作者记录 | Identity MySQL | MySQL 连接账号、Role |
| Governance Principal | 已认证后供授权求值的稳定主体 | Identity 映射 + 已验证 Session | credential、浏览器自报 ID |
| MySQL runtime account | 限制进程能执行哪些 SQL | MySQL grants | 产品用户、管理员 Role |
| 浏览器 Session | 当前设备持有的 bearer | 随机 Cookie + MySQL digest/state | Policy、capability snapshot |

这四者故意使用不同标识和生命周期。`LoginName` 只用于查找 workforce account；`PrincipalID` 由服务端映射；`growthos_identity` 只是一组数据库权限；raw Session token 只证明浏览器持有一个可向服务器解析的 bearer。

## 4. Identity bounded context 的职责

实现分层如下：

```text
internal/identity/domain
  account/session/throttle 的封闭值与不变量

internal/identity/application
  login/resolve/revoke/maintenance 编排与窄端口

internal/identity/adapter/passwordhash
  Argon2id envelope、验证与并发闸门

internal/identity/adapter/mysqlrepo
  account/session/throttle/maintenance 的 MySQL authority

internal/identity/adapter/{sessioncookie,csrf,requestguard,httpapi}
  浏览器 Cookie、CSRF、来源校验与 Session HTTP

cmd/growth-api
  生产 composition root、独立 pool、readiness 与路由装配
```

[Identity application 包边界](../../../internal/identity/application/doc.go)明确禁止 HTTP、SQL、Redis、Cookie、Role、Permission、Policy 和业务 Resource 进入 application；[HTTP adapter 包边界](../../../internal/identity/adapter/httpapi/doc.go)也明确不拥有授权事实。

## 5. 以小提交演进，而不是一次大重构

当前分支按依赖方向逐步形成，主要学习检查点包括：

| 检查点 | 作用 |
| --- | --- |
| `9af7613` | Identity domain 与 workforce account 基线 |
| `8b13c59`、`71553fe` | 有界 Argon2id 与目标机器 benchmark 证据 |
| `5bccea1`、`bff2c6d` | application 编排与 verifier port |
| `8ce16d3`、`fa48ad7` | Identity schema 与 MySQL repository |
| `721216e`、`5fee340`、`1c66749` | HMAC throttle key、并发 reservation 与恢复 probe |
| `492ed29`、`97e9864`、`85fc91c` | CSRF、Cookie 与 Origin/source guard |
| `cd09819`、`7ae02ce`、`d2e4a65`、`75874b0` | 独立 Identity pool、最小 grants、真实 MySQL authority 与生命周期隔离 |
| `3ec5d0f`、`d08e46f`、`849aa04` | 严格 Session HTTP、共享 Origin grammar 与生产装配 |
| `f6de70e`～`7ccf46c`、`3867584` | INSERT-only provisioner、CLI、Compose 与真实 disposable acceptance |
| `3aa28a6`、`4f59781` | one-shot maintenance 配置与命令 |
| `267bdff` | 浏览器共享 transport 与 Session API adapter |

这里的 commit 只是学习导航；是否通过仍以 [第 32 节 QA](../../qa/lessons/lesson-32.md) 中的实际证据为准。最终冻结前还会有文档、UI 和验收提交，因此不能把上表最后一个 hash 当最终 tip。

## 6. WorkforceAccount：只保存认证所需事实

[account domain](../../../internal/identity/domain/account.go)保存：

- canonical `AccountID`、`LoginName`、`PrincipalID`；
- 严格 Argon2id `PasswordEnvelope`；
- `enabled` / `disabled` 状态；
- non-zero credential version 与 `AuthenticationEpoch`；
- canonical UTC 时间。

它不保存 Role、Scope、Permission、tenant 或 capability。登录名精确匹配 `[a-z][a-z0-9._-]{2,63}`，不 trim、不大小写折叠、不做 Unicode normalization。服务端对登录密码接受 1～128 个 Unicode code point 且 UTF-8 不超过 512 bytes；受控 enrollment 额外要求至少 12 个 code point。

## 7. 密码为什么用 Argon2id，而不是快速 hash

SHA-256 适合对 256-bit 随机 Session token 做定长 lookup digest，却不适合人类密码：密码熵低，快速 hash 会让离线猜测极便宜。[passwordhash adapter](../../../internal/identity/adapter/passwordhash/passwordhash.go)使用固定当前 profile：

| 参数 | 值 |
| --- | ---: |
| algorithm/version | Argon2id / 19 |
| memory | 19456 KiB |
| iterations | 2 |
| parallelism | 1 |
| salt/output | 16 / 32 bytes |

严格 parser 先限制 envelope 总长和参数。允许读取的旧 profile 也只能位于 memory `8192..65536 KiB`、iterations `1..4`、parallelism `1..4` 的硬边界内，防止数据库坏值驱动任意资源消耗。成功命中旧 profile 只返回内部 `rehash required`，当前登录事务不偷偷改密码。

unknown login 仍验证服务端持有的 dummy envelope；unknown、wrong password 与 disabled account 对外统一为 `401 authentication_failed`。这降低明显账户枚举差异，但不能被描述成数学上的恒定时间。

### 7.1 资源闸门

Argon2 外围还有 process-wide semaphore：默认并发 2、允许 1～4；默认最多等待 250ms、允许 1ms～1s。资源饱和归为 `503 authentication_unavailable`，不是错误密码，也不无限排队。

### 7.2 已取得的本地 benchmark

在 Apple M2 Pro 上对当前 profile 执行 10 次基线：

- serial：`26.638354ms/op`、profile 19.00 MiB、`19,926,865 B/op`、36 allocs/op；
- parallel capacity=2：`14.179475ms/op`、最大 profile 38.00 MiB、`19,925,776 B/op`、35 allocs/op；
- `/usr/bin/time -l`：max RSS `107,823,104` bytes，peak footprint `105,005,608` bytes。

这是 `71553fe` 对应的本机 baseline，不是生产 SLO、容量承诺或容器限制证明。目标容器和生产规格变化后必须重跑。

## 8. 一次登录如何编排

[login application](../../../internal/identity/application/login.go)把一次尝试拆成：

1. 规范化并 HMAC 摘要 login/source；
2. 在 MySQL 按固定顺序锁住两条 throttle row，先 reservation；
3. 释放事务和连接后竞争 Argon2 gate；
4. 查 account，真实或 dummy 验证；
5. 用不可伪造 receipt 对两维 reservation 做一次 finalize；
6. 成功后重新锁 account，复核 status、credential version 与 epoch；
7. 撤销替换 hint 或确定性最旧 Session，生成全新 token/CSRF；
8. 插入 Session 并确认 COMMIT；
9. 只有 COMMIT 已确认才向 HTTP 层交付 Cookie 与 snapshot。

昂贵 hash 期间不持有数据库事务。密码正确并不保证签发成功：账户可能在验证期间被 disabled、credential version 改变或 epoch 前进，事务内复核必须失败关闭。

## 9. 双维 throttle 与 admission race

单纯“读失败次数 → 做 hash → 写失败次数”在多实例中会超发。当前 [throttle repository](../../../internal/identity/adapter/mysqlrepo/throttle.go)同时管理 `login` 与可信 socket `source` 两维：

- observation window 15 分钟；
- login 5 次、source 30 次；
- backoff 从 30 秒指数增长，封顶 15 分钟；
- blocked 请求不执行 Argon2，也不继续加 failure count；
- 成功只清 login failure，不清 source failure；
- `inflight_count + admission_epoch + inflight_expires_at` 在 hash 前预占预算；
- 过期 reservation 最晚 3 秒回收，并递增 epoch 使旧 receipt 失效；
- backoff 到期只开放一个恢复 probe，避免永久锁死。

login/source 原值不入表，使用独立 HMAC key 的 domain-separated digest；该 key 不能复用 CSRF key。Redis 不参与权威限速。

## 10. Session bearer、digest 与 fixation

每次成功登录都由 `crypto/rand` 生成精确 32 个随机 bytes，编码为 43 字符无填充 base64url token。[session domain](../../../internal/identity/domain/session.go)与 [MySQL Session repository](../../../internal/identity/adapter/mysqlrepo/session.go)只持有其 SHA-256 digest 和非秘密 SessionRef。

旧 Cookie 在 POST 中最多是 replacement hint；服务端永远生成新 token，不接受客户端指定 Session ID，也不沿用旧 bearer。只有 token digest 唯一碰撞可以在最多三次预算内换 token 重试；其他数据库错误和提交不确定都不能重放整个登录。

## 11. Session 生命周期：两个时钟、一个 epoch、五个设备

当前默认值：

| 边界 | 值 |
| --- | ---: |
| idle TTL | 15 分钟 |
| absolute TTL | 8 小时 |
| touch window | 60 秒 |
| 每 account 有效 Session 上限 | 5 |

Session 在 `now >= idle_expires_at` 或 `now >= absolute_expires_at` 时失效。touch 只能延长 idle 且不能越过 absolute。account status、captured epoch、revoked state、两种 expiry 必须同时有效才产生 trusted Principal。

第六次并发登录在锁住 account 后按 `last_seen_at, issued_at, session_id` 撤销确定性最旧 Session。account-wide 安全响应可通过递增 `AuthenticationEpoch` 使旧 epoch Session 全部失效；公开 Session API 本节不暴露 logout-all 或密码重置。

## 12. Cookie policy 为什么按环境分名

[Cookie policy](../../../internal/identity/adapter/sessioncookie/cookie.go)固定：

| 环境 | name | Secure | 其他属性 |
| --- | --- | --- | --- |
| development exact loopback HTTP | `growthos_dev_session` | false | HttpOnly、Path `/`、SameSite Strict、无 Domain |
| staging/production HTTPS | `__Host-growthos_session` | true | HttpOnly、Path `/`、SameSite Strict、无 Domain |

清理 Cookie 必须复用同一 name/path/security/samesite tuple。另一环境 cookie、重复同名 cookie、非 canonical token 都拒绝，而不是在迁移期间静默接受两套 bearer。

## 13. CSRF、Origin 与 source 的职责分工

SameSite Strict 能减少跨站 Cookie 携带，但不是完整 CSRF 策略。当前 unsafe Session 请求同时要求：

1. `Origin` 与 canonical public origin 精确一致；
2. `Sec-Fetch-Site` 若存在必须精确为 `same-origin`；
3. logout 额外携带 session-bound CSRF token；
4. login 使用严格 JSON、exact Origin、双维 throttle；
5. source 只取当前 socket `RemoteAddr`，忽略所有 forwarding header。

[CSRF adapter](../../../internal/identity/adapter/csrf/csrf.go)生成 `v1.<key-id>.<nonce>.<mac>`，nonce/MAC 各 32 bytes，HMAC-SHA-256 绑定 Session digest。active key 签发；active 与至多一个 previous key 验证，previous 窗口不超过 8 小时。CSRF token 只在组件内存和 `X-CSRF-Token` 中短暂存在，不进 URL、Cookie 或 localStorage。

生产若经过反向代理，必须先定义可信 proxy allowlist 和 canonical client source 规则；在此之前只能宣称当前 loopback Compose 拓扑，而不能把 socket peer 当成互联网最终客户端 IP。

## 14. 三张表、三个数据库身份

Migration 12、13、14 分别建立：

```text
identity_workforce_account
identity_session
identity_authentication_throttle
```

进程权限被拆开：

| MySQL identity | 精确能力 |
| --- | --- |
| `growthos_app` | Lottery 两张业务表的既有最小权限；不得读 Identity |
| `growthos_identity` | workforce account 必要读取/受控 `updated_at`，Session/throttle DML |
| `growthos_identity_provisioner` | workforce account 仅 `INSERT`，无 `SELECT/UPDATE/DELETE` |
| migrator/root grant job | DDL 或 grant reconciliation；不充当 API runtime |

业务 pool 与 Identity pool 使用不同连接身份和独立生命周期；装配拒绝用户名/credential alias，也拒绝底层同一 pool。readiness 并发探测二者，任一失败为 not ready；`/health` 仍只表示进程存活。staging/production MySQL 必须使用 `verify_identity` TLS 与 CA。

## 15. Session HTTP：一条路径，三个方法

[Session HTTP adapter](../../../internal/identity/adapter/httpapi/session.go)只注册：

| 方法 | 路径 | 成功 |
| --- | --- | ---: |
| POST | `/api/v1/session` | `201` |
| GET | `/api/v1/session` | `200` |
| DELETE | `/api/v1/session` | `204` |

POST 的当前实现 contract 是 exact JSON `{login_name,password}`。产品/ADR 早期出现的 `{login,password}` 只是设计期简写，不能拿来调用实现。三个方法都拒绝 query 和伪造身份 header；POST 还要求 exact `application/json`、已知 `Content-Length 1..2048`、无 transfer encoding/trailer、无 unknown/duplicate/missing/trailing 字段。

成功 snapshot 只返回：

```json
{
  "data": {
    "authenticated": true,
    "principal": {"kind": "human", "id": "operator-1"},
    "idle_expires_at": "2026-09-02T10:15:30Z",
    "absolute_expires_at": "2026-09-02T17:45:30Z",
    "csrf_token": "v1.key.nonce.mac"
  }
}
```

错误统一为 `{error:{code,message,request_id}}`。malformed `400`、media type `415`、认证/Session `401`、Origin/CSRF `403`、持久 throttle `429`、依赖/提交不确定 `503`、未知缺陷 `500`。所有路径设置 `Cache-Control: no-store`、拒绝嵌入的 CSP/X-Frame-Options、no-referrer、nosniff 等响应头。精确协议见 [第 32 节 API 记录](../../api/lessons/lesson-32.md)。

## 16. 浏览器 transport 与会话边界

[共享 HTTP transport](../../../web/src/api/httpClient.ts)统一执行同源 absolute path、`credentials/mode=same-origin`、`cache=no-store`、`redirect=error`、有界 AbortController 和零自动重试；[Session adapter](../../../web/src/api/sessionApi.ts)公开：

```ts
createSession({ loginName, password }, options?)
readCurrentSession(options?)
revokeCurrentSession(csrfToken, options?)
```

Session timeout 默认 5 秒，只允许 100～5000ms 的 safe integer。成功 decoder 拒绝额外/缺失字段、非 `human` Principal、非法 canonical ID、假 RFC3339 和空/不可见/超长 CSRF。DELETE 只接受 exact、无 payload framing、无响应 bytes 的 `204`。`502/503/504` 非 JSON 页面被单独归为 gateway；调用者取消、内部 timeout、network、HTTP 与 contract failure 不混为一种错误。

当前 [AuthLayout](../../../web/src/layouts/AuthLayout.tsx)、[会话状态机](../../../web/src/layouts/useSessionBoundary.ts)、[登录页](../../../web/src/pages/auth/LoginPage.tsx)和[当前会话页](../../../web/src/pages/auth/CurrentSessionPage.tsx)实现 checking、anonymous、authenticated、unavailable，以及 submitting/logging-out/error 状态。密码不复制进 React state，请求发起后清空 DOM；CSRF snapshot 只保留在组件内存。登录/Session 路由使用 replace 导航，系统状态页和错误页不因认证检查被阻塞。

这只是认证 UX。当前业务、Admin、MCP、Agent 路由尚未由第 33/34 节的服务端授权和 capability 投影保护，不得把 `/login` 与 `/session` 的存在写成全站权限系统完成。

## 17. 为什么账号创建是 one-shot provision，而不是注册 API

[provision CLI](../../../cmd/growth-identity-provision/command.go)只有：

```text
growth-identity-provision create \
  --account-id VALUE \
  --login-name VALUE \
  --principal-id VALUE \
  --password-file PATH
```

没有 inline password、role/status/envelope、update/delete/upsert。password file 必须 caller-owned regular non-symlink、精确 `0600`、hard-link count 1；读取 1..514 transport bytes，只移除一个 LF/CRLF，再按 12..128 code points 与 512 bytes 验证。

Compose wrapper 把它复制进 `0700` 临时目录、以 `0444` 只读快照挂载、用 `cmp` 防替换，结束时改回 `0600`、覆零、unlink、rmdir。数据库账号只有 `INSERT`，所以 duplicate 或 COMMIT outcome unknown 都停止，不 readback、不盲重试。

已执行两轮 disposable acceptance：完整随机端口 `63228` / project `growthosl2427b81c0e6b7ceb17475ddacd`，以及 fresh-volume substitute 端口 `62978` / project `growthosprovision064035f52f31dcb6`；两轮均 PASS、精确清理、未触碰长驻 `growthos` project。证据对应 `3867584`。

真实长驻 provision 还发现过一个只有运行环境才能暴露的问题：`docker compose up --wait` 会把已经快速成功并以 `exited:0` 结束的 `mysql-grants` one-shot prerequisite 误判为等待失败。提交 `af4245e` 把 provision 与 maintenance wrapper 改为最多 180 秒的显式状态轮询；只有唯一容器进入 exact `exited:0` 才继续，ambiguous identity、非零退出、超时或意外状态全部失败关闭。这条经历也说明“Compose 命令返回失败”与“业务 prerequisite 实际失败”必须分开建模。

## 18. 为什么 maintenance 也是固定 one-shot

[maintenance application](../../../internal/identity/application/maintenance.go)与 [maintenance binary](../../../cmd/growth-identity-maintenance/main.go)只允许：

```text
growth-identity-maintenance run
make compose-identity-maintenance
```

调用者不能传 cutoff、时间、table、batch、循环或 pool 参数。进程复用 `growthos_identity` runtime credential，pool 固定 max-open/max-idle 为 1；一个 runtime 只可尝试一次，不自动 retry。

一个 server clock snapshot 派生全部边界：先用独立事务删除 `absolute_expires_at <= observed_at-7d` 或 `revoked_at <= observed_at-7d` 的 Session，最多 250；仅 idle expiry 越界不足以删除。确认后再用独立事务删除 `row_expires_at <= observed_at` 且 `inflight_count=0`、`inflight_expires_at IS NULL` 的 throttle，最多 250。预算不互借，总计不超过 500。Session 事务失败或提交不确定时不进入 throttle；Session 已提交而 throttle 失败是明确的部分进度。

专用配置只读环境/日志、公共 MySQL endpoint/database/TLS/CA/connect/write、现有 runtime Identity user/password，以及 maintenance read/ping/operation timeout；operation 为 1～30 秒，并必须为网络取消/清理在 read/write 内各预留 1 秒。默认 operation/ping 3 秒、read/write 5 秒。不开放 pool 参数，也忽略 HTTP、Lottery、Redis、Argon、CSRF、throttle key、provision/migration/business identity 等变量。

官方 disposable maintenance acceptance 已在 project `growthosl2465e15560c550fd33fc6901bf` 完成：第一次精确报告 Session/throttle/total `2/1/3`，第二次收敛为 `0/0/0`；活跃 Session fingerprint 不变，fixture residue 为 `0:0:0`，其他功能、失败、性能和 grant 断言均通过。清理精确覆盖本次 disposable containers、volumes、networks、5 个临时 images、builder/state/secrets，并保留可复用的 `growthos/identity-maintenance:lesson-32` image。

## 19. 失败语义：宁可不可用，也不伪造成功

| 场景 | 结果 |
| --- | --- |
| wrong/unknown/disabled | 统一认证失败，无 Principal |
| Argon gate/Identity MySQL/entropy 不可用 | 技术 503，无 Cookie/Principal |
| login COMMIT outcome unknown | 503，不发 Cookie，不自动 retry |
| current touch 不可确认 | 503，不复用旧 snapshot |
| logout 已确认或 Session 已失效 | 204 或 401，并清浏览器 Cookie |
| logout COMMIT outcome unknown | 503 `session_revocation_indeterminate`，清 Cookie，但不声称 server token 已撤销 |
| maintenance Session 阶段不确定 | 停止，不执行 throttle |
| readiness 任一 MySQL authority 失败 | not ready；health 可仍为 live |

日志只允许 operation、稳定 result class、request ID 和维护成功计数。password、raw Cookie、CSRF、token/digest、HMAC key、DSN、secret path 和底层数据库 cause 都不能进入 fmt/JSON/slog/错误输出。

## 20. 当前证据账本

| 范围 | 状态 | 证据 |
| --- | --- | --- |
| 浏览器当前工作树 | `ACTUAL-PASS` | `pnpm test` 23 files / 250 tests；typecheck；Vite 8.0.3 build；oxfmt check 均 PASS |
| strict transport 切片 | `ACTUAL-PASS` | 96 个 API 用例、typecheck、oxfmt PASS；提交 `267bdff` |
| Argon2 本机 baseline | `ACTUAL-PASS` | 第 7.2 节精确数值；提交 `71553fe` |
| provision disposable Compose | `ACTUAL-PASS` | 两轮 project/port 与 exact cleanup；提交 `3867584` |
| provision prerequisite 回归 | `ACTUAL-PASS` | 真实运行发现 quick `exited:0` 被 `--wait` 误判；`af4245e` 改为 180s exact-state fail-closed 轮询 |
| Go 各包测试源码 | `IMPLEMENTED-SURFACE` | domain/application/adapters/config/CLI 均有定向测试；本页不把“测试存在”写成最终执行 |
| MySQL 8.4 integration 最终重跑 | `PENDING` | 需执行真实 `TestRepositoryMySQL84Acceptance` 等目标并保留环境证据 |
| maintenance Compose fixture | `ACTUAL-PASS` | project `growthosl2465e15560c550fd33fc6901bf`：`2/1/3`→`0/0/0`、active fingerprint 不变、residue `0:0:0`、精确清理 |
| 浏览器认证旅程 | `ACTUAL-PASS` | 经 Nginx→Go→MySQL 完成 login、refresh current、logout、anonymous refresh、DB outage fail-closed、恢复重试与最终 logout |
| HTTP wire/Cookie 属性 | `PENDING` | 仍需协议级验证状态、headers、Set-Cookie tuple、旧 bearer replay 与负向 framing；浏览器页面现象不替代 wire 证据 |
| staging/production TLS | `PENDING` | 需真实 HTTPS、Secure `__Host-` Cookie 与 `verify_identity` 证据 |
| 最终 Go/全仓门禁 | `PENDING` | 文档、索引和全部实现冻结后统一执行 |

精确复现命令、预期与清理见 [QA](../../qa/lessons/lesson-32.md) 和 [运维手册](../../runbooks/identity-session-operations.md)。

浏览器证据的精确边界是：成功登录跳转 `/session` 且 Principal 精确；刷新恢复；logout 回 `/login` 且再次刷新仍匿名；有效 Session 时停止 MySQL，刷新仍保留 `/session` 并显示“暂时无法确认登录状态”，没有伪装匿名或泄露 Principal；MySQL 恢复后点击重试恢复同一 Principal，最后再次 logout。专用 E2E account 已清理 2 条 revoked Session、2 条本轮空闲 throttle 和 1 条 account，三类 residue 均为 0；私人 password file 先覆写再 unlink，父目录删除。覆写不等于在 SSD/COW 文件系统上证明物理擦除。

## 21. 选型何时需要重评

| 新证据 | 需要另立设计 |
| --- | --- |
| 企业 IAM 成为真实依赖 | OIDC issuer/subject、callback、state/nonce、account binding 与 outage policy |
| 跨地域或 Session 读成为瓶颈 | cache/replication 的撤权一致性与故障模式，不能直接把 Redis miss 当匿名 |
| 需要 service/agent 身份 | 单独 credential、delegation、actor/subject 与短时能力，不能继承 human Cookie |
| 需要 MFA/passkey/recovery | enrollment、challenge、恢复与审计完整生命周期 |
| Argon benchmark 超预算 | 同步更新 profile、资源上限、ADR 和迁移策略，不静默降参 |
| 历史清理持续打满 250+250 | 用实际 backlog/锁等待证据评估频率或 ledger，不让 caller 放大 batch |

## 22. 本节验收清单

- [x] Identity bounded context、账户、Session、throttle 与失败语义进入源码；
- [x] Argon2id profile、strict envelope、dummy work 与有界并发进入源码；
- [x] MySQL Session authority、独立 pool、grants 与 readiness 进入源码；
- [x] Cookie、CSRF、Origin/source guard 与严格 Session HTTP 进入源码；
- [x] INSERT-only provision 与固定 one-shot maintenance 进入源码；
- [x] 浏览器 transport、登录/current/logout 状态机与认证 UI 进入当前工作树；
- [x] 浏览器单元/类型/构建/格式、strict transport、Argon benchmark 和 provision Compose 已取得上表实际证据；
- [x] maintenance disposable acceptance 已证明 `2/1/3`→`0/0/0`、活跃数据不变、失败/grant 边界与精确清理；
- [x] 真实浏览器经 Nginx→Go→MySQL 已完成 login/current refresh/logout、数据库中断不冒充匿名和恢复重试；
- [ ] HTTP wire/Cookie 属性与 staging/production TLS `PENDING`；
- [ ] 本章最终 Go、doccheck、diff、全仓门禁和冻结 ref `PENDING`。

未勾选项完成前，本节只能称为“实现候选持续验收中”，不能称为最终冻结或生产就绪。

## 23. 下一节留下的问题

第 33 节要把本节产生的 trusted Principal 接入受保护 use case：从服务端权威数据加载 Resource fact，读取 exact Policy revision，执行第 31 节 evaluator，并在业务写入前强制 default deny。它还要定义授权失败与资源存在性的低披露策略、Decision audit sink 和 TOCTOU 边界。

第 34 节才把服务端最小 capability snapshot 投影给前端，裁剪导航、路由和操作；第 35 节再以匿名、过期、跨角色、跨对象、直接 URL、直接 API 和浏览器重放做完整负向闭环。

## 24. 官方资料与可追溯入口

技术依据以一手资料为准：

- [RFC 9106：Argon2](https://www.rfc-editor.org/rfc/rfc9106.html)
- [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [NIST SP 800-63B](https://pages.nist.gov/800-63-4/sp800-63b.html)
- [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
- [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- [RFC 6265：HTTP State Management](https://www.rfc-editor.org/rfc/rfc6265.html)
- [W3C Fetch Metadata](https://www.w3.org/TR/fetch-metadata/)
- [Go `crypto/rand`](https://pkg.go.dev/crypto/rand)、[`crypto/sha256`](https://pkg.go.dev/crypto/sha256)、[`crypto/hmac`](https://pkg.go.dev/crypto/hmac)
- [Go `context`](https://pkg.go.dev/context) 与 [`database/sql`](https://pkg.go.dev/database/sql)

项目内追溯：

- 产品基线：[Identity Session Authentication v1](../../product/identity-session-authentication-v1.md)
- 决策：[ADR-0028](../../decisions/ADR-0028-identity-session-authentication.md)
- API：[第 32 节 API](../../api/lessons/lesson-32.md)
- QA：[第 32 节 QA](../../qa/lessons/lesson-32.md)
- 第一性原理手记：[第 32 节设计手记](../../design-thinking/lessons/lesson-32.md)
- 面试问答：[第 32 节面试问答](../../interview/lessons/lesson-32.md)
- 运维：[Identity Session 运维手册](../../runbooks/identity-session-operations.md)
