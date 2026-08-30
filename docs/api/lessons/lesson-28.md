# 第 28 节 API 记录：Strategy 路由图可持久化，公开 HTTP 契约零变化

- **需求基线：** [Lottery Strategy Routing Graph 基线 v1](../../product/lottery-strategy-routing-graph-v1.md)
- **架构决策：** [ADR-0024](../../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)
- **上游边界：** [Lottery 会员等级 Strategy 路由基线 v1](../../product/membership-strategy-routing-v1.md)
- **日期：** 2026-08-30
- **状态：** Lottery domain/application/未装配 MySQL adapter 与 Migration latest v5 已形成；公开 route、DTO、header、status、error code 与 runtime composition 零变化
- **QA：** [第 28 节 QA](../../qa/lessons/lesson-28.md) 独立记录真实 MySQL、长期 Compose 与 final-freeze pending；本文不把内部证据外推成 HTTP 能力

## 1. 结论

第 28 节没有新增、删除或重新解释任何公开 HTTP route、request/response DTO、header、status 或 error code。

本节新增的是 Lottery 内部的、尚未进入运行时组合根的持久化能力：

```text
StrategyRoutingGraph（有界、不可变、rooted DAG）
  -> StrategyRoutingGraphCreator / StrategyRoutingGraphReader
  -> 未装配的 MySQL StrategyRoutingGraphRepository
  -> lottery_strategy_routing_graph
  -> lottery_strategy_routing_node
  -> lottery_strategy_routing_edge
```

Migration latest 从 v2 前进到 v5 是数据库 schema 变化，不是网络 API 变化。三张表存在也不表示 graph 已发布、已被 Activity 引用、已在请求链执行、已获得管理界面，或任何调用者已经有权读写它。

## 2. HTTP surface 保持不变

| Method | Path | 当前语义 | 第 28 节变化 |
| --- | --- | --- | --- |
| `GET` | `/health` | 进程 liveness | 无 |
| `GET` | `/ready` | 当前 API 实例的既有 MySQL readiness | 无；不验证 graph schema、graph repository 权限或 graph 内容 |
| `POST` | `/api/v1/lottery/strategies/:strategy_id/ephemeral-selections` | development/test 专用、按 URL 中已确定的 Strategy ID 执行一次不持久化选择 | 无；不读取或执行 StrategyRoutingGraph |

本节尤其没有新增下列 endpoint：

```text
POST   /api/v1/lottery/strategy-routing-graphs
PUT    /api/v1/lottery/strategy-routing-graphs/:graph_id
PATCH  /api/v1/lottery/strategy-routing-graphs/:graph_id
GET    /api/v1/lottery/strategy-routing-graphs/:graph_id
GET    /api/v1/lottery/strategy-routing-graphs/:graph_id/revisions/:revision
DELETE /api/v1/lottery/strategy-routing-graphs/:graph_id
POST   /api/v1/lottery/strategy-routing-graphs/:graph_id/publish
POST   /api/v1/lottery/strategy-routing-graphs/:graph_id/evaluate
POST   /api/v1/activities/:activity_id/strategy-routing-graphs
```

Go interface 不是 HTTP endpoint。数据库表名也不是可以被客户端拼接、查询或依赖的资源路径。

## 3. 现有 ephemeral selection request contract 零变化

现有 ephemeral selection 仍是 bodyless POST。canonical positive uint64 `strategy_id` 仍来自 URL path，并且在进入选择服务之前已经确定。

第 28 节没有允许客户端新增或提交：

```text
graph_id
graph_revision
schema_version
root_node_id
nodes
edges
node_kind
rule_code
branch_code
is_default
membership_tier
membership_subject_ref
activity_id
publish_revision
```

服务端不会根据这些 body、query、path、cookie 或 trailer 字段选择 graph，也不会把调用者提交的 node/edge 当成可执行表达式。现有 endpoint 继续直接读取 URL 指定的 Strategy，不先进行图恢复或遍历。

## 4. Request header 零变化

本节没有新增：

```text
X-GrowthOS-Graph-ID
X-GrowthOS-Graph-Revision
X-GrowthOS-Graph-Schema-Version
X-GrowthOS-Rule-Branch
X-GrowthOS-Membership-Tier
X-GrowthOS-Publish-Revision
```

已有 `X-GrowthOS-Demo-Mode: ephemeral-selection` 仍只确认调用方理解结果不可恢复。它不是 graph 选择器、schema 协商、发布凭证、身份凭证或授权证明。

GraphID/revision 放进自定义 header 也不能建立可信发布关系：客户端可伪造 header，而本节没有 Activity publication、Principal、permission 或服务端 data-scope 绑定。

## 5. Response DTO 与 response header 零变化

现有 ephemeral selection response 继续只表达既有临时选择结果。本节没有把内部图结构或恢复诊断暴露为 JSON：

```text
graph_id
graph_revision
graph_schema_version
root_node_id
visited_nodes
matched_branch
is_default
route_path
route_depth
stored_graph_valid
```

现有 response header 语义同样不变：

| Header | 当前语义 | 第 28 节变化 |
| --- | --- | --- |
| `Content-Type: application/json` | JSON wire contract | 无 |
| `Cache-Control: no-store` | ephemeral selection 与错误不可被代理重放 | 无 |
| `X-Request-ID` | 有界请求关联 | 无 |
| `Allow: POST` | 精确 route 的 method 提示 | 无 |

不存在 `X-GrowthOS-Graph-*`、`X-GrowthOS-Rule-*` 或 `X-GrowthOS-Route-Path` response header。

GraphID、revision、node ID 与 Strategy ID 可能形成高基数或暴露运营配置。即使未来增加管理 API，也必须先定义披露权限、审计与分页契约，不能直接返回 repository aggregate。

## 6. HTTP status 与 error code 零变化

内部 repository 现在可以区分：

- `ErrStrategyRoutingGraphNotFound`；
- `ErrStrategyRoutingGraphAlreadyExists`；
- `ErrStoredStrategyRoutingGraphInvalid`；
- 既有 repository invalid argument、not configured、retryable、commit outcome unknown 与 failure 类别；
- domain graph identity/schema/node/edge/topology/limit 等失败关闭类别。

这些都是 Go 内部语义，不是已经发布的 HTTP mapping。本节没有决定：

- create-only 冲突将来是否映射 `409`；
- graph 不存在如何在 `404` 与防枚举策略之间取舍；
- unknown schema、数据库损坏和依赖失败是否统一为不透明 `5xx`；
- commit outcome unknown 怎样表达安全重查而非盲目重试；
- graph 无权限与 graph 不存在是否需要相同外部表现；
- graph validation 细节对运营人员披露到什么程度。

因此，不能把 `ErrStoredStrategyRoutingGraphInvalid`、SQL driver error、约束名、表名或 raw cause 原样序列化。现有 `/health`、`/ready` 与 ephemeral selection 的 status/error code 集保持不变。

## 7. 新增内部 domain contract

本节的核心类型是 Lottery-owned `StrategyRoutingGraph`，而不是跨上下文通用 `RuleTree`、任意表达式 DSL 或权限规则模型。

```go
type StrategyRoutingGraphID uint64
type StrategyRoutingGraphRevision string
type StrategyRoutingGraphSchemaVersion uint16
type StrategyRoutingNodeID uint64

type StrategyRoutingGraphIdentity struct { /* private fields */ }
type StrategyRoutingNode struct { /* private discriminated union */ }
type StrategyRoutingEdge struct { /* private immutable edge */ }
type StrategyRoutingGraph struct { /* private bounded aggregate */ }

func NewStrategyRoutingGraphIdentity(
    id StrategyRoutingGraphID,
    revision string,
) (StrategyRoutingGraphIdentity, error)

func NewStrategyRoutingGraph(
    id StrategyRoutingGraphID,
    revision string,
    rootNodeID StrategyRoutingNodeID,
    nodes []StrategyRoutingNode,
    edges []StrategyRoutingEdge,
) (StrategyRoutingGraph, error)

func RestoreStrategyRoutingGraph(/* persisted schema and complete rows */) (
    StrategyRoutingGraph,
    error,
)
```

v1 只接受：

- `decision` 节点，且 rule 精确为 `lottery.membership_tier.route_strategy`；
- `strategy_target` terminal，且只引用一个非零 Strategy ID；
- `premium_override` 非默认边；
- `baseline_default` 默认边。

它还要求唯一 decision root、全部节点可达、边端点不悬空、无环、terminal 无出边，并把单快照限制在 128 nodes、256 edges、最大深度 16。以上是内部 aggregate invariant，不是客户端可协商的 HTTP schema。

`schema_version = 1` 描述持久化解释协议；GraphID 标识配置家族；revision 标识该家族的一份不可变快照。三者不能互相替代，revision 也未承诺为 content hash。

## 8. 新增内部 application port

```go
type StrategyRoutingGraphCreator interface {
    Create(ctx context.Context, graph domain.StrategyRoutingGraph) error
}

type StrategyRoutingGraphReader interface {
    FindByIdentity(
        ctx context.Context,
        identity domain.StrategyRoutingGraphIdentity,
    ) (domain.StrategyRoutingGraph, error)
}
```

这两个 consumer-owned port 的边界刻意很窄：

- creator 只创建完整 immutable revision；
- reader 只按经过验证的 `(GraphID, Revision)` 恢复一个快照；
- 没有 update、upsert、delete 或局部 node/edge mutation；
- 没有 list、search、latest revision 或 pagination；
- 没有 draft、approve、publish、retire 或 rollback；
- 没有 evaluate、execute、simulate 或 explain；
- 没有加载 Strategy aggregate，也不形成 selection/Draw。

它们是进程内 application contract，不是 transport DTO。未来增加 HTTP adapter 时也不能简单地把这两个接口逐方法映射为 CRUD。

## 9. 未装配 MySQL adapter

`StrategyRoutingGraphRepository` 实现上述 creator/reader，但第 28 节不把它注入 `cmd/growth-api`：

```text
Create
  -> 先校验完整 aggregate
  -> 一个事务内按 header -> canonical nodes -> canonical edges 写入
  -> duplicate identity 是 conflict，不是 upsert 或幂等成功

FindByIdentity
  -> validated identity
  -> repeatable-read + read-only snapshot
  -> header -> bounded nodes -> bounded edges
  -> unknown schema 提前失败关闭
  -> snapshot 结束后严格 Restore
```

“代码中存在 constructor”和“adapter 实现接口”都不等于生产已使用。当前没有 handler、service 或 composition root 持有 graph repository，故任何现有 HTTP 请求都不会触发这些 SQL。

## 10. Migration v5 是 schema change，不是 API change

第 28 节将 Migration latest 推进到 v5：

| Migration | 新表 | 作用 |
| --- | --- | --- |
| `000003` | `lottery_strategy_routing_graph` | `(graph_id, revision)` header、schema version、逻辑 root |
| `000004` | `lottery_strategy_routing_node` | decision/strategy_target 封闭 union 与 Strategy FK |
| `000005` | `lottery_strategy_routing_edge` | 有向 branch/default 与同 graph/revision node FK |

这三张表是内部 persistence schema。它们没有：

- 自动生成公开 route；
- 改变现有 JSON；
- 改变现有 header/status/error code；
- 让 `/ready` 变成 migration 或 graph 内容检查；
- 让浏览器获得数据库访问权；
- 让 graph 自动成为 published runtime configuration。

数据库局部约束也不等于完整图验证。唯一 root、全可达、无环、terminal 无出边和深度预算仍由 domain create/restore 失败关闭；root header 到 node 的反向 FK 因 MySQL 非 deferred FK 写入环问题被刻意省略。

## 11. 数据库身份与最小权限边界

### 11.1 长期 runtime 身份

长期运行的 API 身份 `growthos_app` 不获得三张 graph 表的任何权限：

```text
lottery_strategy_routing_graph
lottery_strategy_routing_node
lottery_strategy_routing_edge
```

原因不是 graph “不重要”，而是本节没有 runtime consumer。提前给长期身份授权会让“尚未装配”的架构停止线只停留在代码约定；最小权限通过数据库拒绝把该停止线变成第二道强制边界。

Migration 身份仍只负责 schema 演进。它不能被 API runtime 或 repository integration 冒用。

### 11.2 真实 MySQL repository 验收身份

真实 MySQL repository 验收只能使用一个测试专用 graph repository identity，并且该身份必须同时满足：

1. 与 migrator identity 不同；
2. 与 API/runtime identity 不同；
3. 只对三张 graph 表具有精确 `SELECT`、`INSERT`；
4. 不具有 `UPDATE`、`DELETE`；
5. 不能读取或写入既有 `lottery_strategy`、`lottery_strategy_award`；
6. 不能读取或修改 `schema_migrations`；
7. 不得被写入长期 Compose/runtime 配置或作为未来生产账户复用。

Strategy FK 的存在不要求 graph repository identity 直接读取 Strategy 表。测试种子由独立受控身份准备，MySQL 在 graph node INSERT 时执行 FK 校验；graph repository identity 仍保持最小权限。

本节 API 文档只规定这一验收边界，不把“测试账户可读写三张表”误写成对产品用户、浏览器或长期 API 进程开放了 graph 管理能力。

## 12. `/ready`、Compose 拓扑、Web 与 runtime composition 没有 graph surface 扩张

- `/ready` 保持既有 MySQL readiness 语义；它不报告 Migration latest v5、graph 表权限、graph revision 或 stored graph validity；
- `cmd/growth-api` 不构造、不注入、不调用 `StrategyRoutingGraphRepository`；
- Compose 的 API/Migration image tag 与 build version 已推进到 `lesson-28`，Migration checkpoint 已推进到 v5；但 service、network、secret 和运行 consumer 拓扑不新增 graph management service、endpoint、fixture、worker 或 queue consumer；
- Redis 不新增 graph cache key；RabbitMQ/PostgreSQL 不新增 graph event/projection；
- Web 不新增 graph 列表、编辑器、画布、发布、模拟、详情页或路由入口；
- 前端导航、路由、操作按钮与角色可见性均未因本节改变；
- 现有 ephemeral selection 页面不会选择 graph revision，也不会展示 traversal path。

Migration container 应用 v5 与 API runtime 读取 graph 是两件事。前者可以发生，后者仍被 composition root 和数据库 grant 同时阻断。

## 13. 身份、权限、资格、图结构与执行必须分开

| 决定 | 回答的问题 | 第 28 节是否实现 |
| --- | --- | --- |
| Authentication | 调用者是谁 | 否 |
| Authorization / data scope | Principal 能否对某 graph/revision 执行动作 | 否 |
| Participation eligibility | 主体能否参加 Activity | 已有内部链，但本节不组合 |
| Membership fact | 主体当前等级事实是什么 | 只有既有 port，无生产 adapter |
| Graph persistence | 一份 Lottery 路由拓扑能否被严格保存和恢复 | 是，仅内部且未装配 |
| Graph execution | 给定事实怎样遍历到 Strategy target | 否，第 29 节边界 |
| Activity publication | 哪个 Activity 使用哪个 immutable revision | 否，第 30 节边界 |
| Weighted selection | 固定 Strategy 中选中哪个 Award | 既有能力，本节不调用 |
| Formal Draw/Result | 怎样形成唯一、可恢复结果 | 否 |

graph schema 合法不表示调用者有权限；有权限不表示 Activity 已发布；图恢复成功不表示事实可用；路由成功不表示 eligibility；Strategy target 存在也不表示已完成选择或正式 Draw。

## 14. 未来消费协议与前置条件

### 第 29 节：内部执行消费

在任何 runtime consumer 出现前，至少需要：

1. 只执行已经恢复并通过 v1 完整验证的 immutable graph；
2. 由服务端受控事实 reader 提供会员事实，禁止客户端提交 tier；
3. exact operator dispatch，unknown rule/kind/branch/schema 失败关闭；
4. step、depth、时间与 cancellation budget；
5. 确定 traversal 与多步 path，但不泄露到公共 transport；
6. 与第 27 节 concrete router 建立等价 fixture，避免迁移改义；
7. graph read、fact read、Strategy load 与选择失败的错误优先级；
8. 不把 fallback 当成吞掉事实失败或未知输入。

### 第 30 节：Activity 发布消费

公开或运行时选择 graph revision 前，还需要：

1. Activity draft/approve/publish/retire/rollback 生命周期；
2. published Activity version 对 immutable graph revision 的稳定引用；
3. target Strategy 的存在、版本和发布语义；
4. 并发发布、历史解释、回滚与缓存失效协议；
5. audit actor、reason、time 与 correlation 证据。

### 第 31～35 节：身份和权限消费

管理 API 或 Web 出现前，还需要：

1. credential 到 Principal 的真实 session 绑定；
2. role/permission/resource/action/data-scope 统一模型；
3. 服务端对 create/read/publish 等动作强制授权；
4. 403/404 与资源枚举侧信道策略；
5. 前端仅作为 capability projection 裁剪导航、路由和操作，不能替代服务端授权；
6. 直接 API 越权、跨范围读取、浏览器 E2E 与审计验收。

只有这些前置条件被独立设计和验收后，才可以讨论真实 graph management/evaluation HTTP API。

## 15. 本节 API 验收契约

本节终验应分别证明“新增内部能力存在”和“公开 API 没有偷偷变化”，而不是只跑一个旧接口 happy path。

必须核查：

1. 公开 router 未新增 graph route；
2. 现有 `/health`、`/ready`、ephemeral selection handler/DTO/header/status/error code 无变更；
3. `cmd/growth-api` 没有 graph repository constructor 或 port 注入；
4. Compose/Web 没有 graph consumer 或 UI；
5. Migration latest 精确为 v5，三张表属于 persistence schema；
6. 长期 `growthos_app` 对 graph 三表无权限；
7. 真实 MySQL repository 验收使用独立 identity，并对三张 graph 表只有 `SELECT`/`INSERT`；
8. 该测试 identity 的 `UPDATE`/`DELETE`、旧 Strategy 表和 `schema_migrations` 访问均被拒绝；
9. adapter 的 create/read 证据不能被表述成 HTTP/Compose/browser E2E；
10. 文档没有虚构 endpoint、DTO、status、error code、管理页面或已发布能力。

具体命令、环境、结果和清理证据应写入第 28 节 QA；未执行前只能称为“验收要求”，不能称为“已通过”。

## 16. Stop line：可准确表述与禁止表述

可以准确表述：

> 第 28 节在 Lottery 内部建立了 GraphID/Revision/schema v1 标识的有界不可变 StrategyRoutingGraph，以 decision/strategy_target、显式 premium/default edge 和三表 create-only 持久化支持严格保存与恢复；creator/reader port 及 MySQL adapter 尚未装配，Migration latest v5 和 lesson-28 构建检查点不新增公开 HTTP、Compose service topology 或 Web 契约。

不能表述：

- 新增了 StrategyRoutingGraph HTTP API；
- 现有 Lottery endpoint 已经读取或执行 graph；
- graph 已发布、已被 Activity 使用或支持灰度/回滚；
- 已实现通用规则树平台、规则引擎、DSL、DMN 或任意表达式；
- 客户端可以提交 membership tier、node、edge 或 branch；
- 已有 graph CRUD、列表、latest revision、编辑器、模拟器或管理页面；
- `/ready` 已检查 graph schema、权限或内容；
- Migration v5 等于 API v5；
- `growthos_app` 已获得 graph 表权限；
- 测试专用 graph identity 是生产 runtime identity；
- repository integration 等于 API、Compose 或 browser E2E；
- graph 合法代表调用者已授权、主体已通过资格或正式 Draw 已完成；
- 已完成登录、RBAC、数据范围或前端权限裁剪。

禁止为“让章节看起来完整”而虚构 endpoint。第 28 节真实价值是先冻结安全、可恢复的内部持久化边界，并用未装配和最小数据库权限诚实地保留后续执行、发布与授权设计空间。
