# 第 29 节 API 记录：Graph 求值仍是未装配内核，公开 HTTP 契约零变化

- **需求基线：** [Lottery Strategy Routing Evaluation 基线 v1](../../product/lottery-strategy-routing-evaluation-v1.md)
- **架构决策：** [ADR-0025](../../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)
- **上游图契约：** [Lottery Strategy Routing Graph 基线 v1](../../product/lottery-strategy-routing-graph-v1.md)
- **上游业务 oracle：** [Lottery 会员等级 Strategy 路由基线 v1](../../product/membership-strategy-routing-v1.md)
- **日期：** 2026-08-30
- **状态：** Lottery domain evaluator、immutable decision/path、application orchestration 与停止线测试已经形成；公开 route、DTO、header、status、error code、runtime composition 与 Web 零变化
- **QA：** [第 29 节 QA](../../qa/lessons/lesson-29.md)区分当前已执行的内部定向门禁与尚未执行的 final-freeze 门禁；本文不把 Go 内部能力外推成网络 API

## 1. 结论

第 29 节没有新增、删除或重新解释任何公开 HTTP route、request/response DTO、request/response header、status 或 error code。

本节新增的是未装配的内部执行链：

```text
exact StrategyRoutingGraphIdentity
  -> StrategyRoutingGraphReader
  -> one controlled evaluated-at
  -> one MembershipTierFactSnapshot
  -> closed domain evaluator
  -> immutable StrategyRoutingGraphDecision + ordered path
```

`StrategyRoutingGraphEvaluationService` 没有进入 `cmd/growth-api`，domain evaluator 没有被 HTTP adapter 调用，Web 也没有 graph evaluation request。因此“内部可以对一张明确的 graph 求值”与“客户端可以请求规则求值”是两个不同事实。

## 2. HTTP surface 保持不变

| Method | Path | 当前语义 | 第 29 节变化 |
| --- | --- | --- | --- |
| `GET` | `/health` | 进程 liveness | 无；不执行 graph |
| `GET` | `/ready` | 当前 API 实例既有 MySQL readiness | 无；不检查 evaluator、graph identity、会员 provider 或 step/time budget |
| `POST` | `/api/v1/lottery/strategies/:strategy_id/ephemeral-selections` | development/test 专用，对 URL 中已经确定的 Strategy 做一次不持久化选择 | 无；不读取 graph、不读取会员事实、不返回 route path |

本节尤其没有新增：

```text
POST /api/v1/lottery/strategy-routing-graphs/:graph_id/revisions/:revision/evaluations
POST /api/v1/lottery/strategy-routing-graphs/evaluate
POST /api/v1/lottery/rules/evaluate
POST /api/v1/lottery/decisions
POST /api/v1/activities/:activity_id/route
POST /api/v1/activities/:activity_id/draws
GET  /api/v1/lottery/routing-decisions/:decision_id
```

Go 函数名包含 `Evaluate` 不会自动生成 endpoint。内部 decision value 也不是已经发布的 wire resource。

## 3. 现有 ephemeral selection request 零变化

既有 endpoint 仍然是 bodyless POST，canonical positive uint64 `strategy_id` 仍由 URL path 指定。第 29 节没有允许客户端在 body、query、path extension、header、cookie 或 trailer 中提交：

```text
graph_id
graph_revision
schema_version
root_node_id
membership_subject_ref
membership_tier
fact_source
fact_revision
evaluated_at
max_steps
max_duration
rule_code
branch_code
activity_id
```

客户端尤其不能提交 tier。会员等级在本节只来自内部 `MembershipTierFactReader` 返回的领域快照；而该 reader 仍没有真实 production adapter 或 session-to-subject mapping。

现有 ephemeral endpoint 也继续直接按 URL StrategyID 读取 Strategy 并做临时选择，不先调用 graph evaluation service。因此请求一个 StrategyID 与“让服务端根据会员事实路由到 StrategyID”仍是两种不同用例。

## 4. Request header 零变化

本节没有新增：

```text
X-GrowthOS-Graph-ID
X-GrowthOS-Graph-Revision
X-GrowthOS-Membership-Subject
X-GrowthOS-Membership-Tier
X-GrowthOS-Max-Steps
X-GrowthOS-Evaluation-Timeout
X-GrowthOS-Rule-Branch
X-GrowthOS-Activity-Version
```

现有：

```text
X-GrowthOS-Demo-Mode: ephemeral-selection
```

仍只确认调用方理解现有选择结果不可恢复。它不是 graph selector、事实凭证、内部 budget 配置、会话凭证或 authorization proof。

把 GraphID/Revision 或 tier 放入 header 也不会自动变得可信：客户端可以伪造 header，而本节没有 Activity publication、Principal、permission、scope 或服务端 request-to-subject 绑定。

## 5. Response DTO 与 response header 零变化

现有 ephemeral selection response 继续只表达既有临时选择结果。本节内部 decision 的下列信息没有进入任何 JSON：

```text
graph_id
graph_revision
graph_schema_version
root_node_id
terminal_node_id
strategy_target_id
fact_source
fact_revision
evaluated_at
path[]
path[].from_node_id
path[].rule_code
path[].selected_branch
path[].reason_code
path[].to_node_id
```

Response header 同样没有新增 `X-GrowthOS-Graph-*`、`X-GrowthOS-Decision-*` 或 `X-GrowthOS-Route-Path`。

现有通用 header 语义保持不变：

| Header | 当前语义 | 第 29 节变化 |
| --- | --- | --- |
| `Content-Type: application/json` | 既有 JSON wire contract | 无 |
| `Cache-Control: no-store` | 既有响应不可由代理重放 | 无 |
| `X-Request-ID` | 有界请求关联 | 无；不是 decision identity |
| `Allow: POST` | 精确 Lottery route 的 method 提示 | 无 |

内部 path 含会员派生 branch 和高基数 graph/fact identity。未来若对外返回，必须先定义对象权限、数据范围、披露级别、审计与 retention；不能因为 value 已有 getter 就直接 JSON marshal。

## 6. HTTP status 与 error code 零变化

本节新增或复用的内部 application/domain 错误包括：

- invalid argument / not configured；
- graph not found；
- graph invalid / read failure；
- membership fact not found / unavailable / invalid / stale；
- internal evaluation timeout；
- step budget exceeded；
- operator / branch / decision invariant invalid；
- caller context error。

这些没有形成 HTTP mapping。本节没有决定：

- graph 不存在对外是 `404`，还是与无权限统一表现；
- invalid graph 与存储损坏映射哪一种不透明 `5xx`；
- membership fact absence 是业务不可用还是资源缺失；
- internal timeout 使用 `503`、`504` 还是上层 Activity outcome；
- step budget mismatch 属于部署配置、发布校验还是请求错误；
- route path 哪些字段只对审计角色可见；
- 如何防止 graph/revision/object enumeration。

所以不能把 `ErrStrategyRoutingGraphEvaluationTimedOut`、domain sentinel、SQL/provider cause 或 path invariant detail直接序列化。现有三个 endpoint 的 status/error code 集保持不变。

## 7. 新增内部 domain contract

### 7.1 Step budget

```go
type StrategyRoutingGraphStepBudget struct { /* private */ }

func NewStrategyRoutingGraphStepBudget(
    maxSteps int,
) (StrategyRoutingGraphStepBudget, error)

func (budget StrategyRoutingGraphStepBudget) Validate() error
func (budget StrategyRoutingGraphStepBudget) MaxSteps() int
```

合法范围精确为 `1..16`。这是服务端内部执行配置，不是公开 request parameter，也不是 graph schema 的可协商字段。

### 7.2 Path step

```go
type StrategyRoutingGraphPathStep struct { /* private */ }

func (step StrategyRoutingGraphPathStep) FromNodeID() StrategyRoutingNodeID
func (step StrategyRoutingGraphPathStep) RuleCode() MembershipRoutingRuleCode
func (step StrategyRoutingGraphPathStep) Branch() MembershipRoutingBranch
func (step StrategyRoutingGraphPathStep) ReasonCode() MembershipRoutingReasonCode
func (step StrategyRoutingGraphPathStep) ToNodeID() StrategyRoutingNodeID
```

Path step 是内部最小执行证据，不包含 subject、raw tier、provider payload、elapsed time 或未走分支。

### 7.3 Complete decision

```go
type StrategyRoutingGraphDecision struct { /* private */ }

func (decision StrategyRoutingGraphDecision) Confirmed() bool
func (decision StrategyRoutingGraphDecision) Identity() StrategyRoutingGraphIdentity
func (decision StrategyRoutingGraphDecision) SchemaVersion() StrategyRoutingGraphSchemaVersion
func (decision StrategyRoutingGraphDecision) RootNodeID() StrategyRoutingNodeID
func (decision StrategyRoutingGraphDecision) TerminalNodeID() StrategyRoutingNodeID
func (decision StrategyRoutingGraphDecision) Target() StrategyID
func (decision StrategyRoutingGraphDecision) FactSource() MembershipFactSource
func (decision StrategyRoutingGraphDecision) FactRevision() MembershipFactRevision
func (decision StrategyRoutingGraphDecision) EvaluatedAt() time.Time
func (decision StrategyRoutingGraphDecision) Path() []StrategyRoutingGraphPathStep
```

`Path()` 返回防御性副本。Decision 不是 formal Draw、eligibility、authorization 或已持久化 audit record。

### 7.4 Pure evaluation function

```go
func EvaluateStrategyRoutingGraph(
    ctx context.Context,
    graph StrategyRoutingGraph,
    fact MembershipTierFactSnapshot,
    evaluatedAt time.Time,
    budget StrategyRoutingGraphStepBudget,
) (StrategyRoutingGraphDecision, error)
```

函数执行封闭 v1 graph，任何错误返回 zero decision。它没有 HTTP、SQL、Redis、Strategy reader、selector 或 random source 依赖。

这些 Go exported identifiers 是仓库内部 package contract。它们没有兼容性承诺到浏览器或第三方网络客户端。

## 8. 新增内部 application contract

```go
type StrategyRoutingGraphEvaluationService struct { /* private dependencies */ }

func NewStrategyRoutingGraphEvaluationService(
    graphs StrategyRoutingGraphReader,
    membershipFacts MembershipTierFactReader,
    clock MembershipRoutingClock,
    maxFactAge time.Duration,
    stepBudget domain.StrategyRoutingGraphStepBudget,
    maxDuration time.Duration,
) (*StrategyRoutingGraphEvaluationService, error)

func (service *StrategyRoutingGraphEvaluationService) Validate() error

func (service *StrategyRoutingGraphEvaluationService) Evaluate(
    callerCtx context.Context,
    subjectRef domain.MembershipSubjectRef,
    identity domain.StrategyRoutingGraphIdentity,
) (domain.StrategyRoutingGraphDecision, error)
```

它是进程内 use-case contract，不是 transport handler。`subjectRef` 是对会员 authority 的 opaque lookup ref，不是 Principal；`identity` 是 exact immutable graph snapshot，不是 Activity active binding；`maxDuration` 是 server-owned technical budget，不是 client timeout field 或公开 SLO。

## 9. 内部调用语义

### 9.1 Exact identity，不支持 latest

Service 只调用：

```text
FindByIdentity(GraphID, Revision)
```

不调用 list/latest/active，不在 not-found 后尝试别的 revision。Reader 返回 wrong identity 即 graph-invalid，不能继续读取会员事实。

### 9.2 依赖次数与顺序

一次合法调用的上限是：

```text
graph read = 1
Clock = 1
membership fact read = 1
```

顺序为 graph -> Clock -> fact -> domain traversal。Graph error、wrong identity、invalid aggregate 或 depth 超 budget 时，Clock/fact 都是零调用。

### 9.3 Output 原子性

只有完整、`Confirmed()` 且 final context 仍 live 的 decision 可以返回。以下路径均返回 zero decision：

- invalid input/config；
- caller cancel/deadline；
- internal maxDuration；
- graph/fact dependency failure；
- invalid/stale/future fact；
- budget/operator/branch/invariant error。

不存在 partial path wire contract，因为根本没有 wire endpoint；内部 caller 也不能观察 prefix。

## 10. 内部 error disclosure contract

`StrategyRoutingGraphEvaluationError`：

- `Error()` 只渲染一个 reviewed stable class；
- `errors.Is` 只匹配该 class；
- 没有 `Unwrap()`；
- `Cause()` 需要可信代码显式选择；
- internal timeout 不匹配 `context.DeadlineExceeded`。

因此，即使未来 HTTP adapter 消费该 service，也不能简单执行：

```go
response.Message = err.Error()
response.Detail = fmt.Sprintf("%+v", err)
```

未来 adapter 必须建立独立的低披露 mapping，并考虑未授权调用者是否能区分 not-found、invalid 与 forbidden。

## 11. `/ready` 不承担 evaluator readiness

`/ready` 继续只表达当前 API 实例的既有 MySQL readiness。它不检查：

- exact graph revision 是否存在；
- graph Repository 是否已装配；
- 长期 runtime 是否有 graph table grant；
- 会员 fact provider 是否可用；
- evaluation `maxSteps` / `maxDuration` 是否配置；
- 某个 Activity 是否发布；
- 某次 route 是否会成功。

把领域配置、外部 authority 与每个业务对象都放进全局 readiness，会让一个 graph 或会员 provider 故障拖垮整个进程的发布/流量判断。本节没有做这种扩张。

## 12. Runtime、Compose、数据库与 Web 零变化

- `cmd/growth-api` 不构造 `StrategyRoutingGraphEvaluationService`；
- HTTP router/handler 不引用 domain evaluator/decision/budget；
- app config 没有 `maxSteps`、`maxDuration` 或 graph identity 环境变量；
- 长期 Compose service/network/secret/port 拓扑不变；
- 数据库 Migration latest 仍是第 28 节 v5，本节不新增表/列/index；
- 长期 `growthos_app` 没有因 evaluator 存在而获得 graph 表权限；
- Redis 不新增 graph/decision cache key；
- RabbitMQ/PostgreSQL 不新增 evaluation event/projection；
- Web 不新增 graph evaluation、path、Activity route 或权限页面。

架构测试只对 `cmd`、HTTP adapter/server、app config、非 acceptance Compose、Docker 与 Web production source 扫描五个 evaluator 标识符，能防止这些标识符的直接装配。Migration、grant、Redis、RabbitMQ、PostgreSQL 与其他间接 wiring 不在该 guard 的证明范围；上表其余零变化由 `90844c1..HEAD` 章节 diff、全仓回归与人工审查补证。

## 13. 身份、权限、事实、执行与业务结果必须分开

| 决定 | 回答的问题 | 第 29 节是否实现 |
| --- | --- | --- |
| Authentication | 调用者是谁 | 否 |
| Authorization / scope | Principal 是否能对资源执行动作 | 否 |
| Membership fact | 某 subject 的权威等级快照是什么 | 只消费 port；无真实 adapter |
| Graph structural validity | 一份 revision 是否是可信 DAG | 第 28 节已有，本节复核 |
| Graph evaluation | 给定 exact graph/fact，走到哪个 Strategy target | 是，内部且未装配 |
| Activity publication | 哪份 revision 正在何时生效 | 否，第 30 节 |
| Participation eligibility | subject 是否可以参加 | 已有独立内部链，本节不组合 |
| Weighted selection | 固定 Strategy 选哪个 Award | 既有独立能力，本节不调用 |
| Formal Draw/Result | 唯一、可恢复的抽奖结果 | 否 |

Graph evaluation success 不是 authorization allow，也不是 eligibility success。即使未来 route 进入 API，服务端仍必须先建立 session、Principal、resource/action/scope 与对象访问控制。

## 14. 未来公开消费的前置条件

### 14.1 第 30 节：Activity 发布绑定

在任何业务 endpoint 自动选择 graph 前，需要：

1. Activity aggregate 与生命周期；
2. published Activity version 对 exact graph revision 的引用；
3. Strategy target 的发布/版本一致性；
4. 并发发布、替换、回滚与历史解释；
5. 发布时验证 graph depth 与 runtime budget 的关系。

### 14.2 第 31～35 节：访问控制

在 graph/path 管理 API 或 Web 出现前，需要：

1. 统一 Principal/resource/action/scope 模型；
2. 真实 credential/session 绑定；
3. 服务端 RBAC 与数据范围强制；
4. capability response 和前端导航/路由/操作投影；
5. direct API 越权、跨对象/跨角色与浏览器 E2E；
6. graph path 中会员派生信息的披露与审计规则。

### 14.3 正式 Lottery 主链

Route 后还需要独立解决：

- Participation gate；
- immutable Strategy snapshot；
- weighted selection/random source；
- idempotent Draw/Result；
- 库存、积分扣减与 Benefit 发放；
- 副作用失败与恢复。

第 29 节不能用一个内部 StrategyID target 冒充整条链已经完成。

## 15. 本节 API 验收契约

最终验收至少应证明：

1. 公开 router 没有 evaluation/rule/graph/Activity 新 route；
2. `/health`、`/ready`、ephemeral selection 的 DTO/header/status/error code 无变更；
3. ephemeral selection 仍按 URL StrategyID 执行，不读取 graph/fact；
4. `cmd/growth-api`、HTTP adapter、app config 不引用 evaluation identifiers；
5. production Compose/Docker/Web source 不引用 evaluation identifiers；
6. Migration、长期 graph grant、Redis/RabbitMQ/PG surface 无扩张；
7.内部 service只按 exact identity，且 graph/Clock/fact 调用上限为 1/1/1；
8.内部 errors 不被描述成已经发布的 HTTP code；
9.内部 path 不被描述成已经对产品用户可见；
10.文档不虚构管理页面、会话、角色或越权 E2E。

命令与真实结果记录在第 29 节 QA。未执行的 final-freeze 检查只能称为待验收，不能写成已通过。

## 16. Stop line：可准确表述与禁止表述

可以准确表述：

> 第 29 节在 Lottery domain/application 内形成了未装配的 closed typed graph evaluation contract：读取 exact immutable graph 与一次权威会员事实，返回完整 Strategy target/path 或零决定；公开 HTTP、runtime、Compose、数据库 schema、Web 和权限契约均未变化。

不能表述：

- 新增了规则求值或 graph evaluation HTTP API；
- 客户端可以提交 GraphID/Revision、tier、maxSteps 或 maxDuration；
- 现有 ephemeral endpoint 已根据会员事实选择 Strategy；
- `/ready` 已检查 graph evaluator 或会员 provider；
- graph evaluation error 已有公开 status/error code；
- path 已对运营后台或普通用户公开；
- graph 已 published/active 或绑定 Activity；
- 内部 `MembershipSubjectRef` 就是可信 Principal；
- route success 等于 authorized/eligible；
- 已接真实会员 authority、Strategy selection 或正式 Draw；
- 已实现登录、RBAC、对象范围或前端权限裁剪；
- 架构测试等于浏览器 E2E、渗透测试或生产安全证明。

本节 API 记录的核心不是“没有接口所以无事可写”，而是明确阻止内部执行内核被误报成已经发布的产品能力，并为第 30～35 节的发布与权限协议保留正确边界。
