# 第 16 节第一性原理设计手记：把“我的电脑能跑”变成可证伪的开发环境

> 本文记录第 16 节“Docker Compose 开发环境”背后的架构推导。它不是 Compose 语法教程、操作手册、面试答案或验收报告。本文中的“选择”“声明”“设计为”描述设计意图；镜像是否成功构建、容器是否真实健康、故障是否按预期恢复、浏览器是否正确降级、M0 负载结果如何，必须以[第 16 节 QA](../../qa/lessons/lesson-16.md)中的真实命令、时间和结果为准。尤其不能从本文的目标反推出尚未执行的压测数值。

## 1. 决策命题：开发环境首先是一份可执行的系统边界

表面需求是“加入 Docker Compose”。

最短实现可能只有一个 `compose.yaml`：

```yaml
services:
  api:
    build: .
  mysql:
    image: mysql:latest
  redis:
    image: redis:latest
```

它可以让几个容器出现在 Docker Desktop 中，却没有回答真正决定项目可信度的问题：

- 哪个容器是浏览器入口，哪些端口会暴露到宿主机或局域网？
- Web、API、迁移、数据库和缓存的职责是否被分开？
- “容器 started”是否被误写成“依赖 ready”？
- 数据库在启动之后故障，谁负责发现、恢复和向用户表达？
- API 为什么拥有或不拥有 DDL 权限？
- 密码怎样进入进程，错误信息会不会泄漏文件路径或 Secret 内容？
- Secret 丢失后重新生成，为什么可能让已有 MySQL Volume 永久无法登录？
- Nginx 在 API 不可用时能否仍提供静态故障页面？
- Redis 尚未被业务使用时，为什么要启动；又为什么不能把它算进 readiness？
- 镜像是在当前 Apple Silicon 上原生运行，还是被无意强制成 amd64 模拟？
- `restart`、`depends_on`、healthcheck 和应用重连各自控制哪个生命周期？
- 一次 `/health` 200 能证明什么，又明确不能证明什么？
- 本地环境与生产环境之间，哪些契约可以携带，哪些能力必须重新设计？

因此，本节真正的决策命题是：

> 在不污染开发者已有 Docker 数据、不虚构尚未实现的基础设施依赖、也不把单机 Compose 冒充生产平台的前提下，怎样把 GrowthOS 的 Web、Go API、一次性迁移、MySQL 和预留 Redis 组织成一套可重复创建、可局部失败、可诊断、可安全停止、可用证据推翻其假设的开发环境？

这里的“可复现”不是“我执行一次成功”。

它至少包含：

1. 相同源码和受控版本能够得到相同职责划分；
2. 服务通过稳定名字而不是偶然 IP 互相发现；
3. 启动前置条件由语义检查表达，而不是用 `sleep` 猜时间；
4. Secret、Volume、Network 和 Host Port 都有明确所有者；
5. 失败能停在正确层，不被无限重启或假绿色掩盖；
6. 正常路径和故障路径都能被自动化及人工证据复核；
7. 清理只影响当前 Compose Project，不触碰用户已有容器与数据；
8. 文档不把本地成功外推成生产可用性或业务吞吐能力。

### 1.1 本节要交付的能力

- 一份带项目作用域的 Compose 拓扑；
- `web`、`api`、`migrate`、`mysql`、`redis` 五个职责不同的服务；
- `edge`、内部 `data`、内部 `cache` 三张网络；
- 只有 Web 在宿主机回环地址发布一个入口端口；
- Web 的 Node 构建阶段与 Nginx 运行阶段；
- Go 的构建阶段与 API、Migration 两个运行 target；
- MySQL 8.4 开发实例、版本化 Migration 和命名数据卷；
- API 与 Migration 两套独立最小权限身份；
- Compose file-backed secrets 与 Go `_FILE` 配置边界；
- MySQL 真实认证健康检查、API liveness、Web 自身健康检查；
- Redis 的密码、内部网络与明确的非持久化语义；
- 面向故障注入的 smoke、恢复和 M0 验证入口；
- 保留真实结论的课程、ADR、QA、面试与设计思考文档。

### 1.2 本节明确不交付的能力

- 不交付 Kubernetes、Swarm、Nomad 或跨主机调度；
- 不交付生产高可用 MySQL、主从复制、备份恢复或 PITR；
- 不交付托管 Redis、Redis Sentinel、Cluster 或持久化恢复；
- 不交付 PostgreSQL、RabbitMQ、RocketMQ 的业务接入；
- 不交付 Redis 缓存命中率、一致性、失效或防穿透策略；
- 不交付 TLS 终止、正式域名、证书轮换、WAF 或公网访问；
- 不交付生产 Secret Manager、KMS、Vault 或自动密码轮换；
- 不交付镜像 Registry、签名、SBOM、漏洞门禁或多架构 Manifest；
- 不交付自动扩缩容、滚动发布、蓝绿发布或流量治理；
- 不交付业务 API 吞吐、数据库容量或端到端 SLA 结论；
- 不交付“所有页面业务都已真实化”；第 15 节之外的 Mock 边界仍然存在；
- 不因为本机已经安装某项中间件，就把它写进系统依赖图。

## 2. 为什么这个功能值得做：它减少的不是敲命令，而是歧义

如果只从“少敲几条命令”理解 Compose，就会低估这节的架构价值。

开发环境的主要成本通常不是第一次启动，而是这些歧义：

- A 同学连接本机 `127.0.0.1:3306`，B 同学连接另一个容器；
- 有人先跑 Migration，有人让 API 用高权限账号自动建表；
- 有人把密码写进 shell history，有人把密码复制进 `.env` 并误提交；
- 有人认为 Redis 停止就应该让 API readiness 失败，尽管代码根本没使用 Redis；
- 有人从 Vite dev server 访问，有人从静态产物访问，代理行为不同；
- 一个开发者已有 MySQL 占用 3306，另一个没有，于是同一文档产生不同结果；
- 故障后有人重启 API，有人重建数据库，最终都得到绿色，却不知道哪个恢复机制真正生效；
- 为“清理环境”执行 `down -v`，把尚未备份的数据当缓存删除。

Compose 的价值是把这些隐含选择提升为可审阅的系统关系。

### 2.1 对开发者的价值

- 降低首次启动的认知分支；
- 让端口、路径、账号、网络、卷和启动条件有统一事实源；
- 让失败可以由服务状态、退出码、探针和日志分层定位；
- 让“关闭环境”和“删除数据”成为两个不同动作；
- 让宿主机已安装的同名服务不再是隐形依赖；
- 让开发者能够复现别人的故障，而不只是复现别人的成功。

### 2.2 对架构的价值

- 第一次把运行时信任边界显式画出来；
- 第一次验证第 13 节的数据库身份与迁移边界能跨进程成立；
- 第一次验证第 15 节的同源调用契约能从 Vite 迁移到静态 Nginx；
- 为后续业务、缓存、消息、CI 和部署章节提供可替换的开发基线；
- 暴露“应用契约可以复用，但本地编排机制不能直接冒充生产”的边界。

### 2.3 对学习者和面试叙述的价值

一份可信的项目经历不能只说“使用 Docker Compose 部署 MySQL 和 Redis”。

更有价值的叙述是：

- 为什么划分五个服务；
- 为什么 Web 不等待 API；
- 为什么 API healthcheck 不打 `/ready`；
- 为什么 Redis 启动了却不接入 API；
- 为什么只有 Web 发布回环端口；
- 为什么 Migration 与 API 使用不同账号；
- 为什么 Secret 文件权限不是简单写成 `0600`；
- 为什么已有 Volume 时禁止自动补生成密码；
- 为什么 M0 `/health` 负载不能写成业务 QPS。

这些问题反映的是边界推理，而不是 YAML 记忆。

## 3. 事实、假设与约束先分开

架构推导最容易犯的错误，是把“我希望如此”“通常如此”和“本轮已经观察到”混成同一句话。

### 3.1 本轮作为设计输入的真实发现

| 发现 | 它影响的决定 | 它不能证明什么 |
| --- | --- | --- |
| 本机 Docker Engine/Client 版本检查到 29.7.2 | 可以使用当前 Docker Desktop 提供的 Compose 与 BuildKit 能力 | 其他开发机版本完全相同 |
| Compose 插件检查到 v5.4.0 | 长语法 `depends_on` 等应以当前 Compose 规范和实测为准 | 旧版 Compose 二进制兼容全部字段 |
| Docker Server 为 Linux arm64 | 优先使用有 arm64 变体的官方镜像，避免无必要模拟 | amd64 发布已经验证 |
| 本机已有用户 MySQL 容器并占用宿主机 3306 | 项目 MySQL 不应发布 3306；项目资源必须隔离命名 | 项目应该复用该 MySQL |
| 本机还存在 Redis、PostgreSQL、RabbitMQ、RocketMQ 等容器或镜像 | 清理与启动不能影响这些用户资产 | GrowthOS 已经需要这些组件 |
| 本地 MySQL 实际版本曾检查为 8.4.11 | Compose 可固定到同一 8.4 patch 进行本轮开发 | 未来永远使用此 patch |
| 本地 Redis 实际版本曾检查为 7.4.11 | 可在课程环境固定同一版本，降低差异 | 业务已经采用 Redis |
| 浏览器入口需求只需要本机访问 | Web 发布 `127.0.0.1:8088`，不默认监听全部网卡 | 回环绑定等于认证或 TLS |

这些是设计输入，不是最终 QA 结果。

例如“镜像 tag 存在”不等于“本仓库 Dockerfile 构建成功”；“本机是 arm64”不等于“所有 image layer 都在原生架构运行”；“配置声明 healthcheck”不等于“故障注入已经符合预期”。

### 3.2 当前可依赖的项目事实

- Go API 已有 `/health` 与 `/ready`，且二者语义不同；
- `/health` 不访问 MySQL；
- `/ready` 使用有界 `PingContext` 检查 MySQL；
- API 和 Migration 配置、账号及超时已经分离；
- Migration 有独立命令与版本化迁移文件；
- Web 已通过同源 `/health`、`/ready` 路径消费后端；
- 前端能够区分 API 存活、依赖未就绪和网关不可达；
- 当前 Go 依赖中没有 Redis 客户端；
- PostgreSQL、RabbitMQ 和 RocketMQ 也没有领域需求或代码接入；
- 仓库已有 Makefile 作为稳定开发入口；
- 主分支与课程分支要保留可学习的提交边界。

### 3.3 当前假设

- 单开发机、单 Compose Project 足以覆盖本节目标；
- 本地浏览器只需从 loopback 访问，不需要局域网设备联调；
- MySQL 是当前唯一业务持久化依赖；
- Redis 是后续章节可能使用的环境能力，本节只验证其隔离与安全启动；
- Docker Desktop 可提供内部 DNS、bind mount secrets 和 named volume；
- Alpine 运行镜像提供当前健康检查与调试所需的最小工具；
- `8088` 在目标开发机可用；若冲突，可通过受控变量覆盖；
- 当前开发数据可持久保留，但没有生产级备份承诺；
- 一次性 Migration 在单机启动时序中可以串行执行；
- 本节的 M0 仅用于发现明显回归，不用于容量规划。

每个假设都需要观察信号和复核时机，后文会进入风险账本。

### 3.4 不可跨越的约束

1. 不修改或复用用户已有 MySQL、Redis、PostgreSQL、RabbitMQ、RocketMQ 容器；
2. 不默认删除任何用户 Volume；
3. 不把密码、Secret 路径内容或 DSN 写入日志和 QA；
4. 不让浏览器直接访问数据库、Redis 或 API 宿主机端口；
5. 不让 API 使用 root 或 Migration 账号；
6. 不让 Redis 的存在虚构业务缓存能力；
7. 不用 `sleep N` 代替健康语义；
8. 不把 `depends_on` 描述成运行期自愈；
9. 不把单次 Compose 验收描述成生产 HA；
10. 不在设计文档中预填尚未测得的延迟或吞吐数值。

## 4. 第一性原则推导框架

在选工具前，先问系统要守住哪些基本量。

### 4.1 事实源

每个结论必须有唯一或明确优先级的事实源：

| 结论 | 当前事实源 | 不应成为事实源的东西 |
| --- | --- | --- |
| 服务拓扑 | `compose.yaml` | Docker Desktop 截图、口头约定 |
| 构建输入 | Dockerfile、lockfile、`go.sum`、构建参数 | 开发者本机已安装 SDK |
| 浏览器入口 | Web loopback 端口映射与 Nginx 路由 | API 临时宿主机端口 |
| Schema 版本 | 版本化 Migration 记录 | MySQL init 目录里的随手 SQL |
| API 数据权限 | `growthos_app` grant 与配置 | “网络隔离所以没关系” |
| Migration 权限 | `growthos_migrator` grant 与独立 Secret | API root 密码 |
| liveness | API `/health` | MySQL 是否可查询 |
| readiness | API `/ready` 的当前 MySQL Ping | 容器是否 running |
| Web 存活 | `/container-health` | API 当前是否健康 |
| Redis 是否为业务依赖 | 代码、配置、网络和失败语义共同接入 | Compose 中出现一个 Redis 服务 |
| 性能结果 | 可复跑工具的原始报告与环境元数据 | “页面感觉很快” |

### 4.2 故障域

系统组件分开，不是为了让拓扑图更复杂，而是为了让故障可以独立表达：

- Web 静态服务可能正常，而 API 不可达；
- API 进程可能正常，而 MySQL 不可用；
- MySQL 可能正常，而 Migration 失败；
- Redis 可能不可用，而当前 API 不受影响；
- Secret 文件可能存在，但与持久卷中账号不匹配；
- 容器可能 healthy，但浏览器路径因 Nginx 路由错误而失败；
- 所有功能可能正确，但宿主机端口被其他项目占用；
- 服务可能恢复，但进程被不必要重启，掩盖了连接池恢复能力。

只要两个状态需要不同恢复动作，就不应被压成同一个布尔值。

### 4.3 最小权限

权限从能力需求反推，而不是从“本地环境方便”反推：

- 浏览器只需要 HTTP，不需要看见 API 容器端口；
- Web 只需要连接 API，不需要进入数据网络；
- API 只需要业务 DML，不需要 DDL；
- Migration 需要 schema 变更，不需要进入 edge 网络；
- Redis 当前没有消费者，不需要与 API 联网；
- 每个服务只挂载自己需要的 Secret；
- 运行进程不需要 root、额外 capabilities 或任意根文件系统写权限；
- 宿主机只需要一个回环入口，不需要公开数据库和缓存。

### 4.4 生命周期

必须分别建模：

1. Image 的构建和升级；
2. Container 的创建、健康、退出和重建；
3. Migration Job 的成功或失败终态；
4. Named Volume 的独立持久生命周期；
5. Secret 文件与数据库账号的身份生命周期；
6. Network 的创建和销毁；
7. 应用连接池的断连与重连；
8. 日志文件的轮转与保留；
9. 浏览器静态页面和上游 API 的独立可用性；
10. 开发环境与生产环境的不同治理周期。

大量 Compose 事故都来自把这些生命周期误认为“`up` 创建、`down` 删除”的同一件事。

### 4.5 可证伪性

一个设计只有写清什么现象会证明它错误，才不是自我确认。

本节至少要能被以下测试推翻：

- 停止 MySQL 后 `/health` 也失败，说明 liveness 耦合错误；
- 重启 MySQL 后必须重启 API 才恢复，说明运行期恢复假设不成立；
- 停止 API 后连 SPA 都打不开，说明 Web 生命周期被错误绑定；
- 停止 Redis 后 `/ready` 失败，说明尚未使用的依赖被虚构；
- API 能执行 `CREATE TABLE`，说明最小权限失败；
- 只有 API 正常时 Web 才能创建，说明故障页面不可用；
- MySQL 或 Redis 出现在宿主机公开端口，说明暴露面扩大；
- Secret 内容出现在 `docker compose config`、日志或错误，说明脱敏边界失败；
- 修改一份 Secret 后旧 Volume 仍被文档描述为可直接登录，说明生命周期建模错误；
- 负载工具丢请求却仍退出 0，说明 M0 证据不可信。

### 4.6 可逆性

开发环境应该允许低成本撤销容器和网络，同时保护有状态数据：

- 普通 `compose-down` 停止并删除当前项目容器、网络；
- 默认保留 MySQL named volume；
- 删除 Volume 是独立、明确、破坏性的决定；
- 本地 Secret 不进入 Git，但也不能在 Volume 存在时随意重建；
- Redis 数据是明确可丢弃的 tmpfs，不制造隐形持久状态；
- Image 可以重建，但构建版本和依赖边界需可追踪；
- 故障注入结束后只恢复本项目服务，不影响用户其他容器。

## 5. 从请求、启动与数据三张图推导拓扑

只看一个容器框图容易把“谁调用谁”“谁等谁”“谁保存什么”混在一起。

### 5.1 请求链路

```text
Browser
  │ http://127.0.0.1:8088
  ▼
web / Nginx
  ├── /, /assets/*        → static files
  ├── /health, /ready     → api:8080
  └── /api, /api/*        → api:8080
                               │
                               ▼
                          mysql:3306
```

请求图回答：运行时流量经过哪里、哪个节点需要同时加入两张网络、浏览器能直接看到什么。

### 5.2 启动链路

```text
mysql container starts
  │
  ├── init fresh volume: create scoped users
  │
  └── growthos_app authenticates and SELECT 1 succeeds
            │ service_healthy
            ▼
        migrate runs once
            │ exit code 0 / service_completed_successfully
            ▼
          api starts

web starts independently
redis starts independently
```

启动图回答：首次创建时哪些前置条件必须成立。

它不回答 MySQL 在第十分钟故障时 API 会怎样；那是运行期恢复模型。

### 5.3 数据与状态生命周期

```text
mysql image/container ── writes ──> mysql_data named volume
secret files          ── bootstrap/authenticate ──> MySQL accounts
migrations            ── versioned schema changes ──> growthos database

redis container       ── writes only ──> bounded /data tmpfs
web/api containers    ── write only ──> bounded runtime tmpfs
images                ── immutable source ──> read-only root filesystems
```

数据图回答：重建哪个资源会保留什么、删除哪个资源会损失什么、Secret 和账号为什么必须同步。

### 5.4 五个服务的职责契约

| 服务 | 唯一职责 | 允许依赖 | 成功语义 | 明确不负责 |
| --- | --- | --- | --- | --- |
| `web` | 提供静态 SPA 并同源代理 HTTP | `edge` 网络、请求时可发现 API | 自身静态入口可响应 | 判断 MySQL、迁移或 Redis 健康 |
| `api` | 运行 Go HTTP 服务与业务边界 | `edge`、`data`、应用 MySQL 身份 | `/health` 响应；`/ready` 单独表达 DB 状态 | 执行 DDL、管理 Redis、提供静态构建 |
| `migrate` | 串行应用版本化 schema 变更 | `data`、迁移 MySQL 身份 | 一次运行成功退出 0 | 常驻、接收流量、执行业务 DML |
| `mysql` | 保存当前业务持久化数据与账号 | 内部 `data`、named volume | 应用账号认证并完成最小查询 | 对宿主机发布端口、承担应用逻辑 |
| `redis` | 提供隔离的后续缓存实验环境 | 内部 `cache`、自身 Secret | 认证 PING 成功 | 成为 API readiness、保存当前业务事实 |

### 5.5 为什么不是一个全能容器

把 Nginx、API、Migration、MySQL、Redis 装进一个容器，会减少服务数量，却会破坏：

- 独立构建和升级；
- 独立健康语义；
- 最小 Linux 权限；
- 最小网络成员关系；
- 数据 Volume 生命周期；
- Migration 的一次性退出状态；
- 故障注入和恢复定位；
- 日志来源区分；
- 将来替换托管数据库或缓存的可逆性。

“一个容器一个进程”不是绝对教条，但当前这些职责具有不同生命周期，因此分离有实质证据。

## 6. 为什么不复用本机已经运行的共享服务

用户本机已经有 MySQL、Redis、PostgreSQL、RabbitMQ 等容器。

这降低了下载镜像的成本，却不能自动成为项目架构。

### 6.1 共享本机 MySQL 的隐形变量

复用已有 `mysql` 容器会让项目依赖以下仓库外事实：

- 容器名字；
- 宿主机端口；
- root 密码；
- 已创建的数据库与用户；
- auth plugin 与字符集；
- MySQL patch 版本；
- 已执行过哪些手工 SQL；
- 当前其他项目的负载；
- 是否允许清理或重启；
- 数据丢失影响谁。

这些事实无法由 Git 分支完整复现。

### 6.2 隔离并不是浪费一份数据库

本地多启动一个 MySQL 容器确实增加：

- 镜像磁盘占用；
- 运行时内存；
- 首次初始化时间；
- named volume 空间；
- 开发者理解两个实例的成本。

但它换来：

- 独立账号和 grant；
- 独立 schema 与 Migration 历史；
- 可以安全停止/故障注入；
- 不占用宿主机 3306；
- `compose-down` 不触碰用户服务；
- 新开发者不需要复制某台机器的手工状态；
- QA 可以说明精确版本和资源作用域。

在求职学习项目里，可解释性和可复跑性比节省一个开发容器更重要。

### 6.3 什么时候可以选择共享服务

并非任何情况下都必须自建 Compose MySQL。

以下条件同时具备时，共享开发数据库可能合理：

- 团队有正式管理的开发实例；
- 账号、数据库和数据隔离由平台保证；
- schema migration 有唯一执行方；
- 访问凭据由 Secret 管理系统分发；
- 故障注入不会影响其他团队；
- 延迟和可用性差异是开发目标的一部分；
- 离线开发不是必要能力。

当前本机随手启动的个人容器不满足这些治理条件。

### 6.4 对用户已有资产的保护规则

本节的所有操作都必须以 Compose Project 名称和仓库路径为作用域。

禁止用以下粗暴方式“确保环境干净”：

- 删除全部停止容器；
- prune 全局 Volume；
- prune 全局 Image；
- 抢占或停止宿主机 3306 上的用户 MySQL；
- 重命名用户已有容器；
- 读取、打印或复用其他项目凭据；
- 用模糊名称删除网络或 Volume；
- 将用户已有 Redis 作为故障注入目标。

任何清理都先解析本项目精确资源，再执行可恢复或最小删除。

## 7. 为什么不因为“已经安装”就加入 PostgreSQL、RabbitMQ 和 RocketMQ

基础设施选择必须由系统需要解决的问题推导，而不是由 Docker Desktop 列表推导。

### 7.1 候选组件比较

| 组件 | 它擅长解决什么 | GrowthOS 当前证据 | 现在加入的成本 | 本节决定 |
| --- | --- | --- | --- | --- |
| MySQL | 当前关系数据、事务、Migration | 已有代码、账号、连接池、探针与迁移 | 已被第 13 节接受 | 接入 |
| Redis | 低延迟临时状态、缓存、限流等 | 后续可能使用，但当前无客户端/业务语义 | 密码、网络、内存、故障语义 | 只准备隔离环境，不接业务 |
| PostgreSQL | 另一套关系数据库能力 | 没有必须由 PG 解决的查询或扩展需求 | 双 ORM/SQL、Migration、运维、语义差异 | 不加入 |
| RabbitMQ | AMQP 消息路由与可靠投递 | 当前没有异步业务、交换机或消费模型 | Broker、队列拓扑、重试、DLQ、监控 | 不加入 |
| RocketMQ | 大规模消息与特定生态能力 | 当前没有吞吐、顺序或事务消息需求 | NameServer/Broker、资源、语义和运维 | 不加入 |

### 7.2 “技术丰富”不是架构收益

每加入一个组件，至少需要回答：

- 谁生产，谁消费？
- 数据是不是事实源？
- 丢失、重复、乱序怎样处理？
- 失败是否影响 readiness？
- 是否需要持久化、备份、恢复？
- 如何认证、授权、轮换 Secret？
- 如何设置资源上限和观察指标？
- 如何做本地、CI、生产一致性？
- 如何升级和迁移？
- 如果删掉它，产品哪项指标会恶化？

如果这些问题没有答案，组件只是简历关键词，不是系统能力。

### 7.3 重新评估的触发器

PostgreSQL 只有在出现具体能力差距时重评，例如：

- 必须使用当前 MySQL 不适合的扩展或查询；
- 数据团队已有明确 PostgreSQL 平台约束；
- 迁移收益超过双数据库认知与运维成本。

消息系统只有在出现真实异步边界时重评，例如：

- 请求链路无法承担的耗时工作；
- 可量化的削峰填谷需求；
- 需要发布/订阅解耦多个消费者；
- 能明确接受至少一次投递并实现幂等；
- 能定义重试、死信和人工补偿。

组件的安装状态永远不是上述触发器。

## 8. Web 与 Nginx：同源入口是故障边界，不只是静态托管

### 8.1 为什么 Compose 中不用 Vite dev server

Vite dev server 的职责是开发期 HMR 和源码转换。

Compose 这一节要验证的是可构建静态产物和运行时入口，因此选择：

```text
Node builder
  └── pnpm install --frozen-lockfile
  └── pnpm run build
            │
            ▼
Nginx runtime
  └── only dist + nginx.conf
```

这样可以证明：

- 前端 production build 能独立生成；
- 最终容器不需要 Node、pnpm store 或源码；
- SPA fallback 和 API proxy 在一个明确运行时实现；
- Vite 代理只是宿主机开发适配器，不是生产网关承诺。

宿主机仍可运行 `pnpm dev` 获得 HMR；Compose 不必替代每一种开发工作流。

### 8.2 为什么坚持浏览器同源路径

浏览器始终访问：

- `/health`；
- `/ready`；
- `/api/...`；
- `/assets/...`。

它不需要知道 API 是：

- 宿主机 `127.0.0.1:8080`；
- Compose 服务 `api:8080`；
- 未来某个 Ingress upstream；
- 另一台主机或托管网关。

同源入口保留第 15 节调用契约，避免把 Docker 网络地址编译进 bundle，也避免为了本地双端口过早开放宽泛 CORS。

### 8.3 为什么 Nginx 使用 Docker DNS 动态解析

Compose 服务名稳定，但容器重建后 IP 可能改变。

若 Nginx 只在启动时解析 `api` 并永久缓存旧 IP，会出现：

1. API 首次不存在时 Nginx 启动失败；或
2. API 重建后 Nginx 继续代理旧地址；
3. 用户误以为必须重启 Web 才能恢复。

当前配置使用 Docker 内嵌 DNS resolver `127.0.0.11`，并通过变量形式的 `proxy_pass` 在请求路径重新解析稳定服务名。

这项机制需要真实故障演练验证，不能只靠配置文本宣布有效。

### 8.4 为什么 Web 不依赖 API healthy

Web 有两个职责：

1. 提供静态应用；
2. 在上游故障时展示真实降级状态。

如果 `web depends_on api: service_healthy`，则 API 最不可用时，用户连解释故障的 UI 都无法打开。

因此 Web 可以先启动。

请求 API 时如果上游不存在，Nginx 返回网关失败；第 15 节前端网络边界把它解释成 API 不可达，而不是把整个 Web 容器误判为死亡。

### 8.5 SPA fallback 与 API 路由必须先分流

静态 SPA 常用：

```nginx
try_files $uri $uri/ /index.html;
```

如果这条规则吞掉 `/api/...`、`/health` 或 `/ready`，未知 API 路径会返回 `200 text/html`。

后果不是普通 404，而是契约污染：

- 前端看到成功状态却收到 HTML；
- 错误被误分成 contract failure；
- CDN 可能缓存 SPA shell；
- Request ID 和统一错误 envelope 丢失；
- smoke test 若只看 200 可能假通过。

因此 API 与探针 location 必须先于 SPA fallback 定义，并验证未知 `/api` 路径仍由 Go 返回关联的 JSON 404。

### 8.6 缓存头继承事故为什么值得单独检查

Nginx 的 `add_header` 存在作用域继承规则：子 location 一旦声明自己的 header，父级 header 可能不再按直觉继承。

一个常见事故是：

- 顶层设置安全 header；
- `/assets/` 为缓存设置 `Cache-Control`；
- 结果 `/assets/` 丢失 `X-Content-Type-Options`、`X-Frame-Options` 等父级 header。

另一个事故是：

- `index.html` 被长期缓存；
- 新发布引用的新 hash 资源尚未同步或旧 HTML 长期存在；
- 用户持续加载不一致版本。

当前策略是：

- hash 资产可 `public, max-age=31536000, immutable`；
- `index.html` 使用 `no-store`；
- 容器自身 `/container-health` 使用 `no-store`；
- API 的 no-store 和 Request ID 应由代理透传；
- 在声明不同缓存头的 location 中重复必要安全 header。

这些必须通过真实响应头检查，而不是只读配置。

### 8.7 同源 Nginx 不提供的安全能力

它没有自动提供：

- 用户认证与授权；
- CSRF 防护；
- TLS、证书身份与 HSTS；
- rate limit、WAF 或 Bot 防护；
- 可信代理列表；
- `X-Forwarded-*` 的端到端信任策略；
- API 请求体大小的业务限制；
- 生产缓存、压缩和 CDN 一致性；
- 多实例负载均衡与熔断；
- 公网探针访问控制。

本地同源拓扑只解决本地路径和故障表达问题。

### 8.8 真实故障怎样反过来修正设计：502 也必须可关联

最初的配置关注了 Go 正常响应中的 `X-Request-ID` 透传，却遗漏了一个边界：API 完全停止时，502 由 Nginx 自己产生，没有 upstream header。

真实 API-down 演练发现：

- 静态 Web 仍可打开，符合职责分离；
- Nginx 能返回 502，符合网关故障语义；
- 但响应缺少 `X-Request-ID`；
- 用户看到的网关故障无法和 Nginx 访问日志稳定关联；
- “所有 HTTP 响应可关联”这一设计目标在最需要排障的路径上失效。

这不是补一个 header 那么简单。

若直接在 server 层 `add_header X-Request-ID $request_id always`，正常 Go 响应又可能同时保留 upstream 的同名 header，形成两个值：

```text
X-Request-ID: <Go final ID>
X-Request-ID: <Nginx generated ID>
```

客户端若看到逗号合并值或两个 header，无法判断该用哪个关联 Go 日志。

最终机制是：

1. 使用 `map` 计算统一 `$growthos_log_request_id`；
2. upstream 返回 ID 时优先使用 Go 的最终 ID；
3. 没有 upstream（静态或 502）时使用 Nginx `$request_id`；
4. 代理 location 用 `proxy_hide_header X-Request-ID` 隐藏 upstream 原始同名 header；
5. server/location 统一回写一次最终 `X-Request-ID`；
6. access log 与响应使用同一个映射值。

这使三类响应都只有一个关联 ID：

| 响应来源 | 最终 ID 来源 | 能关联到哪里 |
| --- | --- | --- |
| Go 正常/错误响应 | upstream Go ID | Go access/error log 与 Nginx access log |
| Nginx 502/504 | Nginx `$request_id` | Nginx access log |
| 静态资源/SPA | Nginx `$request_id` | Nginx access log |

演练还暴露了第二个隐患：Nginx stock error log 的请求级 upstream 错误可能附带原始 request target 和 Referer，而 query/referrer 中可能出现授权码、筛选条件或 PII，且该格式不方便按字段脱敏。

最终选择：

- Nginx `error_log` 只保留 `crit` 级关键启动/运行错误；
- 请求级 upstream status、耗时与关联 ID 由定制 access log 承担；
- access log 记录 `$request_method $uri $server_protocol`，不用含 query 的 `$request`；
- 不记录 Referer 与 User-Agent；
- 记录 `status`、`upstream_status`、`request_time`、`upstream_time`；
- 用唯一 query/referrer marker 对全部 Compose 日志做反向搜索，验收结果为 0 次出现。

这里存在真实权衡：把 error log 提到 `crit` 会减少 Nginx 原生 upstream cause 细节。

当前本地环境接受该代价，因为 access log 已提供路径、status、upstream status、时间与 request ID，且能通过 service/container 状态继续定位；若生产需要更细错误原因，应采用可结构化脱敏的日志模块、受控日志处理器或指标，而不是重新把原始 request target 写回公共日志。

这个案例说明架构不是先写完文档再让验证盖章：故障演练发现了设计没有覆盖的证据链，运行结果反过来改变 Nginx header 与日志策略，然后再用重复 header、502关联和 marker 搜索重新证伪。

## 9. Liveness、Readiness 与容器健康：三个控制问题不能混为一谈

### 9.1 三个问题

| 机制 | 回答的问题 | 当前检查 |
| --- | --- | --- |
| API `/health` | Go 进程能否完成最小 HTTP 响应 | 不访问 MySQL |
| API `/ready` | 当前实例能否在预算内访问必要 MySQL 依赖 | 有界 `PingContext` |
| Docker API healthcheck | 容器内 Go HTTP 进程是否活着 | 请求 `/health` |
| MySQL healthcheck | 应用身份是否能实际认证并查询目标库 | `growthos_app` 执行 `SELECT 1` |
| Web healthcheck | Nginx 是否能提供自己的最小响应 | `/container-health` |
| Redis healthcheck | Redis 是否接受当前 Secret 的认证命令 | 认证 `PING` |

### 9.2 为什么 API 容器检查 `/health`，而不是 `/ready`

假设 MySQL 短暂故障：

- Go 仍然能接受 HTTP；
- `/health` 应保持 200；
- `/ready` 应返回依赖未就绪；
- 前端应显示 API 活着但数据库不可用；
- 连接池应在依赖恢复后重新建立可用连接。

若容器 healthcheck 改打 `/ready`，数据库故障就会把 API 容器标记 unhealthy。

在某些编排环境中，这可能进一步触发重启或摘流。

重启 API 不能修复 MySQL，反而会：

- 丢失诊断上下文；
- 产生额外连接风暴；
- 让恢复时间抖动；
- 掩盖应用是否具备重连能力；
- 把依赖故障误诊成进程故障。

因此“杀进程”和“停止接收依赖型流量”必须使用不同信号。

### 9.3 MySQL healthcheck 为什么必须真实认证

只测端口打开最多证明某个进程监听 TCP。

`mysqladmin ping` 在某些认证失败情况下仍可能给出容易误解的退出语义，因此当前检查直接使用应用账号：

```sql
SELECT 1;
```

它至少覆盖：

- MySQL 进程可接受 TCP；
- `growthos_app` 密码匹配；
- 目标 database 可选择；
- 应用身份具有最小查询能力；
- MySQL 能执行一条简单语句。

它仍不能证明：

- Migration 全部兼容；
- 所有业务表存在；
- 索引正确；
- 复杂查询延迟达标；
- 数据内容正确；
- 事务隔离满足业务；
- 备份可恢复。

### 9.4 Web 健康为什么不能依赖 API

`/container-health` 是 Nginx 自己返回的最小 204。

如果 Web healthcheck 代理到 API，则：

- API 故障会让 Web 也 unhealthy；
- 故障域再次合并；
- 静态页面可用性无法独立观察；
- `compose up --wait` 的失败原因变得含混。

Web 自身健康与页面中的上游状态是两类证据。

### 9.5 Redis 为什么不进入 API readiness

当前 API 没有 Redis 客户端，也没有任何业务路径读取 Redis。

把 Redis 加进 `/ready` 会产生虚假的强依赖：

- Redis 停止导致 API 503，尽管所有真实路径不需要它；
- 值班者会为不存在的业务影响重启系统；
- 后续接入 Redis 时无法区分“原本就是依赖”与“新增依赖”；
- 面试叙述会滑向“使用 Redis 提升性能”的虚假结论。

只有实际业务用例定义缓存命中、未命中、不可用和回源策略后，Redis 才有资格影响 readiness。

### 9.6 关键真值表

| Web | API `/health` | API `/ready` | MySQL | Redis | 当前最多可以陈述 |
| --- | --- | --- | --- | --- | --- |
| 正常 | 200 | 200 | 可查询 | 正常/停止 | 当前 Web、API 和 MySQL 最小链路可用 |
| 正常 | 200 | 503 | 不可用 | 任意 | API 活着，MySQL 依赖未就绪 |
| 正常 | 网关失败 | 无法请求 | 任意 | 任意 | 静态入口可用，API 不可达 |
| 异常 | 未观察 | 未观察 | 任意 | 任意 | 浏览器入口失败，不能从外部入口推断 API |
| 正常 | 200 | 200 | 可查询 | 停止 | 当前业务路径应不受影响；Redis 仍是独立环境能力 |
| 正常 | 200 | 200 | 可查询 | unhealthy | 不能说“整个 Compose 全部健康”，但不能误报 API 不就绪 |

## 10. `depends_on`：只表达创建阶段前置条件

### 10.1 三种 condition 的语义

- `service_started`：依赖容器已启动，不代表内部服务可用；
- `service_healthy`：依赖 healthcheck 已通过；
- `service_completed_successfully`：一次性依赖进程已成功退出。

当前启动链：

```text
mysql service_healthy
    └── migrate starts

mysql service_healthy + migrate service_completed_successfully
    └── api starts
```

这比 `sleep 10` 强，因为时间不是语义。

### 10.2 它不负责运行期依赖恢复

`depends_on` 不会持续监控并自动重建依赖图。

MySQL 在 API 启动后停止时：

- API 不会因为 `depends_on` 自动停止；
- API 不会因为 `depends_on` 自动重启；
- MySQL 恢复后，API 是否恢复取决于连接池和应用实现；
- `/ready` 是观察运行期状态的边界；
- 故障注入必须记录 API 容器 ID/启动时间，证明是否发生非预期重启。

### 10.3 为什么不使用 `depends_on.restart: true`

该字段不是“依赖崩溃则消费者自动重启”的通用策略。

即使显式 Compose 操作能够传播重启，也不等于我们希望数据库每次重启都让 API 重启。

当前目标反而是验证：

- API 进程保持存活；
- readiness 正确降级；
- MySQL 恢复后连接池自动恢复；
- 不用重启消费者掩盖恢复能力。

### 10.4 为什么 Web 完全没有 API `depends_on`

Web 的静态职责不以前置 API 为条件。

它使用请求时 DNS 发现来适应上游出现、消失和重建。

这不是“忽略启动顺序”，而是承认 Web 与 API 有不同成功定义。

### 10.5 为什么本地 `restart: "no"`

本地学习环境需要把第一次失败保留下来：

- Migration 语法错误应停住并显示退出码；
- Secret 缺失应直接失败；
- 配置错误不应无限重启刷屏；
- 容器崩溃不应被瞬时恢复掩盖；
- 学习者应亲自区分进程恢复和依赖恢复。

`restart: "no"` 不代表生产也永不重启。

生产策略应按服务类型决定：

- 无状态常驻服务可由控制器重建；
- 一次性 Migration 失败通常应阻断发布；
- 永久配置错误需要告警而非无限循环；
- 有状态数据库的恢复需要数据一致性和存储策略；
- 重启次数、退避和 crash loop 必须可观察。

## 11. 独立 Migration：把高权限、失败和发布时机从 API 中拿出来

### 11.1 为什么不在 API 启动时自动 Migration

API 启动自动迁移看起来简单，但会耦合：

- 每个 API 实例与 DDL 权限；
- 进程启动与不可逆 schema 变更；
- 多实例并发与迁移锁；
- liveness 与长时间 DDL；
- 业务回滚与数据库回滚；
- HTTP 服务可用性与数据库升级窗口。

独立 `migrate` 服务提供：

- 独立凭据；
- 独立退出码；
- 独立日志；
- 独立超时与锁预算；
- 明确的 `service_completed_successfully` 前置；
- 可以单独执行 `up` 或 `status`；
- API 镜像/二进制与迁移二进制共享源码但不同入口。

### 11.2 为什么 Migration 是 Job，不是常驻服务

它的成功状态是“所有目标迁移已经完成，然后退出 0”。

若让它常驻：

- 运行中不再代表正在工作；
- healthcheck 语义模糊；
- 失败可能被 restart 策略反复重放；
- API 无法通过退出状态判断完成；
- 资源长期占用而没有价值。

### 11.3 init 脚本与版本化 Migration 的边界

MySQL 官方镜像 init 目录只在 fresh data directory 初始化阶段运行。

它适合：

- 创建应用账号；
- 创建迁移账号；
- 赋予初始 schema 级权限。

它不适合承载持续业务 schema 演进，因为已有 Volume 不会重跑这些文件。

业务表、索引和版本变化必须进入仓库的版本化 Migration。

### 11.4 Migration 成功仍不等于发布安全

一次任务退出 0 不能证明：

- 大表 DDL 没有锁住业务；
- 新旧应用版本都兼容 schema；
- 降级版本可以继续运行；
- 备份真实可恢复；
- DDL 在所有环境性能相同；
- 数据回填已完成；
- 生产复制延迟可接受。

未来滚动发布需要 expand/contract：先加兼容结构、双写/回填、切读、最后删除旧结构。

## 12. MySQL 身份与最小权限：网络隔离不能替代数据库授权

### 12.1 三类身份

| 身份 | 能力 | 使用者 | 生命周期 |
| --- | --- | --- | --- |
| MySQL root | 初始化实例与账号管理 | 官方 entrypoint/init | 仅 bootstrap/受控管理 |
| `growthos_migrator` | 当前 schema 上的必要 DDL/DML | 一次性 Migration | 发布或显式迁移 |
| `growthos_app` | `SELECT/INSERT/UPDATE/DELETE` | 常驻 API | 每个业务请求 |

API 不获得 `CREATE`、`ALTER`、`DROP` 等能力。

即使应用遭到 SQL 注入或代码路径失控，权限边界仍可缩小破坏半径。

### 12.2 为什么 Migration 账号也不使用 root

Migration 确实需要更高权限，但不必拥有：

- 修改其他 database；
- 创建全局用户；
- 修改 MySQL server 配置；
- 读取系统 schema 中不必要的信息；
- 管理复制或全局权限。

当前 grant 作用于 `growthos.*`，把 schema 变化限制在项目数据库。

### 12.3 `%` host 是否意味着暴露给互联网

MySQL 用户 host `%` 表示账号可从匹配网络来源连接，并不自动把 3306 发布到宿主机或互联网。

真实边界由以下层叠加：

- MySQL 只加入内部 `data` 网络；
- Compose 没有 host port；
- 只有 `api` 和 `migrate` 加入该网络；
- 每个进程只有自己的 Secret；
- MySQL 自身仍做账号认证和 grant。

但网络隔离不是授权替代品，未来多服务加入 `data` 网络时必须重评 host 和账号范围。

### 12.4 健康检查为什么使用应用账号

若 MySQL healthcheck 用 root：

- root 正常、应用密码错误时仍显示 healthy；
- init grant 错误无法被发现；
- API 会在启动后才暴露认证失败；
- `service_healthy` 失去对实际消费者的意义。

使用 `growthos_app` 验证最窄的真实连接路径。

### 12.5 最小权限需要可被测试推翻

验收不只应证明允许动作成功，还要证明禁止动作失败：

- 应用账号 `SELECT 1` 成功；
- 应用账号常规 DML 符合预期；
- 应用账号执行 DDL 被 MySQL 拒绝；
- Migration 账号能应用版本化 schema；
- API 容器没有迁移 Secret；
- Migration 容器没有应用 Secret；
- Web 与 Redis 容器都没有 MySQL Secret。

“配置看起来分开”不是权限证据。

## 13. Secret 设计：真正困难的是生命周期一致性

### 13.1 威胁模型

本地 Secret 设计至少防止：

- 密码进入 Git；
- 密码进入 Docker build context 或 image layer；
- 密码出现在 `compose.yaml`；
- 密码出现在普通环境变量 dump；
- API 获得 root 或 Migration 密码；
- Redis 获得 MySQL 密码；
- 错误消息回显 Secret 内容或敏感文件路径；
- 部分 Secret 丢失后自动生成不匹配集合；
- 已有 MySQL Volume 被新密码“锁死”；
- 调试时通过完整 `docker inspect` 把敏感元数据贴到公共渠道。

它不能防止：

- 拥有宿主机当前用户权限的人读取本地文件；
- 拥有 Docker daemon 权限的人检查容器挂载或进程；
- 已入侵容器且有权读其挂载 Secret 的攻击者；
- Secret 在应用内存中的必要明文生命周期；
- 屏幕录制、剪贴板或人为复制；
- 生产级审计、KMS 或自动轮换需求。

### 13.2 为什么不把密码直接写进环境变量

环境变量简单，且当前 Go 原本支持直接密码配置。

但在 Compose 中，直接变量更容易出现在：

- YAML；
- shell history；
- `.env`；
- process/container inspect；
- crash report；
- 调试脚本输出。

file-backed secret 让 Compose 只把文件挂载到明确服务的 `/run/secrets/...`，Go 通过 `_FILE` 读取。

这不是“文件天然安全”，而是缩小分发和误输出通道。

### 13.3 为什么直接值与 `_FILE` 必须互斥

若同时允许：

```text
GROWTHOS_MYSQL_PASSWORD=old
GROWTHOS_MYSQL_PASSWORD_FILE=/run/secrets/new
```

却不定义优先级，会产生环境相关行为：

- 本机读取直接值；
- Compose 读取文件；
- 某次部署同时存在时静默选择一个；
- 排障者不知道数据库到底收到哪份凭据。

因此 API 与 Migration 各自要求：

- 直接值和 file path 恰好存在一个；
- 同时存在直接失败；
- 都不存在直接失败；
- API 加载器不读取 Migration 密码文件；
- Migration 加载器不读取 API 密码文件。

互斥失败比隐式优先级更可诊断。

### 13.4 为什么 Secret 文件读取有大小上限

Secret 通常很小，但文件路径是配置输入。

无界 `ReadFile` 会允许：

- 误指向大文件导致内存分配；
- 指向设备或异常路径产生不可控行为；
- 错误配置扩大启动资源消耗。

当前读取使用有界 reader，只允许有限密码字节和尾部 CR/LF。

错误只说明变量不可读、为空、冲突或过长，不回显：

- 文件内容；
- 密码；
- 底层驱动 cause；
- 用户提供的敏感路径。

### 13.5 为什么只裁剪尾部 CR/LF，不使用 `TrimSpace`

密码内部或首尾空格理论上可以是合法字符。

使用 `strings.TrimSpace` 会静默改变 Secret 语义。

文件生成器输出末尾换行是常见行为，因此只去掉尾部 `\r`/`\n`，保留其他字节，并在裁剪后校验非空与长度。

### 13.6 `0700` 目录与 `0444` 文件的现实权衡

直觉上本地 Secret 文件应该是 `0600`。

但 Compose 本地 file-backed secrets 常以 root 所有的只读 bind mount 形式出现在容器中。

当 API、Migration、Redis 都使用非 root UID 时，仅 owner-readable 的宿主文件可能在容器内不可读。

当前折中是：

- Secret 目录设为 `0700`；
- 文件设为 `0444`；
- 目录只有当前宿主用户能遍历；
- 每个服务只挂载自己声明的文件；
- 容器内挂载只读；
- 文件不进入 Git 或 build context。

这个方案的代价是：一旦其他进程已经获得目录遍历能力，文件自身没有进一步按用户限制读取。

它是 Docker Desktop 开发兼容性折中，不是生产 Secret 权限模板。

未来如果 Compose/平台支持可移植的 UID/GID/mode 控制，或采用 Secret Store CSI/Vault sidecar，应重新收紧。

### 13.7 为什么四份 Secret 必须作为完整集合生成

集合包含：

- MySQL root 密码；
- MySQL API 密码；
- MySQL Migration 密码；
- Redis 密码。

只存在 1～3 个文件时自动补齐，会掩盖以下情况：

- 文件被意外删除；
- 从另一台机器只复制了部分凭据；
- MySQL 已用旧密码初始化；
- Git 分支切换改变了期望集合；
- 操作者以为“生成缺少的即可”，却制造身份分裂。

因此生成器只有两个正常入口：

1. 四份都不存在：先在私有临时目录生成并验证完整新集合，再逐文件发布；发布中断留下部分集合时，下次运行 fail closed；
2. 四份都存在：只验证格式和权限，不覆盖内容。

部分集合直接失败，并要求操作者先理解状态。

### 13.8 Secret 与 MySQL Volume 失配是本节最重要的隐藏事故

首次初始化 fresh MySQL Volume 时：

```text
secret files ──> root/app/migrator passwords stored in MySQL accounts
```

之后 MySQL 账号信息位于 `mysql_data`。

删除容器不会删除 Volume；重新生成 Secret 也不会自动修改已有账号。

危险序列是：

```text
1. MySQL Volume 仍存在
2. 本地 Secret 文件被删除
3. 脚本静默生成新密码
4. MySQL 用旧账号数据启动
5. API/Migration 用新密码连接
6. 全部认证失败
```

如果脚本为了“自愈”再自动删除 Volume，就会把认证故障升级为数据丢失。

因此生成器在 Secret 全部缺失时检查 `${compose_project}_mysql_data`：

- Volume 不存在时可以创建新集合；
- Volume 存在时拒绝生成；
- 操作者必须恢复原 Secret，或显式执行凭据轮换；
- 若确实要丢弃开发数据，也必须单独确认删除 Volume。

这个设计把“不能确定”转化为安全失败，而不是猜测。

### 13.9 为什么要缩小非原子发布窗口，并让中断后 fail closed

若逐个直接写最终路径，进程在第二个文件后中断，就会留下部分集合。

当前思路是：

1. 在目标目录创建私有临时目录；
2. 在临时目录生成四份随机值；
3. 对每份执行格式与长度验证；
4. 全部通过后逐个移动到最终名称；
5. 通过 trap 清理未完成的临时目录。

严格意义上，跨四个文件的多个 `mv` 不是文件系统级单事务。

但结合“部分集合下次必失败”可以保证中断不会被当作正常完成。

### 13.10 Secret 轮换不是改文件

正确轮换至少需要：

1. 确认当前数据库可访问；
2. 为目标账号生成新凭据；
3. 在 MySQL 中安全更新账号密码；
4. 更新本地 Secret 文件；
5. 以受控顺序重建或通知消费者；
6. 验证新凭据成功、旧凭据失效；
7. 保留失败时的恢复路径；
8. 不在日志或命令历史泄漏密码。

生产中还需双凭据窗口、自动审计和 Secret Manager 版本化。

本节只阻止危险的自动生成，不宣称实现了完整轮换。

## 14. 多阶段镜像：把构建能力与运行能力分开

### 14.1 镜像不是“把本机目录打包”

一个可信镜像需要回答：

- 编译器在哪个阶段存在？
- 依赖从何处解析，是否由 lockfile/checksum 约束？
- 源码何时进入构建上下文？
- 最终运行层包含哪些二进制、CA、时区和诊断工具？
- 构建缓存是否只影响速度，不影响结果？
- 构建目标平台如何传递？
- version 如何写入产物？
- 容器使用哪个 UID/GID？
- 哪些文件绝不应进入任何 layer？

“Docker build 成功”只是最外层结果。

### 14.2 Go Dockerfile 的推导

当前 Go 镜像分为：

```text
golang builder
  ├── copy go.mod/go.sum
  ├── go mod download
  ├── go mod verify
  ├── copy cmd/internal/migrations
  ├── build growth-api
  └── build growth-migrate

alpine runtime base
  ├── CA certificates
  ├── timezone data
  ├── non-root growthos user
  ├── api target: growth-api
  └── migrate target: growth-migrate
```

这个划分实现：

- Go SDK 不进入最终镜像；
- 源码不进入最终镜像；
- API 和 Migration 共用相同编译输入；
- 两个最终镜像只携带自己的入口二进制；
- 运行时仍保留 CA 和时区支持；
- `CGO_ENABLED=0` 降低动态库依赖；
- `-trimpath` 减少构建机路径进入二进制；
- `-buildvcs=false` 避免 Docker build context 缺少 Git 信息时产生隐式差异；
- `-ldflags` 注入明确 lesson version；
- BuildKit cache mount 复用下载与编译缓存，但不复制进产物。

### 14.3 为什么同时构建两个 Go 二进制

一个 builder 同时构建 API 和 Migration，优点是：

- 共享依赖下载；
- 保证同一源码时间切片；
- target 选择只发生在最终 stage；
- 避免两份 Dockerfile 漂移；
- 课程读者能清楚看到相同代码库的不同进程职责。

代价是：

- 即使只构建 API target，builder 目前也编译 Migration；
- 任一二进制构建失败都会阻断两个 target；
- 更大项目中会降低单 target 构建效率。

重新拆分的触发器包括：

- 构建耗时显著；
- 两个命令依赖差异扩大；
- 独立发布节奏成为需求；
- CI 需要按变更范围选择 target；
- Migration 移到独立仓库或发布制品。

### 14.4 `CGO_ENABLED=0` 不是普遍正确

当前依赖可以生成静态 Linux 二进制，适合 Alpine 运行层和跨平台构建。

但未来出现以下需求时必须重评：

- 依赖 C 库；
- 使用基于 CGO 的 SQLite 驱动；
- 对系统 DNS resolver 行为有特殊要求；
- 使用硬件/系统加密模块；
- 需要 FIPS 认证路径；
- 性能库只有 native 实现；
- debug/profiling 依赖系统符号。

“静态更简单”不能压过真实运行依赖。

### 14.5 Alpine、distroless 与 scratch 的权衡

| 运行底座 | 优点 | 代价 | 当前判断 |
| --- | --- | --- | --- |
| Alpine | 小、包管理明确、保留 `wget`/shell 诊断能力 | musl 差异、仍有 shell/包、攻击面不为零 | 开发环境采用 |
| distroless | 运行内容更少、默认非 shell | healthcheck/排障方式需重构、调试门槛高 | 生产 hardened target 可重评 |
| scratch | 最小文件系统、静态二进制直接运行 | CA/时区/用户/探针/调试均需精确补齐 | 当前不采用 |

镜像体积不是唯一指标。

对本地课程环境，能够用容器内最小工具验证探针有教学和排障价值。

### 14.6 Web Dockerfile 的推导

Web 构建分为：

```text
Node builder
  ├── fixed Node image
  ├── Corepack + fixed pnpm
  ├── copy package.json + pnpm-lock.yaml
  ├── pnpm install --frozen-lockfile
  ├── copy web source
  └── pnpm run build

Nginx runtime
  ├── nginx.conf
  ├── dist only
  ├── USER 101:101
  └── foreground nginx
```

这样避免：

- 在运行容器安装 npm 依赖；
- 把 dev dependencies 暴露到运行环境；
- 把源码、测试和 pnpm cache 带入最终镜像；
- 用 Vite preview 冒充 production server；
- 容器启动时才编译导致结果依赖网络；
- 每次运行动态选择依赖版本。

### 14.7 为什么 `--frozen-lockfile`

构建期间 lockfile 与 manifest 不一致时应失败。

如果容器构建静默改写 lockfile：

- 本地工作树没有对应变更；
- CI 与开发机构建结果可能不同；
- cache 命中掩盖依赖变化；
- 供应链审计无法追踪真正安装版本。

冻结安装把依赖更新变成显式源码变更。

### 14.8 `.dockerignore` 是安全边界的一部分

Docker build context 不是 Git tracked files 的同义词。

没有 `.dockerignore` 时，即使 Dockerfile 没有立即 `COPY`，以下内容仍可能被发送给 builder：

- `.git` 历史；
- 本地 `.env`；
- Compose Secret 文件；
- `node_modules`；
- `dist`；
- IDE 配置；
- 日志与覆盖率产物；
- 临时测试证据；
- 其他未跟踪敏感文件。

`.dockerignore` 同时改善：

- 上下文传输时间；
- 缓存稳定性；
- 误 `COPY . .` 的破坏半径；
- Secret 进入 layer 的概率；
- 镜像 provenance 的清晰度。

但它不是万能边界：

- 规则可能被错误修改；
- build argument 仍可能泄密；
- Dockerfile 可以显式复制未排除文件；
- BuildKit cache/export 也需治理；
- 已进入历史的 Secret 不能靠 ignore 撤回。

因此 Dockerfile 仍使用尽量精确的 `COPY`。

### 14.9 精确 tag 仍不是不可变构建

本节固定：

- Go builder 版本；
- Node builder 版本；
- pnpm 版本；
- Nginx 版本；
- MySQL 8.4 patch；
- Redis 7.4 patch；
- Alpine 版本。

这比 `latest` 更可读、更稳定。

但 tag 仍可能被 registry 重新指向，且构建器、平台和依赖下载源也会影响结果。

更强的生产供应链需要：

- tag + digest；
- 多架构 manifest/digest 管理；
- SBOM；
- provenance attestation；
- 镜像签名；
- 漏洞扫描与例外期限；
- 自动依赖升级；
- 可回滚 Registry 保留策略；
- 构建器版本与源码 commit 记录。

本节只实现版本级约束，不宣称 bit-for-bit reproducibility。

## 15. 运行时加固：每一项都要知道它阻止什么

### 15.1 非 root

API/Migration 使用专用 UID/GID，Nginx 与 Redis 也以各自非 root 用户运行。

它降低：

- 应用漏洞直接获得容器内 root 的默认能力；
- 修改 root-owned 系统文件的机会；
- 与宿主 bind mount 权限错误叠加后的破坏范围；
- 某些 setuid 或特权操作的可行性。

它不能防止：

- 应用读取自己获授权的 Secret；
- 应用访问自己所在网络；
- 内核或容器运行时漏洞；
- Docker daemon 权限滥用；
- 业务层越权；
- writable mount 中的数据破坏。

### 15.2 只读根文件系统

`read_only: true` 把容器 root filesystem 设为只读。

它迫使设计者显式列出写需求：

- Nginx PID 与临时目录；
- API 或 runtime 的 `/tmp`；
- Redis runtime config 和 `/data`；
- MySQL 自己的 named volume。

如果应用突然开始写源目录或系统路径，容器应失败而不是悄悄形成不可追踪状态。

它不能保护单独挂载的 writable volume/tmpfs。

### 15.3 有界 tmpfs

只读 root 之后，必要运行写入放进 tmpfs。

每个 tmpfs 应回答：

- 哪个进程需要？
- 是否允许执行？
- 是否需要 setuid？
- 最大大小是多少？
- UID/GID 和 mode 是什么？
- 数据丢失是否可接受？

当前使用：

- 通用 `/tmp`：小容量、`noexec`、`nosuid`；
- Redis `/data`：明确容量、UID/GID、`noexec`、`nosuid`；
- Redis runtime config：位于只对其用户可见的 tmp 目录。

有界容量防止异常临时写入无限吞噬宿主机内存。

### 15.4 `cap_drop: ALL`

Linux root 与 capabilities 是不同维度。

即使进程非 root，明确丢弃 capabilities 仍能：

- 防止镜像或运行时默认赋予不需要的 kernel 能力；
- 让新增需要 capability 的行为显式失败；
- 降低容器逃逸或网络管理相关破坏面。

当前服务监听的端口都高于 1024，不需要 `NET_BIND_SERVICE`。

若未来必须增加某项 capability，应逐项说明用途，而不是删除整个 `cap_drop`。

### 15.5 `no-new-privileges`

该设置阻止进程及子进程通过 setuid/setgid 等路径获得额外权限。

它与非 root、cap drop、read-only 形成纵深防御。

任何一项都不是替代关系：

- 非 root 控制初始身份；
- cap drop 控制 Linux capability 集；
- no-new-privileges 控制后续权限提升；
- read-only 控制文件系统修改；
- 网络和 Secret 声明控制可访问资源。

### 15.6 `init: true`

容器 PID 1 有特殊信号和子进程回收语义。

一个小型 init 进程可以：

- 转发停止信号；
- 回收孤儿子进程；
- 减少僵尸进程积累。

它不能修复应用自身忽略 context 或不实现 graceful shutdown。

API 仍需：

- 捕获 SIGTERM；
- 停止接受新请求；
- 在预算内完成活动请求；
- 关闭数据库连接池；
- 超时后明确退出。

### 15.7 运行加固的反作用

加固会提高某些调试成本：

- 容器内不能随意安装包；
- 根目录无法写临时文件；
- 非 root 无法读取权限错误的 bind mount；
- cap drop 让临时网络工具能力受限；
- distroless 进一步缺少 shell。

正确做法不是在出错时整体取消加固，而是：

- 使用只读检查命令；
- 创建临时 debug container 加入同一网络；
- 明确挂载受限诊断目录；
- 在开发/生产之间设置可审计的 debug profile；
- 把真正运行写需求加入明确 tmpfs/volume。

## 16. 网络与端口：可达性本身也是权限

### 16.1 三张网络

```text
edge:  web <──> api
data:  api <──> mysql <──> migrate
cache: redis
```

`data` 与 `cache` 设置为 internal。

这不等于加密或身份认证，但它减少了默认外部路由和无关服务可达性。

### 16.2 为什么 API 是双网卡桥接点

API 同时：

- 接受 Web 的 HTTP；
- 访问 MySQL。

因此它加入 `edge` 与 `data`。

这也意味着 API 是高价值边界：一旦被攻破，攻击者可能从 edge 横向访问 data。

所以还需要：

- 数据库账号最小权限；
- 输入验证；
- 非 root 与只读运行；
- 日志脱敏；
- 未来的 egress/NetworkPolicy；
- 依赖端 TLS 或服务身份。

网络分层只是减少路径，不是零信任完成标志。

### 16.3 为什么只有 Web 发布 Host Port

宿主机需要的唯一正常入口是：

```text
127.0.0.1:8088 -> web:8080
```

API 只 `expose` 容器端口，MySQL、Redis、Migration 不发布任何端口。

好处包括：

- 不与用户已有 MySQL 3306 冲突；
- 不与用户已有 Redis 6379 冲突；
- 浏览器始终走真实 Nginx 路由；
- 不绕过同源与缓存/header 行为；
- 局域网默认不能直接访问；
- 减少误用高权限服务的入口。

### 16.4 loopback 绑定不等于认证

`127.0.0.1` 缩小网络暴露面，但本机其他进程仍可访问。

它不能替代：

- 用户认证；
- API 授权；
- TLS；
- OS 用户隔离；
- 浏览器 CSRF 防护；
- 本地恶意软件防护。

如果未来需要手机或局域网设备联调，应显式选择监听地址，并同步评估防火墙、认证与敏感数据，而不是把 host 改成 `0.0.0.0` 就结束。

### 16.5 容器内 `localhost` 的语义

每个容器有自己的网络命名空间。

因此：

- API 中 `127.0.0.1:3306` 指向 API 自己，不是 MySQL；
- API 应连接 `mysql:3306`；
- Web 应连接 `api:8080`；
- 宿主机浏览器连接 `127.0.0.1:8088`；
- 宿主机通常不能解析 Compose 服务名 `mysql`；
- `host.docker.internal` 只用于容器访问宿主机服务的特定需求，本节不需要。

把 host port、container port 和 service name 分开，是容器排障的第一步。

### 16.6 为什么 Redis 独占 `cache` 网络

当前没有任何消费者。

让 Redis 单独处在 `cache` 网络可以主动证明：

- API 尚未接入；
- Redis 停止不应影响 API；
- 未来消费者必须显式加入网络；
- 新依赖会在 Compose diff 中可见；
- 不能通过默认共享网络偷偷形成耦合。

这比把所有服务放入 default network 更诚实。

### 16.7 网络隔离验证不能只看 YAML

真实 QA 应从两个方向检查：

1. 正向：允许的服务名和端口可达；
2. 反向：不允许的服务根本没有共同网络或 host port。

还应检查：

- Compose 实际创建的 network 名称带项目作用域；
- 服务重建后 DNS 能解析新地址；
- Web 没有 data/cache；
- MySQL 没有 edge/cache；
- Redis 没有 edge/data；
- Migration 没有 edge；
- 没有意外 default network；
- `docker inspect` 的 published ports 与声明一致。

## 17. Redis：先交付诚实的环境能力，再交付业务语义

### 17.1 为什么本节仍然包含 Redis

Redis 出现在本节，不是为了宣称缓存性能收益，而是为了提前解决一组开发环境边界：

- 精确版本镜像；
- 非 root 运行；
- 文件 Secret 认证；
- 内部独立网络；
- 自身 healthcheck；
- 可丢弃数据语义；
- 资源上限；
- 故障与 API 解耦。

它为后续真正接入减少环境工作，但不提前决定缓存策略。

### 17.2 为什么自定义 Redis entrypoint

Redis 配置需要引用运行时 Secret。

把密码直接放进 command line 或静态 config 会增加泄漏风险。

entrypoint 在容器启动时：

1. 检查 `/run/secrets/redis_password` 可读；
2. 验证为 64 位小写十六进制；
3. 在私有 runtime 目录生成 ACL 文件；
4. 生成 Redis config；
5. 清除 shell 变量；
6. `exec` Redis，使其接收 PID 1 信号链。

这个临时配置位于 tmpfs，不进入 image layer。

### 17.3 为什么关闭 RDB 和 AOF

当前 Redis 没有业务数据，也没有恢复目标。

如果给它 named volume 并默认持久化，会制造：

- 未定义的数据所有权；
- 重启后陈旧缓存是否可信的问题；
- Secret 轮换与 ACL 文件生命周期；
- 数据格式升级；
- 清理时误把缓存当事实；
- 学习者误以为系统依赖 Redis 持久性。

因此显式：

- `save ""`；
- `appendonly no`；
- `/data` 使用有界 tmpfs；
- 容器销毁后数据丢失是预期行为。

### 17.4 不持久化也需要内存治理

tmpfs 限制的是文件系统写入，不等同于 Redis 进程内存上限。

未来真正缓存接入时还要决定：

- 容器 memory limit；
- Redis `maxmemory`；
- eviction policy；
- value 大小上限；
- key 数量与 TTL；
- hot key；
- OOM 行为；
- 命中率和回源压力；
- 故障时降级与熔断。

本节没有业务写入，因此暂不伪造这些参数。

### 17.5 Redis 未来接入前必须回答的问题

1. 缓存的事实源仍是 MySQL 吗？
2. Cache-aside、write-through 还是 write-behind？
3. 未命中与 Redis 故障如何区分？
4. TTL 从业务新鲜度还是经验数字推导？
5. 更新数据库后怎样失效缓存？
6. 删除失败是否允许旧值继续存在？
7. 如何防穿透、击穿和雪崩？
8. 是否允许 stale-while-revalidate？
9. key 如何包含租户、版本与环境？
10. 序列化格式如何演进？
11. Redis 故障是否影响 readiness？
12. 哪些 API 可以回源，哪些必须失败？
13. 是否需要分布式锁；锁失效与 fencing 如何处理？
14. 是否需要持久化；丢失后的恢复目标是什么？
15. 是否真的比进程内缓存或 SQL 优化更合适？

没有这些答案，不应把 API 加入 `cache` 网络。

## 18. Apple Silicon 与多架构：本机原生不等于已经跨架构发布

### 18.1 当前平台事实

本轮 Docker Server 为 Linux arm64。

因此优先选择具有 arm64 变体的官方镜像，并避免：

```yaml
platform: linux/amd64
```

无理由强制 amd64 会带来：

- QEMU 模拟开销；
- 构建与启动变慢；
- 某些指令或 native addon 差异；
- 把模拟成功误写成原生兼容；
- 本地性能证据失真。

### 18.2 Go builder 的平台参数

Dockerfile 使用 BuildKit 提供的：

- `BUILDPLATFORM` 选择 builder 运行平台；
- `TARGETOS`；
- `TARGETARCH`；
- `GOOS`/`GOARCH` 生成目标二进制。

结合 `CGO_ENABLED=0`，这为 Linux 多架构构建预留机制。

但“参数存在”不等于：

- 已执行 `buildx --platform linux/amd64,linux/arm64`；
- 已推送 manifest list；
- 已在两个原生平台运行 smoke；
- 所有 base image digest 都匹配；
- 前端 native dependency 跨架构无问题。

QA 没有记录的能力就不能写进简历。

### 18.3 真正的多架构验收

未来需要：

1. 明确目标平台集合；
2. 检查每个 base image 的 manifest；
3. 使用 buildx 构建并推送；
4. 记录每个平台 digest；
5. 在原生 amd64 与 arm64 runner 运行 smoke；
6. 检查 Go/Node native 依赖；
7. 比较功能，而不是用同一台机器 QEMU 的延迟作性能结论；
8. 建立供应链签名与 provenance；
9. 定义某平台失败时的发布策略。

### 18.4 为什么版本与架构要一起记录

“MySQL 8.4”太宽；“MySQL 8.4.11 arm64 image digest X”才是强证据。

本节课程为了可读性固定 tag，QA 应至少记录：

- Docker client/server；
- Compose 版本；
- server OS/arch；
- 实际容器 image/tag；
- 必要时 image ID/digest；
- Docker Desktop 资源配置；
- 是否使用模拟。

## 19. 重启、恢复与停止：先区分四种动作

### 19.1 四种不同机制

| 机制 | 控制对象 | 当前例子 |
| --- | --- | --- |
| 应用重连 | 进程内连接池 | MySQL 恢复后 API 重新 Ping 成功 |
| 容器重启策略 | 已退出容器 | 本地明确 `restart: "no"` |
| Compose 依赖条件 | 创建阶段顺序 | MySQL healthy → Migration → API |
| graceful stop | 正在运行的进程 | SIGTERM、shutdown timeout、stop grace |

不能用其中一个替代另一个。

### 19.2 MySQL 故障恢复的期望

故障动作：停止项目 MySQL。

期望观察：

- API 容器仍 running；
- API `/health` 仍 200；
- API `/ready` 转为 503；
- Web 仍能加载；
- 页面显示依赖未就绪；
- API 容器没有重启；
- 日志不泄漏 DSN/密码。

恢复动作：启动同一 MySQL 服务。

期望观察：

- MySQL healthcheck 恢复；
- API 不重启；
- 连接池在有限时间内重新建立连接；
- `/ready` 恢复 200；
- 数据仍来自同一 named volume。

如果必须重启 API，说明恢复模型有缺口，需要排查连接生命周期或驱动行为。

### 19.3 API 故障恢复的期望

停止 API 时：

- Web 容器应继续 healthy；
- `/` 和静态资产应继续可用；
- `/health`、`/ready` 经 Nginx 返回网关故障；
- 前端不应显示旧绿色；
- Nginx 不应因启动时固定旧 IP 而永久失效。

重新创建/启动 API 时：

- Nginx 应通过服务名重新解析；
- 无需重启 Web 即恢复代理；
- Request ID 和错误/成功 header 继续透传。

### 19.4 Redis 故障恢复的期望

停止 Redis 时：

- Redis 自身变为停止/不健康；
- API、MySQL、Migration 已完成状态不应改变；
- `/health`、`/ready` 应维持原有语义；
- Web 正常；
- 不应出现 API 对 Redis 的连接日志，因为没有业务接入。

这项反向测试证明“没有依赖”也是可验证的架构事实。

### 19.5 stop grace period 如何推导

`stop_grace_period` 必须大于或至少容纳应用内部 shutdown timeout、连接关闭与运行时开销。

如果 Docker 在应用优雅关闭完成前发送强制终止：

- 活动请求被截断；
- JSON 日志可能未刷新；
- DB 连接/事务可能异常结束；
- QA 会误以为 graceful shutdown 已验证。

当前 API 内部有明确 shutdown budget，Compose 给出独立 grace。

Web 使用 Nginx `STOPSIGNAL SIGQUIT` 以触发优雅停止；MySQL/Redis 也需要比无状态进程更宽裕的停止时间。

### 19.6 停止宽限也不是越长越好

过长会：

- 拖慢开发循环；
- 掩盖进程无法退出；
- 延迟发布/故障恢复；
- 让自动化超时层次混乱。

应通过真实活动请求、慢查询和 signal 测试确定，而不是复制模板数字。

### 19.7 为什么故障演练要记录容器身份

仅看到恢复后 200，无法证明是：

- 原 API 连接池恢复；
- API 被重启后新建连接；
- Web 被重启重新解析；
- 整个 stack 被重新 `up`；
- 请求命中了错误环境。

故障前后应记录最小非敏感证据：

- container ID；
- start time/restart count；
- health state；
- endpoint 状态；
- project name；
- 恢复操作和时间。

## 20. 日志、脱敏与可观测性：能看到故障，但不把秘密变成证据

### 20.1 为什么统一 JSON 日志

容器 stdout/stderr 是本地和未来平台最自然的日志边界。

JSON 便于：

- 按 service/level/request ID 过滤；
- 避免正则解析自由文本；
- 在 Compose logs 中区分来源；
- 未来接入集中采集；
- 自动检查特定事件。

但 JSON 不自动保证结构稳定、低基数或无敏感信息。

### 20.2 为什么关闭 MySQL 驱动默认 logger

数据库驱动的默认 logger 可能绕过应用结构化日志，直接向 stderr 输出底层连接错误。

这会造成：

- JSON 流混入自由文本；
- raw driver cause 可能暴露地址、协议或其他内部细节；
- 与 Request ID/生命周期事件无法一致关联；
- 同一错误被应用安全日志和驱动原始日志重复输出。

当前在连接配置边界安装 `NopLogger`，由应用自己记录安全的生命周期与 readiness 信号。

代价是丢失部分驱动内部诊断细节。

若未来需要更多信息，应通过受控、结构化、脱敏的 adapter 暴露，而不是重新开启全局原始 logger。

### 20.3 哪些字段可以记录

通常安全且有排障价值：

- service；
- version；
- environment；
- event；
- request ID；
- route template；
- HTTP status；
- duration；
- dependency 名称；
- 稳定错误分类；
- container/project 标签；
- Migration version；
- 启停阶段。

### 20.4 哪些字段默认不能记录

- 密码与 Secret 内容；
- 含密码 DSN；
- `/run/secrets/...` 的敏感路径细节；
- 完整 SQL 与用户业务数据；
- access token、Cookie、Authorization；
- 原始数据库 driver cause；
- 完整环境变量 dump；
- 完整 `docker inspect` 输出；
- Redis value；
- 生成脚本的随机输出。

### 20.5 Docker JSON 日志轮转

本地 json-file driver 如果无限增长，会把一次长时间调试变成磁盘事故。

当前设置单文件大小与文件数量上限，提供最小轮转。

这仍不能替代：

- 中央日志采集；
- 保留策略；
- 索引生命周期；
- PII 删除；
- 审计不可抵赖；
- 跨容器关联；
- 告警和 dashboard。

轮转上限还意味着旧日志会丢失，因此重大故障证据要及时提取安全摘要。

### 20.6 当前可观察但尚未实现的缺口

| 维度 | 当前最小能力 | 生产缺口 |
| --- | --- | --- |
| Logs | JSON stdout、Request ID、Compose logs | 集中采集、查询、保留、PII 治理 |
| Metrics | healthload 报告与容器状态 | Prometheus 指标、连接池、runtime、业务指标 |
| Traces | 单 HTTP Request ID | W3C Trace Context、跨服务 span、采样 |
| Health | 容器 healthcheck、HTTP 探针 | 外部黑盒、告警、SLO burn rate |
| Resource | Docker Desktop 人工观察 | CPU/memory limit、历史、告警 |
| Build | version tag | digest、SBOM、签名、provenance |

### 20.7 日志也会成为负载

M0 `/health` 若每次请求都产生访问日志：

```text
日志事件速率 ≈ HTTP 请求速率 × 每请求日志条数
```

在 100 RPS 下，5 分钟可能产生约 30,000 次请求对应的访问事件，具体仍由实际中间件行为决定。

这会影响：

- stdout 写入；
- Docker log driver；
- 磁盘轮转；
- CPU；
- 延迟测量。

因此 M0 测到的是“当前日志配置下的最小 HTTP 路径”，不是纯 handler 极限。

### 20.8 本轮日志脱敏的真实反向证据

日志脱敏不能只做字符串代码审查，因为泄漏可能发生在：

- Nginx stock error log；
- Nginx access log 的 `$request`；
- Referer；
- Go access log；
- MySQL driver stderr；
- Compose 进程前缀；
- 故障恢复时才执行的分支。

本轮使用不会与正常日志碰撞的 query marker 与 referrer marker，经过正常请求和 API-down 故障路径后，对全部 GrowthOS Compose logs 做精确搜索。

最终记录为：

- query marker 命中 0；
- referrer marker 命中 0；
- API 502 响应存在单一 `X-Request-ID`；
- Nginx access log 使用同一关联 ID；
- 正常 upstream 响应未产生重复 `X-Request-ID`；
- MySQL 原始 driver logger 已在配置边界关闭。

这组证据只能证明“测试 marker 在本轮覆盖路径中未出现”。

它不能证明任意未来字段、任意应用日志或任意第三方库永远不会泄漏，因此每次新增 query logging、代理模块、认证 header 或 dependency logger 都要重跑敏感 marker 检查。

## 21. 完整失败模型：从宿主机一直推演到持久状态

### 21.1 分层故障表

| 层 | 失败例子 | 外部表现 | 正确观察 | 错误恢复 |
| --- | --- | --- | --- | --- |
| Host | 8088 被占用 | Web 无法发布端口 | Compose 创建错误 | 停掉用户无关服务 |
| Docker | daemon 不可用 | 所有 Compose 命令失败 | client/server 检查 | 修改应用代码 |
| Build context | Secret 被误包含 | 潜在 image 泄漏 | `.dockerignore`/layer 审计 | 只加 `.gitignore` |
| Image | tag/platform 不兼容 | pull/build 失败 | manifest、arch、build log | 强制 amd64 后不记录 |
| Secret | 文件缺失/格式错误 | 服务启动失败 | 稳定配置错误 | 打印密码排查 |
| Secret/Volume | 新密码配旧账号 | MySQL 认证失败 | Volume+Secret 生命周期 | 自动 `down -v` |
| MySQL init | grant 错误 | MySQL可能 running，health 失败 | 应用身份 `SELECT 1` | health 用 root |
| Migration | DDL/锁/权限失败 | Job 非 0，API 不启动 | migrate log/exit code | 无限 restart |
| API config | 两种密码来源同时存在 | fail fast | 稳定互斥错误 | 静默选一个 |
| API runtime | Go panic/退出 | Nginx gateway error | API state/log | 重启全部服务 |
| MySQL runtime | 数据库停止 | health 200、ready 503 | 两探针+无重启 | 杀死 API |
| Redis runtime | Redis 停止 | 当前 API 无影响 | Redis state、API仍ready | 把 ready 绑 Redis |
| Docker DNS | API IP 改变 | Nginx持续 502 | DNS/容器重建演练 | 每次重启 Web |
| Nginx route | API 路径落入 SPA | 200 HTML/contract error | Content-Type/body/status | 只看 HTTP 200 |
| Nginx cache | index 长缓存 | 发布后旧 shell | Cache-Control | 清浏览器当方案 |
| Browser | API offline | 静态 UI 可开、状态降级 | Network+页面语义 | 隐藏错误继续显示绿 |
| Logging | driver raw cause | stderr 泄漏/非 JSON | 安全日志测试 | 将完整日志贴公共渠道 |
| Shutdown | grace 太短 | 请求被 kill | signal/exit timing | 无限制加长 grace |
| Load tool | 队列丢请求 | 实际 RPS不足 | dropped/errors/exit code | 只看成功请求延迟 |

### 21.2 失败类型与恢复所有者

| 失败类型 | 第一恢复所有者 | 为什么 |
| --- | --- | --- |
| 端口冲突 | 开发者/启动入口 | 应选择空闲受控端口，不应攻击其他项目 |
| Secret 部分缺失 | Secret 生成器 + 操作者 | 自动补齐可能破坏身份一致性 |
| MySQL 暂时停止 | MySQL + API连接池 | 重启 API 不修复根依赖 |
| Migration 语法错误 | 开发者/发布流程 | 自动重试不会改变确定性错误 |
| API crash | 容器/生产控制器 | 无状态进程可按策略重建，本地先保留证据 |
| Redis 停止 | Redis 所有者 | 当前没有业务影响，不应扩散 |
| Nginx 旧 DNS | Nginx resolver 配置 | 应支持服务重建，不要求人工重启 |
| Volume 数据损坏 | 数据恢复流程 | 不是删容器可以修复 |
| 镜像漏洞 | 供应链/升级流程 | 需要新制品和验证 |
| 日志泄密 | 应用边界与事件响应 | 需止损、轮换、清理副本和修复源头 |

### 21.3 为什么故障注入要最小化爆炸半径

每次只改变一个变量：

- 停 MySQL，不同时停 API；
- 停 API，不重建 Web；
- 停 Redis，不修改网络；
- 改路由测试时不清 Volume；
- 测 Secret 失败时用隔离临时 project/目录；
- 测端口冲突时不占用用户服务端口；
- 测 shutdown 时先确认没有重要数据写入。

多变量同时变化只会得到“系统坏了”，无法证明哪个设计生效。

### 21.4 恢复验证要包含反证

“最终恢复 200”不够。

还要确认：

- API container ID 未变化；
- Web container ID 未变化；
- MySQL Volume 未重建；
- Secret 文件未重写；
- Redis 故障没有触发 API 日志；
- 恢复没有依赖全栈 down/up；
- 旧错误状态在浏览器刷新后被真实新响应替代；
- 日志中没有密码/DSN。

## 22. 验证体系：每层证据只证明自己的范围

### 22.1 证据金字塔

```text
设计意图
    ↓
静态配置解析
    ↓
单元/配置测试
    ↓
镜像构建
    ↓
Compose 正常启动
    ↓
HTTP smoke
    ↓
权限与网络反向测试
    ↓
故障注入与恢复
    ↓
真实浏览器验收
    ↓
受控 M0 负载
```

越往下越接近运行现实，但任何一层都不能自动证明生产能力。

### 22.2 `docker compose config --quiet`

它可以发现：

- YAML 解析错误；
- 变量插值问题；
- 路径和结构错误；
- 部分 Compose schema 问题。

它不能发现：

- image 是否存在；
- Dockerfile 是否构建；
- Secret 内容是否匹配 Volume；
- healthcheck 是否有效；
- 服务能否运行；
- 网络恢复是否正确。

### 22.3 单元测试

本节相关单元测试应覆盖：

- `_FILE` 成功加载；
- CRLF 结尾；
- 直接值与文件互斥；
- 缺失、空、不可读、超长文件；
- API/Migration 不越界读取对方变量；
- 配置 String/JSON/slog 脱敏；
- MySQL driver 使用安全 logger；
- healthload 参数校验；
- healthload timeout、意外 status、请求数量、分位数；
- 输出不包含响应 body Secret。

这些测试可以证明函数边界，不证明 Docker mount 权限在当前 Desktop 上可用。

### 22.4 镜像构建

镜像构建至少证伪：

- Dockerfile 语法；
- 基础镜像 tag；
- lockfile；
- Go/前端编译；
- stage/target 名称；
- COPY 路径；
- 当前平台可构建性。

它不能证明进程启动、Secret 可读或网络可达。

### 22.5 正常启动与 smoke

自动 smoke 应检查：

- MySQL/API/Redis/Web running + healthy；
- Migration exited 0；
- `/health` 经 Web 代理返回 200 JSON；
- `/ready` 经 Web 代理返回 200 JSON；
- `/` 返回 SPA；
- 未知 `/api` 返回 Go 的 JSON 404；
- header/body Request ID 一致；
- 只有 Web 发布精确 loopback 端口；
- 临时响应文件在 trap 中清理。

这仍只覆盖正常状态，不覆盖恢复。

### 22.6 权限与隔离验证

需要额外检查：

- API DDL 被拒绝；
- Migration DDL 成功；
- 每个服务只挂载必要 Secret；
- 非 root UID 生效；
- root filesystem 只读；
- tmpfs 类型和上限生效；
- capabilities 被清空；
- no-new-privileges 生效；
- 网络成员关系符合拓扑；
- MySQL/Redis/API 无 host port；
- Docker config/log 不泄漏 Secret 内容。

### 22.7 故障注入验证

至少三组：

1. MySQL stop/start，验证 API 不重启与 readiness 恢复；
2. API stop/start/recreate，验证 Web 保持可用与 Nginx DNS 恢复；
3. Redis stop/start，验证当前 API 完全不依赖。

每组必须记录：

- 前置状态；
- 精确动作；
- 预期；
- 实际；
- 时间；
- container identity；
- 恢复；
- 清理；
- 偏差和后续修复。

### 22.8 真实浏览器验收

HTTP curl 不能证明：

- SPA 真正渲染；
- JavaScript bundle 可执行；
- 页面在网关失败时显示正确文案；
- 缓存或旧状态不会假绿；
- 窄屏、深色模式和可访问性行为正确；
- Nginx Content-Type 与资产路径满足浏览器。

浏览器至少检查：

- 正常状态；
- MySQL 降级；
- API 离线；
- 恢复后刷新；
- 宽屏与窄屏；
- light/dark；
- console error；
- accessibility scan；
- Network 中 status、Content-Type、Cache-Control、Request ID。

### 22.9 QA 是结论账本，不是宣传页

[第 16 节 QA](../../qa/lessons/lesson-16.md)应区分：

- 计划执行；
- 实际执行；
- 通过；
- 失败后修复；
- 未执行；
- 受环境限制；
- 仍有风险。

设计文档里出现的所有矩阵都只是验收需求，不能替代 QA 的真实结果。

## 23. M0 容量基线：建立测量能力，而不是制造性能故事

### 23.1 为什么选择 `/health`

M0 选择最小 `/health` 路径，可以观察：

- loopback TCP；
- Nginx proxy；
- Go HTTP server；
- middleware；
- JSON 编码；
- access logging；
- Docker Desktop 网络与调度。

它刻意不访问 MySQL。

因此它不能代表：

- `/ready` 的数据库 Ping 能力；
- 业务 SQL；
- 事务；
- JSON 大 payload；
- 认证授权；
- Redis；
- 消息系统；
- 生产 TLS/CDN/Ingress；
- 互联网用户延迟。

### 23.2 M0 的目标参数

当前基线目标定义为：

```text
target:   http://127.0.0.1:8088/health
rate:     100 scheduled requests/second
duration: 5 minutes
workers:  bounded
timeout:  per-request bounded
status:   expect 200
```

理论调度请求数为：

```text
100 × 300 = 30,000
```

真实完成数、错误数、丢弃数、actual RPS、P50/P95/P99/Max 必须由工具报告产生。

设计阶段不预填任何结果；只有运行完成后，才把原始报告中的数值作为带环境边界的证据记录到 QA 与本文的验收复盘。

### 23.3 为什么要固定速率，而不是只开 N 个并发压满

固定速率更适合作为 M0 稳态检查：

- 请求到达模型可描述；
- 不会因为服务变慢自动无限降低发送压力而掩盖问题；
- dropped 可以暴露客户端自身无法维持速率；
- 5 分钟能观察短暂抖动和日志轮转影响；
- 便于后续章节用相同基线比较。

并发压满适合寻找极限，但需要更严格的资源、饱和点和排队分析。

### 23.4 为什么负载工具本身要有安全边界

`healthload` 需要限制：

- URL 必须是 absolute HTTP(S)；
- 禁止 URL userinfo；
- 禁止 query/fragment，避免 token 进入报告；
- rate 上限；
- duration 上限；
- worker 上限；
- timeout 上限；
- 总调度请求上限；
- 响应 body 读取上限；
- 不输出响应 body；
- SIGINT/SIGTERM 可停止；
- dropped、timeout、unexpected status 使退出码失败。

否则一个“测试工具”可能成为误打外部服务、泄漏响应或耗尽本机资源的入口。

### 23.5 分位数的证据边界

P99 表示当前样本延迟排序中的高分位，不等于：

- 未来 SLA；
- 所有用户 99% 延迟；
- handler CPU 时间；
- MySQL 查询 P99；
- 排除 warmup 后的稳定值；
- 跨主机可比值。

报告必须同时保留：

- 样本数；
- success/error；
- unexpected status；
- dropped；
- target；
- 开始/结束时间；
- rate/duration/workers/timeout；
- 环境版本与资源。

只有分位数没有请求完整性，是危险的性能叙述。

### 23.6 M0 的通过标准如何设定

本节更适合使用“完整性门槛”而非夸张延迟 SLA：

- scheduled 与预期模型一致；
- completed 等于 scheduled；
- errors 为 0；
- unexpected status 为 0；
- dropped 为 0；
- API/Web 没有重启；
- 所有响应为期望状态；
- P99 被记录但不越权解释；
- 容器保持健康；
- 日志未出现 Secret。

如果要设置延迟阈值，应先积累多个环境和多次重复分布，再解释其业务含义。

### 23.7 为什么 5 分钟仍然很短

5 分钟无法覆盖：

- GC 长周期；
- 连接池生命周期；
- 日志长期增长；
- 内存泄漏；
- 日夜负载；
- Docker Desktop suspend/resume；
- 网络抖动；
- 镜像升级；
- 数据增长；
- 故障恢复与重试风暴。

它只是 M0 smoke-load，不是 soak test。

### 23.8 后续容量演进

未来可逐步增加：

- M1：`/ready` 的低速依赖探针成本；
- M2：首个真实只读业务 API；
- M3：写路径、事务和幂等；
- M4：Redis 命中/未命中/故障三组；
- M5：混合读写与真实数据规模；
- M6：故障期间的恢复与重试放大；
- M7：长时间 soak；
- M8：多实例与生产网络。

每一级都必须重新定义数据、负载模型、指标与退出标准。

### 23.9 本轮实际结果与可以诚实推出的结论

在本轮本地 Docker Desktop/arm64、当前五服务镜像和当前日志配置下，正式 M0 `/health` 报告记录：

| 指标 | 实际值 |
| --- | ---: |
| 目标速率 | 100 RPS |
| 持续时间 | 5 分钟 |
| 理论调度 | 30,000 |
| 完成/成功 | 30,000 / 30,000 |
| errors | 0 |
| unexpected status | 0 |
| dropped | 0 |
| P99 | 4.1495 ms |
| Max | 18.116291 ms |
| 本节 P99 门槛 | 100 ms，已通过 |

同时对 `/ready` 做了受控依赖路径基线：

| 指标 | 实际值 |
| --- | ---: |
| 目标速率 | 20 RPS |
| 持续时间 | 30 秒 |
| 完成/成功 | 600 / 600 |
| P99 | 6.841375 ms |

这组结果可以支持：

- 当前环境下，负载工具完整调度并完成了本轮 `/health` 请求；
- 本轮没有记录 transport error、意外 status 或客户端 queue drop；
- 当前 Nginx→Go 最小链路 P99 低于本节预设的 100 ms 回归门槛；
- `/ready` 在当前单实例 MySQL、当前数据规模下完成了 600 次成功探测；
- 当前容器与日志配置没有在这段时间内暴露明显的最小路径回归。

它不能支持：

- GrowthOS 业务 API 支撑 100 RPS；
- MySQL 业务 SQL 支撑 20 RPS 或更高；
- 生产环境 P99 为 4.1495 ms；
- 系统 SLA 为 100 ms；
- 高并发写入、事务、Redis、MQ 或大 payload 已经验证；
- 五分钟内没有错误就不存在内存泄漏或长周期抖动；
- 单机回环结果可以外推到公网和多副本。

100 ms 是本节为发现明显回归设置的本地门槛，不是产品需求或生产 SLO。

完整命令、开始/结束时间、工具报告和环境信息仍以[第 16 节 QA](../../qa/lessons/lesson-16.md)为唯一验收账本。

## 24. 成本模型：本地可复现不是免费的

### 24.1 资源成本

五个服务会消耗：

- MySQL 内存与数据卷；
- Redis 进程内存与 tmpfs；
- Nginx、API 的常驻内存；
- Docker Desktop VM 资源；
- image layer 磁盘；
- BuildKit、Go module、pnpm store cache；
- JSON 日志磁盘；
- 首次 pull/build 的网络与时间。

不能因为“本地开发”就默认资源无限。

### 24.2 为什么当前没有随意写 CPU/Memory limit

资源限制需要真实工作集证据。

过小会：

- 让 MySQL 初始化随机 OOM；
- 让前端 build 在小内存下失败；
- 把 Docker Desktop 资源设置差异误诊成代码问题；
- 让 M0 延迟只反映人为 throttling。

过大或不限制会：

- 多项目并行时争抢宿主资源；
- 异常内存增长影响整台开发机；
- 无法提前暴露内存预算缺失。

本节先记录实际使用和峰值，再为后续章节定义可解释限额。

Redis `/data` tmpfs 是一个例外，因为其写入语义明确、数据可丢弃且容量需求极小，可以先给出边界。

### 24.3 时间成本

开发者每次可能经历：

- Secret 检查；
- Compose config；
- image build；
- MySQL 首次初始化；
- healthcheck start period；
- Migration；
- API/Web/Redis 健康等待；
- smoke；
- 浏览器验收；
- 故障恢复。

优化方向应该基于阶段耗时：

- 依赖 layer cache；
- BuildKit cache mount；
- 只重建变更服务；
- 保留 named volume；
- 区分快速 smoke 与完整 fault matrix；
- CI 并行非依赖门禁；
- 记录 cold/warm build。

不能用跳过 Migration、复用共享 root 数据库或关闭检查来换取虚假的快。

### 24.4 认知成本

本节增加的概念包括：

- Compose service/network/volume/secret；
- multi-stage target；
- internal network；
- healthcheck condition；
- one-shot job；
- non-root/read-only/tmpfs；
- `_FILE`；
- Docker DNS；
- liveness/readiness；
- signal/grace period；
- architecture platform。

这就是为什么不同时加入 PostgreSQL、RabbitMQ、RocketMQ、Prometheus、Grafana、Jaeger、Kubernetes。

每个额外工具都会争夺理解当前边界的注意力。

### 24.5 维护成本

固定版本意味着要持续：

- 检查新 patch；
- 阅读安全公告；
- 更新镜像 tag/digest；
- 重建并运行 full verification；
- 验证 MySQL/Redis 数据格式；
- 验证 Nginx 配置与 Node build；
- 删除过时 image 时避免影响其他项目；
- 更新课程与面试材料中的事实。

“固定版本”不是一次写死，而是把升级变成可见工作。

### 24.6 安全成本

本地 Secret 文件、Docker daemon 和 named volume 都需要治理：

- Secret 备份与恢复；
- 误提交扫描；
- 泄漏后的轮换；
- Volume 数据隐私；
- Docker Desktop 更新；
- image 漏洞；
- 日志保留；
- 本机恶意进程风险。

开发环境降低了公网风险，但可能含真实样本数据。

应默认只使用合成数据，不把生产 dump 随意放入 named volume。

### 24.7 为什么仍值得

这些成本换来的不是“看起来专业”，而是：

- 更少的环境歧义；
- 更低的误删用户数据风险；
- 更明确的权限边界；
- 可复跑的故障模型；
- 可审查的运行拓扑；
- 后续 CI/部署演进的事实基础；
- 面试中能解释而不是背诵的项目材料。

## 25. 开发 Compose 与生产平台的差距

### 25.1 调度与高可用

当前只有单机 Docker Engine。

缺少：

- 跨节点调度；
- 节点故障转移；
- 多副本控制器；
- anti-affinity；
- topology spread；
- Pod/Container disruption budget；
- 滚动发布控制；
- 自动扩缩容；
- 服务级负载均衡；
- 多可用区容灾。

因此不能说“Compose 部署提高了系统高可用性”。

### 25.2 Secret 管理

当前 file-backed secrets 是本地分发机制。

生产需要重评：

- KMS/HSM；
- Secret Manager/Vault；
- workload identity；
- 动态凭据；
- 轮换和撤销；
- 审计；
- 访问策略；
- Secret at-rest encryption；
- 多环境隔离；
- break-glass 流程。

### 25.3 数据库

当前 MySQL 是单容器 + 单 named volume。

缺少：

- 备份；
- 恢复演练；
- PITR；
- 复制；
- failover；
- 存储耐久度 SLA；
- 加密；
- 参数治理；
- 容量与 IOPS；
- schema rollout 保护；
- 账号自动轮换；
- 数据分类和删除策略。

生产通常应优先评估托管数据库，而不是把此 Compose MySQL 直接搬上服务器。

### 25.4 网络与入口

当前 Nginx 只监听本机回环 HTTP。

生产需要：

- DNS；
- TLS；
- 证书自动续期；
- HSTS；
- Ingress/Load Balancer；
- trusted proxy；
- client IP 策略；
- request/body limit；
- timeout 总账；
- rate limit；
- WAF；
- DDoS；
- egress control；
- NetworkPolicy；
- API 探针私有化。

### 25.5 资源与可观测性

生产需要明确：

- request/limit；
- CPU throttling；
- memory OOM；
- Go runtime metrics；
- DB pool metrics；
- Nginx metrics；
- Redis memory/eviction；
- centralized logs；
- traces；
- dashboards；
- alert routing；
- SLO/error budget；
- on-call runbook。

### 25.6 构建与发布供应链

当前本地 build 缺少：

- hermetic CI builder；
- immutable digest promotion；
- Registry policy；
- SBOM；
- provenance；
- signature verification；
- vulnerability threshold；
- license scan；
- release approval；
- rollback artifact；
- deployment audit。

### 25.7 数据与缓存语义

Redis 当前不接业务。

生产前必须定义：

- 缓存一致性；
- 数据丢失影响；
- Redis topology；
- TLS/auth；
- persistence；
- backup；
- eviction；
- capacity；
- hot key；
- client timeout/retry；
- 降级策略。

### 25.8 开发契约中可以携带到生产的部分

可以携带：

- 服务职责分离；
- 同源浏览器 path；
- API/Migration 身份隔离；
- liveness/readiness 语义；
- 独立 Migration job；
- 配置 fail-fast 和 `_FILE` 抽象；
- 非 root/只读/最小网络原则；
- Secret 不日志化；
- 故障注入思路；
- 可证伪的证据边界。

不能原样携带：

- Compose YAML；
- 本地 bind secret；
- 单机 named volume；
- loopback 入口；
- `restart: "no"`；
- 单实例 MySQL/Redis；
- 本地 image tag；
- 当前 healthcheck interval；
- Docker Desktop M0 数值。

## 26. 备选方案与技术权衡

### 26.1 手工启动宿主进程

方案：本机运行 Go/Vite，连接用户已有 MySQL/Redis。

优点：

- HMR 和 debugger 最直接；
- 构建循环最快；
- 容器更少；
- 本机工具易用。

代价：

- SDK/版本/账号依赖宿主机；
- 数据库状态不可复现；
- 端口冲突；
- 网络与 Secret 边界不同；
- 难以验证生产式静态入口；
- 容易污染用户服务。

结论：保留为快速编码工作流，但不能替代 Compose 集成验收。

### 26.2 一组 `docker run` 脚本

优点：

- 不依赖 Compose；
- 命令行为显式；
- 对单容器学习简单。

代价：

- network、volume、secret、health、dependency 关系散落；
- 清理和幂等复杂；
- shell 跨平台差异；
- 命令很快变成长且难审查。

结论：五服务/三网络/四 Secret 已达到 Compose 更适合的复杂度。

### 26.3 Dev Containers

优点：

- IDE 与开发工具链也可容器化；
- onboarding 更一致；
- 可复用 Compose services。

代价：

- 绑定编辑器或 devcontainer workflow；
- 文件权限、性能和 debugger 复杂；
- 不能替代运行拓扑本身；
- 当前课程重点会偏移。

结论：后续可在 Compose 上加开发容器，不作为本节核心。

### 26.4 Tilt、Skaffold 或 Garden

优点：

- 文件监听；
- 增量构建；
- 本地 Kubernetes 工作流；
- 多服务开发体验。

代价：

- 工具链和配置成本显著；
- 当前没有本地集群需求；
- 会掩盖基础容器/网络概念。

结论：服务数和部署目标增长后再评估。

### 26.5 Kubernetes 本地集群

优点：

- 更接近未来集群对象；
- 可练习 Deployment/Job/Service/Secret/PVC/Probe；
- 控制器和滚动发布语义更真实。

代价：

- 资源和认知成本高；
- 数据库本地持久化更复杂；
- 仍不等于生产托管平台；
- 当前章节只需单机开发编排。

结论：生产部署章节再用真实需求推导，不提前把 YAML 数量当架构成熟度。

### 26.6 Testcontainers

优点：

- 测试按需创建依赖；
- 隔离性强；
- 生命周期跟随测试；
- 适合集成门禁。

代价：

- 不提供浏览器+Web 完整长期开发入口；
- 测试 fixture 与日常开发拓扑仍需协调；
- image pull 和 CI 环境有成本。

结论：后续集成测试可使用，不能替代本节开发 Compose。

### 26.7 托管开发数据库与 Redis

优点：

- 更接近远程网络；
- 团队共享；
- 平台可管理备份和升级；
- 开发机更轻。

代价：

- 网络、成本、权限和数据隔离；
- 离线不可用；
- 故障注入受限；
- 环境污染风险；
- Secret 分发更复杂。

结论：团队规模和远程协作出现后重评；当前学习分支优先本地可控。

### 26.8 Caddy/Traefik 代替 Nginx

可能优点：

- 配置风格不同；
- 自动 TLS 或动态发现能力；
- labels 驱动路由。

当前不选的原因：

- 本地没有 TLS 自动化需求；
- Nginx 静态/代理/cache 行为足够；
- 课程已有明确 config 可检查；
- 换产品不会消除同源、fallback、DNS、header、timeout 等设计问题。

未来由生产入口能力需求重评，而不是偏好争论。

### 26.9 distroless 代替 Alpine

优点：更小的运行用户空间、无 shell。

代价：

- 当前 wget healthcheck 不可用；
- 本地排障更难；
- CA/时区/用户和 debug 方案需重构；
- 安全收益必须由扫描和威胁模型证明。

结论：可以增加 production target，不牺牲当前学习环境可诊断性。

### 26.10 把 Redis 接入 API 只为“验证网络”

短期可以写一个 Ping 并加入 readiness。

但这会把环境验证变成公共运行依赖，而没有业务价值。

更诚实的验证是：

- Redis 自身 healthcheck；
- 网络成员检查；
- API 不在 cache 网络；
- Redis 停止时 API 无影响。

“不连接”也可以被验证，不需要制造假业务。

## 27. 反事实推演：如果选择另一条路会发生什么

### 27.1 如果复用宿主机 MySQL 3306

首次运行更快，但：

- 与用户已有容器耦合；
- 无法安全停库做故障注入；
- root/app/migrator grant 可能不是本仓库创建；
- 新同学无法复现；
- 数据清理影响其他项目；
- 端口冲突被隐藏而不是解决。

### 27.2 如果给 Compose MySQL 也发布 3306

方便 GUI 直连，却会：

- 与现有 MySQL 冲突；
- 扩大宿主机攻击面；
- 鼓励绕过 API/Migration；
- 让 smoke 可能误连错误实例；
- 需要额外决定 host bind 与凭据。

需要调试时可以使用 `docker compose exec` 或显式临时 profile，而不是永久公开。

### 27.3 如果所有服务放在默认网络

短期配置少几行，但：

- Web 可直接发现 MySQL/Redis；
- Redis 未接入这一事实不可见；
- 新服务默认获得过宽可达性；
- 网络图无法表达职责；
- 横向移动面扩大。

### 27.4 如果 API 使用 root 密码

所有 SQL 都“不会因权限失败”，但任何注入或 bug 都可能：

- DROP/ALTER 任意 schema；
- 创建用户；
- 修改权限；
- 读取不相关数据；
- 破坏 Migration 历史。

开发方便会掩盖生产必然要补的权限债务。

### 27.5 如果 API 自动运行 Migration

单实例第一次启动可能顺利。

当有多个副本或失败 DDL 时：

- 多实例争抢锁；
- API 拥有 DDL；
- 启动时间不可预测；
- liveness 与 Migration 耦合；
- 错误被 restart 循环重放；
- 回滚边界含混。

### 27.6 如果 MySQL healthcheck 用 `mysqladmin ping`

它可能只证明 server 回应，不能证明：

- 应用密码正确；
- database 可访问；
- grant 正确；
- SQL 能执行。

启动链会在错误身份上假绿。

### 27.7 如果 API healthcheck 用 `/ready`

MySQL 故障会让 API 容器 unhealthy。

这容易诱发：

- 不必要重启；
- 诊断端点丢失；
- 连接风暴；
- 故障域合并；
- 将依赖故障误写为进程故障。

### 27.8 如果 Web 等 API healthy

API 离线时用户无法打开状态页。

最需要降级 UI 的时刻，入口被依赖图阻断。

### 27.9 如果 Nginx 静态解析 API IP

初次可能工作。

API recreate 后：

- 服务名对应新 IP；
- Nginx 仍持有旧地址；
- 只有重启 Web 才恢复；
- 应用恢复被误判为编排故障。

### 27.10 如果 SPA fallback 覆盖所有路径

未知 API 返回 `200 index.html`，前端收到 contract failure。

监控若只看 status 会假绿，缓存还可能长期保存错误 HTML。

### 27.11 如果把所有静态响应设成长缓存

hash 资产收益明显，但 `index.html` 长缓存会让旧 shell 长期引用旧或不存在资产。

缓存策略必须按资源可变性分层，而不是整个 server 一条 header。

### 27.12 如果 Secret 直接写进 Compose 环境变量

配置更简单，但 Secret 更容易出现在：

- Git diff；
- `docker compose config`；
- inspect；
- shell history；
- 文档截图；
- 调试输出。

### 27.13 如果只丢失一个 Secret 就自动补齐

会形成新旧凭据混合集合。

错误可能只在某个服务启动时出现，且操作者无法判断哪些密码对应 Volume。

### 27.14 如果 Secret 全丢后自动生成并保留旧 Volume

数据库账号仍是旧密码，新文件是新密码，所有消费者认证失败。

自动化制造了不可恢复歧义。

### 27.15 如果脚本自动删除旧 Volume

认证错误表面上被“修好”，代价是全部开发数据丢失。

这种自动修复违反破坏性操作最小授权。

### 27.16 如果 Redis 使用 named volume + AOF

本节没有数据恢复需求，却引入：

- 持久状态；
- 格式升级；
- stale value；
- 清理争议；
- 备份预期；
- 面试叙述歧义。

### 27.17 如果 Redis 进入 readiness

一个尚未使用的环境组件会成为全站强依赖。

这违背“依赖由真实调用决定”。

### 27.18 如果所有容器 `restart: always`

配置错误和 Migration 错误会循环：

- 日志快速轮转；
- 第一次失败原因消失；
- CPU/网络浪费；
- 短暂绿色掩盖 crash loop；
- 学习者无法看到退出语义。

### 27.19 如果所有镜像使用 `latest`

今天和下周可能拉到不同版本。

当行为改变时，很难区分源码回归还是镜像漂移。

### 27.20 如果只看平均延迟

少量慢请求会被平均值稀释。

但只看 P99 也不够：若客户端丢了大量请求，剩余成功样本的 P99 可能很好看。

性能证据必须把完整性和分布一起记录。

## 28. 架构师会主动检查、但原始需求没有逐项说明的点

1. **Compose project name 冲突。** 两个工作树同时使用 `growthos` 可能共享命名资源；并行开发需可覆盖 project name。
2. **端口 TOCTOU。** 启动前检查 8088 空闲不保证发布时仍空闲；最终以 Docker bind 结果为准。
3. **Secret 目录备份。** 文件未进 Git，机器损坏时如何恢复旧 Volume 对应密码需要明确。
4. **Time Machine/云盘同步。** 本地 Secret 目录可能被系统备份或同步，需按组织策略排除或加密。
5. **MySQL init 只运行一次。** 修改 init script 不会更新已有 Volume，测试必须覆盖 fresh 与 existing 两条路径。
6. **`CREATE USER` 的非幂等性。** 它依赖 fresh init 生命周期；若被手工重放应预期失败，不能伪装成通用迁移。
7. **数据库字符集/时区。** `TZ=UTC` 不自动决定所有 column/session time zone；业务时间语义仍需单独治理。
8. **MySQL TLS 被禁用。** 内部开发网络的选择不能携带到生产；生产需要 server identity verification。
9. **Docker Desktop VM 时钟。** suspend/resume 可能影响 timestamp、health 和定时器，需要故障复现记录。
10. **容器 DNS TTL。** Nginx resolver `valid` 与请求恢复时间有关；过短增加 DNS 查询，过长延迟恢复。
11. **Nginx variable proxy URI。** 变量形式 `proxy_pass` 的 URI 拼接语义容易改变，必须用真实 `/api`、query 和编码路径测试。
12. **Host header。** 当前转发外部 `$host`；未来应用若信任 Host 做安全或绝对 URL 生成，必须定义 allowlist。
13. **Forwarded header 信任。** API 不能因为 Nginx 设置 `X-Forwarded-*` 就信任所有来源；生产需可信代理配置。
14. **Nginx access log Request ID。** 本轮 API-down 暴露 502 无关联 ID，最终通过“upstream Go ID 优先、否则 Nginx ID、隐藏原同名 header 后统一回写”保证每个响应只有一个最终 ID；新增代理路径仍需验证不重复。
15. **错误响应缓存。** Nginx/CDN 不应缓存携带 Request ID 的 4xx/5xx；真实 header 需要验证。
16. **`add_header` 继承。** 子 location 添加 Cache-Control 后可能丢失父级安全 header，必须逐路径验收。
17. **MIME 类型。** SPA JavaScript/CSS 的 Content-Type 错误会被浏览器拒绝，curl `/` 成功不够。
18. **压缩差异。** 当前 M0 禁用 client compression，生产 Brotli/gzip 会改变 CPU、带宽和延迟。
19. **Healthcheck 工具存在性。** 最终 image 若切 distroless，`wget` 检查会消失，需要替代 probe binary 或平台 TCP/HTTP probe。
20. **Healthcheck 成本。** MySQL 真实认证查询有频率成本；多个副本与外部监控会放大。
21. **Start period 假设。** 不同磁盘/CPU 上 MySQL 初始化可能更慢；应基于 cold start 记录调整。
22. **Migration 长任务。** Compose wait timeout 与 Migration timeout 需要分层，不应由最外层先杀死而留下不明状态。
23. **DDL 自动提交。** MySQL 部分 DDL 事务语义与普通 DML 不同，失败恢复不能假设完整回滚。
24. **大表迁移锁。** 本地空表成功不代表生产大表安全，需要 online DDL 评估。
25. **Connection pool stale connections。** MySQL 重启后池中旧连接可能先失败，再逐步替换；恢复测试要允许合理过渡但不能无限。
26. **Readiness 抖动。** 单次 Ping 成功/失败可能抖动；生产摘流需要阈值和 hysteresis。
27. **API restart count。** Docker restart policy 为 no 不代表进程绝不被 Compose recreate，故障测试要记录 container identity。
28. **PID 1 与 shell entrypoint。** Redis 脚本最终必须 `exec`，否则信号停在 shell。
29. **Secret shell 变量。** `unset` 只能缩短 shell 变量生命周期，不能擦除 Redis/MySQL 进程必要内存。
30. **ACL 权限广度。** Redis `+@all ~* &*` 适合当前单用户环境，不是未来多服务最小命令/Keyspace权限。
31. **Redis protected mode。** 密码和内部网络仍需共同存在；不能只依赖 protected-mode。
32. **Redis maxmemory。** tmpfs 限制持久目录，不限制进程堆；真正接入前必须设容器/Redis内存策略。
33. **Redis cache stampede。** 即使 Redis 本身可用，key 过期同时回源仍会打爆 MySQL；环境启动不解决业务并发。
34. **Named volume ownership。** MySQL image UID/初始化可能改变权限，升级需在复制数据上演练。
35. **Volume backup.** `compose down` 保留 Volume 不等于备份；宿主磁盘损坏仍会丢失。
36. **Volume project name。** 改 project name 会创建另一 Volume，看起来像“数据消失”，实际旧卷仍存在。
37. **Secret project name。** 生成器检查 Volume 名依赖 project name；命令入口必须传同一值。
38. **并行分支。** 多课程 worktree 若共享 8088 或 project name，会相互干扰，QA要记录分支和作用域。
39. **Image tag 本地覆盖。** 同名 `growthos/api:lesson-16` 可被另一构建覆盖；强证据应记录 image ID。
40. **Build cache 污染判断。** cache 本应不改变结果；若 clean build 与 warm build 不一致，要视为构建缺陷。
41. **Base image patch 安全。** 固定 patch 可复现但也会老化；需要升级节奏，而不是永久不动。
42. **Alpine musl 差异。** DNS、locale、时区或 native dependency 行为要在运行环境验证。
43. **CA 与外部 TLS。** 当前 API 尚无外呼；保留 CA 是未来安全连接的运行需求，但不能证明外呼策略。
44. **Timezone data。** 容器使用 UTC 是运维选择；产品显示时区仍由业务/UI决定。
45. **Rootless Docker。** 当前 Desktop daemon 权限模型与 rootless Engine 不同，跨环境需验证 mount/network。
46. **Compose 版本兼容。** v5.4.0 可解析不等于团队所有机器都可以；应给最低版本或失败说明。
47. **Docker socket。** 应用容器不应挂载 Docker socket；否则几乎等同宿主控制权。
48. **Build secrets。** 当前 build 不需要私有依赖；未来需要时用 BuildKit secret/SSH mount，不能用 ARG 传长期 token。
49. **Source maps。** 前端 production build 是否包含 source map 影响调试、大小和源码披露，需要明确。
50. **Security headers 适用性。** `X-Frame-Options: DENY` 会阻止嵌入；未来产品若需要嵌入应改 CSP frame-ancestors并评审。
51. **CSP 缺失。** 当前基础 header 不等于完整浏览器防护；生产需根据脚本/样式来源建立 CSP。
52. **Nginx server tokens。** 隐藏版本降低无用披露，但不能修复已知漏洞。
53. **Request body temp。** 未来上传/大请求可能需要写临时文件，当前小 tmpfs 会失败；应由业务需求显式扩展。
54. **Web client offline cache。** 若未来加入 Service Worker，旧 index/API response 的缓存语义会更复杂。
55. **Browser DNS 与 proxy DNS。** 浏览器只解析 127.0.0.1；Compose service discovery 发生在 Nginx，不要混淆排障位置。
56. **HTTP keep-alive。** Nginx/API/healthload 的连接复用会影响 M0；不同工具的结果不能直接比较。
57. **Ephemeral port。** 高压测试连接策略不当可能耗尽客户端端口，本节 100 RPS 有界但仍要观察工具错误。
58. **Open-loop queue drop。** 固定速率工具队列满时必须记 dropped，不能阻塞后悄悄变成 closed-loop。
59. **分位数算法。** nearest-rank 与插值算法结果可能不同，报告要保留实现与样本数。
60. **热身。** 首轮 DNS、连接、JIT/缓存会影响样本；M0是否单独热身必须记录，不能事后删除不好看的点。
61. **Docker Desktop 资源。** CPU/Memory 配额和其他容器负载会影响 M0，QA应记录或至少声明未隔离。
62. **日志轮转影响。** 5 分钟测试可能触发 rollover，延迟变化需要结合文件边界观察。
63. **Mac 睡眠。** 测试中休眠会制造大延迟/错误，不应当成服务性能。
64. **HTTP response size cap。** healthload 丢弃有限 body；如果服务异常返回巨量内容，工具不会无限读取。
65. **Error body 脱敏。** load tool不输出 body，避免把上游错误页或 Secret 当证据传播。
66. **SIGINT 报告。** 中断应明确计为未完成/错误，不能输出看似成功的部分报告。
67. **Smoke 临时文件。** headers/body 使用精确 mktemp 目录并 trap 清理，不能删除系统或用户目录。
68. **`jq` 依赖。** smoke 需要本机 jq；文档和失败信息要明确，而不是把 command not found 误判服务失败。
69. **URL override 安全。** smoke/base URL 不应含 credentials、query 或 fragment，避免敏感输入进入命令与输出。
70. **外部目标误压测。** healthload 默认 loopback，覆盖 URL 时操作者必须对目标有授权并理解影响。
71. **用户数据。** 即使是本地环境，也不能默认加载真实生产个人数据用于演示。
72. **数据清理与课程学习。** 保留 Volume 有助于连续学习，但章节测试需要同时验证 fresh boot；两者要用不同 project/备份策略。
73. **Git 分支证据。** 每一节独立提交应包含代码、QA和设计文档，不能事后把多个架构决策压成一个不可学习大提交。
74. **文档时间切片。** 镜像版本和官方行为会变化，文档必须说明本节当时版本，不能当永恒事实。
75. **简历证据边界。** 只有 QA真实通过的内容能进入最终项目经历；设计目标和未来演进不能改写成已落地。

## 29. 决策日志

| ID | 决策 | 主要理由 | 放弃/推迟项 | 重评触发器 |
| --- | --- | --- | --- | --- |
| D16-01 | 使用 Compose 管理本地多服务 | 拓扑、网络、Secret、Volume和健康关系需版本化 | 手工 run 脚本 | 服务极少或平台统一替代 |
| D16-02 | 新建项目 MySQL，不复用用户容器 | 隔离数据、账号、故障注入和端口 | 共享本地实例 | 受治理的团队开发 DB |
| D16-03 | 五服务拆分 | 生命周期和权限不同 | 单全能容器 | 职责真实合并且故障一致 |
| D16-04 | 三张最小网络 | 可达性与职责一致 | 默认共享网络 | 新消费者有真实通信需求 |
| D16-05 | 只发布 Web loopback 端口 | 单一浏览器入口、避免冲突与绕行 | 公开 API/DB/Redis | 局域网联调或受控调试 profile |
| D16-06 | Nginx 静态运行 Web | 验证 production build和代理契约 | Vite preview 运行容器 | 需要 HMR 的独立 dev profile |
| D16-07 | Nginx 动态解析 API 服务名 | API重建 IP 可变，Web需独立存活 | 静态 upstream IP | 平台服务发现/sidecar 取代 |
| D16-08 | Web 不依赖 API 启动 | 故障页必须在上游离线时存在 | Web wait API healthy | 产品不再需要独立静态可用性 |
| D16-09 | MySQL应用身份 SELECT 1 health | 验证真实认证/最小查询 | TCP/mysqladmin/root check | 更精确但低成本的 schema check |
| D16-10 | API healthcheck 使用 `/health` | 进程故障与依赖故障分离 | `/ready` 作 liveness | API无DB时完全无诊断/服务能力且平台策略明确 |
| D16-11 | 独立一次性 Migration | 高权限、失败、时机、并发隔离 | API startup auto-migrate | 单实例短命原型且风险可接受 |
| D16-12 | API/Migration 账号分离 | 最小权限与攻击面 | 共享 root/高权账号 | 无；只可能进一步细分 |
| D16-13 | file-backed secrets + `_FILE` | 减少 YAML/env泄漏通道 | 直接 Compose 环境值 | 生产 Secret Manager adapter |
| D16-14 | 直接密码与文件互斥 | 消除隐式优先级 | silent override | 有正式配置优先级协议且可观测 |
| D16-15 | Secret 完整集合生成 | 避免部分新旧凭据 | 缺什么补什么 | Secret Manager原子版本 |
| D16-16 | Volume存在时拒绝重建 Secret | 防止账号/文件失配和误删数据 | 自动生成/自动删卷 | 具备受控凭据轮换 |
| D16-17 | Secret目录0700、文件0444 | 非root Compose bind兼容与宿主目录保护 | 0600文件 | 可移植 UID/GID/mode 或专用 Secret 平台 |
| D16-18 | 多阶段 Go/Web 镜像 | 分离构建与运行、缩小攻击面 | 运行时编译 | 极特殊动态插件需求 |
| D16-19 | 精确版本 tag | 可读且降低漂移 | latest | CI 引入 digest 自动维护 |
| D16-20 | Alpine runtime | 本地诊断与 CA/时区/探针便利 | scratch/distroless | production hardened target成熟 |
| D16-21 | nonroot/read-only/capdrop/no-new-privileges | 纵深防御 | 默认宽权限 | 仅按明确能力逐项放宽 |
| D16-22 | 有界 tmpfs | 显式运行写需求与资源边界 | writable root | 真实持久写入需求 |
| D16-23 | Redis单独启动但不接业务 | 先准备环境边界，不虚构依赖 | 假缓存Ping | 首个缓存业务用例 |
| D16-24 | Redis无RDB/AOF、tmpfs | 无恢复目标，不制造持久状态 | named volume/AOF | 业务定义恢复目标 |
| D16-25 | 本地 restart no | 保留第一次失败证据 | always/on-failure | 生产控制器或特定开发需求 |
| D16-26 | JSON日志轮转 | 可解析且防无界磁盘 | 无轮转/文件日志 | 中央采集取代本地保留 |
| D16-27 | 关闭driver原始logger | 防脱敏/结构边界旁路 | raw stderr | 安全结构化 driver adapter |
| D16-28 | M0为100RPS/5min `/health` | 有界、可重复的最小链路基线 | 极限压测 | 首个业务 API 和资源数据 |
| D16-29 | 不加入PG/RabbitMQ/RocketMQ | 无当前问题与业务证据 | 技术堆叠 | 明确查询/异步需求与语义 |
| D16-30 | 统一Nginx/Go最终Request ID并脱敏代理日志 | 502必须可关联且query/referrer不能泄漏 | 原样透传+stock error log | 引入结构化网关日志/Tracing |

## 30. 不变量与信任边界

### 30.1 本节不变量

1. 浏览器只通过 Web loopback 入口访问当前 Compose；
2. MySQL、Redis、API、Migration 不发布 host port；
3. Web 与 API 共享 `edge`，MySQL/API/Migration 共享 `data`，Redis独占 `cache`；
4. MySQL 与 cache 网络保持 internal；
5. Web 不以 API healthy 作为创建前置；
6. API 只有在 MySQL healthy 且 Migration 成功后创建；
7. `depends_on` 只被描述为启动条件；
8. API Docker healthcheck 使用 `/health`；
9. 业务 readiness 通过 `/ready` 表达 MySQL状态；
10. MySQL healthcheck 使用应用账号执行真实查询；
11. Migration 成功状态是退出 0，不是常驻 healthy；
12. API 不持有 root 或 Migration Secret；
13. Migration 不持有 API或root Secret；
14. Web 不持有数据库/Redis Secret；
15. Redis 只持有自己的 Secret；
16. Secret 值不进入 Git、image、日志、QA；
17. 直接密码与 `_FILE` 恰好选择一个；
18. Secret 文件读取有界且错误不回显值/路径；
19. 部分 Secret 集合不会自动补齐；
20. 已有 MySQL Volume 时不会自动生成新集合；
21. 普通 down 不删除 MySQL Volume；
22. 任何 Volume 删除都是独立破坏性操作；
23. Redis 当前不属于 API 业务依赖；
24. Redis 当前数据可丢弃且不持久；
25. Redis停止不改变 API readiness；
26. API/MySQL短暂断连后恢复不依赖 API重启；
27. API重建后 Web不需重启即可重新代理；
28. 运行容器默认非root、只读、cap drop、no-new-privileges；
29. 允许写入只存在于明确 volume/tmpfs；
30. Web hash资产与HTML使用不同缓存策略；
31. API路径不会落入 SPA fallback；
32. 错误响应不缓存且保持 Request ID语义；
33. 日志使用稳定安全字段，不输出驱动原始 cause；
34. M0任何 dropped/error/unexpected status 都是失败；
35. M0 `/health` 结果不外推成业务或数据库容量；
36. arm64本地成功不外推成amd64发布；
37. Compose本地成功不外推成生产HA；
38. 本项目命令不修改用户已有 Docker资产；
39. 设计意图与QA证据始终分开；
40. 最终简历只引用真实完成并可复核的事实。
41. 正常Go响应、Nginx网关错误和静态响应都只回写一个最终`X-Request-ID`；
42. Nginx请求日志不记录query string或Referer，敏感marker回归必须保持零命中。

### 30.2 信任边界表

| 边界 | 边界外输入 | 进入前检查 | 边界内能力 | 泄漏/滥用风险 |
| --- | --- | --- | --- | --- |
| Host → Make | project/file/port变量 | 格式、路径、Compose config | 操作本项目资源 | 指向错误Project |
| Filesystem → Secret生成器 | 目录、已有文件、Volume状态 | 完整集合、格式、权限、卷检查 | 生成/验证本地凭据 | 覆盖或失配 |
| Secret文件 → Compose | file path | readable、ignored、非build context | 挂载到声明服务 | host/daemon读取 |
| `/run/secrets` → Go config | path/bytes | 互斥、bounded、nonempty | 数据库认证 | 路径/值泄漏 |
| `/run/secrets` → Redis entrypoint | 64 hex bytes | readable、格式、长度 | 生成ACL | shell/进程内存 |
| Compose → MySQL init | root/app/migrator秘密 | fresh volume、脚本校验 | 创建账号/grant | SQL插值、生命周期误用 |
| API → MySQL | SQL、应用身份 | driver config、grant、timeout | DML/查询 | 注入、数据越权 |
| Migration → MySQL | 版本化DDL、高权身份 | lock、statement timeout、版本 | schema变更 | 锁、不可逆变更 |
| Browser → Web | path/header/body | Nginx路由/limit | 静态与代理 | traversal、header信任 |
| Web → API | HTTP与forwarded headers | 精确location、timeout、DNS | 调用Go | gateway/cache错误 |
| Docker DNS → Nginx | service name/IP | resolver与网络成员 | 上游解析 | stale/poison边界 |
| Network → API | HTTP请求 | router/middleware/validation | 业务能力 | 未授权输入 |
| API error → Logs/HTTP | internal cause | fault映射、Nop driver logger | 安全诊断 | Secret/DSN泄漏 |
| Container → Root FS | 文件写入 | read_only | 无默认持久修改 | writable bypass |
| Container → tmpfs | 临时数据 | size/mode/noexec/nosuid | 受限写入 | 内存耗尽 |
| Container → Network | service连接 | 最小network成员 | 允许路径 | 横向移动 |
| healthload → Target | URL/rate/duration | scheme/userinfo/query/limits | 发HTTP负载 | 未授权压测 |
| HTTP response → healthload | status/body/timing | body cap、不输出body | 统计 | 敏感响应传播 |
| QA → 简历 | 命令与结果 | 完成状态、证据边界 | 项目叙述 | 夸大/虚构 |

### 30.3 Secret 信任链

```text
OS random source / openssl
  ↓
private temporary directory
  ↓ validate exact shape
complete local secret set
  ↓ Compose per-service mount
/run/secrets/<declared-name>
  ↓ bounded loader / entrypoint
process memory
  ↓ authenticated protocol
MySQL / Redis
```

链上任何一步只要：

- 输出值；
- 扩大服务授权；
- 混用身份；
- 忽略 Volume历史；
- 允许不受限读取；

就会破坏整体安全性。

## 31. 假设、风险与验证矩阵

| ID | 假设/风险 | 当前依据 | 失效影响 | 验证/观察 | 重评时机 |
| --- | --- | --- | --- | --- | --- |
| A16-01 | Docker Desktop版本支持当前Compose字段 | 本轮本机版本检查 | config/up失败 | config+实际up | 团队最低版本确定时 |
| A16-02 | 官方image均有arm64变体 | 本轮tag/平台检查 | 模拟或pull失败 | image inspect/build | 每次升级 |
| A16-03 | 8088默认可用 | 当前项目选择 | Web bind失败 | 启动结果 | 并行项目/CI |
| A16-04 | 单机五服务资源可接受 | 当前开发机 | OOM/卡顿 | cold start/M0资源 | 加limits前 |
| A16-05 | Alpine满足运行依赖 | 当前静态Go/Nginx/Redis | runtime错误 | image smoke | native依赖加入时 |
| A16-06 | nonroot可读Compose Secret | 0444兼容折中 | 启动失败 | 实际mount/UID | Docker平台变化 |
| A16-07 | 0700目录足以保护host遍历 | 单用户开发机 | 本机进程读取 | 权限检查/组织策略 | 多用户host |
| A16-08 | Secret完整集合与卷检查避免失配 | 生命周期推演 | DB锁死/误删 | fresh/existing/partial测试 | 轮换实现时 |
| A16-09 | Volume名称可由project稳定推导 | Compose命名规则 | 脚本漏检旧卷 | 实际volume列表 | external/name覆盖时 |
| A16-10 | MySQL init只在fresh卷运行 | 官方image机制 | 修改脚本不生效 | fresh/existing对比 | image升级 |
| A16-11 | 应用grant足够当前API | 当前只探针/后续DML | 运行权限错误 | allow+deny integration | 首个业务SQL |
| A16-12 | Migration grant足够当前DDL | 当前迁移集 | 发布失败 | migration integration | 新DDL类型 |
| A16-13 | MySQL SELECT1是合理启动检查 | 最小真实身份路径 | 假绿/成本 | fault test | schema需求增长 |
| A16-14 | API连接池可跨MySQL重启恢复 | driver/database/sql预期 | 必须重启API | stop/start无重启测试 | driver升级 |
| A16-15 | `/health`不依赖DB | 现有handler | DB故障误杀API | stop MySQL | probe修改时 |
| A16-16 | `/ready`能安全表达DB故障 | 现有handler/error边界 | 泄漏/假绿 | 503/header/log检查 | 多依赖加入时 |
| A16-17 | Nginx动态DNS适应API重建 | resolver+variable配置 | 长期502 | API recreate测试 | Nginx升级 |
| A16-18 | Web离线页不需API前置 | 静态bundle可运行 | 故障页不可达 | stop API browser test | SSR/认证变化 |
| A16-19 | SPA fallback不吞API路径 | location优先级 | 200HTML假成功 | unknown API smoke | 路由修改时 |
| A16-20 | Cache header分层正确 | config意图 | 陈旧HTML/缺header | 浏览器/curl headers | CDN加入时 |
| A16-21 | Redis当前无业务消费者 | go.mod/config/network | 停止影响API | stop Redis + code audit | Redis客户端加入时 |
| A16-22 | Redis数据可全部丢弃 | 无业务写入 | 隐性数据损失 | 代码/文档检查 | 首个缓存用例 |
| A16-23 | Redis tmpfs容量足够启动 | 极小runtime写入 | Redis启动失败 | health+disk usage | 数据写入时 |
| A16-24 | Redis进程内存暂不需limit | 无业务负载 | 异常host内存 | baseline观察 | 缓存接入前 |
| A16-25 | restart no提升本地诊断 | 课程目标 | 服务不自愈 | 故障演练 | CI/生产策略 |
| A16-26 | stop grace覆盖应用shutdown | 当前timeout关系 | 强杀/请求截断 | signal测试 | timeout变化 |
| A16-27 | driver NopLogger不损害必要诊断 | 应用已有安全事件 | 排障信息不足 | 故障日志审查 | 真实事件反馈 |
| A16-28 | JSON log轮转容量合理 | 本地基线 | 日志丢失/磁盘增长 | M0前后日志大小 | 负载提升 |
| A16-29 | fixed tags降低可接受漂移 | 精确版本 | tag重指/漏洞老化 | digest/upgrade review | CI供应链 |
| A16-30 | BuildKit cache不改变结果 | 工具语义 | warm/clean差异 | no-cache对比 | 构建异常时 |
| A16-31 | 100RPS/5min对本机安全 | 最小health路径 | 资源干扰 | 前置资源检查/可中断 | CI或共享host |
| A16-32 | healthload客户端能维持rate | bounded workers/queue | dropped导致假低压 | dropped/actualRPS | 每次M0 |
| A16-33 | nearest-rank分位足够M0 | 明确算法 | 与其他工具差异 | 保存实现/样本 | 性能门禁升级 |
| A16-34 | M0日志开销属于目标路径 | 当前部署真实行为 | 与纯handler不可比 | 同配置重复 | 引入采样时 |
| A16-35 | 项目资源不碰用户容器 | project作用域/无端口复用 | 数据损失/中断 | before/after Docker资产 | 每次清理 |
| R16-01 | Secret误入Git/image | 人为配置变化 | 凭据泄漏 | ignore+history/layer扫描 | 每次构建/提交 |
| R16-02 | Secret被本机备份同步 | OS设置未知 | 扩大副本 | 本机策略检查 | 上线真实凭据前 |
| R16-03 | `docker inspect`输出被共享 | 排障习惯 | 敏感元数据泄漏 | 文档/培训 | 每次事件 |
| R16-04 | DB数据未备份却被当重要 | named volume持久错觉 | 不可恢复丢失 | 数据分级/备份声明 | 业务数据进入时 |
| R16-05 | project name改变造成“数据消失” | volume命名耦合 | 误建新库/误删旧卷 | volume inventory | 分支并行时 |
| R16-06 | 同名image被另一分支覆盖 | local tag可变 | QA对象漂移 | imageID记录 | 多worktree |
| R16-07 | Nginx forwarded header被误信 | 当前未定义trusted proxy | IP/协议伪造 | API proxy配置审查 | 认证/审计上线 |
| R16-08 | 探针被公网暴露 | 未来部署未知 | 信息泄漏/放大DB | ingress policy | 首次部署 |
| R16-09 | MySQL TLS disabled被复制生产 | 开发配置惯性 | 窃听/冒充 | environment gate | staging前 |
| R16-10 | Redis ACL过宽被复制生产 | 单用户开发配置 | key/command越权 | per-service ACL设计 | 首个消费者 |
| R16-11 | 无CPU/memory limit影响host | 当前观测不足 | Docker Desktop不稳 | resource metrics | 服务增长 |
| R16-12 | 固定patch含已知漏洞 | 版本会老化 | 安全风险 | scanner/advisory | 定期/发布前 |
| R16-13 | M0被误写成业务QPS | 求职叙述诱因 | 技术不诚信 | 简历证据审查 | 最终简历 |
| R16-14 | 设计目标被写成已通过 | 文档并行编写 | 虚假证据 | QA链接/status | 每次提交 |

## 32. 什么证据会迫使我们修改当前方案

### 32.1 Compose 本身的替换触发器

- 团队需要跨主机调度；
- 需要多副本自动恢复；
- 需要滚动发布、自动扩缩容；
- 本地环境必须高度贴近 Kubernetes；
- Compose 版本兼容成为团队主要成本；
- 平台已提供统一开发沙箱。

### 32.2 网络拓扑变化触发器

- 新 worker 真实消费 Redis；
- 新服务需要访问 MySQL；
- Web 不再承担入口；
- 引入独立网关；
- 需要外部 OAuth/支付等 egress；
- 生产采用 NetworkPolicy/service mesh；
- 多租户需要更强隔离。

每次加入网络都要附带通信需求与威胁评估。

### 32.3 Secret 方案变化触发器

- 团队多人共享环境；
- 凭据需要自动轮换；
- 进入 staging/production；
- 审计要求动态凭据；
- Docker bind mode 在新平台不兼容；
- 本地 Secret 恢复成为事故；
- 需要短期 token 而非静态密码。

### 32.4 镜像方案变化触发器

- 漏洞扫描要求更小 runtime；
- 需要 distroless；
- 引入 CGO；
- 需要 FIPS；
- 多架构发布；
- 构建耗时不可接受；
- 私有 module/npm依赖；
- 需要 provenance和签名。

### 32.5 Migration 方案变化触发器

- 多API副本滚动发布；
- 大表和在线DDL；
- 数据回填耗时超过部署窗口；
- 需要人工审批；
- 跨服务共享schema；
- Migration失败恢复复杂；
- 需要独立发布流水线。

### 32.6 Redis 方案变化触发器

- 首个可量化缓存用例；
- DB读负载达到瓶颈；
- 需要分布式rate limit；
- 需要短期会话/幂等状态；
- 可定义故障降级；
- 可定义持久性和恢复目标；
- 能观察命中率、内存和eviction。

### 32.7 Health/Readiness 变化触发器

- 新强依赖加入；
- Ping无法代表可服务；
- 多依赖需要并发预算；
- readiness抖动影响流量；
- 探针QPS过高；
- 安全要求限制外部访问；
- 平台需要startup probe。

### 32.8 M0变化触发器

- 出现真实业务API；
- 资源配置固定；
- 多环境要建立回归阈值；
- 引入性能CI；
- 100RPS过低无法暴露问题；
- 100RPS对共享runner过高；
- 延迟阈值有业务SLO依据。

## 33. 给学习者的第一性原则推理模板

面对“给项目加一个基础设施组件”，不要从镜像名开始。

### 33.1 第一步：写问题，而不是写工具

回答：

- 用户或开发者当前遇到什么可观察问题？
- 不解决会造成什么损失？
- 问题是性能、可靠性、一致性、安全、成本还是开发体验？
- 有什么数据证明问题存在？
- 最小可交付结果是什么？

模板：

```text
我们不是为了加入 ______，
而是为了把 ______ 从隐式/人工状态变成 ______。
如果不做，最可能发生 ______；
如果做错，最大破坏半径是 ______。
```

### 33.2 第二步：列事实、假设、未知

```text
事实：代码/配置/实测已经证明的内容。
假设：当前暂时接受、但可失效的前提。
未知：尚无证据，不能写成结论的内容。
```

每项假设都补：

- 失效影响；
- 观察信号；
- 验证动作；
- 重评时机。

### 33.3 第三步：画三张图

1. 请求/调用图；
2. 启动/控制图；
3. 数据/状态生命周期图。

对每条边写：

- 协议；
- 身份；
- 超时；
- 重试；
- 失败语义；
- 谁恢复；
- 是否持久；
- 如何观察。

### 33.4 第四步：给每个组件一句唯一职责

如果一句话里出现多个不同生命周期的“并且”，考虑拆分。

模板：

```text
组件 ______ 只负责 ______。
它可以依赖 ______。
它成功意味着 ______。
它明确不负责 ______。
```

### 33.5 第五步：先设计最小权限

从需要的动作列白名单：

- 需要哪些网络？
- 需要哪些端口？
- 需要哪些数据库动作？
- 需要哪些文件？
- 需要哪些Secret？
- 需要写哪个目录？
- 需要root/capability吗？
- 需要访问宿主机吗？

对每个“需要”给出用例；没有用例就删除。

### 33.6 第六步：分开五种生命周期

```text
build
start
runtime
stop
data/credential retention
```

分别问：

- 谁触发？
- 成功是什么？
- 失败停在哪里？
- 是否自动重试？
- 状态是否跨重建保留？
- Secret是否仍匹配？
- 如何恢复？

### 33.7 第七步：写失败矩阵

至少覆盖：

- 依赖未启动；
- 依赖启动后故障；
- 依赖恢复；
- 凭据错误；
- 权限不足；
- 网络隔离；
- 端口冲突；
- 磁盘/内存不足；
- 超时；
- 进程退出；
- 配置漂移；
- 数据持久状态与新配置失配。

### 33.8 第八步：为每个失败指定恢复所有者

```text
故障 ______
由 ______ 第一响应，
因为它拥有 ______；
不应该通过 ______ 恢复，
因为那会掩盖/扩大 ______。
```

### 33.9 第九步：写反向测试

不仅证明允许行为，还要证明禁止行为：

- 该访问必须失败；
- 该端口必须不存在；
- 该Secret必须未挂载；
- 该组件停止必须不影响另一组件；
- 该账号必须不能执行高权动作；
- 该错误必须不输出内部 cause；
- 该数据删除必须不能由普通命令触发。

### 33.10 第十步：定义证据等级

```text
配置存在 ≠ 配置可解析
可解析 ≠ 镜像可构建
可构建 ≠ 容器可启动
可启动 ≠ 健康
健康 ≠ 全链路可用
正常可用 ≠ 故障可恢复
故障可恢复 ≠ 生产高可用
一次压测 ≠ 容量SLA
```

### 33.11 第十一步：计算成本

至少估算：

- CPU；
- 内存；
- 磁盘；
- 网络；
- 日志；
- 构建时间；
- 开发者认知；
- 升级维护；
- 安全治理；
- 数据恢复。

如果收益无法覆盖成本，就不应加入。

### 33.12 第十二步：写生产差距

明确：

- 当前只在哪个环境成立；
- 哪些安全能力缺失；
- 哪些数据能力缺失；
- 哪些HA能力缺失；
- 哪些性能结论不能外推；
- 进入生产前必须新增哪些门禁。

### 33.13 第十三步：写重评触发器

避免“未来再说”这种不可行动表述。

触发器应该可观察，例如：

- 第三个重复 decoder 出现；
- DB读P95超过目标且SQL/索引优化后仍不足；
- 两个服务需要同一异步事件；
- 团队需要并行多工作树；
- Secret轮换周期进入合规要求；
- 单机故障已经影响发布；
- 某镜像出现高危漏洞。

### 33.14 第十四步：约束简历语言

把动词按证据分层：

| 证据 | 可用动词 | 不可用动词 |
| --- | --- | --- |
| 只有设计 | 设计、规划、权衡 | 实现、验证、提升 |
| 代码+单测 | 实现、覆盖 | 上线、生产稳定 |
| Compose实测 | 搭建、验证本地故障 | 高可用、生产级 |
| M0报告 | 在特定环境完成基线 | 支撑业务峰值 |
| 生产指标 | 优化、降低/提升（带口径） | 无数据夸大 |

最终项目经历必须能从每句话回到 Git commit、QA 或真实指标。

## 34. 本节决策的推导链

### 34.1 从需求到拓扑

```text
需要可复现的完整本地入口
  → 运行静态Web、API、数据库
  → schema变化需要独立高权生命周期
  → 加入一次性Migration
  → 后续缓存环境要准备但尚无业务语义
  → Redis独立存在且隔离
  → 五服务
```

### 34.2 从故障到网络

```text
浏览器只需HTTP
  → 只发布Web

Web只需API
  → edge: web/api

API与Migration需MySQL
  → data: api/migrate/mysql

Redis暂无消费者
  → cache: redis only
```

### 34.3 从权限到进程

```text
API只需DML
  → growthos_app

schema变化需DDL
  → growthos_migrator
  → 独立Migration Job

实例初始化需root
  → 只在MySQL bootstrap边界
```

### 34.4 从Secret到Volume保护

```text
密码不进YAML/env
  → file-backed secrets + _FILE

多身份必须一致创建
  → 完整集合

MySQL账号跨容器重建保存在Volume
  → Secret不能随意重生

Volume存在 + Secret缺失
  → fail closed，不自动生成/删卷
```

### 34.5 从用户体验到Web独立性

```text
API故障时仍需展示解释页
  → Web成功不依赖API
  → Web无depends_on
  → 自身/container-health
  → 请求时Docker DNS解析API
```

### 34.6 从真实故障到探针

```text
Go活着但MySQL可能故障
  → /health与/ready分离
  → 容器liveness打/health
  → 页面同时展示两个事实

MySQL启动必须验证应用身份
  → authenticated SELECT 1 healthcheck
```

### 34.7 从证据边界到M0

```text
需要最小可重复负载证据
  → 固定速率/时长/目标
  → 100RPS×5min /health
  → 记录完整性+分位数
  → 明确不外推业务/DB容量
```

## 35. 可追溯证据

### 35.1 决策、课程与验收

- [第 16 节课程正文](../../course/part-02/lesson-16-docker-compose-development.md)
- [ADR-0012：Docker Compose 开发拓扑与运行边界](../../decisions/ADR-0012-compose-development-topology.md)
- [第 16 节 API 记录](../../api/lessons/lesson-16.md)
- [第 16 节 QA](../../qa/lessons/lesson-16.md)
- [第 16 节面试问答](../../interview/lessons/lesson-16.md)
- [第 15 节同源全栈设计手记](lesson-15.md)
- [第 13 节 MySQL 与 Migration 设计手记](lesson-13.md)
- [配置参考](../../configuration.md)

以上部分文件可能由本节其他交付步骤同步创建；是否存在和内容是否验收，以最终分支状态为准。

### 35.2 Compose 与构建

- [五服务、三网络、Secret、Volume 与健康条件](../../../deploy/compose/compose.yaml)
- [Go API/Migration 多阶段镜像](../../../deploy/docker/Dockerfile.backend)
- [Web Node→Nginx 多阶段镜像](../../../deploy/docker/Dockerfile.web)
- [Redis 运行镜像](../../../deploy/docker/Dockerfile.redis)
- [Nginx 同源路由、动态 DNS、缓存与安全 header](../../../deploy/docker/nginx.conf)
- [Redis Secret→ACL 与非持久配置](../../../deploy/docker/redis-entrypoint.sh)
- [Docker build context 排除规则](../../../.dockerignore)
- [统一 Compose 命令入口](../../../Makefile)

### 35.3 身份、Secret 与数据边界

- [MySQL fresh-volume 用户与最小 grant](../../../deploy/compose/mysql/init/10-create-growthos-users.sh)
- [本地 Secret 目录说明](../../../deploy/compose/secrets/README.md)
- [完整集合、权限与 Volume 保护生成器](../../../scripts/generate-compose-secrets.sh)
- [Go 直接值/`_FILE` 互斥和有界读取](../../../internal/platform/appconfig/config.go)
- [Secret 配置边界测试](../../../internal/platform/appconfig/config_test.go)
- [MySQL driver 安全配置与 logger 边界](../../../internal/infrastructure/mysql/config.go)
- [MySQL 配置测试](../../../internal/infrastructure/mysql/config_test.go)
- [版本化 Migration](../../../migrations)

### 35.4 验证工具

- [Compose 正常路径与端口 smoke](../../../scripts/compose-smoke.sh)
- [M0 固定速率 `/health` 工具](../../../cmd/healthload/main.go)
- [M0 工具测试](../../../cmd/healthload/main_test.go)
- [API liveness](../../../internal/infrastructure/httpapi/health.go)
- [API MySQL readiness](../../../internal/infrastructure/httpapi/readiness.go)
- [Request ID 与中间件](../../../internal/infrastructure/httpapi/request_id.go)
- [安全错误边界](../../../internal/infrastructure/httpapi/errors.go)

### 35.5 官方行为参考

- [Docker Compose 概览](https://docs.docker.com/compose/)
- [Compose 启动顺序与健康条件](https://docs.docker.com/compose/how-tos/startup-order/)
- [Compose services 参考](https://docs.docker.com/reference/compose-file/services/)
- [Compose networking](https://docs.docker.com/compose/how-tos/networking/)
- [Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/)
- [Docker volumes](https://docs.docker.com/engine/storage/volumes/)
- [Docker multi-stage builds](https://docs.docker.com/build/building/multi-stage/)
- [Docker multi-platform builds](https://docs.docker.com/build/building/multi-platform/)
- [Docker build cache 优化](https://docs.docker.com/build/cache/optimize/)
- [Docker build context](https://docs.docker.com/build/concepts/context/)
- [Docker security](https://docs.docker.com/engine/security/)
- [Docker json-file logging driver](https://docs.docker.com/engine/logging/drivers/json-file/)
- [MySQL 官方镜像](https://hub.docker.com/_/mysql)
- [Redis 官方镜像](https://hub.docker.com/_/redis)
- [Vite 静态部署边界](https://vite.dev/guide/static-deploy.html)
- [Nginx proxy module](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)
- [Nginx `try_files`](https://nginx.org/en/docs/http/ngx_http_core_module.html#try_files)

官方文档说明产品机制；本项目是否正确使用仍必须由代码、版本与 QA 实测共同证明。

### 35.6 实现提交时间切片

- `e746a6f`：为 API 与 Migration 增加互斥、受限、脱敏的 MySQL `_FILE` 凭据加载；
- `52c3add`：关闭会旁路结构化脱敏边界的 MySQL driver 默认 logger；
- `7aa6c9e`：加入隔离的 Compose 栈、多阶段镜像、Secret/Volume 保护、smoke 与 M0 工具；
- 后续文档提交记录课程、ADR、QA、面试与本设计手记的最终证据。

这些 commit hash 用于把“为什么这样设计”追溯到代码时间切片；若后续提交修正运行验收发现的 Nginx 关联/脱敏问题，应以分支最终历史和 QA 的精确 hash 为准，而不是假定它已经包含在更早提交中。

## 36. 本节设计结论

第 16 节交付的核心不是“一条命令启动五个容器”，而是一套可以被审查和推翻的开发运行边界。

Web、API、Migration、MySQL 和 Redis 被拆开，是因为它们拥有不同的成功定义、权限、故障与数据生命周期；`edge`、`data`、`cache` 被拆开，是因为可达性本身就是能力；只有 Web 发布 loopback 端口，是因为浏览器只需要同源 HTTP 入口，而不是直连每个基础设施。

MySQL 用应用账号执行真实查询后，Migration 才以独立高权限身份运行，成功退出后 API 才启动。这套启动顺序只解决首次创建，不冒充运行期自愈。运行中 MySQL 故障时，API 应保持 liveness、readiness 降级，并在数据库恢复后不重启地恢复连接；API 故障时，静态 Web 应继续存在；Redis 故障时，当前业务应完全无感。这三个反事实比正常 `up` 更能证明职责是否真实分离。

file-backed Secret 的难点不在生成随机字符串，而在凭据与持久卷拥有不同生命周期。完整集合、部分集合失败、已有 Volume 禁止重生成、`0700` 目录与为非 root bind mount 兼容的 `0444` 文件，都是从失配和误删风险推导出的开发折中。它们不应被复制成生产 Secret 模板，但比把密码写进 YAML 或自动 `down -v` 更诚实。

多阶段镜像、非 root、只读 root、受限 tmpfs、cap drop、no-new-privileges、内部网络、日志轮转和驱动日志脱敏共同缩小运行面；任何一项都不能单独提供安全。Nginx 的动态 Docker DNS、精确 API location、SPA fallback 和分层缓存头共同守住第 15 节同源契约；配置存在仍需通过 API 重建、错误 JSON、Request ID 和真实浏览器响应头来证伪。

Redis 被启动但不接入业务，是本节刻意保留的诚实边界：环境能力可以先准备，依赖语义不能提前编造。PostgreSQL、RabbitMQ 和 RocketMQ没有加入，也不是遗漏，而是当前没有可量化问题、数据语义或故障模型证明其成本。技术选型的成熟度不由组件数量决定，而由每个组件是否有清楚的责任、失败、恢复、证据和退出条件决定。

M0 的 100 RPS、5 分钟 `/health` 只是一条最小 HTTP/Nginx/Go/日志链路的可重复基线。它必须同时报告调度、完成、错误、丢弃、状态和延迟分布；它不能变成业务 QPS、数据库容量或生产 SLA。真实数值必须以 QA 为事实源；设计手记和课程可以引用已经发生的结果解释决策，但不得预言、改写或脱离环境传播它。

最重要的第一性原则仍然是：环境中“存在”一个容器，不等于系统“依赖”它；容器 `running` 不等于服务 `ready`；一次 ready 不等于业务正确；自动重启不等于故障恢复；Volume 保留不等于数据备份；精确 tag 不等于不可变供应链；本机 arm64 构建不等于多架构发布；Compose 成功也不等于生产高可用。

只要后续章节继续用事实源、最小权限、生命周期、故障域、可逆性和可证伪性来约束新增技术，GrowthOS 的架构就不会沦为“把热门组件都放进 YAML”，而会逐步成长为一个知道自己为什么依赖某项能力、依赖失败时会发生什么、又需要什么证据才有资格对外陈述的真实项目。
