# 课程分支检查点

**状态：** 当前

**更新日期：** 2026-08-31

本页把课程章节与可检出的 Git 分支对应起来，方便在 `main` 保持原状的同时，逐节查看实现是怎样演进的。分支记录的是学习检查点，不替代[课程状态台账](status.csv)；章节只有具备正文、API 记录、实际 QA 证据并通过质量门禁后，才能在台账中标记“已完成”。从第 13 节开始，稳定分支还必须包含对应的第一性原理设计手记与面试问答；历史章节将按这一新增规范逐节回填。

## 当前检查点

| 检查点 | 分支 | 起点 | 已确认提交 | 2026-08-31 可用位置 | 学习重点 |
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
| 第 23 节：Lottery 规则需求与边界 | `codex/lesson-23-lottery-strategy-rules` | 第 22 节最终检查点 `1f95779` | 需求/ADR `09a45c2`；课程与配套文档 `479947b`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 决定所有者与事实提供方分离、决策/选择/副作用三分法、失败/版本边界、零运行时代码漂移与渐进式规则路线 |
| 第 24 节：Lottery Strategy Redis 缓存 | `codex/lesson-24-redis-strategy-cache` | 第 23 节最终检查点 `27a552b` | 配置/Redis/cache/composition `272d028`～`68fa59b`；Compose/负载/验收加固 `17e7010`～`d33723c`；文档内容以同名远端最终 tip 为准 | 本地与 `origin` | MySQL 事实源、cache-aside、严格投影、同 key fill、fail-open、最小 ACL、故障恢复与 M1 source-load 证据 |
| 第 25 节：Participation 新用户资格 | `codex/lesson-25-user-eligibility` | 第 24 节最终检查点 `35f94b9` | 需求/ADR `ea2cacd`～`b59bc1e`；domain `959c32a`；application `b718267`；架构/构造加固 `475804b`、`c96b393`；课程/API `bf15a1b`；面试 `4987bdb`；QA/设计 `399948a`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 权威注册事实、含边界 cutoff、freshness、确定决定/技术失败、取消竞态、无 adapter/API/RuleEngine 的渐进停止线 |
| 第 26 节：Participation 前置资格链 | `codex/lesson-26-responsibility-chain` | 第 25 节最终检查点 `8b2f3a6` | 产品基线 `974a61b`；ADR `5271963`；风险准入 `ad25dbe`；固定链 `f77e17f`；课程/API `61282ba`；面试 `d43bbff`；设计 `d87343e`；产品证据/格式修复 `eac4b92`；架构当前态 `ff1f24f`；索引/状态 `833abd0`；partial-chain 反证 `c46410e`；完整章节以包含本 QA/台账的远端冻结 tip 为准 | 本次最终证据提交并推送后冻结于本地与 `origin` | 第二条真实风险准入规则、一次 shared as-of、固定顺序、短路、取消、最小 trace 与无通用 RuleEngine 的停止线 |
| 第 27 节：责任链为什么开始不够用了 | `codex/lesson-27-responsibility-chain-limits` | 第 26 节最终检查点 `47fc94d` | 产品基线 `2d7728a`；ADR `57a3216`；domain `076e399`；stable branch/explicit switch `b307f1a`；application `42caed9`；架构 guard `2dc49b1`；decision evidence `8ebc94a`；revision claim `544f4af`；课程 `a6d2d49`；API `5ecb061`；面试 `f2e5e07`；设计 `d57b963`；架构当前态 `5460bde`；QA 计划 `331c8c7`；revision 语义 `6db36dd`；递归 guard `59499fb`；停止线证据 `a04d89d`；候选门禁 `9ead6e1`；索引 `aaa4a8f`；完整章节以同名远端实际冻结 tip 为准 | 本地与 `origin` | Lottery 会员事实防腐投影、premium override、standard baseline default、单一 as-of、一跳 path 与线性 gate chain 多出口边界 |
| 第 28 节：规则树第一次数据库升级 | `codex/lesson-28-rule-tree-schema` | 第 27 节已验收 tip `809d436` | 已推送学习小提交 `f27ce17`、`2786d96`、`17a6c54`、`ac89423`、`d53b2ec`、`4d9b074`、`e053527`、`8db8c3c`、`4b79d1d`、`97bb783`、`d7deafa`、`2d2c7c2`、`f6b537d`、`ebe0b70`、`3b3886b`、`5c757a9`、`be19827`、`54f4769`、`e4531bf`、`4057f5d`；冻结证据 `3f21b96`；完整章节以同名远端实际冻结 tip 为准 | 本地与 `origin` | Lottery-owned 有界不可变 rooted DAG、Migration latest v5、三表关系模型、严格恢复、最小权限测试身份与未装配边界 |
| 第 29 节：实现规则决策引擎 | `codex/lesson-29-rule-decision-engine` | 第 28 节已验收 tip `90844c1` | 已推送产品/ADR `041dc30`、`27bd514`；共享 oracle/fact session `0ca25d2`、`51dbd61`；domain/application/停止线 `a173d8b`、`3863b06`、`ab056ba`；异构并发隔离 `515d776`；候选冻结 `e74037e`；完整章节以同名远端实际冻结 tip 为准 | 远端引用记录提交后再次对齐同名远端与累计分支 | exact graph 封闭 typed 求值、单一 graph/fact/as-of、完整 path、step/time/cancel budget、zero-decision 与未装配边界 |
| 第 30 节：为什么 Strategy 不等于 Activity | `codex/lesson-30-strategy-vs-activity` | 第 29 节已验收 tip `1b7521e` | 产品/ADR `3dfc028`、`fe0c12e`；snapshot/Activity/schema/application/repository/验收 `7404f91`～`065da2f`；文档/当前态 `5d8b53a`～`cb7255f`；完成/冻结 `887754b`、`6504e91` | 本地与 `origin` | exact Strategy snapshot、Activity immutable publication、CAS/rollback/resolve、Lottery ACL、commit unknown 对账与 v11 证据 |
| 第 31 节：统一访问控制模型与威胁边界 | `codex/lesson-31-access-control-model-threat-boundary` | 第 30 节已验收 tip `6504e91` | 产品/ADR `c7e5248`～`35c0420`；values/roles/policy/evaluator `34c3add`～`a054eb1`；停止线/威胁矩阵 `6cae442`、`ea98390`；术语/课程/设计/面试/QA `c606de1`～`06fedef`；最终冻结以同名远端实际 tip 为准 | 本地与 `origin` | Governance exact capability、角色模板上限、四种 scope、不可变 Policy、default deny/deny precedence、证据与无 session/runtime/UI 停止线 |

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

第 23 节从第 22 节最终检查点 `1f95779` 开始。`09a45c2` 先交付 32 条 Lottery 规则需求、上下文边界与 ADR；`479947b` 再补齐课程、零 API 变化记录、QA、1018 行第一性原理设计手记、24 道面试问答及全局索引，并根据交叉审查把正式 Draw/Result 与 Benefit 发放/补偿拆成两个单一决定所有者。本节刻意不创建 Rule 接口、Migration、Redis 调用、HTTP/React 资格判断或权限实现；第 31～35 节的统一访问控制仍按模型、会话、服务端强制、前端权限投影和越权 E2E 顺序演进。完整学习版本以 `origin/codex/lesson-23-lottery-strategy-rules` 的最终 tip 为准。

第 24 节从第 23 节最终检查点 `27a552b` 线性创建。`272d028`～`765bb00` 依次建立可选配置、默认按需连接且无启动 PING 的 Redis client、TTL 停止线、ADR 与严格 cache-aside；`68fa59b` 装配生命周期和低基数观测；`17e7010`～`8804f87` 接入 internal network、最小 ACL、bodyless POST 基线并根据审查收紧 channel/acknowledgement；`40f1acf` 依据 2 GiB Docker Desktop OOM 事实限制后端编译资源；`d33723c` 最后用 warm/direct/Redis-down 三组同口径负载和 MySQL counter 证明真实路径。`0f70f51` 登记 ADR，`9d44eb1` 与 `5a0baeb` 同步运行/产品当前态，`d621aae` 交付课程、API、QA、1400 行设计手记、24 道面试问答与 Runbook，`13a3210` 登记全局索引与章节状态；`b9e79e2` 再根据最终交叉审查固定 Redis DB 0、校准 MinIdle/ACL/readiness/TTL 证据并补 depth guard 边界测试。最终门禁以同名远端分支冻结 tip 为准。

第 25 节从第 24 节最终检查点 `35f94b9` 线性创建。`ea2cacd` 与 `b59bc1e` 先固定权威注册事实、freshness、含边界 cutoff 和低基数观测边界；`959c32a` 只实现持久化/传输无关的 Participation domain；`b718267` 再用 consumer-owned fact port、一次受控 Clock 和安全类型化错误形成 application 用例；`475804b` 以 AST 测试守住“无通用 Rule/RuleEngine、无跨上下文依赖”的停止线；`c96b393` 根据交叉审查补齐手工 zero/partial service 的失败关闭测试；`bf15a1b`、`4987bdb`、`399948a` 依次交付课程/API、面试问答和 QA/设计手记。全局当前态与最终门禁以同名远端最终 tip 为准。本节没有事实 adapter、Migration、Redis、HTTP/React、真实 Principal、Lottery 编排或 Compose 验收；按上述提交顺序阅读，可以把“事实与政策—纯领域决定—应用失败语义—架构停止线—设计复盘”分开学习。

第 26 节从第 25 节最终检查点 `8b2f3a6` 线性创建。`974a61b` 先说明为何选择既有需求中的风险准入作为第二条真实 Participation 规则，并固定事实所有权、顺序与短路边界；`5271963` 比较次数、Activity、权限、事实预取、双时钟和通用 Rule 接口等方案后，接受固定两节点链；`ad25dbe` 落地最小风险事实、具体 policy/decision、consumer-owned reader 与单一安全错误分类；`f77e17f` 再从新用户和风险两条具体规则共同证明的需求中抽出一次 shared as-of、固定 `new-user -> risk` 顺序、失败/取消短路与只含实际执行节点的 trace。`61282ba`、`d43bbff`、`d87343e` 依次交付课程/API、面试问答与第一性原理设计手记；`eac4b92` 同步产品证据并关闭两个 EOF 格式缺陷；`ff1f24f` 更新架构所有权当前态；`833abd0` 登记章节状态与全局索引；`c46410e` 根据终审补齐 7 组手工 zero/partial chain 的失败关闭与依赖零调用反证。最终 doccheck、`make verify`、全仓 race、20 轮 Participation shuffle、diff-check、runtime negative diff 和构建产物清理均已有 QA 实测证据；根代理提交并推送本 QA/台账后，以 `origin/codex/lesson-26-responsibility-chain` 的实际冻结 tip 作为完整学习检查点，不预写尚未生成的最终 SHA。

第 27 节从第 26 节最终检查点 `47fc94d` 线性创建。`2d7728a` 与 `57a3216` 先把 premium override、standard baseline default、unknown 失败关闭、事实/决定所有权和第 28～35 节停止线冻结为新增产品决定；`076e399` 只实现 Lottery 会员事实、具体 policy、纯 Route 与一跳 path；`b307f1a` 根据交叉审查把稳定 branch literal 与显式 tier switch 变成可执行契约；`42caed9` 增加 consumer-owned reader、单一 Clock/freshness、取消优先和安全 Cause；`2dc49b1` 守住 Lottery/Participation 与无通用 Engine 的边界；`8ebc94a` 再校验 decision/branch/reason/path 内部一致；`544f4af` 诚实校准 revision 字符串尚未绑定唯一内容。课程、API、31 道问答、设计手记、架构当前态和 QA 计划依次由 `a6d2d49`、`5ecb061`、`f2e5e07`、`d57b963`、`5460bde`、`331c8c7` 交付；`6db36dd`、`59499fb`、`a04d89d` 再根据终审校准 revision 确定性、递归扫描泛型函数并对齐停止线证据；`9ead6e1` 与 `aaa4a8f` 登记候选门禁和全局索引，最终 clean-worktree 证据以同名远端冻结 tip 为准。本节没有 membership adapter、DB/schema、Redis、HTTP/React、Strategy load/Selector、Activity、权限或浏览器 E2E。

第 28 节从第 27 节已验收 tip `809d436` 线性创建。实现与依赖学习提交依次为 `f27ce17`～`ebe0b70`，再由 `3b3886b`、`5c757a9`、`be19827`、`54f4769`、`e4531bf`、`4057f5d` 分别交付课程/API、第一性原理设计、QA 证据、架构当前态、运维手册和全局索引。当前数据库 Migration latest 为 5；三张图表存在不代表图已执行、已发布、已接公开 API 或已进入运行时组合根。第 29 节才实现已验证图的决策执行器。完整章节以 `origin/codex/lesson-28-rule-tree-schema` 最终实际冻结 tip 为准；最终 SHA 由 Git 内容产生，不在会改变自身 SHA 的冻结提交中虚构。

第 29 节从第 28 节已验收 tip `90844c1` 线性创建。`041dc30`、`27bd514` 先冻结 exact graph/fact/time、closed typed dispatch、worst-depth admission、step/time/cancel budget 与 zero-decision；`0ca25d2`、`51dbd61` 再把第 27 节 branch oracle 和 Clock/fact/freshness session 收敛为两条 package-private 共享语义；`a173d8b` 实现 immutable decision/path 与迭代 exact-branch evaluator，`3863b06` 实现未装配 application orchestration 和低披露错误，`ab056ba` 守住 generic engine 与 runtime/API/UI stop line，`515d776` 再以两组 subject/graph/tier/target 交错的 64 请求补强并发隔离证据。后续文档/验收提交登记完整课程、API、QA、设计、面试、运维和当前态；完整章节以 `origin/codex/lesson-29-rule-decision-engine` 的最终实际冻结 tip 为准。该 tip 只证明内部 evaluator，不代表 graph 已发布、绑定 Activity、获得真实会员事实或进入受权限保护的线上链路。

第 30 节从第 29 节已验收 tip `1b7521e` 线性创建。它按产品边界、Strategy snapshot、schema、Activity domain/application/repository、隔离 MySQL 门禁、commit unknown 对账和文档验收逐步推进，最终冻结于 `6504e91`。完整分支只证明 exact publication 与工程边界，不代表已有 Activity API/UI、真实审批、访问控制或正式 Draw。

第 31 节从第 30 节已验收 tip `6504e91` 线性创建。前三个提交先冻结威胁边界与 Governance 所有权；随后以 values、role ceiling、immutable Policy、evaluator 四步形成纯内核，再由两轮架构/威胁矩阵和术语审查关闭子包逃逸、session 越界、capability 调包与 evidence 缺口。课程/API、设计、40 道逐题证据面试问答、QA 和模型审查手册继续拆成小提交。该分支没有 session、HTTP、DB、UI 或 runtime consumer；完整学习版本以同名远端最终实际冻结 tip 为准。

## 稳定章节分支与累计快照

课程长期保留稳定章节分支与累计快照；第 28 节的实现与已推送学习提交已经形成，最终 QA/台账与冻结门禁以本节最终提交为准，累计分支随后只做 fast-forward，因此 Git 内容证据与远端引用移动分开记录：

| 类型 | 分支规则 | 是否移动 | 当前事实 |
| --- | --- | --- | --- |
| 单节稳定分支 | `codex/lesson-XX-...` | 一节验收结束后保持稳定 | 第 28 节以包含最终 QA/台账的同名远端实际 tip 冻结；最终 SHA 由 Git 产生，不在提交前猜测 |
| 本次冻结分支 | `codex/lesson-28-rule-tree-schema` | 验收后保持稳定 | 已推送学习小提交见当前检查点；完整章节以 push 后同名远端实际冻结 tip 为准 |
| 最新累计快照 | `codex/complete-implementation` | 每节验收后快进 | 第 28 节冻结 push 验收后由根代理 fast-forward 到同一实际 tip；具体 SHA 以远端引用核查为准，不在提交前猜测 |

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
git switch codex/lesson-23-lottery-strategy-rules
git switch codex/lesson-24-redis-strategy-cache
git switch codex/lesson-25-user-eligibility
git switch codex/lesson-26-responsibility-chain
git switch codex/lesson-27-responsibility-chain-limits
git switch codex/lesson-28-rule-tree-schema
git switch codex/lesson-29-rule-decision-engine
git switch codex/lesson-30-strategy-vs-activity
git switch codex/lesson-31-access-control-model-threat-boundary
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
git switch --track origin/codex/lesson-23-lottery-strategy-rules
git switch --track origin/codex/lesson-24-redis-strategy-cache
git switch --track origin/codex/lesson-25-user-eligibility
git switch --track origin/codex/lesson-26-responsibility-chain
git switch --track origin/codex/lesson-27-responsibility-chain-limits
git switch --track origin/codex/lesson-28-rule-tree-schema
git switch --track origin/codex/lesson-29-rule-decision-engine
git switch --track origin/codex/lesson-30-strategy-vs-activity
git switch --track origin/codex/lesson-31-access-control-model-threat-boundary
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
git diff --stat 1f95779..codex/lesson-23-lottery-strategy-rules
git diff 1f95779..codex/lesson-23-lottery-strategy-rules
git diff --stat 27a552b..codex/lesson-24-redis-strategy-cache
git diff 27a552b..codex/lesson-24-redis-strategy-cache
git diff --stat 35f94b9..codex/lesson-25-user-eligibility
git diff 35f94b9..codex/lesson-25-user-eligibility
git diff --stat 8b2f3a6..codex/lesson-26-responsibility-chain
git diff 8b2f3a6..codex/lesson-26-responsibility-chain
git diff --stat 47fc94d..codex/lesson-27-responsibility-chain-limits
git diff 47fc94d..codex/lesson-27-responsibility-chain-limits
git diff --stat 809d436..codex/lesson-28-rule-tree-schema
git diff 809d436..codex/lesson-28-rule-tree-schema
git diff --stat 90844c1..codex/lesson-29-rule-decision-engine
git diff 90844c1..codex/lesson-29-rule-decision-engine
git diff --stat 1b7521e..codex/lesson-30-strategy-vs-activity
git diff 1b7521e..codex/lesson-30-strategy-vs-activity
git diff --stat 6504e91..codex/lesson-31-access-control-model-threat-boundary
git diff 6504e91..codex/lesson-31-access-control-model-threat-boundary
```

第 11 节的直接起点是工程基线加固，而不是 `main`。这样比较只会显示本节新增的 HTTP 服务、测试和配套文档；若直接与 `main` 比较，还会混入第 9、10 节和工程加固的变化。

第 12 节直接基于第 11 节已验收 tip，完整验收 tip 为 `ac9ad0e`。第 13～22 节的直接起点和核心提交见上表与对应段落。第 23～29 节的需求、实现和停止线提交见当前检查点表。第 30 节从第 29 节最终 tip `1b7521e` 开始并冻结于 `6504e91`。第 31 节再从 `6504e91` 开始，按产品/ADR、values、roles、Policy、evaluator、威胁矩阵和六类文档顺序推进；完整章节以根代理推送包含最终 QA/台账的冻结提交后形成的同名远端实际 tip 为准，随后累计分支再 fast-forward 到该 tip。

## 分支使用约束

1. 每个实现章节使用独立的 `codex/lesson-XX-...` 分支；非课程章节但会改变后续起点的加固工作使用独立命名分支。
2. 下一节从最近一个已验收检查点创建，保留线性、可比较的学习路径。
3. 一个章节可有多个小提交，但最终必须有一个通过完整门禁的分支 tip；QA 记录对应的验证命令、环境和结果，设计手记记录推导与风险，面试问答记录可口述答案和来源边界。
4. 不在 `main` 上提前合并后续实现。学习者可按需 cherry-pick、手工重做或比较差异。
5. 分支已推送不等于章节已完成；以 `status.csv` 和 QA 实测证据为准。

## 后续登记模板

新增章节分支时，在“当前检查点”追加一行，并记录：章节、完整分支名、直接起点、已验收提交或分支 tip、学习重点。若分支被重建或强制移动，必须同步更新本页和对应 QA，避免学习者比较到错误版本。
