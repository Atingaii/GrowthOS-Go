# ADR-0018：development/test 临时 Lottery Selection API 边界

- **状态：** 已接受
- **日期：** 2026-08-29
- **决策者：** GrowthOS-Go 当前实现与验收
- **适用范围：** 第 21 节首个 Lottery HTTP 纵向切片
- **替代关系：** 不替代 ADR-0013、ADR-0016 或 ADR-0017；把三者组合进受限 HTTP 用例

## 背景

第 17～20 节已经建立：

- Strategy/Award 聚合与整数权重；
- 两张 MySQL 表和完整聚合快照读取；
- 无偏加权选择器和密码学随机 adapter；
- Gin、统一 fault、Request ID、MySQL readiness 与 Compose 边缘。

课程第 21 节要求开放第一个 Lottery API。但当前业务模型尚无 Activity 发布态、用户资格、Participation、DrawID、幂等请求、结果持久化、库存或 Benefit 发放。若直接命名并实现正式 `/draws`，HTTP 契约会暗示系统已经创建可查询的最终结果，这与事实不符。

本 ADR 决定怎样交付一个可真实调用、可学习、可验收，同时不冒充正式抽奖的最小纵向切片。

## 决策驱动

1. 不把一次内存选择冒充持久 Draw；
2. 让 HTTP 方法、路径、状态码和缓存语义可解释；
3. 完整支持领域既有 uint64 范围；
4. 保持 handler/application/domain/repository 分层；
5. 不静默接受无法履行的幂等承诺；
6. 让 deadline、错误和 Request ID 穿过真实 Nginx/Go/MySQL 链；
7. 最小化公开内部配置；
8. 运行身份遵循实际读用例的最小权限；
9. 默认关闭，阻止 staging/production 误开；
10. 用一次性 Compose 验收真实成功、并发、故障，并证明两张 Lottery 业务表 fingerprint 前后不变；
11. 保留未来正式 Draw 模型的设计空间；
12. 文档必须能明确说明当前“实现了什么”和“没有实现什么”。

## 非目标

本决策不实现：

- Strategy CRUD 或运营发布；
- public Activity/Campaign identity；
- 用户认证、授权、资格、次数或频控；
- Draw/Result 记录、幂等或结果查询；
- Strategy/Award/算法历史快照；
- 库存、Benefit 发放、MQ、Outbox 或补偿；
- Redis 业务缓存、分布式锁或令牌桶；
- 公网 ingress、TLS、WAF 或生产 SLO；
- React Lottery 真实页面；
- 公平审计、法规认证或可验证随机。

## 候选方案

### 1. 不开放选择，只开放 Strategy GET

| 优点 | 代价 |
| --- | --- |
| 不引入随机重试语义；可先练习 DTO | 无法验收 Repository + Selector 的真实纵向组合；课程“首个 Lottery API”价值有限 |

结论：合理，但不满足本节希望建立完整读取/选择纵向切片的目标。

### 2. 直接开放正式 `POST /draws`

| 优点 | 代价 |
| --- | --- |
| 命名简短；看起来接近业务 | 没有 DrawID/结果持久化/请求身份，却暗示创建最终事实；响应丢失无法恢复；资格和授权缺失 |

结论：拒绝。名称不能替模型背书。

### 3. `GET /strategies/:id/random-award`

| 优点 | 代价 |
| --- | --- |
| 无 body、便于浏览器调用 | 当前虽不写业务状态，却被产品建模为调用者显式发起的一次 selection action；GET 容易进入链接、预取、爬虫和通用缓存路径，也不利于未来持久 Draw 演进 |

结论：拒绝。

### 4. `POST /strategies/:id/selections`，不说明 durability

| 优点 | 代价 |
| --- | --- |
| HTTP 方法正确、路径较干净 | 调用方仍会问 Selection 是否保存；重试边界隐含，文档容易漂移 |

结论：不采用。

### 5. `POST /strategies/:id/ephemeral-selections`

| 优点 | 代价 |
| --- | --- |
| 方法表达新处理；名称诚实说明不持久；可组合已有能力；未来正式 Draw 可独立建模 | 路径较长；调用方必须理解每次调用是新选择；仅适合演示/测试 |

结论：采用，并用 feature gate 和 required demo header 再次确认边界。

## 决策

### 1. Route 与方法

注册：

```text
POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections
```

理由：

- `POST` 表达调用者显式发起的一次新 selection action；当前无业务写入，不能仅凭随机响应不同就把它证明为 RFC 意义上的 unsafe/non-idempotent；
- `ephemeral-selections` 明确不创建持久 Draw；
- StrategyID 是当前可用内部聚合身份，允许受控演示；
- `/api/v1` 把业务 contract 与 `/health`、`/ready` 分开。

成功返回 `200 OK`：没有 `Location` 或可寻址新资源，不返回 201；处理同步完成，不返回 202。

### 2. 可用环境

新增 `GROWTHOS_LOTTERY_EPHEMERAL_SELECTION_ENABLED`：

- 默认 false；
- development/test 可显式 true；
- staging/production true 使配置无效并阻止进程启动；
- Compose development 明确 true；
- false 时不注册 route，而不是注册后每次返回 403/404 feature error。

环境限制是防误用层，不是授权模型。

### 3. 显式调用确认

请求必须恰好有一个：

```http
X-GrowthOS-Demo-Mode: ephemeral-selection
```

它表示调用者理解临时语义，并为本地普通跨站 form 增加 CSRF 摩擦。它不是 authentication、authorization、signature 或 idempotency token。

### 4. 输入采用失败关闭 allowlist

唯一业务输入是 path StrategyID。接口拒绝：

- 非规范 uint64 ID；
- 任意 query；
- 非零或未知 Content-Length；显式 `Content-Length: 0` 与无 body 等价；
- 已通过 Nginx parser 并进入 API location 的非空 Trailer/Transfer-Encoding 声明，以及 Go 自己能观察到的 TransferEncoding/Trailer；
- 不受支持或语法非法的 Transfer-Encoding 允许由 Nginx parser 更早拒绝，不承诺统一 JSON；
- 任意 Idempotency-Key；
- 重复或错误 demo header。

采用 `ParseUint(raw, 10, 64)` 加 `FormatUint` round-trip，禁止自动接受前导零、符号、空白或别的进制。

### 5. Idempotency-Key 必须显式拒绝

服务器没有 `(request identity → persisted result)` 映射，不能实现重复请求返回同一 Award。静默忽略 header 会让 SDK/调用方产生错误安全感，因此出现即返回稳定 400。

Repository/use-case/selector 没有显式重试，不会在一次业务调用中再次执行选择。底层 `database/sql` 可能在事务真正建立前淘汰 `driver.ErrBadConn` 连接并重新尝试 `BeginTx`；这不等于重执行已经开始的 Repository 查询或 selector。上层若重试，会形成新的 ephemeral invocation。

### 6. application 层拥有用例组合

新增 `EphemeralSelectionService`：

- 依赖 application-owned `StrategyReader`；
- 新定义 consumer-owned `AwardSelector`；
- 先读取完整 Strategy，再选择一个 Award；
- 验证返回 StrategyID 与请求一致；
- 验证 Award 是快照内逐字段一致的候选；
- 无 Lottery 业务状态写入，不发布业务事件，也不持久化 selection 结果；MySQL 读取、熵源调用、访问日志与运行指标等技术交互或副作用仍然存在；
- nil/typed-nil/零值组合在 startup 失败。

HTTP handler 不直接依赖 `mysqlrepo.Repository` 或 `WeightedSelector` 具体类型。

### 7. context 与 timeout

HTTP adapter 为每次选择派生 `context.WithTimeout`。默认 3s，上限 30s。配置同时要求：

```text
selection timeout + 1s <= MySQL read timeout
selection timeout + 1s <= HTTP write timeout
```

用例在每个依赖之前和之后检查 `ctx.Err()`；一旦在返回点观察到取消/截止，context error 优先于依赖同时返回的其他 error。

准确限制：`AwardSelector.Select` 当前是同步、无 context 的 domain port，不能被抢占。timeout 是 cooperative budget，不是硬实时中断。通过 `MaxAwardsPerStrategy=1000` 与 Repository `LIMIT 1001` 限制不可取消 O(n) 段。若随机源变为远程依赖，必须重评接口。

### 8. 最小响应 DTO

返回：

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

决定：

- uint64 identity 编码为十进制 string；
- `durability` 固定 `ephemeral`；
- `no_reward` 作为正常 Award/200；
- 不返回 Strategy operator name；
- 不返回 Award weight、total、ticket 或推导概率；
- 不虚构 DrawID、Benefit 或 inventory 状态；
- 不直接序列化 domain struct。

### 9. 错误映射

| 内部条件 | HTTP | 公开 code |
| --- | ---: | --- |
| caller contract invalid | 400 | 精确输入错误 code |
| Strategy not found | 404 | `lottery_strategy_not_found` |
| method mismatch | 405 + Allow | `method_not_allowed` |
| cancel/deadline | 503 | `lottery_selection_unavailable` |
| repository retryable | 503 | `lottery_selection_unavailable` |
| random source failure | 503 | `lottery_selection_unavailable` |
| stored invalid/ordinary repository/composition/selector invariant | 500 | `internal_error` |

选择到 `no_reward` 不进入错误映射。

不返回 `Retry-After`，因为服务没有可靠的恢复时间或退避建议；该 header 本身也不承诺同一结果，调用方仍须独立理解重试会形成新选择。错误 cause 不进入 JSON 或普通模块日志；日志记录安全 StrategyID、Request ID 和低基数 class。

### 10. Cache 与 request correlation

所有 selection success/error 返回 `Cache-Control: no-store`。API location 显式产生的 400/413/502/504 使用 JSON/no-store/单一 Request ID；非 allowlist Host 的 421 是 server-level 非 JSON 拒绝，但仍有 no-store 与单一 Request ID。HEAD 的真实 wire 按协议不发送 body，parser 早期 400/501 也不在 JSON envelope 承诺内。

Nginx：

- 复用字符集受限、最长 64 字节的客户端 ID；
- 否则生成 `$request_id`；
- 将同一个 ID 传给 Go；
- 在 upstream 无响应时仍以它生成 gateway envelope；
- Go middleware 再验证，防止直接绕过 edge。

Request ID 只做关联，不做认证、业务 identity 或幂等。

### 11. Nginx 本地边缘约束

决定：

- loopback-only publish；
- Host allowlist 为 localhost/127.0.0.1/[::1]；
- 进入 API location 的非空 Transfer-Encoding/Trailer 声明在 edge 拒绝；unsupported/invalid framing 可由 HTTP parser 更早原生拒绝；
- client body 上限 16 KiB；body timeout 3s 是相邻读取 inactivity timeout，不是 body 总时长；
- API 不缓冲 request body；
- 413/502/504 自产错误使用统一安全 header；
- `proxy_read_timeout 11s`。

`proxy_read_timeout` 是相邻读取 inactivity timeout，不声明为总请求 deadline。Host allowlist 是开发边缘卫生，不代替公网授权。

### 12. 应用数据库权限

运行态只调用 `StrategyReader.FindByID`。因此 Compose 的 `growthos_app` 从两表 `SELECT, INSERT` 收敛到两表 `SELECT`，并继续禁止：

- INSERT/UPDATE/DELETE；
- schema wildcard DML；
- DDL；
- `schema_migrations` 访问；
- mandatory roles 隐式扩权。

隔离 fixture 使用 migrator 身份。Repository Create 的真实集成测试使用专门隔离 schema/确认变量，不要求 runtime reader 拥有写权限。

### 13. 一次性 Compose acceptance

新增 overlay 和脚本，必须：

1. 每次随机 project/image/port/secret/volume/builder；
2. 不使用长期 `growthos_mysql_data`；
3. 以 migrator 原子写入 max/no_reward/1:3 fixture；
4. 通过 Nginx 调用真实 API；
5. 检查并发、错误、framing、gateway failure 和恢复；
6. 比较请求前后业务表 fingerprint；
7. 精确检查 migration、grants 和 port topology；
8. 清理前核验 Docker label/ID ownership，并核对临时目录的 device/inode identity 与子文件类型；
9. 不执行 prune；
10. 结束时证明长期项目资源身份未改变。

## 关键控制流

```text
request
  -> edge Host/framing/size/request-id
  -> Gin canonical path/header/query/body checks
  -> context.WithTimeout
  -> service.Validate
  -> repository.FindByID(ctx) in read-only RR
  -> context check
  -> selector.Select(strategy)
  -> context check
  -> verify selected Award belongs to snapshot
  -> map minimal DTO
  -> JSON 200 + no-store + request-id
```

失败时没有 fallback Award，也不会把 `no_reward` 当故障替代值。

## 必须保持的约束

1. route 名称和响应必须继续表明 ephemeral，除非先实现并迁移到正式 Draw 模型；
2. production/staging 禁止 flag 的配置门不能静默移除；
3. demo header 不得在文档或代码中升级描述成认证；
4. 没有持久 result 前不得接受或伪实现 Idempotency-Key；
5. uint64 ID 不得改为 JSON number；
6. `no_reward` 不得改为 404/204/5xx；
7. response 不得泄露 weight/total/ticket/operator Strategy name，除非另有公开产品决策；
8. context deadline 不得夸大为可抢占 selector；
9. Award 数量上限必须在构造、恢复和 DB 读取三处一致；
10. runtime reader 不得因测试 Create 而重新预授 INSERT；
11. API location 显式生成的 400/413/502/504 必须保留 JSON/no-store/request-ID；不得把 server-level 421、HEAD 或 parser 早期拒绝写进该承诺；
12. Host allowlist 不得被描述为正式授权；
13. acceptance 只能删除本次所有权已验证的资源；
14. 每次调用不得写 Lottery 业务表；
15. 正式 Draw 不能通过给当前 DTO 增加一个随机 ID 草率实现。

## 影响

### 正面影响

- 第一次证明完整 Nginx→Go→MySQL→domain→crypto 链；
- 名称和 feature gate 防止把 demo 冒充生产 Draw；
- application 层获得清晰用例边界；
- JSON 完整支持 uint64；
- `no_reward` 和技术失败清楚；
- 公开 DTO 最小，不泄露概率配置；
- 启动、timeout、错误和日志边界可测试；
- runtime 数据库权限进一步最小化；
- acceptance 不污染长期数据，可反复学习；
- 为第 22 节真实 React 联调提供诚实后端。

### 成本

- 路径和 required header 较显式；
- 客户端不能使用通用 Idempotency-Key retry middleware；
- 每次调用都访问 MySQL，尚无缓存；
- 只读快照 + crypto 的延迟高于前端 Mock；
- Nginx/Go 两层 contract 增加配置与测试维护；
- 一次性 acceptance 构建多镜像，执行成本高；
- 正式 Draw 将是新模型，而非简单重命名。

### 风险

- 学习者仍可能把 200 reward 写成“已中奖并发奖”；
- endpoint 可被本机进程重复调用，尚无限流；
- 内部 StrategyID 仍可枚举；
- synchronous selector 若未来接远程源会越过 cooperative deadline；
- Nginx 与 Go 的 framing/timeout/header 配置可能漂移；
- 未来 response 扩张可能泄露运营配置；
- 若开发数据库存量数据损坏，会返回通用 500，需日志/数据库诊断；
- acceptance 依赖 Docker/registry，本地网络故障可能阻断构建但不代表业务失败。

## 被刻意发现但延后的问题

### 为什么没有 rate limit

当前只有 loopback development/test 路由，无用户 identity 和正式流量模型。立即接 Redis 限流会让课程顺序与业务 key 选择都失真。边缘体积、timeout、Award 上限和 loopback 先限制单请求成本；真正 rate limit 要随 public Activity、用户/设备/tenant identity 和滥用模型设计。

### 为什么没有缓存 Strategy

第 24 节才引入 Redis。现在直读 MySQL让契约、快照、错误和权限证据清晰，也先建立基线。没有发布态/version/cache key/失效事件前，提前缓存会掩盖错误边界。

### 为什么没有 RFC 9457 Problem Details

项目在第 12 节已发布统一 `{error:{code,message,request_id}}` envelope。第 21 节复用它以保持现有前端和网关一致。未来若采用 `application/problem+json`，应统一迁移所有 API 并决定 type URI、扩展字段和版本兼容，而不是单 route 混用。

### 为什么没有 422

当前所有输入是 path/header/framing 的语法/契约错误，400 足够。尚无语法合法但业务不可处理的 JSON command；引入规则请求后可重新评估 409/422。

### 为什么 not-found 可能泄露枚举

这是当前受控 demo 的可接受开发反馈，不是生产授权策略。正式 API 可能在未授权时统一 404，且以 public Activity identity 解析，不直接枚举内部 Strategy。

## 演进路径

正式价值 Draw 需要至少：

```text
public Activity
  -> published version / time window / audience / authorization
  -> Participation and attempt identity
  -> idempotency record with unique constraint
  -> load immutable Strategy version
  -> select Award
  -> persist DrawResult + snapshots atomically
  -> return DrawID / query location
  -> inventory / Benefit delivery state machine
  -> events / outbox / compensation / audit
```

届时：

- 新建正式 endpoint，而不是悄悄改变 ephemeral 语义；
- 可信 client request identity / Idempotency-Key 必须绑定持久记录；`X-Request-ID` 继续只做诊断关联，不能拿来替代幂等身份；
- response 区分 selection result、inventory reservation 和 delivery；
- timeout/commit unknown 有可查询恢复路径；
- rate limit key 来自可信 identity；
- authorization 在解析对象前后都有明确策略；
- Strategy/Award/算法版本进入历史事实。

## 重评触发器

以下任一发生时必须重评：

1. 路由要在 staging/production 打开；
2. 接入真实用户或有价值奖励；
3. 需要客户端自动重试或 Idempotency-Key；
4. 需要结果查询、争议审计或 exactly-once 语义；
5. 需要认证、租户/对象授权或 public Activity；
6. 需要 rate limit、防刷、资格或次数；
7. Award/Strategy name/weight 的公开策略变化；
8. random source 变成网络/HSM 依赖；
9. Award 数量超过 1000；
10. profile 表明 MySQL 或 O(n) selector 成为瓶颈；
11. 引入 Redis cache 与失效事件；
12. 从现有 error envelope 迁移 RFC 9457；
13. Nginx 被云网关/service mesh 替代；
14. 多副本、跨地域或生产 SLO 出现；
15. 法规要求概率披露、可验证随机或审计保存。

## 验收证据

- [application use case](../../internal/lottery/application/ephemeral_selection.go)
- [HTTP adapter](../../internal/lottery/adapter/httpapi/selection.go)
- [composition root](../../cmd/growth-api/database.go)
- [feature/config validation](../../internal/platform/appconfig/config.go)
- [Repository bound](../../internal/lottery/adapter/mysqlrepo/repository.go)
- [Nginx edge](../../deploy/docker/nginx.conf)
- [Compose topology](../../deploy/compose/compose.yaml)
- [isolated acceptance overlay](../../deploy/compose/compose.lesson21-acceptance.yaml)
- [isolated acceptance script](../../scripts/compose-lottery-api-acceptance.sh)
- [第 21 节课程](../course/part-03/lesson-21-lottery-api.md)
- [第 21 节 API 记录](../api/lessons/lesson-21.md)
- [第 21 节 QA](../qa/lessons/lesson-21.md)

## 参考

- [RFC 9110：HTTP Semantics](https://www.rfc-editor.org/rfc/rfc9110.html)
- [RFC 9111：HTTP Caching](https://www.rfc-editor.org/rfc/rfc9111.html)
- [RFC 8259：JSON](https://www.rfc-editor.org/rfc/rfc8259.html)
- [RFC 9457：Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html)
- [Go context](https://pkg.go.dev/context)
- [Go strconv.ParseUint](https://pkg.go.dev/strconv#ParseUint)
- [Go database/sql](https://pkg.go.dev/database/sql)
- [Nginx proxy module](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)
- [Nginx core module](https://nginx.org/en/docs/http/ngx_http_core_module.html)
- [OWASP REST Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html)
- [OWASP API Security Top 10 2023](https://owasp.org/API-Security/editions/2023/en/0x11-t10/)
