# ADR-0027：由 Governance 拥有统一、默认拒绝的访问控制模型

- 状态：已接受
- 日期：2026-08-31
- 关联章节：第 31 节“统一访问控制模型与威胁边界”
- 关联基线：[GrowthOS 统一访问控制模型与威胁边界 v1](../product/access-control-model-threat-boundary-v1.md)

## 背景

GrowthOS 已经拥有 Lottery Strategy/Graph 和 Marketing Activity publication 等真实业务资源。发布、回滚、退役、创建规则图等动作具有不同风险；未来运营人员、审计人员、平台管理员、普通用户、服务和 Agent 也不能看到或执行同一组能力。

当前前端 `UserProfile.role`、静态导航、会员 tier、Approval evidence、Activity gate、MySQL/Redis ACL 都不是统一产品授权。若在每个业务模块直接加入 `isAdmin`、角色字符串、HTTP header 或页面守卫，将造成策略复制、水平/垂直越权和错误语义混淆。

第 32～35 节已经按真实项目依赖顺序安排会话、服务端强制、前端投影和越权验收。本节必须提供这些章节共同依赖的语言与纯决定内核，同时严格停止在模型边界。

## 驱动因素

1. 一个新资源或动作在没有显式 grant 时必须拒绝；
2. Marketing、Lottery、Governance 和未来上下文必须共享同一授权语义，但仍保留业务所有权；
3. 功能级权限和对象/tenant/owner 数据范围必须同时表达；
4. allow/deny 冲突不能依赖配置顺序；
5. 技术不可判定不能伪装成正常业务拒绝，也不能意外允许；
6. 策略与判定必须可追溯 exact revision；
7. 客户端、前端、数据库 ACL、会员事实、审批和业务 gate 不能被误认为授权证明；
8. 第 31 节没有可信 session 和运行时装配，不能夸大成完整 RBAC 系统；
9. 实现应在模块化单体内保持简单、有界、不可变和可测试；
10. 后续若引入外置 PDP、缓存、多租户或 ReBAC，必须有真实规模与一致性证据。

## 决定

### 1. 所有权

统一 access decision 属于 `internal/governance/domain`。业务上下文拥有自己的资源事实和用例；未来 application/service layer 通过明确端口消费 Governance 判定，不在 Marketing、Lottery 或 `internal/platform` 重复角色判断。

`internal/platform` 只承载技术横切能力。把产品授权放在那里会掩盖策略的业务所有者，并容易与数据库/Redis ACL 混淆。

### 2. 纯领域模型

采用以下不可变对象：

- `Principal(kind, id)`；
- `Resource(kind, type, id?, tenant?, owner?)`；
- `Action`；
- `Permission(resourceKind, resourceType, action)`；
- `Role(id, permissions)`；
- `Scope(system|tenant|owned|resource)`；其中 `owned` 是 tenant-qualified owned，`resource` 是 exact-object resource；
- `RoleBinding(id, principal, role, scope, effect)`；
- `PolicyIdentity(policyID, non-zero revision)` 与 `Policy(identity, roles, bindings)`；
- `AuditReference` 与 `AuditContext(evaluationReference, correlationReference, evaluatedAt)`；
- `AuthorizationRequest(principal, resource, action, auditContext)`；
- `Decision(outcome, reason, exact policy/request evidence, matches)`。

模型不导入 HTTP、Gin、context、SQL、Redis、MQ、JWT 或任何业务上下文包。输入切片防御性复制并规范排序；策略 revision 非零；容量上限在构造期强制。ID/audit ref 最多 128 bytes 且使用 lowercase ASCII canonical grammar；时间是 UTC microsecond。`MaxRolesPerPolicy=64` 与 `MaxPermissionsPerRole=64` 是防御性容量 guard，不是当前有效基数：v1 只有 5 个唯一 RoleID，角色模板最大只有 16 个 Permission；每个 Policy 最多 1024 bindings/evidence。

### 3. 封闭目录

v1 使用精确的 PrincipalKind、RoleID、ResourceType、Action、ScopeKind、Effect、Outcome 与 Reason。当前资源/动作来自已经存在或本节真实拥有的 Activity、Strategy、Routing Graph、Policy 与 Audit 概念。

不支持：

- `*`、路径前缀、正则或模糊匹配；
- role hierarchy；
- implicit global scope；
- 直接 principal-permission binding；
- session role activation；
- environment expression；
- delegation/impersonation；
- 动态脚本策略。

新增枚举需要代码审查和负向测试，避免拼写错误静默扩大权限。

### 4. RBAC 与 Scope 的关系

Role 聚合精确到 ResourceKind/ResourceType/Action 的 permission；RoleBinding 将 Principal、Role、Scope 与 allow/deny effect 关联。Permission 回答“这一角色理论上能对哪类资源执行什么动作”，Scope 回答“这次绑定覆盖哪些具体目标”，binding effect 支持窄范围 restriction 覆盖较宽 grant。

五个封闭 role template 是构造器强制的 capability 上限；一个 Policy revision 可以使用模板子集，但不能给同名 role 注入表外权限。`growth_member` 的空模板是合法的显式默认拒绝角色。

`system`、`tenant`、`owned`、`resource` 四个 `ScopeKind` 是 GrowthOS v1 的应用扩展，不宣称属于 NIST Core RBAC；其中 owned 实现 tenant-qualified owned 语义，resource 实现 exact-object resource 语义。owned 必须同时匹配 exact tenant 与完整 Principal owner，禁止跨 tenant owned。窄 deny 覆盖宽 allow 是典型配置，而不是构造器强制的宽窄关系。没有第 32 节 session role activation，因此第 31 节只能称访问控制策略模型或 RBAC-inspired model。

### 5. 判定组合规则

`Policy.Evaluate` 执行 exact principal、capability 和 scope 匹配：

1. 匹配的 deny RoleBinding 覆盖所有 allow binding；
2. 没有 deny 且存在匹配 allow 时允许；
3. 其他情况按 no-binding、no-permission 或 scope-mismatch 默认拒绝；
4. policy/request 非法时返回 zero Decision 与错误。

deny precedence 是本项目的确定性策略，而非 Core RBAC 的必然组成。它使紧急限制或窄范围例外能够覆盖广范围 allow，但也要求配置变更检查意外 deny。

### 6. 决定与错误分离

有效 deny 是一个成功形成的 Decision。损坏策略、非法请求、无效时间或未知枚举表示没有形成决定，必须返回 `Decision{}` 与错误。

未来 enforcement 对两者均 fail closed，但：

- deny 进入授权拒绝指标与受控审计；
- technical indeterminate 进入故障指标、告警与恢复流程；
- 两者不得映射成 eligible/ineligible、Activity closed 或 Lottery no-reward；
- internal reason 不直接向客户端序列化。

### 7. 判定证据

Decision 固定携带 exact PolicyID/Revision、Principal、Resource、Action、纯领域 AuditContext 和决定性 BindingID/RoleID/Effect/ScopeKind/exact Permission match。证据规范排序、有界并防御性复制。deny 与 allow 同时匹配时两组证据都保留，reason 明确 `explicit_deny_overrode_allow`。

AuditContext 的 EvaluationRef/CorrelationRef 只是有界 opaque correlation，不是 HTTP RequestID、trace、session 或 credential。Decision 也不是持久化审计日志。第 33 节才定义 trusted service layer 如何关联真实 request/operation、做低披露映射并写入受保护审计 sink。

### 8. 角色模板

定义五个封闭角色模板：platform administrator、marketing operator、lottery designer、security auditor、growth member。每个模板只列精确 collection/object capability；即使 platform administrator 也没有 wildcard。角色构造器只接受模板能力上限的子集。

角色模板不带 assignment。本节没有身份目录或绑定 repository，所以不会自动给任何真实用户授权。

### 9. 信任边界

第 31 节构造器验证的是值形状和策略一致性，不是来源可信度：

- Principal 必须由第 32 节 verified session 产生；
- Resource tenant/owner/ID 必须由第 33 节 trusted service layer 从权威数据加载；
- role、scope、tenant、owner 不能接受浏览器声明；
- service/agent 不能默认继承 human 的全部权限；
- infrastructure credential 不映射成产品 Role；
- frontend visibility 不影响服务端 Decision。

## 备选方案

### 方案 A：每个 bounded context 自己检查角色

拒绝。初期文件少，但会复制角色字符串、scope 语义、拒绝原因与审计格式。跨上下文操作无法证明使用同一政策，新动作容易漏检。

### 方案 B：把 access control 放入 `internal/platform`

拒绝。产品授权决定依赖业务资源、动作与组织政策，不是日志或配置一类纯技术设施。platform 归属还会诱导开发者把 Redis/MySQL ACL 与 Principal Role 合并。

### 方案 C：只用简单 RBAC，不表达对象范围

拒绝。`marketing_operator` 能进入 Activity 功能不代表能操作任意 Activity；只做功能级角色会保留 BOLA、tenant 与 owner 越权。

### 方案 D：只用 ABAC 表达式

暂不采用。ABAC 能表达更多属性，但当前没有可信属性源、策略治理、解释器或复杂环境规则需求。先使用封闭 role + scope 可以降低策略错误与角色爆炸的早期风险；出现真实复杂条件后再扩展。

### 方案 E：引入 OPA/OpenFGA/独立授权服务

暂不采用。当前是模块化单体、策略规模小且无跨服务一致性证据。网络 PDP 会提前引入可用性、延迟、策略部署、缓存、撤权与运维成本。领域协议保持可移植，未来可通过 adapter 替换 evaluator。

### 方案 F：只在前端隐藏菜单和按钮

拒绝。客户端可修改状态并直接访问 URL/API。前端权限投影属于第 34 节 UX，服务端强制属于第 33 节安全边界。

### 方案 G：只用数据库行权限或基础设施 ACL

拒绝。连接账号代表进程，不代表最终 Principal；且业务动作如 publish/rollback、审批和低披露不能仅由表权限表达。基础设施最小权限仍保留为纵深防御。

### 方案 H：遇到 deny 一律返回 404

暂不决定。RFC 9110 允许用 404 隐藏被禁止资源，但不同资源的枚举风险、客户端恢复和审计要求不同。第 33 节按资源披露策略映射，domain 只保留内部 reason。

## 后果

### 正面

- 后续 session、服务端 enforcement 和前端 capability 使用同一资源/动作词典；
- 新资源与新动作默认关闭，且 typo 不会静默成为新权限；
- 功能、tenant、owner 和精确对象权限可以在一个确定算法中组合；
- allow/deny 冲突与证据不受输入顺序影响；
- deny 与技术故障可分别审计和处置；
- 业务资格、审批、发布 gate 与授权保持清晰边界；
- 后续引入外置 PDP 时可以保留 consumer contract。

### 代价

- 新资源/动作/角色需要显式改目录、测试和文档；
- 线性扫描只适合当前有界规模，不能冒充大规模授权服务；
- deny precedence 增加策略分析要求；
- tenant/owner Resource facts 的可信装配仍未解决；
- 当前没有 runtime caller，纯模型不能直接阻止任何公开 API；
- role hierarchy、SoD、delegation、field-level projection 等仍需未来需求驱动。

## 安全不变量

1. 没有匹配 allow 必须 deny；
2. 任一匹配 deny 必须覆盖 allow；
3. 非法或损坏输入不得产生有效 Decision；
4. 错误时必须返回 zero Decision；
5. resource/action 必须 exact 且属于封闭目录；
6. scope 缺失或 facts 缺失不得回退 system；
7. collection 不能匹配 `owned`/`resource`；
8. Policy/Role/Decision 输入输出必须不可变；
9. 决定不依赖 roles/bindings/permissions 输入顺序；
10. capability 必须精确匹配 ResourceKind、ResourceType 与 Action；
11. owned 必须同时匹配 tenant 与 Principal owner；
12. Decision 必须引用 exact Policy revision 和 AuditContext；
13. Revision 只是 correlation value，不是 content hash；未来 repository 必须保证 exact identity 唯一；
14. internal reason/evidence 不能被默认当作客户端 DTO；
15. Principal 构造成功不等于认证成功；
16. Role template 存在不等于任何真实主体已绑定；
17. UI 裁剪、数据库 ACL、会员 tier、approval 与业务 gate 都不能替代 Decision；
18. 本节不得新增公开路由、session、middleware、数据库或前端权限实现。

## 验收

- 领域 happy path、默认拒绝、显式拒绝优先和所有 scope 的表驱动测试；
- 非法枚举、非法 capability、重复值、悬空 role、容量上限、zero/partial 值测试；
- 输入顺序置换、并发只读、defensive copy 测试；
- fuzz 覆盖任意字符串、resource/request 和策略组合边界；
- 架构测试证明 Governance domain 不依赖 transport、persistence、session、Web 或业务上下文；
- 全仓普通、race、fuzz、coverage、doccheck 与 build 验收；
- API 文档明确零公开变化；
- QA、第一性原理设计手记、面试问答和模型变更运行手册完整；
- main 不变，第 31 节分支从第 30 节验收 tip 线性创建并独立推送。

## 后续演进

1. 第 32 节：真实 credential/session 到 trusted Principal；
2. 第 33 节：服务端 trusted service layer 强制 Policy Decision；
3. 第 34 节：服务端 capability projection 驱动前端导航/路由/操作裁剪；
4. 第 35 节：direct API、跨角色、跨对象和浏览器 E2E；
5. 第 36 节：首个真实运营后台复用上述能力；
6. 只有真实复杂性出现后，才新增 role hierarchy、SoD、delegation、ABAC/ReBAC、cache 或外置 PDP ADR。
