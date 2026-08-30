# 第 21 节 API 记录：development/test 临时 Lottery Selection

## 1. 本节 API 变化

第 21 节新增一个版本化业务路由：

| Method | Path | 可用环境 | 语义 |
| --- | --- | --- | --- |
| `POST` | `/api/v1/lottery/strategies/:strategy_id/ephemeral-selections` | feature flag 打开的 development/test | 读取一个 Strategy 快照并执行一次不持久化的新选择 |

既有探针保持不变：

| Method | Path | 语义 |
| --- | --- | --- |
| `GET` | `/health` | 进程 liveness，不依赖 Lottery Strategy 是否存在 |
| `GET` | `/ready` | MySQL readiness，不证明某个 Lottery Strategy 可用 |

本节没有发布 Strategy 创建/修改/查询 API，也没有正式 Draw、结果查询、库存或权益接口。

## 2. 发布状态与 feature gate

路由进程默认不注册：

```text
GROWTHOS_LOTTERY_EPHEMERAL_SELECTION_ENABLED=false
```

只有显式设置为 `true` 才注册；`staging` 和 `production` 环境若尝试打开，配置加载直接失败。仓库 Compose 以 `development` 显式打开，用于学习和验收。

这表示：

- `/api/v1` 是 URI 版本，不等于生产发布等级；
- 路由在本地存在，不代表具备公网安全边界；
- `X-GrowthOS-Demo-Mode` 不是认证；
- 正式部署不得通过复制 Compose 的 flag 绕过产品模型。

## 3. 请求 contract

### 3.1 Path

```text
/api/v1/lottery/strategies/{strategy_id}/ephemeral-selections
```

`strategy_id` 必须是完整 uint64 范围内的规范正十进制：

```text
1 <= strategy_id <= 18446744073709551615
```

合法：

```text
1
42
18446744073709551615
```

非法：

```text
0
01
+1
-1
1.0
1e3
 1
18446744073709551616
```

### 3.2 Required header

请求必须恰好携带一个：

```http
X-GrowthOS-Demo-Mode: ephemeral-selection
```

缺失、值错误或重复 header 都返回 `400 demo_mode_required`。

该 header 只表达：调用方理解“这是非持久化演示选择”。它不识别用户、租户或角色，也不授权访问 Strategy。

### 3.3 No request body

该 route 不接收 JSON、form、multipart 或其他 body。当前失败关闭边界是：

- `Content-Length` 非零或未知；
- 已通过 Nginx HTTP parser 并进入 API location 的非空 `Transfer-Encoding`，包括验收覆盖的空 chunked；Go 也拒绝它自己能观察到的 TransferEncoding；
- 非空 `Trailer` 声明在边缘拒绝，Go 也拒绝它自己能观察到的 Request.Trailer；
- 实际发送 `{}`、其他非空内容或文件。

显式 `Content-Length: 0` 与没有 body 的请求等价，当前允许；某些 `curl --data ''` 调用也会被编码成零长度而通过 framing gate。接口拒绝的是实际内容、非零/未知长度、可观察到的 Transfer-Encoding/Trailer，以及边缘仍能看到的非空声明；不能把“命令行出现了 `--data`”本身当作协议事实。

通过 Nginx HTTP parser 并进入 API location 的非空 Transfer-Encoding/Trailer 声明先返回 JSON 400，Go adapter 仍保留自己的 framing 防线。不受支持或语法非法的 Transfer-Encoding 可能在 location 前由 Nginx 原生返回 400/501，响应可能是 HTML。边缘请求体上限为 16 KiB，超过后返回 JSON 413；这不是“允许 16 KiB body”，只是统一控制不合规流量的资源占用。

### 3.4 No query

任何 query 都拒绝，包括：

```text
?seed=1
?award_id=2
?idempotency_key=x
?
```

调用方不能控制随机 seed、ticket、候选或隐藏选项。

### 3.5 Idempotency-Key is forbidden

只要 `Idempotency-Key` header 出现，即使为空，也返回：

```text
400 idempotency_not_supported
```

服务器没有持久结果可与 key 绑定；接受它会形成虚假幂等承诺。

### 3.6 Content negotiation

调用方建议发送：

```http
Accept: application/json
```

对非 `HEAD`、已被 Nginx parser 接受并进入当前 API location/Go handler 的响应，成功与既定 API error 固定使用 JSON，但尚未实现完整 Accept 协商或 406。server-level 421、parser 早期错误以及 HEAD wire body 是明确例外；不能据此推断支持任意 media type 版本。

## 4. 成功响应

### 4.1 reward

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: acceptance.client:42
```

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

### 4.2 no_reward

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

`no_reward` 与 reward 都是 `200`。Outcome 是当前封闭枚举：

| 值 | 含义 |
| --- | --- |
| `reward` | 算法选择到奖励候选；不表示库存或发放成功 |
| `no_reward` | 算法正常完成并选择到未中奖候选 |

### 4.3 字段 contract

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `selection` | object | 本次同步选择 |
| `selection.durability` | string literal | 当前固定 `ephemeral` |
| `selection.strategy_id` | decimal string | 请求对应的 StrategyID，完整 uint64 |
| `selection.award` | object | 选中的配置内 Award |
| `selection.award.id` | decimal string | Strategy 内 AwardID，完整 uint64 |
| `selection.award.name` | string | 展示名；来自已恢复并校验的快照 |
| `selection.award.outcome` | enum string | `reward` 或 `no_reward` |

ID 不得在 JavaScript/TypeScript 中转成 `number`。DTO 有意不返回 Strategy name、weight、total weight、ticket、概率、算法内部错误或不存在的 DrawID。

## 5. 错误 envelope

进入 Go handler 的非 HEAD error，以及 Nginx 为 API location 显式生成的 400/413/502/504，使用同一形状。server-level 421、HTTP parser 早期拒绝与 HEAD wire body 不在这个 envelope 承诺内：

```json
{
  "error": {
    "code": "invalid_strategy_id",
    "message": "strategy_id must be a canonical decimal integer from 1 through 18446744073709551615",
    "request_id": "01H..."
  }
}
```

稳定字段：

| 字段 | 用途 |
| --- | --- |
| `error.code` | 程序分支；低基数稳定值 |
| `error.message` | 安全公开说明，不含 cause |
| `error.request_id` | 与响应 header、Go/Nginx 日志关联 |

客户端不得按 `message` 文本分支，也不得把 request ID 当幂等键。

## 6. 状态码与 code

### 6.1 请求错误

| HTTP | code | 触发条件 |
| ---: | --- | --- |
| 400 | `invalid_strategy_id` | path ID 空、非规范、零、负数或溢出 |
| 400 | `demo_mode_required` | demo header 缺失、重复或不精确 |
| 400 | `query_parameters_not_allowed` | URL 带任何 query |
| 400 | `request_body_not_allowed` | 非零/未知 body framing，或被 API location/Go 观察到的 Transfer-Encoding/Trailer |
| 400 | `idempotency_not_supported` | 出现 Idempotency-Key |
| 404 | `route_not_found` | 尾斜杠或其他未注册路径 |
| 405 | `method_not_allowed` | 对精确 route 使用非 POST；响应 `Allow: POST`；HEAD 在真实 wire 上按协议没有 body |
| 413 | `request_too_large` | Nginx 判断请求体超过 16 KiB |
| 421 | 无稳定业务 code | 本地 Nginx 的 server-level 非 JSON Host 拒绝；仍带 `no-store` 与 Request ID |

对已经到达 Go handler 的请求，校验顺序是公开 contract 的一部分：先验证 path ID，再 idempotency、demo header、query、body framing；同一个请求同时有多个错误时只保证返回该顺序中的第一个。Host、可被边缘观察的 framing 和超过 16 KiB 的 body 可先被 Nginx 拒绝，不进入这条 Go 顺序。

### 6.2 业务与依赖错误

| HTTP | code | 内部类别 | 是否可认为形成了结果 |
| ---: | --- | --- | --- |
| 404 | `lottery_strategy_not_found` | `ErrStrategyNotFound` | 否 |
| 503 | `lottery_selection_unavailable` | cancel/deadline、repository retryable、random source failure | 否；不得当 no_reward |
| 500 | `internal_error` | stored invalid、普通 repository failure、composition/selector/result invariant | 否；程序/数据需诊断 |

500 不区分内部详细 cause，避免通过 API 枚举表、约束、driver、熵源或包结构。可信日志也只记录经过审核的 `error_class`，不渲染原始 cause。

### 6.3 网关错误

| HTTP | code | 含义 |
| ---: | --- | --- |
| 502 | `bad_gateway` | Nginx 无法获得 upstream 响应 |
| 504 | `gateway_timeout` | upstream 在 Nginx inactivity budget 内没有响应 |

Docker 网络端点缓存和连接状态可能让“API 已停止”表现为 502 或 504；两者都表示客户端没有得到可信 selection。它们使用同一个边缘 Request ID、JSON 和 `no-store`。

## 7. Header contract

经 Compose Nginx 入口访问时，成功与 JSON API error 均应有：

```http
Content-Type: application/json
Cache-Control: no-store
X-Request-ID: <safe-value>
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
```

### 7.1 Request ID

Nginx 接受的客户端 ID：

- 长度 1～64 字节；
- 字符仅限 `A-Za-z0-9_.:-`。

安全值会端到端复用；缺失或非法值由边缘替换。Go middleware 仍独立验证，防止绕过 Nginx 的直接调用。对于带 JSON error envelope 的响应，body 中的 request ID 必须等于单一 response header；成功 selection body 本来就没有 request ID。

直连 Go 端口的单元测试只承诺 Go 自己产生的 JSON、`no-store` 与 Request ID；`X-Content-Type-Options`、`X-Frame-Options` 和 `Referrer-Policy` 是 Compose Nginx edge contract，不应归因于 Gin handler。

### 7.2 no-store

每次调用可能产生新的随机结果，不应由 shared/private cache 重放；错误也可能包含与单次请求相关的 request ID。因此应用和 Nginx 自产错误都明确 `Cache-Control: no-store`。

`no-store` 是缓存指令，不是服务端删除日志或客户端内存的隐私保证。

## 8. Timeout contract

| 层 | 当前 Compose 值 | 目的 |
| --- | ---: | --- |
| Lottery application deadline | 3s | MySQL read + synchronous selection 的协作式预算 |
| MySQL driver read timeout | 5s | 网络读上限；比 application 多 1s 以上 |
| HTTP write timeout | 10s | 给 handler 完成 JSON 写入留响应预算 |
| Nginx proxy read timeout | 11s | upstream 相邻读取 inactivity timeout |

配置加载要求：

```text
selection + 1s <= mysql read timeout
selection + 1s <= HTTP write timeout
```

这不是硬实时证明：同步 selector 的接口没有 context。1000 Award 上限把不可取消的 O(n) 工作限制在明确规模；若未来随机源变成远程依赖，必须升级 port 让它接受 context。

## 9. 持久化与重复调用语义

该 route 执行：

```text
SELECT Strategy snapshot → select Award → serialize response
```

它不执行：

```text
INSERT DrawResult
UPDATE inventory
INSERT participation
publish event
deliver benefit
write Redis
```

因此：

- 业务表 fingerprint 在调用前后应完全一致；
- 响应丢失后没有结果查询；
- 客户端不能判断服务端是否曾经完成某个临时选择；
- 重试是新的 selection，可能返回不同 Award；
- request ID 只能定位一次 HTTP 处理，不能恢复业务事实；
- 不满足“一次抽奖只有一个最终结果”的长期不变量。

## 10. 安全边界

当前已做：

- 默认关闭和 production/staging 配置禁用；
- 本地 loopback 发布、Host allowlist；
- demo acknowledgment；
- 规范 path ID、禁止 query、精确 demo/Idempotency-Key 规则与失败关闭的 body framing；其他未使用 header 不是 allowlist，不能控制算法；
- 16 KiB 边缘 limit 和 1000 Award 数据上限；
- 两表 SELECT-only 运行账号；
- 最小 DTO、稳定错误、cause 脱敏；
- Request ID、no-store、超时；
- 隔离的真实 acceptance。

当前未做：

- TLS/public ingress；
- 用户认证、租户/对象级授权；
- Activity 发布态、时间窗、受众和资格；
- rate limit、并发配额、WAF 或防刷；
- CORS 策略；
- 审计日志、结果不可抵赖；
- Draw 幂等和查询；
- 库存/Benefit 一致性。

所以它只能用于受控 development/test，不应暴露到真实价值场景。

## 11. 示例请求

### 11.1 合法调用

```bash
curl --request POST \
  --header 'Accept: application/json' \
  --header 'X-GrowthOS-Demo-Mode: ephemeral-selection' \
  --header 'X-Request-ID: lesson21.demo:1' \
  http://127.0.0.1:8088/api/v1/lottery/strategies/1/ephemeral-selections
```

### 11.2 不支持的幂等声明

```bash
curl --request POST \
  --header 'X-GrowthOS-Demo-Mode: ephemeral-selection' \
  --header 'Idempotency-Key: retry-1' \
  http://127.0.0.1:8088/api/v1/lottery/strategies/1/ephemeral-selections
```

预期 `400 idempotency_not_supported`。

### 11.3 非规范 ID

```bash
curl --request POST \
  --header 'X-GrowthOS-Demo-Mode: ephemeral-selection' \
  http://127.0.0.1:8088/api/v1/lottery/strategies/01/ephemeral-selections
```

预期 `400 invalid_strategy_id`。

## 12. 验证入口

单元/竞态：

```bash
go test ./internal/lottery/application ./internal/lottery/adapter/httpapi
go test -race ./internal/lottery/application ./internal/lottery/adapter/httpapi
```

长期栈只读 smoke：

```bash
make compose-up
make compose-smoke
```

一次性真实 success/failure acceptance：

```bash
make compose-lottery-api-acceptance
```

## 13. 未来兼容性

正式 Lottery API 不应简单删除 `ephemeral` 单词后原样发布。至少需要重新设计：

1. 公共 Activity/Campaign identity，而不是直接暴露内部 StrategyID；
2. published/version/time/audience/authorization；
3. Participation 与用户次数；
4. 客户端请求身份、唯一约束、DrawID、结果表与查询；
5. Strategy/Award/算法快照；
6. inventory 与 Benefit 发放状态；
7. rate limit、abuse prevention 与容量预算；
8. outcome unknown、恢复、补偿和审计；
9. API schema/version migration；
10. 前端对“已选中”“已落库”“已发放”三种状态的区别。

正式端点很可能与本 route 并存一段时间，而不是把 ephemeral response 当作最终 Draw DTO 向后兼容。

## 14. 依据与关联资料

- [第 21 节课程](../../course/part-03/lesson-21-lottery-api.md)
- [第 21 节 QA](../../qa/lessons/lesson-21.md)
- [ADR-0018](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)
- [HTTP adapter](../../../internal/lottery/adapter/httpapi/selection.go)
- [application use case](../../../internal/lottery/application/ephemeral_selection.go)
- [Nginx 配置](../../../deploy/docker/nginx.conf)
- [RFC 9110：POST、方法与状态码](https://www.rfc-editor.org/rfc/rfc9110.html)
- [RFC 9111：no-store](https://www.rfc-editor.org/rfc/rfc9111.html#section-5.2.2.5)
- [RFC 8259：JSON](https://www.rfc-editor.org/rfc/rfc8259.html)
- [Go strconv.ParseUint](https://pkg.go.dev/strconv#ParseUint)
- [OWASP Input Validation](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)
