# 第 27 节：责任链为什么开始不够用了

> 第 26 节的 Participation 责任链只能表达“确定通过后进入固定下一项，确定拒绝或技术失败后终止”。本节引入第一个真实多出口需求：外部会员系统确认 `standard/premium` 事实，Lottery 根据自己的版本化策略选择 baseline 或 premium Strategy。我们保留原资格链，在 Lottery 内建立一个具体、一跳、失败关闭的会员路由切片，用两个成功出口、显式 default 和不可改写 path 证明线性 `next/stop` 模型的边界；本节仍不提前实现数据库规则树、通用决策引擎、Activity、权限或公开 API。

- **起点：** 第 26 节已验收 tip `47fc94d`
- **学习分支：** `codex/lesson-27-responsibility-chain-limits`
- **产品规格提交：** `2d7728a`
- **架构决策提交：** `57a3216`
- **domain 提交：** `076e399`
- **稳定分支修复提交：** `b307f1a`
- **application 提交：** `42caed9`
- **架构测试提交：** `2dc49b1`
- **产品规格：** [Lottery 会员等级 Strategy 路由基线 v1](../../product/membership-strategy-routing-v1.md)
- **架构决策：** [ADR-0023](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)
- **API 记录：** [第 27 节 API](../../api/lessons/lesson-27.md)
- **QA：** [第 27 节 QA](../../qa/lessons/lesson-27.md)
- **设计手记：** [第 27 节第一性原理手记](../../design-thinking/lessons/lesson-27.md)
- **面试问答：** [第 27 节面试问答](../../interview/lessons/lesson-27.md)

## 1. 本节真正交付了什么

本节交付的是尚未接 runtime 的 Lottery 内部路由切片：

```text
external membership authority
  -> MembershipTierFactReader
  -> MembershipTierFactSnapshot
       subject_ref / standard|premium / observed_at / source / revision
  -> one server Clock + max fact age
  -> MembershipStrategyRoutingPolicy
       premium target / baseline default target / policy revision
  -> MembershipStrategyRouteDecision
       rule / branch / target / provenance / evaluated-at / one-hop path
```

当前精确决策表是：

| 已确认事实 | 稳定 branch | 目标 | 结果 |
| --- | --- | --- | --- |
| `premium` | `premium_override` | premium target | 确定 Route |
| `standard` | `baseline_default` | baseline target | 确定 Route |
| zero / unknown / unsupported | 无 | 无 | 零 Route + error |
| missing / unavailable / corrupt / stale / future | 无 | 无 | 零 Route + error |

Route 只确认下一步应读取哪个 `StrategyID`。它不加载 Strategy、不调用 `WeightedSelector`、不创建 Draw，也不表示用户有资格、已授权、中奖或权益到账。

## 2. 为什么这是新增产品决定

第 23 节 LRR-003 只冻结了两个原则：

1. 会员分层路由必须输出明确 Strategy 目标；
2. 没有匹配时必须使用显式缺省策略，不能取 map 第一项。

它没有规定真实 tier 名称、premium 对应哪个 ID、default 是放行还是拒绝。第 26 节课程中的 `standard -> A / premium -> B` 只是下一节问题形状，不是已批准配置。

因此本节先写产品规格，再写代码，并明确新增：

- v1 只接受 authority 确认的 `standard` 与 `premium`；
- premium 有专用 override；
- standard 是当前明确批准使用 baseline default 的业务 tier；
- unknown/unsupported 不属于 default 集合；
- 两个 target 可以暂时相同，但 branch 证据必须不同。

这样 Git 历史能够区分“需求决定”与“实现选择”，避免事后把代码偶然形状伪装成产品要求。

## 3. 第 26 节 chain 能表达什么

第 26 节 private step 协议只有：

```text
eligible   -> index + 1
ineligible -> terminal business rejection
error      -> terminal technical failure
cancel     -> terminal caller cancellation
```

它非常适合所有 gate 都必须通过的 conjunction：

```text
new user AND risk passed
```

`eligible` 只是 continue；runner 完整遍历后才形成 aggregate eligible。顺序固定、tail reader 可证明零调用，trace 是已执行步骤的前缀。

这套模型没有 branch code、edge、target，也没有“成功但去不同终点”的语义。

## 4. 会员路由为什么不是第三个 eligibility gate

会员路由有两个都合法的成功结果：

```text
premium  -> continue to Strategy 200
standard -> continue to Strategy 100
```

若把它塞进原 chain，只剩几种坏办法：

- 给 `eligible` 增加 `eligible_premium` / `eligible_standard`；
- 把 Strategy ID 编进 reason code；
- 让 step 修改共享 context；
- 返回隐藏 `nextIndex`；
- 用 sentinel error 表示 route；
- 复制两条 chain，再复制公共尾部；
- 在 HTTP handler 另写没有 revision/path 的 `if`。

这些做法表面保留了 slice，实际上已经把图藏进控制流，并让 Participation 越权拥有 Lottery target。

本节的正确动作不是修改第 26 节，而是承认两个问题不同：

| 问题 | 决定所有者 | 输出 |
| --- | --- | --- |
| 主体是否满足参与前置条件 | Participation | eligible / ineligible / error |
| 已确认会员事实应进入哪个 Strategy | Lottery | branch + Strategy target / error |

## 5. 事实所有权与决定所有权

外部会员 authority 拥有：

- 谁属于哪个会员等级；
- 等级何时被观察/确认；
- source 与 source revision；
- 等级升级、降级与生命周期。

Lottery 拥有：

- 哪个业务 tier 使用哪个 branch；
- premium/default target；
- policy revision；
- stable rule/branch/reason code；
- Route decision 与最小 path。

外部系统不能返回 GrowthOS `StrategyID`，Lottery 也不能写会员等级。把两者分开，才能在 Strategy 变化时不修改会员协议，在会员模型变化时让 Lottery adapter 明确处理兼容，而不是静默 fallback。

## 6. 为什么新增 Lottery 本地 `MembershipSubjectRef`

`MembershipSubjectRef` 是 Lottery 读取外部会员事实所需的 opaque key。它刻意不叫：

- `UserID`；
- `PrincipalID`；
- `ParticipantRef`；
- `TenantMemberID`。

非零 reference 只证明值的形状合法，不证明调用者就是该主体，也不证明租户、会话、角色或对象权限。直接复用 Participation 的 `ParticipantRef` 会让两个 bounded context 在没有映射契约时共享身份含义；直接叫 Principal 又会提前冒充第 31～35 节的认证授权能力。

## 7. `MembershipTierFactSnapshot` 的最小契约

构造器位于 [membership_fact.go](../../../internal/lottery/domain/membership_fact.go)：

```go
domain.NewMembershipTierFactSnapshot(
    subjectRef,
    tier,
    observedAt,
    source,
    revision,
)
```

必须满足：

- subject ref 非零；
- tier 精确为 `standard` 或 `premium`；
- observed-at 非零并 canonicalize 为 UTC；
- source/revision 非空、trim canonical、合法 UTF-8、可打印且有字节上限；
- 快照不包含 Strategy target 或 Lottery route verdict。

字段私有，构造成功后得到不可变值。adapter 遇到未知 provider tier 时不能偷偷构造 standard；它必须返回映射错误，由 application 安全归类为 invalid fact。

## 8. 为什么 v1 使用封闭 tier，而不是开放 string

开放 token 的优点是会员系统增加等级时 Lottery 不必先升级数据类型；风险是一个刚出现的 `gold` 会在旧 consumer 中自动命中 default，产品可能在不知情时把高价值人群放进普通奖池。

当前没有 provider schema 协商、兼容矩阵、发布审批或动态 policy，所以本节选择封闭 `standard/premium`：

- 新 tier 会显式失败；
- adapter 必须暴露 mapping invalid；
- 产品决定新 tier 是否进 baseline 后，再新增 policy revision 和测试；
- 不把 typo、未来值或不完整 payload 当普通会员。

这是可用性与安全性的主动权衡。未来若产品明确要求开放 vocabulary，应新增 ADR，而不是只给 `Validate` 加一项。

## 9. `MembershipStrategyRoutingPolicy` 为什么仍是具体类型

[membership_routing_policy.go](../../../internal/lottery/domain/membership_routing_policy.go) 只包含：

```text
policy revision
premium target StrategyID
baseline default StrategyID
```

不使用 `map[string]StrategyID`，因为：

- map miss 容易被误当 default；
- map 第一项没有确定顺序；
- 当前只有一个真实 override，不需要运行时集合；
- 具体字段让缺失 target 在构造时明确失败；
- 第 28 节才会为真实树定义持久化 node/edge/default。

两个 target 不强制不同。灰度期间它们可能暂时汇聚到一个 Strategy；branch 仍说明“为什么到达这里”，目标相同不能抹掉决策路径。

当前构造器只验证一份 policy value 内部完整，尚无 repository/publisher 阻止调用方用同一 revision 字符串构造不同 targets。因此本节能证明的是“同一完整 policy snapshot（revision + targets）”确定，而不是“revision 字符串天然是内容哈希”。第 28 节持久化与第 30 节发布绑定必须补上 revision 唯一性/不可变内容约束。

## 10. 稳定 code 为什么需要逐字测试

稳定契约包括：

```text
rule   = lottery.membership_tier.route_strategy
branch = premium_override | baseline_default
reason = premium_strategy_selected | baseline_strategy_selected
```

第一次代码审查发现实现曾使用过缩写 branch 字面值，而已批准文档冻结的是完整 code。只比较常量的测试无法发现“常量和值一起改”的漂移，因此 [membership_routing_test.go](../../../internal/lottery/domain/membership_routing_test.go) 增加 literal contract test。

稳定 code 可以进入低基数统计和历史证据；文案可以修改。code 一旦进入持久化或外部消费就不能改义复用。

## 11. 纯领域路由怎样工作

[RouteMembershipStrategy](../../../internal/lottery/domain/membership_routing.go) 的顺序是：

1. 验证 policy；
2. 验证 fact；
3. canonicalize evaluated-at；
4. 拒绝 future fact；
5. 对 `standard/premium` 做显式 switch；
6. 构造唯一 branch、target、reason 与一跳 path。

显式 switch 的 default 分支仍返回错误。虽然 `fact.Validate()` 当前已经拒绝其他 tier，但这层防御避免未来有人只扩充 tier 枚举、忘记同时更新路由，就让新 tier 静默走 baseline。

该函数是纯计算：不读 Clock、不查数据库、不访问 Redis、不加载 Strategy、不产生随机数。

## 12. decision 与 path 的边界

`MembershipStrategyRouteDecision` 保存：

- target；
- rule/branch/reason；
- policy revision；
- fact source/revision；
- evaluated-at；
- 一跳 path。

path step 只重复实际路径需要的 `rule + branch + target`；policy/fact/as-of 由外层 decision envelope 关联，避免每一跳重复。`Path()` 返回 slice 副本，调用方修改副本不会改写决定。

decision 故意不保存：

- subject ref；
- 原始 provider tier payload；
- 姓名、手机号、订单或成长值；
- provider endpoint/raw error；
- 未走分支；
- 随机票据或 Award。

`Confirmed()` 用于区分完整 Route 与技术失败返回的零值，但不能把任意手工拼接值变成正式审计证据；字段私有且正常调用只能由 evaluator 构造。

## 13. application 为什么在读事实前捕获时刻

[MembershipStrategyRoutingService](../../../internal/lottery/application/membership_routing.go) 的执行顺序是：

```text
input/policy/service validation
  -> pre-cancel check
  -> Clock.Now exactly once
  -> canonical UTC evaluated-at
  -> cancellation check
  -> FindMembershipTierFact exactly once
  -> caller cancellation wins
  -> provider error classification
  -> fact/subject/future/freshness validation
  -> cancellation check
  -> pure domain route
  -> cancellation check
  -> confirmed decision
```

先取时刻让 reader 返回的事实必须属于这个 logical as-of。若在读取后取时刻，慢 I/O 会给旧事实额外续命；若每一步各读时钟，边界请求会出现混合时间。

本节不导出 `RouteAt(time.Time)`，因为 transport/caller 传入旧时间可能让 stale fact 看起来仍新鲜。

## 14. freshness 的纳秒边界

freshness 使用 authority 的 `observed_at`：

```text
age = evaluated_at - observed_at
```

规则是：

- `observedAt == evaluatedAt` 合法；
- `observedAt == evaluatedAt + 1ns` 为 future；
- `age == maxFactAge` 合法；
- `age == maxFactAge + 1ns` 为 stale。

adapter 的读取时间不能替代 observed-at，否则缓存中一小时前的会员等级每次查询都会被重新盖成“刚刚确认”。

单一 as-of 不是跨系统原子快照。它只让本次决定能够解释“依据哪个逻辑时刻”，不能证明 provider 支持历史读取，也不能消除 policy 与会员事实之间的 TOCTOU。

## 15. caller cancellation 为什么优先

服务在 Clock 与 reader 返回后都先看 caller context：

- pre-cancel：Clock/reader 都零调用；
- Clock callback 后 cancel：Clock 一次、reader 零次；
- reader 执行中 caller cancel：reader 返回后原样返回 caller error；
- provider 自己 deadline，但 caller 仍活跃：归类为会员 authority unavailable；
- fact 与 provider error 同返：error 胜，fact 丢弃。

context 不能强制一个不遵守取消的 reader 立即返回，所以测试使用 blocking reader 证明“依赖一旦返回，caller cancellation 先于依赖错误”。生产 adapter 仍必须遵守 ctx 和 timeout budget。

## 16. 为什么读取错误使用显式 `Cause()`

[membership_routing_error.go](../../../internal/lottery/application/membership_routing_error.go) 的 wrapper 只让 `errors.Is` 匹配一个稳定 application class：

```text
not found
unavailable
read failure
invalid provider payload
```

raw cause 不实现 `Unwrap()`，只通过显式 `Cause()` 给受信诊断代码。这是因为 Go 的错误包装会成为 API 契约：一旦底层 provider sentinel 进入公共 error tree，调用方会依赖其实现细节。

`Error()` 不渲染 endpoint、payload、subject 或 provider 文本。unknown class、零 wrapper 与 typed-nil wrapper 都失败关闭到 read failure。

## 17. 为什么 provider mapping error 要归 invalid

领域快照字段私有，构造器会拒绝 unknown tier、空 source 或坏 revision。真实 adapter 遇到这些 payload 时无法返回一个“坏但可检查”的非零 snapshot，只能返回构造/映射 error。

如果 application 把所有此类 error 都归为普通 read failure，那么产品规格中的 `unsupported/corrupt -> invalid` 实际不可达。因此 classifier 显式将：

- `domain.ErrMembershipTierFactInvalid`；
- `domain.ErrMembershipSubjectRefRequired`；
- 已审查的 application invalid class

映射成安全的 `ErrMembershipTierFactInvalid`。raw domain class 只在 `Cause()` 中保留，不进入公共 `errors.Is` tree，也绝不会让同时返回的 standard fact命中 baseline。

## 18. typed-nil 为什么必须在组合期处理

Go interface 可能自身非 nil、内部却装着 nil pointer 或 nil function。只写：

```go
if reader == nil { ... }
```

不能阻止稍后 panic。Lottery application 已有 `dependencyIsNil`，构造器和 `Validate()` 复用它检查：

- nil reader；
- typed-nil reader pointer；
- nil clock；
- typed-nil clock pointer；
- typed-nil `MembershipRoutingClockFunc`；
- 非正 max age；
- 手工创建的 zero/partial service。

无效配置在任何 Clock/fact I/O 前失败，避免 readiness 正常而业务请求 panic。

## 19. 可执行测试怎样证明链真的不够

关键测试不是“写一段文字说有两个分支”，而是同一 policy 下执行两个事实：

```text
standard -> baseline_default -> Strategy 100
premium  -> premium_override -> Strategy 200
```

两者都是 confirmed success，却有不同 target/path。原 chain 的 `eligible -> next sequential step` 无法携带这种选择。

另一个测试让两个 branch 都指向 Strategy 100：target 相同但 branch 不同。这证明 path 是决策原因，不只是终点的别名；未来树出现合流时不能靠最终 ID 反推路径。

## 20. 测试矩阵

### 20.1 domain

- fact UTC canonicalization 与 provenance；
- zero/unsupported tier、zero time、unsafe/oversize token；
- policy revision、zero target、允许 target 合流；
- standard/premium 两个出口；
- stable literal code；
- zero policy/fact/as-of、future 1ns；
- path defensive copy；
- equivalent timezone 与重复求值确定性；
- fuzz 保证 unsupported tier 不会被接受。

### 20.2 application

- `clock -> reader` 固定顺序，各一次；
- exact max age 与 stale +1ns；
- future +1ns、subject mismatch、zero fact；
- fact+error 时 error 胜；
- provider deadline 与 caller deadline 分开；
- provider mapping error 安全归 invalid；
- nil/typed-nil/partial service；
- pre/clock/reader/blocking-reader cancellation；
- 64 并发只读调用；
- safe Error/Is/Cause 与 secret 不泄露。

### 20.3 architecture

[membership_routing_architecture_test.go](../../../internal/lottery/application/membership_routing_architecture_test.go) 检查：

- Lottery domain 不 import 项目包；
- Lottery application 只 import Lottery domain；
- Participation domain/application 仍不 import Lottery；
- 不声明 `Rule/RuleTree/RuleEngine/DecisionEngine/EvaluationContext/DSL` 等通用类型；
- 不声明泛型规则类型；
- 不声明 `map[string]any` fact bag。

它是可执行 guard，不是形式化证明；同义词或语义上的过度抽象仍需代码审查。

## 21. 验证命令

定向验证：

```bash
go test -count=1 ./internal/lottery/...
go test -race -count=1 ./internal/lottery/...
go test -shuffle=on -count=20 \
  ./internal/lottery/domain \
  ./internal/lottery/application
go vet ./internal/lottery/...
```

定向 fuzz：

```bash
go test ./internal/lottery/domain \
  -run='^$' \
  -fuzz='^FuzzMembershipTierFactSnapshotNeverAcceptsUnsupportedTier$' \
  -fuzztime=10s
```

全仓门禁：

```bash
make verify
```

范围负证：

```bash
git diff --name-only 47fc94d..HEAD
git diff 47fc94d..HEAD -- \
  cmd deploy migrations web scripts configs go.mod go.sum \
  internal/participation \
  internal/lottery/adapter
```

第二条命令预期无输出。本节没有 runtime/schema/adapter/UI 变化，因此不会伪造 Compose 或浏览器 E2E。

## 22. 为什么本节仍不用数据库、Redis 或规则引擎

本节已证明的只有：

- 一个分支条件；
- 两条成功边；
- 一个显式 default；
- 一个 terminal target；
- 一跳 path。

它尚未证明：

- root/node/edge 怎样持久化；
- 图能否有环；
- default 是否必填；
- 不可达节点怎样处理；
- Strategy 引用怎样校验；
- schema/version/publish 怎样演进；
- 多规则命中冲突怎样解决；
- 执行深度、步数和时间预算；
- 运营审批、灰度、回滚或模拟。

因此第 27 节只实现 concrete router。现在上数据库/Redis/OPA/DMN 自研解释器，只会让技术 schema 先于真实拓扑。

## 23. 为什么本节没有 API 或前端

目前没有：

- 真实会员 adapter；
- 可信 session/Principal；
- Activity/Participation/Draw 正式资源；
- 已发布 routing policy；
- target Strategy 发布/版本验证；
- 服务端访问控制。

若现在公开 `subject_ref` 或让浏览器提交 tier，任何人都能选择他人身份或 premium 分支。若直接接现有 ephemeral selection，又会让开发演示 route 看起来像正式会员抽奖。

所以 `/health`、`/ready`、ephemeral selection route、DTO/header/status、React 导航和 Compose 都不变。第 27 节的 E2E 证据是“不存在 runtime 变化”的负 diff，不是浏览器截图。

## 24. 性能、隐私与可观测边界

当前每次有效调用最多：

- 一次 Clock；
- 一次会员 fact read；
- 一次常量规模 switch；
- 一跳本地 path 分配。

没有 retry、fan-out、repository load 或随机选择。并发安全依赖 service 无请求级共享写状态，以及注入 reader/clock 自身的并发契约。

普通指标只应使用稳定低基数 rule/outcome/error class；不能用 subject、Strategy ID、fact/policy revision 或原始 tier payload作 label。branch 是会员派生信息，即使低基数也需披露评审，不能因为方便就进入公开页面或跨租户日志。

本节未发布真实 latency/QPS/SLO；单测调用次数不能冒充线上性能数据。

## 25. 当前尚未完成什么

- 没有真实 membership provider adapter；
- 没有身份到 MembershipSubjectRef 的可信映射；
- 没有 policy repository、发布、审批、灰度或回滚；
- 没有验证 target Strategy 存在、发布或版本；
- 没有 Activity 绑定；
- 没有组合 Participation chain；
- 没有公开 HTTP/MCP/Agent route；
- 没有 session、RBAC/ABAC、对象权限或 tenant scope；
- 没有 Strategy load、WeightedSelector、Draw、库存或发奖；
- 没有 trace 持久化、审计或 OpenTelemetry；
- 没有浏览器/Compose E2E；
- 没有跨 authority 原子快照。

## 26. 简历与口述边界

可以准确表述：

> 在 Lottery domain/application 中实现基于权威会员快照的确定性 Strategy 路由，用 premium override、standard baseline default 和一跳不可变 path 证明线性资格责任链无法表达多成功出口；通过单一服务端 as-of、纳秒 freshness、caller cancellation 优先、typed-nil 防护、安全错误 Cause、并发 race/fuzz 与架构 guard 保证未知事实不 fallback，并保持 Participation、Selector、HTTP、持久化与权限边界不变。

不能表述：

- 已实现通用规则树或规则引擎；
- 已接入真实会员系统；
- 已支持任意会员等级动态配置；
- 已把公开 Lottery API 路由到会员 Strategy；
- 已有 Activity 发布或正式 Draw；
- 已有认证、权限或前端角色页面；
- 已持久化/审计 decision path；
- 已做线上性能压测或浏览器 E2E。

## 27. 第 28 节为什么自然出现

第 27 节已经出现真实拓扑词汇：

```text
decision root
  -> premium_override edge -> Strategy target
  -> baseline_default edge  -> Strategy target
```

下一步不应把更多 `if`、hidden index 或 map 堆进 concrete router，而要回答配置怎样成为合法、可发布、可恢复的数据：

- root、node、edge、default 与 target reference；
- schema version 与 immutable revision；
- 唯一 root、完整 default；
- edge/target 引用完整性；
- cycle、depth、不可达节点与重复 branch；
- 草稿/发布边界与迁移回滚。

这正是第 28 节“规则树第一次数据库升级”的范围。第 28 节只建立持久模型和校验，不急着执行任意图；第 29 节才在已验证配置上实现决策引擎、步数/时间预算和多步 path。

## 28. 本节复盘

本节的核心不是把责任链评价为“坏设计”，而是准确限定其适用域：

1. Participation 线性 chain 继续服务所有必经 gate；
2. 多个合法成功出口需要显式 branch/target；
3. 外部会员事实与 Lottery 路由决定保持不同所有者；
4. default 只覆盖批准集合，不吞 unknown 或故障；
5. stable code、path 与 provenance 让决定可解释；
6. single as-of、freshness、cancel 与 error class让 Route 可相信；
7. 架构测试守住 Participation/Selector/adapter/runtime 停止线；
8. 真实一跳路由为第 28 节的 tree schema 提供了必要词汇和失败证据。

渐进架构不是预先猜终态，而是在现有模型开始出现隐藏跳转前，用可执行业务反例升级表达能力，并把下一次升级所需的新问题继续留给下一节。
