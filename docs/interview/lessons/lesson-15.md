# 第 15 节面试问答：第一次前后端真实联调

本章只描述第 15 节建立的浏览器、Vite 与 Go 系统探针链路。当前真实接口只有 `GET /health` 和 `GET /ready`；活动、抽奖、积分、优惠券、Feed、MCP 与 Agent 页面仍是 Mock。自动化和真实浏览器证据是否已经完成，必须以[第 15 节 QA](../../qa/lessons/lesson-15.md)为准，不能把设计稿、代码存在或单元测试存在说成已经通过生产验证。

## 60 秒项目自述

这一节我为 GrowthOS 打通了第一条浏览器到 Go 的真实链路。前端不硬编码 `:8080`，而是请求同源的 `/health` 与 `/ready`；本地 development 和 preview 由 Vite 原样代理，生产则预留给正式同源网关，因此没有为了开发端口不同而给 Gin 开宽泛 CORS。我把原生 `fetch` 收敛成统一 JSON adapter，明确区分 HTTP、gateway、network、timeout、cancelled 与 contract，并在 TypeScript 静态类型之外做响应运行时解码。两个探针并行但独立结算：`/health` 成功而 `/ready` 返回带 `dependency_unavailable` 的 503 时，页面只说明“API 存活、MySQL 未就绪”，不会误报整个系统宕机。刷新和卸载通过 AbortController 与 generation 双保险阻止旧响应覆盖新状态；成功和错误响应都保留 request ID，统一错误出口补上 `Cache-Control: no-store`。本节没有实现业务 API、自动监控、生产网关或集群 SLA，只建立了可验证、可演进的联调边界。

## 来源说明

- `面经启发` 来自牛客用户自行发布的求职复盘，只说明相近题型确实会被候选人讨论；它不是企业官方题库，也无法独立核验帖子中的公司、轮次与逐字题面。
- `社区讨论` 只补充排查和表达角度，不能单独证明浏览器、HTTP、React 或 Vite 的技术事实。
- `官方事实` 以 React、Vite、MDN、TypeScript、Kubernetes、IETF/W3C 或库维护者文档为准。
- `项目事实` 只能由当前仓库代码、测试、ADR、QA 与真实运行记录支持。真实联调尚未在 QA 标为通过时，回答必须使用“实现了/计划验证”，不能使用“线上稳定运行”。
- 有些题是根据本项目故障模型组织的场景追问，没有找到完全同题的可靠面经，本文不会给它虚构企业归属。

## 1. 你如何证明这是真实前后端联调，而不是把 Mock 换了一层包装？

- **短答：** 真实联调证据必须跨越浏览器、代理、Go handler 和依赖四层。浏览器 Network 中请求地址应是 Vite origin 下的 `/health`、`/ready`；响应要有 Go 生成的 `status/version/timestamp`、`X-Request-ID` 与 `Cache-Control: no-store`；同一个 request ID 能在 Go 访问日志定位；停止 API 或让 MySQL readiness 失败后，页面还必须分别出现代理不可达与“API 存活、MySQL 未就绪”。只看 UI 文案或 mock fetch 单测不能证明链路真实。
- **深挖：** 我会至少保存三组证据：正常路径的 200/200、MySQL 不可用时的 health 200 + ready 503、API 不可达时的 gateway/network 状态。对于 503，还要检查 body 的 `error.code/request_id` 与响应 header 是否一致。如果章节另做恢复演练，恢复依赖后还应手动刷新并确认取得新 request ID；第 15 节 QA 没有把这一可选步骤冒充成已执行证据。这样既证明数据来自 Go，也明确区分已实测项与推荐补充项。
- **项目证据：** [真实链路验收步骤](../../qa/lessons/lesson-15.md)、[系统状态页](../../../web/src/pages/system/status/SystemStatusPage.tsx)、[Vite 代理](../../../web/vite.config.ts)、[Go 路由](../../../internal/infrastructure/httpapi/router.go)。
- **追问：** 前端组件测试全部通过，能不能直接说“联调完成”？
  - **追问回答：** 不能。组件测试能证明给定状态下的渲染，fake fetch 能证明 client 分类，但都不会发现代理路径错误、Go 未启动、响应 header 被中间层移除或真实 MySQL 权限问题。最终状态以 QA 中记录的真实命令、浏览器 Network 和故障注入为准。
- **来源：** `面经启发` [牛客用户自述的快手前端复盘，出现“环境如何搭建、前后端如何联调、API 是否有文档”等问题](https://www.nowcoder.com/discuss/372088500135510016)；`官方事实` [Vite server.proxy](https://vite.dev/config/server-options.html#server-proxy)、[Fetch 响应处理](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch)。

## 2. Mock 数据怎样切到真实接口，为什么本节没有把所有页面一起切掉？

- **短答：** 本节采用“最小纵向切片”，只把已有稳定后端契约的系统状态页切到真实接口，其他业务页面继续从集中 Mock 数据读取。没有对应领域 API、schema 和验收证据时，强行全量切换只会制造假接口和无法归因的故障。
- **深挖：** 切换顺序应是：先冻结端点与 error envelope；把网络机制放到通用 client；把领域路径和 decoder 放到 `systemApi`；让页面只消费领域状态；单元测试注入 fake fetch，真实环境通过同源路径访问 Go。当前状态页没有运行时“Mock/Live”开关，因为一个面向用户的健康页不应在不显眼的条件下返回伪健康。如果以后同一业务页面确实需要演示模式，应在 repository/adapter 边界注入实现，并在界面显著标注数据来源；测试环境可用 fake fetch 或 MSW，但 mock 响应也必须通过同一契约测试。
- **项目证据：** [系统 API adapter](../../../web/src/api/systemApi.ts)、[HTTP client 的可注入 fetch](../../../web/src/api/httpClient.ts)、[仍然集中的业务 Mock](../../../web/src/mocks/growthOsMockData.ts)、[同源集成 ADR](../../decisions/ADR-0011-same-origin-frontend-integration.md)。
- **追问：** 为什么不在组件里写 `if (import.meta.env.DEV) mock else fetch`？
  - **追问回答：** 这会把环境判断、数据来源和 UI 耦合，开发环境永远看不到真实接口问题，也可能把 Mock 误带到构建产物。环境决定 adapter 装配，组件只依赖稳定接口；而本节连 adapter 开关都暂不需要，真实状态页始终走真实 client。
- **选型边界：** 当第一个业务领域 API 落地时，应逐页替换而不是一次删除全部 Mock；每个切片都要保留正常、空数据、业务错误、网络故障和契约漂移验收。
- **来源：** `面经启发` [牛客用户自述的快手前端复盘，包含联调与 API 文档问题](https://www.nowcoder.com/discuss/372088500135510016)、[牛客用户自述的去哪儿前端复盘](https://www.nowcoder.com/discuss/404792469555081216)；后者只作为前端项目/联调题型线索，不把帖子标题或内容当作公司官方题库。

## 3. 为什么使用原生 Fetch，而不是 Axios？

- **短答：** 当前只有两个同源 GET，请求需要的是 JSON、timeout、取消、稳定错误分类、request ID 和运行时 decoder。原生 Fetch 已提供 Request/Response 与 AbortSignal，统一封装后足够，而且不增加依赖。Axios 的实例、interceptor、timeout 和错误模型在认证刷新、多种 adapter 或大量一致请求策略出现时很有价值，但当前收益覆盖不了新增抽象成本。
- **深挖：** 选原生 Fetch 不等于让每个组件裸写 `fetch`。项目用 `requestJSON` 统一 `Accept`、`cache`、`credentials`、timeout、取消、Content-Type、非 2xx、error envelope 和 elapsed；领域模块只定义 path 与 decoder。Axios 同样需要运行时 decoder，也不能自动判断 GrowthOS 的 `dependency_unavailable`。相反，如果以后出现 token refresh、统一上传进度、Node/browser 双 adapter 和大量拦截逻辑，继续扩展手写 client 可能比引入 Axios 更贵，届时应重新立决策。
- **项目证据：** [原生 Fetch adapter](../../../web/src/api/httpClient.ts)、[client 单元测试](../../../web/src/api/httpClient.test.ts)、[技术选型 ADR](../../decisions/ADR-0011-same-origin-frontend-integration.md)。
- **追问：** Axios 也支持 AbortController，那原生 Fetch 的“可取消”还是优势吗？
  - **追问回答：** 不是独占优势。Axios 官方也支持 `signal`，所以决策点不是“Axios 不能取消”，而是当前用平台原语即可满足需求。面试中应比较总抽象成本和团队一致性，不能用过时结论贬低替代方案。
- **来源：** `面经启发` [牛客用户自述的前端面经，明确出现 Fetch、Ajax、Axios 区别](https://www.nowcoder.com/discuss/353155430186688512)；`官方事实` [MDN Fetch 指南](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch)、[Axios interceptor](https://axios-http.com/docs/interceptors)、[Axios cancellation](https://axios-http.com/docs/cancellation)。

## 4. Fetch 收到 404 或 503 时会进入 `catch` 吗？项目怎样处理非 2xx？

- **短答：** 通常不会。Fetch 在服务器返回 HTTP 错误状态时仍会 fulfill，只有请求本身失败或被取消等情况才 reject，所以必须显式检查 `response.ok` 或 status。
- **深挖：** GrowthOS 先读取 `X-Request-ID` 与 Content-Type，再解析 JSON。非 2xx 只有在 body 满足统一 `{error:{code,message,request_id}}` 且 header/body request ID 不冲突时，才分类为 `kind=http`；一般非法响应是 `contract`，同源代理返回非 JSON 的 502/503/504 则是经过真实联调确认的 `gateway` 例外。业务分支使用 `kind/status/code`，不解析易变的 message。这样 503 `dependency_unavailable` 可以安全映射成 MySQL 未就绪，而一个代理返回的 HTML 503 不会被误认为 Go 业务错误。
- **项目证据：** [非 2xx 解码](../../../web/src/api/httpClient.ts)、[统一 error envelope](../../../internal/infrastructure/httpapi/errors.go)、[503 与契约测试](../../../web/src/api/httpClient.test.ts)。
- **追问：** 为什么不直接 `throw await response.text()`？
  - **追问回答：** 原始 body 可能是代理 HTML、内部堆栈、驱动错误或不可控大文本，直接回显会泄漏实现细节且无法形成稳定程序分支。应只接受已知 envelope，把未知响应收敛为 contract error，并把详细现场留给受控日志和 request ID。
- **来源：** `官方事实` [MDN 对 Fetch 非 2xx 行为与 `Response.ok` 的说明](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch)、[HTTP 状态语义 RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html)。

## 5. 代理返回 502 与 Go 返回 503 有什么区别，为什么不能统一成“服务挂了”？

- **短答：** 502 表示作为 gateway/proxy 的一方从上游收到无效响应；503 表示服务器当前暂时无法处理请求。项目还必须结合响应契约判断来源：Vite/网关的 502/503/504 非 JSON 响应归为 `gateway`，Go `/ready` 的 503 + 合法 envelope + `dependency_unavailable` 归为 `http`，页面才能准确说明 API 是否仍存活。
- **深挖：** 状态码本身不够。代理也可能返回 JSON，应用也可能经过多层网关，所以项目额外检查 Content-Type、envelope 和 request ID。health 200 + ready 503 的组合说明当前 Go 进程能响应而 MySQL readiness 失败；两个请求都是非 JSON 502，更像代理无法连接 API。任何单一浏览器结果都只代表本次路径与实例，不能推导整个集群状态。
- **项目证据：** [`gateway` 与 `http` 分类](../../../web/src/api/httpClient.ts)、[readiness handler](../../../internal/infrastructure/httpapi/readiness.go)、[页面组合解释](../../../web/src/pages/system/status/SystemStatusPage.tsx)。
- **追问：** 只要 status 是 503，就显示“MySQL 故障”可以吗？
  - **追问回答：** 不可以。只有 `/ready`、合法 Go error envelope、`code=dependency_unavailable` 且 request ID 校验通过这一组条件才能映射成 MySQL 未就绪。代理 503、契约漂移、超时和网络失败都只能说 readiness 未知。
- **来源：** `面经启发` [牛客用户自述的字节前端复盘，包含 HTTP 状态码与项目深挖](https://www.nowcoder.com/discuss/661351242735550464)；`官方事实` [RFC 9110 的 502/503/504 语义](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.6)。

## 6. timeout 和 cancel 有什么区别？调用 `abort()` 后服务端一定停止了吗？

- **短答：** timeout 是 client 自己的等待预算耗尽，cancel 是调用方因刷新、卸载或用户动作主动终止；两者都可以触发 AbortController，但产品语义不同。浏览器停止等待不等于服务端或数据库一定停止，只有取消沿 HTTP request context 传播且每一层都遵守 context，后端工作才有机会尽快结束。
- **深挖：** client 默认 5 秒，使用内部 controller；外部 signal abort 时转发取消，timer 到期则标记 `timedOut` 再 abort，最终分别归类。`growth-api` 默认把配置中的 3 秒 MySQL Ping timeout 传给 `/ready`（只有调用方未提供正值时，handler 才回退到 2 秒），目的是尽量在浏览器预算前返回稳定 503。这是嵌套预算，不是重复代码：服务端控制依赖操作，浏览器控制用户愿意等待多久。清理 timer 与外部 listener 可以避免泄漏。
- **项目证据：** [前端 timeout/cancel](../../../web/src/api/httpClient.ts)、[readiness 的有界 context](../../../internal/infrastructure/httpapi/readiness.go)、[超时与取消测试](../../../web/src/api/httpClient.test.ts)。
- **追问：** 为什么不用 `Promise.race([fetch, timeout])`？
  - **追问回答：** race 只会让外层 Promise 先结束，未必取消底层 fetch；AbortController 能把取消信号交给网络 API。即便如此，仍不能承诺已经发生的服务端副作用被回滚，所以有副作用接口还需要幂等键、事务与服务端取消设计。
- **选型边界：** `AbortSignal.timeout()` 是平台能力，但本项目还需把内部 timeout 与调用方 signal 合并并区分错误分类，因此当前显式 controller 更容易测试和解释。
- **来源：** `面经启发` [牛客用户自述的滴滴前端复盘，出现请求超时题型](https://www.nowcoder.com/discuss/578707289272565760)；`官方事实` [AbortController](https://developer.mozilla.org/en-US/docs/Web/API/AbortController)、[`AbortSignal.timeout()`](https://developer.mozilla.org/en-US/docs/Web/API/AbortSignal/timeout_static)、[Go 的取消传播](https://go.dev/doc/database/cancel-operations)。

## 7. 已经写了 TypeScript interface，为什么还需要 runtime decoder？

- **短答：** TypeScript 类型只约束编译期源码，网络响应在运行时仍是 `unknown`。`response.json() as HealthResponse` 只是告诉编译器“相信我”，不会检查服务端是否返回 HTML、空 version、错误 status 或非法 timestamp。
- **深挖：** `systemApi` 对两个端点分别检查：值必须是 object；health 的 status 必须是 `ok`，readiness 必须是 `ready`；version 非空；timestamp 既符合预期 RFC3339 形状又能解析。额外字段允许通过，便于兼容性扩展；必需字段错误则 fail closed 为 `contract`。通用 client 不知道领域字段，领域 decoder 不处理 timeout 与 request ID，这种分层让责任清晰。
- **项目证据：** [探针 decoder](../../../web/src/api/systemApi.ts)、[领域契约测试](../../../web/src/api/systemApi.test.ts)、[client decoder 边界](../../../web/src/api/httpClient.ts)。
- **追问：** 为什么不用 Zod？
  - **追问回答：** 两个很小的固定 payload 用手写 type guard 足够，新增 schema 库会提高依赖和学习成本。出现大量嵌套、联合类型、复用 schema、表单共享校验或需要生成详细错误路径时，Zod/Valibot 等库可能更划算；届时应统一采用，避免手写 decoder 无限制增长。
- **来源：** `面经启发` [牛客用户自述的快手前端复盘，包含 API 文档与联调问题](https://www.nowcoder.com/discuss/372088500135510016)；`官方事实` [TypeScript 的静态类型边界](https://www.typescriptlang.org/docs/handbook/typescript-from-scratch)、[Fetch JSON 响应读取](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch)。

## 8. React Strict Mode 为什么会让请求看起来发了两次？项目怎样避免竞态？

- **短答：** React 在开发 Strict Mode 下会额外执行一次 effect 的 setup → cleanup → setup，用来暴露清理不完整的问题。不能靠“只执行一次”的 ref 绕过；正确做法是 cleanup 取消旧请求，并保证旧 Promise 即使不遵守 signal 也不能覆盖新状态。
- **深挖：** `useSystemStatus` 每次 refresh 增加 generation、abort 上一 controller、创建新 controller，并行发请求。回调更新前同时检查 generation 相等且 signal 未 abort；cleanup 再增加 generation 并 abort。因此网络层尽量取消，状态层仍用代次门禁兜底。只做 abort 不够，因为测试 fake、缓存层或某些异步任务可能忽略 signal；只做 boolean `ignore` 也不会节省网络资源。
- **项目证据：** [effect 与 generation](../../../web/src/pages/system/status/useSystemStatus.ts)、[Strict Mode 与 stale completion 测试](../../../web/src/pages/system/status/useSystemStatus.test.tsx)。
- **追问：** 能否在 effect 外定义一个全局 `isMounted`？
  - **追问回答：** 不应。多个组件实例会共享全局变量，刷新代次也无法区分；实例级 ref + controller 才能表达当前订阅生命周期。更根本地，要让 cleanup 对称撤销 setup 创建的外部连接。
- **来源：** `面经启发` [牛客用户自述的字节前端面经，包含 useEffect 重复渲染/执行相关题型](https://www.nowcoder.com/discuss/793213320655306752)、[牛客用户自述的前端复盘，包含组件挂载请求与复用问题](https://www.nowcoder.com/discuss/410579665792905216)；`官方事实` [React useEffect](https://react.dev/reference/react/useEffect)、[React effect cleanup 与开发双执行](https://react.dev/learn/synchronizing-with-effects)。

## 9. 为什么用 `Promise.allSettled`，不用 `Promise.all`？

- **短答：** health 与 readiness 是两份独立事实，任何一个失败都不应丢掉另一个结果。`Promise.all` 在首个 rejection 时让组合 Promise reject；`allSettled` 会等待所有输入结算，适合记录“一轮检查完成”。
- **深挖：** 项目并不是等 `allSettled` 后一次性渲染。两个 Promise 各自注册成功/失败回调，谁先完成谁先更新卡片；`allSettled` 只负责在两者都 settled 后写 `completedAt`。这样既保留部分成功，又不人为增加首屏等待。health 200、ready 503 是最重要的降级组合，不能被一个总 error 覆盖。
- **项目证据：** [独立回调与 allSettled](../../../web/src/pages/system/status/useSystemStatus.ts)、[先完成先展示测试](../../../web/src/pages/system/status/useSystemStatus.test.tsx)。
- **追问：** `allSettled` 会自动取消失败后的其他请求吗？
  - **追问回答：** 不会，它只是聚合结果；本项目也不希望一个探针失败就取消另一个。统一取消发生在用户刷新或组件卸载，由共享 controller 负责。
- **来源：** `面经启发` [牛客用户自述的快手前端复盘，出现手写 Promise.allSettled](https://www.nowcoder.com/discuss/372088500135510016)；`官方事实` [MDN Promise.allSettled](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Promise/allSettled)。

## 10. `/health` 与 `/ready` 的区别是什么，前端为什么不能合并成一个布尔值？

- **短答：** health/liveness 回答进程是否仍能工作，失败可能触发重启；readiness 回答实例是否应该接收流量，失败通常只摘流。数据库短暂不可用时，Go 进程仍可健康但暂不就绪，所以必须保留 200/503 的组合。
- **深挖：** `/health` 不访问 MySQL，返回 `ok`；`/ready` 用有界 `PingContext` 检查 MySQL，失败返回统一 503。若把两个结果合成 `isHealthy=false`，运维或 UI 可能错误建议重启进程，造成所有副本同时重启并放大依赖故障。相反，readiness 成功也只说明本次 Ping 成功，不证明 schema、权限、慢 SQL、复制、Migration、业务数据或整个集群正常。
- **项目证据：** [health handler](../../../internal/infrastructure/httpapi/health.go)、[readiness handler](../../../internal/infrastructure/httpapi/readiness.go)、[页面状态矩阵](../../../web/src/pages/system/status/SystemStatusPage.tsx)、[第 13 节面试题第 7 题](lesson-13.md)。
- **追问：** 数据库挂了很久，liveness 是否应该失败让进程重启？
  - **追问回答：** 通常不应仅因外部数据库不可用就重启，因为重启修不好数据库，还会制造重连风暴。应 readiness 摘流、告警与退避；只有进程自身不能推进时才由 liveness 触发重启。最终还取决于部署平台是否真的把两个端点配置到对应 probe。
- **来源：** `官方事实` [Kubernetes liveness、readiness 与 startup probe](https://kubernetes.io/docs/concepts/workloads/pods/probes/)。

## 11. 为什么用同源代理而不是直接给 Gin 开 CORS？

- **短答：** `127.0.0.1:5173` 与 `127.0.0.1:8080` 端口不同，属于不同 origin；但“开发端口不同”不等于产品需要跨域。Vite 本地代理和生产同源网关能让浏览器始终请求 `/health`、`/ready`、`/api/...`，无需扩大 Go 的跨源访问面，也避免把后端 origin 编译进客户端。
- **深挖：** CORS 是浏览器控制脚本读取跨源响应的机制，不是认证，也不阻止浏览器在所有场景发出请求。若未来前后端确实分域，需要明确 allowlist、credentials、preflight、methods、allowed headers、CSRF 模型，并通过 `Access-Control-Expose-Headers` 暴露非 safelisted 的 `X-Request-ID`。当前没有这些真实需求，因此不使用 `Access-Control-Allow-Origin: *` 或动态反射任意 Origin。
- **项目证据：** [Vite 同源代理](../../../web/vite.config.ts)、[同源 ADR](../../decisions/ADR-0011-same-origin-frontend-integration.md)、[浏览器 client 只接受同源路径](../../../web/src/api/httpClient.ts)。
- **追问：** 既然用了 proxy，是不是就没有 CSRF 或认证问题？
  - **追问回答：** 不是。proxy 只改变浏览器看到的 origin 与转发拓扑，不替代身份认证、授权、Cookie SameSite、CSRF token、TLS 或网关策略。未来出现有副作用业务接口时仍需独立威胁建模。
- **来源：** `面经启发` [牛客用户自述的字节前端复盘，出现跨域、后端是否收到请求与兜底方案](https://www.nowcoder.com/discuss/661351242735550464)、[牛客用户自述的腾讯/网易前端复盘](https://www.nowcoder.com/discuss/353154601794871296)；`官方事实` [同源策略](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Same-origin_policy)、[CORS 指南](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS)。

## 12. 为什么代理 `/health`、`/ready` 与 `/api`，而且不 rewrite 成 `/api/v1/health`？

- **短答：** Go 已经公开了根路径探针，第一次联调应复用现有稳定契约，而不是为了前端整齐制造别名。Vite 对三个边界原样转发，浏览器路径和 Go 路由一致；未来业务 API 再进入 `/api` 前缀。
- **深挖：** rewrite 会引入两套心智模型：浏览器看到 A、Go 看到 B，日志和 curl 复现更困难。精确匹配 `^/health$`、`^/ready$` 可以避免误把 `/health-anything` 转给后端；`^/api(?:/|$)` 为未来业务路径保留边界。development 与 preview 共用同一个 proxy 构造函数，避免 dev 可用而 preview 404。`strictPort=true` 让端口占用显式失败，避免浏览器仍打开旧端口。
- **项目证据：** [代理正则、preview 对称配置与 strictPort](../../../web/vite.config.ts)、[API 契约](../../api/lessons/lesson-15.md)、[Go 路由](../../../internal/infrastructure/httpapi/router.go)。
- **追问：** 为什么生产不能直接运行 Vite dev proxy？
  - **追问回答：** Vite dev/preview 是开发与预览工具，不是本节定义的生产网关、安全边界或高可用组件。生产需要正式反向代理配置、TLS、认证、限流、日志、超时与 probe 暴露策略；本节只保持路径契约可迁移。
- **来源：** `官方事实` [Vite server.proxy 与正则规则](https://vite.dev/config/server-options.html#server-proxy)、[Vite host/strictPort 说明](https://vite.dev/config/server-options.html#server-strictport)。

## 13. `X-Request-ID` 有什么用？为什么错误响应也要 `no-store`？

- **短答：** request ID 把浏览器的一次请求与服务端日志关联起来，尤其适合联调时区分 `/health`、`/ready` 和不同刷新轮次。它不是认证令牌、业务幂等键或完整分布式 trace。错误 body 携带请求特定 ID，如果被缓存重放，会把新故障指向旧日志，所以统一成功探针和 error envelope 都发送 `Cache-Control: no-store`。
- **深挖：** 成功响应从 header 读取 ID；错误 envelope 必须有 body request ID，header 同时存在时必须一致，否则 contract fail closed。若未来跨源直连，浏览器默认不能让脚本读取任意响应 header，需要显式 expose `X-Request-ID`。跨服务追踪则应评估 W3C `traceparent` 与标准 trace context；可以同时保留面向用户支持的 request ID，但不能假装两者语义相同。
- **项目证据：** [request ID middleware](../../../internal/infrastructure/httpapi/request_id.go)、[header/body 一致性检查](../../../web/src/api/httpClient.ts)、[错误 no-store](../../../internal/infrastructure/httpapi/errors.go)。
- **追问：** request ID 可以由浏览器随便传吗？
  - **追问回答：** 可以接收客户端候选值时也必须校验长度、字符集与信任边界，避免日志注入和超长 header；当前具体生成/接受策略以 middleware 实现为准。任何外部 request ID 都只能用于关联，不能据此授权或访问他人日志。
- **来源：** `官方事实` [RFC 9111 的 `no-store`](https://www.rfc-editor.org/rfc/rfc9111.html#name-no-store)、[CORS 暴露响应 header](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Access-Control-Expose-Headers)、[W3C Trace Context](https://www.w3.org/TR/trace-context/)。

## 14. 页面显示“无法连接 API”时，你会怎样系统排障？

- **短答：** 按“URL/协议 → 浏览器请求 → Vite proxy → Go 进程 → 路由与契约 → MySQL → 日志关联”的顺序缩小范围，不从重启所有容器开始。
- **深挖：** 我会依次检查：页面是否真在预期 `127.0.0.1:5173`；Network 请求路径和 status；响应是 JSON 还是 Vite/网关 HTML；`GROWTHOS_WEB_API_PROXY_TARGET` 是否为正确无路径 origin；Vite 是否因 strictPort 退出；直接 curl Go 的 `/health`；再看 `/ready` 与 request ID 对应日志；只有 health 正常、ready 503 时才进入 MySQL 连接、账号、TLS、timeout 排查。502/504 优先查代理到上游，contract 优先查 Content-Type/字段/路由，network 则查是否根本没有可信响应。
- **项目证据：** [错误分类](../../../web/src/api/httpClient.ts)、[运行与故障验收](../../qa/lessons/lesson-15.md)、[第 13 节 MySQL Runbook](../../runbooks/mysql-migrations.md)。
- **追问：** 为什么不先看页面报错 message？
  - **追问回答：** 用户文案经过安全收敛，故意不含底层错误；它适合定位阶段，不足以判断根因。应结合 status、kind、code、request ID、响应 header 和受控日志，而不是让 UI 泄露 DSN、驱动错误或代理内部信息。
- **来源：** `面经启发` [牛客用户自述的测开面经，用 URL、协议、状态、返回体、日志与数据库组织排查](https://www.nowcoder.com/discuss/646086829950652416)、[牛客用户自述的 Go 后端面经，包含 Gin、API 文档、Nginx 与 timeout 方向](https://www.nowcoder.com/discuss/703571718387847168)；`官方事实` [HTTP 语义 RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html)、[Gin 安全最佳实践](https://gin-gonic.com/en/docs/middleware/security-guide/)。

## 15. 什么时候应该引入 TanStack Query/React Query，而不是继续手写 Hook？

- **短答：** 当 server state 出现跨页面复用、缓存与去重、分页、后台刷新、mutation 后失效、离线/重连策略或大量重复 loading/error 状态时，引入专门库更合理。当前只有一次初始化加手动刷新、且健康状态要求“本次真实观测”，手写 hook 更透明。
- **深挖：** Query 库不是只为少写几行 `useEffect`。引入后必须设计 query key、staleTime、gcTime、retry、refetchOnFocus、取消和错误边界。对状态探针，默认 retry 或缓存旧成功可能掩盖一次真实故障，因此即使采用也要显式配置。相反，业务列表、详情复用和 mutation invalidation 到来后，继续自建 cache、dedupe 与状态机很容易重复造轮子。
- **项目证据：** [当前一次性状态 Hook](../../../web/src/pages/system/status/useSystemStatus.ts)、[不提前引入的 ADR](../../decisions/ADR-0011-same-origin-frontend-integration.md)。
- **追问：** 为什么不把探针状态放 Zustand？
  - **追问回答：** Zustand 更适合客户端共享状态；探针是短时 server state，有新鲜度、请求、取消和失败语义。放进全局 store 容易让页面展示陈旧成功，也仍需自己实现请求生命周期。真正需要跨页面 server-state 时优先评估 Query 类库。
- **选型边界：** 第一个跨页面业务查询不一定立即引入；应统计重复程度和一致性需求。反过来，一旦开始手写缓存失效、请求去重与重试队列，就应停止扩张并重新 ADR。
- **来源：** `官方事实` [TanStack Query 概览](https://tanstack.com/query/latest/docs/framework/react/overview)、[重要默认值](https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults)、[查询取消](https://tanstack.com/query/latest/docs/framework/react/guides/query-cancellation)。

## 16. 后端改字段、改路径或滚动发布时，前端怎样应对契约漂移？

- **短答：** 把 path、运行时 decoder、错误 envelope 与兼容策略集中在 API adapter；兼容性新增字段允许通过，必需字段改变则显式 contract error。路径变更先做兼容窗口或版本化发布，不能让前端和后端在同一瞬间硬切。
- **深挖：** 当前 decoder 只提取 `status/version/timestamp`，因此后端新增可选字段不会破坏旧前端；删除或改变这些必需字段会失败关闭。错误响应还校验 header/body request ID，防止中间层拼接出自相矛盾的关联信息。滚动发布中 `/health` 和 `/ready` 是两次请求，可能命中不同实例，版本不同不自动判故障；要用 request ID、路由与部署版本判断。真正破坏性路径变更应采用旧新并存、consumer contract 测试和按顺序发布。
- **项目证据：** [领域 decoder](../../../web/src/api/systemApi.ts)、[契约错误分类](../../../web/src/api/httpClient.ts)、[状态页不把版本差异误报故障](../../../web/src/pages/system/status/SystemStatusPage.tsx)。
- **追问：** 能否对所有字段都 `.optional()`，保证页面永不报错？
  - **追问回答：** 不行。可选化必需业务语义只是把契约错误推迟为错误 UI。应区分兼容性扩展字段与构成正确性的必需字段；后者缺失必须尽早失败并提供可关联证据。
- **来源：** `面经启发` [牛客用户自述的前端面试复盘，包含联调与 API 文档题型](https://www.nowcoder.com/discuss/372088500135510016)、[牛客用户自述的接口字段/路径变化相关复盘](https://www.nowcoder.com/discuss/419847458929340416)；链接只作为候选人自述线索，不宣称题面或企业归属已经独立核验。`官方事实` [TypeScript 静态类型边界](https://www.typescriptlang.org/docs/handbook/typescript-from-scratch)。

## 17. 为什么代理变量不用 `VITE_` 前缀，前端环境变量能不能放 Secret？

- **短答：** Vite 会把带配置前缀、被客户端代码访问的变量暴露进 bundle，所以前端环境变量不能保存 Secret。`GROWTHOS_WEB_API_PROXY_TARGET` 只由 Node 侧 `vite.config.ts` 通过 `loadEnv` 读取，不使用 `VITE_`；它还只允许无 credentials、path、query、fragment 的 HTTP(S) origin。
- **深挖：** “没有 `VITE_`”只是防止默认客户端暴露，不等于这个值天然可以存密码。代理 URL 会出现在进程配置、错误和运维环境中，设计上就不应携带 credentials。浏览器调用只写同源路径，构建产物不绑定某个 API 主机。`.env.example` 只提供无秘密示例，真实本地配置不提交。默认 host 绑定 `127.0.0.1`，避免无需求地把开发服务器暴露到局域网。
- **项目证据：** [loadEnv 与严格 origin 校验](../../../web/vite.config.ts)、[环境变量示例](../../../web/.env.example)、[Vite ignore 规则](../../../web/.gitignore)。
- **追问：** 把 API key 写成 `VITE_API_KEY`，再在生产构建时注入，是否安全？
  - **追问回答：** 不安全。只要浏览器必须拿到它，用户就能从 bundle、DevTools 或网络请求中看到。真正的 Secret 应保留在受信任服务端，通过用户身份和授权执行操作；前端只能持有可公开配置或有明确短期/范围约束的客户端凭据。
- **来源：** `面经启发` [牛客用户自述的快手前端复盘，包含项目环境搭建](https://www.nowcoder.com/discuss/372088500135510016)；`官方事实` [Vite env 暴露规则与安全提示](https://vite.dev/guide/env-and-mode)、[Vite `loadEnv`](https://vite.dev/guide/api-javascript.html#loadenv)、[Vite host 安全边界](https://vite.dev/config/server-options.html#server-host)。

## 18. 这一节的测试金字塔怎样设计，哪些结论仍然不能说？

- **短答：** 底层用 Vitest 测 client 的 status/envelope/timeout/cancel/contract，用领域测试测 decoder 与路径，用 React Testing Library 测独立状态、Strict Mode、刷新竞态和可访问文案；Go 测 health/readiness/error no-store；最上层必须用真实 MySQL + Go + Vite + 浏览器完成正常与故障冒烟。每层证明不同风险，不能互相替代。
- **深挖：** fake timer 避免靠真实 100 ms 形成脆弱测试；deferred Promise 可精确制造旧请求晚于新请求；页面测试断言用户可见文案而不是内部 state；Go 测试校验统一 header/body。真实联调再覆盖 Vite 代理、端口、Go 进程、MySQL 与 Network header。构建成功只证明可打包，单测通过只证明受控输入；本地单实例冒烟仍不能证明生产网关、并发容量、多实例滚动发布、告警或 SLA。
- **项目证据：** [client 测试](../../../web/src/api/httpClient.test.ts)、[system API 测试](../../../web/src/api/systemApi.test.ts)、[Hook 测试](../../../web/src/pages/system/status/useSystemStatus.test.tsx)、[页面测试](../../../web/src/pages/system/status/SystemStatusPage.test.tsx)、[Go HTTP 测试](../../../internal/infrastructure/httpapi/errors_test.go)、[QA 证据表](../../qa/lessons/lesson-15.md)。
- **追问：** 为什么还要测试页面没有“全部正常”和六个虚构服务？
  - **追问回答：** 这不是文案偏好，而是真实性回归。状态页最危险的 bug 是把无后端证据的模块展示成健康；负向断言能防止后续 UI 重构重新引入假服务、假延迟或未经验证的整体健康结论。
- **来源：** `面经启发` [牛客用户自述的测开面经，强调分层定位、接口与数据库验证](https://www.nowcoder.com/discuss/646086829950652416)；`官方事实` [Vitest 指南](https://vitest.dev/guide/)、[React Testing Library 原则](https://testing-library.com/docs/react-testing-library/intro/)、[React effect 测试所依据的生命周期语义](https://react.dev/reference/react/useEffect)。

## 不能夸大的事实

- 本节真实接入的只有 `/health` 与 `/ready`；业务页面仍使用 Mock，不能说“GrowthOS 前后端业务已全部打通”。
- Vite proxy 只承担本地 development/preview 转发，不是生产网关、WAF、认证层或高可用证明。
- 同源代理减少了当前 CORS 复杂度，不等于系统没有 CSRF、认证、授权或 TLS 风险。
- Fetch 是当前最小合适方案，不代表它普遍优于 Axios；Axios 也支持 interceptor、timeout 与 AbortController。
- TypeScript interface 不能验证网络 JSON；当前手写 decoder 只覆盖两个小型探针契约。
- timeout/cancel 只证明浏览器停止等待，不能保证服务端与 MySQL 已停止全部工作。
- `Promise.allSettled` 只聚合结算结果，不会自动取消、不负责重试，也不是并行请求本身的唯一实现方式。
- `/health` 200 只证明本次请求命中的 Go 实例可响应；`/ready` 200 只证明本次 MySQL Ping 成功。
- readiness 503 只有在 path、envelope、code 与 request ID 契约均可信时，才能解释成 MySQL 未就绪。
- `X-Request-ID` 是关联标识，不是 trace、用户身份、授权令牌或幂等键。
- 浏览器 elapsed 是从前端观察的往返时间，混合代理、网络与调度开销，不是 Go handler 的纯处理耗时。
- Strict Mode 开发双执行是发现清理问题的机制，不应通过关闭 Strict Mode 或一次性 ref 掩盖。
- 手写 Hook 当前足够，不代表业务复杂后仍应拒绝 TanStack Query；重新评估条件已经明确。
- 单元测试、typecheck 与 build 不能代替真实浏览器代理联调；真实本地联调也不能证明生产 SLA。
- 牛客链接是用户自述，只用于题型启发，不是企业官方原题认证。

## 复习清单

- [ ] 能在 60 秒内讲清同源代理、统一 Fetch、运行时 decoder、独立探针、竞态防护和本节边界。
- [ ] 能用浏览器 Network、request ID、Go 日志和故障注入证明“真实联调”，而不是只展示 UI。
- [ ] 能解释为什么只切系统状态页、其他业务 Mock 暂时保留，以及未来逐页切换的方法。
- [ ] 能准确说明 Fetch 非 2xx 通常不会 reject，并区分 http、gateway、network、timeout、cancelled、contract。
- [ ] 能说明 502、503、504 的角色差异，以及为什么 503 不能一律解释为 MySQL 故障。
- [ ] 能画出浏览器 5 秒 timeout、应用默认 3 秒 MySQL Ping timeout与服务端响应余量的嵌套预算，并说明 handler 的 2 秒仅是未传配置时的回退值。
- [ ] 能解释 TypeScript 静态类型为何不能替代网络数据运行时验证。
- [ ] 能复现 Strict Mode setup → cleanup → setup，并说明 AbortController + generation 双保险。
- [ ] 能说明为什么分别更新两个 Promise，`allSettled` 只记录轮次完成。
- [ ] 能画出 liveness/readiness 的 200/200、200/503、失败/成功与双失败状态矩阵。
- [ ] 能解释同源代理与 CORS、认证、CSRF 的边界，避免把 proxy 当安全方案。
- [ ] 能说明根路径探针不 rewrite、dev/preview 对称和 strictPort 的可诊断价值。
- [ ] 能沿 URL → Network → proxy → Go → contract → MySQL → request ID 日志进行排障。
- [ ] 能列出引入 Axios、TanStack Query 或 schema 库的具体触发条件，而不是按流行度选型。
- [ ] 能说明 `VITE_` 环境变量为何不可存 Secret，以及 Node-side proxy 变量仍不能带凭据。
- [ ] 能区分单元测试、组件测试、Go 测试、真实浏览器联调和生产验证各自能证明什么。
- [ ] 面试前检查“不能夸大的事实”，只陈述 QA 中已有的真实证据。
