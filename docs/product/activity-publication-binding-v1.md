# Activity 发布与 Lottery 配置精确绑定基线 v1

- **状态：** 第 30 节已实现并完成工程验收；新增内核保持未装配，公开 API/UI/认证授权仍不在本节范围
- **日期：** 2026-08-30
- **所有者：** Marketing bounded context；Lottery 对被引用的 Strategy/graph 配置继续拥有事实
- **适用范围：** Activity 最小生命周期、不可变发布版本、Strategy exact revision、Activity 对 exact graph/Strategy revision 闭合集的绑定、并发发布、回滚、退役与 `[start,end)` 时间门控
- **不适用范围：** 公开 API、React/运营后台、runtime composition、真实 Governance/IAM adapter、认证授权、租户/对象数据范围、Participation 编排、会员生产 adapter、随机选择、正式 Draw/Result、库存、发奖、MQ、灰度与多 active version
- **前置事实：** [Lottery 规则所有权基线](lottery-rule-requirements-v1.md)、[Strategy Routing Graph 基线](lottery-strategy-routing-graph-v1.md)和[求值基线](lottery-strategy-routing-evaluation-v1.md)
- **规范性决定：** [ADR-0026：Activity 发布版本与 Lottery 精确配置绑定](../decisions/ADR-0026-activity-publication-binding.md)

## 1. 先给结论

第 29 节已经能回答：

> 调用方明确给出一份合法、不可变的 Lottery routing graph revision 时，怎样在有限预算内确定执行并形成一条可信 Strategy route？

它仍然不能回答：

> 哪个 Activity 在哪个生命周期和时间窗口内，应当让新的参与请求使用哪份 graph revision，以及 graph 中每个 Strategy target 对应哪份不可变 Strategy 配置？

第 30 节只补上这一个发布缺口。新增最小 Marketing-owned `Activity` 与 immutable publication version；每份 publication 精确绑定：

1. 一个 exact `(GraphID, GraphRevision)`；
2. graph 中每个唯一 terminal `StrategyID` 对应的 exact `(StrategyID, StrategyRevision)`；
3. 一个服务端解释的 UTC 半开时间窗 `[starts_at, ends_at)`；
4. 发布类别、回滚来源和可信审批 evidence reference；
5. Activity 自己的不可变 numeric publication version。

这使同一 Lottery 配置可以被多个 Activity 复用，也使同一 Activity 能通过追加版本切换配置，而不把 Marketing 生命周期塞进 Strategy 或 graph。

本节仍是**未装配的内部领域、应用与持久化基线**。Activity gate allow 只表示 Marketing 已确认“当前 Activity publication 允许进入下一业务阶段”，不表示：

- 当前调用者已经认证或获权；
- 参与主体满足新用户、风险、次数或其他 Participation 资格；
- 会员 fact 已读取或 routing graph 已执行；
- Strategy 已加载、Award 已选择或随机票据已消费；
- 正式 Draw/Result 已持久化；
- 库存可用或权益已经发放。

## 2. 为什么 Strategy 不等于 Activity

### 2.1 两者回答的问题不同

| 对象 | 核心问题 | 事实所有者 | 主要变化原因 |
| --- | --- | --- | --- |
| Activity | 为什么做、何时开放、何时停止、哪个发布版本对新参与生效 | Marketing | 运营排期、发布、止损、退役与版本切换 |
| Strategy | 在一个确定候选集合中有哪些 Award、相对权重和 outcome | Lottery | 抽奖候选与权重配置变化 |
| Strategy Routing Graph | 已确认的 Lottery 事实应路由到哪个 Strategy target | Lottery | rule/branch/target 拓扑变化 |
| Authorization | 某 Principal 能否对 Activity/Strategy 执行动作 | Governance | 身份、角色、权限、资源、动作和数据范围变化 |

如果把 Activity 状态和时间窗放入 Strategy：

- 同一 Strategy 不能安全地被两个不同排期的 Activity 复用；
- Activity 回滚会被迫修改 Lottery 配置；
- Strategy cache 会混入 Marketing 的动态状态；
- Lottery selector 会被迫读取时钟、审批和生命周期；
- Activity 的访问控制、退役与审计会污染 Award/Weight 聚合。

如果把 ActivityID/status 直接放入 graph header，问题相同：graph revision 将同时拥有 Lottery 拓扑和 Marketing 发布生命周期，无法独立复用、严格恢复或解释历史。

因此本节采用**引用而非吞并**：Marketing publication 保存 Lottery 配置的精确身份；Lottery 内容仍由 Lottery 构造、持久化和验证。

### 2.2 多对多复用是必须证明的边界

本节至少要允许：

```text
Activity A publication v1 ─┐
                           ├─> graph G:r7 + Strategy S1:r3 / S2:r5
Activity B publication v4 ─┘

Activity A publication v2 ───> graph G:r8 + Strategy S1:r4 / S2:r5
```

这说明：

- 一个配置快照可以被多个 Activity publication 引用；
- 一个 Activity 的新版本可以只替换 graph、某个 Strategy revision、时间窗或它们的组合；
- 旧 Activity publication 继续指向旧 exact refs，不随当前版本变化；
- Strategy 不反向持有 Activity 列表或 Activity lifecycle。

## 3. 当前缺口为什么不能用 StrategyID 掩盖

当前 `Strategy` 只有 `StrategyID`、name、Awards 和 derived total weight。现有 Repository 只支持 create 和 `FindByID`；现有根/子表的 `updated_at` 只是行级诊断字段，不是聚合版本。

graph v1 的 terminal 也只保存 `StrategyID`。该 FK 可以证明 target 行在 graph 创建时存在，但不能证明：

- Activity 发布时使用的是哪份 Award/Weight 快照；
- 以后 Strategy 内容变化时，历史 Activity 应怎样恢复；
- 两个发布版本引用的 Strategy 内容是否相同；
- 当前按 ID 读取的内容是否仍是当时批准的内容。

本节明确拒绝三种伪精确方案：

1. **只保存 StrategyID：** 无法定位历史内容；
2. **把 `updated_at` 当 revision：** 子 Award 更新不一定推进根时间，时间戳也不是受控业务版本；
3. **只保存 content hash，不保存内容：** 可以发现漂移，却无法在原内容被替换后恢复完整历史快照。

因此新增 Lottery-owned create-only `StrategySnapshot`。其 exact identity 是：

```text
(StrategyID, StrategyRevision)
```

每个 identity 在权威表中绑定一份完整、不可变且可严格恢复的 Strategy + Awards 内容。revision 是 registry-bound 业务身份，不等于缓存 codec version、schema version、Migration version、Git commit 或应用版本。

## 4. 统一语言与版本台账

| 术语 | 唯一含义 | 禁止混用 |
| --- | --- | --- |
| `ActivityID` | 一场 Marketing Activity 的稳定身份 | StrategyID、公开短链、Principal |
| `ActivityStateVersion` | Activity header 的非负 CAS token：draft 为 0，published 等于 active version，retired 等于保留的 active version + 1 | publication version 的业务含义、行 `updated_at` |
| `ActivityVersion` | 一份不可变、单调追加的 Activity publication identity | graph revision、Strategy revision |
| `StrategyRevision` | 某 StrategyID 下完整 Strategy/Awards 快照的 create-only revision | Strategy cache payload version |
| `GraphRevision` | 某 GraphID 下不可变 routing topology revision | ActivityVersion、schema version |
| schema version | 决定某类持久内容怎样解析 | 业务内容 revision |
| approval evidence | Governance 对 exact publication candidate 的可信审批结果引用 | caller role、前端勾选框 |
| published | Activity 已选择一份 immutable publication 作为当前版本 | 当前时刻一定在窗口内 |
| scheduled/active/ended | 由 publication window 与一次受控时刻派生的时间相位 | 持久 lifecycle status |
| retired | Activity 永久拒绝新的参与解析，同时保留最后 active version | 删除历史、撤销已完成事实 |
| rollback | 追加一个更高 ActivityVersion，复制旧 exact publication 内容并重新成为 active | UPDATE 旧版本、active 指针倒退 |

这些标识必须按类型和业务语义分离。`published` 状态虽然要求 StateVersion 与 active ActivityVersion 数值相等，也不能因此把 CAS token 当 publication identity 使用：

```text
ActivityStateVersion          = header concurrency token
ActivityVersion               = immutable publication identity
StrategyRevision              = exact Strategy snapshot identity
GraphRevision                 = exact routing topology identity
graph/Strategy schema version = decoding contract
Migration version             = database evolution position
cache codec version           = projection encoding contract
application build version     = deployed artifact identity
```

## 5. Strategy exact revision 契约

### 5.1 最小身份与内容

一份合法 Strategy revision snapshot 至少包含：

- 非零 `StrategyID`；
- `StrategyRevision`，满足 `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`；
- exact `schema_version = 1`；
- 一份通过现有 Lottery 领域构造/恢复校验的 Strategy name；
- 1～1000 个合法 Award；
- AwardID 在该 revision 内唯一；
- 每个 Award 的 name、positive weight 与 `reward | no_reward` outcome；
- 总权重不溢出 `uint64`；
- Award 集合按 AwardID canonical 排序并防御性复制。

Strategy revision snapshot 是对现有 Lottery aggregate 的不可变发布输入，不引入 Activity、窗口、审批、Principal、库存或发奖字段。

### 5.2 create-only 语义

同一 `(StrategyID, StrategyRevision)`：

- 只能创建一次；
- duplicate identity 是 conflict，不是幂等成功；
- 不能 Update、Upsert、Replace 或修改子 Award；
- 新内容必须使用新 StrategyRevision；
- 创建与严格恢复执行同一完整领域校验；
- 一个事务保存 header 与全部 Award，任一步失败整体回滚；
- commit outcome unknown 时按 exact identity 查询确认，不能盲重试不同内容。

现有 Strategy 行不会因为 Migration 自动成为 published revision。只有显式创建并成功恢复的 Strategy revision snapshot 才能进入 Activity publication binding。

### 5.3 revision token 不是内容哈希

v1 revision 是在权威 create-only registry 中与内容绑定的 bounded token。它可以使用人类可读发布编号或外部 promotion token，但不能仅靠调用方口头保证唯一。数据库复合主键、不可变写路径与 strict restore 共同建立 identity/content 绑定。

若未来需要跨环境内容寻址、签名或供应链证明，应新增 canonical digest/signature 协议；不能在本节把任意 revision 字符串冒充密码学 hash。

## 6. Activity 聚合与最小生命周期

### 6.1 Activity header

Marketing-owned Activity header 最少保存：

- 非零 `ActivityID`；
- bounded、canonical 的 operator-facing name；
- `lifecycle_state`：`draft | published | retired`；
- nullable `active_publication_version`；
- non-negative `state_version`；draft 固定为 0，published 固定等于 active publication version，retired 固定等于保留的 active publication version + 1；
- `retired_at`；
- nullable、bounded `retirement_evidence_reference`；
- 行级 `created_at` / `updated_at` 诊断时间。

header 不保存 graph node、Award/Weight、会员 tier、Participation policy、role、permission、tenant、public page 或 Draw state。

### 6.2 状态形状

| 状态 | active version | retired_at / retirement evidence | 含义 |
| --- | --- | --- | --- |
| `draft` | 必须为空 | 都必须为空 | `state_version = 0`；Activity identity 已创建，但没有任何可供新参与解析的 publication |
| `published` | 必须非零 | 都必须为空 | `state_version = active_publication_version`；是否已开始由时间窗派生 |
| `retired` | 保留非零最后版本 | 都必须非空 | `state_version = active_publication_version + 1`；永久停止新的参与解析并保留历史解释入口 |

v1 状态机只有：

```text
draft --publish v1--------------------------> published
published --publish v(n+1)------------------> published
published --rollback old -> append v(n+1)---> published
published --retire--------------------------> retired
```

明确禁止：

- draft 直接变 active 而没有 publication；
- retired 回到 published；
- published 回到 draft；
- 删除 Activity 代替 retire；
- natural window end 自动把 header 改为 retired；
- 用 `status=running` 定时更新数据库；
- 本节发明 pause/resume、cancel、approval 或 archived 状态。

审批是 Governance 的独立事实，不是 Marketing header 的生命周期枚举。草稿取消、暂停恢复、灰度和归档在出现真实操作协议后另行建模。

## 7. Immutable Activity publication

### 7.1 publication 最小内容

每份不可变 publication 保存：

- `ActivityID`；
- positive numeric `ActivityVersion`；
- `kind = release | rollback`；
- nullable `rollback_of_version`；
- exact graph `(GraphID, GraphRevision)`；
- graph terminal 到 exact Strategy revision 的 canonical bindings；
- UTC `starts_at`；
- UTC `ends_at`；
- canonical `published_at`；
- bounded `approval_evidence_ref`；
- create-only row metadata。

publication 不保存 current/active bool。active 是 Activity header 的单一引用；publication 本身永远是历史事实。

### 7.2 numeric version 规则

- 第一个 publication 必须是 version 1；
- 普通替换和回滚都必须生成 `current active + 1`；
- 不允许跳号、复用、倒退或客户端自选任意大版本；
- ActivityVersion 只在同一 Activity 内有序；
- draft 的 `state_version` 从 0 开始；每次成功 publish/rollback/retire 恰好递增 1；
- published 时 `state_version` 必须等于 active ActivityVersion，retired 时必须等于保留的 active ActivityVersion + 1；这是可校验的状态形状，但二者仍承担不同语义，不能互换字段或 token；
- ActivityVersion 一旦创建永不变更。

连续版本和 header CAS 共同让两个并发 publisher 中最多一个成功。loser 必须得到稳定 version conflict，重新读取当前状态后由操作者决定是否重建候选，不能自动覆盖 winner。

### 7.3 release 与 rollback discriminated union

| kind | rollback_of | 内容要求 |
| --- | --- | --- |
| `release` | 必须为空 | exact candidate 经 Lottery 验证与 Governance approval verifier 确认 |
| `rollback` | 必须非零且小于新 version | graph、Strategy bindings 与 window 精确复制同 Activity 的 source publication |

不能出现 `release + rollback_of`、`rollback + empty source`、跨 Activity source、source 指向未来版本或“只复制 graph、不复制 Strategy/window”的半回滚。

## 8. exact graph + Strategy revision 闭合集

### 8.1 为什么是一组绑定

graph v1 可以有多个 terminal node，也允许不同 terminal 指向同一 StrategyID。因此 publication 不只保存一个 Strategy revision，而是保存一个 canonical mapping：

```text
GraphID/GraphRevision
  + unique terminal StrategyID #1 -> exact StrategyRevision
  + unique terminal StrategyID #2 -> exact StrategyRevision
  + ...
```

### 8.2 闭合集不变量

Lottery publication verifier 必须：

1. 按 exact identity 读取 graph，不能查询 latest/active；
2. 重新执行 graph `Validate()`；
3. 收集全部 `strategy_target` node 的唯一 StrategyID 集合；
4. 拒绝 zero、duplicate、unknown 或超过 graph node hard limit 的 binding；
5. 要求 binding StrategyID 集合与 terminal unique set 精确相等；
6. 对每个 binding 按 exact `(StrategyID, StrategyRevision)` 读取一次 snapshot；
7. 重新验证 snapshot identity、schema 与完整 Strategy；
8. 只在全部成功后返回 `nil` 成功裁决；任一步失败只返回 error，不返回 partial proof。

v1 不另造一个“sealed evidence”输出类型：完整、不可变的 candidate 始终是 verifier 输入，成功 `nil` 只表示它已在本次调用中被整体验证。这不是可持久化 proof，也不能替代 Governance approval evidence；resolve 仍必须重新验证 exact refs。

集合必须满足：

```text
terminal StrategyID set == publication StrategyID set
```

而不是子集或超集。缺失会让某条合法 route 无法解析；额外 binding 可能把未经 graph 引用的配置偷渡进审批和审计范围。

### 8.3 不修改 graph v1

本节不把 `StrategyRevision` 回写到 graph terminal，也不重写第 28 节已经发布的 immutable graph schema。graph 继续决定**路由到哪个 StrategyID family**；Activity publication 决定该 ActivityVersion 下该 family 使用哪个 exact StrategyRevision。

执行关系为：

```text
Activity gate allow
  -> exact Activity publication
  -> exact graph evaluation
  -> terminal StrategyID
  -> lookup same publication's exact StrategyRevision binding
```

本节只建立到第三行所需的发布证据，不串联 evaluator、Strategy load 或 selector。

## 9. 普通发布协议

### 9.1 application 顺序

一次 publish 至少遵循：

```text
validate ctx / command / service
  -> read current Activity snapshot
  -> verify expected ActivityStateVersion
  -> exact-read and validate graph
  -> exact-read and validate every Strategy revision
  -> construct complete immutable candidate
  -> Governance approval verifier checks that exact candidate
  -> one repository transaction:
       INSERT publication
       INSERT all canonical Strategy bindings
       CAS UPDATE Activity header to published/new active/state_version+1
       COMMIT
```

Lottery 配置和 approval 是 read-only 前置依赖；唯一 Marketing 状态变化发生在最后一个本地事务。因为被引用的 graph/Strategy revisions 都 create-only，前置验证后不会发生“同 identity 内容变了”的合法写竞争。

### 9.2 原子可见性

以下状态都不能向其他事务成为成功事实：

- header 已切 active，但 publication 不存在；
- publication 存在，但只写了一部分 Strategy bindings；
- publication/bindings 已写，header CAS 失败后仍残留；
- state_version 已增长，但 active version 未切换；
- approval 失败却仍提交 publication。

publication、全部 bindings 与 header CAS 必须在同一个 InnoDB transaction 内完成。Commit 成功返回后，调用方才可确认 publish；Commit 错误且 caller 未取消时属于 outcome unknown，需要 exact read-back。

### 9.3 替换当前版本

published Activity 可以直接追加一个新的 release version。新 version 一旦 commit 就立即成为唯一 active publication；旧 version 保留但不再供新 gate 解析。

v1 不实现“提前发布未来版本、当前版本继续运行、到时自动切换”的双版本调度。若替换版本的 `starts_at` 在未来，Activity 会处于 confirmed `scheduled`，这是可见且需要操作者承担的结果，不得暗中继续旧版本。

## 10. 回滚协议

### 10.1 回滚不是修改历史

假设 current active 是 v4，目标历史版本是 v2：

```text
错误：active_version = 2
错误：UPDATE v4 SET graph = v2.graph

正确：INSERT v5(kind=rollback, rollback_of=2, exact copy of v2)
      + CAS active_version 4 -> 5
```

这样可以同时回答：

- 哪个版本曾经出错；
- 操作者何时发起恢复；
- 回滚依据哪份已验证 publication；
- 回滚后新参与实际使用哪个 ActivityVersion；
- v2 时期与 v5 时期产生的业务事实怎样区分。

### 10.2 回滚前置条件

v1 rollback 只允许：

- Activity 当前为 `published`；
- expected state version 与当前一致；
- source 属于同一 Activity；
- source version 非零且小于 current active；
- source publication 能被严格恢复；
- source exact graph/Strategy bindings 仍能被 Lottery verifier 验证；
- rollback 使用一次受控 evaluated-at，且满足 `evaluated_at < source.ends_at`；source 可以尚未开始或正在开放，但不能已经 ended；
- Governance verifier 批准 exact rollback candidate。

如果 source window 已结束，不能只换一个新结束时间后仍称为 rollback；那是新的 release candidate，必须重新审批和发布。若 source 尚未开始，rollback 复制其原窗口后会像普通 future release 一样成为 active publication，并由 gate 返回 `scheduled`，不会暗中继续旧版本。

### 10.3 不撤销已经发生的事实

rollback 只改变 commit 之后**新解析**的 Activity publication。它不会：

- 删除旧参与或抽奖事实；
- 撤回已经选择的 Award；
- 回补或冲正库存；
- 撤销已发权益；
- 中断已经读到旧 snapshot 的请求；
- 把旧结果重新随机一次。

正式 Draw/Result 出现后必须保存 ActivityVersion，并另行定义进行中请求、结果未知与补偿语义。

## 11. Retire 协议

`retire` 是 v1 唯一显式终止迁移：

- 只允许 `published -> retired`；
- 使用 expected `ActivityStateVersion` 做 CAS；
- 捕获一次 canonical `retired_at`；
- 让 Governance-owned verifier 核对绑定 ActivityID、expected state、last active version 与 `retired_at` 的 exact retirement intent，并保存 bounded evidence reference；
- 保留最后 `active_publication_version`；
- 不删除 Activity、publication、bindings、graph 或 Strategy snapshots；
- retired 是终态，不能 resume 或重新 publish；
- gate 对 retired 优先返回 confirmed business rejection。

保留 active version 是为了历史解释，不表示 retired Activity 仍可参与。

自然到达 `ends_at` 不自动 retire。时间窗结束与运营终态是两个不同事实：前者由 Clock 派生，后者由显式状态迁移确认。

## 12. `[start,end)` 时间门控

### 12.1 时间精度与规范化

所有业务时刻必须：

- 非零；
- 使用服务端受控 Clock；
- 规范到 UTC；
- 不携带 Go monotonic component；
- 可按 MySQL `DATETIME(6)` 无损往返，即 canonical microsecond precision；
- 满足 `starts_at < ends_at`。

浏览器时钟、HTTP header、客户端 timezone offset 或每个规则节点自行 `time.Now()` 都不能成为 Activity gate 事实。运营本地时区到 UTC 的转换属于未来输入/API 适配契约，不能在 domain 中猜测 DST。

### 12.2 gate 矩阵

对一次 RR current snapshot 和一次 `evaluated_at`：

| Activity/时间条件 | 闭集 domain status | `AllowsParticipation()` |
| --- | --- | ---: |
| `draft` | `not_published` | false |
| `retired` | `retired` | false |
| published 且 `now < start` | `scheduled` | false |
| published 且 `start <= now < end` | `active` | true |
| published 且 `now >= end` | `ended` | false |

上表是第 30 节唯一精确决定契约，status 本身就是稳定的业务结果。本节没有 HTTP/API，因此不再冻结第二套 `activity_*` transport reason；未来 adapter 若需要展示文案或错误码，必须从这五个 status 显式映射，不能重新解释时间窗。

边界必须精确：

- `start - 1µs` reject；
- `start` allow；
- `end - 1µs` allow；
- `end` reject。

### 12.3 confirmed rejection 与 technical failure

`not_published` / `scheduled` / `ended` / `retired` 都是基于完整可信 Activity snapshot 形成的 confirmed reject，`active` 是 confirmed allow。下列情况没有可信业务决定：

- Activity 不存在；
- header/publication/binding 损坏；
- active FK 或 identity 不匹配；
- Lottery exact binding 无法恢复；
- Clock zero/非法；
- Repository/approval/verifier unavailable；
- caller cancellation/deadline。

technical failure 必须返回 zero gate decision 和 error，不能 fallback 为 `ended`、`not_published` 或 `no_reward`。

## 13. 审批边界

### 13.1 Governance 拥有 approval

Marketing 不创建角色、权限或审批规则，也不自行判断申请人与审批人是否分离。发布 application 只依赖 consumer-owned approval verifier port：

```text
VerifyPublication(ctx, exact ActivityPublicationCandidate)
  -> trusted approval evidence | rejected | unavailable

VerifyRetirement(ctx, exact ActivityRetirementCandidate)
  -> trusted retirement evidence | rejected | unavailable
```

exact candidate 至少覆盖：

- ActivityID 与 candidate ActivityVersion；
- release/rollback kind 与 rollback source；
- exact graph ref；
- canonical exact Strategy revision set；
- exact `[starts_at,ends_at)`；
- candidate 的规范版本。

审批旧 candidate 后更改任何上述字段，都必须重新验证；不能复用一个只写“同意发布”的自由文本 token。

retirement intent 至少绑定 ActivityID、expected state version、last active publication version 与 canonical `retired_at`。它不把 retirement 伪装成新的 publication，也不能复用某份 release/rollback evidence。

### 13.2 本节没有真实 approval runtime

第 30 节只冻结 verifier port、evidence shape 和失败关闭语义。没有 Governance adapter、Principal、会话、RBAC 或审计 UI，因此：

- service 不进入 `growth-api` composition；
- 不能公开 publish/rollback/retire route；
- test fake 只证明调用契约，不证明真人审批已经上线；
- evidence reference 是受控关联线索，不是密码学签名；
- 第 31～35 节仍需独立完成身份、授权、前端投影与越权验收。

## 14. consumer-owned 端口与应用服务

### 14.1 Lottery 端口

最小 Lottery 能力：

- `StrategySnapshotCreator.CreateSnapshot`：保存一个完整 exact snapshot；
- `StrategySnapshotReader.FindSnapshotByIdentity`：严格恢复 exact snapshot；
- 现有 `StrategyRoutingGraphReader.FindByIdentity`：读取 exact graph；

不得把现有 `StrategyReader.FindByID` 改造成隐式 latest revision reader，也不得给 graph Repository 增加 `FindActive(ActivityID)`。

### 14.2 Marketing 端口

最小 Marketing 持久化端口按用例拆分：

- draft creator；
- current Activity snapshot reader；
- exact publication reader；
- atomic publication writer；
- CAS retire writer；
- `ApprovalVerifier.VerifyPublication` / `VerifyRetirement`；
- Marketing consumer-owned `LotteryVerifier.VerifyPublication`，由 `adapter/lotteryconfig` 通过上述 Lottery exact readers 与 Lottery domain validation 核对 graph terminal / Strategy revision 闭合集。

端口不叫通用 `Save` / `Update` / `Upsert` / `Delete`。Publication writer 的契约是“插入完整不可变版本、插入全部 binding、CAS 切 active”，不能被 adapter 拆成三个可单独调用的 public 方法。

### 14.3 应用服务

本节允许以下未装配 Marketing application services：

1. `CreateDraftService`；
2. `PublishActivityService`；
3. `RollbackActivityService`；
4. `RetireActivityService`；
5. `ResolveActivityService`。

Strategy snapshot 本节只有 Lottery domain aggregate、create/read ports 与 MySQL Repository adapter，没有虚构一个 application service；未来真实配置写入口必须另行定义 command、授权与治理边界。

服务持有只读 ports/config，不保存请求级 candidate、timer、path 或 current state；并发安全取决于注入端口和 Clock 自身可并发。

## 15. 持久化与前向 Migration

### 15.1 五张表、六个单 DDL

本节将 Migration latest 从 5 推进到 11；每个版本仍只包含一个 MySQL DDL：

| Migration | 结构 | 主要职责 |
| --- | --- | --- |
| `000006` | `lottery_strategy_snapshot` | exact Strategy revision header 与完整 name/schema identity |
| `000007` | `lottery_strategy_snapshot_award` | exact revision 下不可变 Award snapshot |
| `000008` | `marketing_activity` | Activity header、lifecycle、active version 与 state CAS |
| `000009` | `marketing_activity_publication` | immutable ActivityVersion、graph ref、window、kind/source、approval evidence |
| `000010` | `marketing_activity_publication_strategy` | publication terminal StrategyID 到 exact StrategyRevision mapping |
| `000011` | `ALTER marketing_activity ...` | header `(activity_id,active_version)` 到 publication 的反向 FK |

这里是五张新表加一个 ALTER，不是六张表。已经执行的 `000001`～`000005` 不回写。

### 15.2 为什么 active 反向 FK 可以后加

graph header 无法对 root node 建反向 FK，因为 graph/node 首次 INSERT 会形成 InnoDB immediate-check insertion cycle。Activity 不同：

1. 先创建 `draft` header，active version 为 NULL；
2. publication 可以引用已存在 Activity；
3. publication 插入后，同一事务再 UPDATE header active pointer；
4. 因此表都存在后可以追加反向 FK。

不应在数据库能够可靠表达时继续接受 dangling active pointer。

### 15.3 数据库与领域各自证明什么

数据库负责各 bounded context 内部能够准确表达的局部事实：

- composite PK/FK 与 scoped identity；
- ID/revision/status/kind 的局部 shape；
- Activity lifecycle、active version、state version 与 retired_at 的完整状态形状；
- Strategy revision header/award 父子引用；
- publication -> Activity 与同 Activity rollback source；
- binding -> 同 Activity publication；
- header active -> exact publication；
- start/end 与 release/rollback 的局部 CHECK；
- RESTRICT 防止历史被级联删除。

Marketing publication **不对 Lottery graph 或 Strategy snapshot 建跨 bounded-context FK**。它只保存 exact refs，由 Lottery verifier 在 publish 前核对，并由未来 resolve 路径再次 fail closed。这样不会让 Marketing Migration 依赖 Lottery 物理表名、主键布局和同库部署，也避免未来拆服务时把数据库 FK 误当业务契约。代价是数据库本身不能证明跨上下文目标存在或闭合集完整；最小权限、exact verifier、strict resolve 和坏引用集成测试必须共同承担这条边界。

领域/application 负责：

- Strategy aggregate 完整不变量；
- graph 全局合法性；
- terminal StrategyID set 与 binding set exact equality；
- version 连续追加与 active/latest 一致；
- rollback 完整复制 source；
- approval evidence 确实针对 exact candidate；
- state transition、Clock 与 gate 决定；
- CAS loser、commit outcome unknown 和错误分类。

### 15.4 最小权限

Strategy revision 与 Activity publication/binding rows 不提供 UPDATE/DELETE。隔离测试 writer 只获得所需 SELECT/INSERT；Marketing writer 仅能对 Activity header 执行受控 UPDATE，并不能修改 Lottery graph/Strategy snapshot。

本节不把这些 grants 加给长期 `growthos_app`，因为 publication/gate/evaluator 没有 runtime composition。拥有 migrator 或 DBA 权限仍可绕过应用协议，严格 Restore、最小权限、审计与故障停止线共同缓解该残余风险。

## 16. 并发、一致性与失败语义

### 16.1 CAS 防止 lost update

每个命令携带 expected `ActivityStateVersion`。Repository 的 header UPDATE 必须同时匹配：

- ActivityID；
- expected state version；
- expected lifecycle state；
- expected active publication version。

RowsAffected 必须恰为 1。为 0 是 version/state conflict，不是成功；大于 1 是内部不变量故障。

### 16.2 current snapshot 不混版

current reader 必须在一个 read-only REPEATABLE READ snapshot 中读取：

```text
Activity header
  + active publication
  + all exact Strategy bindings
```

不得先读 header、事务外再读 publication，也不得在 binding 读取中看到另一个 active version。Repository 在同一 RR 事务内读完所有行后立即严格 Restore，只有 Restore 成功才结束这个只读事务并返回完整 snapshot。

### 16.3 commit outcome unknown

若 COMMIT 返回网络错误，而 caller 与 operation context 在 Repository 返回后都仍存活，应用不能确定事务是否已经提交。错误必须归类为 `ErrCommitOutcomeUnknown`；publish、rollback、retire 仍分别返回 zero publication / zero Activity，不能把 candidate 当作成功结果。

只有这个精确错误类别可以在 `ActivityOperationError` 内附带一份经校验、不可变且防御复制的 `ActivityCommitReceipt`，并由可信恢复流程通过 `ActivityCommitReceiptFromError` 显式提取。Receipt 不进入 `Error()`、`errors.Is` 或普通 unwrap chain；caller cancel、private timeout、conflict、retryable 与 ordinary storage failure 都不能取得 receipt。

Receipt 保存 exact before/after root；publish/rollback 还保存完整 immutable publication，retire 的 after root 保存本次 server-owned `retiredAt` 与 retirement evidence。恢复流程使用新健康连接：publish/rollback 在同一个 RR current snapshot 读取 root + exact active publication 并构造 `ObserveCurrentActivity`；retire exact读取 root 并构造 `ObserveActivityRoot`。随后只调用纯函数 `ReconcileActivityCommit`：

| 结果 | 可证明事实 | 处置边界 |
| --- | --- | --- |
| `committed` | exact after root 命中；publish/rollback 的完整 publication 也逐字段相等 | 采用既有提交事实，不追加版本 |
| `not_committed` | exact before root 仍在，或同一 next state generation 已由另一合法 winner 占据 | 只能由上层另行决定是否发起一项新操作 |
| `indeterminate` | observation 缺失、损坏、partial、identity/name 不符，或 root 已推进到更晚 generation | 停止写入并调查，不能猜测中间历史 |

更晚 generation 不能证明本次未曾先提交；对账函数因此 fail closed 为 `indeterminate`。它不做 I/O、不重读 Clock/approval，也不产生 retry 建议。即使结果为 `not_committed`，也不能绕过上层授权、幂等和重新审批策略盲重放原 command。

## 17. 错误与决定必须正交

应用至少要区分：

- invalid argument / not configured；
- Activity not found / publication not found；
- Activity state conflict / version conflict；
- approval rejected / approval unavailable；
- graph or Strategy revision not found；
- Lottery publication binding invalid / unavailable；
- stored Activity/Strategy snapshot invalid；
- Repository retryable failure / general failure / commit outcome unknown；
- caller canceled / deadline exceeded；
- confirmed gate allow/reject reason。

稳定错误文本不得包含 SQL、table、credential、approval payload、完整 graph、Strategy Awards 或内部 cause。可信代码可以获得受控 cause 用于诊断；公开 HTTP 映射尚未定义。

授权拒绝不进入这组业务错误。第 33 节服务端授权必须在调用 publish/rollback/retire 之前形成独立 access decision，不能把 forbidden 映射为 Activity conflict 或 approval rejected。

## 18. 威胁边界

| 威胁 | 本节控制 | 残余边界 |
| --- | --- | --- |
| caller 用空 revision 请求“随便一个” | exact identity 构造与 Reader；无 latest API | 尚无外部 transport |
| graph terminal 缺少 Strategy version | exact closed binding verifier | DB 不能独立证明跨上下文引用或 set equality |
| 额外 Strategy 偷渡进 publication | exact terminal set equality | 特权旁路写仍需 restore/权限缓解 |
| 并发 publisher 静默覆盖 | state_version + expected active CAS | loser 需人工重建 candidate |
| 回滚改写或删除事故版本 | append-only rollback + rollback_of | 不撤销进行中/已完成事实 |
| 浏览器时间影响活动开关 | 一次 server Clock + UTC `[start,end)` | 本地时区输入留给未来 API |
| UI 隐藏按钮被当作授权 | 无 UI/runtime；31～35 独立实现 | 当前不能公开写入口 |
| approval token 与 candidate 不匹配 | verifier 接收 exact candidate | 尚无真实 Governance adapter |
| 坏数据库行被当合法 snapshot | bounded read + strict Restore | DBA/migrator 仍是受信高权限边界 |
| retired 后旧请求仍执行 | 后续 snapshot 立即拒绝 | 已读取旧 snapshot 的请求不被撤回 |
| Strategy cache 返回按 ID 的旧/新混合 | publication exact reader 不使用现有按 ID cache | 未来可另建 versioned projection |

本节没有会话、Principal、tenant 或对象级 scope，所以不能声称 publish/rollback/retire 已受保护。相反，正因为高风险写入口尚未授权，所有服务保持未装配。

## 19. 观测、审计与敏感信息

允许的低基数观测：

- operation：create_draft/publish/rollback/retire/gate；
- result class：success/conflict/rejected/unavailable/invalid/internal；
- lifecycle state；
- gate outcome/reason；
- dependency：activity_repo/lottery_verifier/approval_verifier；
- duration 与 retryable/commit-unknown counter。

不应作为普通 metric label：

- ActivityID、GraphID、StrategyID；
- 任意 revision/evidence reference；
- Activity name；
- Award name/weight；
- Principal、session、tenant 或用户事实。

受控日志/trace 可以记录 exact ActivityVersion、graph/Strategy revision refs 与 request correlation，但不能记录凭证、approval 原始 payload、会员事实、随机票据或 SQL cause。Governance 的持久审批审计和操作者身份留给后续访问控制/运营章节。

## 20. 与第 31～35 节访问控制的停止线

| 章节 | 后续问题 | 第 30 节禁止预制 |
| --- | --- | --- |
| 31 | Principal、resource、action、scope、默认拒绝与披露威胁边界 | `isAdmin`、owner_role、tenant 猜测字段 |
| 32 | credential/session 到可信 Principal | 把 ActivityID、MembershipSubjectRef 当登录身份 |
| 33 | 服务端对 create/publish/rollback/retire/read 强制 RBAC | 因 approval 通过就跳过授权 |
| 34 | 前端 capability projection、导航/路由/操作裁剪 | 隐藏按钮等于安全 |
| 35 | direct API、跨角色/对象/会话与浏览器 E2E | 在无 route/session 时伪造越权成功证据 |

审批与授权也必须分开：审批回答“这个 exact candidate 是否经过业务治理”，授权回答“当前 Principal 是否能发起此动作”。二者缺一都不能公开执行。

## 21. 与正式 Draw/Result 的停止线

第 30 节只形成 Activity publication snapshot。它不：

- 调用 Participation eligibility chain；
- 读取会员 fact 或执行 graph；
- 解析 terminal 后加载 Strategy revision；
- 调用 WeightedSelector；
- 消费随机数；
- 创建 Participation/DrawID；
- 扣次数、库存或预算；
- 形成 reward/no_reward 最终事实；
- 发起 Benefit 交付；
- 提供幂等、结果查询、补偿或 MQ。

正式流程未来必须把本次使用的 `ActivityID + ActivityVersion`、graph identity、Strategy revision、必要规则/事实版本固化到 Draw/Result；不能事后用当前 active publication 解释历史。

## 22. 验收要求

第 30 节实现后至少证明：

1. Strategy revision snapshot 的 create/restore、canonical order、defensive copy、1000 Award 边界与 create-only conflict；
2. Activity draft/published/retired 的所有合法和非法状态形状，包括 StateVersion 分别为 `0`、`active`、`active+1`；
3. 首次 publish 为 v1，普通替换与 rollback 连续追加；
4. rollback 精确复制 source graph/Strategy/window，允许尚未开始或正在开放的 source、拒绝 `evaluated_at >= source.ends_at`，并记录 source；published_at/version/evidence 属于新事实；
5. retired 保留 active version且永不 resume；
6. `[start,end)` 四个微秒边界和一次 Clock；
7. graph terminal StrategyID 与 exact Strategy refs 对重复、缺失、额外、错 identity、未知 schema 均失败关闭；
8. 两 Activity 可复用同配置，一 Activity 可在版本间切配置；
9. 两个并发 publisher 对同 expected state 恰好一个成功；
10. publication/binding/header CAS 任一步失败整体 rollback；
11. current RR snapshot 不混合 header/publication/binding；
12. publish/rollback/retire 的 commit outcome unknown 保持 zero result，只能显式提取可信 receipt，并以 exact observation 对账为 `committed` / `not_committed` / `indeterminate`；普通失败和 context 结束不携带 receipt；
13. approval verifier 接收 exact candidate；改动 window/ref/kind/source 后旧 evidence 不可复用；
14. pre-cancel、dependency 边界 cancel、error+value、typed nil 与坏 Clock 返回 zero result；
15. Migration 5 -> 11 前向升级、重复 no_change、dirty fail-closed，旧五表指纹不变；
16. 五张表、各上下文内部 FK 与 active reverse FK 的 PK/FK/CHECK/RESTRICT 在真实 MySQL 8.4 验证，并证明 Marketing schema 没有到 Lottery 表的跨上下文 FK；
17. immutable rows的 UPDATE/DELETE 被精确 grant 拒绝；长期 runtime grant 不变；
18. domain/application/repository 普通、race、fuzz/并发测试通过；
19. architecture guard 证明没有 HTTP/UI/composition/auth/selector/Draw 等越界接入；
20. 所有失败不返回可误用的 partial publication、binding 或 allow decision。

## 23. 风险账本与重新决策触发器

| 未决风险 | 当前接受理由 | 重新决策触发器 |
| --- | --- | --- |
| graph terminal 仍只含 StrategyID | 不修改 immutable schema v1，publication exact mapping 补足 | graph schema v2 需要直接绑定 Strategy revision |
| Marketing 到 Lottery 没有物理 FK | 避免跨 bounded-context schema 耦合；发布与解析均 exact fail closed | 出现旁路坏引用事故或跨服务契约演进 |
| 没有提前排期的双 active model | 保持一个 active、一个学习目标 | 真实运营要求无缝定时切换 |
| retire 不撤回已解析请求 | 当前没有正式副作用主链 | 正式 Participation/Draw 串联与紧急 kill switch |
| approval verifier 未装配 | 权限/身份顺序尚未建立 | 第 31～36 节形成受保护运营入口 |
| 没有 tenant/data scope | 不在 Marketing 猜 Governance 模型 | 第 31 节公共资源/范围模型要求持久 scope |
| Strategy snapshot 存储重复内容 | 历史可恢复优先于去重 | 规模/成本测量证明需要内容寻址或压缩 |
| 不缓存 exact publication | 当前读取/失效负载未知 | 真实 runtime/负载证明数据库成为瓶颈 |
| UTC 只保存 instant，不保存运营 timezone | 无输入/UI 协议，不猜 DST | Activity 编辑/展示 API 出现 |
| rollback 只允许 source 尚未 ended | 允许与普通 future release 一致地回到 scheduled 配置，同时避免借 rollback 偷改已结束窗口 | 产品要求回滚配置并独立重设窗口 |

以下变化必须新增或替代 ADR：

- pause/resume、scheduled activation、灰度或多个同时 active version；
- Activity 草稿编辑、审批状态机或高风险字段 diff；
- Strategy revision 内容 hash、签名、跨环境 promotion 或归档删除；
- graph schema v2 直接引用 Strategy revision；
- tenant、业务空间、对象级 data scope；
- emergency kill switch 需要强于普通 snapshot 语义；
- publication event、Outbox、MQ 或跨服务一致性；
- 正式 Draw 持久化 Activity/graph/Strategy snapshot；
- versioned cache、compiled plan 或运行性能瓶颈；
- 数据保留、隐私、合规或不可抵赖审计要求。

## 24. 当前可以与不能宣称的能力

实现并验收后可以准确表述：

> 在 Marketing 上下文建立 draft/published/retired Activity 与 append-only numeric publication version；将每个版本原子绑定 exact Lottery graph revision 及其全部 terminal Strategy 的 exact create-only revision，以 state-version CAS 防止并发覆盖，通过追加版本完成普通发布和可追溯回滚，并用一次服务端 UTC 时刻按 `[start,end)` 形成确定 Activity gate。Strategy revision、publication 与 bindings 使用前向 MySQL schema、严格恢复和最小权限保护，所有服务仍未装配。

不能表述：

- 已上线运营发布平台或审批系统；
- 已实现登录、RBAC、租户/对象授权或前端权限；
- 已提供 Activity/Strategy/graph 管理 API 或 UI；
- Activity allow 表示用户有资格、有权限或已经抽奖；
- 已串联会员路由、Strategy load、selector 或正式 Draw；
- rollback 能撤销已完成参与、库存或权益；
- retire 能同步中断所有已开始请求；
- 已实现 pause/resume、灰度、多 active 或定时无缝切换；
- 单元/集成测试证明生产 SLO、容量或合规。

## 25. 相关资料

- [运营人员工作流 v1](operator-workflow-v1.md)
- [限界上下文地图 v1](bounded-context-map-v1.md)
- [Lottery 业务规则需求基线 v1](lottery-rule-requirements-v1.md)
- [Lottery Strategy Routing Graph 基线 v1](lottery-strategy-routing-graph-v1.md)
- [Lottery Strategy Routing Evaluation 基线 v1](lottery-strategy-routing-evaluation-v1.md)
- [ADR-0019：Lottery 规则所有权与评估边界](../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [ADR-0024：Lottery Strategy 路由图持久化](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)
- [ADR-0025：Lottery Strategy 路由图求值](../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)
