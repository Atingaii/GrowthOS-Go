# 第 19 节 API 记录：Repository 已实现，但 HTTP 契约没有变化

- **章节：** [实现 Lottery Repository](../../course/part-03/lesson-19-lottery-repository.md)
- **日期：** 2026-08-29
- **状态：** Lottery `Create` / `FindByID` 持久化端口和 MySQL adapter 已实现；没有新增、修改或删除 HTTP API
- **QA：** [第 19 节 QA](../../qa/lessons/lesson-19.md)
- **ADR：** [ADR-0016](../../decisions/ADR-0016-lottery-repository-boundaries.md)

## 1. 本节 API 结论

第 19 节新增的是进程内部 Go contract 与数据库 adapter，不是网络 API。现有 HTTP surface 保持不变：

| 类型 | 路径 / 契约 | 第 19 节状态 |
| --- | --- | --- |
| 进程 liveness | `GET /health` | 保持既有成功 body，不变 |
| MySQL readiness | `GET /ready` | 继续做有界 Ping，不调用 Lottery Repository |
| 统一错误 envelope | 404/405/500/503 与 request ID | 保持既有契约，不变 |
| Nginx 同源 `/api` 转发 | 基础设施命名空间 | 保持不变，仍无 Lottery handler |
| 创建 Strategy | 未定义 | 没有 HTTP method/path/body/auth/idempotency |
| 按 ID 查询 Strategy | 未定义 | `FindByID` 只是 Go port，不是公开 route |
| 执行 Draw | 未定义 | 第 20 节算法尚未实现 |
| 更新/删除 Strategy | 未定义 | Repository 本身也没有 Update/Delete/Upsert |
| React `/lottery` | Mock | 没有调用本节 Repository |

因此下面这些示例路径当前都不是已承诺接口：

```http
GET /api/lottery/strategies/42 HTTP/1.1
Host: 127.0.0.1
Accept: application/json
```

```http
POST /api/lottery/strategies HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
```

```http
POST /api/lottery/draw HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
```

它们当前只能命中既有未知路由处理，不能返回 Strategy 或抽奖结果。示例不能被客户端当作未来 path、method 或版本格式的承诺。

## 2. 新增的是哪一种“API”

本节新增两个进程内 application port：

```go
type StrategyCreator interface {
    Create(ctx context.Context, strategy domain.Strategy) error
}

type StrategyReader interface {
    FindByID(ctx context.Context, id domain.StrategyID) (domain.Strategy, error)
}
```

它们是 Go package 之间的协议，和 HTTP API 有四个关键差异：

| 维度 | Repository port | HTTP API |
| --- | --- | --- |
| 调用范围 | 同一 Go 进程 | 跨进程/网络客户端 |
| 输入输出 | `domain.Strategy`、typed ID、Go error | bytes、method/path/header、DTO、status |
| 兼容边界 | 仓库内编译依赖 | 需要面向独立客户端版本兼容 |
| 错误 | `errors.Is` 语义类 + 内部 cause | 稳定公开 code/message/status，不能暴露 cause |

MySQL adapter 对这两个端口的实现已经通过真实数据库验证，但当前 `growth-api` 没有 Lottery use case/handler 装配。测试代码能直接调用 `repository.Create`，不代表浏览器或第三方客户端可以访问它。

## 3. Repository error 不是 HTTP error contract

应用层目前定义：

- `ErrRepositoryInvalidArgument`；
- `ErrRepositoryNotConfigured`；
- `ErrStrategyNotFound`；
- `ErrStrategyAlreadyExists`；
- `ErrStoredStrategyInvalid`；
- `ErrRepositoryRetryable`；
- `ErrCommitOutcomeUnknown`；
- `ErrRepositoryFailure`。

这些类让 Go use case 能通过 `errors.Is` 做稳定判断，同时不渲染 SQL、表名或驱动 cause。它们**没有**自动决定 HTTP status：

| Repository 语义 | 第 19 节已经确定 | HTTP 层仍需决定 |
| --- | --- | --- |
| Strategy not found | 数据库没有该聚合根 | 查询接口是 404、业务空结果还是其他 code |
| Strategy already exists | create-only 身份冲突 | 是否 409；请求 payload 相同也不能自动视为幂等成功 |
| Stored strategy invalid | 持久化快照损坏，禁止返回部分对象 | 公开 500/503 如何区分、是否熔断/告警 |
| Retryable | 当前识别到 1205/1213 | 服务端是否重试、何时返回 503、是否给 `Retry-After` |
| Commit outcome unknown | 写可能已提交也可能未提交 | 客户端如何查询结果、幂等 key 和恢复协议 |
| Repository invalid argument | nil context 等 adapter 调用契约错误 | 这是服务端编程错误还是可归因于请求，需要由 use case 边界判断 |
| Domain validation error | Strategy/Award ID、名称、Weight、Outcome 或聚合不变量不合法 | 400/422 的划分与字段级 error format |
| Repository failure | 未分类依赖/权限/schema/scan 故障 | 500/503 边界和日志/告警策略 |

尤其不能把 MySQL error number、driver message、SQL 文本、约束名或 wrapped cause 直接放进 JSON response。HTTP handler 必须在拥有请求语义时做一次显式翻译，并让内部诊断与公开消息分离。

## 4. Commit outcome unknown 为什么会影响未来 API

`Create` 的 Commit 可能在服务端已经持久化后丢失响应。第 19 节 Repository 正确返回 `ErrCommitOutcomeUnknown`，但它无法单独给远程客户端一个完整恢复协议。

未来创建 API 必须先回答：

1. 客户端是否提供幂等键；
2. 幂等键的作用域、过期时间和 payload 一致性如何验证；
3. StrategyID 是否由客户端生成，能否作为查询依据；
4. 遇到 outcome unknown 时返回什么公开 code/status；
5. 客户端随后查询哪个资源或 operation；
6. AlreadyExists 如何区分“同一次重试”与“不同请求抢占同 ID”；
7. 如果后续还有审计/消息副作用，如何与数据库事实对账。

在这些问题未解决前，不应为了“先有个 POST”而直接把 Repository 暴露出去。

## 5. 当前没有请求 DTO

虽然 `Create` 已接受完整 `domain.Strategy`，HTTP 创建 body 仍没有定义。至少还缺少：

- StrategyID 是客户端字符串、数字还是服务端生成；
- AwardID 是 Strategy 内局部 ID 还是由服务端分配；
- 完整 `uint64` 如何在 JSON 中无损表达；
- Weight 是整数相对权重、百分数还是基点；
- Awards 数组是否有数量/请求体大小限制；
- 名称的 Unicode、128 rune 与字段级校验错误如何返回；
- `reward` / `no_reward` 是否直接成为外部 enum；
- 客户端能否设置/覆盖已有 Strategy；
- 认证主体、运营角色、审批和审计；
- 幂等 key 与 request ID 的关系。

不能把表列或 domain struct 直接 JSON encode 后称为稳定 API。领域对象内部的 `uint64` 完整范围超过 JavaScript safe integer，未导出字段和方法也不是 transport schema。

## 6. 当前没有响应 DTO

`FindByID` 返回完整 `domain.Strategy`，但 HTTP 查询响应仍需决定：

- ID/Weight 是否使用十进制字符串；
- 是否公开原始 Weight，或只提供运营管理视图；
- Awards 是否总按 AwardID 排序，是否还需要展示顺序；
- 是否公开 `TotalWeight` 或计算概率；
- 数据库 `created_at` / `updated_at` 是否属于产品契约；
- 是否需要 ETag/version 来支撑后续编辑；
- `reward` 是否只表示候选类型，如何避免被误解为已发放；
- 管理端 DTO 与用户抽奖 DTO 是否应分开。

本节 Repository 不读取表中的时间戳，因为当前领域聚合并没有这些业务属性。HTTP 层不能因为列存在就擅自公开它们。

## 7. `FindByID` 的一致性不等于公开缓存语义

本节 `FindByID` 在一个 read-only Repeatable Read 事务内读取 root 与 Awards，保证一次调用不会拼接两个提交点。这个事实可以成为未来查询/Draw use case 的基础，但没有承诺：

- 两个独立 HTTP 请求读到同一版本；
- create 返回后读副本立刻可见；
- 客户端能用 ETag 做条件请求；
- Strategy 会被缓存多久；
- Redis 与 MySQL 的可见性一致；
- 进行中的 Activity 固定使用哪个 Strategy 版本。

本节没有 Redis Repository、cache-aside 或 HTTP cache header。未来出现缓存时必须单独定义版本和失效协议。

## 8. 数据库权限变化不是 HTTP 权限变化

为了执行已经实现的 SQL，`growthos_app` 现在恰好拥有：

```text
SELECT, INSERT ON growthos.lottery_strategy
SELECT, INSERT ON growthos.lottery_strategy_award
```

它没有 UPDATE、DELETE、DDL，也不能访问 `schema_migrations`。这是数据库身份的最小权限变化，不代表匿名/登录用户获得“创建 Strategy”权限。

未来 HTTP authorization 仍需独立回答：

- 谁能创建和查看 Strategy；
- 普通抽奖用户是否应该看到 Weight；
- 运营动作是否需要租户/组织隔离；
- 是否需要审批、审计、风控和频率限制；
- 内部管理 API 与公开 Draw API 是否使用不同身份。

数据库 GRANT 是纵深防御的一层，不能替代业务鉴权；HTTP 鉴权也不能成为给数据库账号 root 权限的理由。

## 9. `/ready` 契约没有变化

`GET /ready` 继续只做现有 MySQL pool 的有界 Ping。它没有调用 `FindByID`，也不检查：

- 某个 Strategy 是否存在；
- 所有 Strategy 能否通过 Restore；
- 应用账号 INSERT 是否可用；
- 当前事务隔离是否正确；
- Repository 查询 P99；
- Redis、抽奖算法或 Benefit 是否可用。

Compose 启动时的 Migration/grants gate 和集成测试证明 schema/权限兼容；readiness 证明运行时依赖当前可达。把完整业务写读或全表领域重建放进高频探针会引入副作用、负载和不稳定性，因此本节不改变 `/ready`。

## 10. React 与真实调用状态

`/lottery` 仍是 Mock 页面，没有：

- 获取真实 Strategy；
- 提交 Strategy 创建请求；
- 发起 Draw；
- 处理 not found/already exists/outcome unknown；
- 以字符串安全处理完整 `uint64`；
- 根据服务端选择展示 reward/no_reward；
- 缓存、重试或恢复不确定写结果。

系统状态页继续只调用 `/health` 与 `/ready`。Repository 的实现不构成前端数据源切换。

## 11. HTTP 负向验收

本节应证明“内部能力增长没有让外部契约意外漂移”：

```bash
go test ./internal/infrastructure/httpapi
make compose-up
make compose-smoke
```

验收重点：

- `/health`、`/ready` 成功契约不变；
- 404/405/500/503 envelope 与 request ID 不变；
- 未知 `/api/**` 仍返回 `route_not_found`；
- 没有 Lottery route 被意外挂载；
- Compose grants 已升级为两表 SELECT + INSERT，且负向权限仍成立。

这些检查不能被描述成“Lottery API 测试通过”，因为本节有意没有 Lottery API。

## 12. 第 21 节设计第一个 Lottery API 前的输入

第 19 节现在能够给 transport 层以下确定事实：

1. Strategy 必须作为完整聚合创建；
2. Create 是 create-only，不是 Save/Upsert，也不是自动幂等；
3. 根 ID duplicate 可稳定识别；
4. 写 Commit 可能结果未知；
5. FindByID 区分 not found、stored-invalid 和 dependency failure；
6. FindByID 返回一个父子一致快照；
7. 无效持久化快照不会被部分返回；
8. ID/Weight 支持完整 `uint64`；
9. AwardID 只在 Strategy 内唯一；
10. pool/cause/SQL 都不应穿透到 transport。

第 20 节还需补齐：

- 加权选择算法；
- 可注入随机源；
- reward/no_reward 的成功结果表示；
- 边界值和确定性验证。

第 21 节随后仍需决定：

- 第一个真实 use case 是读取配置、创建配置还是执行 Draw；
- URL、method、认证、授权和限流；
- JSON 中 `uint64` 编码；
- DTO 与 domain mapping；
- timeout、取消、重试与幂等；
- error code/status/message；
- request ID、日志、指标和审计；
- 前端从 Mock 切换的兼容计划。

## 13. 本节不应被外推的能力

为了保持课程和简历陈述准确，本节明确不能写成：

- “完成 Lottery REST API”；
- “支持在线创建/查询策略”；
- “实现高并发抽奖”；
- “接入 Redis 缓存”；
- “实现策略动态更新”；
- “通过幂等保证 exactly-once”；
- “实现奖品发放”；
- “前端已接入真实抽奖服务”。

可以准确表述为：实现 Lottery Strategy Repository 端口和 MySQL adapter，完成父子事务写入、Repeatable Read 一致快照、领域重建、错误分类、取消传播与最小数据库权限验证。

## 14. 下一节

第 20 节实现持久化无关的加权选择算法；它消费已经恢复成功的 `domain.Strategy`。第 21 节才把 Reader、算法与 HTTP use case/handler 组合起来，并正式记录第一个 Lottery 网络契约。
