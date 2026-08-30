# 第 23 节设计手记：先划清规则边界，再决定怎样执行

> 本节的产物不是一套已经运行的规则引擎，而是一份需求与架构边界决策。它回答“抽奖开始需要规则”以后，哪些事实属于谁、哪些失败不能混在一起、什么证据足以支持下一步，以及为什么现在写通用规则代码反而是不负责任。
>
> 最重要的结论是：**第 23 节不新增 Go 业务代码、Migration、Redis 业务接入、HTTP 路由或前端资格判断。** 本节通过场景、决策表、上下文所有权、失败语义和 ADR，约束第 24～30 节按真实问题逐步演进。

---

## 0. 决策命题与时间切片

### 0.1 这一节真正要决定什么

第 17～22 节建立了一条非常窄但真实的链路：

    Strategy/Award 配置
      → MySQL 一致快照
      → WeightedSelector
      → development/test ephemeral API
      → React 页面呈现服务端候选

这条链路只回答：

> 给定一个已经合法的 Strategy 快照，本次怎样按固定相对权重无偏选择一个 Award？

新需求说“抽奖策略需要规则”，但这句话仍然太模糊。它可能同时指：

- 活动是否已经发布、是否处于有效时间窗；
- 用户是否属于目标人群、是否还有参与次数；
- 风险名单或设备风险是否允许继续；
- 应使用哪个 Strategy 或哪个规则分支；
- 哪些 Award 可以进入候选集合；
- 抽中候选后是否有库存、能否完成权益交付；
- 当前操作者是否有权查看、配置或发布；
- 规则拒绝、系统失败和合法未中奖分别怎样表达。

如果不先拆开这些问题，最容易得到一个名叫 RuleEngine、实质上同时承担身份、资格、抽奖、库存、权限、网络调用和数据库写入的万能组件。这样的组件短期看似统一，长期却没有清晰的事实所有者，也无法解释部分失败。

所以本节的决策命题是：

> 在第一条具体资格规则、责任链、规则树和决策引擎出现之前，先定义抽奖规则的业务场景、阶段、决定所有权、原始事实来源、结果语义、信任边界与演进触发器，同时明确哪些抽象现在还没有证据创建。

### 0.2 当前已经实现的事实

当前仓库能够由代码和测试证明：

- Strategy 是 Lottery 聚合根，拥有至少一个 Award；
- Award 使用正整数相对权重，reward 与 no_reward 都是合法 Outcome；
- Strategy 拒绝零 ID、非法名称、重复 AwardID、零权重、未知 Outcome 和总权重溢出；
- MySQL 两张表是当前配置事实源；
- Repository 以原子事务写完整聚合，以同一只读 Repeatable Read 快照恢复聚合；
- WeightedSelector 接收完整 Strategy，使用 bounded random source 和减法桶选择一个 Award；
- 单候选确定性短路，多候选只请求一次随机位置；
- ephemeral API 读取一个 Strategy 并返回一个不持久化的候选；
- React 页面只展示服务端返回的 reward 或 no_reward，不把 reward 写成已经中奖或到账；
- 当前运行身份对两张业务表只有 SELECT；
- 当前没有用户、Activity、资格、次数、正式 Draw、结果持久化、库存、发奖、认证或 RBAC。

这些事实分别可追溯到：

- [Strategy 聚合](../../../internal/lottery/domain/strategy.go)
- [Award 模型](../../../internal/lottery/domain/award.go)
- [WeightedSelector](../../../internal/lottery/domain/weighted_selector.go)
- [Repository 端口](../../../internal/lottery/application/repository.go)
- [EphemeralSelectionService](../../../internal/lottery/application/ephemeral_selection.go)
- [第 21 节临时 API ADR](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)
- [第 22 节 React 纵向切片](../../course/part-03/lesson-22-react-lottery-page.md)

### 0.3 当前明确不存在的事实

本节不能把设计产物写成运行事实。当前不存在：

- Rule、RuleSet、RuleNode 或 DecisionEngine Go 类型；
- Strategy 版本、规则版本、发布版本或乐观锁版本；
- 规则表、决策树表、资格表或 Activity 表；
- 责任链、规则树、DMN runtime 或 JSON DSL；
- 规则配置 API、运营编辑器或规则发布流程；
- 用户资格查询、额度扣减、风控查询或库存查询；
- Redis Strategy cache；
- 服务端会话、权限判定、权限管理页面或越权验收；
- 可持久恢复的一次 Draw 结果；
- 生产吞吐、公平审计、法规认证或在线抽奖 SLO。

不存在不是缺陷掩饰，而是时间切片事实。第 23 节的价值恰恰是防止未知问题被过早代码化。

---

## 1. 不可争辩的业务问题

### 1.1 一个用于拆解、而不是用于假装实现的复合场景

我们用下面的需求作为分析样本：

> 运营准备一场“新用户抽奖”活动。活动只有在发布且处于时间窗内时开放；用户必须符合新人定义、仍有参与次数且不处于风险拒绝名单；不同会员等级未来可能进入不同奖池；当前不可用的奖项不得被当作可兑现奖励，但究竟先过滤、先预占、拒绝整次流程还是进入明确 fallback，必须等库存事实和产品概率策略出现后再决定；允许参与后仍可能正常抽到 no_reward。

这段话包含至少六类判断：

| 判断 | 输入事实 | 可能结果 | 决定所有者 | 原始事实提供方 |
| --- | --- | --- | --- | --- |
| 活动是否可参与 | 发布态、开始/结束时间 | 开放、未开始、已结束、未发布 | Marketing | Activity 发布快照与受控服务端时钟 |
| 用户是否符合人群 | 注册时间、标签、人群版本 | 符合、不符合、数据不可用 | Participation | 外部用户/会员映射 |
| 是否还有次数 | 额度、已消费参与事实 | 可参与、次数不足、状态未知 | Participation | Participation 账户与流水 |
| 是否存在风险拒绝 | 风险信号、策略版本 | 允许、拒绝、依赖不可用 | Participation | 受控风险端口提供的最小 verdict 与版本 |
| 选择哪个策略分支 | 会员层级、规则配置 | 目标 Strategy 或拒绝 | Lottery | Lottery 发布配置与受控事实快照 |
| 从候选中选哪个 Award | Strategy 快照、随机位置 | reward 或 no_reward | Lottery | Strategy/Award 配置与 bounded random source |

后续库存和权益交付仍是另外的事实：

| 判断或动作 | 为什么不属于当前规则判断 |
| --- | --- |
| 奖品是否有可扣减库存 | 它需要并发一致性和库存事实源，不能只靠规则布尔值 |
| 权益是否已到账 | 它是 Benefit 的持久状态转换，不是 Lottery 决策 |
| 重复请求是否返回同一结果 | 它需要 request identity 与持久 Draw，而不是一个 Rule |

### 1.2 为什么业务说“规则”，架构不能只有一个 Rule

“规则”是业务人员的自然语言总称，不是自动成立的软件边界。两个判断都写成 if，并不意味着它们应该由同一个聚合、同一个数据库或同一个引擎拥有。

判断边界至少要看：

1. 谁拥有输入事实；
2. 谁有权宣布结果成立；
3. 失败时谁负责恢复；
4. 规则变化是否和同一组业务对象一起发布；
5. 是否需要与其他变化保持原子性；
6. 是否包含不可逆副作用；
7. 是否需要独立审计或合规控制。

Eric Evans 的 [DDD Reference](https://www.domainlanguage.com/ddd/reference/)强调限界上下文、统一语言和模型边界。这里应用它的方式不是把每条规则拆成微服务，而是先阻止一个术语跨上下文偷走事实所有权。

### 1.3 规则、选择和副作用必须分开

本项目约定三个不同概念：

| 概念 | 回答的问题 | 输入 | 输出 | 当前状态 |
| --- | --- | --- | --- | --- |
| Rule Decision | 是否继续、走哪个分支、采用什么选择计划 | 权威上下文快照与版本 | 可解释的决策值 | 只分析，未实现 |
| Weighted Selection | 给定合法 Strategy，哪一个 Award 被选中 | Strategy 快照、均匀随机位置 | reward/no_reward Award | 第 20 节已实现 |
| Side Effect | 次数、Draw、库存、权益等事实怎样改变 | 命令、幂等身份、当前持久状态 | 新持久状态或结果未知 | 尚未实现 |

正确的概念流是：

    权威输入快照
      → 规则决策
      → 选择计划
      → WeightedSelector
      → Award 候选
      → 后续持久命令与副作用

它不是当前调用链，也不是第 23 节承诺的 API；它只是避免职责混淆的逻辑分段。

如果 Rule 自己扣次数，Rule 就不再只是判断；如果 Selector 自己查用户，固定权重算法就失去纯度；如果库存不足直接返回 no_reward，系统故障就被伪装成合法业务结果；如果权限不足被解释为“不符合新人资格”，安全边界和业务语义都会被破坏。

---

## 2. 为什么现在只建立边界

### 2.1 当前没有第一条足够具体的运行规则

一个可实现的规则至少需要：

- 稳定业务名称；
- 权威输入来源；
- 输入缺失或过期的处理；
- 决策结果集合；
- 优先级或组合语义；
- 版本与发布时间；
- 可解释原因；
- 测试样例与边界值；
- 失败恢复责任。

“不是所有用户都能抽”还没有说明：

- 以登录用户、会员用户还是活动账户为主体；
- 新用户按注册时间、首单时间还是活动入组时间判断；
- 时间使用哪个时区和业务时钟；
- 人群版本变化是否影响已经入组用户；
- 数据源不可用时拒绝、降级还是暂停；
- 资格通过是否需要立即占用一次额度。

这些问题在第 25 节通过具体用户资格需求出现。现在创建接口只能猜测。

### 2.2 过早接口会把未知差异压平

最典型的过早抽象是：

    type Rule interface {
        Evaluate(context.Context, map[string]any) (bool, error)
    }

它看似通用，实际丢失了：

- bool 无法区分允许、业务拒绝、路由、跳过与降级；
- error 容易混淆依赖失败、配置损坏和授权拒绝；
- map 无法表达字段来源、版本、时效与必填性；
- 任意值会把完整 uint64、时间、枚举和 PII 边界推迟到运行时；
- 不知道接口消费者是谁，也不知道 timeout 与取消语义；
- 不知道规则是否允许副作用；
- 不知道顺序、短路和多结果合并方式；
- 不知道解释轨迹是否是结果的一部分。

Go 官方 [Effective Go](https://go.dev/doc/effective_go?lang=en&version=1)说明一两个方法的小接口很常见，但“小”不等于“语义完整”。当前项目坚持 consumer-owned 窄端口；在真正消费者和失败语义出现前，不创建接口比创建一个只有一个方法却含义含混的接口更稳健。

### 2.3 需求与架构章节仍然有可验收产物

不写业务代码不等于空谈。本节必须留下：

- 可复核业务场景；
- 现有能力差距；
- 规则阶段与上下文所有权矩阵；
- 决策、选择、副作用三分法；
- 结果与失败语义；
- 方案比较与拒绝理由；
- 未来章节追踪矩阵；
- ADR 固化跨章节边界；
- 负向验收，证明没有提前加入代码、表或路由；
- 回归验证，证明现有纵向切片未被文档工作破坏。

这些产物会减少第 25～30 节返工，并让每个后续实现都能说明“哪个真实问题使前一个模型不够用了”。

---

## 3. 从第一性原则推导需求

### 3.1 事实源

**事实：** 规则输入来自不同上下文，前端表单和请求 payload 只是声明，不是权威事实。

**风险：** 如果客户端提交 member_level、remaining_quota 或 risk_passed 并被直接信任，用户可以修改请求绕过业务规则。

**需求：**

- 每个输入字段必须标注权威来源；
- 客户端声明只能作为查找线索，不能成为最终资格事实；
- 输入快照需要来源版本、读取时间和时效边界；
- 规则配置也要经过恢复校验，不能因为来自数据库就默认可信。

**证据方向：** 第 25 节开始为第一条资格规则设计权威 fixture 和伪造客户端输入反例。

### 3.2 正确性

**事实：** WeightedSelector 已经精确完成固定权重映射。

**风险：** 若规则逻辑混入 Selector，既有无偏桶测试无法独立证明选择正确；规则分支也可能通过改变随机调用次数制造隐藏偏差。

**需求：**

- Selector 继续只接收合法 Strategy 快照；
- 规则阶段先产生明确选择计划；
- 相同输入快照、规则版本和显式测试随机源应得到确定结果；
- 规则拒绝不调用随机源；
- 技术失败不返回 fallback Award；
- no_reward 只来自合法 Strategy 候选。

**证据方向：** 后续测试同时断言 decision trace 和 random source call count。

### 3.3 可靠性

**事实：** 当前 ephemeral 调用没有持久结果，客户端重试会产生一次新选择。

**风险：** 规则执行增加远程依赖后，超时可能发生在选择前、选择中或副作用后；若全部返回一个 error，调用方会错误重试。

**需求：**

- 区分选择前可安全失败与副作用后的结果未知；
- 每个远程输入要有 timeout、取消和过期策略；
- 依赖不可用不能伪装成用户不合格；
- 明确哪些可选个性化规则允许回退到基线，哪些强约束必须失败关闭；
- 只有持久 request identity 和结果查询出现后，才能承诺安全重试。

**证据方向：** 第 26 节规则链故障测试；正式 Draw 章节验证重复与结果未知。

### 3.4 安全

**事实：** 业务规则、访问授权和客户端展示是三个不同控制层。

**风险：**

- 把“有权限调用”当“符合业务资格”；
- 把“前端隐藏按钮”当服务端保护；
- 把动态规则表达式当可信代码执行；
- 把规则解释轨迹中的用户标签、风险原因或密钥写入日志。

**需求：**

- 第 31～35 节独立建立 Principal、Role、Permission、Resource、Action 与 Scope；
- 服务端逐请求授权，前端只做能力投影；
- 授权成功后仍必须执行业务资格；
- 动态配置按不可信输入进行语法、语义、资源和权限校验；
- 规则追踪使用稳定 reason code，敏感细节只进入受控审计；
- 配置发布与权限变更都要有独立审计。

[OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)要求默认拒绝并逐请求验证权限；这支持第 31～35 节的安全边界，但不意味着业务资格也应塞进同一个 RBAC 判定器。

### 3.5 版本

**事实：** 当前 Strategy 只有行级 updated_at，没有聚合业务版本，也没有 Update 路径。

**风险：**

- 缓存 key、规则发布、历史解释和并发编辑使用不同“版本”含义；
- Award 行变化没有推动根行时间戳；
- 运行中的 Draw 在规则变化后无法解释当时为何通过。

**需求：**

至少区分：

| 版本 | 作用 |
| --- | --- |
| Cache Schema Version | Redis value 编码兼容，不等于业务版本 |
| Strategy Config Version | 一份可发布抽奖配置的不可变身份 |
| Rule Set Version | 一组规则定义和图结构的业务版本 |
| Input Snapshot Version | 人群、风险、额度等输入事实版本 |
| Algorithm Version | 选择算法或编译结构版本 |
| Policy Version | 第 31～35 节访问控制策略版本 |

本节只固定概念，不新增字段。具体版本结构必须由 Update、发布、缓存或历史 Draw 用例推动。

### 3.6 可解释性

**事实：** 面向用户的拒绝原因、运营调试信息和安全审计需要不同粒度。

**风险：**

- 只返回 true/false 无法支持客服、运营和故障定位；
- 返回完整内部表达式会泄露风控策略；
- 使用自然语言作为机器分支会随翻译或文案变化破坏兼容；
- trace 无界增长造成延迟、存储和隐私问题。

**需求：**

- 稳定 RuleCode 与 ReasonCode，不依赖展示文案；
- DecisionResult 能区分允许、业务拒绝、路由和技术失败；
- 有序 trace 至少关联规则身份、版本、结果和低基数耗时分类；
- 用户、运营、安全审计分别使用允许公开的投影；
- trace 有数量、深度、大小和保留期限上限；
- 可解释不等于暴露所有输入。

### 3.7 失败语义

本节先建立概念分类，不承诺 HTTP code：

| 概念结果 | 含义 | 是否调用 Selector | 是否等于 no_reward |
| --- | --- | ---: | ---: |
| allowed | 前置判断允许继续 | 是 | 否 |
| business_denied | 权威业务事实明确拒绝 | 否 | 否 |
| routed | 决定使用一个明确 Strategy/分支 | 后续可能 | 否 |
| dependency_unavailable | 无法取得必需输入 | 否 | 否 |
| invalid_configuration | 规则或 Strategy 配置损坏 | 否 | 否 |
| unauthorized / forbidden | 主体未认证或无访问权限 | 否 | 否 |
| no_reward | 合法选择命中 no_reward Award | 已调用 | 是 |
| result_unknown | 副作用可能发生但结果不可确认 | 取决于阶段 | 否 |

关键原则：

1. 业务拒绝是一个可解释决定，不是依赖故障；
2. 依赖故障可以选择失败关闭，但公开语义仍是暂不可用，不应谎报“用户不符合”；
3. no_reward 必须来自合法抽奖配置；
4. unauthorized 与 business_denied 分开，防止权限逻辑污染业务规则；
5. result_unknown 只有未来持久副作用出现后才成立，当前不虚构。

Go 官方 [Errors are values](https://go.dev/blog/errors-are-values)和 [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)说明 error 可以携带可检查语义。本项目后续可以用类型化结果与错误分类表达上述差异，但第 23 节不提前确定 Go 类型。

---

## 4. 决定所有权、权威状态与事实来源边界

### 4.1 上下文责任矩阵

| 能力 | 拥有的决定 | 拥有的权威状态 | 外部原始事实/输入 | 不得拥有 |
| --- | --- | --- | --- | --- |
| Marketing | Activity 发布、暂停与时间窗判定 | Activity 草稿、发布态与活动版本 | 受控服务端时钟、Strategy 发布版本引用 | Award 权重、用户额度、权限角色 |
| Participation | 人群、额度与本场景准入判定 | Participation 账户、额度与参与事实 | Activity identity、外部用户/会员映射、受控风险 verdict | Strategy 权重、权益余额 |
| Lottery | Strategy 路由、候选选择与 Draw/Result 状态迁移 | Strategy、Award、路由/选择配置与 Draw/Result | 已验证参与请求、Activity 引用、Benefit 可分配决定 | 用户身份生命周期、活动发布、权益到账 |
| Benefit | 候选可分配、发放与补偿判定 | 内部库存、权益发放、余额与补偿状态 | Award/Reward 快照、Draw identity、外部权益回执 | 资格和抽奖概率 |
| Governance | 资源动作授权与策略管理判定 | Principal、Role、Permission、Scope、策略版本与审计 | 身份/IAM 适配、业务资源 identity 与风险等级 | 用户是否满足活动资格、随机选择结果 |
| Web 体验层 | 不拥有权威决定，只呈现服务端投影 | 表单与短生命周期交互状态 | 服务端 session、decision 和 result DTO | 资格、权限、库存和中奖事实 |
| Redis Adapter | 不拥有业务决定，只执行命中/回源机制 | 可丢弃、可重建的读取投影 | MySQL Strategy 快照 | 权威配置、用户资格、最终 Draw |

### 4.2 为什么 User 不能塞进 Strategy

Strategy 是可复用选择配置。若把 user_id、member_level、remaining_quota 或 risk_tags 放进聚合：

- Strategy 会随每个用户状态变化；
- 配置缓存会混入高基数个人数据；
- 同一个策略无法被多个 Activity 复用；
- Lottery 被迫拥有用户数据生命周期；
- 配置版本和用户事实版本无法区分；
- 删除或隐私请求会污染抽奖配置历史。

正确方向是由 Participation 提供已验证参与上下文或窄投影，Lottery 只消费完成选择所需的最小信息。

### 4.3 为什么 Activity 不能塞进 Strategy

Activity 有草稿、发布、开始、结束、暂停和归档生命周期；Strategy 是可复用决策配置。两者变化原因不同：

- 同一 Activity 可能在版本间引用不同 Strategy；
- 同一 Strategy 可能被多个 Activity 复用；
- 活动停止不等于删除策略；
- 策略调整需要独立版本和历史解释；
- 运营权限和审批可能分别作用于 Activity 与 Strategy。

第 30 节会用真实用例建立引用与版本边界。本节只记录约束。

### 4.4 为什么权限模型必须独立

访问控制回答：

> 某个主体能否对某个资源执行某个动作，并处于什么 Scope？

业务资格回答：

> 一个经过认证且有权发起请求的主体，在当前 Activity 与 Participation 事实下是否符合参与条件？

一个有权限查看 Strategy 的运营人员不因此能参与活动；一个符合新人资格的消费者也不因此能编辑 Strategy。二者可以共享 Request ID、资源 identity 和审计基础设施，但不能共享一个布尔判断或同一组角色。

第 31～35 节按以下顺序落地：

1. 统一访问控制模型与威胁边界；
2. 真实会话认证；
3. 服务端 RBAC 强制；
4. 前端消费统一权限/能力投影，并裁剪导航、路由与操作；
5. 越权与浏览器端到端验收。

在此之前，任何 activeRole、隐藏菜单或页面入口都只是演示状态。

---

## 5. 备选方案矩阵

### 5.1 现在只交付需求边界与 ADR

| 维度 | 评价 |
| --- | --- |
| 优点 | 忠于当前事实；让第 25 节具体规则决定接口；保留责任链、规则树和 DMN 等选择空间 |
| 代价 | 本节没有运行时功能；读者需要接受“文档也可形成架构增量” |
| 风险 | 文档可能与后续代码漂移 |
| 控制 | ADR、追踪矩阵、每节 QA 和完成门禁 |
| 结论 | **采用** |

### 5.2 在 handler 中先写 if

| 维度 | 评价 |
| --- | --- |
| 优点 | 修改少，可快速演示 |
| 代价 | transport 拥有业务规则；无法复用于其他入口；单测和错误映射耦合 |
| 适用条件 | 一次性原型且明确不会进入真实业务 |
| 结论 | 当前不采用 |

### 5.3 在 Strategy 聚合中加入全部规则

| 维度 | 评价 |
| --- | --- |
| 优点 | 表面上聚合内聚；调用方只拿一个对象 |
| 代价 | 用户、活动、风险、库存和权限被吸入 Lottery；版本与缓存失控 |
| 适用条件 | 规则只依赖不可变 Strategy 自身字段，例如配置内部合法性 |
| 结论 | 仅保留 Lottery-owned 配置不变量，不采用巨型聚合 |

### 5.4 现在定义统一 Rule 接口

| 维度 | 评价 |
| --- | --- |
| 优点 | 看起来可插拔，便于列出多个实现 |
| 代价 | 尚无消费者、输入、结果、顺序、错误和副作用契约；抽象高度猜测 |
| 适用条件 | 至少两条真实规则在同一阶段拥有稳定共同协议 |
| 结论 | 延迟到第 26 节由前置规则链用例决定 |

### 5.5 建立 common/rules 万能包

| 维度 | 评价 |
| --- | --- |
| 优点 | 多模块可导入，目录显得统一 |
| 代价 | 抹平上下文语言；共享的往往只是名字，不是业务语义；依赖方向模糊 |
| 适用条件 | 多个消费者已经证明共享稳定协议且不泄漏具体上下文 |
| 结论 | 不采用 |

Go 官方 [Code Review Comments](https://go.dev/wiki/CodeReviewComments)明确建议避免 util、common、misc、api、types、interfaces 等无意义包名。这里不是机械禁止 common，而是要求共享包必须先有稳定、可命名的业务责任。

### 5.6 引入通用规则引擎

| 维度 | 评价 |
| --- | --- |
| 优点 | 动态配置、条件动作、可能支持优化执行和可视化管理 |
| 代价 | 新计算模型、规则冲突、优先级、调试、部署、兼容、供应链和运维成本 |
| 适用条件 | 规则数量与变化频率有真实数据，代码发布成为稳定瓶颈 |
| 结论 | 当前不采用 |

Martin Fowler 的 [Rules Engine](https://martinfowler.com/bliki/RulesEngine.html)指出规则引擎适合部分问题，chaining 可能很难推理和调试，并倾向领域特定的窄范围方案。它不是“规则引擎永远不好”的论证，而是提醒项目必须证明替换计算模型的收益。

### 5.7 引入 DMN runtime

| 维度 | 评价 |
| --- | --- |
| 优点 | 标准化决策模型、决策表和跨角色沟通，具有明确语义与生态 |
| 代价 | FEEL 语义、命中策略、模型版本、部署、扩展函数、兼容与合规测试成本 |
| 适用条件 | 多部门维护大量决策表、需要标准交换或业务分析师共同建模 |
| 结论 | 可作为建模参考，当前不引入 runtime |

OMG 的 [DMN 页面](https://www.omg.org/dmn/)将 DMN 定义为精确描述业务决策和规则的建模语言；当前项目只有一个待澄清复合需求，尚无证据承担 DMN runtime。若采用，也必须以正式版本规范和一致性测试为基线，不能把“画了决策表”写成“已符合 DMN”。

### 5.8 使用 JSON DSL

| 维度 | 评价 |
| --- | --- |
| 优点 | 易存储和传输，前端编辑器容易生成 |
| 代价 | 语法不等于语义；类型、版本、引用、深度、资源预算和安全边界都要自建 |
| 适用条件 | 已有稳定规则语言、明确 schema、静态校验器、迁移器和沙箱预算 |
| 结论 | 当前不采用 |

Go 官方 [JSON and Go](https://go.dev/blog/json)说明通用 JSON 解码到 interface 时数字默认成为 float64。项目当前明确支持完整 uint64，若使用松散 map 会重新引入精度和运行时类型风险。即使使用 Decoder.UseNumber 或自定义 decoder 能缓解数值问题，DSL 的领域语义、资源限制和版本迁移仍未解决。

### 5.9 让业务人员直接编写规则

| 维度 | 评价 |
| --- | --- |
| 优点 | 减少研发介入的理想目标 |
| 代价 | 精确语义、冲突、测试、审批、回滚和责任并不会因 GUI 或 DSL 消失 |
| 适用条件 | 有受限表达能力、模拟、双人审批、版本发布、回放与紧急停用 |
| 结论 | 当前不承诺 |

Fowler 的 [Business Readable DSL](https://martinfowler.com/bliki/BusinessReadableDSL.html)提醒“业务可读”不等于“无需程序设计能力”。本项目未来若提供运营配置，目标应是减少误操作并保留治理，而不是把生产代码执行权包装成表单。

---

## 6. 不变量与信任边界

### 6.1 任何后续实现都必须保持的不变量

1. WeightedSelector 不读取用户、Activity、权限、Redis 或库存；
2. 规则拒绝时不调用随机源；
3. no_reward 只能是 Strategy 内合法 Award；
4. 技术故障不转换为 no_reward；
5. 授权拒绝不转换为业务资格拒绝；
6. 输入声明必须回到权威事实源验证；
7. 规则配置从数据库或缓存恢复后仍需语义验证；
8. 规则执行本身不偷偷扣次数、写 Draw、扣库存或发权益；
9. 副作用必须有显式命令、事务/幂等和结果查询语义；
10. 决策版本与缓存编码版本分开；
11. 解释 reason code 稳定，展示文案可本地化；
12. 规则顺序、短路、默认分支和未知输入行为必须显式；
13. 配置无匹配分支时失败关闭，不由 map 遍历顺序决定；
14. trace 有界且不泄露敏感输入；
15. 前端裁剪不成为安全边界。

### 6.2 信任边界

| 边界 | 默认信任 | 必须验证 |
| --- | --- | --- |
| Browser → API | 不可信 | 身份、权限、语法、业务语义、资源预算 |
| API → Application | transport 已解析但业务未成立 | typed ID、调用意图、上下文 deadline |
| Application → Rule Decision | 依赖返回可能失败或过期 | 来源、版本、完整性、时效 |
| Redis → Domain | 可丢弃且可能损坏 | schema version、完整解码、领域重建 |
| MySQL → Domain | 权威存储但数据仍可能损坏 | 约束、跨行不变量、领域恢复 |
| Rule Config → Runtime | 操作者输入，不等于安全代码 | schema、引用、环、深度、复杂度、权限、发布态 |
| Rule Decision → Selector | 只有合法计划可进入 | Strategy identity/version、候选完整性 |
| Selector → Side Effect | Award 只是候选 | Draw identity、库存、权益命令和幂等 |
| Authorization → Business Rule | 只证明可执行动作 | 仍需资格与业务约束 |

OWASP [Input Validation Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)区分语法与业务语义验证，并建议服务端 allowlist；[Business Logic Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Business_Logic_Security_Cheat_Sheet.html)进一步强调格式合法不代表业务合法。这正是动态规则配置不能只过 JSON Schema 就直接执行的原因。

---

## 7. 资源所有权、并发与时间预算

本节没有新增运行资源，但必须提前记录未来实现要回答的问题。

### 7.1 资源所有权

| 资源 | 建议所有者 | 不应由谁关闭或修改 |
| --- | --- | --- |
| MySQL pool | composition root | 单条规则 |
| Redis client | composition root / cache adapter | Strategy 或 Selector |
| Rule definition snapshot | Repository/compiled snapshot owner | 单次请求原地修改 |
| DecisionContext | 单次用例 | 全局缓存跨用户复用 |
| DecisionTrace | 单次决策 | 规则节点写入全局可变 slice |
| RandomSource | Selector composition | 规则节点重新 seed |
| 外部资格 client | adapter/composition | domain 规则直接创建 |

### 7.2 并发

后续规则实现应优先不可变：

- published RuleSet 按版本只读；
- 单次 DecisionContext 不跨请求共享；
- 节点实现无请求级可变字段；
- 编译结果可安全共享，但构建和发布原子切换；
- 动态配置更新不原地修改正在执行的图；
- trace 每请求独立并有上限。

若规则需要扣次数，它已经跨入副作用，应由 Participation 的持久用例以并发控制完成，而不是给 Rule 加锁。

### 7.3 时间预算

未来总预算需要满足：

    handler deadline
      > authentication + authorization
      > authoritative input reads
      > rule decision
      > selection
      > durable side effects
      > response serialization

这不是说每层各自拥有完整总 timeout。每层只能消费剩余预算，并在不可取消段前检查 context。当前 WeightedSelector 无 context 且 Award 数量上限为 1000；若规则引入远程调用，应把 I/O 放在 adapter/application，而不是纯 domain 节点里隐藏阻塞。

---

## 8. 失败模型与恢复语义

### 8.1 输入缺失

| 情况 | 错误做法 | 正确方向 |
| --- | --- | --- |
| member level 不存在 | 当普通会员继续 | 若规则必需则 unavailable；若产品明确 baseline fallback 才降级 |
| quota 读取超时 | 当次数为 0 | 暂不可用，不伪装业务拒绝 |
| risk 服务超时 | 默认放行 | 高风险约束失败关闭，同时保留技术失败语义 |
| Activity 不存在 | 使用请求中的 StrategyID | 由 Activity 边界决定 not found/不可参与 |

### 8.2 配置损坏

可能出现：

- 未知 RuleCode；
- 重复节点 identity；
- 空根；
- 环；
- 超过最大深度或节点数；
- 引用不存在的 Strategy/version；
- 分支无默认行为；
- 相同优先级含义不明确；
- 发布版本仍引用草稿；
- 输入字段类型或枚举不兼容。

这些都不是用户不合格，也不是 no_reward。恢复原则是：

1. 发布前静态校验；
2. 加载后再次恢复校验；
3. 运行时命中不变量则失败关闭；
4. 告警关联配置版本和 request ID；
5. 不自动修复并继续；
6. 支持回滚到上一已验证发布版本。

### 8.3 部分失败

将来链路可能在以下位置失败：

1. 授权前；
2. 资格读取前；
3. 部分资格读取后；
4. 规则已允许但尚未选择；
5. Award 已选但 Draw 尚未持久；
6. Draw 已持久但响应丢失；
7. 库存已扣但 Benefit 未发；
8. Benefit 已发但回执未确认。

第 23 节只覆盖 1～4 的概念边界。5～8 必须由 Draw identity、状态机、Outbox、补偿和结果查询逐步解决，不能让规则引擎“统一重试”。

### 8.4 乱序与旧版本

若规则发布 v7 时某请求已经加载 v6：

- 运行中的请求是继续 v6、强制中止还是重新决策，需要发布协议；
- 不能一半使用 v6 节点、一半使用 v7 Strategy；
- trace 必须记录实际版本；
- 缓存失效不应把正在执行的不可变快照变成悬空引用；
- 高风险紧急停用可能需要比普通版本发布更强的即时拒绝机制。

本节不决定协议，但要求第 28～30 节不能忽略。

### 8.5 可重试边界

- 规则拒绝不是重试理由；
- 选择前 dependency_unavailable 可以由用户明确重试，但仍受退避和限流；
- 当前 ephemeral selection 超时后不能自动重试为“同一次”；
- 正式 Draw 出现后，只有带持久幂等身份和结果查询才能安全恢复；
- 配置损坏需要修复或回滚，重试相同版本没有意义。

---

## 9. 安全、隐私与审计

### 9.1 规则配置是一种高风险输入

动态规则可能造成：

- 所有人被拒绝或所有人被放行；
- 大额奖池被错误路由；
- 深层树或恶意表达式耗尽 CPU；
- 外部函数访问未授权资源；
- 规则原因泄露风控名单；
- 绕过审批直接发布；
- 回滚后仍有旧缓存执行。

因此未来配置面需要：

- 独立读、编辑、校验、模拟、提交、审批、发布、回滚权限；
- schema 与语义双重校验；
- 最大节点、深度、表达式复杂度和执行预算；
- 变更 diff、操作者、审批者、版本与发布时间；
- 模拟使用脱敏数据；
- 发布前后样本回放和 canary；
- break-glass 有时限、理由和强化审计；
- 运行时不允许任意网络、文件或系统调用。

### 9.2 审计不等于普通日志

建议区分：

| 产物 | 用途 | 内容 |
| --- | --- | --- |
| Request Log | 运维关联 | request ID、route、状态、低基数分类 |
| Decision Trace | 调试与解释 | RuleCode、版本、阶段、结果、耗时分类 |
| Security Audit | 权限与高风险变更 | principal、resource、action、scope、policy version、decision |
| Business Audit | 业务配置发布 | Strategy/RuleSet 版本、diff、审批和回滚 |
| Draw Fact | 最终业务事实 | Draw identity、快照/版本、结果与状态 |

OWASP [Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)建议记录授权失败、高风险管理操作和可疑业务流程，同时提醒日志本身可能包含 PII、秘密和有价值的业务信息。DecisionTrace 不能因为“可解释”就无边界记录用户全量画像或风控特征。

### 9.3 拒绝原因的分层

- 用户层：安全、可操作、不过度泄露，如“暂不符合本活动条件”；
- 客服/运营层：稳定 reason code 与允许查看的业务维度；
- 安全层：更细策略版本、来源与异常信号，受独立权限保护；
- 工程层：依赖 error class、耗时和 request ID，不包含秘密；
- 规则作者层：配置路径与模拟输入，但默认脱敏。

同一个 DecisionResult 可以有多种受控投影，不能直接把内部 trace JSON 返回浏览器。

---

## 10. 证据设计

### 10.1 本节需要证明什么

| 结论 | 证据 |
| --- | --- |
| 规则需求被拆成可追踪责任 | 需求矩阵每行有输入所有者、结果和未来章节 |
| 当前不应创建 Rule 接口 | 备选矩阵与缺失契约清单 |
| 规则、选择、副作用没有混用 | 三分法、不变量和失败表 |
| 权限模型保持独立 | 第 31～35 节责任与授权/资格对照 |
| 没有提前实现后续 | 相对第 22 节的源码、Migration、route 负向 diff |
| 既有行为未被破坏 | make verify 回归 |
| 文档闭环成立 | make doc-check 与链接检查 |

### 10.2 建议验收命令

    make doc-check
    make verify

    git diff --exit-code \
      origin/codex/lesson-22-react-lottery-page..HEAD \
      -- cmd internal web migrations deploy configs

负向 diff 是本节的重要证据：它证明“没有实现”是经过范围控制的决定，而不是遗漏后被文案掩盖。

### 10.3 这些证据不能证明什么

- 不能证明某条规则实现正确，因为本节没有规则实现；
- 不能证明责任链或规则树性能；
- 不能证明 Redis 命中率；
- 不能证明用户资格、风控和权限真实存在；
- 不能证明规则作者能够安全自助发布；
- 不能证明 DMN 兼容；
- 不能证明在线抽奖 SLO；
- 不能证明正式 Draw 可恢复；
- 不能证明未来架构一定不变。

### 10.4 Specification by Example 的使用边界

Martin Fowler 的 [Specification by Example](https://martinfowler.com/bliki/SpecificationByExample.html)说明示例有助于形成可判定规格，但不能成为唯一需求技术。因此本节使用复合场景和决策表发现歧义，同时仍保留一般不变量、所有权和失败模型；不会拿五个示例宣称覆盖全部规则空间。

---

## 11. 被刻意推迟的能力

| 能力 | 推迟原因 | 计划章节或触发器 |
| --- | --- | --- |
| Redis Strategy cache | 先固定缓存事实边界与基线 | 第 24 节 |
| 第一条用户资格规则 | 需要具体用户与权威数据语义 | 第 25 节 |
| 前置责任链 | 需要至少两条同阶段规则证明共同协议 | 第 26 节 |
| 规则树 | 需要责任链无法表达的真实分支问题 | 第 27～28 节 |
| 决策引擎 | 需要持久树、执行语义和错误模型 | 第 29 节 |
| Activity/Strategy 引用 | 需要真实活动生命周期 | 第 30 节 |
| 公共访问控制 | 需要稳定资源与动作模型 | 第 31～35 节 |
| 运营后台 | 必须复用服务端授权 | 第 36 节 |
| 正式 Draw/结果恢复 | 需要参与、幂等和持久状态 | 后续活动闭环 |
| 库存与权益副作用 | 可分配/并发库存由第 43～45 节形成主链，结果与补偿由第 46～52 节闭合，Benefit 在第 54～61 节继续演进 | 第 43～61 节分阶段 |
| DMN/DSL | 需要规则规模、变更频率和治理证据 | 触发式重评 |

---

## 12. 需求未提但架构师会主动检查的点

### 12.1 规则复杂度预算

未来必须限制：

- RuleSet 最大节点数；
- 树最大深度；
- 每节点最大分支数；
- 单次决策最大外部读取数；
- trace 最大条数和字节数；
- 单次决策 CPU 与 wall-clock 预算；
- 配置解析和编译内存；
- 最大字符串、列表和集合大小。

没有预算的“灵活规则”可能成为业务层 DoS。

### 12.2 发布与回滚

需要明确：

- 草稿与已发布不可混用；
- 发布版本不可原地编辑；
- 发布切换是否原子；
- 旧版本保留多久；
- 回滚是否生成新版本还是恢复引用；
- 缓存怎样感知；
- 紧急停用是否绕过普通发布窗口；
- 在途请求怎样处理；
- 规则与 Strategy 是否作为一个发布单元。

### 12.3 测试数据与隐私

- 生产用户画像不能直接复制到开发；
- 模拟样本应脱敏并保留代表性；
- 边界样本覆盖时区、缺失值、极端 uint64、未知枚举和旧版本；
- 回放数据的访问权限与保留期独立于普通日志；
- 决策差异报告不能泄露风险标签。

### 12.4 多租户与数据范围

当前没有 tenant_id。未来多租户出现时：

- RuleCode 是否租户内唯一；
- 公共模板与租户覆盖怎样合并；
- Strategy/Activity 引用是否包含 tenant；
- 缓存 key 是否隔离；
- 管理员权限是否限制在 scope；
- trace 和审计如何防止跨租户查询。

本节只记录触发器，不提前把 tenant_id 加到所有表。

### 12.5 供应链与退出策略

若未来引入第三方引擎或 DMN runtime，需要评估：

- 许可证与 CVE；
- 规则模型能否导出；
- 版本升级兼容；
- 执行确定性；
- 沙箱能力；
- 指标和 trace 集成；
- 故障时是否可降级；
- 替换成本与双跑验证。

### 12.6 成本

规则成本不仅是 CPU：

- 配置编辑与审批的人力；
- 规则冲突排查；
- 回放和模拟数据；
- 版本保存；
- 审计存储；
- 外部事实查询；
- 缓存与失效；
- on-call 学习成本。

只有当这些成本小于代码发布瓶颈，动态引擎才有真实收益。

---

## 13. 假设与风险账本

| 编号 | 假设 | 当前证据 | 失效影响 | 观察信号 | 复核点 |
| --- | --- | --- | --- | --- | --- |
| A23-01 | 第一批规则以资格 gate 为主 | 路线第 25～26 节 | 若主要是多分支路由，责任链抽象会偏 | 需求出现多策略分支 | 第 25 节 |
| A23-02 | Strategy 当前可视为 create-only 读取配置 | 只有 Create/FindByID，无 Update route | 第 24 节缓存失效模型不足 | 出现更新/发布需求 | 第 24、30 节 |
| A23-03 | 用户资格属于 Participation | 既有上下文地图 | Lottery 可能吞入用户生命周期 | 规则大量依赖用户事实 | 第 25 节 |
| A23-04 | 规则拒绝不产生副作用 | 当前无额度或 Draw | 若判断即占用额度，需要事务重构 | 产品要求“检查即锁定” | 第 25～26 节 |
| A23-05 | 同一 published RuleSet 在执行中不可变 | 版本推导 | 原地修改导致不可解释 | 运营要求即时编辑生效 | 第 28～30 节 |
| A23-06 | 解释可用稳定 reason code 表达 | 当前错误码实践 | 复杂模型可能需要证据图 | 客服无法定位拒绝 | 第 26、29 节 |
| A23-07 | 规则规模不足以证明通用引擎 | 当前零运行规则 | 自研结构可能快速膨胀 | 数十/数百规则与高频变更 | 每阶段复盘 |
| A23-08 | DMN 只作建模参考 | 当前无跨组织交换要求 | 后续迁移成本 | 合规或合作方要求标准模型 | 第 37 节及触发时 |
| A23-09 | 授权与业务资格可共享基础审计但不共享判定 | OWASP 与上下文边界 | 混用产生越权或误拒 | 代码出现 isAdmin 资格分支 | 第 31～35 节 |
| A23-10 | Redis 不缓存高基数用户决策 | 第 24 节只缓存 Strategy | 热点资格可能需要独立缓存设计 | 权威查询成为瓶颈 | 第 25 后单独评审 |
| A23-11 | 当前 fixed weight Selector 可保持不变 | 已有精确测试 | 动态权重/候选过滤可能改变输入 | 真实需求要求实时变权 | 第 27～29 节 |
| A23-12 | 本节文档边界能约束后续代码 | ADR 与 QA 门禁 | 文档漂移 | 新代码绕过所有权矩阵 | 每节 review |

---

## 14. 未来演进问题

### 14.1 第 24 节 Redis

- 缓存当前 Strategy 完整聚合还是专用只读投影；
- key 先使用 cache schema version + StrategyID，还是等待业务配置版本；
- 命中后怎样重新领域恢复；
- corrupt cache 怎样删除并回源；
- Redis 不可用怎样回 MySQL；
- TTL 抖动、穿透、击穿和热 key 用什么真实负载验证；
- 为什么不能缓存当前尚不存在的用户资格和规则结果；
- M1 应准确称为“策略读取与临时选择性能基线”，而不是完整在线抽奖。

### 14.2 第 25 节用户资格

- 第一条规则的主体和权威来源；
- 缺失、过期和依赖不可用语义；
- 资格检查是否只读；
- 资格通过与额度占用是否分开；
- 什么输入可以进入 Lottery；
- 用户拒绝原因公开到什么粒度。

### 14.3 第 26 节责任链

- 至少哪两条前置规则证明共同协议；
- 节点输入输出、顺序、短路和 trace；
- consumer-owned interface 放在哪里；
- 节点能否 I/O，context 如何传播；
- 业务拒绝和技术 error 怎样并存；
- 如何测试拒绝时随机源未调用。

### 14.4 第 27～29 节规则树与引擎

- 什么真实分支让线性链不够；
- 树如何表达 route 而不是所有规则都返回 bool；
- schema 如何保证 root、edge、无环、深度和可达；
- 引擎怎样加载不可变版本；
- 如何限制复杂度；
- 解释轨迹怎样有界；
- 配置更新怎样发布、回滚和缓存；
- 是否需要编译步骤及算法版本。

### 14.5 第 30 节 Activity

- public Activity identity 与内部 StrategyID 如何解耦；
- Activity 引用 Strategy 当前版本还是发布版本；
- 活动发布与策略发布是否同一事务；
- 时间窗、时区、暂停和归档；
- 历史 Draw 怎样解释引用；
- 哪些资源动作进入第 31 节访问控制模型。

### 14.6 第 31～35 节统一访问控制

- Principal、Role、Permission、Resource、Action、Scope 和 PolicyDecision；
- authentication failure、authorization denial 和 business denial；
- 默认拒绝、逐请求服务端强制；
- session capability snapshot 与策略版本；
- 前端导航、路由和操作只消费服务端能力；
- 权限管理页面自身的读取、分配、审批和审计权限；
- 直接 URL、直接 API、隐藏按钮绕过和跨角色 E2E。

---

## 15. 可追溯资料

### 15.1 本节内部资料

- [第 23 节课程正文](../../course/part-03/lesson-23-lottery-strategy-rule-requirements.md)
- [Lottery 规则需求 v1](../../product/lottery-rule-requirements-v1.md)
- [ADR-0019：规则所有权与求值边界](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [第 23 节 API 记录](../../api/lessons/lesson-23.md)
- [第 23 节 QA](../../qa/lessons/lesson-23.md)
- [第 23 节面试问答](../../interview/lessons/lesson-23.md)
- [课程路线修订记录](../../course/route-revisions.md)
- [限界上下文地图](../../product/bounded-context-map-v1.md)
- [非功能需求基线](../../product/non-functional-requirements-v1.md)

### 15.2 当前实现事实

- [Strategy 聚合](../../../internal/lottery/domain/strategy.go)
- [Award 聚合成员](../../../internal/lottery/domain/award.go)
- [WeightedSelector](../../../internal/lottery/domain/weighted_selector.go)
- [Strategy Repository 端口](../../../internal/lottery/application/repository.go)
- [EphemeralSelectionService](../../../internal/lottery/application/ephemeral_selection.go)
- [HTTP Adapter](../../../internal/lottery/adapter/httpapi/selection.go)
- [第 20 节加权选择 ADR](../../decisions/ADR-0017-lottery-weighted-selection.md)
- [第 21 节临时 API ADR](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)

### 15.3 外部一手资料

- [Go：Effective Go](https://go.dev/doc/effective_go?lang=en&version=1)
- [Go：Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Go：Errors are values](https://go.dev/blog/errors-are-values)
- [Go：Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)
- [Go：JSON and Go](https://go.dev/blog/json)
- [OMG：Decision Model and Notation](https://www.omg.org/dmn/)
- [OWASP：Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)
- [OWASP：Input Validation Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)
- [OWASP：Business Logic Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Business_Logic_Security_Cheat_Sheet.html)
- [OWASP：Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)
- [Martin Fowler：Rules Engine](https://martinfowler.com/bliki/RulesEngine.html)
- [Martin Fowler：Business Readable DSL](https://martinfowler.com/bliki/BusinessReadableDSL.html)
- [Martin Fowler：Specification by Example](https://martinfowler.com/bliki/SpecificationByExample.html)
- [Eric Evans：DDD Reference](https://www.domainlanguage.com/ddd/reference/)

---

## 16. 本节最终结论

第 23 节完成后，可以准确地说：

> 我们用一个复合抽奖需求识别出 Activity、Participation、Lottery、Benefit 和 Governance 的事实边界，明确区分规则决策、加权选择与持久副作用，定义业务拒绝、技术失败、授权拒绝和 no_reward 的不同语义，并通过 ADR、追踪矩阵和负向源码 diff 约束后续逐节演进。

不能说：

- 已实现规则引擎；
- 已实现用户资格；
- 已实现责任链或规则树；
- 已完成 DMN 或 DSL；
- 已接入 Redis 规则缓存；
- 已有真实登录、权限和运营后台；
- 已形成正式在线抽奖闭环。

本节最重要的工程判断不是“选择了哪一种规则框架”，而是：

> 在问题还没有具体到足以形成正确接口之前，拒绝把未知复杂度塞进一个看起来通用的包；先让事实所有权、失败语义和版本边界清楚，再让下一条真实需求决定最小代码。
