# 第 16 节面试问答：Docker Compose 可复现开发环境

本章只描述第 16 节已经落到仓库里的 Docker Compose 开发环境设计：`web`、`api`、一次性 `migrate`、MySQL 与 Redis 五个服务，Go/前端多阶段镜像，同源 Nginx 入口，分层网络，文件型 secrets，MySQL 持久卷与最小权限账号。Redis 在这一节只是被隔离、可启动的环境能力，尚未接入 GrowthOS 业务代码；是否完成镜像构建、故障演练、浏览器验收或压测，必须以[第 16 节 QA](../../qa/lessons/lesson-16.md)中的真实记录为准。本文不能把 Compose 文件存在、容器曾经启动或单次探针成功说成生产可用、线上高可用或已经达到某个 QPS。

## 60 秒项目自述

这一节我把 GrowthOS 的本地运行方式从“开发者分别启动几个进程”收敛成一套可审计的 Compose 拓扑。浏览器只访问绑定在 `127.0.0.1:8088` 的 Nginx；Nginx 提供前端静态资源，并把 `/health`、`/ready` 和未来 `/api` 同源代理到 Go API。API 同时加入 `edge` 和内部 `data` 网络，MySQL 与一次性迁移任务只在 `data` 网络，Redis独占内部 `cache` 网络且目前不与 API 相连，所以代码没有真正使用缓存时，Redis 停止不会被伪装成 API 不就绪。启动链路用“应用账号执行 `SELECT 1` 的 MySQL 健康检查 → 迁移任务成功退出 → API 启动”表达前置条件；Web 不依赖 API，因而上游故障时静态页面仍能展示真实降级状态。构建采用固定版本的 Go、Node、Nginx、MySQL 和 Redis 镜像及多阶段 Dockerfile；API、Migrator、Web 与 Redis 使用非 root、只读根文件系统、受限 tmpfs、`cap_drop: ALL` 和 `no-new-privileges`，MySQL 官方镜像因初始化与数据目录写入模型保留独立边界，不能被误称为同等加固。数据库、迁移与 Redis 密码通过按服务授权的 `/run/secrets` 文件注入，Go 同时支持互斥的 `_FILE` 配置；MySQL 数据用命名卷持久化，而 Redis 当前明确关闭 RDB/AOF、使用 tmpfs。这个方案解决的是单机开发环境的可复现性、边界与故障可诊断性，不替代 Kubernetes、生产 Secret Manager、备份恢复、自动扩缩容或生产容量验证。

## 来源说明

- `面经启发` 主要来自牛客用户自行发布的候选人复盘。它只能说明 Dockerfile、Compose、容器通信、挂载、Redis 持久化、故障排查等方向确实常被讨论；它不是企业官方题库，帖子中的公司、轮次和逐字题面也未被本文独立核验。
- `社区讨论` 可用于发现排障角度，不能作为 Docker、MySQL、Redis 行为的唯一依据。
- `官方事实` 优先使用 Docker、MySQL 和 Redis 的当前官方文档；镜像行为还需结合本章明确固定的版本和本地实测，不能把 `latest` 或其他版本的行为直接套用。
- `项目事实` 只能由当前 Compose、Dockerfile、配置加载、测试和 QA 记录支持。本文出现“设计为”“声明了”“实现了”不等于真实运行验收已经通过；真实结果、耗时和错误必须回到 QA。
- 本章有不少项目故障场景题，没有找到完全相同且可核验的面经，均明确按“项目场景题”组织，不虚构企业归属。

## 1. 为什么选择 Docker Compose，而不是让开发者分别执行 `docker run`？为什么又没有直接上 Kubernetes？

- **直接回答：** GrowthOS 当前需要在一台开发机上协调五个服务、三张网络、四份 secret、一个持久卷、多个健康条件和统一构建参数。逐条 `docker run` 可以启动容器，但启动顺序、网络、挂载和安全选项容易散落在口头说明或 shell history；Compose 把这些关系声明成可版本化配置。Kubernetes解决的是集群调度、控制器、自愈、滚动发布和多机网络等更大问题，本章尚无这些需求，提前引入会把学习重点从应用边界转移到集群运维。
- **优秀回答要点：** 先从需求而不是工具知名度出发；指出 Compose 改善的是“同一开发机上的声明式编排与复现”，不是生产高可用；说明 `Makefile` 只是稳定入口，真正拓扑仍在 `compose.yaml`；能说出未来迁移到集群编排时需要重新设计 Secret、存储、探针、资源与发布策略。
- **追问：** Compose 和 `docker run` 最本质的差别是“能启动多个容器”吗？
  - **追问回答：** 不是，shell 同样能启动多个容器。关键差别是 Compose 把服务关系、网络、卷、secret、build target、healthcheck 和生命周期命令集中成一份可校验的期望状态，并用 project name 给资源做作用域隔离。
- **常见误区：** 把 Compose 说成“轻量版 Kubernetes”；声称写了 Compose 就具备自动扩缩容、跨主机调度、滚动发布或生产级 Secret 管理。
- **技术延伸：** 如果进入多节点生产环境，应比较 Kubernetes、Nomad 或托管容器平台，重新评估 StatefulSet/托管数据库、Ingress、CSI、Secret Store、PDB、HPA 和可观测性，而不是机械地把 Compose YAML 翻译成另一种 YAML。
- **项目证据：** [Compose 拓扑](../../../deploy/compose/compose.yaml)、[统一开发命令](../../../Makefile)。
- **选型边界：** 只有单进程或两个无依赖服务时，Compose 可能比直接运行更重；一旦需要集群级故障恢复、滚动升级或多租户资源治理，Compose 又明显不够。
- **来源：** `面经启发` [牛客候选人自述中出现“Docker Compose 用过吗、写过 file 吗”以及容器化开发环境追问](https://www.nowcoder.com/discuss/353158027186479104)；`官方事实` [Docker Compose 概览](https://docs.docker.com/compose/)、[Compose 如何控制启动顺序](https://docs.docker.com/compose/how-tos/startup-order/)。

## 2. 你能画出这一节的拓扑，并区分请求链路、启动链路和数据链路吗？

- **直接回答：** 请求链路是“浏览器 → 宿主机回环端口 `8088` → Nginx `web` → Go `api`”；数据链路是“API → MySQL”，迁移链路是“`migrate` → MySQL”。启动链路则是“MySQL 通过真实应用账号检查 → `migrate` 成功退出 → API 才创建”。`web` 没有对 API 的启动依赖，Redis 位于独立 `cache` 网络且当前没有任何业务消费者。把三条链分开，才能避免把网络可达、启动先后和业务依赖混成一件事。
- **优秀回答要点：** 能明确五个服务各自职责；能说出 `migrate` 是作业而不是常驻服务；能解释 Web 独立启动是为保留故障页面；能主动指出 Redis 只是环境边界，不能说“系统已经使用 Redis 缓存”。
- **追问：** 为什么 `api` 同时接入 `edge` 和 `data`，而 MySQL 只接入 `data`？
  - **追问回答：** API 是唯一需要同时接受 Web 请求并访问数据库的桥接服务；MySQL 不需要被 Web 或宿主机直接访问。最小网络成员关系缩小误访问面，也让排障时能从网络图快速判断某条连接是否本来就不应存在。
- **常见误区：** 把容器列表当架构图；没有说明端口、网络和信任边界；看到 Redis 服务就宣称缓存命中率、一致性或性能收益。
- **技术延伸：** 生产拓扑还应补充 TLS 终止、身份认证、出站控制、负载均衡、备份、观测采集和跨可用区故障域；本章的单机 bridge 网络不能代表这些能力。
- **项目证据：** [五服务及三网络声明](../../../deploy/compose/compose.yaml)、[Nginx 同源路由](../../../deploy/docker/nginx.conf)。
- **选型边界：** 当新增 worker 真正使用 Redis 时，需要让它加入 `cache` 网络并定义自己的失败语义；不能为了省配置直接把所有服务塞回一张默认网络。
- **来源：** `面经启发` [牛客候选人自述中的 Docker 容器通信与架构图题型](https://www.nowcoder.com/discuss/353154648989179904)、[另一份候选人自述中的 Docker 通信和 Dockerfile 题型](https://www.nowcoder.com/discuss/645752512930160640)；`官方事实` [Compose 网络和服务发现](https://docs.docker.com/compose/how-tos/networking/)。

## 3. 为什么把数据库迁移做成独立的一次性 `migrate` 服务，而不是 API 启动时自动迁移？

- **直接回答：** schema 变更需要比普通业务进程更高的权限、明确的失败出口和可审计的执行时机。独立 `migrate` 使用 `growthos_migrator` 凭据，成功后退出；API 只拿 DML 账号，并通过 `service_completed_successfully` 等待迁移成功。这样迁移失败会阻断 API 首次启动，而不是让 API 在一半 schema 上继续提供不确定服务，也避免每个 API 副本争抢迁移。
- **优秀回答要点：** 同时讲清权限隔离、并发迁移、失败可见性和部署顺序；指出一次性任务成功并不等于所有迁移天然无风险；能说明向后兼容的 expand/contract 仍是滚动发布场景的必要策略。
- **追问：** 如果迁移失败，直接设置 `restart: always` 让它反复重试是否更稳？
  - **追问回答：** 不一定。语法错误、权限错误或非幂等 DDL 可能被无限重放，掩盖根因甚至扩大破坏。本地环境选择 `restart: "no"`，先保留失败状态和日志，由开发者修复后显式重跑。真正自动重试也应只覆盖已分类的瞬时错误，并设置次数、退避和告警。
- **常见误区：** 认为“迁移脚本有事务”就能保证所有 MySQL DDL 完整回滚；让 API 使用 root；把初始化用户脚本和版本化业务 migration 当成同一生命周期。
- **技术延伸：** 多实例发布需要数据库变更向后兼容、迁移锁、超时预算、备份/回滚预案和发布编排；大表 DDL 还可能需要 online schema change 工具。
- **项目证据：** [独立迁移服务和完成条件](../../../deploy/compose/compose.yaml)、[迁移程序入口](../../../cmd/growth-migrate/main.go)、[迁移账号配置边界](../../../internal/platform/appconfig/config.go)。
- **选型边界：** 小型一次性原型可以在启动时自动建表；一旦有多个副本、权限分层或不可逆 DDL，就应把迁移从业务启动路径分离。
- **来源：** `面经启发` [牛客候选人自述强调项目架构、为什么做以及技术取舍的追问](https://www.nowcoder.com/discuss/353158027186479104)；`官方事实` [Compose 的 `service_completed_successfully`](https://docs.docker.com/reference/compose-file/services/#depends_on)。

## 4. 多阶段构建解决了什么问题？GrowthOS 的 Go 和 Web 镜像分别怎样分阶段？

- **直接回答：** 多阶段构建允许在包含编译工具的阶段产生产物，再只把必要产物复制到运行阶段，避免把 Go SDK、Node、pnpm store、源码和构建缓存一起放进最终镜像。Go Dockerfile 用一个 builder 同时构建 `growth-api` 和 `growth-migrate`，再通过 `api`、`migrate` target 生成两个非 root Alpine 运行镜像；Web 在 Node 阶段执行冻结依赖安装和 Vite build，再把 `dist` 复制到 Nginx 运行镜像。
- **优秀回答要点：** 不只说“镜像更小”，还要说攻击面、依赖边界和构建/运行职责；能解释命名 stage 与 target；能指出最终镜像仍需 CA、时区或健康检查工具等实际运行依赖，所以不是越空越好。
- **追问：** Go 二进制为什么设置 `CGO_ENABLED=0`？是否永远应该这样？
  - **追问回答：** 当前依赖可以生成静态 Linux 二进制，关闭 CGO 简化运行库和跨架构构建。但如果未来依赖 C 库、系统 DNS 行为、SQLite CGO 驱动或必须使用 FIPS 模块，就要重新评估；不能把 `CGO_ENABLED=0` 当通用安全规则。
- **常见误区：** 只比较镜像体积；把 builder 中的测试工具误当最终镜像能力；认为 Alpine 一定比 distroless/scratch 更安全；在运行镜像里重新 `go build` 或 `pnpm install`。
- **技术延伸：** 可增加测试 stage、SBOM、签名、漏洞扫描和 provenance；生产若改用 distroless，需要重新提供探针策略、CA、时区和调试逃生舱。
- **项目证据：** [Go 多 target Dockerfile](../../../deploy/docker/Dockerfile.backend)、[Web Node→Nginx Dockerfile](../../../deploy/docker/Dockerfile.web)。
- **选型边界：** 本地需要进入容器交互排查时，Alpine 的最小工具集比 scratch 更实用；高度受限生产镜像可另设 hardened target，而不是牺牲开发可诊断性。
- **来源：** `面经启发` [牛客候选人自述中的 Docker 分层、Dockerfile 和镜像题型](https://www.nowcoder.com/discuss/353158027186479104)、[容器镜像与 Dockerfile 追问记录](https://www.nowcoder.com/discuss/353154648989179904)；`官方事实` [Docker multi-stage builds](https://docs.docker.com/build/building/multi-stage/)。

## 5. Dockerfile 怎样兼顾构建缓存和可复现性？固定版本标签是否已经完全可复现？

- **直接回答：** Go 先复制 `go.mod/go.sum` 并执行 `go mod download && go mod verify`，依赖未变化时可复用这一层；Web 先复制 `package.json/pnpm-lock.yaml` 并用 `--frozen-lockfile` 安装，再复制源码构建。BuildKit cache mount 加速模块和 pnpm 下载，但不进入最终镜像。各基础镜像使用具体版本标签，避免 `latest` 的大幅漂移；不过标签仍可能被上游重新指向，所以这只是“版本级约束”，不是 digest 级不可变复现。
- **优秀回答要点：** 能区分 layer cache、cache mount、lockfile 和镜像 digest 四个概念；说明缓存命中影响速度而不应改变结果；知道把源码先全部 `COPY` 会让每次改代码都使依赖层失效；主动承认当前没有声称 bit-for-bit reproducible。
- **追问：** 为什么不把所有镜像都固定到 SHA256 digest？
  - **追问回答：** digest 提供更强不可变性和供应链审计，但升级维护成本更高，且多架构 manifest 与具体平台 digest 要管理清楚。开发课程先用精确版本提高可读性；进入 CI/CD 后可由自动化同时维护可读 tag 和锁定 digest。
- **常见误区：** 认为有 `go.sum` 就锁住基础镜像；把 BuildKit 缓存当依赖来源；用 `latest` 后声称团队环境一致；把一次本机构建成功当跨平台可复现。
- **技术延伸：** 可引入 Renovate/Dependabot、registry admission policy、SBOM 和签名验证，并记录构建器版本、平台、源码提交与构建参数。
- **项目证据：** [Go 依赖层](../../../deploy/docker/Dockerfile.backend)、[Web 冻结锁文件构建](../../../deploy/docker/Dockerfile.web)、[构建上下文排除规则](../../../.dockerignore)。
- **选型边界：** 课程分支可以先固定语义版本；合规或高风险交付应锁 digest、建立自动升级与回滚流程，而不是永久冻结旧镜像。
- **来源：** `官方事实` [优化 Docker build cache](https://docs.docker.com/build/cache/optimize/)、[Docker image 可使用 tag 或 digest](https://docs.docker.com/reference/compose-file/services/#image)、[multi-stage builds](https://docs.docker.com/build/building/multi-stage/)。

## 6. `depends_on` 能保证依赖真正就绪吗？三个 condition 分别表示什么？

- **直接回答：** 短语法或 `service_started` 只保证依赖容器已进入运行状态，不保证数据库能处理查询。`service_healthy` 会等待依赖的 healthcheck 通过；`service_completed_successfully` 适合迁移这类必须成功退出的一次性作业。GrowthOS 让迁移等待 MySQL healthy，让 API 同时等待 MySQL healthy 和迁移成功。这个保证只覆盖 Compose 创建阶段，不是运行期持续依赖管理器。
- **优秀回答要点：** 准确区分“进程已启动、健康检查通过、任务成功退出”；指出 healthcheck 的质量决定 `service_healthy` 的含义；强调运行期 MySQL 再故障时，Compose 不会因为 `depends_on` 自动重启或暂停 API。
- **追问：** `depends_on` 里写 `restart: true` 是否等于数据库宕机后 API 自动重启？
  - **追问回答：** 不是。Docker 官方说明该字段针对显式 Compose 更新/重启操作传播重启，不等同于容器运行时崩溃联动。GrowthOS 没有依赖这种隐式重启，而要求应用连接池面对断连并通过 readiness 报告真实状态。
- **常见误区：** 看到 `depends_on` 就认为“依赖可用”；把 healthcheck 通过当永久保证；以为依赖 unhealthy 会自动停止消费者；用长时间 sleep 代替语义检查。
- **技术延伸：** 应用还需连接超时、有限重试、指数退避和运行期恢复；生产编排可用 init container/job 与 probe，但同样不能替代应用自身的故障处理。
- **项目证据：** [MySQL→migrate→API 条件](../../../deploy/compose/compose.yaml)。
- **选型边界：** 无状态、能自重试的消费者有时只需 `service_started`；对一次性迁移和严格初始化步骤，应使用可证明完成的条件，避免时间猜测。
- **来源：** `面经启发` [牛客候选人自述中的 Compose file 题型](https://www.nowcoder.com/discuss/353158027186479104)；`官方事实` [Compose 启动顺序和 condition](https://docs.docker.com/compose/how-tos/startup-order/)、[`depends_on` 规范](https://docs.docker.com/reference/compose-file/services/#depends_on)。

## 7. 容器 healthcheck、API `/health` 和 `/ready` 分别在回答什么？

- **直接回答：** Docker healthcheck 是容器级的周期性命令，其价值取决于被检查的端点。API 容器检查 `/health`，只确认 Go 进程能响应，不把 MySQL 短暂故障变成“进程死了”；业务流量是否可接收由 `/ready` 反映 MySQL 依赖。MySQL healthcheck 不是只看端口或进程，而是用 `growthos_app` 真实认证并执行 `SELECT 1`。Web 检查自己的 `/container-health`，所以 API 故障不会把静态 Web 本身误判为死亡。
- **优秀回答要点：** 能画出 liveness/readiness 的失败矩阵；说明 MySQL 检查验证了网络、认证、schema 访问和查询能力的一条最小路径；指出 Docker health 状态本身不会自动修复容器，是否重启由 restart policy/编排器决定。
- **追问：** 为什么 API healthcheck 不直接检查 `/ready`？
  - **追问回答：** 如果 MySQL 暂时不可用，API 仍可返回可解释的 503、日志和请求 ID；把 readiness 当 liveness 可能诱发无意义重启，增加故障抖动。是否摘流和是否杀进程是不同控制动作。
- **常见误区：** 用 `mysqladmin ping` 或只测 TCP 就宣称应用凭据可用；把 healthcheck 成功当业务全链路成功；把 `/health` 200 当数据库健康。
- **技术延伸：** 后续可增加启动探针、业务合成探针和外部黑盒监控，但应控制探针成本、超时与权限，避免健康检查本身压垮依赖。
- **项目证据：** [三个服务的 healthcheck](../../../deploy/compose/compose.yaml)、[Go 探针实现](../../../internal/infrastructure/httpapi/readiness.go)。
- **选型边界：** 若 API 即使数据库不可用也完全没有可服务能力，部署层可能用 readiness 摘流；仍应谨慎决定是否把它升级为 liveness 失败。
- **来源：** `面经启发` [牛客候选人自述中的注册中心判活和端口排查题型](https://www.nowcoder.com/discuss/690263)；`官方事实` [Compose healthcheck](https://docs.docker.com/reference/compose-file/services/#healthcheck)、[Compose 不会只因容器运行就等待就绪](https://docs.docker.com/compose/how-tos/startup-order/)。

## 8. 为什么 Web 不依赖 API healthy？这会不会造成启动顺序错误？

- **直接回答：** Web 的职责包括提供静态页面和呈现“API 不可达/数据库未就绪”的真实降级状态。如果让 Web 依赖 API healthy，最需要错误页面时反而无法打开。Nginx 可以先启动，API 恢复后通过稳定服务名重新解析并代理；用户入口存在与上游可服务是两个独立状态。`docker compose up --wait` 仍会整体等待/报告所有服务状态，但依赖图不会人为阻止 Web 创建。
- **优秀回答要点：** 从用户可诊断性解释，而不是只说“并行启动更快”；能区分 Web 容器自身 health 与代理请求结果；说明恢复后无需重新构建前端；知道独立启动并不代表忽略 API 告警。
- **追问：** API 没启动时 Nginx 启动会不会因为解析不到 `api` 而直接退出？
  - **追问回答：** 配置使用 Docker 内嵌 DNS resolver 和变量形式的 `proxy_pass`，把解析推迟到请求时并周期重解析，避免启动时固定一个上游 IP。请求当下无上游时应返回网关错误，静态路由仍可用。
- **常见误区：** 为消灭短暂 502 强制把 Web 和 API 绑死；认为页面能打开就代表 API 健康；用前端假数据把故障遮住。
- **技术延伸：** 生产环境可用网关重试、熔断和静态错误页，但要避免对非幂等请求盲目重试；真正可用性仍需外部监控和 SLO。
- **项目证据：** [Web 无 `depends_on`](../../../deploy/compose/compose.yaml)、[Docker DNS 动态解析及同源代理](../../../deploy/docker/nginx.conf)、[前端故障分类](../../../web/src/api/httpClient.ts)。
- **选型边界：** 管理后台若所有资源都由 API 动态渲染，独立静态 Web 的价值会降低；但故障可观测入口仍应由外部状态页或网关提供。
- **来源：** `官方事实` [Compose 服务名稳定而容器 IP 可变](https://docs.docker.com/compose/how-tos/networking/)、[Nginx proxy module](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)。

## 9. 容器里为什么不能用 `localhost` 访问另一个服务？服务名和容器 IP 应该选哪个？

- **直接回答：** 每个容器有自己的网络命名空间，容器内 `127.0.0.1` 只指向本容器。Compose 同一网络中的服务通过内嵌 DNS 解析稳定服务名，因此 API 使用 `mysql:3306`，Nginx 使用 `api:8080`。容器重建后 IP 可能变化，硬编码 IP 会失效；服务名保持稳定，由调用方在连接断开后重新解析和建立连接。
- **优秀回答要点：** 同时区分宿主机、容器和浏览器三个 `localhost`；说明 service-to-service 使用 container port；指出已建立连接在上游重建后仍会断开，DNS 不是连接迁移机制。
- **追问：** macOS Docker Desktop 上什么时候使用 `host.docker.internal`？
  - **追问回答：** 当容器确实需要访问宿主机服务时才用；容器间通信不应绕到宿主机端口。GrowthOS 的 MySQL 和 API 都在同一 Compose project 内，因此使用服务名，不依赖 Desktop 特有地址。
- **常见误区：** 把宿主机映射端口写进容器间 DSN；固定 `docker inspect` 得到的 IP；认为 DNS TTL 到期会自动修复现有 TCP 连接。
- **技术延伸：** 长连接客户端需要断线检测、重新解析和退避；Nginx、数据库连接池、gRPC resolver 对 DNS 更新的策略不同，应逐一验证。
- **项目证据：** [容器内 MySQL 地址与网络](../../../deploy/compose/compose.yaml)、[Nginx Docker DNS resolver](../../../deploy/docker/nginx.conf)。
- **选型边界：** 外部托管数据库应使用其稳定 DNS/TLS 名称；跨 Compose project 可使用显式 external network，但会扩大耦合和信任边界。
- **来源：** `面经启发` [牛客候选人自述中明确出现“多个容器如何通信、怎样知道对方 IP”](https://www.nowcoder.com/discuss/353154648989179904)、[Docker 通信题型复盘](https://www.nowcoder.com/discuss/645752512930160640)；`官方事实` [Compose networking 与服务发现](https://docs.docker.com/compose/how-tos/networking/)。

## 10. 为什么拆成 `edge`、`data`、`cache` 三张网络？`internal: true` 是否意味着绝对隔离？

- **直接回答：** 网络成员关系表达最小通信面：`web↔api` 在 `edge`，`api/migrate↔mysql` 在内部 `data`，Redis 单独位于内部 `cache`。因此 Web 不能直接解析 MySQL，API 也不能凭现有网络访问 Redis，未实现的缓存依赖不会偷偷形成。`internal: true` 使该网络没有默认外部网关，但一个同时加入 `edge` 与 `data` 的 API 仍可能通过另一张非内部网络出站，所以它不是对受侵 API 的绝对数据隔离。
- **优秀回答要点：** 把网络分段与应用授权区分；明确“双网卡”服务可转发或出站；说明网络隔离、数据库账号和 TLS 是互补控制；能指出本地 bridge 网络不是生产网络策略证明。
- **追问：** 为什么 Redis 不直接加入 `data` 网络，反正都是后端基础设施？
  - **追问回答：** 当前没有任何业务服务使用 Redis，单独网络让“未接入”成为可验证事实。未来只有真正的消费者加入 `cache`，再设计 key、TTL、失败降级和授权，而不是默认所有后端都能访问所有状态组件。
- **常见误区：** 认为不同网络上的容器永远无法被间接访问；把 network 当身份认证；为了排障把所有服务接入默认网络后忘记收回。
- **技术延伸：** 生产应进一步考虑 NetworkPolicy、service mesh、mTLS、出站代理和数据库防火墙；网络拓扑应由威胁模型与数据流驱动。
- **项目证据：** [三网络及成员关系](../../../deploy/compose/compose.yaml)。
- **选型边界：** 很小的本地示例用一张默认网络更简单；当服务边界、敏感数据或“尚未接入”的事实需要被审计时，显式分网收益更大。
- **来源：** `面经启发` [牛客候选人自述中的 Docker 容器通信题型](https://www.nowcoder.com/discuss/353154648989179904)；`官方事实` [Compose 自定义网络与 internal 网络](https://docs.docker.com/compose/how-tos/networking/#specify-custom-networks)。

## 11. `ports` 和 `expose` 有什么区别？为什么只发布 Web，且绑定 `127.0.0.1`？

- **直接回答：** `ports` 把容器端口发布到宿主机；`expose` 只记录容器端口意图，不创建宿主机映射，服务间通信本来也依赖共享网络和容器端口。GrowthOS 只把 Nginx 的 `8080` 发布为宿主机回环地址 `127.0.0.1:8088`，API 只 expose `8080`，MySQL/Redis 不发布端口。这样浏览器只有一个同源入口，也不与开发者已有的宿主机 `3306/6379` 冲突。
- **优秀回答要点：** 区分 host port 与 container port；解释回环绑定减少局域网暴露但不是认证；说明本地调试数据库可通过 `docker compose exec` 或临时 profile，而不是永久发布敏感端口。
- **追问：** `expose: 8080` 是否是 API 网络可达的必要条件？
  - **追问回答：** 不是。同一 Compose 网络上的服务可直接访问容器监听端口；`expose` 更多是元数据和意图表达。真正决定可达性的是进程监听地址、网络成员关系和防火墙/平台规则。
- **常见误区：** 认为 `EXPOSE` 或 Compose `expose` 自动开放主机端口；使用 `0.0.0.0:3306:3306` 后说“只在本机”；把端口未发布误解为数据库无需密码。
- **技术延伸：** 团队需要宿主机 GUI 连接时，可通过 opt-in override/profile 映射到回环随机端口，并继续使用低权限账号；CI 中通常完全不需要发布数据库端口。
- **项目证据：** [唯一 loopback 端口映射](../../../deploy/compose/compose.yaml)、[Nginx 同源入口](../../../deploy/docker/nginx.conf)。
- **选型边界：** 若 API 要被移动端或独立前端直接访问，需要正式 TLS 网关、认证和 CORS 设计，不能简单把 API 映射到 `0.0.0.0`。
- **来源：** `官方事实` [Compose networking 对 host/container port 的说明](https://docs.docker.com/compose/how-tos/networking/)、[发布容器端口](https://docs.docker.com/get-started/docker-concepts/running-containers/publishing-ports/)。

## 12. 为什么不用普通环境变量传密码，而使用 Compose secrets 和 `_FILE`？

- **直接回答：** 普通环境变量更容易出现在进程环境、诊断输出或误打日志中。Compose secrets 让每个服务只获得显式授权的文件，默认挂载到 `/run/secrets/<name>`；API 只拿应用密码，迁移任务只拿迁移密码，MySQL 初始化才拿需要的三份数据库 secret。Go 配置支持 `GROWTHOS_MYSQL_PASSWORD_FILE` 与迁移对应变量，并要求直接值和文件值二选一，避免优先级含糊。
- **优秀回答要点：** 说出“降低误暴露”和“按服务授权”，而不是声称加密；能解释 `_FILE` 是部署适配层，应用内部最终仍需短暂持有密码；知道错误消息、`String/MarshalJSON/slog` 也要做脱敏；说明本地 secret 源文件仍需宿主机权限和备份策略。
- **追问：** Compose secrets 是否等于 Vault/KMS，文件内容在宿主机上自动加密吗？
  - **追问回答：** 不是。本项目使用 Compose 的 file-backed secret，源文件仍是本地明文，只是避免进入 YAML、环境变量和非授权服务，并以只读文件挂载。生产需要专门 Secret Manager、短期凭据、审计和轮换。
- **常见误区：** 把 secret 值写进 `compose.yaml` 再命名为 secret；同时设置直接密码和 `_FILE` 却不定义优先级；把配置对象整体打印；认为 `.gitignore` 能撤回已经提交的秘密。
- **技术延伸：** 更成熟方案可用 Vault Agent、云 Secret Manager/CSI driver、动态数据库账号或工作负载身份；应用应支持无泄漏的 reload/rotation 语义。
- **项目证据：** [按服务 secret 授权](../../../deploy/compose/compose.yaml)、[Go `_FILE` 加载与互斥校验](../../../internal/platform/appconfig/config.go)、[secret 配置测试](../../../internal/platform/appconfig/config_test.go)。
- **选型边界：** 本地非敏感一次性示例用 `.env` 更简单；真实密码、共享开发机、CI 或生产必须提升 secret 生命周期治理，Compose 文件挂载只是基础层。
- **来源：** `官方事实` [Docker Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/)、[MySQL 官方镜像 `_FILE` 支持](https://hub.docker.com/_/mysql)。

## 13. 为什么 secret 生成脚本拒绝补齐“部分缺失”的文件，也拒绝在 MySQL 卷存在时重新生成整套密码？

- **直接回答：** MySQL 初次初始化会把当时的 root、应用和迁移密码写入持久数据目录。若本地 secret 丢失但 `mysql_data` 仍在，重新随机生成文件不会自动修改数据库里的账号，结果是容器配置与持久状态永久不一致。脚本因此只允许“整套都不存在且没有旧 MySQL 卷”时生成完整集合，或“整套都存在”时校验；1～3 个文件存在时 fail closed，避免拼成不属于同一初始化世代的凭据集。
- **优秀回答要点：** 能把 secret 生命周期与 volume 生命周期关联起来；指出这是防误操作，不是完整轮换协议；说明脚本先在临时目录生成并校验四个 64 位小写十六进制值，再逐个搬入，若进程中断留下部分集合，下次会拒绝继续；知道真正遗失凭据时应先判断保留数据还是显式重置。
- **追问：** 正确的密码轮换应该怎么做？
  - **追问回答：** 先建立可回滚的新凭据，通过受控高权限连接 `ALTER USER`，更新 Secret 来源，按顺序重启/重载消费者并验证，再撤销旧凭据。若追求零停机，数据库需允许短暂双凭据或双用户过渡。本章未实现并验证该流程，不能宣称支持在线轮换。
- **常见误区：** 删除 secret 后直接运行生成脚本；认为重建容器等于重建命名卷；把 `docker compose down -v` 当普通清理；在日志中打印随机值验证。
- **技术延伸：** 可为 secret 集增加 generation metadata、校验指纹和备份恢复 Runbook，但不能把 secret 哈希放到公开位置后误当认证机制。
- **项目证据：** [安全生成与卷保护脚本](../../../scripts/generate-compose-secrets.sh)、[本地 secret 说明](../../../deploy/compose/secrets/README.md)、[MySQL 命名卷](../../../deploy/compose/compose.yaml)。
- **选型边界：** 无持久状态的 Redis 临时密码可以随环境重建；绑定持久 MySQL 身份的凭据必须与数据生命周期协调。
- **来源：** `官方事实` [MySQL 官方镜像说明已有数据库不会被环境变量改变](https://hub.docker.com/_/mysql)、[Docker volume 生命周期](https://docs.docker.com/engine/storage/volumes/#a-volumes-lifecycle)。

## 14. MySQL 的 `/docker-entrypoint-initdb.d` 脚本什么时候执行？修改脚本后为什么旧卷不生效？

- **直接回答：** MySQL 官方镜像只在发现全新数据目录、执行首次初始化时处理 `/docker-entrypoint-initdb.d` 中的脚本，并按文件名顺序执行。命名卷里已有数据库后，镜像会保留现有数据，初始化环境变量和脚本不会重新配置账号。因此修改 `10-create-growthos-users.sh` 后仅重建容器不会更新旧用户，必须写显式 migration/管理操作，或在确认可以丢数据后显式重建卷。
- **优秀回答要点：** 区分 image、container 和 volume 生命周期；说明 init 脚本负责 bootstrap 身份，不承载后续 schema 演进；知道 `compose down` 默认保留命名卷，而 `down -v` 是破坏性重置。
- **追问：** 为什么不把所有业务表 SQL 也放进 init 目录？
  - **追问回答：** init 目录只适合空库 bootstrap，无法对已有卷表达版本、顺序和回滚；业务 schema 应交给版本化 migration。否则新环境与长期环境会走两套不可对账的建表路径。
- **常见误区：** 每次 `up` 都期待 init 脚本重跑；为让脚本生效随手删除卷；认为容器删除会连数据一起删除；修改密码文件后期待旧 MySQL 用户自动同步。
- **技术延伸：** 可对 bootstrap 脚本做空卷集成测试；生产数据库初始化通常交给 IaC/DBA 或受控 job，并需要备份和权限审计。
- **项目证据：** [用户 bootstrap 脚本](../../../deploy/compose/mysql/init/10-create-growthos-users.sh)、[MySQL 卷与挂载](../../../deploy/compose/compose.yaml)、[版本化 migrations](../../../migrations)。
- **选型边界：** 完全无状态、可随时丢弃的测试数据库可以重建卷；包含学习数据或共享数据时必须先备份并明确授权。
- **来源：** `面经启发` [牛客候选人自述中的外部数据挂载和容器数据题型](https://www.nowcoder.com/discuss/353154648989179904)；`官方事实` [MySQL 官方镜像 fresh instance 初始化行为](https://hub.docker.com/_/mysql)、[Docker volumes](https://docs.docker.com/engine/storage/volumes/)。

## 15. 为什么 MySQL 要分 root、migrator 和 app 三种身份？当前授权边界是什么？

- **直接回答：** root 只用于官方镜像首次初始化；`growthos_migrator` 在 `growthos.*` 上显式拥有 `SELECT、INSERT、UPDATE、DELETE、CREATE、ALTER、DROP、INDEX、REFERENCES`，用于版本迁移；`growthos_app` 只有 `SELECT、INSERT、UPDATE、DELETE`，API 不具备 DDL、存储过程执行或跨 schema 管理权限。这样即使 API 出现 SQL 注入或凭据泄漏，也不应直接获得 root 级能力，迁移凭据也不会进入 API 容器。
- **优秀回答要点：** 能说明最小权限降低的是爆炸半径而非消除漏洞；指出 migrator 也没有使用方便但宽泛的 `ALL PRIVILEGES`，而是把当前 migration engine 所需能力显式列出；知道 host `%` 不等于互联网开放，因为还有网络边界，但生产可进一步约束来源。
- **追问：** app 没有 `CREATE` 权限，为什么 MySQL healthcheck 还能验证它“可用”？
  - **追问回答：** 健康检查以应用身份连接指定数据库并执行 `SELECT 1`，验证最小读取/会话路径；DDL 能力本来就不属于 API 健康条件。真正的业务表权限还应由集成测试覆盖，不能从常量查询推导所有 DML 都正确。
- **常见误区：** API 直接使用 root 图省事；把网络不发布端口当权限控制；认为 `SELECT 1` 能证明 schema、索引和全部查询正常；把 `%` 描述成“允许任何互联网主机”而忽略 Docker 网络与端口边界。
- **技术延伸：** 随业务细分可按服务/表/操作进一步拆账号，使用审计、短期凭据和 TLS；若后续确实引入存储过程或新的 DDL 类型，应以新 migration 的真实需求增量授权，而不是提前授予 `EXECUTE` 或 `ALL PRIVILEGES`。
- **项目证据：** [账号创建与 grant](../../../deploy/compose/mysql/init/10-create-growthos-users.sh)、[三个 secret 的服务分配](../../../deploy/compose/compose.yaml)、[API/迁移配置隔离](../../../internal/platform/appconfig/config.go)。
- **选型边界：** 单人本地 demo 可以只有一个普通账号；进入共享、CI 或生产环境后，运行时与迁移权限分离应成为最低要求。
- **来源：** `面经启发` [牛客候选人自述强调“项目里 MySQL 用在哪里、为何称为熟悉”的追问](https://www.nowcoder.com/discuss/353154648989179904)；`官方事实` [MySQL 8.4 `GRANT` 语句](https://dev.mysql.com/doc/refman/8.4/en/grant.html)、[访问控制与账号管理](https://dev.mysql.com/doc/refman/8.4/en/access-control.html)。

## 16. Redis 为什么已经出现在 Compose 中，却没有加入 API 网络或 readiness？

- **直接回答：** 本章只先建立一个安全、可复现的 Redis 环境边界，为后续缓存章节准备基础设施；当前 Go 代码没有 Redis client、key 模型、TTL、一致性或降级语义。让 API 连接 Redis 或把 Redis PING 纳入 readiness 会制造并不存在的业务依赖，所以 Redis 独占 `cache` 网络，停止 Redis 不应影响当前 API `/ready`。这是刻意保留“尚未接入”的真实性。
- **优秀回答要点：** 明确区分“容器可运行”和“业务已使用”；能解释依赖只有在功能语义需要时才加入；指出提前放入环境的收益是镜像、安全、secret 和故障边界可先验收，代价是多一个资源与维护项。
- **追问：** 既然没用，为什么不等后续章节再加？
  - **追问回答：** 这一节的目标包含本地基础设施拓扑和隔离训练，提前加入能验证“未使用依赖不应拖垮核心路径”。但如果维护成本或启动时间明显影响团队，也完全可以通过 Compose profile 延迟启用；当前选择不是普遍真理。
- **常见误区：** 简历写“使用 Redis 提升性能”却没有任何调用、命中率或一致性设计；Redis PING 成功就宣称缓存功能正常；为了看起来技术栈丰富把所有中间件接入 readiness。
- **技术延伸：** 真正接入前必须回答缓存对象、key、TTL、穿透/击穿/雪崩、双写一致性、容量、淘汰、降级和观测；再决定 Redis 是可选加速层还是正确性依赖。
- **项目证据：** [Redis 独立 cache 网络](../../../deploy/compose/compose.yaml)、[Redis 启动配置](../../../deploy/docker/redis-entrypoint.sh)、[当前 Go 依赖清单](../../../go.mod)。
- **选型边界：** 后续若 Redis 承担幂等、锁、会话或队列，它可能成为正确性依赖，readiness 和恢复策略必须重新设计，不能继续沿用“可停”的假设。
- **来源：** `面经启发` [牛客候选人自述中出现缓存作用、分级缓存一致性与 Redis 持久化追问](https://www.nowcoder.com/discuss/690263)；`官方事实` [Redis 官方镜像](https://hub.docker.com/_/redis)。

## 17. 为什么当前 Redis 关闭 RDB 和 AOF，并把 `/data` 放在 tmpfs？如果要持久化该怎么选？

- **直接回答：** 当前 Redis 没有业务数据，目标是可丢弃的开发环境能力。配置 `save ""`、`appendonly no`，`/data` 使用有容量上限的 tmpfs，重建即清空；这避免无意义的命名卷和“旧缓存就是事实来源”的错觉。若未来 Redis 承担需要恢复的数据，必须依据可接受数据丢失窗口、恢复时间和写放大，在 RDB、AOF 或组合方案中选择，并建立备份恢复验证。
- **优秀回答要点：** 先定义 Redis 数据语义再选持久化；知道 RDB 是时间点快照、AOF 记录写操作且可配置 fsync；指出持久化不能自动等于备份、高可用或强一致；能解释 tmpfs 容量仍受宿主机内存影响。
- **追问：** 缓存是不是永远不需要持久化？
  - **追问回答：** 不是。如果冷启动回源会击穿数据库、缓存重建非常昂贵，持久化可缩短恢复时间；但缓存仍应能从权威数据重建。若 Redis 保存的是唯一状态，它就不再只是缓存，需要更严格的持久化、复制和恢复设计。
- **常见误区：** 看到 Redis 默认配置就假设已持久化；把 AOF `everysec` 说成零数据丢失；把主从复制当离线备份；tmpfs 无上限地占内存。
- **技术延伸：** 后续应测试重启恢复、AOF rewrite、RDB fork 内存、maxmemory/eviction、冷启动限流与回源保护；面试时要把理论策略和项目实际选择分开。
- **项目证据：** [显式关闭持久化](../../../deploy/docker/redis-entrypoint.sh)、[Redis `/data` tmpfs 限额](../../../deploy/compose/compose.yaml)。
- **选型边界：** 可丢弃本地缓存适合 no-persistence；会影响订单、积分或唯一事实的数据不应在没有明确持久化和一致性协议时只放 Redis。
- **来源：** `面经启发` [牛客候选人自述中的 Redis 持久化和缓存一致性题型](https://www.nowcoder.com/discuss/690263)、[另一份候选人自述中的 Redis 高可用和阻塞追问](https://www.nowcoder.com/discuss/353154648989179904)；`官方事实` [Redis persistence](https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/)、[Redis 官方镜像持久化说明](https://hub.docker.com/_/redis)。

## 18. 非 root、只读根文件系统、tmpfs、`cap_drop` 和 `no-new-privileges` 分别解决什么？

- **直接回答：** 非 root 降低容器内进程默认权限；`read_only: true` 阻止写入镜像根文件系统；受限 tmpfs 只为 Nginx、API 或 Redis 的明确运行目录提供临时写空间；`cap_drop: ALL` 去掉 Linux capabilities；`no-new-privileges` 阻止进程通过 setuid 等方式获得更高权限。它们是纵深防御，不能修复应用漏洞、镜像漏洞、Docker daemon 权限或过宽网络/secret 授权。
- **优秀回答要点：** 能说出每个控制的不同层次；知道 tmpfs 必须设置路径、权限和容量；指出 MySQL 官方镜像因初始化和数据目录权限模型与其他服务不同，不能盲目套同一只读配置；理解容器隔离不等于虚拟机隔离。
- **追问：** 只读根文件系统为什么还需要 tmpfs？
  - **追问回答：** 很多正常进程需要 PID、临时请求体、代理临时文件或运行期配置。若不给明确可写路径，程序会启动失败；若取消只读，则又扩大任意写入面。最小 tmpfs 是在可运行与限制写面之间取平衡。
- **常见误区：** `USER 1000` 就等于安全；容器内 root 等于宿主机 root 或完全无害这两个极端结论；随意挂载 `/var/run/docker.sock`；使用无限 tmpfs。
- **技术延伸：** 生产还应增加 seccomp/AppArmor/SELinux、rootless runtime、只读 secret、镜像签名、资源限制和运行时检测，并核查每项控制真实生效。
- **项目证据：** [Compose 安全选项与 tmpfs](../../../deploy/compose/compose.yaml)、[Go 非 root 用户](../../../deploy/docker/Dockerfile.backend)、[Nginx 非 root 用户](../../../deploy/docker/Dockerfile.web)、[Redis 非 root 用户](../../../deploy/docker/Dockerfile.redis)。
- **选型边界：** 开发调试镜像可能需要额外工具和临时写路径，但应通过单独 target/profile 提供，避免永久放宽交付镜像。
- **来源：** `面经启发` [牛客候选人自述中的 Docker 与虚拟机、资源隔离题型](https://www.nowcoder.com/discuss/645752512930160640)；`官方事实` [Compose `read_only`、`init` 与 security options 参考](https://docs.docker.com/reference/compose-file/services/)、[Docker security](https://docs.docker.com/engine/security/)。

## 19. 为什么开发环境统一使用 `restart: "no"`？这和“系统要能恢复”矛盾吗？

- **直接回答：** 本地学习环境优先让第一次失败及退出码清晰可见，避免错误配置被无限重启刷屏或短暂恢复掩盖。`restart: "no"` 表示容器退出后不由 Docker 自动拉起；它不妨碍应用在进程仍存活时通过数据库连接池从 MySQL 短暂断连中恢复。两种恢复语义不同：进程重启由编排策略决定，依赖重连由应用负责，都需要单独验证。
- **优秀回答要点：** 区分 crash recovery 与 dependency recovery；指出自动重启不是根因修复；知道 `depends_on` 不做运行期级联恢复；能描述 QA 应记录 API 容器 ID，在 MySQL stop/start 后验证 `/ready` 恢复且 API 未重启。
- **追问：** 生产是否应该全部设置 `always`？
  - **追问回答：** 不能一刀切。常驻无状态服务可由编排器按策略重启；迁移任务失败通常应停住并告警；永久配置错误需要退避和失败上限。生产更应由平台控制器、探针、告警和发布状态共同处理，而不是只靠 Docker restart policy。
- **常见误区：** 重启成功就说故障自愈；用无限重启掩盖 migration 错误；MySQL 恢复后强制重启 API，而没有验证连接池能否自行恢复；把容器 healthy 当没有历史故障。
- **技术延伸：** 应增加错误分类、重试预算、jitter、熔断和恢复时间指标；容器级重启次数要进入观测，而非只看当前状态。
- **项目证据：** [显式 restart 策略](../../../deploy/compose/compose.yaml)、[MySQL 连接池实现](../../../internal/infrastructure/mysql)。
- **选型边界：** 教学/调试环境适合 fail visible；无人值守服务需要经过设计的自动重启，但必须配合限频、告警和健康判断。
- **来源：** `面经启发` [牛客运维候选人自述中的后端宕机、熔断与排障场景](https://www.nowcoder.com/discuss/792889897148248064)；`官方事实` [Compose restart policy](https://docs.docker.com/reference/compose-file/services/#restart)、[Compose 启动依赖边界](https://docs.docker.com/compose/how-tos/startup-order/)。

## 20. Apple Silicon/arm64 上怎样避免“我电脑能跑，CI 跑不了”？当前方案做到了什么、没做到什么？

- **直接回答：** 当前基础镜像选择提供 arm64 变体的官方镜像；Go builder 使用 `FROM --platform=$BUILDPLATFORM`，并把 BuildKit 的 `TARGETOS/TARGETARCH` 传给 `GOOS/GOARCH`，关闭 CGO 后可以为目标 Linux 平台编译静态二进制。本地 Docker Desktop 会优先拉取原生 arm64，避免不必要的 QEMU 模拟。但当前 Compose 普通 build 只证明本机平台镜像，尚未宣称已经发布 amd64+arm64 的 multi-platform manifest。
- **优秀回答要点：** 区分构建器平台、目标平台和运行平台；说明 JavaScript 静态产物跨 CPU，而 Nginx/Node base image 仍需对应架构；知道 `platform: linux/amd64` 在 arm64 上可能走模拟且更慢；不把“tag 支持多架构”与“自建镜像已推送多架构”混淆。
- **追问：** 如何真正产出并验证双架构镜像？
  - **追问回答：** 使用 `docker buildx build --platform linux/amd64,linux/arm64`，推送 registry manifest list，再分别在原生或受控模拟环境运行 smoke test；还要检查所有 base image和依赖是否支持目标平台。本章若 QA 没记录这组命令，就只能说 Dockerfile 为交叉构建预留了参数。
- **常见误区：** 强制 amd64 后把 QEMU 成功当原生验证；忘记 CGO/native addon；只看 Dockerfile 参数就声称发布了多架构镜像；把 macOS 二进制复制进 Linux 镜像。
- **技术延伸：** CI 可采用矩阵原生 runner 或 Buildx cache，生成 SBOM/签名，并比较两架构镜像内容与性能差异。
- **项目证据：** [Go 构建平台参数](../../../deploy/docker/Dockerfile.backend)、[固定且支持 arm64 的服务镜像声明](../../../deploy/compose/compose.yaml)、[Web 基础镜像](../../../deploy/docker/Dockerfile.web)。
- **选型边界：** 团队和生产全是单一架构时可先只交付一个平台；一旦开发机和部署机架构不同，应把 multi-platform build/test 纳入 CI。
- **来源：** `官方事实` [Docker multi-platform builds](https://docs.docker.com/build/building/multi-platform/)、[Redis 官方镜像支持架构列表](https://hub.docker.com/_/redis)。

## 21. `ENTRYPOINT`、`CMD` 和 Compose `init: true` 怎样影响 PID 1、参数覆盖与优雅退出？

- **直接回答：** Go API 镜像用 exec-form `ENTRYPOINT` 固定可执行文件；迁移镜像再用 `CMD ["up"]` 提供可覆盖的默认子命令，所以 Compose 临时任务可以改成 `status` 等参数。Web 的 Nginx同样用 exec-form 前台运行。`init: true` 在容器里加入一个轻量 init，负责转发信号和回收孤儿进程；`stop_grace_period` 给进程处理 SIGTERM/SIGQUIT 的窗口，超时后平台才会强制终止。
- **优秀回答要点：** 说明 exec form 避免无必要 shell 截获信号；区分 ENTRYPOINT 的可执行主体与 CMD 的默认参数；指出 Go 自己仍需监听信号并做有界 shutdown；`init` 不是服务管理器，也不能让不处理信号的应用自动优雅。
- **追问：** 为什么 migration 的默认 `up` 放在 CMD 而不是写死进 ENTRYPOINT？
  - **追问回答：** 这样镜像身份仍是 `growth-migrate`，默认行为是 `up`，但可自然覆盖为 `status` 或受支持子命令，不需要执行 shell 字符串或构建多个镜像。
- **常见误区：** shell-form ENTRYPOINT 导致信号只到 shell；把多个常驻进程塞进一个容器再靠 init 管理；认为 `docker stop` 立即 SIGKILL；把 `init: true` 当自动重启。
- **技术延伸：** 可做信号演练：发起请求时停止 API，记录拒绝新请求、在途请求完成、数据库连接关闭及退出耗时；结果仍必须以 QA 为准。
- **项目证据：** [Go ENTRYPOINT/CMD](../../../deploy/docker/Dockerfile.backend)、[Nginx 前台进程](../../../deploy/docker/Dockerfile.web)、[`init` 与 stop grace](../../../deploy/compose/compose.yaml)、[API shutdown](../../../cmd/growth-api/main.go)。
- **选型边界：** 单一正确 PID 1 且无子进程的程序可能不必额外 init；涉及 shell wrapper 或会产生子进程时，信号转发和回收必须验证。
- **来源：** `面经启发` [牛客 Docker 题型汇总帖；属于社区整理而非官方题库](https://www.nowcoder.com/discuss/949020)；`官方事实` [Dockerfile `ENTRYPOINT` 与 `CMD`](https://docs.docker.com/reference/dockerfile/#entrypoint)、[Compose `init`](https://docs.docker.com/reference/compose-file/services/#init)。

## 22. 为什么 Compose 运行 Web 时使用 Nginx，而不是 Vite dev server 或 `vite preview`？

- **直接回答：** Vite dev server 面向开发期模块转换和 HMR，`vite preview` 用于本地预览构建结果；二者都不承担正式静态资源网关角色。Compose 镜像在 builder 阶段生成静态 `dist`，运行阶段用非 root Nginx 提供 SPA fallback、不可变资产缓存、`index.html` no-store、容器自身 health 和同源反向代理。宿主机需要 HMR 时仍使用 `pnpm dev`，两种工作流职责不同。
- **优秀回答要点：** 不是简单说“Nginx 性能更好”，而是说运行职责、可控配置和同源边界；能解释 hashed assets 长缓存、入口 HTML 不缓存的原因；知道 `try_files ... /index.html` 支持前端路由，但 `/api` 与探针必须在它之前单独代理。
- **追问：** 为什么 Nginx 的 Docker DNS resolver 要配变量 `proxy_pass`？
  - **追问回答：** Compose 重建 API 后 IP 可变，服务名稳定。变量形式配合 `127.0.0.11` resolver 使请求时重新解析，避免 Nginx 在启动时固定旧 IP；这不替代应用级超时、错误处理或连接恢复验证。
- **常见误区：** 把 `vite preview` 当生产 server；对 `index.html` 设置一年缓存导致发布后入口陈旧；SPA fallback 吞掉 `/api/404` 并返回 HTML 200；把代理 502 解释成前端构建失败。
- **技术延伸：** 生产可由 CDN/托管静态站点与独立 API gateway 替代同容器 Nginx；需重新设计缓存失效、TLS、压缩、CSP、限流和观测。
- **项目证据：** [Web 多阶段构建](../../../deploy/docker/Dockerfile.web)、[Nginx 路由、缓存与动态 DNS](../../../deploy/docker/nginx.conf)、[宿主机 Vite 配置](../../../web/vite.config.ts)。
- **选型边界：** 纯本地 HMR 不应每次重建 Nginx 镜像；可交付的容器环境则不应依赖 dev server。
- **来源：** `官方事实` [Vite 静态部署说明及 `vite preview` 边界](https://vite.dev/guide/static-deploy.html)、[Compose 服务 IP 可变和服务名稳定](https://docs.docker.com/compose/how-tos/networking/)、[Nginx `try_files`](https://nginx.org/en/docs/http/ngx_http_core_module.html#try_files)。

## 23. `.dockerignore` 为什么是安全边界的一部分？它和 `.gitignore` 有什么不同？

- **直接回答：** Docker build context 会发送给 builder，Dockerfile 的 `COPY` 可访问其中内容；`.dockerignore` 在发送前排除 Git 历史、编辑器文件、`node_modules`、`dist`、日志、`.env` 和本地 secret，既减少上下文与缓存失效，也降低凭据误进 image layer 的风险。`.gitignore` 只控制 Git 跟踪，不自动阻止 Docker 把未跟踪文件发送到构建器。
- **优秀回答要点：** 能说明 build context、layer history 和缓存三层风险；指出即使后续 `RUN rm secret`，旧 layer 仍可能含内容；知道 BuildKit secret mount 适合构建期私密依赖，不能用 `ARG/ENV` 传长期 secret。
- **追问：** 已经有 `.dockerignore`，Dockerfile 中 `COPY . .` 就可以无条件放心了吗？
  - **追问回答：** 仍不理想。ignore 规则可能漏项或被修改，精确 `COPY` 能形成第二层白名单并改善缓存。GrowthOS Go 镜像只复制 go module、`cmd/internal/migrations`，Web 镜像也按依赖与源码分步复制。
- **常见误区：** 依赖 `.gitignore`；把 secret 复制后删除；用 build arg 传密码后以为最终 `env` 看不到就安全；把整个用户目录作为 build context。
- **技术延伸：** CI 可扫描 image history、SBOM 和 secret pattern，并限制远程 builder 的上下文访问；供应链场景还需签名与 provenance。
- **项目证据：** [Docker build context 排除规则](../../../.dockerignore)、[Go 精确 COPY](../../../deploy/docker/Dockerfile.backend)、[Web 分层 COPY](../../../deploy/docker/Dockerfile.web)。
- **选型边界：** 构建必须访问私有模块时，使用 BuildKit SSH/secret mount 或受控凭据代理，不应放宽 ignore 把密钥复制进去。
- **来源：** `面经启发` [牛客候选人自述中的 Dockerfile 常见指令和外部挂载题型](https://www.nowcoder.com/discuss/353154648989179904)；`官方事实` [Docker build context](https://docs.docker.com/build/concepts/context/)、[Build secrets](https://docs.docker.com/build/building/secrets/)。

## 24. 容器启动失败或浏览器出现 502/503 时，你会怎样按证据排障？

- **直接回答：** 先看 `docker compose config --quiet` 排除变量、路径与 YAML 问题；再用 `compose ps` 判断是 created、running、unhealthy 还是 exited；一次性迁移先看退出码和日志。浏览器路径按“宿主机 `127.0.0.1:8088` → Web 自身 `/container-health` → Nginx 代理响应 → API `/health` → API `/ready` → MySQL health/status”逐层缩小。网络问题再检查服务是否加入预期 network、使用服务名和容器端口；凭据问题只看稳定错误分类，不打印 secret。
- **优秀回答要点：** 先分类后操作；能用 502 表示网关无法获得有效上游响应、503 合法 Go envelope 表示当前 readiness 失败；使用 request ID 对齐 Nginx/API 日志；检查 migration 是否成功，而不是一上来重建所有容器或删除卷。
- **追问：** API `/health` 200、`/ready` 503 时，最先查什么？
  - **追问回答：** 这说明 API 进程能响应，优先检查 MySQL 容器 health、API 到 `mysql:3306` 的网络、应用 secret、账号权限和 Ping timeout；不要先重建前端。若 Go 返回的 503 envelope 和 request ID 不可信，则还要排查代理或契约漂移。
- **常见误区：** `docker compose down -v` 当万能修复；在宿主机 curl `mysql:3306`；把所有 503 都归因 MySQL；只看“container running”不看 health/exit code；把完整 `docker inspect` 含敏感配置贴到公共渠道。
- **技术延伸：** 建立故障矩阵和最小证据包：时间、命令、service/container ID、health log、request ID、响应头/体、相关日志窗口和恢复步骤，避免不可复现的“重启后好了”。
- **项目证据：** [Compose 运维命令](../../../Makefile)、[服务 health 与日志轮转](../../../deploy/compose/compose.yaml)、[Nginx 超时与代理](../../../deploy/docker/nginx.conf)、[第 16 节 QA](../../qa/lessons/lesson-16.md)。
- **选型边界：** 本地 `docker compose exec` 适合单机排障；生产应通过受控日志、metrics、trace 和临时调试容器，不能默认开放 shell 或数据库端口。
- **来源：** `面经启发` [牛客运维候选人自述中的故障、熔断和排障题型](https://www.nowcoder.com/discuss/792889897148248064)、[候选人自述中的 Nginx 502/504 与日志排查](https://www.nowcoder.com/discuss/353154648989179904)；`官方事实` [Compose network debugging](https://docs.docker.com/compose/how-tos/networking/#debugging)、[Compose logs](https://docs.docker.com/reference/cli/docker/compose/logs/)。

## 25. 你会怎样设计 MySQL、API、Redis 分别故障时的恢复演练？哪些结论不能靠一次演练推出？

- **直接回答：** 正常基线先记录 Web、`/health`、`/ready`、容器 ID 与状态。停止 MySQL 时预期 API 进程仍在、`/health` 仍能响应、`/ready` 变 503；重新启动 MySQL 后验证 readiness 在不重启 API 的情况下恢复，并核对 API 容器 ID。停止 API 时预期 Web 静态 health 仍成功，代理返回网关错误；恢复 API 后代理重新解析服务名。停止 Redis 时，当前 API 应完全不受影响，因为尚无网络或代码依赖。所有实际结果、恢复时间和异常只记录在 QA，不在设计文档预填。
- **优秀回答要点：** 每个演练都有前置、故障动作、预期、证据、恢复和清理；验证“未重启”而不只看最终 200；区分预期设计与真实结果；保护用户已有容器、端口和卷，不把其他 Docker Desktop 数据纳入清理。
- **追问：** 本地一次 stop/start 恢复成功，能否说系统高可用？
  - **追问回答：** 不能。它只证明该版本、单机、单实例、特定故障动作下的恢复路径。高可用还涉及副本、故障检测时间、流量切换、数据复制、一致性、持续故障、资源耗尽和 SLO，需要更大范围验证。
- **常见误区：** 只验证最终成功不验证降级过程；故障时顺手重启 API，无法证明连接池恢复；停止 Redis 后仍宣称验证了“缓存降级”——当前根本没有缓存调用；把设计目标写成实测数字。
- **技术延伸：** 后续可将矩阵自动化为 integration/chaos smoke，并记录 RTO 分布、错误率和请求 ID；但自动化脚本也必须有作用域和安全保护，不能误停用户其他容器。
- **项目证据：** [隔离的服务依赖与网络](../../../deploy/compose/compose.yaml)、[API readiness](../../../internal/infrastructure/httpapi/readiness.go)、[第 16 节 QA 证据](../../qa/lessons/lesson-16.md)。
- **选型边界：** 本地手工演练适合学习故障语义；持续交付需要可重复自动化和环境隔离，生产 chaos 还需审批、停止条件和监控。
- **来源：** `面经启发` [牛客候选人自述中的后端节点故障与雪崩场景](https://www.nowcoder.com/discuss/792889897148248064)、[候选人自述中的项目难点与故障排查方向](https://www.nowcoder.com/discuss/645752512930160640)；`官方事实` [Compose 服务生命周期](https://docs.docker.com/reference/cli/docker/compose/)、[Compose 网络重建与重连责任](https://docs.docker.com/compose/how-tos/networking/#update-containers-on-the-network)。

## 26. 日志、健康检查和压测各能证明什么？为什么不能用“小镜像”推导“高性能”？

- **直接回答：** JSON 日志和有限轮转让本地故障可关联且避免日志无限占盘；healthcheck 只证明周期命令当时成功；压测才观察特定环境、请求模型和持续时间下的吞吐、延迟与错误。多阶段构建和较小运行镜像主要改善分发、启动依赖和攻击面，不能直接证明 Go handler、MySQL 查询或端到端链路更快。本章任何 RPS、p95/p99、错误率和资源数据都只能引用 QA 的实际命令和结果，未执行就明确说未验证。
- **优秀回答要点：** 压测前写清 endpoint、并发模型、预热、持续时间、机器架构、Docker 资源、成功判定和采样；`/health` 负载只能测最小 HTTP 路径，不能代表业务 API 或数据库容量；同时记录 CPU、内存、连接池、容器重启和错误响应。
- **追问：** 对 `/health` 跑到很高 QPS，简历能否写“系统支持高并发”？
  - **追问回答：** 不能。探针通常不访问业务存储，也没有鉴权、序列化和业务锁争用；只能表述为“在某环境对健康端点进行基线测量，结果见 QA”。要支持业务容量结论，需要代表性 workload、数据规模、瓶颈分析和可复现报告。
- **常见误区：** 只报平均延迟；忽略错误请求；本机压测 client 与 server 抢资源却不说明；短时 burst 当稳定吞吐；把冷缓存和热缓存数据混在一起；根据镜像大小推断运行性能。
- **技术延伸：** 后续可分 M0 探针基线、M1 单业务读写、M2 混合负载，增加 p50/p95/p99、错误预算、资源曲线和瓶颈实验；不要为了漂亮数字删掉失败样本。
- **项目证据：** [JSON 日志轮转配置](../../../deploy/compose/compose.yaml)、[API 探针](../../../internal/infrastructure/httpapi/router.go)、[第 16 节真实验证记录](../../qa/lessons/lesson-16.md)。
- **选型边界：** 本章仅需建立本地基线和方法；生产容量规划要在接近真实数据、实例规格、网络、数据库和流量分布的环境完成。
- **来源：** `面经启发` [牛客候选人自述中出现“秒杀优化、压测 QPS”并继续追问缓存/数据库的场景](https://www.nowcoder.com/discuss/690263)；`官方事实` [Docker JSON-file logging driver 与轮转注意事项](https://docs.docker.com/engine/logging/drivers/json-file/)、[Compose healthcheck](https://docs.docker.com/reference/compose-file/services/#healthcheck)。

## 27. 这套 Compose 能否直接用于生产？从本地到生产最少还缺哪些决策？

- **直接回答：** 不能直接把本地 Compose 等同生产方案。它缺少生产 Secret Manager 与轮换、TLS/域名/WAF、镜像 registry 与签名、资源 requests/limits、数据库备份恢复和高可用、Redis 真实业务语义、集中日志指标追踪、告警/SLO、滚动发布、漏洞修复、灾备以及多实例迁移协调。可以复用的是经过验证的进程边界、同源路由、探针语义、最小权限和镜像构建思路，而不是原样复制端口、密码文件和单机卷。
- **优秀回答要点：** 既不过度贬低本地方案，也不包装成生产；用“可复用原则”和“必须重做的平台绑定”分类；能说明生产数据库通常优先托管服务，schema migration 仍需独立 job；明确任何 SLA 都需要生产架构与证据。
- **追问：** 那为什么还值得把本地 Compose 做得这么严谨？
  - **追问回答：** 因为错误的权限、假 readiness、secret 泄漏、启动竞态和不可恢复数据卷会从开发一直带到交付。严谨本地环境能提前暴露契约和边界，也给 CI 集成测试提供一致基础，但不会自动补齐生产控制面。
- **常见误区：** “开发环境不用安全”；“容器化后去哪都一样”；把 Compose `deploy` 字段当完整生产平台；因为未来要 K8s 就忽略当前可复现性。
- **技术延伸：** 可以制作环境提升清单：immutable image、外部配置/secret、托管状态服务、Kubernetes Deployment/Job/Service/Ingress、probe、PDB、autoscaling、observability、backup/restore drill 与 supply-chain policy。
- **项目证据：** [当前单机开发拓扑](../../../deploy/compose/compose.yaml)、[镜像构建边界](../../../deploy/docker/Dockerfile.backend)、[Nginx 入口](../../../deploy/docker/nginx.conf)。
- **选型边界：** 内部单机 demo 可直接使用 Compose；面对真实用户、敏感数据或可用性承诺时，必须走单独生产架构评审。
- **来源：** `面经启发` [牛客候选人自述中从容器化开发环境继续追问 K8s、扩容和监控](https://www.nowcoder.com/discuss/353158027186479104)、[运维岗位候选人自述中的 Docker/K8s 和故障场景](https://www.nowcoder.com/discuss/792889897148248064)；`官方事实` [Docker Compose 生产使用注意事项](https://docs.docker.com/compose/how-tos/production/)、[Docker security](https://docs.docker.com/engine/security/)。

## 不能夸大的事实

- 本节实现的是单机 Docker Desktop/Engine 上的开发 Compose 环境，不是 Kubernetes、Swarm 或生产集群。
- `compose.yaml` 声明五个服务，不等于五个业务模块均已实现；Redis 尚未被 Go 代码使用。
- Redis 当前关闭 RDB/AOF 并使用 tmpfs，重建会丢数据；不能写成“Redis 持久化已完成”或“缓存数据安全”。
- Redis 不在 API 网络、不参与 readiness；停止它只能验证当前“无依赖”，不能证明缓存降级逻辑。
- MySQL 使用命名卷只代表跨容器重建持久化，不代表已有备份、恢复演练、复制或高可用。
- MySQL init 脚本只作用于空数据目录；修改脚本不会自动更新旧卷里的用户和权限。
- 应用账号、迁移账号和 root 已分离，但这只是最小权限的一部分，不代表 SQL 注入、网络或凭据风险消失。
- Compose file-backed secrets 的宿主机源文件仍是明文；它不是 Vault、KMS、动态凭据或自动轮换系统。
- secret 生成脚本防止部分集合和旧卷错配，不等于已经实现无停机密码轮换。
- `depends_on` 的 condition 约束启动阶段，不持续监控运行期依赖，也不自动完成级联恢复。
- API 容器检查 `/health`，MySQL 故障时仍可能 healthy；流量可接收性要看 `/ready`。
- MySQL 的 `SELECT 1` healthcheck 验证一条最小认证查询路径，不能证明全部表、索引、事务或业务查询正常。
- Web 容器自身 healthy 不代表 API healthy；这正是保留静态故障页面的设计结果。
- `internal: true` 不等于绝对隔离；同时加入非内部网络的服务仍有其他通信路径。
- 只发布回环 Web 端口降低本机外暴面，但不等于已经具备认证、TLS、CSRF 防护或生产安全。
- 非 root、只读根、`cap_drop` 与 `no-new-privileges` 是纵深防御，不代表容器无法被利用。
- `restart: "no"` 是开发环境的 fail-visible 选择；不能据此宣称无人值守自愈。
- 多阶段构建、固定版本 tag 和 lockfile 提高可复现性，但没有锁 digest 时不是完全不可变构建。
- Dockerfile 的 `TARGETARCH` 支持只是多架构构建准备；若 QA 没有 buildx 和跨架构运行记录，不能说已经发布双架构镜像。
- Nginx 代理与 Vite host 模式承担不同职责；本地 Nginx 可运行不等于生产网关、CDN 或 WAF 已完成。
- healthcheck、单元测试、构建、浏览器 smoke、故障演练和压测分别证明不同风险，不能互相替代。
- 没有 QA 中可复现的 workload 和结果，就不能陈述任何 QPS、p95/p99、错误率、恢复时间或资源收益。
- 小镜像不等于高吞吐；`/health` 压测结果也不能代表真实业务 API 容量。
- 牛客链接是用户自述或社区整理，只用于题型启发，不是企业官方题库或原题认证。

## 复习清单

- [ ] 能在 60 秒内讲清五服务、三网络、四 secrets、一个 MySQL 卷和唯一 Web 入口。
- [ ] 能分别画出请求链路、启动链路、迁移链路和数据链路，而不是只列容器名。
- [ ] 能解释为什么选 Compose、为什么此时不直接上 Kubernetes，以及迁移触发条件。
- [ ] 能说明 Go 与 Web 多阶段构建、target、layer cache、cache mount、lockfile、tag 和 digest 的差别。
- [ ] 能准确说出 `service_started`、`service_healthy`、`service_completed_successfully`，并说明运行期边界。
- [ ] 能解释 `/health`、`/ready`、MySQL `SELECT 1` 和 Web `/container-health` 各自证明什么。
- [ ] 能说明 Web 为什么不依赖 API，以及 Nginx 如何在 API 重建后重新解析服务名。
- [ ] 能区分容器内 localhost、宿主机 localhost、服务名、容器 IP、host port 和 container port。
- [ ] 能画出 `edge/data/cache` 网络成员，并解释 `internal: true` 不是绝对隔离。
- [ ] 能解释为什么只向 `127.0.0.1` 发布 Web，不永久发布 MySQL、Redis 或 API。
- [ ] 能说明 Compose secrets 的收益与限制、Go `_FILE` 互斥规则及不可打印的配置边界。
- [ ] 能推演“secret 丢失但 MySQL 卷保留”为什么不能重新随机生成，并描述正确轮换思路。
- [ ] 能解释 MySQL init 只运行一次、命名卷生命周期和 `down -v` 的破坏性。
- [ ] 能说明 root、migrator、app 的权限差异，以及 `SELECT 1` 不能证明全部业务权限。
- [ ] 能诚实说明 Redis 尚未接入业务、当前不持久化、停止 Redis 不影响 API 的原因。
- [ ] 能比较 RDB、AOF、无持久化的语义，而不是背诵一个“最佳方案”。
- [ ] 能逐项解释非 root、只读根、tmpfs、capability 和 no-new-privileges 的边界。
- [ ] 能区分容器 crash restart 与应用依赖重连，设计 MySQL stop/start 不重启 API 的证据。
- [ ] 能解释 BUILDPLATFORM/TARGETPLATFORM、arm64 原生镜像、QEMU 与真正 multi-platform 发布的差别。
- [ ] 能说明 ENTRYPOINT、CMD、PID 1、`init: true`、stop grace 与应用优雅退出的关系。
- [ ] 能解释为什么 Compose Web 用 Nginx、宿主机 HMR 用 Vite，以及 SPA fallback 不能吞 API 错误。
- [ ] 能解释 `.dockerignore` 与 `.gitignore` 的不同，知道删除 secret 不会从旧 image layer 消失。
- [ ] 能沿 config → ps/health/exit → Web → proxy → API health/ready → MySQL → network/secret 逐层排障。
- [ ] 能设计 MySQL、API、Redis 三组故障演练，并把预期与真实 QA 结果分开。
- [ ] 能说明日志、探针、smoke、故障演练和压测各自能证明什么，拒绝编造漂亮数字。
- [ ] 能列出从本地 Compose 到生产至少还需补齐的 Secret、TLS、HA、备份、发布、观测和容量决策。
- [ ] 面试前逐条检查“不能夸大的事实”，只引用当前提交与 QA 真正支持的结论。
