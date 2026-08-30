# 第 26 节设计手记：让第二条真实规则证明责任链，而不是让模式替业务找理由

> 第 25 节只有一条新用户规则时，任何 `Rule`、`Chain` 或 `Engine` 都只是未经业务事实证明的技术形状。第 26 节真正发生的变化，是 Participation 出现了第二条真实、只读、时间敏感且故障域不同的前置条件：风险事实提供方给出最小 screening verdict，Participation 决定该 verdict 能否进入当前参与场景。
>
> 本节据此得到一个刻意受限的结论：**使用固定的“新用户资格 → 风险准入”顺序，在一次服务端逻辑时刻上逐项求值；只有确定通过才能继续，确定拒绝、技术失败或 caller cancellation 都立即停止。** 这是一条 Participation 专用的线性 gate chain，不是通用规则平台、授权系统、工作流引擎或 Lottery 编排器。
>
> 本手记记录 2026-08-30 的第 26 节实现时间切片。规范性边界以 [ADR-0022](../../decisions/ADR-0022-participation-prerequisite-chain.md) 和 [Participation 前置资格链基线 v1](../../product/participation-prerequisite-chain-v1.md) 为准；最终测试次数、命令和未验项以[第 26 节 QA](../../qa/lessons/lesson-26.md)为准。本文只陈述 Participation domain/application 内核，不声称真实 adapter、HTTP、Activity、Lottery 或前端已经接入。

---

## 1. 先把题目还原：责任链不是目标，可信资格才是目标

### 1.1 此刻系统已经知道什么

第 25 节已经把“是不是新用户”从客户端 bool 推进为一个可复算的服务端决定：

```text
authoritative registration snapshot
  + versioned registration-cutoff policy
  + controlled evaluated-at
  -> confirmed eligible / confirmed ineligible
  or zero decision + technical error
```

这个切片证明了单条规则的事实权威、时间边界、freshness、错误分类和决定所有权，却仍然没有证明“多规则怎样组合”。如果这时直接声明：

```go
type Rule interface {
    Evaluate(map[string]any) (bool, error)
}
```

代码虽然可以编译，却回答不了下面这些业务问题：

1. 第二条规则是什么，它是否真的属于 Participation；
2. 两条规则是否需要同一个逻辑时刻；
3. 哪条先执行，顺序改变是否会改变公开原因、成本或隐私暴露；
4. 某一步通过究竟表示“最终允许”还是“可以继续”；
5. 某一步事实未知时是拒绝、跳过、重试还是失败关闭；
6. 短路是否真的阻止后序事实读取，而不只是最后返回 `false`；
7. 轨迹记录哪些信息，哪些字段不能进入日志、指标或响应；
8. 规则以后出现分支、合流和不同终点时，线性结构从哪里开始失真。

因此，本节的第一性问题不是“如何实现责任链模式”，而是：

> 两条已被业务需求证明的 Participation 前置资格，怎样在不扩大事实所有权、不泄露风险材料、不伪造技术失败语义的前提下，形成确定、可解释、可短路且仍然容易撤销的组合决定？

### 1.2 本节的最小业务公式

用 `N` 表示新用户规则的确定结果，用 `R` 表示风险准入规则的确定结果，最终资格 `E` 只有在二者都确定通过时成立：

```text
E = confirmed(N = eligible) AND confirmed(R = eligible)
```

这个公式中最容易被忽略的是 `confirmed`。`N` 或 `R` 的事实缺失、陈旧、损坏、来自未来、主体不符或依赖不可用，都不是 `false`，而是无法形成业务真值。于是完整状态不是普通布尔代数，而是：

| 单步状态 | 业务含义 | 是否继续 | 是否形成最终业务决定 |
| --- | --- | ---: | ---: |
| confirmed eligible | 当前必经条件已满足 | 是 | 仅最后一步后形成最终 eligible |
| confirmed ineligible | 当前必经条件明确不满足 | 否 | 是，最终 ineligible |
| technical indeterminate | 事实或执行条件不足 | 否 | 否，返回零决定 + error |
| caller canceled/deadline | 调用已失去继续价值 | 否 | 否，返回零决定 + context error |

责任链只是实现这个状态机的一种当前合适形状。若把模式本身当目标，就很容易把 `technical indeterminate` 压成拒绝，把单步通过误写成最终通过，或者为展示“可扩展”而允许任意上下文和任意顺序。

### 1.3 当前代码已经形成的边界

第 26 节的实现切片包括：

- `RiskScreeningDisposition` 与最小 `RiskScreeningFactSnapshot`；
- 固定 rule code `participation.risk.screening_admission`；
- 只有 revision 的具体 `RiskAdmissionPolicy`，v1 映射保持固定；
- 纯领域函数 `EvaluateRiskAdmission` 与不可变 `RiskAdmissionDecision`；
- Participation-owned `RiskScreeningFactReader`；
- 风险事实的主体、future、freshness 和 provider error 校验；
- 包内不可由 transport 伪造的 `evaluationInstant`；
- 固定两节点的 `EligibilityPrerequisiteChain`；
- 组合级 `RuleSetRevision`、`PrerequisiteEvaluation` 和有序 `EligibilityTraceStep`；
- 技术失败时零 aggregate、确定拒绝时保留实际执行前缀、全部通过时返回两步 trace；
- `Steps()` 防御性复制和 typed-nil 配置校验；
- 注册/风险读取 error 的单一公开 class 与显式受信 `Cause()` 通道；
- 禁止通用 Rule/Tree/DSL、跨上下文 import 和无类型事实袋的架构停止线。

实现入口：

- [风险事实](../../../internal/participation/domain/risk_screening_fact.go)
- [风险准入政策](../../../internal/participation/domain/risk_admission_policy.go)
- [风险准入 evaluator](../../../internal/participation/domain/risk_admission.go)
- [事实读取端口](../../../internal/participation/application/ports.go)
- [唯一逻辑时刻 token](../../../internal/participation/application/evaluation_instant.go)
- [风险事实应用编排](../../../internal/participation/application/risk_admission.go)
- [固定前置资格链](../../../internal/participation/application/prerequisite_chain.go)
- [安全错误边界](../../../internal/participation/application/errors.go)
- [架构停止线](../../../internal/participation/application/architecture_test.go)

### 1.4 当前明确没有实现什么

本节没有实现或证明：

- 真实用户目录 adapter 或风险系统 adapter；
- provider credential、mTLS、签名、网络 ACL、重试、熔断或生产 timeout；
- Activity 聚合、活动发布、规则集持久化或运营配置入口；
- 真实 Principal、session、authentication、authorization、RBAC 或 tenant scope；
- Participation request、剩余次数账户、预占、流水、Draw 或 Result；
- Lottery service、现有 ephemeral route 或 WeightedSelector 的资格接入；
- HTTP DTO、status、header、公开 error code 或用户提示文案；
- React 页面、导航、权限裁剪或浏览器端到端验收；
- Migration、MySQL 表、Redis key、消息、Outbox 或 Compose 新服务；
- 规则树、数据库动态规则、决策引擎或多出口会员路由；
- 持久化决策审计、OpenTelemetry span、生产 metrics 或告警；
- 跨事实 authority 的原子历史快照；
- 资格通过到次数消费/抽奖之间的 TOCTOU 闭环。

这些停止线不是文档上的免责声明，而是本节架构诚实性的组成部分。只要没有可信身份和正式 Participation use case，把 chain 接进现有匿名 Lottery demo 就会让“代码走过了资格判断”冒充“真实用户已被安全门控”。

## 2. 为什么第二条规则选择风险，而不是为了凑数选择别的概念

### 2.1 选择标准

第二条规则必须同时满足五个条件：

1. 已在第 23 节的业务规则需求中登记，不是课程标题临时创造；
2. 确实属于 Participation 的参与资格，而不是别的上下文的决定；
3. 能在当前无 Activity、无正式参与账户的阶段独立建模；
4. 与新用户规则存在足够真实的共同组合语义，能够证明顺序与短路；
5. 不要求提前解决未来章节的事务、身份或持久化问题。

风险 screening 恰好满足：风险提供方拥有最小 verdict，Participation 拥有“这个 verdict 对当前参与场景是否可准入”的衍生决定；它是只读前置事实，但比注册事实更敏感、可能更昂贵、也有独立故障域，因此“先低敏筛选、再风险读取”的短路价值可以被真实验证。

### 2.2 为什么不是剩余次数或 quota

“剩余次数大于零”看起来是最自然的第二个 bool gate，但次数不是稳定属性，而是会被并发消费的业务状态。以下实现存在经典检查后再使用问题：

```text
request A: read remaining = 1 -> eligible
request B: read remaining = 1 -> eligible
request A: consume one
request B: consume one  // 已经超用
```

在没有账户、流水、版本、事务、原子扣减或预占协议时，`remaining > 0` 只能展示语法，不能证明业务正确。把它提前塞入责任链还会制造错误心智模型：仿佛 gate 返回 `eligible` 就已经获得了一次可消费权利。

真正的次数资格需要与第 39～45 节的参与账户、并发承诺、幂等和消费事实一起演进。它将迫使系统回答：

- 读取和消费是否在同一事务；
- 重试是否重复扣减；
- 失败如何释放预占；
- 多实例下谁序列化竞争；
- 资格判断结果可保持多久。

因此，本节拒绝 quota 不是因为它不重要，而是因为重要到不能用一个无状态 bool 冒充完成。

### 2.3 为什么不是 Activity 时间窗

Activity 是否已发布、是否在参与时间窗内、是否暂停或终止，属于 Marketing/Activity 生命周期的权威决定。第 30 节之前，当前仓库没有足够的 Activity 聚合、业务版本和发布事实。

如果 Participation 此时自行接受 `startAt/endAt`，会出现两种所有权错误：

1. Participation 复制 Marketing 的生命周期事实并可能形成第二事实源；
2. 为了共享 clock，把两个“时间判断”误认为同一业务抽象。

技术形状相似不代表事实所有权相同。Activity 门控以后可以作为上游上下文提供的已确认入口条件，或由正式用例协调，但不能为了让 chain 多一个节点而提前发明。

### 2.4 为什么不是权限或管理员角色

Eligibility 与 Authorization 回答的是不同问题：

| 问题 | 例子 | 必需前提 | 所有者 |
| --- | --- | --- | --- |
| Authentication | caller 是谁 | credential/session/identity proof | 身份能力 |
| Authorization | caller 能否管理某个 Activity | Principal、resource、action、scope、policy | 公共访问控制能力 |
| Participation eligibility | 已识别主体是否满足本场景参与前置条件 | 业务事实、规则版本、评估时刻 | Participation |

当前 `ParticipantRef` 只是一个外部主体的查找引用，不证明 caller 就是这个主体，也不携带 role、tenant 或 scope。若把 `role == admin` 作为第二条资格规则，既会把公共权限系统降格成业务 gate，又会允许未来调用者把客户端角色塞进同一事实袋。

认证与公共权限按第 31～35 节独立演进，届时服务端必须在进入 Participation 用例前建立可信 Principal 并强制授权；前端裁剪只是体验层投影，不能取代服务端执行。本节不提前声明这些能力已存在。

### 2.5 为什么不是 Lottery 的 Strategy/Award 可用性

候选集合、权重和 `no_reward` 属于 Lottery 的终端选择语义，不属于 Participation 是否具备参与资格。把它放入同一 chain 会造成阶段倒置：

```text
错误：eligibility chain -> read Strategy -> random select -> maybe reject
正确边界：confirmed participation prerequisites -> later formal participation/draw -> Lottery select
```

Lottery 读取和随机选择也可能有副作用或不可重复材料。前置资格链必须保持只读，否则中途拒绝、失败或取消就会引出幂等、补偿和结果恢复问题。本节因此不 import Lottery，也不调用现有 selector。

## 3. 先划所有权，再设计字段

### 3.1 四类所有权不能混在一起

本节涉及的概念可以分成四层：

| 层次 | 例子 | 谁拥有 | 本节如何使用 |
| --- | --- | --- | --- |
| 原始/权威事实 | registered-at、risk screening disposition | 外部用户目录、受控风险提供方 | 通过 consumer-owned port 读取最小不可变快照 |
| Participation policy | cutoff、风险准入映射、policy revision | Participation | 具体 value object，每次显式传入 |
| 组合政策 | new-user 后 risk、ruleset revision | Participation | 固定代码计划 + 独立 RuleSetRevision |
| 最终决定证据 | outcome、reason、executed steps、evaluated-at | Participation | 只在确定结果时形成 aggregate |

这四类 revision 不能互相冒充：

```text
RuleCode          != PolicyRevision
PolicyRevision    != FactRevision
RuleSetRevision   != application Git SHA
RuleSetRevision   != Activity version
FactRevision      != database updated_at 的通用替代品
```

分离它们的价值是让重放与解释有明确问题：我们可以问“同一规则集为什么在两个时刻不同”，再检查事实 revision；也可以问“事实相同为什么决定不同”，再检查 policy/ruleset revision。若只记录一个 `version`，任何变化都会变成猜测。

### 3.2 风险提供方拥有 verdict，Participation 拥有准入含义

`RiskScreeningFactSnapshot` 接受 `passed/blocked` disposition、source-owned `assessed_at`、source 与 revision。它不接受：

- 设备指纹；
- IP、地理位置或账号画像；
- 模型特征和分数；
- provider 阈值或策略 DSL；
- 原始响应 payload；
- Participation 的 `eligible` 结论；
- 面向用户的拒绝文案。

这种最小契约避免 GrowthOS 为一次资格判断复制整套风控模型。Participation 的具体 `RiskAdmissionPolicy` 再把 `passed` 映射为本节点 `eligible`、把 `blocked` 映射为本节点 `ineligible`。即使 v1 映射看似一一对应，仍要保留事实与决定的边界，因为未来可能出现：

- 不同参与场景对同一 source verdict 采用不同准入政策；
- provider verdict 被撤回或更正；
- source revision 与 Participation policy revision 独立演进；
- 对外只能返回通用不可参与提示，而内部保留稳定风险 reason。

本节没有把风险 provider 变成 Participation，也没有让 Participation 推断 provider 的原始模型。

### 3.3 consumer-owned port 只能约束 shape，不能自动证明 authority

`RiskScreeningFactReader` 定义在 Participation application，因为消费方最清楚自己只需要什么。窄接口带来三个好处：

1. adapter 不必把完整外部 SDK 类型渗透进领域；
2. 单元测试可以精确控制 fact、error、call count 和 cancellation；
3. 未来替换 transport 不改变 domain policy。

但 interface 不能证明实现者可信。任何对象都可以实现这个方法并返回格式合法的 source string。真正的 authority 仍需要未来 composition root 和 adapter 证明：

- 注入的是哪一个 provider；
- credential 与网络边界是什么；
- source revision 如何生成和验证；
- provider 是否能返回 as-of 时已存在的快照；
- clock skew、缓存和重试会怎样影响 `assessed_at`；
- provider 删除、更正或撤销怎样传播。

因此，当前代码可以严谨地声称“定义了可信 adapter 必须满足的 consumer contract”，不能声称“已经连接可信风险系统”。

### 3.4 `ParticipantRef` 仍然不是 Principal

注册快照与风险快照都携带非零 `ParticipantRef`，application 会核对它与请求 reference 相同。这能阻止 adapter 把 A 的事实误用于 B，却不能阻止 caller 直接请求 B。

```text
subject consistency check != caller-subject binding
```

后一项需要真实会话、Principal 和授权。没有这些前提时不开放 route，是比“先接一个 demo header，以后再补安全”更诚实的实现选择。

## 4. 为什么整条链只有一个 logical as-of

### 4.1 两个节点各读一次 `Now()` 的问题

假设新用户事实的 freshness 边界和风险事实的 freshness 边界都在当前请求附近：

```text
t0: 新用户节点读 Now()，注册快照刚好有效
t1: 网络等待
t2: 风险节点读 Now()，风险快照刚好过期
```

这次结果本身可能是安全的，但“同一个资格评估使用了哪个时刻”没有答案。重放时如果用 `t0`，风险可能通过；用 `t2`，注册事实的年龄又不同。更糟的是节点顺序或机器调度会悄悄改变边界结果。

LRR-008 要求在相同规则版本、相同事实快照和相同评估时刻下，终端随机票据之外的前置决定可复算。因此组合器必须拥有一个逻辑时刻，而不是把 wall clock 读取散落到每个节点。

### 4.2 为什么在任何事实读取前捕获

当前 chain 的顺序是：

```text
validate request/config
  -> check caller context
  -> capture controlled server instant once
  -> lazy read registration fact
  -> evaluate registration at that instant
  -> only if eligible: lazy read risk fact
  -> evaluate risk at the same instant
```

捕获时刻放在所有读取之前，是在两个目标之间做出的明确选择：

- 若先读取全部事实再捕获时间，可以让每份返回快照都自然早于 evaluated-at；
- 但这样必须预取风险事实，失去新用户拒绝后的隐私、成本和故障隔离收益。

本节选择 short-circuit 的真实价值，所以 logical as-of 位于读取前。代价同样必须写清：后序 provider 若只能返回“读取完成时刚刚产生”的新快照，其 `assessed_at` 可能晚于 as-of，本次评估必须失败关闭，并把新快照留到下一次评估，而不能偷偷把整条链的时间向后移动。

### 4.3 `evaluationInstant` 为什么不直接导出 `EvaluateAt(time.Time)`

如果公开方法允许 transport 或任意调用者传入裸 `time.Time`，调用者可能选择一个旧时刻，使已经陈旧的 fact 在差值计算中重新看起来新鲜。当前实现使用 application 包内的 `evaluationInstant` token：

- 只有受控 `Clock` 能创建；
- 时间统一转为 canonical UTC 并去除 monotonic 部分；
- 零值或非 canonical token 被拒绝；
- domain evaluator 仍保持纯函数，可以接收确定时刻；
- transport 无法通过公共 API 选择历史 as-of。

这不是密码学能力边界；Go 包内代码仍可构造未导出值。但它把误用面限制在 application 实现内部，阻止未来 handler 把请求时间直接透传为 freshness 基准。

### 4.4 为什么保留第 25 节 standalone service 的时序

独立 `NewUserEligibilityService.Evaluate` 在成功读取注册事实后捕获一次 clock。第 26 节没有为了复用 chain 而悄悄改变这个既有契约；它抽出包内 helper，使 standalone 路径继续保持原行为，组合路径则使用 chain-owned instant。

这揭示一个重要设计原则：抽象公共逻辑不等于抹平用例时序。两个入口可以复用 validation/evaluator，却仍由各自 use case 决定“评估会话从何时开始”。若未来只保留组合入口或需要历史重放，应通过新 ADR 和兼容性测试显式改变，而不是靠重构顺手改变 freshness 结果。

### 4.5 单一 as-of 不等于跨系统原子快照

注册目录和风险 provider 是两个 authority。一个共同 `evaluated_at` 只能表达“我们希望基于这个逻辑时刻解释事实”，不能保证两个读取来自同一事务快照：

```text
as-of = t0
read registration snapshot that existed at t0
external account state changes at t1
read risk snapshot that provider says existed at t0
```

若 provider 不支持历史/版本化读取，它可能只能返回当前值。当前 port 没有 as-of 参数，因此未来 adapter 必须证明返回的 immutable snapshot 在 `t0` 已存在，或明确失败；本节尚未验证这个生产契约。

如果未来业务需要法律/财务级历史证明，可能需要：

- source 提供 effective interval 或 as-of query；
- 入口先取得一个跨服务一致性 token；
- 事实事件在本地形成可审计投影；
- 决策时持久化所用事实版本；
- 对迟到、更正和撤销定义重决策协议。

这些都是重决策触发器，不应由当前一个 `time.Time` 声称已经解决。

## 5. 为什么顺序是业务版本，而不是 for-loop 偶然顺序

### 5.1 固定顺序的推导

当前 v1 顺序：

1. `participation.new_user.registered_on_or_after`；
2. `participation.risk.screening_admission`。

排序依据不是“新用户代码先写”，而是：

- 注册事实通常低敏，风险事实通常更敏感；
- 注册读取预期更便宜，风险服务可能有更高延迟或成本；
- 对非新用户已经有充分拒绝理由，无需继续访问风险 authority；
- 固定第一个拒绝原因可以避免外部依赖时序让结果漂移；
- 更少风险读取降低不必要的数据处理与依赖故障传播。

因此，顺序会改变可见 reason、访问哪些 authority、尾延迟、成本和隐私面。它必须由独立 `RuleSetRevision` 标识，而不能依赖 map 遍历、SQL 行顺序、配置数组的偶然排列或源码位置。

### 5.2 短路必须用“后序零调用”证明

下面的代码不是完整短路：

```text
read registration
read risk
if registration rejected { return rejected }
```

虽然最终 reason 与顺序一致，风险读取已经发生。真正的短路证据至少包括：

- 新用户 `ineligible` 时 risk reader 调用数为零；
- 注册 fact not found/stale/invalid/unavailable 时 risk reader 调用数为零；
- caller 在第一步前或第一步后取消时 risk reader 调用数为零；
- 只有新用户 confirmed eligible 才能进入风险读取；
- risk blocked 后不产生任何后续步骤或 Lottery 调用。

当前代码使用 lazy closure 构造固定 plan，for-loop 只在当前 step 返回 confirmed eligible 后继续。它没有预取所有 facts，也没有并行启动 goroutine。

### 5.3 单步 `eligible` 为什么不是最终 `eligible`

新用户规则通过只代表：

```text
registration prerequisite satisfied -> continue
```

它不代表风险已经通过，更不代表权限、次数、Activity、库存和 Lottery 已经满足。组合器只有执行完全部必经节点后，才生成 aggregate reason `all_prerequisites_satisfied`。

这种区分防止一个常见缺陷：调用方看到第一步 decision 非零就提前开始下游副作用。未来正式 Participation use case 仍应只消费 aggregate，而不是把 trace 中任意 `eligible` 当授权票据。

### 5.4 为什么不并行执行两条只读规则

并行可以把理想尾延迟从 `Tregistration + Trisk` 降到约 `max(Tregistration, Trisk)`，但会失去三类保证：

1. 非新用户仍访问风险系统，扩大隐私处理范围；
2. 两个节点同时失败/拒绝时，首要业务 reason 可能受调度影响；
3. caller cancellation 和 provider failure 的组合更复杂，需要等待、取消、聚合错误和 goroutine 生命周期证明。

当前没有 P99、风险调用成本或容量数据证明并行收益大于这些代价。先选择顺序执行并测量，是比先并行再试图恢复确定性更可逆的决策。

若未来指标显示风险读取是主要瓶颈，应先问是否能在 upstream 形成低敏、短寿命、可撤销的 verdict 投影，或是否能改善 provider SLA，而不是直接牺牲短路。只有在产品明确允许为所有新用户候选访问风险事实，并定义多失败优先级后，才重新评估并行。

## 6. “失败关闭”究竟关闭什么

### 6.1 三种失败不能共用一个 `false`

```text
confirmed ineligible: 有足够事实证明条件不满足
technical failure:    没有足够事实形成决定
caller canceled:      调用方已不再等待结果
```

三者都不能进入下游，但只有第一种是业务拒绝。于是“fail closed”的精确定义是：

> 当事实、配置、时钟或依赖不足以形成可信决定时，停止资格链并禁止后续动作；同时返回零业务决定和可分类 error，不把系统故障伪装成用户不合资格。

这样做对重试、告警、指标和用户沟通都重要：

- `ineligible` 通常不应靠立即重试改变；
- provider unavailable 可能适合有界重试或降级提示；
- stale/invalid 需要数据质量告警；
- caller canceled 不应计入 provider 故障；
- blocked reason 可能属于敏感内部原因，不应直接显示。

### 6.2 为什么缺失风险 fact 不能默认通过

风险准入是安全敏感 gate。`not found` 可能表示尚未评估、provider 延迟、subject mapping 错误、数据删除或权限问题，任何一种都不是 `passed` 的证据。

因此：

```text
explicit passed  -> confirmed eligible for this step
explicit blocked -> confirmed ineligible
anything else    -> zero decision + error
```

这比“没有命中黑名单就通过”更严格，也要求未来产品设计考虑 provider 可用性：如果风险系统不可用时所有参与都停止，必须为依赖 SLO、容量、超时和运营响应承担成本。安全姿态不是免费的，但不能通过伪造 `passed` 来隐藏成本。

### 6.3 future fact 为什么不是时钟误差自动容忍

注册 `observed_at` 或风险 `assessed_at` 晚于 chain as-of，意味着本次逻辑评估正在使用“未来才形成”的事实。直接容忍固定秒数看似能处理 clock skew，却会模糊两个问题：

- source clock 是否同步；
- provider 是否返回了 after-as-of 的新快照。

当前选择明确失败关闭。未来若真实 provider 有可测量的时钟误差，应把 skew budget 写入 source contract、监控和 ADR，而不是在 domain 中悄悄减几秒。

### 6.4 cancellation 为什么必须优先于依赖错误

reader 返回后，代码立即检查 `ctx.Err()`。如果 caller 在读取期间已经取消，即使 provider 同时返回了自己的错误，最终结果仍应是 caller context error；否则监控会把用户离开或上游 deadline 误计为 provider failure。

反过来，如果 caller context 仍然有效，而 provider 返回 `context.DeadlineExceeded`，这是 provider 自己的内部 timeout，不应冒充 caller deadline。application 将它归类为 `Unavailable`，并把原始 cause 留给受信诊断通道。

这条顺序需要每个事实读取边界分别验证；仅在 chain 最外层检查一次 context 不足以区分读取期间发生的竞态。

## 7. 错误 Cause 通道：为什么从自动展开改为显式读取

### 7.1 第 25 节暴露的语义碰撞

如果一个安全 wrapper 同时实现：

```go
Is(target error) bool
Unwrap() error
```

那么 `errors.Is` 会先匹配 wrapper 的稳定 application class，再沿 `Unwrap` 遍历 provider cause。若一个错误声明 class 为 `Unavailable`，但 cause 恰好包含 `NotFound` 或另一个 application sentinel，调用方可能同时得到两个互斥分类：

```text
errors.Is(err, ErrUnavailable) == true
errors.Is(err, ErrNotFound)    == true
```

HTTP mapper、重试器或指标只要检查顺序不同，就可能采取不同动作。这不是错误文本泄露问题，而是机器语义被 cause 污染。

### 7.2 当前选择：一个公开 class，一个显式 Cause

第 26 节让 `RegistrationFactReadError` 与 `RiskScreeningFactReadError`：

- `Error()` 只渲染审核过的稳定 class；
- `Is()` 只匹配一个审核过的稳定 class；
- 不实现 `Unwrap()`；
- `Cause()` 显式返回受信诊断 error；
- 未知 class、零值和 typed-nil 都收敛到 read-failure；
- 原始地址、payload、subject detail 不进入普通错误字符串。

这使公共控制流可以可靠地写：

```text
errors.Is(err, ErrRiskScreeningFactUnavailable)
```

而不会因 provider cause 恰好携带别的 sentinel 获得第二分类。需要底层细节的受控 observer 必须先 `errors.As` 到具体 wrapper，再显式访问 `Cause()`。

### 7.3 这项选择的代价

显式 Cause 不是免费改进：

- 标准 `errors.Is/As` 不会自动穿透到 provider cause；
- 通用日志/trace 库若只认识 `Unwrap`，看不到底层原因；
- 嵌套安全 wrapper 需要受信代码显式递归；
- 团队必须区分“业务控制流看 class”和“诊断系统看 cause”；
- 诊断系统仍必须脱敏，`Cause()` 不是可随意打印的许可证。

当前权衡选择确定的单 class 控制流，牺牲通用自动展开。未来若 Go 生态工具要求标准 chain，同时仍需单 class，可以考虑结构化诊断事件、私有 observer callback 或单独 diagnostic record；不能简单恢复 `Unwrap` 而忽略语义碰撞。

### 7.4 原始 cause 不应进入哪些地方

即使 `Cause()` 被保留，下列位置默认禁止记录：

- HTTP response body；
- React 状态或浏览器 console；
- 普通 info/warn 日志；
- metrics label；
- span attribute 的未审核字段；
- 面向运营的可搜索自由文本；
- 持久化业务决定表。

未来受控诊断最多应提取审核过的 provider code、timeout 类别和 correlation ID，并设定访问控制、保留期与采样。原始 SQL、URL、credential、payload 或风险特征不应因为位于 `Cause()` 就自动安全。

## 8. 最小 trace：记录执行证据，不复制敏感事实

### 8.1 trace 要回答什么

一次确定资格结果需要回答：

1. 使用了哪个规则集 revision；
2. 实际执行了哪些具体规则、顺序是什么；
3. 每一步是 eligible 还是 ineligible；
4. 哪个稳定 reason 终止了评估；
5. 每一步使用哪个 policy revision；
6. 事实来自哪个受控 source 和 source revision；
7. 两步是否使用同一个 evaluated-at。

`EligibilityTraceStep` 因此只投影 rule code、outcome、reason、policy revision、fact source/revision 和 evaluated-at。aggregate 再记录最终 outcome、最终 reason、ruleset revision、evaluated-at 与 executed-step slice。

### 8.2 trace 明确不记录什么

当前 trace 不包含：

- `ParticipantRef`；
- registered-at、cutoff 或用户资料；
- risk score、特征、阈值、设备/IP 或原始 verdict payload；
- provider raw error/cause；
- credential、tenant、role 或权限；
- Lottery Strategy/Award、随机票据或选择结果；
- 用户文案；
- 未执行步骤的伪造结果。

“不含 ParticipantRef”降低 trace 值本身的直接可识别性，但并不自动匿名：fact revision、时间和 source 的组合仍可能具备关联性。因此当前 trace 只是进程内决定证据，不应在没有数据分类、访问控制与保留期时直接持久化。

### 8.3 为什么短路 trace 只保留真实前缀

新用户拒绝时，trace 长度应为 1；风险阻断时长度为 2；全部通过时长度为 2。未执行风险步骤不能记录为：

```text
skipped / assumed passed / not applicable
```

否则使用者无法分辨“真的没有调用 provider”与“调用后人为省略”。真实前缀让 call-count 测试、决策解释和未来路径可视化保持一致。

技术失败则返回零 aggregate，不把已经通过的新用户 step 当成可消费的半决定。失败节点的安全 class 由 error 提供；若未来需要失败 trace，应设计独立 execution diagnostic，明确它不能作为业务资格凭据。

### 8.4 为什么 `Steps()` 返回副本

slice 即使结构体本身按值返回，底层数组仍可共享。若调用方能改写 step，就能篡改内存中的执行证据。`Steps()` 返回副本，aggregate 构造时也复制输入 slice，保证外部修改不会改变内部记录。

这只解决进程内可变别名，不提供密码学防篡改、持久化审计完整性或跨服务不可抵赖。如果未来 trace 成为合规证据，需要追加写、完整性校验、访问审计和 retention policy。

### 8.5 metrics 与 trace 的基数边界

普通 metrics 可考虑的维度：

- rule code；
- aggregate/step outcome；
- stable reason code；
- stable error class。

默认不应作为 label 的字段：

- fact revision；
- participant reference；
- evaluated-at；
- provider raw cause；
- request/correlation ID；
- 任意用户或风险特征。

fact revision 可能每次评估都变化，高基数会增加时序数据库成本；它也可能被错误设计为包含 subject 信息。若需要按 revision 排障，应进入受控日志或 trace event，并首先证明 token 格式、脱敏和保留期。

## 9. 为什么是固定 concrete chain，而不是通用规则平台

### 9.1 两条规则真正共有的只有五件事

当前新用户和风险准入共同证明：

1. 都是 Participation 前置 gate；
2. 都返回 confirmed eligible/ineligible 或 error；
3. 都使用同一个 chain-owned evaluated-at；
4. 只有 eligible 才继续；
5. 都能投影为最小 trace step。

它们没有证明：

- 任意规则可动态注册；
- 任意上下文都能放进 `map[string]any`；
- priority 可以替代业务顺序；
- 规则需要插件、脚本或 DSL；
- Authorization、Inventory、Activity 和 Lottery 应共享接口；
- 规则应该从数据库加载；
- plan 需要运行时增删或跳转。

所以当前 `prerequisiteStep` 是 package-private 的 `code + evaluate closure`，外部构造器仍显式接收两个具体 reader 和两个 freshness bound；`Evaluate` 显式接收两份具体 policy 与 ruleset revision。抽取只消除了 for-loop 的重复，没有隐藏业务依赖。

### 9.2 为什么不用泛型

Go 泛型可以抽象“输入 fact、policy，输出 decision”，但两条规则的验证语义并不相同：

- registration 使用 `registered_at` 与 `observed_at`；
- risk 使用 source-owned `assessed_at`；
- future 与 stale error class 不同；
- policy 字段和业务 reason 不同；
- provider contract 与隐私分类不同。

若为了共享几行 validation 写一套类型参数和 constraint，读者需要先理解框架才能看到事实差异。当前 helper 只复用真正共同的 evaluation-instant 与 chain orchestration，保留具体命名。架构测试还显式拒绝 production 泛型类型和 `map[string]any` 事实袋，防止下一次改动绕开这个停止线。

### 9.3 为什么没有 priority 字段

priority 数字会把“为什么先执行”压缩成一个缺乏语义的整数，并诱导运行时排序。当前顺序由固定 plan 与 RuleSetRevision 共同表达，ADR 记录业务理由。

只有当运营确实需要在不发布代码时调整顺序，而且系统已经定义冲突、兼容、审批、回滚和历史解释，priority 才可能成为持久规则模型的一部分。第 26 节没有这些证据。

### 9.4 为什么不用 OPA、XACML、DMN 或脚本

这些技术可以表达更丰富的 policy，但不能替本项目回答：

- 事实由谁拥有、怎样新鲜；
- ParticipantRef 是否绑定真实 Principal；
- quota 怎样原子消费；
- 风险材料怎样最小化；
- 哪条规则能有副作用；
- public reason 怎样披露；
- 历史版本怎样重放。

当前只有两条固定 Participation gate，引入外部 policy runtime 会增加部署、语言、调试、发布、授权和审计面，却没有真实动态配置需求。未来第 28～29 节若通过规则树持久化和决策引擎证明需要，再比较外部引擎，而不是因为“规则多了”就自动选型。

## 10. 备选方案与权衡矩阵

| 方案 | 短路真实性 | 时间一致性 | 所有权清晰度 | 当前复杂度 | 主要风险 | 结论 |
| --- | --- | --- | --- | --- | --- | --- |
| 在 handler 连写两个 `if` | 可做到，但容易分散 | 通常每段各读时间 | 易混入 HTTP/身份 | 低 | 重复错误/trace/取消语义 | 不采用；当前尚无可信 handler |
| 先预取两份 fact，再纯函数组合 | 无，风险总会读取 | 可在读取后取一次时间 | 事实 shape 可清晰 | 中 | 隐私、成本、故障传播 | 拒绝 |
| 两个 service 顺序调用，各读 clock | 有 | 无单一 as-of | service 边界清晰 | 低 | 边界结果不可统一重放 | 拒绝 |
| 两条规则并行 | 无读取短路 | 可共享开始时刻 | 尚可 | 高 | reason 漂移、取消与 goroutine 管理 | 当前拒绝 |
| `[]Rule` + `map[string]any` | 取决于 runner | 可设计 | 极差，事实袋无所有者 | 中 | 运行时类型错、万能接口 | 拒绝 |
| generic `Rule[F,P,D]` | 可做到 | 可设计 | 类型安全但语义被模板化 | 中高 | 为两条规则预支框架 | 当前拒绝 |
| 数据库 priority + 动态加载 | 可做到 | 可设计 | 需要正式规则聚合 | 高 | 发布/回滚/无效组合/审计缺失 | 留给第 28 节之后 |
| OPA/XACML/DMN | 可表达丰富 combining | 可设计 | 仍需属性权威模型 | 很高 | 把资格与授权混成平台 | 当前拒绝 |
| 固定 Participation concrete chain | 真正 lazy | 一个受控 as-of | 具体 reader/policy 清楚 | 与证据相称 | 仍是线性、无生产 adapter | 采用 |

### 10.1 为什么不把 risk policy 写成可配置阈值

当前 risk source 给出离散 `passed/blocked`，没有分数。凭空添加 `maxScore` 会迫使 Participation 解释 provider 模型、阈值区间和版本语义，也可能扩大敏感数据处理。

v1 的 `RiskAdmissionPolicy` 只保留 revision，映射固定。这看似“配置少”，实则把尚无证据的变化维度锁在门外。若未来真实 provider 返回多级 verdict，且不同 Activity 需要不同映射，应新增事实枚举/政策字段与兼容测试，而不是现在预留任意 map。

### 10.2 为什么组合结果不直接复用某个单步 Decision

风险通过的单步 decision 只证明风险 gate；它不知道 ruleset revision，也不能证明新用户 gate 已执行。新建 `PrerequisiteEvaluation` 明确区分：

- 单规则决定的 provenance；
- 整条 plan 的版本与终态；
- 实际执行路径。

这避免“最后一个节点的 eligible 等于整条链 eligible”的隐式约定。未来增加第三条 gate 时，aggregate 语义无需改变。

### 10.3 为什么技术错误不返回部分 aggregate

部分 aggregate 可能帮助诊断，例如“新用户已通过，风险读取失败”。但一旦它和 error 同时返回，调用方很容易误用前半段继续业务，或者把半 trace 持久化成最终结果。

当前选择零 aggregate + error，安全诊断通过稳定 class 和受控 cause 完成。未来若确有运维需求，应增加独立 `ExecutionAttempt` 类型，并让它在类型和存储上不能被当作 `PrerequisiteEvaluation`。

## 11. 测试思维：每条断言要证伪一个错误设计

### 11.1 领域事实与政策

风险领域测试需要证明：

- 非零 ParticipantRef、合法 `passed/blocked`、canonical UTC `assessed_at` 才能构成 fact；
- zero/unknown disposition 被拒绝，不能默认通过；
- source 和 revision 必须非空、规范、可打印且有大小上限；
- fact 不接受 score、特征、阈值或 Participation verdict；
- policy revision 与 ruleset revision 分离且分别验证；
- `passed` 形成 `risk_screening_passed`；
- `blocked` 形成 `risk_screening_blocked`；
- future fact 和零 evaluated-at 返回零 decision；
- 同一 policy、fact、instant 多次执行得到相同决定；
- decision 不含 ParticipantRef、assessed-at、风险 payload 或用户文案。

这些测试证伪的是“格式看起来差不多就能用”和“unknown 当 passed”的实现。

### 11.2 application freshness 与读取错误

边界测试应覆盖：

- `assessed_at == evaluated_at - maxAge` 仍然有效；
- 早一纳秒即 stale；
- 晚于 as-of 一纳秒即 future/invalid；
- 不同 ParticipantRef 返回 invalid；
- max age 非正和 malformed dependency 在任何 I/O 前失败；
- provider not-found、unavailable、unknown failure 映射到一个稳定 class；
- provider 自身 deadline 在 caller context 仍有效时映射为 unavailable；
- caller 在 read 返回前取消时，原始 caller context error 优先；
- error string 不渲染秘密 cause；
- `errors.Is` 不能同时命中两个 application class；
- 受信 `Cause()` 仍能取到底层诊断。

这些测试证伪的是“过期边界 off-by-one”“provider timeout 冒充 caller timeout”和“错误链多重分类”。

### 11.3 chain 真值与短路矩阵

组合测试至少应覆盖：

| 新用户节点 | 风险节点 | aggregate | trace 长度 | risk reader calls |
| --- | --- | --- | ---: | ---: |
| eligible | passed | eligible / all prerequisites satisfied | 2 | 1 |
| eligible | blocked | ineligible / risk blocked | 2 | 1 |
| ineligible | 不执行 | ineligible / registration before cutoff | 1 | 0 |
| registration error | 不执行 | zero + error | 0 | 0 |
| eligible | risk error | zero + error | 0 | 1 |
| caller pre-canceled | 不执行 | zero + context error | 0 | 0 |

特别注意：技术错误时 aggregate trace 仍为零，即使内部曾执行第一步。测试应检查整个 struct 的零值，而不只检查 outcome 空字符串。

### 11.4 时间、顺序和不可变性

还需要证明：

- chain clock 每次 Evaluate 至多调用一次；通过输入/配置校验且未预先取消后恰好调用一次；
- clock 在任一 reader 前调用；
- 两个 step 与 aggregate 的 evaluated-at 完全相同；
- trace rule code 顺序固定为 new-user、risk；
- `Steps()` 返回值被调用方修改后，aggregate 再次读取不变；
- ruleset revision、两份 policy revision、两份 fact revision 互不覆盖；
- 手工构造的 zero/typed-nil chain、reader 或 clock 不 panic；
- concurrent Evaluate 不共享 request trace 或 instant；
- 注入的 reader/clock 并发安全是明确依赖条件；
- `go test -race` 用于捕捉 chain 自身共享状态错误，但不能证明外部 provider 线程安全。

### 11.5 架构和负向证据

本节还要用差异和 AST 测试证明没有做的事：

- Participation production code 不 import Lottery、Gin、SQL、Redis、React 或其他项目上下文；
- 不声明 `RuleEngine`、`RuleTree`、`EvaluationContext`、`RulePriority`、DSL 或泛型规则类型；
- 不出现 `map[string]any`/空接口事实袋；
- 不新增 migration、route、配置、Compose service、Redis key 或 UI；
- Lottery selector、ephemeral API 与 React 页面相对第 25 节保持不变。

AST name guard 只能捕获已知名字和形状，不能证明所有语义上的过度抽象。代码评审仍要检查匿名 interface、反射、闭包内隐式跳转和字符串协议。

### 11.6 测试不能证明什么

即使上述测试全部绿色，也不能证明：

- production provider 是权威且可用；
- 两个 authority 提供原子一致快照；
- 未来网络 P99 满足目标；
- source revision 永不包含 PII；
- blocked reason 可安全向用户披露；
- caller 已认证且有权为该 ParticipantRef 求值；
- 资格到次数消费/抽奖之间没有竞态；
- trace 已满足合规审计；
- 当前链能表达多分支会员路由；
- API、浏览器或 Compose E2E 已经存在。

测试通过的边界必须与宣称能力的边界完全相同。

## 12. 威胁模型：谁能操纵什么，系统必须怎样失败

### 12.1 资产

本节需要保护的资产包括：

- Participation 资格决定的完整性；
- 风险事实与其存在性的机密性；
- provider 原始 error、地址和 payload；
- rule/reason/revision 的可解释性；
- provider 容量与可用性；
- 未来业务不能把零决定当通过；
- trace 不能被调用方改写后冒充执行证据。

### 12.2 信任边界

```text
future authenticated use case
  -> Participation application input validation
  -> consumer-owned RegistrationFactReader boundary
  -> external account authority
  -> consumer-owned RiskScreeningFactReader boundary
  -> external risk authority
  -> pure Participation domain evaluators
```

当前代码只实现中间 domain/application 边界。外部 authority、credential 和 transport 尚不存在；最左侧的真实会话也尚不存在。

### 12.3 主要威胁与当前控制

| 威胁 | 可能后果 | 当前控制 | 剩余缺口 |
| --- | --- | --- | --- |
| 伪造 ParticipantRef | 查询或替他人参与 | 无公开 route；fact subject 双重核对 | 未来必须用 Principal 绑定，当前无认证 |
| provider 返回错主体 fact | A 的事实用于 B | application 比较 ParticipantRef | adapter/source identity 尚未证明 |
| 使用 stale passed verdict | 已撤销主体继续参与 | source-owned time + max age | 无撤销 push、无实时投影 |
| adapter 给旧 verdict 重盖本地时间 | 永久续鲜 | contract 要求 source-owned assessed-at | 当前无真实 adapter 验收 |
| future fact/clock skew | 使用 as-of 后事实 | future fail-closed | 无 provider skew SLO |
| risk provider 不可用 | 所有合资格请求受阻 | typed unavailable + fail-closed | 尚无容量、熔断、运营预案 |
| 非新用户仍触发风险读取 | 不必要敏感处理和成本 | fixed order + lazy short-circuit | 需要 call-count/生产指标证明 |
| raw cause 泄露 | 地址、payload、风险信息外泄 | safe Error + explicit Cause | trusted observer 仍可能误打印 |
| error 多 class | mapper/重试行为不确定 | Cause 不进入 errors.Is tree | 通用 tooling 需适配 |
| trace 被修改 | 伪造进程内证据 | defensive copy | 无持久化完整性保护 |
| 高基数 revision 进入 metrics | 成本/信息泄露 | 文档禁止，类型不等于 label | 尚无实际 observer guard |
| chain 被通用化并注入副作用 | 中途失败需补偿 | private step、架构测试、只读 ports | 代码评审仍需防闭包副作用 |
| 重试重复外部调用 | provider 放大 | 当前无自动重试 | future adapter 必须定义 budget |
| 慢 reader 忽略 context | goroutine/请求长时间占用 | context 传入 | 同步接口无法强制实现及时返回 |

### 12.4 reason disclosure 需要二次策略

内部 stable reason `risk_screening_blocked` 有诊断和规则解释价值，但未来 HTTP 不一定能原样返回。向用户明确“被风控阻断”可能帮助攻击者探测策略或对不同账号做枚举。

本节没有 transport，所以暂不做公开映射。未来 API 层需要单独决定：

- 用户是否只看到通用“暂不可参与”；
- operator 是否在授权后看到稳定内部 reason；
- 安全团队是否看到更细 provider code；
- 每类信息的审计与保留期是什么。

稳定内部 reason 与可修改、分受众的文案必须分离。

## 13. 性能、容量与故障域思考

### 13.1 当前延迟模型

忽略本地纯函数开销，一次 chain 的期望延迟近似：

```text
E[T] = Tclock + Tregistration
       + P(registration eligible) * Trisk
```

一次评估的风险调用率近似：

```text
RiskCallRate = RequestRate * P(registration eligible)
```

这两个公式说明排序需要真实流量数据验证：如果绝大多数主体都是新用户，短路节省很小；如果风险 provider 极慢，串行尾延迟可能显著。当前没有生产测量，所以不能声称顺序已经优化 P99，只能说它优先保护隐私最小化、原因稳定和故障隔离。

### 13.2 需要观测但本节不实现的指标

未来安全 observer 可以考虑：

- chain attempts 按 aggregate outcome/error class；
- step attempts 按 rule/outcome/reason；
- 每个 reader duration 与 timeout 类别；
- short-circuit count；
- risk reader avoided count 或由请求与调用差值推导；
- stale/future/subject-mismatch 计数；
- provider availability 与 deadline；
- chain total duration；
- caller cancellation 与 provider cancellation 分离。

这些指标只能使用低基数审核维度。当前代码没有 observer，文档不能把“可记录”写成“已采集”。

### 13.3 timeout 与重试预算

chain 接收 caller context，但没有自行创建每个 provider 的子 timeout，也没有重试。未来 adapter/composition 需要基于端到端预算回答：

```text
request deadline
  >= registration budget
  + risk budget
  + downstream formal participation budget
  + response margin
```

风险 provider 的自动重试必须考虑：

- 调用是否只读且幂等；
- provider timeout 后是否仍在执行；
- 重试是否放大故障；
- caller 剩余 deadline；
- 相同 as-of 下是否还能返回有效 snapshot；
- 重试返回更新 fact 是否改变本次逻辑时刻语义。

在这些问题未回答前，application 不隐藏重试是合理停止线。

### 13.4 并发安全的真实边界

chain 对象只保存 reader、clock 和两个 duration；每次 Evaluate 的 instant、plan closure 和 trace 都是局部值，所以设计上没有共享请求态。并发安全仍取决于注入依赖：

- reader 是否能被多 goroutine 调用；
- clock implementation 是否线程安全；
- adapter client pool 是否有界；
- provider 是否接受相同主体并发读取；
- observer 是否使用无锁高基数结构。

`go test -race` 能发现当前测试覆盖路径上的内存竞态，不能证明远端容量、逻辑幂等或未执行路径。

## 14. 隐私与数据生命周期

### 14.1 数据最小化不仅是少几个字段

本节采取的最小化包括：

- risk fact 只保留离散 disposition、assessed-at、source/revision；
- 不把 provider 原始 payload 带进 domain；
- 决定和 trace 不携带 ParticipantRef；
- 新用户拒绝后不读取 risk；
- error string 不渲染 cause；
- 未实现持久化 trace、Redis cache 或前端状态；
- 未把风险 reason 定义成公开用户文案。

其中“根本不调用不必要的风险系统”比“调用后不记录字段”更强，因为它减少了外部系统中的访问日志、网络流量和潜在关联记录。

### 14.2 source/revision 仍需数据治理

bounded printable token 只能防止零值、控制字符和无限长度，不能阻止 adapter 把邮箱、手机号或完整用户 ID 编进 revision。未来 provider contract 必须规定：

- token 不含 PII、credential 或原始 payload；
- revision 的唯一性和作用域；
- 是否单调、是否可重放；
- 日志和 trace 中的保留期；
- 删除请求是否要求清除关联记录；
- 哪些角色可检索 source revision。

value-object validation 是数据治理的最后一道格式防线，不是完整治理方案。

### 14.3 为什么当前不缓存资格结果

资格决定依赖 ParticipantRef、两份事实、两份 policy、ruleset revision 和 evaluated-at。安全 cache key 至少需要表达这些变化维度，还要处理：

- risk blocked 的立即撤销；
- registration 更正或账号合并；
- policy/ruleset 发布；
- freshness 继续流逝；
- tenant/scope 隔离；
- PII 与访问审计；
- fail-open/fail-closed 策略。

当前第 24 节 Redis 只缓存可由 Lottery MySQL 重建的 Strategy 投影，不适合顺手承载用户资格。没有失效协议前不缓存，是比猜一个短 TTL 更准确的决定。

## 15. 风险账本

| ID | 风险/未知 | 当前影响 | 现有控制 | 触发后动作 |
| --- | --- | --- | --- | --- |
| R26-01 | 无真实 registration adapter | 不能证明外部注册事实可取 | consumer-owned port、无 route | adapter 章节定义 authority、timeout、revision、删除/更正语义 |
| R26-02 | 无真实 risk adapter | 不能证明 verdict 权威、延迟和可用性 | 最小 port、fail-closed | 做 provider contract test 与故障注入，不用 stub 冒充联通 |
| R26-03 | 两 authority 非原子快照 | as-of 可能只是逻辑基准 | single instant、future reject | 需要历史一致性时引入 as-of query/token 或事实投影 |
| R26-04 | provider 只能返回读取后产生的新 fact | 后序 fact 可能晚于 as-of | fail-closed | 明确 provider snapshot contract，必要时重新设计会话时刻 |
| R26-05 | risk provider 故障阻断全部候选 | 可用性受安全依赖限制 | typed unavailable、无默认通过 | 建立 SLO、容量、timeout、告警和运营预案 |
| R26-06 | 串行读取提高 P99 | 用户等待增加 | 第一节点短路 | 用真实分布测量，再评估投影/并行，不凭直觉改序 |
| R26-07 | source revision 可能高基数或含 PII | metrics 成本或隐私泄露 | trace 最小化、禁止 label | adapter contract + telemetry review |
| R26-08 | blocked reason 被外部枚举 | 攻击者推测风控 | 当前无 HTTP | 未来 transport 做受众分层和最小披露 |
| R26-09 | `Cause()` 被受信代码直接打印 | 原始 provider 信息泄露 | safe Error、显式访问 | observer 白名单提取、脱敏、访问控制、保留期 |
| R26-10 | 非标准 Cause 不被通用 tooling 识别 | 根因可观测性下降 | typed wrapper 和测试 | 评估结构化 diagnostic channel，不恢复多 class Unwrap |
| R26-11 | 资格通过后事实立即变化 | 后续仍可能不应参与 | 当前决定仅是当时快照 | 正式 Participation 定义有效期、再校验或原子承诺 |
| R26-12 | 未来规则含副作用 | 中途失败需要补偿 | 当前 ports 只读、停止线测试 | 副作用移出 gate，进入正式事务/工作流 |
| R26-13 | chain 顺序被当无害重构 | reason、成本、隐私变化 | RuleSetRevision + ADR | 新 revision、兼容与回归证据、披露评审 |
| R26-14 | private closure 隐藏条件跳转 | 线性模型名存实亡 | 固定 plan、AST guard | 第 27 节用显式分支证明并升级模型 |
| R26-15 | caller 可伪造 ParticipantRef | IDOR/代他人判断 | 无公开 route | 第 31～35 节真实会话与服务端 RBAC 后才开放 |
| R26-16 | trace 被误当合规审计 | 缺失持久性、完整性和访问记录 | 明确进程内证据 | 有合规需求时单独设计 audit record |
| R26-17 | max age 是全局构造参数 | 可能不适合不同 source/场景 | 显式注入、测试边界 | 有多个场景后决定归属 source contract 还是 policy revision |
| R26-18 | 当前 risk mapping 只有 passed/blocked | 无法表达真实多级 verdict | 固定 v1、拒绝 unknown | 只有 provider contract 出现真实等级后扩展 |

风险账本的目的不是把所有风险在本节消灭，而是防止它们在“测试绿色”后从认知中消失。每一项都应有可观测触发器，而不是无限期的“以后优化”。

## 16. 重决策触发器：哪些事实出现后必须重新画边界

### 16.1 重新考虑 logical as-of

出现任一情况时，需要新 ADR：

- provider 明确不能返回 as-of 前形成的 immutable snapshot；
- 业务要求历史重放得到与当时完全一致的结果；
- clock skew 超过可接受范围且能量化；
- 多 authority 需要同一一致性 token；
- 资格决定需要跨分钟/小时长期有效；
- 事实更正、撤销或迟到事件需要追溯重算。

可选方向包括 source as-of query、effective interval、事件投影或持久化 decision inputs。不能只把 max age 调大来隐藏契约缺口。

### 16.2 重新考虑顺序或并行

需要同时具备以下证据，而不是只看到一次慢请求：

- 真实流量中各节点 pass/reject 比例；
- 两个 provider 的 P50/P95/P99 与错误率；
- 风险调用的隐私和成本政策允许扩大访问；
- 多个拒绝同时成立时的稳定优先级；
- caller cancellation 与 goroutine 生命周期方案；
- 新顺序对应的新 RuleSetRevision 和兼容说明。

若只是风险 provider 慢，优先优化 provider/投影和 timeout；若产品改变拒绝原因优先级，再考虑调序。

### 16.3 重新考虑错误 Cause 设计

触发条件：

- 生产 observer 必须依赖标准 `errors.Unwrap`；
- 需要跨 RPC 传递结构化 provider diagnostics；
- cause 中出现 application sentinel 的违规输入无法在 adapter 侧杜绝；
- 安全团队要求对底层原因做访问审计；
- 当前显式递归使诊断代码重复或遗漏。

候选方案应比较：单独 diagnostic record、observer callback、typed provider code、日志事件与标准 chain。目标仍是公共控制流恰好一个 semantic class，不能以工具兼容为由恢复歧义。

### 16.4 重新考虑 trace 持久化

只有在以下用例真实出现后持久化：

- 用户申诉需要解释某次 Participation 的规则路径；
- 合规要求保留决定依据；
- 灰度规则需要按历史 revision 复盘；
- 故障调查需要关联 provider snapshot；
- 正式 Draw/Result 需要绑定资格凭据。

届时必须同时定义 schema version、主体关联、加密、访问控制、保留/删除、完整性、防重放和迁移，而不是把当前 struct 直接 JSON 化存表。

### 16.5 重新考虑是否引入通用引擎

只有当真实规则出现下列至少一种复杂度时，才有证据扩大模型：

- 同一事实导致不同后续分支；
- 多个终端业务结果，而非只有 eligible/ineligible；
- 缺省分支和显式无法匹配；
- 路径合流或共享子路径；
- 运营需要持久化发布、审批、回滚和历史版本；
- 多个上下文需要同一种 policy language，且语义确实一致；
- 规则图需要静态验证循环、不可达节点和类型匹配。

“第三条规则出现”本身不是引擎触发器；三条固定必经 gate 仍可由线性 chain 正确表达。

## 17. 第 27 节怎样从本节自然演进

### 17.1 线性 chain 的表达能力边界

当前 plan 的每一步只有两种控制效果：

```text
eligible   -> next sequential step
ineligible -> terminal reject
error      -> terminal technical failure
```

它适合“所有条件都必须通过”的 conjunction。第 27 节计划引入真实会员分层路由后，控制流可能变成：

```text
membership tier?
  ├─ premium -> premium-specific prerequisite -> shared tail
  ├─ standard -> standard-specific prerequisite -> shared tail
  └─ unknown -> explicit default/reject/error
```

此时 `eligible` 不再总是“数组下一个”；节点可能选择不同出口，路径需要记录 branch code，多个路径还可能合流。

### 17.2 哪些坏味道说明不能继续伪装成 chain

如果第 27 节为了保留 `[]step` 开始出现下面任一形状，就说明线性抽象已经不够：

- step 返回 `nextIndex`；
- closure 内根据事实跳过后面若干节点；
- 同一个节点复制到多个 slice；
- 用 `goto`、递归或隐藏 map 表达跳转；
- outcome 增加 `continue_premium`、`continue_standard` 等控制码；
- trace 只能记录执行顺序，不能说明为什么选择该分支；
- default branch 由 slice 越界隐式表示；
- 为分支塞入 `map[string]any` 和运行时 type assertion。

正确演进不是把这些复杂度藏在责任链内部，而是承认新业务需要显式决策拓扑。

### 17.3 第 27 节应保留本节哪些不变量

从 chain 演进到分支模型时，下列原则不能因数据结构变化而丢失：

- 事实 authority 与 Participation decision ownership 分离；
- caller 无法提交 evaluated-at；
- 同一次执行共享明确逻辑时刻；
- confirmed business result 与 technical indeterminate 分离；
- caller cancellation 优先；
- 未选路径不访问其敏感 provider；
- rule、policy、fact、ruleset/schema/app revision 分离；
- trace 只记录真实路径且最小披露；
- 节点继续只读，无随机、扣减、库存或消息副作用；
- 无可信身份和正式用例前仍不接公开 Lottery route。

第 27 节的价值不是“模式升级”，而是用多出口会员路由提供可执行证据：当前只有 `next/stop` 的协议无法准确表达新事实。第 28 节再讨论规则树如何持久化，第 29 节再讨论决策引擎；顺序不能倒置，否则数据库 schema 和 engine API 会先于业务路径决定模型。

## 18. 架构师的逐步思考清单

遇到“再加一个资格规则”时，不应从新建 interface 开始，而应按下面顺序追问：

1. **为什么需要这条规则？** 它阻止哪种真实业务损失或履行哪项产品政策？
2. **它属于哪个阶段？** 是资格、授权、路由、随机选择、库存、发放还是审计？
3. **谁拥有原始事实？** 当前上下文消费什么最小快照，而不复制对方模型？
4. **谁拥有最终决定？** provider 的 verdict 是否等于本场景 decision？
5. **事实何时成立？** 需要产生时间、观察时间、effective interval 还是版本 token？
6. **事实多旧仍可用？** 边界是否 inclusive，谁批准 max age？
7. **unknown 是什么？** 缺失、过期、future、损坏、timeout 是否各自可诊断？
8. **顺序有何后果？** reason、隐私、成本、延迟和故障面是否变化？
9. **拒绝后还能访问谁？** 测试能否用零调用证明真实短路？
10. **节点是否有副作用？** 如果有，为什么它还叫前置 gate，失败如何补偿？
11. **一次评估使用哪个时刻？** 谁能提供，caller 能否伪造？
12. **哪些版本必须分开？** rule/policy/fact/ruleset/schema/app 是否各有生命周期？
13. **最终 reason 给谁看？** 用户、运营和安全团队是否需要不同披露？
14. **trace 最少要留什么？** 哪些字段高基数、敏感或可重识别？
15. **错误怎样分类？** 控制流能否恰好匹配一个 class，底层 cause 怎样受控查看？
16. **当前测试能证伪什么？** call count、边界纳秒、cancel 竞态和 zero decision 是否覆盖？
17. **当前测试不能证明什么？** adapter authority、生产 P99、认证、事务和审计是否仍空缺？
18. **什么时候必须重设计？** 把触发条件写入风险账本，而不是只写“未来扩展”。

这份清单体现第一性原则的核心：先识别不可替代的业务事实和失败代价，再选择最小机制；设计模式只是机制的名字，不是需求的来源。

## 19. 证据索引与最终边界陈述

### 19.1 本地规范与代码

| 设计命题 | 规范/实现入口 |
| --- | --- |
| 第二条规则与停止线 | [Participation 前置资格链基线 v1](../../product/participation-prerequisite-chain-v1.md) |
| 采用固定 chain 的决策与备选方案 | [ADR-0022](../../decisions/ADR-0022-participation-prerequisite-chain.md) |
| 基础事实所有权与顺序要求 | [Lottery 业务规则需求基线 v1](../../product/lottery-rule-requirements-v1.md) |
| 第 25 节新用户边界 | [新用户资格规则基线 v1](../../product/new-user-eligibility-v1.md) |
| source-owned risk snapshot | [risk_screening_fact.go](../../../internal/participation/domain/risk_screening_fact.go) |
| fixed risk admission mapping | [risk_admission.go](../../../internal/participation/domain/risk_admission.go) |
| consumer-owned ports | [ports.go](../../../internal/participation/application/ports.go) |
| one package-owned logical instant | [evaluation_instant.go](../../../internal/participation/application/evaluation_instant.go) |
| fixed order, short circuit and copied trace | [prerequisite_chain.go](../../../internal/participation/application/prerequisite_chain.go) |
| one public error class + explicit diagnostic cause | [errors.go](../../../internal/participation/application/errors.go) |
| no generic platform / cross-context drift | [architecture_test.go](../../../internal/participation/application/architecture_test.go) |

### 19.2 外部资料只用于校准，不替代本地业务决策

1. Go, [Code Review Comments — Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)：支持“由消费方定义最小接口”的语言实践；不能证明某个 adapter 具备组织级 authority。
2. Go, [`context` package](https://pkg.go.dev/context)：用于 cancellation/deadline 的传播约定；不能强制一个忽略 context 的同步 provider 及时返回。
3. Go, [`errors` package](https://pkg.go.dev/errors)：解释 `Is/As/Unwrap` 的标准遍历机制，也说明为何原始 cause 进入 error tree 后可能影响机器分类。
4. OASIS, [XACML 3.0 Core Specification](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-cos01-en.html)：用于校准 Deny 与 Indeterminate 不应混淆；本节不实现 XACML 或授权 policy combining algorithm。
5. Open Policy Agent, [Policy Language](https://www.openpolicyagent.org/docs/policy-language)：用于对照通用 policy runtime 的能力与成本；本节没有选择 OPA，也没有动态 policy language。

### 19.3 本节可以准确地说什么

> GrowthOS-Go 已在 Participation domain/application 内核中新增第二条真实风险准入规则，并从新用户与风险两条具体前置条件中抽取固定、有序、短路的资格链。组合器在任何事实读取前捕获一次受控服务端 logical as-of；新用户确定通过后才读取风险事实；两条规则共享同一时刻。只有全部节点确认通过才形成 aggregate eligible，确定拒绝形成带真实执行前缀的 ineligible，事实/配置/依赖失败或 caller cancellation 则返回零 aggregate。规则集、单规则政策、事实来源 revision 和 rule identity 相互分离；trace 不携带主体或原始风险材料；provider cause 被隔离在不参与 `errors.Is` 的显式受信通道。实现仍被限制在 Participation 内核，没有引入通用规则平台。

### 19.4 本节不能声称什么

本节不能声称：

- 已接入真实用户目录或风险服务；
- source string 自身证明 provider 权威；
- 已完成 Activity 发布或规则集数据库配置；
- 已认证 caller 或执行 RBAC；
- 现有 Lottery API 已被资格保护；
- 已创建 Participation/Draw、扣减次数、选择 Award 或发奖；
- 单一 as-of 提供跨系统原子快照；
- 风险 provider 故障时仍满足可用性目标；
- trace 是持久、不可篡改或合规审计；
- production metrics、日志、trace 或告警已经落地；
- UI、路由、API、Compose 或浏览器 E2E 已经改变；
- 线性 chain 已经能表达第 27 节的会员分层路由；
- 规则树、持久化规则或决策引擎已经实现。

最终应保留的第一性原则是：

> **不要为了展示模式而制造规则，也不要因为两段代码相似就抹掉事实所有权。先让第二个真实业务条件证明共同的顺序、时间、停止和证据语义；再把恰好重复的部分抽成最小协议。确定拒绝与无法决定必须分离，不必要的敏感读取必须真的不发生。等线性协议被真实分支证伪后，再演进到下一种结构。**
