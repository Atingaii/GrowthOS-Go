# 第 30 节 QA：Activity publication 与 exact Lottery 配置绑定验收

- **课程主题：** 为什么 Strategy 不等于 Activity
- **需求基线：** [Activity 发布与 Lottery 配置精确绑定基线 v1](../../product/activity-publication-binding-v1.md)
- **架构决策：** [ADR-0026](../../decisions/ADR-0026-activity-publication-binding.md)
- **上一节：** [第 29 节 QA](lesson-29.md)
- **课程正文：** [第 30 节课程](../../course/part-04/lesson-30-strategy-vs-activity.md)
- **API 记录：** [第 30 节 API 记录](../../api/lessons/lesson-30.md)
- **设计手记：** [第 30 节设计手记](../../design-thinking/lessons/lesson-30.md)
- **面试问答：** [第 30 节面试问答](../../interview/lessons/lesson-30.md)
- **运行手册：** [Activity publication 验收与故障分诊](../../runbooks/activity-publication.md)
- **证据日期：** 2026-08-31（完整最终候选、root 收口与远端冻结真实记录）；accepted implementation/documentation candidate：`887754bb192d9850d231729391152cd8678c11ad`

> 本节验收未装配的内部发布内核：Marketing Activity 以不可变 publication 精确绑定 Lottery graph 与全部 terminal Strategy revision，并通过 CAS、strict restore、一次受控 Clock 和 fail-closed verifier 形成可核查结果。它不验收运营 API、真实审批、身份、RBAC、UI、正式 Draw、库存、发奖或 MQ。

## 1. 证据状态词汇

| 状态 | 精确定义 |
| --- | --- |
| **IMPLEMENTED-SURFACE** | 当前工作树中已有实现与测试代码，可进行代码审查；不代表命令已在最终候选上通过 |
| **EXECUTED-CANDIDATE-EVIDENCE** | 命令已在完整最终候选上真实执行并保留结果；只证明所列环境与执行路径 |
| **REQUIRED-GATE** | root冻结前仍必须实际执行并记录的最终architecture/diff/cleanup/ref检查 |
| **FINAL-FREEZE-VERIFIED** | 代码门禁、root最终检查/清理、accepted candidate、远端同名ref与线性历史均已核对；freeze-attestation只登记结果 |
| **OUT-OF-SCOPE** | 本节刻意不交付；其他命令通过也不能推导该能力已实现 |
| **NOT-CLAIMED** | 没有足够证据形成通过、容量、SLO、生产或合规结论 |

`go test ./...` 可能使用 Go cache，不会自动运行 fuzz，也不会证明真实 MySQL grant、浏览器、认证授权或生产并发。本节禁止用固定测试数量、fuzz exec 数或“全部通过”替代可复核命令证据。

## 2. 实现面清单

### 2.1 Lottery-owned exact Strategy snapshot

| 面 | 当前代码位置 | 冻结前要证明 |
| --- | --- | --- |
| exact identity | `internal/lottery/domain/strategy_snapshot.go` | `(StrategyID, StrategyRevision)` 缺一不可，revision grammar bounded |
| independent schema | 同上 | schema v1 与业务 revision、Activity version、Migration version分离 |
| complete immutable aggregate | 同上 | Strategy name/Awards/weights/outcomes可严格恢复，accessor防御复制 |
| narrow ports | `internal/lottery/application/repository.go` | create + exact find，无 latest/list/update/upsert/delete |
| create-only MySQL adapter | `internal/lottery/adapter/mysqlrepo/strategy_snapshot_repository.go` | root+Awards原子创建、exact RR读取、duplicate与commit unknown分离 |
| schema | Migration `000006`、`000007` | PK/FK/CHECK/RESTRICT、无 mutation surface |

### 2.2 Marketing domain

| 面 | 当前代码位置 | 冻结前要证明 |
| --- | --- | --- |
| compact root | `internal/marketing/domain/activity.go` | draft/published/retired三种且仅三种合法状态形状 |
| immutable publication | `publication.go` | schema v1、release/rollback union、时间窗、exact refs、evidence |
| bounded local refs | `lottery_reference.go` | 不 import Lottery；graph/Strategy ref exact且bounded |
| pure transitions | `transition.go` | publish/rollback/retire返回完整CAS plan，失败为zero plan |
| confirmed gate | `gate.go` | not_published/scheduled/active/ended/retired与技术error分离 |
| value grammar | `value.go` | name/evidence/revision/time canonicalization与边界一致 |
| defensive copy/fuzz | domain tests | manifest、publication/transition accessor不能被外部slice改写 |

### 2.3 Marketing application

| Use case | 必须的成功依赖顺序 | 关键负向证据 |
| --- | --- | --- |
| create draft | validate -> create-only repository | duplicate、bad input、cancel返回zero |
| publish | current -> Clock once -> provisional plan -> Lottery verify -> approval -> rebuild -> CAS | stale expected state在Clock前失败；任一verifier失败不写入 |
| rollback | current -> exact history -> Clock once -> plan -> Lottery verify -> new approval -> rebuild -> CAS | 不接受caller graph/window/manifest；ended/非旧/异Activity source失败 |
| retire | current -> Clock once -> retirement candidate -> approval -> root CAS | 不追加publication，不删除history，draft/retired重复操作失败 |
| resolve | one RR current snapshot -> exact Lottery reverify -> Clock once -> gate | draft不读Lottery；坏ref不伪装confirmed reject；所有error zero decision |

Commit acknowledgement 恢复面还必须证明：

- publish/rollback/retire 的 repository commit unknown 仍返回 zero domain result + error；
- caller或operation context已取消/超时时不附带receipt；ordinary retryable/conflict/storage error也没有receipt；
- 仅 `ErrCommitOutcomeUnknown` 可经 `ActivityCommitReceiptFromError` 显式取得合法防御复制；receipt 不进入
  `Error()`、`errors.Is` 或普通error chain；
- publish/rollback receipt包含exact before/after root与完整publication；retire receipt包含exact before/after
  root及本次retiredAt/evidence；
- rollback receipt额外验证`rollbackOf < before.active`，不能拿领域上合法但与本attempt不匹配的record对账；
- publish/rollback exact RR read-back使用`ObserveCurrentActivity`，retire exact root read-back使用
  `ObserveActivityRoot`；
- `ReconcileActivityCommit`纯函数只产生`committed/not_committed/indeterminate`，坏输入、partial读取、
  identity/name不符或更晚generation一律indeterminate；
- receipt/observation accessor不能被外部manifest slice修改，reconciliation不做I/O、不读取Clock/approval、
  不推荐盲重试。

### 2.4 Cross-context ACL

`internal/marketing/adapter/lotteryconfig/verifier.go` 必须证明：

- Marketing domain/application不 import Lottery；
- 只有 adapter 将 Marketing primitive refs 翻译为 Lottery identities；
- exact graph reader只按 `(GraphID, GraphRevision)` 查询；
- reader value identity 必须与请求完全相等并重新 `Validate()`；
- graph 所有 unique terminal StrategyID 集与 manifest StrategyID 集完全相等；
- missing、extra、duplicate、zero 或错 identity 全部 fail closed；
- 每个 manifest entry exact读取 `(StrategyID, StrategyRevision)` snapshot；
- 不查 latest，不 fallback previous，不返回 partial terminal/snapshot；
- caller cancel 优先于 dependency classification；
- provider not-found/invalid 与 unavailable class 不混淆。

### 2.5 Marketing MySQL adapter

`internal/marketing/adapter/mysqlrepo/repository.go` 必须证明：

- draft create-only；
- root/history/current都按exact identity读取；
- current root、active publication、manifest在同一read-only RR snapshot中；
- draft current返回zero publication；
- published current缺active row/manifest或strict restore失败时fail closed；
- publication transaction先append header/bindings，再以expected lifecycle/state/active CAS root；
- CAS `RowsAffected != 1`是state conflict，整笔事务回滚；
- rollback是新行，不UPDATE旧publication；
- retire只CAS root，保留active pointer与历史；
- caller cancel、retryable transaction error、commit outcome unknown、storage failure分类可区分；
- 普通error rendering不泄露SQL、表名、credential或driver cause。

## 3. 领域不变量验收

### 3.1 Activity root 状态形状

必须逐项覆盖：

```text
draft:
  stateVersion = 0
  activePublicationVersion = 0/none
  retiredAt = zero
  retirementReference = empty

published:
  stateVersion = activePublicationVersion > 0
  retiredAt = zero
  retirementReference = empty

retired:
  activePublicationVersion > 0
  stateVersion = activePublicationVersion + 1
  retiredAt = canonical UTC microsecond
  retirementReference = valid bounded evidence ref
```

负向 fixture 至少包括：zero ActivityID；empty/trim-mismatched/oversized name；unknown lifecycle；draft带非零generation或active；published state/active不等；retired缺active/retiredAt/reference；retired state不是active+1；版本溢出；非canonical持久时间。

### 3.2 Publication identity 与 union

一份合法 record 必须同时满足：

- ActivityID 与 numeric version 均大于零；
- `ActivityPublicationSchemaVersionV1 == 1`；
- release没有`rollbackOf`，rollback满足`0 < rollbackOf < version`；
- graph ref exact且bounded；
- manifest非空、canonical、unique且不超过128；
- startsAt、endsAt、publishedAt均为UTC微秒；
- `startsAt < endsAt`且`publishedAt < endsAt`；
- approval evidence reference合法。

应伪造每一个单字段和混合状态，确认strict restore失败而不是“尽量恢复”。

### 3.3 Publish transition

需要证明：

- draft active 0 -> release v1；published active n -> release v(n+1)；
- next Activity为published，stateVersion与active version同时推进；
- expected lifecycle/state/active保留旧root的CAS谓词；
- retired不能publish；`publishedAt >= endsAt`不能形成candidate；
- max version不能wrap到zero；失败返回完整zero transition。

### 3.4 Rollback transition

需要证明：

- current必须published；target必须same Activity、strict valid且版本严格更旧；
- application必须通过exact history建立“曾发布”事实；
- 本次`publishedAt < target.endsAt`；
- 新版本为active+1，kind为rollback，`rollbackOf=target.version`；
- graph、manifest、startsAt、endsAt逐字段复制target；
- publishedAt与approval evidence来自本次动作；
- target等于active、未来版本、异Activity、ended或unproven全部失败；
- 回滚不修改target、不复用target version。

### 3.5 Retire transition

需要证明：

- 仅published可retire；stateVersion推进一位；active version保持不变；
- retiredAt与retirementReference必填且canonical；
- `AppendsPublication()`为false，`Record()`不存在；
- retired为terminal；max version不能溢出；失败没有partial next state。

### 3.6 Gate truth table

| 输入 | status | allow |
| --- | --- | ---: |
| draft + zero publication | `not_published` | false |
| published, `start - 1µs` | `scheduled` | false |
| published, `start` | `active` | true |
| published, `end - 1µs` | `active` | true |
| published, `end` | `ended` | false |
| retired + exact last publication | `retired` | false |
| retired + zero publication | `retired` | false |

Invalid root、zero clock、published+zero publication、draft+publication、identity mismatch、malformed record、依赖失败或context取消必须返回zero decision + error，不能伪装为confirmed reject。

## 4. 并发与事务验收

### 4.1 同 generation 双发布

```text
root: published, state=3, active=3
A plans v4 with expected state=3/active=3
B plans v4 with expected state=3/active=3
```

判定标准：两个纯plan都可形成，但authoritative MySQL CAS最多一个影响一行；loser得到stable state conflict，其publication/bindings随transaction rollback；不能出现active v4却缺record/manifest；Redis lock不是正确性前提。

### 4.2 Publish atomicity

逐点注入publication insert、binding insert、root CAS与COMMIT失败。COMMIT前失败必须全量rollback。COMMIT transport failure必须分类为commit outcome unknown；在exact read-back之前不能宣称成功、失败或安全重试。

### 4.3 RR current snapshot

需要验证：

```text
BEGIN READ ONLY REPEATABLE READ
  SELECT exact Activity root
  SELECT exact (activity_id, active_version) publication
  SELECT exact publication manifest
COMMIT read transaction
```

不得使用`MAX(version)`、latest、missing fallback或跨snapshot拼接root/header/bindings。

## 5. MySQL schema 与最小权限验收

### 5.1 DDL shape

真实MySQL 8.4隔离schema中必须验证：

- Migration 000006～000011按序应用；
- 五张新增表的PK、owned FK、CHECK与RESTRICT；
- 000011 active reverse FK只能指向same Activity publication；
- rollback FK只能指向same Activity publication；
- Marketing表没有到Lottery graph/snapshot表的跨上下文FK；
- invalid state/schema/kind/window/ref被约束拒绝；
- strict restore覆盖CHECK无法表达的跨行闭合集与canonical规则。

### 5.2 Grant boundary

隔离writer身份必须证明：

- snapshot writer仅有必要SELECT/INSERT；
- Marketing writer仅有必要SELECT/INSERT及Activity header受控UPDATE；
- immutable snapshot/publication/binding的UPDATE/DELETE被拒；
- 两个writer不能跨bounded-context写入；
- 长期`growthos_app` grant不变，因为服务未runtime装配。

DDL、checksum和SQL mock属于IMPLEMENTED-SURFACE。完整最终候选已经取得真实MySQL、isolated grants与坏引用
fixture的EXECUTED-CANDIDATE-EVIDENCE；真实COMMIT transport acknowledgement丢失仍不能由DBA成功或脚本
名称替代，其可执行恢复契约由application commit receipt三态fixture证明。

## 6. Error 与 context 验收

### 6.1 优先级

```text
caller cancellation/deadline
  > internal private deadline attribution
  > reviewed dependency/domain/repository class
```

覆盖caller预取消、dependency期间取消、value+error、caller/internal deadline先后、provider deadline、cleanup cancel、zero Clock及final success前checkpoint。

### 6.2 零值协议

所有失败必须返回zero Activity/publication/gate/snapshot/transition，不得泄露partial manifest、active pointer、
target或status。`ErrCommitOutcomeUnknown` 也不例外：receipt只能从error显式提取，用于后续对账，绝不能冒充
成功业务返回值。

### 6.3 低披露

本节新增的 Marketing operation/dependency/repository wrappers 的普通 `Error()` 与 `errors.Is` 不得泄露 SQL、表/index、DSN、credential、exact identities、Award/Weight、approval payload或private deadline cause。可信诊断代码可显式读取 `Cause()`，但不得原样暴露给未来 HTTP caller。该断言不覆盖本节未改写的既有 Lottery repository wrapper。

## 7. 架构停止线验收

- Marketing domain/application不import Lottery；
- Lottery domain/application不import Marketing；
- cross-context translation只在Marketing-owned ACL adapter；
- Activity service未进入`cmd/growth-api`、HTTP、Compose或Web；
- 未新增runtime env/grant、Redis/RabbitMQ/PostgreSQL；
- 未调用evaluator、selector、random source或正式Draw；
- 未实现真实Governance、authentication、RBAC、tenant或audit actor。

```bash
rg -n "PublishActivity|RollbackActivity|RetireActivity|ResolveActivity|ActivityPublication" \
  cmd internal/infrastructure/httpapi web/src deploy

rg -n 'github.com/Atingaii/GrowthOS-Go/internal/lottery' \
  internal/marketing/domain internal/marketing/application \
  --glob '*.go' --glob '!**/*_test.go'

rg -n 'github.com/Atingaii/GrowthOS-Go/internal/marketing' \
  internal/lottery/domain internal/lottery/application \
  --glob '*.go' --glob '!**/*_test.go'
```

在第30节边界内，第一条不应出现新runtime/public production装配；后两条不应出现 domain/application 跨上下文 import。

## 8. 本节明确不验收

- Activity HTTP/MCP/Agent API；
- 真实approval provider、双人复核或Governance runtime；
- session、Principal、authentication、authorization、RBAC或data scope；
- 运营后台、导航、route、button或浏览器E2E；
- Participation资格、会员authority编排；
- 第29节graph evaluator的runtime接线；
- Strategy selection、random ticket、正式Draw/Result；
- 库存、Benefit、积分、优惠券或发奖；
- 消息事件、RabbitMQ、outbox、CDC或异步补偿；
- 自动timer发布、暂停/恢复、灰度、多active、A/B、canary；
- Redis cache或read projection；
- 多租户、跨区域复制、灾备演练；
- 生产QPS、P95/P99、容量、SLO、渗透、合规或零故障结论。

## 9. 验收门禁与最终证据

### 9.1 定向普通测试

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
```

判定：exit 0。输出不得被本文预写成固定package/test数量。

**最终候选证据：** 在 Go `go1.26.6`、`GOMOD` 指向当前模块、`GOWORK` 为空的环境中，定向普通测试
exit 0；commit receipt 的 publish/rollback/retire zero-result、显式提取、防御复制、低披露及三态
reconciliation fixtures均随application package执行通过。

### 9.2 Race

```bash
go test -race -count=1 ./internal/marketing/...
go test -race -count=1 \
  ./internal/lottery/domain \
  ./internal/lottery/adapter/mysqlrepo
```

判定：exit 0且无race report。它只证明被执行测试路径，不证明未来任意adapter或生产负载。

**最终候选证据：** Marketing与Lottery上述定向race均exit 0且无race report；此外全仓
`go test -race -count=1 ./...` 也exit 0。

### 9.3 Shuffle replay

```bash
for run in $(seq 1 20); do
  go test -count=1 -shuffle=on ./internal/marketing/... || exit 1
done
```

**最终候选证据：** 固定seed `1788183001`～`1788183020` 共20轮全部exit 0；保存连续范围是为了可复放，
不表示只需这些seed即可穷尽顺序依赖。

### 9.4 Fuzz

当前Marketing domain包含边界fuzz target。冻结时先列出target，再逐个执行：

```bash
go test ./internal/marketing/domain -list '^Fuzz'

go test ./internal/marketing/domain -run '^$' \
  -fuzz '^FuzzDecideActivityGatePreservesHalfOpenMicrosecondBoundary$' \
  -fuzztime=10s

go test ./internal/marketing/domain -run '^$' \
  -fuzz '^FuzzPlanPublishNeverAcceptsAmbiguousOrNonCanonicalManifest$' \
  -fuzztime=10s

go test ./internal/marketing/domain -run '^$' \
  -fuzz '^FuzzPlanRollbackNeverReusesTargetOrWrapsVersion$' \
  -fuzztime=10s
```

Fuzz exec数随机器、cache和语料变化，不作为课程KPI。

**最终候选证据：** 三个target各运行10秒并通过，按上方顺序exec分别为`2,019,397`、`1,087,335`、
`1,469,462`；new interesting均为0，total corpus分别为`11`、`44`、`10`。这些数字只记录本次机器、
cache与语料状态，不是质量或性能KPI，也不能替代seed corpus和边界单测审查。

### 9.5 Coverage

```bash
go test -count=1 -cover \
  ./internal/lottery/domain \
  ./internal/lottery/application \
  ./internal/lottery/adapter/mysqlrepo \
  ./internal/marketing/domain \
  ./internal/marketing/application \
  ./internal/marketing/adapter/lotteryconfig \
  ./internal/marketing/adapter/mysqlrepo
```

必须记录最终候选的实际package coverage，不沿用早期候选数字，也不把statement coverage解释成分支、
并发、真实数据库或业务场景覆盖。

**最终候选证据：** Lottery domain `93.5%`、application `88.3%`、MySQL adapter `81.8%`；Marketing
domain `96.0%`、application `77.1%`、Lottery ACL `81.9%`、MySQL adapter `84.8%`。这些是statement
coverage，不是分支覆盖、生产风险评分或SLO。

### 9.6 全仓回归

```bash
make fmt-check
go vet ./...
go test ./...
go test -race ./...
go run ./cmd/doccheck
```

Web未被本节修改，完整最终候选仍通过以下既有门禁（由`make verify`调用）：

```bash
make web-verify
```

这些命令分别证明格式、静态分析、普通测试、race、文档完整性和现有Web回归；不能合并成“生产可用”。

**最终候选证据：** `make verify` exit 0，包含Go vet/test/doccheck、Web 19 files / 152 tests、typecheck与
Vite production build（2462 modules）；全仓`go test -race -count=1 ./...`另行exit 0。第30节仍未装配
Activity runtime/API/UI，因此这些结果不构成上线、容量或浏览器业务验收。

### 9.7 真实 MySQL

必须使用专用、可丢弃的MySQL 8.4 schema与隔离writer身份。具体环境、授权token、执行脚本和清理步骤应在最终验收记录中写明；不得把用户日常数据库或长期`growthos_app`当测试writer。

当前实现提供显式确认保护的独立门禁：

```bash
GROWTHOS_LESSON30_MYSQL_ACCEPTANCE=run-disposable-mysql-8.4.11 \
  make lesson30-mysql-acceptance
```

它使用随机命名、tmpfs MySQL 8.4.11、独立migrator/snapshot/Marketing identities，并运行schema、exact snapshot与Activity repository integration fixtures，最后核对清理与现有GrowthOS Docker资源未被改变。

此前候选上，该命令已连续两次完整 exit 0，形成以下EXECUTED-CANDIDATE-EVIDENCE：

- true embedded v5 baseline与旧五表数据/结构fingerprint；v5→v11、repeat no-change与dirty fail-closed；
- 五张新表的exact columns/PK、6个RESTRICT FK、20个enforced CHECK、binary collation，且无
  Marketing→Lottery FK；
- snapshot writer与Marketing writer exact allowlist；immutable mutation、跨上下文、schema及长期
  `growthos_app` 对八张未装配表的访问均得到MySQL 1142；
- snapshot create/exact RR/1000 Awards/duplicate/concurrent one winner/child failure rollback；
- Activity draft→publish→replace→rollback→retire、exact history、RR cross-replace、CAS/concurrency与
  half-write rollback；DB可物理保存的坏cross-context refs仍被真实ACL fail closed拒绝；
- 最终schema `11:0`、临时CHECK probe归零；随机container/volume/network/identity/secret零残留，长驻
  Docker资源identity未改变。

同一此前候选还完成：长驻Compose原MySQL container/volume/network identity不变的v5→v11升级，旧五表
行数/checksum与长期grant不变，`make compose-status`/`make compose-smoke` exit 0；独立Lottery Compose
acceptance exit 0并清理全部随机资源；`make verify` exit 0，覆盖Go vet/test/doccheck与Web 152 tests、
typecheck、生产build。

Commit receipt/reconciliation收口后的完整最终候选又执行一次同一disposable MySQL 8.4.11门禁并exit 0：
最终schema为`11:0`、临时probe为`0`、脚本cleanup计数为`0`，长驻GrowthOS资源identity保持不变。
随后`make compose-status`报告clean 11/latest 11，`make compose-smoke` exit 0，长期container/volume/network
identity仍保持。由此real MySQL gate不再是REQUIRED-GATE。

该脚本没有把“真实COMMIT transport acknowledgement丢失”伪装成已注入的网络事实；它证明的是repository/
schema/事务/权限与清理面。Commit receipt的committed/not_committed/indeterminate语义由最终application
normal/race/shuffle fixtures另行证明。独立Lottery Compose acceptance来自此前已验收且本次未受
application-only receipt变更影响的runtime切片；不能据此宣称Activity已装配。

## 10. 故障注入清单

| 注入点 | 期望结果 | 禁止结果 |
| --- | --- | --- |
| graph exact not-found | Lottery publication invalid + zero result | latest/fallback |
| graph provider unavailable | unavailable + zero result | confirmed ended |
| manifest missing/extra | invalid，Strategy read按早期校验停止 | partial success |
| Strategy snapshot wrong identity | invalid | 使用返回value |
| approval reject | rejected，CAS零调用 | 写入无evidence publication |
| approval nil error + bad ref | evidence invalid | 接受caller/default evidence |
| stale expected state | conflict，按阶段停止 | last-write-wins |
| history exact missing | publication not-found | 自动选previous/latest |
| rollback target ended | rollback target invalid | 偷偷延长window |
| binding insert failure | whole transaction rollback | orphan publication |
| root CAS zero rows | conflict + rollback | active/history分裂 |
| COMMIT transport failure | commit outcome unknown | 盲目重试/宣称失败 |
| persisted malformed root | stored invalid | best-effort恢复 |
| resolve exact verifier failure | zero decision + technical error | 业务reject |
| Clock zero | clock invalid | 使用`time.Now()`替代 |
| caller cancel | caller context error | provider/storage class覆盖 |

## 11. Commit outcome unknown 验收协议

发生不确定COMMIT时，测试和运行手册都必须要求：

1. 停止自动重试同一高风险command；
2. 断言业务返回值仍为zero publication/zero Activity，并且error class为`ErrCommitOutcomeUnknown`；
3. 仅通过`ActivityCommitReceiptFromError`显式提取可信防御复制；普通retryable/context/storage error提取失败；
4. publish/rollback用新健康连接调用exact current reader，在同一RR snapshot取得root+active publication；
5. retire用新健康连接exact读取root；
6. 分别构造`ObserveCurrentActivity`或`ObserveActivityRoot`；
7. 调用`ReconcileActivityCommit(receipt, observation)`；
8. exact after root（以及publish/rollback的完整publication）相等才是`committed`；exact before或同generation其他winner才是
   `not_committed`；missing/invalid/mismatched/partial/advanced全部`indeterminate`；
9. 不使用latest、时间戳猜测、重新读取Clock/approval或重新生成candidate；
10. reconciliation结果本身不授权retry；not_committed也需上层另行决定是否发起一项新操作。

实际read-back流程见[运行手册](../../runbooks/activity-publication.md)。

## 12. 冻结检查单

- [x] 六份第30节章节文档已按最终 commit receipt/reconciliation API 同步；
- [x] 定向普通测试在最终候选exit 0；
- [x] Marketing/Lottery定向race与全仓race在最终候选exit 0；
- [x] 固定seed `1788183001`～`1788183020` 全部exit 0并留存；
- [x] 三个fuzz target各10秒实际执行并记录exec/corpus（非KPI）；
- [x] final package coverage实际执行并记录；
- [x] real MySQL 8.4 DDL/FK/CHECK/RESTRICT已在此前候选连续两次、最终候选再次执行；
- [x] isolated grant与immutable mutation rejection已实际执行；
- [x] commit receipt committed/not_committed/indeterminate fixtures已随最终application测试执行；
- [x] 长驻v5→v11升级及最终Compose status/smoke、资源identity/旧数据/grant不变已实际核对；
- [x] 独立Lottery Compose acceptance已执行并核对随机资源清理；
- [x] `make verify`已在完整最终候选exit 0；
- [x] root在全部索引收口后再次执行`go run ./cmd/doccheck`；
- [x] root最终diff/architecture stopline检查；
- [x] root精确移除本次Vite生成的ignored `web/dist/`，保留可复用依赖、Compose Secrets与长期Docker资源，并确认无其他意外产物；
- [x] accepted candidate `887754bb192d9850d231729391152cd8678c11ad`、远端同名ref、main不变与线性历史由root实际核查。

Freeze-attestation 提交只登记上述已执行核查；它不改变accepted candidate的代码、schema或运行时能力。

## 13. QA 结论边界

完整最终候选已经取得normal/race/shuffle/fuzz/coverage、真实MySQL、长驻Compose、独立Lottery Compose与
`make verify`证据。尚未完成的是root在全部文档/索引收口后的最终architecture/diff/cleanup/ref冻结，而
不是代码门禁待复跑。准确的阶段性说法是：

> 第30节的Strategy snapshot、Activity domain/application、Lottery ACL和Marketing MySQL adapter已形成
> IMPLEMENTED-SURFACE；完整候选的Go/MySQL/Compose/全仓回归已有EXECUTED-CANDIDATE-EVIDENCE；公开API、
> 认证授权、真实审批、runtime/UI/Draw等保持OUT-OF-SCOPE；accepted candidate、远端学习分支、线性历史与
> root最终architecture/diff/cleanup均已实际核查。
