# 第 20 节面试题：无偏加权选择与密码学随机源

本文只描述提交 `db679cf` 中已经实现、可以由代码和测试核对的时间切片：领域层新增 `WeightedSelector` 与最小 `BoundedRandomSource` 端口，基础设施适配器用 `crypto/rand.Int` 生成 `[0, totalWeight)` 上的均匀整数，选择器再用减法桶映射到按 AwardID 排序的 Award。测试穷举 `1:3`、`100:300`、`2:3:5` 三组代表性小区间的全部 ticket，并覆盖指定边界、`math.MaxUint64`、`no_reward`、随机源失败、契约违例、typed-nil、并发与 benchmark 形状。

本节仍然**没有** Draw/Result 身份、结果持久化、幂等键、Lottery HTTP API、库存预占、权益发放、审计证明或真实前端。因此它证明的是“给定一个合法 Strategy，可以在内存中无偏选择一个 Award”，不是“线上抽奖链路已经完成”，更没有满足 `INV-03`“一次抽奖只能有一个最终结果”。

## 来源与事实边界

- `项目事实` 只表示当前仓库在 `db679cf` 的代码、测试或既有产品约束，可以直接沿链接复核。
- `官方技术事实` 来自 Go 官方文档/源码、RFC、NIST 或公开论文，用于支撑语言、标准库和算法性质；项目自己的业务结论不能倒推成这些来源的结论。
- `面经题型启发` 来自牛客用户发布的候选人自述或整理帖。它们只能证明“有人在该页面自述遇到或整理过这种题型”，本文未独立核验公司、岗位、轮次和题目原文，**不得称为公司官方原题**；帖子中的答案或代码也不作为技术事实依据。
- 本文在 2026-08-29 核验了可访问页面。典型题型包括[候选人自述的按 float 大小加权采样](https://www.nowcoder.com/discuss/353154373217886208)、[候选人自述的按 0.6/0.4 输出](https://www.nowcoder.com/discuss/353154090437910528)、[候选人自述的偏置二元源转均匀三元源](https://www.nowcoder.com/discuss/353159306365313024)和[候选人自述的 `ORDER BY RAND()` 追问](https://www.nowcoder.com/discuss/353156368821592064)。这些链接用于设计追问方向，不替代 Go 官方资料与算法证明。

## 1. 第 20 节到底实现了什么，最容易被简历夸大的地方是什么？

- **直接回答：** 实现的是一个持久化无关的加权选择内核：输入已构造好的 `Strategy`，多候选时从 `BoundedRandomSource` 取得一个 `[0,totalWeight)` ticket，再返回恰好一个 `Award`。生产适配器使用 `crypto/rand.Int`。它没有创建一次 Draw，也没有保存最终结果或发奖。
- **追问：** 为什么不能在简历上直接写“实现了高并发可幂等抽奖系统”？
- **追问回答：** “高并发”至少需要端到端容量与延迟证据；“幂等”需要请求身份、唯一约束、结果复用和结果未知时的查询协议。当前只有纯内存选择与局部并发测试，多候选重试会再次取随机数，所以这两项都不能从本节外推。
- **项目证据：** [选择器职责注释与实现](../../../internal/lottery/domain/weighted_selector.go)、[密码学随机源适配器](../../../internal/lottery/adapter/randomsource/crypto.go)、[`INV-03` 当前证据状态](../../product/non-functional-requirements-v1.md)。
- **选型边界：** 如果只是离线模拟，当前内核已经可复用；若成为价值型在线抽奖，还必须在应用层建立 Draw/Result、幂等、审计、库存和发放边界。
- **来源：** `项目事实` 上述代码与产品约束；`面经题型启发` [牛客候选人自述中的加权采样题型](https://www.nowcoder.com/discuss/353154373217886208)，该页面不是公司官方题库。

## 2. 为什么一个权重为 \(w_i\) 的 Award 被选中的概率恰好是 \(w_i / T\)？

- **直接回答：** 设总权重 \(T=\sum_i w_i\)，随机源让 `[0,T)` 中每个整数 ticket 的概率都是 `1/T`。减法桶为第 i 个 Award 分配恰好 `w_i` 个整数，因此它被选中的概率是 `w_i × (1/T)=w_i/T`。关键前提是每个权重为正、总和准确且 ticket 真正均匀。
- **追问：** `1:3` 与 `100:300` 是否完全相同？
- **追问回答：** 作为单次选择的概率分布相同，都是 `1/4` 与 `3/4`；但总区间、ticket 到 Award 的具体映射、配置字节和审计事实不同，项目不会静默约分运营输入。
- **项目证据：** [Strategy 的正整数相对权重与受检总和](../../../internal/lottery/domain/strategy.go)、[逐个枚举 ticket 后计数恰等于 weight 的测试](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 这个证明只覆盖静态离散权重；动态库存、用户分层、预算封顶或条件规则改变了候选集时，必须先定义哪个版本的候选集是本次 Draw 的权威输入。
- **来源：** `官方技术事实` [`crypto/rand.Int` 保证 `[0,max)` 均匀](https://pkg.go.dev/crypto/rand#Int)；`面经题型启发` [牛客候选人自述的“数值越大、被采样概率越大”题型](https://www.nowcoder.com/discuss/353154373217886208)。

## 3. 为什么随机区间设计成 `[0,total)`，而不是 `[1,total]`？

- **直接回答：** 半开区间与 Go 标准库的有界随机 API 一致，长度直接等于 `total`，边界判断统一为 `<`，而且不需要计算 `total+1`。这使 `total == math.MaxUint64` 仍可表达，避免闭区间实现中的加一溢出。
- **追问：** `[1,total]` 数学上也有 total 个数，为什么工程上仍不选？
- **追问回答：** 它并非数学错误，但通常要把底层零基随机数加一，或请求一个 `total+1` 的上界；两条路径都增加边界转换，后一条在最大值直接溢出。半开区间让随机源契约和桶映射共用一个不变量。
- **项目证据：** [`BoundedRandomSource` 的半开区间契约](../../../internal/lottery/domain/weighted_selector.go)、[最大范围首尾 ticket 测试](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 外部协议若规定一基序号，可以在传输边界显式转换；领域随机契约仍应保持半开区间，不能让两套边界在热路径混用。
- **来源：** `官方技术事实` [`crypto/rand.Int`](https://pkg.go.dev/crypto/rand#Int)与[`math/rand/v2.Uint64N`](https://pkg.go.dev/math/rand/v2#Uint64N)都采用半开区间；`项目事实` 上述实现与测试。

## 4. 减法桶算法如何工作，最常见的 off-by-one 在哪里？

- **直接回答：** 对 ticket 依次检查 `ticket < weight`；命中则返回当前 Award，否则执行 `ticket -= weight` 进入下一个桶。权重 `2,3,5` 对应原始区间 `[0,2)`、`[2,5)`、`[5,10)`。先比较再相减，能保证每个桶的整数个数恰等于权重。
- **追问：** 如果写成 `ticket <= weight` 会怎样？
- **追问回答：** 第一个权重为 2 的桶会错误包含 ticket 2，得到 3 个点；相邻桶边界随之重叠或错位。若使用一基 ticket 才可能出现 `<=`，但必须整套证明和测试一起改变。
- **项目证据：** [减法桶循环](../../../internal/lottery/domain/weighted_selector.go)、[`2,3,5` 对全部十个 ticket 的期望映射](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 前缀和加二分也可以正确实现，但边界条件应写成“第一个累计权重大于 ticket”；不能把闭区间公式直接搬到零基 ticket。
- **来源：** `官方技术事实` [Go `sort.Search` 对单调条件的契约](https://pkg.go.dev/sort#Search)可作为二分方案的边界参照；`面经题型启发` [牛客候选人自述的加权采样题型](https://www.nowcoder.com/discuss/353154373217886208)。

## 5. Award 的遍历顺序为什么是领域契约，而不只是实现细节？

- **直接回答：** Strategy 构造时按 AwardID 规范排序，选择器直接按该顺序分桶。顺序不改变每个 Award 的概率，但会改变同一个 ticket 对应哪个 Award；稳定顺序让测试、持久化恢复和问题复盘不会受调用方 slice、SQL 计划或 map 遍历顺序影响。
- **追问：** 只保存 ticket 就能永久重放结果吗？
- **追问回答：** 不能。还要保存 Strategy 身份及不可变版本、候选和权重快照或其可验证摘要，以及算法版本；否则配置或排序规则变化后，同一 ticket 可能映射到另一个 Award。当前项目尚无这些结果事实。
- **项目证据：** [Strategy 规范排序及防御性复制](../../../internal/lottery/domain/strategy.go)、[乱序输入仍按 AwardID 分桶的测试](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 如果运营需要显式展示/抽取优先级，应新增稳定 `position` 或版本字段；不能借输入顺序承载未建模的业务含义。
- **来源：** `项目事实` 上述领域代码；`官方技术事实` [Go 规范说明 map 迭代顺序未指定](https://go.dev/ref/spec#For_statements)。

## 6. 为什么使用正整数相对权重，而不是直接存 `float64` 概率？

- **直接回答：** 正整数权重使区间长度和概率证明完全离散，避免浮点求和、比较和归一化误差，也能精确表达 `uint64` 范围。`1:3` 足以表达比例，展示百分比可在读模型计算，不必成为选择内核的输入事实。
- **追问：** 业务真的配置 33.33% 怎么办？
- **追问回答：** 可以由明确精度的外部契约转换为整数，例如万分比 `3333:6667`，并保留原始配置/舍入政策供审计。不能让前端、API 和 Go 算法各自用不同浮点规则转换。
- **项目证据：** [`Weight` 的相对权重语义](../../../internal/lottery/domain/award.go)、[总权重溢出检查](../../../internal/lottery/domain/strategy.go)。
- **选型边界：** 科学模拟若天然使用连续分布，浮点可能合理；价值型离散抽奖更需要精确、跨语言一致的整数契约。若公开给 JavaScript，还要处理其安全整数范围。
- **来源：** `官方技术事实` [Go 规范的浮点类型与表示](https://go.dev/ref/spec#Floating-point_types)；`面经题型启发` [牛客候选人自述的 float 加权采样题型](https://www.nowcoder.com/discuss/353154373217886208)，其 float 输入不等于本项目必须采用 float。

## 7. `math.MaxUint64` 是怎样被端到端支持的？

- **直接回答：** Strategy 在相加前用 `weight > MaxUint64-totalWeight` 防溢出，允许总和恰好为 `MaxUint64`。随机源用 `new(big.Int).SetUint64(upper)` 无损构造上界；选择器不算 `total+1`、不构造可能溢出的累计和，而是逐桶减法，所以合法最大 ticket `MaxUint64-1` 仍能落入末桶。
- **追问：** 上界为 `MaxUint64` 时是否可能返回值 `MaxUint64`？
- **追问回答：** 不可能，因为契约是 `[0,upper)`；最大合法返回值是 `upper-1`，即 `MaxUint64-1`。适配器还做了 `value < upper` 的防御性后置检查。
- **项目证据：** [Strategy 溢出保护](../../../internal/lottery/domain/strategy.go)、[SetUint64 与选择器映射](../../../internal/lottery/adapter/randomsource/crypto.go)、[最大范围多桶边界测试](../../../internal/lottery/domain/weighted_selector_test.go)、[随机源最大上界测试](../../../internal/lottery/adapter/randomsource/crypto_test.go)。
- **选型边界：** 任何改用 `int`、`int64`、`big.NewInt(int64(...))` 或 JSON number 的边界都可能缩窄契约，必须通过 ADR/API/Migration 明示，不能静默截断。
- **来源：** `官方技术事实` [`math/big.Int.SetUint64`](https://pkg.go.dev/math/big#Int.SetUint64)、[Go 无符号整数溢出规则](https://go.dev/ref/spec#Integer_overflow)；`项目事实` 上述边界测试。

## 8. 为什么不能直接写 `rawUint64 % totalWeight`？

- **直接回答：** 如果原始随机空间大小不是 `totalWeight` 的整数倍，余数桶接收到的原始值数量不同。对完整 `uint64` 空间而言，除非 `totalWeight` 整除 \(2^{64}\)，否则直接取模有 modulo bias；价值型抽奖不应接受这种可避免的系统偏差。
- **追问：** 偏差可能非常小，为什么还要处理？
- **追问回答：** 正确的标准库有界采样已经提供了成熟实现，省略它没有合理收益；即便单次差异小，高频调用、合规解释和攻击者选择边界都可能放大问题。更重要的是契约应是“均匀”，而不是“看起来差不多”。
- **项目证据：** [`BoundedRandomSource` 明确禁止用 modulo 修复](../../../internal/lottery/domain/weighted_selector.go)、[候选 3 被拒绝而不是 `% 3` 的适配器测试](../../../internal/lottery/adapter/randomsource/crypto_test.go)。
- **选型边界：** 上界是 2 的幂时位掩码可以无偏；其他上界可用标准库的拒绝采样或乘法降域算法，不应自行发明未经证明的映射。
- **来源：** `官方技术事实` [Go `math/rand/v2` 源码对偏差及拒绝区间的推导](https://go.dev/src/math/rand/v2/rand.go)、[Lemire 的快速整数降域论文](https://arxiv.org/abs/1805.10941)；`项目事实` 上述契约与测试。

## 9. `crypto/rand.Int` 的拒绝采样具体做了什么？

- **直接回答：** 官方实现根据 `max-1` 的 bit length 读取足够字节，屏蔽最高字节多余位，把字节解释为整数；候选若 `< max` 就返回，否则重新读取。这样每个可接受整数拥有相同数量的输入表示，不需要取模。
- **追问：** 项目说“选择器最多调用随机源一次”，是否等于只读取一次熵？
- **追问回答：** 不等于。`Select` 对 `Uint64N` 最多调用一次，但 `crypto/rand.Int` 可在这一次方法调用内部因拒绝候选而多次读取。测试用上界 3 和字节 `0x03,0x02` 证明先拒绝 3、再接受 2。
- **项目证据：** [`CryptoSource.Uint64N`](../../../internal/lottery/adapter/randomsource/crypto.go)、[拒绝候选而非取模的确定性测试](../../../internal/lottery/adapter/randomsource/crypto_test.go)。
- **选型边界：** 不应依赖一次调用消耗固定字节数，也不应把具体拒绝次数写成 API 契约；若熵读取延迟需要治理，应在 adapter 指标和上层 deadline 中观测。
- **来源：** `官方技术事实` [`crypto/rand.Int` 文档](https://pkg.go.dev/crypto/rand#Int)及[官方拒绝采样源码](https://go.dev/src/crypto/rand/util.go)；`项目事实` 上述注入 reader 测试。

## 10. 为什么生产适配器选 `crypto/rand`，而不是 `math/rand/v2`？

- **直接回答：** 抽奖结果可能承载价值，项目优先选择不可预测的密码学安全随机源；`math/rand/v2` 的有界映射同样可做到无偏，但它是伪随机工具，不适合安全敏感场景。本项目没有做两者的同条件性能对照，因此不宣称速度结论。领域只依赖有界接口，所以该取舍没有污染选择算法。
- **追问：** 用了 `crypto/rand` 就能宣称“公平、防作弊”吗？
- **追问回答：** 不能。它只加强随机值不可预测性；配置发布、内部权限、随机源健康、结果不可篡改、算法版本、审计和幂等都仍可能破坏公平或可证明性。
- **项目证据：** [CryptoSource 的职责与“不代表审计/可重放/公平证明”注释](../../../internal/lottery/adapter/randomsource/crypto.go)、[领域端口](../../../internal/lottery/domain/weighted_selector.go)。
- **选型边界：** 可复现实验、仿真或性能测试可以注入种子化 PRNG；公开价值抽奖若有监管/可验证随机要求，还可能需要外部随机信标、承诺揭示或独立审计，而不只是 OS CSPRNG。
- **来源：** `官方技术事实` [`crypto/rand` 是密码学安全随机数包](https://pkg.go.dev/crypto/rand)、[`math/rand/v2` 不适用于安全敏感工作](https://pkg.go.dev/math/rand/v2)、[RFC 4086 的安全随机性要求](https://www.rfc-editor.org/rfc/rfc4086)。

## 11. 为什么 `BoundedRandomSource` 接口定义在 domain，而实现返回具体 `*CryptoSource`？

- **直接回答：** 选择算法是接口的消费者，它只需要 `Uint64N(upper)` 这一项能力，所以最小端口放在 domain。基础设施包实现该端口并返回具体类型，避免生产者为“方便 mock”预先定义宽接口；测试可直接提供小 fake。
- **追问：** 为什么接口不直接暴露 `io.Reader` 或 `Uint64()`？
- **追问回答：** 那会迫使 domain 自己处理字节序、降域和偏差，扩大安全敏感代码面。`Uint64N` 把真正需要的“均匀半开区间”作为契约，测试也能直接枚举 ticket。
- **项目证据：** [单方法领域端口](../../../internal/lottery/domain/weighted_selector.go)、[编译期接口断言与具体构造器](../../../internal/lottery/adapter/randomsource/crypto.go)、[recording/constant fake](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 若多个消费者需要不同随机能力，可拆分更小端口或建立专门随机服务；不要把 `NextInt/Float/Bytes/Shuffle` 全塞进本接口。
- **来源：** `官方技术事实` [Go Code Review Comments：接口通常属于使用方，生产者返回具体类型](https://go.dev/wiki/CodeReviewComments#interfaces)；`项目事实` 上述包边界。

## 12. 为什么多候选选择只请求一个有界随机值？

- **直接回答：** 一个均匀 ticket 已经能唯一映射到一个权重桶；再为每个 Award 取随机数既无必要，也更难证明联合分布。当前多候选路径恰好调用一次 `Uint64N(totalWeight)`，之后是确定性映射；随机源错误立即失败。
- **追问：** 随机源暂时失败时，选择器为什么不内部重试三次？
- **追问回答：** 拒绝非均匀候选属于随机源内部算法，不是依赖故障重试。可返回错误的自定义 reader/未来 source 由应用层结合 deadline、重试预算和将来的 Draw 幂等协议决定；selector 内部盲重试会隐藏延迟与失败率。Go 1.26 默认 Reader 的底层失败则是不可恢复的进程级事件，不进入这条重试判断。
- **项目证据：** [一次调用后减法映射](../../../internal/lottery/domain/weighted_selector.go)、[每个 ticket 都断言 calls 为 1 和 upper 为 10](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 动态抽取多个不放回 Award 是另一类算法，需要重新定义状态和概率；不能循环调用当前单选器并假设分布仍正确。
- **来源：** `官方技术事实` [`crypto/rand.Int` 的均匀有界契约](https://pkg.go.dev/crypto/rand#Int)；`面经题型启发` [牛客候选人自述的单次加权采样题型](https://www.nowcoder.com/discuss/353154373217886208)。

## 13. 单候选为什么短路，完全不调用随机源？

- **直接回答：** 一个合法 Strategy 只有一个 Award 时，结果是确定的；读取熵不能增加正确性，只会引入可用性、延迟和分配成本。实现先验证该 Award 的权重等于声明总权重，再直接返回，测试覆盖权重 1、400 和 `MaxUint64` 且 source calls 为 0。
- **追问：** 这会不会让“每次抽奖都必须留下随机证据”的审计不一致？
- **追问回答：** 会，所以短路成立的边界是当前职责只需选择正确 Award。如果未来合规协议要求每次 Draw 都绑定随机信标/nonce，即使结果确定也可能要在应用层记录证据；那是新需求，不应伪装成当前实现。
- **项目证据：** [单候选不取随机数且检查内部总和](../../../internal/lottery/domain/weighted_selector.go)、[三种极值短路测试](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 如果 source 调用还承担计费、审计或序列推进，不能依赖隐含副作用；应把这些职责显式建模，而不是破坏随机端口的纯语义。
- **来源：** `项目事实` 上述代码与测试；`官方技术事实` [`crypto/rand.Int` 的职责只是生成有界随机值](https://pkg.go.dev/crypto/rand#Int)。

## 14. `no_reward` 为什么是成功选择，不是 error 或空 Award？

- **直接回答：** `no_reward` 是一个有 ID、名称、正权重的合法 AwardOutcome，表示抽取成功但没有待发权益。随机源故障、非法 Strategy 和映射不变量破坏才是技术错误。把未中奖当错误会诱发通用重试，反而可能把一次未中奖重抽成中奖。
- **追问：** `no_reward` 是否已经是满足幂等要求的最终 DrawResult？
- **追问回答：** 不是。它目前只是被返回的领域候选；没有 DrawID、持久化或请求关联。第 21 节若建立结果协议，reward 与 no_reward 都必须作为同等终态被复用，不能只持久化中奖。
- **项目证据：** [Outcome 与 HasReward 语义](../../../internal/lottery/domain/award.go)、[选中 no_reward 返回 nil error 的测试](../../../internal/lottery/domain/weighted_selector_test.go)、[`INV-03`](../../product/non-functional-requirements-v1.md)。
- **选型边界：** “库存不足”或“依赖超时”不能降级成 no_reward；运营是否允许全 no_reward 策略则是发布治理问题，不属于随机映射。
- **来源：** `项目事实` 上述领域模型与测试；`面经题型启发` [牛客候选人自述的按指定概率输出 0/1 题型](https://www.nowcoder.com/discuss/353154090437910528)，其中输出 0 仅启发概率题形，不定义本项目业务语义。

## 15. Strategy 已由构造器校验，`Select` 为什么还要防御零值和内部不变量破坏？

- **直接回答：** Go 的结构体零值始终可能传入，且 selector 是决策边界，不能在空 awards 或 total 为 0 时调用随机源。正常 Strategy 依赖构造器建立正权重、无重复和受检总和；selector 再检查空值、单候选总和，并在所有桶耗尽仍未命中时返回 invariant violation，绝不制造一个结果。
- **追问：** 为什么 `Select` 不逐个重新验证所有 Award？
- **追问回答：** 字段未导出且 Strategy 是不可变值，正常外部路径只能通过构造/恢复获得合法聚合；每次热路径全量重复校验会重复责任。当前保留便宜且能防止危险映射的护栏，包内伪造坏 Strategy 的测试验证 fail closed。
- **项目证据：** [选择前置与不可达兜底](../../../internal/lottery/domain/weighted_selector.go)、[零 Strategy、映射洞和单候选总和不一致测试](../../../internal/lottery/domain/weighted_selector_test.go)、[Strategy 构造不变量](../../../internal/lottery/domain/strategy.go)。
- **选型边界：** 一旦开放反序列化直填、插件或 unsafe 边界，应在信任边界调用 Restore/New 完整复验；不能把当前包封装假设扩展到任意输入。
- **来源：** `官方技术事实` [Go 规范的零值](https://go.dev/ref/spec#The_zero_value)；`项目事实` 上述构造与防御测试。

## 16. 如果随机源违反契约，返回 `ticket == upper`，为什么不 `% upper` 修复？

- **直接回答：** 选择器返回 `ErrRandomSourceContractViolation` 和零 Award。取模会掩盖 adapter bug，并把 `upper` 固定映射为 0，既可能引入偏差，也让监控看不到边界破坏。消费者对不可信实现做后置校验，保持 fail closed。
- **追问：** `crypto/rand.Int` 已承诺范围，检查是不是多余？
- **追问回答：** 对当前标准库路径它是防御冗余，但领域端口允许其他实现；一次比较成本极小，却把契约错误从潜在错误发奖变成可分类故障。适配器本身也再次检查返回值。
- **项目证据：** [ticket 后置检查](../../../internal/lottery/domain/weighted_selector.go)、[fake 返回 upper=7 的契约违例测试](../../../internal/lottery/domain/weighted_selector_test.go)、[CryptoSource 后置检查](../../../internal/lottery/adapter/randomsource/crypto.go)。
- **选型边界：** 如果调用的是进程外随机服务，还应校验响应认证、版本和超时；这里的范围检查只覆盖数值契约，不证明来源可信。
- **来源：** `官方技术事实` [`crypto/rand.Int` 的 `[0,max)` 契约](https://pkg.go.dev/crypto/rand#Int)、[Go `math/rand/v2` 对直接降域偏差的源码说明](https://go.dev/src/math/rand/v2/rand.go)；`项目事实` 上述故障测试。

## 17. 什么是 interface typed-nil，构造器如何避免它？

- **直接回答：** Go interface 同时保存动态类型和值；`var p *recordingSource = nil; var s BoundedRandomSource = p` 时，`s != nil`，因为动态类型仍是 `*recordingSource`。构造器先检查 interface nil，再用反射对可 nil 的动态 kind 调用 `IsNil`，把这种输入归类为 `ErrSelectorNotConfigured`。
- **追问：** 为什么要先判断 kind，再调用 `IsNil`？
- **追问回答：** `reflect.Value.IsNil` 只适用于 Chan、Func、Interface、Map、Pointer、Slice 和 UnsafePointer；对实现接口的值类型（例如测试里的 struct source）直接调用会 panic。kind switch 同时支持指针实现和值实现。
- **项目证据：** [typed-nil 检测 helper](../../../internal/lottery/domain/weighted_selector.go)、[typed-nil source 构造失败测试](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 反射只发生在构造阶段，不在每次多候选映射循环；更简单的团队约定仍应避免把 typed-nil 装进接口，但公共构造边界不能只靠约定。
- **来源：** `官方技术事实` [Go FAQ：nil error/interface 的动态类型和值](https://go.dev/doc/faq#nil_error)、[`reflect.Value.IsNil`](https://pkg.go.dev/reflect#Value.IsNil)；`项目事实` 上述回归测试。

## 18. `SelectionError` 的错误链为什么同时需要稳定 class、`errors.Is` 和 `Unwrap`？

- **直接回答：** `Error()` 只输出白名单语义类，避免把 reader、内部 Strategy 或边界细节泄露到默认外部文本；`Is` 让上层稳定区分未配置、非法策略、随机源失败、契约违例和映射不变量；`Unwrap` 保留可信日志/测试需要的原始 cause。三者分别服务公开呈现、控制流和诊断。
- **追问：** 随机 reader 失败时，完整 error chain 是什么？
- **追问回答：** 外层是 `SelectionError(ErrRandomSourceFailure)`，其 cause 是 `SourceError(ErrEntropyUnavailable)`，后者再 unwrap 到 reader 原始错误。`errors.Is` 会沿链匹配，但默认 `err.Error()` 只显示最外层安全类；主动展开日志仍需脱敏。
- **项目证据：** [SelectionError 的 Error/Is/Unwrap 与白名单](../../../internal/lottery/domain/selection_error.go)、[随机源错误包装](../../../internal/lottery/adapter/randomsource/crypto.go)、[公开文本和 cause 断言](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 跨进程 API 应映射显式错误 code/status，而不是序列化 Go error chain；内部日志也不能因为可 Unwrap 就默认记录所有底层内容。
- **来源：** `官方技术事实` [`errors.Is` 的树形遍历规则](https://pkg.go.dev/errors#Is)、[`errors.Unwrap`](https://pkg.go.dev/errors#Unwrap)；`项目事实` 上述错误实现。

## 19. `CryptoSource` 为什么自己还定义三类错误？

- **直接回答：** `ErrSourceNotConfigured` 表示 nil receiver/reader，`ErrUpperBoundRequired` 表示空区间 `[0,0)`，`ErrEntropyUnavailable` 表示配置的 reader 或取样调用返回失败。`SourceError` 与领域错误一样只公开稳定 class 并保留 cause；selector 再把任意可返回的 source error 归到更高层的 `ErrRandomSourceFailure`。Go 1.26 默认 Reader 的底层失败不可恢复，不会走这条 error chain。
- **追问：** 为什么在调用 `crypto/rand.Int` 前必须检查 `upper == 0`？
- **追问回答：** 官方 API 对 `max <= 0` 会 panic；adapter 的接口契约选择返回可分类 error，所以要在边界拦截，不能让配置错误击穿进程。`SetUint64` 后的正上界才传给标准库。
- **项目证据：** [随机源错误类与检查顺序](../../../internal/lottery/adapter/randomsource/crypto.go)、[nil receiver、nil reader、零 upper 和 cause 保留测试](../../../internal/lottery/adapter/randomsource/crypto_test.go)。
- **选型边界：** Go 1.26 默认 OS Reader 的不可恢复失败不是 composition 可改成普通 error 的策略选择，需要由进程退出、runtime 与平台告警发现；可返回错误的远程/自定义 source 才由完整 use case 决定重试。无论哪种故障都不能伪装成业务结果。
- **来源：** `官方技术事实` [`crypto/rand.Int` 对非正 max 的行为](https://pkg.go.dev/crypto/rand#Int)；`项目事实` 上述 adapter 和测试。

## 20. 为什么随机源失败必须 fail closed，不能降级为第一个 Award 或 `no_reward`？

- **直接回答：** 依赖失败意味着本次无法按声明分布决定结果，不是“确定未中奖”。返回第一个 Award 会偏置并可能错误发奖；返回 no_reward 会吞掉事故、污染业务指标并诱导用户承担系统故障。当前所有失败都返回零 Award 加稳定错误。
- **追问：** 为了可用性，降级到 no_reward 不是更安全吗？
- **追问回答：** 对平台成本可能“安全”，对用户公平性和语义却不安全。正确做法是：对可返回的 source error 明确失败并在有幂等边界后重试或查询同一 Draw；对默认 Reader 的不可恢复故障监控进程/runtime/platform 信号并恢复实例。两者都不能通过改变中奖概率换可用性。
- **项目证据：** [随机源失败路径](../../../internal/lottery/domain/weighted_selector.go)、[失败时零 Award、安全文本与 cause 测试](../../../internal/lottery/domain/weighted_selector_test.go)、[no_reward 成功语义](../../../internal/lottery/domain/award.go)。
- **选型边界：** 非价值型推荐排序可以定义显式 fallback，但必须作为产品规则和指标建模；不能复用 Lottery 的 no_reward 来隐藏基础设施事故。
- **来源：** `官方技术事实` [RFC 4086 对随机源失效与安全随机性的讨论](https://www.rfc-editor.org/rfc/rfc4086)；`项目事实` 上述 fail-closed 契约。

## 21. `WeightedSelector` 的并发安全承诺到底到哪一层？

- **直接回答：** selector 自身没有可变状态，Strategy 拥有不可变的 awards 副本，因此共享 selector 调用可并发；但它持有的 `BoundedRandomSource` 必须自己并发安全。默认 `crypto/rand.Reader` 官方明确可并发使用，所以 `CryptoSource` 的生产组合满足当前承诺。
- **追问：** 为什么测试不用会记录 calls 的 `recordingSource` 做并发 fake？
- **追问回答：** 它修改计数和 slice，未加锁，本身不是并发安全实现；并发 selector 测试使用无状态值类型 `constantSource`，生产组合测试使用默认 CryptoSource。这样不会把 fake 的数据竞争误判成 selector 问题。
- **项目证据：** [接口并发契约和 selector 注释](../../../internal/lottery/domain/weighted_selector.go)、[无状态 source 并发测试](../../../internal/lottery/domain/weighted_selector_test.go)、[默认 CryptoSource 及组合并发测试](../../../internal/lottery/adapter/randomsource/crypto_test.go)。
- **选型边界：** 换成带种子状态的 PRNG、网络 client 或轮询 reader 时，必须审查其并发语义；可以每 goroutine 独享、加锁或使用明确并发安全实现，不能由 selector 猜测。
- **来源：** `官方技术事实` [`crypto/rand.Reader` 是全局且可并发安全使用](https://pkg.go.dev/crypto/rand#Reader)、[Go race detector](https://go.dev/doc/articles/race_detector)；`项目事实` 上述并发测试。

## 22. 为什么核心正确性测试枚举 ticket，而不是固定随机种子跑一百万次？

- **直接回答：** 映射层一旦注入确定 ticket，就可以对小总权重穷举整个 `[0,T)`，精确证明每个 Award 的命中数等于 weight，测试快速、可复现且不抖动。固定种子的大样本仍只检查某条伪随机序列，失败阈值还会引入统计误报/漏报。
- **追问：** 穷举小区间能证明 `MaxUint64` 吗？
- **追问回答：** 不能遍历整个最大空间，所以项目把证据拆开：小区间做全覆盖计数，最大总权重测试首桶、桶边界和末桶，CryptoSource 单独验证完整 upper 与拒绝行为。
- **项目证据：** [全 ticket 映射与精确计数测试](../../../internal/lottery/domain/weighted_selector_test.go)、[最大范围边界测试](../../../internal/lottery/domain/weighted_selector_test.go)、[随机源边界测试](../../../internal/lottery/adapter/randomsource/crypto_test.go)。
- **选型边界：** 若未来桶规则、候选过滤或多抽状态复杂，可增加 property/fuzz 测试；它们补充而非替代可穷举的不变量测试。
- **来源：** `官方技术事实` [Go fuzzing 指南](https://go.dev/doc/security/fuzz/)、[`testing` 包](https://pkg.go.dev/testing)；`面经题型启发` [牛客候选人自述的指定概率输出题型](https://www.nowcoder.com/discuss/353154090437910528)。

## 23. 当前测试能证明随机源“统计上公平”吗？

- **直接回答：** 不能。`AlwaysReturnsInsideRequestedRange` 对若干 upper 各取 64 次，只证明观察样本未越界；它既没有显著性水平，也没有分布检验。无偏性证据来自官方 `crypto/rand.Int` 契约/源码与映射的数学及穷举证明，而不是这 64 次抽样。
- **追问：** 加卡方检验或 NIST 套件后就能证明公平吗？
- **追问回答：** 仍不能“证明”。统计测试只能在特定样本和假设下发现某些非随机迹象；NIST 也强调测试套件不能替代密码分析。业务公平还包括配置、权限、审计、幂等和结果完整性。
- **项目证据：** [范围抽样测试](../../../internal/lottery/adapter/randomsource/crypto_test.go)、[确定性精确计数测试](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 生产可监控 outcome 与期望分布偏差作为告警线索，但必须按 Strategy 版本、样本量和多重检验设计；不能据此自动补偿中奖或覆盖单次结果。
- **来源：** `官方技术事实` [NIST SP 800-22 Rev.1a 的测试边界](https://csrc.nist.gov/pubs/sp/800/22/r1/upd1/final)、[`crypto/rand.Int` 契约](https://pkg.go.dev/crypto/rand#Int)；`项目事实` 上述测试范围。

## 24. 两个 benchmark 分别测了什么，又不能证明什么？

- **直接回答：** `BenchmarkWeightedSelectorWorstCase` 用无状态常量 source，对 Award 数 `1/10/100/1000` 取末桶并 `ReportAllocs`，主要观察线性 mapper；其中 size=1 实际走短路。`BenchmarkCryptoSourceUint64N` 以 upper 10000 单测生产随机源，包含 `crypto/rand.Int`、`big.Int` 与熵读取成本。
- **追问：** 可以拿这两个结果相加，宣称 API 的 P99 吗？
- **追问回答：** 不可以。它们没有数据库、HTTP、并发争用、容器、日志、GC 压力或结果持久化，也没有保存基线和回归阈值。微基准只比较受控代码路径，不能替代端到端压测和生产观测。
- **项目证据：** [选择器最坏桶 benchmark](../../../internal/lottery/domain/weighted_selector_test.go)、[CryptoSource benchmark](../../../internal/lottery/adapter/randomsource/crypto_test.go)。
- **选型边界：** 只有 profile/容量数据表明选择器占热点时才值得换算法；未来第 21 节 API 应另建包含真实依赖或明确 stub 边界的负载模型。
- **来源：** `官方技术事实` [Go `testing` benchmark 文档](https://pkg.go.dev/testing#hdr-Benchmarks)、[Go `pprof` 诊断文档](https://go.dev/doc/diagnostics#profiling)；`项目事实` [第 20 节 QA](../../qa/lessons/lesson-20.md)只保存本机微基准，当前没有产品性能 SLO 实测或代码级回归阈值。

## 25. 当前减法桶是 O(n)，为什么不一开始就用前缀和二分或 alias method？

- **直接回答：** 当前最小需求是一次选择一个小型、已规范排序的 Strategy。线性减法桶代码短、无需额外表、没有累计加法溢出，正确性容易穷举；在没有候选规模和吞吐数据前，引入预计算缓存与失效协议是无证据复杂化。
- **追问：** 什么时候应该升级算法？
- **追问回答：** profile 显示扫描占显著 CPU，且 Award 数长期较大、Strategy 版本读多写少时，可预计算前缀和并二分到 O(log n)；极高频静态分布可评估 alias method 的 O(1) 采样，但要承担 O(n) 建表、额外内存、版本切换和验证成本。
- **项目证据：** [线性减法实现](../../../internal/lottery/domain/weighted_selector.go)、[1/10/100/1000 最坏桶 benchmark 形状](../../../internal/lottery/domain/weighted_selector_test.go)。
- **选型边界：** 前缀和必须继续支持 `MaxUint64` 并定义不可变缓存归属；alias table 若使用浮点还要重新证明精度与跨版本一致性。任何优化都不能改变 AwardID 顺序和概率契约。
- **来源：** `官方技术事实` [Go `sort.Search`](https://pkg.go.dev/sort#Search)、[Vose alias method 论文](https://doi.org/10.1109/32.92917)；`项目事实` 当前 benchmark 只提供测量入口，不构成升级证据。

## 26. 为什么 selector 直接访问 `strategy.awards`，而不调用公开的 `Awards()`？

- **直接回答：** `Awards()` 为外部调用者返回防御性副本，保护 Strategy 所有权；selector 与 Strategy 同属 domain 包，可以只读内部规范 slice，避免每次选择复制整个候选集。选择器不修改它，所以不破坏不可变约束。
- **追问：** 这是否已经证明选择路径零分配？
- **追问回答：** 没有。源码只能证明没有因 `Awards()` 产生那次 slice copy；接口调用、标准库随机源、错误路径和编译器逃逸仍要以具体 benchmark/版本测量。本节 QA 已保存这台机器的一组数值，但没有跨环境基线或回归阈值，更不能外推完整选择链和 API。
- **项目证据：** [`Awards()` 防御性复制](../../../internal/lottery/domain/strategy.go)、[selector 只读内部 awards](../../../internal/lottery/domain/weighted_selector.go)、[选择器 benchmark](../../../internal/lottery/domain/weighted_selector_test.go)、[随机源 benchmark](../../../internal/lottery/adapter/randomsource/crypto_test.go)。
- **选型边界：** 若选择算法移动到另一个包，应新增不泄露可变所有权的遍历/快照契约，或接受显式复制；不能为了性能直接暴露内部 slice。
- **来源：** `官方技术事实` [Go slice 共享底层数组的规范](https://go.dev/ref/spec#Slice_types)、[Go benchmark 文档](https://pkg.go.dev/testing#hdr-Benchmarks)；`项目事实` 上述所有权设计。

## 27. 密码学不可预测性、概率无偏和可验证公平有什么区别？

- **直接回答：** 不可预测性关注攻击者能否预知随机值；无偏关注 `[0,T)` 各 ticket 与权重桶的概率是否正确；可验证公平还要求参与方能核验配置、随机材料、算法与最终结果未被操纵。当前 `crypto/rand` 加减法桶覆盖前两者的一部分，不提供第三者验证协议。
- **追问：** 管理员在抽奖前临时改权重，`crypto/rand` 能防止吗？
- **追问回答：** 不能。需要 Strategy 发布版本、权限与审批、不可变快照、审计日志，必要时加承诺揭示或外部信标。CSPRNG 不约束谁选择输入，也不保存输出。
- **项目证据：** [CryptoSource 的边界注释](../../../internal/lottery/adapter/randomsource/crypto.go)、[当前只保存 Strategy、没有 Draw/Result 的产品约束](../../product/non-functional-requirements-v1.md)。
- **选型边界：** 内部营销小游戏可能只需 CSPRNG 与审计；公开开奖或强监管场景的威胁模型更高，应独立设计可验证随机协议，不能用“用了 crypto”替代。
- **来源：** `官方技术事实` [RFC 4086](https://www.rfc-editor.org/rfc/rfc4086)、[NIST SP 800-22 的统计测试边界](https://csrc.nist.gov/pubs/sp/800/22/r1/upd1/final)、[`crypto/rand` 包说明](https://pkg.go.dev/crypto/rand)。

## 28. 为什么不在 MySQL 里用 `ORDER BY RAND() LIMIT 1`？

- **直接回答：** 那条 SQL 是从行集中随机排序后取一行，既没有表达本项目的整数相对权重契约，也把随机决策、数据访问和数据库执行计划耦在一起。当前 Repository 负责恢复合法 Strategy，domain selector 负责概率映射，随机源 adapter 负责无偏有界取样，职责和测试边界更清晰。
- **追问：** 给 SQL 加权表达式就一定更差吗？
- **追问回答：** 不一定，但必须重新证明数据库 `RAND()` 的随机性质、浮点边界、查询计划、快照与版本一致性，并解决全表计算/排序成本。对当前小聚合，加载后在内存 O(n) 选择更容易验证；大数据流抽样则可能需要 reservoir sampling 等另一类算法。
- **项目证据：** [选择器不依赖持久化](../../../internal/lottery/domain/weighted_selector.go)、[CryptoSource 独立 adapter](../../../internal/lottery/adapter/randomsource/crypto.go)、[Strategy Repository 课程边界](../../course/part-03/lesson-19-lottery-repository.md)。
- **选型边界：** 数据库抽样、流式 reservoir sampling 和已加载小集合加权抽样是不同问题，不能只因都叫“随机”就互换实现。
- **来源：** `面经题型启发` [牛客候选人自述中被追问 `ORDER BY RAND()` 的页面](https://www.nowcoder.com/discuss/353156368821592064)，不是网易官方原题；`官方技术事实` [MySQL `RAND()` 文档](https://dev.mysql.com/doc/refman/8.4/en/mathematical-functions.html#function_rand)。

## 29. “偏置二元随机源转均匀三元源”与本项目有什么关系，又有什么本质区别？

- **直接回答：** 两者都考察“先得到可证明均匀的基础样本，再用拒绝区间映射”。若独立偏置 bit 的 `P(1)=p`，序列 `01` 和 `10` 概率同为 `p(1-p)`，丢弃 `00/11` 可得公平 bit；再取两 bit，接受 `00/01/10`、拒绝 `11`，得到均匀三元值。本项目不做前半段纠偏，而是直接信任 `crypto/rand.Int` 的均匀有界契约。
- **追问：** 能不能把一个已知 0.6/0.4 的源直接乘 3 取整？
- **追问回答：** 不能凭直觉缩放；输入状态概率不相等时，简单映射通常保留偏差。必须先证明输出原像具有相等概率，或使用能按目标分布正确拒绝/重采样的算法。
- **项目证据：** [`BoundedRandomSource` 明确要求调用前已经均匀](../../../internal/lottery/domain/weighted_selector.go)、[CryptoSource 委托标准库拒绝采样](../../../internal/lottery/adapter/randomsource/crypto.go)。
- **选型边界：** 如果外部硬件只给有偏 bit，还要验证独立同分布假设；相关性存在时，上述两两消偏也不充分。当前项目不接收这种源，不能声称已解决。
- **来源：** `面经题型启发` [牛客候选人自述的偏置二元源转均匀三元源题型](https://www.nowcoder.com/discuss/353159306365313024)，页面自述不等于快手官方原题；`官方技术事实` [Go `crypto/rand.Int` 的拒绝采样源码](https://go.dev/src/crypto/rand/util.go)用于本项目实际算法事实，上述 `01/10` 等概率由列出的概率乘法直接推导。

## 30. 第 21 节接 API 时，最大的幂等风险是什么？

- **直接回答：** 当前 `Select` 每调用一次都可能取新 ticket。若客户端超时、代理重试或服务在“已选择但响应丢失”后重放请求，就可能得到不同 Award；对 reward 和 no_reward 都是违反“一次抽奖一个最终结果”的风险。第 20 节没有任何状态能识别“这是同一次 Draw”。
- **追问：** 第 21 节最少需要补什么，才能安全处理结果未知？
- **追问回答：** 至少要定义业务请求/Draw 身份，把该身份与唯一最终结果原子持久化，并让同一幂等键返回已有结果；冲突与进行中状态要可查询，响应丢失后先查证而不是重新调用 selector。还要固定 Strategy 版本，并明确随机选择与结果写入的事务/状态机边界。具体表和 API 仍需第 21 节决策，不能写成当前事实。
- **项目证据：** [selector 明确不加载 Strategy、不持久化结果、不预占库存或发奖](../../../internal/lottery/domain/weighted_selector.go)、[`INV-03` 及验证要求](../../product/non-functional-requirements-v1.md)、[既有 API 文档对 no_reward 与重试风险的提醒](../../api/lessons/lesson-17.md)。
- **选型边界：** Redis 锁、请求内 mutex 或“客户端不要重试”都不能单独成为 durable 幂等证据；数据库唯一键加结果记录适合单库起步，跨库存/权益边界则需要 outbox、状态机或可补偿协议，但不应提前堆进纯选择器。
- **来源：** `项目事实` 当前没有 Draw/Result，故 `INV-03` 尚未满足；`官方技术事实` [HTTP 语义中幂等方法的定义](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2)只描述重复请求的预期效果，不会自动替应用创建幂等结果；`面经题型启发` 来自概率题与项目深挖的组合，不声称存在某家公司同文原题。

## 面试回答的收束句

一段准确而不过度宣传的表述是：**“我把随机性拆成领域消费的有界端口和 `crypto/rand.Int` 适配器，用 `[0,total)` 与减法桶避免 modulo bias、off-by-one 和 `MaxUint64` 溢出；用小区间穷举、极值、故障链和并发测试建立证据。同时我明确把选择算法与一次 Draw 的幂等持久化分开，所以第 20 节还不是完整在线抽奖系统，第 21 节必须先解决结果未知时不能重抽。”**
