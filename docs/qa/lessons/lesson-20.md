# 第 20 节 QA：无偏加权 Award 选择与密码学随机源

- **对应章节：** [实现最简单概率抽奖](../../course/part-03/lesson-20-lottery-weighted-selection.md)
- **API 记录：** [第 20 节 API 边界](../../api/lessons/lesson-20.md)
- **设计推导：** [第 20 节第一性原理设计手记](../../design-thinking/lessons/lesson-20.md)
- **面试复盘：** [第 20 节面试问答](../../interview/lessons/lesson-20.md)
- **长期决策：** [ADR-0017](../../decisions/ADR-0017-lottery-weighted-selection.md)
- **分支：** `codex/lesson-20-lottery-weighted-algorithm`
- **实现提交：** `db679cf`（`feat: add unbiased weighted award selection`）
- **语义校准提交：** `f2475fa`（澄清 Go 1.26 默认 Reader 的不可恢复失败与可返回 reader error 的边界；不改变运行行为）
- **文档内容提交：** `6f08b80`（课程、API、QA、ADR、第一性原理设计手记、面试问答与全局索引）
- **完整学习检查点：** 以 `origin/codex/lesson-20-lottery-weighted-algorithm` 的最终 tip 为准
- **日期：** 2026-08-29

> 本记录验收的是“合法 Strategy 快照如何在内存中选择一个 Award”，不是一次可追踪、可重试、可发奖的 Draw。没有 HTTP 请求、DrawID、最终结果表、幂等键、库存、资格、次数、权益发放或 Redis；多候选重复调用会执行新的临时选择，结果不保证相同，跨调用相关性由 source 契约决定；单候选则确定返回唯一 Award。两者都不能外推为 `INV-03` 已满足。

## 1. 验收范围

本节验收以下事实：

1. `WeightedSelector` 只消费已经合法的 `domain.Strategy`，不读取 Repository；
2. 多候选 Strategy 向 consumer-owned `BoundedRandomSource` 请求一个均匀的 `[0,totalWeight)` 整数位置；
3. 选择器以“当前位置小于当前权重，否则减去权重”的线性扫描映射 Award，不使用浮点、`total+1`、有符号整数或 `% total`；
4. 对合法均匀位置，每个 Award 所占整数位置数量精确等于它的 Weight；
5. 总权重等于 `math.MaxUint64` 时，首尾和相邻 bucket 仍可无溢出命中；
6. 单 Award Strategy 是确定性配置，合法时直接返回且不调用随机源；
7. `no_reward` 是成功选择的完整 Award，不是错误或空结果；
8. 生产 adapter 使用标准库 `crypto/rand.Int`，支持完整 `uint64` 上界并保留错误；
9. 未配置、非法 Strategy、随机源失败、随机源越界和内部映射不变量破坏具有不同稳定错误类；
10. 默认 `CryptoSource` 与共享 `WeightedSelector` 的并发路径由 race detector 实际执行；
11. 纯映射与密码学随机源分别有本机微基准，不把它们外推为产品 SLO。

本节不验收：

- Lottery HTTP API、JSON DTO、鉴权、限流或状态码；
- 一次请求只有一个最终结果、超时重试幂等或 Commit unknown；
- Strategy 更新、版本和历史 Draw 对配置快照的引用；
- 库存扣减、不放回抽样、每人中奖唯一性或预算；
- Benefit 发放、MQ、Outbox、补偿或对账；
- 运营配置没有被篡改、监管意义公平或公开可验证随机；
- 生产吞吐、端到端延迟、故障率或有限样本一定贴近理论比例；
- Redis、前缀和、二分、Alias Method 或其他预计算优化。

## 2. 验收环境

| 维度 | 实际环境 |
| --- | --- |
| 宿主机 | macOS，Apple Silicon，Apple M2 Pro |
| 日期/时区 | 2026-08-29，Asia/Shanghai |
| Go | `go1.26.6 darwin/arm64` |
| 生产随机能力 | Go 标准库 `crypto/rand.Int` + `crypto/rand.Reader` |
| 新第三方依赖 | 无；`go.mod` / `go.sum` 未修改 |
| 新配置/数据库/权限 | 无；未新增环境变量、Migration、SQL 或 grant |
| 产品运行态 | Selector 尚未装配进 `growth-api`，Compose 只作既有链路回归 |

实现没有为了本节顺手升级工具链。调研时官方已有后续 Go 补丁/大版本，但本节需要把算法变量和依赖升级变量分开；工具链升级必须按 [ADR-0008](../../decisions/ADR-0008-supported-go-toolchain-baseline.md) 独立复验，而不是夹带在概率算法提交里。

## 3. 证据矩阵

| 风险命题 | 主要证据 | 已证伪的反例 | 仍不能证明 |
| --- | --- | --- | --- |
| bucket 有缺口、重叠或 off-by-one | 小总权重全空间穷举 | `2:3:5` 的 0～9 每点唯一命中，计数精确为 2/3/5 | 随机源本身是否均匀 |
| 相对权重缩放改变比例 | `1:3` 与 `100:300` 全空间计数 | 两组都精确得到 1/4 与 3/4 的位置占比 | 两份配置在审计上是同一事实 |
| `MaxUint64` 引发加法、有符号或上界溢出 | 多候选最大总权重边界表 | 首点、末点与最后三个相邻 bucket 全部命中预期 Award | 任意内存破坏都能自愈 |
| 随机 adapter 偷用 `% upper` | `upper=3` 脚本 Reader | 先输入 3 再输入 2，3 被拒绝且最终返回 2；取模会错误返回 0 | OS 熵质量由有限样本证明 |
| adapter 只支持低位 uint64 | Max 上界高端点测试 | 读取 8 个 `0xff...fe` 字节并返回 `MaxUint64-1` | 所有未来 Go 版本内部实现相同 |
| 单候选被无意义的熵故障拖垮 | 1、400、Max 三个权重的短路测试 | 随机源预设失败但调用次数为 0，仍返回唯一 Award | Draw 审计已经存在 |
| 技术失败被伪装成未中奖 | 可返回错误的注入 source 与 no_reward 分层测试 | source error 返回零 Award；no_reward 返回完整 Award + nil | Go 1.26 默认 Reader 的不可恢复失败可由这条 error path 捕获 |
| 错误泄露 entropy/内部细节 | stable class + wrapped cause 测试 | `Error()` 只显示语义类，`errors.Is` 仍找到 cause | 上层日志一定正确脱敏 |
| 共享实例产生 data race | 默认 CryptoSource 与 Selector 组合并发测试 + `-race` | 同一 source、selector、strategy 被 32 goroutine 反复调用未报告竞态 | 任意第三方 source 都并发安全 |
| 线性扫描偷偷复制 Awards | domain 私有 slice + benchmark | 1/10/100/1000 最坏路径均为 0 B/op、0 allocs/op | 端到端 API 零分配 |

## 4. 数学映射证据

生产算法的核心控制流是：

```text
ticket ∈ [0, TotalWeight)
for award in canonical AwardID order:
    if ticket < award.Weight:
        return award
    ticket -= award.Weight
```

[`weighted_selector_test.go`](../../../internal/lottery/domain/weighted_selector_test.go) 对乱序构造、规范顺序为 `10,20,30`、权重为 `2,3,5` 的 Strategy 穷举全部 10 个 ticket：

```text
ticket 0,1       -> Award 10
ticket 2,3,4     -> Award 20
ticket 5,6,7,8,9 -> Award 30
```

因此边界 `ticket == previous weight` 会进入下一项，最后合法位置 `total-1` 会命中最后一项。测试还分别对 `1:3`、`100:300` 和 `2:3:5` 穷举完整区间并计数，证明在“输入位置均匀”的前提下，mapper 没有再引入偏差。

这类确定性穷举比“随机抽一百万次看频率”更适合作为常规 CI 证据：它没有采样波动，可以直接发现空洞、重叠和 `<=` 错误。但它只证明位置到 Award 的映射，不证明提供位置的随机源均匀或不可预测。

## 5. 随机源证据与边界

[`CryptoSource.Uint64N`](../../../internal/lottery/adapter/randomsource/crypto.go) 使用：

```go
maximum := new(big.Int).SetUint64(upper)
value, err := cryptorand.Int(reader, maximum)
```

关键边界：

- `SetUint64` 不会像 `big.NewInt(int64(upper))` 一样在 `MaxInt64` 以上改变值；
- `crypto/rand.Int` 的标准库契约是均匀返回 `[0,max)`；
- `upper == 0` 在 adapter 内先返回稳定错误，不触发标准库 panic；
- Selector 对任何实现仍做 `value < upper` 防御检查，越界即失败关闭，不取模修复；
- 默认 Reader 可并发共享，但这个性质不自动传递给未来自定义 source；
- CSPRNG 的不可预测性不等于业务公平、幂等、库存正确或公开可验证。

Go 1.26.6 默认 Reader 的底层随机能力失败是不可恢复的进程级事件，不会返回 `ErrEntropyUnavailable`，也不能由 HTTP panic recovery 兜住；该错误类与 wrapped cause 由包内注入 reader 验证，服务接入后还需监控进程退出、runtime 与平台信号。`NewCryptoSource` 在构造时捕获可被进程内代码替换的全局 `crypto/rand.Reader`，当前仓库没有重写它，但最终 composition 与供应链审查不能仅凭 adapter 类型认定来源可信。

脚本 Reader 的 `3 → 2` 用例验证当前锁定 Go 工具链的拒绝路径会丢弃非法候选 3 后再读 2；高端点用例验证完整 64 bit 可以到达 `MaxUint64-1`。真实 Reader 的 64 次范围探针只证明结果未越界，不被写成统计均匀性证明。均匀性的外部事实仍以 Go 官方契约和源码为依据。

## 6. `MaxUint64` 与减法桶

领域允许总权重恰好为 `math.MaxUint64`。本节没有偷偷把契约收窄为 `MaxInt64` 或 `2^53`，也没有计算不存在于 `uint64` 的 `MaxUint64+1`。

测试覆盖：

1. 权重 `[1, MaxUint64-1]`，ticket 0 命中第一项；
2. 同一权重，ticket `MaxUint64-1` 命中第二项；
3. 权重 `[MaxUint64-2, 1, 1]`，ticket `Max-3`、`Max-2`、`Max-1` 分别命中第一、第二、第三项；
4. CryptoSource 的 upper 为 Max，脚本输入到达最大合法返回值 `Max-1`。

扫描只在已经证明 `ticket >= weight` 后执行减法，所以不会下溢；总权重在 Strategy 构造时已经做加法溢出检查，Selector 不再复制另一套求和逻辑。

## 7. 单候选与 `no_reward`

单 Award Strategy 的概率恒为 1。最终实现选择直接返回唯一 Award，不调用随机源，理由是：

- 没有需要随机决定的分支；
- 权重 1、400 或 Max 在概率语义上都确定；
- 调用随机源会让数学等价配置拥有不同的故障与成本语义；
- 本节不保存随机点，因此调用熵源不会形成任何审计证据。

短路前仍检查 Strategy 基本状态，并校验唯一 Award Weight 与声明总权重一致；同 package 手工制造的不一致内部状态会返回 invariant violation，而不是被快捷路径掩盖。

`AwardOutcomeNoReward` 则无论单候选还是多候选都仍是成功结果。它和随机源故障必须分别统计：前者是业务配置中的合法 outcome，后者表示本次选择根本没有返回 Award。

## 8. 错误、安全字符串与 typed-nil

稳定选择错误类：

| 类别 | 含义 | 当前安全动作 |
| --- | --- | --- |
| `ErrSelectorNotConfigured` | nil/typed-nil source 或零值 selector | 修正 composition，不重试业务输入 |
| `ErrSelectionStrategyInvalid` | 零值或不可用 Strategy | 修复上游构造/恢复边界 |
| `ErrRandomSourceFailure` | 多候选选择取熵失败 | 本次没有 Award；上层显式决定是否重试 |
| `ErrRandomSourceContractViolation` | source 返回 `value >= upper` | 隔离错误 adapter，禁止 `%` 修复 |
| `ErrSelectionInvariantViolation` | 合法 ticket 无法映射或内部总权重不一致 | 视为内部缺陷并告警 |

`SelectionError.Error()` 不拼接 source cause、随机字节或内部 Strategy 细节；`Unwrap` 保留可信诊断链。Adapter 自己的 `SourceError` 也只渲染稳定类。未来 HTTP 层只能消费领域/application 语义，不应把 adapter error 文本直接回给客户端。

Go interface 的 typed-nil 不等于 nil interface。构造器用局部反射检查全部 nil-able Kind，使 `var source *recordingSource = nil` 在 composition 阶段稳定返回 not-configured，而不是在热路径调用方法后 panic。这项反射只发生在构造时，不在每次 Select 中执行。

## 9. 并发证据

`WeightedSelector` 自身只有一个只读 interface 字段，Strategy 对私有 Award slice 拥有所有权；但整体并发安全仍取决于 source。

测试分两层：

1. 共享一个无状态 constant source、一个 Selector 与一个 Strategy，32 goroutine 并发选择；
2. 共享真实默认 `CryptoSource`、Selector 与 Strategy，32 goroutine 并发执行；
3. 全 `internal/lottery/...` 和全仓由 `go test -race -count=1` 实际运行。

这证明当前生产组合的已执行路径没有被 race detector 发现竞态。它不能证明任意未来 `math/rand/v2.Rand` 实例或带游标 fake 可无锁共享；端口注释已经要求共享 source 自己提供并发安全。

## 10. 性能微基准

执行命令：

```bash
go test -run '^$' -bench 'BenchmarkWeightedSelectorWorstCase' -benchmem -count=3 ./internal/lottery/domain
go test -run '^$' -bench 'BenchmarkCryptoSourceUint64N' -benchmem -count=3 ./internal/lottery/adapter/randomsource
```

Apple M2 Pro、Go 1.26.6 本机结果：

| 基准 | 3 次范围 | 分配 |
| --- | --- | --- |
| 单候选确定性短路 | 5.117～5.213 ns/op | 0 B/op，0 allocs/op |
| 10 Award 最坏路径 | 11.71～11.80 ns/op | 0 B/op，0 allocs/op |
| 100 Award 最坏路径 | 69.51～72.76 ns/op | 0 B/op，0 allocs/op |
| 1000 Award 最坏路径 | 658.4～663.1 ns/op | 0 B/op，0 allocs/op |
| CryptoSource upper=10,000 | 178.0～181.2 ns/op | 56 B/op，4 allocs/op |

这些数字说明当前实现的线性增长形状与“domain 内直接读取私有 slice，因此 mapper 不复制 Awards”一致，也把 `math/big` / crypto 成本与纯映射分开。它们不含 Repository、MySQL、HTTP、JSON、鉴权、幂等、库存或结果持久化，不能写成 Lottery 10,000 RPS 或 P99 已达标。NFR 的产品抽奖目标仍保持未测量。

## 11. 实际执行的质量门禁

实现收尾实际执行：

```bash
go test -count=1 ./internal/lottery/domain ./internal/lottery/adapter/randomsource
go test -shuffle=on -count=10 ./internal/lottery/domain ./internal/lottery/adapter/randomsource
go test -race -count=1 ./internal/lottery/...
go test -race -count=1 ./...
go vet ./...
make verify
```

结果：

- targeted 普通、重复 shuffle 与 race 全部通过；
- 全仓 Go package 普通测试、race 与 vet 全部通过；
- 文档门禁通过；
- 前端 4 个测试文件共 34 个用例通过；
- TypeScript typecheck 与 Vite production build 通过；
- Vite 主 bundle 大于 500 kB 的 warning 仍为非阻断容量信号，没有调高阈值伪装解决。

### 11.1 文档收束后的最终复验

交叉审查和事实校准完成后，又从 `web/dist` 不存在的干净交付态执行：

```bash
make verify
go test -race -count=1 ./...
go test -shuffle=on -count=10 ./internal/lottery/domain ./internal/lottery/adapter/randomsource
shellcheck $(rg --files -g '*.sh')
make compose-config compose-smoke
git diff --check
go run ./cmd/doccheck
```

最终结果：

- `make verify` 再次通过；Go vet/普通测试、文档检查、前端 4 文件 34 用例、TypeScript typecheck 和 production build 全绿；
- 全仓 race 使用 `-count=1` 重新执行并通过，目标包 shuffle 十轮通过；
- 仓库全部 shell 脚本通过 ShellCheck；
- Compose config 与 smoke 通过：MySQL/API/Redis/Web 全部 healthy，Migration clean latest 2，应用仍只有两表精确 `SELECT, INSERT`，两个探针、SPA、关联 404 与唯一 `127.0.0.1:8088` 暴露均未回归；
- 最终 Markdown/相对链接/章节证据检查和 `git diff --check` 通过；
- `f2475fa` 仅校准源码注释，提交前目标包普通测试与文档检查通过，并已推送到同名远端分支。

Compose 回归仍只证明第 19 节运行镜像与既有工程链路。镜像标签显示 `lesson-19` 是准确事实：第 20 节 Selector 尚未装配进产品二进制，不能为了编号好看重建并宣称 HTTP 已覆盖算法。

## 12. 验证中真实暴露并修复的问题

### 12.1 Benchmark helper 类型过窄

首次 targeted build 失败：既有 `mustAward` 只接受 `*testing.T`，新 benchmark 传入 `*testing.B` 无法编译。修复不是复制一份 benchmark-only helper，而是收窄为测试真正需要的 `Helper + Fatalf` 小接口，使单元测试和 benchmark 共享同一构造失败规则。

### 12.2 单候选短路改变了 invariant 测试路径

引入单候选短路后，最初用“一个 Award、声明 total 大于真实 weight”制造映射空洞的测试没有进入扫描，因而意外得到 nil error。这个失败暴露了两件事：

1. 测试数据必须真的到达它声称验证的控制流；
2. 确定性快捷路径也应检查自己依赖的内部总权重关系。

最终实现为单候选增加 Weight 与 TotalWeight 相等检查，并另用两 Award/空洞 ticket 验证循环后的不可达 invariant 分支。保留这段失败记录，是为了避免把测试修正后的绿色误写成“一次就设计正确”。

### 12.3 Go 1.26 默认 Reader 不是普通 error 通道

文档交叉审查发现，最初的源码注释和部分设计表述容易让人误以为“默认 OS 随机能力失败一定会返回 `ErrEntropyUnavailable`”。对当前锁定的 Go 1.26.6，这不准确：默认 `crypto/rand.Reader` 的底层失败采用不可恢复的进程级语义，HTTP panic recovery 也不能把它变成业务 error。

因此 `f2475fa` 与本节文档统一校准为：

1. `SourceError` / `ErrEntropyUnavailable` 证明可返回错误的注入 reader 和未来 adapter 会失败关闭；
2. 默认 Reader 的不可恢复失败必须依靠进程退出、runtime 与平台告警发现；
3. `NewCryptoSource` 捕获的全局 `crypto/rand.Reader` 可被进程内代码替换，最终 composition 还要审查初始化顺序、依赖和供应链，不能把 `CryptoSource` 类型名当作熵来源证明。

这次校准没有改变算法或 error 行为，却改变了正确的运行手册与面试答案。它说明“函数签名有 error”不能代替对锁定工具链具体默认实现的核查。

## 13. Compose 回归边界

本节没有修改数据库、配置、镜像拓扑、前端或业务路由。默认 Compose smoke 只验证第 19 节已存在的 MySQL/API/Redis/Web 工程链路没有回归；由于 Selector 尚未被 `growth-api` 引用，它不能经 HTTP 触发，也不会被 Compose smoke 证明。

因此本节不会为了编号整齐而修改 MySQL 集成 opt-in 名称、Migration version、grants 或运行镜像标签。第 21 节真正发生 composition 时，才需要把 Repository pool、Selector、use case 和 transport 作为一个新运行时事实共同验收。

## 14. 清理与保留

本节实现阶段 `make verify` 前确认 `web/dist` 不存在；验证新生成的 756 KiB 构建目录已整体移动到本机废纸篓 `GrowthOS-Go-lesson20-web-dist-implementation-20260829-1920`，可恢复。文档收束后的最终 `make verify` 又从无 `web/dist` 状态生成 756 KiB，并移动到可恢复的 `GrowthOS-Go-lesson20-web-dist-final-20260829-1942`。仓库中的 `web/dist` 最终再次不存在。没有删除源码、依赖、凭据或用户数据。

明确保留：

- 默认 `growthos` Compose 开发栈，便于后续回归与第 21 节装配；
- `growthos_mysql_data` 与 `growthos_mysql_socket` named volumes；
- 被 Git 忽略的本地 Compose Secret；
- Go module、pnpm 依赖与可复用构建缓存；
- 第 19 节历史验证产物在废纸篓中的可恢复副本。

本机没有配置课程同步所需的 `VAULT` 环境变量，因此没有执行 Obsidian 同步；Git 仓库文档是本节唯一已验证的文档事实源。

## 15. 剩余风险

1. source 均匀是端口信任前提；仓库当前只提供一个审查过但尚未装配进产品进程的 `CryptoSource` 候选，接口本身无法在类型系统证明概率分布，全局 Reader 被替换也不能靠类型名发现。
2. Go 1.26 默认 Reader 的底层失败不可恢复，不能只盯 `SourceError` 指标；进程/runtime/platform 告警要到真实装配时共同验收。
3. 默认 crypto Reader 的不可预测性不证明运营权重未改、参与机会相同、法规合规或结果不可抵赖。
4. Selector 丢弃 ticket 且不持久化结果；多候选 HTTP 超时后重试可能选到另一个 Award，单候选也仍没有最终 Draw 事实。
5. AwardID 规范顺序不改变每项概率，却固定同一 ticket 对应哪项；新增低 ID Award 或改顺序会改变历史映射。
6. 当前是有放回且不保存跨调用抽样状态的选择；端口不承诺跨调用统计独立性，该性质取决于 source，也不解决库存、预算、唯一中奖用户或批量无放回抽样。
7. 线性扫描只由本机微基准支持当前选择；没有真实 Award 数量分布和热 Strategy 复用证据。
8. `CryptoSource` 当前每次有 4 次分配；没有证据表明它是业务瓶颈，因此未自写随机算法或做对象池。
9. Go 版本升级可能改变 crypto 实现细节；拒绝路径脚本与安全假设需要随工具链复验。
10. 错误 cause 尚未由业务 service 转成低基数指标；第 21 节不能直接记录随机原始字节或把 cause 返回客户端。
11. 当前前端 Lottery 页面仍使用 Mock/`Math.random()` 且存在超前能力文案；第 22 节必须切真实接口并校准，不得把它当成本节演示。

## 16. 验收结论

实现提交 `db679cf`、语义校准提交 `f2475fa` 与文档内容提交 `6f08b80` 已推送到 `origin/codex/lesson-20-lottery-weighted-algorithm`。当前可以准确陈述：**GrowthOS 已实现一个支持完整 `uint64`、以密码学 bounded source 为生产适配器、对合法 Strategy 做无偏权重映射的内存选择器。**

当前不能陈述：**已经实现可上线的抽奖接口、高并发抽奖、最终结果幂等、库存一致性、权益发放或监管公平。** 文档内容提交已登记到[课程分支检查点](../../course/branch-checkpoints.md)，完整章节以同名远端分支最终 tip 为准。
