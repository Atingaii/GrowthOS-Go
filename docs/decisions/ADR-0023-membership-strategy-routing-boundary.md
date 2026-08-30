# ADR-0023：以显式会员分支证明线性资格链边界

- **状态：** 已接受
- **日期：** 2026-08-30
- **负责人：** GrowthOS 架构组
- **适用范围：** 第 27 节“责任链为什么开始不够用了”
- **需求基线：** [Lottery 会员等级 Strategy 路由基线 v1](../product/membership-strategy-routing-v1.md)
- **替代关系：** 不替代 ADR-0019 或 ADR-0022；为 Lottery 新增独立路由决定，保留 Participation 线性资格链

## 背景

ADR-0019 将 Participation 资格与 Lottery Strategy 路由分给不同决定所有者，并要求会员分层路由提供明确目标与显式缺省策略。ADR-0022 随后用“新用户 -> 风险准入”证明了 Participation 专用线性链：每个节点只有 `eligible -> 固定下一项` 与 `ineligible/error/cancel -> 终止`。

现在出现第一个真实多出口需求。外部会员 authority 可以确认主体属于受支持的会员等级，Lottery 需要根据自己的 policy 选择 premium Strategy 或 baseline Strategy。它不能被准确表达为“继续/拒绝”：

- 两种成功结果都应继续，却进入不同 Strategy；
- default 是经过产品定义的合法边，不是错误 fallback；
- 决策证据需要说明实际选择的 branch 与 target；
- 外部会员系统拥有 tier 事实，却不能拥有 GrowthOS Strategy 映射；
- Participation 资格链不能因调用方便开始输出 Lottery target。

第 23 节只说“不同会员等级可能路由到不同 Strategy”，尚未给出 exact mapping。为形成可验收切片，本 ADR 明确把 `premium override + standard baseline default` 作为第 27 节新增产品决定，而不是把它伪装成既有需求的实现细节。

## 决策驱动

1. 多出口 Route 必须和 Continue/Reject 保持不同领域语义；
2. 会员事实所有权与 Strategy 路由决定所有权必须分离；
3. default 必须显式、确定且不能吞掉 unknown/unsupported/依赖失败；
4. 同一 policy、事实快照和 evaluated-at 必须产生唯一 branch/target；
5. 事实 freshness 使用一个受控服务端 as-of，不信任客户端时钟；
6. 技术失败、caller cancellation 与确定 Route 不能共享半成品返回值；
7. path 需要足够解释所选出口，又不能复制会员画像或提前冒充审计系统；
8. 现有 Strategy 只有 ID，本节不能伪造 Strategy version 或目标发布状态；
9. 第 27 节只需要证明链的表达边界，不能提前交付第 28～29 节的持久化树和通用引擎；
10. Activity 绑定、认证与授权必须留在第 30～35 节自己的事实与决定边界。

## 评估过的方案

### 方案一：把会员路由做成 Participation chain 的第三个 gate

| 优点 | 代价 / 风险 |
| --- | --- |
| 复用现有顺序与短路 runner | gate 的成功只会进入固定下一项，无法表示不同 Strategy target |
| trace 形状已有实现证据 | Participation 开始拥有 Lottery branch/target |
| 调用点可能更少 | Route 被压成 eligible/reason，领域语义错误 |

**结论：拒绝。** Participation 继续只确认前置资格，Lottery 独立确认路由。

### 方案二：在 HTTP handler 或未来编排层直接按 tier 写 `if/else`

| 优点 | 代价 / 风险 |
| --- | --- |
| 最少类型和文件 | transport/编排者吞并路由 policy |
| 两个分支容易阅读 | default、revision、freshness、错误与 path 散落 |
| 可以直接调用 Strategy repository | 后续 MCP/任务入口会复制语义，无法稳定回放 |

**结论：拒绝。** 具体 `if/else` 可以存在，但必须位于 Lottery 拥有的具体路由领域服务中，并有明确输入、输出和失败契约。

### 方案三：让会员 authority 直接返回 Strategy ID

| 优点 | 代价 / 风险 |
| --- | --- |
| GrowthOS 无需保存映射 | 外部事实所有者越权拥有 Lottery 决定 |
| provider 可以集中运营等级 | Strategy 生命周期、发布与兼容泄漏到外部协议 |
| 请求只需一次读取 | provider 配置错误可直接路由内部资源 |

**结论：拒绝。** authority 只返回最小 tier 事实；Lottery policy 决定 branch 与 target。

### 方案四：使用 map 查找，miss 时取第一项或 default

| 优点 | 代价 / 风险 |
| --- | --- |
| 添加等级方便 | map/配置顺序可能不确定 |
| default 代码短 | unknown、typo、未来等级与依赖映射错误被静默降级 |
| 容易序列化 | 不能证明哪些 tier 被产品批准命中 default |

**结论：拒绝。** 本节使用封闭枚举与显式 `premium` / `default` 分支；只有 confirmed `standard` 可选 default。

### 方案五：本节立即建立持久化规则树与通用执行器

| 优点 | 代价 / 风险 |
| --- | --- |
| 能表达未来任意分支 | 没有 root、edge、cycle、depth、发布或 schema 兼容证据 |
| 可以支持运营配置 | 提前混合第 28 节持久化和第 29 节执行学习目标 |
| path 看似天然存在 | 通用 Context/Rule 容易吞并 Eligibility、Authorization 和 Inventory |

**结论：暂不采用。** 第 27 节只实现/规定一跳 concrete router；第 28～29 节基于真实分支再引入最小树和执行预算。

### 方案六：Lottery concrete router + premium override + standard default

| 优点 | 成本 / 风险 |
| --- | --- |
| 两个真实成功出口直接证明 chain 表达不足 | 当前映射固定，尚不能运营配置 |
| 事实与决定所有权清楚 | 目标只验证 ID 形状，不证明存在或发布 |
| default 可被精确测试，unknown 仍失败关闭 | 尚无真实会员 adapter |
| 一跳 path 足以解释决定且不创建万能图 API | 下一节仍需重新建模持久化结构 |

**结论：采用。** 复杂度与当前证据相称。

## 决策

### 1. 新增的 exact product mapping

本节冻结以下语义：

```text
confirmed premium  -> branch premium_override -> premium Strategy target
confirmed standard -> branch baseline_default -> baseline Strategy target
unknown/unsupported/missing/stale/future/corrupt
                   -> zero Route + typed error
```

`standard` 与 `premium` 是外部 authority 确认的受支持 tier；`default` 是 Lottery policy 边而不是 tier；`unknown` 不是可路由业务等级。default 只承接已确认 `standard`，绝不充当 provider error、未识别值或未来等级的 fallback。

如果未来产品需要 guest/trial/unclassified，必须新增有名称的业务等级、policy revision 和兼容证据，不得改变 `unknown` 的失败语义。

### 2. 外部 authority 拥有事实，Lottery 拥有决定

外部会员 authority 通过 consumer-owned reader 提供最小不可变快照：opaque subject ref、`standard/premium`、authority observed-at、source 与 fact revision。它不提供 Strategy ID、Lottery branch、角色、权限或完整会员画像。

Lottery policy 明确包含 policy revision、premium target 与 baseline/default target。Lottery 验证事实后纯计算唯一 branch、target 与一跳 path。它不改变会员等级，也不宣布 Participation、Activity 或 Authorization 已通过。

第 27 节可以使用 Lottery 本地 `MembershipSubjectRef`，但它不是 Principal，也不应直接复用 Participation 的 `ParticipantRef` 来伪造跨上下文统一身份。可靠身份映射留给后续真实流程。

### 3. Route decision 与最小一跳 path

成功决定至少携带：

- stable rule code `lottery.membership_tier.route_strategy`；
- branch code `premium_override` 或 `baseline_default`；
- reason code `premium_strategy_selected` 或 `baseline_strategy_selected`；
- target Strategy ID；
- policy revision；
- fact source/revision；
- 唯一 canonical UTC evaluated-at；
- 只包含“会员等级决定 -> selected branch -> Strategy target”的一跳 path。

path 记录实际选择，不列出未走边，不包含 subject ref、原始 tier payload、会员详情、provider error 或随机材料。返回值不可由调用方修改。即使两个合法 branch 暂时指向同一个 Strategy ID，branch/path 仍须保留；本 ADR 不在无产品证据时强制两个 target 永远互异。

错误或取消返回零 Route 和零 path。Route 结果不是 eligibility、authorization、Draw、reward 或 delivery。

### 4. 一次受控 evaluated-at 与严格 freshness

求值先完成输入/依赖/policy 校验并检查 pre-cancel，再读取受控 Clock 一次，随后读取一次会员事实。reader 返回后 caller cancellation 优先；provider 同时返回 fact/error 时 error 胜出。

所有事实校验使用同一个 UTC evaluated-at：

- observed-at 等于 evaluated-at 合法；
- observed-at 晚一纳秒即 future；
- age 恰好等于 max age 合法；
- 超过一纳秒即 stale。

adapter 不能用读取时刻给旧快照续鲜。单一 as-of 是逻辑决策基准，不承诺跨 authority 原子快照或历史查询能力。

### 5. 失败关闭但不伪装拒绝

zero/unknown/unsupported tier、主体不符、空 source/revision、future、stale、clock failure、not found、provider unavailable、未分类读取错误与 caller cancellation都不能命中 default，也不能形成确定业务拒绝。

对外错误只暴露稳定、安全、低基数 class；事实读取边界只通过显式 `Cause()` 保留原始错误供受控诊断，不实现 `Unwrap()`，避免 raw provider error 进入 `errors.Is` tree；`Error()` 不渲染 endpoint、SQL、subject、完整 provider payload 或敏感会员信息。caller 自身取消仍原样返回并可由 `errors.Is` 识别；provider deadline 而 caller 仍存活时仍是 dependency read failure。

### 6. 保留现有职责边界

- Participation 新用户/风险链继续只返回资格领域决定；
- Strategy 聚合与 `WeightedSelector` 不接收会员事实，不读取 Clock/reader，不负责路由；
- 本节 router 不加载 Strategy、不验证 target 存在/发布/版本、不调用 selector；
- 编排者未来可以先取 Participation 决定再取 Lottery Route，但不拥有二者规则；
- 客户端不能提交可信 tier、branch、target、policy revision 或 evaluated-at。

### 7. 第 27 节停止线

本节只允许 Lottery domain/application 内核、单元/架构测试与配套文档。明确不新增或改变：

- 会员 provider adapter、SQL/Redis repository、Migration 或缓存；
- HTTP route、DTO、status/header、runtime config、Compose 或 React；
- Strategy repository lookup、WeightedSelector、随机票据、Draw/Result；
- generic `Rule`、`RuleTree`、`Engine`、DSL、runtime priority 或 `map[string]any`；
- Activity 聚合、发布绑定与 Strategy/rule graph version；
- 会话、Principal、RBAC/ABAC、tenant、资源动作、前端权限投影；
- 在线门控、生产 SLO、持久化审计或浏览器 E2E 声明。

第 28 节负责最小规则树持久化和图合法性；第 29 节负责已验证图的多步执行与预算；第 30 节负责 Activity 发布绑定；第 31～35 节负责认证、服务端授权、前端投影与越权验收。

## 影响

### 正面影响

- 首个真实多出口 Route 清楚暴露线性 continue/reject chain 的边界；
- 会员事实与 Lottery Strategy 映射的所有权没有互相吞并；
- default 成为可版本化、可测试的产品边，而不是错误降级；
- 单一 as-of、严格 freshness、零错误决定和最小 path 提供确定性证据；
- Participation chain、Strategy 聚合与 WeightedSelector 保持各自小而可证明；
- 下一节建立树 schema 时已有 branch/default/target/path 的真实词汇，而不是从技术类图倒推需求。

### 成本与限制

- 当前 policy 是显式内核输入，不支持数据库配置、运营审批、灰度或回滚；
- 没有真实会员 adapter，不能声称外部 authority 已联通；
- target 只校验非零 ID，不证明 Strategy 存在、可用、已发布或具有版本；
- 一跳 path 不是通用规则图、持久化审计或 OTel trace；
- branch 是会员等级派生信息，需要后续访问控制与保留策略；
- 单一 logical as-of 不消除跨 authority 的 TOCTOU；
- 没有 API/runtime 组合，现有公开 ephemeral selection 没有因此获得会员路由保护。

### 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| unknown 被误当 ordinary user | 封闭枚举；事实校验先于分支；只有 confirmed standard 可走 default |
| adapter 给旧事实续鲜 | observed-at 由 authority 提供；精确边界测试 |
| provider 越权指定 Strategy | consumer-owned 最小事实端口不含 target |
| 路由 target 配置损坏 | 本节只接受非零 ID并失败关闭；第 28/30 节补引用/发布完整性 |
| trace 泄露 tier | 不含 subject/payload；branch/revision/target 不作普通 metric label；后续按权限保护 |
| 为复用污染 Participation | 架构测试禁止 Lottery 路由进入 Participation chain |
| concrete router 被无限扩张 | 多层/共享/合流出现时停止增加 hidden if/goto，转入第 28～29 节 |

### 撤销与演进

本 ADR 本身不要求 schema、route 或外部协议迁移。若第 27 节内核尚未被其他模块组合，撤销只涉及 Lottery 内核与文档；一旦 rule/branch code 或 policy revision 进入持久化证据，不能改义复用。

线性演进路径为：

1. 第 27 节以 concrete membership router 证明多出口/default/path；
2. 第 28 节把真实概念建成可验证的最小持久化树；
3. 第 29 节只执行已验证图并引入多步 path 与资源预算；
4. 第 30 节由 Activity 发布版本引用规则图和 Strategy；
5. 第 31～35 节建立可信 Principal、服务端授权与前端能力投影。

## 重新评估触发条件

出现以下任一证据时新增 ADR，不静默扩大本决定：

- 新增第三个及以上受支持会员等级或 tier 组合；
- default 的适用集合需要运营配置；
- 出现多层条件、共享子路径、合流、循环风险或不同 hit policy；
- 需要运营无代码编辑、审批、灰度、模拟或回滚；
- provider 无法提供可信 observed-at/revision 或需要缓存/历史 as-of；
- Activity 需要原子绑定 route policy 与 Strategy version；
- route trace 需要持久化、对外展示或受合规审计；
- 线上错误路由、陈旧事实或 provider timeout 成为可测事故；
- 会员等级被错误用作身份、角色或授权证据。

## 验收证据

第 27 节的实现验收必须能够证明：

1. premium 与 standard 分别选择 `premium_override` / `baseline_default` branch 和确定 target；
2. unknown/unsupported/not-found/unavailable/stale/future/corrupt 均得到零 Route，绝不 fallback；
3. pre-cancel、单次 Clock、单次 reader、精确 freshness 边界和显式 `Cause()` 通道可测试；
4. 成功 path 只有一跳且不可被调用方改写，失败 path 为零；
5. 并发/race/fuzz 不暴露共享请求态或非法枚举 panic；
6. Lottery 本节生产代码不 import Participation、Gin、SQL、Redis、React 或 Governance；
7. Strategy/Repository/WeightedSelector、ephemeral API、Migration、Compose 与 UI 行为不变；
8. 文档与验证不声称 adapter、在线门控、公开 API 或浏览器 E2E 已完成。

## 相关资料

- [Lottery 会员等级 Strategy 路由基线 v1](../product/membership-strategy-routing-v1.md)
- [Lottery 业务规则需求基线 v1](../product/lottery-rule-requirements-v1.md)
- [Participation 前置资格链基线 v1](../product/participation-prerequisite-chain-v1.md)
- [ADR-0019](ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [ADR-0022](ADR-0022-participation-prerequisite-chain.md)
- [Go Code Review Comments：Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)
- [Go context package](https://pkg.go.dev/context)
