# 第 32 节 QA：真实 Session 认证验收

- **课程正文：** [第 32 节课程](../../course/part-04/lesson-32-real-session-authentication.md)
- **API 契约：** [第 32 节 API](../../api/lessons/lesson-32.md)
- **设计手记：** [第 32 节设计手记](../../design-thinking/lessons/lesson-32.md)
- **面试问答：** [第 32 节面试问答](../../interview/lessons/lesson-32.md)
- **运行手册：** [Identity Session 运维手册](../../runbooks/identity-session-operations.md)
- **产品/决策：** [产品基线](../../product/identity-session-authentication-v1.md) / [ADR-0028](../../decisions/ADR-0028-identity-session-authentication.md)
- **记录更新日期：** 2026-09-03，Asia/Shanghai
- **稳定分支：** `codex/lesson-32-real-session-authentication`

> 本文是可复现清单，不是愿望清单。`go test` 文件存在、提交信息写了 test、Compose 脚本能被 shell 解析，都不能替代本轮真实命令。当前 HTTP wire 的登录字段精确为 `{login_name,password}`，所有规范、示例与验收都必须使用这一字段名。

## 1. 证据状态词汇

| 状态 | 精确定义 |
| --- | --- |
| `TRACKED-IMPLEMENTED` | 已进入当前分支提交，可引用源码和测试定义 |
| `WORKTREE-ONLY` | 当前共享工作树可见，但尚未形成远端冻结证据 |
| `ACTUAL-PASS` | 有已提供的精确执行范围、结果和必要清理记录 |
| `EXECUTION-PENDING` | 实现或命令存在，但本章尚无可复核的实际结果 |
| `OUT-OF-SCOPE` | 本节有意不实现，任何相邻 PASS 都不能推出它存在 |
| `FINAL-GATE-PENDING` | 只能在全部代码、文档和索引收口后由 root 最终执行 |
| `FORBIDDEN-CLAIM` | 当前证据明确禁止的结论 |

一个条目可以同时是 `TRACKED-IMPLEMENTED` 和 `EXECUTION-PENDING`：前者回答“代码有没有”，后者回答“这次真的跑过没有”。

## 2. 范围与 Git preflight

第 32 节首个提交 `e41c2af` 的父节点应为第 31 节冻结点 `d06f19a`。最终冻结时执行：

```bash
git branch --show-current
git rev-parse e41c2af^
git merge-base --is-ancestor d06f19a HEAD
git log --first-parent --oneline d06f19a..HEAD
git status --short
```

实际：当前分支名精确匹配，`e41c2af^` 为 `d06f19a`，历史线性且无 merge，`main` 未改；最终提交不内嵌自身 SHA，同名稳定分支与累计学习分支共同指向的实际 tip 定义完整终点。

## 3. 本节应当实现与明确不实现

### 3.1 实现面清单

| 层 | 代码锚点 | 必须观察的不变量 |
| --- | --- | --- |
| account/session/throttle domain | [`internal/identity/domain`](../../../internal/identity/domain) | closed state、canonical ID、zero-result、脱敏 |
| login/resolve/revoke/maintenance | [`internal/identity/application`](../../../internal/identity/application) | trusted Principal 只由已验证路径产生；失败零输出 |
| Argon2id | [`passwordhash`](../../../internal/identity/adapter/passwordhash) | strict PHC、dummy work、constant-time compare、有界 gate |
| MySQL authority | [`mysqlrepo`](../../../internal/identity/adapter/mysqlrepo) | 事务锁序、commit unknown、digest-only、条件更新 |
| browser security | [`sessioncookie`](../../../internal/identity/adapter/sessioncookie)、[`csrf`](../../../internal/identity/adapter/csrf)、[`requestguard`](../../../internal/identity/adapter/requestguard) | exact Cookie、session-bound CSRF、Origin/source 边界 |
| HTTP | [`httpapi`](../../../internal/identity/adapter/httpapi) | 单一路径三方法、strict request、低披露 response |
| runtime/config | [`appconfig`](../../../internal/platform/appconfig)、[`cmd/growth-api`](../../../cmd/growth-api) | 双 pool、TLS、budget、readiness、secret isolation |
| provision | [`cmd/growth-identity-provision`](../../../cmd/growth-identity-provision) | create-only、file secret、INSERT-only、one-shot |
| maintenance | [`cmd/growth-identity-maintenance`](../../../cmd/growth-identity-maintenance) | fixed run、one clock、250+250、one pool/attempt |
| browser adapter/UI | [`web/src/api`](../../../web/src/api)、[`AuthLayout`](../../../web/src/layouts/AuthLayout.tsx)、[`pages/auth`](../../../web/src/pages/auth) | strict transport、内存 Session、诚实故障状态 |

### 3.2 `OUT-OF-SCOPE`

- 第 33 节服务端 Policy repository、Resource fact 加载与 RBAC enforcement；
- 第 34 节 capability endpoint、导航/路由/按钮/字段裁剪；
- 第 35 节跨角色、跨对象、跨 tenant、直接 API/URL 的完整越权 E2E；
- 公开注册、密码修改/找回、MFA、Passkey、OIDC、service/agent credential；
- Redis Session、JWT/refresh token、remember-me、跨区 Session；
- production proxy client-IP 信任链和生产渗透测试结论。

### 3.3 `FORBIDDEN-CLAIM`

- “登录以后才可以访问 `/admin`、`/mcp`、`/agent`”；当前 Auth boundary 只包 `/login` 与 `/session`；
- “不同 Role 已看到不同菜单”；本节 DTO 根本不返回 Role/capability；
- “Session 已接入，所以业务 API 已授权”；认证与授权尚未组合；
- “密码已从浏览器内存彻底擦除”；JS string 无法可靠 zeroize；
- “SameSite 已经解决 CSRF”或“CSRF 可以解决 XSS”；
- “所有 503 都证明数据库没有写入”或“logout 503 都保留 Cookie”；
- “现有 socket source 就是 production 最终客户端 IP”；
- “单元测试通过等于真实 Nginx/MySQL/浏览器/TLS 已通过”。

## 4. Domain 与 application 负向验收

### 4.1 账号与密码边界

逐项验证：

- `LoginName` 只接受 `[a-z][a-z0-9._-]{2,63}`，不 trim/case-fold/normalize；
- 登录 password 接受 1～128 code points、最多 512 UTF-8 bytes；enrollment 是 12～128；
- account 只接受 `enabled|disabled`、non-zero credential version/epoch 和 canonical UTC；
- AccountID/LoginName/PrincipalID/password envelope 的 fmt、`%+v`、`%#v`、JSON、slog 不泄漏私有值；
- 任意 partial/unknown/overflow shape 都返回 error + zero domain value。

### 4.2 Session 生命周期

必须覆盖：

- `now == idle_expires_at` 与 `now == absolute_expires_at` 都已失效；
- touch 只在 60 秒窗口后发生，不能越过 absolute；
- revoke 与 touch 竞争时，撤销不能被续活覆盖；
- account disabled、epoch mismatch、corrupt row 均不产生 Principal；
- 签发先锁 account 并证明当前 epoch 的 active Session 不超过五个；存量已超过五个时必须 fail closed，不能让 replacement hint 掩盖坏状态；
- 合法、同 account 且仍 active 的 replacement hint 先按 `security_response` 撤销；若移除 hint 后仍恰有五个 active Session，才按 `last_seen_at, issued_at, session_ref` 撤销唯一确定性最旧值；
- token digest collision 只允许全新 candidate 最多三次，其他错误不重试；
- issue/revoke COMMIT unknown 保留明确 error class，不能当 rollback。

### 4.3 双维 throttle

必须覆盖 login/source 固定锁序、原子双 reservation、阈值 `5/30`、15 分钟窗口、30 秒至 15 分钟 backoff、blocked 不 hash/不加 failure、成功只 reset login、3 秒 lease 回收、epoch fencing 和单恢复 probe。

关键测试定义可在下列文件审查：

- [application login tests](../../../internal/identity/application/login_test.go)
- [application edge cases](../../../internal/identity/application/edge_cases_test.go)
- [MySQL Session tests](../../../internal/identity/adapter/mysqlrepo/session_test.go)
- [MySQL throttle tests](../../../internal/identity/adapter/mysqlrepo/throttle_test.go)

上述实现面已进入本轮真实定向执行：`go test -count=1 ./internal/identity/...`、对应 race、`-shuffle=on -count=10` 均 PASS。配置与四个运行入口另以 `go test -count=10 ./internal/platform/appconfig ./cmd/growth-api ./cmd/growth-migrate ./cmd/growth-identity-provision ./cmd/growth-identity-maintenance` PASS。该结果证明当前 Go 测试切片稳定，不替代真实 HTTP、MySQL、TLS 或最终全仓门禁。

## 5. Argon2id 可复现验收

### 5.1 正确性与资源攻击面

```bash
go test -count=1 ./internal/identity/adapter/passwordhash
go test -race -count=1 ./internal/identity/adapter/passwordhash
go test ./internal/identity/adapter/passwordhash \
  -run '^$' -fuzz '^FuzzParseEnvelope$' -fuzztime=10s
```

执行 fuzz 前必须先核对真实 target：

```bash
go test ./internal/identity/adapter/passwordhash -list '^Fuzz'
```

若名称与示例不同，按 `-list` 输出修正；`no fuzz tests to fuzz` 不计 PASS。本轮已实际取得以下机器观测值，执行次数只属于该轮调度与语料，不能横向当吞吐指标：

| target | fuzz time | executions |
| --- | ---: | ---: |
| passwordhash `FuzzParseEnvelope` | 10s | 1,485,248 |
| passwordhash `FuzzWorkGateCapacityAndCancellation` | 10s | 625,627 |
| application `FuzzPasswordBounds` | 5s | 255,368 |
| httpapi `FuzzDecodeLoginRequestStrict` | 10s | 25,908 |
| domain `FuzzTokenDigest` | 10s | 44,251 |
| domain `FuzzPrincipalID` | 5s | 707,057 |
| domain `FuzzLoginName` | 5s | 574,005 |
| domain `FuzzThrottleDigest` | 5s | 39,479 |
| domain `FuzzThrottleAggregateCount` | 5s | 683,345 |

passwordhash 还完成普通测试 `count=10` 与 race PASS。`FuzzWorkGateCapacityAndCancellation` 在修复前发现：有可用 slot 与 1ms timer 同时 ready 时，旧 `select` 可能随机选择 timer 并误报 503 unavailable。提交 `5af29e2` 在 context 预检查后先尝试 nonblocking available fast-path，只在满槽时启动 timer，并固化 `(capacity=2, occupied=1, cancel=false)` seed。它是资源准入时序/错误 503 缺陷，不是 credential 绕过。

### 5.2 已执行 benchmark

复现命令：

```bash
go test ./internal/identity/adapter/passwordhash \
  -run '^$' \
  -bench '^BenchmarkCurrentProfileLoginVerification$' \
  -benchmem -count=10
```

`ACTUAL-PASS` 记录（Apple M2 Pro，本地 baseline，提交 `71553fe`）：

| 模式 | 结果 |
| --- | --- |
| serial | `26.638354ms/op`；current profile 19.00 MiB；`19,926,865 B/op`；36 allocs/op |
| parallel capacity=2 | `14.179475ms/op`；maximum profile 38.00 MiB；`19,925,776 B/op`；35 allocs/op |
| `/usr/bin/time -l` | max RSS `107,823,104` bytes；peak footprint `105,005,608` bytes |

判定：足以作为当前开发机回归基线，不足以证明生产吞吐、p99、容器 memory limit 或 DoS 容量。参数、Go 版本、硬件或容器限额变化时必须重跑。

## 6. Cookie、CSRF 与 request guard

建议定向命令：

```bash
go test -count=1 \
  ./internal/identity/adapter/sessioncookie \
  ./internal/identity/adapter/csrf \
  ./internal/identity/adapter/requestguard

go test -race -count=1 \
  ./internal/identity/adapter/sessioncookie \
  ./internal/identity/adapter/csrf \
  ./internal/identity/adapter/requestguard
```

必须检查：

| 主题 | 正向 | 负向 |
| --- | --- | --- |
| development Cookie | exact loopback、`growthos_dev_session`、非 Secure | 非 loopback、alternate name、重复/坏 token |
| staging/prod Cookie | HTTPS、`__Host-growthos_session`、Secure | HTTP、Domain、错误 path/tuple |
| CSRF | active 签发；active/previous 在窗口内验证 | wrong session/key/MAC、previous 超 8h、重复/空 header |
| Origin | exact canonical value | missing、duplicate、same-site/cross-site、Host/Referer fallback |
| source | socket `RemoteAddr` | `Forwarded`/`X-Forwarded-*` 覆盖 |

当前这些测试是 `TRACKED-IMPLEMENTED`，且已被本轮 Identity 普通/race/shuffle 门禁覆盖；真实 development HTTP 的一部分 Cookie/CSRF/Origin/Fetch 行为见第 13 节。staging/production TLS、可信代理来源与浏览器设备级属性仍单独 `PENDING`。

## 7. HTTP adapter 机械契约

### 7.1 路由与登录 request

关键测试定义：

- `TestRegisterRoutesMountsOnlyExactSessionMethods`
- `TestLoginStrictRequestVocabularyHasNoApplicationSideEffects`
- `TestLoginSuccessRotatesCookieAndReturnsMinimalDTO`
- `TestCurrentSessionSuccessIsMinimalAndCSRFBound`
- `TestLogoutOrchestrationRejectsBeforeRevokeAndClearsOnlyRequiredFailures`
- `FuzzDecodeLoginRequestStrict`

建议命令：

```bash
go test -count=1 ./internal/identity/adapter/httpapi
go test -race -count=1 ./internal/identity/adapter/httpapi
go test ./internal/identity/adapter/httpapi \
  -run '^$' -fuzz '^FuzzDecodeLoginRequestStrict$' -fuzztime=10s
```

登录 request matrix：

| 变体 | 预期 |
| --- | --- |
| exact `POST`, no query, exact Origin, exact JSON | 进入 application |
| `{login_name,password}` 以外任意字段集合 | `400 invalid_request`，application 零调用 |
| duplicate/trailing/bad UTF-8/unpaired surrogate | `400`，零调用 |
| `application/json; charset=utf-8`、重复 Content-Type | `415`，零调用 |
| unknown length、0、>2048、TE、trailer、length mismatch | `400`，零调用 |
| missing/duplicate/wrong Origin；Fetch-Site 非 same-origin | `403`，零调用 |
| Authorization/Principal/Role/Scope/Tenant 等伪造 header | `400`，零调用 |
| known wrong/unknown/disabled | 同一 `401 authentication_failed` |
| persistent throttle | `429 authentication_throttled`，不执行 Argon2 |
| gate/DB/random/commit unknown | `503 authentication_unavailable`，无可用 Cookie |

### 7.2 current/logout matrix

| 场景 | current | logout |
| --- | --- | --- |
| valid Cookie | `200` exact snapshot | exact Origin+CSRF 后 confirmed `204` |
| missing/invalid/inactive | `401` + clear attempt | `401` + clear attempt |
| Origin/CSRF bad | GET 不要求 | `403`，不清 Cookie |
| dependency/touch failure | `503`，无 trusted Principal | ordinary unavailable，不声称撤销 |
| revoke commit unknown | n/a | clear Cookie + `503 session_revocation_indeterminate` |
| handler deadline after revoke/clear | n/a | 可能 generic `503`，不能倒推出数据库状态 |

所有成功/失败路径还要断言 `Cache-Control: no-store`、CSP、no-referrer、Permissions-Policy、CORP、nosniff 和 frame deny。当前 handler 已通过 Identity 普通/race/shuffle，`FuzzDecodeLoginRequestStrict` 10 秒执行 25,908 次；第 13 节 HEAD `9fc4e06` 的增强 wire gate 又对每次 Session 响应核对这些 header 的单值与精确值，并实际覆盖 raw 429、TE/Trailer、2049-byte 普通 body 和 clear-Cookie。它仍不能推出未发送的 raw Content-Length 变体或 staging/production TLS 已通过。

## 8. MySQL schema、grants 与真实 adapter

### 8.1 Migration 12～14

先使用 disposable schema，绝不能对长驻开发库直接授予测试写权限。Identity schema integration 需要显式 opt-in：

```bash
GROWTHOS_TEST_MYSQL_ALLOW_IDENTITY_SCHEMA_CHANGES=lesson-32-isolated-schema \
GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_ADDRESS=127.0.0.1:3306 \
GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_DATABASE='<disposable_db>' \
GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_USER='<migration_user>' \
GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_PASSWORD='<secret-from-safe-source>' \
go test -count=1 ./migrations \
  -run '^TestIdentitySchemaMySQLIntegration$'
```

不要把真实 password 写进 shell history；上例只展示变量名。没有 exact opt-in 而显示 `skip` 不计 PASS。上述 schema contract 已由下述官方门禁实际覆盖。

### 8.2 Repository MySQL 8.4 acceptance

```bash
GROWTHOS_TEST_IDENTITY_MYSQL_ACCEPTANCE=lesson-32-disposable-mysql-8.4 \
GROWTHOS_TEST_IDENTITY_MYSQL_ADMIN_DSN='<admin-dsn-from-safe-source>' \
GROWTHOS_TEST_IDENTITY_MYSQL_RUNTIME_DSN='<runtime-dsn-from-safe-source>' \
go test -count=1 ./internal/identity/adapter/mysqlrepo \
  -run '^TestRepositoryMySQL84Acceptance$'
```

helper 要求 distinct admin/runtime DSN，并严格比较 runtime direct grants。真实 adapter 门禁已核查账户恢复、并发 reservation/epoch fencing、account-lock cancel 无部分 Session、五会话/touch/logout、closed-boundary maintenance、权限正反向与 cleanup。raw COMMIT outcome-unknown 故障注入仍只由定向测试定义覆盖，不能写成这次真实 MySQL 已注入网络断提交。

### 8.3 grant allow/deny

预期 allowlist：

- `growthos_app`：Lottery 既有两表能力，不读 Identity；
- `growthos_identity`：workforce 必要读/受控 updated_at，Session/throttle DML；
- `growthos_identity_provisioner`：workforce 仅 INSERT；
- `@@GLOBAL.mandatory_roles` 必须为空。

正向 SQL 和 forbidden SQL 都要真实执行；只看 `SHOW GRANTS` 不足以替代行为证据。独立门禁已对 runtime 执行 workforce lock-read/`updated_at` 正向操作，并真实拒绝 credential/login 更新、account INSERT/DELETE、`schema_migrations`、Lottery、Marketing 与 DDL；helper 同时核对 direct grants 与空 mandatory role。长期 Compose 三账号收敛仍是另一条部署证据，不能由这个两身份 disposable gate 单独推出。

### 8.4 官方 Lesson 32 MySQL 8.4.11 门禁：`ACTUAL-PASS`

授权入口：

```bash
GROWTHOS_LESSON32_MYSQL_ACCEPTANCE=run-disposable-mysql-8.4.11 \
  make lesson32-mysql-acceptance
```

本轮从 HEAD `4149576` 启动，`19s`、exit `0`。随机容器 `growthos-lesson32-mysql-e4e83e6c1b0e7036f42e65f9`，精确 label `com.growthos.acceptance.lesson32=run-e4e83e6c1b0e7036f42e65f9`，`mysql:8.4.11` 的 `VERSION()` 为 `8.4.11`，只绑定随机 loopback port，数据目录为 tmpfs。

- `TestIdentitySchemaMySQLIntegration` PASS：fresh v14、second-up、v11→v14、旧数据/结构不漂移、约束/索引/FK/CHECK/binary semantics、dirty fail-closed；测试清理后数据库为 0 表；
- migration immutability 与 inventory latest 14 PASS；随后真实 `growth-migrate up` 报 `applied/14`，`status` 报 `clean/14/latest=14`；
- `TestRepositoryMySQL84Acceptance` 及其 credential、并发、取消、Session lifecycle、maintenance、grant denial 子项全部 PASS；
- 终态 `schema_migrations=14:0`，workforce/session/throttle 行数 `0:0:0`，保留 DDL 探针 `identity_l32_forbidden=0`；
- 脚本逐个覆写再 unlink 私有 Secret 并删除目录；这不保证 SSD/CoW/快照物理擦除。随机 name、label、临时目录外部复核均为 0，长期 `growthos` containers/volumes/networks 前后不变。

## 9. 配置、双 pool 与 readiness

定向命令：

```bash
go test -count=1 ./internal/platform/appconfig ./cmd/growth-api
go test -race -count=1 ./internal/platform/appconfig ./cmd/growth-api
```

必须检查：

- Identity 与 business user/password/pool 不可 alias，构造后底层 pool 也不可同一对象；
- env 与 `_FILE` secret source 互斥；任一失败返回 zero config；
- staging/production 强制 `verify_identity` 与 CA；
- readiness 并发探测两 pool，任一失败为 not ready；health 与 readiness 不混淆；
- Identity 故障不 fallback 到 mock/Header/anonymous/Redis；
- config 的 fmt/JSON/slog 和聚合错误不回显 secret/path/rejected value。

当前实现已进入上面的普通/race/count=10 门禁；双 pool readiness 也在 Compose 既有 smoke/故障场景中保持 fail-closed。staging/production `verify_identity`、CA/host 与可信代理仍需对应环境实际验证。

## 10. Provision one-shot 验收

### 10.1 复现入口

准备 caller-owned、regular、non-symlink、hard-link count 1、精确 `0600` 的绝对 password file，然后执行：

```bash
make compose-identity-provision \
  IDENTITY_ACCOUNT_ID='<canonical-account-id>' \
  IDENTITY_LOGIN_NAME='<canonical-login-name>' \
  IDENTITY_PRINCIPAL_ID='<canonical-principal-id>' \
  IDENTITY_PASSWORD_FILE='/absolute/private/password-file'
```

不得打开 `set -x`，不得把 password 放进 env/flag/output。预期：operations-only container、non-root、read-only、仅 provisioner DB secret + 临时 enrollment snapshot；账号 enabled，credential/auth epoch 均为 1；二进制不 readback、不 upsert、不返回 envelope。

### 10.2 已执行证据

`ACTUAL-PASS`（对应提交 `3867584`）：

| 场景 | 隔离环境 | 结果 |
| --- | --- | --- |
| full disposable acceptance | project `growthosl2427b81c0e6b7ceb17475ddacd`，随机端口 `63228` | PASS；exact cleanup；长驻 `growthos` 未动 |
| fresh-volume substitute | project `growthosprovision064035f52f31dcb6`，随机端口 `62978` | PASS；exact cleanup；无残留 |

该证据只支撑已执行的 provision Compose 情景；不能扩写成真实 HTTP、maintenance 或 TLS 已通过。

### 10.3 真实运行发现的 prerequisite 状态 bug

长驻 provision 曾实际遇到：`docker compose up --wait` 把已经快速成功并进入 `exited:0` 的 `mysql-grants` one-shot prerequisite 判为等待失败。根因不是 grant job 失败，而是“长期 running/healthy”的等待模型不适合成功后立即退出的任务。

提交 `af4245e` 将 provision 与 maintenance wrapper 改为最多 180 秒的有界轮询。验收判定精确为：

| materialized state | wrapper 结果 |
| --- | --- |
| 唯一、合法 container ID 且 `exited:0` | 成功，继续下一步 |
| `created:0` / `running:0` / `restarting:0`，未超 180s | 继续等待 |
| 空 ID 且未超时 | 继续等待创建 |
| ambiguous/非法 ID、`exited:nonzero`、意外 state、inspect失败 | fail closed |
| 达到 180s | fail closed |

这是一条 `ACTUAL-PASS` 的缺陷发现与修复证据：验收必须同时保留曾经观察到的失败、修复机制和回归结果，不能只写“Compose 启动成功”。

## 11. Maintenance one-shot 验收

### 11.1 静态与单元预期

入口只有：

```bash
make compose-identity-maintenance
# container 内精确执行：growth-identity-maintenance run
```

必须验证：

- 只读 public env/log、公共 MySQL connection、runtime Identity credential 和专用 read/ping/operation timeout；
- operation 1～30s，且在 read/write 内各留 1s network cancellation/cleanup；默认 operation/ping 3s、read/write 5s；
- HTTP/Lottery/Redis/Argon/CSRF/throttle HMAC/provision/migration/business/pool 参数均不被 lookup；
- one pool max-open/max-idle 1、one attempt、无 loop/retry/caller cutoff；
- config/runtime/result 全 fmt/JSON/slog 与聚合错误脱敏。

关键测试定义包括 `TestMaintenanceServiceFreezesOneClockSnapshotAndBudgets`、`TestRunMaintenanceUsesClosedCutoffsStableOrderAndIndependentBudgets`、`TestSessionMaintenanceCommitUnknownStopsSecondStageAndStaysRedacted` 和 `TestMaintenanceRuntimeIsOneShotAndNeverRetriesCommitUnknown`。名称以 `go test -list` 为最终事实。

### 11.2 真实 fixture

准备同一 server-clock 边界两侧的 fixture，至少包含：

| 数据 | 预期 |
| --- | --- |
| `absolute_expires_at <= observed-7d` 或 `revoked_at <= observed-7d` Session | 最多 250，删除时再次校验 |
| idle expired、但 absolute/revoked 未越过 7d cutoff | 保留 |
| active Session | 保留 |
| `row_expires_at <= observed` 且 inflight=0/expiry NULL throttle | 最多 250 |
| active 或 inflight throttle | 保留 |
| session stage failure/unknown | throttle stage 不启动 |
| session 已提交、throttle 失败 | 明确部分进度，不回滚已提交 Session |
| 第二次运行 | 收敛；零删除仍成功 |

还必须断言 session/throttle 各 250、总计 500、预算不互借、容器退出后无残留。

### 11.3 官方 disposable acceptance：`ACTUAL-PASS`

隔离 project：`growthosl2465e15560c550fd33fc6901bf`。

| 断言 | 实际结果 |
| --- | --- |
| 第一次运行计数 | Session `2`、throttle `1`、total `3` |
| 第二次收敛计数 | 精确 `0/0/0` |
| active Session | fingerprint 前后不变 |
| fixture residue | Session/throttle/其他精确 `0:0:0` |
| 功能/失败/性能/grant 断言 | 全部 PASS |
| one-shot 容器 | 退出后无项目残留 |

清理前逐一解析目标，随后移除本次 disposable containers、volumes、networks、5 个临时 images、builder/state/secrets；保留可复用 `growthos/identity-maintenance:lesson-32` image。该结果证明这一官方 fixture 的有界删除、收敛、不误删 active Session、失败/权限边界和清理，不等于生产数据规模或长期调度 SLO。

## 12. 浏览器 transport 与 UI 单元证据

### 12.1 strict transport

精确切片：

```bash
cd web
pnpm exec vitest run src/api/httpClient.test.ts src/api/sessionApi.test.ts
pnpm typecheck
pnpm exec oxfmt --check src/api/httpClient.ts src/api/httpClient.test.ts \
  src/api/sessionApi.ts src/api/sessionApi.test.ts
```

`ACTUAL-PASS`：96 个 API 用例、typecheck、oxfmt PASS，对应 `267bdff`。

这些用例应证明 same-origin credentials/mode、no-store、redirect error、5s 默认且 100..5000ms、有 caller abort、timeout/network 分类、无 retry、exact 201/200/204、成功 DTO strict decoder、request-id 一致性、HTML gateway 和 DELETE 无 payload。

### 12.2 当前前端全量

已执行：

```bash
cd web
pnpm test
pnpm typecheck
pnpm build
pnpm exec oxfmt --check \
  src/api/httpClient.ts src/api/httpClient.test.ts \
  src/api/sessionApi.ts src/api/sessionApi.test.ts \
  src/layouts/AuthLayout.tsx src/layouts/AuthLayout.test.tsx \
  src/layouts/useSessionBoundary.ts src/layouts/useSessionBoundary.test.tsx \
  src/pages/auth/LoginPage.tsx src/pages/auth/CurrentSessionPage.tsx \
  src/routes/appRouter.tsx src/routes/appRouter.test.tsx
```

`ACTUAL-PASS`：23 files / 250 tests PASS；typecheck PASS；Vite 8.0.3 production build PASS；oxfmt check PASS。

这支持当前 React 单元/路由契约，但不能替代真实浏览器或 HTTP wire。仓库根 [Design QA](../../../design-qa.md) 还记录了 1719 × 862 桌面、390 × 844 移动和 1280 × 720 authenticated state 的真实核查；键盘顺序、焦点交接、标签/`aria`/live status 与 reduced-motion 均 PASS。下文另有核心浏览器旅程，旧 bearer replay 与 Cookie wire tuple 则由 HTTP gate 证明。仍未直接检查的是浏览器 storage/console 全矩阵，以及更广设备与辅助技术认证。

## 13. 真实 Compose HTTP 验收

按 [运行手册](../../runbooks/identity-session-operations.md)用 disposable project、随机 loopback port、非默认 account 执行。禁止输出完整 Cookie/CSRF/password。

### 13.1 历史核心 wire：`ACTUAL-PASS`，但不是冻结 provenance

```text
POST exact login -> 201 + rotated Set-Cookie + exact snapshot
GET with cookie -> 200 + same trusted Principal + current expiry/CSRF
POST with old cookie -> 201 + replacement token; old session security_response
DELETE with cookie/origin/CSRF -> 204 exact empty + clear Cookie
replay old cookie -> 401 unauthenticated
```

一次早期验收从 HEAD `8a5e0ce`、认证代码 baseline `5af29e2` 的工作树启动；随机 project `growthosl24d2103fd496568ceac960d315`，总耗时 `302s`、exit `0`。它实际证明：

- development Cookie header 与 curl jar 证明 host-only、HttpOnly、SameSite Strict、Path `/`、非 Secure loopback 等必需属性/形状，replacement token 与旧 token 不同；logout 为无 body/无 Content-Type 的 204 并执行清除尝试。它没有使用后来收紧的六字段签发 tuple 与完整删除 tuple 精确断言，后两项只能归入 HEAD `9fc4e06` 的增强证据；
- missing/wrong/cross-session CSRF、missing/duplicate/wrong Origin，以及 `same-site`/`cross-site` 或重复 Fetch Metadata 均 403 且零误撤销；
- wrong/unknown/disabled login 为同形 `401 authentication_failed`；expired/epoch-mismatch/disabled current 为同形 `401 unauthenticated`；
- 六次独立登录后数据库精确为 total/active/concurrency-limit `8:5:1`，最旧 bearer 401，最新 bearer 200；
- MySQL 停止时有效 current 与 login 都为 503 且无 Set-Cookie；恢复的是同一 MySQL container，原有效 Session 再次 current 为 200；
- 已执行的 strict 子集覆盖 415 media type、malformed/duplicate/trailing JSON、query/body 与伪造认证 header；响应均关联唯一 request ID 与 `no-store`；
- password、wrong password、所有观测到的 raw Session token/CSRF marker 未出现在 provisioner/API/Web 日志；终态先观察 `disabled:2:10:3`，再只删除本轮 Session/throttle/account `10:3:1`，residue `0:0:0`；
- 本轮 disposable containers/volumes/networks/images、builder/state、Secret/response/Identity 私有目录全部精确清理，外部复核无随机 project/label/name/temp 残留，长期 `growthos` 资源前后不变。

这条记录是有效的历史核心行为证据，但 `8a5e0ce` 提交本身尚未包含后来落盘的 Session gate；运行的是该 HEAD 上的工作树候选。它不能作为“冻结提交内脚本可复现”的 provenance，也不能覆盖后来新增的 raw 429、edge framing 与全状态 header 断言。

### 13.2 已提交增强 gate 的失败—修复—复验链

| 提交 / project | 真实结果 | 证据边界与处理 |
| --- | --- | --- |
| `903fd9f` / `growthosl24c1bf7ce29e5efa417fae6932` | `ACTUAL-FAIL`，exit `2` | Session 前置的 Compose、Lottery、cache、performance、grants、maintenance 等门禁已通过；随后 macOS BSD awk 把循环变量 `index` 解析为内建名而报错，尚未进入 Session wire 断言。脚本完成精确清理；没有可信总耗时，禁止补写。 |
| `51b52e0` / `growthosl240da11b08420700da0d07428f` | `ACTUAL-FAIL` | 在此前已统一 API security-header owner 的基础上，修复 BSD awk、invalid-Host JSON 421 与 exact Cookie 断言；复跑却在第二次相同后端 image build 获取 Docker Hub OAuth token 时遇到 `EOF`，未进入 Session。外部复核 containers/volumes/networks/images/builder/temp 均为 `0`。 |
| `9fc4e06` / `growthosl24f6a5acf4d242695ad3e2df19` | `ACTUAL-PASS`，exit `0` | 将 API、migrate、identity-provision、identity-maintenance 四个后端目标合并为一次 Compose Bake；共享 Go builder 实际只执行一次，其他 target 命中 cache。完整官方门禁通过；没有可信总耗时，禁止推算。 |

这条链必须完整保留：`903fd9f` 是验收脚本可移植性失败，不是产品 Session 失败；`51b52e0` 的外部 registry 失败也不能伪装成 Session 结果；只有 `9fc4e06` 为当前已提交增强 gate 的完整成功证据。

### 13.3 HEAD `9fc4e06` 增强 development wire：`ACTUAL-PASS`

最新成功轮实际覆盖：

- 201 login → 200 current → replacement → 204 logout → old-bearer replay，exact Session DTO、CSRF、Origin/Fetch Metadata、五会话确定性淘汰、同形 401，以及 MySQL outage 503 / 同一 Session recovery；
- development issue `Set-Cookie` 与 clear-Cookie 都按完整 tuple 精确比较；malformed/query/media/auth/origin/fetch 等不应改 Cookie 的错误状态均为零 `Set-Cookie`；invalid、replaced、logged-out、expired、epoch-mismatch、disabled 的 401 均返回 exact deletion tuple；
- 每次 Session 响应只有一组 exact `Cache-Control: no-store`、CSP、CORP、Permissions-Policy、`Referrer-Policy: no-referrer`、nosniff、DENY 与 request ID；非法 Host 的 Session 请求在 edge 返回 correlated JSON `421 misdirected_request`，并保持同一 canonical security contract；
- raw `Transfer-Encoding: chunked` 与 `Trailer` 在 edge 返回 400；普通声明长度的 2049-byte body 继续到 Go 并返回 `400 invalid_request`，没有被 edge 误判成 framing；
- 同 login 五次失败后第六次为 Cookie-free 429，持久态精确 `2:10:0:1`；清理这两行后，30 个不同 login 共用 source，第 31 个不同 login 为 Cookie-free 429，持久态精确 `31:30:1:60:0:1`；
- 清理前账户/Session/throttle 终态为 `disabled:2:10:31`；只删除本轮 fixture 得到 `10:31:1`，随后三表 residue `0:0:0`；password、raw Session token 与 CSRF marker 的进程/网关日志扫描零命中；
- 同一次官方脚本的完整 Compose topology、Secret/grant、maintenance、Lottery、Redis cache/repair/degradation 与 performance gates 全部通过；disposable Docker 与临时目录外部 residue 全为 `0`，长期 `growthos` 资源前后不变且健康。

### 13.4 仍未完成的 wire/fault 范围

增强门禁已经真实覆盖 TE、Trailer、2049-byte 普通 body、raw 429、exact Cookie/clear-Cookie、安全 header 单值矩阵和 invalid-Host JSON 421，因此不得再把它们标成 `PENDING`。仍未通过代理逐一发送并证明的 framing 变体是 raw Content-Length absent、zero 与 mismatch；对应 handler/race/fuzz 证据不能替代这些真实 proxy wire。issue/revoke 的 COMMIT outcome-unknown 仍只有 deterministic unit/repository failure coverage，尚无真实网络断提交 fault injection。

## 14. 真实浏览器与 TLS 验收

### 14.1 真实浏览器核心旅程：`ACTUAL-PASS`

真实 browser context 经 Nginx → Go → MySQL 已观察：

1. 登录成功后跳转 `/session`，显示的 Principal 与专用 E2E account 精确一致；
2. 刷新 `/session` 能从服务端恢复当前会话；
3. logout 后回到 `/login`，再次刷新仍为匿名，没有从旧页面状态恢复 Principal；
4. 建立有效会话后停止 MySQL，刷新仍保留 `/session` 路径并显示“暂时无法确认登录状态”；它没有把技术故障伪装成 anonymous，也没有泄露 Principal；
5. 恢复 MySQL 后点击“重新核查”，同一 Principal 恢复；
6. 最终再次 logout，测试账户进入可清理状态。

这组证据证明浏览器核心状态机和真实依赖 outage 的 fail-closed 呈现。仓库根 [Design QA](../../../design-qa.md) 还在 1719 × 862、390 × 844 与 1280 × 720 三种状态/视口完成布局和交互核查，并验证键盘顺序、focus handoff、语义 label、`aria-pressed`、status/alert 与 reduced-motion。它没有窥探浏览器内部 Cookie store；第 13 节已通过受控 HTTP header/curl jar 证明 development Cookie tuple、原始 201/200/204 与 replay，但这仍不等于浏览器直接读取 HttpOnly store，也不能证明 production Secure `__Host-` 属性。

浏览器侧仍需补证的是 storage/console 全矩阵，以及更广浏览器、设备和辅助技术认证；真实 COMMIT outcome-unknown fault 见第 13.4 节。`/home`、`/admin`、`/mcp`、`/agent` 当前仍不受本 Auth boundary，这是L33～L35停止线，而不是本次浏览器 PASS 所包含的权限能力。

### 14.2 staging/production TLS `PENDING`

需要真实 HTTPS origin 证明 `__Host-growthos_session; Secure; HttpOnly; Path=/; SameSite=Strict`，无 Domain；MySQL 使用 `verify_identity` + CA。development loopback HTTP 证据不能替代该项。

### 14.3 专用 E2E 数据与秘密清理：`ACTUAL-PASS`

三条清理证据必须分开理解：早先浏览器旅程删除 2 条 revoked Session、2 条本轮空闲 throttle、1 条 account；历史核心 HTTP 工作树轮删除 `10:3:1`；HEAD `9fc4e06` 增强官方轮删除 `10:31:1`。每轮随后都确认对应 Identity fixture residue 为零，没有用通配条件删除共享/长驻数据；增强轮的 Docker/temp 外部 residue 也全为零，长期 `growthos` 不变且健康。

私人 password file 先按已知长度覆写，再 unlink，最后删除其私有父目录。该证据只证明应用层清理步骤完成；SSD、copy-on-write、快照和文件系统缓存意味着覆写不能被描述为物理不可恢复。

## 15. 安全泄漏与格式边界

对 password、Cookie、CSRF、token/digest、HMAC key、MySQL secret、DSN/path sentinel 执行：

```text
fmt %v / %+v / %#v / %s / %q
json.Marshal
slog.Any / slog.LogValuer
joined validation errors
CLI stdout/stderr
HTTP log/response
browser URL/storage/console
```

预期只见 redacted/stable class 或允许的 request ID/count；任何 sentinel 命中都失败。HEAD `9fc4e06` 增强官方门禁已把两份 password 与本轮全部 raw Session/CSRF 值作为私有 pattern 扫描 provisioner/API/Web 输出并得到零命中；它没有检查浏览器 storage/console、APM、生产代理或所有第三方日志。源码搜索也只能作为补充。

## 16. 最终门禁

以下必须在所有源码、四类文档、运行手册和全局索引收口后统一执行：

```bash
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go run ./cmd/doccheck

cd web
pnpm test
pnpm typecheck
pnpm build

cd ..
make verify
git diff --check
```

另做 shuffle/repeat：

```bash
go test -shuffle=on -count=10 ./internal/identity/...
go test -count=10 ./internal/platform/appconfig \
  ./cmd/growth-api ./cmd/growth-migrate \
  ./cmd/growth-identity-provision ./cmd/growth-identity-maintenance
```

收口过程中已经取得 Go 全量（23.2 秒）、race（25.8 秒）、vet、fmt-check 和 Web 23 files / 250 tests / typecheck / build 证据；HEAD `9fc4e06` 的完整 Compose gate 也已 exit 0。2026-09-03 又对 appconfig 与 `growth-api`、`growth-migrate`、`growth-identity-provision`、`growth-identity-maintenance` 五个包执行 `count=10` 并全部通过。完整代码、文档和索引工作树随后再次执行 `make verify`、`go test -race -count=1 ./...`、doccheck、acceptance script 的 `sh`/`dash` 语法与 shellcheck、Compose 配置渲染和 `git diff --check`，均 exit 0；冻结提交只固化这份已通过内容，稳定 ref 再承担最终 tip 定位。

## 17. 证据与清理记录模板

每次真实运行至少记录：

```text
date/timezone:
branch + commit/worktree fingerprint:
tool versions:
exact command (secret values removed):
disposable compose project / random loopback port:
expected:
actual exit/status/count:
skipped tests:
secret scan result:
cleanup targets resolved before removal:
cleanup result / long-lived project unchanged:
remaining PENDING:
```

清理只针对本次创建的 container、network、volume、password snapshot、cookie jar、coverage/profile、Vite `dist` 等明确目标。caller 原始 password file 默认由caller保留；只有像本次专用E2E文件一样，在创建时已明确授权为disposable fixture并重新解析精确路径后，才可覆写、unlink和删除其私有目录。不得删除长驻 `growthos` project、预存 Secret 或共享依赖。

## 18. 当前冻结检查单

- [x] Session HTTP/浏览器 transport 的 contract 已落盘；
- [x] strict transport 96 tests/typecheck/format 已有 `ACTUAL-PASS`；
- [x] 当前前端 23 files/250 tests/typecheck/build/format 已有 `ACTUAL-PASS`；
- [x] Argon2 Apple M2 Pro benchmark 已有本地 baseline；
- [x] 两轮 disposable provision Compose 已有 exact project/port/cleanup 记录；
- [x] 长驻 provision 暴露 one-shot `exited:0` 与 `compose --wait` 的语义错配，`af4245e` 已用 180s exact-state 轮询修复；
- [x] official disposable maintenance 已有 `2/1/3`→`0/0/0`、active fingerprint、residue、失败/grant 与 exact cleanup 证据；
- [x] 真实浏览器已完成 Nginx→Go→MySQL login/refresh/logout、MySQL outage fail-closed 与恢复重试；
- [x] 专用 E2E 的 2 Session、2 throttle、1 account 和私人 password file 已按边界清理；
- [x] Identity Go 普通/race/shuffle/count=10 与列出的九个 fuzz target 已取得本轮精确执行证据；
- [x] disposable MySQL 8.4.11 migration/Repository/runtime-grant acceptance 已在 HEAD `4149576` exit 0；
- [x] development HTTP 201→200→replacement→204→replay、Cookie/CSRF/Origin/Fetch、同形 401、五会话、MySQL 503/recovery 与 secret/fixture cleanup 已通过；
- [x] HEAD `9fc4e06` 已实跑 raw login/source 429、TE/Trailer、2049-byte body、逐状态 exact Cookie/clear-Cookie、安全 header 单值矩阵与 invalid-Host JSON 421；
- [x] 1719 × 862、390 × 844、1280 × 720，以及 keyboard/focus/aria/reduced-motion 和浏览器核心旅程已有设计 QA 实证；
- [ ] raw Content-Length absent/zero/mismatch 的完整 proxy wire 与 issue/revoke COMMIT outcome-unknown 真实 fault injection（后续环境证据，不阻塞本节 development DoD）；
- [ ] 浏览器 storage/console 全矩阵和更广设备/辅助技术验收（后续扩展矩阵）；
- [ ] staging/production TLS 与可信代理/client-IP 边界（生产验收范围）；
- [x] 最终全仓门禁、diff whitelist、远端稳定 ref 与累计学习分支冻结协议完成。

结论：第 32 节的核心 Go、独立 MySQL、完整 development Compose/Session wire、浏览器核心旅程和最终代码/文档门禁已形成可复核证据，development Definition of Done 已完成并冻结。raw Content-Length 特定变体、真实 commit-unknown fault、production TLS/可信代理与浏览器扩展矩阵仍为 `PENDING`，因此不能标记为生产就绪；第 33～35 节授权闭环也仍未实现。
