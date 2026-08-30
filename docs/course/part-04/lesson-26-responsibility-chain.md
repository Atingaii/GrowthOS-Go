# 第 26 节：责任链实现前置规则

> 第 25 节只有一条新用户 evaluator，因此拒绝了 `Rule` 接口。本节先增加第 23 节已经登记的第二条真实 Participation 规则——风险 screening 准入——再由两个具体规则反推一个固定、线性、串行、失败关闭的 ordered gate chain。它在一次受控逻辑 evaluated-at 下执行“新用户 → 风险准入”，只有前一步确定通过才读取后一步事实；拒绝、技术失败和 caller cancellation 都立即终止。它还不是规则树、数据库规则、Activity、权限系统或正式抽奖门控。

- **起点：** 第 25 节已验收 tip `8b2f3a6`
- **学习分支：** `codex/lesson-26-responsibility-chain`
- **规格提交：** `974a61b`
- **ADR 提交：** `5271963`
- **domain 提交：** `ad25dbe`
- **application 提交：** `f77e17f`
- **产品规格：** [Participation 前置资格链基线 v1](../../product/participation-prerequisite-chain-v1.md)
- **架构决策：** [ADR-0022](../../decisions/ADR-0022-participation-prerequisite-chain.md)
- **API 记录：** [第 26 节 API](../../api/lessons/lesson-26.md)
- **QA：** [第 26 节 QA](../../qa/lessons/lesson-26.md)
- **设计手记：** [第 26 节第一性原理手记](../../design-thinking/lessons/lesson-26.md)
- **面试问答：** [第 26 节面试问答](../../interview/lessons/lesson-26.md)

## 1. 这一节真正交付了什么

这一节交付的是一个可执行、但尚未接公开入口的 Participation 资格组合内核：

```text
EligibilityPrerequisiteChain
  ├─ RegistrationFactReader
  │    └─ NewUserPolicy -> NewUserEligibilityDecision
  ├─ RiskScreeningFactReader
  │    └─ RiskAdmissionPolicy -> RiskAdmissionDecision
  └─ one Clock -> one logical evaluated-at

fixed plan:
  new-user eligible   -> read risk fact
  new-user ineligible -> confirmed ineligible, risk reader 0 calls
  new-user error      -> zero aggregate + error, risk reader 0 calls
  risk passed         -> confirmed aggregate eligible
  risk blocked        -> confirmed aggregate ineligible
  risk error          -> zero aggregate + error
```

源码新增：

- `RiskScreeningFactSnapshot`：风险 authority 的最小、不可变事实；
- `RiskAdmissionPolicy`：Participation 对风险事实的准入口径；
- `RiskAdmissionDecision` 与纯 `EvaluateRiskAdmission`；
- `RuleSetRevision`：线性组合版本，与单规则 policy/fact/app revision 分离；
- `RiskScreeningFactReader`：由消费方拥有的窄端口；
- `EligibilityPrerequisiteChain`：固定两步的 ordered gate chain；
- `EligibilityTraceStep` 与 `PrerequisiteEvaluation`：有界内部执行证据；
- `evaluationInstant`：只能在 application 包内由受控 Clock 构造的逻辑时刻 token；
- 风险读取错误的稳定分类，以及注册错误诊断从 `Unwrap` 到显式 `Cause()` 的加固。

## 2. 为什么不能只给旧 evaluator 套一层 slice

如果第 25 节就写：

```go
type Rule interface {
    Evaluate(map[string]any) (bool, error)
}
```

它无法回答：

1. 两个真实消费者的共同输入究竟是什么；
2. `true` 是“该节点继续”还是“整体最终 eligible”；
3. 哪条规则先执行，为什么；
4. 业务拒绝与事实未知怎样区分；
5. 何时读取外部事实；
6. 是否共享一个时间；
7. trace 可以记录哪些字段；
8. Authorization、Inventory 是否会被错误塞进同一个接口。

第 26 节先让第二条需求成立，再只抽共同事实。这是 Go 官方“接口通常由消费方定义，并在真实使用出现后抽取”的项目化实践，而不是为了背设计模式画类图。

## 3. 为什么第二条规则选择风险准入

第 23 节的业务句子已经要求：用户满足新用户定义、仍有次数，且没有被风险判断拒绝。三个候选不能随便实现：

| 候选 | 是否现在做 | 原因 |
| --- | --- | --- |
| 风险 screening 准入 | 是 | 已登记真实需求；只读；同属 Participation；第二 authority 让短路具有隐私与成本价值 |
| 剩余次数 | 否 | 可并发消费；只有 `remaining > 0` 快照会产生 TOCTOU；账户/流水/预占在第 39～45 节 |
| Activity 时间窗 | 否 | Marketing 拥有发布门控；Activity 到第 30 节才建模 |
| 会员等级 | 留给第 27 节 | 它产生 route，不只是 continue/reject，正好暴露线性链局限 |
| 角色权限 | 否 | 需要真实 Principal、资源、动作和范围；统一访问控制在第 31～35 节 |

技术模式不能决定业务需求。选风险准入的原因是它已经在事实台账中存在，而且不会提前伪造可消费余额、Activity 或认证。

## 4. 风险事实与风险准入不是同一个决定

风险 provider 拥有：

```text
participant_ref
passed | blocked
assessed_at
source
source_revision
```

Participation 拥有：

```text
在 risk-admission policy revision 下，
该 screening fact 是否允许进入当前 Participation 前置流程。
```

这样做不是形式主义。若把 provider 的 `passed` 直接称作“GrowthOS 用户有资格抽奖”，外部系统就越权拥有了 Activity、次数、其他资格和最终抽奖流程。相反，Participation 只消费最小 verdict，不复制：

- 风险分数；
- 设备指纹；
- 模型特征；
- 阈值；
- 原始 payload；
- 完整用户资料；
- provider 内部文案。

## 5. `RiskScreeningFactSnapshot` 的不变量

构造器：

```go
domain.NewRiskScreeningFactSnapshot(
    participantRef,
    disposition,
    assessedAt,
    source,
    revision,
)
```

必须满足：

- ParticipantRef 非零；
- disposition 只能是 `passed` 或 `blocked`；
- assessed-at 非零并 canonicalize 为 UTC、去掉 monotonic 部分；
- source/revision 非空、trim canonical、UTF-8 合法、可打印且有字节上限；
- revision 不得编码 PII 或原始 payload。

`pending`、`unknown`、空值都不是一个可偷偷映射为 `passed` 的领域状态。它们应由 adapter 作为无法形成可信快照的技术类别处理。

## 6. freshness 为什么看 `assessed_at`

风险结论的年龄从 authority 真正形成 verdict 的时刻计算：

```text
age = logical_evaluated_at - assessed_at
```

读取时刻不能给旧 verdict 续鲜。错误示例：

```text
数据库里是一小时前的 blocked/passed
adapter 每次查询后写 observed_at = time.Now()
-> 旧结论永远不会 stale
```

本节规定：

- `age == maxAge` 仍有效；
- `age == maxAge + 1ns` 即 stale；
- `assessedAt > evaluatedAt` 是 future fact，失败关闭；
- stale blocked 也不能当作当前确定拒绝，因为我们没有可信证据说明它仍有效；
- stale passed 更不能继续下游。

## 7. 风险准入纯领域规则

固定 rule code：

```text
participation.risk.screening_admission
```

稳定 reason：

```text
risk_screening_passed
risk_screening_blocked
```

纯 evaluator：

```go
decision, err := domain.EvaluateRiskAdmission(policy, fact, evaluatedAt)
```

确定映射：

| fact | outcome | reason |
| --- | --- | --- |
| `passed` | `eligible` | `risk_screening_passed` |
| `blocked` | `ineligible` | `risk_screening_blocked` |

这里的单节点 `eligible` 只是“该 gate 可以继续”。直到组合器确认全部必经 gate 通过，才产生 aggregate eligible 与：

```text
all_prerequisites_satisfied
```

## 8. 为什么使用一个 chain-wide evaluated-at

若两个节点各自 `Clock.Now()`：

```text
T1 new-user: eligible
T2 risk: stale/valid
```

同一个请求会混合两个时间切片，边界附近难以重放。若先并行读齐事实再捕获时间，非新用户也会访问风险源，短路只剩代码外观。

本节选择：

1. 完整校验 context、participant、ruleset、两个 policy 和所有依赖；
2. pre-cancel 检查；
3. chain Clock 调用一次，得到 canonical UTC logical as-of；
4. 读取并评估 registration fact；
5. 只有 confirmed eligible 才读取风险 fact；
6. 两个 decision/trace step 必须带同一个 evaluated-at。

source 在 as-of 后产生的新快照本次不用，下一次评估再读取。当前 reader 端口没有伪造“历史查询”能力；它返回 latest 后由 application 检查 source-owned time 不晚于 as-of。未来真实 provider 若不能满足，就应返回 unavailable/as-of-miss，而不是把未来时间 clamp 回去。

`evaluationInstant` 是包内私有 token，避免未来 HTTP handler 传一个很旧的裸 `time.Time` 来绕过 freshness。

## 9. 固定顺序的业务理由

顺序是：

```text
participation.new_user.registered_on_or_after
participation.risk.screening_admission
```

原因：

- 注册事实通常敏感度较低；
- 新用户规则拒绝后，风险结论不可能把整体改为 eligible；
- 不访问不必要的风险 authority 符合数据最小化；
- 降低风险系统调用成本与故障传播；
- 对外首要业务拒绝 reason 稳定；
- 测试可以直接证明 risk reader 零调用。

“便宜的永远放前面”不是普遍定律。如果将来规则存在业务依赖或更强隐私要求，顺序应重新决策。正因为顺序影响 reason、I/O 和故障域，它属于 ruleset revision，而不是普通代码重排。

## 10. `RuleSetRevision` 为什么独立

至少有五类版本：

| 版本 | 回答什么 |
| --- | --- |
| RuleCode | 哪条稳定业务规则 |
| PolicyRevision | 这条规则用哪份不可变政策 |
| FactRevision | authority 提供的哪份事实 |
| RuleSetRevision | 哪些规则按什么顺序组合 |
| application build | 哪份二进制代码 |

第 26 节把 `RuleSetRevision` 建成有界 opaque value，但没有建立数据库规则集、schema version 或发布系统。`participation-prerequisites-v1` 只标识当前代码内固定 plan，不意味着运营可以动态配置。

## 11. 责任链的最小实现

`EligibilityPrerequisiteChain` 直接拥有：

```text
RegistrationFactReader
RiskScreeningFactReader
Clock
maxRegistrationFactAge
maxRiskScreeningAge
```

构造器拒绝：

- nil / typed-nil reader；
- nil / typed-nil clock；
- zero/negative freshness；
- 手工构造的 zero/partial chain。

Evaluate 内部建立两个 private `prerequisiteStep`：

```go
type prerequisiteStep struct {
    code     domain.RuleCode
    evaluate func(context.Context) (EligibilityTraceStep, error)
}
```

这不是导出的插件协议。runner 只做固定 for-loop，并验证 concrete step 返回的 code、outcome、revision、provenance 和 evaluated-at。没有：

- `Rule`；
- 泛型；
- `map[string]any` fact bag；
- priority；
- 动态 sort；
- route；
- tree/graph；
- JSON DSL；
- 第三方 engine。

## 12. 四种语义为什么不能用一个 bool

| 语义 | 含义 | 是否继续 |
| --- | --- | --- |
| step eligible | 当前 gate 已确认通过 | 有下一节点则继续 |
| confirmed ineligible | 事实充分，业务条件不成立 | 停止，error 为 nil |
| technical inability | 事实/依赖/clock/配置不能形成可信决定 | 停止，零 aggregate + typed error |
| cancellation | caller 不再需要或 deadline 到期 | 停止，零 aggregate + context error |

若函数只返回 `bool`：

- `false` 不知道是 blocked、stale、not-found 还是 timeout；
- `true` 不知道是一个节点通过还是整体通过；
- fail-open 很容易藏在默认值；
- retry、指标、用户文案和审计全部失真。

## 13. 失败关闭并不等于“判定不合格”

本节的 fail-closed 是：

```text
无法可信判断 -> 不允许继续后续动作
```

而不是：

```text
无法可信判断 -> 伪造 ineligible 业务结果
```

因此：

- business reject：confirmed aggregate + nil error；
- dependency/invalid/stale：zero aggregate + typed error；
- cancel/deadline：zero aggregate + context error。

`PrerequisiteEvaluation.Confirmed()` 为调用方提供显式检查，但 Go 调用方仍必须先处理 error。

## 14. 为什么错误诊断改用 `Cause()`

第 25 节的 `RegistrationFactReadError` 同时实现稳定 class `Is` 和 diagnostic `Unwrap`。若违规 adapter 把另一个 application sentinel 放进 cause，`errors.Is` 会遍历完整链并可能同时匹配两个 class。

第 26 节在进入多节点组合时关闭这一风险：

- `Error()` 只渲染经过评审的稳定 class；
- `Is()` 只匹配唯一 class；
- diagnostic cause 通过显式 `Cause()` 供受信代码读取；
- `Cause()` 不进入标准 `errors.Is` 树；
- raw secret、SQL、上游地址和 payload 不进入普通错误文本。

风险读取错误从一开始采用相同契约。unknown provider error 收敛为 `read failed`；provider 自身 deadline 在 caller context 仍存活时归为 unavailable，但底层 deadline 只留在显式诊断 cause。

## 15. Context 与取消优先级

chain 在以下位置检查 `ctx.Err()`：

1. 任何 clock/reader 前；
2. clock 返回后；
3. 每个 reader 返回后；
4. concrete evaluator 返回后；
5. 进入下一个节点前。

若 reader 返回错误的同时 caller cancellation 已可见，caller context 胜出。context 不存进 struct，不放业务 fact 到 `context.Value`，也不为抢占一个不遵守 context 的同步 reader 泄漏 goroutine。

## 16. 内部 trace 记录什么

`PrerequisiteEvaluation.Steps()` 返回 copy，调用方修改切片不能篡改已形成的证据。每项只包含：

```text
rule_code
outcome
reason_code
policy_revision
fact_source
fact_revision
evaluated_at
```

只记录实际执行节点。新用户拒绝时 trace 长度为 1，不能伪造第二节点“skipped success”。trace 不含：

- ParticipantRef；
- 风险分数、阈值或特征；
- raw error；
- 用户文案；
- 随机材料；
- HTTP request；
- session/cookie/token。

FactRevision 只能进入受控内部诊断，不是普通 metric label 或公开 DTO。当前 trace 是进程内 value，不是持久审计、OTel span 或正式 Draw 回放。

## 17. 测试怎样证明短路是真的

只断言最终 outcome 不够。专项测试直接观察依赖调用次数：

| 场景 | registration | risk | clock | 结果 |
| --- | ---: | ---: | ---: | --- |
| pre-cancel | 0 | 0 | 0 | zero + context error |
| zero clock | 0 | 0 | 1 | zero + clock error |
| registration reject | 1 | 0 | 1 | confirmed ineligible，1 step |
| registration error/cancel | 1 | 0 | 1 | zero + error |
| both pass | 1 | 1 | 1 | confirmed eligible，2 steps |
| risk blocked | 1 | 1 | 1 | confirmed ineligible，2 steps |
| risk stale/future/error | 1 | 1 | 1 | zero + error |

还验证：

- exact max-age 有效、`+1ns` stale；
- post-as-of `+1ns` invalid；
- 不同 ParticipantRef invalid；
- fact 与 error 同时返回时 error 路径优先；
- blocking risk reader 期间修改 fake clock，不会重读或漂移 as-of；
- trace 两步 evaluated-at 完全相等；
- trace copy 防外部修改；
- typed-nil 与手工 partial chain fail closed；
- 64 goroutine 共享 immutable chain，race 通过；
- `-shuffle=on -count=20` 不依赖测试顺序；
- domain fuzz seed 与定向 fuzz 覆盖 assessed-at 边界；
- AST 架构门禁阻止跨上下文 import、泛型、已知通用引擎名和 string-any fact bag。

## 18. 怎样运行本节验证

快速验证：

```bash
go test -count=1 ./internal/participation/...
go test -race -count=1 ./internal/participation/...
go test -shuffle=on -count=20 ./internal/participation/application
go vet ./internal/participation/...
```

定向 fuzz：

```bash
go test ./internal/participation/domain \
  -run='^$' \
  -fuzz='^FuzzEvaluateRiskAdmissionAssessedAtBoundary$' \
  -fuzztime=10s
```

全仓门禁：

```bash
make verify
```

负向 diff：

```bash
git diff --name-only 8b2f3a6..HEAD
git diff 8b2f3a6..HEAD -- \
  cmd internal/lottery migrations deploy web scripts configs go.mod go.sum
```

预期第二条命令无输出。本节没有 runtime/schema/transport/React 变化，因此不伪造 Compose 或浏览器 E2E。

## 19. 架构测试的真实能力边界

AST 测试证明 Participation 生产文件：

- domain 不 import 项目包；
- application 只 import Participation domain；
- 不声明 exact `Rule`、`RuleChain`、`RuleEngine`、`RuleTree`、`Specification`、`EvaluationContext`、`RulePriority` 或 `DSL`；
- 不声明类型参数；
- 不声明 `map[string]any` / `map[string]interface{}` fact bag。

它不能证明语义上绝无过度抽象，也不能识别任意同义命名。代码评审仍要检查固定 plan 是否被偷偷变成动态平台。文档不能把 AST test 描述为形式化架构证明。

## 20. 为什么本节没有 HTTP、MySQL、Redis 或 UI

没有真实会话时，公开 `participant_ref` 只是可伪造用户选择器；没有 Activity 时，ruleset/policy 没有正式业务资源；没有生产 fact adapter 时，curl fixture 也不等于权威事实。把 chain 接到现有 ephemeral Lottery API 会制造“资格已保护”的假象。

所以本节保持：

- Migration latest = 2；
- MySQL grant 不变；
- Redis 仍只缓存 Strategy projection；
- `/health`、`/ready`、ephemeral route 全部不变；
- React 页面与导航不变；
- Compose 拓扑不变；
- `WeightedSelector` 仍无用户、风险、Activity 或权限依赖。

本节只能声称 risk tail 被短路，不能声称 selector 已经零调用，因为 selector 根本没有进入这条链。

## 21. 它与经典责任链哪里相同、哪里不同

相同点：

- 请求依次交给有序 handler；
- 当前 handler 决定是否继续；
- 调用者不手写每个终止分支；
- 节点可独立测试。

不同点：

- 经典 CoR 常表示“多个候选处理者，某个处理后停止”；
- 本项目是“每个必经 gate 都要通过，reject/error 即停止”；
- 节点不拥有 next 指针，不动态改链；
- concrete policy 仍保持各自领域类型；
- 它更准确地说是 ordered eligibility gate chain / CoR variation。

面试时诚实说明差异，比机械声称“完全照搬 GoF”更可信。

## 22. 为什么不是其他模式

| 方案 | 为什么当前不选 |
| --- | --- |
| 一个大 `if` 方法 | 两个 authority 的读取、错误、trace、短路会继续纠缠；但本节内部固定 plan 仍保持足够具体 |
| Template Method | 更适合固定算法骨架与 subclass hook；这里关注有序 gate 的 continue/stop |
| Strategy | 选择一个可替换算法；本节要求两条必经规则依次执行 |
| middleware | 常包围 transport/next 并有 before/after；本节是领域资格决定，不污染 HTTP |
| Specification | 容易把外部 I/O、错误、时间和 trace 压成布尔谓词 |
| 并行 fan-out | 失去 risk 零调用、稳定原因与隐私最小化 |
| OPA/XACML/DMN | 需要语言、数据、编译、冲突/undefined、发布、安全和运维；当前两条 typed gate 没有这些需求 |

## 23. 当前尚未完成什么

- 没有 production RegistrationFactReader adapter；
- 没有 production RiskScreeningFactReader adapter；
- 没有真实用户/风险系统联调；
- 没有 Activity 或已发布 ruleset；
- 没有可信 Principal/session；
- 没有服务端 Authorization；
- 没有公开 Participation API；
- 没有接入 Lottery 或 `WeightedSelector`；
- 没有次数账户、订单、库存、正式 Draw 或发奖；
- 没有 trace 持久化、审计或 OTel；
- 没有 rule tree、route、DSL 或动态配置；
- 没有浏览器/Compose E2E，因为本节没有对应 runtime change。

## 24. 简历与口述边界

可以准确表述：

> 在 Participation domain/application 中新增基于权威风险快照的准入规则，并从新用户与风险两个具体 gate 提炼固定有序的短路资格链；使用单一服务端 logical as-of、规则集/策略/事实独立版本、业务拒绝与技术失败分离、受控 trace 与显式 Cause 诊断通道，通过 freshness 纳秒边界、取消、typed-nil、tail 零调用、并发 race 和架构测试验证。

不能表述：

- 已实现通用规则引擎；
- 已接入真实风控；
- 已完成抽奖全流程资格门控；
- 已实现用户登录或权限系统；
- 已保存审计轨迹；
- 已通过公开 API/浏览器端到端；
- 已解决跨系统原子快照或 TOCTOU。

## 25. 下一节为什么自然出现

第 27 节引入真实会员分层路由：

```text
standard -> Strategy A
premium  -> Strategy B
no match -> explicit default / reject
```

此时输出从 `continue/reject` 变成 `route`，路径可能共享后续节点、需要显式 default 和 path trace。若继续硬塞线性链，会出现：

- 在通用 context 偷写 StrategyID；
- 用 sentinel error 表示 route；
- 根据上一步偷偷跳过节点；
- 复制公共尾节点；
- 用 priority 模拟 default；
- trace 不再是简单前缀。

第 27 节要用代码和需求证据展示这些局限，而不是马上重构成终态。第 28 节才设计可持久化、可发布、可校验的规则树；第 29 节才执行已验证配置。

## 26. 本节复盘

责任链的价值不在多写一个接口，而在把顺序、继续、拒绝、失败和依赖访问变成可测试契约。本节真正的进步是：

1. 第二条规则来自已有风险需求；
2. 每个事实和决定仍有明确所有者；
3. 单一 as-of 让两个时间规则可解释；
4. 前序拒绝真的阻止后序敏感读取；
5. error 不被压成 ineligible 或 pass；
6. trace 只记录最小必要证据；
7. 抽象被限制在 Participation 和固定线性 plan；
8. 后续分支需求可以诚实证明这套方案何时失效。

这就是渐进架构：不是每节堆更多技术，而是每次只让当前方案承担已经出现的问题，并保留下一次改变的可验证理由。
