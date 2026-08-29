# 第 18 节面试问答：首个 Lottery Schema、Migration 与最小权限

本文只描述第 18 节已经进入代码的能力：两张 MySQL 8.4 Lottery 配置表、两条单 DDL 前向 Migration、复合主键/外键/CHECK、无符号大整数与 Unicode 边界验证，以及 Compose 中 migration→精确授权→API 的启动门。Repository、加权随机算法、Lottery 业务 API、真实 React 抽奖页和 Redis 业务缓存仍不存在。

## 60 秒项目自述

我先用纯 Go 领域模型确定 Strategy/Award 的身份、正整数权重和 reward/no_reward 语义，再在第 18 节把它映射为两张 MySQL 8.4 表。Strategy 用 `BIGINT UNSIGNED` 单主键，Award 用 `(strategy_id, award_id)` 复合主键，符合奖项只在策略内唯一；外键采用 RESTRICT，避免无业务用例时隐式级联删除。名称使用 `utf8mb4_0900_bin VARCHAR(128)`，Outcome 用 `ascii_bin VARCHAR + CHECK`，Weight 也是 unsigned bigint。数据库 CHECK 只守能准确表达的单行子集；“至少一个 Award”“总权重不溢出”和完整 Unicode TrimSpace/控制字符规则仍由下一节 Repository 通过领域构造器 fail closed。两张表拆成两个 Migration，因为 MySQL atomic DDL 是单语句边界。Compose 还新增 socket-only、无网络的一次性授权任务，在迁移后撤销旧权限、只授予应用两表 SELECT，并用完整 SHOW GRANTS 和空 mandatory roles 断言有效权限；授权或迁移失败时 API 不启动。真实 MySQL 测试验证三个 MaxUint64、128/129 中文字符、复合键、外键、CHECK 和 1142 权限拒绝。这个切片完成的是可信持久化结构，不是在线抽奖功能。

## 来源说明

- `项目事实` 只来自当前 SQL、Go 测试、Compose、脚本、课程、QA 和 ADR。命令是否最终通过以[第 18 节 QA](../../qa/lessons/lesson-18.md)为准，不能从本文的答案倒推测试已经运行。
- `官方事实` 主要来自 MySQL 8.4 Reference Manual；它解释数据库行为，不替项目做选型。
- `面经启发` 来自牛客用户自述。帖子能证明“表设计、索引、逻辑外键、B+ 树、事务、online DDL、项目取舍”等方向真实出现在候选人复盘中；公司、轮次和题面按作者自述看待，本文未独立核验。
- 牛客部分页面可能需要登录、依赖动态渲染或随站点迁移而只显示标题。本文只对实际可见内容做题型归纳，所有技术结论回到官方文档和项目证据。
- 找不到完全同构面经的问题会标记为 `项目场景题`，不虚构成某公司的原题。

可见题型参考包括：[初创公司 Java 面试二：手写建表/索引基础](https://www.nowcoder.com/discuss/869717328408150016)、[字节后台面经：按查询需求设计 SQL 与索引](https://www.nowcoder.com/discuss/353155518426456064)、[4399 Web 后端：项目索引策略与逻辑外键取舍](https://www.nowcoder.com/discuss/732675514812370944)、[数据库内核社招：online DDL、MVCC、执行计划和索引合理性](https://www.nowcoder.com/discuss/721384389753446400)、[腾讯/美团/携程等后台复盘：项目表设计和 SQL](https://www.nowcoder.com/discuss/353155632142426112)、[数据库基础题汇总：事务、InnoDB、B+ 树、联合索引最左前缀](https://www.nowcoder.com/discuss/353147574485983232)。

## 1. 为什么 Strategy 和 Award 设计成两张表，而不是一张宽表或 JSON？

- **直接回答：** Strategy 是聚合根，Award 是一对多子实体。两张表能用复合主键保证同策略 AwardID 唯一，用外键拒绝孤儿，用 CHECK 单独约束 Weight/Outcome，还能按 `strategy_id` 稳定读取奖项。一张宽表会重复 Strategy 名称和时间列，更新时产生异常；JSON 虽能单行保存，但子项键、外键、值域、局部查询和后续迁移都更弱。当前 Award 是有身份的关系实体，不是无法查询的任意配置 blob。
- **追问：** JSON 单行不是更容易原子更新整个聚合吗？
  - **追问回答：** 是，这是 JSON 的真实优势。如果数据只整体读写、内部结构不需要关系约束、查询索引或独立演进，JSON 可更经济。但 GrowthOS 已有 scoped AwardID、父子完整性、Outcome/Weight 边界，且未来 Repository/运营会按 Award 处理；两表的结构化收益更大。事务性整聚合写入仍由 Repository 负责，不能靠 JSON 优势掩盖其他损失。
- **项目证据：** [两条 Migration](../../../migrations/sql/README.md)、[ADR-0014 方案矩阵](../../decisions/ADR-0014-lottery-persistence-schema.md)、[第 18 节课程](../../course/part-03/lesson-18-lottery-schema.md)。
- **选型边界：** 不把“两表”推广成所有聚合的固定答案；结构高度动态、无需内部约束且只整体访问时 JSON 合理。反过来，有稳定身份、关系完整性和查询需求时，不应只因少一次 join 就选 JSON。
- **来源：** `面经启发` [后台项目表结构会被要求具体说明并按查询写 SQL](https://www.nowcoder.com/discuss/353155632142426112)；`官方事实` [MySQL CREATE TABLE](https://dev.mysql.com/doc/refman/8.4/en/create-table.html)；`项目事实` [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 2. 为什么 Award 使用 `(strategy_id, award_id)` 复合主键，而不是全局 `award_id`？

- **直接回答：** 第 17 节领域事实是 AwardID 在一个 Strategy 内唯一，而不是全局唯一。复合主键直接表达 scoped identity：同一 Strategy 重复 AwardID 被拒绝，不同 Strategy 可以都拥有 AwardID 1；同时其最左前缀支持按 `strategy_id` 读取整组 Awards。全局主键会无证据地加强业务规则，surrogate row ID 又会增加一个没有业务含义的身份。
- **追问：** 以后 Benefit 或事件只想引用 Award 怎么办？
  - **追问回答：** 现在完整引用必须携带 StrategyID + AwardID。若未来大量跨上下文引用确实需要稳定全局身份，应新增经 ADR 论证的 global award identity，并保留组合唯一约束；不能因为调用方便就假设当前 AwardID 恰好全局不重复。
- **项目证据：** [Award 主键 DDL](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql)、[集成测试的重复键/不同 Strategy 边界](../../../migrations/lottery_schema_integration_test.go)、[第 17 节领域模型](../../decisions/ADR-0013-lottery-domain-model.md)。
- **选型边界：** 如果 Award 获得独立生命周期并成为跨 Strategy 聚合根，全局 ID 可能更合适；当前它是 Strategy 内实体，组合身份最忠实。
- **来源：** `面经启发` [字节后台复盘中按具体查询设计订单表和索引](https://www.nowcoder.com/discuss/353155518426456064)；`官方事实` [MySQL Multiple-Column Indexes](https://dev.mysql.com/doc/refman/8.4/en/multiple-column-indexes.html)；`项目事实` [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 3. 为什么 ID 和 Weight 都用 `BIGINT UNSIGNED`？

- **直接回答：** Go 领域类型是正 `uint64`，上限 `2^64-1`；MySQL 有符号 BIGINT 的正上限只有 `2^63-1`。`BIGINT UNSIGNED` 可以无损保存完整范围。本节真实写入并读回 StrategyID、AwardID、Weight 三个 `math.MaxUint64`，防止 DDL 看似正确但 driver/scan 实际只支持 int64。
- **追问：** 完整 uint64 会给 API 带来什么问题？
  - **追问回答：** JavaScript `number` 只保证到 `2^53-1` 的整数精度。第 21 节必须把 ID/可能的 Weight 编成十进制字符串、收窄公开范围或采用其他明确契约；不能直接把数据库数字原样 JSON number 输出。跨 PostgreSQL 等数据库也要重新映射 unsigned 语义。
- **项目证据：** [两表 DDL](../../../migrations/sql/README.md)、[三个 MaxUint64 集成探针](../../../migrations/lottery_schema_integration_test.go)、[API 边界](../../api/lessons/lesson-18.md)。
- **选型边界：** 如果业务正式限定 ID 在 signed bigint 或 JS safe integer 范围，使用有符号类型能降低生态摩擦；当前领域已承诺完整 uint64，所以存储不能先截断。
- **来源：** `官方事实` [MySQL 8.4 Integer Types](https://dev.mysql.com/doc/refman/8.4/en/integer-types.html)；`项目事实` [ADR-0014 数值选择](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 4. 为什么不用 `AUTO_INCREMENT`？ID 到底由谁生成？

- **直接回答：** 当前领域构造器接收显式 ID，但还没有“创建 Strategy”用例，因此尚不能决定是数据库自增、集中 ID 服务、雪花、UUID/ULID 还是外部导入 ID。现在加 AUTO_INCREMENT 会提前把生成、插入返回值、分库和离线导入协议锁给数据库。DDL 只负责保存与校验稳定 ID，不伪造尚不存在的创建流程。
- **追问：** 这会不会只是把问题推迟？
  - **追问回答：** 是有意推迟，而非忽略。没有用例时任何生成方案都缺少约束；第 19 节若只读仍无需决定，首次创建/管理 API 出现时再基于吞吐、可排序性、跨环境合并和信息泄露要求立决策。
- **项目证据：** [DDL 无 AUTO_INCREMENT](../../../migrations/sql/000001_create_lottery_strategy.up.sql)、[课程非目标](../../course/part-03/lesson-18-lottery-schema.md)。
- **选型边界：** 单库、低写入且无需离线合并时 AUTO_INCREMENT 很实用；分布式写、多源导入或不可猜 ID 需求出现时应评估其他方案。
- **来源：** `面经启发` [初创公司面试要求现场写 MySQL 建表语句](https://www.nowcoder.com/discuss/869717328408150016)；`官方事实` [CREATE TABLE 的 AUTO_INCREMENT 语法](https://dev.mysql.com/doc/refman/8.4/en/create-table.html)；`项目事实` [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 5. 为什么使用物理外键？“互联网项目都用逻辑外键”对吗？

- **直接回答：** 不对，外键应按部署边界、写吞吐、完整性和团队运维能力权衡。当前是单 MySQL schema 的模块化单体，Award 离开 Strategy 没有合法语义，数据规模与写吞吐尚小；物理 FK 能让脚本、人工和未来 adapter 都无法产生孤儿。只靠逻辑外键会把完整性完全交给每个写入口，目前没有收益证据。
- **追问：** 分库分表以后物理外键怎么办？
  - **追问回答：** 跨库通常无法使用同样的 FK，需要用事务内应用校验、事件/outbox、对账巡检和补偿替代。但“未来可能分库”不是今天放弃本地强约束的理由；真正拆分时再迁移责任并留验证机制。
- **项目证据：** [命名 FK](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql)、[孤儿 1452 测试](../../../migrations/lottery_schema_integration_test.go)。
- **选型边界：** 极高写吞吐、跨库、批量导入或数据库引擎不支持时逻辑外键可能合理，但必须有等价完整性和巡检；不能只删除 FK。
- **来源：** `面经启发` [4399 后端复盘明确出现“逻辑外键的优劣”追问](https://www.nowcoder.com/discuss/732675514812370944)；`官方事实` [MySQL Foreign Key Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)。

## 6. 为什么外键选择 RESTRICT，而不是 CASCADE？

- **直接回答：** 当前没有删除 Strategy 的业务用例、权限、审计和恢复协议。CASCADE 会让一次父表 DELETE 自动变成多行不可见破坏动作；RESTRICT 让有子 Award 的父删除以 1451 失败，迫使未来用例显式决定先归档、删除子行、软删除或拒绝操作。它最符合“没有需求时不赋予破坏语义”。
- **追问：** CASCADE 什么时候更合适？
  - **追问回答：** 子记录完全无独立保留/审计价值，父删除已经是明确且授权的业务原子动作，并且事务规模可控时，CASCADE 可减少遗漏。仍需评估大级联的锁、复制和审计影响。
- **项目证据：** [ON DELETE/UPDATE RESTRICT](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql)、[删除父行 1451 探针](../../../migrations/lottery_schema_integration_test.go)。
- **选型边界：** RESTRICT 不是永不删除；只是要求删除协议先成为显式用例。不要为让测试“方便清理”改业务外键动作。
- **来源：** `官方事实` [MySQL 外键 reference options](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)；`项目事实` [ADR-0014 外键矩阵](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 7. CHECK 约束与应用校验为什么要同时存在？

- **直接回答：** CHECK 是任何写入口都必须通过的数据库底线，能拒绝零 ID、零 Weight 和非法 Outcome；领域构造器表达更丰富的业务规则并提供应用错误语义。只靠应用会让脚本/迁移绕过；只靠 CHECK 又无法表示至少一个 Award、跨行总和、完整 Unicode 空白与控制字符。两者是不同信任边界的纵深防御，不是重复劳动。
- **追问：** MySQL 不是曾经忽略 CHECK 吗？
  - **追问回答：** 旧版本确有兼容历史，所以项目固定 MySQL 8.4 基线，并用真实写入得到 3819 验证 enforced 行为。不能只看到 DDL 文本就认为所有部署版本一致。
- **项目证据：** [命名 CHECK](../../../migrations/sql/README.md)、[3819 负向集成测试](../../../migrations/lottery_schema_integration_test.go)、[领域构造器测试](../../../internal/lottery/domain)。
- **选型边界：** CHECK 适合确定、行内、低歧义规则；跨行、外部依赖或需要复杂错误上下文的规则应由领域/事务处理，并考虑数据库其他机制。
- **来源：** `官方事实` [MySQL 8.4 CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)；`项目事实` [设计手记不变量边界](../../design-thinking/lessons/lesson-18.md)。

## 8. 为什么约束叫 `name_basic`？MySQL `TRIM` 和 Go `TrimSpace` 有什么差异？

- **直接回答：** DDL 的 `name = TRIM(name)` 只可靠表达默认 TRIM 对首尾 ASCII U+0020 空格的处理，加上 `CHAR_LENGTH > 0`；它不等价于 Go `strings.TrimSpace` 的 Unicode 空白集合，也不拒绝内部控制字符。约束叫 `basic` 是为了避免虚假承诺。集成测试分别验证首 U+0020、尾 U+0020 和纯空格被 3819 拒绝；第 19 节仍须用领域构造器加载，非法旧行 fail closed。
- **追问：** 为什么不在 SQL 写更复杂正则完全复制？
  - **追问回答：** 两套 Unicode 数据/函数版本很难长期等价，复杂 CHECK 还增加迁移和索引/性能认知成本。数据库守清楚的底线，领域守完整规则更可测试。若未来需要数据库级全量规范，应先定义精确 Unicode 版本和一致性测试。
- **项目证据：** [两个 name_basic CHECK](../../../migrations/sql/README.md)、[首尾空格探针](../../../migrations/lottery_schema_integration_test.go)、[第 19 节交接](../../course/part-03/lesson-18-lottery-schema.md)。
- **选型边界：** 不要说 SQL 没价值；它仍阻止最明显空值和普通首尾空格。也不要在 Repository 静默 trim 坏行，那会篡改权威事实。
- **来源：** `官方事实` [MySQL String Functions/TRIM](https://dev.mysql.com/doc/refman/8.4/en/string-functions.html)、[CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)；`项目事实` [QA 名称边界](../../qa/lessons/lesson-18.md)。

## 9. 为什么用 `utf8mb4_0900_bin`，不选常见的 `_ai_ci`？

- **直接回答：** 当前目标是无损保存运营名称，并没有大小写不敏感、重音不敏感搜索需求。`utf8mb4` 支持四字节 Unicode，`_bin` 使比较行为可预测，不会在没有业务决定时把大小写、重音或分解/预组形式折叠。测试验证 128 个中文字符和 NFD/NFC 两个原始值可区分。
- **追问：** 二进制 collation 会不会让搜索体验很差？
  - **追问回答：** 会，所以它不是搜索方案。未来名称检索可增加规范化搜索列、全文索引或搜索服务，并明确定义语言/大小写/重音规则；权威原始名称仍可保真。保存和搜索不必用同一投影。
- **项目证据：** [表/列 collation DDL](../../../migrations/sql/README.md)、[Unicode 探针](../../../migrations/lottery_schema_integration_test.go)。
- **选型边界：** 用户登录名、自然语言搜索等已有明确不区分规则的字段，可能应选语言学 collation；本节名称不是身份也没有检索用例。
- **来源：** `官方事实` [MySQL utf8mb4](https://dev.mysql.com/doc/refman/8.4/en/charset-unicode-utf8mb4.html)、[Collation Naming](https://dev.mysql.com/doc/refman/8.4/en/charset-collation-names.html)；`项目事实` [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 10. Outcome 为什么是 `VARCHAR + CHECK`，不直接用 ENUM 或字典表？

- **直接回答：** 当前只有两个可读、稳定值，但仍可能演进。`VARCHAR(16) ascii_bin + CHECK` 同时提供可读存储、大小写敏感和值域约束；新增值通过前向 Migration 显式修改 CHECK。ENUM 紧凑但把值集/内部序号/排序绑定到列类型；字典表适合值有独立元数据和生命周期，但两个常量时过重。
- **追问：** VARCHAR 更占空间，是否性能差？
  - **追问回答：** 需要结合行数、索引和查询证明。当前 Outcome 不建索引、数据量未知，用几字节差异换清晰演进更合理。若亿级行、存储/缓存成为瓶颈，再基准测试 ENUM/小整数编码，不能先声称性能问题。
- **项目证据：** [Outcome DDL](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql)、[大小写负向测试](../../../migrations/lottery_schema_integration_test.go)。
- **选型边界：** 取值永不变化且团队接受 ENUM 迁移语义时 ENUM 合理；取值有展示文案/状态/权限时字典或独立领域对象可能更适合。
- **来源：** `官方事实` [MySQL ENUM](https://dev.mysql.com/doc/refman/8.4/en/enum.html)、[CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)；`项目事实` [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 11. 为什么不存 `total_weight`？Weight 为什么不是浮点概率？

- **直接回答：** 第 17 节已选择正整数相对权重，避免二进制浮点误差和固定 100/10000 分母。`total_weight` 可由 Awards 派生；现在冗余存储会出现子行已改但父 total 未改，需要事务、触发器或校验协议。Repository/算法还不存在，没有性能证据值得引入双事实源。
- **追问：** 每次 SUM 会不会慢？
  - **追问回答：** 当前 Awards 预计少，Repository 还要加载各行构造领域对象，逐项求和成本很小。若真实数据表明计算热点，可缓存不可变 Strategy 版本或存派生 total，但必须定义更新一致性和校验，不能单纯加列。
- **项目证据：** [DDL 无 total_weight](../../../migrations/sql/README.md)、[领域总权重溢出检查](../../../internal/lottery/domain/strategy.go)。
- **选型边界：** 固定精度金融金额不是相对 Weight，应使用 DECIMAL/最小货币单位；不能把此选择泛化到所有数值。
- **来源：** `项目事实` [第 17 节权重设计](../../course/part-03/lesson-17-lottery-domain-objects.md)、[ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 12. 数据库为什么不能保证每个 Strategy 至少一个 Award？怎么补？

- **直接回答：** FK 只能保证子行的父存在，不反向保证父行至少一个子。普通单行 CHECK 也不能查询另一张表。因此物理上可先插入一个空 Strategy，领域上却非法。第 19 节读取必须调用 `NewStrategy`，空集合 fail closed；如果实现写入，应在事务中提交完整父子聚合，并决定并发/发布可见性。
- **追问：** 用触发器不是可以保证吗？
  - **追问回答：** 很难在“先父后子”的正常事务中即时强制父始终有子，延迟约束语义也复杂；触发器会把业务规则藏在数据库并增加迁移/测试成本。未来若草稿需要空策略，应显式建状态，而不是触发器猜业务阶段。
- **项目证据：** [课程约束矩阵](../../course/part-03/lesson-18-lottery-schema.md)、[设计手记信任边界](../../design-thinking/lessons/lesson-18.md)。
- **选型边界：** 数据库可通过更复杂模型/过程实现部分跨行规则，但当前收益不足；责任必须明确落到 Repository/领域而非被遗漏。
- **来源：** `官方事实` [CHECK 表达式限制](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)、[Foreign Keys](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)；`项目事实` [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 13. 每行 Weight 都合法，为什么总和仍可能溢出？

- **直接回答：** `BIGINT UNSIGNED` 和 `weight > 0` 只约束单行。两行分别为 MaxUint64 和 1 都合法，但 Go `uint64` 求和会回绕。当前数据库没有跨行总和 CHECK，也不保存 total；第 19 节必须逐行构造 Strategy，让已有溢出检查 fail closed。写入也应先验证完整聚合。
- **追问：** 为什么不用 SQL `SUM(weight)` 判断？
  - **追问回答：** 可以作为辅助，但要核对 MySQL SUM 返回类型、driver scan 和并发快照；最终仍应交给同一个领域构造规则，避免 SQL/Go 两套边界漂移。若写事务并发修改，还需锁或版本避免各自验证后组合溢出。
- **项目证据：** [Weight DDL](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql)、[领域溢出测试](../../../internal/lottery/domain/strategy_test.go)、[第 19 节要求](../../design-thinking/lessons/lesson-18.md)。
- **选型边界：** 本节只证明单行 MaxUint64 可保存，不证明任何多行组合都可构造 Strategy。
- **来源：** `项目事实` [ADR-0013](../../decisions/ADR-0013-lottery-domain-model.md)、[ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 14. 为什么两张表拆成两个 Migration？

- **直接回答：** MySQL atomic DDL 的原子边界是单条受支持 DDL，不是包含多条 CREATE 的整个 Migration 文件。拆为 v1/v2 后，每个版本对应一条原子 DDL；若 v2 失败，现场清楚表达“父表 v1 完成，子表 v2 dirty”，API 因未达到 clean latest v2 不启动。
- **追问：** 中间 clean v1 只有父表，会不会不一致？
  - **追问回答：** 它是迁移过程中的合法版本，不是兼容 API 的最终版本。发布编排等待全部 Migration 成功；如果有人只部署 v1 构件，只有 v1 的能力。版本边界与应用兼容必须共同管理。
- **项目证据：** [Migration 清单](../../../migrations/sql/README.md)、[dirty 故障注入说明](../../qa/lessons/lesson-18.md)、[Compose gate](../../../deploy/compose/compose.yaml)。
- **选型边界：** 一个 DDL 内部仍可能有锁/磁盘/复制风险；“atomic”不等于“在线无影响”或“多语句事务”。
- **来源：** `面经启发` [数据库内核社招复盘出现 online DDL 追问](https://www.nowcoder.com/discuss/721384389753446400)；`官方事实` [MySQL Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)。

## 15. MySQL DDL 失败后 dirty 是什么？为什么不能直接 force？

- **直接回答：** dirty 表示某个版本执行未完整确认，可能已经有部分不可回滚 DDL 副作用。直接 force 只改版本标记，不恢复结构；若错误判断，会让后续 Migration 建立在错误对象上。应先冻结、保存构件/checksum、查版本和 `SHOW CREATE TABLE`、核对日志与数据，再决定受控修复或新增前向 Migration。
- **追问：** 本节故障注入的预期是什么？
  - **追问回答：** 预建冲突子表后运行 up：v1 父表保留，v2 失败并显示 `2:1`；后续 status/up fail closed。它证明不能把两张表误认为事务整体回滚。
- **项目证据：** [QA dirty 场景](../../qa/lessons/lesson-18.md)、[Migration runbook](../../runbooks/mysql-migrations.md)、[Runner](../../../internal/infrastructure/migration/runner.go)。
- **选型边界：** force 是某些事故中可用的维护工具，但必须基于已核实结构和审批；它不属于产品正常命令，也不是 rollback。
- **来源：** `官方事实` [MySQL Atomic DDL](https://dev.mysql.com/doc/refman/8.4/en/atomic-ddl.html)；`项目事实` [ADR-0010](../../decisions/ADR-0010-mysql-migration-boundaries.md)。

## 16. 为什么 Migration 不使用 `IF NOT EXISTS`？

- **直接回答：** `IF NOT EXISTS` 只说明同名对象存在，不证明结构、约束、collation 与当前 Migration 一致。若人工预建了同名但缺 CHECK/FK 的表，语句可能 warning/继续，版本却被标记成功，产生最危险的“看起来绿色”的漂移。初始结构冲突必须显式失败并调查。
- **追问：** 幂等不是好实践吗？
  - **追问回答：** Migration 的幂等由版本表保证：已应用版本不再执行。SQL 自身的 `IF NOT EXISTS` 适合某些运维场景，但不能替代 schema equality。修复漂移应新增审查过的 Migration 或受控恢复。
- **项目证据：** [两份 DDL](../../../migrations/sql/README.md)、[Migration 规则](../../../migrations/README.md)。
- **选型边界：** 临时表、可丢对象或明确随后校验完整定义的脚本可能使用 IF NOT EXISTS；核心版本化表默认失败更安全。
- **来源：** `官方事实` [MySQL CREATE TABLE IF NOT EXISTS 语义](https://dev.mysql.com/doc/refman/8.4/en/create-table.html)；`项目事实` [ADR-0010](../../decisions/ADR-0010-mysql-migration-boundaries.md)。

## 17. 为什么给初始 Migration 固定 SHA-256？它能防什么、不能防什么？

- **直接回答：** 进入共享历史的 v1/v2 不应被回写。测试固定字节 checksum，任何换行、约束或 SQL 修改都会使门禁失败，要求新增更高版本；同时检查每文件一个语句终止符。它能防团队误改和学习分支漂移，但不是数字签名，不能证明作者身份、制品供应链或运行数据库确实来自该二进制。
- **追问：** SQL 格式化也不能改吗？
  - **追问回答：** 共享后不能，即使语义自认为不变，也会导致不同构件拥有同版本不同字节。格式修正应在冻结前完成；冻结后结构修正新增 Migration，文档修正不改 SQL。
- **项目证据：** [不可变测试](../../../migrations/embed_test.go)、[`.gitattributes`](../../../.gitattributes)、[迁移规则](../../../migrations/sql/README.md)。
- **选型边界：** 生产还需要构件签名、镜像 digest/SBOM 和部署审计；checksum 只是仓库内的一层。
- **来源：** `项目场景题`；`项目事实` [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 18. 为什么当前不添加更多索引？复合主键如何利用最左前缀？

- **直接回答：** 当前唯一明确查询是按 StrategyID 取全部 Awards；复合 B-tree 主键 `(strategy_id, award_id)` 可用最左前缀定位一个 Strategy 的连续键范围，并支持显式 `ORDER BY award_id` 的有序范围扫描。SQL 结果仍必须写 `ORDER BY`，不能依赖存储布局、执行计划或当前样本碰巧返回的物理顺序。没有按 name/outcome/time 查询、基数或 EXPLAIN，因此额外索引只会增加写放大、空间、buffer pool 和 DDL 成本。
- **追问：** 如果查询 `WHERE award_id=?` 呢？
  - **追问回答：** 不能高效使用该复合键的首列定位，可能全索引扫描；但 scoped identity 本就要求 StrategyID。如果未来出现合法跨 Strategy 的 AwardID 查询，应先确认用例，再评估 `(award_id, strategy_id)` 二级索引或全局身份。
- **项目证据：** [复合 PK DDL](../../../migrations/sql/000002_create_lottery_strategy_award.up.sql)、[无额外索引决策](../../decisions/ADR-0014-lottery-persistence-schema.md)。
- **选型边界：** “少建索引”不是反索引；第 19 节有 SQL 后要用数据分布、EXPLAIN、读写比和覆盖性决定。
- **来源：** `面经启发` [字节订单查询与索引设计](https://www.nowcoder.com/discuss/353155518426456064)、[数据库基础汇总的联合索引最左前缀](https://www.nowcoder.com/discuss/353147574485983232)、[4399 的项目索引策略追问](https://www.nowcoder.com/discuss/732675514812370944)；`官方事实` [Multiple-Column Indexes](https://dev.mysql.com/doc/refman/8.4/en/multiple-column-indexes.html)。

## 19. `created_at/updated_at` 为什么不是聚合版本或乐观锁？

- **直接回答：** 两列只是每一行的 MySQL 创建/最后更新元数据。更新 Award 行不会刷新父 Strategy 行；相同微秒、时钟和非业务更新也可能让时间值不适合作 CAS。当前没有 `WHERE version=?`、冲突错误或整聚合版本协议，所以不能用父 `updated_at` 做 ETag、缓存版本或“策略最后修改时间”。
- **追问：** 真正乐观锁怎么做？
  - **追问回答：** 常见做法是父表显式整数 version，更新完整聚合时同事务执行 `UPDATE ... SET version=version+1 WHERE version=?`，影响行数 0 表示冲突；也可以用不可变配置版本。具体取决于第 19/规则发布用例。
- **项目证据：** [时间列 DDL](../../../migrations/sql/README.md)、[设计手记时间风险](../../design-thinking/lessons/lesson-18.md)。
- **选型边界：** 单行简单资源可把时间戳用于条件更新，但必须有精度/比较/更新源的明确协议；本聚合跨两表，当前不满足。
- **来源：** `官方事实` [MySQL DATETIME/TIMESTAMP 自动初始化](https://dev.mysql.com/doc/refman/8.4/en/timestamp-initialization.html)；`项目事实` [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)。

## 20. 为什么第 18 节应用账号只有 SELECT，连业务表都不能写？

- **直接回答：** 权限应对应已实现用例。当前 API 没有 Repository、INSERT/UPDATE/DELETE SQL，授写权限没有正当消费者；两表 SELECT 是为第 19 节加载准备的最小面。更关键的是撤销 schema wildcard 后，应用不能再读写 `schema_migrations` 或未来任意表。第 19 节若有真实写 Repository，再按 SQL 精确扩展表级 DML。
- **追问：** 为什么不干脆本节零权限？
  - **追问回答：** 下一切片明确是读取 Repository，且真实集成可提前验证账号/表可见性；SELECT 是可解释准备。readiness 的 Ping 本身不需要表权限，所以 smoke 还专门执行两表零行查询，防止仅 Ping 掩盖授权缺失。
- **项目证据：** [grant 脚本](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)、[1142 权限测试](../../../migrations/lottery_schema_integration_test.go)、[API 边界](../../api/lessons/lesson-18.md)。
- **选型边界：** 最小权限是随版本演进的 allowlist，不是永久只读；不能为了少改脚本一次性授未来 CRUD。
- **来源：** `官方事实` [MySQL GRANT](https://dev.mysql.com/doc/refman/8.4/en/grant.html)、[REVOKE](https://dev.mysql.com/doc/refman/8.4/en/revoke.html)；`项目事实` [ADR-0015](../../decisions/ADR-0015-compose-schema-grant-reconciliation.md)。

## 21. 为什么修改 MySQL init 脚本还不够？

- **直接回答：** 官方镜像的 `/docker-entrypoint-initdb.d` 只在数据目录首次初始化时运行。用户保留的 `growthos_mysql_data` 已有账号和旧 schema wildcard grant，改仓库脚本不会自动撤销。若靠删 volume 重跑会破坏数据，也无法演练真实升级。因此必须在每次适当启动/迁移后运行可重复的权限 reconciliation。
- **追问：** 新 volume 还需要 init 脚本吗？
  - **追问回答：** 需要。init 创建两个账号并给 Migrator 初始 schema 能力；Migration 建表后，grant job 给 app 精确表级 SELECT。新 volume 与旧 volume 走向同一个最终权限状态，只是起点不同。
- **项目证据：** [init 脚本](../../../deploy/compose/mysql/init/10-create-growthos-users.sh)、[reconcile 脚本](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)、[独立新 volume 两轮 smoke 记录](../../qa/lessons/lesson-18.md)。
- **选型边界：** init-only 适合真正一次性、永不复用数据目录的测试；长期开发/生产必须有升级路径。
- **来源：** `官方事实` [MySQL Docker Official Image 初始化说明](https://hub.docker.com/_/mysql)；`项目事实` [ADR-0015](../../decisions/ADR-0015-compose-schema-grant-reconciliation.md)。

## 22. 为什么 grant job 使用 Unix socket、`network_mode: none` 和一次性生命周期？

- **直接回答：** 任务需要短时 root 数据库能力，但不应把 root 网络会话或 Secret 给长期进程。它只读挂载 MySQL socket，显式 `--protocol=socket`，无 Compose 网络，执行固定 REVOKE/GRANT/验证后退出；同时 read-only rootfs、drop ALL、no-new-privileges。API 只在其 exit 0 后启动。
- **追问：** 没网络是不是绝对安全？
  - **追问回答：** 不是。Unix socket 是高权限 IPC，容器还能读取 root Secret；若脚本/镜像被篡改仍可控制数据库。安全来自缩小接口、短生命周期、只读挂载、固定脚本、版本镜像和 fail-closed 的组合，而不是单一网络开关。
- **项目证据：** [Compose mysql-grants 服务](../../../deploy/compose/compose.yaml)、[ADR-0015 信任图](../../decisions/ADR-0015-compose-schema-grant-reconciliation.md)。
- **选型边界：** 生产可用平台 job、短期 IAM/角色或 DBA pipeline；不必照搬 socket，但应保留身份分离和完整期望状态验证。
- **来源：** `项目场景题`；`项目事实` [ADR-0015](../../decisions/ADR-0015-compose-schema-grant-reconciliation.md)。

## 23. 为什么完整比较 SHOW GRANTS 还要检查 `mandatory_roles`？

- **直接回答：** 包含目标 SELECT 只能证明权限不少，不能证明没有额外 direct grant，所以脚本先对排序后的完整 SHOW GRANTS 做精确相等比较。即使 direct grants 相等，MySQL 全局 `mandatory_roles` 仍可能让所有账号继承额外有效权限；本地基线因此要求 `@@GLOBAL.mandatory_roles` 为空，非空就阻断 API。安全审计要看有效权限，不只看账号对象上的文本。
- **追问：** 这是否已经穷举所有 MySQL 权限路径？
  - **追问回答：** 没有做形式化穷举；它针对固定 Compose 基线覆盖账号直接 grant 与强制角色这两个现实路径。若未来启用普通角色、代理用户、云 IAM 或数据库代理，必须重新定义有效权限测试，不能删除断言后继续声称精确最小权限。
- **项目证据：** [grant 脚本 allowlist/mandatory roles](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)、[QA 精确授权](../../qa/lessons/lesson-18.md)。
- **选型边界：** SHOW GRANTS 文本格式与 MySQL 版本耦合；升级导致失败应触发审查，而不是改成模糊 contains。
- **来源：** `官方事实` [MySQL SHOW GRANTS](https://dev.mysql.com/doc/refman/8.4/en/show-grants.html)、[Mandatory Roles](https://dev.mysql.com/doc/refman/8.4/en/roles.html#roles-mandatory)；`项目事实` [ADR-0015](../../decisions/ADR-0015-compose-schema-grant-reconciliation.md)。

## 24. 真实 MySQL 集成测试为什么不能只断言 SQL 字符串？本节最终能写进简历什么？

- **直接回答：** SQL 文本存在不能证明 MySQL 8.4 真正 enforce CHECK/FK、strict mode 不截断、driver 无损处理 unsigned、权限返回 1142 或 Migration dirty 正确。集成测试用独立 schema、不同 app/migrator 身份和显式破坏性 opt-in，验证三个 MaxUint64、128/129 中文、首/尾 U+0020、Outcome、孤儿、复合重复、RESTRICT 和精确授权；探针数据事务回滚，容器/volume 按精确目标清理。简历可写“设计并验证两表 Lottery schema、前向 Migration、复合键/外键/CHECK 和最小权限启动门”，不能写“完成高并发抽奖、Redis 缓存、亿级优化或零停机 DDL”。
- **追问：** 单元测试、集成测试、Compose smoke 分别证明什么？
  - **追问回答：** 单元/静态测试证明 Migration 文件不可变和代码状态机；真实 MySQL 测试证明引擎/driver/权限行为；Compose smoke 证明服务生命周期、live v2、grant job、端口与 HTTP 回归。三者互补，普通 `go test` 的 skip 不等于联调通过。
- **项目证据：** [集成测试](../../../migrations/lottery_schema_integration_test.go)、[Compose smoke](../../../scripts/compose-smoke.sh)、[完整 QA](../../qa/lessons/lesson-18.md)。
- **选型边界：** 本地 MySQL 8.4.11 和小数据不能证明生产锁、复制、容量或 SLO；大表 ALTER 还需影子库、online DDL/备份和回滚演练。
- **来源：** `面经启发` [数据库内核复盘中的 online DDL、执行计划与索引合理性](https://www.nowcoder.com/discuss/721384389753446400)、[后台面经对项目表设计的追问](https://www.nowcoder.com/discuss/353155632142426112)；`官方事实` [InnoDB Online DDL](https://dev.mysql.com/doc/refman/8.4/en/innodb-online-ddl.html)；`项目事实` [QA](../../qa/lessons/lesson-18.md)。

## 不能夸大的事实

- 两张表存在，不等于 Repository、业务 CRUD 或抽奖接口存在。
- Weight 存在，不等于随机算法、概率公平或库存控制存在。
- `reward` 是候选类别，不等于用户已中奖或权益已到账。
- 外键保证子有父，不保证父至少一个子。
- 单行 CHECK 不能保证跨行总权重不溢出。
- `name_basic` 只覆盖非空和默认 TRIM 的 U+0020 子集，不是完整 Go Unicode 契约。
- `utf8mb4_0900_bin` 保留敏感比较，不是 normalization、搜索或反欺骗方案。
- row `updated_at` 不是聚合版本、ETag 或缓存版本。
- atomic DDL 是单语句原子边界，不是多语句事务，也不代表无锁。
- SELECT only 是第 18 节最小权限，不是永久只读架构。
- socket/no-network 缩小 root job 攻击面，不让 root Secret 变成低风险。
- direct SHOW GRANTS 精确仍需 mandatory role/环境继承检查。
- 本地集成/Compose 结果不能冒充生产容量、复制、备份或零停机证据。
- 牛客面经是候选人自述，只用于题型启发，不是官方技术规范。

## 复习清单

- [ ] 60 秒内讲清两表、两 Migration、复合键、外键、CHECK 和权限门。
- [ ] 能解释 Award scoped identity 与复合主键最左前缀。
- [ ] 能说明为什么三个字段都要验证 MaxUint64，以及 JS 精度后果。
- [ ] 能比较物理/逻辑外键、RESTRICT/CASCADE 的适用边界。
- [ ] 能解释 ENUM、VARCHAR+CHECK、字典表的取舍。
- [ ] 能准确说出 `name_basic` 只覆盖 U+0020 子集。
- [ ] 能说明数据库为何不能保证至少一个 Award 和总权重安全。
- [ ] 能画出 v1 成功、v2 dirty 的失败路径，不把 atomic DDL 说成事务。
- [ ] 能解释为什么不用 IF NOT EXISTS、为什么冻结 checksum。
- [ ] 能从真实查询而非“数据量大”决定索引。
- [ ] 能说明行 timestamp 与聚合 version 的区别。
- [ ] 能画出 mysql→migrate→mysql-grants→api。
- [ ] 能解释旧 volume 为什么不重跑 init、为什么需要 REVOKE 后重建。
- [ ] 能解释 direct grants、mandatory roles 与有效权限。
- [ ] 能区分静态测试、真实 MySQL、Compose smoke 和生产证据。
- [ ] 回答前检查“不能夸大的事实”，不把下一章节能力倒灌到本节。
