# 第 24 节 QA：Redis Strategy Cache-Aside 读取投影验收

- **章节：** [第 24 节：第一次 Redis 缓存](../../course/part-03/lesson-24-redis-strategy-cache.md)
- **架构决策：** [ADR-0020](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)
- **API 记录：** [第 24 节 API](../../api/lessons/lesson-24.md)
- **运维手册：** [Redis Strategy Cache Runbook](../../runbooks/redis-strategy-cache.md)
- **基准提交：** `27a552b673ff2f8dfac815e8f765c59574fce9df`（第 23 节已验收 tip）
- **运行时证据提交：** `d33723c1ce372050ed531272c45dbddcc23e42ca`
- **验收日期：** 2026-08-30，Asia/Shanghai
- **当前记录状态：** 隔离 Compose acceptance、文档合并后的 `make verify` 与全仓 `go test -race ./...` 均已实际通过

> 本节验收的不是“Redis 能 `SET/GET`”，而是一个受控的 cache-aside 边界：MySQL 仍是唯一事实源；Redis 只保存完整、严格、可重建的 Strategy 投影；缓存 miss、损坏或不可用都不改变既有 HTTP/Repository 语义；缓存不能吞掉 MySQL 错误、复用随机选择结果或扩大权限面。

## 1. 验收范围

### 1.1 必须成立

- 缓存默认关闭，启用时配置、Secret、TLS 和 timeout budget 在启动前严格校验；
- MySQL `StrategyRepository` 保持权威，Redis 只装饰 application `StrategyReader`；
- key 固定包含 namespace、environment、对象语义和 schema version；
- projection v1 完整包含 Strategy 与 Awards，并经过独立严格 codec；
- payload 最大 2 MiB，Award 数最大 1000，ID/weight 保持 canonical `uint64` decimal string；
- TTL 最大 5 分钟，并减去 0～10% 抖动；
- hit 跳过 MySQL；miss/error/corrupt 回源；source error 原样保留；回填失败不覆盖成功 source read；
- 同 key 只在单进程内合并 MySQL source load，selector 仍逐请求执行；
- 不做 not-found/error 负缓存；
- Redis 不进入 startup probe 或 `/ready`；
- Compose 使用命名用户、最小命令/精确 key ACL、关闭 default user、internal cache network 和 Secret；
- 公开 route、DTO、header、status 和 error code 零变化；
- 隔离 acceptance 能验证正常、poison、Redis down、MySQL down、恢复、ACL 与真实 M1 三条路径。

### 1.2 明确不验收

- Strategy 创建、更新、发布、删除或精准失效；
- 负缓存、Bloom filter、缓存预热、分布式锁、跨实例 singleflight；
- 用户、会话、权限、资格、风控、库存、随机数、selection、Draw 或 Result 缓存；
- Redis Sentinel/Cluster、复制、持久化、生产 TLS 证书部署、容量规划或灾备；
- 正式抽奖幂等、结果查询或奖励发放；
- 用本机一次 M1 推导生产命中率、容量、P99 或 SLO。

## 2. 核心命题与证据矩阵

| 命题 | 自动化/运行时证据 | 必须同时检查的负向边界 | 当前结论 |
| --- | --- | --- | --- |
| MySQL 是唯一事实源 | miss/down 路径的 MySQL statement counter；MySQL down cold read 失败 | warm hit 成功不能改写 `/ready` | acceptance 已通过 |
| hit 真正绕过 MySQL | warm M1：500 hit、MySQL executes 0 | 不能只看低延迟猜路径 | acceptance 已通过 |
| disabled 真正直连 MySQL | direct M1：500 source load、1000 executes、cache events 0 | 不能只关掉 Redis 容器冒充 disabled | acceptance 已通过 |
| Redis failure 能 fail-open | Redis-down M1 全部 200，source/execute counter 非零 | MySQL error 不能被吞掉 | acceptance 已通过 |
| projection 不可信 | codec 单测；poison exact key 自动删除并重建 | 坏值不能进入 selector，不能 pattern delete | acceptance 与实现测试已覆盖 |
| 击穿保护边界准确 | per-key flight 并发/取消/race 测试 | 不同 key 不串行；selector 不合并；非跨实例 | 实现测试已覆盖 |
| TTL 有界且分散 | TTL 上限、jitter clamp 与 atomic `SET` 测试 | 不允许永久 key；不声称精准失效 | 实现测试已覆盖 |
| ACL 最小化 | 正向无 key PING、前缀内 GETRANGE/SET/DEL；GET/SCAN/CONFIG/ACL/EVAL/PubSub/越界 key 负向 | default user 关闭，channels 清空 | acceptance 已通过 |
| 公开 API 不漂移 | 原 route 正反契约、MaxUint64、no_reward、gateway 413/502/504 | 无 cache header/code/route；仍无 body/幂等 | acceptance 已通过 |
| 故障恢复安全 | Redis/MySQL 停止、恢复、重新回填，业务表 fingerprint 不变 | 不靠 FLUSHALL、删 volume 或改 ACL 恢复 | acceptance 已通过 |
| 全仓最终状态无回归 | `make verify`、`go test -race ./...` | 不以已通过的 Compose 替代全仓检查 | 最终复验已通过 |

## 3. 按提交核查真实演进

从第 23 节 tip 开始按顺序检查：

| 顺序 | 提交 | 验收焦点 |
| --- | --- | --- |
| 1 | `272d028` | 默认关闭配置、Redis Secret/TLS/pool/budget 校验 |
| 2 | `44e7ed1` | 默认按需连接、无启动 PING、窄 `GETRANGE/SET/DEL` surface |
| 3 | `3475fa2` | TTL 上限由过宽范围收紧到 5 分钟 |
| 4 | `764f74a` | ADR 固化事实源、投影、fail-open 与非目标 |
| 5 | `765bb00` | codec、cache-aside、poison repair、jitter、per-key flight |
| 6 | `68fa59b` | composition root 开关、pool 生命周期、低基数 observer |
| 7 | `17e7010` | Compose Redis、Secret、internal network、ACL、acceptance |
| 8 | `c4c622f` | healthload 支持严格 bodyless POST |
| 9 | `b7250f` | `resetchannels` 撤销隐式 Pub/Sub 权限 |
| 10 | `8804f87` | M1 请求补齐 ephemeral acknowledgement header |
| 11 | `40f1acf` | Docker Desktop 低资源构建预算 |
| 12 | `d33723c` | warm/direct/Redis-down 同口径 M1 与 source-load 证据 |

检查历史：

```bash
git log --reverse --oneline \
  27a552b673ff2f8dfac815e8f765c59574fce9df..d33723c1ce372050ed531272c45dbddcc23e42ca

git diff --stat \
  27a552b673ff2f8dfac815e8f765c59574fce9df..d33723c1ce372050ed531272c45dbddcc23e42ca
```

通过条件不是 commit 数量本身，而是每一步只解决表中对应问题，后续修正没有被 squash 成“第一版就完美”的假历史。

## 4. 配置与启动边界

### 4.1 开关和默认值

```text
GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED=false
GROWTHOS_LOTTERY_STRATEGY_CACHE_TTL=5m
GROWTHOS_LOTTERY_STRATEGY_CACHE_LOOKUP_TIMEOUT=75ms
GROWTHOS_LOTTERY_STRATEGY_CACHE_WRITE_TIMEOUT=75ms
GROWTHOS_LOTTERY_STRATEGY_CACHE_FILL_TIMEOUT=2s
```

校验要点：

- cache disabled 时 Redis password 不是 API 启动前提；
- enabled 时 `GROWTHOS_REDIS_PASSWORD` 与 `GROWTHOS_REDIS_PASSWORD_FILE` 必须恰好一个；
- TTL 必须在 `1s..5m`；lookup/write 必须在 `1ms..1s`；fill 必须在 `1ms..30s`；
- lookup + fill + write 不得超过 Lottery selection timeout；
- staging/production enabled 时 TLS mode 必须为 `verify_identity`；
- TLS disabled 时不得留下 CA file；
- pool size、minimum idle 与 operation timeout 都有上界；
- validation error 不回显密码、地址值或 Secret 文件内容。

对应测试集中在：

```bash
go test ./internal/platform/appconfig
```

### 4.2 Redis client 不成为启动权威

`redisstore.Open` 构造 pool，但不执行 `PING` 或同步可用性探测。默认 `MinIdleConns=0` 时首命令建连；正值允许 go-redis 后台预建连接，其失败不会由 `Open` 同步返回。必须验证：

- Redis down 时 API 仍可启动；
- `MaxRetries=-1`，应用不做隐藏 Redis retry；
- command wrapper 拒绝 nil context、空/非法 key、非法 range、空 payload 与非正 TTL；
- `SET` 原子携带 expiration；
- `DEL` 方法只能删一个精确 key；
- `Close` 并发安全且幂等；
- driver cause 可供可信代码检查，但日志/字符串只渲染稳定 stage。

对应测试：

```bash
go test ./internal/infrastructure/redisstore
```

## 5. projection codec 验收

固定 key 模式：

```text
growthos:<environment>:lottery:strategy:projection:v1:<canonical-strategy-id>
```

development 示例：

```text
growthos:development:lottery:strategy:projection:v1:21003
```

合法 v1 值必须恰好包含：

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

必须拒绝：

- 空值、超过 2 MiB 的值、第二个 trailing JSON value；
- schema/version 错误、字段缺失、未知字段、大小写变体、重复 JSON name；
- 非 canonical decimal、零/溢出 ID 或 weight；
- 空 Awards、超过 1000 个 Awards、重复 AwardID；
- 非法 outcome、非法名称或其他领域不变量；
- payload 内 StrategyID 与请求 ID 不同；
- 过深 JSON。

验收原则：Redis 数据必须先通过 wire shape、范围和领域恢复，不能因为“由自己写入”就信任。MaxUint64 场景还必须证明 JSON string 不发生 JavaScript/float 精度损失。

对应测试：

```bash
go test ./internal/lottery/adapter/strategycache -run Projection
```

## 6. cache-aside 状态机验收

| 输入状态 | 必须观察到 | 明确禁止 |
| --- | --- | --- |
| valid hit | `hit`，返回恢复后的 Strategy，source 0 次 | 不重新写缓存；不跳过领域恢复 |
| miss | `miss`、flight、source，成功后 `write_ok` | 不把 miss 当 not found |
| Redis read error/timeout | `read_error` 后回源 | 不新增 Redis HTTP error |
| corrupt/oversize/ID mismatch | `corrupt`、best-effort 精确 `DEL`、回源回填 | 不交给 selector；不删 namespace |
| MySQL not found | `source_error` 与原 not-found | 不负缓存；不返回 `no_reward` |
| MySQL retryable/permanent error | 原 Repository error | 不由 fail-open 吞错 |
| encode/SET failure | source Strategy 成功返回，`write_error` | 不让缓存写失败覆盖业务成功 |
| caller canceled | caller cancellation 优先 | 不因 cache fallback 延长已取消请求 |

关键单测：

```bash
go test ./internal/lottery/adapter/strategycache -run 'Hit|Miss|Failure|Corrupt|Negative|Cancellation'
```

## 7. TTL 与陈旧窗口

配置 TTL 为 `T` 时，实际写入 TTL 为：

```text
T - uniform_clamped_jitter(0..T/10)
```

默认 `T=5m`，因此写入时的物理 TTL 位于 `4m30s..5m`。验收必须确认：

- jitter 只减不加，所以值不会比配置窗口更陈旧；
- 越界 jitter 实现会被 clamp；
- `SET` 与 TTL 是一个命令，不存在先写永久 key 再 `EXPIRE` 的窗口；
- 当前没有 Strategy 写路径，只承诺最多 5 分钟的有界陈旧；
- 未来出现 Update/Publish/Delete 时必须另立 ADR，不能把当前 TTL 说成精准失效。

不通过示例：固定所有 key 为整点 5 分钟过期、允许 24 小时 TTL、没有 expiration 的 `SET`、把 Redis 剩余 TTL 当业务版本。

证据采用分层口径：配置与 Reader/Store 单测证明传给 `SET` 的 expiration/jitter 范围以及 expiration 与 value 同命令；隔离 acceptance 的 ACL helper 明确执行 `SET ... EX 30`，应用 refill 路径证明带 expiration 的写入可读/可重建。go-redis 对应用 duration 的 wire 编码也可能是 `PX`，因此不能把 helper 的 `EX` 冒充应用命令一定使用 `EX`。runtime ACL 故意没有 `TTL/PTTL`，所以本轮没有采样服务器实际剩余 TTL，不能把计算边界写成真实 TTL probe。

## 8. 并发与 singleflight 边界

实现测试必须覆盖：

- 同 key 冷读只产生一个 leader source call，其余 caller join；
- 不同 key 的 fill 可以并行；
- 等待者取消只结束自身，不毒化共享 fill；
- leader caller 取消不能无限悬挂共享 fill；
- process lifecycle 取消能终止共享 fill；
- abandoned fill 受 `FillTimeout` 约束；
- source error 不被持久化或负缓存；
- 获得 Strategy 后，每个请求仍独立执行 `WeightedSelector.Select`。

对应测试：

```bash
go test -race ./internal/lottery/adapter/strategycache \
  -run 'Concurrent|DifferentKey|Selection|Cancellation|Abandoned|Lifecycle'
```

这条命令既可单独复验窄并发边界，也已被随后实际通过的全仓 `go test -race ./...` 覆盖；全仓 race 仍不能替代真实 Redis/MySQL 故障注入。

## 9. composition、readiness 与观测

### 9.1 可逆装配

```text
disabled: MySQL Repository -> SelectionService
enabled:  MySQL Repository -> Cache Reader -> SelectionService
```

必须验证：

- enabled 才创建并注入 Redis runtime；
- enabled 但 cache runtime 缺失，或 disabled 却意外注入，都拒绝启动；
- MySQL pool 仍由 Repository、readiness 和 runtime 共享；
- Redis/MySQL pool 在正常退出和部分启动失败时都精确关闭；
- Redis shutdown error 脱敏，不打印地址/credential/cause。

### 9.2 readiness

`/health` 只回答进程是否存活；`/ready` 仍以 MySQL 为权威依赖。正反预期：

| 条件 | `/health` | `/ready` | 原因 |
| --- | ---: | ---: | --- |
| Redis down，MySQL up | 200 | 200 | 可回源，不应摘流 |
| Redis up，MySQL down，warm key | 200 | 503 | 单个 warm key 不代表完整读取能力 |
| Redis up，MySQL down，cold key | 200 | 503 | 权威源不可用 |

### 9.3 低基数 observation

允许固定 kind：

```text
hit miss read_error corrupt delete_error
fill_leader fill_joined source_error write_ok write_error
```

事件不得携带 key、StrategyID、payload、Award name、Redis address、credential 或 raw driver error。普通 outcome 为 debug；`read_error/delete_error/write_error` 按 kind 以 10 秒窗口限频，并记录抑制数。Redis-down M1 的 500 次 read failure 只产生 1 条 `read_error` warning，正是限频证据，不表示只失败过一次。

## 10. Compose 安全边界

ACL 必须精确等价于：

```text
user default off
user growthos_api on >REDACTED \
  resetkeys ~growthos:development:lottery:strategy:projection:v1:* \
  resetchannels -@all +ping +getrange +set +del
```

正向命令：

```text
无 key：PING
仅版本化 prefix：GETRANGE SET DEL
```

配置与 adapter 还必须拒绝非 0 logical DB，避免 go-redis 在连接初始化阶段发送 ACL 未授权的 `SELECT`。

负向门禁至少覆盖：

- prefix 内 `GET`；
- prefix 外 `SET`；
- `SCAN`、`CONFIG`、`ACL`、`EVAL`；
- `SUBSCRIBE`、`PUBLISH`；
- 未认证 default user；
- 任意通配 key/namespace 管理能力。

拓扑还必须满足：

- `cache` network 为 internal，只有 API 与 Redis；
- Redis 不发布宿主机端口；
- `redis_password` 只挂载到 API 与 Redis；
- web、mysql、migrate、mysql-grants 不接入 cache 网络；
- Redis `/data` 是 64 MiB tmpfs，不持久化；
- `maxmemory 48mb`、`allkeys-lru`，缓存可被驱逐；
- cache 不成为 API `depends_on`、startup 或 readiness authority。

## 11. 公开 API 回归

唯一业务 route 仍是：

```http
POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections
Accept: application/json
X-GrowthOS-Demo-Mode: ephemeral-selection
```

必须证明：

- bodyless POST 成功，response 仍是 `durability=ephemeral`；
- reward/no_reward 只来自配置 Awards；
- `Cache-Control: no-store`、`Content-Type`、`X-Request-ID` 保持；
- MaxUint64 ID 以 decimal JSON string 保真；
- missing/invalid ID、header、query、method、path、body framing、idempotency 按原 code/status 拒绝；
- 16385-byte gateway body 仍返回 correlated JSON 413；
- API down 时 Nginx 仍生成 correlated JSON 502/504；
- 没有 `X-Cache`、`Age`、cache DTO/route/error code；
- 每次 HTTP 请求仍进行一次随机选择，不透明重试。

详细 public contract 见[第 24 节 API 记录](../../api/lessons/lesson-24.md)。

## 12. 隔离 Compose 故障与恢复验收

实际执行命令：

```bash
make compose-lottery-api-acceptance
```

结果：**已实际通过。** 脚本为每次运行创建随机、唯一的 `growthosl24<24-hex>` project 与 Docker-assigned loopback port，使用独立 Secret、volume、image 和 buildx builder；退出时按 Compose label、资源 ID、临时目录 identity/type 精确清理，并证明长期 `growthos` project 的容器、volume、network 身份未变化。

### 12.1 已实际覆盖的故障矩阵

| 场景 | selection route | `/ready` | 恢复证据 | 结果 |
| --- | --- | ---: | --- | --- |
| Redis stopped，MySQL up | `21003` 返回 200，经 MySQL | 200 | Redis 恢复后请求回填严格 v1 projection | 通过 |
| MySQL stopped，Redis warm | `21003` 返回 200 | 503 | MySQL 恢复后 ready 200 | 通过 |
| MySQL stopped，Redis cold | 未知 `999998` 返回既有 503 | 503 | 恢复后删除精确 key、请求并回填 | 通过 |
| poison exact key | 请求仍返回合法 no_reward | 不变 | poison 被删并替换为合法 v1 projection | 通过 |
| API stopped | gateway 返回既有 JSON 502/504 | 上游不可用 | API 恢复 healthy | 通过 |

“Redis 与 MySQL 同时 down”可由设计推导为没有可用读取路径、沿用 MySQL error mapping，`/health` 200、`/ready` 503；本次脚本没有单独把它登记成独立故障注入，因此不把推导冒充实测。

### 12.2 其他已通过项目

- Compose build/config 和所有 service 预期状态；
- schema clean version 2 与 exact MySQL grants；
- Redis topology、ACL 正反命令、disabled default user 与 memory policy；
- max/no_reward/1:3 multi-award fixture 原子插入；
- 首次 miss 回填严格 v1 projection；
- 总计 64 个请求、最大并行度 16，只返回配置 outcome；
- 全部 HTTP 后业务表 fingerprint 不变；
- 故障恢复后的 migration/grants/loopback port 仍精确；
- 所有 disposable resource 和 acceptance 临时证据被精确清理。

## 13. M1 方法与真实结果

### 13.1 同口径方法

三组均使用：

```text
route:    loopback Nginx -> Go API -> bodyless ephemeral POST
fixture:  Strategy 21003，两个 Award，权重 1:3
rate:     50 req/s
duration: 10s
workers:  16
timeout:  3s
expected: HTTP 200
header:   X-GrowthOS-Demo-Mode: ephemeral-selection
```

healthload 必须报告 `method=POST`、`ephemeral_selection=true`、`scheduled=completed=success`、errors/unexpected/dropped 全为 0，并且不会透明重试 transport failure。

路径证据来源：

- MySQL `performance_schema` 按应用账号统计两条 Strategy prepared SELECT 的 execute 增量；
- 一次 source load 固定执行 Strategy 与 Awards 两条 SELECT，因此 500 source loads 对应 1000 executes；
- API JSON 日志统计低基数 cache outcome；
- 每组开始前重置到明确 warm/disabled/stopped 条件，不通过延迟猜测来源。

### 13.2 2026-08-30 本机单次快照

| 场景 | scheduled/completed/success | errors/unexpected/dropped | actual RPS（四位小数） | P50 ms | P95 ms | P99 ms | max ms | 路径证据 |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| warm cache | `500/500/500` | `0/0/0` | 50.0862 | 1.730167 | 4.129458 | 5.202 | 7.387375 | MySQL executes `0`；hits `500` |
| cache disabled / direct MySQL | `500/500/500` | `0/0/0` | 50.0770 | 2.390709 | 5.382042 | 9.747167 | 25.003417 | MySQL executes `1000`；source loads `500`；cache events `0` |
| Redis down / fail-open | `500/500/500` | `0/0/0` | 50.0505 | 2.602833 | 5.777708 | 8.222959 | 10.596541 | MySQL executes `1000`；source loads `500`；leaders `500`；joined `0`；read-error logs `1` |

三组的 scheduled/completed/success 都是 500/500/500，且 errors/unexpected/dropped 都是 0/0/0，证明这三个固定负载窗口没有丢弃；warm 的 0 execute 与 500 hit、direct 的 0 cache event、Redis-down 的 source/leader/read-error 共同证明路径，不只是展示 latency。

### 13.3 能得出与不能得出的结论

能得出：

- 这次本机运行中 warm path 没有执行两条 MySQL source SELECT；
- disabled path 确实逐请求加载 source；
- Redis-down path 在有界 cache failure 后逐请求回源且全部成功；
- 三条路径的实际 RPS 接近相同 schedule，比较口径一致；
- 10 秒 warning throttle 避免 Redis outage 每请求打一条 warning。

不能得出：

- production P99/SLO、峰值 RPS、容量上限或成本节省；
- 长时间 steady-state 命中率、eviction 行为或连接池最优值；
- 多 API instance 的跨进程击穿效果；
- Redis HA、网络分区、TLS、复制延迟或 failover 结论；
- “缓存一定比数据库快”的普适排名。direct 单次 max 的差异也不能解释为稳定尾延迟结论。

这是一份本机单次开发证据，不是性能承诺。若要升级为 SLO 证据，需要固定硬件/容器资源、重复次数、预热、数据规模、并发模型、统计置信区间和生产相似拓扑，并将容量门槛单独评审。

## 14. 最终复验顺序

已实际通过：

```bash
make compose-lottery-api-acceptance
make verify
go test -race ./...
git diff --check
```

最终 `make verify` 实际完成 Go vet/test、文档检查、前端 19 个测试文件共 152 个测试、TypeScript typecheck 与 Vite production build，exit 0；全仓 race 覆盖全部 Go package，exit 0。Compose acceptance 仍是独立的真实依赖/故障证据，不能由前两项替代。

记录规则：

1. 只有命令实际 exit 0 才记录为“通过”；
2. `make verify` 包含 format/vet/test/doc-check/web verify，但不替代 race；
3. Compose acceptance 不替代全仓源码/文档/前端门禁；
4. race 通过不替代真实 Redis/MySQL 故障注入；
5. `git diff --check` 只检查 whitespace，不证明链接和技术内容正确；
6. 工作树中其他章节的协作修改必须由根任务统一归属，不能由本 QA 擅自清理。

## 15. 精确负向边界

任一项出现即不通过：

- cache hit 返回一个未经过严格 decode/领域恢复的结构；
- Redis error 直接新增 `redis_unavailable` HTTP code；
- MySQL error 被改成 cache miss、`no_reward` 或成功；
- not-found/error 被负缓存；
- TTL 超过 5 分钟、加 jitter、没有 expiration 或依赖第二条 `EXPIRE`；
- key 缺 environment/schema version，或客户端能提交完整 key；
- 缓存只存 Strategy 不存 Awards，导致半聚合回源；
- 缓存权限、资格、风控、库存、随机选择或正式结果；
- singleflight 合并 selector、跨 key 全局锁或被描述为跨实例保护；
- Redis 被加入 `/ready` 或 API 的 required startup dependency；
- ACL 开放 `@all`、`GET`、`KEYS/SCAN`、Lua、Pub/Sub、外部 key 或 default user；
- Redis 暴露 host port，Secret 被非 API/Redis service 挂载；
- 日志包含 key、StrategyID、payload、Award name、地址、密码或 raw driver error；
- 用 `FLUSHALL`、`FLUSHDB`、pattern delete、删 Redis volume 或放宽 ACL 修复一个 poison key；
- 把 M1 一次结果写成生产 SLA/SLO 或“已完成容量规划”。

## 16. 剩余风险与后续责任

| 风险 | 当前控制 | 后续触发条件 |
| --- | --- | --- |
| TTL 内陈旧 | TTL 上限 5m，Redis 可丢，MySQL 权威 | 第一次 Strategy Update/Publish/Delete 前另立精准失效 ADR |
| 多实例同时 miss | 每进程同 key flight | 真实多实例 source amplification 证据出现后再评估分布式协调 |
| Redis outage 放大 MySQL | bounded timeout、fail-open、低基数 warning | 生产容量/熔断/限流需独立容量测试 |
| allkeys-lru eviction | miss 可回源，不做负缓存 | 实测 memory/eviction/命中率后做容量规划 |
| development 无 TLS | internal network、无 host port、named ACL/Secret | staging/production 必须 `verify_identity` 并部署受信 CA |
| projection rolling upgrade | namespace + schema + strict codec | schema v2 需双读/预热/回滚方案独立设计 |
| 无持久化 | cache 可从 MySQL 重建 | 不得把 Redis 纳入业务备份或恢复点 |
| 服务器实际剩余 TTL 未采样 | appconfig/Reader/Store 单测 + ACL helper `SET ... EX 30` + 应用带 expiration 的 refill | 需要实测窗口时使用独立受控观察身份，不扩宽 runtime ACL |
| 真实 2 MiB Redis value 未单列探针 | 2 MiB codec/sentinel 单测 + 有界 `GETRANGE` 实现 | 接近上限的真实 value 分布出现后补真实依赖与内存/网络测试 |
| Redis 与 MySQL 同时停止未单列注入 | 状态机/单测 + 两种单服务 outage/warm-cold Compose 证据 | 正式灾备/故障矩阵阶段补联合故障与恢复顺序演练 |

## 17. 验收结论

截至运行时证据提交 `d33723c1ce372050ed531272c45dbddcc23e42ca`，`make compose-lottery-api-acceptance` 已实际通过，证明了隔离 Compose 构建/config、最小 ACL、严格 projection、poison repair、Redis/MySQL 故障与恢复、公开 API 回归，以及 warm/direct/Redis-down 三组真实 M1 路径。

文档合并后的 `make verify` 与全仓 `go test -race ./...` 已实际 exit 0；交叉链接由 doccheck 通过。第 24 节因此具备封板所需的源码、前端、文档、race 与真实依赖故障证据；其边界仍限于本 QA 明确列出的 Strategy 读取投影，不扩张为正式 Draw、业务 SLO 或生产就绪声明。
