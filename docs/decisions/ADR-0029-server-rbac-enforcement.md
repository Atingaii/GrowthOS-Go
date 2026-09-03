# ADR-0029：在业务用例内部强制 exact Policy RBAC

- **状态：** 已接受，待第 33 节按阶段实现与验收
- **日期：** 2026-09-03
- **决策者：** GrowthOS-Go 课程演进
- **关联：** [服务端 RBAC 强制执行基线 v1](../product/server-rbac-enforcement-v1.md)、[ADR-0027](ADR-0027-governance-access-control-model.md)、[ADR-0028](ADR-0028-identity-session-authentication.md)

## 背景

GrowthOS-Go 已经有：

- 封闭的 Principal、Resource、Action、Permission、Role、Scope、RoleBinding 与 Policy 模型；
- 确定性 default-deny evaluator 与 deny-overrides-allow；
- 本地 workforce credential、MySQL server-side Session 与可信 `VerifiedSession`；
- 一个已装配的 development/test-only Lottery ephemeral selection API。

但业务 API 仍未消费 Session 或 Policy。攻击者只要直接调用 API 就可以绕过任何未来前端菜单隐藏。现有 Strategy schema 也没有 tenant/owner，RoleBinding 尚无持久化权威，Decision 尚无 durable audit。

第 33 节必须建立第一条真实服务端授权纵切，同时避免提前把前端投影、完整越权浏览器 E2E、动态权限后台或正式 Lottery Draw 混入一节。

## 决策

### 1. 保护现有 Lottery ephemeral selection

首个 protected use case 是：

```text
POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections
```

它继续只在 development/test 显式 feature gate 下注册，继续要求 Demo-Mode acknowledgement，继续返回 non-durable ephemeral result。

### 2. 新增 exact `simulate` Action

该调用会产生一次新的随机选择，不是读取同一个稳定表示。新增：

```text
object:lottery.strategy:simulate
```

只有 `platform_administrator` 与 `lottery_designer` 的代码评审 capability ceiling 包含它。真实权限仍由 active exact Policy revision 的 RoleBinding 决定。不得把 `read`、HTTP POST 或“已登录”当成 simulate 权限。

### 3. Policy revision 持久化完整角色子集与 assignment

使用规范化 MySQL 表保存 immutable Policy header、roles、role permissions 和 role bindings；用一个 CAS active pointer 指向完整 revision。代码继续拥有封闭词典和角色能力上限，数据库 revision 可以收窄但不能扩权。

operations-only publisher 在单事务中 append 完整 revision 并激活；runtime 只读 Policy/active，不能变更 assignment。授权审计由 runtime 以独立 INSERT 权限写入 header + matches。

### 4. 每请求读取一个一致 Policy snapshot，不引入缓存

runtime 在 read-only repeatable-read transaction 中读取 active pointer 与 exact revision。禁止 latest 查询、部分回退和 Redis fallback。撤权传播边界是下一请求；已开始的 ephemeral 请求可以按 audit 中记录的 revision 完成。

### 5. Principal、Resource 和 Decision 分属不同边界

- Identity request authenticator：Cookie → VerifiedSession；
- composition/integration：VerifiedSession PrincipalID → Governance human Principal；
- Lottery：加载 canonical Strategy 后提供 resource facts；
- Governance：读取 Policy、evaluate、audit、返回闭合授权分类；
- Lottery selector：只在 confirmed audited allow 后执行。

Identity 不 import Governance，Governance 不读 Lottery 表，Lottery application 不解析 Cookie 或 SQL Policy。

### 6. 强制点位于 Strategy load 与随机选择之间

加载与使用同一个 immutable Strategy snapshot，授权后不重复查询 Strategy。deny、Policy error、audit error 或 cancellation 下 selector 必须为零调用。

### 7. 公开错误采用低披露矩阵

- unauthenticated Session：401 `unauthenticated`；
- Identity dependency：503 `authentication_unavailable`；
- invalid Strategy ID：400 `invalid_strategy_id`；
- authenticated not-found 与 confirmed deny：同形 404 `lottery_strategy_not_found`；
- Policy/evaluator/allow-audit 技术失败：503 `authorization_unavailable`；
- audited allow：进入原有 200 或业务错误语义。

内部 Decision reason、Role、binding、scope、Policy revision 和 audit reference 不进入客户端错误。

### 8. allow audit 是执行业务的前置条件

confirmed allow 的 audit header + matches 必须同事务持久化成功，才能调用 selector。allow audit COMMIT outcome unknown 也按 503 且不盲重试。

confirmed deny 仍同步尝试 audit；若 audit sink 故障，为保持 forbidden 与 not-found 同形，继续返回 404，并发出不含敏感值的 emergency operational signal。技术错误不伪造成 domain Decision。

### 9. 当前 CSRF 边界保持为 development-only 精确组合

当前 route 无持久业务副作用，且已有 non-safelisted exact Demo header。第 33 节增加 exact Origin / Fetch Metadata 校验，并依赖 no-CORS、SameSite 与唯一 Session Cookie grammar；不改变当前 React request signature。

该决定不适用于正式 unsafe writes。Activity publish、Policy change、正式 Draw 等必须使用 session-bound CSRF，并独立验收。

### 10. 三个 MySQL authority 使用三个不别名 pool

business、Identity、Governance pool 使用不同 credential、Secret、grants 和 readiness probe。任一 required dependency 缺失或别名时进程启动失败。Redis 不参与认证、Policy、assignment 或 audit authority。

## 备选方案

### A. 只在前端按角色隐藏菜单

拒绝。浏览器不是安全边界，直接 API 调用不经过导航与路由。前端裁剪留给第 34 节，仅改善体验。

### B. 通用 Gin middleware 只检查 `role == admin`

拒绝。middleware 不拥有服务端 Strategy facts，也无法精确表达 action、object ID、scope、deny precedence 与 TOCTOU。认证可以 route middleware 化，资源授权必须进入业务 use case/decorator。

### C. 把 simulate 当成 object read

拒绝。会意外让 security auditor 和其他只读角色触发随机试运行，也让审计失去业务动词。

### D. 把角色放进 Session/JWT

拒绝。Session 生命周期与 Policy 生命周期不同；snapshot claim 会引入撤权延迟、stale scope 和 token invalidation 复杂度。Session 只提供 trusted Principal。

### E. composition root 硬编码一个 baseline Policy

拒绝。它不能表达真实人员 assignment、不可运营、不可审计 revision，也无法证明旧 volume 的权限配置。测试 stub 可以使用内存 Policy，production composition 必须读取权威源。

### F. 只持久化 `principal_id + role_id`

拒绝。scope/effect 和 role permission subset 会丢失；assignment 原地变更会让同一 Policy revision 历史含义漂移。

### G. 角色模板只存在代码，Policy 只保存 RoleBinding

未采用。即使给 code catalog 加 version，也会让历史重放依赖旧二进制，并丢失第 31 节已经支持的 role narrowing。完整 permission subset 一并持久化更符合 immutable Policy 语义。

### H. 每次查询最大 revision

拒绝。最高 revision 不等于已激活 revision；并发发布可能暴露半成品。使用单事务 append + CAS active pointer。

### I. Redis/本地缓存 Policy

暂不采用。当前没有真实流量证据证明 MySQL snapshot load 是瓶颈；缓存会先引入撤权传播 SLA、invalidations、stale allow 与 fail-open 风险。

### J. deny 和 not-found 都返回 403

拒绝。该 exact-object endpoint 的 Strategy ID 可枚举，403 会确认对象存在。统一返回 404；未来集合动作再单独决定 403。

### K. 所有 audit 异步写入 MQ

拒绝。当前没有已验收 MQ delivery/outbox，异步写会让 allow 在审计永久丢失时已经执行。先同步建立安全闭环，后续有性能证据再演进 outbox。

### L. 现在接入 Casbin、OPA 或 OpenFGA

暂不采用。当前需求是一个模块化单体、五个封闭 role、exact action/resource/scope 和单个受保护用例；现有 domain evaluator 已满足语义。引入外部 DSL/sidecar 会新增翻译、部署、可用性和策略供应链问题。以后出现跨服务关系授权、独立策略团队或高频热更新再评估。

### M. 先实现动态权限管理 UI

拒绝。没有服务端 enforcement 时，管理 UI 只能编辑一份不会保护业务的配置。第 33 节先交付 operations-only publisher；自助后台必须在后续真实需求、审批和 SoD 边界明确后演进。

## 后果

### 正面

- 直接 API 请求首次受到真实服务端 default-deny 保护；
- 身份、资源和 Policy 事实来源独立且可追溯；
- exact action 避免只读角色意外获得模拟能力；
- immutable revision + durable matches 支持审计与历史解释；
- 独立数据库身份能在 grants 层证明最小权限；
- 后续任何业务用例可以复用统一 Principal/Resource/Action 模型，而不是再次发明权限。

### 代价

- 每次请求增加 Identity Session 解析、Policy snapshot 查询和 audit 写；
- 新增第三 MySQL pool、七张表、publisher、Secret、grants、readiness 与运维流程；
- deny 审计在数据库全面故障时只能降级为 emergency operational signal；
- 当前 Strategy 无 tenant/owner，无法借本节证明多租户；
- active revision 切换与已开始请求之间存在有界 in-flight window。

### 风险

- Policy publisher 的部分提交、重复发布和 COMMIT outcome unknown；
- repository 把不同 revision 行混合或静默忽略未知枚举；
- scope nullable union 在 SQL 与 domain 恢复间不一致；
- audit 写放大 p99 或连接池压力；
- 错误映射意外泄露 deny reason 或对象存在性；
- 新的 Cookie-auth POST 未维持 Origin/no-CORS/custom-header 组合；
- 架构门禁被简单删除后，其他模块任意 import Governance domain。

每项都需要代码级负向测试、真实 MySQL/HTTP 证据和运维 Runbook，不以 ADR 本身视为解决。

## 发布与回滚

1. 先以 additive forward migrations 建表；
2. 创建/reconcile governance runtime 与 publisher grants；
3. 发布一个完整、已验证、默认最小权限 Policy revision；
4. 装配第三 pool、request authenticator、enforcer 和 audit；
5. 保持 feature gate，只在 disposable development 环境执行真实 Cookie/API 验收；
6. 通过后再冻结第 33 节分支。

应用回滚使用上一镜像并关闭受保护 route；保留 additive tables 和 audit，不执行破坏性 down migration。Policy 回滚不是修改旧 revision，而是发布一个新 revision，其内容可复制已审核历史版本，再 CAS 激活。若 publish COMMIT outcome unknown，先按 exact identity/content digest 调查，禁止盲目再次激活。

## 验收门禁

- capability/role ceiling 全矩阵；
- Policy publisher append-only、digest、CAS 与冲突；
- repository RR exact snapshot、上限、腐败、unknown enum 与 mixed revision；
- Identity Cookie-only authentication 与伪造 header 反证；
- Strategy load → authorize → audit → selector 调用顺序；
- no binding/no permission/scope mismatch/explicit deny/deny-overrides-allow；
- 401/404/503 同形与无内部 evidence；
- audit header/matches 原子性和 COMMIT outcome unknown；
- 三 pool 不别名、三权威 readiness；
- clean/fresh 与 existing volume grant reconciliation；
- real Cookie jar、Governance outage/recovery、Redis outage 不改变 Policy authority；
- 泄密扫描、race、fuzz、full verify 和精确清理。

## 停止线

本 ADR 不授权在第 33 节实现前端 capability projection、导航/路由/按钮裁剪或完整浏览器越权矩阵。它也不声称完成多租户、正式 Draw、Policy 自助后台、production TLS 或独立授权服务。
