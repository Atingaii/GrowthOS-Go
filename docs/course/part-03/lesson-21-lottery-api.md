# 第 21 节：开放第一个 Lottery API

> 本节第一次把第 17～20 节的领域对象、MySQL 聚合快照和无偏加权选择器接进真实 HTTP 链路。它开放的是 development/test 环境下的 **ephemeral selection（临时选择）**，不是已经持久化、可恢复重放、可发奖的正式 Draw。

## 1. 本节目标

完成本节后，仓库应能回答以下问题：

1. 一个没有请求体、每次调用都形成新选择观察的动作，为什么使用 `POST`；
2. 在没有 DrawID 和结果表时，接口名称怎样诚实表达“每次调用都会重新选择”；
3. `uint64` StrategyID/AwardID 怎样跨 JSON 和 JavaScript 边界无损传输；
4. `no_reward`、不存在、依赖暂态失败、数据损坏和程序缺陷怎样分开；
5. HTTP handler 怎样只负责协议，不直接写 SQL 或概率算法；
6. 请求 deadline 怎样沿 `context.Context` 传到 MySQL，同时承认同步随机选择不能被抢占；
7. 为什么开发演示接口必须默认关闭，并明确禁止在 staging/production 打开；
8. 为什么应用运行身份在本节从 `SELECT, INSERT` 收敛为两表 `SELECT`；
9. Nginx 和 Go 两层怎样共同约束 Host、请求体、超时、错误 JSON、缓存与 Request ID；
10. 怎样用业务数据与 Compose 资源隔离、可清理的一次性验收证明真实 `Nginx → Go → MySQL → CryptoSource` 链路，而不污染学习者长期数据库；它仍共享宿主 Docker daemon、CPU、网络与 registry，并非 hermetic sandbox。

## 2. 开始前的事实

第 20 节结束时已经存在：

- `domain.Strategy` / `domain.Award` 及完整领域不变量；
- MySQL 两表和 `mysqlrepo.Repository.FindByID` 的只读快照；
- `domain.WeightedSelector` 与 `randomsource.CryptoSource`；
- Gin 进程、统一错误、Request ID、MySQL readiness 和 Compose 同源入口。

但还不存在：

- Lottery application use case；
- Lottery HTTP route 和 DTO；
- composition root 中的 Repository/Selector 装配；
- 面向业务 API 的 timeout 配置；
- 真实 Lottery HTTP acceptance；
- Draw、Participation、资格、次数、幂等记录、结果查询、库存与权益发放。

这一区分很重要：拥有“读配置”和“选候选”的两个函数，不等于拥有一次完整抽奖。

## 3. 先决定接口的诚实边界

### 3.1 为什么不直接命名 `/draws`

在 GrowthOS 的长期模型中，Draw 至少应表示一个可以被再次查询、审计和恢复的最终业务事实。当前没有：

- DrawID；
- 用户/参与身份；
- 请求身份或唯一约束；
- Strategy 版本快照；
- 最终 Award 记录；
- 结果状态机；
- 响应丢失后的查询入口。

如果此时叫 `/draws`，调用者会自然推断服务器创建了一个持久资源；这个推断是错误的。因此本节使用：

```text
POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections
```

`ephemeral` 是契约的一部分：一次调用只在当前进程中读取 Strategy 并选出 Award，不写业务表；下一次调用是一次新的选择。

### 3.2 为什么是 POST

当前实现不写业务状态，因此不能仅凭“随机响应不同”就断言它在 RFC 9110 的严格定义下 unsafe 或 non-idempotent；HTTP safety/idempotency 讨论的是调用者请求造成的预期服务器状态效果，而不是响应值是否相同。这里仍选择 `POST`，因为产品把它建模为调用者显式发起的一次选择 action，不希望可链接的 `GET` 被预取、爬虫或通用缓存流程意外触发，也要与未来会持久化的正式 Draw 演进保持方向一致。

`PUT` 通常表示客户端知道目标资源身份并以幂等方式创建/替换它；本节没有客户端提供的 SelectionID，也没有结果资源。

`POST` 允许服务端对资源执行一次处理并返回处理结果。这里返回 `200 OK`，因为没有创建可定位的持久资源，不能诚实返回 `201 Created` 和 `Location`；也没有异步排队，不能返回 `202 Accepted`。

### 3.3 为什么显式拒绝 Idempotency-Key

没有持久结果，服务器无法把 key 与第一次 Award 绑定。如果静默接受：

```text
第一次：选择 Award A，响应丢失
重试：  选择 Award B，客户端以为 key 保证相同结果
```

这比明确报错更危险。因此只要请求出现 `Idempotency-Key`，即使值为空，也返回 `400 idempotency_not_supported`。真正的幂等将在拥有请求记录、唯一约束和结果查询以后实现。

## 4. 纵向切片架构

```text
Browser / curl
  │ POST + demo acknowledgement
  ▼
Nginx local edge
  │ Host / framing / 16 KiB / request ID / gateway timeout
  ▼
Gin Lottery adapter
  │ canonical ID / header / query / body / error DTO
  ▼
EphemeralSelectionService
  │ consumer-owned StrategyReader + AwardSelector ports
  ├──────────────► MySQL Repository.FindByID(ctx, id)
  │                  one read-only REPEATABLE READ snapshot
  ▼
WeightedSelector.Select(strategy)
  │
  └──────────────► CryptoSource.Uint64N(total)
  ▼
minimal response DTO
```

依赖方向仍然向内：

- application 定义自己需要的 `AwardSelector`；
- 既有 `StrategyReader` 由 application 拥有；
- domain selector 自然满足这个窄接口；
- MySQL 和 HTTP 都是 adapter；
- `cmd/growth-api` 是唯一装配点；
- handler 不知道 SQL、事务、随机字节或连接池。

## 5. application use case

[`ephemeral_selection.go`](../../../internal/lottery/application/ephemeral_selection.go) 引入：

```go
type AwardSelector interface {
    Select(strategy domain.Strategy) (domain.Award, error)
}

type EphemeralSelection struct {
    Strategy domain.Strategy
    Award    domain.Award
}
```

服务执行顺序固定为：

1. 拒绝 nil context、零 StrategyID 和未配置依赖；
2. 在任何依赖调用前检查 `ctx.Err()`；
3. 用同一个 context 读取 Strategy；
4. 依赖返回后再次检查 context，让已经观察到的取消/超时优先；
5. 确认 Repository 返回的 StrategyID 与请求一致；
6. 在 selector 前再次检查 context；
7. 选择 Award；
8. selector 返回后再次检查 context；
9. 从 Strategy 内按 ID 找回 Award，并逐字段确认它确实属于该快照；
10. 最后一次检查 context，返回临时结果。

这些防御不是因为当前两个生产 adapter 会故意撒谎，而是为了把 use case 的信任边界写成可执行契约：错误 Strategy、凭空 Award 或修改字段都不能越过 application 层。

### 5.1 context 的准确承诺

MySQL `FindByID` 接收 context，因此等待连接、查询和事务操作可以观察取消。同步 `AwardSelector.Select` 当前不接收 context，`crypto/rand.Int` 也不是可抢占的远程调用。

所以本节的 timeout 是 **cooperative deadline（协作式截止）**，不是能强制中断任意 Go 指令的硬实时上限。为限制不可取消区段，本节同时将每个 Strategy 的 Award 数量上限固定为 1000，Repository 读取 `LIMIT 1001` 并把超限快照判为损坏。

## 6. HTTP 请求契约

### 6.1 必需输入

| 输入 | 契约 |
| --- | --- |
| Method | 仅 `POST` |
| Path ID | 1～`18446744073709551615` 的规范十进制字符串 |
| Header | 恰好一个 `X-GrowthOS-Demo-Mode: ephemeral-selection` |
| Query | 不允许，包括只有 `?` 的 force-query |
| Body | 不允许非零/未知 Content-Length；进入 API location 的非空 Transfer-Encoding/Trailer 声明会在边缘拒绝，Go 仍拒绝它能观察到的 TransferEncoding/Trailer；显式 `Content-Length: 0` 与普通无 body 等价 |
| Idempotency-Key | 不支持，出现即拒绝 |

“规范十进制”意味着：

- `1` 合法；
- `01`、`+1`、` 1`、`0`、负数、指数写法和超过 uint64 的值非法；
- `strconv.ParseUint(raw, 10, 64)` 负责范围，`FormatUint(parsed, 10) == raw` 负责规范表示。

### 6.2 为什么 demo header 不是认证

这个自定义 header 有两个目的：

1. 要求调用者显式承认这是临时演示；
2. 它不是 CORS safelisted header，普通跨站 HTML form 不能无预检地附带它。

它没有用户身份、签名、权限或防重放能力，所以只能作为本地演示的 CSRF 摩擦和语义确认，不能写成认证或授权。

### 6.3 为什么无 body 仍检查 framing

只检查 `ContentLength == 0` 不够：HTTP/1.1 chunked 请求可能没有 Content-Length，Nginx 还可能在转发前把空 chunked 正规化成无 body。为了让入口契约一致：

- Nginx 在请求通过 HTTP parser 并进入 API location 时拒绝非空 `Transfer-Encoding` 与非空 `Trailer` 声明；无法被 parser 接受的 framing 可能更早以 Nginx 原生 400/501（可能是 HTML）结束；
- Go adapter 仍拒绝非零/未知长度，以及它能观察到的 TransferEncoding 和 Trailer；
- adapter 不读取“一个字节探测”，避免慢请求体把 handler 占用放到 selection deadline 之外。

## 7. 响应 DTO

### 7.1 reward 示例

```json
{
  "selection": {
    "durability": "ephemeral",
    "strategy_id": "21003",
    "award": {
      "id": "1",
      "name": "Reward",
      "outcome": "reward"
    }
  }
}
```

### 7.2 no_reward 示例

```json
{
  "selection": {
    "durability": "ephemeral",
    "strategy_id": "21002",
    "award": {
      "id": "1",
      "name": "Try again",
      "outcome": "no_reward"
    }
  }
}
```

`no_reward` 是 `200`：算法成功选中了配置中的合法业务结果。把它返回 404/204/500 会丢失“选择成功但没有奖励”的事实。

### 7.3 为什么 ID 是 string

JavaScript `number` 不能精确表示全部 uint64。DTO 使用十进制 string，使 `math.MaxUint64` 可以从路径进入、经过 Go/MySQL/领域层，再无损返回。Weight 没有进入 DTO，避免调用者把内部相对权重误当发布概率，也减少运营配置泄露。

### 7.4 为什么响应极小

接口不返回：

- operator-facing Strategy name；
- Award weight、total weight 或随机 ticket；
- Repository、算法或错误 cause；
- 虚构的 DrawID、BenefitID、inventory status；
- “最终”“已发放”或“可重试”等未成立承诺。

Transport DTO 由调用方契约拥有，不直接 JSON 序列化领域对象。

## 8. 错误映射

对非 `HEAD` 请求，所有进入 Go handler 后由 Go 产生的 API error 都使用既有 envelope，并包含单一 `X-Request-ID`、`Cache-Control: no-store` 与 JSON。真实 HTTP wire 会按 HEAD 语义抑制 body，因此 `HEAD` 精确 route 虽返回 405 和 `Allow: POST`，不能声称线上还能收到 JSON body：

```json
{
  "error": {
    "code": "lottery_strategy_not_found",
    "message": "lottery strategy not found",
    "request_id": "..."
  }
}
```

| 条件 | HTTP | code | 公开含义 |
| --- | ---: | --- | --- |
| ID/header/query/body/idempotency 非法 | 400 | 对应稳定 code | 调用契约错误 |
| Strategy 不存在 | 404 | `lottery_strategy_not_found` | 没有这个配置 |
| Method 不支持 | 405 + `Allow: POST` | `method_not_allowed` | 路径存在但方法错误 |
| deadline/cancel、可重试 Repository 或随机源失败 | 503 | `lottery_selection_unavailable` | 当前无法可信选择 |
| 存量快照损坏、组合错误、selector 违约、普通内部故障 | 500 | `internal_error` | 服务端不变量或依赖故障 |
| Nginx body 超限 | 413 | `request_too_large` | 边缘拒绝 |
| 非 allowlist Host | 421 | 无稳定业务 code | Nginx server-level 非 JSON 拒绝；本地入口不接受该 authority |
| upstream 不可达/超时 | 502/504 | `bad_gateway` / `gateway_timeout` | 网关没有得到可用响应 |

底层 SQL、driver message、熵错误和 error chain 不进入响应或普通日志。Lottery 模块失败日志只记录安全的 request ID、规范 StrategyID 和低基数 error class。

本节没有返回 `Retry-After`，因为服务没有可靠的恢复时间或退避建议。该 header 本来也只表示“多久后再发请求较合适”，不承诺同一结果；客户端还必须独立理解，当前任何重试都会形成一次新的临时选择。

## 9. 配置与启动失败关闭

新增配置：

| 变量 | 默认 | 约束 |
| --- | --- | --- |
| `GROWTHOS_LOTTERY_EPHEMERAL_SELECTION_ENABLED` | `false` | 仅 development/test 可为 true；staging/production 拒绝启动 |
| `GROWTHOS_LOTTERY_SELECTION_TIMEOUT` | `3s` | 正数且不超过 30s |

跨配置关系：

- `selection timeout + 1s <= HTTP write timeout`；
- `selection timeout + 1s <= MySQL read timeout`；
- Compose 使用 selection 3s、MySQL read 5s、HTTP write 10s；
- Nginx `proxy_read_timeout 11s` 是相邻读取的 inactivity timeout，不是整次请求硬总时长。

进程始终构造并验证 Repository、Selector 和 Service；只有 feature flag 为 true 时注册路由。缺失/typed-nil/零值 service 会在启动时失败，而不是 readiness 继续绿色、首个业务请求才报错。

## 10. 最小数据库权限

第 19 节为了验证 Repository `Create`，长期应用身份曾拥有两表 `SELECT, INSERT`。第 21 节生产 composition 只把 Repository 当 `StrategyReader`，HTTP use case 没有写路径。

因此 Compose reconciliation 把 `growthos_app` 收敛为：

```text
GRANT SELECT ON growthos.lottery_strategy
GRANT SELECT ON growthos.lottery_strategy_award
GRANT USAGE ON *.*
```

fixture 由隔离验收中的 migrator 身份写入。写 Repository 的集成测试使用显式隔离 schema 和独立确认变量，不能成为给线上 reader 预授 INSERT 的理由。

## 11. Nginx 本地边缘

第 21 节把 Nginx 从“静态页面 + 简单代理”加固成可验证的本地 API 边缘：

- 只允许 `localhost`、`127.0.0.1`、`[::1]` authority；其他 Host 返回带 `no-store` 与 Request ID 的 server-level 421，但当前不是 JSON error envelope；
- 从安全字符集和 64 字节上限中复用客户端 Request ID，否则生成 `$request_id`；
- 把同一个 ID 注入 Go，使 upstream 超时也能与边缘日志关联；
- `client_max_body_size 16k`；`client_body_timeout 3s` 是相邻 body read 的 inactivity timeout，不是请求体总时长；
- API 不请求缓冲 body；通过 Nginx parser 并进入 API location 的非空 Transfer-Encoding/Trailer 声明返回 JSON 400；不受支持或语法非法的 Transfer-Encoding 可能在 location 前被 Nginx 原生拒绝，响应未承诺 JSON；
- 413、502、504 都返回与应用风格一致的 JSON/no-store/request-ID；
- `proxy_read_timeout 11s`，给 Go 3s use-case 和 10s write budget 留出清楚层次。

Host allowlist 只缩小本机开发攻击面，不是公开部署访问控制。正式发布仍需要 TLS、认证、对象级授权、限流、审计和真实反向代理策略。

Nginx access log 使用规范化 `$uri`，因此会记录实际 StrategyID 路径；它刻意不记录 query string 与 Referer。StrategyID 当前不是用户身份，但依然属于需要评估保留期和访问权限的业务标识，不能把“没有 query”误写成“没有业务 ID 日志”。

## 12. 隔离 Compose acceptance

[`compose-lottery-api-acceptance.sh`](../../../scripts/compose-lottery-api-acceptance.sh) 每次运行创建：

- 96-bit 随机 Compose project 名；
- 唯一镜像 tag；
- Docker 原子分配的 loopback host port；
- `mktemp` Secret 与 response 目录，并记录其 device/inode identity 供清理前复核；
- 两个独立 named volume；
- 独立 Buildx builder 和 cache。

脚本先记录长期 `growthos` 容器、卷和网络身份，只允许删除带本次精确 label/ID 的资源；不执行 system prune，也不删除默认项目数据。

验收 fixture 覆盖：

- MaxUint64 Strategy/Award；
- 单一 `no_reward`；
- `1:3` 多 Award；
- 真实 migrator 写入和 app SELECT-only grants。

真实 HTTP 断言覆盖：

- health/readiness；
- Host 421；
- reward/no_reward/MaxUint64 最小 DTO；
- 404、400、405、尾斜杠、query、body、空 chunked、非空 Trailer 声明、idempotency；
- 16 KiB + 1 的 JSON 413；
- 总计 64 个选择请求、最大并行 16，只返回配置内结果；
- 停止 API 后 Nginx JSON 502/504，再恢复健康；
- 请求前后两张业务表完整 fingerprint 不变；
- migration/grants/端口拓扑在并发后没有漂移；
- 结束时按 Docker label/ID 与临时目录 identity/文件类型复核后，精确删除本次容器、网络、卷、镜像、builder、cache、Secret 和 response。

这一验收证明的是开发环境的真实纵向切片，不是生产容量测试、公平认证或灾备演练。

## 13. 本节提交学习顺序

分支：`codex/lesson-21-lottery-api`

建议依次比较：

1. `ea71640..65e9627`：application use case、HTTP adapter、装配与首版契约；
2. `65e9627..be41d92`：公开边界最小化、feature flag、timeout、Award 上限、最小权限、Nginx 防护；
3. `be41d92..9100221`：真实代理发现空 chunked 正规化后，在边缘拒绝 Transfer-Encoding；
4. `9100221..e32ecd4`：一次性真实 Compose acceptance；
5. `e32ecd4..93f5694`：长期 smoke 动态选择缺失 StrategyID，避免假设 MaxUint64 永远不存在；
6. `93f5694..3d4a44a`：真实代理发现 Trailer 声明会在转发时消失，在仍可观察非空声明的边缘拒绝；
7. `3d4a44a..ef3f266`：清理前复核临时目录 device/inode identity，并把并发证据改写为“64 个请求、最大并行 16”；
8. `ef3f266..7c43456`：补齐空白 ID、纯 unknown Content-Length 和 force-query 的独立请求边界测试；
9. 后续文档提交：课程、API、QA、设计手记、面试题、ADR 与全局索引；
10. 最终 checkpoint：记录完整 SHA 与验收事实。

用下面的命令阅读变化：

```bash
git diff ea71640..65e9627
git diff 65e9627..be41d92
git diff be41d92..9100221
git diff 9100221..e32ecd4
git diff e32ecd4..93f5694
git diff 93f5694..3d4a44a
git diff 3d4a44a..ef3f266
git diff ef3f266..7c43456
```

## 14. 可以怎样运行

### 14.1 长期开发栈

```bash
make compose-up
make compose-smoke
```

长期数据库默认没有演示 fixture。smoke 会只读推导一个不存在的 StrategyID，并验证该路由真实到达 MySQL 后返回相关联的 404。

### 14.2 业务数据与 Compose 资源隔离的一次性验收

```bash
make compose-lottery-api-acceptance
```

该命令会构建和删除一次性资源，适合冻结第 21 节证据；不要把它改成使用长期 `growthos_mysql_data`。

### 14.3 直接请求已有 Strategy

仅当你的开发数据库已经有合法 Strategy 时：

```bash
curl --request POST \
  --header 'Accept: application/json' \
  --header 'X-GrowthOS-Demo-Mode: ephemeral-selection' \
  http://127.0.0.1:8088/api/v1/lottery/strategies/1/ephemeral-selections
```

重复调用可以返回不同 Award；这正是 ephemeral 契约，它不提供“同一请求身份恢复同一结果”的业务幂等能力。

## 15. 本节不能写进简历的夸大表述

不能表述：

- “完成高并发在线抽奖闭环”；
- “保证一次请求 exactly-once”；
- “Redis 分布式锁保证幂等”；
- “用户资格、次数、库存和发奖已完成”；
- “抽奖概率达到监管公平”；
- “接口已经生产发布”；
- “具备鉴权、限流、审计和防刷”；
- “前端 Lottery 页已经调用后端”；
- “压测达到某个业务 QPS/P99”。

可以准确表述：

> 设计并实现 development/test 专用的临时 Lottery Selection API，以 application port 组合 MySQL 一致快照和无偏加权选择器；使用十进制字符串保护完整 uint64，建立稳定错误/超时/Request-ID/最小权限边界，并通过一次性 Compose 环境验收 Nginx→Go→MySQL→CryptoSource、并发与故障，证明两张 Lottery 业务表全列 fingerprint 不变。

## 16. 下一节停止条件

第 22 节可以让 React Lottery 页面调用这个临时 API，但必须继续显示：

- 这是演示选择，不是正式 Draw；
- 每次点击是新选择；
- `no_reward` 与系统错误是不同 UI 状态；
- 页面不能发送 seed、AwardID 或伪造幂等键；
- 大整数 ID 在 TypeScript 中保持 string；
- 前端不能把 Mock 的“锁、库存、发奖”文案当已实现能力。

若需求变成真实价值抽奖，应停止在 UI 层继续包装，而先新增 Activity/public identity、发布态、资格、Participation、Draw/Result 持久化、幂等、结果查询、限流、防刷、库存和 Benefit 发放模型。

## 17. 关联资料

- [第 21 节 API 记录](../../api/lessons/lesson-21.md)
- [第 21 节 QA](../../qa/lessons/lesson-21.md)
- [第 21 节第一性原理设计手记](../../design-thinking/lessons/lesson-21.md)
- [第 21 节面试问答](../../interview/lessons/lesson-21.md)
- [ADR-0018：临时 Lottery Selection API](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)
- [RFC 9110：HTTP Semantics](https://www.rfc-editor.org/rfc/rfc9110.html)
- [Go context](https://pkg.go.dev/context)
- [OWASP REST Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html)
- [Nginx proxy module](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)
