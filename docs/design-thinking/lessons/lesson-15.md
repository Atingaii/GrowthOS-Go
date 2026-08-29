# 第 15 节第一性原理设计手记：第一条可诚实解释的全栈链路

> 本文记录第 15 节“前后端第一次联调”背后的推导过程。它不是 Vite 代理教程、接口说明、测试结果或面试答案。当前时间切片中，Go 已有 `/health`、`/ready`、统一错误 envelope、Request ID 和 MySQL 连接边界，React 已有完整页面骨架，但业务页面仍使用 Mock；本节只让浏览器真实消费两个系统探针。任何命令是否通过、真实浏览器是否联通，必须以[第 15 节 QA](../../qa/lessons/lesson-15.md)为准，不能从本文的设计意图反推为已验证事实。

## 1. 决策命题：第一条链路首先要证明“边界可信”，而不是“页面很多”

表面命题是“做第一次前后端联调”。最短实现可以是在任意 React 组件里写：

```ts
fetch("http://localhost:8080/health")
  .then((response) => response.json())
  .then(setData);
```

这段代码可能在开发者电脑上显示一段 JSON，却没有回答真正决定后续质量的问题：

- 浏览器究竟信任哪个 origin，开发拓扑会不会泄漏进 bundle？
- 收到的真是 Go JSON，还是代理生成的 HTML/502 页面？
- TypeScript interface 如何约束运行时网络数据？
- “进程活着”和“MySQL 可以接受流量”为什么不是一个布尔值？
- 超时、调用方取消、网络失败、服务明确返回 503、契约漂移分别意味着什么？
- 两个异步结果乱序返回时，旧结果会不会覆盖新一轮？
- 页面上显示的服务、延迟、版本和“正常”是否都有事实来源？
- 本地开发代理与生产网关之间，哪些性质必须保持，哪些实现可以更换？

因此，本节真正的决策命题是：

> 在尚无真实业务 API、认证、写操作和业务表的时间切片中，怎样选择一个最小但完整的纵向切片，让浏览器、开发代理、Go HTTP 边界和 MySQL readiness 首次形成可失败、可诊断、可证伪的链路，同时不把 Mock、一次 Ping 或本地拓扑冒充成产品已经完成？

“最小”不是代码行数最少，而是同时具备输入、传输、运行时契约、状态机、故障表达、可观察证据和 UI 解释的最小闭环。“完整”也不是覆盖全部产品，而是这条窄链路内部没有靠假数据填补缺口。

### 1.1 当前已经拥有的能力

- Go Router 已注册不带业务版本的 `GET /health` 和 `GET /ready`；
- `/health` 返回当前进程响应、构建版本和服务端 UTC 时间，不访问 MySQL；
- `/ready` 使用请求派生的有界 context 执行 MySQL `PingContext`；
- readiness 依赖失败通过统一 503 `dependency_unavailable` envelope 输出，不泄漏驱动 cause；
- HTTP 中间件给每个请求分配或校验 `X-Request-ID`，访问日志包含关联 ID、路由、状态和耗时；
- 成功探针已有 `Cache-Control: no-store`；
- React/Vite 已有路由、布局、样式、主题和大量 Mock 页面；
- Vite 原来只有 `/api` 代理，系统探针根路径尚未被浏览器真实消费。

### 1.2 当前明确没有的能力

- 没有 Strategy、Award、抽奖、积分、优惠券等真实业务 HTTP API；
- 没有可供浏览器读取的真实业务表或业务查询；
- 没有登录态、Cookie/OAuth、CSRF、租户或权限模型；
- 没有生产反向代理、TLS 终止、CDN 或跨域产品需求的部署事实；
- 没有前端 mutation、分页、缓存失效、离线恢复或后台刷新需求；
- 没有 OpenAPI 代码生成、分布式 Trace、前端 RUM、SLA 监控或告警；
- 没有多实例状态聚合；状态页观察的是两次具体 HTTP 请求，不是全局控制面；
- 没有证据支持自动轮询频率、重试次数或退避策略。

这些“没有”直接约束本节不能把系统状态页包装成运维平台，也不能为了看起来技术栈丰富而提前引入一套业务 SDK、缓存框架或可观测性平台。

## 2. 为什么第一条纵向切片选择系统探针

第一条真实链路不是随便找一个容易返回 200 的接口。候选用例至少要同时满足五个条件：

1. **事实已经存在。** 后端输出必须由真实运行时产生，不能为了联调新增猜测性的业务表或伪造领域数据。
2. **契约足够稳定。** 第一次联调应聚焦边界，不应同时争论尚未完成的业务不变量。
3. **存在有价值的失败组合。** 只有永远成功的接口无法迫使设计处理部分失败、取消和错误语义。
4. **副作用接近零。** 在认证、幂等和事务尚未出现前，不应以写操作作为第一条链路。
5. **能够暴露全链路问题。** 请求必须实际经过浏览器、Vite、Go 中间件、handler；readiness 还要触达 MySQL。

### 2.1 候选切片比较

| 候选 | 真实事实是否已存在 | 失败模型 | 副作用 | 会提前绑定什么 | 当前结论 |
| --- | --- | --- | --- | --- | --- |
| 用户登录 | 没有身份与凭据模型 | 安全价值高但边界未定义 | 创建/读取身份状态 | Cookie、Token、OAuth、CSRF、权限 | 不适合作为第一条 |
| 活动列表 | 只有 Mock，真实 schema/Repository 尚未实现 | DB、分页、权限、缓存 | 只读但需要领域事实 | 表结构、查询、DTO、排序 | 延迟到业务章节 |
| 创建活动 | 没有业务不变量和事务 | 验证、冲突、幂等、权限、事务 | 有写入副作用 | 全套写模型 | 明确不选 |
| `/health` 单探针 | 已存在 | 能验证浏览器到 Go，故障维度较窄 | 无 | 几乎不绑定业务 | 必要但单独不够 |
| `/health` + `/ready` | 已存在 | 可表达进程存活、依赖未就绪、网络/代理/契约失败 | 仅 MySQL Ping | 只绑定既有运行边界 | 采用 |
| 新建 `/api/v1/status` 聚合端点 | 可以实现但当前没有必要 | 服务端可统一结果 | 无业务写入 | 新契约、聚合语义和依赖策略 | 不采用 |

两个已有探针形成的闭环足够窄，却拥有一个非常重要的部分成功状态：`health=200`、`ready=503`。这会逼迫前端承认“服务不是只有正常/宕机”，也验证第 13 节 liveness/readiness 分离不是只停留在后端文档中。

### 2.2 为什么不是只接 `/health`

只接 `/health` 能证明：浏览器通过某条路径得到当前 Go 进程生成的 JSON。它不能证明：

- 前端可以解释统一错误 envelope；
- MySQL 故障时页面不会把 API 进程误报为离线；
- 两个请求的独立状态和乱序结算正确；
- Request ID 在成功与错误路径都能工作；
- 依赖失败后恢复并手动刷新时，UI 可以重新取得事实。

加入 `/ready` 并不是“多接一个接口”，而是增加一个独立故障域，使设计必须处理部分成功。

### 2.3 为什么不新建聚合 `/status`

聚合端点在生产控制面中可能合理，但当前会损失两类教学和架构证据：

1. 服务端若把 health/readiness 合成一个结果，前端无法证明自己保留了两个独立事实；
2. 聚合 handler 需要决定状态码、部分依赖、并发探测、超时、缓存和未来多依赖扩展，而当前需求只需要消费既有契约。

当探针数量很多、浏览器 RTT 成本显著、必须获得同一实例的原子快照，或需要后端实施访问控制时，聚合端点应重新评估。当前两个 GET 的额外成本小于引入一个新公共契约的长期成本。

## 3. 证据边界：页面只能说它刚刚观察到了什么

状态页最危险的错误不是请求失败，而是把弱证据写成强结论。设计先建立以下证据等级：

| 观察 | 最多可以推出 | 不能推出 |
| --- | --- | --- |
| 浏览器收到 `/health` 合法 200 JSON | 某个 Go 实例能够完成这次 HTTP 请求 | 全部实例存活、业务 handler 正确、MySQL 可用 |
| 浏览器收到 `/ready` 合法 200 JSON | 某个实例在服务端预算内完成一次 MySQL Ping | schema 兼容、SQL P99、事务正确、数据正确、整个集群正常 |
| `/health=200` 且 `/ready=503 dependency_unavailable` | API 可响应，但该 readiness 检查未通过 | Go 已崩溃、MySQL 数据损坏、所有实例都不可用 |
| 两探针都成功 | 本轮两次请求都取得其各自最小成功证据 | 业务上线、SLA 达标、Redis/MQ/PostgreSQL 正常 |
| 显示 `elapsedMs=12` | 浏览器侧从发出 fetch 到读取/解码响应的近似往返 | Go handler 只耗时 12ms、MySQL Ping 为 12ms |
| 显示 Request ID | 可尝试在当前服务日志中关联该 HTTP 请求 | 调用方身份可信、端到端 Trace 完整、请求没有经过恶意代理 |
| 两卡片版本不同 | 两次响应携带不同构建标签 | 必然发生故障；滚动发布/负载均衡也可能产生该现象 |

由此推导 UI 文案：使用“当前 API 实例能够响应”“MySQL 连接已就绪”“无法确认”，不使用“全平台正常”“所有服务在线”“SLA 正常”。旧状态页中不存在事实源的六个服务、假延迟和“全部正常”必须删除，而不是用更逼真的随机数继续伪装。

这条原则同样限制文档：组件测试存在不等于真实代理已联通；`make verify` 绿色不等于 MySQL 故障场景已在浏览器复现。设计、代码、自动化和人工联调是四类不同证据。

## 4. 不可争辩的事实与约束

| 类别 | 当前事实 | 直接设计含义 |
| --- | --- | --- |
| 浏览器安全模型 | `127.0.0.1:5173` 与 `127.0.0.1:8080` 端口不同，属于不同 origin | 直接请求 Go 会进入 CORS 决策；不能把“都在本机”当同源 |
| 路由事实 | 系统探针真实路径是根路径 `/health`、`/ready` | 代理必须覆盖精确根路径，不能凭空加 `/api/v1` |
| 运行时事实 | 网络 JSON 在进入 decoder 前是 `unknown` | TypeScript interface 不能替代运行时校验 |
| 故障事实 | HTTP 503 可能来自 Go 的合法 JSON，也可能来自代理的非 JSON 网关页 | 不能只看 status；需要同时判断 Content-Type 和 envelope |
| 时间事实 | Promise 可以乱序完成，旧请求也可能忽略 abort 后继续 resolve | 取消不足以阻止 stale update，还需要代际检查 |
| React 事实 | 开发 Strict Mode 会做额外 setup/cleanup 以暴露副作用问题 | effect 必须可重复、可取消，不能依赖“只挂载一次” |
| 可观测事实 | Request ID 属于单次 HTTP 关联；当前没有 Trace Context | 页面可展示关联 ID，但不能把它称为 trace |
| 缓存事实 | 错误 body 包含请求特定 request ID | 统一错误出口必须 `no-store`，避免旧错误被重放 |
| 产品事实 | 业务页面仍是 Mock | 本节不能宣称活动、抽奖、积分等已联调 |
| 部署事实 | 当前只有本地 Vite/Go 拓扑，没有生产反向代理方案 | 本地代理建立路径契约，但不能冒充生产组件 |
| 工具链事实 | React Router 当前版本要求与 React 版本相容，测试需要 DOM 环境 | 锁文件、Node engine、React/ReactDOM 和测试依赖要成为可复现边界 |

## 5. 从第一性维度推导机制

### 5.1 事实源：谁有资格宣布“正常”

服务状态不是 UI 自己拥有的事实。UI 只能解释后端返回的探针结果，而后端探针也只有受限语义：

- Go handler 是进程 liveness 字段的事实源；
- `ReadinessChecker.PingContext` 是当前 MySQL 连接检查的事实源；
- 浏览器单调时钟是浏览器侧 elapsed 的事实源；
- Request ID middleware 是服务端最终采用关联 ID的事实源；
- UI 只负责把这些事实组合成不超过其证据强度的文字。

因此，组件不能维护“已有 6 个服务正常”的数组，也不能在没有计时边界时显示伪造毫秒数。状态页展示的两个 card 与两个真实端点一一对应。

### 5.2 故障域：先区分失败发生在哪一层

一条请求至少跨越四个可独立失败的层：

```text
Browser / React
  │  same-origin fetch
  ▼
Vite dev/preview proxy
  │  upstream HTTP
  ▼
Go router / middleware / handler
  │  readiness only: PingContext
  ▼
MySQL
```

如果把所有失败都映射成“网络错误”，排查者无法区分 API 未启动、代理无法连接、Go 明确返回 503、响应格式漂移或用户切换页面导致取消。因此 `ApiFailureKind` 不是 UI 装饰，而是故障域压缩后的诊断模型。

### 5.3 权限：状态可见性也需要最小披露

本地学习页面可以读取两个探针，不代表生产应把 readiness 暴露给公共互联网。`/ready` 可能泄漏依赖存在性和故障时间，即使错误已脱敏。当前同源代理没有新增认证，也不是授权层。

由此得出两条结论：

1. 本节只定义本地调用路径和响应解释，不决定生产公网暴露；
2. 页面不得渲染驱动 cause、数据库地址、用户、DSN、SQL 或代理目标。

### 5.4 发布：调用路径必须与 upstream 拓扑解耦

浏览器若硬编码 `http://127.0.0.1:8080`，构建产物会携带开发拓扑；环境变化需要重新构建或注入公开变量，还会触发 CORS。使用 `/health`、`/ready` 同源路径后，开发由 Vite 转发，生产可以由 Nginx、Ingress、API Gateway 或同一进程静态托管承接，而浏览器调用契约不变。

可替换的是代理产品，不可悄悄改变的是：路径、方法、JSON 契约、Request ID/no-store 传播和错误语义。

### 5.5 可逆性：第一条链路必须容易撤销

本节只有 GET 探针和 UI 状态，没有业务写入，因此撤销成本低。若请求边界不合适，可以替换 `requestJSON` 实现而不回滚数据。反过来，若第一条链路选择“创建活动”，错误契约会留下持久化数据、幂等键和权限历史，学习成本与恢复半径都会放大。

### 5.6 可观测性：关联信息必须来自真实响应

成功响应从 `X-Request-ID` header 取 ID；错误响应还要求 envelope 中有 `error.request_id`。两者同时存在但不一致时 fail closed，因为任选一个都会让排查关联不确定。成功 header 缺失不破坏业务 payload，但 UI明确显示“响应未提供”，而不是生成一个假的 ID。

### 5.7 学习成本：先建立薄边界，再引入框架

原生 `fetch`、小型 decoder、明确 hook 状态机让每个机制可见：哪里设置 timeout、哪里取消、哪里做契约校验、哪里防 stale result。此时引入 Axios、React Query、Zod、MSW 和 OpenAPI 会一次性增加多套概念，却没有足够用例证明它们各自解决了真实重复成本。

这不是把手写视为美德，而是要求抽象必须由重复和风险证明。后续若手写 decoder 数量、缓存失效或 mock 维护成本显著增长，当前选择应主动被推翻。

## 6. 同源代理与 CORS：先决定产品拓扑，再写响应头

### 6.1 为什么本地端口不同不能自动推导出生产跨域

浏览器判断 origin 使用 scheme、host、port 三元组。开发页面 `http://127.0.0.1:5173` 直接访问 `http://127.0.0.1:8080` 确实跨源，但这只证明“本地开发有两个端口”，不证明“生产产品需要跨站浏览器客户端”。

若仅为解决本地端口问题就在 Gin 添加 `Access-Control-Allow-Origin: *`，就会把开发便利变成后端公共策略。将来一旦有 Cookie、Authorization header、CSRF 或第三方嵌入，宽泛 CORS 会成为安全债务。

### 6.2 当前同源方案

`web/vite.config.ts` 在 development 与 preview 中共用代理表：

- 精确 `^/health$`；
- 精确 `^/ready$`；
- `/api` 或 `/api/...`；
- 原样转发，不 rewrite；
- 默认 upstream 为 `http://127.0.0.1:8080`；
- dev 默认 `127.0.0.1:5173`，preview 默认 `127.0.0.1:4173`；
- 两者都 `strictPort: true`，端口占用直接失败，不静默漂移。

只代理精确系统路径避免 `/health-anything` 被意外转发；预留 `/api` 是为后续业务路由保持原样路径，并不证明已有业务 API。

### 6.3 Node-side 环境变量为什么不使用 `VITE_`

Vite 会把特定前缀环境变量暴露给客户端代码。代理 upstream 是 Node 配置，不是浏览器运行时配置，因此采用 `GROWTHOS_WEB_API_PROXY_TARGET` 并由 `loadEnv` 在配置端读取。

该值只接受：

- `http:` 或 `https:`；
- 无 username/password；
- pathname 必须是 `/`；
- 无 query、fragment。

这阻止配置把凭据嵌在 URL 中，也防止调用路径与 upstream base path 发生隐式叠加。它不是 Secret 容器；即使没有 `VITE_`，也不应放数据库密码或长期令牌。

### 6.4 同源不是万能安全边界

同源代理只解决浏览器跨源访问和拓扑解耦，不提供：

- 用户认证或 API 授权；
- CSRF 防护；
- TLS 或证书身份；
- rate limit、WAF、输入校验；
- upstream allowlist；
- 可信客户端 IP；
- 生产缓存与 header 转发保证。

本地 Vite proxy 甚至不应被部署为生产网关。生产必须另行验证静态资源与 API 的路由、TLS、缓存、超时、header 传播和探针暴露策略。

### 6.5 何时应该改为显式 CORS

只有出现可信的真实需求，例如独立域 SPA、第三方嵌入或多前端 origin，才重评 CORS。届时至少要同时决定：

- 精确 origin allowlist，是否允许环境通配；
- credentials 是否开启；
- Cookie 的 SameSite/Secure/Domain；
- CSRF token 或双重提交策略；
- allowed methods/headers 和 preflight 缓存；
- `Access-Control-Expose-Headers: X-Request-ID`；
- 错误响应和重定向的跨源行为；
- CDN/网关是否正确合并 `Vary: Origin`。

缺其中任一项，都不能用“加一个 CORS middleware”概括完成。

## 7. 统一 HTTP 边界：transport 机制与端点契约分层

### 7.1 为什么不能让组件直接 `fetch`

组件直接 fetch 的初始代码少，但会快速复制：

- path 构造；
- Accept、cache、credentials、redirect；
- timeout 和调用方取消合并；
- Content-Type/JSON 判断；
- error envelope 解码；
- Request ID 读取与一致性；
- elapsed 计算；
- 原始错误脱敏。

重复不仅是维护问题，更会导致同一种后端错误在不同页面被解释成不同故障。于是建立两层：

```text
httpClient.ts
  负责 transport、时间、取消、通用错误、关联元数据
          │ unknown JSON
          ▼
systemApi.ts
  负责具体路径、status literal、version、timestamp 的运行时契约
          │ typed ApiResponse
          ▼
useSystemStatus.ts / SystemStatusPage.tsx
  负责并发状态与用户解释
```

`httpClient` 不应该知道 MySQL 或 readiness；`systemApi` 不应该重新实现 timeout；组件不应该解析 error body。

### 7.2 同源路径本身也是输入

`requestJSON` 只接受以单个 `/` 开始、且不含反斜杠的路径。它拒绝：

- `http://example.com/...`；
- `//example.com/...`；
- `health`；
- 带反斜杠的混淆路径。

这不是完整 URL 安全验证器，而是让调用方只能表达当前设计允许的同源路径。浏览器的 `mode: "same-origin"` 再提供运行时防线。未来若确实需要访问外部 origin，应建立独立客户端与 allowlist，而不是放宽这个函数。

### 7.3 请求默认值是有语义的

当前 GET 请求明确设置：

| 选项 | 当前值 | 推导理由 |
| --- | --- | --- |
| `Accept` | `application/json` | 声明客户端只理解 JSON 契约 |
| `cache` | `no-store` | 每次读取当前探针，不复用浏览器缓存 |
| `credentials` | `same-origin` | 保留未来同源凭证语义，不向跨源发送 |
| `mode` | `same-origin` | 与同源路径不变量双重对齐 |
| `redirect` | `error` | 探针被登录页/错误页重定向时不能静默接受 |
| timeout | 默认 5000ms，允许 100～30000ms 安全整数 | 防止无限等待，也避免 0/负数/极端值 |

这些默认值只适合当前只读 JSON 探针。未来 POST、上传、流式响应、SSE 或下载不能直接沿用并假装通用。

## 8. 运行时 decoder：TypeScript 只保护编译器看得到的世界

`interface HealthResponse` 能阻止源码把 `version` 当 number，却不能阻止服务器、代理或旧版本发送任意 JSON。`response.json() as HealthResponse` 只是告诉编译器“相信我”，没有产生验证。

当前 `systemApi` 对两个端点分别检查：

- 顶层必须是非数组 object；
- health 的 `status` 必须精确为 `"ok"`；
- readiness 的 `status` 必须精确为 `"ready"`；
- `version` 必须是非空 string；
- `timestamp` 必须匹配 RFC3339 形状且可由 `Date.parse` 解析；
- 额外字段允许存在，decoder 只返回已承诺字段。

### 8.1 为什么允许额外字段

若 decoder 要求对象字段集合精确相等，服务端增加一个兼容的可选字段也会让旧前端全部失败。允许额外字段支持 additive evolution；必需字段和值保持严格，避免把破坏性变化当成功。

### 8.2 为什么暂时不用 Zod

Zod 等 schema 库可提供组合、错误路径、推导类型和复用，真实项目中很有价值。当前只有两个三字段响应，手写 decoder 的优点是：

- 无新增生产依赖；
- 成功/失败边界完全可读；
- 能精确选择兼容的额外字段策略；
- 学习者看到“unknown 进入、typed value 离开”的本质。

重评触发器不是“字段达到某个神奇数字”，而是出现可测问题：decoder 大量重复、嵌套 union/recursive schema、错误定位需求、前后端 schema 共享、手写校验缺陷增加。届时可以引入 Zod/Valibot 或由 OpenAPI 生成 validator，但必须保留运行时校验，不能只生成 TypeScript type。

### 8.3 为什么当前时间校验仍不是绝对证明

regex + `Date.parse` 能拒绝明显非法值，但不能证明服务端时钟准确、时区配置合理或时间没有漂移。页面把它标为“服务端时间”，不拿它计算 SLA。未来若做延迟或时钟偏差分析，需要服务端 timing、RUM 和时间同步证据。

## 9. 错误分类：让可恢复动作跟随故障语义

当前客户端分类为 `http`、`gateway`、`network`、`timeout`、`cancelled`、`contract`。分类依据不是底层异常名字，而是调用方能安全知道的事实。

| kind | 当前判定 | 可以采取的动作 | 不能断言 |
| --- | --- | --- | --- |
| `http` | 收到非 2xx 且 JSON error envelope 合法 | 用 status/code/request ID 排查服务明确结果 | 网络完全正常、根因就是 message 文本 |
| `gateway` | 收到 502/503/504，且响应不是 JSON | 检查代理/upstream | Go 一定没运行；也可能是中间网关策略 |
| `network` | fetch 没有产生可信 HTTP 响应 | 检查 API、代理、连接 | MySQL 一定故障 |
| `timeout` | 客户端本地 timer 先到并 abort | 提示观察窗口耗尽，考虑重试/排查 | 服务端工作已停止 |
| `cancelled` | 调用方 signal 或生命周期取消 | 静默丢弃旧轮次 | 服务发生故障 |
| `contract` | 本地路径/timeout 非法，或 Content-Type/JSON/envelope/payload/ID 不可信 | 修复调用或版本/路由契约 | 单纯重试一定恢复 |

### 9.1 502/503/504 gateway 与 Go 503 的关键区别

不能写成“503 就是 MySQL 未就绪”。同一个 status 可以由不同层产生：

- Go `/ready` 返回 `503 + application/json + dependency_unavailable envelope`，分类为 `http`；
- Vite/反向代理返回 `502/503/504 + 非 JSON`，分类为 `gateway`；
- 非 2xx JSON 但 envelope 不合法，分类为 `contract`；
- 没有取得任何 HTTP Response，分类为 `network`。

只有第一种，并且同轮 health 成功，UI 才显示“API 存活，MySQL 未就绪”。这个判断故意同时依赖 transport、status、code 和另一探针事实。

### 9.2 为什么不解析 message 做分支

`message` 面向人，可能调整文案、翻译或脱敏。程序分支使用稳定 `kind/status/code`。否则把“service unavailable”改成中文就可能破坏状态机。

### 9.3 为什么不把原始异常直接显示给用户

fetch exception、代理 body 或数据库 cause 可能包含 URL、内部拓扑、驱动信息、Secret 或浏览器差异文本。统一 client 将未知错误压缩成安全话术；详细诊断应通过受控日志和 Request ID，不通过页面回显任意对象。

### 9.4 `no-store` 为什么必须补到错误出口

成功探针已经禁止缓存，但统一错误 envelope 中也有请求特定 `request_id`。若 CDN、浏览器或共享代理缓存 503/404 后重放，后来请求会携带旧 ID，破坏排障关联，并可能把另一请求的相关性暴露给错误用户。

因此 `abortWithFaultStatus` 统一设置 `Cache-Control: no-store`。这覆盖通过该出口产生的 readiness 503、404、405 和 fault 响应。它是服务端缓存指令，不证明生产代理一定遵守；生产仍需实际缓存验收。

## 10. health 与 readiness：独立状态优先于“一个漂亮的总状态”

### 10.1 两个探针回答不同问题

- `/health`：进程能否处理这次 HTTP 请求；不访问 MySQL；
- `/ready`：当前实例能否在有界时间内 Ping MySQL；失败表示不适合承接依赖该数据库的流量。

将二者串行会增加延迟，并在 health 失败时丢掉 readiness 的诊断结果。用 `Promise.all` 合成单个 Promise 又会让第一个 rejection 遮蔽另一个结果。因此 hook 同时发出两个请求，分别更新各自 `loading/success/error`，`Promise.allSettled` 只负责记录“本轮全部结算”的时间。

### 10.2 聚合真值表

| health | readiness | 汇总结论 | 设计理由 |
| --- | --- | --- | --- |
| loading | 任意 | 正在检查 | 一轮尚未完整结算，但单卡可先显示 |
| 任意 | loading | 正在检查 | 不提前宣布整体健康 |
| success | success | 已接入检查正常 | 两个当前证据都成立 |
| success | 合法 503 `dependency_unavailable` | API 存活，MySQL 未就绪 | 精确保留最有价值的降级状态 |
| success | 其他 error | API 存活，就绪状态未知 | 不能把代理/contract/timeout 都说成 MySQL 故障 |
| error | success | 无法确认 API 状态 | readiness 不能替代 liveness；可能两请求命中不同实例 |
| error | error | 无法确认 API 状态 | 没有可信 liveness 证据 |

“unknown”不是敷衍，而是诚实地表达证据不足。错误系统经常因害怕显示未知而强行二值化，最终得到错误确定性。

### 10.3 为什么版本不一致不是自动红灯

`/health` 和 `/ready` 是两次请求。生产负载均衡或滚动发布时，它们可能命中不同实例，因此 version 不同可能是合法发布窗口。当前 UI 分别展示版本并提示边界，不自动比较后报警。

如果未来必须证明同一实例的原子快照，应由路由粘性、实例 ID 或聚合端点提供新证据，而不是由浏览器猜测。

## 11. 并发、取消、timeout 与 React Strict Mode

### 11.1 资源所有权

每一轮 refresh 创建一个 `AbortController`，hook 成为该轮两请求的生命周期 owner：

```text
refresh generation N
  ├─ abort generation N-1 controller
  ├─ create controller N
  ├─ start health(signal N)
  ├─ start readiness(signal N)
  └─ both settle -> completedAt, only if N is still current

effect cleanup / unmount
  ├─ increment generation
  └─ abort active controller
```

`requestJSON` 对单次请求另建内部 controller，并把外部 signal 转发进来，同时拥有自己的 timeout timer。它在 `finally` 中清 timer、移除外部 abort listener。由此形成两层所有权：hook 决定一轮何时过期，HTTP client 决定单请求何时超时并清理监听器。

### 11.2 为什么 abort 还不够

标准 fetch 会响应 AbortSignal，但测试 fake、旧 polyfill、某些封装或已完成的 microtask 可能忽略取消并继续 resolve。因此状态更新同时检查：

- `generation.current === currentGeneration`；
- 当前 controller 尚未 aborted。

Abort 尽量停止无用工作，generation 保证即使工作没有停止也不能覆盖新状态。两者解决的是不同问题。

### 11.3 Strict Mode 不是“开发环境多请求的 bug”

React Strict Mode 在开发中执行 setup → cleanup → setup，用来暴露 effect 缺少清理的问题。如果第一轮 cleanup 后仍能更新 UI，生产中的快速路由切换同样可能出错。

当前 effect 调用稳定的 `refresh`，cleanup 增加代际并取消活跃请求。测试使用 `reactStrictMode: true` 证明第一轮 signal 被取消，第二轮仍处于可用状态。设计目标不是阻止开发环境观察到两次请求，而是保证多一次生命周期探测不污染状态。

### 11.4 timeout 的语义与预算顺序

浏览器默认 timeout 为 5 秒；Go readiness 使用配置中的 MySQL Ping timeout，当前默认 3 秒，并且后端配置还为写出响应预留 1 秒。默认情况下，服务端应先给出 503，再由浏览器 5 秒窗口兜底。

但这不是不可变定律：client 允许 100ms～30s 覆盖，生产网关也会有自己的 timeout。未来必须满足可解释的预算链：

```text
MySQL probe deadline
  + Go 序列化/写响应余量
  < gateway upstream timeout
  < browser observation timeout
```

并为网络抖动留余量。若浏览器先超时，它只能说“我停止等待”；服务端是否因 request context 取消而真正停止，还取决于代理是否传播断开、Go handler 和驱动是否尊重 context。

### 11.5 为什么当前不自动重试

探针是一次观测。自动重试会改变用户看到的事实：第一次 503 可能被第二次 200 掩盖，还会在依赖故障时同步放大请求。当前失败后保留结果，用户手动刷新。

未来若引入自动重试，必须先决定：

- 哪些 kind 可重试；contract/cancelled 通常不应盲重试；
- 最大次数、指数退避、jitter；
- 页面隐藏时是否暂停；
- 是否显示第一次失败和恢复时间；
- 多用户同时打开页面对 API/MySQL 的放大；
- retry 是否消耗同一总体 deadline。

## 12. Request ID 与 Trace：相关不等于因果链

### 12.1 当前 Request ID 能做什么

Go middleware：

- 接受单个、长度不超过 64、字符集安全的 incoming `X-Request-ID`；
- 缺失、多值或非法时生成新 ID；
- 写回响应 header；
- 放入 Gin 与 `context.Context`；
- 访问日志记录该 ID。

前端显示每个请求的响应 ID。排查者可以用它关联当前 Go access log，这已经显著优于“页面报错但没有查询键”。

### 12.2 为什么它不是身份或安全令牌

安全格式的 incoming ID可以由客户端提供，因此它不是不可伪造证据。它不能用于：

- 认证用户；
- 授权资源；
- 幂等键；
- 防重放；
- 计费；
- 证明请求来自某个可信代理。

页面展示它也不意味着应把它复制到分析事件或外部工单而无数据治理。

### 12.3 为什么它不是分布式 Trace

Trace 通常需要 trace ID、span ID、父子关系、采样、跨服务传播、时序与属性。当前 Request ID 只有单次 HTTP 关联，两个探针各有一个 ID，也没有把 MySQL Ping 建成 span。

未来接入 OpenTelemetry/W3C Trace Context 时，可以同时保留 Request ID 作为人类友好的日志关联键，但不能简单把 Request ID 改名为 trace ID。需要明确：

- 信任/重建 incoming `traceparent` 的规则；
- gateway、Go、数据库和前端 RUM 的 span 边界；
- 采样导致“有 ID 但无完整 trace”的解释；
- PII、SQL、header 的 attribute 脱敏；
- Request ID 与 trace ID 在日志中的双向关联。

### 12.4 header/body ID 一致性为什么 fail closed

错误 body 与 header 同时出现却不同，可能来自缓存重放、中间代理改写、服务 bug 或响应拼接。任选其一都会让排查者搜索错误请求。客户端因此将其视为 contract error。对成功响应，只有 header 是当前契约来源，缺失则如实显示缺失。

## 13. UI 设计：可访问性也是事实完整性

### 13.1 状态不能只靠颜色

红绿色对部分用户不可区分，屏幕阅读器也不会自动理解背景色。因此每个状态同时使用：

- 明确文字；
- 语义一致的图标；
- 颜色作为冗余线索；
- `role="status"`、`aria-live="polite"` 和 `aria-atomic="true"`。

loading 还使用“检查中”文字与旋转图标；刷新是原生 button，检查期间禁用，避免 UI 产生无边界的并发轮次。

### 13.2 为什么显示 endpoint、版本、服务端时间和 ID

状态卡不只说“正常”，还展示可复核上下文：

- endpoint 说明观察的是哪个契约；
- version 说明响应来自哪个构建标签；
- 服务端时间说明 payload 生成时间；
- Request ID 提供日志关联键；
- 浏览器 RTT 提供本次用户侧观察值。

这些字段使截图或口头报告更可诊断，但仍不能代替 Network 面板、服务日志或监控。

### 13.3 浏览器 RTT 为什么不能叫“服务延迟”

`elapsedMs` 使用 `performance.now()`（不可用时回退 `Date.now()`），范围包括浏览器调度、Vite/生产代理、网络、Go、readiness MySQL Ping、响应读取和 JSON 解码。它不是 server timing，也没有排除客户端负载。

因此 UI 标为“浏览器往返”，不标“接口耗时”或“MySQL 延迟”。若未来需要分层耗时，应增加 `Server-Timing`、后端 metrics/span 和 RUM，而不是从一个数反推内部阶段。

### 13.4 为什么没有常驻绿色徽标

页面不自动轮询，状态会随着时间陈旧。一个长期固定在导航栏的绿色圆点容易被解释为实时监控。当前只提供 Status 入口，状态页明确是“浏览器刚刚取得”的结果，并显示本轮完成时间。

## 14. 备选技术矩阵与重评条件

### 14.1 请求库与 server state

| 方案 | 能解决什么 | 当前额外成本/风险 | 当前结论 | 重新评估触发器 |
| --- | --- | --- | --- | --- |
| 组件直接 fetch | 最少初始代码 | 机制散落、错误语义漂移 | 不采用 | 不适用；通用边界已经形成 |
| 统一原生 fetch adapter | timeout、取消、decoder、ID、错误统一 | 需维护薄封装 | 采用 | HTTP 能力增长到封装难以清晰承载 |
| Axios | interceptor、transform、timeout、生态 | 新依赖；与平台 fetch/AbortSignal 形成双抽象 | 暂不采用 | 多 transport adapter、上传进度、成熟 interceptor 需求有证据 |
| TanStack Query / SWR | cache、dedupe、retry、失效、后台刷新 | 探针一次观测不需要 cache；默认重试可能掩盖故障 | 暂不采用 | 多页面共享 server state、mutation invalidation、后台刷新出现 |
| Zustand 保存探针 | 客户端全局访问方便 | 瞬时 server state 与主题/本地偏好生命周期不匹配，容易陈旧 | 不采用 | 只有明确跨路由共享且定义 freshness 后才讨论 |

### 14.2 契约工具

| 方案 | 优点 | 当前问题 | 当前结论 | 触发器 |
| --- | --- | --- | --- | --- |
| 手写 decoder | 显式、无生产依赖、易理解 | 复杂对象会重复 | 采用 | 重复/嵌套/错误路径成本上升 |
| Zod/Valibot | 可组合 schema、类型推导、细粒度错误 | 当前两份三字段契约收益有限 | 暂不采用 | 多模块共享 schema 或手写缺陷可测 |
| OpenAPI + codegen | 单一规范、SDK/文档生成 | 当前没有业务 API 治理流程；生成 type 仍需 runtime 行为选择 | 暂不采用 | 首批业务 API 稳定且多客户端需要契约治理 |
| JSON Schema validator | 标准化 runtime 校验 | 构建/生成/错误映射复杂 | 暂不采用 | 外部客户端或契约测试要求标准 schema |

### 14.3 测试与 Mock 工具

| 方案 | 当前价值 | 当前代价 | 结论 | 重评触发器 |
| --- | --- | --- | --- | --- |
| 注入 `FetchLike` | 可精确制造 Response、reject、abort、时钟 | 测试要手写响应 | 采用 |
| Vitest + jsdom + Testing Library | 快速验证 client、hook、DOM 语义 | 不经过真实浏览器网络/代理 | 采用 |
| MSW | 接近网络层、可复用 handler | 当前两个端点会多一套 Mock 契约；可能与真实 API 漂移 | 暂不采用 | 组件/集成测试跨多个端点且 handler 复用显著 |
| 只做端到端浏览器测试 | 能看到真实链路 | 慢、难穷举竞态和 timer、故障定位粗 | 不单独采用 | 始终作为顶部证据而非替代单测 |

### 14.4 行为策略

| 方案 | 当前结论 | 原因 | 触发重评 |
| --- | --- | --- | --- |
| 自动轮询 | 不做 | 无 freshness/SLA/容量预算，页面不是监控器 | 明确实时性目标、可见性暂停、退避和容量 |
| 自动重试 | 不做 | 会掩盖首次失败并放大故障 | 明确 kind 策略、预算、jitter、展示语义 |
| 聚合端点 | 不做 | 两端点足够，保留独立事实 | 探针数量/RTT/原子快照/权限需求增长 |
| CORS | 不做 | 只有本地双端口，没有产品跨域需求 | 真实跨域客户端与身份模型出现 |
| `/api/v1/health` 别名 | 不做 | 系统探针不是版本化业务资源，重复路径会漂移 | 生产网关存在不可改变的统一前缀要求 |

## 15. 失败模型：从浏览器一直推演到 MySQL

### 15.1 分层故障表

| 故障位置 | 可能观察 | 当前分类/页面 | 恢复动作 | 尚存歧义 |
| --- | --- | --- | --- | --- |
| 组件卸载/新轮 refresh | AbortSignal 触发 | cancelled，不让旧轮更新 | 新轮继续或页面离开 | 服务端是否已经停止 |
| 浏览器本地 deadline | controller abort | timeout，状态未知 | 手动重试/查慢点 | 请求是否已到服务器 |
| DNS/TCP/API 未监听 | fetch reject | network | 检查 Vite/API/端口 | 浏览器错误不一定指出哪跳 |
| Vite upstream 失败 | 非 JSON 502/503/504 | gateway | 检查 proxy target/API | 其他网关 status 可能归 contract |
| 路由错误返回 HTML | 非 JSON 或 decoder 失败 | contract | 查 path/rewrite/deploy | 可能是 SPA fallback 而非 Go |
| Go 404/405 envelope | 合法非 2xx JSON | http | 检查方法/路由 | 服务本身仍可运行 |
| Go panic 未提交响应 | 安全 500 envelope | http | 用 Request ID 查受控日志 | 页面看不到内部 cause |
| `/ready` MySQL Ping 失败 | 503 dependency_unavailable | health 成功时 warning | 修复依赖后手动刷新 | 不能区分认证/网络/容量，因有意脱敏 |
| MySQL 恢复但旧页面未刷新 | 页面仍显示旧失败 | 这是陈旧观察，不是新故障 | 手动刷新 | 当前没有 TTL/自动轮询 |
| 服务响应字段漂移 | 200 但 decoder 拒绝 | contract | 协调版本/契约 | 额外字段不触发失败 |
| error header/body ID 冲突 | 两个不同 ID | contract | 查缓存/代理/服务 | 不选择任一 ID继续 |

### 15.2 部分成功

两个请求并行，任一可以先完成、失败或被取消。设计没有事务性地“同时发布”两张卡片，因为真实网络本来就不是原子快照。先完成的单项先显示，汇总在任一 loading 时保持“正在检查”。

这带来一个明确权衡：用户可更早看到单项事实，但短暂时段内汇总不是终态。若未来产品要求原子状态快照，需要在服务端聚合，而不是让前端等待后假装两请求同一时刻。

### 15.3 重复与乱序

- Strict Mode 可能产生 setup/cleanup/setup；
- 程序可以在上一轮未结束时直接调用 `refresh`；
- 旧 Promise 即使忽略 abort，也可能在新 Promise 之后完成；
- 两个探针内部完成顺序不固定。

generation 使“最新轮次拥有写状态权”成为不变量。页面检查期间禁用按钮减少用户重复操作，但正确性不依赖按钮；hook 测试直接制造旧轮晚到。

### 15.4 错误恢复

当前恢复语义是“修复外部条件后显式发起新一轮”。不把错误 state 自动改回成功，也不在时间到后自动清空。新一轮先清为 loading，成功/失败只来自新 response；旧 request ID 不复用。

### 15.5 旧版本与滚动发布

开发 Vite 通常只有一个 upstream，但生产可能有多实例。旧/新 Go 版本只要都满足当前 probe 契约，前端应兼容额外字段。破坏 `status/version/timestamp` 则成为 contract error。

版本不同只是诊断线索，不是兼容性证明。真正的 rolling compatibility 需要 API 版本策略、部署矩阵和契约测试，本节没有建立。

## 16. 安全与隐私推导

### 16.1 秘密生命周期

- `web/.env.example` 只包含非秘密的 loopback proxy origin；
- `.env*` 默认忽略，仅 `.env.example` 可提交；
- proxy URL 明确拒绝 username/password；
- MySQL 密码、DSN、CA、测试账号不进入 web env、bundle、UI 或文档；
- Node-side 非 `VITE_` 变量降低意外编译暴露，但不是存 Secret 的许可。

### 16.2 输入与输出

- 路径在发送前限制为同源形式；
- 网络 JSON 从 `unknown` 开始验证；
- error envelope 的三个字段必须为非空字符串；
- UI 不渲染任意后端 message/cause，而使用按 kind/code 定义的安全话术；
- 长 Request ID 使用可换行样式，防止布局破坏；Go 端还限制 incoming ID 长度与字符集。

### 16.3 缓存与关联隐私

请求端和服务端都使用 `no-store`，原因不只是“状态要新鲜”，也包括错误 body 的 request-specific ID。生产需确认 CDN 不覆盖指令、错误页不被默认缓存、认证响应设置更严格的缓存策略。

### 16.4 readiness 暴露风险

即使响应没有数据库详情，攻击者也可能用频繁请求推断依赖故障窗口或造成 Ping 放大。生产应该考虑：

- 将 liveness/readiness 只开放给编排器/内网；
- 为用户状态页提供权限受控、语义更合适的产品状态 API；
- 对探针应用网络 allowlist 和速率限制；
- 不让每个公网用户请求都直达 MySQL Ping。

当前本地页面不解决这些生产问题。

## 17. 依赖与工具链：测试能力也是架构的一部分

### 17.1 为什么增加 Vitest、Testing Library 和 jsdom

本节首次出现纯类型检查无法覆盖的行为：fake timer、AbortSignal、Promise 乱序、effect cleanup、Strict Mode 和可见文案。选择：

- Vitest：与 Vite/ESM/TypeScript 配合，提供 fake timer、mock 和测试运行器；
- Testing Library：从用户可见 DOM 和 hook 行为验证，不依赖组件内部实现细节；
- jsdom：为 hook/组件测试提供 DOM 环境。

它们是开发依赖，不进入生产 bundle。`Makefile` 将 `web-test` 纳入 `web-verify`，总门禁 `make verify` 继承 test、typecheck 和 build。

### 17.2 为什么锁版本与 engine 边界重要

首次加入前端测试链会放大 peer dependency 和 Node runtime 差异。`package.json` 记录 Node 最低版本、package manager，并让 lockfile 固化完整解析；React/ReactDOM 更新到与当前 React Router peer 约束相容的版本。

这不意味着 semver 范围永远安全。CI 和本地都应使用 frozen lockfile；升级必须重新跑测试、typecheck、build 和浏览器联调。

### 17.3 供应链主动检查

架构师还应检查：

- lockfile 是否来自受信 registry，安装脚本是否可接受；
- 新依赖许可证与漏洞；
- jsdom/Vitest 只在 dev dependency；
- bundle 中是否意外包含测试代码或 proxy target；
- `node_modules`、pnpm store 是可复用依赖，不作为任务临时垃圾删除；
- 生产制品应来自干净、可复现的安装和构建环境。

这些不是本节代码能一次证明的长期事实，需要 CI 和依赖治理持续执行。

## 18. 测试金字塔：每一层试图证伪什么

### 18.1 HTTP client 单元测试

`web/src/api/httpClient.test.ts` 使用注入的 `FetchLike`、可控时钟和 fake timer，设计用于证伪：

- 请求遗漏 `no-store/same-origin/AbortSignal`；
- HTTP 503 丢失 code/request ID；
- decoder 异常或字段漂移被当成功；
- header/body ID 冲突仍继续；
- fetch reject 被错误分类；
- 非 JSON 502 被误认为 Go JSON 503；
- timeout 和外部取消混淆；
- 危险路径在发送前没有拒绝。

它不能证明 Vite proxy 实际转发、真实浏览器 fetch 行为完全一致、生产网关返回哪种错误页。

### 18.2 端点 decoder 测试

`web/src/api/systemApi.test.ts` 设计用于证明调用精确根路径 `/health`、`/ready`，并拒绝空 version/非法 timestamp。它不能证明 Go 实际当前版本响应恰好一致；那需要真实 HTTP 联调。

### 18.3 Hook 并发测试

`useSystemStatus.test.tsx` 用 deferred Promise 设计用于证伪：

- health 完成时 readiness 状态被错误覆盖；
- refresh 没有 abort 旧 signal；
- 旧 Promise 晚到覆盖新结果；
- unmount 后仍拥有写状态权；
- Strict Mode 第一轮污染第二轮。

它不能证明真实网络一定响应 abort，也不能测用户视觉体验。

### 18.4 组件测试

`SystemStatusPage.test.tsx` 设计用于证伪：

- loading 时显示“全部正常”；
- 页面重新出现六个虚构服务；
- readiness 503 没有保留 API 存活；
- health 失败却宣布整体健康；
- 内部错误 message 被渲染；
- 刷新按钮没有调用 hook。

它在 jsdom 中运行，不能证明 CSS 对比度、真实布局、浏览器 Network 和代理联通。

### 18.5 Go handler 测试

`internal/infrastructure/httpapi/errors_test.go` 的统一 assertion 检查 error envelope 具有 `Cache-Control: no-store`、JSON、Request ID；既有 health/readiness 测试检查成功/503、timeout、脱敏和方法边界。

它不能证明 CDN 遵守 no-store，也不能证明生产 MySQL 权限与网络正确。

### 18.6 真实浏览器与真实 MySQL

顶层证据必须同时启动真实 MySQL、Go API 与 Vite，在浏览器确认：

- 地址栏 origin 下发出 `/health`、`/ready`；
- Vite 不 rewrite，Go access log 能关联 ID；
- 正常状态只显示两项真实服务；
- MySQL 不可用时 health 保持 200、ready 变 503、页面显示降级；
- API 不可达时显示 gateway/network/unknown，而非 MySQL 故障；
- 恢复后手动刷新取得新 ID 与新结果。

实际执行情况只写入 QA。本文只解释为何需要这些证据。

### 18.7 测试绿色仍然证明不了什么

即使所有层都通过，也不能证明：

- 生产反向代理与 Vite 行为相同；
- 多实例滚动发布没有契约漂移；
- 5 秒 client timeout 符合真实 P99/P99.9；
- 页面满足全部 WCAG 对比度/读屏器组合；
- 高频刷新不会放大 MySQL；
- 供应链无漏洞；
- readiness 等于业务正确或 SLA；
- Request ID 能跨未来 MQ/异步任务形成完整因果链。

## 19. 生产部署边界

### 19.1 当前可携带到生产的是契约，不是 Vite server

可以携带的性质：

- 浏览器使用同源路径；
- `/health`、`/ready` 不 rewrite；
- JSON、Request ID、Cache-Control 等 header 被保留；
- 非 JSON gateway error 与 Go envelope 可区分；
- 静态资源 fallback 不吞 API 路径。

不能携带的假设：

- Vite dev/preview 作为生产服务；
- API 永远在 loopback 8080；
- `changeOrigin` 足以定义可信 Host；
- 没有 TLS、认证、缓存或限流；
- readiness 对所有浏览器公开。

### 19.2 生产反向代理必须回答的问题

1. 静态 SPA 与 API 由同一域、子路径还是不同域提供？
2. `/health`、`/ready` 是给编排器还是最终用户？
3. 代理连接、首字节和总响应 timeout 分别多少？
4. 客户端断开是否传播到 Go request context？
5. `X-Request-ID` 是保留、重建还是双 header，如何防注入？
6. 是否传播 `traceparent`，与 Request ID 如何关联？
7. 503/502 是否被 CDN/Ingress 缓存或替换成 HTML？
8. SPA fallback 是否错误地把 `/ready` 返回 `index.html`？
9. 是否给 probe 配 rate limit、网络 allowlist 和日志采样？
10. 发布中旧/新版本的 probe payload 是否向后兼容？

这些答案出现前，不能把“本地联调通过”写成“生产架构完成”。

## 20. 被刻意推迟的能力

| 推迟能力 | 为什么现在不做 | 当前风险 | 明确重评触发器 |
| --- | --- | --- | --- |
| 业务 API 联调 | 没有 schema/Repository/用例事实 | 业务页面继续是 Mock | 对应领域章节完成真实 read/write 用例 |
| Axios | 两个 GET 的统一 fetch 已足够 | 自维护 adapter 可能增长 | interceptor/上传/复杂 transport 需求可测 |
| Zod | 三字段 decoder 很小 | decoder 重复可能增加 | 嵌套 schema、共享验证、错误路径需要 |
| React Query/SWR | 无 cache/invalidation 背景刷新需求 | 手写 server state 未来可能膨胀 | 多页面共享、mutation、dedupe、后台刷新 |
| MSW | 注入 FetchLike 足以穷举当前错误 | Mock 分散后会难维护 | 多端点集成测试需要统一网络模拟 |
| OpenAPI/codegen | 当前仅消费稳定系统探针 | 手写契约可能漂移 | 首批业务 API、多客户端与版本治理出现 |
| 聚合 status API | 两个独立请求成本低、诊断价值高 | 非原子、可能命中不同实例 | 探针数量增长、原子快照或权限需求 |
| 自动轮询/重试 | 没有 freshness、容量、退避策略 | 页面状态会陈旧 | 明确实时目标、QPS 预算、visibility 策略 |
| CORS | 没有产品跨域需求 | 未来分域需重新设计 | 真实跨域客户端与认证/CSRF 模型 |
| OTel/Trace Context | 当前只有单 Go 进程和 MySQL Ping | Request ID 无完整因果图 | 跨服务/MQ/性能诊断进入观测章节 |
| 前端 RUM/Server-Timing | 当前 RTT 只用于一次诊断 | 不能分解耗时 | 有真实 SLO、采样和隐私策略 |
| 生产网关/Compose | 第 16 节才定义容器拓扑 | 本地方案不能直接生产化 | Compose/部署章节出现实际网络边界 |

## 21. 架构师会主动检查、但原始需求没有明说的点

1. **Probe 放大系数。** 每次页面刷新执行一次 health 和一次 MySQL Ping。若 N 个用户每 T 秒轮询，额外 readiness QPS 是 `N/T`，并会消耗连接池。当前手动刷新避免在没有预算时制造放大。
2. **故障时的同步行为。** 如果未来自动轮询，所有页面整点同时请求会在 MySQL 故障时形成惊群，必须使用 jitter、退避和可见性暂停。
3. **客户端与编排器 probe 不一定应共用入口。** 编排器需要高频、低开销、内网语义；用户状态页需要权限、聚合和可解释历史。未来可能拆分，而不是无限扩展 `/ready`。
4. **SPA fallback 顺序。** 生产代理若先把未知路径重写到 `index.html`，`/ready` 路由错误会成为 200 HTML；runtime decoder 会 fail closed，但运维仍需修正路由。
5. **网关错误页面的状态差异。** 当前仅把非 JSON 502/503/504识别为 gateway；某些代理用 500、200 HTML 或 JSON 自有格式，可能落入 contract。这是保守分类，不是覆盖所有网关实现。
6. **代理 header 行为。** `changeOrigin` 修改 upstream Host，但没有定义可信客户端 IP、Forwarded/X-Forwarded-* 或 TLS scheme。Go 当前不信任代理列表；生产必须显式配置。
7. **缓存层的错误默认。** 一些 CDN 会对 404/503 应用负缓存或自定义 error page，可能剥离 Request ID/no-store。必须在实际链路验证。
8. **Request ID 基数与日志成本。** 每个探针一条访问日志；轮询会增加日志量。未来应计算采样、保留期和查询成本，不能随意丢失错误关联。
9. **版本字段的发布用途。** version 可辅助判断滚动发布，但不是制品签名或 commit provenance。供应链需要另行记录 artifact digest/SBOM。
10. **服务端时间与时钟漂移。** 两个实例时钟不同会让 timestamp 看起来乱序。没有 NTP/时钟监控时，不应用它排序跨实例事件。
11. **取消不等于回滚。** GET Ping 无业务副作用，风险小；未来 POST 即使浏览器 abort，服务端也可能已提交事务。HTTP client 的取消语义不能原样解释为业务撤销。
12. **浏览器 tab 生命周期。** 后台 tab timer 会被节流；自动监控依赖浏览器是不可靠的。真正告警应在服务端监控系统。
13. **可访问性重复播报。** 每张卡和汇总都使用 live region，真实读屏器下可能过度播报；当前应在浏览器验收，未来可只保留一个聚合 live region。
14. **国际化与时区。** 当前 `Intl.DateTimeFormat("zh-CN")` 以用户本地时区显示，payload 是服务端 UTC。若面向多语言，需要明确 locale 和原始值复制方式。
15. **长版本与 ID 布局。** version/request ID 是外部字符串；UI 使用 break-all，但仍需窄屏、缩放和超长值实测。
16. **“无 CORS”也要验证。** 未来某个通用 middleware 可能意外加宽 `Access-Control-Allow-Origin`；安全回归需要检查响应头，而不是只看本节没写 CORS。
17. **预览与开发一致性。** dev 正常而 preview 404 是常见遗漏，因此二者共用代理构造。生产 build 本身仍不含 proxy，不能把 preview 成功等同部署完成。
18. **端口确定性。** `strictPort` 让脚本、浏览器地址和 QA 不因自动换端口而指向错误服务。Docker 暴露需要显式重新决策 host，而不是恢复全局 `0.0.0.0` 默认。
19. **错误 message 的产品治理。** 后端 message 当前安全，但 UI 分支仍不渲染任意 message。未来若要展示服务端本地化消息，必须建立允许列表、i18n 和注入防线。
20. **成功 header 缺失策略。** 当前 payload 合法仍成功，ID 显示缺失。这是可用性优先；若将来合规要求每个响应强制可关联，可以把缺失升级为 contract failure，但会改变可用性。
21. **探针与 schema 版本。** readiness 只 Ping MySQL，不检查 Migration version。状态页明确写出该边界，避免用户误把 ready 当 schema compatible。
22. **依赖恢复的抖动。** 单次 ready 成功不证明依赖稳定。若未来显示历史健康，需要时间窗口、成功阈值和 hysteresis，而不是一次 200 立即全绿。

## 22. 容量与时间总账

### 22.1 请求数量

当前每次挂载/刷新：

```text
1 × GET /health
+ 1 × GET /ready
= 2 个 HTTP 请求，其中 1 次触发 MySQL Ping
```

Strict Mode 开发探测可能创建并取消第一轮，再建立第二轮；这在本地增加短暂请求，不应据此估算生产 QPS。若未来轮询：

```text
readiness QPS ≈ 活跃页面数 / 轮询秒数 × 重试放大系数
日志事件 QPS ≈ 2 × 活跃页面数 / 轮询秒数 × 重试放大系数
```

还需加入编排器 probe、监控系统和运维手工检查。容量是所有调用方总和，不是单页面看起来“只有一个 Ping”。

### 22.2 timeout 总账

当前相关默认：

- 浏览器 JSON client：5s；
- MySQL Ping：应用配置默认 3s；
- Go HTTP write timeout：默认 30s，配置校验要求 Ping + 1s 不超过它；
- Vite proxy：本节没有显式自定义 upstream timeout。

最后一项意味着本地行为仍部分依赖代理默认值。生产必须显式建立端到端预算，避免网关先超时却让 Go/MySQL继续占用资源，或 client 太短产生大量假超时。

### 22.3 状态数据生命周期

hook 只保留当前轮次内存状态，不落 localStorage、Zustand 或服务器缓存。页面刷新/卸载即丢失。这个选择避免陈旧状态被跨会话误用，也意味着没有历史趋势；趋势应来自 metrics/monitoring，不从 UI 状态缓存推导。

## 23. 不变量与信任边界

### 23.1 本节不变量

1. 浏览器调用系统探针时只使用同源 `/health`、`/ready`，不硬编码 upstream origin。
2. Vite dev 与 preview 对上述路径不 rewrite；`/api` 只是预留，不代表业务实现。
3. proxy target 不允许凭据、路径、query 或 fragment，不进入浏览器代码。
4. health 与 readiness 始终有独立状态；任何一个失败不能抹掉另一个结果。
5. 只有两个合法成功结果才能显示当前已接入检查正常。
6. 只有 health 成功和 readiness 合法 503/code 组合才能解释为 MySQL 未就绪。
7. 网络输入必须经过 runtime decoder；TypeScript `as` 不能作为契约验证。
8. 最新 generation 才能写状态；取消的或旧轮次结果必须被丢弃。
9. timeout、cancelled、network、gateway、HTTP 和 contract 不得合并成同一个不可诊断错误。
10. 页面只展示真实端点、真实响应元数据和真实浏览器 RTT，不生成假服务或假性能值。
11. Request ID 只用于关联，不用于身份、授权、幂等或宣称完整 trace。
12. 统一 error envelope 不缓存；页面不渲染底层 cause 或 Secret。
13. 本地 Vite proxy 不被文档描述为生产网关。
14. 当前其他业务页面仍是 Mock，不能因状态页联通而改变其完成状态。

### 23.2 信任边界表

| 边界 | 边界外输入 | 进入前检查 | 边界内能力 |
| --- | --- | --- | --- |
| `.env` → Vite config | 任意字符串 upstream | URL、protocol、credentials、path/query/hash、port | Node proxy 建连目标 |
| React caller → HTTP client | path、timeout、signal、decoder | 同源路径、safe integer timeout | 发起一次 GET |
| Network → HTTP client | status、headers、body | gateway 分流、Content-Type、JSON、envelope/ID | 形成安全 error 或 unknown payload |
| unknown JSON → system API | 任意 JS value | object、literal status、version、timestamp | typed probe payload |
| Promise callback → React state | 乱序成功/失败 | generation、controller 未取消 | 更新对应单探针状态 |
| React state → UI | success/error metadata | 聚合真值表、安全文案 | 向用户陈述当前观察 |
| Browser → Go Request ID | 可控 header | 单值、长度、字符集；否则重建 | 日志/响应关联 |
| Go error → HTTP response | 可能含内部 cause | fault 映射、统一安全 message | 稳定 status/code/request ID |
| `/ready` → MySQL | 连接池和 request context | 有界 Ping、错误脱敏 | 只检查连接，不读业务数据 |

## 24. 假设与风险账本

| ID | 当前假设/风险 | 当前证据 | 失效影响 | 观察信号与复核点 |
| --- | --- | --- | --- | --- |
| A15-01 | 两个系统探针足以作为首条真实切片 | 已有稳定后端契约，且包含部分失败 | 学习价值不足或需要新接口 | 第 16/首个业务 API 完成后复盘 |
| A15-02 | 同源路径与未来生产拓扑相容 | 本地代理可保持原路径；生产尚未实现 | 部署需 rewrite/CORS，调用契约改变 | 第 16 节 Compose 与首次预发部署 |
| A15-03 | 5s 浏览器 timeout 对本地合理 | 当前是受控默认，没有真实分位数据 | 慢环境误报 timeout | 预发 RUM/HTTP latency 分布 |
| A15-04 | MySQL Ping 默认 3s 能在 client 前结束 | 配置默认与后端预算约束 | 网关/client 先断开，错误语义变模糊 | 首次完整 timeout 注入测试 |
| A15-05 | 手写 decoder 在当前规模可维护 | 只有两个三字段 payload | 重复/遗漏导致契约缺陷 | 每新增一类响应时审查重复度 |
| A15-06 | 原生 fetch 薄封装足够 | 当前只读 GET、无认证/上传 | adapter 变成自研大框架 | 首批 mutation/分页/文件 API |
| A15-07 | 不轮询不会伤害当前目标 | 页面定位为手动诊断 | 用户误以为状态实时 | UX 反馈、状态陈旧投诉、监控需求 |
| A15-08 | success 缺少 Request ID仍可接受 | payload 事实可独立成立，UI显示缺失 | 排障关联断裂 | 预发 header 丢失率；合规要求 |
| A15-09 | 额外字段兼容策略足够 | additive evolution 原则 | 语义变化未被结构校验发现 | 版本发布与契约测试 |
| A15-10 | `changeOrigin` 不影响当前 Go 行为 | Go handler 当前不依赖 Host 认证 | 未来 Host-based 安全/路由异常 | 引入生产反向代理时 |
| A15-11 | React Strict Mode 清理模型覆盖常见竞态 | hook 有 abort + generation 测试 | 边缘 microtask 顺序造成 stale UI | 浏览器压力测试、错误报告 |
| A15-12 | 非 JSON 502/503/504 足以识别当前代理失败 | client 有显式分支与测试 | 代理用 500/200 HTML 时显示 contract | 第 16 节/生产网关故障注入 |
| A15-13 | Request ID 可从浏览器关联 Go 日志 | header 显示、access log 已记录 | 代理剥离/重建导致断链 | 真实 Vite及预发日志核对 |
| A15-14 | `no-store` 能经过本地代理 | Go设置 header，Vite预期透传 | 缓存重放旧状态/ID | 浏览器 Network 与 CDN 验收 |
| R15-01 | 公网状态页放大 readiness Ping | 当前无轮询但每用户一次 Ping | DB 故障被探针加压 | 生产暴露评审、QPS/连接池指标 |
| R15-02 | 生产 SPA fallback 返回 200 HTML | 当前生产代理未定义 | contract error，大面积状态未知 | 首次部署 path matrix 测试 |
| R15-03 | 多实例让两个探针版本/状态不一致 | 两请求无会话绑定 | 用户困惑或误诊 | 实例 ID/负载均衡日志、发布演练 |
| R15-04 | live region 产生重复播报 | 汇总与卡片均有 live region | 屏幕阅读器体验差 | VoiceOver/NVDA 人工验收 |
| R15-05 | 依赖升级破坏 Node/React 兼容 | lockfile 和 engine 已固定入口 | 安装或运行失败 | Renovate/升级 PR 的 peer check 与全门禁 |

## 25. 反事实推演：如果当时选择另一条路

### 25.1 如果直接硬编码 8080 并开放 `*` CORS

短期会少写代理配置；长期每个环境要知道 API origin，bundle 与部署耦合，Cookie/CSRF 出现时要重做 CORS，Request ID 跨源还需要 expose header。更严重的是，开发端口差异会被错误固化为产品安全策略。

若未来确实分域，重做并不代表当前选择错误；那时有真实身份和部署事实，能做比今天更正确的 CORS 决策。

### 25.2 如果只用 TypeScript `as`

测试和 build 仍可能全部绿色，但生产若返回 `index.html`、`version: null` 或旧状态值，UI 会在运行时使用错误数据。错误可能在格式化时间时才爆炸，离边界更远、更难诊断。runtime decoder 把失败固定在网络入口。

### 25.3 如果用 `Promise.all`

readiness 503 会 reject 整体 Promise，health 的 200 可能被丢失，页面只能显示“失败”。这会把后端精心分开的 liveness/readiness 再次压平成一个布尔值。

### 25.4 如果只调用 abort、不做 generation

正常浏览器 fetch 多数会停止，但一个忽略 signal 的 fake/封装或已经排队的回调仍可能晚到覆盖新轮。取消是资源优化，代际是状态正确性；缺一不可。

### 25.5 如果自动轮询并重试

演示看起来更“实时”，但失败会被后续成功覆盖、页面后台仍请求、用户越多 MySQL Ping 越多、故障时形成重试风暴。没有 freshness 与容量预算时，手动刷新更诚实。

### 25.6 如果一开始就引入全套库

Axios + React Query + Zod + MSW + OpenAPI 可以组成成熟体系，但在两个 GET 上无法区分哪些抽象真有价值，也让学习者把“用了库”误认为“理解了故障”。未来真实复杂度出现后引入，能用迁移前后的缺陷/样板/性能数据证明收益。

## 26. 设计复盘：什么证据会迫使我们承认当前方案错了

设计手记的价值不在于证明当前决定永远正确，而在于预先写下可证伪条件：

- 如果生产必须跨域且同源网关造成不可接受的组织/性能成本，应采用精确 CORS，并保留其他不变量；
- 如果手写 decoder 缺陷率或维护时间显著高，应迁移到 schema/codegen；
- 如果三处以上页面共享相同 server state，并反复实现 cache/dedupe/retry，应引入专用 server-state 库；
- 如果状态页承担真实运维职责，应停止直接让每个浏览器 Ping MySQL，改为受控聚合、历史 metrics 和告警；
- 如果 Vite/生产代理的错误形状无法稳定归类，应定义标准 gateway envelope，而不是持续堆 status 特例；
- 如果 Request ID 无法跨服务排障，应引入 Trace Context/OTel，同时保留安全关联策略；
- 如果 5 秒 timeout 在真实延迟分布下误报，应基于分位数和总体预算调整，不靠感受；
- 如果 live region 真实用户测试产生噪声，应合并播报区域；
- 如果成功响应缺少 Request ID频繁发生，应从“可选显示”升级为部署门禁或契约错误；
- 如果两个探针经常命中不同实例导致误解，应引入实例标识、粘性或服务端聚合。

反过来，没有这些证据时，也不应仅因“某技术更流行”改写边界。

## 27. 未来演进必须回答的问题

### 第 16 节 Compose 前

- Vite/静态资源、Go 与 MySQL 在容器网络中的 host/port 分别是什么？
- 浏览器入口由 Vite dev server、专用 Nginx 还是其他代理承接？
- 容器内需要监听 `0.0.0.0` 时，如何只暴露必要端口？
- Compose healthcheck 使用 `/health` 还是 `/ready`，启动依赖与运行摘流语义如何分开？
- Secret 如何注入而不提交 `.env.local`？
- proxy/upstream timeout 与 Go/MySQL timeout 如何排序？

### 首个业务 API 前

- DTO、runtime validation 与 OpenAPI 的单一事实源在哪里？
- 认证凭证放 Cookie 还是 header，CSRF 模型是什么？
- POST 的 body、Content-Type、幂等、错误和取消语义怎样扩展 `httpClient`？
- 哪些 server state 需要 cache，freshness 和 invalidation 依据是什么？
- 业务错误哪些可直接展示，哪些必须由前端映射？

### 生产部署前

- `/health`、`/ready` 的受众、网络和 rate limit 是什么？
- 正式代理如何处理 SPA fallback、error page、缓存和 `X-Request-ID`？
- 是否采用 CORS；若采用，origin/credentials/CSRF/expose header如何组合？
- 如何验证滚动发布中的 probe/API 契约兼容？
- 是否需要实例 ID，怎样避免泄漏不必要拓扑？

### 可观测性章节前

- Request ID 与 `traceparent` 谁生成、谁信任、如何同时记录？
- 浏览器 RTT、Server-Timing、Go span、MySQL span各自回答什么？
- trace/log/metric 的采样、基数、PII 和保留成本如何控制？
- 状态页是人工诊断入口还是产品 SLA 页面；若后者，历史与告警事实源是什么？

## 28. 可追溯证据

### 决策、课程与验收

- [第 15 节课程正文](../../course/part-02/lesson-15-first-fullstack-integration.md)
- [ADR-0011：前端同源代理与系统探针集成边界](../../decisions/ADR-0011-same-origin-frontend-integration.md)
- [第 15 节 API 记录](../../api/lessons/lesson-15.md)
- [第 15 节 QA 验收证据](../../qa/lessons/lesson-15.md)
- [第 13 节 MySQL 与探针设计手记](lesson-13.md)
- [配置参考](../../configuration.md)

### 前端运行时代码

- [Vite dev/preview 同源代理、环境变量与端口边界](../../../web/vite.config.ts)
- [可提交的非秘密代理配置示例](../../../web/.env.example)
- [统一 fetch、timeout、取消、错误分类与 Request ID](../../../web/src/api/httpClient.ts)
- [系统探针路径与运行时 decoder](../../../web/src/api/systemApi.ts)
- [两探针独立状态、代际与 effect 清理](../../../web/src/pages/system/status/useSystemStatus.ts)
- [状态聚合、真实证据展示与可访问性](../../../web/src/pages/system/status/SystemStatusPage.tsx)
- [状态页路由](../../../web/src/routes/appRouter.tsx)
- [用户布局中的状态入口](../../../web/src/layouts/UserLayout.tsx)
- [前端脚本、依赖、engine 与 package manager](../../../web/package.json)
- [全仓前端质量门禁](../../../Makefile)

### Go 运行时代码

- [系统探针路由与 Router 选项](../../../internal/infrastructure/httpapi/router.go)
- [无依赖 health handler](../../../internal/infrastructure/httpapi/health.go)
- [有界 MySQL readiness handler](../../../internal/infrastructure/httpapi/readiness.go)
- [统一安全错误与 no-store](../../../internal/infrastructure/httpapi/errors.go)
- [Request ID 校验、生成与 context 传播](../../../internal/infrastructure/httpapi/request_id.go)
- [访问日志与 panic recovery](../../../internal/infrastructure/httpapi/middleware.go)
- [API 把 MySQL Ping timeout 装配到 readiness](../../../cmd/growth-api/main.go)
- [跨组件 timeout 配置校验](../../../internal/platform/appconfig/config.go)

### 证明性质的测试

- [HTTP client 成功、HTTP/gateway/network/timeout/cancel/contract 测试](../../../web/src/api/httpClient.test.ts)
- [系统路径与 runtime decoder 测试](../../../web/src/api/systemApi.test.ts)
- [并行、旧代际、卸载与 Strict Mode 测试](../../../web/src/pages/system/status/useSystemStatus.test.tsx)
- [真实文案、降级、未知状态与刷新组件测试](../../../web/src/pages/system/status/SystemStatusPage.test.tsx)
- [统一错误 envelope/no-store 测试](../../../internal/infrastructure/httpapi/errors_test.go)
- [health 成功契约测试](../../../internal/infrastructure/httpapi/health_test.go)
- [readiness 成功、503、timeout 与脱敏测试](../../../internal/infrastructure/httpapi/readiness_test.go)
- [Request ID 信任边界测试](../../../internal/infrastructure/httpapi/request_id_test.go)

## 29. 本节设计结论

第 15 节交付的核心不是“React 能显示 Go 返回的一段 JSON”，而是第一条可以诚实解释的全栈链路：浏览器只表达同源路径，开发代理只适配本地拓扑，HTTP client 区分 transport 与契约故障，runtime decoder 不让 TypeScript 假装验证了网络，hook 保留两个故障域并阻止旧代际写入，UI 只陈述本轮真实可观察事实，Request ID 提供有限但实用的关联，而后端错误不缓存也不泄漏内部 cause。

选择 `/health` 与 `/ready` 而不是业务 CRUD，是为了在没有业务事实时先把证据、失败和生命周期做对；不引入 Axios、Zod、React Query、MSW、OpenAPI、自动轮询和聚合端点，是因为当前尚无足够重复或风险证明这些成本。每个推迟项都有重新决策触发器，因此“小”不是封闭，而是给未来复杂度留下有证据的入口。

最重要的不变量是认知上的：一次 200 不等于系统正常，一次 Ping 不等于业务可用，一段 TypeScript 类型不等于运行时契约，一个 Request ID 不等于分布式 Trace，本地 Vite 代理也不等于生产网关。只要后续章节继续守住这些证据边界，GrowthOS 才会从“页面看起来完整”逐步演进为“每一层都知道自己能证明什么、不能证明什么”的真实系统。
