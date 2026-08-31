# ADR-0026：Activity 以不可变版本绑定 Lottery 精确配置

- **状态：** 已接受
- **日期：** 2026-08-30
- **负责人：** GrowthOS 架构组
- **适用范围：** 第 30 节“为什么 Strategy 不等于 Activity”
- **需求基线：** [Activity 发布与 Lottery 配置精确绑定基线 v1](../product/activity-publication-binding-v1.md)
- **替代关系：** 不替代 ADR-0019、ADR-0024 或 ADR-0025；在既有规则所有权、不可变 graph 与 exact evaluator 边界之上新增 Marketing publication selection

## 背景

第 17～24 节形成了 Lottery `Strategy` / `Award`、MySQL Repository、`WeightedSelector`、受限 ephemeral API/React 和按 StrategyID 缓存的可重建读取投影。该 Strategy 当前只有稳定 ID，没有业务 revision；根/子行 `updated_at` 只是物理行元数据。

第 27～29 节继续形成：

- 一个 Lottery-owned 会员 tier 到 StrategyID 的具体 route 语义；
- exact `(GraphID, Revision)`、create-only、不可变、有界且可严格恢复的 routing graph；
- 只按调用方显式 identity 求值的 closed typed evaluator；
- 一次 graph、Clock 和会员 fact、确定 single path、step/time/cancel budget 与 zero-decision failure。

这些能力仍没有回答哪份配置对真实 Activity 生效。让 evaluator 查询 `latest graph` 会把发布选择混入执行；只让 Activity 保存 StrategyID 又无法定位具体 Award/Weight 历史；把 status/window 放进 Strategy 或 graph 则会混淆 Marketing 与 Lottery 的事实所有权。

第 30 节因此需要同时建立两个最小、互相引用但不互相吞并的事实：

1. Lottery 为现有 Strategy 内容建立 exact create-only revision snapshot；
2. Marketing 让 Activity 通过不可变 numeric publication version 精确引用 graph revision 与 graph 全部 terminal Strategy 的 revision 闭合集。

运营工作流还要求草稿、发布、回滚、停止与审批边界。当前仓库没有 Governance runtime、真实会话、服务端授权、运营 API 或正式 Draw，因此本 ADR 必须既定义可执行发布内核，也保留严格停止线。

## 决策驱动

1. Strategy 是 Lottery 可复用选择配置，Activity 是 Marketing 有时间窗和生命周期的业务资源，两者变化原因不同；
2. graph revision 只绑定拓扑，terminal StrategyID 不足以标识 Award/Weight 历史内容；
3. 已发布配置必须可恢复，不能只记录能发现漂移却无法恢复内容的 hash；
4. graph、Strategy、Activity publication、Activity state CAS、schema、Migration 和应用版本必须分离；
5. publication 必须按 exact refs 解析，不能依赖 latest/max revision 或缓存偶然值；
6. 一次发布要么使完整 publication 与全部 Strategy bindings 原子成为 active，要么完全不改变 Activity；
7. 两个并发发布不能静默覆盖，必须由 expected state/version 产生一个明确 winner；
8. 回滚必须可追溯且不改写历史，也不能撤销已经发生的参与、选择或权益事实；
9. 时间窗需要一个受控时刻和精确边界，不能由浏览器或异步状态任务决定；
10. Governance 拥有审批与授权，Marketing 只能消费针对 exact candidate 的可信审批 evidence；
11. 跨 bounded-context 引用是业务契约，不应固化为 Marketing 到 Lottery 物理表的外键；
12. 第 31～35 节必须仍能按公共模型、会话、服务端授权、前端投影和越权 E2E 的顺序演进；
13. 本节不能因为“发布”就顺带装配真实抽奖、selector、Draw、库存或发奖；
14. 每个 Migration 继续对应一个 MySQL DDL，已执行历史只向前追加。

## 非目标

本 ADR 不决定或实现：

- Activity HTTP/GraphQL/gRPC/MCP/Agent API；
- React Activity 页面、运营后台、发布按钮或导航；
- Principal、session、Role、Permission、Resource、Action、Scope、Policy 或 tenant；
- 真实 Governance 审批存储、审批 UI、操作者审计或职责分离实现；
- Activity 草稿编辑器、预览、diff、pause/resume、cancel/archive；
- 灰度、多个 active version、定时无缝 activation 或 scheduler；
- graph schema v2、Strategy revision 直接进入 terminal、DSL 或 operator registry；
- evaluator runtime composition、生产会员 adapter、Participation 组合；
- Strategy/Award 随机选择、正式 Draw/Result、幂等或结果查询；
- 库存、预算扣减、Benefit 交付、Outbox、MQ 或补偿；
- Redis publication/graph/Strategy revision cache 或精准失效；
- 多租户数据隔离、跨服务事务、服务拆分或生产 SLO；
- 内容签名、不可抵赖审计、跨环境 promotion 或法规保留期。

## 评估过的方案

### 方案一：把 Activity 时间窗和状态加入 Strategy

| 优点 | 代价 / 风险 |
| --- | --- |
| 一个对象看似即可完成“活动抽奖” | 同一 Strategy 无法被不同排期 Activity 复用 |
| selector 调用方参数较少 | Lottery 聚合吞并 Marketing 生命周期、审批与退役 |
| 可以继续按 StrategyID 查询 | Redis cache、Repository 和 selector 被迫理解 Activity 动态状态 |

**结论：拒绝。** Strategy 继续只拥有 Award/Weight/outcome 配置；Activity 的目标、窗口与生命周期归 Marketing。

### 方案二：在 graph header 增加 ActivityID、status 与 active 标志

| 优点 | 代价 / 风险 |
| --- | --- |
| graph lookup 可以直接返回“当前配置” | immutable Lottery topology 与 Marketing 发布状态耦合 |
| 少一组 Activity 表 | 一个 graph 难以被多个 Activity 复用 |
| evaluator 可以隐藏 identity 参数 | 执行器重新承担发布选择，测试边界倒退 |

**结论：拒绝。** 第 28 节 graph revision 保持不变。Activity publication 通过 exact reference 选择 graph，evaluator 继续只执行调用方给出的 exact identity。

### 方案三：Activity publication 只保存 graph revision 与 StrategyID

| 优点 | 代价 / 风险 |
| --- | --- |
| 不需要新的 Strategy schema | StrategyID 无法定位某次 Award/Weight 快照 |
| graph terminal 已有 StrategyID | 历史 Activity 会随当前按 ID 读取内容漂移 |
| 数据行较少 | 回滚无法恢复被替换的 Strategy 内容 |

**结论：拒绝。** graph revision 精确不代表 target content 精确；每个 unique terminal StrategyID 必须绑定一个 exact Strategy revision。

### 方案四：只保存 Strategy content hash

| 优点 | 代价 / 风险 |
| --- | --- |
| 可以检测当前内容是否变化 | 原内容被替换后无法从 hash 恢复 Awards |
| 固定长度、易比较 | canonical encoding、算法和签名语义尚需定义 |
| 无需复制快照行 | “发现不一致”不等于“历史可解释” |

**结论：不采用为 v1 权威版本。** 新增完整 create-only Strategy snapshot。未来可以在完整快照旁增加 canonical digest/signature，不用 hash 取代内容。

### 方案五：直接给现有 Strategy/Award 表增加 revision 并改复合主键

| 优点 | 代价 / 风险 |
| --- | --- |
| 不增加平行 snapshot 表 | 改写第 18～24 节既有读取、FK、cache 和 ephemeral API 语义 |
| 所有查询天然 versioned | 需要复杂 expand/contract 与旧数据 backfill |
| 表数量较少 | 本节会同时承担旧 runtime 迁移和发布建模两项主要目标 |

**结论：不采用。** 保留 legacy Strategy 读取路径，新建 `lottery_strategy_snapshot` / `_award` 作为发布历史。现有行不会自动成为 published snapshot。

### 方案六：Activity header 直接保存可变 graph/Strategy/window

| 优点 | 代价 / 风险 |
| --- | --- |
| 一行读取 current 配置 | 每次发布 UPDATE 覆盖历史，无法解释进行中/已完成请求 |
| 回滚可再 UPDATE 一次 | 审批 evidence 与实际内容容易错配 |
| 不需要 publication child 表 | 并发覆盖、partial update 与历史审计边界不清 |

**结论：拒绝。** header 只保存 lifecycle、active version 与 state CAS；配置进入 append-only publication。

### 方案七：回滚把 active 指针改回旧版本

| 优点 | 代价 / 风险 |
| --- | --- |
| 实现简单，不复制 binding | ActivityVersion 时间顺序倒退，无法区分原 v2 与回滚后的新时期 |
| 旧内容本来不可变 | 回滚动作、审批和发生时间缺少新的业务 identity |
| 少一个 publication row | current+1 单调协议被破坏，缓存/审计更复杂 |

**结论：拒绝。** 回滚追加新 ActivityVersion，精确复制 source 内容并记录 `rollback_of`，再原子切 active。

### 方案八：把 scheduled/active/ended 持久化为状态并由 scheduler 更新

| 优点 | 代价 / 风险 |
| --- | --- |
| 查询列表时状态直观 | 状态会与 Clock/window 漂移，scheduler 失败产生双重真相 |
| 可直接筛 running | DST、延迟和批量更新成为核心正确性依赖 |
| UI 不用计算 | 当前没有 runtime/UI/scheduler 需求证据 |

**结论：拒绝。** header 只存 draft/published/retired；时间相位由一次受控 UTC 时刻和 `[start,end)` 计算。

### 方案九：Marketing 表通过 FK 直接引用 Lottery graph/snapshot 表

| 优点 | 代价 / 风险 |
| --- | --- |
| MySQL 能拒绝不存在的跨上下文 ref | Marketing Migration 固化 Lottery 表名、主键与同库部署 |
| 查询前已有局部存在性 | 数据所有权被物理 schema 耦合，未来拆服务成本高 |
| 少一次应用验证误解 | FK 仍不能证明 graph terminal 与 Strategy binding set exact equality |

**结论：不采用。** Marketing 只保存 exact refs。Lottery verifier 在 publish 前验证，current resolve 在使用前再次 fail closed；数据库只建立各 bounded context 内部 FK。

### 方案十：共享通用 Repository/CRUD 和状态机框架

| 优点 | 代价 / 风险 |
| --- | --- |
| 表面减少接口数量 | Save/Update/Upsert 隐藏 publication append-only/CAS 语义 |
| 以后可给其他聚合复用 | 当前只有一个真实状态机，抽象稳定点未经证明 |
| adapter API 看似整齐 | 调用方可能绕过原子 publication transaction |

**结论：拒绝。** 端口按 CreateDraft、Publish、ReadCurrent、ReadExact、Retire 等真实用例拆分。

### 方案十一：Marketing Activity + Lottery Strategy snapshot + immutable publication binding

| 优点 | 成本 / 风险 |
| --- | --- |
| 所有权清楚，同配置可多 Activity 复用 | 增加五张表、六个 DDL 与两组 Repository |
| exact graph/Strategy/history 可恢复 | DB 不独立证明跨上下文 refs/闭合集 |
| append-only publish/rollback 可追溯 | v1 没有灰度、双 active 或 pause/resume |
| state CAS 与 RR snapshot 可验证并发一致性 | 服务未授权、未装配，不能公开使用 |

**结论：采用。** 这是当前能够真实回答第 30 节问题、同时不倒灌第 31～45 节能力的最小边界。

## 决策

### 1. Marketing 拥有 Activity；Lottery 拥有被引用配置

Marketing 定义和改变：

- ActivityID 与稳定 operator-facing name；
- draft/published/retired lifecycle；
- ActivityStateVersion；
- immutable ActivityVersion；
- active publication selection；
- publication time window；
- release/rollback/retire 迁移。

Lottery 定义和改变：

- Strategy/Award/Weight/outcome；
- exact Strategy snapshot revision；
- routing graph identity/schema/topology；
- graph terminal StrategyID set；
- graph evaluation 与 Strategy route。

Marketing 只保存 Lottery exact refs，不复制或解释 Award/Weight、node/edge、会员 branch 或 selector 算法。Lottery 也不反向保存 Activity lifecycle。

### 2. 新增 create-only Strategy snapshot revision

新增 Lottery aggregate 的 publication identity：

```text
StrategySnapshotIdentity = (StrategyID, StrategyRevision)
```

`StrategyRevision` 使用独立 bounded ASCII grammar，v1 为 `[A-Za-z0-9][A-Za-z0-9._:-]{0,127}`。Snapshot 包含 schema v1 与完整、严格校验的 Strategy/Awards；AwardID canonical 排序，collection defensive copy，现有 1000 Award 与 total weight overflow 边界不变。

同 identity 只允许 create 一次。Snapshot Repository 只提供 Create 与 FindByIdentity，不提供 latest/list/update/upsert/delete。Duplicate 是 conflict；commit outcome unknown 通过 exact read-back 处理。

Revision token 是 registry-bound identity，不宣称 content hash。现有 StrategyID path 与 Redis cache 不因此获得版本语义，也不用于读取 publication snapshot。

### 3. Activity header 使用三态生命周期

状态只允许：

```text
draft -> published -> retired
          ^   |
          |---| publish/rollback append new version, state remains published
```

形状：

- draft：active NULL、retired_at/evidence NULL、state version = 0；
- published：active nonzero、retired_at/evidence NULL、state version = active ActivityVersion；
- retired：保留 nonzero active、retired_at/evidence nonzero、state version = active ActivityVersion + 1。

第一份 publish 为 ActivityVersion 1。retired 为 v1 终态；不允许 resume、published->draft 或删除恢复。Natural window end 不改变 lifecycle。

### 4. ActivityVersion append-only，StateVersion 专用于 CAS

ActivityVersion 是 per-Activity positive numeric sequence：1、2、3……。普通 publish 和 rollback 都只能追加 current active + 1，并立即成为唯一 active。旧 publication 永不 UPDATE/DELETE。

ActivityStateVersion 是 header 的 non-negative 并发 token：CreateDraft 初始化为 0；publish/rollback/retire 每成功一次恰好递增 1。published 时它必须等于 active ActivityVersion；retired 时必须等于保留的 active ActivityVersion + 1。这个数值关系是可校验的状态形状，但两个字段仍承担不同语义，不能互作 command token。

所有 write command 必须携带 expected lifecycle、state version 与 active version；Repository CAS affected rows 必须恰为 1。并发 loser 返回 stable conflict，不自动重放或覆盖 winner。

### 5. Publication 是封闭不可变快照

每份 publication 必须包含：

- ActivityID/ActivityVersion；
- `release | rollback` kind；
- rollback source；
- exact GraphID/GraphRevision；
- canonical exact Strategy revision bindings；
- UTC canonical microsecond `starts_at/ends_at/published_at`；
- trusted approval evidence reference。

Release 的 source 必须为空。Rollback 的 source 必须属于同 Activity、早于新 version，并精确复制 source 的 graph ref、Strategy binding set 与 window；新 version、published_at 和 approval evidence 是本次回滚的新事实。

### 6. exact closure 由 Lottery verifier 证明

Publication binding 的 StrategyID set 必须与 exact graph 的所有 unique terminal StrategyID 完全相等。Lottery verifier：

1. exact read + Validate graph；
2. 收集 canonical unique terminal set；
3. 拒绝 binding zero/duplicate/missing/extra；
4. exact read + Validate 每个 Strategy snapshot；
5. 核对返回 identity；
6. 只在整体完整时返回 `nil` 成功裁决，任一失败都不返回 partial proof。

v1 不定义额外 sealed-proof 类型。被验证的 immutable candidate 已经是完整输入；`nil` 只是本次调用的整体裁决，不会被持久化，也不替代 Governance approval evidence。Resolve 仍每次重新验证 exact refs。

不得使用 `FindLatest`、max revision、empty fallback、Redis current value或“缺失时继续旧配置”。Publish 前验证，未来每次 resolve current publication 时也必须 fail closed，防止特权旁路写入坏 ref 后直接使用。

### 7. 不修改 graph schema v1

graph terminal 继续只持有 StrategyID。它决定 route family；Activity publication 的 mapping 决定该 ActivityVersion 下 family 对应 exact StrategyRevision。这样第 28 节 immutable graph 不被回写，也允许同一 graph 被不同 Activity 绑定不同 Strategy revisions。

若未来 graph 本身必须直接固定 Strategy revision，使用 schema v2 和新 ADR/Migration，不静默改变 v1 terminal 语义。

### 8. 发布和回滚使用同一原子事务边界

Publication application 在事务前完成 read-only Lottery/approval 验证；Marketing Repository 在一个 InnoDB transaction 中：

1. INSERT immutable publication；
2. INSERT 全部 canonical Strategy binding rows；
3. CAS UPDATE Activity header lifecycle/active/state version；
4. COMMIT。

任一步在 commit 前失败，整体 rollback。Header 不可指向半个 publication，orphan publication/binding 也不能在 CAS 失败后保留。

Commit failure 若 caller 与 operation context 在 Repository 返回后仍存活，且错误精确分类为 `ErrCommitOutcomeUnknown`，application 仍返回 zero domain result，但在低披露的 `ActivityOperationError` 内保留一份经校验、防御复制的 `ActivityCommitReceipt`。Receipt 保存 exact before/after；publish/rollback 还保存完整 publication，retire 的 after root保存本次 retiredAt/evidence。它只能由可信恢复流程通过显式 accessor 取得，不进入错误文本、`errors.Is` 或普通 unwrap chain；其他错误类别与 context 结束不携带 receipt。

恢复 I/O 与 receipt value 分离：publish/rollback 用新健康连接读取一个 RR current snapshot并构造 `ObserveCurrentActivity`，retire exact读 root并构造 `ObserveActivityRoot`。纯 `ReconcileActivityCommit` 仅返回 `committed | not_committed | indeterminate`：exact after（以及完整 publication）命中才是 committed；exact before 或同一 next generation 的另一合法 winner 是 not_committed；坏/缺/partial/mismatched observation 与更晚 generation 都是 indeterminate。它不重读 Clock/approval、不猜 latest、不授权 retry；调用方不能盲目改版本重试。

### 9. 回滚追加新版本，不倒退 active

Rollback 仅允许 published Activity 指向同 Activity 的早期、严格恢复且满足 `evaluated_at < source.ends_at` 的 publication。Source 可以尚未开始或正在开放，但不能已经 ended。它再次经过 Lottery verifier 和 exact-candidate approval verifier，然后追加 current+1 rollback publication。

Source window 已结束时，不允许修改 ends_at 后仍称 rollback；应构造新的 release candidate。若 source 尚未开始，rollback 仍精确复制原窗口并立即成为 active，gate 像普通 future release 一样返回 `scheduled`，不会继续旧 active。

Rollback 只影响 commit 后新解析的请求，不撤销已经读取旧 snapshot 的请求，不删除参与、Draw、库存或权益事实。

### 10. Retire 是保留历史的终态迁移

Retire 只允许 published->retired，使用 expected state CAS 和一次 canonical retired_at。Governance-owned verifier 必须核对绑定 ActivityID、expected state、last active version 与 retired_at 的 exact retirement intent，Repository 保存 bounded retirement evidence reference。最后 active version 保留，所有 publication/bindings 保留。Gate 对 retired 优先拒绝。

本节不实现紧急分布式中断。已经解析到旧 publication 的调用可能继续；正式 Draw 串联时必须决定强制重查/kill switch、版本快照和副作用停止点。

### 11. 时间门控采用一次 UTC Clock 与半开窗口

Publication `start < end`，所有时间为 UTC、无 monotonic component、可按 MySQL `DATETIME(6)` 无损往返。Gate 使用一次 controlled Clock：

```text
draft                       -> not_published / allow=false
retired                     -> retired       / allow=false
published && now < start    -> scheduled     / allow=false
published && start<=now<end -> active        / allow=true
published && now >= end     -> ended         / allow=false
```

`not_published | scheduled | active | ended | retired` 是 v1 唯一闭集 domain status。本节没有 HTTP/API，不同时冻结另一套 `activity_*` transport reason；未来 adapter 只能对这五个 status 做显式映射。

start inclusive、end exclusive。Browser time 和客户端 timezone 不可信；本地排期到 UTC/DST 的转换留给未来 API adapter。

`not_published` / `scheduled` / `ended` / `retired` 是 confirmed reject，`active` 是 confirmed allow。Not-found、损坏 snapshot、verifier/Repository/Clock error 或 cancellation 返回 zero decision + error，不伪装业务拒绝。

### 12. 审批归 Governance，Marketing 消费 exact evidence

Marketing application 定义 consumer-owned verifier port，输入完整 `ActivityPublicationCandidate`；retire 使用独立的 exact `ActivityRetirementCandidate`。可信 publication evidence 必须绑定 candidate 的 Activity/version、kind/source、graph ref、canonical Strategy ref set 与 window；retirement evidence 必须绑定 ActivityID、expected state、last active version 与 canonical retired_at。

修改 publication/retirement candidate 任一字段后必须重新验证。Publication evidence 与 retirement evidence 不得互相复用。Marketing 不接受浏览器 checkbox、自由文本“已批准”、caller role 或任意 token 自报。

本节不实现 verifier adapter、Principal 或权限；服务保持未装配。第 31～35 节必须在 access decision 允许之后才能调用发布服务。Approval 通过不等于当前 caller 获权，Authorization 允许也不等于 candidate 已批准。

### 13. 五张表与六个前向 DDL

Migration latest 5 -> 11：

| Version | DDL | 边界 |
| --- | --- | --- |
| 000006 | CREATE `lottery_strategy_snapshot` | Lottery exact Strategy revision header |
| 000007 | CREATE `lottery_strategy_snapshot_award` | Lottery exact revision child Awards |
| 000008 | CREATE `marketing_activity` | Marketing header/lifecycle/active/CAS |
| 000009 | CREATE `marketing_activity_publication` | immutable ActivityVersion/graph ref/window/kind/evidence |
| 000010 | CREATE `marketing_activity_publication_strategy` | terminal StrategyID -> exact StrategyRevision mapping |
| 000011 | ALTER `marketing_activity` | `(activity_id,active_version)` reverse FK to publication |

每个文件仍只有一个 DDL；`000001`～`000005` 不回写。

Lottery 内部可以建立：snapshot -> legacy Strategy family、snapshot award -> exact snapshot 的 FK。Marketing 内部建立：publication -> Activity、rollback source -> same Activity publication、binding -> publication、header active -> publication 的 FK。

Marketing publication 和 binding **不对 Lottery graph/snapshot 建 FK**。它们保存 exact primitive refs；跨上下文正确性由 verifier/resolve 契约负责。数据库仍以 CHECK 约束 ref 的非零、长度、ASCII/shape，但不宣称目标存在。

### 14. Active reverse FK 在本模型中可实现

Activity draft 先以 NULL active 插入。Publication 后续引用 Activity；同一 publish transaction 在 publication 存在后才更新 active。因此两个表创建完成后，000011 可以增加 reverse FK，不会形成第 28 节 graph/root 的首次插入死循环。

在数据库能可靠阻止 dangling active 时，不应只依赖应用检查。跨上下文 Lottery refs 与 Marketing 内部 active ref 的物理边界不能混为一谈。

### 15. 权限按 owned table 与动作收敛

Snapshot/publication/binding 是 immutable rows，运行 writer 不拥有 UPDATE/DELETE。Marketing write adapter 只对自己的 header 执行受控 UPDATE；不能修改 Lottery tables。Lottery snapshot writer 不能修改 Marketing。

本节只为隔离 Integration 身份配置最小测试 grants；长期 `growthos_app` 不新增 graph/snapshot/Activity 权限，因为没有 runtime composition。Migrator/DBA 旁路仍是高权限残余风险，由 strict restore、resolve fail closed、审计和运维停止线缓解。

### 16. Current read 使用一个 RR snapshot

Marketing current reader 在一个 read-only REPEATABLE READ transaction 中读取 header、active publication 和全部 bindings，仍在该事务内用已读行做严格 Restore；只有 Restore 成功才结束只读事务并返回。它不在两次独立请求间读取 header 与 child，也不依赖最大 publication version猜 active。

Lottery cross-context resolve 在得到完整 Marketing snapshot 后 exact 验证 refs。失败返回 zero resolution；不能尝试另一个 ActivityVersion 或当前 latest Strategy。

### 17. 按用例定义窄端口与错误

Lottery application 端口：`StrategySnapshotCreator.CreateSnapshot`、`StrategySnapshotReader.FindSnapshotByIdentity` 与 existing exact graph reader。Lottery 不拥有 publication verifier。

Marketing application 端口：ActivityDraftCreator、ActivityReader、ActivityCurrentReader、ActivityPublicationReader、atomic ActivityPublicationWriter、ActivityRetirer、ApprovalVerifier、consumer-owned `LotteryVerifier.VerifyPublication`、Clock。`adapter/lotteryconfig` 使用 Lottery-owned exact readers 与 domain validation 实现该 ACL。Application-owned recovery surface 另提供 `ActivityCommitReceiptFromError`、`ObserveCurrentActivity` / `ObserveActivityRoot` 与纯 `ReconcileActivityCommit`；它们不新增 generic Repository、网络调用或自动 retry。

不提供 generic Save/CRUD、partial binding writer、publication updater、latest graph/Strategy reader。

稳定错误至少区分 invalid/not-configured、not-found、stored-invalid、approval rejected/unavailable、Lottery binding invalid/unavailable、state/version conflict、repository retryable/failure、commit outcome unknown 与 caller cancellation/deadline。Gate allow/reject 是领域决定，不通过 error 字符串表达。

底层 SQL、table、credential、approval payload、graph/Strategy 内容不进入稳定公开错误。具体 HTTP status/envelope 尚未定义。

### 18. 第 30 节停止线

本节明确不装配或新增：

- `cmd/growth-api` Activity publication/gate service；
- app config、Compose env、长期 runtime DB grant；
- HTTP/MCP/Agent route、DTO 或公开错误映射；
- React 页面、导航、按钮或工作台；
- session、Principal、RBAC/ABAC、tenant、scope 或权限缓存；
- real Governance approval adapter；
- Participation eligibility、quota、会员 provider adapter；
- graph evaluator runtime、Strategy load、selector/random；
- formal Draw/Result、idempotency、inventory、Benefit、MQ；
- pause/resume、scheduled activation、gray release、multi-active；
- Redis cache、event publishing、persistent decision/audit UI。

现有 ephemeral API/React、Strategy by-ID cache、Participation chain、concrete router 与 graph evaluator 行为保持不变。

## 影响

### 正面影响

- Strategy/Award 历史第一次拥有可恢复的 exact business revision；
- Activity 生命周期与 Lottery 配置所有权清晰分离；
- graph topology 与 target Strategy content 都被 exact pin，不再猜 latest；
- 同配置可跨 Activity 复用，同 Activity 可追加版本演进；
- publication/rollback append-only，事故版本和恢复动作都可解释；
- state CAS 防止并发 lost update；
- `[start,end)` 和一次 Clock 固定时间边界；
- RR current snapshot 与原子 publish transaction 防止 header/child 混版；
- Marketing 内部 FK 保护 active/history，同时避免跨 bounded-context schema FK；
- 第 31～36 节可在真实 Activity/动作上建立公共访问控制和运营入口。

### 成本与限制

- 增加五张表、六个 Migration 版本与两个 bounded context 的 Repository；
- Strategy snapshot 复制 name/Awards，占用额外存储；
- Marketing DB 不能单独证明 Lottery refs 存在或 binding set 完整；
- 每次 publish/resolve 需要跨端口 exact 验证；
- v1 一个 active version，不能提前排期无缝切换；
- retired 不同步取消已经解析旧 snapshot 的请求；
- 没有真实 approval/auth/runtime，暂不能公开操作；
- 没有 tenant/public identity，Activity 仍是内部全局 ID；
- rollback 不能改变 source window，已经 ended 的 source 必须新 release；
- 没有正式 Draw，尚不能持久证明某次用户结果使用了哪个 publication。

### 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| StrategyID 被误当 exact content | Activity binding 强制 `(StrategyID,Revision)`，snapshot strict restore |
| graph terminal/binding set 不一致 | publish + resolve 两次 Lottery verifier exact equality |
| 无跨上下文 FK写入坏 ref | 最小权限、严格 verifier、坏 ref MySQL fixture、resolve fail closed |
| 并发 publish/rollback 覆盖 | expected lifecycle/state/active CAS + unique ActivityVersion |
| partial publication 可见 | publication+all bindings+header CAS 同事务 |
| commit error 被误重试 | zero result + trusted receipt + exact observation 三态对账；reconcile 不授权 retry |
| rollback 擦除事故证据 | append current+1 + rollback_of，旧行 RESTRICT/immutable |
| time boundary/DST 漂移 | UTC canonical microsecond + `[start,end)`；timezone conversion后置 |
| persisted running 状态漂移 | 只持久 draft/published/retired，phase按 Clock派生 |
| approval 与 candidate 错配 | verifier输入完整 exact candidate；变更字段重新审批 |
| approval 被当 authorization | docs/architecture guard；第31～35独立强制 access decision |
| retire 被夸大为同步撤回 | 明确 snapshot语义；正式主链再设计 kill/recheck |
| existing Redis cache 返回错误 revision | publication path不使用 by-ID cache；未来另做 versioned projection |
| high-privilege旁路改历史 | immutable grants、strict restore、审计与停止发布 |

## 数据库边界复盘

### 为什么 Marketing 不对 Lottery 建 FK

Activity publication 对 graph/snapshot 的依赖是业务 contract，不是“同一数据库中的永久 join”。物理 FK 会让：

- Marketing Migration 必须知道 Lottery 表名和复合 PK；
- Lottery schema 演进必须协调 Marketing DDL；
- 将来拆库/拆服务时先打破数据库约束才能部署；
- 团队误以为 FK 已证明 terminal closed set，而它实际上只能证明单行存在。

所以本节选择 exact typed refs + Marketing-owned ACL verifier，由它调用 Lottery-owned exact readers 和 domain validation。这个选择不是放弃完整性：每次 publish 和 current resolve 都必须验证，bad ref 不得进入 gate allow 或 evaluator。

### 为什么 Marketing 内部仍应使用 FK

Activity、publication 与 binding 同属 Marketing、共享生命周期和本地事务。Publication 不可能脱离 Activity；binding 不可能脱离 publication；active pointer 也只能指向同 Activity publication。这里 FK 精确表达 owned aggregate persistence，不造成 bounded-context 泄漏，因此应使用。

### 为什么 snapshot 不是缓存

Strategy snapshot 是权威 immutable publication history：

- 不可随意丢弃；
- exact revision 必须长期恢复；
- Activity FK-like contract 依赖它；
- 缺失是配置故障，不是 cache miss；
- 不能 TTL 过期或 fail-open 回当前 Strategy。

现有 Redis projection 仍只是 by-ID、可丢弃、TTL 有界的读取优化，不能承担 Activity version history。

## 并发和请求生命周期

- Services 构造后只持有 ports/Clock，不保存 current Activity；
- Candidate、approval evidence、exact refs、timer 和 transaction 都是调用局部值；
- Snapshot/revision/publication value objects immutable，collection access defensive copy；
- Repository pool 由 composition root 管理，本节 adapter 不拥有 Close；
- Concurrent readers 使用 RR snapshot，不加应用级全局锁；
- Concurrent writers 由 DB unique/CAS 仲裁，不使用 Redis lock；
- Retryable deadlock/lock-timeout 与 commit unknown 分开；
- Context cancel 在 commit 前阻止提交；commit 同时失败时按已定义 ownership 分类；
- Gate read 后 retire 可能发生，已返回 snapshot 不被异步改写；未来 orchestration 必须携带 ActivityVersion。

## 安全与威胁模型

信任边界包括：

- 未认证的未来 transport 输入；
- caller 提供的 Activity/graph/Strategy revisions；
- Governance approval provider；
- Lottery graph/snapshot repositories；
- Marketing MySQL rows；
- server Clock；
- migrator/DBA high privilege；
- future UI capability projection。

本节默认所有外部字符串、DB rows 和 provider outputs 均需校验。Exact revision 不等于有权限，approval evidence 不等于 session，Activity gate allow 不等于 Participation eligible。普通日志不记录 approval payload、Award/Weight、会员事实、credential 或 arbitrary revision label。

因为没有 Principal/RBAC，最重要的安全控制是**不装配写能力**。任何为了演示而把 publish route 暴露在 development/test 且无服务端 authorization 的做法都不属于本 ADR。

## 撤销与演进

若 Activity publication service 设计需要撤销，在没有 runtime consumer 时可以停止构造 application services；已经执行的 Migration 和已保存 immutable rows不能回写或删除，只能追加兼容 schema/归档协议。

线性演进保持为：

1. 第 27 节：concrete membership route；
2. 第 28 节：immutable graph persistence；
3. 第 29 节：exact closed evaluator；
4. 第 30 节：Strategy snapshot + Activity immutable publication binding；
5. 第 31 节：公共 Principal/resource/action/scope 与威胁边界；
6. 第 32 节：真实会话认证；
7. 第 33 节：服务端 RBAC 对 Activity/graph/Strategy 动作强制；
8. 第 34 节：前端 capability projection；
9. 第 35 节：direct API 与浏览器越权 E2E；
10. 第 36 节：运营后台消费受保护的服务端能力；
11. 第 45/51 节：正式抽奖主链与结果快照。

## 重新评估触发条件

出现以下任一真实证据时新增 ADR，而不是静默扩大 v1：

- 运营需要提前发布 future version 且旧 version 持续运行；
- 需要灰度、实验、多渠道或多个同时 active publication；
- 需要 pause/resume、草稿取消、审批状态机或 archive；
- rollback 需要独立重设 window 或跨 Activity 复制；
- emergency stop 必须阻止已经解析 snapshot 的请求；
- Strategy snapshot 规模/成本需要 content-addressing、去重或压缩；
- graph schema v2 需要 terminal 直接携带 Strategy revision；
- cross-context bad ref 事故表明 verifier/权限不足；
- Activity/Marketing 拆服务或独立数据库；
- 需要 tenant/business-space/object scope；
- 需要公开 Activity identity、slug 或防枚举策略；
- 需要 publication cache、compiled plan 或性能 SLO；
- 需要 durable approval/audit、签名、防篡改或合规保留；
- 需要 event/Outbox/MQ 传播 active publication；
- 正式 Draw 要求原子保存 Activity/graph/Strategy/fact snapshot。

## 验收证据

第 30 节必须能够证明：

1. Strategy snapshot exact identity、schema、完整 aggregate、canonical order、defensive copy 和 create-only；
2. 现有 Strategy path/cache/API 不被改成 versioned latest；
3. Activity draft/published/retired 形状和非法 transition 全覆盖；
4. ActivityVersion 从 1 连续追加，StateVersion 从 0 开始并满足 draft/published/retired 的 0、active、active+1 形状，同时独立承担 CAS 语义；
5. 普通 publish/rollback/retire 的 success/conflict/cancel/commit unknown 语义；仅 live commit unknown 可显式取得 defensive receipt，其余失败无 receipt 且全部返回 zero domain result；
6. rollback exact复制 source graph/Strategy/window并记录 source；scheduled/open source 允许，`evaluated_at >= source.ends_at` 拒绝；
7. retired 保留 active history且不能恢复；
8. exact graph terminal set 与 Strategy revision binding set 的 equal/missing/extra/duplicate/wrong-identity fixtures；
9. approval verifier 对 exact publication candidate / retirement intent，修改任一字段后旧 evidence 不可复用，二者 evidence 也不能互换；
10. UTC microsecond canonicalization与start-1µs/start/end-1µs/end gate；
11. confirmed rejection 与 technical zero decision 分离；
12. 两个 concurrent publishers恰一个成功，最终只有一个完整 active version；
13. child insert/header CAS/commit故障不暴露半 publication；commit unknown receipt 通过 exact current/root observation 只产生 committed/not_committed/indeterminate，later generation 与坏/缺/partial input 均 fail closed；
14. Marketing current RR snapshot不混 header/publication/bindings；
15. Migration latest 5->11、repeat no_change、dirty fail closed、旧五表指纹不变；
16. MySQL PK/CHECK、Lottery内部FK、Marketing内部FK、000011 active reverse FK和RESTRICT；
17. information_schema 明确没有 Marketing->Lottery graph/snapshot跨上下文FK；
18. isolated writers拥有最小 grants，immutable UPDATE/DELETE 与跨上下文写入被拒；长期 runtime grants不变；
19. corruption/dangling cross-context refs在 verifier/resolve 时失败关闭；
20. domain/application/repository normal/race/fuzz/real MySQL tests通过；
21. architecture guard证明无 HTTP/UI/runtime/auth/selector/Draw/Redis/MQ 越界；
22. 每次失败返回 zero/empty publication、binding或gate decision，不泄露 partial state。

## 相关资料

- [Activity 发布与 Lottery 配置精确绑定基线 v1](../product/activity-publication-binding-v1.md)
- [运营人员工作流 v1](../product/operator-workflow-v1.md)
- [限界上下文地图 v1](../product/bounded-context-map-v1.md)
- [Lottery 业务规则需求基线 v1](../product/lottery-rule-requirements-v1.md)
- [ADR-0014：Lottery Strategy/Award 首个持久化结构](ADR-0014-lottery-persistence-schema.md)
- [ADR-0019：Lottery 规则所有权与评估边界](ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- [ADR-0024：Lottery Strategy 路由图持久化](ADR-0024-lottery-strategy-routing-graph-persistence.md)
- [ADR-0025：Lottery Strategy 路由图求值](ADR-0025-lottery-strategy-routing-graph-evaluation.md)
