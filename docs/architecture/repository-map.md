# 仓库地图

**状态：** 当前
**更新日期：** 2026-08-29

本文件描述当前仓库，而不是第 96 节的目标仓库。目录随需求演进，改动目录职责时必须同步更新本文件。

第 9 节已验收仓库工程基线。第 11～12 节落实 Gin 进程、配置、结构化日志、请求关联和错误适配；第 13 节加入 MySQL 连接池、独立 Migration 命令、数据库 readiness 与真实 MySQL 8.4 验收；第 14～15 节完成 React 框架和首个真实前后端系统探针切片；第 16 节把这些能力装配为隔离的 Compose M0 开发栈；第 17 节第一次在 `internal/lottery/domain` 落地持久化无关的 Strategy/Award 聚合。当前仍没有业务表、业务 Repository、抽奖算法或业务 API，Redis 容器也只是尚未接入 API 的环境占位。

| 路径 | 当前职责 | 引入产品代码的章节 |
| --- | --- | --- |
| `cmd/doccheck` | 文档完整性与课程证据检查器 | 项目基线 |
| `cmd/docsync` | 项目 README/docs 到 Obsidian Vault 的单向镜像工具 | 项目基线 |
| `cmd/growth-api` | Go 产品进程入口：版本、信号、MySQL pool、Gin/HTTP 装配与资源关闭 | 第 11～13 节 |
| `cmd/growth-migrate` | 独立前向迁移入口，只暴露 `up/status`，装配 Migrator 账号、嵌入 source 与 Runner | 第 13 节 |
| `cmd/healthload` | 标准库定速 HTTP 负载器；输出单行 JSON，并以错误、异常状态、丢弃请求或 P99 越界作为失败 | 第 16 节 |
| `internal/platform/appconfig` | `GROWTHOS_` API/Migration 独立配置、直接/文件秘密二选一、公开默认值、秘密脱敏与聚合/跨组件校验 | 第 12～13、16 节 |
| `internal/platform/logging` | 标准库 `slog` logger 构造、级别、格式与基础字段 | 第 12 节 |
| `internal/platform/fault` | 与传输无关的 fault kind、稳定 code、公开消息与内部 cause；包含 unavailable 语义 | 第 12～13 节 |
| `internal/infrastructure/httpapi` | Gin router、`/health`、MySQL `/ready`、请求中间件、错误映射、统一错误 `no-store` 缓存策略与 HTTP 契约测试 | 第 11～15 节 |
| `internal/infrastructure/httpserver` | 标准库 HTTP Server 运行、配置化 timeout、context 取消、优雅关闭与脱敏 ErrorLog 桥接 | 第 11～12 节 |
| `internal/infrastructure/mysql` | 安全 driver Config、TLS、API `sqlx` pool、Migration 单连接、首次 Ping、稳定错误 stage 与不绕过 JSON 边界的 driver logger | 第 13、16 节 |
| `internal/infrastructure/migration` | 嵌入 source 校验、前向 `up/status`、dirty/version/cancelled 状态机与资源所有权 | 第 13 节 |
| `internal/lottery/domain` | 持久化/传输无关的 Strategy 聚合、Award 候选、正整数相对权重、显式 reward/no_reward、名称契约与聚合单元测试 | 第 17 节 |
| `internal/*` | Lottery 之外仍预留的私有领域与基础设施边界；占位不代表实现 | 随对应领域章节引入 |
| `pkg` | 可被外部导入的少量稳定 Go 包 | 仅在确有跨模块公共契约时 |
| `configs/growth-api.env.example` | 不自动加载且不给密码赋值的 API/Migration 公开环境变量示例 | 第 12～13 节 |
| `migrations/embed.go` | 编译期嵌入 Migration 说明与 `sql/` source | 第 13 节 |
| `migrations/sql` | 严格命名的前向 `.up.sql`；当前为空，第 18 节加入首个 `000001` | 第 13 节机制，第 18 节业务结构 |
| `scripts/generate-compose-secrets.sh` | 完整 Secret 集合生成/验证；部分集合与“旧 MySQL volume + 缺凭据”状态 fail closed，阻止静默错配 | 第 16 节 |
| `scripts/compose-smoke.sh` | 服务状态、迁移结果、HTTP 契约与宿主机端口隔离的只读冒烟检查 | 第 16 节 |
| `deploy/compose` | M0 服务拓扑、精确 MySQL grants、named volume 与被忽略的本地 Secret 文件约定 | 第 16 节 |
| `deploy/docker` | API/Migrator/Web/Redis 构建边界、非 root 运行入口和脱敏 Nginx 同源网关 | 第 16 节 |
| `web` | 统一 React 用户端、运营端、MCP 与 AI Operator 框架；系统状态真实联调，其余业务页面仍为 Mock | 第 14～15 节 |
| `web/src/api` | 只访问同源路径的 HTTP client、六类失败语义、运行时 decoder 与系统探针 API | 第 15 节 |
| `web/src/pages/system/status` | `/system/status` 页面、并行探针 hook、取消/竞态控制和组件测试 | 第 15 节 |
| `web/vite.config.ts` | dev/preview 的 `127.0.0.1` 严格端口和 `/health`、`/ready`、`/api` 精确同源代理 | 第 15 节 |
| `docs` | 产品、架构、决策、QA、第一性原理设计推导、面试问答和课程事实 | 全程 |
| `docs/design-thinking` | 按章节保存事实到机制的推导、备选方案、失败模型、风险账本与重决策条件 | 第 13 节起，历史章节回填 |
| `docs/interview` | 按章节保存可口述问答、追问、项目证据、选型边界与分级外部来源 | 第 13 节起，历史章节回填 |
| `docs/runbooks` | MySQL Migration 与本地 Compose 的运行、发布、故障停止条件、恢复和数据清理纪律 | 第 13、16 节 |

## 当前依赖规则

1. `cmd/*` 只做装配和进程生命周期管理。
2. `internal/<domain>` 拥有自己的领域模型、应用用例和端口。
3. 领域模块不得直接依赖另一个模块的数据库实现。
4. `internal/infrastructure` 实现数据库、缓存、消息等技术适配器，不承载业务规则。
5. `pkg` 不是杂物目录；不稳定或仅仓库内部使用的代码留在 `internal`。
6. 当前 `.gitkeep` 只表示计划边界，不代表能力已经实现。
7. 浏览器 API 适配只接受同源路径；开发代理目标由仅 Vite 进程读取的 `GROWTHOS_WEB_API_PROXY_TARGET` 配置，默认 `http://127.0.0.1:8080`，不向浏览器暴露后端 origin。
8. `internal/lottery/domain` 不导入 Gin、SQL/sqlx、Redis 或 JSON/DB tag；未来 adapter 只能通过构造器重建合法聚合。

第 11 节的 `/health` 仍是无外部依赖的进程 liveness，只证明 Gin 路由和 handler 能响应。第 13 节的 API 在监听前必须打开并 Ping MySQL，运行中 `/ready` 每次用有界 Ping 表示数据库 readiness；依赖故障时 `/ready` 为 503 而 `/health` 仍可为 200。两者都不证明业务数据正确、Migration 最新或 SLO 达标。

第 12 节保持 `request_id` 与未来 OpenTelemetry `trace_id` 分离；fault 平台层不导入 Gin/HTTP，只有 HTTP adapter 决定 status 和公开 error envelope。配置与隐私规则见[配置参考](../configuration.md)，长期边界见[ADR-0009](../decisions/ADR-0009-runtime-boundaries.md)。

第 13 节保持 API 与 Migration 身份和进程分离：`growth-api` 使用受限 pool 且不执行 DDL，`growth-migrate` 使用专用单连接且只提供前向 `up/status`。当前空 source 正确返回 `no_migrations`，不会创建业务表或占用首个版本号。边界见 [ADR-0010](../decisions/ADR-0010-mysql-migration-boundaries.md)，操作步骤见 [MySQL Migration 运维手册](../runbooks/mysql-migrations.md)。

第 15 节的系统状态页通过 Vite dev/preview 同源代理真实消费 `GET /health` 与 `GET /ready`，统一 client 做运行时 JSON 契约检查，并由状态 hook 管理并行请求、取消和过期结果。Go 统一错误响应明确携带 `Cache-Control: no-store`。正常、数据库不可用和 API 离线场景已做真实浏览器关联验收，但这些证据不代表吞吐、长期可用性或生产 SLO 已验证。前端工具链要求 Node.js `>=22.22.2`、pnpm `10.13.1`，质量门包含 `test`、`typecheck` 和 `build`。

第 16 节的 Compose 拓扑只发布 `127.0.0.1:8088` 的 Nginx Web 入口。Web/API 位于 `edge`，API/MySQL/Migrator 位于内部 `data`，Redis 单独位于内部 `cache`；Migration 成功退出后 API 才启动。API、Migrator、Web、Redis 使用非 root、只读根文件系统、去除 capabilities 与 `no-new-privileges`，MySQL 官方镜像则保留初始化阶段 root 和可写数据目录这一明确例外。Nginx 动态解析 API 容器地址，统一回写 `X-Request-ID`，并从 access/error 日志中排除 query 与 Referer。长期边界见 [ADR-0012](../decisions/ADR-0012-compose-development-topology.md)，操作步骤见 [Docker Compose 运维手册](../runbooks/local-compose.md)。

第 17 节的 `Strategy` 拥有至少一个 `Award`，在构造时检查正 ID、名称、正权重、封闭 Outcome、AwardID 唯一和总权重溢出，并对候选 slice 做防御性复制和 AwardID 规范排序。`reward` 只表示待后续 Benefit 流程处理的奖励候选，`no_reward` 是合法候选而不是 error。该包没有被 `growth-api` 装配，也没有表、Repository、算法、API、真实前端或 Redis 调用；一次 Draw 的最终结果尚不存在，INV-03 尚未满足。长期边界见 [ADR-0013](../decisions/ADR-0013-lottery-domain-model.md)。

第一版运行时采用 [ADR-0007](../decisions/ADR-0007-modular-monolith-first.md) 确定的模块化单体：一个 Go 产品进程可以装配多个领域模块，但共享进程和数据库实例不改变事实所有权。服务拆分必须等待第 73 节后的负载、发布、故障域、合规或团队证据。

## 有意延迟的决定

- Gin 在第 11 节作为 Go HTTP 基线接入；gRPC + Protobuf 是后续服务间 RPC 基线，到第 75 节再按拆分需求接入。
- 第 12 节已把监听地址、HTTP timeout、日志级别和格式纳入显式配置，并建立请求关联与统一错误。
- MySQL 连接、`sqlx` pool 与前向 Migration 机制已在第 13 节接入；业务手写 SQL、Repository 和事务用例分别等待真实业务章节。
- React、TypeScript、Vite、Tailwind CSS、Lucide、Recharts 和 Zustand 在第 14 节接入；第 15 节只接通系统探针，业务页面继续使用 Mock，等待对应业务 API 和表真正出现。
- 第 16 节已引入 Compose 本地开发环境，但它仍是单机开发拓扑：没有镜像 digest 固定、内部 TLS、生产资源配额、Secret Manager 或业务 Redis 接入。
- 第 17 节只建立 Lottery 纯领域模型；SQL 表、第一个 `000001` Migration、Repository、概率算法、业务 API、真实抽奖页和 Redis 分别等待第 18～24 节的真实问题。
- 服务拆分、RPC 和注册中心延迟至第 73 节以后。
- 最终目录图和 ER 图延迟至第 96 节复盘。
