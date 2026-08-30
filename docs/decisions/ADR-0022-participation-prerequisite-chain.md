# ADR-0022：以风险准入证明 Participation 最小线性前置资格链

- **状态：** 已接受
- **日期：** 2026-08-30
- **负责人：** GrowthOS 架构组

## 背景

ADR-0021 在只有一条新用户规则时明确拒绝通用规则接口。第 26 节出现了第二条已在第 23 节需求台账登记的真实前置条件：风险事实提供方给出最小 screening verdict，Participation 需要决定该主体能否进入当前参与场景。

我们现在必须解决的不是“怎样造一个规则平台”，而是：两条只读资格规则怎样共享可信时间、按有业务意义的顺序执行，在确定拒绝、技术失败或取消时准确短路，并提供不泄密的最小执行证据。

## 决策驱动

1. 第二条规则必须来自既有业务需求，不能用未来 Activity、额度或权限模型凑数；
2. `eligible`、`ineligible`、技术失败与 caller cancellation 必须保持不同语义；
3. 新用户拒绝后不能继续读取更敏感、更昂贵的风险事实；
4. 两条时间敏感规则必须共享一次受控 logical evaluated-at；
5. risk freshness 必须依据 verdict 产生时刻，读取不能给旧结论续鲜；
6. 规则顺序、规则集 revision、单规则 policy revision、fact revision 和应用版本必须分离；
7. 最终决定与内部 trace 不得携带完整主体、风险特征、阈值或原始 provider error；
8. 不得提前实现第 27～29 节的分支、持久化规则树或决策引擎；
9. 不得提前接入第 30～35 节的 Activity、认证和公共授权。

## 评估过的方案

### 方案一：用剩余次数作为第二个 bool gate

| 优点 | 代价 / 风险 |
| --- | --- |
| 产品上直观，容易写 `remaining > 0` | 次数是并发可消费事实，先查后用存在 TOCTOU |
| 可以快速演示两个节点 | 没有账户、流水、事务、版本或预占证据 |
| 顺序容易固定 | 会把第 39～45 节的并发语义伪装成已完成 |

**结论：拒绝。** 次数资格必须与真实参与账户和消费承诺一起建模。

### 方案二：用 Activity 时间窗作为第二个 gate

| 优点 | 代价 / 风险 |
| --- | --- |
| 时间判断与新用户规则形状相似 | 第 30 节才建立 Activity 生命周期与版本引用 |
| 可以共享 clock | 会让 Participation 擅自拥有 Marketing 的发布决定 |

**结论：拒绝。** Activity 门控属于 Marketing；不能为了责任链示例提前虚构聚合。

### 方案三：用角色或管理员身份作为第二个 gate

| 优点 | 代价 / 风险 |
| --- | --- |
| 能回应权限诉求 | Eligibility 与 Authorization 语义不同 |
| 看似可复用 Rule 接口 | 尚无可信 Principal、会话、资源、动作和范围 |

**结论：拒绝。** 公共访问控制按第 31～35 节线性落地，不能用 ParticipantRef 或前端角色冒充。

### 方案四：预取注册与风险事实后统一评估

| 优点 | 代价 / 风险 |
| --- | --- |
| 可以在读取完成后捕获一个时间 | 非新用户仍会访问风险 authority |
| 规则执行阶段容易纯化 | 短路不再减少敏感读取、成本和故障传播 |

**结论：拒绝。** 顺序读取与短路本身就是本节的业务价值。

### 方案五：两个节点各读一次 clock

| 优点 | 代价 / 风险 |
| --- | --- |
| 每个现有 service 可独立复用 | 边界附近会得到混合时点，难以回放和解释 |
| 后序即时事实更容易通过 future 检查 | 违反 LRR-008 与事实台账的一次评估时刻约束 |

**结论：拒绝。** chain 使用一个服务端 logical as-of；after-as-of 快照留给下一次评估。

### 方案六：公开通用 `Rule` 接口与动态 slice

| 优点 | 代价 / 风险 |
| --- | --- |
| 容易继续添加任意节点 | Authorization、Inventory、Activity 会被压成万能协议 |
| 看起来符合经典模式 | priority、generic context、动态装配和无效组合随之出现 |
| 可以单测 runner | 两条固定 gate 尚未证明运行时插件需求 |

**结论：当前不采用。** 只在 Participation application 内抽取包内线性 step 协议，外部构造器明确装配注册与风险两个事实端口。

### 方案七：Participation 专用固定 ordered gate chain

| 优点 | 成本 / 风险 |
| --- | --- |
| 两条规则、一次时间、固定顺序和短路都可执行验证 | 当前组合仍是硬编码线性 plan |
| 不预取风险事实，也不引入通用平台 | 两次外部读取不是原子快照 |
| trace 可由具体决定安全投影 | 暂无生产 adapter、observer 或持久化审计 |
| 能自然暴露第 27 节的分支需求 | 未来调序必须新增 ruleset revision |

**结论：采用。** 复杂度与当前证据相称。

## 决策

### 1. 第二条具体规则是风险 screening 准入

风险事实 provider 只给出 `passed/blocked`、`assessed_at`、source 和 revision。Participation 固定 rule code 为：

```text
participation.risk.screening_admission
```

`passed` 只让该节点继续，`blocked` 形成确定业务拒绝。事实缺失、过期、future、损坏、主体不符、provider unavailable 或未分类读取失败均为零决定 + typed error，默认失败关闭。

### 2. 固定执行 new-user 后 risk-admission

第一个节点是低敏、低成本的新用户判断；它确定拒绝、失败或被取消后，risk reader 必须零调用。只有两节点都确定通过，组合器才形成最终 `eligible`。

规则集 revision 显式传入并与节点 policy revision 分离。改变顺序需要新的规则集 revision 和回归证据，不能依赖 map、SQL 或调用方偶然顺序。

### 3. chain 拥有唯一 logical evaluated-at

chain 在读取事实前调用受控 Clock 一次，构造包内不可伪造的 evaluation-instant。注册与风险 evaluator 都使用该时刻进行 future/freshness 判断。

现有 standalone `NewUserEligibilityService.Evaluate` 的“成功读 fact 后调用 clock”保持兼容。公共 API 不导出接受裸 `time.Time` 的 `EvaluateAt`，避免调用者传旧时刻绕过 freshness。包内 helper 接受受控 token，chain 直接拥有所需 reader、max-age 与唯一 clock，避免多个未使用或不一致的 clock 配置。

### 4. 包内最小 step，不开放规则平台

application 可以使用私有 step closure/struct 统一 `code + evaluate` 形状，runner 只实现确定 for-loop：

1. 调用前检查 context；
2. 执行当前具体规则；
3. 返回后再次让 caller cancellation 优先；
4. `eligible` 才进入下一项；
5. `ineligible` 或 error 立即停止。

不导出万能 `Rule`、generic context、priority、插件、树、图或 DSL。构造器固定接收 registration reader、risk reader、Clock 与各自 freshness 上限；Evaluate 每次显式接收 ParticipantRef、ruleset revision 和两份 concrete policy。

### 5. 最终决定与 trace 分开理解

业务确定时返回不可变 `PrerequisiteEvaluation`：最终 outcome/reason、ruleset revision、同一个 evaluated-at 和已执行 step 的有序 trace。`Steps()` 返回副本，防止调用方改写内部证据。

技术失败或取消返回零 evaluation + error，调用方不得使用半成品决定。错误仍能通过稳定 class 和当前 rule code 定位，但 `Error()` 不渲染 SQL、地址、风险特征或原始 payload。未来 observer 可以记录安全 error class；本节不伪造完整审计或 OTel span。

### 6. 本节只改 Participation 内核

不新增 Migration、Redis key、HTTP route/status/header、React 状态、Compose 服务或运行配置。Lottery Strategy、WeightedSelector、ephemeral selection API 和现有页面保持不变。没有可信会话与正式 Participation/Draw 之前，不把这条链接到公开入口。

## 影响

### 正面影响

- 第二条真实规则使共同的 continue/reject/error/cancel 和顺序语义有了证据；
- 确定拒绝真正阻止后序风险读取，而不是只改变最终 bool；
- 单一 evaluated-at、ruleset revision 与有序 trace 提高可回放和解释能力；
- risk verdict 与 Participation admission 的所有权保持分离；
- 模式被限制在当前 Participation 用例，没有污染 Lottery、Authorization 或 Inventory。

### 成本与限制

- 固定两节点 plan 不是动态规则平台；
- 顺序读取增加尾延迟，但换取隐私最小化、短路和稳定原因；
- 逻辑 as-of 不使跨 authority 读取成为原子快照；
- 没有真实 adapter，无法声称已联通用户/风险系统；
- 没有 HTTP/Lottery 装配，当前公开抽奖仍未获得资格保护；
- trace 未持久化，也不是安全审计或分布式追踪。

### 撤销与演进

本节无 schema、route 或持久数据，撤销只影响 Participation 内核调用方。已有 rule/reason code 一旦被外部观测使用就不得改义复用。

第 27 节将在真实会员分层路由中引入多出口、缺省分支和 path trace；当线性 slice 需要条件跳过、重复节点或隐式 next-index 时，用可执行证据说明 chain 已不够，再在第 28～29 节推进持久化规则树与决策引擎。

## 相关资料

- [Participation 前置资格链基线 v1](../product/participation-prerequisite-chain-v1.md)
- [新用户资格规则基线 v1](../product/new-user-eligibility-v1.md)
- [Lottery 业务规则需求基线 v1](../product/lottery-rule-requirements-v1.md)
- [ADR-0019](ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [ADR-0021](ADR-0021-participation-new-user-eligibility.md)
- [Go Code Review Comments：Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)
- [Go blog：Contexts and structs](https://go.dev/blog/context-and-structs)
- [OASIS XACML 3.0 Standard](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-en.html)
