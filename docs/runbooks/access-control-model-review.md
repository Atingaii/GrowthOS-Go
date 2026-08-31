# 访问控制模型评审、紧急撤权分析与证据手册

- **适用课程：** [第 31 节：统一访问控制模型与威胁边界](../course/part-04/lesson-31-access-control-model-threat-boundary.md)
- **产品基线：** [GrowthOS 统一访问控制模型与威胁边界 v1](../product/access-control-model-threat-boundary-v1.md)
- **架构决策：** [ADR-0027](../decisions/ADR-0027-governance-access-control-model.md)
- **QA：** [第 31 节 QA](../qa/lessons/lesson-31.md)
- **API 记录：** [第 31 节 API 记录](../api/lessons/lesson-31.md)
- **设计手记：** [第 31 节设计手记](../design-thinking/lessons/lesson-31.md)
- **适用代码：** `internal/governance/domain`
- **版本：** v1，2026-08-31

> 这是一份纯模型变更与证据评审手册，不是线上授权事故操作手册。第 31 节没有 Policy repository/cache、assignment service、session、HTTP enforcement、audit sink 或 runtime deployment，所以本文不能执行线上撤权、刷新缓存、封禁账号、重启服务或宣称恢复生产。遇到真实线上事件，应先使用当时已经验收的身份、Policy 发布和服务端 enforcement 运行手册；如果这些能力尚不存在，只能升级人工安全处置，不能拿本手册冒充自动恢复程序。

## 1. 使用场景

本文用于四类工作：

1. 审查新增/修改 ResourceType、Action、capability、Role ceiling、Scope 或 decision semantics；
2. 在代码/配置进入后续 Policy 发布系统前，离线分析一项 proposed grant/restriction；
3. 模拟“紧急撤权”应如何用 exact deny RoleBinding 与新 Policy revision 表达，并验证影响面；
4. 保存可复核的测试、diff、威胁分析、证据与回滚计划。

本文不用于：

- 验证 caller 的密码、token、session 或身份；
- 直接编辑数据库、Redis、配置中心或生产 Policy；
- 把浏览器传来的 Principal/tenant/owner 当权威事实；
- 选择 HTTP 401/403/404；
- 修改前端菜单或按钮；
- 绕过审批/SoD；
- 证明 deny 已传播到线上请求；
- 对真实事故承诺 RTO/RPO 或撤权时延。

## 2. 角色与职责

在后续真实流程形成前，至少以评审职责区分：

| 职责 | 负责内容 | 不得单独完成 |
| --- | --- | --- |
| 变更提出者 | 描述业务动作、资源所有者、最小 scope、迁移影响 | 自己批准高风险扩大授权 |
| Governance reviewer | 核对 closed catalog、role ceiling、deny/default semantics | 把 UI visibility 当 server authorization |
| 业务上下文 owner | 证明 Resource fact 来源与业务 action 含义 | 直接让客户端提供 owner/tenant |
| 安全 reviewer | 威胁矩阵、BOLA/BFLA、低披露、撤权/回滚风险 | 用“管理员”字符串替代 exact permission |
| 验收执行者 | 在 exact candidate 上执行命令、记录环境与 exit | 预写通过或删除失败证据 |
| 最终冻结者 | 核对线性历史、范围、artifact cleanup、accepted/remote refs | 在证据不全时宣称生产可用 |

真实 assignment 管理、审批人/执行人 separation-of-duty 尚未实现；表格只是代码评审责任，不是系统强制的角色模型。

## 3. 评审输入包

任何模型变更开始前，提出者必须提供：

```text
Change reference:
Business resource owner:
Exact ResourceKind/ResourceType:
Exact Action:
Collection or object semantics:
Required tenant/owner/object facts:
Roles that need the capability:
Roles explicitly excluded:
Proposed allow/deny scopes:
Default-deny behavior when facts are absent:
Expected internal Decision reason/evidence:
Threats changed:
Future runtime/enforcement dependency:
Rollback trigger and rollback candidate:
```

若提出者只能说“管理员都能做”“前端隐藏即可”“先加 `*`”“先信 header/tenant ID”，评审应停止，不进入实现。

## 4. 当前模型基线

### 4.1 固定容量与 grammar

```text
MaxOpaqueIdentifierBytes   = 128
MaxRolesPerPolicy          = 64
MaxPermissionsPerRole      = 64
MaxRoleBindingsPerPolicy   = 1024
MaxDecisionMatches         = 1024
```

ID/AuditReference grammar：

```text
single = [a-z0-9]
multi  = [a-z0-9][a-z0-9._:-]{0,126}[a-z0-9]
```

不允许 trim、case-fold、Unicode normalize、wildcard 或 path semantics。64 roles/permissions 是 defensive collection guard；v1 只有 5 个唯一 RoleID，最大模板只有 16 个 permission。

### 4.2 决定规则

```text
invalid policy/request -> Decision{} + error
matching deny          -> confirmed deny
no deny + allow        -> confirmed allow
otherwise              -> confirmed default deny
```

Matching 必须同时满足 exact Principal、ResourceKind、ResourceType、Action 与 Scope。Effect 属于 RoleBinding，不属于 Permission。输入顺序不改变结果。

### 4.3 信任停止线

- Principal constructor 只证明 shape，不证明 authentication；
- Resource constructor 只证明 shape，不证明 tenant/owner/object facts 来自 server；
- Policy revision 只做 correlation，不是 hash，当前无 repository 保证全局唯一；
- Decision 不是 bearer token、browser DTO 或 durable audit record；
- internal reason/evidence 不对客户端直出；
- 当前没有 runtime consumer，任何离线 allow/deny 都不是线上执行事实。

## 5. Catalog 变更评审

### 5.1 新 ResourceType

逐项回答：

1. 哪个 bounded context 拥有这个资源？
2. 它是 collection、object，还是两者都有？
3. object 的 exact ID 来自哪里？
4. tenant/owner 是否是资源固有且可由权威 repository 加载的事实？
5. 缺失 fact 时是否明确 deny，而不是 system fallback？
6. read 是否包含 list、count、filter option、field、export 等不同披露面？若风险不同，应拆 action/资源而非依赖 UI；
7. 是否真的需要进入统一 Governance，而非业务 eligibility、Approval、Activity gate 或数据库 ACL？
8. 哪个后续 use case 会 server-side enforce？没有 consumer 仍可先建模，但不得宣称已保护。

### 5.2 新 Action

Action 必须是业务动词，不能直接用 `GET/POST/PUT/DELETE`。审查：

- collection/object 是否分别注册；
- read/create/change 是否过宽，是否应使用 publish/rollback/retire 等明确 verb；
- 新 action 在所有未更新 role 中是否 default deny；
- capability matrix 是否独立枚举所有合法与非法组合；
- Marketing Approval、Lottery eligibility、Activity gate 是否仍独立存在；
- 前端 operation 名称不能决定 server action。

### 5.3 Role ceiling

对每个 RoleID 生成 before/after exact permission set：

```text
RoleID:
Added permissions:
Removed permissions:
Reason each permission is required:
Collection/object distinction:
Least-privilege alternative considered:
Existing bindings affected:
Emergency deny interaction:
```

禁止：

- wildcard role；
- `isAdmin`/`SuperAdmin`/`AllowAll`；
- 因为 UI workspace 名称相同就自动授予全部 action；
- 给 Principal 直接绑定 Permission；
- 把 `growth_member` 临时改成运营角色；
- 把 role template 当真实 assignment 数据。

新增 RoleID 也要说明为什么现有职责组合不能满足，并评估 role explosion；不能只为了表达一次 exact-object exception 新建角色。

## 6. Scope 与 binding 评审

### 6.1 Scope 选择顺序

按最小权限从窄到宽考虑：

```text
exact resource -> tenant-qualified owned -> exact tenant -> explicit system
```

这不是 evaluator precedence；只是评审时的 least-privilege 顺序。Evaluator 对所有 matching binding 收集证据，再执行 deny precedence。

### 6.2 Scope 检查表

| Scope | 必查问题 |
| --- | --- |
| resource | type/id/tenant presence 是否 exact；对象删除/重建后 ID 语义是否仍安全 |
| owned | tenant 是否必填；owner 是否完整 Principal；owner 变化/转移时 TOCTOU 如何处理 |
| tenant | authoritative tenant fact 从哪里加载；list/count 是否也按 tenant 查询 |
| system | 为什么窄 scope 不足；谁批准；是否影响所有 current/future object |

`system` 必须被标记为高风险显式选择，绝不能由空 tenant/scope 推导。

### 6.3 Binding conflict

构造器会拒绝：

- 重复 BindingID；
- 同 `(Principal, RoleID, Scope, Effect)` 的语义重复；
- dangling RoleID；
- unsupported effect。

构造器允许 allow 与 deny 重叠，因为匹配 deny 必须覆盖 allow。评审者仍要分析：

- deny 是否命中与期望相同的 exact role capability；
- deny scope 是否意外比 allow 更宽；
- 同 role 的一个 deny 会限制该 role 在 scope 内所有命中的 permissions，而不是只限制评审者脑中某一个 action；
- 使用另一个 role 的 deny 时，该 role 是否确实拥有目标 permission；空 `growth_member` deny 不会“全局投毒”；
- nonmatching tenant/resource deny 不应改变合法 allow。

## 7. Policy snapshot 评审

### 7.1 构造前

- 选择新的非零 proposed revision；
- 明确它只是离线 correlation，当前不能宣称 repository uniqueness；
- 以 immutable full snapshot 表达变更，不原地改旧 Role/Binding slice；
- 检查 5 个封闭 RoleID、role ceiling 和 1024 bindings/evidence bound；
- 记录 old/new exact sets 与预期 Decision matrix。

### 7.2 构造后

检查：

- Roles/Bindings 已 canonical sort；
- input slice 修改不会影响 Policy；
- getter slice 修改不会影响 Policy；
- duplicate/conflict/dangling/oversized fixture 失败；
- 空 Policy 或空 role 明确 default deny；
- 每个 intended grant 都有正向和最邻近负向 case；
- 每个 high-risk grant 都有匹配 deny 与 nonmatching deny case；
- reversal of input order 不改变 Decision/evidence。

## 8. 紧急撤权模型分析

### 8.1 重要边界

本节只能分析一份 proposed emergency restriction：

```text
old immutable Policy snapshot
  + one reviewed deny RoleBinding
  -> new immutable proposed Policy snapshot/revision
  -> offline evaluation matrix
```

它不能发布到生产、刷新 cache、结束 session 或阻止真实 HTTP request。若有人报告“某主体正在越权”，应并行启动真实安全事件流程（冻结高风险外部入口、人工撤销 credential、保护证据等），而不是等待本模型分析完成；具体动作必须依赖当时已部署系统和正式授权，本文不授予操作权限。

### 8.2 选择 restriction

记录：

```text
Target PrincipalKind/PrincipalID:
Target RoleID:
Target capability(s) inside that role:
Target Resource facts:
Narrowest sufficient Scope:
Why existing allow matches:
Why proposed deny matches:
Expected unaffected scopes/actions:
Expected DecisionReason:
Proposed PolicyID/Revision:
Expiry/removal decision owner (process only; not implemented):
```

优先选 exact-resource deny；若影响同一 tenant 的多对象，再论证 tenant；只有无法用窄 scope 表达且风险接受时才考虑 system。Owned 只适合 tenant-qualified object ownership，不能表达任意列表或跨 tenant 撤权。

### 8.3 必跑撤权矩阵

对 proposed Policy 至少验证：

| Case | 预期 |
| --- | --- |
| target Principal + target capability + target scope | confirmed deny |
| target Principal + same capability + adjacent non-target resource/tenant | 原策略行为；不能被意外扩大 |
| different Principal + target resource | 原策略行为 |
| target Principal + different capability | 取决于 role-wide binding是否含该 permission；必须显式记录 |
| target Principal + missing tenant/owner fact | default deny，不 fallback system |
| allow 与 matching deny 同时存在 | `explicit_deny_overrode_allow`，保留两类 evidence |
| deny role 不含 target permission | deny 不参与 match，不能误判撤权成功 |
| bindings reverse order | outcome/reason/matches 完全一致 |
| invalid/oversized/duplicate proposed snapshot | `Decision{}` + error，不产生“尽力而为” Policy |

若“different capability”也被同一 role-wide deny 限制，这不是 evaluator bug，而是当前 binding effect 的明确语义。若业务只想撤销一个 action，需要重新设计 role/capability/binding 模型并走 ADR，不能假装 deny 是 per-action selector。

### 8.4 证据解释

一个 offline confirmed deny 应保存并核对：

- exact proposed PolicyID/Revision；
- exact Principal、Resource、Action；
- AuditContext 的两个 `AuditReference` 与 canonical evaluated-at；
- matching BindingID、RoleID、Effect、ScopeKind、Permission；
- `explicit_deny` 或 `explicit_deny_overrode_allow`。

这些证据只证明纯函数对给定输入的结果。它不证明：

- Principal 是真实 caller；
- Resource facts 来自权威 source；
- proposed revision 已发布或被 runtime 使用；
- old cache/session 已失效；
- 某个线上请求被阻止；
- evidence 已进入 durable protected audit sink。

## 9. 回滚分析

### 9.1 模型代码回滚

触发条件示例：

- closed catalog 意外扩大；
- role ceiling 与批准矩阵不一致；
- missing fact 发生 fallback；
- deny precedence/order independence 被破坏；
- zero-result 或 immutability 失效；
- architecture stopline 被绕过。

处理原则：

1. 停止冻结/合并，不隐藏失败测试；
2. 保存 exact failing candidate、命令、seed/corpus、diff 与最小复现；
3. 使用新的修复提交恢复不变量，不改写已推送学习历史；
4. 若必须撤销提交，使用可审查的 revert，不执行 destructive reset；
5. 复跑 focused normal/race/vet/fuzz/coverage、architecture 与全仓门禁；
6. 更新产品/ADR/API/QA/runbook，禁止代码语义与文档分叉。

### 9.2 Proposed Policy 回滚

未来有 repository 后，Policy snapshot 应 immutable，回滚应创建一个新的 revision，内容恢复到 reviewed old set，而不是覆盖旧 revision。当前第 31 节没有 repository，因此只能：

- 保存 old/new proposed snapshot fixture；
- 对 restored proposed snapshot 重跑完整 Decision matrix；
- 明确“离线回滚候选已验证”，不能声称“线上已回滚”。

回滚也必须保留 emergency deny 是否移除的明确决定。简单恢复旧 allow set 可能重新开放事件期间需要保留的 restriction。

### 9.3 不确定或证据不足

若无法证明 exact Policy revision、Principal/Resource facts 或 intended impact：

- 不形成 allow 结论；
- 模型求值错误保持 zero Decision；
- 真实 enforcement 未来必须 fail closed；
- 升级人工安全评审；
- 不通过 blind retry、换 latest revision、猜 tenant/owner 或忽略 invalid evidence 得出结果。

## 10. Evidence bundle

每次模型变更/撤权分析至少保存：

```text
Repository/branch/commit candidate:
Base accepted tip:
Product/ADR change reference:
Exact catalog/role/scope diff:
Old/new PolicyID + Revision (if fixture):
Threat matrix:
Positive and nearest-negative cases:
Expected Decision outcome/reason/matches:
Normal test command/result:
Race command/result:
Vet command/result:
Fuzz target/time/result (exec count marked non-KPI):
Statement coverage (marked non-security-score):
Architecture rg/AST result:
Full-repository gates:
git diff --check/status/range result:
Disposable artifact cleanup:
Accepted/remote/main/linear-history verification:
Unimplemented runtime dependencies:
Approvers/reviewers:
```

禁止在普通 evidence bundle 中记录 password、token、Cookie、raw credential、Secret、完整 Approval/会员 payload、完整 Policy dump或敏感业务对象内容。使用 stable opaque references 和最少必要 match fields。

## 11. 可执行 focused gates

### 11.1 Normal、race、vet

```bash
go test -count=1 ./internal/governance/domain
go test -race -count=1 ./internal/governance/domain
go vet ./internal/governance/domain
```

判定：全部 exit 0，race 无 report。它只覆盖 pure domain 的已有测试路径。

### 11.2 Fuzz

```bash
go test ./internal/governance/domain -list '^Fuzz'

go test ./internal/governance/domain -run '^$' \
  -fuzz '^FuzzGovernanceIdentifiersNeverNormalizeOrAcceptWildcards$' \
  -fuzztime=10s

go test ./internal/governance/domain -run '^$' \
  -fuzz '^FuzzPolicyEvaluationDenyPrecedenceAndOrderIndependence$' \
  -fuzztime=10s
```

每次先列 target，防止新增 target 被漏跑。记录实际时间、PASS/FAIL、seed/corpus；exec 数不固定，不设门槛。

### 11.3 Coverage

```bash
go test -count=1 -cover ./internal/governance/domain
```

记录实际 statement coverage，不把它解释为 branch/威胁穷举、生产风险分或合规证明。

## 12. Architecture 与停止线

### 12.1 AST guard

普通 domain test 包含自测 architecture guard，检查 pure import allowlist、forbidden vocabulary、untyped policy bag 和 premature runtime import。

### 12.2 精确源码复核

```bash
rg -n 'github.com/Atingaii/GrowthOS-Go/internal/governance/domain' \
  --glob '*.go' --glob '!**/*_test.go' \
  --glob '!internal/governance/domain/**' .

rg -n 'RuleEngine|PolicyEngine|ExpressionEngine|FactBag|AttributeBag|IsAdmin|SuperAdmin|AllowAll|PrincipalPermission|DirectPermission|Session|Credential|Password|Token|JWT|Middleware' \
  internal/governance/domain --glob '*.go' --glob '!**/*_test.go'

rg -n 'internal/governance/domain|Policy\.Evaluate|NewAuthorizationRequest' \
  cmd internal/infrastructure web/src --glob '*.go' --glob '*.ts' --glob '*.tsx'
```

在第 31 节 production candidate 上三条均应零命中。`rg` exit 1 在“预期无匹配”时是正常结果，记录时不要误写成测试失败。若命中未来章节代码，应先核对 branch/章节范围，而不是静默删除 guard。

### 12.3 Diff guard

```bash
git diff --check
git status --short
git diff --name-only 6504e91..HEAD
git log --reverse --oneline 6504e91..HEAD
```

禁止为得到 clean status 删除用户或其他代理文件。只清理由本任务明确产生、路径已解析的 disposable artifact；Go build/fuzz global cache 属于可复用依赖，除非任务创建了专用临时目录，否则不要广泛清空。

## 13. 最终全仓门禁

章节全部代码、课程、API、QA、设计、面试、runbook 与索引落盘后执行：

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

每条按实际 exit 记录；不要因 `make verify` 内部调用部分命令就虚构未执行的独立 gate。Web gate 只证明既有 UI regression，不证明 role-aware frontend。第 31 节没有 MySQL/Compose/browser acceptance surface，不应创建虚假命令或借用第 30 节数据库证据。

## 14. Review 决策

### 14.1 Approve model candidate

仅当以下全部满足：

- business owner、resource/action/scope 语义明确；
- closed catalog 与 independent matrix 同步；
- role ceiling 是最小权限且没有 shortcut/wildcard；
- missing fact/default deny/deny precedence/zero-result 保持；
- threat matrix 说明本节控制与后续闭环；
- immutability/capacity/order/concurrency evidence 完整；
- API/QA/设计/面试/runbook 术语与真实 Go contract 一致；
- focused 与最终 required gates 有真实记录；
- out-of-scope 和 forbidden claims 明确。

结论写作：

> 该 candidate 可以作为未装配 Governance model 的下一学习切片；它尚未认证 Principal、加载权威 Resource facts、发布 Policy 或强制任何线上请求。

### 14.2 Reject / revise

出现以下任一情况应退回：

- unknown/default allow；
- missing tenant/owner fallback system；
- matching deny 不能稳定覆盖 allow；
- effect 放进 Permission 与 binding 语义冲突；
- collection/object 或 type/action 模糊匹配；
- role ceiling 可注入表外 permission；
- caller/getter slice 可改变 snapshot/decision；
- technical error 返回 partial/confirmed Decision；
- internal reason/evidence 默认暴露给客户端；
- 提前加入 session/middleware/persistence/UI；
- 文档声称真实 runtime/DB/UI/线上撤权；
- 门禁未跑却写成通过。

## 15. 手册结论边界

本文能帮助团队一致地审查访问控制模型、构造离线紧急 deny 候选、验证决定证据并计划 immutable rollback。它不能执行线上处置。

第 31 节阶段最精确的说法是：

> 紧急撤权的 policy semantics 可以被纯模型分析和测试，但 credential/session 撤销、Policy 发布、cache invalidation、server enforcement、audit sink 与线上恢复仍未实现，必须由后续章节及正式事故流程承担。
