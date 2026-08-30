# 第 26 节 QA：风险准入与最小线性前置资格链验收

- **需求基线：** [Participation 前置资格链基线 v1](../../product/participation-prerequisite-chain-v1.md)
- **架构决策：** [ADR-0022](../../decisions/ADR-0022-participation-prerequisite-chain.md)
- **上游规则边界：** [Lottery 业务规则需求基线 v1](../../product/lottery-rule-requirements-v1.md)
- **上一节：** [第 25 节 QA](lesson-25.md)
- **基准提交：** `8b2f3a61dc037689d7bd4930916f5bfb847705ab`（第 25 节已验收 tip）
- **实现提交：** `ad25dbe`（风险事实与具体准入规则）；`f77e17f`（固定顺序、共享 as-of、短路与 trace）
- **规格与证据提交：** `974a61b`（产品基线）；`5271963`（ADR-0022）；`61282ba`（课程/API）；`d43bbff`（面试问答）；`d87343e`（设计手记）；`eac4b92`（产品证据与 EOF 修复）；`ff1f24f`（架构当前态）；`833abd0`（全局索引与章节登记）；`c46410e`（手工 partial-chain 反证加固）
- **最终验收提交：** 包含本 QA 与分支台账的冻结提交；由根代理在提交、推送后以同名远端 tip 确认，不预写尚未生成的 SHA
- **验收日期：** 2026-08-30，Asia/Shanghai
- **当前记录状态：** Participation 普通测试、race、20 轮 shuffle、atomic coverage 和两组 10 秒 domain fuzz 均已通过；`go run ./cmd/doccheck`、最终 `make verify`、全仓 race、最终 `git diff --check`、从第 25 节 tip 到证据 HEAD 的 runtime negative diff 与构建产物清理也均已实际通过。本节没有真实 fact adapter、HTTP/API、数据库、Compose 或浏览器路径，因此不执行也不声称这些资格链 E2E 已通过

> 本节验收的是 Participation 内部第二条真实规则和一个固定的两节点只读资格链。它能证明“先判断新用户，再按需读取风险事实”，并在拒绝、技术失败和 caller cancellation 时停止；它不能证明公开抽奖入口已经受保护。当前 `ParticipantRef` 仍不是登录身份，风险 reader 仍是端口，现有 Lottery API、数据库和前端都没有装配这条链。

## 1. 验收范围

### 1.1 本节必须成立

- 风险事实提供方拥有最小 `passed/blocked` screening disposition，Participation 只拥有“该事实是否允许进入当前参与场景”的决定；
- 风险快照只包含 participant reference、disposition、source-owned assessed-at、source 和 source revision，不包含设备指纹、模型特征、分数、阈值、原始 payload 或用户文案；
- `passed` 形成风险节点 `eligible`，`blocked` 形成风险节点 `ineligible`；零值或未知 disposition 不形成业务决定；
- 风险 fact age 恰好等于最大年龄仍有效，超过 1ns 才是 stale；晚于 shared as-of 的事实是 invalid/future，不能以负年龄绕过；
- 组合顺序固定为 `participation.new_user.registered_on_or_after` 后 `participation.risk.screening_admission`；
- 新用户节点只有确定 `eligible` 才访问风险 reader；确定拒绝、读取失败、事实错误或取消都使风险 reader 保持零调用；
- 两个节点共享 chain 在任何 reader 之前捕获的一次受控服务端 logical as-of；clock 不因 reader 阻塞或节点切换而再次读取；
- 只有两节点都确定通过，聚合结果才是 `eligible/all_prerequisites_satisfied`；单节点通过只表示 continue；
- 技术失败和 cancellation 返回零 `PrerequisiteEvaluation` 加 error，不返回可被误用的半成品业务结果；
- 业务确定结果携带 ruleset revision、同一 evaluated-at 和仅含实际执行节点的有序 trace；
- `Steps()` 返回副本，调用方不能篡改聚合内部证据；
- error wrapper 对普通 `Error()` / `errors.Is` 只暴露一个审核过的语义 class，诊断 cause 只能通过显式 `Cause()` 取得；
- nil、typed-nil 和手工 zero/partial chain 在任何 I/O 前失败关闭；
- 64 个 goroutine 可以共享只读 chain；race detector 对当前实现与线程安全 test doubles 不报告数据竞争；
- Participation domain 不依赖仓库内其他项目包，application 只依赖 Participation domain；
- 不出现通用 Rule platform、generic type、untyped fact bag、priority、规则树、DSL 或任意执行图；
- 相对第 25 节，Lottery、HTTP、composition root、Migration、Redis、Compose、配置和 Web runtime 保持零变化。

### 1.2 本节明确不验收

- 真实用户目录或风险系统 adapter、SDK、凭证、重试、熔断、连接池与生产 timeout；
- `ParticipantRef` 与登录 Principal、租户、角色、session 或授权 scope 的可信绑定；
- Activity 生命周期、发布版本、活动级 ruleset 选择或 Marketing 门控；
- 剩余次数账户、次数预占、扣减流水、并发消费或检查后再使用竞态的关闭；
- HTTP route、DTO、status、error code、header、request ID、浏览器交互或导航裁剪；
- MySQL table/Migration/grant、Redis key/cache、RabbitMQ message 或 PostgreSQL projection；
- Lottery selector、正式 Participation、Draw、Result、Award、库存或发奖编排；
- 多分支路由、规则树、数据库配置规则、决策引擎、DMN、OPA 或 XACML runtime；
- trace 持久化、OpenTelemetry span、安全审计、指标 exporter 或告警；
- 多 authority 的原子一致快照、正式重放接口、生产隐私保留策略或 SLO；
- 认证、服务端 RBAC、前端权限裁剪与浏览器越权 E2E；这些仍按后续权限章节演进。

## 2. 证据边界：已证、未证与禁止推导

| 命题 | 当前证据 | 当前结论 | 不能由该证据推出 |
| --- | --- | --- | --- |
| 第二条规则来自真实 Participation 需求 | 产品基线、ADR 和风险 domain tests | 已证于当前业务建模 | 风险 provider 已接入或其 verdict 一定正确 |
| passed/blocked 到准入决定的映射确定 | domain table、时区、重复执行与 fuzz | 已证于纯领域输入 | provider timeout 可以当 blocked 或 passed |
| freshness 使用 source-owned assessed-at | application 1ns 边界测试 | 已证于当前 helper | adapter 不会错误地用读取时间给旧 verdict 续鲜 |
| new-user 拒绝后不读风险事实 | reader 调用计数与一步 trace | 已证于固定链 | 任意未来动态 plan 都自动保持该隐私边界 |
| 两节点共享一次 as-of | clock 计数、UTC 规范化、阻塞 risk reader 测试 | 已证于单次进程内调用 | 两个 authority 的读取成为原子快照 |
| 取消比 dependency result 优先 | pre-cancel、clock 后、两个 reader 后取消测试 | 已证于可观察取消点 | 忽略 context 且永不返回的 adapter 能被同步调用强制终止 |
| 技术失败不冒充业务拒绝 | zero aggregate/error matrix | 已证于当前 application | 未来 HTTP mapping 一定保留该区分 |
| error class 单值且 cause 不外泄 | `Is`、`Cause()`、安全字符串与 typed-nil tests | 已证于当前 wrapper | 日志调用方绝不会主动打印 `Cause()` |
| trace 有序且不可由返回 slice 改写 | 顺序、长度、字段和 copy test | 已证于内存值对象 | trace 已持久化、可跨服务审计或满足合规要求 |
| chain 可并发只读执行 | 64 worker test + Participation race | 已证于线程安全 doubles | 任意生产 adapter 都并发安全 |
| 没有提前造通用规则平台 | AST architecture test + source review | 已证于已知类型名/import/map/generic 形状 | 所有换名后的语义越界都能由 AST 自动发现 |
| runtime 零漂移 | 从 `8b2f3a6` 到证据 HEAD `c46410e` 的 path-scoped negative diff | 已证于完整实现、配套文档与审查加固切片 | 公开 Lottery 已执行资格检查 |
| 当前 Participation 测试无回归 | normal/race/shuffle/coverage/fuzz | 已证于本节 package | 全仓、前端、Compose、MySQL、Redis 或浏览器无回归 |

核心禁止推导有两个：

1. **“风险规则和责任链存在”不等于“真实请求已被资格链保护”。** 当前没有 adapter、composition 和公开 transport。
2. **“共享 as-of”不等于“跨系统事务快照”。** 两个 reader 仍是顺序的独立 authority 调用；as-of 提供可解释的逻辑基准，不制造原子性。

## 3. 按提交核查线性演进

从第 25 节已验收 tip 顺序检查：

| 顺序 | 提交 | 验收焦点 |
| --- | --- | --- |
| 1 | `974a61b` | 先定义第二条真实规则、事实所有权、一次 as-of、固定顺序、短路和未做边界 |
| 2 | `5271963` | 比较次数、Activity、权限、预取、双时钟、通用 Rule 等备选方案并接受固定 Participation chain |
| 3 | `ad25dbe` | 落地 RiskScreeningFactSnapshot、RiskAdmissionPolicy、具体 evaluator、reader port 与安全错误分类 |
| 4 | `f77e17f` | 在第二条规则已存在后抽取固定两节点执行协议、共享 as-of、trace、取消与架构停止线 |

可复跑：

```bash
git log --reverse --oneline \
  8b2f3a61dc037689d7bd4930916f5bfb847705ab..f77e17f

git diff --stat \
  8b2f3a61dc037689d7bd4930916f5bfb847705ab..f77e17f
```

通过条件不是“提交数量足够”，而是抽象顺序真实：先写业务边界，再建立第二条具体规则，最后才从两条规则共同需要的 continue/reject/error/cancel 语义抽出最小链。若先提交通用 `RuleEngine` 再寻找风险业务，本节应判不通过。

## 4. 风险事实与具体准入规则

### 4.1 `RiskScreeningFactSnapshot`

构造器与 `Validate` 必须证明：

- participant reference 非零；
- disposition 只能是显式 `passed` 或 `blocked`，`review`、空值和未来新增但未协商的枚举都失败关闭；
- assessed-at 非零、进入对象时规范为 UTC，并去除附带单调时钟信息；
- source 非空、无首尾空白、UTF-8 有效、字符可打印且不含危险格式控制字符，最多 128 bytes；
- revision 遵守相同安全 token 规则并最多 256 bytes；
- 构造失败返回零 snapshot；手工绕过构造器的零值仍会被 `Validate` 拒绝；
- snapshot 没有风险分数、阈值、设备指纹、模型输入、完整主体资料或最终 Participation verdict。

source-owned `assessed_at` 是 freshness 起点。adapter 将“读取发生时刻”写成 assessed-at 会给陈旧结论续鲜，违反本节契约，即使所有单元测试仍可能绿色。

### 4.2 `RiskAdmissionPolicy` 与 revision 分离

- 固定 rule code 为 `participation.risk.screening_admission`；
- policy revision 是独立、受界 token；
- ruleset revision 另行标识 `new-user -> risk` 的顺序计划；
- fact revision 来自 source snapshot；
- 三类 revision 与应用 commit/version 不能混成一个 `version` 字段。

调序会改变首要拒绝 reason、风险 authority 流量和隐私暴露，因此 ruleset revision 不是装饰字段。相反，风险 policy v1 的映射目前固定，尚无证据支持数据库动态阈值。

### 4.3 领域决定矩阵

| 输入 | 决定 | reason | error | 实测结果 |
| --- | --- | --- | --- | --- |
| passed，assessed-at 在 as-of 或之前 | `eligible` | `risk_screening_passed` | nil | 通过 |
| blocked，assessed-at 在 as-of 或之前 | `ineligible` | `risk_screening_blocked` | nil | 通过 |
| assessed-at 恰好等于 as-of | 依据 disposition 确定 | 对应稳定 reason | nil | 通过 |
| assessed-at 晚于 as-of 1ns | zero | 空 | future/evaluation error | 通过 |
| zero policy/fact/as-of | zero | 空 | domain validation error | 通过 |
| 同一 instant 的 UTC/UTC+8/UTC-4 表示 | 完整决定一致 | 一致 | nil | 通过 |
| 相同输入重复执行 | 完整决定一致 | 一致 | nil | 通过 |

决定本身只保留 rule/reason、policy revision、fact source/revision 和 evaluated-at，不保留 participant reference 或敏感风险数据。

## 5. 固定链、顺序与短路验收

执行协议为：

```text
validate request, ruleset and two concrete policies
  -> validate chain dependencies and freshness bounds
  -> honor pre-cancellation
  -> capture one server as-of
  -> honor cancellation observed after clock
  -> read + validate + evaluate registration prerequisite
       error/cancel     -> zero aggregate, risk calls = 0
       ineligible       -> one-step confirmed aggregate, risk calls = 0
       eligible         -> continue
  -> read + validate + evaluate risk prerequisite
       error/cancel     -> zero aggregate
       blocked          -> two-step confirmed ineligible
       passed           -> two-step confirmed eligible
```

### 5.1 业务路径矩阵

| 新用户节点 | 风险节点 | 聚合结果 | trace 长度 | risk reader 调用 | 实测结果 |
| --- | --- | --- | ---: | ---: | --- |
| eligible | passed/eligible | `eligible/all_prerequisites_satisfied` | 2 | 1 | 通过 |
| ineligible | 未执行 | `ineligible/registration_before_cutoff` | 1 | 0 | 通过 |
| eligible | blocked/ineligible | `ineligible/risk_screening_blocked` | 2 | 1 | 通过 |
| registration read error/stale/future | 未执行 | zero + typed error | 0 | 0 | 通过 |
| eligible | risk read error/stale/future/mismatch | zero + typed error | 0（不返回半成品） | 1 | 通过 |

最后一行在内部确实已经执行过新用户节点，但公开返回必须是 zero aggregate，而不是带一步 trace 的“部分决定”。这避免调用方忽略 error 后继续使用一个未完成的资格结果。运行时 observer 若未来需要记录失败位置，应消费安全 error class，而不是把半成品业务对象交给 transport。

### 5.2 短路的验收标准

短路不能只断言最终 bool，还必须断言副作用边界：

- 新用户拒绝时 registration reader 1 次、risk reader 0 次、clock 1 次；
- registration reader 返回 fact 加 error 时仍按 error 失败，risk reader 0 次；
- registration stale/future 时 risk reader 0 次；
- 风险阻断发生在第二节点，两个 reader 各 1 次；
- trace 只包含实际执行的节点，不写一个伪造的 `risk: skipped/pass`；
- fixed order 不来自 map iteration、数据库 priority 或调用方 slice。

这组证据同时保护隐私、成本和故障隔离：已被低敏规则确定拒绝的主体不会再访问更敏感的风险 authority。

## 6. shared as-of 与 freshness

### 6.1 时间语义

chain 在任何事实读取前读取一次 `Clock.Now()`，规范为 UTC，并封装成 application 包内 `evaluationInstant`。生产公共 API 不导出接受裸 `time.Time` 的 `EvaluateAt`，因此 transport 或浏览器不能传一个故意过旧的时间把 stale fact 伪装为 fresh。

两种调用路径要区分：

- 第 25 节 standalone `NewUserEligibilityService.Evaluate` 保持“注册 fact 成功读取后捕获 clock”的既有契约；
- 第 26 节 chain 为两节点可解释性，在第一个 reader 前捕获一次 shared as-of。

这不是隐式兼容变化。测试必须同时保护 standalone 旧语义与 chain 新组合语义。

### 6.2 已覆盖边界

| 边界 | 预期 | 实测结果 |
| --- | --- | --- |
| registration observed-at age == max age | 可继续 | 通过（继承并回归第 25 节） |
| registration observed-at age > max age 1ns | zero + stale；risk 0 调用 | 通过 |
| registration observed/registered-at > as-of | zero + invalid；risk 0 调用 | 通过 |
| risk assessed-at age == max age | passed/blocked 均可形成对应决定 | 通过 |
| risk assessed-at age > max age 1ns | zero + stale | 通过 |
| risk assessed-at > as-of 1ns | zero + invalid | 通过 |
| clock 返回 UTC+8 的同一 instant | 聚合与两步 trace 均规范为 UTC | 通过 |
| registration reader 将 clock test double 改到 +12h | 本次仍使用捕获的原 as-of | 通过 |
| risk reader 阻塞期间 clock 改到 +12h | 释放后仍只调用 clock 1 次且沿用原 as-of | 通过 |
| clock 返回 zero time | 两个 reader 都为 0 调用，zero aggregate | 通过 |

### 6.3 不能夸大的地方

- 单一 as-of 不锁住两个 authority；
- as-of 后产生的 snapshot 会在本次被视为 future，并留给下一次评估；
- adapter 必须能返回在 as-of 已存在的不可变快照，或明确失败；
- 顺序读取会增加尾延迟，这是为短路和敏感读取最小化接受的成本；
- 当前没有正式重放 API，ruleset/fact revisions 只是重放所需证据的一部分。

## 7. 取消与 deadline 优先级

### 7.1 已实际由测试覆盖的点

- pre-canceled context：clock、registration reader、risk reader 全部 0 次；
- clock callback 触发取消：clock 1 次、两个 reader 0 次，返回原始 `context.Canceled`；
- registration reader 返回时触发取消：clock 1 次、registration 1 次、risk 0 次，caller cancellation 覆盖 reader result；
- risk reader 返回时触发取消：clock 1 次、两个 reader 各 1 次，返回 zero aggregate + caller cancellation；
- provider 自己返回 `context.DeadlineExceeded` 但 caller context 仍活跃：分类为 provider unavailable，不能伪装成 caller deadline；
- wrapped provider deadline 同样保留为受信诊断 cause，但不进入公开 `errors.Is` 树。

每个节点调用前后都检查 `ctx.Err()`，目的是让已经可观察的 caller cancellation 优先。它不可能消除所有纳秒级竞态，也不能抢占一个忽略 context 且永不返回的同步 reader；真实 adapter 必须配置 transport timeout 并遵守 context。

### 7.2 负向断言

以下任一行为都应判失败：

- pre-cancel 后仍读取 clock 或事实；
- registration 已失败后还访问 risk authority；
- caller 已取消却把 provider error 返回为主要语义；
- provider 自有 deadline 被误报为 caller deadline；
- cancellation 返回一份 `Confirmed()==true` 的 aggregate；
- 用 goroutine 泄漏来“强制”中断不遵守 context 的 adapter。

## 8. error class、显式 `Cause()` 与脱敏

### 8.1 当前错误协议

`RegistrationFactReadError` 与 `RiskScreeningFactReadError` 均：

- `Error()` 只渲染稳定 application class；
- `Is(target)` 只匹配一个审核过的 class；
- 未知 class、零值和 typed-nil error wrapper 都 fail closed 到通用 read failure；
- `Cause()` 显式保留诊断 cause；
- 不实现把 cause 接入公共匹配树的 `Unwrap()`；因此 `errors.Is(wrapper, cause)` 必须为 false；
- 普通错误字符串不得包含测试中注入的 `secret endpoint/subject detail`。

这关闭了第 25 节曾记录的“class A 的 wrapper 通过 Unwrap 又命中 cause 中 class B”的多 class 风险。代价是可信诊断代码不能只依赖通用 `errors.Is` 沿 cause 递归，必须显式、受控地读取 `Cause()`；日志边界仍不得无审查打印它。

### 8.2 风险读取分类矩阵

| reader 返回 | public class | `Cause()` | 业务决定 |
| --- | --- | --- | --- |
| classified not found | `ErrRiskScreeningFactNotFound` | 原诊断 cause | zero |
| classified unavailable | `ErrRiskScreeningFactUnavailable` | 原诊断 cause | zero |
| classified read failure | `ErrRiskScreeningFactReadFailure` | 原诊断 cause | zero |
| unknown error | `ErrRiskScreeningFactReadFailure` | unknown error | zero |
| raw/wrapped provider deadline，caller 仍活跃 | `ErrRiskScreeningFactUnavailable` | deadline chain | zero |
| reader 返回 fact + error | error 优先，fact 丢弃 | 对应 cause | zero |
| caller 在 reader 返回时取消 | 原始 caller context error | 不包装为 provider class | zero |

### 8.3 仍需后续关闭的风险

- `Cause()` 是显式诊断出口，不是自动脱敏器；调用方仍可能误打印；
- 还没有 HTTP mapping，不能声称 public status/code 已稳定；
- 还没有 metrics/observer，不能声称错误 class 已进入低基数监控；
- 嵌套同类 wrapper 可以保留多层 cause，未来 adapter contract 应限制无意义重复包装；
- source/revision 受长度与字符集约束，但 fact revision 仍可能高基数，不得直接成为 metric label。

## 9. nil、typed-nil 与配置失败

### 9.1 构造边界

构造器必须拒绝：

- nil / typed-nil `RegistrationFactReader`；
- nil / typed-nil `RiskScreeningFactReader`；
- nil / typed-nil pointer clock；
- typed-nil `ClockFunc`；
- zero 或负数 registration max age；
- zero 或负数 risk max age。

Go interface 的动态类型非 nil、动态值为 nil 时，`interface != nil` 仍可能成立。本实现只在组合防御处用受限 reflection 检查 nil-capable kinds，不把反射放进规则热路径，也不发展为 DI framework。

### 9.2 调用边界

以下输入必须在 clock/readers 之前失败：

- nil context；
- zero participant reference；
- zero/invalid ruleset revision；
- zero/invalid new-user policy；
- zero/invalid risk policy；
- nil chain 或手工构造的 zero/partial chain。

error wrapper 自身的 typed-nil 也有安全行为：`Error()` 返回通用 failure 文本、`errors.Is` 只命中通用 failure、`Cause()` 返回 nil，不 panic。

## 10. trace 与最小披露

### 10.1 确定结果必须携带

聚合层：

- final `eligible/ineligible`；
- terminal reason 或 `all_prerequisites_satisfied`；
- ruleset revision；
- chain-wide evaluated-at；
- 有序 executed-step trace。

每个 step：

- concrete rule code；
- `eligible/ineligible`；
- stable reason；
- concrete policy revision；
- fact source/revision；
- 与聚合相同的 evaluated-at。

### 10.2 不得携带或伪造

- ParticipantRef、账号、邮箱、手机号或租户；
- 风险分数、阈值、模型特征、设备指纹或 raw payload；
- 原始 provider error 或用户文案；
- 未执行节点的伪造结果；
- 技术失败时可被业务继续使用的 partial trace；
- 把 fact revision、ParticipantRef 或 raw error 放进 metrics label 的建议。

`Steps()` copy test 会把第一次返回 slice 的第一个元素改成零值，再次读取必须保持原证据不变。它证明 slice backing array 不暴露；所有 step 字段本身也是值类型和受控 token。

## 11. 并发、乱序与覆盖率证据

### 11.1 并发

64 个 goroutine 共享同一个 immutable chain、ruleset 和两份 policy：

- 每个调用形成相同的 confirmed aggregate；
- registration reader、risk reader 和 clock 各累计 64 次；
- chain 不保存 request-local trace/as-of；
- `go test -race` 对当前 test doubles 与实现无报告。

这个证据的前提是 reader/clock 本身并发安全。chain 不替 adapter 加锁、不拥有或关闭其资源，也不提供 singleflight/cache。

### 11.2 shuffle

实际运行 `-shuffle=on -count=20`，用于发现测试依赖全局执行顺序、复用可变状态或泄漏 goroutine 的问题。20 轮通过只能说明当前随机顺序未暴露耦合，不是形式化证明；未来增加 package-level mutable fixture 后仍应继续执行。

### 11.3 coverage

本轮使用 atomic mode 实际结果：

| package | statement coverage |
| --- | ---: |
| `internal/participation/domain` | 100.0% |
| `internal/participation/application` | 94.8% |
| Participation 合计 | 96.7% |

coverage 不是质量目标或生产正确性证明。application 未满覆盖的防御分支不能仅为追求数字而暴露不安全测试入口；真正关键的拒绝、短路、取消、typed-nil、shared as-of、Cause 隔离与 trace copy 已有行为断言。

## 12. 架构停止线与 runtime negative diff

### 12.1 AST 停止线

`TestLesson26ParticipationArchitectureKeepsBoundedEligibilityChain` 实际随普通、race、shuffle 和 coverage 测试通过，并检查：

- domain 不 import 任何仓库内项目 package；
- application 只允许 import `internal/participation/domain`；
- production type 不得声明 `Rule`、`RuleChain`、`RuleEngine`、`RuleTree`、`Specification`、`EvaluationContext`、`RulePriority` 或 `DSL`；
- 不得声明 generic production type；
- 不得声明 `map[string]any` 或 `map[string]interface{}` 式无类型 fact bag；
- 两个 package 必须确实含 production Go 文件，防止空目录让检查假通过。

它只守住已知语法形状。私有 `prerequisiteStep` 是两个真实 concrete decision 的包内执行适配，runner 仍是固定 plan；若未来用另一个名字藏万能协议，必须由 code review、依赖 diff 和后续真实消费者发现。

### 12.2 已实际执行的负向 diff

```bash
git diff --name-only \
  8b2f3a61dc037689d7bd4930916f5bfb847705ab..c46410e \
  -- cmd deploy migrations web internal/lottery \
     internal/infrastructure internal/platform configs go.mod go.sum
```

结果：**无输出。** 另实际确认 `internal/participation/adapter` 目录不存在，`migrations` 中没有 participation migration。

这证明从第 25 节最终 tip 到包含第 26 节实现、课程、API、设计、面试、产品/架构当前态、全局索引与 partial-chain 审查加固的证据 HEAD `c46410e` 为止：

- 没有 composition root 装配；
- 没有 route/status/header/DTO；
- 没有更改 Lottery selector 或 ephemeral API；
- 没有 MySQL/Redis/RabbitMQ/PostgreSQL 持久化；
- 没有配置、Compose 服务或 secret；
- 没有 Web 页面、导航、按钮或伪造角色状态；
- 没有真实身份、权限或越权 E2E。

本 QA 与分支台账的冻结提交只修改文档，不改变上述 runtime path 集。最终 accepted tip 不在本文虚构 SHA；根代理提交并推送这两份最终证据后，以 `origin/codex/lesson-26-responsibility-chain` 的实际 tip 为准。

## 13. 已实际执行并通过的命令

第 13.1、13.2、13.4～13.6 的定向证据最初在实现 HEAD `f77e17f` 实际执行；第 13.3 又在完整文档证据 HEAD `833abd0` 对 Participation 运行 20 轮 shuffle。交叉审查随后发现手工 zero/partial chain 的文档声明缺少直接反证，`c46410e` 因而补 7 组配置实例并通过定向、Participation 全量与 race 测试。第 14 节再在该加固 HEAD 和完整文档工作树上复跑 doccheck、`make verify`、全仓 race、20 轮 shuffle、diff-check 与最终负向 diff，避免把早期实现切片误写成最终仓库门禁。

### 13.1 普通定向测试

```bash
go test -count=1 \
  ./internal/participation/domain \
  ./internal/participation/application
```

- **预期：** 两个 package 均 exit 0；覆盖风险 fact/policy/evaluator、既有新用户回归、chain 顺序/短路、失败、取消、typed-nil、trace、并发与架构停止线。
- **实际：** exit 0；domain `ok`，application `ok`。

### 13.2 Participation race

```bash
go test -race -count=1 ./internal/participation/...
```

- **预期：** 两个 package 均 exit 0，64-worker chain 和既有 service 并发用例不报告 data race。
- **实际：** exit 0；application 与 domain 均 `ok`，无 race 报告。

### 13.3 二十轮乱序

```bash
go test -shuffle=on -count=20 ./internal/participation/...
```

- **预期：** 两个 package 均 exit 0，不依赖 test 执行顺序。
- **实际：** exit 0；application 与 domain 均 `ok`。

### 13.4 atomic coverage

```bash
go test -count=1 -covermode=atomic \
  -coverprofile=/tmp/growthos_lesson26_participation.cover \
  ./internal/participation/...

go tool cover \
  -func=/tmp/growthos_lesson26_participation.cover
```

- **预期：** exit 0，并输出可审计的 package/函数语句覆盖率；不设置为了数字而设的强制阈值。
- **实际：** exit 0；domain 100.0%、application 94.8%、合计 96.7%。

### 13.5 风险 assessed-at 边界 fuzz

```bash
go test ./internal/participation/domain -run='^$' \
  -fuzz='^FuzzEvaluateRiskAdmissionAssessedAtBoundary$' \
  -fuzztime=10s
```

- **预期：** offset > 0 必须是 zero decision + future error；offset <= 0 必须按 passed/blocked 形成相应决定；无 panic/crash。
- **实际：** PASS；12 workers、5,079,283 execs、`new interesting: 1 (total: 5)`，未生成仓库内 crash corpus。

新增 interesting input 表示 Go fuzz engine 扩充了共享 fuzz cache 中的覆盖输入，不表示发现失败；它也不是“覆盖全部 int64”的证明。

### 13.6 新用户 cutoff 回归 fuzz

```bash
go test ./internal/participation/domain -run='^$' \
  -fuzz='^FuzzEvaluateNewUserEligibilityCutoffBoundary$' \
  -fuzztime=10s
```

- **预期：** cutoff 前后和边界的不变量继续成立，证明 chain helper 重构没有破坏第 25 节规则。
- **实际：** PASS；12 workers、5,182,482 execs、`new interesting: 0 (total: 4)`，无 crash。

### 13.7 最终 runtime negative diff

上节 12.2 从 `8b2f3a6` 到证据 HEAD `c46410e` 的 path-scoped 命令已实际执行且为空；`internal/participation/adapter` 与 participation migration 也已实际确认不存在。包含本 QA/台账的冻结提交仍只属于文档证据，不新增 runtime 路径。

## 14. 最终章节冻结门禁：已实际通过

完成课程/API/设计/面试文档、索引和 review fix 后，已在完整证据工作树实际执行：

```bash
go run ./cmd/doccheck
make verify
go test -race -count=1 ./...
go test -shuffle=on -count=20 ./internal/participation/...
git diff --check

git diff --name-only \
  8b2f3a61dc037689d7bd4930916f5bfb847705ab..c46410e \
  -- cmd deploy migrations web internal/lottery \
     internal/infrastructure internal/platform configs go.mod go.sum
```

| 门禁 | 预期 | 实际结果 |
| --- | --- | --- |
| 独立 doccheck | 文档链接、ADR、状态与索引规则通过 | exit 0，输出 `documentation checks passed` |
| `make verify` | fmt-check、vet、全仓 Go test、doc-check、前端 tests/typecheck/build 全部 exit 0 | exit 0；Go vet/test/doccheck 通过；前端 19 files / 152 tests、typecheck 与 Vite production build 通过 |
| 全仓 race | 所有 Go package exit 0，无 race | exit 0，无 race 报告 |
| Participation shuffle | 20 轮随机测试顺序全部通过 | exit 0 |
| `git diff --check` | 无 trailing whitespace/EOF 空行错误 | exit 0，无输出 |
| 最终 negative diff | 上述 runtime 路径无输出 | exit 0，无输出；adapter 目录和 participation migration 均不存在 |
| 文档闭合 | status/index/API/课程/设计/面试/QA 链接一致 | 独立 doccheck 与 `make verify` 内 doccheck 均通过 |
| 构建产物边界 | 构建前后状态与任务产物可解释、可清理 | 构建前 `web/dist` 不存在；构建后枚举 6 个文件并精确删除整个生成目录，确认不存在 |

最终 accepted tip 是根代理随后创建并推送的“包含本 QA/台账的冻结提交”，不是表中任何预先猜测的 SHA。推送与累计分支快进是 Git 交接动作，不改变已经实际通过的代码、文档与负向门禁结论。

### 14.1 EOF 格式缺陷的发现与关闭历史

在早期实现切片 `f77e17f` 上，预检曾实际执行：

```bash
git diff --check \
  8b2f3a61dc037689d7bd4930916f5bfb847705ab..f77e17f
```

当时输出：

```text
docs/decisions/ADR-0022-participation-prerequisite-chain.md:175: new blank line at EOF.
docs/product/participation-prerequisite-chain-v1.md:173: new blank line at EOF.
```

`eac4b92` 随产品证据同步分别删除 ADR 与产品基线末尾的一个多余空行，没有借机改写 ADR 结论。此后在完整证据工作树再次运行 `git diff --check`，exit 0、无输出。保留这段失败历史，是为了证明“发现问题—形成可定位输出—最小修复—最终复跑”已经闭环，而不是把早期失败从 QA 中抹去。

## 15. 为什么没有 Compose、API 与浏览器 E2E

本节没有 transport、schema、adapter 或 UI 变化。强行为“责任链”启动 Docker、访问旧 Lottery route 或截取旧页面，只能证明既有系统仍能启动，不能证明风险资格链被真实调用。

因此本节没有执行也不声称：

- curl 某个 eligibility endpoint；
- 浏览器输入用户 ID 后看到资格结果；
- MySQL 中存在 registration/risk fact；
- Redis 缓存 risk verdict；
- RabbitMQ 发送 screening event；
- Compose 将真实 request 绑定到 ParticipantRef；
- 非法用户或不同角色已被服务端拒绝；
- 前端隐藏按钮即可阻止越权。

未来出现真实 adapter/公开消费者时，至少需要补：adapter contract、provider timeout/cancellation、fact provenance/freshness、错误映射、Principal 绑定、服务端强制、跨层 trace、越权反例与浏览器 E2E。届时不能拿本节的内存 doubles 代替。

## 16. 精确负向断言

以下任一情况出现，本节应判为不通过：

- 用“剩余次数 > 0”快照充当第二规则，却没有账户、预占或并发消费语义；
- 用 Activity、角色或权限凑第二节点，提前越过其所属章节；
- 风险 adapter 用本地读取时间覆盖 source-owned assessed-at；
- `review/unknown/not_found/timeout` 默认映射为 passed；
- blocked 与 dependency failure 都返回同一个 `ineligible`；
- 新用户拒绝或 registration failure 后 risk reader 仍被调用；
- 为了缩短尾延迟而预取风险事实，破坏敏感读取短路；
- 两节点各自读取 clock，边界附近形成混合时点；
- 接受浏览器或 transport 提交的 evaluated-at；
- as-of 后产生的 fact 被当成当前评估可用；
- 单节点 eligible 被当作最终 aggregate eligible；
- 技术失败返回 `Confirmed()==true` 或可用 partial aggregate；
- trace 含未执行节点、ParticipantRef、风险特征或 raw error；
- `Steps()` 返回的 slice 可改写内部聚合；
- wrapper 的 `errors.Is` 同时命中 public class 和 diagnostic cause；
- `Error()` 输出 provider endpoint、subject、SQL 或 payload；
- nil/typed-nil reader/clock 到运行时才 panic；
- fixed order 被替换成 map iteration、未版本化 priority 或任意调用方 slice；
- 新增 generic Rule/Engine/Tree/DSL 或 `map[string]any` fact bag；
- domain import Lottery、Gin、SQL、Redis、platform fault 或其他上下文；
- 修改 Lottery/API/Web 却仍声称“本节只有 Participation 内核”；
- 用旧 API/页面 smoke 冒充资格链 E2E；
- 用 96.7% coverage 或 10 秒 fuzz 声称生产正确性已证明；
- 在最终 `make verify`、全仓 race 或 `git diff --check` 未运行时把章节状态写成全部验收完成。

## 17. 剩余风险与下一责任

| 风险 | 当前影响 | 下一触发器 / 责任 |
| --- | --- | --- |
| 无真实 registration/risk adapter | chain 无 production 数据来源 | authority 协议明确后新增独立 adapter 章节与 contract/integration tests |
| 无可信 Principal 绑定 | ParticipantRef 可被调用方错误理解 | 真实 session 与服务端授权章节建立主体绑定 |
| 两 authority 非原子 | shared as-of 不能消除跨源 TOCTOU | provider snapshot contract、revision 重检或正式参与承诺时关闭 |
| after-as-of fact 必须失败 | 慢 reader 可能更常返回 future snapshot | adapter 提供 as-of snapshot/read revision，或明确返回技术失败 |
| provider 可忽略 context | 同步 read 可能长时间阻塞 | 真实 transport timeout、context contract 和故障注入 |
| max age 只要求正数 | 错配过大值会长期接受陈旧事实 | composition/config 落地时定义安全上限和环境校验 |
| `Cause()` 可被误打印 | 诊断出口仍需受信边界 | observer/logger contract、字段 allowlist 与泄密测试 |
| fact revision 高基数 | 直接做 metrics label 会放大 cardinality | 仅用于受控 trace/audit，指标使用 rule/outcome/error class |
| trace 仅在内存 | 不能跨请求审计或正式回放 | Participation/Draw 持久化设计时决定 snapshot/audit |
| fixed linear plan 只有两节点 | 无法表达会员分层多出口、缺省分支和合流 | 第 27 节用真实路由需求暴露局限，不在 handler 中写隐式 next-index |
| 无正式 Participation 消费 | 资格通过后事实仍可变化 | 次数/Draw 章节定义事务、lease、revision 或重检 |
| AST 检查基于已知形状 | 换名可能规避 | code review、最终 diff 与真实消费者约束共同守护 |

## 18. 清理与环境影响

### 18.1 定向证据创建与清理

- `/tmp/growthos_lesson26_participation.cover`：仅用于本节 coverage 报告；读取结果后实际解析为 `/private/tmp/growthos_lesson26_participation.cover`、确认是 31,061-byte regular file，再以原始精确绝对目标删除并确认路径已不存在；
- Go fuzz engine 可能把新增 interesting input 保存到共享 Go fuzz cache；它是可复用测试缓存，不是仓库交付物或 crash corpus；
- 最终门禁开始前再次确认 `/tmp/growthos_lesson26_participation.cover` 不存在；最终门禁结束后仍不存在；
- 未创建 Docker container、image、volume、network、Compose project、数据库 schema、Redis key、RabbitMQ queue、浏览器 trace 或下载图片。

### 18.2 最终 `make verify` 构建产物

- 构建前已确认 `/Users/florian/Desktop/Tencent/Go/GrowthOS-Go/web/dist` 不存在，因此后续目录可归因于本轮 Vite build；
- `make verify` 的 Vite production build 生成 `web/dist`，清理前用精确目录逐个枚举，共 6 个文件；
- 枚举后对精确绝对目录使用 `find -depth -delete`，没有使用通配符、未解析变量或仓库根递归删除；
- 删除后再次确认 `web/dist` 不存在；
- 共享 Go module/build/test/fuzz cache 可供后续章节复用，按约束保留；
- 源文件、依赖、用户数据、数据库和既有开发资源均未清理。

## 19. 最终验收清单

- [x] 第二条真实 Participation 风险准入规则已实现并由 domain/application tests 覆盖；
- [x] 固定 `new-user -> risk` 顺序、短路与 reader 调用计数已验证；
- [x] shared as-of、UTC 规范化、reader 阻塞后不重读 clock 已验证；
- [x] stale/future/mismatch/not-found/unavailable/unknown failure 返回 zero aggregate；
- [x] pre-cancel、clock 后取消、两个 reader 后取消的 caller 优先级已验证；
- [x] nil/typed-nil/zero/partial 配置在 I/O 前失败关闭；
- [x] error public class 与显式 `Cause()` 隔离、字符串脱敏已验证；
- [x] trace 顺序、最小字段、实际节点和返回副本已验证；
- [x] 64-worker、Participation race、20 轮 shuffle 已验证；
- [x] atomic coverage 与两组 10 秒 fuzz 已实际运行并记录精确结果；
- [x] 从第 25 节 tip `8b2f3a6` 到完整证据 HEAD `c46410e` 的 runtime negative diff 已验证为空；
- [x] 未伪造 adapter/API/Compose/browser E2E；
- [x] 本轮唯一 task-only coverage profile 已精确解析、删除并确认不存在；共享 Go caches 保留；
- [x] `eac4b92` 已最小修复规格与 ADR 的两个 EOF 多余空行，并由最终 diff-check 证明关闭；
- [x] 课程、API、设计手记、面试问答、产品/架构当前态、章节状态与全局索引已完成；
- [x] 独立 `go run ./cmd/doccheck` 已 exit 0；
- [x] 最终 `make verify` 已 exit 0，包含 Go vet/test/doccheck 与前端 19 files / 152 tests、typecheck、Vite build；
- [x] 全仓 `go test -race -count=1 ./...` 已 exit 0；
- [x] 最终 `git diff --check` 已 exit 0；
- [x] 最终 runtime negative diff、adapter 目录与 participation migration 反证已复核；
- [x] 构建前 `web/dist` 不存在；构建生成的 6 个文件已逐个枚举，随后精确删除目录并确认不存在；
- [x] 最终清理后 coverage profile 仍不存在，共享 Go caches 保留。

根代理的最后交接动作是创建并推送“包含本 QA/台账的冻结提交”，再以实际远端 tip 冻结第 26 节，并仅在该推送确认后把 `codex/complete-implementation` fast-forward 到同一 tip。该动作不需要、也不允许本文预写一个虚构 commit SHA。
