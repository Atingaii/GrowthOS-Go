# ADR-0017：Lottery 无偏加权选择与随机源边界

- **状态：** 已接受
- **日期：** 2026-08-29
- **负责人：** GrowthOS 维护者
- **实现时间切片：** `db679cf`

## 背景

第 17 节建立了 Strategy/Award 纯领域模型：Award 使用正 `uint64` 相对权重，Strategy 至少有一个 Award、按 AwardID 形成规范顺序，并在构造时保证总权重不溢出。第 18 节以两张 MySQL 表保存这些事实，第 19 节又用原子 Create 和一致快照 FindByID 保证算法不会收到半聚合或被静默修复的坏数据。

第 20 节需要决定如何从合法 Strategy 选择一个 Award。这个问题表面是一段循环，实际包含长期兼容和安全边界：

1. 随机区间的开闭规则；
2. `math.MaxUint64` 总权重的完整支持；
3. 随机源的均匀性、不可预测性、失败和可替换性；
4. 票据到 Award 的稳定映射；
5. 单候选、`no_reward` 与技术失败的语义；
6. 并发调用和资源所有权；
7. 算法性能证据与业务 SLO 的边界。

当前仍没有 Draw/Result 身份、最终结果持久化、HTTP API、规则、库存、Benefit 或 Redis 业务接入。因此本 ADR 只决定“Strategy 内的一次加权 Award 选择”，不把它扩大成完整在线抽奖协议。

## 决策驱动

1. 每个 Award 的实际命中区间必须与 Weight 精确相等，不使用浮点概率。
2. 随机源必须提供无偏 `[0,total)` 值，不能用简单取模修正。
3. Strategy 已接受的完整 `uint64` 范围必须继续成立，包括总权重 `MaxUint64`。
4. 同一 Strategy 与同一 ticket 必须得到确定的 Award，不能依赖输入、map 或 SQL 偶然顺序。
5. `no_reward` 是合法成功结果；随机源/配置/算法故障没有 Award，二者不可混淆。
6. 算法必须能用确定 source 精确测试，不让 CI 依赖随机频率。
7. 领域不能直接依赖某个随机库具体类型；adapter 不能反向决定领域接口。
8. 对可能承载价值权益的选择，默认实现不应使用可预测 PRNG，除非以后有明确威胁模型接受该风险。
9. 当前没有性能证据支持预计算表、缓存或第三方采样库。
10. 内部算法错误不得把熵源细节或随机材料泄露给未来客户端。

## 非目标

- 不定义 DrawID、用户/活动/参与身份或最终结果记录；
- 不验证或声明 `INV-03` 已满足；
- 不定义 HTTP route、DTO、status/code、鉴权、限流或重试；
- 不读取 Repository，不在 selector 内管理 MySQL/Redis；
- 不实现资格、次数、规则链、库存或权益发放；
- 不实现无放回抽样、批量抽样或多阶段选择；
- 不实现 alias method、前缀和缓存或 SIMD 优化；
- 不记录随机字节、seed 或 ticket；
- 不承诺可重放、可验证随机、公平审计或监管合规；
- 不用微基准宣称端到端 Lottery QPS/P99。

## 候选方案矩阵

### 1. 算法与随机端口放在哪里

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| handler/use case 中直接写随机循环 | 文件少、调用直接 | 核心概率规则散落在 transport；难做稳定映射与复用 | 不采用 |
| `Strategy.Draw()` 内直接调用标准库全局随机源 | 领域方法直观 | 随机依赖不可注入；测试和威胁模型被写死；名称误导为完整 Draw | 不采用 |
| application 拥有算法和随机接口 | 易与未来用例组合 | Strategy 内部规范序列需复制暴露；核心选择规则离开其不变量所有者 | 可行但不采用 |
| domain-owned `BoundedRandomSource` + `WeightedSelector` | 核心规则与 Strategy 不变量同层；接口由 consumer 拥有；adapter 向内依赖 | domain 多一个技术能力抽象；未来远程熵源需重评 context | **采用** |

### 2. 随机源

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| `math/rand/v2` 全局函数 | 标准库、有无偏有界 API | PRNG 可预测，不适合安全敏感选择；无法返回熵源错误；本节未做同条件性能对照 | 当前不采用生产实现 |
| 固定 seed PRNG | 测试可重放 | 生产结果高度可预测；重启/副本可能重复序列 | 禁止作为默认生产源 |
| 读取 `uint64` 后 `% upper` | 实现短 | `2^64` 不能整除多数 upper，产生 modulo bias | 禁止 |
| 项目自写 rejection sampling | 可控制分配与错误 | 安全敏感边界重复造轮子；更大审计负担 | 不采用 |
| `crypto/rand.Int` + `big.Int.SetUint64` | 标准库承诺均匀 `[0,max)`；密码学不可预测；完整 uint64；可保留自定义 Reader error | 有系统熵源与分配成本；不可取消；Go 1.26 默认 Reader 底层失败不可恢复而非返回 error；不自动提供审计 | **采用** |

### 3. 随机区间

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| 浮点 `[0,1)` 后乘 total | 公式常见 | 精度和大整数转换会损失完整 uint64；边界复杂 | 不采用 |
| 整数 `[1,total]` | 人工解释直观 | 容易计算 `total+1`；与标准库有界 API 语义不同 | 不采用 |
| 整数 `[0,total)` | 标准半开区间；边界可穷举；无 `total+1` | 调试时需理解 0-based ticket | **采用** |

### 4. 桶定位

| 方案 | 复杂度 | 优点 | 代价/风险 | 结论 |
| --- | ---: | --- | --- | --- |
| 累计权重线性扫描 | `O(n)` | 常见、直观 | 需要重新推理累计加法边界 | 可行但不采用 |
| ticket 逐项减 weight | `O(n)` | `O(1)` 额外空间；不做累计加法；MaxUint64 推理简单 | 最坏扫描全部 Award | **采用** |
| 前缀和 + 二分 | 构建 `O(n)`、查询 `O(log n)` | 高频读时更快 | 派生结构、版本和缓存失效成本；当前无规模证据 | 暂不采用 |
| alias method | 构建 `O(n)`、查询 `O(1)` | 大量重复采样高效 | 实现/内存/审计复杂；配置变化需重建 | 暂不采用 |

### 5. 单候选行为

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| 仍调用随机源 | 路径统一；每次都有熵调用 | 数学结果不变却增加失败与开销 | 不采用 |
| 验证唯一桶后直接返回 | 确定、零随机失败、支持最大权重 | 若未来审计要求每次熵证据需重评 | **采用** |

### 6. 测试随机分布

| 方案 | 优点 | 代价/风险 | 结论 |
| --- | --- | --- | --- |
| CI 随机抽一万次并设置比例容差 | 看似接近业务 | 可能偶发失败；无法精确定位边界；弱样本可漏 bug | 不作为正确性门禁 |
| 注入每个合法 ticket 并完整枚举小区间 | 精确证明桶数量、边界、空洞和重叠 | 不能独立证明生产熵源均匀 | **采用** |
| 信任标准库随机 contract、adapter 测范围/rejection/error | 不重复验证密码学库统计性质 | 依赖 Go 标准库正确性 | **采用** |

## 决策

### 1. Domain-owned 随机端口

1. 在 `internal/lottery/domain` 定义：

   ```go
   type BoundedRandomSource interface {
       Uint64N(upper uint64) (uint64, error)
   }
   ```

2. source 必须均匀返回半开区间 `[0,upper)`；`upper == 0` 必须拒绝。
3. source 不得用 modulo 修复越界候选。
4. source error 表示本次选择没有完成，不能返回 fallback Award。
5. 接口当前不携带 context；若熵源变成远程/可阻塞依赖，必须新增决策。
6. 一个 source 被并发共享时，它自己负责并发安全。

### 2. Selector 构造与所有权

1. `NewWeightedSelector` 必须显式接收非 nil `BoundedRandomSource`。
2. 普通 nil 和 typed-nil source 都返回 `ErrSelectorNotConfigured`。
3. selector 只借用 source，不关闭、不替换、不重新 seed。
4. selector 自身没有可变状态；其并发安全以 source 并发安全为前提。
5. 零值/nil selector 调用失败关闭，不 panic。

### 3. 单候选短路

1. Strategy 只有一个 Award 时，不调用 `Uint64N`。
2. 返回前验证唯一 Award 的 Weight 与 Strategy TotalWeight 一致。
3. 一致则直接返回唯一 Award，包括 Weight 为 `math.MaxUint64` 的情况。
4. 不一致返回 `ErrSelectionInvariantViolation` 和零 Award。
5. 构造 selector 时仍要求 source，避免 Strategy 从单候选演进成多候选后组合状态失效。

### 4. 多候选选择

1. 将 `strategy.totalWeight` 原样传给 source，恰好调用一次。
2. 收到 error 时返回 `ErrRandomSourceFailure` 和零 Award。
3. 即使接口承诺范围，selector 仍验证 `ticket < totalWeight`；越界返回 `ErrRandomSourceContractViolation`，禁止取模修复。
4. 按 Strategy 内部 AwardID 规范顺序遍历。
5. 对每个 Award：若 `ticket < weight` 则命中，否则执行 `ticket -= weight`。
6. 遍历结束仍未命中，返回 `ErrSelectionInvariantViolation`。
7. 返回的是完整不可变 `domain.Award` 值，不返回索引、概率小数、裸 ID 或 nil。

### 5. 完整 uint64 契约

1. 不把 ID、Weight、TotalWeight 或 ticket 转换为 `int`/`int64`。
2. 区间固定为 `[0,totalWeight)`，不计算 `totalWeight+1`。
3. 减法桶不使用可能回绕的累计加法。
4. 多候选总和为 `MaxUint64` 时支持从 ticket 0 到 `MaxUint64-1` 的完整合法区间。
5. 若未来 adapter 无法支持完整范围，必须以新 ADR 明确收窄领域契约；不得静默截断。

### 6. CryptoSource adapter

1. adapter 位于 `internal/lottery/adapter/randomsource`。
2. `NewCryptoSource()` 使用标准库进程级 `crypto/rand.Reader`。
3. `Uint64N` 使用 `new(big.Int).SetUint64(upper)` 与 `crypto/rand.Int`。
4. 不自行实现 modulo/rejection 算法；标准库负责均匀有界采样。
5. nil receiver/reader、零上界和可返回的 reader/sampling failure 分别有稳定错误类别；Go 1.26 默认 Reader 的底层失败是进程级不可恢复事件，不会进入 `SourceError`。
6. adapter 再次验证返回 big.Int 是非负、可表示为 uint64 且小于 upper，异常时失败关闭。
7. 测试使用包内不可导出的 Reader 注入点；生产 API 不开放任意 Reader 配置，避免无需求地允许弱随机源。

### 7. 结果语义

1. `reward` 与 `no_reward` Award 都是 `Select` 的成功返回，error 为 nil。
2. source/contract/invariant 错误都返回零值 Award。
3. 选择 reward 不表示库存预占、Benefit 创建或交付成功。
4. 返回 Award 不等于形成一个持久化最终 DrawResult，也不满足 `INV-03`。

### 8. 错误 contract

选择层固定以下稳定类：

| 类 | 含义 |
| --- | --- |
| `ErrSelectorNotConfigured` | selector 没有可用 source |
| `ErrSelectionStrategyInvalid` | Strategy 不可选择 |
| `ErrRandomSourceFailure` | source 返回错误 |
| `ErrRandomSourceContractViolation` | source 返回范围外值 |
| `ErrSelectionInvariantViolation` | ticket 无法映射到声明桶 |

randomsource adapter 固定以下稳定类：

| 类 | 含义 |
| --- | --- |
| `ErrSourceNotConfigured` | CryptoSource 没有 Reader |
| `ErrUpperBoundRequired` | upper 为 0 |
| `ErrEntropyUnavailable` | 熵源/标准库采样失败或返回异常值 |

`SelectionError` 与 `SourceError` 的 `Error()` 只渲染稳定分类，`Unwrap` 保留可信 cause。未来 HTTP adapter 必须显式映射，不能原样序列化 error chain。

## 关键控制流

### 单候选

```text
Strategy(one Award)
  -> 验证 weight == total
      -> true: 直接返回 Award，source calls = 0
      -> false: invariant violation，零 Award
```

### 多候选

```text
Strategy(multiple Awards)
  -> source.Uint64N(total) exactly once
      -> error: random source failure，零 Award
      -> ticket >= total: source contract violation，零 Award
      -> valid ticket:
           for Award in canonical AwardID order
             ticket < weight ? return Award : ticket -= weight
           fallthrough: invariant violation，零 Award
```

## 必须保持的实现约束

1. 不得把 Weight 解释为百分比或要求固定总和。
2. 不得把 ticket 或 total 转成有符号整数。
3. 不得计算 `total+1`。
4. 不得使用 `% total` 生成或修复有界随机值。
5. 不得依赖调用方输入顺序、map 顺序或 SQL 默认顺序。
6. 多候选每次 Select 恰好调用 source 一次；成功时消费一个合法随机票据，source 失败或违约时不产生合法票据；不得为每个 Award 分别抽随机值。
7. 单候选不得无意义调用随机源。
8. source error、contract violation 和 invariant violation 不得降级为 `no_reward`。
9. 失败不得返回非零/部分 Award。
10. 不得把底层 entropy error 或随机材料放入安全公开错误字符串。
11. selector 不得加载 Repository、持久化结果、修改 Strategy 或产生外部副作用。
12. selector 并发安全声明必须包含“source 也安全”的前提。
13. 微基准结果不得外推为 HTTP、数据库或业务 SLO。
14. 更改规范排序或 ticket 映射必须考虑历史兼容与算法版本。

## 不变量与信任边界

### Strategy 到 selector

Strategy 私有字段和构造器保证 Awards 非空、Weight 正数、ID 唯一、总和不溢出并按 ID 排序。selector 仍防御零值 Strategy 和仅包内才可能构造的 total/桶不一致，但不在每次选择时重新复制、排序或完整 Restore 聚合。

### Selector 到随机 source

source 是一个信任边界。接口规定均匀 `[0,upper)`，selector 仍检查返回范围；它无法在一次调用中自行统计证明 source 均匀，也不会用 modulo 掩盖错误实现。

### Randomsource 到标准库/操作系统

CryptoSource 信任 Go `crypto/rand.Int` 的均匀采样契约和默认 Reader 的操作系统熵。adapter 为测试注入 reader 与未来可返回错误的实现保留 error chain；Go 1.26 默认 Reader 的底层失败不可恢复，不会变成 `ErrEntropyUnavailable`，需要由进程退出、runtime 与平台告警发现。本节没有运行时装配、熵健康服务、HSM 或跨主机随机性审计；`crypto/rand.Reader` 又是可被进程内代码替换的导出变量，因此最终 composition 还必须审查初始化顺序与依赖。

### 内部返回到未来 transport

返回的 Award 是内部领域值，不是 JSON DTO。未来 transport 必须决定 uint64 编码、枚举、错误、权限和结果身份，不能直接反射私有对象或 error cause。

## 并发与时间边界

1. WeightedSelector 不写内部状态，多个 goroutine 可共享。
2. 默认 `crypto/rand.Reader` 可并发使用，因此 `NewCryptoSource()` 可由多个选择共享。
3. 自定义 source 的并发安全由其实现负责；selector 不加全局锁补救。
4. 当前 source API 没有 context，不能承诺外部取消会中断熵读取。
5. 当前选择没有数据库事务、锁或结果写入，因此并发测试只证明内存数据竞争边界，不证明业务幂等、库存一致性或唯一最终结果。

## 验证策略

### 确定性映射

- 枚举 `2:3:5` 的全部十个 ticket；
- 输入 Awards 逆序，验证仍按 AwardID 映射；
- 验证 source upper 等于 total 且多候选只调用一次；
- 枚举小区间，验证每个 Award 的命中次数精确等于 Weight；
- 对比 `1:3` 与 `100:300` 的比例语义。

### 边界与失败

- 单候选 Weight 1、400、MaxUint64 均短路且 source 调用 0 次；
- 多候选总和 MaxUint64 的第一、倒数第二和最后桶；
- nil/typed-nil source、零 selector、零 Strategy；
- source error 与 wrapped cause；
- source 返回 upper 的 contract violation；
- 人工破坏的 total/桶不一致返回 invariant violation；
- 所有错误路径返回零 Award，公开 Error 不泄露 cause。

### CryptoSource

- nil receiver/reader 和 zero upper；
- failing Reader 的 cause 保留；
- MaxUint64 上界返回 MaxUint64-1；
- 构造 rejected candidate 后继续读取合法 candidate，证明非 modulo；
- 多种 upper 的范围断言；
- 默认 Reader 和完整 selector 的并发/race 覆盖。

确定性测试证明桶映射；标准库契约与 adapter 测试证明所选实现按边界调用。随机频率测试不作为 CI 正确性证据。

## 基准与性能边界

本节保留两个微基准：

- `BenchmarkWeightedSelectorWorstCase`：1、10、100、1000 Award，ticket 固定命中最后一项；
- `BenchmarkCryptoSourceUint64N`：单独测默认密码学有界随机调用。

它们用于观察 `O(n)` 扫描、分配和标准库熵源局部成本，并为以后重评前缀和/alias method 提供起点。当前没有固定门槛，因为没有真实 Award 数量、调用比例和硬件矩阵。

这些微基准不包含 Repository、MySQL、HTTP、JSON、网络、Redis、库存、权益、日志、容器调度或结果持久化，不能更新 Lottery 业务 NFR，不能表述为端到端 QPS/P99，也不能证明公平性。

## 影响

### 正面影响

- 概率规则与 Strategy 不变量在同一领域边界；
- 随机 adapter 可替换，测试不依赖真实随机频率；
- 标准库有界密码学采样避免自写 modulo/rejection 错误；
- 半开区间和减法桶完整支持 MaxUint64；
- 单候选不增加无意义 entropy 故障；
- `no_reward` 与技术失败保持清楚；
- 规范顺序让相同 ticket 映射确定；
- 安全错误分类可供未来 application/transport 显式映射；
- 无第三方依赖、无数据库或运行拓扑变化。

### 成本

- 多候选选择为线性最坏扫描；
- `crypto/rand.Int`/`big.Int` 有系统调用和分配成本；
- domain 包需要认识一个有错误返回的随机能力端口；
- source contract 的均匀性不能由 selector 单次调用自行证明；
- 当前接口无法通过 context 取消熵读取；
- 规范 ticket 映射在发布后成为兼容性负担。

### 风险

- 未来开发者可能把“密码学随机”夸大为“公平、可审计或合规”；
- 第 21 节可能在没有结果身份/持久化时开放可重试 Draw，生成多个结果；
- 自定义 source 可能违反范围、均匀性或并发 contract；
- 随意记录 ticket/熵材料可能扩大预测或泄露风险；
- 候选规模增长后线性扫描可能成为瓶颈；
- Strategy 排序改变会让固定 ticket 映射漂移；
- 单候选短路若未来审计政策要求每次必须消费可验证熵，需要重新决策。

## 失败与恢复语义

| 失败点 | 当前已知状态 | 安全动作 |
| --- | --- | --- |
| selector 未配置 | 未选择、无副作用 | 修复 composition；不得当 no_reward |
| Strategy 无效 | 未调用 source、无副作用 | 修复调用者/加载链；不得随机修复 |
| entropy/source error | 没有可信 Award | 返回 selection failure；不 fallback、不重抽冒充同一次 |
| source 返回越界 | adapter contract 已破坏 | 失败关闭并告警；不得 `% total` |
| 内部 mapping hole | Strategy/算法不变量破坏 | 失败关闭、阻断流量并诊断版本/数据 |
| 选择到 no_reward | 选择成功 | 作为业务结果处理，不进入技术错误恢复 |
| Award 已返回但未来 HTTP 响应丢失 | 本节没有最终结果事实 | 当前无法安全恢复；第 21 节不得盲重抽 |

由于本节没有持久化副作用，selector 自身不做重试。未来一次 Draw 是否允许重试只能由掌握请求身份、结果记录和外部副作用的用例决定。

## 第 21 节约束

1. 第 21 节不得把内部 `Select` 直接等同于一个已经持久化的 Draw。
2. 若 HTTP endpoint 可能被客户端重试，必须明确重复调用是否形成新选择。
3. 在没有 DrawID/结果唯一约束时，不得声明幂等或满足 `INV-03`。
4. 可以优先开放 Strategy 查询，或明确标注内部演示选择；真实价值 Draw 需要先增加请求身份、原子结果保存与状态查询。
5. Repository not-found/stored-invalid/dependency error 与 selection source/contract/invariant error 必须分别映射。
6. `no_reward` 应是成功业务响应，不得和 5xx 合并。
7. JSON 不能用 JavaScript number 无损表达全部 uint64，必须决策 string/范围策略。
8. 当前随机源没有 context；HTTP deadline 是否覆盖随机能力必须明确记录。
9. 不记录随机材料；日志/metrics 只记录低基数结果类别和安全错误分类。
10. HTTP、浏览器或 Compose 只有真正装配 selector 后，才能作为算法运行链证据。

## 演进与撤销方式

- 若真实 profile 证明线性扫描是瓶颈，可新增不可变前缀表或 alias table，但必须用同一 ticket contract、版本兼容和属性测试证明概率不变。
- 若选择变成无放回或库存感知，新增明确领域行为；不得在当前 Weight 上偷偷返回“下一个可用奖”。
- 若使用远程 HSM/随机服务，重新设计 context、timeout、熔断、并发和可用性；不要把网络故障降级为 no_reward。
- 若法规要求可验证随机，新增承诺/披露、算法版本、结果审计与争议协议；仅保存 seed 或打印 ticket 不构成安全方案。
- 若业务明确接受可预测 PRNG 以换取性能，应以威胁模型、隔离场景和基准写新 ADR，不直接替换默认 CryptoSource。
- 若总权重必须收窄，先处理领域、数据库和 API 的兼容性，不能只改随机 adapter。

## 重评触发器

以下任一证据出现时重新评估：

1. p95/max Award 数量或 profile 证明线性扫描成为显著瓶颈；
2. CryptoSource 成为端到端延迟或成本热点；
3. 引入远程 HSM、随机服务、FIPS/监管或第三方公证要求；
4. 需要可重放、争议验证、算法版本或随机承诺协议；
5. 出现无放回、批量、多阶段、库存感知或动态权重选择；
6. Strategy/Award 排序或版本语义变化；
7. 单候选也必须产生审计熵证据；
8. 需要根据 request context 取消随机源；
9. 生产发现 source contract、并发或熵失败；
10. 第 21 节要开放面向真实价值的可重试 Draw API；
11. 产品要求缩小 uint64 权重范围或固定概率分母；
12. 线上分布、配置公示或审计发现理论与实际不一致。

## 验收证据

- [Domain selector](../../internal/lottery/domain/weighted_selector.go)
- [Selection errors](../../internal/lottery/domain/selection_error.go)
- [Selector tests and benchmark](../../internal/lottery/domain/weighted_selector_test.go)
- [CryptoSource adapter](../../internal/lottery/adapter/randomsource/crypto.go)
- [CryptoSource tests and benchmark](../../internal/lottery/adapter/randomsource/crypto_test.go)
- [Strategy invariants](../../internal/lottery/domain/strategy.go)
- [第 20 节课程](../course/part-03/lesson-20-lottery-weighted-selection.md)
- [第 20 节 API 记录](../api/lessons/lesson-20.md)
- [ADR-0013：Lottery 最小领域模型](ADR-0013-lottery-domain-model.md)
- [ADR-0016：Lottery Repository 边界](ADR-0016-lottery-repository-boundaries.md)

## 参考

- [Go `crypto/rand.Int`](https://pkg.go.dev/crypto/rand#Int)
- [Go `crypto/rand.Reader`](https://pkg.go.dev/crypto/rand#Reader)
- [Go `math/rand/v2`](https://pkg.go.dev/math/rand/v2)
- [Go `math/big.Int.SetUint64`](https://pkg.go.dev/math/big#Int.SetUint64)
- [Go 规范：整数溢出](https://go.dev/ref/spec#Integer_overflow)
- [Go 内存模型](https://go.dev/ref/mem)
