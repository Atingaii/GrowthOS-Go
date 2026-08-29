# 第 20 节：实现最简单概率抽奖——无偏加权 Award 选择

> 本节对应实现提交 `db679cf`。这里的“概率抽奖”准确含义是：给定一个已经合法的 `domain.Strategy`，在内存中按相对权重选择且只选择一个 `Award`。本节没有创建一次 Draw 的身份，没有持久化最终结果，也没有 HTTP、库存、权益发放或 Redis 业务链路。

## 1. 为什么仓储完成后还不能直接开放抽奖接口

第 17～19 节已经依次回答了三个前置问题：

1. 什么样的 Strategy/Award 配置是合法的；
2. 合法配置怎样无损保存到两张 MySQL 表；
3. 怎样以一个一致快照恢复完整 Strategy 聚合。

但“能加载一组候选”不等于“会按候选权重选择”。如果此时直接在 handler 中写一段 `rand.Intn` 和循环，会把至少五类长期契约藏进传输层：

- 随机数究竟来自 `[0,total)` 还是 `[1,total]`；
- 最大 `uint64` 权重怎样避免转换或加一溢出；
- 随机源是否均匀、是否可预测、失败后怎样处理；
- 同一个随机值按什么稳定顺序映射到 Award；
- `no_reward` 与技术失败是否仍然可区分。

第 20 节因此只关闭“随机票据生成 + 加权区间映射”这一条缝。它先把选择行为做成独立、可测的领域能力，再把 Repository、用例和 HTTP 装配留给第 21 节。

## 2. 本节交付与明确非目标

### 2.1 已实现

- domain-owned 的 `BoundedRandomSource` 最小接口；
- `WeightedSelector` 领域服务及显式构造器；
- 多候选 Strategy 的无偏半开区间选择；
- 单候选 Strategy 的确定性短路；
- 完整 `uint64` 总权重，包括 `math.MaxUint64`；
- 基于 AwardID 规范顺序的减法桶映射；
- `reward` / `no_reward` 同一成功结果路径；
- selector 配置、Strategy、随机源、adapter contract 和内部映射的稳定错误边界；
- 基于 `crypto/rand.Int` 的生产随机源 adapter；
- 确定性边界、完整小区间枚举、失败、并发和微基准测试。

### 2.2 本节没有实现

- 没有从 MySQL 自动加载 Strategy；
- 没有 `DrawID`、用户 ID、活动 ID、请求幂等键或最终结果表；
- 没有 Lottery HTTP 路由、请求/响应 DTO、公开错误码或鉴权；
- 没有 React 真实调用，现有 Lottery 页面仍是 Mock；
- 没有资格、次数、黑白名单、风控或规则链；
- 没有库存预占、扣减、回补或 Benefit 发放；
- 没有 Redis 缓存、锁、Lua 或限流；
- 没有 alias method、前缀和索引或批量抽样；
- 没有公平性审计、结果可验证证明或监管合规结论；
- 没有验证产品 NFR 中的业务吞吐和 P99 目标。

一个被选中的 `Award` 仍然只是配置候选。即使它的 Outcome 是 `reward`，也不表示奖品已经有库存、已经发放或已经到账。

## 3. 先建立准确的数学模型

假设 Strategy 已按 AwardID 排序，三项权重分别为 `2、3、5`，总权重为 `10`。本节把随机空间定义为半开区间：

```text
[0, 10)

ticket 0,1       -> Award 1，weight 2
ticket 2,3,4     -> Award 2，weight 3
ticket 5,6,7,8,9 -> Award 3，weight 5
```

每个 Award 占有的整数点数量恰好等于自己的 Weight，因此概率是：

```text
P(Award i) = Weight(i) / TotalWeight
```

Weight 是相对整数，不是百分比。`1:3` 与 `100:300` 的概率语义相同；算法不会要求总和为 100、10000 或其他固定分母。

### 3.1 为什么使用 `[0,total)`

半开区间带来三个直接好处：

1. 与 Go 标准库有界随机 API 的语义一致；
2. 第一个合法值固定为 0，最后一个合法值固定为 `total-1`，边界容易穷举；
3. 不需要计算 `total+1`，因此总权重为 `math.MaxUint64` 时不会溢出。

使用 `[1,total]` 并非数学上不可能，但它更容易诱导实现计算上界加一，也会让测试和标准库 adapter 多一次不必要的语义转换。

## 4. 为什么随机源接口归 domain 所有

本节在 `internal/lottery/domain` 定义：

```go
type BoundedRandomSource interface {
    Uint64N(upper uint64) (uint64, error)
}
```

接口由消费随机能力的核心算法拥有，而不是由某个 adapter 反向规定。它只承诺一件事：输入正上界，返回均匀分布于 `[0,upper)` 的 `uint64`，或者返回错误。

这个接口刻意没有暴露：

- seed；
- 随机字节长度；
- `math/rand` 或 `crypto/rand` 类型；
- `io.Reader`；
- HTTP、数据库或配置对象；
- “失败时返回 0”之类的 fallback。

因此领域算法可以在测试中注入确定票据，在生产中注入密码学随机源，而不会依赖某个标准库包的具体类型。

### 4.1 为什么接口带 error

随机源可能是操作系统熵源、硬件模块或未来的其他实现。技术失败不能伪装成一个合法票据，更不能伪装成 `no_reward`。`Uint64N` 因而返回 `(uint64, error)`；选择器将可返回的 adapter 错误归类为 `ErrRandomSourceFailure`，并返回零值 Award。这里不能误读为默认生产 Reader 的所有故障都可恢复：Go 1.26 的默认 `crypto/rand.Reader` 底层失败采用不可恢复语义，不会进入这条 error chain；显式 error 路径由测试注入 reader 和未来 adapter 验证。

### 4.2 为什么接口当前没有 context

本节 adapter 只调用本机 Go 标准库的密码学随机 Reader，没有网络服务或远程 HSM。为了保持纯选择边界，当前接口没有加入 `context.Context`。

这是有意接受的限制，不是“随机源永远不需要超时”。如果第 21 节或生产环境把熵源换成可能长时间阻塞、可取消的远程能力，就必须重新设计 timeout/cancellation，而不能假装 HTTP context 已经能中断当前接口。

## 5. `WeightedSelector` 的职责

`WeightedSelector` 只读持有一个 `BoundedRandomSource` 依赖，构造后不再修改这个字段；接口背后的 source 仍可拥有自己的可变状态，并且必须自行承担并发安全。Selector 不保存抽奖次数、不保存上一次结果，也不拥有 Strategy 或任何外部资源。

```text
合法 domain.Strategy
        │
        ▼
WeightedSelector
        │ 多候选：Uint64N(totalWeight)，恰好一次
        │ 单候选：不调用随机源
        ▼
按 AwardID 规范顺序执行减法桶
        │
        ├─ 命中 -> 返回完整 domain.Award
        └─ 失败 -> 返回零 Award + 稳定错误
```

选择器不会主动调用 `StrategyReader`。这是重要边界：加载失败属于 Repository/use case 问题，随机选择失败属于算法/熵源问题；两者不能混成一个“未中奖”。

## 6. 单候选为什么直接短路

Strategy 允许只有一个 Award。此时结果在数学上已经确定，调用随机源没有任何信息价值，反而会引入额外失败点和开销。

实现因此先验证：

```text
len(awards) == 1
且该 Award 的 weight == declared totalWeight
```

成立时直接返回唯一 Award，随机源调用次数为 0。测试覆盖 Weight 为 `1`、`400` 和 `math.MaxUint64` 的单候选 Strategy，即使注入源预设为失败也不会被观察到。

短路不等于构造器可以省略随机源：`NewWeightedSelector` 仍要求显式注入非 nil source，使 selector 的组合状态一致，并避免同一类型在候选数量变化后突然没有可用依赖。

如果内部单候选 Strategy 的 weight 与 total 不一致，选择器不会“将错就错”，而是返回 `ErrSelectionInvariantViolation`。

## 7. 为什么使用减法桶

多候选路径拿到 ticket 后按规范顺序执行：

```go
for _, award := range awards {
    weight := uint64(award.Weight())
    if ticket < weight {
        return award, nil
    }
    ticket -= weight
}
```

以权重 `2、3、5` 为例：

- ticket 0 或 1 小于 2，命中第一项；
- ticket 2 先减 2 变成 0，再命中第二项；
- ticket 9 依次减 2、3 后变成 4，命中 Weight 5 的最后一项。

### 7.1 与累计桶比较

累计实现通常写成：

```text
cumulative += weight
if ticket < cumulative { ... }
```

只要 Strategy 永远合法，这也可以工作。但减法形式直接复用“ticket 已小于 total”的事实，不需要在热路径再次构造累计上界，也更直观地避免累计加法与 `MaxUint64` 的边界推理。

### 7.2 为什么暂不使用前缀和、二分或 alias table

当前没有真实 Award 数量分布和业务负载。线性扫描的复杂度为 `O(n)`，额外映射空间为 `O(1)`，实现和边界证据最简单。

前缀和加二分会增加派生结构及其版本一致性；alias method 会增加预计算、内存和更新复杂度。只有 profile/benchmark 证明 `n` 或选择频率让线性扫描成为真实瓶颈时，才值得引入这些结构。

## 8. `MaxUint64` 怎样被完整支持

第 17 节已经接受总权重可以恰好等于 `math.MaxUint64`，第 20 节不能静默收窄这个契约。

实现避免了四种常见错误：

1. 不把 total 转成 `int` 或 `int64`；
2. 不计算 `total+1`；
3. 不使用 `random % total`；
4. 不在桶定位中依赖可能溢出的有符号累计值。

多候选总权重为 `MaxUint64` 时，合法 ticket 范围为 `0` 到 `MaxUint64-1`。测试覆盖第一桶、最后一桶、倒数第二桶和三候选交界。

单候选 Weight 为 `MaxUint64` 时直接短路，不调用熵源。

## 9. 为什么生产 adapter 选择 `crypto/rand.Int`

`internal/lottery/adapter/randomsource.CryptoSource` 使用：

```go
maximum := new(big.Int).SetUint64(upper)
value, err := crypto/rand.Int(reader, maximum)
```

采用它有四个原因：

1. 标准库直接承诺结果均匀分布于 `[0,maximum)`；
2. 标准库负责 rejection sampling，项目不自行实现容易出错的取模修正；
3. `big.Int.SetUint64` 无损支持完整 `uint64` 上界；
4. 默认 `crypto/rand.Reader` 适合价值相关选择且可并发使用。

`math/rand/v2` 适合模拟和非安全随机任务，也提供无偏有界 API；但它不适用于安全敏感场景。当前平台未来会承载积分、优惠券或其他价值权益，而尚无证据证明“结果可预测”是可接受风险，因此本节只交付一个密码学随机生产 adapter，不同时维护两套未被用例需要的实现。

密码学不可预测性仍不等于公平、可审计或不可抵赖。它不能证明运营配置被正确审批、展示概率与生效版本一致、最终结果未被重复生成，或奖品已经按结果发放。

## 10. 两层错误边界

### 10.1 随机源 adapter 错误

`CryptoSource` 定义：

| 错误 | 含义 |
| --- | --- |
| `ErrSourceNotConfigured` | nil receiver 或没有 Reader |
| `ErrUpperBoundRequired` | 直接请求了空区间 `[0,0)` |
| `ErrEntropyUnavailable` | Reader/`crypto/rand.Int` 无法产生合法结果 |

`SourceError.Error()` 只输出稳定分类，`Unwrap` 为可信诊断保留底层 cause。

Go 1.26.6 的默认 `crypto/rand.Reader` 失败不会以普通 error 返回，而会导致不可恢复的进程级终止，因此 HTTP recovery 捕获不到它。`ErrEntropyUnavailable` 当前证明的是“adapter 能为可返回错误的注入 reader/未来实现失败关闭”，不是默认 OS CSPRNG 的运行期告警通道；生产还要依赖进程重启、runtime 与平台告警。`NewCryptoSource` 捕获的全局 `crypto/rand.Reader` 也可被进程内代码替换，最终 composition 必须审查依赖与初始化，而不能只看 `CryptoSource` 类型名。

### 10.2 领域选择错误

`WeightedSelector` 定义：

| 错误 | 判定点 | 是否返回 Award |
| --- | --- | --- |
| `ErrSelectorNotConfigured` | nil/typed-nil source 或零值 selector | 否 |
| `ErrSelectionStrategyInvalid` | 零值或不可选择 Strategy | 否 |
| `ErrRandomSourceFailure` | source 返回 error | 否 |
| `ErrRandomSourceContractViolation` | source 返回 `value >= upper` | 否 |
| `ErrSelectionInvariantViolation` | 声明 total 与真实桶无法映射 | 否 |

`SelectionError` 同样只渲染稳定分类，同时保留 cause 供 `errors.Is`、可信日志和测试使用。任何失败都返回零值 Award；调用方不能拿到“部分成功”结果。

### 10.3 为什么还检查随机源返回范围

接口注释承诺 `[0,upper)`，但接口实现可能有 bug。选择器不使用 `% total` 修复越界值，因为这样会掩盖 adapter 缺陷并可能引入偏差。它返回 `ErrRandomSourceContractViolation`，让错误在跨越信任边界时被发现。

## 11. `no_reward` 不是错误路径

`AwardOutcomeNoReward` 与普通 reward Award 使用完全相同的权重区间。选择到它时：

- 返回完整 Award；
- error 为 nil；
- `HasReward()` 为 false。

这与随机源错误有本质区别：前者是配置允许的业务结果，后者表示本次选择根本没有可信完成。监控、HTTP 映射和未来结果持久化都必须保持这条边界。

## 12. 规范顺序已经成为兼容性契约

Strategy 构造时按 AwardID 升序保存 Awards。选择器直接读取这个私有规范序列，不使用调用方输入顺序、map 遍历顺序或数据库偶然顺序。

排序不会改变每个 Award 的概率，但会改变“某个具体 ticket 对应哪个 Award”。第 20 节发布后，AwardID 顺序和桶映射共同形成兼容性敏感契约：如果未来要支持结果重放、审计或跨版本一致映射，不能无版本地改成权重排序、展示顺序或其他规则。

## 13. 并发前提与资源所有权

`WeightedSelector` 自身没有可变状态，因此同一个 selector 可以被多个 goroutine 调用，前提是注入的 `BoundedRandomSource` 也支持并发。

- 标准 `crypto/rand.Reader` 官方保证可并发使用；
- `NewCryptoSource()` 创建的默认 adapter 因而可以共享；
- 自定义 source 的并发安全不由 selector 自动补偿；
- selector 不为未知 source 加一把全局锁，因为这会隐藏 adapter contract 并无条件串行化所有请求。

并发单元测试分别覆盖“无状态 selector + 安全 fake source”和“默认 CryptoSource + selector”。`go test -race` 只能检查被执行路径中的数据竞争，不能证明未来自定义 source 自动安全，也不证明抽奖业务的库存、幂等或最终结果一致性。

## 14. 测试怎样分层证明

### 14.1 精确桶映射

`weighted_selector_test.go` 对 `2:3:5` 的十个 ticket 全部枚举，验证：

- 每个整数点只命中一个 Award；
- 第一、第二、最后一个桶的边界准确；
- 输入 Award 逆序时仍按 AwardID 规范顺序；
- source 恰好调用一次且 upper 恰好为 total。

### 14.2 权重计数而不是随机频率

测试还对小区间枚举所有 ticket，并验证每个 Award 的命中次数恰好等于 Weight。它覆盖 `1:3`、`100:300` 和 `2:3:5`。

这种确定性证据证明映射没有空洞或重叠，比“随机跑一万次，看起来接近 30%”更稳定，也更容易定位 off-by-one。它不独立证明生产熵源均匀；随机源均匀性来自 `crypto/rand.Int` 的标准库契约和 adapter 边界测试。

### 14.3 最大值与失败

测试覆盖：

- 多候选总和为 `MaxUint64` 的首尾区间；
- 单候选 `MaxUint64` 短路；
- nil 和 typed-nil source；
- 零值 Strategy；
- entropy error；
- source 返回 upper 的 contract violation；
- 仅包内可构造的错误 total/桶映射；
- 错误公开字符串不泄露 cause。

### 14.4 CryptoSource

adapter 测试用确定字节验证完整 `uint64` 上界，并构造第一次候选被 rejection、第二次候选合法的场景，证明 adapter 没有对越界候选取模。失败 Reader 验证 cause 保留但公开字符串安全；默认 Reader 测试只断言范围和并发，不用随机频率制造偶发 CI。

## 15. 基准只能回答局部成本

本节提供两个微基准：

```bash
go test -run '^$' -bench BenchmarkWeightedSelectorWorstCase -benchmem \
  ./internal/lottery/domain

go test -run '^$' -bench BenchmarkCryptoSourceUint64N -benchmem \
  ./internal/lottery/adapter/randomsource
```

选择器基准用固定 source，把 ticket 放在最后一个桶，并分别测 1、10、100、1000 个 Award；它用于观察线性最坏路径随候选数增长的成本。CryptoSource 基准单独观察操作系统熵源和 `big.Int` 路径。

这些数据不能证明：

- Repository 加载、HTTP 编解码和网络延迟；
- 连接池、MySQL、Redis、库存或权益调用成本；
- 多进程吞吐、尾延迟、容器资源和生产噪声；
- 随机结果公平、不可操纵或可审计；
- NFR 中 10,000 RPS / P99 目标已达成。

当前基准没有硬编码性能门槛，也没有足够真实 Award 数量分布。它是以后发现回归和决定是否引入前缀和/alias table 的局部基线，不是简历里的端到端 QPS。

## 16. 安全、公平与审计边界

本节选择密码学随机源降低结果被预测的风险，但完整公平性至少还需要：

- Strategy 的审批、发布版本和生效时间；
- 展示概率与实际配置的对账；
- 一次请求的稳定 DrawID；
- 最终结果原子持久化与重复请求查询；
- 配置版本、Award 快照和算法版本的审计；
- 权限、限流、防刷和异常分布告警；
- 按业务与法规决定的争议处理或第三方验证。

当前实现不记录原始随机字节或 ticket。随意把熵材料写入日志可能扩大预测与泄露风险；将来若需要可验证抽奖，应先设计专门的承诺、披露和审计协议，而不是打开 debug 日志。

## 17. 第 21 节最重要的风险

第 21 节若把当前选择器直接包装成一个公开 `POST /draw`，但仍不保存 Draw 身份和最终结果，多候选 Strategy 会出现危险语义：

```text
客户端请求
  -> 服务完成选择
  -> 响应在网络中丢失
  -> 客户端重试
  -> 再次生成随机票据（多候选）
  -> 可能返回另一个 Award
```

这时服务无法回答“第一次究竟选中了什么”，也不满足 `INV-03`“一次抽奖只能有一个最终结果”。单候选虽会确定返回同一 Award，也仍没有 Draw 身份和最终结果事实，不能据此宣称幂等。因此第 21 节必须在以下方向中做诚实选择：

1. 首个 Lottery API 只开放 Strategy 查询等不会制造最终结果的能力；
2. 将选择接口明确限定为内部/演示且不可声称幂等或最终抽奖；
3. 若要面向真实价值场景开放 Draw，则提前引入请求身份、结果持久化、重复请求返回同一结果和 outcome-unknown 恢复协议。

无论选择哪条路，都不能把 HTTP 重试、Redis 锁或“同一用户短时间限流”冒充最终结果幂等。

## 18. 建议阅读与验证顺序

1. [`strategy.go`](../../../internal/lottery/domain/strategy.go)：复习合法聚合、总权重和 AwardID 规范顺序；
2. [`weighted_selector.go`](../../../internal/lottery/domain/weighted_selector.go)：阅读单候选短路、随机调用和减法桶；
3. [`selection_error.go`](../../../internal/lottery/domain/selection_error.go)：核对稳定分类与 cause 边界；
4. [`crypto.go`](../../../internal/lottery/adapter/randomsource/crypto.go)：核对标准库 adapter 与完整 uint64；
5. [`weighted_selector_test.go`](../../../internal/lottery/domain/weighted_selector_test.go)：逐个重放 ticket、错误、并发和微基准；
6. [`crypto_test.go`](../../../internal/lottery/adapter/randomsource/crypto_test.go)：观察 rejection、entropy error、范围和并发证据。

建议本节验收命令：

```bash
go test ./internal/lottery/domain ./internal/lottery/adapter/randomsource
go test -race ./internal/lottery/...
go vet ./...
make verify
```

完整命令是否实际执行成功、运行环境和剩余风险应由本节 QA 记录，而不能仅凭课程文档宣称。

## 19. 本节小结

第 20 节建立了一条窄而完整的选择链：合法 Strategy 提供正整数权重和规范顺序，domain-owned `BoundedRandomSource` 隔离熵源，`CryptoSource` 生成无偏 `[0,total)` 票据，`WeightedSelector` 用减法桶返回一个完整 Award；单候选不制造无意义随机失败，任何技术或 contract 故障都返回零 Award 和稳定错误，`no_reward` 则保持合法成功结果。

这足以准确表述为“实现支持完整 uint64 权重的无偏加权 Award 选择算法，并以可注入密码学随机源验证边界和失败语义”。它仍不足以表述为“完成在线抽奖、抽奖幂等、结果持久化、库存扣减、权益发放、高并发 SLO 或公平合规”。

## 20. 关联资料

- [第 20 节 API 记录](../../api/lessons/lesson-20.md)
- [ADR-0017：Lottery 无偏加权选择与随机源边界](../../decisions/ADR-0017-lottery-weighted-selection.md)
- [第 19 节：实现仓储层](lesson-19-lottery-repository.md)
- [ADR-0013：Lottery 最小领域模型](../../decisions/ADR-0013-lottery-domain-model.md)
- [ADR-0016：Lottery Repository 边界](../../decisions/ADR-0016-lottery-repository-boundaries.md)

## 参考

- [Go `crypto/rand.Int`](https://pkg.go.dev/crypto/rand#Int)
- [Go `crypto/rand.Reader`](https://pkg.go.dev/crypto/rand#Reader)
- [Go `math/rand/v2`](https://pkg.go.dev/math/rand/v2)
- [Go `math/big.Int.SetUint64`](https://pkg.go.dev/math/big#Int.SetUint64)
- [Go 规范：整数溢出](https://go.dev/ref/spec#Integer_overflow)
- [Go 内存模型](https://go.dev/ref/mem)
