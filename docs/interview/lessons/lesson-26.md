# 第 26 节面试题：责任链前置规则、确定短路与可解释失败

本文面向“Go 业务规则建模、责任链、风险准入、失败关闭、context 取消、错误分类与可观察性”类面试。第 26 节在第 25 节单条新用户规则之上增加了第二条真实规则——风险 screening 准入，并实现 Participation 专用的固定线性前置资格链。它能按“新用户资格 → 风险准入”的版本化顺序执行，在确定拒绝、技术失败或 caller cancellation 时短路，并在业务结果确定时返回最小有序执行证据。

这里必须诚实限定能力：当前没有生产注册/风险 adapter、可信会话、Activity、正式 Participation/Draw、HTTP 接口、前端入口、持久化审计或 OpenTelemetry 接入。<code>ParticipantRef</code> 也不是 Principal。本文的代码与测试链接说明当前实现和覆盖意图；命令是否实际通过、race 是否通过以及 runtime negative diff，以第 26 节 QA 验收记录为准。

## 60 秒项目自述

> 第 25 节只有一条新用户资格规则，所以我拒绝提前创建通用 <code>Rule</code> 或规则引擎；一个例子无法证明公共协议。第 26 节出现第二条已登记的真实 Participation 前置条件：风险系统提供最小的 <code>passed/blocked</code> screening fact，Participation 决定它是否允许进入当前场景。两条具体规则第一次共同证明了顺序、继续、拒绝、错误、取消和 trace 语义，因此我只抽取了包内最小线性 step，而没有开放插件平台。
>
> <code>EligibilityPrerequisiteChain</code> 固定先执行 <code>participation.new_user.registered_on_or_after</code>，再执行 <code>participation.risk.screening_admission</code>。组合器在任何事实读取前只捕获一次受控 UTC evaluated-at；两个 reader 保持懒加载。前一条规则确定拒绝、报错或被取消后，风险 reader 不会调用。只有节点得到确定 <code>eligible</code> 才继续；任一确定 <code>ineligible</code> 返回带终止 reason 的最终业务结果；事实缺失、过期、future、损坏、依赖失败和 caller cancellation 都返回零 <code>PrerequisiteEvaluation</code> 加 error，绝不伪装成业务拒绝。
>
> 确定结果携带 ruleset revision、统一 evaluated-at 和仅含实际执行节点的只读 trace。每步只保留 rule、outcome、reason、policy revision 与 fact provenance，不携带主体、风险分数、特征、阈值或原始 provider error。这个实现是有意受限的 ordered gate chain，不是动态规则引擎；第 27 节只有在真实会员分层带来多出口、缺省分支和 path trace 后，才有证据证明线性链不够。

## 来源与可信度

- **项目事实：** 只以当前仓库的 [ADR-0022](../../decisions/ADR-0022-participation-prerequisite-chain.md)、[Participation 前置资格链基线 v1](../../product/participation-prerequisite-chain-v1.md)、Participation domain/application 代码及测试为依据。设计文档是已批准契约，代码是当前实现，测试是可执行证据；三者含义不同，不能互相冒充。
- **官方技术事实：** Go 语义只采用 Go 官方的 [Interfaces 指南](https://go.dev/wiki/CodeReviewComments#interfaces)、[context 包](https://pkg.go.dev/context)、[Contexts and structs](https://go.dev/blog/context-and-structs)、[errors 包](https://pkg.go.dev/errors)、[Go 1.13 errors](https://go.dev/blog/go1.13-errors)、[接口与 nil FAQ](https://go.dev/doc/faq#nil_error)和 [Race Detector](https://go.dev/doc/articles/race_detector)。决策结果和组合算法用 OASIS [XACML 3.0 正式标准](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-en.html)作概念校准，不声称本项目实现 XACML。责任链形态参考 Apache 官方的 [Commons Chain](https://commons.apache.org/dormant/commons-chain/)与 AWS 官方的 [Chain of Responsibility guidance](https://docs.aws.amazon.com/prescriptive-guidance/latest/best-practices-cdk-typescript-iac/reusable-patterns-best-practices.html)；Commons Chain 已进入 dormant，只能作为历史模式资料，不能据此推荐依赖。
- **规则引擎与观测资料：** OPA 官方的 [Policy Language](https://www.openpolicyagent.org/docs/policy-language)、[Integration](https://www.openpolicyagent.org/docs/integration)和 [Operations](https://www.openpolicyagent.org/docs/operations)用于说明真正策略系统还涉及声明式语言、求值集成、bundle/discovery、运行和故障边界。观测结论使用 OpenTelemetry 官方的 [Trace API](https://opentelemetry.io/docs/specs/otel/trace/api/)、[Recording errors](https://opentelemetry.io/docs/specs/semconv/general/recording-errors/)、[error.type](https://opentelemetry.io/docs/specs/semconv/registry/attributes/error/)与 [Metrics SDK cardinality](https://opentelemetry.io/docs/specs/otel/metrics/sdk/)，以及 W3C [Trace Context](https://www.w3.org/TR/trace-context/)；这些是未来接入约束，不表示本节已经接入观测系统。
- **面经题型启发：** 牛客用户发布的[字节生活服务 Go 面经](https://www.nowcoder.com/feed/main/detail/d9c7149407d9456d80d863274b1409b2)、[美团暑期后端面经](https://www.nowcoder.com/discuss/732945501980565504)、[美团实习面经](https://www.nowcoder.com/feed/main/detail/7a197c54ad294c5496eb6146a6f740d3)、[快手营销面经](https://www.nowcoder.com/feed/main/detail/394f5367a0af499faac7d1e6bcdceecc)、[百度营销面经](https://www.nowcoder.com/feed/main/detail/96aa803ee7c94f14bd50325fd6fc1e9d)、[百度后端面经](https://www.nowcoder.com/feed/main/detail/fa3706d7d465417f83d550b7f471941b)、[滴滴安全面经](https://www.nowcoder.com/feed/main/detail/249f392caf544641a86a8c67b39daec0)与[腾讯云智面经](https://www.nowcoder.com/feed/main/detail/de7161a16ee44838a8377b0f89fecdb8)只说明候选人曾自述遇到“为什么用链、节点怎样排序、context 放什么、失败后是否回滚、为什么不把副作用放进链、链与策略如何组合”等题型。它们无法独立验证，不是公司官方题库，更不是技术事实；本文不采纳其中答案，只把题型转化为可由项目证据回答的追问。
- 外部链接复核日期为 **2026-08-30**。

---

## 1. 为什么第 25 节拒绝抽象，第 26 节却开始抽责任链？

- **直接回答：** 第 25 节只有一个 concrete evaluator，无法证明公共接口应该携带什么、怎样排序、何时停止、怎样区分拒绝与错误。第 26 节新增真实风险准入后，两条规则共同证明了一个最小交集：接收同一 caller context，在同一 evaluated-at 下形成可验证 step decision，只有确定 eligible 才继续。因此只抽了包内 <code>prerequisiteStep</code>，没有先设计对外通用 <code>Rule</code>。
- **追问：** “三个例子再抽象”是不是硬规则？
  - **追问回答：** 不是。抽象时机取决于是否出现稳定共同变化轴；本项目第二条规则已足以证明顺序、短路和失败语义，但还不足以证明运行时插件、priority、树或 DSL。
- **权衡：** 现在抽最小 runner 减少两套控制流漂移；保持私有则避免错误接口成为长期兼容负担。
- **代码 / 测试证据：** [固定链与私有 step](../../../internal/participation/application/prerequisite_chain.go)、[架构停止线测试](../../../internal/participation/application/architecture_test.go)、[ADR 的抽象时机](../../decisions/ADR-0022-participation-prerequisite-chain.md)。
- **官方来源：** Go 官方 [Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)建议接口由消费者定义，并警告不要在使用前先定义；这里正是从两个真实消费者形状反推包内协议。
- **题型来源（非技术依据）：** 牛客用户的[字节生活服务 Go 面经](https://www.nowcoder.com/feed/main/detail/d9c7149407d9456d80d863274b1409b2)自述出现过“为什么抽链、抽什么接口”的追问，真实性未独立核验。

## 2. 为什么第二条规则选风险准入，而不是剩余次数、Activity 时间窗或角色权限？

- **直接回答：** 风险准入已经是需求台账中的真实 Participation 前置条件，而且与新用户规则共享“只读事实 → 场景决定 → 继续/拒绝”的语义。剩余次数是可并发消费资源，简单 bool 会制造 TOCTOU；Activity 生命周期尚未建立且属于 Marketing；角色权限需要可信 Principal、资源、动作与范围，不能拿 <code>ParticipantRef</code> 冒充。
- **追问：** 为什么不为了演示链先写一个假的 always-pass 节点？
  - **追问回答：** 假节点只能证明 for-loop 能跑，不能证明事实所有权、失败分类、隐私、成本和顺序。抽象应由业务差异施压，而不是由设计模式演示施压。
- **权衡：** 风险源更敏感、更昂贵，使短路价值可测；代价是必须认真建模 freshness、错误脱敏和数据最小化。
- **代码 / 测试证据：** [风险快照](../../../internal/participation/domain/risk_screening_fact.go)、[风险准入 evaluator](../../../internal/participation/domain/risk_admission.go)、[风险规则边界测试](../../../internal/participation/domain/risk_admission_test.go)、[ADR 方案比较](../../decisions/ADR-0022-participation-prerequisite-chain.md)。
- **官方来源：** Go 官方接口指南支持先由实际使用证明接口；MITRE [CWE-367](https://cwe.mitre.org/data/definitions/367.html)可用于理解 check/use 状态变化风险，但本项目的次数一致性是业务并发问题，不等同于该文件系统漏洞。

## 3. 风险系统已经给出 passed 或 blocked，为什么 Participation 还要再做一层决定？

- **直接回答：** <code>RiskScreeningDisposition</code> 是风险 authority 拥有的源事实；<code>RiskAdmissionDecision</code> 是 Participation 对当前参与场景的准入解释。前者不能宣布 GrowthOS 最终资格，后者也不能复制模型特征、阈值或修改风险事实。分离后，同一风险 verdict 可以被不同场景以不同 policy revision 消费，而不让风险系统耦合所有业务。
- **追问：** 当前 policy 只带 revision，没有阈值，是否多余？
  - **追问回答：** 不多余。revision 先固定“passed 才继续、blocked 拒绝”这一不可变解释版本；未来若政策语义改变，可新增版本或新 rule code，而不是悄悄改变旧决定的含义。
- **权衡：** 多一层模型增加类型和版本管理，但换来事实所有权、场景所有权与可回放边界。
- **代码 / 测试证据：** [RiskAdmissionPolicy](../../../internal/participation/domain/risk_admission_policy.go)、[RiskAdmissionDecision](../../../internal/participation/domain/risk_admission.go)、[policy 与 ruleset revision 测试](../../../internal/participation/domain/risk_admission_policy_test.go)。
- **官方来源：** OASIS [XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-en.html)把属性、策略求值与决定作为不同概念；这里只作所有权类比，本项目没有实现 XACML。

## 4. 为什么风险事实只允许 passed 或 blocked，不把 unknown、review、timeout 都塞进枚举？

- **直接回答：** 当前只有 <code>passed</code> 和 <code>blocked</code> 是权威源能够确认的业务事实。unknown、缺失、过期、future、损坏和依赖 timeout 表示“本次无法形成可信业务决定”，应走零 decision 加 typed error；把它们塞进业务枚举会把来源质量、技术故障和人工审核生命周期混成一个字段。
- **追问：** 如果以后真有人工审核状态呢？
  - **追问回答：** 应先定义它是不是正式 Participation 状态、由谁推进、多久过期、怎样恢复，再新增明确领域状态或异步流程；不能复用当前 <code>blocked</code> 或技术 error。
- **权衡：** 两值事实让模型窄而安全，但不可表达未来人工流程；扩展必须由真实生命周期驱动。
- **代码 / 测试证据：** [disposition 的封闭枚举与 Validate](../../../internal/participation/domain/risk_screening_fact.go)、[unknown/zero 反例测试](../../../internal/participation/domain/risk_screening_fact_test.go)、[结果与终止语义](../../product/participation-prerequisite-chain-v1.md)。
- **官方来源：** OASIS XACML 正式标准区分 Permit、Deny、NotApplicable 与 Indeterminate，说明“确定拒绝”和“无法决定”不是同一语义；本项目选择 Go error 表达技术上无法决定，而不是复制其四态模型。

## 5. 这是不是经典责任链？为什么又叫 ordered gate chain？

- **直接回答：** 它是责任链的受限变体：多个处理步骤按顺序执行，并可停止后续处理。但经典责任链常表达“某个 handler 能处理就结束，否则传给下一个”；本项目两条都是必经 gate，<code>eligible</code> 表示继续，<code>ineligible</code> 表示终止，error/cancel 表示没有最终业务决定。因此用 ordered eligibility gate chain 更准确。
- **追问：** 名称不同会影响实现吗？
  - **追问回答：** 会。若误套“第一个能处理者胜出”，第一条 eligible 就可能提前成为最终成功；而本项目必须等所有必经 gate 都通过。
- **权衡：** 借用模式名有助沟通，但业务组合语义必须由类型和测试固定，不能由模式教科书替代。
- **代码 / 测试证据：** [for-loop 与 outcome 分支](../../../internal/participation/application/prerequisite_chain.go)、[基线中的组合语义](../../product/participation-prerequisite-chain-v1.md)。
- **官方来源：** Apache 官方 [Commons Chain](https://commons.apache.org/dormant/commons-chain/)记录了 command chain 的历史实现；AWS 官方 [Chain of Responsibility guidance](https://docs.aws.amazon.com/prescriptive-guidance/latest/best-practices-cdk-typescript-iac/reusable-patterns-best-practices.html)描述按序传递请求的形态。两者都不能替代本项目的 gate 契约。

## 6. 为什么不返回 bool？eligible、ineligible、unknown、error 和 cancel 怎样区分？

- **直接回答：** bool 只能表达两值，无法同时说明“本节点允许继续”“整条链最终允许”“有效事实明确拒绝”“依赖故障无法决定”和“caller 已放弃”。当前节点使用具体 decision 的 outcome/reason；链只有全部步骤通过才形成最终 eligible，确定拒绝返回 ineligible，技术失败和取消返回零 aggregate 加 error。
- **追问：** fail-closed 时用户都不能继续，为什么不统一 false？
  - **追问回答：** 控制动作相同不代表事实相同。确定拒绝通常不应重试；依赖 unavailable 可能重试并告警；cancel 是 caller 生命周期。混成 false 会污染拒绝率、客服解释、HTTP 映射和审计。
- **权衡：** <code>Decision + error</code> 比 bool 复杂，但保存了恢复、观测和解释所需的最小语义。
- **代码 / 测试证据：** [PrerequisiteEvaluation 返回契约](../../../internal/participation/application/prerequisite_chain.go)、[风险 evaluator 的零 decision 反例](../../../internal/participation/domain/risk_admission_test.go)、[风险 application 失败测试](../../../internal/participation/application/risk_admission_test.go)。
- **官方来源：** OASIS XACML 的决定模型用于校准 determinate 与 indeterminate 的区别；Go 官方 [errors](https://pkg.go.dev/errors)提供独立的 error 链与分类机制。

## 7. 为什么只用私有 prerequisiteStep，不导出 Rule 接口和动态 slice？

- **直接回答：** 当前外部组合只有固定的 registration reader、risk reader、Clock 和两个 freshness 上限；运行时没有第三方节点、priority、插件注册或非研发配置需求。私有 step 只统一 <code>code + evaluate</code> 的局部形状，构造器和 Evaluate 仍显式暴露具体业务依赖与 policy，避免万能 context bag。
- **追问：** 本地 plan 本身也是 slice，为什么不直接允许调用方传 slice？
  - **追问回答：** 数据结构相同不等于扩展契约相同。局部 slice 只是确定 for-loop 的实现细节；允许外部传入就必须定义重复 code、未知节点、排序、信任、版本、兼容性和恶意实现等全新问题。
- **权衡：** 固定装配牺牲运行时灵活性，换来可审查的组合、稳定顺序和更小攻击面。
- **代码 / 测试证据：** [局部 plan 与私有类型](../../../internal/participation/application/prerequisite_chain.go)、[禁止 Rule/RuleEngine/EvaluationContext 的 AST 测试](../../../internal/participation/application/architecture_test.go)。
- **官方来源：** Go 官方 [Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)强调 consumer-owned、小接口和真实使用后再抽象。

## 8. 为什么线性链不等于规则引擎？

- **直接回答：** 当前链是编译期固定的两个 concrete gate 加确定 for-loop；它没有声明式规则语言、数据模型、动态装载、冲突消解、发布/回滚、bundle、权限、调试、持久版本或非研发编辑能力。<code>RuleSetRevision</code> 只是标识这份固定顺序，不会把代码自动升级为 engine。
- **追问：** 如果把步骤放进数据库，是不是就成规则引擎？
  - **追问回答：** 不是。数据库只解决存储，还要定义合法表达式、类型检查、安全执行、组合算法、版本激活、回放、迁移、权限和故障语义；一个 string-to-function map 也不是完整引擎。
- **权衡：** 领域代码上线需要研发发布，但类型安全、可测试、变更面小；真正引擎适合规则由多团队独立发布且复杂度已有证据时。
- **代码 / 测试证据：** [固定 EligibilityPrerequisiteChain](../../../internal/participation/application/prerequisite_chain.go)、[架构禁止项](../../../internal/participation/application/architecture_test.go)、[ADR 方案六与停止线](../../decisions/ADR-0022-participation-prerequisite-chain.md)。
- **官方来源：** OPA 官方 [Policy Language](https://www.openpolicyagent.org/docs/policy-language)、[Integration](https://www.openpolicyagent.org/docs/integration)和 [Operations](https://www.openpolicyagent.org/docs/operations)展示了真实策略系统除 for-loop 外还需处理的语言、分发与运行边界；本项目没有实现或依赖 OPA。

## 9. 为什么执行顺序固定为新用户后风险？顺序为什么要版本化？

- **直接回答：** 注册事实通常低敏、低成本；风险 authority 更敏感、更昂贵且是独立故障域。先执行新用户规则能让确定非新用户不再触达风险源，并固定首要拒绝 reason。顺序会改变外部访问、延迟、暴露面和终止 reason，因此属于业务契约；调序必须使用新的 <code>RuleSetRevision</code>。
- **追问：** 两条都是 AND，数学结果不是与顺序无关吗？
  - **追问回答：** 最终真值可能相同，但执行行为不同：调用了哪些 authority、返回哪个 reason、观察到什么故障、花费多少延迟都可能变化。工程语义不只是真值表。
- **权衡：** 固定低成本优先降低敏感读取和故障传播；代价是若风险源将来极快而注册源极慢，仍不能私自调序，需用数据和新 revision 评审。
- **代码 / 测试证据：** [固定 plan 顺序](../../../internal/participation/application/prerequisite_chain.go)、[RuleSetRevision 类型与测试](../../../internal/participation/domain/new_user_policy.go)、[revision 独立性测试](../../../internal/participation/domain/risk_admission_policy_test.go)。
- **官方来源：** OASIS XACML 的 first-applicable/ordered combining algorithm 表明有序求值本身可以是策略语义；本项目不是 XACML，但同样不能把顺序当无害实现细节。
- **题型来源（非技术依据）：** 牛客用户的[腾讯云智面经](https://www.nowcoder.com/feed/main/detail/de7161a16ee44838a8377b0f89fecdb8)自述出现“执行流程、顺序和策略组合”追问，真实性未独立核验。

## 10. 短路怎样证明是真短路，而不是最后少算一个 bool？

- **直接回答：** 两个 reader 位于各自 step closure 内，按执行到该 step 时才读取。新用户 ineligible 时 runner 立即返回；新用户读取错误或 caller cancel 也直接返回零 aggregate。因此“风险 reader 调用次数为零”才是关键证据，单看最终 ineligible 不足以证明隐私、成本和故障隔离。
- **追问：** 未执行节点要不要在 trace 标记 skipped？
  - **追问回答：** 当前不标。trace 只陈述实际执行事实；把未访问风险源伪造成 skipped step 容易被误解为已完成一次风险判断。若未来产品确需规划视图，应与 executed trace 分开建模。
- **权衡：** 懒读取放弃批量预取优化，换取真实的敏感数据最小化和尾依赖隔离。
- **代码 / 测试证据：** [step closure 与立即返回](../../../internal/participation/application/prerequisite_chain.go)、[固定顺序、trace 长度与 risk zero-call 测试](../../../internal/participation/application/prerequisite_chain_test.go)、[短路验收矩阵](../../product/participation-prerequisite-chain-v1.md)。
- **官方来源：** OASIS XACML first-applicable 的规范语义是只在前项 NotApplicable 时继续，说明“是否继续”必须由明确结果驱动；本项目的 eligible/ineligible 语义不同，但同样要求可验证的停止点。
- **题型来源（非技术依据）：** 牛客用户的[美团暑期后端面经](https://www.nowcoder.com/discuss/732945501980565504)自述出现“怎么验证链、如何单测和监控”的追问，真实性未独立核验。

## 11. 为什么不并行读取注册事实和风险事实，再一起判断？

- **直接回答：** 并行可以降低两次读取都发生时的尾延迟，但会让已确定非新用户仍访问更敏感的风险源，增加成本、隐私暴露和故障传播，也失去前序拒绝后的零调用保证。当前优先级是最小披露与稳定终止语义，不是尚未测量的延迟优化。
- **追问：** 什么时候值得改成并行？
  - **追问回答：** 必须先有真实 QPS、两源延迟分布、风险读取成本、数据分级和产品 SLO，并明确愿意放弃短路隐私收益；这会改变行为契约，需要新 ADR、ruleset revision 和回归证据。
- **权衡：** 顺序读取增加全通过路径的延迟，换来拒绝路径更低成本和更小依赖面。
- **代码 / 测试证据：** [顺序 for-loop](../../../internal/participation/application/prerequisite_chain.go)、[ADR 的预取方案评估](../../decisions/ADR-0022-participation-prerequisite-chain.md)。
- **官方来源：** Go 官方 [context](https://pkg.go.dev/context)支持并发取消传播，但不会替业务决定哪些 I/O 应发生；并发能力不是并行读取的业务授权。

## 12. 为什么 chain 在事实读取前捕获一次 evaluated-at，而第 25 节 standalone service 是读完 fact 后捕获？

- **直接回答：** 第 25 节 standalone 契约回答“读到单一事实后，在形成决定时它有多旧”；第 26 节需要两个懒读取共享一个逻辑 as-of，若读完两个再取时刻，会让后序慢读取改变前序 freshness，若各取一次又形成混合时间。因此 chain 在任何 fact I/O 前只取一次，作为整次规则集求值基准，同时保持 standalone API 兼容。
- **追问：** 为什么不统一修改第 25 节行为？
  - **追问回答：** 没有必要为新组合器破坏已验收的独立服务契约。两条入口的时点语义不同但都明确；未来若合并，应单独评估兼容性和调用者预期。
- **权衡：** 预先捕获提供一致 as-of 与可解释 trace；代价是读取期间刚产生的新事实不属于本次评估。
- **代码 / 测试证据：** [evaluationInstant token](../../../internal/participation/application/evaluation_instant.go)、[chain 捕获顺序](../../../internal/participation/application/prerequisite_chain.go)、[一次 UTC as-of 与阻塞 reader 不重取 clock 测试](../../../internal/participation/application/prerequisite_chain_test.go)、[standalone service 顺序](../../../internal/participation/application/new_user_eligibility.go)。
- **官方来源：** Go 官方 [time](https://pkg.go.dev/time)只定义时间值与比较原语；“哪个 instant 是业务 as-of”必须由应用契约明确。

## 13. source 在读取期间返回一个晚于 evaluated-at 的新快照，为什么不直接使用？

- **直接回答：** 本次评估声明自己基于一个固定 logical as-of；晚于该时刻形成的 <code>ObservedAt</code> 或 <code>AssessedAt</code> 不属于这个时间切片。接受它会让 trace 声称在时刻 T 使用了 T 之后才存在的证据，破坏回放和解释，所以返回 invalid，留给下一次评估。
- **追问：** 这会不会拒绝其实更准确的新数据？
  - **追问回答：** 会使本次无法决定，但不会伪造旧时点。若 provider 无法按 as-of 返回快照，未来 adapter 可以定义版本读取、短暂重试或明确 unavailable；不能悄悄混合时点。
- **权衡：** 严格 as-of 可能降低可用性，但保护因果一致性；放宽则要显式记录多时点而不是假装单时点。
- **代码 / 测试证据：** [注册 future 检查](../../../internal/participation/application/new_user_eligibility.go)、[风险 future 检查](../../../internal/participation/application/risk_admission.go)、[风险晚 1ns 测试](../../../internal/participation/application/risk_admission_test.go)。
- **官方来源：** OpenFeature 官方 [Evaluation details 类型](https://openfeature.dev/specification/types/)体现求值结果需要 reason/error/context 等明确元数据；这里只作可解释求值类比，本项目不是 feature flag 系统。

## 14. 为什么风险 freshness 从 AssessedAt 算，而不是 adapter 的读取时间？

- **直接回答：** <code>AssessedAt</code> 是风险 authority 形成 verdict 的源时间；读取时间只说明 adapter 何时拿到它。若每次读取都把旧 verdict 盖成“刚读取”，任何缓存或重复查询都能无限续鲜，风险策略变化也无法被发现。age 等于 max 仍有效，严格大于 max 才 stale。
- **追问：** provider 的时钟不准怎么办？
  - **追问回答：** UTC 规范化只统一表示，不证明时钟准确。生产 adapter 仍需来源 SLA、同步监控、允许偏差和异常告警；当前没有这些外部证据，不能声称跨系统时间完全可信。
- **权衡：** 源时间更诚实但依赖 provider 时钟质量；本地读取时间可用却掩盖事实年龄。
- **代码 / 测试证据：** [AssessedAt 所有权注释](../../../internal/participation/domain/risk_screening_fact.go)、[freshness 检查](../../../internal/participation/application/risk_admission.go)、[exact max 与 stale 1ns 测试](../../../internal/participation/application/risk_admission_test.go)。
- **官方来源：** Go 官方 [time](https://pkg.go.dev/time)用于 instant 比较；freshness 边界本身来自项目产品契约，而不是标准库默认。

## 15. 为什么单个 step eligible 不等于最终 eligible？

- **直接回答：** step eligible 只表示该必经条件已满足、runner 可以继续。新用户 eligible 后仍可能被风险 blocked，只有 plan 完整遍历后才构造最终 <code>eligible + all_prerequisites_satisfied</code>。这正是不能照搬“第一个 handler 成功就结束”的原因。
- **追问：** 为什么最终成功不用最后一个节点的 <code>risk_screening_passed</code>？
  - **追问回答：** 那只说明风险节点通过，不能证明新用户节点也执行并通过。aggregate success reason 明确表达“全部前置条件满足”，详细原因仍留在两步 trace。
- **权衡：** 区分 step 与 aggregate 增加一个结果类型，却避免局部成功被调用方误用为全局授权。
- **代码 / 测试证据：** [runner 的 continue 与最终构造](../../../internal/participation/application/prerequisite_chain.go)、[全部通过、首步拒绝和末步拒绝测试](../../../internal/participation/application/prerequisite_chain_test.go)、[ReasonAllPrerequisitesSatisfied 常量与 ruleset/reason 测试](../../../internal/participation/domain/risk_admission_policy_test.go)。
- **官方来源：** OASIS XACML 的 combining algorithms说明单个 rule result 与最终 policy decision 是不同层次；本项目只借鉴层次区分。

## 16. ineligible、技术 error 与 caller cancellation 在链上分别怎样终止？

- **直接回答：** ineligible 是有效事实和 policy 得出的确定业务结论，返回 <code>Confirmed() == true</code> 的非零 aggregate、终止节点 reason、nil error；配置、clock、fact 或 provider 失败返回 <code>Confirmed() == false</code> 的零 aggregate 加稳定 error；caller cancel/deadline 也返回零 aggregate，并保留可由 <code>errors.Is</code> 识别的原始 context error。三者都停止 tail，但语义和恢复动作不同。
- **追问：** 是否应该把 cancel 记录成某个 reason code？
  - **追问回答：** 不应该把 caller 生命周期伪装成业务理由。未来 observer 可以记录安全的 error class 或 cancellation signal，但不能生成 <code>risk_screening_blocked</code> 一类业务 reason。
- **权衡：** 多通道结果要求调用方正确处理 error；好处是重试、告警、用户文案和业务统计不会互相污染。
- **代码 / 测试证据：** [Evaluate、Confirmed 与返回分支](../../../internal/participation/application/prerequisite_chain.go)、[稳定 errors](../../../internal/participation/application/errors.go)、[组合层业务/技术/cancellation 测试](../../../internal/participation/application/prerequisite_chain_test.go)。
- **官方来源：** Go 官方 [context](https://pkg.go.dev/context)定义 Canceled 与 DeadlineExceeded；[errors.Is](https://pkg.go.dev/errors#Is)定义错误链匹配。OASIS XACML 用 Indeterminate 区分无法形成正常决定，可作概念校准。

## 17. 为什么 ruleset、rule、policy、fact 和 application version 必须分开？

- **直接回答：** rule code 标识稳定算法语义；policy revision 标识某条规则的参数/解释快照；fact revision 标识外部 authority 的证据版本；ruleset revision 标识具体规则及其顺序组合；application version 标识部署构建。它们变更频率和所有者不同，不能用一个 Git SHA 或更新时间代替。
- **追问：** 只要 trace 有两个 policy revision，为什么还需要 ruleset revision？
  - **追问回答：** 同样两份 policy 可以按不同顺序、缺失某节点或使用不同组合算法；ruleset revision 固定组合契约，而不是重复单规则版本。
- **权衡：** 多维 revision 增加传递和治理成本，但使回放、灰度比较和事故定位不再依赖猜测。
- **代码 / 测试证据：** [RuleSetRevision 定义](../../../internal/participation/domain/new_user_policy.go)、[step 与 aggregate revision 字段](../../../internal/participation/application/prerequisite_chain.go)、[revision 独立性测试](../../../internal/participation/domain/risk_admission_policy_test.go)。
- **官方来源：** OPA 官方 [Bundles 与 discovery 运行资料](https://www.openpolicyagent.org/docs/operations)说明策略内容、分发和运行版本需要独立治理；本项目当前仍是代码内固定 ruleset。

## 18. PrerequisiteEvaluation 的 trace 为什么只记录实际执行节点？

- **直接回答：** trace 是本次确定业务结果的执行证据，不是 plan 展示。新用户拒绝时只记录新用户 step；风险从未读取，就不能记录风险成功、失败或伪造的 skipped decision。全通过或风险阻断时才有两步，顺序与执行顺序一致。
- **追问：** 运维怎样知道 tail 是短路还是配置里根本没有？
  - **追问回答：** aggregate 的 ruleset revision 决定计划版本，executed trace 说明实际路径；未来 plan registry 或 observer 可以关联 revision 展示未执行原因，但不应修改业务 trace 的事实含义。
- **权衡：** executed-only trace 最诚实且最小；调试完整计划需要另一个受控视图。
- **代码 / 测试证据：** [trace append 与短路位置](../../../internal/participation/application/prerequisite_chain.go)、[一/两步 executed trace 测试](../../../internal/participation/application/prerequisite_chain_test.go)、[trace 契约](../../product/participation-prerequisite-chain-v1.md)。
- **官方来源：** OpenTelemetry [Trace API](https://opentelemetry.io/docs/specs/otel/trace/api/)区分实际 span/event 记录与传播上下文；本项目 trace 不是 OTel span，但同样不应捏造未发生事件。

## 19. trace 为什么保留 fact source/revision，却不保留 ParticipantRef、风险分数和阈值？

- **直接回答：** source/revision 是解释“使用了哪份权威快照”的最小 provenance；ParticipantRef、设备/模型特征、分数、阈值和原始 payload 会扩大隐私与泄漏风险，而且组合器不需要它们判断。Participation 消费 source-owned verdict，不复制风险模型内部。
- **追问：** fact revision 会不会本身包含用户信息？
  - **追问回答：** 长度和字符校验不能证明无 PII，所以 provider contract 必须规定 opaque、受控格式；普通指标 label 更不能使用 revision。真实 adapter 还需数据分级、保留期与访问审计。
- **权衡：** 最小 trace 支持定位但不支持完整风险取证；深度诊断应留在风险 authority 的受控系统中。
- **代码 / 测试证据：** [EligibilityTraceStep 字段](../../../internal/participation/application/prerequisite_chain.go)、[风险快照最小字段](../../../internal/participation/domain/risk_screening_fact.go)、[metadata 反例测试](../../../internal/participation/domain/risk_screening_fact_test.go)。
- **官方来源：** OpenTelemetry [Metrics SDK](https://opentelemetry.io/docs/specs/otel/metrics/sdk/)明确讨论 cardinality limit；低基数 metric 与高基数诊断字段需要不同治理。

## 20. Steps 为什么返回副本？这就完全不可变了吗？

- **直接回答：** 构造 aggregate 时先复制输入 slice，<code>Steps()</code> 再返回一份副本，调用方不能通过共享 backing array 改写已形成的 trace。每个 step 字段私有且只提供 getter，外部也无法直接修改。
- **追问：** time.Time 或 string 是否还会暴露可变引用？
  - **追问回答：** string 是不可变值；time.Time 按值返回。当前 step 没有 map、slice 或 pointer payload，因此浅拷贝足够。将来若增加复合字段，必须重新审查深拷贝和所有权。
- **权衡：** 小 slice 复制有少量分配成本，换来结果证据不被调用方改写；当前只有一到两步，成本可忽略但仍应由 profile 而非猜测判断。
- **代码 / 测试证据：** [newPrerequisiteEvaluation 与 Steps](../../../internal/participation/application/prerequisite_chain.go)、[篡改返回 slice 不影响存储证据的测试](../../../internal/participation/application/prerequisite_chain_test.go)、[不可变输出契约](../../product/participation-prerequisite-chain-v1.md)。
- **官方来源：** Go 官方 [Slices: usage and internals](https://go.dev/blog/slices-intro)说明 slice 描述符共享底层数组，因此复制元素而非只复制 slice header 才能隔离改写。

## 21. 为什么技术失败返回零 aggregate，不把已通过的前半段 trace 一并返回？

- **直接回答：** 半条 trace 不是最终业务决定。若新用户通过后风险 reader unavailable，调用方看到非零 aggregate 很容易误把前半段 eligible 当最终允许。当前契约让 <code>PrerequisiteEvaluation{}</code> 明确表示无可信结论，失败定位通过稳定 error class 和未来受控 observer 完成。
- **追问：** 这样是否损失排障信息？
  - **追问回答：** 对普通调用方是有意损失。高权限诊断可以在 step 边界记录低敏事件，但不能把半成品 decision 暴露成业务 API；本节尚未实现 observer 或审计。
- **权衡：** 零 aggregate 降低误用风险，但未来需要单独建设安全诊断通道。
- **代码 / 测试证据：** [所有 error/cancel 分支返回零值](../../../internal/participation/application/prerequisite_chain.go)、[注册/风险技术失败零 aggregate 测试](../../../internal/participation/application/prerequisite_chain_test.go)、[风险 helper 零 decision 测试](../../../internal/participation/application/risk_admission_test.go)、[ADR 的 trace 决策](../../decisions/ADR-0022-participation-prerequisite-chain.md)。
- **官方来源：** OpenTelemetry [Recording errors](https://opentelemetry.io/docs/specs/semconv/general/recording-errors/)建议用受控 telemetry 表达错误；业务返回值不需要兼任完整诊断载体。

## 22. 为什么 error wrapper 用 Cause 而不再用 Unwrap？

- **直接回答：** <code>errors.Is</code> 会遍历 Unwrap 链。若公开 class 是 unavailable，而 cause 恰好又包装 not-found 或 context deadline，一个 error 可能同时命中多个语义 class，HTTP/重试映射就会歧义。当前 wrapper 自定义 <code>Is</code> 只暴露一个经过评审的 class，<code>Error()</code> 只渲染安全文案，原 cause 通过显式 <code>Cause()</code> 留给可信诊断代码但不进入标准 error tree。
- **追问：** 自定义 Cause 会不会让通用日志工具看不到根因？
  - **追问回答：** 会，这是刻意的信任边界。需要根因的受控 observer 必须显式认识该接口；普通 transport/logger 不应自动展开 SQL、地址、subject detail 或第二语义 sentinel。
- **权衡：** 放弃通用 Unwrap 生态的一部分便利，换取 exactly-one public class 和默认不泄密。
- **代码 / 测试证据：** [RegistrationFactReadError 与 RiskScreeningFactReadError](../../../internal/participation/application/errors.go)、[单一 class 测试](../../../internal/participation/application/errors_test.go)、[风险 secret/cause 测试](../../../internal/participation/application/risk_admission_test.go)。
- **官方来源：** Go 官方 [errors 包](https://pkg.go.dev/errors)和 [Go 1.13 errors](https://go.dev/blog/go1.13-errors)明确说明 <code>Is</code> 会检查 error tree 及自定义 Is 方法；是否公开 cause 因而是 API 契约。

## 23. provider 自己 timeout 与 caller cancellation 怎样区分？谁优先？

- **直接回答：** reader 返回后先检查传入 <code>ctx.Err()</code>。若 caller context 已取消，原始 Canceled/DeadlineExceeded 胜出；若 caller 仍存活但 provider 返回 context deadline/cancel，这代表 adapter 的内部预算失败，分类为对应 fact unavailable，并把原始 cause 放进受控 Cause 通道。
- **追问：** 为什么仅检查返回 error 中是否含 DeadlineExceeded 不够？
  - **追问回答：** 同一个 sentinel 既可能来自 caller，也可能来自 provider 的子 context。只有 caller context 当前状态能说明此次请求生命周期是否被调用方终止。
- **权衡：** 边界后多一次 ctx 检查使归因更精确；仍无法定义物理上的绝对同时顺序。
- **代码 / 测试证据：** [readRegistrationFact](../../../internal/participation/application/new_user_eligibility.go)、[readRiskScreeningFact](../../../internal/participation/application/risk_admission.go)、[caller cancel 胜出与 provider deadline 测试](../../../internal/participation/application/risk_admission_test.go)。
- **官方来源：** Go 官方 [context](https://pkg.go.dev/context)定义 Done/Err 的协作取消语义；Go 官方 [errors.Is](https://pkg.go.dev/errors#Is)只回答 error tree 匹配，不回答错误由哪个预算产生。

## 24. 为什么 context 必须显式作为第一个参数，不能存进 chain 或放业务事实？

- **直接回答：** context 表达单次调用的 deadline、cancellation 和 request-scoped metadata，应由调用链显式传递并作为首参；chain 是可复用服务，若把 context 存进 struct，会跨请求共享生命周期并破坏并发安全。ParticipantRef、policy、ruleset revision 等业务必需参数也应显式传入，不能藏在 <code>ctx.Value</code>。
- **追问：** trace ID 可以放 context，为什么 ParticipantRef 不行？
  - **追问回答：** trace ID 是跨 API/进程的 request-scoped metadata；ParticipantRef 是函数正确性所必需的业务输入。隐藏必需参数会让签名、测试和权限审查失真。
- **权衡：** 显式签名较长，但类型、依赖和调用责任清晰；context value 只保留真正跨边界的请求元数据。
- **代码 / 测试证据：** [Evaluate 显式签名](../../../internal/participation/application/prerequisite_chain.go)、[reader ports 的 context 首参](../../../internal/participation/application/ports.go)。
- **官方来源：** Go 官方 [context 包](https://pkg.go.dev/context)要求 Context 作为首参、不要存入 struct，并指出 Values 只应用于跨 API/进程的 request-scoped data；[Contexts and structs](https://go.dev/blog/context-and-structs)进一步解释例外边界。
- **题型来源（非技术依据）：** 牛客用户的[字节生活服务 Go 面经](https://www.nowcoder.com/feed/main/detail/d9c7149407d9456d80d863274b1409b2)自述出现“哪些放 context、哪些显式传参”的追问，真实性未独立核验。

## 25. runner 为什么在 clock、每个 step 前后都检查 ctx.Err？还漏掉什么？

- **直接回答：** pre-check 阻止已取消请求触达依赖；clock 后检查让捕获同时发生的取消优先；每步前阻止启动新 I/O；每步后检查让 reader/evaluator 返回同时已经可观察到的 caller cancellation 胜出。它提供清晰的协作取消点。
- **追问：** 这样能强制中断一个忽略 context 的 reader 吗？
  - **追问回答：** 不能。context 不是线程中断；reader 必须主动监听 Done 或使用支持 context 的客户端。若 reader 永不返回，runner 无法越过调用边界观察取消。
- **权衡：** 多检查成本极低且归因清晰；真正的 timeout 仍依赖 adapter 合作、连接池和下游预算。
- **代码 / 测试证据：** [Evaluate 的 cancellation checkpoints](../../../internal/participation/application/prerequisite_chain.go)、[clock/注册/risk 各边界 cancellation 测试](../../../internal/participation/application/prerequisite_chain_test.go)、[standalone blocking reader 测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **官方来源：** Go 官方 [context](https://pkg.go.dev/context)明确取消是通过 Done channel 协作传播，调用函数需要主动停止工作。

## 26. typed-nil reader 或 Clock 为什么危险？当前怎样 fail early？

- **直接回答：** Go interface 包含动态类型和值；<code>(*Reader)(nil)</code> 放进接口后，接口本身不等于 nil。若只做普通 nil 判断，服务会看似已配置，运行时调用方法时可能 panic。构造器和 <code>Validate</code> 使用包内 <code>dependencyIsNil</code> 检查 reader、Clock 和 typed-nil function，并拒绝非正 freshness。
- **追问：** reflection 是否值得？
  - **追问回答：** 当前 reflection 只位于 composition guard，不进入规则求值或通用事实系统。替代方案是每个 adapter 自证非 nil 或生成式 DI，但在没有更强构造约束前不能忽略已知 interface 陷阱。
- **权衡：** 一次构造/调用前反射换取更确定的配置错误；它不证明 adapter 内部连接、权限或并发安全。
- **代码 / 测试证据：** [dependencyIsNil](../../../internal/participation/application/new_user_eligibility.go)、[chain Validate](../../../internal/participation/application/prerequisite_chain.go)、[chain 两 reader/clock/ClockFunc typed-nil 测试](../../../internal/participation/application/prerequisite_chain_test.go)。
- **官方来源：** Go 官方 [FAQ 的 nil error 说明](https://go.dev/doc/faq#nil_error)与[语言规范的 Interface types](https://go.dev/ref/spec#Interface_types)说明动态类型/值共同决定 interface 是否为 nil。

## 27. step 已经由包内代码构造，为什么还要 validate rule code、outcome 和 evaluated-at？

- **直接回答：** 私有不等于永不出错。closure 可能接错 evaluator、投影漏字段、返回错误 rule code、未知 outcome 或不同时间。runner 在把 step 写入确定 trace 前验证 expected code、允许 outcome、非空 reason/policy/source/revision 和统一 evaluated-at；违反内部协议就返回 <code>ErrPrerequisiteStepInvalid</code> 加零 aggregate。
- **追问：** 这是不是把 programmer bug 当普通运行时错误吞掉？
  - **追问回答：** 它没有恢复 panic，也没有宣称自动修复；它阻止损坏结果穿过业务边界，并提供稳定告警 class。测试和 code review 仍应尽早发现 bug。
- **权衡：** 防御性校验增加少量分支，换取业务证据在内部重构后仍 fail closed。
- **代码 / 测试证据：** [EligibilityTraceStep.validate](../../../internal/participation/application/prerequisite_chain.go)、[ErrPrerequisiteStepInvalid](../../../internal/participation/application/errors.go)、[domain 决定字段测试](../../../internal/participation/domain/risk_admission_test.go)。
- **官方来源：** Go 官方 [Code Review Comments](https://go.dev/wiki/CodeReviewComments)强调清晰错误处理与接口边界；具体不变量来自项目契约。

## 28. 为什么链内节点必须只读？中途失败为什么没有 rollback？

- **直接回答：** 当前两个节点只读取权威事实并做纯决定，没有创建 Participation、扣次数、选择 Award、扣库存或发消息，所以短路不需要补偿。若把副作用塞进 step，前几步成功、后一步失败就会引入幂等、事务、结果未知、补偿和恢复问题；那已经不是本节的只读资格链。
- **追问：** 将来正式参与流程能不能仍用这条链？
  - **追问回答：** 可以把它作为写入前的只读门控，但额度/库存承诺必须在自己的原子边界重新校验或预占；不能把一次 eligible 当长期锁。
- **权衡：** 只读链简单、可重试、可并发；它不解决资格检查与后续写入之间的 TOCTOU。
- **代码 / 测试证据：** [两个 reader-only ports](../../../internal/participation/application/ports.go)、[纯 domain evaluator](../../../internal/participation/domain/risk_admission.go)、[ADR 的副作用停止线](../../decisions/ADR-0022-participation-prerequisite-chain.md)。
- **官方来源：** Go 官方 context 文档说明取消与资源释放，不提供分布式事务或补偿语义；这些必须由业务写模型另行设计。
- **题型来源（非技术依据）：** 牛客用户的[美团实习面经](https://www.nowcoder.com/feed/main/detail/7a197c54ad294c5496eb6146a6f740d3)自述出现“中间失败如何回滚”，[百度营销面经](https://www.nowcoder.com/feed/main/detail/96aa803ee7c94f14bd50325fd6fc1e9d)自述出现“为什么不把创建动作放进链”，真实性均未独立核验。

## 29. 未来怎样做 metrics、trace 和 error 记录，才不会把业务拒绝当系统故障？

- **直接回答：** 普通指标只用经过评审的低基数 rule/outcome/reason/error class；ParticipantRef、fact revision、时间和原始 cause 不做 label。确定 ineligible 是正常业务结果，不应仅因“拒绝”自动把 span status 设为 Error；技术失败才按语义记录 error type/status。exception event 与 span status/metrics 要约定一个负责点，避免同一错误在每层重复记录。
- **追问：** 当前 EligibilityTraceStep 是不是已经等于分布式 trace？
  - **追问回答：** 不是。它是进程内业务执行证据，没有 trace/span ID、传播、采样、exporter、持久化或跨服务因果关系。未来可以把安全字段投影到 OTel，但两者不能混名。
- **权衡：** 低基数聚合便于告警且成本可控；深度诊断依赖采样、受控日志或 provider 侧查询。
- **代码 / 测试证据：** [trace 字段和敏感字段省略](../../../internal/participation/application/prerequisite_chain.go)、[安全 error wrapper](../../../internal/participation/application/errors.go)、[观测停止线](../../product/participation-prerequisite-chain-v1.md)。
- **官方来源：** OpenTelemetry [Recording errors](https://opentelemetry.io/docs/specs/semconv/general/recording-errors/)要求只在操作按语义失败时记录错误；[error.type](https://opentelemetry.io/docs/specs/semconv/registry/attributes/error/)和 [Metrics SDK](https://opentelemetry.io/docs/specs/otel/metrics/sdk/)用于校准错误分类与 cardinality。W3C [Trace Context](https://www.w3.org/TR/trace-context/)定义传播格式，不定义本项目业务 trace。

## 30. 一个 evaluated-at 能保证两次跨 authority 读取是原子快照吗？

- **直接回答：** 不能。它只保证两个 evaluator使用同一逻辑 as-of，并拒绝明显来自该时刻之后的快照；registration 与 risk 仍是顺序、独立读取，期间可能发生更新，provider 也未证明支持历史 as-of 查询。这是可解释性增强，不是分布式 snapshot isolation。
- **追问：** 正式系统怎样进一步加强？
  - **追问回答：** 取决于业务风险：可以要求来源返回不可变 revision、支持 as-of/version read、把决定绑定到事件版本、在写入时重检，或用统一投影/事务边界；没有真实 adapter 和一致性 SLA 前不能预选。
- **权衡：** 当前方案不引入跨系统协调，简单且边界诚实；代价是仍存在跨 authority TOCTOU。
- **代码 / 测试证据：** [一次 instant 与顺序读取](../../../internal/participation/application/prerequisite_chain.go)、[future/freshness 校验](../../../internal/participation/application/risk_admission.go)、[共享 as-of 与 after-as-of 失败测试](../../../internal/participation/application/prerequisite_chain_test.go)、[ADR 的剩余风险](../../decisions/ADR-0022-participation-prerequisite-chain.md)。
- **官方来源：** Go 标准库 context/time 只能控制调用生命周期和本地时点，不能提供跨数据源事务；一致性承诺必须由外部系统协议证明。

## 31. 这条链并发安全吗？为什么不加锁、缓存或 singleflight？

- **直接回答：** chain 构造后只保存 reader、Clock 和两个 duration，Evaluate 的 instant、plan、trace 都是调用栈局部值，没有共享请求态，因此自身不需要锁。真正并发安全还依赖注入的 readers/Clock；当前也没有重复主体流量、共享 freshness、撤销或 provider 成本数据，不能先加缓存/singleflight 改变 caller deadline 与 fact age 语义。
- **追问：** 线性两次 I/O 会不会性能差？
  - **追问回答：** 全通过路径会累加两源延迟，短路路径只支付前序成本。没有 benchmark、生产 adapter 和延迟分布时只能陈述复杂度与调用次数，不能声称达到某个 QPS/SLO。
- **权衡：** 无共享状态降低 race 和失效复杂度；可能重复读取同一主体事实，等真实 profile 再设计。
- **代码 / 测试证据：** [chain 的只读字段与局部 plan](../../../internal/participation/application/prerequisite_chain.go)、[chain 64 goroutine 一致结果与精确调用次数测试](../../../internal/participation/application/prerequisite_chain_test.go)、[Race Detector 验收要求](../../product/participation-prerequisite-chain-v1.md)。
- **官方来源：** Go 官方 [Race Detector](https://go.dev/doc/articles/race_detector)说明 race 检测是动态证据，只覆盖实际执行路径；测试通过也不替被注入 adapter 保证线程安全。

## 32. 你怎样测试顺序、短路、时间和错误，而不是只覆盖 happy path？

- **直接回答：** 领域层覆盖 passed/blocked、UTC 确定性、future 1ns、zero/unknown 与 fuzz 边界；application helper 覆盖 exact freshness、stale 1ns、主体不符、source future、provider error 脱敏、caller cancellation 优先和 typed nil；组合层应以调用顺序、两个 reader/clock 精确次数、trace 长度/顺序、零 aggregate 和副本隔离为断言；architecture test 还禁止提前出现通用 Rule/Engine、泛型和 string-any fact bag。
- **追问：** 单元测试能证明真实用户已经被门控吗？
  - **追问回答：** 不能。没有生产 adapter、公开 route、session、Activity、Lottery 编排和浏览器 E2E；这里只证明内核语义。最终命令、race 和负面差异还必须在 QA 记录。
- **权衡：** 精确 call-count 和 1ns 边界测试能冻结契约，但对实现结构较敏感；应断言可观察业务行为，不锁私有函数细节。
- **代码 / 测试证据：** [组合顺序/短路/clock/cancel/zero/并发测试](../../../internal/participation/application/prerequisite_chain_test.go)、[risk domain tests](../../../internal/participation/domain/risk_admission_test.go)、[risk fact tests](../../../internal/participation/domain/risk_screening_fact_test.go)、[risk application tests](../../../internal/participation/application/risk_admission_test.go)、[architecture test](../../../internal/participation/application/architecture_test.go)。
- **官方来源：** Go 官方 [testing 包](https://pkg.go.dev/testing)与 [Race Detector](https://go.dev/doc/articles/race_detector)界定单测、fuzz 和动态竞态证据。
- **题型来源（非技术依据）：** 牛客用户的[美团暑期后端面经](https://www.nowcoder.com/discuss/732945501980565504)自述出现“链怎么单测、怎么监控”的追问，真实性未独立核验。

## 33. 为什么不用 Template Method、Strategy、middleware、Specification 或 OPA？

- **直接回答：** Template Method 适合固定算法骨架由子类/步骤定制，但 Go 这里没有继承层次；Strategy 更适合互斥算法选择，而两条规则是都必须通过；HTTP middleware 绑定 transport 且容易混入认证/日志，不适合 Participation domain；Specification 适合组合纯谓词，却不能自动解决两次 I/O、context、error 与 provenance；OPA 适合声明式策略和独立发布，但当前没有运营编辑、bundle、data contract、sidecar/embedded 运行和故障预算需求。
- **追问：** 是否意味着这些方案以后都不采用？
  - **追问回答：** 不是。真实变化轴出现后再比较：算法互斥用 Strategy，横切 transport 用 middleware，可组合纯谓词可考虑 Specification，跨团队独立策略发布再 PoC OPA；不能先用名称统一不相同的问题。
- **权衡：** 专用链可读、类型化、故障边界明确，但规则增多或需要非研发配置时发布效率可能不足。
- **代码 / 测试证据：** [Participation 专用 chain](../../../internal/participation/application/prerequisite_chain.go)、[禁止万能抽象的 architecture test](../../../internal/participation/application/architecture_test.go)、[基线的“组合器不是什么”](../../product/participation-prerequisite-chain-v1.md)。
- **官方来源：** OPA 官方 [Policy Language](https://www.openpolicyagent.org/docs/policy-language)和 [Integration](https://www.openpolicyagent.org/docs/integration)说明采用 OPA 意味着引入明确策略/数据/集成协议，而不只是把 if 移出 Go。
- **题型来源（非技术依据）：** 牛客用户的[百度后端面经](https://www.nowcoder.com/feed/main/detail/fa3706d7d465417f83d550b7f471941b)与[腾讯云智面经](https://www.nowcoder.com/feed/main/detail/de7161a16ee44838a8377b0f89fecdb8)自述出现 Template Method、责任链与 Strategy 对比题型，真实性未独立核验。

## 34. 到第 27 节出现什么证据时，线性链才算真的不够？

- **直接回答：** 真实会员分层若要求根据 tier 走不同后继、存在多出口、default branch、共享子路径、分支合流或 path trace，线性 <code>next index</code> 就无法诚实表达拓扑。若开始在 step 里返回隐藏 index、条件跳过多个节点、重复节点或用 closure 捕获隐式跳转，就是“链不够”的可执行证据，应升级为显式路由/规则树模型。
- **追问：** 为什么现在不先做树，避免下一节重构？
  - **追问回答：** 当前两条必经 AND gate 用树会提前引入 node ID、edge、cycle、reachability、default path、发布校验和持久化问题，却没有一个真实分支验证这些概念。下一节的小规模重构是用新需求校正模型，不是失败。
- **权衡：** 线性模型现在更小、更安全；未来分支出现时会有一次有证据的模型升级。提前建树减少表面重构，却增加长期错误抽象成本。
- **代码 / 测试证据：** [固定线性 runner](../../../internal/participation/application/prerequisite_chain.go)、[ADR 的演进触发条件](../../decisions/ADR-0022-participation-prerequisite-chain.md)、[基线剩余风险](../../product/participation-prerequisite-chain-v1.md)。
- **官方来源：** OASIS XACML 的多种 combining algorithms说明不同组合语义需要显式定义；它不能证明本项目现在需要树，只用于提醒“ordered”本身不等于所有分支/冲突语义。

## 35. 面试时怎样准确描述本节完成度，避免把内核能力夸成线上闭环？

- **直接回答：** 可以说已经建立第二条真实风险准入规则、consumer-owned 两事实端口、单一 logical as-of、固定短路 runner、稳定 error class、最小不可变 trace 和架构停止线；可以展示 domain/application/architecture 测试。不能说已经接入用户目录或风险系统、已经保护公开 Lottery API、已经认证授权、已经持久化审计或已经上线 OTel/SLO。
- **追问：** 没有 adapter 和 E2E，这一节是否“只是 demo”？
  - **追问回答：** 它是可执行且可验收的内核 vertical preparation，但不是完成的生产纵向链路。真实项目演进允许先冻结正确的模型和 consumer contract；前提是文档明确剩余边界，后续必须用 adapter/contract/integration/E2E 补齐，不能停在接口幻觉。
- **权衡：** 分层交付让每节风险可控、Git 证据清楚；代价是任何单节都必须诚实说明尚未闭环的部分。
- **代码 / 测试证据：** [ADR 的本节范围](../../decisions/ADR-0022-participation-prerequisite-chain.md)、[产品基线的架构停止线](../../product/participation-prerequisite-chain-v1.md)、[Participation application 实现](../../../internal/participation/application/prerequisite_chain.go)、[architecture test](../../../internal/participation/application/architecture_test.go)。
- **官方来源：** Go 官方 testing/race 资料说明测试证据的作用边界；OpenTelemetry、OPA 等官方资料也表明“设计了字段/端口”与“真实集成并运行”是不同完成层次。

## 不能夸大的结论

可以准确说：

- 已从两条真实 Participation 规则反推包内最小 ordered gate chain，而不是从模式名先造平台；
- 已固定 new-user → risk-admission 的版本化顺序，并把 tail zero-call 作为短路语义；
- 已用一次受控 logical evaluated-at 约束两条规则的 future/freshness 判断；
- 已区分 step eligible、aggregate eligible、确定 ineligible、技术 failure 与 caller cancellation；
- 已形成只含已执行节点、最小 provenance、可复制的 <code>PrerequisiteEvaluation</code>；
- 已让安全 error wrapper 对 <code>errors.Is</code> 只暴露一个稳定 class，并把详细 cause 放在显式受控通道；
- 已用 domain/application/architecture 测试覆盖风险边界、错误脱敏、取消、typed nil 与禁止提前引擎化；最终执行状态以 QA 为准。

不能说：

- 已接入真实用户目录、风险平台、数据库、Redis、RabbitMQ 或任何生产 adapter；
- 已有可信 session、Principal、RBAC、对象级授权，或 <code>ParticipantRef</code> 能证明调用者身份；
- 现有 Lottery API、React 页面或 Nginx gateway 已执行前置资格链；
- 已创建 Activity、正式 Participation/Draw、次数账户、幂等、事务、库存或发奖闭环；
- 一次 evaluated-at 已提供跨 authority 原子快照或解决 TOCTOU；
- 内部 EligibilityTraceStep 已经是分布式 trace、持久化审计或合规日志；
- 固定两节点 for-loop 已经是规则树、规则引擎、OPA/XACML、动态 DSL 或运营配置平台；
- 单元测试等于生产 E2E、容量基线、故障演练、安全评审或业务 SLO。

## 复习清单

- [ ] 能在 60 秒内解释为什么第 25 节不抽象、第 26 节只抽私有最小 step；
- [ ] 能区分 risk disposition 的所有者与 Participation admission decision 的所有者；
- [ ] 能画出 one clock → new-user → risk 的固定顺序，以及四类终止路径；
- [ ] 能说明 step eligible 只是 continue，全部完成后才 aggregate eligible；
- [ ] 能证明短路的核心是 tail reader 零调用，而不只是最终 bool；
- [ ] 能解释为什么不预取/并行，以及调序为什么必须升级 ruleset revision；
- [ ] 能区分 AssessedAt、ObservedAt、EvaluatedAt，并说明 after-as-of 与 stale 1ns；
- [ ] 能区分 rule、reason、policy revision、fact revision、ruleset revision 和 application version；
- [ ] 能说明技术失败为何返回零 aggregate，失败诊断为何不塞进半条业务 trace；
- [ ] 能解释 errors.Is、custom Is、Cause 与不提供 Unwrap 的安全权衡；
- [ ] 能解释 caller cancellation 与 provider timeout 的归因方式和 context 协作限制；
- [ ] 能解释 typed-nil interface 与 composition guard；
- [ ] 能列出 trace 保留和刻意省略的字段，并说明 metric cardinality；
- [ ] 能说明为什么只读链不需要 rollback，却仍未解决后续写入的 TOCTOU；
- [ ] 能对比责任链、Strategy、Template Method、middleware、Specification 和规则引擎；
- [ ] 能指出第 27 节出现多出口、default branch、合流或隐藏 next-index 时，线性链才有升级证据；
- [ ] 能同时讲清已完成的内核能力与未完成的 adapter、认证、HTTP、E2E 和 observability。
