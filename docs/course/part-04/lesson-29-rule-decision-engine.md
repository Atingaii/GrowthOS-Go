# 第 29 节：实现规则决策引擎——先让一张明确的 Lottery 路由图可信地跑完

> 第 28 节已经证明一份 `StrategyRoutingGraph` 可以被严格构造、持久化和恢复，但“结构可信”仍不等于“执行可信”。本节不借课程标题发明万能规则平台，而是在第 27 节 concrete router 和第 28 节 immutable graph 之间补上最小闭环：读取 exact graph revision 与一份权威会员事实，在一个受控逻辑时刻内，以封闭类型化分派沿唯一分支走到 Strategy terminal，并且让取消、超时、步数和所有失败都只能得到零决定。

- **起点：** 第 28 节已验收 tip `90844c1`
- **学习分支：** `codex/lesson-29-rule-decision-engine`
- **产品规格提交：** `041dc30`
- **架构决策提交：** `27bd514`
- **typed branch 共享提交：** `0ca25d2`
- **fact session 共享提交：** `51dbd61`
- **domain evaluator 提交：** `a173d8b`
- **application orchestration 提交：** `3863b06`
- **架构停止线提交：** `ab056ba`
- **异构并发隔离加固提交：** `515d776`
- **产品规格：** [Lottery Strategy Routing Evaluation 基线 v1](../../product/lottery-strategy-routing-evaluation-v1.md)
- **架构决策：** [ADR-0025](../../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)
- **API 记录：** [第 29 节 API](../../api/lessons/lesson-29.md)
- **QA：** [第 29 节 QA](../../qa/lessons/lesson-29.md)分别记录当前已执行门禁与 final-freeze pending，本文不预写最终远端冻结结果
- **设计手记：** [第 29 节第一性原理手记](../../design-thinking/lessons/lesson-29.md)
- **面试问答：** [第 29 节面试问答](../../interview/lessons/lesson-29.md)

## 1. 本节真正解决的问题

前两节留下了两份互补事实。

第 27 节已经有一条可以直接运行的具体业务语义：

```text
confirmed premium
  -> premium_override
  -> premium Strategy target

confirmed standard
  -> baseline_default
  -> baseline Strategy target
```

第 28 节则把同一组 rule、branch 与 target 词汇保存为：

```text
exact (GraphID, Revision)
  + schema_version = 1
  + explicit root
  + validated immutable rooted DAG
  + nodes <= 128
  + edges <= 256
  + longest depth <= 16
```

但是中间仍有一个空白：

> 谁读取这张图和会员事实，怎样从 root 走到唯一 terminal，怎样证明路径没有依赖 edge 排序，失败时又怎样保证半条路径不会逃逸？

本节答案是：

```text
exact graph identity
  -> one validated graph snapshot
  -> one server-owned evaluated-at
  -> one authoritative membership fact snapshot
  -> closed typed branch decision
  -> iterative single-path traversal
  -> one complete immutable decision OR zero decision + error
```

这里的“引擎”只是课程演进标题。生产代码使用 `StrategyRoutingGraphEvaluationService` 与 `EvaluateStrategyRoutingGraph`，把能力限定在 Lottery Strategy routing graph，不声明跨上下文 `RuleEngine`。

## 2. 为什么不能从“实现规则引擎”直接开始

如果先从通用接口出发，最容易写出：

```go
type Rule interface {
    Evaluate(context.Context, map[string]any) (any, error)
}
```

这段代码看似可扩展，实际上当前无法回答：

- `map` 中每个事实由谁拥有；
- 字段缺失、类型错误与 provider failure 怎样区分；
- operator 能否访问网络、数据库或随机源；
- default 是标准会员业务分支，还是任意错误 fallback；
- operator 版本怎样与 graph schema 兼容；
- 权限规则是否可以混入 Lottery graph；
- 取消、资源上限和 side effect 怎样统一。

本节只有一个已经被产品、领域和值对象共同证明的 operator：

```text
lottery.membership_tier.route_strategy
```

所以当前最诚实的抽象是封闭类型化 dispatch。第二种真实 Lottery rule 出现后，才有两个实际样本去比较 closed union、visitor 或 registry；在此之前，registry 只是把未知复杂度包装成扩展性。

## 3. 本节交付的完整切片

### 3.1 共享 concrete branch oracle

[membership_routing.go](../../../internal/lottery/domain/membership_routing.go)抽出 package-private：

```go
func evaluateMembershipRoutingBranch(
    fact MembershipTierFactSnapshot,
    evaluatedAt time.Time,
) (MembershipRoutingBranch, MembershipRoutingReasonCode, error)
```

它只回答：

```text
validated fact + evaluated-at -> exact branch + stable reason
```

它不读取 graph，不选择 Strategy target，不访问 provider，也不导出通用规则接口。第 27 节 `RouteMembershipStrategy` 和本节 graph evaluator 共同消费它，从源头避免两个 tier switch 漂移。

### 3.2 共享 application fact session

[membership_routing.go](../../../internal/lottery/application/membership_routing.go)抽出：

```go
func readFreshMembershipTierFact(
    ctx context.Context,
    subjectRef domain.MembershipSubjectRef,
    membershipFacts MembershipTierFactReader,
    clock MembershipRoutingClock,
    maxFactAge time.Duration,
) (domain.MembershipTierFactSnapshot, time.Time, error)
```

它保持既有第 27 节顺序：

```text
Clock once
  -> context check
  -> fact reader once
  -> error wins returned value
  -> structural validation
  -> exact subject
  -> not future
  -> within maxFactAge
  -> final context check
```

这是“同一份事实会话”的复用，不是把会员事实提升为万能 evaluation context。

### 3.3 domain step budget、path 与 decision

[strategy_routing_evaluation.go](../../../internal/lottery/domain/strategy_routing_evaluation.go)新增：

- `StrategyRoutingGraphStepBudget`；
- `StrategyRoutingGraphPathStep`；
- `StrategyRoutingGraphDecision`；
- `EvaluateStrategyRoutingGraph`；
- 运行时 visited guard、exact edge match 与 zero-decision 协议。

[strategy_routing_evaluation_errors.go](../../../internal/lottery/domain/strategy_routing_evaluation_errors.go)新增封闭错误类别：

- evaluation invalid；
- step budget exceeded；
- operator unsupported；
- selected branch unavailable；
- decision invalid。

### 3.4 未装配 application service

[strategy_routing_graph_evaluation.go](../../../internal/lottery/application/strategy_routing_graph_evaluation.go)组合：

- `StrategyRoutingGraphReader`；
- `MembershipTierFactReader`；
- `MembershipRoutingClock`；
- positive `maxFactAge`；
- `StrategyRoutingGraphStepBudget`；
- positive `maxDuration`。

[strategy_routing_graph_evaluation_error.go](../../../internal/lottery/application/strategy_routing_graph_evaluation_error.go)提供低披露 application wrapper。Service 存在于内部代码中，但没有进入 `cmd/growth-api`、HTTP adapter、Compose 或 Web。

### 3.5 架构停止线

[membership_routing_architecture_test.go](../../../internal/lottery/application/membership_routing_architecture_test.go)继续拒绝 generic `Rule` / `RuleEngine` / registry / DSL / untyped fact bag，并新增扫描：

- `cmd`；
- Lottery / infrastructure HTTP adapter；
- runtime app config；
- 非 acceptance Compose / Docker production source；
- Web production source。

这些位置不得提前引用 graph evaluation service、domain evaluator、decision 或 budget 标识符。

## 4. 两次小重构为什么先于新功能

真实项目演进不应让新实现复制旧逻辑后再“未来有空统一”。本节先做了两个独立、可回归的小提交。

第一步只抽 branch oracle：

```text
旧 concrete router 内部 tier switch
  -> package-private typed helper
  -> 旧 router 行为保持
  -> 新 evaluator 复用
```

第二步只抽 fact session：

```text
旧 application Route 中的 clock/read/freshness
  -> package-private helper
  -> 旧 service 保持调用顺序和错误分类
  -> 新 graph service 复用
```

这两步的价值不只是减少行数。它们明确了两个不同层次：

- domain helper 拥有业务 branch 语义；
- application helper 拥有 authority read、受控时间与 freshness 编排。

如果直接把两者揉成 `EvaluateRule`，provider、时钟和业务 branch 又会重新耦合。

## 5. 一次 evaluation 的三个 snapshot

本节把一次调用冻结为：

```text
one exact immutable graph
  + one canonical UTC evaluated-at
  + one authoritative membership fact
  -> one deterministic route
```

### 5.1 graph 只读一次

Application 只调用一次：

```go
graphs.FindByIdentity(evaluationCtx, identity)
```

并且必须检查返回 aggregate 的 `Identity()` 与请求 identity 完全相等，再调用 `Validate()`。不允许：

- `latest` / `active`；
- not-found 后换 revision；
- 仅比较 GraphID、不比较 Revision；
- 从裸 node/edge rows 边读边执行；
- 自动修复坏 graph 后继续。

### 5.2 Clock 只读一次

`MembershipRoutingClock` 提供业务逻辑时刻 `evaluatedAt`。它用于：

- 判断 fact 是否来自未来；
- 判断 fact 是否超过 `maxFactAge`；
- 写入 decision evidence。

它不是技术 timeout timer。Service 另用 `time.Now().Add(maxDuration)` 形成内部 wall-clock deadline。两个时间概念不能合并：可测试业务时刻由 port 注入，技术资源预算由 context deadline 承担。

### 5.3 fact 只读一次

无论 path 有 1 步还是 16 步，会员 reader 都只调用一次。全部 decision node 复用同一 immutable snapshot，因此不会出现：

```text
node 1 看见 premium
node 2 重新读取后看见 standard
```

这不是跨 MySQL 与会员 authority 的分布式事务，只是 evaluator 不主动在一条 path 中混用多个 provider 时刻。

## 6. Application 的固定执行顺序

`StrategyRoutingGraphEvaluationService.Evaluate` 的真实顺序是：

1. 校验非 nil caller context、非零 subject ref 和 exact graph identity；
2. 校验 service 非 nil、port 非 nil/typed-nil、`maxFactAge > 0`、budget 合法、`maxDuration > 0`；
3. pre-cancel 直接返回 caller error；
4. 依据 caller 与 internal deadline 的绝对时间派生 child context；
5. 用 child context 读取 exact graph 一次；
6. 先应用 caller/internal context 优先级，再处理 graph reader error；
7. 核对返回 identity 并重新 `Validate()`；
8. `graph.Depth() > maxSteps` 时，在 Clock/fact 前拒绝；
9. Clock 一次、fact reader 一次、freshness 校验一次；
10. 调用 domain evaluator；
11. 再次应用 caller/internal context 优先级；
12. 只有 `decision.Confirmed()` 且 final context 仍 live 才返回成功。

这个顺序产生重要负向保证：

| 失败点 | graph read | Clock | fact read | traversal |
| --- | ---: | ---: | ---: | ---: |
| invalid argument/config/pre-cancel | 0 | 0 | 0 | 0 |
| graph not found/error/wrong identity/invalid | 1 | 0 | 0 | 0 |
| graph depth 超 service budget | 1 | 0 | 0 | 0 |
| zero Clock | 1 | 1 | 0 | 0 |
| fact error/invalid/stale | 1 | 1 | 1 | 0 |
| valid input | 1 | 1 | 1 | 1 条实际 path |

读取依赖不仅要有 happy path，还要证明不该发生的调用确实为零。

## 7. Domain evaluator 为什么仍要 Validate graph

正常 MySQL reader 已经 strict Restore，application 也重新 Validate。Domain 入口再次执行 `graph.Validate()`，看起来有重复，但它守住一个独立 public function 的契约：

```go
EvaluateStrategyRoutingGraph(ctx, graph, fact, evaluatedAt, budget)
```

该函数不能假定所有 caller 都来自当前 application service，也不能信任同包测试或未来代码不会伪造 zero/derived field。Graph 最大 128 nodes、256 edges、depth 16；当前 `Validate()` 还会复制并规范排序 node/edge，因此复杂度上界是 `O(V log V + E log E)`，但在上述硬上限内仍是有界的 defense in depth。

它仍不会修图。验证失败只返回：

```text
zero StrategyRoutingGraphDecision + classified error
```

## 8. 为什么用迭代单路径，而不是 DFS 执行全图

Graph 是 rooted DAG，但一次会员路由在每个 decision 只选一条 edge。执行状态只需要：

```text
currentNodeID
visited node set
ordered path
step budget
```

循环逻辑是：

```text
check context
  -> find current node
  -> if terminal: build and confirm whole decision
  -> if decision: check exact rule + step budget
  -> typed branch/reason
  -> exact matching outgoing edge
  -> append one complete step
  -> move to successor
```

静态建图阶段使用 DFS 验证所有路径；运行阶段不应再 DFS 全图。否则会：

- 执行未选择分支；
- 把共享后继误解为多路径合并；
- 让一次业务 route 产生多个 target；
- 让 path 变成遍历日志而不是实际决定证据。

显式循环也比递归更容易在每步放置 context 与 budget checkpoint。

## 9. canonical order 不是 branch priority

第 28 节为了持久化稳定，将 edge canonicalize。当前排序下 `baseline_default` 可以出现在 `premium_override` 前面。

错误实现是：

```go
selected := graph.OutgoingEdges(nodeID)[0]
```

正确实现逐条比较 branch identity，并要求精确一个 match：

```text
branch = premium_override
  -> only edge.Branch() == premium_override
  -> matches must equal 1
```

这使执行结果与 slice 顺序、SQL 返回顺序和 Go map iteration 无关。找不到 premium edge 时也不能转去 `is_default=true` 的 edge；那是坏 graph/invariant failure，不是 standard 业务事实。

## 10. `baseline_default` 为什么仍然不是 catch-all

`baseline_default` 只代表：

```text
validated confirmed standard fact
```

以下情况全部没有 branch：

- zero/unknown/unsupported tier；
- fact not-found；
- provider unavailable/failure；
- fact subject mismatch；
- future/stale fact；
- caller cancellation；
- internal timeout；
- unknown rule/kind/branch；
- selected edge 缺失；
- step budget exceeded。

这条区分很关键：default 是 graph 中一个经过批准的业务出口，不是系统遇到不确定性时的“兜底成功”。

## 11. 静态 depth 与运行 maxSteps 是两个预算

第 28 节的 graph 静态预算是 schema 安全边界：

```text
longest root-to-terminal depth <= 16
```

本节的 service budget 是这一个 consumer 愿意承诺的运行工作量：

```text
1 <= maxSteps <= 16
```

Step 精确定义为：在 decision 上求得 branch，并沿一条 edge 移动到 successor。因此：

- decision 直达 terminal = 1 step；
- terminal 不另计 step；
- path length = 实际 step count；
- graph depth 16 + maxSteps 16 合法；
- maxSteps 0 或 17 连 service 都不能构造。

Application 在读取 graph 后先执行最坏路径准入：

```text
graph.Depth() <= maxSteps
```

这样同一 graph revision 不会因会员事实不同而出现“standard 短路径成功，premium 深路径跑到一半失败”的可用性差异。

Domain loop 仍保留 actual step hard stop。即使今天 graph Validate 已能证明 depth，运行 guard 也保护未来 validator 或调用路径回归。

## 12. maxDuration 不是 evaluated-at，也不是生产 SLO

`maxDuration` 是一次 application 调用的 cooperative technical budget。它的 timer window 在 graph read 前开始，并一直跨过 Clock、fact read 与 traversal，但不能写成严格的耗时不等式：

```text
context-aware graph/fact wait + evaluator checkpoints -> 可被 deadline 协作终止
MembershipRoutingClock.Now()                  -> 不接收 context，返回后才能观察 deadline
```

它不写入业务 decision，也不能表述为 P99。Context 只能协作取消：

- graph/fact reader 必须遵守传入 context 才能及时退出；
- `MembershipRoutingClock.Now()` 必须是有界本地调用；若它阻塞，deadline 只能在返回后的检查点被发现；
- 已进入的同步 Go 计算不能被 context 强制抢占；
- 当前 typed operator 是 O(1)，path 最多 16 steps，所以不可抢占区间有明确小上限。

若未来 operator 变成远程调用或长计算，应重新定义 per-operator cost、deadline 与隔离，不能继续拿当前 16-step 假设外推。

## 13. 相同 deadline 为什么要显式让 caller 获胜

若简单调用 `context.WithTimeout(callerCtx, maxDuration)`，caller 和 internal deadline 恰好相同时，谁先触发可能取决于 timer 调度。业务优先级不能交给调度偶然性。

实现先比较绝对 deadline：

```text
callerDeadline <= internalDeadline
  -> context.WithCancel(callerCtx)

internalDeadline < callerDeadline or caller has no deadline
  -> context.WithDeadlineCause(callerCtx, internalDeadline, privateCause)
```

因此：

- caller 更早：返回 caller 自身 error；
- caller 相等：仍由 caller 拥有；
- internal 严格更早：映射为 service timeout；
- deferred cleanup cancel：不能伪装成 internal timeout。

这也是为什么实现使用私有 internal cause，而不是只检查 `evaluationCtx.Err() == context.DeadlineExceeded`。

## 14. caller、internal 与 provider error 的固定优先级

依赖返回时按以下顺序分类：

1. 原始 caller context error；
2. caller live，但 evaluation child 已因 private internal deadline 结束；
3. 两个 context 均 live，才处理 dependency/provider error；
4. nil error 后再检查 returned value；
5. 所有 invariant 与 final context 都成立才成功。

由此得到：

| 同时可见的事实 | 最终类别 |
| --- | --- |
| graph error + caller canceled | caller cancellation |
| fact error + caller canceled | caller cancellation |
| provider error + internal deadline | internal evaluation timeout |
| provider 返回 `context.DeadlineExceeded`，两个 operation context 均 live | provider-owned failure/unavailable |
| value + error | error，value 丢弃 |

Provider 自己的 deadline 不能伪装成 caller；内部 budget 也不能通过 `errors.Is(err, context.DeadlineExceeded)` 冒充 caller deadline。

## 15. 低披露 error wrapper 怎样工作

`StrategyRoutingGraphEvaluationError` 保存：

```text
reviewed public class
private trusted cause
```

普通调用者只能通过：

- `Error()` 看到稳定 class 文案；
- `errors.Is` 匹配这一项 reviewed class。

它刻意没有 `Unwrap()`。需要可信诊断的内部代码必须显式调用 `Cause()`。因此内部 timeout：

```text
errors.Is(err, ErrStrategyRoutingGraphEvaluationTimedOut) == true
errors.Is(err, context.DeadlineExceeded) == false
```

同样，stored graph invalid 的 SQL/row/topology cause 不会通过 error chain 自动成为外部可匹配信息。

注意：会员 fact read 继续使用既有 `MembershipTierFactReadError`，所以 graph service 不重新包裹已经审核过的 fact not-found/unavailable/invalid/stale 分类。两个错误边界各自保留自己的故障域。

## 16. 成功 decision 保存什么

`StrategyRoutingGraphDecision` 保存：

- exact graph identity；
- schema version；
- root node ID；
- reached terminal node ID；
- final non-zero StrategyID；
- fact source 与 revision；
- canonical UTC evaluated-at；
- server-configured step budget；
- ordered actual path。

每个 `StrategyRoutingGraphPathStep` 保存：

```text
from_node_id
rule_code
selected_branch
reason_code
to_node_id
```

它不保存 subject ref、原始 tier payload、session、Principal、未走 branch、完整 graph、SQL、loaded Strategy、Award 或 Draw。

`Path()` 返回防御性副本。测试修改返回 slice 后，原 decision 仍必须 `Confirmed()`。

## 17. `Confirmed()` 为什么不能只检查 target 非零

一个非零 StrategyID 不足以证明 route 完整。`Confirmed()` 还核对：

- graph identity 与 schema 合法；
- root、terminal、target 非零且 root != terminal；
- target 与 evaluator 保存的 terminal target 一致；
- fact source/revision token 合法；
- evaluated-at 已 canonicalize 为 UTC 且无 monotonic 部分；
- path 长度在 1..budget；
- 第一条从 root 开始；
- 相邻 step 连续；
- 每步 rule 固定为 membership rule；
- branch/reason 精确配对；
- path 内不重复 node；
- 最后一条到达 terminal。

Evaluator 在构造 decision 前还逐步核对了 graph node/edge/terminal，因此 `Confirmed()` 是 value 自身的最后一道一致性检查，不是替代 graph validation 的完整证明。

## 18. 所有失败为什么必须返回整个零值

Domain 和 application 均冻结：

```text
success -> complete confirmed decision
failure -> zero decision + error
```

不返回：

- partial path；
- last successful node；
- guessed target；
- baseline fallback；
- `Confirmed() == false` 的 nil-error value。

原因是 partial prefix 没有稳定业务语义。调用方若只看见前两步，很容易把“执行被取消”误当成“已经做出部分路由决定”。当前没有持久副作用，丢弃局部内存 path 也是最安全、最简单的原子边界。

## 19. 并发模型与资源上限

Domain evaluator 的请求级状态全部在调用栈内：

- graph/fact/evaluated-at；
- current node；
- visited map；
- path slice；
- step count；
- context。

Application service 构造后只持有只读 ports 与配置，不保存上一次请求的 path、fact 或 timer。当前测试用 64 个并发 worker 验证：

- domain evaluator 对同一 immutable graph/fact 得到一致决定；
- application service 交错两组不同 subject、graph identity、tier、branch 与 target，逐个结果校验 identity/target/path；
- 两个 graph identity 和两个 subject 各自恰有 32 次读取，共享 Clock 总计 64 次，当前被测请求没有串线。

真实并发安全仍要求注入的 reader 与 Clock 自己可并发；service 不能替 adapter 隐式加锁。

资源量级为：

```text
graph validation: O(V log V + E log E), V <= 128, E <= 256
single traversal: O(path length * outgoing scan), path <= 16, v1 outdegree = 2
visited: <= 17 node identities
path: <= 16 steps
successful port calls: graph reader 1 + Clock 1 + membership fact reader 1
```

这只是复杂度上界，不是生产 latency、吞吐或 SLO 证据。

## 20. 测试矩阵怎样对应设计风险

### 20.1 concrete oracle 与 edge order

- standard/premium 与第 27 节 concrete router 的 target、branch、reason、fact provenance 和 evaluated-at 相等；
- fixture 刻意让 canonical baseline edge 排在 premium 前；
- premium 仍按 identity 选择 override；
- 两 branch 汇聚同一 terminal 时，target 相同但 path branch/reason 不丢失。

### 20.2 多步、深度与 unsigned identity

- branch 汇聚到共享 decision 后继续形成连续两步 path；
- depth 16 premium path 完整执行；
- budget 15 在 fact validation 前因 worst-depth admission 失败；
- GraphID、NodeID、StrategyID 使用 `MaxUint64` 仍不被 signed narrowing。

### 20.3 decision 与失败原子性

- zero/forged budget、graph、fact、evaluated-at 全部零决定；
- path slice mutation 不改写原 value；
- identity/schema/root/terminal/target/provenance/time/budget/path 各类伪造都让 `Confirmed()` 为 false；
- mid-traversal cancellation 与 final-success 前 cancellation 均返回零决定。

### 20.4 application orchestration

- graph -> Clock -> fact 的精确顺序；
- exact identity、一个 shared child context 和 1/1/1 调用次数；
- invalid argument/typed-nil config 零依赖；
- graph error/wrong identity/stored-invalid/depth 超限零 Clock/fact；
- zero Clock 零 fact；
- fact value+error 中 error 胜出。

### 20.5 timeout ownership

- graph block 由 internal deadline 释放，不使用 `time.Sleep` 猜调度；
- fact block 时 internal timeout 胜 provider error；
- caller 更早 deadline 端到端传给 reader 并原样返回；
- caller earlier/equal 与 internal strictly earlier 用绝对 deadline helper 验证；
- cleanup cancel 不会被分类为 timeout；
- 内部 timeout 不匹配 `context.DeadlineExceeded`。

### 20.6 fuzz、race 与停止线

- fuzz 将 depth/tier/budget/future/aggregate mutation 投影为有界输入，要求不 panic、不循环、不返回 partial decision；
- domain/application 各有 64-worker 并发证据；
- architecture guard 拒绝 generic abstraction，并在指定 runtime/HTTP/Web/Compose production source 中扫描五个 evaluator 标识符的直接装配。

测试文件存在属于 CODE-EVIDENCE；命令真实运行才是执行证据。当前实现候选已实际通过定向普通/race/20 轮 shuffle/vet、10 秒 fuzz、全仓普通/race、atomic coverage、Web test/typecheck/build 与第 28 节 MySQL 8.4.11 上游回归；完整文档/索引后的聚合门禁与远端冻结状态仍以第 29 节 QA 为准。

## 21. 建议学习与复现实验顺序

### 实验一：先看旧 router 怎样决定 branch

阅读：

1. [membership_routing.go](../../../internal/lottery/domain/membership_routing.go) 的 `evaluateMembershipRoutingBranch`；
2. `RouteMembershipStrategy` 怎样再由 branch 选 policy target；
3. [membership_routing_branch_test.go](../../../internal/lottery/domain/membership_routing_branch_test.go) 的 standard/premium/future/UTC 用例。

先确认“branch 语义”和“target 查找”是两件事。

### 实验二：只运行纯 domain evaluator

阅读：

1. `StrategyRoutingGraphStepBudget`；
2. `EvaluateStrategyRoutingGraph` 的入口校验顺序；
3. canonical-order trap fixture；
4. shared successor 与 depth-16 fixture；
5. `Confirmed()` 和 zero-decision assertion。

运行：

```bash
go test -count=1 ./internal/lottery/domain \
  -run='StrategyRoutingGraph|EvaluateMembershipRoutingBranch'
```

### 实验三：再看 application 怎样控制 I/O

阅读：

1. `StrategyRoutingGraphEvaluationService.Validate`；
2. `Evaluate` 的依赖顺序；
3. `readFreshMembershipTierFact`；
4. context deadline helper 与 error wrapper；
5. blocking reader tests。

运行：

```bash
go test -count=1 ./internal/lottery/application \
  -run='StrategyRoutingGraphEvaluation|Lesson29'
```

### 实验四：最后验证没有偷偷上线

```bash
git diff 90844c1..HEAD -- \
  cmd \
  web \
  deploy/compose \
  internal/lottery/adapter/httpapi \
  internal/infrastructure/httpapi \
  internal/platform/appconfig \
  migrations
```

预期没有本节 graph evaluator 的 runtime wiring、公开 route、UI 或 Migration。不要把 product/ADR 中的文字命中误当成生产代码装配；架构测试扫描的是对应 production sources。

## 22. 当前验证命令与证据边界

本节文档形成前，以下命令已在当前 Lesson29 实现上真实 exit 0：

```bash
go test -count=1 \
  ./internal/lottery/domain \
  ./internal/lottery/application

go test -race -count=1 \
  ./internal/lottery/domain \
  ./internal/lottery/application

go test -shuffle=on -count=20 \
  ./internal/lottery/domain \
  ./internal/lottery/application

go vet \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

同一 production 实现候选还已实际执行：

```bash
go test ./internal/lottery/domain \
  -run='^$' \
  -fuzz='^FuzzEvaluateStrategyRoutingGraphNeverPanicsLoopsOrReturnsPartialDecision$' \
  -fuzztime=10s
go test -count=1 ./...
go test -race -count=1 ./...
```

10 秒 fuzz 完成 2,569,203 次执行，新增 1 个 interesting input（总数 41）；Lottery domain/application atomic coverage 为 93.6%/88.3%，合并 92.1%；Web 为 19/19 files、152/152 tests，typecheck/build 通过；第 28 节 disposable MySQL 8.4.11 六组 Integration 上游回归也通过。它们不证明 runtime 已装配或业务 SLO。

完整文档/索引收口后仍须执行 `make verify`、最终 doccheck/diff、stop-line、历史、产物清理和远端 refs 核查。任何尚未执行的 post-doc/freeze 项都不能预写通过，远端 SHA 与累计分支状态尤其只能以实际 refs 为准。

## 23. 本节没有新增公开 API 或运行时能力

- 没有 HTTP/MCP/Agent endpoint；
- 没有 request/response DTO、header、status 或 error code；
- 没有 `cmd/growth-api` composition；
- 没有长期 MySQL graph grant；
- 没有 Migration、Redis key、RabbitMQ event 或 PostgreSQL projection；
- 没有 React 页面、菜单、路由、按钮或浏览器 E2E；
- 没有真实 membership adapter；
- 没有 Activity、publish、active/latest revision；
- 没有 session、Principal、RBAC/ABAC 或 data scope；
- 没有 Strategy load、weighted selection、Award、Draw、库存或 Benefit。

第 28 节 MySQL graph Repository 继续存在，但本节 application service 只是依赖它的 reader port；代码中没有把真实 adapter 注入 runtime。

## 24. 为什么本节不需要新增数据库或 Docker 依赖

Evaluator 消费现有 immutable graph aggregate 与 consumer-owned fact port，新增状态全部是调用内存中的短生命周期 value。因此本节没有理由新增表、缓存或消息：

```text
no persistent evaluator state
no decision audit consumer
no runtime composition
no external API
```

用户已安装的 MySQL、Redis、RabbitMQ 与 PostgreSQL 仍可用于全仓既有回归，但“依赖可用”不是“本节必须用上”的理由。真实架构选择应由数据所有权、生命周期和消费者推动，而不是为了技术栈数量堆组件。

## 25. 可准确写进简历的边界

在本节实现和最终验收完成后，可以表述：

> 为 Lottery 不可变 Strategy 路由图实现封闭类型化求值器：按 exact GraphID/Revision 读取并复核有界 DAG，在单一受控时刻只读取一次权威会员事实，以 exact branch lookup 迭代形成最多 16 步的 defensive-copy 决策路径；通过最坏深度准入、运行 hard stop、caller/internal/provider 超时优先级、私有 cause 低披露包装与全失败零决定协议，防止 default 掩盖未知事实及取消后半结果逃逸。

不能表述：

- 实现了通用规则引擎或动态 DSL；
- graph 已发布、active 或进入线上 API；
- 已接入真实会员中心；
- route success 等于 eligibility 或 authorization allow；
- 已执行 Strategy/Award 随机选择或正式 Draw；
- 已完成权限系统、前端管理台或浏览器闭环；
- 单元/race/fuzz 结果等于生产 P99、容量或可用性。

## 26. 第 30 节为什么自然出现

第 29 节只能回答：

> 明确给出一份合法 immutable graph revision 时，怎样可信地执行？

它仍不能回答：

> 哪个 Activity 在哪个生命周期阶段，应该使用哪一份 graph revision 和哪一组 Strategy 配置？

因此下一节才引入 Activity 发布与绑定问题，包括：

- Activity aggregate 与状态机；
- draft/approve/publish/retire/rollback；
- active binding 与 immutable revision；
- Strategy 配置的发布一致性；
- 并发发布、历史解释和回滚。

第 31～35 节再按公共访问控制模型、真实会话、服务端 RBAC、前端 capability projection、越权与浏览器 E2E 的顺序演进。执行成功、发布生效与访问允许仍是三个正交决定。

## 27. 本节复盘

本节最重要的不是写了一个 `for` 循环，而是把一次决定的可信边界拆清楚：

1. 第 27 节 concrete router 继续是业务 branch oracle；
2. 第 28 节 validated graph 继续拥有结构真相；
3. 两次小重构先共享 branch 与 fact session，不复制旧语义；
4. Application 只负责 exact graph、受控时刻、权威事实和技术 deadline 的编排；
5. Domain 只做封闭类型化、确定性、单路径求值；
6. canonical storage order 不成为隐式 priority；
7. default 只承接 confirmed standard，不吞未知和故障；
8. 静态 depth、service worst-depth admission 与 loop hard stop 形成三层有界控制；
9. caller、internal、provider error 的责任优先级不交给调度偶然性；
10. 成功必须一次形成完整 immutable decision，任何失败只返回零值；
11. 架构 guard 证明指定 production source 中没有五个 evaluator 标识符的直接装配；章节 diff 再补 Migration、grant 与其他边界；
12. Activity 发布、权限、正式 Draw 继续按真实需求顺序留给后续章节。

渐进式架构不是每节都把更多组件“接上线”，而是每节先把下一步将要依赖的一项事实做成可验证、可失败关闭、也不夸大边界的能力。
