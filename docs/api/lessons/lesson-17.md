# 第 17 节 API 记录：Lottery 领域对象但无业务接口

- **章节：** [最简单随机抽奖需要什么对象](../../course/part-03/lesson-17-lottery-domain-objects.md)
- **日期：** 2026-08-29
- **状态：** 领域对象已实现；没有新增或修改 HTTP API
- **实现提交：** `0b59217 feat: model lottery strategy domain objects`
- **QA：** [第 17 节 QA 验收证据](../../qa/lessons/lesson-17.md)
- **ADR：** [ADR-0013](../../decisions/ADR-0013-lottery-domain-model.md)

## 1. 本节 API 结论

本节没有新增、修改或删除 HTTP 路由，也没有让前端调用 Lottery 后端。它只在 `internal/lottery/domain` 建立 Strategy/Award 领域契约。

| 类型 | 路径 / 契约 | 当前状态 |
| --- | --- | --- |
| 系统 liveness | `GET /health` | 保持第 11～16 节契约，不变 |
| MySQL readiness | `GET /ready` | 保持第 13～16 节契约，不变 |
| 统一错误 | 404 / 405 / 500 与 readiness 503 envelope | 不变 |
| 预留命名空间 | `/api` | Nginx/Vite 仍可转发，但当前没有 Lottery route |
| Lottery 策略 API | 未定义 | 第 21 节前不得伪造 |
| Lottery 抽奖 API | 未定义 | 第 20 节算法、第 21 节接口后实现 |
| React `/lottery` 数据源 | 集中 Mock / 客户端演示 | 未接入本节 Go 领域对象 |

因此，下面这样的请求当前仍应命中统一 404，而不是返回业务结果：

```http
POST /api/lottery/draw HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
```

本节不把某个未来路径登记为已承诺接口；示例只说明不存在业务路由。第 21 节必须根据当时的用例重新确定 path、method、版本、DTO、认证、幂等和错误语义。

## 2. 为什么领域对象不直接成为 JSON 契约

`Strategy` 和 `Award` 字段有意保持未导出，也没有 `json` tag：

```text
HTTP request/response DTO（未来 adapter）
                 │
                 │ 校验、映射、构造
                 ▼
       internal/lottery/domain
```

不能使用 `json.Marshal(strategy)` 的结果作为未来 API 设计依据。Go 内部对象和网络契约变化原因不同：

| 关注点 | 领域对象 | HTTP DTO |
| --- | --- | --- |
| 目标 | 保持业务不变量 | 与客户端版本兼容、表达一次用例 |
| 字段可见性 | 私有状态 + getter | 明确公开字段 |
| 错误 | 领域哨兵错误 | HTTP status + 稳定公开 code/message |
| 名称 | 已规范化领域值 | 输入需校验，输出需编码 |
| 权重 | 正 `uint64` 相对值 | 需要明确 JSON 数值/字符串范围与 JS 精度 |
| Outcome | Go 封闭常量 | 需要版本化的公开 enum |
| 候选顺序 | AwardID 规范顺序 | 不自动等于 UI 展示顺序 |

特别是 JavaScript `number` 不能精确表示全部 `uint64`。当前 Go 领域契约允许权重和总和达到 `math.MaxUint64`，所以第 21 节不能未经决策把这些值当作普通 JSON number 发给浏览器。可选方案包括收窄公开范围、使用十进制字符串或引入更明确的 API 数值契约；本节不提前决定。

## 3. 本节没有请求 DTO

当前不存在以下任何已接受契约：

- 创建或编辑 Strategy 的 request；
- Award 的 JSON shape；
- `weight` 是 number、string、百分数还是万分比；
- `outcome` 的外部 enum 版本；
- `strategy_id` / `award_id` 的 URL 或 body 表示；
- 发起一次 Draw 的用户、活动、请求 ID 或幂等 key；
- 抽奖成功、未中奖、结果未知和系统失败的 response；
- 分页、列表、管理后台或删除接口；
- 认证、授权、审批、限流和审计字段。

代码中的 `NewStrategy` / `NewAward` 是 Go 领域构造器，不是 API handler，也不是 DTO 校验器。

## 4. 本节没有响应 DTO

以下概念目前也不能出现在真实 HTTP 成功响应中：

```json
{
  "strategy_id": "future-contract",
  "award_id": "future-contract",
  "outcome": "reward",
  "delivered": true
}
```

原因分别是：

- 还没有 Repository，无法从权威存储加载 Strategy；
- 还没有抽奖算法，无法产生可信 Award 选择；
- 还没有 Draw/Result 持久化，无法满足一次请求只有一个最终结果；
- `reward` 只表示候选类别，不表示 Benefit 已经发放；
- 还没有用户资格、次数、活动或库存语义；
- 还没有定义外部 ID 编码与 JSON 精度。

因此第 17 节没有 OpenAPI 变化，也不创建一个只返回硬编码或 Mock Award 的后端接口。

## 5. 错误映射仍未定义

领域包已经有稳定校验类别：

- Strategy ID/名称/候选缺失；
- Award ID/名称/权重/Outcome 非法；
- Strategy 内 AwardID 重复；
- 总权重溢出。

这些错误当前只供 Go 代码和单元测试使用。它们不直接决定：

- HTTP 400、409、422 还是其他 status；
- 对外 `error.code`；
- 是否向终端用户、运营后台公开同样信息；
- 是否记录审计或允许重试。

未来 handler 必须在 transport adapter 中映射领域错误，不能把 `err.Error()` 原样返回，也不能把 Go 变量名当成长期客户端契约。

## 6. `no_reward` 不是 error envelope

`AwardOutcomeNoReward` 表示未来算法可以合法选择一个不产生奖励的 Award。它与统一 HTTP error envelope 完全不同：

| 情况 | 业务执行含义 | 未来 HTTP 方向（尚未锁定） |
| --- | --- | --- |
| 选中 `reward` | 选择成功，有后续奖励处理 | 成功结果 |
| 选中 `no_reward` | 选择成功，无奖励 | 仍是成功结果，不应返回 5xx |
| Strategy 不合法 | 配置无法构造 | 管理/内部错误，需要明确映射 |
| 依赖不可用 | 无法确定抽奖结果 | 不能伪装为 no_reward |
| 结果未知 | 请求可能已执行但无法确认 | 需要结果查询/幂等语义 |

若把 `no_reward` 返回为 error，通用重试器、用户刷新或代理重试可能改变一次业务结果。第 21 节设计 API 时必须保持这一区分。

## 7. 当前前端边界

`/lottery` 页面当前仍读取 `web/src/mocks/growthOsMockData.ts` 等 Mock 数据，并在浏览器中做演示性随机选择。它没有：

- 调用 Go Lottery API；
- 使用本节 Strategy/Award；
- 从 MySQL 加载配置；
- 保存一次抽奖结果；
- 消耗参与次数；
- 触发 Benefit 发放；
- 使用 Redis 缓存。

页面上的奖品、概率文案、锁或后端描述都不能反向证明服务端能力已经存在。第 22 节接入真实页面时，需要把对应 Mock 明确替换或隔离，并通过浏览器—API—领域—数据库整条链路验收。

## 8. 当前数据库与缓存边界

本节未改变 API 的 readiness 依赖：

- `/ready` 仍只对 MySQL 执行有界 Ping；
- `migrations/sql` 仍没有业务 Migration；
- MySQL 中没有 Strategy/Award 权威表；
- Compose Redis 不在 API readiness 中，也没有 Lottery key；
- 不存在“先查 Redis、miss 再查 MySQL”的调用路径。

纯领域单元测试不需要启动 MySQL、Redis、Docker 或浏览器。不能因为 Compose 中已经有这些容器，就把它们写进本节业务 API 的依赖图。

## 9. 对第 21 节 API 设计的输入

本节只给未来 API 提供以下领域事实：

1. Strategy 和 Award ID 必须为正；
2. Strategy 至少一个合法 Award；
3. AwardID 在 Strategy 内唯一，名称不承担身份；
4. 名称经过 UTF-8、trim、控制字符与 128 rune 校验；
5. Weight 是正 `uint64` 相对权重，不是百分比；
6. Outcome 当前只有 `reward` / `no_reward`；
7. AwardID 顺序是领域规范迭代顺序，不是 UI 展示顺序；
8. `reward` 不等于权益已发放；
9. `no_reward` 是合法结果候选，不是错误；
10. 领域错误类别可以被 adapter 稳定识别。

第 21 节还必须基于第 18～20 节的新事实补齐：

- Strategy 从哪里加载、找不到如何表达；
- 随机源与算法失败如何分类；
- 一次 Draw 的输入、身份与结果是什么；
- 是否已经具备幂等和结果查询；
- 认证用户、活动/参与关系和权限边界；
- HTTP timeout、取消与服务端继续执行的关系；
- `uint64` 在 JSON/JavaScript 中的安全表示；
- 对 reward/no_reward 的响应是否需要共同 schema；
- `X-Request-ID` 与未来业务 request ID 如何区分。

## 10. 验证方式

本节 API 台账的核心负向断言是：没有新增 Lottery route，已有系统契约不漂移。对应检查包括：

```bash
go test ./internal/lottery/domain
go test ./internal/infrastructure/httpapi
make verify
```

完整命令和结果见[第 17 节 QA](../../qa/lessons/lesson-17.md)。这些检查不能证明未来 Lottery API 可用，因为该 API 当前不存在。

## 11. 下一节

第 18 节创建第一组业务表和 Migration，但仍不需要新增前端 API。只有数据库结构、Repository 和概率算法分别完成后，第 21 节才开放第一个 Lottery API。

这种顺序防止使用硬编码 JSON 或 Mock handler 掩盖“没有权威配置、没有算法、没有最终结果”的事实。
