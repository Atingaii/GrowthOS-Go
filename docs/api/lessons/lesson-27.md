# 第 27 节 API 记录：Lottery 会员路由内核可执行，公开 HTTP 契约零变化

- **需求基线：** [Lottery 会员等级 Strategy 路由基线 v1](../../product/membership-strategy-routing-v1.md)
- **架构决策：** [ADR-0023](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)
- **上游边界：** [Lottery 业务规则需求基线 v1](../../product/lottery-rule-requirements-v1.md)
- **日期：** 2026-08-30
- **状态：** Lottery domain/application 新增具体会员等级路由；公开 route、request/response DTO、header、status 与 error code 零变化
- **QA：** [第 27 节 QA](../../qa/lessons/lesson-27.md)

## 1. 结论

第 27 节没有新增、删除或重新解释任何公开 HTTP route、request DTO、response DTO、header、status 或 error code。

新增的是尚未接入 adapter/composition root 的内部 Go contract：

```text
MembershipSubjectRef
  -> MembershipTierFactReader（只有端口，没有生产 adapter）
  -> authoritative standard | premium snapshot
  -> one controlled as-of + freshness
  -> MembershipStrategyRoutingPolicy
  -> premium_override | baseline_default
  -> StrategyID + one-hop internal path
```

它证明 Lottery 可以根据受控事实形成确定 Strategy 目标，不证明现有请求已经携带可信主体，不证明目标 Strategy 已加载，更不证明已经完成随机选择、正式 Draw、认证或授权。

## 2. HTTP surface 保持不变

| Method | Path | 当前语义 | 第 27 节变化 |
| --- | --- | --- | --- |
| `GET` | `/health` | 进程 liveness | 无 |
| `GET` | `/ready` | 当前 API 实例的既有 MySQL readiness | 无；不检查会员 authority |
| `POST` | `/api/v1/lottery/strategies/:strategy_id/ephemeral-selections` | development/test 专用、无主体、按 URL 中 Strategy ID 执行一次不持久化选择 | 无；没有调用会员 router |

本节没有新增：

```text
GET  /api/v1/memberships/:id/tier
POST /api/v1/lottery/membership-routes
POST /api/v1/lottery/strategy-routes
POST /api/v1/activities/:id/strategy-routes
POST /api/v1/activities/:id/participations
POST /api/v1/activities/:id/draws
GET  /api/v1/lottery/routing-decisions/:id
```

`MembershipTierFactReader` 是 Go consumer-owned port，不是 endpoint，也不承诺未来 provider 使用 HTTP、gRPC、数据库还是进程内 ACL adapter。

## 3. 现有 request contract 零变化

现有 ephemeral selection 仍是 bodyless POST，并继续由 URL 中的 canonical positive uint64 `strategy_id` 指定一个已经确定的 Strategy。第 27 节没有让该 route 在服务端先执行会员路由。

客户端不能新增或提交：

```text
membership_subject_ref
member_id
user_id
principal_id
membership_tier
is_premium
baseline_strategy_id
premium_strategy_id
route_target
route_branch
policy_revision
fact_source
fact_revision
observed_at
evaluated_at
```

这些字段不在 body、query、path 或 trailer。尤其浏览器提交的 `membership_tier=premium` 不能成为权威路由事实。

## 4. Request header 零变化

现有 header 契约没有增加：

```text
X-GrowthOS-Membership-Subject
X-GrowthOS-Membership-Tier
X-GrowthOS-Route-Policy
X-GrowthOS-Route-Target
X-GrowthOS-Evaluated-At
```

已有 `X-GrowthOS-Demo-Mode: ephemeral-selection` 仍只要求调用者确认结果是不可恢复的临时选择。它不是身份、会员事实、Activity、资格、路由 policy、权限或幂等证据。

给 header 起一个 `Demo-User` 名称，也不能把可伪造数字变成认证 Principal。当前没有 credential 到 MembershipSubjectRef 的服务端绑定，因此本节拒绝用 header 制造虚假安全闭环。

## 5. Response DTO 零变化

现有成功 response 继续只描述 ephemeral selection，不新增：

```text
membership_tier
route_rule_code
route_branch
route_reason
routing_policy_revision
membership_fact_source
membership_fact_revision
route_evaluated_at
route_path
selected_strategy_version
```

内部 `MembershipStrategyRouteDecision` 不是 JSON DTO。branch 是会员等级的派生信息，Strategy ID、fact/policy revision 也可能高基数；没有披露与授权决策前，不能直接放进公开 body、header 或普通 metric label。

现有 `selection.award.outcome = reward | no_reward` 语义也不变：

- `reward` 不证明用户已通过会员路由、Participation 资格或授权；
- `no_reward` 不是 unknown tier、fact stale、provider timeout 或 route failure；
- route success 只得到 Strategy ID，不表示 Award 已选择、库存已预占或权益已发放。

## 6. Response header 零变化

现有 wire 边界继续使用既有 header，例如：

| Header | 当前语义 | 第 27 节变化 |
| --- | --- | --- |
| `Content-Type: application/json` | JSON wire contract | 无 |
| `Cache-Control: no-store` | ephemeral selection 与错误不可被代理重放 | 无 |
| `X-Request-ID` | 有界请求关联 | 无 |
| `Allow: POST` | 精确 route 的 method 提示 | 无 |

不存在 `X-Membership-*`、`X-Route-*`、`X-Strategy-Version` 或 `X-Policy-Revision` response header。

## 7. HTTP status 与 error code 零变化

内部 application 已区分：

- invalid argument；
- not configured；
- clock invalid；
- internal route decision invalid；
- membership fact not found；
- provider unavailable；
- read failure；
- invalid/corrupt/mismatched/future fact；
- stale fact；
- caller cancellation/deadline。

这些是 Go 内部语义，不是已发布的 HTTP mapping。本节没有决定：

- missing/corrupt membership fact 应映射 4xx 还是 5xx；
- provider unavailable 是否公开 503、Retry-After 或隐藏细节；
- unknown tier 对普通用户怎样措辞；
- route target 不存在由哪个资源和 status 表达；
- 无权限与资源不存在如何避免枚举侧信道；
- cancellation、正式 Draw result unknown 与安全重试怎样组合。

因此不能把内部 `ErrMembershipTierFactInvalid` 直接序列化，也不能从 `errors.Is` 推导一个尚不存在的公开 error code。现有 ephemeral selection status/error code 集保持第 21～26 节契约。

`ErrMembershipRoutingDecisionInvalid` 表示 domain 返回 nil error 却产生不自洽的内部决定，是 invariant breach，不是 provider unavailable、standard default 或 confirmed business rejection。它同样没有公开 status/code 映射。

## 8. 内部 domain Go contract

本节新增的内部类型包括：

```go
type MembershipSubjectRef uint64

type MembershipTier string

const (
    MembershipTierStandard MembershipTier = "standard"
    MembershipTierPremium  MembershipTier = "premium"
)

func NewMembershipTierFactSnapshot(
    subjectRef MembershipSubjectRef,
    tier MembershipTier,
    observedAt time.Time,
    source string,
    revision string,
) (MembershipTierFactSnapshot, error)

func NewMembershipStrategyRoutingPolicy(
    revision string,
    premiumTarget StrategyID,
    baselineDefault StrategyID,
) (MembershipStrategyRoutingPolicy, error)

func RouteMembershipStrategy(
    policy MembershipStrategyRoutingPolicy,
    fact MembershipTierFactSnapshot,
    evaluatedAt time.Time,
) (MembershipStrategyRouteDecision, error)
```

精确 branch/reason 为：

| fact | branch | reason | target |
| --- | --- | --- | --- |
| confirmed premium | `premium_override` | `premium_strategy_selected` | policy premium target |
| confirmed standard | `baseline_default` | `baseline_strategy_selected` | policy baseline target |
| unknown/unsupported/corrupt | 无 | 无 | zero + error |

default 不是网络 fallback，也不接受客户端 tier。domain pure function 的 `evaluatedAt` 由 application 内受控 clock 提供；它不是未来 transport DTO 字段。

`MembershipRoutingPolicyRevision` 当前只是一段受界、canonical 的内部标识。没有 registry、内容 hash、发布仓储或唯一约束把 revision 强制绑定到 premium/baseline targets；因此网络调用方不能只提交 revision 并期待系统解析出 policy，本文也不承诺“相同 revision 必然代表相同内容”。当前确定性依赖完整 policy snapshot、完整 fact snapshot 与同一 as-of。

## 9. 内部 application Go contract

```go
type MembershipTierFactReader interface {
    FindMembershipTierFact(
        ctx context.Context,
        subjectRef domain.MembershipSubjectRef,
    ) (domain.MembershipTierFactSnapshot, error)
}

type MembershipRoutingClock interface {
    Now() time.Time
}

func NewMembershipStrategyRoutingService(
    membershipFacts MembershipTierFactReader,
    clock MembershipRoutingClock,
    maxFactAge time.Duration,
) (*MembershipStrategyRoutingService, error)

func (service *MembershipStrategyRoutingService) Route(
    ctx context.Context,
    subjectRef domain.MembershipSubjectRef,
    policy domain.MembershipStrategyRoutingPolicy,
) (domain.MembershipStrategyRouteDecision, error)
```

调用语义：

1. subject、policy、service 与 caller context 先校验；
2. clock 在 reader 前恰好读取一次；
3. reader 只返回外部事实，不返回 Strategy ID；
4. fact/error 同时返回时 error 胜；
5. caller cancellation 在每个可观察边界优先；
6. freshness 使用同一 canonical UTC as-of；
7. domain 成功后，application 再断言 `Confirmed()`；branch/reason 或外层/path rule/branch/target 不一致时返回 `ErrMembershipRoutingDecisionInvalid`；
8. 只有证据自洽才返回 confirmed route；技术失败返回零 route。

这是进程内 application API，不是 HTTP DTO，也没有在 `cmd/growth-api` 中装配。

`Confirmed()` 校验的不只是 target 非零，还包括 branch/reason 配对、唯一 path step，以及 step 与外层 rule/branch/target 完全一致。该防御保证未来内部重构不能把半合法 route 送入 Strategy 加载层。

## 10. 内部错误的安全边界

`MembershipTierFactReadError`：

- `Error()` 只渲染一个审核过的 application class；
- `Is()` 只匹配该 class；
- raw provider/domain payload error 不进入公共 `errors.Is` tree；
- `Cause()` 是受信诊断代码主动读取的出口；
- zero/typed-nil/unknown class fail closed。

provider 报告 `domain.ErrMembershipTierFactInvalid` 或 `domain.ErrMembershipSubjectRefRequired` 时，application 对普通调用方只暴露 `ErrMembershipTierFactInvalid`；raw domain cause 仍可通过 `Cause()` 诊断。未来 HTTP adapter 即使出现，也不能原样输出 `Cause()`。

## 11. 身份、资格、路由与选择必须分开

| 决定 | 回答的问题 | 第 27 节是否实现 |
| --- | --- | --- |
| Authentication | 调用者是谁 | 否 |
| Authorization | Principal 能否执行资源动作 | 否 |
| Participation eligibility | 主体能否参加 Activity | 已有内部链，但本节不组合 |
| Membership routing | 已确认等级进入哪个 Strategy ID | 是，仅内部内核 |
| Weighted selection | 固定 Strategy 中选中哪个 Award | 既有能力，本节不调用 |
| Formal Draw/Result | 怎样形成唯一、可恢复结果 | 否 |

会员等级不是角色；premium 不等于管理员；route success 不等于 eligible 或 authorized。未来 UI 隐藏菜单也不能替代服务端授权。

## 12. Adapter、数据库、readiness、Compose 与 Web 不变

- `internal/lottery/adapter` 没有 membership adapter；
- `cmd/growth-api` 没有构造 `MembershipStrategyRoutingService`；
- Migration latest、MySQL grant 与 Strategy schema 不变；
- Redis 仍只缓存既有 Strategy projection，不缓存 tier 或 route decision；
- RabbitMQ/PostgreSQL 没有新增消息或 projection；
- `/ready` 不检查会员 authority；
- Compose 没有新增 provider、secret、service、network 或 fixture；
- React 没有会员路由页面、tier 字段、branch/path 展示或按 tier 选择 Strategy 的逻辑。

端口和 test fake 不能冒充 production adapter。没有 runtime consumer，所以本节不执行也不声称 membership API/Compose/browser E2E。

## 13. API 零变化验证

从第 26 节 tip 到终审加固 HEAD 的下列 diff 已实际为空：

```bash
git diff --name-only 47fc94d..544f4af -- \
  cmd deploy migrations web internal/lottery/adapter \
  internal/infrastructure internal/platform internal/participation \
  configs go.mod go.sum
```

最终文档提交形成后，根代理仍必须对最终 HEAD 复跑。旧 route 的测试或浏览器 smoke 只能证明旧 contract 未回归，不能证明本路由已经上线。

## 14. 未来公开组合的前置条件

至少需要：

1. 真实会话把 credential 绑定到 Principal；
2. 服务端受控映射 Principal/业务主体到 MembershipSubjectRef；
3. 真实会员 adapter 的枚举兼容、observed-at/revision、timeout、取消、错误与隐私 contract；
4. Activity 发布版本引用明确的 routing policy 与合法 Strategy target；
5. policy registry/发布模型保证 revision 与不可变内容的唯一绑定，或公开 contract 明确使用内容地址；
6. Participation eligibility、route、Strategy 加载、selection 与正式 Draw 的编排/幂等边界；
7. 服务端授权、数据范围与 403/404 侧信道策略；
8. transport DTO、status、error code、披露级别与 retry contract 的独立 ADR；
9. direct API 越权/伪造 tier 反例、adapter integration、Compose 与 browser E2E。

## 15. 可准确表述与禁止表述

可以准确表述：

> 第 27 节在 Lottery domain/application 中实现了基于权威会员等级快照的 premium override 与 standard baseline 路由，并以一次服务端 as-of、严格 freshness、零决定失败关闭和一跳只读 path 暴露线性资格链无法表达多出口的边界；公开 HTTP/React/schema/Compose 契约保持零变化。

不能表述：

- 新增了会员路由 HTTP API；
- 现有 Lottery endpoint 会自动识别会员；
- 前端可以提交 tier 或 target；
- 已接入真实会员系统；
- route target 已验证存在或发布；
- 路由成功代表资格或授权通过；
- 已调用 WeightedSelector 或形成正式 Draw；
- 已实现规则树、动态规则引擎或数据库配置；
- 已通过 membership Adapter/API/Compose/browser E2E；
- 已完成登录、RBAC、租户隔离或前端权限裁剪。

保持这条零公开契约边界，才能让后续真实 Activity、身份和服务端强制出现时再设计可验证的网络 API，而不是用一个可伪造 tier header 提前背负安全与兼容债务。
