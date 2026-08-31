# 第 31 节 API 记录：Governance policy kernel 仍为零公开 API

- **课程主题：** 统一访问控制模型与威胁边界
- **产品基线：** [GrowthOS 统一访问控制模型与威胁边界 v1](../../product/access-control-model-threat-boundary-v1.md)
- **架构决策：** [ADR-0027](../../decisions/ADR-0027-governance-access-control-model.md)
- **课程正文：** [第 31 节课程](../../course/part-04/lesson-31-access-control-model-threat-boundary.md)
- **QA：** [第 31 节 QA](../../qa/lessons/lesson-31.md)
- **设计手记：** [第 31 节设计手记](../../design-thinking/lessons/lesson-31.md)
- **面试问答：** [第 31 节面试问答](../../interview/lessons/lesson-31.md)
- **运行手册：** [访问控制模型评审、紧急撤权分析与证据手册](../../runbooks/access-control-model-review.md)
- **记录日期：** 2026-08-31
- **结论：** 第 31 节没有新增、修改或承诺任何公开 HTTP、MCP、Agent、schema 或浏览器 API

> 本文记录的是 `internal/governance/domain` 的仓库内 Go contract。Go identifier 以大写字母导出只表示 package consumer 可见；它不自动成为稳定 REST DTO、公开 SDK、Policy 管理接口或浏览器 capability payload。

## 1. 公开 surface 零变化

第 31 节没有注册任何以下 route 或同义 endpoint：

```text
POST   /api/v1/sessions
DELETE /api/v1/sessions/current
GET    /api/v1/me/capabilities
GET    /api/v1/policies
POST   /api/v1/policies
PUT    /api/v1/policies/:policy_id
POST   /api/v1/role-bindings
DELETE /api/v1/role-bindings/:binding_id
GET    /api/v1/governance/audit
```

也没有新增：

- GraphQL field、WebSocket event、MCP tool 或 Agent action；
- request/response JSON schema、OpenAPI schema 或 generated client；
- HTTP path/query/header/cookie、status code 或业务 error code；
- session/token/password/credential wire format；
- Policy/Role/Binding persistence schema、Migration 或 seed assignment；
- navigation、route、page、field、button 或 frontend capability projection；
- runtime config、secret/env、Compose service 或数据库/Redis/MQ grant；
- existing Marketing/Lottery handler 的 authorization hook。

现有公开 API 的路径、请求、响应和行为保持原样。本节不能被解释为旧 endpoint 已经自动受到 RBAC 保护。

## 2. 为什么内部模型不能直接暴露

当前课程还没有：

1. credential 到 verified session 和 trusted Principal 的边界；
2. 从权威 repository 加载 Resource tenant/owner/object facts 的 service layer；
3. 每请求执行 Policy 的 server-side enforcement point；
4. exact Policy repository、revision uniqueness、cache/撤权传播和发布审批；
5. internal reason/evidence 到 401/403/404/业务响应的低披露映射；
6. audit sink、日志脱敏、速率限制、CSRF/CORS 与浏览器越权 E2E。

在这些能力之前把 Go struct 直接 marshal 或把 evaluator 接到 handler，会把“形状合法”误写成“来源可信”，并可能允许 caller 自报 Principal、Role、Scope、tenant 或 owner。

## 3. Package ownership 与 import path

内部 contract 位于：

```go
import governance "github.com/Atingaii/GrowthOS-Go/internal/governance/domain"
```

第 31 节架构门禁要求仓库其他 production Go package 尚不得导入这个 package 或其子包；上述 import 只说明未来 consumer 的 Go 路径，不表示当前 runtime 已存在 consumer。

Governance domain 生产代码只导入：

```text
cmp | errors | fmt | slices | time
```

它不导入 HTTP/Gin、`context`、SQL、Redis、MQ、JWT、session、业务 bounded context 或 UI package。

## 4. Canonical identifier contract

### 4.1 Go 类型

```go
type PrincipalID string
type ResourceID string
type TenantID string
type RoleBindingID string
type PolicyID string
type AuditReference string

const MaxOpaqueIdentifierBytes = 128
```

构造器：

```go
func NewPrincipalID(string) (PrincipalID, error)
func NewResourceID(string) (ResourceID, error)
func NewTenantID(string) (TenantID, error)
func NewRoleBindingID(string) (RoleBindingID, error)
func NewPolicyID(string) (PolicyID, error)
func NewAuditReference(string) (AuditReference, error)
```

每种值都提供 `Validate() error` 与 `String() string`。

共同 grammar 为：

```text
single = [a-z0-9]
multi  = [a-z0-9][a-z0-9._:-]{0,126}[a-z0-9]
```

最大 128 bytes；不 trim、不 case-fold、不 Unicode-normalize。`*`、slash/backslash、uppercase、Unicode、首尾标点、空值或超限输入失败，失败返回对应零值。

`AuditReference` 同时承载 evaluation correlation 和 operation correlation。`EvaluationReference` 与 `CorrelationReference` 是 AuditContext accessor/字段语义，不是两个独立 Go 类型，也不能写成 `EvaluationRef`/`CorrelationRef` bearer token。

## 5. Closed enum contract

### 5.1 Principal 与资源目录

```go
type PrincipalKind string

const (
    PrincipalKindHuman   PrincipalKind = "human"
    PrincipalKindService PrincipalKind = "service"
    PrincipalKindAgent   PrincipalKind = "agent"
)

type ResourceKind string

const (
    ResourceKindCollection ResourceKind = "collection"
    ResourceKindObject     ResourceKind = "object"
)

type ResourceType string

const (
    ResourceTypeMarketingActivity   ResourceType = "marketing.activity"
    ResourceTypeLotteryStrategy     ResourceType = "lottery.strategy"
    ResourceTypeLotteryRoutingGraph ResourceType = "lottery.routing_graph"
    ResourceTypeGovernancePolicy    ResourceType = "governance.policy"
    ResourceTypeGovernanceAudit     ResourceType = "governance.audit"
)
```

### 5.2 Action、Scope 与 Effect

```go
type Action string

const (
    ActionCreate   Action = "create"
    ActionRead     Action = "read"
    ActionPublish  Action = "publish"
    ActionRollback Action = "rollback"
    ActionRetire   Action = "retire"
    ActionChange   Action = "change"
)

type ScopeKind string

const (
    ScopeKindSystem   ScopeKind = "system"
    ScopeKindTenant   ScopeKind = "tenant"
    ScopeKindOwned    ScopeKind = "owned"
    ScopeKindResource ScopeKind = "resource"
)

type BindingEffect string

const (
    BindingEffectAllow BindingEffect = "allow"
    BindingEffectDeny  BindingEffect = "deny"
)
```

所有 enum 都有 `Valid() bool`。它只校验单值是否属于 vocabulary；kind/type/action 组合还必须通过：

```go
func ValidateCapability(ResourceKind, ResourceType, Action) error
```

不存在 wildcard、prefix、regex、HTTP-method inference 或 unknown fallback。

## 6. Principal、Resource 与 Scope contract

### 6.1 Principal

```go
type Principal struct { /* private immutable fields */ }

func NewPrincipal(PrincipalKind, PrincipalID) (Principal, error)
func (Principal) Validate() error
func (Principal) Kind() PrincipalKind
func (Principal) ID() PrincipalID
```

构造成功只证明 shape canonical，不证明 caller 已认证。Kind 与 ID 一起参与 exact equality；相同 ID 的 human/service/agent 是不同 Principal。

### 6.2 Resource

```go
type Resource struct { /* private immutable fields */ }

func NewCollectionResource(ResourceType, TenantID) (Resource, error)

func NewObjectResource(
    resourceType ResourceType,
    id ResourceID,
    tenantID TenantID,
    owner Principal,
) (Resource, error)

func (Resource) Validate() error
func (Resource) Kind() ResourceKind
func (Resource) Type() ResourceType
func (Resource) ID() (ResourceID, bool)
func (Resource) TenantID() (TenantID, bool)
func (Resource) Owner() (Principal, bool)
```

Collection 不得携带 object ID/owner；object 必须有 ID。Tenant/owner 可缺失只表示权威对象确实没有该 fact，绝不表示 wildcard。构造器无法证明事实来源，未来第 33 节必须从可信 server repository 装配。

### 6.3 Scope

```go
type Scope struct { /* private closed union */ }

func NewSystemScope() Scope
func NewTenantScope(TenantID) (Scope, error)
func NewOwnedScope(TenantID) (Scope, error)
func NewResourceScope(ResourceType, ResourceID, TenantID) (Scope, error)

func (Scope) Validate() error
func (Scope) Kind() ScopeKind
func (Scope) TenantID() (TenantID, bool)
func (Scope) Resource() (ResourceType, ResourceID, bool)
```

语义：

- `system`：显式全局数据范围，但仍需 exact Permission；
- `tenant`：Resource 必须明确携带相同 tenant；
- `owned`：只匹配 object，且 tenant 与完整 owner Principal 同时 exact；
- `resource`：只匹配 object type/id，tenant 的存在性和值也必须 exact。

空 union、混合 foreign fields、缺失 facts 或 tenant mismatch 都不会 fallback system。

## 7. Permission、Role 与 RoleBinding contract

### 7.1 Permission

```go
type Permission struct { /* ResourceKind + ResourceType + Action */ }

func NewPermission(ResourceKind, ResourceType, Action) (Permission, error)
func (Permission) Validate() error
func (Permission) ResourceKind() ResourceKind
func (Permission) ResourceType() ResourceType
func (Permission) Action() Action
```

Permission 不含 Principal、Scope 或 allow/deny effect。它是 role capability ceiling 的一个 exact element。

### 7.2 Role

```go
type RoleID string

const (
    RolePlatformAdministrator RoleID = "platform_administrator"
    RoleMarketingOperator     RoleID = "marketing_operator"
    RoleLotteryDesigner       RoleID = "lottery_designer"
    RoleSecurityAuditor       RoleID = "security_auditor"
    RoleGrowthMember          RoleID = "growth_member"
)

const (
    MaxRolesPerPolicy      = 64
    MaxPermissionsPerRole  = 64
)

type Role struct { /* private ID + owned canonical permissions */ }

func NewRole(RoleID, []Permission) (Role, error)
func BaselineRoles() []Role
func (Role) Validate() error
func (Role) ID() RoleID
func (Role) Permissions() []Permission
```

`NewRole` 只接受对应 reviewed template ceiling 的子集。当前五个唯一 RoleID 中最大模板为 platform administrator 的 16 个 capability；64 是 defensive collection bound，不表示可构造 64 个不同有效 RoleID。`Permissions()` 与 `BaselineRoles()` 返回防御性副本。

### 7.3 RoleBinding

```go
type RoleBinding struct { /* private scalar values */ }

func NewRoleBinding(
    id RoleBindingID,
    principal Principal,
    roleID RoleID,
    scope Scope,
    effect BindingEffect,
) (RoleBinding, error)

func (RoleBinding) Validate() error
func (RoleBinding) ID() RoleBindingID
func (RoleBinding) Principal() Principal
func (RoleBinding) RoleID() RoleID
func (RoleBinding) Scope() Scope
func (RoleBinding) Effect() BindingEffect
```

RoleBinding effect 决定这个 role/scope association 是 grant 还是 restriction。Deny 只有在 Principal、role permission 和 scope 全部匹配本次 request 时才参与 precedence。

## 8. Policy snapshot contract

```go
type PolicyRevision uint64

type PolicyIdentity struct { /* PolicyID + non-zero revision */ }

func NewPolicyIdentity(PolicyID, PolicyRevision) (PolicyIdentity, error)
func (PolicyIdentity) Validate() error
func (PolicyIdentity) ID() PolicyID
func (PolicyIdentity) Revision() PolicyRevision

const MaxRoleBindingsPerPolicy = 1024

type Policy struct { /* private immutable snapshot */ }

func NewPolicy(
    identity PolicyIdentity,
    roles []Role,
    bindings []RoleBinding,
) (Policy, error)

func NewBaselinePolicy(
    identity PolicyIdentity,
    bindings []RoleBinding,
) (Policy, error)

func (Policy) Validate() error
func (Policy) Identity() PolicyIdentity
func (Policy) Roles() []Role
func (Policy) RoleBindings() []RoleBinding
```

构造器 deep-copy nested Roles、复制 bindings、规范排序，并拒绝：

- zero/non-canonical identity 或 revision 0；
- duplicate RoleID/BindingID；
- 同 `(Principal, RoleID, Scope, Effect)` 的 semantic duplicate；
- dangling RoleBinding；
- role template 外 permission；
- oversized collections 或 non-canonical forged internal state。

同一 Principal/Role/Scope 的 allow 与 deny 可以同时存在，用于 deterministic deny precedence。Revision 是 correlation value，不是 hash；本节没有 repository 来保证 `(PolicyID, Revision)` 全局唯一。

## 9. AuditContext 与 AuthorizationRequest contract

```go
type AuditContext struct { /* two AuditReference values + time.Time */ }

func NewAuditContext(
    evaluationReference AuditReference,
    correlationReference AuditReference,
    evaluatedAt time.Time,
) (AuditContext, error)

func (AuditContext) Validate() error
func (AuditContext) EvaluationReference() AuditReference
func (AuditContext) CorrelationReference() AuditReference
func (AuditContext) EvaluatedAt() time.Time
```

`evaluatedAt` 被规范为 UTC microsecond。AuditContext 不是 HTTP RequestID、trace context、session、credential、idempotency key 或 durable audit event。

```go
type AuthorizationRequest struct { /* private exact input */ }

func NewAuthorizationRequest(
    principal Principal,
    resource Resource,
    action Action,
    auditContext AuditContext,
) (AuthorizationRequest, error)

func (AuthorizationRequest) Validate() error
func (AuthorizationRequest) Principal() Principal
func (AuthorizationRequest) Resource() Resource
func (AuthorizationRequest) Action() Action
func (AuthorizationRequest) AuditContext() AuditContext
```

Request constructor 只验证 shape、catalog 与组合，不验证 Principal/Resource facts 的 authority。

## 10. Evaluation 与 Decision contract

### 10.1 调用

```go
func (Policy) Evaluate(AuthorizationRequest) (Decision, error)
```

调用协议：

```go
decision, err := policy.Evaluate(request)
if err != nil {
    // technical indeterminate: decision is exactly domain.Decision{}
    // a later enforcement layer must fail closed
}
if !decision.Allowed() {
    // confirmed deny; not a technical evaluation error
}
```

错误返回必须伴随 strict zero `Decision`。Confirmed default deny 返回 `nil` error，不能把 deny 与 technical failure 混为一谈。

### 10.2 Outcome 与 Reason

```go
type DecisionOutcome string

const (
    DecisionOutcomeAllow DecisionOutcome = "allow"
    DecisionOutcomeDeny  DecisionOutcome = "deny"
)

type DecisionReason string

const (
    DecisionReasonExplicitAllow             DecisionReason = "explicit_allow"
    DecisionReasonExplicitDeny              DecisionReason = "explicit_deny"
    DecisionReasonExplicitDenyOverrodeAllow DecisionReason = "explicit_deny_overrode_allow"
    DecisionReasonNoBinding                 DecisionReason = "no_binding"
    DecisionReasonNoPermission              DecisionReason = "no_permission"
    DecisionReasonScopeMismatch             DecisionReason = "scope_mismatch"
)
```

Reason 是 internal low-cardinality explanation，不是 HTTP error code。未来 transport 不得默认把这些值原样返回客户端。

### 10.3 Decision 与 match evidence

```go
const MaxDecisionMatches = MaxRoleBindingsPerPolicy // 1024

type DecisionMatch struct { /* private evidence fields */ }

func (DecisionMatch) Validate() error
func (DecisionMatch) BindingID() RoleBindingID
func (DecisionMatch) RoleID() RoleID
func (DecisionMatch) Effect() BindingEffect
func (DecisionMatch) ScopeKind() ScopeKind
func (DecisionMatch) Permission() Permission

type Decision struct { /* private confirmed result */ }

func (Decision) Validate() error
func (Decision) Confirmed() bool
func (Decision) Allowed() bool
func (Decision) Outcome() DecisionOutcome
func (Decision) Reason() DecisionReason
func (Decision) PolicyIdentity() PolicyIdentity
func (Decision) Principal() Principal
func (Decision) Resource() Resource
func (Decision) Action() Action
func (Decision) AuditContext() AuditContext
func (Decision) Matches() []DecisionMatch
```

Decision 保存 exact policy/request evidence；matches 规范排序、有界且防御复制。`explicit_deny_overrode_allow` 同时保留 allow 和 deny matches；default deny matches 为空。Decision 不是 authorization token、cache key、durable audit row 或 browser DTO。

## 11. Stable internal error categories

当前 package error vocabulary 精确为：

### 11.1 Value object

```text
ErrIdentifierInvalid
ErrPrincipalInvalid
ErrPrincipalKindUnsupported
ErrResourceInvalid
ErrResourceTypeUnsupported
ErrActionUnsupported
ErrCapabilityUnsupported
ErrScopeInvalid
ErrScopeKindUnsupported
ErrPermissionInvalid
ErrRoleInvalid
ErrRoleUnsupported
ErrRolePermissionDuplicate
ErrRolePermissionLimit
ErrAuditContextInvalid
```

### 11.2 Policy snapshot

```text
ErrRoleBindingInvalid
ErrBindingEffectUnsupported
ErrPolicyInvalid
ErrPolicyIdentityInvalid
ErrPolicyRevisionInvalid
ErrPolicyRoleDuplicate
ErrPolicyBindingDuplicate
ErrPolicyBindingConflict
ErrPolicyBindingRoleMissing
ErrPolicyRoleLimit
ErrPolicyBindingLimit
```

### 11.3 Evaluation

```text
ErrAuthorizationRequestInvalid
ErrAuthorizationEvaluationInvalid
ErrDecisionInvalid
```

这些值用于 Go `errors.Is` 分类和内部测试。第 31 节没有定义它们到 400/401/403/404/409/422/500/503 的任何网络映射，也没有批准将完整 `Error()` 文本返回终端用户。

## 12. Zero-value 与 immutability contract

所有有错误返回值的 constructor 在失败时返回其类型零值。`Policy.Evaluate` 的任何技术错误返回 exact `Decision{}`。

外部调用方不能依赖 private struct layout，也不能绕过 constructor 把 struct 当 schema。以下 getter 返回防御性副本：

- `Role.Permissions()`；
- `Policy.Roles()`，包括 nested permissions；
- `Policy.RoleBindings()`；
- `Decision.Matches()`；
- `BaselineRoles()` 每次调用的结果。

其余暴露值均为 immutable scalar/value object。相同 immutable Policy 支持并发只读求值；这不是对未来 repository/cache/adapter 线程安全的承诺。

## 13. Stopline：本节 contract 明确不含什么

`internal/governance/domain` 不得出现：

- Session、Credential、Password、Token、JWT、Cookie 或 Middleware；
- transport/HTTP/context、persistence、SQL、Redis、MQ 或 browser code；
- `RuleEngine`、`PolicyEngine`、expression engine、registry、plugin、DSL；
- `FactBag`/`AttributeBag`/`map[string]any` policy；
- `IsAdmin`、`SuperAdmin`、`AllowAll`；
- direct Principal-Permission assignment；
- generic wildcard/prefix capability；
- business use-case enforcement 或 runtime composition。

可复核：

```bash
go test -count=1 ./internal/governance/domain

rg -n 'github.com/Atingaii/GrowthOS-Go/internal/governance/domain' \
  --glob '*.go' --glob '!**/*_test.go' \
  --glob '!internal/governance/domain/**' .

rg -n 'internal/governance/domain|Policy\.Evaluate|NewAuthorizationRequest' \
  cmd internal/infrastructure web/src --glob '*.go' --glob '*.ts' --glob '*.tsx'
```

当前第 31 节 production runtime 预期零命中。后续章节如果有明确 ADR/课程范围，必须同步更新 architecture guard；不能静默删除 guard 来“让测试通过”。

## 14. 数据库、Compose 与 UI 状态

第 31 节没有 Migration 或 adapter。Policy、RoleBinding、Decision 与 audit evidence 没有任何 MySQL/PostgreSQL table；也没有 Redis key、RabbitMQ event 或 Compose service。现有基础设施可用不构成本节接入证据。

第 31 节没有 React component、route、navigation item、workspace projection 或 browser request。现有 Web test/build 只用于全仓回归，不能宣称已显示 role-specific UI。

## 15. 后续网络/API 演进

后续章节必须按依赖顺序另行冻结：

1. 第 32 节：credential/session → verified trusted Principal；
2. 第 33 节：trusted Resource/Policy assembly、每请求 `Evaluate`、fail-closed enforcement、低披露与 protected audit；
3. 第 34 节：服务端最小 capability projection，驱动导航/route/page/field/action UX；
4. 第 35 节：anonymous/expired/cross-role/cross-tenant/cross-owner/direct URL/API/browser E2E；
5. 第 36 节：首个真实运营后台消费已验收链路。

未来任何公开权限 API 还必须独立定义 wire ID/time format、pagination/filter、Policy concurrency、idempotency/replay、assignment approval、SoD、审计披露、rate limit 和 cache/撤权 semantics；不能从本文的 Go contract 自动推导。

## 16. API 结论

第 31 节的准确 API 表述是：

> 新增一个未装配、仅仓库内部可用的 Governance pure-domain Go contract；公开 HTTP/MCP/Agent/schema/UI surface 为零变化。

“已有登录”“已有权限管理接口”“现有 API 已鉴权”“浏览器已有角色菜单”“数据库已有 Policy 表”都属于超出证据的错误表述。
