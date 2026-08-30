# 新用户资格规则基线 v1

- **状态：** 已接受
- **形成章节：** 第 25 节
- **日期：** 2026-08-30
- **决策所有者：** Participation
- **原始事实所有者：** 外部用户目录 / Account 能力

## 1. 为什么现在需要一条具体规则

第 23 节已经把“新用户抽奖”拆成访问决策、Participation 资格、Lottery 选择和 Benefit 承诺等不同阶段，但当时没有足够事实去设计一个通用规则接口。第 25 节只解决其中第一条已经能精确定义的问题：

> 对一份来自权威用户目录的注册事实快照，如果平台注册时间不早于当前政策的注册时间下界，则该参与主体满足本条新用户资格；否则形成确定的业务拒绝。

这个规则的目的不是识别登录用户、管理权限、扣减次数或完成抽奖，而是首次把“业务资格成立”“业务资格不成立”和“系统无法形成可信决定”变成可执行、可测试且不会互相伪装的语言。

## 2. 当前时间切片

第 25 节开始前已经存在：

- Lottery Strategy/Award 聚合、MySQL Repository 和无偏加权选择；
- development/test 专用且没有用户身份的 ephemeral selection API；
- React 临时选择消费者；
- 仅缓存 Strategy 读取投影的 Redis cache-aside；
- 第 23 节规则事实所有权与失败分类基线。

当前仍不存在：

- 真实会话、Principal、授权或 RBAC；
- Activity、发布版本和活动级政策绑定；
- 外部用户目录 adapter、同步/CDC 协议或本地用户投影；
- Participation 请求、次数账户、Draw/Result 或幂等身份；
- 两条以上需要编排的具体 Participation 规则。

因此，本节交付一个可执行的 Participation domain/application slice，但不把可伪造的浏览器标识接到 Lottery 演示接口，也不以本地表冒充外部用户目录。

## 3. 统一语言

| 术语 | 本基线中的含义 | 不代表 |
| --- | --- | --- |
| ParticipantRef | Participation 用来向可信事实提供方查询主体的非零不透明引用 | 登录证明、Principal、租户或授权范围 |
| RegistrationFactSnapshot | 一次求值所使用的不可变注册事实值，带来源、来源版本和观察时刻 | 本地用户主表、长期审计记录或客户端 DTO |
| RegisteredAt | 权威来源确认的平台注册时刻 | 浏览器时间、首单时间或活动入组时间 |
| ObservedAt | 事实提供方声明该快照有效到的时刻 | 数据库 `updated_at` 的通用替身 |
| PolicyRevision | 新用户政策配置的稳定修订标识 | Git SHA、Migration version、Strategy version |
| EvaluatedAt | 应用服务在完成事实读取后由受控服务端时钟捕获的一次求值时刻 | 客户端时间或每个节点各自读取的 `now` |
| Eligible | 事实充分且注册时刻位于含边界的允许区间 | 已授权、已参与、已中奖或已发奖 |
| Ineligible | 事实充分且注册时刻早于政策下界 | 技术错误、资源不可用或 `no_reward` |

## 4. 政策定义

首个规则身份固定为：

```text
participation.new_user.registered_on_or_after
```

规则输入为一个具体 `NewUserPolicy` 和一个具体 `RegistrationFactSnapshot`：

```text
registered_at <  registered_at_or_after  -> ineligible
registered_at == registered_at_or_after  -> eligible
registered_at >  registered_at_or_after  -> eligible
```

下界采用 **inclusive** 语义。所有时刻按 instant 比较，进入模型时规范为 UTC；相同时刻的不同 time zone 表示不得改变决定。

本规则不把“新用户”解释为首单、首次登录、首次入组或最近 N 天注册。口径变化必须形成新的政策修订或新规则，而不是悄悄改变当前 code 的含义。

## 5. 权威事实契约

Registration fact 至少包含：

- 非零 `ParticipantRef`；
- 非零 `RegisteredAt`；
- 非零 `ObservedAt`，并且不能早于 `RegisteredAt`；
- 非空、规范化的事实来源；
- 非空、规范化的来源修订标识。

应用消费者拥有窄端口 `RegistrationFactReader`。端口只读取本条规则需要的最小事实，不允许调用方提交 `is_new_user`、`registered_at`、最终 verdict、客户端时间或完整用户画像。

事实提供方还必须遵守以下语义：

1. not found 在没有更强 provider contract 前表示无法确认，而不是自动表示新用户；
2. snapshot age 等于允许的最大值仍有效，超过才是 stale；
3. 观察时刻或注册时刻晚于服务端求值时刻是事实契约/时钟问题；
4. 来源超时、数据损坏、主体引用不匹配或未知错误都不得产生业务拒绝决定；
5. source/revision 用于内部追溯，不能把完整外部 payload、PII 或内部错误链暴露给客户端。

## 6. 决定与失败语义

本节使用 Participation 专用 `NewUserEligibilityDecision`，不创建跨上下文通用 PolicyDecision：

| 场景 | 返回值 | error | 是否允许进入后续选择 |
| --- | --- | --- | --- |
| 注册时刻等于或晚于下界 | `eligible` + 稳定 reason | `nil` | 当前规则允许；不等于完整流程允许 |
| 注册时刻早于下界 | `ineligible` + `registration_before_cutoff` | `nil` | 否 |
| fact not found | 零 decision | typed error | 否 |
| fact stale | 零 decision | typed error | 否 |
| 来源 unavailable/timeout | 零 decision | typed error / context error | 否 |
| fact 损坏或主体不匹配 | 零 decision | typed error | 否 |
| 调用取消 | 零 decision | 原始 context error | 否 |

这里的 fail-closed 是“不能继续执行”，不是把未知事实伪造成 `ineligible`。确定业务拒绝与依赖失败必须进入不同指标、重试策略、告警和未来 HTTP 映射。

决定只携带低基数、可追溯的必要摘要：outcome、rule code、reason code、policy revision、fact source/revision 与一次 evaluated-at。它不携带注册时刻、政策阈值、昵称、手机号或完整用户属性。

## 7. 确定性与副作用边界

相同 policy、fact snapshot 和 evaluated-at 必须得到相同决定。纯 evaluator 不读取系统时钟、不访问数据库、不使用随机源、不修改 snapshot，也不产生任何持久副作用。

应用服务的顺序固定为：

```text
validate request/service
  -> honor pre-cancellation
  -> read authoritative registration fact
  -> let observed cancellation win
  -> capture server evaluated-at exactly once
  -> validate participant/time/freshness
  -> evaluate concrete new-user policy
```

本节不扣次数、不预占资格、不创建 Participation、Draw 或 Result，因此不声称已经解决资格检查与最终使用之间的 TOCTOU。

## 8. 本节明确不持久化 Registration fact

本地 `participation_user` 或 `registration_fact` 表现在会制造第二事实源，因为系统尚未定义：

- 外部目录到本地的摄取、幂等、纠正、删除和重放协议；
- Activity / policy scope；
- 一次 Participation / Draw 的关联身份；
- 个人数据保留、删除、访问审计和加密要求；
- source revision 冲突与迟到事件语义。

因此第 25 节不新增 Migration，MySQL latest 继续为 2。未来若出现受控用户投影或正式结果快照需求，应以新需求、同步协议和隐私边界追加 Migration，不回写当前历史。

## 9. 本节明确不接公开 API

当前 ephemeral route 没有可信主体。新增 `X-Demo-User-ID` 或请求体中的 participant ID 只能证明调用方会填一个值，不能证明该值属于调用者；把它接入资格门控会制造虚假的安全闭环。

所以本节：

- 不修改现有 Lottery route、request、response、status 或 error code；
- 不修改 React 页面和导航；
- 不把 ParticipantRef 解释成登录身份；
- 不声称现有临时选择 API 已受资格保护；
- 不用前端隐藏按钮代替服务端判断。

真实会话在第 32 节形成；第 30 节前也没有 Activity。公开组合必须等待可信主体和正确业务资源出现，并继续区分认证、授权、资格和 Lottery 结果。

## 10. 不引入通用规则抽象

第 25 节只有一个具体 evaluator，因此禁止新增：

- `Rule`、`RuleChain`、`RuleEngine` 或 `Specification`；
- `[]Evaluator`、priority/order、通用 `map[string]any` context；
- trace tree、DSL、动态配置表或第三方规则引擎；
- 与 Authorization、Inventory 共用的万能 Decision 类型。

第 26 节出现第二条具体 Participation 前置规则后，再由两个真实消费者共同需要的输入、顺序、短路和错误语义反推最小线性协议。

## 11. 验收要求

### 11.1 正向与边界

- cutoff 前 1ns 为 ineligible；等于 cutoff 和晚 1ns 为 eligible；
- 相同 instant 的不同时区表示产生相同决定；
- 相同输入重复执行，决定完全一致；
- fact age 等于最大值有效，超过 1ns 为 stale；
- application clock 每次成功事实读取只调用一次。

### 11.2 失败与安全反例

- zero participant、空 source/revision、zero/倒置时间、非法 policy 不产生有效对象；
- not found、stale、unavailable、未知 source error、future fact、participant mismatch 都不产生 business decision；
- pre-cancel 不调用 reader，reader 后取消不调用 clock/evaluator；
- context cancellation 在与 dependency error 竞态时保留原始 context 语义；
- production service 签名不存在 `is_new_user`、客户端 `registered_at` 或客户端 evaluated-at；
- Participation domain 不依赖 Lottery、Gin、MySQL、Redis 或 React；
- Lottery、HTTP、Migration、Redis、Compose 与 Web 相对第 24 节保持零运行时变化。

### 11.3 证据边界

本节可以证明具体资格模型、权威 fact port、确定拒绝与技术失败分类、时间边界和取消语义可执行。它不能证明真实用户目录 adapter、登录身份、Activity 级政策、公开 HTTP 映射、浏览器 E2E、完整前置短路或正式 Draw 已经完成。

## 12. 后续演进

1. 第 26 节增加第二条真实 Participation 前置规则，再引入最小线性短路组合；
2. 第 27～29 节由真实分支、持久配置和执行需求推动规则树/决策引擎；
3. 第 30 节建立 Activity 与 Strategy 的边界；
4. 第 31～35 节依次建立公共访问控制模型、真实会话、服务端强制、前端感知和越权验收；
5. 正式 Participation/Draw 出现后，再决定事实快照、版本关联和 TOCTOU 的持久语义。

## 13. 追溯

- 上游需求：[Lottery 业务规则需求基线 v1](lottery-rule-requirements-v1.md)
- 上游决策：[ADR-0019](../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- 本节决策：[ADR-0021](../decisions/ADR-0021-participation-new-user-eligibility.md)

