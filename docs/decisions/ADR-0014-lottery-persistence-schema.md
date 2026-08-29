# ADR-0014：Lottery Strategy/Award 的首个持久化结构

- **状态：** 已接受
- **日期：** 2026-08-29
- **负责人：** GrowthOS 维护者

## 背景

第 17 节已经把 Lottery 最小配置建模为 Strategy 聚合与 Strategy 内 Award 实体：ID 和 Weight 是正 `uint64`，Award ID 在 Strategy 内唯一，Strategy 至少一个 Award，总权重不能溢出，Outcome 只有 `reward` / `no_reward`，名称具有 UTF-8、Unicode trim、控制字符与 128 rune 契约。

第 18 节需要创建第一组业务表。目标不是把 Go 字段机械映射为 SQL，而是决定：

- 哪些领域事实应成为数据库原生约束；
- 哪些聚合不变量无法由当前关系结构独立保证；
- ID、Weight、名称和 Outcome 的物理类型；
- Award 的身份范围、主键和父子引用；
- 删除、更新、排序、时间戳和索引的当前语义；
- MySQL DDL 原子边界如何映射到 Migration 版本；
- 如何避免为了未来可能需求预建状态、版本、展示顺序或冗余总权重。

本 ADR 只决定 schema。应用权限的升级/收敛生命周期由 [ADR-0015](ADR-0015-compose-schema-grant-reconciliation.md) 单独记录。

## 约束与已知事实

1. 当前数据库基线为 MySQL 8.4、InnoDB、UTC 会话和 `utf8mb4`。
2. Migration 采用不可变、只向前追加的嵌入式 `.up.sql` 历史。
3. MySQL atomic DDL 覆盖单条受支持 DDL，不把一个多语句 Migration 文件变成事务。
4. `uint64` 的完整上限超过有符号 `BIGINT`。
5. Award ID 的领域唯一范围是单个 Strategy，不是全局。
6. 名称不是身份，也没有搜索、模糊匹配或本地化查询用例。
7. 当前没有 Repository、创建/编辑/删除用例、并发版本、抽奖算法、API 或缓存。
8. 数据库单行 CHECK 无法直接保证“父行至少有一个子行”或“多子行权重求和不溢出”。

## 评估过的方案

### 表边界

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| Strategy 一张表，Awards 存 JSON | 单行读取、DDL 少 | 主键/外键/CHECK 难覆盖 Award；部分更新、查询和审计不透明 | 不采用 |
| Strategy 与 Award 两张表 | 关系和约束可见；符合聚合根—实体结构 | 完整加载需要多行映射；跨行不变量仍需应用负责 | 采用 |
| Activity、Strategy、Award 一次建齐 | 看似完整 | 提前跨上下文，Activity 尚未有对象和用例 | 不采用 |

### ID 与主键

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| 数据库 `AUTO_INCREMENT` | 创建简单 | 提前决定 ID 生成与返回协议；不表达已有领域输入 | 不采用 |
| Award 全局单列主键 | 引用简单 | 比领域要求更强，改变“Strategy 内实体”语义 | 不采用 |
| `(strategy_id, award_id)` 复合主键 | 直接表达 scoped identity；支持按 Strategy 读取 | 所有外部引用需携带 Strategy ID | 采用 |
| UUID/ULID 字符串 | 分布式生成方便 | 当前领域已选 uint64；存储/索引成本与迁移无证据收益 | 不采用 |

### 数值类型

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| `BIGINT` | 常见，有符号生态兼容 | 无法保存 `2^63..2^64-1` | 不采用 |
| `DECIMAL(20,0)` | 覆盖 uint64 | Go driver 扫描、比较和索引语义更复杂 | 不采用 |
| `BIGINT UNSIGNED` | 与 Go `uint64` 范围直接对应 | 跨数据库和 JS/API 需要显式映射 | 采用 |

### 父子完整性与删除

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| 无外键，仅 Repository 保证 | 写入灵活 | 脚本/人工/未来 adapter 可留下孤儿 | 不采用 |
| 外键 + `CASCADE` | 删除父行方便 | 当前没有删除用例；隐式多行破坏与审计边界不明 | 不采用 |
| 外键 + `SET NULL` | 保留子行 | Award 没有脱离 Strategy 的合法语义 | 不采用 |
| 外键 + `RESTRICT` | 孤儿被拒绝；破坏动作必须显式 | 删除时需要明确先处理子行 | 采用 |

### Outcome 表示

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| 布尔 `has_reward` | 列短 | 词义弱；未来类别演进和可读性差 | 不采用 |
| MySQL ENUM | 值域紧凑 | 类型修改耦合、内部序号/排序语义容易误用 | 不采用 |
| 字典表 + FK | 可附加元数据 | 两个稳定值引入额外表和 join，没有独立生命周期 | 不采用 |
| `VARCHAR(16) ascii_bin + CHECK` | 值可读、大小写敏感、约束明确、前向演进可审查 | 需要修改 CHECK 才能扩值 | 采用 |

### 名称字符集和比较

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| `utf8mb3` 或 latin1 | 空间可能更小 | 不能覆盖完整 Unicode | 不采用 |
| 语言学不区分大小写/重音 collation | 搜索友好 | 当前没有搜索语义；会静默折叠不同原始值 | 不采用 |
| `utf8mb4_0900_bin` | 支持四字节 Unicode；比较可预测 | 不提供语言学搜索和 Unicode normalization | 采用 |

### 名称 CHECK

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| 不加 CHECK，只靠 Repository | 迁移最简单 | 任何写入者都能保存明显空值 | 不采用 |
| 用 SQL 模拟全部 Go Unicode 契约 | 试图单点保证 | MySQL TRIM/字符函数与 Go Unicode 规则并不等价，容易制造虚假保证 | 不采用 |
| 只声明可证明的 `name_basic` 子集 | 拒绝空值/首尾 U+0020，同时诚实保留边界 | 完整合法性需要 Repository 重建 | 采用 |

### Migration 粒度

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| 一个文件包含两条 CREATE | 版本数少 | 第二条失败时第一条可能已提交；版本与 atomic DDL 边界不一致 | 不采用 |
| 两个版本、每个一个 CREATE | 失败定位清楚；v1/v2 各自对应单 DDL | 中间 clean v1 只有父表 | 采用；API 只有 latest clean 后才启动 |

### 时间戳、版本与冗余

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| 无时间列 | 最小结构 | 基本行级诊断信息缺失 | 不采用 |
| 每行 `created_at/updated_at` | 低成本行元数据 | 容易被误当聚合版本 | 采用，但限定语义 |
| 额外 `version` 乐观锁列 | 支持 CAS | 当前没有写协议，无法定义谁递增 | 暂不采用 |
| Strategy 存 `total_weight` | 读取算法更快 | 与 Award 行重复事实，更新一致性尚未设计 | 不采用 |
| Award 存 `position` | 显式展示顺序 | 当前领域以 ID 规范顺序，UI 排序没有需求 | 不采用 |

## 决策

1. 创建 `lottery_strategy` 与 `lottery_strategy_award` 两张 InnoDB 表，默认字符集 `utf8mb4`、排序规则 `utf8mb4_0900_bin`。
2. Strategy 使用 `strategy_id BIGINT UNSIGNED` 单列主键；Award 使用 `(strategy_id, award_id)` 复合主键。所有 ID 都 `NOT NULL` 且通过 CHECK 拒绝零值。
3. 不使用 `AUTO_INCREMENT`；ID 生成协议留给有真实创建用例的章节。
4. `lottery_strategy_award.strategy_id` 通过显式命名外键引用父表，`ON DELETE RESTRICT ON UPDATE RESTRICT`。
5. Weight 使用 `BIGINT UNSIGNED NOT NULL CHECK(weight > 0)`，不保存冗余 `total_weight`。
6. Outcome 使用 `VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin`，CHECK 只允许 `reward`、`no_reward`。
7. Strategy/Award 名称使用 `VARCHAR(128) ... utf8mb4_0900_bin`。`name_basic` CHECK 只保证 `CHAR_LENGTH > 0` 和默认 `TRIM` 后不变；该约束明确只覆盖首尾 ASCII U+0020 子集，不宣称等价于 Go `strings.TrimSpace`、控制字符或完整领域合法性。
8. 每表保留 `created_at` / `updated_at DATETIME(6)` 行级元数据。它们不是聚合版本、乐观锁、缓存版本或审计日志；Award 行更新不会触碰父表时间戳。
9. 不添加业务尚未要求的 status、position、version、soft-delete、benefit payload、库存、Activity/DrawResult 外键或二级索引。
10. 两条 CREATE 分别放在 `000001` 与 `000002`，每个版本一条 DDL。共享后不可改写，修改只能追加新版本。
11. 对初始 Migration 内容固定 SHA-256 并检查单语句终止符，降低误改历史的风险。
12. 第 19 节 Repository 必须通过领域构造器重建：对 SQL 可接受但领域拒绝的名称、空 Award 集合或跨行总权重溢出 fail closed。
13. 第 19 节出现真实 SQL 前不增加猜测性索引；届时按查询谓词、排序、基数、执行计划和写放大重新评估。

## 数据库不能独立保证的事实

本 ADR 明确不把以下事项伪装成已解决：

- 一条 Strategy 记录至少对应一个 Award；
- 一个 Strategy 的所有 Weight 求和不超过 `math.MaxUint64`；
- 名称符合 Go 对所有 Unicode 空白和控制字符的规则；
- Strategy/Award 行属于一个原子更新过的聚合版本；
- 配置已发布、可抽、与 Activity 绑定或拥有库存；
- 一次 Draw 只有一个最终结果。

这些要由未来领域/Repository/用例边界和对应存储结构分别负责。

## 影响

### 正面影响

- 表结构直接表达当前最小领域语义，而不是围绕页面或 ORM 生成；
- 完整 `uint64` 范围可以无损持久化；
- 复合键和外键使 Award scoped identity 与父子完整性在数据库可见；
- 值域、零值和最基本名称错误在任何写入口都被拒绝；
- 一条 DDL 一个版本使失败现场与 MySQL 原子边界一致；
- 没有猜测性索引和冗余字段，后续改变的理由可追溯。

### 成本与风险

- Repository 必须正确处理复合键和多行聚合加载；
- MySQL unsigned 类型增加跨数据库、JSON 与 JavaScript 映射责任；
- `utf8mb4_0900_bin` 不是语言学搜索排序，未来搜索可能需要专用投影/索引；
- `name_basic` 只是完整领域规则的子集，绕过 Repository 的写入可能留下领域不可加载数据；
- 外键 RESTRICT 让未来删除流程需要显式事务和审计；
- 行时间戳很容易被误用为聚合版本，文档和后续代码必须保持边界；
- clean v1 是合法迁移中间态但不是 API 可运行最终态，编排必须等待 latest clean v2。

## 演进与重新决策触发器

- 第 19 节真实 Repository 查询表明复合主键不足或执行计划需要二级索引；
- 出现创建/编辑并发，需要聚合 version 或 CAS；
- 出现运营展示排序，需要定义 `position` 的唯一性和重排事务；
- Outcome 类别增长到具有独立元数据/生命周期，需要字典或类型表；
- 需要名称搜索、本地化或反欺骗规则，需要单独搜索投影和 normalization 决策；
- Award 数量巨大，整聚合加载不再经济，需要重新评估聚合和存储边界；
- 出现删除、软删除、归档或历史重放需求；
- 迁移在生产规模出现锁/耗时风险，需要在线 DDL/变更平台 ADR。

## 验收证据

- 两份实际 Migration：[000001](../../migrations/sql/000001_create_lottery_strategy.up.sql)、[000002](../../migrations/sql/000002_create_lottery_strategy_award.up.sql)；
- 不可变检查：[migrations/embed_test.go](../../migrations/embed_test.go)；
- 真实 MySQL 结构/边界验证：[migrations/lottery_schema_integration_test.go](../../migrations/lottery_schema_integration_test.go)；
- 章节说明与执行记录：[第 18 节课程](../course/part-03/lesson-18-lottery-schema.md)、[第 18 节 QA](../qa/lessons/lesson-18.md)。

## 参考

- [MySQL 8.4 CREATE TABLE](https://dev.mysql.com/doc/refman/8.4/en/create-table.html)
- [MySQL 8.4 CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)
- [MySQL 8.4 FOREIGN KEY Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)
- [MySQL 8.4 Integer Types](https://dev.mysql.com/doc/refman/8.4/en/integer-types.html)
- [MySQL 8.4 utf8mb4](https://dev.mysql.com/doc/refman/8.4/en/charset-unicode-utf8mb4.html)
- [MySQL 8.4 ENUM](https://dev.mysql.com/doc/refman/8.4/en/enum.html)
- [MySQL 8.4 Multiple-Column Indexes](https://dev.mysql.com/doc/refman/8.4/en/multiple-column-indexes.html)
- [MySQL 8.4 Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)
- [MySQL 8.4 DATETIME/TIMESTAMP 自动初始化](https://dev.mysql.com/doc/refman/8.4/en/timestamp-initialization.html)
