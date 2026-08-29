# 第 17 节面试问答：最小抽奖领域对象与真实能力边界

本文只描述提交 `0b59217` 中已经实现并可由代码核对的第 17 节时间切片：`internal/lottery/domain` 新增 `Strategy`、`Award`、强类型标识、相对整数权重、`reward` / `no_reward` 结果类别、名称规范化、聚合校验、稳定顺序、切片防御性复制和领域错误。本节建立的是“一个抽奖策略由哪些合法对象组成”，不是“系统已经可以完成一次在线抽奖”。

特别需要先说清楚：本节**没有**随机选择算法、随机源、数据库表、Migration、Repository、业务 API、Redis 业务接入或真实抽奖前端；`AwardOutcomeNoReward` 只是一个可被选择的候选结果类别，不是已经持久化的一次抽奖结果。因此产品不变量 `INV-03`“一次抽奖只能有一个最终结果，结果未知时不能重抽”在本节仍未满足。后续章节计划也不能倒灌成当前项目事实。

## 60 秒项目自述

在进入抽奖建表和接口之前，我先把最小业务语言固化成一个纯 Go 领域包。`Strategy` 是聚合根，拥有至少一个 `Award`；每个 Award 有非零标识、规范化名称、正整数相对权重，以及“有奖励”或“明确未中奖”两种封闭结果类别。权重不用 `float64` 百分比，而用比例整数，因此 `1:3` 与 `100:300` 表达相同分布，同时在聚合构造时检查总和是否溢出 `uint64`。对象字段不导出，只能通过构造器建立有效状态；Strategy 会拒绝重复 Award ID、复制调用方切片、按 Award ID 形成稳定顺序，读取时再次返回副本。测试覆盖合法对象、零值对象、非法 UTF-8、控制字符、128 个 Unicode 码点边界、重复 ID、权重溢出、切片别名和查找行为。这个切片只证明领域配置结构与不变量成立；还没有抽取算法、持久化、并发库存、幂等结果或在线链路，所以我不会把它描述成“抽奖系统已上线”。

## 来源说明

- `项目事实` 只来自当前实现、测试、课程、API 边界、QA、设计手记和 ADR。最终测试执行结果以[第 17 节 QA](../../qa/lessons/lesson-17.md)为准，不能因为本文写了某个测试意图就宣称命令已经通过。
- `官方事实` 优先使用 Go 官方规范、标准库文档、Go 内存模型，以及 Eric Evans 的 DDD Reference 和 Martin Fowler 对 Aggregate、Value Object、Anemic Domain Model 的解释。领域术语帮助组织思考，但不能替代项目自己的业务证据。
- `面经启发` 使用牛客候选人自行发布的复盘，只能证明 DDD、概率题、随机源、加权抽样、Go 并发、map 安全与单元测试等方向曾被讨论；帖子所属公司、轮次和题面均按作者自述看待，本文未独立核验，也不把帖子答案当技术事实。
- 部分牛客旧页面可能因登录、动态渲染或内容迁移只能看到有限信息，因此本文只把其作为题型索引；所有关键结论都回到官方资料和当前代码校准。
- 本章有不少“为什么不提前实现”的项目场景题。找不到完全同构的面经时会明确标记为 `项目场景题`，不虚构企业原题。

## 1. 为什么第 17 节先建领域对象，而不是直接建表或写抽奖接口？

- **直接回答：** 当前最先需要消除的不确定性不是 SQL 语法，而是业务语言：什么是策略、什么是奖项、哪些状态一定非法、未中奖是不是错误。先用不依赖 JSON、SQL、HTTP 的对象和构造器表达这些规则，能够在建表前发现边界问题，也让后续数据库和 API 成为领域模型的适配器，而不是反过来让表字段决定业务概念。本节范围停在对象与不变量，是为了让第 18 节能基于已验证的语义设计两张表，而不是把算法、存储和传输一次性耦合。
- **追问：** 这是不是“过度设计”，用两个 struct 不就够了吗？
  - **追问回答：** 如果 struct 允许零 ID、空名称、零权重、未知结果类别和重复奖项随意进入，错误会延迟到 SQL、接口甚至线上抽取时才暴露。当前封装的成本只有一个小型纯 Go 包和测试，却把非法状态的拒绝点提前到了对象创建时。若业务只是一次性脚本且对象不会跨边界流转，普通 struct 可能更经济；GrowthOS 的对象后续要进入表、Repository、算法和 API，因此提前建立统一语义是有收益的。
- **项目证据：** [领域包说明](../../../internal/lottery/domain/doc.go)、[Strategy 构造与聚合校验](../../../internal/lottery/domain/strategy.go)、[课程正文](../../course/part-03/lesson-17-lottery-domain-objects.md)、[本节 API 边界](../../api/lessons/lesson-17.md)。
- **选型边界：** 当模型只负责无规则的数据搬运、生命周期极短且不会被多个适配器复用时，可以采用更轻的 DTO；一旦规则需要跨入口保持一致，就不能依赖每个 handler 或 SQL 调用者各自校验。
- **来源：** `面经启发` [牛客候选人自述中出现“聊聊领域驱动设计”的项目追问](https://www.nowcoder.com/discuss/703571718387847168)；`官方事实` [DDD Reference](https://www.domainlanguage.com/ddd/reference/)、[Go 官方模块组织建议](https://go.dev/doc/modules/layout)；`项目事实` [ADR-0013](../../decisions/ADR-0013-lottery-domain-model.md)。

## 2. 为什么 `Strategy` 不等于营销 `Activity`，`Award` 也不等于实际发放的 `Benefit`？

- **直接回答：** Strategy 回答“在一组候选结果中如何配置选择空间”，Activity 回答“谁在什么时间、通过什么入口参与哪项营销活动”，Benefit 回答“选中后实际发什么、如何履约”。把三者合并会让可复用的选择策略被活动时间、资格和库存生命周期绑死，也会把“选中了一个奖励描述”误写成“权益已经到账”。本节只建 Lottery 上下文里的 Strategy 和 Award，并通过 `AwardOutcome` 表示是否存在后续奖励，不保存活动、库存或发放状态。
- **追问：** 为什么 Award 不直接包含优惠券模板 ID、积分数量和发放状态？
  - **追问回答：** 那些字段属于具体权益类型与履约流程，目前 Benefit 上下文还没有实现。现在提前塞入会形成跨上下文的联合对象，并让新增奖励类型不断修改 Lottery 核心。未来应在明确的上下文边界上使用引用、快照或防腐层，具体选择取决于一致性与审计需求；本节只保留“有奖励/无奖励”的最小语义。
- **项目证据：** [Award 及结果类别](../../../internal/lottery/domain/award.go)、[限界上下文图](../../product/bounded-context-map-v1.md)、[第一性原理手记](../../design-thinking/lessons/lesson-17.md)。
- **选型边界：** 极小且永不扩展的单活动 demo 可以把活动、奖项和发放写成一个对象；当策略要复用、权益类型会增长、库存与抽取需要不同一致性边界时，必须拆分职责。
- **来源：** `面经启发` [牛客候选人自述中的 DDD、抽奖与库存类项目追问](https://www.nowcoder.com/discuss/515644743280291840)；`官方事实` [DDD Reference 的 Bounded Context 与 Ubiquitous Language](https://www.domainlanguage.com/ddd/reference/)；`项目事实` [第 6 节限界上下文](../../course/part-01/lesson-06-first-bounded-contexts.md)。

## 3. 为什么把 `Strategy` 设计成聚合根，而不是让调用者直接维护 `[]Award`？

- **直接回答：** “至少一个候选项、Award ID 在策略内唯一、每个 Award 本身有效、总权重可安全求和、集合顺序规范化”都是跨集合的不变量，单个 Award 无法独立维护。Strategy 在构造时一次性验证这些规则，并拥有候选集合；调用者只能通过 Strategy 读取，不能直接替换内部切片。它因此是这一小块一致性边界的入口。
- **追问：** 聚合是不是越大越好，把 Activity、库存、用户次数也放进 Strategy 就能一次校验？
  - **追问回答：** 不是。聚合越大，加载、更新与并发冲突范围越大，还会混合不同生命周期。当前 Strategy 只维护配置内部的一致性；用户次数、库存扣减和结果幂等有自己的事实源与并发语义，不能为了“一次事务”就塞进同一个内存对象。
- **项目证据：** [Strategy 聚合实现](../../../internal/lottery/domain/strategy.go)、[聚合集合与查找测试](../../../internal/lottery/domain/strategy_test.go)、[ADR-0013](../../decisions/ADR-0013-lottery-domain-model.md)。
- **选型边界：** 如果 Award 需要独立高频更新或数量大到无法随 Strategy 一起加载，聚合边界需要重评，可能采用受控的领域服务或按版本读取；不能继续假设“全量集合始终廉价”。
- **来源：** `官方事实` [Martin Fowler 对 DDD Aggregate 的说明](https://martinfowler.com/bliki/DDD_Aggregate.html)、[DDD Reference](https://www.domainlanguage.com/ddd/reference/)；`项目事实` [Strategy 不变量测试](../../../internal/lottery/domain/strategy_test.go)。

## 4. `Award` 是实体还是值对象？为什么？

- **直接回答：** 当前 Award 具有 `AwardID`，Strategy 以 ID 判重并提供按 ID 查找，所以即使两个 Award 名称、权重和结果类别完全相同，它们仍可代表两个不同候选项；按当前模型应把它视为策略内部的实体。名称、权重与结果类别更接近值属性。不能因为 Go struct 可以用 `==` 比较，就把业务身份语义误判为值对象。
- **追问：** 测试里为什么可以直接比较两个 Award struct？
  - **追问回答：** 当前字段都是 Go 可比较类型，所以测试可以用语言层面的相等比较核对返回值；这只是实现便利，不改变领域上的身份判断。未来如果 Award 增加 slice、map 或不可比较字段，测试方式可以变化，但 AwardID 的身份语义仍应由业务决策决定。
- **项目证据：** [AwardID 与 Award](../../../internal/lottery/domain/award.go)、[策略内 ID 判重和查找](../../../internal/lottery/domain/strategy.go)、[查找测试](../../../internal/lottery/domain/strategy_test.go)。
- **选型边界：** 若未来业务证明候选项完全由属性定义、无需独立追踪或引用，可以重构为值对象；若 Award 获得独立生命周期，则可能提升为自己的聚合根，但这都需要新的业务证据。
- **来源：** `官方事实` [Martin Fowler 的 Value Object](https://martinfowler.com/bliki/ValueObject.html)、[DDD Reference 的 Entity / Value Object](https://www.domainlanguage.com/ddd/reference/)、[Go 可比较类型规范](https://go.dev/ref/spec#Comparison_operators)；`项目事实` [Award 实现](../../../internal/lottery/domain/award.go)。

## 5. 为什么字段不导出并使用构造器？Go 的零值哲学是不是被破坏了？

- **直接回答：** 业务上不存在 ID 为零、名称为空、权重为零或结果类别未知的合法 Award，也不存在没有候选项的合法 Strategy。字段不导出、构造器集中校验，可以让正常创建路径只返回有效对象。Go 仍允许 `var Award` 得到零值，因此代码没有声称“语言上绝对无法构造非法值”；关键是零值无法通过 `NewStrategy` 的二次校验进入聚合。
- **追问：** 为什么不让零值也成为合法对象，减少构造器？
  - **追问回答：** 如果把零值解释为“未初始化但可用”，后续每个算法、Repository 和 handler 都必须重复判断，且容易遗漏。对计数器、buffer 等自然有用的类型，零值可用很好；对具有外部身份和正权重约束的领域实体，显式构造更准确。
- **项目证据：** [Award 构造与 validate](../../../internal/lottery/domain/award.go)、[Strategy 构造](../../../internal/lottery/domain/strategy.go)、[零值 Award 被拒绝的测试](../../../internal/lottery/domain/strategy_test.go)。
- **选型边界：** 如果未来需要 ORM 先创建空对象再逐字段填充，不应因此放开领域字段；应由持久化映射层读取行后调用重建入口，并再次验证。只有业务明确允许“草稿/未配置”状态时，才应把它建模为显式状态。
- **来源：** `面经启发` [牛客后端面经汇总中的 MVC、DDD 与单元测试类追问](https://www.nowcoder.com/discuss/354318720291926016)；`官方事实` [Go 规范的零值](https://go.dev/ref/spec#The_zero_value)、[Go 模块布局](https://go.dev/doc/modules/layout)；`项目事实` [领域错误](../../../internal/lottery/domain/errors.go)。

## 6. 为什么定义 `StrategyID`、`AwardID`，而不是所有地方都用 `uint64`？

- **直接回答：** 两者底层都是 `uint64`，但 Go 的命名类型能在编译期区分策略身份和奖项身份，避免把 AwardID 误传给需要 StrategyID 的函数；类型名也把业务语言带进签名。`0` 被保留为无效值，由构造器拒绝，非零值才表示持久身份。
- **追问：** 既然都能显式转换，强类型还有价值吗？
  - **追问回答：** 显式转换正好让跨语义转换变得可见并可审查，普通调用不会悄悄混用。它不能阻止故意错误转换，也没有自动解决分布式 ID 生成、租户隔离或数据库范围问题；那些是后续独立决策。
- **项目证据：** [StrategyID](../../../internal/lottery/domain/strategy.go)、[AwardID](../../../internal/lottery/domain/award.go)、[构造器 ID 失败测试](../../../internal/lottery/domain/award_test.go)。
- **选型边界：** 只在非常局部、不会交叉的计算变量上，直接整数足够；跨模块、跨表或接口传递的领域身份应保持语义类型，并在适配器边界做显式解析与范围校验。
- **来源：** `官方事实` [Go 规范中的类型定义与底层类型](https://go.dev/ref/spec#Type_definitions)、[可赋值性规则](https://go.dev/ref/spec#Assignability)；`项目事实` [ID 错误定义](../../../internal/lottery/domain/errors.go)。

## 7. 名称校验为什么按 Unicode 码点计数，并同时检查 UTF-8、空白和控制字符？

- **直接回答：** 用户看到的是字符而不是 UTF-8 字节，因此 128 的上限按 rune 计数，避免中文名称因为每个字符占多个字节而被不公平截短。构造器先拒绝非法 UTF-8，再 `TrimSpace`，拒绝空名称和任意 Unicode 控制字符，最后检查最多 128 个码点；成功后保存规范化名称。这样领域、未来数据库和 UI 至少共享一个明确的字符级契约。
- **追问：** 这是否已经解决 Unicode 安全、同形异义字符和 XSS？
  - **追问回答：** 没有。当前没有做 NFC/NFKC 归一化、混淆字符检测、敏感词处理或 HTML 转义。控制字符拒绝能减少日志换行等意外，但前端输出仍必须按上下文转义，数据库列宽也必须按实际字符集设计，不能把名称校验夸大为完整内容安全方案。
- **项目证据：** [名称规范化实现](../../../internal/lottery/domain/name.go)、[Award 名称边界测试](../../../internal/lottery/domain/award_test.go)、[Strategy 名称边界测试](../../../internal/lottery/domain/strategy_test.go)。
- **选型边界：** 若产品需要搜索去重、多语言等价、法规过滤或表情支持，需要新增清晰的规范化与审核运营策略；不能在不评估历史数据兼容性的情况下更换规范化规则。
- **来源：** `官方事实` [Go 规范的 UTF-8 源码与码点说明](https://go.dev/ref/spec#Source_code_representation)、[`unicode/utf8`](https://pkg.go.dev/unicode/utf8)、[`unicode`](https://pkg.go.dev/unicode)；`项目事实` [128-rune 契约](../../../internal/lottery/domain/name.go)。

## 8. 为什么权重使用正整数相对值，而不是 `float64` 百分比或固定万分比？

- **直接回答：** 抽奖配置首先需要表达比例，而不是强迫运营输入某个固定分母。正整数权重使 `1:3` 和 `100:300` 具有相同数学分布，避免把十进制文本直接映射到二进制浮点后产生边界比较和累计误差，也允许以后使用整数区间做加权选择。当前只存相对权重，没有把 `400` 解释成 40% 或万分之 400；单项概率要由 `weight / totalWeight` 推导。
- **追问：** 那为什么不直接用 DECIMAL 百分比，运营更容易理解？
  - **追问回答：** 展示层完全可以接收百分比或小数，但必须在适配器里定义精度、舍入和总和规则，再转换为领域可验证的整数比例。若业务要求“精确到万分之一且总和必须为 10000”，固定分母会更利于审计；当前最小对象没有这项需求，所以不提前锁死。
- **项目证据：** [Weight 定义和注释](../../../internal/lottery/domain/award.go)、[任意分母测试](../../../internal/lottery/domain/strategy_test.go)、[设计权衡](../../design-thinking/lessons/lesson-17.md)。
- **选型边界：** 需要法规审计、跨系统精确交换或运营固定刻度时，应引入明确分母/有理数协议；权重动态跨度极大时还要评估配置可读性和采样算法性能。
- **来源：** `面经启发` [牛客候选人自述中的“按 0.6 概率输出”题型](https://www.nowcoder.com/discuss/353154090437910528)、[牛客候选人自述中的浮点加权采样题型](https://www.nowcoder.com/discuss/353154373217886208)；`官方事实` [Go 浮点类型规范](https://go.dev/ref/spec#Numeric_types)；`项目事实` [整数权重实现](../../../internal/lottery/domain/award.go)。

## 9. 为什么要在构造 Strategy 时检查总权重溢出？Go 的无符号整数不是会自动回绕吗？

- **直接回答：** 正因为运行时无符号整数运算会按模 `2^n` 计算，溢出后总权重可能变成很小的数甚至零，后续抽取区间和实际候选权重就不一致。`NewStrategy` 在每次相加前检查 `weight > math.MaxUint64-totalWeight`，发现溢出立即返回 `ErrTotalWeightOverflow`；恰好等于 `MaxUint64` 的总和仍被允许。
- **追问：** 允许 `MaxUint64` 会不会让未来算法出现 `total+1` 溢出？
  - **追问回答：** 会，所以第 20 节选择算法必须使用半开区间 `[0,total)` 或其他不会计算 `total+1` 的方式，并为最大值写边界测试。本节只保证总和本身准确，不保证尚未实现的随机 API 能处理该范围；若实际算法无法安全支持，就应收紧领域上限并以 ADR/Migration 说明兼容影响。
- **项目证据：** [溢出保护](../../../internal/lottery/domain/strategy.go)、[最大总和与溢出测试](../../../internal/lottery/domain/strategy_test.go)、[领域错误](../../../internal/lottery/domain/errors.go)。
- **选型边界：** 若配置入口已经有远小于 `uint64` 的业务上限，仍应保留领域防线，同时可增加更易理解的运营上限；不能仅依赖数据库列或前端输入限制。
- **来源：** `官方事实` [Go 规范的整数溢出语义](https://go.dev/ref/spec#Integer_overflow)、[`math.MaxUint64`](https://pkg.go.dev/math#MaxUint64)；`项目事实` [权重边界测试](../../../internal/lottery/domain/strategy_test.go)。

## 10. 为什么把“未中奖”建模为 `AwardOutcomeNoReward`，而不是返回 `nil` 或 error？

- **直接回答：** 用户合法参与并抽到“谢谢参与”是一次成功完成的业务结果，不是系统故障；如果用 `nil`，调用者无法区分“明确未中奖”“没有执行抽取”“奖项配置丢失”。显式候选项有自己的 ID、名称和权重，未来算法可以像其他 Award 一样选择它，观测与审计也能区分业务 miss 和技术 error。
- **追问：** `no_reward` 是否等于一次最终 DrawResult？
  - **追问回答：** 不等于。它只是 Award 的分类；当前没有 DrawID、参与者、发生时间、策略版本、幂等键或结果存储。只有后续抽取并以不可重复的方式落下结果，才能满足“一次抽奖一个最终结果”。
- **项目证据：** [AwardOutcome 定义](../../../internal/lottery/domain/award.go)、[明确未中奖测试](../../../internal/lottery/domain/award_test.go)、[产品不变量](../../product/non-functional-requirements-v1.md)。
- **选型边界：** 如果某业务规定所有参与者必得权益，可以在更上层策略规则中禁止 no-reward；仍不应把基础设施错误伪装成未中奖，技术失败需要独立错误通道。
- **来源：** `面经启发` [牛客候选人自述中的分段概率题型](https://www.nowcoder.com/discuss/353153938134343680)；`官方事实` [DDD Reference 的显式领域语言](https://www.domainlanguage.com/ddd/reference/)；`项目事实` [AwardOutcome](../../../internal/lottery/domain/award.go)。

## 11. 为什么单个 Award、重复名称、甚至全是 `no_reward` 的 Strategy 都允许创建？

- **直接回答：** 本节只守住“结构上能形成明确选择空间”的不变量：至少一个有效候选、身份唯一、权重为正、总和安全。单候选策略代表确定性选择；名称只是展示文本，不是身份，所以可以重复；全未中奖策略在数学与结构上仍可选择。是否允许运营发布这种策略属于活动政策、审核和生命周期规则，尚未实现，不能偷偷写死在最底层对象里。
- **追问：** 全未中奖不会伤害用户体验吗？为什么不提前禁止？
  - **追问回答：** 可能会，所以未来发布校验应结合活动类型、合规和运营权限判断。但也可能存在维护降级、资格不满足后的明确结果或测试策略。没有业务证据时，底层聚合只拒绝必然无意义或计算不安全的状态，政策性限制留给有上下文的规则层。
- **项目证据：** [单候选、任意分母、重复名称和全 miss 测试](../../../internal/lottery/domain/strategy_test.go)、[Strategy 构造规则](../../../internal/lottery/domain/strategy.go)、[课程边界](../../course/part-03/lesson-17-lottery-domain-objects.md)。
- **选型边界：** 一旦产品明确规定“上线策略至少一个 reward”“奖励率不得低于某值”或“名称必须唯一”，应建立可版本化的发布规则及错误码，而不是悄悄改变历史对象的读取语义。
- **来源：** `面经启发` [牛客候选人自述中的抽奖库存与领域设计追问](https://www.nowcoder.com/discuss/515644743280291840)；`官方事实` [DDD Reference](https://www.domainlanguage.com/ddd/reference/)；`项目事实` [边界测试](../../../internal/lottery/domain/strategy_test.go)。

## 12. 为什么同一 Strategy 内拒绝重复 Award ID，却允许重复名称？

- **直接回答：** ID 承担稳定身份，重复会让 `Award(id)` 查找、未来持久化主键/外键和结果引用产生歧义；名称只是展示属性，两个不同奖项可以都叫“神秘礼盒”。`NewStrategy` 用局部 map 检查 ID，重复时用 `%w` 包装 `ErrDuplicateAwardID` 并附带冲突 ID。
- **追问：** 为什么不让全系统 Award ID 唯一？
  - **追问回答：** 当前代码只证明策略内唯一，因为 Award 的生命周期边界还没有通过数据库设计确定。第 18 节若选择全局主键，存储会提供更强约束；若选择 `(strategy_id, award_id)` 复合身份，则策略内唯一就是准确语义。本节不能预先宣称数据库方案。
- **项目证据：** [ID 判重实现](../../../internal/lottery/domain/strategy.go)、[重复 ID 测试](../../../internal/lottery/domain/strategy_test.go)、[第 18 节之前的 API/存储空白](../../api/lessons/lesson-17.md)。
- **选型边界：** 如果产品后来要求同名也唯一，应先定义大小写、空白、Unicode 归一化和作用域；直接给 name 加唯一索引可能与当前允许重复的领域契约冲突。
- **来源：** `官方事实` [Go map 类型规范](https://go.dev/ref/spec#Map_types)、[`fmt.Errorf` 的 `%w`](https://pkg.go.dev/fmt#Errorf)、[`errors.Is`](https://pkg.go.dev/errors#Is)；`项目事实` [重复 ID 错误](../../../internal/lottery/domain/errors.go)。

## 13. 为什么 Strategy 会按 Award ID 排序，而不保留调用者传入顺序？

- **直接回答：** 当前领域没有“配置顺序影响概率或展示”的规则，保留任意输入顺序只会让同一组对象因数据库返回顺序或 map 遍历顺序不同而产生不稳定快照。构造器复制后按 Award ID 排序，使 `Awards()` 的遍历顺序可预测，便于测试、序列化比较和未来缓存键设计；排序本身不代表优先级，也不改变相对权重。
- **追问：** 如果前端转盘需要运营配置的展示顺序怎么办？
  - **追问回答：** 那是新的业务属性，应新增显式 `DisplayOrder` 并定义唯一性、缺省值与迁移规则，而不是借用输入偶然顺序。抽取算法也不应把展示顺序误当概率优先级。
- **项目证据：** [规范排序实现](../../../internal/lottery/domain/strategy.go)、[乱序输入的稳定顺序测试](../../../internal/lottery/domain/strategy_test.go)、[设计手记](../../design-thinking/lessons/lesson-17.md)。
- **选型边界：** 若领域明确规定顺序本身有语义，例如优先级规则链，就必须把顺序建模并持久化；当前按 ID 排序的约定需要版本化调整。
- **来源：** `官方事实` [`slices.SortFunc`](https://pkg.go.dev/slices#SortFunc)、[`cmp.Compare`](https://pkg.go.dev/cmp#Compare)、[Go map 遍历顺序未指定](https://go.dev/ref/spec#For_statements)；`项目事实` [排序测试](../../../internal/lottery/domain/strategy_test.go)。

## 14. 为什么构造时和读取时都要复制 Award slice？

- **直接回答：** Go slice 是指向底层数组的描述符。若 Strategy 直接保存调用者切片，调用者在构造成功后修改元素就能绕过聚合校验；若 `Awards()` 直接返回内部切片，读取者也能修改聚合状态。构造时复制一次取得所有权，读取时再复制一次隔离返回值，配合 Award 的不导出字段形成 API 层面的不可变纪律。
- **追问：** 这是否意味着 Strategy 是“深度不可变”并且任何并发都安全？
  - **追问回答：** 当前 Award 只含值字段，所以 slice 复制足以隔离元素；Strategy 本身没有写方法，多 goroutine 只读通常没有共享写竞争。但 Go 没有通用不可变类型，`unsafe`、反射或未来新增引用字段都会改变前提。若 Award 后续加入 map/slice/指针，需要重新定义深拷贝或只读视图。
- **项目证据：** [构造和读取的两次复制](../../../internal/lottery/domain/strategy.go)、[输入/返回切片别名测试](../../../internal/lottery/domain/strategy_test.go)、[第 17 节 QA](../../qa/lessons/lesson-17.md)。
- **选型边界：** 候选数巨大或读取极高频时，每次复制可能成为成本热点，可改用迭代器、不可变快照或受控 callback；必须以 benchmark 与所有权证明为依据，不能先暴露内部 slice 再补锁。
- **来源：** `官方事实` [Go 官方 Slice 内部结构与共享底层数组](https://go.dev/blog/slices-intro)、[Go 内存模型](https://go.dev/ref/mem)；`项目事实` [所有权测试](../../../internal/lottery/domain/strategy_test.go)。

## 15. 为什么使用可被 `errors.Is` 识别的领域错误，而不是返回字符串或一个通用参数错误？

- **直接回答：** 每个失败原因都有稳定 sentinel，例如 `ErrAwardWeightRequired`、`ErrDuplicateAwardID` 和 `ErrTotalWeightOverflow`。上层可以用 `errors.Is` 做类别映射，而不用解析易变化的人类文本；重复 ID 同时用 `%w` 保留类别并附加冲突 ID，便于诊断。领域包没有直接决定 HTTP 状态码或日志格式，这些属于适配器。
- **追问：** 为什么不直接定义带字段的统一 ValidationError？
  - **追问回答：** 当前错误数量小，sentinel 足以支撑测试与上层分类，结构化错误会增加 API 和兼容成本。若后续需要一次返回多个字段错误、错误路径、国际化消息或可机器修复建议，再引入结构化类型更合适；仍应保证 `errors.Is/As` 链可用。
- **项目证据：** [领域错误清单](../../../internal/lottery/domain/errors.go)、[重复 ID 包装](../../../internal/lottery/domain/strategy.go)、[errors.Is 测试](../../../internal/lottery/domain/strategy_test.go)。
- **选型边界：** 错误要跨进程传输时不能直接暴露 Go error 文本，应映射为版本化错误码；日志也不应把用户输入或内部对象无界输出。
- **来源：** `官方事实` [`errors.Is`](https://pkg.go.dev/errors#Is)、[`errors.As`](https://pkg.go.dev/errors#As)、[`fmt.Errorf`](https://pkg.go.dev/fmt#Errorf)；`项目事实` [错误定义](../../../internal/lottery/domain/errors.go)。

## 16. 为什么领域包没有 JSON tag、SQL tag、Gin 类型或 Redis key？

- **直接回答：** 领域对象要表达业务不变量，不应同时承担 HTTP 字段命名、数据库列映射和缓存编码。当前 `internal/lottery/domain` 只依赖 Go 标准库，包注释明确排除 JSON、SQL、cache 和 HTTP；后续 transport DTO、数据库 row model 与 cache snapshot 可以各自演进，并在边界调用领域构造器重建合法对象。
- **追问：** 多写几层映射是不是纯粹增加样板代码？
  - **追问回答：** 映射确实有成本，所以只在语义或生命周期不同的边界建立。这里 HTTP 可选字段、SQL NULL/DECIMAL 和领域必填值显然不同，直接复用一个 struct 会让存储迁移或 API 兼容修改污染核心规则。若某个内部工具的 DTO 与领域恰好稳定一致，可以用小型显式转换，不必为了层数制造空壳接口。
- **项目证据：** [领域包边界](../../../internal/lottery/domain/doc.go)、[纯领域类型](../../../internal/lottery/domain/award.go)、[本节 API 明确无新增端点](../../api/lessons/lesson-17.md)、[ADR-0013](../../decisions/ADR-0013-lottery-domain-model.md)。
- **选型边界：** 纯 CRUD 且没有领域规则的小模块可以合并持久化模型；一旦出现独立不变量、多个适配器或兼容节奏差异，应恢复边界，避免“一个 struct 走天下”。
- **来源：** `面经启发` [牛客候选人自述中的 MVC、DDD 与项目分层题型](https://www.nowcoder.com/discuss/354318720291926016)；`官方事实` [Go 官方模块布局](https://go.dev/doc/modules/layout)、[DDD Reference 的 Layered Architecture](https://www.domainlanguage.com/ddd/reference/)；`项目事实` [包注释](../../../internal/lottery/domain/doc.go)。

## 17. 当前模型会不会是“贫血模型”？只有 getter 和校验算领域行为吗？

- **直接回答：** 贫血模型的核心问题不是 getter 数量，而是业务规则全部散落到 service，领域对象只当数据袋。当前对象已经把创建合法性、集合唯一性、权重总和、稳定顺序和所有权封装在 Strategy/Award 内，因此不是完全无行为的数据袋。不过本节业务行为确实很少，因为抽取算法尚未实现；不能为了显得“充血”就提前编造库存、资格或发放方法。
- **追问：** 将来加抽取算法时，一定要写成 `strategy.Draw()` 吗？
  - **追问回答：** 不一定。若算法只依赖 Strategy 快照和可注入随机源，方法或领域服务都可行；若还需要库存、用户次数和持久化事务，把所有依赖塞进实体方法会破坏边界。应根据不变量和依赖所有权决定，而不是按 DDD 标签决定方法放置。
- **项目证据：** [Strategy 构造行为](../../../internal/lottery/domain/strategy.go)、[Award 本地不变量](../../../internal/lottery/domain/award.go)、[设计手记的候选方案](../../design-thinking/lessons/lesson-17.md)。
- **选型边界：** 当业务规则持续增长却全部进入 application service，应该重评领域行为归属；当对象只是查询投影或报表 DTO，则“贫血”并不自动是问题。
- **来源：** `官方事实` [Martin Fowler 的 Anemic Domain Model](https://martinfowler.com/bliki/AnemicDomainModel.html)、[DDD Reference](https://www.domainlanguage.com/ddd/reference/)；`面经启发` [牛客 DDD 项目追问](https://www.nowcoder.com/discuss/703571718387847168)；`项目事实` [领域实现](../../../internal/lottery/domain)。

## 18. 现在已经有权重和总权重，为什么还不能说“随机抽奖已实现”？

- **直接回答：** 权重只定义选择空间，当前没有生成随机位置、累计区间定位、错误处理或返回 DrawResult 的代码。`TotalWeight()` 是未来算法所需输入，不是算法本身；单元测试也没有做任何抽取次数或分布断言。准确表述应是“完成可供后续抽取算法使用的合法策略快照”。
- **追问：** 最简单的加权算法会怎么做？
  - **追问回答：** 第 20 节可以注入一个返回 `[0,total)` 的随机源，再按稳定候选集合累计权重并定位区间；也可以预计算前缀和后二分。候选数少、配置读多写少时线性扫描足够，规模与吞吐上升后再评估前缀和、alias method 等方案。这里只是未来候选方案，不是当前实现事实。
- **项目证据：** [Strategy 只暴露候选与总权重](../../../internal/lottery/domain/strategy.go)、[当前测试没有抽取算法](../../../internal/lottery/domain/strategy_test.go)、[课程路线第 20 节边界](../../course/README.md)。
- **选型边界：** 权重经常动态变化时，昂贵预计算可能不划算；需要无放回抽样、库存联动或多阶段规则时，简单累计算法不再覆盖完整需求。
- **来源：** `面经启发` [牛客候选人自述中的分段概率题型](https://www.nowcoder.com/discuss/353153938134343680)、[带权不重复抽样题型](https://www.nowcoder.com/discuss/353158472894193664)；`官方事实` [`math/rand/v2`](https://pkg.go.dev/math/rand/v2)；`项目事实` [第 17 节 API 边界](../../api/lessons/lesson-17.md)。

## 19. 未来随机源应该选 `math/rand/v2` 还是 `crypto/rand`？为什么本节不决定？

- **直接回答：** 两者解决的问题不同。`math/rand/v2` 面向快速伪随机模拟，适合可注入、可重复的普通概率选择；`crypto/rand` 提供密码学安全随机字节，适合结果不可预测是安全需求的场景。营销抽奖是否要求对用户不可预测、是否有合规审计、吞吐预算和可复现测试要求尚未形成证据，所以本节只建立整数权重，不绑定任何随机包。
- **追问：** 为了单测固定 seed，生产也用固定 seed 可以吗？
  - **追问回答：** 不可以把测试确定性直接带入生产。正确方式是定义窄随机源接口：测试注入有限、可预测序列，生产按威胁模型注入实现；还要处理随机源失败、范围偏差和并发安全。即便用 `crypto/rand`，库存、幂等和审计问题也不会自动解决。
- **项目证据：** [当前 domain 无随机依赖](../../../internal/lottery/domain/doc.go)、[权重模型](../../../internal/lottery/domain/award.go)、[被推迟能力](../../design-thinking/lessons/lesson-17.md)。
- **选型边界：** 内部无价值实验可优先性能和可复现；涉及真实财产、攻击者可观察大量结果或监管要求时，应进行威胁建模并评估密码学随机、审计记录和第三方公证，不能只换一个包名。
- **来源：** `面经启发` [牛客候选人自述中的有限随机源题型](https://www.nowcoder.com/discuss/544867105775026176)、[0.6 概率题型](https://www.nowcoder.com/discuss/353154090437910528)；`官方事实` [`math/rand/v2` 文档](https://pkg.go.dev/math/rand/v2)、[`crypto/rand` 文档](https://pkg.go.dev/crypto/rand)；`项目事实` [包依赖边界](../../../internal/lottery/domain/doc.go)。

## 20. 概率代码应该怎样测试？为什么“跑一万次接近 40%”不能替代单元测试？

- **直接回答：** 确定性单测应注入边界随机值，逐个验证半开区间、第一项、最后一项、单候选、最大总权重和随机源错误；统计测试只能在足够样本和明确容差下发现严重分布偏差，却可能偶发失败，也无法精确定位 off-by-one。本节还没有算法，所以现有测试只验证对象不变量，不存在“概率已验证”的结论。
- **追问：** 那统计测试完全没用吗？
  - **追问回答：** 有用，但应作为算法属性/离线验证，固定置信标准、记录 seed 或样本，并避免成为脆弱的每次提交门禁。更重要的是先证明区间覆盖无空洞、无重叠且权重和一致，再用统计证据补充，不要用一次绿色频率掩盖边界错误。
- **项目证据：** [Award 表驱动测试](../../../internal/lottery/domain/award_test.go)、[Strategy 边界测试](../../../internal/lottery/domain/strategy_test.go)、[第 17 节 QA](../../qa/lessons/lesson-17.md)。
- **选型边界：** 对监管抽奖或高价值权益，需进一步做可审计随机性、独立验证和线上分布监控；普通单元测试和一次本地模拟不足以满足合规证明。
- **来源：** `面经启发` [牛客 0.6 概率题型](https://www.nowcoder.com/discuss/353154090437910528)、[浮点加权采样题型](https://www.nowcoder.com/discuss/353154373217886208)；`官方事实` [Go `testing` 包](https://pkg.go.dev/testing)、[`math/rand/v2` 文档](https://pkg.go.dev/math/rand/v2)；`项目事实` [当前测试范围](../../../internal/lottery/domain/strategy_test.go)。

## 21. 当前 Strategy 可以被多个 goroutine 并发读取吗？为什么代码里没有锁？

- **直接回答：** 构造完成后没有任何写方法，字段不导出，输入切片被复制，`Awards()` 返回新副本；查找只遍历内部只读 slice。构造器使用的 map 是函数局部变量，完成后不会存入 Strategy。因此在不使用 `unsafe`、且未来字段仍保持只读值语义的前提下，多 goroutine 并发调用只读方法没有共享写竞争，不需要为了“可能并发”提前加锁。
- **追问：** `go test -race` 通过是否证明绝对线程安全？
  - **追问回答：** Race detector 只能发现实际执行路径上的数据竞争，不能证明所有路径、原子性或业务一致性。当前测试主要并行运行构造与只读行为；如果未来加入可变缓存、懒加载或共享 RNG，必须重新建立同步关系和针对性并发测试。
- **项目证据：** [Strategy 只读 API](../../../internal/lottery/domain/strategy.go)、[防御性复制测试](../../../internal/lottery/domain/strategy_test.go)、[QA 中的 race 结果](../../qa/lessons/lesson-17.md)。
- **选型边界：** 一旦聚合允许原地修改，或内部新增 map/slice 的可变共享，就必须选择单线程所有权、锁、copy-on-write 或不可变快照；不要假设“Go map 读写偶尔也能工作”。
- **来源：** `面经启发` [牛客 Go 并发题型复盘](https://www.nowcoder.com/discuss/498230763914010624)、[牛客“map 不安全如何解决”题型](https://www.nowcoder.com/discuss/622461750427832320)；`官方事实` [Go 内存模型](https://go.dev/ref/mem)、[Go Race Detector](https://go.dev/doc/articles/race_detector)、[Go map 规范](https://go.dev/ref/spec#Map_types)；`项目事实` [Strategy 实现](../../../internal/lottery/domain/strategy.go)。

## 22. 当前对象校验解决了哪些安全与可观测性问题，又没有解决什么？

- **直接回答：** 非法 UTF-8、控制字符、空名称、超长名称、零 ID、零权重和总和溢出会在领域入口被拒绝，减少脏数据、日志换行污染和计算边界异常。错误类别稳定，未来适配器可以映射指标或错误码。但本节没有 handler、鉴权、审计日志、trace、指标、XSS 转义、速率限制或敏感信息策略，所以不能说已经建立业务安全与可观测体系。
- **追问：** 领域层是否应该直接打日志，记录所有非法配置？
  - **追问回答：** 不应让纯领域对象决定日志目的地、request ID 或用户身份。领域返回稳定错误，上层在拥有调用上下文时按采样、隐私和告警策略记录；否则会产生重复日志、无法关联请求，甚至把原始用户输入泄露出去。
- **项目证据：** [名称与数值校验](../../../internal/lottery/domain/name.go)、[领域错误](../../../internal/lottery/domain/errors.go)、[API 尚未新增](../../api/lessons/lesson-17.md)、[QA 证据边界](../../qa/lessons/lesson-17.md)。
- **选型边界：** 当出现真实运营写入接口时，必须增加身份与权限、请求大小、结构化错误、审计与指标；领域校验只能作为纵深防线之一，不能替代边界安全。
- **来源：** `官方事实` [Go `unicode/utf8`](https://pkg.go.dev/unicode/utf8)、[Go 错误链](https://pkg.go.dev/errors)；`面经启发` [牛客项目围绕中间件与日志继续追问的复盘](https://www.nowcoder.com/discuss/703571718387847168)；`项目事实` [本节设计手记](../../design-thinking/lessons/lesson-17.md)。

## 23. 第 18 节建表和 Repository 应如何映射这些对象？当前已经完成了吗？

- **直接回答：** 当前完全没有 Lottery 业务表、Migration 或 Repository。下一节设计至少要逐项保存 Strategy 身份/名称和 Award 身份/名称/权重/结果类别，并让数据库约束、事务边界和领域构造器保持一致；读取行后仍应调用领域重建逻辑，不能把 SQL 扫描成功等同于领域对象合法。具体主键、外键、唯一约束、字符集、权重列类型和删除策略必须在第 18 节基于查询与生命周期决定。
- **追问：** 数据库有 CHECK/UNIQUE 以后，领域校验是否可以删除？
  - **追问回答：** 不可以简单删除。数据库约束保护所有写入路径，领域校验提供更早、更语义化的失败；两者是纵深防御。需要避免的是两套规则无版本管理地漂移，因此 Migration、row mapping、构造器和契约测试必须一起演进。
- **项目证据：** [当前纯领域包](../../../internal/lottery/domain/doc.go)、[课程数据库节奏](../../course/README.md)、[本节 API/存储边界](../../api/lessons/lesson-17.md)、[ADR-0013](../../decisions/ADR-0013-lottery-domain-model.md)。
- **选型边界：** 如果将来从 MySQL 转为事件存储，重建路径和并发控制会变化，但领域不变量仍应可复用；若数据库成为跨语言共享事实源，约束与 schema contract 的权重会更高。
- **来源：** `面经启发` [牛客 DDD、抽奖与库存项目追问](https://www.nowcoder.com/discuss/515644743280291840)、[牛客项目中 MySQL 表与 ORM 追问](https://www.nowcoder.com/discuss/703571718387847168)；`官方事实` [DDD Reference 的 Repository 概念](https://www.domainlanguage.com/ddd/reference/)；`项目事实` [第 17 节课程](../../course/part-03/lesson-17-lottery-domain-objects.md)。

## 24. 当前是否满足 `INV-03`：一次抽奖只有一个最终结果，结果未知不能重抽？

- **直接回答：** 不满足。当前只有可复用的策略配置和候选 Award，没有一次参与的身份、DrawID、幂等键、策略版本快照、随机选择、结果持久化或唯一约束。`AwardOutcome` 描述候选项类型，不能证明某个用户的一次请求已经产生且只产生一个最终结果。
- **追问：** 后续最少需要哪些机制才能开始证明它？
  - **追问回答：** 至少需要稳定的参与/抽奖标识、在持久化层以唯一约束或原子状态机保存首次结果、重复请求返回同一事实，以及故障发生在“已选择但响应丢失”时仍能恢复已有结果。库存扣减和权益发放还要有各自的幂等与一致性协议。具体在哪一节实现必须以课程和实际提交为准，不能由本文提前宣称。
- **项目证据：** [产品不变量 INV-03](../../product/non-functional-requirements-v1.md)、[当前 Strategy 只保存配置](../../../internal/lottery/domain/strategy.go)、[本节 QA 的未实现清单](../../qa/lessons/lesson-17.md)、[本节 API 边界](../../api/lessons/lesson-17.md)。
- **选型边界：** 单机内存 demo 可以用进程内 map 暂存结果，但进程重启、水平扩展和响应丢失都会打破保证；进入在线业务必须依赖可恢复的权威存储与原子协议。
- **来源：** `面经启发` [牛客候选人自述中的抽奖、库存一致性追问](https://www.nowcoder.com/discuss/515644743280291840)、[Go 并发场景题型](https://www.nowcoder.com/discuss/498230763914010624)；`官方事实` [Go 内存模型只定义并发可见性、不会提供业务幂等](https://go.dev/ref/mem)、[DDD Aggregate](https://martinfowler.com/bliki/DDD_Aggregate.html)；`项目事实` [NFR](../../product/non-functional-requirements-v1.md)。

## 不能夸大的事实

1. **不能说已经实现随机抽奖。** 当前没有随机源、区间选择、抽取方法或分布测试；只有合法配置对象。
2. **不能说已经完成抽奖数据持久化。** 本节没有 Strategy/Award 表、业务 Migration、Repository、事务或真实数据读取。
3. **不能说已经提供 Lottery API。** 没有新增 Gin 路由、请求/响应 DTO、鉴权、幂等键或业务错误映射。
4. **不能说 React 抽奖页已经接入。** 现有业务页仍是 Mock；真实业务页属于后续章节。
5. **不能说 Redis 已用于抽奖。** Compose 中的 Redis 仍是隔离环境能力，没有 Lottery key、缓存、锁、Lua 或一致性设计。
6. **不能说库存与发奖已经解决。** Award 只描述候选结果，既没有库存也没有 Benefit 发放状态。
7. **不能说满足 `INV-03`。** `AwardOutcomeNoReward` 不是持久化 DrawResult，更不能保证未知结果不重抽。
8. **不能说采用了完整 DDD。** 当前只是一个受 DDD 边界与聚合思想指导的最小领域包，没有因此自动获得所有 DDD 战术/战略模式。
9. **不能说对象在语言层面绝对不可变。** 这是通过不导出字段、无写方法和切片复制建立的 API 纪律；Go 零值仍存在，未来新增引用字段要重新审计。
10. **不能说 race 绿色证明所有并发正确。** 实际命令结果看 QA，Race Detector 也只覆盖被执行路径，且不证明业务原子性。
11. **不能把权重当百分比。** `400` 只是相对整数；没有固定 100、10000 或其他分母，也没有概率审计。
12. **不能把 128 rune 当数据库列已匹配。** 数据库表尚不存在，未来需明确字符集、列宽和迁移契约。
13. **不能把名称校验说成完整内容安全。** 当前没有 Unicode 归一化、同形字检测、HTML 转义、鉴权或审计。
14. **不能把牛客页面说成企业官方题库。** 它们是作者自述和题型线索，技术事实由官方文档与项目代码校准。

## 复习清单

- [ ] 60 秒内说清“Strategy 聚合 + Award 实体 + 相对整数权重 + 显式未中奖 + 防御性复制”，并主动补一句“尚无算法/DB/API”。
- [ ] 能画出 `Strategy 1 ── * Award` 的当前对象图，同时把 Activity、Benefit、DrawResult 画在边界外。
- [ ] 能从代码指出 ID、名称、权重、结果类别、至少一个 Award、ID 唯一与总和不溢出分别在哪里校验。
- [ ] 能解释 `1:3 == 100:300`，并说明为什么 `400` 不是 40%。
- [ ] 能口述 `MaxUint64` 边界，以及未来算法为什么要用 `[0,total)` 避免 `total+1`。
- [ ] 能解释 `no_reward` 是合法候选，不是 error、nil 或已经持久化的 DrawResult。
- [ ] 能说明为何允许单候选、重复名称和全 miss，同时指出发布政策属于后续规则。
- [ ] 能现场解释 Go slice 别名风险，并指出构造与 getter 两处复制。
- [ ] 能解释按 Award ID 排序只是稳定性契约，不是优先级或概率顺序。
- [ ] 能说明 sentinel error、`%w`、`errors.Is` 各自解决什么，并拒绝在 transport 中暴露原始错误文本。
- [ ] 能解释当前并发只读为何不需要锁，以及哪些未来变化会让这个结论失效。
- [ ] 能对比 `math/rand/v2` 与 `crypto/rand`，但不把任何一个说成当前已采用。
- [ ] 能设计未来算法的确定性边界测试与统计验证，并说出两者各自证明不了什么。
- [ ] 能从 `INV-03` 推导 DrawID、首次结果持久化、重复请求返回同一结果和响应丢失恢复需求。
- [ ] 能逐一打开代码、测试、课程、API、QA、设计手记和 ADR 的相对链接，核对口述没有超过当前提交事实。
- [ ] 能明确说明牛客面经只证明题型出现，领域/Go 技术结论来自官方资料和项目证据。
