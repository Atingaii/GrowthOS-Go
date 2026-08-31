# 第 30 节面试问答：Activity、不可变发布、exact binding 与回滚

- **课程主题：** 为什么 Strategy 不等于 Activity
- **产品基线：** [Activity 发布与 Lottery 配置精确绑定基线 v1](../../product/activity-publication-binding-v1.md)
- **架构决策：** [ADR-0026](../../decisions/ADR-0026-activity-publication-binding.md)
- **课程正文：** [第 30 节课程](../../course/part-04/lesson-30-strategy-vs-activity.md)
- **QA：** [第 30 节 QA](../../qa/lessons/lesson-30.md)
- **设计手记：** [第 30 节设计手记](../../design-thinking/lessons/lesson-30.md)
- **运行手册：** [Activity publication 验收与故障分诊](../../runbooks/activity-publication.md)
- **资料访问日期：** 2026-08-30
- **题目数量：** 40 个互不重复的精准问题；不预写面试命中率或录用结论

> 本文把三类证据分开：`项目事实`以仓库产品基线、ADR和代码为准；`官方事实`只用于Go/MySQL/平台语义；`个人面经启发`只说明真实求职者曾记录过相近追问方向，不证明题目由某公司官方发布，也不作为技术正确性的依据。

## 1. 资料使用规则

### 1.1 官方/一手技术资料

以下页面正文可直接访问并核验，均于2026-08-30访问：

| 来源 | 只支持的事实 |
| --- | --- |
| [Go `time` package](https://pkg.go.dev/time) | `Time`比较、location/monotonic表示、Timer基础语义 |
| [Go `context` package](https://pkg.go.dev/context) | deadline/cancel传播、derived context与CancelFunc责任 |
| [Go 数据库取消指南](https://go.dev/doc/database/cancel-operations) | 数据库操作传播request context并及时调用cancel |
| [`database/sql.Result`](https://pkg.go.dev/database/sql#Result) | `RowsAffected()`由driver报告且调用可能失败 |
| [MySQL 8.4 CHECK constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html) | CHECK表达式限制，不能跨表/subquery证明业务闭包 |
| [MySQL 8.4 foreign keys](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html) | FK、索引与referential action语义 |
| [MySQL 8.4 consistent reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html) | InnoDB consistent nonlocking read与snapshot |
| [MySQL 8.4 locking reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-locking-reads.html) | locking read适用场景与语义 |
| [Microsoft DDD domain model](https://learn.microsoft.com/en-us/dotnet/architecture/microservices/microservice-ddd-cqrs-patterns/microservice-domain-model) | 复杂domain rules/invariants与bounded-context语言 |
| [Microsoft domain analysis](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/domain-analysis) | 业务能力、子域与bounded context分析方法 |
| [Apollo 配置中心介绍](https://github.com/apolloconfig/apollo/wiki/Apollo%E9%85%8D%E7%BD%AE%E4%B8%AD%E5%BF%83%E4%BB%8B%E7%BB%8D) | Apollo公开介绍的配置发布/客户端更新能力 |
| [Apollo 使用指南](https://github.com/apolloconfig/apollo/wiki/Apollo%E4%BD%BF%E7%94%A8%E6%8C%87%E5%8D%97/a6611f0bef63986aebe02bf37502999933f3efeb) | Apollo文档中的发布历史与回滚操作 |
| [AWS AppConfig概览](https://docs.aws.amazon.com/appconfig/latest/userguide/what-is-appconfig.html) | AppConfig官方描述的validation/deployment/monitoring能力 |
| [AWS AppConfig deployment strategy](https://docs.aws.amazon.com/appconfig/latest/userguide/appconfig-creating-deployment-strategy.html) | deployment strategy的时间、增长与bake配置 |
| [AWS AppConfig revert](https://docs.aws.amazon.com/appconfig/latest/userguide/appconfig-deploying-reverting.html) | 官方revert deployment操作语义 |

这些资料不证明GrowthOS采用Apollo/AWS，也不替代本项目ADR。

### 1.2 牛客个人面经

下列页面在访问日可以读取完整正文，但都是用户自述，无法独立核验提问公司、轮次、逐字原题或答案质量。只用于发现追问形态：

| 个人帖子 | 正文可核验程度 | 只启发什么 |
| --- | --- | --- |
| [疯狂游戏后端开发一面10.30](https://www.nowcoder.com/discuss/813508587199688704?sourceSSR=enterprise) | 完整正文可读；个人记录，未核验 | 活动灰度、配置填写/复核/发布/回滚/历史、ZSet定时精度 |
| [0330美团后端开发一面](https://www.nowcoder.com/discuss/471056135512936448) | 完整正文可读；个人记录，未核验 | 项目状态迁移与状态机追问 |
| [社招面经-后端开发](https://www.nowcoder.com/discuss/353155586684559360) | 完整正文可读；个人记录，未核验 | 自研配置中心表设计、实时推送、配置依赖 |
| [字节后端日常实习一二三面面经（已发offer）](https://ac.nowcoder.com/discuss/953988?type=2) | 完整正文可读；个人记录，未核验 | 帖子中记录的配置中心相关追问方向 |
| [面经分享：滴滴和涂鸦智能金三银四面经](https://www.nowcoder.com/discuss/353157979690180608) | 完整正文可读；个人记录，未核验 | Apollo原理和ZK watch类追问 |
| [Java后端 快手 效率工程 一面面经](https://www.nowcoder.com/discuss/353158756638859264) | 完整正文可读；个人记录，未核验 | 定时任务分发与只执行一次 |
| [腾讯 Java 后端一面 8/19](https://www.nowcoder.com/discuss/353158293965185024) | 完整正文可读；个人记录，未核验 | 多实例定时重复、锁与timer |
| [字节跳动-生活服务后端开发一面](https://www.nowcoder.com/discuss/682693768398548992) | 完整正文可读；个人记录，未核验 | 定时任务实现与幂等追问 |
| [沥泉科技 Golang 后端一面面经](https://www.nowcoder.com/discuss/703571718387847168) | 完整正文可读；个人记录，未核验 | DDD四层和项目落地 |
| [长沙秋招面经合集](https://www.nowcoder.com/discuss/556438629988450304) | 完整正文可读；个人记录，未核验 | DDD与三层、对象生命周期和领域拆分 |
| [阿里、百度、腾讯、某易、好未来、端点等面经](https://www.nowcoder.com/discuss/353158296846671872) | 完整正文可读；个人汇总，未核验 | 线上版本回滚、owner 与技术选型理由 |
| [自己的春招求职全记录](https://www.nowcoder.com/discuss/353156492373204992) | 完整正文可读；个人复盘，未核验 | “线上bug只答版本回滚”不够，需要完整处置链 |

后文提到“面经常见追问”时只表示这些帖子启发了题型，绝不表示企业官方题库。

## 2. 精准问答

### Q1：一句话说清第30节解决了什么问题？

**答（项目事实）：** 第29节只能执行调用方给出的exact graph；第30节新增Marketing-owned Activity publication，让系统能确定“哪个Activity在一个受控服务端时刻，对新请求采用哪份exact graph与全部terminal Strategy snapshot”，并以不可变历史、CAS和事务防止漂移与并发覆盖。

**追问边界：** 这不等于正式抽奖链已完成。Gate active不代表授权、参与资格、Strategy已加载、Award已选、库存可用或权益已发。

### Q2：为什么 Strategy 不等于 Activity？

**答（项目事实）：** 两者变化原因和owner不同。Activity由运营排期、发布、回滚、退役驱动；Strategy由Award/Weight/outcome配置变化驱动。把时间窗和生命周期放进Strategy会破坏复用，让Lottery cache/selector理解Marketing治理，并使Activity回滚变成修改Lottery aggregate。

**加分点：** Bounded context不是为了目录好看，而是保护独立语言、所有权和变化节奏。DDD资料只提供分析方法，不强制拆成网络微服务。

### Q3：为什么 Marketing 只保存 Lottery primitive ref，不直接 import Lottery domain？

**答（项目事实）：** Marketing需要表达自身publication，但不应在domain/application层依赖provider类型。它保存bounded local `(graphID, revision)`和`(strategyID, revision)`；Marketing-owned ACL adapter负责转换、exact读取和重新验证。这样provider模型变化不会渗入Marketing核心。

**追问：** primitive ref不是弱类型字符串堆。它仍有构造、grammar、limit和Validate，并且只有ACL可以把它翻译成Lottery identity。

### Q4：为什么 StrategyID 不足以支持活动回滚？

**答（项目事实）：** StrategyID只标识family，不标识当时Award/Weight内容。按ID读取会随当前数据变化，历史publication无法复现。Exact snapshot用`(StrategyID, StrategyRevision)`绑定完整immutable aggregate。

**反例：** `updated_at`也不是revision；Award子行变化、数据库精度和物理写时间都不能充当受控业务identity。

### Q5：Strategy snapshot 为什么是完整快照，而不是只存content hash？

**答（项目事实）：** Hash只能检测内容是否不同，不能在原内容被覆盖/删除后恢复Award集合。完整snapshot才能exact read、回滚和审计。当前revision只是registry-bound token，没有canonical content encoding或签名，因此不能宣称防篡改hash。

### Q6：为什么不把现有 by-ID Strategy repository 全部改成versioned？

**答（项目事实）：** 本节需求只要求Activity publication获得exact content。全面替换会同时改变旧Redis projection、ephemeral selection和API语义，扩大兼容风险。采用并行create-only snapshot端口，让旧路径保持不变，后续迁移另立章节。

### Q7：为什么业务version、state generation、schema version必须分开？

**答（项目事实）：** Publication version标识一次历史发布；state generation用于CAS；schema version选择持久形状parser。Published时state和active数值碰巧相等，但retire只推进state、不追加publication，证明语义不同。Schema升级更不应伪装运营发布。

### Q8：Activity为什么只有draft/published/retired三态？

**答（项目事实）：** 这是当前需求的最小持久生命周期。Scheduled/active/ended由`[start,end)`和Clock计算；把它们持久化会产生一个随时间自动过期、需要任务修正的第二真相。Pause、gray、archive没有冻结语义，不提前加入。

**面经启发：** 个人面经中会追问状态机，但回答重点应是不变量与非法迁移，而不是背状态机模式。

### Q9：draft为什么允许`stateVersion=0`？

**答（项目事实）：** Draft尚未发生publication或retire transition，generation 0是自然初始值。First publish从active 0生成v1。若把zero一律判invalid，创建draft时会被迫伪造版本。

### Q10：为什么retired保留最后active publication pointer？

**答（项目事实）：** Retire停止新gate但不能删除历史解释。保留pointer可定位退役前最后生效配置和rollback链。Gate优先返回confirmed retired，保留pointer不表示配置继续开放。

### Q11：publication为什么append-only？

**答（项目事实）：** Release/rollback是已发生事实。UPDATE会让审批对象漂移、历史不可解释、事故版本可被擦除、回滚没有动作identity。Append-only以`(ActivityID, version)`给每次动作稳定identity，header只选择active。

### Q12：普通publish和rollback如何用一个record表达，又避免混合状态？

**答（项目事实）：** 使用discriminated union：release必须`rollbackOf`为空；rollback必须`0 < rollbackOf < version`。Domain strict validation和MySQL CHECK都验证组合，不允许“release带source”或“rollback无source”。

### Q13：为什么rollback不是把active pointer直接指回旧v1？

**答（项目事实）：** 直接指回无法区分原始v1时期和事故后恢复时期，也没有新publishedAt和approval evidence。正确做法是active v3时追加rollback v4，`rollbackOf=v1`，复制v1内容但记录本次动作。

**面经启发：** 个人复盘表明只答“版本回滚”容易被继续追问；需要说明历史、审批、并发、read-back和副作用。

### Q14：rollback复制哪些字段，哪些必须新生成？

**答（项目事实）：** 精确复制target graph、canonical Strategy manifest、startsAt和endsAt；新生成next version、kind rollback、rollbackOf、publishedAt和approval evidence。Caller只提交ActivityID、expected state和target version，不能自己改source内容。

### Q15：为什么不能回滚到已经结束的source window？

**答（项目事实）：** 本次publishedAt若已到source exclusive end，新active会立即ended，无法恢复可参与配置。若延长endsAt，那已不是恢复exact source，而是新release，必须重新形成candidate与审批。

### Q16：回滚能撤销已经发出的奖品吗？

**答（项目事实）：** 不能。Publication rollback只影响commit后重新resolve的请求，不撤销已形成的Participation、route decision、未来Draw、库存扣减、权益或消息。副作用补偿需要独立ledger/saga与幂等设计，本节没有实现。

### Q17：为什么publication必须绑定graph全部terminal Strategy，而不是只绑高流量路径？

**答（项目事实）：** 不同请求可能走不同branch。若只绑常见terminal，冷门branch会在运行时隐式读取current/latest Strategy，导致同一publication混版。发布单位必须对整份graph闭合。

### Q18：如何证明manifest是闭合集？

**答（项目事实）：** ACL exact读取并Validate graph，收集unique terminal StrategyID set，与manifest StrategyID set做完全相等；然后逐条exact读取Strategy snapshot并核对identity/Validate。Missing、extra、duplicate、zero、not-found或wrong identity全部失败。

### Q19：manifest为什么canonical排序且最多128条？

**答（项目事实）：** Canonical排序让同一集合有唯一表达、approval candidate比较稳定且不依赖map/SQL顺序。128是v1复杂度预算，限制验证、存储和resolve exact reads，防止无界配置成为资源耗尽入口；不是生产容量结论。

### Q20：为什么publish和resolve都要验证Lottery exact refs？

**答（项目事实）：** Publish验证阻止正常写路径提交坏ref；resolve重验防DBA/migrator旁路、历史bug、错误恢复或provider损坏。跨上下文无FK时，两次fail-closed验证共同保护gate。Resolve不fallback previous/latest，否则会自行选择未批准配置。

### Q21：为什么时间窗采用`[start,end)`？

**答（项目事实）：** Start inclusive、end exclusive给边界唯一归属：`start`立即active，`end`立即ended。相邻`[a,b)`和`[b,c)`不重叠，避免“恰好b算哪场”的歧义。测试必须覆盖start-1µs、start、end-1µs、end。

### Q22：为什么统一成UTC微秒？

**答（项目事实 + 官方事实）：** MySQL使用`DATETIME(6)`，持久精度是微秒。Domain把instant转为UTC微秒并去除Go monotonic component，使内存值和SQL往返一致。Go `time`文档提醒`Time`包含location/可能的monotonic reading，持久/比较语义不能依赖结构体`==`。

**边界：** 用户本地timezone和DST输入转换属于未来API adapter，domain不猜。

### Q23：为什么需要业务时刻的 operation 只读取一次 Clock？

**答（项目事实）：** Publish/rollback/retire/resolve 需要业务时刻，多次读 Clock 可能跨 start/end，使“candidate校验时未结束、写入时已结束”，或 status 与 evaluatedAt 不一致。一次注入 Clock 给这些操作唯一业务 instant，也让边界测试确定化；CreateDraft 不需要时刻，因此不读 Clock。

### Q24：为什么不使用定时任务到点把Activity状态改成active/ended？

**答（项目事实 + 官方事实）：** 当前业务真相是request-time `now`与window比较，不需要额外持久状态。定时器/调度任务可能延迟、重复或在重启时漏执行，还需job identity、lease和幂等。Go `time`文档中的timer只提供运行时定时能力，不能自动成为持久业务事实。

**面经启发：** 多份个人面经追问多实例定时重复与只执行一次；正确回答不是“加分布式锁”四个字，而是先判断是否真的需要任务。本节不需要。

### Q25：`scheduled/active/ended/retired`为什么是confirmed decision，不是error？

**答（项目事实）：** 它们是输入可信后形成的业务结果。Error用于输入/依赖不可信。把正常ended当error会污染告警；把DB损坏当ended则会掩盖事故。因此业务decision与technical failure使用不同返回协议。

### Q26：Draft gate为什么返回`not_published`而不是not-found？

**答（项目事实）：** Activity root存在、shape合法，只是尚无publication，这是确认的业务状态。Not-found表示root不存在；published却缺active record则是技术损坏。三者必须可区分。

### Q27：CAS具体防止什么，不能防止什么？

**答（项目事实）：** CAS用expected lifecycle/state/active防止stale writer覆盖已提交赢家，保证同一generation最多一个transition成功。它不自动提供请求幂等、跨Activity事务、审批唯一性、commit结果判定或副作用补偿。

### Q28：为什么选择optimistic CAS而不是Redis锁或长事务`SELECT ... FOR UPDATE`？

**答（项目事实 + 官方事实）：** Graph/approval验证可能跨网络，持锁会扩大事务和阻塞。Redis锁增加lease/分区/双重权威，仍不能替代MySQL unique、transaction和CAS。MySQL官方区分consistent read与locking read；本节在外部验证后用短authoritative transaction检测冲突。

**何时重评：** 如果真实冲突率、starvation或复杂多row invariant有证据，再比较pessimistic locking；现在不预设。

### Q29：为什么必须检查`RowsAffected()==1`？

**答（项目事实 + 官方事实）：** 0表示expected predicate没命中，属于state conflict；大于1违背Activity identity假设。`database/sql.Result`官方API允许`RowsAffected()`返回数值或error，adapter必须同时处理调用失败，不能把Exec无error直接视为CAS成功。

### Q30：为什么publication、bindings和root header必须一个事务？

**答（项目事实）：** 它们共同形成一个可解析配置。分开写会出现active指向缺record、record缺binding或orphan history。事务内先insert record/bindings再CAS root，CAS失败rollback全部，COMMIT后一次可见。

### Q31：Current read为什么用一个read-only REPEATABLE READ snapshot？

**答（项目事实 + 官方事实）：** Resolve必须读取同一时点的root、root指定的exact publication与manifest。若先读root v2、并发发布v3后再查latest，可能拼出v2 header + v3 content。InnoDB consistent read官方语义支撑同一transaction snapshot；业务上仍必须按exact active pointer读取，RR不会自动纠正错误SQL。

### Q32：MySQL CHECK、FK、domain Validate和ACL各负责哪层完整性？

**答（项目事实 + 官方事实）：** CHECK保护单行shape，如kind union、window和root状态；FK保护同owner持久引用，如publication->Activity、binding->publication；domain保护构造/恢复和跨字段不变量；ACL读取provider authority证明terminal/manifest closure。MySQL CHECK官方限制决定它不能用subquery跨表证明闭包。

### Q33：为什么Marketing内部建FK，却不对Lottery表建跨上下文FK？

**答（项目事实）：** Marketing root/publication/binding共享生命周期、事务和部署，内部FK准确表达所有权。跨到Lottery会固化provider表名/复合键/同库假设，仍不能证明闭合集。外部完整性由exact verifier与resolve fail closed承担。

**权衡：** 没跨context FK不是“最终一致性自动正确”，而是明确接受DBA旁路风险，并用最小权限、strict restore和运行停止线补偿。

### Q34：什么是commit outcome unknown，为什么不能自动重试？

**答（项目事实）：** COMMIT时断线，数据库可能已durable但响应丢失，也可能未提交。两种世界对caller都是error。若已提交再自动重放，可能追加另一个publication，所以业务返回值仍必须是zero value + error。只有class精确为`ErrCommitOutcomeUnknown`且caller/operation context均仍存活时，可信内部流程才能通过`ActivityCommitReceiptFromError`显式提取防御复制的exact receipt；普通retryable/context/storage failure没有receipt，receipt也不进入`Error()`、`errors.Is`或unwrap链。Rollback receipt还会验证`rollbackOf < before.active`，防止把领域上合法但不属于该attempt的record当作恢复凭据。

Publish/rollback用新健康连接读取同一RR current snapshot并构造`ObserveCurrentActivity`；retire exact读root并构造`ObserveActivityRoot`。`ReconcileActivityCommit`纯函数只返回`committed/not_committed/indeterminate`：exact after root和完整publication命中才算committed；exact before或同一next generation的另一合法winner算not_committed；缺失、损坏、partial、identity不符或已推进到更晚generation都算indeterminate。它不做I/O，也不建议retry。

**追问：** 为什么“已推进到更晚generation”不是not_committed？因为本次写可能先提交，随后又被另一合法动作推进；当前状态无法证明中间历史。Fail closed为indeterminate才能避免凭猜测重放。

### Q35：Approval和Authorization有什么区别？

**答（项目事实）：** Approval针对exact candidate，回答配置是否经治理流程批准；Authorization针对Principal+Resource+Action+Scope，回答caller是否有权执行。Evidence reference既不是session也不是permission token。第30节没有真实approval adapter、Principal或RBAC，所以服务必须未装配。

### Q36：为什么publish在approval前后做两次pure planning？

**答（项目事实）：** 第一次用内部临时evidence形成exact candidate供Lottery/approval验证；approval返回真实evidence后重新规划，并逐字段证明version/window/graph/manifest/publishedAt未漂移。Publication私有不可变字段不提供“只换evidence”的mutation入口。

**关键细节：** `application-plan`不能传给外部或落库，只是建立provisional immutable value所需的内部token。

### Q37：Context错误优先级为什么是caller > internal deadline > provider class？

**答（项目事实 + 官方事实）：** Caller已取消时，稍后到达的SQL/provider error不应覆盖真实控制原因；只有caller仍live且private deadline先到，才归类internal timeout。Go `context`官方强调cancel/deadline沿调用链传播；本节在每个dependency boundary后重新检查归属。

**限制：** Context是协作式取消，不会强制抢占忽略context的依赖；maxDuration也不是生产SLO。

### Q38：为什么error wrapper不使用普通`Unwrap()`暴露底层cause？

**答（项目事实）：** Activity/graph/Strategy/approval/SQL细节可能被未来HTTP或日志意外展开。本节新增的 Marketing operation/dependency/repository wrapper 的 `Error()`/`errors.Is` 只暴露 reviewed stable class，可信诊断代码显式 `Cause()` 才能查看原始 cause。这不改写既有 Lottery repository wrapper，也不代表 HTTP status mapping 已完成。

### Q39：为什么本节不接API、Redis、RabbitMQ或前端？

**答（项目事实）：** API需要认证授权、对象scope、idempotency、error disclosure和浏览器安全；Redis需要projection freshness与invalidation协议；MQ需要event schema、outbox、consumer/replay；前端需要capability projection与服务端RBAC。当前核心问题能由domain+MySQL+ACL解决，提前接入会增加无法验收的契约。

### Q40：如果面试官问“为什么不直接用Apollo/AWS AppConfig”，如何回答？

**答（项目事实 + 官方资料对照）：** Apollo/AWS AppConfig官方资料展示了配置发布、历史/回滚、deployment strategy、monitor/revert等平台能力，适合配置分发与部署治理；但本项目仍需定义Activity lifecycle、exact graph+Strategy closure、业务审批对象、CAS和Draw边界。平台不能替代领域identity。

当前选择是先实现最小authoritative kernel，不代表永远自研。若未来出现多环境promotion、canary、推送、海量client或平台治理需求，应比较：

- 平台version怎样映射Activity publication identity；
- exact Lottery artifact能否不可变promotion；
- rollback是平台deployment revert还是业务新publication；
- approval/RBAC/audit谁负责；
- vendor outage与read path怎样fail closed。

**面经启发：** 个人面经会追问“为什么选这个而不是那个”“配置如何实时一致”；回答应先声明需求和owner，再比较机制，不能只背Apollo/ZK名词。

## 3. 进一步追问题库

在40问基础上，面试官还可能沿这些维度深入，但它们属于后续设计而非本节完成项：

1. 如何设计publish request idempotency key与result registry？
2. 多租户下ActivityID、publication version和scope怎样组合？
3. 双人审批怎样保证publisher与approver职责分离？
4. 灰度发布如何把“唯一active”演进成audience-specific resolution？
5. 多区域写入怎样选择leader和冲突策略？
6. 正式Draw怎样保存Activity/graph/Strategy/fact exact provenance？
7. Retire后已resolve未Draw的请求是否需要再次检查？
8. Commit unknown自动化read-back怎样避免越权和信息泄露？
9. Publication历史保留多久，如何做legal hold与tamper evidence？
10. 配置平台接入后，谁是最终authoritative version owner？

回答这些问题时要明确“设计方向”与“当前实现”之间的界线。

## 4. 面试表达模板

可以用以下顺序在两分钟内讲清项目：

```text
问题：第29节能执行exact graph，却没有Activity发布选择与Strategy内容版本。

边界：Marketing owns Activity；Lottery owns graph/Strategy snapshots；
      approval与authorization独立。

模型：draft/published/retired root + append-only publication；
      exact graph ref + complete terminal Strategy revision manifest；
      UTC [start,end) gate。

一致性：publish/rollback append new version，DB CAS防stale overwrite；
        publication/bindings/header同事务；current用exact RR snapshot。

可靠性：publish与resolve均fail-closed验证；commit unknown exact read-back；
        confirmed business decision与technical error分开。

停止线：没有API、真实审批、认证RBAC、UI、正式Draw、库存、MQ或灰度。
```

这套表达避免把未实现能力包装成“生产系统”，也能让面试官继续追问真正有证据的技术权衡。
