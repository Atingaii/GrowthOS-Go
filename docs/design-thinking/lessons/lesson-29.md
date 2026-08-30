# 第 29 节设计手记：执行一张图之前，先限定“执行”究竟意味着什么

> 第 28 节已经证明：一份 Lottery Strategy 路由配置可以被保存为 exact `(GraphID, Revision)` 标识、schema-v1、全可达、无环、资源有界、不可原地覆盖的 immutable rooted DAG。第 29 节继续解决下一件、且只解决下一件事：**给定一份明确的合法 graph、一份权威会员事实和一个受控求值时刻，怎样在有限时间与有限步骤内确定地走到唯一 Strategy terminal，并且只在全部证据完整时返回结果。**
>
> 这不是通用规则引擎、线上活动系统或正式抽奖主链。这里没有 graph 发布、active/latest revision、Activity 绑定、真实会员 adapter、HTTP/runtime composition、Strategy 加载、随机选择、Draw、session、RBAC 或前端页面。课程标题里的“规则决策引擎”不能替实现扩大权限；本节实际交付的是 **Lottery bounded context 内、封闭类型、单事实、单路径的 Strategy routing graph evaluator**。
>
> 本手记记录 2026-08-30 的 Lesson 29 实现切片。规范性边界以 [Lottery Strategy Routing Evaluation 基线 v1](../../product/lottery-strategy-routing-evaluation-v1.md) 与 [ADR-0025](../../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md) 为准；实际执行命令、最终验收和远程冻结证据应以本节 QA 为准。本文解释架构推导、反例、取舍和残余风险，不用尚未完成的发布、权限、线上流量或性能数据替当前实现背书。

---

## 1. 第一性起点：持久化成功为什么仍然不能叫“会执行”

### 1.1 第 28 节解决的是可信输入，不是运行结果

第 28 节已经能回答：

```text
这份 revision 是谁？
root 在哪里？
有哪些 node/edge？
拓扑是否全可达、无环、终止且有界？
数据库中的 rows 能否严格恢复成同一领域值？
```

它不能回答：

```text
本次请求读取哪一个 exact revision？
会员事实从哪里取得？
求值采用哪个时刻？
premium 到底走哪条 edge？
多步 path 以什么顺序形成？
执行中取消或超时怎样分类？
失败时是否允许暴露半条 path？
```

如果把“能够 Restore graph”直接说成“规则引擎已经运行”，就把配置完整性与运行语义混成了一层。

### 1.2 数据结构不会自动产生业务语义

一个 decision node 有两条合法 edge：

```text
baseline_default
premium_override
```

仅凭图结构无法知道：

- 哪一种 tier 选择哪一条 branch；
- default 是否承接 unknown；
- edge 排序是否意味着优先级；
- fact provider error 时是否重试或 fallback；
- 到达 Strategy target 后是否继续加载 Strategy。

这些必须由受控代码和明确失败模型定义，而不是从字段名猜。

### 1.3 真正问题

本节的真正问题可以写成：

> 给定 exact immutable graph snapshot、受控 membership fact snapshot、单一 logical evaluated-at 与服务端预算，怎样形成一个确定、可解释、不可部分成功、不会越过 Lottery 所有权的 routing decision？

这里有七个相互独立的维度：

1. **定位：** 哪一份 graph 被执行；
2. **事实：** 哪个 authority 提供什么最小事实；
3. **时间：** 业务求值时刻与技术 deadline 如何分开；
4. **算法：** 怎样确定地从 root 到 terminal；
5. **证据：** 成功结果如何证明自己完整；
6. **故障：** 多个错误同时可见时怎样固定优先级；
7. **停止线：** terminal 之后哪些能力明确不属于本节。

## 2. 本节成功的正向与负向定义

### 2.1 正向定义

一次成功 evaluation 必须形成以下完整链路：

```text
valid caller/context/configuration
  -> exact graph identity
  -> one exact graph read under the evaluation context
  -> returned identity equality
  -> graph Validate
  -> worst-path budget admission
  -> one controlled evaluated-at
  -> one authoritative fresh fact
  -> closed typed branch dispatch
  -> exact edge lookup
  -> iterative bounded path
  -> one Strategy terminal
  -> immutable confirmed decision
  -> final caller/internal context check
```

缺少任何一环，都不是“部分成功”，而是 zero decision。

### 2.2 负向定义

本节成功不等于：

- graph 已经 published 或 active；
- 调用方有权执行或查看 graph；
- MembershipSubjectRef 就是当前登录 Principal；
- route success 就是 Participation eligible；
- StrategyID 已经加载、发布或不可变；
- Award 已被抽中；
- Draw 已持久化；
- path 是合规审计记录；
- 单元测试证明生产 P99、QPS 或高可用；
- 一个 typed operator 已经证明需要通用 registry；
- 多个相同 membership decision 已经构成多条件规则平台。

### 2.3 为什么负向定义必须写在前面

工程事故常见于能力名称扩大：

```text
graph evaluator
  被说成 rule engine
  被说成 activity decision service
  被说成 lottery draw engine
  被直接接进 API
```

每次名称升级都偷偷增加新的所有权、错误和安全假设。先写负向定义，可以让代码评审知道什么样的“顺手集成”其实是越界。

## 3. 证据链：本节不是凭空发明引擎

### 3.1 第 27 节提供语义 oracle

第 27 节已经冻结：

```text
confirmed premium
  -> premium_override
  -> premium Strategy target

confirmed standard
  -> baseline_default
  -> baseline Strategy target
```

并明确：

- unknown/unsupported tier 失败；
- future/stale/corrupt fact 失败；
- provider failure 失败；
- cancellation 失败；
- default 不是异常兜底；
- route 只返回 Strategy identity。

这段 concrete router 是本节的行为 oracle。Graph evaluator 不能因为“更抽象”而重新解释 default。

### 3.2 第 28 节提供结构 oracle

第 28 节冻结：

- exact `GraphID + Revision`；
- schema version 1；
- root 必须是 decision；
- node kind 只允许 decision / strategy_target；
- rule code 只允许 `lottery.membership_tier.route_strategy`；
- branch 只允许 `premium_override` / `baseline_default`；
- 每个 decision 恰好两条 edge；
- terminal 无出边；
- 全可达、无环；
- node/edge/depth 上限为 128/256/16；
- Create 与 Restore 都重新建立完整领域不变量。

本节不重新发明图格式，也不从 SQL rows 边走边猜。

### 3.3 本节新增的唯一缺口

两层 oracle 合并后，仍缺少：

```text
graph structure
  + membership branch semantics
  + runtime budget/cancellation
  + immutable evidence
```

第 29 节只补这条执行边。

## 4. 事实所有权：执行器不是事实的主人

### 4.1 所有权矩阵

| 事实或决定 | 权威所有者 | 本节如何使用 | 本节不能宣布 |
| --- | --- | --- | --- |
| graph topology | Lottery | 按 exact identity 读取并复核 | active/published |
| membership tier fact | 外部会员 authority | 通过 Lottery-owned consumer port 读一次 | 会员生命周期、角色 |
| fact freshness policy | Lottery application | 用 server Clock 与 maxFactAge 判断可用性 | provider SLA |
| tier -> branch 语义 | Lottery domain | 封闭 typed helper | 通用规则语言 |
| branch -> successor | immutable graph | exact branch lookup | edge priority |
| Strategy target identity | Lottery graph terminal | 返回非零 StrategyID | Strategy 内容/版本/可抽奖 |
| Activity 生效关系 | Marketing / 第 30 节 | 本节不使用 | active graph |
| caller Principal/权限 | Governance / 第 31～35 节 | 本节不建模 | access allow |
| Participation 资格 | Participation | 本节不使用 | eligible/reject |
| Award/Draw/Benefit | 各自领域 | 本节不调用 | 正式业务结果 |

### 4.2 MembershipSubjectRef 为什么不是 Principal

`MembershipSubjectRef` 是 Lottery 查询会员 authority 的 opaque 业务引用。它只回答：

> 我要读取哪个会员主体的 tier fact？

它不回答：

> 当前是谁登录？他能否替这个主体查询？他能否读取 graph path？他属于哪个 role？

把 subject ref 直接当 Principal，会让后续 session/RBAC 无法区分“操作人”和“被路由的业务对象”。

### 4.3 graph identity 为什么不是授权范围

exact `(GraphID, Revision)` 只定位内容，不代表调用者有权查看、求值、发布、绑定 Activity 或查询 path。

第 29 节没有公开入口，因此暂时不存在 server authorization enforcement。这个缺口被刻意保留给第 31～35 节，而不是默认“内部调用都可信”后永远不补。

### 4.4 Strategy target 为什么只返回 ID

Terminal 的职责是：

```text
结束路由
  + 暴露 Strategy identity
```

它不是：

```text
加载 Strategy aggregate
  -> 获取 Awards
  -> 生成随机 ticket
  -> 选择 Award
  -> 持久化 Draw
```

将这些步骤拆开，才能分别处理 Strategy not found、cache freshness、random source failure、inventory、idempotency、benefit delivery 和正式结果审计。

## 5. 为什么名称是 Strategy Routing Graph Evaluation

### 5.1 名称里的每个词都在限制能力

- `Strategy`：terminal 只指向 Lottery Strategy；
- `Routing`：形成目标选择，不做资格或发奖；
- `Graph`：执行第 28 节的显式 topology；
- `Evaluation`：纯求值得到内存 decision，不发布、不持久化副作用。

### 5.2 为什么不导出 `RuleEngine`

一个真正的通用规则引擎至少要回答：

- operator 如何注册、版本化与冲突处理；
- fact 是否强类型；
- null/unknown 使用几值逻辑；
- expression 如何解析；
- 数字、字符串、时间怎样比较；
- 外部调用是否允许；
- operator cost 怎样计费；
- timeout/cancel 怎样传播；
- side effect 怎样禁止或补偿；
- schema 与 evaluator 怎样兼容；
- trace 和指标如何避免泄露与高基数。

当前只有一个已证明的 membership operator。现在导出 `RuleEngine` 只会用一个案例替十几个未决问题作假设。

### 5.3 为什么不建立 `internal/rules`

如果过早放入 shared package：

- Participation 容易把 eligibility 塞进 routing；
- Governance 容易把 permission 当会员 tier；
- Inventory 容易把库存副作用塞进 operator；
- Lottery vocabulary 会伪装成跨上下文标准；
- 失败分类失去领域归属。

保持在 `internal/lottery/domain` 和 `internal/lottery/application`，是用包边界表达业务所有权。

## 6. 两层执行边界：Application 组合，Domain 求值

### 6.1 Application 应负责什么

`StrategyRoutingGraphEvaluationService` 负责：

- 验证 caller 输入和 service configuration；
- 创建内部 duration budget；
- exact graph read；
- graph identity equality；
- graph Validate；
- graph worst-depth admission；
- 捕获一个 controlled business time；
- 读取并校验一个 fresh membership fact；
- 调用 domain evaluator；
- 应用 caller/internal/provider 错误优先级；
- 将 domain 细节收敛为低披露 application error；
- 最终成功前再次检查 context。

这些事情依赖端口、配置与调用生命周期，属于 application orchestration。

### 6.2 Domain 应负责什么

`EvaluateStrategyRoutingGraph` 负责：

- 只接受领域值；
- 再次校验 graph/fact/budget；
- 规范 evaluated-at；
- 从 root 开始迭代；
- closed typed branch evaluation；
- exact edge lookup；
- step/cycle/context hard stop；
- 构造完整 path；
- 到 terminal 形成 immutable decision；
- 失败始终返回 zero decision。

它不读取 Repository、Clock 或外部 provider，也不知道 HTTP/session/Activity。

Domain evaluator 没有 `maxFactAge` 参数：它验证 fact 结构，并拒绝 `ObservedAt > evaluatedAt`，但不会自行判断“多久以前算 stale”。Freshness policy 由 application 的 `readFreshMembershipTierFact` 在调用 domain 前执行。因此直接调用 domain evaluator 只能声称使用了 structurally valid、not-from-future 的 fact，不能绕开 application 后仍声称满足本 service 的 freshness contract。

### 6.3 为什么不把全部逻辑放 Application

如果遍历只写在 service：

- graph 语义与 I/O 顺序耦合；
- 纯拓扑反例必须构造 stubs；
- 其他受信 caller 可能绕过同一求值规则；
- path/decision invariant 更难集中；
- domain 不再拥有 branch 到 edge 的关系。

### 6.4 为什么不把 Clock/Repository 放 Domain

Domain 若直接读取 `time.Now()`、graph reader 或 membership fact reader，同样输入就不再得到同样输出，单次 evaluation 也难以重放。这里的端口只表达读取契约；本节没有假设它一定是本地、远程、HTTP 或某种特定存储。

Domain 应接收已经取得的 values 与 context cancellation 信号，而不是取得 values 的能力。

### 6.5 两层都 Validate 是否重复

重复是有意的：

| 层 | 防守的入口 |
| --- | --- |
| Repository Restore | 不可信数据库 rows |
| Application | 不可信/错误 port 返回值与 use-case 配置 |
| Domain evaluator | 任意直接领域调用 |

每层只依赖自己的前置条件，避免“只有经过某一条 composition 才安全”的隐藏约束。

## 7. Exact identity：拒绝“帮你选一个能用的版本”

### 7.1 输入必须完整

Service 接收已经构造并验证的 `StrategyRoutingGraphIdentity`：

```text
GraphID != 0
Revision 符合 exact ASCII grammar
```

Reader 只有：

```go
FindByIdentity(ctx, identity)
```

没有：

```go
FindLatest
FindActive
FindFallback
ListThenChoose
```

### 7.2 为什么 not-found 不尝试另一个 revision

假设请求的是 `graph=71, revision=r1`，但 r1 不存在。若 service 自动选择 r2：

1. caller 以为执行 r1；
2. 实际 path 来自 r2；
3. decision identity 要么撒谎，要么暴露替换；
4. 历史解释无法重放；
5. Activity 发布责任被 evaluator 偷走。

not-found 必须是 not-found。

### 7.3 Reader 返回值仍要验证 identity

即使 port 方法名是 `FindByIdentity`，adapter bug 或 test double 仍可能返回另一张图。Service 显式要求：

```text
returnedGraph.Identity() == requestedIdentity
```

这不是多余防御。端口描述意图，返回值校验证明事实。

### 7.4 读取一次的意义

一次调用最多一次 graph read，避免：

- path 中途切 revision；
- not-found 后 fallback；
- 每 node I/O；
- graph 变化与 fact 变化交叉；
- provider latency 按 step 放大。

### 7.5 为什么不在 service 内缓存 latest

Exact immutable revision 未来可以被缓存，但缓存 key、容量、负缓存和失效仍需证据。本节没有 runtime read workload；加入 latest cache 更会偷渡发布语义。因此当前 reader 只表达一次 exact lookup。

## 8. 一次求值固定三个 snapshot

### 8.1 三个 snapshot

```text
one exact immutable graph
  + one canonical evaluated-at
  + one authoritative membership fact
  -> one decision or one error
```

这里的 snapshot 是应用层一致观察，不是跨系统分布式事务。

“authoritative membership fact”在这里指 `MembershipTierFactReader` 的消费端契约和 fact provenance，不表示 Lesson 29 已经装配了某个真实会员服务、网络 client 或生产数据源；当前 application test 源码使用的是受控 test doubles，实际命令结果仍见 QA。

### 8.2 依赖顺序为什么是 graph -> Clock -> fact

顺序不是偶然：

1. graph 不存在或损坏时，不应读取会员派生事实；
2. graph depth 超出 service budget 时，也不应读取会员事实；
3. graph 被接受后，Clock 形成唯一 freshness 基准；
4. fact 再相对该时刻验证 future/stale。

成功调用的依赖顺序被测试冻结为：

```text
graph
clock
fact
```

### 8.3 为什么 Clock 只调用一次

如果在读 fact 前后各取一次时间：

- future 判断使用 t1；
- freshness 判断使用 t2；
- decision evidence 可能记录 t3；
- 边界上的同一 fact 可前后改变分类。

一次 logical Clock 让业务判断和证据共享同一时刻。

### 8.4 为什么 fact 只读一次

schema v1 即使走 16 个 decision，也复用同一 `MembershipTierFactSnapshot`。

否则可能出现：

```text
node 1 读取 premium
会员等级变化
node 2 读取 standard
```

同一 path 将同时声称两种会员事实。一次 fact read 让 route 对一个明确 provider revision 可解释。

### 8.5 一致性的诚实边界

Graph reader、fact reader 与 application Clock 只通过各自端口协作；端口没有声明它们共享事务、存储或一致性 token，所以本节不能声称：

- graph 与 fact 在同一数据库时刻；
- provider 支持历史 as-of；
- Activity 发布和会员事实原子；
- 跨端口 linearizability。

我们只保证 application 不主动在 path 中混用多份 graph/fact/time。

## 9. 共享 membership branch helper：复用语义，不制造平台

### 9.1 被抽取的最小函数

现有 concrete router 和新 graph evaluator 共用 package-private：

```text
fact + evaluatedAt
  -> exact branch + stable reason
```

它只认识：

```text
standard -> baseline_default / baseline_strategy_selected
premium  -> premium_override / premium_strategy_selected
```

### 9.2 为什么 helper 不返回 target

第 27 节 policy target 与第 29 节 graph edge target 的来源不同：

- concrete router 从 `MembershipStrategyRoutingPolicy` 取 target；
- graph evaluator 从 selected edge 的 successor terminal 取 target。

共享 target selection 会再次绕过 graph topology。共享的正确粒度只是 tier-to-branch 语义。

### 9.3 为什么 helper 是 package-private

如果导出为 `Rule` 接口：

- 外部包开始依赖尚未成熟的抽象；
- 第二个 operator 会被迫迁就第一个签名；
- registry/metadata/version 问题提前出现。

package-private helper 能消除两个 switch 漂移，却不形成新的跨上下文承诺。

### 9.4 helper 仍然校验 fact 与 time

Domain evaluator 在入口校验 fact/time，helper 在每个 decision 又检查同一 typed contract。最多 16 次、常量规模的重复校验，换来：

- concrete router 独立调用仍安全；
- helper 不依赖“某个调用方一定预校验”；
- 未来 refactor 不容易绕开 zero/future guard。

这不是性能热点证据。若未来 profiling 证明重复校验有成本，再用测量触发重构。

## 10. 为什么 evaluator 入口还要 `graph.Validate()`

### 10.1 “Repository 已 Restore”不是普遍前提

Domain evaluator 也可能由单元测试、未来内存 adapter、application 直接构造的 graph 或同包错误代码调用。只信任某一个 Repository adapter 会把 domain 安全性绑定到一条特定调用链。

### 10.2 双重校验分别防守什么

```text
Repository Restore Validate
  防守 stored rows -> domain value

Application graph.Validate
  防守 port 返回值与错误 adapter

Domain evaluator graph.Validate
  防守所有直接 caller
```

每一层都不是在宣称上一层无用，而是在自己的入口重新建立前置条件。

### 10.3 成本为何可接受

Graph 已被静态限制：

- nodes <= 128；
- edges <= 256；
- depth <= 16。

重新 Validate 的绝对成本有界。当前没有 profiling 证明它是热点，正确性优先于提前 compiled-plan cache。

### 10.4 不能因此删除运行 guard

graph Validate 成功仍不能删除：

- actual step guard；
- path-local visited set；
- context checkpoints；
- exact selected-edge match；
- terminal decision confirmation。

这些是 execution defense-in-depth，防止未来 validator、accessor 或构造链回归。

## 11. DAG 的执行语义：验证全图，执行单路径

### 11.1 静态验证与动态遍历不同

第 28 节验证完整 DAG：

```text
所有 node/edge
  -> 全可达
  -> 无环
  -> 最长深度
```

第 29 节一次求值只走：

```text
root
  -> selected edge
  -> selected edge
  -> one terminal
```

因此 evaluator 不是 DFS，也不枚举全部 path。

### 11.2 为什么 shared successor 合法

两个 branch 可以汇聚：

```text
premium  ----+
              +--> terminal
baseline ----+
```

或汇聚到下一 decision。静态 graph 是 DAG；单次 evaluation 只从其中一个父路径到达 shared successor。

### 11.3 为什么不并行执行 branch

Branch 语义是互斥 route，不是 all-match、first-success、highest-score、parallel join 或 conflict resolution。

同时执行两条 edge 再挑结果，会执行未选择路径、放大成本、破坏 path 证据，并为 future side-effect node 埋下风险。

### 11.4 到达 terminal 必须立即停止

Terminal 后不应扫描其他 node、检查未走 branch、加载 Strategy、调用 selector、写审计或发布 event。

完整 graph 的合法性已在 Validate 阶段证明；evaluation 只需对实际 path 形成结果。

## 12. 为什么采用迭代循环而不是递归

### 12.1 迭代状态清晰可见

实现显式维护：

```text
currentNodeID
visited
path
step count = len(path)
```

每一轮：

1. 检查 context；
2. 读取 current node；
3. terminal 则尝试形成完整 decision；
4. decision 则检查 budget；
5. closed dispatch 求 branch；
6. exact lookup edge；
7. 追加完整 step；
8. 移动 current node。

### 12.2 递归并非因为深度 16 就不安全

深度 16 下递归栈本身可控。拒绝递归的主要原因不是 stack overflow，而是审计性：

- step budget 在哪里扣减；
- cancellation 在哪里检查；
- partial path 谁拥有；
- terminal 前后谁验证；
- error 返回时 path 是否泄露。

迭代让这些控制点在同一函数中顺序可见。

### 12.3 迭代也不是自动正确

仍需防止：

- current node missing；
- unknown node kind；
- selected successor missing；
- path revisit；
- edge from 与 current 不一致；
- budget 在 append 后才检查；
- terminal target 为 zero；
- `len(path)==0` 的伪成功。

所以算法形态只是可审查性的工具，不替代 invariant。

## 13. Exact branch lookup：最小但最关键的正确性决定

### 13.1 canonical order 不是业务优先级

Graph accessor 的 edge 可能按稳定 code 排序。按字典序：

```text
baseline_default
premium_override
```

baseline 很可能排在前面。若 evaluator 使用 `edges[0]`，premium 会静默进入 baseline。

### 13.2 正确协议

先由 typed helper 得到 `selectedBranch`，再遍历当前 node 的 outgoing edges，要求：

```text
edge.Branch() == selectedBranch
matches == 1
```

最后再验证 selected edge 自身与 current node/branch 一致。

### 13.3 为什么不扫描 `IsDefault()`

`is_default=true` 是 graph 结构属性，只证明 baseline edge 被正确标记。它不表示 unknown、provider error、missing premium edge 或 unsupported operator 都应该走 default。

Default 的业务含义仍来自 confirmed standard。找不到 selected branch 是 invariant breach，不是 fallback 机会。

### 13.4 为什么要求恰好一个 match

Graph Validate 已经拒绝 duplicate branch，但 evaluator 仍检查 `matches == 1`：

- 0 表示 selected branch 不存在；
- 大于 1 表示 aggregate/accessor 被伪造或回归；
- 任一情况都不能猜。

这让 branch selection 和 edge resolution 之间有清晰的信任边界。

## 14. Path 是实际证据，不是 debug trace

### 14.1 每一步保存什么

`StrategyRoutingGraphPathStep` 保存：

```text
from_node_id
rule_code
selected_branch
reason_code
to_node_id
```

它回答：

> 在哪个 decision，用哪个封闭 rule，为什么选择哪条 branch，走到了哪里？

### 14.2 为什么 reason 与 branch 都要保留

Branch 是机器拓扑 identity；reason 是稳定业务解释：

```text
premium_override -> premium_strategy_selected
baseline_default -> baseline_strategy_selected
```

两者配对进入 `Confirmed()`，防止 premium branch 配 baseline reason、文案变化改写机器语义，或 converged target 擦除到达原因。

### 14.3 汇聚 target 为什么更需要 path

如果 premium 和 standard 都到 Strategy 808，单看 `Target = 808` 无法知道实际 branch。

Path 仍保留 `premium_override` 或 `baseline_default`。这对解释、测试与未来受权审计都重要，但当前 path 尚未持久化或公开。

### 14.4 为什么不保存完整 graph

Decision 已携带 exact identity，完整 graph 仍由权威 Repository 管理。复制全图会放大结果、暴露未走分支、增加配置披露、制造双真相，并诱导把内存结果当审计快照。

### 14.5 为什么不保存 subject

Path 中 branch 已属于会员派生信息，再放 subject 会形成更敏感的可关联记录。当前 evaluation 只需证明 fact provenance，不需让结果成为“谁是 premium”的查询索引。

## 15. Immutable decision：完整性必须能被对象自己拒绝

### 15.1 成功 decision 的组成

`StrategyRoutingGraphDecision` 内部保存：

- exact graph identity；
- schema version；
- root node ID；
- terminal node ID；
- Strategy target；
- terminal target 的内部一致性副本；
- fact source；
- fact revision；
- canonical UTC evaluated-at；
- step budget；
- ordered path。

### 15.2 `Confirmed()` 验证什么

至少验证：

- identity 可验证；
- schema 恰好 v1；
- root/terminal 非零且不同；
- target 非零且与内部 terminal target 一致；
- step budget 合法；
- fact source/revision token 合法；
- evaluated-at 非零且 canonical；
- path 长度在 1..budget；
- 第一条从 root 出发；
- steps 连续；
- rule/branch/reason 是 exact v1 配对；
- path 不重复 node；
- 最后一条到 terminal。

### 15.3 `Confirmed()` 不能单独证明什么

Decision 不携带完整 graph，所以 `Confirmed()` 单独不能重新证明每个 step 真的是 graph 中的 edge、terminal node 在 graph 中对应 target，或 graph revision 内容未被外部篡改。

这些由 evaluator 构造前对 graph 的 exact lookup 保证，再依靠 decision fields 私有、无公开 constructor/setter、path defensive copy 与 graph revision create-only 维持。

这是重要的诚实边界：`Confirmed()` 是内部一致性检查，不是密码学证明或独立审计验证器。

### 15.4 为什么保存 stepBudget

Decision 需要知道 path 被哪个上限确认，才能检查 `len(path) <= maxSteps`。

但 budget 不是业务事实，不对外声称为 Activity 配置或生产 SLO。它目前只是 decision 内部完整性的一部分。

### 15.5 Defensive copy 是并发和不可变的共同前提

`Path()` 返回新 slice。否则 caller 可以改写内部 step，或与并发 reader 形成数据竞争。

字段私有并不自动让 slice 不可变；防御性复制才切断底层数组别名。

## 16. Zero decision：把“没有完整证明”编码为不能使用

### 16.1 所有失败返回同一种值形态

无论失败发生在 input、configuration、graph read、graph Validate、depth admission、Clock、fact read、fact freshness、branch dispatch、edge lookup、budget、cancellation 或 decision confirmation，返回值都是 `StrategyRoutingGraphDecision{}`。

### 16.2 为什么不能返回 partial path + error

Partial path 很容易被误用：

```text
走到 node 5 失败
上层看到最后一个“似乎有效”的节点或 target
把 prefix 当 route success
```

即使类型上没有 terminal target，日志/handler 也可能把 prefix 当业务解释。Zero result 让失败值没有可用分支、target 或 provenance。

### 16.3 为什么不能返回 fallback target

Fallback target 会混淆业务 standard、provider unavailable、graph corruption、timeout 与 unsupported operator。

它提高表面成功率，却降低正确性和可调查性。

### 16.4 与事务思想的类比

本节没有数据库副作用，但结果形成采用类似事务的原子边界：

```text
局部计算只存在于函数栈
  -> terminal + path + context + invariant 全部确认
  -> 一次性返回完整 decision
```

失败时局部 path 被丢弃，不存在 commit outcome unknown。

### 16.5 Zero decision 不是什么

Zero decision 不是业务 reject、standard route、no reward、authorization denied、Activity inactive 或 retry instruction。

它只表示：没有形成可信的 Lottery routing decision。

## 17. 两套运行预算，加上一套静态图预算

### 17.1 静态预算

第 28 节 graph schema 限制：

| 项目 | 上限 |
| --- | ---: |
| nodes | 128 |
| edges | 256 |
| longest depth | 16 edges |

它回答：

> 这份配置能否被安全构造、恢复和遍历？

### 17.2 运行步数预算

`StrategyRoutingGraphStepBudget` 限制：

```text
1 <= maxSteps <= 16
```

它回答：

> 当前 service 愿意为一次 evaluation 承诺多少 decision edges？

### 17.3 运行时间预算

positive `maxDuration` 在输入、service 配置和 pre-cancel 检查通过后创建 child context；它定义从该点到最终返回前 orchestration 的 cooperative deadline window：

- graph read；
- Clock boundary；
- fact read；
- domain traversal；
- 最终返回前检查。

Graph reader 与 fact reader 会收到 child context；domain 会检查 context。`MembershipRoutingClock.Now()` 本身不接收 context，service 只能在它返回后发现 deadline 已过，不能用该 deadline 抢占一个阻塞的 Clock 实现。

它表达的策略是：

> caller 仍存活时，这个 service 愿意给本次 evaluation 多少协作式时间预算？

它不是对实际 wall-clock 返回时长的硬上限；不合作的 reader 或 Clock 仍可能超过该时间才返回。

### 17.4 为什么三者不能合并

- depth 16 不代表 provider 一定在 16ms 内响应；
- maxDuration 1s 不允许 depth 17 graph；
- maxSteps 8 不表示 graph nodes 上限也是 8；
- static graph budget 不拥有 caller deadline；
- deadline 不知道每个 step 的拓扑成本。

把它们分开，才能知道失败来自配置结构、service policy 还是调用生命周期。

## 18. Step 的精确定义与 off-by-one 防线

### 18.1 一个 step 是什么

一个 step 精确表示：

> 在一个 decision node 上形成 branch，并沿唯一匹配 edge 移动到 successor。

因此：

- root node 本身不先计 1；
- decision -> terminal 是 1 step；
- terminal 不额外计 step；
- 未选择 edge 不计；
- graph/Clock/fact I/O 不计 step；
- path length 等于实际 step count。

### 18.2 为什么在 dispatch 前检查

Loop 采用：

```text
if len(path) >= maxSteps and current is still decision:
    fail before evaluating next branch
```

若 append 后才检查，会执行第 `maxSteps+1` 个 operator，再宣布超限，违反资源边界。

### 18.3 恰好等于预算应成功

深度 16 graph 在 `maxSteps=16`：

- 第 16 个 decision 可以 dispatch；
- 第 16 条 edge 可以 append；
- 到达 terminal 后成功；
- terminal 不消耗第 17 步。

这类边界必须有显式测试，不能只测“小于”和“超出”。

### 18.4 Zero 为什么不能表示 unlimited

Zero value 在 Go 中很容易由未初始化 field、漏传 config、nil service 或 decode 缺失产生。若 zero 表示 unlimited，配置错误会自动关闭安全上限。这里 zero 必须 invalid。

## 19. 为什么用 graph 最坏深度做 service 准入

### 19.1 看似可行的 actual-path-only 方案

假设 graph：

```text
standard path = 1 step
premium path  = 8 steps
service maxSteps = 4
```

如果只按实际 path 检查：

- standard 成功；
- premium 走到第 4 步后失败。

同一 graph revision 在同一 service 配置下，是否具备执行能力取决于会员事实。

### 19.2 当前选择

Service 读取并 Validate graph 后先要求：

```text
graph.Depth() <= maxSteps
```

若不满足，在 Clock 和 fact read 前失败。

### 19.3 为什么这是能力准入而非性能优化

主要目的不是少一次 provider 调用，而是建立承诺：

> 只要 graph 被这个 service 接受，任一合法事实对应的路径都在 step budget 内。

这避免某类会员总是在运行中途遇到预算失败，也让第 30 节未来发布时可以检查 Activity 使用的 service budget。

### 19.4 Actual hard stop 为什么仍不能删

Domain loop 仍在每次 dispatch 前检查 actual step count。它防止 forged graph depth、validator 回归、accessor/graph invariant 失效，或 application admission 被其他 domain caller 绕过。

准入和 hard stop 是两层不同保证。

### 19.5 方案的代价

一个 graph 即使当前请求必走浅路径，只要另一条合法路径更深，就会整体被低 budget service 拒绝。这是当前有意选择的一致可用性。

若未来 Activity 明确允许 tier-specific partial capability，必须新增产品决定，不能悄悄删除 worst-case admission。

## 20. 两种时间：业务 evaluated-at 与技术 deadline

### 20.1 业务时间

注入的 `MembershipRoutingClock` 形成：

- fact future 判断；
- fact freshness 判断；
- decision evidence 的 evaluated-at。

它被规范为 UTC 并去除 monotonic 部分，便于稳定比较和存证。

### 20.2 技术时间

Service 使用当前进程时间计算：

```text
internalDeadline = time.Now() + maxDuration
```

它给 application 调用设置协作式 deadline，不进入业务 decision。能否及时停止仍取决于各 reader 是否遵守 context；无 context 参数的 Clock 只能在返回后被检查。

### 20.3 为什么不能共用一个 Clock

若用业务 Clock 计算 context deadline：

- 测试固定时间可能已经在过去；
- 业务时钟回放会立刻超时；
- wall-clock/monotonic 语义混淆；
- evaluated-at 被错误解释为资源预算起点。

若用 `time.Now()` 作为业务 evaluated-at：

- domain 测试难以稳定；
- provider future/freshness 难以重放；
- 同一输入无法产生同一证据。

两种时间服务不同目的，必须分离。

### 20.4 maxDuration 不是 P99

它只是 cooperative safety budget，不代表 99% 调用能在该时间内完成、provider SLA、production latency、hard real-time 或 goroutine 必然被抢占。

没有 runtime 和生产负载之前，不能拿构造参数冒充性能承诺。

## 21. Deadline 所有权：相同时间也必须有确定规则

### 21.1 直觉写法的问题

最简单写法可能是：

```go
context.WithTimeout(callerCtx, maxDuration)
```

但 caller 也可能已经有 deadline。若 caller deadline 与 internal deadline 完全相同，两套 timer 的调度顺序可能不同。

Go context 保留首先发生的 cancellation cause。不能用 scheduler 偶然性定义业务错误分类。

### 21.2 当前绝对时间比较

实现先计算 internal deadline，再读取 caller deadline：

```text
caller deadline <= internal deadline
  -> context.WithCancel(caller)
  -> caller 拥有 deadline/cause

internal deadline < caller deadline
  -> context.WithDeadlineCause(caller, internal, privateCause)
  -> service 拥有更早 deadline
```

“相等时 caller 获胜”是显式决策，而不是 timer race 的结果。

### 21.3 为什么 caller 更早时仍派生 child

`context.WithCancel(caller)` 保留 caller value/deadline/cancellation，给 service 一个 cleanup function，不安装竞争的内部 timer；dependency 接收 child，而语义仍由 caller 所有。

### 21.4 私有 cause 的作用

只有 internal deadline 严格更早时安装 package-private cause：

```text
lottery strategy routing graph evaluation: internal deadline
```

Application 通过 identity 比较识别自己的 budget exhaustion，不靠字符串或 `context.DeadlineExceeded` 猜。

### 21.5 cleanup cancellation 不能冒充 timeout

每次调用 `defer cleanup()`。主动 cleanup 产生 cancellation，但 cause 不是 private internal deadline。因此 classifier 不应把正常清理映射成 timeout。

测试显式覆盖这个反例，防止以后“child Err 非 nil 就是内部超时”的简化。

## 22. Context 取消是协作式，不是硬中断

### 22.1 Service 能保证什么

- child context 传给 graph reader；
- 同一个 child 传给 fact reader；
- 依赖返回后重新检查 caller/internal 状态；
- domain 在遍历边界检查 context；
- success 返回前最后检查一次。

### 22.2 Service 不能保证什么

- 不保证忽略 context 的 reader 会立刻停止；
- 不保证无 context 参数的 Clock 能在 deadline 到期时被抢占；
- 已经进入的同步函数被 Go 强制抢占；
- 最终检查之后、return 被 caller 收到之前发生的取消能撤回内存值；
- 外部 side effect 被回滚。

v1 operator 是常量规模纯计算，path 最多 16，因此本地不可抢占窗口被限制在很小范围。但这仍不是 hard real-time guarantee。

### 22.3 为什么每个 decision 前检查

如果只在 I/O 前后检查，深 path 在 caller 已取消后仍可能做多次本地工作。每轮检查使取消成本与一个小 step 边界对齐。

### 22.4 为什么 success 前还要检查

Terminal decision 可能已经构造并通过 `Confirmed()`，但 caller 恰好在形成结果时取消。最后 checkpoint 确保已可观察的 cancellation 比成功返回优先。

Domain 测试不是模糊地“取消一次”，而是校准 depth-1 success 的 checkpoint，并锁定最终返回前的检查位置。

## 23. 错误优先级：谁拥有失败必须稳定

### 23.1 同时可见的事实

依赖返回时可能同时出现 caller 已取消、internal child deadline 已到、provider 返回 error、provider 同时返回 value，或 returned value 自身非法。

如果按“先写的 if”随意处理，错误责任会随 refactor 改变。

### 23.2 固定优先级

| 顺序 | 条件 | 返回语义 |
| ---: | --- | --- |
| 1 | original caller context 已结束 | 原样 caller error |
| 2 | caller live，evaluation child 因内部 budget 结束 | stable internal timeout |
| 3 | 两个 context live，dependency 返回 error | provider/repository 稳定类别 |
| 4 | 无 error，但 returned value 非法 | invalid graph/fact/decision |
| 5 | context live 且完整值合法 | 继续或成功 |

### 23.3 Caller error 为什么原样返回

Caller 拥有自己的 lifecycle。将 `context.Canceled` 或 caller `DeadlineExceeded` 包装成 provider unavailable，会让上层误判可重试性与责任。

### 23.4 Internal timeout 为什么不等于 dependency-owned timeout

Caller live，而 service 的 maxDuration 到期，说明本 use case 的总体预算耗尽。即使 dependency 随后返回普通 error，internal budget 仍优先。

### 23.5 Dependency-owned deadline 为什么仍是 dependency failure

如果 graph/fact reader 返回一个包含 `context.DeadlineExceeded` 的 error，但 caller 和 evaluation child 都 live，它只能被视为该 dependency 自己拥有的失败；端口没有证明它来自远程 transport，也没有证明它使用了哪一种内部预算。

此时不能冒充 caller deadline 或 service internal timeout。Graph reader 的这类未知 deadline error 映射为 evaluation failure；membership fact reader 沿既有 fact boundary 映射 unavailable。二者都不走 default。

### 23.6 Error + value 为什么 error 胜出

Provider 明确返回 error 时，value 可能是 stale cache、partial decode、fallback object 或上一次调用残留。

继续使用 value 会违反 dependency contract。Graph error 时 Clock/fact 保持零调用；fact error 时 traversal 保持零调用。

## 24. 低披露错误：稳定类别与可信诊断不是二选一

### 24.1 原始 cause 的风险

Graph Repository 或 domain error 可能包含：

- GraphID/Revision；
- NodeID；
- branch/rule；
- SQL/table/constraint；
- adapter/storage implementation detail；
- membership provider detail；
- subject/tier/payload；
- private internal deadline marker。

如果 application 直接用 `fmt.Errorf("%w", cause)` 形成公开链，上层 `errors.Is`、日志或未来 transport 可能意外暴露这些细节。

### 24.2 Application wrapper 的两个通道

`StrategyRoutingGraphEvaluationError` 保存：

```text
class: 经过评审的稳定语义
cause: 受信诊断细节
```

普通调用：

- `Error()` 只输出 class 文本；
- `errors.Is` 只匹配 class；
- 没有 `Unwrap()`。

受信诊断必须显式 `errors.As` 到具体 wrapper，再调用 `Cause()`。

### 24.3 为什么没有 `Unwrap()`

如果实现 `Unwrap() error`，那么：

```go
errors.Is(publicError, context.DeadlineExceeded)
```

可能穿透 private cause，导致 internal timeout 被误判成 caller/provider deadline。也可能让 SQL/storage error 进入上层分支。

“保留 cause”不等于“让整个错误链公开可匹配”。

### 24.4 Unknown public class 为什么降为 generic failure

Wrapper 只接受白名单 class。调用方若误传一个新的、未经评审的 error 作为 class，wrapper 降级为 `ErrStrategyRoutingGraphEvaluationFailure`。

这样新增 raw error 不会因为一次 refactor 自动成为公共协议。

### 24.5 当前稳定类别的边界

Application graph evaluation 至少区分：

- invalid argument；
- not configured；
- graph not found；
- graph invalid；
- internal timed out；
- decision invalid；
- general failure；
- domain step budget exceeded；
- 既有 membership fact not-found/unavailable/invalid/stale classes；
- raw caller context errors。

这些类别仍不是 HTTP status mapping，因为本节没有 transport。

### 24.6 Domain error 细节与 Application 低披露的关系

Domain 内部 error 可以携带 node/branch detail，帮助受信开发调试。Application 把它包进不展开的 cause。

这说明错误披露应在信任边界收敛，而不是要求 domain 永远只返回一句“failed”，也不是让 transport 直接序列化 domain error。

## 25. Service 配置验证：错误装配不能等到第一次请求

### 25.1 构造器验证的字段

Service 只有在以下条件完整时可构造：

- graph reader 非 nil/typed-nil；
- membership fact reader 非 nil/typed-nil；
- Clock 非 nil/typed-nil；
- maxFactAge > 0；
- step budget Validate 成功；
- maxDuration > 0。

### 25.2 为什么 typed-nil 需要单独防守

Go interface 可能满足：

```go
var reader *ConcreteReader = nil
var port StrategyRoutingGraphReader = reader
```

此时 `port != nil`，但调用 method 可能 panic。项目复用 `dependencyIsNil` 收敛 nil 与 typed-nil。

### 25.3 为什么 constructor 与 `Validate()` 都存在

- constructor 防止正常调用者拿到坏 service；
- `Validate()` 防止 zero/同包伪造 service；
- `Evaluate()` 每次入口再检查，避免 nil receiver panic。

这是 Go zero-value 现实下的 defense-in-depth。

### 25.4 为什么 maxFactAge 属于 service，而不是 graph

Fact freshness 是 Lottery application 对外部 authority 的使用政策。把它写进 graph 会：

- 让业务 topology 控制 provider trust policy；
- 使不同 runtime 不能独立配置；
- revision 内容混入技术/集成参数；
- 运营可通过 graph 放宽 stale window。

当前由 server configuration 注入。

### 25.5 为什么本节没有环境变量

Service 尚未 runtime-wired，所以没有理由猜 production maxDuration、maxSteps 默认值、maxFactAge 环境名或 per-Activity override。

测试显式构造服务即可。真正 composition 出现时，再由上层总预算和运行证据决定配置表面。

## 26. 共享 fresh-fact session：重构必须保持旧行为

### 26.1 为什么抽取 `readFreshMembershipTierFact`

第 27 节 concrete service 与第 29 节 graph service 都需要：

```text
one Clock
  -> one fact read
  -> provider error classification
  -> fact Validate
  -> subject equality
  -> future/stale checks
```

复制这段 orchestration 会产生两套 freshness 与错误语义。

### 26.2 helper 拥有什么，不拥有什么

Helper 拥有：

- 捕获 canonical evaluated-at；
- 读取一次 fact；
- 既有 fact error classification；
- value/freshness validation。

Caller 仍拥有：

- 输入/config validation；
- graph-before-fact 顺序；
- internal duration 分类；
- domain decision；
- final context check。

### 26.3 为什么 error classification 没有被 graph service 重写

Membership provider 的错误语义在第 27 节已经冻结。Graph service 应复用 not found、unavailable、read failure、invalid 与 stale。

若为了“统一 evaluation error”把它们全部改成 graph failure，会丢失依赖故障域，也破坏旧 concrete router。

### 26.4 重构的回归风险

抽 helper 最容易改变：

- Clock/fact 调用顺序；
- context 检查位置；
- error + value 优先级；
- zero Clock 是否读 fact；
- stale 边界是大于还是大于等于；
- provider deadline 是否穿透。

所以旧 membership routing tests 必须与新 tests 一起回归，而不能只测新 service。

## 27. 并发模型：共享的是只读配置，不是请求状态

### 27.1 Service 持有什么

构造后的 service 只保存 ports、maxFactAge、immutable step budget value 与 maxDuration。

它不保存 current node、path、visited、caller context、child timer、graph/fact result、last decision 或 global registry。

### 27.2 每次调用的局部状态

每次 `Evaluate` 独立创建：

- evaluation child context；
- exact graph value；
- evaluated-at；
- fact snapshot；
- domain visited map；
- path slice；
- decision。

因此多个 caller 不应共享 path 或 step count。

### 27.3 64 并发 fixture 断言什么

测试源码让一个 service 被 64 个 goroutine 同时调用，并在普通功能层断言：

- 每个 graph/fact/Clock 各调用 64 次；
- 每个结果都 confirmed；
- target/path 一致；
- 结果之间 deep equal。

只有把这些 tests 实际放在 `go test -race` 下执行并通过，才可把动态 race detector 结果列为额外证据；fixture 源码本身和普通 `go test` 都不能单独证明“无数据竞争”。实际执行结果属于 QA。

### 27.4 它不能证明什么

- 真正 adapter thread-safe；
- 具体 adapter/连接池容量；
- 注入 dependency 的限流或并发能力；
- scheduler fairness；
- 生产吞吐；
- goroutine 泄漏不存在于所有失败路径。

Service 的并发安全仍依赖注入 ports 自身的并发契约。

### 27.5 为什么不加 singleflight/cache

当前每次调用读 exact graph，可能未来存在缓存价值。但现在没有 runtime workload、cache key/eviction/negative-cache/observability 证据。

引入 singleflight 还会让不同 caller context 的取消所有权变复杂：哪个 caller 控制共享 I/O？因此先保证每次调用独立。

## 28. 复杂度、容量与性能边界

### 28.1 一次 application 调用的依赖复杂度

```text
graph read: <= 1
Clock:      <= 1
fact read:  <= 1
```

合法成功调用三者都恰好为 1；前置失败会短路为 0。上限与实际 path depth 无关。

这里描述的是端口调用次数，不是每个端口内部的 I/O 次数、复杂度或 latency；interface 本身没有给这些实现细节作保证。

### 28.2 Domain 成本

一次 `graph.Validate()` 会复制并排序 nodes/edges，再构建索引 map、检查局部 shape 并做 DAG depth/reachability 遍历，因此时间复杂度为：

```text
O(V log V + E log E + V + E)
```

临时空间为 `O(V + E)`。Application 与 domain 各调用一次 Validate，所以渐进阶不变，但常数、排序和临时分配会发生两次。

Traversal：

```text
最多 16 loops
每个 decision 对 node/edge canonical slices 做二分定位
再扫描该 node 恰好两条 outgoing edges
path <= 16
visited <= 17 nodes
```

`Node` 是 `O(log V)`；`OutgoingEdges` 是 `O(log E + outdegree)`，schema v1 的 outdegree 恰好为 2；successor existence 又做一次 node 二分。因此路径遍历可写成 `O(D(log V + log E))`，其中 `D <= 16`，额外 path/visited 空间为 `O(D)`。只有“两条 edge 中找 branch”这一步是常量规模，不能把完整 node/edge lookup 都称为 `O(1)`。

### 28.3 重复 Validate 的真实代价

Service 与 domain 都 Validate，同一调用会对合法 graph 做两次完整检查。对 128/256 上限，这是有意接受的安全成本。

若未来实际 profiling 显示热点，可评估：

- validated/compiled immutable plan；
- revision-keyed bounded cache；
- constructor-only capability token。

但必须同时解决 invalidation、内存上限、schema compatibility 与可信构造来源。

### 28.4 为什么没有 benchmark 数字就不下性能结论

当前代码没有提供生产容量 benchmark。即使以后增加只使用内存 graph 与 test doubles 的本地 benchmark，它也只能代表某个 CPU 和固定 fixture。

它不能替真实 graph distribution、具体 reader 实现的 latency、并发负载或运行环境。当前只可准确说领域数据与路径有结构上限，不能宣称具体 P99。

### 28.5 maxDuration 的容量误区

把 maxDuration 设得很小不会自动增加吞吐，反而可能制造 timeout storm；设得很大也不会提升正确性。

未来值需要由上层 request budget、具体 graph/fact reader 的 latency 分布、retry policy、Activity SLO 与 load test 共同推导。

## 29. 威胁与隐私边界

### 29.1 保护资产

- exact graph revision 被正确执行；
- branch/default 语义不被 fallback 改写；
- Strategy target 不被 edge order 污染；
- 会员事实 provenance；
- path 不形成未授权画像泄露；
- caller/internal/provider failure ownership；
- 服务资源不被 graph/path 无界消耗；
- 未来发布和授权边界不被提前绕过。

### 29.2 可能的故障或攻击主体

- 错误的内部 caller；
- 返回错 identity 的 graph adapter；
- error + value 的 provider；
- 伪造/损坏的 aggregate；
- 未支持的新 tier/rule；
- 恶意或过大的 graph；
- 取消后仍返回数据的依赖；
- 将 path 直接写进日志的上层；
- 把 subject ref 当 Principal 的后续开发者；
- 试图把 evaluator 接入无授权 API 的 composition。

### 29.3 威胁控制矩阵

| 威胁 | 当前控制 | 残余风险 |
| --- | --- | --- |
| 隐式错 revision | exact identity + return equality | Activity 尚未选择 active revision |
| edge order 污染 | exact branch match | evaluator/accessor 实现 bug |
| unknown 走 default | typed fact + closed switch | 新 tier 发布需兼容流程 |
| dependency error 时仍使用 value | error priority | adapter 自身可能隐瞒 error |
| graph bomb | 128/256/16 + maxSteps | future schema 扩容需重评 |
| 无限循环 | graph Validate + visited + step stop | 同包 unsafe mutation |
| partial path 被误用 | zero decision | 受信日志仍可能记录内部 detail |
| timeout 责任混淆 | caller/internal/dependency priority | 不合作 reader 或阻塞 Clock 仍可拖延 goroutine |
| path 泄露 tier | 不公开、不持久化、不含 subject | 后续 API/UI 必须做 scope |
| evaluator 被越权调用 | 当前无 runtime surface | 第 30+ composition 必须加 auth |

### 29.4 Path 为什么是敏感派生信息

即使不保存 raw tier，branch：

```text
premium_override
```

仍可推断主体具有 premium tier。Fact source/revision 也可能形成外部系统关联信息。

未来若持久化或公开 path，需要至少评估：

- 谁可读；
- 对象/租户 scope；
- 是否只返回 target 而隐藏 branch；
- retention；
- 脱敏；
- 审计访问；
- 导出和删除政策。

### 29.5 当前“没有 API”是一项真实安全控制，但不是终态

未装配意味着：

- 浏览器不能提交 tier；
- 未认证用户不能枚举 graph；
- runtime identity 尚未因 evaluator 自动扩权；
- path 不通过 JSON 暴露。

但这不等于系统已经完成授权。它只说明第 29 节安全停止线有效。

## 30. 可观测性：先定义低基数结果，不急着记录 path

### 30.1 未来适合 metric 的维度

可考虑：

```text
operation = graph_evaluate
outcome =
  success
  invalid_argument
  not_configured
  graph_not_found
  graph_invalid
  fact_not_found
  fact_unavailable
  fact_invalid
  fact_stale
  step_budget
  internal_timeout
  caller_canceled
  failure
```

### 30.2 不应成为普通 label 的字段

- GraphID；
- Revision；
- NodeID；
- StrategyID；
- subject ref；
- fact revision；
- branch/path；
- provider endpoint；
- raw error。

它们会造成高基数或隐私泄露。

### 30.3 日志中的可信关联

受控内部日志若需要 incident investigation，可以记录：

- request/correlation ID；
- stable class；
- stage；
- elapsed bucket；
- path length bucket；
- schema version；
- internal timeout flag。

Graph identity 或 NodeID 只有在访问控制、保留期和脱敏策略明确后，才进入受限字段。

### 30.4 为什么当前实现没有打点

本节没有 runtime/composition，也没有统一 telemetry contract。直接在 domain/application 写全局 metrics：

- 会引入基础设施依赖；
- 容易使用高基数 label；
- 无法确定 service 名称与采样；
- 会把内部 evaluator 包装成已上线能力。

先冻结 outcome taxonomy，等真实 runtime 再装配观测。

## 31. 方案矩阵：不是“更通用”就更好

| 方案 | 优点 | 主要问题 | 决定 |
| --- | --- | --- | --- |
| 按 tier 直接找 target | 代码最短 | 绕过 graph/path，多步无意义 | 拒绝 |
| 执行裸 SQL rows | 可边读边走 | 绕过 Restore，耦合存储 | 拒绝 |
| 每 node 重读 graph/fact | 看似最新 | snapshot 混用，I/O 放大 | 拒绝 |
| operator registry | 表面扩展 | 当前只有一个 rule，无类型/版本证据 | 拒绝 |
| JSON/DSL/script | 运营灵活 | schema、安全、预算均不存在 | 拒绝 |
| 递归单路径 | 代码简短 | checkpoint/budget/path ownership 分散 | 不采用 |
| actual-path-only budget | 浅路径可用 | 同 revision 事实相关部分可用 | 不采用 |
| default fallback | 表面可用性 | unknown/error 静默改义 | 拒绝 |
| terminal 后直接抽奖 | 一次返回 Award | 路由与 Draw/库存/幂等混合 | 拒绝 |
| closed typed iterative evaluator | 与现有证据一致 | v1 表达力有限、重复 Validate | 采用 |

### 31.1 选择标准

每个方案按以下问题评估：

1. 是否由当前真实业务证据需要；
2. 是否保持 bounded-context ownership；
3. 是否能失败关闭；
4. 是否能被确定测试；
5. 是否让资源成本有界；
6. 是否提前引入未定义生命周期；
7. 是否可在后续新证据出现时演进。

### 31.2 为什么“先做通用，以后省重构”不成立

抽象成本不会消失，只会转为隐含假设。没有第二个 operator 时，我们不知道共同点是输入模型、cost、failure、side effect、branch shape、version 还是 dependency。

过早 registry 可能让未来真正第二个 rule 为错误接口买单。

## 32. 测试设计：每组测试都在证伪一种危险直觉

> 本节逐项描述当前 test/fuzz/architecture-guard **源码准备断言的性质**。下面的“成功”“失败”“证伪”是 test case 的 expected outcome，不是宣告本次冻结已经执行通过。普通、shuffle、race、fuzz campaign 与全仓回归的实际命令、次数和结果统一以 Lesson 29 QA 为准。

### 32.1 预算 value object

`StrategyRoutingGraphStepBudget` tests 覆盖：

- 1 成功；
- 16 成功；
- -1、0、17、极大整数失败；
- zero value Validate 失败；
- 同包伪造 `uint8` 最大值失败；
- 构造失败返回 zero budget。

它证伪：

> “只要类型是整数，调用方自己别传错就行。”

### 32.2 Concrete router parity

同一 standard/premium fact 分别交给：

- 第 27 节 `RouteMembershipStrategy`；
- 第 29 节 `EvaluateStrategyRoutingGraph`。

比较：

- target；
- rule；
- branch；
- reason；
- fact source/revision；
- evaluated-at。

它证伪：

> “Graph evaluator 是新实现，行为稍微不同也没关系。”

### 32.3 Canonical edge order trap

Fixture 明确确认 `baseline_default` 在 outgoing slice 中排在前面，再用 premium fact 求值，必须得到 premium target。

它证伪：

> “Graph 已 canonical，所以第一条 edge 就是优先路径。”

### 32.4 Shared terminal

两条 branch 指向同一个 terminal：

- target 相同；
- terminal 相同；
- premium/standard path branch 不同；
- reason 不同。

它证伪：

> “只要 target 相同，branch evidence 可以丢。”

### 32.5 Shared decision

两条 root branch 先汇聚到同一 decision，再继续到 terminal。测试要求：

- path 有两步；
- `step[0].To == step[1].From`；
- 每步按同一事实选择相同 typed branch；
- shared successor 不被当 cycle。

它证伪：

> “DAG 合流只在 terminal 情况有效。”

### 32.6 Depth 16

premium 走完整 16-step path：

- budget 16 成功；
- path 长度 16；
- from/to 连续；
- 最后一条到 premium terminal；
- target 正确。

同一 graph 在 budget 15 时，即使传 zero fact，也必须先报 depth budget，证明 fact 尚未被评价。

它证伪：

> “最坏深度准入只是文档，没有执行顺序证据。”

### 32.7 MaxUint64 identity

GraphID、NodeID、StrategyID 使用 `math.MaxUint64` 附近值求值，结果不得截断。

它证伪：

> “持久化层测过 unsigned，domain path 就不需要再测。”

### 32.8 Invalid input table

覆盖：

- nil context；
- zero budget；
- zero graph；
- forged derived depth；
- zero fact；
- zero evaluated-at；
- future fact。

每个 case 都要求：

- expected error class；
- general evaluation-invalid class；
- zero decision。

它证伪：

> “只要返回 error，残留 decision 无所谓。”

### 32.9 Decision forgery

同包测试逐项篡改：

- identity；
- schema；
- root/terminal；
- target/internal terminal target；
- fact source/revision；
- evaluated-at canonicality；
- step budget；
- path length；
- first source；
- continuity；
- rule；
- reason；
- repeated node；
- terminal。

每项都必须让 `Confirmed()` 变 false。

它证伪：

> “Decision 是 evaluator 构造的，所以不需要自检。”

### 32.10 Defensive path copy

Caller 修改 `decision.Path()[0]` 后：

- original decision 仍 confirmed；
- 再次读取的 first step 未变化。

它证伪：

> “private field 已经保证 slice 不可变。”

### 32.11 Domain cancellation checkpoints

一组 context 在第 N 次 `Err()` 调用后返回 canceled，测试要求：

- mid-traversal 停止；
- zero decision；
- final-success checkpoint 被精确锁定。

它证伪：

> “函数末尾大概检查一次 context 就够了。”

### 32.12 Application dependency order

Stubs 记录调用序列，成功必须严格是：

```text
graph -> clock -> fact
```

并且：

- graph identity exact；
- graph/fact 使用同一 child context；
- caller context value 被保留；
- 每个依赖恰好一次。

它证伪：

> “依赖都调用一次，先后无所谓。”

### 32.13 Wrong identity/not-found

- Reader 返回另一 exact identity：graph-invalid；
- Repository not-found：保留 not-found class；
- 两者都不读取 Clock/fact；
- 不尝试另一 revision。

它证伪：

> “方法名 FindByIdentity 已经足够可信。”

### 32.14 Error + value

Graph reader 同时返回合法 graph 与 private error：

- error 胜；
- low-disclosure general failure；
- cause 只经 `Cause()` 可见；
- Clock/fact zero。

Fact reader 同时返回合法 fact 与 provider deadline：

- error 胜；
- membership unavailable；
- decision zero。

它证伪：

> “既然 value 看起来合法，可以尽量继续。”

### 32.15 Stored-invalid 低披露

Repository wrapper 内含：

- stored-invalid class；
- private SQL/identity/corruption cause。

Application 只公开 graph-invalid，不让 `errors.Is` 穿透 storage class/cause；可信 `Cause()` 仍保留完整 repository wrapper。

它证伪：

> “Error 文本不打印 cause，就已经低披露。”

### 32.16 Zero Clock

Graph 合法后 Clock 返回 zero：

- Clock 调一次；
- fact 零调用；
- invalid clock；
- zero decision。

它证伪：

> “zero time 可以当当前时间或让 fact 自己判断。”

### 32.17 Dependency-boundary cancellation

分别在：

- graph return；
- Clock return；
- fact return；

触发 caller cancel，并让 provider 同时返回其他 error。Caller cancel 必须胜出。

它证伪：

> “只在请求开始检查 pre-cancel 就足够。”

### 32.18 Internal timeout 无 sleep 设计

Blocking reader：

1. 关闭 started channel；
2. 等待 received context Done；
3. 返回 provider error。

测试用 service internal deadline 驱动，不靠 `time.Sleep` 猜调度。要求：

- stable timed-out；
- 不匹配 context deadline/canceled；
- private cause exact；
- provider error 不胜出；
- 后续依赖零调用。

它证伪：

> “timeout test 只要 sleep 比 deadline 多一点就可靠。”

### 32.19 Caller earlier deadline end-to-end

Caller deadline 比 maxDuration 更早，blocking graph reader 观察到的 deadline 必须等于 caller deadline；最终返回 exact caller `DeadlineExceeded`。

它证伪：

> “有 child timeout 后，caller deadline 自然总会分类正确。”

### 32.20 Equal deadline helper

使用已经到期的 absolute deadline 构造：

- caller deadline == internal；
- caller deadline < internal；
- internal < caller；
- explicit cleanup。

检查 cause ownership，避免等待真实相等 timer 的非确定调度。

它证伪：

> “相同 duration 就等于相同 deadline，而且测试跑几次没问题即可。”

### 32.21 64 并发与 race

Domain 纯 evaluator 和 application service 都有 64-worker fixture。

普通 test 断言并发结果与调用次数；只有同一 test suite 在 `-race` 下实际通过，才能增加动态 race-detector 证据。它们若按相应方式执行并通过，才共同证伪：

> “代码里没有显式 global，就自动无共享状态。”

### 32.22 Fuzz

Fuzzer 将 bytes 映射为：

- depth 1..16；
- tier standard/premium/unknown；
- budget 0..17；
- future flag；
- graph mutation：depth/root/edge/kind/schema/id/revision。

核心性质：

- 不 panic；
- 不死循环；
- 失败必 zero decision；
- success 必 confirmed；
- path 不超过 budget/16；
- premium/standard target/path 与 fixture 语义一致。

常规 `go test` 只会运行 seed corpus；持续变异探索需要显式 `go test -fuzz` 与记录的时间预算。对应 campaign 执行并通过后，才进一步证伪：

> “手写 happy/negative cases 已经覆盖组合空间。”

### 32.23 测试仍不能证明什么

- production traffic；
- real membership adapter；
- graph runtime grant；
- Activity binding；
- HTTP auth；
- browser E2E；
- path retention；
- operational alert；
- P99/QPS；
- 跨端口/数据源一致性。

测试证据必须与能力边界一起陈述。

## 33. Architecture guard：负向能力也要有可执行证据

### 33.1 禁止的通用抽象

AST guard 源码被写成在 test 执行时拒绝 Lottery/Participation 核心包过早出现：

- `Rule` / `RuleChain` / `RuleTree`；
- `RuleEngine` / `DecisionEngine`；
- registry；
- FactBag；
- expression/script/DSL；
- `map[string]any`；
- 无真实需求的 generic type/function。

### 33.2 为什么 `StrategyRoutingGraphDecision` 不等于 `DecisionEngine`

前者是一个具体领域结果；后者暗示可装配、跨 operator 的平台能力。Guard 拒绝的是扩大抽象，不是禁止业务名词“decision”。

### 33.3 禁止 runtime/HTTP 装配

Lesson 29 guard test 源码会扫描：

- `cmd`；
- Lottery/infrastructure HTTP；
- httpserver；
- appconfig；
- production Compose；
- Docker/Nginx；
- Web production sources。

若在这些表面发现 evaluator/service/decision/budget identifier，test 应失败；它不等于已经记录了本次命令通过。

### 33.4 为什么架构负证不只靠 git diff

手工 diff 容易漏：

- 间接 import；
- 新增 nested file；
- build tag；
- 配置字符串；
- Web route；
- future refactor 后的悄然装配。

AST/source guard 让停止线具备持续回归能力；当前冻结是否实际执行通过仍由 QA 记录。

### 33.5 Guard 的局限

- identifier 重命名可能绕过字符串检查；
- reflection/config 间接 assembly 难检测；
- 它不证明数据库 grant；
- 它不做 security review；
- 它不能替人工 bounded-context 判断。

因此 guard 是辅助证据，不是“架构自动正确”。

## 34. 反事实推演：如果当时选择更“省事”的路径

### 34.1 直接按 tier 找 target

事故路径：

1. graph 有多步 topology；
2. evaluator 忽略 nodes/edges；
3. 直接搜索 premium terminal；
4. graph path 与执行结果无关；
5. 审计以为 topology 被执行，实际是隐藏 if/else。

### 34.2 取 outgoing 第一条

事故路径：

1. canonical sort 把 baseline 放前；
2. premium fact 形成 premium branch；
3. evaluator 仍取 `edges[0]`；
4. premium 被路由到 baseline；
5. graph Validate 和数据库 constraints 全部通过，却产生业务错配。

### 34.3 Missing branch 走 default

事故路径：

1. corrupted graph 缺 premium edge；
2. evaluator 找不到 selected branch；
3. 扫到 `is_default=true`；
4. premium 静默落入 baseline；
5. corruption 被“可用性策略”掩盖。

### 34.4 每 node 重新读取 fact

事故路径：

1. node 1 看到 premium；
2. provider revision 更新；
3. node 2 看到 standard；
4. path 同时包含 premium/standard 语义；
5. 结果无法由任何单一 fact revision 重放。

### 34.5 只用 actual path budget

事故路径：

1. 同一 graph 对 standard 成功；
2. premium 走到半途超限；
3. 某类用户持续遭受技术失败；
4. 发布系统却无法提前判定 graph/service 是否兼容；
5. 运营把它误认为会员业务拒绝。

### 34.6 直接 `context.WithTimeout`

事故路径：

1. caller 与 internal deadline 相同；
2. 两个 timer 竞争；
3. 一次返回 caller deadline；
4. 另一次返回 internal timeout；
5. 相同输入在监控中归属不同。

### 34.7 Error wrapper 实现 `Unwrap`

事故路径：

1. internal timeout cause 被保存；
2. public wrapper Unwrap；
3. 上层 `errors.Is(context.DeadlineExceeded)` 成立；
4. retry/mapping 误以为 caller/provider deadline；
5. private cause 变成公共协议。

### 34.8 返回 partial path

事故路径：

1. 第 4 步 context canceled；
2. 返回前三步 + error；
3. handler 打印/缓存 prefix；
4. 另一个组件误取最后 node；
5. 未完成 evaluation 变成业务证据。

### 34.9 Terminal 后顺带 selector

事故路径：

1. graph route 成功；
2. Strategy load 或 random 失败；
3. 上层看到“graph evaluation failed”；
4. 无法判断路由、存储还是随机源故障；
5. retry 可能重复正式抽奖。

### 34.10 本节直接装进 API

事故路径：

1. caller 提交 graph identity 与 subject；
2. 无 session/authorization；
3. 可枚举 graph not-found/invalid；
4. 可借 path 推断会员 tier；
5. runtime DB 权限被提前扩大；
6. 第 31～35 节只能事后补洞。

## 35. 风险账本

| 风险 | 当前决定 | 为什么现在可接受 | 重新评估触发器 |
| --- | --- | --- | --- |
| 只有一个 operator | closed typed switch/helper | 唯一有完整证据的 rule | 第二个 Lottery-owned rule |
| 多步重复同一 tier rule | 只证明 traversal | schema v1 无 node 参数 | 异构 typed fact 出现 |
| graph 每次重复 Validate | 接受有界成本 | 128/256/16 且无热点证据 | profiling 显示瓶颈 |
| graph 每次读 Repository | 不缓存 | 尚无 runtime workload | measured read QPS/latency |
| time budget cooperative | context checkpoints | typed operator 为本地有界校验/分支、path <=16 | 出现 I/O 或长计算 operator |
| reader 可忽略 context、Clock 无 context 参数 | 无法强制抢占 | 当前只有 port contract 与返回后检查 | 实际 adapter 阻塞/泄漏 |
| decision 非密码学证据 | private fields + exact revision | 当前只在内存内部使用 | 跨环境审计/签名 |
| path 不持久化 | zero side effect | 无审计 consumer/scope | 回放、合规、运营查询 |
| path 可推断 tier | 不公开、不含 subject | 当前无 transport | API/UI/持久化出现 |
| graph identity 由 caller 给出 | exact input | Activity 尚未实现 | 第 30 节 active binding |
| target 只到 StrategyID | 不加载 | 路由与选择分层 | immutable Strategy revision |
| error class 尚无 HTTP mapping | 无 transport | 避免预制 status | 受权 API 出现 |
| maxDuration 无 runtime 默认值 | 测试显式注入 | 无 SLO/流量证据 | composition 与上层预算 |
| service 依赖 port thread-safety | 明确外部契约 | service 无共享请求态 | 真实 adapter 并发验收 |

### 35.1 风险账本不是待办堆积

每项都要有：

- 当前为何接受；
- 哪种证据触发重决策；
- 谁拥有新决定。

否则“以后再说”只是在隐藏技术债。

## 36. 本节停止线：代码没有出现什么，同样是交付事实

### 36.1 没有发布生命周期

本节没有 draft、approve、publish、active/latest、retire、rollback 或 Activity -> graph binding。

Exact identity 由受信测试/caller 显式给出，不表示任何 revision 已获业务批准。

### 36.2 没有线上业务链

本节没有真实 membership adapter、Principal -> subject mapping、Participation prerequisite、Strategy load/cache、WeightedSelector、Crypto random、Award availability、inventory、Draw/Result、idempotency 或 Benefit。

### 36.3 没有通用规则平台

本节没有 cross-context `RuleEngine`、registry/plugin、JSON expression、DMN/OPA/XACML、script/remote-call node、dynamic priority、retry/loop/parallel branch 或 arbitrary fact bag。

### 36.4 没有 runtime surface

本节没有 `cmd/growth-api` assembly、HTTP/MCP/Agent endpoint、DTO、Nginx/Compose topology、runtime config/env、graph cache 或 React route/page/button。

### 36.5 没有访问控制

本节没有 session、Principal、Role/Permission、Resource/Action、tenant/data scope、server RBAC、frontend capability projection 或 overreach/E2E。

### 36.6 为什么停止线不是“功能没做完”

每一项都需要独立模型和验收：

- 发布决定哪份 revision 生效；
- authentication 决定 caller 是谁；
- authorization 决定能做什么；
- selection 决定 Strategy 中哪个 Award；
- Draw 决定幂等和正式结果；
- UI 决定交互与披露。

把这些一次塞进 evaluator，会让任何失败都只能叫 “decision failed”，反而无法形成真实项目演进。

## 37. 第 30 节演进：从“能执行”到“哪份配置应该执行”

### 37.1 第 29 节留下的输入缺口

当前 service 要求 caller 传：

```text
exact GraphID + Revision
```

这对验证 evaluator 正确性足够，对线上 Activity 不够。真正请求通常只知道 Activity，而不知道 graph revision。

### 37.2 第 30 节必须拥有的问题

第 30 节应明确：

- Activity aggregate identity；
- lifecycle；
- draft/approved/published/retired；
- Activity version；
- exact graph revision binding；
- Strategy 配置是否也需 immutable version；
- start/end/timezone；
- concurrent publish；
- rollback 是切引用还是改内容；
- 历史请求怎样解释；
- 发布时 graph/target/budget 如何复核。

### 37.3 为什么 Activity 不能写进 graph header

同一 graph revision 可能被多个 Activity 复用、暂未被任何 Activity 使用，或在不同环境被不同发布记录引用。

Graph 拥有 topology，Activity 拥有生效语境。把 ActivityID/status 塞入 graph 会让两个 aggregate 生命周期互锁。

### 37.4 发布时的可能验收链

后续可以评估：

```text
Activity draft
  -> exact graph revision exists
  -> graph Validate
  -> graph depth compatible with configured evaluation budget
  -> terminal Strategy references satisfy publication policy
  -> optimistic concurrency
  -> immutable published Activity version
```

这只是演进输入，不是第 29 节已实现事实。

### 37.5 rollback 的第一性问题

Create-only graph revision 不应被回写。Rollback 更可能是：

```text
发布一个新的 Activity binding/version
  指回旧 graph revision
```

而不是修改历史 graph 内容。第 30 节需要明确历史生效时段与并发语义。

### 37.6 Evaluator 在第 30 节仍应保持纯净

理想组合：

```text
Activity resolver
  -> exact approved graph identity
  -> StrategyRoutingGraphEvaluationService
```

Evaluator 不应自己查询 latest Activity，否则发布选择和遍历正确性再次耦合。

## 38. 第 31～35 节演进：访问控制是正交控制面

### 38.1 第 31 节：公共访问控制模型与威胁边界

需要先统一：

- Principal；
- Subject 与 Actor 区分；
- Resource；
- Action；
- Role；
- Permission；
- tenant/object/data scope；
- default deny；
- disclosure policy；
- audit subject；
- service-to-service identity。

不能用 membership tier、graph branch、`isAdmin` boolean 或前端菜单代替统一模型。

### 38.2 第 32 节：真实会话认证

需要回答：

- credential 如何验证；
- session 如何创建/续期/撤销；
- Cookie/token 属性；
- CSRF；
- fixation；
- rotation；
- Principal 如何进入 server context；
- anonymous/authenticated 边界。

只有这一步以后，server 才知道“谁在调用”。

### 38.3 第 33 节：服务端 RBAC 强制

需要把：

```text
Principal + resource + action + scope
  -> allow/deny
```

放在服务端入口与对象读取之前。

Graph evaluation 可能涉及的动作至少要区分：

- read graph metadata；
- evaluate graph；
- inspect path；
- create revision；
- approve/publish Activity。

“能 evaluate”不必然“能 inspect path”，因为 path 带会员派生信息。

### 38.4 第 34 节：前端权限投影

前端只能消费 server capability 来改善体验：

- 导航裁剪；
- route guard；
- disabled/hidden action；
- 空状态；
- 权限变化刷新。

它不能成为安全边界。隐藏按钮不阻止 direct API call。

### 38.5 第 35 节：越权与浏览器 E2E

需要真实验收：

- anonymous direct API；
- low-role direct API；
- 横向对象越权；
- tenant/data scope；
- session expiry/revocation；
- CSRF；
- 前端导航与 server capability 一致；
- 手工构造 URL/request；
- path/branch 披露。

### 38.6 为什么第 29 节不能提前放 `role` 字段

Graph topology 与 routing decision 都不拥有 caller authority。提前放 role 会：

- 把会员 tier 与访问 role 混淆；
- 让 graph 配置决定权限；
- 使服务端授权无法统一；
- 让 Activity/operator UI 各自再造权限表。

正确演进是公共模型先收敛，再由各资源声明 action/scope。

## 39. 重新决策触发器

以下真实证据出现时，应新增 ADR，而不是静默扩大 v1。

### 39.1 第二个 typed operator

必须先回答：

- 它仍属于 Lottery 吗；
- fact owner 是谁；
- 输入类型是什么；
- 是否有 side effect；
- cost 怎样定义；
- branch set 是否固定；
- provider error 怎样分类；
- schema 是否升级。

然后再比较 closed tagged union、visitor、compile-time dispatch table 或 registry。

### 39.2 Operator 需要外部 I/O

需要重新设计：

- per-operator deadline；
- overall budget；
- retry；
- idempotency；
- circuit breaker；
- snapshot consistency；
- cancellation；
- partial dependency failure。

当前纯 local helper 的结论不能直接复用。

### 39.3 需要 compiled plan/cache

触发条件应是：

- graph Validate/traversal 的 measured hotspot；
- known revision read QPS；
- acceptable staleness；
- bounded memory；
- invalidation/publish contract。

不能因“规则引擎通常要缓存”而提前添加。

### 39.4 需要 path 持久化

必须新增：

- record identity；
- request/Activity/Principal relationship；
- PII classification；
- encryption；
- retention/deletion；
- authorized query；
- tamper evidence；
- replay compatibility；
- write idempotency。

当前 in-memory decision 不能直接序列化落表就叫审计。

### 39.5 Strategy target 需要 immutable revision

如果 Activity 历史必须证明具体 award/weight 内容，单一 StrategyID 不够。需要 Strategy business revision，并决定 graph terminal 是否升级引用。

### 39.6 需要 partial availability

如果产品明确允许 shallow tier 在低 budget service 成功、deep tier 返回特殊业务状态，必须重新定义公平性、错误语义、发布验证和用户体验。

不能只删除 `graph.Depth()` admission。

### 39.7 需要跨环境 promotion/signature

若 graph 作为发布包跨环境移动，需要评估：

- canonical serialization；
- content hash；
- signature；
- dependency manifest；
- environment-local target mapping；
- approval provenance。

Revision token 当前不是 hash。

## 40. 架构师实施检查清单

### 40.1 边界与命名

- [ ] 能力是否仍命名为 Lottery Strategy routing evaluation？
- [ ] 是否没有跨上下文 `RuleEngine`？
- [ ] 是否明确 route ≠ eligibility ≠ authorization ≠ Draw？
- [ ] terminal 是否只返回 StrategyID？
- [ ] 是否没有把 MembershipSubjectRef 当 Principal？

### 40.2 Exact input

- [ ] GraphID/Revision 是否都验证？
- [ ] 是否只调用一次 `FindByIdentity`？
- [ ] 是否禁止 latest/active/fallback？
- [ ] returned identity 是否精确相等？
- [ ] graph error + value 是否丢弃 value？
- [ ] graph invalid 时 Clock/fact 是否零调用？

### 40.3 Snapshot 与事实

- [ ] dependency order 是否 graph -> Clock -> fact？
- [ ] Clock 是否恰好一次？
- [ ] fact 是否恰好一次？
- [ ] 所有 node 是否共享同一 fact/evaluated-at？
- [ ] subject equality 是否检查？
- [ ] future/stale 是否失败关闭？
- [ ] default 是否只承接 confirmed standard？

### 40.4 遍历

- [ ] 是否从显式 root 开始？
- [ ] 是否使用迭代 current-node loop？
- [ ] 每个 decision 前是否检查 context/budget？
- [ ] 是否 closed typed dispatch？
- [ ] 是否 exact branch lookup？
- [ ] 是否要求 matches == 1？
- [ ] 是否不依赖 edge/map order？
- [ ] 是否维护 path-local visited？
- [ ] 到 terminal 是否立即停止？

### 40.5 预算与时间

- [ ] maxSteps 是否 1..16？
- [ ] graph depth 是否在 fact read 前做 worst-case admission？
- [ ] actual hard stop 是否保留？
- [ ] maxDuration 是否正数？
- [ ] business Clock 与 technical deadline 是否分离？
- [ ] caller earlier/equal deadline 是否拥有 cause？
- [ ] internal deadline 是否只在严格更早时安装？
- [ ] cleanup cancel 是否不会被识别成 timeout？

### 40.6 错误

- [ ] caller error 是否第一优先？
- [ ] internal timeout 是否第二优先？
- [ ] provider error 是否只在 contexts live 时分类？
- [ ] provider-owned deadline 是否不冒充 caller/internal？
- [ ] error + value 是否 error 胜？
- [ ] error wrapper 是否无 `Unwrap`？
- [ ] `Error()` 是否只有稳定 class？
- [ ] graph/subject/node/SQL 是否不进入普通文本？

### 40.7 Decision

- [ ] 失败是否始终 zero decision？
- [ ] 是否从不返回 partial path/target？
- [ ] path 是否 from/rule/branch/reason/to 完整？
- [ ] path 是否连续且无重复？
- [ ] target 是否与 terminal 一致？
- [ ] fact provenance 是否完整？
- [ ] evaluated-at 是否 canonical UTC？
- [ ] `Path()` 是否 defensive copy？
- [ ] success return 前是否再次检查 context？

### 40.8 并发与资源

- [ ] service 是否无请求级字段？
- [ ] child context 是否每次创建/清理？
- [ ] path/visited 是否调用局部？
- [ ] port 并发契约是否明确？
- [ ] race 是否覆盖 domain/application？
- [ ] fuzz 是否有真实 topology/budget mutation？
- [ ] 是否没有无证据 cache/singleflight/goroutine fan-out？

### 40.9 Stop line

- [ ] 没有 Activity publish/active/latest？
- [ ] 没有 HTTP/runtime/UI？
- [ ] 没有 Strategy load/selector/random？
- [ ] 没有 Draw/Result/Benefit？
- [ ] 没有 session/RBAC？
- [ ] 没有新 migration/grant/cache/event？
- [ ] 第 27、28 节回归是否保持？

## 41. 当前代码证据索引

### 41.1 Domain

- [会员 branch oracle helper](../../../internal/lottery/domain/membership_routing.go)
- [helper 语义测试](../../../internal/lottery/domain/membership_routing_branch_test.go)
- [Graph evaluation、path 与 decision](../../../internal/lottery/domain/strategy_routing_evaluation.go)
- [Domain error taxonomy](../../../internal/lottery/domain/strategy_routing_evaluation_errors.go)
- [Graph evaluation tests](../../../internal/lottery/domain/strategy_routing_evaluation_test.go)
- [Graph evaluation fuzz](../../../internal/lottery/domain/strategy_routing_evaluation_fuzz_test.go)

### 41.2 Application

- [共享 fact session](../../../internal/lottery/application/membership_routing.go)
- [Exact graph evaluation service](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)
- [低披露 application errors](../../../internal/lottery/application/strategy_routing_graph_evaluation_error.go)
- [Application orchestration tests](../../../internal/lottery/application/strategy_routing_graph_evaluation_test.go)
- [架构停止线](../../../internal/lottery/application/membership_routing_architecture_test.go)

### 41.3 规范与历史

- [Lesson 29 产品基线](../../product/lottery-strategy-routing-evaluation-v1.md)
- [ADR-0025](../../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)
- [第 27 节会员路由基线](../../product/membership-strategy-routing-v1.md)
- [第 28 节 graph 基线](../../product/lottery-strategy-routing-graph-v1.md)
- [ADR-0023](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)
- [ADR-0024](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)

## 42. 可准确声称与禁止声称

### 42.1 可准确声称

> Lesson 29 在 Lottery bounded context 内为 immutable Strategy routing graph 建立了封闭类型 evaluator：Application 按 exact GraphID/Revision 读取并复核一次 graph，在最坏深度准入后捕获一次受控 evaluated-at、读取一次 fresh membership fact；Domain 从显式 root 进行确定迭代单路径遍历，按 concrete membership branch 精确匹配 edge，形成 defensive-copy 多步 path 与完整 Strategy target decision，并以 1～16 step、positive cooperative duration、caller/internal/dependency 优先级与 path-local cycle guard 限制资源；失败统一返回 zero decision，Application wrapper 另行收敛公开错误披露。

### 42.2 禁止声称

- 通用规则引擎、DSL、DMN、OPA 或插件平台已完成；
- 多条件/多事实 operator 已完成；
- graph 已发布或 active；
- Activity 已绑定 graph；
- evaluator 已接入 production runtime/API；
- 真实会员 provider 已接入；
- MembershipSubjectRef 是可信登录身份；
- route success 表示 eligible 或 authorized；
- Strategy 已加载或 Award 已选择；
- Draw/Result 已形成；
- path 已持久化或构成合规审计；
- session/RBAC/data scope/UI 权限已完成；
- 测试证明生产 P99、QPS、容量或可用性。

### 42.3 面试表达中的准确尺度

推荐表达：

> 我没有一开始引入通用规则引擎，而是以已存在的会员 tier 路由为语义 oracle，先把持久化 DAG 的执行边界做成 closed typed evaluator。重点解决 exact revision、单次事实快照、exact branch lookup、最坏深度与实际步数双重预算、caller/internal/provider timeout ownership，以及失败不返回 partial path。等第二个真实 operator 出现再评估 registry。

不推荐表达：

> 我从零设计了一套支持任意规则、线上高并发和运营配置的通用决策平台。

后者超出了当前代码、runtime 与验收证据。

## 43. 最终第一性结论

本节最重要的结论有十五条：

1. **可信保存不等于可信执行。** Graph persistence 与 evaluation 必须分别定义失败模型。
2. **执行器只使用自己拥有的业务语言。** Membership fact 属于外部 authority，tier-to-branch 属于 Lottery。
3. **Exact identity 先于“最新可用”。** 版本选择是发布问题，不是 evaluator fallback。
4. **一次决定只应使用一份 graph、一份 fact、一个业务时刻。** 不主动制造混合版本。
5. **复用语义的最小粒度是 branch helper。** 共享一个 switch 不需要通用 Rule 接口。
6. **验证全图，执行单路径。** DAG 合流不等于并行 branch。
7. **Canonical order 绝不是业务优先级。** Branch 必须精确匹配，default 不能吞错误。
8. **Path 是最小实际证据。** 它保留 branch/reason，却不复制全图、subject 或 payload。
9. **成功对象必须内部一致，失败对象必须不可用。** Confirmed + private fields + defensive copy 与 zero decision 共同形成边界。
10. **静态深度、运行步数和 wall-clock deadline 是三种预算。** 不能互相替代。
11. **最坏路径准入是一致可用性承诺。** Actual hard stop 则是最后一道运行防线。
12. **业务 Clock 与技术 deadline 必须分开。** 一个用于事实解释，一个用于资源生命周期。
13. **相同 deadline 也需要明确所有者。** Caller earlier-or-equal，internal only when strictly earlier。
14. **低披露不等于丢诊断。** Stable class 与 explicit trusted Cause 可以并存，而不暴露 Unwrap 链。
15. **停止线是架构能力的一部分。** 第 30 节负责发布，第 31～35 节负责统一权限；本节不靠越界集成伪装完整系统。

一个成熟的决策系统，不是“规则越多、抽象越通用”就越先进。真正困难的是把每个事实的所有者、每个版本的身份、每个失败的责任、每一份结果的可证明范围以及每一次未来扩展的触发条件都写清楚。

第 29 节的价值正在于此：它没有假装已经拥有整个规则平台，而是把第一条真实 Lottery graph path 做成了一个确定、有界、失败关闭、可被后续发布和权限层安全组合的内核。
