# 第 31 节设计手记：从第一性原则推导统一访问控制模型

- **课程主题：** 统一访问控制模型与威胁边界
- **产品基线：** [GrowthOS 统一访问控制模型与威胁边界 v1](../../product/access-control-model-threat-boundary-v1.md)
- **架构决策：** [ADR-0027](../../decisions/ADR-0027-governance-access-control-model.md)
- **课程正文：** [第 31 节课程](../../course/part-04/lesson-31-access-control-model-threat-boundary.md)
- **API 记录：** [第 31 节 API](../../api/lessons/lesson-31.md)
- **QA：** [第 31 节 QA](../../qa/lessons/lesson-31.md)
- **面试问答：** [第 31 节面试问答](../../interview/lessons/lesson-31.md)
- **设计日期：** 2026-08-31
- **事实边界：** 本节只有 `internal/governance/domain` 中的纯策略模型；没有 credential、session、policy repository、HTTP middleware、业务用例装配、前端权限投影或真实运行时保护

> 这份手记不是从“项目需要 RBAC”这个技术名词出发，也不是先选择 OPA、OpenFGA 或一张角色表，再把业务硬塞进去。真正的推导顺序是：先找出值得保护的资产，再确定谁在请求、请求做什么、目标是哪一个、哪些事实可信、策略如何组合、失败如何封闭，最后才选择足够小的模型。

## 1. 为什么第 31 节先做模型，而不是先做登录页

第 30 节之后，GrowthOS 已经出现了有安全价值的真实资产和动作：

- Marketing Activity 可以创建、读取、发布、回滚和退役；
- Lottery Strategy 与 Routing Graph 可以创建和读取；
- Activity publication 会精确引用 Lottery 配置；
- Governance policy 与未来审计记录本身也会成为敏感资源。

一旦这些动作进入真实运营流程，“页面上谁能看到按钮”就不再是纯 UI 问题。攻击者可以绕过页面直接请求 API；同一个运营角色也不必然能操作所有租户、所有 Activity 或所有对象。只检查登录、不检查动作，会产生垂直越权；只检查功能、不检查对象，会产生水平越权；把会员等级、审批结果或数据库账号当权限，又会混淆完全不同的业务事实。

但直接做登录同样会把顺序倒过来。登录只能回答“凭据对应谁”，不能回答“这个人能否发布这个 tenant 下的 Activity”。如果授权语言尚未稳定，会话里就不知道应该承载什么最小身份、服务端不知道应该构造什么授权问题、前端也不知道应该消费什么 capability 投影。

所以依赖顺序是：

```text
L31 先定义：授权问题长什么样，怎样得到确定的 allow/deny
  ↓
L32 再证明：credential 怎样变成可信 Principal
  ↓
L33 再执行：每个服务端用例怎样构造可信 Resource/Action 并强制判定
  ↓
L34 再投影：前端怎样依据服务端给出的最小 capability 改善体验
  ↓
L35 再攻击：匿名、过期、跨角色、跨对象、跨租户、直接 API 与浏览器负向验收
```

这不是把安全延后，而是避免用一个尚未定义的问题去设计 credential、middleware 和页面。

## 2. 第一性原则：授权系统必须回答哪七个问题

把框架名全部拿掉，一个访问控制决定至少要回答七个问题：

1. **资产是什么？** 是 Activity 集合、某个 Activity、Strategy 集合，还是某个 Policy？
2. **主体是谁？** 是人、服务还是 Agent？这个引用是否已经过可信认证？
3. **动作是什么？** 是 read、create、publish、rollback、retire 还是 change？
4. **数据范围是什么？** 是全系统、一个 tenant、该主体拥有的对象，还是一个精确对象？
5. **策略是什么？** 哪些角色拥有哪种精确 capability，哪些 binding 把主体、角色和范围关联起来？
6. **组合规则是什么？** 多条 allow/deny 同时命中时谁优先；没有规则时是允许还是拒绝？
7. **证据是什么？** 决定使用了哪一版 Policy、哪些 binding、哪个 Resource 和什么评估时刻？

少掉其中任意一个，都会出现典型漏洞：

| 缺失项 | 看似可行的简化 | 实际后果 |
| --- | --- | --- |
| 资产 | 只判断 `isAdmin` | 不同资源和动作被压成一个全能开关 |
| 主体 | 相信请求体里的 `user_id` | 客户端可替换主体 |
| 动作 | 只看 HTTP method | `publish`、`rollback` 等业务风险被掩盖在 `POST` 中 |
| 范围 | 角色能进模块就能看全部 | BOLA、跨 tenant、跨 owner 越权 |
| 策略 | handler 内散落 if/else | 不同上下文产生不同权限语义 |
| 组合 | 第一条命中即返回 | 调整配置或查询顺序就可能提权 |
| 证据 | 只返回 boolean | 无法解释、审计、关联 exact policy revision |

因此本节的核心问题被压缩为：

> 一个由后续可信边界确认的 Principal，是否可以在一个 exact Policy revision 下，对一个由服务端确认事实的 Resource 执行一个 exact Action？

“由后续可信边界确认”非常重要。本节的构造器只能证明值形状合法，不能证明值来源真实。

## 3. 先盘点资产，再决定资源目录

### 3.1 资产不是数据库表，也不是页面菜单

安全资源应按业务所有权和动作语义定义，而不是按技术载体定义：

- Activity 是 Marketing 资产，即使它存储在 MySQL；
- Strategy 与 Routing Graph 是 Lottery 资产，即使页面把它们放在同一个运营工作台；
- Policy 与 Audit 是 Governance 资产；
- Redis key、MySQL table 和 RabbitMQ queue 属于基础设施 ACL，不是产品 Principal 的业务资源。

如果把表名直接当资源，`UPDATE marketing_activity` 无法表达“publish 与 retire 风险不同”；如果把菜单名当资源，直接 API 请求就绕过了模型。

### 3.2 为什么资源目录必须封闭

当前代码只承认五个 `ResourceType`：

| ResourceType | 所有者 | 本节承认的 capability |
| --- | --- | --- |
| `marketing.activity` | Marketing | collection `create/read`；object `read/publish/rollback/retire` |
| `lottery.strategy` | Lottery | collection `create/read`；object `read` |
| `lottery.routing_graph` | Lottery | collection `create/read`；object `read` |
| `governance.policy` | Governance | collection `read`；object `read/change` |
| `governance.audit` | Governance | collection `read` |

封闭目录的意义不是让扩展困难，而是让新增权限成为显式安全变更。若任意字符串都能成为资源和动作，拼写错误可能生成一个从未审核的新 permission；若未知动作自动回退为 read，更会静默扩大权限。

当前行为必须区分两类“拒绝访问”：

- 对**已注册 capability**，没有匹配 allow 会形成 confirmed deny；
- 对**未知或不合法 capability**，请求本身无效，返回 zero `Decision` 与 error，未来 enforcement 仍必须 fail closed。

技术错误不是第三种授权 Outcome，不能伪造成正常 deny。

## 4. 从主体来源推导 Principal，而不是 User

### 4.1 为什么 Principal 只有 kind 与 opaque ID

授权模型需要的是“谁在执行”，而不是一份完整用户档案。当前 `Principal` 只包含：

```text
Principal(kind, id)
kind ∈ {human, service, agent}
```

这三个 kind 是不同的安全主体类别：

- `human` 表示人类操作者或用户；
- `service` 表示工作负载身份；
- `agent` 表示受约束的 AI 或自动化执行主体。

完整姓名、头像、邮箱、会员 tier、组织名称、浏览器信息都不属于最小授权主体。把它们塞进 Principal 会让 Policy 逐步退化成无边界属性袋，也会把 PII 带进每条决定证据。

### 4.2 构造成功不等于认证成功

任何包都可以在内存里构造一个形状合法的 `Principal(human, operator-1)`。这只能证明：

- kind 属于封闭枚举；
- ID 符合 canonical grammar；
- 值不是 zero/partial。

它不能证明：

- 请求者真的控制该账号；
- session 未过期、未撤销；
- 这个人属于某个 tenant；
- 前端声明的 role 可信。

第 32 节必须把 credential 验证、session 生命周期和 trusted Principal 建立起来。本节若提前读取 header、Cookie 或 JWT，会把认证与授权绑死在一个未经设计的 transport 上。

### 4.3 为什么 service/agent 不能默认继承 human

一个 scheduler 使用数据库连接成功，不代表它拥有 platform administrator；一个 Agent 由管理员发起，也不代表它永久继承管理员全部权限。这是 confused deputy 的常见来源：具有技术能力的中间服务被诱导替调用者执行它本不该执行的动作。

当前模型只保留一个 exact Principal，不实现 delegation、impersonation 或 actor/subject 双主体链。等真实代理需求出现时，必须新增明确的委托证据、期限、动作上限和审计模型，而不是把发起人的 role 复制给 Agent。

## 5. 从业务动词推导 Action，而不是从 HTTP 推导权限

当前 Action 封闭为：

```text
create, read, publish, rollback, retire, change
```

Action 是业务语义，不是 HTTP method 的别名：

- `POST /activities/{id}/publish` 与 `POST /activities/{id}/rollback` 都可能是 POST，但风险不同；
- `PATCH` Policy 可能对应 `change`，不能因为都是写请求就等价于 Activity `publish`；
- list 与 detail 都可能是 GET，却分别作用于 collection 与 object。

因此 `AuthorizationRequest` 需要 exact `Resource + Action`，并在构造时校验 kind/type/action 组合是否出现在 capability 目录。Transport 到 Action 的映射属于第 33 节服务端用例，不由 domain 猜测。

## 6. 为什么 collection 与 object 必须成为 capability 的一部分

### 6.1 一个 ResourceType 不够

假设某人有 `marketing.activity:read`。这句话至少有两个不同含义：

1. 可以读取 Activity 列表；
2. 可以读取 Activity `activity-42` 的详情。

列表可能泄露总量、名称、租户分布和存在性；详情可能泄露精确配置。创建发生在尚无对象 ID 的集合上，发布和回滚则只能发生在已有对象上。如果 permission 只有 type/action，系统就无法表达这种差异。

当前 `Permission` 因此是：

```text
Permission(resourceKind, resourceType, action)
resourceKind ∈ {collection, object}
```

### 6.2 Resource 的合法形状

| kind | 必须字段 | 可选服务端事实 | 明确禁止 |
| --- | --- | --- | --- |
| `collection` | ResourceType | TenantID | ResourceID、owner |
| `object` | ResourceType、ResourceID | TenantID、owner Principal | 缺少 ResourceID |

Collection 可以是 system collection，也可以明确属于一个 tenant；object 的 tenant/owner 是否存在取决于权威业务数据。缺失值不是 wildcard。

### 6.3 为什么 collection create 不能复用 object permission

创建前没有新对象 ID，不能伪造一个 `new` 或 `*` ID 来套 object scope。创建目标应是 collection，tenant 应来自可信 session/membership 或服务端上下文，而不是请求体里任意选择的 tenant。

反过来，collection-read 也不能自动授予 object-read。后续第 33 节必须在 list query 之前决定集合范围，在 detail query 之前决定精确对象范围；第 35 节再验证更换 ObjectID、直接详情 URL 和列表过滤不能绕过。

### 6.4 决策表

| 请求 | 正确 kind | 仅有另一 kind permission 时 |
| --- | --- | --- |
| 创建 Activity | collection + create | object permission 不匹配 |
| 查询 Activity 列表 | collection + read | object-read 不匹配 |
| 查询 Activity 详情 | object + read | collection-read 不匹配 |
| 发布/回滚/退役 Activity | object + exact action | collection permission 不匹配 |
| 创建 Strategy/Graph | collection + create | object-read 不匹配 |

这条边界同时防 Broken Function Level Authorization 和 Broken Object Level Authorization；只做其中一个不够。

## 7. 从数据范围推导四种 ScopeKind

### 7.1 Scope 回答的不是“能做什么”

Permission 回答“角色理论上能执行什么动作”，Scope 回答“这次角色绑定覆盖哪些目标”。把 scope 放入 Permission 会导致角色爆炸：`tenant-a-marketing-operator`、`tenant-b-marketing-operator`、`activity-42-publisher` 会不断增长，角色失去稳定责任语义。

当前 `ScopeKind` **只有**：

```text
system, tenant, owned, resource
```

没有 `tenant_owned`、`workspace`、`department`、`*`、父子路径或隐式 global。`owned` 本身就是 tenant-qualified owned 语义；`resource` 本身就是 exact-object 语义。

### 7.2 精确匹配表

| ScopeKind | Scope 携带的事实 | 匹配条件 | 典型风险 |
| --- | --- | --- | --- |
| `system` | 无 | exact permission 命中后匹配任意 Resource | 高风险全局范围，但不是 superuser |
| `tenant` | exact TenantID | Resource 必须存在同一 TenantID；collection/object 都可 | 浏览器伪造 tenant、repository 忘记过滤 |
| `owned` | exact TenantID | 必须是 object，Resource tenant 相同，owner 必须等于完整 Principal | 只比 owner ID、漏比 tenant、相信请求体 owner |
| `resource` | exact ResourceType、ResourceID、可选 TenantID | 必须是 object，type/id/tenant 逐字段完全一致 | ID 替换、tenant 缺失被当 wildcard |

### 7.3 system 为什么不是管理员后门

`system` 只扩大 binding 的数据范围，不扩大 role 的 capability。一个只有 Activity read permission 的角色，即使绑定 system scope，也不能 publish Policy，更不能执行未注册动作。

它仍然是高风险配置，因为它覆盖所有 tenant；所以后续 Policy 变更流程应特别标记 system binding，但 domain 不把 scope 推断成角色。

### 7.4 tenant 的缺失事实为什么不能回退

Tenant scope 只在 Resource 明确携带同一 TenantID 时匹配。Resource 缺 tenant、tenant 不同或格式非法，都不能退回 system。

需要精确说明一个容易混淆的边界：如果 Policy 另有**显式** system allow binding，它仍然可以匹配缺 tenant 的 Resource；“缺失不扩大权限”指模型不会把 tenant scope 自动解释成 system，而不是禁止已审核的 system binding。

### 7.5 owned 为什么必须 tenant-qualified

只检查 `owner == principal` 不够。不同 tenant 可能存在同名或迁移后的主体引用；一个人在多个 tenant 也可能拥有不同对象。当前 owned 同时要求：

```text
resource.kind == object
resource.tenant == scope.tenant
resource.owner == request.principal  // kind 与 id 都相同
```

Owner 或 tenant 任一缺失都不匹配；collection 永不匹配 owned。这样可以防止“同一人跨 tenant 看自己对象”被误解释为全局 owner 权限。

### 7.6 resource 为什么连 tenant 也精确比较

ResourceID 未必天然全局唯一。Exact resource scope 因此比较 kind、type、ID 和 tenant。Scope 不带 tenant 时只匹配同样不带 tenant 的 system object，不会匹配任意 tenant 下同 ID 对象。

### 7.7 当前没有实现多租户

有 TenantID 与 tenant scope，不等于已经实现多租户隔离。当前缺少：

- 可信 tenant lifecycle 和 membership；
- tenant-scoped repository；
- 每请求强制装配；
- 跨 tenant 负向 API/E2E；
- 可能的数据库 RLS 或连接隔离纵深防御。

本节只是把未来实现必须提供的事实与 fail-closed 语义固定下来。

## 8. 从责任稳定性推导固定 Role capability ceiling

### 8.1 Role 是责任模板，不是人员分组

Role 聚合一组精确 Permission。人员或服务并不直接存进 Role；`RoleBinding` 才把 Principal 与 Role 关联。这样同一个责任模板可以在不同 tenant 或对象范围复用。

当前五个 `RoleID` 是封闭词汇：

- `platform_administrator`；
- `marketing_operator`；
- `lottery_designer`；
- `security_auditor`；
- `growth_member`。

它们不是前端 workspace 名、会员 tier、Identity Provider group 或 session claim。

### 8.2 为什么角色模板是上限而不是默认任意列表

如果 Policy 可以给 `marketing_operator` 动态塞入 `governance.policy:change`，角色名字仍然一样，实际权限却已悄悄变成管理员。代码审查、文档和面试沟通都会失真。

所以内置模板定义 capability ceiling；`NewRole` 只接受该 ceiling 的子集：

- 一个 Policy revision 可以临时减少角色能力；
- 不能给同名角色增加模板外能力；
- role 内 permission 必须精确、唯一、规范排序；
- `growth_member` 的空 ceiling 是合法的显式默认拒绝。

### 8.3 当前 exact ceiling

| Role | Activity | Strategy | Routing Graph | Policy | Audit |
| --- | --- | --- | --- | --- | --- |
| platform administrator | c:create/read；o:read/publish/rollback/retire | c:create/read；o:read | c:create/read；o:read | c:read；o:read/change | c:read |
| marketing operator | c:create/read；o:read/publish/rollback/retire | c:read；o:read | c:read；o:read | — | — |
| lottery designer | c:read；o:read | c:create/read；o:read | c:create/read；o:read | — | — |
| security auditor | c:read；o:read | c:read；o:read | c:read；o:read | c:read；o:read | c:read |
| growth member | — | — | — | — | — |

`c` 表示 collection，`o` 表示 object。即使 platform administrator 也没有 wildcard、隐式新动作或绕过 scope 的特殊分支。

### 8.4 为什么不做角色继承

从表面看，platform administrator 像是其他角色的并集，可以引入 hierarchy。但当前没有真实组织层级、冲突语义和 separation-of-duty 要求。角色继承会立即带来：

- 多层继承后的 effective permission 难以解释；
- deny 与继承 permission 如何组合；
- 修改父角色造成大范围隐式提权；
- session 激活哪些角色仍未定义。

当前五个明确模板更容易审查。只有组织责任形成稳定偏序并出现大量重复配置时，才值得为 hierarchy 单独写 ADR。

### 8.5 为什么不允许 Principal 直接绑定 Permission

直接 grant 能快速解决个案，却会绕过角色 ceiling，形成无法回答“这个人为什么有权限”的长期债务。当前模型刻意只有：

```text
Principal -> RoleBinding -> Role -> Permission
```

精确对象例外由 `resource` scope 表达，紧急限制由 deny binding 表达，不需要直接 principal-permission 通道。

## 9. 从 assignment 与 restriction 推导 RoleBinding effect

`RoleBinding` 包含：

```text
RoleBinding(id, principal, roleID, scope, effect)
effect ∈ {allow, deny}
```

它回答“谁以什么角色、在什么范围、产生 grant 还是 restriction”。Permission 本身不带 allow/deny，因为同一个角色能力可以在 tenant A 被允许，又对其中一个对象被禁止。

典型配置：

```text
allow: operator-1 + marketing_operator + tenant-a
deny:  operator-1 + marketing_operator + activity-9@tenant-a
```

这能表达“运营者可以管理 tenant-a，但事故 Activity-9 暂停操作”，而不修改全局角色模板。

需要强调：模型允许宽 allow + 窄 deny，但不推断 scope 谁更宽，也不要求 deny 必须更窄。任何配置都必须通过治理评审；domain 只执行确定的匹配规则。

## 10. 从配置顺序无关推导 deny precedence

### 10.1 为什么 first match 不可接受

若 evaluator 遇到第一条匹配 binding 就返回，结果会依赖：

- SQL 是否加 ORDER BY；
- map 遍历顺序；
- 配置文件合并顺序；
- 新 binding 插入在前还是后。

安全结果不能由这些偶然性决定。当前 Policy 在构造时规范排序，Evaluate 收集全部 matching allow/deny，再统一组合。

### 10.2 当前唯一组合规则

```text
任一 matching deny 存在 -> deny
否则任一 matching allow 存在 -> allow
否则 -> default deny
```

“matching”必须同时满足：

1. binding Principal 与请求 Principal exact 相同；
2. binding 引用的 Role 存在；
3. Role 含 exact kind/type/action Permission；
4. binding Scope 与服务端 Resource facts exact 匹配。

Nonmatching deny 不会污染有效 allow；没有目标 Permission 的 deny binding 也不会变成全局否决。反过来，matching deny 无论来自哪个 Role、scope 是否比 allow 更宽、输入顺序如何，都覆盖 allow。

### 10.3 判定表

| Policy/Request 状态 | matching allow | matching deny | 结果 | Reason |
| --- | ---: | ---: | --- | --- |
| 任一非法或损坏 | 任意 | 任意 | zero Decision + error | 无 confirmed reason |
| 合法且 Principal 无任何 binding | 0 | 0 | deny | `no_binding` |
| 有 binding，但没有 exact permission | 0 | 0 | deny | `no_permission` |
| 有 exact permission，但 scope 全不匹配 | 0 | 0 | deny | `scope_mismatch` |
| 至少一个 allow，deny 为 0 | >0 | 0 | allow | `explicit_allow` |
| allow 为 0，至少一个 deny | 0 | >0 | deny | `explicit_deny` |
| allow 与 deny 同时存在 | >0 | >0 | deny | `explicit_deny_overrode_allow` |

### 10.4 为什么选择 deny 优先

Deny 优先提供稳定的紧急 restriction 和防御性例外，避免一条广范围 grant 意外覆盖精确禁用。代价是“意外 deny”会造成可用性故障，所以 Policy 变更必须展示 effective diff，并对 system/tenant deny 做额外评审。

这是一项 GrowthOS v1 应用策略，不是 NIST Core RBAC 的必然组成，也不代表复刻 AWS IAM 或 XACML 的全部组合算法。

## 11. 从可追溯性推导不可变 Policy revision

### 11.1 为什么 Policy 必须是 snapshot

如果 Role 与 Binding 是可变共享对象，一次 Evaluate 过程中可能先读到旧 role、再读到新 binding，形成从未存在过的混合策略。事故后也无法回答当时依据哪套规则。

当前 `Policy` 因此是不可变、深拷贝、规范排序的内存快照：

- `PolicyIdentity = PolicyID + non-zero Revision`；
- roles、bindings 和 role permissions 在构造时复制；
- 输入顺序不影响决定；
- getter 返回防御性副本；
- 重复 role、重复/语义重复 binding、悬空 role、非法值和超限全部拒绝。

### 11.2 Revision 不是什么

当前 Revision 只是 exact correlation value：

- 不是 content hash；
- 不是签名；
- 不是数据库自增实现承诺；
- 纯 domain 不能证明 `(PolicyID, Revision)` 全局唯一；
- 当前没有 repository、active pointer 或 activation protocol。

未来 persistence 必须保证 exact identity 唯一，并定义如何选择 active revision；在那之前不能宣称已支持动态策略发布或撤权。

### 11.3 为什么不用 latest

如果 Decision 只记录 `policy=growthos-access`，审计时读取 latest 会把历史决定解释成新策略。Exact revision 让一次决定固定关联当时 snapshot，也为未来缓存 key、回放和变更对比提供稳定边界。

## 12. 从“可判定”推导 zero Decision + error

### 12.1 Outcome 只有 allow 与 deny

当前 `DecisionOutcome` **只有**：

```text
allow, deny
```

技术不可判定不是第三 Outcome。Policy 损坏、Request 非法、未知枚举、非法 capability、错误 AuditContext 等情况意味着系统没有形成可信决定，必须返回：

```text
Decision{} + error
```

### 12.2 为什么不能把 error 包成 deny Outcome

Enforcement 对 deny 和 error 都应 fail closed，但它们的运营语义完全不同：

| 情况 | 安全动作 | 指标/告警 | 重试 | 审计含义 |
| --- | --- | --- | --- | --- |
| confirmed deny | 阻止操作 | authorization denied | 通常不自动重试 | 策略正常拒绝 |
| zero Decision + error | 阻止操作 | authorization indeterminate/error | 视故障分类决定 | 未形成可信决定 |

如果把 repository 故障、损坏 snapshot 或程序 bug 记录成“用户没权限”，系统会隐藏安全基础设施故障，错误率也无法触发告警。反过来，如果 error 时允许，则直接 fail open。

### 12.3 为什么 Decision 还要自校验

Decision 携带 Outcome、Reason、PolicyIdentity、Principal、Resource、Action、AuditContext 和 matches。它会拒绝：

- allow 却没有 allow evidence；
- deny reason 与 match 组成矛盾；
- duplicate match；
- match Permission 与请求 capability 不同；
- 未排序、超限、zero/partial evidence。

因此 `Allowed()` 只对完整且自校验通过的 allow 返回 true；zero value 永远不是 allow。

## 13. 从审计问题推导最小 Decision evidence

### 13.1 决定需要回答什么

一次受控内部审计至少需要知道：

- exact PolicyID/Revision；
- exact Principal；
- exact Resource 与 Action；
- EvaluationRef、CorrelationRef 与 canonical evaluated-at；
- 哪些 BindingID、RoleID、Effect、ScopeKind 与 exact Permission 形成 match；
- allow、deny-only，还是 deny-overrode-allow。

当前 match 规范排序、有固定上限并防御性复制。Deny-overrode-allow 会保留冲突双方，不因最终 deny 而丢失 allow 证据。

### 13.2 AuditContext 为什么不是万能 metadata

`AuditContext` 只含两个有界 opaque reference 和 UTC microsecond evaluated-at。它不是：

- credential 或 session；
- HTTP Request 对象；
- trace span；
- 任意 `map[string]any`；
- 持久化 audit event。

任意 metadata bag 会让 PII、凭据、业务内容和不受控 key 混入判定内核。第 33 节应在受信 service layer 关联真实 request/operation，并把最小必要字段写入受保护 audit sink。

## 14. 信任边界：类型正确不等于事实可信

### 14.1 当前模型位于哪里

```text
Browser / API client              不可信 Principal、role、tenant、owner、action 声明
        |
        v
[L32 credential/session]          未来产生 trusted Principal
        |
        v
[L33 trusted service layer]       未来从权威数据加载 Resource ID/tenant/owner，映射 Action
        |
        v
[L31 Governance Policy.Evaluate]  当前仅此纯决定内核存在
        |
        v
[L33 business use case]           未来对 allow 执行动作，对 deny/error fail closed
        |
        v
Repository / DB / Redis           基础设施 ACL 仅作纵深防御
```

### 14.2 哪些字段绝不能直接相信浏览器

- PrincipalID；
- RoleID 或完整 role list；
- TenantID；
- object owner；
- ResourceType 与 Action 的最终解释；
- Policy revision；
- allow/deny reason；
- approval evidence。

浏览器当然会提交 URL、表单和期望动作，但服务端必须根据已匹配 route、用例和权威数据重新构造授权请求。

### 14.3 confused deputy 检查

每一个未来调用点都应问：

1. 服务正在以自己的 Principal 行动，还是代表 human/agent？
2. 如果代表别人，委托证据和能力上限在哪里？
3. 服务的数据库账号是否比最终 Principal 权限更大？
4. Resource tenant/owner 是由服务端加载，还是从调用方透传？
5. 一个低权限调用者能否诱导高权限服务访问另一个 ObjectID？

当前模型没有 delegation，因此不能用“service Principal + 原用户 ID metadata”自行拼出代理语义。

## 15. TOCTOU：正确判定也可能在执行前失效

TOCTOU 指 check 与 use 之间事实发生变化。授权场景至少有四种竞争：

1. Policy revision 在判定后被替换或撤销；
2. Resource tenant/owner 在判定后改变；
3. Activity 状态在判定后被其他请求推进；
4. session 在判定后过期或撤销。

本节的不可变 Policy 和 exact Decision evidence只能回答“当时依据什么判定”，不能自动让后续写入原子化。第 33 节需要按动作风险选择：

- 尽量在业务写入前就近判定；
- 将 Resource fact 读取与写入放入适当事务或用 CAS/version 防止状态漂移；
- 高风险长事务在提交前重新验证关键事实；
- 不把 Decision 当成可长期复用的 bearer capability；
- 记录 exact policy revision 和业务对象 version，区分策略竞争与业务竞争；
- 对撤权生效时限另行定义，不能从纯内存 snapshot 推导 SLO。

若未来将 PDP 外置，还要增加网络重试、缓存陈旧、policy bundle 传播和一致性 token 的 TOCTOU 分析。

## 16. 字段披露：内部可解释不等于客户端可见

内部 Reason 很有诊断价值，但也可能泄露策略形状：

- `no_binding` 暗示主体没有任何 assignment；
- `no_permission` 暗示存在 binding；
- `scope_mismatch` 暗示目标可能存在但不在范围；
- match evidence 会暴露 RoleID、BindingID、Policy revision 与 capability。

因此这些字段不是默认 API DTO。第 33 节需要按资源枚举风险设计低披露映射：

- unauthenticated、deny、not-found 和 technical error 在内部必须可区分；
- 对外使用 401、403、404 或统一错误需要按资源策略决定；
- 日志不得写 credential、完整 Policy payload 或敏感 Resource 内容；
- 列表总数、筛选项、导出、字段和聚合都属于披露面，不是隐藏详情按钮就完成授权；
- 第 34 节前端只能获得最小 capability projection，不能下载全量 Policy/Binding/Reason。

## 17. 容量上限与为什么当前选择有界线性扫描

### 17.1 当前真实上限

代码的防御性 guard 是：

- `MaxRolesPerPolicy = 64`；
- `MaxPermissionsPerRole = 64`；
- `MaxRoleBindingsPerPolicy = 1024`；
- `MaxDecisionMatches = 1024`。

但这些不是当前有效业务基数：RoleID 只有 5 个且 Policy 禁止重复 Role；内置角色最大 capability ceiling 是 platform administrator 的 16 项。64/64 是构造期资源保护和未来变更警戒线，不应被写成“当前可配置 64 个不同角色、每个角色已有 64 种有效权限”。

### 17.2 复杂度

当前 Evaluate 在不可变 snapshot 上：

1. 线性扫描最多 1024 个 binding；
2. 对同 Principal binding 查找 exact role；
3. 在线性有界 Permission 集中查找 exact capability；
4. 计算 scope match 并收集有界 evidence；
5. 规范排序 matches 后组合结果。

可以近似看成 `O(B × (log R + P) + M log M)`；当前 `R <= 5`、模板 `P <= 16`、`B/M <= 1024`。不可变结构也让并发只读容易用 race 证明。

### 17.3 为什么不立刻建立索引或缓存

索引和缓存不是免费的：

- 需要定义 key 是否包含 Principal、kind/type/action、scope 和 Policy revision；
- 撤权与 revision 切换会产生失效问题；
- deny/allow evidence 仍需可解释；
- 缓存 miss、provider 故障和 stale value 都要有 fail-closed 语义。

当前规模没有性能证据支持复杂化。只有真实 profiling 显示判定成为瓶颈，或 binding 数量、跨服务调用明显增长，才比较按 Principal/capability 建索引、revision cache 或外置 PDP。

## 18. 为什么不是其他方案

### 18.1 只做简单 RBAC

最简单 RBAC 是 `user -> role -> permission`。它适合表达功能责任，但不能单独回答：

- marketing operator 能否操作 tenant-a；
- 是否只能读取自己拥有的对象；
- 能否只禁止 activity-9；
- list 与 detail 是否不同。

当前模型保留 RBAC 的角色稳定性，又用 Scope 与 binding effect 表达 GrowthOS 所需的数据范围和 restriction，因此只能称 RBAC-inspired access-control model，不宣称完整实现 NIST Core/Hierarchical/Constrained RBAC。

### 18.2 ABAC

ABAC 可以把 tenant、owner、风险、设备、时间、网络位置等属性写入策略，表达力更强。但真正成本在属性治理：

- 谁是每个属性的权威来源；
- 属性缺失、过期、冲突怎样处理；
- 表达式如何审查、版本化、解释和测试；
- 多条 policy 如何组合；
- 动态环境属性是否导致不可复现决定。

当前需求只需要四个封闭 ScopeKind，不值得引入通用 expression engine 或 `map[string]any` fact bag。Owned/tenant 是固定领域语义，不等于已经实现通用 ABAC。

**未来触发条件：** 出现真实且反复出现的时间段、风险等级、设备可信度、数据分类等条件，并具备权威属性源与治理流程。

### 18.3 ReBAC

ReBAC 适合“用户属于团队、团队拥有项目、项目共享给另一个组织、代理代表用户”等关系图查询。当前 owned 只比较 Resource 上的 exact owner Principal，不读取关系图，也没有 group、parent、member、viewer 等关系。

提前引入 ReBAC 会要求关系 tuple 生命周期、一致性、遍历深度、循环、撤权和解释语义，而当前没有对应业务。

**未来触发条件：** 出现共享对象、嵌套团队、组织层级、委托链或大量关系继承，fixed scope 开始制造大量重复 binding。

### 18.4 OPA

OPA 适合以通用 policy language 和 PDP 方式集中评估复杂策略，也便于多语言消费者复用。但引入 OPA 不会自动解决：

- input 中 Principal/tenant/owner 是否可信；
- policy bundle 怎样发布、回滚和关联 exact revision；
- sidecar/remote PDP 故障时如何 fail closed；
- data document 与业务数据怎样保持一致；
- Rego 规则和应用 Resource/Action 词典谁拥有。

当前纯 Go 模块化单体用 typed kernel 更容易静态审查和单元测试。未来即使换 OPA，现有 AuthorizationRequest/Decision 边界仍可作为 consumer contract。

**未来触发条件：** 多语言/多服务必须共享复杂 policy，组织已经具备 policy-as-code 发布、测试、观测和故障治理能力。

### 18.5 OpenFGA

OpenFGA 面向 relationship-based authorization，适合高基数 user-object relation 与模型化继承。它比当前 fixed role + scope 更适合共享文档、团队成员关系、文件夹继承等问题，但会新增模型 DSL、tuple store、consistency、迁移和运维边界。

当前 Activity/Strategy 管理权限仍是少量责任模板与 tenant/object scope，没有证据需要关系数据库式授权查询。

**未来触发条件：** ReBAC 关系成为核心产品数据，单次 check 需要跨多跳关系且 binding 规模无法由当前模型清晰表达。

### 18.6 Zanzibar

Zanzibar 是面向全球规模、关系 tuple、低延迟 check 与一致性约束的授权系统设计，不是给单体“装一个库”就能获得的能力。它要求独立存储、索引、缓存、变更传播、快照一致性与大规模运维。

用 Zanzibar 解决当前五个 RoleID、最多 1024 bindings 的问题，会让基础设施复杂度远大于业务复杂度。

**未来触发条件：** 真正出现全球多服务、高 QPS、超大关系图、严格一致性 token 与独立授权平台团队，而不是因为简历需要一个流行名词。

### 18.7 数据库 RLS

RLS 能在数据库层限制行，是 tenant/owner 隔离的有价值纵深防御。但它不能单独表达：

- collection create 与 object publish 的业务差异；
- 跨 MySQL、Redis、MQ 的统一权限；
- Approval、Activity gate 与 authorization 的责任边界；

而且应用常使用连接池和共享数据库账号，必须安全传递 session/tenant context；配置错误会导致查询漏行或越权。RLS 也不能替代服务端在对象动作前构造 exact AuthorizationRequest。

**未来触发条件：** tenant 数据量和防御深度要求明确，repository 已完成 tenant-aware 设计，并能验证连接池/session variable 不串租户。

### 18.8 前端路由守卫

前端守卫只能改善体验：隐藏无权导航、阻止误点、减少无效请求。用户可以修改 JS 状态、手写 URL 或直接调用 API，因此它不是安全边界。

第 34 节会消费服务端最小 capability projection，但只有第 33 节服务端每请求 enforcement 才能真正阻止动作；第 35 节必须用 direct API 和浏览器负向测试证明两者没有错位。

## 19. 反事实推演：如果当初选了“更简单”的路

| 反事实 | 短期收益 | 迟早出现的失败 | 当前设计的应对 |
| --- | --- | --- | --- |
| 相信 `X-Role: admin` | handler 少写代码 | 客户端自提权 | L32 可信 Principal，Role 来自 Policy binding |
| 只隐藏菜单 | UI 很快可演示 | direct API 越权 | L33 server enforcement，L34 仅 UX |
| `isAdmin` 分散在各上下文 | 文件少 | 语义漂移、新动作漏检 | Governance 统一词典与 evaluator |
| Permission 只有 type/action | 模型简单 | list/detail、create/object 混淆 | kind 是 capability 的一部分 |
| owner 来自请求体 | 少一次查询 | 更换 owner 即越权 | L33 权威 Resource facts |
| tenant 缺失当 global | 兼容旧数据 | 数据缺失变成全局提权 | exact scope、缺失不匹配 |
| 第一条 binding 命中即返回 | 求值更快 | 顺序变化导致提权 | 全量匹配、deny precedence、规范排序 |
| Role 可任意加 permission | 动态灵活 | 同名角色静默变权 | fixed capability ceiling |
| error 统一包装成 deny | API 简单 | 授权基础设施故障被隐藏 | zero Decision + error 分轨 |
| error 时容错 allow | 可用性看似更高 | fail-open 安全事故 | enforcement 对两者均 fail closed |
| Policy 原地修改 | 写入方便 | 混版、不可追溯 | immutable exact revision |
| 把全量 Policy 发给前端 | 页面判断方便 | 策略结构与范围泄露 | L34 最小 capability projection |
| 只靠数据库账号 | 无需产品授权 | 进程身份冒充最终主体 | 产品 Decision + 基础设施纵深防御 |

## 20. 失败模式清单与处置思路

### 20.1 模型输入失败

| 失败 | 当前 domain 行为 | 未来调用方责任 |
| --- | --- | --- |
| unknown PrincipalKind/ResourceType/Action/ScopeKind | 构造失败 | 记录技术错误，不形成 allow |
| 非法 kind/type/action 组合 | Request/Permission 构造失败 | fail closed，修正目录或调用点 |
| zero/partial Principal、Resource、Scope、AuditContext | 构造/Validate 失败 | 不用默认值猜测 |
| wildcard、路径或非法 canonical ID | ID 构造失败 | 不 trim/normalize 后偷偷接受 |
| Policy 重复 Role/Binding、悬空 Role | Policy 构造失败 | 拒绝该 snapshot 激活 |
| 超容量 | 构造失败 | 拆分需求或重新评估架构，不截断 |

### 20.2 合法策略的拒绝

| Reason | 含义 | 不能误判成 |
| --- | --- | --- |
| `no_binding` | Principal 无任何 binding | 未登录、对象不存在 |
| `no_permission` | 有 binding，但 role 无 exact capability | 会员资格失败 |
| `scope_mismatch` | capability 存在，但范围不覆盖 Resource | Activity closed |
| `explicit_deny` | 只有 matching deny evidence | 技术故障 |
| `explicit_deny_overrode_allow` | allow/deny 都匹配，deny 胜出 | 随机冲突 |

这些 Reason 是内部解释，不应直接暴露给不可信客户端。

### 20.3 运行时失败尚未实现

当前没有 Policy repository、active revision、cache、provider 或 middleware，所以以下问题只能进入后续设计，不能宣称已经解决：

- Policy load timeout/not-found/corruption；
- cache stale 与撤权延迟；
- session expiration/revocation；
- Resource authoritative read failure；
- handler 漏调用 evaluator；
- 判定成功后业务事务失败；
- audit sink 不可用时是否阻断高风险动作。

## 21. 威胁检查表

| 威胁 | 当前 L31 能提供的约束 | 仍需哪一节闭环 |
| --- | --- | --- |
| 客户端伪造 Principal/Role | 类型不等于信任的明确停止线 | L32/L33 |
| 更换 ActivityID/StrategyID/GraphID | exact object/resource scope 语言 | L33/L35 |
| 跨 tenant | tenant/owned/resource exact 匹配 | L33 repository + L35 |
| owned 对象横向访问 | tenant-qualified owner 语义 | L33 authoritative owner + L35 |
| 管理动作垂直越权 | fixed role ceiling + exact Action | L33/L35 |
| 新动作默认放行 | 封闭目录；未知输入 error、无 grant deny | L33 guard + L35 |
| allow/deny 顺序攻击 | order independent + deny precedence | L31 tests，L35 integration |
| 策略混版 | immutable exact revision evidence | 未来 repository/activation |
| confused deputy | Principal kind 与信任边界明确 | 后续 workload/delegation |
| 前端 state 篡改 | 前端非安全边界 | L33/L34/L35 |
| 低披露与资源枚举 | internal reason 与客户端 DTO 分离 | L33/L35 |
| TOCTOU | exact evidence 与问题清单 | L32/L33 transaction/recheck |

## 22. 真实架构师怎样变更这套模型

### 22.1 新增 Resource/Action 的流程

不能只在枚举中加一行。应按以下顺序：

1. 写清受保护资产和 bounded-context owner；
2. 列出业务 Action，不从 HTTP method 猜；
3. 对每个 Action 判断 collection 还是 object；
4. 明确 tenant/owner/ID 的权威来源和缺失语义；
5. 把 exact capability 加入封闭目录；
6. 逐角色评审是否进入 capability ceiling；默认不进入；
7. 建立 allow、default deny、cross-kind、cross-action、cross-tenant、cross-owner 负向矩阵；
8. 更新产品基线、ADR、课程、QA、设计手记、面试问答；
9. 在第 33 节对应 service layer 增加唯一 enforcement point；
10. 在第 34 节只投影必要 capability；
11. 在第 35 节增加 direct API 与浏览器攻击验收；
12. 保留旧 Policy revision 的解释能力，并定义新 revision 激活流程。

### 22.2 修改角色 ceiling 的流程

角色能力变化是安全策略变更，不是普通配置：

- 展示旧/新 exact permission diff；
- 标记是否新增写动作、system 范围或 Governance 能力；
- 验证该角色名称仍准确描述责任；
- 检查 separation-of-duty 与审批需求；
- 对所有现有 binding 计算 blast radius；
- 生成新 Policy revision，不原地改历史；
- 增加负向测试证明相邻角色没有被连带提权；
- 准备 rollback/撤权与审计方案。

### 22.3 修改 Scope 的流程

新增 ScopeKind 必须回答：

1. 它比现有 tenant/owned/resource 多表达了什么真实需求？
2. 所需 facts 由谁权威提供？
3. facts 缺失时是否严格不匹配？
4. collection 与 object 各怎样处理？
5. 与 deny precedence、缓存 key、Policy diff 怎样组合？
6. 如何做跨范围负向测试？
7. 是否已经进入 ABAC/ReBAC，应改用更合适模型？

不允许用 `custom` 或 arbitrary expression 作为“以后再说”的逃生口。

## 23. 何时应该升级架构

| 观察到的真实信号 | 候选演进 | 仍需证明 |
| --- | --- | --- |
| 重复出现时间、风险、设备、数据分类条件 | typed ABAC 扩展或 OPA | 属性权威、组合、版本、解释、故障语义 |
| 共享、团队、组织继承、委托关系增长 | ReBAC/OpenFGA | tuple 生命周期、一致性、撤权、迁移 |
| 多语言微服务共享大量策略 | OPA/独立 PDP | 延迟、可用性、bundle 发布、缓存与观测 |
| 全球超大关系图与高 QPS check | Zanzibar 类平台 | 专门团队、存储/索引/一致性 token/SLO |
| tenant 数据需要数据库纵深隔离 | RLS/连接隔离 | pool context 安全、迁移、旁路查询验收 |
| 线性 Evaluate 在 profiling 中成为瓶颈 | Principal/capability 索引、revision cache | 命中率、撤权时限、stale 行为、证据保持 |
| 组织责任形成稳定继承结构 | Role hierarchy | 循环、冲突、deny、effective diff |
| 高风险动作要求双人审批 | SoD/constraint | assignment/activation、审批与授权边界 |
| Agent 代表 human 执行 | delegation model | actor/subject、期限、能力上限、撤销、审计 |
| 客户端只应看到部分字段 | field-level capability/projection | 服务端 query/DTO 强制与枚举风险 |

升级条件必须来自生产或产品证据，不来自“技术更先进”或“简历上更好看”。

## 24. 第 32～35 节为什么不能交换顺序

### 24.1 第 32 节：真实会话认证

先把 credential 变成可验证、可过期、可撤销的 session，再从 session 产生 trusted Principal。没有它，第 33 节只能继续相信伪造 header。

但登录成功仍不代表 authorized；第 32 节不能提前把 role 放进前端并宣称 RBAC 完成。

### 24.2 第 33 节：服务端 RBAC 强制

这是第一个真实 runtime protection：

- 每请求/每用例选择 exact Action；
- 从权威 repository 加载 Resource ID/tenant/owner；
- 获取 exact Policy snapshot；
- 对 deny 与 error 都 fail closed；
- 处理 TOCTOU、低披露、审计和遗漏调用门禁。

在此之前，本节纯模型不能阻止任何现有 API。

### 24.3 第 34 节：前端权限投影

前端只能消费服务端生成的最小 capability view，裁剪导航、路由、页面、字段和操作。它改善体验和减少误操作，但不得下载全量 binding 或替代服务端 enforcement。

如果先做前端，团队很容易把“按钮消失”误认为安全完成。

### 24.4 第 35 节：越权与浏览器 E2E

只有前面三层真实存在后，负向验收才有对象：

- anonymous/expired/revoked；
- cross-role；
- cross-object/cross-owner/cross-tenant；
- 直接 URL 与 direct API；
- 页面隐藏但 API 是否仍拒绝；
- deny 与 technical error 的低披露和审计关联。

E2E 不是替代单测，而是证明 browser → session → server enforcement → repository 的完整链路没有断点。

## 25. 候选人可复述的第一性原则思维模板

面试中遇到“设计一个权限系统”，可以按下面十二问组织答案：

1. **资产：** 我在保护哪些业务资产，而不是哪些页面或表？
2. **所有权：** 哪个 bounded context 拥有资源事实，哪个上下文拥有 access decision？
3. **主体：** human/service/agent 如何被可信识别？构造值与认证证据是否分离？
4. **动作：** 业务 Action 是什么，是否被 HTTP method 掩盖？
5. **目标：** collection 与 object 是否分开；ObjectID 是否可能被替换？
6. **范围：** system/tenant/owned/resource 需要哪些权威 facts，缺失时怎样 fail closed？
7. **能力：** Role ceiling 是否封闭，是否存在 wildcard、直接 grant 或隐式继承？
8. **组合：** 多条 allow/deny 如何确定组合，是否与顺序无关？
9. **失败：** confirmed deny 与 technical indeterminate 是否分离；错误时是否 zero result？
10. **证据：** 是否记录 exact Policy revision、Principal、Resource、Action 和决定性 match？
11. **执行：** enforcement point 在哪里，如何避免 TOCTOU、confused deputy 和字段泄露？
12. **规模：** 当前复杂度和上限是什么；什么真实指标才触发 ABAC/ReBAC/OPA/OpenFGA/RLS 或外置 PDP？

可以压缩成一段候选人答案：

> 我不会从角色表或登录页开始，而会先定义资产、主体、业务动作和资源范围。Permission 精确到 collection/object、resource type 和 action；Role 只给稳定责任设置 capability ceiling；RoleBinding 再把 Principal、Role、Scope 与 allow/deny 关联。Resource 的 tenant/owner 必须来自服务端权威数据，缺失不扩权；任何 matching deny 覆盖 allow，其他情况默认拒绝。合法拒绝返回 confirmed deny，技术不可判定返回 zero Decision 与 error。Policy 使用不可变 exact revision，Decision 保留有界内部证据。当前先用有界纯 Go 线性 evaluator，只有关系、属性或跨服务规模出现真实证据时，才升级到 ReBAC、OPA/OpenFGA 或独立 PDP。最后按认证、服务端强制、前端投影、越权 E2E 的依赖顺序落地。

## 26. 本节完成后能说什么、不能说什么

可以说：

- Governance 已拥有统一、不可变、默认拒绝的纯授权模型；
- Principal、Resource、Action、Permission、Role、RoleBinding、Policy、Scope 和 Decision 词汇已经可执行；
- `ScopeKind` 精确限定为 `system/tenant/owned/resource`；
- collection/object、tenant/owned/resource、fixed role ceiling 与 deny precedence 有明确语义；
- 技术不可判定返回 zero Decision + error，不是第三 Outcome；
- immutable revision、有界 evidence、容量 guard、线性扫描和威胁停止线已经进入设计。

不能说：

- 用户已经可以真实登录；
- Policy 已经持久化、动态发布或撤销；
- 任何现有 API 已经被 RBAC 保护；
- tenant/owner facts 已经由可信 repository 装配；
- 不同角色已经看到不同 UI；
- direct API、浏览器、跨对象或跨 tenant 越权已经验收；
- 已实现完整 NIST RBAC、ABAC、ReBAC、OPA、OpenFGA、Zanzibar 或 RLS；
- 已达到生产安全、性能、可用性或合规指标。

真正的架构成熟不是一次把所有技术都加上，而是每一步都能回答：为什么现在需要、谁拥有事实、最小机制是什么、失败时怎样封闭、证据在哪里，以及下一步在什么边界上继续演进。
