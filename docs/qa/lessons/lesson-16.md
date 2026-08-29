# 第 16 节 QA：Docker Compose 开发环境验收记录

- **对应章节：** [Docker Compose 开发环境](../../course/part-02/lesson-16-docker-compose-development.md)
- **API 契约：** [第 16 节 API 记录](../../api/lessons/lesson-16.md)
- **设计推导：** [第 16 节第一性原理设计手记](../../design-thinking/lessons/lesson-16.md)
- **面试复盘：** [第 16 节面试问答](../../interview/lessons/lesson-16.md)
- **分支：** codex/lesson-16-docker-compose-development
- **实现提交：** e746a6f、52c3add、7aa6c9e
- **文档内容提交：** ad8078c
- **最终检查点：** 以包含本清单的 `codex/lesson-16-docker-compose-development` 分支 tip 为准
- **验收日期：** 2026-08-29
- **验收结果：** 通过

> 本记录只把真实执行过、可由仓库代码和运行栈交叉验证的结果写成证据。三个实现提交与文档内容提交 `ad8078c` 均已存在；包含该哈希登记的最终清单提交无法在自身内容中引用自身，因此完整检查点以同名分支 tip 为准。本文记录的是 Apple Silicon 单机开发环境的可复现性、安全边界、故障恢复和 M0 门禁，不把它外推成生产高可用、容量结论或安全认证。

---

## 1. 验收目标与判定口径

本节不是以“仓库里出现了一个 compose.yaml”为完成标准，而是验证以下完整链路：

1. 五个服务能够从固定版本镜像构建并启动；
2. MySQL 健康后，一次性迁移成功退出，随后 API 才进入服务态；
3. Web、API、MySQL、Redis 与 Migration 的端口、网络和凭据权限符合最小暴露原则；
4. 可收紧的运行容器以非 root、只读根文件系统、全能力丢弃和 no-new-privileges 运行；
5. MySQL 官方镜像的启动例外被明确记录，没有用“全部非 root”掩盖事实；
6. API 和 Migration 从文件读取不同 MySQL 密码，直接值与文件值不能同时存在；
7. Secret 生成器对完整生成、重复运行、部分集合和已有数据卷四种状态 fail closed；
8. MySQL 应用账号和迁移账号的授权在全新数据卷中可重放；
9. Nginx 只开放回环入口，并正确处理同源代理、SPA fallback、安全头、缓存头和动态 Docker DNS；
10. MySQL、API、Redis 分别中断时，页面与探针给出真实状态，恢复不依赖无关服务重启；
11. 故障日志保持结构化且不记录查询参数、Referer 或密码；
12. API 不可达时，Nginx 最终 502 仍返回可与安全 access log 关联的 X-Request-ID；
13. 宽屏、移动端、深色和离线状态均通过浏览器检查，axe 自动扫描均为 0 violation；
14. 固定的 5 分钟 100 RPS M0 健康探针门禁真实完成，P99 低于 100 ms；
15. 验收结束后恢复正常栈，不删除用户既有容器、数据或本节仍需复用的 Secret。

以下术语在本文中保持严格区分：

- “声明”表示 Compose、Dockerfile 或脚本中写出了配置；
- “静态检查通过”表示配置被解析或代码测试验证；
- “运行态通过”表示真实容器、网络、HTTP 或日志被检查；
- “恢复通过”表示故障注入后重新启动目标服务并再次得到正常结果；
- “未覆盖”表示本节没有做出该能力，不能通过措辞暗示已经具备。

---

## 2. 环境与版本证据

### 2.1 宿主机与工具链

| 项目 | 验收值 |
| --- | --- |
| 宿主机 | macOS 26.5.1，Build 25F80 |
| 内核 | Darwin 25.5.0 |
| CPU 架构 | arm64，Apple Silicon |
| Go | go1.26.6 darwin/arm64 |
| Node.js | v24.19.0 |
| 宿主机 pnpm | 11.22.0 |
| Web 构建阶段固定 pnpm | 10.13.1 |
| Docker Client | 29.7.2，darwin/arm64 |
| Docker Server | 29.7.2，linux/arm64 |
| Docker Compose | v5.4.0 |
| 浏览器自动化 | agent-browser 0.35.1 |
| 浏览器 | Chrome for Testing 152.0.7977.64 |
| 可访问性规则 | axe-core 4.12.1 |

宿主机 pnpm 与镜像构建阶段 pnpm 特意分开记录。Dockerfile 通过 Corepack 固定 pnpm 10.13.1，因此镜像构建不依赖宿主机当前安装的 11.22.0。

复查命令：

~~~bash
sw_vers
uname -m
uname -r
go version
node --version
pnpm --version
docker version
docker compose version
~~~

### 2.2 固定镜像版本

| 用途 | Dockerfile 或 Compose 中固定的镜像 |
| --- | --- |
| Go Builder | golang:1.26.6-alpine3.23 |
| API 与 Migration Runtime | alpine:3.23 |
| Web Builder | node:24.19.0-alpine3.23 |
| Web Runtime | nginx:1.28.0-alpine3.21 |
| MySQL | mysql:8.4.11 |
| Redis Runtime | redis:7.4.11-alpine3.21 |

运行容器内复核结果：

- Nginx 报告 nginx/1.28.0；
- Redis 报告 Redis server v=7.4.11；
- MySQL 报告 8.4.11 for Linux on aarch64；
- growthos/api、growthos/migrate、growthos/web、growthos/redis 与 mysql 运行镜像均为 linux/arm64；
- 未强制 amd64 模拟，Apple Silicon 上没有依赖 Rosetta 或 QEMU 的架构兜底。

这里验证的是当前选定 tag 对 arm64 可用。镜像仍按 tag 引用而不是 digest 引用，这一供应链局限在本文第 20 节单独列出。

---

## 3. 实现提交与文件清单

### 3.1 已确认的实现提交

| 提交 | 实际主题 | 本节证据用途 |
| --- | --- | --- |
| e746a6f | feat: support file-backed MySQL credentials | API 与 Migration 的密码文件加载、互斥与脱敏 |
| 52c3add | fix: keep MySQL driver failures in structured logs | 每连接配置的 MySQL Driver NopLogger 与日志边界 |
| 7aa6c9e | feat: add isolated Docker Compose development stack | Compose、Dockerfile、Nginx、Secret 脚本、Smoke 与 M0 |

复核命令：

~~~bash
git show --no-patch --format='%h %s' e746a6f
git show --no-patch --format='%h %s' 52c3add
git show --no-patch --format='%h %s' 7aa6c9e
~~~

正文、API、QA、设计手记、面试问答、ADR、Runbook 与全局索引固定在文档内容提交 `ad8078c`；随后的小型清单提交只登记这一事实，不改变实现语义。

### 3.2 主要实现文件

| 文件 | 验收关注点 |
| --- | --- |
| [.dockerignore](../../../.dockerignore) | Secret、node_modules、dist、日志与 VCS 元数据不进入构建上下文 |
| [Makefile](../../../Makefile) | 稳定的 config、build、up、smoke、M0、down 与显式 reset 入口 |
| [Compose 拓扑](../../../deploy/compose/compose.yaml) | 五服务、依赖、网络、Secret、health、volume 与安全属性 |
| [MySQL 初始化脚本](../../../deploy/compose/mysql/init/10-create-growthos-users.sh) | 应用与迁移账号的创建及最小授权 |
| [Secret 目录说明](../../../deploy/compose/secrets/README.md) | 本机权限、完整集合、卷绑定与轮换边界 |
| [Secret 忽略规则](../../../deploy/compose/secrets/.gitignore) | 四个真实秘密不进入 Git |
| [Go 多阶段镜像](../../../deploy/docker/Dockerfile.backend) | API 与 Migration 的原生静态二进制、非 root runtime |
| [Web 多阶段镜像](../../../deploy/docker/Dockerfile.web) | 固定 pnpm 构建与非 root Nginx runtime |
| [Redis 镜像](../../../deploy/docker/Dockerfile.redis) | 固定 Redis 版本和 UID 999 |
| [Redis 启动脚本](../../../deploy/docker/redis-entrypoint.sh) | Secret 转 ACL、临时配置、显式关闭持久化 |
| [Nginx 配置](../../../deploy/docker/nginx.conf) | 同源代理、动态 DNS、头部、缓存和安全日志 |
| [Secret 生成器](../../../scripts/generate-compose-secrets.sh) | complete、repeat、partial 与 volume guard |
| [Compose Smoke](../../../scripts/compose-smoke.sh) | 服务状态、HTTP 契约、request ID 与端口隔离 |
| [healthload](../../../cmd/healthload/main.go) | 固定速率、统计 JSON、错误门禁与可选 P99 门禁 |
| [healthload 测试](../../../cmd/healthload/main_test.go) | 计数、失败返回码、URL 防泄漏与 P99 门禁 |
| [应用配置](../../../internal/platform/appconfig/config.go) | 直接密码与密码文件互斥、大小限制和安全错误 |
| [应用配置测试](../../../internal/platform/appconfig/config_test.go) | API 与 Migration 的独立读取及负向测试 |
| [MySQL Driver 配置](../../../internal/infrastructure/mysql/config.go) | 每配置 NopLogger、TLS 与 DSN 安全边界 |
| [MySQL Driver 测试](../../../internal/infrastructure/mysql/config_test.go) | NopLogger 类型和敏感信息不泄漏 |

---

## 4. Secret 与密码文件验收

### 4.1 API 与 Migration 的读取规则

API 只接受以下二选一：

- GROWTHOS_MYSQL_PASSWORD；
- GROWTHOS_MYSQL_PASSWORD_FILE。

Migration 只接受以下二选一：

- GROWTHOS_MYSQL_MIGRATION_PASSWORD；
- GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE。

验收得到的确定规则：

1. 同一身份的直接密码和文件路径同时设置时失败；
2. 两者都未设置时失败；
3. 文件变量只包含空白时失败；
4. 文件不存在、不可读或读取失败时失败；
5. 密码为空时失败；
6. 密码正文最多 1024 字节；
7. 文件读取使用有界 Reader，不会把任意大文件整体读入内存；
8. 仅移除结尾连续的 CR 与 LF；
9. 除结尾换行外的空白原样保留，不偷偷 TrimSpace 改密码；
10. API Loader 不读取 Migration 密码文件；
11. Migration Loader 不读取 API 密码文件或 HTTP 配置；
12. 错误只指出环境变量类别，不回显密码内容；
13. 文件读取发生在进程启动时，不支持运行中热重载。

相关单元测试覆盖：

- 精确正文；
- 单个 LF；
- CRLF 与重复行结束符；
- 其他前后空白保留；
- 1024 字节上界加 CRLF；
- 空文件；
- 不存在文件；
- 超长文件；
- 直接值和文件同时存在；
- 错误文本不含秘密和秘密路径；
- API 与 Migration 的环境变量相互隔离。

### 4.2 Compose 中的 Secret 分配

| 服务 | 可见 Secret |
| --- | --- |
| api | mysql_app_password |
| migrate | mysql_migration_password |
| redis | redis_password |
| mysql | mysql_root_password、mysql_app_password、mysql_migration_password |
| web | 无 |

运行态 inspect 确认：

- API 只挂载 /run/secrets/mysql_app_password；
- Migration 只挂载 /run/secrets/mysql_migration_password；
- Redis 只挂载 /run/secrets/redis_password；
- Web 没有 Secret mount；
- 所有 Secret mount 都是只读；
- MySQL 因首次初始化与后续认证需要，获得三个 MySQL Secret；
- Compose 展开结果中不包含四个随机值本身。

### 4.3 Secret 生成器四态矩阵

生成器维护以下完整集合：

1. mysql_root_password；
2. mysql_app_password；
3. mysql_migration_password；
4. redis_password。

每个值由 openssl rand -hex 32 生成，为 64 个小写十六进制字符。验收没有把值、散列或截图写入文档。

| 初始状态 | 预期 | 实测结果 |
| --- | --- | --- |
| 四个文件均不存在，目标 MySQL volume 也不存在 | 私有临时目录完整生成/验证后逐文件发布；中断形成部分集合时下次运行 fail closed | 通过 |
| 四个文件全部存在且格式合法 | 只验证，不覆盖 | 通过 |
| 仅存在一至三个文件 | 拒绝补齐，防止错配 | 通过 |
| 四个文件均不存在，但项目 MySQL volume 已存在 | 拒绝生成新密码 | 通过 |
| 文件不是 64 位小写十六进制 | 拒绝继续 | 通过 |

完整生成路径还验证了：

- 先在同目录临时目录中生成和验证；
- 四个文件全部通过后才逐个移动到最终路径；四次移动不是集合级原子事务；
- 发布阶段若中断，可能留下 1～3 个最终文件，但下一次运行会拒绝补齐而不会把它当作完整集合；
- 异常退出通过 trap 清理临时目录；
- umask 为 077；
- 最终宿主目录模式为 0700；
- 最终四个文件模式为 0444。

重复运行路径还验证了：

- 文件内容没有被覆盖；
- 文件集合没有被部分更新；
- 权限被规范为约定模式；
- 输出只报告目录和状态，不输出 Secret。

partial 路径还验证了：

- 生成器返回非零；
- 明确要求恢复缺失文件或在确认数据卷后处理整套凭据；
- 不为剩余文件生成新值；
- 不删除已有文件。

volume guard 路径还验证了：

- 检查的卷名由 Compose project 加 mysql_data 组成；
- 已有卷但整套 Secret 丢失时返回非零；
- 不创建新 Secret；
- 不删除旧卷；
- 测试专用 GROWTHOS_COMPOSE_SKIP_VOLUME_CHECK 只用于隔离测试，不是日常恢复方案。

### 4.4 真实发现：0600 Secret 在非 root 容器中不可读

最初设计把 Secret 文件设为 0600，直觉上看比 0444 更严格，但 Docker Desktop 的本地 Compose Secret 实际表现为容器内 root 所有的只读 bind mount。

真实失败链路为：

1. 宿主机文件是 0600；
2. API、Migration、Redis 分别以 UID 65532、65532、999 运行；
3. 容器内文件所有者不是这些运行用户；
4. 非 root 进程不能读取文件；
5. 服务以稳定的 secret unavailable 类错误退出，而不是假装健康。

修复不是把进程改回 root，而是：

- 宿主机 Secret 目录保持 0700，只有当前宿主用户能遍历；
- 文件设为 0444，解决容器内非 root 读取；
- 每个服务仍只挂载自己声明的 Secret；
- Secret 继续被 Git 和 Docker build context 排除；
- 文档明确这是 Docker Desktop 本地开发兼容策略，不等于生产 Secret Manager。

修复后运行态权限复核：

~~~text
deploy/compose/secrets                         0700
deploy/compose/secrets/mysql_root_password     0444
deploy/compose/secrets/mysql_app_password      0444
deploy/compose/secrets/mysql_migration_password 0444
deploy/compose/secrets/redis_password          0444
~~~

这次问题说明，权限数字必须结合挂载实现、容器 UID 和目录遍历权限判断，不能只比较 0600 与 0444 哪个“看起来更安全”。

### 4.5 泄漏边界

验收确认以下位置没有出现四个真实值：

- Git 跟踪文件；
- Docker build context；
- docker compose config 输出；
- 自定义镜像 history；
- API 与 Migration 配置错误；
- MySQL Driver 错误；
- Web access log 与 error log；
- Smoke 与 healthload JSON。

文档也不记录秘密的散列。随机值的散列虽然不等于明文，但没有必要成为长期关联标识。

---

## 5. Compose 配置、构建与正常态

### 5.1 可复现入口

正常路径：

~~~bash
make compose-config
make compose-build
make compose-up
make compose-ps
make compose-smoke
~~~

对应语义：

- compose-config 先生成或验证完整 Secret，再执行 Compose 模型校验；
- compose-build 构建 API、Migration、Web 和 Redis 的本地镜像；
- compose-up 使用 detach、build、wait，并设置 180 秒等待上限；
- compose-ps 同时展示长驻服务 health 和 Migration 的一次性退出状态；
- compose-smoke 做只读容器、HTTP 和端口检查。

### 5.2 构建结果

真实构建覆盖：

- growthos/api:lesson-16；
- growthos/migrate:lesson-16；
- growthos/web:lesson-16；
- growthos/redis:7.4.11；
- 官方 mysql:8.4.11。

Go 构建阶段确认：

- 使用多阶段构建；
- go mod download 后执行 go mod verify；
- CGO_ENABLED=0；
- GOOS 与 GOARCH 来自 BuildKit 目标平台；
- 使用 trimpath；
- 不嵌入 VCS 工作区信息；
- 去除调试符号；
- API 与 Migration 复用 builder，但输出不同 target；
- Runtime 不包含 Go 编译器和源代码；
- 最终用户为 65532:65532。

Web 构建阶段确认：

- Node 与 pnpm 版本固定；
- 依赖安装使用 frozen lockfile；
- BuildKit cache 只加速依赖，不进入最终镜像；
- 最终镜像只包含 Nginx、配置和 dist；
- 最终用户为 101:101。

Redis 构建确认：

- 版本固定为 7.4.11-alpine3.21；
- 启动脚本只读 Secret 并在 tmpfs 生成 ACL 与 redis.conf；
- 最终用户为 999:999；
- 密码不出现在镜像层、命令行参数或 Compose environment。

### 5.3 正常运行态

验收时正常态为：

| 服务 | 状态 |
| --- | --- |
| mysql | running，healthy |
| migrate | exited，exit code 0 |
| api | running，healthy |
| redis | running，healthy |
| web | running，healthy |

依赖顺序实际生效：

1. MySQL 启动；
2. MySQL 使用 growthos_app 执行真实 SELECT 1 健康检查；
3. Migration 在 MySQL healthy 后启动；
4. Migration 安全识别当前空迁移集为 no_migrations，并以 0 退出；
5. API 在 Migration service_completed_successfully 且 MySQL healthy 后启动；
6. Web 没有等待 API，能够独立提供静态故障页面；
7. Redis 没有被伪装成 API 依赖，独立启动。

Smoke 通过的 HTTP 契约：

- /health 返回 200 与合法 JSON；
- /ready 返回 200 与合法 JSON；
- / 返回 200 HTML；
- 缺失 API 路由返回 404 JSON；
- 404 body 的 request_id 与 X-Request-ID 相同；
- Migration 必须是 exited 0；
- 四个长驻服务必须 running 且 healthy；
- 只有 Web 发布预期回环端口。

---

## 6. 端口与网络隔离

### 6.1 宿主机端口

运行态 inspect 结果：

| 服务 | 容器端口 | 宿主机发布 |
| --- | --- | --- |
| web | 8080/tcp | 127.0.0.1:8088 |
| api | 8080/tcp | 无 |
| mysql | 3306/tcp、33060/tcp | 无 |
| redis | 6379/tcp | 无 |
| migrate | 无 | 无 |

API 的 expose 只表达容器网络内端口，不等于 publish。MySQL 和 Redis 在 docker ps 中出现容器端口，也不等于宿主机可以连接。

Smoke 对发布端口使用 inspect，而不是只凭 compose.yaml 肉眼判断：

~~~text
web: 8080/tcp 127.0.0.1 8088
api/mysql/redis/migrate: no published bindings
~~~

因此浏览器入口只有：

~~~text
http://127.0.0.1:8088
~~~

回环绑定降低了局域网暴露，但不提供认证、TLS 或租户隔离。

### 6.2 三张网络

| 网络 | internal | 成员 | 目的 |
| --- | --- | --- | --- |
| growthos_edge | false | web、api | Web 到 API，同时时 Web 能发布回环端口 |
| growthos_data | true | api、mysql；运行 Migration 时还包括 migrate | 数据访问边界 |
| growthos_cache | true | redis | 为后续缓存集成保留的隔离边界 |

运行态 docker network inspect 确认：

- edge 只有 Web 与 API；
- data 只有当前长驻 API 与 MySQL；
- cache 只有 Redis；
- data 与 cache 的 Internal 均为 true；
- Web 不在 data；
- API 不在 cache；
- Redis 不在 edge 或 data；
- MySQL 不在 edge；
- 项目没有显式 container_name，名称由 Compose project 隔离。

Migration 完成并退出后可能不再显示为 network inspect 的活动端点；其 Compose 声明仍只有 data，运行迁移时按此连接。

这不是完整的网络防火墙：

- edge 必须非 internal，Web 才能发布宿主机入口；
- 同一网络成员之间仍可通信；
- 网络隔离不能代替数据库账号权限；
- 本节没有对 Docker daemon 或宿主机管理员建立防护。

---

## 7. 运行身份与文件系统边界

### 7.1 四个可收紧服务

运行态 inspect 的精确结论：

| 服务 | User | Read-only RootFS | CapDrop | no-new-privileges | init |
| --- | --- | --- | --- | --- | --- |
| api | 65532:65532 | true | ALL | true | true |
| migrate | 65532:65532 | true | ALL | true | true |
| web | 101:101 | true | ALL | true | true |
| redis | 999:999 | true | ALL | true | true |

每个服务显式 restart: no，符合可观察、可教学的本地开发策略。

可写位置受限为：

- API：16 MiB /tmp tmpfs；
- Migration：16 MiB /tmp tmpfs；
- Web：16 MiB /tmp tmpfs；
- Redis：16 MiB /tmp，加 64 MiB /data tmpfs；
- tmpfs 带 noexec 和 nosuid；
- Redis /data 归 UID/GID 999，模式 0700。

Nginx 的 pid、client body temp、proxy temp 等路径均指向 /tmp，避免只读根文件系统下启动失败。

Redis 显式配置：

- protected-mode yes；
- ACL 文件在 tmpfs；
- save 为空；
- appendonly no；
- /data 为 tmpfs。

因此本节 Redis 是易失、隔离、带认证的开发占位服务，不是持久化业务缓存。

### 7.2 MySQL 官方镜像例外

MySQL 的 inspect 结果不同：

| 属性 | 实际值 |
| --- | --- |
| Compose User | 未覆盖，官方 entrypoint 先按镜像语义启动 |
| Read-only RootFS | false |
| CapDrop | 未设置 |
| no-new-privileges | 未设置 |
| init | 未设置 |
| restart | no |
| 数据目录 | 可写 named volume |

长期 mysqld 进程由 docker top 验证为 UID 999、GID 999，但官方 entrypoint 的初始化阶段需要 root 语义与可写数据目录。

因此正确结论是：

- API、Migration、Web、Redis 具备本节声明的完整收紧项；
- MySQL 长期数据库进程降权为 999；
- MySQL 容器本身不是只读、无能力、no-new-privileges 的同等边界；
- 这是已知例外，而不是遗漏后用一句“全部非 root”掩盖。

如果未来要进一步收紧，需要先验证官方 entrypoint、数据目录所有权、升级路径和恢复工具，而不能直接复制其他服务的安全属性。

---

## 8. MySQL 数据、身份与最小权限

### 8.1 数据卷生命周期

本节只有一个 named volume：

~~~text
growthos_mysql_data -> /var/lib/mysql
~~~

日常命令语义：

- make compose-down 删除本项目容器和网络，但保留 volume；
- make compose-reset 必须显式提供 CONFIRM=reset-growthos-data；
- reset 才删除本项目 volume；
- Redis 不使用 named volume；
- Secret 文件也不会被 compose-down 删除。

Secret 和 MySQL volume 是一组有状态资产：删除其中一边、重新生成另一边，会导致已存账号密码与文件不匹配。volume guard 正是为阻止这种隐式漂移。

### 8.2 精确授权

应用账号最终授权：

~~~sql
GRANT USAGE ON *.* TO 'growthos_app'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE
ON growthos.* TO 'growthos_app'@'%';
~~~

迁移账号最终授权：

~~~sql
GRANT USAGE ON *.* TO 'growthos_migrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE,
      CREATE, DROP, REFERENCES, INDEX, ALTER
ON growthos.* TO 'growthos_migrator'@'%';
~~~

负向边界：

- API 账号没有 CREATE、ALTER、DROP、INDEX 或 REFERENCES；
- API 账号没有 EXECUTE；
- Migration 账号没有全局 ALL PRIVILEGES；
- 两个账号都没有 GRANT OPTION；
- 两个账号都没有 root Secret；
- Migration Secret 不进入 API；
- API Secret 不进入 Migration。

### 8.3 真实发现：旧卷不会重放 init 脚本

首次实现检查发现，已有开发卷仍保留过宽授权：应用账号出现不需要的 EXECUTE，迁移账号接近 ALL。

原因不是 Compose 没加载新脚本，而是 MySQL 官方镜像只在空数据目录执行 docker-entrypoint-initdb.d。修改初始化脚本不会自动迁移已有账号权限。

修复与验收分为两层：

1. 修改初始化脚本为上面的显式最小权限集合；
2. 在隔离 Compose project 和全新临时 volume 中从零重放；
3. 等待 MySQL 健康；
4. 验证两个用户的 SHOW GRANTS；
5. 验证 Migration exit code 0；
6. 验证 API/ready 正常；
7. 删除只为重放创建的临时项目与临时 volume；
8. 对当前 GrowthOS 数据卷显式收敛授权并再次检查。

全新 volume 重放结果与当前运行卷结果一致：

- growthos_app 只有四项 DML；
- growthos_migrator 只有四项 DML 加五项 DDL/索引相关权限；
- 不存在全局 ALL；
- Migration 对当前空迁移集返回 no_migrations 并成功退出；
- 应用探针成功。

这一验收不可用“脚本内容看起来正确”替代，因为 init-once 语义会让代码与旧运行卷产生真实偏差。

---

## 9. Nginx 路由、头部、DNS 与日志

### 9.1 同源路由

Web 在同一 origin 提供：

| 路径 | 处理方式 |
| --- | --- |
| /container-health | Nginx 本地 204 |
| /health | 代理 API |
| /ready | 代理 API |
| /api 及其子路径 | 代理 API |
| /assets | 静态资源，不存在即 404 |
| /index.html | SPA 入口 |
| 其他前端路径 | try_files 后 fallback 到 index.html |

验收确认：

- 深链接 /system/status 可直接刷新；
- API 路由不会被 SPA fallback 吞掉；
- 浏览器不需要跨域配置；
- Web 在 API 中断时仍可加载静态页面。

### 9.2 Docker DNS 动态解析

配置使用：

~~~nginx
resolver 127.0.0.11 valid=5s ipv6=off;
set $growthos_api_origin http://api:8080;
proxy_pass $growthos_api_origin$request_uri;
~~~

这使 Nginx 使用 Docker 内嵌 DNS 解析稳定服务名 api，并在 API 容器重建、IP 改变后重新解析，而不是把启动时 IP 固定到 worker 生命周期。

运行态故障恢复验证见第 12 节：

- API 停止时返回真实 502；
- API 重新创建后 IP 可以改变；
- Web 容器不重启；
- 5 秒 DNS 有效期后代理恢复；
- /health 与 /ready 再次返回 200。

### 9.3 安全头与缓存头

验收的安全头：

- X-Content-Type-Options: nosniff；
- X-Frame-Options: DENY；
- Referrer-Policy: strict-origin-when-cross-origin；
- X-Request-ID: 非空且每个响应只有一个最终值。

验收的缓存规则：

- /container-health 为 no-store；
- API 健康与就绪响应保持 no-store；
- index.html 为 no-store；
- hashed assets 为 public, max-age=31536000, immutable；
- asset 响应没有重复 Cache-Control；
- 404 与 502 也保留需要的安全头和 request ID。

### 9.4 真实发现：add_header 继承与重复缓存头

初版在 server 层声明安全头，又在子 location 中声明 Cache-Control。

Nginx 的规则是：当前层只要出现任意 add_header，就不会自动继承上一层的 add_header 集合。真实结果是 assets、index 和 container-health 等位置会丢失父层安全头。

同时，初版 assets 组合 expires 与 add_header Cache-Control，导致重复缓存头。

修复方式：

- 在有局部 add_header 的 location 中显式重复安全头；
- assets 只保留一个 Cache-Control 生成来源；
- index 和 container-health 明确 no-store；
- 重新构建 Web 镜像；
- 对静态入口、asset、探针、404 与故障响应逐一检查响应头。

最终结果：

- 子 location 不再因继承规则丢安全头；
- asset 只有一个长期缓存策略；
- HTML 与探针不会被长期缓存；
- 所有被测响应只有一个 X-Request-ID。

### 9.5 安全结构化 access log

Nginx 当前采用便于检索的 key=value 结构化行，不是 JSON；不能因为 Docker 使用 json-file logging driver 就把应用原始行误称为 JSON。逐行 JSON 的约束适用于 GrowthOS API 与 Migration 的 Go 日志，MySQL 故障重放后已确认 API 原始日志每行均可解析为 JSON。官方 MySQL、Redis 与 Nginx 自身仍保留各自的原生日志格式，Docker json-file 只负责容器日志封装和轮转。

Nginx access log 只记录：

- remote address；
- 最终 request ID；
- time；
- HTTP method；
- 规范化 path；
- protocol；
- status；
- upstream_status；
- response bytes；
- request_time；
- upstream_time。

明确不记录：

- args 或 request_uri 的查询字符串部分；
- Referer；
- Authorization；
- Cookie；
- request body；
- Secret 文件内容。

正常 API 响应的关联规则：

1. API 生成或规范化最终 X-Request-ID；
2. Nginx 隐藏上游原始响应头，避免重复；
3. map 优先选择 upstream X-Request-ID；
4. Nginx 用同一个值输出响应头；
5. access log 用同一个值记录 request_id；
6. API JSON 日志可用该值关联。

静态或 gateway 响应没有上游 ID 时，map 回退到 Nginx 自己的 request_id。

### 9.6 真实发现：Nginx error_log 泄漏请求目标

仅修改 access log 不足以完成脱敏。真实 API-down 测试中，Nginx notice/error 行仍可能附带原始请求 target 与 Referer；这会绕过自定义 access log，泄漏查询参数中的 token、授权码、筛选条件或 PII。

修复方式：

- error_log 提升到 crit；
- 请求级上游诊断放到经过裁剪的 access log；
- access log 增加 upstream_status；
- 以两个唯一、非秘密的 query/Referer marker 重放 API-down 请求；
- 扫描 Web stdout 与 stderr；
- 确认两个 marker 都没有出现；
- 确认安全 access log 仍有 path、status、upstream_status、耗时和 request ID。

这里的结论限定为已测试路径：当前 API-down 502 不泄漏 query 或 Referer。它不是对所有未来 Nginx 模块和所有 crit 级错误格式的形式化证明。

### 9.7 真实发现：Nginx 生成的 502 缺少 X-Request-ID

初版仅依赖上游 API 返回 X-Request-ID。当 API 根本不可达时，没有上游响应头，最终 Nginx 502 缺少关联 ID。

修复后：

- map 在上游 ID 为空时回退到 Nginx request_id；
- add_header X-Request-ID 使用 always；
- proxy_hide_header 防止正常响应出现重复头；
- 502 response header 得到非空 ID；
- 同一次请求的安全 access log 记录相同 ID；
- query 和 Referer marker 仍未进入日志。

最终 502 验收同时满足：

1. HTTP status 为 502；
2. X-Request-ID 恰好一个；
3. ID 非空；
4. access log status 为 502；
5. access log upstream_status 可见；
6. access log request_id 与响应头相同；
7. query marker 不存在于日志；
8. Referer marker 不存在于日志。

---

## 10. MySQL 故障与恢复

### 10.1 注入前基线

注入前记录：

- API 容器 ID 与 StartedAt；
- Web 容器 ID 与 StartedAt；
- /health 为 200；
- /ready 为 200；
- MySQL healthy；
- 末次 smoke 通过。

复现时可使用：

~~~bash
docker compose --project-name growthos --file deploy/compose/compose.yaml ps -q api
docker compose --project-name growthos --file deploy/compose/compose.yaml ps -q web
docker inspect --format '{{.Id}} {{.State.StartedAt}}' growthos-api-1
docker inspect --format '{{.Id}} {{.State.StartedAt}}' growthos-web-1
~~~

### 10.2 故障态

只停止 GrowthOS 项目的 MySQL 服务，不停止用户原有 MySQL 容器，也不执行 down 或 volume 删除。

观察结果：

| 检查 | 结果 |
| --- | --- |
| API 容器 | 继续运行，ID 与 StartedAt 未变 |
| Web 容器 | 继续运行，ID 与 StartedAt 未变 |
| /health | 200 |
| /ready | 503 |
| /ready body | 稳定、安全的依赖未就绪 JSON |
| Cache-Control | no-store |
| X-Request-ID | 非空且可关联 |
| 静态 SPA | 仍为 200 |

/health 保持 200 是故意的：进程和 HTTP server 仍存活。/ready 变为 503 才表示依赖不可用。若两个探针都失败，编排层无法区分“进程死了”和“数据库暂时不可用”。

### 10.3 恢复态

显式启动目标 MySQL 容器并等待 healthy 后：

- /ready 自动恢复为 200；
- API 容器 ID 与 StartedAt 仍未改变；
- Web 容器 ID 与 StartedAt 仍未改变；
- database/sql 连接池淘汰坏连接后重新建立连接；
- 最终 smoke 再次通过；
- 没有删除 MySQL volume；
- 没有重新生成 Secret。

由于 restart 策略为 no，这里的“恢复”是操作者显式启动目标服务，不是自动自愈。

### 10.4 真实发现：MySQL Driver 原始 stderr

首次数据库重启演练出现类似 MySQL Driver closing bad idle connection 的原始 stderr 行。该行：

- 不是 JSON；
- 绕过 GrowthOS slog；
- 可能暴露 Driver 内部 cause；
- 破坏“容器日志每行均为结构化 JSON”的边界。

修复在每个 drivermysql.Config 上设置 NopLogger：

- 不调用全局 SetLogger；
- 不引入跨测试或跨连接的全局竞态；
- API readiness 和生命周期日志继续输出安全的业务级信号；
- Driver 内部原始行不再直接写 stderr。

故障重放后：

- API 原始日志逐行可解析为 JSON；
- 不再出现 MySQL Driver 前缀原始行；
- 不出现密码、DSN 或 Secret 路径；
- /ready 的 503 与恢复后的 200 仍可通过结构化日志观察。

---

## 11. API 故障、动态 DNS 与恢复

### 11.1 故障态

只停止 API，保持 Web 运行。

结果：

| 检查 | 结果 |
| --- | --- |
| Web 容器 | ID 与 StartedAt 未变 |
| Web health | healthy |
| / | 200，静态 SPA 可加载 |
| /system/status | 200，SPA fallback 正常 |
| /health | 502 |
| /ready | 502 |
| 前端状态 | gateway/offline，不保留伪绿色 |
| 502 X-Request-ID | 非空 |
| 502 access log | 与响应头 ID 相同 |

Web 没有 depends_on API，正是为了让依赖故障时仍能展示诊断页面。

### 11.2 查询参数与 Referer 脱敏复测

在 API 停止期间发出带唯一 query marker 和唯一 Referer marker 的请求。

检查范围：

- Web stdout；
- Web stderr；
- Nginx access log；
- Nginx error log。

结果：

- 查询参数 marker 出现次数为 0；
- Referer marker 出现次数为 0；
- 安全 access log 仍记录规范化 /health 或 /ready path；
- status 为 502；
- upstream_status 可用于判断 gateway failure；
- request_id 可与客户端响应头关联。

### 11.3 恢复态

恢复时只启动或重建 API：

- Web 没有重启；
- API 重新加入 edge 和 data；
- 容器 IP 允许变化；
- Nginx 通过 127.0.0.11 重新解析 api；
- /health 恢复 200；
- /ready 恢复 200；
- 404 API JSON 与 request ID 契约恢复；
- 末尾 smoke 通过。

这证明恢复依赖服务名和 DNS，不依赖把 API IP 写入配置，也不依赖重启 Web。

---

## 12. Redis 故障与恢复

### 12.1 故障态

只停止 Redis。

结果：

| 检查 | 结果 |
| --- | --- |
| API 容器 | ID 与 StartedAt 未变 |
| Web 容器 | ID 与 StartedAt 未变 |
| /health | 200 |
| /ready | 200 |
| Web | healthy |
| MySQL | healthy |

这不是“Redis 故障被系统容错”的证据，而是“第 16 节 Redis 尚未接入业务依赖”的证据。若 API readiness 因 Redis 停止而失败，反而说明 Compose 声明与当前代码依赖不一致。

### 12.2 恢复态

显式启动 Redis 后：

- Redis health 恢复为 healthy；
- API 与 Web 不重启；
- /health 与 /ready 继续为 200；
- Redis 数据不要求恢复，因为 /data 是 tmpfs 且 RDB/AOF 均关闭；
- 最终 smoke 通过。

restart: no 同样意味着恢复是显式操作。Redis 未来接入业务后，必须重新定义数据丢失语义、readiness 影响和恢复测试。

---

## 13. 自动化测试与静态门禁

### 13.1 Go 配置与 Driver

本节相关测试验证：

- API 和 Migration 的文件 Secret 正常读取；
- 直接密码与文件密码互斥；
- 空、不可读、超长文件失败；
- CRLF 只从结尾移除；
- 错误不泄漏 Secret 或路径；
- API 与 Migration Loader 相互隔离；
- Config 的 String、GoString、LogValue 和 JSON 均脱敏；
- MySQL Driver 使用每配置 NopLogger；
- DSN、密码和 CA 私有路径不进入错误；
- race detector 路径通过。

代表命令：

~~~bash
go test ./internal/platform/appconfig
go test -race ./internal/platform/appconfig
go test ./internal/infrastructure/mysql
go test -race ./internal/infrastructure/mysql
~~~

### 13.2 healthload

测试覆盖：

- 固定速率调度；
- scheduled、completed、success、errors、unexpected_status 与 dropped 计数；
- 超时和取消；
- 非预期状态导致非零退出；
- 请求响应正文不进入输出；
- 带 query 的目标 URL 被拒绝；
- P99 上限为负或过大时被拒绝；
- P99 超限时 JSON 标记且命令失败；
- P99 通过时命令成功；
- 输出保持单行 JSON。

代表命令：

~~~bash
go test ./cmd/healthload
go test -race ./cmd/healthload
~~~

### 13.3 Shell 与 Compose

检查范围：

- generate-compose-secrets.sh；
- compose-smoke.sh；
- mysql init 脚本；
- redis entrypoint；
- Compose 配置展开；
- 端口和网络运行态。

代表命令：

~~~bash
sh -n scripts/generate-compose-secrets.sh
sh -n scripts/compose-smoke.sh
sh -n deploy/compose/mysql/init/10-create-growthos-users.sh
sh -n deploy/docker/redis-entrypoint.sh
shellcheck scripts/generate-compose-secrets.sh
shellcheck scripts/compose-smoke.sh
shellcheck deploy/compose/mysql/init/10-create-growthos-users.sh
shellcheck deploy/docker/redis-entrypoint.sh
make compose-config
make compose-smoke
~~~

### 13.4 最终全仓门禁

所有第 16 节正文、API、QA、设计手记、面试问答、ADR、Runbook 与上层索引落盘后，执行：

~~~bash
make verify
~~~

最终退出码为 0，覆盖：

- Go 格式检查；
- `go vet ./...`；
- `go test ./...`，包含 `cmd/healthload`、配置、MySQL、HTTP 与既有工具包；
- `go run ./cmd/doccheck`，结果为 `documentation checks passed`；
- 4 个 Vitest 文件、34 项前端测试全部通过；
- TypeScript `tsc --noEmit` 通过；
- Vite 生产构建通过。

Vite 仍报告主 JavaScript chunk 超过 500 kB 的非阻断 warning；这项前端体积债务没有被删除或包装成“无警告”。文档内容与这次门禁记录随后固定为 `ad8078c`；包含该哈希登记的最终清单提交仍以分支 tip 标识，避免虚构自引用哈希。

---

## 14. 浏览器与可访问性验收

### 14.1 正常宽屏

访问：

~~~text
http://127.0.0.1:8088/system/status
~~~

检查结果：

- 页面通过同源 Nginx 正常加载；
- 健康与就绪状态都显示成功；
- 不出现跨域错误；
- 结构、卡片、状态文案和 request ID 区域可读；
- 深链接刷新不返回 Nginx 404；
- 没有未捕获异常或错误覆盖层；
- axe 4.12.1 自动扫描为 0 violation、0 incomplete、21 pass。

### 14.2 移动窄屏

在 `390 × 844` 视口检查，结果为：

- 卡片和操作区域按窄屏重排；
- 没有页面级横向溢出；
- 长 request ID 能换行或被容器约束；
- 交互控件可见且可触达；
- 状态不只依赖颜色表达；
- axe 自动扫描为 0 violation。

### 14.3 深色模式

深色模式与 `390 × 844` 移动视口组合检查，结果为：

- 背景、文字、边框和状态色对比可读；
- healthy、degraded、gateway/offline 状态不因暗色 token 丢失；
- 键盘焦点和可操作元素仍可辨认；
- 重新加载后仍能得到真实 API 状态；
- axe 自动扫描为 0 violation。

### 14.4 MySQL 不可用

浏览器观察与 HTTP 证据一致：

- 页面本身仍可加载；
- health 显示进程存活；
- ready 显示依赖未就绪；
- 不保留上一次成功的伪绿色；
- 恢复 MySQL 后无需刷新容器即可重新获得 ready；
- axe 自动扫描为 0 violation。

### 14.5 API 离线

浏览器观察：

- 静态页面仍加载；
- health 与 ready 都进入 gateway/offline 状态；
- 页面不把 502 当成 JSON 业务错误；
- 不显示旧的成功结果；
- API 恢复后再次请求可恢复；
- 预期的失败网络请求不会造成未捕获页面异常；
- axe 自动扫描为 0 violation。

### 14.6 浏览器网络离线

在页面已经加载后切断浏览器网络，两个探针都显示“无法连接 API”，而不是 MySQL readiness 失败或旧的成功状态；页面布局、导航和本地静态内容仍可读，无未捕获异常。恢复网络后重新触发请求可以回到真实服务状态。

这一场景与“API 容器停止”不同：前者没有任何 HTTP response，统一 Client 进入 `network`；后者仍能从 Nginx 获得带关联 ID 的 502，进入 `gateway`。把两者分开验证，避免 UI 用一个笼统的 offline 文案掩盖排障层次。

axe 的“0 violation”只表示本次页面状态和规则集未发现自动化违规，不代表人工可用性测试、所有辅助技术组合或未来页面都已证明无障碍。

---

## 15. 正式 M0 门禁

### 15.1 固定仓库入口与本轮等价执行

仓库提供的固定复验入口是：

~~~bash
make compose-m0
~~~

该 recipe 固定执行：

1. compose-up；
2. compose-smoke；
3. /health：100 RPS、5 分钟、32 workers、2 秒 timeout、P99 上限 100 ms；
4. /ready：20 RPS、30 秒、32 workers、2 秒 timeout；
5. 末尾再次 compose-smoke。

recipe 直接写死 M0 参数，避免通过外部 Make 变量把 5 分钟门禁缩短后仍声称完成 M0。

本轮为了在五分钟窗口前后分别保存资源快照和中间证据，没有把一次 `make compose-m0` 的整段输出当作黑盒；实际执行的是与 recipe 等价的可观察步骤：已构建并启动栈、前置 smoke、下面两个精确 `go run ./cmd/healthload` 命令、末尾 smoke。两个负载命令的 rate、duration、workers、timeout 和 P99 门槛与 Make recipe 完全一致。因而本节声称的是“固定 M0 参数与完整前后检查已实际执行”，不是虚构某条未直接运行的 shell 历史。

~~~bash
go run ./cmd/healthload -url http://127.0.0.1:8088/health -rate 100 -duration 5m -workers 32 -timeout 2s -max-p99 100ms
go run ./cmd/healthload -url http://127.0.0.1:8088/ready -rate 20 -duration 30s -workers 32 -timeout 2s
~~~

### 15.2 /health 精确结果

以下为正式单行 JSON 中与门禁有关的精确字段：

~~~json
{
  "target": "http://127.0.0.1:8088/health",
  "rate": 100,
  "duration_ms": 300000,
  "workers": 32,
  "timeout_ms": 2000,
  "expected_status": 200,
  "scheduled": 30000,
  "completed": 30000,
  "success": 30000,
  "errors": 0,
  "unexpected_status": 0,
  "dropped": 0,
  "status_counts": {
    "200": 30000
  },
  "actual_rps": 100.0026697,
  "p50_ms": 1.084208,
  "p95_ms": 2.744875,
  "p99_ms": 4.1495,
  "max_ms": 18.116291,
  "max_p99_ms": 100,
  "p99_limit_exceeded": false
}
~~~

判定：

- scheduled 与 completed 完全相等；
- 30000 次全部成功；
- errors 为 0；
- unexpected_status 为 0；
- dropped 为 0；
- 实际速率 100.0026697 RPS；
- P99 为 4.1495 ms；
- P99 低于 100 ms 门禁；
- healthload 退出成功。

### 15.3 /ready 精确结果

以下为正式单行 JSON 中已长期记录的精确字段：

~~~json
{
  "target": "http://127.0.0.1:8088/ready",
  "rate": 20,
  "duration_ms": 30000,
  "workers": 32,
  "timeout_ms": 2000,
  "expected_status": 200,
  "scheduled": 600,
  "completed": 600,
  "success": 600,
  "errors": 0,
  "unexpected_status": 0,
  "dropped": 0,
  "status_counts": {
    "200": 600
  },
  "actual_rps": 20.0276841,
  "p50_ms": 4.08525,
  "p95_ms": 5.935083,
  "p99_ms": 6.841375,
  "max_ms": 8.570541,
  "p99_limit_exceeded": false
}
~~~

判定：

- scheduled 与 completed 完全相等；
- 600 次全部成功；
- errors、unexpected_status 与 dropped 均为 0；
- 实际速率 20.0276841 RPS；
- P50 为 4.08525 ms，P95 为 5.935083 ms；
- P99 为 6.841375 ms；
- max 为 8.570541 ms；
- healthload 退出成功。

未在本记录中填写的 started_at、finished_at、min、ready P50/P95 等瞬时字段，不以猜测补齐。

### 15.4 M0 解释边界

该 M0 证明：

- 当前 Apple Silicon 开发机；
- 当前 Docker Desktop 配额；
- 当前单实例 Compose 拓扑；
- 通过 Web/Nginx 同源入口；
- /health 在固定 100 RPS、5 分钟下无错误且 P99 通过 100 ms；
- /ready 在固定 20 RPS、30 秒下无错误；
- 压测后完整 smoke 仍通过。

该 M0 不证明：

- 业务写路径吞吐；
- 多租户容量；
- MySQL 复杂查询性能；
- 峰值容量；
- 长时间稳定性或内存无泄漏；
- 多机网络表现；
- 生产 SLA；
- 自动扩缩容；
- 故障期间请求零丢失。

---

## 16. 资源快照

32 workers readiness 复测与最终 smoke 后的 docker stats 瞬时内存快照：

| 服务 | 内存 |
| --- | ---: |
| API | 6.664 MiB |
| Web | 5.535 MiB |
| MySQL | 438 MiB |
| Redis | 23.41 MiB |
| Docker Desktop 配额 | 1.924 GiB |

Migration 已成功退出，因此不在长驻进程快照中。

正确解读：

- MySQL 是当前主要内存消费者；
- API、Web、Redis 在该时刻均远低于 MySQL；
- allocator/cache 会让相邻取样明显变化，尤其不能用一次 Redis 快照推导稳定基线；
- 数值是最终复测后的瞬时值，不是峰值；
- Compose 没有设置资源 limits，因此这不是配额执行证据；
- 没有长时间采样，因此不能用于证明无泄漏；
- Docker Desktop 的 1.924 GiB 是当前配额背景，不是本项目实际总占用。

---

## 17. 真实发现与修复回归总表

| 优先级 | 真实问题 | 风险 | 修复 | 回归证据 |
| --- | --- | --- | --- | --- |
| P1 | 0600 Secret 在 Docker Desktop bind-secret 中归 root，非 root 服务不可读 | API、Migration、Redis 无法启动 | 目录 0700、文件 0444、按服务最小挂载 | 三服务非 root 启动，Secret 不泄漏 |
| P1 | 子 location 的 add_header 打断父层继承 | 静态资源或健康端点缺安全头 | 子 location 显式补齐安全头 | entry、asset、health、404、502 逐项验证 |
| P1 | expires 与 add_header 叠加 | 重复 Cache-Control，缓存语义含混 | 只保留单一来源 | asset 只有一条 immutable 策略 |
| P1 | MySQL Driver 默认 Logger 写原始 stderr | 破坏 JSON 日志边界，可能暴露底层 cause | 每个 Driver Config 使用 NopLogger | MySQL 故障重放后 API 每行 JSON |
| P1 | Nginx error_log 仍带 raw target/Referer | query 或 Referer 中敏感数据泄漏 | error_log=crit，安全 access log 加 upstream_status | API-down marker 扫描均为 0 |
| P1 | Nginx 自生成 502 没有上游 request ID | 客户端无法关联 gateway 故障 | map 回退到 Nginx request_id，always 输出 | 502 header 与 access log ID 相同 |
| P1 | 初始 MySQL 授权过宽且旧卷不重放 init | API 可拥有不需要权限，代码与运行卷漂移 | 显式 grants、全新 volume replay、当前卷收敛 | 两身份 SHOW GRANTS 精确匹配 |
| P1 | Migration stop grace 初始短于锁等待预算 | 停止时可能被强杀，留下难解释迁移状态 | stop_grace_period 提升为 50s | 默认 lock 40s、statement 30s 有退出余量 |
| P1 | 初版 M0 没有机器执行 P99 阈值 | 可记录延迟但不会因超限失败 | healthload 增加 max-p99 | 正式 P99 4.1495ms，100ms gate false |
| P1 | 负载结束后缺少完整终态检查 | 压测可能结束但服务已退化 | compose-m0 末尾再次 smoke | 最终正常态全部通过 |

这些问题不是文档中的假设题，而是实现和运行验收中真实暴露、修复并重放的缺陷。

---

## 18. 故障恢复不重启矩阵

| 故障目标 | 必须恢复的目标 | 不应重启 | 实测 |
| --- | --- | --- | --- |
| MySQL | MySQL | API、Web | API/Web ID 与 StartedAt 不变，ready 自恢复 |
| API | API | Web、MySQL、Redis | Web ID 与 StartedAt 不变，动态 DNS 恢复 |
| Redis | Redis | API、Web、MySQL | API/Web 不变，health/ready 始终 200 |

检查容器 ID 与 StartedAt 的意义：

- 仅看到最终 HTTP 200，不能证明没有通过重启整个栈恢复；
- ID 不变可排除容器替换；
- StartedAt 不变可排除同容器 stop/start；
- 对故障目标本身允许且需要显式启动或重建；
- 对无关服务保持不变，才证明依赖和恢复边界有效。

本节没有声称自动自愈，因为所有服务 restart 都是 no。该选择让本地故障更可见，也意味着操作者必须显式恢复目标。

---

## 19. 验收后状态与清理

### 19.1 保留

以下资产有意保留：

- GrowthOS 正常态 Compose 栈，便于主任务完成最终 smoke 和交付；
- growthos_mysql_data named volume，保留本节可复用开发数据；
- deploy/compose/secrets 下四个本机 Secret，保持与现有 MySQL 账号匹配；
- 本地构建镜像，供后续学习和复验；
- 本 QA 文档。

Secret 与 volume 的保留是功能需要，不是忘记清理。四个 Secret 仍被 Git 和 Docker build context 排除。

### 19.2 已清理

本轮只为验证创建的可丢弃资产已按精确目标清理：

- fresh-volume replay 的隔离临时 Compose 项目；
- fresh-volume replay 的临时 MySQL volume；
- Secret 分阶段生成使用的私有临时目录；
- Smoke 的临时 header/body 目录；
- 故障注入后遗留的测试状态。

没有删除：

- 用户既有 MySQL、Redis、RabbitMQ 或 PostgreSQL 容器；
- 用户已有数据卷；
- 项目源文件；
- 可复用依赖缓存；
- 当前 GrowthOS 数据卷和匹配 Secret。

最终栈恢复到：

- MySQL healthy；
- Migration exited 0；
- API healthy；
- Redis healthy；
- Web healthy；
- /health 200；
- /ready 200；
- 末尾 compose-smoke 通过。

---

## 20. 已知局限与后续工作

### 20.1 必须诚实保留的局限

1. **MySQL 启动边界：** 官方镜像 entrypoint 启动阶段使用 root 语义，根文件系统可写；只有长期 mysqld 进程验证为 UID 999。
2. **镜像供应链：** 版本 tag 已固定，但没有固定 OCI digest，也没有签名验证或 SBOM 门禁。
3. **传输安全：** 本地 Compose 中 MySQL TLS mode 为 disabled，Web 入口也是 HTTP，没有 TLS。
4. **资源治理：** Compose 没有 CPU、memory 或 PID quota；当前 memory 只是快照。
5. **Secret 后端：** 使用 Docker Desktop 本地文件 Secret，不是 Vault、云 Secret Manager 或 Swarm/Kubernetes Secret。
6. **Secret 轮换：** 进程启动时读取一次，不支持热轮换；MySQL 账号与文件必须协调更新。
7. **Redis 持久性：** Redis 是未接业务的易失占位，RDB/AOF 关闭，/data 是 tmpfs。
8. **自动恢复：** restart: no，不提供生产式自动拉起。
9. **单机范围：** 没有多节点、滚动升级、leader election、跨机网络或存储故障测试。
10. **日志能力：** 已验证当前故障路径脱敏，但没有集中日志、保留策略审计、指标或 trace backend。
11. **数据库备份：** 本节没有实现备份、恢复演练、PITR 或灾难恢复目标。
12. **生产入口：** 回环端口适合开发，不包含认证、WAF、限流或公网暴露策略。

### 20.2 后续进入生产化前的优先项

- 以 digest 和签名/SBOM 固定供应链；
- 为 API、Web、Redis 和 MySQL设计并验证资源配额；
- 引入 TLS 与证书轮换；
- 把文件 Secret 替换为受管 Secret 后端；
- 设计可审计的密码轮换流程；
- 明确 Redis 是否成为业务依赖，以及持久化和降级语义；
- 为 MySQL 补充备份恢复和升级演练；
- 增加 metrics、trace、集中安全日志与告警；
- 在 CI 的原生 amd64 和 arm64 runner 上双架构构建；
- 把故障矩阵扩展到磁盘满、慢查询、DNS 异常和优雅停止超时。

---

## 21. 最终结论

第 16 节达到本地 Docker Compose 开发环境的验收标准：

- 五服务拓扑真实构建并在 Apple Silicon 原生运行；
- MySQL → Migration → API 的条件启动语义生效；
- 只有 Web 发布 127.0.0.1:8088；
- edge、data、cache 网络成员和 internal 隔离符合设计；
- API、Migration、Web、Redis 的非 root、只读、cap drop 与 no-new-privileges 运行边界得到 inspect 证明；
- MySQL 官方镜像的 root/可写例外被准确记录；
- 文件型 Secret、四态生成器和 volume guard 经正负路径验证；
- 最小 MySQL grants 在全新 volume 中重放，并与当前卷一致；
- MySQL、API、Redis 故障均恢复，未通过重启无关 API/Web 掩盖问题；
- Nginx 的安全头、缓存头、动态 DNS、脱敏日志和 502 request ID 均经故障路径回归；
- 宽屏、移动端、深色与离线浏览器状态均完成，axe 扫描均为 0 violation；
- 正式 /health 门禁完成 30000/30000、零错误、零丢弃，P99 4.1495 ms，通过 100 ms 阈值；
- 正式 /ready 门禁完成 600/600、零错误、零丢弃，P99 6.841375 ms；
- 负载后最终 smoke 通过，栈恢复正常态。

本节可以作为后续学习分支的可复现实现与证据基线，但不能被表述为生产级高可用、TLS、安全供应链、资源隔离、热 Secret 轮换或持久化 Redis 已经完成。
