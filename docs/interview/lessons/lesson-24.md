# 第 24 节面试题：Redis Strategy Cache-Aside、降级与 M1 基线

本文面向“Redis 缓存设计 + Go 并发 + 故障降级 + 项目压测深挖”类面试。第 24 节已经把 Redis 接入真实的 Lottery Strategy 读取链，但缓存对象仍只是可由 MySQL 重建的 Strategy/Award 投影；一次 HTTP 响应依然是不可持久化、不可恢复、不能安全重试的 ephemeral selection。回答时要把已实现机制、单机实测和未来方案分开。

## 60 秒项目自述

> 第 24 节把 Redis 从 Compose 占位能力变成第一个真实业务消费者。我保留 application-owned `StrategyReader` 端口，让 MySQL Repository 继续作为唯一权威读取，再在组合根用 Redis cache-aside decorator 包装它。命中时先用最大 2 MiB 的有界 `GETRANGE` 读取 v1 投影，严格校验 schema、完整正 `uint64` 字符串、StrategyID、Award 数量与领域不变量；miss、坏值和 Redis 运行故障都在短预算内回源 MySQL，成功后用带 4 分 30 秒到 5 分钟 TTL 的原子 `SET` best-effort 回填。坏值会精确删除，同进程同 key 的重叠回源由自有生命周期的 singleflight 合并，但每个请求仍独立执行随机选择。
>
> 基础设施上使用独立 `growthos_api` Redis 用户、精确 key prefix、`resetchannels -@all +ping +getrange +set +del`、文件 Secret 和只供 API/Redis 使用的 internal cache network；Redis 不进入 API readiness。2026-08-30 的 Mac Docker Desktop 单机单次 M1 中，三组各 500 个请求均成功：warm cache 有 500 次 hit、0 次 MySQL execute；cache disabled 和 Redis down 都有 500 次 source load、1000 次 MySQL execute。延迟只是一轮开发基线，不是生产容量或 SLO。

## 来源与可信度

- **项目事实：** 只来自当前仓库代码、[ADR-0020](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)、测试和第 24 节实际验收记录。ADR 约束不自动等于某条命令已执行；运行事实以 QA/acceptance 记录为准。
- **官方技术事实：** 主要使用 Redis 官方的 [cache-aside](https://redis.io/docs/latest/develop/use-cases/cache-aside/)、[ACL](https://redis.io/docs/latest/operate/oss_and_stack/management/security/acl/)、[`GETRANGE`](https://redis.io/docs/latest/commands/getrange/)、[`SET`](https://redis.io/docs/latest/commands/set/)、[`TTL`](https://redis.io/docs/latest/commands/ttl/)、[eviction](https://redis.io/docs/latest/develop/reference/eviction/)、[client-side caching](https://redis.io/docs/latest/develop/clients/client-side-caching/)与[分布式锁](https://redis.io/docs/latest/develop/clients/patterns/distributed-locks/)文档，以及 Go 官方的 [`singleflight`](https://pkg.go.dev/golang.org/x/sync/singleflight)、[`go build -p`](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies)、[`GOMEMLIMIT`](https://go.dev/doc/gc-guide#Memory_limit)和[容器感知 `GOMAXPROCS`](https://go.dev/blog/container-aware-gomaxprocs)资料。
- **面经题型启发：** [番茄小说后端一面](https://www.nowcoder.com/discuss/893833080824164352)、[百度 Go 一面](https://www.nowcoder.com/discuss/739562005740130304)和[淘天秋招 Java 一面](https://www.nowcoder.com/discuss/813867782348570624)都是牛客用户发布的公开个人自述。它们只说明候选人记录过缓存灾难、一致性、本地缓存、singleflight 和分布式锁等追问；真实性未独立核验，也不是公司官方题库或技术结论来源。
- 外部链接复核日期为 **2026-08-30**；本文只概述题型，不复制长段内容。

---

## 1. 为什么第一个缓存对象选择 Strategy，而 MySQL 仍是唯一事实源？

- **直接回答：** Strategy/Award 是跨请求复用、读多写少、能从两张 MySQL 表和领域恢复函数完整重建的配置聚合。Redis 在这里适合做可丢弃读取投影：hit 跳过权威读取，miss 回 MySQL 并回填；删除全部 key、淘汰或重启 Redis 都不能造成业务事实丢失。Redis `SET` 成功也不能反向证明 MySQL 状态，更不能让 Redis 获得 Create/Update/Publish 所有权。
- **追问：** Redis 比 MySQL 快，为什么不把 Redis 设成主存储？
  - **追问回答：** 速度不等于事实所有权。当前 Repository 用同一只读 Repeatable Read 快照恢复完整聚合，Migration、约束和运维恢复都围绕 MySQL；Redis 则明确关闭持久化并允许 LRU 淘汰。把它升级为主存储会引入写入确认、持久化、恢复、历史版本和跨副本一致性的新问题，已经超出本节目标。
- **项目证据：** [cache-aside Reader](../../../internal/lottery/adapter/strategycache/reader.go)、[MySQL Repository](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[ADR-0020](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)。
- **选型边界：** 如果未来权威模型改为事件流、不可变发布快照或另一种数据库，应重画事实边界；“缓存可以重建”仍是最低要求，不能因为命中率高就静默倒置主从关系。
- **来源：** 项目事实；Redis 官方 [cache-aside](https://redis.io/docs/latest/develop/use-cases/cache-aside/)描述了先查缓存、miss 回主存储并回填的模式。

## 2. 为什么缓存做成 `StrategyReader` decorator，而不塞进 MySQL Repository、domain 或 HTTP handler？

- **直接回答：** `StrategyReader` 是 application consumer 拥有的窄端口。MySQL Repository 只负责权威持久化，cache decorator 只负责 key、codec、TTL、回源与降级，domain 不导入 Redis/JSON，HTTP handler 也不知道是否命中。组合根可以在启用时注入 decorator、禁用时直接注入 Repository，因此业务契约和缓存策略可以分别测试、撤销和演进。
- **追问：** 多一层类型是不是过度设计？
  - **追问回答：** 这一层承载了独立失败语义和生命周期，不只是转发。若塞进 Repository，Redis error、MySQL error、codec error 与事务语义会耦合；若塞进 handler，其他用例无法复用且 transport 会拼 key。当前层次正好对应一个可独立关闭的策略边界。
- **项目证据：** [组合根](../../../cmd/growth-api/database.go)、[application 端口](../../../internal/lottery/application/repository.go)、[Reader 构造与校验](../../../internal/lottery/adapter/strategycache/reader.go)。
- **选型边界：** 当多个上下文出现不同对象、时效和失败语义时，不应抽成 `common/cache` 万能接口；只有重复机制稳定且不抹平领域差异时才考虑共享基础设施。
- **来源：** 项目事实；Redis 官方 cache-aside 只定义访问模式，不要求把缓存嵌入某个数据库 adapter。

## 3. 一次 cache-aside 读取的完整控制流是什么？

- **直接回答：** 先生成规范 key，再在 lookup budget 内执行 `GETRANGE 0..2MiB`。合法命中经严格 codec 和领域恢复后返回；miss、Redis 运行错误或坏值进入同 key source-load group。MySQL 成功后编码并用带 TTL 的 `SET` best-effort 回填；编码或写回失败仍返回该次 MySQL 结果。MySQL not-found、stored-invalid、普通失败、deadline/cancel 保留原分类，不能被前面的缓存错误覆盖。
- **追问：** Redis miss 为什么不能直接返回 Strategy not found？
  - **追问回答：** miss 只说明派生投影不存在，可能是首次访问、过期、淘汰、重启或手工删除；只有权威 MySQL 读取才能判断业务不存在。把两者合并会让缓存状态改变 HTTP 业务语义。
- **项目证据：** [Reader `FindByID/read/fill`](../../../internal/lottery/adapter/strategycache/reader.go)、[Redis Store adapter](../../../internal/infrastructure/redisstore/client.go)、[Reader 控制流测试](../../../internal/lottery/adapter/strategycache/reader_test.go)。
- **选型边界：** 对必须强制经过最新权威判断的对象可以直接 bypass cache；对允许 stale 服务的对象也必须先写清 staleness 和失败语义，不能默认套用本节策略。
- **来源：** 项目事实；Redis 官方 [cache-aside](https://redis.io/docs/latest/develop/use-cases/cache-aside/)和 [`SET`](https://redis.io/docs/latest/commands/set/)命令文档。

## 4. 当前缓存一致性承诺是什么？未来有 Strategy 更新时怎样失效？

- **直接回答：** 当前没有 Strategy Update/Delete/Publish、聚合业务版本或变更事件，所以只承诺“合法旧投影最多服务到该 key 剩余 TTL”，不承诺 read-after-write、单调读或跨实例同时刷新。外部绕过应用修改 MySQL 后，旧值可能继续存在到物理过期。官方 cache-aside 的常见更新路径是先写权威存储，再失效缓存，但本项目尚未实现这条写路径，不能把建议说成已完成。
- **追问：** 以后简单地“更新 MySQL 后 DEL”就一定一致吗？
  - **追问回答：** 不一定，还要处理数据库提交成功但 DEL 失败、并发旧请求在失效后回填旧值、发布版本、删除与历史 Draw 引用等问题。未来至少要决定聚合业务版本或不可变发布快照、CAS/旧值拒写、失效重试或事件/outbox，以及滚动版本兼容，并另立 ADR。
- **项目证据：** [ADR-0020 一致性边界](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)、[当前 SELECT-only 组合](../../../cmd/growth-api/database.go)、[两张现有 Migration](../../../migrations/sql)。
- **选型边界：** 业务要求更短陈旧窗口、read-after-write 或正式 Draw 历史回放时，TTL-only 已不够；不能为了命中率把 5 分钟上限直接拉长，也不能用根行 `updated_at` 冒充聚合版本。
- **来源：** 项目事实；Redis 官方 [cache-aside](https://redis.io/docs/latest/develop/use-cases/cache-aside/)说明更新主存储后按 key 失效的基本路径。

## 5. 为什么默认 TTL 是 5 分钟并减去 0～10% 抖动？

- **直接回答：** 可配置 `base_ttl` 必须在 `(0, 5m]`，默认和最大值都是 5 分钟；每次 fill 减去 `[0, base_ttl×10%]` 抖动，所以初始 TTL 位于 `[90%, 100%]`，默认即 4 分 30 秒到 5 分钟。value 和 expiration 通过同一个 `SET` 写入，Store 还拒绝非正 expiration，避免永久 Strategy key。减法抖动不会突破已承诺的最大陈旧上界。
- **追问：** 加了 jitter 就解决缓存雪崩了吗？
  - **追问回答：** 没有。它只降低一批同时创建 key 集中过期的概率；单个极热 key、Redis 整体故障、多实例同时 miss 和 MySQL 容量仍未解决。业务随机选择使用独立的 CSPRNG；TTL jitter 是可注入、可夹紧的运行机制随机量，不能影响 Award 概率。
- **项目证据：** [TTL 与 jitter 实现](../../../internal/lottery/adapter/strategycache/reader.go)、[确定性 jitter 测试](../../../internal/lottery/adapter/strategycache/reader_test.go)、[配置范围](../../../internal/platform/appconfig/redis.go)。
- **选型边界：** 真实变更频率、命中率、value 分布和可接受陈旧窗口出现后可以缩短 TTL；突破 5 分钟或引入软/硬双 TTL、提前刷新必须重新评审。
- **来源：** 项目事实；Redis 官方 [`SET`](https://redis.io/docs/latest/commands/set/)、[`TTL`](https://redis.io/docs/latest/commands/ttl/)与 [`EXPIRE`](https://redis.io/docs/latest/commands/expire/)文档。

## 6. Redis key 为什么这样设计，`v1` 又代表什么？

- **直接回答：** key 固定为 `growthos:<environment>:lottery:strategy:projection:v1:<strategy_id>`。environment 只能是 `development|test|staging|production` 的受控配置枚举；ID 用 `strconv.FormatUint(id, 10)` 生成无前导零的规范十进制；`v1` 是 key/payload schema version，不是 Strategy 业务版本、Migration 版本或 Git SHA。key 不含用户、名称、IP、权限或随机结果。
- **追问：** 为什么 schema version 同时进入 key 和 payload？
  - **追问回答：** key version 让新旧格式在滚动期间隔离并自然过期，payload version 防止错误 namespace 或手工写入被旧 decoder 误解；二者仍不能提供业务发布身份。请求只提供 StrategyID，namespace 和环境不能由客户端拼接。
- **项目证据：** [key builder 与 namespace 白名单](../../../internal/lottery/adapter/strategycache/reader.go)、[Compose development ACL prefix](../../../deploy/docker/redis-entrypoint.sh)、[最大 ID acceptance](../../../scripts/compose-lottery-api-acceptance.sh)。
- **选型边界：** 进入 Cluster、跨地域或多租户前要重新评审 slot/hash tag、租户隔离和 key cardinality；不能把当前 development prefix 复制成 `growthos:*` 的宽授权。
- **来源：** 项目事实；Redis 官方 [ACL key pattern](https://redis.io/docs/latest/operate/oss_and_stack/management/security/acl/)规则。

## 7. 为什么缓存 value 要使用严格独立 codec，完整 `uint64` 为什么编码成字符串？

- **直接回答：** Redis value 是跨进程、跨发布可见的不可信外部输入，不能直接给 domain 增加 JSON tag 后反序列化。v1 codec 要求精确且大小写敏感的字段集合，拒绝未知字段、缺字段、重复 JSON name、trailing document、过深结构和未知 schema；Strategy/Award ID 与 Weight 用规范十进制 string，无损覆盖 `1..MaxUint64`，再经 `RestoreAward`/`RestoreStrategy` 恢复名称、Outcome、重复 ID、正 Weight、总权重和最多 1000 个 Award 等不变量。
- **追问：** JSON number 也能表示整数，为什么不用 number？
  - **追问回答：** JSON 标准本身不限定消费者精度，JavaScript 的安全整数范围也小于 `uint64`。用 canonical decimal string 能与现有 HTTP identity 契约一致，并拒绝符号、前导零、指数写法和精度收窄；acceptance 已用 `18446744073709551615` 验证 key、payload 和响应都保持字符串。
- **项目证据：** [严格 codec](../../../internal/lottery/adapter/strategycache/codec.go)、[codec 边界测试](../../../internal/lottery/adapter/strategycache/codec_test.go)、[领域恢复函数](../../../internal/lottery/domain/strategy.go)。
- **选型边界：** 新字段必须通过新 schema/key version 或明确兼容规则发布，不能原地放宽 v1；若改成二进制协议，也仍要保留大小、identity 和领域恢复边界。
- **来源：** 项目事实；Go 官方 [`encoding/json`](https://pkg.go.dev/encoding/json)契约用于校准 decoder 行为。

## 8. 为什么读取用 `GETRANGE` 而不是 `GET`？poison value 怎样处理？

- **直接回答：** 最大投影为 2 MiB。代码调用 inclusive end 的 `GETRANGE key 0 2097152`，最多取回“上限 + 1 byte”；若长度超过 2 MiB，可在 JSON 解码前拒绝，而不是把任意大 Redis string 全量载入进程。空值、超大值、坏 JSON、未知 schema、ID 不匹配或领域恢复失败都记为 `corrupt`，先 best-effort 精确 `DEL` 一个 key，再回源 MySQL并尝试修复。DEL 失败不能阻止权威读取，MySQL 失败时也绝不服务 poison。
- **追问：** 只限制网络返回大小，Redis 里不是仍可能存在大 key 吗？
  - **追问回答：** 是。`GETRANGE` 保护单次客户端读取和解码预算，不替代 Redis dataset 内存治理；ACL、2 MiB encoder 上限、`maxmemory`、淘汰观测和受控写入共同降低风险。若其他有权限的客户端可写同 prefix，信任边界已经被破坏。
- **项目证据：** [有界读取与坏值删除](../../../internal/lottery/adapter/strategycache/reader.go)、[Redis `GetRange` adapter](../../../internal/infrastructure/redisstore/client.go)、[poison 修复 acceptance](../../../scripts/compose-lottery-api-acceptance.sh)。
- **选型边界：** value 分布接近 2 MiB 或解码 CPU 变成瓶颈时，应缩紧对象模型、拆投影或改协议；不能未经内存/网络证据直接扩大上限。
- **来源：** 项目事实；Redis 官方 [`GETRANGE`](https://redis.io/docs/latest/commands/getrange/)说明端点为包含式并按返回子串长度计成本。

## 9. 缓存穿透、击穿、雪崩有什么区别？本节分别做到哪一步？

- **直接回答：** 穿透是查询缓存和权威源都不存在的 key，因无法回填而反复打 MySQL；当前明确保留该风险。击穿是一个存在的热点 key 过期时出现并发回源；当前只用进程内 same-key flight 合并重叠 source load。雪崩是大量 key 集中过期或 Redis 整体不可用造成大范围回源；当前 TTL jitter 只缓解集中到期，Redis-down fail-open 保持功能路径，但没有证明 MySQL 能承受生产洪峰。
- **追问：** 为什么不能把三种问题都回答成“加锁”？
  - **追问回答：** 它们的触发条件和保护对象不同。不存在 key 加锁后仍会查询权威源；大量 key 或 Redis outage 需要容量、限流、breaker/backpressure 等系统级保护；热点单 key 才可能受请求合并或互斥保护。方案必须对应实际流量和失败域。
- **项目证据：** [ADR 风险矩阵](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)、[singleflight 实现](../../../internal/lottery/adapter/strategycache/flight.go)、[Redis-down acceptance](../../../scripts/compose-lottery-api-acceptance.sh)。
- **选型边界：** 当前没有多副本、恶意枚举或大规模同时过期实测，不能声称已解决三大缓存灾难；出现 source-load 峰值和下游容量数据后再选择限流、预热、提前刷新、Bloom Filter 或跨实例协调。
- **来源：** 技术边界来自项目与 Redis 官方 [cache-aside](https://redis.io/docs/latest/develop/use-cases/cache-aside/)；题型来自牛客用户发布的[番茄小说后端一面](https://www.nowcoder.com/discuss/893833080824164352)个人自述，真实性未独立核验。

## 10. 为什么本节不做 not-found 负缓存？

- **直接回答：** MySQL not-found、timeout、stored-invalid、普通错误和取消都不写 Redis。同一 flight 的重叠等待者可以暂时共享一次 source error/not-found，但 group 完成即删除，不形成跨请求负缓存。原因是当前没有 Strategy 发布版本、可信不存在证明和创建后的精准失效；缓存空值会让刚创建的 Strategy 在负 TTL 内继续返回 404，也可能把临时故障固化成业务不存在。
- **追问：** 那未知 ID 枚举不是会一直穿透吗？
  - **追问回答：** 是，这是公开剩余风险，不应假装消失。先测 unknown 比例、重复度、主体与滥用模型，再比较入口准入/限流、短 TTL versioned negative cache 或 Bloom Filter；Bloom Filter 也要定义权威集合更新和误判边界。
- **项目证据：** [不写 source error 的 Reader](../../../internal/lottery/adapter/strategycache/reader.go)、[不负缓存测试](../../../internal/lottery/adapter/strategycache/reader_test.go)、[ADR-0020 非目标](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)。
- **选型边界：** 若存在不可变发布目录、稳定高比例重复 unknown 请求和清晰创建失效事件，短时负缓存可能值得；仍不能把 dependency error 与 not-found 共用一个 value。
- **来源：** 项目事实；Redis 官方 cache-aside 只提供模式能力，不替项目决定负缓存业务语义。

## 11. 这个 singleflight 怎样处理 caller 取消、leader 超时和不同 key？

- **直接回答：** group 只包围权威 source load，以 canonical cache key 分组。同进程同 key 的重叠调用最多保留一个 in-flight fill；不同 key 独立。每个等待者用自己的 context，取消后立即返回；共享 fill 使用 `context.WithoutCancel` 脱离首个 caller 的取消，再叠加硬 `FillTimeout` 和进程 lifecycle cancellation，因此一个急躁 caller 不会毒化其他等待者，后台工作也不会无界存活。
- **追问：** 为什么不直接使用 `x/sync/singleflight.Group.Do`？
  - **追问回答：** 官方包提供同 key 重复调用抑制，但本项目还要让每个 waiter 独立取消、把 fill 生命周期与首个 caller 分离并绑定进程 shutdown。自定义实现不是“更高级”，而是为这些已测试语义付出额外维护成本；需要持续 race/cancellation 测试。
- **项目证据：** [flight group](../../../internal/lottery/adapter/strategycache/flight.go)、[并发、取消、超时和 lifecycle 测试](../../../internal/lottery/adapter/strategycache/reader_test.go)。
- **选型边界：** 它只在一个进程内生效。多 API 副本、进程崩溃和跨主机热点仍会各自回源；只有实际副本压测证明必要时才升级方案。
- **来源：** 项目事实；Go 官方 [`singleflight`](https://pkg.go.dev/golang.org/x/sync/singleflight)将其定义为 duplicate function call suppression。

## 12. 进程内 singleflight 与 Redis 分布式锁有什么本质区别？

- **直接回答：** singleflight 是进程内同 key 的临时函数调用合并，没有网络、租约、锁身份或跨实例互斥。Redis 分布式锁面向多个进程共享资源，需要唯一 owner value、`SET NX PX`、到期、只删除自己持有的锁、续期/失败恢复，正确性敏感场景还要考虑 fencing 和故障模型。第 24 节只是减少重复回源，不需要把 cache correctness 建立在 Redis 锁上。
- **追问：** 为什么 Redis 已经在用，不顺手加锁把多实例击穿也解决？
  - **追问回答：** Redis 正在故障时，锁服务也会故障；锁会扩大命令 ACL、脚本、等待和观测面。当前只有单实例 M1，没有多副本 source-load 证据，先引入锁是在为假设付复杂度。现有 ACL 也明确拒绝 `EVAL`、`GET` 和任意锁 namespace。
- **项目证据：** [ADR-0020 方案七](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)、[最小 ACL](../../../deploy/docker/redis-entrypoint.sh)、[ACL 负向 acceptance](../../../scripts/compose-lottery-api-acceptance.sh)。
- **选型边界：** 多副本热点回源确实超过 MySQL 预算时，应同时比较提前刷新、软/硬 TTL、下游限流和跨实例协调；若锁保护副作用而非缓存回填，必须用更严格的一致性/fencing 设计。
- **来源：** Redis 官方[分布式锁](https://redis.io/docs/latest/develop/clients/patterns/distributed-locks/)明确讨论 owner token、租约和故障限制；题型来自牛客用户发布的[番茄小说后端一面](https://www.nowcoder.com/discuss/893833080824164352)与[淘天秋招 Java 一面](https://www.nowcoder.com/discuss/813867782348570624)个人自述，真实性未独立核验。

## 13. 为什么 singleflight 只共享 Strategy 读取，不能共享随机选择或缓存 selection result？

- **直接回答：** cache 和 flight 只提供一份合法 Strategy 输入；每个 `EphemeralSelectionService.Select` 仍各自调用 `WeightedSelector` 和随机源。若共享 selected Award，就会把“同一配置读取”错误变成“多个请求复用一次随机结果”。当前响应没有持久化 request identity、Draw/Result 或幂等协议，缓存它会伪造可恢复性与安全重试。
- **追问：** 同一个用户重试时返回相同结果不是更友好吗？
  - **追问回答：** 只有正式 Draw 以幂等 key、持久化状态、库存/权益协议和 outcome-unknown 恢复实现后，才能安全返回同一业务结果。当前明确拒绝 `Idempotency-Key`，所以重试可能产生新随机候选，前端也必须展示 ephemeral durability。
- **项目证据：** [应用用例](../../../internal/lottery/application/ephemeral_selection.go)、[“只合并读取、不合并选择”测试](../../../internal/lottery/adapter/strategycache/reader_test.go)、[ephemeral API ADR](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)。
- **选型边界：** 正式抽奖出现后，应持久化 Draw/Result 并按业务 identity 查询，而不是把随机响应塞进 Strategy cache；两种缓存的正确性、隐私和保留期完全不同。
- **来源：** 项目事实。

## 14. Redis timeout、retry、caller deadline 和 fail-open 如何组成一个预算？

- **直接回答：** Compose 的 selection 总预算是 3 秒；Strategy cache 默认 lookup 75ms、write 75ms、fill 2s，配置还校验三者之和不超过 selection timeout。Redis client 使用 context timeout、有限连接池，普通命令自动 retry 关闭，避免每个请求先长时间卡 Redis再访问 MySQL。cache fail-open 只表示 Redis 运行错误后尝试权威 MySQL；caller cancel/deadline 始终优先，source error 也不能被 cache error 覆盖。
- **追问：** Redis 配置错误也应该 fail-open 吗？
  - **追问回答：** 不应该。非法地址、username、TTL、timeout、pool、TLS 模式、缺失或冲突 Secret 属于 operator mistake，应在配置/组合阶段失败。配置有效但 Redis 网络、ACL、OOM 或命令运行失败才 bypass。client 构造是 lazy 的，不用启动 `PING` 把可选加速层变成硬依赖。
- **项目证据：** [配置校验](../../../internal/platform/appconfig/redis.go)、[go-redis 有界配置](../../../internal/infrastructure/redisstore/config.go)、[Reader timeout 控制流](../../../internal/lottery/adapter/strategycache/reader.go)。
- **选型边界：** fail-open 会把 Redis outage 的流量转给 MySQL；当前没有完整 breaker、backpressure 或 shedding。若 degraded source load 超过预算，应先保护权威库，而不是只继续缩短 Redis timeout。
- **来源：** 项目事实；Redis 官方 [go-redis production usage](https://redis.io/docs/latest/develop/clients/go/produsage/)用于校准 timeout、pool 和连接使用边界。

## 15. 为什么 Redis 不进入 `/ready`？MySQL 宕机时 warm hit 又为什么还能成功？

- **直接回答：** readiness 表示当前权威 MySQL 依赖是否可用，Redis只是加速层。Redis down、MySQL up 时请求回源成功且 `/ready` 仍 200；MySQL down 时 `/ready` 必须 503，即使某个合法未过期 warm projection 仍能完成一次 selection。冷 key 或不存在 key 在 MySQL down 时返回既有技术失败，不能用空 Strategy 或 `no_reward` 掩盖。
- **追问：** warm hit 能成功，readiness 503 不是矛盾吗？
  - **追问回答：** 不是。单个读请求能从有界陈旧投影服务，不代表系统能读取任意 Strategy、完成迁移或恢复权威状态。readiness 保守地反映 sole truth 不可用，负载均衡是否继续送流量是更上层策略。
- **项目证据：** [HTTP readiness 只注入 database](../../../cmd/growth-api/main.go)、[Redis/MySQL 故障 acceptance](../../../scripts/compose-lottery-api-acceptance.sh)、[ADR-0020 降级边界](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)。
- **选型边界：** 若未来产品明确允许某类 stale-only 模式，需要单独定义覆盖率、最大陈旧、写禁用和流量策略；不能从本次 warm hit 外推业务可用性 SLO。
- **来源：** 项目事实。

## 16. Redis ACL 为什么同时需要 command、key pattern 和 `resetchannels`？

- **直接回答：** ACL 是三条独立维度：能执行哪些命令、能访问哪些 key、能访问哪些 Pub/Sub channel。当前关闭 default user，业务用户精确为 `resetkeys ~growthos:development:lottery:strategy:projection:v1:* resetchannels -@all +ping +getrange +set +del`。`resetchannels` 明确清空频道权限，防止虽然没打算用 Pub/Sub，却因默认/历史配置得到 channel 访问。
- **追问：** 有 `-@all` 后为什么还要真实做负向测试？
  - **追问回答：** ACL 规则按顺序生效，配置文本、未来模块命令和 key/channel 维度都可能漂移。acceptance 实际证明允许 PING/SET with EX/GETRANGE/DEL，同时拒绝同 prefix 的 GET、prefix 外 SET、SCAN、CONFIG、ACL、EVAL、SUBSCRIBE、PUBLISH，并验证 default user 不能认证。
- **项目证据：** [ACL 生成入口](../../../deploy/docker/redis-entrypoint.sh)、[长期 smoke ACL 检查](../../../scripts/compose-smoke.sh)、[隔离 acceptance ACL 检查](../../../scripts/compose-lottery-api-acceptance.sh)。
- **选型边界：** 新增命令、schema 或业务消费者时必须增加独立用户/keyspace 并重算 allowlist；共享 Redis 进程不等于共享权限。Redis ACL 也不是面向产品用户的 RBAC。
- **来源：** 项目事实；Redis 官方 [ACL](https://redis.io/docs/latest/operate/oss_and_stack/management/security/acl/)定义 `-@all`、`~pattern`、`resetkeys` 和 `resetchannels` 的独立语义。

## 17. Secret、internal cache network 和 TLS 各解决什么问题？

- **直接回答：** Compose 中只有 API 与 Redis 连接 Docker-internal `cache` network并挂载 Redis Secret；Redis 不发布宿主机端口，Web/MySQL/Migrate/mysql-grants 都不持有该 Secret。入口脚本只接受 64 位小写十六进制文件内容，配置/错误的常见格式化边界会 redaction。development 内网明确配置 TLS disabled；staging/production 启用 cache 时配置门要求 `verify_identity`，可加载受限大小的 CA 并校验主机身份。
- **追问：** internal network + 密码是否已经等于生产安全？
  - **追问回答：** 不是。它只是本地最小暴露边界，不包含生产 Secret Manager、轮换、主机隔离、网络策略、mTLS、审计或 Redis HA。容器内进程一旦被攻破仍可能使用已挂载凭据执行 allowlist 命令，所以 ACL 和 payload 校验仍必要。
- **项目证据：** [Compose topology](../../../deploy/compose/compose.yaml)、[Secret 生成器](../../../scripts/generate-compose-secrets.sh)、[Redis TLS/配置 redaction](../../../internal/infrastructure/redisstore/config.go)。
- **选型边界：** staging/production 必须按实际证书、DNS、Secret manager 和网络策略部署；不能原样复制本地 disabled TLS、文件路径或单实例 Redis。
- **来源：** 项目事实；Redis 官方 [ACL](https://redis.io/docs/latest/operate/oss_and_stack/management/security/acl/)用于最小权限边界。

## 18. `maxmemory 48mb`、`allkeys-lru`、tmpfs 和关闭持久化分别意味着什么？

- **直接回答：** development Redis dataset 的 `maxmemory` 是 48 MiB，超出后按 `allkeys-lru` 淘汰近似最久未使用的 key；所有值都可由 MySQL 重建，所以淘汰等价于 miss。`/data` 使用 64 MiB tmpfs，同时 `save ""`、`appendonly no`，重启不恢复缓存。这些设置把 Redis 保持为易失加速层。
- **追问：** 48 MiB 是否就是 Redis 容器内存硬上限？
  - **追问回答：** 不是。Redis 官方说明 `maxmemory` 主要约束参与 eviction 的数据，进程、allocator、连接和某些 buffer 仍会占内存；tmpfs 上限也不是整个进程 RSS limit。acceptance 只验证配置边界，没有执行填满 48 MiB 的 eviction 容量测试。
- **项目证据：** [Redis 运行配置](../../../deploy/docker/redis-entrypoint.sh)、[Compose tmpfs](../../../deploy/compose/compose.yaml)、[smoke/acceptance 配置断言](../../../scripts/compose-smoke.sh)。
- **选型边界：** 生产需要基于 value 分布、连接数、hit/miss、`evicted_keys`、碎片和进程内存设置 dataset 与容器预算；若 Redis 混合持久数据，应重新选择实例隔离和 policy，不能照搬 `allkeys-lru`。
- **来源：** 项目事实；Redis 官方 [key eviction](https://redis.io/docs/latest/develop/reference/eviction/)解释 `maxmemory`、`allkeys-lru` 和未计入 eviction 比较的内存。

## 19. 缓存观测为什么强调低基数和日志限频？

- **直接回答：** adapter 只发有限 outcome：`hit`、`miss`、`read_error`、`corrupt`、`delete_error`、`fill_leader`、`fill_joined`、`source_error`、`write_ok`、`write_error`，附有 duration，不携带 StrategyID、key、payload、地址、username、Secret 或随机材料。常规事件为 debug；read/delete/write error 按类别每 10 秒最多输出一条 warning，并带累计 suppressed 数，避免 Redis 长故障每请求刷屏。
- **追问：** Redis-down M1 只有一条 `read_error` 日志，是否说明只失败了一次？
  - **追问回答：** 不能。日志被限频，一条 warning 不等于一次底层失败。M1 还结合 500 个请求、500 个 fill leader、1000 次 MySQL execute 和 500 次 source load判断降级路径；观测源要交叉验证，不能把被采样日志当精确计数器。
- **项目证据：** [observer](../../../cmd/growth-api/cache_observer.go)、[低基数/限频测试](../../../cmd/growth-api/cache_observer_test.go)、[M1 计数实现](../../../scripts/compose-lottery-api-acceptance.sh)。
- **选型边界：** 当前是结构化日志 observer，不是完整 metrics/dashboard/alert。生产命中率、eviction、pool、source amplification 和 latency histogram 需要后续可观测体系；StrategyID 只能在受控 trace 中按数据分级关联，不能做无界 label。
- **来源：** 项目事实；Redis 官方 [eviction observability](https://redis.io/docs/latest/develop/reference/eviction/)列出 hit/miss、expired 与 evicted 等运行信号。

## 20. 三组 M1 数据具体是什么，能证明什么？

- **直接回答：** 2026-08-30 在 Mac Docker Desktop（4 CPU、约 1.92 GiB）上，对同一 loopback Nginx → Go ephemeral endpoint、同一 immutable fixture，以 50 RPS、10 秒、16 workers、3 秒 timeout 单次运行。三组都 scheduled/completed/success=`500/500/500`，errors/unexpected-status/dropped=`0/0/0`：

  | 场景 | actual RPS | p50 / p95 / p99 / max（ms） | 独立 source/cache 证据 |
  | --- | ---: | --- | --- |
  | warm cache | `50.08618245771574` | `1.730167 / 4.129458 / 5.202 / 7.387375` | MySQL execute `0`；cache hit `500` |
  | cache disabled / direct MySQL | `50.076986275497326` | `2.390709 / 5.382042 / 9.747167 / 25.003417` | MySQL execute `1000`；source load `500`；cache event `0` |
  | Redis down | `50.05050617599433` | `2.602833 / 5.777708 / 8.222959 / 10.596541` | MySQL execute `1000`；source load `500`；fill leader `500`；joined `0`；限频 read-error log `1` |

- **追问：** 能否写“系统达到 500 RPS，缓存把 P99 优化了固定百分比”？
  - **追问回答：** 不能。workload 是 50 RPS、10 秒、单实例、单 key、单机、单次；没有多轮方差、长稳、资源曲线、生产网络、真实 key/Award 分布或正式 Draw。warm P99 在这一轮更低只能作为开发观察，不能外推固定收益、生产容量或 SLO；原 M1 候选目标 500 RPS/10 分钟/P99≤200ms 仍不能据此标为达到。
- **项目证据：** [acceptance `run_m1_load` 与三组计数](../../../scripts/compose-lottery-api-acceptance.sh)、[NFR 候选目标](../../product/non-functional-requirements-v1.md)、[ADR 性能边界](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)。
- **选型边界：** 下一步性能结论至少需要固定资源、重复轮次、置信区间或分布、热/冷 key 混合、不同并发/速率、CPU/内存/连接池和多副本 source amplification；故障基线也要单独预算。
- **来源：** 第 24 节已执行验收记录与父任务提供的原始 M1 输出；本题不使用面经或厂商示例数字替代项目证据。

## 21. 面试官让你现场画故障矩阵，应该怎样回答？

- **直接回答：** 先按“缓存状态、MySQL 状态、调用结果、readiness、证据级别”分开，不能只说统一降级：

  | 缓存状态 | MySQL | 读取/selection 结果 | `/ready` | 当前证据 |
  | --- | --- | --- | --- | --- |
  | valid hit | up | 返回领域恢复后的 Strategy，跳过 source | 200 | unit + Compose warm path |
  | Redis down/error | up | 短预算后回源；恢复后可 refill | 200 | Compose 已执行 |
  | valid warm hit | down | 该 key 可继续 selection | 503 | Compose 已执行 |
  | miss/cold key | down | 现有技术失败，不伪造结果 | 503 | Compose 已执行 |
  | poison | up | 记 corrupt、best-effort DEL、回源并修复 | 200 | unit + Compose 已执行 |
  | poison | down | 不服务 poison，返回 source 技术失败 | 503 | Reader/ADR 契约；未单列 Compose 场景 |
  | SET 失败 | up | 当前 MySQL 成功仍返回，后续可能再 miss | 200 | unit 已执行 |
  | DEL 失败 + poison | up | DEL error 不阻止回源/回填 | 200 | unit 已执行 |
  | caller cancel/deadline | 任意 | caller context 胜出，不包装成功 | 取决于请求层映射 | unit 已执行 |

- **追问：** 为什么矩阵要写证据级别？
  - **追问回答：** 设计约束、unit test 和真实 Compose fault injection 回答不同问题。把“由代码推导”写成“已停容器验证”会虚构证据；反过来，一次容器成功也不能证明所有时序和多副本故障。
- **项目证据：** [Reader unit tests](../../../internal/lottery/adapter/strategycache/reader_test.go)、[Compose failure injection](../../../scripts/compose-lottery-api-acceptance.sh)、[ADR 风险矩阵](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)。
- **选型边界：** 当前未验证网络分区、Redis OOM eviction 压力、Sentinel/Cluster failover、多 API 副本、MySQL 慢查询叠加 Redis outage 或持续故障下的 shedding；不能称为高可用认证。
- **来源：** 项目事实。

## 22. 为什么现在不叠加进程内 L1、本地 LRU 或 Redis client-side caching？

- **直接回答：** 本地缓存可省一次 Redis RTT，但每个实例会独立预热、复制内存并引入第三份状态；失效、容量、发布清空和多实例一致性都会更复杂。Redis client-side caching 还依赖 server tracking 和 invalidation 消息，本节 ACL、生命周期和观测都没有授权/实现这些能力。当前目标是先验证共享 Redis 投影协议和故障边界，没有 profile 证明 Redis RTT 已成瓶颈。
- **追问：** 什么时候 L1 + Redis L2 值得做？
  - **追问回答：** 当真实 profile 显示 Redis RTT/网络占显著比例、热点集合有界、实例内存可预算，并且业务版本或失效协议足以处理多层陈旧时，再比较 bounded LRU、server-assisted tracking 或短 L1 TTL；必须测实例数放大和失效丢失。
- **项目证据：** [ADR-0020 方案二与重评触发器](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)、[当前只有 Redis decorator 的组合](../../../cmd/growth-api/database.go)。
- **选型边界：** 单实例极低延迟内存应用可能直接用 L1；跨实例共享和集中治理更重要时 Redis 更合适。不能把“多级缓存”当默认成熟度指标。
- **来源：** Redis 官方 [client-side caching](https://redis.io/docs/latest/develop/clients/client-side-caching/)说明 tracking/invalidation 模型；题型来自牛客用户发布的[番茄小说后端一面](https://www.nowcoder.com/discuss/893833080824164352)个人自述，真实性未独立核验。

## 23. 为什么不缓存用户资格、授权、库存、not-found 或一次选择结果？

- **直接回答：** 本节只缓存完整 Strategy + Awards 配置投影。User/Membership/Risk/Participation 资格、Authorization policy decision、quota/frequency、库存与预占、随机 ticket、selected Award、ephemeral response、正式 Draw/Result、错误和 not-found 都有不同的事实所有者、时效、隐私、并发和恢复语义。把它们塞进同一 key/TTL 会让陈旧配置优化越界成业务正确性与安全决定。
- **追问：** HTTP 仍返回 `Cache-Control: no-store`，和服务端 Redis cache 是否冲突？
  - **追问回答：** 不冲突。HTTP header 禁止客户端/中间层保存一次 ephemeral response；Redis 保存的是服务端内部、可重建的 Strategy 输入。两者对象、信任边界和生命周期不同，前端也不能通过 `X-Cache` 等 header 依赖命中状态。
- **项目证据：** [ADR 缓存对象白名单](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)、[projection DTO](../../../internal/lottery/adapter/strategycache/codec.go)、[HTTP no-store acceptance](../../../scripts/compose-lottery-api-acceptance.sh)。
- **选型边界：** 未来某类资格读取若确实成为瓶颈，应为主体、撤销、freshness、PII、权限和失败关闭单独立 ADR；正式结果应进入权威持久化和幂等查询，不应复用 Strategy cache。
- **来源：** 项目事实；Redis 官方 cache-aside 说明能力，不替代对象级一致性和隐私评审。

## 24. 为什么 Docker build 同时设置 `go build -p=1`、`GOMAXPROCS=2` 和 `GOMEMLIMIT=768MiB`？

- **直接回答：** 三者控制不同层面。`go build -p=1` 限制 go command 同时运行的 build command/test binary 数；`GOMAXPROCS=2` 限制编译器 Go runtime 的可用并行度；`GOMEMLIMIT=768MiB` 给编译进程一个软内存目标。acceptance 还按 api → migrate → redis → web 顺序构建，让 API 先填充共享 build cache、migrate 复用，避免两个后端 target 同时复制编译峰值。这是约 1.92 GiB Docker Desktop 开发预算下的构建稳定性控制，不是线上 API runtime 参数。
- **追问：** `GOMEMLIMIT=768MiB` 是否保证编译绝不超过 768 MiB？Go 1.25 以后还需要显式 `GOMAXPROCS` 吗？
  - **追问回答：** Go 官方明确说 memory limit 是 soft limit，运行时可能为避免 GC thrashing 暂时超过，而且应给非 Go runtime 内存留 headroom。Go 1.25 起默认可以感知 Linux container CPU limit，但显式环境变量会覆盖默认；这里固定值是本次可复现 build policy，不能泛化为所有 CI 的最优值。
- **项目证据：** [backend Dockerfile](../../../deploy/docker/Dockerfile.backend)、[acceptance 顺序构建](../../../scripts/compose-lottery-api-acceptance.sh)。
- **选型边界：** 更大 CI runner 可基于冷/暖 build 峰值调整并行度；设置过低会延长构建或触发 GC thrashing。不得从“构建通过”推导生产容器的 CPU、内存或请求容量。
- **来源：** Go 官方 [`go build -p`](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies)、[`GOMEMLIMIT`](https://go.dev/doc/gc-guide#Memory_limit)与[容器感知 `GOMAXPROCS`](https://go.dev/blog/container-aware-gomaxprocs)文档；项目事实。

## 不能夸大的结论

可以准确说：

- 已实现 Lottery-owned Redis cache-aside Strategy 读取投影，MySQL 仍是唯一权威事实源；
- 已实现严格 v1 codec、完整正 `uint64`、2 MiB/1000 Award 上限、TTL jitter、poison 修复和进程内同 key 回源合并；
- 已验证 Redis 运行故障回源、MySQL down 的 warm/cold 差异、最小 ACL、internal network、Secret ownership 与三组单机 M1；
- 三组 M1 各 500 个请求均完成且成功，并有 MySQL execute/cache outcome 交叉证据。

不能说：

- 已实现强一致缓存、read-after-write、Strategy 更新精准失效或历史版本回放；
- singleflight 是分布式锁，或多实例缓存击穿已经解决；
- 已实现负缓存、Bloom Filter、L1、本地 LRU、client-side tracking、breaker、限流或自动 shedding；
- Redis ACL 等于产品 RBAC，internal network 等于生产零信任网络；
- warm hit 证明正式 Draw 可用、幂等、库存安全或权益已发放；
- 50 RPS/10 秒的 Mac 单次结果达成 500 RPS/10 分钟候选目标、生产 P99、容量或可用性 SLO。

## 复习清单

- [ ] 能画出 `GETRANGE → decode → hit` 与 `miss/error/poison → flight → MySQL → SET` 两条路径；
- [ ] 能解释 cache miss、Strategy not-found、Redis error 和 MySQL error 为什么不能混淆；
- [ ] 能写出 key schema，并说明 environment、`v1` 和 canonical `uint64` 各自职责；
- [ ] 能说明 2 MiB inclusive `GETRANGE` 为什么会读取最多上限加一 byte；
- [ ] 能区分穿透、击穿、雪崩，以及本节已缓解与未解决部分；
- [ ] 能解释 singleflight 的进程范围、每 caller cancellation、fill timeout 和 lifecycle；
- [ ] 能比较 singleflight 与分布式锁的 owner、lease、fencing 和故障模型；
- [ ] 能复述 TTL `[90%, 100%]`、最大 5 分钟和“只承诺有界陈旧”；
- [ ] 能写出 Redis ACL exact prefix/commands，并解释 `resetchannels`；
- [ ] 能说明 Redis down/MySQL up、warm hit/MySQL down、cold miss/MySQL down 三种 readiness 结果；
- [ ] 能逐项复述三组 M1 的请求数、延迟和 source/cache 证据，同时主动声明单机单次非 SLO；
- [ ] 能解释 `-p=1`、`GOMAXPROCS=2`、`GOMEMLIMIT=768MiB` 分别控制什么，以及 soft limit 边界。
