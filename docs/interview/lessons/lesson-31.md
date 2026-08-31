# 第 31 节面试问答：统一访问控制模型与威胁边界

- **课程主题：** 统一访问控制模型与威胁边界
- **产品基线：** [GrowthOS 统一访问控制模型与威胁边界 v1](../../product/access-control-model-threat-boundary-v1.md)
- **架构决策：** [ADR-0027](../../decisions/ADR-0027-governance-access-control-model.md)
- **核心代码：** [Governance access-control domain](../../../internal/governance/domain/doc.go)
- **资料访问日期：** 2026-08-31
- **题目数量：** 40 题

> 本文严格区分三类证据：项目行为以当前 Governance domain、产品基线和 ADR 为准；安全与标准结论只引用文末官方一手资料；牛客帖子只证明“有求职者记录过相近追问”，不证明公司官方题库、逐字原题或帖子答案正确。

## 1. 先记住本节的真实边界

### 1.1 60 秒项目自述

> 第 30 节已经形成 Activity 发布、回滚和退役等高风险动作，所以我在第 31 节没有先写页面角色判断，而是由 Governance 建立一套统一、默认拒绝的纯访问控制模型。它用 `Principal + Resource + Action` 表达精确请求，用五个封闭 Role 模板和 `system/tenant/owned/resource` Scope 表达 capability 上限与数据范围；Policy snapshot 不可变且带 exact revision，求值时 matching deny 覆盖 allow，无匹配则按 no-binding/no-permission/scope-mismatch 拒绝，损坏输入严格返回 zero Decision + error。实现通过表驱动、fuzz、race、defensive-copy 和架构停止线验证，但尚未接 session、HTTP、数据库、前端或运行时业务。第 32～35 节会依次完成真实会话、服务端强制、前端最小投影和越权 E2E。

### 1.2 事实账本与停止线

第 31 节交付的是一个未装配的纯 Go 访问控制策略内核。它已经能用封闭类型表达 Principal、Resource、Action、Permission、Role、Scope、RoleBinding、Policy、AuthorizationRequest 和 Decision，并确定性形成 allow、deny 或 zero Decision + error。

它还没有可信会话、HTTP middleware、application-service enforcement、Policy repository、动态权限后台、React 权限投影或浏览器越权 E2E。面试时必须把“模型能求值”和“线上请求已受保护”分开。

当前代码词典和数字如下，后文所有回答都以此为准：

| 维度 | 当前代码事实 |
| --- | --- |
| PrincipalKind | 3 个：`human`、`service`、`agent` |
| ResourceKind | 2 个：`collection`、`object` |
| ResourceType | 5 个：Activity、Strategy、Routing Graph、Policy、Audit |
| Action | 6 个：`create`、`read`、`publish`、`rollback`、`retire`、`change` |
| 合法 capability | 16 个精确的 kind/type/action 三元组 |
| RoleID | 5 个；模板 permission 数量依次为 16、10、8、9、0 |
| ScopeKind | 4 个代码值：`system`、`tenant`、`owned`、`resource` |
| BindingEffect | 2 个：`allow`、`deny` |
| DecisionOutcome | 2 个：`allow`、`deny` |
| DecisionReason | 6 个：3 个显式结果原因、3 个默认拒绝原因 |
| 技术不可判定 | error + 严格 `Decision{}`，不是第三种 outcome |
| 容量 guard | ID/ref 128 bytes；64 roles；每 role 64 permissions；1024 bindings/matches |

其中 64 roles 和每 role 64 permissions 是防御性容量 guard，不是当前有效词典的实际基数。v1 只有 5 个唯一 RoleID，单个模板的实际能力上限也不会超过 16。

## 2. 精准面试问答

### Q1：第 31 节到底解决了什么问题，又刻意没有解决什么？

**面试官意图**

判断候选人是否会把一个领域模型夸大成完整权限系统，以及是否理解真实项目要按依赖顺序演进。

**候选人回答**

第 31 节先统一“授权问题的语言和纯判定”。输入是一份已经可信的 `Principal`、由服务端确认事实的 `Resource`、精确 `Action`、不可变 `Policy` snapshot 和最小 `AuditContext`；输出是确认的 allow/deny `Decision`，或者在输入、策略损坏时返回 `Decision{}` + error。

本节没有验证 credential，也没有让任何 HTTP handler 调用 `Policy.Evaluate`。因此它不能证明用户可以登录、API 已受 RBAC 保护、不同角色已看到不同页面，或完成了多租户隔离。第 32～35 节分别补可信会话、服务端强制、前端投影和越权 E2E。

**追问与权衡**

为什么不一步做到 UI？因为 UI 裁剪依赖稳定 capability 词典，服务端强制又依赖可信 Principal。反过来先做菜单角色，会让浏览器状态变成事实源，并在后续接服务端时发生大重构。当前先做纯内核，代价是本节尚不能保护任何线上入口，但边界可独立测试和审查。

**常见误区**

- 说“第 31 节已经完成 RBAC 登录系统”；
- 把 domain 构造器能创建 Principal 说成已经认证 caller；
- 把架构停止线误解成缺功能，而不是有意阻止未可信装配。

**项目证据：** [包级停止线](../../../internal/governance/domain/doc.go)、[产品基线“本节交付与停止线”](../../product/access-control-model-threat-boundary-v1.md)、[运行时未装配架构测试](../../../internal/governance/domain/architecture_test.go)。

**选型边界：** 当前答案只对“纯模型章节”成立；接入 credential、handler 或真实业务写入后，必须分别补 L32 身份和 L33 enforcement，不能继续依赖构造器来源假设。

**来源：** `项目事实`：[ADR-0027 的决定与后续演进](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)。

### Q2：认证、授权、审批、业务资格和基础设施 ACL 有什么区别？

**面试官意图**

检查候选人能否拆开多个都会“拒绝请求”、但所有者和证据完全不同的决定。

**候选人回答**

认证回答 credential 是否映射为有效会话和可信 Principal；授权回答这个 Principal 能否对 Resource 执行 Action；审批回答一个 exact 业务 candidate 是否经过治理流程；业务资格回答用户是否满足活动参与条件；Activity gate 回答活动此刻是否开放；MySQL/Redis ACL 回答进程账号能执行哪些表或命令。

这些决定不能压成一个 `allowed bool`。例如 Marketing Operator 有 publish permission，不代表 candidate 已获 Approval；数据库账号有 UPDATE，也不代表最终用户有发布权；会员 premium 是 Lottery 路由事实，不是后台 Role。

**追问与权衡**

如果一个请求同时需要授权和审批，应怎样组合？由未来可信 service layer 分别取得两份证据，并在业务用例中全部通过才执行；任何一项失败都 fail closed，但审计分类、客户端披露、告警和恢复方式必须分开。本节只提供授权决定，不实现组合。

**常见误区**

- 用“登录成功”推导可访问所有对象；
- 把 Approval evidence 当 bearer permission；
- 把数据库最小权限当产品级用户权限系统。

**项目证据：** [产品基线“决定所有者与相邻决定”](../../product/access-control-model-threat-boundary-v1.md)、[AuthorizationRequest 与 Decision](../../../internal/governance/domain/evaluation.go)。

**选型边界：** 当一个用例需要授权、审批和业务 gate 同时成立时，应由 application service 显式编排三类决定；不能扩张 `Policy.Evaluate` 去拥有审批或资格。

**来源：** `项目事实`：[ADR-0027 背景与驱动因素](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) 与 [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)。

### Q3：为什么统一访问决定归 Governance，而不是 `internal/platform` 或各业务包？

**面试官意图**

考察 bounded context 所有权和横切能力是否一定属于技术平台层。

**候选人回答**

访问决定包含角色、业务资源、动作、scope、组织政策和拒绝语义，是业务治理语言，因此由 `internal/governance/domain` 拥有。Marketing 和 Lottery 仍拥有自己的资源事实与业务用例；未来它们通过明确端口消费 Governance Decision，而不是复制 `isAdmin` 或角色字符串。

`internal/platform` 更适合日志、配置、时钟等技术能力。若把产品授权放进去，很容易把 Principal Role 与数据库账号、Redis ACL 混为一谈，也会掩盖策略变更的业务责任人。

**追问与权衡**

统一所有权是否意味着 Governance 拥有所有业务对象？不是。Governance 只拥有授权语言和策略决定；Resource 的 tenant、owner、ID 等事实仍由资源所属上下文从权威存储加载。统一语义不等于集中数据所有权，也不要求立刻拆成独立网络服务。

**常见误区**

- 认为“横切”必然等于 platform utility；
- 让 Governance 直接查询所有业务表；
- 每个 handler 自己比较 `role == "admin"`，再称为领域自治。

**项目证据：** [ADR-0027“所有权”决定](../../decisions/ADR-0027-governance-access-control-model.md)、[Governance domain 包边界](../../../internal/governance/domain/doc.go)。

**选型边界：** 若未来出现跨服务策略发布、独立治理团队和可验证的一致性需求，可把 evaluator 放到外置 PDP adapter 后；Resource facts 的业务所有权仍不随部署方式转移。

**来源：** `项目事实`：[访问控制产品基线](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[NIST RBAC FAQ](https://csrc.nist.gov/Projects/role-based-access-control/faqs)。

### Q4：`Principal` 是什么？为什么构造成功不等于认证成功？

**面试官意图**

确认候选人理解“值合法”和“来源可信”是两件事。

**候选人回答**

`Principal` 是授权请求中的最小主体引用，由 `PrincipalKind + PrincipalID` 组成。当前 kind 封闭为 `human`、`service`、`agent`。`NewPrincipal` 只检查 kind 和 ID 的规范形状，无法证明当前网络请求真的由该主体发起。

可信度必须在第 32 节身份边界建立：credential 被验证、session 有效且未过期/撤销后，才能生成 trusted Principal。请求体、query、header 或前端 store 中传来的 PrincipalID 都只是非可信声明。

**追问与权衡**

为什么不让 Principal 携带 token？因为 token 是认证证明和敏感凭据，会把 credential 生命周期、解析与泄露风险带入纯授权模型。授权内核只需要稳定主体引用；认证 adapter 负责把外部身份映射成内部 canonical ID。

**常见误区**

- 把知道某个 PrincipalID 当作拥有该身份；
- 在 Decision 或日志中保存 raw token；
- 把 human、service、agent 合成一个字符串并默认等权。

**项目证据：** [Principal 实现](../../../internal/governance/domain/principal.go)、[Principal/Resource/Scope 负向测试](../../../internal/governance/domain/principal_resource_scope_test.go)。

**选型边界：** `NewPrincipal` 永远只验证内部值形状；只有 L32 identity adapter 才能赋予来源可信度，不能通过给构造器增加 token 参数越过章节边界。

**来源：** `项目事实`：[ADR-0027“信任边界”](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[NIST SP 800-207](https://csrc.nist.gov/pubs/sp/800/207/final)。

### Q5：为什么要区分 `human`、`service` 和 `agent` 三类 Principal？

**面试官意图**

考察 confused deputy、工作负载身份和 AI Agent 爆炸半径意识。

**候选人回答**

三类主体的信任来源和授权风险不同。human 通常来自用户会话；service 来自受控工作负载身份；agent 是被约束的自动化执行主体。service 不能因为“系统内部调用”就天然拥有 system scope，agent 也不能自动继承发起人的全部权限。

分类不会自动完成 delegation，但能阻止模型在一开始就把所有主体压成 `userID`。未来需要“代表用户”时，应显式携带调用主体、最终主体、委托范围和审计证据，而不是覆盖 Principal。

**追问与权衡**

为什么不现在实现 impersonation/delegation？当前没有真实会话和跨服务调用证据，提前设计会产生无法验证的复杂协议。v1 先保留类型边界；等出现真实代理操作，再新增独立 ADR、生命周期与撤销语义。

**常见误区**

- 给 service account 默认管理员权限；
- 让 Agent 使用创建者永久全量权限；
- 日志只记录最终用户，丢失实际执行者。

**项目证据：** [PrincipalKind 封闭枚举](../../../internal/governance/domain/principal.go)、[产品基线 Principal 词典](../../product/access-control-model-threat-boundary-v1.md)。

**选型边界：** 出现真实“代表用户操作”后，需要单独建模 delegation/impersonation、范围、期限与撤销；三种 kind 本身不提供委托协议。

**来源：** `项目事实`：[ADR-0027 不支持 delegation 的决定](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP ASVS V8 Authorization](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md)。

### Q6：`ResourceKind` 为什么也是 capability 的一部分？

**面试官意图**

判断候选人是否理解 collection 和 object 的安全语义，而不只会列资源名和动作。

**候选人回答**

`Permission` 是 `ResourceKind + ResourceType + Action` 精确三元组。collection 表示尚无具体对象的集合目标，适合 create 或 collection read；object 必须有 exact ResourceID，适合 read、publish、rollback、retire 等对象级动作。

如果只校验 `ResourceType + Action`，collection read 和 object read 会被错误合并，未来列表、计数或导出权限可能泄露全量数据；object create 也可能被误注册。`ValidateCapability` 因而同时检查 kind、type、action 组合。

**追问与权衡**

为什么没有单独 `list` Action？v1 选择 collection + read 表达当前最小需求，降低词典规模。如果未来列表与聚合统计的披露策略不同，应新增精确 Action，而不是在 handler 里偷偷解释同一个 read。

**常见误区**

- 认为相同 HTTP GET 就是相同 permission；
- 给 object ID 加密后省略对象级授权；
- 把 collection 当没有 ID 的 object 并允许 scope 空值回退。

**项目证据：** [ResourceKind 与构造器](../../../internal/governance/domain/resource.go)、[capability 目录](../../../internal/governance/domain/capability.go)、[合法/非法组合测试](../../../internal/governance/domain/capability_test.go)。

**选型边界：** 若列表、计数、搜索或导出出现不同披露风险，应新增更精确 Action；不能继续让 collection read 承载语义已经不同的操作。

**来源：** `项目事实`：[产品基线 Resource 目录](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[OWASP API1:2023 BOLA](https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/)。

### Q7：为什么 `Action` 是业务动词，而不是 HTTP method 或页面名称？

**面试官意图**

检查候选人能否把传输语义与业务授权语义分离。

**候选人回答**

`Action` 当前封闭为 `create/read/publish/rollback/retire/change`。它表达业务意图：例如 `POST /activities/{id}/publish` 对应 publish，不因为使用 POST 就获得 create 权限；`governance.policy` 的 object change 也不能被普通 read 覆盖。

页面名称同样不稳定。一个页面可能读取多个 Resource，也可能包含 publish 和 rollback 两个风险不同的操作。授权必须在服务端按实际 use case 选择精确 Action，不能按“进入了活动页”一次放行。

**追问与权衡**

新业务动作怎样加入？同时修改封闭枚举、合法 capability 目录、角色模板上限、负向测试、文档和威胁矩阵。新增动作默认没有 grant，从而保持 fail closed。

**常见误区**

- `POST == create`、`GET == read`；
- 用前端 route path 当 ResourceType；
- 为省事新增 `manage` 覆盖所有写动作。

**项目证据：** [Action 与 ValidateCapability](../../../internal/governance/domain/capability.go)、[capability table tests](../../../internal/governance/domain/capability_test.go)。

**选型边界：** 新业务动词只有在风险、审计或角色分配确实不同于既有 Action 时才新增；若只是传输 method 改变，不应扩张授权词典。

**来源：** `项目事实`：[ADR-0027“封闭目录”](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP API5:2023 BFLA](https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/)。

### Q8：Permission、Role、Scope 和 RoleBinding 分别回答什么？

**面试官意图**

确认候选人真正理解模型组合，而不是只会背“用户—角色—权限”。

**候选人回答**

`Permission` 回答某角色理论上可以对哪种 kind/type 执行什么 Action；`Role` 是一个经过评审的 Permission 集合；`Scope` 限制一次角色绑定可影响哪些具体 Resource；`RoleBinding` 再把 Principal、RoleID、Scope 和 `BindingEffect` 关联起来。

因此 Role 不直接包含人员或 tenant，Permission 也不带 allow/deny。allow/deny 属于 binding，才能表达同一角色在 tenant 范围允许、对某个 exact object 显式限制。v1 不支持 principal-permission 直绑、角色继承或 session role activation。

**追问与权衡**

为什么不是把 tenant 写进 Permission？那会让 Permission 数量随租户和对象爆炸，并把静态 capability 与动态数据范围混在一起。Role + Scope 让能力上限和作用范围分别治理，但增加了策略分析复杂度，因此必须确定性排序和保留匹配证据。

**常见误区**

- 说 Permission 自带 allow/deny；
- 把 RoleBinding 当用户表中的一个 role 字段；
- 用 Role 名称同时表达岗位、租户、对象和审批状态。

**项目证据：** [Permission 三元组](../../../internal/governance/domain/permission.go)、[RoleBinding 与 BindingEffect](../../../internal/governance/domain/role_binding.go)、[Scope union](../../../internal/governance/domain/scope.go)。

**选型边界：** 当策略需要环境条件、团队关系或多级委托时，role + scope 可能不足；应基于真实条件比较 ABAC/ReBAC，而不是把任意字段继续塞进 Role 名。

**来源：** `项目事实`：[ADR-0027“RBAC 与 Scope 的关系”](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[NIST RBAC FAQ](https://csrc.nist.gov/Projects/role-based-access-control/faqs) 与 [NIST ABAC](https://csrc.nist.gov/Projects/Attribute-Based-Access-Control)。

### Q9：为什么只能称“RBAC-inspired 访问控制模型”，不能称完整 NIST RBAC？

**面试官意图**

判断候选人是否尊重标准边界，避免把使用了 Role 词汇就包装成完整标准实现。

**候选人回答**

NIST RBAC 的核心概念包含 user-role assignment、permission-role assignment、session 中角色激活，并可扩展角色层级与职责分离。GrowthOS v1 使用了 Role/Permission/Binding 语言，但没有 Identity directory、session role activation、role hierarchy 或 SoD runtime。

此外 GrowthOS 增加了自己的 `system/tenant/owned/resource` scope 和 matching deny precedence。这是项目的应用策略，不应冒充 Core、Hierarchical 或 Constrained RBAC 的必然语义。准确说法是“统一、默认拒绝的访问控制策略模型”或“RBAC-inspired role + scope model”。

**追问与权衡**

为什么不为了“标准完整”补齐全部功能？没有真实 session、组织层级和职责冲突需求时，提前实现会扩大策略面和误配置风险。先交付最小可执行模型，后续由真实需求驱动 hierarchy、SoD 或 ABAC。

**常见误区**

- 只要有五个角色就称完整 RBAC；
- 把 deny precedence 归因于 NIST Core RBAC；
- 为了简历名词一次性加入未使用的层级和动态策略。

**项目证据：** [ADR-0027 的模型边界](../../decisions/ADR-0027-governance-access-control-model.md)、[当前 Role 实现](../../../internal/governance/domain/role.go)。

**选型边界：** 出现 session role activation、角色层级或职责冲突的真实需求后，必须新增模型和证据；在此之前仍应称 RBAC-inspired，而不是预写完整标准能力。

**来源：** `项目事实`：[产品基线 Permission/Role 说明](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[NIST RBAC FAQ](https://csrc.nist.gov/Projects/role-based-access-control/faqs) 与 [NIST RBAC Models PDF](https://csrc.nist.gov/csrc/media/projects/role-based-access-control/documents/sandhu96.pdf)。

### Q10：五个角色模板是什么？“模板上限”为什么不是实际授权？

**面试官意图**

考察候选人是否掌握当前精确数字，以及能否区分 capability ceiling、Policy subset 和 assignment。

**候选人回答**

当前角色及满模板 Permission 数为：

- `platform_administrator`：16；
- `marketing_operator`：10；
- `lottery_designer`：8；
- `security_auditor`：9；
- `growth_member`：0。

`BaselineRoles()` 返回五个完整模板；`NewRole` 只允许同名模板能力的子集，不能给 Marketing Operator 注入 policy change。Policy revision 可以进一步删减能力。真正授权还要求该 Role 存在于 exact Policy，并有匹配 Principal、Scope、Effect 的 RoleBinding。本节没有 assignment repository，所以没有任何真实人员因模板存在而自动获权。

**追问与权衡**

为什么 Growth Member 是空模板？当前受保护目录都是运营资源，尚无个人用户资源。保留封闭空角色可明确默认拒绝，并防止把会员 tier 当后台权限；未来出现真实个人 Resource 时再经过模型变更加入能力。

**常见误区**

- 看到 `platform_administrator` 就认为存在 wildcard；
- 把模板 permission 数量当已授权用户数；
- 给 `growth_member` 偷加 Activity read，只为让现有页面“看起来可用”。

**项目证据：** [五个模板与精确能力表](../../../internal/governance/domain/role.go)、[16/10/8/9/0 计数测试](../../../internal/governance/domain/permission_role_test.go)。

**选型边界：** 角色职责或受保护资源发生真实变化时，应显式评审模板并迁移 Policy；不能把模板上限动态当作人员 assignment。

**来源：** `项目事实`：[产品基线“内置角色模板”](../../product/access-control-model-threat-boundary-v1.md)；`面经启发`：[小米Java面经](https://www.nowcoder.com/discuss/353147585177264128) 仅启发 RBAC 模型追问，不采用帖子答案作技术依据。

### Q11：`system` scope 是不是 superuser bypass？

**面试官意图**

检查候选人是否会把全局数据范围误解成跳过 capability 检查。

**候选人回答**

不是。`NewSystemScope()` 表示 binding 的数据范围可匹配所有合法目标，但求值仍要求 Role 中存在与请求 `ResourceKind + ResourceType + Action` 完全一致的 Permission。即使 Platform Administrator 使用 system scope，也只能执行模板明确列出的 16 个 capability，不存在 `*:*`。

system 还是一个显式、高风险的 scope 值；scope 缺失、tenant 缺失或构造失败都不会回退为 system。它适合经过审查的平台管理或受控服务绑定，而不是默认值。

**追问与权衡**

怎样降低 system scope 的风险？未来策略写入端应限制谁能创建这类 binding、要求审批或职责分离、记录 exact policy revision，并通过审计观察使用情况。本节只定义匹配语义，没有实现这些治理流程。

**常见误区**

- 把空 tenant 解释为 system；
- 认为管理员 Role 不再需要精确 Action；
- 为内部 service 自动创建 system allow。

**项目证据：** [NewSystemScope 与匹配实现](../../../internal/governance/domain/scope.go)、[system scope 求值测试](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** 当 system binding 可以由运行时策略后台创建时，必须增加写入授权、审批/SoD 和审计；当前纯构造器不承担管理面治理。

**来源：** `项目事实`：[ADR-0027 的 Scope 安全不变量](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)。

### Q12：`tenant` scope 如何匹配？它为什么还不等于多租户隔离？

**面试官意图**

考察 tenant 作用域的精确语义与实现声明边界。

**候选人回答**

`NewTenantScope(tenantID)` 要求 Resource 携带 tenant fact，且与 binding 中 TenantID 完全相等。它可以约束 collection 或 object，但仍要先有精确 Permission。Resource 没有 tenant、tenant 不同或值不规范时都不匹配。

这只是一段可执行的授权匹配语义。当前没有可信 tenant lifecycle、membership、tenant-scoped repository、行级安全或运行时事实 adapter，因此不能据此宣称完成多租户隔离。

**追问与权衡**

tenant ID 应从哪里来？第 33 节可信 service layer 应基于已认证 Principal 和服务端权威数据解析 membership，并从对象仓储加载对象 tenant；客户端提供的 tenant 只能作为查找候选，不能直接成为授权事实。

**常见误区**

- 只比较请求 header 中的 `X-Tenant-ID`；
- 把“有 TenantID 类型”写成多租户已上线；
- tenant 不存在时查询全量数据再由前端过滤。

**项目证据：** [tenant scope 实现](../../../internal/governance/domain/scope.go)、[tenant 匹配/错租户测试](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** L33 接入真实 tenant membership 和 tenant-scoped repository 前，本答案只能说明匹配语义，不能说明数据隔离链已闭环。

**来源：** `项目事实`：[产品基线 tenant 停止线](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[OWASP Multi-Tenant Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Multi_Tenant_Security_Cheat_Sheet.html)；`面经启发`：[腾讯S3后台开发个人面经](https://www.nowcoder.com/discuss/846448031389003776) 仅启发多租户追问。

### Q13：`owned` scope 为什么必须同时匹配 tenant 和 owner？

**面试官意图**

判断候选人是否看见同一用户跨租户、owner fact 缺失和对象级授权的组合风险。

**候选人回答**

`NewOwnedScope(tenantID)` 只匹配 object Resource，要求 Resource tenant 与 binding tenant 完全一致，并且 Resource owner 与当前完整 Principal（kind + ID）完全相等。tenant 或 owner 任一缺失都不匹配，也不存在跨 tenant owned。

同时校验 tenant 是为了防止同一个 PrincipalID 在多个业务空间中被错误复用，或对象被移动/恢复后只靠 owner 继续放行。绑定范围表达的是“这个主体在这个 tenant 内拥有的对象”，不是全局 `owner_id == caller_id`。

**追问与权衡**

如果同一个人属于两个 tenant 怎么办？分别创建两个 owned binding，并由可信 membership/资源事实支撑。若未来 owner 可能是 team 或 relation，需要新模型或 ReBAC 证据，不能把 team ID 塞进 PrincipalID 冒充。

**常见误区**

- owned 只比较 owner，不比较 tenant；
- collection 使用 owned scope；
- owner 由请求体提交，服务端不重新加载。

**项目证据：** [owned scope exact matcher](../../../internal/governance/domain/scope.go)、[owned/tenant/owner 负向测试](../../../internal/governance/domain/principal_resource_scope_test.go)。

**选型边界：** 当 owner 从单一 Principal 扩展为团队、组织或关系集合时，当前等值比较不再足够，应评估显式关系模型而非放宽 owned。

**来源：** `项目事实`：[ADR-0027 owned 不变量](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP API1:2023 BOLA](https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/)。

### Q14：`resource` scope 的“exact”体现在哪里？

**面试官意图**

确认候选人能准确描述精确对象授权，而不是泛泛说“按资源控制”。

**候选人回答**

`NewResourceScope(resourceType, resourceID, tenantID)` 隐含只针对 object。匹配时 Resource 必须也是 object，type 和 ID 完全一致，tenant 的存在性和值也必须完全相同。绑定没有 tenant 时，只能匹配同样没有 tenant fact 的系统对象，不能把空值当 wildcard。

Scope 本身不存 Action；Action 仍由 Role Permission 精确限制。因此 exact Activity binding 也不会自动获得 publish、rollback、retire 全部动作。

**追问与权衡**

为什么构造器没有 ResourceKind 参数？因为 `resource` scope 语义封闭为 exact object，collection 明确不匹配。少一个可变参数减少非法组合；如果未来需要 exact collection partition，应新增独立 scope 语义，而不是放宽当前定义。

**常见误区**

- 只比较 ResourceID，不比较 type/tenant；
- 让空 tenant 同时匹配任意租户；
- 把 exact-resource 当成某个 URL 字符串。

**项目证据：** [resource scope 构造和匹配](../../../internal/governance/domain/scope.go)、[Resource object 事实模型](../../../internal/governance/domain/resource.go)。

**选型边界：** 若未来需要 collection partition、字段级或 relation-based grant，应新增独立范围语义；`resource` 仍保持 exact object，不扩成字符串 pattern。

**来源：** `项目事实`：[产品基线 Scope 表](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)。

### Q15：tenant、owner 等事实缺失时，为什么必须不匹配而不是“尽量放行”？

**面试官意图**

检查 fail-closed 思维和数据缺失、系统可用性之间的权衡。

**候选人回答**

Scope 的安全含义依赖这些事实。缺失 tenant 时无法证明同租户，缺失 owner 时无法证明本人所有，因此 tenant/owned scope 必须不匹配，最终形成默认拒绝或由其他独立 binding 决定。任何空值回退为 system 都会把数据质量故障转成提权。

这不表示所有缺失都应该伪装成正常无权。若权威仓储读取失败或返回损坏对象，第 33 节应形成技术错误并 fail closed；若对象合法地没有 tenant/owner，纯模型才按该 Resource 事实求值。

**追问与权衡**

如何避免可用性过差？用明确的数据不变量、仓储恢复校验、健康告警和受控修复提高事实可靠性，而不是在授权处放宽。对确实属于系统级的对象，应显式建模无 tenant Resource 和对应 scope。

**常见误区**

- `nil tenant` 等于公共资源；
- 依赖故障统一返回业务 deny，导致无法告警；
- 为减少 403 把 unknown 当 allow。

**项目证据：** [Resource 可选事实校验](../../../internal/governance/domain/resource.go)、[Scope 缺失事实匹配](../../../internal/governance/domain/scope.go)、[scope mismatch tests](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** 合法的系统对象可以显式没有 tenant；除此之外，权威事实读取失败应在 L33 作为技术错误处理，不能由 domain 猜测缺失原因。

**来源：** `项目事实`：[ADR-0027“scope 缺失不得回退 system”](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP ASVS V8 Authorization](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md)。

### Q16：BOLA 和 BFLA 分别怎样映射到这套模型？

**面试官意图**

考察候选人能否把 OWASP 风险落到具体授权维度。

**候选人回答**

BOLA 是对象级授权破坏：攻击者把 ActivityID、GraphID 或 StrategyID 换成另一个合法 ID，而服务端只检查登录或对象存在。模型通过 object Resource、server-derived tenant/owner 和 tenant/owned/resource scope 表达防线。

BFLA 是功能级授权破坏：普通用户直接调用 publish、rollback、policy change 等高权限功能。模型通过精确 Action 和 Role Permission 表达防线。真实服务端必须同时执行 capability 与 scope 检查，任何一层缺失都可能越权。

**追问与权衡**

不可猜测 UUID 能防 BOLA 吗？不能。随机 ID 只能降低枚举概率，泄露、日志、引用或侧信道仍可能暴露 ID；OWASP API Security 明确要求所有接收对象 ID 的端点做对象级授权。

**常见误区**

- “ID 很长，所以安全”；
- 只做角色检查，不检查具体对象；
- 只检查对象 owner，不限制 publish/change 等功能。

**项目证据：** [capability + scope 求值实现](../../../internal/governance/domain/evaluation.go)、[跨 tenant 和对象测试](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** 当前代码只证明纯模型能表达 BOLA/BFLA 防线；路由遗漏、事实加载和响应披露必须由 L33/L35 的真实链路验收。

**来源：** `项目事实`：[产品基线威胁清单](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[OWASP BOLA](https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/) 与 [OWASP BFLA](https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/)；`面经启发`：[安全服务实习个人面经](https://www.nowcoder.com/discuss/353157956873166848) 仅启发水平越权题型。

### Q17：当前 16 个合法 capability 是怎样组成的？

**面试官意图**

验证候选人是否真正读过代码目录，而不是只背五个角色名。

**候选人回答**

`ValidateCapability` 注册的精确组合是：

- `marketing.activity`：collection create/read；object read/publish/rollback/retire，共 6 个；
- `lottery.strategy`：collection create/read；object read，共 3 个；
- `lottery.routing_graph`：collection create/read；object read，共 3 个；
- `governance.policy`：collection read；object read/change，共 3 个；
- `governance.audit`：collection read，共 1 个。

合计 16。单个 ResourceType 或 Action 分别合法，不代表组合合法，例如 Activity object create、Strategy object publish、Audit object read 和 Policy collection change 都会返回 `ErrCapabilityUnsupported`。

**追问与权衡**

为什么 Audit 只有 collection read？当前只是授权语言中的未来受保护概念，没有审计仓储或详情 API。目录只冻结当前需要评审的最小 capability，不应为了对称性补出未实现动作。

**常见误区**

- 只数 5 个 ResourceType × 6 个 Action；
- 把非法组合当普通 deny；
- 从角色模板反推公开 API 已经存在。

**项目证据：** [16 个合法 capability 的封闭目录](../../../internal/governance/domain/capability.go)、[非法 tuple 分类测试](../../../internal/governance/domain/capability_test.go)。

**选型边界：** 新 Resource/Action 进入真实需求后，16 这个数会显式变化；变更必须同步目录、模板、测试、文档与威胁矩阵，不能保留旧数字。

**来源：** `项目事实`：[产品基线 Resource/Action 表](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[OWASP API5:2023 BFLA](https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/)。

### Q18：这套 tenant/owned/resource scope 与真正多租户安全之间还差什么？

**面试官意图**

检查候选人是否理解授权只是租户隔离的一层，而非完整答案。

**候选人回答**

当前代码能确定性比较 scope，但还缺可信 tenant 身份与 membership、对象事实加载、repository 查询约束、缓存 key 隔离、队列消息 tenant context、存储/备份隔离、日志与指标分区、管理员跨租户治理，以及负向 E2E。

OWASP 多租户安全指南强调客户端 tenant ID 只能作为选择器，必须与服务端验证的身份和授权绑定。AWS SaaS 架构资料也明确区分身份认证/普通授权与 tenant isolation。GrowthOS 当前只建立了未来实现这些控制时共享的语言。

**追问与权衡**

是否必须立刻使用数据库 RLS？不一定。应根据数据库、部署和查询模式比较 application-enforced tenant predicate、schema/database 隔离或 RLS，并用旁路测试证明。无论选哪种，服务端授权和最小基础设施权限仍是纵深防御的一部分。

**常见误区**

- 有 `tenant_id` 列就称多租户安全；
- 只在 ORM 默认 scope 中过滤，不测原生 SQL/管理任务；
- 把 tenant 隔离完全交给前端。

**项目证据：** [TenantID 的明确免责声明](../../../internal/governance/domain/identifier.go)、[产品基线多租户停止线](../../product/access-control-model-threat-boundary-v1.md)。

**选型边界：** 选 application predicate、RLS、schema 或 database 隔离要由部署、查询和旁路风险决定；当前章节没有足够证据接受其中任一生产方案。

**来源：** `项目事实`：[ADR-0027 后果与停止线](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Multi-Tenant Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Multi_Tenant_Security_Cheat_Sheet.html) 与 [AWS SaaS tenant isolation](https://docs.aws.amazon.com/whitepapers/latest/saas-architecture-fundamentals/tenant-isolation.html)。

### Q19：`Policy.Evaluate` 的精确求值顺序是什么？

**面试官意图**

考察候选人能否解释默认拒绝原因怎样确定，以及算法是否依赖输入顺序。

**候选人回答**

求值先重新验证 Policy 和 AuthorizationRequest；然后只看 Principal 完全相同的 bindings，解析其 Role，筛选 kind/type/action 完全匹配的 Permission，再进行 Scope 匹配并收集 `DecisionMatch`。

若同时有 matching deny 和 allow，reason 是 `explicit_deny_overrode_allow`；只有 deny 是 `explicit_deny`；只有 allow 是 `explicit_allow`。没有 match 时依次根据已观察事实分类：没有任何该 Principal binding 是 `no_binding`；有 binding 但没有目标 capability 是 `no_permission`；有 capability 但 scope 均不匹配是 `scope_mismatch`。

**追问与权衡**

为什么先记录 `hasBinding` 和 `hasPermission`？它们让默认拒绝具有可运营的内部原因，但不会改变安全结果。调用方不能根据 reason 放宽访问，reason 也不能直接暴露给浏览器。

**常见误区**

- 找到第一个 allow 就立即返回；
- 把 scope mismatch 分类成 resource not found；
- 认为 slice 顺序决定最终结果。

**项目证据：** [Policy.Evaluate 完整算法](../../../internal/governance/domain/evaluation.go)、[六种 reason 测试](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** 若未来引入 ABAC/ReBAC 或外置 PDP，内部算法可以替换，但 consumer contract 仍须保持 confirmed decision、技术错误和低披露的清晰分离。

**来源：** `项目事实`：[产品基线“授权请求与判定算法”](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[OASIS XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html) 仅用于比较结果分类，不表示 GrowthOS 采用 XACML。

### Q20：为什么必须默认拒绝？新资源或新动作如何保持 fail closed？

**面试官意图**

检查最小权限和安全演进意识。

**候选人回答**

授权的证明责任在 grant 一侧：只有找到完整、精确、匹配 scope 的 allow，并且没有 matching deny，才能允许。没有 binding、没有 permission、scope 不匹配都形成确认的 deny。OWASP Authorization Cheat Sheet 和 ASVS 都要求默认拒绝并在可信服务端验证每次访问。

新 ResourceType、Action 或组合不在封闭目录时，构造 Policy/Request 就失败并返回 zero Decision + error；目录新增后，既有角色模板也不会自动获得它。必须显式更新模板和策略，避免新功能因默认通配而开放。

**追问与权衡**

默认拒绝会增加发布协调成本，但这正是安全价值：开发者必须回答谁能执行新动作、在哪个 scope、需要何种审计。可通过编译期枚举、架构测试和部署前策略检查降低遗漏，而不是改成 default allow。

**常见误区**

- 未匹配规则时沿用上一次 Decision；
- 新 action 自动继承同 HTTP method 的权限；
- 为“兼容旧客户端”在错误时放行。

**项目证据：** [default deny 分支](../../../internal/governance/domain/evaluation.go)、[无 binding/permission/scope 测试](../../../internal/governance/domain/evaluation_test.go)、[封闭词典架构门禁](../../../internal/governance/domain/architecture_test.go)。

**选型边界：** default deny 不会替代发布前的策略完整性检查；当新动作上线时仍需显式角色/策略迁移和合法调用回归。

**来源：** `项目事实`：[ADR-0027 安全不变量](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html) 与 [OWASP ASVS V8](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md)。

### Q21：为什么选择 matching deny 覆盖 allow？这是 RBAC 标准规定的吗？

**面试官意图**

判断候选人能否解释冲突组合规则及其来源边界。

**候选人回答**

GrowthOS v1 规定：任一 matching deny 存在就覆盖所有 matching allow。典型用途是 tenant allow 加 exact resource deny，用一个窄限制覆盖宽 grant；Decision 同时保留两类证据，并用 `explicit_deny_overrode_allow` 区分纯 deny。

这不是 NIST Core RBAC 必然规定，而是本项目选择的确定性组合策略。AWS IAM 也有显式 deny 覆盖 allow 的成熟语义，但 GrowthOS 没有复刻 IAM 的多种 policy 类型、NotAction 或完整求值逻辑。

**追问与权衡**

代码是否验证 deny 一定比 allow 更窄？没有。算法只判断两者是否都精确匹配当前请求，不能证明 scope 偏序。因而“窄 deny”是推荐配置方式，不是已实现约束；未来策略写入端需要静态分析意外全局 deny。

**常见误区**

- 把 deny precedence 说成所有 RBAC 的标准语义；
- 发现 allow 后短路，永远看不到后续 deny；
- 把 technical error 当显式 deny binding。

**项目证据：** [deny precedence 实现](../../../internal/governance/domain/evaluation.go)、[allow/deny 冲突证据测试](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** 当前 evaluator 不证明 deny 比 allow 更窄；上线动态策略管理前，应增加冲突分析和高风险全局 deny 审查。

**来源：** `项目事实`：[ADR-0027“判定组合规则”](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[AWS IAM deny/allow evaluation](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_evaluation-logic_policy-eval-denyallow.html) 只作厂商组合语义实例。

### Q22：确认的 deny 和技术不可判定为什么必须分开？

**面试官意图**

检查候选人是否能设计诚实的失败协议，而不是把一切压成 false。

**候选人回答**

确认 deny 表示 Policy 和 Request 都合法，求值成功，只是结果不允许；`Decision.Confirmed()` 为 true，`Allowed()` 为 false，并有六种封闭 reason 之一。技术不可判定表示策略、请求、时间或内部证据损坏，没有形成可信决定，返回 error 和严格 zero `Decision{}`。

未来 enforcement 对两者都必须 fail closed，但处理不同：deny 进入授权拒绝指标和受控审计；技术失败进入故障指标、告警与恢复流程。它们也不能被映射成 Participation ineligible、Activity closed 或 Lottery no-reward。

**追问与权衡**

为什么技术错误不也生成 reason=indeterminate 的 Decision？当前 `DecisionOutcome` 只有 allow/deny，有效 Decision 必须是确认结果。引入第三种 outcome 会让调用方容易把 unknown 当业务状态；zero result + error 更强制地要求处理失败。

**常见误区**

- `error == deny Decision`；
- 为减少告警把损坏 Policy 归为 `no_permission`；
- 收到 deny 就自动重试。

**项目证据：** [DecisionOutcome/Reason 与 Validate](../../../internal/governance/domain/evaluation.go)、[损坏输入 zero-result 测试](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** 若未来 consumer contract 引入显式 indeterminate，必须重新设计所有调用方和低披露映射；当前代码没有第三种 confirmed outcome。

**来源：** `项目事实`：[产品基线封闭结果表](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[OASIS XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html) 仅校准外部四值模型的差异。

### Q23：为什么 error 时必须返回严格 zero `Decision{}`？

**面试官意图**

考察部分结果、错误处理和安全调用契约。

**候选人回答**

若错误同时返回带 outcome、reason 或 matches 的半成品，调用方可能忽略 error、读取旧字段或把部分 allow evidence 当有效授权。严格 zero 让任何 `Outcome/Reason/Matches` 都没有可信含义；只有 error 为 nil 且 Decision 自身 Validate/Confirmed 通过，才能消费。

`Policy.Evaluate` 会重新 Validate Policy 和 AuthorizationRequest，构造最终 Decision 后再 Validate；任一阶段失败都返回 `Decision{}`。这也防止同包测试或未来反序列化制造的 partial struct 绕过构造器。

**追问与权衡**

调用方是否只检查 `Allowed()` 就够？`Allowed()` 对非法 Decision 返回 false，安全上会 fail closed，但仍应先处理 error，以便区分正常拒绝和系统故障；否则可观测性与恢复会失真。

**常见误区**

- 忽略 error，只看 `Outcome()`；
- error 时返回“尽可能多”的 matches；
- 复用上一次非零 Decision 作为 fallback。

**项目证据：** [Evaluate 的全部 zero-return 分支](../../../internal/governance/domain/evaluation.go)、[zero/partial Decision 负向测试](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** 上层可以把 error 映射成受控服务故障，但不得改变“无可信 Decision”这一契约；需要恢复时应修复/重载事实，而不是消费半成品。

**来源：** `项目事实`：[ADR-0027“决定与错误分离”](../../decisions/ADR-0027-governance-access-control-model.md)。

### Q24：`Decision` 和 `DecisionMatch` 保存哪些证据？为什么不能直接返回给浏览器？

**面试官意图**

判断候选人是否理解可解释性、最小证据和敏感策略披露之间的平衡。

**候选人回答**

有效 Decision 固定携带 exact `PolicyIdentity`、Principal、Resource、Action、完整 AuditContext，以及规范排序的 matches。每个 `DecisionMatch` 包含 `BindingID`、`RoleID`、`BindingEffect`、`ScopeKind` 和 exact `Permission`。

allow 只会有 matching allow；纯 deny 只有 matching deny；冲突 deny 同时保留 allow 和 deny；默认拒绝没有伪造 match。这些信息足以内部解释“哪条 binding 和 capability 决定了结果”，但也会暴露角色、策略形状、对象与租户信息，不能直接序列化给客户端。

**追问与权衡**

审计是否应该保存完整 Policy？通常不应。保存 exact identity、决定、最小 match reference 和受控上下文即可；需要调查时由有权限的审计系统关联 Policy snapshot。全量策略、credential 和业务 payload 会扩大二次泄露面。

**常见误区**

- 证据只列 BindingID，漏掉实际 Permission；
- 默认拒绝伪造一条 deny match；
- 把内部 reason 和完整 RoleBinding 放进 403 JSON。

**项目证据：** [DecisionMatch 五项证据与 getter](../../../internal/governance/domain/evaluation.go)、[证据防御复制/冲突测试](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** 接入持久 audit sink 后，只应保存调查所需白名单；若合规要求增加字段，需重新做敏感数据与保留期评审，不能默认落完整 Policy。

**来源：** `项目事实`：[ADR-0027“判定证据”](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)。

### Q25：低披露为什么也是授权设计的一部分？403 和 404 应怎样选择？

**面试官意图**

检查资源枚举、错误侧信道和客户端可用性的权衡能力。

**候选人回答**

即使服务端正确拒绝，响应状态、字段、计数、耗时或错误文案仍可能泄露对象是否存在、属于哪个租户或策略结构。Domain 因此只保留内部 reason，不规定 HTTP 映射；第 33 节再按资源披露策略映射为最少客户端信息。

RFC 9110 允许服务端在希望隐藏被禁止资源时使用 404 代替 403，但这是 MAY，不是所有 deny 必须 404。对公开资源、管理资源和可恢复操作，枚举风险与客户端行为不同；需要逐资源决定，并让内部审计仍能区分 unauthenticated、deny 和 failure。

**追问与权衡**

只统一返回 404 是否足够？不够。列表数量、搜索建议、字段级投影、导出、响应时间和缓存命中差异都可能泄露；第 35 节要用 direct API 与浏览器负向测试验证完整披露面。

**常见误区**

- 认为 403 一定比 404 “更标准”；
- 把内部 `no_binding` 文案直接返回；
- 只隐藏详情页，列表仍返回全量对象。

**项目证据：** [产品基线“低披露原则”](../../product/access-control-model-threat-boundary-v1.md)、[ADR-0027 暂不统一 404 的决定](../../decisions/ADR-0027-governance-access-control-model.md)。

**选型边界：** 403/404、列表、字段和耗时策略必须由 L33 按资源定义并由 L35 验收；domain reason 不承担传输层披露政策。

**来源：** `项目事实`：[DecisionReason 内部属性注释](../../../internal/governance/domain/evaluation.go)；`官方事实`：[RFC 9110 `15.5.4](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.4) 与 [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)。

### Q26：怎样保证求值不依赖 roles、permissions、bindings 的输入顺序？

**面试官意图**

考察确定性、规范化和策略变更可审查性。

**候选人回答**

构造器防御性复制输入后进行规范排序：Policy roles 按 RoleID，Role permissions 按 ResourceType、ResourceKind、Action，RoleBindings 按 BindingID；Decision matches 先按 BindingID，再按其余证据字段。Validate 会拒绝同包伪造的非规范内部状态。

求值遍历全部匹配项后按计数决定 outcome/reason，不采用“第一条命中获胜”。因此对同一语义集合做排列，结果与证据顺序稳定，适合测试、审计和比较。

**追问与权衡**

规范排序是否意味着可以把序列化内容直接 hash 成 revision？不能。当前没有冻结 canonical wire encoding，`PolicyRevision` 也明确不是 content hash。排序只保证内存模型的确定性，不自动提供签名或防篡改。

**常见误区**

- 使用 map 迭代顺序产生证据；
- allow/deny 采用 first-match；
- 看到 canonical 就宣称具备密码学完整性。

**项目证据：** [Role/Permission 规范排序](../../../internal/governance/domain/role.go)、[Policy 排序](../../../internal/governance/domain/policy.go)、[DecisionMatch 排序](../../../internal/governance/domain/evaluation.go)。

**选型边界：** 若未来需要签名、内容寻址或跨语言一致 hash，必须先冻结 canonical serialization；当前排序只保证 Go 模型内确定性。

**来源：** `项目事实`：[排列不变与 canonical tests](../../../internal/governance/domain/evaluation_test.go)、[ADR-0027 revision 边界](../../decisions/ADR-0027-governance-access-control-model.md)。

### Q27：Policy、Role 和 Decision 如何做到不可变？

**面试官意图**

检查 Go 中“没有 const object”时怎样保护快照语义。

**候选人回答**

核心 struct 字段不导出，只能通过构造器建立；构造器复制并排序输入 slice；`Role.Permissions()`、`Policy.Roles()`、`Policy.RoleBindings()` 和 `Decision.Matches()` 都返回防御性副本。`BaselineRoles()` 每次也返回新快照，调用方修改结果不会污染后续调用。

不可变使 exact Policy revision 在求值期间保持稳定，也让并发只读无需内部锁。它不证明外部 repository 永远不变；未来 repository 应按 `PolicyID + Revision` 保存不可变 snapshot。

**追问与权衡**

为什么不暴露指针并要求调用者自律？权限对象的误修改会造成非确定提权，安全边界不应依赖口头约定。复制增加少量内存成本，但集合已有 64/1024 的硬上限，当前更重视可审查性。

**常见误区**

- getter 直接返回内部 slice；
- 只复制顶层 roles，内部 permissions 仍共享；
- 把 Go value receiver 自动等同深度不可变。

**项目证据：** [Role defensive copy](../../../internal/governance/domain/role.go)、[Policy deep copy](../../../internal/governance/domain/policy.go)、[Policy/Role 防御复制测试](../../../internal/governance/domain/policy_test.go)。

**选型边界：** Policy 热更新应替换 exact snapshot，而非原地 mutation；若集合规模显著增长，需要用真实 profile 重新评估复制成本。

**来源：** `项目事实`：[ADR-0027“纯领域模型”](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[Go Memory Model](https://go.dev/ref/mem)。

### Q28：`PolicyRevision` 解决什么？它为什么不是撤权、缓存或 TOCTOU 的完整答案？

**面试官意图**

考察版本证据与运行时一致性的边界。

**候选人回答**

Decision 携带 exact `PolicyID + PolicyRevision`，让审计能说明“按哪一份策略作出决定”。revision 是非零 `uint64` 派生值，只作 snapshot correlation；纯构造器不能保证同一 ID/revision 全局唯一，未来 repository 必须强制。

它不能自动解决缓存过旧和授权后对象变化。若策略缓存滞后，撤权仍可能延迟；若授权后再执行写操作，Resource 或 Policy 可能发生 TOCTOU。第 33 节需要根据风险把事实加载、授权和业务写入放进合适的一致性/事务边界，或在写入时重验关键 version。

**追问与权衡**

为什么不现在设计 Policy cache？当前没有 repository、调用流量、撤权时限或跨服务证据。提前加缓存会引入失效、版本唯一性和故障语义；先保留 exact identity，再由真实数据决定策略。

**常见误区**

- revision 等于内容 hash；
- Decision 形成后永远有效，可作为 bearer token；
- 加一个 TTL 就解决撤权一致性。

**项目证据：** [PolicyIdentity/PolicyRevision 定义](../../../internal/governance/domain/policy.go)、[revision 校验测试](../../../internal/governance/domain/policy_test.go)。

**选型边界：** 只有出现真实 repository/cache 和撤权时限后，才能选择一致性、失效与重验策略；当前 revision 只是 correlation evidence。

**来源：** `项目事实`：[产品基线“不可变 Policy snapshot”](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[Google Zanzibar](https://research.google/pubs/zanzibar-googles-consistent-global-authorization-system/) 仅用于说明大规模授权的一致性问题，不移植其性能结论。

### Q29：`AuditContext` 为什么不是 HTTP RequestID、trace 或持久审计日志？

**面试官意图**

判断候选人是否会把 correlation metadata 夸大成完整审计系统。

**候选人回答**

代码只有统一类型 `AuditReference`，`AuditContext` 包含 `EvaluationReference()`、`CorrelationReference()` 和 `EvaluatedAt()`。构造器把时间规范为 UTC 微秒，并验证两个引用的有界格式。

EvaluationReference 标识本次纯求值，CorrelationReference 供未来上层关联一次操作；它们不是 token、session、HTTP request ID 或 OpenTelemetry trace。Decision 也没有持久化。第 33 节可信 service layer 才能从 request/operation context 建立映射，并把白名单字段写入受保护 audit sink。

**追问与权衡**

为什么 domain 仍需要 evaluated-at？它让 Decision 证据具有单一规范时刻，并为未来关联提供稳定值；但当前判定算法没有时间条件，不能把这个字段称为基于环境属性的 ABAC。

**常见误区**

- 把不存在的 `EvaluationRef`、`CorrelationRef` 写成 Go 类型；
- 把 correlation ID 当认证证据；
- 声称本节已经实现安全审计仓储。

**项目证据：** [AuditReference 类型](../../../internal/governance/domain/identifier.go)、[AuditContext 三个字段与规范时间](../../../internal/governance/domain/audit_context.go)、[AuditContext tests](../../../internal/governance/domain/audit_context_test.go)。

**选型边界：** L33 接入 request/trace/audit sink 时应通过 adapter 显式映射；如果需要 durable event identity，必须新增独立类型而不是重用 correlation ref。

**来源：** `项目事实`：[ADR-0027 AuditContext 边界](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)。

### Q30：为什么所有 opaque ID/reference 都使用 128 bytes 小写 ASCII grammar？

**面试官意图**

检查输入规范化、别名和资源耗尽意识。

**候选人回答**

`PrincipalID`、`TenantID`、`ResourceID`、`PolicyID`、`RoleBindingID` 和 `AuditReference` 共用 `MaxOpaqueIdentifierBytes = 128`。值必须以小写字母或数字开头、结尾，内部才允许点、下划线、冒号和连字符；构造器不 trim、不自动 lowercase，也不做 Unicode normalization。

这样避免 `Admin`、`admin`、前后空格或 Unicode 近似字符成为不同边界中的别名，并限制复制到策略和证据中的内存。外部身份 adapter 应把原生 ID 显式映射为 canonical internal ID，而不是把 email、JWT 或任意路径直接塞入。

**追问与权衡**

为什么不强制 UUID？授权模型只需要不透明、稳定且规范的内部 key；不同身份提供方可能使用其他 ID。允许有限 ASCII 语法保持可移植，唯一性仍由未来身份/策略 repository 负责。

**常见误区**

- 构造器自动修剪非法输入；
- 128 是字符数而不是 bytes；
- 把 RoleID、Action 等封闭枚举也当任意 128-byte ID。

**项目证据：** [128-byte canonical grammar](../../../internal/governance/domain/identifier.go)、[边界/非法字符测试](../../../internal/governance/domain/identifier_test.go)。

**选型边界：** 外部 IdP ID 不满足内部 grammar 时必须显式、稳定映射；若未来必须保留 Unicode 展示名，应作为非授权显示字段单独建模。

**来源：** `项目事实`：[产品基线“规范值、容量与确定性边界”](../../product/access-control-model-threat-boundary-v1.md)、[ADR-0027 标量决定](../../decisions/ADR-0027-governance-access-control-model.md)。

### Q31：64、64、1024 这些容量数字应该怎样解释？

**面试官意图**

检查候选人能否区分安全预算、当前有效基数、性能指标和生产容量。

**候选人回答**

代码 guard 是每个 Policy 最多 64 roles、每个 Role 最多 64 permissions、每个 Policy 最多 1024 RoleBindings，`MaxDecisionMatches` 也等于 1024。由于一个合法 Role 不含重复 Permission，同一 binding 对一个 exact request 至多贡献一个 match。

但 v1 RoleID 只有 5 个且 Policy 要求唯一，所以当前有效 Policy 最多 5 个 Role；五个模板能力上限是 16/10/8/9/0，单 Role 实际也不可能达到 64 个合法 Permission。64 是面向模型演进的先验资源 guard，1024 是同步扫描和证据大小上界，都不是吞吐、延迟或生产 SLO。

**追问与权衡**

为什么保留看似不可达的 64？它让未来增加受评审角色/能力时仍有统一硬上限，也能在校验早期拒绝恶意大 slice。若真实策略规模接近上限，应基于 profile 和业务分布比较索引或外置 PDP，不能用上限倒推系统性能。

**常见误区**

- 宣称当前支持 64 种角色；
- 把 1024 matches 写成 QPS；
- 只限制 bindings，不限制证据复制和角色能力。

**项目证据：** [MaxRoles/MaxPermissions 与模板](../../../internal/governance/domain/role.go)、[MaxBindings](../../../internal/governance/domain/policy.go)、[MaxDecisionMatches](../../../internal/governance/domain/evaluation.go)。

**选型边界：** 策略规模逼近 guard 或出现延迟证据时，才可基于 profile 比较索引/缓存/PDP；这些常量本身不能支持容量结论。

**来源：** `项目事实`：[容量与模板计数测试](../../../internal/governance/domain/permission_role_test.go)、[ADR-0027 容量决定](../../decisions/ADR-0027-governance-access-control-model.md)。

### Q32：不可变 Policy 为什么适合并发只读？race 通过又不能证明什么？

**面试官意图**

考察 Go 并发安全、共享状态和测试证据边界。

**候选人回答**

Policy、Role、Binding 和 Decision 在构造后没有写方法，slice 已由对象拥有，getter 返回副本；`Evaluate` 只读取 snapshot 并创建局部 matches。因此多个 goroutine 可并发求值同一 Policy，不需要在纯内核里加 mutex。

Race detector 能发现实际测试执行路径上的数据竞争；并发只读测试与 defensive-copy 测试共同反证常见共享 slice 问题。但它不证明未来 Policy repository 的线性一致性、缓存撤权时限、handler 每请求都执行授权，或分布式系统没有 TOCTOU。

**追问与权衡**

如果未来需要热更新 Policy，应该修改原对象吗？不应。加载新 immutable snapshot，通过原子引用或受控 repository 按 revision 切换；正在执行的请求继续引用其 exact snapshot。切换策略和撤权 SLA 需要独立设计与测试。

**常见误区**

- 给 immutable object 每次读取都加锁；
- race 一次通过就宣称线程安全已被数学证明；
- 在 slice getter 返回值中修改后期待更新 Policy。

**项目证据：** [Policy immutable representation](../../../internal/governance/domain/policy.go)、[并发求值与防御复制测试](../../../internal/governance/domain/evaluation_test.go)。

**选型边界：** 引入 mutable repository/cache 后，并发边界转移到 snapshot 发布与失效；纯 domain 的无锁只读结论不能直接外推。

**来源：** `项目事实`：[ADR-0027 输入输出不可变不变量](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[Go Memory Model](https://go.dev/ref/mem) 与 [Go Race Detector](https://go.dev/doc/articles/race_detector)。

### Q33：Policy 构造时具体拒绝哪些重复、悬空和越界状态？

**面试官意图**

检查候选人是否能说明策略写入前的结构完整性。

**候选人回答**

`NewPolicy` 校验 exact PolicyIdentity，复制并排序 roles/bindings，执行容量检查；RoleID 必须唯一，每个 Role 必须是对应模板合法子集且 Permission 不重复；RoleBindingID 必须唯一，binding 引用的 Role 必须存在。

语义重复 binding 由 `Principal + RoleID + Scope + Effect` 判定，即使 BindingID 不同也拒绝。Effect 是语义的一部分，因此同一 Principal/Role/Scope 的 allow 与 deny 可以同时存在并在求值时形成冲突证据；典型配置仍是宽 allow 加窄 deny。

**追问与权衡**

为什么不自动去重？静默丢弃重复配置会掩盖策略生成器或迁移缺陷，也让操作者误以为两条规则都生效。构造失败并返回明确 error 更容易在写入前修复。

**常见误区**

- 只检查 BindingID，不检查语义重复；
- dangling Role 在求值时再猜默认权限；
- 重复 Permission 自动合并后继续运行。

**项目证据：** [Policy duplicate/dangling validation](../../../internal/governance/domain/policy.go)、[重复/悬空/allow+deny tests](../../../internal/governance/domain/policy_test.go)。

**选型边界：** 动态策略编辑器可在提交前提供更友好的冲突诊断，但最终 domain Validate 仍应保持严格；UI 不能成为唯一校验层。

**来源：** `项目事实`：[产品基线“不可变 Policy snapshot”](../../product/access-control-model-threat-boundary-v1.md)、[ADR-0027 安全不变量](../../decisions/ADR-0027-governance-access-control-model.md)。

### Q34：如何测试一个纯授权内核，才能证明的不只是 happy path？

**面试官意图**

考察安全测试的负向、性质和架构维度。

**候选人回答**

单元表驱动应覆盖三种 PrincipalKind、所有 capability 合法/非法组合、四种 scope 的精确命中与缺失事实、五个角色模板、三种默认拒绝、纯 deny、allow/deny 冲突、zero/partial 值、重复/悬空/容量和 defensive copy。

性质测试要验证输入排列不改变 Decision、不同 tenant/owner 永不产生意外 allow、任何 error 都伴随 zero Decision。Fuzz 用任意字符串和策略组合攻击 parser/Validate 边界；race 覆盖共享 snapshot 并发只读。架构测试还要限制 domain import、禁止 untyped attribute bag/`IsAdmin` 快捷方式，并证明外部生产代码尚未提前装配内核。

**追问与权衡**

这些测试能证明线上没有越权吗？不能。它们证明模型内部语义和停止线；直到第 33 节装配后才能测 direct API enforcement，第 35 节才覆盖浏览器、跨对象、跨租户与低披露 E2E。

**常见误区**

- 只测管理员 allow；
- fuzz 只追求执行次数，不定义安全不变量；
- 用 domain coverage 代替端到端越权验收。

**项目证据：** [capability tests](../../../internal/governance/domain/capability_test.go)、[scope tests](../../../internal/governance/domain/principal_resource_scope_test.go)、[evaluation fuzz](../../../internal/governance/domain/evaluation_fuzz_test.go)、[architecture tests](../../../internal/governance/domain/architecture_test.go)。

**选型边界：** 模型测试只覆盖纯求值；L32～L35 每增加一个真实边界，都要新增会话、handler、repository、浏览器和低披露层的测试。

**来源：** `项目事实`：[ADR-0027 验收项](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[Go Fuzzing](https://go.dev/doc/security/fuzz/) 与 [Go Race Detector](https://go.dev/doc/articles/race_detector)。

### Q35：为什么当前不直接使用 OPA、OpenFGA 或 Zanzibar 模型？

**面试官意图**

判断候选人是否能做有证据的 build-vs-buy，而不是追逐技术名词。

**候选人回答**

当前是模块化单体，资源、动作和角色词典小，尚无跨服务策略一致性、关系图查询或独立策略发布需求。纯 Go 内核没有网络失败、远端部署、缓存失效和额外运维面，最适合先验证语义。

OPA 官方示例体现应用向策略引擎请求决策；OpenFGA 用 user/relation/object tuple 表达关系授权；Google Zanzibar 论文展示了统一关系模型和一致性设计。它们解决的复杂度比当前 role + scope 更大。现在引入会提前承担可用性、延迟、策略版本、撤权和调试成本，却没有真实规模收益。

**追问与权衡**

怎样保留未来迁移空间？让业务层依赖稳定的授权请求/决定端口，而不是依赖当前扫描实现；收集策略规模、关系深度、延迟预算、跨服务一致性和运维能力。证据出现后再用 ADR 比较 adapter、双读验证和迁移路径。

**常见误区**

- “外置策略引擎一定更安全”；
- 把 OpenFGA/ReBAC 与普通角色表视为同一模型；
- 引用 Google 规模和延迟当作本项目性能数据。

**项目证据：** [ADR-0027 对 OPA/OpenFGA 的备选方案评估](../../decisions/ADR-0027-governance-access-control-model.md)、[当前纯 domain contract](../../../internal/governance/domain/doc.go)。

**选型边界：** 出现跨服务决策、深关系查询、独立策略发布或当前扫描无法满足的已测需求时，应重新做 ADR 和迁移验证。

**来源：** `项目事实`：[产品基线容量与替代方案边界](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[OPA HTTP Authorization](https://www.openpolicyagent.org/docs/http-api-authorization)、[OpenFGA Concepts](https://openfga.dev/docs/concepts)、[Google Zanzibar](https://research.google/pubs/zanzibar-googles-consistent-global-authorization-system/)。

### Q36：第 31 节的真实信任边界在哪里？架构门禁保护什么？

**面试官意图**

检查候选人是否能画出数据从不可信输入到业务执行的完整边界。

**候选人回答**

目标链路是：

`Browser/API client/Agent → L32 identity boundary → trusted Principal → L33 trusted service layer → server-derived Resource + exact Policy → Policy.Evaluate → business use case`。

第 31 节只实现中间纯求值内核。构造器验证形状，不验证来源；浏览器 role/tenant/owner、会员 tier、Approval、Activity gate 和基础设施 credential 都不能成为授权证明。

架构测试限制 Governance domain 只导入少量标准库，禁止 HTTP、SQL、JWT、业务包、动态字符串 bag、`IsAdmin` 等快捷方式；还扫描外部生产 Go 文件，确保没有在可信 session 和 service layer 之前提前 import/装配内核。

**追问与权衡**

为什么连“先接一个 demo handler”都禁止？未认证请求若能自行构造 Principal/Resource，会制造看似已授权的危险示例，后续容易被复用成生产路径。停止线用短期不可调用换取后续装配时的可信顺序。

**常见误区**

- 构造成功就标记 trusted；
- middleware 只读取请求头 role；
- 架构测试通过就宣称线上 enforcement 已存在。

**项目证据：** [domain import/shortcut 与 runtime stop-line tests](../../../internal/governance/domain/architecture_test.go)、[包级来源免责声明](../../../internal/governance/domain/doc.go)。

**选型边界：** L32/L33 完成后，外部生产代码会按受审查端口消费模型，现有“零外部 import”门禁必须被新的“只能从可信层 import”门禁替代，而不是简单删除。

**来源：** `项目事实`：[产品基线信任边界图](../../product/access-control-model-threat-boundary-v1.md)；`官方事实`：[OWASP Threat Modeling Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Threat_Modeling_Cheat_Sheet.html) 与 [NIST SP 800-207](https://csrc.nist.gov/pubs/sp/800/207/final)。

### Q37：第 32 节真实会话认证必须补什么，才能安全地产生 trusted Principal？

**面试官意图**

确认候选人理解认证章节的唯一职责和安全要素。

**候选人回答**

第 32 节要建立 credential → verified session → trusted Principal：包括凭据校验、会话标识不可预测、Cookie 安全属性、过期与空闲超时、注销/撤销、session fixation 防护、credential 不落日志，以及浏览器场景的 CSRF 边界。

认证成功只证明 caller 身份，不决定其 Role、Scope 或 Resource 权限。输出 Principal 应由服务端身份记录映射，不接受客户端提交 kind/ID。该节仍不能宣称 API 已执行 RBAC，因为 Policy 加载和 `Evaluate` 强制属于第 33 节。

**追问与权衡**

JWT 是否天然优于服务端 session？不是。应按撤销需求、密钥轮换、跨服务、Cookie/CSRF、泄露后窗口和运维能力比较；无论载体是什么，验证后都只生成最小可信 Principal。

**常见误区**

- JWT 解码成功等于授权通过；
- role 永久写入长寿命 token 且不考虑撤权；
- 登录页完成就称权限系统完成。

**项目证据：** [当前 Principal 仅验证形状](../../../internal/governance/domain/principal.go)、[L32 停止线](../../product/access-control-model-threat-boundary-v1.md)。

**选型边界：** credential 载体、会话存储和撤销方案要由 L32 的真实浏览器/部署需求决定；本节只规定认证后输出最小 trusted Principal。

**来源：** `项目事实`：[ADR-0027 后续演进 L32](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 与 [CSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)。

### Q38：第 33 节服务端 RBAC 强制应怎样消费当前模型？

**面试官意图**

考察从纯模型到安全 use case 的装配方式。

**候选人回答**

可信 service layer 从第 32 节取得 Principal，根据具体用例选择 exact Action，从资源 owner 的 repository 加载 ID、tenant、owner 等事实，并读取 exact Policy snapshot和构造 AuditContext。每个受保护请求都调用 `Policy.Evaluate`；只有 confirmed allow 才进入业务写入。

deny 和 error 都 fail closed，但映射不同：未认证、无权、隐藏存在性的 not-found 策略、技术失败和审计事件分别处理。高风险写操作还要设计授权与对象 version/事务的关系，降低授权后对象变化的 TOCTOU。

**追问与权衡**

是否只在通用 middleware 校验一次？middleware 可以处理认证和粗粒度入口，但对象 facts、Action 和事务边界属于具体 use case。只做 URL 级角色检查无法防 BOLA；可以用 decorator/template 减少遗漏，但不能丢失业务资源语义。

**常见误区**

- handler 从请求体取 tenant/owner 后直接 Evaluate；
- 首页 middleware allow 后，子操作全部放行；
- technical error 自动重试并绕过一次授权。

**项目证据：** [AuthorizationRequest/Policy.Evaluate contract](../../../internal/governance/domain/evaluation.go)、[L33 trusted service layer 规划](../../product/access-control-model-threat-boundary-v1.md)。

**选型边界：** service decorator 只能抽取共性；Resource facts、Action 和事务重验仍由具体 use case 定义。尚无真实 repository/handler 时不能预写统一 middleware 已安全。

**来源：** `项目事实`：[ADR-0027 后续演进 L33](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html) 与 [OWASP ASVS V8](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md)。

### Q39：第 34 节前端权限投影怎样改善体验，又不成为安全边界？

**面试官意图**

检查候选人能否设计统一的导航、路由和操作裁剪，同时坚持服务端权威。

**候选人回答**

前端应消费服务端基于同一词典生成的最小 capability projection，用于裁剪导航、路由入口、页面区块、字段和按钮，并提供无权、加载中、会话失效等可理解状态。投影不应下发完整 Policy、RoleBindings、internal reason 或敏感 tenant 范围。

浏览器数据可以被修改，隐藏按钮只能减少误操作和噪声。用户仍可直接输入 URL、修改 store 或调用 API，所以第 33 节服务端必须对每次请求重新强制授权；前后端不允许维护两套独立角色 if/else。

**追问与权衡**

前端应该按 Role 还是 capability 渲染？优先按面向 UI 需求的最小 capability，而不是硬编码角色名。角色可能组合、scope 可能按对象变化；对于对象级按钮，可能需要服务端随资源投影可执行动作，同时控制披露和缓存。

**常见误区**

- `display: none` 等于授权；
- 把全量 Policy JSON 发给浏览器；
- Admin workspace 路径本身就是身份凭证。

**项目证据：** [Decision evidence 的内部披露注释](../../../internal/governance/domain/evaluation.go)、[L34 capability projection 停止线](../../product/access-control-model-threat-boundary-v1.md)。

**选型边界：** 对象级动作会随 Resource/Scope 变化时，静态全局 capability 不足，应采用服务端按资源给出的最小动作投影并设计失效策略。

**来源：** `项目事实`：[ADR-0027“只在前端隐藏菜单”备选方案](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html) 与 [OWASP ASVS V8](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md)；`面经启发`：[趣链 Java 后端个人面经](https://www.nowcoder.com/discuss/723953710098886656) 仅启发“不同用户菜单”追问。

### Q40：第 35 节越权与浏览器 E2E 应覆盖什么，何时才可以宣称权限链完成？

**面试官意图**

考察安全验收是否包含真实攻击路径，而不是只测正常管理员操作。

**候选人回答**

第 35 节至少覆盖 anonymous、失效/撤销 session、跨 Role、跨 object、跨 tenant、owner 不同、直接 URL、直接 API、篡改前端 capability、替换 ID、缺失 tenant/owner、deny/error 低披露，以及导航/路由/按钮与服务端决定一致。敏感资源还要观察 403/404、列表数量、字段和响应差异是否泄露存在性。

验收必须从真实浏览器或 HTTP 客户端穿过会话、service layer、Policy、Resource repository 和业务入口，证明拒绝后没有状态变化；同时回归合法角色的正向路径。只有 L32～L35 全部有证据后，才能说“真实请求链已按权限裁剪并由服务端强制”，仍不能自动宣称生产合规、零漏洞或多租户全面完成。

**追问与权衡**

为何 domain 单测不能替代 E2E？domain 测的是给定可信事实时算法正确；越权往往发生在事实加载、route 漏装、对象 ID 替换、客户端披露或事务边界。E2E 专门验证这些接缝。

**常见误区**

- 只测管理员成功；
- UI 显示正确就不再直接调 API；
- 一轮 E2E 通过就承诺生产安全和永久无越权。

**项目证据：** [L35 威胁与验收停止线](../../product/access-control-model-threat-boundary-v1.md)、[当前仍禁止 runtime 装配的反证](../../../internal/governance/domain/architecture_test.go)。

**选型边界：** L35 只能证明约定环境和攻击矩阵；资源、route、角色或披露策略变化后必须持续做授权回归，不能把一次验收当永久证明。

**来源：** `项目事实`：[ADR-0027 后续演进 L35](../../decisions/ADR-0027-governance-access-control-model.md)；`官方事实`：[OWASP BOLA](https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/)、[OWASP BFLA](https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/) 与 [OWASP ASVS V8](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md)。

## 3. 官方一手资料

以下资料均于 2026-08-31 访问。它们用于校准安全概念，不替代 GrowthOS 当前代码、产品基线或 ADR：

| 官方来源 | 本文采用的范围 |
| --- | --- |
| [NIST RBAC FAQ](https://csrc.nist.gov/Projects/role-based-access-control/faqs) | user-role、permission-role、session activation、层级与职责分离边界 |
| [NIST《Role-Based Access Control Models》](https://csrc.nist.gov/csrc/media/projects/role-based-access-control/documents/sandhu96.pdf) | RBAC 模型族、角色与组的差异；不用于声称 GrowthOS 完整实现标准 |
| [NIST ABAC 项目](https://csrc.nist.gov/Projects/Attribute-Based-Access-Control) | subject/object/action/environment attribute 语言；GrowthOS v1 不实现 ABAC engine |
| [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html) | 最小权限、默认拒绝、每请求校验、对象级授权、服务端可信边界与测试 |
| [OWASP ASVS 5.0 V8 Authorization](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md) | 服务端执行、功能/数据/字段权限、最终主体和跨租户验证 |
| [OWASP API1:2023 BOLA](https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/) | 所有接收对象 ID 的端点仍须执行对象级授权 |
| [OWASP API5:2023 BFLA](https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/) | 管理功能、功能级授权和 deny-by-default |
| [OWASP Multi-Tenant Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Multi_Tenant_Security_Cheat_Sheet.html) | 客户端 tenant ID 只能作选择器，须绑定服务端确认身份与权限 |
| [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) | 认证与敏感操作重新认证的边界；不把认证成功当授权结果 |
| [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) | session ID、Cookie 属性、过期、注销与 fixation 防护 |
| [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html) | 浏览器 Cookie 会话下的 CSRF 防护边界 |
| [OWASP Threat Modeling Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Threat_Modeling_Cheat_Sheet.html) | 数据流、资产、信任边界、威胁与缓解的结构化分析 |
| [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html) | 安全事件日志及敏感数据排除原则 |
| [RFC 9110 `15.5.4](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.4) | 服务端在希望隐藏被禁止资源时可以使用 404；这是 MAY 而非全局强制 |
| [AWS IAM policy evaluation logic](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_evaluation-logic_policy-eval-denyallow.html) | 显式 deny 覆盖 allow 的厂商实例；不冒充 GrowthOS 或 Core RBAC 标准 |
| [AWS SaaS tenant isolation](https://docs.aws.amazon.com/whitepapers/latest/saas-architecture-fundamentals/tenant-isolation.html) | 认证/普通授权与 tenant isolation 的区别；仅作厂商架构案例 |
| [NIST SP 800-207 Zero Trust Architecture](https://csrc.nist.gov/pubs/sp/800/207/final) | 不因网络位置隐式信任、在资源访问前分别认证和授权；不声称本项目实现 ZTA |
| [OASIS XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-os-en.html) | Permit/Deny/NotApplicable/Indeterminate 与组合算法比较；GrowthOS 未采用 XACML |
| [Go Memory Model](https://go.dev/ref/mem) | 数据竞争与同步语义边界 |
| [Go Data Race Detector](https://go.dev/doc/articles/race_detector) | race detector 的使用和“只发现已执行路径问题”的证据边界 |
| [Go Fuzzing](https://go.dev/doc/security/fuzz/) | Go fuzz 测试用于探索异常输入和边界 |
| [Google Zanzibar](https://research.google/pubs/zanzibar-googles-consistent-global-authorization-system/) | 关系授权、统一数据模型与一致性设计的 build-vs-buy 比较 |
| [OPA HTTP API Authorization](https://www.openpolicyagent.org/docs/http-api-authorization) | 应用向策略引擎请求授权决定的外置 PDP 示例 |
| [OpenFGA Concepts](https://openfga.dev/docs/concepts) | authorization model 与 user/relation/object tuple；不声称本项目采用 ReBAC |

## 4. 已核验的个人面经题型灵感

以下链接的标题和相关正文在 2026-08-31 可以直接读取，但均为牛客用户个人记录，未独立核验公司、轮次、逐字原题或作者答案。本文只从中提炼题目方向，所有技术回答仍以前述官方来源和项目事实为准：

| 牛客用户个人记录 | 只用作什么题型灵感 |
| --- | --- |
| [发面经攒RP 字节后端开发三面（已意向）](https://www.nowcoder.com/discuss/353156848733855744) | 权限管理系统解决什么、权限如何设计 |
| [小米Java面经](https://www.nowcoder.com/discuss/353147585177264128) | RBAC 表模型设计；帖子只写“共 7 张表”，本文不猜七张表内容 |
| [趣链JAVA后端日常实习面经-1面](https://www.nowcoder.com/discuss/723953710098886656) | 不同用户菜单、权限存储和 RBAC 的关系 |
| [面经｜腾讯S3后台开发暑期提前批（一面）](https://www.nowcoder.com/discuss/846448031389003776) | 多租户隔离与权限校验设计 |
| [安全服务实习面经](https://www.nowcoder.com/discuss/353157956873166848) | 水平越权、成因和可控对象 ID；不采用帖子个人答案作权威依据 |
| [YY后台开发全程面试经验](https://www.nowcoder.com/discuss/353157935578685440) | 数据权限控制、认证与单点登录等项目追问背景；不改写成页面未记录的精确问句 |

## 5. 不能夸大的事实

- 当前只有 `internal/governance/domain` 纯模型，没有用户登录、credential 验证或 session repository；
- `NewPrincipal` 只证明值形状，不证明 caller 身份；
- 角色模板没有真实人员 assignment，`growth_member` 也不等于会员 tier；
- tenant/owned/resource 匹配不等于多租户、行级安全或对象仓储已经隔离；
- 没有生产 handler、middleware 或 application service 调用 `Policy.Evaluate`；
- 没有 React 导航、路由、字段或按钮权限投影；
- Decision/AuditContext 不是持久审计日志、trace 或授权 token；
- 64/64/1024 是同步模型 guard，不是吞吐、延迟或用户规模 SLO；
- GrowthOS 没有采用 XACML、OPA、OpenFGA、Zanzibar，也没有完成 NIST 全套 RBAC；
- direct API、浏览器、跨角色、跨对象和跨租户越权验收属于第 35 节，不能用 domain 单测替代。

## 6. 面试作答总模板

面对任一权限题，可以按五步回答：

1. 先确定可信 Principal 从哪里来；
2. 再把目标表示为 server-derived Resource 和 exact Action；
3. 说明 Role Permission 只给 capability ceiling，RoleBinding Scope 才约束 tenant/owner/object；
4. 说明 default deny、matching deny precedence，以及 error 必须返回 zero Decision；
5. 最后诚实指出当前实现层级：模型、会话、服务端强制、前端投影还是越权 E2E，绝不跨章节夸大。

## 7. 复习清单

- [ ] 能在 60 秒内说清本节问题、模型、验证和停止线；
- [ ] 能不看文档写出 3 个 PrincipalKind、2 个 ResourceKind 和 4 个 ScopeKind；
- [ ] 能列出五个 Role 的 16/10/8/9/0 permission 数量；
- [ ] 能解释 capability 为什么是 kind/type/action 三元组；
- [ ] 能画出 Browser → L32 → L33 → Policy.Evaluate → use case 信任边界；
- [ ] 能手推 explicit allow、纯 deny、deny-overrode-allow 和三种默认拒绝；
- [ ] 能解释为什么 error 必须伴随严格 zero Decision；
- [ ] 能在代码中定位 Scope matcher、Policy validation、Decision evidence 和 architecture stop line；
- [ ] 能分别回答 BOLA、BFLA、跨 tenant 和 confused deputy；
- [ ] 能明确第 32、33、34、35 节各自新增什么，且不把未来能力说成已完成。
