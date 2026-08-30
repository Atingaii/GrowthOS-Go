# GrowthOS 本地 Docker Compose 运维手册

**适用范围：** 第 16～28 节单机 Docker Desktop/Engine 开发环境

**默认入口：** `http://127.0.0.1:8088`

**默认 Compose project：** `growthos`

**数据边界：** 只有本项目 `mysql_data` named volume 持久业务/迁移数据；`mysql_socket` named volume 只承载运行期 Unix socket；Redis 明确不持久；用户已有 MySQL、Redis、RabbitMQ、PostgreSQL 等资源不在本手册操作范围内

架构依据见 [ADR-0012](../decisions/ADR-0012-compose-development-topology.md)、[ADR-0015](../decisions/ADR-0015-compose-schema-grant-reconciliation.md)、[ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)、[ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)与 [ADR-0024](../decisions/ADR-0024-lottery-strategy-routing-graph-persistence.md)。当前缓存契约见[第 24 节 API](../api/lessons/lesson-24.md)，第 28 节 graph 仍为[零公开 API](../api/lessons/lesson-28.md)；MySQL 结构与权限证据见[第 28 节 QA](../qa/lessons/lesson-28.md)和 [MySQL Migration 运维手册](mysql-migrations.md)。

## 1. 目的

本手册说明如何安全创建、启动、检查、演练、停止和排查 GrowthOS Compose 环境，包括 latest 5 Lottery Migration、长期 runtime 的旧两表 SELECT-only/新三表 denied 授权门、Redis Strategy 投影缓存、长期 smoke、一次性 Lottery/cache acceptance，以及第 28 节 disposable MySQL 8.4.11 gate。所有命令默认从包含 `go.mod`、`Makefile` 和 `deploy/compose/compose.yaml` 的仓库根目录执行；不要把某位开发者的绝对路径写入脚本或交接材料。

这里区分“当前源码契约”和“某个已经运行的本地实例”：源码、镜像 tag、Migration 与 smoke 已要求 latest 5，旧 volume 仍只有实际执行前向迁移后才能记录为 v5。本节默认长期 `growthos` 已在保持同一 MySQL container ID、named volumes 和 networks 的情况下从 `2:0` 前向到 `5:0`；旧两表 `0:0:empty` 指纹前后相同，新三表 `0:0:0`，`growthos_app` graph SELECT 真实返回 1142，随后 `make compose-smoke` exit 0。隔离 Lottery/cache acceptance 也已在 v5 exit 0，并清理其随机 project、volumes、networks、images、builder、Secret 和响应；长期资源 identity 不变。这是本节验收事实，不代表以后任意旧 volume 会自动升级。

它不是生产发布手册，不授权操作者删除用户现有容器/volume、修改共享数据库账号、绕过 Secret guard，或把本地 HTTP/密码/TLS 配置复制到 staging/production。

## 2. 绝对安全规则

执行任何命令前先接受以下不变量：

1. **只操作明确 Compose project。** 所有底层 Compose 命令同时带 `--project-name growthos` 和 `--file deploy/compose/compose.yaml`；使用 Make 时由 `COMPOSE_PROJECT`、`COMPOSE_FILE` 提供同一边界。
2. **不按容器显示名称猜目标。** 用户可能已有名为 `mysql`、`redis` 的容器；GrowthOS 不设置 `container_name`，由 Compose label 标识所属 project。
3. **不使用全局清理。** 禁止以 `docker system prune`、`docker volume prune`、`docker container prune`、通配符删除或 Docker Desktop 批量删除作为本节清理方式。
4. **普通停止不删除 named volumes。** `make compose-down` 不带 `--volumes`，必须保留 `growthos_mysql_data` 与 `growthos_mysql_socket`；后者可重建但属于当前拓扑的明确资源。
5. **不读取/打印 Secret 内容。** 不执行 `cat deploy/compose/secrets/*`，不把内容复制到命令行、聊天、QA、截图或日志。
6. **不补齐部分 Secret 集合。** 四个 Secret 必须来自同一批；缺一项时恢复原文件或进行经过确认的数据/凭据重置。
7. **已有 volume 时不随机重建密码。** MySQL 初始化脚本只在空数据目录执行；新 Secret 不会自动修改 volume 内账号。
8. **故障演练只停止 GrowthOS service。** 不停止用户外部 MySQL/Redis 来模拟故障。
9. **不把容器停止当作事务回滚。** Migration/DDL 中断需要先检查状态，不得靠删除容器或版本表恢复。
10. **M0 是本地门禁。** 结果不得写成生产容量、SLA、峰值或资源上限。

## 3. 组件和影响范围速查

| service | 宿主机访问 | 持久状态 | 停止后的主要表现 |
| --- | --- | --- | --- |
| `web` | 唯一 published loopback 端口 | 无 | 浏览器入口连接失败；内部服务不自动停止 |
| `api` | 不发布；仅 Web 经 edge 访问 | 无 | SPA 仍可访问；代理返回带 ID 的 502/504 |
| `migrate` | 不发布 | 对 MySQL schema 可能有持久影响 | 正常为退出 0；失败会阻止 API 初始启动 |
| `mysql-grants` | 不发布且 `network_mode: none` | 修改 `growthos_app` 授权；不修改业务行 | 正常为退出 0；失败会阻止 API 初始启动 |
| `mysql` | 不发布 | `growthos_mysql_data` | API `/health` 可继续 200，`/ready` 应 503 |
| `redis` | 不发布 | 无；`/data` tmpfs | 缓存读写有界失败并回源 MySQL；探针不应变化 |

网络：`edge` 只连接 Web/API；`data` 只连接 API/Migrate/MySQL；`cache` 是 Docker internal 网络，只连接 API/Redis。`mysql-grants` 不连接任何网络，只通过只读 `growthos_mysql_socket` 访问 MySQL。Web、Migrate、MySQL、mysql-grants 不得读取 Redis Secret；不要为了临时调试把 service 永久加入不需要的网络。

## 4. 主机前置检查

### 4.1 必需工具

```bash
docker version
docker compose version
make --version
openssl version
curl --version
jq --version
go version
```

- Docker/Compose 用于配置、构建与运行；
- OpenSSL 用于本地随机 Secret；
- curl、jq 用于 smoke；
- Go 用于 `make verify` 和 `cmd/healthload`；
- 完整前端/Go 编译主要在 Docker build 中完成，但仓库质量门禁仍使用本机 Go、Node/pnpm 工具链。

本节最终验收环境为 Docker Engine/Desktop 29.7.2、Compose 5.4.0；这只是记录，不是宣称所有更早版本一定不兼容。若本机 Compose 不支持长语法 dependency condition、`--wait` 或当前 tmpfs 选项，应升级 Compose，不要静默删掉约束。

### 4.2 记录现有资源，不修改

```bash
docker ps --all --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
docker volume ls
```

这一步的目的是识别用户已有资源和端口，不是寻找可以删除的对象。特别是已有 `mysql:8.4` 容器使用 3306 不会与本项目冲突，因为 GrowthOS MySQL 不发布宿主机端口。

### 4.3 检查 Web 端口

macOS：

```bash
lsof -nP -iTCP:8088 -sTCP:LISTEN
```

无输出表示当前没有监听者。如果端口已占用，不停止不属于 GrowthOS 的进程；改用明确端口：

```bash
make compose-up GROWTHOS_COMPOSE_WEB_PORT=18088
make compose-smoke GROWTHOS_COMPOSE_WEB_PORT=18088
```

同一轮命令必须使用同一端口覆盖。浏览器入口也相应改为 `http://127.0.0.1:18088`。

### 4.4 确认目标 project

默认命令：

```text
COMPOSE_PROJECT=growthos
COMPOSE_FILE=deploy/compose/compose.yaml
```

如为独立实验改 project 名，后续所有 Make/Compose 命令和资源核对必须使用相同值。Secret 目录仍是仓库内同一目录，不能因为换 project 就假设会自动生成独立凭据。

## 5. Secret 初始创建与状态判断

### 5.1 正常创建/验证

```bash
make compose-secrets
```

生成器维护以下完整集合：

```text
deploy/compose/secrets/mysql_root_password
deploy/compose/secrets/mysql_app_password
deploy/compose/secrets/mysql_migration_password
deploy/compose/secrets/redis_password
```

只检查名称和权限，不读取内容：

```bash
ls -ld deploy/compose/secrets
ls -l deploy/compose/secrets
```

预期目录权限 `0700`、四个 Secret 文件 `0444`。文件 `0444` 是 Docker Desktop file secret 对非 root container 的兼容要求；宿主机可达边界依赖上层目录 `0700`。不要为了看起来“更安全”直接改成 `0400`，那可能让容器内 root-owned bind mount 无法被非 root 进程读取。

### 5.2 生成器的四种状态

| 文件状态 | MySQL volume | 结果 | 正确处理 |
| --- | --- | --- | --- |
| 0/4 | 不存在 | 在私有临时目录生成并验证完整集合，再逐文件发布 | 正常首次启动；若发布中断形成部分集合，下次运行会拒绝继续 |
| 4/4 | 任意 | 只验证格式/权限，不覆盖 | 正常复用 |
| 1～3/4 | 任意 | 失败 | 恢复缺失原文件；不要随机补齐 |
| 0/4 | `${project}_mysql_data` 已存在 | 失败 | 恢复原集合，或执行经过授权的完整数据/凭据重置 |

生成器只接受可读普通文件和 64 位小写十六进制内容。失败输出可以记录变量/文件名和规则，但不能把文件内容粘贴到工单。

### 5.3 禁止绕过 volume guard

脚本内部有测试专用跳过开关，用于任务临时目录验证。真实 project 不得设置它来“修好”已有 volume + 缺 Secret 的现场。随机新密码与旧 MySQL 账号不一致，绕过只会把可诊断的早期失败推迟为 MySQL/API 认证失败。

### 5.4 密码轮换不是编辑文件

保留 MySQL volume 时，轮换顺序至少需要：

1. 通过受控管理员身份更新 MySQL 对应账号；
2. 通过支持版本切换与回滚的 Secret 来源发布完整、匹配的新集合；
3. 按服务重建/重启并验证两个账号权限；
4. 安全撤销旧凭据；
5. 更新审计和恢复材料。

本节没有实现这套自动轮换。不要只改 `mysql_app_password` 文件并期待 MySQL 官方入口再次执行初始化脚本。

## 6. 配置和镜像预检

### 6.1 Compose 模型

```bash
make compose-config
```

该目标先创建/验证 Secret，再执行 `docker compose ... config --quiet`。它验证语法和模型，不启动容器，也不证明镜像可构建或服务可用。

如需查看展开模型用于排查，应输出到受控终端并再次确认没有人为将密码写入 environment；正常流程不需要把展开结果提交到文档。

### 6.2 仓库质量门禁

```bash
make verify
```

包括 Go format/vet/test、文档检查、前端 test/typecheck/build。Compose 运行成功不能替代源码质量门禁。

### 6.3 构建镜像

```bash
make compose-build
```

预期本地 image：

```text
growthos/api:lesson-28
growthos/migrate:lesson-28
growthos/web:lesson-22
growthos/redis:7.4.11-lesson-24
```

构建使用固定 toolchain/tag、Go module verify 和 pnpm frozen lockfile。不要在失败时改成 `latest` 或取消 lockfile 检查；先定位 registry、网络、架构或依赖问题。

## 7. 正常启动

```bash
make compose-up
```

该目标执行 config，然后 `up --detach --build --wait --wait-timeout 180`。预期启动因果链：

```text
MySQL healthy (Migrator identity authenticated SELECT 1)
  -> Migration reaches clean latest 5 and exits 0
  -> mysql-grants reconciles exact app allowlist and exits 0
  -> API starts and /health becomes healthy

Redis starts independently; API does not wait for Redis health
```

`web` 不等待 API，`redis` 不被 API 依赖。看到创建顺序不同不等于错误，判断依据是最终状态与契约。

### 7.1 读取状态

```bash
make compose-ps
```

必要时查看包含一次性/失败容器的完整状态：

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml ps --all
```

预期：

| service | 状态 |
| --- | --- |
| `mysql` | running, healthy |
| `api` | running, healthy |
| `redis` | running, healthy |
| `web` | running, healthy |
| `migrate` | exited, code 0 |
| `mysql-grants` | exited, code 0 |

不要因 `migrate` 或 `mysql-grants` 显示 exited 就认为它崩溃；两个 one-shot 成功退出正是设计状态。前者执行到 latest 5，后者只经 Unix socket 撤销旧应用授权、建立旧两表 `SELECT` allowlist，并确保 graph 三表不可访问；两者职责不可互换。

### 7.2 手工只读检查

```bash
curl --silent --show-error --include http://127.0.0.1:8088/health
curl --silent --show-error --include http://127.0.0.1:8088/ready
curl --silent --show-error --include http://127.0.0.1:8088/container-health
```

预期 `/health=200`、`/ready=200`、`/container-health=204`，三者都有单一 `X-Request-ID`。前两个 JSON 仍来自 Go，最后一个来自 Nginx。

访问系统状态页：

```text
http://127.0.0.1:8088/system/status
```

## 8. 标准冒烟与完整验收

### 8.1 Smoke

```bash
make compose-smoke
```

脚本对 MySQL 与业务事实保持只读；为证明 Redis ACL，会在专用 StrategyID=0 key 上执行一次 `SET ... EX 30` / `GETRANGE` / 精确 `DEL`，并在退出路径清理该探针。检查包括：

- Web/API/MySQL/Redis 四个常驻服务 running + healthy；
- Migration 与 mysql-grants 两个 one-shot 均 exited 0；
- Migrator 身份读取到 `schema_migrations version=5, dirty=0`；
- 两张表存在预期的 `*_name_basic` 约束，不残留旧约束名；
- 应用身份的 `SHOW GRANTS` 精确等于 USAGE + 两张表 `SELECT`，`@@GLOBAL.mandatory_roles` 为空；
- graph header/node/edge 三张表已经由 `000003` / `000004` / `000005` 创建；
- 应用身份能读旧两张业务表；其 INSERT、UPDATE、DELETE、`schema_migrations` 访问，以及对 graph 三表的 SELECT/INSERT 均被拒绝，smoke 的负向语句不改变数据；
- API 只在内部 `cache` 网络消费 Redis Secret；其余服务没有缓存网络/Secret；
- Redis 默认用户关闭，`growthos_api` 可执行无 key 的 `PING`，并只可对缓存前缀执行 `GETRANGE/SET/DEL`；普通 `GET`、前缀外 `SET`、`KEYS`、`SCAN`、`FLUSHALL`、`CONFIG`、`ACL`、`PUBLISH`、`SUBSCRIBE` 均被拒绝；
- Redis 精确启用 `48mb`、`allkeys-lru`、无 RDB/AOF 持久化；
- `/health`、`/ready` 为 200 JSON；
- SPA `/` 为 200；
- 未知 `/api/...` 为 Go `route_not_found` 404 JSON；未播种、动态推导的 StrategyID 访问 ephemeral route 为 `lottery_strategy_not_found` 404；
- 404 header/body request ID 一致；
- API/MySQL/Redis/Migrate/mysql-grants 没有 published port；
- Web 只有配置的 `127.0.0.1:<port>`。

脚本用任务专用 `mktemp` 保存响应，并在退出时删除，不会留下 body/header 文件。

### 8.2 第 24 节隔离 Lottery/cache acceptance

长期 `growthos` project 的 smoke 故意不写业务 fixture；要验证真实成功选择、缓存命中/回源、依赖故障与网关恢复，使用一次性环境：

```bash
make compose-lottery-api-acceptance
```

该脚本创建随机 Compose project、任务专用 Secret 目录、MySQL data/socket volumes 和 acceptance image tags；Migrator 应用到 latest 5 并写入隔离旧表 fixture，随后把 runtime app 收敛为旧两表 SELECT-only、graph 三表 denied。它会核对：

- `reward`、`no_reward` 与 MaxUint64 identity 的最小 decimal-string DTO；
- invalid ID/demo header/query/body/idempotency、方法与尾斜杠错误；
- 非法 Host 是带 no-store/request ID 的 server-level 421，但不是 JSON envelope；
- 经 API location 识别的空 chunked 与非空 `Trailer` 声明是 JSON 400；不受支持或非法 Transfer-Encoding 可能被 Nginx HTTP parser 更早以非 JSON 501/400 拒绝，不能声称所有 framing 错误都统一 JSON；
- `HEAD` 在语义上匹配 405 与 `Allow: POST`，但 HEAD 的 wire response 没有 body；
- 16 KiB + 1 的已知长度请求由 edge 返回 JSON 413；
- 多 Award 批次**总计 64 个请求，最大并行度 16**，只返回配置内结果；这不是 64 个同时请求、64 RPS、定速负载或生产容量；
- 缓存 value 的严格 v1 JSON 与完整 uint64 decimal string；
- poison value 被精确删除、从 MySQL 重载并修复；not-found 不做 negative cache；
- Redis ACL 的命令/key/channel 边界、默认用户关闭、48 MiB `allkeys-lru` 与无持久化配置；
- Redis 停止时 cold request 回源 MySQL且 API `/health`、`/ready` 不受影响；Redis 恢复后无需重启 API即可重新填充；
- MySQL 停止时 warm cache hit 仍可选择，`/ready` 与 cold miss 按既有 unavailable 语义失败；MySQL 恢复后无需重启 API即可回源并填充；
- 调用前后两张 Lottery 业务表的内容 fingerprint 不变；这只说明该用例没有 Lottery 业务状态写路径，不排除访问日志、连接统计等技术副作用；
- API stop 时 502/504 的 JSON、no-store 与 request ID 保持关联，恢复后重新通过检查。
- graph 三表存在且 `growthos_app` 的 SELECT/INSERT 均被 MySQL 拒绝；隔离 cache/API 回归没有把尚未装配的 graph Repository 偷偷接入运行链。

2 MiB sentinel、1000 Award、最多 10% TTL jitter、同一 cold key 合并和不同 key fill 不串行，分别由 Strategy cache 的 unit/race 测试证明，不属于上述 Compose 脚本已直接探测的场景；证据分层与未实测边界见[第 24 节 QA](../qa/lessons/lesson-24.md)。

脚本退出时只清理由 label、Compose project 与精确 ID 共同证明归属本次任务的容器、网络、volumes、临时 Secret/响应和无其他引用的 acceptance images；不会删除长期 `growthos` volumes、Secret、用户容器或可复用依赖。若 cleanup 报错，先按脚本输出解析精确 project/label，不使用全局 prune。

该脚本还运行三组 10 秒、50 RPS、最大 16 workers 的 M1 本地基线：warm-cache、cache disabled/direct-MySQL、Redis down。它同时读取 Performance Schema 的 prepared statement execute 计数和低基数缓存事件，证明 warm hit 没有权威读取、直接回源每请求执行两条 SELECT、Redis 故障时仍然回源。结果只属于本机当次开发证据，不是容量、SLO 或生产性能对比；精确数据与解释见[第 24 节 QA](../qa/lessons/lesson-24.md)。

### 8.3 第 28 节 disposable MySQL 8.4.11 acceptance

若只需验证源码 Migration、两类 Repository 和身份隔离，不应为此写入长期 Compose volume；运行：

```bash
make lesson28-mysql-acceptance
```

脚本启动随机 name/label、回环动态端口、`/var/lib/mysql` tmpfs 的一次性 `mysql:8.4.11`，在任务 `mktemp` 目录生成 root/Migrator/legacy writer/graph writer Secret。Migrator 应用 latest 5；legacy writer 只获旧两表 `SELECT, INSERT`，graph writer 只获新三表 `SELECT, INSERT`，长期 runtime 能力不会参与或被扩宽。随后顺序运行 MySQL platform、Migration、schema 与两个 Repository 的六组 Integration。

第 28 节实跑已 exit 0：六组全部通过；脚本按精确 container ID + name + label 停止 `--rm` 容器，删除明确的任务 Secret 文件和空目录，并确认没有本次 label 残留，长期 `growthos` containers/volumes/networks 快照前后相同。若 `mysql:8.4.11` 镜像是本次首次下载，它作为可复用依赖保留。不要手工按名称前缀删除容器，不要删除长期 volume/Secret，也不要用全局 prune“补清理”。

### 8.4 代码 + Compose 验证

```bash
make compose-verify
```

它运行完整仓库 `make verify`、Compose up 和 smoke，但**不包含** 5 分钟 M0。用于日常回归时不能被误记成已执行完整负载门禁。

### 8.5 固定 M0

```bash
make compose-m0
```

至少预留 6 分钟并避免同时运行其他重负载。该 target 内部固定：

```text
/health 100 RPS × 5m，32 workers，2s timeout，P99 <= 100ms
/ready   20 RPS × 30s，32 workers，2s timeout
```

固定 recipe 不接受用辅助变量缩短门禁。`cmd/healthload` 输出单行 JSON；应记录原始输出、Docker/Compose 版本、主机条件和退出码，不记录响应 body 或 Secret。

第 16 节已经记录的 M0 基线（第 28 节没有重跑这组 5 分钟负载）：

| 目标 | scheduled/completed/success | errors/unexpected/dropped | 延迟 |
| --- | --- | --- | --- |
| `/health` | `30000/30000/30000` | `0/0/0` | P50 `1.084208ms`、P95 `2.744875ms`、P99 `4.1495ms`、max `18.116291ms`、实际 `100.0027 RPS`；100ms 门槛通过 |
| `/ready` | `600/600/600` | `0/0/0` | P50 `4.08525ms`、P95 `5.935083ms`、P99 `6.841375ms`、max `8.570541ms`、实际 `20.0276841 RPS` |

第 16 节 32 workers readiness 复测与当时 smoke 后的资源快照：Web/API/MySQL/Redis 约 `5.535/6.664/438/23.41 MiB`，Docker 配额 `1.924 GiB`。第 28 节的 v5 smoke/acceptance 不会把这组旧瞬时值更新成新容量结论；使用以下命令可以获取同类证据：

```bash
docker stats --no-stream
```

不要把全局 `docker stats` 中其他用户容器混入 GrowthOS 报告；按 Compose `ps -q` 解析准确容器后再归属数据。单次 `--no-stream` 是瞬时快照，不是峰值或 leak 证明。

### 8.6 可调辅助负载

开发时可单独执行：

```bash
make compose-load-health HEALTHLOAD_RATE=10 HEALTHLOAD_DURATION=10s
make compose-load-ready READYLOAD_RATE=5 READYLOAD_DURATION=10s
```

这些命令用于快速定位，不替代固定 M0。

## 9. 日志与隐私检查

### 9.1 安全查看

```bash
make compose-logs
```

按 service 缩小范围：

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml logs --tail=200 web
docker compose --project-name growthos --file deploy/compose/compose.yaml logs --tail=200 api
docker compose --project-name growthos --file deploy/compose/compose.yaml logs --tail=200 mysql
```

Web access log 允许规范化 path、request ID、status、upstream status、bytes、request/upstream time；因此 ephemeral route 的实际 StrategyID 会出现在 path 中。它不应出现 query string 或 Referer，但“没有 query”不等于“没有业务标识”，仍需控制日志访问和保留期。Nginx error log 只保留 critical，避免默认请求级错误拼入原始 target/Referer。Go 日志不打印 DSN、密码、SQL、driver raw cause 或内部 Secret path；MySQL driver 使用 `NopLogger`。

### 9.2 request ID 关联

正常 Go 响应：response header、Go error body（如有）、Nginx access log 使用同一个 API request ID。

API-down gateway：Nginx 返回一个自身 request ID，response header 与 Nginx access log 一致；没有伪造 Go error body。

如果一个响应出现两个 `X-Request-ID` 或 Go error body/header 不一致，停止验收并检查 `proxy_hide_header`、Nginx `map/add_header` 和 Go middleware。

### 9.3 敏感 marker 验收

允许在本地、无真实 Secret 的 query/Referer 中注入一次唯一假 marker，然后检索**本 project**日志，验证 marker 不存在。不得使用真实 token、手机号、邮箱或密码作为 marker，也不得把 marker 测试发到共享环境。

最终验收的 query/referrer marker 未在 Web/API/MySQL/Redis/Migration/mysql-grants 的 Compose 日志中出现。这个结果只覆盖当前链路；增加业务参数、新中间件或 Nginx 模块后必须重跑。

## 10. Migration 操作

状态：

```bash
make compose-status
```

前向执行：

```bash
make compose-migrate
```

当前源码 Migration latest 为 5：`000001` / `000002` 创建 Strategy/Award，`000003` / `000004` / `000005` 创建 routing graph/node/edge。`make compose-status` 应报告 `clean` 且 `version=latest=5`；从旧 volume 前向执行 `make compose-migrate` 可为 `applied`，重复执行应为 `no_change`。该目标随后运行 `mysql-grants`，因此成功条件还包括长期应用授权被重新收敛。迁移前后都要记录真实 status；不能只根据源码或镜像 tag 宣布现有 volume 已是 v5。

如只需在已经完成 Migration 的当前栈重新核对/收敛授权，可执行：

```bash
make compose-grants
```

授权作业只经 `growthos_mysql_socket`，没有 TCP 或容器网络；它先 `REVOKE` 应用身份的旧权限，再只授予 `lottery_strategy` / `lottery_strategy_award` 的 `SELECT`，精确比较 `SHOW GRANTS`，要求 `@@GLOBAL.mandatory_roles` 为空，并确认 routing graph/node/edge 的 SELECT/INSERT 都被拒绝。任何多余权限、mandatory role 或 socket/Secret 错误都会非零退出。不要为“兼容”已有额外角色而放宽脚本；legacy/graph Repository 的隔离测试写身份都不是给长期 runtime 恢复 INSERT 或 graph SELECT 的理由。

遵循 [MySQL Migration 运维手册](mysql-migrations.md)：先 status、审批/备份/影子库演练，再 up，成功后再次 status 和授权核对。产品命令不提供 down/drop/force，不能用数据库版本表手工编辑绕过 dirty。

## 11. 故障演练

所有命令只针对默认 `growthos` project。若使用覆盖名，先替换每条命令中的 project，并核对 label。

### 11.1 MySQL 停止：验证 liveness/readiness 分离

停止本项目 MySQL：

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml stop mysql
```

检查：

```bash
curl --silent --show-error --include http://127.0.0.1:8088/health
curl --silent --show-error --include http://127.0.0.1:8088/ready
make compose-ps
```

预期：

- `/health=200`；
- `/ready=503`，合法 `dependency_unavailable` JSON，header/body ID 一致；
- API container 仍 running/healthy；
- Web 仍 healthy；
- API 不被自动重启。

恢复 MySQL：

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml up --detach --wait --wait-timeout 180 mysql
```

等待 MySQL healthy 后重新请求 `/ready`。预期恢复 200，API 容器 ID/启动时间不变。若必须重启 API，保存状态与日志并调查连接池/网络，不把重启写成正常步骤。

### 11.2 API 停止：验证 Web 独立和动态 DNS

先记录 GrowthOS API/Web 容器 ID：

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml ps --quiet api
docker compose --project-name growthos --file deploy/compose/compose.yaml ps --quiet web
```

停止 API：

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml stop api
```

预期：

- `/` 和 `/system/status` 静态页面仍可访问；
- `/container-health=204`；
- `/health`、`/ready` 为 gateway 502/504，而不是伪造的 Go JSON；
- gateway response 有一个 Nginx `X-Request-ID`，与 access log 一致；
- Web 容器保持 healthy、ID 不变。

重建/启动 API：

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml up --detach --wait --wait-timeout 180 api
```

预期 Nginx 通过 Docker DNS 解析当前 API 地址，不重启 Web 即恢复 `/health`、`/ready`。

### 11.3 Redis 停止：验证可选缓存 fail-open

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml stop redis
curl --silent --show-error --include http://127.0.0.1:8088/health
curl --silent --show-error --include http://127.0.0.1:8088/ready
```

预期两个 Go 探针继续 200，API/Web/MySQL 不重启；对已有 fixture 的 Lottery 请求会在有界缓存读失败后回源 MySQL，业务结果不因 Redis 错误改变。长期 smoke 没有写 fixture，不能仅靠探针证明回源；完整 warm/cold/恢复路径必须使用第 24 节隔离 acceptance。恢复：

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml up --detach --wait --wait-timeout 180 redis
```

Redis `/data` 为 tmpfs，重建或移除容器后数据丢失是本节设计。不要用测试数据残留判断持久化能力。

Redis healthy 后无需重启 API；下一次请求会惰性重连并重新填充。若恢复后业务只能靠重启 API，保留日志并检查 client pool 的坏连接驱逐/重连，不把重启写成标准恢复步骤。不要执行 `FLUSHALL` 验证恢复，也不要停止用户自己的 Redis。

### 11.4 Web 停止

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml stop web
```

预期 loopback 入口连接失败，但 API/MySQL/Redis 不因 Web 停止而退出。恢复 Web 后重新运行 smoke。

### 11.5 Migration 失败

不要修改已共享 Migration 或对真实 volume 执行破坏性 SQL 制造失败。使用任务专用临时 project/schema 或单元/集成测试注入失败，验证 `migrate` 非零时 API 初始启动被阻断。真实失败现场按 `make compose-status`、Migration 日志和专门 Runbook 处理。

### 11.6 授权收敛失败

不要通过给 `growthos_app` 增加 schema wildcard 权限、让 API 使用 Migrator 密码、把 `mysql-grants` 加入 data 网络或删除 mandatory-role 检查来恢复。先查看 one-shot 日志和受控管理员侧的有效授权；确认是旧权限、角色策略、socket、root Secret 还是目标表未创建。修复环境后单独执行 `make compose-grants`，再运行 smoke；只有授权作业成功退出，API 初始启动门才算满足。

### 11.7 演练收尾

每次演练后：

```bash
make compose-up
make compose-smoke
```

确保四个常驻服务恢复、Migration/mysql-grants 均退出 0、latest 5、旧两表 SELECT-only 与 graph 三表 denied 均成立、唯一端口边界不变，再决定是否执行 M0。不要让“已恢复”只基于首页一次 200。

## 12. 常见故障排查

### 12.1 `only part of the Compose secret set exists`

原因：四个文件只有 1～3 个存在。

处理：

1. 停止继续生成；
2. 从受控备份恢复同一批缺失文件；
3. 核对 `growthos_mysql_data` 是否已初始化；
4. 四个文件齐全后重新 `make compose-secrets`；
5. 无法恢复时，先决定是否允许丢弃整个 GrowthOS 数据，再走精确 reset。

不要随机补一个文件，不要复制其他项目密码，不要关闭检查。

### 12.2 `Docker volume growthos_mysql_data already exists while its secret set is missing`

原因：数据库账号状态仍在 volume，但宿主机凭据集合丢失。

处理优先级：恢复原 Secret > 从受控凭据管理恢复/轮换账号 > 明确授权的完整数据重置。不能仅重新生成随机值。

### 12.3 Web 端口绑定失败

先用 `lsof` 确认占用者。若不是 GrowthOS，不停止它；选择新 loopback 端口，并在 up/smoke/load/browser 全部使用相同 `GROWTHOS_COMPOSE_WEB_PORT`。

### 12.4 MySQL 一直 unhealthy

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml ps --all
docker compose --project-name growthos --file deploy/compose/compose.yaml logs --tail=200 mysql
```

检查：

- volume 是首次初始化还是复用；
- Secret 集合是否与该 volume 同批；
- init 脚本是否成功创建两个账号；
- 磁盘/内存是否足够；
- health 使用的 Migrator 账号是否能对目标 schema 执行认证 `SELECT 1`；
- 是否有人只编辑 Secret 文件但未轮换 MySQL 账号。

不要改成匿名 TCP ping 来让 health 变绿；这会掩盖真正的身份/权限失败。

### 12.5 Migration 非零退出，API 未创建/未启动

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml ps --all
docker compose --project-name growthos --file deploy/compose/compose.yaml logs --tail=200 migrate
make compose-status
```

`service_completed_successfully` 正在按设计阻断后续授权与 API。检查稳定 stage、dirty/version（当前构建必须 clean latest 5）、账号权限和 timeout；不要临时删除 `depends_on`，不要让 API 使用 Migrator 密码。

### 12.6 mysql-grants 非零退出，API 未创建/未启动

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml ps --all
docker compose --project-name growthos --file deploy/compose/compose.yaml logs --tail=200 mysql-grants
make compose-status
```

先确认 Migration 已 clean latest 5，再由受控管理员核查 root Secret、Unix socket、`SHOW GRANTS FOR 'growthos_app'@'%'` 与 `@@GLOBAL.mandatory_roles`。脚本要求 mandatory role 为空，最终授权精确等于 USAGE + 旧两张 Lottery 表 `SELECT`，并要求 graph 三表不可访问；任意额外角色/权限都会故意失败。不要把脚本切换到 TCP、授予 schema wildcard、恢复 INSERT/graph SELECT、改用 Migrator 身份启动 API 或删掉校验。

### 12.7 API 启动失败

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml logs --tail=200 api
docker compose --project-name growthos --file deploy/compose/compose.yaml ps --all
```

常见边界：

- `_FILE` 不可读、为空、过大或与直接变量冲突；
- app Secret 与 MySQL volume 不匹配；
- Migration 或 mysql-grants 没成功；
- MySQL 未 healthy；
- HTTP/MySQL timeout 配置非法；
- Strategy 缓存启用但 Redis Secret 缺失/冲突，Redis地址、TLS、pool 或缓存预算非法；

日志故意没有 driver raw cause。需要进一步诊断时使用受控 MySQL 管理工具和 `SHOW GRANTS`，不要放宽应用日志打印 DSN/密码。

### 12.8 Web healthy 但 `/health` 为 502

这通常表示 Nginx 正常、API 不可连接。检查 API service 状态/日志和 edge 网络，不要重启 MySQL/Redis 作为第一反应。API 恢复/recreate 后等待 Docker DNS 重新解析，Web 不应需要 restart。

502 的 `X-Request-ID` 应能在 Web access log 找到；它不是 Go request ID，也不会出现在 Go 日志。

### 12.9 Redis unhealthy

检查 Redis Secret 格式、ACL/config 临时目录、`/data` tmpfs 所有权和 Redis 日志。API 已是唯一业务消费者并加入 internal `cache` 网络，但 Redis 仍不是启动/readiness authority；不要添加 `depends_on: redis: service_healthy` 或把 Redis 放进 `/ready` 来“修复”依赖。先确认 MySQL 正常时请求能在有界失败后回源，再按 [Redis Strategy 缓存运维手册](redis-strategy-cache.md)检查 ACL、poison value 和重连。不要放宽到 `+@all`、通配 key 或默认用户，也不要用 `FLUSHALL` 清理单个 Strategy。

### 12.10 read-only filesystem / permission denied

先判断进程是否尝试写入设计外路径。允许写入只有明确 tmpfs/volume：应用 `/tmp`，Redis `/tmp/growthos-redis` 与 `/data`，MySQL `/var/lib/mysql` 和 socket 目录。`mysql-grants` 的 `/var/lib/mysql` 是仓库空目录的只读 bind，用来遮蔽官方镜像匿名 volume；它只读共享 socket，不应写数据目录。不要用 `privileged: true`、root user 或取消 read-only 做永久修复；若出现真实必要写路径，评估最小 tmpfs/volume 与容量后修改架构。

### 12.11 M0 出现 dropped/error/unexpected status

1. 保存单行 JSON 和退出码；
2. 查看 Web/API status 与安全日志；
3. 检查同时运行的宿主机负载和 Docker 配额；
4. 区分 transport error、HTTP status、worker drop 和 P99 threshold；
5. 先复现固定参数，再用可调 target 缩小问题；
6. 不提高 timeout/workers/P99 threshold 让测试变绿，除非有新的测量和 ADR。

## 13. 正常停止与可恢复清理

### 13.1 停止并保留数据

```bash
make compose-down
```

它移除当前 project 容器和网络，保留：

- `growthos_mysql_data`；
- `growthos_mysql_socket`（仅运行期 socket 载体，可随下次启动复用；不是业务备份）；
- `deploy/compose/secrets` 下四个本机 Secret；
- 构建镜像和可复用 dependency cache。

这些不是任务临时垃圾：volume 与 Secret 必须匹配，镜像/cache 可用于下次构建。不要为了“清理彻底”删除它们。

确认 GrowthOS loopback 已释放：

```bash
lsof -nP -iTCP:8088 -sTCP:LISTEN
```

确认 project 常驻容器已移除：

```bash
docker compose --project-name growthos --file deploy/compose/compose.yaml ps --all
```

脚本创建的 smoke 临时目录由 trap 自动删除；如果进程被主机强制终止，可只清理由本次运行确认创建、名称匹配 `growthos-compose-smoke.*` 的临时目录，绝不能删除整个 `/tmp`。

### 13.2 破坏性数据 reset 前置核对

只有用户明确允许删除 GrowthOS 本地 MySQL 数据时才能继续。先解析目标：

```bash
docker volume ls --filter label=com.docker.compose.project=growthos
docker volume inspect growthos_mysql_data
docker volume inspect growthos_mysql_socket
```

必须同时确认 label：

```text
com.docker.compose.project=growthos
com.docker.compose.volume=mysql_data
```

socket volume 还必须匹配 `com.docker.compose.volume=mysql_socket`。如果任一名称/label 不匹配、data volume 包含需保留数据、或无法确认归属，停止。不要删除名为 `mysql`、`mysql_data`、`mysql_socket` 的其他 volume，不要依赖视觉相似名称。

### 13.3 显式 reset

完成备份/授权后：

```bash
make compose-reset CONFIRM=reset-growthos-data
```

该目标对当前 Compose project 执行 down + volumes + orphans，会永久删除 GrowthOS `mysql_data`，并删除可重建的 `mysql_socket` volume；数据库事实默认不可恢复。Secret 文件不会被该命令删除，下一次 up 会用原集合重新初始化账号。

如果目标还包括生成全新身份，必须在确认 volume 已删除、无数据恢复需求后，再由操作者精确处理这四个 Secret 文件并重新运行生成器。这是独立破坏性决定，不能把删除 Secret 当作 `compose-reset` 的隐式步骤。

### 13.4 永远不要执行的替代命令

```text
docker system prune ...
docker volume prune ...
docker container prune ...
docker rm -f mysql redis ...
rm -rf deploy/compose/secrets
```

这些目标要么过宽，要么会让凭据/volume 错配。即使 Docker Desktop 显示资源“未使用”，也不代表用户不再需要。

## 14. 交接记录

一次完整验收应记录：

- Git commit/branch；第 16 节实现提交为 `_FILE` `e746a6f`、driver 日志边界 `52c3add`、Compose 栈与验收工具 `7aa6c9e`；
- Docker Engine、Compose、Go 版本和主机架构；
- Compose project/file、Web loopback 端口；
- 六个 service 最终状态：四个常驻 healthy，Migration 与 mysql-grants 两个 one-shot exit code 0；
- `schema_migrations` clean latest 5、五张 Lottery 表存在、运行应用精确旧两表 `SELECT`、graph 三表 denied 和 mandatory role 为空；
- smoke 输出；
- 两段 healthload 单行 JSON、退出码；
- 资源快照及其“瞬时非峰值”限制；
- MySQL/Redis/API 故障演练前后状态；
- 端口隔离、非 root/read-only/capdrop 证据；
- 正常/502 的单一 request ID 关联证据；
- query/Referer 假 marker 未出现在 project 日志的证据；
- 停止后保留/删除了哪些明确资源；
- 确认用户已有容器、volume、数据库、镜像未被修改或删除。

不得记录 Secret 值、DSN、完整数据库原始错误、个人数据、真实 token 或内部凭据路径。

## 15. 生产迁移提醒

不要把本 Runbook 中的以下开发选择复制到生产：

- MySQL TLS disabled；
- 用户 host `%`；
- loopback HTTP + 本地 Nginx；
- 宿主机 file Secret 和手工恢复；
- 单 MySQL 数据 volume + 单独 socket IPC volume，无备份/HA；
- 依赖 root Secret、共享本机 Unix socket 和 `mandatory_roles` 为空的授权作业；
- Redis 不持久、固定 48 MiB 且使用 `allkeys-lru`；这只是开发缓存策略，不是生产容量测算、备份或 HA；
- Docker bridge 内置 DNS；
- `restart: no`、无资源 limit/调度；
- 本地 M0 探针负载；
- 无认证、rate limit、集中可观测性和告警。

生产部署必须基于独立环境规格和 ADR，重新决定 Secret manager、Redis/MySQL TLS、身份与 ACL、网络策略、probe 暴露、资源、备份恢复、滚动发布、可观测性、缓存容量/淘汰与失效模型。staging/production 启用 Strategy 缓存时配置会强制 Redis `verify_identity`；但通过配置校验本身不等于证书、HA 或容量已经验收。

## 16. 官方参考

- [Docker Compose 启停顺序](https://docs.docker.com/compose/how-tos/startup-order/)
- [Docker Compose Secret](https://docs.docker.com/compose/how-tos/use-secrets/)
- [Compose service 属性](https://docs.docker.com/reference/compose-file/services/)
- [Compose volume](https://docs.docker.com/reference/compose-file/volumes/)
- [`docker compose down`](https://docs.docker.com/reference/cli/docker/compose/down/)
- [Docker bridge 网络](https://docs.docker.com/engine/network/drivers/bridge/)
- [MySQL 官方镜像首次初始化](https://hub.docker.com/_/mysql)
- [Redis 官方镜像](https://hub.docker.com/_/redis)
