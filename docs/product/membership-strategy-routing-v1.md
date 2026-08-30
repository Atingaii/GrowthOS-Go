# Lottery 会员等级 Strategy 路由基线 v1

- **状态：** 第 27 节已批准实现基线
- **日期：** 2026-08-30
- **适用范围：** Lottery domain/application 内核中的会员等级路由切片
- **不适用范围：** 会员主数据写模型、Participation 资格、Strategy 持久化校验、规则树/引擎、Activity、身份认证、访问授权、公开 API、adapter、runtime 与前端

## 1. 为什么责任链现在开始不够用

第 26 节的 Participation 前置资格链解决的是合取问题：新用户资格通过后继续执行风险准入，任何确定拒绝或技术失败都立即停止。它的控制流只有两种：

```text
eligible   -> 固定的下一个 gate
ineligible -> 终止
```

第 23 节的 LRR-003 还登记了另一种真实问题：不同会员等级可能进入不同 Lottery Strategy。这不是“是否继续”的资格判断，而是“从多个合法后续目标中选择哪一个”的路由决定。若继续沿用第 26 节协议，只能把 Strategy ID 偷塞进 reason、在 handler 外挂 `if/else`、让 step 修改隐式 next-index，或复制多条链。这些做法都会让真实分支、缺省语义和所走路径变成不可审查的控制流。

因此第 27 节不废弃线性链，也不把 Participation 改造成路由平台；它在 Lottery 内新增一个具体会员等级路由切片，用两个真实出口、一个显式缺省分支和一跳 path trace 证明链的表达边界。持久化树与通用执行器仍由第 28～29 节的新增证据驱动。

## 2. 本节新增的产品决定

第 23 节只规定“会员分层路由必须有明确目标和显式缺省策略”，没有规定 `standard`、`premium` 的精确映射。本节新增并冻结以下最小产品语义：

1. 外部会员 authority 当前只确认 `standard` 或 `premium` 两种受支持等级；
2. `premium` 命中显式 override 分支，路由到 premium Strategy 目标；
3. `standard` 命中基线 `default` 分支，路由到 baseline Strategy 目标；
4. `unknown`、零值、未来新增但本策略不支持的等级，以及缺失/损坏/过期事实都不进入 `default`；
5. 只有一份已验证事实与一份合法策略才能形成确定 Route；任何事实、依赖、配置、时钟或取消问题均返回零决定并失败关闭。

最小决策表如下：

| 权威会员事实 | 选择的 branch | Strategy 目标 | 结果类别 |
| --- | --- | --- | --- |
| confirmed `premium` | `premium_override` | policy 的 premium target | 确定 Route |
| confirmed `standard` | `baseline_default` | policy 的 baseline/default target | 确定 Route |
| zero / `unknown` | 无 | 无 | 技术失败，零 Route |
| unsupported tier | 无 | 无 | 技术失败，零 Route |
| not found / unavailable / stale / future / corrupt | 无 | 无 | 技术失败，零 Route |

`default` 是一条经过产品批准、只对“已确认且受支持但没有专用 override 的 standard”生效的确定边，不是异常 fallback。不得用它把未知事实、依赖超时或未来会员等级静默降级为普通用户。

## 3. 业务不变量

1. **先确认事实，再选择分支。** 未经服务端权威来源确认的会员等级不能参与路由。
2. **一个输入只选一个出口。** 同一 policy revision、同一事实快照和同一 evaluated-at 只能产生一个 branch 与一个 Strategy target。
3. **Route 不是资格。** 路由成功不表示 Activity 可参与、Participation 已通过、次数已扣或调用者已获授权。
4. **Route 不是选择。** 路由成功只得到 Strategy ID，不加载 Strategy、不调用 `WeightedSelector`、不消费随机票据。
5. **default 不是容错。** 只有 confirmed `standard` 可以进入本版 default；未知、未支持、缺失、过期和依赖失败全部失败关闭。
6. **分支身份与目标身份分离。** path 必须说明选择了 `premium` 还是 `default`；即使某次配置让两个分支暂时指向同一 Strategy ID，也不能丢失分支证据。
7. **事实版本、路由 policy revision、Strategy ID 与未来 Strategy version 分离。** 当前 Strategy 只有 ID，本节不得伪造 Strategy version。
8. **一个受控 as-of。** 每次有效求值至多读取一次服务端 Clock，所有 future/freshness 判断和决定证据使用同一 canonical UTC 时刻。
9. **错误没有半成品决定。** provider 同时返回 snapshot 与 error 时 error 胜出；错误或取消时 Route 与 path 都为零。
10. **客户端不拥有路由输入。** 浏览器不能提交可信 tier、evaluated-at、policy revision、branch 或 Strategy target。

## 4. 事实、决定与所有权

| 概念 | 权威所有者 | 本节 Lottery 可以做什么 | 本节 Lottery 不能做什么 |
| --- | --- | --- | --- |
| 会员等级生命周期 | 外部会员 authority | 通过 consumer-owned 端口读取最小不可变摘要，并验证来源、revision、主体与时间 | 创建/升级/降级会员，解释会员权益，接受客户端自报等级 |
| 会员事实防腐映射 | 未来 adapter | 把外部协议严格映射为本节支持的事实枚举与错误 | 把未知值默认成 `standard`，用读取时间给旧事实续鲜 |
| 等级到 Strategy 的路由 policy | Lottery | 定义 premium override、baseline default、policy revision 与目标 | 让外部会员系统直接返回 GrowthOS Strategy ID |
| Route 决定与 path | Lottery | 根据已验证事实纯计算唯一 branch 与目标 | 宣布用户已具备 Participation 资格或访问权限 |
| 新用户/风险/次数资格 | Participation | 本节不拥有 | 把资格 gate 塞进 Lottery router |
| Strategy/Award/Weight | Lottery 现有 Strategy 聚合 | 本节只使用非零 Strategy ID 作为目标形状 | 加载/验证目标存在、发布或版本，改变 Award/Weight |
| Activity 发布绑定 | Marketing（第 30 节） | 本节不拥有 | 提前决定哪个 Activity 使用哪份 route policy/Strategy |
| Principal、角色、资源动作与范围 | Governance（第 31～35 节） | 本节不拥有 | 用会员等级代替认证或授权 |

本节建议使用 Lottery 本地的 opaque `MembershipSubjectRef` 作为读取键。它只是外部会员事实的内部查找引用，不复用 Participation 的 `ParticipantRef`，也不是 Principal、登录证明、租户、角色或对象级数据范围。真正身份如何可靠映射到该引用，延后到认证与流程编排出现时决定。

## 5. 最小输入契约

一次路由求值需要：

- caller `context.Context`；
- 非零、不可从 tier 推导的 `MembershipSubjectRef`；
- 一份具体 `MembershipStrategyRoutingPolicy`：
  - 非空 policy revision；
  - 非零 premium Strategy target；
  - 非零 baseline/default Strategy target；
- consumer-owned `MembershipTierFactReader`；
- 受控服务端 `Clock`；
- 正的 maximum fact age。

reader 返回的最小事实快照只包含：

- 与请求一致的 opaque subject ref；
- 枚举 tier：`standard` 或 `premium`；
- authority 产生/确认该事实的 `observed_at`；
- 受控 source；
- 非空 fact revision。

快照不包含姓名、手机号、邮箱、订单、消费金额、成长值、会员权益、完整画像、角色、Cookie、token 或由客户端传来的 Strategy ID。adapter 不得以本地读取时间替代 `observed_at`。

## 6. 最小输出契约

确定成功时形成一个不可变 `MembershipStrategyRouteDecision`，至少能回答：

- 使用了哪条稳定路由规则；
- 选择了 `premium_override` 还是 `baseline_default` branch；
- 对应稳定 reason 是 `premium_strategy_selected` 还是 `baseline_strategy_selected`；
- 目标 Strategy ID 是什么；
- 使用了哪一 policy revision；
- 使用了哪一 fact source/revision；
- 唯一 evaluated-at 是什么；
- 实际走过的一跳 path 是什么。

概念形状如下，名称是本节领域语言而非跨上下文通用协议：

```text
MembershipStrategyRouteDecision
  RuleCode       = lottery.membership_tier.route_strategy
  BranchCode     = premium_override | baseline_default
  ReasonCode     = premium_strategy_selected | baseline_strategy_selected
  Target         = StrategyID
  PolicyRevision
  FactSource
  FactRevision
  EvaluatedAt
  Path           = [membership-tier-decision --branch--> strategy target]
```

path 只记录实际选择的一条边和终点，不伪造未走分支，也不声明一个通用 `Node/Edge/RuleTree` API。调用方取得的是副本或不可变值，不能改写内部证据。业务错误或 caller cancellation 返回零 decision，不能携带“猜测 target”或半条 path。

## 7. standard、premium、unknown 与 default 的精确定义

### 7.1 standard

`standard` 是外部 authority 明确确认、且当前 policy 支持的业务等级。它没有专用 override，因而按产品定义选择 `baseline_default` branch。path 中记录 `baseline_default`，而不是虚构 `standard` 专用边。

### 7.2 premium

`premium` 是外部 authority 明确确认、且当前 policy 有专用 override 的业务等级。它选择 `premium_override` branch。会员 authority 只提供等级事实，不能决定 premium target。

### 7.3 unknown

`unknown` 在本节不是一个可路由的业务等级。它表示消费者无法把输入确认成受支持事实，包括零值、未分类 provider 值、损坏映射或无法确认的状态。它属于“尚无可信 Route”的技术边界，不是 ADR-0019 中“可能已经产生副作用但最终结果暂时无法确认”的流程结果 `Unknown`，也不能映射成 `standard`。

未来若产品真的需要 guest、unclassified 或 trial 群体，必须把它命名为新的业务等级、更新 policy revision 与兼容测试，而不是复用 `unknown`。

### 7.4 default

`default` 是 Lottery policy 的显式基线边，不是会员等级。当前只有 confirmed `standard` 选择它。未来增加受支持等级时，必须明确该等级是否允许命中基线边；执行器不得因为 map 没匹配就自动 fallback。

## 8. 求值顺序、时间与短路

一次求值按固定顺序执行：

1. 校验 context、subject ref、policy、target、revision、reader、Clock 与 freshness 配置；
2. 检查 caller 是否已取消；预取消时 Clock 与 reader 都是零调用；
3. 调用受控 Clock 一次并规范为 UTC；零值或非法时钟结果失败，reader 零调用；
4. 使用 subject ref 读取一次会员事实；
5. reader 返回后再次检查 caller context；caller cancellation/deadline 优先；
6. 若 reader 返回 error，丢弃同时返回的 snapshot，并通过稳定读取错误边界保留 cause；
7. 校验 snapshot 主体、tier、source、revision、future 与 freshness；
8. 纯计算唯一 branch、target 和一跳 path。

`evaluated_at - observed_at == max_age` 时事实仍有效；多一纳秒即 stale。`observed_at` 比 evaluated-at 晚一纳秒即 future，不通过。单一 evaluated-at 只是可解释的逻辑 as-of，不使外部 authority 与 Lottery policy 成为原子快照，也不证明 adapter 支持历史 as-of 查询。

确定 Route 后本节没有 I/O 或副作用。不得为了“补偿取消”撤销、换路或重新读取；未来上层在调用正式 Draw 前仍须重新遵守自己的取消与幂等边界。

## 9. 失败与 Cause 通道

### 9.1 稳定边界

本节至少区分：

- 调用契约/配置错误；
- 时钟错误；
- 会员事实读取失败；
- 会员事实无效：zero/unknown/unsupported/mismatch/future/stale/corrupt；
- caller cancellation/deadline。

对外稳定错误只暴露安全 class 与必要的 rule/fact 边界，不渲染 endpoint、SQL、完整 provider payload、外部错误文本、subject ref 或会员资料。读取 wrapper 只能通过显式 `Cause()` 方法保留原始错误供受控诊断，不实现 `Unwrap()`，避免 raw provider error 进入 `errors.Is` tree；不能用字符串拼接后再解析错误类别。

### 9.2 取消优先级

- 调用前 caller 已取消：原样返回 caller context error；
- reader 执行期间 caller 被取消：reader 返回后以 caller context error 为准；
- provider 自身超时但 caller 仍存活：属于会员 authority unavailable/read failure，不伪装成 caller cancellation；
- fact 与 error 同时返回：error 胜出；
- 所有失败路径：零 Route、零 path，不调用 Strategy repository 或 selector。

“失败关闭”只表示不能继续到 Strategy 加载/选择，不表示把错误伪装成业务 `reject` 或把用户降级到 default。

## 10. 最小 Route 证据、path 与披露

成功 Route 的决定包保存解释该路由所需的最小字段；其中一跳 path 自身只复制 rule code、selected branch 与 target，policy/fact/as-of 由外层 decision 关联，避免每一跳重复：

| 字段 | 用途 | 披露限制 |
| --- | --- | --- |
| stable rule code | 识别本节路由决定 | 可用于低基数指标 |
| selected branch code | 证明 premium override 或 baseline default 被选择 | 属会员派生信息，不能作为公开用户画像或普通 metric label |
| target Strategy ID | 让后序明确加载目标 | 当前不是 Strategy version；不得进入无界 metric label |
| policy revision | 回放哪份映射 | 受控日志/决策证据，不作高基数 label |
| fact source/revision | 证明事实来源 | 不包含原始 payload；revision 不作 label |
| evaluated-at | 解释 freshness 边界 | 不记录客户端时间 |

trace 不记录 MembershipSubjectRef、原始 tier payload、会员订单/金额、provider endpoint、完整 error、凭证、随机票据或未走分支。第 27 节 trace 是进程内决定证据，不是持久化审计、OpenTelemetry span 或面向普通用户的完整解释。

## 11. 为什么不能扩展第 26 节 chain

| 方案 | 表面收益 | 破坏 |
| --- | --- | --- |
| 把 target 塞进 eligibility reason | 复用返回类型 | reason 同时承担拒绝与路由，Participation 开始拥有 Lottery 目标 |
| `eligible` 后由 handler 按 tier 再 `if/else` | 改动少 | 路由 policy、default、trace 与版本散落 transport |
| step 返回隐式 next-index / goto | 看似支持分支 | 线性 slice 变成未验证的图，循环/越界/缺省不可审查 |
| 为 premium/standard 复制两条链 | 每条仍线性 | 前置规则重复，版本漂移，分支合流和 path 不明确 |
| 会员 authority 直接返回 Strategy ID | Lottery 代码最少 | 外部事实所有者越权拥有 GrowthOS 路由决定 |
| 立即上通用规则树/引擎 | 一次覆盖未来需求 | 第 27 节尚无持久化、发布、图校验或多节点执行证据 |

正确边界是：Participation chain 继续回答“能否进入后序”，Lottery router 回答“进入哪一个 Strategy 目标”。未来编排者可以顺序消费两个决定，但不能把二者压成同一个 bool、Rule 接口或上下文大对象。

## 12. 验收矩阵

### 12.1 路由语义

- confirmed premium 选择 `premium_override` branch 与 premium target；
- confirmed standard 选择 `baseline_default` branch 与 baseline target；
- 测试 fixture 使用不同 target，证明存在两个真实出口；
- 即使两个 target 配置成相同合法 ID，branch/path 仍可区分；本节不凭空规定 target 必须互异；
- 同一 policy、fact、as-of 重复求值得到相同 Route 与 path；
- zero/unknown/unsupported tier 不命中 default；
- Route 成功不调用 repository、selector、随机源或任何副作用端口。

### 12.2 输入、事实和时间

- 零 subject、空 policy/fact revision、零 target、nil/typed-nil dependency、非正 freshness 在 I/O 前失败；
- mismatched subject、空 source、future、stale、corrupt snapshot 返回零 Route；
- freshness 恰好等于 max age 有效，超过一纳秒无效；
- observed-at 恰好等于 evaluated-at 有效，晚一纳秒无效；
- reader not found/unavailable/未分类错误均不 fallback；
- reader 同时返回 fact/error 时 error 胜出且 fact 不可观察。

### 12.3 时钟、取消和错误

- pre-cancel 时 Clock/reader 都零调用；
- 合法求值恰好读取 Clock 一次、reader 一次，并使用 UTC as-of；
- Clock 非法时 reader 零调用；
- reader 返回后 caller cancellation 优先且 decision/path 为零；
- provider timeout 而 caller 存活时保留为受控 read failure；
- public error class 稳定，`Cause()` 可由受控代码读取，但 raw cause 不进入 `errors.Is` tree，错误文本不泄露 payload/subject/endpoint。

### 12.4 path、并发与架构

- 成功 path 恰好一跳，其 rule/branch/target 与 decision 一致；policy/fact/as-of 由同一 decision envelope 关联；
- 返回 path 不可被调用方改写；
- 失败不返回半条 path；
- 并发求值没有共享请求态竞态，race test 通过；
- fuzz/表驱动测试覆盖非法枚举、revision、时间边界与 panic safety；
- Lottery 本节代码不 import Participation、Gin、SQL、Redis、React 或 Governance；
- 不新增 Migration、Redis key、HTTP route、runtime config、Compose 服务或 UI；
- 不新增通用 `Rule`、`RuleTree`、`Engine`、DSL、`map[string]any` 或 runtime priority；
- 现有 Strategy、Repository、WeightedSelector、ephemeral API 与前端相对第 26 节行为不变。

这些测试只能证明内核契约，不证明真实会员系统、在线门控、公开 API 或浏览器 E2E 已完成。

## 13. 威胁、性能、隐私与可观测边界

| 风险面 | 本节控制 | 尚未解决 |
| --- | --- | --- |
| 客户端伪造 tier/target | 输入契约不接收可信 tier/target；事实只从 reader 获取 | 尚无真实认证身份与 transport |
| provider 返回未知值 | 严格枚举、unsupported 失败关闭 | adapter 映射与告警尚未实现 |
| 陈旧事实被当新事实 | authority observed-at + 单一 as-of + max age | 跨系统原子快照、推送失效 |
| default 掩盖错误 | 只有 confirmed standard 可选 default | 未来新增 tier 的发布兼容流程 |
| 配置指向坏 Strategy | 仅校验 ID 非零 | 目标存在、发布、版本与引用完整性 |
| trace 泄露会员等级 | 最小字段、无 subject/原始 payload、禁止高基数 label | 持久化审计、访问控制、保留期 |
| 依赖尾延迟 | pre-cancel、一次读、无重复/fallback 请求 | adapter timeout budget、SLO、熔断 |
| 高并发共享状态 | router 保持只读、请求态局部化 | 真实 provider 容量与限流 |

本节不发布性能或可用性 SLO。性能基线只要求每次有效路由最多一次 Clock、一次会员事实读取、一次常量规模的本地分支；不得因 unknown/default 做隐藏重试或 fan-out。指标只使用 rule、outcome、error class 等稳定低基数维度，不能用 subject、Strategy ID、fact/policy revision 或会员详情作 label。

## 14. 风险账本与重决策触发器

| 未决风险 | 当前接受理由 | 重新决策触发器 |
| --- | --- | --- |
| 没有真实会员 adapter | 本节只证明领域路由边界 | 接入具体 provider、历史 as-of 或缓存 |
| 目标只校验非零 ID | 当前 Strategy 无业务 version，repository 接入不属本节 | Activity 发布需绑定可验证目标 |
| policy 只在内核显式提供 | 尚无运营编辑/发布需求 | 出现无代码编辑、审批、灰度、回滚 |
| 只有一跳决策 | 足以证明多出口和 default | 出现多层条件、共享子路径、合流或深度 |
| branch 泄露会员派生信息 | 当前 trace 不公开、不持久化 | trace 对外展示、持久化或跨租户查询 |
| logical as-of 非原子快照 | 当前没有跨 authority 事务能力 | 线上错误路由或事实水位不一致事故 |
| unknown 不可路由 | 安全关键事实未知时失败关闭 | 产品明确引入 guest/unclassified 业务等级 |

## 15. 后续章节停止线

### 第 27 节现在做

- Lottery 内一个具体会员等级事实契约；
- premium override + standard default 的确定路由；
- 单一 as-of、严格 freshness、失败关闭和取消优先；
- 一跳最小 path trace；
- 证明 Participation 线性 continue/reject chain 不能表达多目标 Route。

### 第 28 节再做

- 最小规则树持久化 schema；
- root、node、edge、default、target reference；
- schema version、发布前循环/深度/不可达/缺省/引用完整性验证。

### 第 29 节再做

- 只执行已验证、已发布图的规则引擎；
- 多步 path、步数/深度/时间预算、未知算子与执行失败语义；
- 不把第 27 节 concrete router 无证据扩成跨上下文万能引擎。

### 第 30 节再做

- Activity 生命周期和发布版本；
- Activity 对已发布 route policy/rule graph 与 Strategy 的版本化引用；
- 运行时启用/停用、时间窗与回滚语义。

### 第 31～35 节再做

- 真实会话认证与服务端 Principal；
- 统一资源、动作、范围与 RBAC 强制执行；
- 前端按服务端能力投影裁剪导航/路由/操作；
- 越权与浏览器 E2E。

会员等级永远不是管理员角色，路由成功永远不是访问授权。第 27 节不得为“以后方便”提前加入 `admin`、permission、tenant、menu 或前端可见性判断。

## 16. 参考基线

- [Lottery 业务规则需求基线 v1](lottery-rule-requirements-v1.md)
- [Participation 前置资格链基线 v1](participation-prerequisite-chain-v1.md)
- [ADR-0019：Lottery 规则所有权与评估边界](../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [ADR-0022：Participation 最小线性前置资格链](../decisions/ADR-0022-participation-prerequisite-chain.md)
- [ADR-0023：会员等级 Strategy 路由边界](../decisions/ADR-0023-membership-strategy-routing-boundary.md)
- [Go context package](https://pkg.go.dev/context)
