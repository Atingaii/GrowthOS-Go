# 第 21 节第一性原理设计手记：把“通过 HTTP 观察一次选择”与“完成一次可兑现抽奖”继续分开

> 第 21 节交付的是一个仅限 development/test、默认关闭、同步且不持久化的 Lottery ephemeral selection API。成功响应只证明：服务在本次调用中读取了一个合法 Strategy 快照，并返回其中一个配置过的 Award；它不证明用户有资格、次数已扣减、奖励已预占或发放、结果已持久化、请求可幂等重放，也不构成最终 DrawResult。

这份文档不从“用了 Gin、MySQL、Nginx”开始，而从不可绕过的事实开始。架构师的工作不是尽可能多地加入技术，而是：先找出系统必须守住的事实，再选择足够小、可被证伪、未来可演进的机制。

## 0. 如何阅读这份设计手记

每个重要决策都按同一组问题展开：

1. **Why：** 为什么现在必须解决；
2. **Fact：** 当前已经成立的业务/技术事实；
3. **Constraint：** 不允许被破坏的约束；
4. **Options：** 至少有哪些可行方案；
5. **Trade-off：** 为什么采用当前方案，而不是“更流行”或“更复杂”的方案；
6. **Failure：** 失败时系统到底知道什么；
7. **Evidence：** 哪条可执行证据能推翻错误实现；
8. **Limit：** 证据不能证明什么；
9. **Trigger：** 什么新事实出现时必须重评。

这九问比“套哪种架构模板”更接近真实工程思考。

## 1. 决策命题与事实时间切片

### 1.1 真正的问题不是“怎样写一个抽奖接口”

第 20 节以后，代码已经能做两件独立的事：

```text
StrategyReader.FindByID(ctx, id) -> Strategy
WeightedSelector.Select(strategy) -> Award
```

把两行调用放进 handler 很容易，但网络接口同时产生新的永久承诺：

- 调用者能否安全重试；
- 响应是否代表一个最终事实；
- ID 能否被 JavaScript 无损表达；
- 谁可以选择哪个 Strategy；
- 不存在与未授权是否可区分；
- 超时后结果是否未知；
- `no_reward` 与系统失败是否相同；
- 内部权重、名称和错误是否可公开；
- Nginx 改写请求以后，Go 看到的是否仍是原始 contract；
- 运行账号为什么需要写权限；
- 如何证明真实链路没有污染开发数据。

因此本节的核心任务是：**找到当前事实允许发布的最小网络语义，并明确拒绝尚未能兑现的语义。**

### 1.2 本节之前已经成立的事实

- `Strategy` 是一个可复用的 Lottery 决策配置，不是 Marketing Activity；
- `Award` 是 Strategy 内的候选描述，不是用户已经拥有的 Benefit；
- Weight 是正整数相对权重；
- `reward` 与 `no_reward` 都是合法候选；
- MySQL Repository 能在一个只读 `REPEATABLE READ` 快照中恢复完整聚合；
- WeightedSelector 能把均匀 `[0,total)` ticket 无偏映射到 Award；
- CryptoSource 使用标准库有界密码学随机；
- Gin 已有统一 error、Request ID、access log、recovery；
- Compose 已有 Nginx、API、MySQL、Redis 和一次性 Migration/Grant job。

### 1.3 本节形成的新事实

- 存在 `POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections`；
- 真实产品进程装配 Repository、CryptoSource、WeightedSelector 和 application service；
- 一次调用读取 MySQL 快照并返回配置内 Award；
- `reward` 与 `no_reward` 都能经过真实 Nginx 获得 200；
- MaxUint64 identity 可以从 path 到 JSON string 无损往返；
- 应用失败被翻译为稳定 HTTP contract；
- edge 和 origin 共享安全 Request ID；
- runtime app 对两张业务表只有 SELECT；
- 一次性 Compose acceptance 证明调用前后业务表 fingerprint 相同。

### 1.4 本节仍未形成的事实

以下能力即使在代码里“看起来只差一点”，也必须明确写成未实现：

- 用户身份、租户、认证、对象级授权；
- Activity/Campaign 对 Strategy 的公开绑定；
- 草稿、发布、归档、版本、时间窗和受众；
- Participation、资格、次数、积分扣减；
- DrawID、DrawResult、唯一约束、结果查询；
- Strategy/Award/算法历史快照；
- 库存预占、发奖、Benefit 状态；
- 幂等、结果未知恢复、重试对账；
- rate limit、反刷、风控；
- Redis 业务缓存或锁；
- MQ、Outbox、补偿；
- 真实 Lottery React 联调；
- 指标、trace、业务审计和生产 SLO；
- 公平认证、概率披露或可验证随机。

### 1.5 一条可以从零重放的推理链

```text
现有事实只有 StrategyReader + WeightedSelector
→ 能诚实表达的最小行为只是“一次瞬时选择”
→ 名称、路径和 DTO 必须显式带 ephemeral
→ 不创建可寻址资源，因此返回 200，不返回 201/Location
→ 每次调用都会重新选择，因此拒绝 Idempotency-Key
→ 当前没有命令载荷，因此 query/body/transfer framing 采用失败关闭 allowlist
→ 没有业务写入，因此 runtime identity 收敛为 SELECT-only
→ 内部 Strategy 不是公开活动，因此 route 默认关闭且禁止 staging/production 开启
→ 只有 DrawResult、identity、authorization、幂等与发奖边界形成后，才能新设计正式 Draw API
```

这条链中的任何一步若无法从前一步推出，就说明方案可能夹带了尚未出现的需求。

## 2. 为什么现在做一个 ephemeral API

### 2.1 它的真实价值

这个 API 的价值不是“简历上有 REST 接口”，而是第一次验证前四节的架构是否真的能组成纵向切片：

- application 是否能拥有 use case，而不是 handler 直接调用 concrete adapter；
- 一个共享 `sqlx.DB` 是否能同时服务 readiness 和 Repository，所有权仍在 composition root；
- Repository error 和 selector error 是否能被 transport 明确翻译；
- context 是否能从 HTTP 传到数据库；
- uint64 是否能穿越 JSON/JavaScript 边界；
- Nginx 是否会改变单元测试里的请求事实；
- runtime grant 是否与当前在线 SQL 完全相等；
- 前端下一节是否有一个真实但不夸大的服务端事实源。

### 2.2 为什么不等完整 Draw 再开放 API

等待完整 Draw 的优点是产品语义最完整，但代价是把许多独立风险拖到同一次交付：

- 数据身份与幂等；
- 用户规则；
- HTTP DTO；
- MySQL 结果事务；
- Nginx；
- 前端；
- 库存和权益。

当它们一起失败时，很难判断是分层、协议、事务还是业务模型有问题。当前方案通过五个保险丝缩小误用：

1. 名称明确 `ephemeral`；
2. 默认关闭；
3. staging/production 配置硬拒绝；
4. required demo acknowledgment；
5. SELECT-only + 无结果持久化。

### 2.3 为什么不把所有未来能力先做了

“以后肯定需要”不等于“现在已经知道怎样正确做”。例如 rate limit 的 key 可以是 IP、用户、设备、Activity、tenant 或它们的组合；在没有可信 identity 和滥用模型时先接 Redis，只会把猜测固化成基础设施。

第一性原则不是永远做最少，而是只为已经出现的约束支付复杂度。

### 2.4 本节的停止条件

当以下三件事同时成立，本节就应停止，而不是继续膨胀：

- 真实 HTTP 能读 Strategy 并返回配置内 Award；
- contract 诚实说明它不持久，也不支持“同一请求身份返回同一结果”的业务幂等重放；
- 失败、权限、edge 和真实验收有证据。

Activity、规则、缓存和正式结果属于后续新事实，不应偷偷进入本节。

## 3. 领域语言：名字是防止错误承诺的第一道边界

### 3.1 术语表

| 术语 | 当前准确含义 | 不代表 |
| --- | --- | --- |
| Strategy | 可复用的加权候选配置 | Activity、发布版本、用户入口 |
| Award | Strategy 中可被选中的描述 | 已有库存、已到账权益 |
| Selection | 本次同步计算返回的候选 | 可查询最终抽奖事实 |
| `reward` | 选择到奖励型候选 | 库存已锁定、Benefit 已发放 |
| `no_reward` | 正常选择到未中奖候选 | 系统错误、fallback、空结果 |
| EphemeralSelection | 没有身份/持久化的瞬时输出 | DrawResult |
| Request ID | 一次 HTTP 处理的相关 ID | Idempotency key、DrawID、trace/audit ID |

### 3.2 为什么 application 输出刻意不叫 DrawResult

名字会引导调用者对生命周期做推断。`DrawResult` 通常意味着：

- 有 identity；
- 有持久化；
- 可再次查询；
- 在配置变化后仍能解释；
- 与某次 Participation 绑定；
- 外部副作用可以关联。

当前 struct 只有 Strategy 和 Award 两个值，所以叫 `EphemeralSelection`。这是用语言阻止过度承诺，而不是文字洁癖。

### 3.3 `no_reward` 的不变量

技术失败不能为了“用户体验友好”被降级成 `no_reward`。否则：

```text
MySQL 故障 / 熵故障 / 数据损坏
→ 用户看到未中奖
→ 业务把系统可用性问题转嫁为用户损失
→ 监控也无法区分真实概率与故障率
```

所以：

- `no_reward` = 完成了合法选择，200；
- error = 没有可信 Award，4xx/5xx；
- fallback 到第一个 Award 或 no_reward 均禁止。

### 3.4 reward 也不是“发奖成功”

response 中的 `reward` 只说明候选类型。未来至少还有：

```text
selected → inventory reserved → benefit created → delivery succeeded/failed
```

把第一步直接显示成“奖励到账”，会让后续补偿和结果未知无法表达。

## 4. 系统边界与依赖方向

### 4.1 当前控制流

```text
Browser / curl
    ↓
loopback Nginx
    ↓  Host / TE / size / request-id / gateway errors
Gin shared middleware
    ↓  request-id / access log / recovery
Lottery HTTP adapter
    ↓  canonical input / DTO / public fault mapping
EphemeralSelectionService
    ├── StrategyReader
    │     ↓
    │   MySQL Repository
    │     ↓
    │   read-only REPEATABLE READ snapshot
    └── AwardSelector
          ↓
        WeightedSelector
          ↓
        CryptoSource → OS entropy
```

没有任何运行箭头指向 Redis、RabbitMQ、PostgreSQL、Participation、Benefit 或库存。

### 4.2 为什么 interface 由 consumer 拥有

application 需要的是：

```go
type AwardSelector interface {
    Select(domain.Strategy) (domain.Award, error)
}
```

它不需要知道 concrete `WeightedSelector` 的构造方式，也不需要把一个巨大的“LotteryRepository”接口引进来。consumer-owned interface 的收益是：

- 端口只包含 use case 实际依赖；
- domain 不反向认识 application；
- HTTP 不依赖 random adapter；
- test double 可精确模拟边界；
- 未来更换 selector 不改变 handler。

代价是接口数量增加；只有当某个 use case 真正需要能力时才定义，避免“为了六边形架构而造接口”。

### 4.3 composition root 为什么拥有 pool

一个 `sqlx.DB` 同时被：

- readiness checker；
- Repository；
- runtime close path

共享。Repository 借用 pool，不关闭它；`cmd/growth-api` 在 process shutdown 只关闭一次。部分装配失败时，已打开 pool 也由 root 负责关闭。

这是资源所有权规则：**创建长生命周期资源的一层决定它何时关闭；借用 adapter 不越权关闭共享资源。**

### 4.4 feature flag 关闭时为何仍构造 service

当前进程无论 route 是否开启，都构造 Repository、CryptoSource、Selector 和 Service；只有注册 route 受 flag 控制。

优点：

- 同一个 binary 的 composition 不会在关闭期间静默腐化；
- startup 测试能发现 zero/typed-nil service；
- 打开 flag 不会首次走一条从未装配的路径。

成本：

- flag 关闭时仍有少量无用对象；
- 若未来 random source 是远程/HSM，关闭功能仍建立外部依赖可能不合理。

重评条件：装配开始产生连接、费用、外部副作用或 readiness 依赖。

### 4.5 为什么不做一个全能 LotteryService

一个拥有 Strategy CRUD、抽取、资格、库存、发奖、缓存和日志的 service 会把变化原因混在一起。当前 service 只负责：

```text
load one canonical Strategy → select one configured Award → validate result
```

这个边界可以被一句话、一个小接口和一组测试完整解释。

## 5. application 用例：不能只信任 adapter 的返回值

### 5.1 调用顺序

1. 验证 context、StrategyID 和 composition；
2. 调用 `StrategyReader.FindByID(ctx,id)`；
3. 依赖返回后检查 context；
4. 验证返回 StrategyID 等于请求 ID；
5. selector 前再次检查 context；
6. 调用 selector；
7. 返回后检查 context；
8. 在 Strategy 中按 ID 查找 Award；
9. 对 ID/name/weight/outcome 做全字段一致性比较；
10. 返回前再检查 context。

### 5.2 为什么要验证 StrategyID

接口抽象并不能证明实现正确。一个损坏 adapter 可能收到 10 却返回 11；如果 application 直接选择，响应会把别人的配置映射到请求 ID。

这是典型的对象级边界错误。即使当前 MySQL SQL 使用参数绑定，application 仍把 adapter 视为信任边界。

### 5.3 为什么要验证 Award 全字段

只检查 AwardID 仍可能接受：

- 同 ID、不同 name；
- 同 ID、不同 weight；
- 同 ID、不同 outcome。

selector 的 contract 是返回“Strategy 中的那个 Award”，而不是返回“碰巧 ID 相同的另一个对象”。全字段复核将这个语义变成可执行证据。

### 5.4 为什么不用 map 加速复核

当前 Strategy 最多 1000 Award，selection 本身 O(n)，复核再 O(n)。引入 map 会：

- 增加聚合内存；
- 产生第二份派生结构；
- 需要维护 immutable ownership；
- 只为尚未出现的 profile 结论优化。

现在用线性复核换取简单、可审计和更少状态。若 profile 显示为热点，可在保持相同 contract 下重构。

### 5.5 错误返回时为什么必须是零结果

部分 Strategy 或 Award 与 error 同时返回会让上层面临“该不该展示/发放”的歧义。用例固定：

```text
success → complete EphemeralSelection + nil
failure → zero EphemeralSelection + error
```

这不是 Go 语言自动保证的，而是本用例的失败关闭约束。

## 6. HTTP 方法、资源与状态码的第一性推导

### 6.1 为什么是 POST，不是 GET

HTTP safety/idempotency 取决于调用者请求造成的预期服务器状态效果，不取决于响应是否每次相同。当前实现不写业务状态，所以不能只凭“消费熵、Award 不同”就证明它在 RFC 9110 的严格定义下 unsafe/non-idempotent。这里仍选择 POST，是因为产品把它建模为调用者显式发起的一次 selection action，并希望：

- browser/preloader 不应自动触发；
- cache 不应把旧结果当新结果；
- crawler 不应替用户形成新的选择观察；
- API 与未来会持久化的正式 Draw 演进保持同一 action 方向。

因此 GET 不合适。

### 6.2 为什么不是 PUT

PUT 通常由客户端指定目标资源 identity，并承诺相同请求可幂等替换。当前没有 SelectionID 或 DrawID，也没有保存结果；PUT 会制造不存在的 resource semantics。

### 6.3 为什么是 200，不是 201

201 要回答“创建了哪个资源”，通常伴随 `Location`。当前调用结束后没有可查询 resource，所以 201 是假信息。200 只表达同步处理成功。

### 6.4 为什么不是 202

没有排队、后台任务或后续状态查询，处理在响应前完成。202 会暗示服务只接受命令，结果未来产生；与实现相反。

### 6.5 为什么 `no_reward` 不是 204

204 会丢失选中的 Award identity/name/outcome，也让“成功未中奖”和“服务器没有返回 representation”混淆。`no_reward` 是有内容的业务结果，返回同一 200 DTO。

### 6.6 400、404、409、422 怎样选择

当前输入只有 path/header/framing：

- 非规范 ID、header、query、body → 400；
- 合法 ID 对应 Strategy 不存在 → 404；
- 当前没有资源状态冲突 → 不用 409；
- 当前没有语法合法但语义不可处理的 JSON command → 不用 422。

未来 Activity 已结束、次数耗尽、版本冲突或重复 request 出现时，必须重新建立业务 error taxonomy，而不是机械复制本节状态码。

### 6.7 为什么 405 必须带 Allow

405 表示资源路径存在但 method 不支持；`Allow: POST` 让 client 知道可用方法，也把 Gin NoMethod 行为锁进测试。尾斜杠使用 404 且关闭自动 redirect，避免 POST 被 307/308 重新发送到未明确发布的 canonical path。

### 6.8 为什么 503 没有 Retry-After

通常 503 会让人想到重试，但本接口没有可靠的恢复时间或退避建议，所以不返回 `Retry-After`。该 header 的规范语义只是建议多久后再发请求，并不承诺相同结果；调用方还必须独立知道，本接口的下一次请求是新的 selection。

## 7. 路径和 DTO：公开最少事实

### 7.1 为什么路径很长

```text
/api/v1/lottery/strategies/:strategy_id/ephemeral-selections
```

较长路径支付了可读性成本，却避免：

- `/draw` 暗示最终结果；
- `/random` 暗示只是工具函数；
- `/select` RPC 风格隐藏资源语义；
- `/strategies/:id/awards/random` 被误作 safe query。

### 7.2 为什么直接使用 StrategyID 只适合 demo

Strategy 是运营内部配置。生产用户真正访问的通常是 Activity/Campaign；Activity 再解析一个已发布 Strategy version。直接暴露 StrategyID 会产生：

- 枚举；
- BOLA；
- 绕过发布态和时间窗；
- 运营内部 ID 与外部兼容绑定。

本节通过 dev/test gate 控制范围，但不把风险消除。正式 API 必须换 public identity。

### 7.3 为什么 uint64 ID 是 decimal string

JavaScript 的安全整数上限低于 uint64。若 JSON number 返回 MaxUint64：

- browser 解析后可能舍入；
- log/cache key 发生碰撞；
- 请求与响应 identity 不相等；
- 前端再发回错误 ID。

十进制 string 保留领域范围，也让格式规范可检查。

### 7.4 为什么必须 canonical decimal

只要解析成相同数值就接受，会产生别名：

```text
1 == 01 == +1
```

别名会影响 cache、日志聚合、授权 key、幂等和签名。`ParseUint` 加 `FormatUint` round-trip 把同一 identity 固定为一个外部表示。

### 7.5 为什么不返回 Weight 和 TotalWeight

Weight 是运营内部配置，不是已发布概率承诺。返回它会：

- 泄露策略；
- 让 client 自行推导并展示概率；
- 把相对权重误读为百分比；
- 增加后续调整兼容负担。

不返回也不等于保密：无限重复采样仍可统计估计分布。因此真正保护依赖授权、限流和产品披露策略，而不是仅隐藏字段。

### 7.6 为什么不返回 Strategy name

领域注释明确它是 operator-facing。直接暴露可能遇到：

- 内部命名；
- PII/敏感备注；
- 未发布文案；
- 本地化缺失；
- Unicode 欺骗或内容审核问题。

未来用户展示应有独立发布 projection，而不是重用运营字段。

### 7.7 为什么 DTO 不直接复用 domain struct

domain 对象包含 Weight 等内部事实，且未来字段变化不应自动成为 API 变化。显式 mapper 提供：

- allowlist；
- string encoding；
- enum projection；
- API compatibility review point。

样板代码是有意支付的边界成本。

## 8. 请求输入：没有载荷也是一种严格 contract

### 8.1 为什么只允许 path ID

当前选择所需的唯一 caller fact 是 Strategy identity。seed、ticket、Award、Weight 或 algorithm 都由 server 管理。允许未使用字段会让调用者以为它们生效。

### 8.2 为什么拒绝 query

以下 query 都会改变调用者对行为的理解：

- `seed=`：调用者可操纵随机；
- `award_id=`：指定结果；
- `preview=`：混淆真实/预览；
- `idempotency_key=`：绕过 header contract；
- 未知参数：拼写错误被静默忽略。

因此 RawQuery 或 ForceQuery 非空即 400。

### 8.3 body framing 的准确边界

Go adapter 接受普通无 body 和显式 `Content-Length: 0`；它拒绝：

- `ContentLength != 0`；
- unknown length；
- 它能观察到的任意 `TransferEncoding`；
- 它能观察到的任意 Request.Trailer。

某些 `curl --data ''` 只形成零长度 Content-Length，因此可能通过；不能把 CLI flag 当成网络事实。

### 8.4 为什么不读取一个字节探测 body

读取 unknown/chunked body 会让慢客户端在 selection deadline 建立前占用 handler。只根据 framing 决策可以立即失败，不把 ingress body I/O 混入用例预算。

### 8.5 为什么 Go 检查还不够

Nginx 会解码 chunked body，Trailer 声明也可能在代理后不可见。空 chunked 到 Go 时可能已经与普通无 body 无法区分，所以进入 API location 的非空 Transfer-Encoding/Trailer 声明必须在 Nginx 仍能观察该值时拒绝。空 Trailer 值与 header 缺失在当前 map 中不可区分；unsupported/invalid Transfer-Encoding 还可能在 location 前由 Nginx parser 原生返回 400/501（可能是 HTML），不能把统一 JSON 承诺扩张到 parser 早期错误。

这是通用原则：**在最早仍拥有某项事实的边界验证它。**

### 8.6 为什么 required demo header 必须恰好一个

重复 header 可能被不同代理按 first/last/comma-join 解释。恰好一个精确值避免多层解析歧义。

它有两个作用：

- 调用者显式确认 ephemeral；
- 非 CORS-safelisted header 让普通跨站 HTML form 不能直接发出同形请求。

它不提供认证、授权、签名、防重放或 XSS 防护。同源恶意脚本和非浏览器客户端都能设置它。

### 8.7 为什么 Idempotency-Key 出现即拒绝

服务器没有 key→result 存储。静默忽略比拒绝更危险，因为 SDK 可能自动发送 header 并假设重试安全。

未来真正支持需要：

```text
trusted request identity
  + unique constraint
  + persisted result
  + request hash/conflict policy
  + commit-unknown recovery
  + result query
```

## 9. 数据读取、一致性与容量

### 9.1 为什么读取完整聚合

选择概率取决于根和全部 Award。若根、Award 在不同数据库快照读取，运营更新可能产生混合配置：

```text
old root + new awards
new root + partial awards
```

Repository 使用一个 read-only `REPEATABLE READ` transaction，保证本次两条查询处于同一快照。

### 9.2 为什么不用 `SELECT ... FOR UPDATE`

ephemeral selection 不修改配置。加锁会：

- 阻塞运营更新；
- 增加锁等待；
- 把只读流量变成写竞争；
- 仍不能形成历史版本。

请求内一致由 MVCC snapshot 解决；跨请求使用同一版本需要正式 published version/snapshot 模型。

### 9.3 为什么两条 SQL 而不是 JOIN

根 + 子表 JOIN 可一次 round trip，但会重复根字段并需要处理无 Award、行展开和聚合边界。当前两条参数化 SQL 在同一事务内更直观：

1. root 不存在可准确分类 404；
2. Award 按 ID 排序；
3. domain restore 在 transaction 后执行；
4. 测试可明确锁定顺序。

在真实 profile 证明 round trip 是热点前，不为少一条 query 牺牲恢复语义清晰度。

### 9.4 为什么最多 1000 个 Award

若没有上限，单个 Strategy 可放大：

- DB 返回行数；
- Go slice 内存；
- Unicode/领域恢复；
- O(n) selector；
- O(n) Award 复核；
- handler 占用时间。

1000 是当前工程 safety bound，不是产品建议值或生产容量证明。

### 9.5 为什么 SQL 用 LIMIT 1001

只 `LIMIT 1000` 会把超限配置截断成“看似合法的另一个 Strategy”。读取 `N+1` 才能区分：

```text
<= 1000 → 完整候选集
1001     → 至少超限，stored invalid
```

LIMIT 限制传回应用的行，不保证数据库完全零扫描；仍依赖 `(strategy_id,award_id)` 索引和执行计划。

### 9.6 为什么恢复在 transaction 结束后

数据库 transaction 只保护读取快照，不应该持有连接做纯 Go Unicode/领域校验。先复制 stored rows，commit read-only transaction，再 Restore：

- 缩短连接占用；
- 领域不依赖 SQL tx；
- bad snapshot 仍被 fail closed；
- 不会在校验期间维持数据库资源。

### 9.7 当前 success path 的复杂度

粗略为：

```text
DB read O(n)
domain restore O(n log n) 或既有规范排序成本
weighted select O(n)
result membership verification O(n)
```

最大 n=1000。这个界限让简单算法目前比 prefix/map/alias 的额外状态更合理，但 profile 仍是未来优化依据。

## 10. 超时与取消：合作式预算，不是魔法中断

### 10.1 当前预算表

| 层 | 当前 Compose 值 | 真实含义 |
| --- | ---: | --- |
| Nginx `client_body_timeout` | 3s | 两次请求体读取之间的 timeout，不是请求总时长 |
| Nginx `proxy_connect_timeout` | 2s | 连接 Go upstream |
| Nginx `proxy_send_timeout` | 4s | 向 upstream 相邻写操作 inactivity |
| Lottery selection context | 3s | application 协作式 deadline |
| MySQL driver read timeout | 5s | DB socket read；必须比 selection 至少多 1s |
| Go HTTP write timeout | 10s | server response write budget；必须比 selection 至少多 1s |
| Nginx `proxy_read_timeout` | 11s | upstream 相邻读取 inactivity，不是端到端总时长 |

### 10.2 deadline 从哪里开始

path/header/query/body framing 全部通过以后，handler 才创建 selection context。它有意不声称覆盖完整 ingress 生命周期；edge body timeout 处理前半段。

### 10.3 context 怎样传播

```text
http.Request.Context
  → context.WithTimeout
  → EphemeralSelectionService.Select
  → StrategyReader.FindByID
  → database/sql operations
```

application 在每个阶段之间检查 `ctx.Err()`，防止已经取消的请求继续选择或返回过期 success。

### 10.4 为什么取消要压过同时返回的 dependency error

若 dependency 返回 error 的同一时刻 deadline 已被观察到，返回 context error 能保持“该请求已过期”的 transport 语义，且停止后续阶段。原 dependency cause 可以通过受控 telemetry 保留，但不能让同一竞态随机变成 404/500/503。

### 10.5 selector 为什么不能被取消

domain `AwardSelector` 是同步接口，没有 context；`crypto/rand.Int` 也不接受 request context。deadline 到期不会抢占正在执行的 Go 函数，只能在它返回后被观察。

本节通过：

- source 为本地标准库；
- Award 上限 1000；
- 无网络随机源；
- gateway/HTTP 外层预算

限制风险，但不把它写成硬实时保证。

### 10.6 为什么不现在给 selector 加 context

当前 selector 是纯领域同步计算，加入 context 会让 domain API 为尚未存在的远程能力支付复杂度。真正出现 HSM/remote RNG/重计算后，可选择：

- context-aware random port；
- 在 application 包一层 remote adapter；
- 预取 entropy；
- 独立 timeout/circuit breaker。

需要基于真实依赖，而不是为了“所有函数都带 ctx”机械修改。

### 10.7 driver timeout 与 application timeout 的顺序

若 MySQL driver 1s 先超时、application 30s 后超时，客户端可能收到普通 repository 500，而非统一 deadline 503。配置因此要求 selection + 1s 不超过 MySQL read timeout，让更外层、语义更清楚的 use-case deadline先有机会生效。

同时 Repository 的共享 classifier 将 MySQL 1205/1213（锁等待超时/死锁）、`driver.ErrBadConn` 和 `net.Error` 分类为 retryable。Repository/use-case/selector 没有显式重试，分类主要影响上层 503 和诊断；底层 `database/sql` 仍可能在事务建立前淘汰坏连接并重新尝试 `BeginTx`，但不会重执行已经开始的 selection。将来一旦加入业务 retry，必须重新审核 `net.Error` 是否过宽、backoff、次数和新选择语义。

## 11. 错误模型：失败时系统到底知道什么

### 11.1 第一条原则：错误分类不能伪造业务结果

系统只在满足以下条件时返回 success：

- Strategy 快照完整且合法；
- selector 返回 nil error；
- Award 与快照中的完整字段一致；
- context 在返回点未取消。

任一条件不成立，都没有可信 selection。错误不能被转换成第一个 Award、随机 fallback 或 `no_reward`。

### 11.2 公开三层模型

| 层 | 例子 | 公开响应 | 恢复含义 |
| --- | --- | --- | --- |
| caller contract | 非规范 ID、header/query/body、幂等声明 | 400 | 修正请求，不曾选择 |
| business lookup | Strategy 不存在 | 404 | 选择未发生；检查 identity/发布关系 |
| temporarily unavailable | deadline/cancel、retryable DB、entropy failure | 503 | 没有可信结果；下一次是新 selection |
| internal invariant | stored invalid、普通 repository、selector/result contract | 500 | 数据/代码/依赖需诊断 |
| valid business outcome | `no_reward` | 200 | 选择成功，不走恢复 |

### 11.3 为什么 stored invalid 是 500，不是 404

表中有对应 root，但快照违反领域 contract，说明系统内部数据不一致。返回 404 会掩盖损坏并误导操作者“只是没有配置”。对外隐藏细节，对内记录 `stored_strategy_invalid` class。

### 11.4 为什么 random source failure 是 503

Strategy 合法，但当前无法获得可信随机值。它更接近临时能力不可用而非代码不变量；公开 503。若 source 返回越界值，则是 contract violation，属于内部 500，因为继续使用会破坏概率正确性。

### 11.5 为什么普通 repository failure 是 500

不是所有数据库错误都可安全标记“临时重试”。权限、schema、scan type、坏 SQL 和恢复契约都可能需要修复。只把审查过的 1205/1213、BadConn 和 net.Error 归为 retryable，其他保持 500。

当前 `net.Error` 分类仍较宽；因为项目层没有显式 retry，影响主要是 503 分类。未来若自动重试，必须区分 timeout、temporary、TLS/认证和永久网络配置错误。

### 11.6 为什么不把 raw cause 写进日志

raw cause 可能包含：

- SQL/table/constraint；
- network address；
- driver topology；
- 内部 package；
- entropy reader 细节；
- Secret/path。

普通 application log 只写 request ID、canonical StrategyID、稳定 error class。代价是现场调试信息减少；未来需要受权限控制的 trace/diagnostic event，而不是把敏感 cause 永久公开。

### 11.7 Error envelope 与 RFC 9457 的取舍

项目第 12 节已经发布：

```json
{"error":{"code":"...","message":"...","request_id":"..."}}
```

RFC 9457 `application/problem+json` 是成熟标准，但单独让 Lottery route 改格式会产生两套 client decoder、网关模板和错误治理。当前复用全局 envelope。未来整体迁移时再决定 type URI、status、instance、extension 和兼容窗口。

这个统一形状只覆盖非 HEAD、已经进入 Go handler 的错误，以及 API location 显式生成的 400/413/502/504。HEAD 的真实 wire 按协议不发送 body，server-level 421 是非 JSON，Nginx parser 早期 400/501 也可能是 HTML。

### 11.8 502 与 504 为什么 acceptance 都允许

停止 Docker API container 后：

- upstream 连接立即失败可能是 502；
- Nginx 缓存/解析到不可响应 endpoint 可能在 inactivity timeout 后是 504。

脚本不是“任意 5xx 都通过”，而是根据实际 status 检查精确 JSON code/message、no-store、Request ID，并随后恢复 API 健康。

### 11.9 421 的特殊边界

非法 Host 在 Nginx server 级 `return 421`，当前不经过 JSON error_page，所以：

- 有 HTTP 421；
- 有 server-level security/no-store/Request-ID header；
- 没有稳定 JSON error code；
- 不属于 Gin/public business fault。

把它写成 `misdirected_request` JSON 会超出真实实现。若未来 client 需要统一 JSON，应显式增加 named location/error_page 和 E2E 断言。

## 12. 威胁模型：先列资产，再谈安全机制

### 12.1 需要保护的资产

- Strategy、Award 和隐藏 Weight；
- 选择完整性与随机边界；
- MySQL 业务表；
- entropy/ticket；
- app/migrator/root Secret；
- request correlation 与日志；
- DB pool、CPU、goroutine、熵资源；
- 本机 Docker 其他项目与数据卷；
- 未来用户对“中奖/到账”的信任。

### 12.2 可能的威胁主体

- 好奇或恶意本机调用者；
- 跨站网页与 DNS rebinding；
- 被攻陷/XSS 的同源前端；
- 自动化重复采样者；
- slow body / framing 攻击者；
- 误配置运维人员；
- 损坏或恶意 adapter；
- 上游 MySQL/entropy 故障；
- 供应链或同进程不可信代码。

### 12.3 威胁矩阵

| 威胁 | 当前防线 | 为什么有效 | 剩余风险 |
| --- | --- | --- | --- |
| 调用者传 seed/ticket/award | query/body 全拒绝 | caller 无算法控制面 | 仍可反复采样 |
| 重复抽取提高命中机会 | dev/test-only、无真实发奖 | 当前无价值副作用 | 仍消耗 DB/entropy |
| 推断隐藏权重 | DTO 不返回 weight | 降低直接泄露 | 大样本可统计估计 |
| 枚举 Strategy | canonical ID + 404 | 输入可控且 SQL 参数化 | 无 auth/BOLA，生产不可接受 |
| 跨站普通 form | required custom header | 非 safelisted header 需预检 | XSS/同源/非浏览器可设置 |
| DNS rebinding/任意 Host | loopback + Host allowlist | 不接受攻击域 authority | 不是 TLS/auth |
| Request-ID 日志注入 | 单值、字符 allowlist、64 bytes | 避免换行/任意文本 | ID 仍可碰撞且非业务 identity |
| duplicate header ambiguity | demo/idempotency/request ID 精确检查 | 多层解释一致 | 其他未使用 header 未治理 |
| empty chunked/Trailer 声明绕过 | Nginx 在仍可见非空声明时拒绝 | 在仍有该事实的边界验证 | parser 早期错误未统一 JSON，空 Trailer 值与缺失不可区分，规则作用于全部 `/api` |
| oversized/slow body | 16 KiB、3s、framing gate | 限制 ingress 工作量 | 未来 body/upload API 必须重设 |
| SQL injection | 参数绑定 + canonical integer | 无字符串拼接 | 对象枚举仍存在 |
| runtime 误写 | table-level SELECT-only | DB 本身拒绝 DML | migrator Secret 仍高权限 |
| 坏存量数据 | domain Restore + max bound | fail closed，不随机修复 | 无自动隔离/修复 runbook |
| adapter 伪造结果 | Strategy/Award 全字段复核 | use case 不盲信 port | 同进程恶意代码仍可破坏更多状态 |
| entropy failure | fail closed 503 | 不变成 no_reward | 无 entropy health/audit |
| raw error 泄露 | stable class | 降低拓扑/SQL泄露 | 诊断能力有限 |
| API 停止 | JSON 502/504 | caller 得到明确 gateway failure | 无 HA/自动切换 |
| acceptance 误删资源 | 随机 project + Docker label/ID + temp directory identity/type verify | 缩小删除范围并在漂移时拒绝 | 同用户 TOCTOU 不是完全隔离的文件系统沙箱 |

### 12.4 为什么 feature flag 不是访问控制

flag 决定 route 是否注册，但一旦注册，任何能访问本地 edge 的 caller 都可调用。它没有 identity、policy、scope 或 audit。它是发布保险丝，不是 auth。

### 12.5 为什么 demo header 不是 CSRF token

它没有随机性、session 绑定、origin 绑定或服务端校验状态。它只让普通 HTML form 难以构造。若同源页面有 XSS，攻击脚本可以自由设置。

### 12.6 为什么隐藏 Weight 不是保密方案

对于重复独立抽样，样本频率会逐步估计真实分布。要保护商业概率，需要：

- 身份与授权；
- 次数/rate limit；
- 公开披露策略；
- 监控异常采样；
- 可能的分桶/个性化权限。

“DTO 没字段”只是最小暴露，不是信息论保密。

### 12.7 CSPRNG、不偏、公平、可审计是四件事

| 概念 | 当前是否具备 | 含义 |
| --- | --- | --- |
| 不偏 bucket mapping | 是 | 合法均匀 ticket 按 Weight 精确分配 |
| 难以预测的本地随机 | 是 | 默认 crypto/rand |
| 公平治理 | 否 | 配置、发布、监控和人员流程可信 |
| 可验证/可争议审计 | 否 | 第三方能重放或验证某次结果 |

不能把前两项写成后两项。

## 13. Nginx 边缘：代理不是透明管道

### 13.1 为什么必须真实经过 Nginx

`httptest` 能验证 Go handler，却不能证明：

- chunked 解码/正规化；
- client body limit；
- Host server selection；
- upstream 502/504；
- edge security headers；
- request ID 跨 proxy 注入；
- Docker 网络/端口拓扑。

首轮 acceptance 的空 chunked 200 正是单元测试不能发现的例子。

### 13.2 Request ID 的端到端设计

边缘先决定 ID：

```text
safe incoming 1..64 [A-Za-z0-9_.:-] → reuse
otherwise                          → nginx $request_id
```

再注入 Go。这样即使 Go 未返回 response header，Nginx 504 和 edge log 仍使用同一 ID。Go middleware 再验证，覆盖直连/bypass 情况。

### 13.3 为什么 Request ID 不是幂等键

它可能由客户端重用、边缘生成或只存在于日志，没有持久业务唯一约束。用它做幂等会混淆 observability identity 和 business identity。

### 13.4 Host allowlist 的边界

允许 localhost/127.0.0.1/[::1] 缩小本地 DNS rebinding/误路由风险。它不验证用户，不提供 TLS，不处理 trusted proxy，不能复制到 production 当主要访问控制。

### 13.5 16 KiB 为什么不是“接口允许 16 KiB”

route 仍不允许 body。16 KiB 是 edge 资源保险：不合规 caller 在到达 Go 前最多消耗有界 body。未来 JSON route 的真正 request schema/size 需要独立决定。

### 13.6 `proxy_request_buffering off` 的取舍

关闭完整缓冲让 Go 更早根据 framing 拒绝，不先把整个不合规 body 写入 Nginx buffer/temp file。代价是未来 upstream retry 与大 body 行为不同；本 route 无 body，当前更合适。

### 13.7 timeout 不能按名字误读

Nginx 官方说明 `proxy_read_timeout` 是连续两次 read 之间的 timeout，而非整个 response 总时长。`client_body_timeout` 和 `proxy_send_timeout` 同样是相邻 I/O inactivity 概念。

文档若写成“11 秒总超时”，将给 SLO 和故障分析错误结论。

### 13.8 access log 到底记录什么

Nginx 使用 `$uri`，所以会记录实际 path，包括 StrategyID；刻意不记录 `$args`/完整 request URI 和 Referer。它减少 query/Referer 泄露，但并非“没有业务 ID”。日志保留期、访问权限和脱敏仍需治理。

### 13.9 Nginx 与 Go 复制 error envelope 的代价

进入 API location 的 413/400-TE-or-Trailer/502/504 由 Nginx 拼 JSON，其他非 HEAD Go 错误由 Go 拼；421、HEAD body 和 parser 早期错误是例外。好处是 upstream 不可用时仍有统一 client shape；代价是：

- code/message 可能漂移；
- 新字段要改两处；
- JSON escaping/config quoting 更难；
- 需要 E2E 锁定。

若未来使用 API gateway/service mesh，可统一在一个受测试的 error policy，但不能假设代理默认会保持现有 envelope。

### 13.10 全 `/api` 规则的未来冲突

当前 TE 拒绝、16 KiB、body timeout 作用于整个 `/api` location。未来出现：

- 大 JSON；
- file upload；
- streaming；
- chunked event ingestion

时必须拆成 route-specific location/policy。不能为兼容新接口直接放松所有旧接口。

## 14. 数据库最小权限与发布/回滚

### 14.1 为什么本节撤掉 INSERT

第 19 节 Repository 实现了 `Create`，所以当时的学习集成身份需要 SELECT+INSERT。第 21 节在线 composition 只暴露 `StrategyReader`，runtime SQL 完全只读。

最小权限按**当前在线用例**推导，而不是按 package 中“可能调用的方法”推导。因此 app 收敛到两表 SELECT。

### 14.2 保留 Create 代码是否矛盾

不矛盾：

- package 保留历史已验收能力；
- production composition 没有 creator use case；
- DB grant 阻止潜在误调用；
- lesson-19 隔离集成测试可使用专门 writer identity。

这形成 application port + database grant 两层防线。

### 14.3 ReadOnly transaction 不是授权

`TxOptions.ReadOnly` 表达事务意图并让数据库优化/拒绝写，但不能替代 grant。若账号有 INSERT，应用 bug 仍可能在另一个普通 tx 写入。真正权限由 MySQL account/table grant 决定。

### 14.4 为什么 grant reconciliation 先 REVOKE 再 GRANT

旧 volume 不会重放 init script。只执行新的 GRANT 无法删除历史 INSERT/UPDATE 权限。reconciliation 必须：

1. revoke 旧直接权限；
2. grant 精确 allowlist；
3. sort/compare `SHOW GRANTS`；
4. 要求 mandatory roles 为空；
5. 不一致则阻止 API 启动。

### 14.5 mandatory roles 为什么必须检查

`SHOW GRANTS FOR CURRENT_USER` 的直接 grant 精确，不代表 server mandatory role 没有额外有效权限。要求为空避免“文本看起来最小，实际角色扩权”。

### 14.6 权限变更的回滚陷阱

二进制回滚不会自动恢复 grant。如果回滚到依赖 INSERT 的旧在线版本：

- process 可能启动；
- readiness SELECT 仍通过；
- 写业务才失败。

所以真实发布需要 binary/schema/grant compatibility matrix。最小权限不是单向越小越好，而是与部署版本精确匹配。

### 14.7 为什么 fixture 用 migrator

acceptance success 需要数据，但不能因此给 app 写权限。migrator 在一次性 schema 中原子插入 fixture，app 随后只读。这让“准备测试数据”和“在线权限”分开。

### 14.8 Secret 边界

app 只挂 app Secret，migrator 只挂 migration Secret，grant job 经 Unix socket 使用 root Secret且无网络。任何一个身份泄漏的影响范围不同；这比在代码里用一个 root DSN 更可审计。

## 15. 并发、资源所有权与性能诚实

### 15.1 service 是否并发安全

service 无可变字段，只持 interface 引用；并发安全条件是：

- Repository 使用并发安全 `sqlx.DB` pool；
- Strategy 是不可变值；
- WeightedSelector 不写共享状态；
- CryptoSource/Reader 可安全并发。

不能只说“service 没 mutex，所以肯定安全”，必须把依赖前提一起说。

### 15.2 为什么不加全局 mutex

mutex 会串行化所有 Strategy 请求，却不能提供业务幂等、库存一致性或公平性。当前共享组件本身并发安全；没有共享可变业务事实，不需要锁。

### 15.3 为什么没有 Redis 分布式锁

当前无持久结果/库存/次数，锁没有要保护的业务临界区。加锁只会：

- 增加依赖和故障模式；
- 让人误以为 exactly-once；
- 无法解决响应丢失后的结果查询；
- 可能在锁超时后重复选择。

真正 Draw 需要数据库唯一事实，分布式锁最多是流量协调，不是最终正确性的唯一来源。

### 15.4 连接池排队怎样受控

默认 pool 最大 10 个连接；超过的 handler 等待 connection。context deadline 会让 `database/sql` 等待/查询有机会取消，但：

- goroutine 在等待期间仍存在；
- 高请求率可耗尽 CPU/memory；
- 当前没有 admission/rate limit。

Award 上限控制单请求成本，不控制请求率。

### 15.5 64/100 goroutine 与“64 请求、并行度 16”证明什么

它们证明：

- 测试路径没有报告 data race；
- shared selector/source 返回配置内 Award；
- application unit 的 64 goroutine、HTTP unit 的 100 goroutine 保持结果边界；
- 真实 Compose 的总计 64 个请求在最大并行 16 下保持 contract；
- DB fingerprint 不变。

它们不证明：

- 生产高并发；
- 某个 QPS/P99；
- 随机样本独立；
- 负载稳定性；
- 多副本/跨地域行为。

### 15.6 为什么本节不设性能数字

没有真实用户模型、Strategy 数量、Award 分布、部署资源和 SLO。随意写一个本机 QPS 会把环境偶然性变成简历承诺。当前只保留可重复功能/竞态/隔离证据，为第 95 节性能优化留真实基线。

### 15.7 CryptoSource 的成本与选择

密码学随机比普通 PRNG 有更多系统/大整数成本，但当前每请求只调用一次，且风险是有价值营销结果被预测。没有 profile 证明它是瓶颈前，安全/正确优先。

## 16. 可观测性：相关日志不等于业务审计

### 16.1 当前已有的观察点

- safe Request ID；
- Gin access log：method、route pattern、status、duration；
- selection failure：request ID、canonical StrategyID、error class；
- Nginx log：实际 `$uri`、status、upstream status、timing；
- JSON error body 与 response header correlation；
- readiness/health；
- Docker service health。

### 16.2 当前没有的观察能力

- OpenTelemetry trace；
- handler/repository/selector metrics；
- in-flight/connection pool dashboard；
- business audit event；
- DrawResult history；
- entropy/algorithm proof；
- SLO/alert policy。

### 16.3 建议未来指标

低基数：

```text
lottery_ephemeral_selection_requests_total{result_class}
lottery_ephemeral_selection_failures_total{error_class}
lottery_ephemeral_selection_duration_seconds
lottery_repository_read_duration_seconds
lottery_selection_in_flight
lottery_request_rejections_total{code}
```

不应作 label：

- Request ID；
- StrategyID/AwardID；
- Award name；
- raw path；
- ticket/entropy；
- Weight。

高基数会造成成本和可用性问题；结果/概率指标还可能泄露商业配置，访问权限也需治理。

### 16.4 access log 为什么不是 audit log

它只能说明某次 HTTP 处理经过系统，不能证明：

- 结果持久；
- 用户身份；
- Award 未被篡改；
- Benefit 已交付；
- 可重放争议。

业务 audit 需要 durable identity、snapshot、actor、time、decision version 和不可抵赖/保留策略。

### 16.5 context canceled 是否都应告警

当前 cancel 映射 503/error class；但客户端主动断开、部署 shutdown 与依赖超时含义不同。未来 metrics 需区分并避免把大量用户取消当服务事故，同时不能丢掉真实 deadline。

### 16.6 readiness 的准确边界

`/ready` Ping MySQL，不能证明：

- 任意 Strategy 存在；
- grants 精确；
- schema/constraint 完整；
- CryptoSource 健康；
- Lottery route 已开启。

把所有业务检查塞进 readiness 可能造成不必要重启。grant/schema 由 startup jobs/smoke，业务数据由请求和监控验证。

## 17. 证据设计：测试是为了推翻命题

### 17.1 证据分层

| 证据 | 能支持 | 不能支持 |
| --- | --- | --- |
| application unit | 调用顺序、取消、结果复核 | MySQL/Nginx 真实行为 |
| HTTP unit | DTO、status、MaxUint64、日志脱敏 | proxy normalization/security headers |
| selector exhaustive | bucket mapping、MaxUint64 | 生产公平/审计 |
| race | 已执行路径无报告竞态 | 所有未来路径/QPS |
| Repository sqlmock | SQL 顺序/error mapping | MySQL 引擎语义 |
| MySQL integration | RR/read-only/grants/real errors | Nginx/HTTP |
| standard smoke | 长期栈、只读 404、port | 成功 selection fixture |
| isolated acceptance | 真实 Nginx→Go→MySQL→crypto | production TLS/auth/HA |
| DB fingerprint | 两张表本轮前后不变 | 外部系统副作用/长期审计 |
| API-stop test | gateway envelope/recovery | 自动 HA/SLO |

### 17.2 为什么 success fixture 不进 migration

生产 migration 应只表达 schema，不把课程演示业务数据变成永久事实。fixture 放在一次性 acceptance，并由 migrator 插入，结束后随 volume 删除。

### 17.3 为什么长期 smoke 只测动态 404

长期数据库属于学习者，不能假设空库，也不能插入/删除 fixture。smoke 通过只读 SQL推导一个真正缺失 ID，再验证：

- route 注册；
- request 到达 MySQL；
- not-found mapping；
- 不改用户数据。

固定 MaxUint64 是不稳健假设，`93f5694` 已修正。

### 17.4 为什么 fingerprint 覆盖所有业务列

仅比较 row count 会漏掉 UPDATE。脚本将 root/award 每个持久列形成排序 payload 后 SHA-256，调用前后相等，支持“本次 endpoint 没有修改两表”的结论。

它不证明未来代码永不写，也不证明数据库外没有副作用。

### 17.5 为什么 random Compose project 是安全边界

每次 96-bit suffix 防止名称碰撞，并将 image/volume/network/builder/secret/response 都绑定本次 identity。cleanup 校验 Docker label/ID、临时目录创建时记录的 device/inode 和子文件类型，避免仅凭字符串删除。它仍不是对抗同用户恶意并发替换的无竞态沙箱。

### 17.6 为什么不能执行 prune

Docker daemon 共享用户其他项目。`system prune`/`volume prune` 的删除集合不是本任务可证明所有权的资源；即使“通常安全”，也违反最小破坏范围。

### 17.7 cleanup 失败时为什么保留资源

如果 label/ID 漂移，强行清理可能删错；保留并报告比扩大破坏更安全。这是 fail closed 在运维脚本中的应用。

### 17.8 当前证据缺口

- flag 关闭后的 route absence 可增加直接 E2E；
- >1000 Award 的真实 DB read 可更强；
- slow MySQL/selector 经 Nginx 的完整 timeout path 尚未模拟；
- duplicate safe Request ID edge case 可扩展；
- 无认证、限流、TLS、审计、分布频率、生产 load 证据。

证据缺口不等于实现失败，但必须限制表述。

## 18. 候选方案与权衡矩阵

| 决策命题 | 当前采用 | 未采用 | 核心理由 |
| --- | --- | --- | --- |
| 何时开放 | dev/test ephemeral | 等完整 Draw | 先验证纵向链，用硬 gate 控制误用 |
| 行为方法 | POST | GET | 把它建模为显式 selection action，避免链接/预取/爬虫触发并为未来 Draw 演进留一致方向；不是用“随机响应不同”误证 unsafe |
| 结果状态 | 200 | 201/202/204 | 无资源、非异步、有 representation |
| 路径 | `ephemeral-selections` | `/draw`、`:select` | 命名诚实胜过短路径 |
| 输入 | path ID + 空 body | body strategy/seed | server 管理配置和随机 |
| 重试 | 拒绝 Idempotency-Key | 忽略/假装支持 | 无持久 key→result |
| identity encoding | decimal string | JSON number | 完整 uint64 |
| no_reward | 200 typed Award | nil/error/204 | 合法业务结果 |
| DTO | 显式最小 projection | domain 自动 JSON | 避免字段泄露/耦合 |
| Strategy read | RR 完整快照 | 多次无事务读/锁读 | 请求内一致且不阻塞写 |
| query shape | root + awards 两条 | JOIN/JSON aggregate | error/恢复边界更清晰 |
| selector port | application-owned | handler concrete dependency | consumer owns abstraction |
| runtime grant | SELECT-only | 保留 INSERT | 在线 SQL 只读 |
| caching | 每次 MySQL | 立即 Redis | 尚无 version/失效事实 |
| concurrency | 无全局锁 | mutex/Redis lock | 无共享可变业务事实 |
| ingress framing | Nginx + Go | 只在 Go | Nginx 会解码 TE |
| body handling | streaming/off buffering | 完整 buffer | 尽早拒绝无 body route |
| errors | 既有 envelope | 单 route RFC9457 | 平台一致性 |
| cause | stable class | raw SQL/driver | 安全优先 |
| feature gate | default off + prod hard reject | default on | demo 防误发布 |
| acceptance | 一次性随机 project | 复用长期 volume | 无污染、可精确清理 |
| performance | O(n) bounded | prefix/alias/map everywhere | 先有 profile，再引入派生状态 |

## 19. 架构师应主动发现的隐藏问题

这一节专门记录需求没有直接提出、但一个真实设计者应主动问的问题。

### 19.1 隐藏 Weight 仍可能被统计推断

最小 DTO 只阻止直接读取。无限调用者仍可估计概率，所以正式系统必须有 identity、次数和监控。

### 19.2 StrategyID 不是 public Activity identity

直接从浏览器指定 Strategy 会绕过发布态、受众和授权；第 30 节的建模不能被本 demo 路径绑死。

### 19.3 feature flag 只能防发布误操作

开启后没有访问控制；关闭策略也只在标准 appconfig/composition root 成立，直接测试调用 `RegisterRoutes` 可绕过环境政策。

### 19.4 success 没有 Strategy version

不同请求可能跨配置更新，结果无法历史解释。本节 RR 只保证单请求一致，不保证跨请求版本稳定。

### 19.5 Award name 未必适合用户展示

它可能是运营内部文案，尚无发布、本地化、内容审核、HTML/Unicode 欺骗和无障碍策略。第 22 节需转义并诚实展示，但长期应使用 public projection。

### 19.6 canonical ID 也影响安全

`01`/`1` 别名不仅是格式问题，还会让 cache、日志、签名、授权和幂等 key 分裂。

### 19.7 校验顺序会泄露信息

Go handler 先解析 ID，再检查 demo header。真实 auth 出现后，应先决定未认证 caller 是否能观察对象格式/存在性，不能原样沿用 demo 顺序。

### 19.8 Nginx 是语义参与者

它会解码 chunked、选择 server、限制 body、生成 gateway error；不能把 proxy 当透明 TCP 管道。

### 19.9 全 `/api` TE/size 策略会阻塞未来接口

上传/流式/大 command 到来时应拆 location，而不是放宽所有 route。

### 19.10 WriteTimeout 不等于 handler deadline

socket write budget不能抢占任意计算。业务 deadline 必须独立存在。

### 19.11 selector context 缺失是有条件债务

本地计算可接受；一旦 remote RNG/HSM 出现，必须升级 port，不应靠 Nginx 超时隐藏 goroutine。

### 19.12 context error 优先会隐藏 dependency cause

public contract 更稳定，但运维需要独立受控 telemetry 捕获同时发生的 underlying error。

### 19.13 503 容易诱导自动重试

SDK 可能默认重试 503；本 endpoint 重试会新选一次。client 文档和第 22 节前端必须禁用透明 retry。

### 19.14 no-store 不能阻止 caller 保存结果

它只约束 HTTP cache；截图、本地存储、日志和统计采样仍可发生。

### 19.15 Request ID 不是全局唯一业务事实

客户端可重用安全 ID；没有数据库 uniqueness。不能拿它恢复 selection。

### 19.16 Nginx/Go 两份 error template 会漂移

任何新字段或 code 变化都需 E2E；单元测试只覆盖 Go 一份。

### 19.17 502/504 不稳定会影响 SDK 策略

若 client 对 502 重试、对 504 不重试，Docker/network 差异会改变行为。正式 API 要统一“结果是否可能提交”的语义。

### 19.18 Grant 收敛影响回滚

权限属于 deployment version，不能只回滚 image；应有兼容矩阵和发布顺序。

### 19.19 Repository.Create 保留但 runtime 无 INSERT 是有意的

代码能力、application 暴露和数据库权限是三层不同边界，不必同步删除历史实现。

### 19.20 LIMIT 1001 不能完全限制 DB 扫描

索引选择、数据分布和 execution plan 仍重要；只限制返回给应用的聚合规模。

### 19.21 结果复核增加一次线性扫描

这是用 CPU 换 adapter contract 完整性。达到热点前不应为微优化取消防御。

### 19.22 concurrency test 不等于高并发能力

64/100 goroutine unit 与 64 次请求、最大并行 16 的 E2E 只覆盖正确性和 race，不提供生产容量数字。

### 19.23 CryptoSource 不等于公平系统

运营可配置权重、代码可替换 source、日志无审计；密码学随机只是一个组件属性。

### 19.24 readiness 不检查 Strategy 或随机源

这避免业务数据导致重启风暴，但意味着 ready=200 不能证明 Lottery success。

### 19.25 remote RNG 是否进入 readiness 是新决策

硬依赖进入 readiness 可能造成级联重启；不进入又可能所有选择 503。需基于降级和 SLO 决定。

### 19.26 `net.Error` retryable 分类较粗

现在项目层不显式 retry 尚可；`database/sql` 仍可能在事务建立前处理坏连接。未来加入业务重试前必须细分，防止权限/TLS 永久故障被无限重试。

### 19.27 client cancellation 可能制造告警噪声

应区分 caller abort、server deadline 和 shutdown，避免把用户离开页面当事故。

### 19.28 outcome metrics 可能泄露概率

即使 label 低基数，按 Strategy 下钻的 dashboard 也可能暴露商业数据；观测平台需要权限。

### 19.29 Nginx log 仍包含 StrategyID path

不记录 query/Referer 不等于不记录业务标识。需要保留期与访问控制。

### 19.30 acceptance 下载/构建依赖外部 registry

registry 500 会阻断验证，但不是业务失败；供应链 cache/digest/镜像镜像源是后续可靠性问题。

### 19.31 build cache 是否清理要区分所有权

一次性 builder cache 是任务创建，可删；长期可复用 Docker/Go/pnpm cache 不应因“清洁”而清空。

### 19.32 复用长期 volume 做验收会破坏学习者数据

因此 success fixture 必须在随机 project；长期 smoke 只读。

### 19.33 frontend 当前仍由 Math.random 决定结果

第 21 节后端完成不自动改变页面。第 22 节必须让服务端 response 决定 Award，动画只能展示，不能再次随机。

### 19.34 动画失败不能触发重新 selection

一旦 response 已到，UI 动画中断/刷新不应自动 POST 第二次来“恢复画面”；对 ephemeral 也会改变结果，对正式 Draw 更危险。

### 19.35 duplicate click 的 UI 语义

前端应在 in-flight 阶段禁用按钮，但这只是 UX，不是后端幂等。网络重试仍需明确禁用。

### 19.36 late response 的处理

若用户发出 A、又发 B，A 晚到可能覆盖 B 的 UI。第 22 节需要 request sequence/AbortController，只展示当前请求；这仍不是业务 Draw identity。

### 19.37 404 存在性在真实 auth 下可能需隐藏

未授权 caller 获得 404/403 的策略必须统一，避免枚举；本 demo 不能提前代表生产选择。

### 19.38 production Host/trusted proxy 完全不同

云 LB、TLS SNI、Forwarded header、trusted proxy CIDR 都需新部署设计，本地 allowlist 不可照搬。

### 19.39 正式 Draw 的 commit unknown 比本节复杂

当前没有写入，所以响应丢失只丢掉观察值；未来结果已 commit 但响应丢失，必须查询而非重抽。

### 19.40 最危险的简历夸大

“高并发抽奖、Redis 锁、幂等、库存、发奖闭环”都没有证据。准确描述反而能在面试中展示边界意识。

## 20. 正式 Draw API 的前置门

### 20.1 业务身份

1. public Activity/Campaign ID；
2. authenticated actor/user/device/tenant；
3. Activity→published Strategy version；
4. authorization 与对象隐藏策略。

### 20.2 参与与规则

5. 时间窗、受众、资格；
6. Participation/attempt identity；
7. 次数/积分/前置规则原子约束；
8. rate limit、反刷、风控。

### 20.3 结果事实

9. business request ID / Idempotency-Key；
10. DrawID；
11. 数据库唯一约束；
12. Strategy/Award/algorithm snapshot；
13. 原子 DrawResult；
14. request hash/conflict policy；
15. result query；
16. commit-unknown recovery。

### 20.4 奖励兑现

17. inventory reservation；
18. Benefit identity/state；
19. Outbox/MQ；
20. retry/compensation/reconciliation；
21. “selected / reserved / delivered” UI 状态分离。

### 20.5 生产治理

22. TLS/ingress/trusted proxies/WAF；
23. metrics/trace/SLO/alerts；
24. audit、保留、隐私；
25. 概率披露和争议处理；
26. load/capacity/failure/DR 验收；
27. API schema/version migration。

如果这些前置门尚未形成，给当前 response 加一个随机 UUID 也不会变成正式 Draw。

## 21. 假设与风险账本

| 当前假设 | 风险 | 失效信号 | 应对动作 |
| --- | --- | --- | --- |
| route 仅本地开发 | 被误部署 | 外网 ingress/真实用户 | 立即关闭，先补生产门 |
| StrategyID 可直接输入 | BOLA/绕发布 | 接 Activity/多租户 | 换 public identity + auth |
| Award name 可展示 | 内部文本/PII/本地化 | 真实运营内容 | public projection/review |
| 1000 Award 足够 | CPU/DB升高 | profile/P99/需求越界 | 重新建模和算法 |
| crypto source 本地快速 | 不可取消阻塞 | HSM/remote RNG | context-aware port/timeout |
| 每次 MySQL 可接受 | pool/DB瓶颈 | connection wait/latency | 先加 version，再设计 cache |
| 无 Lottery 业务状态写路径 | 当前重试无已实现的财务状态写入 | 接入次数/库存/Benefit | 先持久 Draw/幂等 |
| loopback/Host 足够 | 安全误用 | staging/production | TLS/auth/WAF/trusted proxy |
| 404 可公开 | 枚举 | 外部 API/tenant | auth-first 或 resource hiding |
| stable class 足够诊断 | MTTR 上升 | 事故难定位 | 受控 trace/diagnostic event |
| 502/504 同类即可 | SDK 策略分歧 | 自动 retry | 明确 outcome-unknown contract |
| error templates 同步 | edge/origin 漂移 | 新字段/code | E2E schema tests/统一 gateway |
| long-lived DB 可保留 | smoke 误写 | fixture/reset需求 | 只读 smoke + disposable acceptance |

## 22. 重新设计触发器

以下任一出现，应新增 ADR/变更 contract，而不是在当前 endpoint 内隐式扩张：

1. staging/production 要开启；
2. 接入真实用户或有价值奖励；
3. 客户端需要自动 retry/Idempotency-Key；
4. 需要结果查询或争议恢复；
5. 增加 Activity、发布态、受众或资格；
6. 增加次数、积分、库存、Benefit；
7. 增加认证/tenant/object authorization；
8. 增加 rate limit/反刷；
9. Award/Strategy 字段公开策略变化；
10. random source 远程化；
11. Award 数量上限变化；
12. MySQL/profile 表明需要 Redis/cache；
13. error envelope 整体迁移 RFC 9457；
14. Nginx 替换为云网关/service mesh；
15. 多副本、跨地域、生产 SLO；
16. 法规要求概率披露、可验证随机或审计。

## 23. 架构思考收束

第 21 节最重要的设计不是某一行 Gin route，而是几次有意识的拒绝：

- 拒绝把 Selection 叫 DrawResult；
- 拒绝用 GET 伪装随机行为；
- 拒绝用 201 伪装持久资源；
- 拒绝接受无法兑现的 Idempotency-Key；
- 拒绝暴露 Weight/operator name；
- 拒绝把 demo header 叫认证；
- 拒绝让 app 保留无需求的 INSERT；
- 拒绝用长期 volume 做 fixture；
- 拒绝把“64 次请求、最大并行 16”和 64/100 goroutine unit 叫高并发压测；
- 拒绝把 CSPRNG 叫公平审计。

一个成熟设计并不以“加入了多少技术”衡量，而以“每项承诺都有模型、实现和证据，未完成的承诺能被明确挡住”衡量。

准确的项目表述是：

> 在受控开发/测试环境中实现一个无 Lottery 业务状态写路径的 Selection 纵向切片，以 application-owned ports 组合 MySQL 一致快照和无偏密码学随机选择；建立严格 HTTP/uint64/error/context/Nginx/最小权限边界，并用随机隔离 Compose 环境验证成功、并发、故障恢复及两张业务表全列 fingerprint 不变。

## 24. 源码与证据入口

- [Application use case](../../../internal/lottery/application/ephemeral_selection.go)
- [Application ports](../../../internal/lottery/application/repository.go)
- [HTTP adapter](../../../internal/lottery/adapter/httpapi/selection.go)
- [MySQL Repository](../../../internal/lottery/adapter/mysqlrepo/repository.go)
- [Strategy bound](../../../internal/lottery/domain/strategy.go)
- [Weighted selector](../../../internal/lottery/domain/weighted_selector.go)
- [CryptoSource](../../../internal/lottery/adapter/randomsource/crypto.go)
- [Composition root](../../../cmd/growth-api/database.go)
- [Process route registration](../../../cmd/growth-api/main.go)
- [Configuration](../../../internal/platform/appconfig/config.go)
- [Shared HTTP middleware](../../../internal/infrastructure/httpapi)
- [Nginx edge](../../../deploy/docker/nginx.conf)
- [Compose topology](../../../deploy/compose/compose.yaml)
- [Grant reconciliation](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)
- [Disposable acceptance](../../../scripts/compose-lottery-api-acceptance.sh)
- [第 21 节课程](../../course/part-03/lesson-21-lottery-api.md)
- [第 21 节 API](../../api/lessons/lesson-21.md)
- [第 21 节 QA](../../qa/lessons/lesson-21.md)
- [ADR-0018](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)

## 25. 外部依据

技术结论优先使用规范/官方文档：

- [RFC 9110：HTTP Semantics](https://www.rfc-editor.org/rfc/rfc9110.html)
- [RFC 9111：HTTP Caching](https://www.rfc-editor.org/rfc/rfc9111.html)
- [RFC 9112：HTTP/1.1 message framing](https://www.rfc-editor.org/rfc/rfc9112.html)
- [RFC 8259：JSON](https://www.rfc-editor.org/rfc/rfc8259.html)
- [RFC 9457：Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html)
- [Go context](https://pkg.go.dev/context)
- [Go database/sql](https://pkg.go.dev/database/sql)
- [Go strconv.ParseUint](https://pkg.go.dev/strconv#ParseUint)
- [Go encoding/json](https://pkg.go.dev/encoding/json)
- [Gin documentation](https://gin-gonic.com/en/docs/)
- [Nginx proxy module](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)
- [Nginx core module](https://nginx.org/en/docs/http/ngx_http_core_module.html)
- [OWASP Input Validation](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)
- [OWASP REST Security](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html)
- [OWASP API Security Top 10 2023](https://owasp.org/API-Security/editions/2023/en/0x11-t10/)
- [Docker Compose project name](https://docs.docker.com/compose/how-tos/project-name/)
- [MySQL GRANT](https://dev.mysql.com/doc/refman/8.4/en/grant.html)
