# 第 24 节 API 记录：Strategy 读取增加 Redis，公开 HTTP 契约零变化

- **章节：** [第 24 节：第一次 Redis 缓存](../../course/part-03/lesson-24-redis-strategy-cache.md)
- **架构决策：** [ADR-0020](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)
- **日期：** 2026-08-30
- **状态：** 内部 Strategy 读取路径变化；公开 route/DTO/header/status/error code 零变化
- **QA：** [第 24 节 QA](../../qa/lessons/lesson-24.md)
- **运维：** [Redis Strategy Cache Runbook](../../runbooks/redis-strategy-cache.md)

## 1. 本节 API 结论

第 24 节没有新增、删除或重新解释任何公开 HTTP route、请求 DTO、响应 DTO、header、status 或 error code。

唯一运行时变化发生在 application port 背后：

```text
第 23 节：HTTP -> EphemeralSelectionService -> MySQL StrategyRepository
第 24 节：HTTP -> EphemeralSelectionService -> cache-aside StrategyReader -> MySQL StrategyRepository
```

Redis 是可绕过的读取加速器，不是 HTTP 资源、业务事实源、授权系统或 error contract。因此下列表述全部错误：

- “新增了 Strategy cache API”；
- “响应会返回 cache hit/miss”；
- “Redis down 会产生新的 5xx code”；
- “Redis hit 表示不再需要 MySQL readiness”；
- “缓存命中后 ephemeral selection 可以安全重试”；
- “缓存层会把 MySQL 错误降级成 `no_reward`”。

## 2. HTTP surface 保持不变

| Method | Path | 当前语义 | 第 24 节变化 |
| --- | --- | --- | --- |
| `GET` | `/health` | 进程 liveness | 无 |
| `GET` | `/ready` | 当前 API 实例的 MySQL readiness | 无；Redis 不加入检查 |
| `POST` | `/api/v1/lottery/strategies/:strategy_id/ephemeral-selections` | development/test 专用、无持久结果的一次临时加权选择 | 仅内部 Strategy reader 可选使用 Redis |

本节没有新增：

```text
GET    /api/v1/lottery/strategies/:id/cache
DELETE /api/v1/lottery/strategies/:id/cache
POST   /api/v1/lottery/cache/flush
GET    /internal/cache/metrics
POST   /api/v1/lottery/draws
```

缓存的检查、删除和故障恢复是受控运维动作，不公开成浏览器 API。

## 3. ephemeral selection 请求零变化

合法请求仍为：

```http
POST /api/v1/lottery/strategies/21003/ephemeral-selections HTTP/1.1
Accept: application/json
X-GrowthOS-Demo-Mode: ephemeral-selection
```

约束仍然是：

- `strategy_id` 必须是 `1..18446744073709551615` 的 canonical decimal string；
- 请求没有 body、query、fragment 或 trailer；
- 不接受 `Idempotency-Key`；
- `X-GrowthOS-Demo-Mode` 必须恰好出现一次且值精确；
- route 仍只允许 development/test 显式启用；
- 一次调用仍执行一次新的随机选择；
- 客户端仍不得透明重试不确定结果。

第 24 节没有请求 DTO，也没有新增 Redis 相关 header。客户端不能提交：

```text
cache_key
cache_bypass
cache_ttl
cache_version
force_refresh
user_id
eligibility
permission
```

客户端给出的 `strategy_id` 只用于业务路由和 Repository 查找；服务端按照受控 environment 与固定规则构建 Redis key，绝不接受完整 key 或 namespace。

## 4. 成功响应零变化

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

仍只允许：

```text
award.outcome = reward | no_reward
```

响应不会新增：

```text
cache_hit
cache_age
cache_ttl
cache_key
projection_schema
source
strategy_version
```

Strategy 来自 Redis hit 或 MySQL source load，不改变 Award 的领域含义。`reward` 仍只是临时候选，不表示库存预占、持久中奖或权益发放；`no_reward` 仍是合法选择结果，不是 cache miss、Redis error 或 MySQL error 的兜底值。

## 5. response header 零变化

成功和错误响应继续遵循既有边界：

| Header | 语义 | 第 24 节变化 |
| --- | --- | --- |
| `Content-Type: application/json` | JSON wire contract | 无 |
| `Cache-Control: no-store` | 浏览器/中间代理不得缓存临时选择或错误 | 无 |
| `X-Request-ID` | 有界请求关联 | 无 |
| `Allow: POST` | 对错误 method 的提示 | 无 |

不存在 `X-Cache`、`Age`、`X-Redis-*` 或 `Server-Timing` cache detail。原因是：

- cache key 与拓扑属于内部边界；
- 暴露 hit/miss 会让客户端错误依赖实现细节；
- `Cache-Control: no-store` 约束的是 HTTP response，不能因为服务端内部缓存 Strategy 就移除；
- Redis observation 进入受控日志，不进入用户响应。

## 6. status 与 error code 零变化

公开错误 envelope 保持：

```json
{
  "error": {
    "code": "lottery_selection_unavailable",
    "message": "lottery selection is temporarily unavailable",
    "request_id": "req-..."
  }
}
```

本节沿用的主要契约如下：

| Status | Code | 触发条件 | Redis 是否改变它 |
| ---: | --- | --- | --- |
| `200` | 无 error | 合法 ephemeral selection，包括 `no_reward` | 否 |
| `400` | `invalid_strategy_id` | ID 非 canonical/越界/为零 | 否；在 cache 前拒绝 |
| `400` | `request_body_not_allowed` | body/chunked/trailer framing | 否 |
| `400` | `idempotency_not_supported` | 携带 `Idempotency-Key` | 否 |
| `400` | `demo_mode_required` | demo header 缺失、重复或错误 | 否 |
| `400` | `query_parameters_not_allowed` | 存在 query | 否 |
| `404` | `lottery_strategy_not_found` | MySQL 权威源确认 Strategy 不存在 | 否；not-found 不做负缓存 |
| `404` | `route_not_found` | 路径不存在或多余 `/` | 否 |
| `405` | `method_not_allowed` | 对 route 使用非 POST | 否 |
| `413` | `request_too_large` | 网关 body 上限拒绝 | 否；请求未到 cache |
| `421` | 既有网关错误 | 任意未授权 Host | 否 |
| `500` | `internal_error` | Repository 非重试失败、存储聚合非法或其他内部不变量失败 | 否；cache 不吞错 |
| `503` | `lottery_selection_unavailable` | deadline/cancel、MySQL retryable failure 或随机源失败 | 否；Redis error 本身先回源 |

没有新增：

```text
redis_unavailable
cache_timeout
cache_corrupt
cache_write_failed
cache_miss
```

这些是内部 observation，而不是用户可操作的业务错误。

## 7. Redis fail-open 的精确定义

cache-aside Reader 只对 Redis 失败 fail-open：

| 内部状态 | 下一步 | 最终 HTTP 由什么决定 |
| --- | --- | --- |
| Redis miss | 回源 MySQL，成功后 best-effort 回填 | 原有 source/selector 语义 |
| Redis read timeout/error | 记录 `read_error`，回源 MySQL | 原有 source/selector 语义 |
| Redis 值损坏/超大/ID 不匹配 | best-effort 精确删除，回源并回填 | 原有 source/selector 语义 |
| Redis SET/DEL 失败 | 当前成功 source Strategy 仍可继续 | 原有 selector 语义 |
| caller canceled | 立即保留 caller cancellation | 原有 503 mapping |
| MySQL not found | 原样返回 not found | `404 lottery_strategy_not_found` |
| MySQL retryable error | 原样返回 retryable error | `503 lottery_selection_unavailable` |
| MySQL permanent/invalid data error | 原样返回现有错误 | `500 internal_error` |

所以：

```text
Redis failure + MySQL success  -> 可以成功
Redis failure + MySQL failure  -> 仍然失败
```

“fail-open”不是 catch-all success，也不允许把任何异常映射为 `no_reward`。

## 8. Redis/MySQL 故障对 route 的影响

| Redis | MySQL | key 状态 | selection route | `/health` | `/ready` |
| --- | --- | --- | --- | --- | --- |
| up | up | warm valid | `200`，从投影读取后独立选择 | `200` | `200` |
| down | up | 任意 | 回源成功则 `200` | `200` | `200` |
| up | down | warm valid | 该 Strategy 可 `200`；不代表事实源健康 | `200` | `503` |
| up | down | cold/miss/corrupt | 使用既有 MySQL error mapping，通常 `503` | `200` | `503` |
| down | down | 任意 | 无可用读取路径，使用既有 MySQL error mapping | `200` | `503` |
| disabled | up | 不适用 | 直接 MySQL，成功则 `200` | `200` | `200` |
| disabled | down | 不适用 | 使用既有 MySQL error mapping | `200` | `503` |

warm cache 在 MySQL down 时仍能服务一次临时选择，是可用性效果，不是把 Redis 提升为事实源。`/ready` 继续为 503，明确告诉流量管理和操作者该实例失去权威依赖。

## 9. readiness 为什么只看 MySQL

`/ready` 的当前问题是：

> 这个实例是否能完整执行需要权威 Strategy 事实的 Lottery 读取？

Redis 不满足事实源条件，也不是所有 Strategy 都必然已 warm。因此：

- Redis down 不应让一个仍可回源 MySQL 的实例摘流；
- warm hit 不应让 MySQL down 的实例伪装成完整 ready；
- Redis `PING` 不进入进程启动或 readiness；
- Redis 连接错误通过受限 observation 诊断；
- production HA/readiness 策略若改变，必须另立决策。

## 10. 缓存不改变选择次数与幂等边界

singleflight 只合并：

```text
StrategyReader.FindByID -> MySQL source load
```

它不合并：

```text
EphemeralSelectionService.Select -> WeightedSelector.Select
```

因此 20 个并发请求即使共享一次 Strategy 回源，也会执行 20 次随机选择并得到 20 个独立 ephemeral result。结果仍未持久化，route 仍拒绝 `Idempotency-Key`，调用方仍不能自动重试。

## 11. cache wire format 不是公开 DTO

Redis projection v1 与 HTTP response 恰好都包含 Strategy/Award 信息，但它们不是同一个 schema：

- cache projection 包含 weight，HTTP response 不返回 weight；
- cache projection 包含所有 Awards，HTTP 只返回选中的 Award；
- cache projection 有独立 schema/version，HTTP 使用 URL API version；
- cache ID/weight 用 decimal string 保真，decode 后重新经过 domain restore；
- cache format 可以通过 key/schema version 演进，不要求浏览器同步 decoder；
- HTTP response 变化仍需独立 API 记录，不能由 Redis format 倒推。

## 12. 安全与信息披露

HTTP caller 看不到也不能控制：

- Redis address、username、password 或 TLS CA；
- cache namespace、完整 key 或 TTL；
- raw Redis/go-redis error；
- poison payload 或被删除值；
- pool size、timeout、eviction policy；
- hit/miss/source-load debug 数据。

Redis adapter error 对外只渲染稳定阶段；cache observer 不携带 key、payload、StrategyID、Award name 或 credential。既有 HTTP error mapping 继续只暴露稳定 code/message/request ID。

## 13. 可执行 API 核查

先运行现有 HTTP adapter 回归：

```bash
go test ./internal/lottery/adapter/httpapi
go test -race ./internal/lottery/adapter/httpapi
```

验证 composition 与缓存路径：

```bash
go test ./cmd/growth-api ./internal/lottery/adapter/strategycache
go test -race ./cmd/growth-api ./internal/lottery/adapter/strategycache
```

运行真实 MySQL/Redis/网关故障矩阵：

```bash
make compose-lottery-api-acceptance
```

该 acceptance 已覆盖：

- MaxUint64 cache 与 HTTP decimal string 保真；
- poison projection 自动删除、回源和修复；
- `reward` / `no_reward` 成功响应；
- route、method、query、header、body、idempotency 负向契约；
- Redis down 时 selection 成功且 `/ready` 仍 200；
- MySQL down 时 warm hit 成功、`/ready` 503、cold read 503；
- 依赖恢复后重新回填；
- M1 warm/direct/Redis-down 三条真实路径。

## 14. 正反例

### 正例：Redis down，MySQL 正常

```text
GETRANGE error
  -> 记录限频 read_error
  -> MySQL FindByID 成功
  -> SET 失败但忽略缓存写错误
  -> selector 正常执行
  -> 原有 200 response
```

### 反例：Redis down 直接返回 503

Redis 只是可选 accelerator。如果 MySQL 仍可服务，直接返回 503 会把可降级依赖错误升级成用户故障。

### 反例：MySQL down 返回 `no_reward`

无法读取权威 Strategy 不等于合法选择了 `no_reward` Award。这样做会混淆系统失败与业务结果，并可能损害用户权益。

### 反例：公开 `X-Cache-Key`

这会泄漏内部 namespace，并诱导调用方依赖不可兼容的实现细节。运维定位通过受控 request ID、outcome 和精确 StrategyID 完成。

## 15. 能准确表述与不能表述

可以表述：

> 第 24 节在既有 `StrategyReader` 内部增加可选 Redis cache-aside decorator；公开 route、无 body 请求、成功 DTO、`Cache-Control: no-store`、status/error code、随机选择次数和 MySQL readiness 全部保持不变。Redis 错误 fail-open 到 MySQL，MySQL 错误继续使用原有 HTTP mapping。

不能表述：

- 新增了 cache 管理 API 或 debug header；
- Redis down 必然返回成功；
- warm hit 使 MySQL 不再是 readiness authority；
- cache hit 复用了或持久化了随机结果；
- API 已支持幂等重试、正式 Draw、资格、库存或权限；
- projection schema version 就是公开 API version 或 Strategy aggregate version。

## 16. 关联资料

- [第 21 节 API：development/test 临时 Lottery Selection](lesson-21.md)
- [第 22 节 API：React 消费临时 Lottery Selection](lesson-22.md)
- [第 23 节 API：规则需求与运行时零变化](lesson-23.md)
- [第 24 节课程正文](../../course/part-03/lesson-24-redis-strategy-cache.md)
- [ADR-0020](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)
- [第 24 节 QA](../../qa/lessons/lesson-24.md)
- [Redis Strategy Cache Runbook](../../runbooks/redis-strategy-cache.md)
