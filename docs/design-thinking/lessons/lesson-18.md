# 第 18 节第一性原理设计手记：从合法领域对象到可恢复的关系结构

本文不从“我要用 MySQL、所以建几列”出发，而从事实源、身份、完整性、失败、权限和演进成本逐层推导第 18 节。结论只适用于 2026-08-29 的当前切片：两张 Lottery 配置表、两条 Migration、真实 MySQL 约束验证和 Compose 精确授权生命周期已经进入实现；Repository、算法、业务 API、真实前端与 Redis 业务仍未发生。

## 1. 决策命题与时间切片：本节保存配置，不执行抽奖

### 1.1 真正的决策命题

第 17 节已经能在内存中构造合法 Strategy/Award。如果进程退出，这些对象不会留下；如果直接让任意 SQL 写行，数据库又可能保存领域无法加载的状态。

因此本节核心问题是：

> 在不提前实现 Repository、算法和在线接口的前提下，怎样把 Lottery 配置映射为最小关系结构，使数据库独立守住它能够证明的局部事实，明确暴露它无法证明的跨行/Unicode 事实，并让迁移和权限失败在 API 接流量前停止；同时保留足够清楚的演进路线，而不预建没有需求的状态、索引和缓存协议？

### 1.2 本节完成的时间切片

- `lottery_strategy` 聚合根表；
- `lottery_strategy_award` 子实体表；
- 完整 `uint64` 映射、复合身份、父子外键、Outcome/Weight/基本名称约束；
- 每表行级创建/更新时间；
- 两条单 DDL、不可变的前向 Migration；
- isolated MySQL 结构/数据/权限集成测试；
- Compose 的 migration→grant reconciliation→API gate；
- 应用身份的两表 SELECT only；
- 直接 grant allowlist 与 mandatory role 空值验证。

### 1.3 本节没有完成的时间切片

- 没有从表恢复 Strategy 的 Repository；
- 没有创建、更新、删除或发布 Strategy 的业务用例；
- 没有事务性保证“Strategy 对外可见时至少一个 Award”；
- 没有随机算法、Draw、幂等、库存或权益；
- 没有 `/api/lottery`；
- 没有 React 真实数据；
- 没有 Redis 缓存；
- 没有生产在线 DDL、备份/恢复、主从复制或容量结论。

### 1.4 可重放推理链

```text
领域对象需要跨进程恢复
→ 需要持久化结构，但表行不能替代领域对象
→ Strategy 与多 Award 是一对多关系
→ 两表；Award 身份只在 Strategy 内唯一
→ (strategy_id, award_id) 复合主键
→ 孤儿 Award 没有合法含义
→ 外键；当前无删除用例
→ RESTRICT 而非 CASCADE
→ Go ID/Weight 是完整 uint64
→ BIGINT UNSIGNED
→ Outcome 是封闭可读词汇
→ ascii_bin VARCHAR + CHECK
→ 名称要保存完整 Unicode 且不要静默折叠
→ utf8mb4_0900_bin + VARCHAR(128)
→ SQL 不能完整复制 Go Unicode 规则
→ 只命名为 name_basic，Repository 再构造 fail closed
→ MySQL atomic DDL 是单语句边界
→ 两张表拆为两个 Migration
→ API 还没有业务写用例
→ 应用只需要两表 SELECT，版本表不可见
→ 已有 volume 不会重跑 init
→ socket-only one-shot 撤权/授权/完整验证
→ Migration 与 grant 都成功后 API 才启动
```

## 2. 不可争辩的事实、约束与假设

### 2.1 当前代码事实

| 事实 | 能证明 | 不能证明 |
| --- | --- | --- |
| 两个 `.up.sql` 各含一条 CREATE TABLE | 版本 1/2 的目标 DDL 可审查 | 所有环境执行成功 |
| checksum 测试固定初始字节 | 后续误改会让测试失败 | 内容本身一定正确、来源已签名 |
| Strategy/Award 列使用 `BIGINT UNSIGNED` | 列可覆盖 uint64 范围 | API/JS/其他数据库能自动无损处理 |
| 复合 PK `(strategy_id, award_id)` | 同策略 scoped identity 被约束 | AwardID 全局唯一 |
| RESTRICT 外键 | 无孤儿且父行不能隐式删除 | Strategy 一定有子行 |
| `name_basic` CHECK | 非空、默认 TRIM 的 U+0020 子集 | Go TrimSpace、控制字符、NFC、反欺骗 |
| Weight CHECK > 0 | 每一行权重为正 | 多行和不溢出、算法公平 |
| Outcome CHECK | 只有两种存储值 | reward 已发放、no_reward 已发生 |
| 行 `updated_at` 自动刷新 | 该行被 SQL 更新的时间元数据 | 聚合版本、父表随子表更新、缓存失效 |
| API direct grants 是两表 SELECT | 直接 grant 白名单可验证 | 如果 mandatory roles/代理权限存在，有效权限仍相同 |
| 脚本断言 mandatory_roles 为空 | 本地 fixed topology 排除强制角色隐式扩权 | 所有 MySQL 权限机制已形式化证明 |
| API depends on grants success | 正常 Compose 启动 gate | 运行中权限永不漂移、外部手工启动不会绕过 |

### 2.2 平台事实

- MySQL 8.4 支持 enforced CHECK，但 CHECK 只能引用当前行可允许的表达式，不能方便表达“父行至少一个子行”或跨行安全求和；
- InnoDB 外键可以拒绝孤儿与受引用删除；
- DDL 会隐式提交，atomic DDL 的原子性以单条受支持语句为单位；
- MySQL 官方镜像 init 脚本只在数据目录首次初始化时运行；
- Docker 镜像 VOLUME 可能为客户端型 one-shot 生成匿名 volume；
- Unix socket 不需要 IP 网络，但仍是进程间通信和高权限数据库入口；
- `SHOW GRANTS` 展示直接授权/角色语句，但 MySQL mandatory roles 还能改变有效权限；
- Go `uint64` 与 MySQL unsigned bigint 对应，但 JavaScript safe integer 范围更窄；
- Go `strings.TrimSpace` 与 MySQL 默认 `TRIM` 语义不同。

### 2.3 业务事实

- Strategy 是 Lottery 配置，不是 Marketing Activity；
- Award 是候选，不是 Benefit 实物/积分本身；
- AwardID 的唯一范围是 Strategy；
- `reward` / `no_reward` 都是合法候选类型；
- Weight 是相对选择质量，不是库存或固定百分比；
- 合法 Strategy 至少一个 Award，总权重不能溢出；
- 当前没有删除、排序、发布、版本或审批需求。

### 2.4 当前假设

1. 单个 Strategy 的 Award 数量足够小，未来整聚合加载可接受；尚无生产数据证明。
2. 二进制敏感名称比较比语言学折叠更安全地保留原始事实；未来搜索另行设计。
3. 外键带来的写入/删除约束成本值得换取本地完整性；尚无极端写吞吐证据反驳。
4. 第 19 节最先需要读取，暂不需要业务写；因此 SELECT only 合理。
5. fixed MySQL 8.4.11 的 SHOW GRANTS 文本可作为本地 allowlist；升级时必须复核。
6. `created_at/updated_at` 的基础诊断价值超过两列成本，但团队能遵守“非聚合版本”约束。

### 2.5 不可越界的描述

- 不得说“数据库完整保证 Strategy 合法”；
- 不得说“表已经实现 Repository/抽奖”；
- 不得说“updated_at 是版本号”；
- 不得说“binary collation 做了 Unicode normalization”；
- 不得说“network_mode:none 意味着没有通信”；
- 不得说“SHOW GRANTS 相等就覆盖所有外部权限来源”，必须同时说明 mandatory role 检查与 fixed topology；
- 不得说“两个 Migration 构成事务”；
- 不得说“SELECT only 是永久权限模型”。

## 3. 为什么现在做、为什么不多做

### 3.1 为什么领域对象之后立即建表

先有对象使 DDL 能从不变量推导；建表又为 Repository 提供真实边界。若先写 Repository，SQL 会围绕不存在或临时的表演进；若先写算法，配置只能硬编码；若先写 API，就会用 Mock/JSON 替代事实源。

第 18 节把问题压缩成“能否无损保存、拒绝明显非法状态、可控迁移和授权”。这是一个可独立证伪的切片。

### 3.2 为什么不同时写 Repository

schema 与 Repository 的失败模型不同：

- schema 失败关注 DDL、键、类型、约束、版本和锁；
- Repository 失败关注查询形状、扫描、事务、一致读取、not found、坏行和领域重建。

如果同节完成，测试绿色可能只证明 SQL 能返回行，却掩盖表本身约束不足；也可能让为了某个临时查询添加的索引被误认为长期 schema 事实。分开后，第 19 节必须明确消费本节契约。

### 3.3 为什么不建 DrawResult

Strategy/Award 是配置，DrawResult 是一次事件事实。后者需要 request/draw identity、用户/参与、策略版本或快照、结果唯一性、超时未知、幂等和 Benefit 关联。当前都没有，提前建表只会发明字段。

### 3.4 为什么不使用 Redis

表首次出现时还没有读取路径，更没有热点、延迟或缓存命中证据。现在接 Redis 会先产生 key/TTL/失效责任，再去寻找消费者。第 24 节必须在 Repository/API 真实存在后决定缓存派生数据，而不是让缓存成为第二事实源。

### 3.5 为什么不增加“可能有用”的索引

索引服务查询，不服务想象。当前唯一可从聚合推导的形状是按 `strategy_id` 取 Awards，复合主键已经覆盖最左前缀。对 name、outcome、updated_at 建索引会增加每次写入、DDL、buffer pool 和审查成本，却没有查询、基数或 EXPLAIN 证据。

### 3.6 为什么此时反而要做权限收敛

最小权限只能基于实际对象/语句。业务表未出现前，通配 DML 是临时占位；真实表出现而 Repository 仍不存在时，最小权限恰好是两表 SELECT。此时不收敛，`schema_migrations` 会一直暴露给在线账号，后续能力越多越难撤销。

## 4. 从第一性维度推导需求

每个维度按“事实 → 风险 → 机制 → 证据 → 边界”展开。

### 4.1 事实源

**事实：** Strategy/Award 合法性由 Lottery 领域定义，MySQL 是权威持久化载体但不是领域规则的唯一表达语言。

**风险：** 让表行结构成为领域对象，会把 NULL、默认值、SQL 字符函数和 join 结果误当业务语义。

**机制：** 表只保存必要字段；数据库守局部约束；第 19 节必须通过领域构造器恢复。

**证据：** schema 约束集成测试 + 领域构造测试；未来需要坏行 hydration 测试。

**边界：** 本节还没有实现 hydration，因此只建立强制交接条件。

### 4.2 身份

**事实：** AwardID 只在 Strategy 内唯一。

**风险：** 全局单列主键会加强不存在的规则；只用 AwardID 又会碰撞。

**机制：** `(strategy_id, award_id)` 复合主键。

**证据：** 同策略重复被 1062 拒绝；不同 Strategy 可共享 AwardID。

**边界：** 未来外部资源 URL、事件或 Benefit 引用必须携带组合身份或新增独立全局 identity，不能隐式只传 AwardID。

### 4.3 数值完整性

**事实：** ID/Weight 是正 uint64。

**风险：** 有符号 bigint 截断上半区；ORM/driver 扫描到 int64；总和跨行溢出。

**机制：** `BIGINT UNSIGNED` + CHECK >0；真实 MaxUint64 round-trip；总和仍由领域构造。

**证据：** 集成测试读回 MaxUint64；零值 3819。

**边界：** SQL `SUM()` 的返回类型、Go scan target 和 JSON 表示需各层重新审查。

### 4.4 父子完整性

**事实：** Award 脱离 Strategy 没有合法含义。

**风险：** 脚本、迁移或未来 adapter 留下孤儿；父删除静默丢失多行。

**机制：** FK + RESTRICT。

**证据：** 孤儿 1452、父删除 1451。

**边界：** FK 方向不能保证父至少一子，也不能提供聚合事务版本。

### 4.5 值域

**事实：** Outcome 只有两个稳定词汇，大小写有意义。

**风险：** 自由字符串产生拼写/大小写漂移；布尔丢失语言；ENUM 把演进锁进类型内部序号语义。

**机制：** ascii_bin VARCHAR + CHECK。

**证据：** `reward`/`no_reward` 成功，`REWARD` 3819。

**边界：** 若将来 Outcome 有独立数据/生命周期，CHECK 不再是最佳模型。

### 4.6 字符与比较

**事实：** 名称可含中文/四字节 Unicode，名称不是身份，当前无语言学搜索。

**风险：** `utf8mb3` 丢字符；不区分 collation 静默折叠；声称 SQL 完整复制 Go Unicode 规则。

**机制：** utf8mb4_0900_bin，128 字符；`name_basic` 只负责可证明子集。

**证据：** 128 中文成功、129 失败；NFD/NFC 原始值区分。

**边界：** rune、MySQL character、grapheme cluster 并非完全等价；第 19 节仍需 Go 构造。

### 4.7 迁移原子性

**事实：** MySQL DDL 会隐式提交，atomic DDL 是单条语句边界。

**风险：** 一个文件写两条 CREATE，第二条失败却假装全事务回滚；恢复人员不知道哪一步完成。

**机制：** 一文件一 DDL、v1 parent/v2 child、latest-clean API gate、dirty fail closed。

**证据：** 分号/checksum 测试；隔离故障注入得到 `2:1`。

**边界：** 单 DDL atomic 也不消除 metadata lock、磁盘、复制和版本兼容风险。

### 4.8 最小权限

**事实：** API 当前没有业务 SQL；下一节最早需要读取两表。

**风险：** schema wildcard DML 可篡改版本表和未来任意表；只追加 GRANT 无法撤销旧权限。

**机制：** REVOKE ALL 后两表 SELECT；完整 SHOW GRANTS 相等；mandatory_roles 必须空。

**证据：** SELECT 成功、业务写/版本表读写 1142、grant job exit 0。

**边界：** fixed Compose topology 外，生产 role/IAM/proxy 需重新建模；第 19 节真实写 SQL会触发权限变更。

### 4.9 发布顺序

**事实：** 表级 grant 必须在表存在后执行；API image 假设 schema v2。

**风险：** API 先启动遇到缺表/过权；grant 失败仍接流量。

**机制：** mysql healthy → migrate success → grants success → API。

**证据：** Compose dependencies 和 smoke one-shot exit checks。

**边界：** `depends_on` 是启动 gate，不是运行中持续控制器；外部平台需等价 job/readiness。

### 4.10 可逆性

**事实：** forward-only，不提供 down/force。

**风险：** 自动回滚 DDL 丢数据；为修错改写 v1/v2 历史。

**机制：** checksum 固定；任何结构修正新增 v3；dirty 人工核实。

**证据：** embed test、Runner failure semantics。

**边界：** 前向不等于零风险；发布前仍需要备份、影子库与兼容窗口。

### 4.11 可观测性

**事实：** Migration 和 grant 是 one-shot，成功/失败必须可分辨。

**风险：** 只看 API healthy，忽略初始化任务失败；日志泄露 Secret/SQL。

**机制：** exit code、stable smoke checks、版本状态和安全消息；不输出 Secret。

**证据：** `migrate/mysql-grants exited 0`、`2:0`、exact grants。

**边界：** 没有生产 metrics/traces；当前是本地验收证据。

### 4.12 时间

**事实：** 每行需要基本诊断元数据，但没有聚合版本需求。

**风险：** 无时间难排查；自动 updated_at 被误当业务版本。

**机制：** UTC 会话下 `DATETIME(6)` row metadata，并在文档中禁止扩义。

**证据：** SHOW CREATE/插入更新可观察。

**边界：** 子行更新不更新父行，数据库时间也不是业务事件时间。

### 4.13 完整证据链

```text
领域事实（lesson 17 constructors/tests）
  → DDL（two migrations）
  → MySQL enforced behavior（integration error numbers/round-trip）
  → migration state（clean v2 / dirty v2 fail closed）
  → effective local access（direct grants exact + mandatory roles empty）
  → Compose gate（migrate/grants before API）
  → existing HTTP regression checks
```

任何一层不能替代下一层：读 SQL 文本不等于 MySQL 执行；SHOW GRANTS 不等于业务 query；API healthy 不等于 Lottery 功能。

## 5. 备选方案矩阵

### 5.1 两表、JSON 或宽表

| 方案 | 完整性 | 查询/演进 | 当前结论 |
| --- | --- | --- | --- |
| Strategy + Awards JSON | 单行原子，但子项约束弱 | 局部索引/迁移复杂 | 不选 |
| 每个 Award 重复 Strategy 名称的宽表 | 无 join | 根信息重复、更新异常 | 不选 |
| Strategy/Award 两表 | FK/PK/CHECK 可见 | 聚合加载需 Repository | 选择 |

### 5.2 ID 类型

| 方案 | 范围/生成 | 代价 | 结论 |
| --- | --- | --- | --- |
| signed BIGINT auto increment | DB 生成，只有 63-bit 正范围 | 与 uint64/创建协议不符 | 不选 |
| BIGINT UNSIGNED 外部 ID | 完整 uint64 | API/跨库需显式映射 | 选择 |
| BINARY(16) UUID | 分布式 | 当前模型与调试成本改变 | 暂不选 |
| VARCHAR 业务码 | 可读 | 可变、索引大、规则未定义 | 不选 |

### 5.3 Award 主键

| 方案 | 语义 | 结论 |
| --- | --- | --- |
| `award_id` 单列 PK | 强制全局唯一 | 不符合当前领域 |
| surrogate row id + unique(strategy, award) | 多一个无业务身份键 | 没有外部需求，不选 |
| `(strategy_id, award_id)` | 直接 scoped identity | 选择 |

### 5.4 外键

| 方案 | 删除/完整性 | 结论 |
| --- | --- | --- |
| 无 FK | 应用灵活但可孤儿 | 不选 |
| CASCADE | 删除方便但隐式破坏 | 当前不选 |
| SET NULL | 产生无父 Award | 非法 |
| RESTRICT | 明确阻止 | 选择 |

### 5.5 Outcome

| 方案 | 演进与可读性 | 结论 |
| --- | --- | --- |
| bool | 弱语义 | 不选 |
| ENUM | 紧凑但列类型演进/序号语义 | 当前不选 |
| 字典表 | 适合独立元数据 | 当前过重 |
| ascii VARCHAR + CHECK | 明确、可审查 | 选择 |

### 5.6 Collation

| 方案 | 比较行为 | 结论 |
| --- | --- | --- |
| `_ai_ci` | 折叠大小写/重音，便于搜索 | 当前无搜索，可能丢原始区分 |
| binary data type | 字节级但字符串函数/字符语义更弱 | 不选 |
| `utf8mb4_0900_bin` | Unicode 存储 + 二进制敏感比较 | 选择 |

### 5.7 Migration 粒度

| 方案 | 失败现场 | 结论 |
| --- | --- | --- |
| 一文件两 CREATE | 文件内可能部分提交 | 不选 |
| 两版本各一 CREATE | v1 完成/v2 dirty 清楚 | 选择 |
| 一个存储过程包住 | 复杂且不改变 DDL事务本质 | 不选 |

### 5.8 时间戳/版本

| 方案 | 可表达 | 结论 |
| --- | --- | --- |
| 无时间列 | 最小但诊断弱 | 不选 |
| row timestamps | 行级元数据 | 选择，禁止当版本 |
| aggregate version | 并发 CAS | 没有写用例，推迟 |
| temporal/audit table | 历史重放 | 当前无需求 |

### 5.9 应用权限

| 方案 | 风险 | 结论 |
| --- | --- | --- |
| root/migrator 共用 | 极高 | 禁止 |
| schema wildcard DML | 可碰版本表/未来表 | 撤销 |
| 两表 CRUD | 当前无写路径 | 过权 |
| 两表 SELECT | 支撑下一节读取，最小 | 选择 |
| 无权限 | 第 19 节无法读，且 readiness 仅 Ping掩盖 | 不选 |

### 5.10 授权生命周期

| 方案 | 新/旧 volume | 结论 |
| --- | --- | --- |
| init only | 仅新 volume | 不足 |
| 手工 SQL | 不可重复/易漂移 | 不选 |
| API 自授权 | 长期持高权 | 禁止 |
| socket-only one-shot reconcile | 新旧都适用、可 gate | 选择 |

## 6. 不变量与信任边界

### 6.1 数据库直接不变量

1. 两表非空主键；
2. ID、Weight 大于零；
3. 同 Strategy 内 AwardID 唯一；
4. Award 父 Strategy 存在；
5. 父子 ID 更新/删除不级联；
6. Outcome 在两个精确值内；
7. 名称长度不超过列上限；
8. 名称通过 `name_basic`；
9. 行时间戳非空并由 MySQL初始化。

### 6.2 领域但非数据库直接不变量

1. Strategy 至少一个 Award；
2. 所有 Weight 总和不溢出；
3. 名称通过完整 Go UTF-8/TrimSpace/control/128-rune 契约；
4. 聚合内集合顺序按 AwardID 规范化；
5. 输入/输出集合所有权；
6. reward/no_reward 的领域解释。

### 6.3 权限信任边界

| 身份 | 信任/能力 | 不能做 |
| --- | --- | --- |
| MySQL root | 账号和所有数据控制 | 不进入 API/Migrator，不走网络 |
| Migrator | 目标 schema 审核 DDL/DML | 不处理在线请求，不使用 root |
| grant job | 短时 root over socket，固定脚本 | 无网络、无真实数据目录、无任意命令接口 |
| API app | 两表 SELECT | 无 schema_migrations、无业务写、无 DDL |
| Web/Redis | 不参与 DB | 无数据库 Secret/socket |

### 6.4 从数据库到领域的零信任原则

“数据库返回成功”不等于“聚合合法”。第 19 节必须把每个 SQL 值视为不受信任输入：

- 检查 NULL/scan range；
- 用 NewAward 验证每行；
- 用 NewStrategy 验证非空/重复/总和；
- 任一失败返回稳定 corrupt/invariant error；
- 不丢弃坏行、不 trim 修复、不把空 Awards 当 not found。

## 7. 资源所有权、并发与时间预算

### 7.1 数据所有权

- `mysql_data` 只属于 MySQL server；
- `mysql_socket` 是 MySQL 与 grant job 的显式 IPC；
- grant job 的 `/var/lib/mysql` 是空只读 bind，不是数据所有权；
- Migration 文件属于发布构件，运行时不从宿主相对路径读取；
- API 连接池仍属于 API，但本节没有业务查询。

### 7.2 并发

当前 schema 没有 version/locking 协议。两个未来写者可能：

- 分别更新不同 Award，形成业务不希望的组合；
- 删除/插入集合时暂时产生空 Strategy；
- 先各自验证总和，再并发写入导致最终总和溢出；
- 用行 updated_at 错误判断聚合未变化。

因此本节不能声称支持并发编辑。第 19 节若只读，这些问题继续推迟；一旦写用例出现，必须决定事务隔离、锁/版本和整聚合替换协议。

### 7.3 Migration 并发

第 13 节 Runner 的数据库锁和单连接语义继续适用。两条单 DDL 减少文件内部模糊，但不消除 metadata lock。API 编排只在 migration 完成后启动，避免本地同版本并发接流量。

### 7.4 时间预算

- Migration statement/read/lock timeout 沿用第 13 节分层；
- grant job stop grace 10s，脚本语句短但没有被描述为生产 SLO；
- schema integration 总 context 45s，各 open/statement 有界；
- API readiness 仍使用 2s Ping，不执行 schema/业务查询；
- 当前没有 Repository query budget。

### 7.5 容量

两张空/小表没有生产容量证据。复合 PK 适合按 Strategy 读取，但以下仍未知：Strategy 数量、平均 Awards、名称基数、更新频率、buffer pool、写放大、备份窗口和复制延迟。

## 8. 失败模型与恢复语义

### 8.1 v1 失败

父表 CREATE 失败时 Migration 停止，API 不启动。先核实是否已有同名不同结构对象、权限、磁盘、metadata lock 或 MySQL 版本问题。

### 8.2 v1 成功、v2 失败

父表是已完成的合法 v1；v2 可能 dirty。不能因“业务需要两表”就删除父表，也不能假设自动回滚。恢复步骤：冻结写、记录构件/checksum、查版本、SHOW CREATE/约束、确定副作用，再新增修复或受控处理。

### 8.3 授权 REVOKE 后失败

应用权限可能为空/不完整，但 API 因 dependency gate 尚未启动。这是选择 fail closed 的代价。修复表/脚本/Secret 后重跑整个 reconciliation，从全部撤权开始重建。

### 8.4 授权有额外直接 grant

完整相等检查失败，而不是记录 warning 后继续。查明是人工漂移、旧版本、role/grant 变化还是 MySQL 输出升级。

### 8.5 mandatory roles 非空

即使 direct SHOW GRANTS 恰好等于 allowlist，强制角色也可能扩大有效权限。因此 job 非零退出。不能在第 18 节“接受但记录”，因为那会让最小权限证据失真。

### 8.6 读取到领域非法数据

本节没有 Repository；第 19 节必须 fail closed。恢复人员不能在读路径自动 trim、删坏 Award 或压缩权重，因为这会改变权威业务事实且难以审计。

### 8.7 外键删除失败

1451 是保护，不是可重试瞬时错误。未来删除用例必须明确子记录处理、审计/归档和事务，不应改成 CASCADE 来“修复测试”。

### 8.8 名称 129 字符失败

strict mode 下返回 1406。不能切换非 strict 让 MySQL静默截断，因为名称变化可能造成运营误认；未来 API 应在数据库前校验并返回稳定业务错误。

### 8.9 取消、deadline 与未知结果

Migration 取消沿用 Runner terminal/dirty 语义。普通 DML 尚不存在。不能把 Migration 取消类比为抽奖结果未知；Draw 结果幂等是完全不同的后续模型。

### 8.10 恢复反模式

- 删除 `mysql_data` 让 init 重跑；
- 改写 v1/v2 SQL 和 checksum；
- 手工 force 版本而不核实对象；
- 临时给 API root/migrator；
- 关闭 CHECK/外键/strict mode 让测试过；
- 把坏行静默修正；
- 用广域 Docker prune 清理。

## 9. 安全与隐私推导

### 9.1 最小权限

能力应随用例增长：第 18 节只有读取准备，所以只授 SELECT。DDL 归 Migrator，授权归短时 job，在线 API 不持高权限。

### 9.2 有效权限而非直接文本

安全审查不仅问“SHOW GRANTS 有什么”，还问“账号实际获得什么”。强制角色是一个容易遗漏的路径。本地门禁要求 `@@GLOBAL.mandatory_roles` 为空；若将来使用 roles，应显式计算有效权限并更新模型，不能绕过检查。

### 9.3 Root Secret 生命周期

root Secret 由固定 file secret 挂载，脚本只验证和使用，不打印。容器无网络、只读、短时运行、drop capabilities；但任何能在该容器内执行任意代码的人仍可能用 socket 和 Secret 控制数据库。因此脚本/镜像/挂载是高信任供应链。

### 9.4 SQL 与错误隐私

Migration/driver 原始错误可能包含表名、SQL 或拓扑。通用日志继续只记录安全阶段；QA 可以记录稳定 MySQL error number，但不记录 Secret/DSN/内部路径。

### 9.5 名称内容

名称不是个人资料字段，但未来运营可能输入敏感/恶意文本。当前 binary storage 只保真，不做 HTML sanitization、反欺骗、敏感词或 PII 策略；输出到 UI 时仍需上下文转义。

### 9.6 数据破坏面

RESTRICT、forward-only、API read-only 和无自动 reset 共同缩小破坏面。它们不能替代备份、审计和生产恢复演练。

### 9.7 供应链

固定 MySQL 8.4.11 镜像 tag 提供版本可复现性但不是 digest pin；Migration checksum 是仓库不变性保护而非制品签名。生产需要更完整的镜像 provenance/SBOM/签名策略。

## 10. 证据设计：每个检查试图推翻什么

### 10.1 Migration checksum

试图推翻“共享的 v1/v2 可以被无声改写”。不证明 SQL 正确。

### 10.2 一条分号

试图推翻“一个版本藏多条 DDL，却声称与 atomic DDL 对齐”。不能识别存储程序等复杂语法；当前文件简单且受审查。

### 10.3 MaxUint64 round-trip

试图推翻“列/driver 只能处理 int64”。不证明未来 Repository/JSON 正确。

### 10.4 Unicode 双表示

试图推翻“collation 会把两个原始值视为同一精确匹配”。不证明 UI 显示不同或已做 normalization。

### 10.5 3819/1406/1451/1452/1062

试图推翻 CHECK、strict length、RESTRICT、FK、复合唯一未 enforced 的可能。

### 10.6 应用 1142

试图推翻 API 拥有写业务/读版本表的可能。写探针用事务回滚，防权限回归污染。

### 10.7 grant exact + mandatory roles empty

试图推翻“旧/额外 direct grant 或强制角色隐式扩权”。不覆盖 fixed topology 外的代理/IAM/外部 server 配置。

### 10.8 dirty fault injection

试图推翻“两 DDL 失败会整体回滚”这一错误认知，并验证 v2 dirty fail closed。

### 10.9 Compose smoke

试图推翻迁移/grants 未运行、旧镜像、约束标签漂移、宿主端口泄露或 HTTP 回归。它不证明生产部署。

### 10.10 负向范围检查

代码审查还要确认没有 Repository、算法、route、Redis client、前端真实接入；“没做”的证据和“做了”的证据同等重要。

## 11. 被刻意推迟的能力与触发器

| 推迟项 | 原因 | 触发器 | 归属 |
| --- | --- | --- | --- |
| Repository | 需独立设计 hydration/事务/error | 第 19 节 | Lottery adapter |
| 二级索引 | 无真实查询/EXPLAIN | Repository SQL/慢查询 | schema ADR 增量 |
| 业务写权限 | 无写 SQL | 第 19 节写用例 | grants allowlist |
| total_weight 列 | 冗余一致性未定义 | 性能证据且可维护 | schema/Repository |
| aggregate version | 无并发编辑 | 冲突用例 | domain/schema/API |
| position | 无展示排序事实 | 运营重排需求 | domain/schema |
| soft delete/status | 无生命周期 | 发布/归档需求 | product/domain |
| DrawResult | 缺请求/幂等/快照 | 结果模型章节 | Participation/Lottery |
| API | 缺 Repository/算法 | 第 21 节 | HTTP adapter |
| React 真实接入 | 缺 API | 第 22 节 | frontend |
| Redis cache | 缺真实读路径/热点 | 第 24 节 | cache adapter |
| online DDL 平台 | 无规模风险数据 | 表规模/锁 SLO | platform |
| Unicode normalization/search | 当前只需保真 | 搜索/国际化/反欺骗需求 | product/search |

## 12. 需求未提但架构师会主动检查的点

### 12.1 空 Strategy 是“坏数据”还是“草稿”

当前领域明确非法，数据库物理允许。若运营以后需要分步编辑，不能让空状态偷偷成为永久事实；应设计 draft aggregate/state 或 staging table，并定义何时转为可用配置。

### 12.2 总权重查询的 SQL 类型

即使 MySQL `SUM(BIGINT UNSIGNED)` 返回 DECIMAL 形态，扫描和边界仍需验证。最安全的第 19 节路径可能是逐行让领域构造器累加，而不是依赖一个已经可能超范围的聚合值。

### 12.3 `updated_at` 的误用压力

开发者很容易用父表 updated_at 做 ETag/缓存 key；但子表变化不触发父表。若真需要聚合 version，应显式单调递增并在同一事务更新，或采用不可变 Strategy version 模型。

### 12.4 Unicode 视觉欺骗

binary collation 保留原始数据，也意味着 `Gift`、`Ｇift`、不同 normalization、同形字符可并存。保存层不应擅自折叠，但管理 UI 可能需要可视化 code points、重复警告或规范化搜索投影。

### 12.5 复合主键与未来事件引用

事件只发 `award_id` 会丢上下文。需要在事件 schema、日志、Benefit 请求和 API 中坚持组合身份，或正式新增全局 ID；不能由调用者猜“当前恰好全局不重复”。

### 12.6 外键与未来分库

外键适合当前单 MySQL schema。若未来跨库拆分 Lottery，物理 FK 无法跨服务，必须用 outbox/防腐层/一致性巡检替代；当前无需为假想拆分放弃本地完整性。

### 12.7 MySQL CHECK 与版本基线

旧 MySQL 版本曾解析但不 enforced CHECK。项目固定 8.4 并用真实错误验证，不能把 DDL复制到不受支持版本后仍宣称相同约束。

### 12.8 `IF NOT EXISTS` 的诱惑

它会让同名但结构不同的表被静默接受，migration 显示成功却 schema 漂移。初始 Migration 故意不用，冲突必须显式失败。

### 12.9 Grant 文本比较的升级成本

SHOW GRANTS 输出格式或角色语义变化可能造成升级失败。失败是安全信号，应在 MySQL 升级分支验证并更新 allowlist，不能用模糊 contains 降低门槛。

### 12.10 Mandatory roles

账号直接 grants 很干净，仍可能通过 `@@GLOBAL.mandatory_roles` 获权。这是“配置有效集”思维：审计不能只看对象本身，还要看环境继承。若未来启用 mandatory roles，需显式列出角色权限或改变应用身份模型。

### 12.11 授权不是事务

REVOKE 后 GRANT 失败会造成暂时无权限。当前 API 尚未启动，因此 fail closed。生产多实例滚动时需要兼容窗口：可能先扩权部署代码，再收权；或使用新角色/新账号切换，不能机械复制本地顺序。

### 12.12 MySQL 镜像匿名 volume

客户端 one-shot 使用 server 镜像会继承 VOLUME。若不显式覆盖，任务每次可能留下匿名 volume。空只读 bind 是局部解决方案；更小的纯 client 镜像也是未来选项，但要权衡供应链和版本一致性。

### 12.13 表名前缀与模块化单体

共享 schema 的 `lottery_` 提高所有权可见性，但不是数据库层真正隔离。权限现在按表隔离；若上下文增长，可考虑单独 schema/用户，但会增加跨上下文事务和迁移成本。

### 12.14 数据导入与旁路写

未来运营导入、DBA 脚本或数据修复若使用 Migrator/root，可以绕过 Repository 的完整领域校验。必须提供离线验证器、影子导入和审计，而不是因为 CHECK 存在就允许任意 CSV。

### 12.15 备份恢复与 Migration 历史

恢复旧备份后数据库 version 可能落后构件；Runner 可看到 pending，但数据/应用兼容仍需 runbook。恢复新备份给旧构件会 version mismatch，不能降级版本表。

### 12.16 时间与时区

会话 UTC + DATETIME 避免隐式时区转换，但应用读取仍须按 UTC解释。数据库服务器时钟漂移会影响元数据；时间戳不用于概率、排序身份或幂等。

### 12.17 数据保留与审计

当前没有软删除/历史表。未来合规要求出现时，不应把 update timestamp 当审计；需要操作人、before/after、原因、关联 request 和不可抵赖边界。

### 12.18 索引成本与外键隐式索引

InnoDB 会要求/创建支撑外键的索引；复合主键以 strategy_id 开头已经满足。审查 `SHOW CREATE TABLE/SHOW INDEX` 时需区分显式设计与引擎为约束创建的结构，避免重复索引。

### 12.19 生产 DDL 算法

初始空表 CREATE 风险低，但后续 ALTER 可能 COPY/INPLACE/INSTANT，有 metadata lock 和复制影响。不能把首建表经验推广到大表变更；新 Migration 必须单独评估 ALGORITHM/LOCK 与回滚窗口。

### 12.20 简历可信表达

可以说“设计并验证两表关系 schema、复合主键/外键/CHECK、前向 Migration 和最小权限启动门”；不能说“完成高并发抽奖平台、缓存命中优化、亿级表或零停机 DDL”，因为本节没有这些证据。

## 13. 假设与风险账本

| ID | 假设/风险 | 当前控制 | 失效信号 | 下一动作 |
| --- | --- | --- | --- | --- |
| R18-01 | Awards 数量小，可整聚合加载 | 复合 PK 最左前缀 | 单 Strategy 行数/延迟过大 | 重评聚合和分页/版本 |
| R18-02 | binary collation 符合保存需求 | 真实 Unicode 探针 | 需要搜索/去重/本地化 | 建搜索投影/normalization ADR |
| R18-03 | 外键成本可接受 | 当前小表、完整性优先 | 写吞吐/跨库受阻 | 基于数据重评，不先删 FK |
| R18-04 | SELECT only 足够第 18 节 | API 无 Repository | 第 19 节出现写 SQL | 精确扩表级 DML |
| R18-05 | SHOW GRANTS 格式稳定 | 固定 MySQL 8.4.11 | 升级输出变化 | 升级测试与 allowlist更新 |
| R18-06 | mandatory roles 为空 | 启动断言 | 环境启用强制角色 | 建模有效角色权限 |
| R18-07 | name_basic 只作子集不会被误解 | 约束命名/文档 | 代码绕过构造器 | 第19节坏行 fail-closed 测试 |
| R18-08 | row timestamp 不被当版本 | 明确文档 | ETag/cache/CAS 使用它 | 新增 aggregate version ADR |
| R18-09 | Migration 两版本可诊断 | dirty 注入 | 恢复人员直接 force | 强化 runbook/审批 |
| R18-10 | root job 短时风险可接受 | socket/no-network/read-only | 任意代码/Secret暴露 | 专用 client/短期凭据/平台 job |
| R18-11 | 空 Strategy 不会被在线使用 | 无 Repository/API | 旁路写产生空父行 | 数据检查 + Repository fail closed |
| R18-12 | 总权重由领域重验 | NewStrategy 已有检查 | mapper 绕过或 SQL sum 截断 | hydration 测试/禁止绕过 |
| R18-13 | 复合 ID 可贯穿后续 | 文档/PK | 事件只传 award_id | 契约审查或新增全局 ID |
| R18-14 | 初始 CREATE 锁风险低 | 空/小表环境 | 生产已有冲突/大表 | 影子库/online DDL |
| R18-15 | tmpfs 测试代表 MySQL 语义 | 固定 8.4.11 | 生产配置/版本差异 | staging 验收 |
| R18-16 | app direct grants 不被外部修改 | 每次启动 reconcile | 运行中人工 GRANT | 持续权限审计 |
| R18-17 | 没有展示顺序需求 | ID 规范顺序 | 运营拖拽排序 | 显式 position 与重排事务 |
| R18-18 | 不需要软删除/审计 | 无删除用例 | 合规/误删恢复需求 | 历史/审计模型 |

## 14. 未来演进问题

### 14.1 第 19 节 Repository 必须回答

- 一次查询还是两次查询？怎样保证读取一致性？
- `sqlx` row 类型如何扫描 unsigned bigint？
- 0 rows 是 not found，还是空 Strategy 数据损坏？
- 如何按 AwardID 排序并调用构造器？
- SQL 允许但 Go 拒绝的名称怎样分类？
- 如何区分 dependency error 与 invariant/corrupt data？
- 写完整聚合是否在范围内；若是，事务与权限如何设计？
- 怎样避免空父行和总权重并发问题？
- 需要什么真实索引和 timeout？

### 14.2 第 20 节算法必须回答

- 如何在 `[0,totalWeight)` 无偏取样，尤其 total=MaxUint64；
- 使用何种随机源，如何注入；
- modulo bias 与拒绝采样；
- 确定性边界测试和统计测试各证明什么；
- no_reward 如何作为正常 Award 返回。

### 14.3 第 21 节 API 必须回答

- uint64 如何 JSON 编码；
- scoped Award identity 如何表达；
- 找不到/坏配置/依赖失败/结果未知的公开错误；
- Draw 是否已有最终结果持久化与幂等；
- 认证/资格/Activity 尚未存在时接口边界；
- timeout/取消后是否可重试。

### 14.4 第 22 节前端必须回答

- 如何移除/隔离 Mock；
- 如何安全显示大整数和 Unicode；
- reward/no_reward/系统失败的不同 UI；
- request ID 与重试提示；
- 无障碍与降级。

### 14.5 第 23 节规则必须回答

- 规则属于 Strategy、Activity 还是 Participation；
- 发布时合法性与草稿；
- 配置版本/快照；
- 规则变化对历史 Draw 的影响。

### 14.6 第 24 节 Redis 必须回答

- 缓存的是完整 Strategy 还是投影；
- key 是否包含版本；
- TTL/失效/回源/坏缓存 fail closed；
- row timestamp 为何不能直接做聚合缓存版本；
- Redis 失败是否降级到 MySQL；
- 防穿透/击穿需要什么真实流量证据。

### 14.7 更远问题

- 聚合版本、发布与历史重放；
- 数据导入审批、审计和回滚；
- 大表 online DDL、复制和备份；
- 跨上下文外键或逻辑引用；
- 数据保留/删除与合规；
- 生产身份、动态凭据和持续权限审计。

## 15. 可追溯证据

### 15.1 实现

- [Migration 000001](../../../migrations/sql/000001_create_lottery_strategy.up.sql)
- [Migration 000002](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql)
- [嵌入与不可变测试](../../../migrations/embed_test.go)
- [真实 MySQL schema 测试](../../../migrations/lottery_schema_integration_test.go)
- [首次身份初始化](../../../deploy/compose/mysql/init/10-create-growthos-users.sh)
- [授权收敛脚本](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)
- [Compose 拓扑](../../../deploy/compose/compose.yaml)
- [Compose smoke](../../../scripts/compose-smoke.sh)

### 15.2 决策与课程

- [第 17 节领域对象](../../course/part-03/lesson-17-lottery-domain-objects.md)
- [第 18 节课程](../../course/part-03/lesson-18-lottery-schema.md)
- [ADR-0010：Migration 边界](../../decisions/ADR-0010-mysql-migration-boundaries.md)
- [ADR-0012：Compose 拓扑](../../decisions/ADR-0012-compose-development-topology.md)
- [ADR-0013：Lottery 领域模型](../../decisions/ADR-0013-lottery-domain-model.md)
- [ADR-0014：持久化结构](../../decisions/ADR-0014-lottery-persistence-schema.md)
- [ADR-0015：授权收敛](../../decisions/ADR-0015-compose-schema-grant-reconciliation.md)

### 15.3 验收与接口边界

- [第 18 节 QA](../../qa/lessons/lesson-18.md)
- [第 18 节 API 记录](../../api/lessons/lesson-18.md)
- [第 18 节面试问答](../../interview/lessons/lesson-18.md)

### 15.4 官方依据

- [MySQL 8.4 CREATE TABLE](https://dev.mysql.com/doc/refman/8.4/en/create-table.html)
- [CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)
- [FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)
- [Integer Types](https://dev.mysql.com/doc/refman/8.4/en/integer-types.html)
- [utf8mb4](https://dev.mysql.com/doc/refman/8.4/en/charset-unicode-utf8mb4.html)
- [Multiple-Column Indexes](https://dev.mysql.com/doc/refman/8.4/en/multiple-column-indexes.html)
- [Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)
- [InnoDB Online DDL](https://dev.mysql.com/doc/refman/8.4/en/innodb-online-ddl.html)
- [GRANT](https://dev.mysql.com/doc/refman/8.4/en/grant.html)
- [REVOKE](https://dev.mysql.com/doc/refman/8.4/en/revoke.html)

### 15.5 证据总边界

这套证据能证明第 18 节 schema 与本地授权设计在固定 MySQL 8.4.11、当前代码和测试范围内成立。它不能证明 Repository 正确、抽奖公平、在线链路可用、生产规模性能、零停机迁移、权限永久无漂移或最终结果幂等。下一节必须继续从这些明确空白出发，而不是把表存在当作系统完成。
