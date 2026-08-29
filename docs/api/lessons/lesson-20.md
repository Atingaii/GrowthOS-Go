# 第 20 节 API 记录：加权选择已实现，但没有新增 HTTP API

## 1. 结论

第 20 节没有新增、修改或删除任何 HTTP API。

实现提交 `db679cf` 只增加 Lottery 进程内领域能力：`domain.WeightedSelector` 接收一个已经合法的 `domain.Strategy`，通过 domain-owned `BoundedRandomSource` 选择一个 `domain.Award`；生产随机源 adapter 使用 `crypto/rand.Int`。这些 Go 类型不是网络 DTO，也没有装配进 `cmd/growth-api` 或 Gin router。

当前真实 HTTP 契约仍只有既有系统探针：

| Method | Path | 第 20 节变化 |
| --- | --- | --- |
| `GET` | `/health` | 无 |
| `GET` | `/ready` | 无 |

当前不存在以下路由：

- `/api/lottery/**`；
- `/api/strategies/**`；
- `/api/draws/**`；
- `/api/awards/**`。

## 2. 为什么本节不直接开放网络接口

一个内部选择函数只需要回答“给定这个 Strategy，本次返回哪个 Award”。网络 API 还必须回答完全不同的问题：

- StrategyID、AwardID 和 Weight 怎样无损编码 `uint64`；
- 谁可查询 Strategy、谁可触发一次选择；
- 请求是否有用户、活动、参与和幂等身份；
- timeout、取消和客户端重试会不会导致重新选择；
- 内部错误映射成什么 HTTP status、稳定 code 和公开 message；
- `no_reward` 怎样与 5xx、超时和结果未知区分；
- 是否需要持久化最终结果，响应丢失后怎样查询；
- request ID、日志、指标和审计如何避免泄露随机材料。

在这些问题没有决策前，把 selector 直接塞进 handler 会让一个看似简单的 endpoint 暗中承诺错误的最终结果与重试语义。第 20 节因此只固定内部算法 contract，第 21 节再设计首个 Lottery API。

## 3. 当前内部调用 contract

以下内容只用于 Go 进程内部，不是客户端协议。

### 3.1 随机源端口

```go
type BoundedRandomSource interface {
    Uint64N(upper uint64) (uint64, error)
}
```

契约要求 source 均匀返回 `[0,upper)`，拒绝零上界，并在失败时返回 error。若多个 goroutine 共享 source，source 自身必须并发安全。

### 3.2 选择器构造

```go
func NewWeightedSelector(source BoundedRandomSource) (*WeightedSelector, error)
```

nil 和 typed-nil source 返回 `ErrSelectorNotConfigured`。默认生产组合可使用：

```go
selector, err := domain.NewWeightedSelector(randomsource.NewCryptoSource())
```

第 20 节没有在产品 composition root 中实际执行这段装配。

### 3.3 选择

```go
func (s *WeightedSelector) Select(strategy Strategy) (Award, error)
```

行为边界：

- 单候选 Strategy 直接返回唯一 Award，不调用随机源；
- 多候选只请求一次 `[0,totalWeight)` 票据；
- 按 AwardID 规范顺序使用减法桶；
- 支持 `TotalWeight == math.MaxUint64`；
- 选择到 `no_reward` 时返回完整 Award 和 nil error；
- 任一技术/contract/不变量失败都返回零值 Award 和 error；
- 方法不读数据库、不持久化结果、不扣库存、不发权益。

## 4. 内部错误不是 HTTP 错误码

### 4.1 Selector 语义类

| 内部错误 | 当前含义 | HTTP 映射状态 |
| --- | --- | --- |
| `ErrSelectorNotConfigured` | composition 缺少随机源 | 未定义 |
| `ErrSelectionStrategyInvalid` | 传入零值/不可用 Strategy | 未定义 |
| `ErrRandomSourceFailure` | 熵源失败 | 未定义 |
| `ErrRandomSourceContractViolation` | source 返回 `[0,upper)` 外的值 | 未定义 |
| `ErrSelectionInvariantViolation` | 合法票据无法映射到声明桶 | 未定义 |

### 4.2 CryptoSource 语义类

| 内部错误 | 当前含义 | HTTP 映射状态 |
| --- | --- | --- |
| `ErrSourceNotConfigured` | adapter 没有 Reader | 未定义 |
| `ErrUpperBoundRequired` | 直接请求 `[0,0)` | 未定义 |
| `ErrEntropyUnavailable` | `crypto/rand.Int`/Reader 失败 | 未定义 |

两层错误对象的公开 `Error()` 都只渲染稳定分类，`Unwrap` 为可信诊断保留 cause。未来 transport 不能把 cause、随机字节、ticket、包路径或内部 Strategy 数据直接放进 JSON。

`ErrRandomSourceFailure` 也不能映射成“未中奖”。`no_reward` 是一次成功选择出的 Award，error 为 nil；系统故障则没有可信 Award。

## 5. 当前没有请求与响应 DTO

第 20 节没有决定：

- StrategyID、AwardID 使用 JSON number 还是十进制 string；
- Weight 和 TotalWeight 是否对客户端暴露；
- Award Outcome 的公开枚举与向后兼容策略；
- 是否返回 Strategy version 或算法 version；
- 是否返回 DrawID、request ID、审计 ID 或结果查询地址；
- reward Award 是否携带 Benefit 引用；
- `no_reward` 使用普通 2xx body 还是其他表达。

特别是 JavaScript `number` 无法精确表示全部 `uint64`。内部算法支持 `math.MaxUint64` 不等于前端可以直接把 ID/Weight 当 number；第 21 节必须单独决定序列化契约。

## 6. 前端状态没有变化

现有 `/lottery` 页面仍使用前端 Mock 和 `Math.random()`，没有调用本节 Go 选择器。第 20 节没有修改其数据源、loading/error 状态或展示逻辑。

因此不能用浏览器页面证明：

- `CryptoSource` 被产品进程使用；
- 后端按数据库 Strategy 权重选择；
- `no_reward` 与系统失败正确展示；
- Redis 锁、防刷、库存或权益链路存在。

真实 Lottery 页面属于第 22 节。第 20 节 API 记录保留“无 HTTP 变化”，正是为了防止把内部 Go 能力误写成前端已经联调。

## 7. 第 21 节开放 API 前的停止条件

### 7.1 最危险的重试场景

若第 21 节直接为多候选 Strategy 实现一个无状态 `POST /draw`：

```text
请求 A -> 已选择 Award X -> 响应丢失
请求 A 重试 -> 再次调用 CryptoSource -> 可能选择 Award Y
```

当前没有 DrawID、request identity、结果唯一约束或结果查询，服务无法证明 X/Y 哪个是最终事实。这不满足 `INV-03`，也不能靠 HTTP 幂等方法名、Redis 短锁或客户端禁用按钮修复。单候选不会重新取随机数，但同样没有可查询的最终 Draw 事实，因此也不能宣称请求幂等。

因此首个 API 必须明确选择一种边界：

1. 先提供 Strategy 查询，不开放形成最终抽奖事实的 endpoint；
2. 只提供有明确非幂等/非持久化边界的内部演示选择；
3. 若面向真实价值场景，则先加入 Draw/request identity、原子结果持久化、重复请求返回同一结果和 outcome-unknown 恢复。

### 7.2 其他必须决策

- composition root 谁创建并共享 MySQL pool、Repository、CryptoSource 和 Selector；
- Repository failure、not found、stored invalid 与 selection failure 分别怎样映射；
- request deadline 是否能覆盖全部依赖；当前 `BoundedRandomSource` 没有 context；
- 鉴权、业务授权、限流和防刷边界；
- `uint64` 的 JSON 精度；
- 低基数 metrics 怎样区分 reward、no_reward 和技术失败；
- 禁止记录随机字节/ticket，同时保留足够故障诊断；
- Strategy 快照和算法版本是否需要进入结果记录。

## 8. 本节 API 验证边界

可以通过源码确认本节没有新增 router/handler/DTO：

```bash
rg -n "WeightedSelector|NewCryptoSource|/api/(lottery|draw|strateg)" \
  cmd internal/infrastructure/httpapi web/src
```

领域与 adapter 测试可以验证内部 contract，但不能充当 HTTP 契约测试：

```bash
go test ./internal/lottery/domain ./internal/lottery/adapter/randomsource
```

实际测试执行结果由本节 QA 记录；本文件只定义网络边界和未来决策，不把测试源码存在写成已运行事实。

## 9. 能准确表述与不能表述

可以表述：

> 第 20 节实现了持久化和传输无关的无偏加权 Award 选择，并通过可注入的密码学随机源保持完整 uint64、错误和并发边界；本节没有新增 HTTP API。

不能表述：

- 已开放 Lottery REST API；
- 已完成在线抽奖或抽奖幂等；
- 前端已经调用真实后端；
- 结果已经持久化；
- Redis 已用于防刷或锁；
- reward 已扣库存或发放；
- 业务 QPS/P99 或公平合规已验证。

## 10. 关联资料

- [第 20 节课程](../../course/part-03/lesson-20-lottery-weighted-selection.md)
- [ADR-0017：Lottery 无偏加权选择与随机源边界](../../decisions/ADR-0017-lottery-weighted-selection.md)
- [第 19 节 API 记录](lesson-19.md)
- [领域选择器](../../../internal/lottery/domain/weighted_selector.go)
- [密码学随机源 adapter](../../../internal/lottery/adapter/randomsource/crypto.go)
