# ADR-0020：Lottery Strategy Redis Cache-Aside 读取投影

- **状态：** 已接受
- **日期：** 2026-08-30
- **负责人：** GrowthOS 维护者
- **适用范围：** 第 24 节“第一次 Redis 缓存”
- **替代关系：** 不替代 ADR-0013、ADR-0016、ADR-0018 或 ADR-0019；将 ADR-0019 对 Strategy 可重建读取投影的约束落实为第一个 Redis 业务消费者

## 背景

第 17～23 节已经形成一条可真实调用但边界受限的 Lottery 纵向读取链：

- `Strategy` / `Award` 领域对象通过私有状态、构造器和恢复函数维护名称、身份、权重、Outcome、Award 数量与总权重不变量；
- MySQL 是当前 Strategy/Award 配置的持久化事实源，Repository 使用同一只读 Repeatable Read 快照恢复完整聚合；
- `EphemeralSelectionService` 依赖 application-owned `StrategyReader`，每次调用先从 MySQL 读取完整 Strategy，再交给纯 `WeightedSelector`；
- development/test 专用 HTTP API 和 React 页面真实消费该用例，但一次响应仍是不持久化、不可恢复、不可安全重试的 ephemeral selection；
- 第 23 节已经把用户资格、风险、访问授权、库存、正式 Draw/Result 和权益发放划给各自决定所有者，禁止把这些高时效或一次性事实混入 Strategy 缓存。

当前每次 ephemeral selection 都执行一次 MySQL 聚合读取。Strategy 已证明可跨请求复用且可以由两张 MySQL 表完整重建，因此成为首个有证据评估缓存的对象；但项目没有生产读写比分布，“读多写少”仍是待验证假设。与此同时，项目仍然没有 Strategy Update/Delete/Publish 运行路径、聚合业务版本、变更事件或正式 Draw。因此，本节不能以“接入 Redis”为理由发明精确失效协议，也不能把缓存升级为新的事实源。

第 16 节已经准备了隔离、认证且不持久化的 Redis 7.4 开发容器，但 API 当时没有加入 cache 网络，Redis 也不属于业务 readiness。那是“环境能力存在但没有业务消费者”的诚实切片。第 24 节出现首个真实消费者后，必须同时定义：缓存对象、key 与 value schema、TTL、命中和回源、坏值处理、并发回源、最小 ACL、超时、关闭、可观测与降级边界。

本 ADR 决定这些长期兼容和正确性约束。具体本机性能数值只由第 24 节 QA 的同环境实测产生，不在 ADR 中预先承诺。

## 决策驱动

1. MySQL 必须继续是 Strategy/Award 的唯一权威事实源，Redis 丢失后系统仍能重建缓存。
2. 缓存命中不得绕过领域恢复，也不得制造 Repository 当前不能返回的非法聚合。
3. 现有 `StrategyReader` 端口、Repository 错误分类、HTTP route/DTO/status/code 和 `no_reward` 语义不得为缓存改变。
4. Redis 是延迟与下游负载优化，不是业务正确性、资格、授权、库存或最终结果保证。
5. Redis miss、坏 payload、timeout、ACL 拒绝、OOM 或网络不可用不能被映射为 `no_reward`、Strategy not found 或业务拒绝。
6. 缓存 value 是跨进程、跨发布可见的外部格式，需要显式 schema version、严格解码、大小上限和回滚安全边界。
7. Strategy/Award ID 与 Weight 支持完整正 `uint64`，缓存格式不能收窄精度或依赖 JavaScript number。
8. 当前没有 Strategy 聚合业务版本或变更通知，只能诚实承诺 TTL 限定的最大陈旧窗口。
9. 同一热点 key 并发 miss 可能放大 MySQL 压力，需要在不引入分布式锁的前提下先合并单进程回源。
10. 缓存降级必须受 context、Redis 操作 timeout 和 Lottery 总请求预算约束，不能让非关键依赖拖垮主路径。
11. Redis 首个业务身份只能访问自己的 key namespace 和实际使用命令，不能沿用 `+@all ~*`。
12. 指标和日志必须保持低基数，不记录 payload、Secret、随机材料、用户事实或完整 Redis key。
13. 本节形成的是本机 Strategy 读取/临时选择 M1 基线，不是正式 Draw、生产容量或业务 SLO 证明。

## 非目标

本 ADR 不决定或实现：

- Strategy Update、Delete、Upsert、Publish、草稿、审批、乐观锁或聚合业务版本；
- MySQL 与 Redis 双写事务、CDC、Binlog 订阅、Outbox、消息失效或跨地域失效；
- 用户、会员、设备、租户、画像、风险原始特征或 Participation 资格缓存；
- 访问控制的 Identity、Principal、Role、Permission、Resource、Action、Scope 或 Policy Decision 缓存；
- Activity 时间窗、次数、额度、频控、黑白名单或规则链中间结果缓存；
- 库存可用性、库存预占、权益模板、Benefit 发放状态或补偿状态缓存；
- 随机 ticket、选中的 Award、ephemeral selection、正式 Draw/Result、幂等响应或结果未知状态缓存；
- not-found、依赖错误或任意业务拒绝的负缓存；
- Bloom Filter、Redis 分布式锁、Lua、Redlock、软/硬双 TTL 或 stale-while-revalidate；
- Redis RDB/AOF、Sentinel、Cluster、跨区域复制或生产灾备；
- 通用 `common/cache`、跨上下文万能 Cache 接口或让领域对象直接依赖 Redis/JSON；
- 修改 Lottery HTTP route、请求/响应 DTO、公开 header、错误 envelope 或 React 页面；
- 将 Redis 加入业务 readiness；
- 10,000 RPS、生产 P99、生产可用性、缓存强一致或完整在线抽奖闭环结论。

## 评估过的方案

### 方案一：继续每次直接读取 MySQL

| 优点 | 代价 / 风险 |
| --- | --- |
| 语义最简单，没有第二份派生状态 | 每次 selection 都执行根与 Awards 查询及领域恢复 |
| 读取始终来自权威快照 | 热 Strategy 会重复消耗连接池、事务和数据库 CPU |
| 无 key、TTL、codec 和 ACL 成本 | 无法形成第 24 节要求的缓存故障与性能学习闭环 |

**结论：不作为第 24 节终态。** 该方案继续作为 cache bypass 和故障降级路径，并作为 M1 对照基线。

### 方案二：使用进程内 `map` / LRU 缓存

| 优点 | 代价 / 风险 |
| --- | --- |
| 无网络往返，单进程实现较少 | 每个实例拥有不同副本与冷启动，实例数放大下游加载 |
| 不增加外部依赖 | 容量、淘汰和发布清空都与进程耦合 |
| 类型可以直接保存领域对象 | 无法验证已经准备好的 Redis 网络、ACL、序列化与故障边界 |

**结论：暂不采用。** 当前目标不仅是减少一次进程内构造，还要建立可跨实例共享、可丢弃、可版本化的外部读投影协议。以后若 profile 证明 Redis RTT 高于收益，可基于证据评估 L1 + Redis L2，而不是在本节直接引入多级缓存。

### 方案三：在 MySQL Repository 内部直接加入 Redis

| 优点 | 代价 / 风险 |
| --- | --- |
| 调用方无感，表面只有一个 Repository | MySQL adapter 同时拥有 Redis、codec、TTL、回源与降级 |
| 容易复用现有 `FindByID` | Repository 测试和错误语义被缓存实现污染 |
| 少一个组合类型 | 未来更换缓存或绕过缓存需要修改权威存储 adapter |

**结论：不采用。** MySQL Repository 继续只实现权威持久化端口；Strategy cache 作为 `StrategyReader` decorator 组合在 application consumer 与 MySQL reader 之间。缓存与权威读取可以独立测试、启用、禁用和撤销。

### 方案四：Redis Cache-Aside `StrategyReader` decorator

| 优点 | 成本 / 风险 |
| --- | --- |
| 保留 application-owned 窄端口和 MySQL sole truth | 增加 Redis client、codec、TTL、ACL 与运行生命周期 |
| hit 可跳过 MySQL，miss/坏值/不可用可显式回源 | 缓存可能陈旧，错误处理不当会形成 poison value |
| 可独立启停与回滚 | 并发 miss 可能形成 thundering herd |
| 不改变 selector、HTTP 或领域对象 | 需要为外部 payload 维护 schema 兼容 |

**结论：采用。** 本节实现一个 Lottery-owned Strategy 读取投影 cache-aside adapter，由组合根把它装配为 `EphemeralSelectionService` 的 `StrategyReader`。

### 方案五：Write-Through / Write-Behind 或 MySQL 与 Redis 双写

| 优点 | 代价 / 风险 |
| --- | --- |
| 理论上可在写入时立即准备缓存 | 当前运行时根本没有 Strategy 写/更新/发布用例 |
| 可能减少首次 miss | Redis 写失败、MySQL commit unknown、顺序和补偿都未定义 |
| write-behind 可降低数据库写延迟 | 会把可丢弃缓存变成事实暂存，违反 MySQL sole truth |

**结论：拒绝。** 当前只使用 cache-aside 读取和 best-effort fill。未来出现正式写路径时，必须另立 ADR 定义业务版本、失效顺序、并发陈旧写回和恢复，不能偷偷把本决策扩成双写。

### 方案六：缓存选择结果、用户判断或 not-found

| 优点 | 代价 / 风险 |
| --- | --- |
| 可能进一步减少计算或依赖调用 | 一次随机结果缺少请求身份与持久事实，重复读取会伪造幂等 |
| 负缓存可减少不存在 ID 穿透 | 新创建 Strategy 在 TTL 内可能被错误隐藏，当前也没有发布版本 |
| 用户 verdict 可降低资格查询 | 主体、时间、风险、额度、授权与隐私失效协议均未建立 |

**结论：拒绝。** 本节只缓存可由 MySQL 完整重建的 Strategy 配置投影；同一次 singleflight 可以共享未持久化的 source error，但 error/not-found 不写入 Redis，也不在 group 结束后继续保留。

### 方案七：用 Redis 锁、Lua 或分布式 singleflight 保护回源

| 优点 | 代价 / 风险 |
| --- | --- |
| 可以跨 API 实例减少同 key 回源 | 需要锁身份、租约、续期、释放、超时和 fencing 语义 |
| 热 key 失效时数据库压力更低 | Redis 正是发生故障时，锁依赖也可能放大失败 |
| Lua 可原子比较与删除 | 扩大 ACL、脚本和可观测面，当前单实例 M1 证据不足 |

**结论：暂不采用。** 本节只做进程内、同 key 的回源请求合并。多实例回源压力必须先通过后续真实副本和负载证据证明，再单独比较分布式协调、提前刷新、软/硬 TTL 或数据库保护。

## 决策

### 1. MySQL 是唯一事实源，Redis 是可丢弃读取投影

1. `lottery_strategy` 与 `lottery_strategy_award` 的同一 Repository 快照继续定义当前可读取 Strategy；
2. Redis 不接受独立业务写入，不拥有 Strategy Create/Update/Delete/Publish 能力；
3. 删除所有 Redis key、Redis eviction、Redis restart 或整个缓存实例丢失，都不能造成业务事实丢失；
4. cache miss 后只能调用现有 `StrategyReader.FindByID` 权威读取，不能从前端、Mock、日志或另一个缓存猜测 Strategy；
5. cache hit 只代表找到一个通过格式与领域恢复的派生投影，不代表它是最新发布版本或正式 Draw 引用快照；
6. Redis 与 MySQL 同时不可用时必须返回权威读取的技术失败，不能返回默认 Award、空 Strategy 或 `no_reward`。

这里的 cache **fail-open** 仅指“非权威 Redis 失败时绕过缓存并尝试 MySQL”。它绝不表示授权、资格、风险、额度、库存或业务规则可以 fail open。

### 2. 只缓存完整 Strategy + Awards 投影

每个 value 必须足以独立调用领域恢复函数重建一份完整 Strategy，至少包含：

- cache payload schema version；
- StrategyID 与 Strategy name；
- 该 Strategy 的完整 Award 集合；
- 每个 Award 的 AwardID、name、weight 与 outcome。

不缓存：

- SQL row、事务句柄、Repository error/cause；
- `WeightedSelector` 内部结构、累计权重表、alias table 或随机 source；
- HTTP DTO、页面展示状态或浏览器本地角色；
- User、Membership、Device、Tenant、Risk 或 Participation 事实；
- eligibility、authorization、quota、frequency、rule trace 或 policy decision；
- inventory availability、reservation、Benefit delivery 或 compensation；
- random ticket、selected Award、ephemeral selection、Draw、Result 或幂等响应。

Strategy 配置与上述对象拥有不同的事实所有者、时效、并发、隐私和恢复语义。字段看起来“以后可能有用”不是把它加入 cache payload 的理由。

### 3. 使用版本化、环境隔离且规范化的 key

第一个 key schema 固定为：

```text
growthos:<environment>:lottery:strategy:projection:v1:<strategy_id>
```

约束：

1. `<environment>` 只能来自已经验证的 `development|test|staging|production` 配置枚举，不能来自请求参数；
2. `<strategy_id>` 使用 `strconv.FormatUint(id, 10)` 的规范十进制形式，禁止前导零、符号、空白、其他进制或原始 path 拼接；
3. `v1` 是 cache payload/key schema version，不是 Strategy 业务版本、Migration version 或应用 Git SHA；
4. key 不包含用户、会员、IP、设备、权限、Award name、Strategy name 或随机结果；
5. 新 schema 使用新 key version，让旧 key 自然过期；不得原地改变 v1 value 含义；
6. 当前不依赖 Redis Cluster hash tag、slot 或跨 key 事务，未来引入 Cluster 时另行评审 key placement。

生产环境、测试环境和开发环境不能共享同一命名空间。当前 Compose ACL 只授权它实际承载的 development prefix；其他环境必须各自配置最小 key pattern，不能用 `growthos:*` 代替环境隔离。

### 4. 使用严格、独立、最大 2 MiB 的 v1 codec

缓存 adapter 定义独立 payload DTO，不给 `domain.Strategy` / `domain.Award` 增加 JSON tag，也不直接序列化私有领域状态。

v1 codec 必须满足：

1. value 最大 `2 MiB`（`2 * 1024 * 1024` bytes）；编码结果超过上限时不写缓存，读取时用有界 `GETRANGE 0..2MiB` 最多取回“上限 + 1 byte”，超过上限时在 JSON 解码前判为无效；
2. JSON 只接受一个完整 document，拒绝 trailing token；
3. 拒绝未知字段、缺失必填字段、重复语义字段和未知 schema version；
4. StrategyID、AwardID 与 Weight 使用规范十进制 string，无损覆盖完整正 `uint64`；
5. payload StrategyID 必须与 key/request StrategyID 一致；
6. Award 数量必须在 `1..1000`，与 `domain.MaxAwardsPerStrategy` 现有边界一致；
7. AwardID 重复、非法名称、零 Weight、总权重溢出或未知 Outcome 都是无效 payload；
8. Award 编码顺序使用领域规范顺序，不能依赖 map 或 SQL 偶然顺序；
9. 解码完成后必须调用 `domain.RestoreAward` / `domain.RestoreStrategy` 等既有恢复边界；
10. codec error 只暴露稳定低基数类别，不渲染 value、名称或底层解析片段到公开响应。

`2 MiB` 是缓存 payload 防御上限，不是产品承诺的常见 Strategy 大小。真实 value 分布、内存与网络证据出现后可以收紧；扩大上限必须重新检查 Redis 内存、客户端 buffer、解码 CPU 和单请求预算。

### 5. 采用 Cache-Aside，并区分所有读取状态

读取控制流固定为：

```text
FindByID(ctx, strategyID)
  -> build canonical key
  -> bounded Redis GETRANGE within cache-operation budget
     -> hit + strict decode + domain restore -> return Strategy
     -> miss -> enter same-key source-load group
     -> invalid/unsupported/oversized payload
          -> best-effort DEL
          -> enter same-key source-load group
     -> Redis technical error
          -> record bounded cache bypass
          -> enter same-key source-load group
  -> authoritative MySQL reader
     -> success -> strict encode -> best-effort SET with TTL -> return Strategy
     -> not found/error/cancel -> return source semantic; do not write Redis
```

具体规则：

- Redis nil/missing 是 cache miss，不是 Strategy not found；
- GETRANGE timeout、连接失败、ACL 拒绝、wrong type、OOM 相关错误或 Redis server error 都进入 cache bypass；
- 未知 schema、坏 JSON、超大 value、ID 不匹配或领域恢复失败视为 poison/invalid cache，best-effort 删除后回源；
- DEL 失败不能阻止回源，但必须留下低基数诊断；
- MySQL 成功而 codec/SET 失败时，本次调用仍返回 MySQL Strategy；
- MySQL not found、存储损坏、普通失败、retryable、cancel/deadline 保持现有语义，不由先前 cache 错误覆盖；
- 任意 cache failure 都不能创造 fallback Award、空 Awards 或 `no_reward`；
- caller context 已取消时不得把缓存命中或后续回源包装成成功，也不得启动无界后台填充。

### 6. TTL 基准默认 5 分钟、上限 5 分钟，并减去 0～10% 抖动

每次成功 fill 的物理 TTL 为：

```text
appconfig base_ttl = validated value in [1s, 5m], default 5m
internal decorator = validated value in [1ms, 5m] for focused tests
jitter   = [0, base_ttl * 10%]
ttl      = base_ttl - jitter
```

因此每个 key 的初始 TTL 必须位于闭区间 `[base_ttl * 90%, base_ttl]`；默认配置对应 `[4m30s, 5m]`。TTL 必须通过同一个原子 `SET` + expiration 写入，禁止出现无 TTL 的永久 Strategy key。配置只允许缩短当前陈旧上界，不能把基准调到 5 分钟以上。

决定含义：

1. TTL 抖动只降低大量 key 同时创建后集中到期的概率，不解决单个极热 key、Redis 整体故障或多实例同时回源；
2. 抖动是运行机制随机量，与 `WeightedSelector` 的业务随机 source 隔离，不能影响 Award 选择或进入业务日志；
3. 抖动边界必须可注入/可确定测试，不能用 sleep 猜测；
4. 当前没有 Strategy Update/Publish 运行路径，故只承诺最多当前 key 剩余 TTL 的有界陈旧，不承诺 read-after-external-write；
5. 不使用 soft TTL/hard TTL，不在 MySQL 不可用时继续服务已经物理过期的值；
6. Redis restart、eviction 或手工删除都等价于 miss，并由 MySQL 重建。

当前默认 5 分钟也是允许的最大值，不是永恒业务常量。operator 可以基于更严格的陈旧要求缩短它；命中率、变更频率、可接受陈旧窗口、内存和回源压力出现真实数据后，若要提高 5 分钟上限，必须通过新决策重新评估，不能为了漂亮命中率直接延长。

### 7. 同 key 只做进程内 singleflight 回源合并

cache miss、坏值或 Redis bypass 后，以 canonical Strategy key 对权威 reader 调用做进程内请求合并：

1. 同一 API 进程、同一 key、重叠时间窗口内最多保留一个进行中的 MySQL source load；
2. 不同 Strategy key 互不阻塞；
3. cache hit 不进入 group；
4. group 只共享 Strategy source load 的返回值/error，不共享 `WeightedSelector.Select`；每个 ephemeral invocation 仍独立执行一次随机选择；
5. group 可以让并发 not-found/error 等待者观察同一次 source 结果，但结果不写 Redis，也不形成跨请求负缓存；
6. 等待调用必须继续遵守各自 context；取消的 caller 不能被返回成功，也不能无限等待 leader；
7. source load 和 cache fill 都保持有界，不创建无生命周期 owner 的后台 goroutine；
8. 本机制不跨进程、不跨主机、不提供锁、租约、fencing、幂等或 exactly-once。

如果未来多副本压测证明同一 key 在 TTL 到期时仍压垮 MySQL，应以实际下游查询增量重新评估提前刷新、软/硬 TTL、跨实例协调或数据库限流，不得把本地 singleflight 宣称成分布式防击穿。

### 8. Redis 不可用时 fail-open 到 MySQL，且不进入 readiness

Redis 是可选加速层：

- 配置语法、必填 Secret 文件、username、地址、TTL、timeout 等若无效，应在配置加载/组合阶段失败，避免把 operator mistake 静默当网络降级；
- 配置有效但 Redis 连接、认证或命令在运行时失败时，请求绕过缓存读取 MySQL；
- API 启动不以 Redis `PING` 成功作为接收流量前提，`GET /ready` 继续只检查当前权威 MySQL 依赖；
- Redis service 可以拥有自己的容器 healthcheck，但它不改变 growth-api readiness 响应；
- Redis 操作使用显著小于 Lottery 总 selection timeout 的独立 budget，并限制客户端自动 retry，避免每个请求先长时间等待 Redis 再访问 MySQL；
- Redis 故障期间的每请求回源会增加 MySQL 压力，M1 必须测量 cache-disabled/Redis-down 基线；本节不声称已经有完整 circuit breaker、负载卸载或多副本 backpressure；
- MySQL 不可用而 Redis 有合法未过期 hit 时，reader 可以返回该投影，但 `/ready` 仍准确报告 MySQL 不可用；这不能外推为系统满足业务可用性目标；
- Redis 和 MySQL 都不可用，或 cache value 非法且 MySQL 不可用时，返回现有技术失败，不服务坏值或伪造结果。

缓存降级不能覆盖 context cancellation，也不能把授权/资格/库存依赖错误套用本节的 fail-open 原则。

### 9. 不做负缓存

本节不把下列对象写入 Redis：

- Strategy not found；
- MySQL timeout/retryable/failure；
- stored Strategy invalid；
- codec error、Redis error 或 source cancel；
- 空 Strategy、空 Award 集合或默认错误对象。

理由：当前没有公开 Activity identity、Strategy 发布版本、可信不存在证明或创建后失效事件。负缓存会让新创建但此前查询过的 Strategy 在 TTL 内继续 404，也可能把临时 MySQL 故障固化为业务不存在。

重复请求未知 ID 仍可能穿透 MySQL，这是明确剩余风险。出现真实枚举流量、not-found 比例和权威存在集合后，再比较短 TTL negative cache、输入准入、Bloom Filter 或边缘限流；不得在没有身份和滥用模型时先堆技术。

### 10. Redis 业务身份使用最小 ACL

首个业务消费者出现后，不再允许沿用 default user 的 `+@all ~*`。开发 Compose 至少满足：

1. 使用独立命名用户和 Secret，default user 关闭或不能执行业务命令；
2. key pattern 精确限制为 `growthos:development:lottery:strategy:projection:v1:*`；
3. 只授权连接/客户端握手实际需要的最小命令，以及 `PING`、有界读取所需的 `GETRANGE`、带 expiration 的 `SET`、清理坏值所需的 `DEL`；
4. 不授权 `KEYS`、`SCAN`、`FLUSHDB`、`FLUSHALL`、`CONFIG`、`ACL`、`EVAL`、`SCRIPT`、Pub/Sub、Stream、持久化管理或任意 key pattern；
5. API 不拥有读取 Secret 文件之外的 Redis 管理凭证；Web、MySQL、Migration 与浏览器不持有 Redis Secret；
6. Redis 与 API 只通过项目 internal cache network 连接，不发布 Redis host port；
7. ACL 文件、进程参数、日志和错误响应不得回显密码；
8. Redis logical database 固定为 0；appconfig 与 adapter 都拒绝非 0，因为 go-redis 会为非 0 DB 在连接初始化时发送本 ACL 未授权的 `SELECT`；
9. acceptance 通过管理/测试边界验证业务用户允许项和禁止项，不能以配置文本存在代替真实授权测试。

未来加入另一个 Redis 消费者时，应为它定义独立用户、keyspace 与命令集合。共享 Redis 进程不等于共享业务权限模型，也不属于第 31～35 节面向产品主体的 RBAC。

### 11. 连接池和生命周期属于 composition root

1. Redis client/pool 由进程组合根创建并拥有，Strategy cache adapter 借用 client，不在每次请求创建连接；
2. MySQL pool 与 Redis client 是两个独立资源，不能因为关闭一个而跳过另一个；
3. startup 任一后续组合步骤失败时，已经创建的资源必须按所有权关闭；
4. shutdown 必须关闭 HTTP server、Redis client 和 MySQL pool，并分别记录低基数关闭错误；
5. cache adapter 的零值、nil/typed-nil client、nil source reader、非法 codec/clock/jitter 配置在组合阶段失败；
6. client timeout、pool size 和 retry 必须有验证范围，不能依赖库默认值后在文档里声称已经调优；
7. 不从普通业务 handler 暴露 Redis client，也不允许 handler 直接拼 key 或执行命令。

这延续了 MySQL Repository 不拥有共享 pool 的现有原则：资源生命周期由创建者负责，领域与 application port 不导入具体基础设施 client。

### 12. HTTP 与领域语义保持不变

引入 Strategy cache 不改变：

- `POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections`；
- required demo header、canonical StrategyID、无 query/body 和 Idempotency-Key 拒绝；
- `200` response DTO、十进制 string identity 与 `durability: ephemeral`；
- HTTP `Cache-Control: no-store`；它约束客户端/中间 HTTP cache，不与服务端 Redis Strategy 投影冲突；
- Strategy not found、stored invalid、dependency failure、context timeout、selector failure 和 `no_reward` 映射；
- `WeightedSelector` 输入、概率和每调用随机性；
- React adapter、Hook、页面状态与前端 API decoder。

不新增 `X-Cache`、Redis latency、TTL、internal key、source 等公开 response 字段/header。前端不能依赖是否命中缓存决定业务文案或重试。

### 13. 当前一致性只承诺 TTL 有界陈旧

当前运行路径没有 Strategy 更新、删除或发布，因此本 ADR 不定义“写数据库后删除缓存”的执行顺序，也不使用根表 `updated_at` 充当聚合 version：Award 子行变化不会可靠推进根行时间，时间戳也不提供单调 CAS 语义。

当前承诺只有：

- value 成功写入后最多存活 5 分钟；默认基准下初始 TTL 为 4 分 30 秒到 5 分钟，缩短配置时按相同比例收缩；
- 缓存删除、过期、淘汰或 Redis 重启后由 MySQL 当前快照重建；
- 外部越过应用直接修改 MySQL，旧投影可能服务到该 key 剩余 TTL；
- schema/key version 变化通过新 namespace 隔离，而不是原地 reinterpret；
- 不保证 read-after-external-write、跨请求单调读、跨实例同时刷新或历史版本回放。

第一次真实 Strategy Update/Publish/Delete 出现前，必须新增 ADR，至少决定：聚合业务版本、不可变发布快照或 CAS、写与失效顺序、失败恢复、并发旧值回填、事件/CDC、灰度兼容和运行中 Activity 的引用。该未来 ADR 可以 supersede 本节 TTL-only 一致性部分，但不得回写本历史切片。

### 14. 可观测性保持低基数并区分 cache 与业务结果

至少区分以下有限状态：

- `hit`；
- `miss`；
- `invalid_payload`；
- `redis_unavailable` / `bypass`；
- `source_load`；
- `source_shared`；
- `fill_success`；
- `fill_failure`；
- `delete_failure`。

观测要求：

1. cache outcome、operation、稳定 error class 和耗时使用有限枚举；
2. 不把 StrategyID、AwardID、名称、完整 key、payload、Redis address、username、Secret、底层协议错误或随机材料作为 metrics label；
3. 普通日志不记录 value，不输出可用于重建概率配置的完整 Awards/Weights；
4. `no_reward`、reward、repository failure、selector failure 与 cache miss/error 分开统计；
5. M1 验收同时记录 Redis hit/miss、MySQL source load 和端到端延迟，不能只因 P99 变化推断缓存生效；
6. 日志采样/限频必须防止 Redis 长时故障时每请求刷屏；完整 OpenTelemetry、Dashboard 和告警仍由后续可观测章节承接。

StrategyID 可以在受控 trace/诊断上下文中按既有数据分级策略关联，但不能因此变成无界指标维度或公开响应字段。

## 信任边界

### MySQL 到 Redis

只有通过 MySQL Repository 和领域恢复得到的合法完整 Strategy 才能进入 encoder。Redis 不是复制协议、事务参与者或事实确认者；SET 成功不能反向证明 MySQL 状态。

### Redis 到 application

Redis value 是不可信外部输入。即使只有内部网络和 ACL，旧版本、手工写入、内存损坏、错误客户端或未来部署漂移仍可能产生 poison value。必须执行大小、schema、字段、identity 和领域不变量校验。

### 客户端到 cache key

客户端只提交受现有 HTTP adapter 规范化的 StrategyID。environment、namespace、schema version 和 Redis username 都来自服务端受控配置；客户端不能提交 raw key、TTL、cache bypass、payload 或 verdict。

### Cache 到 Lottery selection

缓存只提供一份合法 Strategy 输入。每次 `EphemeralSelectionService.Select` 仍独立调用 `WeightedSelector`，不能从缓存复用上一请求 Award、随机位置或 Outcome。

### Cache 与访问控制

Redis ACL 保护基础设施 keyspace，未来 GrowthOS RBAC 保护产品主体访问业务资源。两者目标不同：Redis 密码不能当用户会话，页面角色不能授权 Redis，业务权限决定也不得进入本 Strategy cache。

## 关键控制流

```text
ephemeral HTTP request
  -> existing transport validation and request deadline
  -> EphemeralSelectionService.Select
  -> cached StrategyReader.FindByID
       -> canonical key
       -> bounded Redis GETRANGE
          -> valid hit: strict decode + domain restore
          -> miss/error/invalid: same-key in-process source group
               -> MySQL Repository read-only RR snapshot
               -> strict encode
               -> best-effort Redis SET with [90% of base, base] TTL (base <= 5m)
  -> each request independently calls WeightedSelector
  -> existing response/error mapping
```

Redis failure 只改变读取成本和观测状态，不改变 Strategy、Award、`no_reward` 或 HTTP 业务语义。

## 必须保持的约束

1. MySQL 始终是当前 Strategy/Award 的唯一权威事实源。
2. 只缓存完整、可重建的 Strategy + Awards 投影。
3. cache key 必须包含 namespace、受控 environment、projection/schema version 与规范 StrategyID。
4. payload schema version 不得冒充 Strategy 业务版本。
5. 每个 value 最大 2 MiB、Award 最大 1000，并经严格 codec 与领域恢复。
6. Redis miss/error/坏值不得映射为 `no_reward`、not found 或资格拒绝。
7. MySQL 成功后 cache fill 失败不改变本次成功；MySQL 失败不被 cache error 覆盖。
8. TTL 必须在 `[base_ttl * 90%, base_ttl]`，且 `base_ttl` 不得超过 5 分钟；不允许无过期 Strategy key。
9. singleflight 只合并同进程同 key 的权威回源，不合并 selector、不跨实例、不提供锁或幂等。
10. not-found、error、资格、授权、库存、随机结果、Draw/Result 都不得写入本缓存。
11. Redis 运行故障 fail-open 到 MySQL，但无效配置/Secret 不静默降级。
12. Redis 不加入 API readiness，Redis service health 与业务 readiness 保持独立。
13. Redis 业务身份使用最小 keyspace/command ACL，不允许 default `+@all ~*`。
14. HTTP contract、`Cache-Control: no-store`、Repository error 和 domain/selector 语义保持不变。
15. 当前只承诺 TTL 有界陈旧；未来精确失效、业务版本和写并发必须另立 ADR。
16. metrics/log 保持低基数，不泄露 key、payload、Secret、PII、权重或随机材料。

## 影响

### 正面影响

- 热 Strategy 命中时跳过 MySQL 事务和两次表读取；
- MySQL Repository、application port、domain 与 HTTP adapter 保持单一职责；
- Redis 丢失、重启和淘汰不会丢失权威业务事实；
- 坏缓存经过严格 codec 与领域恢复，不能静默制造非法 Strategy；
- Redis 故障可以回源，不把非关键依赖变成硬 readiness；
- 默认 5 分钟、最大 5 分钟并减抖动的 TTL，给当前无更新路径一个诚实、可测且可向下收紧的陈旧上界；
- 进程内 same-key 合并降低单实例冷 key/过期 key 的并发回源；
- 最小 ACL 将第 16 节的基础设施占位收敛为真实业务身份；
- key/payload version 为后续滚动发布和 schema 前向演进保留隔离边界；
- M1 可以分别测量 direct MySQL、warm cache 与 Redis-down 降级路径。

### 成本

- 新增 go-redis client、连接池配置、Secret、cache network 和关闭生命周期；
- 新增外部 payload schema、key contract、TTL 和 codec 测试维护；
- 每次 miss 先付出 Redis GETRANGE，再付出 MySQL 读取和 best-effort SET；
- fail-open 使 Redis 故障期间每请求仍可能承受一次短 Redis 失败成本；
- singleflight 引入等待、leader 失败共享和 context 协调复杂度；
- 无业务 version/失效事件时允许有限陈旧；
- 严格 codec 与领域恢复会增加少量 hit CPU，但换取不变量安全；
- 2 MiB 上限和 1000 Award 仍需要真实分布与内存证据持续校准。

### 风险与缓解

| 风险 | 当前缓解 | 剩余边界 |
| --- | --- | --- |
| 缓存投影陈旧 | base TTL 不超过 5m、实际 TTL 为其 90%～100%、无永久 key | 不保证 read-after-external-write |
| poison/未知 payload | 有界 GETRANGE、2 MiB、strict schema、ID 对照、领域恢复、best-effort DEL | Redis server/client 连接与数据集内存仍需限制 |
| 热 key 同时 miss | 单进程 same-key singleflight | 多实例 herd 尚未解决 |
| Redis outage 放大延迟 | 短 operation budget、受限 retry、fail-open MySQL | 尚无完整 breaker/backpressure |
| MySQL 在 Redis 故障时过载 | M1 cache-disabled/Redis-down 测试 | 正式容量和自动 shedding 未实现 |
| 大量不存在 ID 穿透 | canonical ID、现有受限 demo surface | 无 negative cache/Bloom/rate limit |
| ACL 扩权 | named user、exact prefix/commands、真实负向测试 | 未来新增命令需重新收敛 |
| 旧进程与新 schema 混跑 | v1 同时进入 key 与 payload，新 schema 新 namespace | 跨版本 rollout 仍需兼容测试 |
| cache hit 掩盖坏 MySQL 新状态 | TTL 后重新读取；MySQL 仍 sole truth | TTL 内可能继续服务旧合法投影 |
| 日志/指标基数和泄露 | 有限 outcome/error class，不记录 payload/key/权重 | 完整 OTel 与告警后续实现 |

## 迁移、发布与撤销

本节没有数据库 Migration。发布顺序为：

1. 增加并验证 Redis client 配置与 Secret redaction；
2. 为 growth-api 建立命名 Redis 用户、最小 ACL 与 cache network；
3. 部署能够识别 v1 key/payload、严格回源且 Redis 不可用可降级的代码；
4. 在 development/test 显式启用 Strategy cache；
5. 先验证 miss/fill/hit，再执行坏值、Redis down、restart、ACL 和 M1 负载场景；
6. 不因 Redis 健康而改变 MySQL migration/grant 或 `/ready` 门槛。

撤销方式：

- 关闭 Strategy cache 装配，恢复把 MySQL Repository 直接注入 `EphemeralSelectionService`；
- 保留或删除 v1 key 都不影响 MySQL 事实；未删除 key 会在最多 5 分钟内自然过期；
- 移除 API 的 cache network/Redis Secret/ACL 用户前，先确认运行版本已不再创建 client；
- 不需要回滚数据库 schema，也不需要从 Redis 恢复任何数据；
- 回滚版本不得尝试读取未来 schema key。

发布/撤销都不能执行 `FLUSHALL` 或清理用户其他 Redis 数据。验收只删除经过项目 label、namespace 和资源身份确认的本节临时对象。

## 重评触发器

以下任一发生时必须新增或修订 ADR：

1. 出现 Strategy Update/Delete/Publish、草稿、审批或业务版本；
2. Activity 引用不可变 Strategy version，或正式 Draw 需要历史快照；
3. 要求 read-after-write、单调读或小于当前 TTL 的陈旧 SLO；
4. 需要事件、Outbox、CDC/Binlog 或跨实例精准失效；
5. 多副本负载证明本地 singleflight 仍产生不可接受的回源风暴；
6. Redis-down 负载证明 fail-open 会拖垮 MySQL，需要 breaker、backpressure 或 shedding；
7. unknown Strategy 流量成为实际穿透风险，需要负缓存、Bloom Filter 或 rate limit；
8. Strategy payload 接近 2 MiB、Award 接近 1000 或出现大 key/网络/解码瓶颈；
9. 真实命中率、变更率或陈旧容忍度证明当前 TTL 不合理，尤其是需要突破 5 分钟上限；
10. 需要 L1 进程缓存、Redis client-side cache、软/硬 TTL 或 stale-while-revalidate；
11. Redis 从单实例迁移 Sentinel、Cluster、托管服务、TLS 或跨区域拓扑；
12. 新业务消费者需要不同 keyspace、命令或 Redis 身份；
13. cache payload 增加用户、权限、风险、库存、结果或其他高风险字段；
14. 需要在 staging/production 运行并形成正式容量、可用性、灾备与告警目标；
15. go-redis major/协议变化影响 retry、timeout、连接握手或 ACL 命令集合。

## 验收证据要求

第 24 节封板采用分层证据，不要求每个边界同时由 unit 与 Compose 重复证明。封板前至少提供：

- key builder 与严格 codec 的单元/边界/模糊输入测试；
- hit、miss、invalid、Redis error、source error、SET/DEL failure 和 cancellation 控制流测试；
- 同 key singleflight、不同 key 独立、selector 不共享和 race test；
- 通过单元/边界测试证明 application expiration/jitter、最大 uint64、2 MiB sentinel、1000 Award 与严格 codec；通过真实 Redis 7.4 的 ACL helper `SET ... EX 30`、应用带 expiration 的 refill、restart/refill、坏值修复和 ACL 允许/拒绝集证明运行边界，不要求应用 wire 固定为 `EX` 而不是 `PX`；
- 独立 Compose 中的 cache hit、Redis down → MySQL、MySQL down + warm hit/cold miss，以及各单服务恢复；
- Migration 仍为 clean version 2、MySQL runtime 仍为两表 `SELECT` 的负向证据；
- 现有 Lottery HTTP response/header/error 与 React 测试零契约漂移；
- cache-disabled、warm cache、Redis unavailable 三组同口径 M1 数据；
- Redis hit/miss 与 MySQL source-load 证据，不能只用 latency 推断命中；
- `make verify`、`go test -race ./...`、文档门禁和精确 Git 变更白名单；
- 第 24 节课程、API、QA、设计手记和面试问答中的已实现/未实现边界。

本 ADR 是决策约束，不替代实际 QA 结果。任何未执行或受环境阻断的场景必须在 QA 中如实登记。本轮 runtime ACL 故意不授权 `TTL/PTTL`，因此没有采样服务器实际剩余 TTL；2 MiB Redis value 没有单列真实依赖探针；Redis 与 MySQL 同时停止也没有单列 Compose 故障注入。这三项分别由 expiration/size 单测、ACL helper `SET ... EX 30` 与应用带 expiration 的 refill，以及故障状态机/单测约束，并作为剩余证据边界保留，不能在文档中冒充实测。

## 参考资料

- [Redis：Go cache-aside](https://redis.io/docs/latest/develop/use-cases/cache-aside/go/)
- [Redis：go-redis production usage](https://redis.io/docs/latest/develop/clients/go/produsage/)
- [Redis：go-redis error handling](https://redis.io/docs/latest/develop/clients/go/error-handling/)
- [Redis：Go client connection](https://redis.io/docs/latest/develop/clients/go/connect/)
- [Redis：key expiration](https://redis.io/docs/latest/develop/use/keyspace/)
- [Redis：key eviction](https://redis.io/docs/latest/develop/reference/eviction/)
- [Redis：ACL](https://redis.io/docs/latest/operate/oss_and_stack/management/security/acl/)
- [Go：`singleflight`](https://pkg.go.dev/golang.org/x/sync/singleflight)

## 最终决策摘要

第 24 节采用 Redis cache-aside `StrategyReader` decorator。MySQL 继续是 Strategy/Award 唯一事实源；Redis 只保存完整、最大 2 MiB、最多 1000 个 Award、可经严格 v1 codec 与领域恢复重建的 Strategy 读取投影。key 使用 namespace、受控 environment、projection schema version 与规范十进制 StrategyID；每次 fill 的基准 TTL 默认且最多为 5 分钟，实际 TTL 再减去 0～10% 抖动。

Redis miss、坏值或运行故障都在有界预算内 fail-open 到 MySQL，但 Redis 不进入 API readiness。进程内 singleflight 只合并同 key 的权威回源，不共享随机选择、不跨实例、也不提供锁或幂等。not-found、用户/资格/风险/授权、库存、随机结果和 Draw/Result 一律不缓存。当前没有 Strategy 更新写路径，因此只承诺 TTL 有界陈旧；未来业务版本、精准失效和写并发必须另立 ADR。
