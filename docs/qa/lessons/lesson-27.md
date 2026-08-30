# 第 27 节 QA：会员等级多出口路由与责任链边界验收

- **需求基线：** [Lottery 会员等级 Strategy 路由基线 v1](../../product/membership-strategy-routing-v1.md)
- **架构决策：** [ADR-0023](../../decisions/ADR-0023-membership-strategy-routing-boundary.md)
- **上游规则边界：** [Lottery 业务规则需求基线 v1](../../product/lottery-rule-requirements-v1.md)
- **上一节：** [第 26 节 QA](lesson-26.md)
- **API 记录：** [第 27 节 API 记录](../../api/lessons/lesson-27.md)
- **基准提交：** `47fc94d`（第 26 节已验收 tip）
- **已知实现与加固序列：** `2d7728a`、`57a3216`、`076e399`、`b307f1a`、`42caed9`、`2dc49b1`、`8ebc94a`、`544f4af`、`d57b963`、`5460bde`、`331c8c7`、`6db36dd`、`59499fb`、`a04d89d`、`9ead6e1`、`aaa4a8f`
- **验收日期：** 2026-08-30，Asia/Shanghai
- **当前记录状态：** domain/application/architecture、课程、API、QA、设计与面试材料已经形成。根代理在证据候选 `a04d89d` 对应内容上实际复跑 domain/application、Lottery 全量 race、20 轮 shuffle、10 秒 fuzz、atomic coverage、全仓 race、`make verify`、doccheck、章节 diff-check 与 runtime negative diff，结果均通过；随后在包含 QA 与全局索引的干净提交 `aaa4a8f` 上再次完成 clean-worktree `make verify`、全仓 race、章节 diff-check、runtime negative diff、线性历史和清理复核。本记录随最后一笔冻结证据提交推送，以同名远端实际 SHA 为 accepted tip；push 后的远端一致性与实际 tip 文档/差异/清洁复核由根代理在最终交接中报告，本文不预写由 Git 尚未生成的 SHA

> 本节验收的是 Lottery 内部一个具体、代码拥有的会员等级到 Strategy ID 的路由切片。它证明 `premium` 和 `standard` 需要不同出口、显式基线分支与可审查 path，因而不能继续伪装成 Participation 的线性 `continue/reject` chain。它没有真实会员 adapter、公开路由、Activity 绑定、Strategy 加载、随机选择、正式 Draw、认证或授权。

## 1. 验收范围

### 1.1 本节必须成立

- 外部会员 authority 拥有会员等级生命周期，Lottery 只消费最小不可变事实；
- `MembershipSubjectRef` 是 Lottery 本地 opaque lookup reference，不是 Principal、登录证明、ParticipantRef、租户或角色；
- v1 事实词汇封闭为 `standard` 与 `premium`；zero、`unknown`、`gold`、`vip` 或损坏映射均不能进入 default；
- `premium` 必须选择 `premium_override` branch 与 premium target；
- confirmed `standard` 必须选择显式 `baseline_default` branch 与 baseline target；
- default 是经批准的 standard 业务边，不是 not-found、timeout、stale、future、corrupt 或 unsupported 的异常 fallback；
- policy revision、fact revision、branch、Strategy ID 与未来 Strategy version 分离；
- policy revision 当前只是受界标识，不存在 registry、内容 hash 或“revision 唯一对应 targets”的强制；确定性必须以完整 policy snapshot、完整 fact snapshot 与同一 as-of 为前提；
- 两个 branch 可以暂时收敛到相同 Strategy ID，但 branch/path 证据不能被目标相同抹除；
- 每次有效 application 调用只读取一次受控 clock、一次事实 reader，并先 clock 后 reader；
- fact age 恰好等于 maximum age 仍有效，超过 1ns 才 stale；observed-at 晚于 evaluated-at 1ns 即 invalid；
- reader 返回 fact 加 error 时 error 获胜，不能使用同时返回的 standard fact 走 default；
- caller cancellation 在可观察点优先于 dependency result；provider 自己的 deadline 在 caller 仍存活时是 unavailable；
- provider 报告无法映射的领域事实错误时，对普通调用方只暴露 application invalid class，raw cause 只通过显式 `Cause()` 取得；
- 技术错误与取消返回零 `MembershipStrategyRouteDecision`、零 target、零 path；
- 成功 path 恰好一跳，包含稳定 rule、selected branch 与 target；`Path()` 返回防御性副本；
- `Confirmed()` 必须同时核对 branch/reason 对、外层 rule/branch/target 与唯一 path step 一致；application 在成功返回前再次断言，违约返回 `ErrMembershipRoutingDecisionInvalid` 与零决定；
- nil、typed-nil、zero/partial service 在任何外部工作前失败关闭；
- 只读 service/policy 能被 64 个 goroutine 共享，当前线程安全 doubles 下 race detector 无报告；
- Lottery domain 不依赖仓库内项目包，Lottery application 只依赖 Lottery domain；Participation 仍只依赖自身 domain；
- 不提前出现通用 Rule/Tree/Engine、generic production type/function、无类型 fact bag、数据库规则图、runtime priority 或 DSL；
- 相对第 26 节，HTTP adapter、composition root、Migration、Redis、Compose、配置、Web 与 Participation runtime 保持零变化。

### 1.2 本节明确不验收

- 真实会员系统 adapter、SDK、凭证、连接池、timeout、retry、熔断、cache 或历史 as-of 查询；
- Principal 到 MembershipSubjectRef 的可信映射、会话、RBAC、ABAC、租户隔离或对象级授权；
- 会员等级的创建、升级、降级、权益解释或外部主数据写模型；
- Activity 发布版本怎样选择 routing policy，以及目标 Strategy 是否存在、已发布或与 Activity 兼容；
- Participation eligibility、次数账户、风险准入与本路由的正式编排；
- Strategy Repository 加载、Redis cache、WeightedSelector、随机票据、Award、Draw、Result、库存或发奖；
- HTTP route、request/response DTO、header、status、error code、retry hint 或用户文案；
- MySQL/PostgreSQL schema、Migration、grant、RabbitMQ message、Redis key 或 readiness dependency；
- path 持久化、审计、OpenTelemetry、指标 exporter、告警或数据保留策略；
- 第 28 节规则树 schema、第 29 节执行引擎或任意动态规则发布；
- Compose/API/browser E2E、越权 E2E、生产容量、SLO、公平性或跨 authority 原子快照。

## 2. 需求到证据映射

| 需求 | 代码/测试证据 | 当前结论 | 不能推出 |
| --- | --- | --- | --- |
| premium 与 standard 产生不同出口 | domain route table + 稳定 branch literal tests | 已由纯领域测试建模 | 目标 Strategy 已存在或可用 |
| standard default 显式且受限 | closed tier constructor、route switch、unsupported fuzz seed | 只有 confirmed standard 可进入 baseline | 新增等级会自动安全兼容 |
| branch 与 target 分离 | target convergence test | 相同 target 仍保留不同 branch | 已有持久化规则图或共享子图 |
| 确定性与 UTC | equivalent-instant/repeat tests | 同 policy/fact/as-of 得到同 route | 多系统形成原子业务快照 |
| revision 不冒充内容 registry | policy constructor/ADR review | 完整 snapshot 确定，不是 revision string 单独确定 | 相同 revision 一定对应相同 targets |
| path 最小且不可改写 | one-hop/path-copy tests | 返回 slice 不能篡改决定 | path 已持久化或可公开披露 |
| confirmed evidence 自洽 | forged path/branch/reason negative tests + application guard | 不一致决定不能成为成功返回 | 当前已有通用执行器或持久化校验 |
| freshness 精确 | application max-age/1ns/future table | 当前 service 使用同一 evaluated-at | adapter 不会给旧事实续鲜 |
| clock/read 调用预算 | call-order 与 clock mutation tests | 单次有效调用为 clock 1、reader 1 | provider 延迟满足生产 SLO |
| fact+error 失败关闭 | application fact+error test | 同时返回的 fact 不可被使用 | 所有第三方 SDK 都遵守该契约 |
| caller cancellation 优先 | pre/clock/reader/blocking-reader tests | 已观察取消时返回 caller error | 忽略 context 的永久阻塞调用可被强制抢占 |
| provider deadline 分类 | live caller + provider deadline test | 归为 unavailable，不冒充 caller deadline | 已决定未来 HTTP 503/retry 语义 |
| corrupt provider payload 安全分类 | domain invalid/ref-required reader tests | public class 为 application invalid，raw cause 隔离 | adapter 映射和告警已实现 |
| nil/typed-nil/partial 失败关闭 | constructor、Validate、manual service table | 不会因 interface typed nil 调用 panic | 任意未来依赖都自动满足此边界 |
| 并发只读 | 64-worker application test + application race | service 无共享 request state | 真实 adapter 并发安全 |
| 边界没有越权 | AST architecture test + scoped negative diff | Lottery/Participation import 方向与停止线受保护 | 换名后的所有语义越界都能被 AST 发现 |
| 公开 API 零变化 | [API 记录](../../api/lessons/lesson-27.md) + adapter negative diff | 没有 membership HTTP surface | 现有 Lottery endpoint 已执行本路由 |

两个最重要的禁止推导是：

1. **“得到 Strategy ID”不等于“完成一次 Lottery 选择”。** 本节不加载 Strategy、不调用 selector，也没有正式结果。
2. **“路由成功”不等于“主体有资格或有权限”。** Membership tier 不是 Participation eligibility，更不是 Governance role。

## 3. 按提交核查渐进演进

| 顺序 | 提交 | 验收焦点 |
| --- | --- | --- |
| 1 | `2d7728a` | 先冻结 premium override、standard default、unknown 失败、一次 as-of 与未做边界 |
| 2 | `57a3216` | ADR-0023 决定事实/路由所有权，拒绝扩 Participation chain、handler if、map-first 与提前 engine |
| 3 | `076e399` | 建立最小会员事实、routing policy、route decision 与一跳 path |
| 4 | `b307f1a` | 将 branch/reason 固定为显式 literal，并使未知 tier 在 evaluator 内也失败关闭 |
| 5 | `42caed9` | 建立 consumer-owned reader、clock、freshness、错误/取消、typed-nil 与并发 application contract |
| 6 | `2dc49b1` | 用 AST 检查保持 Lottery/Participation 所有权和第 28～29 节停止线 |
| 7 | `8ebc94a` | 收紧 `Confirmed()` 的 branch/reason/path 一致性，并在 application 返回前加内部决定防线 |
| 8 | `544f4af` | 明确 revision 尚未绑定内容 registry，确定性只对完整 snapshot 成立 |

可复跑历史：

```bash
git log --reverse --oneline 47fc94d..544f4af
git diff --stat 47fc94d..544f4af
```

判定标准不是“提交够多”，而是先有多出口业务需求，再有 concrete router，最后才有 application 与架构防线。若先建立通用引擎再寻找会员路由，本节不通过。

## 4. 领域事实验收

### 4.1 `MembershipTierFactSnapshot`

构造与 `Validate` 必须保证：

- subject ref 非零；
- tier 只能为 literal `standard` 或 `premium`；
- observed-at 非零，进入对象时转换为 UTC 并去除单调时钟信息；
- source 非空、无首尾空白、UTF-8 有效、字符可打印且最多 128 bytes；
- revision 遵循相同 token 规范且最多 256 bytes；
- 构造失败返回零 snapshot；手工绕过构造器的 zero/unsupported 值仍被 `Validate` 拒绝；
- snapshot 不含姓名、手机号、邮箱、消费金额、成长值、角色、凭证、完整会员 payload 或 Strategy target。

现有 fuzz target `FuzzMembershipTierFactSnapshotNeverAcceptsUnsupportedTier` 的不变量是：只有精确 `standard` 和 `premium` 可以成功；任意其他字符串不能形成合法 fact。普通测试只运行 seed；第 14 节另有实际 10 秒 fuzz 窗口，但它仍是有限探索，不是对所有字符串的穷举证明。

### 4.2 为什么 unknown 不能走 default

unknown 可能来自 provider 新增未协商枚举、坏 payload、映射缺陷或无法确认状态。把它当 standard 会把依赖/兼容错误静默变成业务 Route。因此：

```text
confirmed standard -> baseline_default
confirmed premium  -> premium_override
unknown/corrupt     -> zero decision + error
```

未来若产品需要 guest/trial，必须新增明确业务枚举与 policy revision，而不是扩大 default 的异常容错含义。

## 5. policy、多出口与责任链反例

`MembershipStrategyRoutingPolicy` 当前包含：

- bounded canonical policy revision；
- non-zero premium target；
- non-zero baseline target。

premium 与 baseline target 允许相同。这不是冗余：branch 仍说明“为什么来到这个目标”，并允许灰度期间多个分支收敛。目标存在性、发布态与 Strategy version 未在本节证明。

revision 只校验非空、canonical、可打印与长度。当前没有 policy registry、发布仓储、内容 digest 或唯一约束，所以两个不同 target snapshot 理论上仍可被调用方错误地赋予同一 revision。当前确定性契约是：

```text
same complete policy snapshot
+ same complete fact snapshot
+ same evaluated-at
= same route decision
```

不能缩写成“只要 revision 相同，路由一定相同”。第 28～30 节出现持久化、发布和 Activity 引用后，才有条件建立 revision 到不可变内容的唯一性。

第 26 节 chain 只有：

```text
eligible   -> next fixed gate
ineligible -> terminal stop
error      -> terminal failure
```

本节真实输出为：

```text
premium  -> Strategy 200
standard -> Strategy 100
```

若把 target 塞进 eligibility reason、返回 hidden next-index、在 handler 按 tier 分支或复制两条 chain，都会混淆 Participation 与 Lottery 的决定所有权。当前 concrete router 是可执行反例，但不是第 28 节的树或第 29 节的引擎。

## 6. Route decision 与 path 验收

成功决定必须同时满足：

- `Confirmed() == true`；
- target 非零；
- rule code 为 `lottery.membership_tier.route_strategy`；
- branch 为 `premium_override` 或 `baseline_default`；
- reason 为对应的 `premium_strategy_selected` 或 `baseline_strategy_selected`；
- policy revision、fact source/revision 非空；
- evaluated-at 为 canonical UTC；
- path 恰好一跳，且 step 的 rule/branch/target 与外层决定一致。

path step 不重复存储 evaluated-at；single as-of、policy 与 fact provenance 由同一个 decision envelope 关联。path 不含 subject、原始 tier payload、error、凭证或未走分支。`Path()` 返回新 slice；修改调用方 slice 后再次读取，内部 step 必须保持不变。

`Confirmed()` 不是只检查“target 非零”。它还验证：

- premium branch 只能搭配 premium reason；
- baseline branch 只能搭配 baseline reason；
- path 长度恰好为一；
- path rule/branch/target 必须分别等于 decision envelope 的 rule/branch/target；
- policy/fact provenance 和 evaluated-at 必须存在。

domain negative test 通过包内构造不一致 path target、path branch、path rule 与 branch/reason 配对，逐一证明 `Confirmed()` 为 false。application 在 domain evaluator 返回 nil error 后仍执行 `Confirmed()`；若未来内部改动破坏契约，返回零决定与 `ErrMembershipRoutingDecisionInvalid`。当前 concrete evaluator 总会产生自洽对象，因此没有为了直接注入“坏 domain success”而增加可替换 evaluator 端口。

任何 domain/application error 都必须返回：

```text
Confirmed false
Target    0
Rule      empty
Branch    empty
Reason    empty
Evidence  empty/zero
Path      empty
```

## 7. application 执行顺序

```text
validate ctx, subject and policy
  -> validate service dependencies/max age
  -> honor pre-cancellation
  -> capture Clock.Now once and canonicalize UTC
  -> honor cancellation observed after clock
  -> FindMembershipTierFact once
  -> honor cancellation observed after reader
  -> if fact+error, classify error and discard fact
  -> validate fact shape, subject, future time and freshness
  -> honor cancellation
  -> pure domain routing with the same as-of
  -> honor cancellation observed after evaluator
  -> assert the returned decision is internally confirmed and consistent
  -> return one confirmed route
```

### 7.1 调用计数断言

| 场景 | clock | reader | 决定 |
| --- | ---: | ---: | --- |
| nil ctx / zero subject / invalid policy | 0 | 0 | zero + invalid argument |
| invalid/typed-nil/partial service | 0 | 0 | zero + not configured |
| pre-canceled caller | 0 | 0 | zero + caller error |
| clock callback 取消 | 1 | 0 | zero + caller error |
| zero clock | 1 | 0 | zero + clock invalid |
| reader 返回 fact/error | 1 | 1 | zero + classified error |
| valid standard/premium | 1 | 1 | confirmed route |

clock-mutation test 在 reader 返回时把 clock double 改到 24 小时后；决定仍必须使用首次捕获时刻，证明没有二次 clock read。

## 8. freshness 与时间边界

```text
age   = evaluated_at - observed_at
valid = age <= max_fact_age
stale = age >  max_fact_age
```

| 输入 | 预期 | 当前行为测试 |
| --- | --- | --- |
| observed-at == evaluated-at | 合法 | domain/application success path |
| age == max age | 合法，可路由 | application table |
| age == max age + 1ns | zero + stale | application table |
| observed-at == evaluated-at + 1ns | zero + invalid/future | domain/application table |
| clock 用 UTC+8 表示同一 instant | 规范为 UTC，决定不变 | domain/app evidence test |
| reader 后 clock double 变化 | 不影响本次 as-of | clock-once test |

单一 as-of 不锁住外部会员系统，也不让 reader 获得历史查询能力。adapter 若在读取时把当前本地时间写入 observed-at，会给陈旧事实续鲜并违反契约；当前没有真实 adapter test 能证明这一点。

## 9. 错误分类、fact+error 与显式 `Cause()`

| reader/application 场景 | public application class | 决定 |
| --- | --- | --- |
| classified not found | `ErrMembershipTierFactNotFound` | zero |
| raw/wrapped provider deadline，caller live | `ErrMembershipTierFactUnavailable` | zero |
| classified unavailable | `ErrMembershipTierFactUnavailable` | zero |
| unknown provider error | `ErrMembershipTierFactReadFailure` | zero |
| provider 返回 domain invalid/ref-required | `ErrMembershipTierFactInvalid` | zero |
| returned fact 自身 zero/unsupported/mismatch/future | `ErrMembershipTierFactInvalid` | zero |
| valid fact 超龄 | `ErrMembershipTierFactStale` | zero |
| clock zero | `ErrMembershipRoutingClockInvalid` | zero |
| domain 返回 nil error 但决定证据不自洽 | `ErrMembershipRoutingDecisionInvalid` | zero |

`MembershipTierFactReadError` 的约束：

- `Error()` 只输出审核过的 application class；
- `Is()` 只匹配一个 application class；
- unknown class、zero wrapper、typed-nil wrapper fail closed 到 read failure；
- raw provider/domain error 只能通过 `Cause()` 读取；
- 不实现暴露 cause 的 `Unwrap()`；
- secret endpoint/payload 不出现在普通字符串或 `errors.Is` tree。

坏 payload 测试故意让 reader 同时返回一个可走 baseline 的 standard fact，以及 `domain.ErrMembershipTierFactInvalid` 或 `domain.ErrMembershipSubjectRefRequired`。预期仍是 application invalid + zero decision，证明 raw payload error 不会被 default 掩盖。

`ErrMembershipRoutingDecisionInvalid` 不属于 provider read wrapper；它表示 Lottery 内部 invariant breach，不能归类为会员 authority unavailable，也不能自动重试或对外泄露内部结构。未来 transport 出现时需要单独映射。

## 10. cancellation 与 blocking provider 边界

已建模的取消点：

- pre-cancel：clock/reader 均不调用；
- clock callback 取消：clock 一次，reader 零次；
- reader callback 取消并同时返回 fact/error：caller cancellation 胜出；
- blocking reader：caller 取消后，只有 reader 被测试代码释放返回，service 才能观察 cancellation 并优先返回它；
- provider deadline 而 caller context 仍 live：归为 provider unavailable。

这些测试不能证明 service 可以抢占一个忽略 context 且永不返回的同步 provider。真实 adapter 必须自行配置 transport timeout、传播 context，并接受故障注入。

## 11. nil、typed-nil、并发与 race

配置矩阵覆盖：

- nil/typed-nil reader；
- nil/typed-nil clock pointer；
- typed-nil `MembershipRoutingClockFunc`；
- zero/negative max fact age；
- nil service；
- 手工 zero service；
- 缺 reader、缺 clock、缺 freshness、typed-nil field 的 partial service。

所有路径在 I/O 前返回 not-configured，不 panic。

64-worker test 共享一个 immutable service 与 policy。每个调用获得相同 premium route，thread-safe reader/clock 计数各为 64。application race 已在文档起草前实际通过；最终 Lottery/all-repository race 仍待根代理冻结复跑。该证据不替代生产 adapter 的线程安全合同。

## 12. 架构停止线与负向差异

### 12.1 AST 架构测试

`TestLesson27MembershipRoutingKeepsContextOwnershipAndEngineStopLine` 检查：

- Lottery domain 不 import 仓库内项目 package；
- Lottery application 只允许 import Lottery domain；
- Participation domain/application 继续只依赖自身边界；
- production type 不得声明 `Rule`、`RuleChain`、`RuleTree`、`RuleNode`、`RuleEdge`、`RuleEngine`、`DecisionEngine`、`EvaluationContext`、`RulePriority` 或 `DSL`；
- 递归扫描目标目录，不声明 generic production type/function；
- 不声明 `map[string]any` / `map[string]interface{}` 无类型事实袋；
- 每个受检边界确实包含 production Go 文件，且嵌套泛型函数 fixture 必须被 guard 拒绝，防止只扫当前目录或空目录假通过。

AST 只能防已知语法形状；换名的万能抽象、隐藏跳转或语义越权仍依赖 code review、路径 diff 与下一节真实模型约束。

### 12.2 起草时已执行的 runtime negative diff

```bash
git diff --name-only 47fc94d..544f4af -- \
  cmd deploy migrations web internal/lottery/adapter \
  internal/infrastructure internal/platform internal/participation \
  configs go.mod go.sum
```

在 `544f4af` 终审加固提交后实际结果：无输出。另检查现有 Lottery Strategy/Award/WeightedSelector、repository 与 ephemeral selection 核心文件的定向 diff，无输出；`internal/lottery/adapter` 与 `migrations` 下没有 membership 命名文件。

冻结前又在证据候选 `a04d89d` 上用加入 `scripts` 的并集重跑：

```bash
git diff --name-only 47fc94d..a04d89d -- \
  cmd deploy migrations web scripts configs go.mod go.sum \
  internal/lottery/adapter internal/infrastructure \
  internal/platform internal/participation
```

实际仍无输出。该证据证明本节没有悄悄装配 runtime；accepted tip 形成后还要以同一白名单复核一次。

## 13. 起草前已实际执行的定向命令

### 13.1 Application 普通测试

```bash
go test -count=1 ./internal/lottery/application
```

- **实际：** exit 0；包含 routing、error、64-worker 与架构测试。
- **边界：** 编译 Lottery domain，但不执行 domain 的 `_test.go`。

### 13.2 Application race

```bash
go test -race -count=1 ./internal/lottery/application
```

- **实际：** exit 0，无 race 报告。
- **边界：** 只覆盖 application package 及其测试依赖，不是全仓 race。

### 13.3 Application 二十轮 shuffle

```bash
go test -shuffle=on -count=20 ./internal/lottery/application
```

- **实际：** exit 0。
- **边界：** 只证明当前 application tests 不依赖固定执行顺序。

### 13.4 独立 doccheck

```bash
go run ./cmd/doccheck
```

- **实际：** QA/API 与并行课程、设计、面试文件全部落盘后复跑 exit 0，输出 `documentation checks passed`。
- **边界：** 只检查当前文档链接、ADR 注册、课程登记规则等结构约束；根代理仍需在索引/状态与最终证据全部收口后再次复跑。

## 14. 根代理最终复跑结果

以下命令均由根代理在 2026-08-30 实际执行。代码/文档候选内容包括递归架构 guard、revision 语义校准和全部第 27 节材料；最终 clean-worktree 重跑见 14.6。

### 14.1 Domain + application 普通测试

```bash
go test -count=1 \
  ./internal/lottery/domain \
  ./internal/lottery/application
```

- **实际：** 两个 package 均 exit 0；domain 事实、policy、route/path、application contract、递归架构 guard 与 nested generic-function 反例均执行通过。

### 14.2 Lottery race 与 shuffle

```bash
go test -race -count=1 ./internal/lottery/...
go test -shuffle=on -count=20 ./internal/lottery/...
```

- **实际：** 两条命令均 exit 0；Lottery 全部 package 无 race 报告，20 轮顺序扰动通过。

### 14.3 Unsupported tier 独立 fuzz

```bash
go test ./internal/lottery/domain -run='^$' \
  -fuzz='^FuzzMembershipTierFactSnapshotNeverAcceptsUnsupportedTier$' \
  -fuzztime=10s
```

- **实际：** exit 0；本次 10 秒窗口执行 `5,150,324` 次，`new interesting: 0 (total: 2)`，无 panic/crash。该数字只描述本次有限探索，不外推为穷举证明。

### 14.4 Atomic coverage

```bash
lesson27_cover_dir=$(mktemp -d /tmp/growthos-lesson27-cover.XXXXXX)
lesson27_cover_file="$lesson27_cover_dir/lottery.cover"

go test -count=1 -covermode=atomic \
  -coverprofile="$lesson27_cover_file" \
  ./internal/lottery/domain \
  ./internal/lottery/application

go tool cover -func="$lesson27_cover_file"
```

- **实际：** exit 0；domain `95.3%`，application `90.7%`，合计 statements `93.7%`。不设置为了数字而设的阈值。
- **清理：** 实际 profile 为 `/tmp/growthos-lesson27-cover.6Os6mW/lottery.cover`，`28,124` bytes；先以 `find`/`stat` 精确确认只有该文件，再删除文件并 `rmdir` 该随机目录，最后确认目录不存在。更早一次固定路径 profile 也在执行前确认不存在、读取后精确删除；仓库内从未生成 coverage 文件。

### 14.5 最终仓库门禁

```bash
go run ./cmd/doccheck
make verify
go test -race -count=1 ./...
git diff --check 47fc94d..HEAD
git diff --check

git diff --name-only 47fc94d..HEAD -- \
  cmd deploy migrations web scripts configs go.mod go.sum \
  internal/lottery/adapter \
  internal/infrastructure internal/platform internal/participation \
```

- **实际：** `make verify` exit 0；Go 全仓测试、vet、doccheck、19 个前端测试文件/152 个测试、TypeScript typecheck 与 Vite production build 全部通过。全仓 race exit 0；两个 diff-check exit 0；candidate runtime negative diff 无输出。
- **构建清理：** `make verify` 每次只生成 `web/dist/index.html`、一个 CSS chunk 与四个 JS chunk；根代理先列出所有 8 个目录/文件节点，再只对本次执行前已确认不存在的 `web/dist` 执行 depth-first 删除，并确认目录不存在。
- **边界：** 这一轮验证完整候选内容，但执行时索引/QA 尚未提交，因此不能冒充 clean-worktree 证据。

### 14.6 Clean-worktree accepted-tip 门禁

最终证据提交后执行：

```bash
test -z "$(git status --porcelain)"
make verify
go test -race -count=1 ./...
git diff --check 47fc94d..HEAD
git diff --name-only 47fc94d..HEAD -- \
  cmd deploy migrations web scripts configs go.mod go.sum \
  internal/lottery/adapter internal/infrastructure \
  internal/platform internal/participation
```

- **实际候选：** `aaa4a8fd63c4b94a8d1ead38d1067655f2b43e48`。命令开始前和 `web/dist` 清理后两次 `git status --porcelain` 均为空；`make verify`、全仓 race、章节 diff-check、祖先/无 merge 检查均 exit 0，runtime negative diff 无输出。
- **构建清理：** clean gate 生成与前述一致的 8 个 `web/dist` 节点，逐项列出后 depth-first 精确删除，删除后目录不存在且工作树恢复为空。
- **冻结边界：** 本次只验证包含所有实现、正文、QA 候选与索引的提交；当前证据更新提交后还会对远端实际 accepted tip 复跑 doccheck、章节 diff-check、negative diff、clean status 与远端一致性，避免把候选 SHA 冒充最终 SHA。

## 15. 为什么没有 Adapter、API、Compose 或浏览器 E2E

本节只有 Lottery domain/application 内核。强行对旧 ephemeral endpoint 执行 curl 或浏览器截图，只能证明旧路径仍存在，不能证明 membership router 被调用。

因此本文不声称：

- 已从真实 IAM/会员平台取得 tier；
- HTTP request 已绑定 MembershipSubjectRef；
- Activity 已选择 routing policy；
- Strategy target 已被 repository 成功加载；
- selector 已在路由目标上运行；
- unknown/越权用户已通过服务端真实请求被拒绝；
- Compose readiness 已检查会员 authority；
- React 已显示 branch/path 或按会员等级改变界面；
- browser/API E2E 已覆盖路由。

未来真实消费者出现时至少要补 adapter contract、身份映射、timeout/freshness、错误映射、Activity 发布引用、目标完整性、服务端授权、跨层 trace、直接 API 反例与浏览器 E2E。

## 16. 精确负向断言

以下任一情况出现，本节应判不通过：

- unsupported/unknown/not-found/timeout/stale/future/corrupt 进入 baseline default；
- provider 直接返回 Strategy ID 或最终 route verdict；
- 客户端提交的 tier、target、branch、policy revision 或 evaluated-at 被当权威事实；
- MembershipSubjectRef 被描述成认证 Principal；
- 会员等级被当作 admin 角色或权限；
- route success 被当作 Participation eligible 或 access allowed；
- standard/premium 目标相同时丢失 branch 证据；
- target 为零仍形成 confirmed decision；
- 只凭 target 非零就确认决定，或忽略 branch/reason/path 之间的不一致；
- application 在 `Confirmed()==false` 时仍返回 nil error；
- 把 `ErrMembershipRoutingDecisionInvalid` 伪装成 provider unavailable 或业务 default；
- 把同一 policy revision 当成内容唯一性证明，而没有 registry/hash/发布约束；
- fact+error 时使用 fact；
- provider domain invalid cause 通过 `errors.Is` 泄露为第二 public class；
- technical failure 返回 target 或半条 path；
- `Path()` 返回值可改写内部决定；
- 一次 Route 读取 clock 或 reader 多于一次；
- pre-cancel/invalid config 后仍访问依赖；
- caller cancellation 已可观察却返回 provider error；
- provider deadline 被误报为 caller deadline；
- nil/typed-nil/partial service panic；
- router 加载 Strategy、调用 selector、随机源、repository、Redis 或发送消息；
- Lottery import Participation，或 Participation 开始拥有 Lottery target；
- 出现 hidden next-index、map-first default、通用 Rule/Tree/Engine/DSL 或 untyped fact bag；
- 新增 HTTP/DB/Web/Compose 改动却仍声称本节只有内核；
- 用 application-only race、fuzz seed 或旧 endpoint smoke 冒充完整章节验收；
- 最终门禁未运行就把所有 checklist 写成完成。

## 17. 剩余风险与下一责任

| 风险 | 当前影响 | 下一触发器 / 责任 |
| --- | --- | --- |
| 无真实会员 adapter | router 无 production fact source | provider 协议明确后新增 adapter/contract 章节 |
| SubjectRef 无 Principal 绑定 | 不能安全暴露公开 route | 第 31～35 节会话与服务端授权 |
| 只支持两种 tier | provider 新枚举会失败关闭 | 新 tier 产品语义与 policy revision 审查 |
| target 只校验非零 | 可能指向不存在/未发布 Strategy | 第 28/30 节引用完整性与 Activity 发布 |
| policy 由调用方显式提供 | 无运营发布、灰度、回滚 | 第 28～30 节配置/发布模型 |
| revision 未绑定不可变内容 | 相同 revision 仍可能被误用于不同 targets | 规则树/Activity 发布 registry 建立唯一内容约束 |
| 单一 as-of 非原子 | provider 与 policy 可跨时点 | adapter revision/as-of contract 或上层重检 |
| provider 可忽略 context | 同步调用可能长期阻塞 | 真实 transport timeout 与故障注入 |
| max age 仅要求正数 | 错配过大值会接受陈旧事实 | composition/config 安全上下限 |
| Cause 可被误打印 | 显式诊断出口仍可能泄密 | logger/observer allowlist 与泄密测试 |
| path 含会员派生 branch | 对外/持久化可能泄露 | 访问控制、披露级别、保留策略 |
| 一跳 router 不是树/引擎 | 无法表达多层和共享子路径 | 第 28～29 节按真实需求演进 |
| 无正式流程消费者 | route 后事实仍可能变化 | Activity/Participation/Draw 编排与重检 |

## 18. 清理与环境影响

- 本节未启动容器、未修改数据库、未创建 Redis key、RabbitMQ queue、浏览器 trace、下载资源或外部服务状态；
- atomic coverage 使用 `mktemp -d` 的随机目录；profile 经读取和精确枚举后删除，目录经 `rmdir` 删除，固定 `/tmp/growthos_lesson27_lottery.cover` 也确认不存在；
- 普通/race/shuffle 使用共享 Go build/test cache，它可供后续章节复用，不作为 disposable task artifact 删除；
- 10 秒 fuzz 已执行；它可能更新共享 Go fuzz/build cache，缓存用于后续验证且不在仓库内，因此保留；没有新增失败 corpus 或仓库内 artifact；
- 两次 `make verify` 都在执行前确认 `web/dist` 不存在；生成的同一组 8 个目录/文件节点逐项枚举后被精确删除，最终再次确认 `web/dist` 不存在；
- 当前 shell 未设置 `VAULT`，也没有可发现且获授权的个人 Obsidian Vault 路径，因此未执行 `make docs-sync VAULT=...`，不伪造同步成功；仓库内 `go run ./cmd/doccheck` 已独立通过，但它不等于个人 Vault 同步；
- 第 27 节没有安装额外 skill、插件、系统包或项目依赖，也没有产生需要清理的下载图片、临时目录或构建缓存。

## 19. 当前验收清单

- [x] 会员事实、封闭 tier 与 source/revision 边界已有 domain tests；
- [x] premium override、standard baseline default 与稳定 literal 已有 route tests；
- [x] target convergence 不丢 branch、path 防御性复制已有 tests；
- [x] zero/unsupported/future/policy/as-of 错误返回 zero decision 已有 tests；
- [x] application call-order、clock once、freshness 1ns、fact+error 已有 tests；
- [x] caller cancellation、provider deadline 与 blocking-reader 边界已有 tests；
- [x] domain bad payload 到 application invalid、单一 class 与显式 Cause 已有 tests；
- [x] nil/typed-nil/partial service 与 64-worker 并发已有 tests；
- [x] AST ownership/engine stop line 已有架构测试，并以嵌套 generic function fixture 证明递归扫描；
- [x] 起草时 application normal/race/20-round shuffle 已实际通过；
- [x] 从 `47fc94d` 到终审加固 `544f4af` 的 runtime negative diff 已实际为空；
- [x] API 记录明确公开 route/DTO/header/status 零变化；
- [x] 未伪造 adapter/runtime/API/Compose/browser E2E；
- [x] 本文落盘后的独立 doccheck 已 exit 0；
- [x] domain + application 最终普通测试；
- [x] Lottery 全量 race 与 20 轮 shuffle；
- [x] 独立 10 秒 fuzz 与 atomic coverage；
- [x] 完整候选内容上的 `make verify`、全仓 race、章节/worktree diff-check 与 runtime negative diff；
- [x] coverage/web build artifact 均已精确枚举与清理；
- [x] 当前 shell 无 `VAULT`，已如实记录未执行个人 Obsidian 同步；
- [x] 干净候选 `aaa4a8f` 的 clean-worktree `make verify`、全仓 race、diff、线性历史与清理门禁；
- [x] 根代理以本次最终证据提交推送并按同名远端实际 tip 冻结本节；具体 SHA 不在提交前猜测。

最终 accepted tip 必须以根代理提交并推送后的同名远端分支实际 tip 为准，本文不虚构尚未生成的 SHA，也不把待复跑门禁提前写成通过。
