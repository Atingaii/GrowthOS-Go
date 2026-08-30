# 第 27 节设计手记：用一个真实多出口路由证明线性链的边界，但不让“树”抢在问题前面

> 第 26 节证明了 Participation 的两个必经资格条件可以组成一条固定线性链：当前节点确定通过才进入固定下一节点，确定拒绝、技术失败或 caller cancellation 都停止。第 27 节出现的变化不是“再加一条资格规则”，而是 Lottery 第一次必须在两个都合法的后续 Strategy 目标之间做确定路由。
>
> 本节据此冻结一个刻意很小、但业务语义真实的决定：**外部会员 authority 只提供已确认的 `standard` / `premium` 事实；Lottery policy 规定 `premium -> premium_override`、`standard -> baseline_default`；未知、未支持、缺失、陈旧、未来或损坏事实都不能命中 default。成功时返回唯一 Strategy ID 与一跳 path，失败或取消时返回零决定。**
>
> 本手记记录 2026-08-30 的第 27 节当前实现时间切片。规范性边界以 [ADR-0023](../../decisions/ADR-0023-membership-strategy-routing-boundary.md) 和 [Lottery 会员等级 Strategy 路由基线 v1](../../product/membership-strategy-routing-v1.md) 为准。本文只陈述 Lottery domain/application 内核及其测试证据，不声称会员 provider adapter、HTTP/API、Activity 绑定、正式 Draw、权限、UI 或浏览器 E2E 已经接入。

---

## 1. 先把问题还原：我们需要的不是“树”，而是一个可信 Route

### 1.1 第 26 节能够表达什么

第 26 节的 Participation chain 本质上计算一个合取：

```text
new-user eligible
  AND risk-admission eligible
  -> prerequisite eligible
```

它的控制流协议只有：

| 当前节点结果 | runner 能做什么 | 后序位置 |
| --- | --- | --- |
| confirmed eligible | 继续 | slice 中固定的下一项 |
| confirmed ineligible | 终止 | 无 |
| technical error | 终止且返回零 aggregate | 无 |
| caller cancellation | 终止且返回零 aggregate | 无 |

这对于“所有 gate 都必须通过”的资格前置是准确的。runner 不需要知道多个成功终点，也不需要选择边；`eligible` 的唯一含义就是“进入固定下一项”。

### 1.2 第 27 节出现了什么新事实

第 23 节的 LRR-003 已经登记：会员分层可能路由到不同 Strategy，而且缺省语义必须显式。第 27 节把这条开放需求收敛成首个可验收产品决定：

```text
confirmed premium
  -> premium_override
  -> premium Strategy target

confirmed standard
  -> baseline_default
  -> baseline Strategy target
```

这两个结果都不是拒绝，也都不是“继续到同一个下一节点”。它们是两个不同的成功出口。于是问题从布尔合取变成分段函数：

```text
Route(policy, fact, evaluated-at) =
  premium target   if fact.tier == confirmed premium
  baseline target  if fact.tier == confirmed standard
  no decision      otherwise
```

形式上可以写为：

```text
R : ValidPolicy × ValidMembershipFact × ValidInstant
    -> Confirmed(branch, target, evidence)
    OR ZeroDecision + TechnicalError
```

这里没有 `Reject` 分支。对 v1 来说，`standard` 和 `premium` 都是可路由事实；无法确认 tier 时不是“会员不合格”，而是 Lottery 无法形成可信 Route。

### 1.3 第一性问题

因此，本节真正要回答的是：

> 在不让外部会员系统拥有 GrowthOS Strategy 映射、不让 Participation 资格链吞并 Lottery 路由、不把 unknown 静默降级成普通用户、也不提前发明持久化规则树的前提下，怎样用最小代码形成确定、可解释、可取消且失败关闭的 Strategy Route？

这个问题可以继续拆成九个子问题：

1. 谁确认会员等级事实；
2. 谁决定等级到 Strategy 的映射；
3. `standard`、`premium`、`unknown` 与 `default` 分别是什么；
4. Route 与 eligibility、authorization、selection 有何区别；
5. 同一次路由使用哪个时间；
6. provider 错误与 caller cancellation 怎样区分；
7. path 最少需要记录什么；
8. 为什么当前 chain 已经不够，但 rule tree 仍然过早；
9. 当前测试能证明什么，不能证明什么。

### 1.4 本节的最小成功标准

本节只有同时满足以下条件才算形成一个可信内核切片：

- `standard` 与 `premium` 能产生两个可区分 branch；
- 两个 branch 能指向两个不同 Strategy ID；
- 即使两个 target 在 rollout 中临时相同，branch 证据仍不丢失；
- unknown/unsupported 不能借 default 获得 target；
- subject、fact source/revision、policy revision 和 evaluated-at 可被验证；
- 时间边界、取消与 provider failure 不返回半成品决定；
- path 只记录真实的一跳，不伪造未走分支；
- Lottery 不读取 Participation 内部模型；
- Strategy 不加载、Award 不选择、随机票据不消费；
- 没有为了“一步到位”创建 RuleTree、Engine、DSL 或无类型事实袋。

## 2. 当前代码切片：准确说已经有什么

### 2.1 domain 层

当前 Lottery domain 已形成：

- `MembershipSubjectRef`：Lottery 本地 opaque 会员查找引用；
- `MembershipTier`：只接受 `standard` 与 `premium` 的封闭词汇；
- `MembershipTierFactSnapshot`：subject、tier、authority `observedAt`、source、fact revision；
- `MembershipStrategyRoutingPolicy`：policy revision、premium target、baseline/default target；
- 稳定 rule/branch/reason code；
- `MembershipRoutingPathStep`：一跳中的 rule、branch、target；
- `MembershipStrategyRouteDecision`：confirmed Route 与外层证据；
- 纯函数 `RouteMembershipStrategy`；
- domain sentinel error：事实、policy、evaluation instant 与 future fact 的非法类别。

实现入口：

- [会员事实模型](../../../internal/lottery/domain/membership_fact.go)
- [具体路由 policy](../../../internal/lottery/domain/membership_routing_policy.go)
- [纯路由与一跳 path](../../../internal/lottery/domain/membership_routing.go)
- [domain 错误契约](../../../internal/lottery/domain/membership_routing_errors.go)

### 2.2 application 层

当前 Lottery application 已形成：

- consumer-owned `MembershipTierFactReader`；
- 受控 `MembershipRoutingClock` 与函数 adapter；
- `MembershipStrategyRoutingService`；
- 正的 `maxFactAge` 配置；
- 固定的输入校验、pre-cancel、Clock、reader、fact 校验、freshness、纯 domain route 顺序；
- application 返回前对 `decision.Confirmed()` 的内部 invariant check；
- caller cancellation 优先；
- provider read error 的稳定分类；
- 内部决定不一致时的 `ErrMembershipRoutingDecisionInvalid`；
- `MembershipTierFactReadError` 的一个公开 class + 显式 `Cause()` 诊断通道；
- nil、typed-nil、partial service 配置防御。

实现入口：

- [会员路由应用服务](../../../internal/lottery/application/membership_routing.go)
- [安全读取错误边界](../../../internal/lottery/application/membership_routing_error.go)

### 2.3 测试证据

当前测试覆盖：

- fact/policy 构造和 metadata canonicality；
- exact branch、reason、target 和 path；
- stable literal code；
- 两个 branch 的 target 收敛时仍保留 branch；
- path 防御性复制；
- `Confirmed()` 对 branch-reason 配对以及 path rule/branch/target 一致性的拒绝；
- UTC 与重复确定性；
- exact max-age 与 ±1ns 边界；
- fact+error 时 error 胜出；
- caller deadline 与 provider deadline 区分；
- explicit `Cause()` 不进入 `errors.Is` tree；
- 无效输入和错误配置在依赖前失败；
- pre-cancel、clock 后取消、reader 后取消和阻塞 reader 取消；
- 每次有效请求 Clock/reader 各一次；
- 64 goroutine 的只读一致结果；
- AST 级跨上下文 import、generic type、Rule/Tree/Engine 名称和 `map[string]any` 停止线。

测试入口：

- [事实测试](../../../internal/lottery/domain/membership_fact_test.go)
- [policy 测试](../../../internal/lottery/domain/membership_routing_policy_test.go)
- [路由决定测试](../../../internal/lottery/domain/membership_routing_test.go)
- [应用服务测试](../../../internal/lottery/application/membership_routing_test.go)
- [错误边界测试](../../../internal/lottery/application/membership_routing_error_test.go)
- [架构停止线测试](../../../internal/lottery/application/membership_routing_architecture_test.go)

### 2.4 当前明确没有什么

当前没有实现或证明：

- 真实会员目录、CRM、付费会员或用户中心 adapter；
- provider endpoint、credential、mTLS、签名、网络 ACL 或 source allowlist；
- 数据库会员表、routing policy 表、Redis key、Migration 或缓存失效；
- HTTP route、DTO、status、header、公开错误 envelope；
- runtime composition、配置文件、Compose 服务或 readiness；
- Activity 对 route policy / Strategy 的发布版本绑定；
- Strategy repository lookup、目标存在性、发布态或业务版本校验；
- `WeightedSelector` 调用、随机票据、Award、Draw、Result 或发奖；
- Participation 新用户/风险 chain 与 Lottery Route 的在线编排；
- 真实 session、Principal、RBAC/ABAC、tenant、resource/action/scope；
- React 页面、导航、路由、按钮裁剪或浏览器 E2E；
- 持久化决策 trace、OpenTelemetry span、生产 metrics、dashboard 或告警；
- 规则树 schema、节点表、边表、发布流程或通用决策引擎；
- 线上容量、可用性、准确率、公平性或合规 SLO。

这张“没有什么”列表不是谦虚措辞。它规定本节的证据边界：测试 stub 可以证明 application 怎样消费一个端口，不能证明现实中的 provider 有权威性；非零 Strategy ID 可以证明目标形状，不能证明目标存在；64 并发单测可以证明服务没有明显共享请求态，不能证明线上 provider 容量。

## 3. 先划所有权，再讨论类型

### 3.1 五类所有权

| 对象 | 事实/决定所有者 | 本节怎样消费 | 本节绝不能做 |
| --- | --- | --- | --- |
| 会员等级生命周期 | 外部会员 authority | 读取最小快照 | 创建、升级、降级或解释会员权益 |
| 外部 payload 到内部快照 | 未来 adapter / 防腐层 | 端口约束目标 shape | unknown 自动映射 standard |
| tier 到 Strategy 映射 | Lottery | 持有具体 policy | 交给会员 authority 返回 target |
| 新用户、风险、次数资格 | Participation | 本节不读取 | 把 Route 当第三个资格 gate |
| Activity 发布与版本绑定 | Marketing | 第 30 节以后引用 | 本节提前声明 target 已发布 |
| 调用主体与访问权 | 身份/Governance | 第 31～35 节以后建立 | 把会员 tier 当 role |

“同一个 Go 单体里可以直接 import”不是所有权证明。模块化单体仍然要回答：这个字段由谁形成、谁能改、谁能解释、谁在失败时负责。

### 3.2 为什么 provider 只返回 tier，不返回 Strategy ID

如果会员 provider 直接返回 `strategy_id=200`，调用代码会很短，但产生四个长期问题：

1. 外部系统必须认识 GrowthOS Lottery 内部资源；
2. Strategy 重建、归档或发布会迫使外部协议同步；
3. provider 配置错误可以绕过 Lottery policy；
4. 事后无法区分“会员事实是什么”与“当时 Lottery 怎样解释该事实”。

正确分离是：

```text
membership authority:
  subject 42 is premium
  observed at T
  source/revision = ...

Lottery:
  under policy route-v1,
  premium selects premium_override,
  target Strategy 200
```

这让同一份会员事实可以在不同 Lottery policy snapshot 下产生不同业务 Route，而不需要篡改原事实。当前确定性依赖完整 snapshot（revision + 两个 targets）；revision 字符串还没有被 registry/publisher 强制绑定唯一内容。

### 3.3 `MembershipSubjectRef` 为什么是 Lottery 本地类型

代码在 `membership_fact.go` 中定义 `MembershipSubjectRef uint64`，并在注释中明确“拥有它不证明 caller identity 或 access”。这项小决定阻止了三个偷换：

- 把外部用户 ID 直接当登录身份；
- 把 Participation 的 `ParticipantRef` 当跨上下文全局 ID；
- 把知道某个 ref 当成有权查询或代抽。

opaque ref 只回答“reader 用哪个键查事实”。它不回答：

- HTTP caller 是谁；
- caller 与 subject 是否同一人；
- caller 是否可代理操作；
- subject 属于哪个 tenant；
- 是否有权查看 premium branch；
- 是否有权调用某个 Lottery Strategy。

这些问题必须等真实 Principal 与服务端授权出现后处理。

### 3.4 Strategy ID 仍然只是目标身份

`MembershipStrategyRoutingPolicy` 接受两个非零 `StrategyID`，但没有调用 repository 验证：

- ID 是否存在；
- Strategy 聚合是否合法；
- 是否启用或已发布；
- 当前 Award/Weight 是哪一版本；
- Activity 是否引用它；
- caller 是否有权读或执行它。

这是刻意停止，而不是漏写一行 repository call。当前 Strategy 聚合没有独立业务 version，本节若把 `StrategyID` 写成“Strategy snapshot/version”会伪造证据。目标引用完整性要等第 28 节规则树 schema 和第 30 节 Activity 发布绑定有真实生命周期后再收敛。

### 3.5 Route、Eligibility、Authorization、Selection 的边界

| 决定 | 回答的问题 | 当前结果类型 | 本节 Route 成功能否替代 |
| --- | --- | --- | ---: |
| Participation eligibility | 该业务主体是否满足参与条件 | eligible/ineligible/error | 否 |
| Authorization | caller 能否对资源执行动作 | allow/deny/unauthenticated | 否 |
| Lottery route | 后序应该使用哪个 Strategy target | branch + target | 是，本节只做这个 |
| Weighted selection | 固定 Strategy 候选中选中哪个 Award | reward/no_reward/error | 否 |
| Draw finalization | 唯一参与是否形成可查询结果 | result/processing/unknown | 否 |
| Benefit delivery | 权益是否受理并兑现 | accepted/success/failure | 否 |

Route 成功只说明“如果上游已经合法进入 Lottery，应该加载哪个 Strategy”。它不自动提供上游合法性，也不产生任何下游副作用。

## 4. 产品语义：default 为什么不是 fallback

### 4.1 这是一项本节新增决定

第 23 节只写“会员分层可能路由到不同 Strategy”，没有写 literal tier 或具体目标。本节选择：

- confirmed `premium` -> `premium_override`；
- confirmed `standard` -> `baseline_default`；
- 其余 -> no Route。

因此这项映射必须被标为“第 27 节新增产品决定”，不能声称代码只是机械实现一条早已完整定义的需求。真实项目中，补全一个开放需求与实现既有精确规格是两种不同变更，审批与回滚责任也不同。

### 4.2 四个词不能混用

| 词 | 类型 | 由谁确认 | 是否可路由 |
| --- | --- | --- | ---: |
| `standard` | 受支持会员等级事实 | 外部 authority | 是，走 baseline default |
| `premium` | 受支持会员等级事实 | 外部 authority | 是，走 premium override |
| `unknown` | 无法确认受支持事实的状态 | 当前系统无法确认 | 否 |
| `default` | Lottery policy 的显式基线边 | Lottery | 只对 confirmed standard |

最危险的错误是：

```go
target, ok := mapping[tier]
if !ok {
    target = defaultTarget
}
```

这段代码会把拼写错误 `premuim`、未来等级 `platinum`、adapter 映射缺陷、空字符串甚至被攻击者构造的值都当作 standard。它不是“高可用”，而是在证据不足时制造确定业务结论。

当前代码先由 `MembershipTierFactSnapshot.Validate` 封闭 tier 词汇，再由 `RouteMembershipStrategy` 使用显式 `switch`，而不是 map miss fallback。这是 default 安全性的核心。

### 4.3 为什么 standard 走 `baseline_default`，而不是 `standard` 分支

本节希望证明两件事：

1. 存在一个专用 override；
2. 存在一个产品批准的缺省基线。

如果把两个出口都写成精确等值边：

```text
standard -> standard_target
premium  -> premium_target
```

虽然仍有多出口，却没有证明 default 的语义。让 standard 走 `baseline_default` 表达的是：

> 对当前已确认且受支持、又没有专用 override 的基线会员，Lottery 有一条必填默认边。

注意这不意味着未来任何新 tier 自动命中 baseline。新 tier 是否属于 default 适用集合仍需要新 policy revision 和产品决定。

### 4.4 branch、reason 与 target 为什么分开

当前决定同时记录：

- rule：`lottery.membership_tier.route_strategy`；
- branch：`premium_override` / `baseline_default`；
- reason：`premium_strategy_selected` / `baseline_strategy_selected`；
- target：Strategy ID。

它们回答不同问题：

| 字段 | 回答 |
| --- | --- |
| rule code | 哪个业务决定节点执行了 |
| branch code | 走了哪条出边 |
| reason code | 稳定解释是什么 |
| target | 后序明确加载哪个资源 |

若只记录 target，当两个 branch 在迁移期都指向 Strategy 100 时，无法判断为何到达 100；若只记录 branch，后序仍不知道加载哪个 Strategy；若把 target 塞进 reason，reason 就不再是稳定低基数语义。

### 4.5 为什么允许两个 branch 暂时指向同一 target

`MembershipStrategyRoutingPolicy.Validate` 只要求两个 target 非零，不要求互异。这个选择支持：

- 灰度前两个等级暂时共用基线；
- premium Strategy 回滚期间临时收敛；
- 配置发布先建立 branch identity，再切 target；
- 测试仍能证明 branch 语义不因目标相同而消失。

`TestMembershipStrategyRoutingPolicyAllowsConvergingTargets` 与 `TestRouteMembershipStrategyKeepsBranchEvidenceWhenTargetsConverge` 明确保护这个行为。

强制 target 不同看似更能展示“多出口”，却把一个测试 fixture 偏好变成了没有产品依据的永久业务不变量。验收时应使用不同 target 证明多出口；模型本身不必禁止合法收敛。

## 5. 为什么线性 chain 不够

### 5.1 chain 的数学结构是有序合取

第 26 节 chain 可以理解为：

```text
g1 AND_THEN g2 AND_THEN ... AND_THEN gn
```

每个 `gi` 只有：

```text
continue to i+1
or stop
```

其拓扑在构造时就由 slice 顺序固定。节点不选择 edge，也不返回 terminal target。

会员路由则是：

```text
if premium  -> target P
if standard -> target B
```

两个结果都是 confirmed success，但 terminal target 不同。任何只返回 `continue/reject` 的协议都丢失一个必要维度。

### 5.2 五种“硬扩 chain”方式及其坏味道

#### 把 target 塞进 reason

```text
eligible(reason = strategy_200)
```

问题：

- eligibility reason 被迫承载 Lottery resource；
- Participation 拥有了不属于它的 Strategy 映射；
- reason 基数随 target 增长；
- 调用方开始解析字符串控制流。

#### 把 outcome 扩成 `continue_premium` / `continue_standard`

问题：

- “continue”不再指固定下一项；
- runner 必须认识所有业务 tier；
- 每新增分支都修改通用 protocol；
- chain 已经变成未命名图，却没有图校验。

#### step 返回 next-index

问题：

- slice index 变成隐式 edge；
- 越界、循环、跳过和缺省不再显式；
- path 只能事后猜；
- 数据库持久化时 index 稳定性极差。

#### 在 handler 外加 `if/else`

问题：

- HTTP、MCP、job 各复制一份 policy；
- freshness 与错误边界留在 service，route 却散落 transport；
- default 与 trace 无法统一；
- handler 变成事实和决定所有者。

#### 复制 premium/standard 两条 chain

问题：

- 公共 gate 重复；
- ruleset revision 漂移；
- 分支合流时更难表达；
- 无法证明哪条 path 真正执行。

### 5.3 “chain 不够”不等于“chain 错了”

Participation chain 仍然准确回答自己的问题。第 27 节没有重写、废弃或泛化它。真实架构演进应允许：

```text
Participation prerequisite chain
  -> confirmed eligible
  -> Lottery membership router
  -> confirmed Strategy target
  -> later Strategy load / selection
```

未来编排者负责顺序消费这些决定，但不获得各自事实所有权。把一个局部模式留在它仍然成立的上下文，比为了“统一架构”把所有决定压进一个 Engine 更健康。

### 5.4 代码如何形成可执行证据

第 27 节没有只在文档里说“chain 不够”，而是让新模型出现 chain 无法承载的字段：

- `MembershipRoutingBranch`；
- `StrategyID target`；
- `MembershipRoutingPathStep`；
- branch-specific reason；
- policy 中的两个 terminal target。

`TestRouteMembershipStrategyExposesBranchSpecificTerminalPaths` 用 standard/premium 两个 fixture 证明两条成功路径；架构测试又保证没有把这些概念反向塞入 Participation 或通用 Rule 类型。

## 6. 为什么还不急着上规则树

### 6.1 当前真实拓扑只有一个决定点

现在的可执行形状是：

```text
             premium_override  -> Strategy P
membership
decision
             baseline_default  -> Strategy B
```

它有：

- 一个固定 decision node；
- 两条固定出边；
- 两个 terminal Strategy ID；
- 一条必选 default；
- 没有子决策；
- 没有共享子路径；
- 没有合流；
- 没有循环；
- 没有运行时编辑；
- 没有发布/回滚；
- 没有 schema version。

用显式 domain type 和 `switch` 正好能表达。此时引入 node table、edge table、JSON expression、operator registry 或 engine context 会先制造技术自由度，再寻找业务填充。

### 6.2 树会立即带来的新问题

一旦声明“规则树”，至少必须回答：

1. root 怎样唯一；
2. node identity 是否稳定；
3. edge 顺序是否有意义；
4. default 是否每个 decision node 都必填；
5. 一个输入命中多条 edge 时采用什么 hit policy；
6. terminal target 怎样做引用完整性；
7. 是否允许循环；
8. 最大深度与节点数是多少；
9. 不可达节点是否拒绝发布；
10. 未知 operator 怎样处理；
11. schema version 怎样升级；
12. draft/published/retired 怎样流转；
13. 老执行器能否读新 schema；
14. 发布中途失败怎样回滚；
15. path 怎样跨多步保存；
16. evaluator 的时间、I/O 与步数预算是多少。

第 27 节没有这些事实。提前写 schema 不会消灭问题，只会把未经证明的答案固化为迁移成本。

### 6.3 当前抽象为什么停在 concrete router

代码没有导出：

- `Rule`；
- `RuleChain`；
- `RuleTree`；
- `RuleNode` / `RuleEdge`；
- `RuleEngine` / `DecisionEngine`；
- `EvaluationContext`；
- `RulePriority`；
- `DSL`；
- 泛型规则类型；
- `map[string]any` 事实袋。

`TestLesson27MembershipRoutingKeepsContextOwnershipAndEngineStopLine` 使用 Go AST 检查 Lottery/Participation 的生产文件：

- 项目内 import 只能指向被允许的本上下文 domain；
- 禁止上述通用类型名；
- 禁止 type parameter；
- 禁止 `map[string]any` 或 `map[string]interface{}`。

这项测试不是说这些技术永远错误，而是防止本节在没有证据时无声扩张。

### 6.4 第 28 节真正要得到的新证据

下一节只有在出现“运营需要把当前一跳拓扑作为版本化配置保存、发布前验证并引用”时，才有理由引入最小树 schema。那时 schema 的字段应从本节已经真实存在的概念生长：

- stable decision identity；
- branch identity；
- default edge；
- terminal Strategy target；
- policy/schema revision；
- path；
- 引用完整性。

不是从某个通用规则引擎的类图倒推业务。

## 7. 数据模型：每个字段都要回答一个问题

### 7.1 `MembershipTierFactSnapshot`

| 字段 | 回答的问题 | 为什么需要 | 为什么还不够 |
| --- | --- | --- | --- |
| subject ref | 这份事实属于谁 | 阻止错主体路由 | 不是 Principal |
| tier | authority 确认了什么等级 | 路由输入 | 只支持 v1 两值 |
| observed-at | authority 何时形成快照 | future/freshness | 不证明历史 as-of 查询 |
| source | 哪个受控来源 | provenance | string 自身不证明权威 |
| revision | 哪一份来源快照 | 回放/诊断 | 语义由 provider 定义 |

所有字段私有，通过构造器和只读 getter 暴露。这样 adapter 或测试不能构造一半合法、一半未验证的公开 struct；即使同包手工构造，`Validate` 仍可二次检查。

### 7.2 为什么 tier 是封闭字符串枚举

`MembershipTier` 底层是 string，便于外部防腐映射，但 domain 只接受两个 literal：

- `standard`；
- `premium`。

好处：

- 对外协议易映射；
- stable literal 易测试；
- unknown 不需要额外“合法枚举”占位；
- future tier 会失败关闭。

代价：

- 新 tier 必须发布代码或后续 schema；
- adapter 不能透传任意 provider 字符串；
- 大小写、空格和本地化名称都必须在边界前规范化或拒绝。

`FuzzMembershipTierFactSnapshotNeverAcceptsUnsupportedTier` 用 fuzz 保护“只有两个 literal 可接受”，防止未来宽松校验把随机输入变成 baseline。

### 7.3 source/revision 为什么限制 canonical metadata

`validateMembershipMetadataToken` 要求：

- 非空；
- UTF-8 有效；
- 首尾无空白；
- source 最多 128 bytes；
- revision 最多 256 bytes；
- 所有 rune 可打印。

这不是完整的安全编码或 provider allowlist，却能减少：

- 控制字符污染日志；
- 无界 metadata 造成内存/存储放大；
- 首尾空白导致同一版本出现多个 spelling；
- 无效 UTF-8 穿过边界。

仍需注意：`unicode.IsPrint` 允许很多可打印字符，不能替代 source 注册表、混淆字符治理、日志转义或访问控制。本节只保证最小 canonicality。

### 7.4 `MembershipStrategyRoutingPolicy`

policy 只包含：

- 独立 revision；
- premium target；
- baseline/default target。

它不包含：

- Activity ID；
- effective time；
- creator/approver；
- draft/published 状态；
- Strategy version；
- JSON expression；
- arbitrary conditions；
- permission；
- fallback retry。

这是“代码拥有的一份具体 v1 policy”，不是运营规则配置平台。`NewMembershipStrategyRoutingPolicy` 与 `Validate` 保证 revision canonical、target 非零；target 是否存在由后续生命周期负责。

还必须精确理解 revision：当前构造器接受任意 canonical revision 字符串，没有 registry、repository 或 publisher 保证：

```text
revision "route-v1" -> 永远唯一对应 premium=200, baseline=100
```

调用方仍可能用同一个 `route-v1` 构造另一组 targets。于是当前确定性命题只能写成：

> 同一份完整 policy snapshot（revision + premium target + baseline target）、同一事实快照和同一 evaluated-at，得到同一 Route。

不能缩写成“同一 revision 必然同一 Route”。revision 到内容的唯一绑定、不可变发布和引用完整性留给第 28/30 节。

### 7.5 `MembershipStrategyRouteDecision`

成功决定包含：

- target；
- rule code；
- branch；
- reason；
- policy revision；
- fact source/revision；
- evaluated-at；
- path slice。

`Confirmed()` 不只看 target 非零，而是检查全部核心证据：

- rule code 必须为固定值；
- branch 必须属于两个已知值；
- reason/revision/source/revision 非空；
- evaluated-at 非零；
- path 长度恰好为一。
- `premium_override` 必须配 `premium_strategy_selected`；
- `baseline_default` 必须配 `baseline_strategy_selected`；
- path step 的 rule/branch/target 必须与外层 decision 完全一致。

`TestMembershipRouteDecisionConfirmedRejectsInconsistentEvidence` 在 domain 同包测试里分别伪造 path target、path branch、path rule 和 branch-reason 配对，确保 `Confirmed()` 拒绝。

application 在 domain route 返回后还会显式断言 `decision.Confirmed()`；若内部实现违约，返回零 decision + `ErrMembershipRoutingDecisionInvalid`。这是一条防御性不变量边界，不是正常用户/会员错误，也不能 fallback。

即便如此，后续若引入反序列化/数据库重建，仍需专门构造器和完整 schema 校验，不能因为当前私有字段与 `Confirmed()` 就假设跨进程数据天然可信。

### 7.6 一跳 path 为什么仍使用 slice

当前 path 永远一跳，直觉上可以只放一个 struct。但使用只读 slice 有两个目的：

1. 调用方已经能按“有序路径”消费，不必在第 29 节多步执行时重新定义概念；
2. `Path()` 的防御性复制可以立即验证证据不可由调用方改写。

它不是提前实现树。path step 只有 rule、selected branch、target，没有 node registry、edge condition、parent、children 或 generic payload。

代价是当前有一次极小 slice 分配与复制。与一次外部会员事实读取相比可以忽略；若未来高吞吐 profile 证明它显著，再考虑固定数组、value step 或延迟 materialization，不能凭感觉提前优化。

## 8. 纯 domain route：为什么把 I/O 与决定分开

### 8.1 纯函数的契约

`RouteMembershipStrategy(policy, fact, evaluatedAt)`：

1. 再验证 policy；
2. 再验证 fact；
3. canonicalize evaluated-at 为 UTC、去 monotonic component；
4. 拒绝零时刻；
5. 拒绝 future fact；
6. 用显式 switch 选择 branch/reason/target；
7. 构造一跳 path 与 immutable decision。

它不读取：

- context；
- clock；
- repository；
- Redis；
- HTTP；
- Participation；
- 随机源。

这种分离让业务映射可以在没有 adapter 的情况下确定性测试，也让 application 负责 I/O 生命周期、freshness 与 cancellation。

### 8.2 为什么 domain 仍检查 future

application 已在调用 domain 前检查 future，domain 再检查一次看似重复。保留二次检查的理由：

- domain 函数是公开包 API，可能被其他 application 调用；
- future 是 Route 不变量，不只是当前 use case 的防御；
- 纯函数自己必须拒绝不可信组合；
- 二次检查成本为常数比较。

但 freshness 没有放进 domain，因为 max age 是 application use case 的依赖配置；domain 只确认“事实不晚于本次评估”，application 决定“多旧仍可接受”。

### 8.3 为什么显式 `switch` 比 map 更诚实

`switch` 同时完成两件事：

- 明确列出每个受支持 tier；
- default case 返回错误，而不是选择 baseline。

即使 fact `Validate` 已经保证 tier，这个 default 仍保护未来维护者：若新增 enum literal 却忘记更新 router，系统失败关闭，而不是自动走 standard。

### 8.4 零决定是失败路径唯一合法值

domain 的所有 error path 都返回 `MembershipStrategyRouteDecision{}`。测试会逐字段检查：

- target 0；
- rule/branch/reason 空；
- revisions/source 空；
- evaluated-at zero；
- `Confirmed()==false`；
- path 长度 0。

这避免调用方出现：

```go
decision, err := route(...)
if err != nil {
    log.Warn(err)
}
use(decision.Target()) // 半成品或 fallback
```

当 error 非空时，没有任何可消费 target。

## 9. 单一时间：把“现在”变成可解释输入

### 9.1 application 为什么先读 Clock，再读 fact

当前顺序为：

```text
validate input/policy/service
  -> pre-cancel
  -> clock.Now exactly once
  -> cancel check
  -> reader.FindMembershipTierFact
  -> cancel check
  -> read error / fact validation
  -> freshness
  -> cancel check
  -> pure domain route
  -> cancel check
  -> return
```

Clock 在 reader 前捕获，使 evaluated-at 表示“这次决策开始观察外部事实时的逻辑基准”。若 reader 返回的 `observedAt` 晚于它，则这份快照属于本次 as-of 之后，当前求值失败关闭，留给下一次求值。

### 9.2 为什么只调用一次 `Now()`

如果读取前、读取后各读一次：

- freshness 可能用第二个时刻；
- decision trace 可能记录第一个时刻；
- 慢 reader 会让同一 fact 在不同判断中得到不同年龄；
- 测试和回放无法回答究竟用哪个“现在”。

`TestMembershipStrategyRoutingServiceCapturesClockExactlyOnce` 让 reader 返回后把 stub clock 推进 24 小时。如果 service 重读时钟，原本新鲜的 fact 会变 stale，或 evidence 漂移；测试要求仍使用初始时刻且 Clock 只调用一次。

### 9.3 canonical UTC 与 monotonic component

domain/application 都使用：

```go
value.UTC().Round(0)
```

`UTC()` 统一 location；`Round(0)` 去掉 Go `time.Time` 可能携带的 monotonic reading，使跨结构比较、深比较和未来持久化语义更稳定。

这不决定业务时区。会员 fact freshness 是 instant 差值，不是“某地自然日”。未来 Activity 时间窗仍要有自己的业务时区规则。

### 9.4 freshness 的纳秒边界

application 采用：

```go
if evaluatedAt.Sub(fact.ObservedAt()) > maxFactAge {
    stale
}
```

因此：

| age | 结果 |
| ---: | --- |
| `maxAge - 1ns` | valid |
| `maxAge` | valid |
| `maxAge + 1ns` | stale |
| negative | future -> invalid |

`TestMembershipStrategyRoutingServiceEnforcesFreshnessNanosecondBoundary` 精确覆盖 exact max、+1ns stale、future +1ns、错主体和 zero fact。边界必须写成测试，因为 `>=` 与 `>` 的一字符差异会改变产品语义。

### 9.5 单一 as-of 不提供什么

当前 as-of 不保证：

- provider 能按历史时刻查询；
- reader 返回的是 clock 之前已存在的快照；
- policy 与 fact 来自同一个原子事务；
- 多个系统共享时钟同步；
- 网络延迟不会让 fact 在读取期间变化；
- 下一步 Strategy 加载使用同一业务快照；
- 正式 Draw 能回放完整历史状态。

它只保证：本次内核使用一个服务端控制的逻辑时刻来拒绝 future/stale 并记录证据。跨 authority 一致性要靠未来 adapter 契约、版本水位、发布引用和正式结果快照继续解决。

## 10. cancellation：停止工作价值，而不是制造业务结论

### 10.1 当前优先级

`MembershipStrategyRoutingService.Route` 在多个边界观察 caller context：

1. 输入与配置合法后、任何依赖前；
2. Clock 返回后；
3. reader 返回后、读取 error 分类前；
4. fact/freshness 校验后、domain route 前；
5. domain route 返回后、最终交付 decision 前。

优先级可以概括为：

```text
if caller cancellation has been observed:
    return zero decision + caller context error
else if dependency/contract failed:
    return zero decision + stable application error
else:
    return confirmed route
```

这样 caller 已经放弃请求时，不会把几乎同时发生的 provider error 误报为系统故障，也不会交付一个调用方已经没有承诺继续使用的 Route。

### 10.2 pre-cancel 为什么必须零调用

`TestMembershipStrategyRoutingServiceMakesObservedCallerCancellationWin` 的第一段先取消 context，再调用 Route，并断言：

- 返回 `context.Canceled`；
- Clock 0 次；
- reader 0 次；
- decision 全零。

这既是性能要求，也是隐私要求：一个已经没有处理价值的请求不应继续访问会员 authority。

### 10.3 Clock 后取消为什么 reader 必须零调用

Clock interface 没有 context 参数，所以它返回后必须立刻重新检查 caller。测试让 `afterNow` 回调取消 context，并断言：

- Clock 1 次；
- reader 0 次；
- caller cancellation 胜出。

否则系统会在已知调用取消后继续读取会员事实。

### 10.4 reader 后取消为什么胜过 provider error

测试让 reader 同时：

- 返回一个 provider failure；
- 在返回前取消 caller context。

service 在分类 provider error 前检查 `ctx.Err()`，因此返回 caller cancellation，provider failure 不进入公开控制流。这项顺序必须显式，因为把 error 分类放在 context 检查前会得到相反结果。

### 10.5 阻塞 reader 测试证明的只是“返回后观察”

`TestMembershipStrategyRoutingServiceObservesCancellationAfterBlockingReaderReturns` 启动 goroutine，reader 阻塞，caller 取消后再释放 reader。最终 service 返回 canceled。

这个测试**没有证明 reader 可被即时中断**。stub 故意忽略传入 context，service 也无法强制终止同步函数。真实 adapter 必须：

- 把 context 传给 HTTP/SQL/RPC client；
- 配置自己的连接/读取 timeout；
- 在 provider 不响应时及时返回；
- 避免 goroutine 泄漏。

同理，`MembershipRoutingClock.Now()` 若自身阻塞，service 只能等它返回后观察取消。生产 Clock 通常是本地快速调用，但接口契约没有提供强制超时。

### 10.6 为什么 Route 成功前还有最后一次 cancellation check

domain route 是纯本地 O(1) 计算，通常不会长时间运行。最后一次 check 的价值是让“reader 返回后、domain route 期间”被观察到的 caller cancellation 仍可阻止决定交付。

代价是存在经典不可消除竞态：

```text
last ctx.Err() == nil
caller cancels immediately after
function returns confirmed decision
```

context cancellation 从来不是事务回滚。调用方仍需接受“完成与取消同时发生”可能返回完成结果；本节没有副作用，所以最坏只是一个未被使用的 Route。未来 Draw 有副作用时，必须靠业务 identity 查询结果，不能靠 context 判断是否发生。

## 11. 错误语义：一个安全 class，一个显式 Cause

### 11.1 application 稳定错误目录

| error class | 含义 | 是否调用 reader | 是否可以命中 default |
| --- | --- | ---: | ---: |
| `ErrMembershipRoutingInvalidArgument` | nil context、zero subject、非法 policy | 否 | 否 |
| `ErrMembershipRoutingNotConfigured` | service/reader/clock/freshness 配置不完整 | 否 | 否 |
| `ErrMembershipRoutingClockInvalid` | Clock 返回 zero instant | 否 | 否 |
| `ErrMembershipRoutingDecisionInvalid` | domain 返回不完整或自相矛盾的内部决定 | 是 | 否 |
| `ErrMembershipTierFactNotFound` | authority 无快照 | 是 | 否 |
| `ErrMembershipTierFactUnavailable` | provider 无法回答 | 是 | 否 |
| `ErrMembershipTierFactReadFailure` | 未分类读取失败 | 是 | 否 |
| `ErrMembershipTierFactInvalid` | zero/unsupported/corrupt/mismatch/future | 是 | 否 |
| `ErrMembershipTierFactStale` | fact 超过 max age | 是 | 否 |
| caller context error | caller 取消或 deadline | 视观察点 | 否 |

注意：`not found` 不是 “standard”；`unavailable` 不是“拒绝”；`stale` 不是“沿用旧 Route”；任何错误都不能选择 baseline。

### 11.2 provider deadline 与 caller deadline 为什么不同

两者底层都可能包含 `context.DeadlineExceeded`，但业务控制流不同：

- caller deadline：请求方给的整体预算已耗尽，应原样返回 caller context error；
- provider deadline：caller 仍愿意等待，但会员 authority 自己超时，应分类为 dependency unavailable。

`TestMembershipStrategyRoutingServiceClassifiesProviderDeadlineWhileCallerLives` 使用 `context.Background()` 作为活跃 caller，让 reader 返回包裹的 `DeadlineExceeded`。结果：

- public `errors.Is(err, ErrMembershipTierFactUnavailable) == true`；
- public `errors.Is(err, context.DeadlineExceeded) == false`；
- 显式 `Cause()` 仍能识别 provider deadline。

如果 raw cause 自动进入 `errors.Is` tree，上层可能把 provider 故障误当 caller 取消，错误地跳过告警或重试决策。

### 11.3 为什么不实现 `Unwrap()`

`MembershipTierFactReadError` 保存：

- `class`：受评审、稳定、低基数的应用语义；
- `cause`：受信诊断代码显式读取的原始 provider error。

它实现：

- `Error()`：只渲染安全 class；
- `Is(target)`：只匹配一个 class；
- `Cause()`：显式取原始诊断；
- 不实现 `Unwrap()`。

这意味着：

```go
errors.Is(err, ErrMembershipTierFactUnavailable) // true
errors.Is(err, rawProviderError)                 // false
errors.Unwrap(err)                               // nil
readErr.Cause()                                  // rawProviderError
```

设计目标不是隐藏所有诊断，而是要求调用者明确选择受信通道，避免通用 error tree 把 SQL、endpoint、外部类别或敏感 payload 传播到 transport 控制流。

### 11.4 exactly one public class

`TestMembershipTierFactReadErrorExposesExactlyOneSemanticClass` 故意构造：

```text
public class = unavailable
cause        = not found
```

断言只有 unavailable 能被 `errors.Is` 匹配，not-found cause 只能经 `Cause()` 获取。否则一个 error 同时属于两个互斥语义，上层 switch 的匹配顺序就会决定行为。

这个“恰好一个 class”保证针对的是 `MembershipTierFactReadError` 的 provider read wrapper。service 在本地验证一个无 error 但结构非法的 fact 时，会以 application invalid class 包裹 domain validation error；那是内部契约诊断，不应误写成“整个系统任意 error tree 永远只有一个 sentinel”。

### 11.5 unknown class 为什么收敛到 read failure

`WrapMembershipTierFactReadError` 若收到未评审 class，会降为 `ErrMembershipTierFactReadFailure`。zero value 或 typed-nil wrapper 的 `Error/Is/Cause` 也稳定失败关闭。

这防止 adapter 随意发明新 class 后影响上层控制流。新增公共类别必须修改受控 allowlist 与测试，而不是把任何 error 当作 class。

### 11.6 fact 与 error 同时返回时为什么 error 胜出

Go reader 端口允许返回 `(fact, error)`。错误 adapter 可能返回：

```text
fact = valid-looking standard
err  = provider contract failure
```

如果 service 先用 fact，就会让一个明确失败的 provider 响应命中 baseline default。`TestMembershipStrategyRoutingServiceMakesFactErrorWinReturnedFact` 和 provider payload contract tests 强制：

- error 胜出；
- fact 完全不可观察；
- decision/path 全零；
- cause 留在显式通道；
- public error text 只有应用 class。

### 11.7 domain payload error 的边界

未来 adapter 若无法把 provider payload 映射成合法 tier，可以用 domain contract sentinel 作为 cause。`classifyMembershipTierFactReadError` 将其映射为 application `ErrMembershipTierFactInvalid`，wrapper 隔离 raw domain/provider chain。

这区分：

- domain 说“这个 fact shape 不合法”；
- application 说“本次读取没有得到可用会员事实”；
- transport 未来决定怎样安全映射公开响应。

本节没有 transport，所以不能声称已有 HTTP status 或用户文案。

## 12. fail-closed：关闭的是下游动作，不是把人判成失败

### 12.1 四种状态必须分开

| 状态 | 是否有可信 tier | 是否有 Route | error |
| --- | ---: | ---: | --- |
| confirmed standard | 是 | baseline target | nil |
| confirmed premium | 是 | premium target | nil |
| invalid/unknown/unavailable/stale | 否 | 无 | typed error |
| caller canceled | 不再关心 | 无 | context error |

这里没有 “unknown -> reject” 或 “unknown -> standard”。fail-closed 的意思是：

> 在没有可信 Route 时，不允许继续加载 Strategy 或消费随机票据。

它不是：

- 把用户永久判为非会员；
- 把技术故障记成业务拒绝；
- 自动给 baseline 以提高成功率；
- 吞掉错误返回空 target；
- 在后台偷偷重试并换一个 Route。

### 12.2 为什么 default 不能承担可用性降级

把 provider 故障 fallback 到 baseline 会产生：

- premium 用户在故障期间体验降级；
- 路由分布随 provider 健康状态变化；
- 故障被成功指标掩盖；
- 攻击者可通过制造超时获得另一路径；
- 事后没有证据区分真实 standard 与降级。

如果未来业务真的允许某类非关键增强在 provider 故障时降级，必须新增：

- 明确降级政策；
- 独立 revision；
- 风险评审；
- 用户/运营披露；
- 指标和告警；
- 不同 branch/reason；
- 不影响正式权益或公平性的证明。

不能复用当前 `baseline_default` 名称偷偷实现。

### 12.3 为什么失败不返回 partial path

假设 Clock/fact 已读，甚至 domain 已经算出 target，但 caller cancellation 在返回前被观察到。当前 service 丢弃 decision，返回零值。这样上层不需要猜：

- path 是诊断草稿还是业务承诺；
- target 能不能继续使用；
- error 与 decision 哪个优先。

技术诊断可以由 future observer 记录安全阶段信息，但业务返回只有两种：

```text
confirmed decision + nil
zero decision + error
```

### 12.4 fail-closed 的可用性代价

安全边界会放大会员 authority 故障对 Lottery 的影响。本节接受这个代价，因为：

- 路由决定影响后续概率/奖品集合；
- 错误 target 可能比暂时不可用更难补偿；
- 当前没有正式 Draw 或降级审批；
- unknown 与 standard 没有产品等价证据。

未来可用性优化应从 provider SLO、缓存一致性、预发布快照、熔断和明确降级政策入手，而不是把错误当默认分支。

## 13. path 与决定证据：解释选择，不复制整份事实

### 13.1 外层 decision 与 path step 的分工

外层 `MembershipStrategyRouteDecision` 保存：

- rule、branch、reason、target；
- policy revision；
- fact source/revision；
- evaluated-at；
- path。

一跳 `MembershipRoutingPathStep` 只保存：

- rule；
- selected branch；
- terminal target。

这样 path 表示控制流，外层 envelope 表示本次评估证据。policy/fact/as-of 不在每个 step 重复。第 29 节若出现多步 path，需要重新决定哪些证据是 evaluation-wide，哪些是 step-local。

### 13.2 为什么 path 不记录 subject ref

path 的问题是“怎样到达这个 target”，不是“这个人是谁”。不记录 subject 可以：

- 降低泄露和重识别风险；
- 避免 path 成为用户画像；
- 允许未来由受保护的正式业务 identity 关联，而不是复制身份；
- 降低日志误用。

但外部业务若要把 Route 关联到 Participation/Draw，仍需要由未来 orchestrator/结果模型保存受控引用。本节没有做这件事。

### 13.3 branch 本身仍是敏感派生信息

即使 path 不含 raw tier，`premium_override` 几乎可以推断主体是 premium。因此不能说“没有 PII 就没有隐私风险”。

当前控制：

- path 只在内核返回；
- 没有 HTTP DTO；
- 没有持久化；
- 没有日志/metrics 实现；
- 不含 subject ref；
- `Path()` 返回副本。

未来若 path 对运营或用户可见，需要访问授权、字段级披露、保留期和审计。

### 13.4 `Path()` 为什么防御性复制

decision 内部保存 slice。若 getter 直接返回内部 slice：

```go
path := decision.Path()
path[0] = zero
```

调用方就能改写历史证据，使 `Confirmed()` 与 path 内容分裂。当前 getter 使用：

```go
append([]MembershipRoutingPathStep(nil), decision.path...)
```

`TestMembershipRoutePathIsDefensivelyCopied` 修改第一次返回值，再读取一次，确认内部 branch/target 未变。

### 13.5 这还不是审计日志

当前 decision/path 缺少：

- 持久化 identity；
- append-only/tamper evidence；
- actor/Principal；
- Activity/Participation/Draw reference；
- application version；
- duration；
- adapter instance/provider request ID；
- retention；
- authorized reader；
- result linkage。

所以只能称“进程内决定证据”或“最小 path”，不能称合规审计、完整 lineage 或分布式 trace。

## 14. 隐私与数据最小化

### 14.1 reader 为什么只返回五项事实

路由只需要：

```text
subject match
tier
observed-at
source
revision
```

不需要：

- 姓名、手机、邮箱；
- 订单和消费金额；
- 成长值、积分余额；
- 会员到期权益明细；
- 支付渠道；
- 地址和设备；
- 角色/权限；
- 完整 provider JSON。

把完整 Profile 传进“通用 EvaluationContext”虽然省一次类型设计，却扩大了内存、日志、debug dump、测试 fixture 和越权面。

### 14.2 fact revision 可能是间接标识

revision 看起来是技术 metadata，但如果 provider 把 user ID、订单号、邮箱或可逆 token 编入 revision，它仍可能构成个人数据。

本节只能限制长度与可打印性，不能证明 revision 无敏感内容。真实 adapter 接入前必须定义：

- revision 格式；
- 是否全局唯一；
- 是否能反推 subject；
- 日志是否允许；
- 保留多久；
- 谁能查询；
- 是否需要哈希/映射。

### 14.3 source 也不能直接进普通指标

source 未来可能是环境、租户、provider 或区域组合，基数和敏感度都不确定。即使当前测试写死 `membership-directory`，也不能推导生产 source 可安全作为 metric label。

更稳妥的做法是：

- adapter 层将 provider 映射为受控低基数 ID；
- 普通 metrics 默认不带 source/revision；
- 受保护日志按采样和权限记录；
- 未知 source 在 adapter/配置阶段失败，而不是动态创建 label。

### 14.4 错误文本为什么不能包含 raw cause

provider error 可能包含：

- endpoint/IP；
- SQL；
- subject ref；
- 原始 tier payload；
- credential fragment；
- request header；
- 内部 topology。

`MembershipTierFactReadError.Error()` 只返回 class 的安全固定文案。显式 `Cause()` 仍然需要调用者自律：不能因为有这个方法就把 cause 直接写到普通日志或 HTTP response。

### 14.5 当前数据生命周期

当前 fact、decision、path 都是请求内 Go value：

- 没有数据库；
- 没有 Redis；
- 没有消息；
- 没有浏览器响应；
- 没有持久化日志实现。

这降低了当前数据留存风险，也意味着无法做历史追责。第 30 节以后若正式流程需要回放，必须单独设计保存哪些最小证据，而不是序列化全部 struct。

## 15. 威胁模型：攻击者会从 default、时间和边界下手

### 15.1 资产

本节相关资产包括：

- 正确的 Strategy target；
- 会员等级事实的机密性与完整性；
- routing policy 的完整性；
- path/branch 的最小披露；
- provider 可用性；
- future/stale 判断的时间完整性；
- 公开错误语义的稳定性；
- Lottery 与 Participation/Governance 的所有权边界。

### 15.2 信任边界

```text
future HTTP/browser
    -> future identity/auth boundary
    -> Lottery application
    -> MembershipTierFactReader port
    -> future adapter
    -> external membership authority

Lottery application
    -> pure Lottery domain route
```

当前只实现中间两层。端口存在不代表两侧安全链已完成。

### 15.3 主要威胁与当前控制

| 威胁 | 可能后果 | 当前控制 | 剩余风险 |
| --- | --- | --- | --- |
| 客户端伪造 premium | 进入高价值 Strategy | Route 不接收 tier 参数，只从 reader 取 fact | 尚无真实 transport/identity |
| adapter 把 unknown 映射 standard | 错误命中 baseline | 封闭 enum、invalid fact | adapter 尚未实现 |
| 重放旧 premium fact | 使用已失效等级 | observed-at + max age | provider revision/撤销语义未接 |
| provider 返回错主体 | 交叉用户路由 | subject equality check | subject 映射 authority 未证明 |
| provider 返回 future fact | 绕过时序 | single as-of + future reject | 分布式 clock skew 政策未定义 |
| policy target 被篡改 | 路由错误资源 | 私有值、构造校验、非零 ID | 无发布/签名/引用完整性 |
| 利用 miss 触发 default | unknown 获得 Route | Validate + explicit switch | future schema 仍需同约束 |
| 错误 cause 注入日志 | 泄密/日志注入 | safe Error、explicit Cause | 调用方仍可能误记 Cause |
| 大 metadata 消耗资源 | 内存/日志放大 | byte limit、printability | Unicode 混淆/source allowlist 未做 |
| 调用取消仍读会员事实 | 隐私与成本浪费 | 多点 cancellation check | 阻塞依赖若忽略 ctx 无法强停 |
| path 被调用方改写 | 审计证据漂移 | 私有字段、防御性复制 | 反序列化后校验未设计 |
| branch 进入普通 metrics | 泄露 premium 分布 | 文档停止线 | 尚无 telemetry enforcement |
| 会员 tier 被当 role | 越权 | 类型/架构边界、无 auth 字段 | 后续调用方仍需治理 |

### 15.4 source string 不是 authority proof

测试 fact 写 `source=membership-directory` 只能证明字符串被保存和验证。它不能证明：

- 请求真的发到受信 provider；
- 响应被签名；
- adapter 没被替换；
- source 与 credential 绑定；
- 数据未被中间人修改；
- provider 本身有权定义会员等级。

真实 authority 需要部署、credential、网络、组织和审计证据。本节只定义消费契约。

### 15.5 default 是安全敏感配置

baseline target 看起来“普通”，但仍可能：

- 包含奖品；
- 消耗库存；
- 改变中奖概率；
- 触发成本；
- 暴露 Activity。

所以 default target 同样需要后续发布审批、引用完整性和授权，不能被当作永远安全的万能兜底。

## 16. 可观测性：先定义可回答的问题，不伪造已经埋点

### 16.1 当前已经存在的是决定证据

成功 decision 可以在进程内回答：

- 使用哪条 rule；
- 选择哪条 branch；
- 给出哪个 target；
- 使用哪份 policy revision；
- fact 来自哪个 source/revision；
- evaluated-at 是什么；
- 一跳 path 是什么。

错误可以回答：

- 稳定 application class；
- caller cancellation 还是 provider failure；
- 受信代码显式查看 raw cause。

这是一套“可观测所需语义”，不是已实现 telemetry。当前代码没有 logger、meter、tracer 或 observer port。

### 16.2 未来普通 metrics 的低基数建议

若后续接入 metrics，可以考虑：

```text
lottery_membership_route_requests_total{
  rule,
  outcome = confirmed | error | canceled,
  error_class
}

lottery_membership_route_duration_seconds{
  stage = total | membership_fact_read
}
```

但需要控制：

- rule 是固定低基数；
- outcome/error class 是受控枚举；
- subject ref 禁止作 label；
- fact/policy revision 禁止作 label；
- Strategy ID 禁止作 label；
- raw source 默认禁止作 label；
- branch 直接推断会员等级，不进入普通 metrics，除非有专门隐私评审和受控聚合。

### 16.3 为什么 target 不能作为 metric label

Strategy ID 的数量会随业务增长，甚至可由配置产生。把它放入 label 会：

- 造成高基数；
- 增加时序成本；
- 暴露内部资源分布；
- 让监控查询依赖业务 ID；
- 在删除/重建 Strategy 后留下难解释历史。

需要按 target 分析时，可以在受保护的数据分析/审计域通过事件字段或离线聚合完成，不应直接污染通用时序标签。

### 16.4 日志最小字段

未来受控日志可能记录：

- correlation/request ID（由上层提供）；
- stable rule；
- outcome/error class；
- duration；
- policy revision 的安全摘要；
- provider/source 的受控 ID；
- 是否 stale/future/mismatch；
- application build version。

不应记录：

- subject ref 明文；
- raw tier payload；
- 完整 provider cause；
- credential/header；
- 会员 profile；
- random ticket；
- 未走分支；
- 未来 Draw 的券码或权益码。

### 16.5 告警应该面向 failure mode

未来真正值得告警的不是“出现一条 error”，而是：

- membership unavailable 比例持续升高；
- invalid payload 突增，可能表示 provider schema 漂移；
- stale 比例升高，可能表示同步停滞；
- future fact 出现，可能表示时钟/映射异常；
- decision invariant invalid 出现，意味着内部 bug，应高优先级；
- cancel 比例升高且 fact read latency 同时升高；
- route 成功率与 provider SLO 脱钩，可能存在隐藏 fallback。

当前没有这些指标和阈值，本文只登记未来设计，不声称已上线告警。

### 16.6 `ErrMembershipRoutingDecisionInvalid` 的观测级别

这个错误不是普通外部输入问题。正常代码路径由 domain 构造的 decision 应永远 `Confirmed()==true`。若 application 捕获不一致，可能意味着：

- domain 构造 bug；
- 内存/并发违规；
- 未来反序列化绕过构造器；
- 新 branch 没同步 reason/path invariant；
- 代码升级不兼容。

因此未来应：

- 返回安全技术失败；
- 不 fallback；
- 记录低基数内部 invariant class；
- 关联 build/release；
- 快速告警；
- 避免把整个 decision 或会员事实明文打日志。

## 17. 性能、容量与故障域

### 17.1 当前延迟模型

一次合法 Route 的关键路径：

```text
T_total =
  T_validation
  + T_clock
  + T_membership_reader
  + T_fact_validation
  + T_domain_switch
  + T_decision_copy
```

其中：

- validation、clock、switch 都是 O(1)；
- path 长度固定 1；
- 最大不确定项是外部 reader；
- 没有 Strategy repository；
- 没有 Redis；
- 没有随机源；
- 没有重试或 fan-out。

所以优化重点不应是把 switch 换成 map，而是未来 provider 的 latency、timeout、连接池、容量和 freshness 策略。

### 17.2 每请求依赖调用预算

当前成功路径严格期望：

- Clock 1 次；
- membership reader 1 次；
- domain route 1 次；
- Strategy repository 0 次；
- selector/random 0 次。

无效输入/配置：

- Clock 0；
- reader 0。

zero Clock：

- Clock 1；
- reader 0。

pre-cancel：

- Clock 0；
- reader 0。

这些 call-count 断言防止后续“为了保险”偷偷重复读取，导致不同 fact snapshot、额外成本或不可解释路径。

### 17.3 为什么当前没有 retry

在 use case 内自动 retry reader 会引入：

- 两次可能不同 revision 的 fact；
- 总预算不可控；
- provider 雪崩；
- cancellation 语义变复杂；
- error/cause 选择困难；
- freshness 与 as-of 解释冲突。

未来 adapter 若要 retry，必须满足：

- caller budget；
- 只读/幂等；
- bounded attempts；
- backoff/jitter；
- 单一 as-of 下如何接受新快照的明确政策；
- 可观测 attempt；
- 不在 not-found/invalid 上盲重试。

本节没有证据批准。

### 17.4 为什么当前不缓存 Route

缓存最终 Route 看似能避开 provider，但 key 至少需要：

- subject；
- fact revision；
- policy **完整内容或唯一内容绑定 revision**；
- freshness/as-of；
- 可能的 Activity/tenant scope。

当前缺少：

- provider invalidation；
- revision 内容唯一绑定；
- Activity 发布；
- Strategy version；
- 会员升降级撤销；
- cache privacy/authorization。

缓存一个 `subject -> target` 会让旧会员事实和旧 policy 在 TTL 内继续生效。第 24 节 Strategy cache-aside 的可重建读取投影经验不能直接迁移到一次用户路由决定。

### 17.5 current path allocation 是否值得优化

每个成功 decision 创建一项 slice，`Path()` 每次 getter 再复制一项。理论上有分配成本，但：

- 当前关键路径有一次外部 reader；
- path 复制保护不可变证据；
- 没有 benchmark/profile 证明热点；
- 下一节 path 可能变多步。

因此先保持清晰与安全。只有 profile 显示分配占显著比例，并且业务 path 形状稳定后，才考虑优化。

### 17.6 容量推导仍缺哪些数据

要做生产容量规划，至少需要：

- route QPS/P95/P99；
- provider QPS quota；
- provider latency/error distribution；
- caller timeout；
- fact update/revocation frequency；
- standard/premium 分布；
- stale/future 比例；
- Activity 峰值与突发系数；
- 单实例 goroutine/memory；
- adapter connection pool；
- fallback/降级是否被产品批准。

本节单测没有这些数据，所以不能给出 production SLO。

## 18. 并发：只读服务不等于全链路线程安全

### 18.1 当前 service 为什么可以共享

构造后 `MembershipStrategyRoutingService` 的字段不在 Route 中修改：

- reader interface；
- clock interface；
- maxFactAge value。

policy/fact/decision 的字段私有，按值传递或只读访问；请求态局部存在栈中；path 返回副本。

这使 service 自身适合作为共享 application dependency。

### 18.2 64 worker 测试证明什么

`TestMembershipStrategyRoutingServiceSupportsConcurrentReadOnlyCalls`：

- 启动 64 goroutine；
- 共享同一个 service/policy；
- reader/clock stub 用 mutex 保护内部 call count；
- 每个调用得到 confirmed premium target 200；
- 所有 decision `reflect.DeepEqual`；
- reader/clock 各恰好 64 次。

配合 `go test -race`，它能发现本节代码的明显共享请求态竞态。

### 18.3 它不能证明什么

该测试不能证明：

- 真实 adapter 线程安全；
- provider 返回一致 snapshot；
- 网络连接池容量；
- policy 被外部调用方并发替换时安全；
- future repository 的事务隔离；
- 正式 Draw 幂等；
- 多实例之间一致；
- race detector 覆盖未执行路径；
- 线上 64 并发性能。

尤其 interface 指向的 reader/clock 仍由实现负责并发安全。service 的只读并不能修复一个有数据竞态的 adapter。

### 18.4 policy snapshot 不可变性的当前边界

`MembershipStrategyRoutingPolicy` 是值对象、字段私有，因此创建后在 domain 外不能改 target。这给单进程调用提供快照语义。

但同一个 revision 可以被另一次构造调用绑定不同 targets；不同 goroutine 若各自拿到不同 snapshot，都会各自确定地求值。系统目前没有全局 registry 宣布哪一份才是发布版本。

这正是“局部不可变”与“全局发布一致”必须分开的例子：

- 第 27 节证明前者；
- 第 28/30 节需要证明后者。

### 18.5 TOCTOU 仍在哪里

即使 Route 本身纯且无副作用，未来流程仍可能：

```text
T1: read membership premium
T2: route Strategy P
T3: membership downgrades to standard
T4: load/select from Strategy P
```

是否允许要由正式 Participation/Activity/Draw 的快照政策决定。本节没有业务 identity 或结果持久化，不能解决跨阶段 TOCTOU。

## 19. 方案矩阵：为什么选当前最小切片

| 方案 | 所有权正确 | default 明确 | 错误语义 | path | 当前复杂度 | 结论 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Participation chain 第三 gate | 否 | 否 | 易混资格 | 弱 | 低 | 拒绝 |
| handler `if/else` | 否 | 可写但易复制 | transport 污染 | 弱 | 最低 | 拒绝 |
| provider 直接给 Strategy ID | 否 | provider 决定 | 依赖故障直达 target | 无 | 低 | 拒绝 |
| map miss -> default | 部分 | 表面有 | unknown 被吞 | 弱 | 低 | 拒绝 |
| 完整 profile + generic context | 否 | 可配置 | PII/类型丢失 | 泛化 | 中 | 拒绝 |
| Route 内加载 Strategy | 部分 | 是 | 路由/资源失败混合 | 中 | 中 | 延后 |
| 立即 RuleTree/Engine | 可设计 | 可设计 | 需大量新语义 | 强 | 高 | 过早 |
| DMN/OPA/脚本 | 可防腐 | 可表达 | runtime/security 新面 | 强 | 很高 | 无证据 |
| concrete Lottery router | 是 | 是 | fail-closed | 一跳 | 适中 | 采用 |

### 19.1 为什么不把 Route 做成 Strategy 方法

类似：

```go
strategy.RouteFor(tier)
```

会让某一个 Strategy 聚合反过来决定“应该选择哪个 Strategy”，语义自相矛盾；它还要求先加载一个 Strategy 才能知道目标。路由 policy 是多个 Strategy identity 之间的选择，不属于任一 target 聚合。

### 19.2 为什么不在 application 里直接 switch

application 当然可以写 switch，但把纯决定放 domain 有三项收益：

- 同样 policy/fact/as-of 可独立复算；
- application 只负责依赖/时间/取消；
- branch/reason/path 不变量集中；
- 未来不同入口可复用同一领域决定。

代价是 domain/application 都做部分验证。这个重复用于保护各自边界，不是 DRY 失败。

### 19.3 为什么不使用 `map[MembershipTier]StrategyID` policy

map 对当前两值没有收益，却会带来：

- 缺省语义依赖 miss；
- 遍历顺序不稳定；
- 调用方可带任意 key；
- deep copy/不可变性更复杂；
- 分支 reason/path 仍需另一张映射；
- schema 形状在需求证明前泄漏。

显式字段 `premiumTarget/baselineDefault` 把必填项变成构造不变量。

### 19.4 为什么不把 unknown 做合法枚举

如果定义：

```go
MembershipTierUnknown
```

调用方可能把“没有事实”和“authority 明确声明某个业务等级”混为一谈。当前 unknown 不是 domain 内合法事实，所以 zero/unsupported 在构造阶段失败。

未来若产品需要 guest/unclassified，应该新增有业务含义的 tier，而不是复用技术 unknown。

### 19.5 为什么没有 strategy existence check

在 Route 内调用 repository 会把错误域混在一起：

- membership fact failure；
- policy invalid；
- target not found；
- Strategy aggregate corrupt；
- database/cache unavailable。

当前学习目标是证明多出口边界。目标引用完整性需要发布时校验，而不一定每请求加载。第 28/30 节出现持久化与 Activity 发布后再决定 validation timing 更合理。

## 20. 测试思维：每个测试都要证伪一种坏设计

### 20.1 事实模型测试

#### `TestNewMembershipTierFactSnapshotCanonicalizesUTC`

证明：

- non-UTC observed-at 被规范为 UTC；
- subject/tier 不丢失；
- source/revision 被保留；
- location 确实是 `time.UTC`。

不能证明：

- source 真的权威；
- observed-at 由 provider 可信生成；
- revision 全局唯一。

#### `TestMembershipTierFactSnapshotRejectsInvalidInputs`

表驱动覆盖：

- zero ref；
- zero tier；
- unsupported `gold`；
- zero observed-at；
- blank/前导空格 source；
- revision 控制字符；
- oversized source/revision。

每个非法构造必须返回对应 sentinel 且 fact 为零。

#### `TestMembershipTierFactSnapshotValidateRejectsManualZeroAndUnknown`

在 domain 同包手工构造 zero/`vip` fact，证明即使绕过构造器，`Validate` 仍失败关闭。

#### `FuzzMembershipTierFactSnapshotNeverAcceptsUnsupportedTier`

对任意字符串只允许 exact `standard` / `premium`。它证伪“未来宽松字符串被自动当 baseline”，但不是完整 fuzz security audit。

### 20.2 policy 测试

#### `TestMembershipStrategyRoutingPolicyPreservesDistinctVersionAndTargets`

证明 revision、premium target、baseline target 是三个独立值，不会因 getter/构造丢失。

它不证明 revision 唯一绑定这两个 targets；当前没有 registry。

#### `TestMembershipStrategyRoutingPolicyAllowsConvergingTargets`

证明相同非零 target 是合法 rollout 状态，不把“分支不同”误写成“资源 ID 永远不同”。

#### `TestMembershipStrategyRoutingPolicyRejectsInvalidInputs`

覆盖：

- empty/trimmed/control/oversized revision；
- zero premium；
- zero baseline；
- manual zero policy。

非法构造返回 zero policy。

### 20.3 domain route 测试

#### `TestRouteMembershipStrategyExposesBranchSpecificTerminalPaths`

这是本节核心业务测试：

| tier | branch | reason | target |
| --- | --- | --- | ---: |
| standard | baseline_default | baseline_strategy_selected | 100 |
| premium | premium_override | premium_strategy_selected | 200 |

同时断言：

- decision confirmed；
- rule code；
- policy/fact revision；
- fact source；
- evaluated-at；
- path 长度 1；
- path rule/branch/target 与外层一致。

#### `TestMembershipRoutingStableCodesAreLiteralContracts`

固定四类 literal：

- `lottery.membership_tier.route_strategy`；
- `premium_override`；
- `baseline_default`；
- 两个 reason code。

这防止普通重命名无声破坏 trace/指标/未来持久化兼容。

#### `TestRouteMembershipStrategyKeepsBranchEvidenceWhenTargetsConverge`

让 standard/premium 都指向 Strategy 100，断言 target 相同但 branch 不同。它保护“路径原因不由终点唯一决定”。

#### `TestMembershipRoutePathIsDefensivelyCopied`

调用方修改 getter 返回 slice 后，第二次读取仍是原 branch/target，证明内部证据不被外部 mutation。

#### `TestMembershipRouteDecisionConfirmedRejectsInconsistentEvidence`

同包测试复制合法 decision 后分别伪造：

- path target；
- path branch；
- path rule；
- branch-reason pair。

`Confirmed()` 必须全部返回 false。它保护 application 返回前 invariant check 的含义。

#### `TestRouteMembershipStrategyRejectsInvalidOrFutureInputsWithZeroDecision`

覆盖 zero policy、zero fact、zero instant、future +1ns，并逐字段断言 zero decision。

#### `TestRouteMembershipStrategyIsUTCAndRepeatDeterministic`

同一 instant 的 UTC+8 和 UTC 表达得到相同 target/branch/evaluated-at。准确口径是：测试使用同一完整 policy value、fact 和等价 instant；它不证明单独 revision 字符串绑定内容。

### 20.4 application 时序与 freshness 测试

#### `TestMembershipStrategyRoutingServiceRoutesClosedTiersInDependencyOrder`

证明：

- standard/premium exact mapping；
- context 原样传给 reader；
- subject ref 原样传递；
- Clock 与 reader 各一次；
- 调用顺序严格 `clock -> reader`；
- decision evidence 与输入一致；
- evaluated-at 为 UTC。

#### `TestMembershipStrategyRoutingServiceCapturesClockExactlyOnce`

reader 后修改 clock 24 小时，decision 仍使用初始 instant；证伪二次 `Now()`。

#### `TestMembershipStrategyRoutingServiceEnforcesFreshnessNanosecondBoundary`

证明 exact max age inclusive、+1ns stale、future +1ns invalid、subject mismatch invalid、zero fact invalid。每个失败 decision 全零。

#### `TestMembershipStrategyRoutingServiceMakesFactErrorWinReturnedFact`

reader 同时返回 valid-looking premium fact 与 private error，error 必须胜出；public class 为 read failure，cause 仅显式读取。

#### `TestMembershipStrategyRoutingServiceClassifiesProviderDeadlineWhileCallerLives`

证明 provider deadline 不冒充 caller deadline。

#### `TestMembershipStrategyRoutingServiceClassifiesProviderPayloadContractErrorsAsInvalid`

reader 返回 valid-looking standard fact，同时返回包裹 domain invalid/zero-subject error。若错误被忽略就会命中 baseline；测试要求：

- application invalid class；
- raw domain class 不进入 public `errors.Is`；
- raw detail 仅在 `Cause()`；
- safe `Error()`；
- zero decision。

### 20.5 输入、配置与取消测试

#### `TestMembershipStrategyRoutingServiceRejectsInvalidInputsBeforeDependencies`

nil context、zero subject、zero policy 均在 Clock/reader 前失败。

#### `TestMembershipStrategyRoutingServiceRejectsNilTypedNilAndPartialConfiguration`

覆盖：

- nil/typed-nil reader；
- nil/typed-nil clock pointer；
- typed-nil clock func；
- zero/negative max age；
- nil service；
- zero/partial service。

所有情况返回 not-configured、zero decision、依赖零调用。这防止 Go interface 中 typed-nil 看似非 nil 后 panic。

#### `TestMembershipStrategyRoutingServiceRejectsZeroClockBeforeFactRead`

zero Clock 结果时 Clock 1、reader 0，证明时间不可信就不访问会员事实。

#### `TestMembershipStrategyRoutingServiceMakesObservedCallerCancellationWin`

覆盖：

- pre-cancel：0/0；
- Clock 回调取消：1/0；
- reader 回调取消且同时返回 dependency error：1/1，caller cancellation only。

#### `TestMembershipStrategyRoutingServiceObservesCancellationAfterBlockingReaderReturns`

证明 reader 返回后观察 cancellation；不证明能强制中断忽略 context 的 reader。

#### `TestMembershipRoutingClockFuncAdaptsControlledClock`

只证明 function adapter 正确透传 instant，不证明这个函数是可信生产时钟。

### 20.6 错误 wrapper 测试

#### `TestMembershipTierFactReadErrorPreservesSafeClassAndExplicitCause`

对 not-found/unavailable/read-failure/invalid 四类断言：

- public class 可匹配；
- raw cause 不可匹配；
- `errors.Unwrap()==nil`；
- `Cause()` 保留；
- `Error()` 不含 secret。

#### `TestMembershipTierFactReadErrorFailsClosedForUnknownZeroAndTypedNil`

unknown class、zero wrapper、typed-nil wrapper 都稳定收敛 read failure，不 panic。

#### `TestMembershipTierFactReadErrorExposesExactlyOneSemanticClass`

证明 class 与 cause 不会形成两个公开机器类别。

#### `TestMembershipTierFactReadErrorKeepsDomainPayloadErrorOnlyInCause`

证明 provider/domain payload contract error 只经显式 Cause 保留。

### 20.7 并发与架构测试

#### `TestMembershipStrategyRoutingServiceSupportsConcurrentReadOnlyCalls`

64 worker 得到完全一致 decision，依赖各调用 64 次。配合 race detector检查本切片共享状态。

#### `TestLesson27MembershipRoutingKeepsContextOwnershipAndEngineStopLine`

AST 扫描 Lottery domain/application 和 Participation domain/application 的所有 production Go 文件：

- 只允许本上下文受控 project import；
- 禁止通用 Rule/Tree/Engine 类型名；
- 禁止泛型类型；
- 禁止 untyped string fact bag。

它能发现源码层漂移，不能证明整个仓库所有 transport/runtime 文件绝无变化；那仍需 Git diff、课程验收和集成检查。

### 20.8 当前测试没有覆盖什么

明确未覆盖：

- 真实 provider contract/authority；
- adapter 网络 timeout、retry、credential；
- provider schema 漂移集成测试；
- policy registry/revision 内容唯一绑定；
- Strategy target 存在性/版本/发布；
- MySQL/Redis/Migration；
- HTTP contract；
- Activity/Participation/Lottery 在线编排；
- authentication/authorization；
- UI/E2E；
- 持久化审计/OTel；
- production benchmark/load/SLO；
- 多实例和跨系统 TOCTOU；
- decision invariant error 的可注入 application 路径；
- 正式 Draw/Result/库存/发放。

## 21. 风险账本：当前接受什么，什么证据会迫使重设计

| 编号 | 风险 | 当前控制 | 为什么暂时接受 | 触发器 / 后续 |
| --- | --- | --- | --- | --- |
| R27-01 | 没有真实会员 authority adapter | consumer-owned port + stub tests | 本节只证明内核 | 接 provider 时新增 adapter 契约 |
| R27-02 | source string 不证明权威 | canonical metadata | 部署/组织证据尚未出现 | adapter credential/source registry |
| R27-03 | revision 未唯一绑定 policy 内容 | 完整 snapshot 才是确定性输入 | 无 registry/publisher | 第 28/30 节绑定内容与发布 |
| R27-04 | Strategy target 只校验非零 | policy Validate | 目标生命周期未进入本节 | schema/publish reference validation |
| R27-05 | Strategy 没有业务 version | 文档明确不伪造 | 现有模型事实 | Activity/Draw 历史回放需求 |
| R27-06 | fact revision 语义由 provider 定义 | 保存原 token | adapter 未实现 | 定义格式、水位、撤销 |
| R27-07 | stale window 内会员已降级 | max age | 没有推送/撤销 | 线上准确性/SLO要求 |
| R27-08 | Clock 与 provider 时钟偏差 | future fail-closed | 无 skew policy | future fact 告警或真实事故 |
| R27-09 | logical as-of 非原子快照 | 单次 Clock | 跨系统事务不现实 | 正式 Draw 回放/争议 |
| R27-10 | default 被误用作 fallback | enum + switch + tests | v1 闭集 | 新 tier/降级政策 |
| R27-11 | branch 暗示会员等级 | path 无 subject、不公开 | 尚无 telemetry/API | trace 持久化或对外展示 |
| R27-12 | Cause 被误写入日志 | safe Error + explicit access | 调用方治理尚未出现 | logger/observer 接入评审 |
| R27-13 | blocking reader 忽略 context | 返回后 cancellation check | 无真实 adapter | 网络 adapter 必须传 ctx/timeout |
| R27-14 | 没有自动 retry 降低可用性 | fail-closed | 无预算/幂等证据 | provider SLO 与 retry policy |
| R27-15 | Route 未缓存增加依赖负载 | 一次 reader | 正确性优先 | profile/容量证明后设计 |
| R27-16 | path slice 有分配 | 固定一项、防御复制 | 相对 I/O 极小 | benchmark/profile |
| R27-17 | service 依赖实现可能不线程安全 | interface + race unit | adapter 不在范围 | adapter race/load test |
| R27-18 | Route 与 Participation 未编排 | 明确停止线 | 无可信身份/正式流程 | 后续 orchestrator |
| R27-19 | Route 与 Activity 未绑定 | 明确停止线 | Activity 第 30 节 | 发布版本模型 |
| R27-20 | Route 未授权 | ref 明确非 Principal | auth 第 31～35 节 | public use case 出现 |
| R27-21 | internal decision invariant breach | Confirmed + app check + typed error | 纯构造当前可控 | 任何 invariant alert |
| R27-22 | 未来反序列化绕过私有字段 | 当前无 persistence | 不提前建 schema | 第 28 节重建校验 |
| R27-23 | stable code 改义 | literal tests | 当前单版本 | 持久化/外部消费后版本治理 |
| R27-24 | 两 target 相同掩盖产品错误 | branch 仍保留 | 合法 rollout 场景 | 发布审批可另加策略 |
| R27-25 | 未知 tier 造成可用性失败 | explicit unsupported error | 不允许错误路由 | 产品新增正式 tier |
| R27-26 | metrics 高基数/隐私 | 文档约束 | 尚未埋点 | observer implementation review |
| R27-27 | 没有生产 SLO | 不做虚假承诺 | 无数据 | adapter/load 上线前基线 |

### 21.1 风险账本不是 backlog 垃圾桶

每条风险都应有：

- 当前为什么能接受；
- 现有控制；
- 需要什么新事实；
- 由哪一章节/所有者解决；
- 什么情况下必须停止上线。

否则“以后再做”只是遗忘。尤其 R27-03（revision 内容绑定）不能因为字段名叫 revision 就假装已解决。

### 21.2 最高优先级重决策风险

如果真实接入前出现以下任一情况，应先停下再设计：

- 同一 revision 在不同实例出现不同 targets；
- provider 不能提供 observed-at；
- provider unknown 值会频繁出现；
- Strategy target 可被未授权调用方配置；
- Route 将直接影响高价值奖品；
- route trace 要跨租户展示；
- provider timeout 期间产品要求继续抽奖；
- Activity 发布必须原子引用 policy/Strategy；
- 正式 Draw 争议要求历史回放。

## 22. 第 28 节触发器：什么时候最小树 schema 才有必要

### 22.1 当前 concrete router 给出的真实词汇

第 28 节不需要从空白设计。第 27 节已经证明：

- 有一个稳定 decision identity；
- 有两个 named branches；
- 有一条 required default edge；
- 有 terminal Strategy target；
- 有 policy revision；
- 有一条实际 path；
- unknown/unsupported 不能 fallback；
- 输入/输出与错误必须确定。

这些概念可以成为最小持久化模型的业务来源。

### 22.2 足以触发树的产品证据

任一真实需求都可能触发：

- 运营需要无代码调整 target；
- 需要 draft/approve/publish/rollback；
- 新增第三个受支持 tier；
- tier 后还要判断渠道/地域/用户段；
- 多个 branch 共享后序子判断；
- 多条路径合流到一个 terminal；
- 不同 Activity 引用不同 published graph；
- 历史 Draw 必须引用旧 graph；
- 需要发布前模拟/验证；
- 单个 switch 已开始出现隐式 nested if/goto。

但“课程下一节是规则树”本身不是产品证据。实现仍必须用上述真实概念约束最小范围。

### 22.3 第 28 节最少要回答的 schema 问题

最低限度：

1. graph/policy identity；
2. content revision 与 schema version 分离；
3. draft/published/retired；
4. root 唯一；
5. node/edge stable identity；
6. branch code 与 default edge；
7. terminal Strategy reference；
8. root/edge/target 引用完整性；
9. cycle 拒绝；
10. depth/node/edge 上限；
11. 不可达节点拒绝或告警；
12. 每个 decision 的 default 完整性；
13. unknown operator/schema 拒绝；
14. revision 到不可变内容唯一绑定；
15. 发布并发与回滚；
16. Activity 如何只引用 published revision（第 30 节最终组合）。

### 22.4 第 28 节仍不应提前做第 29 节

持久化并验证一个图，不等于已有安全执行引擎。第 28 节可以建立：

- schema；
- repository；
- publish-time validator；
- round-trip tests；
- migration；
- revision/content binding。

第 29 节再建立：

- evaluator；
- node/operator registry；
- multi-step path；
- step/depth/time budget；
- cancellation；
- unknown operator；
- runtime error；
- deterministic traversal；
- engine architecture boundary。

把两节合并会让“存得下”被误当“执行得安全”。

### 22.5 concrete router 应怎样帮助迁移

本节 concrete function 可以成为规则树的 oracle：

| fact | concrete expected | persisted graph expected |
| --- | --- | --- |
| standard | baseline_default -> B | 同 |
| premium | premium_override -> P | 同 |
| unknown | error | 同 |
| future/stale | application 拒绝 | graph 不执行 |

第 28/29 节应跑等价 fixture，证明抽象化没有改变产品语义。不能因为换成树就让 unknown map miss 到 default。

### 22.6 必须保留的不变量

从 concrete router 到树/engine，以下不能丢：

- 事实 authority 与 Lottery decision ownership 分离；
- MembershipSubjectRef 不是 Principal；
- 完整 published policy snapshot 决定确定性；
- revision/schema/Strategy/app version 分离；
- single controlled as-of；
- exact freshness；
- caller cancellation 优先；
- fact+error 时 error 胜出；
- default 不吞 unknown；
- technical error 返回零 decision/path；
- path 只记录实际边；
- raw cause 不进入 public `errors.Is` tree；
- branch/target/reason/path 一致；
- selector 保持纯终端能力；
- 没有权限时不能公开配置或 trace。

## 23. 对第 29～35 节的继续演进

### 23.1 第 29 节：执行已验证图

重点不是再造 DSL，而是：

- evaluator 只接受已发布、已验证 snapshot；
- 执行有 step/depth/time budget；
- 每一步 path 可验证；
- unknown operator/edge 失败关闭；
- 取消不返回 partial business decision；
- 不让 engine 直接读所有上下文表；
- concrete input port 仍受所有权约束。

### 23.2 第 30 节：Activity 发布绑定

第 30 节需要解决：

- 哪个 Activity version 引用哪个 route graph/policy revision；
- 目标 Strategy snapshot/version 怎样引用；
- publish/rollback 是否原子；
- 请求怎样拿到一致 released snapshot；
- 历史结果怎样保留引用。

这也是 policy revision 字符串真正绑定内容和发布生命周期的重要边界。

### 23.3 第 31～35 节：权限

至少分开：

- authentication：caller 是谁；
- authorization：能否 view/edit/publish/execute 某资源；
- frontend projection：根据服务端能力裁剪 UI；
- negative API tests：隐藏按钮之外仍阻止直调；
- browser E2E：不同角色界面与越权路径。

会员等级不是管理员角色，`premium` 不能自动获得策略管理权；`MembershipSubjectRef` 不能当 session Principal。

### 23.4 后续正式 Lottery 流程

Route 以后只是：

```text
trusted identity/authorization
  -> Activity gate
  -> Participation eligibility/account
  -> Lottery route
  -> Strategy snapshot
  -> inventory/availability policy
  -> WeightedSelector
  -> Draw/Result
  -> Benefit delivery
```

任何一步的成功都不能提前宣称后续已完成。

## 24. 架构师逐步思考清单

遇到“根据某属性走不同策略”时，按顺序问：

1. 这真是 Route，还是 Eligibility/Authorization/Selection？
2. 输入事实由谁拥有？
3. 当前上下文只需哪些最小字段？
4. provider 是否越权返回内部 target？
5. subject lookup ref 是否被误当身份？
6. 支持哪些 exact values？
7. unknown 是业务值还是无法决定？
8. default 是显式产品边还是 map miss？
9. 一个成功结果需要几个 terminal target？
10. branch、reason、target 是否应该分离？
11. target 相同是否仍需保留 branch？
12. policy revision 是否真的绑定唯一内容？
13. target 是否只是 ID，还是已有 version/publish 证据？
14. evaluated-at 由谁提供？
15. freshness 边界 inclusive 还是 exclusive？
16. fact 在 future 时怎样处理？
17. provider error 与 caller cancel 怎样区分？
18. fact+error 谁胜出？
19. error 是否泄露 raw cause？
20. path 最少记录什么？
21. path 是否含可推断敏感属性？
22. 决定怎样防止调用方 mutation？
23. internal invariant 怎样在返回前复核？
24. 依赖调用次数能否被测试固定？
25. 为什么 chain 不能表达？
26. 是否真的需要树，还是一个 switch 足够？
27. 树出现后需要哪些发布/验证语义？
28. 当前测试能证明哪些内核事实？
29. adapter/API/E2E 是否仍不存在？
30. 什么事故或产品变化会迫使重决策？

第一性原则不是“永远写简单代码”，而是：

> 先找到不可替代的业务区分和失败代价，再选择刚好能表达它的最小结构；当真实反例证明结构不够时升级，但不把下一层复杂度提前伪装成扩展性。

## 25. 证据索引与最终边界

### 25.1 规范与 ADR

| 命题 | 证据 |
| --- | --- |
| 第 23 节事实/决定所有权与 Route 类别 | [Lottery 业务规则需求基线 v1](../../product/lottery-rule-requirements-v1.md) |
| 第 26 节线性 chain 的能力与停止线 | [Participation 前置资格链基线 v1](../../product/participation-prerequisite-chain-v1.md) |
| 第 27 节 exact product mapping | [会员等级 Strategy 路由基线 v1](../../product/membership-strategy-routing-v1.md) |
| concrete router、default、path 与演进决定 | [ADR-0023](../../decisions/ADR-0023-membership-strategy-routing-boundary.md) |
| 规则所有权总边界 | [ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md) |
| Participation chain 不被替代 | [ADR-0022](../../decisions/ADR-0022-participation-prerequisite-chain.md) |

### 25.2 代码与测试

| 设计命题 | 实现/测试 |
| --- | --- |
| closed tier 与最小 authority fact | [membership_fact.go](../../../internal/lottery/domain/membership_fact.go) / [fact tests](../../../internal/lottery/domain/membership_fact_test.go) |
| complete code-owned policy snapshot | [membership_routing_policy.go](../../../internal/lottery/domain/membership_routing_policy.go) / [policy tests](../../../internal/lottery/domain/membership_routing_policy_test.go) |
| explicit branch/reason/target/path | [membership_routing.go](../../../internal/lottery/domain/membership_routing.go) / [route tests](../../../internal/lottery/domain/membership_routing_test.go) |
| single as-of、freshness、cancel、reader | [application service](../../../internal/lottery/application/membership_routing.go) / [service tests](../../../internal/lottery/application/membership_routing_test.go) |
| one class + explicit Cause | [application errors](../../../internal/lottery/application/membership_routing_error.go) / [error tests](../../../internal/lottery/application/membership_routing_error_test.go) |
| no cross-context/generic engine drift | [architecture test](../../../internal/lottery/application/membership_routing_architecture_test.go) |

### 25.3 外部资料只用于语言/机制校准

1. Go, [Code Review Comments — Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)：支持由消费方定义窄接口；不证明 provider 是组织权威。
2. Go, [`context` package](https://pkg.go.dev/context)：说明 cancellation/deadline 传播；不能强制一个忽略 context 的同步依赖立即返回。
3. Go, [`errors` package](https://pkg.go.dev/errors)：说明 `Is/As/Unwrap` 的遍历机制；本节据此隔离 raw provider cause。
4. OMG, [DMN 1.5](https://www.omg.org/spec/DMN/1.5/PDF)：帮助识别 decision、input、hit policy 与模型验证问题；不表示本节采用 DMN runtime。

### 25.4 本节可以准确声称什么

> GrowthOS-Go 已在 Lottery domain/application 内核建立一个具体、只读的会员等级 Strategy 路由切片。外部事实契约只接受 canonical `standard` / `premium` 快照；Lottery 的完整 policy snapshot 持有 premium override 与 baseline default target。service 在输入/配置合法且 caller 未取消后捕获一次 UTC logical as-of，读取一次会员事实，严格检查主体、future 和 freshness，再调用纯 domain route。standard 与 premium 分别形成 `baseline_default` / `premium_override`、稳定 reason、明确 Strategy ID 和一跳 path；unknown、unsupported、缺失、过期、future、读取错误与取消均返回零决定，绝不 fallback。provider error 通过一个公开 application class 和不参与 `errors.Is` tree 的显式 `Cause()` 通道隔离。`Confirmed()` 校验 branch-reason 与 path 一致，application 返回前再次断言内部不变量。实现没有把 Route 塞进 Participation chain，也没有创建通用树或引擎。

### 25.5 本节不能声称什么

本节不能声称：

- 已接入真实会员系统；
- source/revision 字符串本身证明 authority；
- policy revision 已由 registry 绑定唯一内容；
- target Strategy 存在、已发布或有业务 version；
- 已持久化 policy/tree/path；
- 已实现第 28 节规则树 schema；
- 已实现第 29 节决策引擎；
- 已绑定第 30 节 Activity；
- 已有 session、Principal、RBAC/ABAC；
- 前端已按权限裁剪；
- 已有 HTTP/API/runtime/Compose 接线；
- 现有 ephemeral Lottery route 已被会员门控；
- 已调用 Strategy repository 或 WeightedSelector；
- 已消费随机票据、创建 Draw/Result、扣库存或发奖；
- trace 是持久化审计或 OTel；
- 已有生产 metrics/dashboard/alert；
- 64 worker 单测证明生产容量；
- single as-of 提供跨 authority 原子快照；
- provider timeout 时有获批 fallback；
- 已完成浏览器或任何端到端验收。

## 26. 最终第一性结论

第 27 节最重要的不是新增了几个 Go struct，而是完成了三次边界澄清：

1. **从资格到路由：** “是否继续”与“去哪里”是不同决定；线性 chain 对前者仍正确，对后者表达不足。
2. **从 default 到可信 default：** 缺省边必须只承接产品批准的已确认输入，不能把 unknown、provider failure 或未来值变成成功。
3. **从分支到树的节制：** 一个真实 decision node 足以证明多出口，却还不足以证明数据库树、DSL 或通用 engine；下一层复杂度必须由持久化、发布和多步拓扑证据触发。

最终应保留的原则是：

> **事实所有者只确认事实，决定所有者解释事实；一个 Route 必须由完整 policy snapshot、可信事实和单一受控时刻共同形成。成功路径要显式记录 branch 与 target，失败路径不能留下可消费半成品。线性链被真实多出口证伪时，应新增恰好能表达 Route 的领域模型；但在 root、edge、发布、校验和多步执行需求尚未出现前，不要为了“架构先进”把一个 switch 包装成规则引擎。**
