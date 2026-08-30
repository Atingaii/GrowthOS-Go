# ADR-0021：以权威注册事实实现首个 Participation 新用户资格切片

- **状态：** 已接受
- **日期：** 2026-08-30
- **负责人：** GrowthOS 架构组

## 背景

第 23 节已经决定按事实所有者与业务阶段拆分抽奖规则，第 24 节只为 Lottery Strategy 建立可丢弃 Redis 读取投影。第 25 节需要把“不是所有用户都能抽”推进为第一条可执行资格规则，同时面对四个尚未具备的前提：

1. 系统没有真实会话或 Principal，现有 ephemeral route 的调用者与参与主体没有可信绑定；
2. 系统没有 Activity 或发布规则集，无法声称一个 cutoff 已经绑定到某场活动；
3. 外部用户目录才拥有注册事实，本地没有同步、修正、删除和隐私保留协议；
4. 当前只有一条具体资格规则，不足以证明责任链、规则树或通用执行引擎的正确抽象。

本 ADR 决定第 25 节的事实所有权、可执行边界、失败语义以及必须停止的位置。

## 决策驱动

1. `registered_at` 必须来自权威服务端事实，不能由浏览器提交 `is_new_user` 或客户端时钟推断；
2. Participation 拥有“是否满足本场景新用户条件”的决定，但不因此拥有外部账户生命周期；
3. 确定不符合与无法形成可信决定必须分开，否则指标、重试、文案和审计都会说谎；
4. 时间规则必须使用一次受控 evaluated-at，并明确 cutoff 和 freshness 的含边界语义；
5. 当前实现必须能被生产代码调用和单元/竞态测试证伪，不能只增加概念文档；
6. 不得为了演示端到端而提前伪造身份、Activity、事实表或正式 Draw；
7. 不得让新用户规则进入 Lottery Strategy、WeightedSelector、Redis projection 或 React 本地判断；
8. 第二条真实规则出现前，不创建万能 `Rule` 接口或执行框架。

## 评估过的方案

### 方案一：浏览器提交 `is_new_user` 或注册时间

| 优点 | 代价 / 风险 |
| --- | --- |
| 最少后端代码，页面容易演示 | 调用方可以直接篡改结论或时间 |
| 不需要事实读取端口 | 把 UI 状态误当权威事实，无法审计来源和版本 |
| 可立即接现有 route | 没有可信主体绑定，端到端外观掩盖真实越权边界 |

**结论：拒绝。** 客户端只能在未来提交业务意图与必要标识，最终资格必须由服务端从可信事实重新求值。

### 方案二：新增 demo user header 并包装现有 Lottery API

| 优点 | 代价 / 风险 |
| --- | --- |
| 可以展示资格拒绝后不执行选择 | header 可伪造，不能证明调用者就是该用户 |
| 容易形成 HTTP/React/Compose E2E | 会让课程在真实会话前产生虚假安全闭环 |
| 可以演示 409/503 区分 | 现在决定 status 会锁定尚无 Activity/正式参与用例的契约 |

**结论：当前不采用。** 现有 route 继续是无用户语义的开发模拟；不能用醒目的 `demo` 注释消除身份未绑定的事实。

### 方案三：在 MySQL 新建用户注册事实表

| 优点 | 代价 / 风险 |
| --- | --- |
| 能提供真实 SQL adapter 与 Compose 验收 | 没有摄取/更正/删除协议，会形成不受控第二事实源 |
| 可以固定 fixture | 没有 Activity/Participation identity，无法解释快照作用域 |
| 后续读取方便 | 未定义个人数据权限、保留期、加密、迟到和 revision 冲突 |

**结论：当前不采用。** `RegistrationFactSnapshot` 是一次求值的不可变 value，不等于应持久化的本地用户表。未来有受控投影需求时追加新 ADR 和 Migration。

### 方案四：只写一个纯 `IsNewUser(registeredAt, cutoff) bool`

| 优点 | 代价 / 风险 |
| --- | --- |
| 函数很小，边界容易测试 | 丢失 fact 来源、版本、新鲜度和主体一致性 |
| 没有依赖 | `false` 无法区分确定拒绝、缺事实和依赖失败 |
| 不会过度设计 | 无法证明真实应用边界不会信任客户端结论 |

**结论：不充分。** 保留纯 concrete evaluator，但用领域 policy/fact/decision value 与 application-owned fact reader 包住它。

### 方案五：直接引入责任链或规则引擎

| 优点 | 代价 / 风险 |
| --- | --- |
| 表面符合后续路线 | 唯一规则无法证明接口、顺序、短路和 context 形状 |
| 容易继续加节点 | 提前引入 priority、通用 map、trace、DSL 和发布治理 |
| 可包装多种上下文判断 | Eligibility、Authorization、Inventory 会被压成万能语言 |

**结论：拒绝。** 第 26 节至少出现第二条具体 Participation 规则后，再由真实消费方反推最小链。

### 方案六：Participation 的可执行 domain/application slice

| 优点 | 成本 / 风险 |
| --- | --- |
| 权威事实读取端口、规则、决定和错误都可执行 | 暂时没有生产 fact adapter 或公开消费者 |
| 不伪造身份、Activity 或本地事实所有权 | 当前 ephemeral Lottery API 仍未受资格门控 |
| 能精确测试 cutoff、freshness、取消和失败分类 | 必须在 QA/简历中克制表述，不能冒充完整闭环 |
| 保持 Lottery/Redis/React 零漂移 | 下一节组合时仍会调整应用编排 |

**结论：采用。** 这与第 17～20 节先形成领域、持久化和算法能力，再在第 21 节开放 API 的演进颗粒度一致。

## 决策

### 1. 外部用户目录拥有注册事实，Participation 拥有资格决定

Participation 使用非零 `ParticipantRef` 通过 consumer-owned `RegistrationFactReader` 获取必要最小快照。快照至少包含 registered-at、observed-at、source 和 source revision；它不复制完整用户资料，也不允许 Participation 反向修改用户生命周期。

ParticipantRef 只是内部 lookup reference，不是登录证明、Principal、角色或数据范围。真实身份与该引用的绑定留给第 32 节会话能力及后续业务编排。

### 2. 首个规则是固定身份的 registration-cutoff policy

规则 code 固定为：

```text
participation.new_user.registered_on_or_after
```

`registered_at >= cutoff`（含边界）形成 eligible；早于 cutoff 形成 ineligible。policy revision、fact revision 和应用发布版本保持概念分离。

本规则不读取系统时钟；domain evaluator 接受一个已经捕获的 evaluated-at。application service 先读取事实并处理取消，然后调用受控 Clock 一次，完成 future/freshness/主体一致性验证后执行 evaluator。

### 3. 使用具体 Decision + error，而不是 bool 或通用四态模型

事实充分时只形成两种 Participation 业务结果：eligible 或 ineligible。决定包含稳定 rule/reason、policy revision、fact source/revision 与 evaluated-at，不包含完整 PII、原始 payload、注册时间或 cutoff 文案。

fact not found、stale、source unavailable、事实损坏、主体不匹配、clock 非法和 context cancellation 均返回零 decision + typed error。Fail-closed 只意味着不能继续，不得把这些场景改写成 ineligible。

XACML 等成熟规范对 Deny 与 Indeterminate 的区分可校准这一语义，但本节不实现 XACML、OPA 或跨上下文 PolicyDecision。

### 4. 不持久化、不缓存、不接公开入口

第 25 节不新增 Migration、MySQL grant、Redis key、运行配置、HTTP header/route/status、React request/state 或 Compose fixture。现有 Lottery API 与页面继续保持无用户、无正式 Draw 的 ephemeral 边界。

Registration fact 和一次 eligibility decision 都不进入第 24 节 Strategy cache。它们的主体基数、时效、撤销、隐私和事实所有权与 Strategy 投影不同。

### 5. 不引入通用执行抽象

本节只新增具体 `NewUserPolicy`、`RegistrationFactSnapshot`、`NewUserEligibilityDecision`、pure evaluator、fact reader 和 application service。禁止 `Rule`、chain、tree、engine、generic context、priority、DSL 和动态规则表。

第 26 节必须先给出第二条具体 Participation 规则及其顺序/短路证据，再决定哪些共同语义值得抽取。

## 影响

### 正面影响

- “新用户”从可伪造布尔值变成基于权威时间事实、来源与版本的确定规则；
- 业务拒绝与事实未知/依赖失败具有不同的可测试语义；
- cutoff、freshness、time zone、单次时钟和取消竞态成为显式契约；
- Participation 与 Lottery、Governance、外部用户目录保持清晰依赖方向；
- 下一节能从至少两个真实规则消费者反推链，而不是让抽象先于需求。

### 成本与限制

- 本节没有生产事实 adapter，无法声明已经联通真实用户系统；
- 当前 Lottery demo route 仍可进行无主体的临时选择，不能称为资格受控抽奖；
- 没有 Activity scope、持久结果或审计记录，无法回放一次正式参与决定；
- 每次未来求值的事实读取性能、缓存和降级仍需基于真实 provider 证据评估；
- 公开状态码、用户文案和 UI 投影继续待定。

### 撤销与演进

纯 domain/application slice 没有外部 schema、route 或持久数据，撤销成本较低。未来事实来源、用户口径或 Activity 模型变化时，可以新增 adapter、policy revision 或组合服务；不得改变当前 rule code 的既有语义后仍声称是同一版本。

当出现受控用户投影、真实会话、Activity 绑定、正式 Participation/Draw 或多规则组合时，应分别追加 ADR、Migration 和验收证据，不直接改写本记录的历史时间切片。

## 相关资料

- [新用户资格规则基线 v1](../product/new-user-eligibility-v1.md)
- [Lottery 业务规则需求基线 v1](../product/lottery-rule-requirements-v1.md)
- [ADR-0019](ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [NIST SP 800-205](https://www.nist.gov/publications/attribute-considerations-access-control-systems)
- [OASIS XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-cos01-en.html)
- [Go FAQ：Interfaces](https://go.dev/doc/faq)

