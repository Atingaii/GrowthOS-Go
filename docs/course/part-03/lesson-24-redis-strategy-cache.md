# 第 24 节：第一次 Redis 缓存——Strategy 可重建读取投影

> 第 23 节已经把边界说清楚：规则、资格、权限、库存和一次随机结果都不是可以随手塞进缓存的对象。本节只选择一个事实边界稳定、能够从 MySQL 完整重建、失败后可以直接绕过的对象——完整 `Strategy + Awards` 读取投影。实现采用 `StrategyReader` decorator、严格 v1 codec、TTL 有界陈旧、进程内同 key 回源合并和 Redis fail-open；公开 HTTP 契约、Lottery 选择语义和 MySQL readiness 均不改变。

- **起点：** `27a552b`（第 23 节已验收 tip）
- **长期决策：** [ADR-0020：Lottery Strategy Redis Cache-Aside 读取投影](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)
- **API 记录：** [第 24 节 API](../../api/lessons/lesson-24.md)
- **验收证据：** [第 24 节 QA](../../qa/lessons/lesson-24.md)
- **运维手册：** [Redis Strategy Cache Runbook](../../runbooks/redis-strategy-cache.md)
- **面试问答：** [第 24 节面试问答](../../interview/lessons/lesson-24.md)

## 1. 为什么第一次缓存先选 Strategy

缓存不是“给数据库加速”的通用开关。先问三个第一性问题：

1. 哪个系统拥有事实；
2. 缓存值能否从事实源完整、确定地重建；
3. 缓存不可用、过期或损坏时，业务语义能否保持不变。

当前 Strategy 满足这些条件：

- `lottery_strategy` 与 `lottery_strategy_award` 仍由 MySQL 持久化；
- Repository 已能在一致快照中恢复一个通过领域校验的完整聚合；
- Lottery application 已依赖窄接口 `StrategyReader`，适合增加 decorator；
- 读取失败已有明确 Repository 错误语义；
- Strategy 当前没有更新、发布或删除写路径，因此可以先承诺 TTL 有界陈旧，而不是虚构精准失效。

相比之下，下列对象不满足条件：

- 用户资格和风控结论依赖主体、时间与外部事实新鲜度；
- 权限结论依赖会话、资源、动作与数据范围；
- 库存和可分配性会在并发操作中变化；
- 随机数和 ephemeral selection 每次调用都应重新执行；
- 正式 Draw/Result 尚未形成持久身份、幂等和恢复协议。

因此本节不是“把 Redis 用起来”，而是建立第一个有明确真相源、失效边界、失败语义和撤销开关的缓存切片。

## 2. 本节目标

完成本节后，仓库应能准确回答：

1. 为什么 MySQL 仍是唯一事实源，Redis 只能保存可丢弃投影；
2. 为什么缓存完整 Strategy 聚合，而不是半个 Strategy、单个 Award 或最终选择结果；
3. key 为什么包含 namespace、environment、对象语义与 schema version；
4. 为什么 codec 独立于 HTTP DTO，并严格拒绝未知字段、重复字段、非 canonical uint64、超大 payload 与非法领域值；
5. cache hit、miss、read error、corrupt、source error、write error 分别怎样处理；
6. 为什么 Redis 失败时回源 MySQL，但 MySQL 失败不能被缓存层吞掉；
7. 为什么同 key singleflight 只合并回源，不能合并 `WeightedSelector` 调用；
8. TTL、lookup、write、fill 与整个 selection deadline 怎样形成预算关系；
9. Redis 为什么不进入 startup probe 或 `/ready`；
10. 命名 ACL、Secret、internal network 和精确 key 删除怎样限制故障半径；
11. 如何用单元、race、Compose 故障注入和 MySQL statement counter 证明边界；
12. 哪些结论只是本机单次 M1 证据，不能升级为生产 SLO。

## 3. 开始前的真实路径

第 21～23 节的读取链路是：

```text
bodyless ephemeral POST
  -> EphemeralSelectionService
  -> MySQL StrategyRepository.FindByID
  -> RestoreStrategy(Strategy + Awards)
  -> WeightedSelector.Select
  -> ephemeral reward | no_reward response
```

这一条路径已经具备：

- canonical uint64 StrategyID；
- 一致快照恢复；
- 领域不变量校验；
- 无偏固定权重选择；
- development/test feature gate；
- 无 body、无透明重试、无持久结果；
- MySQL readiness 与 HTTP liveness 分离。

它的性能问题也很具体：每次 Strategy source load 都执行两条 prepared `SELECT`，一条读取 Strategy，一条读取 Awards。重复读取同一个不常变化的 Strategy 时，这部分工作可以由可重建投影吸收，但 selector 仍必须逐请求执行。

## 4. 先画出不允许颠倒的事实流

```text
HTTP StrategyID
  -> application StrategyReader
       -> Redis projection GETRANGE
            hit + strict decode + domain restore ------+
            miss/error/corrupt                         |
                 -> per-key in-process source flight   |
                      -> MySQL Repository --------------+
                      -> best-effort Redis SET with TTL
  -> WeightedSelector（每个请求独立执行）
  -> 原有 ephemeral HTTP response
```

这张图包含四条硬约束：

1. Redis 永远不能反向成为 MySQL 的写入来源；
2. 缓存命中仍要经过严格 decode 与领域恢复；
3. cache error 只能改变读取路径，不能发明新的业务结果；
4. singleflight 的结果是 Strategy，不是随机选择结果。

## 5. 本节明确非目标

本节不实现：

- Strategy 创建、修改、发布、删除和精准 cache invalidation；
- user、permission、RBAC、session、risk、eligibility 或 quota 缓存；
- inventory、Award 可分配性、Benefit、订单或积分缓存；
- random value、ephemeral selection、Draw 或 Result 缓存；
- not-found/空值负缓存、Bloom filter 或缓存预热平台；
- Redis 分布式锁、Lua、Redlock、跨实例 singleflight；
- Redis Sentinel、Cluster、复制、AOF/RDB 或生产 HA；
- 多级本地 LRU、Redis client-side caching 或 Pub/Sub 失效；
- 公开 `X-Cache`、`Age`、cache debug header 或新的 HTTP error code；
- 把 Redis 加入 `/ready`，或在 `Open` 时主动 `PING`；
- 用本机 M1 数字承诺生产容量、命中率、P99 或 SLO。

这些不是遗漏。它们分别需要真实写路径、生产拓扑、更新一致性、运维责任或业务身份后再单独决策。

## 6. 按真实提交逐步学习

本分支从第 23 节已验收 tip 线性演进。建议按下表顺序阅读、切换和验证，不要只看最终 diff。

| 顺序 | 提交 | 本步只解决什么 | 建议验证 |
| --- | --- | --- | --- |
| 1 | `272d028 feat(config): define optional strategy cache contract` | 建立默认关闭的缓存配置、Redis 连接配置、Secret/TLS/预算校验 | `go test ./internal/platform/appconfig` |
| 2 | `44e7ed1 feat(redis): add bounded lazy client` | 引入 go-redis，封装只含 `GETRANGE/SET/DEL/Close` 的有界资源 | `go test ./internal/infrastructure/redisstore` |
| 3 | `3475fa2 fix(config): cap strategy cache stale window` | 发现“默认 5m 但允许 24h”违背决策，将配置最大 TTL 收紧为 5m | `go test ./internal/platform/appconfig` |
| 4 | `764f74a docs(cache): define strategy projection boundary` | 用 ADR 固化事实源、投影、TTL、singleflight、ACL 与非目标 | `git diff --check 3475fa2..764f74a` |
| 5 | `765bb00 feat(lottery): cache strategy projections safely` | 实现严格 codec、cache-aside Reader、损坏修复、TTL jitter 与 per-key flight | `go test -race ./internal/lottery/adapter/strategycache` |
| 6 | `68fa59b feat(api): compose optional strategy cache` | 在 composition root 中按开关装饰 Repository，管理 Redis pool 生命周期与低基数日志 | `go test -race ./cmd/growth-api` |
| 7 | `17e7010 feat(compose): enforce strategy cache boundary` | 接入 Secret、internal cache network、ACL、Redis memory policy、smoke/acceptance | `docker compose -f deploy/compose/compose.yaml config --quiet` |
| 8 | `c4c622f feat(load): support bodyless post baselines` | 让 healthload 严格支持 GET/POST，为真实业务 route 提供无 body 负载 | `go test -race ./cmd/healthload` |
| 9 | `b7250f fix(redis): remove implicit channel access` | 审查发现 ACL 仍有隐式 Pub/Sub 权限，改成 `resetchannels` 并补负向验收 | `sh -n deploy/docker/redis-entrypoint.sh scripts/compose-smoke.sh scripts/compose-lottery-api-acceptance.sh` |
| 10 | `8804f87 fix(load): acknowledge ephemeral selections` | 负载器显式发送 demo acknowledgement，避免把 400 当业务负载 | `go test -race ./cmd/healthload` |
| 11 | `40f1acf fix(build): bound backend compiler resources` | Docker Desktop 2 GiB 环境中限制 Go 构建并行与软内存预算 | `docker build --file deploy/docker/Dockerfile.backend --target api .` |
| 12 | `d33723c test(cache): compare strategy source-load baselines` | 用同 route、同 fixture、同负载比较 warm/direct/Redis-down，并用 MySQL counter 证明回源次数 | `make compose-lottery-api-acceptance` |
| 13 | `0f70f51 docs(adr): register strategy cache decision` | 把 ADR-0020 登记到长期决策索引，避免规范性决定成为孤立文件 | `go run ./cmd/doccheck` |
| 14 | `9d44eb1 docs(runtime): explain Redis cache operations` | 同步配置、仓库地图与 Compose/Redis 运维边界 | `git diff --check 0f70f51..9d44eb1` |
| 15 | `5a0baeb docs(product): calibrate cache capability boundary` | 校准产品/NFR/限界上下文/前端事实，不把缓存写成事实源、SLO 或浏览器契约 | `git diff --check 9d44eb1..5a0baeb` |
| 16 | `d621aae docs(course): teach Redis strategy caching` | 交付课程、API、QA、设计手记、24 道面试问答与专用 Runbook | `go run ./cmd/doccheck` |

全局索引与最终门禁在后续检查点提交中收口；完整章节始终以 `origin/codex/lesson-24-redis-strategy-cache` 的冻结 tip 为准，不能把上表任一中间提交单独当成完整验收版本。

这段历史刻意保留了三次真实修正：TTL 上限收紧、ACL channel 权限收敛、压测请求补齐 ephemeral acknowledgement。它们展示的是审查与验收怎样让架构逐步变得可信，而不是假装第一版没有偏差。

逐提交学习命令：

```bash
git fetch origin
git log --reverse --oneline 27a552b..codex/lesson-24-redis-strategy-cache
git show --stat 272d028
git diff 272d028..44e7ed1 -- internal/infrastructure/redisstore go.mod go.sum
git diff 764f74a..765bb00 -- internal/lottery/adapter/strategycache
git diff 765bb00..68fa59b -- cmd/growth-api
git diff 68fa59b..d33723c -- deploy scripts cmd/healthload
```

## 7. 第一步：先定义配置，而不是先连接 Redis

缓存开关默认关闭：

```text
GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED=false
```

启用后才要求恰好一个 Redis password 来源。主要缓存预算为：

| 配置 | 默认值 | 当前边界 |
| --- | ---: | ---: |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_TTL` | `5m` | `1s..5m` |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_LOOKUP_TIMEOUT` | `75ms` | `1ms..1s` |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_WRITE_TIMEOUT` | `75ms` | `1ms..1s` |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_FILL_TIMEOUT` | `2s` | `1ms..30s` |

还必须满足：

```text
lookup timeout + fill timeout + write timeout <= selection timeout
```

这是最坏路径的保守预算约束，不代表每次请求一定串行消耗三段完整时间。staging/production 启用缓存时还必须使用 `verify_identity` Redis TLS；development Compose 才明确使用隔离网络内的明文连接。

## 8. 第二步：建立窄且懒连接的 Redis 资源

[redisstore](../../../internal/infrastructure/redisstore) 不暴露通用 go-redis client，只提供：

```go
GetRange(ctx, key, start, end)
Set(ctx, key, payload, ttl)
Del(ctx, key)
Close()
```

关键选择是：

- `Open` 只校验本地配置并创建 lazy client，不 `PING`；
- `MaxRetries=-1`，缓存适配器没有应用层重试；
- `GETRANGE 0..2MiB` 最多取回 2 MiB 加一字节，用于识别超大值；
- `SET` 必须同时带正 TTL，拒绝永久缓存项；
- `Del` 只接受一个精确 key，不提供 variadic 或 pattern 删除；
- error 对外只渲染稳定阶段，例如 `redis_getrange`，cause 只留给可信代码检查；
- 配置的 `String`、`GoString`、structured log 与 JSON 边界全部脱敏；
- pool 由 composition root 长期持有并在 shutdown 精确关闭。

## 9. 第三步：把缓存 wire format 当作不可信输入

Redis 中的值不是领域对象内存快照，而是独立版本化投影：

```json
{
  "schema": "growthos.lottery.strategy.projection",
  "schema_version": 1,
  "strategy": {
    "id": "21003",
    "name": "Multi award strategy",
    "awards": [
      {"id": "1", "name": "Reward", "weight": "1", "outcome": "reward"},
      {"id": "2", "name": "No reward", "weight": "3", "outcome": "no_reward"}
    ]
  }
}
```

严格 codec 保证：

- ID 与 weight 用 canonical decimal string，完整支持 `uint64`；
- schema/name 大小写精确；
- 每个字段恰好一次；
- 未知字段、重复 JSON name、trailing value 和过深结构被拒绝；
- payload 最大 2 MiB；
- Award 数为 `1..1000`；
- StrategyID、AwardID、weight 必须为正；
- outcome 与所有领域不变量重新经过 `RestoreAward/RestoreStrategy`；
- 缓存内 StrategyID 必须等于请求的 StrategyID。

缓存命中不等于可信。只有完成完整解码和领域恢复后，值才重新进入 application 边界。

## 10. 第四步：逐状态定义 cache-aside

| Redis 状态 | Reader 行为 | 对外语义 |
| --- | --- | --- |
| 合法 hit | 返回恢复后的 Strategy，不查 MySQL | HTTP 行为不变 |
| miss | 同 key 进入 source flight，查 MySQL，best-effort 回填 | 使用原 Repository 结果 |
| read timeout/error | 记录低基数 `read_error`，回源 | 不返回 Redis 错误 |
| 空值、超大值、坏 JSON、ID 不匹配 | 标记 `corrupt`，best-effort 精确 `DEL`，回源并回填 | 不把 poison 值交给 selector |
| MySQL not found | 返回原 `ErrStrategyNotFound` | 不做负缓存 |
| MySQL retryable/failure | 返回原 Repository error | 不改成 `no_reward` 或 cache miss |
| encode/SET error | 返回成功 source Strategy，记录 `write_error` | 当前请求成功；缓存仍可丢弃 |
| caller canceled | caller cancellation 优先 | 不用 fail-open 掩盖取消 |

“fail-open”只表示 Redis 错误允许绕过缓存，不表示所有依赖错误都变成成功。

## 11. 第五步：只合并同 key 回源

同一个进程中多个请求同时 miss 相同 key 时，只需要一个 MySQL source load。其他 caller 等待同一 `Strategy` 结果，但保留各自 context：

- 一个等待者取消，不会取消其他等待者；
- leader 的请求取消不会让共享 fill 永久失控；
- fill 使用独立 hard timeout；
- process lifecycle 取消会停止共享 fill；
- 不同 key 独立并发，不使用全局锁；
- source error 不进入负缓存；
- 每个 HTTP 请求拿到 Strategy 后仍独立调用 selector。

因此 singleflight 是“防止同进程热点 key 同时回源”的吞吐保护，不是分布式锁、幂等锁或抽奖结果去重。

## 12. 第六步：在 composition root 中可逆装配

开关关闭时：

```text
MySQL Repository -> EphemeralSelectionService
```

开关开启时：

```text
MySQL Repository -> strategycache.Reader -> EphemeralSelectionService
```

Repository、cache decorator、selector 与 HTTP adapter 都不读取环境变量。只有 `cmd/growth-api`：

- 读取已验证配置；
- 创建并拥有 MySQL 与 Redis pool；
- 根据开关选择是否装饰 `StrategyReader`；
- 把 environment 映射到缓存 namespace；
- 注入 process lifecycle 和 observer；
- 在正常退出与部分启动失败时关闭已取得资源。

Redis client 创建不访问网络，因此 Redis 停止不会阻止 API 启动。`/ready` 仍只检查 MySQL，因为 Redis 只是可绕过优化；把它加入 readiness 会把可降级故障升级成实例摘流，违背 fail-open 目标。

## 13. 第七步：把安全边界落到 Compose

development Compose 使用：

```text
key prefix:
growthos:development:lottery:strategy:projection:v1:*

allowed commands:
PING GETRANGE SET DEL

denied by default:
all other commands, all other keys, all Pub/Sub channels
```

同时满足：

- default Redis user 关闭；
- API 与 Redis 才能进入 `cache` internal network；
- 只有 API 与 Redis 挂载 `redis_password` Secret；
- web、mysql、migrate、mysql-grants 既不接入 cache 网络也不挂 Secret；
- Redis 无 host port；
- `/data` 为 64 MiB tmpfs，Redis 不持久化；
- `maxmemory 48mb` 与 `allkeys-lru` 限制可丢缓存数据；
- ACL 不开放 `GET`、`SCAN`、`KEYS`、`CONFIG`、`ACL`、`EVAL`、`SCRIPT`、`SUBSCRIBE` 或 `PUBLISH`。

第一次实现曾保留隐式 channel 权限；后续提交将其收紧为 `resetchannels`。这是威胁边界审查产生的真实修复。

## 14. 第八步：可观测，但不制造高基数

observer 使用固定 outcome：

```text
hit miss read_error corrupt delete_error
fill_leader fill_joined source_error write_ok write_error
```

它不携带 key、StrategyID、payload、Award name、Redis 地址、credential 或原始 driver error。普通结果进入 debug；`read_error`、`delete_error`、`write_error` 按 kind 独立限频，默认十秒窗口只记录一次并统计抑制数。

这些事件用于判断路径，不是业务结果：

- `hit` 不表示 reward；
- `miss` 不表示 Strategy 不存在；
- `source_error` 不表示 Redis 故障；
- `fill_joined` 不表示抽奖请求被合并成一个结果。

## 15. 跟学验证顺序

先验证最窄 package，再扩大到全仓和真实依赖：

```bash
go test ./internal/platform/appconfig
go test ./internal/infrastructure/redisstore
go test ./internal/lottery/adapter/strategycache
go test ./cmd/growth-api ./cmd/healthload

go test -race ./internal/infrastructure/redisstore
go test -race ./internal/lottery/adapter/strategycache
go test -race ./cmd/growth-api ./cmd/healthload

go vet ./...
make verify
```

检查 Compose 与 shell：

```bash
docker compose --project-name growthos \
  --file deploy/compose/compose.yaml config --quiet

sh -n \
  deploy/docker/redis-entrypoint.sh \
  scripts/compose-smoke.sh \
  scripts/compose-lottery-api-acceptance.sh
```

最后运行隔离 acceptance：

```bash
make compose-lottery-api-acceptance
```

该脚本为每次运行生成唯一 Compose project、Docker-assigned loopback port、临时 Secret、volume、image 和 buildx builder；清理前会核对 label/ID，并确认长期 `growthos` project 的资源身份没有变化。不要把脚本中的随机 project 名替换成长期项目名。

## 16. 本节真实 M1 快照

同一台开发机、同一 fixture、同一条 loopback Nginx → Go route，以 `50 req/s × 10s`、16 workers、bodyless POST 运行一次：

| 场景 | completed | actual RPS | P50 ms | P95 ms | P99 ms | max ms | 独立来源证据 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| warm cache | 500 | 50.0862 | 1.730167 | 4.129458 | 5.202 | 7.387375 | MySQL executes `0`；cache hits `500` |
| cache disabled / direct MySQL | 500 | 50.0770 | 2.390709 | 5.382042 | 9.747167 | 25.003417 | MySQL executes `1000`；source loads `500`；cache events `0` |
| Redis down / fail-open | 500 | 50.0505 | 2.602833 | 5.777708 | 8.222959 | 10.596541 | MySQL executes `1000`；source loads `500`；leaders `500`；joined `0`；read-error logs `1` |

这一组数据能证明本次运行中三条路径真实发生，并且 warm hit 没有访问 MySQL、cache disabled 没有偷偷使用 Redis、Redis down 能持续回源。它只是本机单次开发证据，不是生产 SLO、容量结论或普适性能排名。完整口径见 [第 24 节 QA](../../qa/lessons/lesson-24.md)。

## 17. 本节完成后的真实能力

可以准确表述：

> 为 Lottery 的完整 Strategy 聚合增加 Redis cache-aside 读取投影，以 MySQL 作为唯一事实源；实现严格版本化 codec、2 MiB/1000 Award 边界、5 分钟减 0～10% TTL 抖动、同 key 进程内回源合并、poison 精确修复、Redis fail-open、最小 ACL/Secret/internal network 和低基数观测。隔离 Compose 故障矩阵及三组同口径 M1 source-load 证据已实际通过；文档合并后的全仓 `make verify` 与 `go test -race ./...` 仍以 [QA](../../qa/lessons/lesson-24.md) 的最终复验记录为准。

不能表述：

- Redis 已成为 Lottery 数据库或 Strategy 写模型；
- 已有 Strategy 更新后的精准失效或强一致双写；
- singleflight 是分布式锁或跨实例防击穿；
- 缓存了用户资格、权限、库存、随机数或 Draw/Result；
- Redis 故障永远没有数据库放大风险；
- warm-cache 本机 P99 是生产 SLO；
- 已完成 Redis HA、生产 TLS 证书部署、容量规划或灾备；
- 已实现第 25 节用户资格规则或第 31～35 节公共权限系统。

## 18. 后续演进约束

第 25 节应基于第 23 节需求基线实现第一条最小真实资格判断，不能把 eligibility verdict 放入本节 Strategy cache。

未来第一次出现 Strategy Update/Publish/Delete 时，必须另立 ADR 选择版本化 key、事务后失效、outbox/CDC 或其他一致性方案，并补并发写、回填竞态和回滚验收。不能仅因为当前 TTL 能自动过期，就宣称未来写路径已经解决。

如果出现多实例同时回源导致数据库压力、真实 Redis 容量/命中率问题、生产 TLS/HA 或运维扫描需求，也应以新证据单独演进；不在本节提前开放分布式锁、通用 ACL 或 namespace-wide 删除。

## 19. 复盘

本节最重要的学习点不是 Redis 命令，而是缓存的从属关系。事实源先存在，投影才能被定义；失败语义先存在，fail-open 才不会吞掉业务错误；key、codec、TTL、并发和 ACL 都是同一条信任边界的不同表现。

真实项目演进还体现在修复历史中：配置范围过宽就收紧，ACL 留下隐式权限就撤销，压测工具缺少业务确认 header 就补契约，Docker Desktop 构建超出资源就限制编译并行。每一步都可以独立验证，也保留了为什么这样做的证据。
