# 第 31 节 QA：统一访问控制模型与威胁边界验收

- **课程主题：** 统一访问控制模型与威胁边界
- **需求基线：** [GrowthOS 统一访问控制模型与威胁边界 v1](../../product/access-control-model-threat-boundary-v1.md)
- **架构决策：** [ADR-0027](../../decisions/ADR-0027-governance-access-control-model.md)
- **路线修订：** [访问控制章节插入说明](../../course/route-revisions.md)
- **上一节：** [第 30 节 QA](lesson-30.md)
- **课程正文：** [第 31 节课程](../../course/part-04/lesson-31-access-control-model-threat-boundary.md)
- **API 记录：** [第 31 节 API 记录](../../api/lessons/lesson-31.md)
- **设计手记：** [第 31 节设计手记](../../design-thinking/lessons/lesson-31.md)
- **面试问答：** [第 31 节面试问答](../../interview/lessons/lesson-31.md)
- **运行手册：** [访问控制模型评审、紧急撤权分析与证据手册](../../runbooks/access-control-model-review.md)
- **证据日期：** 2026-08-31，Asia/Shanghai
- **当前候选：** `codex/lesson-31-access-control-model-threat-boundary`；本文编写时实现 tip 为 `c606de1`

> 本节验收的是 `internal/governance/domain` 中未装配的纯 Go 策略语言与决定内核。它可以对一份已经给定的 Principal、server-derived Resource、exact Action 和 immutable Policy snapshot 形成 confirmed allow/deny，或在技术上无法判定时返回 `Decision{}` + error。它不验收身份真实性、Policy repository、服务端 middleware/use-case enforcement、前端权限投影或浏览器越权闭环。

## 1. 证据状态词汇

| 状态 | 精确定义 |
| --- | --- |
| **IMPLEMENTED-SURFACE** | 当前候选已有实现和测试，可审查；不能单独替代命令证据 |
| **EXECUTED-AUTHORING-EVIDENCE** | 本文编写时已在实现 tip 上真实执行并 exit 0；文档或后续提交变化后仍需 root 最终复跑 |
| **ACTUAL-PASS** | root 已在当前完整候选上实际执行所列命令并 exit 0；只证明该命令覆盖面，不自动冻结 accepted/remote refs |
| **FINAL-GATE-PENDING** | 必须在章节全部文档、索引和冻结内容落盘后由 root 实际执行；本文不得预写通过 |
| **OUT-OF-SCOPE** | 本节刻意不实现，其他门禁通过也不能推导该能力存在 |
| **FORBIDDEN-CLAIM** | 证据边界明确禁止的表述；不能用“模型已实现”偷换成链路已安全 |
| **NOT-A-SLO** | 容量 guard、coverage 或 fuzz exec 只描述本次代码/执行，不是生产性能、可用性或安全合规指标 |

普通 `go test ./...` 可能使用 Go cache，不会自动持续运行 fuzz，也不能证明真实 session、HTTP enforcement、Policy 数据源、MySQL/Redis ACL、前端 UI 或浏览器攻击路径。每条结论必须绑定到它真正覆盖的命令和边界。

## 2. 本节已实现面

### 2.1 纯领域包

唯一新增生产包是：

```text
internal/governance/domain
```

它拥有以下统一词汇：

- `Principal`：`human | service | agent` 与 canonical `PrincipalID`；
- `Resource`：`collection | object`、封闭 `ResourceType`、可选 exact object/tenant/owner facts；
- `Action` 与 exact `Permission(ResourceKind, ResourceType, Action)`；
- 五个封闭 `RoleID` 及其 reviewed capability ceiling；
- `Scope(system | tenant | owned | resource)`；
- `RoleBinding(Principal, Role, Scope, allow|deny)`；
- immutable `PolicyIdentity` / `Policy` snapshot；
- `AuditReference` / `AuditContext`；
- `AuthorizationRequest`；
- confirmed `Decision`、内部 `DecisionReason` 与 bounded `DecisionMatch` evidence。

`Permission` 不携带 effect。Effect 属于 `RoleBinding`，因此一个 matching deny binding 会对该 role 在该 scope 内命中的精确 capability 产生 deny；它不是单独的 action-deny DTO，也不是外部策略表达式。

### 2.2 纯求值协议

`Policy.Evaluate(request)` 已实现：

1. 重新校验完整 Policy 与 AuthorizationRequest；
2. 只处理 exact Principal 的 bindings；
3. 解析 binding 引用的 exact Role；
4. 同时 exact 匹配 ResourceKind、ResourceType 与 Action；
5. exact 匹配 system/tenant/tenant-qualified-owned/exact-resource scope；
6. 保留所有 matching allow/deny evidence；
7. 任一 matching deny 覆盖 allow；
8. 没有 matching allow 时按原因默认拒绝；
9. 任一非法/损坏状态返回严格 zero `Decision` + error。

### 2.3 架构停止线

当前架构测试同时守住：

- Governance domain 生产代码只可导入 reviewed pure-domain allowlist：`cmp`、`errors`、`fmt`、`slices`、`time`；
- 不允许 generic engine/registry/DSL、untyped `map[string]any` policy bag、`isAdmin`/super-admin/allow-all shortcut、direct principal-permission grant；
- 不允许在本包提前声明 Session/Credential/Password/Token/JWT/Middleware；
- 仓库其他 production Go package 在第 31 节不得导入 Governance kernel 或其子包；
- 当前模型没有被装配进 runtime、transport、persistence 或业务 use case。

## 3. 本节没有实现

以下全部为 `OUT-OF-SCOPE`：

- 用户表、密码、SSO、OAuth、JWT、Cookie、session、CSRF、登录、注销、过期、续期、撤销和 session fixation 防护；
- credential 到 trusted Principal 的映射；
- Policy repository、assignment 管理、cache、watch、撤权传播、revision 全局唯一性或持久化 audit sink；
- Gin/HTTP middleware、handler decorator、application-service enforcement、trusted Resource fact loader；
- 任何现有 Marketing/Lottery API 的 RBAC 强制；
- 403/404/401、业务 error code、response body 与资源枚举低披露映射；
- 前端 capability projection、导航/路由/页面/字段/按钮裁剪；
- direct URL/API、跨角色、跨 tenant、跨 owner、浏览器 E2E；
- 多租户 lifecycle/storage isolation、数据库 row-level security、MySQL/Redis/RabbitMQ/PostgreSQL grant 变化；
- role hierarchy、session role activation、SoD、delegation、impersonation、ABAC/ReBAC、OPA/OpenFGA、XACML/DSL/plugin；
- 权限管理 UI、真实 Policy 变更审批、管理员防自提权；
- 生产 QPS、P95/P99、撤权时延、可用性、渗透或合规认证。

## 4. 禁止声称

即使本节所有测试通过，也禁止声称：

- “用户已经能登录并获得角色”；
- “API 已经受到 RBAC 保护”；
- “Marketing/Lottery 的越权漏洞已经闭环”；
- “不同身份已经看到不同页面”；
- “已经实现多租户/owner 数据隔离”；
- “管理员、运营、设计师、审计员和普通用户已绑定到真实账号”；
- “紧急 deny 已经发布到线上并实时撤权”；
- “Policy revision 已由数据库保证唯一”；
- “Decision 可以直接返回浏览器或当 bearer token”；
- “MySQL、Redis、RabbitMQ、PostgreSQL 或 Compose 已因本节发生运行时变化”；
- “通过单元测试即达到生产安全、性能或合规标准”。

准确表述只能是：

> Governance 已拥有一个有界、不可变、封闭目录、默认拒绝且 deny 优先的纯访问控制模型；真实信任来源与每请求服务端强制仍由第 32～35 节闭环。

## 5. 封闭目录验收

### 5.1 Principal、Resource 与 Action

| 目录 | 合法值 |
| --- | --- |
| PrincipalKind | `human`、`service`、`agent` |
| ResourceKind | `collection`、`object` |
| ResourceType | `marketing.activity`、`lottery.strategy`、`lottery.routing_graph`、`governance.policy`、`governance.audit` |
| Action | `create`、`read`、`publish`、`rollback`、`retire`、`change` |

知道单个枚举值不等于组合合法。独立矩阵测试必须证明总目录精确为以下 16 个 capability：

| ResourceType | collection | object |
| --- | --- | --- |
| `marketing.activity` | create、read | read、publish、rollback、retire |
| `lottery.strategy` | create、read | read |
| `lottery.routing_graph` | create、read | read |
| `governance.policy` | read | read、change |
| `governance.audit` | read | — |

所有其他 kind/type/action 组合都必须返回 `ErrCapabilityUnsupported`，不能因为 action 名称已知而被接受。collection/object 的同名 `read` 是两个不同 permission，不能跨 kind 命中。

### 5.2 Role capability ceiling

| RoleID | 精确能力上限 |
| --- | --- |
| `platform_administrator` | 上表全部 16 个 capability |
| `marketing_operator` | Activity collection create/read 与 object read/publish/rollback/retire；Strategy/Graph collection/object read |
| `lottery_designer` | Activity collection/object read；Strategy/Graph collection create/read 与 object read |
| `security_auditor` | Activity/Strategy/Graph collection/object read；Policy collection/object read；Audit collection read |
| `growth_member` | 空 capability ceiling |

Policy revision 可以把某个 role 缩减为 ceiling 子集，但不能给同名 role 注入表外 capability。`growth_member` 的空 Role 合法且明确默认拒绝；Role 模板存在不等于任何真实 Principal 已获得 assignment。

### 5.3 Scope truth table

| Scope | 必须匹配 | 明确不匹配 |
| --- | --- | --- |
| `system` | 任意合法 Resource；仍需 exact Permission | 缺失 Permission、非法 Resource/Action |
| `tenant(t)` | Resource 明确携带同一个 tenant `t` | tenant 缺失或不同 |
| `owned(t)` | object、tenant 等于 `t`、owner 完整 Principal 等于 request Principal | collection、tenant/owner 缺失、不同 tenant、不同 PrincipalKind 或 PrincipalID |
| `resource(type,id,tenant?)` | object type/id 完全一致，tenant 的存在性和值也完全一致 | collection、type/id 不同、一个有 tenant 一个无 tenant、tenant 不同 |

空 Scope、空 tenant、缺失 owner 或缺失 resource fact 都不能回退为 `system`。`owned` 一定 tenant-qualified；v1 没有跨 tenant owned。

## 6. Policy snapshot 与冲突验收

### 6.1 Identity 与容量

必须同时满足：

- `PolicyID` canonical 且 revision 为非零 `uint64`；
- revision 只是 snapshot correlation value，不是 content hash；
- 一个 Policy 最多 64 Roles，但封闭 RoleID 使当前最多只有 5 个唯一有效 role；
- 一个 Role 最多 64 Permissions，但当前最大模板只有 16 个；
- 一个 Policy 最多 1024 RoleBindings；
- 一次 Decision 最多 1024 matches；
- 超限在构造期失败，不先做无界 deep copy；
- 空 bindings 或空 role capability 可以形成合法 default-deny Policy。

这些数值是同步纯求值的 defensive guard，不是生产吞吐或租户规模承诺。

### 6.2 Canonicalization、duplicate 与 conflict

构造器必须：

- deep-copy Role permissions，复制 bindings，并分别按 RoleID/BindingID 规范排序；
- 拒绝重复 RoleID；
- 拒绝重复 BindingID；
- 拒绝同一 `(Principal, RoleID, Scope, Effect)` 的语义重复 binding，即使 BindingID 不同；
- 拒绝引用不存在 Role 的 dangling binding；
- 允许同一 Principal/Role/Scope 的 allow 与 deny 同时存在，以便 deny precedence 形成可解释冲突；
- 允许宽 allow 与窄 deny，也允许其他重叠；构造器不声称能证明“deny 必须比 allow 更窄”。

### 6.3 Immutability

必须篡改并复查：

- `NewRole` 的 caller permission slice；
- `Role.Permissions()` 返回 slice；
- `NewPolicy` 的 roles/bindings input slices；
- `Policy.Roles()` 的 nested permission slice；
- `Policy.RoleBindings()` 返回 slice；
- `Decision.Matches()` 返回 slice；
- `BaselineRoles()` 某次返回值。

后续读取与求值不得变化。所有字段都是 private scalar/value 或 owned slice；并发只读同一 Policy 不得出现 data race。

## 7. Decision 与 zero-result 验收

### 7.1 封闭结果

| Outcome | Reason | evidence 要求 |
| --- | --- | --- |
| allow | `explicit_allow` | 至少一个 allow match，deny 为零 |
| deny | `explicit_deny` | 至少一个 deny match，allow 为零 |
| deny | `explicit_deny_overrode_allow` | allow 与 deny 都至少一个 |
| deny | `no_binding` | 没有 Principal binding，matches 为空 |
| deny | `no_permission` | 有 Principal binding，但没有 exact capability，matches 为空 |
| deny | `scope_mismatch` | 有 exact capability，但没有 scope match，matches 为空 |
| error | 无 confirmed outcome | 返回严格 `Decision{}` |

default deny 是成功形成的 deny Decision，不是 error。损坏 Policy、非法 request、非法 capability、zero/partial audit context 或内部 contradictory evidence 是 technical indeterminate：必须 error + zero Decision，不能伪装为 `no_permission` 或 `scope_mismatch`。

调用方将来必须先检查 `err`。`Allowed()` 只对 confirmed allow 返回 true；`Confirmed()` 只验证领域 Decision 完整性。`Outcome()`/`Reason()` accessor 的存在不授权调用方忽略 `err`，也不构成 HTTP status/error code。

### 7.2 Evidence

一个 confirmed Decision 保存：

- exact PolicyID/Revision；
- exact Principal、Resource、Action；
- 完整 AuditContext；
- 每个 decisive match 的 BindingID、RoleID、BindingEffect、ScopeKind 和 exact Permission。

Evidence 按 BindingID 等字段规范排序、最多 1024 项并防御复制。deny-overrode-allow 必须同时保留两类 matches；默认拒绝不得伪造 match。Decision 不是 durable audit record，也不得默认 JSON 序列化给浏览器。

### 7.3 Deny threat matrix

实现测试必须至少覆盖：

| Allow | Deny | 预期 |
| --- | --- | --- |
| tenant A allow | tenant B deny | allow；只保留 tenant A match |
| Marketing role allow | 同 tenant Auditor role deny，且两者都有目标 permission | deny-overrode-allow；两个 match |
| exact-resource allow | system deny | deny-overrode-allow |
| owned allow | system deny | deny-overrode-allow |
| 同 scope allow | 同 scope deny | deny-overrode-allow |
| target permission allow | 空 `growth_member` deny | allow；没有 target permission 的 deny 不“投毒” |

这个矩阵证明 effect、capability 与 scope 必须全部 matching 后 deny 才生效；它不证明线上撤权传播，因为本节没有 Policy repository/cache/runtime consumer。

## 8. 规范值验收

`PrincipalID`、`ResourceID`、`TenantID`、`RoleBindingID`、`PolicyID` 与 `AuditReference` 共用：

```text
single = [a-z0-9]
multi  = [a-z0-9][a-z0-9._:-]{0,126}[a-z0-9]
```

并且最多 128 bytes。构造器不 trim、不 lowercase、不做 Unicode normalization；空值、首尾标点、uppercase、Unicode、`*`、`/`、`\` 或超长值失败并返回相应零值。

`EvaluationReference()` 与 `CorrelationReference()` 都返回 `AuditReference`。Evaluation/Correlation 是 AuditContext 字段语义，不是两个独立 Go 类型。`EvaluatedAt` 在构造时规范到 UTC microsecond；zero 或 forged non-canonical 时间失败。

## 9. 完整威胁矩阵

| 威胁 | 第 31 节可实测控制 | 本节状态 | 真正闭环 |
| --- | --- | --- | --- |
| 客户端伪造 Principal/role/scope | private fields、closed constructors、文档明确“可构造≠已认证” | 边界已定义；来源未可信 | L32 verified session + L33 trusted assembly |
| 替换 ActivityID/GraphID/StrategyID | exact object/tenant/owner/resource scope 语义 | 模型可判定；未接 use case | L33/L35 |
| 依赖 UUID 不可猜 | ID grammar 不赋予授权强度 | 已禁止错误假设 | L35 BOLA 负向攻击 |
| 隐藏按钮代替授权 | 架构停止线与零 UI 变更 | 已禁止装配/声称 | L33 server enforcement，L34/L35 UX/E2E |
| 新 action 默认放行 | closed capability catalog + default deny | 已实测 | L33 每个 use case 强制调用 |
| allow/deny 受输入顺序影响 | canonical sort + explicit deny precedence + unit/fuzz/race | 已实测 | Policy 发布流程仍待后续 |
| tenant fact 缺失查全量 | missing tenant/owner 不匹配，绝不 fallback system | 已实测 | L33 authoritative repository facts |
| service 代表 human 形成 confused deputy | PrincipalKind 分离 | 类型已形成；delegation 未实现 | 后续 delegation/tool policy ADR |
| stale Policy cache 延迟撤权 | exact Policy revision 写入 Decision | correlation 已形成；无 cache/repository | 引入 cache 时单独 ADR/验收 |
| 授权后对象或 Policy 改变 | exact request/policy evidence | 风险可追踪；无 transaction binding | L33 TOCTOU/use-case 设计 |
| 403/404/数量/耗时泄露对象存在 | reason 标记 internal、Decision 不是 DTO | 模型低披露边界已定义 | L33 mapping + L35 side-channel/E2E |
| 管理员给自己提权 | `governance.policy + object + change` 是受保护 capability | capability 已建模；管理流未实现 | 后续 SoD/Approval |
| audit 记录 credential/payload | Decision evidence 白名单且字段 private | 纯证据已受限；无 sink | L33 protected audit sink |
| Agent 继承 human 全权 | `agent` 是独立 PrincipalKind，无隐式继承 | 类型已实测；真实 Agent policy 未装配 | 后续 delegation/tool policy |

矩阵中“已实测”只说明纯领域行为。涉及真实 caller、数据源、网络、浏览器、Policy 发布或撤权传播的行都不能在第 31 节标为闭环。

## 10. 定向验证命令与真实记录

### 10.1 环境记录

本文编写时实际环境：

```text
go version go1.26.6 darwin/arm64
GOMOD=/Users/florian/Desktop/Tencent/Go/GrowthOS-Go/go.mod
GOWORK=
```

环境路径只记录本次本机证据，不构成其他机器要求。

### 10.2 普通测试、race 与 vet

```bash
go test -count=1 ./internal/governance/domain
go test -race -count=1 ./internal/governance/domain
go vet ./internal/governance/domain
```

**EXECUTED-AUTHORING-EVIDENCE：** 三条命令在实现 tip `c606de1` 上分别 exit 0；race 无 report。它们仍需在最终文档/索引候选上由 root 复跑，不能替代全仓验证。

### 10.3 Fuzz

先列出真实 target：

```bash
go test ./internal/governance/domain -list '^Fuzz'
```

当前只有两个：

```text
FuzzGovernanceIdentifiersNeverNormalizeOrAcceptWildcards
FuzzPolicyEvaluationDenyPrecedenceAndOrderIndependence
```

逐个执行：

```bash
go test ./internal/governance/domain -run '^$' \
  -fuzz '^FuzzGovernanceIdentifiersNeverNormalizeOrAcceptWildcards$' \
  -fuzztime=10s

go test ./internal/governance/domain -run '^$' \
  -fuzz '^FuzzPolicyEvaluationDenyPrecedenceAndOrderIndependence$' \
  -fuzztime=10s
```

**EXECUTED-AUTHORING-EVIDENCE：** 最新证据使用上面列出的两个精确 target 名，各运行 10 秒并 PASS：

- `FuzzGovernanceIdentifiersNeverNormalizeOrAcceptWildcards`：1,185,596 execs，0 new interesting，total 27；
- `FuzzPolicyEvaluationDenyPrecedenceAndOrderIndependence`：250,627 execs，2 new interesting，total 11。

此前曾以两个错误名称发起 fuzz 命令，Go 只输出 `no fuzz tests to fuzz`；那两次运行不计入 fuzz 证据。发现后先用 `-list '^Fuzz'` 校正为当前源码中的精确名称，再取得上述 10 秒结果。Exec/new/total 受 CPU、worker、cache 和 corpus 影响，只是本次记录，不是 KPI、coverage 或穷举证明。

### 10.4 Coverage

```bash
go test -count=1 -cover ./internal/governance/domain
```

**EXECUTED-AUTHORING-EVIDENCE：** 新增独立 capability/role/deny threat matrix 后，statement coverage 为 `92.5%`。临时 profile `/tmp/growthos-lesson31-final.cover` 已精确 unlink 并确认不存在。该数值不是 branch coverage，不覆盖真实认证、HTTP、database、browser 或生产 workload，也不是安全评分。

### 10.5 Architecture stopline

架构门禁随普通 package test 执行。还应精确复核生产面：

```bash
rg -n 'github.com/Atingaii/GrowthOS-Go/internal/governance/domain' \
  --glob '*.go' --glob '!**/*_test.go' \
  --glob '!internal/governance/domain/**' .

rg -n 'RuleEngine|PolicyEngine|ExpressionEngine|FactBag|AttributeBag|IsAdmin|SuperAdmin|AllowAll|PrincipalPermission|DirectPermission|Session|Credential|Password|Token|JWT|Middleware' \
  internal/governance/domain --glob '*.go' --glob '!**/*_test.go'

rg -n 'internal/governance/domain|Policy\.Evaluate|NewAuthorizationRequest' \
  cmd internal/infrastructure web/src --glob '*.go' --glob '*.ts' --glob '*.tsx'
```

第一个命令对当前 L31 production Go 应零命中；第二个命令对 Governance production Go 应零命中；第三个命令对 runtime/HTTP/Web 应零命中。测试、自身 package declaration、文档或明确后续章节产生的命中不能机械判定，必须查看路径与上下文。

### 10.6 Diff 与范围停止线

```bash
git diff --check
git status --short
git diff --name-only 6504e91..HEAD
git log --reverse --oneline 6504e91..HEAD
```

最终 root 必须确认：

- 第 31 节从第 30 节已验收 tip 线性创建；
- production 变化只落在 `internal/governance/domain`；
- 文档只落在本节计划范围；
- 没有 route、handler、session、migration、Compose、Web 或 runtime wiring；
- 没有覆盖或删除用户/并行代理的既有工作；
- disposable test artifact 已精确清理，未误删可复用依赖或用户数据。

**ACTUAL-PASS（最终证据候选）：** root 已对第 30 节基线 `6504e91` 到当前工作树执行 exact path whitelist，所有变化仅落在 `internal/governance/domain`、第 31 节六类正文/runbook、产品/ADR 以及明确列出的全局导航/架构索引文件。同时再次执行：

```bash
git diff --exit-code 6504e91..HEAD -- \
  cmd internal/infrastructure internal/lottery internal/marketing \
  internal/participation internal/platform migrations deploy web configs \
  scripts Makefile go.mod go.sum
```

命令 exit 0。三条 production stopline 搜索也均为零命中：没有外部 production package 导入 Governance kernel，没有在纯 domain 中混入 Session/Credential/Token/JWT/Middleware 等认证词汇，也没有 runtime/HTTP/Web wiring。`git diff --check 6504e91 --` exit 0。这证明最终证据候选没有改写既有 runtime/business/infrastructure/Web/module surface。证据候选 `a4097e4` 推送后，root 又独立核对了 remote 与线性历史；本次冻结提交只更新验收记录。

## 11. 最终全仓门禁

以下命令构成最终全仓门禁；状态必须逐条记录，不能把一条通过扩写成全部通过：

```bash
make fmt-check
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go run ./cmd/doccheck
make web-verify
make verify
git diff --check
```

| 命令 | 当前状态 | 实际证据/待项 |
| --- | --- | --- |
| `make fmt-check` | ACTUAL-PASS | root 在全部章节文档与全局索引落盘后独立执行，exit 0、无格式差异 |
| `go vet ./...` | ACTUAL-PASS | root 通过最终 `make verify` 实际执行，全仓 exit 0 |
| `go test -count=1 ./...` | ACTUAL-PASS | root 实际执行，全仓列出的 package 全部 PASS，17.3s |
| `go test -race -count=1 ./...` | ACTUAL-PASS | root 实际执行，全仓列出的 package 全部 PASS，18.6s，无 race report |
| `go run ./cmd/doccheck` | ACTUAL-PASS | root 在全部章节文档与全局索引落盘后通过最终 `make verify` 实际执行，documentation checks passed |
| `make web-verify` | ACTUAL-PASS | root 独立执行：19/19 test files、152/152 tests、TypeScript typecheck 与 Vite production build 全部通过；只证明既有 Web 回归，不证明 L31 权限 UI |
| `make verify` | ACTUAL-PASS | root 在全部代码、文档与全局索引提交后执行，Go vet/test、doccheck、Web test/typecheck/build 全部通过 |
| `git diff --check` | ACTUAL-PASS | root 对 `6504e91` 到最终证据候选的完整工作树执行，exit 0 |

`make verify` 可能包含部分前置命令，但冻结记录仍应说明实际执行入口和结果；已经 ACTUAL-PASS 的全仓 normal/race 不授权把其余命令预写为通过。

本节没有新增 Migration、MySQL adapter、Compose service、Redis/RabbitMQ/PostgreSQL integration 或浏览器页面，因此没有 L31 “真实数据库/Compose/UI acceptance”命令。不得为了显得完整而虚构这些运行面。

## 12. 故障与负向注入清单

| 注入 | 必须结果 | 禁止结果 |
| --- | --- | --- |
| unknown PrincipalKind/RoleID/ResourceType/Action/ScopeKind/Effect | constructor/Validate error + 对应零值 | trim/fallback/新建隐式值 |
| wildcard、slash、uppercase、Unicode 或超长 ID | `ErrIdentifierInvalid` | normalize 后接受 |
| individually known 但未注册 capability | `ErrCapabilityUnsupported` | action 已知即允许 |
| collection permission 对 object request | `no_permission` deny | 同名 read 跨 kind |
| tenant/owner fact 缺失 | scope mismatch deny | 回退 system |
| other-tenant exact/owned scope | scope mismatch deny | 只按 object ID 命中 |
| allow + matching deny | confirmed deny-overrode-allow | 按输入顺序取最后一个 |
| nonmatching deny | matching allow 保持 allow | 任意 deny 全局投毒 |
| duplicate Role/Binding/semantic binding | Policy construction error | 静默覆盖 |
| dangling binding role | Policy construction error | 求值时跳过并继续 allow |
| oversized Role/Permission/Binding collection | bounded error | 无界复制/扫描 |
| mutated caller/getter slice | snapshot/decision 不变 | 外部改写授权结果 |
| invalid Policy/request/audit time | error + strict `Decision{}` | confirmed default deny 或 partial evidence |
| no matching allow | confirmed default deny | implicit allow |
| concurrent reads of one Policy | deterministic equal decisions、race 无 report | 共享可变状态 |

## 13. 冻结检查单

- [x] 产品基线与 ADR 已冻结 Governance ownership、closed catalog、default deny、deny precedence 与 L31 停止线；
- [x] trusted value、role ceiling、Policy、Scope、evaluation、evidence 与 architecture guard 已形成 IMPLEMENTED-SURFACE；
- [x] deny threat matrix、independent capability/role matrix、capacity、immutability、并发与 zero-result fixture 已落盘；
- [x] 当前实现 tip 的 focused normal/race/vet 已实际 exit 0；
- [x] 两个 fuzz target 已各运行 10 秒并记录非 KPI 结果；
- [x] focused statement coverage 已实际记录为 92.5%；
- [x] root 已实际执行全仓 `go test -count=1 ./...` 与 `go test -race -count=1 ./...`，分别 17.3s/18.6s 并 PASS；
- [x] root 已对当前 committed candidate 执行 runtime/business/infrastructure/Web/module zero-diff 并 exit 0；
- [x] 课程正文、API、QA、设计手记、面试问答、runbook 与全局索引已在同一候选收口；
- [x] root 在最终候选执行 exact path whitelist、runtime zero-diff、architecture 三条精确停止线与 `git diff --check`，全部通过；
- [x] root 在最终文档态执行全仓 vet/doccheck/Web test/typecheck/build/聚合门禁并全部通过；
- [x] root 已精确列出并清理本轮生成的 `/tmp/growthos-lesson31-final.cover` 与 `web/dist`，未触碰依赖、Secrets、Docker 资源或用户数据；
- [x] 冻结前证据候选 `a4097e4`、远端同名 branch、第 30→31 节首提交父节点与本地/远端 `main` 已实际核对并一致；冻结记录提交推送后再做最终 ref 复核。

任一项目都只能在真实命令取得证据后勾选；冻结记录提交本身不改变已验收的代码、测试或架构范围。

## 14. QA 结论

截至本文冻结记录，纯 Governance model 的 focused normal/race/vet/fuzz/coverage、全仓 normal/race、最终 vet/doccheck/Web test/typecheck/build/`make verify`、exact path whitelist、runtime zero-diff、architecture stopline、`git diff --check`、disposable artifact cleanup，以及证据候选 `a4097e4` 的第 30→31 节线性历史、远端同名分支和 `main` 不变均已取得 `ACTUAL-PASS`。本次只写验收结果；提交并推送后还会在仓库外层再次核对最终 branch ref，并将累计实现分支严格快进到最终冻结点。

本节最终可宣称的上限是：

> 已实现并验证未装配的 Governance access-control policy kernel：closed exact capability、reviewed role ceiling、system/tenant/owned/resource scope、immutable bounded Policy、default deny、deny precedence、zero-result technical failure 与最小内部 evidence。真实 authentication、server enforcement、frontend projection 和越权 E2E 尚未实现。
