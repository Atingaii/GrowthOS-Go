# 第 13 节第一性原理设计手记：MySQL 连接与 Migration 边界

> 本文记录第 13 节的决策推导，不是安装 MySQL 的教程、验收命令清单或面试题。当前时间切片中，系统已经有可启动的 Gin 进程、配置/日志/错误边界和无依赖 `/health`，但还没有业务表、Repository、事务管理器或业务 API。

## 1. 决策命题：先建立“改变事实的秩序”，再创造业务事实

表面命题是“接入 MySQL”。如果把它翻译成“让 Go 程序成功连接 3306”，只需要一个驱动、一段 DSN 和一次 `Ping`。但真正不可逆的事情不是建立 TCP 连接，而是从第一次建表开始，数据库结构会成为被多个二进制、部署批次、维护者和真实数据共同依赖的历史。

因此这一节真正要回答的是：

> 在第一个业务表尚不存在时，怎样先固定连接身份、资源生命周期、发布入口、版本历史、探针语义和失败停止条件，使第 18 节第一次建表不会同时承担基础设施试错？

这解释了一个看似反直觉的结果：第 13 节实现了数据库连接和 Migration 机制，却没有提交任何真实 `.up.sql`，也没有业务表。空产品迁移集不是“功能没做完”，而是保护首个版本号和首个 schema 决策不被占位需求污染。

### 1.1 当前已经拥有的能力

- `growth-api` 有明确的进程启动、信号和关闭边界；
- 环境配置先经过类型化、聚合校验，再进入运行时装配；
- 日志和 HTTP 错误已有稳定、脱敏的输出边界；
- `/health` 已定义为不访问外部依赖的进程 liveness；
- 课程路线已经把第一批业务表放在第 18 节，把 Repository 放在第 19 节。

### 1.2 当前明确没有的能力

- 没有 `strategy`、`strategy_award` 或任何其他业务表；
- 没有产品 `000001_*.up.sql`；
- 没有 Repository、Unit of Work 或事务管理器；
- 没有让 API 在启动时比较 schema 精确版本的 gate；
- 没有让 API 自动执行 DDL；
- 没有 Redis、消息队列、读写分离或分库分表。

后三项尤其需要被正确理解：它们不是遗漏。没有业务写入用例时，事务管理器没有可验证的事务边界；没有真实 schema 时，API schema exact-version gate 没有兼容性模型可判断；没有发布系统和业务表时，自动迁移只会把高风险能力塞进在线进程，却没有带来业务价值。

## 2. 先把事实分层：哪些是事实，哪些只是偏好

设计若从“我喜欢 sqlx”开始，就很容易把工具偏好伪装成架构理由。本节先列出不依赖具体库的事实。

| 类别 | 当前事实 | 直接含义 |
| --- | --- | --- |
| 事实源 | 后续营销策略、奖品和库存会有必须持久化的权威事实 | 需要事务型持久化系统，但此刻还不能凭空决定表形状 |
| 历史 | schema 一旦承载数据，就不能像普通代码文件一样随意覆盖 | 结构变化必须有有序、可审计、不可改写的历史 |
| 故障域 | API 请求、数据库连接、DDL 发布和运维恢复具有不同失败半径 | 不能让在线进程天然拥有结构变更能力 |
| 权限 | 在线 DML 与结构 DDL 需要的权限集合不同 | 必须有 API 与 Migrator 两个身份，而不只是两组配置名称 |
| 发布 | 将来会有旧、新二进制在滚动发布窗口共存 | schema 演进要优先兼容窗口，不能把应用回滚等同于 schema 回滚 |
| MySQL 行为 | DDL 可能隐式提交，多语句文件可能部分生效 | “失败”不等于“什么都没发生”，必须显式处理 dirty 与人工恢复 |
| 观测 | 进程活着、数据库可连接、schema 兼容、业务正确是四个不同命题 | `/health`、`/ready`、Migration status 和业务监控不能混为一个探针 |
| 教学节奏 | 第 18 节才有足够领域需求决定首批表 | 现在只能建机制，不能倒推一个所谓最终 schema |

工具选择要服务这些事实，而不是反过来。MySQL、`sqlx` 和 `golang-migrate` 是当前满足约束的实现，不是不加条件的永久真理。

## 3. 从七个第一性维度推导需求

### 3.1 事实源：谁有权写入什么

未来 MySQL 会保存需要事务一致性的业务事实，但第 13 节还没有定义任何业务事实。于是可以推导出两条边界：

1. 先建立通用连接端口，不能让基础设施包凭空拥有 `strategy` 等领域词汇；
2. 不创建“以后大概用得到”的表，否则一个未经领域建模的猜测会伪装成事实源。

这也是为什么 `internal/infrastructure/mysql` 只负责连接，不提供业务查询；事务边界要等用例出现后由应用层决定，而不是由连接包预设。

### 3.2 故障域：哪种失败应该影响谁

API 查询失败只应影响对应请求或 readiness；DDL 失败可能改变整个 schema；Migration 恢复操作甚至可能删除数据。若 API 与 Migration 是同一个进程、同一个账号，最小的代码错误就拥有最大的故障半径。

因此需要：

- 在线 API 与迁移命令分成两个进程入口；
- API 启动只打开连接池和 Ping，不执行 Migration；
- `/health` 不访问 MySQL，避免依赖抖动诱发重启风暴；
- `/ready` 只决定是否接流量，数据库不可用时返回 503；
- dangerous recovery 不进入产品 CLI。

### 3.3 权限：代码约束不能替代数据库授权

“API 代码里没有 `CREATE TABLE`”不是安全边界，因为注入、误调用或未来代码都可能执行意外 SQL。真正的边界必须在 MySQL 每次鉴权时成立。

所以 API 身份不授予 DDL，Migrator 身份才拥有目标 schema 上经审核的 DDL；两者使用独立配置类型、独立变量和独立建连函数。API connector 再禁用 multi-statements、本地文件和弱认证能力，形成数据库授权与客户端约束两层防线。

### 3.4 发布：结构变化必须先于还是晚于应用

未来滚动发布意味着一段时间内至少有两个应用版本共存。若新 schema 只兼容新二进制，发布顺序无论怎么选都会出现窗口故障。由此推导出前向、兼容优先的 expand/contract 思路：

- 先增加旧代码可以忽略的新结构；
- 再发布使用新结构的代码；
- 等旧代码完全退出、数据迁移完成后，才在后续版本移除旧结构。

第 13 节还没有真实表，所以没有实现 schema compatibility checker。当前迁移命令会拒绝不属于嵌入历史的数据库版本；API 只检查连接，不检查 exact version。等首批 schema 和滚动发布策略出现后，才能决定 API 需要“最低兼容版本”“版本区间”还是“能力标志”，而不是现在武断地要求精确相等。

### 3.5 可逆性：应用回滚不等于数据库回滚

MySQL DDL 的隐式提交、已有新数据和长时间元数据锁，使通用 `down` 很难拥有一致语义。例如删除新列可以让旧应用恢复，却可能永久丢失新版本已经写入的数据。因此当前产品面只允许向前追加：失败时停止、判断实际落地状态、修复原因，再增加纠正 Migration。

这不是宣称“up-only 天然安全”。它只是拒绝提供一个看似方便、实际无法普遍保证数据安全的自动反向按钮。真正的可逆性来自兼容发布、备份恢复、影子演练和明确停止条件。

### 3.6 可观测性：把四个不同问题分开回答

| 问题 | 当前回答者 | 不能推出的结论 |
| --- | --- | --- |
| 进程是否能响应 | `/health` | 不能证明 MySQL 可用 |
| 当前是否适合接收依赖 MySQL 的流量 | `/ready` 的有界 Ping | 不能证明 schema 兼容、查询延迟或业务数据正确 |
| Migration 历史处于什么状态 | `growth-migrate status` | 不能证明业务数据迁移完整 |
| 业务事实是否正确 | 未来业务指标、对账和一致性检查 | 当前尚不存在 |

把所有依赖塞进 `/health` 会让故障诊断失去分辨率；把 schema 查询塞进每次 readiness 又可能放大数据库压力。第 13 节选择最小而稳定的探针语义。

### 3.7 学习成本：隔离概念，而不是压缩文件数

如果第一个业务 Repository 同时引入驱动、池、TLS、账号、Migration、readiness、事务和表设计，学习者看到的是一次“大爆炸提交”，很难判断每个约束为什么存在。本节把数据库运行边界单独形成分支，让第 18、19 节分别聚焦 schema 与数据访问。这增加了一个阶段，却降低了每一步的认知耦合。

## 4. 备选方案矩阵

### 4.1 数据访问层

| 方案 | SQL 可见性 | 映射成本 | 隐式行为 | 当前结论 | 重新评估触发器 |
| --- | --- | --- | --- | --- | --- |
| 仅 `database/sql` | 高 | 手写扫描和命名参数样板较多 | 最少 | 保留为底层，但不作为上层主入口 | `sqlx` 带来的能力长期未使用，或供应链要求进一步减依赖 |
| `sqlx` + 手写 SQL | 高 | 降低常见扫描/参数样板 | 较少，仍沿用 `database/sql` 池 | 采用 | 查询生成、类型安全或跨库复杂度显著超过手写 SQL 可维护范围 |
| GORM/ORM + AutoMigrate | 中到低，视用法而定 | CRUD 初期低 | 模型映射、自动建表与查询生成较多 | 不采用 | 大量低风险 CRUD 的生产力收益有数据证明，且仍能把 schema 历史与 AutoMigrate 解耦 |

这里拒绝的不是 ORM 本身，而是让 ORM 模型在 API 启动路径自动决定生产 schema。未来即使引入 ORM，也不自动推翻独立 Migration 和账号隔离。

### 4.2 持久化产品

| 方案 | 擅长的事实 | 当前缺口 | 当前结论 | 重新评估触发器 |
| --- | --- | --- | --- | --- |
| MySQL 8.4 | 事务、约束、索引、团队成熟度 | 热点吞吐和复杂分析不是单机 OLTP 的强项 | 作为首个权威事务库 | 真实 SQL/运维约束无法满足需求，且迁移收益高于切换成本 |
| PostgreSQL | 丰富 SQL、扩展与并发能力 | 当前团队/基础设施基线不是它，切换没有业务驱动力 | 不在本节引入 | JSON/扩展/查询或组织标准形成明确优势 |
| Redis | 低延迟缓存、计数、原子脚本 | 不是关系事实和长期 schema 历史的替代品 | 延迟到热点需求 | 第 24 节后抽奖性能和库存竞争出现可测瓶颈 |

“Docker Desktop 已经安装”只能降低本地启动成本，不能单独构成架构选型理由。

### 4.3 Migration 执行入口

| 方案 | 优点 | 主要风险 | 当前结论 |
| --- | --- | --- | --- |
| API 启动自动迁移 | 部署步骤少 | 多实例竞争、DDL 与流量启动耦合、API 需要高权限、失败恢复模糊 | 拒绝 |
| 独立 `growth-migrate` CLI | 身份、审计、时机和失败域清楚 | 发布流程增加一步，需要显式编排 | 采用 |
| 完全自研 Runner | 行为可完全定制 | 重写锁、dirty、版本表和 source，测试面巨大 | 不采用；用受限外壳封装成熟 adapter |

### 4.4 前向与回滚

| 方案 | 表面便利 | 数据现实 | 当前结论 |
| --- | --- | --- | --- |
| 每个 up 强制配 down 并自动回滚 | 操作对称 | DDL 隐式提交、数据不可逆、回滚窗口难定义 | 不提供产品能力 |
| up-only + 人工事故流程 | 强迫兼容设计和停止判断 | 运维纪律要求更高 | 采用 |
| 任意 `force`/版本跳转 | dirty 时快速解锁 | 可能把版本表改“干净”而实际 schema 仍部分变化 | 产品 CLI 禁止，只有审批事故流程可讨论 |

### 4.5 健康与就绪

| 方案 | 故障时平台行为 | 诊断质量 | 当前结论 |
| --- | --- | --- | --- |
| `/health` 同时 Ping 数据库 | 依赖抖动可能触发进程重启 | 无法区分进程与依赖 | 拒绝 |
| `/health` liveness + `/ready` DB Ping | 活着但不可接流量时可摘流 | 语义清楚 | 采用 |
| readiness 同时校验 schema exact version | 可提前阻止不匹配流量 | 还没有兼容模型；精确相等会破坏滚动发布 | 本节推迟 |

## 5. 不变量：实现可以变，性质不能悄悄变

### 5.1 连接不变量

1. API 和 Migration 的凭据来自不同配置字段；密码没有仓库默认值。
2. API connector 的 `MultiStatements` 永远为 false；只有专用 Migration connector 可以为 true。
3. Migration 池最大打开/空闲连接固定为 1，因为 MySQL advisory lock 的获取、执行和释放依赖同一连接。
4. API 池在首次 Ping 前已经设置打开数、空闲数、寿命和空闲时长边界；失败时关闭半成品池。
5. 时间解析使用 UTC，会话时区固定为 `+00:00`，字符集使用驱动专用的 `utf8mb4` 配置。
6. staging/production 只能使用 `verify_identity`；自定义 CA 扩展系统根，不能替代主机名校验。

### 5.2 Migration 不变量

1. 产品历史只接受 `NNNNNN_description.up.sql`；版本非零、不重复，出现 down 或非法 SQL 文件即在执行前失败。
2. 已共享 Migration 不改写；修正必须新增版本。
3. 产品命令只暴露 `up` 和 `status`，不暴露 `down/drop/force`。
4. dirty 和版本不属于当前嵌入历史都 fail closed。
5. 当前没有 `.up.sql` 时返回 `no_migrations`，不创建产品 `schema_migrations`，不占用 `000001`。
6. 无迁移不等于整个 CLI 完全不连接 MySQL：当前装配会先用专用身份打开并 Ping 数据库，再由 Runner 识别空 source；保证的是不初始化版本表、不执行 DDL。

### 5.3 探针不变量

1. `/health` 不访问 MySQL。
2. `/ready` 使用请求派生的有界 context，只报告连接可用性。
3. readiness 失败只返回稳定的 `dependency_unavailable`，不返回驱动 cause、地址或账号。
4. `/ready=200` 不能被文档或调用方解释为“schema 最新”或“业务正确”。

### 5.4 输出不变量

密码、完整连接配置、DSN、SQL、CA 路径和驱动原始错误不得进入 HTTP 响应或通用日志。错误类型只渲染稳定 stage，可信内部代码仍可通过错误链做程序化判断；这不授权入口直接打印 `Unwrap` 后的 cause。

## 6. 信任边界

| 边界 | 边界外输入 | 进入前验证 | 边界内拥有的能力 |
| --- | --- | --- | --- |
| 环境 → `appconfig` | 字符串、秘密、可能缺失或恶意的值 | 枚举、长度、地址、环境/TLS、timeout 关系、池关系 | 类型化 API/Migration 配置 |
| 配置 → MySQL connector | 用户、密码、CA 路径、连接参数 | 再次防御性校验，结构化 Config，不拼可打印 DSN | 建立认证连接 |
| 构建产物 → Migration source | 仓库中的 SQL 与说明文件 | embed、严格文件名、版本去重、拒绝 down/未知 SQL | 改变目标 schema |
| 操作者 → `growth-migrate` | `up/status` 与环境身份 | 严格单参数、配置 loader、最小权限 | 查询或前向改变版本 |
| HTTP 调用方 → readiness | 请求、取消、request ID | 路由/方法、request context、安全 error envelope | 只触发 Ping，不获得数据库细节 |
| 数据库/驱动 → 日志 | 可能含拓扑、SQL、账号的错误 | stage 映射、入口不格式化 cause | 可诊断但脱敏的失败阶段 |

被嵌入的 SQL 不是“天然可信”。它只是把运行时可变文件转换成经过 Git/评审/构建链的供应链输入；恶意或错误 SQL 仍可能被构建并执行，所以代码评审、签名制品和发布审批仍是边界的一部分。

## 7. 资源所有权：用单一关闭责任消除泄漏和 double close

数据库生命周期最容易在错误路径泄漏，因为 `sql.OpenDB` 本身通常不建连，真正错误可能出现在 Ping、获取专用连接或 adapter 初始化之后。本节用所有权转移而不是“大家都 defer Close”解决。

### 7.1 API 所有权链

```text
mysqlstore.Open
  ├─ connector/open pool
  ├─ 安装 pool 边界
  ├─ Ping 失败：内部 Close，返回 nil + safe error
  └─ 成功：把 pool 所有权交给 growth-api

growth-api
  ├─ HTTP 装配前失败：关闭 pool
  └─ 正常/异常停止：进程生命周期统一关闭 pool
```

### 7.2 Migration 所有权链

```text
OpenMigration
  ├─ 构造/Ping 失败：内部关闭
  └─ 成功：调用方暂时拥有 *sql.DB
             │
             ▼
dbmigration.New(ctx, fs, ownedDB, config)
  ├─ 从收到非 nil ownedDB 起接管
  ├─ source/config/adapter 构造失败：New 关闭 ownedDB
  └─ 成功：Runner 成为唯一 owner
             │
             ▼
Runner.Close（幂等）关闭 source、专用 conn 与 pool
```

强所有权契约的价值在失败路径：调用方知道 `New` 返回错误后不能再补一次 Close；Runner 构造函数也不能把一半初始化资源留给不知道内部状态的上层猜测。

## 8. 并发模型：串行不是性能缺陷，而是 schema 的一致性选择

`Runner` 用 mutex 串行化 `Up`、`Status` 与 `Close`。这意味着一个进程内不会在读取版本的同时关闭连接，也不会并发执行两个 `Up`。跨进程并发由 MySQL adapter 的 advisory lock 再约束。

Migration 池只有一个连接，不是因为 DDL 永远只需一个连接，而是 MySQL `GET_LOCK` 必须在同一 session 释放。如果允许池切换连接，可能出现 A 连接获取锁、B 连接执行/释放，最终遗留锁或错误判断。单连接把 session 身份与 Runner 生命周期对齐。

API 池则允许有限并发；“max open = 10”是上限而不是预建 10 条连接。真正容量应按以下总账推导：

```text
每实例 maxOpen × 同时存活实例数 × 滚动发布放大系数
  + Migration/运维连接
  + 其他应用连接
  < MySQL 可用连接预算（保留管理与故障余量）
```

第 13 节没有负载数据，因此 10/10 只是受控起点，不是容量结论。

## 9. 超时预算：timeout 不是越短越安全

不同 timeout 保护的是不同资源：

- connect timeout 限制握手和建连；
- API read/write timeout 限制单连接网络 I/O；
- startup/readiness Ping timeout 限制启动和探针占用；
- Migration statement timeout 限制单个 SQL；
- Migration network read timeout 给 statement 取消后的协议响应留空间；
- Migration lock timeout 只限制 `golang-migrate` 获取数据库锁的外层等待；它不包住后续 DDL，也不限制解锁。

当前 Migration 默认关系为：

```text
statement 30s + 5s <= network read 35s
network read 35s + 5s <= lock-acquire 40s
```

五秒不是精确物理常数，而是显式调度/取消余量。第一条关系让单条 SQL 的 statement timeout 先于驱动读取超时；第二条只作用于取锁阶段，让连接读取截止先于 `golang-migrate` 的外层取锁截止。上游 MySQL adapter 的 `GET_LOCK` 固定在服务端最多等待 10 秒，因此外层 lock 小于或等于 10 秒可能先返回，而取锁 goroutine 仍在等待；配置将最小值设为 11 秒。锁一旦取得，`LockTimeout` 就不再计时，整次迁移仍可能持续多个 statement timeout；当前上游 `unlock` 也没有由这个参数提供截止时间。

取消也只能在 Migration 边界安全生效。`GracefulStop` 不承诺中断正在执行的任意 DDL；当前 statement timeout 才限制单语句。如果 `Up` 已经进入 engine 后 context 才取消，Runner 会请求 graceful stop，等待 engine 到安全边界、重新检查 dirty，并把自身标成 terminal。若 context 在启动 engine 前已经取消，调用会直接失败但 Runner 尚未进入不确定执行状态，因此不会被标成 terminal。执行中取消后的 terminal Runner 不能复用，因为其 source/adapter 内部是否仍可继续不值得猜测。

## 10. 失败模型：重点是“部分改变之后怎么办”

### 10.1 建连失败

可能发生在配置校验、TLS 根加载、connector 构造、认证或 Ping。系统在 HTTP 监听前失败并关闭池。安全输出只说明阶段，不复制密码、DSN 或驱动详情。

### 10.2 DDL 部分成功与 dirty

典型序列是 Migration 引擎先将目标版本标为 dirty，再执行 SQL，成功后写 clean。若多语句文件第二条失败，第一条 DDL 可能已经隐式提交，版本表则仍 dirty。此时：

- 不能把 dirty 当成“数据库没变”；
- 不能直接重跑并期待幂等；
- 不能只 `force` 版本表为 clean；
- 必须比较预期 SQL、实际 `information_schema`、版本表和业务数据，再决定前向修复。

产品 CLI 不暴露 `force` 正是为了让这一检查不可被一个便捷命令绕过。

### 10.3 版本 mismatch

若版本表中的 clean version 不在当前二进制嵌入的版本集合中，可能意味着：

- 用旧 migration 二进制连接了已升级数据库；
- 分支历史发生分叉或有人改写了已发布 Migration；
- 连接到了错误环境/schema；
- 版本表被人工修改。

`status` 不把这种情况误报为 clean，而是 `migration_version_mismatch`。版本号小于 latest 但属于同一历史才是可解释的 `pending`。

### 10.4 旧 API 二进制

当前 API 没有 schema exact-version gate，所以旧 API 只要能连接就可能启动。这在第 13 节没有业务表时不产生 schema 兼容问题，但未来必须靠 expand/contract 保证滚动窗口。精确相等 gate 反而可能在合法的向前兼容迁移后把全部旧实例摘流，造成发布中断；正确问题应是“该二进制支持哪个版本区间/哪些能力”，不是“是否等于最新数字”。

### 10.5 锁、网络和取消竞争

- 两个 Migrator 同时启动：advisory lock 使其中一个等待/失败，不能并行改变历史；
- 网络在 DDL 已提交后断开：客户端看到错误不代表服务端未执行，必须查实际状态；
- context 取消与 Up 完成同时发生：Runner fail closed，并在退出前检查 dirty；
- Close 与 Up 并发：进程内 mutex 保证 Close 等待当前操作边界；
- 数据库重启：API pool 可能丢失旧连接，readiness 通过后续 Ping 反映恢复，liveness 保持独立。

### 10.6 无迁移的边界情况

空 source 被当作成功状态，而不是错误或伪造版本 0。这样第 13 节可以验收机制，又不为课程外观创建没有业务含义的历史。需要注意：生产命令仍先建连/Ping，证明身份与数据库可达；Runner 只是不会建立 `schema_migrations`。

## 11. 安全与隐私：从能力最小化推导防线

### 11.1 身份与授权

- API 账号只获得未来业务需要的 DML，不拥有 DDL；
- Migrator 只在发布作业中使用，不进入 API 进程；
- 管理员身份不进入应用配置；
- 集成测试必须实际证明 API DDL 收到 MySQL 1142，而不是只检查代码中没有 `CREATE`。

### 11.2 认证与传输

项目以 MySQL 8.4 的 `caching_sha2_password` 为基线，显式拒绝 legacy native、old、cleartext 和 fallback-to-plaintext 选项。staging/production 必须 `verify_identity`：至少 TLS 1.2，证书链使用系统根加可选 CA，`ServerName` 从连接地址 host 推导，不能仅加密而不验身份。

development/test 允许显式 disabled，是本地便利，不是生产建议。未来若使用代理或服务发现，地址 host 与证书 SAN 的关系必须重新验证。

### 11.3 秘密与输出

结构化 driver Config 避免先生成一条容易被复制到日志的 DSN；含秘密配置在 `String`、`GoString`、slog 和 JSON 常见边界整体脱敏。CA 文件读取错误不回显原路径。错误 `Unwrap` 的存在是给受控程序判断，不是给 HTTP/logger 直接打印。

仍需承认：任何持有配置对象的内存进程理论上都能读取密码，脱敏方法不能代替 Secret 注入、进程隔离、最小访问和轮换。

### 11.4 SQL 供应链

embed 防止运行时目录被替换，但不能证明 SQL 正确。未来应让 Migration 文件经过代码评审、构建制品追踪和发布审批；若合规要求提高，可增加 migration checksum、制品签名和构建来源证明。

## 12. 测试怎样“证明”，而不只是变绿

测试设计从要推翻的错误命题出发。

| 要证明的主张 | 有效反例 | 当前证据怎样捕获 | 仍不能证明什么 |
| --- | --- | --- | --- |
| API 不拥有 DDL | API 成功创建表 | 真实双账号测试要求 API DDL 返回 1142 | 不能证明所有环境授权都永久正确，部署仍需 `SHOW GRANTS`/策略审计 |
| multi-statements 没扩散到 API | API 一次执行两条语句成功 | driver config 单测 + Migration 专用多语句集成测试 | 不能证明未来代码不会绕过该构造器 |
| 空产品迁移集不污染历史 | 产生 `schema_migrations` 或占用 000001 | MapFS no-migrations 单测 + 真实库检查 | 不能证明第 18 节首个 SQL 设计正确 |
| dirty 不会被当成功 | engine 返回错误但数据库 dirty | fake engine 注入、dirty/version 状态机测试 | 不能覆盖所有 MySQL DDL 部分提交组合 |
| 版本漂移 fail closed | 旧二进制把陌生版本报告 clean | known-version 集合正反测试 | API 当前没有 schema compatibility gate |
| 执行中的 Up 被取消后不会让 Runner 假装可复用 | engine 已启动后 context 取消，再次 Status/Up 却成功 | blocking fake、terminal 状态测试 | 不能让不可取消 DDL 瞬间停止；也不把启动前已取消误判为 terminal |
| 所有权没有泄漏/double close | constructor 中途失败留下 pool | fake connector/closer 的失败路径计数 | 不能替代长时间连接泄漏观测 |
| readiness 与 liveness 分离 | DB 故障让 `/health` 失败 | handler 单测 + 真实 kill 连接 HTTP 冒烟 | 不能证明业务查询 P99 或 schema 可用 |
| 普通测试没有伪造联调 | 无环境时 integration 被 skip 却被宣传 PASS | 普通测试允许 skip；Make 目标先强制八个变量，再精确运行 Integration | 不能证明 CI/生产使用了正确目标，流程仍需门禁 |

### 12.1 为什么既要 fake 又要真实 MySQL

fake/MapFS 能穷举构造失败、取消竞争、typed nil、重复版本等难以稳定制造的状态，并精确统计 Close；真实 MySQL 才能证明认证插件、字符集/时区、MySQL 错误号、多语句、DDL 授权和 adapter 行为。两者是互补证据，不是“集成测试更高级所以可删除单测”。

### 12.2 为什么清理也是证据

集成测试使用随机隔离表和独立版本表，Runner 关闭后再开 cleanup 连接精确删除。若测试通过却遗留账号、表或 schema，它仍然破坏了共享环境，不能算完整证据。清理目标必须可枚举，不能用宽泛 DROP 影响用户原有数据。

### 12.3 绿色测试的边界

当前绿色不能证明大表 DDL 不锁表、连接池容量合理、证书轮换无中断、备份可恢复、复制延迟可接受或滚动发布兼容。这些必须等有真实 schema、部署形态和数据规模后建立新证据。

## 13. 被刻意推迟的能力与重决策条件

| 推迟能力 | 为什么现在不做 | 什么时候必须重评 |
| --- | --- | --- |
| 第一批业务表 | 领域对象与查询在第 17～18 节才确定 | Strategy/Award 不变量和查询模式完成评审 |
| Repository 与事务管理器 | 没有业务用例，无法定义原子边界 | 第 19～20 节出现跨多条 SQL 的应用用例 |
| API schema exact-version gate | 没有 schema/兼容区间；精确相等妨碍滚动发布 | 首个 Migration 发布前，定义应用支持的 schema 范围 |
| 自动 Migration | 会扩大在线进程权限和故障域 | 只有部署平台无法可靠执行独立 Job，且能证明等价隔离时 |
| down/force 产品命令 | 无法普遍保证数据可逆 | 不作为普通功能重评；仅事故流程在审批、备份、实际 schema 对比后处理 |
| checksum/签名历史 | 当前单仓库、空迁移集，收益有限 | 多团队/多制品发布、审计要求或历史被改写事件出现 |
| 在线 DDL 工具 | 还没有大表 | 表规模、锁窗口或可用性目标超过普通 DDL 能力 |
| 读写分离/代理 | 没有查询和负载证据 | 连接总账、读负载、复制与故障转移需求形成数据 |
| Redis | 不是关系事实的替代品 | 抽奖热点和库存竞争产生可测瓶颈 |
| DB tracing/SQL metrics | 第 94 节才统一 OTel，当前无业务 SQL | Repository 出现后先测 query latency/error，再决定采集粒度与脱敏 |

## 14. 架构师会主动想到、但原始需求没有明说的点

1. **连接预算是集群级而非单实例配置。** 滚动发布会暂时放大实例数，Migrator、DBA 和监控也需要保留连接。
2. **发布顺序需要兼容窗口。** 独立 CLI 只是执行入口，不自动保证 SQL 对旧二进制兼容。
3. **readiness 可能产生探针放大。** 实例数、探针频率和数据库故障时的重试同步会形成额外压力，部署章节需要加抖动/频率预算。
4. **Migration advisory lock 名称属于共享命名空间。** 未来多 schema/租户或同库多应用时，要避免锁 ID 冲突并明确作用域。
5. **DDL 算法与元数据锁需要发布前评估。** `ALTER TABLE` 的 `ALGORITHM/LOCK`、表大小、磁盘临时空间和长事务都可能改变风险。
6. **备份存在不等于可恢复。** 首次破坏性变更前必须做恢复演练并记录 RTO/RPO，而不是只勾选“已备份”。
7. **复制/binlog 语义。** 若以后有副本，DDL 和大批数据回填会影响 lag、故障转移和只读流量。
8. **秘密与证书轮换。** 长连接不会因环境变量更新自动换凭据；需要决定 drain/restart、双凭据窗口和 CA 轮换顺序。
9. **构建制品与 SQL 同源。** embed 让二进制携带确定历史，但也要求发布时确认执行的 migration 二进制与目标应用版本对应。
10. **错误脱敏和可诊断性的张力。** 通用日志只记录 stage 后，深入诊断需要受控管理员通道，而不是诱使开发者重新打印原始 cause。
11. **字符集、排序规则与时区会成为数据契约。** 当前只固定连接字符集/时区，真正建表时还要显式决定 schema/table collation 与时间字段语义。
12. **测试数据库权限必须近似真实环境。** 用 root 跑绿的集成测试无法证明 API 最小权限；本节因此把 1142 作为正向证据。
13. **旧二进制不只是 API。** 旧 `growth-migrate` 连接新数据库会被 version mismatch 拒绝；发布系统应固定制品，不从开发机任意运行旧命令。
14. **多语句扩大审查单位。** Migration 文件中的每条语句都可能部分成功；文件应小而聚焦，不能把无关 DDL 打成一个“方便”的大脚本。

## 15. 假设与风险账本

| ID | 当前假设/风险 | 当前证据 | 假设失效的影响 | 观察信号与复核点 |
| --- | --- | --- | --- | --- |
| A13-01 | MySQL 8.4 与团队运维基线匹配 | 本地 8.4.11 联调、路线技术基线 | 驱动、SQL、认证或运维流程重做 | 首个云/预发环境落地时复核版本与参数 |
| A13-02 | `sqlx` 的轻量便利足够支持前几节 Repository | 当前仍保留手写 SQL 可见性 | 样板或映射错误快速增长 | 第 24 节统计查询数量、映射缺陷和测试成本 |
| A13-03 | 默认 API pool 10/10 不会耗尽 MySQL | 只有边界测试，没有容量数据 | 多实例导致连接拒绝或管理连接不足 | 第 16 节 Compose 与第 24/40 节压测建立总账 |
| A13-04 | 3 秒 Ping 适合当前部署网络 | 本地故障冒烟 | 短暂网络抖动造成 readiness 摘流或启动失败 | 预发 latency 分布、DNS/TLS 握手数据 |
| A13-05 | 前向兼容发布足以替代通用 down | MySQL DDL 风险分析，尚无真实发布 | 设计不兼容时难以快速回退 | 第 18 节首个 Migration 必须提交兼容/恢复方案 |
| A13-06 | 单 Migrator + advisory lock 足够 | adapter/集成测试 | 多区域或多管线重复执行导致锁等待/超时 | 第一次 CI/CD 并发发布演练 |
| A13-07 | 通用 stage 日志足够日常诊断 | 隐私测试、当前失败类型有限 | 值班无法快速定位根因 | 真实故障 MTTR；必要时设计受控诊断通道 |
| A13-08 | `verify_identity` 的地址 host 与证书 SAN 一致 | 配置/TLS 单测，真实联调使用 disabled | 预发连接失败或被迫降级 | 第 16 节/预发证书联调，禁止用 skip-verify 绕过 |
| A13-09 | 无 API schema gate 在早期可接受 | 当前无业务表 | 首个不兼容变更导致旧实例运行错误 | 第 18 节前定义兼容版本模型和部署顺序 |
| A13-10 | embed 的 Migration 历史不会被共享后改写 | Git 规范、source 命名校验 | 分支历史分叉、版本号相同但 SQL 不同 | 首个 Migration 时引入历史 diff/checksum 评审 |
| R13-01 | DDL 部分提交留下 dirty | 已知 MySQL 行为，Runner fail closed | 发布停止，需要人工恢复 | 每条真实 Migration 的影子演练和 Runbook 记录 |
| R13-02 | readiness 在 DB 故障时形成同步 Ping 洪峰 | 当前没有部署探针配置 | 加重故障数据库负载 | 第 16 节按实例数计算探针频率与阈值 |
| R13-03 | 密码/CA 轮换需要重启连接池 | 当前无动态 reload | 轮换窗口出现认证失败 | 部署 Secret 轮换演练与连接 drain 设计 |

## 16. 未来演进必须回答的问题

### 第 16 节前

- Compose 中如何创建最小权限双账号而不把密码提交到仓库？
- API、MySQL healthcheck 与 Migration Job 的启动顺序是什么？
- 探针频率和失败阈值会不会把 MySQL 故障放大？

### 第 18 节首个 schema 前

- Strategy/Award 的事实所有权、唯一约束、时间字段和查询路径是什么？
- `000001` 对旧/新二进制的兼容区间如何定义？
- MySQL collation、索引长度、ID 策略和审计字段依据是什么？
- 首次建表失败、重复执行和环境已有同名对象时如何停止与恢复？

### 第 19～24 节 Repository 与 MVP

- 事务由应用用例还是 Repository 开启，跨 Repository 如何传递？
- 是否需要 `TxManager`，它能否不把 `sqlx.Tx` 泄漏到领域层？
- query timeout、慢查询、连接等待和错误分类如何观测？
- API 应检查 schema 最低版本、版本区间还是具体 capability？

### 更远阶段

- 大表变更何时转向在线 DDL/变更平台？
- Redis/MQ 引入后 readiness 包含哪些依赖，哪些采用降级而非摘流？
- 多区域、只读副本、备份恢复和灾备将如何改变连接与 Migration 所有权？

## 17. 可追溯证据

### 决策与课程

- [第 13 节课程正文](../../course/part-02/lesson-13-mysql-migrations.md)
- [ADR-0010：MySQL 连接、账号隔离与前向 Migration 边界](../../decisions/ADR-0010-mysql-migration-boundaries.md)
- [第 13 节 API 记录](../../api/lessons/lesson-13.md)
- [第 13 节 QA 验收证据](../../qa/lessons/lesson-13.md)
- [MySQL Migration 运维手册](../../runbooks/mysql-migrations.md)
- [配置参考](../../configuration.md)
- [仓库地图](../../architecture/repository-map.md)

### 运行时代码

- [MySQL 类型化连接与 TLS 配置](../../../internal/infrastructure/mysql/config.go)
- [API/Migration 连接池构造与 Ping](../../../internal/infrastructure/mysql/open.go)
- [安全阶段错误](../../../internal/infrastructure/mysql/error.go)
- [前向 Migration Runner](../../../internal/infrastructure/migration/runner.go)
- [Migration 错误与 dirty/version sentinel](../../../internal/infrastructure/migration/error.go)
- [嵌入式 Migration source](../../../migrations/embed.go)
- [Migration 目录规则](../../../migrations/README.md)
- [API 数据库装配](../../../cmd/growth-api/database.go)
- [readiness handler](../../../internal/infrastructure/httpapi/readiness.go)
- [Migration CLI](../../../cmd/growth-migrate/main.go)
- [Migration 生产装配与所有权转移](../../../cmd/growth-migrate/production.go)
- [本地数据库与集成测试入口](../../../Makefile)

### 证明性质的测试

- [MySQL connector 单元测试](../../../internal/infrastructure/mysql/config_test.go)
- [MySQL 双身份真实集成测试](../../../internal/infrastructure/mysql/mysql_integration_test.go)
- [Runner 状态机、取消和所有权测试](../../../internal/infrastructure/migration/runner_test.go)
- [Runner 真实 MySQL 集成测试](../../../internal/infrastructure/migration/migration_integration_test.go)
- [readiness 契约测试](../../../internal/infrastructure/httpapi/readiness_test.go)
- [Migration 命令装配测试](../../../cmd/growth-migrate/production_test.go)

## 18. 本节设计结论

第 13 节交付的核心不是一张表，而是一条以后每张表都必须经过的受控路径：低权限在线连接负责业务流量，独立高权限身份负责经过评审的前向结构变化；liveness、readiness、版本状态和业务正确性分别回答不同问题；错误、取消和版本漂移默认停止，而不是猜测性继续。

没有业务表、没有事务管理器、没有 API schema exact-version gate，是三个相互一致的范围选择：此刻还没有足够业务事实定义 schema，也没有用例定义事务，更没有滚动兼容模型定义 API 应接受哪些 schema。先把这些未知诚实地保留下来，比提前提供一个貌似完整、实则无法证明正确的终态更符合演进式架构。
