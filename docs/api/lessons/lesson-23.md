# 第 23 节 API 记录：规则需求已冻结，运行时契约零变化

- **章节：** [第 23 节：需求升级——抽奖策略开始需要规则](../../course/part-03/lesson-23-lottery-strategy-rule-requirements.md)
- **需求基线：** [Lottery 规则需求 v1](../../product/lottery-rule-requirements-v1.md)
- **决策：** [ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- **日期：** 2026-08-30
- **状态：** 需求与边界文档切片；没有运行时 API 变更
- **QA：** [第 23 节 QA](../../qa/lessons/lesson-23.md)

## 1. 本节 API 结论

第 23 节没有新增、修改或删除任何 HTTP route、请求 DTO、响应 DTO、header、status/code、timeout、feature gate、Nginx 规则或前端 transport。

本节新增的是供后续实现使用的需求语言：Activity gate、Participation eligibility、Lottery routing、terminal selection、候选可用性、访问控制和技术失败必须由正确的事实所有者解释。它们不是已经可以调用的 API。

因此下面的表述都是错误的：

- “第 23 节新增规则查询/执行接口”；
- “ephemeral selection 已经校验新人资格”；
- “React 已按会员等级选择 Strategy”；
- “服务端已返回规则命中轨迹”；
- “API 已经区分未授权、资格拒绝和库存不足”；
- “增加了 Redis 规则缓存或用户决策缓存”。

## 2. 保持不变的 HTTP surface

| Method | Path | 当前语义 | 第 23 节变化 |
| --- | --- | --- | --- |
| `GET` | `/health` | 进程 liveness | 无 |
| `GET` | `/ready` | 当前实例 MySQL readiness | 无 |
| `POST` | `/api/v1/lottery/strategies/:id/ephemeral-selections` | development/test 专用、不持久化的 Strategy Award 临时选择 | 无 |

不存在以下 route：

```text
POST /api/v1/lottery/rules/evaluations
POST /api/v1/lottery/draws
GET  /api/v1/lottery/rule-sets/:id
GET  /api/v1/activities/:id/eligibility
POST /api/v1/activities/:id/participations
```

这些路径只是常见命名示例，不是已经批准的未来 API 设计，也不能据此创建客户端类型。

## 3. 现有 ephemeral selection 请求保持不变

```http
POST /api/v1/lottery/strategies/21003/ephemeral-selections HTTP/1.1
Accept: application/json
X-GrowthOS-Demo-Mode: ephemeral-selection
```

它仍然：

- 只接受 canonical uint64 decimal StrategyID；
- 没有 body、query 或 fragment；
- 不接受 `Idempotency-Key`；
- 必须通过 development/test feature gate；
- 只读取 Strategy 聚合快照；
- 把该 Strategy 直接交给 `WeightedSelector`；
- 返回不可恢复、不可兑付的 ephemeral Award 候选；
- 不会透明重试。

第 23 节不能向请求偷偷增加：

```json
{
  "user_id": "...",
  "activity_id": "...",
  "member_tier": "...",
  "rule_set_version": "...",
  "available_award_ids": ["..."]
}
```

这些字段分别涉及身份、Activity、用户事实、版本发布和候选可用性；它们尚无经过实现与验收的权威来源。让浏览器提交这些值还会把可伪造的客户端声明误当作服务端事实。

## 4. 现有成功响应保持不变

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

仍只允许：

```text
award.outcome = reward | no_reward
```

响应没有、也不能由前端猜测下列字段：

```text
activity_id
participant_id
eligibility
rule_code
reason_code
rule_set_version
strategy_version
decision_trace
draw_id
inventory_reservation_id
benefit_id
```

`reward` 仍表示“加权算法选中了奖励候选”，不是资格已验证、中奖已持久化、库存已预占或权益已发放。`no_reward` 仍是一次成功选择中的合法候选，不是业务拒绝或异常兜底。

## 5. 现有错误契约保持不变

公开错误 envelope 仍为：

```json
{
  "error": {
    "code": "lottery_strategy_not_found",
    "message": "lottery strategy not found",
    "request_id": "req-..."
  }
}
```

第 23 节没有新增以下 error code：

```text
activity_not_active
participant_not_eligible
participation_quota_exhausted
risk_rejected
rule_evaluation_failed
award_unavailable
unauthenticated
permission_denied
```

这些字符串只表示未来需要独立语义类别，不是当前 wire contract。直到对应资源、事实来源、隐私披露边界和 use case 存在前，不能先占用 status/code 并让调用方依赖。

## 6. 需求语义不是当前 wire DTO

[Lottery 规则需求 v1](../../product/lottery-rule-requirements-v1.md) 要求未来求值边界能够区分：

```text
continue
route(target)
business_reject(rule_code, reason_code)
technical_failure(cause)
```

还要求最终选择的 `no_reward`、候选不可用和授权拒绝保持独立。但是这些只是语义约束，不是已经冻结的 JSON schema、Go interface 或 HTTP status mapping。

后续 API 设计仍必须回答：

- 这些结果是内部 application contract，还是公开 HTTP response；
- 哪些原因可以安全暴露给用户，哪些只能进入受限审计；
- 拒绝是否使用 2xx 业务结果或 4xx，以及客户端应该如何分支；
- rule/strategy/config/schema version 怎样编码；
- 多条规则 trace 是否返回、分页、截断或只写审计；
- dependency timeout 如何与业务拒绝区分并映射可重试性；
- 正式 Draw 如何提供幂等身份和结果恢复。

本节拒绝用一个貌似完整的 DTO 提前替后续章节回答这些问题。

## 7. 八类结果不得混淆

| 类别 | 示例 | 当前 ephemeral API 是否会产生 | 未来 API 约束 |
| --- | --- | --- | --- |
| 继续 | Activity 生效且当前规则通过 | 否 | 只能作为中间求值结果 |
| 路由 | Gold tier 选择 Strategy B | 否 | target 必须来自服务端可信配置 |
| 业务拒绝 | 活动过期、新人资格不成立 | 否 | 使用稳定代码，不匹配展示文案 |
| `no_reward` | 合法 Strategy 选中未中奖候选 | 是 | 保持成功终端选择，不当作拒绝 |
| 候选不可用 | Award 无法兑现 | 否 | 不得静默改写为 `no_reward` |
| 授权拒绝 | 主体无权操作受保护 Activity | 否 | 与业务资格分离，避免泄漏对象信息 |
| 技术失败 | 事实源 timeout、配置损坏 | 部分仅以既有依赖错误体现 | 不得降级为不合格或 `no_reward` |
| 结果未知 | 未来正式 Draw 可能执行但响应丢失 | 否 | 必须凭业务身份恢复，不得重新随机 |

“当前不会产生”意味着没有相应执行路径，而不是当前请求总能通过这些判断。

## 8. 客户端与服务端信任边界

### 8.1 前端不能成为规则事实源

React 可以在未来根据服务端返回的权限/业务状态改善交互，但不能：

- 用本地时间决定 Activity 是否有效；
- 用 Mock profile 决定用户是否是新人；
- 从隐藏导航推断用户有权调用 API；
- 过滤 Award 数组后声称库存可用；
- 生成 `X-Role`、`X-User-ID` 等自声明 header 作为身份；
- 把网络失败渲染成“资格不通过”或“未中奖”。

### 8.2 API adapter 不能提前新增规划字段

TypeScript interface 只约束编译期代码，不会让服务端产生真实字段。第 23 节不修改 `lotteryApi` decoder；如果有人在前端类型中增加 `ruleTrace?: ...`，却没有 wire contract 与运行时 decoder，这仍然不是 API 能力。

### 8.3 权限不由工作台类型决定

`/admin`、`/mcp` 或 `/agent` 路径不是身份凭证。第 31～35 节会建立统一主体、角色、权限、资源、动作和数据范围；服务端必须执行强制授权，前端只消费同一模型的投影。第 23 节不添加任何临时角色开关。

## 9. 数据库与 Migration 对 API 的影响为零

第 23 节没有 `000003` Migration，也没有规则表、Activity 表、Participation 表、decision trace 表或 audit 表。

当前 `lottery_strategy` / `lottery_strategy_award` 两张表和运行应用的精确 `SELECT` allowlist 不变。行级 `updated_at` 仍不是聚合版本，因此 API 不能返回一个虚构的 `strategy_version`。

数据库里没有规则数据并不意味着可以把规则 JSON 临时塞入现有 name 字段或 Award outcome。持久化结构必须等真实写入、发布、版本和查询语义明确后以新 Migration 演进。

## 10. Redis 与 API readiness 保持不变

Compose Redis 仍是隔离、可丢且没有 API 业务消费者的环境能力：

- API 不读取 Redis 配置或 Secret；
- API 不连接 cache 网络；
- Redis 不影响 `/ready`；
- 没有 Lottery key、TTL、cache-aside、规则缓存、锁或 Lua；
- Redis 停止仍不应改变当前 API readiness。

第 24 节只会在明确定义权威源、key/version、miss/损坏/timeout 和回源语义后，考虑缓存可重建 Strategy 读取投影。用户资格、规则决策、授权决策和 ephemeral selection 结果都不是第 23 节已经批准的缓存对象。

## 11. 认证与授权状态保持不变

当前仍没有：

- 登录 endpoint；
- session cookie/token；
- Subject/Role/Permission 的运行时模型；
- RBAC 或对象级授权 middleware；
- CSRF 完整防线；
- 按权限裁剪的前端路由/导航/操作；
- 越权 E2E 验收。

`X-GrowthOS-Demo-Mode` 只是调用者确认 development/test 临时语义，不是认证、授权或防伪签名。`credentials: "same-origin"` 是浏览器传输选项，也不证明存在会话。

## 12. 当前可执行 API 核查

从第 22 节最终 tip 验证 API/runtime 零漂移：

```bash
lesson23_base=1f95779277b1ea882d607a59e0fd2c475f58bd7a

git diff --exit-code "$lesson23_base" -- \
  cmd internal web migrations configs deploy go.mod go.sum Makefile
```

该命令在第 23 节内容全部提交后必须 exit 0。它直接证明确认范围内没有 tracked runtime diff。

再运行既有全仓门禁：

```bash
make verify
```

通过只表示第 22 节已有 API/前端行为没有回归，以及第 23 节文档满足门禁；它不证明规则需求已经被程序执行。

## 13. 正反例

### 正例：保持当前临时选择

```text
输入：调用者明确提供合法 StrategyID
执行：Repository -> WeightedSelector
输出：ephemeral reward 或 no_reward 候选
解释：没有用户资格、Activity、库存或权限判断
```

### 反例：客户端自报会员等级

```text
POST /ephemeral-selections
body: { "member_tier": "gold" }
```

现有 route 不接受 body；未来也不能直接信任浏览器自报的用户事实。

### 反例：把依赖失败转成 `no_reward`

```text
用户事实服务 timeout -> 返回 no_reward
```

这会把系统无法判断伪装成正常业务结果，破坏恢复、监控和用户权益，属于禁止语义。

### 反例：用 403 表示新人资格不成立

身份已认证且有参与 Activity 的权限，但业务新人条件不成立，属于业务拒绝；不能与“无权访问该 Activity”的授权拒绝共用含糊含义。

### 反例：在前端隐藏按钮就算授权

攻击者可以直接调用 HTTP endpoint。未来前端裁剪只能改善发现性与误操作，服务端授权才是边界。

## 14. 后续兼容约束

当后续章节真正增加规则相关 API 或内部 application contract 时，必须同时满足：

1. RuleCode/ReasonCode 与展示文案分离；
2. RuleSetVersion、Strategy/配置版本和 schema/解释器版本分离；
3. 输入事实记录来源与采集时间，但公开响应不泄漏 PII/风控细节；
4. 继续、路由、业务拒绝和技术失败是封闭、可测试的结果类别；
5. `no_reward` 继续属于 terminal selection；
6. 规则顺序和短路变化可发布、可追溯；
7. 正式 Draw 出现后再设计幂等身份、结果恢复和库存/权益关联；
8. 认证授权由公共访问控制能力提供，不复制进 Lottery DTO；
9. 新 API 必须新增独立 API 记录、QA、设计手记、面试问答和必要 ADR/Migration；
10. 前端 decoder 只在 wire contract 实际变化时同步升级。

## 15. 本节能准确表述与不能表述

可以表述：

> 第 23 节把规则需求、事实所有权、失败语义和未来兼容点冻结为产品与架构契约，同时用相对第 22 节 tip 的 runtime negative diff 保证现有 HTTP/React/数据库/Redis 边界零变化。

不能表述：

- 新增了规则 API 或规则 DTO；
- ephemeral selection 已具备用户资格、Activity、库存或权限判断；
- 规则错误码已经对外发布；
- RuleSetVersion/StrategyVersion 已持久化或返回；
- Redis 已缓存规则或 Strategy；
- 认证、RBAC、Draw、幂等或审计已经完成。

## 16. 关联资料

- [第 21 节 API：development/test 临时 Lottery Selection](lesson-21.md)
- [第 22 节 API：React 消费临时 Lottery Selection](lesson-22.md)
- [第 23 节课程正文](../../course/part-03/lesson-23-lottery-strategy-rule-requirements.md)
- [Lottery 规则需求 v1](../../product/lottery-rule-requirements-v1.md)
- [ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [第 23 节 QA](../../qa/lessons/lesson-23.md)
