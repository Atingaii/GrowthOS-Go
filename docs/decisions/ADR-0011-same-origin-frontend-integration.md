# ADR-0011：前端同源代理与系统探针集成边界

- **状态：** 已接受
- **日期：** 2026-08-29
- **负责人：** GrowthOS 维护者

## 背景

第 11～13 节已经提供 Go `GET /health`、MySQL `GET /ready`、统一 error envelope 和 `X-Request-ID`。第 14 节建立 React/Vite 页面框架，但所有业务页面仍使用 Mock。第 15 节需要打通浏览器到 Go 的第一条真实链路，同时避免为一次本地联调提前固定生产拓扑、开放 CORS 或引入没有真实复杂度支撑的数据请求框架。

当前事实和约束：

1. Vite development 页面默认在 `127.0.0.1:5173`，Go API 默认在 `127.0.0.1:8080`；浏览器直接访问后者属于跨 origin。
2. 系统探针是当前唯一已经稳定实现、无需业务表的真实 JSON 契约。
3. `/health` 和 `/ready` 的语义故意不同，前端必须保留“进程存活但 MySQL 未就绪”的组合，不能合并为一个布尔值。
4. TypeScript 只检查编译期源码，不能证明网络 JSON 与 interface 一致。
5. `X-Request-ID` 是联调排障的关键证据，但错误 body 自带 request ID，缓存旧错误会制造错误关联。
6. 业务查询、认证、重试、分页、mutation 和 server-state cache 尚无真实需求。
7. 本节本地开发边界不能冒充生产网关、监控系统或集群健康证明。

## 决策驱动

方案必须同时满足：

- 浏览器代码不硬编码开发 API origin；
- 本地无需放宽 Go CORS 即可联调；
- 开发与 preview 行为一致；
- proxy target 不能携带凭据或被编译到客户端；
- 网络、HTTP、timeout、取消和契约漂移可区分；
- 响应在运行时验证，而不是只使用类型断言；
- 两个探针独立、并行并能处理竞态；
- UI 只陈述当前真实可观测事实；
- 方案足够小，未来业务复杂度出现时可以演进。

## 评估过的方案

### 1. 浏览器直连 Go，并在 Gin 开放 CORS

| 优点 | 代价 / 风险 |
| --- | --- |
| 网络拓扑直接；无需本地代理 | 必须定义 origin allowlist、credentials、preflight、exposed headers 和环境差异 |
| 可模拟前后端不同域部署 | 当前没有跨域产品需求，容易用 `*` 或反射 Origin 形成不必要暴露 |
| 浏览器直接看到 API origin | 后端地址会进入客户端配置；`X-Request-ID` 还需显式 expose |

**结论：不采用。** 如果未来生产确实分域，再基于真实身份、Cookie/Token 和 CSRF 模型新增 ADR；不能把开发端口不同当成产品跨域需求。

### 2. 使用 `VITE_API_BASE_URL` 把绝对 URL 编译进前端

| 优点 | 代价 / 风险 |
| --- | --- |
| 实现简单；每个环境替换变量 | `VITE_` 值公开进入 bundle，不适合承载任何 Secret |
| 不依赖 Vite proxy | 仍需 CORS；调用代码感知拓扑；build artifact 与环境绑定 |
| 可以直接请求远端 | 配错 URL 可能把凭据或请求发往非预期 origin |

**结论：不采用。** 浏览器 API 使用同源路径；Node-side proxy upstream 使用无 `VITE_` 前缀的变量。

### 3. 本地 Vite 同源代理，生产由正式反向代理保持路径

| 优点 | 成本 / 注意点 |
| --- | --- |
| 浏览器只请求 `/health`、`/ready`、`/api/...` | 本地代理不是生产网关，需要另行部署配置 |
| 无需为开发开放 CORS | proxy 配置错误仍会导致 502/连接失败，需要真实联调 |
| 前端 bundle 与 upstream origin 解耦 | dev 与 preview 必须共享配置，Docker 场景需重新决定监听地址 |
| 同源可直接读取 `X-Request-ID` | proxy 只转发，不提供认证或信任边界 |

**结论：采用。**

### 4. 原生 fetch、Axios 与更高层 server-state 库

| 方案 | 优点 | 当前问题 | 结论 |
| --- | --- | --- | --- |
| 每个组件直接 `fetch` | 无新增依赖 | timeout、错误、decoder、request ID 和取消会散落 | 不采用 |
| 统一原生 `fetch` adapter | 平台内置；AbortSignal 与 cache/credentials 语义直接；封装面小 | 需要自己定义 error 与 decoder | 采用 |
| Axios | interceptors、timeout 与生态成熟 | 当前两个 GET 不足以证明新增依赖与双重抽象有价值 | 暂不采用 |
| TanStack Query/SWR | cache、dedupe、retry、失效管理成熟 | 当前是手动读取实时探针，自动缓存/重试可能模糊一次观测 | 暂不采用 |
| Zustand 保存探针 server state | 可跨页面共享 | 服务端瞬时状态与客户端全局偏好生命周期不同，容易陈旧 | 不采用 |

“暂不采用”不是永久禁令。出现跨页面共享、分页、mutation 后失效、后台刷新、离线策略或显著重复请求时，应重新评估 server-state 库，而不是继续扩大手写 cache。

### 5. 两探针请求编排

| 方案 | 问题 | 结论 |
| --- | --- | --- |
| 先 health、成功后再 ready | 增加总延迟；health 失败时丢失 readiness 诊断事实 | 不采用 |
| `Promise.all` 合并一个成功/失败 | 任一 rejection 会遮蔽另一结果 | 不采用 |
| 并行请求、分别更新，`allSettled` 只记录轮次完成 | 最快取得两份独立事实，保留部分成功 | 采用 |

## 决策

1. 浏览器 API 调用统一使用同源绝对路径，不接受绝对 URL、protocol-relative URL、反斜杠或缺少 `/` 的相对路径；请求同时设置 `mode: same-origin` 与 `redirect: error`，不能经 URL 归一化或 30x 跟随绕出 origin。
2. Vite development 与 preview 均原样代理精确的 `/health`、`/ready` 与 `/api` 命名空间，不 rewrite；正则边界不能把 `/health-anything`、`/readyXYZ` 或 `/apiwhatever` 转给 Go。生产环境由正式同源反向代理或网关承接相同路径，本 ADR 不把 Vite 作为生产组件。
3. upstream 默认是 `http://127.0.0.1:8080`，仅由 Node-side `GROWTHOS_WEB_API_PROXY_TARGET` 覆盖。变量只接受无 credentials、path、query、fragment 的 HTTP(S) origin，不使用 `VITE_` 前缀，也不能存放 Secret。
4. Vite dev/preview 默认只绑定 `127.0.0.1` 并启用 `strictPort`。第 16 节需要容器监听时显式重新配置，不在本节默认扩大网络暴露面。
5. Go 不为本节新增 CORS。若未来必须跨源，需独立决策精确 origin allowlist、credentials、preflight、allowed methods/headers、`Access-Control-Expose-Headers: X-Request-ID` 与 CSRF 风险。
6. 建立一个基于原生 `fetch` 的 JSON adapter，统一 GET、Accept JSON、`cache: no-store`、`credentials: same-origin`、`mode: same-origin`、`redirect: error`、timeout、外部取消、elapsed、request ID 和安全错误分类。
7. timeout 默认 5 秒，可配置为 100 ms～30 秒的安全整数。timeout 与调用方取消分别分类；二者都通过 AbortController 停止浏览器等待，但不夸大为服务端工作必然停止。
8. 非 2xx 只有满足统一 `error.code/message/request_id` envelope 才分类为 HTTP error；Content-Type、JSON、envelope 或成功 payload 不可信时分类为 contract error。唯一额外分支是代理层返回非 JSON 502/503/504：它分类为 `gateway`，保留 status 但不伪造服务端 code。
9. 错误响应 header 与 body 同时提供 request ID 时必须一致，否则 fail closed 为 contract error。成功 header 缺少 request ID 不改变 payload 成功，但 UI 明确显示缺失。
10. `systemApi` 为 `/health` 与 `/ready` 分别定义 runtime decoder：验证固定 status、非空 version，以及具有 RFC 3339 形状且可解析的 timestamp；允许额外字段以支持兼容扩展。
11. 两个探针并行、独立结算，不共用一个 success flag。`allSettled` 只用于判断本轮完成，不决定单个状态。
12. 每次刷新取消上一轮并增加 generation；只有当前 generation 且未取消的结果能更新状态。React effect cleanup 在卸载时同样取消并使旧回调失效。
13. 本节只在首次挂载和用户手动刷新时请求，不自动轮询、不重试、不缓存 server state。监控、退避与可见性策略在真实需求出现后决策。
14. 系统状态页只展示当前真实存在的 Go liveness 与 MySQL readiness，不展示虚构服务数、假延迟或“全部正常”。elapsed 明确是浏览器往返时间，不是服务端处理耗时。
15. readiness 只有在 health 成功，且收到合法 503 `dependency_unavailable` 时才解释为“API 存活、MySQL 未就绪”；health 失败时整体始终为未知，即使 readiness 单项成功也只作为诊断事实保留。
16. 所有统一 Go error envelope 设置 `Cache-Control: no-store`，避免携带旧 request ID 的响应被浏览器、CDN 或共享代理重放。
17. 当前 HTTP adapter 不是最终业务 SDK。POST/body、认证、幂等、重试、分页、文件上传和缓存只能由后续真实业务用例驱动扩展。

## 安全与隐私边界

- 同源代理减少当前 CORS 面，但不替代认证、授权、CSRF、TLS、网关 allowlist 或生产网络隔离；
- proxy target 不允许 credentials，MySQL 密码、DSN、CA 与内部拓扑不能进入 web env、bundle 或页面；
- client 不渲染原始 response body、fetch exception 或后端 cause；
- request ID 只用于关联，不作为身份、授权或不可伪造 trace 证据；
- `no-store` 是服务端缓存指令，仍需在生产代理验收实际缓存行为；
- `/ready` 是否应暴露给公共互联网由生产部署边界另行决定，本 ADR 只定义浏览器本地联调。

## 影响

### 正面影响

- 前端第一次使用真实 Go 契约，同时保持业务 Mock 与真实能力的清晰界线；
- 本地联调不需要开放 CORS，调用代码也不包含开发端口；
- timeout、取消、HTTP、gateway、network 和 contract failure 可分别诊断；
- 运行时 decoder 阻止错误 HTML、版本漂移或字段缺失伪装成类型安全；
- 两探针独立状态保留 liveness/readiness 最有价值的故障组合；
- request ID 可以从页面关联到 Go 日志，错误缓存不会重放旧关联 ID；
- 方案不引入当前用不到的请求/缓存依赖，未来仍可替换。

### 成本与风险

- 项目需要维护一小层 fetch adapter、runtime decoder 和并发状态测试；
- Vite proxy 配置是本地专用，生产必须另有同源反向代理；
- 无自动轮询意味着页面不是监控器，状态会在最后一次手动检查后陈旧；
- 5 秒客户端 timeout 与实际网络/服务端预算需要在真实环境继续校准；
- 允许响应额外字段提供兼容性，但无法捕获所有语义变化；破坏性变化仍需要 API 版本和契约治理；
- 两个请求可能命中不同实例，版本不同不必然是故障，排查必须结合 request ID 和路由。

## 迁移与撤销

当前调用方只依赖 `requestJSON`、`systemApi` 与 hook 返回类型。如果未来采用 Axios 或 server-state 库，应先保留以下不可变契约，再替换 transport 实现：

- 浏览器仍使用同源业务路径；
- 运行时 decoder 不被 `as` 类型断言替代；
- timeout/cancelled/network/gateway/http/contract 的 error kind，以及 status/code/request ID 保持稳定或提供迁移适配；
- liveness/readiness 独立状态不被合并；
- 取消与 stale result 防护继续存在；
- 原始错误、Secret 与内部拓扑不进入 UI。

撤销 Vite proxy 时必须先有等价生产/本地入口。不能先删除代理，再用宽泛 CORS 或硬编码 URL 临时补洞。

## 重新评估触发条件

出现以下证据时，应新增 ADR 或修订本决策：

- 生产产品明确要求可信的跨 origin 浏览器客户端；
- Cookie、OAuth、CSRF 或第三方嵌入改变请求身份模型；
- 三个以上页面共享相同 server state，并出现 cache、dedupe、失效或后台刷新需求；
- mutation、乐观更新、重试/退避、离线恢复成为真实业务需求；
- 需要上传、流式响应、SSE、WebSocket 或大文件下载；
- Compose/远程开发要求 Vite 监听非 loopback；
- production 网关重写 path、缓存或剥离 `X-Request-ID`；
- 多实例探针需要聚合、持续监控、告警或 SLA，而不再是一次人工诊断；
- 前端 timeout 在真实延迟分布下产生可测误报；
- API 契约进入 OpenAPI/codegen 阶段，需要用 schema 生成或验证替换手写 decoder。

## 参考

- [Vite `server.proxy`](https://vite.dev/config/server-options.html#server-proxy)
- [Vite 环境变量与 mode](https://vite.dev/guide/env-and-mode)
- [Vite `loadEnv`](https://vite.dev/guide/api-javascript.html#loadenv)
- [MDN：Using the Fetch API](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch)
- [MDN：AbortController](https://developer.mozilla.org/en-US/docs/Web/API/AbortController)
- [MDN：Same-origin policy](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Same-origin_policy)
- [MDN：CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS)
- [MDN：Access-Control-Expose-Headers](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Access-Control-Expose-Headers)
- [React：Synchronizing with Effects](https://react.dev/learn/synchronizing-with-effects)
- [React：`useEffect`](https://react.dev/reference/react/useEffect)
- [TypeScript for the New Programmer](https://www.typescriptlang.org/docs/handbook/typescript-from-scratch)
- [RFC 9110：HTTP Semantics](https://www.rfc-editor.org/rfc/rfc9110.html)
- [RFC 3339：Date and Time on the Internet](https://www.rfc-editor.org/rfc/rfc3339.html)
- [Kubernetes：Liveness, Readiness, and Startup Probes](https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/)
