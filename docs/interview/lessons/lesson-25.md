# 第 25 节面试题：权威注册事实、新用户资格与失败语义

本文面向“业务资格建模 + 事实所有权 + Go application service + 失败关闭 + 渐进式规则系统”类面试。第 25 节已经交付第一个可执行的 Participation domain/application slice：它能根据一份权威注册事实快照和一个具体政策形成 `eligible` 或 `ineligible` 决定，也能把 not-found、stale、unavailable、事实损坏和调用取消保留为不同失败。它还没有生产事实 adapter、登录会话、Activity、公开 HTTP、Lottery 组合、Redis 资格缓存或 Compose 端到端验收；回答时不能把“可调用的内部能力”升级成“线上抽奖已经受资格保护”。

## 60 秒项目自述

> 第 25 节把“新用户才能参与”从可伪造的布尔字段变成第一个可执行 Participation 资格切片。外部用户目录仍拥有注册事实，Participation 只通过 consumer-owned `RegistrationFactReader` 读取最小不可变快照；快照包含 participant reference、registered-at、observed-at、source 和 source revision。具体 rule code 固定为 `participation.new_user.registered_on_or_after`，每份 policy 自身携带 revision 与 inclusive cutoff：早 1ns 是确定不合格，等于或晚于 cutoff 才是合格。
>
> application service 在事实读取后只捕获一次受控服务端时间，先验证主体、未来时间和 freshness，再调用无 I/O 的领域 evaluator。事实充分时返回带 rule/reason、policy revision 和 fact provenance 的具体 decision；not-found、过期、依赖失败和损坏返回零 decision 加分类错误，caller cancellation 则原样返回 `context.Canceled` / `context.DeadlineExceeded`。这里的 fail-closed 是“不允许继续”，不是把未知事实伪装成 ineligible。代码还覆盖了 typed-nil 依赖、错误原因脱敏、取消竞态、64 个并发只读求值和禁止通用 Rule/Engine 的架构停止线。
>
> 本节刻意没有建用户表、没有接现有 ephemeral Lottery API、没有缓存资格，也没有引入责任链。原因是当前没有受控事实同步、可信 Principal、Activity 或第二条具体规则；第 26 节必须先出现第二条真实 Participation 前置规则，再从共同需求反推最小线性短路协议。

## 来源与可信度

- **项目事实：** 只来自当前仓库的 [ADR-0021](../../decisions/ADR-0021-participation-new-user-eligibility.md)、[新用户资格规则基线 v1](../../product/new-user-eligibility-v1.md)、domain/application 实现与测试。代码中的测试只能证明其覆盖意图；最终实际执行状态以[第 25 节 QA](../../qa/lessons/lesson-25.md)为准。
- **官方技术事实：** 主要使用 NIST [SP 800-205](https://www.nist.gov/publications/attribute-considerations-access-control-systems)校准属性权威性和时效，OWASP [Input Validation](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)、[Error Handling](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html)与 [Logging](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)校准服务端信任、错误和隐私边界，OASIS [XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-cos01-en.html)和 OPA [运行故障语义](https://www.openpolicyagent.org/docs/operations)校准“确定拒绝”与“无法决定”的区别，以及 Go 官方的 [`context`](https://pkg.go.dev/context)、[`errors`](https://pkg.go.dev/errors)、[`time`](https://pkg.go.dev/time)、[语言规范](https://go.dev/ref/spec#Interface_types)和 [FAQ](https://go.dev/doc/faq#nil_error)资料。
- **面经题型启发：** 牛客用户发布的[字节后端开发实习一面](https://www.nowcoder.com/feed/main/detail/f5781a0d287c4816862bee438a88072b)、[美团支付日常一面](https://www.nowcoder.com/feed/main/detail/a63ccf3476dd4c07abf0df5d569d2213)、[字节暑期一面](https://www.nowcoder.com/feed/main/detail/3f541e24b94c4765a0744651edd757c9)、[小红书风控引擎二面](https://www.nowcoder.com/feed/main/detail/1271f150bca2411f95f35e775e430080)和[两年社招后端面经](https://www.nowcoder.com/discuss/353158062057922560)是公开个人自述，只说明有人记录过规则引擎、准入所有权、版本、存储、同步/异步和缓存时效追问；真实性未独立核验，不是公司官方题库或技术答案。
- 二次整理帖可能混入作者演绎。例如[“字节后端面经：3 小时极限压榨”](https://www.nowcoder.com/discuss/641577109550288896)正文明确说明回答与配图是演绎，本文不把其中对话当逐字面试记录。外部链接复核日期为 **2026-08-30**。

---

## 1. 为什么新用户资格属于 Participation，而不是 Lottery Strategy 或用户目录？

- **直接回答：** 用户目录拥有“账户何时注册”这一原始事实，Participation 拥有“该事实是否满足本次参与政策”的业务决定，Lottery 只应在所有前置门控已经得到可信结论后执行 Strategy 路由和 Award 选择。事实所有者、决定所有者和终端选择所有者是三个角色；把资格放进 Strategy 会让同一概率配置因用户而变化，把决定放回用户目录又会让外部账户系统理解 GrowthOS 的活动语义。
- **追问：** Participation 不拥有注册事实，为什么 decision 中还保留 fact source/revision？
  - **追问回答：** 这是一次决定使用了哪份受控事实的 provenance，不是取得事实写权限。Participation 不能修改外部账户生命周期，也没有本地用户主表。
- **项目证据：** [ADR 的事实/决定所有权](../../decisions/ADR-0021-participation-new-user-eligibility.md)、[最小快照模型](../../../internal/participation/domain/registration_fact.go)、[决定模型](../../../internal/participation/domain/new_user_eligibility.go)。
- **选型边界：** 若未来组织边界证明 Account 与 Participation 实际属于同一 bounded context，可以合并部署或存储，但概念上的原始事实与场景决定仍应分开；不能因同库就混为同一字段。
- **来源：** 项目事实；NIST [SP 800-205](https://www.nist.gov/publications/attribute-considerations-access-control-systems)强调决策属性的权威、准确和及时可用。

## 2. 什么叫 authoritative registration fact？客户端为什么不能提交 `is_new_user`？

- **直接回答：** authoritative 不是“字段看起来像真的”，而是来源、语义、修订、捕获时刻和主体映射都有受控契约。当前端口只接受 `ParticipantRef` 去查询最小注册快照，生产签名没有 `is_new_user`、客户端 `registered_at`、最终 verdict 或客户端 evaluated-at。客户端可以表达参与意图，但不能同时提交决定问题和答案。
- **追问：** 如果把 `registered_at` 放进签名 JWT，就能直接信任吗？
  - **追问回答：** 只有 issuer、claim 语义、签名校验、最大陈旧、撤销、主体映射和时钟契约都被明确接受后，它才可能成为受控快照。当前项目尚无真实会话，不能把这个未来方案说成已实现。
- **项目证据：** [fact reader 端口](../../../internal/participation/application/ports.go)、[资格基线的权威事实契约](../../product/new-user-eligibility-v1.md)。
- **选型边界：** 对无安全或业务承诺的 UI 预览可以使用客户端提示，但服务端最终决定仍要重算；本节连预览 API 也没有新增。
- **来源：** 项目事实；OWASP [Input Validation Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)要求可信服务层执行语义校验，客户端校验只能改善体验。

## 3. `ParticipantRef` 为什么不叫 `UserID`、`Principal` 或登录用户？

- **直接回答：** `ParticipantRef` 只是 Participation 用来向事实提供者查询主体的非零不透明引用。知道数值 `42` 不证明调用者就是该主体，也不携带租户、角色、session assurance 或数据范围。命名主动阻止调用方把 lookup key 误解为认证证据。
- **追问：** 那当前代码能防止 A 传入 B 的 reference 吗？
  - **追问回答：** 不能。application service 能验证 provider 返回的 snapshot 与请求 reference 一致，但没有可信会话把调用者绑定到 reference。真实身份绑定属于后续会话和公开业务编排，不在第 25 节。
- **项目证据：** [`ParticipantRef` 注释与类型](../../../internal/participation/domain/registration_fact.go)、[主体不匹配测试](../../../internal/participation/application/new_user_eligibility_test.go)、[ADR 的身份停止线](../../decisions/ADR-0021-participation-new-user-eligibility.md)。
- **选型边界：** 第 32 节形成真实 session 后，transport 可以从可信 Principal 派生或映射 ParticipantRef；届时仍不能允许请求体覆盖该绑定。
- **来源：** 项目事实；面试题形态受牛客用户[两年社招后端面经](https://www.nowcoder.com/discuss/353158062057922560)中“第三方准入由谁负责”的个人自述启发，真实性未独立核验。

## 4. 新用户规则的精确定义是什么？为什么 cutoff 采用 inclusive？

- **直接回答：** 固定 rule code 是 `participation.new_user.registered_on_or_after`。`registered_at < cutoff` 为 ineligible；`registered_at == cutoff` 和 `registered_at > cutoff` 为 eligible。inclusive 是明确产品契约，不是由 `Before` API 偶然形成；测试直接覆盖 cutoff 前 1ns、等于和后 1ns，避免边界用户在不同实现里漂移。
- **追问：** “新用户”为什么不是最近 30 天注册、首单用户或首次登录？
  - **追问回答：** 那是其他业务定义。当前 code 只能表示注册时刻下界；改变口径必须形成新 policy revision，若含义根本变化还应创建新 rule code，不能悄悄改同一身份。
- **项目证据：** [policy 类型与固定 rule code](../../../internal/participation/domain/new_user_policy.go)、[cutoff 边界测试](../../../internal/participation/domain/new_user_eligibility_test.go)、[规则基线](../../product/new-user-eligibility-v1.md)。
- **选型边界：** Activity 出现后，cutoff 可能成为 Activity 发布快照的一部分；当前 per-call policy 不代表已经有全局或活动级配置中心。
- **来源：** 项目事实；Go 官方 [`time.Time.Before`](https://pkg.go.dev/time#Time.Before)只提供比较原语，是否含边界仍由业务契约决定。

## 5. 为什么 `evaluatedAt` 由 application clock 在事实读取后只捕获一次？

- **直接回答：** domain evaluator 不读取系统时钟，而接收一个已捕获的服务端 instant。application 在成功读到 fact 后调用 `Clock.Now()` 一次，使 future 检查、freshness 和最终决定共享同一时间切片；如果多个步骤各读一次 `now`，一次请求可能在边界两侧得到自相矛盾的结论，也难以复现测试。
- **追问：** 为什么不是请求一进来就捕获时间？
  - **追问回答：** 当前 freshness 回答“在真正形成决定时，这份已读取快照有多旧”，所以在读取后捕获。若未来正式业务定义要求 request-received-at 作为法律或产品时点，应把两个时刻分别命名并持久化，不能复用一个模糊 `now`。
- **项目证据：** [Clock 端口](../../../internal/participation/application/ports.go)、[application 求值顺序](../../../internal/participation/application/new_user_eligibility.go)、[clock 调用次数测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **选型边界：** 单机 `Clock` 只控制代码语义，不证明跨节点时钟同步质量；多节点正式结果需要 NTP/时钟监控和可接受偏差设计。
- **来源：** 项目事实；Go 官方 [`time`](https://pkg.go.dev/time)资料用于校准 instant、location 与 monotonic reading 行为。

## 6. `ObservedAt` 与 `RegisteredAt` 有什么区别？freshness 怎样计算？

- **直接回答：** `RegisteredAt` 是业务事实“账户何时注册”，`ObservedAt` 是事实提供方何时捕获或观察这份快照。freshness age 使用 `evaluatedAt - observedAt`，不是 `evaluatedAt - registeredAt`；老账户也可以有刚观察到的新鲜快照。age 恰好等于 `maxFactAge` 仍有效，只有严格大于才返回 `ErrRegistrationFactStale`。
- **追问：** 为什么不能把数据库 `updated_at` 直接当 `ObservedAt`？
  - **追问回答：** `updated_at` 可能表示任意字段最后修改，不一定代表这份注册事实何时被权威读取或发布。adapter 必须明确映射契约，不能只因字段类型相同就替代。
- **项目证据：** [快照字段语义](../../../internal/participation/domain/registration_fact.go)、[freshness 实现](../../../internal/participation/application/new_user_eligibility.go)、[等于上限与超 1ns 测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **选型边界：** 当前 max age 是 service 构造参数，不是 Activity policy；不同事实源或风险等级需要不同 freshness 时，应形成显式配置与观测，不能共享一个全局常量。
- **来源：** 项目事实；NIST [SP 800-205](https://www.nist.gov/publications/attribute-considerations-access-control-systems)把及时可用和属性真实性列为决策可信度的一部分。

## 7. 为什么所有时间都规范为 UTC？哪些时间关系会让 fact 无效？

- **直接回答：** 构造器把所有非零 `time.Time` 转为同一 UTC instant，并通过 `Round(0)` 去掉不应跨边界传播的 monotonic reading。不同 location 表示同一 instant 时决定必须相同。快照内部要求 `registeredAt <= observedAt`；application 还要求 `registeredAt <= evaluatedAt` 且 `observedAt <= evaluatedAt`，未来事实属于来源或时钟契约错误，不是业务拒绝。
- **追问：** UTC 能解决夏令时和业务时区所有问题吗？
  - **追问回答：** UTC 解决存储和 instant 比较歧义，不替产品定义“当地自然日”。若规则变为某地区零点或日历周期，policy 必须显式携带 IANA 时区与日历规则，再转换为确定 instant。
- **项目证据：** [时间规范化](../../../internal/participation/domain/registration_fact.go)、[跨时区确定性测试](../../../internal/participation/domain/new_user_eligibility_test.go)、[future fact 测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **选型边界：** 当前只比较一个注册 instant 与一个 cutoff，不包含日历、闰秒业务口径或外部时钟置信区间。
- **来源：** 项目事实；Go 官方 [`time`](https://pkg.go.dev/time)文档。

## 8. 为什么返回具体 `Decision + error`，而不是 `bool`？

- **直接回答：** `false` 无法区分“可信事实证明不合格”和“没找到事实、事实过期、依赖超时或数据损坏”，而这些场景的重试、指标、文案和审计完全不同。当前只有事实充分时才返回非零 `NewUserEligibilityDecision`；无法决定时返回零 decision 和 error。decision 还保留稳定 rule/reason、policy revision 与 fact provenance，便于解释同一次确定结果。
- **追问：** 为什么不把 unavailable 也做成 decision 的第三个 outcome？
  - **追问回答：** 当前 Participation 业务结果只有 eligible/ineligible；技术上无法求值走 Go error 更符合本项目调用边界。XACML 的多态结果只能用于校准概念，不能反过来迫使唯一规则创建跨上下文四态模型。
- **项目证据：** [Decision 类型与 evaluator](../../../internal/participation/domain/new_user_eligibility.go)、[application error 分类](../../../internal/participation/application/errors.go)、[零 decision 失败测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **选型边界：** 若未来协议需要把 pending/manual-review 当成正式 Participation 业务状态，应新增领域结果并定义生命周期；不能用技术 error 假扮人工审核。
- **来源：** 项目事实；OASIS [XACML 3.0](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-cos01-en.html)区分 Deny 与 Indeterminate，但本节不实现 XACML。

## 9. “fail-closed”为什么不等于返回 `ineligible`？

- **直接回答：** fail-closed 描述控制动作：没有可信允许证据时不得进入下游选择。`ineligible` 描述业务事实：有效且足够新鲜的注册事实明确早于 cutoff。依赖失败时系统只能说“无法决定，所以不继续”，不能说“用户不符合”；否则业务拒绝率、重试、客服解释和审计都会被污染。
- **追问：** 对用户来说反正都不能继续，区分有什么价值？
  - **追问回答：** 确定拒绝在同一事实与 policy 下重试没有意义；临时 unavailable 可能稍后恢复并应触发依赖告警。两者也可能映射不同 HTTP 状态与用户文案，但本节尚未定义公开映射。
- **项目证据：** [ADR 的 fail-closed 定义](../../decisions/ADR-0021-participation-new-user-eligibility.md)、[stale/read failure 测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **选型边界：** 只有经过产品和风险评审的非关键增强规则才可能显式 fail-open；当前新用户准入没有这样的授权。
- **来源：** 项目事实；OPA [运行故障语义](https://www.openpolicyagent.org/docs/operations)说明 undefined/未就绪不能被简单当成正常决定。

## 10. fact not found 为什么不是“新用户”，也不是确定不合格？

- **直接回答：** not-found 只说明当前 provider 没有返回该主体的注册快照，可能来自错误 reference、投影延迟、删除、数据损坏或真实不存在。在没有更强 provider contract 前，它是 `ErrRegistrationFactNotFound` 加零 decision；“无记录等于新用户”会让攻击者通过伪造未知 reference 获得准入，“无记录等于老用户”又会把依赖数据问题伪装成业务拒绝。
- **追问：** 未来 provider 明确保证不存在就代表未注册，能改吗？
  - **追问回答：** 可以新增明确契约，但“未注册主体是否允许参与”仍是独立产品规则，不能偷偷映射到当前 `registered_on_or_after` code。
- **项目证据：** [not-found error](../../../internal/participation/application/errors.go)、[read failure 分类测试](../../../internal/participation/application/new_user_eligibility_test.go)、[事实契约](../../product/new-user-eligibility-v1.md)。
- **选型边界：** 本节没有真实 provider adapter，所以没有证明任何外部目录的 404、空行或 tombstone 具体含义。
- **来源：** 项目事实；NIST [SP 800-205](https://www.nist.gov/publications/attribute-considerations-access-control-systems)支持先定义属性来源与 assurance，再消费决定。

## 11. provider error、provider 自身 timeout 和 caller cancellation 怎样分类？

- **直接回答：** adapter 可以用安全 wrapper 标记 not-found、unavailable 或普通 read failure；未知错误默认收敛为 read failure。reader 返回后，service 先检查 caller `ctx.Err()`：若 caller 已取消，原始 `context.Canceled/DeadlineExceeded` 胜出；若 caller 仍存活而 provider 返回 context deadline/cancel，则这是 provider 内部预算失败，分类为 unavailable，同时保留原 cause 供受控诊断。
- **追问：** 为什么不能看到 `context.DeadlineExceeded` 就直接返回 caller timeout？
  - **追问回答：** 同一个 sentinel 可以来自 caller context，也可以来自 adapter 自己的子预算。只检查 error chain 会错误归因；caller context 的真实状态才决定请求是否被调用方取消。
- **项目证据：** [`classifyRegistrationFactReadError`](../../../internal/participation/application/new_user_eligibility.go)、[provider deadline 与 caller cancellation 测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **选型边界：** future adapter 必须遵循 error contract；任意字符串匹配错误类型会破坏 `errors.Is` 和安全渲染。当前 wrapper 还允许 `Unwrap` cause：若违规 cause 携带另一 application sentinel，`errors.Is` 可能同时命中多个 class；真实 adapter 前必须以 contract test 强制 cause 不含语义 sentinel，或改用不参与标准 error chain 的受控诊断通道。
- **来源：** 项目事实；Go 官方 [`context`](https://pkg.go.dev/context)与 [`errors.Is`](https://pkg.go.dev/errors#Is)契约。

## 12. `RegistrationFactReadError` 怎样既保留根因又避免错误泄漏？

- **直接回答：** wrapper 的 `Error()` 只输出经过评审的稳定 class，`Is` 支持按 class 判断，`Unwrap()` 则让可信诊断代码仍可找到底层 cause。这样 SQL、上游地址和原始 payload 不会因普通 `%s` 渲染进入 transport 或日志；未知 class 也收敛为 read failure，而不是回显未经评审文本。
- **追问：** 有 `Unwrap` 后是否已经绝对不会泄漏？
  - **追问回答：** 不是。高权限日志、错误聚合器或 `%+v` 工具若主动遍历 chain，仍可能看到 cause，所以必须在日志边界做数据分级和脱敏。当前没有 HTTP adapter，也不能声称已经完成公网错误映射。
- **项目证据：** [安全 error wrapper](../../../internal/participation/application/errors.go)、[class/cause/渲染测试](../../../internal/participation/application/errors_test.go)、[service 的 secret 反例测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **选型边界：** 若未来统一 error envelope 只保留 correlation ID，可把底层 cause 送入受控 trace；仍不能删除稳定业务/技术分类。当前 `Is + Unwrap` 尚未强制单 class invariant，在风险关闭前 HTTP adapter 不能按任一 `errors.Is` 命中直接映射状态。
- **来源：** 项目事实；OWASP [Error Handling Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html)要求通用外部错误与详细内部诊断分离。

## 13. 为什么 Go interface 的 typed nil 是配置风险？当前怎样防御？

- **直接回答：** 一个 interface 值包含动态类型和动态值；当动态值是 `(*Reader)(nil)` 时，interface 本身不等于 `nil`，直接保存会让服务看似已配置，调用方法时却可能 panic。构造器和 `Validate` 用 `dependencyIsNil` 检查 reader、clock 和 typed-nil `ClockFunc`，拒绝 nil service 与非正 max age；typed-nil `RegistrationFactReadError` 的方法也定义了安全默认分类。
- **追问：** 使用 reflection 是不是过度设计？
  - **追问回答：** 它只位于 composition guard，解决 Go interface 的确定陷阱，不进入每次业务决策的通用反射框架。更强的构造类型或依赖注入生成器出现后可以替换，但不能放弃 typed-nil 测试。
- **项目证据：** [`dependencyIsNil`](../../../internal/participation/application/new_user_eligibility.go)、[typed-nil 依赖测试](../../../internal/participation/application/new_user_eligibility_test.go)、[typed-nil error 测试](../../../internal/participation/application/errors_test.go)。
- **选型边界：** 通过非 nil interface 只证明外层值存在，不证明 adapter 内部连接、配置或并发安全；这些仍由具体 adapter 验证。
- **来源：** 项目事实；Go 官方 [FAQ 的 nil error 说明](https://go.dev/doc/faq#nil_error)和[接口值规范](https://go.dev/ref/spec#Interface_types)。

## 14. application service 为什么在 reader、clock 和 evaluator 后反复检查 `ctx.Err()`？

- **直接回答：** reader 和 clock 是不受控边界，取消可能与返回同时发生；service 在每个边界后先让已经可观察到的 caller cancellation 胜出，避免返回依赖错误或成功决定。pre-canceled 调用不触达 reader，reader 后取消不调用 clock，clock 后取消不返回决定，domain evaluator 后也再做最后检查。
- **追问：** 这样能保证任何时间发生取消都绝不返回成功吗？
  - **追问回答：** 不能建立现实世界的绝对同时性；取消若发生在最终检查之后，调用可能已经返回。当前 evaluator 无副作用，所以没有结果未知问题。reader 若完全不合作，context 也不能强制中断它；blocking 测试必须先让 reader 返回，service 才能观察取消。
- **项目证据：** [Evaluate 控制流](../../../internal/participation/application/new_user_eligibility.go)、[pre/read/clock/blocking reader 取消测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **选型边界：** 未来出现额度扣减或 Draw 写入后，不能靠多次 `ctx.Err()` 解决结果未知；需要幂等业务身份、事务和查询/恢复协议。
- **来源：** 项目事实；Go 官方 [`context`](https://pkg.go.dev/context)说明取消信号需要被调用链协作观察。

## 15. 这个 service 为什么能并发只读求值？64 goroutine 测试又不能证明什么？

- **直接回答：** service 构造后只保存 reader、clock 和正 freshness limit，不在 `Evaluate` 中修改内部状态；policy、fact 和 decision 都是 value。测试让 64 个 goroutine 对同一输入求值得到一致决定，并断言 reader 调用 64 次。真正线程安全还要求注入的 reader 和 clock 自己可并发使用。
- **追问：** 为什么不顺便 singleflight 合并这 64 次事实读取？
  - **追问回答：** 当前没有重复主体流量、provider 成本、可共享 freshness 或取消行为证据。请求合并会改变每 caller deadline、fact age 和 source load 语义，应在真实 adapter/负载出现后单独设计。
- **项目证据：** [immutable service](../../../internal/participation/application/new_user_eligibility.go)、[64 并发测试](../../../internal/participation/application/new_user_eligibility_test.go)。
- **选型边界：** 一次 64 并发单元测试不是吞吐、生产 race 或容量证明；最终竞态执行状态以[第 25 节 QA](../../qa/lessons/lesson-25.md)为准。
- **来源：** 项目事实；Go 官方 [Data Race Detector](https://go.dev/doc/articles/race_detector)用于校准竞态证据边界。

## 16. decision 为什么同时有 rule code、reason code、policy revision 和 fact revision？

- **直接回答：** rule code 标识稳定算法语义；reason code 标识本次确定结果的低基数解释；policy revision 标识使用了哪份 cutoff 配置；fact source/revision 标识使用了哪份来源快照。它们的生命周期不同，不能用 Git SHA、Migration version、Strategy version 或 `updated_at` 互相冒充。
- **追问：** eligible 也需要 reason code 吗？
  - **追问回答：** 需要。`registration_on_or_after_cutoff` 说明这次允许来自哪个明确条件，避免把“没报错”当理由；但它仍只证明本规则允许，不代表完整流程允许或已经中奖。
- **项目证据：** [Decision 字段](../../../internal/participation/domain/new_user_eligibility.go)、[policy/fact 类型](../../../internal/participation/domain/new_user_policy.go)、[决定字段测试](../../../internal/participation/domain/new_user_eligibility_test.go)。
- **选型边界：** 正式 Participation/Draw 需要持久回放时，可能还要关联 Activity/ruleset/Strategy 版本；当前内存 decision 不是审计账本。
- **来源：** 项目事实；AWS Verified Permissions 的 [`IsAuthorized`](https://docs.aws.amazon.com/verifiedpermissions/latest/apireference/API_IsAuthorized.html)响应同时区分决定、决定策略和 evaluation errors，可作为可追溯决定的类比，但本节不是授权系统。

## 17. 哪些字段可以做指标 label？为什么 revision/source 不能直接放进去？

- **直接回答：** 普通指标只应使用经过评审的低基数 `outcome`、固定 `rule_code` 和稳定 `reason_code`。ParticipantRef、FactRevision、PolicyRevision、source 和 evaluated-at 可能随主体、发布或时间无限增长，应进入受控 trace/log 字段而不是 label；否则时间序列数量会膨胀，成本和查询稳定性都会恶化。
- **追问：** source 通常只有几个，为什么也默认不做 label？
  - **追问回答：** 当前没有 provider registry 或允许集合，source 只是受限长度 token，不证明低基数。先建立枚举、容量预算和观测需求，再决定是否提升为 label。
- **项目证据：** [资格基线的指标规则](../../product/new-user-eligibility-v1.md)、[bounded metadata token](../../../internal/participation/domain/registration_fact.go)、[Decision 输出](../../../internal/participation/domain/new_user_eligibility.go)。
- **选型边界：** 本节没有真正 metrics adapter/dashboard；这里只冻结 cardinality contract，不能声称已上线告警。
- **来源：** 项目事实；Prometheus 官方 [instrumentation best practices](https://prometheus.io/docs/practices/instrumentation/#do-not-overuse-labels)要求谨慎使用高基数 label。

## 18. 资格事实和决定怎样做数据最小化？还剩什么隐私风险？

- **直接回答：** fact 只包含 lookup reference、两个时间、source 和 revision，不复制昵称、手机号、邮箱或完整用户画像；decision 进一步省略 ParticipantRef、registered-at、cutoff 和原始 payload，只保留结果及最小 provenance。source 最长 128 bytes、revision 最长 256 bytes，并拒绝空白边界、控制/格式字符和非规范 token。
- **追问：** 长度和可打印校验能保证 revision 不含 PII 吗？
  - **追问回答：** 不能。邮箱和手机号仍然可打印且很短，所以 provider contract 明确禁止把 PII 或原始 payload 编进 revision；日志、trace、保留期与访问审计仍需后续数据治理。
- **项目证据：** [RegistrationFactSnapshot](../../../internal/participation/domain/registration_fact.go)、[Decision 的省略字段](../../../internal/participation/domain/new_user_eligibility.go)、[metadata 反例测试](../../../internal/participation/domain/registration_fact_test.go)。
- **选型边界：** 本节没有真实外部 adapter、持久审计或删除流程，不能声称已经满足某项隐私法规认证。
- **来源：** 项目事实；OWASP [Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)要求对敏感数据记录进行分类、脱敏和访问控制。

## 19. 为什么本节不在 MySQL 新建 `registration_fact` 或 `participation_user` 表？

- **直接回答：** 外部用户目录才拥有账户注册事实；当前没有摄取、幂等、纠正、删除、重放、迟到事件、revision 冲突、保留期和访问审计协议。本地建表只会得到一个看似真实、实际无法解释更新与删除的第二事实源。`RegistrationFactSnapshot` 是一次求值 value，不等于本地 User aggregate。
- **追问：** 没有数据库 adapter，这节算真实实现吗？
  - **追问回答：** 它是可执行、可测试的 domain/application capability，但不是已联通生产事实源的纵向切片。课程的真实演进允许先把模型和 consumer-owned port 做对，再由后续真实 provider 需求增加 adapter。
- **项目证据：** [ADR 的 MySQL 方案评估](../../decisions/ADR-0021-participation-new-user-eligibility.md)、[fact reader port](../../../internal/participation/application/ports.go)、[第 25 节 API 零变化记录](../../api/lessons/lesson-25.md)。
- **选型边界：** 当存在受控 CDC/API 投影、冲突处理、隐私保留和真实消费者时，可以新增 Migration/adapter；必须另立 ADR，不能把 fixture 表称为权威用户库。
- **来源：** 项目事实；AWS [database-per-service pattern](https://docs.aws.amazon.com/prescriptive-guidance/latest/modernization-data-persistence/database-per-service.html)用于校准服务数据所有权与 API 边界。

## 20. 为什么不把资格接到现有 Lottery API，也不新增 demo user header？

- **直接回答：** 当前 ephemeral route 没有可信主体，`X-Demo-User-ID` 或请求体 reference 只证明调用方会填值，不能证明值属于调用方；系统也没有 Activity 把 policy、主体和 Strategy 绑定。贸然接入会制造“看起来有资格控制”的虚假安全闭环，并提前锁定尚无正式 Participation 用例的 HTTP status 和 DTO。
- **追问：** 那现有 Lottery API 是否仍能绕过资格直接选择？
  - **追问回答：** 它继续是 development/test、无用户语义的 ephemeral selection，确实没有资格门控；这不是安全漏洞修复完成，而是公开且受限的课程时间切片。不能把它称为正式抽奖入口。
- **项目证据：** [ADR 的 demo header 方案评估](../../decisions/ADR-0021-participation-new-user-eligibility.md)、[API 零变化记录](../../api/lessons/lesson-25.md)、[架构停止线测试](../../../internal/participation/application/architecture_test.go)。
- **选型边界：** 真实会话、Activity 和业务参与 identity 出现后，公开编排仍要分开认证、授权、业务资格和 Lottery 结果；前端隐藏按钮不能替代服务端执行。
- **来源：** 项目事实；OWASP [Input Validation Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)说明客户端控制可被绕过。

## 21. 为什么新用户 fact 或 decision 不进入第 24 节 Strategy Redis cache？

- **直接回答：** Strategy cache 保存低敏、跨用户共享、可从 MySQL 与领域恢复函数重建的配置投影；用户资格是高基数、带主体、freshness、撤销和隐私语义的决定。复用同一 key、TTL、ACL 或 fail-open 策略会让可丢弃配置优化越界成业务准入风险。本节没有真实 provider 性能证据，所以不缓存 fact，也不缓存一次 decision。
- **追问：** 将来流量大，缓存 fact 还是缓存 decision？
  - **追问回答：** 要先明确权威版本、撤销、新鲜度、policy revision、主体隔离和故障关闭。缓存 decision 还会把 policy/fact 组合固化；通常先评估带 provenance 的受控事实投影，但不能在没有 profile 时预选。
- **项目证据：** [ADR 的缓存停止线](../../decisions/ADR-0021-participation-new-user-eligibility.md)、[第 24 节缓存对象边界](../../decisions/ADR-0020-lottery-strategy-cache-aside.md)、[资格基线](../../product/new-user-eligibility-v1.md)。
- **选型边界：** 真实 provider 延迟、QPS、撤销窗口和数据等级出现后，应单独立缓存 ADR 和故障验收；不能声称当前已解决资格读取性能。
- **来源：** 项目事实；题型受牛客用户[两年社招后端面经](https://www.nowcoder.com/discuss/353158062057922560)中“Redis 是缓存还是唯一存储、临时数据中途过期”的个人自述启发，真实性未独立核验。

## 22. 为什么只有一个具体 evaluator，不先抽 `Rule`、责任链或规则引擎？

- **直接回答：** 一个规则无法证明通用接口应接收什么 context、是否允许 I/O、怎样排序/短路、怎样组合 reason 或处理 cancellation。提前抽象容易把 Eligibility、Authorization 和 Inventory 压成 `map[string]any -> bool`，丢掉各自语言。当前只保留 concrete policy、fact、decision、reader 和 service，架构测试还显式禁止 `Rule`、`RuleChain`、`RuleEngine`、`Specification` 与通用 `EvaluationContext`。
- **追问：** 什么时候第 26 节可以引入链？
  - **追问回答：** 至少第二条真实 Participation 前置规则出现，并且能用测试证明共同顺序、短路、context 和失败语义时，才从两个消费者反推最小线性协议；真实分支、持久配置、运营发布和版本回放再推动后续树/引擎。
- **项目证据：** [架构停止线测试](../../../internal/participation/application/architecture_test.go)、[ADR 的方案五](../../decisions/ADR-0021-participation-new-user-eligibility.md)、[第 26 节演进条件](../../product/new-user-eligibility-v1.md)。
- **选型边界：** 当规则频繁由非研发编辑、需要独立发布/回滚、规则数和交互复杂度已有数据时，应 PoC 比较领域代码、专用引擎和成熟产品，而不是永远拒绝引擎。
- **来源：** 项目事实；Martin Fowler 的 [Rules Engine](https://martinfowler.com/bliki/RulesEngine.html)说明隐式执行和 chaining 的维护成本；Go 官方 [FAQ](https://go.dev/doc/faq)说明接口可以在真实需要出现后补充。面试题形态受牛客用户[美团支付日常一面](https://www.nowcoder.com/feed/main/detail/a63ccf3476dd4c07abf0df5d569d2213)个人自述启发，真实性未独立核验。

## 23. 资格通过为什么不立即扣次数或保证后续一定能抽？怎样看 TOCTOU？

- **直接回答：** 本节 evaluator 和 service 都是只读判断：eligible 只表示该注册事实满足这一条 policy，不占用额度、不创建 Participation/Draw、不锁定 Activity 或 Strategy。决定形成后，事实或额度仍可能变化，因此当前没有解决 check 与 use 之间的 TOCTOU；把一次内存 decision 当长期承诺会产生超额参与或历史无法解释。
- **追问：** 正式抽奖时应该怎样解决？
  - **追问回答：** 需要先有唯一 Participation/Draw identity，再根据事实性质选择同事务重检、版本快照、额度原子扣减/预占、幂等约束或可靠恢复；不是给 `eligible` 加长 TTL。具体方案必须等 Activity、次数账户和正式结果模型出现。
- **项目证据：** [纯 evaluator](../../../internal/participation/domain/new_user_eligibility.go)、[application service](../../../internal/participation/application/new_user_eligibility.go)、[ADR 成本与限制](../../decisions/ADR-0021-participation-new-user-eligibility.md)。
- **选型边界：** 如果未来事实是不可变且决定绑定到不可变发布快照，部分重检可以省略；额度、库存等可变资源仍需要自己的原子承诺边界。
- **来源：** 项目事实；MITRE [CWE-367](https://cwe.mitre.org/data/definitions/367.html)用于校准 check/use 之间状态变化这一通用风险，但本节业务一致性不等同于文件系统漏洞案例。

## 24. 你如何证明这节真的实现了资格能力，又如何避免夸大？

- **直接回答：** 证据分三层：domain tests 覆盖 policy/fact 不变量、UTC、inclusive cutoff、确定性和 fuzz 边界；application tests 覆盖 freshness、not-found/unavailable/unknown error、future/mismatch、typed nil、错误脱敏、取消竞态和 64 并发只读求值；architecture test 检查依赖方向并禁止通用规则类型。最终命令是否通过、race 是否通过以及 runtime negative diff 以 QA 记录为准。
- **追问：** 这些测试能证明真实用户已被保护吗？
  - **追问回答：** 不能。没有生产 fact adapter、认证 Principal、Activity、HTTP 路由、Lottery 组合或浏览器 E2E，也没有 Compose fault injection 和性能基线。本节证明的是内部资格模型与失败语义可执行，不是完整抽奖闭环。
- **项目证据：** [domain evaluator tests](../../../internal/participation/domain/new_user_eligibility_test.go)、[application tests](../../../internal/participation/application/new_user_eligibility_test.go)、[architecture test](../../../internal/participation/application/architecture_test.go)、[第 25 节 QA](../../qa/lessons/lesson-25.md)。
- **选型边界：** 下一步若新增 adapter 或公开消费者，必须补 contract/integration/E2E、权限与故障证据；不能让本节单测替代未来纵向验收。
- **来源：** 项目事实；Go 官方 [Data Race Detector](https://go.dev/doc/articles/race_detector)说明 race run 是动态证据，只覆盖实际执行路径。

## 不能夸大的结论

可以准确说：

- 已建立 Participation 首个 concrete 新用户资格 policy、最小权威事实 snapshot、consumer-owned fact reader 和 application service；
- 已在代码中明确 inclusive cutoff、UTC instant、ObservedAt freshness、一次受控 evaluated-at 和稳定 rule/reason/revision；
- 已把 eligible/ineligible 与 not-found、stale、unavailable、invalid、clock error 和 caller cancellation 分开；
- 已用具体测试覆盖 typed nil、安全错误渲染、取消竞态、并发只读与架构停止线；实际命令状态以 QA 为准。

不能说：

- 已接入真实 Account/User Directory、MySQL 用户表、CDC、JWT、session、Principal、RBAC 或对象级授权；
- 现有 ephemeral Lottery API、React 页面或 Nginx gateway 已执行资格门控；
- 已有 Activity、正式 Participation/Draw、次数扣减、幂等、结果持久化、库存或发奖；
- ParticipantRef 能证明调用者身份，或前端隐藏入口构成安全边界；
- 已缓存资格、实现规则链/规则树/规则引擎、动态 DSL、配置发布或回放；
- 单元测试等于 Compose E2E、生产故障演练、容量基线或业务 SLO。

## 复习清单

- [ ] 能画出 `pre-cancel → fact reader → ctx check → one clock → fact/freshness validation → concrete evaluator → ctx check`；
- [ ] 能解释 User Directory 拥有注册事实、Participation 拥有资格决定、Lottery 拥有终端选择；
- [ ] 能复述 `registered_at < cutoff` 拒绝、等于和大于 cutoff 允许的 inclusive 语义；
- [ ] 能区分 RegisteredAt、ObservedAt、EvaluatedAt，并说清 age 等于 max 仍有效；
- [ ] 能说明为什么 fail-closed 不等于 ineligible，not-found 也不等于新用户；
- [ ] 能解释 caller cancellation 与 provider 自身 deadline 怎样区分；
- [ ] 能解释 typed-nil interface、safe error wrapper 和 `errors.Is/Unwrap` 各自边界；
- [ ] 能列出 decision 包含和刻意省略的字段，并说明指标 label 基数；
- [ ] 能说明为什么现在不建用户表、不接 API、不缓存资格、不抽通用 Rule；
- [ ] 能指出 eligible 不占额度、不创建 Draw，TOCTOU 仍待正式业务身份与原子边界解决；
- [ ] 能把 domain/application/architecture 测试证据与尚无 adapter/E2E/Compose 的限制同时讲清；
- [ ] 能说明第 26 节只有在第二条真实 Participation 规则出现后才引入最小线性链。
