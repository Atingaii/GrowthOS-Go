# Redis Strategy Cache 运维手册

**适用范围：** 第 24 节 Lottery Strategy 可重建读取投影；本地 Docker Compose 与采用同一契约的受控环境

**事实源：** MySQL `lottery_strategy` + `lottery_strategy_award`

**缓存对象：** 完整 `Strategy + Awards` projection v1

**默认本地 Compose project：** `growthos`

架构依据见 [ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)，公开契约见[第 24 节 API](../api/lessons/lesson-24.md)，实现与故障证据见[第 24 节 QA](../qa/lessons/lesson-24.md)，完整 Compose 生命周期见[本地 Compose 运维手册](local-compose.md)。

## 1. 目的与责任边界

本手册回答五件事：

1. 怎样确认当前实例是否启用 Strategy cache；
2. 怎样在不泄漏 Secret、不扩大 ACL、不扫描 keyspace 的前提下诊断已知 Strategy key；
3. Redis、MySQL、poison projection 分别故障时，route/readiness 应如何表现；
4. 怎样精确恢复或撤销缓存，不误删其他数据；
5. 怎样在隔离 project 中复验 ACL、故障矩阵和 M1。

它不是 Redis 通用管理手册，也不是生产 HA/备份/扩缩容方案。当前 Redis 内容可由 MySQL 重建、没有持久化，不是业务恢复点。

## 2. 绝对安全规则

1. **先确认 Compose project。** 本文底层命令都显式使用 `--project-name growthos --file deploy/compose/compose.yaml`；若使用其他 project，必须从本轮创建记录取得精确名称，不能按容器名猜。
2. **不打印 Secret。** 不执行 `cat deploy/compose/secrets/redis_password`，不把 `/run/secrets/redis_password`、`REDISCLI_AUTH`、ACL password、expanded environment 或连接 URL 贴入日志、截图和工单。
3. **禁止 `FLUSHALL` 和 `FLUSHDB`。** 一个 Strategy 的陈旧或 poison 只能精确 `DEL` 一个经过验证的 key；不得使用 pattern delete、Lua 批量删除、`KEYS` 或 `SCAN`。
4. **不临时放宽业务 ACL。** 不给 `growthos_api` 增加 `@all`、`GET`、`KEYS/SCAN`、`CONFIG`、`ACL`、Lua、Pub/Sub、通配 key 或 channel 权限来“方便排查”。
5. **不开放 Redis host port。** 本地 Redis 只在 Compose internal `cache` network 内供 API 使用；不要把 `6379` 发布到宿主机。
6. **不把 Redis 加入 readiness。** Redis down 且 MySQL up 时服务可以回源；把 Redis 加入 `/ready` 会把可降级故障升级成摘流。
7. **不操作用户已有 Redis。** 故障演练只对 acceptance 的随机隔离 project 执行，不停止 Docker Desktop 中其他 Redis/MySQL 容器。
8. **不靠删 volume 恢复。** Redis `/data` 是 tmpfs，本来就可丢；MySQL volume 是权威业务数据，绝不能为缓存问题删除。
9. **不把 warm hit 当 MySQL 健康。** MySQL down 时某个 warm key 可继续服务，但 `/ready` 必须 503；恢复判断以权威源和 readiness 为准。
10. **不把本机 M1 当 SLO。** 单次 50 RPS 结果只证明路径和本次开发环境行为。

## 3. 架构速查

```text
ephemeral selection request
  -> StrategyReader decorator
       -> Redis GETRANGE exact projection key
            valid hit -> strict decode + domain restore
            miss/error/corrupt
              -> per-process, per-key source flight
                   -> MySQL StrategyRepository (sole truth)
                   -> best-effort Redis SET with TTL
  -> WeightedSelector.Select（每个请求独立执行）
```

不变量：

- Redis 只能缩短 Strategy 读取路径，不能生成、修改或确认业务事实；
- cache error fail-open 到 MySQL，MySQL error 不 fail-open；
- cache hit 不跳过 codec/领域恢复；
- singleflight 只合并同进程同 key 的 source load，不合并随机选择；
- `no_reward` 是配置 Award 的合法结果，不是缓存/数据库错误兜底；
- `/health` 是进程 liveness，`/ready` 仍检查 MySQL。

## 4. 开关与配置

### 4.1 主要缓存开关

| 环境变量 | 默认值 | 允许范围/语义 |
| --- | ---: | --- |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED` | `false` | 只有 `true/false`；关闭时 composition 直接注入 MySQL Reader |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_TTL` | `5m` | `1s..5m`；实际 TTL 再减 0～10% jitter |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_LOOKUP_TIMEOUT` | `75ms` | `1ms..1s` |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_WRITE_TIMEOUT` | `75ms` | `1ms..1s` |
| `GROWTHOS_LOTTERY_STRATEGY_CACHE_FILL_TIMEOUT` | `2s` | `1ms..30s`；约束共享 fill |

还必须满足：

```text
lookup timeout + fill timeout + write timeout <= lottery selection timeout
```

Redis connection/pool 变量详见 [configuration](../configuration.md)。启用缓存时 password value 与 password file 必须恰好一个；staging/production 必须使用 `GROWTHOS_REDIS_TLS_MODE=verify_identity`。development Compose 才明确在 internal network 内使用 `disabled` TLS。

### 4.2 当前 Compose 快照

当前 `deploy/compose/compose.yaml` 为第 24 节教学快照，明确设置：

```text
GROWTHOS_ENVIRONMENT=development
GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED=true
GROWTHOS_LOTTERY_STRATEGY_CACHE_TTL=5m
GROWTHOS_REDIS_ADDRESS=redis:6379
GROWTHOS_REDIS_USERNAME=growthos_api
GROWTHOS_REDIS_PASSWORD_FILE=/run/secrets/redis_password
GROWTHOS_REDIS_TLS_MODE=disabled
```

只读取非 Secret 缓存开关：

```bash
docker compose \
  --project-name growthos \
  --file deploy/compose/compose.yaml \
  exec -T api sh -eu -c '
    env | sed -n "/^GROWTHOS_LOTTERY_STRATEGY_CACHE_/p"
  '
```

不要改成输出全部 environment；未来可能有直接 Secret value 来源。

### 4.3 开关不是热更新

修改部署 environment 后必须重新创建 API process。对当前 tracked Compose 快照，shell 前缀：

```bash
GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED=false make compose-up
```

**不会**覆盖 YAML 中硬编码的 `"true"`，因此不是有效回滚。正确做法是：

1. 通过受评审的 Compose/config 变更或明确的部署 overlay 把 API service 变量设为 `false`；
2. 运行 config validation，确认展开模型中 API 的值为 `false`，且没有打印 Secret；
3. 重新创建 API；
4. 用业务 route 与 MySQL counter 证明已直连；
5. 保留 Redis service/key 等待 TTL 自然过期，不做 flush。

独立 acceptance overlay 的 `GROWTHOS_LESSON24_ACCEPTANCE_CACHE_ENABLED` 只属于随机测试 project，不能当长期环境开关。

## 5. key、value 与 TTL 契约

### 5.1 key

```text
growthos:<environment>:lottery:strategy:projection:v1:<canonical-strategy-id>
```

允许 environment：

```text
development test staging production
```

示例：

```text
growthos:development:lottery:strategy:projection:v1:21003
```

不要从客户端接受完整 key，也不要跨 environment 复制。`v1` 同时存在于 key 和 payload schema；升级 schema 时必须新 namespace/迁移方案，不能原地让旧 decoder 猜格式。

### 5.2 value

projection 完整包含 Strategy 和所有 Awards，使用独立严格 JSON schema：

```text
schema = growthos.lottery.strategy.projection
schema_version = 1
max payload = 2 MiB
award count = 1..1000
ID/weight = canonical uint64 decimal string
```

Redis 中的值始终视为不可信。未知/重复字段、缺字段、大小写变化、trailing JSON、超大值、非法 ID/weight/outcome、领域恢复失败或请求 ID 不匹配都属于 `corrupt`。

### 5.3 TTL

默认 5 分钟时，写入 TTL 为：

```text
4m30s..5m
```

jitter 只减不加，且 TTL 与 value 在一条 `SET` 中原子写入。业务 ACL 故意没有 `TTL/PTTL` 命令；不要为了现场查询剩余时间而扩大权限。配置校验与单元测试证明应用传给 `SET` 的 expiration/jitter 边界；隔离 acceptance 的 ACL helper 明确执行并通过 `SET ... EX 30`，应用 refill 路径则证明带 expiration 的写入可读且可在 restart 后重建（go-redis 的具体 wire 参数可为 `EX` 或 `PX`）。本轮没有采样服务器实际剩余 TTL 窗口。

当前没有 Strategy 更新写路径，所以只承诺 TTL 有界陈旧。未来第一次出现 Update/Publish/Delete 时，精准失效、版本化 key、outbox/CDC 或写后失效必须另立 ADR。

## 6. Redis topology 与 ACL

development Compose 边界：

```text
network: cache (internal)
consumers: api, redis only
host port: none
secret consumers: api, redis only
/data: 64 MiB tmpfs
maxmemory: 48mb
maxmemory-policy: allkeys-lru
persistence: save ""; appendonly no
```

ACL 语义必须精确等价于：

```text
user default off
user growthos_api on >REDACTED \
  resetkeys ~growthos:development:lottery:strategy:projection:v1:* \
  resetchannels -@all +ping +getrange +set +del
```

业务用户允许：

```text
无 key：PING
仅版本化 prefix：GETRANGE SET DEL
```

logical DB 固定为 0，因此连接初始化不需要也不获准 `SELECT`。

故意不允许：

```text
GET MGET KEYS SCAN FLUSHALL FLUSHDB TTL PTTL
CONFIG ACL EVAL SCRIPT SUBSCRIBE PUBLISH
其他 prefix、其他 environment、任意 channel
```

`allkeys-lru` 表示缓存值可能在 TTL 前被驱逐；这是正常 miss，不是事实丢失。Redis 无持久化也表示重启后 cold start 是预期行为。

## 7. 标准启动与基础健康检查

在仓库根目录：

```bash
make compose-secrets
make compose-config
make compose-up
make compose-smoke
```

然后读取状态：

```bash
docker compose \
  --project-name growthos \
  --file deploy/compose/compose.yaml \
  ps

curl --fail --silent --show-error http://127.0.0.1:8088/health
curl --fail --silent --show-error http://127.0.0.1:8088/ready
```

若使用非默认 `GROWTHOS_COMPOSE_WEB_PORT`，后续所有请求必须使用同一端口。不要为了 Redis 问题直接 `docker compose down --volumes`。

## 8. 安全诊断已知 key

### 8.1 建立受限 helper

以下 helper 不把 password 带到宿主机命令行或标准输出；Redis container 内从只读 Secret 读取：

```bash
growthos_compose() {
  docker compose \
    --project-name growthos \
    --file deploy/compose/compose.yaml \
    "$@"
}

growthos_redis() {
  growthos_compose exec -T redis sh -eu -c '
    export REDISCLI_AUTH="$(cat /run/secrets/redis_password)"
    exec redis-cli --raw --no-auth-warning --user growthos_api "$@"
  ' sh "$@"
}
```

基础连接：

```bash
growthos_redis ping
```

预期只输出：

```text
PONG
```

### 8.2 验证 StrategyID 后再构造 key

不要把任意用户输入拼进删除命令。以下 validation 支持完整 `uint64` 正数且拒绝前导零：

```bash
validate_strategy_id() {
  candidate=$1
  case "$candidate" in
    ''|0|0*|*[!0-9]*) return 1 ;;
  esac

  candidate_length=${#candidate}
  if [ "$candidate_length" -lt 20 ]; then
    return 0
  fi
  if [ "$candidate_length" -gt 20 ]; then
    return 1
  fi

  LC_ALL=C [ "$candidate" = 18446744073709551615 ] ||
    LC_ALL=C [ "$candidate" \< 18446744073709551615 ]
}

strategy_id=21003
environment=development

case "$environment" in
  development|test|staging|production) ;;
  *) printf '%s\n' 'invalid environment' >&2; return 1 2>/dev/null || exit 1 ;;
esac
if ! validate_strategy_id "$strategy_id"; then
  printf '%s\n' 'invalid canonical StrategyID' >&2
  return 1 2>/dev/null || exit 1
fi

strategy_key="growthos:${environment}:lottery:strategy:projection:v1:${strategy_id}"
printf 'target key: %s\n' "$strategy_key"
```

在执行删除前，操作者必须把打印的唯一 key 与工单中的 environment、StrategyID 和 projection version 再核对一次。

### 8.3 只验证，不打印 payload

本地 development 可对已知 key 做严格 shape 检查而不输出名称/奖品内容：

```bash
set -o pipefail
growthos_redis getrange "$strategy_key" 0 2097152 |
  jq -e --arg id "$strategy_id" '
    .schema == "growthos.lottery.strategy.projection" and
    .schema_version == 1 and
    .strategy.id == $id and
    (.strategy.awards | type == "array" and length >= 1 and length <= 1000)
  ' >/dev/null
```

exit 0 只证明几个可见 shape 条件，不等价于应用严格 codec/领域恢复。不要把 raw projection 复制到聊天、截图或公开工单。missing key 会让检查失败；它本身是正常 cold/evicted 状态。

业务 ACL 没有 `EXISTS`、`TTL`、`MEMORY`、`INFO` 或 `SCAN`。不要为了区分 miss/eviction 去加这些命令；结合请求路径、低基数日志和 acceptance 诊断。

## 9. 日志与观测

固定 outcome：

```text
hit miss read_error corrupt delete_error
fill_leader fill_joined source_error write_ok write_error
```

读取最近十分钟缓存日志：

```bash
growthos_compose logs \
  --since 10m \
  --no-color \
  --no-log-prefix \
  api |
  jq -Rrc '
    fromjson? |
    select(.cache_outcome != null) |
    {
      level,
      cache_outcome,
      duration_ms,
      suppressed_since_last
    }
  '
```

注意：

- `hit/miss/fill/source/write_ok` 是 debug；默认非 debug 日志级别看不到，不可据此断言缓存未运行；
- `read_error/delete_error/write_error` 是 warning，并按 kind 以 10 秒窗口限频；
- `suppressed_since_last` 是被抑制的同类 warning 数，不是请求总数；
- 事件不应包含 key、StrategyID、payload、Award name、Redis address、credential 或 raw cause；
- `source_error` 表示 MySQL/source path 失败，不应归因成 Redis error；
- `corrupt` 不是业务输入错误，对外仍由回源结果决定。

## 10. 故障判断矩阵

| Redis | MySQL | key | selection route | `/health` | `/ready` | 证据级别 |
| --- | --- | --- | --- | ---: | ---: | --- |
| up | up | valid warm | 200，strict hit 后逐请求选择 | 200 | 200 | acceptance + unit |
| down | up | 任意 | cache read 有界失败，MySQL 成功则 200 | 200 | 200 | acceptance 实测 |
| up | down | valid warm | 该 key 可 200 | 200 | 503 | acceptance 实测 |
| up | down | cold/miss | 沿用 MySQL error，当前场景 503 | 200 | 503 | acceptance 实测 |
| up | up | poison | 不服务 poison；精确 DEL、回源、回填，source 成功则 200 | 200 | 200 | acceptance 实测 |
| up | down | poison | 不服务 poison；删除后 source 失败，沿用 MySQL error | 200 | 503 | codec/reader 设计与单测；未单列 Compose 注入 |
| down | down | 任意 | 无可用读取路径，沿用 MySQL error | 200 | 503 | 设计推导；未单列 Compose 注入 |
| disabled | up | 不适用 | 直接 MySQL，成功则 200 | 200 | 200 | acceptance direct M1 |
| disabled | down | 不适用 | 沿用 MySQL error | 200 | 503 | 组合语义；未单列 M1 |

其他实现级边界：

| 情况 | 当前请求 | 后续状态 | 证据 |
| --- | --- | --- | --- |
| `SET` failure，source success | 成功 | key 仍 cold，后续再回源 | unit |
| poison 的 `DEL` failure，source success | 成功 | poison 可能仍在，下次再次检测 | unit |
| caller canceled | cancellation 优先 | 不由 fail-open 伪装成功 | unit |
| 同 key concurrent miss | caller 各自等待/取消，source 合并 | 只限当前进程 | unit + race |
| 不同 key miss | fill 不共享执行锁，只短暂共享 flight map 记账 mutex | 不因另一 key 的 source fill 串行 | unit + race |

## 11. 排障决策树

### 11.1 selection 失败且 `/ready` 503

优先排查 MySQL，而不是 Redis：

```text
/ready 503
  -> 检查 mysql service 是否 healthy
  -> 检查 migration/grants 是否 clean/exact
  -> 检查 API 的 source_error 和既有 Repository error
  -> 恢复 MySQL authority
  -> /ready 回到 200
```

warm key 偶尔 200 不能改变此结论。按[本地 Compose 运维手册](local-compose.md)和 [MySQL Migration 手册](mysql-migrations.md)恢复；不要删除 MySQL volume。

### 11.2 `/ready` 200，但出现 `read_error`

只说明当前 `/ready` 的 MySQL `PingContext` 成功，而缓存访问正在退化；它不证明 Strategy SQL、表、授权、Migration 或数据正确。要证明权威 source path，必须再执行有效业务请求，并核对 source observation 与 MySQL SQL/execute 证据：

1. `growthos_compose ps redis` 确认只检查目标 project；
2. `growthos_redis ping` 验证 named user/Secret/网络；
3. 检查 API、Redis service 是否同时接入 `cache` network，且 Redis 无 host port；
4. 检查 Secret 是否来自同一批，不读取内容；
5. 检查 lookup/dial/read/pool timeout 是否被错误放大；
6. 观察 MySQL source load 和数据库容量，Redis outage 会增加回源；
7. 修复 Redis 或通过受评审配置关闭 cache，不扩大 ACL、不加入 retry storm。

### 11.3 持续 `write_error`

当前请求可能仍成功，但缓存无法 warm：

- 确认 Redis health 与 business `PING`；
- 确认 key environment 与 ACL prefix 一致；
- 确认 projection 未超过 2 MiB/1000 Awards；
- 确认 memory policy 未被非受控配置修改；
- 检查 write/pool timeout 与连接池饱和；
- 不通过取消 TTL、开放永久 `SET` 或扩大 ACL 规避。

### 11.4 `corrupt` 或 poison

Reader 的正常自动修复顺序：

```text
strict decode fails
  -> best-effort DEL exact key
  -> MySQL source load
  -> strict encode
  -> SET exact key with TTL
```

如果 MySQL 正常，一次合法业务请求应完成修复。如果 `corrupt` 重复：

1. 检查是否存在不同应用版本共用同一 v1 namespace；
2. 检查是否有超出 ACL 边界的运维账号/脚本写入；
3. 检查 `delete_error` 与 `write_error`；
4. 若多个 key 同时受影响，先通过开关关闭 cache，避免重复回源和日志；
5. 修复 writer/version/ACL 后，只对已核对的 exact key 执行下一节删除；
6. 发一个合法 bodyless selection 请求重新回填；
7. 验证 route、readiness 和 strict shape。

不要把 poison 内容导出、修改后写回；权威恢复来源只能是 MySQL。

### 11.5 命中内容疑似陈旧

当前系统没有 Strategy 写路径，先核对是否有人绕过应用直接改 MySQL。如果确实发生受控事实变更：

- 默认等待最长 5 分钟有界窗口；或
- 在影响需要立即消除且目标完全确认时，精确删除一个 key；
- 由下一次读取从 MySQL 回填。

不要因此声称系统已有写后精准失效；任何正式写路径上线前都要另立 ADR 和并发验收。

## 12. 精确删除与安全回填

执行前必须已经完成第 8.2 节的 canonical ID、environment、version 三次核对，并保存唯一 `strategy_key`。

### 12.1 删除一个 key

```bash
deleted_count=$(growthos_redis del "$strategy_key")
case "$deleted_count" in
  0) printf '%s\n' 'exact key was already absent' ;;
  1) printf '%s\n' 'exact key was deleted' ;;
  *) printf '%s\n' 'unexpected DEL result; stop and investigate' >&2; return 1 2>/dev/null || exit 1 ;;
esac
```

`DEL` 只接收一个明确参数。禁止：

```text
FLUSHALL
FLUSHDB
KEYS growthos:* | xargs redis-cli DEL
SCAN + pattern delete
EVAL 批量删除
删除 Redis container/volume 来清一个 Strategy
```

### 12.2 触发回填

development/test 的临时 route：

```bash
curl --fail --silent --show-error \
  --request POST \
  --header 'Accept: application/json' \
  --header 'X-GrowthOS-Demo-Mode: ephemeral-selection' \
  "http://127.0.0.1:8088/api/v1/lottery/strategies/${strategy_id}/ephemeral-selections" \
  >/dev/null
```

该请求会执行一次新的随机 ephemeral selection；不要透明重试 timeout 或不确定响应。它不适用于 staging/production 的正式恢复，也不产生可查询 Draw/Result。

回填后：

```bash
curl --fail --silent --show-error http://127.0.0.1:8088/ready >/dev/null

set -o pipefail
growthos_redis getrange "$strategy_key" 0 2097152 |
  jq -e \
    --arg id "$strategy_id" \
    '.schema_version == 1 and .strategy.id == $id' \
    >/dev/null
```

## 13. 依赖恢复顺序

### 13.1 Redis 单独故障

只对已确认的 GrowthOS project：

```bash
growthos_compose up --detach --wait --wait-timeout 60 redis
growthos_compose ps redis
growthos_redis ping
```

Redis 无持久化，重启后 cold 是预期行为。不要预先全量扫描/预热；让已知业务读取按需回填，并观察 MySQL 负载。

### 13.2 MySQL 单独故障

```bash
growthos_compose up --detach --wait --wait-timeout 120 mysql
curl --fail --silent --show-error http://127.0.0.1:8088/ready >/dev/null
```

若 MySQL 不是简单 container stop，而是 migration、grant、credential 或数据问题，停止在这里，按专门 MySQL runbook 恢复；不要靠 warm Redis 掩盖 authority 故障。

### 13.3 Redis 与 MySQL 同时故障

恢复顺序：

1. 先恢复 MySQL 并让 `/ready` 回到 200；
2. 再恢复 Redis named user/Secret/internal network；
3. 让请求按需回填；
4. 观察 source load，防止 cold start 压力；
5. 验证一个已知 exact key，不做全库扫描。

这个顺序强调权威源优先。不要先把 Redis 临时改造成可写事实库。

## 14. 功能撤销与回滚

当 Redis 故障持续、MySQL 回源放大可控但缓存恢复不可预期，最小可逆措施是关闭 Strategy cache：

1. 确认 MySQL 有能力承受 direct path；
2. 通过受评审部署配置设置 `GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED=false`；
3. config validation；
4. 只重新创建 API，保留 MySQL；
5. 验证 `/health`、`/ready` 与业务 route；
6. 用 source-load 证据确认 direct MySQL；
7. Redis key 等待 TTL 自然过期，不 flush；
8. 保留故障日志、时间线和配置 revision，不保留 Secret/raw payload。

回滚代码版本时也要保留 namespace/schema 兼容性。若旧版本不认识当前 projection，应关闭 cache 或使用旧版本独立 namespace；不要把 v1 payload 原地改成另一格式。

## 15. 隔离 acceptance 与 M1

### 15.1 唯一推荐的故障演练入口

```bash
make compose-lottery-api-acceptance
```

该脚本会：

- 创建随机唯一 `growthosl24<24-hex>` Compose project；
- 使用 Docker-assigned loopback port；
- 创建独立 Secret、volume、image 和 buildx builder；
- 验证 Compose build/config、migration/grants、ACL 正反命令和 topology；
- 验证 strict MaxUint64、poison repair、HTTP 正反契约与并发选择；
- 停止/恢复隔离 Redis 与 MySQL；
- 运行 warm/direct/Redis-down 三组 M1；
- 按 label、resource ID、临时目录 identity/type 精确清理；
- 证明长期 `growthos` project 的资源身份未改变。

不要复制脚本中的 stop/remove 步骤到长期 project，也不要把随机 project 名替换为 `growthos`。

### 15.2 M1 内部负载命令

acceptance 对三种状态使用同一命令形状：

```bash
go run ./cmd/healthload \
  -url 'http://127.0.0.1:<docker-assigned-port>/api/v1/lottery/strategies/21003/ephemeral-selections' \
  -method POST \
  -ephemeral-selection=true \
  -rate 50 \
  -duration 10s \
  -workers 16 \
  -timeout 3s \
  -expected-status 200
```

`<docker-assigned-port>` 与 warm/disabled/Redis-down 状态由 acceptance 管理。不要只手工跑这条 healthload 就声称完成 M1；路径证明还需要 MySQL statement counter、cache outcome、严格成功计数和独立场景重置。

2026-08-30 实际单次结果见[第 24 节 QA](../qa/lessons/lesson-24.md)。它是本机开发证据，不是 SLO。

## 16. 停止与清理

普通停止：

```bash
make compose-down
```

该命令不应删除 MySQL named volumes。不要额外运行全局 prune 或手工删 volume。

隔离 acceptance 自己负责清理；若它异常终止，先按脚本输出取得 exact random project、label 和 resource ID，再使用脚本既有清理/重跑路径。不要按 `growthosl24*` 通配删除，因为可能有另一轮或其他操作者的验收正在运行。

缓存数据无需备份：Redis restart/eviction 后由 MySQL 按需重建。需要保留的是：

- 发生时间和受影响环境；
- API/Redis/MySQL service 状态；
- 低基数 outcome 与抑制数；
- HTTP status/error code/request ID；
- 部署 revision、开关和非 Secret timeout；
- exact StrategyID 仅放在受控工单，不放公共日志。

不得保留：Secret、ACL password、raw projection、完整 Redis error cause、连接地址凭据或 Docker 临时 acceptance 文件。

## 17. 值班检查清单

### 17.1 事件开始

- [ ] 已确认 environment、Compose project/部署 revision；
- [ ] 没有停止或修改用户外部 Redis/MySQL；
- [ ] `/health` 与 `/ready` 已分别记录；
- [ ] 已区分 Redis `read_error` 与 MySQL `source_error`；
- [ ] 没有读取/打印 Secret 或 raw projection；
- [ ] 已评估 Redis fail-open 对 MySQL source load 的放大。

### 17.2 执行修复

- [ ] 优先恢复 MySQL authority；
- [ ] Redis 修复不增加 ACL、不开放 host port、不加入 readiness；
- [ ] poison/陈旧只处理一个已验证 exact key；
- [ ] 未执行 `FLUSHALL/FLUSHDB/KEYS/SCAN/pattern delete`；
- [ ] 需要撤销时通过 reviewed config 关闭 cache 并重建 API；
- [ ] 没有透明重试不确定的 ephemeral selection。

### 17.3 恢复后

- [ ] `/ready` 为 200；
- [ ] Redis business `PING` 正常或 cache 已明确关闭；
- [ ] 已知 exact key 可由 MySQL 回填并通过最小 shape 检查；
- [ ] `read_error/write_error/corrupt` 不再持续；
- [ ] MySQL source load 回到预期；
- [ ] 未把单个 warm hit 当完整恢复；
- [ ] 事件记录不含 Secret/raw payload；
- [ ] 需要新增写路径、HA、精准失效或容量治理时另立章节/ADR。

## 18. 能准确表述与不能表述

可以表述：

> Redis Strategy cache 是 MySQL 权威读取前的可丢弃 cache-aside decorator；使用严格 v1 projection、最多 5 分钟减 0～10% TTL jitter、同进程同 key source flight、Redis fail-open、精确 poison repair、命名最小 ACL 与 internal network。Redis 不进入 readiness，故障恢复不需要也不允许 flush 全库。

不能表述：

- Redis 已成为 Strategy 数据库、备份或灾备恢复点；
- warm hit 证明 MySQL 健康；
- singleflight 是分布式锁或跨实例击穿保护；
- 当前已有 Strategy 写后精准失效；
- cache error 与 MySQL error 都会无条件成功；
- 允许缓存用户、权限、资格、风控、库存或随机结果；
- 本地 Compose 的明文 internal Redis 可直接复制到 production；
- 一次 M1 latency 是生产 SLO 或容量结论。
