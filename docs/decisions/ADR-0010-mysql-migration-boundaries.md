# ADR-0010：MySQL 连接、账号隔离与前向 Migration 边界

- **状态：** 已接受
- **日期：** 2026-08-29
- **负责人：** GrowthOS 维护者

## 背景

第 12 节已经建立类型化配置、日志和 HTTP 错误边界。第 13 节开始连接 MySQL，但第 18 节才会出现第一批业务表。现在需要决定的是数据库基础设施如何运行，而不是提前设计最终 schema。

如果只以“本机能连上 MySQL”为目标，最容易形成以下长期风险：API 与迁移共用高权限账号；应用启动自动执行 DDL；连接池没有上限；liveness 把数据库故障误判为进程崩溃；Migration 暴露 `force/drop/down`；失败日志复制 DSN、SQL 或驱动 cause；为空占位而消耗首个版本号。

本 ADR 固定第 13 节以后必须共同遵守的连接、身份、探针和 Migration 边界。

## 评估过的方案

### 数据访问

| 方案 | 优点 | 当前问题 | 结论 |
| --- | --- | --- | --- |
| ORM + AutoMigrate | 模型到表上手快 | schema 历史和 SQL 代价不透明，容易在运行进程中隐式 DDL | 不采用 |
| 标准 `database/sql` + 手写扫描 | 依赖最少 | 后续结构体映射和命名查询样板较多 | 保留为底层 |
| `sqlx` + 手写 SQL | 标准库超集、SQL 仍可审查、迁移成本低 | 需要显式维护 SQL 与映射 | 采用 |

### Migration 执行

| 方案 | 优点 | 当前问题 | 结论 |
| --- | --- | --- | --- |
| API 启动自动迁移 | 部署步骤少 | 流量启动与 DDL 耦合，多实例竞争，失败恢复不可控 | 不采用 |
| 自研版本表、锁和执行器 | 完全控制 | 重复实现 source、锁、dirty、版本与数据库 adapter | 不采用 |
| `golang-migrate` 库 + 项目受限命令 | 复用成熟机制，同时限制产品操作面 | 需要封装并审查上游 adapter 行为 | 采用 |

### 健康语义

| 方案 | 问题 | 结论 |
| --- | --- | --- |
| `/health` 每次 Ping 所有依赖 | 依赖抖动会触发进程重启，无法区分 liveness 与 readiness | 不采用 |
| `/health` 只看进程，`/ready` 检查接流量依赖 | 语义清楚，平台可分别配置重启与摘流 | 采用 |

## 决策

1. 当前数据库服务端基线为 MySQL 8.4；Go 使用 `go-sql-driver/mysql` v1.10.0，数据访问使用 `sqlx` v1.4.0 和手写 SQL，Migration 使用 `golang-migrate/migrate` v4.19.1。
2. API 账号和 Migrator 账号必须分开配置、授权和建连。API 账号不拥有 DDL 权限；Migration 账号不进入在线请求进程。
3. 连接配置使用 driver 的结构化 Config 构建 connector，不在应用边界拼接或记录可打印 DSN。密码无默认、只由外部 Secret 注入；含密码配置在常见格式化、slog 和 JSON 边界整体脱敏。
4. API 使用有界 `sqlx.DB` 池，设置最大打开/空闲连接、最大寿命和空闲时长；监听 HTTP 前用有界 `PingContext` 验证配置与可达性。失败关闭部分创建的池并安全退出。
5. `/health` 保持无外部依赖的进程 liveness。新增 `/ready`，通过同一 API 池执行有界 Ping：成功返回 200；失败返回统一 503 `dependency_unavailable`，不公开驱动 cause。
6. readiness 的 Ping timeout 加固定 1 秒响应预算不得超过 HTTP write timeout。
7. staging 和 production 必须使用证书链与主机名都验证的 `verify_identity`。可选 CA 只扩展系统信任根，且必须可读、为有效 PEM、不得超过 1 MiB；TLS disabled 时禁止提供 CA。development/test 可以显式 disabled。
8. API 连接禁用 multi-statements、弱密码插件、明文/降级认证和任意本地文件访问。Migration 因版本文件可包含多条语句，只在专用单连接池中开启 multi-statements。
9. Migration 文件编译期嵌入二进制，只接受 `NNNNNN_description.up.sql`，拒绝 `.down.sql`、重复/零版本和非法命名。已共享的文件不可修改，只能增加下一条前向 Migration。
10. 产品命令 `growth-migrate` 只暴露严格小写的 `up` 与 `status`；不暴露 `down`、`drop`、`force` 或任意版本跳转，API 启动也不自动执行 Migration。
11. 第 13 节不创建空 Migration，不占用 `000001`。首个真实 `000001_*.up.sql` 留给第 18 节基于实际业务需求建表。
12. Migration timeout 默认满足 `statement 30s + 5s <= network read 35s + 5s <= lock acquisition 40s`。三层使用独立变量，并在配置、连接器和 Runner 边界重复校验。
13. 上游 v4.19.1 MySQL adapter 的 `GET_LOCK` 使用固定 10 秒服务端等待；GrowthOS 锁获取 timeout 最小 11 秒、默认 40 秒，并保证 network read 至少提前 5 秒结束，避免外层获取锁等待先返回。它不限制 `RELEASE_LOCK`、整个 Migration 或总清理时间。
14. dirty、当前嵌入历史不存在的版本、比二进制更新的版本以及取消都属于 fail-closed。Migration engine 启动后的失败/取消会重新读取 dirty；执行中的 `Up` 被取消后 Runner 进入 terminal，必须关闭并重建，调用前已取消或 `Status` 被取消不改变其可复用状态。
15. 错误对外和通用日志只渲染稳定 stage。底层驱动 cause 保留在受控错误链中，但入口不直接格式化它；SQL、DSN、密码、CA 路径和拓扑不能进入通用日志。
16. 普通测试可以在缺少真实数据库变量时 skip 集成测试，但不能据此宣称 MySQL 联调通过。真实验收必须显式执行缺变量即失败的 `make test-integration-mysql` 并记录 MySQL 版本、身份隔离和清理结果。

## 为什么采用前向而不是自动回滚

MySQL DDL 具有隐式提交与在线变更风险，应用回滚也不等于 schema 可以安全回滚。自动 `down` 容易丢失新字段、新表或已经写入的数据。GrowthOS 采用：

- 发布前备份和影子库演练；
- 兼容性优先的 expand/contract；
- 失败时停止、诊断并新增纠正 Migration；
- dirty/版本漂移由审批后的运维流程处理，而不是在产品 CLI 暴露通用 `force`。

这不意味着任何前向 SQL 都天然安全；每条 Migration 仍要评估锁、耗时、磁盘、复制和应用兼容窗口。

## 影响

- `growth-api` 从第 13 节起必须获得 API 密码并能在启动时连接 MySQL，不能仅凭公开默认值启动。
- API 运行中 MySQL 故障不会改变 `/health`，但会让 `/ready` 返回 503，供流量平台摘除实例。
- 发布流程多一个独立 Migration 步骤，但 DDL 不再与在线进程启动耦合。
- 两套账号和 Secret 增加本地/部署配置成本，换取最小权限和审计清晰度。
- `sqlx` 当前只建立连接边界；在第 19 节出现 Repository 前，不代表事务封装或业务 SQL 已完成。
- 当前空迁移集不会创建业务表或消耗版本号，课程可以在第 18 节真实展示第一次 schema 决策。
- forward-only 降低自动破坏面，但要求发布前兼容性设计、备份和人工恢复纪律。

## 未来演进

- 第 16 节把 MySQL 和探针接入 Docker Compose，但不改变账号与命令边界；
- 第 18 节创建首个业务 Migration，第 19 节用 `sqlx` 实现 Repository；
- 数据量和锁风险出现后，引入影子库、在线 DDL 或专用变更平台需新增 ADR；
- 读写分离、代理、分库分表或独立数据库只有真实负载和故障证据后再决策；
- 第 94 节增加 OTel 时，可以在数据库调用上增加 span，但仍不得把 DSN、SQL 参数或密码作为 attribute。

## 参考

- [MySQL 8.4 Access Control](https://dev.mysql.com/doc/refman/8.4/en/access-control.html)
- [MySQL VERIFY_IDENTITY 连接选项](https://dev.mysql.com/doc/refman/8.4/en/connection-options.html)
- [Go `database/sql`](https://pkg.go.dev/database/sql)
- [`go-sql-driver/mysql` v1.10.0 连接池与 timeout](https://github.com/go-sql-driver/mysql/tree/v1.10.0#connection-pool-and-timeouts)
- [`sqlx` 项目说明](https://github.com/jmoiron/sqlx)
- [`golang-migrate` v4.19.1 MySQL adapter](https://github.com/golang-migrate/migrate/blob/v4.19.1/database/mysql/mysql.go)
- [`golang-migrate` v4.19.1 dirty 与数据库锁 FAQ](https://github.com/golang-migrate/migrate/blob/v4.19.1/FAQ.md)
