# 第 25 节 QA：权威注册事实与新用户资格切片验收

- **需求基线：** [新用户资格规则基线 v1](../../product/new-user-eligibility-v1.md)
- **架构决策：** [ADR-0021](../../decisions/ADR-0021-participation-new-user-eligibility.md)
- **上游边界：** [Lottery 业务规则需求基线 v1](../../product/lottery-rule-requirements-v1.md)
- **上一节：** [第 24 节 QA](lesson-24.md)
- **设计推导：** [第 25 节设计手记](../../design-thinking/lessons/lesson-25.md)
- **基准提交：** `35f94b98c99dd4a8b6540388325b74af472e4ad1`（第 24 节已验收 tip）
- **实现证据提交：** `475804b`（domain、application 与架构停止线）；`c96b393`（手工 zero/partial service 构造防御加固）
- **验收日期：** 2026-08-30，Asia/Shanghai
- **当前记录状态：** domain/application 定向测试、10 秒 fuzz、Participation 与全仓 race、最终 `make verify`、文档/负向差异均已实际通过；本节没有 adapter、HTTP/UI 或 runtime/schema 变化，因此未执行也不声称 Compose/浏览器资格 E2E

> 本节验收的是一个可执行但尚未对外组合的 Participation 资格切片。它能从权威注册事实形成确定的 `eligible` / `ineligible` 决定，并把事实未知、过期、不可用、损坏和调用取消保留为“无法形成决定”。它不验收“真实用户已经不能绕过抽奖”：当前还没有真实会话、事实 adapter、Activity、正式 Participation 或受资格门控的 Lottery API。

## 1. 验收范围

### 1.1 本轮必须成立

- `ParticipantRef` 只是 Participation 查询外部主体事实的不透明引用，不被命名或解释为 Principal、登录证明、租户或权限；
- `RegistrationFactSnapshot` 只保存本条规则需要的最小快照：participant reference、registered-at、observed-at、source 与 source revision；
- 浏览器或调用方不能向 production service 传入 `is_new_user`、最终 verdict、客户端 registered-at 或客户端 evaluated-at；
- `NewUserPolicy` 明确携带 policy revision 与含边界注册时间下界；
- `registered_at < cutoff` 为确定 `ineligible`，`registered_at >= cutoff` 为确定 `eligible`；
- 同一 instant 的不同时区表示、相同输入的重复执行都产生相同决定；
- application service 先读取事实，再由受控 server clock 捕获一次 evaluated-at；
- fact age 等于最大允许值仍有效，超过 1ns 才是 stale；
- fact not found、stale、provider unavailable、unknown provider failure、future fact、participant mismatch、invalid clock 与 cancellation 都返回零 decision；
- 业务拒绝与技术无法决定不能共享同一个 outcome；
- provider read 的 `RegistrationFactReadError` 普通字符串只渲染稳定语义 class，不泄露受信诊断 cause；其他参数/领域校验错误只包含项目定义的安全校验详情；
- domain 不依赖仓库其他项目包，application 只依赖 Participation domain；
- 第 25 节不提前创建 `Rule`、`RuleChain`、`RuleEngine`、`Specification` 或通用 `EvaluationContext`；
- 相对第 24 节，Lottery、HTTP、composition root、Migration、Redis、Compose、配置与 Web 运行时保持零变化。

### 1.2 本轮明确不验收

- 外部用户目录、IAM、Account 服务或本地数据库的事实 adapter；
- `ParticipantRef` 与已认证调用者之间的可信绑定；
- Activity scope、活动发布版本或 cutoff 的配置来源；
- HTTP route、status、error code、公开 DTO、浏览器交互或导航裁剪；
- MySQL fact projection、Migration、runtime grant、Redis cache 或 Compose fixture；
- 两条规则的顺序、短路、责任链、规则树、DSL 或通用决策引擎；
- Participation request、次数扣减、资格预占、正式 Draw/Result 或幂等；
- 资格检查与最终消费之间的 TOCTOU 关闭；
- 生产用户目录延迟、容量、可用性、隐私保留或 SLO；
- 服务端 RBAC、页面权限或浏览器越权 E2E。

## 2. 证据边界：已证、未证与禁止推导

| 命题 | 当前证据 | 当前结论 | 不能由该证据推出 |
| --- | --- | --- | --- |
| 注册 cutoff 规则可确定执行 | domain table tests、时区与重复执行测试 | 已证于纯领域输入 | cutoff 已绑定真实活动 |
| 事实新鲜度在业务决定前检查 | application freshness 边界测试 | 已证 | provider 自身一定正确标记 observed-at |
| 技术失败不会伪装成 ineligible | error/failure table tests | 已证于当前 service | 未来 HTTP mapping 一定正确 |
| caller cancellation 优先 | pre-cancel、read 后 cancel、clock 后 cancel、blocking reader 测试 | 部分已证 | 不遵守 context 的 provider 能被本服务强制中断 |
| read-only service 可并发调用 | 64 worker + race | 已证于线程安全 test doubles | 任意未来 adapter 都并发安全 |
| domain/application 依赖停止线 | AST architecture test + targeted negative diff | 已证于当前 production Go 文件 | 所有未来重命名后的万能抽象都会自动被发现 |
| Lottery/API/Web 零漂移 | 基准到实现提交的 path-scoped negative diff | 已证 | 抽奖入口已经执行资格检查 |
| 全仓 Go 行为无现有测试回归 | `go test -count=1 ./...` | 已证于当前 Go 测试集 | 前端、Compose、真实 MySQL/Redis 或浏览器无回归 |
| 文档链接与结构 | `make doc-check` | 本文落盘后已实际通过 | 文档语义与代码必然一致 |
| 完整仓库质量门禁 | 最终 `make verify` + 全仓 `go test -race -count=1 ./...` | 已证于当前源码/文档和现有测试集 | 不能推出真实用户目录、Compose 资格链或生产 SLO 已验收 |

最重要的禁止推导是：**“资格 evaluator 存在”不等于“现有 Lottery API 已受资格保护”。** 当前 `cmd/growth-api` 没有装配 Participation，`internal/participation` 也没有 adapter，浏览器页面没有主体或资格状态。

## 3. 按提交核查线性演进

从第 24 节 tip 顺序检查：

| 顺序 | 提交 | 验收焦点 |
| --- | --- | --- |
| 1 | `ea2cacd` | 固化事实所有权、公开入口停止线、失败语义和备选方案 |
| 2 | `b59bc1e` | 明确 freshness 边界和低/高基数观测字段 |
| 3 | `959c32a` | 建立 Registration fact、concrete policy、decision 与纯 evaluator |
| 4 | `b718267` | 建立 consumer-owned reader、controlled clock、freshness/cancellation/error service |
| 5 | `475804b` | 以 AST 测试守住无通用 Rule、无跨上下文依赖的第 25 节停止线 |

可复跑历史检查：

```bash
git log --reverse --oneline \
  35f94b98c99dd4a8b6540388325b74af472e4ad1..475804b

git diff --stat \
  35f94b98c99dd4a8b6540388325b74af472e4ad1..475804b
```

通过条件不是“有五个提交”，而是演进顺序可解释：先固化事实与停止线，再建立领域值与规则，再建立应用协作，最后用负向架构测试防止当前只有一条规则时提前抽象。

## 4. 领域对象验收

### 4.1 `RegistrationFactSnapshot`

构造器必须证明：

- `ParticipantRef > 0`；
- registered-at 与 observed-at 都非零；
- `registered_at <= observed_at`；
- 时间进入对象时规范为 UTC，并去除不应参与相等语义的附带单调时钟信息；
- source 非空、无首尾空白、UTF-8 有效、全部字符可打印且最多 128 bytes；
- revision 使用相同规范并最多 256 bytes；
- 构造失败返回零值，不形成半合法快照；
- `Validate` 能拒绝 adapter 绕过构造器返回的零值或损坏值。

当前 snapshot 没有 `is_new_user` 字段。source/revision 是内部追溯 token，不是浏览器文案，也不得编码手机号、邮箱、原始 payload 或数据库错误。

### 4.2 `NewUserPolicy`

构造器必须证明：

- policy revision 非空、规范、可打印且最多 256 bytes；
- cutoff 非零并规范为 UTC instant；
- policy code 固定为 `participation.new_user.registered_on_or_after`；
- cutoff 是含边界下界，不是“最近 N 天”或首单时间；
- policy 没有读取系统时钟、Activity 表或全局配置。

policy 仍是 application 的每次调用参数。这是诚实边界：第 25 节还不能说明某个 cutoff 属于哪场 Activity。

### 4.3 `NewUserEligibilityDecision`

合法决定只可能来自 evaluator，并包含：

- `eligible` 或 `ineligible`；
- stable rule code；
- stable reason code；
- policy revision；
- fact source/revision；
- canonical evaluated-at。

决定不携带 ParticipantRef、registered-at、cutoff、昵称或完整用户属性。零 decision 不是第三种业务 outcome，而是与 error 一起表示没有形成可信决定。

## 5. cutoff、时区与确定性测试矩阵

| 输入 | 预期 outcome | 预期 reason | 结果 |
| --- | --- | --- | --- |
| cutoff 前 1ns | `ineligible` | `registration_before_cutoff` | 通过 |
| 恰好 cutoff | `eligible` | `registration_on_or_after_cutoff` | 通过 |
| cutoff 后 1ns | `eligible` | `registration_on_or_after_cutoff` | 通过 |
| UTC 与 UTC+8 表示同一组 instant | 完整 decision 相等 | 相同 | 通过 |
| evaluated-at 使用 UTC 与 UTC-4 表示同一 instant | 完整 decision 相等 | 相同 | 通过 |
| 相同 policy/fact/evaluated-at 重复执行 | 完整 decision 相等 | 相同 | 通过 |
| zero policy/fact/evaluated-at | 零 decision | error | 通过 |
| registered-at 或 observed-at 晚于 evaluated-at | 零 decision | future/invalid error | 通过 |

除普通 `go test` 会执行的 4 个 seed 外，本轮还实际运行：

```bash
go test ./internal/participation/domain -run='^$' \
  -fuzz='^FuzzEvaluateNewUserEligibilityCutoffBoundary$' -fuzztime=10s
```

结果为 `PASS`：12 workers 在 10 秒 fuzz 窗口执行 5,289,510 次，`new interesting=0`，未生成 crash corpus。它只证明当次有界随机探索没有发现反例，不证明所有 `int64` 时间组合都已穷举，也不能替代 table/边界测试。

## 6. application 调用顺序验收

实现顺序固定为：

```text
validate request and concrete policy
  -> validate service dependencies
  -> honor pre-cancellation
  -> FindRegistrationFact(ctx, participantRef)
  -> let observed caller cancellation win
  -> capture Clock.Now exactly once
  -> let observed caller cancellation win
  -> validate fact shape, subject, future time and freshness
  -> EvaluateNewUserEligibility
  -> let observed caller cancellation win
  -> return confirmed decision
```

对应断言：

- nil context、zero participant、invalid policy 不调用 reader/clock；
- nil/typed-nil reader、nil/typed-nil clock、non-positive max age 拒绝构造；
- 手工构造的 zero/partial service 或 nil service 在 `Validate` / `Evaluate` 处失败关闭；
- fact read error 后不调用 clock；
- 成功 fact read 后 clock 只调用一次；
- participant mismatch、future fact、stale fact 都发生在 business evaluator 前；
- evaluator 返回后若 caller cancellation 已可观察，context error 仍优先。

## 7. freshness 与时间预算验收

freshness 计算为：

```text
fact_age = evaluated_at - observed_at
valid    = fact_age <= max_fact_age
stale    = fact_age >  max_fact_age
```

| 边界 | 预期 | 结果 |
| --- | --- | --- |
| fact age 恰好等于 max age | 可进入业务 evaluator | 通过 |
| fact age 比 max age 多 1ns | 零 decision + stale error | 通过 |
| observed-at 晚于 evaluated-at | invalid fact，不以负 age 绕过 | 通过 |
| registered-at 晚于 evaluated-at | invalid fact | 通过 |
| clock 返回 zero time | clock invalid，零 decision | 通过 |

当前 service 不自己创建 timeout，也没有为 `maxFactAge` 设置上限。它把 caller context 传给 reader；如果 adapter 忽略 context 并永久阻塞，service 不能越过同步 Go 调用强制终止它。现有 blocking-reader 测试是在 reader 被释放后确认 cancellation 获胜，不能被描述为“任意坏 provider 都可及时取消”。

## 8. 业务决定与失败分类矩阵

| 场景 | decision | error class | 是否是业务拒绝 |
| --- | --- | --- | ---: |
| registration >= cutoff | `eligible` | nil | 否 |
| registration < cutoff | `ineligible` | nil | 是 |
| provider not found | zero | `ErrRegistrationFactNotFound` | 否 |
| provider unavailable | zero | `ErrRegistrationFactUnavailable` | 否 |
| provider raw deadline 且 caller 仍活跃 | zero | unavailable，保留 trusted deadline cause | 否 |
| provider unknown error | zero | `ErrRegistrationFactReadFailure` | 否 |
| fact stale | zero | `ErrRegistrationFactStale` | 否 |
| fact corrupt / wrong subject / future | zero | `ErrRegistrationFactInvalid` | 否 |
| clock zero | zero | `ErrEligibilityClockInvalid` | 否 |
| caller canceled/deadline | zero | 原始 context error | 否 |

`RegistrationFactReadError.Error()` 只返回审核过的 class 文本；测试用带 `secret` 的 cause 证明普通字符串不包含该内容，同时 `errors.Is` 仍能让可信诊断代码看到 class 与 cause。

### 8.1 已知的多 class 风险

当前 wrapper 同时实现 `Is` 并通过 `Unwrap` 暴露 cause。若未来 adapter 错误地把另一个 application sentinel 包进 cause，或形成 class A + cause class B 的嵌套链，`errors.Is` 可能对多个语义 class 同时为真；`classifyRegistrationFactReadError` 的检查顺序还可能把这种歧义归到先匹配的 class。

本轮测试证明了单 class + opaque cause 的路径，**没有证明恶意或违规 adapter 的错误链一定互斥**。在真实 adapter 落地前应二选一并补 negative tests：

1. 规定 diagnostic cause 绝不能包含 application semantic sentinel，并在 adapter contract test 中强制；或
2. 不以标准 `Unwrap` 暴露 cause，改用受信 `Cause()` / structured diagnostic channel，使公开分类保持单值。

在这项风险关闭前，HTTP adapter 不应按“任一 `errors.Is` 命中”随意决定可重试状态。

## 9. 取消与并发证据

### 9.1 已覆盖取消点

- pre-canceled context：reader/clock 均为 0 次；
- reader 返回同时触发取消：caller cancellation 覆盖 dependency error，clock 0 次；
- clock 返回同时触发取消：caller cancellation 覆盖后续决定；
- blocking reader 释放后返回错误：已取消 caller 得到 `context.Canceled`；
- provider 自身返回 `context.DeadlineExceeded`、但 caller context 尚未过期：分类为 provider unavailable，不伪装成 caller deadline。

### 9.2 已覆盖并发点

64 个 goroutine 对同一个 immutable service 与 policy 并发求值：

- 每个调用独立读取一次事实；
- 每个调用产生相同 confirmed decision；
- race detector 通过；
- service 本身没有 mutable request state。

该证据的前提是注入的 reader/clock 并发安全。`NewUserEligibilityService` 不拥有、关闭或串行化 adapter 资源，也不进行 singleflight/cache；未来共享 client/pool 的生命周期属于 composition root 与具体 adapter。

## 10. 架构停止线验收

`architecture_test.go` 解析 Participation domain/application 的 production Go AST，并检查：

- domain 不导入任何仓库内项目包；
- application 只允许导入 `internal/participation/domain`；
- production type declarations 不出现 `Rule`、`RuleChain`、`RuleEngine`、`Specification`、`EvaluationContext`；
- 两个 package 都确实存在 production Go 文件，避免空目录让检查假通过。

这条测试能守住本节已知的具体越界名称和 import 方向，但不能发现所有语义上的万能抽象。例如换一个未列名的 type，或把通用协议藏进函数/map，仍需要 code review 与下一节真实消费者证据。

## 11. 运行时 negative diff

实际执行：

```bash
git diff --name-only \
  35f94b98c99dd4a8b6540388325b74af472e4ad1..475804b \
  -- cmd deploy migrations web internal/lottery \
     internal/infrastructure internal/platform configs go.mod go.sum
```

结果：**无输出。** 另外实际检查 `internal/participation/adapter` 不存在，`migrations/000003_participation_registration_facts.up.sql` 不存在。

这组反证意味着：

- 没有 composition root 装配 Participation；
- 没有新增 route/header/status/DTO；
- 没有修改现有 ephemeral selection；
- 没有本地 registration fact 表或 MySQL grant；
- 没有把 fact/decision 塞进 Strategy Redis projection；
- 没有前端用户 ID、资格 badge、隐藏按钮或角色 UI；
- 没有 Compose user fixture 或伪造 E2E。

它证明了“运行时零漂移”，也同时证明当前不是完整端到端资格闭环。

## 12. 可复跑命令矩阵

### 12.1 已实际执行并通过

```bash
go test -count=1 \
  ./internal/participation/domain \
  ./internal/participation/application

go test -race -count=1 ./internal/participation/...

go test ./internal/participation/domain -run='^$' \
  -fuzz='^FuzzEvaluateNewUserEligibilityCutoffBoundary$' -fuzztime=10s

go test -count=1 ./...

make doc-check
```

| 命令 | 本次结果 | 失败数 | 主要证明 |
| --- | --- | ---: | --- |
| domain + application 定向测试 | exit 0 | 0 package | cutoff、fact/policy 构造、freshness、failure、取消、依赖与架构停止线 |
| Participation race | exit 0 | 0 package | 当前并发 test doubles 和 service 无已检测 data race |
| 10 秒 domain fuzz | exit 0；5,289,510 execs，0 new interesting | 0 crash | cutoff 两侧有界随机探索未发现反例；不是穷举证明 |
| 全仓 Go 测试 | exit 0 | 0 package | 当前 Go 测试集无回归 |
| `make doc-check` | exit 0，`documentation checks passed` | 0 | Markdown 链接、ADR 登记、课程注册结构和文档治理检查 |

### 12.2 最终仓库门禁

```bash
make verify
go test -race -count=1 ./...
git diff --check
```

三条命令均已 exit 0。`make verify` 实际完成 `go vet`、全仓 Go test、doc-check、19 个 Vitest 文件/152 个前端测试、TypeScript typecheck 和 Vite production build；全仓 race 覆盖 22 个 Go package 并全部通过。它们证明当前仓库已有测试没有回归，不证明第 25 节存在尚未实现的 fact adapter、HTTP、Compose 或浏览器资格路径。由于本节没有 runtime/schema/transport 变化，不为“看起来完整”新增虚假 Docker E2E；未来真实 adapter 或公开消费者出现时必须另补 contract/integration/Compose/browser 证据。

## 13. 精确负向断言

以下任一情况出现，本节当前实现都应判为不通过：

- `registered_at == cutoff` 被拒绝；
- stale fact 先形成 `ineligible` 再返回 warning；
- not found 自动解释为“刚注册所以是新用户”；
- provider timeout 自动 fail-open 为 eligible；
- caller 提交 `is_new_user=true` 可绕过 fact reader；
- ParticipantRef 被文档或代码称为认证用户 ID；
- decision 包含手机号、邮箱、完整 source payload 或 raw provider error；
- revision/source 直接成为普通 metrics label；
- clock 在一次成功求值中读取多次；
- reader error 后仍调用 clock/evaluator；
- domain 导入 Lottery、MySQL、Redis、Gin 或 platform fault；
- 第一个具体规则就引入 priority/DSL/generic context；
- 新建本地用户事实表但没有同步、纠正、删除和隐私协议；
- 在无真实会话时用 demo header 声称已完成身份绑定；
- 把 `ineligible`、权限不足、`no_reward` 和系统不可用映射成同一个状态。

## 14. 清理与环境影响

本轮没有启动 Docker，也没有创建 Compose project、容器、volume、network、builder、临时 Secret、下载图片、浏览器 trace、coverage 文件、fuzz crash corpus 或任务专用临时目录。

冻结前已先确认 `web/dist` 原本不存在；两次 `make verify` 的 Vite build 会生成该目录。最终门禁完成后，逐项解析其中 6 个静态文件并使用精确绝对路径删除整个任务生成目录，再次确认路径不存在。Go fuzz/module/build/test cache 是共享且可复用的开发缓存，不是只服务本任务的交付产物，予以保留；源代码、文档、依赖和已有开发数据均未清理。

## 15. 剩余风险与下一责任

| 风险 | 当前影响 | 下一触发器 / 责任 |
| --- | --- | --- |
| 无真实 fact adapter | service 无 production 调用者 | 明确权威目录协议后新增 adapter 章节/提交 |
| 无可信主体绑定 | 不能安全接公开 Lottery route | 真实 session 与业务编排出现后组合 |
| policy 每次传入但无 Activity owner | cutoff provenance 尚未闭合 | Activity 与规则发布模型形成时绑定 revision |
| max fact age 仅要求正数、无上限 | 错配过大值会让陈旧事实长期有效 | composition/config 落地时定义上限与环境校验 |
| provider 可忽略 context | 请求可能卡在同步 read | adapter timeout/transport contract 与 fault test |
| error cause 可能导致多 class | 未来 HTTP mapping 存在歧义 | adapter 前关闭单 class invariant |
| 无决定持久化 | 无法回放正式参与依据 | 正式 Participation/Draw 设计时决定 snapshot/audit |
| 每次 read 后再决定 | 事实可能在决定后变化 | 正式消费时定义事务、lease、revision 或重检 |
| architecture test 是名称 allow/deny | 可被换名规避 | code review + 第二规则反推最小抽象 |

## 16. 阶段性验收结论

在当前证据范围内，可以准确地说：

> GrowthOS-Go 已建立首个可执行 Participation 新用户资格切片：定义面向外部事实所有者的 consumer-owned port 契约；给定受控最小注册快照时，具体、带版本的含边界 policy 由纯 evaluator 求值；application service 以一次 server clock 控制 future/freshness，区分确定业务拒绝与事实未知、过期、不可用、损坏及取消；领域与应用的依赖方向、并发，以及对跨上下文 import 和五个已知过早规则抽象名称的停止线都有自动化证据。真实 provider 权威性仍等待 adapter 章节验证。

仍然不能说：

- 现有 Lottery API 已受资格门控；
- 系统已经知道请求者是谁；
- 已连接真实用户目录或持久化注册事实；
- 已完成 Activity 级 policy 发布；
- 已实现责任链、规则树或决策引擎；
- 已完成浏览器、Compose 或越权 E2E；
- 已解决正式参与的幂等、额度、TOCTOU 或审计；

因此本文是对当前 domain/application slice 的真实 QA 记录；课程正文、API、QA、设计手记、面试问答和全局索引已经在同一分支完成，最终质量门禁已通过，第 25 节可以冻结。这个完成状态只属于本文明确的内部能力边界，不会把未来 adapter、在线资格门控或权限系统提前写成已交付。
