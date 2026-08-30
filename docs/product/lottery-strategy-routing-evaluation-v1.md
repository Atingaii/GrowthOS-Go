# Lottery Strategy Routing Evaluation 基线 v1

- **状态：** 第 29 节已批准实现基线；实现与验收证据尚未形成
- **日期：** 2026-08-30
- **所有者：** Lottery bounded context
- **适用范围：** 显式读取一个不可变 `StrategyRoutingGraph` revision、一次权威会员事实读取、封闭 typed dispatch、确定性单路径遍历、多步 path、步数/时间/取消预算与安全失败语义
- **不适用范围：** graph 发布或 active revision、Activity、公开 API、runtime composition、真实会员 adapter、Strategy 加载与随机选择、Participation 编排、认证授权、UI、持久审计、正式 Draw/Result、库存与权益发放
- **前置事实：** [第 27 节会员路由基线](membership-strategy-routing-v1.md)提供 exact rule/branch/default 语义；[第 28 节路由图基线](lottery-strategy-routing-graph-v1.md)提供已经构造或严格恢复的有界不可变图

## 1. 先给结论

第 28 节已经能把一份 Lottery Strategy 路由拓扑保存成可信的 immutable rooted DAG，但“可信地保存”不等于“已经执行”。第 29 节只补上这一个缺口：

> 给定调用方显式指定的 exact `(GraphID, Revision)`、经领域校验或 Repository 严格恢复的 schema-v1 graph，以及从权威会员来源取得的一份受控事实快照，Lottery 怎样在有限步数和有限时间内确定走过的唯一 path，并返回一个 Strategy target？

本节形成的是 **Lottery 内部受限执行基线**。它不是跨上下文规则平台，也不是线上活动能力。graph 在本节仍然：

- 尚未发布；
- 没有 active/latest 选择语义；
- 没有绑定 Activity；
- 没有装配到 `growth-api` 或长期 Compose runtime；
- 没有通过 API、MCP、Agent 或浏览器暴露；
- 没有调用 `StrategyReader`、`WeightedSelector` 或正式 Draw。

因此，本文件中的“evaluation 成功”只表示：在一次受控内部调用中，执行器对一份明确的合法 graph 和可信会员事实形成了确定 Lottery Route。它不表示用户有资格参与、调用者获得授权、Activity 正在生效、Strategy 已发布、Award 已被选择或最终结果已经落定。

## 2. 为什么现在才有资格实现执行

第 27 节冻结了第一个真实业务反例：

```text
confirmed premium  -> premium_override -> premium Strategy target
confirmed standard -> baseline_default -> baseline Strategy target
```

它证明固定 `continue/reject` 责任链不能表达两个合法成功出口，同时也留下了一个可信语义 oracle：

- 只有 confirmed `premium` 可以选择 `premium_override`；
- 只有 confirmed `standard` 可以选择 `baseline_default`；
- default 不是 unknown、过期、缺失、依赖故障或取消的 fallback；
- route 成功只得到 Strategy identity，不执行随机选择。

第 28 节再冻结了执行器可以信任的结构输入：

- exact `GraphID + Revision + schema_version = 1`；
- 显式 root；
- 封闭的 `decision | strategy_target` node kind；
- 封闭的 rule/branch/default vocabulary；
- 全可达、无环、无悬空、terminal 无出边；
- 128 nodes、256 edges、longest depth 16；
- create 与 restore 使用同一完整领域校验；
- Repository 只按 exact identity 返回一个 immutable aggregate。

如果没有这两层前置证据，执行器就必须一边遍历一边猜 root、修坏数据、解释自由表达式或决定 default，最终会把存储修复、业务语义和运行控制混在一起。现在输入和词汇已经有界，第 29 节才可以只解决 evaluation。

## 3. 本节新增的精确产品能力

第 29 节批准一个 Lottery-owned routing evaluation use case，具备以下能力：

1. 调用方必须提供一个经过领域构造器验证的 exact `StrategyRoutingGraphIdentity`，不能请求 `latest`、`active` 或“任意可用 revision”；
2. application 通过 consumer-owned `StrategyRoutingGraphReader` 恰好读取一次该 identity；
3. reader 返回的 graph 必须再次通过 `Validate()`，zero、伪造或损坏 aggregate 不得进入事实读取；
4. application 使用一次受控 server Clock 形成唯一 logical `evaluated_at`；
5. application 通过已有 consumer-owned `MembershipTierFactReader` 恰好读取一次该 subject 的会员事实；
6. 同一份已经校验的 fact snapshot 在所有 decision node 中复用，不按节点重新访问 provider；
7. 执行器从 graph 的显式 root 开始，只评估实际路径上的 decision；
8. v1 以封闭 typed dispatch 执行唯一批准的会员路由 operator；
9. 每一步必须精确匹配一个实际 outgoing edge，不能使用 slice 第一项、数据库行序或“未命中即 default”；
10. 到达 `strategy_target` 后立即终止并返回该 `StrategyID`；未走分支不执行；
11. 成功决定携带 exact graph identity、事实 provenance、唯一 evaluated-at 与 ordered multi-step path；
12. 步数、child deadline 和 caller cancellation 形成独立运行预算；
13. 任何参数、依赖、事实、配置、预算、取消或内部不变量错误都返回 zero decision 和 zero path。

这项能力第一次让“已验证 graph 可以怎样运行”成为可测试事实，但它仍是未发布、未装配的 domain/application 内核。

## 4. 输入契约与所有权

### 4.1 一次 evaluation 的最小输入

一次内部 evaluation 只接受以下业务输入和服务端配置：

| 输入 | 所有者 | 精确要求 | 不能代表 |
| --- | --- | --- | --- |
| `context.Context` | 上层调用方 | 非 nil；承载 caller cancel/deadline | 已认证会话或授权决定 |
| `StrategyRoutingGraphIdentity` | Lottery | 非零 GraphID + 符合 v1 ASCII grammar 的 exact Revision | Activity、active/latest revision、租户 |
| `MembershipSubjectRef` | Lottery 对外部会员 authority 的查询引用 | 非零 opaque ref | Principal、当前调用者、对象权限 |
| `maxSteps` | 服务端 evaluation 配置 | `1..16`，zero 不表示 unlimited | graph schema depth 或业务 SLO |
| `maxDuration` | 服务端 evaluation 配置 | 正 `time.Duration`；每次调用派生 child deadline | P99、provider SLA 或硬实时抢占 |
| graph reader | Lottery consumer-owned port | 按 exact identity 恰好读取一次合法 aggregate | publisher、latest registry 或裸 SQL rows |
| membership fact reader | Lottery consumer-owned port | 按 subject 恰好读取一次最小事实快照 | Strategy target、角色、资格决定 |
| logical Clock | Lottery application | 在 graph 合法后恰好读取一次 | 每节点计时器或客户端时钟 |

`maxSteps` 与 `maxDuration` 是服务端构造的不可变执行配置，不来自浏览器、graph row、会员 provider 或自由参数 bag。本节没有 runtime composition，因此也不新增环境变量或对外 DTO；测试和未来受信 composition 显式注入它们。

### 4.2 graph identity 必须 exact

本节只允许：

```text
FindByIdentity(GraphID, Revision)
```

明确禁止：

- `FindLatest(GraphID)`；
- `FindActive(ActivityID)`；
- revision 为空时自动选择最大值；
- Repository not-found 后尝试另一 revision；
- 从客户端传入 `published=true`、status 或 fallback graph；
- 用 schema version、Migration version、Strategy ID 或 Activity ID 代替 graph revision。

这是一个重要的产品停止线。第 29 节只证明“明确给我哪份 immutable revision，我能否可信执行”；第 30 节才拥有“哪份 revision 对哪一个 Activity 生效”的发布问题。

### 4.3 graph、Clock 与 fact 都至多一次

一次有效调用的依赖上限固定为：

```text
graph read <= 1
logical Clock read <= 1
membership fact read <= 1
```

即使实际 path 有 16 个 decision，也不能进行 16 次会员 provider 读取，不能让每个 node 单独读取当前时间，更不能在 branch 未命中时重新拉取事实。这样可以保证：

- 所有节点观察同一 fact revision；
- future/freshness 使用同一个 logical as-of；
- provider 尾延迟不会按路径长度线性放大；
- 同一决定不会混用多个外部时刻；
- 路径确定性不依赖节点执行速度。

graph 在事实读取前加载并复核。若 identity 不存在、Repository 失败或 aggregate 无效，Clock 与会员 reader 都必须零调用，避免为一份不可执行配置读取用户派生事实。

## 5. v1 使用封闭 typed dispatch

### 5.1 唯一批准的 operator

schema v1 的每个 decision 只允许：

```text
lottery.membership_tier.route_strategy
```

执行器不接收 `map[string]any`、JSON expression、script、反射函数名或动态插件。它按稳定 rule code 进行 exhaustive closed dispatch，并把已经校验的 `MembershipTierFactSnapshot` 交给 typed membership branch evaluator：

| fact tier | selected branch | 稳定语义 |
| --- | --- | --- |
| confirmed `premium` | `premium_override` | 选择显式 premium override edge |
| confirmed `standard` | `baseline_default` | 选择批准的 baseline edge |
| zero / unknown / unsupported | 无 | 无可信 route，失败关闭 |

typed evaluator 只形成 branch/reason，不读取 graph、Repository、Clock、context、Strategy 或随机源。application 再用 selected branch 在当前 decision 的两条已验证 outgoing edges 中寻找 exact match。

第 27 节 concrete router 与第 29 节 graph evaluator 应共享或对照同一 typed tier-to-branch 语义，并用独立 fixture 做等价验收。不能为了“复用”把 Participation gate、Authorization、库存或任意 operator 塞进同一个接口。

### 5.2 为什么本节不建 operator registry

当前只有一个真实 operator。此时建立插件 registry 会提前引入：

- 任意 operator 注册顺序；
- 同 code 重复注册策略；
- 动态输入类型检查；
- operator 生命周期与依赖注入；
- 跨上下文 rule ownership；
- 缺失插件的部署兼容矩阵。

这些问题没有第二个真实 rule 证明。本节采用 closed switch 或等价的封闭 typed dispatch。未来第二个 Lottery-owned operator 出现后，再由两个实际消费者反推最小 registry；不能因课程标题包含“引擎”就预制终态平台。

### 5.3 unknown operator 的可达性

正常 v1 graph 在 Create/Restore 时已经拒绝未知 rule code，因此 unknown operator 不应成为普通业务分支。执行层仍必须保留 exhaustive default，以处理：

- zero/同包伪造 aggregate；
- graph schema 与 evaluator build 不兼容；
- 未来代码错误地绕过 Restore；
- typed dispatcher 漏配一个领域已经批准的 code。

这些情况统一失败关闭，不能走 `baseline_default`，不能把未知 code 当成 no-op，也不能把完整 rule code 或 graph 内容泄露进公共错误文本。

## 6. 确定性遍历协议

### 6.1 固定执行顺序

一次 evaluation 必须按以下顺序进行：

1. 校验 context、exact graph identity、subject ref、dependencies、`maxSteps` 与 `maxDuration`；
2. 检查 caller context；pre-cancel 时 graph/Clock/fact 均零调用；
3. 用正 `maxDuration` 从 caller context 派生 child context，较早 deadline 自然生效；
4. 使用 child context 按 exact identity 读取 graph 一次；
5. graph reader 返回后先按错误优先级检查 caller/internal deadline，再处理 reader error；
6. 对返回 graph 执行 `Validate()`；invalid graph 时停止，Clock/fact 零调用；
7. 检查 child context，调用 logical Clock 一次并规范为 UTC；zero Clock 结果失败；
8. 检查 child context，使用同一 child context 读取会员 fact 一次；
9. reader 返回后再次应用 caller/internal/provider priority；error 与 fact 同时返回时 error 胜出；
10. 校验 fact 的 subject、tier、source、revision、future 与 freshness；
11. 从 graph root 开始，在每个 decision 前检查 context 与 step budget；
12. closed dispatch 形成 exact branch，只取匹配该 branch 的一条 outgoing edge；
13. 追加一个实际 path step、递增 step count，再进入 destination；
14. destination 是 decision 时继续，destination 是 `strategy_target` 时形成最终 target；
15. 返回前最后检查 caller/internal deadline，并验证完整 decision/path 一致性；
16. 只有完整决定通过确认后才返回 success。

任何一步失败都立即短路。未选 branch、Strategy Repository、selector、random source、Participation、库存和发奖端口必须保持零调用。

### 6.2 branch 匹配不是 fallback

执行器不得：

- 取 outgoing slice 第一条边；
- 依赖数据库返回顺序；
- 找不到 premium edge 时改走 default；
- 将 `is_default=true` 解释为 unknown/error fallback；
- 同时执行两条边后再选一个结果；
- 在共享后继处重复执行已经不在实际 path 上的父分支；
- 到达 terminal 后继续扫描其他 node。

graph invariant 已保证每个 decision 恰有两条 approved edge；执行器仍应按 selected branch 做 exact lookup，并把“没有唯一匹配边”视为内部 graph/evaluator invariant failure。

### 6.3 DAG 不等于并行执行

rooted DAG 允许多个父节点共享后继，但一次 membership route 只选择一条出边，因此一次 evaluation 始终形成一条 ordered path：

```text
root decision
  -> selected edge
  -> optional next decision
  -> ...
  -> one strategy_target terminal
```

本节不并行执行多个 branch，不合并多个 operator result，也没有“全部命中”“优先级最高”或冲突消解策略。共享后继只表示不同可能路径可汇聚到同一节点，不表示本次请求走过所有父路径。

### 6.4 当前多步不等于多条件引擎

schema v1 的所有 decision 都使用同一 membership tier operator，并复用同一 fact snapshot。多步 graph 因而首先证明 traversal、共享后继、path、预算和终止语义，而不是证明系统已经支持多个业务条件。

这一限制必须诚实保留。新增地域、渠道、时间窗、风险、额度或复合 operator 时，应先登记事实所有权和新产品需求，再升级 schema/ADR；不能把这些值偷放进通用 context map。

## 7. 运行预算

### 7.1 静态 graph 预算与运行预算分离

第 28 节的预算回答“这份配置是否能被安全构造和恢复”：

| 静态预算 | 上限 |
| --- | ---: |
| graph nodes | 128 |
| graph edges | 256 |
| longest root-to-terminal depth | 16 edges |

第 29 节的预算回答“当前 evaluation service 是否承诺完整执行这份 graph，以及单次调用最多允许多少工作”：

| 运行预算 | v1 规则 |
| --- | --- |
| `maxSteps` | 服务端配置，必须为 `1..16` |
| `maxDuration` | 服务端配置，必须为正 duration |
| caller cancellation/deadline | 始终保留并与 child deadline 取较早者 |

静态 depth 16 合法不表示每个 service 都必须接纳 16 层 graph；运行配置可以更小。读取后若 `graph.Depth() > maxSteps`，本 service 在读取 Clock 与会员事实前拒绝该 revision。反过来，运行 `maxSteps=16` 也不能让 depth 17 graph 绕过 Restore。

### 7.2 step 的精确定义

一个 step 精确表示：

> 在一个 decision node 上成功求得 branch，并沿唯一匹配 edge 移动到 destination。

因此：

- root 本身不预先计 step；
- 每条实际走过的 decision edge 计 1；
- 到达 terminal 不额外加 1；
- 未选 edge 不计数；
- 读取 graph、Clock、fact 不计 step；graph/fact reader 接收 child context，Clock `Now()` 不接收 context，必须保持为有界本地调用，并在返回后立刻检查 budget/cancel；
- 直接 `decision -> terminal` 的成功 path 长度和 step count 都是 1；
- 16 条 edge 的 path 在 `maxSteps=16` 下合法；
- 当 step count 已等于 maxSteps 且当前节点仍是 decision 时，在 dispatch 前返回 budget exceeded，不执行第 `maxSteps+1` 个 operator。

Service 在 graph 读取并复核后先要求 `graph.Depth() <= maxSteps`；遍历 loop 在追加 edge 前再按实际 step count 执行同一 hard stop。采用最坏路径准入，是为了避免同一个 graph revision 在相同 service 配置下出现“standard 短路径成功、premium 深路径运行到一半失败”这类事实相关可用性差异。实际 step guard 仍不可删除，它防止未来 validator 或调用链回归绕过准入。

### 7.3 positive maxDuration 与 child deadline

`maxDuration <= 0` 是配置错误，必须在依赖调用前失败。一次合法 evaluation 使用：

```text
internalDeadline = monotonicNow + maxDuration
if caller deadline <= internalDeadline:
  evaluationCtx = context.WithCancel(callerCtx)
else:
  evaluationCtx = context.WithDeadlineCause(callerCtx, internalDeadline, privateInternalCause)
```

caller 已有更早或相同 deadline 时，显式让 caller 拥有 deadline；只有内部 deadline 更早时才安装私有 cause。这样不依赖两个相同时间 timer 的调度先后，也能把内部超时映射成不匹配 `context.DeadlineExceeded` 的稳定 budget class。主动调用 cleanup cancel 不得伪装成内部 timeout。child context 必须传给 graph/fact reader，并在每个 decision 前和成功返回前检查。

这个 time budget 是 cooperative deadline，不是硬实时抢占：

- Repository/provider 必须遵守 context 才能及时停止 I/O；
- `MembershipRoutingClock.Now()` 没有 context 参数，当前契约要求它是有界本地读取；若它阻塞，deadline 只能在它返回后的检查点被观察，不能形成 wall-clock 硬上界；
- 一个已经开始的同步 typed operator 不能被 Go 强行中断；
- v1 operator 是常量规模纯计算；
- 最多 16 steps 把不可抢占本地段限制在严格小规模内。

`maxDuration` 不是 P99 或可用性 SLO。第 29 节没有 runtime、真实 provider、生产流量或 Activity，因此不能把单元测试中的 timeout 值写成生产性能承诺。

## 8. caller、内部 deadline 与 provider error 的优先级

### 8.1 为什么必须显式排序

依赖返回时可能同时观察到多个事实：caller 刚取消、内部 child deadline 到期、provider 返回自己的 timeout/error，甚至 provider 同时返回 value 和 error。若没有固定优先级，同一次故障会随 goroutine 调度被分类成不同结果，监控和调用方也无法判断责任边界。

本节冻结以下优先级：

| 优先级 | 条件 | 对外语义 | value/path |
| ---: | --- | --- | --- |
| 1 | `callerCtx.Err() != nil` | 原样 caller cancellation/deadline | 全部丢弃，zero decision |
| 2 | caller 仍存活且 `evaluationCtx.Err() != nil` | 内部 evaluation time budget exceeded | 全部丢弃，zero decision |
| 3 | 两个 context 均存活且 dependency 返回 error | graph/fact provider 的稳定 not-found/unavailable/invalid/failure 分类 | 同时返回的 value 丢弃，zero decision |
| 4 | context 存活、无 error、value 非法 | graph/fact contract invalid | zero decision |
| 5 | context 存活、value 完整合法 | 继续执行 | 尚未到 terminal 时不返回决定 |

这个顺序在 graph read、fact read、每个 decision 前和最终返回前一致应用。

### 8.2 provider timeout 不是 caller cancellation

如果 provider 返回 `context.DeadlineExceeded`，但 caller context 和 evaluation child context 都仍存活，它表示 provider 自己的 operation budget、transport 或依赖失败。它不得伪装成 caller cancel，也不得自动重试或选择 default。

相反，如果 provider 返回普通错误的同时 caller 已取消，caller error 优先。若 caller 仍存活但内部 child deadline 已到期，内部 budget error 优先于 provider error。

### 8.3 error 与 value 同时返回

graph reader 或 fact reader 同时返回非零 value 与 error 时：

- error 胜出；
- value 完全不可观察；
- 不调用 `Validate()` 后尝试继续；
- 不从 value 猜 target、branch 或 provenance；
- 不保留半条 path。

这与第 27 节 fact reader 边界一致，也避免“依赖明确说失败，但系统仍使用可能过期的 payload”。

## 9. 输出、path 与 zero-decision 协议

### 9.1 成功决定的最小信息

成功时形成一个 Lottery-owned immutable routing decision，至少能够回答：

| 字段 | 作用 | 披露边界 |
| --- | --- | --- |
| exact GraphID/Revision | 标识本次执行的不可变 topology | 不作普通 metric label，不等于 Activity version |
| schema version | 说明 evaluator 使用的解释协议 | 当前必须为 1 |
| final StrategyID | 后序明确知道 route target | 不等于 Strategy version/已发布/已加载 |
| fact source/revision | 说明会员事实 provenance | 不含原始 payload，不作 metric label |
| evaluated-at | 解释 future/freshness 使用的唯一逻辑时刻 | 使用 canonical UTC，不接收客户端时间 |
| ordered path | 证明实际走过哪些 decision/edge | 只保存最小步骤，不保存未走分支或完整 graph |

每个 path step 至少包含：

```text
from_node_id
rule_code
selected_branch
to_node_id
```

path 顺序就是实际执行顺序。调用方取得 defensive copy，修改返回 slice 不得改写内部决定。即使不同 branch 最终汇聚到同一个 target，path 中的 branch 身份也不能丢失。

decision 可以沿用会员路由的稳定最终 reason，但 intermediate step 不得用自然语言冒充另一条业务规则；v1 的机器解释以 exact rule/branch/path 为准。

### 9.2 明确不进入决定的信息

本节 decision/path 不保存：

- MembershipSubjectRef；
- 原始 tier/provider payload；
- 姓名、手机号、订单、等级成长值或画像；
- caller session、Cookie、token、Principal、role、permission；
- 未走分支或完整 graph 副本；
- SQL、endpoint、driver/provider 原始错误文本；
- elapsed duration 作为业务事实；
- Strategy/Award 内容、随机 ticket 或 selector source；
- Activity、Participation、库存、Draw 或 Benefit 状态。

需要性能诊断时可以在受控观测层记录低基数 outcome/error class 与聚合耗时，但不能把高基数 identity 或会员派生信息变成普通 label。

### 9.3 zero decision 是所有失败路径的唯一结果

以下任何情况都必须返回 zero decision 和空 path：

- invalid argument/config/dependency；
- pre/mid/post caller cancellation；
- internal duration budget exceeded；
- runtime step budget exceeded；
- graph not found、storage failure、invalid aggregate；
- Clock zero/invalid；
- fact not found、unavailable、read failure；
- fact subject mismatch、unknown/unsupported、future、stale、corrupt；
- unknown/missing dispatcher；
- selected branch 没有唯一 matching edge；
- destination missing/kind invalid；
- path/terminal/target consistency failure。

“zero decision”不表示业务拒绝、standard route、`no_reward` 或 authorization denied；它只表示没有形成可信 Lottery Route。

## 10. 错误分类与低披露

内部 application 至少需要区分：

- invalid argument；
- service not configured；
- graph not found；
- graph read/storage failure；
- stored/returned graph invalid；
- logical Clock invalid；
- membership fact not found；
- membership fact unavailable/read failure；
- membership fact invalid/stale；
- operator/dispatch unsupported；
- step/time budget exceeded；
- internal evaluation invariant invalid；
- caller cancellation/deadline。

这些类别不能相互降级：

- graph not found 不是 baseline route；
- provider unavailable 不是会员 `standard`；
- invalid graph 不是 Activity 未发布；
- budget exceeded 不是业务拒绝；
- route failure 不是 authorization denial；
- route success 也不是 eligibility success。

公开 `Error()` 只渲染稳定、低披露 class，不包含 GraphID/revision、NodeID、subject、tier、provider endpoint、SQL、完整 topology 或 raw payload。受控诊断 cause 是否保留以及采用 `Cause()` 还是 `Unwrap()`，应与现有 Repository/fact error 边界一致并由第 29 节 ADR 明确；产品基线只要求原始错误不得靠字符串解析或进入用户文案。

## 11. 验收矩阵

### 11.1 单节点语义 oracle

- confirmed premium 在 single-decision graph 上选择 `premium_override` 和 premium target；
- confirmed standard 选择 `baseline_default` 和 baseline target；
- branch、target、fact provenance、evaluated-at 与第 27 节 concrete router fixture 等价；
- unknown/unsupported tier 不能命中 default；
- 两 branch 指向同一 terminal 时，premium/standard path 仍保留不同 branch；
- success 不调用 Strategy reader、selector 或 random source。

### 11.2 多步与 DAG

- 两步及多步 graph 按 root 到 terminal 返回 ordered path；
- shared successor 不被当 cycle，也不执行未选父路径；
- 两条 branch 汇聚到同一 decision/terminal 时只执行本次实际 path；
- 输入 node/edge canonical order 不改变 path；
- 相同 graph/fact/evaluated-at/budget 重复求值得到相同 target/path；
- terminal 到达后立即停止，不扫描其他 node；
- path step 的 from/to 与 graph 实际 edge 一致；
- 最后 node 的 StrategyID 与 decision target 一致。

### 11.3 exact identity 与调用次数

- zero/bad graph identity 在依赖前失败；
- reader 只收到 exact GraphID/Revision，不存在 latest fallback；
- graph not-found 不尝试另一 revision，Clock/fact 零调用；
- graph error 与 value 同时返回时 error 胜出，Clock/fact 零调用；
- graph 返回 zero/invalid aggregate 时 fact 零调用；
- 任意合法 path 中 graph reader、Clock、fact reader 都恰好一次；
- 16-step path 仍只读取一次 fact；
- fact error 与 value 同时返回时 error 胜出；
- fact subject mismatch/future/stale/invalid 在 traversal 前失败。

### 11.4 step/depth 边界

- `maxSteps=0` 和 `maxSteps=17` 在依赖前拒绝；
- `maxSteps=1` 允许一条 edge 直达 terminal；
- 实际 path 长度恰好等于 maxSteps 时成功；
- 仍处于 decision 且已消耗 maxSteps 时，在下一次 dispatch 前失败；
- depth 16 graph + `maxSteps=16` 的 16-edge path 成功；
- depth 17 graph 在 Restore/Validate 失败，不能靠运行 budget 接受；
- graph longest depth 大于 maxSteps 时在 Clock/fact read 前拒绝，不能只让碰巧较短的事实路径成功；
- step exceeded 返回 zero decision/path，不保留前 N 步公共结果。

### 11.5 duration 与取消优先级

- `maxDuration <= 0` 在依赖前拒绝；
- pre-cancel 时 graph/Clock/fact 零调用并原样返回 caller error；
- caller deadline 早于 maxDuration 时 caller deadline 优先；
- parent 存活、internal child deadline 到期时返回稳定 budget-exceeded；
- graph/fact reader 返回后 caller cancel 优先于 internal/provider error；
- caller 存活、internal deadline 到期时 internal budget 优先于 provider error；
- 两个 context 均存活时 provider timeout 保持 provider failure；
- Clock 后、fact 后、每个 decision 前和成功返回前的取消都产生 zero decision/path；
- timeout/cancel 不自动 retry graph/fact，不改走 default。

### 11.6 immutability、并发与安全

- 调用方修改返回 path slice 不影响 decision；
- decision 的 graph identity、schema、path、terminal 与 target 不能形成不一致 confirmed 状态；
- domain 同输入 64-worker 结果可重复；application 交错不同 subject/identity/tier/target 的 64 个请求时不混淆被测 decision/path 与按 key 计数；
- race test 无数据竞争；
- fuzz 对 identity、budget、fact 与合法/伪造 graph 输入不 panic、不死循环、不越界；
- unknown operator/branch/invariant mismatch 失败关闭；
- error text 不泄露 subject、tier、GraphID/revision、NodeID、SQL、endpoint 或 raw cause；
- 普通 metric label 不包含 GraphID/revision/NodeID/StrategyID/fact revision/subject。

### 11.7 架构停止线

- 不新增 generic `Rule` / `RuleEngine` / registry / DSL / `map[string]any`；
- Lottery domain/application 不导入 Gin、SQL/sqlx、Redis、Participation、Governance、Marketing 或前端；
- 不新增 Migration、表、Redis key、RabbitMQ event 或 PG projection；
- 不修改长期 MySQL runtime grants，不把 graph Repository 装配进 `growth-api`；
- 不新增 HTTP route、DTO、runtime config、Compose service/network/secret 或 React 页面；
- 不新增 Activity/status/published/active/latest 语义；
- 不调用 Strategy load/selector/random，不形成 Draw/Result；
- 不实现 session、Principal、RBAC/ABAC、tenant/data scope、前端权限或浏览器 E2E；
- 第 27 节 concrete router 与第 28 节 graph persistence 行为保持回归通过。

这些测试只证明内部 evaluation contract，不证明真实会员系统、线上活动、业务 QPS/P99、生产容量、安全渗透或用户端闭环。

## 12. 威胁、隐私、性能与可观测边界

| 风险面 | 第 29 节控制 | 尚未解决 |
| --- | --- | --- |
| 客户端伪造 graph/tier/branch | 无公开 route；exact identity 与事实来自受控调用/reader；server-side dispatch | 真实 session 与 API authorization |
| 隐式选择错误 revision | 只允许 exact GraphID/Revision | Activity publisher/active binding |
| 坏 graph 进入执行 | reader strict Restore + application `Validate()` | 特权旁路写入的运维审计 |
| graph bomb/无限循环 | 128/256/depth16 + actual maxSteps 1..16 | 更大 schema 的兼容与容量 |
| provider N+1/事实漂移 | 一次 fact + 一次 Clock，全 path 复用 | 跨 authority 原子 snapshot |
| default 掩盖 unknown/error | typed tier validation + exact edge match | 新等级发布兼容流程 |
| dependency 无界阻塞 | positive maxDuration child context | provider 是否合作取消、熔断与真实 SLA |
| partial path 被误用 | 所有失败 zero decision/path | 持久审计与授权查询 |
| trace 泄露会员派生信息 | 最小 path，无 subject/payload，高基数禁止普通 label | retention、脱敏、对象级访问 |
| success 被冒充正式抽奖 | 不加载 Strategy/selector，不创建 Draw | Activity/Participation/库存/幂等主链 |

当前 NFR 中“抽奖决策 P99 ≤ 150ms”仍是未来候选目标，不是第 29 节实测。`maxDuration` 是单次内部调用的 cooperative safety budget，不等于 SLO；单测、benchmark 或本地 race 也不能冒充生产吞吐与可用性证据。

## 13. 本节明确不实现什么

### 13.1 发布与 Activity

- draft/approve/publish/retire/rollback；
- active/latest revision；
- ActivityID、Activity lifecycle、时间窗或渠道；
- Activity -> graph revision / Strategy version binding；
- 发布并发、审批、灰度和历史 request 解释。

### 13.2 在线业务链

- 真实 membership provider adapter；
- session/Principal 到 MembershipSubjectRef 的可信映射；
- Participation 新用户/风险链组合；
- Strategy Repository load 与 cache；
- `WeightedSelector` / random source；
- Award availability、库存预占、正式 Draw/Result、幂等与 Benefit。

### 13.3 通用规则平台

- 跨上下文 `common/rules`；
- 任意 operator registry；
- expression/JSON DSL/DMN/OPA/script/plugin；
- 动态 priority、并行 branch、循环或 retry；
- 用户可编辑规则设计器、模拟器或批量导入。

### 13.4 Runtime 与访问控制

- `cmd/growth-api` composition；
- 长期 graph 表权限；
- HTTP/MCP/Agent route 与公开错误映射；
- React 导航、页面、按钮或可视化；
- Identity、Principal、role、permission、resource、action、scope；
- 服务端 RBAC、前端能力投影、越权或浏览器 E2E。

## 14. 与第 30 节的边界

第 29 节回答：

> 已经明确给出一份 validated immutable graph revision 时，Lottery 怎样确定执行并形成可信 path？

第 30 节才回答：

> 哪份 graph revision 与哪份 Strategy 配置在什么 Activity 生命周期和版本中生效？

因此第 30 节负责：

- Activity aggregate 与生命周期；
- draft/approve/publish/retire/rollback 的真实状态边界；
- Activity version 对 exact graph revision 与 Strategy 配置的引用；
- active binding、历史解释、发布并发和替换语义；
- Activity 发布态/时间窗门控。

第 29 节不得为了构造调用样例把 graph 称为 published，也不得在 graph header 加 status/ActivityID。内部测试显式传入 identity，只证明 evaluator input 可定位，不证明该 identity 对任何真实 Activity 生效。

## 15. 与第 31～35 节的边界

访问控制与业务路由保持正交：

| 章节 | 后续新增问题 | 第 29 节不能预制 |
| --- | --- | --- |
| 31 公共访问控制模型与威胁边界 | Principal、resource、action、scope、默认拒绝与披露策略 | `isAdmin`、owner role、graph header permission |
| 32 真实会话认证 | credential/session 到可信 Principal | 把 MembershipSubjectRef 或 tier 当身份 |
| 33 服务端 RBAC 强制 | graph/Activity 动作的服务端授权与拒绝审计 | 用 route/fact error 伪装授权拒绝 |
| 34 前端权限投影 | 服务端 capability 到导航/路由/操作体验 | 因页面隐藏就认为 API 安全 |
| 35 越权与浏览器 E2E | direct API、对象范围、跨角色/会话与浏览器验收 | 在无 API/session 时预写成功证据 |

route evaluation 成功永远不是 access allow；授权拒绝也不能改写成 fact invalid、standard default 或用户不合格。第 29 节的 path 含会员派生 branch，未来对外查询必须先经过第 31～35 节的数据范围和披露决策。

## 16. 当前可以与不能宣称的能力

第 29 节实现并验收后，可以准确表述：

> 为 Lottery immutable Strategy routing graph 实现封闭 typed evaluator：按 exact GraphID/Revision 严格读取并复核 graph，在一次服务端 evaluated-at 下只读取一次权威会员事实，从 root 确定遍历到唯一 Strategy terminal，输出 defensive-copy 多步 path，并以 maxSteps 1..16、positive maxDuration child deadline、caller/internal/provider 错误优先级和 zero-decision 协议限制失败与资源消耗。

不能表述：

- 已实现跨上下文通用规则引擎、DSL、DMN 或插件平台；
- graph 已发布、active 或绑定 Activity；
- graph evaluator 已装配进长期 runtime/API；
- 已接入真实会员系统或可信 Principal；
- route success 表示用户有资格或有权限；
- 已加载 Strategy、调用 selector 或形成正式 Draw；
- path 已持久化、可供普通运营查询或形成安全审计；
- 已实现登录、RBAC、租户/对象级数据范围或前端权限；
- 单元/race/fuzz/本地测试证明生产 P99、容量或可用性。

## 17. 风险账本与重新决策触发器

| 未决风险 | 当前接受理由 | 重新决策触发器 |
| --- | --- | --- |
| 只有一个 typed operator | 当前只有会员路由拥有完整事实/branch/schema 证据 | 第二个 Lottery-owned rule 进入真实 graph |
| 多步仍复用同一 tier fact | 本节先证明 traversal/path/budget | 新地域、渠道、时间窗或组合事实出现 |
| graph identity 由内部调用显式给出 | 尚无 Activity publisher | 第 30 节 active binding |
| target 只到 StrategyID | 当前 Strategy 无业务 version/发布语义 | Activity 需要引用 immutable Strategy revision |
| maxDuration 尚无 runtime 默认值 | 本节未装配，不虚构生产参数 | composition/API 出现并拥有上层总预算 |
| child timeout 是 cooperative | operator O(1)、path <=16 | 出现远程/长计算 operator |
| path 只在内存 | 当前无审计消费者和权限 | 需要历史回放、合规审计或运营查询 |
| branch 是会员派生信息 | 当前不公开、不持久化 | API/UI/跨租户查询出现 |
| graph Repository 长期无权限 | 本节只交付内部内核 | runtime 真实消费 graph，需单独 grant/acceptance |

以下变化必须新增或替代架构决定，不能静默扩大 v1：

- 新 node kind、rule code、branch vocabulary 或 schema v2；
- generic operator registry、外部 DMN/OPA/规则平台；
- 多事实并行读取、历史 as-of 或跨 authority snapshot；
- retry、fallback、循环、并行 branch 或多命中策略；
- path 持久化、签名、跨环境 promotion 或审计保留；
- 多租户 graph、对象级 scope 或对外 evaluate API；
- 真实 graph evaluation 成为可测性能瓶颈。

## 18. 相关基线

- [Lottery 会员等级 Strategy 路由基线 v1](membership-strategy-routing-v1.md)
- [Lottery Strategy Routing Graph 基线 v1](lottery-strategy-routing-graph-v1.md)
- [Lottery 业务规则需求基线 v1](lottery-rule-requirements-v1.md)
- [Participation 前置资格链基线 v1](participation-prerequisite-chain-v1.md)
- [非功能需求 v1](non-functional-requirements-v1.md)
