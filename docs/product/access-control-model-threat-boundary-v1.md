# GrowthOS 统一访问控制模型与威胁边界 v1

> 状态：第 31 节基线。本文定义 Governance 拥有的统一授权语言、纯策略判定和后续安全实施的停止线。它不是登录系统、在线鉴权中间件、权限管理后台、多租户实现或生产安全证明。

## 1. 为什么现在建立这套模型

第 30 节已经出现了真正需要保护的高价值动作：创建 Strategy 与规则图、创建 Activity、发布、回滚和退役。继续在每个业务包里写 `isAdmin`、比较页面角色或相信请求头，会同时产生四类问题：

1. Marketing、Lottery 和未来 Benefit 会发明不同的角色与拒绝语义；
2. 能看到按钮的人会被误认为有权直接调用 API；
3. 只检查“能否进入功能”而不检查“能否操作这个对象”，形成 BOLA；
4. 会员等级、审批结果、业务资格、数据库账号会被误用为产品授权。

因此第 31 节先建立一个跨业务上下文复用、但由 Governance 拥有的授权决定模型。它回答：

> 一个已经由可信边界确认的 Principal，是否可以在当前 Policy revision 下，对一个由服务端确认事实的 Resource 执行一个精确 Action，并处于哪一个 Scope？

它不回答凭据是否真实、请求来自哪个 session、handler 是否调用了判定器、页面如何裁剪，也不回答业务对象是否满足发布、参与或发奖条件。

## 2. 本节交付与停止线

### 2.1 本节交付

- `Principal`、`Role`、`Permission`、`Resource`、`Action`、`Scope`、`RoleBinding`、`Policy` 与 `Decision` 的统一词汇；
- Human、Service、Agent 三类主体引用；
- GrowthOS 当前真实管理资源和动作的封闭目录；
- system、tenant、owned、exact-resource 四种封闭作用域；
- 默认拒绝、显式拒绝优先、精确匹配和确定性证据；
- 不可变且有 revision 的策略快照；
- 无匹配绑定、无匹配权限、scope 不匹配、显式拒绝、允许与技术不可判定的分离；
- 客户端、服务端事实、基础设施账号、外部 provider 与浏览器之间的威胁边界；
- 纯 Go domain 测试、race、fuzz 和架构停止线。

### 2.2 明确不交付

- 用户表、密码、SSO、OAuth、JWT、Cookie、session、CSRF、注销、过期或撤销；
- 从 credential 到可信 Principal 的映射；
- Gin middleware、handler、application-service decorator 或 composition-root 装配；
- MySQL/Redis/MQ schema、policy repository、策略缓存或动态角色管理；
- 401/403/404 的 HTTP 映射和资源存在性披露策略实现；
- React 导航、路由、页面、字段或按钮裁剪；
- 匿名、跨角色、跨对象、跨租户、直接 URL/API 或浏览器 E2E；
- 行级安全、完整多租户、ABAC/ReBAC、OPA、OpenFGA 或 Zanzibar；
- Governance 审批、职责分离和高风险二次确认的运行时实现。

## 3. 决定所有者与相邻决定

| 问题 | 决定所有者 | 结果 | 不能冒充它的事实 |
| --- | --- | --- | --- |
| credential 是否对应有效会话 | 第 32 节身份/会话能力 | trusted Principal 或 unauthenticated | 请求体 user ID、前端 role |
| Principal 能否执行资源动作 | Governance access control | allow/deny 或技术失败 | 登录成功、会员 tier、Approval |
| Activity candidate 是否批准 | Governance approval provider | approval evidence | role 或 access allow |
| Activity 当前是否可参与 | Marketing gate | open/closed | authorization allow |
| 用户是否有资格参与 | Participation | eligible/ineligible | role 或会员后台权限 |
| 规则图路由到哪个 Strategy | Lottery | exact target/path | caller 权限 |
| 数据库/Redis 连接能执行什么 | Infrastructure ACL | command/table/key capability | 产品 Principal/Role |

认证、授权、审批、业务资格和业务状态都可能拒绝一次操作，但证据、责任人、恢复方式和对外披露不同，不能压成一个 boolean。

## 4. 统一词典

### 4.1 Identity 与 Principal

`Identity` 是身份权威所管理的真实身份及其生命周期。GrowthOS 第 31 节不拥有 Identity，也不建用户表。

`Principal` 是授权请求中的最小主体引用，由 `kind + opaque id` 唯一标识：

| kind | 含义 | 典型来源 | 不能推导出的事实 |
| --- | --- | --- | --- |
| `human` | 人类操作者或用户 | 第 32 节已验证 session | 岗位、租户、会员 tier、owner |
| `service` | 服务到服务调用身份 | 后续可信工作负载身份 | 代表哪个最终用户、可访问所有租户 |
| `agent` | 被约束的 AI/自动化执行主体 | 后续 Agent credential/delegation | 发起人的全部权限、可绕过审批 |

Principal 不是认证证明。知道合法 Principal ID 不能证明 caller 就是它；构造器只保证值形状，可信来源由第 32 节建立。

### 4.2 Resource

Resource 是服务端授权的目标，分为 collection 和 object：

- collection 用于 `create` 等尚无对象 ID 的动作；
- object 必须有精确 ResourceID，用于 read/publish/rollback/retire 等对象级动作；
- Resource 可以携带 tenant 与 owner 事实；缺失事实不会扩大权限；
- tenant 与 owner 必须来自服务端可信数据源，不能从请求体、query、隐藏字段或本地状态直接采用。

当前封闭资源目录：

| ResourceType | 所有者 | 当前动作 |
| --- | --- | --- |
| `marketing.activity` | Marketing | `create/read/publish/rollback/retire` |
| `lottery.strategy` | Lottery | `create/read` |
| `lottery.routing_graph` | Lottery | `create/read` |
| `governance.policy` | Governance | `read/change` |
| `governance.audit` | Governance | `read` |

目录只描述授权语言，不表示这些公开 API、管理页面或审计仓储已经存在。新增资源或动作必须显式改代码、测试、文档和威胁矩阵；没有通配符和字符串前缀匹配。

### 4.3 Action

Action 表达业务意图，而不是 HTTP method 或页面名称。`POST /activities/{id}/publish` 的动作是 `publish`；不能因为都是 POST 就共享 `create` 权限，也不能通过改 method 绕过动作检查。

一个 Action 只在资源目录规定的组合中合法。例如 `marketing.activity + publish` 合法，`lottery.strategy + publish` 非法。非法组合在策略构造或请求构造时产生技术错误，不形成 deny 决定。

### 4.4 Permission、Role 与 RoleBinding

Permission 是三元组：

```text
effect + resource_type + action
```

effect 是 `allow` 或 `deny`。Role 是命名 Permission 集合；RoleBinding 把一个 Principal 绑定到一个 Role 和一个 Scope。v1 不允许把 Permission 直接绑定给 Principal，也不实现角色继承或 session 内角色激活。

这是一套使用 RBAC 词汇并增加 GrowthOS scope/deny 语义的应用模型，不应声称完整实现 NIST Core/Hierarchical/Constrained RBAC。

### 4.5 Scope

Scope 约束 RoleBinding 能影响哪些 Resource：

| Scope | 匹配规则 | 典型用途 | 缺失事实时 |
| --- | --- | --- | --- |
| `system` | 所有合法目标 | 平台管理员或受控服务 | 仍需精确 Permission |
| `tenant` | Resource.tenant 与绑定 tenant 完全一致 | 租户运营者 | 不匹配，默认拒绝 |
| `owned` | object.owner 与当前 Principal 完全一致 | 个人资源 | 不匹配，默认拒绝 |
| `resource` | kind、type、ID、tenant 完全一致 | 临时或精确对象授权 | collection 不匹配 |

Scope 不使用 `*`、父子继承、隐式全局、路径前缀或空值回退。`system` 是明确且高风险的全局范围，不是 scope 缺失时的默认值。

当前仓库还没有可信 tenant lifecycle、tenant membership 或 tenant-scoped repository，因此 tenant scope 只是可执行匹配语义和威胁边界，不等于已经实现多租户隔离。

## 5. 内置角色模板

v1 角色 ID 是封闭集合。它们是 Permission 模板，不包含人员、租户或对象绑定：

| Role | Activity | Strategy | Routing Graph | Policy | Audit |
| --- | --- | --- | --- | --- | --- |
| `platform_administrator` | create/read/publish/rollback/retire | create/read | create/read | read/change | read |
| `marketing_operator` | create/read/publish/rollback/retire | read | read | — | — |
| `lottery_designer` | read | create/read | create/read | — | — |
| `security_auditor` | read | read | read | read | read |
| `growth_member` | — | — | — | — | — |

关键解释：

- 表中是能力上限，真实可操作对象还由 RoleBinding scope 决定；
- `platform_administrator` 仍需显式列出每个 resource/action，不存在 `*:*`；
- `marketing_operator` 的 publish 权限不替代 Approval；两项都通过才可发布；
- `growth_member` 对当前运营资源没有权限，未来个人资源出现时再新增精确权限；
- `security_auditor` 只读并不意味着能看到 secret、credential、raw approval payload 或所有敏感字段；字段级披露仍需后续投影设计；
- 当前没有 assignment 数据源，因此没有任何真实人员因这张表自动获权。

## 6. 不可变 Policy snapshot

Policy 由以下内容组成：

```text
PolicyID + non-zero Revision + Roles + RoleBindings
```

构造时完成：

- ID、类型、组合和容量上限校验；
- RoleID、BindingID 唯一性校验；
- binding 引用的 role 必须存在；
- Permission 与 Binding 规范排序；
- 输入切片防御性复制；
- 同一个 role 内完全重复的 permission 被拒绝，但同一 capability 的 allow/deny 可以并存，以便显式冲突确定性求值。

Policy 对外只返回防御性副本。revision 进入每个 Decision，未来持久化、缓存、撤权与审计必须以 exact revision 为边界；本节没有 repository 或 cache。

## 7. 授权请求与判定算法

请求由以下最小事实组成：

```text
trusted Principal + server-derived Resource + exact Action + canonical EvaluatedAt
```

纯求值顺序：

1. 重新校验 Policy 与 Request；损坏或非法输入返回 `Decision{}` 与错误；
2. 只选择 Principal 完全相同的 bindings；
3. 解析 binding 指向的 role；
4. 只选择 resource type/action 完全相同的 permissions；
5. 用 binding scope 与 Resource 的 server facts 精确匹配；
6. 任一 matching explicit deny 存在时，结果为 deny；
7. 否则任一 matching allow 存在时，结果为 allow；
8. 否则形成有原因的 default deny。

算法不依赖输入顺序。deny precedence 是 GrowthOS v1 的明确应用策略，参考 AWS IAM 的一种成熟组合语义，但不声称复刻 AWS IAM。

### 7.1 封闭结果

| 结果 | reason | 含义 |
| --- | --- | --- |
| allow | `explicit_allow` | 至少一个精确 allow，且没有 matching deny |
| deny | `explicit_deny` | 至少一个精确 matching deny |
| deny | `no_binding` | Principal 没有任何 binding |
| deny | `no_permission` | 有 binding，但没有目标 capability |
| deny | `scope_mismatch` | 有 capability，但无 binding scope 匹配目标 |
| error + zero Decision | 无 | Policy/Request 损坏、非法 action/resource、非法时间等技术不可判定 |

deny 是成功形成的安全决定；error 表示没有形成决定。未来调用方对二者都必须 fail closed，但日志、指标、告警、重试和客户端披露不能混为一谈。

## 8. Decision 与内部证据

有效 Decision 保存：

- outcome 与内部 reason；
- exact PolicyID/Revision；
- Principal、Resource、Action 与 evaluated-at；
- 决定性 matching evidence：BindingID、RoleID、Effect、ScopeKind。

证据有固定容量上限、规范排序并以防御性副本返回。显式 deny 只保留决定性 deny matches；allow 只保留 allow matches；默认拒绝没有伪造 match。

Decision 不是 durable audit record，也不能直接 JSON 序列化给浏览器。后续可信 service layer 需要将内部 reason 映射为低披露错误，并把必要字段写入受保护审计 sink。禁止记录 token、Cookie、password、raw credential、会员事实 payload、Approval payload、Secret、完整策略或敏感对象内容。

## 9. 信任边界

```text
Browser / API client / Agent
        | untrusted credential, IDs, fields, headers
        v
Lesson 32 identity boundary  -- verifies --> Principal
        |
        v
Lesson 33 trusted service layer -- loads --> Resource facts + exact Policy
        |
        v
Governance Policy.Evaluate
        | allow/deny/error + internal evidence
        v
Business use case (Marketing / Lottery / ...)
```

第 31 节只实现图中最中间的纯判定内核。调用者即使能在 Go 代码里构造 Principal 或 Resource，也不代表输入已被认证或事实已被服务端确认。

## 10. 威胁清单与缓解安排

| 威胁 | 失败方式 | 第 31 节控制 | 后续闭环 |
| --- | --- | --- | --- |
| 客户端伪造 Principal/role/scope | 垂直越权 | 类型边界与“不可信构造≠认证”停止线 | L32/L33 |
| 更换 ActivityID/GraphID/StrategyID | BOLA/水平越权 | object、owner、tenant、exact scope 语义 | L33/L35 |
| 猜不到 UUID 就安全 | 未检查对象权限 | ID 不参与授权强度声明 | L35 负向攻击 |
| 隐藏按钮即授权 | 直接 API 绕过 | 明确 UI 非安全边界 | L33/L34/L35 |
| 新 action 未加策略 | 默认放行 | 封闭目录 + default deny | L33 架构门禁 |
| allow/deny 冲突随顺序变化 | 非确定提权 | 显式 deny 优先 + 规范排序 | L31 unit/fuzz/race |
| tenant fact 缺失时查全量 | 跨租户泄露 | 缺失不匹配，无空值全局回退 | L33 repository binding |
| 服务代表用户调用下游 | confused deputy | service 与 human Principal 分型 | 后续 delegation 设计 |
| policy cache 过旧 | 撤权延迟 | exact revision 进入 Decision | 引入 cache 时单独 ADR |
| 授权后对象/策略变化 | TOCTOU | exact evidence 与风险登记 | L33 use-case transaction 设计 |
| 403/404/数量/耗时泄露 | 资源枚举 | internal reason 不对外直出 | L33/L35 披露验收 |
| 管理员给自己提权 | 权限管理失控 | policy/change 自身是受保护动作 | 后续 SoD/Approval |
| 审计记录 credential/payload | 二次泄露 | 最小证据白名单 | 后续 audit sink |
| Agent 继承人类全部权限 | 自动化扩大爆炸半径 | Agent 独立 PrincipalKind | L86+ delegation/tool policy |

## 11. 低披露原则

内部 reason 用于安全运营，外部响应只应暴露完成客户端行为所需的最少信息：

- 未认证与已认证但无权属于不同内部类别；HTTP 映射留到第 33 节；
- 对敏感对象，RFC 9110 允许服务端用 404 隐藏被禁止资源的存在，但不是全局强制规则；
- 列表、计数、筛选项、字段和导出同样需要权限投影，隐藏详情页不足以阻止泄露；
- 前端 capability 只能改善体验，不能接收 internal reason、完整 role binding 或全量 policy；
- 日志必须能区分 deny 与 technical indeterminate，同时不能泄露凭据和敏感业务内容。

## 12. 容量与确定性边界

v1 在构造时限制 policy、role、permission、binding 和 evidence 数量。目的不是宣称性能 SLO，而是避免一个纯同步判定被无界配置拖垮，并让最坏复杂度可审查。

当前求值采用内存快照上的有界线性扫描，适合模块化单体和课程阶段。只有在出现真实策略规模、跨服务一致性、关系查询或独立治理需求后，才比较索引、缓存、OPA/OpenFGA 或外置 PDP。没有证据时提前引入网络策略服务，会先增加一致性、可用性、撤权和运维问题。

## 13. 第 32～35 节线性演进

| 章节 | 新增唯一核心能力 | 不能回写本节已完成 |
| --- | --- | --- |
| 32 真实会话认证 | credential → verified session → trusted Principal；过期、撤销、固定攻击、Cookie/CSRF | 不是 server RBAC |
| 33 服务端 RBAC 强制 | trusted service layer 每请求构造 Resource/Action 并执行 Policy；低披露与审计关联 | 不是 UI 安全 |
| 34 前端权限投影 | 服务端最小 capability 投影裁剪导航、路由、页面、字段和操作 | 不能替代 L33 |
| 35 越权与浏览器 E2E | anonymous/expired/cross-role/cross-object/direct URL/API/browser 负向证据 | 才能宣称链路验收 |

第 36 节首个运营后台只能消费上述已验收能力，不能再创建另一套页面角色判断。

## 14. 验收口径

第 31 节完成只能宣称：

- Governance 拥有统一、不可变、默认拒绝的纯授权模型；
- 当前资源/动作/角色模板与 scope 语义可执行；
- 冲突、缺失、非法值和技术不可判定有确定行为；
- 威胁边界和后续实施停止线进入文档、测试和架构门禁。

不能宣称：

- 用户已经能登录；
- 真实 API 已受 RBAC 保护；
- 不同角色已经看到不同 UI；
- 已完成多租户或对象级数据隔离；
- 已通过 direct API、浏览器或生产越权测试；
- 已达到安全合规、性能或可用性指标。

## 15. 官方参考

1. [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)：默认拒绝、最小权限、每请求校验与安全失败；每请求服务端执行留到第 33 节。
2. [OWASP API1:2023 Broken Object Level Authorization](https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/)：对象 ID 出现在请求中时仍必须执行对象级授权；攻击验收留到第 35 节。
3. [OWASP API5:2023 Broken Function Level Authorization](https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/)：管理功能必须默认拒绝并有一致的授权模块。
4. [NIST RBAC FAQ](https://csrc.nist.gov/Projects/role-based-access-control/faqs)：用于校准 user-role、role-permission、session activation、hierarchy 与 separation-of-duty 边界。
5. [NIST SP 800-162](https://csrc.nist.gov/pubs/sp/800/162/upd2/final)：用于校准 subject、object、operation 和环境属性的 ABAC 扩展语言；本节不实现 ABAC engine。
6. [OASIS XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html)：用于比较 Permit/Deny/NotApplicable/Indeterminate；GrowthOS 没有采用 XACML。
7. [RFC 9110 §15.5.4](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.4)：用于后续 403/404 低披露策略，不在本节定义 HTTP 映射。

