# 第 13 节 QA 验收证据

- **日期：** 2026-08-29
- **产物：** [接入 MySQL 与 Migration](../../course/part-02/lesson-13-mysql-migrations.md)
- **分支：** `codex/lesson-13-mysql-migrations`
- **实现提交：** `b3f5aa7`
- **交叉审查加固提交：** `b734463`（均已推送至同名 `origin` 分支）
- **结果：** 通过

## 验收环境

| 项目 | 实际值 |
| --- | --- |
| 宿主地址 | `127.0.0.1:3306` |
| Docker 容器 | `mysql` |
| Docker 镜像 | `mysql:8.4` |
| MySQL 服务端 | 8.4.11 |
| Go | 1.26.6 |
| MySQL driver | `go-sql-driver/mysql` v1.10.0 |
| 数据访问 | `sqlx` v1.4.0 |
| Migration | `golang-migrate/migrate` v4.19.1 |

真实凭据由任务环境临时注入，未写入命令输出、本文件、Git 或日志。

## 验收结论

| 检查项 | 实际证据 | 结果 |
| --- | --- | --- |
| API 与 Migration 使用独立账号、密码和配置类型 | 真实双账号连接、装配测试和配置边界 | 通过 |
| API 账号没有 DDL 权限 | API 尝试创建隔离表，MySQL 返回权限错误 1142 | 通过 |
| Migrator 可执行审核的多语句 DDL | 专用单连接、多语句隔离探针 | 通过 |
| API pool、首次 Ping 与会话不变量正确 | 真实连接、`DBStats`、`SELECT 1`、UTC/`utf8mb4` 查询 | 通过 |
| staging/production TLS、地址、用户名、密码和 timeout 严格校验 | 配置与 connector 正负向测试 | 通过 |
| 密码、DSN、SQL、CA 路径和驱动 cause 不进入通用输出 | 格式化/slog/JSON 哨兵测试与真实日志扫描 | 通过 |
| `/health` 与 `/ready` 语义分离 | 正常、锁连接、kill 连接后的真实 HTTP 冒烟 | 通过 |
| readiness 失败使用安全 503 envelope | 真实 `/ready` 503 与响应/日志检查 | 通过 |
| API 数据库 opener/首次 Ping 失败时不监听且关闭部分资源 | 入口与 adapter 失败路径测试 | 通过 |
| 空产品迁移集不占 `000001` | 真实 `up/status=no_migrations`，未创建 `schema_migrations` | 通过 |
| Runner 首次 applied、再次 no_change、status clean | 隔离版本表真实集成测试 | 通过 |
| dirty、版本漂移、取消与 terminal fail closed | Runner 状态机、错误分类和命令测试 | 通过 |
| 非法 source、`.down.sql`、重复版本被执行前拒绝 | 内存 FS source 测试 | 通过 |
| 普通测试 skip 与真实集成 PASS 被明确区分 | 普通全仓测试 + 显式 `make test-integration-mysql` | 通过 |
| 临时 schema、账号和任务文件精确清理 | 按本任务临时命名 pattern 查询 `information_schema`、`mysql.user` 与 TMP 文件，计数均为 0 | 通过 |

## 自动化验证

实际完成：

```bash
go mod verify
go test -count=1 ./...
go test -race -count=20 \
  ./cmd/growth-api \
  ./cmd/growth-migrate \
  ./internal/platform/appconfig \
  ./internal/infrastructure/httpapi \
  ./internal/infrastructure/mysql \
  ./internal/infrastructure/migration
go vet ./...
make test-integration-mysql
make verify
```

结果：

- module 校验、全仓普通测试与 `go vet ./...` 通过；
- 六个第 13 节目标包各执行 20 次 Race 测试，全部通过且无 data race；
- `make test-integration-mysql` 在八个必需变量都由临时环境提供时直接通过，没有 skip；
- 普通 `go test ./...` 在没有真实测试变量的环境中仍允许两个 MySQL 集成测试 skip，这是可复现单元门禁的设计，不能替代上一条显式结果；
- 所有课程、API、QA、ADR、Runbook、面试问答和第一性原理设计手记落齐后，最终 `make verify` 通过：Go vet/test、文档结构与断链检查、TypeScript 类型检查和 Vite 生产构建均成功；Vite 保留一个主 chunk 超过 500 kB 的非阻断 warning，作为后续按真实路由拆包的观察项。

## 真实身份与连接验收

真实 MySQL 8.4.11 中建立了任务隔离的 API 与 Migrator 账号：

1. 两个账号分别使用自己的 address/database/user/password 建连并完成 `SELECT 1`；
2. API session 的 `time_zone=+00:00`、`character_set_connection=utf8mb4`；
3. API pool 的 `MaxOpenConnections` 与测试配置一致；
4. API 执行 `CREATE TABLE` 被 MySQL 1142 明确拒绝，证明失败来自数据库授权，而不是语法、网络或错误 schema；
5. Migrator 专用连接可以执行创建、插入和删除组成的多语句 DDL 探针；
6. Migration pool 保持单连接，multi-statements 没有扩散到 API pool。

测试使用隔离对象，不占用产品 Migration 文件或业务表。

## 真实 Migration 验收

### 产品空迁移集

当前 `migrations/sql` 没有 `.up.sql`：

- `growth-migrate status` 返回 `no_migrations`；
- `growth-migrate up` 返回 `no_migrations`；
- 目标库没有创建产品 `schema_migrations`；
- `000001` 仍保留给第 18 节首个真实业务建表。

### 隔离 Runner

集成测试用内存 FS 提供任务隔离的 `000001_integration_probe.up.sql`，并使用独立临时版本表：

- 第一次 `Up` 返回 `applied/version=1`；
- 第二次 `Up` 返回 `no_change`；
- `Status` 返回 `clean` 且 current/latest 都为 1；
- 测试完成后关闭 Runner，并删除隔离版本表。

这份隔离证据只验证机制，不代表产品 schema 已经有版本 1。

## 真实 HTTP 与故障冒烟

API 使用真实数据库启动并完成首次 Ping 后：

| 动作 | 实际结果 |
| --- | --- |
| `GET /health` | 200，保持 `ok/version/timestamp` 三字段 liveness |
| `GET /ready` | 200，返回 `ready/version/timestamp` |
| 锁定并 kill API 数据库连接后访问 `/ready` | 503，统一 `dependency_unavailable/service unavailable/request_id` envelope |
| 同一故障窗口访问 `/health` | 200，证明 liveness 不依赖 MySQL |
| 进程优雅停止 | 数据库池关闭，退出路径通过 |

故障响应没有数据库 host、账号、DSN、SQL、驱动 error 或密码。真实日志扫描也没有凭据哨兵。

## 配置与安全证据

- API 只要求 `GROWTHOS_MYSQL_PASSWORD`，Migration 只要求 `GROWTHOS_MYSQL_MIGRATION_PASSWORD`；
- `LoadMigration` 不因非法 HTTP/API 专属变量失败；
- MySQL address 必须有非空 host，数据库名使用严格标识符；
- 用户名为 1～32 个可打印 Unicode 字符，无控制字符与首尾空白；
- staging/production 强制 `verify_identity`，disabled 时拒绝 CA；
- API pool 范围和 idle/open 关系受校验；
- Ping 加 1 秒 response budget 不超过 HTTP write timeout；
- Migration 默认在 SQL 执行阶段满足 `statement 30s + 5s <= read 35s`，在获取锁阶段满足 `read 35s + 5s <= lock-acquire 40s`；lock 最小 11 秒；
- 所有含密码配置类型在 String、GoString、slog 和 JSON 边界脱敏；
- 命令错误日志只记录稳定 component/operation/stage，不格式化底层 cause。

## 当前未覆盖项与剩余风险

- 当前没有业务 Migration、业务表、Repository 或事务，不能证明任何营销事实已持久化；
- readiness 只验证连接，不证明 SQL P99、锁等待、复制延迟或业务数据正确；
- 默认 10/10 pool 没有容量压测，M0 第 16 节仍需验证连接池与 Compose 稳定性；
- 没有大表 DDL、元数据锁、崩溃恢复、备份恢复或灾备演练；
- development/test 使用 disabled TLS 的真实联调，不能替代 staging/production 证书与 hostname 验收；
- 产品命令没有 `force`，dirty 修复仍需要经过审批的 DBA 事故流程；
- React 尚未消费 `/health`、`/ready` 或 error envelope，第 15 节以前没有浏览器联调证据；
- 当前 Vite 主 chunk 为约 695 kB（gzip 约 206 kB），构建只给出非阻断 warning；本节没有为消除提示提前拆分尚未联调的页面，第 15 节起随真实路由与加载路径复核。

## 清理记录

- 已删除真实联调创建的临时 schema、API/Migration 临时账号、隔离表、隔离版本表和任务文件；
- 清理后按本任务临时命名 pattern 查询 `information_schema`、`mysql.user` 与 TMP 文件，计数均为 0；
- 保留用户原有 `mysql` 容器、Docker Volume、Go module/build cache 和其他可复用依赖；
- 没有删除用户原有数据库、账号或源码。
