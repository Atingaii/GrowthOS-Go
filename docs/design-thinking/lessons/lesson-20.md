# 第 20 节第一性原理设计手记：把“按权重选一个候选”与“完成一次抽奖”彻底分开

本文记录第 20 节“实现最简单概率抽奖”的架构推导。

它不是随机数算法百科，也不是面向监管机构的公平性证明。

它试图回答的是一个更窄、也更容易被说错的问题：

> 在已经拥有合法且不可变的 Lottery Strategy 快照之后，怎样以完整 `uint64` 精度、无模偏差、可注入随机源和失败关闭语义，恰好选择一个配置中的 Award，同时拒绝把这次内存选择夸大成 Draw、幂等结果、库存扣减、权益到账或合规公平？

本文结论只适用于 2026-08-29、实现提交 `db679cf` 的代码时间切片。

这个时间切片已经包含：

- `domain.BoundedRandomSource`；
- `domain.WeightedSelector`；
- 单候选确定性短路；
- `[0,totalWeight)` 上的剩余权重减法扫描；
- `randomsource.CryptoSource`；
- 基于 `crypto/rand.Int` 的拒绝采样；
- 完整 `uint64` 上界支持；
- 稳定且脱敏的 Selection/Source error；
- 精确区间、最大值、拒绝采样、失败关闭和并发测试；
- 纯扫描与密码学随机源 benchmark。

这个时间切片仍然没有：

- Lottery HTTP API；
- Draw、DrawID 或 DrawResult；
- 请求幂等键；
- 用户资格与参与次数；
- 库存或预算；
- Strategy 版本；
- 结果持久化；
- 权益发放；
- Redis 业务缓存；
- 生产流量；
- 线上概率监控；
- 可公开验证随机性；
- 合规审计结论。

因此，全文中的“选择成功”只表示函数返回了一个配置内 Award 和 `nil` error。

它绝不自动等价于：

```text
用户完成了一次抽奖
≈ 结果已经唯一持久化
≈ 奖励库存已经保留
≈ 权益已经到账
≈ 客户端重试仍会得到相同结果
≈ 整个系统已经公平、不可作弊且可审计
```

这些等号在当前实现中全部不成立。

---

## 1. 决策命题与事实切片：先定义“我们到底选择了什么”

### 1.1 为什么第 20 节现在才出现

第 17 节只建立 Strategy 与 Award 领域对象。

第 18 节只建立两张 MySQL 业务表。

第 19 节只建立 Strategy 的 Create/FindByID Repository。

这个顺序不是为了把代码拆得更碎。

它是在逐步消除算法入口的不确定性：

```text
第 17 节
候选是否合法？
权重是什么？
未中奖怎样表达？

第 18 节
这些事实怎样无损保存？
数据库能挡住哪些局部坏数据？

第 19 节
父子多行怎样组成一个一致快照？
坏存量数据怎样失败关闭？

第 20 节
在合法快照上怎样无偏选择一个候选？
```

如果算法早于领域模型出现，算法会自己猜测：

- 空列表该返回什么；
- 权重是不是百分比；
- 权重能不能为零；
- 未中奖是不是 `nil`；
- 候选顺序是否稳定；
- 总权重能否溢出。

如果算法早于 Repository 出现，算法还会被迫承担：

- SQL 查询；
- 父子行拼装；
- 一致性快照；
- 数据修复；
- MySQL 错误分类。

现在这些问题已经由前置边界回答。

所以第 20 节可以只处理一个数学与安全命题。

### 1.2 本节最小业务事实

给定一个 Strategy：

- 它至少有一个 Award；
- 每个 Award 的 Weight 都是正整数；
- AwardID 在 Strategy 内唯一；
- Awards 按 AwardID 升序形成规范顺序；
- `TotalWeight` 是所有 Weight 的精确和；
- 求和已经检查 `uint64` 溢出；
- Strategy 与内部 slice 均不可由调用方任意修改。

我们需要从这些 Award 中返回一个。

返回概率必须满足：

```text
P(Award_i) = Weight_i / TotalWeight
```

这是一种“有放回”的单次加权选择。

当前没有“不放回”语义。

当前没有库存耗尽后移除候选的语义。

当前没有基于用户、时间或规则动态改变权重的语义。

### 1.3 当前实现事实

当前 [`WeightedSelector`](../../../internal/lottery/domain/weighted_selector.go) 的行为是：

1. 拒绝零值或未配置 selector；
2. 拒绝没有 Award 或总权重为零的 Strategy；
3. 对单 Award Strategy 校验内部权重与总权重一致；
4. 单 Award 校验通过后直接返回，不读取随机源；
5. 多 Award Strategy 请求一个 `[0,totalWeight)` 的随机 ticket；
6. 随机源 error 被分类为 `ErrRandomSourceFailure`；
7. 随机源返回越界值被分类为 `ErrRandomSourceContractViolation`；
8. 按 Strategy 内部 AwardID 规范顺序逐项减去权重；
9. 第一个满足 `ticket < weight` 的 Award 被返回；
10. 若合法 ticket 无法映射，返回 `ErrSelectionInvariantViolation`；
11. 所有失败都返回零值 Award；
12. `SelectionError.Error()` 只显示稳定语义类，cause 只通过 `Unwrap()` 保留。

当前 [`CryptoSource`](../../../internal/lottery/adapter/randomsource/crypto.go) 的行为是：

1. 使用 `crypto/rand.Reader` 作为默认熵边界；
2. 使用 `new(big.Int).SetUint64(upper)` 保留完整无符号范围；
3. 使用 `crypto/rand.Int(reader, maximum)` 生成 `[0,upper)`；
4. 依赖标准库拒绝超出范围的候选，而不是 `% upper`；
5. 再次检查返回值非负、可表示为 `uint64` 且小于 upper；
6. 对 nil receiver、nil reader、零上界和熵失败分类；
7. `SourceError.Error()` 不公开底层 reader cause；
8. 测试用 reader 注入口保持为包内未导出能力。

### 1.4 当前没有形成的业务事实

`Select(strategy)` 没有接收：

- user ID；
- activity ID；
- participation ID；
- request ID；
- idempotency key；
- inventory key；
- current time；
- eligibility rules；
- tenant；
- operator；
- strategy version；
- trace context。

它也没有写入任何数据库、缓存或消息。

所以函数返回后只存在一个进程内局部值。

进程崩溃不会留下可查询的 DrawResult。

调用者超时重试会再次运行随机选择。

两个并发调用会分别执行选择，但端口不承诺它们在统计上独立；结果可能相同，也可能不同。

任何调用者都不能仅凭本节 API 查询“之前那一次选中了什么”。

这正是为什么本节不能声称满足 `INV-03`。

### 1.5 决策推导主链

```text
Weight 是正 uint64 相对权重
→ 不能用 float64 百分比重新解释
→ 需要在整数区间上选择

TotalWeight 已经检查溢出且允许 MaxUint64
→ 随机上界必须支持完整 uint64
→ 不能转 int64
→ 不能计算 total+1

希望每个 Award 概率精确等于 weight/total
→ 随机 ticket 必须在 [0,total) 均匀
→ 不能直接 rawRandom % total
→ bounded source 必须执行无偏缩减/拒绝采样

Strategy 已按 AwardID 规范排序
→ 算法遍历这个稳定顺序
→ 不按权重重新排序
→ 同一 Strategy + 同一 ticket 可确定映射

Strategy 内部 slice 不可变且 selector 在 domain 包
→ 可以直接遍历私有 slice
→ 不必调用 Awards() 复制
→ 纯扫描可做到零分配

单 Award 的结果概率恒为 1
→ 不存在随机决策
→ 可以在验证内部总权重后短路

no_reward 是合法 Award
→ 选中它仍然成功
→ 不得转换成 error 或触发重抽

随机源是外部不确定能力
→ 通过窄接口注入
→ 测试可提供精确 ticket
→ 生产 adapter 使用 CSPRNG

选择没有持久化身份
→ 只能叫 Weighted Selection
→ 不能叫完成 Draw
```

---

## 2. Why / What / How：用第一性原则排除“看起来像抽奖”的伪需求

### 2.1 Why：为什么必须有一个独立选择器

最直接的写法可能是把几行随机逻辑放进未来 Gin handler。

这会立即制造五种耦合：

1. HTTP DTO 决定领域权重精度；
2. handler 决定随机源；
3. handler 决定 `no_reward` 是否成功；
4. handler 决定算法错误怎样重试；
5. handler 测试同时依赖路由、JSON 和概率边界。

另一个直接写法是把随机 SQL 放进 Repository。

例如：

```sql
ORDER BY RAND()
LIMIT 1
```

这不仅不能表达相对权重，还把概率正确性、数据库计划、数据规模和事务快照揉成一个黑盒。

即使增加复杂 SQL 权重表达，测试 MaxUint64、模偏差和随机源失败也会变得困难。

因此独立 selector 的价值不是“DDD 文件夹更漂亮”。

它使以下问题可以单独被证伪：

- 一个 ticket 是否恰好落入一个区间；
- 区间长度是否等于 Weight；
- 最大无符号范围是否安全；
- `no_reward` 是否正常返回；
- source error 是否失败关闭；
- 并发调用是否共享可变算法状态；
- 算法复杂度是否随 Award 数量线性增长。

### 2.2 What：本节真正交付的能力

本节交付的是：

```text
合法 Strategy 快照
      +
均匀 bounded random source
      ↓
配置中的一个 Award 或明确 error
```

它保证：

- 成功时返回配置内完整 Award；
- 多候选时 source 上界等于 Strategy TotalWeight；
- 随机值到 Award 的映射没有空洞或重叠；
- 单候选时结果确定；
- 技术失败不会伪装业务未中奖；
- 内部诊断 cause 不进入公开错误字符串；
- selector 自身没有可变共享状态。

### 2.3 What not：本节刻意不交付的能力

本节不交付：

- “用户有资格抽”；
- “用户还有一次机会”；
- “一次机会已经扣减”；
- “请求重复时返回同一结果”；
- “库存足够”；
- “奖励已经锁定”；
- “抽奖结果已经落库”；
- “奖励已经发放”；
- “前端动画结束”；
- “某次活动使用了该 Strategy”；
- “随机过程可由第三方验证”；
- “运营修改受到审批”；
- “概率满足某地监管规范”。

这些不是本节漏写的辅助代码。

它们是新的业务事实和新的失败模型。

### 2.4 How：当前实现选择的最小机制

当前机制分成两个责任：

```text
domain.WeightedSelector
负责：
- 单候选短路
- bounded source 调用
- ticket 越界防御
- 权重区间映射
- no_reward 结果保真
- selection error 分类

adapter/randomsource.CryptoSource
负责：
- 生产熵来源
- uint64 上界转 big.Int
- crypto/rand.Int 拒绝采样
- source 配置/上界/熵错误分类
- 默认并发安全来源
```

domain 不知道：

- `crypto/rand.Reader`；
- `math/big.Int`；
- `io.Reader`；
- 操作系统随机 API；
- FIPS 模式；
- 测试字节序列。

adapter 不知道：

- Strategy；
- Award；
- Weight 的业务含义；
- `reward` / `no_reward`；
- AwardID 规范顺序；
- 一次选择之后做什么。

### 2.5 为什么不是一个全能 `LotteryService`

如果现在定义：

```go
LotteryService.Draw(ctx, userID, activityID)
```

函数签名看似更接近产品。

但它会隐藏大量尚无答案的问题：

- Strategy 从哪里来；
- activity 与 strategy 如何关联；
- user 是否有资格；
- 次数如何原子扣减；
- 结果是否持久化；
- 重试是否返回同一结果；
- 库存不足如何处理；
- Benefit 失败是否重抽；
- context 取消后结果是否已经成立。

一个名字不能替代这些协议。

因此本节选择诚实的 `WeightedSelector.Select(strategy)`。

### 2.6 为什么不直接把 selector 与 Repository 组合

Repository 失败和随机选择失败是两类不同事实。

如果 selector 内部执行 `FindByID`：

- 单元测试会依赖数据库 fake；
- benchmark 会混入 SQL；
- not found 与 entropy failure 可能被一个 error 掩盖；
- context、事务和概率算法纠缠；
- 未来缓存无法单独替换 StrategyReader；
- 无法在纯内存 fixture 上穷举所有 ticket。

第 21 节出现真实用例时，由 application 组合：

```text
StrategyReader
      ↓
合法 Strategy
      ↓
WeightedSelector
      ↓
Award
```

组合不等于合并责任。

---

## 3. 约束与不变量：算法只能建立在已经成立的事实之上

### 3.1 Strategy 构造不变量

[`Strategy`](../../../internal/lottery/domain/strategy.go) 在构造时保证：

- StrategyID 非零；
- 名称规范且合法；
- 至少一个 Award；
- 每个 Award 自身合法；
- AwardID 不重复；
- 每个 Weight 大于零；
- Outcome 属于封闭词汇；
- 总权重不会溢出；
- 输入 slice 被复制；
- Award 按 ID 排序；
- totalWeight 只由成员计算。

selector 不重新解释这些规则。

这是一条重要的信任边界：

```text
不可信 DTO / SQL row
→ 构造器或 Restore
→ 合法 Strategy
→ Selector
```

绕过构造器制造半合法 Strategy，不是 selector 应正常兼容的外部输入路径。

### 3.2 selector 自身仍做的防御

即使依赖领域不变量，当前 selector 仍检查：

- selector 是否配置；
- source 是否存在；
- awards 是否为空；
- totalWeight 是否为零；
- 单项 weight 是否等于 totalWeight；
- source ticket 是否越界；
- 扫描结束前是否找到 Award。

这些检查不是重新实现完整 Strategy 校验。

它们保护的是：

- Go 零值；
- 错误的包内组装；
- future refactor 引入的派生字段漂移；
- 第三方 source 违反接口语义；
- 理论上不可达的映射空洞。

### 3.3 selector 明确不重复检查的事实

当前 `Select` 不逐次重新校验：

- StrategyID；
- Strategy 名称；
- Award 名称；
- AwardID 唯一；
- Award Outcome；
- 每个 Award Weight 非零；
- totalWeight 是否重新求和完全一致，多候选只通过映射空洞间接发现部分问题。

原因是 Strategy 是不可变领域值。

每次选择都调用 `RestoreStrategy` 会产生额外 O(n) 校验、复制和排序。

那会让领域类型失去“构造后可信”的价值。

代价是：

- 包内代码必须保持纪律；
- adapter 必须使用构造器/Restore；
- 不能未来把字段导出后仍沿用这项信任；
- `unsafe` 或反射篡改不在普通正确性保证内。

### 3.4 成功不变量

对合法多候选 Strategy 和符合契约的 source：

1. source 接收到的 upper 必须等于 TotalWeight；
2. source 返回值必须位于 `[0,upper)`；
3. selector 恰好返回一个 Award；
4. 返回 Award 必须来自该 Strategy；
5. 返回 Award 的完整 ID/Name/Weight/Outcome 保持不变；
6. selector 不修改 Strategy；
7. selector 不修改 Award；
8. selector 不调用 Repository；
9. selector 不持久化结果；
10. selector 不解释 reward 内容。

### 3.5 失败不变量

发生任一失败时：

- 返回零值 Award；
- 返回非 nil error；
- error 可用 `errors.Is` 匹配稳定类别；
- `Error()` 不包含底层熵错误详情；
- source failure 不转换成 no_reward；
- contract violation 不用 `%` 修复；
- invariant violation 不跳过坏 bucket；
- selector 不自动换 source；
- selector 不偷偷重试业务选择；
- selector 不产生数据库、缓存、库存、发奖或消息等业务持久化副作用；source 调用仍可能推进内部状态、读取随机能力、阻塞或在失败前消耗字节，因此它不是引用透明纯函数。

### 3.6 概率不变量

设：

```text
T = Σ wi
```

对每个 Award i：

```text
wi > 0
0 < T <= MaxUint64
```

若 source 在 `[0,T)` 上均匀，则：

```text
P(select Award_i) = wi / T
```

这个命题是条件命题。

selector 无法仅凭接口类型证明 source 真正均匀。

### 3.7 顺序不变量

候选按 AwardID 升序排列。

它决定：

- ticket 对应哪个具体 Award；
- 确定性测试怎样构造边界；
- 同一 Strategy 快照怎样复现映射。

它不决定：

- 哪个 Award 概率更大；
- 哪个 Award 应优先展示；
- 哪个 Award 业务价值更高；
- 哪个 Award 更早发放。

按权重重新排序虽然可能降低某些分布下的平均扫描长度，却会破坏规范映射。

当前实现拒绝用隐藏顺序优化换取语义漂移。

### 3.8 no_reward 不变量

`AwardOutcomeNoReward` 是完整 Award。

它有：

- AwardID；
- 名称；
- 正权重；
- 明确 Outcome。

选中 no_reward 时：

```text
Award != zero value
error == nil
Award.HasReward() == false
```

调用方不能因为没有待发权益就重抽。

### 3.9 单候选不变量

当 Strategy 只有一个 Award：

```text
P(only Award) = 1
```

结果与随机 ticket 无关。

当前实现还要求：

```text
uint64(onlyAward.weight) == strategy.totalWeight
```

若不相等，则说明内部派生事实漂移。

此时返回 invariant violation，而不是信任其中任意一个值。

### 3.10 并发不变量

selector 没有内部计数器、种子、缓存表或可变 slice。

因此 selector 的并发安全是条件性的：

```text
Strategy 不可变
AND
BoundedRandomSource 可并发安全
→ WeightedSelector 可并发安全
```

若注入一个有数据竞争的 source，selector 不会自动使它安全。

---

## 4. 数学模型与正确性证明：区间长度，而不是感觉，决定概率

### 4.1 离散区间模型

设规范顺序中的 Award 权重为：

```text
w1, w2, ..., wn
```

总权重为：

```text
T = w1 + w2 + ... + wn
```

定义前缀：

```text
p0 = 0
p1 = w1
p2 = w1 + w2
...
pn = T
```

Award i 的区间是：

```text
[p(i-1), pi)
```

### 4.2 区间不重叠

相邻区间分别为：

```text
[p(i-1), pi)
[pi, p(i+1))
```

第一个不包含 `pi`。

第二个包含 `pi`。

所以边界点只属于后一个区间。

代码中的严格 `<` 正是这个半开区间定义。

若使用 `<=`，相邻区间会在边界重叠。

### 4.3 区间无空洞

因为每个权重都大于零：

```text
p0 < p1 < ... < pn
```

第一段从 0 开始。

最后一段在 T 结束。

所有相邻段首尾相接。

所以并集恰好是 `[0,T)`。

### 4.4 恰好选择一个 Award

任何合法 ticket 满足：

```text
0 <= ticket < T
```

由于区间覆盖整个 `[0,T)`，ticket 至少属于一个区间。

由于区间互不重叠，ticket 至多属于一个区间。

所以 ticket 恰好属于一个区间。

因此返回恰好一个 Award。

### 4.5 权重比例证明

Award i 区间包含的整数点数量为：

```text
pi - p(i-1) = wi
```

全体合法 ticket 数量为 T。

若 ticket 均匀，则每个点概率为 `1/T`。

所以：

```text
P(Award_i)
= 区间点数 × 单点概率
= wi × 1/T
= wi/T
```

### 4.6 剩余量减法与前缀区间等价

当前实现没有显式保存前缀数组。

它执行：

```text
remaining = ticket

若 remaining < w1，返回 Award1
否则 remaining -= w1

若 remaining < w2，返回 Award2
否则 remaining -= w2
...
```

进入 Award i 前：

```text
remaining = ticket - p(i-1)
```

判断：

```text
remaining < wi
```

等价于：

```text
ticket - p(i-1) < pi - p(i-1)
```

即：

```text
ticket < pi
```

同时能进入这一项说明 ticket 已不在前面的区间。

所以减法扫描与前缀区间完全等价。

### 4.7 为什么比例相同的配置分布相同

配置：

```text
[1, 3]
```

对应概率：

```text
[1/4, 3/4]
```

配置：

```text
[100, 300]
```

对应概率：

```text
[100/400, 300/400]
= [1/4, 3/4]
```

所以相对权重比例相同，理论分布相同。

但具体 ticket 空间不同。

同一个整数 ticket 在两套配置中的位置含义也不同。

因此“分布等价”不等于“随机 ticket 可跨配置重放”。

### 4.8 有限样本为什么不会严格等于权重比例

数学证明描述的是理论概率。

真实抽样的有限序列可能出现：

- 连续多次中奖；
- 连续多次 no_reward；
- 短窗口比例明显偏离；
- 稀有 Award 长时间不出现。

这些现象本身不能直接证明算法有 bug。

同样，某个短窗口“看起来很接近配置比例”也不能证明 source 安全或无偏。

### 4.9 本节证明的条件性

证明成立需要三个前提：

1. Strategy 权重事实正确；
2. source 对 `[0,T)` 真正均匀；
3. selector 按已证明区间映射。

当前单元测试强力覆盖第 3 项。

领域构造器覆盖第 1 项。

第 2 项主要依赖标准库与系统熵信任边界。

小样本测试无法重新认证 CSPRNG。

---

## 5. 候选算法矩阵：复杂度不是唯一决策变量

### 5.1 评估维度

算法不能只比较 Big-O。

本节至少需要比较：

- 是否精确保留整数权重；
- 是否支持 `MaxUint64`；
- 是否需要浮点；
- 是否需要预处理；
- 单次选择复杂度；
- 额外内存；
- 是否适合不可变 Strategy；
- 是否要求 Strategy 被多次复用；
- 是否容易确定性测试；
- 是否容易安全审查；
- 是否会把算法结构带入缓存/持久化；
- 失败时是否能清晰分类。

### 5.2 总体矩阵

| 方案 | 预处理 | 单次选择 | 内存 | 精确整数 | MaxUint64 | 当前结论 |
| --- | ---: | ---: | ---: | --- | --- | --- |
| 剩余权重线性扫描 | 无 | O(n) | O(1) | 是 | 是 | 采用 |
| 前缀和 + 线性扫描 | O(n) 或边走边算 | O(n) | O(n) 或 O(1) | 是 | 是 | 无收益 |
| 前缀和 + 二分 | O(n) | O(log n) | O(n) | 是 | 是 | 暂缓 |
| Alias Method | O(n) | O(1) | O(n) | 常见实现否 | 需额外证明 | 不采用 |
| Fenwick Tree | O(n) | O(log n) | O(n) | 是 | 可实现 | 无动态更新需求 |
| Segment Tree | O(n) | O(log n) | O(n) | 是 | 可实现 | 复杂度过高 |
| 展开权重数组 | O(T) | O(1) | O(T) | 是 | 不可行 | 禁止 |
| float CDF | O(n) | O(n)/O(log n) | O(n) | 否 | 否 | 禁止 |
| `raw % total` | 无 | O(1) + 扫描 | O(1) | 有模偏差 | 表面支持 | 禁止 |
| SQL `ORDER BY RAND()` | 数据库执行 | 不透明 | 数据库成本 | 不表达当前权重 | 不清晰 | 禁止 |
| request hash `% total` | 无 | O(1) + 扫描 | O(1) | 有偏/可操控风险 | 表面支持 | 禁止 |

### 5.3 为什么当前采用线性扫描

当前没有真实 Award 数量分布。

当前没有 Strategy 缓存。

当前没有运行时 Draw API。

当前 Repository 每次读取完整聚合。

当前 Strategy 是不可变值。

在这些事实下，线性扫描的优势是：

- 零预处理；
- 零额外表；
- 零分配实现；
- 完整数值范围；
- 证明简单；
- 错误路径短；
- 不引入缓存一致性；
- 不引入算法版本。

### 5.4 线性扫描的真实成本

最坏比较次数是 n。

平均比较次数不是固定 `n/2`。

若 Award i 在规范顺序中的位置是 i，则：

```text
E[comparisons] = Σ (i × wi/T)
```

若大权重 Award 恰好排在末尾，平均扫描可能接近 n。

若大权重 Award 排在开头，平均扫描会更小。

当前不按权重排序来优化平均比较次数。

因为 AwardID 顺序是已建立的规范语义。

### 5.5 为什么不按权重降序扫描

按权重降序可能减少某些分布的平均扫描。

但它会带来：

- 相同权重时新的 tie-breaker；
- 权重修改导致区间重排；
- 同一 ticket 映射改变；
- domain canonical order 与 selection order 分裂；
- 测试、缓存和审计需要解释两个顺序。

当前没有 profile 证明这项复杂度值得。

所以不采用。

### 5.6 前缀和 + 二分何时更合适

前缀和数组形如：

```text
[w1, w1+w2, ..., T]
```

对 ticket 查找第一个 `prefix > ticket`。

优点：

- 单次 O(log n)；
- 仍可使用精确整数；
- 边界语义清晰；
- 可构造成不可变表并并发共享。

成本：

- 每个 Strategy 需要 O(n) 构建；
- 需要 O(n) 额外内存；
- 若每次从 Repository 读取后只选一次，预处理不能摊销；
- 表必须绑定精确 Strategy 快照；
- Future Update/Cache 需要版本与失效协议。

重评前缀方案的必要信号是：

- Strategy 被缓存并高频复用；
- Award 数量分布显著上升；
- profile 证明扫描占用可见 CPU；
- benchmark 比较包含构建成本与复用次数。

### 5.7 Alias Method 为什么当前不合适

Alias Method 的吸引力是：

- O(n) 构建；
- O(1) 抽样。

但常见实现会归一化：

```text
scaled_i = wi × n / T
```

这立即提出：

- `wi × n` 是否溢出 `uint64`；
- 使用 float 是否损失精度；
- 使用大整数是否增加构建成本；
- 两次随机选择怎样保持无偏；
- alias 表怎样绑定 Strategy 版本；
- 表的序列化格式是否成为兼容契约；
- 更新时怎样原子切换表。

当前没有证据需要为 O(1) 付出这些成本。

### 5.8 Fenwick/Segment Tree 为什么不是“更高级就更好”

这些结构适合：

- 权重频繁更新；
- 动态增删候选；
- 同一结构上大量查询。

当前 Strategy 没有 Update。

将动态树加入不可变聚合，会为了未来猜测引入：

- 双状态同步；
- 锁或 copy-on-write；
- 复杂构造；
- 更多 invariant；
- 更难的持久化/缓存一致性。

所以暂不实现。

### 5.9 展开权重数组为什么不可接受

一种教学式做法是：

```text
weight 3 → 把 Award 放进数组 3 次
```

这样可直接随机索引。

但当前 Weight 允许 MaxUint64。

空间复杂度 O(T) 在设计上已经不可行。

即使业务常见总权重较小，也不能让一个合法大值触发内存灾难。

### 5.10 浮点 CDF 为什么违反领域契约

`float64` 只有 53 位有效整数精度。

当前 total 可以达到 64 位无符号最大值。

把权重转 float 会导致：

- 小权重在大总和下不可区分；
- 边界取整漂移；
- 某些 Award 理论上正权重却可能失去可达点；
- Go、JavaScript、数据库显示不一致。

所以不使用浮点参与选择。

### 5.11 raw modulo 为什么有偏

设原始 source 均匀返回 `2^64` 个值。

若 T 不能整除 `2^64`，执行：

```text
raw % T
```

会使某些余数比其他余数多一个原像。

所以输出不完全均匀。

偏差可能很小，但“很小”不是“没有”。

本节已经有标准库拒绝采样，不需要接受该偏差。

### 5.12 SQL 随机为什么不属于当前边界

将选择放进 SQL 会：

- 把随机源交给数据库；
- 把权重映射与表结构绑定；
- 难以注入确定 ticket；
- 难以做全范围边界测试；
- 增加数据库 CPU；
- 让缓存与算法产生两套结果；
- 模糊 Repository 与 domain 责任。

所以不采用。

---

## 6. 随机源威胁模型：均匀、不可预测、可审计是三件不同的事

### 6.1 为什么随机源是一个端口

算法需要的最小能力不是：

- 一个 seed；
- 一个 `math/rand.Rand`；
- 一个字节 reader；
- 一个操作系统设备路径；
- 一个具体密码库。

算法真正需要的是：

> 对任意正 `upper`，返回一个在 `[0,upper)` 上均匀分布的 `uint64`，或者返回错误。

所以 [`BoundedRandomSource`](../../../internal/lottery/domain/weighted_selector.go) 只定义：

```go
Uint64N(upper uint64) (uint64, error)
```

接口没有暴露实现细节。

### 6.2 接口名字为什么包含 Bounded

如果接口只叫 `RandomSource`，调用者仍不知道：

- 返回 raw 64 bit 还是已缩减值；
- 上界是否包含；
- 是否可能返回负数；
- 是否允许 upper 为零；
- 是否负责消除模偏差。

`BoundedRandomSource` 把最关键的范围语义写进了端口。

注释进一步明确：

- 半开区间；
- upper 必须非零；
- 不得用 modulo 修复越界；
- 共享时 source 自己必须并发安全。

### 6.3 接口无法证明均匀性

Go interface 只能证明方法集合。

任何实现都可以：

- 永远返回 0；
- 偏向某些数字；
- 使用时间戳；
- 使用用户可预测 seed；
- 返回范围内但被操控的值。

这些实现仍然满足编译期接口。

所以“均匀”是语义契约和安全信任，不是类型系统保证。

当前缓解方式是：

- 仓库目前只实现经过审查的 CryptoSource 生产候选，但尚未装配进产品进程；
- 测试 fake 只用于确定性边界；
- selector 检查范围，但不伪装能检测统计偏差；
- composition root 未来必须明确装配生产 adapter；
- `NewCryptoSource` 捕获的是可被进程内代码替换的全局 `crypto/rand.Reader`，当前仓库没有重写它，但类型白名单不能替代初始化顺序、依赖与供应链审查。

### 6.4 威胁主体

至少需要考虑四类主体：

| 主体 | 可能能力 | 当前 CSPRNG 能缓解什么 | 不能缓解什么 |
| --- | --- | --- | --- |
| 普通用户 | 反复请求、选择时机、观察结果 | 降低预测下一 ticket 的能力 | 不能阻止重复请求/grinding |
| 恶意客户端 | 构造 request ID、并发轰击、重放 | 随机值不由请求 ID 直接决定 | 不能提供幂等、限流、资格 |
| 运营人员 | 配置权重、选择发布时间 | 不让运营直接预测每次 entropy | 不能阻止恶意配置或选择性发布 |
| 被入侵服务 | 替换依赖、记录内部数据 | 标准 source 默认更强 | 进程被控制后无法保证公平 |

### 6.5 为什么营销抽奖也属于安全敏感场景

只要 reward 具有经济价值，就存在：

- 券；
- 积分；
- 返利；
- 实物；
- 活动资格；
- 游戏资产。

可预测 PRNG 可能让攻击者：

- 观察若干输出后推断状态；
- 选择请求时机；
- 并发试探；
- 在多个账户间协调请求；
- 只保留有利结果。

所以当前默认选择 CSPRNG，而不是把 `math/rand/v2` 当生产随机源。

### 6.6 `math/rand/v2` 的准确边界

Go 官方 [`math/rand/v2`](https://pkg.go.dev/math/rand/v2) 说明：

- 它适用于模拟等任务；
- 不应被用于安全敏感工作；
- 顶层函数并发安全；
- 自建 `Source` / `Rand` 默认不是多 goroutine 安全；
- `Uint64N` 提供 `[0,n)` 范围。

所以不能把两个问题混成一个：

```text
Uint64N 无模偏差
≠
输出不可预测
```

一个 PRNG 可以范围映射正确，却仍然可预测。

### 6.7 为什么选择 `crypto/rand.Int`

Go 官方 [`crypto/rand.Int`](https://pkg.go.dev/crypto/rand#Int) 明确返回 `[0,max)` 上的均匀值。

它内部使用拒绝采样。

当前 adapter 因而不需要手写：

- raw uint64 读取；
- bit mask；
- threshold 计算；
- modulo rejection；
- Lemire multiply-high；
- 重试循环。

减少自写密码学边界代码，本身就是风险控制。

### 6.8 为什么不使用 `crypto/rand.Read` 后 `% upper`

即使 raw bytes 来自 CSPRNG，执行 `% upper` 仍可能产生模偏差。

“输入安全”不自动推出“范围缩减无偏”。

所以本节使用 `rand.Int` 的 bounded uniform 契约。

### 6.9 Go 1.26 的 `crypto/rand.Read` 注意点

当前工程 Go 版本是 1.26.6。

该版本官方文档说明 `crypto/rand.Read` 会填满 buffer，并在默认随机能力出现异常时采用不可恢复失败语义。默认 `crypto/rand.Reader` 的读取不会把底层失败作为普通 error 返回；这类失败不会进入 `SourceError`，HTTP panic recovery 也无法把 runtime fatal 变成业务 5xx。

领域端口仍需要显式 `error`，因为测试注入 reader、未来远程/HSM adapter 或其他 `BoundedRandomSource` 可以合法返回错误。因此 adapter 调用 `crypto/rand.Int(reader, maximum)` 并保留可返回 reader 的 error 通道，但不能把这条通道写成 Go 1.26 默认 OS CSPRNG 的故障监控机制。

这项行为必须在 Go 版本升级时复核，不能仅凭旧版本记忆判断。

### 6.10 为什么 source error 不降级

若 entropy source 失败后改用：

- 当前时间；
- request ID hash；
- process PID；
- 固定 seed；
- `math/rand` 默认实例；
- 永远 no_reward；

系统会把技术故障变成：

- 可预测结果；
- 有偏结果；
- 对用户不利的静默降级；
- 无法审计的策略变化。

当前选择是失败关闭。

### 6.11 不可预测不等于可审计

CSPRNG 提供的核心价值是降低预测能力。

它不自动提供：

- 公开验证；
- 第三方重放；
- 操作员不可篡改；
- seed 承诺；
- 结果不可抵赖；
- 配置版本审计。

未来若需要这些能力，可能评估：

- commit-reveal；
- VRF；
- 带密钥 PRF；
- 第三方随机信标；
- HSM/审计密钥；
- 不可变结果日志。

这些都不是本节 `CryptoSource` 已经实现的能力。

### 6.12 不可预测不等于公平治理

即使随机数完美，操作者仍可能：

- 配置极低中奖权重；
- 创建全部 no_reward 策略；
- 只向部分用户开放；
- 在不利窗口切换策略；
- 丢弃某些结果；
- 重复执行直到得到想要结果。

公平治理需要：

- 配置审批；
- Strategy 版本；
- 生效时间；
- 权限；
- 审计；
- Draw 唯一事实；
- 结果不可选择性丢弃。

### 6.13 NIST 资料怎样被正确引用

[NIST SP 800-90A Rev.1](https://csrc.nist.gov/pubs/sp/800/90/a/r1/final) 讨论 DRBG 机制。

[NIST SP 800-90B](https://csrc.nist.gov/pubs/sp/800/90/b/final) 讨论 entropy source 设计与验证。

它们可以帮助理解：

- entropy source；
- deterministic generator；
- health testing；
- 预测阻力。

但引用 NIST 文档不等于当前应用通过 NIST/FIPS 认证。

当前项目没有形成任何认证结论。

### 6.14 为什么不把 seed 暴露为 API 参数

允许客户端提交 seed 会使客户端能够：

- 离线枚举；
- 选择有利 seed；
- 重放预测序列；
- 影响其他调用结果。

测试确定性应通过进程内依赖注入实现。

不能把测试 seam 变成生产攻击面。

### 6.15 当前 source 测试能证明什么

[`crypto_test.go`](../../../internal/lottery/adapter/randomsource/crypto_test.go) 证明：

- nil receiver 被拒绝；
- nil reader 被拒绝；
- zero upper 被拒绝；
- reader error cause 被保留但不公开；
- MaxUint64 上界可用；
- 上界 3 时候选 3 被拒绝，随后候选 2 被接受；
- 默认 source 在多个上界下始终返回范围内值；
- 默认 Reader 的并发调用在 race 测试窗口内工作；
- 默认 source 与 selector 可并发组合。

它不能证明：

- 操作系统 entropy 永远健康；
- 生产容器/FIPS 配置与本机相同；
- 长期输出通过所有统计检验；
- 第三方无法控制进程；
- 系统满足某项监管认证。

---

## 7. 单候选短路：为什么“不读取随机源”不是偷懒

### 7.1 数学推导

若 Strategy 只有一个 Award，权重为 w：

```text
T = w
P(only Award) = w/T = 1
```

无论 ticket 是多少，只要它在 `[0,T)`，都落入唯一 Award 区间。

随机数不能改变输出。

所以随机选择在信息论上没有贡献。

### 7.2 当前短路流程

当前代码先检查：

```text
len(awards) == 1
```

然后检查：

```text
uint64(award.weight) == strategy.totalWeight
```

一致时直接返回 Award。

不调用 source。

不一致时返回 `ErrSelectionInvariantViolation`。

### 7.3 为什么仍校验派生总权重

若代码直接返回唯一 Award，而内部出现：

```text
award.weight = 1
strategy.totalWeight = 2
```

它会掩盖聚合内部状态漂移。

虽然正常构造器不允许这种状态，但：

- 包内 future refactor；
- 手写测试 fixture；
- 非法 hydration；
- unsafe 操作；

可能制造它。

短路前校验让错误不会被“反正只有一个”掩盖。

### 7.4 可用性收益

单候选可能用于：

- 保底奖励；
- 全量发放；
- 维护期固定结果；
- 灰度验证；
- 纯 no_reward 安全配置；
- 测试 fixture。

这些场景没有随机决策。

熵边界短暂不可用时，强制依赖它只会增加无业务价值的失败。

### 7.5 短路不是随机 fallback

随机 fallback 是：

```text
本来有多个可能结果
→ source 失败
→ 改用另一个机制决定结果
```

单候选短路是：

```text
从一开始就只有一个可能结果
→ 不需要随机能力
```

两者语义完全不同。

### 7.6 短路对监控的影响

单候选不会触发 source。

所以：

- 单候选成功不能证明 entropy source 健康；
- 随机源调用计数可能小于选择次数；
- source error rate 的分母不能直接使用全部 selection；
- readiness 不应因单候选成功而宣称随机能力已验证。

### 7.7 为什么不能对“全部相同 Outcome”短路

多个 Award 即使都是 `no_reward`，仍可能拥有不同：

- AwardID；
- 名称；
- Weight；
- 后续审计含义。

选择哪个 Award 仍是有意义的配置结果。

所以不能因为 `Outcome` 相同就返回第一项。

### 7.8 为什么不能对“相同名称”短路

名称不是身份。

同名 Award 可以有不同 ID 与后续 Reward 引用。

所以名称相同不意味着结果相同。

### 7.9 单候选 MaxUint64

当前测试覆盖 Weight 为：

- 1；
- 400；
- `math.MaxUint64`。

三者都不调用 source。

这直接证明：

- 短路不依赖总权重大小；
- source failure 不会影响确定结果。

整个实现不计算 `total+1`、不做 signed conversion，则由源码审查、CryptoSource 最大上界和多 bucket MaxUint64 边界测试共同证明，不能只归因于这条未进入随机/扫描路径的单候选测试。

### 7.10 单候选短路的重评条件

如果未来业务要求每次 Draw 都保存：

- random commitment；
- entropy receipt；
- public proof；
- 统一随机审计链；

即使单候选，也可能需要调用审计随机协议。

那是新的合规/审计需求。

当前不为它提前牺牲简单性。

---

## 8. 数值、溢出与边界：`uint64` 契约必须贯穿到底

### 8.1 当前范围

Weight 类型是 `uint64`。

TotalWeight 类型是 `uint64`。

Strategy 允许：

```text
TotalWeight == math.MaxUint64
```

所以算法不能把“正常业务一般不大”当作缩窄类型的理由。

### 8.2 为什么区间选择用 `[0,T)`

半开区间允许：

- 第一个合法点是 0；
- 最后一个合法点是 T-1；
- 上界直接传给标准 bounded API；
- 不需要计算 T+1；
- 相邻 bucket 用 `<` 自然表达。

若使用 `[1,T]`，常见实现容易：

- 生成 `[0,T)` 后再 `+1`；
- 在 T=MaxUint64 时溢出；
- 在边界比较中混用 `<` / `<=`。

### 8.3 为什么不能转 `int64`

当：

```text
T > math.MaxInt64
```

转成 `int64` 会得到负数或失真。

所以不能使用：

- `rand.Int63n(int64(T))`；
- `big.NewInt(int64(T))`；
- signed SQL/DTO 临时变量；
- `int(T)` 作为随机上界。

### 8.4 为什么 `SetUint64` 是关键

当前 CryptoSource 使用：

```go
new(big.Int).SetUint64(upper)
```

它精确表达整个 uint64 范围。

如果使用：

```go
big.NewInt(int64(upper))
```

MaxUint64 会在转换时失真。

### 8.5 为什么扫描使用减法

一种写法是：

```text
cumulative += weight
if ticket < cumulative { ... }
```

领域已经保证总和不溢出，因此它可以正确实现。

当前仍选择减法：

```text
if ticket < weight return
ticket -= weight
```

好处是：

- 局部没有累计加法；
- MaxUint64 边界更直观；
- 不需要证明每个中间 prefix；
- 与“剩余区间”心智模型一致。

### 8.6 减法为什么不会下溢

只有当：

```text
ticket >= weight
```

时才执行：

```text
ticket -= weight
```

所以每次减法都安全。

### 8.7 最后一个 ticket

最大合法 ticket 是：

```text
T - 1
```

当 T=MaxUint64：

```text
ticket = MaxUint64 - 1
```

它仍可由 uint64 表示。

当前测试覆盖该点落入最后 bucket。

### 8.8 越界 ticket

source 若返回：

```text
ticket == T
```

它违反半开区间契约。

当前 selector 不执行：

```text
ticket %= T
```

而是返回 `ErrRandomSourceContractViolation`。

修复越界会隐藏 adapter bug，并可能重新引入偏差。

### 8.9 最大范围拒绝采样

当：

```text
upper = MaxUint64
```

合法输出为：

```text
0 ... MaxUint64-1
```

注意这里共有 `MaxUint64` 个值，而不是 `2^64` 个值。

全 1 的 raw 64-bit 候选等于 MaxUint64，必须被拒绝。

当前 source 测试使用 `MaxUint64-1` 字节序列证明合法上边界可返回。

### 8.10 上界 3 的拒绝证据

测试 reader 依次提供：

```text
0x03
0x02
```

对 upper=3：

- 3 越界，被 `crypto/rand.Int` 拒绝；
- 2 合法，被返回；
- reader 最终耗尽。

这证明当前 adapter 没有简单取模：

```text
3 % 3 == 0
```

否则返回会错误地是 0。

### 8.11 scaled weights 的隐藏兼容问题

`[1,3]` 与 `[100,300]` 分布相同。

但：

- TotalWeight 不同；
- ticket 空间不同；
- 具体 ticket 映射不同；
- 配置审计 diff 不同；
- cache payload 不同。

未来若保存 ticket 以便重放，必须同时保存精确 Strategy 版本/快照。

只保存“概率比例”不足以恢复当时映射。

### 8.12 JavaScript 边界不在本节

Go 与 MySQL 已支持完整 uint64。

JavaScript `Number` 不能无损表达整个范围。

第 21/22 节必须决定：

- ID 是否用 JSON string；
- Weight 是否用 JSON string；
- UI 概率怎样计算；
- 是否使用 BigInt；
- OpenAPI 怎样表达。

本节不能通过偷偷缩小算法范围来回避 transport 问题。

---

## 9. 错误模型与失败关闭：错误不能变成另一份中奖概率

### 9.1 当前稳定错误类别

[`selection_error.go`](../../../internal/lottery/domain/selection_error.go) 定义：

| 类别 | 含义 | 是否有 Award |
| --- | --- | --- |
| `ErrSelectorNotConfigured` | composition 没有 source | 否 |
| `ErrSelectionStrategyInvalid` | 输入不是可用 Strategy | 否 |
| `ErrRandomSourceFailure` | entropy adapter 失败 | 否 |
| `ErrRandomSourceContractViolation` | source 返回越界值 | 否 |
| `ErrSelectionInvariantViolation` | 合法映射理论被破坏 | 否 |

### 9.2 为什么没有 `ErrNoReward`

no_reward 是正常 Award。

若把它变成 error：

- error rate 被业务结果污染；
- retry middleware 可能再次选择；
- 用户可从未中奖重试成中奖；
- 可观测性无法区分技术故障与业务概率。

### 9.3 为什么 source failure 返回零 Award

随机源失败时没有可信 ticket。

没有 ticket 就没有合法区间映射。

所以不能返回：

- 第一个 Award；
- 最大权重 Award；
- no_reward Award；
- 上一次 Award；
- 部分构造结果。

### 9.4 为什么 contract violation 是独立类别

source 返回 error 与 source 返回越界值不同：

- 前者是声明失败；
- 后者是声称成功却违反契约。

越界值通常意味着：

- adapter bug；
- fake 配置错误；
- source 被错误替换；
- 上界理解不一致。

它应比普通 entropy unavailable 更强烈地告警。

### 9.5 为什么 invariant violation 是独立类别

合法 Strategy + 合法 ticket 理论上必须命中。

若未命中，意味着：

- totalWeight 与成员和不一致；
- scan 逻辑被改坏；
- package 内部状态被绕过；
- future optimization 引入空洞。

这不是用户输入错误。

这通常是代码/数据完整性事故。

### 9.6 `SelectionError` 为什么不直接 `fmt.Errorf`

直接写：

```go
fmt.Errorf("%w: %v", ErrRandomSourceFailure, cause)
```

会让 `Error()` 包含 cause。

底层 cause 可能包含：

- reader 实现细节；
- 环境路径；
- 测试/供应商信息；
- future remote source endpoint。

当前 `SelectionError`：

- `Error()` 只返回稳定 class；
- `Is()` 支持语义匹配；
- `Unwrap()` 保留 cause 给可信诊断。

### 9.7 安全字符串不等于安全日志

即使顶层 `Error()` 脱敏，调用者仍可：

- 递归展开 cause；
- 使用 `%+v` 的第三方 formatter；
- 将底层错误单独记录；
- 把可信日志发送到不可信前端。

所以第 21 节仍需定义日志与响应边界。

### 9.8 typed nil 为什么被显式处理

Go interface 可能：

```text
动态类型非 nil
动态值为 nil pointer
interface != nil
```

如果构造器只判断 `source == nil`，typed nil 可能通过。

当前 `boundedRandomSourceIsNil` 使用 reflect 拒绝可 nil kind 的 nil 值。

反射只发生在构造阶段，不在每次选择热路径。

### 9.9 typed nil 检查的边界

该检查不能保证：

- 非 nil source 内部配置完整；
- source 方法不会 panic；
- source 真正均匀；
- source 并发安全；
- source 不会永久阻塞。

它只解决 interface nil 语义。

### 9.10 为什么 selector 不 recover source panic

当前没有 `recover`。

原因是：

- panic 通常表示 source 实现 bug；
- 热路径 recover 会掩盖程序错误；
- arbitrary panic 不一定可安全继续；
- 普通 source panic 可能到达 HTTP 的更高层 recovery，但这不是领域承诺；Go 1.26 默认随机能力的 runtime fatal 不可由该 recovery 捕获。

若未来 source 是远程插件，隔离方式应在 adapter/process 边界设计，不应由 domain 猜测恢复。

### 9.11 自动重试为什么不在 selector

随机 source error 后重试在数学上可能取得新 ticket。

但 selector 不知道：

- 调用是否已绑定 DrawID；
- 上层是否已经记录部分状态；
- retry budget；
- request deadline；
- 安全策略是否允许切换 source。

所以当前一次调用 source 后直接返回 error。

### 9.12 error 到 HTTP 的映射尚未决定

未来可能映射：

- 配置错误；
- 服务暂不可用；
- 内部错误；
- dependency failure。

但当前没有 HTTP API。

本节错误类别没有 status code、JSON code 或用户文案。

### 9.13 error 与 DrawResult 的关系尚未存在

未来若选择结果已经落库后响应失败，客户端看到的可能是“结果未知”。

当前 selector 没有持久化副作用。

所以 selection error 只表示函数没有返回可信 Award。

不能把它与 Repository 的 `ErrCommitOutcomeUnknown` 混为一谈。

---

## 10. 并发、所有权与确定性：无共享状态不等于业务 exactly-once

### 10.1 selector 的状态

`WeightedSelector` 只保存一个 interface：

```go
source BoundedRandomSource
```

它不保存：

- 上一次 ticket；
- PRNG seed；
- 统计计数；
- Strategy cache；
- prefix table；
- Award 指针；
- 用户状态。

### 10.2 Strategy 的不可变性

Strategy 构造时复制 awards。

字段不导出。

Award 没有 setter。

selector 只读私有 slice。

所以多个 goroutine 可读同一 Strategy。

### 10.3 source 决定条件性并发安全

当前注释明确：

> selector 在 source 可并发安全时可并发安全。

默认 `crypto/rand.Reader` 官方声明可并发安全。

所以 `NewCryptoSource()` 可共享。

测试中的 `recordingSource` 有可变计数与 slice，不适合无锁共享。

并发测试使用无状态 `constantSource` 或生产 CryptoSource。

### 10.4 为什么 selector 不加 mutex

若 selector 无条件给 source 加锁：

- 所有选择被串行化；
- 隐藏 source 自身错误契约；
- crypto Reader 已有并发保证却仍受限；
- future shard/source pool 无法利用并行；
- mutex 成为全局热点。

正确做法是让共享 source 明确提供并发安全。

### 10.5 race 测试当前覆盖

当前测试执行：

- 32 workers；
- selector + constant safe source，每 worker 128 次；
- CryptoSource 自身 32 workers × 64 次；
- CryptoSource + WeightedSelector 32 workers × 64 次；
- targeted packages 的 `go test -race`。

这可以发现测试路径中的普通数据竞争。

它不能证明：

- 任意第三方 source 安全；
- 生产长时间无 race；
- future metrics/cache 不引入共享状态；
- 多进程业务结果唯一。

### 10.6 并发安全与独立性

默认 CSPRNG 的并发调用可以产生多个随机结果。

“没有 data race”只说明内存访问同步正确。

它不说明：

- 每个用户只能调用一次；
- 两个重复请求共享结果；
- 只有一个 goroutine 成功；
- 库存只扣一次；
- Award 结果有顺序保证。

### 10.7 并发调用不是 exactly-once

两个 goroutine 同时执行：

```go
selector.Select(strategy)
```

当前语义是执行两次选择，没有共享 Draw 身份或结果状态。端口只承诺每次调用的边际均匀性，不承诺跨调用统计独立。

两次都可能成功。

两次可能返回不同 Award。

不存在唯一约束阻止它们。

### 10.8 同一 source 序列与调度

若未来注入一个有内部序列的并发 PRNG：

- 每个值仍可能合法；
- 但哪个 goroutine 获得哪个值取决于调度；
- 测试不能假设 goroutine 与序列位置固定。

需要可复现并发结果时，应在更高层分配请求身份和确定协议。

### 10.9 context 为什么没有进入 selector

当前选择只有：

- 本地字段读取；
- 一次 bounded source 调用；
- 线性内存扫描。

`crypto/rand.Int` 接受 reader，不接受 context。

给 `Select` 增加 context 但无法中断 reader，会造成虚假取消承诺。

第 21 节 application use case 仍应接收 context，用于：

- Repository；
- 结果持久化；
- 外部依赖；
- 请求生命周期。

### 10.10 source 永久阻塞的隐藏风险

接口允许某个实现永久阻塞。

当前 selector 无法取消它。

默认 OS CSPRNG 的运行特性使当前风险可接受。

若未来换成：

- 远程 RNG；
- HSM 网络调用；
- sidecar；
- 外部 beacon；

必须重新设计 context、timeout、bulkhead 与降级语义。

### 10.11 规范顺序与确定性

对同一 Strategy 和同一 ticket，映射确定。

这依赖：

- Awards 的 ID 规范排序；
- 权重不变；
- 半开区间不变；
- 扫描算法不变。

若未来保存 ticket 用于重放，这四项都会成为兼容性事实。

### 10.12 当前没有承诺跨版本随机序列稳定

CryptoSource 输出不承诺：

- 跨 Go 版本相同；
- 跨操作系统相同；
- 跨进程相同；
- 可通过 seed 重放。

当前需求只需要不可预测的单次选择。

因此不应把输出序列写成 golden test。

---

## 11. 测试策略与证据认识论：概率代码首先要做确定性证明

### 11.1 为什么不能只“随机跑很多次”

假设测试运行十万次，观察比例接近期望。

即使通过，也不能排除：

- 第一个边界 off-by-one；
- 最后一个 ticket 丢失；
- 极小权重不可达；
- MaxUint64 溢出；
- source error 被吞；
- no_reward 被错误重试；
- 特定 total 下的模偏差；
- typed nil panic。

统计接近只是一种弱信号。

### 11.2 为什么 scripted source 是关键测试 seam

测试 source 可以精确返回：

- 0；
- bucket 左边界；
- bucket 右边界前一位；
- `total-1`；
- `total`；
- MaxUint64 附近；
- 一个 error。

因此每条逻辑分支都可确定重放。

### 11.3 全 ticket 穷举比随机样本更强

对小总权重 T，可以枚举：

```text
0, 1, 2, ..., T-1
```

然后统计每个 Award 的命中次数。

若每个命中次数恰好等于 Weight，则直接证明：

- 没有空洞；
- 没有重叠；
- 区间长度正确；
- scaled weights 的比例语义正确。

当前测试覆盖：

- `[1,3]`；
- `[100,300]`；
- `[2,3,5]`。

### 11.4 规范顺序映射测试

测试输入 Awards 的顺序是：

```text
ID 30, ID 10, ID 20
```

Strategy 构造后按 ID 排为：

```text
10(weight 2)
20(weight 3)
30(weight 5)
```

测试逐个 ticket 断言：

```text
0..1   → 10
2..4   → 20
5..9   → 30
```

这同时验证：

- 输入顺序不成为隐藏概率状态；
- source 每次只调用一次；
- upper 精确为 10；
- no_reward bucket 不被跳过。

### 11.5 单候选测试

测试为 source 配置一个“若被调用就失败”的 error。

对权重：

- 1；
- 400；
- MaxUint64；

selector 都成功返回唯一 Award。

并断言 source 调用次数为 0。

这证明短路是可观察契约，不只是优化猜测。

### 11.6 no_reward 测试

测试构造 reward 与 no_reward 各一个候选。

ticket 落入 no_reward 区间。

断言：

- 返回 no_reward Award；
- error 为 nil；
- `HasReward()` 为 false。

它试图证伪“未中奖被当成技术失败”。

### 11.7 MaxUint64 多 bucket 测试

当前测试不只验证单候选 MaxUint64。

它还验证多候选总和等于 MaxUint64：

```text
[1, MaxUint64-1]
```

以及：

```text
[MaxUint64-2, 1, 1]
```

覆盖：

- 第一个 bucket；
- 第一个 bucket 最末点；
- 倒数第二 bucket；
- 最后 bucket；
- `MaxUint64-1` ticket。

### 11.8 失败关闭表驱动测试

当前表覆盖：

- zero selector；
- zero Strategy；
- source failure；
- source 返回 upper。

每项都断言：

- `errors.Is` 匹配类别；
- 需要时 cause 可展开；
- Award 为零值；
- source 调用次数符合预期；
- `Error()` 只等于安全 class 文本。

### 11.9 包内非法状态测试的价值

正常包外代码无法设置 Strategy 私有字段。

同包测试故意构造：

```text
实际 bucket 和 = 2
declared total = 3
```

ticket=2 无法映射。

selector 返回 invariant violation。

这不是模拟正常用户输入。

它是在验证 future refactor 破坏派生不变量时是否失败关闭。

### 11.10 单候选非法状态测试

测试还构造：

```text
only weight = 1
declared total = 2
```

selector 在调用 source 前拒绝。

这证明短路不会掩盖内部漂移。

### 11.11 typed nil 测试

测试声明：

```go
var source *recordingSource
```

再把它作为 interface 传入构造器。

构造器返回 `ErrSelectorNotConfigured`。

这证明检查覆盖 Go interface typed nil 陷阱。

### 11.12 CryptoSource 拒绝采样测试

上界 3 的字节序列测试是确定性的。

它比“跑一万次看频率”更直接地证明：

- 超界候选被拒绝；
- source 继续读取；
- 没有 `% upper` 修复。

### 11.13 默认 source 范围测试的边界

测试对若干 upper 每个取 64 次：

```text
1
2
3
7
256
2^32+1
MaxUint64
```

它证明测试窗口内所有返回值均在范围内。

它不证明每个值出现频率完全均匀。

### 11.14 为什么没有以真实 CSPRNG 频率卡 CI

真实随机样本存在统计波动。

若 CI 用过窄阈值：

- 正确实现会偶发失败；
- 团队会放宽到失去意义；
- flaky test 会被重跑掩盖；
- 无法精确定位边界 bug。

所以当前主要依赖：

- 标准库契约；
- 确定性 rejection 测试；
- 精确区间穷举。

### 11.15 统计测试未来怎样使用

统计测试可作为：

- 非阻塞诊断；
- 长时间 soak 的异常检测；
- adapter 被错误替换的粗筛；
- 生产聚合分布巡检。

使用时需要明确：

- 样本量；
- 置信水平；
- 多重检验；
- 极小概率 bucket；
- Strategy 版本窗口；
- 业务流量选择偏差。

### 11.16 fuzz/property 测试的未来价值

当前还没有长期 fuzz corpus。

可以未来增加性质：

- 任意合法小权重集合；
- 任意 `ticket < total`；
- 返回 Award 必须来自集合；
- 每个点恰好映射；
- `ticket == total` 必须失败；
- selector 不修改 Strategy。

但 fuzz 不能替代 MaxUint64 的手写边界。

### 11.17 race 检测的准确结论

定向命令：

```bash
go test -race ./internal/lottery/domain ./internal/lottery/adapter/randomsource
```

在当前代码与测试调度窗口中通过。

它能说明：

- 已执行路径没有被 race detector 捕获的数据竞争。

它不能说明：

- 所有调度都被覆盖；
- future source 都安全；
- 跨进程业务不会重复。

### 11.18 测试通过不等于线上公平

当前测试不包含：

- 真实用户请求；
- 恶意重放；
- 配置审批；
- Strategy 热更新；
- Draw 结果表；
- 库存；
- 多实例；
- 生产 entropy 环境；
- 合规评审。

因此“测试通过”不能被写成“抽奖公平上线”。

---

## 12. 性能证据：先分解成本，再拒绝把 benchmark 外推成 SLO

### 12.1 为什么要做 benchmark

算法方案比较包含复杂度判断。

没有 benchmark 时，容易出现两种相反错误：

- 凭感觉提前上 Alias；
- 凭感觉认为线性扫描永远足够。

当前 benchmark 的目标是建立本机基线，不是证明业务容量。

### 12.2 当前纯扫描 benchmark 形状

[`BenchmarkWeightedSelectorWorstCase`](../../../internal/lottery/domain/weighted_selector_test.go) 使用：

- 每个 Award 权重 1；
- ticket 指向最后 Award；
- Award 数量 1、10、100、1000；
- 无状态 constant source；
- `ReportAllocs`；
- package-level sink 防止结果被优化掉。

这是刻意的最坏扫描位置。

### 12.3 当前本机复核环境

本手记编写时定向复核环境：

```text
Go: go1.26.6
OS: darwin
Arch: arm64
CPU: Apple M2 Pro
Commit: db679cf
```

命令：

```bash
go test ./internal/lottery/domain ./internal/lottery/adapter/randomsource \
  -run '^$' \
  -bench 'Benchmark(WeightedSelectorWorstCase|CryptoSourceUint64N)$' \
  -benchmem \
  -count=3
```

### 12.4 纯扫描实测范围

与第 20 节 QA 同一组三次结果范围为：

| Award 数 | 最坏位置 ns/op 范围 | B/op | allocs/op |
| ---: | ---: | ---: | ---: |
| 1 | 5.117～5.213 | 0 | 0 |
| 10 | 11.71～11.80 | 0 | 0 |
| 100 | 69.51～72.76 | 0 | 0 |
| 1000 | 658.4～663.1 | 0 | 0 |

这些结果与 O(n) 趋势一致。

零分配结果与“当前实现没有调用会防御性复制的 `Awards()`”一致；这是一台机器、一个工具链对已执行路径的测量，不是源码之外的永久证明。

### 12.5 CryptoSource 实测范围

`BenchmarkCryptoSourceUint64N` 使用 upper=10,000。

同组三次结果为：

```text
178.0～181.2 ns/op
56 B/op
4 allocs/op
```

这包含当前 `big.Int` 与 `crypto/rand.Int` 路径。

### 12.6 为什么两组 benchmark 必须拆开

若只测完整 selector：

- 随机源成本可能掩盖扫描差异；
- 无法判断优化哪一层；
- source 波动会影响算法数据；
- Prefix/Alias 的收益被错误估计。

拆开后可以看到：

- 100 Award 的最坏纯扫描当前约 70ns；
- 当前 CryptoSource 约 178ns；
- 1000 Award 扫描才明显超过 source。

这只是本机微基准观察。

### 12.7 为什么不能从 ns/op 算“支持多少 QPS”

简单执行：

```text
1 second / 178 ns
```

得到的数字不是业务吞吐。

真实 API 还包含：

- HTTP 解析；
- 鉴权；
- 限流；
- MySQL pool wait；
- Repository 查询；
- JSON 编码；
- 结果持久化；
- 日志与 trace；
- GC；
- goroutine 调度；
- 网络；
- future inventory/Benefit。

### 12.8 benchmark 不是 P99

Go benchmark 通常给出平均 ns/op。

它不直接给出：

- P50；
- P95；
- P99；
- timeout；
- 排队；
- 多实例；
- 资源水位。

所以不能把 178ns 写成业务 P99。

### 12.9 benchmark 不是 M1 SLO 证据

NFR 候选目标中的 Lottery 决策：

```text
峰值 10,000 RPS / 5min
P99 <= 150ms
```

当前 benchmark 完全不能证明这个目标已达到。

第 24 节 M1 需要完整运行链负载与环境记录。

### 12.10 为什么 1000 Award 数据不能代表生产

当前 benchmark 的 1000 Award 是人工构造。

仓库还没有：

- 真实 Award count 分布；
- 创建 API 上限；
- 生产 Strategy；
- 业务最常见权重形状。

所以它只提供算法增长曲线上的一个点。

### 12.11 最坏位置与平均位置

当前 benchmark 始终选择最后 Award。

真实平均成本取决于：

- AwardID 顺序；
- 每项权重；
- ticket 分布。

未来可以补：

- first bucket；
- middle bucket；
- last bucket；
- 高权重在开头；
- 高权重在末尾。

### 12.12 单候选 benchmark 的特殊性

Award 数为 1 时：

- source 不调用；
- 只做配置检查并返回；
- 5ns 级数据不能代表任何随机选择。

必须在文档中标注短路，否则读者可能误解。

### 12.13 CryptoSource allocations 是否现在优化

当前有 4 allocs/op。

可能优化方向包括：

- 手写 uint64 rejection；
- 复用 buffer；
- 使用 multiply-high 缩减；
- 批量 entropy；
- vetted DRBG。

但这些会增加：

- 安全代码量；
- 并发状态；
- 审查成本；
- 版本/seed 管理；
- failure mode。

当前没有端到端 profile 证明 4 allocations 是瓶颈。

所以不优化。

### 12.14 比较 Prefix/Alias 之前需要什么

公平比较至少要包含：

- 表构建时间；
- 每次选择时间；
- 内存；
- Strategy 重用次数；
- MaxUint64 正确性；
- source 次数；
- cold/hot cache；
- 更新/失效成本。

只比较 hot select 会偏袒预计算算法。

### 12.15 benchmark 可重复性

未来正式记录应：

- 固定 commit；
- 记录 Go version；
- 记录 CPU/OS；
- 使用多次 count；
- 保存原始输出；
- 用 benchstat 比较；
- 控制后台负载；
- 不只摘最好一次。

### 12.16 性能决策触发器

只有出现以下证据才应升级算法：

- p95/max Award count 增长；
- selector CPU 占业务进程明显比例；
- selection latency 在 trace 中可见；
- Strategy 被缓存且大量复用；
- benchmark 显示构建成本可摊销；
- 优化后仍保持相同整数/错误语义。

### 12.17 当前可诚实表达的性能结论

可以说：

> 在 Go 1.26.6、Apple M2 Pro 的定向微基准中，当前剩余权重线性扫描在 1～1000 个 Award 的最坏位置为零分配，并呈线性增长；CryptoSource 单次 bounded generation 约 178ns、56B、4 allocations。该数据只用于算法基线，不代表 HTTP/数据库/业务 SLO。

不能说：

> 系统已支持千万 QPS 或达到抽奖 P99 目标。

---

## 13. 失败模式分析：系统最危险的时候往往是“看起来还能返回结果”

### 13.1 失败模式总表

| ID | 失败模式 | 当前检测/控制 | 当前结果 | 尚未覆盖 |
| --- | --- | --- | --- | --- |
| F20-01 | selector 为 nil | receiver 检查 | not configured | composition readiness |
| F20-02 | source 为 nil | 构造/调用检查 | not configured | runtime wiring test |
| F20-03 | typed nil source | reflect 检查 | not configured | source 内部坏配置 |
| F20-04 | zero Strategy | len/total 检查 | strategy invalid | 完整重复校验 |
| F20-05 | 单项 total 漂移 | weight==total | invariant violation | 包外 unsafe |
| F20-06 | source 返回 error | wrap | random source failure | retry policy |
| F20-07 | source 返回 upper | 范围检查 | contract violation | 统计偏差检测 |
| F20-08 | 多项 total 有空洞 | 扫描尾部检查 | invariant violation | 所有非法状态组合 |
| F20-09 | no_reward 被选中 | Award 正常返回 | success | UI/HTTP 表达 |
| F20-10 | raw modulo bias | crypto/rand.Int | 拒绝采样 | OS entropy certification |
| F20-11 | int64 截断 | SetUint64/uint64 scan | 完整范围 | JSON transport |
| F20-12 | source data race | 条件契约/race test | 默认 source 通过 | 第三方 source |
| F20-13 | source 永久阻塞 | 无 | 调用可能阻塞 | context/timeout |
| F20-14 | 巨大 Award 集合 | O(n) | 延迟线性增长 | 创建上限/DoS |
| F20-15 | 重复业务请求 | 无 Draw identity | 会再次选择 | 幂等持久化 |
| F20-16 | 选择后进程崩溃 | 无持久化 | 结果丢失 | DrawResult |
| F20-17 | 调用方丢弃不利结果 | 无协议 | 无法阻止 | 审计/唯一请求 |
| F20-18 | operator 恶意改权重 | 不在本节 | 无法阻止 | 版本/审批 |
| F20-19 | fake source 进生产 | 无 composition gate | 风险 | wiring/security test |
| F20-20 | cause 泄露 | safe Error() | 顶层脱敏 | 调用方展开日志 |

### 13.2 selector 未配置

这是 composition error。

不能：

- 默认创建弱 PRNG；
- 自动使用全局 math/rand；
- 返回 no_reward；
- panic 后依赖 middleware。

当前显式失败。

### 13.3 source 语义偏差但仍在范围内

这是最难检测的边界之一。

例如 source 永远返回 0。

它始终满足：

```text
0 <= value < upper
```

selector 的范围检查不会报错。

但概率完全错误。

控制只能来自：

- 生产 adapter 白名单；
- 代码审查；
- 供应链治理；
- future 统计监控；
- composition test。

### 13.4 source 阻塞

当前 BoundedRandomSource 没有 context。

一个错误实现可永久阻塞。

默认本地 CSPRNG 的已知行为使本节接受这项风险。

远程 source 会让该假设失效。

### 13.5 单候选掩盖 source 故障

短路意味着 entropy source 即使失效，单候选仍成功。

这是有意的业务可用性选择。

但运维不能用单候选流量证明随机源健康。

### 13.6 巨大集合与资源滥用

领域没有 Award 数量上限。

Repository 会整体加载。

selector 会线性扫描。

未来若创建 API 对不可信用户开放，必须考虑：

- request body 上限；
- Award count 上限；
- 创建授权；
- DB 行数；
- cache payload；
- selection worst-case。

本节不凭空定义产品上限。

### 13.7 多次请求直到中奖

CSPRNG 无法阻止用户重复调用。

如果 API 每次都直接 Select，攻击者可以：

- 高频请求；
- 使用多个账号；
- 只保留中奖响应；
- 利用客户端超时重试。

需要 Participation/Draw idempotency、额度与限流。

### 13.8 响应失败后的重抽

未来场景：

```text
服务已选中 Award
→ 响应丢失
→ 客户端重试
→ 再次 Select
```

当前会产生另一个结果。

所以第 21 节若开放“真正 Draw API”，必须先决定结果持久化与查询。

### 13.9 调用方选择性接受结果

若调用者能看到 Award 后决定是否持久化，就可能：

- 丢弃 no_reward；
- 重选直到 reward；
- 只保存特定 Award。

未来 use case 必须把选择与最终事实建立在不可选择性丢弃的事务/协议内。

### 13.10 策略更新导致历史不可解释

当前 Strategy create-only。

未来若同 ID 更新 Weight，而结果只保存 AwardID：

- 无法知道当时概率；
- 无法重放 ticket；
- 审计无法确认配置。

需要 Strategy version 或完整结果快照。

### 13.11 规范顺序变化

即使权重不变，改变 Award 顺序也会改变 ticket→Award 映射。

总体概率可能不变。

但重放和审计会变化。

所以一旦保存 ticket，排序规则成为版本契约。

### 13.12 side-channel 时间差

线性扫描耗时依赖命中位置。

理论上观察者可能从极精细 timing 推断 bucket 位置。

当前 API 尚不存在，而且最终 Award 本就会返回给调用方。

所以不做 constant-time 扫描。

若未来结果在返回前需要保密，应重新评估。

### 13.13 反复统计“异常连败”

用户可能把随机 streak 当成系统作弊。

技术系统需要区分：

- 合法随机波动；
- 配置错误；
- source 偏差；
- 重试/选择性保存；
- UI 展示误导。

这需要可审计 Draw 数据，而当前没有。

### 13.14 adapter error 细节泄露

SourceError 与 SelectionError 顶层已脱敏。

但未来日志若直接展开 cause，仍可能泄露内部信息。

需定义 trusted diagnostic sink 与访问控制。

### 13.15 Go/标准库升级

随机 API、FIPS 行为、性能与 error 语义可能随 Go 版本演进。

升级必须重跑：

- rejection fixture；
- MaxUint64；
- source failure；
- concurrency/race；
- benchmark。

---

## 14. 可观测性与安全运营：不要为了监控而泄露随机过程

### 14.1 当前事实

当前 selector 尚未装配到运行时。

没有 production metric、trace 或 log。

本节只能设计未来观测边界，不能声称已有 dashboard。

### 14.2 建议的低基数指标

未来可考虑：

```text
lottery_selection_total{result_class="reward|no_reward|error"}
lottery_selection_error_total{class="..."}
lottery_random_source_error_total{class="..."}
lottery_selection_duration_seconds
lottery_random_source_duration_seconds
lottery_strategy_award_count
```

标签必须受控。

### 14.3 不应直接做 metric label 的字段

避免把以下高基数字段作为常规 label：

- StrategyID；
- AwardID；
- user ID；
- request ID；
- Award name；
- raw error cause；
- random ticket。

高基数会增加成本并可能泄露业务数据。

### 14.4 为什么要拆分 selector 与 source latency

若只记录一个总 duration：

- 无法区分 entropy 变慢；
- 无法区分 Award 数量增长；
- 无法判断是否需要算法优化；
- 无法发现 source adapter 切换。

所以 future instrumentation 应分别观察。

### 14.5 哪些错误需要高优先级告警

通常优先级：

```text
contract violation / invariant violation
>
source unavailable
>
invalid strategy / not configured（部署期）
```

原因：

- contract/invariant 说明正确性边界被破坏；
- source unavailable 说明依赖故障；
- invalid/not configured 多为发布与组装问题。

具体阈值要等运行时流量出现。

### 14.6 不记录 raw ticket

raw ticket 可能：

- 帮助攻击者分析随机序列；
- 泄露内部选择细节；
- 形成高基数日志；
- 被误当成可重放凭据。

当前没有审计协议需要它。

所以默认不记录。

### 14.7 不记录 entropy bytes

entropy bytes 更不应进入：

- log；
- trace；
- metric exemplar；
- error message；
- analytics event。

### 14.8 结果指标与商业敏感性

即使只按 reward/no_reward 聚合，中奖率也可能是商业敏感数据。

访问 dashboard 需要：

- 权限；
- 租户隔离；
- 时间窗口；
- 数据保留策略。

### 14.9 公平性监控不是简单报警

未来比较实际与理论分布时，需要：

- 同一 Strategy version；
- 足够样本；
- 排除资格与库存过滤；
- 处理多重比较；
- 区分随机波动；
- 防止自动“修正”合法 streak。

不能看到短时偏差就动态改权重。

### 14.10 trace 边界

未来 trace 可以记录：

- selector semantic outcome；
- award count bucket；
- duration；
- error class；
- Strategy version 的受控引用。

不应记录：

- entropy；
- random ticket；
- 完整用户数据；
- 未脱敏 cause。

### 14.11 readiness 是否探测随机源

对 OS CSPRNG 做主动 readiness 抽样有问题：

- 会消费随机能力但不证明未来一定成功；
- 单次成功不是长期健康；
- 单候选路径本来不需要 source；
- 探针可能制造噪声。

当前更合理的是：对可返回错误的自定义/未来 source 监控真实 error；对 Go 1.26 默认 Reader 监控进程退出、runtime fatal 与平台重启/告警。单靠 `SourceError` 指标看不到默认随机能力的不可恢复失败。

若 future HSM/remote RNG 有正式健康协议，再单独设计。

### 14.12 安全日志的两层错误

外部响应只需要 stable class。

受控内部日志可能需要 cause。

两者必须分开：

```text
public:
lottery selection unavailable

trusted diagnostics:
semantic class + reviewed cause + trace correlation
```

### 14.13 配置与代码供应链

未来 composition 应防止：

- 测试 fake 被生产构建引用；
- source 被环境变量任意切换；
- 未审查 plugin 实现 port；
- Go/toolchain 升级跳过测试；
- 随机 adapter 被 shadow import。

### 14.14 权限边界

随机 source 不需要：

- MySQL 权限；
- Redis 权限；
- 网络权限；
- filesystem secret；
- 用户身份。

保持 adapter 最小依赖能缩小攻击面。

### 14.15 隐私边界

selector 不接收 user 信息。

这降低了：

- PII 泄露；
- 用户属性暗中影响概率；
- source seed 与用户身份耦合。

未来个性化规则必须显式建模，不能偷偷把 user ID 混入 random source。

---

## 15. 安全、公平与合规边界：最重要的结论是不要说谎

### 15.1 当前可以说的“无偏”

可以准确说：

> 当前 CryptoSource 使用标准库 `crypto/rand.Int` 在半开区间生成 bounded value，selector 使用整数减法区间映射；在 source 均匀、Strategy 合法的条件下，每个 Award 的理论概率等于其 Weight/TotalWeight。

### 15.2 当前不可以说的“公平”

不可以说：

- 抽奖系统已经公平认证；
- 用户无法作弊；
- 运营无法作弊；
- 每次结果可公开验证；
- 所有用户机会相等；
- 库存与概率一致；
- 有完整审计链；
- 符合特定国家/行业监管。

### 15.3 密码学随机性的有限贡献

CSPRNG 主要缓解：

- 从公开输出预测未来输出；
- 通过时间 seed 猜测序列；
- 简单状态恢复攻击。

它不缓解：

- 重复请求；
- 多账户；
- 权重恶意配置；
- 选择性保存结果；
- 服务端被控制；
- 结果表被篡改。

### 15.4 算法公平与资格公平

即使每次 selection 权重正确，用户是否能进入 selection 仍可能不公平。

资格规则可能受：

- 会员等级；
- 地区；
- 设备；
- 活动预算；
- 风控；
- 历史参与；
- 实验分组。

这些属于规则、Participation 和 Marketing 边界。

### 15.5 算法公平与库存公平

若高价值 Award 库存不足，系统需要决定：

- 选择前过滤；
- 选择后保留；
- 保留失败补偿；
- 降级为其他 Award；
- 返回处理中。

任何处理都会改变最终用户看到的分布。

当前 selector 完全不知道库存。

### 15.6 算法公平与结果持久化

若只有中奖结果被保存、no_reward 被丢弃，最终数据会呈现虚假分布。

若调用者可重抽，理论单次概率不能代表用户最终结果概率。

所以公平证据必须建立在不可选择性丢弃的 DrawResult 上。

### 15.7 公示概率

UI 未来可能把整数权重转换成百分比。

需要决定：

- 四舍五入位数；
- 展示总和是否恰好 100%；
- 极小概率怎样显示；
- no_reward 是否合并展示；
- Strategy 更新后公示怎样留档。

算法正确不等于公示准确。

### 15.8 合规需求会改变架构

如果出现监管要求，可能新增：

- 概率备案；
- 配置审批；
- 不可变发布版本；
- 第三方随机源；
- 可验证承诺；
- 完整 Draw 审计；
- 数据留存；
- 争议查询；
- 地区隔离。

届时当前 selector 可以作为映射组件，但不能独自满足合规。

### 15.9 FIPS 误表述边界

Go `crypto/rand` 文档描述某些 FIPS 模式行为。

当前项目没有：

- 认证构建证明；
- 部署模式证明；
- CMVP 证书范围核对；
- 运行环境审计。

所以不能写“项目已通过 FIPS”。

### 15.10 面试与简历表达

可以表达：

> 设计可注入的 bounded random port，以 `crypto/rand.Int` 拒绝采样避免 modulo bias；用半开区间和剩余权重减法支持 MaxUint64，并通过全 ticket 穷举、边界、错误、并发与微基准验证。

必须紧接着说明：

> 该组件只选择配置候选，尚不等于幂等 Draw、库存或权益交付，也不是合规公平证明。

### 15.11 “高并发抽奖”表述边界

race test 和零状态 selector 只能支持：

> 选择器在并发安全 source 下可被多 goroutine 安全调用。

不能支持：

> 已完成高并发在线抽奖。

因为后者还需要 API、数据库、结果、库存、压测与容量证据。

### 15.12 “零分配”表述边界

当前纯扫描 benchmark 是零分配。

CryptoSource 仍有 4 allocs/op。

完整 Select 使用 crypto source 时不能笼统说“整个抽奖零分配”。

### 15.13 最诚实的本节命名

课程标题为了学习节奏叫“最简单概率抽奖”。

代码对象叫 `WeightedSelector` 更准确。

本节完成的是 weighted award selection primitive。

不是 finished lottery transaction。

---

## 16. 未来演进：保持 primitive 稳定，让新事实进入正确边界

### 16.1 第 21 节首先要决定 API 到底暴露什么

未来第一个 Lottery API 可能有两种完全不同的语义：

1. 演示性地返回一次临时选择；
2. 创建一个可查询、幂等且最终唯一的 DrawResult。

两者不能使用同一个模糊“draw success”文案。

如果只是临时选择：

- 客户端重试会再次执行选择，结果不保证相同；
- 响应丢失无法查询；
- INV-03 仍未满足；
- 不应接价值奖励。

如果创建 DrawResult：

- 需要 DrawID/request identity；
- 需要唯一约束；
- 需要结果状态；
- 需要提交未知处理；
- 需要查询接口；
- 需要 Strategy version/snapshot；
- 需要资格与次数事务边界。

### 16.2 application composition 应怎样出现

下一层可以消费：

```text
StrategyReader
WeightedSelector（或 consumer-owned selector port）
```

但不应让 domain selector 反向知道 Repository。

application use case 负责：

- context；
- 读取；
- error translation；
- future result persistence；
- authorization orchestration。

### 16.3 selector interface 何时由 application 定义

当前没有 application consumer 必须替换 selector。

所以无需提前定义一个完全重复的：

```go
type AwardSelector interface { ... }
```

第 21 节真实 use case 出现后，由消费者决定它需要：

- `Select(strategy)`；
- 还是 `Select(ctx,strategy)`；
- 是否返回更丰富的内部结果；
- 是否需要 batch。

接口应跟随消费者，而不是跟随实现文件夹。

### 16.4 HTTP error 映射

未来 transport 必须区分：

- strategy not found；
- stored strategy invalid；
- repository failure；
- selector not configured；
- random source unavailable；
- invariant violation；
- no_reward success。

底层 class 不应直接变成用户文案。

### 16.5 第 22 节前端必须避免客户端重抽

当前前端 Mock 随机演示不能继续冒充后端事实。

真实页面需要区分：

- reward；
- no_reward；
- 系统失败；
- 请求处理中；
- 结果未知；
- 重试是否安全。

动画不能决定结果。

结果必须先由服务端事实决定，再驱动动画。

### 16.6 第 23 节规则系统会改变“谁进入 selector”

规则可能决定：

- 用户是否可抽；
- 使用哪个 Strategy；
- 哪些 Award 当前可用；
- 是否需要不同权重；
- 活动时间窗；
- 风控拒绝。

不能把规则塞进 RandomSource。

RandomSource 只产生均匀 bounded value。

### 16.7 Strategy 版本化

一旦 Strategy 可更新，历史 Draw 需要引用当时配置。

候选方案：

- immutable StrategyVersion ID；
- Strategy ID + optimistic version；
- Draw 内完整 Award/weight snapshot；
- versioned compiled selection table。

当前 selector 接收完整快照，天然可以复用。

但结果模型必须保存版本关联。

### 16.8 第 24 节缓存与预计算

缓存出现后才有证据讨论：

- Strategy 重用次数；
- prefix table 构建摊销；
- compiled selector 生命周期；
- cache key 是否含 version；
- corrupt cache 怎样 Restore；
- Redis failure 是否回源。

不应把 Alias table 直接序列化进 Redis，除非先定义算法版本和兼容协议。

### 16.9 Prefix table 的演进路径

若 benchmark 与运行数据支持，可以新增不可变 compiled value：

```text
CompiledWeightedStrategy
- strategy version
- canonical awards
- exact uint64 prefixes
- total
```

构建时校验：

- prefix 单调；
- last == total；
- 不溢出；
- Award 顺序稳定。

选择时二分第一个 `prefix > ticket`。

### 16.10 Alias 的演进前提

Alias 只有在以下条件同时存在时值得：

- Award 数量大；
- 同一表重用极高；
- prefix 二分仍是瓶颈；
- exact arithmetic 方案已审查；
- table versioning 已建立；
- 构建/切换可原子完成；
- 统计与边界测试完善。

### 16.11 动态库存不是修改 Weight 的简单理由

库存变化快于 Strategy 配置。

若每次库存变化就修改 Weight：

- 配置版本爆炸；
- 历史概率难解释；
- cache 频繁失效；
- 并发库存仍未解决。

未来可能需要：

- 先选择后原子保留；
- 候选过滤与重采样协议；
- 独立库存算法；
- fallback Award 明确建模。

这些都不是当前 selector 的隐式责任。

### 16.12 不放回抽样

当前每次调用都是有放回且不保存抽样状态的选择；跨调用是否统计独立由 source 决定，领域端口没有作此承诺。

不放回需要维护剩余集合/权重。

它引入：

- 状态；
- 并发；
- 持久化；
- 恢复；
- 事务；
- 顺序依赖。

不能通过在 selector 中删除 slice 元素临时实现。

### 16.13 批量选择

future batch 可能用于预生成或活动池。

需要先回答：

- 有放回还是无放回；
- source 是否批量；
- 部分失败怎样表达；
- 结果是否一次提交；
- batch 是否可重放；
- entropy 消耗与审计。

当前单次 API 不提前承诺。

### 16.14 确定性 PRF 的可能场景

未来为了 request-level deterministic result，可能考虑：

```text
HMAC(secret, drawID || strategyVersion)
→ uniform bounded mapping
```

它可能支持重算与幂等。

但会引入：

- secret 管理；
- key rotation；
- drawID 可选择/grinding；
- domain separation；
- 泄露影响；
- bounded conversion；
- 旧 key 留存。

不能把普通 hash(requestID) 当作安全替代。

### 16.15 VRF / commit-reveal

公开可验证场景可能需要：

- 服务先承诺 seed；
- 活动结束后揭示；
- 或使用 VRF proof；
- 或引用外部 beacon。

这会改变：

- API；
- 延迟；
- 可用性；
- key custody；
- 审计数据；
- dispute process。

当前 CSPRNG adapter 不能冒充这些能力。

### 16.16 多实例与多区域

当前 source 是进程内能力。

多实例仍可各自安全产生随机值。

但业务唯一性不由 source 协调。

跨实例 Draw 必须依赖：

- 唯一业务 ID；
- 权威结果存储；
- 并发约束；
- outcome unknown 查询；
- region failover 语义。

### 16.17 Benefit 发放失败不能重选

未来流程：

```text
Award selected
→ DrawResult durable
→ Benefit delivery requested
→ delivery failed/retrying
```

发放失败不能再次调用 selector 改结果。

否则失败用户可能获得不同 Award，破坏唯一结果。

### 16.18 MQ/Outbox

当选中 reward 后需要异步发放，可能使用：

- DrawResult transaction；
- Outbox；
- MQ；
- idempotent Benefit consumer；
- compensation。

selector 仍保持纯内存选择 primitive。

### 16.19 多租户

未来 tenant 影响：

- Strategy 身份；
- 配置权限；
- 结果隔离；
- 指标隔离；
- 审计。

它不应该改变 bounded random 数学契约。

### 16.20 规则、概率与实验

A/B 实验可能给不同用户选择不同 Strategy。

应记录：

- experiment assignment；
- Strategy version；
- DrawResult。

不能把实验分组随机与 Award selection 随机混成同一个 source 调用而不做 domain separation。

---

## 17. 假设与风险账本：每一项“目前足够”都有失效信号

| ID | 假设/风险 | 当前证据或控制 | 失效影响 | 观察信号 | 重评动作 |
| --- | --- | --- | --- | --- | --- |
| R20-01 | Strategy 构造后可信 | 私有字段、构造/Restore | 非法状态进入热路径 | invariant error | 审计所有 hydration |
| R20-02 | Award 数量当前较小 | 只有 fixture/benchmark | O(n) 延迟增大 | p95/max award count | 比较 prefix |
| R20-03 | Strategy 每次选择无需预编译 | 当前无缓存 | 重复扫描浪费 CPU | reuse count/profile | compiled table |
| R20-04 | AwardID 顺序可作规范映射 | domain 已排序 | 重放兼容变化 | position 需求 | versioned order ADR |
| R20-05 | 相对整数权重满足业务 | 领域契约 | 公示/监管不匹配 | 固定精度需求 | 输入/展示 ADR |
| R20-06 | MaxUint64 必须继续支持 | 领域、source、测试 | 优化缩窄范围 | 新算法转换 | 全范围 gate |
| R20-07 | 减法线性扫描足够 | 零分配微基准 | CPU/尾延迟上升 | selector profile | prefix/alias 评审 |
| R20-08 | CryptoSource 性能足够 | 本机约178ns | allocation/CPU 增长 | production profile | 优化 source |
| R20-09 | crypto/rand.Reader 适合部署 | 官方契约、本机测试 | 默认故障不可恢复 | process exit/runtime/platform alerts；custom source errors | platform review |
| R20-10 | source 可本地同步调用 | 当前 OS source | 阻塞不可取消 | remote/HSM 需求 | context port |
| R20-11 | 仓库只有 CryptoSource 生产候选 | 尚未运行时装配；全局 Reader 可变 | 弱 source 进入生产 | wiring/init/dependency diff | composition 与供应链审查 |
| R20-12 | interface 实现真正均匀 | 官方 adapter 候选 | 范围内偏差不可检测 | 分布异常/reader provenance 变化 | adapter、Reader 与依赖审查 |
| R20-13 | CSPRNG 威胁模型足够 | 价值型奖励推导 | 需公开验证 | 合规要求 | VRF/commit-reveal |
| R20-14 | 单候选无需 entropy | 数学短路/测试 | source 健康被掩盖 | 单候选占比 | 独立 source monitor |
| R20-15 | no_reward 是成功 | 领域枚举/测试 | 上层误重试 | retry/no_reward 关联 | API contract test |
| R20-16 | selection error 无业务持久化副作用 | 当前无 DB/库存/发奖；source 仍可能推进状态 | future persistence 混入 | use case change | 拆分事务语义 |
| R20-17 | `Error()` 脱敏足够 | safe wrappers | cause 被外部日志展开 | log audit | trusted sink policy |
| R20-18 | typed nil reflect 成本可忽略 | 只在构造路径 | composition overhead | selector churn | 单例装配 |
| R20-19 | selector 长期无共享状态 | 当前结构 | metrics/cache 加 mutable state | race/profile | immutable instrumentation |
| R20-20 | source 并发安全由实现保证 | 注释+crypto tests | 第三方 source race | race detector | wrapper/source contract |
| R20-21 | race 测试窗口足够发现常见问题 | targeted race 通过 | 罕见调度遗漏 | production anomaly | stress/soak |
| R20-22 | 单元穷举覆盖映射 | 小权重 exact counts | 大组合 bug | fuzz findings | property fuzz |
| R20-23 | 标准库拒绝采样正确 | official code+fixture | Go upgrade behavior | toolchain change | source tests |
| R20-24 | big.Int allocations 可接受 | 4 allocs/op 基线 | GC 成本 | alloc/profile | bounded uint64 implementation |
| R20-25 | benchmark 形状代表最坏扫描 | last bucket fixture | 真实分布不同 | trace/profile | 多分布 benchmark |
| R20-26 | M2 Pro 数据仅作本机基线 | 文档限定 | 被外推成 SLO | 简历/README 宣称 | 修正文案 |
| R20-27 | 结果无需当前持久化 | 本节范围 | 重试结果不保证相同 | API 接入 | DrawResult first |
| R20-28 | INV-03 尚未满足被清楚表达 | 文档/代码边界 | 用户重复获不同结果 | API design | unique Draw |
| R20-29 | Strategy 暂不可更新 | Create-only repo | 历史概率不可解释 | Update 需求 | version/snapshot |
| R20-30 | 多个相同 Outcome 仍需选择 ID | Award 身份语义 | 错误短路 | UI 合并需求 | 保持 selection identity |
| R20-31 | 用户不能控制 seed | 无 seed API | grinding/prediction | API 参数提案 | 拒绝或安全 PRF |
| R20-32 | 可返回的 source error 失败关闭可接受 | 无 fallback | 可用性下降 | custom/future source errors；默认 Reader 看进程/runtime 告警 | runbook，不降级弱 RNG |
| R20-33 | invariant violation 应高优告警 | 不可达状态 | 静默错误概率 | error count >0 | stop/repair |
| R20-34 | 线性 timing 泄露不敏感 | Award 最终返回 | secret result 场景 | security review | constant-time/隔离 |
| R20-35 | 全 no_reward Strategy 合法 | domain 决策 | 运营误配置 | publish validation | governance rule |
| R20-36 | source metrics 不需主动 probe | 本地可靠 source | 潜伏故障 | zero random traffic | platform-specific health |
| R20-37 | Go 1.26.6 行为稳定 | pinned toolchain/tests | upgrade drift | go.mod/toolchain change | rerun full matrix |
| R20-38 | NIST 引用不被当认证 | 文档限定 | 合规夸大 | external statement | compliance review |
| R20-39 | 当前没有 PII 输入是优势 | selector signature | future personalization 偷渡 | source accepts user | explicit rule context |
| R20-40 | cache 不保存算法内部表 | 当前无 cache | stale/format lock-in | Redis lesson | versioned payload ADR |

### 17.1 风险账本如何使用

风险不是“以后再说”的同义词。

每一行都需要：

- 可观察信号；
- 失效影响；
- 下一动作；
- 重新决策时点。

### 17.2 哪些风险是 correctness 级

以下应视为 correctness/security 高优先级：

- contract violation；
- invariant violation；
- weak source 被装配；
- no_reward 被重试；
- 重复请求产生多个最终结果；
- Strategy version 丢失。

### 17.3 哪些风险是性能级

以下不能在没有 profile 时提前优化：

- O(n) scan；
- big.Int allocations；
- reflect typed nil；
- source latency；
- prefix/Alias。

### 17.4 哪些风险是治理级

以下需要产品、法务或安全参与：

- 公示概率；
- 可验证随机性；
- 运营审批；
- 数据留存；
- 地区合规；
- 争议处理。

---

## 18. 隐藏问题与重评触发器：第一性原则要求主动寻找尚未被问到的点

### 18.1 source port 应不应该接收 context

当前不接收是因为默认 source 本地且标准 API 不支持 context。

触发重评：

- remote RNG；
- HSM；
- sidecar；
- source latency 出现在 trace；
- shutdown 需要中断。

### 18.2 source port 应返回 raw uint64 还是 bounded value

当前 bounded port 把无偏缩减交给 adapter。

优点：domain 只消费需要的语义。

代价：interface 无法证明实现均匀。

触发重评：

- 需要统一审计 rejection；
- 多个 adapter 重复实现错误；
- 性能要求手写 uint64 reducer。

### 18.3 是否应把 deterministic mapping 独立成纯函数

当前 mapping 在 selector 内，测试通过 fake ticket 控制。

独立函数可用于：

- property tests；
- replay；
- compiled table 对照；
- formal verification。

但公开 ticket API 也可能被误用为指定中奖结果。

触发重评：需要审计重放或多 source 共用 mapping。

### 18.4 单候选是否应要求 source 配置

当前构造 selector 仍要求 source 非 nil，但执行短路不调用。

这在 composition 与业务可用性之间取得平衡。

若未来允许完全没有 source 的“deterministic selector”，需要单独类型或更清晰构造器，不能让多候选运行时才失败。

### 18.5 selector 是否应完全重新验证 Strategy

当前信任领域值。

触发重评：

- 字段被导出；
- cache decoder 绕过 Restore；
- plugin/unsafe 构造；
- invariant violation 真实出现。

### 18.6 为什么 zero Strategy cause 使用 AwardsRequired

当前 `ErrSelectionStrategyInvalid` 包装 `ErrStrategyAwardsRequired`。

它足以表明无法选择。

未来若 API 需要区分 ID/名称/集合错误，不应在 selector 热路径重跑完整校验；应在边界构造时分类。

### 18.7 reflect typed nil 是否值得

它解决真实 Go interface 陷阱。

成本仅在构造。

若 selector 被每请求构造，真正问题是 composition 生命周期错误，而不是 reflect 本身。

selector 应作为长期依赖组装。

### 18.8 source 方法 panic 怎样处理

当前不 recover。

触发重评：

- source 来自不可信插件；
- 进程隔离模型改变；
- panic 影响可用性。

优先方案通常是隔离 adapter，而不是 domain recover。

### 18.9 rejection loop 是否可能无限

拒绝采样理论上可能连续拒绝任意多次。

对正确随机源，期望次数有界且通常很小。

恶意/坏 reader 可永久给出拒绝值。

当前测试 reader 有限；官方默认 Reader 是当前生产候选，但其全局变量可被进程内代码替换，最终可信性仍需 composition 与依赖审查。

remote/custom reader 会触发超时重评。

### 18.10 random source 健康怎样定义

“一次调用成功”不是长期健康。

“统计分布看起来正常”也不是密码学认证。

未来健康应结合：

- 可返回的 custom/future source error；
- latency；
- 默认 Reader 对应的进程退出、runtime fatal 与 platform alerts；
- deployment configuration；
- library upgrade validation。

### 18.11 是否记录 random ticket 用于重放

记录 ticket 能帮助确定性重放。

但也增加：

- 安全暴露；
- 数据模型；
- Strategy version 依赖；
- 隐私/留存；
- 被误用指定结果。

当前不记录。

触发重评：合规审计或 dispute resolution。

### 18.12 是否记录 entropy proof

普通 `crypto/rand` 没有可公开 proof。

若需要 proof，必须替换协议，不是多打一条日志。

### 18.13 common-factor normalization

权重 `[100,300]` 可约分为 `[1,3]`。

自动约分可能：

- 降低 total；
- 改变 ticket 空间；
- 改变审计原始输入；
- 影响重放。

当前不自动约分。

触发重评：运营导入、极大权重、版本治理。

### 18.14 0 weight 是否可表示禁用

当前 Weight 必须正。

用 0 表示禁用会让 bucket 无长度，并模糊配置状态。

未来禁用应显式字段/规则，而不是改变算法不变量。

### 18.15 多个 no_reward 是否应合并

算法层不能合并，因为 ID/Name 可能有意义。

UI 可以展示合并概率，但必须保留后台事实。

### 18.16 Reward value 是否应该影响随机安全级别

高价值奖励可能要求：

- 更强审计；
- key custody；
- 人工审批；
- public verification；
- 独立风险模型。

当前统一使用 CSPRNG 是安全默认，但不替代分级治理。

### 18.17 Award count 是否要上限

当前没有业务证据定义具体数值。

第 21 节开放写 API 前必须基于：

- UI 使用；
- DB 成本；
- request size；
- selector benchmark；
- cache payload；
- abuse risk；

决定上限。

### 18.18 选择算法是否需要版本号

当前只有一种算法，没有结果持久化。

未来如果 Draw 保存 ticket 并需重放，应该保存：

- algorithm version；
- ordering rule；
- Strategy version；
- bounded source/proof metadata（按需求）。

### 18.19 Prefix/Alias 的上线兼容

新算法必须对相同 Strategy 与 ticket 明确：

- 是否要求同一 Award 映射；
- 还是只要求相同分布。

若历史 audit 依赖 ticket，必须保持映射或版本化。

### 18.20 source 更换的兼容性

从 CSPRNG A 换到 B 可能保持均匀分布，却改变：

- 输出序列；
- latency；
- error；
- compliance；
- proof。

当前不承诺序列兼容。

### 18.21 是否需要随机批量池

批量预取可能减少 per-call 成本。

但会引入：

- 共享状态；
- buffer 生命周期；
- crash 后未用 entropy；
- fork/process 语义；
- 并发锁；
- observability。

只有 profile 证明 source 成为瓶颈时评估。

### 18.22 多租户是否共享 source

共享 OS CSPRNG 通常可以。

但审计、密钥与合规可能要求租户隔离。

不能用 tenant ID 直接当 seed。

### 18.23 A/B 分组与 Award selection 是否共用随机源

数学上可共用底层 CSPRNG 能力。

语义上应分开 port/domain separation，避免：

- 一个序列影响另一业务；
- audit 混淆；
- deterministic PRF 输入碰撞。

### 18.24 错误是否应该进入重试中间件

`ErrRandomSourceFailure` 不应被通用 HTTP retry 自动重放，而不考虑 Draw identity。

第 21 节必须把 retry policy 放在完整用例语义下。

### 18.25 source contract violation 是否应该熔断

它通常代表代码/供应链错误，而非瞬时抖动。

future runtime 可考虑：

- 高优告警；
- 停止该实例接收 Draw；
- readiness 失败；
- 自动回滚发布。

不能 `%` 修复继续服务。

### 18.26 invariant violation 是否应该隔离 Strategy

若只某个 cached Strategy 损坏，可能隔离该 Strategy 并回源。

若算法本身损坏，应停止服务。

当前没有 cache/runtime，暂不决定。

### 18.27 用户是否应该看到理论概率

这取决于产品与法规。

若展示，需要基于发布版本计算，不应让前端自行从可能过期 Weight 猜测。

### 18.28 统计异常由谁处理

可能需要：

- 数据团队；
- 风控；
- Lottery owner；
- 安全；
- 法务。

算法自动调整权重不是默认答案。

### 18.29 灾难恢复后怎样证明随机结果

当前没有结果持久化，所以无法恢复。

未来 DR 需要恢复：

- DrawResult；
- Strategy version；
- audit；
- delivery state。

随机 source 本身通常不需要恢复旧内部状态，除非采用确定性协议。

### 18.30 简历数字怎样避免误导

可以列 microbenchmark 环境与范围。

不能把 ns/op 换算成“支撑 X 万 QPS”。

端到端数字只能来自带环境、持续时间、成功率和 P99 的负载报告。

### 18.31 全局 `crypto/rand.Reader` 可变性

`NewCryptoSource()` 在构造时捕获导出的进程级 `crypto/rand.Reader` 变量。

当前仓库没有重写它，但未来依赖或初始化代码可以在构造前替换 Reader；此时对象类型仍是 `CryptoSource`，仅做 adapter 类型白名单无法发现弱随机实现。

因此第 21 节 composition 和后续供应链审查必须核对：

- Reader 是否仍为 Go 官方默认实现；
- 哪个包在何时构造 source；
- 是否存在对 `crypto/rand.Reader` 的赋值；
- GODEBUG、FIPS 与构建模式是否改变实际随机边界；
- 依赖升级是否改变初始化路径。

若未来确实需要可替换 source，应建立显式、受审计的构造与配置边界，而不是借全局变量进行隐式替换。

### 18.32 明确重评触发器清单

出现任一条件必须重新评审：

1. Award count p95/max 显著增长；
2. selector CPU 超过服务 profile 的可见比例；
3. CryptoSource latency/allocation 成为瓶颈；
4. 引入 Strategy cache；
5. 引入 Strategy Update/version；
6. API 对外执行价值型 Draw；
7. 出现 request retry/response lost 问题；
8. 引入库存或预算；
9. 引入 remote RNG/HSM；
10. 需要可公开验证随机性；
11. 监管要求概率备案/审计；
12. Go toolchain/crypto 实现升级；
13. source contract/invariant error 出现；
14. 需要不放回或 batch；
15. 需要 deterministic replay；
16. 多租户/多区域改变信任边界；
17. 需要按用户个性化权重；
18. 前端展示精确概率；
19. 真实分布监控发现异常；
20. 简历/文档开始声称线上公平或高并发。

---

## 19. 可追溯证据、官方来源与最终边界

### 19.1 核心实现

- [WeightedSelector 与 BoundedRandomSource](../../../internal/lottery/domain/weighted_selector.go)
- [Selection 稳定错误与安全 cause](../../../internal/lottery/domain/selection_error.go)
- [CryptoSource 与 bounded CSPRNG adapter](../../../internal/lottery/adapter/randomsource/crypto.go)
- [Lottery domain package 边界](../../../internal/lottery/domain/doc.go)
- [randomsource package 边界](../../../internal/lottery/adapter/randomsource/doc.go)

### 19.2 领域前置证据

- [Strategy 聚合、TotalWeight 与规范顺序](../../../internal/lottery/domain/strategy.go)
- [Award、Weight 与 reward/no_reward](../../../internal/lottery/domain/award.go)
- [领域稳定错误](../../../internal/lottery/domain/errors.go)
- [Strategy 构造与所有权测试](../../../internal/lottery/domain/strategy_test.go)
- [Award 边界测试](../../../internal/lottery/domain/award_test.go)

### 19.3 本节测试与 benchmark

- [WeightedSelector 精确映射、短路、最大值、失败、并发与 benchmark](../../../internal/lottery/domain/weighted_selector_test.go)
- [CryptoSource 拒绝采样、错误、最大值、并发与 benchmark](../../../internal/lottery/adapter/randomsource/crypto_test.go)

### 19.4 持久化前置边界

- [Strategy Repository port](../../../internal/lottery/application/repository.go)
- [MySQL Repository](../../../internal/lottery/adapter/mysqlrepo/repository.go)
- [第 19 节第一性原理手记](lesson-19.md)
- [ADR-0016：Repository 边界](../../decisions/ADR-0016-lottery-repository-boundaries.md)

### 19.5 本节交付文档

- [第 20 节课程正文](../../course/part-03/lesson-20-lottery-weighted-selection.md)
- [第 20 节 API 记录](../../api/lessons/lesson-20.md)
- [第 20 节 QA](../../qa/lessons/lesson-20.md)
- [ADR-0017：Lottery weighted selection](../../decisions/ADR-0017-lottery-weighted-selection.md)

### 19.6 产品与 NFR 边界

- [非功能需求与 INV-03](../../product/non-functional-requirements-v1.md)
- [Lottery bounded context](../../product/bounded-context-map-v1.md)
- [系统设计 V0](../../product/system-design-v0.md)

### 19.7 Go 官方来源

- [`crypto/rand` package（Go 1.26.6）](https://pkg.go.dev/crypto/rand@go1.26.6)
- [`crypto/rand.Int`（Go 1.26.6）](https://pkg.go.dev/crypto/rand@go1.26.6#Int)
- [`crypto/rand` 源码（Go 1.26.6 tag）](https://cs.opensource.google/go/go/+/refs/tags/go1.26.6:src/crypto/rand/)
- [`math/big.Int.SetUint64`（Go 1.26.6）](https://pkg.go.dev/math/big@go1.26.6#Int.SetUint64)
- [`math/rand/v2`（Go 1.26.6）](https://pkg.go.dev/math/rand/v2@go1.26.6)
- [Go Memory Model](https://go.dev/ref/mem)
- [Go Race Detector](https://go.dev/doc/articles/race_detector)
- [`testing` benchmark 文档](https://pkg.go.dev/testing)
- [`errors.Is` / `errors.As`](https://pkg.go.dev/errors)

### 19.8 随机性权威资料

- [NIST SP 800-90A Rev.1：DRBG](https://csrc.nist.gov/pubs/sp/800/90/a/r1/final)
- [NIST SP 800-90B：Entropy Sources](https://csrc.nist.gov/pubs/sp/800/90/b/final)
- [NIST SP 800-90C：RBG Constructions](https://csrc.nist.gov/pubs/sp/800/90/c/final)

这些资料用于解释威胁模型和术语。

它们不是当前项目认证证书。

### 19.9 定向验证命令

```bash
go test ./internal/lottery/domain ./internal/lottery/adapter/randomsource

go test -race \
  ./internal/lottery/domain \
  ./internal/lottery/adapter/randomsource

go test \
  ./internal/lottery/domain \
  ./internal/lottery/adapter/randomsource \
  -run '^$' \
  -bench 'Benchmark(WeightedSelectorWorstCase|CryptoSourceUint64N)$' \
  -benchmem \
  -count=3
```

完整仓库仍应执行：

```bash
make verify
go test -race ./...
```

真实结果以第 20 节 QA 最终记录为准。

### 19.10 当前证据能支持的最强结论

当前可以证明：

- 合法多候选 Strategy 被划分成精确、无重叠、无空洞的整数区间；
- source 上界使用 `[0,totalWeight)`；
- selector 使用剩余权重减法，不做 `total+1` 或 signed conversion；
- TotalWeight 为 MaxUint64 时仍可选择第一、边界与最后 bucket；
- 单候选在校验内部一致后不读取 source；
- no_reward 作为正常 Award 返回；
- random error、越界 source 与内部映射坏状态失败关闭；
- 公开错误字符串不泄露底层 cause；
- CryptoSource 使用 `crypto/rand.Int` 拒绝采样，而不是 modulo；
- default source 与 selector 在当前 race 测试窗口内可并发调用；
- 当前线性扫描零分配并在 1～1000 Award 最坏位置呈线性增长；
- 当前 CryptoSource 有独立的性能/分配基线。

### 19.11 当前证据不能支持的结论

当前不能证明：

- Lottery HTTP API 可用；
- 同一业务请求只形成一个结果；
- 响应丢失后可安全重试；
- 用户资格/次数正确；
- 库存不超发；
- Benefit 已到账；
- Strategy Update 历史可解释；
- Redis 缓存一致；
- 生产 CSPRNG 经过本项目独立认证；
- 操作员无法修改/丢弃结果；
- 随机过程可公开验证；
- 符合某项法规；
- 达到 10,000 RPS 或 P99 150ms；
- microbenchmark 等于端到端 SLO；
- INV-03 已满足。

### 19.12 第一性原则最终复盘

本节真正做对的事情不是“选了一个更高级的随机库”。

而是把五种不同事实拆开：

```text
Strategy 配置事实
≠
bounded entropy 事实
≠
内存中的 Award selection
≠
持久化且幂等的一次 DrawResult
≠
Benefit 实际到账
```

只要这些事实没有被重新混在一起，未来就可以：

- 替换 Repository；
- 替换随机 adapter；
- 升级选择算法；
- 增加缓存；
- 引入 DrawResult；
- 引入库存与发奖；
- 增加审计与合规；

而不需要让一个组件谎称自己拥有全部责任。

当前最准确的结论是：

> 第 20 节实现了一个支持完整 `uint64`、无模偏差、可注入 CSPRNG、单候选短路、失败关闭与条件并发安全的加权 Award 选择 primitive；它尚未形成一次可查询、幂等、库存安全、已发奖或合规可验证的业务 Draw，微基准也不能外推为线上 SLO。
