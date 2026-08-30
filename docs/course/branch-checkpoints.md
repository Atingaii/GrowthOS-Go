# 课程分支检查点

**状态：** 当前

**更新日期：** 2026-08-30

本页把课程章节与可检出的 Git 分支对应起来，方便在 `main` 保持原状的同时，逐节查看实现是怎样演进的。分支记录的是学习检查点，不替代[课程状态台账](status.csv)；章节只有具备正文、API 记录、实际 QA 证据并通过质量门禁后，才能在台账中标记“已完成”。从第 13 节开始，稳定分支还必须包含对应的第一性原理设计手记与面试问答；历史章节将按这一新增规范逐节回填。

## 当前检查点

| 检查点 | 分支 | 起点 | 已确认提交 | 2026-08-30 可用位置 | 学习重点 |
| --- | --- | --- | --- | --- | --- |
| 第 9 节：仓库工程基线 | `codex/lesson-09-repository-baseline` | `main` | `e5516b1` | 本地与 `origin` | 单 Go module、目录职责、文档与统一质量门禁 |
| 第 10 节：模块化单体 | `codex/lesson-10-modular-monolith` | 第 9 节 | `2a8f126` | 本地与 `origin` | 单进程边界、模块依赖规则与拆分条件 |
| 工程基线加固 | `codex/engineering-baseline-hardening` | 第 10 节 | `ce7f8a0` | 本地与 `origin` | module 身份、测试隔离、文档镜像安全与领域占位边界 |
| 第 11 节：Gin HTTP 服务 | `codex/lesson-11-gin-http-service` | 工程基线加固 | 实现 `ade1fad`；验收 tip `4734706` | 本地与 `origin` | Gin 进程、健康接口、HTTP Server 生命周期与优雅关闭 |
| 第 12 节：配置、日志与错误 | `codex/lesson-12-config-logging-errors` | 第 11 节已验收 tip | 实现 `60a6116`；验收 tip `ac9ad0e` | 本地与 `origin` | `GROWTHOS_` 配置、`slog`、请求关联、fault 与 HTTP 错误 |
| 第 13 节：MySQL 与 Migration | `codex/lesson-13-mysql-migrations` | 第 12 节验收 tip `ac9ad0e` | 实现 `b3f5aa7`；加固 `b734463`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 身份隔离、`sqlx` pool、readiness、前向 Migration 与真实 MySQL 验收 |
| 第 15 节：前后端第一次联调 | `codex/lesson-15-first-fullstack-integration` | 累计检查点 `f0cd8e1` | 实现 `7e499cc`；浏览器契约加固 `2283a70`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 同源代理、统一 Fetch、运行时解码、独立探针状态、竞态与真实浏览器故障验收 |
| 第 16 节：Docker Compose 开发环境 | `codex/lesson-16-docker-compose-development` | 第 15 节验收 tip `03ebe56` | 文件凭据 `e746a6f`；日志边界 `52c3add`；Compose 栈 `7aa6c9e`；文档内容 `ad8078c`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 唯一同源入口、网络与身份隔离、一次性迁移、秘密文件、故障演练和 M0 负载门禁 |
| 第 17 节：Lottery 最小领域对象 | `codex/lesson-17-lottery-domain-objects` | 第 16 节最终检查点 `f9cdd3c` | 实现 `0b59217`；文档内容 `792f04e`；完整章节以最终分支 tip 为准 | 本地与 `origin` | Strategy 聚合、Award 候选、整数相对权重、显式 reward/no_reward、领域不变量与适配器边界 |
| 第 18 节：Lottery 首组业务表 | `codex/lesson-18-lottery-schema` | 第 17 节最终检查点 `24e606a` | 实现 `7593aaa`；策略内身份加固 `f74fdf2`；文档内容 `215abd1`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 两表最小 schema、复合身份、迁移故障语义、最小权限授权与真实 MySQL/Compose 验收 |
| 第 19 节：Lottery Strategy 仓储 | `codex/lesson-19-lottery-repository` | 第 18 节最终检查点 `4c06e25` | 实现 `50ac811`；证据加固 `2c420c9`；文档内容 `09556d8`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 窄 application 端口、聚合写事务、只读 RR 快照、恢复校验、语义错误与精确 SELECT/INSERT 权限 |
| 第 20 节：Lottery 加权选择算法 | `codex/lesson-20-lottery-weighted-algorithm` | 第 19 节最终检查点 `7b67d2c` | 实现 `db679cf`；Go 1.26 随机源语义校准 `f2475fa`；文档内容 `6f08b80`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 完整 `uint64` 无偏区间、整数 bucket 映射、单候选短路、失败关闭、随机源端口与并发边界 |
| 第 21 节：Lottery 临时选择 API | `codex/lesson-21-lottery-api` | 第 20 节最终检查点 `ea71640` | 实现 `65e9627`；边界/协议/验收加固 `be41d92`～`7c43456`；文档内容 `90129b6`；完整章节以最终分支 tip 为准 | 本地与 `origin` | development/test feature gate、只读纵向链、严格 HTTP/Nginx 契约、SELECT-only、隔离 Compose acceptance 与证据边界 |
| 第 22 节：真实 React Lottery 页面 | `codex/lesson-22-react-lottery-page` | 第 21 节累计检查点 `9e3ed50` | transport/API `cbb87d6`～`428ae0d`；Compose 快照 `9cc2d07`；工作台迭代 `3b22628`～`06a4a38`；文档内容 `72e285f`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 真实 ephemeral API 消费、完整 uint64 string、竞态/取消边界、Credits 风格共享工作台、响应式与可访问性验收 |

第 11 节可运行实现固定在 `ade1fad`，配套课程和 QA 文档紧随其后提交。完整章节以 `origin/codex/lesson-11-gin-http-service` 的 tip 为准；这种“实现提交 + 验收文档提交”的顺序既能精确引用代码，也方便逐步比较。

第 12 节可运行实现固定在 `60a6116`，完整验收 tip 为 `ac9ad0e`。第 13 节先在 `b3f5aa7` 完成实现，再由交叉审查在 `b734463` 加固 timeout、dirty/version mismatch、取消 terminal、readiness 响应预算和显式集成门禁。学习实现细节时可依次比较这两个提交，学习完整章节时以 `origin/codex/lesson-13-mysql-migrations` 的最终文档 tip 为准。

第 15 节从包含第 14 节前端框架与第 13 节完整后端证据的累计检查点 `f0cd8e1` 开始。`7e499cc` 建立真实系统状态纵向切片；浏览器离线演练随后发现 Vite 会返回非 JSON 502，`2283a70` 因而把 `gateway` 与 Go 的合法 JSON 503 分开并补齐回归测试。完整课程、API、QA、设计手记与面试问答以 `origin/codex/lesson-15-first-fullstack-integration` 的最终 tip 为准。

第 16 节从第 15 节最终验收 tip `03ebe56` 开始。`e746a6f` 建立直接密码与 `_FILE` 二选一的秘密输入边界，`52c3add` 阻止 MySQL driver 绕过 JSON 日志，`7aa6c9e` 再交付隔离的 Compose 栈、脱敏同源网关、Secret 生成器、smoke 与定速负载工具，`ad8078c` 固化课程、QA、设计手记、面试问答、ADR、Runbook 与全局索引。按这四个提交依次阅读，可以分别理解“配置边界—可观测边界—运行拓扑—验证与设计复盘”，完整章节以同名远端分支最终 tip 为准。

第 17 节从第 16 节最终检查点 `f9cdd3c` 开始。`0b59217` 只把 `internal/lottery/.gitkeep` 替换为持久化无关的 Strategy/Award 纯领域模型及单元测试，没有混入表、Repository、随机算法、API、真实前端或 Redis；`792f04e` 再固化课程、API、QA、第一性原理设计手记、24 道面试问答、ADR 与全局当前态。先比较实现提交可以专注理解对象与不变量，再比较文档提交，可以学习“设计意图—可执行模型—证据边界”如何闭环。

第 18 节从第 17 节最终检查点 `24e606a` 开始。`7593aaa` 增加 Strategy/Award 两个前向 Migration、数据库约束、授权 reconciliation、Compose 编排与真实 MySQL 集成验收；`f74fdf2` 用可执行测试明确 AwardID 只在 Strategy 内唯一；`215abd1` 再固化课程、API、QA、第一性原理设计手记、24 道面试问答、两份 ADR 与全局当前态。最后的检查点提交只登记已复跑事实，因此完整学习版本以 `origin/codex/lesson-18-lottery-schema` 的最终 tip 为准。

第 19 节从第 18 节最终检查点 `4c06e25` 开始。`50ac811` 增加 consumer-owned Create/FindByID 窄端口、MySQL adapter、父子原子写事务、只读 RR 聚合快照、存量数据恢复校验、稳定错误分类、真实 MySQL 并发/取消/执行计划测试，以及两表精确 `SELECT, INSERT` 授权；`2c420c9` 再以生产 `TxOptions` 的真实只读事务探针、真实 1205 和逆序多 Award SQL 控制流收紧证据；`09556d8` 固化课程、API、QA、1098 行第一性原理设计手记、28 道面试问答、ADR 与全局当前态。依次阅读这三个提交，可以分别理解实现、证据加固和设计复盘；完整学习版本以同名远端分支最终 tip 为准。

第 20 节从第 19 节最终检查点 `7b67d2c` 开始。`db679cf` 增加领域拥有的 bounded random port、覆盖完整 `uint64` 的 CryptoSource 适配器、无浮点和无取模偏差的加权 bucket 映射、稳定错误分类，以及穷举边界、MaxUint64、拒绝采样、typed-nil、并发与 benchmark 证据；`f2475fa` 根据 Go 1.26.6 官方契约校准默认 `crypto/rand.Reader` 不可恢复失败与自定义 reader 可返回错误的语义边界；`6f08b80` 固化课程、API、QA、4223 行第一性原理设计手记、30 道带来源的面试问答、ADR 与全局当前态。依次阅读这三个提交，可以把核心算法、运行时语义和架构复盘分开学习；完整学习版本以同名远端分支最终 tip 为准。

第 21 节从第 20 节最终检查点 `ea71640` 开始。`65e9627` 首次把 StrategyReader、WeightedSelector、CryptoSource 与 Gin adapter 装配成受限 ephemeral selection 纵向链；`be41d92` 收紧 DTO、feature gate、timeout、错误分类、Award 上限和 SELECT-only 权限；`9100221` 把代理正规化前仍可观察的 Transfer-Encoding 拒绝放到 Nginx 边缘；`e32ecd4` 建立随机 project 的真实 Nginx→Go→MySQL→CryptoSource acceptance；`93f5694` 让长期 smoke 动态推导缺失 Strategy；`3d4a44a` 补上非空 Trailer 声明边缘拒绝；`ef3f266` 用 device/inode 与子文件类型复核一次性临时目录；`7c43456` 补齐编码空白、未知 Content-Length 与裸问号等请求分支；`90129b6` 固化课程、API、QA、1500 余行第一性原理手记、32 道带真实面经题型与官方技术来源的问答、ADR、Runbook 和全局当前态。按这个顺序阅读，可以把“最小纵向链—边界收敛—真实入口缺陷—隔离验收—清理安全—设计复盘”逐步拆开；完整学习版本以同名远端分支最终 tip 为准。

第 22 节从第 21 节累计检查点 `9e3ed50` 开始。`cbb87d6`、`41a7833`、`428ae0d` 依次建立无请求体 JSON transport、严格 Lottery adapter 与 React 请求状态切片；`9cc2d07` 固化可联调 Compose 镜像快照；`3b22628`～`06a4a38` 再按“Lottery 页面收口—共享 WorkspaceShell—用户工作台—operator 工作台—交互/可访问性—路由分包”逐步迭代；`72e285f` 固化课程、API、QA、第一性原理设计手记、31 道带真实面经题型与官方来源的问答及设计验收。最终检查点同步 101 节路线，但不把当前 Mock 工作区写成认证或 RBAC。完整学习版本以 `origin/codex/lesson-22-react-lottery-page` 的最终 tip 为准。

## 稳定章节分支与累计快照

课程同时保留两类用途不同的分支：

| 类型 | 分支规则 | 是否移动 | 当前事实 |
| --- | --- | --- | --- |
| 单节稳定分支 | `codex/lesson-XX-...` | 一节验收结束后保持稳定 | 第 22 节实现、浏览器/设计验收与最终文档均推送后，冻结同名远端 tip |
| 最新累计快照 | `codex/complete-implementation` | 每节验收后快进 | 第 22 节最终检查点和门禁通过后快进至该节稳定分支 tip |

学习单节变化时使用对应稳定分支；想直接查看目前全部已验收实现时使用 `codex/complete-implementation`。累计分支只做 fast-forward，不替代每节固定检查点，也不能因为代码已合入工作树就提前代表“已验收”。

## 历史章节分支

| 章节 | 分支 | 检查点提交 | 2026-08-29 可用位置 | 说明 |
| --- | --- | --- | --- | --- |
| 第 1 节 | `codex/lesson-01-growth-platform` | `1653614` | 仅本地 | 严格完整检查点与第 2 节共享，见下方历史说明 |
| 第 2 节 | `codex/lesson-02-user-growth-journey` | `1653614` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 3 节 | `codex/lesson-03-operator-workflow` | `eb538d4` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 4 节 | `codex/lesson-04-ai-operator-workflow` | `788b7f6` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 5 节 | `codex/lesson-05-event-storm` | `16f0f19` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 6 节 | `codex/lesson-06-bounded-contexts` | `9b5507e` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 7 节 | `codex/lesson-07-non-functional-requirements` | `c283ea1` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 8 节 | `codex/lesson-08-system-design-v0` | `6e70006` | 本地与 `origin` | 系统设计 V0 完整检查点 |
| 第 14 节 | `codex/lesson-14-react-frontend-framework` | `d8eacb8` | 仅本地 | 前端实现提交之后补齐章节 API 治理的完整检查点 |

“仅本地”不是遗漏标记：这些分支通过 HTTPS 推送时，当前 OAuth 凭据因历史中包含 workflow 变更而被 GitHub 拒绝。权限问题解决且实际推送成功前，不得把它们写成远端分支。

第 1 节在更早提交已经有正文、QA 和完成状态，但当时尚未建立章节 API 记录体系；第 2 节提交才补齐第 1 节 API。因此严格采用“正文 + QA + API + 索引”口径时，第 1、2 节分支会指向同一提交。第 14 节的前端实现先落在 `5f8a75b`，API 治理随后在 `d8eacb8` 补齐；学习时可以把这两个提交分别理解为实现与文档治理。

## 建议学习顺序

先刷新远端引用，再按下面的顺序切换：

```bash
git fetch origin
git switch codex/lesson-09-repository-baseline
git switch codex/lesson-10-modular-monolith
git switch codex/engineering-baseline-hardening
git switch codex/lesson-11-gin-http-service
git switch codex/lesson-12-config-logging-errors
git switch codex/lesson-13-mysql-migrations
git switch codex/lesson-14-react-frontend-framework
git switch codex/lesson-15-first-fullstack-integration
git switch codex/lesson-16-docker-compose-development
git switch codex/lesson-17-lottery-domain-objects
git switch codex/lesson-18-lottery-schema
git switch codex/lesson-19-lottery-repository
git switch codex/lesson-20-lottery-weighted-algorithm
git switch codex/lesson-21-lottery-api
git switch codex/lesson-22-react-lottery-page
```

如果本地还没有某个**已经确认存在于远端**的课程分支，可从对应远端分支创建跟踪分支：

```bash
git switch --track origin/codex/lesson-11-gin-http-service
git switch --track origin/codex/lesson-12-config-logging-errors
git switch --track origin/codex/lesson-13-mysql-migrations
git switch --track origin/codex/lesson-15-first-fullstack-integration
git switch --track origin/codex/lesson-16-docker-compose-development
git switch --track origin/codex/lesson-17-lottery-domain-objects
git switch --track origin/codex/lesson-18-lottery-schema
git switch --track origin/codex/lesson-19-lottery-repository
git switch --track origin/codex/lesson-20-lottery-weighted-algorithm
git switch --track origin/codex/lesson-21-lottery-api
git switch --track origin/codex/lesson-22-react-lottery-page
```

每一节建议先看提交摘要，再比较与上一检查点的差异：

```bash
git log --oneline --decorate --graph --max-count=20
git diff --stat codex/engineering-baseline-hardening..codex/lesson-11-gin-http-service
git diff codex/engineering-baseline-hardening..codex/lesson-11-gin-http-service
git diff --stat codex/lesson-11-gin-http-service..codex/lesson-12-config-logging-errors
git diff codex/lesson-11-gin-http-service..codex/lesson-12-config-logging-errors
git diff --stat codex/lesson-12-config-logging-errors..codex/lesson-13-mysql-migrations
git diff codex/lesson-12-config-logging-errors..codex/lesson-13-mysql-migrations
git diff --stat f0cd8e1..codex/lesson-15-first-fullstack-integration
git diff f0cd8e1..codex/lesson-15-first-fullstack-integration
git diff --stat 03ebe56..codex/lesson-16-docker-compose-development
git diff 03ebe56..codex/lesson-16-docker-compose-development
git diff --stat f9cdd3c..codex/lesson-17-lottery-domain-objects
git diff f9cdd3c..codex/lesson-17-lottery-domain-objects
git diff --stat 24e606a..codex/lesson-18-lottery-schema
git diff 24e606a..codex/lesson-18-lottery-schema
git diff --stat 4c06e25..codex/lesson-19-lottery-repository
git diff 4c06e25..codex/lesson-19-lottery-repository
git diff --stat 7b67d2c..codex/lesson-20-lottery-weighted-algorithm
git diff 7b67d2c..codex/lesson-20-lottery-weighted-algorithm
git diff --stat ea71640..codex/lesson-21-lottery-api
git diff ea71640..codex/lesson-21-lottery-api
git diff --stat 9e3ed50..codex/lesson-22-react-lottery-page
git diff 9e3ed50..codex/lesson-22-react-lottery-page
```

第 11 节的直接起点是工程基线加固，而不是 `main`。这样比较只会显示本节新增的 HTTP 服务、测试和配套文档；若直接与 `main` 比较，还会混入第 9、10 节和工程加固的变化。

第 12 节直接基于第 11 节已验收 tip，完整验收 tip 为 `ac9ad0e`。第 13 节直接基于 `ac9ad0e`；实现 `b3f5aa7` 和交叉审查加固 `b734463` 已推送至 `origin/codex/lesson-13-mysql-migrations`。第 15 节的直接起点是累计检查点 `f0cd8e1`，因此比较本节时应以该提交为基线；实现 `7e499cc` 与浏览器契约加固 `2283a70` 已推送。第 16 节直接基于第 15 节最终 tip `03ebe56`，实现提交为 `e746a6f`、`52c3add` 与 `7aa6c9e`，文档内容提交为 `ad8078c`。第 17 节直接基于 `f9cdd3c`，纯领域实现提交为 `0b59217`，文档内容提交为 `792f04e`。第 18 节直接基于 `24e606a`，实现提交为 `7593aaa`，策略内身份加固为 `f74fdf2`，文档内容提交为 `215abd1`。第 19 节直接基于 `4c06e25`，实现提交为 `50ac811`，证据加固为 `2c420c9`，文档内容为 `09556d8`。第 20 节直接基于 `7b67d2c`，实现提交为 `db679cf`，随机源语义校准为 `f2475fa`，文档内容为 `6f08b80`。第 21 节直接基于 `ea71640`，纵向链实现为 `65e9627`，协议、真实入口、隔离验收和清理加固依次为 `be41d92`、`9100221`、`e32ecd4`、`93f5694`、`3d4a44a`、`ef3f266`、`7c43456`，文档内容为 `90129b6`。第 22 节直接基于 `9e3ed50`，前端 transport/API 切片为 `cbb87d6`～`428ae0d`，Compose 快照为 `9cc2d07`，工作台、响应式、可访问性与路由分包迭代为 `3b22628`～`06a4a38`，文档内容为 `72e285f`；每节最终仍以同名远端分支 tip 作为完整学习检查点。

## 分支使用约束

1. 每个实现章节使用独立的 `codex/lesson-XX-...` 分支；非课程章节但会改变后续起点的加固工作使用独立命名分支。
2. 下一节从最近一个已验收检查点创建，保留线性、可比较的学习路径。
3. 一个章节可有多个小提交，但最终必须有一个通过完整门禁的分支 tip；QA 记录对应的验证命令、环境和结果，设计手记记录推导与风险，面试问答记录可口述答案和来源边界。
4. 不在 `main` 上提前合并后续实现。学习者可按需 cherry-pick、手工重做或比较差异。
5. 分支已推送不等于章节已完成；以 `status.csv` 和 QA 实测证据为准。

## 后续登记模板

新增章节分支时，在“当前检查点”追加一行，并记录：章节、完整分支名、直接起点、已验收提交或分支 tip、学习重点。若分支被重建或强制移动，必须同步更新本页和对应 QA，避免学习者比较到错误版本。
