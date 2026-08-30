# 第 25 节 API 记录：Participation 新用户资格可执行，公开 HTTP 契约零变化

- **章节：** [第 25 节：需求升级——不是所有用户都能抽](../../course/part-04/lesson-25-user-eligibility.md)
- **规则基线：** [新用户资格规则基线 v1](../../product/new-user-eligibility-v1.md)
- **架构决策：** [ADR-0021](../../decisions/ADR-0021-participation-new-user-eligibility.md)
- **日期：** 2026-08-30
- **状态：** Participation domain/application 新增可执行资格能力；公开 route/DTO/header/status/error code 零变化
- **QA：** [第 25 节 QA](../../qa/lessons/lesson-25.md)

## 1. 本节 API 结论

第 25 节没有新增、删除或重新解释任何公开 HTTP route、请求 DTO、响应 DTO、header、status 或 error code。

本节新增的是尚未接入 transport/composition root 的内部能力：

```text
Participation application
  -> RegistrationFactReader（端口；当前没有生产 adapter）
  -> authoritative RegistrationFactSnapshot
  -> freshness + one controlled server instant
  -> concrete new-user cutoff evaluator
  -> eligible | ineligible | typed inability-to-decide error
```

它证明第一条资格规则的 domain/application 语义已经可执行，不证明浏览器请求已经绑定可信主体，也不证明现有 Lottery API 已受资格门控。

因此下列表述全部错误：

- “新增了用户资格查询 API”；
- “Lottery endpoint 现在会拒绝老用户”；
- “前端可以传 user ID 判断新人”；
- “返回 403 就表示新用户规则不通过”；
- “fact not found 自动表示用户是新用户”；
- “eligible 表示调用者已认证或有权限抽奖”；
- “Redis 已缓存用户资格”；
- “已经完成 Activity、次数扣减或正式 Draw”。

## 2. HTTP surface 保持不变

| Method | Path | 当前语义 | 第 25 节变化 |
| --- | --- | --- | --- |
| `GET` | `/health` | 进程 liveness | 无 |
| `GET` | `/ready` | 当前 API 实例的 MySQL readiness | 无 |
| `POST` | `/api/v1/lottery/strategies/:strategy_id/ephemeral-selections` | development/test 专用、无主体且无持久结果的一次临时选择 | 无；没有资格门控 |

本节没有新增：

```text
GET  /api/v1/participation/eligibility
POST /api/v1/participation/eligibility/evaluations
GET  /api/v1/users/:id/eligibility
POST /api/v1/lottery/strategies/:id/draws
GET  /api/v1/activities/:id/eligibility
```

也没有新增内部 HTTP 或管理端 route。`RegistrationFactReader` 是 Go application port，不是 HTTP endpoint 名称，也不承诺未来 provider 一定使用 HTTP。

## 3. 现有 ephemeral selection 请求零变化

合法请求仍为：

```http
POST /api/v1/lottery/strategies/21003/ephemeral-selections HTTP/1.1
Accept: application/json
X-GrowthOS-Demo-Mode: ephemeral-selection
```

已有约束继续有效：

- `strategy_id` 是 canonical positive uint64 decimal；
- 请求没有 body、query 或 trailer；
- 不接受 `Idempotency-Key`；
- `X-GrowthOS-Demo-Mode` 必须恰好一次且值精确；
- route 只允许 development/test 显式启用；
- 一次请求产生一次新的、不持久化的随机选择；
- 客户端不得对不确定结果透明重试。

第 25 节不允许客户端新增：

```text
user_id
participant_ref
principal_id
is_new_user
registered_at
observed_at
fact_source
fact_revision
policy_revision
evaluated_at
eligibility
```

这些字段既不在 header，也不在 query/body。现有 bodyless contract 不变。

## 4. 为什么不加 demo user header

一种看似快捷的接法是：

```http
X-GrowthOS-Demo-User-ID: 42
```

然后让后端读取主体 42 的权威注册事实。问题是，这只能证明“某个调用方要求评估 42”，不能证明调用方就是 42。

当前系统没有：

- 会话 cookie、bearer token 或其他真实凭据；
- 凭据到 Principal 的服务端绑定；
- Activity/resource/action scope；
- 对象级授权与数据范围；
- 正式 Participation/Draw 身份。

即使 header 名称中写着 `Demo`，它仍然会让 UI 和 API 外观暗示一个不存在的安全闭环。注释不能把可伪造标识变成认证证据。

所以本节选择零 HTTP 变化。`ParticipantRef` 只是 application 内部 fact lookup reference，不是公开 caller identity。

## 5. `X-GrowthOS-Demo-Mode` 不是认证

现有 header：

```text
X-GrowthOS-Demo-Mode: ephemeral-selection
```

只要求调用者明确承认返回结果是 ephemeral。它不表达：

- 用户身份；
- 角色；
- tenant；
- Activity；
- 用户资格；
- 对 Strategy 的权限；
- 幂等身份。

因此不能复用它携带 `participant_ref`，也不能把它当作“测试环境已登录”。本节不会重新解释已有 header 的安全语义。

## 6. 成功响应零变化

现有成功响应仍为：

```json
{
  "selection": {
    "durability": "ephemeral",
    "strategy_id": "21003",
    "award": {
      "id": "1",
      "name": "Reward",
      "outcome": "reward"
    }
  }
}
```

第 25 节不新增：

```text
participant_id
eligibility
eligibility_reason
rule_code
policy_revision
fact_source
fact_revision
evaluated_at
```

现有 `award.outcome = reward | no_reward` 仍只是 Lottery 选择结果。它不表达 Participation eligibility：

- `no_reward` 不是老用户拒绝；
- `reward` 不是资格通过证明；
- Redis/MySQL/事实 provider error 不能降级为 `no_reward`；
- eligible/ineligible 当前不会出现在 HTTP response 中。

## 7. response header 零变化

成功和错误响应继续使用既有 header 边界：

| Header | 当前语义 | 第 25 节变化 |
| --- | --- | --- |
| `Content-Type: application/json` | JSON wire contract | 无 |
| `Cache-Control: no-store` | 临时选择与错误不得被浏览器/代理缓存 | 无 |
| `X-Request-ID` | 有界请求关联 | 无 |
| `Allow: POST` | 错误 method 提示 | 无 |

不存在 `X-Eligibility-*`、`X-Participant-*`、`X-Policy-*` 或 `X-Fact-*`。

fact source/revision 和 evaluated-at 可能具有高基数或敏感性；即使未来有正式入口，也不能未经单独审查直接作为公开 response header。

## 8. status 与 error code 零变化

Participation application 已经区分：

- confirmed eligible；
- confirmed ineligible；
- fact not found；
- fact unavailable；
- fact read failure；
- fact stale；
- fact invalid；
- clock invalid；
- caller cancellation。

但这些是 Go 内部语义，不是已承诺的 HTTP 映射。本节没有决定：

- ineligible 应返回 403、409、422 还是一个正式 Participation resource 状态；
- not-found/stale/unavailable 应怎样映射 5xx；
- 客户端能否重试；
- 哪些 reason 可展示给用户；
- 哪些字段只进入受控 trace/audit；
- 正式 Draw 创建失败和资格拒绝如何组合。

还有一个尚未关闭的映射风险：`RegistrationFactReadError` 既用 `Is` 暴露外层稳定 class，又用 `Unwrap` 保留诊断 cause；违规 adapter 若把另一 application sentinel 放入 cause，`errors.Is` 可能同时命中多个 semantic class。真实 adapter 前必须通过 contract test 禁止这种 cause，或改用不参与标准 error chain 的受控诊断通道。在此之前，未来 HTTP adapter 不能按“任一 `errors.Is` 命中”直接选择 status/code。

因此不能从 `ErrRegistrationFactStale` 推导一个尚不存在的公开 error code，也不能直接把 `err.Error()` 返回给浏览器。

现有 Lottery route 的 status/error code 集保持第 21～24 节契约，不新增 eligibility 类别。

## 9. 为什么业务不符合不能先映射为 401/403

即使未来接入正式 route，也需要保持：

| 类别 | 回答的问题 | 示例 |
| --- | --- | --- |
| Authentication | 调用者是谁 | session 无效 |
| Authorization | 该 Principal 能否对资源执行动作 | 无 `activity:participate` 权限 |
| Eligibility | 已认证/授权主体是否满足业务条件 | 注册时间早于 cutoff |
| Lottery outcome | 允许参与后选择了什么 | `no_reward` |

401/403 通常用于认证/授权边界。老用户不满足某场活动业务资格，不应因为“也不能继续”就自动变成访问控制语义。

本节选择暂不锁定 status，等 Activity、正式 Participation 用例和真实会话形成后再设计公开契约。这避免未来为兼容一个过早 status 被迫混淆业务拒绝与越权。

## 10. 为什么 fact not found 不是 eligible 或 ineligible

当前 provider contract 没有承诺“找不到记录就代表从未注册”。所以：

```text
fact not found -> 无法形成可信决定
```

不能写成：

```text
fact not found -> 新用户 -> eligible
fact not found -> 老用户 -> ineligible
```

这条语义未来若要改变，必须由事实 provider 给出更强、可审计的 contract。transport adapter 也不能自行决定默认值。

同理，stale/unavailable/invalid 都返回零 decision + error，而不是业务拒绝。HTTP 映射必须保留这种分类，不能为了统一文案全部返回“暂不符合活动条件”。

## 11. 本节没有 request DTO

`NewUserEligibilityService.Evaluate(ctx, participantRef, policy)` 是 Go application API，不是网络 DTO。

其中三个参数的来源在未来必须分别解决：

| 参数 | 当前含义 | 未来公开组合前必须证明 |
| --- | --- | --- |
| `ctx` | 调用生命周期与取消 | transport deadline/取消传播 |
| `participantRef` | 权威 fact lookup reference | 真实会话 Principal 到引用的可信绑定 |
| `policy` | 明确版本的含边界政策 | Activity/发布规则集如何选择不可变版本 |

浏览器不能直接提交 `NewUserPolicy` 或 `RegistrationFactSnapshot`。否则调用方可以修改 cutoff、注册时间、新鲜度和最终决定。

## 12. 本节没有 response DTO

`NewUserEligibilityDecision` 是内部 domain value，不是自动可 JSON 序列化的公开对象。它的字段保持私有且没有 JSON tag。

未来公开 DTO 仍要单独决定：

- 是否只返回可理解的稳定业务 code；
- 是否对最终用户隐藏具体反作弊原因；
- policy/fact revision 是否仅进入审计；
- evaluated-at 的精度和隐私；
- 多规则时怎样表达短路和可解释性；
- 是否存在可查询的 Participation/Draw resource。

不能直接 `json.Marshal` domain decision，也不能把 getter 列表当成对外字段清单。

## 13. 数据库、缓存与 readiness 零变化

第 25 节没有新增 Migration：

- Migration latest 仍为 2；
- MySQL 仍只有既有 Lottery 业务表；
- API runtime grant 不新增 Participation 表权限；
- `/ready` 仍只表示 API 的既有 MySQL readiness；
- Redis 仍只保存可重建 Strategy 读取投影；
- 没有 registration fact key、eligibility verdict key 或用户维度 cache；
- 没有 Compose user fixture 或 provider service。

这不是说权威 fact 永远不能来自数据库或缓存，而是当前没有同步、隐私、撤销和 revision 协议，不能用基础设施存在替代事实所有权设计。

## 14. 前端契约零变化

React：

- 不新增用户选择器；
- 不发送 ParticipantRef；
- 不在浏览器计算 `is_new_user`；
- 不隐藏/显示抽奖按钮来冒充服务端强制；
- 不增加 eligibility 状态或错误文案；
- 不新增导航和权限裁剪；
- 继续准确展示现有 ephemeral selection 的边界。

前端权限投影在第 34 节形成；业务资格 UI 也必须等待正式 Activity/Participation API。前端交互可以改善体验，但不能成为资格或授权事实源。

## 15. 安全与隐私边界

第 25 节内部 decision 刻意不携带 ParticipantRef、注册时间、cutoff 或完整 provider payload。未来 transport 还必须遵守：

- 不把 fact reader 底层错误原样输出；
- 不在响应中暴露上游地址、SQL 或 PII；
- 不把 fact revision 当作用户可控回放 token；
- 不把 ParticipantRef 当成认证凭据；
- 不允许客户端覆盖 evaluated-at；
- 不允许客户端声明 `is_new_user`；
- 不因为 UI 隐藏操作就省略服务端资格/授权执行。

## 16. 当前可以调用什么

仓库内部可信代码可以在未来 composition boundary 准备好真实依赖后调用：

```go
service, err := application.NewNewUserEligibilityService(
    registrationFactReader,
    application.ClockFunc(time.Now),
    maxFactAge,
)
if err != nil {
    // Abort composition. A nil/typed-nil dependency or invalid freshness
    // budget must not reach Evaluate.
    return err
}
decision, err := service.Evaluate(ctx, participantRef, policy)
if err != nil {
    // A zero decision plus error cannot authorize the next step.
    return err
}
```

但第 25 节没有实例化这段 composition，也没有 production `registrationFactReader`。示例只解释 Go application contract，不能当成运行中的 API 证据。

本节的可执行证据来自 domain/application 单元、10 秒定向 fuzz campaign、并发、取消和架构测试，而不是 curl 或浏览器截图。示例中的 `return err` 代表组合调用方立即停止；第 25 节没有固定未来 composition function 的具体返回签名。

## 17. 验证 API 零变化

推荐从源码和既有回归两方面验证：

```bash
git diff --name-only 35f94b9..HEAD
git diff 35f94b9..HEAD -- cmd internal/lottery web migrations deploy scripts
go test -race ./internal/participation/...
go test ./...
go run ./cmd/doccheck
```

预期：

- 新增实现只在 `internal/participation/domain` 和 `internal/participation/application`；
- 没有 `cmd/growth-api` composition change；
- 没有 Lottery HTTP handler change；
- 没有 `web` change；
- 没有 Migration/Compose/Redis change；
- 既有 HTTP 测试保持通过；
- Participation tests 证明内部错误边界，但没有伪造 curl E2E。

## 18. 对后续 API 设计的输入

本节只给未来正式 API 提供以下已经验证的事实：

1. 调用方不能提交最终 eligibility verdict；
2. ParticipantRef 必须由可信身份/资源编排获得，不能等同公开 user ID；
3. registration fact 必须来自 consumer-owned port 的受信 adapter；
4. cutoff 是 inclusive，并绑定明确 policy revision；
5. evaluated-at 由服务端一次捕获；
6. fact age 等于上限仍有效，超过才 stale；
7. confirmed ineligible 与 inability-to-decide 必须保持不同；
8. caller cancellation 必须优先传播；
9. domain decision 不自动成为 JSON DTO；
10. source/revision 等追溯字段不自动成为公开 header 或 metric label。

这些输入会约束后续正式 Participation/Activity API，但本节没有预先承诺路径、status、error code 或 DTO shape。

## 19. 本节 API 证据边界

可以准确表述：

> 第 25 节在内部 Participation domain/application 中实现了可由未来服务端消费者调用的新用户资格 contract，并通过零 transport diff 保持现有 HTTP/React 契约不变。

不能表述：

- 已上线资格 API；
- 已绑定真实登录用户；
- 已由服务端拦截老用户抽奖；
- 已定义 eligibility 的 HTTP status/error code；
- 已完成浏览器端到端验收；
- 已实现权限系统、Activity 或正式 Draw；
- 已提供 user/fact/decision 的持久化或缓存。

## 20. 下一次公开契约变化的前置条件

后续若要把资格接入公开业务入口，至少需要：

1. 正式 Activity/Participation 用例，而不是继续扩张 ephemeral Lottery demo；
2. 可信会话把凭据绑定到 Principal；
3. Principal 到 ParticipantRef 的服务端受控映射；
4. 服务端授权决定允许该主体对目标资源发起参与；
5. 真实 RegistrationFactReader adapter 及其 timeout、freshness、隐私和观测协议；
6. 已发布 Activity 选择不可变 policy revision；
7. 多规则组合、短路与错误传播已经由具体需求验证；
8. 独立设计 request/response/status/error code、幂等和结果恢复；
9. 服务端强制与浏览器端到端越权测试。

在这些前提出现前，维持零 HTTP 变化比增加一个看似可演示、实则可伪造的 user header 更符合真实项目演进。
