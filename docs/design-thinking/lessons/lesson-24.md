# 第 24 节设计手记：让缓存只加速事实读取，不接管事实

> 本节第一次让 Redis 成为真实业务读取链的一部分，但最重要的设计成果不是“用了 Redis”，而是证明缓存即使丢失、过期、损坏或不可用，也不能改变 Lottery 的业务事实和公开语义。
>
> 当前结论是：**MySQL 继续是 Strategy/Award 的唯一事实源；Redis 只保存可由 MySQL 与领域恢复函数完整重建的 Strategy 读取投影。** cache hit 可以跳过 MySQL，cache miss、坏值和 Redis 运行故障必须回源；用户资格、访问权限、库存、随机票据、一次选择结果和正式 Draw/Result 不进入本缓存。
>
> 本手记记录 2026-08-30 的实现时间切片。它解释当前边界从哪里推导出来，也保留哪些事实变化会迫使我们重新设计。长期约束以 [ADR-0020](../../decisions/ADR-0020-lottery-strategy-cache-aside.md) 为准，实际验收数字以[第 24 节 QA](../../qa/lessons/lesson-24.md)为准。

---

## 0. 决策命题与时间切片

### 0.1 这一节真正要决定什么

第 17～23 节已经建立下面这条真实但受限的读取链：

    canonical StrategyID
      → MySQL read-only Repeatable Read snapshot
      → RestoreAward / RestoreStrategy
      → WeightedSelector
      → ephemeral HTTP response

它的优点是边界诚实：每次选择都读取同一份权威快照，Repository error、领域不变量、随机选择和 HTTP error mapping 可以分别验证。它的代价也开始清楚：同一个可跨请求复用的 Strategy 被重复读取时，每次调用都重新开启事务、读取根与 Awards 两组数据并恢复聚合。这里尚无生产读写比例证据，“读多写少”仍只是待验证假设。

因此第 24 节的问题不是抽象的“怎样使用 Redis”，而是：

> 在不改变 MySQL 事实所有权、不改变领域和 HTTP 契约、也不提前发明 Strategy 写入/发布协议的前提下，怎样让重复 Strategy 读取可以复用一个外部投影，同时让缓存的所有失败都保持可撤销、可诊断和有界？

这包含九个必须同时回答的问题：

1. 缓存的对象究竟是 SQL row、领域对象、HTTP DTO，还是独立读取投影；
2. key 如何隔离环境、上下文、schema 与 identity；
3. value 如何兼容完整正 `uint64`、严格 schema、大小与领域不变量；
4. hit、miss、坏值、Redis error、MySQL error 和取消分别是什么语义；
5. 同一热点 key 并发 miss 是否放大 MySQL；
6. TTL 是什么一致性承诺，抖动解决什么、又不解决什么；
7. Redis client、连接池、Secret、ACL、网络和关闭由谁拥有；
8. Redis 是否影响启动、health 或 readiness；
9. 怎样用负载与 source-load 证据证明缓存生效，而不是只看一次延迟数字讲故事。

### 0.2 当前已经实现的事实

本节结束时，仓库能够由代码和测试证明：

- `application.StrategyReader` 仍是消费方拥有的窄读取端口；
- MySQL Repository 仍独立实现权威读取，没有导入 Redis 或缓存策略；
- `strategycache.Reader` 作为 decorator 选择性包裹 MySQL reader；
- cache disabled 时，组合根直接注入 MySQL Repository，并且不创建 Redis client；
- cache enabled 时，组合根创建一个长生命周期 Redis client，把 decorator 注入 `EphemeralSelectionService`；
- key 固定为 `growthos:<environment>:lottery:strategy:projection:v1:<canonical_strategy_id>`；
- value 是独立严格 JSON v1 投影，不给领域对象添加 JSON tag；
- StrategyID、AwardID 和 Weight 都使用规范十进制字符串；
- value 最大 2 MiB，读取通过 inclusive `GETRANGE 0..2MiB` 最多拿到上限加一字节；
- Award 数量仍受 `1..1000` 领域边界约束；
- hit 必须经过 exact shape、duplicate-name、unknown-field、EOF、schema、identity 和领域恢复校验；
- miss、Redis read error、poison value 和 oversized value 都回源 MySQL；
- poison value 只 best-effort 删除精确 key，不使用 pattern、scan 或全库清理；
- MySQL 成功而 Redis SET/codec 失败时，本次请求仍成功；
- MySQL not found、retryable、failure、stored-invalid、cancel 和 deadline 不被 cache error 覆盖；
- not-found 与 error 不做负缓存；
- 同进程同 key 的重叠 source load 由自有 flight group 合并；
- 不同 key 独立，cache hit 不进 group，每次选择仍独立调用 `WeightedSelector`；
- 每个 caller 保留自己的取消语义，共享 fill 有独立硬 timeout，并受进程 lifecycle 取消；
- 默认 base TTL 为 5 分钟、允许上限也是 5 分钟，写入 TTL 再减去 0～10% 抖动；
- Redis 操作有独立短 timeout，客户端关闭自动 command retry；
- Redis 不参与 API startup probe 或 `/ready`；
- Redis 运行故障时回源 MySQL，MySQL 不可用但 Redis 有合法 warm hit 时仍能读取投影，而 readiness 继续失败；
- Redis 业务用户只获准无 key 的 `PING`，以及 exact development key prefix 上的 `GETRANGE/SET/DEL`；
- Redis 不发布 host port，只与 API 共享 internal cache network；
- cache observation 只有低基数 outcome 与 duration，故障 warning 按种类以 10 秒窗口限频；
- 公开 Lottery route、DTO、header、status、error code/message、前端 decoder 和页面状态没有为缓存改变；
- M1 已在同一独立 Compose 环境完成 warm-cache、direct-MySQL 和 Redis-down 三组同口径短时基线。

主要实现入口：

- [Strategy cache decorator](../../../internal/lottery/adapter/strategycache/reader.go)
- [Strict v1 codec](../../../internal/lottery/adapter/strategycache/codec.go)
- [Per-key flight group](../../../internal/lottery/adapter/strategycache/flight.go)
- [Redis client boundary](../../../internal/infrastructure/redisstore/client.go)
- [Redis configuration](../../../internal/infrastructure/redisstore/config.go)
- [Application composition](../../../cmd/growth-api/database.go)
- [Cache observer](../../../cmd/growth-api/cache_observer.go)
- [Typed application configuration](../../../internal/platform/appconfig/redis.go)
- [Compose topology](../../../deploy/compose/compose.yaml)
- [Redis ACL bootstrap](../../../deploy/docker/redis-entrypoint.sh)

### 0.3 当前明确没有实现什么

本节没有实现或证明：

- Strategy Update、Delete、Publish、草稿、审批、业务版本或乐观锁；
- 写数据库后的精准缓存失效、Outbox、CDC、Binlog 或跨实例 invalidation；
- Redis Cluster、Sentinel、RDB、AOF、跨区域复制或生产灾备；
- 跨进程 singleflight、Redis 锁、Lua、Redlock、lease、fencing token；
- L1 本地缓存、Redis client-side caching、soft TTL、hard TTL 或 stale-while-revalidate；
- unknown Strategy 负缓存、Bloom Filter 或正式边缘 rate limit；
- 用户资格、风险、额度、次数、频控或规则中间结果缓存；
- Principal、Role、Permission、Scope、PolicyDecision 或浏览器角色缓存；
- 库存、预占、Benefit 发放、补偿或到账状态缓存；
- random ticket、selected Award、ephemeral selection、Draw/Result 或幂等响应缓存；
- 正式 Draw、可恢复结果、幂等请求或安全自动重试；
- 生产吞吐、容量、P99、可用性、强一致、灾备或 SLA/SLO 达标结论。

这些不是遗漏清单，而是防止缓存越权成为新事实源的停止线。

---

## 1. 不可争辩的事实与约束

### 1.1 业务事实：配置复用与一次结果不是同一种数据

Strategy/Award 是当前 Lottery 的配置事实：它跨多次选择复用，完整内容可以从 MySQL 两张表恢复。一次 ephemeral selection 则是调用时由随机源产生的候选，没有 DrawID、没有持久化、也不能在重试时被解释为同一次结果。

因此两者虽然都出现在同一响应链，缓存性质完全不同：

| 对象 | 是否跨请求复用 | 是否可由 MySQL 当前表完整重建 | 时效/身份 | 本节结论 |
| --- | ---: | ---: | --- | --- |
| Strategy + Awards | 是 | 是 | 配置级、当前无业务版本 | 可以缓存严格读取投影 |
| 随机 ticket | 否 | 否 | 单次调用随机材料 | 禁止缓存 |
| selected Award | 否 | 否 | 当前只是一份临时候选 | 禁止缓存 |
| no_reward outcome | 否 | 否 | 合法选择结果，不是错误 | 禁止缓存 |
| Strategy not found | 可重复观察但会变化 | 否，缺少发布/不存在证明 | 当前无失效事件 | 不做负缓存 |
| 用户资格或权限决定 | 可能短时复用 | 不属于 Lottery 两表 | 主体、时间、版本、scope 高时效 | 禁止进入本缓存 |

### 1.2 数据事实：MySQL 是 sole truth，Redis 是可丢弃投影

MySQL 的权威性来自已有边界，而不是因为关系数据库天然正确：

- Migration 与表约束定义可持久结构；
- runtime identity 只有两张业务表的 SELECT；
- Repository 使用单个 read-only Repeatable Read 事务读取根与 Awards；
- SQL row 必须经过 `RestoreAward` / `RestoreStrategy` 才能成为领域对象；
- Repository error 有现有 application 分类和 HTTP 映射。

Redis value 则可能来自旧进程、手工测试、错误客户端、未来版本、内存损坏或不完整写入。internal network 和 ACL 能降低攻击面，却不能把缓存字节提升成可信领域对象。删除全部 Redis 数据、Redis eviction、容器 restart 或关闭缓存功能，都必须只影响延迟与 MySQL 负载，不能丢失业务事实。

### 1.3 平台事实：开发 Redis 有限、易失且与服务共享资源

当前 Docker Desktop 报告 4 CPU、约 1.92 GiB 内存。Compose Redis：

- 使用 Redis 7.4 基线；
- `/data` 是 64 MiB tmpfs；
- `maxmemory` 为 48 MiB；
- eviction policy 为 `allkeys-lru`；
- 关闭 RDB 与 AOF；
- 容器停止或重建后不承诺保留 cache value；
- Redis 不发布 host port。

这组事实要求 value 有大小上限、key 有 TTL、应用能重建、故障测试不能依赖持久缓存。Redis 官方的 [eviction 说明](https://redis.io/docs/latest/develop/reference/eviction/)也提醒：`maxmemory` 只约束参与 eviction 计算的部分内存，并不等于容器或 Docker VM 总内存。因此“48 MiB maxmemory”不是完整资源隔离证明。

### 1.4 公开契约事实：HTTP `no-store` 与服务端 Redis 不冲突

公开响应继续发送 `Cache-Control: no-store`。它限制浏览器、代理和共享 HTTP cache 重放带 request ID 的响应；服务端 Redis 保存的是 Strategy 配置投影，不是 HTTP response。二者处于不同层：

    browser/proxy response cache: forbidden
    server-side Strategy projection: bounded and rebuildable

因此不能因为引入 Redis 就删除 `no-store`，也不能把 cache hit、TTL、Redis key 或 source 暴露成前端业务契约。

### 1.5 外部系统事实

Redis 官方 [cache-aside 资料](https://redis.io/docs/latest/develop/use-cases/cache-aside/)把该模式描述为由应用先查缓存、miss 后读取主存储并回填。它支持本节的控制流，但不会替本项目决定：什么是事实源、怎样验证领域不变量、是否允许 stale、怎样分类 MySQL error、谁拥有连接或哪些数据绝不能缓存。

Redis 官方 [ACL 资料](https://redis.io/docs/latest/operate/oss_and_stack/management/security/acl/)说明命令、key pattern 和 channel 可以分别限制。这提供最小权限机制，但“配置里写了 ACL”仍不等于授权真实生效，必须同时做允许与拒绝探针。

Redis 官方 [EXPIRE](https://redis.io/docs/latest/commands/expire/) 与 [TTL](https://redis.io/docs/latest/commands/ttl/)定义 key 过期和剩余寿命语义。它们能证明物理生命周期，却不能证明业务版本或 read-after-write 一致性。

### 1.6 仍待验证的假设

以下内容不是既成事实：

- 真实生产 Strategy 是读多写少；
- 五分钟陈旧窗口符合未来产品要求；
- 2 MiB 足够所有真实 Strategy 且不会过于宽松；
- 1000 Award 是合理产品上限而不只是领域防御上限；
- 单实例 same-key singleflight 足以保护未来多副本 MySQL；
- Redis RTT 在生产仍低于跳过两次 SQL 的收益；
- 50 RPS、10 秒本机结果能预测长稳或峰值行为；
- allkeys-lru 适合未来同一 Redis 实例上的所有消费者；
- 当前 Redis Secret、ACL 和 internal network 模型可直接搬到生产。

这些假设进入第 13 节风险账本，而不能被“测试绿色”自动升级为结论。

---

## 2. 为什么现在缓存，为什么不多缓存

### 2.1 为什么第 17～19 节没有立即加缓存

在领域与 Repository 边界尚未稳定时加缓存，会掩盖三个问题：

1. 不知道完整聚合是什么，就不知道 value 是否完整；
2. 不知道读后恢复边界，就容易把 SQL row 或松散 JSON 当领域对象；
3. 不知道权威 error 语义，就容易把 miss、not-found 和依赖失败混为一谈。

先完成领域不变量、两表 schema、RR 快照和 Repository error，才有资格说“这个对象可以被重建”。缓存不是在架构前面替代建模，而是在权威读取成立以后装饰它。

### 2.2 为什么第 20～21 节没有立即加缓存

第 20 节需要先证明固定 Strategy 输入到 Award 的选择算法无偏；第 21 节需要先证明公开 API 的 identity、error、timeout、`no_reward` 和 ephemeral 边界。若同一节同时引入 Redis，看到错误结果时无法快速区分是：

- MySQL 快照问题；
- cache codec 或 stale 问题；
- selector 问题；
- HTTP decoder/mapping 问题。

直接 MySQL 路径也成为今天 M1 的必要对照，而不是被缓存实现删除的旧代码。

### 2.3 为什么第 24 节是合适时点

此时已经同时出现：

- 一个稳定、完整、可恢复的读取对象；
- 一个真实 application consumer；
- 一个真实 HTTP/React 纵向调用；
- 一个已准备但尚无业务消费者的 Redis 开发拓扑；
- 对重复读取成本进行同口径测量的能力；
- 第 23 节明确的事实所有权停止线。

换句话说，本节不是为了路线表“轮到 Redis”，而是首次满足了缓存成立所需的前提。

### 2.4 为什么不缓存用户资格、权限或一次结果

缓存是否安全，不由字段大小或访问频率决定，而由下面的问题决定：

1. 权威事实是谁；
2. 何时变化；
3. 旧值服务多久仍正确；
4. 怎样精准失效；
5. 主体和 scope 怎样隔离；
6. 故障时允许放行还是必须拒绝；
7. 是否包含不可重复的一次性决定；
8. 是否含 PII、风险特征或高价值业务信息。

当前用户资格、权限、额度和库存连权威模型都尚未实现，谈缓存只能猜。一次选择结果又缺少持久 request identity，缓存它会伪造幂等。第 24 节因此只处理一个低敏、可重建、失败后可安全回源的配置投影。

---

## 3. 从第一性维度推导需求

### 3.1 事实源：第二份数据不能获得第一份数据没有授予的权力

**事实：** Redis value 由 MySQL Strategy 派生。

**风险：** 如果 Redis 可被独立写入或 cache hit 绕过恢复，它就能创造 MySQL 从未认可的 Award、Weight 或 Outcome。

**需求：**

- encoder 只接受已经通过领域恢复的完整 Strategy；
- decoder 必须重新调用领域恢复；
- Redis 不提供业务 Create/Update/Delete/Publish；
- cache error 永远不能覆盖 source error；
- 双依赖失败时返回 MySQL/调用 context 的技术失败。

**机制：** `StrategyReader` decorator + strict codec + source-error preservation。

**证据：** codec 边界测试、source failure 测试、poison cache 回源、Redis/MySQL 故障验收。

### 3.2 故障域：非关键加速层不能扩大关键路径故障面

**事实：** 没有 Redis 时，系统原本可以直接读 MySQL。

**风险：** 如果启动、readiness 或每次请求都长时间等待 Redis，增加缓存反而降低可用性。

**需求：**

- client 创建不主动 PING；
- Redis 运行故障在短 lookup budget 后绕过；
- SET/DEL failure 是可观测的 best-effort failure；
- `/ready` 继续只检查 MySQL；
- Redis 配置语法与 Secret 缺失在启动前失败，不能把 operator mistake 当运行降级。

**机制：** 无启动 `PING`/同步可用性探测的 Redis pool、独立 timeout、fail-open-to-source、配置 fail-closed、readiness 保持 MySQL-only。默认 `MinIdleConns=0` 时首命令建连；显式正值只允许后台预热，预热失败不由 `Open` 同步返回。

**证据：** Redis stopped 仍 200 且 `/ready` 200；MySQL stopped 时 warm hit 200、cold miss 503 且 `/ready` 503。Redis 与 MySQL 同时停止的冷读结果由相同状态机和单测推导，本轮没有把“双服务同时 down”单列为 Compose 故障注入。

### 3.3 权限：基础设施 ACL 与产品 RBAC 必须分层

**事实：** Redis 命名用户是进程身份；未来 Principal/Role 是产品主体身份。

**风险：** 把 Redis 密码当用户会话，或让前端角色决定 Redis key，会把基础设施权限与业务授权混在一起。

**需求：**

- API 持有 Redis Secret，浏览器、Web、MySQL、Migration 不持有；
- ACL 同时限制命令与 exact keyspace；
- cache key 不含客户端可控 environment/namespace；
- 不缓存未来 authorization decision；
- 第 31～35 节仍需服务端逐请求 RBAC。

**机制：** named user、internal network、Secret file、server-owned key builder。

**证据：** allow/deny command probes、host-port topology、Secret mount 检查、前端契约零变化。

### 3.4 可逆性：关掉缓存必须恢复旧路径，不需要恢复数据

**事实：** MySQL direct reader 在引入缓存前已经成立。

**风险：** 如果关闭缓存还要回填数据库或迁移 schema，缓存已经变成事实参与者。

**需求：**

- feature/config disabled 时直接注入 Repository；
- 不新增数据库 Migration；
- 删除 Redis key 不影响 MySQL；
- 撤销代码前先移除 client 消费，再移除 Secret/network/ACL；
- 不使用 `FLUSHALL` 作为回滚步骤。

**机制：** composition-time decorator、无双写、TTL 自然清理。

**证据：** cache-disabled M1 的零 cache event 与每请求两次 MySQL execute；Migration version 和业务表 fingerprint 保持不变。

### 3.5 一致性：没有业务版本时只能承诺 TTL 有界陈旧

**事实：** 当前没有 Strategy Update/Publish，也没有可靠聚合业务 version；Award 子行变化不保证推动根行 `updated_at`。

**风险：** 声称精准失效或 read-after-write 会伪造不存在的协议。

**需求：**

- key/payload `v1` 只代表 cache schema；
- base TTL 最大 5 分钟；
- 新 schema 使用新 namespace；
- 外部直改 MySQL 时，旧合法投影可能服务到剩余 TTL；
- 第一次真实写路径出现前必须新增 ADR。

**机制：** physical TTL + schema-key version + 明确不承诺。

**证据：** 配置/单元测试证明传给 `SET` 的 expiration 与抖动边界；隔离 acceptance 的 ACL helper 明确执行 `SET ... EX 30`，应用 refill 路径证明带 expiration 的写入可读且 restart 后可 miss/refill，但不把 helper 的 `EX` 冒充应用 wire 一定使用 `EX`（go-redis 也可编码为 `PX`）。业务 ACL 无 `TTL/PTTL`，本轮未采样服务器实际剩余 TTL 窗口；这些证据仍不能证明业务版本一致性。

### 3.6 可观测性：必须证明数据源变化，而不是用延迟猜命中

**事实：** 单次本机延迟受 scheduler、连接复用、日志、Docker VM 和邻居负载影响。

**风险：** warm P99 更低不一定是缓存生效，Redis-down P99 偶然更低也不代表故障路径更优。

**需求：**

- 同时记录 scheduled/completed/error/status/latency；
- 用 cache outcome 证明 hit/miss/fill/bypass；
- 用 MySQL Performance Schema account counter 证明 source load；
- 三组使用相同 endpoint、fixture、rate、duration、workers 和 timeout；
- 数值只作本机短时基线，不设生产门槛。

**机制：** M1 load harness + low-cardinality cache observation + MySQL `COM_STMT_EXECUTE` delta。

**证据：** 第 10 节三组真实数据。

### 3.7 学习成本：第一节缓存必须能被完整解释和删除

**事实：** 多级缓存、分布式锁、CDC、软硬 TTL 和 Bloom Filter 都可能合理，但每个都引入独立故障与一致性模型。

**风险：** 一次性堆叠会让学习者只记住技术名词，无法指出哪一层保护哪个失败。

**需求：**

- 先实现一个 cache-aside decorator；
- 只做单进程同 key source-load 合并；
- 对每个未采用机制写出重评触发器；
- 保留 direct MySQL 路径和可执行负向验收。

**机制：** 小端口、明确包边界、方案矩阵、假设账本。

**证据：** 代码可在组合根一处禁用；领域、Repository、selector、HTTP 和 React 无缓存依赖。

---

## 4. 备选方案矩阵

### 4.1 总体比较

| 方案 | 事实源 | hit 成本 | 一致性/失效负担 | 故障面 | 并发 miss | 可撤销性 | 当前结论 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 每次直接 MySQL | MySQL | 事务 + 两次 SELECT + 恢复 | 最简单 | 只有 MySQL | 每请求读取 | 最强 | 保留为 bypass/对照，不作终态 |
| Redis cache-aside decorator | MySQL | Redis RTT + strict restore | 当前 TTL-only | Redis 可绕过 | 单进程同 key 合并 | 强 | 采用 |
| 基础设施 read-through service | 容易模糊 | 单次 cache API | cache service 必须理解 source | 新远程服务 | 可跨实例但复杂 | 中 | 当前不采用 |
| Repository 内嵌 Redis | 名义 MySQL，职责混合 | 同 cache-aside | 与 SQL adapter 耦合 | 测试面扩大 | 可做但难替换 | 较弱 | 不采用 |
| write-through | MySQL/Redis 顺序待定 | warm | 需要真实写协议 | 双写部分失败 | 写时准备 | 较弱 | 当前无写路径，拒绝 |
| write-behind | Redis 可能临时成事实源 | warm | 极高 | 丢写/乱序/恢复 | 与本节无关 | 弱 | 拒绝 |
| 进程内 LRU/L1 | MySQL | 最低 | 每实例副本与失效 | 进程内 | 实例内 | 强 | 暂缓 |
| Redis client-side cache | MySQL + Redis tracking | 本地 | tracking/invalidation | Redis + 本地 | 每实例 | 中 | 暂缓 |
| 负缓存/Bloom | MySQL/存在集合 | 低 | 创建/发布失效 | 误判/陈旧不存在 | 降低穿透 | 中 | 无真实穿透证据，暂缓 |
| Redis 分布式锁/Lua | MySQL | miss 增加协调 | lease/fencing/释放 | Redis 故障更关键 | 跨实例 | 较弱 | 无多副本证据，暂缓 |

### 4.2 为什么选择 decorator 而不是修改 Repository

缓存消费的是 `StrategyReader` 语义，不拥有 SQL 表。decorator 让依赖方向保持：

    EphemeralSelectionService
      → application.StrategyReader
          → strategycache.Reader
              → MySQL Repository

MySQL Repository 仍可以独立做集成测试；cache 可以用 fake Store 和 fake source 精确覆盖状态机；禁用 cache 只改变 composition，不改变 application。若把 Redis 塞进 mysqlrepo，未来另一种 source 或 cache bypass 都必须修改权威 adapter。

### 4.3 为什么不是通用 `common/cache`

Strategy cache 的 value、TTL、miss、错误、大小和可缓存对象都由 Lottery 语义决定。一个只提供 `Get(key) any` / `Set(key, any)` 的公共包会隐藏：

- 什么可以缓存；
- 谁构建 key；
- 哪个 error 可以绕过；
- 是否允许负缓存；
- 怎样恢复领域对象；
- TTL 是否满足对象时效；
- 指标是否泄露 identity。

本节复用的是窄 Redis client 命令边界，不复用万能业务缓存策略。未来其他上下文可以共享 `redisstore.Client` 的基础设施能力，但必须拥有自己的 adapter、keyspace、ACL 和决策。

### 4.4 为什么不是 local cache

进程内缓存会省去 Redis RTT，未来 profile 可能证明它有价值。但现在采用它会同时引入：

- 多实例副本和不同冷启动状态；
- 每实例容量与 eviction；
- 发布时本地清空；
- Redis + L1 两层 TTL/失效顺序；
- 更难解释的 hit 来源；
- 更复杂的 M1 证据。

当前目标还包括验证 Redis ACL、Secret、网络、codec 和故障恢复，因此只做 Redis L2。若以后 Redis RTT 已成为主要延迟且变更/失效模型已经成熟，再用 profile 和副本数据重评 L1。

### 4.5 为什么不是 write-through 或 write-behind

当前 runtime identity 对 Strategy 表只有 SELECT，没有产品写用例。为了“让缓存总是热”创造写路径，会强迫我们猜测：

- MySQL commit 与 Redis SET 的先后；
- 一边成功一边失败怎样恢复；
- 并发旧请求能否把旧值写回；
- 发布版本怎样原子切换；
- rollback 是否删除 cache；
- commit outcome unknown 怎样解释。

write-behind 更会让 Redis 暂存尚未进入 MySQL 的业务事实，直接违反 sole truth。它们不是技术上永远错误，而是当前没有真实写需求支撑。

### 4.6 为什么不是分布式锁

本节只在单进程合并重叠 source load，明确不承诺跨实例。分布式锁至少需要 lock identity、租约、续期、超时、释放原子性、旧 owner fencing 和 Redis 故障语义。若 Redis 正在故障，依赖同一个 Redis 锁保护 MySQL 还可能让降级更脆弱。

只有未来多副本测试证明：同 key 过期时跨实例回源足以伤害 MySQL，才有证据比较提前刷新、soft/hard TTL、分布式协调、数据库限流或请求 shedding。现在先引入锁会解决尚未测出的系统。

---

## 5. Key、Value 与严格重建协议

### 5.1 Key 不是字符串拼接细节，而是隔离边界

当前 key 形状固定为：

    growthos:<environment>:lottery:strategy:projection:v1:<canonical_strategy_id>

每一段各自承担一个责任：

| 片段 | 责任 | 不能替代什么 |
| --- | --- | --- |
| `growthos` | 产品级命名空间 | 不能替代实例/ACL 隔离；本节为避免授权 `SELECT` 固定使用 DB 0 |
| `<environment>` | development/test/staging/production 隔离 | 不能接受请求输入或任意字符串 |
| `lottery` | bounded context 归属 | 不能代表用户权限 |
| `strategy` | 聚合类型 | 不能泛化成任意对象 |
| `projection` | 声明 value 是派生读模型 | 不能宣称是事实 |
| `v1` | key/value 技术 schema 版本 | 不能冒充 Strategy 业务版本 |
| `<canonical_strategy_id>` | 精确聚合 identity | 不能放原始未校验路径参数 |

StrategyID 必须先经过应用层 canonical identity 校验，key builder 不接受浏览器提供的 namespace、environment 或完整 key。当前 key 不含邮箱、用户名、Principal、request ID、随机 ticket、Award 名称等敏感或高基数内容。

这也解释 ACL 为什么可以写成 exact prefix：Redis 身份只应访问本上下文本环境的 v1 投影，不能访问未来权限缓存或其他 bounded context。

### 5.2 Value 为什么是独立 projection

四种常见候选都被逐一排除：

| 候选 | 问题 |
| --- | --- |
| SQL row JSON | 两张表的完整性、排序和领域恢复责任泄漏到消费者 |
| 领域 struct 直接 JSON | 迫使领域对象承担缓存兼容 tag，未来重构字段会无意改变 wire schema |
| HTTP response DTO | 混入 request ID、随机结果和 presentation contract，无法跨请求安全复用 |
| 独立 Strategy projection | 明确是派生数据，可独立版本化并重新走领域恢复 |

因此 v1 value 只表达重建 Strategy 所需的最小完整事实：

- 固定 schema 名 `growthos.lottery.strategy.projection`；
- 固定整数 version `1`；
- Strategy identity；
- 完整 Award 列表；
- 每个 Award 的 identity、name、weight 与 outcome；
- 不保存 selected Award、ticket、Principal、eligibility、permission 或 request metadata。

### 5.3 为什么 `uint64` 必须是规范十进制字符串

StrategyID、AwardID 和 Weight 的领域范围是正 `uint64`。若使用 JSON number：

- JavaScript number 无法精确表达大于 `2^53-1` 的全部整数；
- 不同 decoder 可能接受指数、浮点或溢出后的近似值；
- 同一个业务 identity 可能出现多种文本表示。

v1 因此只接受不带符号、不带前导零、非零、完整落在 `uint64` 范围内的十进制字符串。`"7"` 合法，`7`、`"07"`、`"+7"`、`"0"`、`"-1"`、`"1e3"`、空串和溢出字符串均非法。这不仅是编码偏好，也是 identity 唯一性与跨语言可逆性约束。

### 5.4 严格 JSON 的防线

缓存不是可信输入。decoder 依次执行：

1. 总字节数不得超过 2 MiB；
2. JSON nesting depth 不得超过 16；
3. root、strategy 和 award 只允许 v1 声明的 exact case-sensitive 字段；
4. 所有字段必须出现且只出现一次；
5. unknown field、duplicate name、大小写近似字段一律拒绝；
6. schema 名与 version 必须完全匹配；
7. canonical StrategyID 必须与请求 identity 相同；
8. Awards 必须在 `1..1000`；
9. 所有 `uint64` 字段按规范十进制解析；
10. 文档结束后必须 EOF，拒绝 trailing token 或第二个 JSON document；
11. 每个 Award 调用 `domain.RestoreAward`；
12. 完整列表调用 `domain.RestoreStrategy`。

严格 decoder 的目标不是让 Redis 成为安全边界，而是确保任何 cache hit 都只能产生当前领域承认的对象。即便攻击者或旧程序能写入合法 key，也不能靠松散 JSON 绕过领域约束。

### 5.5 2 MiB 边界为什么用 `GETRANGE 0..2MiB`

Redis `GET` 会先把整个 value 交给客户端，应用只能在分配和网络传输后发现过大。当前 Store 使用 inclusive range：

    GETRANGE key 0 2097152

结束下标是 inclusive，所以客户端最多收到 2 MiB + 1 byte：

- 长度 `<= 2 MiB`：继续严格解码；
- 长度 `== 2 MiB + 1`：可以确定实际 value 超限，拒绝并精确删除；
- 不需要为一个被污染的巨大 value 把全部内容拉进进程。

这一设计是传输上界，不是 Redis 侧写入绝对上界：拥有 SET 权限的应用身份仍可能写入大 value。因此 encoder 同样在 SET 前做大小检查，Redis 的 tmpfs/maxmemory 和 ACL 继续承担纵深防御。

### 5.6 Poison value 的处理顺序

“坏值”包括 oversized、invalid JSON、unknown/duplicate field、错误 schema/version、identity mismatch、非法数字、Award 超限或领域恢复失败。处理顺序是：

    detect poison
      → record corrupt
      → best-effort DEL exact key
      → load authoritative source
      → source success then attempt fresh SET

只删除当前 canonical key，不能 SCAN prefix、模糊 DEL 或 FLUSHDB。DEL failure 记录 `delete_error`，但不能阻止回源。若 MySQL 同时失败，返回 MySQL error；不能把“缓存坏了”伪装成 not found，也不能返回未经验证的旧值。

### 5.7 为什么不负缓存

unknown Strategy 可能来自拼写错误，也可能是未来创建/发布尚未被当前读模型观察。当前没有 Strategy 写协议、创建事件或可靠 invalidation，给 not-found 设置 TTL 会制造“已创建但仍不存在”的额外窗口。

因此：

- source not found 不 SET；
- source technical error 不 SET；
- caller cancel/deadline 不 SET；
- 只缓存成功恢复的完整 Strategy；
- 穿透攻击留给未来认证、rate limit、输入空间约束或有证据后的 Bloom/negative cache 设计。

---

## 6. 不变量与信任边界

### 6.1 本节必须长期保持的不变量

1. MySQL 是 Strategy/Award 唯一事实源。
2. Redis 中任意 key 都可以被删除而不造成业务数据丢失。
3. 关闭 cache 后公开 API 语义不变。
4. cache hit 只能返回经过 v1 strict codec 和领域恢复的 Strategy。
5. 请求 StrategyID 与 payload StrategyID 必须完全相同。
6. 每个 value 最多 2 MiB，最多 1000 个 Award。
7. schema version 不是业务 version，不能用于并发写控制。
8. miss、corrupt 和 Redis read error 都回源，不返回伪造的 not found。
9. source error 保留原分类；cache error 不覆盖 source error。
10. source success 后的 cache write failure 不改变本次响应。
11. not found 与任何 error 不负缓存。
12. 只合并同进程、同 key、时间重叠的 source load。
13. 不同 key 不互相阻塞。
14. cache hit 不进入 flight，不共享随机选择。
15. 每个调用者可以独立取消等待。
16. shared fill 有独立硬 deadline，并在应用 lifecycle 结束时取消。
17. TTL 总是正数且不超过 base TTL，base TTL 不超过五分钟。
18. Redis 不决定 startup/readiness。
19. Redis Secret 只授予 API/Redis，不进入 Web bundle、日志或 key。
20. ACL 同时限制命令与 exact key prefix。
21. observation 不含 key、StrategyID、payload、Secret 或 Principal。
22. 用户资格、权限、库存、随机 ticket 和一次结果不进入本缓存。

任意后续变更若打破其中一项，都不能被解释成“小优化”；它已经改变事实、一致性、安全或公开契约，必须新增决策和验收。

### 6.2 信任转换图

    HTTP strategyId
      │ canonical parse; untrusted
      ▼
    server-owned cache key
      │ no caller namespace
      ▼
    Redis bytes
      │ untrusted projection
      ▼
    strict v1 decode + identity check
      │ still not a domain object
      ▼
    RestoreAward + RestoreStrategy
      │ domain-approved
      ▼
    application StrategyReader result
      │ selector consumes immutable aggregate
      ▼
    fresh per-call random selection

另一条 source 路径是：

    MySQL rows
      → repository mapping
      → RestoreAward / RestoreStrategy
      → authoritative application result
      → strict encoder
      → disposable Redis projection

两条路径最终都通过同一领域恢复边界。Redis ACL 保护的是进程到 Redis 的基础设施能力；未来服务端 RBAC 保护的是 Principal 对业务 use case 的权限。二者不可合并，也不能由前端导航可见性代替。

### 6.3 端口归属

| 端口/组件 | 所属层 | 拥有什么 | 不拥有什么 |
| --- | --- | --- | --- |
| `application.StrategyReader` | application | 消费方所需读取语义 | SQL、Redis 命令 |
| MySQL Repository | infrastructure/adapter | 权威 SQL 读取与 error translation | cache policy |
| `strategycache.Reader` | Lottery adapter | cache-aside 状态机、codec、key、TTL、flight | Redis 连接创建、业务写 |
| `strategycache.Store` | cache adapter consumer port | `GetRange/Set/Del` 最小命令语义 | go-redis 类型、PING、ACL |
| `redisstore.Client` | infrastructure | 连接池、命令执行、close | Strategy schema 与领域规则 |
| composition root | `cmd/growth-api` | enable/disable、实例创建、生命周期、observer wiring | 领域选择规则 |

Store 的 `GetRange(ctx,key,start,end) ([]byte, found bool, err error)` 让 miss 成为显式状态，不要求 Strategy adapter import 某个 Redis 客户端的 sentinel error。`found=false,nil` 是 miss；`found=true,nil` 才允许 decode；非 nil error 是运行故障并回源。

---

## 7. 并发、取消、时间与资源预算

### 7.1 为什么只合并 source load

同一个 Strategy 的多个并发 miss 若都读 MySQL，会同时放大两次 SELECT 和事务开销。合并边界应当是：

    Redis lookup per caller
      → only miss/bypass enters per-key flight
          → one leader calls source
          → followers await same immutable Strategy/source error
      → every caller independently runs WeightedSelector

若把整个 selection 合并，多个用户会收到同一次随机结果；若把 Redis lookup 也合并，一次慢 Redis 请求会绑住所有 hit/miss 语义；若跨 key 合并，会制造 head-of-line blocking。因此 flight 只包住权威 `StrategyReader.GetByID` 与 best-effort fill。

### 7.2 为什么使用包内 flight，而不是直接套一个黑盒

通用 singleflight 可以合并函数调用，但本节还要求明确三个生命周期：

- caller wait context：每个 HTTP 请求自己的 cancel/deadline；
- shared fill context：不因 leader 首个 caller 退出而立刻中断，否则 follower 会继承偶然失败；
- process lifecycle：shutdown 后不允许后台 fill 悬挂。

包内 group 用 key→call map 表达这一小段协议，测试可以观测 leader/joined、移除、不同 key 和全部取消。选择自有实现不是否定通用库，而是当前取消语义需要成为可读、可验证的本节知识。若未来需求收敛到标准 singleflight 语义，可再替换。

### 7.3 独立取消的精确语义

leader 创建 shared fill 时，保留 caller 的 values，但通过 `context.WithoutCancel(leaderCtx)` 去除 leader 的 cancel/deadline，再叠加固定 fill timeout 与 application lifecycle cancel：

    leader request ctx ── values only ─┐
                                      ├─ shared fill ctx ─ source + SET
    application lifecycle ────────────┤
    hard fill timeout ────────────────┘

每个 caller—including leader—等待时都在自己的 `ctx.Done()` 与 call completion 之间选择：

- caller A 超时：A 立即得到自己的 context error；
- caller B 仍在等待：B 可继续拿到 shared source result；
- lifecycle cancel：取消 shared fill context；若权威读取尚未成功，等待者通常看到 source/cancel error；若 source 已成功而取消发生在 best-effort `SET`，仍可能返回成功 Strategy；
- hard fill timeout：限制孤立后台 work；最终结果同样取决于 timeout 发生在权威读取前，还是发生在已成功 source 之后的 best-effort 写入阶段；
- call 完成：从 map 移除并关闭 done，后续 miss 可创建新 flight。

共享 fill 不无限延续，但也不因最早离开的 caller 绑架其他 caller。hard timeout/lifecycle cancel 取消的是共享 context，不会把已经取得的权威 Strategy 因后续缓存写失败改写成失败。context values 的保留要谨慎：不能在 values 中放 Secret 或依赖 caller authorization 的 source decision；当前 Strategy 配置读取与 Principal 无关。

### 7.4 Timeout 预算不是三个独立旋钮

默认预算：

| 操作 | 默认上限 | 失败后动作 |
| --- | ---: | --- |
| Redis lookup | 75 ms | 记录 read_error，回源 |
| shared source fill | 2 s | 返回 source deadline/error |
| Redis SET/DEL | 75 ms | 记录 error，不改变 source success |
| cache base TTL | 5 min | 到期 miss/refill |

配置验证保持：

    lookup timeout + fill timeout + write timeout
      <= outer lottery selection timeout

这不是说每条路径必然串行花满三段，而是防止内部预算理论上超过公开 handler deadline。outer context 仍可更早取消 caller 等待；shared fill 受自己的硬上限和 lifecycle 约束。

### 7.5 TTL 与 0～10% jitter

Redis 官方 `EXPIRE`/`TTL` 提供物理过期与剩余寿命。当前写入 TTL 为：

    effectiveTTL = baseTTL - uniform[0, floor(baseTTL/10)]

约束：

- effective TTL 始终为正；
- 不会超过 base TTL；
- base TTL 默认且最大为五分钟；
- jitter source 可注入，边界可确定性测试。

jitter 解决的是同批 key 在同一时刻过期造成的集中 refill，不解决：

- 单个超热点 key 到期；
- 多实例同时 miss；
- MySQL 变更后的精准失效；
- Redis 全量重启后的 cold storm；
- 业务版本或 read-after-write。

对这些问题误用 jitter，会把概率摊开误称为一致性协议。

### 7.6 Redis 运行资源预算

当前开发拓扑通过多层上界防止 cache 占满小型 Docker Desktop：

| 层 | 当前边界 | 意义 | 限制 |
| --- | --- | --- | --- |
| value read | 2 MiB + 1 sentinel byte | 限制单次拉取 | 不限制恶意 Redis 内已存大小 |
| Award count | 1000 | 限制 decode/领域对象 | 不直接等于字节容量 |
| Redis `/data` | 64 MiB tmpfs | 限制文件系统占用 | Redis heap 不只来自 `/data` |
| `maxmemory` | 48 MiB | 触发 eviction | 不等于容器总 RSS |
| policy | allkeys-lru | cache 满时允许丢弃 | 未来共享非缓存 key 时必须重评 |
| TTL | ≤5 min | 限制无访问旧 key 寿命 | 热 key 仍可常驻到每次重填 |
| client pool/timeouts | 有界 | 限连接与阻塞 | 需要生产负载重新定标 |

因为 Redis 是可丢弃投影，eviction 等价于未来 miss，而不是数据丢失事故；但 eviction rate 激增仍是容量或热点信号，不能只靠 fail-open 隐藏。

### 7.7 构建资源边界

本节在约 1.92 GiB Docker Desktop 中遇到 Go 编译 `github.com/ugorji/go/codec` 的峰值内存问题。Docker builder 当前提供可覆盖 ARG，默认：

    GO_BUILD_PARALLELISM=1
    GO_BUILD_MAX_PROCS=2
    GO_BUILD_MEMORY_LIMIT=768MiB

并让两个 binary 在同一 builder 中顺序构建：

- `go build -p=1` 限制并行构建 package 数；
- `GOMAXPROCS=2` 限制单个 Go process 的并行执行；
- `GOMEMLIMIT=768MiB` 给 Go runtime GC 提供 soft memory limit。

Go 官方 [GC memory limit 指南](https://go.dev/doc/gc-guide#Memory_limit)明确这是 runtime 管理内存的软限制，不是进程 RSS、mmap、page cache 或 C toolchain 的硬 cgroup 上限。Go command 官方 [`-p` 说明](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies)限定并行 build/test command 数，但也不等同于总内存硬上限。

因此当前可说“默认降低并行峰值并在该开发机复现通过”，不能说“构建最多使用 768 MiB”。若未来二进制数量、CGO、依赖图或 CI runner 变化，应重新测 peak RSS；拆成两个 builder stage 可能改善 layer/target 隔离，但不会天然比同一 stage 顺序 build 更省峰值，反而可能重复依赖编译并增加时间与镜像缓存占用。

---

## 8. 失败矩阵与恢复协议

### 8.1 请求级失败矩阵

| 场景 | cache 动作 | source 动作 | caller 结果 | 后续状态 |
| --- | --- | --- | --- | --- |
| legal hit | decode + restore | 不访问 | 正常 Strategy，随后独立选择 | key 保持 |
| miss | `miss` | flight 回源 | source 结果 | 成功时 best-effort SET |
| read timeout/error | `read_error` | flight 回源 | source 结果 | Redis 故障不覆盖结果 |
| invalid/unknown schema | `corrupt` + exact DEL | flight 回源 | source 结果 | 成功时重填 v1 |
| oversized | `corrupt` + exact DEL | flight 回源 | source 结果 | 不拉取超限剩余字节 |
| identity mismatch | `corrupt` + exact DEL | flight 回源请求 identity | source 结果 | 防止 key/value 交叉污染 |
| DEL failure | `delete_error` | 仍回源 | source 结果 | poison 可能暂留，下一次继续拒绝 |
| source not found | 不写 | 返回 not found | 保留现有 application/HTTP mapping | 不负缓存 |
| source retryable/failure | 不写 | 原样分类 | 原 source error | 不被 Redis error 替代 |
| source stored-invalid | 不写 | 原样分类 | 原 source error | 要修 MySQL 事实 |
| source success + encode error | `write_error` | 已成功 | 本次成功 | 下次再次回源 |
| source success + SET error | `write_error` | 已成功 | 本次成功 | 下次 miss/bypass |
| caller cancel while waiting | caller 独立退出 | shared fill 可为其他 waiter 继续 | caller context error | fill 受 hard/lifecycle 限制 |
| Redis down + MySQL up | 短 read error | 回源 | 正常 200 | warning 限频，恢复后自然重填 |
| Redis up + MySQL down + warm legal hit | hit | 不访问 | 正常 200 | `/ready` 仍 503 |
| Redis up + MySQL down + cold miss | miss | 失败 | 503 selection unavailable | 不写负缓存 |
| Redis down + MySQL down | read error | 失败 | MySQL 对应 503 | Redis 错误只留 observation |
| Redis restart/eviction | miss | 回源 | 正常或 source error | 自动重建，无恢复导入 |

### 8.2 Fail-open 的准确含义

“fail-open”只指：

> 当 Redis 这个非权威加速层在运行时失败时，允许继续尝试权威 MySQL 读取。

它不表示：

- MySQL error 被吞掉后返回空 Strategy；
- invalid cache 被当作合法 hit；
- 权限、资格或库存检查失败时允许业务放行；
- Redis 配置错误、缺 Secret 或生产 TLS 不安全时仍启动；
- handler timeout 后在后台无限读取；
- stale value 在没有明确协议时无限服务。

这一区分很重要：对低敏可重建投影 fail-open 是可用性策略；对 authorization decision fail-open 往往是安全漏洞。

### 8.3 Readiness 为什么不检查 Redis

服务 readiness 回答“当前实例是否应继续接收需要权威能力的业务流量”。MySQL 是唯一事实源和 cold-read 依赖；Redis 只是优化。如果把 Redis 加进 `/ready`：

- Redis outage 会把原本可经 MySQL 成功的实例全部摘流；
- 负载集中到剩余实例，可能放大故障；
- 把加速层错误升级成服务不可用。

当前 `/ready` 继续检查 MySQL。精确公开语义：

- readiness unavailable：HTTP 503，code `dependency_unavailable`，message `service unavailable`；
- cold selection 的 MySQL failure：HTTP 503，code `lottery_selection_unavailable`，message `lottery selection is temporarily unavailable`；
- warm hit 在 MySQL outage 时可 200，但 readiness 必须 503；
- Redis outage、MySQL healthy 时 selection 200 且 readiness 200。

warm hit 与 readiness 同时出现“业务请求成功、实例未就绪”并不矛盾：前者是某一个已缓存 Strategy 的局部能力，后者是任意 cold authoritative read 的整体能力。

### 8.4 故障恢复顺序

#### Redis 故障

1. 观察 `read_error` warning 与 suppressed count，而不是立即改业务数据；
2. 确认 MySQL readiness 与 source latency；
3. 恢复 Redis/网络/ACL；
4. 不执行数据恢复导入；
5. 后续请求自然 miss/refill；
6. 验证允许/拒绝命令、带 expiration 的写入与 hit；若要验证服务器实际剩余 TTL 窗口，使用单独受控观察身份，不扩宽 runtime ACL；
7. 若 MySQL 已被放大，先控制流量，再考虑预热或新协调协议。

#### MySQL 故障

1. `/ready` 应 503，不能因 warm key 掩盖；
2. warm Strategy 可能临时服务到 TTL，cold Strategy 必须失败；
3. 恢复 MySQL 后先验证 readiness 和 direct read；
4. 不把 Redis dump 回灌成事实；
5. poison/expired key 继续按常规回源修复。

#### 双依赖故障

1. Redis read error 仅作为诊断；
2. cold selection 以 MySQL/source error 对外；
3. 先恢复 MySQL 的权威能力；
4. Redis 可随后恢复并自然重建；
5. 不为追求 200 返回未经验证缓存或默认 Award。

### 8.5 回滚与撤销

最小业务回滚是在组合根禁用 cache，让 consumer 重新直连 MySQL reader。基础设施回滚顺序必须与依赖方向相反：

1. 确认所有 API 实例已不再创建 Redis client；
2. 移除 API 的 Redis Secret/network/config；
3. 移除 Redis 服务、ACL bootstrap 与相关开发脚本；
4. 允许旧 key TTL/容器销毁自然消失。

不能先删除 Secret 或 Redis 再保留 enabled client，把预期回滚变成持续 read error 噪声；也不需要删除 MySQL 数据或运行 schema rollback，因为本节没有把缓存写入事实表。

---

## 9. 安全、隐私与最小权限

### 9.1 三层边界必须同时成立

| 层 | 当前机制 | 防什么 | 防不了什么 |
| --- | --- | --- | --- |
| 网络可达性 | Redis 不发布 host port，只加入 internal cache network | 宿主机/其他 Compose 网络的偶然访问 | 已进入同网络的恶意容器 |
| Redis 身份 | named user + Secret file，default user off | 匿名/default 登录与凭据硬编码 | Secret 被进程读取后的滥用 |
| Redis 授权 | 无 key `PING` + exact key pattern 上的 `GETRANGE/SET/DEL` | 跨 keyspace 和危险命令 | 在获准 prefix 内写 poison value |

纵深防御意味着不能因为有 internal network 就省略 ACL，也不能因为有 ACL 就发布 host port。Redis 官方 [ACL 文档](https://redis.io/docs/latest/operate/oss_and_stack/management/security/acl/)提供命令与 key pattern 约束能力；具体 prefix、Secret ownership 和拒绝探针仍由本项目负责。

### 9.2 当前 ACL 为什么只有四个业务命令

- `PING`：显式运维/验收探针；runtime client 创建不强制 PING；
- `GETRANGE`：有界读取，并通过 found 状态区分 miss；
- `SET`：成功 source load 后写带 TTL 的投影；
- `DEL`：只清除当前 exact poison key。

业务用户不需要 `GET`、`MGET`、`KEYS`、`SCAN`、`FLUSHDB`、`CONFIG`、`ACL`、`EVAL`、Pub/Sub 或任意写命令。即便未来为了批量预热想用更多命令，也应新增操作身份或精确授权，而不是把 runtime 用户升级成 `+@all`。

允许探针只能证明正路径；本轮 acceptance 的拒绝探针实际证明：

- 非 v1 prefix 的 `SET` 被拒绝；
- `KEYS`、`FLUSHDB`、`CONFIG GET` 等未授权命令被拒绝；
- default user 不能认证。

ACL 文件的 exact key pattern 和命令 allowlist 可推导前缀外 `GETRANGE/DEL` 以及错误 Secret 都应失败，但本轮脚本没有逐条直接探测这三项，不能把配置推论冒充运行探针。

### 9.3 Secret 生命周期

Secret 必须：

- 由 Compose secret file 挂载，不进入镜像 layer；
- 只提供给 Redis bootstrap 与 API；
- 不提供给 Web/Nginx、MySQL、Migration 或浏览器；
- 不进入 environment dump、build ARG、Git、日志、metric label 或 error message；
- 读取后只用于 client credential；
- 在 cache disabled 时不强制存在；
- 在 cache enabled 但缺失/空值时启动前失败。

运行时 Redis 故障允许回源，不等于配置 fail-open。operator 明确启用 cache 却没有 Secret，是部署契约错误，应该尽早暴露；否则系统会在每个请求上制造可避免的 timeout 与 warning。

### 9.4 TLS 与环境分级

development 可以在 internal Compose network 中使用无 TLS Redis，目的是让本机链路可观察。staging/production 开启 cache 时要求 TLS 且验证服务端 identity，不能用 skip-verify 把“加密了”误作“认证了”。

internal network 不是跨主机生产网络的传输保护。未来外部托管 Redis 还需决定：

- CA trust source 与证书轮换；
- hostname/SNI；
- connection URI 是否泄露 credential；
- Secret rotation 时是否允许双 credential 窗口；
- 网络策略、安全组和出口限制；
- Redis ACL 用户如何审计和撤销。

### 9.5 Privacy 与高价值数据停止线

当前投影保存 Strategy/Award 配置，不含 Principal 或用户行为，但仍不能默认“公开”。Award name 可能暴露未发布活动、商业规则或权益价值，因此：

- value 不写日志；
- key 不记录到 warning；
- observer 不带 StrategyID；
- ACL 不允许扫描整个 keyspace；
- Redis 容器与 Secret 不暴露给 Web。

未来若 Strategy 内容按 tenant、地域或权限裁剪，单一 StrategyID key 将不再安全：必须先明确事实模型、scope identity、授权检查顺序与 cache partition。绝不能先读一个全量投影，再依赖前端隐藏字段。

---

## 10. 可观测性与 M1 真实数据

### 10.1 事件词汇表

当前 cache observer 只接受固定低基数 outcome：

| outcome | 说明 | 默认日志级别/动作 |
| --- | --- | --- |
| `hit` | 合法 value 解码并恢复 | debug/计数 |
| `miss` | key 不存在 | debug/计数 |
| `read_error` | GETRANGE 技术失败 | warning，按种类限频 |
| `corrupt` | value 超限或严格解码/恢复失败 | debug/计数，随后 exact DEL |
| `delete_error` | poison key 删除失败 | warning，按种类限频 |
| `fill_leader` | 本调用创建 shared source load | debug/计数 |
| `fill_joined` | 本调用加入已有 source load | debug/计数 |
| `source_error` | 权威读取失败 | debug/计数，原 error 返回 |
| `write_ok` | 成功写入 projection | debug/计数 |
| `write_error` | encode/SET 失败 | warning，按种类限频 |

每个 observation 只携带 outcome 与 duration_ms。禁止把 key、StrategyID、payload、Redis address、Secret、Principal 或 raw error 变成无界 label。

### 10.2 为什么 warning 要限频

Redis down 时每个请求都可能 read error。若每次都 warning：

- 日志 I/O 可能进一步拖慢降级路径；
- 真正 source failure 被噪声淹没；
- 日志平台成本和告警数量失控；
- 同一 credential/network error 被重复暴露。

当前按 error kind 使用 10 秒 warning 窗口，并在下一次允许日志中报告 `suppressed_since_last`。因此一次测试中的 `read_error_logs=1` 表示限频后的日志条数，不表示 Redis 只失败了一次。routine hit/miss 不提升到 info，避免把正常 cache 行为变成生产噪声。

### 10.3 M1 固定实验口径

三组均于 2026-08-30 在同一 Mac、Docker Desktop 4 CPU、约 1.92 GiB 环境执行一次：

- 同一个不可变 Strategy `21003`；
- 同一个 loopback Nginx → Go HTTP endpoint；
- 目标 50 RPS；
- 10 秒；
- 16 workers；
- 单请求 timeout 3 秒；
- 三组都发送 500 个 scheduled request；
- MySQL source load 用 Performance Schema account 的 `COM_STMT_EXECUTE` delta 证明；
- 当前 Repository 每次 source load 恰好执行 Strategy root 与 Awards 两个 prepared SELECT，因此 1000 executes = 500 source loads。

### 10.4 三组精确结果

| 指标 | warm-cache | direct-MySQL | Redis-down |
| --- | ---: | ---: | ---: |
| scheduled | 500 | 500 | 500 |
| completed | 500 | 500 | 500 |
| success | 500 | 500 | 500 |
| errors | 0 | 0 | 0 |
| unexpected_status | 0 | 0 | 0 |
| dropped | 0 | 0 | 0 |
| HTTP 200 | 500 | 500 | 500 |
| actual_rps | 50.08618245771574 | 50.076986275497326 | 50.05050617599433 |
| p50 | 1.730167 ms | 2.390709 ms | 2.602833 ms |
| p95 | 4.129458 ms | 5.382042 ms | 5.777708 ms |
| p99 | 5.202 ms | 9.747167 ms | 8.222959 ms |
| max | 7.387375 ms | 25.003417 ms | 10.596541 ms |
| mysql_select_executes | 0 | 1000 | 1000 |
| source_loads | 0（由 execute=0 推导） | 500 | 500 |
| cache_hits | 500 | —（未单列） | —（未单列） |
| cache_events | —（hit 已单列） | 0 | —（各事件已单列） |
| fill_leaders | —（未单列） | —（cache disabled） | 500 |
| fill_joined | —（未单列） | —（cache disabled） | 0 |
| read_error_logs | —（未单列） | —（cache disabled） | 1 |

### 10.5 这些数据真正证明什么

可以证明：

1. warm-cache 的 500 次请求都命中 cache observation；
2. 同一窗口 MySQL account 的相关 execute delta 为 0，所以 hit 确实跳过权威读取；
3. direct-MySQL 没有 cache event，并产生 1000 次 SELECT execute，对应 500 次 source load；
4. Redis-down 时 500 次请求仍全部 200，并产生 500 次 source load，证明运行故障回源；
5. Redis-down 只输出 1 条 read-error warning，证明限频生效；
6. 在这一次短实验中，warm-cache 的各延迟分位低于 direct-MySQL；
7. direct-MySQL 的单次 max 高于另外两组，但只是本次样本现象。

不能证明：

- warm-cache 在生产一定提高某个百分比；
- Redis-down 比 direct-MySQL 的 P99 更优；
- 系统容量是 50 RPS 或高于 50 RPS；
- P99 达到生产 SLO；
- 2 MiB、5 分钟 TTL、连接池或 MySQL pool 已正确按生产容量定标；
- 长稳、突发、饱和、不同 Strategy 分布、多副本或跨网络性能；
- Redis 故障没有资源放大；
- MySQL/Redis 的 CPU、RSS、连接、GC、eviction、网络都健康。

`fill_joined=0` 只说明当前 50 RPS 与本机 source latency 下，500 次 source load 没有在同 key 上形成时间重叠；它不否定 deterministic concurrency test 已验证的 overlapping-call 合并语义，也不能预测 Redis 全量重启时的 cold storm。

### 10.6 为什么不用延迟单指标下结论

三组 actual_rps 均接近目标，且 completed/success/error/status/dropped 完整，这使延迟分位至少可比较同一请求完成集合。但 500 个样本、一次运行、loopback 与共享 Docker VM 仍容易受：

- Go scheduler 与 GC；
- Docker Desktop 邻居进程；
- MySQL/Redis warm state；
- connection reuse；
- 日志与文件系统；
- CPU frequency；
- 测量程序自身；
- 顺序执行造成的时间漂移

影响。设计结论因此主要依赖 source-load 与 failure semantics 证据，延迟只作开发基线。要形成 SLO/容量结论，需要重复试验、置信区间、持续时间、饱和阶梯、资源 telemetry、生产相似网络与独立环境。

---

## 11. 证据体系：每一层证明不同命题

### 11.1 单元与 codec 测试

应覆盖并已作为本节验收重点：

- canonical key 与 environment whitelist；
- exact v1 round trip；
- `uint64` 最小/最大、前导零、符号、指数、溢出；
- required/unknown/duplicate/case-sensitive/trailing token；
- JSON traversal depth guard 的允许/拒绝边界；
- 1 与 1000 Award 合法、0 与 1001 非法；
- 2 MiB 边界与 2 MiB + 1 sentinel；
- schema/version/identity mismatch；
- 所有解码成功路径都经过 RestoreAward/RestoreStrategy；
- TTL 最小、最大和 0～10% jitter 边界。

这些测试证明 deterministic state machine 与格式边界，不证明真实 Redis ACL、网络、TTL clock 或 MySQL 性能。

### 11.2 Reader 状态机与并发测试

fake Store/source 应精确覆盖：

- hit 不访问 source、不 SET；
- miss 回源，成功后 SET；
- read error 回源；
- poison exact DEL 后回源；
- DEL/SET failure 不覆盖 source success；
- source error 原样保留且不 SET；
- not found 不负缓存；
- caller cancellation 独立；
- leader 退出而 follower 可成功；
- lifecycle cancel 终止 shared fill；
- hard timeout 终止孤立 fill；
- 同 key overlapping 只有一次 source load；
- 不同 key 并行；
- flight 完成后 map 清理；
- selector 不在 flight 内。

race test 证明实现未发现数据竞争，不证明不存在逻辑竞态、跨进程 stampede 或未来写入的 stale overwrite。

### 11.3 Redis/Compose 真实验收

真实环境需要证明 fake 不能证明的部分。本轮已执行项与未执行项必须分开记录：

- Secret file 能启动 named user；
- default user 关闭；
- exact prefix 允许；
- 跨 prefix 与危险命令拒绝；
- ACL helper 的 `SET ... EX 30` 被允许，应用带 expiration 的写入可读且 restart 后自然 miss/refill；应用 wire 可能使用 `EX` 或 `PX`，服务器实际剩余 TTL 窗口本轮未采样；
- Redis 不发布 host port；
- internal cache network 只连接预期服务；
- `/data` tmpfs、maxmemory、eviction/no-persistence 与配置一致；
- container stop 后 API 回源；
- restart 后无需数据恢复，可自然 refill；
- MySQL stop 与 Redis stop 的状态码/error body/readiness 契约。

Compose 绿色仍不等于云生产安全：它没有证明 Security Group、托管 Redis ACL、TLS CA、Secret rotation、跨 AZ latency 或生产容量。

### 11.4 负向证据比“能跑”更重要

缓存的核心承诺大量存在于“不发生”：

- warm hit 时 MySQL execute 不增加；
- direct mode 时 cache event 为 0；
- source error 时没有 SET；
- not found 时没有负缓存；
- Redis down 时公开 error 不变化；
- MySQL down 时 readiness 不被 warm hit 掩盖；
- poison 时不返回缓存内容；
- payload 不进入日志；
- ACL 用户不能扫描或跨 prefix。

如果验收只检查 200 和页面可见，就无法区分真正 cache-aside、静态 mock、吞错或权限过宽。

### 11.5 验收必须保持可复现而非依赖手工记忆

脚本需要：

- 以明确 Compose project name 隔离；
- 在运行前解析精确容器/网络/Secret 目标；
- POSIX `#!/bin/sh` + `set -eu`，并避免依赖非 POSIX `pipefail`；
- 对预期失败命令显式检查，而不是让 `!`/pipeline 吞掉错误；
- 正常路径显式恢复被停止的 Redis/MySQL；成功或异常退出的 trap 都精确销毁整个隔离 project，而不是承诺保留并恢复其中服务；
- readiness polling 有 deadline；
- 精确断言 status、code 与 message；
- 最终输出环境、fixture、负载参数和数据；
- 清理只针对本脚本创建的 Compose project/artifact。

若故障验收中途退出后依赖没有恢复，下一组数字会被污染；如果 cleanup 使用宽泛 project/volume 名称，又可能删掉用户的开发数据。真实项目的验收脚本本身也是需要威胁建模的代码。

---

## 12. 主动发现：用户没有点名但架构必须追问的点

### 12.1 “可重建”是否有可验证定义

不是“Redis 丢了还能查到一些东西”，而是：

- MySQL 拥有重建所需全部字段；
- projection 不含仅存在于缓存的决定；
- decoder 重新执行领域不变量；
- 删除 Redis 不需要人工数据迁移；
- direct path 与 cached path 对同一 Strategy 产生等价领域对象。

未来若 projection 加入计算后无法从 MySQL/确定性规则重建的字段，它就不再是当前 cache，必须迁移到正式事实或结果模型。

### 12.2 Strategy 热点分布未知

M1 只读 Strategy `21003`，它证明单热点 warm hit，但无法估算：

- 总 Strategy 基数；
- Zipf 热点程度；
- value 大小分布；
- 每分钟独立 miss 数；
- Redis 内存命中率；
- 单 key 到期时峰值 source concurrency。

在没有这些数据前，48 MiB/5 分钟只是开发安全边界，不是生产容量方案。

### 12.3 全量 Redis 重启比单 key 过期更危险

jitter 只在写入时间分散时分散过期；Redis restart 会让全部 key 同时消失。多实例会分别 singleflight，同一 key 仍可能每实例一次回源。上线前需用真实副本数与热点集测试：

- cold-start source QPS；
- MySQL pool wait；
- API deadline/error；
- 是否需要限并发、预热、stale snapshot、分层 TTL 或 load shedding。

当前不预热是因为没有完整、授权安全、容量有界的 Strategy 列表用例；盲目 SCAN/MySQL 全表会创造新的高风险路径。

### 12.4 未来写路径可能产生 stale refill race

当前无 Strategy Update，所以 source load 与 SET 之间没有业务版本竞态。一旦加入写路径，可能发生：

1. reader 读取旧 MySQL snapshot；
2. writer commit 新 Strategy 并 DEL cache；
3. reader 把旧 projection SET 回 cache。

只有 TTL 不能阻止这个旧值回填。届时必须比较 versioned key、compare-and-set、publish version、transactional outbox/CDC、write-side invalidation 与 soft/hard TTL，而不是简单“写后 DEL”。

### 12.5 ACL key pattern 不是 tenant authorization

Redis ACL 按 Redis connection identity 授权；当前 API 所有请求共享一个连接身份。未来 multi-tenant 不能把 tenant ID 拼进 key 就宣布隔离，因为同一 API credential 仍可访问所有获准 key。产品授权必须在服务端 use case 先验证 Principal/Scope，cache 只能在授权后读取合适投影。

### 12.6 Clock 与 jitter 的可测试性

TTL 使用 Redis 服务器过期时钟，jitter 在应用侧计算。单元测试可证明计算范围，真实 TTL probe 才能证明 SET 参数和 Redis 行为。容器/宿主时间漂移虽然不改变相对 TTL 的基本语义，却会影响日志关联与绝对事件分析，因此运维仍需时钟同步。

### 12.7 Client retry 可能放大 tail latency

go-redis 默认 retry 若叠加 lookup timeout，故障时可能产生多次 dial/command 与更长尾延迟。本节显式关闭 command automatic retry，并限制 dial retry；业务层已有安全回源，不需要基础设施库悄悄重试。未来若对幂等 GETRANGE 开启 retry，必须把每次尝试纳入总 budget，并分别测 Redis 恢复收益与 MySQL 降级延迟。

### 12.8 观测限频本身需要生命周期

限频器持有按 kind 状态。kind 必须是固定枚举，否则 raw error 会让 map 无界增长；进程 restart 会清空 suppressed count，这是可接受的日志控制状态而非业务事实。未来 metrics backend 可累计 counter，但仍应避免 StrategyID label。

### 12.9 开发 Secret 轮换与缓存数据无关

轮换 credential 不需要迁移 projection；它只改变连接授权。正确顺序应允许新旧 identity 短暂共存或滚动实例，随后撤销旧 identity。把缓存 flush 当 Secret rotation 步骤没有安全收益，反而制造 cold storm。

### 12.10 Dependency placement 与供应链

Strategy adapter 依赖抽象 Store，使业务包不直接被某个 Redis SDK 类型污染；具体 client 仍是供应链依赖，需要：

- 锁定 go.mod/go.sum；
- 关注协议默认值、retry 与 timeout 的版本变化；
- 保持 RESP 版本显式；
- 镜像与 Redis tag 使用可审计版本；
- 构建 resource ARG 有默认和合法性校验。

“抽象了接口”不会消除依赖风险，只把变化限制在更小边界。

---

## 13. 假设与风险账本

状态含义：

- **已验证**：当前时间切片有自动化或真实环境证据；
- **部分验证**：机制成立，但真实规模/生产条件未知；
- **未验证**：目前只是设计假设；
- **接受并推迟**：风险真实存在，但没有足够需求或证据支持本节扩张。

| ID | 假设/风险 | 当前证据 | 失败后果 | 触发器与动作 | 状态 |
| --- | --- | --- | --- | --- | --- |
| A24-01 | Strategy 是读多写少 | 当前 runtime 只有 SELECT、无正式写用例，但这不构成生产读写比分布 | cache 收益不足或 stale 过多 | 第一条写用例前采集读写比并重开 ADR | 未验证 |
| A24-02 | MySQL 可完整重建 projection | Repository 两表恢复 + codec round trip | Redis 变成隐性事实源 | 每个新增 value 字段审查 provenance；无法重建则禁止入 cache | 已验证 |
| A24-03 | 五分钟 stale window 可接受 | 当前没有产品写/发布语义 | 未来活动变更延迟可见 | 产品定义 publish/read-after-write 后重新定 TTL/invalidation | 未验证 |
| A24-04 | 2 MiB 足够且不会造成资源风险 | unit 边界与 GETRANGE sentinel | 合法大 Strategy 频繁 bypass，或单 value 占用过大 | 收集 value histogram/p99，修改上限需容量与 DoS 评审 | 部分验证 |
| A24-05 | 1000 Awards 是合理上限 | 领域已有不变量 | 产品需求受限或 decode/selector 过重 | 真实产品提出 >1000 时先评估模型与 UX，不直接提常量 | 部分验证 |
| A24-06 | 单实例 same-key flight 足够 | deterministic concurrency test；M1 joined=0 | 多副本 cold storm 放大 MySQL | 副本数>1且 cold-load 压测出现 pool wait/error 时比较协调方案 | 部分验证 |
| A24-07 | Redis lookup 75 ms 值得等待 | 本机一次 M1 | 故障时增加每请求 tail latency | 生产相似网络测 hit/read-error 分布，按 outer budget 调整 | 未验证 |
| A24-08 | Redis failure 可安全 fail-open | Redis-down 500/500 HTTP 200；MySQL executes=1000 | MySQL 在 Redis outage 时过载 | Redis-down 饱和阶梯与 pool telemetry；必要时限并发/shedding | 部分验证 |
| A24-09 | warning 10 秒窗口足够 | Redis-down read_error_logs=1 | 过度抑制或日志洪泛 | 告警演练中验证首次、suppressed、恢复日志；调整为 metrics+sample | 部分验证 |
| A24-10 | allkeys-lru 只承载可丢弃 cache | 当前 Redis 服务专用于本节 keyspace | 未来非缓存 key 被 eviction | 任一新消费者加入前拆实例或重评 policy/ACL/memory | 已验证于当前拓扑 |
| A24-11 | no persistence 符合恢复目标 | restart 可 miss/refill | cold restart 暂时放大 MySQL | 测全冷启动；若无法承受，先改 source protection，不把 Redis 当备份 | 部分验证 |
| A24-12 | exact ACL 在真实启动中生效 | allow/deny probe 设计与 Compose 验收 | 越权命令/key 访问 | 每次 Redis/entrypoint 升级重跑正负探针 | 已验证于开发环境 |
| A24-13 | internal network 足够开发隔离 | 无 host port、network membership 检查 | 同网络恶意服务仍可探测 | 新服务入网时审查；生产叠加 TLS/网络策略 | 部分验证 |
| A24-14 | Secret 不泄露 | secret mount 与日志字段约束 | credential 泄露导致 prefix 内篡改 | secret scan、日志审计、rotation 演练 | 部分验证 |
| A24-15 | strict JSON 能拒绝旧/恶意值 | duplicate/unknown/depth/size/domain tests | poison hit 绕过领域约束 | codec fuzz、依赖升级与新 schema 兼容测试 | 已验证于已覆盖输入 |
| A24-16 | schema key `v1` 可安全演进 | key 与 payload 双 version | rollout 期间 miss/capacity 突增 | v2 前写兼容/读切换/旧 key TTL 计划，不原地放宽 v1 | 部分验证 |
| A24-17 | cache hit 与 direct source 领域等价 | 同一 Restore 边界和 round trip | selector 输入不同 | golden property tests 比较完整聚合；字段新增必须更新 codec | 已验证于 v1 |
| A24-18 | MySQL execute delta 是 source-load 可靠代理 | 当前每 load 两个 prepared SELECT | Repository 改查询后计数误读 | Repository 查询形态变化时更新证据解释或加 source counter | 部分验证 |
| A24-19 | M1 延迟趋势可作为开发回归提示 | 一次 500-request 三组基线 | 被误写成生产性能承诺 | 所有文档同时保留环境/样本/限制；性能结论需新实验 | 接受并推迟 |
| A24-20 | build resource defaults 足够当前依赖图 | 4 CPU/~1.92 GiB Docker build 通过 | CI/开发 OOM 或构建极慢 | 记录 peak RSS/time；依赖/runner 变化重测，ARG 可覆盖 | 部分验证 |
| A24-21 | 768MiB 是合理 Go soft limit | Go runtime 官方机制 + 本机通过 | 误认为硬 RSS cap | 文档明确 soft；需要硬隔离时使用 cgroup/runner limit | 已知限制 |
| A24-22 | caller values 可安全传入 shared fill | 当前 source 与 Principal 无关 | 未来授权/trace value 跨 caller 语义混淆 | source 一旦用户相关，取消共享或重定义 flight key/context | 部分验证 |
| A24-23 | direct DB 修改极少 | 当前开发 fixture 管理 | 合法 cache 可 stale 到 TTL | 禁止生产手改；未来写路径拥有 invalidation/version | 未验证 |
| A24-24 | Redis protocol/client defaults 稳定 | protocol/retry/timeout 显式配置 | 升级后命令、retry、error mapping 改变 | 依赖升级跑 integration/fault suite并审 changelog | 部分验证 |
| A24-25 | 权限数据不会误入本缓存 | package/codec 字段与停止线 | 授权撤销不生效、跨主体泄露 | code review 检查 projection；权限章节建立独立 Policy 边界 | 已验证于 v1 |

### 13.1 风险优先级

当前最高优先级不是“再加一个缓存算法”，而是三类触发器：

1. **第一次 Strategy 写/发布**：会使 TTL-only consistency 和旧值回填竞态成为真实问题；
2. **第一次多副本/冷启动负载**：会检验单进程 flight 是否足够；
3. **第一次 tenant/权限裁剪**：会检验单一 Strategy 投影能否在授权后安全复用。

它们到来前，继续增加分布式锁、L1 或 CDC 只会增加不可验证复杂度；到来后仍不重开设计，则是在拿旧假设冒充新事实。

---

## 14. 明确推迟项与重新评估条件

| 推迟项 | 现在不做的原因 | 重新评估条件 |
| --- | --- | --- |
| Strategy write invalidation | 没有真实写/发布 use case 与 version | 第一条 Update/Publish 需求 |
| Outbox/CDC/Binlog | 没有变更事件，也无跨消费者需求 | 写后跨服务投影或失效必须可靠传播 |
| Versioned business key | 当前只有技术 schema v1 | 聚合拥有单调业务版本并定义发布语义 |
| L1 local cache | 尚未证明 Redis RTT 是瓶颈 | profile 显示 Redis RTT/费用主导，且 invalidation 成熟 |
| soft/hard TTL + stale-while-revalidate | 当前 stale policy 尚未由产品定义 | availability 要求允许明确 stale window |
| proactive refresh/prewarm | 没有安全有界的热点/全量列表 | 冷启动测试证明 source storm，且有热点清单 |
| distributed singleflight/lock | 无副本级 stampede 证据 | 多副本 cold test 伤害 MySQL |
| negative cache | 无创建失效和穿透数据 | unknown-ID 攻击或成本被测量，且创建可精准失效 |
| Bloom Filter | Strategy 集合生命周期未定义 | 基数巨大且 negative lookup 主导 |
| Redis Cluster/Sentinel | 开发 cache 非关键且可重建 | 生产规模/拓扑/故障目标明确 |
| RDB/AOF | cache 不需要数据恢复 | 对象不再可重建时应先质疑是否还是 cache |
| multi-region cache | 无生产区域模型 | 跨区延迟、数据驻留和一致性要求明确 |
| per-tenant projection | 当前无 tenant model | 服务端 tenant authorization 与事实边界落地 |
| permission/eligibility cache | 撤销、scope、fail-closed 未设计 | 权限/资格模型成熟且独立威胁评审完成 |
| Draw/Result cache | 正式结果与幂等尚未实现 | 结果先持久化为事实，再设计读取投影 |
| Redis-based rate limit | 非本节读投影职责 | 边缘/服务端 rate-limit 威胁模型单独立项 |

推迟不是“永远不做”，而是给每个机制绑定事实触发器。这样路线演进来自需求和证据，而不是看到基础设施已安装就寻找使用场景。

---

## 15. 下一节边界：从共享配置转向用户相关决定

### 15.1 第 25 节不能直接复用 Strategy cache 的语义

下一节若进入用户资格、参与约束或其他用户相关能力，最先要问的不是 Redis key 怎样加 user ID，而是：

- Principal 从哪里来，是否已经真实认证；
- 资格事实存在哪里；
- 资格由哪些输入、时间点与规则版本决定；
- 撤销要多快生效；
- dependency failure 应 fail-open 还是 fail-closed；
- tenant/scope 怎样隔离；
- 决策是否要审计；
- 同一用户并发请求怎样处理；
- 资格通过与一次选择是否需要原子关系。

在这些问题回答前，用户资格、Permission、PolicyDecision 和临时结果都必须走自己的权威路径，不能塞入 `growthos:*:lottery:strategy:projection:v1:*`，也不能共用 Strategy TTL/ACL/observer 语义。

### 15.2 第一次真实 Strategy 写入前的强制门

任何新增 Strategy Create/Update/Delete/Publish 代码之前，应先写新的 ADR，至少决定：

1. 聚合业务 version 从哪里产生；
2. Award 子行变化如何推动 version；
3. commit 与 invalidation 的原子/最终一致性；
4. 旧 source reader 是否会 stale refill；
5. 发布失败、重试、rollback 与 unknown outcome；
6. v1 key 是 DEL、versioned key 还是双读；
7. read-after-write 与最大 stale 承诺；
8. 多实例传播；
9. 审计与操作者权限；
10. 相应故障注入和迁移/回滚。

没有这道门，就不应在本节 Reader 上随手加 `Invalidate(id)` 并声称问题解决。

### 15.3 保持线性演进的原则

本节为后续留下的是一个已验收 tip：

    sole-truth MySQL
      + strict rebuildable Strategy projection
      + bounded cache-aside failure semantics
      + direct bypass
      + real fault/load evidence

下一节从这个事实继续，但只新增自己的一个核心能力。若遇到不属于原计划却成为前置条件的公共组件，应新增标准编号小节，明确动机、边界、QA、设计手记和验收；不能把多个威胁模型塞进“24A/24B/24C”，也不能因为用户刚提到权限就在缓存章节临时拼装 RBAC。

---

## 16. 代码、契约与文档追踪

### 16.1 代码追踪

| 设计命题 | 实现/证据入口 |
| --- | --- |
| application-owned reader port | [application lottery contracts](../../../internal/lottery/application) |
| cache-aside state machine | [strategycache reader](../../../internal/lottery/adapter/strategycache/reader.go) |
| strict JSON v1 | [strategycache codec](../../../internal/lottery/adapter/strategycache/codec.go) |
| per-key flight/cancellation | [strategycache flight](../../../internal/lottery/adapter/strategycache/flight.go) |
| Store boundary and tests | [strategycache package](../../../internal/lottery/adapter/strategycache) |
| concrete Redis client | [redisstore client](../../../internal/infrastructure/redisstore/client.go) |
| typed Redis settings | [redisstore config](../../../internal/infrastructure/redisstore/config.go) |
| app config validation | [appconfig Redis settings](../../../internal/platform/appconfig/redis.go) |
| lifecycle/composition | [growth-api database wiring](../../../cmd/growth-api/database.go) |
| low-cardinality observer | [cache observer](../../../cmd/growth-api/cache_observer.go) |
| Compose Redis topology | [compose.yaml](../../../deploy/compose/compose.yaml) |
| ACL bootstrap | [redis-entrypoint.sh](../../../deploy/docker/redis-entrypoint.sh) |
| constrained Go build | [backend Dockerfile](../../../deploy/docker/Dockerfile.backend) |
| real fault/HTTP acceptance | [compose lottery API acceptance](../../../scripts/compose-lottery-api-acceptance.sh) |

### 16.2 课程与验收追踪

- 上一节边界：[第 23 节设计手记](lesson-23.md)
- 长期决策：[ADR-0020：Lottery Strategy cache-aside](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)
- 课程正文：[第 24 节课程](../../course/part-03/lesson-24-redis-strategy-cache.md)
- API/运行契约：[第 24 节 API 文档](../../api/lessons/lesson-24.md)
- QA 与真实证据：[第 24 节 QA](../../qa/lessons/lesson-24.md)
- 面试问答：[第 24 节面试问答](../../interview/lessons/lesson-24.md)
- 本地运维：[Local Compose Runbook](../../runbooks/local-compose.md)
- 配置说明：[Configuration](../../configuration.md)

### 16.3 外部技术资料

以下资料均访问于 **2026-08-30**：

1. Redis, [Cache-aside pattern](https://redis.io/docs/latest/develop/use-cases/cache-aside/)：用于校准应用负责 lookup/miss/source/fill 的基本控制流；本项目另外定义 sole truth、strict restore 和错误优先级。
2. Redis, [Access Control List](https://redis.io/docs/latest/operate/oss_and_stack/management/security/acl/)：用于命令、key pattern 和用户最小权限设计；不能替代产品 RBAC。
3. Redis, [`EXPIRE`](https://redis.io/docs/latest/commands/expire/)：用于物理 key 过期语义。
4. Redis, [`TTL`](https://redis.io/docs/latest/commands/ttl/)：用于理解剩余寿命与 key 状态语义；runtime ACL 未授权该命令，本轮也未执行真实 TTL probe，因此它不能被当作本节已取得的运行证据或业务版本。
5. Redis, [Key eviction](https://redis.io/docs/latest/develop/reference/eviction/)：用于理解 maxmemory、policy 与并非所有进程内存都计入 eviction 的边界。
6. Go, [A Guide to the Go Garbage Collector — Memory limit](https://go.dev/doc/gc-guide#Memory_limit)：用于解释 `GOMEMLIMIT` 是 Go runtime 的 soft limit，而不是容器 RSS 硬上限。
7. Go command, [Compile packages and dependencies](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies)：用于解释 `go build -p` 的并行 package 构建约束。

引用官方资料是为了确认机制语义，不是把官方示例直接升级成本项目的业务正确性。最终边界仍必须由本地 code、tests、ACL probe、fault injection 和 M1 证据共同证明。

---

## 17. 本节最终结论

完成第 24 节后，可以严谨地说：

> GrowthOS-Go 已在不改变公开 Lottery 契约的前提下，为 Strategy/Award 权威读取增加严格 JSON v1 Redis cache-aside 投影。MySQL 仍是唯一事实源；cache miss、坏值和 Redis 运行故障回源；write/delete failure 不影响权威成功；同进程同 key 的重叠 source load 被合并；Redis 不参与 readiness；ACL、Secret、internal network、TTL、value size、观测基数与构建资源均有明确边界。一次本机 M1 证明 warm hit 跳过 MySQL，Redis-down 能回源，但不构成生产性能、容量或 SLO 结论。

仍然不能说：

- 系统已经拥有强一致或精准失效的 Strategy cache；
- 五分钟 TTL 已得到产品确认；
- 多副本 cold storm 已解决；
- Redis 是高可用、持久或生产定标的；
- 用户资格、权限、库存或正式结果可以复用本缓存；
- 一次 500 请求实验证明生产延迟收益；
- `GOMEMLIMIT=768MiB` 是构建进程硬内存上限；
- 缓存命中等于业务授权或业务成功。

本节真正值得保留的能力不是 Redis 本身，而是一条可检查的原则：

> **任何加速层都必须在事实、权限和失败语义之外；只有能被权威源完整重建、能被严格验证、能被安全丢弃的数据，才有资格进入本缓存。**
