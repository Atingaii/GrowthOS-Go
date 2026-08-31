# 第 31 节：统一访问控制模型与威胁边界

> 本节从第 30 节已验收 tip `6504e91390e2601aa4a9742acc7dfb57f69f8662` 线性创建独立学习分支 `codex/lesson-31-access-control-model-threat-boundary`。本节只建立 Governance 访问控制语言和纯策略判定内核，不实现登录、会话、HTTP 强制、前端裁剪或浏览器越权验收。

## 1. 这一节为什么必须出现在运营后台之前

第 30 节以后，GrowthOS 已经有了多个风险不同的管理动作：

- 创建 Lottery Strategy 和 Routing Graph；
- 创建 Marketing Activity；
- 发布、回滚和退役 Activity；
- 读取策略、活动、政策和审计信息。

如果直接做运营后台，很容易把“页面上能看到什么”当成“服务端允许做什么”。这会留下两类常见越权：

1. **垂直越权：** 普通人员直接调用管理员动作；
2. **水平越权：** 有权操作某个对象的人，通过替换对象 ID 操作别人的对象或其他 tenant 的对象。

只在前端隐藏按钮不能建立安全边界，只检查一个 `role == admin` 也不能表达 collection、object、tenant、owner 和 exact resource 的差异。因此真实演进顺序必须是：

```text
L31 统一语言与纯决定
  -> L32 credential/session 产生可信 Principal
  -> L33 服务端加载可信 Resource facts 并强制决定
  -> L34 前端消费服务端 capability projection 改善体验
  -> L35 从浏览器和直接 API 做越权验收
  -> L36 首个真实运营后台复用前述能力
```

本节先解决“怎样表达并确定性回答一次访问问题”。没有这个基础，后续章节会各自发明 role、scope、错误码和审计字段，最终只能大重构。

## 2. 学习目标与诚实停止线

完成本节后，你应该能独立解释并在代码中定位：

1. 为什么认证、授权、审批、业务资格、业务状态和基础设施 ACL 是六类不同决定；
2. 为什么 Permission 必须包含 `ResourceKind + ResourceType + Action`；
3. 为什么 Role 只是能力上限，RoleBinding 才把 Principal、Role、Scope 和 effect 连起来；
4. `system`、`tenant`、`owned`、`resource` 四个 ScopeKind 如何 fail closed；
5. 为什么 matching deny 必须确定性覆盖 allow；
6. 为什么技术错误返回 `Decision{}`，而不是伪造成正常 deny 或第三种 outcome；
7. 为什么 Policy、Role 和 Decision 都要不可变、规范排序并携带 exact revision；
8. 什么证据能证明纯内核正确，什么证据仍不能证明任意真实 API 已受保护。

本节明确不做：

- 用户、密码、SSO、OAuth、JWT、Cookie、session、CSRF、注销、过期与撤销；
- credential 到可信 Principal 的映射；
- Gin middleware、handler、application decorator 或 composition root 装配；
- policy repository、Migration、Redis cache、MQ 或动态角色管理；
- 401/403/404 映射、资源存在性披露或审计落库；
- React 导航、路由、页面、字段和按钮裁剪；
- 直接 URL/API、跨角色、跨对象、跨 tenant 和浏览器 E2E；
- OPA、OpenFGA、Zanzibar、ABAC、ReBAC、RLS 或独立授权服务。

因此，本节结束时只能说“访问控制模型与纯判定内核已验收”，不能说“系统已经登录”“接口已有 RBAC”或“多租户安全已经完成”。

## 3. 先拆开六类看似相同的拒绝

一次 Activity 发布可能被多层拒绝：

| 问题 | 决定所有者 | 典型输出 | 不是它的东西 |
| --- | --- | --- | --- |
| caller 的 credential 是否有效 | L32 身份/会话 | trusted Principal 或 unauthenticated | 请求体 user ID、前端 role |
| Principal 能否执行 publish | Governance access control | allow/deny/error | 登录成功、会员等级、Approval |
| publication 是否获批 | Governance approval provider | approval evidence | role、access allow |
| Activity 当前是否开放 | Marketing gate | open/closed | authorization allow |
| 用户是否满足参与资格 | Participation | eligible/ineligible/error | 后台岗位权限 |
| 进程能否读写表或 key | Infrastructure ACL | SQL/command/key 权限 | 最终用户 Principal |

这些层都可能使请求失败，但责任人、证据、恢复动作、客户端披露和审计要求不同。把它们压成一个 boolean 会让事故定位和权限审查都失真。

## 4. 从一个访问问题推导最小词典

一次授权问题可以写成：

```text
在 exact Policy revision 下，
一个可信 Principal，
是否可以对服务端确认事实的 Resource，
执行一个 exact Action，
并且命中某个 RoleBinding 的 Scope？
```

由此得到本节的核心值对象：

```text
Principal
Resource
Action
Permission
Role
Scope
RoleBinding
PolicyIdentity + Policy
AuditContext + AuthorizationRequest
Decision + DecisionMatch
```

### 4.1 Principal 不是身份凭据

Principal 只有 `kind + opaque id`：

| PrincipalKind | 表达什么 | 不自动表达什么 |
| --- | --- | --- |
| `human` | 人类主体引用 | 密码已验证、属于某 tenant、拥有某岗位 |
| `service` | 服务主体引用 | 可以代表所有最终用户或所有 tenant |
| `agent` | AI/自动化主体引用 | 自动继承发起人的全部权限 |

`NewPrincipal` 只验证值的形状。攻击者知道一个合法 ID，不代表其已经成为这个 Principal。真正的信任建立属于第 32 节。

### 4.2 ResourceKind 是能力的一部分

Resource 分为：

- `collection`：尚无对象 ID 的集合动作，例如 create 或 list/read；
- `object`：必须携带 exact ResourceID 的对象动作，例如 read、publish、rollback、retire。

同一个 `read` 也必须区分 collection read 和 object read。允许列出 Activity 不代表允许读取任意 Activity 详情，反过来也一样。

Resource 可以携带 tenant 和 owner，但这些只是值对象中的事实位置。本节无法证明事实来自数据库还是浏览器；第 33 节必须从权威服务端来源加载它们。

### 4.3 Action 是业务动词，不是 HTTP method

封闭动作是：

```text
create / read / publish / rollback / retire / change
```

`POST /activities` 对应 create，`POST /activities/{id}/publish` 对应 publish。不能因为 HTTP method 相同就共享权限，也不能因为路由名称变化就产生新授权语义。

### 4.4 Permission 是 exact capability

Permission 是精确三元组：

```text
ResourceKind + ResourceType + Action
```

当前目录只有 16 个有效 tuple：

| ResourceType | collection | object |
| --- | --- | --- |
| `marketing.activity` | create、read | read、publish、rollback、retire |
| `lottery.strategy` | create、read | read |
| `lottery.routing_graph` | create、read | read |
| `governance.policy` | read | read、change |
| `governance.audit` | read | — |

不存在 `*`、前缀、正则或“未知 action 默认允许”。`2 kinds × 5 types × 6 actions` 的 60 组矩阵测试独立列出 16 个合法 tuple，其余全部必须被 `ErrCapabilityUnsupported` 拒绝。

## 5. Role 是责任模板上限，不是人员 assignment

v1 有五个封闭 RoleID：

| RoleID | 精确 Permission 数 | 责任边界 |
| --- | ---: | --- |
| `platform_administrator` | 16 | 当前目录全部精确 capability，无 wildcard |
| `marketing_operator` | 10 | Activity 操作；Strategy/Graph 只读 |
| `lottery_designer` | 8 | Strategy/Graph 创建读取；Activity 只读 |
| `security_auditor` | 9 | Activity/Strategy/Graph/Policy/Audit 只读 |
| `growth_member` | 0 | 当前运营资源默认无权限 |

这里要区分三个概念：

1. **模板上限：** 同名 Role 最多能有哪些 capability；
2. **Policy revision 内的 Role：** 可以使用模板的子集，不能把表外 capability 塞入同名 Role；
3. **RoleBinding：** 哪个 Principal 在哪个 Scope 下获得或被限制这个 Role。

因此，仅仅存在 `platform_administrator` 模板不会让任何真实人员成为管理员。当前没有 assignment repository，也没有 session role activation。

`MaxPermissionsPerRole=64` 是防御性容量 guard，不表示当前任一角色真有 64 项；真实模板最大只有 16。`MaxRolesPerPolicy=64` 也是防御性 guard，而封闭 RoleID 使当前有效 Policy 最多拥有 5 个唯一 Role。

## 6. Scope 同时回答功能范围与数据范围

四个真实枚举字面值只有：

```text
system / tenant / owned / resource
```

| ScopeKind | 精确匹配规则 | 失败时 |
| --- | --- | --- |
| `system` | 对任意合法 Resource 匹配，但仍需 exact Permission | 没有 Permission 仍拒绝 |
| `tenant` | Resource tenant 必须存在且等于绑定 tenant | tenant 缺失或不同即不匹配 |
| `owned` | 必须是 object；tenant 精确相等；owner 完整 Principal 精确相等 | 缺 tenant/owner、跨 tenant、kind/id 任一不同即不匹配 |
| `resource` | 必须是 exact object；type、ID、tenant 的存在性和值全部一致 | collection、ID/type/tenant 任一不同即不匹配 |

`owned` 是 tenant-qualified owned 语义，`resource` 是 exact-object resource 语义；这两个长名称不是代码中的 ScopeKind 字面值。

几个容易忽略的边界：

- human `operator-1` 与 service `operator-1` 是不同 Principal；
- tenant-qualified exact resource 不匹配缺 tenant 的同 ID 对象；
- system object 的 exact resource scope 不匹配带 tenant 的同 ID 对象；
- 空 tenant 不是 wildcard；
- collection 永远不能命中 `owned` 或 `resource`。

## 7. RoleBinding 为什么同时携带 allow 与 deny

RoleBinding 组合：

```text
RoleBindingID + Principal + RoleID + Scope + BindingEffect
```

BindingEffect 只有 `allow` 和 `deny`。effect 不属于 Permission，因为 Permission 描述角色能力目录；effect 属于一次 Principal/Role/Scope 关联。

典型配置是：

```text
tenant allow
  + exact resource deny
  = 该 tenant 大范围允许，但一个对象被明确限制
```

不过实现没有声明“deny 必须比 allow 更窄”。只要两个合法 binding 都命中，deny 就覆盖 allow。这样既能表达窄 restriction，也能表达紧急 system deny。配置治理必须另外检查误伤范围。

同 ID binding 被拒绝；完全相同的 Principal/Role/Scope/Effect 即使换一个 ID 也被拒绝；allow 与 deny 可以重叠，因为它们表达冲突组合语义。

## 8. Policy 是不可变、可关联的快照

Policy 的真实构造是：

```text
PolicyIdentity(PolicyID, non-zero PolicyRevision)
Policy(identity, roles, bindings)
```

构造器会：

- 先检查外层容量，避免先复制无界输入；
- 深复制 Role 与 Permission；
- 复制 RoleBinding；
- 按 RoleID、BindingID 规范排序；
- 拒绝重复 Role、Binding、语义重复 binding 和悬空 RoleID；
- 重新验证每个封闭值对象；
- 让 getter 返回防御性副本。

Policy revision 是非零 `uint64` correlation value。它不是 content hash，纯构造器也不能保证 `(PolicyID, Revision)` 在全局唯一；未来 repository 必须建立 exact identity 唯一性。

当前容量：

```text
MaxOpaqueIdentifierBytes = 128
MaxRolesPerPolicy = 64          // 防御性 guard，当前 RoleID 只有 5 个
MaxPermissionsPerRole = 64      // 防御性 guard，当前模板最大 16 个
MaxRoleBindingsPerPolicy = 1024
MaxDecisionMatches = 1024
```

有界和不可变让一个 Policy 可以安全地被并发只读求值。本节没有 repository、热更新或 cache，因此也不声称已经解决撤权传播时延。

## 9. AuditContext 是最小关联，不是审计系统

`AuditContext` 保存：

- evaluation reference：一次判定的 opaque 关联值；
- correlation reference：调用方操作分组的 opaque 关联值；
- evaluated-at：UTC、microsecond 精度的单一判定时刻。

两个 reference 的 Go 类型都叫 `AuditReference`；读取方法是 `EvaluationReference()` 和 `CorrelationReference()`。不要在文档或面试里虚构 `EvaluationRef`、`CorrelationRef` 两个公开类型。

AuditContext 不是：

- HTTP RequestID；
- trace/span；
- session 或 credential；
- 任意 metadata bag；
- 持久审计事件。

第 33 节才会决定怎样从可信 request/operation context 建立它，以及把哪些最小字段写入受保护 audit sink。

## 10. 纯判定算法

`Policy.Evaluate(request)` 的顺序是：

1. 重新验证 Policy；损坏即返回 `Decision{}` + error；
2. 重新验证 AuthorizationRequest；损坏即返回 `Decision{}` + error；
3. 选择 Principal 完全相等的 bindings；
4. 解析 binding 指向的 Role；
5. 选择 ResourceKind/ResourceType/Action 完全相等的 Permission；
6. 用 binding Scope 匹配服务端 Resource facts；
7. 收集有界、规范排序的 DecisionMatch；
8. matching deny 存在时覆盖 allow；
9. 没有 deny 且有 allow 时允许；
10. 其他情况按阶段形成可解释 default deny。

结果矩阵：

| Outcome | Reason | Evidence |
| --- | --- | --- |
| allow | `explicit_allow` | 至少一个 matching allow |
| deny | `explicit_deny` | 至少一个 matching deny，无 matching allow |
| deny | `explicit_deny_overrode_allow` | 同时保留 matching allow 与 deny |
| deny | `no_binding` | 无 Principal binding；无伪造 match |
| deny | `no_permission` | 有 binding、无 exact capability；无伪造 match |
| deny | `scope_mismatch` | 有 capability、无 scope match；无伪造 match |
| error + `Decision{}` | 无 reason/outcome | 输入或内部状态技术不可判定 |

技术不可判定不是第三个 `DecisionOutcome`。有效 Outcome 只有 allow/deny。未来强制层对 deny 和 error 都必须 fail closed，但指标、告警、重试、审计和客户端披露不同。

### 10.1 为什么 `Allowed()` 还要重新确认 Decision

零值 Decision 的 outcome 为空，不会被误认为 allow。`Allowed()` 只有在 outcome 是 allow 且整个 Decision 再次 `Validate()` 成功时才返回 true。这避免调用者只读取一个被部分伪造的字段。

### 10.2 DecisionMatch 必须包含 exact Permission

每个 match 保存：

```text
BindingID + RoleID + BindingEffect + ScopeKind + exact Permission
```

没有 exact Permission，就无法解释“同一 Role 中究竟哪项 capability 对这次请求起作用”。证据不保存 credential、Cookie、policy payload、资源内容或任意 metadata，避免把审计变成新的敏感数据泄露面。

## 11. 威胁边界图

```text
Browser / API client / Agent
        | untrusted credential, IDs, headers, role/scope claims
        v
L32 Identity boundary
        | verifies credential and creates trusted Principal
        v
L33 Trusted service layer
        | loads authoritative Resource facts and exact Policy
        v
Governance Policy.Evaluate
        | allow / deny / error + internal evidence
        v
Business use case
```

本节只实现图中 Policy language 与 `Evaluate`。当前仓库其他 production Go 文件不允许导入这个包，正是为了阻止学习顺序被“顺手接一个中间件”破坏。

## 12. 实现顺序：怎样避免一次大重构

本节按下列小步发生：

| 顺序 | 提交 | 解决的问题 |
| ---: | --- | --- |
| 1 | `c7e5248` | 从高价值动作、BOLA/BFLA 与信任边界写产品基线 |
| 2 | `ae78e02` | ADR 确定 Governance 所有权、默认拒绝与替代方案 |
| 3 | `35c0420` | 独立设计审查补 AuditContext、角色上限、tenant-owned、冲突证据和容量语义 |
| 4 | `34c3add` | 建 canonical ID、Principal、Resource、Action、Scope 等可信值位置 |
| 5 | `59eb2b0` | 建 exact Permission 与五种封闭 Role template ceiling |
| 6 | `6fdc7ee` | 建 RoleBinding、PolicyIdentity 和不可变 Policy snapshot |
| 7 | `a054eb1` | 建 AuthorizationRequest、Decision、evidence 和确定求值算法 |
| 8 | `6cae442` | 建 fuzz、race、architecture 与停止线证据 |
| 9 | `ea98390` | 根据独立代码审计补完整威胁矩阵、子包逃逸门禁和 BindingEffect 错误分类 |
| 10 | `c606de1` | 校准 Scope/AuditReference/evidence/容量文档术语 |

后续课程、QA、设计手记、面试和最终验收继续作为独立小提交产生。学习者应按表逐个 `git show`，而不是只看最后一个大 diff。

## 13. 架构门禁怎样保护本节停止线

`architecture_test.go` 建立两层负证：

### 13.1 Domain 内纯度

生产文件只允许经过评审的标准库 import：

```text
cmp / errors / fmt / slices / time
```

门禁会递归扫描子目录，并拒绝：

- 第三方包或其他标准库未经评审的依赖；
- RuleEngine、PolicyEngine、DSL、Registry、FactBag、AttributeBag 等泛化策略扩张；
- `map[string]any` 或 `map[string]interface{}` 形式的无类型策略 bag；
- IsAdmin、SuperAdmin、AllowAll；
- PrincipalPermission、DirectPermission；
- Session、Credential、Password、Token、JWT、Middleware。

门禁自身使用临时 fixture 证明这些反例确实能被检测，避免写出永远为绿的“装饰测试”。

### 13.2 Runtime 零装配

仓库中 Governance domain 外的 production Go 文件不能导入：

```text
internal/governance/domain
internal/governance/domain/任意未来子包
```

这条门禁不是说内核永远不能被调用，而是说第 31 节还没有可信 Principal、Resource facts 和 enforcement 位置。第 33 节必须在新章节、独立 ADR/测试和明确调用点中有意识地解除或改造这条门禁。

Git 章节 diff 还要单独核对没有修改 `cmd`、现有 runtime、Web、Migration、deploy 或 Compose。AST 测试不能证明所有非 Go 文件零变化，两类证据不能互相替代。

## 14. 关键测试不是“多”，而是能证伪错误设计

| 测试域 | 想证伪的错误 |
| --- | --- |
| 60 组 capability 矩阵 | 新 action/type/kind 因漏写 switch 而被默认接受 |
| 五角色独立真值表 | 同数量 capability 被“调包”，计数测试仍通过 |
| collection/object 双向 read | 只把 Action 当 capability，忽略 ResourceKind |
| tenant/owned/resource 边界 | tenant 缺失回退全局、同 ID 跨 PrincipalKind、collection 命中 object scope |
| deny 威胁矩阵 | deny 顺序依赖、不同 Role deny 无效、nonmatching deny 毒化 allow |
| Policy duplicate/capacity | 重复/悬空/无界配置进入求值 |
| zero/partial/forged values | Go 零值或包内伪造值绕过构造器 |
| defensive copy | 调用者修改输入/输出切片改变已发布策略或证据 |
| order independence | 配置顺序改变授权结果 |
| concurrent read + race | 共享不可变 Policy 在并发读取中出现竞态 |
| fuzz identifier/evaluator | 任意字符串、顺序和 tenant 组合打破封闭语义 |
| architecture fixtures | 停止线测试本身无法检测预期反例 |

定向命令：

```bash
go test -count=1 ./internal/governance/domain
go test -race -count=1 ./internal/governance/domain
go vet ./internal/governance/domain
go test -cover ./internal/governance/domain
go test -run '^$' -fuzz '^FuzzGovernanceIdentifiersNeverNormalizeOrAcceptWildcards$' -fuzztime=10s ./internal/governance/domain
go test -run '^$' -fuzz '^FuzzPolicyEvaluationDenyPrecedenceAndOrderIndependence$' -fuzztime=10s ./internal/governance/domain
```

最终章节冻结前还必须实际执行 `make doc-check`、`make verify`、全仓 race、章节 diff 和远端一致性核查。文档列出命令不等于命令已经执行；真实结果只在本节 QA 中登记。

## 15. 用一个例子重放判定

假设 human `operator-1`：

- 在 `tenant-a` 有 MarketingOperator allow；
- 对 `tenant-a/activity-9` 有 MarketingOperator deny；
- 请求对 `tenant-a/activity-9` 执行 publish。

判定步骤：

1. Principal 与两个 binding 都完全相同；
2. MarketingOperator 包含 object marketing.activity publish；
3. tenant allow 与 Resource tenant 匹配；
4. exact resource deny 与 type/ID/tenant 匹配；
5. 同时得到 allow 和 deny evidence；
6. 结果为 `deny / explicit_deny_overrode_allow`；
7. Decision 保留两个 match，并按 BindingID 规范排序。

如果 deny 指向 `tenant-b`，它不匹配，结果仍为 explicit allow；nonmatching deny 不能“毒化”一次合法 allow。

如果 request action 是 collection publish，这个 capability 本身不在目录，构造 AuthorizationRequest 就失败；它不会被包装成 no_permission deny。

## 16. 常见错误与正确说法

### 错误 1：我们实现了 RBAC，所以接口安全了

正确说法：本节实现 RBAC-inspired 的封闭 role + scope + deny 纯判定模型；没有 session 或 runtime caller，任何真实 endpoint 都尚未受它保护。

### 错误 2：Principal 构造成功就是认证成功

正确说法：构造器只校验 canonical shape；第 32 节才把 credential 验证结果映射为 trusted Principal。

### 错误 3：system scope 就是 super admin

正确说法：system 只扩大 binding 的资源范围，仍需精确 Role Permission；不存在 wildcard 或代码旁路。

### 错误 4：deny 一定必须比 allow 更窄

正确说法：窄 deny 覆盖宽 allow 是典型策略，不是构造器不变量；任意同时 matching 的 deny 都覆盖 allow。

### 错误 5：技术失败就是 deny

正确说法：enforcement 都要 fail closed，但 domain 语义不同。有效 deny 有 confirmed Decision；技术失败返回 zero Decision + error。

### 错误 6：前端拿到 Role 就能自己算所有权限

正确说法：前端属于不可信边界。第 34 节只能消费服务端 capability projection 改善 UX；服务端第 33 节的判定才是安全边界。

### 错误 7：tenant scope 等于多租户已经完成

正确说法：本节只有精确匹配语义；tenant lifecycle、membership、repository isolation、事实加载和越权 E2E 仍未实现。

## 17. 选型边界：什么时候需要重评

当前模型适合“模块化单体、封闭资源目录、最多 1024 bindings、同步内存求值”的阶段。出现以下证据时应另开 ADR：

| 信号 | 可能演进 |
| --- | --- |
| 资源关系形成组织/项目/文件夹深层图 | 评估 ReBAC/OpenFGA/Zanzibar 风格关系模型 |
| 大量可信动态属性与环境条件 | 评估 ABAC 或 OPA policy-as-code |
| 多服务必须共享并实时撤权 | 评估独立 PDP、缓存、revision/失效协议与可用性预算 |
| 单 Policy bindings 接近 1024 或求值延迟不可接受 | 建索引、预编译或外置引擎，先拿 profile/benchmark 证据 |
| 数据库本身需要 tenant 纵深隔离 | 评估 RLS，但仍不能替代业务 action authorization |
| 管理员自提权或高风险冲突成为真实问题 | 加 SoD、双人审批、break-glass 与受控恢复流程 |
| Agent 代表人类执行多步工具调用 | 设计 delegation、attenuation、tool policy 与短时证据 |

在这些信号出现前直接引入复杂授权平台，会提前承担网络依赖、策略部署、可用性、缓存一致性和运维成本；在信号出现后仍坚持线性扫描，也同样是不负责任。

## 18. 官方资料怎样校准本节

- [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)：校准 deny-by-default、每个请求服务端校验和最小权限；
- [OWASP API1:2023 Broken Object Level Authorization](https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/)：校准 object ID 替换与对象级检查；
- [OWASP API5:2023 Broken Function Level Authorization](https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/)：校准功能级/管理动作越权；
- [NIST RBAC 项目](https://csrc.nist.gov/projects/role-based-access-control)：校准 RBAC 术语，同时避免把 GrowthOS scope/deny 扩展冒充 Core RBAC；
- [NIST SP 800-162 ABAC](https://csrc.nist.gov/pubs/sp/800/162/upd2/final)：校准 ABAC 替代方案的属性、对象、操作和环境概念；
- [OWASP Multi-Tenant Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Multi_Tenant_Security_Cheat_Sheet.html)：校准 tenant context 不能由客户端任意声明；
- [RFC 9110 15.5.4](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.4)：403 的服务端可以用 404 隐藏资源存在性，但并非所有 forbidden 都必须返回 404；具体披露策略留给第 33 节。

官方资料说明通用风险和标准语义；当前仓库的具体 Role、Scope、容量和算法仍由产品基线、ADR、代码和测试共同决定。

## 19. 学习练习

### 练习 A：画出一次 publish 的六层决定

要求分别写出 authentication、authorization、approval、Marketing gate、Participation 和 Infrastructure ACL 的输入、输出和错误，不允许合并成一个 boolean。

### 练习 B：手算 deny precedence

为同一 Principal 创建 system allow、tenant deny、exact allow 三个 binding，分别对三个 tenant/object 组合手算 Decision 和 evidence，再用测试验证。

### 练习 C：设计一个非法 capability

尝试构造 collection publish、object create、governance.audit object read，解释为什么应在 request/policy 构造阶段报错，而不是进入 default deny。

### 练习 D：寻找信任边界漏洞

假设 HTTP handler 直接把 body 中的 `principal_id`、`tenant_id`、`owner_id` 传给本节构造器。解释为什么所有单元测试仍可能通过，但系统仍然越权；给第 32/33 节写出必须增加的边界。

### 练习 E：解释证据最小化

列出为什么 DecisionMatch 要保存 Permission，却不应保存 token、完整策略、资源内容和 arbitrary attributes。

## 20. 本节验收清单

- [x] 产品基线与 ADR 定义访问模型、威胁边界和停止线；
- [x] canonical ID、Principal、Resource、Action、Scope 和 AuditContext 已实现；
- [x] 16 个 exact capability 与五个 Role template ceiling 已实现；
- [x] RoleBinding、PolicyIdentity、不可变 Policy snapshot 已实现；
- [x] default deny、deny precedence、zero Decision + error 和 exact evidence 已实现；
- [x] 普通、race、fuzz、并发、defensive copy、容量与架构测试已建立；
- [x] 独立代码/测试/术语审查未发现 P0/P1，P2 缺口已最小修复；
- [ ] 课程、API、QA、设计手记、面试文档和 Runbook 全部完成并通过 doccheck；
- [ ] 全仓 `make verify`、race、章节 diff、构建产物清理和远端一致性最终通过；
- [ ] 章节状态、全局导航、分支检查点和累计分支在最终证据后更新。

最后三项只在真实执行后勾选。课程正文不会用计划命令冒充实际证据。

## 21. 下一节留下的问题

本节可以构造 `Principal{human, operator-1}`，却不能回答：

- caller 用什么 credential 证明自己是它；
- credential 怎样签发、存储、轮换、过期、撤销；
- browser 如何安全携带会话；
- Cookie、CSRF、固定会话、并发登录和注销如何处理；
- unauthenticated 与 authenticated-but-forbidden 如何区分；
- trusted Principal 怎样进入第 33 节服务端强制层。

这些问题组成第 32 节“真实会话认证”。第 32 节仍不会把前端隐藏按钮当作授权，也不会绕过第 31 节的 exact Policy Decision。

## 22. 可追溯入口

- 产品基线：[统一访问控制模型与威胁边界 v1](../../product/access-control-model-threat-boundary-v1.md)
- ADR：[ADR-0027](../../decisions/ADR-0027-governance-access-control-model.md)
- 领域代码：[`internal/governance/domain`](../../../internal/governance/domain)
- API 记录：[第 31 节 API](../../api/lessons/lesson-31.md)
- QA：[第 31 节验收](../../qa/lessons/lesson-31.md)
- 第一性原理手记：[第 31 节设计手记](../../design-thinking/lessons/lesson-31.md)
- 面试问答：[第 31 节面试问答](../../interview/lessons/lesson-31.md)
- 模型审查 Runbook：[访问控制模型审查](../../runbooks/access-control-model-review.md)
