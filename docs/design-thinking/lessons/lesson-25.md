# 第 25 节设计手记：先证明谁有资格作决定，再讨论怎样组合规则

> “不是所有用户都能抽”表面上像一个 `if`，真正困难的却是：谁是这个用户、谁拥有“新用户”的原始事实、事实多旧仍可使用、哪些失败是确定拒绝、以及在没有真实身份和 Activity 时系统最多能诚实实现到哪里。
>
> 本节的结论是：**外部 Account / 用户目录继续拥有注册事实；Participation 只消费最小、带来源与版本的注册事实快照，并拥有本场景的资格决定。** 第 25 节实现一个具体的 registration-cutoff policy、纯 evaluator 和 application-owned fact reader，不持久化事实、不接公开 API、不缓存决定，也不提前创建责任链或规则引擎。
>
> 本手记记录 2026-08-30、实现证据提交 `475804b` 的时间切片。规范性边界以 [ADR-0021](../../decisions/ADR-0021-participation-new-user-eligibility.md) 和[新用户资格规则基线 v1](../../product/new-user-eligibility-v1.md)为准；实际执行证据与未验项以[第 25 节 QA](../../qa/lessons/lesson-25.md)为准。

---

## 1. 决策命题与时间切片

### 1.1 此刻真正要决定什么

第 17～24 节已经形成一条真实但仍是无用户语义的 Lottery 路径：

```text
canonical StrategyID
  -> authoritative Strategy/Award read
  -> optional rebuildable Redis projection
  -> unbiased WeightedSelector
  -> development/test ephemeral response
```

这条路径能回答“给定一份合法 Strategy，随机选择哪个 Award”，却不能回答“这个主体是否有资格进入选择”。第 23 节已经从需求上规定：新用户资格属于 Participation，不能塞进 WeightedSelector；第 24 节进一步规定用户资格与权限不能进入 Strategy cache。

因此本节的问题不是“给 Lottery service 加一个 bool”，而是：

> 在没有真实 session、Activity、用户事实 adapter、Participation 记录和正式 Draw 的前提下，如何让第一条用户资格成为可执行、可测试的服务端决定，同时不伪造尚不存在的安全与业务闭环？

这个命题必须同时回答：

1. `registered_at` 的权威来源是谁；
2. Participation 获得什么最小事实，而不复制完整用户模型；
3. “新用户”的政策边界和版本如何表达；
4. cutoff、freshness、future time 和时区怎样比较；
5. eligible、ineligible、unknown/unavailable 如何避免互相冒充；
6. caller cancellation 与 provider timeout 谁优先；
7. 决定中哪些字段可追溯、哪些不应外泄；
8. 本节为何能进入 production package，却不能接现有公开 route；
9. 为什么只有一条规则时不能证明一个通用 Rule/chain 是正确抽象。

### 1.2 当前已经实现的能力

实现时间切片包含：

- Participation-owned `ParticipantRef`，明确不是 Principal 或认证证明；
- 最小不可变 `RegistrationFactSnapshot`，包含 registered-at、observed-at、source 与 source revision；
- 具体、带版本、含边界的 `NewUserPolicy`；
- 固定 rule code `participation.new_user.registered_on_or_after`；
- `NewUserEligibilityDecision`，只表达 confirmed eligible/ineligible；
- 不读 I/O、不读系统时钟的纯领域 evaluator；
- application-owned `RegistrationFactReader`；
- application-owned `Clock` / `ClockFunc`；
- freshness、主体一致性、future fact、取消和 provider error 的服务编排；
- 安全错误字符串与受信 cause 的分层；
- 领域、应用、并发、race 与 AST 架构停止线测试。

主要入口：

- [Participation domain](../../../internal/participation/domain)
- [Participation application](../../../internal/participation/application)
- [领域 evaluator](../../../internal/participation/domain/new_user_eligibility.go)
- [应用 service](../../../internal/participation/application/new_user_eligibility.go)
- [端口定义](../../../internal/participation/application/ports.go)
- [架构停止线测试](../../../internal/participation/application/architecture_test.go)

### 1.3 当前明确没有实现的能力

本节没有：

- 真实用户目录或本地事实 adapter；
- MySQL table、Migration、grant 或 Redis key；
- Principal、session、authentication、authorization 或 RBAC；
- Activity、规则集发布、cutoff 配置来源或运营审批；
- HTTP route、header、DTO、status、error code 或 React UI；
- Participation request、quota、reservation、Draw、Result 或幂等；
- 两条规则、线性 chain、规则树或决策引擎；
- 正式 audit record、事实 snapshot persistence 或 replay；
- Compose/浏览器资格 E2E。

这不是“只做了一半”的包装，而是本节最重要的停止线：没有可信主体时，接 API 只会把可伪造的 participant ID 包装成安全闭环；没有事实同步协议时，新建本地 user table 只会制造第二事实源；只有一条规则时，通用 chain 只能来自想象。

## 2. 不可争辩的事实、约束与待验证假设

### 2.1 业务事实

- Eligibility 回答的是主体是否满足某次 Participation 的业务前置条件；
- Authentication 回答主体是谁，Authorization 回答已识别主体可否对资源执行动作；
- Lottery 终端选择只在前置资格已经可信成立后发生；
- `no_reward` 是合法 Lottery outcome，不是资格拒绝；
- “用户不是新用户”是确定业务拒绝；“不知道用户何时注册”是无法决定；
- 资格通过不等于已经消费次数、创建参与记录、中奖或发奖。

这些概念如果混在一个 bool 或一个 HTTP 403 中，系统就无法准确决定重试、告警、用户文案、业务指标与审计责任。

### 2.2 事实所有权约束

外部 Account / 用户目录拥有账户生命周期，包括注册、合并、更正、注销和删除。Participation 没有足够信息重建这些事实，也不应因“需要判断新用户”而获得修改用户的权力。

Participation 拥有的是衍生决定：

```text
authoritative registration fact
  + versioned Participation policy
  + controlled evaluation instant
  -> Participation eligibility decision
```

这里的 source string 只是 trace metadata，并不能使任意快照自动变权威。真正的信任来自 composition root 只注入经过审核的 adapter；domain constructor 验证 shape，不验证组织级 authority。

### 2.3 当前平台与代码约束

- 现有 ephemeral Lottery route 明确没有用户身份；
- 当前 composition root 只装配 Lottery、MySQL 和可选 Strategy Redis cache；
- MySQL latest migration 仍为 2，没有 Participation 用户事实表；
- Strategy cache 只允许可由 Lottery MySQL 事实重建的 Strategy projection；
- 当前没有 Activity aggregate，所以无法把 cutoff 声称为“某活动已发布规则”；
- 当前只有一个具体 Participation eligibility rule；
- Go domain package 应保持无 I/O、无 context、无 framework 依赖；
- application service 可以协调 context、reader、clock 和 typed errors，但不拥有外部资源生命周期。

### 2.4 外部系统事实

NIST 关于 attribute-based access control 的资料提醒：属性有 authority、质量、时效和语义问题，不能因为拿到一个字段就假设它适合所有决定。XACML 把 Deny 与 Indeterminate 分开，也支持本项目不把技术未知伪装成业务拒绝的原则。但本节做的是 Participation eligibility，不是实现 ABAC/XACML 或授权引擎。

Go interface 由消费者定义，使 Participation 能表达自己真正需要的最小事实读取能力；这不等于“每个依赖都抽一个接口”，也不等于 adapter 的正确性已经自动获得。

### 2.5 仍待验证的假设

以下都不是当前实现已证明的事实：

- 外部目录使用正 `uint64` 就能稳定标识所有参与主体；
- `registered_at` 不会因账号合并、导入或更正而变化；
- provider 能给出可靠且可比较的 observed-at；
- source revision 唯一、单调或可用于冲突解决；
- 每次参与都实时读取外部目录能满足未来延迟和可用性目标；
- max fact age 应是全局 source contract，而不是 Activity policy 的一部分；
- 未来产品对“新用户”的定义仍是平台注册时间下界；
- 一个 confirmed decision 无需持久化即可满足审计；
- 当前错误因果链在真实 adapter 中始终只有一个 application semantic class。

这些假设进入第 13 节风险账本，不能被单元测试绿色自动升级为产品承诺。

## 3. 为什么现在做，为什么只做到 domain/application

### 3.1 为什么第 23 节没有直接写规则

第 23 节先解决事实归属和阶段语义，是因为在不知道谁拥有事实、谁拥有决定、错误怎样分类前写代码，很容易得到以下错误接口：

```go
CanDraw(isNewUser bool, hasQuota bool) bool
```

它看起来简洁，却隐藏了主体、事实来源、时间、版本、unknown、取消、额度并发和最终消费。先形成需求停止线，才知道第一个可执行切片应该是 Registration fact + concrete policy，而不是万能 map。

### 3.2 为什么第 24 节仍先做 Strategy cache

Strategy 是共享、可重建配置；资格是主体相关、高时效决定。先把 cache 限制在可重建 Strategy projection，能避免后续因为 Redis 已存在就顺手缓存用户资格。第 24 节的 cache 不是第 25 节事实 adapter，也不具备用户撤销、隐私或授权语义。

### 3.3 为什么本节必须有 application slice

只有纯函数 `registeredAt >= cutoff` 无法证明生产调用方不会：

- 信任客户端提交时间；
- 忽略 fact not found；
- 对 stale fact 仍形成拒绝；
- 在不同时刻多次读取 clock；
- 把 provider timeout 当 caller timeout；
- 忘记校验 fact subject 与请求 subject 一致。

application-owned reader 与 service 把这些风险变成接口、顺序和测试，而不是留给未来每个 handler 自己拼装。

### 3.4 为什么本节不接 API

现有 route 的 `X-GrowthOS-Demo-Mode` 只确认调用者知道这是 ephemeral demo，不认证用户。再加一个 `X-Demo-User-ID` 仍可任意伪造。若现在据此门控 Lottery，页面演示可能很完整，安全事实却是假的。

把 slice 留在 production package 但暂不公开，与早期先建立 Lottery domain/repository/selector、随后才开放 ephemeral API 的线性演进一致：先证明一个内部能力正确，再等待真正具备消费者前提。

### 3.5 为什么不持久化事实

在没有 ingestion、更正、删除、迟到、revision 冲突、保留期、加密和访问审计前，本地表无法被称为“外部注册事实投影”。它只会成为一个容易陈旧、没人负责纠错的第二事实源。

本节的 snapshot 是一次 evaluation 的值，不是持久化承诺。将来若真实用例需要本地投影，必须由同步协议和隐私目标推动新 ADR/Migration，而不是把当前 struct 直接映射成表。

### 3.6 为什么不引入 chain

一条规则无法提供关于以下问题的证据：

- 第二条规则是否使用同一输入；
- 顺序是否有业务含义；
- 哪些结果短路；
- unknown 是否停止还是路由；
- trace 应记录什么；
- rule 是否有副作用；
- policy revision 是否共享。

此时创建 `Rule.Evaluate(map[string]any)` 只是在预支未来复杂度。第 26 节先出现第二个具体前置判断，再从真实重复中抽取最小线性协议，能让抽象由消费者而不是课程标题决定。

## 4. 从第一性维度推导需求

### 4.1 事实源：shape 正确不等于 authority 正确

```text
事实：注册生命周期由外部 Account 能力拥有。
风险：浏览器、本地表或任意 adapter 可以提供格式正确但不权威的 registered-at。
需求：Participation 只通过 consumer-owned trusted port 获取最小事实；客户端没有 verdict/time 参数。
机制：RegistrationFactReader + RegistrationFactSnapshot；composition 负责选择 adapter。
证据：service signature、domain/application import test、无 API/adapter negative diff。
```

constructor 能拒绝零值、倒置时间和非法 metadata，却不能证明 source 组织身份。这个最后一跳必须由装配、credential、transport 和 adapter contract 解决，不能要求 value object 完成密码学认证。

### 4.2 故障域：未知不能获得“确定不符合”的名字

```text
事实：provider miss、stale、timeout 和 corrupt 都没有足够事实判断 cutoff。
风险：全部返回 false 会把依赖故障计入业务拒绝，并阻止正确重试/告警。
需求：confirmed decision 与 inability-to-decide 结构性分离。
机制：NewUserEligibilityDecision + nil error，或 zero decision + typed error。
证据：failure table tests 逐项断言 zero decision。
```

“fail-closed”在这里仅意味着不能继续选择，不意味着一律返回 ineligible。否则安全姿态虽然保守，业务语义仍然是谎言。

### 4.3 权限：资格不是访问控制

```text
事实：知道 participant reference 不证明 caller 就是该主体。
风险：把 eligibility service 接到 demo ID 会产生 IDOR/代他人判断或参与。
需求：在真实主体绑定前不开放用户相关入口；将来服务端仍需先认证/授权再求资格。
机制：ParticipantRef 命名停止线；本节无 handler；后续公共访问控制独立演进。
证据：无 cmd/http/web diff；package docs 明确非认证。
```

前端未来可以根据已授权 projection 隐藏或禁用操作，但真正的资格和权限都必须由服务端强制。两者也不能共用一个 `allowed bool` 后丢失原因与责任边界。

### 4.4 发布：无外部 effect 是当前最安全的 rollout

```text
事实：本节只有新 package，没有 schema、route 或 composition 变更。
风险：未成熟资格规则若直接接入会意外拒绝真实流量。
需求：先让 code/test/ADR 合并而不改变现有运行行为。
机制：internal domain/application slice；零 runtime diff。
证据：基准到实现提交的 path-scoped negative diff。
```

这种 rollout 的代价是存在暂时无 production caller 的代码。它应通过明确后续消费者和章节停止线管理，不能无限期堆积“将来会用”的框架。

### 4.5 可逆性：撤销不应需要数据修复

```text
事实：当前无表、无 key、无 route、无配置。
风险：模型不合适时若已有持久数据/公开契约，回滚代价会迅速升高。
需求：第一次规则建模保持 code-only 可撤销。
机制：value/service 均在独立 Participation package；不修改 Lottery。
证据：negative diff + package boundaries。
```

未来一旦公开 rule/reason 或持久化 decision，这些字符串就变成兼容性资产，必须版本化而不是原地改义。

### 4.6 可观测性：能追溯不等于所有字段都适合做 label

```text
事实：policy/fact revision、source、evaluated-at 有诊断价值，也可能高基数或敏感。
风险：全部成为 metrics label 会增加成本、泄露内部结构或重识别主体。
需求：决定保留最小 provenance，但普通指标只用审核过的 outcome/rule/reason。
机制：decision 不含 ParticipantRef/原始时间阈值/PII；文档规定字段用途。
证据：decision shape 与 product contract review。
```

本节没有生产 observer，因此只能证明“输出形状允许正确观测”，不能声称 metrics、trace 或告警已经落地。

### 4.7 学习成本：具体名字优先于可复用名字

```text
事实：当前只有 registration cutoff 一条规则。
风险：generic Rule、context map、priority 和 DSL 会让读者先理解框架再理解业务。
需求：每个类型直接表达事实、政策和决定。
机制：NewUserPolicy、RegistrationFactSnapshot、NewUserEligibilityDecision。
证据：AST forbidden-type test 与无跨上下文 import。
```

这不是反对抽象，而是要求抽象的每个方法和字段都有至少两个真实消费者或清晰的变化证据。

## 5. 备选方案与权衡矩阵

### 5.1 总体比较

| 方案 | 事实可信度 | 失败语义 | 当前可验证性 | 外部契约成本 | 演进风险 | 当前结论 |
| --- | --- | --- | --- | --- | --- | --- |
| 浏览器提交 `is_new_user` / registered-at | 极低 | 通常只有 bool | 容易做假 E2E | 立即锁定不可信 DTO | 直接可绕过 | 拒绝 |
| demo user header 包装现有 route | 低，无身份绑定 | 可做 status 但语义虚假 | UI/Compose 容易展示 | 提前锁定 route/status | 制造安全幻觉 | 不采用 |
| 本地 MySQL registration fact 表 | 中，取决于同步协议 | 可分类 | 可做 SQL/E2E | schema/grant/data 生命周期 | 第二事实源、隐私债务 | 当前拒绝 |
| 纯 `IsNewUser(time,time) bool` | 输入可信度由调用方决定 | 无 unknown | cutoff 易测试 | 很低 | 每个 caller 重复拼错 | 不充分 |
| 通用 Rule/chain/engine | 未改善 | 看框架设计 | 一条规则无法证伪 | 抽象/API 面大 | 过早统一不同阶段 | 拒绝 |
| OPA/XACML/外部 policy engine | 需另建属性与身份链 | 可以很丰富 | 当前基础不足 | 运维、发布、语言、审计大 | 把业务资格误当统一授权 | 推迟 |
| 具体 Participation domain/application slice | adapter 尚待实现，但端口可信边界明确 | confirmed 与 unknown 分开 | 领域、应用、race 可精确证伪 | 当前无公开外部成本 | 暂无端到端消费者 | 采用 |

### 5.2 为什么是 `Decision + error`，不是 bool 或四态 enum

bool 无法表达 unknown；把 `unavailable/stale/invalid` 全部放进业务 outcome，又会让调用者把系统失败当正常分支。当前选择：

- `decision, nil`：事实充分，得到 confirmed eligible/ineligible；
- `zero decision, error`：无法安全形成业务决定；
- cancellation 保留 context identity；
- provider errors 归到窄 application class。

通用四态 PolicyDecision 可能在未来有价值，但现在会诱导 Authorization、Eligibility、Routing 和 Inventory 共用一个语义不清的枚举。具体 decision 让错误边界先稳定。

代价是 Go 调用者必须遵守“error != nil 时忽略 decision”的常见契约；测试用零 decision 强制当前实现不返回部分决定。若未来需要收集逐规则 trace，应另建明确 execution result，而不是悄悄开始返回 decision + error 双有效值。

### 5.3 为什么 freshness 在 application，不在纯 rule

registration cutoff 是 Participation 业务政策；fact freshness 依赖 provider 的 observed-at 语义、读取路径和可接受陈旧窗口。把 max age硬编码进 domain rule 会把来源可靠性契约与“什么叫新用户”混为一谈。

当前 service 注入 `maxFactAge`，domain evaluator只接收已验证快照。未来若 max age 真正按 Activity 或 rule revision 变化，它可能需要升级为可追溯 policy input；当前设计没有阻止这一演进，但也没有假装已经决定。

### 5.4 为什么 policy 每次显式传入

把 cutoff 放进全局 config 很方便，却会暗示所有 Activity 共用一个“新用户”口径；放进 Strategy 更会让 Lottery 获得 Participation policy 所有权。当前显式 per-call policy 保留未来 Activity/规则集拥有它的可能。

代价是当前 service 不能单独从外部 request 构造完整 use case；这正是没有 Activity 时应暴露的缺口，而不是用全局 magic value 填平。

### 5.5 为什么 ParticipantRef 不是 `UserID`

`UserID` 容易被误读为本系统用户 aggregate 或认证 Principal。`ParticipantRef` 明确它只是查询参考：即使值正确，也不能证明 caller 拥有该身份、属于哪个 tenant 或有执行哪项 action 的权限。

使用 `uint64` 保持当前项目 identity 风格和 cutoff slice 简洁，但未来外部 provider 若使用 UUID、复合 tenant key 或多 authority namespace，必须重新评估结构，不能靠 hash 截断或无碰撞证明地塞进 uint64。

## 6. 不变量、统一语言与信任边界

### 6.1 长期不变量

任何后续实现都必须保留：

1. 客户端不能决定自己是否新用户；
2. ParticipantRef 不是认证或授权证明；
3. 外部目录拥有注册事实，Participation 拥有资格决定；
4. `registered_at >= cutoff` 是当前 rule code 的含边界语义；
5. policy revision、fact revision、应用版本和 schema version 相互独立；
6. 相同 policy/fact/evaluated-at 产生相同 decision；
7. 时间按 instant 比较，不按本地日期字符串比较；
8. unknown/stale/unavailable/invalid 不得形成 ineligible；
9. eligible 不等于已授权、已参与、已中奖或已发奖；
10. 资格失败时终端 Lottery 选择不应开始；
11. decision 不携带不必要 PII 或完整 fact payload；
12. 用户资格与决定不能进入 Strategy Redis projection；
13. 正式消费之前必须另行解决身份绑定、Activity scope、幂等与 TOCTOU；
14. 第二条真实规则出现前，不把当前 evaluator 泛化成万能 engine。

### 6.2 信任转换图

```text
external account authority
  -- authenticated/authorized adapter contract (future) -->
RegistrationFactReader
  -- shape + subject + time + freshness validation -->
trusted evaluation input
  -- concrete versioned policy -->
confirmed Participation decision
  -- future authenticated orchestration -->
allowed to continue / business reject
```

当前代码只实现中间两段。第一段具体 adapter 与最后一段业务编排仍缺失，因此不能把整个图画成已闭环。

### 6.3 三种版本不可合并

| 版本 | 回答问题 | 当前字段 | 不能代替 |
| --- | --- | --- | --- |
| PolicyRevision | 使用哪版“注册时间下界”政策 | decision/policy | 事实快照版本、Git SHA |
| FactRevision | provider 的哪份快照 | fact/decision | 政策版本、单调数据库 version |
| RuleCode | 哪个稳定规则语义 | decision | 每次配置修订 |

若原地改变 rule code 的语义，即使 policy revision 变化，历史解释仍会混乱。不同口径如“首次下单后 N 天”应成为新 rule 或清楚的新 policy schema，不能沿用当前 code 偷换定义。

### 6.4 metadata 校验边界

source/revision 当前要求有效 UTF-8、无首尾空白、可打印且字节有界。这能拒绝控制字符、格式字符、空值和超长输入，却不保证：

- ASCII-only；
- Unicode homoglyph 不混淆；
- revision 单调或唯一；
- source 确实被组织授权；
- 不包含业务标识或可重识别信息。

因此 adapter 还需要自己的枚举/allowlist 和 privacy contract。domain shape validation 不是 authority validation。

## 7. 资源所有权、并发与时间预算

### 7.1 谁创建、谁接管、谁关闭

| 对象 | 创建者 | 生命周期 | 谁关闭 |
| --- | --- | --- | --- |
| policy/fact/decision value | 调用者/adapter/evaluator | 单次调用或只读共享 | 无资源 |
| `NewUserEligibilityService` | future composition root | 进程级 immutable | 无 `Close` |
| `RegistrationFactReader` adapter | future composition root | 取决于 client/pool | composition root |
| Clock | composition root | 通常进程级 | 无资源 |
| request context | 上游 use case/transport | 单次调用 | 上游 cancel |

service 不应关闭注入 reader，因为它不拥有 reader 的连接池或共享 client。未来 adapter 若创建 goroutine/pool，构造失败、正常 shutdown 和部分启动失败都必须由 composition root 精确接管。

### 7.2 并发模型

service 字段构造后不修改，因此本身可并发读。并发正确性仍依赖：

- reader 的 client/pool 并发安全；
- Clock 并发安全；
- policy/fact values 不暴露可变 slice/map；
- 每个调用自己的 context 不被跨请求复用。

当前没有 singleflight。用户相关 fact 即使 key 相同，也可能需要 caller scope、trace、credential 和撤销语义；在没有负载证据前合并请求会引入跨 caller context 风险。每次调用独立读取是更易解释的第一版。

### 7.3 一次受控时钟的顺序

application 在事实读取成功后调用 `Clock.Now()` 一次：

1. 读取期间经过的时间计入 fact age；
2. cutoff/future/freshness 使用同一个 instant；
3. 测试可以精确控制边界；
4. browser time 无入口；
5. 不同系统时区只改变表示，不改变结果。

若先捕获 now 再执行慢读，一个读了很久的 fact 可能按旧 now 被判 fresh；若在每个检查中分别 `time.Now()`，边界请求可能自相矛盾。

### 7.4 三种时间不能混淆

```text
registered_at <= observed_at <= evaluated_at
evaluated_at - observed_at <= max_fact_age
registered_at >= policy_cutoff  -> eligible
```

- registered-at：账户事实发生时刻；
- observed-at：provider 捕获/确认该快照的时刻；
- evaluated-at：本服务实际形成决定的时刻。

数据库通用 `updated_at` 未必等于 observed-at；若 adapter 直接拿一个不具备该语义的列填充，freshness 计算即使数学正确，业务仍然错误。

### 7.5 I/O timeout 与 fact freshness 不是同一个预算

`maxFactAge` 限制数据陈旧程度，不限制 reader 单次耗时。当前 service 不创建 timeout，只传播 caller context。这保留上层总预算所有权，但留下两个明确责任：

- future adapter 必须遵守 context，并设置不超过外层 budget 的 transport/query timeout；
- future use case 必须为事实读取、其他规则与 Lottery 选择分配有序预算。

给 `maxFactAge` 取 15 分钟不能阻止 provider 卡 15 分钟；给 HTTP timeout 取 2 秒也不能说明允许 2 秒陈旧。两个旋钮必须分开命名和验证。

### 7.6 取消的可实现边界

service 在 dependency 和 clock 边界后重新检查 context，让已经可观察的 caller cancellation 优先于 provider error 或刚形成的 decision。但 Go 无法强制终止一个忽略 context、永不返回的同步方法。当前 blocking test 只有在 stub 释放后才能观察取消。

因此未来 adapter contract 必须要求：

- I/O 原语接受并遵守 context；
- 内部 retry 受同一总 deadline；
- goroutine 不因 caller cancel 泄漏；
- provider 自身 timeout 与 caller timeout可区分。

## 8. 失败模型与恢复语义

### 8.1 请求级失败矩阵

| 场景 | 当前语义 | 是否可自动重试 | 恢复责任 |
| --- | --- | --- | --- |
| confirmed ineligible | 正常业务拒绝 | 否，除非事实/政策真实变化 | 产品/用户状态 |
| fact not found | 无法决定 | 取决于 provider contract，当前不承诺 | fact provider/数据修复 |
| fact stale | 无法决定 | 可重新读取，但 service 本身不 retry | provider/adapter |
| provider unavailable | 无法决定 | 上层按幂等与总预算决定 | provider/运维 |
| unknown provider error | 永久或未知失败 | 不应盲目 retry | adapter 分类/开发 |
| wrong-subject fact | 数据契约破坏 | 否，先隔离/告警 | adapter/provider |
| future fact | 时钟或数据契约破坏 | 否，先校时/修复 | platform/provider |
| invalid policy | programmer/config error | 否 | Activity/policy owner |
| invalid clock | platform wiring error | 否 | composition/platform |
| caller canceled/deadline | 原始 context failure | 由 caller 决定新请求 | caller |

当前 service 没有自动 retry。事实读取虽然表面是 read-only，但隐式 retry 会增加尾延迟、放大 provider 负载，并可能跨过 policy/fact freshness 边界。未来如需 retry，必须每次仍受同一 outer budget，并说明重复读取可能拿到不同 revision 时怎样处理。

### 8.2 部分成功

“fact 已读成功，但随后发现 stale/subject mismatch/clock invalid”不是业务部分成功，不返回半个 decision。相同地，“evaluator 已计算，但此刻观察到 caller canceled”仍返回 context error和零 decision。

这条规则避免调用者同时看到 decision 与 error 后自行猜优先级。未来若审计需要记录被取消前发生的中间步骤，应使用内部 trace，而不是改变 public return invariant。

### 8.3 多 class error 的剩余风险

`RegistrationFactReadError` 让普通 `Error()` 只展示稳定 class，同时 `Unwrap()` 保留 trusted cause。这便于诊断，但存在一个未关闭的组合风险：如果 adapter 把另一个 application sentinel 放在 cause 中，标准 `errors.Is` 可能对 wrapper class 和 cause class 同时为真。

例如理论上：

```text
outer class: unavailable
cause chain: not found
```

调用者按不同检查顺序可能得到不同分类；当前 classifier 本身也使用 `errors.Is` 顺序匹配。现有测试覆盖 opaque cause、raw deadline 与单一 class，没有覆盖恶意/违规的多 class 链。

真实 adapter 之前必须确定单 class invariant：要么 adapter cause 禁止携带 application sentinels并做 contract test，要么使用不参与标准 unwrap 的受信诊断字段。错误脱敏与错误分类唯一性是两件不同的事，不能因字符串安全就宣布分类也安全。

### 8.4 provider not found 为什么不是新用户

“目录里查不到”可能表示：

- reference 错误；
- 数据同步延迟；
- 账号已删除；
- provider 分片故障；
- caller 越权后被 provider 隐藏；
- 真实不存在。

在 provider 没有更强、可审计的语义前，not found 只表示无法确认。自动把它当“从未注册，所以是新用户”会给数据故障一条 eligibility fail-open 路径。

### 8.5 rollback 与恢复

本节无 schema、数据、route 和 config，代码回滚不需要 down migration、cache flush 或数据补偿。恢复主要是：

- policy/fact adapter未来错误时停止装配 consumer；
- 保留已知正确的 Lottery ephemeral 路径，不让未成熟 slice 改变流量；
- 修复 provider/clock 后重新求值，不复用错误决定。

一旦未来持久化 decision 或创建 Participation，恢复语义会改变：必须知道是否已经消费、是否可重放以及事实/policy revision；不能沿用本节“重新调用即可”的简单模型。

## 9. 安全、隐私与最小权限推导

### 9.1 客户端威胁

攻击者可以修改 request body、header、隐藏字段、本地状态和客户端时间。因此本节生产接口完全没有：

- `is_new_user`；
- caller-supplied registered-at；
- caller-supplied evaluated-at；
- caller-supplied final reason/outcome。

将来请求即使携带 participant reference，服务端也必须先从已认证 principal 和资源 scope 推导/校验该 reference，再读取权威事实。

### 9.2 adapter 威胁

受信 adapter 仍可能有 bug 或受到错误配置影响：返回其他主体事实、陈旧 snapshot、future time、空 revision、错误 source 或敏感 cause。application 通过 subject/time/freshness/shape 校验减少损害，但无法自行验证 transport credential 或外部 source 真实性。

未来 adapter至少需要：

- 服务到服务认证与最小读取 scope；
- TLS/网络边界；
- 明确字段 mapping 与 source allowlist；
- timeout/cancellation；
- PII 最小化与日志脱敏；
- provider contract tests；
- key rotation 与访问审计。

### 9.3 decision 最小化

decision 故意不保存 ParticipantRef、registered-at、cutoff 或完整 payload。即使如此，fact revision、source 和 evaluated-at 组合仍可能具有关联性，不能无条件进入普通日志、analytics 或前端 DTO。

推荐未来观测分层：

| 数据 | 普通 metrics label | 受控 trace/log | 对外响应 |
| --- | ---: | ---: | ---: |
| outcome/rule/reviewed reason | 可以 | 可以 | 需产品/安全评审 |
| policy revision | 否 | 可以 | 通常否 |
| fact source/revision | 否 | 必要时 | 否 |
| evaluated-at | 否 | 必要时 | 通常否 |
| ParticipantRef/registered-at | 否 | 最小化、脱敏 | 否 |

### 9.4 缓存为什么不是默认优化

资格事实与决定包含主体、高时效和撤销含义。缓存它们前必须回答：

- key 是否包含 tenant/subject/policy/source revision；
- 谁能读该 key；
- 撤销最大延迟；
- stale 是否允许；
- negative/unknown 是否缓存；
- 决定是否跨 Activity 复用；
- 删除请求如何传播；
- cache down 是 fail-open 还是回源。

第 24 节 Strategy cache 的 5 分钟 TTL、ACL、projection schema 和 fail-open 不能直接复用。本节选择每次走 reader，直到真实 provider 性能和撤销需求出现。

### 9.5 与权限系统的边界

未来访问控制可以决定“某 principal 能否代表某 ParticipantRef 发起 participation”，资格服务再决定“该主体是否满足活动业务条件”。顺序通常是：

```text
authenticate -> authorize resource/action -> evaluate eligibility -> consume participation
```

前端导航裁剪只是授权 projection 的 UX 表达，不能替代服务端检查；同样，资格 badge 不能替代服务端事实读取。公共权限模型应在路线中按独立章节演进，不能因为本节第一次出现 user reference 就顺手塞进 Participation。

## 10. 证据设计：每个测试试图证伪什么

### 10.1 构造器测试

试图证伪：零 reference、零/倒置时间、空/非规范 metadata、超长 token 或 zero policy 能否混入领域。它证明已列输入被拒绝，不证明任意 Unicode homoglyph、安全 authority 或 provider provenance。

### 10.2 cutoff 与时区测试

试图证伪：`>`/`>=` 写反、纳秒边界丢失、time zone 表示影响决定、重复执行漂移。普通 `go test` 运行 fuzz seed，但不等于长期 fuzz campaign。

### 10.3 freshness 测试

试图证伪：exact max age 被误判 stale、stale fact 先形成业务拒绝、future observed-at 利用负 duration 绕过。它不证明 provider 的 observed-at 字段语义正确。

### 10.4 error 与取消测试

试图证伪：not found/unavailable/unknown/cause 泄露被混为一类、provider deadline冒充 caller deadline、取消后仍返回 decision。它没有覆盖真实网络、数据库 driver、retry 或多 class sentinel cause。

### 10.5 并发与 race

64 worker 与 `go test -race` 试图证伪 service 的共享可变状态和 test double data race。它不证明未来 adapter、连接池、外部服务或高并发容量。

### 10.6 AST 架构测试

试图证伪：domain/application 导入错误上下文，或在唯一规则阶段提前声明一组已知万能类型。它是结构护栏，不是语义证明；换名、函数式万能 map 或 code generation 仍需 review。

### 10.7 negative diff

基准到实现提交对 cmd/deploy/migrations/web/Lottery/infrastructure/platform/config/dependency 文件无差异，试图证伪“为了演示悄悄改了运行入口”。它既是范围正确性的证据，也是没有 E2E 的直接证据。

### 10.8 全仓 Go 与文档检查

全仓 Go 测试证明当前已存在的 Go suite 没有回归；doc-check 证明链接、ADR 登记和课程 registry 结构。它们不能替代 Web、Compose、真实 provider、浏览器和语义评审。本文形成时最终 `make verify` 仍待父级在所有文档/索引合并后的 tip 执行。

## 11. 被刻意推迟的能力与重评条件

| 推迟项 | 现在不做的原因 | 明确触发器 |
| --- | --- | --- |
| 第二条 Participation 规则 | 本节只需新用户 cutoff | 下一节出现真实第二前置判断 |
| 最小责任链 | 无顺序/短路消费者证据 | 两个具体 evaluator 的共同协议被观察到 |
| 规则树/决策引擎 | 无分支、路由、动态配置需求 | 真实分支与持久规则发布需求 |
| 外部目录 adapter | provider/credential/schema 未给出 | 明确 authoritative API 与运行环境 |
| 本地 fact projection | 无 ingestion/correction/deletion/privacy | 远程读无法满足已测目标，且同步协议完成 |
| fact/decision cache | 无性能与撤销证据 | provider load/latency 达到阈值并完成 threat review |
| Activity policy binding | Activity 尚不存在 | Activity aggregate/发布模型落地 |
| 公开 HTTP mapping | 无可信主体和真实 consumer | session + resource/action authorization + use case |
| React eligibility UI | 无受权服务端 projection | 后端契约稳定且服务端强制成立 |
| decision persistence/audit | 无正式 Participation/Draw | 正式参与需要回放、合规或争议处理 |
| quota reservation/TOCTOU | 当前规则只读 | 参与次数或权益消费进入关键路径 |
| OPA/XACML/DSL | 一条业务规则不值得平台成本 | 多团队、多语言、动态 policy 治理证据 |
| 多租户 subject reference | 当前无 tenant model | 第一条 tenant-scoped use case |
| error cause channel hardening | 真实 adapter 尚未存在 | adapter/HTTP mapping 前必须关闭多 class 风险 |

推迟项都绑定事实触发器。它们不是“后续优化”占位符，也不能因为 Docker Desktop 已安装 MySQL/Redis/RabbitMQ/PostgreSQL 就寻找使用理由。

## 12. 架构师会主动检查、但需求未直接点名的点

### 12.1 policy provenance

PolicyRevision 说明“哪一版”，却不说明谁发布、何时生效、作用于哪场 Activity。当前 per-call policy 保留接口，但正式 use case 必须让 Activity/规则发布拥有 provenance，防止调用者随意构造一个更宽 cutoff。

### 12.2 max fact age provenance

`maxFactAge` 当前只是 service constructor 参数，decision 不记录它。若将来需要审计“为何当时相信这份事实仍新鲜”，必须记录 source contract/policy revision，或让 freshness policy 本身版本化。否则同一 fact/policy 在配置变化后可能无法完整回放。

### 12.3 source/revision 字符集

可打印 Unicode 仍可能产生视觉同形、规范化差异或 dashboard 混淆。真实 adapter 更适合将 source 收紧到 code allowlist，将 revision 视为 opaque bytes/string但禁止 PII；如需跨系统比较，还要定义 Unicode normalization 或 ASCII contract。

### 12.4 subject namespace

单一 uint64 未表达 authority/tenant。多用户目录出现时，`42` 可能在不同 source 重复。重评时可能需要 `{authority, tenant, subject}` structured reference，而不是扩大整数或把 namespace 拼进未经解析字符串。

### 12.5 迟到与更正

当前 snapshot 没有 valid-from/to、correction relation 或 monotonic revision contract。注册时间若被更正，之前 eligibility decision 是否仍有效需要正式 Participation policy。实时读取也不能自动解决历史回放。

### 12.6 决定和消费的时间差

资格 service 是 read-only。eligible 返回后，事实、policy、额度或风险可能变化。对一次正式 Participation，可能需要：

- 同事务内重检本地权威事实；
- 按 revision 条件创建 participation；
- 短期 lease；
- decision snapshot + policy version；
- quota reservation。

本节没有副作用，不能用“决定不可变”误称流程原子。

### 12.7 provider 防枚举与 privacy

未来 fact reader 若对任意 ParticipantRef 暴露 not-found 差异，可能成为账号枚举渠道。authorization 应先于读取；公开错误不应暴露 source 是否存在该主体。内部 typed error仍可用于运维，但 transport mapping 要做 threat review。

### 12.8 观测容量

outcome/rule/reason 是低基数候选；policy revision、source、fact revision 和 evaluated-at 可能高基数。metrics 与 trace backend 的 retention、sampling、访问控制和删除流程必须在生产 adapter 前定义。

### 12.9 依赖降级

provider outage 时 fail-closed 保护资格，却可能让所有参与不可用。是否允许使用有界 stale projection 是产品 availability 与安全共同决策，需要真实 outage/latency 数据；不能在 incident 中临时把 unavailable 改成 eligible。

### 12.10 供应链与接口兼容

本节没有新增第三方依赖，降低供应链面。但 exported application/domain API 一旦被多个包使用就会形成内部兼容成本。下一节抽取 chain 时应以适配方式演进，避免为了框架美观同时重写当前 concrete semantics 和 tests。

### 12.11 dead-code drift

暂时无 runtime consumer 的 slice 容易与未来真实需求漂移。控制方法不是现在接假 API，而是：下一节只在真实第二规则基础上演进；到首次 production composition 时重新验证 provider、Activity、session 和 error mapping，不把本节 test 当永久充分证据。

### 12.12 发布顺序

未来完整闭环不能一口气上线。至少应分为：provider contract/adapter → server-side authenticated orchestration → audit/metrics → frontend projection → adversarial E2E。任何 frontend 提前依赖未强制的资格结果，都会制造可绕过窗口。

## 13. 假设与风险账本

状态含义：

- **已验证于当前切片**：有代码/测试证据，但不外推到真实 adapter；
- **部分验证**：机制或 shape 已证，环境/规模未知；
- **未验证**：仍是设计假设；
- **接受并推迟**：风险已知，当前扩张成本高于收益。

| ID | 假设/风险 | 当前证据 | 失效影响 | 观察信号与动作 | 状态 |
| --- | --- | --- | --- | --- | --- |
| A25-01 | 注册时间下界就是首个“新用户”口径 | product baseline + concrete policy | 产品实际按首单/入组会语义错误 | 产品口径变化时新 rule/policy schema，不改旧 code 含义 | 部分验证 |
| A25-02 | 外部目录是唯一注册事实 authority | ADR 与边界分析 | 多 authority/账号合并无法解释 | 第一条 provider 集成前确认 owner、correction、deletion | 未验证 |
| A25-03 | uint64 ParticipantRef 足够 | 当前项目 identity 风格与 tests | UUID/tenant/provider collision | 多 authority/tenant 出现时改 structured reference | 未验证 |
| A25-04 | observed-at 语义可靠 | domain 明确定义和边界 test | stale 判断失真 | adapter mapping contract + provider field provenance | 未验证 |
| A25-05 | source revision 可追溯 | 非空/有界 shape test | 无法回放或判断更正 | provider 定义 uniqueness/order；必要时结构化 | 部分验证 |
| A25-06 | exact max age 可作为全局 source contract | application边界 test | 不同 Activity 对陈旧容忍不同 | Activity 出现时决定归属并版本化 | 未验证 |
| A25-07 | max age 只需正数即可 | constructor 仅 `>0` | 误配超大值长期接受旧事实 | 配置装配前设置合理上限和环境校验 | 已知风险 |
| A25-08 | 单次 server clock 足够一致 | order/timezone/determinism tests | 跨节点时钟漂移产生不同决定 | NTP/clock health；需全局顺序时引入 source time contract | 部分验证 |
| A25-09 | provider 遵守 context | interface 传入 ctx，blocking test需人工 release | goroutine/request 卡死 | adapter timeout/cancel fault test | 未验证 |
| A25-10 | 每次实时读取成本可接受 | 无 runtime adapter | 延迟/可用性限制参与 | 集成后测 latency/error/load，再评估投影/cache | 未验证 |
| A25-11 | decision 无需持久化 | 当前无正式 Participation | 争议/审计无法回放 | 正式 Draw/Participation 前决定 snapshot/audit | 接受并推迟 |
| A25-12 | policy + fact + evaluated-at 足够重放 | pure evaluator deterministic | freshness max age/provenance 缺失 | 审计需求出现时记录 execution policy/source contract | 部分验证 |
| A25-13 | error semantic class 始终唯一 | single-class tests | HTTP/retry/metric 分类歧义 | adapter 前添加 multi-class negative test或改 cause channel | 未验证 |
| A25-14 | Error() 脱敏足够防日志泄露 | secret cause string test | structured logger/trace 仍可能展开 cause | 真实 logger adapter test + field allowlist | 部分验证 |
| A25-15 | printable metadata 足够规范 | control/format/trim/size tests | homoglyph/normalization 运维混淆 | adapter source allowlist、revision字符规范 | 部分验证 |
| A25-16 | decision 字段是最小必要集 | 不含 ref/time threshold/PII | revision/source仍可关联 | privacy review、retention/access control | 部分验证 |
| A25-17 | 64-worker race代表 service并发安全 | race + immutable service | real adapter data race/容量故障 | adapter race/integration/load test | 部分验证 |
| A25-18 | AST stop-line能阻止过早框架 | forbidden type/import test | 换名/函数 map 可绕过 | semantic review；第二规则只抽真实重复 | 部分验证 |
| A25-19 | 不接 API 比 demo E2E更诚实 | negative diff | 内部 slice暂时无真实消费者 | 首次组合前重验全部前提，不长期堆 dead code | 接受并推迟 |
| A25-20 | 不持久化避免第二事实源 | 无 Migration/adapter | 每次读取依赖远端可用 | 有实测需求后设计受控 projection | 已验证于当前范围 |
| A25-21 | eligible 后事实不会在消费前变化 | 当前无消费，未处理 | TOCTOU/超额/撤销绕过 | 正式参与时 revision check/reservation/transaction | 未验证 |
| A25-22 | not found 应保守 unknown | 当前 provider contract未知 | 过度拒绝或误重试 | provider 明确强语义后才细分 | 接受并推迟 |
| A25-23 | 不需要缓存用户资格 | 无性能数据；安全停止线 | provider成本或 outage放大 | 集成后用 load/outage evidence重开设计 | 未验证 |
| A25-24 | policy caller可信 | 当前无 production caller | 任意 cutoff放宽资格 | Activity/policy repository + authorization before composition | 未验证 |
| A25-25 | 当前 rule/reason可长期稳定 | concrete tests/ADR | 改义破坏历史解释 | 新语义使用新 code/revision并做兼容计划 | 部分验证 |

### 13.1 当前最高优先级风险

按“发生概率 × 影响 × 即将到来的触发器”排序，最先要关闭的不是性能：

1. **多 class error 与真实 adapter contract**：否则 transport/retry 可能错误分类；
2. **第二条具体规则的事实与短路语义**：否则无法诚实抽 chain；
3. **未来主体绑定与 policy provenance**：否则内部 slice不能安全接外部 use case；
4. **正式参与的 TOCTOU/审计**：在出现写入/额度消费时立即变成高风险。

## 14. 未来演进问题与重新决策触发器

### 14.1 下一条具体规则必须回答

在抽取任何责任链前，第二条 Participation 规则至少要说明：

- 它读取同一个 fact snapshot 还是另一 authority；
- 是否只读；
- eligible/ineligible/unknown 是否同构；
- 顺序是成本优化、隐私最小化还是业务正确性；
- 第一条 reject 后是否绝不读取第二依赖；
- cancellation、timeout 与 error 怎样传播；
- trace 是仅记录已执行节点还是全部 planned 节点；
- policy revision 是每节点独立还是规则集版本。

只有这些事实出现，最小 chain 接口才有依据。当前 `NewUserEligibilityService` 不应为了迁就猜测中的 chain 而先改成 generic context。

### 14.2 Activity 出现时必须回答

- 谁创建/发布 cutoff；
- policy revision 是否 immutable；
- 生效/失效时间与 evaluated-at 如何比较；
- 草稿、审批、回滚和旧 participation 用哪版 policy；
- Strategy routing 与 eligibility 谁先谁后；
- tenant/scope 如何隔离；
- operator 需要什么权限和审计。

Activity 是 policy provenance 的自然候选，但不能因此让 Activity 拥有外部注册事实或 Lottery selector。

### 14.3 真实会话与权限出现时必须回答

- session principal 怎样映射 ParticipantRef；
- 是否允许代理/运营代用户操作；
- action/resource/scope 怎样服务端授权；
- not-found 是否因防枚举统一对外；
- frontend 只拿到哪些 projection；
- API 直接调用怎样证明不能绕过 UI；
- 权限撤销与资格事实刷新分别有什么时效。

访问控制应按路线独立形成公共模型、会话、服务端强制、前端感知与越权验收，而不是在本节用 role string 补丁解决。

### 14.4 事实投影触发器

只有同时具备以下证据，才值得讨论本地 projection/cache：

- provider p95/p99/error 或成本已测量；
- acceptable stale/revocation window由产品与安全确认；
- ingestion、idempotency、correction、delete 和 replay 协议；
- source revision 冲突规则；
- PII retention/encryption/access audit；
- cold start/outage 行为；
- authoritative fallback 与 rebuild 定义。

否则使用已安装 MySQL/Redis只会把基础设施可用性误当架构需求。

### 14.5 正式 Participation/Draw 触发器

一旦 eligibility 与额度消费、随机选择或 Benefit 发生写操作，就必须重新决定：

- idempotency key 的业务 identity；
- decision/policy/fact snapshot 是否持久；
- quota reservation 和 selector 的事务/消息边界；
- retry 是否重用同一次 Draw；
- eligible 后事实撤销的处理；
- partial success 与 compensation；
- audit 与隐私删除冲突。

本节 read-only service 不提供这些答案，也不能被包装成“已有 eligibility，所以正式抽奖只差接一下”。

## 15. 可追溯证据与本节结论

### 15.1 需求与决策追踪

- 上游规则边界：[Lottery 业务规则需求基线 v1](../../product/lottery-rule-requirements-v1.md)
- 本节规则基线：[新用户资格规则基线 v1](../../product/new-user-eligibility-v1.md)
- 本节规范决策：[ADR-0021](../../decisions/ADR-0021-participation-new-user-eligibility.md)
- 上游事实所有权决策：[ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- 上一节缓存停止线：[第 24 节设计手记](lesson-24.md)
- 实际测试与剩余风险：[第 25 节 QA](../../qa/lessons/lesson-25.md)

### 15.2 代码与测试追踪

| 设计命题 | 实现/证据入口 |
| --- | --- |
| opaque participant lookup reference | [registration_fact.go](../../../internal/participation/domain/registration_fact.go) |
| minimum authoritative fact shape | [registration fact tests](../../../internal/participation/domain/registration_fact_test.go) |
| concrete inclusive policy | [new_user_policy.go](../../../internal/participation/domain/new_user_policy.go) |
| confirmed decision and pure evaluator | [new_user_eligibility.go](../../../internal/participation/domain/new_user_eligibility.go) |
| cutoff/timezone/determinism/future tests | [domain evaluator tests](../../../internal/participation/domain/new_user_eligibility_test.go) |
| consumer-owned fact and clock ports | [ports.go](../../../internal/participation/application/ports.go) |
| freshness/cancellation/application ordering | [application service](../../../internal/participation/application/new_user_eligibility.go) |
| error class/cause separation | [application errors](../../../internal/participation/application/errors.go) |
| failure/concurrency/race evidence | [application tests](../../../internal/participation/application/new_user_eligibility_test.go) |
| no generic rule / dependency drift | [architecture test](../../../internal/participation/application/architecture_test.go) |

### 15.3 外部资料

以下资料用于校准机制，不替代本地业务决策：

1. NIST, [Attribute Considerations for Access Control Systems](https://www.nist.gov/publications/attribute-considerations-access-control-systems)：用于理解 attribute authority、质量与时效；本节不是 ABAC 实现。
2. OASIS, [XACML 3.0 Core Specification](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-cos01-en.html)：用于校准 Deny 与 Indeterminate 不应混淆；本节不实现 XACML。
3. Go, [Frequently Asked Questions — Interfaces](https://go.dev/doc/faq#implements_interface)：用于消费者拥有窄接口的语言机制；不能证明 adapter 自动正确。
4. Go, [`context` package](https://pkg.go.dev/context)：用于 cancellation/deadline 传播语义；无法强制一个忽略 context 的同步 provider 返回。
5. Go, [`errors` package](https://pkg.go.dev/errors)：用于 `Is`/`Unwrap` error chain；也正因 chain traversal，当前保留多 semantic class 的风险记录。

### 15.4 最终结论

本节可以严谨地说：

> GrowthOS-Go 已把“新用户”从客户端可伪造的 bool 推进为 Participation-owned 的具体服务端资格模型：系统定义了面向权威外部注册事实的最小 consumer-owned port 契约；给定受控事实快照、带版本的含边界政策与一次 evaluation instant，可以确定复算 pure decision；application 在求值前验证主体、future time 与 freshness，并把 not found、stale、unavailable、invalid 和 cancellation 保留为无决定错误。具体 adapter 的权威性、历史事实与 freshness 参数的持久来源尚未验证，因此当前不声称支持历史重放。该 slice 没有修改 Lottery、Redis、HTTP、Web 或数据库，也没有提前发明通用规则框架。

本节仍然不能说：

- 已经认证或授权真实用户；
- 当前 Lottery route 已阻止不合资格用户；
- 已连接或复制权威用户事实；
- 已发布 Activity 级规则；
- 已解决错误链单 class 的所有违规输入；
- 已完成 chain/tree/engine；
- 已关闭 eligibility 到正式消费的 TOCTOU；
- 已有浏览器/Compose/真实 provider E2E；
- 已通过最终整仓 `make verify` 并冻结章节。

真正值得保留的第一性原则是：

> **先确认事实权威、决定所有者和未知语义，再编排规则；先让一个具体决定在正确边界内成立，再从第二个真实消费者中抽象复用。界面完整、框架通用或基础设施现成，都不能替代这两步。**
