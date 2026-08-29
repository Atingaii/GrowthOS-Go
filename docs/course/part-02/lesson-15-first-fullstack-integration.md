# 第 15 节：前后端第一次联调

**状态：** 已完成并验收

**日期：** 2026-08-29

**阶段：** Go + React 从零搭建

**本节只把系统状态页接到已有的 `GET /health` 与 `GET /ready`，建立浏览器到 Go API 的第一条真实链路；不新增业务 API，不把 MySQL readiness 解释为业务、集群或 SLA 全部正常。**

## 1. 为什么第一次联调选择系统探针

第 14 节已经建立 React 页面、路由、主题和 Mock 数据边界，第 11～13 节已经建立 Gin、请求 ID、统一错误、MySQL 连接池与两个系统探针。如果第一次联调直接从抽奖或活动开始，就会同时引入领域模型、业务表、鉴权、事务和前端请求基础设施，任何故障都很难定位是网络、契约还是业务逻辑导致。

`/health` 与 `/ready` 适合作为最小垂直切片，因为它们已经有稳定且不同的语义：

- `/health` 只回答当前 Go 进程能否处理 HTTP 请求；
- `/ready` 只回答当前实例能否连接 MySQL、是否适合接收依赖数据库的流量；
- 两者都返回版本、服务端时间、`Cache-Control: no-store` 与 `X-Request-ID`；
- readiness 失败已经使用统一的 503 error envelope；
- 不需要为“看见一次真实请求”提前创建虚假业务表或资源接口。

本节因此验证的是一条可复用的集成骨架：浏览器发起请求，Vite 只在本地开发/预览环境代理，同源路径到达 Gin，中间件生成请求 ID，handler 返回稳定 JSON，前端执行运行时解码并形成可解释的 UI 状态。

## 2. 本节范围与非目标

### 2.1 交付范围

- Vite development 与 preview 都代理 `/health`、`/ready` 和未来 `/api` 前缀；
- 代理目标使用仅在 Node/Vite 进程读取的 `GROWTHOS_WEB_API_PROXY_TARGET`；
- 默认只监听 `127.0.0.1`，端口冲突时失败而不是自动漂移；
- 浏览器始终请求 `/health` 与 `/ready` 同源路径，不感知 Go API 的主机和端口；
- 使用原生 `fetch` 建立统一 JSON client，处理 timeout、取消、网络失败、非 2xx 和契约错误；
- TypeScript 静态类型之外再做响应的运行时解码；
- 读取并展示 `X-Request-ID`，同时校验 header 与错误 body 中的 request ID 不冲突；
- `/health` 和 `/ready` 并行、独立更新，刷新或卸载时取消旧请求，旧一轮结果不能覆盖新一轮；
- 系统状态页只展示 Go API 与 MySQL readiness 两张真实探针卡片；
- Go 的所有统一错误响应补充 `Cache-Control: no-store`，防止带关联 ID 的错误被缓存重放；
- 前端质量门禁增加 Vitest 组件/单元测试入口，并让 `web-verify` 顺序执行 test、typecheck 和 build。

### 2.2 明确不做

- 不新增抽奖、活动、积分、优惠券、Feed、MCP 或 Agent 业务 API；
- 不创建业务表或 Migration；
- 不接入 Redis、RabbitMQ/RocketMQ、PostgreSQL 或其他依赖；
- 不为浏览器直连 Go API 开放宽泛 CORS；
- 不新增 `/api/v1/health` 或 `/api/v1/ready` 别名；
- 不把代理当作生产部署方案；生产环境仍需要由同源反向代理或网关承接静态资源和 API；
- 不引入 Axios、TanStack Query 或把这两个服务端探针放入 Zustand；
- 不做自动轮询、后台常驻监控、告警、SLA 计算或多实例聚合；
- 不把 readiness 成功夸大为数据库性能、Migration 版本、业务数据或整个集群均正确；
- 不把 Mock 业务页面改写为“已经联调”。

## 3. 第一次真实链路

```text
Browser: GET /health, GET /ready
             │
             │ same-origin
             ▼
Vite dev / preview server :5173 / :4173
             │ exact prefix proxy, no rewrite
             ▼
Go API :8080
  ├─ request ID middleware
  ├─ GET /health ──────> process liveness
  └─ GET /ready ───────> bounded MySQL PingContext
             │
             ▼
JSON + X-Request-ID + Cache-Control: no-store
             │
             ▼
runtime decoder → independent probe states → truthful status UI
```

本地开发中的 Vite proxy 是开发便利和同源契约的适配器，不是安全边界，也不改变 Go 路由。浏览器地址栏仍位于 Vite origin，请求路径不重写；Go 看到的仍是 `/health`、`/ready` 或 `/api/...`。

## 4. 同源代理边界

### 4.1 为什么不让浏览器直接访问 `http://127.0.0.1:8080`

页面位于 `http://127.0.0.1:5173` 时，直接请求 `:8080` 已经跨 origin，需要服务端配置 CORS。第一次联调没有跨站产品需求，提前开放 CORS 会增加 allow-origin、credentials、preflight、暴露 header 和部署环境差异等问题，还容易用 `*` 掩盖真实拓扑设计。

本节让浏览器只请求当前 origin 下的路径。本地由 Vite 转发；未来生产由正式反向代理或网关保持相同路径契约。这样调用代码不包含开发端口，也不会把后端拓扑编译进前端 bundle。

### 4.2 代理目标配置

默认目标：

```text
http://127.0.0.1:8080
```

需要覆盖时，在 `web/.env.local` 或运行环境中设置：

```dotenv
GROWTHOS_WEB_API_PROXY_TARGET=http://127.0.0.1:8080
```

该变量没有 `VITE_` 前缀，意图是只供 `vite.config.ts` 所在的 Node 进程读取，不暴露给浏览器代码。配置只接受 HTTP(S) origin，并拒绝用户名、密码、非根路径、query 和 fragment。它只应该描述代理 upstream，不能承载 Secret。

开发和 preview 使用相同代理表，避免 `vite dev` 正常而 `vite preview` 404。二者均绑定 loopback、启用 `strictPort`，使端口占用成为显式失败；第 16 节如需 Docker 容器对外监听，应由容器网络需求显式修改，而不是本节默认扩大暴露面。

## 5. 统一原生 fetch 边界

`web/src/api/httpClient.ts` 集中处理跨领域都相同的 HTTP 机制，领域文件只负责路径与运行时解码。每个请求当前固定：

- 只接受以单个 `/` 开始的同源绝对路径，拒绝绝对 URL、`//host` 与可被 URL 解析器归一化为 host 分隔符的反斜杠；
- 方法为 `GET`；
- 请求头包含 `Accept: application/json`；
- `cache: "no-store"`，避免探针被浏览器缓存；
- `credentials: "same-origin"`，不会向跨源目标发送凭证；
- `mode: "same-origin"`，禁止请求模式越过当前 origin；
- `redirect: "error"`，不跟随后端 30x 跳到未授权 origin；
- 默认 timeout 为 5 秒，可配置范围为 100 ms～30 秒；
- 使用 `AbortController` 合并调用方取消与内部 timeout；
- 使用单调时钟记录浏览器观察到的真实往返时间，而不是后端伪造“延迟”；
- 非 JSON、非法 JSON、字段结构不符合预期通常归为 contract error；同源代理返回非 JSON 的 502/503/504 是明确的 gateway 例外；
- 非 2xx 只有在符合统一 error envelope 时才归为 HTTP error；非 JSON 502/503/504 则归为 gateway，不冒充 Go error envelope；
- 错误响应 header 与 body 都携带 request ID 时必须一致，否则 fail closed 为 contract error；
- 不把原始响应 body、网络错误对象、数据库错误或代理目标渲染给用户。

### 5.1 前端错误分类

| `kind` | 触发条件 | UI/排查含义 |
| --- | --- | --- |
| `http` | 收到非 2xx，且 body 符合统一 error envelope | 服务明确拒绝或不可用，可使用 status、code、request ID 排查 |
| `gateway` | 同源代理返回非 JSON 的 502/503/504 | 浏览器到达代理，但代理未取得可信 API 响应；不能误写成后端契约错误 |
| `network` | `fetch` 没有产生 HTTP response | 浏览器到当前 origin 的连接中断、DNS/TLS 或其他传输失败；不能证明 API 或 MySQL 的具体状态 |
| `timeout` | 本地 5 秒预算耗尽 | 浏览器主动取消该次等待，不证明服务端一定停止执行 |
| `cancelled` | 页面卸载或刷新时调用方取消 | 生命周期控制，不应显示成一次新的服务故障 |
| `contract` | 路径/timeout 配置非法，或响应 Content-Type/JSON/字段/request ID 不可信 | 前后端契约或环境路由需要修复，不能用 TypeScript 类型断言掩盖 |

错误 `message` 只适合作为用户可读说明；程序分支使用稳定 `kind`、HTTP `status` 和服务端 `code`。成功响应则返回已解码数据、HTTP status、可选 request ID 和浏览器 elapsed time。

## 6. 为什么 TypeScript 类型还不够

`HealthResponse` 和 `ReadinessResponse` 能约束前端源码如何使用数据，却不能让网络上的任意 JSON 自动变成该类型。服务端版本漂移、代理命中错误页面、字段为空或时间戳非法时，直接 `response.json() as HealthResponse` 会让错误值穿过类型系统。

本节为两个探针执行运行时检查：

| 端点 | 必须满足 |
| --- | --- |
| `/health` | object；`status === "ok"`；`version` 为非空 string；`timestamp` 具有 RFC 3339 形状且可解析 |
| `/ready` | object；`status === "ready"`；`version` 为非空 string；`timestamp` 具有 RFC 3339 形状且可解析 |

响应可以包含额外字段，以允许服务端做兼容性扩展；当前必需字段缺失或错误则拒绝。运行时 decoder 与 TypeScript interface 放在同一领域 API 模块，使调用方不能绕过契约边界。

## 7. 两个探针必须独立

`useSystemStatus` 在同一刷新动作中并行发出两个请求，但不使用一个共同的“成功/失败”布尔值：

```text
refresh generation N
  ├─ /health ──> loading | success | error
  └─ /ready  ──> loading | success | error
        │
        └─ 两个 settled 后记录本轮完成时间
```

原因是最有价值的故障事实恰好是“进程仍活着，但 MySQL 未就绪”。如果使用 `Promise.all` 只保留一个 rejection，或者在任一失败时清空另一个结果，就会破坏 liveness/readiness 分离的设计。

页面初始化时检查一次，用户可以手动刷新，但本节不自动轮询。刷新会：

1. 增加 generation；
2. 取消上一轮 controller；
3. 把两个探针分别重置为 loading；
4. 并行启动新请求；
5. 只有 generation 仍匹配且没有取消的回调才能更新状态。

组件卸载时同样增加 generation 并取消 controller。这同时处理 React Strict Mode 开发环境的 setup/cleanup 探测、快速连续刷新和慢旧响应覆盖新结果的问题。

## 8. 页面如何解释结果

| liveness | readiness | 页面结论 | 不能推导 |
| --- | --- | --- | --- |
| 成功 | 成功 | 当前 API 实例可响应，MySQL 连接就绪 | 业务正确、集群全部健康、SQL 性能达标 |
| 成功 | 503 `dependency_unavailable` | API 存活，MySQL 未就绪；实例不应接业务流量 | 进程已经崩溃 |
| 成功 | timeout/network/gateway/contract 等 | API 存活，就绪状态未知 | MySQL 一定故障 |
| 失败 | 成功 | 整体仍为未知；保留 readiness 单项事实用于排查 | readiness 可替代 liveness |
| 失败 | 失败 | 无法确认 API 状态 | 全平台所有实例都离线 |
| 任一 loading | 任一 loading | 正在检查；先返回的卡片先显示 | 已完成一轮判定 |

UI 同时使用文字、图标和颜色，不只依赖颜色表达状态；汇总区域使用 `aria-live="polite"`。成功卡片展示 endpoint、浏览器 RTT、服务版本、服务端时间和响应 request ID；错误卡片在可用时展示 request ID，不显示后端内部 cause。

状态页只保留两张有真实后端事实来源的卡片。第 14 节曾展示的虚构服务数量、假延迟和“全部正常”不再出现。两个探针可能在滚动发布或代理负载均衡时命中不同实例，因此版本不同本身不是必然故障，必须结合路由与 request ID 判断。

## 9. Go 错误响应为何补 `no-store`

成功探针已经设置 `Cache-Control: no-store`，但第 12 节统一错误出口此前没有统一设置。错误 body 含有请求特定的 `request_id`，如果浏览器、中间代理或 CDN 缓存后重放，会让后来一次失败携带旧关联 ID，既误导排障，也可能泄漏另一次请求的相关性。

因此 `abortWithFaultStatus` 在所有统一错误响应上设置 `Cache-Control: no-store`。这同时覆盖 readiness 503、404、405 和其他 fault 映射。它不代替代理缓存策略或认证响应设计，但建立了服务端明确的禁止存储信号。

## 10. 本地运行

Go API 仍需要第 13 节的 MySQL API 账号配置。启动 API 后，再启动前端：

```bash
make api-run
```

```bash
cd web
pnpm install --frozen-lockfile
pnpm run dev
```

默认打开：

```text
http://127.0.0.1:5173/system/status
```

若 API 不在默认 origin，在 `web/.env.local` 配置 `GROWTHOS_WEB_API_PROXY_TARGET` 后重启 Vite。浏览器代码和页面 URL 不需要改变。

## 11. 测试与验收策略

| 风险 | 应有自动化/联调保护 |
| --- | --- |
| 代理只覆盖 `/api`，系统端点 404 | development 与 preview 共享 `/health`、`/ready`、`/api` 代理表 |
| 绝对后端 URL 被编译入浏览器 | HTTP client 只允许同源绝对路径；env 使用非 `VITE_` 前缀 |
| 2xx 错误 HTML 或结构漂移被当成功 | Content-Type、JSON 和运行时 decoder 测试 |
| 503 原因、request ID 丢失 | 统一 error envelope 与 header/body 一致性测试 |
| timeout 被误报 network | fake fetch + timer 覆盖 timeout 与外部取消分类 |
| 两探针互相遮蔽 | hook 独立完成/失败状态测试 |
| 快速刷新导致旧响应覆盖 | AbortController + generation stale-response 测试 |
| React 卸载后继续更新 | effect cleanup 与取消测试 |
| UI 恢复虚构服务或错误话术 | 页面组件测试与真实浏览器检查 |
| 后端错误 request ID 被缓存 | Go error envelope 的 `Cache-Control: no-store` 测试 |
| 单元测试通过但代理没连通 | 同时运行真实 Go API、MySQL 和 Vite 的浏览器冒烟 |

实际执行：

```bash
cd web
pnpm run test
pnpm run typecheck
pnpm run build

go test ./internal/infrastructure/httpapi
make verify
```

本地工具链前置为 Node.js `>=22.22.2` 与 pnpm `10.13.1`。最终结果为：冻结 lockfile 使用 pnpm 10.13.1 重装成功；Vitest 4 个文件、34 项展开测试全部通过；TypeScript、Vite build、`go test ./...`、`go vet ./...` 和最终 `make verify` 均通过。Vite 仍报告约 708 kB 主 chunk 的非阻断 warning，本节没有用未经需求验证的拆包掩盖它。

真实浏览器联调覆盖正常、MySQL 依赖降级和 API 离线三态。离线演练发现 Vite 会把上游拒绝连接转换成 `502 text/plain`，最初被误归为 contract；实现随后增加 `gateway` 分类和回归测试。axe-core 初检也发现浅色模式 14 个低对比度节点，修复后在正常、降级、离线三态均为 0 violation。逐项证据见[第 15 节 QA](../../qa/lessons/lesson-15.md)。

## 12. 当前真实边界与下一节

本节完成后，GrowthOS 第一次拥有浏览器可见的真实 Go 数据，但这条数据只来自系统探针。其他用户端、运营端、MCP 和 Agent 页面仍使用集中 Mock，不代表相应业务已经实现。

第 16 节可以把 API、前端和 MySQL 接入 Docker Compose，并把 probe 用于容器依赖与健康检查；届时需要再次决定容器监听地址、反向代理入口和 Secret 注入。本节不提前用 `0.0.0.0`、CORS 或 Compose 假设替代那一节的真实需求。

## 13. 关键文件

| 文件 | 责任 |
| --- | --- |
| `web/vite.config.ts` | Node-side 代理目标校验、dev/preview 同源转发与监听边界 |
| `web/.env.example` | 非秘密代理变量示例 |
| `web/src/api/httpClient.ts` | JSON、timeout、取消、错误分类与 request ID 的统一边界 |
| `web/src/api/systemApi.ts` | 两个系统端点及其运行时 decoder |
| `web/src/pages/system/status/useSystemStatus.ts` | 并行独立状态、generation 与生命周期取消 |
| `web/src/pages/system/status/SystemStatusPage.tsx` | 真实状态展示、降级解释和可访问性 |
| `internal/infrastructure/httpapi/errors.go` | 所有统一错误响应的 `no-store` |
| `Makefile`、`web/package.json` | 前端 test/typecheck/build 质量门禁 |

本节同源决策见 [ADR-0011](../../decisions/ADR-0011-same-origin-frontend-integration.md)，前端消费契约见[第 15 节 API 记录](../../api/lessons/lesson-15.md)，开放推导见[第一性原理设计手记](../../design-thinking/lessons/lesson-15.md)，求职复盘见[面试问答](../../interview/lessons/lesson-15.md)。
