# ADR-0025：以封闭类型化执行器求值 Lottery Strategy 路由图

- **状态：** 已接受
- **日期：** 2026-08-30
- **负责人：** GrowthOS 架构组
- **适用范围：** 第 29 节“实现规则决策引擎”中的 Lottery Strategy 路由图求值
- **上游基线：** [Lottery 会员等级 Strategy 路由基线 v1](../product/membership-strategy-routing-v1.md)、[Lottery Strategy Routing Graph 基线 v1](../product/lottery-strategy-routing-graph-v1.md)
- **替代关系：** 不替代 ADR-0019、ADR-0023 或 ADR-0024；在既有规则所有权、具体会员路由和不可变 graph 边界内新增受限执行语义

## 背景

第 27 节已经交付一个具体、可执行的会员 Strategy 路由语义：

```text
confirmed premium
  -> premium_override
  -> premium Strategy target

confirmed standard
  -> baseline_default
  -> baseline Strategy target
```

unknown tier、缺失/过期/未来/损坏的会员事实、provider 故障和 caller cancellation 都不能被解释成 `baseline_default`。这个 concrete router 是第 29 节的语义 oracle，而不是需要被新“引擎”推翻的临时代码。

第 28 节进一步把同一组 rule/branch/default/target 词汇保存为 Lottery-owned、create-only、不可变的 `StrategyRoutingGraph`。一份可恢复 graph 已经具备：

- exact `(GraphID, Revision)` identity；
- exact `schema_version = 1`；
- 显式 decision root；
- `decision` / `strategy_target` 两种封闭 node kind；
- `premium_override` / `baseline_default` 两种封闭 edge；
- 全可达、无环、terminal 无出边和 exact branch set；
- 128 nodes、256 edges、最长路径 16 edges 的静态安全边界；
- Repository 按 exact identity 读取，并在数据库 snapshot 结束后严格 Restore；
- 未知 schema/rule/kind/branch、坏 root、环、孤儿和超限内容失败关闭。

但“能保存和恢复 graph”仍不等于“能执行 graph”。第 29 节必须新增并冻结以下运行语义：

1. 谁提供 graph、事实和求值时刻；
2. 一次调用是否允许在不同 node 重读配置或事实；
3. 怎样从 root 确定地走到唯一 terminal；
4. path 记录什么、失败是否允许返回半条 path；
5. 静态 graph depth 与运行时 step/time budget 怎样协作；
6. caller cancellation、内部 deadline 和 provider error 同时出现时谁优先；
7. unknown tier、missing branch 或坏 graph 是否会被 default 吞掉；
8. executor 到达 Strategy target 后是否顺带加载 Strategy、执行随机选择或形成 Draw。

第 29 节标题使用“决策引擎”，但当前证据只支持一个 Lottery 专属、封闭类型、单事实、单路径的 graph evaluator。名称不能倒逼出跨上下文 `RuleEngine`、operator registry、DSL 或运行时插件系统。

## 决策驱动

1. 执行语义必须与第 27 节 concrete router 等价，不能让持久化 graph 重新解释 `default`；
2. executor 只能消费第 28 节已经成功 Validate / Restore 的 immutable graph aggregate，不能消费裸 SQL rows；
3. graph 必须按调用方给定的 exact identity 读取，不能隐式查找“latest revision”；
4. 一次求值只使用一份 graph、一份权威会员事实和一个受控求值时刻，保证可解释与可复现；
5. v1 的每个 decision 都是同一个 concrete membership rule，没有 node 参数、表达式或异构 operator；
6. 遍历必须确定、有限、可取消，不能依赖 map iteration、edge 返回顺序或递归调用栈；
7. 静态 `depth <= 16` 不能代替运行时 step budget 和 wall-clock budget；
8. provider 的 deadline/error、caller 的 cancellation/deadline 和 evaluator 自己的 max duration 必须可区分；
9. 失败不能返回可被误用的 target 或 partial decision path；
10. 到达 `strategy_target` 只形成 Strategy identity 路由决定，不加载 Strategy、不做 Award selection、不持久化 Draw/Result；
11. 第 29 节仍没有 Activity 发布、真实运行时装配、公开 API、UI、认证或授权；
12. 新抽象必须由当前封闭协议证明，不能为尚未出现的第三种 rule 预制扩展点。

## 评估过的方案

### 方案一：继续使用 `if/else`，读取 graph 后直接按 tier 挑 StrategyID

示意：

```text
if tier == premium {
    return graph 中的 premium target
}
return graph 中的 default target
```

| 优点 | 代价 / 风险 |
| --- | --- |
| 实现短；一层 graph 可以得到正确 target | 绕过 root、node 和 edge，graph 拓扑成为无效摆设 |
| 容易复用第 27 节 switch | 无法形成多步 path，也无法证明 shared successor 或 depth 语义 |
| 不需要运行预算 | 容易把 unknown 重新落入 `else/default` |

**结论：拒绝。** 第 27 节 switch 继续作为 branch 语义 oracle，但第 29 节必须执行第 28 节保存的 exact graph，而不是从表中重新拼一个隐藏 policy。

### 方案二：executor 直接查询 graph/node/edge 裸行，并在运行时尽力修复

| 优点 | 代价 / 风险 |
| --- | --- |
| 可以边读边走，表面减少 aggregate materialization | executor 与 MySQL schema、NULL 和查询顺序耦合 |
| 缺 edge 时可以临时补 default | 绕过严格 Restore，使 revision 对应的实际运行内容发生漂移 |
| 可以忽略不可达或未知 node | 坏数据被“修好后执行”，故障不再可追溯 |

**结论：拒绝。** Executor 只接受 `StrategyRoutingGraph`。MySQL adapter 仍负责 bounded read 与 strict Restore；读取失败或 graph 损坏时返回零决定，不执行、不修复。

### 方案三：每到一个 decision 就重新读取 graph 或会员事实

| 优点 | 代价 / 风险 |
| --- | --- |
| 每个 node 看似都能获得“最新”内容 | 一条 path 可能混用多个 revision 或多个会员事实时刻 |
| 将来不同 operator 可各自取数 | provider 调用次数随 path 深度放大，取消和错误优先级变复杂 |
| 不需要在调用开始准备完整输入 | 无法重放；同一请求前后可能在 premium/standard 间跳变 |

**结论：拒绝。** 一次调用只读 exact graph 一次、Clock 一次、会员事实一次。全部 decision 共享同一 immutable graph、同一事实 snapshot 和同一 evaluated-at。

### 方案四：先实现 operator registry、通用 `Rule` 与 `map[string]any` fact bag

| 优点 | 代价 / 风险 |
| --- | --- |
| 表面上容易新增渠道、地域、库存或权限规则 | 当前只有一个 membership rule，无法证明通用输入/输出协议 |
| 可以按 rule code 动态 dispatch | unknown operator、类型检查、依赖声明、sandbox、版本兼容均未定义 |
| 课程标题看起来更像“引擎” | `map[string]any` 会混合 Lottery、Participation、Governance 和 Inventory 事实 |

**结论：拒绝。** v1 evaluator 只认识 `MembershipTierFactSnapshot` 与 `MembershipStrategyRoutingRuleCode`。若未来出现第二种真实 graph decision，再用新 ADR 比较封闭 union、visitor 或 registry；不能提前选择。

### 方案五：把 condition/expression/script/DSL 存入 node 并动态解释

| 优点 | 代价 / 风险 |
| --- | --- |
| 运营可以无代码编写条件 | 第 28 节 schema 没有 expression 字段或类型协议 |
| 一个 evaluator 可以支持复杂逻辑 | 缺少 parser、类型系统、资源预算、安全沙箱和兼容策略 |
| 可以快速演示“规则平台” | 代码与持久化 schema 事实不一致，未知表达式可能被尽力执行 |

**结论：拒绝。** 第 29 节只执行 schema v1 已知 token；不实现 DSL、DMN、OPA、XACML、脚本或远程 operator。

### 方案六：第 29 节顺便引入 Activity 与 active revision

| 优点 | 代价 / 风险 |
| --- | --- |
| 能回答“线上到底执行哪张图” | Activity 的所有者、生命周期、发布原子性和回滚尚未建模 |
| 可以隐藏 exact identity 参数 | evaluator 与发布选择混成一个决定，无法单独测试 |
| 更接近最终运营流程 | 把第 30 节问题倒灌到执行正确性章节 |

**结论：拒绝。** 第 29 节调用方必须显式提供 exact graph identity。第 30 节才决定 Activity 怎样引用、批准、发布或回滚 immutable revision。

### 方案七：到达 terminal 后顺带加载 Strategy 并执行 Award selection

| 优点 | 代价 / 风险 |
| --- | --- |
| 一次调用即可返回 Award | 路由、Strategy snapshot、随机选择和正式 Draw 语义混在一起 |
| 可复用现有 StrategyReader / WeightedSelector | target Strategy 不存在、读取失败、随机源失败会污染 graph evaluation 错误 |
| 容易对接现有 ephemeral API | 可能把不持久化选择冒充正式结果 |

**结论：拒绝。** Executor 只返回非零 StrategyID 和 path evidence。加载 Strategy、加权选择、Draw identity、幂等、库存与发奖继续由各自用例负责。

### 方案八：递归执行选中的分支

| 优点 | 代价 / 风险 |
| --- | --- |
| 代码接近树定义 | cancellation / step budget checkpoint 容易散落在调用栈中 |
| graph depth 已限制为 16 | partial path 和错误 step 的所有权不直观 |
| 实现量小 | 未来 depth 预算调整会增加栈与调试风险 |

**结论：不采用。** 当前只走一条 path，使用显式 `currentNodeID` 的迭代循环更容易审计、预算、取消和构造零或完整结果。

### 方案九：Lottery-owned closed typed evaluator + exact snapshots + iterative single path

| 优点 | 成本 / 风险 |
| --- | --- |
| 完全复用现有 graph 与 membership 领域语言 | v1 只能执行一种 concrete rule |
| exact identity 可定位同一 topology，single snapshots 让同一输入结果确定 | 每次求值要再次 Validate graph；尚无历史 fact 回读/持久审计，不能完整重放 |
| step/time budget 与取消点可明确验证 | 需要新的 decision/path/error 契约 |
| 不引入 registry/DSL/Activity/selection | 尚不能回答线上 active revision |

**结论：采用。** 这是当前最小、可验证且不夸大能力的执行边界。

## 决策

### 1. 执行器属于 Lottery，并保持封闭类型

新增能力命名为 Strategy routing graph evaluation，而不是跨上下文通用 Rule Engine。Domain evaluator 的输入只能是：

- 非 nil `context.Context`，只承载取消和 deadline；
- 已构造的 `StrategyRoutingGraph`；
- 已构造的 `MembershipTierFactSnapshot`；
- application 捕获并规范化的一次 `evaluatedAt`；
- 1～16 的 step budget。

禁止输入：

- `map[string]any`、JSON condition 或自由 fact bag；
- 裸 SQL rows、table name 或 query handle；
- client 提交的 tier；
- operator registry、插件、脚本或远程回调；
- Activity、Principal、Role 或 permission；
- Strategy aggregate、Award selector 或随机源。

Evaluator 可以使用 package-private helper 共享第 27 节 `fact + evaluatedAt -> branch + reason` 的 concrete 语义。现有 `RouteMembershipStrategy` 与 graph evaluator 必须共同使用或通过等价测试锁定这一 helper，防止两个 switch 逐渐漂移；不为了共享这一小段语义导出通用 `Rule` 接口。

### 2. Application service 只组合现有窄端口

Application 新增未装配的 graph evaluation service，只依赖：

- `StrategyRoutingGraphReader`；
- `MembershipTierFactReader`；
- `MembershipRoutingClock`；
- positive `maxFactAge`；
- `maxSteps`，范围 1～16；
- positive `maxDuration`。

构造器拒绝 nil、typed-nil、zero/negative duration、zero step 或大于 16 的 step。Budget 是服务端受控配置，不由浏览器、HTTP 参数或会员 provider 指定。

一次 `Evaluate(ctx, subjectRef, graphIdentity)` 采用以下顺序：

1. 验证 ctx、subject ref、exact graph identity 与 service configuration；非法输入零依赖调用；
2. 若 caller context 已取消，直接返回 caller error；
3. 从 caller context 派生 positive `maxDuration` child deadline；timer window 横跨 graph read、Clock、fact read 与 traversal，但只协作取消接收 context 的依赖，本地 Clock 返回后立即检查 deadline；
4. `FindByIdentity(childCtx, exactIdentity)` 恰好一次；不查 latest、不 list、不重试成另一个 revision；
5. 检查 caller/internal deadline，再处理 reader error；reader 返回 nil error 时仍检查 identity 完全相等且 `graph.Validate()` 成功；
6. 调用 Clock 恰好一次，规范为 UTC；检查 caller/internal deadline并拒绝 zero instant；
7. `FindMembershipTierFact(childCtx, subjectRef)` 恰好一次；检查 caller/internal deadline，再处理 provider error；
8. 校验 fact、subject 相等、`ObservedAt <= evaluatedAt` 和 `evaluatedAt - ObservedAt <= maxFactAge`；
9. 使用同一 graph、fact、evaluated-at 和 maxSteps 执行 domain evaluator；
10. 再次检查 caller/internal deadline；只有完整且 `Confirmed()` 的 decision 才返回成功。

Graph 使用 exact immutable identity，因此不参与“最新时刻”的选择；先读取 graph 可以让 missing/corrupt configuration 在 Clock 和会员 provider 调用前失败。Clock 在会员 fact read 前捕获一次，保持 future/freshness 判断共享一个受控时刻。

本 ADR 不把 service 装配进 `growth-api` 或 Compose。Application port 的存在不自动授权长期运行账号读取 graph 表。

### 3. 一次求值固定三个 snapshot

同一次调用只允许：

```text
one exact graph snapshot
  + one canonical evaluated-at
  + one authoritative membership fact snapshot
  -> one deterministic decision or one error
```

遍历 node 时不得：

- 重新 `FindByIdentity`；
- 改查另一个 revision；
- 再次调用 Clock；
- 再次读取 membership fact；
- 读取 Strategy/Award；
- 将上一步 path 写回数据库或缓存。

这不表示 graph 与会员 provider 获得了分布式 snapshot isolation。它只表示 evaluator 对已经取得的 immutable values 保持一致，不在一条 path 内主动制造版本混用。

### 4. 采用确定的迭代单路径算法

执行从 `graph.RootNodeID()` 开始，维护：

- 当前 node ID；
- 已执行 edge/step 计数；
- 当前完整 path slice；
- 当前 path 内已见 node ID 集合，作为 defense-in-depth cycle guard。

每轮遵守：

1. 检查 context；
2. 用 `graph.Node(current)` 读取 node，missing 为 graph invariant breach；
3. `strategy_target` 只在 StrategyID 非零且 path 已完整时终止；
4. `decision` 必须携带 exact v1 membership rule；
5. 在执行下一条 edge 前检查 step budget；
6. 使用同一 fact/evaluated-at 得到 exact branch 与 reason；
7. 在 `graph.OutgoingEdges(current)` 中按 branch identity 查找恰好一条 edge；
8. 不能使用 collection 的第一个元素，不能用 map iteration，不能“未命中就找 default”；
9. 将 `from/rule/branch/reason/to` 作为一个完整 step 追加，再移动到 successor；
10. 重复 node、missing successor、unknown kind/rule/branch、duplicate/missing selected edge 均失败关闭；
11. 到达 terminal 后再次检查 context 和完整 decision invariant，再原子返回。

这是 deterministic single-path traversal，不是对全图 DFS。Shared successor 对 graph 是合法 DAG 合流；一次调用只走一条 edge，因此不会同时合并两个父 path。Visited set 只阻止同一路径重复，不能把合法的静态 shared successor 误报为 cycle。

即使 Repository Restore 已验证 graph，executor 入口仍调用 `Validate()`，并保留 step hard stop 与 visited guard。重复校验在 128/256/16 上限内是可接受的防御；它不执行自动修复。

### 5. Step budget 为 1～16，并以 graph 最坏路径做准入

Step 定义为“从一个 decision 选择并走过一条 edge”。

- 一个 decision 直接到 terminal 消耗 1 step；
- terminal 本身不额外消耗 step；
- path 中 step 数等于实际走过的 edge 数；
- budget 小于 1 或大于 16 是无效配置；
- graph 的静态 depth 仍必须不超过 16；
- service 在读取 graph 后要求 `graph.Depth() <= maxSteps`，超出时在读取 Clock/会员 fact 前失败；
- loop 在追加每条 edge 前再次检查实际 step count，防止未来 validator 或调用链回归绕过准入。

选择按 graph 最坏路径做准入，而不是“浅分支先成功、深分支运行到一半再失败”，可以让同一 graph revision 在当前 service budget 下对所有合法事实都具备有限执行承诺。第 30 节若为不同 Activity 配置不同 budget，需要重新决定发布时怎样验证，不在本节暗中引入。

Step budget exceeded 返回稳定技术错误和零 decision；不把已经走过的 prefix 暴露成部分成功。

### 6. Positive max duration 覆盖整个 application use case

`maxDuration` 必须为正，由 application constructor 收敛。Service 在所有输入和配置通过、caller 尚未取消后创建一个 child context：

```text
caller context
  -> internal deadline = monotonic now + maxDuration
  -> caller deadline 早于或等于 internal：WithCancel(caller)
  -> internal deadline 更早：WithDeadlineCause(caller, internal, private cause)
  -> graph read
  -> clock
  -> fact read
  -> bounded traversal
```

不为每个 node 创建 timer，不在 domain 内读取 wall clock，也不把 `maxDuration` 当业务 evaluated-at。两类时间含义必须分离：

- `evaluatedAt`：会员事实 future/freshness 与 decision evidence 使用的业务逻辑时刻；
- child deadline：这次 application 调用最多等待多久的技术资源预算。

Context cancellation 是协作式的：service 在每个依赖边界前后和 evaluator 每个 step 检查 context；无法保证抢占一个完全不遵守 context 的 provider，也不声称取消发生在最终检查之后仍能撤回已经返回的内存值。本节没有副作用，因此不存在 commit outcome unknown。

`MembershipRoutingClock.Now()` 是既有的本地业务时刻端口，不接收 context。当前契约要求它保持有界、无远程 I/O；若一个错误实现长期阻塞，child deadline 会继续流逝，但只能在 `Now()` 返回后的检查点被观察。因此 `maxDuration` 不是 wall-clock 硬上界。若未来 Clock 需要 I/O，必须重设计为 context-aware 端口，而不是沿用当前接口并宣称可取消。

这里不能只启动两个相同截止时间的 timer 然后假设 caller 固定获胜。Go `context` 保存第一次取消原因；完全相同 deadline 的调度先后不是业务优先级。实现必须先比较绝对 deadline：caller 更早或相等时只派生普通 child，internal 更早时才安装私有 timeout cause。内部 cause 通过受控诊断通道保留，对外稳定类不能 `errors.Is(context.DeadlineExceeded)`；构造器返回的 cleanup cancel 也不能被误判为内部超时。

### 7. Caller、内部 budget 与 provider error 的优先级固定

在 graph reader、Clock、fact reader 和 domain evaluator 每个边界返回后，application 按以下顺序决定错误：

1. **caller context error**：若原始 caller context 已经 canceled/deadline exceeded，原样返回 caller error；
2. **internal max-duration error**：caller 仍 live，但 child context 因本服务 maxDuration 到期，返回稳定 evaluation-timeout class；
3. **provider/dependency error**：caller 和 child 都 live，才按 graph Repository 或 membership provider 的稳定分类处理返回错误；
4. **returned value/invariant error**：依赖 nil error 后再验证 identity、graph、fact、path 与 decision；
5. **success**：所有 context 与 invariant 都成立才返回完整 decision。

因此：

- provider 返回错误的同时 caller cancellation 已可观察，caller cancellation 胜出；
- child deadline 到期而 caller 仍 live，内部 timeout 胜出，不伪装成 provider unavailable；
- provider 自己返回/包装 `context.DeadlineExceeded`，但 caller 与 child 都 live，仍是 provider-owned unavailable/failure，不冒充 caller 或 internal timeout；
- step budget exceeded 只有在 caller 与 child 均 live 时才返回；
- error 与看似合法 graph/fact 同时返回时，error 胜出，不能继续执行 value。

错误的普通文本只暴露稳定类别，不输出 SQL、DSN、原始会员 payload 或完整 graph topology。可信诊断若保留 cause，必须通过既有受控 error channel，不能改变公共 `errors.Is` 语义。

### 8. Unknown、default 与坏 graph 全部失败关闭

`baseline_default` 仍只承接 confirmed `standard`，不是 catch-all。

| 情况 | 结果 |
| --- | --- |
| confirmed premium | exact `premium_override` |
| confirmed standard | exact `baseline_default` |
| unknown/unsupported tier | invalid fact，零 decision |
| fact missing/stale/future/corrupt | 对应稳定 fact error，零 decision |
| provider unavailable/failure | 技术错误，零 decision |
| caller/internal deadline | 对应 context/budget error，零 decision |
| graph unknown schema/rule/kind/branch | invalid graph，零 decision |
| selected branch 缺失或重复 | graph invariant breach，零 decision |
| terminal 缺 target 或仍有出边 | invalid graph，零 decision |

Executor 不根据 `IsDefault()` 扫描第一条 default edge来替代 exact branch lookup；`is_default` 是持久图的结构证据，不重新定义会员 tier 语义。

### 9. Decision 与 path 只表达完整、同步、非持久路由结果

成功 decision 至少携带：

- exact graph identity 与 schema version；
- root node ID 与最终 terminal node ID；
- 非零 Strategy target ID；
- fact source 与 fact revision；
- canonical UTC evaluated-at；
- 1～maxSteps 条有序 path steps。

每个 path step 至少携带：

- from node ID；
- concrete rule code；
- selected branch code；
- stable reason code；
- to node ID。

Path 表达“实际走过的 edge”，不是全图、候选路径或 SQL trace。中间 `to` 可以是另一个 decision；只有 decision 顶层 target 与最终 terminal node 表示 Strategy target。

`Confirmed()` 必须至少验证：

1. graph identity/schema/root/terminal/target 非零且合法；
2. fact source/revision 与 evaluated-at 完整；
3. path 长度在 1～maxSteps 以内；
4. 第一条 From 等于 root；
5. 相邻 step 的 To/From 连续；
6. 每步 rule/branch/reason 是 exact v1 配对；
7. 最后一条 To 等于 terminal；
8. executor 形成 decision 时，path 每条 edge 和 terminal target 与输入 graph 完全一致。

Path accessor 返回防御性副本。Decision 不包含 subject ref、原始会员 payload、loaded Strategy、Award、随机结果或权限信息。

任何 invalid input、依赖错误、取消、timeout、budget exceeded、bad graph 或 internal invariant breach 都返回整个 zero decision。不得返回：

- partial target；
- partial path；
- “最后一个成功 node”；
- baseline fallback；
- nil error + `Confirmed() == false`。

如需失败 step 诊断，只能通过低披露错误/可信日志记录，不改变业务返回值。第 29 节不持久化 decision/path，也不宣称形成合规审计日志。

### 10. v1 多步语义的诚实边界

Schema v1 的全部 decision 都使用同一个 rule code，且 node 不含不同参数。对于同一 membership fact：

- premium 在每个 decision 都选择 `premium_override`；
- standard 在每个 decision 都选择 `baseline_default`。

因此 v1 可以验证多步 traversal、branch-specific topology、shared successor、budget、cancellation 与 path evidence，但不能声称已经支持“会员 -> 地域 -> 渠道 -> 库存”等异构决策。为了展示深度而堆叠相同 decision 也不是推荐的生产建模；真实 graph 应保持满足业务语义的最浅结构。

未来只有在第二种真实 graph rule 出现，并明确其事实所有者、输入类型、错误和 side-effect contract 后，才重新评估 closed union、visitor 或 registry。不得把 v1 的重复 membership node 当成通用扩展性证据。

### 11. 第 29 节停止线

本节明确不实现或修改：

- 通用 `Rule` / `RuleEngine` / `DecisionEngine` / operator registry；
- `map[string]any` fact bag、DSL、DMN、OPA、XACML、expression、script 或 remote call node；
- graph create/update/list/latest/publish/retire/rollback；
- Activity aggregate、Activity -> graph binding 或 active revision；
- MySQL graph adapter 的 runtime composition、长期 API graph grants 或 graph cache；
- HTTP/MCP/Agent route、DTO、Nginx、Compose 行为或 React UI；
- session、Principal、RBAC/ABAC、tenant/data scope 或前端权限裁剪；
- Strategy load/cache、WeightedSelector、CryptoSource 或 Award selection；
- 正式 Draw/Result、幂等、库存、积分扣减或 Benefit 发放；
- decision/path 持久化、消息发布、审计后台或浏览器 E2E。

现有 ephemeral Lottery API、React 页面、Strategy Repository/cache/selector、Participation 前置资格链和长期 Compose 运行权限保持不变。Graph evaluator、application service 与 MySQL graph Repository 在本节仍不进入 production composition root。

## 错误分类

第 29 节应形成或复用以下稳定语义；具体 Go 名称可以在实现中保持一致但不能合并不同故障域：

| 故障域 | 语义 | 是否返回 decision |
| --- | --- | --- |
| caller contract | nil ctx、zero subject、invalid graph identity | zero |
| service configuration | nil/typed-nil dependency、invalid max age/steps/duration | zero |
| graph repository | not found、stored invalid、retryable/failure | zero |
| graph result | nil-error wrong identity、zero/forged graph | zero |
| controlled clock | zero instant | zero |
| membership provider | not found、unavailable、read failure、invalid payload | zero |
| membership value | subject mismatch、future、stale、unsupported tier | zero |
| execution budget | graph depth/actual steps exceed configured 1～16 | zero |
| internal duration | child deadline expires while caller remains live | zero |
| graph evaluation | missing selected edge、unknown rule/kind/branch、cycle/invariant breach | zero |
| decision invariant | executor 形成不完整或不一致结果 | zero |
| caller context | caller canceled/deadline exceeded | zero |

不存在“no matching branch 所以成功 default”或“partial path + error”的类别。

## 影响

### 正面影响

- 第 27 节 concrete route 与第 28 节 immutable graph 首次形成可执行闭环；
- exact graph/fact/time snapshots 使相同输入的 target 和 path 确定，并让 topology 可重新定位；
- iterative single path 把 step、cancellation 和错误位置变成显式控制流；
- 1～16 step 限制本地遍历规模，positive maxDuration 为 context-aware 等待与检查点提供服务端 cooperative budget；
- caller/internal/provider priority 避免把超时来源混成一个 `context deadline exceeded`；
- zero partial result 阻止失败 prefix 被误当成成功 route；
- closed typed input 保持 Lottery ownership，不污染 Participation 或 Governance；
- 不装配、不公开 API，使执行正确性可以在发布/权限之前独立验收。

### 成本与限制

- 每次求值会重新 Validate 至多 128 nodes / 256 edges 的 graph；
- application 需要 graph reader、Clock、fact reader、freshness 与两个执行预算配置；
- v1 所有 decision 使用同一 membership rule，多步业务表达力有限；
- context 只能协作取消，不能强制停止忽略 context 的 provider；Clock 端口还必须维持有界本地实现；
- maxDuration 使用 wall-clock deadline，但不是业务 evaluated-at 或性能 SLO；
- graph identity 仍由调用方显式提供，没有 Activity 决定 active revision；
- target 只是 StrategyID，不证明 Strategy published、不可变或可抽奖；
- path 只在内存返回，不是持久审计记录；
- evaluator 尚未接生产流量，不能声称线上 QPS、P99 或容量收益。

### 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| concrete router 与 graph evaluator 漂移 | 共享私有 branch helper + standard/premium/unknown oracle parity tests |
| canonical edge order被误当优先级 | exact branch lookup；fixture 让 baseline 排在 premium 前仍命中 premium |
| default 吞掉 unknown/provider error | fact 先验证；context/provider error优先；无 catch-all |
| graph reader 返回错 identity 或伪造 aggregate | application identity equality + evaluator入口 `Validate()` |
| 深 graph 消耗过多步骤 | graph depth 16 + configured maxSteps 1～16 + loop hard stop |
| provider 阻塞超过预算 | context-aware reader 接收 child context；Clock 必须有界本地；所有边界返回后优先检查 caller/internal |
| timeout 来源不可区分 | caller -> internal -> provider固定优先级与稳定错误类别 |
| cancellation 返回半条 path | failure始终 zero decision，path只在 terminal后原子形成 |
| 多次读取产生混合版本 | graph/clock/fact各一次，node loop零 I/O |
| 重复 membership node被夸大为多规则 | 文档/QA/面试明确 closed v1 与重评触发器 |
| 顺带执行 Strategy/Award 形成隐式 Draw | terminal只返回 StrategyID；架构负向测试禁止 selector/random依赖 |
| evaluator被误认为已上线 | 不进入 composition、HTTP/UI；运行权限保持不变 |

## 并发、资源与生命周期

- Service 构造后只持有只读 port/config，不保存请求级 current node、path 或 timer；
- child context/timer 每次调用创建并在调用结束时 cancel；
- graph、fact、evaluated-at、visited 与 path 都是调用栈局部值；
- shared service 的并发安全取决于注入 reader/Clock 自身可并发；
- evaluator 不缓存 graph/fact/decision，不使用 global mutable registry；
- path 最多 16 steps，visited 最多 17 nodes；
- graph 入口验证含复制、规范排序与结构检查，当前为 `O(V log V + E log E + V + E)`；单路径 traversal 至多 16 轮；
- `Node` / `OutgoingEdges` 使用 aggregate 的确定访问器，不暴露内部 slice；
- 本节没有 goroutine fan-out、parallel branch、background retry 或异步副作用。

## 撤销与演进

本节没有运行时装配、公开 API、Activity 绑定或新增数据库结构。若 evaluator 设计被推翻，可以停止构造 application service 并删除未被消费者使用的代码；第 28 节既有 Migration 和 immutable graph 数据仍按原 ADR 保留，不能回写历史。

线性演进保持为：

1. 第 27 节：具体会员 route 语义 oracle；
2. 第 28 节：immutable graph 与 strict persistence/restore；
3. 第 29 节：closed typed evaluator、single path、budget、cancellation 和完整 decision；
4. 第 30 节：Activity 生命周期与 approved graph revision 绑定；
5. 第 31～35 节：统一主体、会话、服务端授权、前端投影与越权 E2E；
6. 第 36 节：运营后台只消费受保护的服务端能力。

## 重新评估触发条件

出现以下任一真实证据时，新增 ADR 而不是改义复用本决定：

- 第二种 graph decision rule 及其类型化事实已经实现；
- node 需要独立参数、版本化 operator 或外部依赖；
- 真实需求需要 parallel branch、subgraph、loop、random 或 side effect；
- maxSteps 需要按 rule cost 加权，而不再等于 edge count；
- 不同 Activity 需要不同 step/time budget 与发布时静态检查；
- evaluator graph Validate 或 traversal 成为可测性能瓶颈；
- 需要缓存 compiled plan，并能定义 revision invalidation 与内存上限；
- 需要持久 decision/path、合规审计、重放或隐私保留期；
- Strategy target 升级为 immutable Strategy revision；
- provider 支持历史 as-of 查询或跨事实一致性 token；
- 需要运营草稿、模拟、审批、灰度、发布或回滚；
- 决定采用 DMN/OPA/外部规则平台，并已具备类型、安全、预算与故障域评估。

## 验收证据

第 29 节必须能够证明：

1. depth-1 graph 的 standard/premium target、branch、reason、fact provenance 与 evaluated-at 和第 27 节 concrete router 等价；
2. 两个 branch 指向同一 terminal 时 target 相同但 path branch 不丢失；
3. multi-step、shared successor 与 depth 16 graph 使用确定 single path 并形成连续 steps；
4. canonical edge order 不是执行优先级，premium 不会误选排序在前的 baseline；
5. unknown tier、future/stale/invalid fact 不会命中 baseline；
6. exact graph identity 只读一次，错 identity/坏 graph/error+value 都不读取会员事实；
7. Clock 与会员事实各读取一次，所有 decision node 共享同一 snapshot；
8. graph depth 等于 maxSteps 成功，超过配置在 fact read 前失败，loop 仍有 actual hard stop；
9. maxSteps 0、17 与 maxDuration zero/negative 在构造阶段失败；
10. pre-cancel、graph/clock/fact/evaluator 边界取消都返回 caller error与零 decision；
11. caller deadline、internal maxDuration 与 provider-owned deadline 按固定优先级分类；
12. budget/timeout/bad graph 都不返回 partial path 或 target；
13. Decision `Confirmed()` 拒绝伪造 identity、非连续 path、branch/reason 不匹配和 terminal/target 不一致；
14. Path accessor 防御复制，并发只读求值得到相同结果且通过 race；
15. fuzz 对 graph/fact/time 组合不 panic、不越过 16 steps、不无限循环；
16. architecture guard 拒绝 generic Rule/Engine/registry/DSL，并在指定 runtime/HTTP/Compose/Docker/Web production source 中拒绝五个 evaluator 标识符的直接装配；raw row、Activity、Strategy selection、auth 与其他零变化由类型依赖、章节 diff 和回归审查补证；
17. 第 28 节 Repository/MySQL strict restore、现有 ephemeral API/React 和 Participation 行为保持不变。

## 相关资料

- [Lottery Strategy Routing Graph 基线 v1](../product/lottery-strategy-routing-graph-v1.md)
- [Lottery 会员等级 Strategy 路由基线 v1](../product/membership-strategy-routing-v1.md)
- [ADR-0019：Lottery 规则所有权与评估边界](ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [ADR-0023：会员等级 Strategy 路由边界](ADR-0023-membership-strategy-routing-boundary.md)
- [ADR-0024：Lottery Strategy 路由图持久化](ADR-0024-lottery-strategy-routing-graph-persistence.md)
- [第 27 节 QA](../qa/lessons/lesson-27.md)
- [第 28 节 QA](../qa/lessons/lesson-28.md)
- [Go `context` package](https://pkg.go.dev/context)
