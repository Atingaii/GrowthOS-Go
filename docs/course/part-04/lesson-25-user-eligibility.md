# 第 25 节：需求升级——不是所有用户都能抽

> 第 23 节只定义了规则事实与决定的所有权，第 24 节只缓存可重建的 Lottery Strategy 读取投影。本节第一次让 Participation 拥有一条可执行的业务资格规则：从消费方拥有的窄端口读取权威注册事实，在一次受控服务端时刻下检查事实新鲜度，再用含边界的 registration cutoff 形成 `eligible` 或 `ineligible`。本节刻意停在 domain/application：没有事实 adapter、HTTP、React、MySQL、Redis、Activity、认证、授权、责任链或正式 Draw，现有 ephemeral Lottery API 也没有因此获得用户资格保护。

- **起点：** `35f94b9`（第 24 节已验收 tip）
- **规则基线：** [新用户资格规则基线 v1](../../product/new-user-eligibility-v1.md)
- **长期决策：** [ADR-0021：以权威注册事实实现首个 Participation 新用户资格切片](../../decisions/ADR-0021-participation-new-user-eligibility.md)
- **API 记录：** [第 25 节 API](../../api/lessons/lesson-25.md)
- **验收证据：** [第 25 节 QA](../../qa/lessons/lesson-25.md)
- **设计手记：** [第 25 节设计手记](../../design-thinking/lessons/lesson-25.md)
- **面试问答：** [第 25 节面试问答](../../interview/lessons/lesson-25.md)

## 1. 为什么“不是所有用户都能抽”不是一个 `if`

产品最初的一句话可能是：

> 只有新用户可以参与这次抽奖。

最容易写出的代码是：

```go
if request.IsNewUser {
    selectAward()
}
```

但这段代码没有回答任何真正决定系统是否可信的问题：

1. `IsNewUser` 是谁计算的，浏览器能否篡改；
2. “新”按注册时间、首单、首次登录还是首次入组定义；
3. 注册时间来自哪个事实所有者，怎样知道读到的不是旧快照；
4. 边界时刻算新还是不算新；
5. 事实不存在、超时、损坏和确定不符合是否返回同一种结果；
6. 一次求值读取几次时钟，跨越 cutoff 时会不会自相矛盾；
7. 本条资格通过是否已经等于“允许访问”“已经抽奖”或“已经获奖”；
8. 当前还没有登录用户，怎样避免用 demo header 伪造安全闭环；
9. 只有一个规则时，是否真的需要责任链或规则引擎。

所以本节的主要工作不是添加一个布尔字段，而是让“注册事实”“新用户政策”“确定业务决定”和“无法可信决定”成为不同的、可验证的概念。

## 2. 先把四类判断彻底分开

同一次未来抽奖请求至少可能经过四类判断，它们不能共用一个 `allowed bool`：

| 问题 | 决定所有者 | 当前状态 | 失败不能伪装成 |
| --- | --- | --- | --- |
| 调用者是谁 | 未来会话/身份能力 | 第 32 节前未实现 | 用户资格不通过 |
| 主体能否对资源执行动作 | 未来统一访问控制 | 第 31～35 节逐步实现 | 老用户、未中奖 |
| 主体是否满足活动业务条件 | Participation | 本节只实现第一条具体规则 | 401/403、`no_reward` |
| 合法候选中选中哪一个 | Lottery | 已有 Strategy + WeightedSelector | 资格拒绝、依赖故障 |

`eligible` 的准确含义只是：**这份权威注册事实满足这一版 registration-cutoff policy。**

它不表示：

- 已经认证；
- 有权访问某个 Activity；
- 其他资格、次数、风险或库存规则也通过；
- 已经创建 Participation 或 Draw；
- 已经选择 Award；
- 已经发放 Benefit。

这种语言上的克制直接决定后面的错误码、监控、重试和审计是否会说真话。

## 3. 本节开始前的真实系统边界

第 24 节结束后已有：

- Lottery `Strategy` / `Award` 聚合；
- MySQL Strategy Repository；
- 无偏固定权重选择；
- development/test 专用的 ephemeral selection API；
- React 临时选择消费者；
- 只缓存 Strategy 可重建读取投影的 Redis cache-aside；
- 第 23 节关于 Activity、Participation、Lottery、Benefit、Governance 的决定所有权基线。

但仍没有：

- 真实会话、Principal、角色和资源级授权；
- Activity 与已发布政策版本；
- 外部用户目录 adapter 或本地受控用户投影；
- Participation 请求、次数账户、订单、Draw/Result 或幂等身份；
- 两条以上需要顺序、短路或路由的具体资格规则。

这意味着我们可以先形成可执行的 Participation 内核，却不能把一个任意 user ID 接到公开 route 后就宣称“抽奖已受用户资格保护”。

## 4. 本节交付和明确非目标

### 4.1 本节交付

- 新建 `internal/participation/domain`；
- 定义非零、不透明的 `ParticipantRef`；
- 定义最小 `RegistrationFactSnapshot`；
- 定义版本化且含边界的 `NewUserPolicy`；
- 定义固定规则 code、稳定 outcome 和 reason；
- 实现纯函数 `EvaluateNewUserEligibility`；
- 在 application 包定义消费方拥有的 `RegistrationFactReader`；
- 用 `Clock` 在事实读取成功后只捕获一次服务端时刻；
- 用 `NewUserEligibilityService` 处理事实读取、取消、未来时间、主体匹配和 freshness；
- 区分确定的业务不符合与 not-found/stale/unavailable/invalid；
- 用类型化安全错误保留可信诊断 cause，但不在普通字符串中泄漏它；
- 用单元、fuzz、并发、取消和架构测试覆盖关键边界。

### 4.2 本节明确不做

- 不新增 HTTP route、header、request/response DTO、status 或 error code；
- 不把 `ParticipantRef` 放进现有 ephemeral Lottery API；
- 不新增 `X-Demo-User-ID` 或 `is_new_user`；
- 不修改 React 页面、导航、按钮或状态；
- 不实现外部 Account/User Directory adapter；
- 不新增 MySQL Migration、runtime grant 或本地用户表；
- 不把 registration fact 或 eligibility decision 放入 Redis；
- 不新增 Activity、正式 Participation、Draw/Result、幂等或次数扣减；
- 不实现认证、RBAC、权限管理或前端权限裁剪；
- 不引入 `Rule`、`RuleChain`、`RuleEngine`、`Specification`、DSL 或通用 context；
- 不把资格通过直接委托给 Lottery selector。

这些非目标不是“少做了”。它们是为了保证每一节只承担当前已经具备可信前提的问题。

## 5. 统一语言

| 术语 | 本节唯一含义 | 不能理解为 |
| --- | --- | --- |
| `ParticipantRef` | Participation 向事实提供方查询主体的非零、不透明内部引用 | 登录证明、Principal、User 聚合、角色或 tenant |
| `RegistrationFactSnapshot` | 某次求值读取到的最小不可变注册事实 | GrowthOS 用户主表、客户端 DTO、长期审计记录 |
| `RegisteredAt` | 权威目录确认的平台账户注册 instant | 首单、首次登录、首次参加活动 |
| `ObservedAt` | provider 捕获或观察该快照的 instant | 通用 `updated_at`、TTL 或政策生效时间 |
| `FactSource` | 提供注册事实的权威来源标识 | 用户可见文案或指标默认 label |
| `FactRevision` | provider 侧快照修订标识 | policy revision、Git SHA 或 Strategy version |
| `PolicyRevision` | 本次新用户政策快照的稳定修订标识 | fact revision、Migration version |
| `EvaluatedAt` | 事实读取后由服务端时钟捕获的一次求值 instant | 客户端时间、数据库观察时间 |
| `eligible` | 本条具体规则被可信事实确认通过 | 已授权、已完成全部资格、已中奖 |
| `ineligible` | 事实充分且注册时刻早于 cutoff | 依赖异常、事实过期、401/403、`no_reward` |

## 6. 决定所有权：外部目录拥有事实，Participation 拥有判断

外部 Account/User Directory 才知道账户何时完成注册。Participation 不应通过新增一张本地 `user` 表夺走这个事实所有权，也不能让浏览器提供最终结论。

反过来，外部目录也不应回答“这个人能否参加 GrowthOS 某个新用户活动”。目录只提供最小事实；Participation 结合业务 policy 形成决定：

```text
external account directory                    Participation
--------------------------                    -------------
participant reference ----------------------> lookup
                                             RegistrationFactSnapshot
registered_at / observed_at / revision ------>
                                             freshness + cutoff evaluation
                                             eligible | ineligible
```

这就是为什么端口定义在消费方 application 包：

```go
type RegistrationFactReader interface {
    FindRegistrationFact(
        ctx context.Context,
        participantRef domain.ParticipantRef,
    ) (domain.RegistrationFactSnapshot, error)
}
```

未来无论 adapter 使用 HTTP、gRPC、数据库投影还是测试 fixture，它都必须满足 Participation 已经明确的最小读取语义。应用核心不需要依赖 provider SDK，也不会吸收完整用户画像。

## 7. `ParticipantRef` 为什么不叫 `UserID` 或 `Principal`

```go
type ParticipantRef uint64
```

这个值只解决“向受信 provider 查哪一个主体”。知道 `42` 不代表调用者就是主体 42，更不代表主体 42 有权访问某个 Activity。

若现在把它叫 `Principal`，后续开发者很容易误认为：

- header 中带有这个值就完成认证；
- 前端隐藏别人的 ID 就完成对象级授权；
- 资格通过就等于有权限；
- 所有上下文都应共享这个 ID 生命周期。

因此命名本身是一条安全停止线。第 32 节的真实会话能力将负责把凭据绑定到 Principal；本节不会倒灌未来语义。

## 8. 注册事实快照：最小字段也必须能自证一致

`RegistrationFactSnapshot` 保留五项私有状态：

```text
participant_ref
registered_at
observed_at
fact_source
fact_revision
```

构造成功必须满足：

1. `participant_ref > 0`；
2. `registered_at` 非零；
3. `observed_at` 非零；
4. `registered_at <= observed_at`；
5. source 非空、UTF-8 合法、无首尾空白、只含可打印字符、最多 128 bytes；
6. revision 同样规范，最多 256 bytes；
7. 时间规范为 UTC，并用 `Round(0)` 去掉单调时钟成分。

source/revision 不是装饰字段。没有它们，未来无法回答“哪个 provider 的哪一版事实参与了决定”。但它们也不能携带手机号、邮箱或原始 payload；否则诊断字段会成为隐蔽 PII 通道。

构造器与 `Validate` 同时存在有两个原因：

- 正常调用方通过构造器得到合法 value；
- application 仍要防止未来 adapter、反序列化或包内代码返回零值/损坏值。

## 9. 新用户政策：一句话必须变成精确边界

本节把“新用户”定义为：

```text
registered_at <  registered_at_or_after  -> ineligible
registered_at == registered_at_or_after  -> eligible
registered_at >  registered_at_or_after  -> eligible
```

规则 code 固定为：

```text
participation.new_user.registered_on_or_after
```

政策由 `PolicyRevision` 和 `RegisteredAtOrAfter` 组成。它不使用“最近 N 天”这类会随每次 `now` 滑动的模糊定义，也不把 cutoff 写死在 evaluator 中。

`>=` 的 inclusive 语义必须明确。否则运营写“8 月 30 日起注册”时，恰好在边界注册的用户可能因服务、SQL 和前端各自理解不同而得到相反结果。

所有时间按 instant 比较，而不是按时区显示字符串比较：

```text
2026-08-30T16:00:00+08:00
2026-08-30T08:00:00Z
```

这两个表示相同 instant，必须产生相同决定。

## 10. 具体 Decision，而不是 `bool`

纯 evaluator 的签名为：

```go
func EvaluateNewUserEligibility(
    policy NewUserPolicy,
    fact RegistrationFactSnapshot,
    evaluatedAt time.Time,
) (NewUserEligibilityDecision, error)
```

事实充分时返回的决定包含：

- `Outcome`：`eligible` / `ineligible`；
- 固定 `RuleCode`；
- 稳定 `ReasonCode`；
- `PolicyRevision`；
- `FactSource` / `FactRevision`；
- 单一 `EvaluatedAt`。

决定刻意不包含：

- `ParticipantRef`；
- 原始注册时间；
- cutoff；
- 完整事实 payload；
- 用户可见文案；
- SQL、上游地址或内部错误。

`bool` 只能回答“真或假”，不能回答使用了哪版政策、哪个事实快照，也无法让调用方区分确定拒绝和根本没有形成决定。具体 Decision 仍然很小，但已经足够支持受控追溯。

## 11. 纯领域 evaluator 的职责

领域 evaluator 只做三件事：

1. 重新验证 policy、fact 和 evaluated-at；
2. 拒绝 registered/observed time 位于 evaluated-at 之后的事实；
3. 按含边界 cutoff 形成确定决定。

它不读取：

- 系统时钟；
- context；
- 数据库或网络；
- Redis；
- Lottery Strategy；
- 随机源。

同一组 policy、fact 和 evaluated-at 必须得到完全相同的结果。这样边界、时区和重复执行可以被纯单元/fuzz 测试证伪，不需要启动 Docker。

freshness 不放进纯 evaluator，因为 freshness 是“这份 provider 快照还能否用于当前应用调用”的读取契约，不是 `registered_at >= cutoff` 这个业务谓词自身的一部分。

## 12. application service：把读取、时钟与业务决定按顺序装配

`NewUserEligibilityService` 持有：

```text
RegistrationFactReader
Clock
maxFactAge
```

policy 保持每次调用显式传入。当前还没有 Activity 或发布规则集，如果把 policy 放进全局配置或 service 构造器，就会虚构“全平台只有一个新用户 cutoff”。

一次求值严格按以下顺序执行：

```text
validate ctx / participant / policy / service
  -> honor pre-cancellation
  -> read authoritative RegistrationFactSnapshot
  -> let observed caller cancellation win
  -> capture server evaluated-at exactly once
  -> let observed caller cancellation win
  -> validate fact structure and participant match
  -> reject future registered/observed time
  -> reject age > maxFactAge as stale
  -> honor cancellation
  -> call concrete domain evaluator
  -> let observed cancellation win
  -> return confirmed decision
```

顺序很重要：

- 事实读取失败时不调用 Clock，因为没有可能形成业务决定；
- 在读取成功后才捕获 evaluated-at，freshness 衡量的是本次实际使用事实的时刻；
- Clock 每次成功读取只调用一次，避免跨秒或跨 cutoff 的内部不一致；
- stale 在 cutoff 判断前被拒绝，旧事实不能生成看似确定的 business denial；
- caller cancellation 一旦可观察，就不能被 provider 错误或刚形成的决定覆盖。

## 13. freshness 的精确含边界

应用层使用：

```text
fact_age = evaluated_at - observed_at
fact_age <= max_fact_age  -> 可用
fact_age >  max_fact_age  -> stale，无决定
```

所以恰好等于最大年龄仍然有效，超过 1ns 才 stale。这个边界与 registration cutoff 一样必须用测试固定，不能留给调用方猜。

`maxFactAge` 必须为正，但本节不提供环境变量，因为尚无真实 adapter 和 composition root 消费者。将默认值偷偷写成配置会让使用者以为生产 freshness SLA 已经决定。

本节也不缓存 eligibility decision。Strategy cache 的 5 分钟 TTL 只适用于可重建配置投影；它不能拿来证明某个用户事实或业务决定在同样时间内仍有效。

## 14. 业务拒绝与无法决定的错误矩阵

| 场景 | decision | error 类别 | 语义 |
| --- | --- | --- | --- |
| 注册时刻在 cutoff 前 | `ineligible` | `nil` | 事实充分的确定业务拒绝 |
| 注册时刻等于/晚于 cutoff | `eligible` | `nil` | 本条规则确定通过 |
| provider not found | 零值 | `ErrRegistrationFactNotFound` | 当前无法确认，不默认是新人 |
| provider unavailable/内部 deadline | 零值 | `ErrRegistrationFactUnavailable` | 依赖暂时不可用 |
| 未分类 provider failure | 零值 | `ErrRegistrationFactReadFailure` | 未形成可信决定 |
| snapshot 超龄 | 零值 | `ErrRegistrationFactStale` | 事实可能已经失效 |
| snapshot 损坏/主体不符/来自未来 | 零值 | `ErrRegistrationFactInvalid` | provider/adapter 契约异常 |
| Clock 返回零值 | 零值 | `ErrEligibilityClockInvalid` | 服务端时钟不可用 |
| caller 已取消/超时 | 零值 | 原始 context error | 调用生命周期结束 |

这里的 fail-closed 只表示后续流程不能继续。它不意味着把每一种技术失败都记成 `ineligible`。

如果把依赖超时伪装成“老用户”，会产生三个直接问题：

1. 用户会收到错误的业务解释；
2. 指标会把基础设施故障统计成转化漏斗；
3. 调用方不会按依赖故障策略重试或告警。

## 15. 安全错误怎样保留 cause 而不泄漏

adapter 未来可能拿到包含上游地址、SQL 或用户数据的底层错误。application 使用 `RegistrationFactReadError` 同时保留：

- 一个经过评审的稳定 class，供 `errors.Is` 与普通渲染使用；
- 一个可信诊断 cause，供受控日志/trace 检查使用。

`Error()` 只返回稳定 class；`Unwrap()` 保留 cause。未知 class 自动收敛为 `ErrRegistrationFactReadFailure`。

这不是说 cause 可以随意记录。日志仍需要脱敏、访问控制和采样。这里解决的是“错误对象不应默认把内部细节拼进所有上层字符串”。

当前仍有一个必须显式保留的风险：若未来 adapter 把另一 application semantic sentinel 包进 cause，标准 `errors.Is` 可能同时命中 wrapper class 与 cause class，`classifyRegistrationFactReadError` 也可能按检查顺序重新分类。本节只验证了单 class + opaque cause。真实 adapter/HTTP 映射前必须禁止 diagnostic cause 携带 application sentinel 并增加 contract test，或改用不参与标准 error chain 的受控 `Cause`/structured diagnostic 通道；关闭风险前不能按任一 `errors.Is` 命中直接决定 HTTP 状态。

## 16. 为什么本节没有 adapter、MySQL 或 Redis

### 16.1 不造本地用户表

目前没有外部目录到 GrowthOS 的：

- 摄取与幂等协议；
- 更正、删除和迟到事件处理；
- revision 冲突规则；
- PII 保留、加密、访问审计；
- Activity/policy scope；
- 正式 Participation/Draw 快照关联。

现在新增 `participation_user` 或 `registration_fact` 表会制造没有生命周期协议的第二事实源。因此 MySQL Migration latest 仍为 2，runtime grant 不变。

### 16.2 不造内存 production fixture

把几个 demo 用户写死在 composition root 可以让 endpoint 看起来跑通，却不能证明事实权威性、身份绑定或故障语义。测试 stub 只用于验证 application port，不是生产 adapter。

### 16.3 不复用 Strategy Redis cache

registration fact 与 eligibility decision 具有用户维度、隐私、撤销和更高时效要求。它们不能进入 `growthos:*:lottery:strategy:projection:*`，也不能复用 Strategy codec、TTL、ACL 或 fail-open 语义。

## 17. 为什么现有 API 和 React 必须零变化

当前 ephemeral route 只有：

```http
POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections
X-GrowthOS-Demo-Mode: ephemeral-selection
```

`X-GrowthOS-Demo-Mode` 只是对无持久临时结果的显式确认，不是登录凭据。若增加：

```http
X-GrowthOS-Demo-User-ID: 42
```

调用方可以任意填写 `42`，服务端无法证明它代表当前调用者。即使再从可信 provider 查询 42 的注册事实，也只证明“42 符合”，不能证明“调用方就是 42”。

因此本节不修改 route，也不让 React 选择 demo user。公开 API 的完整零变化记录见[第 25 节 API](../../api/lessons/lesson-25.md)。

这也意味着不能宣称“现在不是所有用户都能抽”。准确表述是：**系统已经具备第一条可执行、可组合的 Participation 资格能力，但尚未把可信主体与正式业务入口连接起来。**

## 18. 为什么还不创建责任链

当前只有：

```text
NewUserEligibilityService
  -> EvaluateNewUserEligibility
```

如果现在创建通用 `Rule`，我们必须猜：

- context 中放哪些字段；
- result 是 bool、四态、route 还是副作用；
- 谁决定顺序；
- 是否所有规则都读同一份事实；
- 失败时继续、短路还是 fallback；
- 如何解释 trace；
- 是否和 Authorization、Lottery routing、Inventory 共用。

这些问题没有第二个真实消费者就没有证据。架构测试因此拦截本节 production Go 文件中五个已知过早类型名——`Rule`、`RuleChain`、`RuleEngine`、`Specification`、`EvaluationContext`——并限制 Participation domain/application 的项目内 import。换名类型、generic function 或 `map` context 仍可能绕过名称检查，语义性过度抽象继续依赖代码评审。

第 26 节要先出现第二条具体 Participation 前置规则及真实顺序/短路需求，再提取最小公共协议。

## 19. 代码导读

建议按“事实 → 政策 → 决定 → 编排 → 保护线”的顺序阅读。

### 19.1 领域错误与包边界

```text
internal/participation/domain/doc.go
internal/participation/domain/errors.go
```

先确认 domain 不依赖 Lottery、HTTP、数据库、Redis 或规则引擎，并理解 invalid input 为什么不产生部分决定。

### 19.2 权威事实 value

```text
internal/participation/domain/registration_fact.go
internal/participation/domain/registration_fact_test.go
```

重点看 UTC 规范化、registered/observed 顺序、元数据长度和不可打印 Unicode 反例。

### 19.3 版本化含边界 policy

```text
internal/participation/domain/new_user_policy.go
internal/participation/domain/new_user_policy_test.go
```

重点看 policy revision 与 cutoff 是两个不同维度，且 cutoff 不能为零。

### 19.4 纯决定函数

```text
internal/participation/domain/new_user_eligibility.go
internal/participation/domain/new_user_eligibility_test.go
```

重点看 `<` 是唯一不符合分支，`==` 自然进入 eligible；future fact 返回 error + 零 decision。

### 19.5 application-owned ports 与安全错误

```text
internal/participation/application/ports.go
internal/participation/application/errors.go
internal/participation/application/errors_test.go
```

重点看 reader 为什么由 consumer 定义，以及 `Error` / `Is` / `Unwrap` 各自服务哪种观察方式。

### 19.6 application service

```text
internal/participation/application/new_user_eligibility.go
internal/participation/application/new_user_eligibility_test.go
```

重点看每个 context check 的位置、Clock 一次调用、freshness 边界、participant mismatch 和 provider deadline 分类。

### 19.7 架构停止线

```text
internal/participation/application/architecture_test.go
```

该测试不是业务正确性的替代品；它自动拦截跨上下文 import 和五个已知过早抽象名称，其他形式的通用规则抽象仍需要代码评审发现。

## 20. 按真实提交逐步学习

本节从第 24 节已验收 tip 线性推进。当前实现切片可按以下顺序阅读：

| 顺序 | 提交 | 本步解决的问题 | 建议验证 |
| --- | --- | --- | --- |
| 1 | `ea2cacd` | 定义新用户资格、权威事实与零 API 的产品边界 | `git show --stat ea2cacd` |
| 2 | `b59bc1e` | 收紧 freshness 含边界和高基数观测边界 | `git show --stat b59bc1e` |
| 3 | `959c32a` | 实现 fact、policy、decision 与纯 evaluator | `go test ./internal/participation/domain` |
| 4 | `b718267` | 实现 fact reader、clock、freshness、取消与错误分类 | `go test -race ./internal/participation/application` |
| 5 | `475804b` | 用源码架构测试固定“不提前引擎化”的停止线 | `go test ./internal/participation/application -run Architecture` |
| 6 | `c96b393` | 根据交叉审查补齐手工 zero/partial service 的失败关闭证据 | `go test -race ./internal/participation/application` |
| 7 | `bf15a1b` | 交付课程正文与公开 HTTP 零变化记录 | `go run ./cmd/doccheck` |
| 8 | `4987bdb` | 交付 24 道带官方来源与真实面经题型边界的面试问答 | `go run ./cmd/doccheck` |
| 9 | `399948a` | 交付 QA、10 秒 fuzz 证据与第一性原理设计手记 | `go run ./cmd/doccheck` |

章节最终冻结 tip 还会包含 QA、设计手记、面试问答、索引和门禁记录。上表中的任一中间提交都不能单独冒充完整章节验收版本。

逐提交学习命令：

```bash
git fetch origin
git log --reverse --oneline 35f94b9..codex/lesson-25-user-eligibility
git diff 35f94b9..ea2cacd -- docs/product docs/decisions
git diff b59bc1e..959c32a -- internal/participation/domain
git diff 959c32a..b718267 -- internal/participation/application
git diff b718267..475804b -- internal/participation/application/architecture_test.go
```

## 21. 测试怎样证明边界

### 21.1 domain 测试

- cutoff 前 1ns 为 ineligible；
- 恰好 cutoff 和晚 1ns 为 eligible；
- 相同 instant 的不同时区表示得到完全相同决定；
- 相同输入重复求值完全一致；
- fuzz 在 cutoff 两侧检查 outcome；
- zero policy/fact/evaluated-at 与 future fact 返回零决定；
- fact 构造拒绝零引用、倒置时间、空/超长/非规范元数据。

### 21.2 application 测试

- reader 和 Clock 的调用次数与顺序；
- fact age 恰好等于上限有效，超过 1ns stale；
- not-found、unavailable、provider deadline 和未知错误分类；
- 错误字符串不泄漏测试中的 secret cause；
- participant mismatch、future fact 和 zero Clock 不产生决定；
- pre-cancel 不触达 reader，读取后/Clock 后取消优先；
- blocking reader 释放后仍返回 caller cancellation；
- typed-nil dependency 与非法 max age 在构造期失败；
- 64 个并发只读求值产生一致决定，并由 `-race` 检查共享使用。

### 21.3 架构测试

- domain 不得 import 其他项目包；
- application 只允许 import Participation domain；
- production 文件不得声明五个已知过早规则/链/引擎类型名；换名或其他通用化仍由评审兜底。

### 21.4 适合本节的验证命令

```bash
go test ./internal/participation/domain
go test -race ./internal/participation/application
go test -race ./internal/participation/...
go test ./internal/participation/domain -run='^$' \
  -fuzz='^FuzzEvaluateNewUserEligibilityCutoffBoundary$' -fuzztime=10s
go test ./...
go run ./cmd/doccheck
```

本节没有 adapter、route、Migration、Compose 或 React 变化，所以不新增虚假的 HTTP/浏览器/数据库 E2E。全仓门禁仍应验证既有能力没有回归，但它不能被表述为“真实用户目录已经联通”。

## 22. 观察性与隐私边界

本节没有接入运行时日志或指标。未来组合时，可以考虑：

- 低基数：outcome、rule code、经过评审的 reason code；
- 受控 trace/log 字段：policy revision、fact source/revision、evaluated-at；
- 禁止默认 label：ParticipantRef、完整 revision、上游错误、原始时间或 PII。

revision/source 可能具有高基数，不能因为 Decision 有 getter 就直接全部放进 Prometheus label。结构中可追溯与所有字段都适合公开观测是两回事。

## 23. 本节完成后的准确能力表述

可以表述：

> 在独立 Participation domain/application 切片中，实现基于权威注册事实快照的版本化新用户资格判断：使用含边界 registration cutoff、一次受控服务端时钟、事实 freshness、主体一致性、取消优先和类型化失败语义，区分确定业务拒绝与无法形成可信决定，并以架构测试拦截跨上下文 import 与五个已知过早规则抽象名称。

不能表述：

- 已接入真实用户中心或完成用户同步；
- 现有 Lottery API 已按用户资格拦截；
- 已有登录、RBAC 或前端权限系统；
- 已实现 Activity 级新用户政策发布；
- 已实现责任链、规则树或通用决策引擎；
- 已完成次数扣减、正式 Draw、库存或发奖；
- fact/decision 已持久化或缓存；
- 已通过浏览器端到端验证真实用户资格。

## 24. 复盘：为什么这一小步是真实推进

这一节没有新增页面或数据库表，但它不是概念文档。domain 和 application 都已经可执行，最危险的模糊处——事实权威性、cutoff、freshness、时间、主体匹配、取消和失败分类——都有源码与测试。

更重要的是，本节拒绝了两个看似“更完整”的假闭环：

1. 不用 demo user header 冒充认证；
2. 不用本地 registration 表冒充外部事实所有权。

这让后续每个能力都必须在正确前提出现时再进入：adapter 需要真实 provider contract，公开门控需要可信主体与正式资源，多规则组合需要第二条真实规则。项目因此可以线性演进，而不是先画一个万能终态再不断解释为什么运行事实对不上。

## 25. 下一节怎样自然演进

第 26 节“责任链实现前置规则”不能只把当前 evaluator 包进一个 `[]Rule`。它必须先出现第二条真实、同属 Participation 的前置规则，并回答：

1. 两条规则读取哪些事实；
2. 顺序为什么具有业务意义；
3. 第一条 ineligible 后是否必须短路；
4. dependency error 与 business denial 怎样传播；
5. 是否需要共享一次 evaluated-at 或事实快照；
6. 最小 trace 需要什么、不能泄漏什么；
7. 哪些共同形状已经由两个具体消费者证明，而不是猜测。

只有这些证据出现后，抽取最小线性组合协议才是对重复和顺序问题的回应。规则树、动态数据库 schema 和通用决策引擎仍分别留到第 27～29 节由真实分支复杂度推动。
