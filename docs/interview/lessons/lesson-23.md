# 第 23 节面试题：Lottery 规则需求、领域边界与渐进式架构

本文面向“项目深挖 + 系统设计 + 规则引擎选型 + 权限边界”类面试。第 23 节是需求与架构章节：它没有新增 Go 业务代码、Rule 接口、Migration、Redis 业务访问、HTTP 路由或前端资格逻辑。回答时应把“有意识地不提前实现”说成基于事实和风险做出的范围决策，而不是把尚未完成包装成已经落地。

## 60 秒项目自述

> 第 17～22 节已经形成一个最小 Lottery 纵向切片：MySQL 保存 Strategy/Award，Repository 恢复一致快照，WeightedSelector 按完整 uint64 相对权重无偏选择，development/test ephemeral API 和 React 页面展示服务端候选。但这条链只回答“给定合法 Strategy 怎样选 Award”，不能回答用户是否有资格、活动是否发布、应走哪个策略、库存是否可用或操作者是否有权限。
>
> 第 23 节我没有马上写一个通用 Rule 接口，而是先用“新用户抽奖”的复合场景拆出 Activity、Participation、Lottery、Benefit 和 Governance 的决策所有者，并把会员、风险等原始事实提供方单独标注，明确规则决策、加权选择、持久副作用三者不同，并区分 business_denied、dependency_unavailable、unauthorized、no_reward 和未来 result_unknown。通过需求矩阵、ADR、失败模型、版本模型和负向源码 diff，证明本节没有提前实现第 25～29 节。Redis 在第 24 节只缓存可重建 Strategy 投影；用户资格在第 25 节出现，责任链、规则树和决策引擎再由实际复杂度逐步推动；统一权限模型保持独立，在第 31～35 节按模型、会话、服务端强制、前端投影和越权验收演进。

## 来源与可信度

- **项目事实：** 只来自当前仓库代码、第 23 节课程、需求文档、ADR 和 QA。设计约束不等于运行能力。
- **官方事实：** Go、OMG、OWASP、Eric Evans DDD Reference 与 Martin Fowler 原文用于校准语言、标准、安全与选型边界。
- **面经题型启发：** 牛客链接是平台用户发布的个人复盘、题目整理或经验文章，只能说明有人记录过相似追问。它们不是公司官方题库，帖内答案也不是本项目技术规范。
- 外部链接复核日期为 **2026-08-30**；本文只概述题型，不复制长段内容。

---

## 1. 第 23 节没有写 Go 业务代码，为什么仍算真实推进？

- **直接回答：** 真实推进不等于每节都增加运行代码。当前业务只说“抽奖需要规则”，但没有给出第一条规则的权威输入、结果集合、失败语义、顺序、副作用和版本。如果此时写 Rule 接口，我只能猜。第 23 节把复合需求拆成可追踪的决定所有权、原始事实来源、失败分类、版本概念和后续章节依赖，并用 ADR 固化边界；它减少了第 25～30 节把用户、Activity、库存和权限塞进 Lottery 的返工风险。QA 还通过负向 diff 证明没有提前加入源码、Migration 或 route。
- **追问：** 什么时候纯架构章节会变成“只写文档不落地”？
  - **追问回答：** 当结论没有约束后续实现、没有可证伪条件、没有追踪矩阵或完成门禁时就是空文档。本节每条需求都有事实所有者、失败语义和计划章节；后续若代码出现 User 字段塞入 Strategy、权限判断塞入资格链或技术失败返回 no_reward，评审可以直接判定违反 ADR。
- **项目证据：** [第 23 节设计手记](../../design-thinking/lessons/lesson-23.md)、[第 23 节课程](../../course/part-03/lesson-23-lottery-strategy-rule-requirements.md)、[第 23 节 QA](../../qa/lessons/lesson-23.md)、[ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)。
- **选型边界：** 如果已经有稳定的具体规则、消费者和验收样例，继续只写边界而不实现就是拖延；第 25 节开始必须让第一条资格需求落到代码和测试。
- **来源：** 项目事实；Martin Fowler [Specification by Example](https://martinfowler.com/bliki/SpecificationByExample.html)说明示例有助于形成可判定规格，但不能是唯一需求技术。

## 2. 为什么现在不先定义一个只有 Evaluate 方法的 Rule 接口？

- **直接回答：** 方法数量少不等于语义清晰。当前不知道输入是用户、Activity 还是 Strategy；输出是 allow/deny、路由、候选过滤还是权重调整；error 是依赖失败、配置损坏还是授权拒绝；也不知道规则是否允许 I/O、副作用、短路和 trace。一个 Evaluate(context, map) (bool, error) 会把这些差异压成 bool 与 error，未来每加一类规则都需要在参数 map、特殊 error 或隐式约定里补洞。
- **追问：** Go 不是鼓励小接口吗？
  - **追问回答：** Go 确实常见一两个方法的小接口，但接口仍要表达消费者真正需要的行为。项目已有 StrategyReader 和 AwardSelector 这种 consumer-owned 窄端口，因为它们的输入、输出、错误和所有权已经明确。第 26 节出现至少两条同阶段前置规则后，才有证据形成规则链协议。
- **项目证据：** [现有 Repository 窄端口](../../../internal/lottery/application/repository.go)、[EphemeralSelectionService 组合](../../../internal/lottery/application/ephemeral_selection.go)、[规则需求 v1](../../product/lottery-rule-requirements-v1.md)。
- **选型边界：** 当两条以上具体规则共享稳定输入、结果、短路和 trace 契约时，应及时抽取接口；不能用“避免过度设计”拒绝已经重复且稳定的协议。
- **来源：** Go 官方 [Effective Go：Interfaces](https://go.dev/doc/effective_go?lang=en&version=1#interfaces)；项目事实。

## 3. 规则判断、WeightedSelector 和副作用有什么区别？

- **直接回答：** 规则判断决定“是否继续、走哪个分支、采用哪个选择计划”；WeightedSelector 在一个已经合法的 Strategy 内根据均匀随机位置选择 Award；副作用则改变额度、Draw、库存或权益等持久事实。规则拒绝不应调用随机源，Selector 不应读取用户或权限，Rule 也不应偷偷扣次数。把三者分开后，正确性、重试、决定所有权和权威状态才能独立验证。
- **追问：** 为什么不能让规则 action 直接扣库存，更像传统生产规则系统？
  - **追问回答：** action 当然可以在某些生产规则系统中产生状态变化，但本项目的库存扣减需要并发控制、幂等、结果查询和补偿。若隐藏在 Rule 内，调用方无法知道规则执行了多少、超时后能否重试，也无法在 Lottery、Inventory 和 Benefit 之间保持事实边界。
- **项目证据：** [WeightedSelector](../../../internal/lottery/domain/weighted_selector.go)、[加权选择 ADR](../../decisions/ADR-0017-lottery-weighted-selection.md)、[第 23 节设计手记三分法](../../design-thinking/lessons/lesson-23.md)。
- **选型边界：** 纯计算且可逆的派生值可以作为决策动作；任何持久写、外部调用或不可逆动作都应升级为显式用例/命令并设计恢复语义。
- **来源：** 项目事实；Martin Fowler [Rules Engine](https://martinfowler.com/bliki/RulesEngine.html)用于理解 production rule 的 condition/action 模型，本项目没有直接照搬其副作用模型。

## 4. “抽奖规则”为什么被拆到多个限界上下文，而不是都归 Lottery？

- **直接回答：** 软件边界按决策所有权和变化原因划分，不按自然语言里都叫“规则”划分。Activity 的发布和时间窗决策归 Marketing；用户人群、资格和次数决策归 Participation，会员与风险系统只提供必要原始事实；Strategy、Award 和选择分支归 Lottery；Benefit 拥有内部库存子能力与权益交付；操作者对资源的访问决策归 Governance。Lottery 可以消费最小快照或已验证请求，但不能复制一份可独立修改的用户、活动和权益事实。
- **追问：** 这样会不会把简单流程拆得太碎？
  - **追问回答：** 限界上下文不等于微服务。当前仍是模块化单体和共享 MySQL，边界首先用于统一语言、依赖方向和事实责任。只有团队、负载、合规或可用性证据支持时才物理拆分。
- **项目证据：** [限界上下文地图](../../product/bounded-context-map-v1.md)、[规则需求 v1](../../product/lottery-rule-requirements-v1.md)、[Strategy 聚合](../../../internal/lottery/domain/strategy.go)。
- **选型边界：** 如果真实业务证明两个模型总是同事务、同生命周期、同团队变化，边界可以合并；但需要依据，而不是因为减少 package 数量。
- **来源：** Eric Evans [DDD Reference](https://www.domainlanguage.com/ddd/reference/)；项目事实。

## 5. 为什么 User、member level 或 remaining quota 不放进 Strategy？

- **直接回答：** Strategy 是可复用的 Lottery 配置；用户等级和额度是高基数、频繁变化的 Participation 事实。放进去会让同一 Strategy 随每个用户变化，缓存混入个人数据，配置版本与用户事实版本无法区分，还会让隐私删除和用户生命周期污染抽奖配置。更合理的是用例从权威来源读取最小参与上下文，再把明确选择计划交给 Lottery。
- **追问：** 如果某个用户确实需要个性化权重怎么办？
  - **追问回答：** 仍不必把 User 聚合塞进 Strategy。可以由规则决策选择一个已版本化 Strategy、产生受限的 Lottery-owned 调整计划，或未来使用编译的个性化投影；但必须定义权威输入、版本、审计、公平和缓存边界。
- **项目证据：** [Strategy 字段与不变量](../../../internal/lottery/domain/strategy.go)、[第 17 节领域边界](../../course/part-03/lesson-17-lottery-domain-objects.md)、[ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)。
- **选型边界：** 如果策略本来就是一次性、只属于一个主体且生命周期完全一致，需要重新评估聚合；当前复用 Strategy 的产品语言不满足该条件。
- **来源：** 项目事实；Eric Evans [DDD Reference](https://www.domainlanguage.com/ddd/reference/)。

## 6. 为什么不建立一个 common/rules 包，让用户、抽奖、权限都复用？

- **直接回答：** 共享名字不代表共享语义。用户资格规则可能读取额度并产生业务拒绝；Lottery 规则可能路由 Strategy；权限规则判定 Principal 对 Resource/Action/Scope 的访问。把它们放进 common 往往得到 map、bool、字符串 code 和大量开关，反而让上下文互相耦合。应先在各自消费者旁建立窄协议，只有多处出现稳定且不泄漏领域差异的机制时再抽取。
- **追问：** 哪些东西未来可以公共化？
  - **追问回答：** 例如有界 trace 容器、稳定 request correlation、版本值对象或只含机制的图校验器可能公共化，但前提是至少两个真实消费者证明相同不变量；RuleCode、资格原因和权限 Scope 仍应保留领域语言。
- **项目证据：** [课程路线修订记录](../../course/route-revisions.md)、[第 23 节设计手记备选矩阵](../../design-thinking/lessons/lesson-23.md)。
- **选型边界：** 如果多个上下文已复制相同算法和故障模型，拒绝抽取同样会制造维护成本；公共化需要以重复证据而不是目录偏好决定。
- **来源：** Go 官方 [Code Review Comments：Package Names](https://go.dev/wiki/CodeReviewComments#package-names)建议避免 common、util 等无意义包名；项目事实。

## 7. 为什么现在不引入通用规则引擎？

- **直接回答：** 规则引擎不是一个普通库，而是替换部分应用代码的计算模型，需要处理规则冲突、优先级、chaining、动态发布、调试、版本、回放、资源预算和治理。当前运行规则数量为零，没有规则变更频率、业务自助诉求或代码发布瓶颈数据，无法证明这些成本值得。现在先建立领域特定边界，等规则规模和变化模式出现再评估。
- **追问：** 规则多到多少才应该引入？
  - **追问回答：** 没有通用数量阈值。应观察变更频率、跨团队维护、冲突数量、发布等待、回放需求、决策表复杂度和现有代码可理解性；十条高度动态跨部门规则可能值得，几百条稳定配置也可能不值得。
- **项目证据：** [规则需求 v1](../../product/lottery-rule-requirements-v1.md)、[ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)。
- **选型边界：** 当代码发布成为稳定瓶颈、业务需要受控自助、规则数量和组合复杂度有测量证据，并能承担治理与退出策略时，应重新比较成熟引擎、自研窄引擎与编译配置。
- **来源：** Martin Fowler [Rules Engine](https://martinfowler.com/bliki/RulesEngine.html)；牛客用户发布的[规则引擎设计与面试复盘](https://www.nowcoder.com/feed/main/detail/232bdb7f1bcf44cf916c9227d1dd4150)记录了规则引擎项目追问，仅作个人自述的题型启发，不是公司官方题库或技术规范。

## 8. OMG DMN 是什么？为什么本节不直接采用 DMN runtime？

- **直接回答：** DMN 是 OMG 用于精确描述业务决策和业务规则的标准化建模语言，包含决策需求、决策表和 FEEL 等语义。它适合多角色沟通、标准交换和复杂决策管理。但采用 runtime 还要承担规范版本、FEEL 行为、hit policy、扩展函数、模型发布、兼容与 conformance 测试。当前只有一个待澄清复合需求，使用普通决策表即可发现歧义，引入 runtime 证据不足。
- **追问：** 那为什么文档还引用 DMN？
  - **追问回答：** 标准可以作为建模词汇和方案上界，帮助我们知道成熟决策管理会处理哪些问题；引用不等于实现或兼容。只有实际模型通过明确 DMN 版本的语义与一致性测试，才能宣称 DMN 支持。
- **项目证据：** [第 23 节设计手记 DMN 方案](../../design-thinking/lessons/lesson-23.md)、[ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)。
- **选型边界：** 出现跨部门决策表维护、合作方模型交换、合规要求或大量多条件决策时，应进行 DMN PoC 和双跑验证。
- **来源：** OMG 官方 [Decision Model and Notation](https://www.omg.org/dmn/)及 [DMN 1.5 Specification](https://www.omg.org/spec/DMN/1.5/PDF)。

## 9. 为什么不把规则保存成 JSON DSL？

- **直接回答：** JSON 只解决树状数据编码，不自动解决规则语义。我们仍需定义类型、操作符、引用、缺失值、时区、版本、循环、深度、复杂度、错误、沙箱、迁移和解释。当前 Strategy 的 ID/Weight 支持完整 uint64，而 Go 把通用 JSON object 解码到 interface 时数字默认是 float64；松散 map 会重新引入精度和运行时类型风险。即使用 UseNumber 或强类型 decoder 解决数字问题，领域语言和治理成本仍存在。
- **追问：** 第 28 节规则树不是也可能用 JSON 吗？
  - **追问回答：** 存储格式要由查询、约束、版本和发布需求决定。可以使用关系表、受 schema 约束的 JSON 列或编译产物，但必须先有稳定节点类型和图不变量；不能因为前端容易生成 JSON 就让数据库接受任意表达式。
- **项目证据：** [完整 uint64 领域模型](../../../internal/lottery/domain/award.go)、[第 21 节 API string ID 契约](../../api/lessons/lesson-21.md)、[第 23 节设计手记 JSON DSL 比较](../../design-thinking/lessons/lesson-23.md)。
- **选型边界：** 当规则语言稳定、有版本化 schema、静态校验、迁移器、资源预算和沙箱后，JSON 可以是其中一种传输/存储格式；不能把格式等同于引擎。
- **来源：** Go 官方 [JSON and Go](https://go.dev/blog/json)；OWASP [Input Validation Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)。

## 10. business_denied、dependency_unavailable、unauthorized 和 no_reward 为什么必须分开？

- **直接回答：** 它们对应不同事实和恢复动作。business_denied 表示权威业务事实明确不允许参与；dependency_unavailable 表示无法取得必需事实，应稍后重试或告警；unauthorized/forbidden 是身份或权限问题；no_reward 是已经合法参与并执行选择后命中一个合法候选。混在一起会造成错误重试、误导用户、隐藏故障或越权。
- **追问：** 对用户都显示“无法参与”不是更安全吗？
  - **追问回答：** 外部文案可以按安全策略收敛，但内部类型、状态、指标和审计必须区分。否则客服无法解释、SRE 看不到依赖故障、客户端不知道是否可重试，安全团队也无法发现越权尝试。
- **项目证据：** [AwardOutcome](../../../internal/lottery/domain/award.go)、[临时 API 错误边界](../../api/lessons/lesson-21.md)、[第 23 节失败语义表](../../design-thinking/lessons/lesson-23.md)。
- **选型边界：** 在特定威胁模型下可以把多个内部原因映射成同一外部状态以防资源枚举，但内部分类和审计不能丢失。
- **来源：** Go 官方 [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)支持可检查错误链；OWASP [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)；项目事实。

## 11. “失败关闭”是否意味着依赖超时就把用户判成不合格？

- **直接回答：** 不是。失败关闭描述安全或业务动作不能在缺少必需事实时继续，但失败的真实语义仍是 dependency_unavailable，而不是 business_denied。系统可以暂停抽奖、返回暂不可用并告警，但不能写入“该用户不符合资格”，否则技术故障会污染业务事实与用户解释。
- **追问：** 哪些规则可以降级放行？
  - **追问回答：** 只有产品和风险评审明确标注为可选增强的规则，例如个性化路由失败后回到公开基线 Strategy，才允许受控降级；资格、额度、合规、权限和高风险拒绝通常不应默认放行。降级策略必须版本化、可观测并有退出条件。
- **项目证据：** [非功能需求基线](../../product/non-functional-requirements-v1.md)、[规则需求 v1](../../product/lottery-rule-requirements-v1.md)、[第 23 节失败模型](../../design-thinking/lessons/lesson-23.md)。
- **选型边界：** 低风险只读推荐可以选择 fail open；价值权益、权限、库存和合规决策通常要 fail closed。结论取决于业务损失模型，不能套统一口号。
- **来源：** OWASP [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)的 deny by default 用于授权边界；OWASP [Business Logic Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Business_Logic_Security_Cheat_Sheet.html)用于业务语义校准。

## 12. 为什么要区分 Strategy 版本、RuleSet 版本、缓存格式版本和权限策略版本？

- **直接回答：** 它们解决不同兼容问题。Strategy 版本说明抽奖配置；RuleSet 版本说明决策逻辑；输入快照版本说明人群、额度或风险事实；缓存格式版本只说明 Redis bytes 怎样解码；算法版本说明选择或编译方法；权限策略版本说明当时为何允许访问。用一个 updated_at 代替全部版本，既无法稳定 cache key，也无法解释历史 Draw。
- **追问：** 当前表有 updated_at，为什么不直接用？
  - **追问回答：** 当前 Strategy 和 Award 是父子表，Award 更新不会自动推进根行时间戳；updated_at 是行元数据，不是聚合发布身份。当前甚至没有 Update/发布用例，所以第 23 节只固定概念，第 28～30 节再由真实 schema 和发布需求实现。
- **项目证据：** [当前 Strategy Migration](../../../migrations/sql/000001_create_lottery_strategy.up.sql)、[Award Migration](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql)、[第 18 节设计手记](../../design-thinking/lessons/lesson-18.md)。
- **选型边界：** 若配置永久 immutable 且每次变更创建新全局 ID，显式 version 字段可能不是必需；但 cache schema、policy 和输入版本仍不能混为一谈。
- **来源：** 项目事实；Eric Evans [DDD Reference](https://www.domainlanguage.com/ddd/reference/)用于聚合与模型边界。

## 13. 规则可解释性应该记录什么，又不能记录什么？

- **直接回答：** 至少需要稳定 RuleCode、RuleSetVersion、阶段、结果、ReasonCode、低基数耗时分类和 request correlation。用户看到安全可操作文案，运营看到受权限控制的业务原因，安全审计可看到更细策略版本。不能把完整用户画像、风险特征、密码、token、任意表达式或全量输入直接写普通日志；trace 还要限制条数、深度、字节数和保留期。
- **追问：** 为什么不用自然语言直接作为 reason？
  - **追问回答：** 文案会本地化和修改，不能成为机器分支或长期审计 identity。稳定 code 支持指标、客户端映射和历史查询，文案只是受控投影。
- **项目证据：** [第 23 节安全与审计设计](../../design-thinking/lessons/lesson-23.md)、[第 12 节日志错误体系](../../course/part-02/lesson-12-config-logging-errors.md)。
- **选型边界：** 监管场景可能要求更完整证据链和不可篡改存储；高隐私场景则可能只保存 hash/reference。应由合规、用途和保留策略决定。
- **来源：** OWASP [Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)；项目事实。

## 14. 第 24 节 Redis 应缓存什么，为什么不缓存用户规则结果？

- **直接回答：** 第 24 节只缓存可从 MySQL 重建的 Strategy 读取投影，MySQL 和领域构造器仍是事实边界。当前没有 Strategy Update/业务版本，可先用 cache schema version + StrategyID、TTL 和回源，明确这不是发布版本。用户资格和决策结果高基数、时效短、包含个人数据且权威来源尚未定义，不能顺手塞入同一 cache。
- **追问：** Redis 不可用时怎么办？
  - **追问回答：** 对 Strategy cache 采用 cache-aside 回 MySQL，坏 value 删除并重新恢复；需要限制回源风暴和穿透。未来若资格查询成为瓶颈，应单独评审其 staleness、隐私、撤销和一致性，不能复用 Strategy TTL。
- **项目证据：** [第 24 节前置问题](../../design-thinking/lessons/lesson-23.md)、[Repository Reader](../../../internal/lottery/application/repository.go)、[第 21 节 API 直读事实](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)。
- **选型边界：** 当 Strategy 支持发布/更新时，必须增加业务版本 key 或明确失效协议；继续只按 ID + TTL 可能返回已撤销配置。
- **来源：** 项目事实；本题没有借用社区文章证明尚未实现的 Redis 行为。

## 15. 第 26 节为什么可能选择责任链？接口应由什么需求推动？

- **直接回答：** 第 25 节先落第一条真实的 Participation 资格规则；只有第 26 节再出现第二条同属 Participation、具有明确顺序和短路语义的前置判断时，才有证据把两条具体规则抽成责任链。接口应由这些真实节点共同需要的只读输入、DecisionResult、context、trace 和 error 形成，而不是第 23 节先造一个万能 Rule。
- **追问：** 每个链节点可以直接查数据库吗？
  - **追问回答：** 应优先由 application/adapter 构造权威输入快照，纯节点只判断。若确实需要按需 I/O，接口必须显式携带 context、timeout 和依赖端口，并统计调用预算；不能在 domain 节点里悄悄创建 client。
- **项目证据：** [课程状态路线](../../course/status.csv)、[第 23 节未来演进问题](../../design-thinking/lessons/lesson-23.md)、[现有 consumer-owned AwardSelector](../../../internal/lottery/application/ephemeral_selection.go)。
- **选型边界：** 当规则需要多分支路由、合流、共享子决策或非线性解释时，责任链会变成大量条件跳转，第 27 节应以真实失败推动树/图，而不是无限扩链。
- **来源：** 项目事实；Martin Fowler [Rules Engine](https://martinfowler.com/bliki/RulesEngine.html)提醒 chaining 会增加推理和调试难度；牛客用户发布的[责任链设计与面试复盘](https://www.nowcoder.com/feed/main/detail/d9c7149407d9456d80d863274b1409b2)只用于补充真实追问形态，不作为本项目选型依据。

## 16. 责任链什么时候不够，为什么下一步可能是规则树？

- **直接回答：** 线性链适合“依次检查，任一拒绝即结束”。若规则结果不是单纯 pass/fail，而是会员等级走不同 Strategy、某分支再判断人群、多个分支共享后续节点，链会充满 next-if 和隐藏跳转。规则树能显式建模节点与有向边，但也引入根、无环、深度、可达性、默认边、确定性分支和版本发布问题，所以要等真实反例出现。
- **追问：** 为什么不直接上 DAG，树也可能复用不了节点？
  - **追问回答：** DAG 支持共享子图，但循环检测、拓扑语义、节点复用副作用和解释更复杂。应先看第 27 节的真实分支需求；如果树足够就不承担 DAG，一旦共享子决策带来可量化重复，再升级。
- **项目证据：** [第 23 节复杂度预算](../../design-thinking/lessons/lesson-23.md)、[课程第 27～29 节路线](../../course/status.csv)。
- **选型边界：** 规则只有固定少量分支时，普通领域代码可能比数据驱动树更清楚；不要把图结构本身当架构成熟度。
- **来源：** OMG [DMN](https://www.omg.org/dmn/)用于理解决策依赖建模；项目事实。

## 17. 业务资格与 RBAC 授权有什么区别？为什么不共用一个规则引擎？

- **直接回答：** 授权回答“Principal 能否对 Resource 执行 Action，并处于哪个 Scope”；业务资格回答“已经认证且有权发起请求的用户，在 Activity 和 Participation 事实下是否符合条件”。授权成功不等于有抽奖次数，符合新人资格也不等于能编辑 Strategy。两者可以共享 request ID、资源 identity 和审计设施，但不能共享角色布尔值或同一个 deny reason。
- **追问：** ABAC 也使用用户、资源和环境属性，和业务规则不是很像吗？
  - **追问回答：** 计算形式可能相似，但目标、默认、泄露风险、审计和所有者不同。ABAC 的输出决定访问控制，应默认拒绝并逐请求强制；活动资格是业务事实，可能有产品解释、额度和补偿。可复用机制必须在语义分离后评估，不能因为都读属性就合并模型。
- **项目证据：** [课程路线修订记录](../../course/route-revisions.md)、[第 23 节权限独立边界](../../design-thinking/lessons/lesson-23.md)、[限界上下文地图](../../product/bounded-context-map-v1.md)。
- **选型边界：** 某些风险策略可能同时影响访问和业务，但仍应由明确上游风险信号分别投影到两种决策，避免一方状态变更静默改变另一方语义。
- **来源：** OWASP [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)；项目事实。

## 18. 前端隐藏导航、路由或按钮为什么不等于规则或权限已经实现？

- **直接回答：** 浏览器代码和状态都能被绕过，用户可以直接访问 URL 或调用 API。业务资格和授权必须由服务端使用权威事实决定；前端只消费会话能力与业务 decision 的投影，减少无效入口并解释结果。当前 GrowthOS 的 activeRole、Mock 用户和工作区入口只是信息架构演示，不能作为 RBAC 证据。
- **追问：** 那前端为什么还要做权限裁剪？
  - **追问回答：** 它改善体验、减少误操作、避免展示用户永远不能使用的动作，并能为禁用状态提供原因；但安全验收必须覆盖直接 URL、直接 API、修改前端状态和隐藏按钮绕过。
- **项目证据：** [前端架构当前边界](../../frontend/frontend-architecture.md)、[课程路线第 31～35 节](../../course/route-revisions.md)、[WorkspaceShell](../../../web/src/components/layout/WorkspaceShell.tsx)。
- **选型边界：** 公开只读内容可不要求登录，但敏感数据和动作仍需资源级授权；不能因为页面公开就默认所有字段公开。
- **来源：** OWASP [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)要求逐请求验证权限；OWASP [Input Validation Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)指出客户端校验可绕过。

## 19. 规则依赖超时后能否自动重试？

- **直接回答：** 取决于失败阶段。若在选择和任何副作用之前读取资格失败，重试可能是一个新的安全尝试，但仍要退避、限流并重新读取权威事实；若 Award 已选、额度已扣或 Draw 可能已写，自动重试可能产生第二次结果。当前 ephemeral API 没有持久 request identity，超时后绝不能声称重试得到同一次选择。
- **追问：** Rule 都设计成纯函数就能解决重试吗？
  - **追问回答：** 纯函数可以让规则判断可重放，但不能解决输入读取、随机选择和持久副作用。完整恢复需要版本化输入、随机/选择事实、幂等 identity、状态机和结果查询。
- **项目证据：** [Ephemeral API 不重试边界](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)、[React Hook 不自动重试](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts)、[第 23 节部分失败](../../design-thinking/lessons/lesson-23.md)。
- **选型边界：** 对明确幂等的只读输入查询可以在 adapter 内做有界重试，但要尊重 context 和总体预算；不能把基础设施重试外推为业务命令幂等。
- **来源：** 项目事实；Go 官方 [Errors are values](https://go.dev/blog/errors-are-values)用于类型化失败思路。

## 20. 需求/架构章节怎样设计可执行验收？

- **直接回答：** 一方面验证文档结构和链接，另一方面验证没有越过时间切片：make doc-check、make verify；再把本分支与第 22 节 tip 比较，要求 cmd、internal、web、migrations、deploy 和 configs 没有变化。语义验收还检查需求矩阵每行是否有唯一事实所有者、失败分类和未来章节，并检查文档没有声称 Rule、Redis、资格、RBAC 或 Draw 已实现。
- **追问：** 负向 diff 能证明架构设计正确吗？
  - **追问回答：** 不能。它只证明没有提前写代码。设计正确性需要同行评审、场景反例、ADR 一致性和后续实现反馈；未来事实变化时应更新或替代决策，而不是因为文档已合并就永远正确。
- **项目证据：** [第 23 节 QA](../../qa/lessons/lesson-23.md)、[设计手记证据设计](../../design-thinking/lessons/lesson-23.md)、[文档完成标准](../../design-thinking/README.md)。
- **选型边界：** 实现章节必须增加对应单元、集成和 E2E；不能沿用“文档通过”代替运行行为。
- **来源：** 项目事实；Martin Fowler [Specification by Example](https://martinfowler.com/bliki/SpecificationByExample.html)。

## 21. Strategy 为什么不等于 Activity，这和规则版本有什么关系？

- **直接回答：** Activity 面向用户，拥有目标、发布态、时间窗和生命周期；Strategy 是 Lottery 内可复用的选择配置。一个 Activity 可引用一个已发布 Strategy 版本，同一个 Strategy 也可被多个 Activity 复用。若二者合并，活动暂停可能被误解为删除策略，策略调整也无法判断影响哪些活动，历史 Draw 更无法解释当时引用的配置。
- **追问：** Activity 发布时是否应复制完整 Strategy？
  - **追问回答：** 可能保存不可变版本引用，也可能保存必要快照；选择取决于审计、存储和变更需求。关键是不引用可原地修改的“当前对象”并假装历史稳定。第 30 节需要用真实发布流程决定。
- **项目证据：** [限界上下文统一语言](../../product/bounded-context-map-v1.md)、[第 6 节上下文划分](../../course/part-01/lesson-06-first-bounded-contexts.md)、[第 23 节版本推导](../../design-thinking/lessons/lesson-23.md)。
- **选型边界：** 若产品证明 Strategy 永远只属于一个 Activity 且同生命周期，可合并聚合；当前路线明确要求可复用 Strategy，所以不满足。
- **来源：** Eric Evans [DDD Reference](https://www.domainlanguage.com/ddd/reference/)；项目事实。

## 22. 如果未来允许运营人员动态配置规则，最少需要哪些治理能力？

- **直接回答：** 至少需要草稿与发布版本、schema 和语义校验、样本模拟、变更 diff、最小权限、审批分离、原子发布、回滚、紧急停用、配置与决策审计、复杂度预算、缓存失效和在途请求版本语义。动态配置不是把 JSON 写进数据库；它把代码评审的一部分风险转移到了产品化治理。
- **追问：** 为什么权限管理页面本身也要受权限控制？
  - **追问回答：** 能分配权限或发布规则的动作比普通业务操作风险更高，不能因为页面叫“管理中心”就默认看到全量。第 31～35 节会将读取、编辑、审批、发布和审计拆成资源动作权限，并由服务端强制。
- **项目证据：** [第 23 节安全推导](../../design-thinking/lessons/lesson-23.md)、[路线修订的公共权限模型](../../course/route-revisions.md)、[非功能需求安全目标](../../product/non-functional-requirements-v1.md)。
- **选型边界：** 只有研发在代码中维护少量规则时，可以沿用代码评审和部署治理；一旦业务自助或运行时发布出现，上述控制必须产品化。
- **来源：** OWASP [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)、[Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)；Martin Fowler [Business Readable DSL](https://martinfowler.com/bliki/BusinessReadableDSL.html)。

## 23. 面试时怎样回答“你们为什么不用 Drools/DMN/现成规则引擎”，避免变成技术排斥？

- **直接回答：** 我不会说现成引擎不好，而会给出时间切片证据：当时运行规则为零，第一条资格规则的输入和失败语义尚未明确，团队也没有标准交换、业务自助或高频动态发布需求。引入成熟引擎会同时引入新的计算模型、部署和治理成本。因此先用领域边界和窄协议学习问题；当规则数量、跨团队维护、发布瓶颈和决策表需求达到触发条件时，再用 PoC 比较成熟产品、自研领域引擎与代码方案。
- **追问：** 你会怎样做 PoC？
  - **追问回答：** 选真实规则集和历史样本双跑，比较语义一致性、P99、资源上限、trace、版本发布、回滚、故障注入、Go 集成、供应链和运维成本；还要验证退出与模型导出，不能只跑 hello world。
- **项目证据：** [ADR-0019 候选方案](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)、[设计手记触发条件](../../design-thinking/lessons/lesson-23.md)。
- **选型边界：** 如果组织已有经过治理的统一决策平台，复用成本可能远低于项目自建；需要把组织现状纳入决策，而不是孤立看仓库。
- **来源：** OMG [DMN](https://www.omg.org/dmn/)；Martin Fowler [Rules Engine](https://martinfowler.com/bliki/RulesEngine.html)。

## 24. 这一节在简历和面试中最容易夸大什么？

- **直接回答：** 最容易把“设计了规则边界”写成“实现高性能规则引擎”，把决策表写成 DMN runtime，把未来 RuleSetVersion 写成已建表，把权限路线写成已完成 RBAC，把 make verify 写成规则正确性或生产 SLO。准确表述应是“完成 Lottery 规则需求分解与 ADR，建立上下文所有权、决策/选择/副作用边界、失败与版本模型，并以负向 diff 约束后续渐进实现”。
- **追问：** 没有代码的章节值得写进简历吗？
  - **追问回答：** 最终简历不必逐节罗列，但在项目面试中它能解释为何后续责任链、规则树、缓存和权限不会成为补丁。简历主句应以最终已实现能力为准，本节作为架构决策证据，不单独虚构性能收益。
- **项目证据：** [第 23 节 QA](../../qa/lessons/lesson-23.md)、[课程状态](../../course/status.csv)、[第 23 节设计手记最终结论](../../design-thinking/lessons/lesson-23.md)。
- **选型边界：** 后续第 29 节真正实现决策引擎并完成测试后，才能升级措辞；仍不能在无生产数据时写虚假 QPS 或命中率。
- **来源：** 项目事实；牛客用户发布的[27 届秋招找 Agent 开发，简历到底应该怎么写](https://www.nowcoder.com/discuss/906102877850980352)强调项目深挖和规模追问，只作用户经验题型启发，不是招聘方官方标准。

---

## 不能夸大的事实

当前可以说：

- 已完成 Lottery 规则需求的决定所有权与原始事实来源分解；
- 已明确规则决策、Weighted Selection 和持久副作用的不同职责；
- 已形成规则拒绝、依赖失败、授权拒绝、no_reward 和未来结果未知的语义边界；
- 已比较 handler if、巨型 Strategy、统一 Rule、common 包、通用规则引擎、DMN runtime 和 JSON DSL；
- 已定义版本、可解释性、安全、资源预算和演进触发器；
- 已把公共访问控制保持为第 31～35 节独立路线；
- 最终验收要求文档门禁、完整回归和负向源码 diff 同时通过。

当前不能说：

- 已实现任何 Rule Go 接口或规则引擎；
- 已实现责任链、规则树、DMN、DSL 或动态发布；
- 已接入 Redis 业务缓存；
- 已实现用户资格、Activity、库存、权益或正式 Draw；
- 已实现登录、RBAC、对象级授权、权限管理页面或越权 E2E；
- 已完成规则性能测试、生产 SLO 或公平认证；
- 牛客帖子是公司官方面试题；
- 外部方案比较等于已经部署这些技术。

## 复习清单

- 能否在白板上画出 Activity → Participation → Lottery → Benefit（含内部库存子能力），并说明 Governance 是正交控制面？
- 能否用一句话区分 business_denied、dependency_unavailable、unauthorized 和 no_reward？
- 能否解释为什么 Rule、Selector 和 Side Effect 不应互相吞并？
- 能否说出为什么当前没有证据定义 Evaluate(map) (bool, error)？
- 能否比较 common 包、领域窄接口、通用规则引擎、DMN 和 JSON DSL？
- 能否解释完整 uint64 为什么让松散 JSON map 更危险？
- 能否列出 StrategyVersion、RuleSetVersion、CacheSchemaVersion、InputSnapshotVersion 和 PolicyVersion？
- 能否说明 Redis 只缓存可重建 Strategy 投影，不缓存尚未定义的用户决策？
- 能否说明责任链适合线性 gate，规则树何时由真实分支推动？
- 能否说明授权与资格的目标、默认、审计和事实所有者不同？
- 能否给出动态配置的校验、模拟、审批、发布、回滚和资源预算？
- 能否准确说出本节没有新增 Go 业务代码，并把它解释为可验收范围控制？

## 来源清单（访问日期：2026-08-30）

### A. 项目事实

- [第 23 节课程](../../course/part-03/lesson-23-lottery-strategy-rule-requirements.md)
- [Lottery 规则需求 v1](../../product/lottery-rule-requirements-v1.md)
- [ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [第 23 节 API 记录](../../api/lessons/lesson-23.md)
- [第 23 节 QA](../../qa/lessons/lesson-23.md)
- [第 23 节设计手记](../../design-thinking/lessons/lesson-23.md)
- [课程路线修订记录](../../course/route-revisions.md)
- [Strategy](../../../internal/lottery/domain/strategy.go)
- [WeightedSelector](../../../internal/lottery/domain/weighted_selector.go)
- [EphemeralSelectionService](../../../internal/lottery/application/ephemeral_selection.go)

### B. 官方与一手技术资料

- [Go：Effective Go](https://go.dev/doc/effective_go?lang=en&version=1)
- [Go：Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Go：Errors are values](https://go.dev/blog/errors-are-values)
- [Go：Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)
- [Go：JSON and Go](https://go.dev/blog/json)
- [OMG：Decision Model and Notation](https://www.omg.org/dmn/)
- [OMG：DMN 1.5 Specification](https://www.omg.org/spec/DMN/1.5/PDF)
- [OWASP：Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)
- [OWASP：Input Validation Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)
- [OWASP：Business Logic Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Business_Logic_Security_Cheat_Sheet.html)
- [OWASP：Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)
- [Martin Fowler：Rules Engine](https://martinfowler.com/bliki/RulesEngine.html)
- [Martin Fowler：Business Readable DSL](https://martinfowler.com/bliki/BusinessReadableDSL.html)
- [Martin Fowler：Specification by Example](https://martinfowler.com/bliki/SpecificationByExample.html)
- [Eric Evans：DDD Reference](https://www.domainlanguage.com/ddd/reference/)

### C. 牛客用户内容：只作题型启发，不是官方题库

- [规则引擎设计与面试复盘](https://www.nowcoder.com/feed/main/detail/232bdb7f1bcf44cf916c9227d1dd4150)：平台用户的个人复盘，用于说明规则引擎项目会被追问边界、结构和选型；其身份、公司与原题未由本项目独立核验，帖内方案不作为技术依据。
- [责任链设计与面试复盘](https://www.nowcoder.com/feed/main/detail/d9c7149407d9456d80d863274b1409b2)：平台用户的个人复盘，用于补充责任链职责拆分的追问形态；不是公司官方题库，也不证明本项目已经实现责任链。
- [27 届秋招找 Agent 开发，简历到底应该怎么写](https://www.nowcoder.com/discuss/906102877850980352)：平台用户的求职经验内容，用于提醒项目会被追问规模、故障和设计依据；不是招聘方官方评价标准。
