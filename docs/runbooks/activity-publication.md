# Activity publication 验收与故障分诊手册

- **适用课程：** [第 30 节：为什么 Strategy 不等于 Activity](../course/part-04/lesson-30-strategy-vs-activity.md)
- **产品基线：** [Activity 发布与 Lottery 配置精确绑定基线 v1](../product/activity-publication-binding-v1.md)
- **架构决策：** [ADR-0026](../decisions/ADR-0026-activity-publication-binding.md)
- **QA 矩阵：** [第 30 节 QA](../qa/lessons/lesson-30.md)
- **API 状态：** [第 30 节零公开 API](../api/lessons/lesson-30.md)
- **更新日期：** 2026-08-31
- **当前运行状态：** 内部domain/application/adapter切片，未装配到`cmd/growth-api`、HTTP、Compose长期runtime或Web

> 这不是生产运营平台操作手册。当前没有可调用的Activity endpoint、CLI、真实approval adapter、authentication/RBAC或长期runtime grant。本手册用于开发验收、隔离MySQL故障注入和未来装配前的处置基线；禁止把伪造evidence或直接改表当成“临时发布方案”。

## 1. 安全原则

1. **先确认环境。** 只在专用、可丢弃的验收schema执行写入与grant测试，不连接个人日常库或生产库。
2. **先读后判。** 状态冲突和commit unknown先exact read-back，不盲重试。
3. **不改历史。** Strategy snapshot、publication和bindings只追加/读取，不UPDATE或DELETE。
4. **不越过服务边界。** 不用migrator/DBA直接构造active publication；高权限写入会绕过Lottery closure、approval和CAS。
5. **不猜latest。** 所有调查都使用exact ActivityID/version、graph ID/revision与Strategy ID/revision。
6. **不把技术故障当业务拒绝。** Storage/provider/corruption返回error；只有可信输入才能形成confirmed gate。
7. **不泄露cause。** 普通日志/工单只记录reviewed class和受控correlation，不复制DSN、SQL、Award、approval payload或credential。
8. **不虚构运行能力。** 当前没有“上线”“停止生产流量”“一键回滚”入口；任何装配必须进入后续章节。

## 2. 当前组件地图

| 责任 | 位置 | 当前能力 |
| --- | --- | --- |
| Activity/publication/gate | `internal/marketing/domain` | pure construct/restore/plan/decide |
| use cases | `internal/marketing/application` | create/publish/rollback/retire/resolve，均未装配 |
| Lottery closure verifier | `internal/marketing/adapter/lotteryconfig` | exact graph + terminal Strategy snapshots |
| Marketing repository | `internal/marketing/adapter/mysqlrepo` | exact read、RR current、publication/retire CAS |
| Strategy snapshot | `internal/lottery/domain` + MySQL adapter | create-only、exact find |
| schema | `migrations/sql/000006`～`000011` | forward-only新增表/FK |

不存在的组件：Activity handler/router/DTO、approval implementation、session/RBAC、event/outbox、scheduler、Redis projection、UI和正式Draw。

## 3. 状态速查

### 3.1 Root shape

```text
draft:     state=0, active=none, retirement=none
published: state=active>0, retirement=none
retired:   state=active+1, retirement=(UTCµs, evidence)
```

任何其他组合都按stored invalid处理，不尝试修补后继续。

### 3.2 Gate matrix

| Root/publication/instant | confirmed status | allow |
| --- | --- | ---: |
| draft + zero publication | `not_published` | false |
| published，now < start | `scheduled` | false |
| published，start <= now < end | `active` | true |
| published，now >= end | `ended` | false |
| retired | `retired` | false |

Root/record mismatch、exact Lottery verify失败、Clock zero或dependency failure不是表中状态，必须返回zero decision + technical error。

## 4. 开始验收前的 preflight

### 4.1 确认工作树与代码边界

```bash
git status --short
git diff --check

rg -n "PublishActivity|RollbackActivity|RetireActivity|ResolveActivity|ActivityPublication" \
  cmd internal/infrastructure/httpapi web/src deploy
```

第30节不应在runtime/public production surface出现Activity装配。如果命中，先核对是否来自明确后续章节，不要继续以第30节标准验收。

### 4.2 确认Go环境

```bash
go version
go env GOMOD
go env GOWORK
```

`GOMOD`必须指向当前GrowthOS-Go模块。不要在不明`GOWORK`覆盖下记录accepted evidence。

### 4.3 确认数据库环境

在执行任何迁移/写测试前记录但不要在日志打印secret：

- MySQL server version应为验收要求的8.4系列；
- schema名称必须是专用、可丢弃的验收schema；
- migrator与isolated writer是不同identity；
- writer grant不包含全局权限、DBA或`ALL PRIVILEGES`；
- 长期`growthos_app` grant不因第30节改变。

若任何一项不确定，停止写测试，只做静态/unit检查。

### 4.4 使用受保护的隔离入口

仓库当前提供一次性门禁：

```bash
GROWTHOS_LESSON30_MYSQL_ACCEPTANCE=run-disposable-mysql-8.4.11 \
  make lesson30-mysql-acceptance
```

确认口令只授权脚本创建随机命名、带Lesson 30 label的tmpfs MySQL 8.4.11容器；它不授权操作现有Compose volume/container/network。运行前仍应阅读脚本diff，运行后核对其清理检查。没有确认口令时脚本应fail closed。

## 5. 静态与单元验收

### 5.1 定向普通测试

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

记录实际exit code和shuffle seed。不要把本手册中的命令列表当成通过证据。

### 5.2 Race 与 fuzz

```bash
go test -race -count=1 ./internal/marketing/...
go test ./internal/marketing/domain -list '^Fuzz'
```

逐个执行列出的fuzz target并记录实际时间/结果。Fuzz exec数不稳定，不设固定通过数字。

### 5.3 全仓门禁

```bash
make fmt-check
go vet ./...
go test ./...
go run ./cmd/doccheck
make web-verify
```

这些命令覆盖范围不同。任何一个失败都先定位本节还是并行工作树变化，不得删除用户/其他章节改动来“清干净”。

## 6. Migration 验收

### 6.1 顺序

```text
000006 lottery Strategy snapshot header
000007 snapshot Awards
000008 Marketing Activity root
000009 immutable Activity publication
000010 publication Strategy bindings
000011 Activity active publication reverse FK
```

旧Migration不可回写；任何修正都必须新增forward Migration并单独ADR。

### 6.2 必查schema事实

在专用MySQL 8.4实例读取`information_schema`或`SHOW CREATE TABLE`，逐项核对：

- 五张新增表存在；
- exact复合PK与owned FK存在；
- publication schema version、kind union和window CHECK存在；
- Activity lifecycle shape CHECK存在；
- publication/binding/snapshot历史引用使用RESTRICT；
- 000011 active reverse FK包含`(activity_id, active_version)`；
- Marketing表没有引用Lottery表的FK；
- immutable表writer没有UPDATE/DELETE grant。

不要只看Migration runner显示latest=11；版本号到达不能证明约束语义和grant正确。

### 6.3 坏数据fixture

使用isolated identity逐项尝试并期待拒绝：

- draft带active version；
- published state与active不等；
- rollback无source或source不小于version；
- start不小于end；
- publishedAt不早于end；
- zero/invalid exact identity；
- binding指向不存在publication；
- active pointer指向另一Activity或不存在version；
- immutable snapshot/publication/binding UPDATE/DELETE。

DB能接受但domain/ACL应拒绝的fixture也要验证，例如manifest missing/extra或跨上下文exact ref不存在。这正是CHECK/FK边界，不应通过临时跨context FK“修复”。

## 7. 正常发布核查

当前没有runtime命令；以下是service-level验收顺序，不是供运营直接执行的API：

```text
validate command
  -> read exact current root
  -> expected state check
  -> Clock once
  -> provisional PlanPublish
  -> exact Lottery closure verification
  -> approval verifier
  -> rebuild/prove unchanged
  -> publication CAS transaction
```

核查点：

1. Caller不提供next version、publishedAt或approval evidence；
2. stale expected state在Clock/provider前失败；
3. Lottery或approval失败时repository写调用为零；
4. 成功record version为active+1；
5. manifest canonical且与graph terminal set完全相等；
6. transaction中record/bindings/header同时提交；
7. 返回record与exact read-back逐字段一致。

若任何candidate字段在approval后发生变化，视为operation failure，禁止CAS。

## 8. CAS conflict 分诊

### 8.1 症状

Stable class：`marketing repository: activity state conflict`。

可能原因：

- 另一个publisher先提交；
- Activity已retired；
- caller使用旧state generation；
- 当前active和预期active不一致；
- 极端情况下SQL predicate或stored state损坏。

### 8.2 处置

1. 不把同一transition原样循环重试；
2. exact读取Activity root；
3. 若root strict restore失败，转stored-invalid流程；
4. 对比lifecycle/state/active与caller预期；
5. 若只是正常赢家推进，让上层展示“状态已变化”，重新形成candidate和审批；
6. 若已经retired，停止publish/rollback；
7. 确认loser transaction没有orphan publication/binding；
8. 记录受控correlation，不记录raw SQL/approval payload。

CAS conflict不是storage outage，也不是自动幂等成功。

## 9. Commit outcome unknown 分诊

### 9.1 立即动作

遇到`marketing repository: commit outcome is unknown`：

1. **停止自动重试；**
2. 确认service业务返回值仍是zero publication/zero Activity；candidate不是成功结果；
3. 仅通过`ActivityCommitReceiptFromError(err)`显式提取可信receipt；若`ok == false`，停止自动恢复并按
   ordinary/context/invalid error分诊；
4. 保留原command、受控correlation与receipt operation/before/after generation；不要把完整manifest、
   evidence或底层cause写入普通日志；
5. 不重新读Clock、不重新调用approval来“试一次”；
6. 使用新的健康连接执行只读exact read-back；
7. 在三态对账前不向调用方宣称成功或失败。

Receipt只存在于caller与operation context均仍存活且stable class精确为`ErrCommitOutcomeUnknown`的
`ActivityOperationError`；它不会通过普通`Error()`、`errors.Is`或unwrap chain暴露。Ordinary retryable、
conflict、storage/provider error、caller cancel/private deadline和非法receipt都不能进入本流程。

### 9.2 Exact read-back

使用parameterized repository/query，不拼接外部字符串。Publish/rollback必须调用exact current reader，在
**同一个read-only RR snapshot**中读取：

```text
Activity root by exact ActivityID
root.active指向的 exact publication
该 publication 的完整 canonical bindings
```

然后构造：

```go
observation := application.ObserveCurrentActivity(root, publication)
```

Retire只需在新健康连接上exact读取Activity root，再构造：

```go
observation := application.ObserveActivityRoot(root)
```

Receipt已经保存exact before/after root；publish/rollback还保存完整candidate publication（rollback receipt
额外验证`rollbackOf < before.active`），retire after root已经包含server-owned retiredAt与retirement
evidence。调用纯函数：

```go
result := application.ReconcileActivityCommit(receipt, observation)
```

函数会逐字段比较：

- root lifecycle/state/active；
- publication schema/kind/rollbackOf；
- startsAt/endsAt/publishedAt；
- exact graph ref；
- canonical complete manifest；
- approval evidence reference。

### 9.3 三态结论

| Read-back | 结论 | 后续 |
| --- | --- | --- |
| exact after root命中；publish/rollback的完整publication也逐字段等于receipt | `committed` | 登记既有提交事实，不追加新version |
| exact before root仍在，或同一next generation出现另一合法winner | `not_committed` | 只可由上层策略决定是否发起一项新的完整操作 |
| missing/invalid/mismatched/partial observation，或root已推进到更晚generation | `indeterminate` | 停止该Activity写入，升级人工与DB调查 |

更晚generation必须是indeterminate：本次写可能先提交，之后才被另一操作推进，当前root不能证明中间历史。
`ReconcileActivityCommit`不做I/O、不读取Clock/approval，也不建议retry。禁止使用`MAX(version)`、
created_at最近、latest或“看起来像”来判断；即使结果是not_committed，也不能绕过上层的授权、幂等和重新
审批策略盲重放原command。

## 10. Rollback 核查

Rollback command只包含ActivityID、expected state和target version。正确流程：

1. exact读取current root；
2. exact读取same Activity target publication；
3. 验证target严格更旧且来自append-only history；
4. Clock once，要求本次publishedAt早于target end；
5. 从target复制graph/manifest/window形成candidate；
6. 重新Lottery closure验证；
7. 获得本次新approval evidence；
8. append next rollback publication并CAS root。

拒绝以下“快捷操作”：

- 直接把active改回target version；
- UPDATE target的endsAt；
- caller提交一份“类似target”的新manifest；
- ended target偷偷延期；
- 跳过新approval；
- 假称已经撤销历史Draw/Benefit副作用。

## 11. Retire 核查

Retire是terminal root CAS：

```text
published state=n, active=n
  -> retired state=n+1, active仍为n
  + retiredAt + retirementReference
```

核查：

- 只允许published进入；
- Clock一次；
- 使用独立retirement approval evidence；
- 不append publication；
- 不删除或修改last active/history；
- resolve形成confirmed retired；
- 后续publish/rollback/retire均state/lifecycle conflict。

当前没有全局kill switch。已经resolve旧snapshot的进行中请求是否继续，属于未来正式Draw orchestration；不要把retire宣传为同步撤销所有流量和副作用。

## 12. Resolve/gate 故障分诊

| Stable class/现象 | 优先检查 | 处置 |
| --- | --- | --- |
| Activity not found | exact ID、reader | 资源不存在；未来对外披露策略未定 |
| stored Activity invalid | root shape、schema/Migration旁路 | 停止该Activity解析与写入 |
| stored publication invalid | active record、manifest、UTCµs | 停止解析，不fallback |
| Lottery publication invalid | exact graph/snapshot、closure | 修复authority/重新release，不能查latest |
| Lottery unavailable | provider/DB/context | 保持technical failure，恢复依赖后重试resolve |
| Clock invalid | injected Clock/config | 不用临时`time.Now()`掩盖 |
| operation timeout | internal budget与dependency latency | caller仍live时调查stage；不是ended |
| context canceled/deadline | caller ownership | 保留caller error，不覆盖成provider class |
| confirmed scheduled/ended/retired | window/root事实 | 正常业务拒绝，不触发storage告警 |

任何technical failure都不得返回partial publication或gate decision。

## 13. Stored-invalid 处置

### 13.1 首要原则

- Fail closed：停止该Activity的publish/rollback/resolve；
- 保护证据：只读导出受控行与Migration/grant元数据；
- 不在线UPDATE“修好”；
- 不删除坏历史；
- 不使用旧publication/latest fallback恢复流量。

### 13.2 调查顺序

1. 检查应用是否使用预期schema/Migration；
2. 检查是否有DBA/migrator/脚本旁路；
3. strict restore定位root、header还是binding shape；
4. 校验exact Lottery authority是否存在且有效；
5. 检查transaction/commit unknown记录；
6. 形成独立incident与forward repair方案；
7. 修复必须通过新的Migration或经过批准的新release，不改写immutable history。

## 14. Approval 故障分诊

| 结果 | 含义 | 可否重试 |
| --- | --- | --- |
| rejected | exact candidate被明确拒绝 | 不能原样自动重试绕过；修改candidate需新流程 |
| unavailable | 未能建立approval结果 | 恢复provider并重新执行完整操作，先确认无commit |
| evidence invalid | provider nil error但返回空/坏reference | provider contract故障，禁止写入 |

Approval通过不等于caller获权。当前没有真实adapter，禁止用固定字符串、环境变量或caller payload伪造production evidence。

## 15. 最小观测字段

未来装配时建议记录：

- correlation/request ID；
- operation kind；
- reviewed result class；
- Activity identity的受控表示；
- expected/observed state generation；
- publication version与kind；
- gate status/evaluatedAt；
- dependency stage与bounded latency；
- commit-unknown disposition；
- 未来authorized actor/audit reference。

禁止普通日志记录Award/Weight、完整manifest payload、approval payload、raw provider/SQL cause、DSN、cookie/token或credential。

## 16. 当前没有的“紧急按钮”

本节没有：

- production pause/kill switch；
- 自动rollback；
- canary/gray；
- scheduler；
- cache invalidation；
- MQ广播；
- 运营UI；
- API授权。

发生开发验收事故时，可执行的安全动作是停止测试writer/后续发布、保留数据、只读调查和修复代码/Migration。不要通过直接改表伪造运行时应急能力。

## 17. 清理与证据留存

验收完成后：

1. 先解析并记录专用schema、临时container、临时grant和测试artifact的精确名称；
2. 只删除本次创建且已确认可丢弃的资源；
3. 不删除用户现有volume、依赖cache、源码、credential或其他并行任务产物；
4. 保留命令、exit code、shuffle seed、MySQL版本、grant检查与失败注入结果；
5. 对日志中的secret/DSN做脱敏；
6. 运行`git status --short`确认没有fuzz corpus、coverage、binary或临时SQL落入工作树；
7. 文档中只记录实际证据，不补写固定通过数量。

## 18. 升级条件

立即停止该Activity写入并升级人工处理，如果出现：

- commit unknown reconciliation得到`indeterminate`；
- root active指向缺失或错Activity publication；
- half manifest或exact content与approved candidate不等；
- immutable history被UPDATE/DELETE；
- 长期runtime identity意外获得第30节写grant；
- cross-context ref被旁路写入且resolve失败；
- 发现服务被未经第31～35节授权边界装配到公开surface；
- 任意技术failure被上层转换为confirmed active。

升级时提供reviewed error class、受控identity、时间线、exact read-back三态和环境信息；不要在普通渠道粘贴底层cause或secret。

## 19. 当前真实证据与 root 冻结剩余项

2026-08-31（Asia/Shanghai）的此前候选已经完成：

- disposable MySQL **8.4.11**门禁连续两次完整 exit 0：真实v5 baseline→v11、repeat no-change、dirty
  fail-closed、五张新表、6个RESTRICT FK、20个enforced CHECK、binary collation、snapshot/Marketing
  isolated writer allowlist、1142越权拒绝、exact RR、并发/CAS、rollback/retire、half-write rollback与ACL
  bad-ref拒绝；
- 长驻Compose保持原MySQL container、`growthos_mysql_data`/`growthos_mysql_socket` volume与三个network
  identity不变完成`5:0`→`11:0`；旧五表行数/checksum不变，长期`growthos_app`仍只SELECT旧两表，八张
  未装配表继续1142；`make compose-status`、`make compose-smoke` exit 0；
- 独立Lottery Compose acceptance exit 0，覆盖Redis/MySQL故障降级恢复与并发/M1场景；随机project的
  container/volume/network/image/BuildKit/secret清理完毕，长驻identity未变；
- `make verify` exit 0，覆盖Go vet/test/doccheck、Web 152 tests、typecheck与production build。

Commit receipt/reconciliation收口后的完整最终候选又取得以下实际结果：

```text
execution date/timezone: 2026-08-31 / Asia/Shanghai
Go: go1.26.6
module context: GOMOD=当前GrowthOS-Go/go.mod；GOWORK为空
focused normal: exit 0
focused race (Marketing/Lottery): exit 0
full repository race: go test -race -count=1 ./...，exit 0
shuffle: seeds 1788183001..1788183020，20/20 exit 0
fuzz 10s each: exec 2,019,397 / 1,087,335 / 1,469,462；new interesting 0/0/0；corpus 11/44/10
coverage Lottery: domain 93.5% / application 88.3% / MySQL 81.8%
coverage Marketing: domain 96.0% / application 77.1% / ACL 81.9% / MySQL 84.8%
make verify: exit 0；Go vet/test/doccheck；Web 19 files/152 tests；typecheck；build 2462 modules
real MySQL 8.4.11: exit 0；schema 11:0；probes 0；cleanup 0；long-lived resources unchanged
Compose: clean 11/latest 11；status/smoke exit 0；resource identities unchanged
commit receipt: zero-result/explicit extraction/defensive copy/three-state fixtures exit 0
known exclusions: API/auth/RBAC/UI/Draw/inventory/MQ/gray
```

Fuzz exec/corpus与coverage数字只记录本次环境，不是KPI、SLO或生产风险结论。MySQL cleanup `0`指该
disposable gate核对的临时资源零残留；root仍需在全部文档/索引完成后处理本任务最终build artifact与工作树
清理。

尚未写入本文、也不得预写的是accepted tip/SHA/远端同名ref/线性历史。Root还需最终复跑architecture、
doccheck、diff，完成精确清理并冻结远端；这些剩余项不代表代码门禁仍待复跑。
