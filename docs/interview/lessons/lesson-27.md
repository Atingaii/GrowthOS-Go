# 第 27 节面试题：责任链边界、会员多出口路由与显式缺省

本文面向“Go 业务路由建模、责任链边界、Strategy 对比、确定性、错误隔离、context、时间新鲜度、决策路径与可观测性”类面试。第 27 节没有把第 26 节 Participation 前置资格链扩成万能规则平台，而是在 Lottery 内实现了第一个具体会员等级路由：权威 `premium` 事实选择 premium Strategy，权威 `standard` 事实选择显式 baseline/default Strategy；未知、未支持、缺失、损坏、过期、future 或读取失败全部返回零 Route 加 error。

能力边界必须说清：当前只有 Lottery domain/application 内核、测试和架构停止线；没有真实会员 provider adapter、HTTP 路由、数据库/Redis、Activity 绑定、可信会话、Principal、RBAC、前端入口、持久化规则树、通用规则引擎或浏览器 E2E。`MembershipSubjectRef` 只是 Lottery 本地查找引用，不是登录身份或授权证明；Route 只返回 `StrategyID`，不加载 Strategy、不调用 `WeightedSelector`、不创建 Draw。

## 60 秒项目自述

> 第 26 节的责任链回答“所有资格 gate 是否都允许继续”，每个成功节点只有一个固定后继；第 27 节出现真实的会员分层需求，`premium` 和 `standard` 都是成功结果，却必须进入不同 Strategy 目标。这不是再加一个 continue/reject handler 能诚实表达的问题，所以我没有污染 Participation chain，而是在 Lottery 内建立具体、受限的一跳 router。
>
> 外部会员 authority 只拥有最小 tier fact：opaque subject ref、`standard/premium`、observed-at、source 和 fact revision；Lottery 拥有 policy revision、premium target、baseline/default target，以及最终 branch/target/path。服务先校验输入和依赖，检查 caller cancellation，只读取一次服务端 Clock，再读取一次事实，用同一 UTC as-of 校验 subject、future 和 freshness，最后交给纯领域函数确定路由。
>
> `baseline_default` 不是故障兜底，只对已确认的 `standard` 生效。unknown、unsupported、not-found、stale、provider timeout 都不会静默降级。成功决定保留稳定 rule/branch/reason、target、policy/fact provenance、evaluated-at 和一跳 path；错误返回零决定。读取错误只通过 `Cause()` 向显式受信诊断代码开放原始 cause，不让 provider 错误进入公共 `errors.Is` tree。这个具体 router 已足以证明线性链的表达边界，但还没有证据把它升级成持久化树或通用规则引擎。

## 来源与可信度

- **项目事实：** 以 [会员等级 Strategy 路由基线 v1](../../product/membership-strategy-routing-v1.md)、[ADR-0023](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)、Lottery domain/application 代码和测试为准。需求/ADR 说明批准语义，代码说明当前实现，测试说明覆盖意图；最终命令执行结果应以本节 QA 验收记录为准。
- **标准与官方资料：** 决策分支参考 OMG 官方 [DMN 1.4](https://www.omg.org/spec/DMN/1.4/PDF)和 OASIS 官方 [XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html)校准“命中策略、无匹配、求值失败、确定结果”等概念；本项目没有实现 DMN 或 XACML。复杂条件表达参考 Martin Fowler 原站的 [Decision Table](https://martinfowler.com/dslCatalog/decisionTable.html)与 [Adaptive Model](https://martinfowler.com/articles/refactoring-adaptive-model.html)，不因此宣称本节已经是规则引擎。
- **Go 官方资料：** 使用 Go 官方 [语言规范](https://go.dev/ref/spec)、[context](https://pkg.go.dev/context)、[Go 1.13 errors](https://go.dev/blog/go1.13-errors)、[interface nil FAQ](https://go.dev/doc/faq#nil_error)、[time](https://pkg.go.dev/time)、[Fuzzing](https://go.dev/doc/security/fuzz/)和 [Race Detector](https://go.dev/doc/articles/race_detector)解释语言与运行时语义。
- **隐私与可观测资料：** 使用 OpenTelemetry 官方 [Handling sensitive data](https://opentelemetry.io/docs/security/handling-sensitive-data/)、[Metrics SDK cardinality](https://opentelemetry.io/docs/specs/otel/metrics/sdk/#cardinality-limits)与 [error.type](https://opentelemetry.io/docs/specs/semconv/registry/attributes/error/)校准数据最小化和低基数；本节尚未接入 OpenTelemetry。
- 外部链接复核日期为 **2026-08-30**。

## 牛客面经真实性未独立核验声明

下列页面均为牛客社区用户的个人复盘，只用于观察面试官可能怎样追问，不能视为公司官方题库、录音原文或技术结论。项目团队没有独立核验发帖者身份、公司归属、面试轮次、题目原句或最终结果；本文不采用帖子中的答案，所有技术回答仍以官方资料、项目契约和可执行代码为准。

- [快手日常实习（支付方向）面经](https://www.nowcoder.com/discuss/730058801084125184)：用户自述遇到“设计模式比 if-else 更复杂时如何平衡”“Strategy 与责任链有什么区别”。
- [蚂蚁金服 Java 社招面经](https://www.nowcoder.com/discuss/353155231544451072)：用户自述遇到“使用责任链前后有什么变化”“责任链与 Strategy、Decorator 的区别”。
- [字节跳动 TikTok 后端实习面经](https://www.nowcoder.com/discuss/724635714276601856)：用户自述遇到“责任链和规则引擎的区别”“责任链的缺点”“重构优化了什么”。
- [美团 Java 岗面经](https://www.nowcoder.com/discuss/353154318717100032)：用户自述遇到“责任链怎样实现、有哪些角色/类、项目设计亮点是什么”。

---

## 1. 第 26 节责任链能做什么，为什么到第 27 节开始不够用？

- **直接回答：** 第 26 节是固定 AND gate：节点 `eligible` 只能进入固定下一项，`ineligible/error/cancel` 终止。第 27 节的 `standard` 和 `premium` 都是可信成功事实，却分别要进入 baseline 与 premium 两个合法目标。若仍用 continue/reject 协议，只能把 target 偷进 reason、在链外再写隐藏分支、返回 next-index，或复制两条链；真实出口和 path 都无法被类型直接表达。
- **追问：** “责任链也可以让 handler 返回任意对象，为什么一定不行？”
  - **追问回答：** 语言上可以，领域上会改义。Participation handler 一旦返回 Lottery `StrategyID`，资格链就同时承担准入和路由两个决定所有权；所谓复用只是把两个模型压进一个宽返回值。
- **权衡：** 保留 Participation 线性链并新增 Lottery concrete router，多了一个专用类型，却换来清楚的边界、出口、版本和失败语义；代价小于把现有链暗中改造成无校验图。
- **代码 / 测试证据：** [纯领域路由分支](../../../internal/lottery/domain/membership_routing.go)、[两个真实出口测试](../../../internal/lottery/domain/membership_routing_test.go)、[ADR 的方案比较](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)。
- **官方来源：** OMG [DMN 1.4](https://www.omg.org/spec/DMN/1.4/PDF)明确把 hit policy 与规则输出作为决策语义；Martin Fowler [Decision Table](https://martinfowler.com/dslCatalog/decisionTable.html)说明条件组合增加后应显式呈现输入到结果的关系。
- **题型来源（非技术依据）：** 牛客用户的[字节跳动面经](https://www.nowcoder.com/discuss/724635714276601856)自述出现“责任链有什么缺点”，真实性未独立核验。

## 2. 会员路由和 Strategy 模式是什么关系？

- **直接回答：** 本节做的是“选择哪一个 Lottery `StrategyID`”，不是在 router 内执行可互换算法。GrowthOS 的 Strategy 还是 Lottery 领域里的奖项权重聚合；`MembershipStrategyRoutingService` 只产生 target identity，不加载 Strategy、不调用 `WeightedSelector`。从控制流看它有“选择策略”的味道，但不能因此把 Route decision、Strategy aggregate 和选择算法叫成同一个对象。
- **追问：** “那为什么类型名里仍有 Strategy？”
  - **追问回答：** 因为输出确实是现有领域概念 `StrategyID`，不是设计模式标签。面试时应先说明项目领域术语，再谈模式相似性，避免把同名概念混淆。
- **权衡：** 分离 router 与 selector 多一次明确调用边界，却能分别验证事实 freshness、路由确定性、Strategy 存在性和加权随机性；若合并，依赖错误、业务路由和随机选择会共享一个难以解释的返回值。
- **代码 / 测试证据：** [MembershipStrategyRoutingService](../../../internal/lottery/application/membership_routing.go)、[现有 WeightedSelector](../../../internal/lottery/domain/weighted_selector.go)、[路由架构停止线测试](../../../internal/lottery/application/membership_routing_architecture_test.go)。
- **官方来源：** Martin Fowler [Adaptive Model](https://martinfowler.com/articles/refactoring-adaptive-model.html)展示了“决策数据”和“执行该模型的代码”可以分离；这用于解释边界，不表示本节已采用其通用 production-rule system。
- **题型来源（非技术依据）：** 牛客用户的[快手面经](https://www.nowcoder.com/discuss/730058801084125184)与[蚂蚁面经](https://www.nowcoder.com/discuss/353155231544451072)均自述出现 Strategy/责任链对比，真实性未独立核验。

## 3. `baseline_default`、`standard` 和 `unknown` 分别是什么？

- **直接回答：** `standard` 是 authority 明确确认且 v1 支持的会员事实；`baseline_default` 是 Lottery policy 给 confirmed standard 选择的显式基线边；`unknown` 表示没有形成受支持的可信事实，不是一个可路由会员等级。只有 standard 能走 default，unknown、unsupported、missing、stale、future 或 provider failure 都必须返回零 Route 加 error。
- **追问：** “既然 standard 是普通用户，unknown 降级成 standard 不是可用性更好吗？”
  - **追问回答：** 这会把“已确认普通会员”和“系统不知道”合并。依赖事故或新 tier 上线时，全部用户可能被静默路由到 baseline，既污染业务结果，也掩盖 provider/schema 故障。
- **权衡：** 失败关闭会在会员 authority 异常时降低可用性，但保住路由正确性与事故可见性；若未来产品接受 guest，应新增具名等级和 policy revision，而不是改变 unknown 的含义。
- **代码 / 测试证据：** [封闭 tier 与 Validate](../../../internal/lottery/domain/membership_fact.go)、[事实非法/fuzz 测试](../../../internal/lottery/domain/membership_fact_test.go)、[default 精确语义](../../product/membership-strategy-routing-v1.md)。
- **官方来源：** OASIS [XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html)区分 `NotApplicable`、`Indeterminate` 与确定 `Permit/Deny`，说明“无匹配/无法求值”和“确定业务结果”不应混写；这里只作语义类比。

## 4. 为什么没有单独的 reject Route outcome？

- **直接回答：** v1 产品只有两个确定成功出口，没有“受支持事实明确拒绝路由”的业务规则。unknown/unsupported/fact failure 是无法形成可信 Route 的技术边界，因此返回零 decision 加 typed error，而不是伪造一个 reject。将来若产品新增“已确认 restricted tier 必须拒绝”，才应新增具名业务结果、reason 和测试。
- **追问：** “失败关闭最终也不能继续，和 reject 有什么区别？”
  - **追问回答：** 控制动作相同不代表事实相同。业务 reject 通常不可重试且进入拒绝统计；dependency unavailable 可能重试并触发告警；caller cancellation 只表示调用者放弃。混合后 HTTP 映射、监控与客服解释都会错误。
- **权衡：** `Decision + error` 比 bool 复杂，但能保留恢复与归因语义；当前不预造 reject 则保持模型与真实产品规则一致。
- **代码 / 测试证据：** [RouteMembershipStrategy 的零错误决定](../../../internal/lottery/domain/membership_routing.go)、[application 零决定断言](../../../internal/lottery/application/membership_routing_test.go)、[失败契约](../../product/membership-strategy-routing-v1.md)。
- **官方来源：** OASIS [XACML 3.0 combining algorithms](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html)把 determinate、not-applicable 与 indeterminate 分开；Go 官方 [errors](https://pkg.go.dev/errors)提供独立错误分类机制。

## 5. 会员 authority 和 Lottery 各自拥有什么？

- **直接回答：** 外部会员 authority 拥有会员等级生命周期和事实 revision，只返回最小 `MembershipTierFactSnapshot`；Lottery 拥有 tier 到内部 Strategy target 的 policy、branch、reason 和 Route decision。authority 不能返回 GrowthOS `StrategyID`，Lottery 也不能升级/降级会员或解释完整权益。
- **追问：** “让 provider 直接返回 target 可以少一层映射，为什么不做？”
  - **追问回答：** 那会让外部系统拥有 GrowthOS 内部 Strategy 生命周期和发布兼容。provider 配置错误可直接指向内部资源，Lottery 也失去独立版本、回放和替换 provider 的能力。
- **权衡：** 防腐映射增加一份 policy 和 adapter 责任，但隔离外部协议与内部资源；当前 adapter 尚未实现，所以不能宣称已经完成真实集成。
- **代码 / 测试证据：** [consumer-owned reader port](../../../internal/lottery/application/membership_routing.go)、[最小事实](../../../internal/lottery/domain/membership_fact.go)、[具体 Lottery policy](../../../internal/lottery/domain/membership_routing_policy.go)。
- **官方来源：** Go 官方 [Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)建议接口由使用者定义；OASIS [XACML 数据流模型](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html)也将属性来源、策略和决定分为不同角色。这里只借鉴边界思想。

## 6. 为什么 Lottery 新建 `MembershipSubjectRef`，而不复用 ParticipantRef 或用户 ID？

- **直接回答：** 它只表示 Lottery 读取外部会员事实所需的 opaque key。复用 Participation 标识会制造跨上下文同一性承诺；叫 UserID/Principal 又会暗示它能证明登录身份、租户、角色或数据范围。当前没有可靠映射证据，所以使用本地窄类型并在注释中否定身份/授权含义。
- **追问：** “拿到这个 ref 的调用方是否就能查询会员路由？”
  - **追问回答：** 不能据此得出结论。当前没有公开 route；未来 transport 必须先从可信 session 形成 Principal，再做服务端授权和主体映射，不能接受浏览器自报 ref 就放行。
- **权衡：** 本地引用增加未来编排映射步骤，却避免在认证模型尚未建立时冻结错误全局 ID；第 31～35 节再用真实会话和权限事实统一。
- **代码 / 测试证据：** [MembershipSubjectRef 注释与校验](../../../internal/lottery/domain/membership_fact.go)、[跨上下文 import 停止线](../../../internal/lottery/application/membership_routing_architecture_test.go)、[所有权表](../../product/membership-strategy-routing-v1.md)。
- **官方来源：** OWASP 官方 [Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)区分身份认证与后续访问控制；一个业务查找键本身不是认证凭据。

## 7. 为什么 v1 tier 使用封闭枚举，而不是开放 string 或配置表？

- **直接回答：** 当前 authority 只确认 `standard/premium`，产品只批准这两种语义。封闭枚举让未来 `gold`、拼写错误、零值和未映射 provider 值全部显式失败，避免自动落入 default。等新增真实等级时再同步升级 fact vocabulary、policy revision、adapter contract 和兼容测试。
- **追问：** “这是否违反开闭原则？”
  - **追问回答：** 开闭原则不要求业务词汇永不修改。对安全关键事实，编译期/构造期显式变更比运行时吞掉未知值更可靠；过早开放只把变更风险从代码审查转移到未验证配置。
- **权衡：** 封闭词汇降低无代码扩展能力，却获得穷举分支、严格失败和清楚发布面；当运营配置需求真实出现时，再引入 schema/version/发布验证。
- **代码 / 测试证据：** [MembershipTier 常量与 Validate](../../../internal/lottery/domain/membership_fact.go)、[unsupported/zero 表驱动测试与 fuzz](../../../internal/lottery/domain/membership_fact_test.go)。
- **官方来源：** Go 官方 [Fuzzing](https://go.dev/doc/security/fuzz/)说明 fuzz 可探索人工遗漏的边界输入；本节用它证明任意非 `standard/premium` 字符串不会被接受。

## 8. 路由确定性具体指什么，怎样证明？

- **直接回答：** 相同完整 policy snapshot（revision + premium target + baseline target）、相同 fact snapshot、相同 logical evaluated-at 必须产生相同 rule、branch、reason、target 和 path。revision 单独只是关联 token，不能代表内容相同。实现使用 concrete switch，不读全局状态、不读取随机源；application 每次调用只捕获一次 Clock，domain 将时间规范为 UTC。确定性不表示跨系统原子快照，只表示给定完整输入下纯求值稳定。
- **追问：** “同一用户两次调用可能结果不同，是否违背确定性？”
  - **追问回答：** 如果 fact snapshot、policy snapshot 任一字段或 evaluated-at 变化，输入已经不同；当前还必须警惕上层错误复用同一个 policy revision 表示不同 targets。真正需要追查的是哪份完整输入变化，而不是要求业务永远不变。
- **权衡：** 保存 policy/fact provenance 增加决定字段，却使回放与差异定位成为可能；它仍不伪造不存在的 Strategy version。
- **代码 / 测试证据：** [纯 RouteMembershipStrategy](../../../internal/lottery/domain/membership_routing.go)、[UTC/重复确定性测试](../../../internal/lottery/domain/membership_routing_test.go)、[单次 Clock 测试](../../../internal/lottery/application/membership_routing_test.go)。
- **官方来源：** OMG [DMN 1.4](https://www.omg.org/spec/DMN/1.4/PDF)要求命中策略明确决定决策表语义；Go 官方 [time](https://pkg.go.dev/time)定义 `Time` 比较、`Sub` 与 location/monotonic 表示边界。

## 9. 为什么不能把 map 的第一项当 default？

- **直接回答：** Go 规范明确 map 迭代顺序未指定，也不保证两次相同。以“第一项”作为 default 会让同一配置在不同进程或不同迭代中选择不同 target。更严重的是，miss 的原因可能是 typo、新 tier 或损坏映射，自动拿任意值还会掩盖契约错误。
- **追问：** “先把 key 排序不就确定了吗？”
  - **追问回答：** 排序只能解决顺序，不能回答哪个未匹配输入被产品批准走 default。本节 default 的业务语义是 confirmed standard，而不是字典序第一项；两类问题不能用一个技术技巧替代。
- **权衡：** concrete switch 扩展时需要改代码和测试，但当前只有两个真实等级，审查成本低且语义最明确；第 28 节持久化模型再显式保存 default edge。
- **代码 / 测试证据：** [显式 tier switch](../../../internal/lottery/domain/membership_routing.go)、[stable branch/target 测试](../../../internal/lottery/domain/membership_routing_test.go)、[拒绝 map 方案的 ADR](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)。
- **官方来源：** Go [语言规范的 range 语义](https://go.dev/ref/spec#For_statements)明确指出 map 迭代顺序不确定且不保证重复一致。

## 10. premium 和 baseline target 为什么允许相同？

- **直接回答：** branch 是“哪条业务边被选择”的证据，target 是“当前边指向哪个 Strategy”的配置结果。灰度或回滚期间两个 branch 可能暂时收敛到同一 Strategy；强制不同会制造没有产品依据的约束。即使 target 相同，`premium_override` 与 `baseline_default` 仍必须保留不同 path。
- **追问：** “目标相同还记录 branch 有什么价值？”
  - **追问回答：** 它能说明是事实/路由判断一致，只是配置暂时汇合；若只看 target，会误以为会员分层没有生效，也无法安全评估后续再次拆分。
- **权衡：** 保留 branch 让证据更完整，但 branch 属会员派生信息，披露范围必须小于普通功能日志；不能因此把 branch 做成用户画像 label。
- **代码 / 测试证据：** [允许相同 target 的 policy](../../../internal/lottery/domain/membership_routing_policy.go)、[收敛 target 不抹除 branch 测试](../../../internal/lottery/domain/membership_routing_test.go)、[policy 构造测试](../../../internal/lottery/domain/membership_routing_policy_test.go)。
- **官方来源：** OMG [DMN 1.4](https://www.omg.org/spec/DMN/1.4/PDF)区分规则命中语义与输出；这里借鉴“命中哪条规则”和“输出值”可以是不同证据维度。

## 11. rule、branch、reason、target、policy revision 与 fact revision 为什么要分开？

- **直接回答：** rule 标识决策类型；branch 标识实际出口；reason 是稳定人机解释码；target 是后序资源 ID；policy revision 标识 Lottery 映射版本；fact revision 标识 authority 快照版本。把它们压成字符串会失去“规则没变但配置变了”“事实变了但 target 汇合”“同一 target 从不同 branch 到达”等诊断能力。
- **追问：** “为什么不把 application version 也当 policy revision？”
  - **追问回答：** application version 包含大量无关代码，无法准确说明映射语义；反过来 policy revision 也不证明 Strategy 已发布或存在。版本必须对应各自所有者和变更轴。
- **权衡：** 多个稳定 code/revision 增加命名治理成本，却减少错误归因；当前没有持久化审计，所以只能说决定对象具备最小内核证据。
- **代码 / 测试证据：** [decision 字段](../../../internal/lottery/domain/membership_routing.go)、[policy revision 类型](../../../internal/lottery/domain/membership_routing_policy.go)、[literal contract 测试](../../../internal/lottery/domain/membership_routing_test.go)。
- **官方来源：** OpenTelemetry 官方 [`error.type`](https://opentelemetry.io/docs/specs/semconv/registry/attributes/error/)强调错误分类值应可预测且低基数；这支持稳定 code 与任意错误文本/版本值分离，但本节尚未接入 OTel。

## 12. 为什么读取错误要提供 `Cause()`，却故意不实现 `Unwrap()`？

- **直接回答：** application 对调用方承诺的是一个经评审的稳定 class：not-found、unavailable、read-failure 或 invalid。原始 provider error 可能包含 endpoint、payload、SQL 或外部类型，只允许明确 opt-in 的受信诊断代码通过 `Cause()` 读取；不实现 `Unwrap()`，可避免原始 cause 自动加入公共 `errors.Is/As` 链并成为长期 API 契约。
- **追问：** “这是不是违背 Go 推荐的 `%w`？”
  - **追问回答：** Go 官方并不要求总是 wrap；它明确说是否暴露底层错误是 API 选择。caller 自身 cancellation 原样返回并可 `errors.Is`；provider deadline 在 caller 仍存活时只公开 unavailable，原始 deadline 留在 Cause。
- **权衡：** 不可自动 unwrap 会让通用错误工具少一部分细节，但换来抽象稳定与脱敏；需要建立受控 Cause 采集点，避免每层随意记录。
- **代码 / 测试证据：** [MembershipTierFactReadError](../../../internal/lottery/application/membership_routing_error.go)、[单一公开 class/Cause/无 Unwrap 测试](../../../internal/lottery/application/membership_routing_error_test.go)、[fact+error 与 provider deadline 测试](../../../internal/lottery/application/membership_routing_test.go)。
- **官方来源：** Go 官方 [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)明确指出 wrapping 会把底层错误暴露为调用者可依赖的 API，是否 wrap 应按抽象边界决定。

## 13. provider timeout 和 caller deadline 为什么不能混为一谈？

- **直接回答：** caller context 仍存活而 provider 内部超时时，失败所有者是会员 authority，公开 class 应是 unavailable；若 caller 已取消/超时，服务应返回 caller context error。二者重试、SLO、告警和责任归属不同。`classifyMembershipTierFactReadError` 会识别 provider 的 context error，但包进 unavailable wrapper，不让它伪装成 caller deadline。
- **追问：** “两者底层都是 `context.DeadlineExceeded`，怎么判断？”
  - **追问回答：** 在 reader 返回后先检查传入 caller `ctx.Err()`；它非 nil 时 caller cancellation 优先。caller 仍 live 时，reader error 中的 deadline 只代表 dependency 内部预算。
- **权衡：** 需要在 I/O 边界多一次 context 检查和错误分类，但能避免把 provider 故障算成用户主动取消；仍无法强制一个不配合 context 的 reader 立即返回。
- **代码 / 测试证据：** [Route 的取消优先顺序](../../../internal/lottery/application/membership_routing.go)、[provider deadline while caller lives 测试](../../../internal/lottery/application/membership_routing_test.go)。
- **官方来源：** Go 官方 [context 包](https://pkg.go.dev/context)定义 `Err()` 只在该 Context 的 Done 关闭后返回取消原因，并要求 Context 沿调用链传播。

## 14. 为什么 Route 在调用依赖前后多次检查 context？

- **直接回答：** pre-cancel 时应让 Clock/reader 零调用；Clock 返回后若 caller 已取消，不应再触达会员 authority；reader 返回后 caller cancellation 应优先于依赖结果；纯 domain 计算后再检查一次，避免把已经观察到的 caller cancellation 包装成成功决定。检查是协作式边界，不是抢占式终止。
- **追问：** “为什么不能只把 ctx 传给 reader，让它自己处理？”
  - **追问回答：** Clock 没有 ctx，reader 也可能在返回时与取消竞态。服务拥有调用生命周期和最终返回契约，不能把归因完全委托给 adapter；但正在阻塞且忽略 ctx 的 reader 仍需 adapter timeout/实现治理。
- **权衡：** 多个检查增加少量分支，却冻结零调用和取消优先语义；不能把它宣传为强制中断 goroutine 或自动释放所有 provider 资源。
- **代码 / 测试证据：** [Route context checkpoints](../../../internal/lottery/application/membership_routing.go)、[pre-cancel/Clock 后取消/reader 后取消/阻塞 reader 测试](../../../internal/lottery/application/membership_routing_test.go)。
- **官方来源：** Go 官方 [Go Concurrency Patterns: Context](https://go.dev/blog/context)说明 Done 是协作取消信号，收到取消后代表该请求工作的函数应尽快退出。

## 15. 为什么先读一次 Clock，再读会员事实？

- **直接回答：** 服务需要一个由自己拥有的 logical as-of 来判断 future/freshness，并把同一时刻写入决定证据。先捕获一次后，reader 延迟不会改变本次阈值；若读事实后再次取时，慢依赖会让同一事实因为调用耗时被额外老化，解释和测试也会漂移。
- **追问：** “这是不是意味着事实读取和 policy 是同一原子快照？”
  - **追问回答：** 不是。一次 Clock 只保证本次计算共享同一逻辑时点；provider 仍可能在读取期间更新，也没有历史 as-of/read-watermark 协议。跨 authority 一致性需要 adapter/版本/事件或事务证据另行设计。
- **权衡：** 单一时点确定、易回放，但不是最接近 reader 返回时刻的墙钟年龄；这是经文档批准的逻辑语义，而不是自然真理。
- **代码 / 测试证据：** [Clock→reader 顺序](../../../internal/lottery/application/membership_routing.go)、[精确调用顺序与一次 Clock 测试](../../../internal/lottery/application/membership_routing_test.go)。
- **官方来源：** Go 官方 [time 包](https://pkg.go.dev/time)说明 `Time`、`Sub`、location 与 monotonic reading 的行为；跨系统快照隔离不由 `time.Time` 或 context 提供。

## 16. freshness 为什么定义为“等于 max age 有效，多 1ns 过期”？

- **直接回答：** 服务使用 `evaluatedAt.Sub(observedAt) > maxFactAge` 判 stale，所以等号属于有效集合；超过一纳秒才 stale。future 则用 `ObservedAt().After(evaluatedAt)`，相等有效，晚一纳秒无效。精确边界避免不同 adapter/调用点分别使用 `>=` 与 `>` 导致同一 snapshot 结论不同。
- **追问：** “纳秒精度是不是过度设计，数据库可能只有毫秒？”
  - **追问回答：** 1ns 是冻结比较运算符的测试工具，不是声称来源真的精确到纳秒。adapter 应保留来源真实精度，不能用本地读取时间续鲜；未来协议精度变化时仍要明确边界。
- **权衡：** 严格比较提供确定契约，但 max age 仍是静态配置，尚未由 provider SLA 或线上误路由数据校准。
- **代码 / 测试证据：** [freshness/future 校验](../../../internal/lottery/application/membership_routing.go)、[exact/+1ns 边界测试](../../../internal/lottery/application/membership_routing_test.go)、[domain future 测试](../../../internal/lottery/domain/membership_routing_test.go)。
- **官方来源：** Go 官方 [time.Time.Sub](https://pkg.go.dev/time#Time.Sub)定义两个时刻之差，且 [Time.After](https://pkg.go.dev/time#Time.After)只在调用者时刻严格晚于参数时返回 true。

## 17. 为什么 source、revision 和 policy revision 要限制长度、空白和不可打印字符？

- **直接回答：** 它们最终可能进入日志、决策证据或外部序列化边界。拒绝空值、前后空白、非法 UTF-8、不可打印字符和超长内容，可以避免同义不同串、日志换行注入、无界内存/存储以及无法稳定比较的 provenance。它们仍是受控 metadata，不是用户自由文本。
- **追问：** “为什么允许可打印 Unicode，而不是只允许 ASCII？”
  - **追问回答：** 当前契约只要求 canonical UTF-8、无两端空白、可打印与长度上限，没有产品证据强制 ASCII。未来若进入跨语言标识协议，应另行定义 normalization/字符集，不能静默收紧已有 revision。
- **权衡：** 当前 validator 保留更多合法标识灵活性，但 Unicode 同形异义仍可能存在；这也是为什么 source/revision 必须由受控 authority/发布流程生成，而非客户端提交。
- **代码 / 测试证据：** [validateMembershipMetadataToken](../../../internal/lottery/domain/membership_fact.go)、[source/revision 边界测试](../../../internal/lottery/domain/membership_fact_test.go)、[policy revision 边界测试](../../../internal/lottery/domain/membership_routing_policy_test.go)。
- **官方来源：** Go [语言规范的 UTF-8 源文本说明](https://go.dev/ref/spec#Source_code_representation)与标准库 [unicode/utf8](https://pkg.go.dev/unicode/utf8)提供字符串有效性工具；具体长度与 canonical 规则来自本项目威胁边界。

## 18. path 为什么只有一跳，为什么不记录未走分支？

- **直接回答：** 本节只有一个具体决策节点和一个 terminal Strategy target，真实执行路径就是一跳。记录未走分支会把“可选拓扑”混进“实际证据”，并增加会员信息披露；创建通用 Node/Edge 则会提前带入 cycle、reachability、depth、operator 等第 28～29 节问题。
- **追问：** “一跳 path 和 branch 字段是否重复？”
  - **追问回答：** 当前有轻微重复，但 path 冻结了未来执行证据的最小形状，decision envelope 则保存 policy/fact/as-of。等多步图出现时可重评序列化；本节不为去重而发明通用图 API。
- **权衡：** 一跳 concrete path 可解释且低复杂度，但不能表示共享子路径、合流、循环或多步预算；这正是下一节的演进证据。
- **代码 / 测试证据：** [MembershipRoutingPathStep 与 Path](../../../internal/lottery/domain/membership_routing.go)、[branch-specific one-hop path 测试](../../../internal/lottery/domain/membership_routing_test.go)、[架构禁止通用 RuleTree](../../../internal/lottery/application/membership_routing_architecture_test.go)。
- **官方来源：** OMG [DMN 1.4](https://www.omg.org/spec/DMN/1.4/PDF)把决策表规则/命中策略显式建模；它提示复杂决策需要明确模型，但不证明本节现在需要通用图。

## 19. 为什么 `Path()` 返回副本？这能证明并发安全吗？

- **直接回答：** decision 内部保存 slice；若直接返回，调用方可改写第一个 step，使 `Confirmed()`、branch 和审计证据互相矛盾。`Path()` 复制 slice，阻止调用方通过返回值修改内部 backing array。它只保护这一处封装，不自动证明 reader、Clock 或整个系统并发安全。
- **追问：** “step 只有值字段，为什么仍会被改？”
  - **追问回答：** 值字段并不改变 slice 共享 backing array 的事实；调用方执行 `path[0] = zero` 会覆盖内部元素，除非返回副本或完全不用 slice。
- **权衡：** 每次读取 path 有一次常量规模复制；当前只有一跳，成本可忽略，换来不可变证据。若未来路径很长，应以基准和 API 需求评估复制策略。
- **代码 / 测试证据：** [Path 防御性复制](../../../internal/lottery/domain/membership_routing.go)、[调用方篡改不影响决定测试](../../../internal/lottery/domain/membership_routing_test.go)、[64 goroutine 一致结果测试](../../../internal/lottery/application/membership_routing_test.go)。
- **官方来源：** Go [语言规范的 Slice types](https://go.dev/ref/spec#Slice_types)说明 slice 描述底层数组的一段并共享存储；[Race Detector](https://go.dev/doc/articles/race_detector)说明竞态检测只覆盖实际执行路径，不是线程安全形式证明。

## 20. 为什么成功 Route 不能直接代表有资格、已授权或已经完成抽奖？

- **直接回答：** Route 只回答“根据会员事实选择哪个 Strategy ID”。Participation 资格回答能否进入流程；Authorization 回答 Principal 能否对资源执行动作；Strategy repository 回答目标是否存在；`WeightedSelector` 才根据权重和随机票据选择 Award；Draw/Result/Delivery 还未建立。任何一个决定都不能替代其他所有者。
- **追问：** “上层最终不是要顺序调用它们吗，为什么不合成一个 service？”
  - **追问回答：** 未来可以有 orchestration，但编排者只协调决定，不能吞并规则所有权。合成一个巨大 service 会把认证、资格、路由、随机、库存和副作用的错误/幂等边界混在一起。
- **权衡：** 分阶段需要显式传递多个不可变结果，代码更多；换来可以独立测试、重试、监控和演进。当前没有上层闭环，因此不能宣称已有端到端门控。
- **代码 / 测试证据：** [Route decision 的类型注释](../../../internal/lottery/domain/membership_routing.go)、[WeightedSelector](../../../internal/lottery/domain/weighted_selector.go)、[上下文所有权架构测试](../../../internal/lottery/application/membership_routing_architecture_test.go)。
- **官方来源：** Go 官方 [Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)鼓励在消费者处定义小接口；这支持按真实依赖边界组合，而不是预造万能 service。

## 21. typed-nil reader/Clock 为什么会绕过普通 nil 判断？

- **直接回答：** Go interface 由动态类型和值组成。一个 `(*membershipFactReaderStub)(nil)` 放入接口后，接口动态类型非空，因此接口本身不等于 nil；调用方法可能 panic。构造器和 `Validate` 通过既有 `dependencyIsNil` 拒绝 nil、typed-nil pointer 和 typed-nil function，并拒绝非正 freshness。
- **追问：** “使用 reflection 检查是否值得？”
  - **追问回答：** 当前 reflection 只在 composition guard，未进入业务求值或通用 fact 系统；它把已知配置陷阱提前变成稳定错误。未来若 DI 工具或构造约束能静态消除 typed nil，可重新评估。
- **权衡：** 运行时校验增加少量构造成本，换取 fail early；它不验证 adapter 连接、凭证、权限或线程安全。
- **代码 / 测试证据：** [service Validate](../../../internal/lottery/application/membership_routing.go)、[nil/typed-nil/partial 配置测试](../../../internal/lottery/application/membership_routing_test.go)。
- **官方来源：** Go 官方 [FAQ：Why is my nil error value not equal to nil?](https://go.dev/doc/faq#nil_error)解释 interface 只有动态类型和值都为空时才等于 nil。

## 22. 为什么 fact 与 error 同时返回时必须让 error 胜出？

- **直接回答：** reader 返回 error 表示无法对同时返回的 snapshot 作可信成功承诺；继续使用 fact 可能把部分解码、陈旧缓存或 provider 合同错误路由成 baseline/premium。服务先检查 caller cancellation，再处理 error；只在 error 为 nil 时验证和消费 fact，所有失败都返回零 decision/path。
- **追问：** “如果 fact 看起来完全合法，能否降级使用？”
  - **追问回答：** 除非 reader contract 明确设计 stale-while-error、来源水位和可接受风险，否则不能猜。当前没有这种产品/SLA 证据，安全默认是错误胜出。
- **权衡：** 丢弃可能可用的 fact 会降低故障时可用性，却避免把未确认值当确定路由；未来缓存策略必须新增明确 ADR、age/revision 和观测指标。
- **代码 / 测试证据：** [Route 的 fact/error 顺序](../../../internal/lottery/application/membership_routing.go)、[fact+error 返回零决定并保留 Cause 测试](../../../internal/lottery/application/membership_routing_test.go)。
- **官方来源：** Go 官方 [Effective Go：Errors](https://go.dev/doc/effective_go#errors)把非 nil error 作为调用失败信号；是否允许携带可用部分结果必须由具体 API 明确约定，本 reader 没有该约定。

## 23. 决策 path 与错误怎样避免隐私泄露和高基数？

- **直接回答：** path 只含 stable rule、selected branch 和 target；decision envelope 再关联受控 policy/fact provenance 与 evaluated-at。它不含 subject ref、原始 tier payload、姓名、订单、金额、凭证、endpoint 或 provider error。普通 metrics 只应使用审核后的低基数 rule/outcome/error class；subject、target、revision、时间和会员详情不能作 label。
- **追问：** “branch 只有两个值，能否直接对外展示 premium/default？”
  - **追问回答：** 低基数不等于非敏感。branch 是会员等级派生信息；当前 trace 仅是进程内决定证据，尚无公开展示、访问控制、保留期或合规评审。
- **权衡：** 数据最小化减少泄露和指标成本，但深度排障需通过受控 Cause、provider revision 查询或受权限保护的审计系统；本节尚未实现后两者。
- **代码 / 测试证据：** [最小 path/decision 字段](../../../internal/lottery/domain/membership_routing.go)、[安全 error rendering](../../../internal/lottery/application/membership_routing_error.go)、[披露边界](../../product/membership-strategy-routing-v1.md)。
- **官方来源：** OpenTelemetry [Handling sensitive data](https://opentelemetry.io/docs/security/handling-sensitive-data/)要求实施者识别敏感数据并进行数据最小化；[Metrics SDK cardinality](https://opentelemetry.io/docs/specs/otel/metrics/sdk/#cardinality-limits)定义 attribute 组合基数及其限制。

## 24. 当前 router 的性能与并发语义是什么？

- **直接回答：** 一次合法 Route 恰好读取一次 Clock、一次 membership fact，然后执行常量规模 switch 和一跳 path 构造；没有隐藏重试、fan-out、缓存、锁或随机源。service 构造后只有 reader/Clock/max age，只读 policy/fact 和请求态都在调用栈；自身没有共享可变请求态，但总体并发安全仍依赖注入 reader/Clock。
- **追问：** “能否声称支持高并发或达到某个 QPS？”
  - **追问回答：** 不能。64 goroutine/race 测试只是并发语义证据，没有真实 provider、网络、容量、benchmark、p99 或 SLO；当前最多陈述调用次数和复杂度。
- **权衡：** 不提前加 cache/singleflight 保持 freshness 与 caller deadline 清楚，却可能对同一主体重复读取；只有生产 profile、撤销语义和 provider SLA 出现后才值得引入缓存。
- **代码 / 测试证据：** [只读 service](../../../internal/lottery/application/membership_routing.go)、[精确调用次数与 64 goroutine 测试](../../../internal/lottery/application/membership_routing_test.go)、[Race 验收要求](../../product/membership-strategy-routing-v1.md)。
- **官方来源：** Go 官方 [Race Detector](https://go.dev/doc/articles/race_detector)强调它只能发现运行时被执行路径上的数据竞争；Go [testing benchmarks](https://pkg.go.dev/testing#hdr-Benchmarks)说明性能结论应由可重复基准支撑。

## 25. 什么时候应该从 concrete router 升级到规则树？

- **直接回答：** 当出现多层条件、第三种以上等级、共享子路径、分支合流、显式 default edge、不可达节点、循环风险、target 引用完整性或持久化版本时，继续在 switch/next-index 上叠逻辑会隐藏拓扑。第 28 节才基于这些真实词汇建立最小 root/node/edge/default/target schema 和发布前 cycle/depth/reachability 校验。
- **追问：** “既然下一节肯定做树，为什么本节不一次完成？”
  - **追问回答：** 本节的一跳分支只证明“线性 chain 不够”，还没有证明存储 schema、通用 operator、图发布和执行预算。分节能先冻结正确领域语言，再让后续结构由实际不变量驱动，而不是从技术类图倒推业务。
- **权衡：** 现在保留 concrete router 意味着后续有一次有证据的模型升级；提前建树表面少重构，却会提前承担 cycle、migration、兼容和错误恢复成本。
- **代码 / 测试证据：** [concrete switch 与 one-hop path](../../../internal/lottery/domain/membership_routing.go)、[禁止 RuleTree/RuleNode/RuleEdge 的架构测试](../../../internal/lottery/application/membership_routing_architecture_test.go)、[后续停止线](../../product/membership-strategy-routing-v1.md)。
- **官方来源：** Martin Fowler [Decision Table](https://martinfowler.com/dslCatalog/decisionTable.html)说明复杂条件组合可显式表格化；OMG [DMN 1.4](https://www.omg.org/spec/DMN/1.4/PDF)进一步展示命中策略属于正式决策模型。它们用于识别触发器，不意味着必须采用某一产品。

## 26. 什么时候应该升级成规则引擎，而不只是规则树？

- **直接回答：** 规则树解决结构化表示还不等于引擎。只有出现运营无代码编辑、审批/灰度/回滚、声明式表达式、类型检查、冲突消解、模拟、bundle 分发、执行预算、未知 operator、审计和独立发布故障域时，才有引擎证据。第 29 节也只执行已验证、已发布图，不应立即造跨 Eligibility/Authorization/Inventory 的万能 engine。
- **追问：** “把规则 JSON 放数据库再循环执行，算不算引擎？”
  - **追问回答：** 只能算数据驱动执行雏形；缺少 schema version、合法性、发布、组合算法、安全沙箱、资源上限、回放和运维协议时，数据库并没有解决核心问题。
- **权衡：** 代码内 concrete policy 需要研发发布，但类型安全、故障边界小；引擎提高非研发变更效率，同时引入一套需要独立治理的语言和运行平台。
- **代码 / 测试证据：** [当前具体 policy](../../../internal/lottery/domain/membership_routing_policy.go)、[架构禁止 Engine/DSL/generic/string-any bag](../../../internal/lottery/application/membership_routing_architecture_test.go)、[ADR 重新评估触发器](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)。
- **官方来源：** OPA 官方 [Policy Language](https://www.openpolicyagent.org/docs/policy-language)、[Integration](https://www.openpolicyagent.org/docs/integration)和 [Operations](https://www.openpolicyagent.org/docs/operations)展示真实策略系统涉及语言、数据、分发和运行边界，而不只是一个 for-loop；本项目没有采用 OPA。
- **题型来源（非技术依据）：** 牛客用户的[字节跳动面经](https://www.nowcoder.com/discuss/724635714276601856)自述出现“责任链和规则引擎的区别”，真实性未独立核验。

## 27. 为什么本节没有数据库、Redis、HTTP 或前端？

- **直接回答：** 本节要验证的是事实词汇、所有权、两个出口、显式 default、单一 as-of、错误/取消和一跳 path。数据库会提前要求 tree schema/version/reference integrity，HTTP 会要求可信身份与错误映射，Redis 会要求 freshness/撤销/一致性，前端会诱导客户端自报 tier/target；这些都不是当前切片已证明的问题。
- **追问：** “没有 API 是否只是单元测试 demo？”
  - **追问回答：** 它是可执行的领域/application 内核，不是线上纵向闭环。真实项目可以分阶段冻结正确契约，但必须明确后续 adapter、contract、integration 和 E2E，不能把接口存在冒充用户已经受保护。
- **权衡：** 小切片让 Git 演进和回归面清楚，代价是本节不能展示浏览器效果或真实 provider SLA；下一节也不应跳过集成债务说明。
- **代码 / 测试证据：** [application port/service](../../../internal/lottery/application/membership_routing.go)、[架构 import/engine 停止线](../../../internal/lottery/application/membership_routing_architecture_test.go)、[ADR 第 27 节停止线](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)。
- **官方来源：** Go 官方 [testing](https://pkg.go.dev/testing)说明单测/基准/fuzz 各自的执行模型；[Race Detector](https://go.dev/doc/articles/race_detector)也明确动态检测只覆盖实际执行路径，因此内核测试不能替代真实 adapter 与 E2E。

## 28. 你怎样测试本节，而不是只测两个 happy path？

- **直接回答：** domain 覆盖 standard/premium 两出口、literal code、target 收敛仍保留 branch、path 副本、zero/future 零决定、UTC/重复确定性、policy/source/revision 边界和 unsupported-tier fuzz；application 覆盖 Clock→reader 顺序、一次调用、freshness exact/+1ns、subject mismatch、fact+error、provider deadline、payload contract error、pre-cancel/阻塞取消、typed nil、并发一致性；architecture test 禁止跨上下文 import、generic Rule/Tree/Engine/DSL 和 `map[string]any`。
- **追问：** “fuzz seed 通过是否证明所有输入安全？”
  - **追问回答：** 不能。普通 `go test` 只跑 seed corpus；限时 fuzz 也不是穷举。它补充人工边界测试，最终仍需 race、静态检查、集成/E2E 和真实故障演练。
- **权衡：** 精确 call-count、literal code 和 1ns 测试会冻结重要契约，也会使无语义变化的重构需要更新测试；应只锁对调用方/架构可观察的行为。
- **代码 / 测试证据：** [domain routing tests](../../../internal/lottery/domain/membership_routing_test.go)、[fact/policy tests](../../../internal/lottery/domain/membership_fact_test.go)、[application tests](../../../internal/lottery/application/membership_routing_test.go)、[error boundary tests](../../../internal/lottery/application/membership_routing_error_test.go)、[architecture test](../../../internal/lottery/application/membership_routing_architecture_test.go)。
- **官方来源：** Go 官方 [Fuzzing](https://go.dev/doc/security/fuzz/)要求 fuzz target 快速、确定且无跨调用状态；[Race Detector](https://go.dev/doc/articles/race_detector)界定动态竞态证据的覆盖边界。

## 29. 为什么会员等级不能顺便当角色或权限？

- **直接回答：** tier 是外部 authority 的业务分层事实，只用于 Lottery 路由；授权需要可信 Principal、资源、动作、scope、角色/策略和服务端强制执行。premium 不等于 admin，Route 成功也不表示能读取他人会员信息或调用受限操作。权限系统将在第 31～35 节按自己的威胁边界建立。
- **追问：** “前端看到 premium 就显示管理员菜单是否可以？”
  - **追问回答：** 不可以把显示裁剪当授权，更不能用 tier 推导 admin。未来前端只消费服务端授权后的能力投影，服务端每个资源动作仍需强制检查；当前连可信 session 都没有。
- **权衡：** 分开会员与权限会增加身份映射和能力投影，但避免业务营销分层升级成安全凭据；这是必须的安全边界，不是重复建模。
- **代码 / 测试证据：** [MembershipSubjectRef 非身份注释](../../../internal/lottery/domain/membership_fact.go)、[不 import Governance/Participation 的架构测试](../../../internal/lottery/application/membership_routing_architecture_test.go)、[后续权限停止线](../../product/membership-strategy-routing-v1.md)。
- **官方来源：** OWASP 官方 [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)强调默认拒绝、每次请求校验权限和服务端访问控制；客户端 UI 裁剪不能替代授权执行。

## 30. 面试时怎样准确描述本节完成度？

- **直接回答：** 可以说已用一个具体会员路由切片证明线性 continue/reject chain 的边界；实现了 Lottery-owned policy、consumer-owned fact port、两个确定出口、显式 standard default、unknown 失败关闭、一次 Clock、freshness、取消优先、安全 Cause 和不可变 one-hop path，并有 domain/application/architecture 测试。不能说已接入会员中心、已在线保护 API、已加载/发布 Strategy、已完成抽奖、已认证授权、已有规则树/引擎、OTel、SLO 或浏览器 E2E。
- **追问：** “简历上怎样说才有亮点又不夸大？”
  - **追问回答：** 重点讲“用真实多出口需求识别模式边界”和可验证的不变量，而不是写“自研高性能规则引擎”。可量化的是每次合法路由一次 Clock/一次 fact read、错误零 decision、一跳 path、精确时间边界和 64 goroutine 测试；不能编造 QPS、命中率或线上收益。
- **权衡：** 诚实的分层完成度不如“大而全平台”听起来华丽，却能经得住代码、Git、测试和追问；后续章节完成后再逐步升级简历表述。
- **代码 / 测试证据：** [课程第 27 节](../../course/part-04/lesson-27-responsibility-chain-limits.md)、[会员路由基线](../../product/membership-strategy-routing-v1.md)、[ADR-0023](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)、[完整本节测试集合](../../../internal/lottery/application/membership_routing_test.go)。
- **官方来源：** Go 官方 [Race Detector](https://go.dev/doc/articles/race_detector)明确工具只能发现被执行路径中的竞争，提醒面试表述必须区分“测试证据”与“生产证明”；OpenTelemetry 官方资料同样说明设计字段不等于已部署观测系统。

## 31. policy revision 为什么不自动等于内容哈希？`Confirmed()` 又到底确认了什么？

- **直接回答：** 当前 `MembershipRoutingPolicyRevision` 是调用方提供并通过格式校验的业务版本标识，policy 仍是一次完整、显式传入的内存 snapshot；代码没有 canonical serialization、内容哈希、repository 唯一约束或 publisher，因此不能宣称“同 revision 必然同内容”。`Confirmed()` 只确认返回决定包内部自洽：非零 target、固定 rule、合法 branch→reason 对应、非空 policy/fact provenance、有效 evaluated-at、恰好一跳 path，且 path 的 rule/branch/target 与 envelope 一致。
- **追问：** “为什么不在构造器里把两个 target 哈希后自动生成 revision？”
  - **追问回答：** 内容寻址先要冻结 canonical bytes、schema version、字段顺序/编码、哈希算法与迁移规则；哈希相同只能说明被哈希表示相同，不能证明它已审批、已发布、target 存在或 Activity 正在引用它。第 28 节 repository 才能绑定 version 与不可变规则内容并做引用完整性，第 30 节 publisher 才能绑定已发布 Activity/Strategy snapshot。
- **权衡：** 人工/上层提供 revision 简单，足以关联当前完整 snapshot 的一次确定求值，但依赖未来存储/发布层阻止 revision 重用；内容哈希可增强去重和完整性，却会把序列化与兼容协议变成长期公共契约，不能在一跳内核中顺手决定。
- **代码 / 测试证据：** [policy revision 与完整 targets](../../../internal/lottery/domain/membership_routing_policy.go)、[`Confirmed()` 的内部一致性检查](../../../internal/lottery/domain/membership_routing.go)、[literal/branch/path 与 target 收敛测试](../../../internal/lottery/domain/membership_routing_test.go)、[第 28/30 节停止线](../../product/membership-strategy-routing-v1.md)。
- **官方来源：** IETF [RFC 8785：JSON Canonicalization Scheme](https://www.rfc-editor.org/rfc/rfc8785)说明对结构化内容做可重复哈希需要先定义不变的 canonical representation；OMG [DMN 1.4](https://www.omg.org/spec/DMN/1.4/PDF)说明决策模型/命中语义本身仍需明确建模，digest 不能替代发布语义。

## 不能夸大的结论

可以准确说：

- 已用 `premium_override` 与 `baseline_default` 两个真实成功出口证明 Participation 线性资格链不能承担 Lottery 路由；
- 已把外部会员 fact 所有权与 Lottery Strategy 路由决定所有权分开；
- 已将 default 限定为 confirmed standard 的显式产品边，unknown/unsupported/依赖错误不会自动降级；
- 已保证相同 policy/fact/evaluated-at 的 rule、branch、reason、target 与 path 确定；
- 已实现一次 Clock、一次 fact read、UTC canonicalization、future/freshness 纳秒边界和 caller cancellation 优先；
- 已让公共 `errors.Is` 只看到一个稳定读取 class，原始 provider cause 仅通过显式 `Cause()` 提供；
- 已形成一跳、最小、可防御复制的 route path，并明确其不是 OTel trace 或持久化审计；
- 已用 domain/application/error/architecture 测试覆盖 happy path、反例、fuzz seed、typed nil、并发与禁止提前引擎化；最终命令状态以 QA 为准。

不能说：

- 已接入真实会员中心、用户目录、数据库、Redis、RabbitMQ、HTTP route、runtime config 或前端；
- `MembershipSubjectRef` 是 Principal、登录证明、租户/用户全局 ID 或授权凭据；
- premium 是管理员角色，会员路由已经执行 RBAC/ABAC 或前端权限裁剪；
- Route 已验证 target Strategy 存在、已发布、具有业务 version，或已调用 repository/`WeightedSelector`；
- 已创建 Activity、Participation、Draw、Result、次数、库存、发奖或消息闭环；
- 一次 evaluated-at 提供了跨 authority 原子快照、历史 as-of 或消除了 TOCTOU；
- 一跳 path 已经是规则树、规则引擎、DMN/XACML/OPA、审计日志或分布式 trace；
- 单元测试、64 goroutine 或 `-race` 已证明真实 provider 容量、QPS、p99、SLO、安全性或生产 E2E。

## 复习清单

- [ ] 能在 60 秒内说清第 26 节 continue/reject chain 与第 27 节 multi-target Route 的差异；
- [ ] 能区分项目的 Lottery Strategy 领域术语和 Strategy 设计模式；
- [ ] 能解释 standard、baseline_default、unknown 与技术 failure 的不同语义；
- [ ] 能说明为什么 unknown/unsupported/provider error 绝不能走 default；
- [ ] 能说明会员 authority、Lottery、Participation、Governance 和 Strategy selector 的决定所有权；
- [ ] 能说明 `MembershipSubjectRef` 为什么不是 Principal 或权限；
- [ ] 能解释封闭 tier 的收益、成本与未来升级触发器；
- [ ] 能证明确定性依赖 policy/fact/as-of 三个输入，而不是“用户永远同一路由”；
- [ ] 能引用 Go map 顺序未定义，且说明排序也不能替代 default 的产品语义；
- [ ] 能解释 target 收敛时为什么仍保留 branch/path；
- [ ] 能区分 rule、branch、reason、target、policy revision 和 fact revision；
- [ ] 能解释 `Cause()`、`Unwrap()`、`errors.Is` 与 provider 错误脱敏的权衡；
- [ ] 能区分 caller deadline 与 provider timeout；
- [ ] 能画出 validate → pre-cancel → Clock → reader → cancel → fact/freshness → domain route 的顺序；
- [ ] 能说明 exact max age/+1ns 与 exact as-of/future +1ns 边界；
- [ ] 能说明 path 为什么只有一跳、为何返回副本、为何不是审计/OTel trace；
- [ ] 能说明 branch 低基数也可能敏感，revision/subject/target 为什么不能做普通 metric label；
- [ ] 能解释 typed-nil interface 与 fail-early composition guard；
- [ ] 能说明 fact+error 为什么 error 胜出且返回零 decision；
- [ ] 能说明何时上规则树、何时才有规则引擎证据；
- [ ] 能明确本节没有 adapter/API/UI/权限/E2E，也没有 Strategy 加载和抽奖闭环；
- [ ] 能把牛客面经只当追问形态，不当技术依据或公司官方题库。
