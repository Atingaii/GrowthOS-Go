# 第 17 节 QA：Lottery 领域对象与不变量验收记录

- **对应章节：** [最简单随机抽奖需要什么对象](../../course/part-03/lesson-17-lottery-domain-objects.md)
- **API 记录：** [第 17 节 API 边界](../../api/lessons/lesson-17.md)
- **设计推导：** [第 17 节第一性原理设计手记](../../design-thinking/lessons/lesson-17.md)
- **面试复盘：** [第 17 节面试问答](../../interview/lessons/lesson-17.md)
- **长期决策：** [ADR-0013：Lottery 最小领域模型](../../decisions/ADR-0013-lottery-domain-model.md)
- **分支：** `codex/lesson-17-lottery-domain-objects`
- **直接起点：** `f9cdd3c`（第 16 节最终检查点）
- **实现提交：** `0b59217`
- **完整检查点：** 以包含本记录的同名远端分支最终 tip 为准
- **验收日期：** 2026-08-29
- **验收结果：** 通过

> 本记录只证明第 17 节纯领域对象的构造不变量、集合所有权和依赖边界。它不证明随机算法公平，不证明一次业务请求只有一个最终结果，也不证明 MySQL、Repository、HTTP、React、Redis、库存、用户资格或权益发放已经完成。

---

## 1. 本节验收命题

第 16 节完成了可重复启动的本地系统栈，但业务层仍为空。本节需要回答的不是“接口能不能返回一个随机奖品”，而是：在不借助数据库、HTTP 或前端类型的情况下，最小 Lottery 配置能否表达一个完整且不会自相矛盾的选择空间。

验收分成五个命题：

1. `Strategy` 是否能作为聚合根拥有一组 `Award`；
2. 每个对象是否只能通过守住不变量的构造路径得到；
3. 权重、未中奖、名称和集合顺序是否有明确且可测试的语义；
4. 调用方能否绕过聚合修改内部切片，或靠数据库返回顺序改变领域状态；
5. 包是否保持纯领域边界，没有提前引入下一节以后的基础设施和用例能力。

以下内容不是本节完成条件：

- 创建 `strategy` 或 `strategy_award` 表；
- 从 MySQL 读取或写入策略；
- 根据权重执行随机选择；
- 保存抽奖请求或最终结果；
- 暴露 Lottery HTTP API；
- 让 React 抽奖页调用真实后端；
- 使用 Redis 缓存、锁或库存；
- 校验活动时间、用户资格、参与次数或库存；
- 交付积分、优惠券或实物权益。

这些边界同时是负向验收项：如果代码中出现上述能力，本节反而越过了课程切片。

## 2. 环境记录

### 2.1 宿主工具链

| 项目 | 实测值 |
| --- | --- |
| 操作系统 | macOS 26.5.1，Build 25F80 |
| CPU | arm64，Apple Silicon |
| Go | go1.26.6 darwin/arm64 |
| Node.js | v24.19.0 |
| 宿主 pnpm | 11.22.0 |
| Go module | `github.com/Atingaii/GrowthOS-Go` |

复查命令：

```bash
sw_vers
uname -m
go version
node --version
pnpm --version
```

Node.js 与 pnpm 只在最终统一门禁中验证既有前端没有回归；第 17 节的实现包本身不依赖 Node、浏览器或前端构建产物。

### 2.2 运行环境边界

第 17 节没有新增进程、端口、容器、环境变量、Secret、数据库连接、Redis 连接或外部网络调用。因此没有为了“显得完整”重复做 Compose 故障注入、浏览器截图或 M0 压测。

第 16 节保留的 Compose 栈不构成本节领域模型的运行依赖。即使 Docker Desktop、MySQL 和 Redis全部停止，本节单元测试也应得到相同结果。

## 3. 实现检查点

### 3.1 独立实现提交

实现被固定为一笔可单独学习的提交：

```text
0b59217 feat: model lottery strategy domain objects
```

远端核查结果：

```text
0b59217ef8c9b8cbb681009016d3528babd98ae4 refs/heads/codex/lesson-17-lottery-domain-objects
```

该提交从 `f9cdd3c` 线性前进，没有修改 `main`。学习者可以先比较：

```bash
git diff f9cdd3c..0b59217 -- internal/lottery
```

### 3.2 文件清单

| 文件 | 验收职责 |
| --- | --- |
| [`doc.go`](../../../internal/lottery/domain/doc.go) | 声明纯领域包边界 |
| [`errors.go`](../../../internal/lottery/domain/errors.go) | 可用 `errors.Is` 判断的领域构造错误 |
| [`name.go`](../../../internal/lottery/domain/name.go) | 名称规范化、UTF-8、控制字符和 128 rune 上限 |
| [`award.go`](../../../internal/lottery/domain/award.go) | Award、强类型 ID/Weight、reward/no_reward |
| [`strategy.go`](../../../internal/lottery/domain/strategy.go) | Strategy 聚合、不变量、总权重、规范顺序和查询 |
| [`award_test.go`](../../../internal/lottery/domain/award_test.go) | Award 正负向和 Unicode 边界测试 |
| [`strategy_test.go`](../../../internal/lottery/domain/strategy_test.go) | 聚合、溢出、所有权、顺序和查找测试 |

原有 `internal/lottery/.gitkeep` 被删除，因为该目录已经有真实职责，不再是能力占位。

## 4. 对象与统一语言验收

### 4.1 `Strategy`

`Strategy` 被验证为 Lottery 内的可复用决策配置聚合根，当前拥有：

- 正整数 `StrategyID`；
- 规范化后的运营名称；
- 至少一个合法 `Award`；
- 防御性复制后的奖项集合；
- 经溢出检查的派生总权重。

它没有活动时间窗、活动状态、用户、库存、权益到账状态或 HTTP 字段。由此保持 `Strategy != Activity`。

### 4.2 `Award`

`Award` 是 Strategy 内具有身份的可选择结果，当前拥有：

- 正整数 `AwardID`；
- 规范化后的展示名称；
- 正整数相对 `Weight`；
- 闭合的 `AwardOutcome`。

它不包含 points/coupon/physical 等具体 Benefit 类型，不包含库存和发放状态，也不是已经持久化的抽奖结果。

### 4.3 `AwardOutcome`

允许值只有：

| 值 | 含义 | 不等于 |
| --- | --- | --- |
| `reward` | 这次选择表达一个后续可交给 Benefit 的奖励意图 | 权益已经到账 |
| `no_reward` | 抽奖可以正常完成但没有奖励 | 系统错误、空结果、超时未知 |

测试拒绝任意其他字符串，例如把 `coupon` 直接塞进 Lottery outcome。具体奖励类型属于后续统一 Reward/Benefit 模型，而不是本节枚举。

## 5. 名称契约验收

Strategy 与 Award 共用同一字符级约束：

1. 输入必须是有效 UTF-8；
2. 先移除首尾 Unicode 空白；
3. 移除后不得为空；
4. 名称内部不得包含 Unicode 控制字符；
5. 长度不得超过 128 个 rune；
6. 恰好 128 个中文 rune 合法；
7. 存储的是规范化值，而不是原始带空白输入。

### 5.1 为什么按 rune 而不是 byte

按 byte 限制会让相同可见长度的中文和 ASCII 获得完全不同的业务上限。按 rune 更接近 MySQL `utf8mb4 VARCHAR(128)` 与 UI 字符级约束，也能让第 18 节从领域规则推导字段，而不是反过来由表结构决定业务对象。

该证据不能证明 128 rune 一定能在所有 UI 中等宽显示。rune 也不等于用户感知的字素簇，组合字符和 emoji ZWJ 序列仍可能由多个 rune 组成；这项限制已保留为后续契约风险。

### 5.2 已执行边界用例

| 用例 | 期望 | 结果 |
| --- | --- | --- |
| 首尾普通空格 | 规范化后保存 | 通过 |
| 只有空格、Tab、换行 | required 错误 | 通过 |
| 非法 UTF-8 byte `0xff` | invalid 错误 | 通过 |
| 名称内部换行 | invalid 错误 | 通过 |
| 128 个中文字符 | 成功 | 通过 |
| 129 个中文字符 | too long 错误 | 通过 |

## 6. 权重与数值安全验收

### 6.1 相对权重

`Weight` 是 `uint64` 的强类型正整数，表达比例而不是百分比。测试确认：

- `2:5` 合法，总权重为 7；
- 权重总和不要求等于 100、1000、10000 或 1,000,000；
- 单个权重为 0 会被拒绝；
- Go 类型本身排除了负数、NaN 和正负无穷；
- `1:3` 与 `100:300` 在未来加权选择中应表达同一分布，但本节没有执行随机抽样。

### 6.2 溢出

聚合逐项执行以下等价检查：

```text
nextWeight > MaxUint64 - currentTotal
```

验收覆盖：

- 单个 Award 权重恰好为 `math.MaxUint64` 时构造成功；
- 在 `math.MaxUint64` 后再增加权重 1 时返回 `ErrTotalWeightOverflow`；
- 不依赖发生 wraparound 后再观察异常值。

这个检查只保证配置求和不会溢出。第 20 节仍必须保证随机区间、边界开闭和随机源到权重区间的映射无偏。

## 7. 聚合不变量验收

### 7.1 构造前置条件

| 不变量 | 失败错误 | 测试结果 |
| --- | --- | --- |
| Strategy ID 大于 0 | `ErrStrategyIDRequired` | 通过 |
| Strategy 名称合法 | required/invalid/too long | 通过 |
| Strategy 至少一个 Award | `ErrStrategyAwardsRequired` | 通过 |
| 每个 Award 都重新验证 | 对应 Award 错误 | 通过 |
| 同一 Strategy 内 Award ID 唯一 | `ErrDuplicateAwardID` | 通过 |
| 总权重可在 uint64 表达 | `ErrTotalWeightOverflow` | 通过 |
| Award ID 大于 0 | `ErrAwardIDRequired` | 通过 |
| Award 名称合法 | required/invalid/too long | 通过 |
| Award 权重大于 0 | `ErrAwardWeightRequired` | 通过 |
| Outcome 属于闭合集合 | `ErrAwardOutcomeInvalid` | 通过 |

`Award` 虽然字段私有，Go 的导出类型仍有可构造的零值。`NewStrategy` 因此不会相信传入 Award 一定来自 `NewAward`，而会再次执行 `award.validate()`。`[]Award{{}}` 的负向测试确认零值不能潜入合法聚合。

### 7.2 有意允许的配置

以下不是结构错误：

- 只有一个 Award 的确定性策略；
- 多个 Award 展示名称相同但 ID 不同；
- 所有 Award 都是 `no_reward`；
- 总权重不是固定基数。

允许这些配置不等于建议运营直接发布。是否至少存在一个 reward、是否需要两个以上候选、是否限制中奖率属于应用政策或后续规则；当前证据不足以把它们永久写成领域结构不变量。

## 8. 集合所有权与顺序验收

### 8.1 输入切片复制

测试先用 `[]Award{first, second}` 构造 Strategy，再修改调用方原切片第一个元素。聚合返回的第一个 Award 仍是 `first`，证明构造器没有保留调用方 slice header/底层数组作为内部可变状态。

### 8.2 输出切片复制

测试取得 `strategy.Awards()`，修改返回切片，再次读取聚合。内部集合未变化，证明 getter 没有把内部底层数组暴露给调用方。

### 8.3 规范顺序

调用方按 `30, 10, 20` 传入 Award，聚合读取结果固定为 `10, 20, 30`。因此：

- 调用方传入顺序不是领域事实；
- 第 19 节 Repository 即使收到不同数据库行迭代顺序，也能重建相同领域表示；
- 第 20 节的累积区间不会被未声明的数据库顺序改变；
- 如未来运营需要拖拽展示顺序，必须显式增加新的领域字段，而不能偷用 slice 当前顺序。

当前 Award 元素只有整数、字符串和枚举值，浅复制足够。如果以后 Award 持有 map、slice 或 pointer，现有复制只保护第一层，必须重新评估深复制或继续使用不可变值。

## 9. 错误语义验收

领域包返回标准库 error 和可判定 sentinel：

- 测试使用 `errors.Is`，不依赖完整错误文本；
- 重复 Award ID 可包装具体 ID，同时仍能 `errors.Is(err, ErrDuplicateAwardID)`；
- 错误没有 HTTP status、JSON code、Gin context、SQL error 或 Redis error；
- 构造失败返回零值和 error，不返回半有效对象供调用方继续使用。

第 21 节才需要决定领域错误怎样映射为公开 HTTP code。现在提前绑定 400/409 会让领域模型依赖尚未存在的传输契约。

## 10. 测试执行证据

### 10.1 领域测试枚举

```bash
go test -list . ./internal/lottery/domain
```

输出五个顶层测试：

```text
TestNewAward
TestNewStrategy
TestStrategyOwnsAwardCollection
TestStrategyCanonicalizesAwardOrder
TestStrategyAwardLookup
```

### 10.2 单次、覆盖率与事件数

执行：

```bash
go test -count=1 ./internal/lottery/domain
go test -count=1 -cover ./internal/lottery/domain
go test -json -count=1 ./internal/lottery/domain
```

结果：

| 指标 | 实测 |
| --- | --- |
| 包结果 | PASS |
| 顶层测试 | 5 |
| 子测试 | 26 |
| JSON pass test events | 31 |
| statement coverage | 95.8% |
| 失败 | 0 |

Coverage 衡量当前测试执行过的语句比例，不等价于需求覆盖、分支完全性或概率正确性。当前没有随机算法，因此也没有用“跑很多次频率差不多”伪装数学正确性。

### 10.3 Race Detector

执行：

```bash
go test -race -count=10 ./internal/lottery/domain
```

结果为 PASS，10 次均未报告 data race。这里重复运行用于增加当前测试路径的调度变化；Go Race Detector 只能检测实际执行路径中的竞态，不能证明所有未来访问方式绝对无竞态。

防御性复制的确定性测试与 `-race` 解决不同问题：前者证明调用方拿不到内部切片别名，后者观察已执行并发路径。本节对象构造后没有 setter 或后台 goroutine，因此不在对象内加入 Mutex。

### 10.4 Vet 与全 Go 回归

执行：

```bash
go vet ./internal/lottery/domain
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

结果：

- 领域包 vet 无输出，退出码 0；
- 全仓 13 个有测试的 Go package 全部 PASS；
- `migrations` 当前仍正确报告 `[no test files]`；
- 全仓 race 运行全部 PASS；
- 没有因新增 Lottery 包破坏既有 API、Migration、配置或工具测试。

## 11. 依赖与副作用边界

### 11.1 直接 import

执行：

```bash
go list -f '{{range .Imports}}{{println .}}{{end}}' ./internal/lottery/domain
```

实测直接依赖只有：

```text
cmp
errors
fmt
math
slices
strings
unicode
unicode/utf8
```

全部来自 Go 标准库。

### 11.2 禁止边界静态扫描

执行：

```bash
rg -n 'gin|sqlx|mysql|redis|json:|db:|math/rand|crypto/rand|net/http|context\.' \
  internal/lottery/domain --glob '*.go'
```

结果：0 条匹配。由此确认当前源码中没有：

- Gin/HTTP；
- SQL、MySQL、sqlx 或数据库 tag；
- Redis；
- JSON tag/DTO；
- 随机源或抽样算法；
- context、timeout、goroutine 或 I/O 生命周期。

静态扫描不是通用架构证明，但对本节列出的具体禁止依赖给出了可重复的负向证据。

## 12. API、数据库与前端回归边界

### 12.1 HTTP

本节没有新增、修改或删除路由。真实 HTTP 契约仍只有系统探针：

- `GET /health`；
- `GET /ready`。

`StrategyID`、`AwardID`、`Weight` 和 `AwardOutcome` 是 Go 领域类型，不是已发布 JSON Schema。特别是两个 ID 使用 `uint64`，第 21 节不能未经决策直接输出为 JavaScript number；超过 `Number.MAX_SAFE_INTEGER` 会产生精度风险。

### 12.2 数据库

本节没有新增 Migration，`migrations/sql` 仍为空。`000001` 留给第 18 节，由当前对象的不变量、名称长度、结果枚举、归属关系和查询路径共同推导。

### 12.3 前端

现有 `/lottery` 页面仍消费 `mockLotteryPrizes` 并在浏览器端均匀 `Math.random()`。页面文字中提到的 `lottery-service` 与 Redis 分布式锁仍只是 Mock 展示，不是本节实现证据。真实 Lottery 页面留给第 22 节。

因此本节没有执行新的浏览器功能验收；统一 `make verify` 只用来确认既有前端测试、类型检查和构建没有回归。

## 13. 完整质量门禁

在五份章节正文、ADR 和索引全部落盘后执行：

```bash
make verify
git diff --check
go run ./cmd/doccheck
```

最终结果：

| 门禁 | 结果 | 失败数 |
| --- | --- | --- |
| Go format check | 通过 | 0 |
| Go vet | 通过 | 0 |
| Go tests | 通过 | 0 |
| 文档完整性与相对链接 | 通过 | 0 |
| 前端 Vitest | 4 个测试文件、34 个测试通过 | 0 |
| TypeScript typecheck | 通过 | 0 |
| Vite production build | 通过 | 0 |
| Git whitespace check | 通过 | 0 |

Vite 8.0.3 实际完成 2,456 个 module 的 production build；主 JavaScript chunk 为 708.34 kB、gzip 后 210.22 kB。构建继续报告单 chunk 大于 500 kB，这是第 14～16 节已登记的非阻塞优化项，不由纯领域模型章节冒充修复。

## 14. 本节证据能证明什么

当前证据可以证明：

- 当前构造器拒绝已列出的非法对象状态；
- `Strategy` 持有合法 Award 集合并安全计算总权重；
- 输入和输出切片没有共享可修改的底层数组；
- Award 内部顺序按 ID 规范化；
- reward 与正常 no_reward 在领域语义上可区分；
- 名称的 UTF-8、控制字符和 rune 长度边界可重复验证；
- 当前包没有下一节以后的技术依赖；
- 新代码没有破坏当前 Go/React 统一门禁。

## 15. 本节证据不能证明什么

当前证据不能证明：

1. 权重选择算法正确或无偏，因为算法尚不存在；
2. `math/rand/v2` 或 `crypto/rand` 已被正确选择，因为随机边界留给第 20 节；
3. 长期样本频率与配置比例一致；
4. 同一请求重试返回同一抽奖结果；
5. NFR `INV-03`“一次抽奖只有一个最终结果”已经满足；
6. Strategy/Award 能正确写入或重建自 MySQL；
7. 并发更新策略、库存扣减或权益发放安全；
8. HTTP DTO、错误码、鉴权或限流存在；
9. React 页面已真实联调；
10. Redis 缓存、分布式锁或缓存失效正确；
11. 生产吞吐、延迟、可用性、公平性或合规目标已达到；
12. 128 rune 是永远不需要调整的产品限制。

## 16. 剩余风险与后续验证点

| 风险 | 当前处理 | 重新验证章节 |
| --- | --- | --- |
| MySQL 字段、约束与领域规则漂移 | 下一节由领域规则推导 Migration，并做脏数据负向测试 | 第 18 节 |
| Repository 依赖无序行集 | 领域按 AwardID 规范化，Repository 仍需稳定查询和重建测试 | 第 19 节 |
| 随机源或区间映射有偏 | 本节不选算法；后续注入随机边界、做确定性和统计验证 | 第 20 节 |
| uint64 ID 超出 JS 安全整数 | HTTP 契约发布前决定字符串编码或限制范围 | 第 21 节 |
| Mock 页面误导为真实能力 | 文档持续标记；真实 API 与页面完成后移除 Mock 声明 | 第 21～22 节 |
| 规则把 Strategy 变成上帝对象 | 规则升级时重新评估对象和决策引擎边界 | 第 23、26～29 节 |
| 奖项数量没有领域上限 | API body、DB 查询和算法容量有数据后再设边界 | 第 18～21、95 节 |
| shallow copy 遇到复合字段失效 | Award 新增引用型字段时强制重审所有权测试 | 任一模型变更 |
| all-no_reward 策略可能违背运营承诺 | 当前视为结构合法；发布策略出现时由应用政策/治理约束 | 第 23、31 节 |
| rune 不等于字素簇 | UI/国际化需求出现后重评显示和输入限制 | 第 21～22 节 |
| 结果事实与奖励到账被混淆 | 保持 Lottery/Benefit 边界；后续分别保存结果和发放状态 | 第 46～49 节 |

## 17. Obsidian 同步记录

`Definition of Done` 要求在已配置个人 Vault 时执行同步。本机当前没有用户提供的 macOS Vault 绝对路径；历史 QA 中的 `/mnt/e/TencentGo/growthOS` 是 WSL 路径，本机检查不可访问。因此本节未执行 `make docs-sync`，也没有把历史路径猜测映射到用户目录，更没有伪造同步成功。

仓库内 `docs/` 仍是事实源，全部文档已经进入 Git 分支。用户提供明确可写的 macOS Vault 路径后，可执行：

```bash
make docs-sync VAULT=/absolute/path/to/growthOS
```

## 18. 清理记录

本节实现与领域验收没有下载持久文件、创建测试数据库、生成 coverage profile 或启动新容器；协作检查产生的临时日志已由执行代理删除，工作树没有残留。`make verify` 生成了被忽略的 `web/dist`；该目录在门禁前已确认不存在，验证后已经完整移动到 macOS 废纸篓中的 `GrowthOS-Go-lesson17-web-dist-20260829-170304`，仓库路径已不存在。该操作可从废纸篓恢复。

以下资源不会删除：

- 第 16 节保留的 Compose 栈、named volume 与本地 Secret 集合；
- `web/node_modules` 可复用依赖；
- Docker 镜像和构建缓存；
- 全局 `agent-browser` 与 Chrome for Testing；
- 用户原有 MySQL、Redis、RabbitMQ、PostgreSQL 等容器或数据。

## 19. 最终结论

第 17 节通过。当前仓库第一次拥有真实业务领域代码：Lottery 可以构造自洽的 Strategy/Award 配置，并能拒绝空身份、非法名称、零权重、非法 outcome、空集合、重复 Award ID、零值 Award 和总权重溢出；聚合还隔离切片别名并建立稳定 AwardID 顺序。

“领域对象合法”不等于“抽奖系统已经可用”。第 18 节需要把这些不变量转换为第一批正式业务表与可回放 Migration，并继续保留领域模型不依赖数据库结构的方向。
