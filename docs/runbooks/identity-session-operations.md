# Identity Session 运维手册

- **适用章节：** 第 32 节“真实会话认证”
- **产品基线：** [GrowthOS Identity 与真实会话认证基线 v1](../product/identity-session-authentication-v1.md)
- **架构决策：** [ADR-0028](../decisions/ADR-0028-identity-session-authentication.md)
- **HTTP 契约：** [第 32 节 API 记录](../api/lessons/lesson-32.md)
- **配置总表：** [配置、日志与错误体系](../configuration.md)
- **记录日期：** 2026-09-03
- **证据状态：** 第 32 节 development DoD 已完成；provision、maintenance、浏览器核心旅程、development HTTP 增强 wire、独立 MySQL 8.4.11 与最终代码/文档门禁已有实际证据，剩余 raw Content-Length 变体、TLS/可信代理、真实 COMMIT outcome-unknown 故障注入与浏览器扩展矩阵仍为 `PENDING`

> 本手册用于本地/受控环境的账号 enrollment、Session 生命周期核查和有界历史清理。
> 示例从不把 password、Cookie、CSRF 或数据库 Secret 放进命令行、URL、环境变量、终端输出或版本库。
> 不要启用 `set -x`、curl verbose/trace 或保存完整 Nginx access log。所有“预期”都必须由本次真实运行证据确认，
> 不能因为源码或测试存在就写成已经通过。

## 1. 权限边界与职责

| 身份/进程 | 可以做什么 | 不能做什么 |
| --- | --- | --- |
| `growth-api` + `growthos_identity` | account 只读/锁读、Session 与 throttle DML、登录/current/logout | provision account、改 credential/status/epoch、DDL/GRANT、读业务表 |
| `growth-identity-provision` + `growthos_identity_provisioner` | 向 workforce account 表 `INSERT` 一行 | `SELECT`/readback、UPDATE、DELETE、upsert、Session/throttle/业务/migration 表 |
| `growth-identity-maintenance` + `growthos_identity` | 一次固定 Session/throttle 历史清理 | caller 自定时间/cutoff/batch、循环、重试、DDL/GRANT、账号清理 |
| `growth-migrate` + migrator | forward-only schema migration | 处理浏览器 credential 或充当 runtime |
| `mysql-grants` + root socket | 撤销 direct grant drift 并重授固定 allowlist | caller SQL、网络访问、账号/业务生命周期管理 |
| 浏览器/受控 curl | login、current、logout | 自报 Principal/Role/tenant、读取 HttpOnly bearer、调用 provision/maintenance API |

账号创建与 maintenance 都是 operations-only one-shot 进程，不是 HTTP route。`growthos_identity_provisioner`
的 INSERT-only 权限是有意设计：duplicate 与 COMMIT outcome unknown 都必须停止，由另一个经批准的读取身份核查，
不能为了“方便确认”给 provisioner 增加 `SELECT`。

## 2. 总体执行顺序与全局停止条件

建议按下列顺序执行，每一步保留脱敏摘要与退出码：

```text
preflight
  -> validate/generate local secrets
  -> Compose config + migrate + exact grants
  -> one-shot provision
  -> POST login -> GET current -> DELETE logout -> old-cookie replay
  -> fixed one-shot maintenance（仅在准备好受控 fixture 时）
  -> grant drift recheck
  -> explicit artifact cleanup
```

任一条件出现都必须停止，不得“先继续看看”：

- Secret 集合 partial、符号链接、权限/长度非法、key/password reuse，或已有 MySQL volume 但 Secret 全失；
- `docker compose config`、Migration、grant reconciliation、credential probe、readiness 任一失败；
- `@@GLOBAL.mandatory_roles` 非空，或任一实际 grant 与固定 allowlist 不一致；
- HTTP 状态/body/header/Cookie 与 [API contract](../api/lessons/lesson-32.md) 不一致；
- password、raw Cookie、CSRF、token/digest 或 Secret path/material 出现在输出、日志、截图、提交或报告中；
- provision、logout 或 maintenance 出现 COMMIT outcome unknown；
- maintenance 结果超过 Session 250、throttle 250 或总计 500；
- staging/production 未使用 HTTPS + `verify_identity` + Secure `__Host-` Cookie；
- 临时秘密文件无法验证所有权或无法精确清理。

停止后只保存低披露故障类别、request ID、时间、版本/commit、预期与实际状态；不要保存 credential。

## 3. Secret 基线

### 3.1 本地 Compose 的八个文件

默认目录是 `deploy/compose/secrets`，必须由调用者私有的 `0700` 父目录保护：

| 文件 | 格式 | 主要消费者 |
| --- | --- | --- |
| `mysql_root_password` | 64 个 lowercase hex，可有一个 LF/CRLF | MySQL init、grant job |
| `mysql_app_password` | 同上 | MySQL init、API business pool |
| `mysql_migration_password` | 同上 | MySQL init、migrator |
| `mysql_identity_password` | 同上 | MySQL init、grant job、API Identity pool、maintenance |
| `mysql_identity_provisioner_password` | 同上 | MySQL init、grant job、provisioner |
| `redis_password` | 同上 | Redis、API cache client |
| `identity_throttle_hmac_key` | 精确 32 个 raw nonzero bytes | API login throttle digest |
| `identity_csrf_active_key` | 精确 32 个 raw nonzero bytes | API CSRF active signer/verifier |

生成器强制 throttle 与 CSRF key 不同，也强制 provisioner MySQL password 与 root/app/migration/identity/Redis
password 全部不同。Compose bind-secret 为兼容 Docker Desktop 非 root 容器会把文件改成 `0444`；真正的主机访问边界是
`0700` 父目录，容器可见范围由逐服务只读挂载限制。这不是生产 Secret Manager，也不支持热轮换。

### 3.2 可接受状态与生成

```sh
make compose-secrets
make compose-config
```

生成器只接受以下精确文件计数/历史形态：

- `0`：生成完整八件套；但若 `${COMPOSE_PROJECT}_mysql_data` 已存在则拒绝，避免新密码与旧 volume 账号失配；
- `4`：验证旧 root/app/migration/Redis 后补 Identity runtime、provisioner 与两把 protocol key；
- `5`：验证含 Identity runtime password 的过渡集合后补 provisioner 与两把 key；
- `7`：验证已有 Lesson 32 runtime 集合后只补 provisioner password；
- `8`：只验证并复用，绝不覆盖。

其他 partial 集合全部拒绝。不要删除一个文件后再次运行来“轮换”，也不要在不知道 volume 所有权时设置跳过 volume 检查。

### 3.3 服务级最小挂载

| Compose service | 可见 Secret |
| --- | --- |
| `mysql` | 五个 MySQL password（root/app/migration/identity/provisioner） |
| `migrate` | `mysql_migration_password` |
| `mysql-grants` | root、identity、identity-provisioner password |
| `identity-provision` | 仅 identity-provisioner password；enrollment password 是单次额外挂载快照 |
| `identity-maintenance` | 仅 identity runtime password |
| `api` | app、identity、Redis password；throttle 与 active CSRF key |
| `redis` | Redis password |
| `web` | 无 Secret |

若 `docker compose config` 显示跨边界挂载，先修复装配，不得启动。

## 4. Compose 与数据库 preflight

默认值：

```text
COMPOSE_PROJECT=growthos
COMPOSE_FILE=deploy/compose/compose.yaml
GROWTHOS_COMPOSE_WEB_PORT=8088
development public origin=http://127.0.0.1:8088
MySQL=8.4.11
latest migration=14
```

标准入口：

```sh
make compose-config
make compose-up
make compose-ps
make compose-status
make compose-smoke
```

`compose-up` 的主链是 MySQL → migrate → `mysql-grants` → API；operations profile 下的 provision/maintenance
不会常驻启动。Migration 12、13、14 分别建立 workforce account、Session 与 authentication throttle 表。
`/health` 只证明进程存活；`/ready` 同时依赖 business 与 Identity pool。Identity MySQL 不可用时预期
`/ready=503`、Session API fail closed 为 503，而不是 mock user、Header Principal、Redis Session 或 anonymous fallback。

截至本文日期，两个 provision disposable project、official maintenance fixture、真实浏览器核心旅程和 official
development HTTP 增强门禁已经分别执行；第 14 节记录其精确边界。增强门禁已覆盖 chunked/Trailer、普通 2049-byte
body、逐状态 canonical security header、Cookie 签发/清除和持久 throttle，但不能替代 raw Content-Length
缺失/零值/不匹配变体、staging/production TLS、可信代理来源或 COMMIT outcome-unknown fault injection。后续每次执行仍必须
把 image/commit、Compose project、Migration status、service health、grant allow/deny 与清理结果写入验收记录，不能只记
“容器启动成功”。

长驻运行曾暴露 one-shot 判定错误：`docker compose up --wait` 会把已经快速成功的 `mysql-grants`
`exited:0` 当作失败。提交 `af4245e` 将 provision/maintenance wrapper 改为最长 180 秒的 exact-state 轮询：
只接受唯一目标容器的 `exited:0`，对非零退出、歧义、意外状态、inspect 失败和超时一律停止；
`created:0`、`running:0`、`restarting:0` 仅继续等待。不要用长期 service 的 healthy 语义判断 one-shot 成功。

## 5. 一次性账号 provision

### 5.1 输入准备

由受控 password manager 或不回显秘密的工具创建 caller-owned enrollment 文件。文件必须：

- 是 invoking user 拥有的可读 regular file，不是 symlink；
- mode 精确 `0600`，hard-link count 精确为 1；
- transport 为 `1..514` bytes；binary 只去掉至多一个尾部 LF 或 CRLF；
- 去掉 transport newline 后为有效密码：`12..128` Unicode code point、最多 `512` UTF-8 bytes；
- 不在 repository、共享目录、shell variable、process argument 或环境变量中保存 password bytes。

AccountID/PrincipalID 使用第 31 节 canonical identifier；login name 精确匹配
`[a-z][a-z0-9._-]{2,63}`。操作者可以通过参数选择三种 ID，但不能选择 status、credential/epoch version、时间、
Argon2 envelope、Role、Scope、tenant 或 Permission。

### 5.2 执行

```sh
make compose-identity-provision \
  IDENTITY_ACCOUNT_ID=learning-operator-01 \
  IDENTITY_LOGIN_NAME=learning.operator \
  IDENTITY_PRINCIPAL_ID=learning-operator-01 \
  IDENTITY_PASSWORD_FILE=/absolute/private/path/enrollment-password
```

底层唯一命令 grammar 是：

```text
growth-identity-provision create \
  --account-id VALUE \
  --login-name VALUE \
  --principal-id VALUE \
  --password-file PATH
```

四个 flag 可换序，但必须各出现一次且只能使用 separate-value form。Make target 对命令行变量中的 dollar 做额外拒绝；
确有特殊 pathname 时可直接调用已审计 wrapper，但 password bytes 仍不能出现在参数中。

wrapper 会：

1. 验证 Compose 文件、Secret generator、project name 与 enrollment source；
2. 生成/校验 Secret，构建 provision image，并等候 MySQL/grants；
3. 在 `0700` 临时目录中复制一个 `0444`、最多 514 bytes 的只读快照；
4. 用 `cmp` 确认快照与刚复核的 source 一致；
5. 只把快照挂到 operations container；
6. 退出时先把快照改回 `0600`，按已知长度覆零、unlink，再 rmdir 临时目录。

覆零不能保证 copy-on-write/SSD 上的物理擦除；高敏感环境应使用加密 ephemeral storage，并以销毁密钥作为清理边界。
wrapper 不删除 caller 原始 enrollment 文件，操作者必须在验收后单独处理。

### 5.3 数据库效果与退出语义

成功只 `INSERT` 一条：enabled account、credential version `1`、authentication epoch `1`、
当前 Argon2id enrollment envelope 与可信 server timestamp。不会 readback、update、replace、upsert、delete 或绑定权限。

| exit | 含义 | 下一步 |
| ---: | --- | --- |
| `0` | create 与 pool close 均确认成功 | 进入 Session HTTP 验收 |
| `1` | validation、hash、DB、duplicate、cancel、unknown outcome 或 cleanup failure | 根据低披露类别停止并核查 |
| `2` | CLI grammar/Make 必需参数缺失 | 修正调用，不触碰数据库 |

duplicate 不是幂等成功。COMMIT outcome unknown 也不是“失败已回滚”：可能已经插入。不得盲目重跑同一命令，
也不得给 provisioner 添加 SELECT。由单独授权的 runtime/DBA 读取渠道按 AccountID、login、Principal 三个唯一事实核查后，
再决定保留、新建不同身份或执行另一个明确批准的修复流程。

当前没有账号删除、密码修改、disabled/epoch 管理命令。需要临时账号时，应优先使用可整体销毁的独立 Compose project/volume；
否则把它作为明确记录的学习账号保留。不要假装 provisioner 可以“回滚清理账号”。

provision success 已在两个 disposable Compose project 实际通过并精确清理；duplicate 与 COMMIT outcome-unknown
停止/核查流程仍缺少对应的真实故障注入证据，不得由成功路径外推。

## 6. 私密 HTTP 验收工作区

下面流程避免把 bearer/CSRF 写到屏幕或 shell argument。创建隔离目录后，先把实际绝对路径记在本地受控记录中：

```sh
umask 077
evidence_dir=$(mktemp -d "${TMPDIR:-/tmp}/growthos-session-evidence.XXXXXX")
chmod 0700 "$evidence_dir"
login_request_file="$evidence_dir/login-request.json"
login_response_file="$evidence_dir/login-response.json"
current_response_file="$evidence_dir/current-response.json"
logout_response_file="$evidence_dir/logout-response.bin"
replay_response_file="$evidence_dir/replay-response.json"
cookie_jar="$evidence_dir/session.cookies"
replay_cookie_jar="$evidence_dir/session-before-logout.cookies"
csrf_config="$evidence_dir/csrf.curl-config"
public_origin=http://127.0.0.1:8088
```

不要把这个 snippet 放进启用 xtrace 的 shell。变量只保存路径与公开 origin，不保存 password/token/CSRF。
使用 password manager/受控程序直接生成 mode `0600` 的 exact JSON 到 `$login_request_file`：

```json
{"login_name":"learning.operator","password":"REPLACE_IN_PRIVATE_FILE_ONLY"}
```

上面的占位文本只展示结构，不能作为实际密码，也不能把真实文件提交。确认文件不含 extra field、BOM 或 trailing JSON；
请求体可以有普通尾部空白，但为减少取证歧义建议使用单一 object。不要通过 `echo`、history、environment 或命令参数填密码。

## 7. Login → current → logout → replay

### 7.1 Login

```sh
login_status=$(curl --silent --show-error \
  --request POST \
  --header 'Accept: application/json' \
  --header 'Content-Type: application/json' \
  --header "Origin: $public_origin" \
  --cookie-jar "$cookie_jar" \
  --data-binary "@$login_request_file" \
  --output "$login_response_file" \
  --write-out '%{http_code}' \
  "$public_origin/api/v1/session")
test "$login_status" = 201
chmod 0600 "$cookie_jar" "$login_response_file"
```

不要加 `-v`、`--trace`、`-i` 或打印 response。预期是 `201`、`Cache-Control: no-store`、一个 development
`growthos_dev_session` HttpOnly/Strict/Path `/` Cookie 与 exact Session JSON。实际 header 证据应使用能对
`Set-Cookie` value 自动遮盖的采集器；不能把完整 header dump 留存。

POST unknown/wrong/disabled 的同形 `401 authentication_failed`、CSRF/Origin/Fetch 拒绝、MySQL unavailable 503、
持久 login/source throttle 的 raw 429 与所有非 Cookie-changing error 的零 Set-Cookie，均已在 HEAD `9fc4e06` 的
official disposable HTTP 增强门禁通过。invalid/replaced/logged-out/expired/epoch/disabled 401 也已逐类证明 exact
clear-Cookie；每个 Session 响应的 canonical security header 均为单值。此前 handler 与 edge 重复输出不同
`Referrer-Policy` 的缺陷已由 `8fc0302` 修复并在本轮复验关闭，不再是当前风险。仍未覆盖的是 raw Content-Length
缺失、零值和 declared/actual 不匹配等变体，不得把已覆盖的 framing 样例外推为所有 HTTP parser 行为。

### 7.2 Current 与 CSRF 私密提取

在 logout 前复制旧 bearer，用于后续 replay：

```sh
cp "$cookie_jar" "$replay_cookie_jar"
chmod 0600 "$replay_cookie_jar"

current_status=$(curl --silent --show-error \
  --request GET \
  --header 'Accept: application/json' \
  --cookie "$cookie_jar" \
  --output "$current_response_file" \
  --write-out '%{http_code}' \
  "$public_origin/api/v1/session")
test "$current_status" = 200
chmod 0600 "$current_response_file"
```

在不输出值的前提下验证并写入 curl 私密 config：

```sh
jq -e '
  type == "object" and (keys == ["data"]) and
  (.data | type == "object") and
  (.data | keys == ["absolute_expires_at","authenticated","csrf_token","idle_expires_at","principal"]) and
  (.data.authenticated == true) and
  (.data.principal | type == "object" and keys == ["id","kind"]) and
  (.data.principal.kind == "human") and
  (.data.csrf_token | type == "string" and length > 0)
' "$current_response_file" >/dev/null

jq -r '"header = \"X-CSRF-Token: \(.data.csrf_token)\""' \
  "$current_response_file" > "$csrf_config"
chmod 0600 "$csrf_config"
```

current 成功必须是 `200`，并返回最新的 session-bound CSRF。不要用 login response 中的旧副本代替此次 current 证据；
不要打印、复制到剪贴板或存入 localStorage/sessionStorage。

### 7.3 Logout

```sh
logout_status=$(curl --silent --show-error \
  --request DELETE \
  --header 'Accept: application/json' \
  --header "Origin: $public_origin" \
  --config "$csrf_config" \
  --cookie "$cookie_jar" \
  --cookie-jar "$cookie_jar" \
  --output "$logout_response_file" \
  --write-out '%{http_code}' \
  "$public_origin/api/v1/session")
test "$logout_status" = 204
test "$(wc -c < "$logout_response_file" | tr -d ' ')" = 0
```

预期只有 confirmed revoke 才得到 `204`，body 精确为零 bytes，并清 Cookie。curl 不发送 `Sec-Fetch-Site` 是允许的受控
non-browser 情形，但 exact Origin 与 CSRF 都不能省略；真实浏览器若发送该 header，必须为 `same-origin`。

若收到 `503 session_revocation_indeterminate`，服务端会清浏览器 Cookie，但数据库是否已撤销仍未知：停止、保留 request ID，
通过批准的 DB/incident 读取路径核查，不盲重放 DELETE。普通 `authentication_unavailable` 并不保证 Cookie 已清。

### 7.4 旧 Cookie replay

```sh
replay_status=$(curl --silent --show-error \
  --request GET \
  --header 'Accept: application/json' \
  --cookie "$replay_cookie_jar" \
  --output "$replay_response_file" \
  --write-out '%{http_code}' \
  "$public_origin/api/v1/session")
test "$replay_status" = 401
chmod 0600 "$replay_response_file"
```

预期是 `401 unauthenticated`，旧 Cookie 不能恢复 Principal。验收报告只记录状态、公开 error code、request ID 与
“old bearer rejected”，不附 cookie jar 或 CSRF config。HEAD `9fc4e06` 的 official disposable HTTP gate 已实际通过
201→200→replacement→bodyless 204→old-bearer 401，并对该 replay 响应验证 exact clear-Cookie 与 canonical
security header；raw Content-Length 变体和生产 TLS 仍是独立边界。

## 8. 浏览器验收

在隔离、可精确删除的 browser profile 中完成，至少核对：

1. 登录 request wire 是 `{login_name,password}`，状态 `201`，没有 redirect；
2. Application/Cookies 显示 development host-only、HttpOnly、Strict、Path `/`；页面 JavaScript 不能读 bearer；
3. reload 后 `GET /api/v1/session` 返回 `200`，UI 只持有 Principal/expiry/CSRF；
4. password、raw Cookie、CSRF 不出现在 URL、console、localStorage、sessionStorage、IndexedDB 或 error UI；
5. cross-origin、`same-site` sibling、missing/wrong/cross-session CSRF 均失败且零 revoke；
6. logout 返回 bodyless `204`，Cookie 被清，旧 cookie replay 为 `401`；
7. malformed/expired/revoked Cookie 为 `401` 并清理；Identity DB down 为 `503`，不得展示假登录；
8. 当前页面不能因 Principal 存在就展示/声称 Role、Permission 或 capability。

浏览器证据要遮盖 Cookie/CSRF，并记录 browser/version、origin、commit/image、request ID 与观察结论。
不得上传包含 bearer 的 HAR。development HTTP 结果不能替代 staging/production 的 HTTPS `__Host-` Cookie 验收。
截至本文日期，真实 development 浏览器已经通过登录、reload/current、logout、匿名刷新，以及 MySQL outage 时
unknown/unavailable 呈现和恢复后重核同一 Principal。该旅程没有直接读取 HttpOnly Cookie store，也没有完成
storage/console、全面设备/辅助技术或 production TLS 矩阵；这些仍为 `PENDING`。

## 9. 一次性 Identity maintenance

### 9.1 入口与配置边界

标准入口无参数：

```sh
make compose-identity-maintenance
```

底层 grammar 只有：

```text
growth-identity-maintenance run
```

任何额外 flag/argument 都是 usage error。命令使用现有 runtime `growthos_identity` credential，而不是 root、migrator、
business app 或 provisioner credential；只挂载 `mysql_identity_password`。进程只加载 environment/log、共同 MySQL
address/database/TLS/CA/connect/write、runtime Identity user/password，以及 maintenance 专用：

```text
GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_READ_TIMEOUT=5s
GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_PING_TIMEOUT=3s
GROWTHOS_IDENTITY_MAINTENANCE_OPERATION_TIMEOUT=3s
```

operation timeout 必须为 `1s..30s`，且必须为 read 与 write network cancellation/cleanup 各预留至少 `1s`；
默认 `3s + 1s <= 5s`。配置不开放 max-open/max-idle/lifetime 参数，composition 固定一个 pool、
`MaxOpenConnections=1`、`MaxIdleConnections=1`。staging/production 仍强制 MySQL `verify_identity` TLS；可选 CA
只能扩展验证根，不能关闭证书/主机名验证。

maintenance loader 故意忽略 HTTP、Lottery、Redis、Argon2、CSRF/throttle key、provisioner、migrator、business app
credential 与其 pool 变量，避免无关配置或更高权限身份渗入 one-shot 进程。

### 9.2 固定清理语义

单次进程只允许一个 `Run`、一个可信进程时钟观察、一个 operation deadline、无循环且无自动重试：

| 阶段 | eligibility | 固定预算 | 事务顺序 |
| --- | --- | ---: | ---: |
| Session history | `absolute_expires_at <= observed_at-7d` 或 non-null `revoked_at <= observed_at-7d` | 250 | 1 |
| inactive throttle | `row_expires_at <= observed_at` 且 `inflight_count=0` 且 `inflight_expires_at IS NULL` | 250 | 2 |

candidate 按 cleanup time 与稳定 identity 排序，DELETE 时再次带 eligibility predicate；Session 的 expired/revoked candidate
会合并去重并截断到 250。两个预算不能互借，总数最多 500。Session transaction 总是先执行；它失败或 COMMIT unknown
时不会进入 throttle。两个事务独立，因此 Session 可能已提交而 throttle 随后失败，这不是全局原子操作。

成功日志只输出 `sessions_deleted`、`throttles_deleted`、`total_deleted` 等稳定计数；`0/0/0` 也是成功。
失败日志不得含 SQL、host、账号、digest 或 Secret；pool close 失败会把原本成功降为失败。

### 9.3 重复执行与不确定结果

- 同一个 runtime 实例第二次 Run 必须拒绝；wrapper 每次创建一个新进程；
- 任何失败都没有内部 retry；commit-unknown 优先于并发 cancellation 分类；
- outcome unknown 时不要因为 DELETE 看似幂等就盲目重跑：先确认哪一个 transaction 可能提交，核查 counts/eligible rows，
  再由操作者明确批准一个全新的 one-shot run；
- 若一次删满 Session 250 或 throttle 250，只说明仍可能有 backlog；确认结果后才能启动下一次，不能改 batch 或循环跑；
- maintenance 不删除 account/credential，不修改 active Session/throttle，不回收 provision duplicate。

official disposable maintenance fixture 已实际证明第一次删除 `2/1/3`、第二次精确 `0/0/0`、active Session
fingerprint 不变、fixture residue `0:0:0` 与精确清理。250+250 满预算、partial success 和 COMMIT outcome-unknown
真实故障注入仍为 `PENDING`。

## 10. Grant drift 检查与收敛

### 10.1 固定 allowlist

`growthos_app`：

```text
USAGE
SELECT growthos.lottery_strategy
SELECT growthos.lottery_strategy_award
```

`growthos_identity`：

```text
USAGE
SELECT, UPDATE(updated_at) growthos.identity_workforce_account
SELECT, INSERT, UPDATE, DELETE growthos.identity_session
SELECT, INSERT, UPDATE, DELETE growthos.identity_authentication_throttle
```

`growthos_identity_provisioner`：

```text
USAGE
INSERT growthos.identity_workforce_account
```

必须同时满足 `@@GLOBAL.mandatory_roles` 为空。Identity runtime 的 account 列级 UPDATE 只允许
`updated_at`，不能改 login、credential、status 或 epoch；provisioner 不能 SELECT/readback。

### 10.2 收敛入口

```sh
make compose-grants
make compose-smoke
```

grant job 通过只读 Unix socket、`network_mode: none` 运行。它对三个 runtime/one-shot account 先撤销所有 direct grants，
再重授上面的 exact allowlist，随后 exact `SHOW GRANTS` 比较并以 mounted identity/provisioner credential 做固定 `SELECT 1`
认证探针。它不读取 caller SQL，不给 wildcard schema 权限，也不更改已经存在账号的 password。

若 Secret 与旧 volume 中账号 password 不匹配，`CREATE USER IF NOT EXISTS` 不会修复它；job 必须失败。
不要通过给 runtime root/migrator/`ALL PRIVILEGES`、给 provisioner SELECT 或设置 mandatory role 来绕过。
进入第 11 节批准的 credential rotation/recovery。

## 11. Rotation 与事故恢复

### 11.1 MySQL credential

当前 Compose generator 只生成/验证初始 Secret，grant job也不会 `ALTER USER`。因此仅替换文件会造成 retained volume
credential drift。标准原则是：

1. 在 Secret Manager/私密文件中生成新值并验证格式、所有权与差异，不输出；
2. 通过单独批准的 DBA 连接更新 exact MySQL account；不要把 password 写进 shell history、日志或本手册；
3. 原子发布匹配的 `_FILE` Secret，并只 recreate 对应消费者；
4. 跑 credential probe、exact grants、readiness 与目标业务负向矩阵；
5. 确认旧 credential 已失效，再销毁旧材料并记录 cleanup。

运行 Identity password rotation 会同时影响 API Identity pool 与 maintenance；provisioner password 只影响 grant job/provisioner，
但 MySQL init 也要在灾备重建时得到匹配 Secret。仓库没有自动双账号无中断轮换或通用 DBA 命令；需要这类能力必须另立小节，
不得在事故中临时扩大本手册。

### 11.2 CSRF active/previous key

API config 支持一个 active tuple 与至多一个 previous tuple：

```text
GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_ID
GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY(_FILE)
GROWTHOS_IDENTITY_CSRF_PREVIOUS_KEY_ID
GROWTHOS_IDENTITY_CSRF_PREVIOUS_KEY(_FILE)
GROWTHOS_IDENTITY_CSRF_PREVIOUS_ACCEPT_UNTIL
```

active/previous key ID 必须不同，key bytes 必须不同，且都不得复用 throttle HMAC key；previous accept-until 必须是未来的
canonical RFC3339 instant，并且从配置时刻起不超过 8 小时。轮换顺序是先把旧 active 作为 previous、发布新 active、
recreate API、验证新签发与旧 token 临时验证，再在窗口到期后删除 previous tuple 并再次 recreate/验证。

本地 Compose 当前只装配 active key，没有自动 previous-key 文件/窗口编排；生产轮换自动化为 `PENDING`。
key 遗失或来源不确定时，unsafe mutation 必须失败关闭。不要用 throttle key 临时替代。

### 11.3 throttle HMAC、Session bearer 与账号 epoch

throttle HMAC rotation 会改变 login/source subject digest，可能暂时形成新旧两组 limiter state；当前没有双 key迁移协议，
必须经独立安全决策、维护窗口和监控后执行。Session bearer 没有一把可“轮换”的共享加密 key：raw bearer 随机生成，
MySQL 存 digest；单 Session 通过 logout/revoke 失效，全账号撤权应通过 authentication epoch 或批量 revoke。

当前公开 HTTP 与 operations CLI 都没有 logout-all、epoch bump 或 account-disable 命令。事故需要这些动作时走另一个已批准的
DBA/application 运维路径；不要发明未实现命令或直接手改不变量。Redis restart/eviction 不会撤销 Session，因为 Redis 不是 authority。

## 12. 故障诊断矩阵

| 观察 | 可信解释 | 运维动作 |
| --- | --- | --- |
| POST `400 invalid_request` | request shape/login/Cookie 不合法 | 修正非秘密输入；不要猜测账号状态 |
| POST `401 authentication_failed` | unknown/wrong/disabled/stale 之一 | 对外不区分；走账号所有者/安全流程 |
| GET/DELETE `401 unauthenticated` | missing/invalid/inactive Session | 预期清 Cookie；重新登录，不恢复旧 Principal |
| unsafe `403 request_origin_rejected` | Origin/Fetch/CSRF 任一拒绝 | 检查 exact origin 与当前 CSRF；不降低 guard |
| POST `415 unsupported_media_type` | 非 exact JSON media type | 只发送单个 `application/json` |
| POST `429 authentication_throttled` | persistent login/source budget 拒绝 | 等待有界 backoff；不绕过 throttle/Argon gate |
| JSON `503 authentication_unavailable` | Identity/entropy/capacity/timeout/unknown | 看 readiness 与 stable logs；POST 不自动重试 |
| JSON `503 session_revocation_indeterminate` | logout commit 结果未知 | client cookie 已清；批准读取核查，不盲重试 |
| JSON `502/504` | 本仓库 Nginx edge 无可用上游或上游超时 | 查 Nginx/API health；这是 edge 自己的 canonical JSON，不伪造 Go 错误 |
| HTML/text `5xx` | 仓库外层 CDN/LB/proxy 或非本仓库入口返回了非 JSON | 前端归 `gateway`，按实际外层基础设施排查；不能归因于当前 Nginx 契约 |
| `/health=200`、`/ready=503` | 进程活着但 required dependency 不 ready | 修复 Identity/business DB，不做 auth fallback |
| 浏览器 login source 都相同 | 当前 guard 只信 socket peer，Nginx 后会聚合 | 不信 `X-Forwarded-For`；生产 proxy trust 另立设计 |
| provision duplicate | exact unique identity 已存在 | 另授权读取核查；不把 duplicate 当成功 |
| maintenance Session 有删、整体失败 | 第二个 throttle transaction 可能失败 | 保留计数/分类；确认后决定新 run |
| grant job credential probe 失败 | Secret 与 retained volume 或账号不匹配 | 停止并走批准的 credential rotation |

所有后端失败日志只能用 stable component/operation/result class/request ID/count。若错误文本含 host、account、SQL、Secret path
或敏感 bytes，先隔离日志访问并作为泄密事故处理，而不是把日志贴进 issue。

## 13. 精确清理

### 13.1 HTTP/enrollment 临时文件

清理前先逐一解析并确认目标位于本次创建的 `0700` evidence directory，且不是 symlink、不是用户预存文件。
本流程需要清理：

- caller-owned enrollment password source（仅在所有者明确批准后）；
- `login-request.json`、login/current/logout/replay response；
- Session cookie jar、pre-logout replay cookie jar；
- 私密 curl CSRF config；
- 本次隔离 browser profile/HAR（若包含 bearer，不得作为交付附件）。

对每个已确认的具体文件使用显式 `unlink`，最后只对已确认为空的本次目录使用 `rmdir`；不要把 `$TMPDIR`、workspace root、
home、glob 或未解析变量作为 recursive delete 目标。示例顺序：

```sh
unlink "$login_request_file"
unlink "$login_response_file"
unlink "$current_response_file"
unlink "$logout_response_file"
unlink "$replay_response_file"
unlink "$cookie_jar"
unlink "$replay_cookie_jar"
unlink "$csrf_config"
rmdir "$evidence_dir"
```

任何目标意外变成 directory/symlink、目录不为空或 unlink 失败时停止，保留 exact path 供本机检查，不扩大删除范围。
账号不是临时文件，provisioner/maintenance 都不会清理它。

### 13.2 Compose 生命周期

```sh
make compose-down
```

这会停止项目并保留 named volumes。只有数据所有者明确授权丢弃该 exact Compose project 的 MySQL/Redis/socket volume 时才允许：

```sh
make compose-reset CONFIRM=reset-growthos-data
```

reset 是破坏性数据操作，不是 Session logout 或普通验收清理手段。禁止全局 Docker prune、模糊 project name 或手工删除未知 volume。
构建缓存、依赖和交付文档属于可复用资产，不因本手册执行而删除；只清理本次生成的临时证据与明确授权的隔离环境。

## 14. 证据与交接模板

第 32 节 development Definition of Done 的整体状态为 `COMPLETE`。下表的 `PARTIAL-ACTUAL` / `PENDING` 是额外运维演练或跨环境补证，不是章节完成阻塞项。

每项只记录脱敏事实，不粘贴 secret-bearing 原始输出：

| Gate | 当前状态 | 完成时必须记录 |
| --- | --- | --- |
| eight-secret validate/upgrade 与 mount matrix | `PARTIAL-ACTUAL` | official HTTP project 已验证本轮 fresh/8 Secret、最小挂载与精确清理；state 4/5/7 升级和生产 Secret Manager/轮换另验 |
| Migration 12～14 clean/second-up/dirty stop | `ACTUAL-PASS（独立 MySQL）` | MySQL 8.4.11、latest 14、second-up/dirty/inventory、隔离环境与清理已实测 |
| exact grants 与反向 deny | `ACTUAL-PASS（disposable）` | 独立 MySQL runtime 正反向和 official Compose 三身份 allowlist 已实测；生产 host/credential rotation 另验 |
| provision success/duplicate/unknown stop | `PARTIAL-ACTUAL` | success 两轮通过；duplicate/unknown 真实注入仍待完成 |
| HTTP login/current/logout/replay | `ACTUAL-PASS（development 增强门禁）` | 201/200/replacement/bodyless 204/old-bearer 401、exact Cookie、request ID、no-store 与 canonical security header 已实测 |
| HTTP negative/gateway/readiness | `ACTUAL-PASS（已定义 development wire 矩阵）` | 400/401/403/415/421/429/502/503/504、chunked/Trailer/2049-byte body、无 Cookie 与清 Cookie、MySQL/Redis outage/recovery 已实测；raw Content-Length 变体和真实 commit-unknown 另验 |
| browser Cookie/storage/CSRF/reload/logout | `PARTIAL-ACTUAL` | development 核心旅程已实测；未直接读取 HttpOnly store，storage/console/全面设备与 AT 仍待完成 |
| staging HTTPS `__Host-` | `PENDING` | TLS host、Secure/host-only/Strict/Path 属性 |
| maintenance 0/250/250/500/partial/unknown | `PARTIAL-ACTUAL` | `2/1/3`→`0/0/0`、active fingerprint 与 cleanup 已实测；满预算/partial/unknown 仍待完成 |
| rotation drills | `PENDING` | old/new overlap、consumer recreate、old invalid、material cleanup |
| disposable artifact cleanup | `ACTUAL-PASS（已记录核心、增强与 MySQL 门禁）` | exact project/name/label/path、增强门禁六类外部 residue 为零且长期资源 identity 不变/healthy 已实测；后续每轮仍独立核对 |

### 14.1 已执行证据的精确边界

- 认证 baseline `5af29e2` 修复了 WorkGate 在可用 slot 与 1ms timer 同时 ready 时随机误报 unavailable 的准入时序缺陷：
  context 预检查后先走 nonblocking available fast-path，只在满槽时启动 timer；对应 `(2,1,false)` seed、count=10、race
  与 10 秒 fuzz 625,627 次执行已通过。这不是 credential 绕过，也不应把执行次数外推成吞吐能力。
- 历史核心 HTTP 门禁运行时工作树 HEAD 为 `8a5e0ce`、认证代码 baseline 为 `5af29e2`，project
  `growthosl24d2103fd496568ceac960d315`，302 秒、exit 0；覆盖 201→200→replacement→bodyless 204→old-bearer 401、
  development Cookie jar/header、CSRF/Origin/Fetch、同形 401、五会话上限与 MySQL 503/recovery。终态只删除本轮
  Session/throttle/account `10:3:1`，三表 residue `0:0:0`；password/raw Session/CSRF marker 扫描零命中，
  disposable 资源和私有文件清零，长期 `growthos` 快照不变。该次是含后续工作树验收改动的历史 run，不能当作
  `8a5e0ce` committed tree 的可复现证明；这里只保留其 core PASS 边界与受限 provenance。
- 增强脚本的第一个 committed-tree 候选是 `903fd9f`，project `growthosl24c1bf7ce29e5efa417fae6932`。
  构建、Compose prerequisites、maintenance fixture 等 Session 前置门禁已经通过，但脚本在进入 Session wire 断言前，
  因 macOS BSD awk 把循环变量 `index` 解释为内建名而解析失败，exit 2；因此该轮必须记为 `ACTUAL-FAIL`，不能从前置
  成功推导 Session 结果。没有可信总时长；退出 trap 精确清除了本 project 的 containers/networks/volumes、六个
  acceptance images、builder 与私有临时目录。
- `51b52e0` 修复了 BSD awk 变量、invalid-Host Session 的 canonical JSON 421，以及 development Cookie 签发/删除
  tuple 的严格断言；但随后 project `growthosl240da11b08420700da0d07428f` 只完成第一轮 backend image build，第二次
  相同 backend build 在获取 Docker Hub OAuth token 时收到 EOF，尚未进入 Session gate。该轮也不是协议 PASS；退出后
  containers/volumes/networks/images/builder/tempdirs 外部 residue 均为 0。
- `9fc4e06` 将 `api`、`migrate`、`identity-provision`、`identity-maintenance` 四个 image target 放进同一 BuildKit/Bake
  invocation：共同 Go builder 只执行一次，随后各 target 复用缓存；Redis 与 Web 仍顺序构建。这里是四个镜像 target，
  不是第 32 节新增四个身份运行入口：身份链只有既有 `growth-api` 与两个 operations one-shot 共三个入口，
  `growth-migrate` 是复用的既有迁移命令。
- 当前可冻结的增强 run 来自 exact HEAD `9fc4e06`，project `growthosl24f6a5acf4d242695ad3e2df19`，exit 0；本轮没有
  可复核的总时长，因此不填写猜测值。它实际通过 Session lifecycle/五 active-session 上限、所有定义状态的单值
  canonical security header、invalid Host correlated JSON 421、chunked 与 Trailer 拒绝、普通 2049-byte body 到 Go
  后的 400、malformed/query/media/auth/origin/fetch 的零 Set-Cookie、invalid/replaced/logged-out/expired/epoch/disabled
  401 的 exact clear-Cookie，以及 login/source 两维持久 throttle raw 429。它也通过 MySQL outage/recovery、Redis
  outage/recovery、edge JSON 502/504、Secret marker 与全部既有 Lottery/cache/maintenance 门禁。
- 增强 run 清理前 account/session/throttle 状态为 `disabled:2:10:31`（account status、authentication epoch、Session
  count、throttle count），随后只删除本轮 Session/throttle/account `10:31:1`，三表 residue `0:0:0`。退出后的
  project containers/volumes/networks、acceptance images、builder 与任务临时目录六类 residue 均为 0；长期
  `growthos` resource identity 前后不变且保持 healthy。
- 独立 MySQL 门禁从 HEAD `4149576` 启动，随机容器
  `growthos-lesson32-mysql-e4e83e6c1b0e7036f42e65f9`、label
  `com.growthos.acceptance.lesson32=run-e4e83e6c1b0e7036f42e65f9`，MySQL 8.4.11，19 秒、exit 0；schema、
  migration immutability/inventory、真实 migrator、Repository 与 runtime grant 均通过，终态 `14:0`、三表
  `0:0:0`、reserved probe 0。任务 name/label/temp/Secret 精确清理，长期 `growthos` resources 前后不变。

增强 run 关闭了 raw 429、已定义 framing、401 clear-Cookie、security-header 精确所有权/唯一值与仓库内 edge JSON
gateway 项；`Referrer-Policy` 重复输出是 `8fc0302` 以前的历史缺陷，不是当前残留。仍不能声称覆盖 raw
Content-Length 缺失/零值/declared-actual mismatch、issue/revoke COMMIT outcome-unknown 的真实故障、
staging/production TLS + 可信代理 client IP，或浏览器 storage/console、全面设备与辅助技术矩阵。浏览器证据也没有、
且不应通过 JavaScript 读取 HttpOnly bearer。

建议交接摘要：

```text
commit/image:
environment + exact public origin:
Compose project / isolated data owner:
provision result class:
HTTP status chain: 201 -> 200 -> 204 -> 401
negative matrix summary:
maintenance counts/result class:
grant allow/deny summary:
secret-leak scan summary:
PENDING items:
cleanup removed:
cleanup deliberately retained:
stop-line confirmation: no RBAC/capability/browser-authorization claim
```

## 15. 本章停止线

即使本手册所有 Session 项目通过，也只能宣称 credential → authoritative server-side Session → trusted human Principal。
仍不能宣称：

- 业务 API 已执行服务端 RBAC；
- Role/Scope/Permission 已通过 Session 返回；
- 不同人员已经只能看到/操作各自获准界面；
- direct API、跨角色、跨对象、跨 tenant 的越权矩阵已完成；
- production proxy client IP、TLS Cookie、轮换自动化、MFA/SSO 或合规认证已完成。

第 33 节负责服务端 enforcement，第 34 节负责 capability 驱动的前端裁剪，第 35 节负责越权与浏览器端到端闭环。
本节不能用 UI 隐藏代替服务端授权，也不能把 development HTTP、source unit test 或 Compose script 的存在当生产证据。

## 16. 来源分层

### 16.1 官方技术资料

以下资料用于解释操作边界，不是本仓库已执行证明：

1. [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)：enrollment hash 与 work factor 生命周期。
2. [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)：低披露认证与运维账号边界。
3. [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)：Session entropy、Cookie、expiry、revoke 与 fixation。
4. [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)：Origin、SameSite 与 HMAC CSRF token。
5. [OWASP Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)：Secret storage、rotation 与 least privilege。
6. [MySQL 8.4 Account Management Statements](https://dev.mysql.com/doc/refman/8.4/en/account-management-statements.html)：账号、grant/revoke 与 credential 管理参考。
7. [Docker Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/)：Compose secret mount 模型。

### 16.2 牛客/面试题型线索

面试训练可以围绕：为什么 provisioner 必须 INSERT-only、为什么 maintenance 复用 runtime identity 但固定 pool/预算、
为什么两事务不追求全局原子、为什么 COMMIT unknown 禁止盲重试、为什么 Secret 文件变化不等于数据库密码轮换、
为什么 logout 仍需要 CSRF、为什么 204 必须无 body、为什么 Redis 不能成为 Session authority。

这些题型只是表达训练，不是技术规范。已存在的[第 32 节精准面试问答](../interview/lessons/lesson-32.md)将官方技术资料
与牛客等社区题型线索分层记录；社区链接只用于确认题型存在，不作为规范性技术依据，也不伪造成企业官方题库或本项目实跑证据。
