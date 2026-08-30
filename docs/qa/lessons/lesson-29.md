# 第 29 节 QA：封闭 Strategy 路由图求值验收

- **课程主题：** 实现规则决策引擎
- **需求基线：** [Lottery Strategy Routing Evaluation 基线 v1](../../product/lottery-strategy-routing-evaluation-v1.md)
- **架构决策：** [ADR-0025](../../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)
- **上一节：** [第 28 节 QA](lesson-28.md)
- **课程正文：** [第 29 节课程](../../course/part-04/lesson-29-rule-decision-engine.md)
- **API 记录：** [第 29 节 API 记录](../../api/lessons/lesson-29.md)
- **设计手记：** [第 29 节设计手记](../../design-thinking/lessons/lesson-29.md)
- **面试问答：** [第 29 节面试问答](../../interview/lessons/lesson-29.md)
- **基准提交：** `90844c1`（第 28 节已验收 tip）
- **当前实现序列：** `041dc30`、`27bd514`、`0ca25d2`、`51dbd61`、`a173d8b`、`3863b06`、`ab056ba`、`515d776`
- **验收日期：** 2026-08-30，Asia/Shanghai
- **当前记录状态：** product/ADR、共享 typed branch、共享 fact session、domain evaluator/decision/path、application exact-graph orchestration、低披露 error 与架构停止线已经形成。定向普通/race/20 轮 shuffle/vet、final-code 10 秒 fuzz、全仓普通/race、atomic coverage、Web test/typecheck/build 与第 28 节真实 MySQL 回归已实际通过；完整文档/索引收口后的 `make verify`、最终 doccheck/diff、线性历史、最终产物清理、远端同名分支与累计分支冻结仍明确保留为 pending

> 本节验收的是“给定 exact validated immutable graph 和一份权威会员事实时，Lottery 能否在有界 context 内形成唯一完整 Strategy route”。它不验收 Activity 发布、公开 API、真实会员 adapter、Strategy/Award selection、正式 Draw、会话、RBAC、UI 或浏览器闭环。课程标题中的“引擎”不能被用来外推通用 Rule Engine/DSL 已完成。

## 1. 证据状态词汇

| 状态 | 精确定义 |
| --- | --- |
| **ACTUAL-PASS** | 命令已经在当前本机工作树真实执行并 exit 0；只对该命令覆盖范围负责 |
| **CODE-EVIDENCE** | 实现和测试已经落盘并可审查，但不能代替 final accepted-tip 的独立复跑 |
| **FINAL-CANDIDATE-PASS** | 完整课程、API、QA、设计、面试和索引均已落盘后，在待冻结候选上实际 exit 0；仍不自动代表远端/cumulative refs 已冻结 |
| **FINAL-FREEZE-PENDING** | 必须在完整课程/API/QA/设计/面试/索引收口后的候选内容上执行，当前不得预写通过 |
| **OUT-OF-SCOPE** | 本节刻意没有交付；旧路径通过也不能伪装该能力存在 |

普通 `go test ./...` 可能使用 Go cache，也不会运行 fuzz target，更不会自动证明 Web、Compose、真实会员 authority 或浏览器 E2E。每条证据必须按实际命令边界解释。

## 2. 本节必须成立的能力

### 2.1 业务 branch 语义

- production rule code 精确为 `lottery.membership_tier.route_strategy`；
- confirmed premium 精确选择 `premium_override` + `premium_strategy_selected`；
- confirmed standard 精确选择 `baseline_default` + `baseline_strategy_selected`；
- `baseline_default` 不是 unknown、事实缺失、provider error、取消或坏 graph 的 catch-all；
- 第 27 节 concrete router 与第 29 节 graph evaluator 共享 package-private typed branch helper；
- helper 不选择 Strategy target、不访问 graph/provider/Clock，也不导出 generic `Rule`。

### 2.2 Exact snapshots 与 application 顺序

- 调用方必须提供已验证的 exact `(GraphID, Revision)` identity；
- graph reader 只调用一次，不能查 latest/active/list，不能 not-found 后换 revision；
- reader 返回 value+error 时 error 胜出，value 不得继续被验证或执行；
- 返回 graph identity 必须与请求完全相等，并再次通过 `Validate()`；
- graph invalid/not-found/wrong identity/depth over budget 时 Clock 与 fact reader 均零调用；
- 受控 business Clock 只调用一次并 canonicalize 为 UTC；
- 会员 fact reader 只调用一次，所有 decision 复用同一 snapshot；
- fact value+error 时 error 胜出；subject mismatch、future、stale、unsupported 均在 traversal 前失败；
- 成功调用依赖顺序精确为 graph -> Clock -> fact -> domain evaluator。

### 2.3 确定性单路径遍历

- 从 graph 显式 root 开始，使用迭代 `currentNodeID`；
- 每个 decision 只执行 closed v1 membership operator；
- selected branch必须在 outgoing edges中精确匹配一条；
- 不能取 edges[0]、依赖 map/SQL order 或 missing 时改走 default；
- 一个 step 等于求得 branch 并沿一条 edge 移动；terminal 不另计 step；
- shared successor合法，但一次 evaluation只走一个父 path；
- 到达 terminal 立即停止，不扫描未走 branch；
- path中每个 from/rule/branch/reason/to必须连续且与实际选择一致；
- visited set防御同一路径重复，不把静态合法 shared successor误报为 cycle。

### 2.4 Step/depth/time/cancellation budget

- `StrategyRoutingGraphStepBudget` 只允许 `1..16`；zero/17/forged 值失败；
- graph静态 depth仍必须 `<=16`；
- application在 Clock/fact之前要求 `graph.Depth() <= maxSteps`；
- domain loop在追加 edge前继续保留 actual step hard stop；
- depth16与 maxSteps16成功，maxSteps15不能先验证/使用 fact；
- `maxDuration` 必须为正；timer window 横跨 graph read、Clock、fact read 和 traversal，但只协作取消 context-aware reader/检查点，`Clock.Now()` 必须是有界本地调用且只能在返回后观察 timeout；
- caller deadline早于或等于 internal deadline时，caller拥有 deadline；
- 只有 internal deadline 严格更早时才安装 private cause；
- pre/mid/post caller cancellation与 internal timeout全部返回零 decision；
- 最后一次 success return 之前仍检查 context。

### 2.5 完整 immutable decision

- 成功 decision 保存 exact graph identity/schema/root/terminal/target；
- 保存 fact source/revision 与 canonical UTC evaluated-at；
- path长度在 1..budget，第一步从 root，最后一步到 terminal；
- 相邻 step 的 To/From 连续，branch/reason 精确配对；
- path内不允许重复 node；
- target必须与 evaluator实际到达的 terminal StrategyID一致；
- `Path()` 返回防御性副本；
- 任何失败返回整个 zero `StrategyRoutingGraphDecision`，不返回 partial path/target/last node；
- decision不包含 subject、raw tier、session、Principal、loaded Strategy、Award 或 Draw。

### 2.6 Error disclosure

- application wrapper只公开一个 reviewed stable class；
- `Error()` 不泄露 GraphID/Revision、NodeID、subject、tier、SQL、endpoint或raw cause；
- `errors.Is`不穿透 private cause；
- 可信代码必须显式 `Cause()` 才能读取诊断；
- internal timeout匹配 evaluation timeout class，但不匹配 `context.DeadlineExceeded`/`context.Canceled`；
- caller error原样优先；provider-owned deadline在 operation context均 live时保持 provider failure/unavailable；
- cleanup cancel不能被误报为 internal timeout。

## 3. 本节明确不验收

- generic `Rule` / `RuleEngine` / `DecisionEngine` / operator registry；
- `map[string]any` fact bag、DSL、DMN、OPA、XACML、script/plugin；
- 第二种 Lottery operator 或异构多事实决策；
- graph CRUD/list/latest/active/publish/retire/rollback；
- 第 30 节 Activity lifecycle 与 graph/Strategy 发布绑定；
- 真实 membership provider adapter、session 到 subject 的可信映射；
- Participation编排、Strategy load、WeightedSelector、random source；
- 正式 Draw/Result、幂等、库存、积分或 Benefit；
- HTTP/MCP/Agent endpoint、DTO、header、status或 error code；
- `cmd/growth-api`、app config或长期 Compose composition；
- 新 Migration、长期 runtime graph grant、Redis/RabbitMQ/PostgreSQL surface；
- React graph页面、导航、路由、操作、path展示或浏览器 E2E；
- 第 31～35 节 Principal/session/RBAC/data scope/frontend capability/越权验收；
- 生产 QPS、P95/P99、容量、可用性、渗透或合规审计结论。

## 4. 需求到证据覆盖矩阵

| 验收面 | 主要代码/测试证据 | 当前状态 | 不能推出 |
| --- | --- | --- | --- |
| shared typed branch oracle | `membership_routing.go` / `membership_routing_branch_test.go` | ACTUAL-PASS（随 domain 定向门禁） | 已有 generic operator |
| step budget 1..16 | constructor/Validate boundary tests | ACTUAL-PASS | 生产 budget 默认值已决定 |
| concrete router parity | standard/premium oracle + canonical edge trap | ACTUAL-PASS | 任意 rule均可执行 |
| shared terminal/decision path | convergence与two-step tests | ACTUAL-PASS | DAG会并行执行所有 branch |
| depth16与 worst-depth admission | domain/application boundary tests | ACTUAL-PASS | depth16是推荐业务建模 |
| `MaxUint64` identities | graph/domain evaluation test | ACTUAL-PASS | 网络 JSON/JS 已安全承载 unsigned ID |
| zero-decision协议 | invalid/forged/cancel assertions | ACTUAL-PASS | 失败已持久审计 |
| immutable decision/path | forgery + defensive-copy tests | ACTUAL-PASS | 对外披露已授权 |
| exact graph/Clock/fact 1/1/1 | dependency-order/count stubs | ACTUAL-PASS | 已接真实 graph/fact adapter runtime |
| typed-nil/config/input fail-fast | constructor与 nil service tests | ACTUAL-PASS | 配置中心/环境变量已完成 |
| caller/internal/provider priority | cancel/block/deadline/error+value tests | ACTUAL-PASS | context可强制抢占不合作 provider |
| low-disclosure wrapper | `Error`/`Is`/`Cause` tests | ACTUAL-PASS | 已有 HTTP mapping |
| 64 worker request isolation | domain/application concurrency tests | ACTUAL-PASS + race定向通过 | 注入的任意真实 adapter都线程安全 |
| evaluator fuzz target | `FuzzEvaluateStrategyRoutingGraphNeverPanicsLoopsOrReturnsPartialDecision` | ACTUAL-PASS；10 秒、2,569,203 execs、1 new interesting（total 41） | 对所有输入的穷举证明 |
| no generic engine/fact bag | AST architecture guard | ACTUAL-PASS（application定向门禁） | 未来永远不需要扩展 |
| no runtime/HTTP/Web/Compose wiring | production-source identifier guard | ACTUAL-PASS（application定向门禁） | 生产部署/浏览器已测试 |
| repository/frontend regression | 全仓普通/race 与 Web test/typecheck/build | ACTUAL-PASS；最终聚合 `make verify` 仍待文档收口后复跑 | 当前可以宣称 accepted tip |
| remote/cumulative/main refs | push后 exact refs | FINAL-FREEZE-PENDING | 远端已冻结 |

## 5. 按提交核查真实演进

| 顺序 | 提交 | 演进焦点 |
| ---: | --- | --- |
| 1 | `041dc30` | 先冻结 exact graph、single fact/time、closed dispatch、budget 与停止线产品语言 |
| 2 | `27bd514` | ADR 比较 direct if/else、裸行执行、重复读事实、registry/DSL、Activity/selection混装等方案 |
| 3 | `0ca25d2` | 从第 27 节 concrete router 抽出 package-private typed branch oracle，并保持旧行为 |
| 4 | `51dbd61` | 抽出 Clock + authority read + freshness session，让旧/新 application service共享同一事实边界 |
| 5 | `a173d8b` | 实现 step budget、immutable decision/path、iterative exact-branch evaluator、并发/fuzz测试 |
| 6 | `3863b06` | 实现 exact graph application orchestration、deadline ownership、低披露错误和 1/1/1调用证据 |
| 7 | `ab056ba` | 扩展架构测试，防止 evaluator被提前装配到 runtime/HTTP/Compose/Web |
| 8 | `515d776` | 把同输入并发证据加固为两组 subject/identity/tier/branch/target 交错的 64 请求隔离证据 |

可复核当前线性切片：

```bash
git log --reverse --oneline 90844c1..HEAD
git diff --stat 90844c1..HEAD
git diff --name-only 90844c1..HEAD
```

文档与索引提交将继续线性追加，因此最终提交数会增长；这里的八个 SHA 只描述已形成的产品/实现切片，不预写后续文档 SHA。

## 6. Shared branch oracle 验收

### 6.1 为什么它必须 package-private

`evaluateMembershipRoutingBranch` 共享的只是一个已经被证明的映射：

| 输入 tier | branch | reason |
| --- | --- | --- |
| `standard` | `baseline_default` | `baseline_strategy_selected` |
| `premium` | `premium_override` | `premium_strategy_selected` |

导出 `Rule` interface会让调用方误以为可以注册任意事实和 operator。保持 package-private可以让第 27 节 router与第 29 节 evaluator共享源代码，同时不新增跨 bounded-context扩展承诺。

### 6.2 负向输入

测试覆盖：

- zero fact；
- zero evaluated-at；
- observed-at晚于 evaluated-at；
- 等价本地时区/UTC instant 得到相同 branch/reason。

失败时 branch/reason 都是空值。Helper不执行 baseline fallback。

## 7. Domain evaluator 验收

### 7.1 Canonical-order trap

Graph accessor返回的 canonical edge中 baseline排在 premium前。Premium fixture仍必须选择 premium target，并与第 27 节 oracle的 target/branch/reason/provenance/time全部一致。

这直接防止以下回归：

```go
selected := graph.OutgoingEdges(root)[0]
```

### 7.2 Shared successor 与 path evidence

两条 branch可以指向同一个 terminal。Standard/premium最后 target相同，但第一个 path step的 branch/reason必须不同。

两条 branch也可以先汇聚到同一个 decision，再共同走第二步。测试要求：

```text
step[0].To == step[1].From
```

且两步都保留当前 tier对应的 exact branch/reason。Shared successor不是 cycle，也不会让未选择父 path进入结果。

### 7.3 Depth 与 worst-path admission

Depth-16 graph的 premium path包含 16 steps并到达 target。相同 graph配 budget15、甚至传入 zero fact时，必须先返回 step-budget exceeded，而不能先暴露 fact invalid。这证明准入顺序是 graph worst depth优先。

Application有同一层 evidence：depth2/maxSteps1时 graph reader调用1次，而 Clock/fact为0；maxSteps2时 1/1/1并成功。

### 7.4 Decision forgery

测试逐项篡改：

- identity/schema/root/terminal/target；
- terminal target一致性；
- fact source/revision；
- canonical evaluated-at；
- budget/path length；
- first source/path continuity；
- rule/branch/reason；
- repeated node/final terminal。

任何伪造都让 `Confirmed()` 为 false。`Path()` 返回 slice被外部改写后，原 decision仍 confirmed。

### 7.5 Cancellation checkpoint

一个自定义 counting context锁定 depth-one成功路径在 final return前仍执行最后一次 `Err()` 检查。另一个 cancel-after context在该 checkpoint恰好返回 cancellation，结果必须是 zero decision。

这个测试不是依赖 `time.Sleep` 的概率竞态，而是针对控制流 checkpoint 的确定反例。

## 8. Application orchestration 验收

### 8.1 精确依赖顺序

Happy path实际断言：

```text
calls == [graph, clock, fact]
```

Graph reader与 fact reader收到同一个 derived child context；child保留 caller context value，但不是原 caller context本身。Graph identity精确相等，subject ref精确相等。

### 8.2 输入与配置 fail-fast

构造器拒绝：

- nil/typed-nil graph reader；
- nil/typed-nil fact reader；
- nil/typed-nil pointer/function Clock；
- zero/negative maxFactAge；
- zero step budget；domain `Validate()` 另覆盖手工 forged out-of-range，service 构造器复用该校验；
- zero/negative maxDuration。

`nilService.Validate()` 和 `nilService.Evaluate()` 失败关闭，不 panic。Nil context、zero subject、zero graph identity都在依赖前返回 invalid argument。

### 8.3 Graph error边界

测试覆盖：

- graph value + private repository error：error胜出，Clock/fact 0；
- provider返回 wrapped `context.DeadlineExceeded`，但 caller/child均 live：分类为 evaluation failure，不是 caller/internal timeout；
- wrong exact identity：graph-invalid；
- not-found：保留 graph not-found reviewed class；
- stored-invalid：映射为 evaluation graph-invalid，普通 `errors.Is`不能穿透 repository/storage cause。

### 8.4 Fact error边界

Fact reader同时返回 valid fact与 provider deadline时，既有 fact error wrapper将其分类为 fact unavailable；decision为零。Service不因 graph已成功而继续执行 returned fact。

Zero Clock在 fact reader前停止。Fact stale/future/subject mismatch等既有 helper回归由 application package全套测试共同覆盖。

## 9. Deadline 与错误优先级验收

### 9.1 Internal graph/fact timeout

Blocking graph reader与 blocking fact reader都：

- 先关闭 started channel，证明调用已进入依赖；
- 等待传入 context Done；
- 不使用 sleep 猜测何时超时；
- 外层只用 3 秒 watchdog 防测试永久挂死。

Internal timeout结果要求：

```text
errors.Is(err, ErrStrategyRoutingGraphEvaluationTimedOut) == true
errors.Is(err, context.DeadlineExceeded) == false
errors.Is(err, context.Canceled) == false
Cause() == privateInternalDeadlineCause
```

Fact reader随后返回 provider error时，internal timeout仍优先，不能泄露 provider错误类。

### 9.2 Caller earlier/equal deadline

端到端 earlier test让 caller deadline短于 service maxDuration，graph reader观察到的 deadline必须与 caller精确相等，最终 error必须是原始 `context.DeadlineExceeded`，不是 internal timeout。

Helper test再使用绝对过去时间验证：

- caller deadline与 internal相等；
- caller deadline早于 internal；
- internal严格早于 live caller；
- 主动 cleanup cancel。

前两种保留 caller cause；第三种使用 private internal cause；第四种不能误报 timeout。

## 10. 并发、race 与 fuzz边界

### 10.1 64-worker evidence

Domain 64 workers 共享同一 immutable graph/fact/budget，只证明相同输入并发可重复且不报告错误。Application 测试则交错两组不同 subject、graph identity、tier、branch 与 target，逐个 result 校验 identity/target/path，并分别统计：

```text
premium graph identity calls  == 32
standard graph identity calls == 32
premium subject fact calls    == 32
standard subject fact calls   == 32
shared Clock total calls      == 64
```

因此 application 被测的 64 个交错请求各自读取一次依赖，且 subject/identity/tier/target/path 没有串线。它仍不证明任意未来真实 adapter 都线程安全，也不把共享 test-double 计数器说成请求私有状态。

### 10.2 Race当前证据

以下定向 race已真实 exit 0：

```bash
go test -race -count=1 \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

它证明这两个 package当前测试覆盖路径没有报告 data race；不证明未装配的真实 provider、HTTP、MySQL或所有第三方 adapter都并发安全。

### 10.3 Fuzz 实际证据与边界

Fuzz target已经落盘，并覆盖：

- depth 1..16；
- standard/premium/unknown tier；
- budget 0..17投影；
- future fact；
- derived depth/root/edge/kind/schema/id/revision mutations；
- 成功 path 在 1..maxSteps/16 内；
- 失败只能返回 zero decision。

在当前 production 实现上实际执行：

```bash
go test ./internal/lottery/domain \
  -run='^$' \
  -fuzz='^FuzzEvaluateStrategyRoutingGraphNeverPanicsLoopsOrReturnsPartialDecision$' \
  -fuzztime=10s
```

- **状态：** ACTUAL-PASS，exit 0；
- **结果：** 2,569,203 次执行，新增 1 个 interesting input，总数 41；
- **边界：** coverage-guided fuzz 不是输入空间穷举，也没有经过 Repository、真实 timer 调度或外部 provider；执行数不能冒充数学证明。

## 11. 架构停止线验收

### 11.1 Generic abstraction guard

既有 AST guard对 Lottery/Participation domain/application拒绝：

- `Rule`、`RuleChain`、`RuleTree`、`RuleEngine`、`DecisionEngine`；
- registry/expression/script/DSL相关类型；
- production generic type/function；
- `map[string]any`/string-to-empty-interface fact bag；
- 跨 bounded-context project import。

注意：领域名 `StrategyRoutingGraph` 是已批准具体 aggregate，guard禁止的是泛化 `RuleTree`/engine，而不是把当前 graph删掉。

### 11.2 Runtime/HTTP/Web/Compose guard

Lesson29 guard扫描 production sources不得引用：

- `StrategyRoutingGraphEvaluationService`；
- `NewStrategyRoutingGraphEvaluationService`；
- `EvaluateStrategyRoutingGraph`；
- `StrategyRoutingGraphDecision`；
- `StrategyRoutingGraphStepBudget`。

扫描范围包括 `cmd`、HTTP adapter/server、app config、非 acceptance Compose、Docker production source和 Web production source。测试/mocks/node_modules被明确排除，避免 fixture文字误报。

这证明当前 source没有通过这些标识符直接装配 evaluator；它不等于所有未来动态调用、部署环境或浏览器安全都已证明。

## 12. 当前已实际执行的定向门禁

文档形成前，在 `codex/lesson-29-rule-decision-engine` 当前实现上实际执行：

```bash
/usr/bin/time -p go test -count=1 \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

- **状态：** ACTUAL-PASS，exit 0；
- **package输出：** domain `0.764s`，application `1.838s`；
- **wall time：** `real 2.04s`；
- **覆盖：** 两个 package全部普通 tests，包括 architecture guard与 deadline/concurrency tests。

实际执行：

```bash
/usr/bin/time -p go test -race -count=1 \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

- **状态：** ACTUAL-PASS，exit 0；
- **package输出：** domain `2.272s`，application `2.201s`；
- **wall time：** `real 2.61s`；
- **结果：** 无 race report。

实际执行：

```bash
/usr/bin/time -p go test -shuffle=on -count=20 \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

- **状态：** ACTUAL-PASS，exit 0；
- **package输出：** domain `1.026s`，application `14.828s`；
- **wall time：** `real 14.99s`；
- **结果：** 20轮通过，当前测试不依赖固定 package test顺序。

实际执行：

```bash
/usr/bin/time -p go vet \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

- **状态：** ACTUAL-PASS，exit 0；
- **wall time：** `real 0.12s`；
- **结果：** 两个 package vet无诊断。

这些是实现阶段定向证据，不是完整文档/索引收口后的 accepted-tip gate。

### 12.1 当前文档草稿检查

课程、API 与本 QA 落盘，且并行设计/面试/运维文档已经可见后，实际执行：

```bash
/usr/bin/time -p go run ./cmd/doccheck
git diff --check
git diff --no-index --check /dev/null <每一份新增 Lesson29 文档>
```

- **状态：** ACTUAL-PASS；
- **doccheck：** 输出 `documentation checks passed`，`real 0.32s`；
- **whitespace：** tracked diff 与本代理三份 untracked 文档均无 `--check` 诊断；
- **边界：** `git diff --no-index` 会因为两个输入内容不同正常返回 1，所以这里只以其 `--check` 是否输出 whitespace error 判断；
- **边界：** 这是当前文档形成阶段检查，不替代所有索引与后续提交收口后的 final doccheck。

### 12.2 全仓普通/race 实际证据

在同一 production 实现候选上实际执行并 exit 0：

```bash
go test -count=1 ./...
go test -race -count=1 ./...
```

全仓 race 无报告。随后新增的 64-worker 异构请求隔离测试也单独通过普通、race 与 20 轮 shuffle；完整文档候选仍会由最终 `make verify` 再聚合执行。

### 12.3 Atomic coverage 实际证据

使用 `mktemp -d` 创建任务专用临时目录并以 `-covermode=atomic` 生成 profile，实际结果为：

| 口径 | 覆盖率 |
| --- | ---: |
| `internal/lottery/domain` | 93.6% |
| `internal/lottery/application` | 88.3% |
| 两个 package 合并 profile | 92.1% |

Coverage 只表示本次 statement coverage，不证明语义完整、生产路径或安全性。临时 profile 和临时目录已按精确路径删除；没有在仓库留下 `coverage.out` / `*.prof`。

### 12.4 Web 门禁实际证据

Web test、typecheck 与 build 已实际通过：19/19 个 test files、152/152 个 tests。Build 当时生成以下 6 个 task-only 文件：

```text
web/dist/index.html
web/dist/assets/AdminDashboardPage-BozlG9Gm.js
web/dist/assets/ProductPage-BxD9cmG4.js
web/dist/assets/UserHomePage-Dls9IY2p.js
web/dist/assets/index-DXcl1hu0.css
web/dist/assets/index-DtgKhDQL.js
```

它们已逐个删除，并依次移除空的 `web/dist/assets`、`web/dist` 目录；预先存在的 `web/node_modules` 和依赖 cache 保留。

### 12.5 第 28 节真实 MySQL 上游回归

`make lesson28-mysql-acceptance` 已在一次性 MySQL 8.4.11 上重跑并 exit 0，六组 Integration 覆盖 MySQL pools、Migration runner、Lottery schema、routing graph schema、Strategy Repository 与 graph Repository。Disposable 资源由脚本清理。

这只证明第 29 节没有破坏它所消费的 schema/Repository 输入契约；本节 evaluator 仍未装配 graph Repository 或真实 membership provider，不能把这项回归写成 evaluator 的数据库端到端验收。

## 13. 已执行门禁与 post-doc freeze 矩阵

本节把“production 实现上已经执行”与“完整文档/索引后的 accepted-tip 候选复跑”分开。前者不能自动替代后者。

| 验收项 | 命令/证据 | 当前状态 |
| --- | --- | --- |
| 独立 evaluator fuzz | 10 秒、2,569,203 execs、1 new / total 41 | ACTUAL-PASS |
| 全仓普通测试 | `go test -count=1 ./...` | ACTUAL-PASS |
| 全仓 race | `go test -race -count=1 ./...` | ACTUAL-PASS |
| atomic coverage | domain 93.6%、application 88.3%、combined 92.1% | ACTUAL-PASS |
| Web test/typecheck/build | 19/19 files、152/152 tests、typecheck/build | ACTUAL-PASS；build 产物已精确清理 |
| 第 28 节 MySQL 上游回归 | MySQL 8.4.11 六组 Integration | ACTUAL-PASS；不是 evaluator E2E |
| post-doc 文档检查 | `go run ./cmd/doccheck` | FINAL-FREEZE-PENDING |
| post-doc 聚合门禁 | `make verify` | FINAL-FREEZE-PENDING |
| 章节 whitespace diff | `git diff --check 90844c1..HEAD` | FINAL-FREEZE-PENDING |
| runtime stop-line diff | 精确检查 cmd/web/http/appconfig/compose/migrations | FINAL-FREEZE-PENDING |
| 线性历史 | base ancestor + `rev-list --merges` | FINAL-FREEZE-PENDING |
| clean worktree/artifacts | 枚举并精确清理本任务生成物 | FINAL-FREEZE-PENDING |
| remote lesson branch | local HEAD 与同名 origin ref 相等 | FINAL-FREEZE-PENDING |
| cumulative branch | `codex/complete-implementation` fast-forward 到 accepted tip | FINAL-FREEZE-PENDING |
| main 不变 | local/origin main 仍为进入章节前基线 | FINAL-FREEZE-PENDING |

## 14. Final gate建议命令

```bash
go test ./internal/lottery/domain \
  -run='^$' \
  -fuzz='^FuzzEvaluateStrategyRoutingGraphNeverPanicsLoopsOrReturnsPartialDecision$' \
  -fuzztime=10s

go run ./cmd/doccheck
make verify
go test -race -count=1 ./...
git diff --check 90844c1..HEAD
```

停止线建议分别核查：

```bash
git diff --name-only 90844c1..HEAD

git diff 90844c1..HEAD -- \
  cmd \
  web \
  deploy/compose \
  deploy/docker \
  migrations \
  internal/lottery/adapter/httpapi \
  internal/infrastructure/httpapi \
  internal/infrastructure/httpserver \
  internal/platform/appconfig
```

Production source预期不出现 evaluator wiring。Docs/architecture test本身出现 Lesson29文字不构成 runtime扩张。

## 15. 为什么本节不新增真实依赖验收脚本

第 29 节没有新增：

- schema/Migration；
- MySQL repository SQL；
- Redis key/ACL；
- RabbitMQ exchange/queue；
- PostgreSQL projection；
- Docker service/network/secret；
- 真实会员 authority adapter。

因此没有合理目标去复制一个 Lesson29 disposable MySQL/Redis/RabbitMQ/PG脚本。第 28 节 graph Repository真实 MySQL证据仍是上游已冻结事实；本节 final全仓回归只需证明没有破坏它，不能把旧 integration默认 skip误写为重新验收真实 MySQL。

“用户环境已经安装依赖”不等于每节必须人为增加一个依赖。没有新的网络/存储边界时，纯 domain/application定向、race/fuzz和架构停止线才是与风险匹配的证据。

## 16. 停止线与精确负向断言

出现以下任一情况，本节应判不通过：

- 新建 generic `RuleEngine`、operator registry 或 untyped fact bag；
- 第 27 节 concrete router 和 graph evaluator 各自保留独立 tier switch 并可漂移；
- 读取 graph 后按 tier 直接拿 target，绕过 root/node/edge/path；
- graph/fact在一个多步 path中重复读取；
- graph not-found后自动尝试 latest/another revision；
- reader返回 wrong identity仍继续；
- canonical edges第一项被当 priority；
- premium edge缺失时改走 baseline default；
- unknown/stale/future/provider error被当 standard；
- 只依赖静态 depth，不保留 runtime step hard stop；
- 只按实际短路径执行，允许同 graph 在相同 budget 下产生事实相关部分可用；
- maxSteps zero/17或 maxDuration non-positive被当 unlimited；
- caller相同 deadline与 internal timer竞争，分类取决于调度；
- provider-owned `context.DeadlineExceeded`被误报为 caller/internal timeout；
- internal timeout通过 `errors.Is`泄露 `context.DeadlineExceeded`；
- cleanup cancel被误报为 timeout；
- value+error时继续使用 graph/fact value；
- cancellation/timeout/error返回 partial path或 target；
- `Path()` 暴露内部 slice；
- decision只检查 target非零就 confirmed；
- subject/raw tier/credential/permission被写进 decision path；
- 到达 terminal 后顺带 load Strategy 或调用 selector/random；
- 把 evaluator 装进 cmd/HTTP/Compose/Web 却仍称“未装配”；
- 新增 Activity/publish/auth/UI/Migration/cache/event 等超范围能力；
- 只运行定向 tests 就预写 final fuzz、make verify、前端、远端冻结通过。

## 17. 剩余风险与下一责任

| 风险 | 当前影响 | 下一触发器 / 责任 |
| --- | --- | --- |
| 只有一个 typed operator | 多步只能重复同一 membership事实语义 | 第二个真实 Lottery rule出现时新 ADR |
| GraphID/Revision由调用方显式提供 | 不能说明哪个 revision正在业务生效 | 第 30 节 Activity publication/binding |
| target只有 StrategyID | 不证明 Strategy version/published/available | 第 30 节发布一致性与后续 selection主链 |
| maxDuration无 runtime默认值 | 当前 service未装配 | 首个真实 composition拥有上层总预算时决定 |
| context为 cooperative | 不合作 provider可能拖延退出 | 真实 adapter出现后 timeout/circuit-breaker验收 |
|每次 graph重新 Validate | 128/256上限内可控，但无生产 latency证据 | profiling证明瓶颈后评估 compiled plan/cache |
| path只在内存 | 不能审计、查询或回放 | 有真实审计消费者与权限/retention后设计持久化 |
| path含会员派生 branch | 若公开可能泄露等级信息 | 第 31～35 节 disclosure/scope/E2E |
|没有真实 membership adapter | 当前只能以 stub证明 orchestration | authority契约与 session-subject mapping形成后集成验收 |
|没有正式副作用 | route success不能恢复 Draw | 后续幂等 Draw/库存/Benefit章节 |

## 18. 清理与环境影响

### 18.1 当前已完成的精确清理

当前门禁产生过的 task-only 资源已按明确目标清理：

- atomic coverage 使用的 `mktemp` 目录及其中 profile 已精确删除，仓库没有 `coverage.out` / `*.prof`；
- Web build 的 `index.html` 与 5 个 hashed assets 已逐个删除，空的 `assets` / `dist` 目录已依次移除；
- 10 秒 fuzz 没有写入失败 corpus 或仓库 `testdata/fuzz`；Go test/build cache 属于可复用依赖，保留；
- 第 28 节 MySQL acceptance 的 disposable container/network/volume/credentials 由脚本清理，长期 Compose 资源不作为删除目标；
- 没有下载图片或工具二进制，没有创建 RabbitMQ/PostgreSQL 任务资源；
- 预先存在的 `web/node_modules`、Go cache、Docker 基础镜像、长期 volume 与 credentials 均保留。

### 18.2 Final freeze责任

final `make verify` 会再次生成 `web/dist`；若 coverage/fuzz 或其他复跑再创建 disposable 资源，根任务必须：

1.先精确枚举本任务创建的路径/资源；
2.逐项删除，不对 workspace/cache/volume做宽泛递归；
3.保留 pre-existing `web/node_modules`、Go cache、Docker基础镜像、长期 Compose volume与 credentials；
4.清理后重新核对 `git status --short`与目标资源不存在；
5.在最终 QA回填实际清理事实。

当前只能确认上述已执行批次的产物已清理；post-doc final gate 的第二次清理仍是 FINAL-FREEZE-PENDING。

## 19. 文档同步状态

- Git仓库内文档是当前可验证事实源；
- 本代理没有执行外部 Vault/Obsidian 同步，也不声称外部知识库已更新；
- 课程/API/QA 当前由本任务生成，设计手记、面试问答与索引由同一 Lesson29 根任务线性收口；
- 完整文档落盘后仍需独立 doccheck 与 tracked/untracked whitespace 检查；
- 同名远端分支与累计分支只有在最终文档提交、push 和 ref 核对后才能称为冻结。

## 20. 当前验收清单

- [x] 产品基线冻结 exact graph、single fact/time、closed dispatch、budget与停止线；
- [x] ADR比较并拒绝 direct target shortcut、裸行执行、重复读、registry/DSL、Activity/selection混装；
- [x] shared typed branch helper已实现，并由第 27/29节共同消费；
- [x] shared Clock/fact/freshness session已实现，并保持旧 router回归；
- [x] step budget、immutable path/decision和 iterative evaluator已实现；
- [x] exact branch lookup、shared successor、depth16、MaxUint与 zero-decision tests已形成；
- [x] application exact graph/1 Clock/1 fact orchestration已实现；
- [x] caller earlier/equal、internal strictly earlier、provider-owned deadline优先级已实现；
- [x]低披露 `Error`/`Is`/explicit `Cause` wrapper已实现；
- [x] 64-worker、race与 architecture guard tests已形成；
- [x] domain/application普通测试已实际 exit 0；
- [x] domain/application race已实际 exit 0；
- [x] domain/application 20轮 shuffle已实际 exit 0；
- [x] domain/application vet已实际 exit 0；
- [x] application 64-worker 异构 subject/identity/tier/target 隔离测试已通过普通/race/20 轮 shuffle；
- [x] 独立 evaluator 10秒 fuzz 已实际通过（2,569,203 execs，1 new / total 41）；
- [x] 全仓普通与全仓 race 已实际通过；
- [x] atomic coverage 已记录 93.6%/88.3%/92.1%，临时 profile 已清理；
- [x] Web 19/19 files、152/152 tests、typecheck/build 已通过，首轮 6 个 dist 文件已清理；
- [x] MySQL 8.4.11 六组 Integration 上游回归已通过且 disposable 资源已清理；
- [ ] 完整课程/API/QA/设计/面试与索引收口后的 doccheck；
- [ ] post-doc final candidate `make verify` 聚合复跑；
- [ ] `90844c1..HEAD` diff/stop-line/线性历史核对；
- [ ] final gate 再生成的任务自有产物精确清理与 clean worktree；
- [ ] 冻结提交 push 后同名远端 SHA 核对；
- [ ] 累计 `codex/complete-implementation` fast-forward；
- [ ] main/local origin保持不变复核。

在最后七项真实完成前，本节只能称为“实现切片与 pre-freeze 实际证据已形成”，不能称为远端 accepted tip 已冻结。最终 SHA 不能安全预写在产生该 SHA 的提交内容里，应以实际 Git refs 和根任务最终交接记录为准。
