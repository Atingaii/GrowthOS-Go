# 第 30 节设计手记：从第一性原则推导 Activity publication

- **课程主题：** 为什么 Strategy 不等于 Activity
- **产品基线：** [Activity 发布与 Lottery 配置精确绑定基线 v1](../../product/activity-publication-binding-v1.md)
- **架构决策：** [ADR-0026](../../decisions/ADR-0026-activity-publication-binding.md)
- **课程正文：** [第 30 节课程](../../course/part-04/lesson-30-strategy-vs-activity.md)
- **API 记录：** [第 30 节 API](../../api/lessons/lesson-30.md)
- **QA：** [第 30 节 QA](../../qa/lessons/lesson-30.md)
- **面试问答：** [第 30 节面试问答](../../interview/lessons/lesson-30.md)
- **运行手册：** [Activity publication 验收与故障分诊](../../runbooks/activity-publication.md)
- **设计日期：** 2026-08-30
- **证据纪律：** 只记录真实执行的冻结前候选证据并明确停止线；不预写最终测试数字、提交或生产结论

> 这不是“先决定用状态机、DDD、版本表，再找理由”的倒推文档。思考顺序是：先问系统必须保存哪些不可逆事实、谁拥有这些事实、并发和失败会破坏什么，再选择最小模型与机制。

## 1. 第一性原则方法

面对“做一个活动发布功能”，最容易犯的错误是从页面开始：建一张活动表、加一个状态字段、做几个按钮。这样得到的是可操作页面，不一定得到可解释系统。

本节使用六个连续问题：

1. **业务事实是什么？** 哪些事实一旦发生就不能靠覆写消失？
2. **事实由谁拥有？** 谁能定义其合法性、生命周期和变化原因？
3. **什么必须保持不变？** 哪些不变量一旦被破坏会导致历史无法解释或请求混版？
4. **失败发生在哪里？** 并发、网络、数据库、Clock、跨上下文引用各自怎样失败？
5. **最小机制是什么？** 能否用不可变记录、exact identity、CAS和事务解决，而不引入锁服务、MQ或通用工作流？
6. **现在不能承诺什么？** 认证授权、审批系统、灰度、正式Draw等缺少证据时必须留在停止线外。

把答案压缩成一句话：

> 对新请求而言，某个 Activity 在某个服务端时刻是否开放，必须由一个可追溯且并发安全的 active publication 决定；该 publication 必须精确指向一套闭合、不可变、可恢复的 Lottery 配置。

## 2. 从业务现象还原最小事实

设想运营说：

> 9 月 1 日零点开启“新用户抽奖”，使用图 G 的第 7 版，图里两个 Strategy 分别使用 S1:r3 与 S2:r5；如果上线后发现配置错误，恢复到之前版本。

这句话至少包含七个独立事实：

1. 稳定的 Activity identity；
2. Activity 自己的生命周期；
3. 一次发布动作形成的历史版本；
4. 发布生效的半开时间窗；
5. exact graph identity；
6. graph 所有 terminal Strategy 的 exact content identity；
7. 谁/什么流程批准了这次 exact candidate 的 evidence pointer。

如果只保存：

```text
activity.current_strategy_id = 42
```

系统无法回答：当时 graph 是哪一版、Strategy 42 的 Award/Weight 是哪一版、另一个 terminal 是否漏绑、何时发布、谁批准、回滚前是什么以及并发覆盖了谁。

所以“活动表加StrategyID”不是简化，而是丢失了问题本身要求的事实。

## 3. 先区分三种时间

发布系统里常混淆三种时间：

| 时间 | 本节字段 | 回答的问题 |
| --- | --- | --- |
| 业务窗口 | `startsAt` / `endsAt` | 哪个服务端时刻允许新请求继续 |
| 发布事实时间 | `publishedAt` | 这次release/rollback动作何时形成 |
| 持久化元数据 | `created_at` / `updated_at` | 数据库行何时写入或更新 |

它们不能互相代替：

- `created_at` 不是运营窗口；
- `startsAt` 不是审批/发布发生时间；
- `updated_at` 不是受控业务revision；
- rollback复制source窗口，但必须有新的`publishedAt`。

这个区分直接导出独立字段和独立校验。

## 4. 谁拥有什么：按变化原因而不是按页面分组

### 4.1 Marketing owns Activity

Activity回答：

- 为什么有这场活动；
- 它处于draft、published还是retired；
- 哪个publication对新解析生效；
- 何时开放、结束；
- 什么时候被terminal retire。

这些变化来自运营发布和生命周期，不来自Award/Weight配置本身。因此Activity属于Marketing。

### 4.2 Lottery owns graph 与 Strategy snapshot

Lottery回答：

- 已确认事实走哪条branch；
- terminal指向哪个Strategy family；
- exact Strategy里有哪些Award、weight和outcome；
- graph/snapshot怎样构造、验证和严格恢复。

这些变化来自抽奖规则和候选集合。因此graph与Strategy snapshot继续属于Lottery。

### 4.3 Governance owns approval；Access Control owns authorization

Approval回答“这份exact candidate是否经过可信流程批准”；Authorization回答“当前Principal是否可以执行动作”。二者不是一个问题：

```text
authorized publisher + unapproved candidate -> reject
approved candidate + unauthorized caller   -> reject
```

本节只有consumer-owned `ApprovalVerifier` port，没有真实adapter；也没有Principal/RBAC。Evidence reference只是指针，不能冒充session或permission。

### 4.4 为什么不用“都放Marketing里”

把Lottery对象复制进Marketing会产生双主：

- 哪边负责校验Award/Weight？
- 两份Strategy何时同步？
- graph schema升级谁负责？
- 历史Marketing副本与Lottery authority不一致时信谁？

本节选择primitive exact references + Marketing-owned ACL adapter。引用承认所有权，ACL负责语言翻译和fail-closed验证。

## 5. 统一语言先于类图

“版本”是本节最危险的歧义词。必须建立版本台账：

| 名称 | 实质 | 推进者 | 是否内容identity |
| --- | --- | --- | ---: |
| Activity state version | root CAS generation | publish/rollback/retire | 否 |
| Activity publication version | append-only历史identity | publish/rollback | 是 |
| graph revision | exact topology identity | Lottery graph owner | 是 |
| Strategy revision | exact Award/Weight snapshot identity | Lottery Strategy owner | 是 |
| schema version | persisted shape parser选择 | code/Migration | 否 |
| Migration version | DDL演进顺序 | migrator | 否 |
| cache codec version | projection bytes格式 | cache adapter | 否 |

即使published状态下`stateVersion == activePublicationVersion`，也不能把概念合并。Retire只推进stateVersion、不追加publication，立即证明两者语义不同。

## 6. 从不变量推导 Activity root

### 6.1 为什么只有三种 lifecycle

当前真实需求只有：

```text
draft -> published -> retired
          |      |
          + publish/rollback新版本后仍published
```

因此只建：

- `draft`：未有publication；
- `published`：有唯一active publication；
- `retired`：terminal停止新gate，但保留最后active pointer供历史解释。

`scheduled/active/ended`由Clock与window计算，不是持久状态。Pause、cancel、archive、gray不存在已冻结语义，不能因为“以后可能有”就先塞进枚举。

### 6.2 状态形状比字段非空更强

合法shape：

```text
draft:
  state=0, active=none, retirement=none

published:
  state=active>0, retirement=none

retired:
  active>0, state=active+1, retirement=(at, evidence)
```

这让单个root就能检测很多损坏：draft带active、published的state/active不一致、retired缺证据都不能进入业务逻辑。

### 6.3 为什么draft generation是0

Draft从未发生publication transition，所以generation 0是自然起点。把0一律当invalid会迫使创建draft时伪造一次状态变化，也会让first publication不是v1。

## 7. 从历史可解释性推导 immutable publication

一份publication是一次已经发生的release/rollback事实。若允许UPDATE：

- 审批的candidate会被事后换掉；
- gate结果无法关联当时内容；
- 事故版本可以被擦除；
- rollback等同于覆盖，无法回答“何时回滚”；
- cache/audit无法使用稳定identity。

所以publication采用append-only numeric version：

```text
(ActivityID, ActivityPublicationVersion)
```

Header只保存哪个版本active，内容留在不可变record与bindings中。

### 7.1 为什么numeric version由服务端推进

客户端不能决定next version，否则两个caller可能跳号、重用或猜测历史。Domain从current active安全计算`active+1`并拒绝溢出；Repository用expected root状态决定谁真正提交。

### 7.2 为什么publication有独立schema version

业务version回答“第几次发布”，schema version回答“这些持久字段怎样解释”。如果混用，schema升级会被误认为一次运营发布，或旧业务版本无法选择正确parser。本节只支持schema v1，future/zero均strict reject。

## 8. 从可恢复性推导 exact Strategy snapshot

现有StrategyID只标识family。历史Activity不能依赖“现在按ID读到的Strategy”，因为Award/Weight可能变化。

因此Lottery新增create-only：

```text
(StrategyID, StrategyRevision)
  -> schema v1
  -> complete immutable Strategy + Awards
```

### 8.1 为什么只存hash不够

Hash能检测内容变化，但原内容被覆盖后无法恢复。要可回滚、可审计，必须保存完整snapshot。Revision token也不能未经canonical encoding和安全设计就宣称content hash、签名或防篡改证明。

### 8.2 为什么snapshot不替换legacy by-ID path

第30节只给Activity publication提供exact content。直接把现有Strategy repository/cache/API全部改成versioned current会把切片扩大成兼容迁移，难以判断回归来自哪里。因此保留旧by-ID路径，新exact snapshot是并行窄端口。

### 8.3 为什么只有create和exact find

Latest/list/update/upsert/delete都会扩大语义：

- latest引入隐式选择；
- update/delete破坏历史；
- upsert掩盖duplicate identity；
- list暗示运营查询和分页契约。

当前需求只需要创建一个exact snapshot并按identity读取，所以ports精确保持两种能力。

## 9. 从闭包完整性推导 Strategy manifest

Graph terminal目前只保存StrategyID，Activity必须为每个unique terminal选择一个exact Strategy revision。

正确不变量不是“manifest里的每个ref存在”，而是：

```text
unique StrategyID set(graph terminal nodes)
  == StrategyID set(publication manifest)
```

左包含右防止extra，右包含左防止missing。再逐个exact读取snapshot，才能证明闭合集完整。

### 9.1 为什么不能只绑定常见路径

尚未走到的branch仍可能被未来请求选择。只绑定“当前常见路径”会让同一publication在不同请求上隐式读取不同时间的Strategy。发布单位必须覆盖整份graph的全部terminal。

### 9.2 为什么同一StrategyID只能有一份revision

同一publication内若`S1:r3`和`S1:r4`同时出现，graph terminal只有S1，无法决定采用哪一份。拒绝ambiguous manifest比按顺序取第一条更安全。

### 9.3 为什么上限是128

每个binding会引发校验、持久化和resolve exact read。无界manifest会把一个“配置对象”变成资源耗尽入口。128不是生产容量结论，而是v1显式复杂度预算；超过上限需要新的需求和性能证据。

### 9.4 为什么canonical排序

排序让：

- 同一集合有唯一内存表达；
- approval candidate逐字段比较稳定；
- persistence/read-back容易核对；
- map/SQL行顺序不进入业务语义；
- 测试能识别duplicate或noncanonical持久数据。

## 10. 从动作语义推导 release/rollback union

Publication kind只有：

```text
release:  rollbackOf = none
rollback: rollbackOf = exact older version
```

Union shape必须同时在domain与CHECK中表达。只设`rollback_of nullable`却不校验kind，会允许“release带source”或“rollback无source”的模糊事实。

### 10.1 为什么rollback追加新版本

假设active v3，恢复v1内容：

```text
错误：active pointer直接改回v1
正确：append v4(kind=rollback, rollbackOf=v1), active=v4
```

v4保留本次publishedAt、新approval evidence和动作身份。历史可以区分原始v1时期与事故后的恢复时期。

### 10.2 rollback到底复制什么

复制source的：

- graph exact ref；
- canonical Strategy manifest；
- startsAt/endsAt。

不复制：

- source version；
- source kind/rollbackOf；
- source publishedAt；
- source approval evidence。

新record使用next version、kind rollback、rollbackOf target、本次Clock和本次approval。

### 10.3 为什么ended source不能rollback

如果本次publishedAt已经到达source exclusive end，新active会立即ended，不能满足“恢复一个仍可运行配置”的止损意图。修改endsAt后仍叫rollback会篡改source内容；正确动作是新release，重新形成candidate与审批。

### 10.4 rollback不撤销副作用

切换active只影响commit后重新resolve的请求。它不会撤销已形成的Participation、route decision、未来Draw、库存扣减、权益发放或外部消息。真正的业务补偿必须由保存exact因果链的后续流程设计。

## 11. 从唯一边界推导 `[start,end)`

时间窗口必须回答start和end的等号归属。半开区间给出唯一规则：

```text
now < start          -> scheduled
start <= now < end   -> active
now >= end           -> ended
```

它的优点不是“数学上好看”，而是相邻窗口可以`[a,b)`与`[b,c)`无重叠拼接，恰好在b只有后一窗口包含该时刻。

### 11.1 UTC微秒不是展示格式

MySQL `DATETIME(6)`持久精度是微秒。Domain在边界把时刻规范为UTC微秒并去除Go monotonic component，使memory/SQL round-trip不会因纳秒或location表示不同而漂移。

运营本地时区、DST选择和输入格式属于未来API adapter；domain不能猜“北京时间字符串”的含义。

### 11.2 为什么Clock只读一次

同一用例中多次`time.Now()`可能跨过start/end：校验时未结束，写入时已结束；或者gate status和decision timestamp不一致。注入Clock并只读一次，使整次业务判断共享一个事实。

### 11.3 为什么不靠定时任务改变状态

Timer只能保证“不早于某时尝试执行”，不能把wall clock变成精确且唯一的状态转换事实；进程重启、调度延迟、重复worker都需要恢复与幂等。当前需求只需要request-time gate，直接比较受控时刻更小、更可靠。

如果未来要求“到点主动推送、预热、通知”，那是新的异步工作流，不能反向改变gate真相。

## 12. Confirmed business decision 与技术失败

这是产品语义与可靠性语义的分水岭。

Confirmed decision：

- draft -> not_published；
- future -> scheduled；
- window -> active；
- end reached -> ended；
- terminal root -> retired。

Technical failure：

- root/publication/manifest损坏；
- active pointer mismatch；
- exact graph/snapshot缺失；
- provider/DB不可用；
- Clock zero；
- context取消或内部deadline。

技术失败若被映射成ended，会把系统坏掉伪装成活动正常结束；若被映射成active则更危险。因此所有failure都返回zero decision + error。

### 12.1 为什么draft是confirmed not_published

Draft+zero publication是合法且可解释的业务状态，不是数据缺失。把它返回error会让上层无法区分“尚未发布”和“读取损坏”。

### 12.2 为什么retired可以root-only gate

Retirement是root上的terminal事实。即使调用者只需要判断已退役，也不必为形成retired decision依赖publication内容。但当前`ResolveActivityService`从repository读取retired root时仍可携带最后active publication并做exact验证；领域函数支持root-only是更小的truth rule，不是允许published缺record。

## 13. 从并发覆盖风险推导 CAS

两个运营者从同一root读取state 3，都会规划v4。Pure planning不能防并发；最终序列化点是数据库root update：

```sql
UPDATE marketing_activity
SET ...
WHERE activity_id = ?
  AND lifecycle_state = ?
  AND state_version = ?
  AND active_version = ?
```

只有一个transaction影响一行。另一个`RowsAffected=0`并rollback其append attempt。

### 13.1 为什么不用Redis分布式锁

Redis lock仍不能替代：

- MySQL unique key；
- exact expected state；
- publication+bindings+header原子事务；
- lock holder崩溃和lease过期处理。

反而增加锁与事务的双重顺序、租约和网络分区。既然MySQL是authoritative state，CAS直接在权威写点完成冲突检测。

### 13.2 为什么不用悲观锁覆盖整个审批过程

Graph验证和approval可能是外部调用。持有数据库row lock跨网络调用会扩大锁时间、阻塞与死锁面。当前方案先读取和验证，最后用短CAS transaction提交；冲突时要求caller基于新state重新规划。

### 13.3 expected state不是幂等键

ExpectedStateVersion防止stale overwrite，但同一请求在commit outcome unknown后重放可能基于已推进状态形成另一个版本。幂等需要独立operation identity与结果registry，本节没有实现。

## 14. 从原子可见性推导事务边界

一次publication写入包含：

1. immutable publication header；
2. 全部Strategy revision bindings；
3. Activity active pointer/state generation。

三者必须同一transaction：

```text
INSERT publication
INSERT binding * N
CAS UPDATE Activity root
COMMIT
```

若先更新root，reader可能看到active指向不存在record；若header和bindings分事务，reader可能看到half manifest；若CAS失败却保留record，会形成未被授权的orphan历史。

### 14.1 为什么先INSERT再CAS仍安全

它们在同一transaction内对外不可见。先insert可以让000011 reverse FK在root指向新publication时已满足；CAS失败则rollback所有insert。

### 14.2 为什么`RowsAffected`必须精确等于1

0表示expected predicate没命中，属于state conflict；大于1表示identity/SQL假设被破坏。两者都不能当成功。`database/sql.Result.RowsAffected`返回driver报告的影响行数，adapter仍需处理driver不支持或调用出错的情况。

## 15. 从读一致性推导 exact RR snapshot

Current resolve需要root、active record和manifest是同一个数据库snapshot。

错误序列：

```text
read root -> active v2
another tx commits v3
read latest publication -> v3
```

本节使用read-only REPEATABLE READ transaction，并始终从root的exact active version读取，不猜latest。严格恢复在transaction读取结束后执行，任何缺行、错identity或noncanonical state都失败。

### 15.1 为什么不是“单条大JOIN就一定正确”

大JOIN也可以处于一致snapshot，但会把root/header/N bindings膨胀成重复行，容易隐藏missing child与limit问题。当前分步exact读取更利于边界检查；关键是同一RR transaction和exact pointer，而不是查询条数本身。

### 15.2 为什么resolve还要重验Lottery

Publish-time verification阻止正常应用写坏ref，但不能消除：

- DBA/migrator旁路；
- 旧bug或错误恢复；
- Lottery authority损坏；
- 跨上下文没有物理FK。

Resolve reverify把坏配置阻挡在gate allow前。它不fallback previous/latest，因为那会让系统在未审批配置上自行选择。

## 16. CHECK、FK 与领域校验各负责什么

### 16.1 CHECK适合单行shape

MySQL CHECK可以表达：

- ID/version > 0；
- schema == 1；
- kind/rollback nullable union；
- `starts_at < ends_at`；
- draft/published/retired单行shape；
- bounded token基本grammar。

但CHECK不能读取另一张表、使用subquery或证明graph terminal set等于manifest。Domain/ACL不能被DDL替代。

### 16.2 FK适合同一所有权边界

Marketing内部FK合理：

- publication -> Activity；
- binding -> publication；
- rollback source -> same Activity publication；
- active pointer -> same Activity publication。

这些对象共享数据库事务和生命周期。

### 16.3 为什么不跨上下文建FK

Marketing publication不对Lottery graph/snapshot建FK，因为这样会：

- 固化Lottery物理表和复合键；
- 假设永远同库；
- 让Marketing migration依赖provider schema；
- 仍无法证明terminal/manifest集合相等。

代价是数据库本身不能证明ref存在，必须以exact verifier、strict resolve、最小权限和故障停止线补偿。

## 17. 从审批candidate漂移推导“两次规划”

Publish先用内部临时evidence形成provisional record，以此创建exact candidate；Lottery与approval验证该candidate。Approval返回真实evidence后，service重新调用pure plan，并逐字段比较candidate是否变化。

为什么不直接给record换evidence？因为publication私有不可变字段不应开放mutation。重新规划还能证明：

- version没有变；
- window/graph/manifest/publishedAt没有漂移；
- approval的对象与最终CAS record相同。

`application-plan`只是内部合法token，绝不能落库或传给外部provider。

## 18. 从网络不确定性推导 commit outcome unknown

COMMIT时断线存在两个世界：

```text
数据库已提交，响应丢失
数据库未提交，连接中断
```

客户端观察到同一个error，却不能知道是哪一个。把它一律当retryable会在第一种世界追加重复版本。

因此repository独立分类`ErrCommitOutcomeUnknown`，但 application 不能只把“candidate identity”写进日志后让
人猜。它需要保留由本次已验证 transition 推导出的 exact attempt，同时继续维护失败零值协议。

最终取舍是 application-owned `ActivityCommitReceipt`：publish/rollback/retire 发生 commit unknown 时，
service 仍分别返回 zero publication/zero Activity + error；仅当 caller/operation context 均仍存活、stable
class 精确为 `ErrCommitOutcomeUnknown` 且 receipt 自身合法时，可信调用方才能通过
`ActivityCommitReceiptFromError` 显式取得防御复制。Receipt 保存 before/after root；publish/rollback 还保存
完整 immutable publication，rollback还校验`rollbackOf < before.active`，retire 的 after root保留本次
retiredAt/evidence。它不进入普通错误文本、
`errors.Is` 或 unwrap 链，其他失败类别也没有 receipt。

为什么不是让 receipt 自己查数据库？因为这会把 repository、连接选择、权限和重试策略偷偷塞进 value
object。Read-back I/O 应由上层用新健康连接完成：publish/rollback 取得同一 RR current snapshot，retire
取得 exact root；然后把可信读取包装成 `ActivityCommitObservation`，交给纯
`ReconcileActivityCommit`。结果闭合为：

- `committed`：exact after-image 命中，且发布动作的完整 publication 逐字段相等；
- `not_committed`：exact before-image仍在，或同一 next generation 出现另一合法 winner；
- `indeterminate`：observation缺失、损坏、identity/name不符、partial，或已经推进到更晚 generation。

“更晚 generation”不能证明本次没有短暂提交后又被后续动作推进，所以必须 indeterminate。这条规则比
“active不是candidate就算失败”更保守，也避免恢复逻辑制造第二个历史。Reconcile不做I/O、不重新读
Clock/approval、不输出retry建议；上层只有在 not_committed 且另有幂等/授权策略时，才可能重新发起一项
新操作，绝不能盲重放原command。

## 19. 从取消归属推导 context 策略

Application为每次操作设置private maxDuration，但错误归属遵循：

```text
caller context error
  > service internal deadline
  > provider/domain/repository error
```

原因：caller已取消时，provider稍后返回SQL error不应覆盖真实控制原因；反过来，caller仍live时provider自己的deadline应保持provider failure，不冒充service timeout。

### 19.1 maxDuration不是什么

它是协作式安全预算，不是：

- goroutine强制抢占；
- production P99/SLO；
- 对不遵守context的driver的硬终止；
- runtime默认值已经决定的证明。

服务目前未装配，所以没有生产配置承诺。

## 20. 从攻击面推导低披露error

Activity、graph revision、Strategy snapshot和approval都是高价值配置。若普通`Error()`展开底层cause，未来API/log可能泄露：

- SQL/table/index；
- exact identity是否存在；
- Award/Weight；
- approval backend；
- credential/endpoint。

本节新增的 Marketing operation/dependency/repository wrapper 只公开 reviewed class，`errors.Is` 不穿透 private cause；可信诊断代码必须显式 `Cause()`。这不是对既有 Lottery repository error contract 的全局改写，也不是完整的 HTTP disclosure policy；它只防止 Marketing 新链路意外展开底层原因。

## 21. 为什么本节不做公开 API

Public write surface至少需要Principal、server-side authorization、resource scope、error disclosure、CSRF/session/idempotency/audit等契约。当前只有approval port，不能用“内网接口”绕过威胁建模。

所以领域和application存在，但architecture test要求它们不进入`cmd/growth-api`、HTTP、Compose和Web。能力未装配是设计结果，不是漏做。

## 22. 替代方案逐项比较

### 22.1 Activity直接保存可变current config

优点：表少、读简单。缺点：历史被覆写、审批对象漂移、回滚无新事实、并发last-write-wins。拒绝。

### 22.2 Activity只保存graph revision

优点：比StrategyID更精确。缺点：terminal Strategy内容仍会按ID漂移，无法完整恢复。拒绝。

### 22.3 Publication只保存StrategyID列表

优点：manifest小。缺点：不是exact content identity。拒绝。

### 22.4 每次resolve查询latest Strategy revision

优点：自动获得新配置。缺点：绕过Activity审批与历史，两个请求可能读到不同配置。拒绝。

### 22.5 回滚直接把active指回旧version

优点：写入少。缺点：没有新publishedAt/approval/action identity，无法解释恢复时期。拒绝。

### 22.6 为每个published request加Redis lock

优点：看似串行。缺点：多一个不可靠协调面，仍需要DB事务/CAS/unique。拒绝。

### 22.7 使用数据库悲观锁直到审批结束

优点：不会stale。缺点：跨网络长事务、锁竞争和failure复杂。拒绝；采用验证后短CAS。

### 22.8 Marketing到Lottery建FK

优点：单ref存在性由DB保证。缺点：跨上下文物理耦合且不能证明闭合集。拒绝。

### 22.9 立即引入通用配置中心

优点：可能已有发布、审计、灰度能力。缺点：Activity状态、exact cross-context closure和业务回滚仍需领域定义；引入平台并不自动解决业务identity。当前先形成最小内核，未来按部署/运营需求比较build/buy。

### 22.10 立即引入MQ/outbox

优点：可通知cache/consumer。缺点：本节没有事件消费者与异步一致性需求，先发事件会伪造contract。拒绝；先保证authoritative transaction。

## 23. 失败模式预演

| 失败 | 第一响应 | 为什么 |
| --- | --- | --- |
| stale expected state | conflict，重新读取 | 防止覆盖赢家 |
| exact graph/snapshot missing | 停止发布/resolve | 不选择latest |
| manifest mismatch | candidate invalid | 闭包不完整 |
| approval reject | 不写入 | 业务治理拒绝 |
| approval unavailable | 不写入，可在核实后重试 | 未形成批准事实 |
| binding insert失败 | rollback transaction | 防half manifest |
| CAS 0 rows | rollback + conflict | 另一个writer已推进 |
| COMMIT unknown | 提取可信receipt，exact read-back后三态对账 | 可能已durable，失败结果仍为zero |
| persisted root invalid | technical failure | 不best-effort恢复 |
| gate provider unavailable | zero decision + error | 不伪装业务reject |
| retired | confirmed reject | 合法terminal事实 |

## 24. 观测与审计应记录什么

当前未实现production telemetry，但未来安全字段至少应以bounded、低披露形式表达：

- operation kind：create/publish/rollback/retire/resolve；
- stable result class；
- expected与observed state generation是否冲突；
- candidate/publication exact identity的受控表示；
- gate status与evaluatedAt；
- latency stage：repository/Lottery/approval/CAS；
- commit unknown与read-back disposition；
- correlation ID与未来audit actor reference。

普通日志不应记录Award/Weight、approval payload、credentials、raw SQL/provider response或任意unbounded revision。

## 25. 测试为什么按层分配

### Domain

证明所有合法/非法shape、边界时刻、溢出、defensive copy和zero result；最适合table/fuzz，不需要数据库。

### Application

证明依赖顺序、调用次数、Clock一次、candidate rebuild、context/error优先级和failure不写入。

### ACL adapter

证明exact translation、terminal/manifest集合相等、snapshot exact read与provider classification。

### SQL mock repository

证明SQL顺序、exact predicate、transaction rollback、RowsAffected和commit classification；不能代替真实MySQL语义。

### Real MySQL

证明Migration、FK/CHECK/RESTRICT、isolation、grant拒绝、concurrent CAS与driver行为；不能推导生产容量。

当前仓库为此提供显式确认保护的`make lesson30-mysql-acceptance`：它启动随机命名的tmpfs MySQL 8.4.11并使用隔离身份执行schema/repository fixtures，再核对清理。

冻结前候选上，该门禁已连续两次真实 exit 0：从真实 v5 baseline 升至 v11、重复 no-change、dirty
fail-closed，核对五张新表、6 个 RESTRICT FK、20 个 enforced CHECK 与 binary collation；隔离 snapshot/
Marketing writer 只得到所需权限，immutable/cross-context/schema 写读越界均由 MySQL 1142 拒绝；真实
repository fixture覆盖 exact RR、并发单赢家、rollback/retire、half-write rollback 与 bad cross-context ref
由 ACL 拒绝。脚本随后证明临时 container/volume/network/identity/secret 零残留，且长驻资源未变。

长驻 Compose 也已在保持原 MySQL container、volume、network identity 与旧表数据/checksum不变的前提下
完成 v5→v11；长期 `growthos_app` grant仍只含旧两表 SELECT，八张未装配表继续1142拒绝，status/smoke
均 exit 0。独立 Lottery Compose acceptance 已通过并完成随机资源清理；`make verify` 已覆盖 Go 与 Web
现有回归。以上都是此前候选的真实证据，不是 accepted tip 结论；commit receipt收口后的最终 race、
shuffle、fuzz、coverage、real MySQL 与全仓门禁仍需复跑。

### Architecture guard

证明本节没有越界接入runtime/HTTP/Web；它是“没有做某事”的可执行证据。

## 26. 外部一手资料如何影响本节

以下资料均于 **2026-08-30** 访问。它们用于核验语言/数据库/平台事实，不替代本项目产品基线。

| 来源 | 可核验事实 | 本节只据此推导什么 | 不据此宣称什么 |
| --- | --- | --- | --- |
| [Go `time` package](https://pkg.go.dev/time) | `Time`比较、monotonic clock表示、Timer语义 | 持久时刻先canonical；领域窗口用明确比较 | Go timer能精确完成业务发布 |
| [Go `context` package](https://pkg.go.dev/context) | deadline/cancel在API边界传播；CancelFunc释放资源 | service使用derived context与明确归属 | context能强制抢占不合作依赖 |
| [Go 数据库取消指南](https://go.dev/doc/database/cancel-operations) | DB调用应传播request context并defer cancel | repository调用沿用operation context | 所有driver都能瞬时停止 |
| [`database/sql.Result`](https://pkg.go.dev/database/sql#Result) | `RowsAffected`报告受影响行数且可能返回error | CAS必须检查精确一行并处理查询失败 | 它单独证明transaction原子性 |
| [MySQL 8.4 CHECK constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html) | CHECK表达式受限，不能用subquery/引用其他表等 | 单行shape放CHECK，闭合集留给domain/ACL | DDL可证明跨上下文闭包 |
| [MySQL 8.4 foreign keys](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html) | FK/RESTRICT的声明与限制 | 同owner强引用使用FK | 跨上下文一定都应建FK |
| [MySQL 8.4 consistent reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html) | InnoDB consistent read与snapshot规则 | current reader用同一RR read transaction | RR自动修复业务错identity |
| [MySQL 8.4 locking reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-locking-reads.html) | locking read用于显式锁定读取 | 比较后选择短CAS而非跨审批长锁 | 悲观锁永远不适用 |
| [Microsoft DDD microservice domain model](https://learn.microsoft.com/en-us/dotnet/architecture/microservices/microservice-ddd-cqrs-patterns/microservice-domain-model) | domain model适合复杂规则/不变量，bounded context内语言一致 | 把Activity与Lottery按规则/所有权隔离 | 微软文档是本项目规范 |
| [Microsoft domain analysis](https://learn.microsoft.com/en-us/azure/architecture/microservices/model/domain-analysis) | 从业务能力、子域、bounded context分析边界 | 从变化原因而非表名划分owner | 必须拆成独立网络微服务 |

## 27. 业界配置发布方案只作为对照

以下一手/官方资料同样于 **2026-08-30** 访问：

- [Apollo 配置中心介绍](https://github.com/apolloconfig/apollo/wiki/Apollo%E9%85%8D%E7%BD%AE%E4%B8%AD%E5%BF%83%E4%BB%8B%E7%BB%8D)：说明配置发布、客户端更新等平台能力；本节只用来比较“平台配置分发”和“业务Activity publication”不是同一层。
- [Apollo 使用指南](https://github.com/apolloconfig/apollo/wiki/Apollo%E4%BD%BF%E7%94%A8%E6%8C%87%E5%8D%97/a6611f0bef63986aebe02bf37502999933f3efeb)：包含发布历史与回滚操作说明；只启发面试中的build/buy讨论，不证明GrowthOS采用Apollo。
- [Apollo 设计](https://github.com/apolloconfig/apollo/wiki/Apollo%E9%85%8D%E7%BD%AE%E4%B8%AD%E5%BF%83%E8%AE%BE%E8%AE%A1)：用于理解配置通知与读取体系；不替代本节exact业务identity和事务。
- [AWS AppConfig 概览](https://docs.aws.amazon.com/appconfig/latest/userguide/what-is-appconfig.html)、[deployment strategy](https://docs.aws.amazon.com/appconfig/latest/userguide/appconfig-creating-deployment-strategy.html)与[revert deployment](https://docs.aws.amazon.com/appconfig/latest/userguide/appconfig-deploying-reverting.html)：用于比较validation、deployment strategy、monitor/revert能力；不证明本节已实现canary、自动回滚或云部署。
- [Martin Fowler: Feature Toggles](https://martinfowler.com/articles/feature-toggles.html)：这是工程实践文章而非标准；只帮助区分release toggle/experiment与本节单active publication，不作为权威规范。

结论：若未来规模、灰度或多环境promotion证明需要配置平台，应将Activity领域identity映射到平台deployment，而不是用平台的“版本”吞掉本节业务语义。

## 28. 安全与残余风险

本节已缓解：

- normal writer覆写历史；
- stale concurrent publish；
- manifest缺失/额外/错revision；
- current snapshot混版；
- 技术失败伪装业务拒绝；
- 普通error泄露底层cause。

仍存在且明确未解决：

- DBA/migrator可旁路应用协议；
- 没有真实Principal/RBAC/tenant；
- 没有真实approval adapter或职责分离；
- 没有公网transport威胁模型；
- 没有正式Draw前后的retire重查策略；
- 没有audit storage、retention或tamper evidence；
- 没有多区域复制、一致性或灾备；
- 没有运行态指标和告警。

这不是“后续补一补”式备注，而是决定当前服务必须未装配的安全理由。

## 29. 何时应该扩展模型

只有出现可验证需求才新增：

| 新需求 | 可能的新模型/ADR | 为什么不能塞进v1 |
| --- | --- | --- |
| pause/resume | 独立operator override或新lifecycle | 与terminal retire不同 |
| gray/canary | audience/percentage多active deployment | 破坏唯一active假设 |
| scheduled proactive jobs | durable job identity/idempotency/lease | request-time gate不够 |
| cross-environment promotion | signed artifact/canonical digest | revision token不等于签名 |
| public operations | session/RBAC/API/idempotency/audit | 新攻击面 |
| event consumers | outbox/event schema/replay | transaction后异步contract |
| formal Draw | Participation/route/Strategy snapshot/result | publication allow不等于抽奖结果 |
| compensation | saga/ledger/idempotent compensator | rollback不能撤销副作用 |
| multi-region | consistency/leadership/conflict policy | 单MySQL CAS假设改变 |

## 30. 如何像项目设计者一样复核功能请求

当有人提出“加一个一键回滚按钮”，不能直接答应UI。应连续追问：

1. 回滚的是配置、流量还是已发生业务结果？
2. source exact identity是什么？
3. 是否允许回到已结束window？
4. 谁授权，谁审批，是否职责分离？
5. 并发发布发生时expected state是什么？
6. commit结果未知时按钮显示什么？
7. 已经读取旧snapshot的请求怎么办？
8. 哪些日志对普通运营可见？
9. 是恢复旧内容形成新历史，还是改写旧记录？
10. 怎样验收DB权限确实不能UPDATE历史？

只有这些问题有答案，按钮才是功能；否则只是给不完整协议加了一个入口。

## 31. 本节最终推导链

```text
历史必须可解释
  -> immutable Strategy snapshot + Activity publication

发布必须复现完整Lottery配置
  -> exact graph ref + terminal Strategy revision closure

并发不能覆盖
  -> state generation + DB CAS

record/bindings/header不能分裂
  -> one transaction + internal FKs

request不能混版
  -> exact active pointer + RR snapshot

业务边界必须唯一
  -> one controlled UTC-microsecond Clock + [start,end)

恢复动作本身必须可审计
  -> append rollback publication

技术失败不能伪装业务结果
  -> confirmed gate vs zero decision + error

权限与审批尚不完整
  -> internal unassembled service stopline
```

这条链中每个机制都对应一个先出现的问题；没有问题支撑的技术没有被提前加入。
