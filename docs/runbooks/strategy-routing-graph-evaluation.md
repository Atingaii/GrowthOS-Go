# Lottery Strategy Routing Graph 求值验收与故障分诊手册

**状态：** 第 29 节内部能力基线  
**更新日期：** 2026-08-30  
**适用分支：** `codex/lesson-29-rule-decision-engine`  
**当前运行状态：** 未装配到 `growth-api`、HTTP、React、Compose 或任何生产事实适配器

本手册回答两个容易被混在一起的问题：

1. 当前怎样验证一个有界、确定、失败关闭的 Lottery 路由图求值内核；
2. 未来真正装配后，值班人员应怎样区分 caller、内部预算、graph Repository、会员事实和领域不变量故障。

它不是“规则平台操作手册”。第 29 节没有发布按钮、active revision、Activity 绑定、公开 endpoint、后台页面、真实会员 provider、认证、RBAC 或持久化 decision audit。因而当前没有可以在线启停的 evaluator，也没有可以由浏览器填写的 `maxSteps` / `maxDuration`。本手册中的运行命令用于开发与候选发布验收；未来出现正式 composition root 后，必须在不删除本节失败语义的前提下追加运行参数、指标、告警和回滚步骤。

规范性边界见：

- [Lottery Strategy Routing Graph 求值基线 v1](../product/lottery-strategy-routing-evaluation-v1.md)
- [ADR-0025：Lottery Strategy Routing Graph 封闭求值](../decisions/ADR-0025-lottery-strategy-routing-graph-evaluation.md)
- [第 29 节课程](../course/part-04/lesson-29-rule-decision-engine.md)
- [第 29 节 QA](../qa/lessons/lesson-29.md)

## 1. 当前可运维对象是什么

当前真正存在的是一个未装配的 application use case：

```text
exact (GraphID, Revision)
  -> StrategyRoutingGraphReader.FindByIdentity once
  -> validate exact immutable graph and worst depth
  -> MembershipRoutingClock.Now once
  -> MembershipTierFactReader.FindMembershipTierFact once
  -> validate subject/future/freshness
  -> closed typed domain traversal
  -> one confirmed immutable decision or one error
```

这条链有四个重要操作含义：

- graph 必须由调用方显式给出完整 identity，服务不会猜 `latest` 或退到另一 revision；
- graph 最坏深度超过服务端 step budget 时，在 Clock 和会员事实读取前拒绝；
- 一次调用中的所有 decision node 复用同一 graph、同一 fact 和同一业务 `evaluatedAt`；
- 任意失败都返回整个 zero decision，不返回 prefix path、临时 target 或“先按默认规则继续”。

当前不能把以下对象当作已经存在：

- graph publish/activate/archive 状态；
- Activity 到 graph revision 的绑定；
- Strategy aggregate 的加载与 Award 随机选择；
- Participation 资格链与 Lottery graph 的在线组合；
- 用户身份、操作者身份、角色、权限或数据范围；
- decision/path 的数据库审计记录；
- production timeout 默认值或 SLO。

## 2. 三种时间不能混用

排障前先区分三种时间：

| 时间 | 所有者 | 用途 | 不能代表 |
| --- | --- | --- | --- |
| `evaluatedAt` | `MembershipRoutingClock` | 判断 fact 是否来自未来、是否过期，并进入成功 decision 证据 | wall-clock 执行耗时、caller deadline |
| caller deadline | 上游调用方 | 上游对整个调用链的预算；更早或相同 deadline 拥有错误语义 | 本服务的内部超时 |
| internal deadline | graph evaluation service | timer window 横跨 graph read、Clock、fact read 与 traversal；协作取消 context-aware reader 并在本地检查点停止 | P99、wall-clock 硬上界、业务发生时间 |

`maxDuration` 到期只能取消尊重 context 的依赖或在检查点停止本地遍历。`MembershipRoutingClock.Now()` 不接收 context，必须保持为有界本地调用；如果它阻塞，只能在返回后的检查点观察 timeout。这个预算不能强制中断忽略 context 的 provider，也不能证明服务满足某个生产延迟目标。

## 3. 服务端受控参数

第 29 节构造器要求全部参数显式提供：

| 参数/依赖 | 当前约束 | 失败位置 |
| --- | --- | --- |
| `StrategyRoutingGraphReader` | 非 nil 且拒绝 typed-nil | 构造/`Validate` |
| `MembershipTierFactReader` | 非 nil 且拒绝 typed-nil | 构造/`Validate` |
| `MembershipRoutingClock` | 非 nil 且拒绝 typed-nil | 构造/`Validate` |
| `maxFactAge` | `> 0` | 构造/`Validate` |
| `StrategyRoutingGraphStepBudget` | `1..16` | domain 构造与 service `Validate` |
| `maxDuration` | `> 0` | service 构造/`Validate` |
| subject ref | 非零 opaque ref | 每次 `Evaluate` 入参校验 |
| graph identity | 合法非零 ID 与合法 revision | 每次 `Evaluate` 入参校验 |

这些配置当前没有环境变量，也不能来自 graph row、HTTP query、浏览器、本地存储或会员 provider。未来装配时，必须由受信 runtime configuration 提供并接受边界测试；不允许为了“方便调试”让用户扩大 step/time budget。

## 4. 开发验收前检查

在仓库根目录执行：

```bash
git status --short --branch
git branch --show-current
git rev-parse HEAD
git rev-parse origin/codex/lesson-29-rule-decision-engine
```

候选验收应满足：

1. 当前分支是 `codex/lesson-29-rule-decision-engine`；
2. 分支线性包含第 28 节已验收 tip；
3. `main` 与 `origin/main` 未被本节推进；
4. 没有不明工作树文件；
5. Go/前端依赖来自项目现有 lock/module 文件，不为本节临时升级。

精确 ancestry 检查：

```bash
git merge-base --is-ancestor \
  origin/codex/lesson-28-rule-tree-schema \
  codex/lesson-29-rule-decision-engine

git rev-list --merges \
  origin/codex/lesson-28-rule-tree-schema..codex/lesson-29-rule-decision-engine
```

第一条应以状态 0 结束；第二条应没有输出。本节学习历史不使用 merge commit 拼接旁支。

## 5. 最小功能验收

先验证两个直接拥有新行为的包：

```bash
go test -count=1 -shuffle=on \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

验收至少应覆盖：

- depth-1 standard/premium 与第 27 节 concrete router 等价；
- canonical edge 顺序为 default-first 时仍按 exact branch 命中；
- 两个 branch 指向同一 terminal 时仍保存不同 branch/reason；
- 两个 branch 汇聚到同一 decision 后可以继续形成连续 path；
- depth 16 成功、graph worst depth 超预算在 fact read 前失败；
- graph reader、Clock、fact reader 分别最多一次；
- error 与 value 同时返回时 error 胜出；
- Path accessor 防御性复制，伪造 decision 不能 `Confirmed`；
- pre-cancel、遍历中取消、最终成功返回前取消都返回 zero decision；
- domain 64-worker 对同一 immutable graph/fact/budget 得到一致结果；
- application 64-worker 交错两组 subject、graph identity、tier、branch 与 target，逐请求核对结果，并按 identity/subject 核对各 32 次依赖调用，证明这些被测输入没有串线；共享 Clock 的总调用数仍是 64。

如果 focused test 失败，先不要运行全仓或 Compose 验收；按第 10～13 节的分诊顺序定位，避免用更大的测试输出掩盖首个语义差异。

## 6. Race、重复顺序与 fuzz

### 6.1 Race

```bash
go test -race -count=1 \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

Race 通过只说明被测试路径没有被检测到数据竞争。它不能证明 provider 本身线程安全，也不能替代生产并发、故障注入或对象授权测试。

### 6.2 重复随机顺序

```bash
go test -count=20 -shuffle=on \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

重复随机顺序主要捕获测试间共享状态、全局 registry、未复位 stub 和依赖顺序耦合。它不是统计性能测试。

### 6.3 有界 fuzz

```bash
go test -run '^$' \
  -fuzz '^FuzzEvaluateStrategyRoutingGraphNeverPanicsLoopsOrReturnsPartialDecision$' \
  -fuzztime=10s \
  ./internal/lottery/domain
```

fuzz 组合 depth、tier、budget、future fact 与 graph identity/topology/schema mutation。成功样本必须形成 confirmed path；失败样本必须保留整个 zero decision。执行次数和 `new interesting` 数量依机器、worker、Go cache 与 seed 而变，只记录当次事实，不能写成固定门槛。

## 7. 全仓回归

第 29 节没有改变公开 API、Migration、Compose 或 React，但仍必须证明既有能力未被破坏：

```bash
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
make doc-check

cd web
pnpm run test
pnpm run typecheck
pnpm run build
```

前端验证的结论只能是“现有前端未回归”。它不能写成“已有规则图页面”或“前端按权限展示 graph”。

`pnpm run build` 会生成 `web/dist`。完成验证后应先列出本次文件，再逐个删除并移除空目录；不要删除 `web/node_modules`、pnpm store、Vite/Vitest 可复用 cache 或任何用户文件。

## 8. 数据库回归为什么仍然有用

求值器本身不新增 SQL 或 Migration，也没有装配 graph MySQL Repository。若需要验证“新内核没有破坏它所消费的第 28 节持久化输入”，可以运行：

```bash
make lesson28-mysql-acceptance
```

该门禁使用一次性 MySQL 8.4.11 和任务专用身份，验证 Migration latest 5、旧两表、graph 三表、Repository round-trip、事务、权限与清理。正确结论是“第 28 节输入契约仍可用”；不能据此宣称第 29 节 service 已经通过真实 MySQL 运行，因为本节没有 composition 将二者连接。

运行后应确认脚本报告临时资源清理成功；不要手工删除名称不明的 Docker volume/network，也不要将长期 `growthos` 栈当成 disposable 目标。

## 9. 架构停止线验收

运行：

```bash
go test -count=1 ./internal/lottery/application \
  -run 'TestLesson(27|29)'
```

AST/source guard 的精确证据边界是：

- domain/application 没有出现通用 `RuleEngine`、`DecisionEngine`、operator registry、自由 fact bag、expression/script/DSL；
- `cmd`、runtime config、HTTP server/adapter、Compose/Docker 和 Web production source 没有通过五个受禁 evaluator 标识符做直接装配；
- Participation 没有反向依赖 Lottery；
- 现有被扫描 production source 没有通过上述标识符把 Lottery graph evaluation 接入 ephemeral selection 路径。

该 guard 不扫描 Migration、grant、Redis/RabbitMQ/PostgreSQL，也不证明反射或其他间接 wiring；这些零变化必须由章节 diff、全仓回归和人工边界审查补证。

未来第 30 节若按批准范围引入发布/Activity 绑定，应先更新规范与测试，再最小调整停止线。不能通过删除 guard 或改名绕过。

## 10. 错误分类总表

| 对外稳定类别 | 常见来源 | 值班含义 | 禁止动作 |
| --- | --- | --- | --- |
| invalid argument | nil context、zero subject、非法 exact identity | 上游调用契约错误 | 猜测默认 subject/revision |
| not configured | nil/typed-nil port、非正 freshness/duration、非法 step budget | composition/config 错误 | 临时放宽预算或跳过依赖 |
| graph not found | exact `(GraphID, Revision)` 不存在 | 指定 revision 不可用 | 自动尝试 latest/上一版 |
| graph invalid | wrong identity、损坏聚合、unsupported schema/operator/branch | 配置不可执行 | 走 default edge 或继续读取 fact |
| step budget exceeded | graph worst depth 大于服务预算或防御性 actual hard stop | 当前 service 不接纳该 graph | 由调用者动态扩大预算 |
| membership fact not found/stale/invalid | 权威事实缺失、过期、未来、主体不符或损坏 | 没有可信路由事实 | 当作 standard/default |
| membership fact unavailable/read failure | provider-owned timeout 或依赖错误 | 事实来源技术失败 | 冒充 caller/internal timeout |
| timed out | caller 仍 live，但内部 `maxDuration` 先到 | 本服务合作式预算耗尽 | 用 `errors.Is(context.DeadlineExceeded)` 对外冒充 caller |
| caller cancellation/deadline | 原始 caller context 已取消/超时 | 上游拥有终止原因 | 重试或降级到 default |
| decision invalid / failure | 内部不变量或未分类故障 | fail closed，需要受控诊断 | 返回 partial path/target |

所有错误都伴随 zero decision。若观测到“error + 非零 Target/Path”，应视为契约回归并停止发布。

## 11. caller、internal 与 provider 优先级分诊

依赖返回后按固定次序检查：

```text
caller error
  > internal evaluation deadline
  > provider/dependency error
  > returned value validation
```

### 场景 A：返回原始 `context.Canceled`

检查原始 caller context 是否已取消。若是，直接归上游所有，不应包装成会员 provider unavailable 或内部 timeout。

### 场景 B：返回原始 `context.DeadlineExceeded`

只有 caller 自己先到 deadline 时才应原样返回。检查 caller 的绝对 deadline 是否早于或等于服务内部 deadline。

### 场景 C：返回稳定 evaluation timed-out

caller 必须仍然 live，child cause 应是私有 internal-deadline cause。普通 `errors.Is` 不应匹配 `context.DeadlineExceeded`；可信诊断代码可以显式读取 `Cause()`，但不能把 cause 直接写入用户响应。

### 场景 D：provider 返回包装的 `context.DeadlineExceeded`

若 caller 与 evaluation child 都仍 live，这是 provider-owned failure。graph reader 当前映射到 evaluation failure；membership reader 按既有事实错误映射到 unavailable。不要仅因 cause 文本含 `deadline` 就重分类。

## 12. Graph 故障分诊

按以下顺序检查：

1. 请求是否提供合法且完整的 exact identity；
2. Repository 是否恰好读取一次该 identity；
3. error 与 graph 同时返回时是否丢弃 graph；
4. nil error 时返回 identity 是否与请求完全相等；
5. aggregate `Validate()` 是否成功；
6. `graph.Depth()` 是否小于等于 service step budget；
7. 在上述任一步失败时，Clock 和 fact reader 是否保持零调用；
8. 是否有人尝试 fallback 到另一 revision 或使用 `edges[0]`。

低披露公共文本不得包含 GraphID、revision、NodeID、完整 topology、SQL、DSN 或表内容。Repository 的原始错误可以通过受控 `Cause()` 保留给可信诊断，但不能借 `Unwrap()` 改变公共 `errors.Is` 语义。

## 13. Membership fact 故障分诊

确认本次调用只发生：

- 一次 `Clock.Now()`；
- 一次 `FindMembershipTierFact(subjectRef)`；
- 全 path 复用同一个 snapshot。

依次核对：

1. fact 自身 typed validation；
2. `fact.SubjectRef() == requested subjectRef`；
3. `ObservedAt <= evaluatedAt`；
4. `evaluatedAt - ObservedAt <= maxFactAge`；
5. tier 只属于封闭 `standard/premium`；
6. source/revision 是合法最小 provenance token。

未知 tier、not-found、stale、future、provider error 都不能解释成 `baseline_default`。Default 是 standard 已被可信确认后的显式业务边，不是异常兜底。

## 14. Path 与 decision 异常

成功 decision 应满足：

- identity、schema version 与 graph 一致；
- root、terminal、target 非零且连续；
- path 至少一条且不超过本次 step budget；
- 每个 step 的 `FromNodeID` 等于上一节点；
- rule code 是批准的 concrete membership rule；
- branch 与 reason 精确配对；
- path 不重复 node；
- 最后 `ToNodeID` 等于 terminal；
- `Path()` 返回防御性副本；
- fact source/revision 与 evaluated-at 完整。

本节 decision 不包含 subject、原始 tier payload、loaded Strategy、Award、随机值或 Draw。缺少这些字段不是数据丢失，而是本节最小披露边界。

## 15. 低披露日志与未来指标

当前未装配，因此没有新增生产日志、metric 或 trace。未来装配前至少要评审：

| 可作为低基数维度候选 | 不得作为普通 label |
| --- | --- |
| success / caller_cancel / internal_timeout / graph_invalid / fact_unavailable | GraphID、revision、NodeID、StrategyID、subject ref、fact revision |
| fixed operator family | raw tier payload、source endpoint、SQL、完整 error cause |
| bounded path-length bucket | 完整 path、拓扑或会员派生 branch（未经披露评审） |

高基数和会员派生证据只可进入受控、分级访问、明确留存期的诊断通道。第 31～35 节还没有实现前，不能把内部 path 暴露给浏览器或普通操作者。

## 16. 发布、回滚与停止条件

### 16.1 当前章节

当前没有运行时发布动作。代码回滚只意味着未来调用者不再编译/调用这个未装配 service；第 28 节已存在的 Migration 与 graph 数据不能被删除或回写。

### 16.2 未来首次装配前的强制停止条件

出现任一情况都不得装配：

- graph 不是 exact immutable revision；
- Activity 尚未拥有明确发布绑定；
- 真实 membership adapter 不能保证 subject、freshness、source/revision；
- caller/internal/provider 错误会被统一成 default；
- step/time budget 可以由浏览器任意扩大；
- 最终成功 checkpoint 已观察到 context 取消，却仍返回 success decision；最终检查之后并发发生的取消不能撤回已经返回的内存值；
- error 会携带 partial path/target；
- graph/path 将被公开但尚无认证、对象权限和披露模型；
- 计划把 evaluation success 当作 authorization allow 或 Participation eligible；
- 没有明确 rollback 后 Activity 应引用哪个已发布 revision。

### 16.3 未来回滚原则

未来第 30 节应优先通过发布绑定回退到一份已验收 immutable revision，而不是原地修改 graph。代码回滚和配置回滚是两个独立动作；任何一方回滚后都要重新验证 schema/operator compatibility。不要用 fallback-to-latest 掩盖不兼容。

## 17. 安全事件判定

以下情况应按安全/数据披露事件升级，而不是普通功能缺陷：

- 未认证或无对象权限的调用者得到 graph/path/会员分支；
- 错误文本包含 subject、GraphID/revision、SQL、provider endpoint 或 raw fact；
- unknown/provider failure 被当成 baseline 成功；
- 调用者可以指定任意大 budget 造成资源放大；
- authorization deny 被改写为 graph/fact invalid；
- decision path 被当作不可篡改审计，而实际上尚未持久化或签名；
- 普通用户能够读取可信诊断 `Cause()`。

第 29 节的架构测试只能证明指定 production source 中没有五个 evaluator 标识符的直接装配；它不证明所有间接 wiring，更不能替代第 31～35 节的威胁模型、真实会话、服务端 RBAC、前端投影与浏览器越权验收。

## 18. 清理纪律

本节验证可能创建的临时产物只有测试/构建输出：

- `web/dist`：先列出本次构建文件，再逐个删除并移除空目录；
- 自建 coverage 临时目录：使用任务专用 `mktemp -d`，记录精确文件后删除；
- disposable MySQL acceptance：由脚本根据随机 project/resource label 清理并复核零残留。

不得清理：

- `web/node_modules` 或 pnpm store；
- Go build/module cache；
- 长期 `growthos` Compose 容器、network、volume 或 Secret；
- 第 28 节 Migration、graph 表或用户已有 graph 数据；
- 用户未提交工作树变化。

## 19. 当前证据与诚实结论

第 29 节应在 QA 最终冻结时记录实际命令、时间、通过数量、fuzz 执行次数、coverage、MySQL 回归、远端 tip 和清理结果。本手册只定义口径，不提前伪造最终 SHA。

可以得出的结论：

> 一个 exact、已验证的 Lottery Strategy routing graph 可以在单次可信会员事实和单次业务时刻下，被封闭 typed evaluator 以 1～16 step 预算、合作式总时长预算、caller/internal/provider 固定错误优先级和 zero-decision 协议确定性求值，并返回防御性复制的完整 path 证据。

不能得出的结论：

- 该 graph 已发布或绑定 Activity；
- `growth-api` 正在调用它；
- 已接入真实会员系统；
- 已有规则管理 UI、认证、RBAC 或对象级授权；
- 已形成正式 Draw/Result、库存扣减或发奖；
- 单元测试 timeout、coverage、fuzz 或本机运行结果等于生产 P99/SLO；
- 当前只有一个 concrete operator 却已经建成通用规则引擎。
