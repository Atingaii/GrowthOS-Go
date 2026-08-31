# 第 30 节 API 记录：Activity publication 仍为零公开 API

- **课程主题：** 为什么 Strategy 不等于 Activity
- **产品基线：** [Activity 发布与 Lottery 配置精确绑定基线 v1](../../product/activity-publication-binding-v1.md)
- **架构决策：** [ADR-0026](../../decisions/ADR-0026-activity-publication-binding.md)
- **课程正文：** [第 30 节课程](../../course/part-04/lesson-30-strategy-vs-activity.md)
- **QA：** [第 30 节 QA](../../qa/lessons/lesson-30.md)
- **设计手记：** [第 30 节设计手记](../../design-thinking/lessons/lesson-30.md)
- **运行手册：** [Activity publication 验收与故障分诊](../../runbooks/activity-publication.md)
- **记录日期：** 2026-08-31
- **结论：** 第 30 节没有新增、修改或承诺任何公开 HTTP、MCP、Agent 或浏览器 API

> 本文的作用是把“零公开 API”写成可核查的工程事实。内部 exported Go identifier、数据库表、领域 decision 或 application command 都不自动构成网络契约。

## 1. 公开网络面零变化

第 30 节没有注册以下任何路由：

```text
POST   /api/v1/activities
GET    /api/v1/activities/:activity_id
POST   /api/v1/activities/:activity_id/publications
POST   /api/v1/activities/:activity_id/rollbacks
POST   /api/v1/activities/:activity_id/retirement
GET    /api/v1/activities/:activity_id/current
```

也没有它们的同义 route、query endpoint、GraphQL field、WebSocket event、MCP tool 或 Agent action。

因此本节没有新增：

- request/response JSON schema；
- path/query/header 参数；
- HTTP status 与业务 error code 映射；
- pagination、filter、sort、ETag 或 idempotency key；
- Activity capability、导航、路由或操作按钮；
- session、Principal、role、permission 或 resource scope；
- runtime configuration、Compose environment 或生产数据库 grant。

现有公开 endpoint 的路径、请求和响应保持原样。本节内部实现不能被用来推导旧 endpoint 已经自动支持 Activity。

## 2. 为什么必须刻意保持零 API

Activity publish、rollback 和 retire 都会改变新请求将采用的 exact Lottery 配置，是高风险写动作。当前课程尚未形成：

1. 可信 session 到 Principal 的映射；
2. 服务端 Resource/Action/Scope 授权；
3. 运营者和审批者的职责分离；
4. 对资源不存在与无权限的低披露策略；
5. 浏览器 CSRF、cookie、CORS 与越权 E2E；
6. 对外幂等、审计、限流和重放策略。

在这些边界尚未实现时暴露写 API，会把领域模型“可调用”误写成“可安全供外部调用”。所以本节只形成内部领域、应用、ACL 和持久化能力，并由架构测试守住未装配状态。

## 3. 本节新增的是内部 Go contract

以下 contract 位于 `internal/`，只允许仓库内受 Go internal package 规则约束的代码使用。

### 3.1 Marketing domain contract

核心值与聚合包括：

```go
type ActivityID uint64
type ActivityStateVersion uint64
type ActivityLifecycle string
type ActivityPublicationVersion uint64
type ActivityPublicationSchemaVersion uint16
type ActivityPublicationKind string

type Activity struct { /* private state */ }
type ActivityPublication struct { /* private immutable state */ }
type ActivityTransition struct { /* private CAS write plan */ }
type ActivityGateDecision struct { /* private confirmed decision */ }
```

公开构造/恢复/规划函数包括：

```go
func NewActivity(id ActivityID, name string) (Activity, error)

func RestoreActivity(/* exact persisted fields */) (Activity, error)

func RestoreActivityPublication(/* exact persisted fields */) (
    ActivityPublication,
    error,
)

func PlanPublish(/* current + exact candidate */) (ActivityTransition, error)
func PlanRollback(/* current + exact historical target */) (ActivityTransition, error)
func PlanRetire(/* current + evidence + instant */) (ActivityTransition, error)

func DecideActivityGate(
    activity Activity,
    publication ActivityPublication,
    evaluatedAt time.Time,
) (ActivityGateDecision, error)
```

这些签名是 package-level implementation contract，不是 REST DTO。私有字段、防御性副本和 strict `Validate()` 也意味着调用方不能把 struct 当作稳定 wire format。

### 3.2 Internal application commands

内部 use case command 包括：

```go
type CreateDraftCommand struct {
    ActivityID domain.ActivityID
    Name       string
}

type PublishActivityCommand struct {
    ActivityID               domain.ActivityID
    ExpectedStateVersion     domain.ActivityStateVersion
    StartsAt                 time.Time
    EndsAt                   time.Time
    GraphReference           domain.LotteryGraphReference
    StrategyRevisionManifest []domain.LotteryStrategyRevisionReference
}

type RollbackActivityCommand struct {
    ActivityID           domain.ActivityID
    ExpectedStateVersion domain.ActivityStateVersion
    TargetVersion        domain.ActivityPublicationVersion
}

type RetireActivityCommand struct {
    ActivityID           domain.ActivityID
    ExpectedStateVersion domain.ActivityStateVersion
}
```

它们不能直接 JSON marshal 后当成公开协议，原因包括：

- `uint64` identity 对 JavaScript number 精度有额外约束；
- `time.Time` 的文本格式、timezone 与微秒规则尚未形成 wire contract；
- `ExpectedStateVersion` 尚未决定使用 body、header 还是 ETag；
- approval evidence 由 verifier 返回，而不是让浏览器任意提交；
- rollback 的 graph、manifest 和 window 必须从 exact history 复制，不接受 caller payload；
- command 中没有 Principal、authorization result、tenant 或 audit subject。

### 3.3 Internal service results

内部服务目前返回：

| Service | 成功结果 | 失败结果 |
| --- | --- | --- |
| `CreateDraftService.Create` | exact `domain.Activity` draft | zero Activity + error |
| `PublishActivityService.Publish` | exact appended publication | zero publication + error |
| `RollbackActivityService.Rollback` | exact appended rollback publication | zero publication + error |
| `RetireActivityService.Retire` | exact retired Activity | zero Activity + error |
| `ResolveActivityService.Resolve` | confirmed gate decision | zero decision + error |

“返回 Go value”不代表任何字段已获准向终端用户披露。

### 3.4 Commit acknowledgement 丢失时的内部恢复 contract

Publish、rollback、retire 仍严格遵守上表的零值协议：即使 repository 报告
`ErrCommitOutcomeUnknown`，业务返回值也分别是 zero publication 或 zero Activity，不能把尚未对账的
candidate 当作成功结果。唯一的额外能力是可信内部调用方可以从该 error 显式提取一份不可变、经校验且
防御复制的 `ActivityCommitReceipt`：

```go
receipt, ok := application.ActivityCommitReceiptFromError(err)
```

`ok` 只可能在以下条件同时成立时为 true：

- operation class 精确为 `ErrCommitOutcomeUnknown`；
- caller 与 operation context 在 repository 边界返回后均仍存活，未被 caller cancel/private deadline 覆盖；
- receipt 的 before/after root 与 publication/retirement transition 能通过完整校验；
- 调用方走显式 accessor，而不是普通 `Error()`、`errors.Is` 或 error chain。

Receipt 的 operation 只可能是 `publish | rollback | retire`。Publish/rollback receipt 保存 exact
before root、after root 与完整 immutable publication；retire receipt 保存 exact before/after root，after
中包含本次服务端 retiredAt 与 retirement evidence。普通 retryable、conflict、storage/provider failure、
context cancellation 和 malformed receipt 都不能取得 receipt。

新的健康只读连接完成 exact read-back 后，内部调用方构造 observation 并执行纯函数对账：

```go
// publish / rollback：同一 RR snapshot 的 root + exact active publication
observation := application.ObserveCurrentActivity(root, publication)

// retire：exact root-only read-back
observation := application.ObserveActivityRoot(root)

result := application.ReconcileActivityCommit(receipt, observation)
```

结果是闭合三态 `ActivityCommitReconciliationCommitted | ActivityCommitReconciliationNotCommitted |
ActivityCommitReconciliationIndeterminate`，其稳定值分别为 `committed | not_committed |
indeterminate`：exact after-image（以及发布动作的完整
publication）相等才是 committed；exact before-image或同一 next generation 的另一合法赢家是
not_committed；缺行、坏值、identity/name 不匹配、partial snapshot 或已经推进到更晚 generation 一律
indeterminate。该纯函数不做 I/O，也不返回“请重试”；任何状态下都禁止在未制定上层策略时盲重放高风险
command。

## 4. Gate decision 不是 HTTP response

领域 gate 的 confirmed status 精确为：

```text
not_published | scheduled | active | ended | retired
```

它的含义只是 Marketing publication 时间门控：

| status | `AllowsParticipation()` | 含义 |
| --- | ---: | --- |
| `not_published` | false | draft 尚无 publication |
| `scheduled` | false | `evaluatedAt < startsAt` |
| `active` | true | `startsAt <= evaluatedAt < endsAt` |
| `ended` | false | `evaluatedAt >= endsAt` |
| `retired` | false | root 已 terminal retire |

它不是：

- 调用者授权结果；
- 用户资格或次数判断；
- route/Strategy/Award 选择结果；
- 库存、权益或 Draw 结果；
- HTTP status 或公开 error code。

技术失败——例如存储损坏、exact Lottery 配置缺失、依赖不可用、Clock zero 或 context 取消——必须返回 zero decision + error，不能伪装成某个 confirmed status。

## 5. Error class 尚未形成网络映射

内部 application/repository 已定义 reviewed stable classes，例如：

- invalid argument / not configured；
- Activity not-found / already exists；
- publication exact not-found；
- stored root/publication invalid；
- state conflict；
- Lottery publication invalid / unavailable；
- approval rejected / unavailable / evidence invalid；
- operation timeout；
- repository retryable；
- commit outcome unknown；
- unclassified operation/storage failure。

本节没有决定它们分别映射到 `400`、`403`、`404`、`409`、`422`、`500`、`503` 或其他状态。尤其不能提前假设：

- `ActivityNotFound` 一定对未授权 caller 披露为 `404`；
- CAS conflict 一定等于公开 `409`；
- approval rejected 可以把审批细节发给浏览器；
- commit outcome unknown 可以直接映射成可自动重试响应；
- stored invalid 可以返回 SQL/table/identity 细节。

本节新增的 Marketing operation/dependency/repository wrapper 的 `Error()`/`errors.Is` 只暴露 reviewed class；底层 cause 仅供可信诊断代码显式读取。这不改写既有 Lottery repository error contract；未来 transport adapter 仍需独立的 disclosure policy。

## 6. Approval reference 不是身份或授权 token

Publication 保存 `approval_reference`，retired root 保存 `retirement_reference`。二者都是 bounded evidence pointer，只证明内部 verifier 返回了合法形状的引用。

它们当前不证明：

- caller 是谁；
- caller 有什么 role/permission；
- 审批人和发布人不是同一个人；
- evidence 后端真实存在且满足生产策略；
- evidence 可以由外部客户端读取；
- reference 可当 bearer credential 使用。

所以未来 API 不能把 evidence reference 当授权 header，也不能接受 caller 自报 evidence 来绕过 `ApprovalVerifier`。

## 7. 数据库 schema 也不是 API

第 30 节新增的权威 schema 包含：

```text
lottery_strategy_snapshot
lottery_strategy_snapshot_award
marketing_activity
marketing_activity_publication
marketing_activity_publication_strategy
```

这些表是 adapter persistence detail。客户端不得依赖列名、join 形状、`DATETIME(6)` 编码或内部 FK。Marketing 与 Lottery 没有跨 bounded-context FK；这种数据库边界更不能被解释成允许客户端跨上下文直接查询表。

## 8. Runtime 装配状态

本节服务未接入：

- `cmd/growth-api` composition root；
- HTTP router/handler/middleware；
- Compose 长期服务配置；
- runtime secret/env；
- application MySQL table grant allowlist；
- Redis、RabbitMQ 或 PostgreSQL；
- Web navigation/router/page；
- MCP/Agent registry。

因此即使代码能在 unit test 中构造 service，也不存在可供浏览器或第三方调用的 Activity endpoint。

## 9. 未来公开 API 前必须先冻结什么

一个后续章节若要开放 Activity API，至少要另行冻结：

1. Principal、Resource、Action、Scope 的统一权限模型；
2. threat boundary 与 fail-closed 服务端授权点；
3. 真实 session authentication；
4. role/permission 与对象/租户数据范围；
5. create/publish/rollback/retire/read 的服务端 RBAC；
6. approval 与 authorization 的职责分离；
7. request/response schema、时间格式、ID 编码和 version concurrency contract；
8. idempotency、replay、audit actor、rate limit 和 commit-unknown read-back；
9. error disclosure 与 resource enumeration 防护；
10. 前端 capability projection 及越权浏览器 E2E。

这些属于后续课程，不是第 30 节的隐含完成项。

## 10. 可复核的停止线

代码审查时应确认 Activity 标识没有进入 runtime/public surface：

```bash
rg -n "PublishActivity|RollbackActivity|RetireActivity|ResolveActivity|ActivityPublication" \
  cmd internal/infrastructure/httpapi web/src
```

如果未来出现命中，需要判断它是否来自明确的新章节；在第 30 节冻结范围内，production runtime、HTTP 和 Web 命中都应视为越界，而不是“顺手完成”。

## 11. 本节 API 结论

第 30 节形成了可测试的内部发布协议，却没有增加网络攻击面。准确表述是：

> 已实现未装配的 Marketing Activity publication 领域、应用、Lottery ACL 与 MySQL persistence contract；公开 Activity API、认证授权、真实审批 adapter、runtime composition 和前端均未实现。

任何“已有 Activity API”“运营人员已经能发布”“浏览器已按角色展示”或“审批已上线”的说法都超出本节证据。
