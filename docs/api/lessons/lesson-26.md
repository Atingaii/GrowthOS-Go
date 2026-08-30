# 第 26 节 API 记录：Participation 前置资格链可执行，公开 HTTP 契约零变化

- **章节：** [第 26 节：责任链实现前置规则](../../course/part-04/lesson-26-responsibility-chain.md)
- **规则基线：** [Participation 前置资格链基线 v1](../../product/participation-prerequisite-chain-v1.md)
- **架构决策：** [ADR-0022](../../decisions/ADR-0022-participation-prerequisite-chain.md)
- **日期：** 2026-08-30
- **状态：** Participation domain/application 新增风险准入与固定短路链；route/DTO/header/status/error code 零变化
- **QA：** [第 26 节 QA](../../qa/lessons/lesson-26.md)

## 1. 结论

第 26 节没有新增、删除或重新解释任何公开 HTTP route、request/response DTO、header、status 或 error code。

新增的是尚未装配到 transport 的内部 Go application contract：

```text
PrerequisiteEvaluation, error = chain.Evaluate(
    ctx,
    participantRef,
    ruleSetRevision,
    newUserPolicy,
    riskAdmissionPolicy,
)
```

它按固定顺序读取权威注册/风险事实并形成内部业务决定，但不证明：

- 浏览器调用者已绑定 ParticipantRef；
- 已存在正式 Activity/Participation resource；
- 现有 Lottery API 已执行资格链；
- 真实用户目录或风险 provider 已接入；
- 已完成认证、授权、次数扣减、Draw、库存或发奖。

## 2. HTTP surface 不变

| Method | Path | 当前语义 | 第 26 节变化 |
| --- | --- | --- | --- |
| `GET` | `/health` | 进程 liveness | 无 |
| `GET` | `/ready` | 当前 API 实例的 MySQL readiness | 无 |
| `POST` | `/api/v1/lottery/strategies/:strategy_id/ephemeral-selections` | development/test 专用、无主体、无持久结果的临时选择 | 无；未接资格链 |

没有新增：

```text
POST /api/v1/participation/eligibility-evaluations
GET  /api/v1/participants/:id/eligibility
GET  /api/v1/risk/screenings/:id
POST /api/v1/activities/:id/participations
POST /api/v1/activities/:id/draws
```

`RegistrationFactReader` 和 `RiskScreeningFactReader` 是 Go consumer-owned port，不是 HTTP endpoint，也不预设真实 provider 使用 HTTP、gRPC 还是进程内 adapter。

## 3. 现有 Lottery 请求不增加用户或风险字段

现有 bodyless ephemeral request 不接受：

```text
participant_ref
user_id
principal_id
is_new_user
registered_at
risk_verdict
risk_score
policy_revision
ruleset_revision
evaluated_at
```

也没有新增 `X-GrowthOS-User-ID`、`X-Risk-Verdict`、`X-Eligibility-*` 等 header。客户端不能提交最终资格、角色、风险 verdict 或评估时间。

`X-GrowthOS-Demo-Mode: ephemeral-selection` 仍只表达调用者承认结果不可恢复；它不是认证、授权、用户身份或资格证据。

## 4. 现有成功响应不增加 trace

成功 response 仍只描述 ephemeral Lottery selection。本节不返回：

```text
eligibility
eligibility_reason
rule_code
ruleset_revision
policy_revision
fact_source
fact_revision
evaluated_at
executed_steps
```

内部 `EligibilityTraceStep` 含受控 source/revision，但它不是 JSON DTO。尤其 FactRevision 可能高基数，只能用于受限诊断，不自动进入 response、header 或 metric label。

## 5. `reward/no_reward` 与资格仍是不同语义

| 概念 | 含义 |
| --- | --- |
| aggregate eligible | 两条 Participation gate 都确认通过 |
| aggregate ineligible | 某条 gate 由充分事实确认业务拒绝 |
| zero aggregate + error | 无法形成可信资格决定 |
| `reward` | 已允许选择后选中 reward Award |
| `no_reward` | 已允许选择后选中显式 no-reward Award |

因此：

- blocked 不能序列化成 `no_reward`；
- risk timeout 不能降级为 `no_reward`；
- `reward` 不表示资格、库存或发奖成功；
- 当前 route 根本没有调用 chain，不能把现有结果解释为通过资格。

## 6. 为什么暂不定义 HTTP status

application 已区分：

- confirmed business ineligible；
- registration/risk not-found；
- unavailable/read-failure；
- stale/invalid/future；
- invalid policy/ruleset/config/clock；
- caller cancel/deadline。

但本节没有正式 Activity/Participation resource、可信 Principal 或 transport consumer，所以没有决定：

- business ineligible 是 409、422 还是资源状态；
- 风险原因对普通用户披露到什么程度；
- dependency unavailable 如何映射 503 与 retry hint；
- object existence 与无权限如何避免侧信道；
- 哪些 error 只进入安全审计；
- 创建正式 Participation/Draw 后如何表达结果未知。

不能把 `err.Error()` 原样返回，也不能把 Eligibility 自动映射成 401/403。Authentication、Authorization 与业务 Eligibility 回答不同问题。

## 7. 错误 Cause 加固仍不是公开契约

第 26 节将内部 fact read wrapper 的诊断 cause 从标准 `Unwrap()` 改为显式 `Cause()`：

- `errors.Is` 只匹配一个稳定 application class；
- raw provider cause 不进入公开 error tree；
- `Error()` 不渲染 secret、SQL、地址或 payload；
- 受信诊断代码可以显式读取 `Cause()`。

这是内部安全加固，不代表公开 HTTP error code 已确定。未来 adapter/handler 仍需独立 contract test 与安全 mapping。

## 8. 为什么不能现在加 demo participant header

一个 header 只能证明调用者提交了某个数字，不能证明它就是该主体。当前没有：

- credential 到 Principal 的绑定；
- Principal 到 ParticipantRef 的受控映射；
- Activity resource/action scope；
- 对象级数据范围；
- 服务端授权；
- 正式 Participation/Draw identity。

因此 `X-Demo-Participant: 42` 会制造虚假安全闭环。真实会话与服务端 RBAC 分别在第 32～33 节建立，前端权限投影和越权 E2E 在第 34～35 节建立；这些也不能替代业务资格本身。

## 9. 数据库、缓存、readiness 与 Compose 不变

- Migration latest 仍为 2；
- 没有 Participation/risk/user 表；
- MySQL runtime grant 不变；
- Redis 仍只缓存可重建 Strategy projection；
- 没有 eligibility/risk cache key；
- `/ready` 不检查不存在的事实 provider；
- Compose 没有新增 provider、fixture、secret 或网络；
- Nginx/API image 与 route 不变。

端口与测试 fake 不能冒充 production adapter。没有 runtime change，因此本节不执行伪造的 curl/Compose/浏览器 E2E。

## 10. 前端不根据权限或资格裁剪

React 本节零变化：

- 不新增资格页或风险状态；
- 不在浏览器计算 new-user/risk；
- 不发送 ParticipantRef；
- 不隐藏按钮来冒充服务端门控；
- 不增加角色菜单；
- 不展示内部 trace/source/revision；
- 不把 Mock 用户当真实登录。

第 34 节的前端权限投影也只改善体验，安全边界仍在服务端；未来业务资格展示还需要正式 Activity/Participation API。

## 11. 内部调用示例不是网络 DTO

```go
chain, err := application.NewEligibilityPrerequisiteChain(
    registrationFacts,
    riskScreeningFacts,
    application.ClockFunc(time.Now),
    registrationMaxAge,
    riskMaxAge,
)
if err != nil {
    return err
}

evaluation, err := chain.Evaluate(
    ctx,
    participantRef,
    ruleSetRevision,
    newUserPolicy,
    riskAdmissionPolicy,
)
if err != nil {
    // Zero aggregate: do not continue.
    return err
}
if !evaluation.Confirmed() {
    // Defensive programmer error; confirmed results are required on nil error.
    return errUnexpectedEvaluation
}
```

参数必须由未来可信 composition boundary 准备。浏览器不能提交 policy、ruleset、fact snapshot 或 evaluated-at。示例没有在当前进程装配，也不是已运行 route 证据。

## 12. API 零变化验证

```bash
git diff 8b2f3a6..HEAD -- \
  cmd internal/lottery migrations deploy web scripts configs go.mod go.sum
```

预期无输出。全仓 tests/build 证明既有 surface 没有回归，但不表示资格链已经进入 HTTP 流程。

## 13. 未来正式 API 的前置条件

至少需要：

1. 第 30 节建立 Activity lifecycle/version；
2. 第 31～35 节建立公共身份与授权；
3. 真实 Principal 到 ParticipantRef 的受控映射；
4. 两个 provider adapter 的 timeout、as-of、freshness、隐私和错误契约；
5. 正式 Participation/Draw identity 与幂等/未知结果处理；
6. 公开 reason 最小披露与 403/404 侧信道策略；
7. transport DTO/status/error code 独立 ADR；
8. 服务端直接越权测试与浏览器 E2E。

## 14. 可准确表述与禁止表述

可以说：

> 第 26 节增加了内部 Participation 风险准入规则和固定短路资格链，同时保持公开 HTTP/React/schema/Compose 契约零变化。

不能说：

- 已开放资格 API；
- 已接入真实风险系统；
- Lottery API 已受新用户/风险门控；
- 已实现登录或 RBAC；
- 前端已按权限/资格裁剪；
- 已持久化 trace 或审计；
- 已完成正式 Draw、库存或发奖。

保持这些边界，才能让后续每一次公开契约变化都建立在真实资源、身份和服务端强制之上，而不是从一个可伪造 demo header 开始背负兼容债务。
