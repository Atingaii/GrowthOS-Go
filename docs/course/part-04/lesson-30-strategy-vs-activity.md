# 第 30 节：为什么 Strategy 不等于 Activity——用不可变发布版本精确绑定 Lottery 配置

> 第 29 节已经能在调用方明确给出 exact graph revision 时形成一条可信 Strategy route，但它不知道“哪场活动、在什么时间、应该使用哪套 Lottery 配置”。本节新增 Marketing-owned `Activity`、Lottery-owned immutable Strategy snapshot，以及 append-only publication/CAS 协议。它只建立未装配的内部发布基线，不公开运营 API，不实现真实审批、认证、RBAC、前端、正式 Draw、库存或发奖。

- **学习分支：** `codex/lesson-30-strategy-vs-activity`
- **日期：** 2026-08-30
- **产品规格：** [Activity 发布与 Lottery 配置精确绑定基线 v1](../../product/activity-publication-binding-v1.md)
- **架构决策：** [ADR-0026：Activity 以不可变版本绑定 Lottery 精确配置](../../decisions/ADR-0026-activity-publication-binding.md)
- **API 记录：** [第 30 节 API](../../api/lessons/lesson-30.md)
- **QA：** [第 30 节 QA](../../qa/lessons/lesson-30.md)
- **设计手记：** [第 30 节第一性原理手记](../../design-thinking/lessons/lesson-30.md)
- **面试问答：** [第 30 节面试问答](../../interview/lessons/lesson-30.md)
- **运行手册：** [Activity publication 验收与故障分诊](../../runbooks/activity-publication.md)
- **证据状态：** 最终候选的 normal/race/shuffle/fuzz/coverage/real MySQL/Compose/`make verify`、root 最终检查与清理均已真实执行；accepted implementation/documentation candidate 为 `887754bb192d9850d231729391152cd8678c11ad`，同名远端学习分支与线性历史已核对

## 1. 本节真正补上的缺口

第 29 节的输入仍然是调用方已经知道的：

```text
exact (GraphID, GraphRevision)
  + authoritative membership fact
  + one evaluated-at
  -> exact StrategyID route decision
```

这条链无法回答：

1. 哪个 Activity 当前应该被解析；
2. Activity 是未发布、排期中、开放、结束还是永久退役；
3. graph terminal 的 `StrategyID` 应读取哪一份 Award/Weight 内容；
4. 两个运营人员同时发布时谁赢；
5. 回滚后怎样保留事故版本和回滚动作本身；
6. current header、publication 与 Strategy bindings 怎样避免混版。

第 30 节建立的最小答案是：

```text
Marketing Activity root
  -> exact active Activity publication version
  -> exact Lottery graph (ID, revision)
  -> exact terminal Strategy snapshot manifest
  -> one server Clock
  -> not_published | scheduled | active | ended | retired
```

这里的 `active` 只表示 Marketing 时间门控允许继续。它不表示调用者有权限、参与者有资格、随机选择已发生、库存可用或权益已经发放。

## 2. 为什么 Strategy 不是 Activity

先从变化原因判断对象边界，而不是从页面或表名判断。

| 对象 | 回答的问题 | 所有者 | 主要变化原因 |
| --- | --- | --- | --- |
| Activity | 为什么做、何时开始/结束、哪次发布对新解析生效 | Marketing | 运营排期、发布、止损、退役 |
| Strategy snapshot | 候选 Award、权重与 outcome 是什么 | Lottery | 奖项/权重配置变化 |
| Strategy Routing Graph | 根据已确认 Lottery 事实路由到哪个 Strategy family | Lottery | rule、branch、target 拓扑变化 |
| Approval | exact candidate 是否经过可信审批 | Governance | 审批政策、职责分离、审计变化 |
| Authorization | 当前 Principal 能否执行某动作 | Governance / Access Control | 身份、角色、资源、scope 变化 |

若把 Activity 时间窗放进 Strategy：

- 同一 Strategy 无法被两个不同排期的 Activity 复用；
- Activity 回滚会迫使 Lottery aggregate 改版本；
- selector/cache 被迫理解运营生命周期；
- 审批、权限和退役会污染 Award/Weight 模型。

若把 Activity 放进 graph header，immutable topology 同样会被 Marketing 的 active pointer 污染。

因此本节采用“精确引用，不跨上下文吞并”：Marketing publication 保存 primitive exact refs；Lottery 继续拥有 graph 与 Strategy 内容。

## 3. 交付切片总览

### 3.1 Lottery exact Strategy snapshot

[strategy_snapshot.go](../../../internal/lottery/domain/strategy_snapshot.go)新增：

- `StrategySnapshotIdentity = (StrategyID, StrategyRevision)`；
- 独立 `StrategySnapshotSchemaVersionV1`；
- 完整 immutable Strategy/Award snapshot；
- Award canonical 排序、防御性复制、严格恢复；
- revision token 与 schema/cache/Migration version 分离。

[strategy_snapshot_repository.go](../../../internal/lottery/adapter/mysqlrepo/strategy_snapshot_repository.go)只提供 create-only 与 exact read，不提供 latest、update、upsert 或 delete。

现有 by-ID Strategy、Redis projection 和 ephemeral selection 没有被悄悄改成 versioned current。

### 3.2 Marketing domain

[activity.go](../../../internal/marketing/domain/activity.go)定义 compact root：

```text
ActivityID + canonical name
  + draft | published | retired
  + stateVersion
  + activePublicationVersion
  + retiredAt / retirementReference
```

[publication.go](../../../internal/marketing/domain/publication.go)定义 immutable publication：

```text
ActivityID + numeric ActivityPublicationVersion
  + schema v1
  + release | rollback
  + optional rollbackOf
  + exact graph ref
  + canonical unique Strategy revision manifest
  + [startsAt, endsAt)
  + publishedAt
  + approvalEvidenceReference
```

[transition.go](../../../internal/marketing/domain/transition.go)提供纯规划：

- `PlanPublish`；
- `PlanRollback`；
- `PlanRetire`；
- 带 expected lifecycle/state/active CAS 谓词的 `ActivityTransition`。

[gate.go](../../../internal/marketing/domain/gate.go)提供 confirmed gate decision：

- `not_published`；
- `scheduled`；
- `active`；
- `ended`；
- `retired`。

任何技术失败都返回 zero decision + error。

### 3.3 Marketing application

[application](../../../internal/marketing/application)按真实用例拆出：

- `CreateDraftService`；
- `PublishActivityService`；
- `RollbackActivityService`；
- `RetireActivityService`；
- `ResolveActivityService`。

服务使用 consumer-owned ports：Activity readers/writers、Lottery verifier、Approval verifier 与 Clock。它们存在于内部包，但没有装配到 `cmd/growth-api`。

### 3.4 Cross-context ACL

[lotteryconfig verifier](../../../internal/marketing/adapter/lotteryconfig/verifier.go)是 Marketing 到 Lottery 的 anti-corruption adapter：

1. 将 Marketing 本地 primitive ref 转换为 Lottery identity；
2. exact 读取 graph；
3. 收集 canonical unique terminal StrategyID set；
4. 与 candidate manifest 做集合完全相等校验；
5. exact 读取并 Validate 每份 Strategy snapshot；
6. 任一缺失、损坏、错 identity 或依赖失败都不返回 partial success。

它不会查询 latest revision，也不会在缺失时继续旧配置。

### 3.5 MySQL persistence

[Marketing MySQL repository](../../../internal/marketing/adapter/mysqlrepo/repository.go)实现：

- create-only draft；
- exact Activity read；
- exact historical publication read；
- root + active publication + manifest 的单个 read-only RR snapshot；
- publication/header CAS 原子事务；
- retirement CAS；
- stable repository error classification 与 commit outcome unknown。

本节前向新增 Migration `000006`～`000011`：

| Migration | 单一 DDL 责任 |
| --- | --- |
| `000006` | Lottery Strategy snapshot header |
| `000007` | exact Strategy snapshot Awards |
| `000008` | Marketing Activity header |
| `000009` | immutable Activity publication |
| `000010` | publication Strategy revision bindings |
| `000011` | Activity active publication reverse FK |

旧 Migration 不回写。

## 4. 版本为什么必须分开

本节至少同时出现六种“版本”：

| 名称 | 语义 | 谁推进 |
| --- | --- | --- |
| `ActivityStateVersion` | Activity header CAS generation | publish/rollback/retire |
| `ActivityPublicationVersion` | 同一 Activity 下 append-only publication identity | publish/rollback |
| `GraphRevision` | exact routing topology | Lottery graph owner |
| `StrategyRevision` | exact Strategy/Award content snapshot | Lottery Strategy owner |
| schema version | 持久内容解析形状 | code/Migration |
| Migration version | 数据库结构演进位置 | migrator |

状态形状故意可校验：

```text
draft:     stateVersion = 0, activeVersion = none
published: stateVersion = activeVersion > 0
retired:   stateVersion = activeVersion + 1
```

published 状态下两个数字相等，不代表语义相同。一个是并发 token，一个是历史业务 identity；retire 不追加 publication，却仍推进 stateVersion，正好证明二者不能混称。

## 5. Activity 生命周期为什么只有三态

合法迁移只有：

```text
draft --publish v1--------------------------> published
published --publish v(n+1)------------------> published
published --rollback old -> append v(n+1)---> published
published --retire--------------------------> retired
```

明确禁止：

- draft 没有 publication 却变成 active；
- published 回 draft；
- retired 再 publish/rollback/resume；
- 自然到达 `endsAt` 就持久化 `ended`；
- 当前没有需求却发明 pause、cancel、archive、gray 等状态。

`scheduled/active/ended` 是“时间相位”，不是 Activity lifecycle。将它们持久化会创建第二个会随 Clock 漂移的真相。

## 6. 普通发布的固定顺序

`PublishActivityService` 的顺序是：

```text
validate command
  -> read exact current Activity
  -> check expected stateVersion
  -> Clock once -> canonical publishedAt
  -> pure provisional PlanPublish
  -> form exact candidate
  -> Lottery exact closure verification
  -> Governance approval verification
  -> rebuild plan with returned evidence
  -> prove candidate unchanged
  -> one repository CAS transaction
```

临时 `application-plan` evidence 只用于纯规划，不会传给外部依赖或写入数据库。审批返回后，服务重新生成 publication，并逐字段证明审批前后 candidate 没有漂移。

Repository 事务必须一起完成：

1. INSERT publication；
2. INSERT 全部 Strategy bindings；
3. CAS UPDATE Activity header；
4. COMMIT。

CAS 失败时事务回滚，不能遗留 orphan publication 或 half manifest。

## 7. 为什么 publication 必须 immutable

若 active 配置直接存在 Activity header 并允许 UPDATE：

- 当前请求和历史请求无法解释读取了哪份配置；
- 审批 evidence 可能与被修改后的内容不一致；
- 回滚会擦除事故版本；
- cache key 与审计无法引用稳定 identity；
- 并发写更容易变成 last-write-wins。

append-only publication 的代价是多占存储，但它把每次 release/rollback 变成可引用事实。Header 只选择 active version，不复制可变内容。

## 8. 为什么 StrategyID 仍不够

第 28 节 graph terminal 只保存 `StrategyID`，这只说明 route family。若 Activity publication 也只保存 StrategyID，那么以后 Strategy Award/Weight 变化时，历史 Activity 会随 by-ID 读取漂移。

本节新增 exact snapshot：

```text
(StrategyID, StrategyRevision)
  -> schema v1
  -> canonical immutable Strategy + Awards
```

Revision 是 create-only registry 中的业务 identity，不自动等于内容哈希。若未来需要签名、跨环境 promotion 或 content addressing，需要新的 canonical encoding 与 ADR，而不是把普通 token 口头升级成密码学证明。

## 9. 为什么 manifest 必须是闭合集

一份 publication 不能只绑定“常见 terminal”。它必须满足：

```text
unique terminal StrategyID set(exact graph)
  == StrategyID set(exact revision manifest)
```

这意味着：

- missing 拒绝；
- extra 拒绝；
- duplicate/ambiguous revision 拒绝；
- zero identity 拒绝；
- exact Strategy snapshot not-found/invalid 拒绝；
- 同一 StrategyID 在 manifest 中只能有一份 revision。

Manifest 最多 128 条并按 StrategyID canonical 排序。Domain 负责形状、唯一性、上限和防御复制；Lottery verifier 负责 graph terminal set 与 snapshot 存在性。

## 10. 回滚为什么追加新版本

错误做法：

```text
active v3 -> 把指针改回 v1
```

这样无法区分“最初 v1 生效时期”和“事故后恢复 v1 内容的新时期”，也没有新的 publishedAt、审批证据或单调版本。

本节的回滚是：

```text
active v3
  + historical source v1
  -> append rollback v4 (rollbackOf=v1)
  -> copy v1 graph + Strategy manifest + window
  -> new publishedAt + new approval evidence
  -> CAS active=v4
```

目标必须：

- 属于同一 Activity；
- 版本严格早于 current active；
- 能 exact restore；
- 已经是历史 publication；
- 在本次 Clock 上满足 `publishedAt < target.endsAt`。

已结束目标不能通过修改 window 后仍称 rollback；那是新的 release。

## 11. 回滚不撤销已经发生的事实

Publication 切换只影响事务提交后重新解析 current Activity 的请求。它不会自动撤销：

- 已形成的 Participation fact/decision；
- 已读取旧 publication 的进行中请求；
- 已形成的 route decision；
- 未来正式 Draw/Result；
- 库存扣减、权益发放或外部消息。

本节还没有正式 Draw 链，因此不能宣称“全链路业务回滚”。未来必须让业务结果保存 exact Activity/graph/Strategy/fact snapshot，并明确补偿而非假装时间倒流。

## 12. Retire 为什么保留 active pointer

Retire 是 terminal root transition：

```text
published stateVersion=n, active=n
  -> retired stateVersion=n+1, active仍为n
  + retiredAt
  + retirementReference
```

保留 active version 的原因是历史解释，不是继续开放。Gate 对 retired 优先返回 confirmed retired；旧 publication、rollback history 和 bindings 都不删除。

本节不提供分布式 kill switch。已经解析出旧 snapshot 的请求可能继续，未来正式主链必须另行决定每个副作用前是否重查 retire。

## 13. `[start,end)` 怎样形成唯一边界

所有 publication 时间在构造时规范为：

```text
UTC + microsecond precision + no monotonic component
```

这是为了与 MySQL `DATETIME(6)` 无损往返。半开窗口规定：

| 条件 | confirmed status | allow |
| --- | --- | --- |
| draft + zero publication | `not_published` | false |
| retired | `retired` | false |
| `now < startsAt` | `scheduled` | false |
| `startsAt <= now < endsAt` | `active` | true |
| `now >= endsAt` | `ended` | false |

start inclusive、end exclusive 让相邻窗口可以无重叠拼接，并避免“恰好 end 算哪边”的歧义。

Gate 每次只消费一个受控 Clock instant。浏览器时间、用户 timezone、数据库 `NOW()` 或后台 timer 都不是领域权威。

## 14. Confirmed decision 与技术错误必须分开

以下是已形成的业务决定，不是 error：

- draft -> `not_published`；
- future window -> `scheduled`；
- open window -> `active`；
- reached exclusive end -> `ended`；
- retired root -> `retired`。

以下必须返回 zero decision + error：

- Activity not-found；
- persisted root/publication/manifest 损坏；
- active pointer 与 publication 不匹配；
- Lottery graph/snapshot 缺失或闭合集不一致；
- Clock 返回 zero；
- Repository/provider failure；
- caller cancel 或内部 deadline。

把技术故障伪装成 `ended` 会让监控误以为业务正常拒绝，也可能掩盖配置损坏。

## 15. 为什么 publish 与 resolve 都验证 Lottery

Publish 前验证能阻止正常写路径提交坏引用，但仍存在：

- 高权限 DBA/migrator 旁路；
- 历史 bug；
- 外部 Lottery 数据被错误删除或破坏；
- 恢复/导入流程绕过应用。

因此 `ResolveActivityService` 在 gate 前重新验证 active publication 的 exact graph 与 Strategy snapshots。验证失败不降级到 previous/latest，也不返回业务拒绝；它是 technical failure。

双验证增加读取成本，换来无跨上下文 FK 情况下的 fail-closed 防线。若未来需要 cache，cache key 也必须包含完整 exact identities，且不能改变缺失语义。

## 16. 为什么 Marketing 不对 Lottery 建物理 FK

Marketing 内部使用 FK：

- publication -> Activity；
- binding -> publication；
- rollback source -> same Activity publication；
- Activity active pointer -> same Activity publication。

这些对象属于同一事务/生命周期。

Marketing 对 Lottery graph/snapshot 则只保存 exact primitive refs，不建跨上下文 FK。原因是：

- Marketing Migration 不应固化 Lottery 表结构；
- 未来拆库/拆服务不应先破坏 DDL；
- 单行 FK 也证明不了 terminal set 与 manifest 完全相等；
- Marketing-owned ACL verifier 才能通过 Lottery-owned exact readers 与 domain validation 执行完整业务校验。

这不是放弃完整性，而是把完整性放到真正能表达它的边界。

## 17. CAS 如何处理并发发布

两个调用方都从 stateVersion 3 规划 version 4：

```text
publisher A: expected state=3, active=3 -> proposed v4
publisher B: expected state=3, active=3 -> proposed v4
```

数据库事务使用 expected lifecycle/state/active 条件更新 header。最多一个调用影响一行；loser 得到 stable state conflict，且其 publication/bindings 随事务回滚。

这里不需要 Redis 分布式锁。数据库已经是 authoritative serialization point；额外锁会引入租约、过期、双写和锁/事务顺序问题，仍不能替代 unique key 与 CAS。

## 18. RR current snapshot 为什么必要

错误读法：

```text
SELECT Activity header  -- 读到 active v2
另一个事务发布 v3
SELECT publication      -- 若猜 latest，读到 v3
```

本节 current reader 在一个 read-only REPEATABLE READ transaction 中读取：

1. Activity root；
2. root 明确指向的 exact publication；
3. 该 publication 的全部 Strategy bindings。

它从不使用 `MAX(version)` 或 latest fallback。全部行在同一 RR 事务中读完后立即 strict Restore，只有 Restore 成功才结束只读事务并返回；任何缺行、超限、错 identity 或非 canonical state 都失败关闭。

## 19. Commit outcome unknown 为什么不能盲重试

如果 COMMIT 时连接中断，客户端可能不知道事务究竟：

- 已经 durable，但响应丢失；
- 没有提交；
- 状态仍需 exact read-back 才能判断。

盲目重试 publish 可能基于新 active 追加另一个版本。Repository 因而把 caller cancel、retryable transaction failure 与 `ErrCommitOutcomeUnknown` 分开。

Application 仍把 publish/rollback 的业务结果返回为 zero publication，把 retire 的业务结果返回为 zero
Activity；error 不等于成功结果。只有 operation class 精确为 `ErrCommitOutcomeUnknown`、caller 与
operation context 在 repository 返回后均仍存活，且本次 transition 能构造可信 receipt 时，内部恢复流程
才可显式调用：

```go
receipt, ok := application.ActivityCommitReceiptFromError(err)
```

`ActivityCommitReceipt` 是不可变、防御复制的 exact attempt 描述：它保留 operation、before root、after
root；publish/rollback 还保留完整 publication，retire 的 after root 则保留本次 server-owned retiredAt
与 retirement evidence；rollback receipt还必须自证`rollbackOf < before.active`。Receipt 不进入
`Error()`、`errors.Is` 或普通 unwrap 链；retryable/conflict/
storage/context error 也不能取得它。

Read-back 必须使用新健康连接和 exact reader。Publish/rollback 读取同一 RR current snapshot 的 root 与
active publication，构造 `ObserveCurrentActivity`；retire exact读取 root 并构造
`ObserveActivityRoot`。随后纯函数：

```go
result := application.ReconcileActivityCommit(receipt, observation)
```

只返回以下闭合三态：

| result | 可证明事实 | 处置边界 |
| --- | --- | --- |
| `committed` | exact after root 相等；publish/rollback 的完整 publication 也逐字段相等 | 采用已提交事实，不追加版本 |
| `not_committed` | exact before root仍在，或同一 next generation 已由另一合法 winner 占据 | 只能由上层决定是否重新发起一项新操作 |
| `indeterminate` | missing/invalid/mismatched/partial observation，或状态已推进到更晚 generation | 停止写入并人工调查，不能猜测先前是否提交 |

这个函数不做 I/O、不读取 Clock、不重新审批，也不推荐 retry。正确恢复动作是先保存 receipt，再按 exact
identity 读取、构造 observation 并对账；不得使用 latest、时间戳相似度或重新生成 candidate。运行步骤见
[故障分诊手册](../../runbooks/activity-publication.md)。

## 20. Approval 为什么仍不等于 Authorization

本节只有 `ApprovalVerifier` consumer port，没有真实 Governance adapter。Evidence reference 是 bounded pointer，不是：

- session；
- Principal；
- role/permission；
- 当前 caller 的 authorization proof；
- 审批 payload 本身；
- 双人复核已上线的证明。

Publish/rollback/retire service 必须在未来服务端 authorization allow 之后才能被调用。反方向也成立：caller 有权限不代表 exact candidate 已批准。

第 31～35 节将按公共权限模型、真实会话、服务端 RBAC、前端 capability projection 与越权 E2E 逐步补上这条边界；本节不提前伪造。

## 21. Context 与低披露错误

Application service 为每次调用建立 private maxDuration：

```text
caller cancellation/deadline
  > internal private deadline attribution
  > dependency/domain classification
```

每个依赖边界后先检查 caller/operation context。本节新增的 Marketing operation/dependency/repository wrappers 只通过普通 `Error()`/`errors.Is` 公开 reviewed class，不向未来 transport 展开 SQL、credential、table、approval payload、graph 或 Strategy/Award 内容；可信诊断代码可显式读取 retained cause。这不是对既有 Lottery repository error contract 的全局改写。

`maxDuration` 是 cooperative safety budget，不是硬抢占、P99 或生产 SLO。当前服务也没有 runtime 默认值。

## 22. 建议的学习实验顺序

### 实验一：只看 Strategy snapshot

阅读：

- [strategy_snapshot.go](../../../internal/lottery/domain/strategy_snapshot.go)；
- [strategy_snapshot_repository.go](../../../internal/lottery/adapter/mysqlrepo/strategy_snapshot_repository.go)。

回答：revision、schema、legacy StrategyID 和 Redis codec version 为什么不同。

### 实验二：只跑纯 domain

```bash
go test -count=1 -shuffle=on ./internal/marketing/domain
```

观察 draft/published/retired 形状、版本溢出、防御复制、rollback copy 与 `[start,end)` 边界。

### 实验三：跟一遍 publish candidate

从 [publish.go](../../../internal/marketing/application/publish.go)逐步确认：

```text
provisional plan
  -> Lottery verify
  -> approval evidence
  -> rebuild exact plan
  -> compare candidate
  -> repository CAS
```

### 实验四：制造 concurrent conflict

让两个 transition 使用同一 expected state/active，验证 repository winner/loser 语义，并确认 loser 没有 orphan publication/bindings。

### 实验五：逐点走 gate

固定 start/end，分别传入 start-1µs、start、end-1µs、end，再测试 draft zero publication 与 retired root-only。

### 实验六：检查没有偷偷上线

```bash
rg -n 'PublishActivityService|RollbackActivityService|RetireActivityService|ResolveActivityService|ActivityPublicationWriter|internal/marketing' \
  cmd/growth-api internal/infrastructure/httpapi web/src deploy
```

正确结论应是没有 Activity public route、DTO、页面、导航或 composition root。

## 23. 验收命令与证据边界

本节的完整验收矩阵见[第 30 节 QA](../../qa/lessons/lesson-30.md)。常用命令包括：

```bash
go test -count=1 -shuffle=on \
  ./internal/lottery/domain \
  ./internal/lottery/application \
  ./internal/lottery/adapter/mysqlrepo \
  ./internal/marketing/domain \
  ./internal/marketing/application \
  ./internal/marketing/adapter/lotteryconfig \
  ./internal/marketing/adapter/mysqlrepo \
  ./migrations

go test -race ./internal/marketing/...
GROWTHOS_LESSON30_MYSQL_ACCEPTANCE=run-disposable-mysql-8.4.11 \
  make lesson30-mysql-acceptance
go test ./...
go run ./cmd/doccheck
```

`lesson30-mysql-acceptance` 是有显式确认口令、随机命名、tmpfs MySQL 8.4.11 与清理保护的隔离门禁；它不是长期 Compose runtime。

此前候选已经形成以下真实执行证据：

- disposable MySQL 8.4.11 门禁连续两次完整 exit 0，覆盖真实 v5 baseline、v5→v11、重复 no-change、
  dirty-state fail-closed、五张新表/owned FK/20 个 CHECK/binary collation、隔离 writer grant 与 1142 拒绝、
  snapshot create/exact RR/并发、Activity publish/replace/rollback/retire/CAS/half-write rollback；临时
  container、volume、network、identity 与 secret 均核对为零残留；
- 长驻 Compose 在原 MySQL container、volume 与 network identity 不变的前提下从 schema `5:0` 升级到
  `11:0`；旧五张业务表行数/checksum不变，长期 `growthos_app` 仍仅能 SELECT 旧两张表，对八张未装配
  业务表继续得到 1142；`make compose-status` 与 `make compose-smoke` 均 exit 0；
- 独立 Lottery Compose acceptance exit 0，覆盖 Redis poison repair、并发、Redis/MySQL 故障降级与恢复、
  三种 M1 场景；随机 project 的容器、volume、network、镜像、BuildKit 与 secret 已清理，长驻资源 identity
  未变；
- `make verify` 已在该冻结前候选上 exit 0，覆盖 Go vet/test/doccheck、Web 152 tests、typecheck 与生产
  build。

Commit receipt/reconciliation 收口后，完整最终候选又真实执行并通过：

- 环境为 Go `go1.26.6`，`GOMOD` 指向当前 GrowthOS-Go 的 `go.mod`，`GOWORK` 为空；
- 定向 normal、Marketing/Lottery 定向 race 及全仓 `go test -race -count=1 ./...` 均 exit 0；
- 固定 seed `1788183001`～`1788183020` 的 20 轮 shuffle 全部 exit 0；
- 三个 Marketing domain fuzz target 各运行 10 秒，exec 分别为 `2,019,397 / 1,087,335 /
  1,469,462`，new interesting 均为 0，total corpus 分别为 `11 / 44 / 10`；这些是复现记录，不是性能
  或质量 KPI；
- 最终 statement coverage：Lottery domain `93.5%`、application `88.3%`、MySQL adapter `81.8%`；
  Marketing domain `96.0%`、application `77.1%`、Lottery ACL `81.9%`、MySQL adapter `84.8%`；
- `make verify` exit 0，覆盖 Go vet/test/doccheck、Web 19 files / 152 tests、typecheck 与 Vite production
  build（2462 modules）；
- disposable MySQL 8.4.11 在最终候选再次 exit 0，最终 schema `11:0`、probe `0`、临时资源 cleanup
  `0`，长驻资源不变；`make compose-status` 为 clean 11/latest 11，`make compose-smoke` exit 0 且资源
  identity 保持。

因此代码与数据库门禁不再处于“待复跑”。Root 在全部索引/文档收口后又完成 architecture、doccheck、
diff、任务 build artifact 清理、远端同名 ref 与线性历史核对；`887754bb192d9850d231729391152cd8678c11ad`
被接受为完整实现/文档候选。本次随后追加的 freeze-attestation 提交只登记这一验收事实，不改变候选代码、
schema 或运行时边界。Fuzz exec 与 corpus 数依机器、语料和 cache 变化，只用于记录本次执行。

## 24. 第 30 节没有新增公开 API

本节没有：

- `/api/v1/activities` route；
- publish/rollback/retire/resolve HTTP DTO；
- status/error mapping；
- Activity response header；
- React Activity 页面或运营后台；
- Compose env 或 runtime DB grant；
- MCP/Agent action。

内部 service 的存在不等于网络能力。完整记录见[第 30 节 API](../../api/lessons/lesson-30.md)。

## 25. 可以准确写进简历的边界

可以准确说：

> 在 Go 分层架构中建模 Marketing Activity 与 Lottery Strategy 的限界上下文边界，引入 create-only Strategy snapshot、append-only Activity publication、exact graph/Strategy revision closure、乐观 CAS、RR current snapshot、追加式回滚与 UTC 半开时间门控，并通过 Marketing-owned ACL verifier 隔离跨上下文依赖。

不能说：

- 已上线运营平台；
- 已实现真实审批、会话、RBAC 或多租户；
- 已支持灰度、自动定时发布、自动告警回滚；
- 已实现正式抽奖、库存、发奖或 MQ；
- 已证明生产并发、SLO、审计合规或零故障。

## 26. 第 31 节为什么自然出现

第 30 节第一次出现高风险资源和动作：

```text
Activity: create / publish / rollback / retire / read
Lottery snapshot: create / verify / read exact
```

但当前没有 Principal、Resource、Action、Scope 或 threat boundary。若直接暴露 API，任何调用方都可能改变 active publication。

因此下一步不是先画运营页面，而是建立公共权限模型与威胁边界，再按会话、服务端 RBAC、前端裁剪和越权 E2E 顺序演进。

## 27. 本节复盘

第 30 节最重要的不是多了几张表，而是建立四条不可混淆的真相：

1. Marketing Activity 生命周期不属于 Lottery Strategy；
2. graph topology exact 不代表 terminal Strategy content exact；
3. rollback 是新的历史事实，不是把时间倒回去；
4. gate business decision、技术 failure、approval 与 authorization 必须正交。

在这些边界上，immutable snapshot、append-only publication、exact closure、CAS、RR snapshot 和一次 Clock 才各自找到正确位置。当前切片已经能作为内部架构学习样本，但仍故意保持未装配，直到后续权限与正式业务主链完成。
