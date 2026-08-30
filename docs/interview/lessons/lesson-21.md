# 第 21 节面试题：受限 Lottery Ephemeral Selection API

本文只描述第 21 节已经实现并可由代码、测试与隔离 Compose 验收复核的时间切片：`POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections` 在显式打开 feature flag 的 development/test 环境中，读取 MySQL 的一个合法 Strategy 快照，用第 20 节的密码学随机加权选择器返回一个配置内 Award。它是同步、没有 Lottery 业务状态写路径的 **ephemeral selection**，不是正式 Draw；E2E 证明的是两张 Lottery 业务表 fingerprint 不变，访问日志与运行指标等技术副作用仍会存在。

当前仍然没有用户/租户认证、抽奖次数、活动资格、库存预占、权益发放、Draw/Result 身份、结果持久化、幂等重放、审计账本、限流、正式前端接入或生产开放。因此本文的回答会刻意区分“代码可证明的事实”“协议或标准的技术事实”和“未来正式抽奖应做的设计”，避免把学习用纵向切片包装成生产抽奖平台。

## 来源与使用说明

- `项目事实` 来自当前仓库的实现、测试、QA 和一次性 Compose 验收；数字只按各自测试层解释，不能跨层外推。
- `官方技术事实` 优先引用 RFC、Go、Nginx、MySQL、Docker 与 OWASP 官方资料。面经帖子中的答案不作为技术结论。
- `面经题型启发` 来自牛客用户发布的候选人自述或题目整理，只能说明该类追问真实出现在公开面试复盘中；本文未核验公司、岗位、轮次或逐字题干，不能称为公司官方原题。部分页面可能要求登录或受反爬限制。
- 本文在 2026-08-30 复核链接。高频方向包括：HTTP 方法与状态码、Gin 路由/中间件、`context`、高并发保护、数据库事务、幂等与项目深挖。代表性页面为[Go 后端社招面经实录](https://www.nowcoder.com/discuss/603841169302245376)、[Go 后端日常实习面经](https://www.nowcoder.com/discuss/595762627133882368)、[沥泉科技 Golang 后端一面面经](https://www.nowcoder.com/discuss/703571718387847168)、[API 测试与幂等题型整理](https://www.nowcoder.com/discuss/427423513633460224)。

## 1. 第 21 节到底实现了什么，最容易夸大的地方是什么？

- **面试官意图：** 判断候选人能否准确界定自己做过的工作，而不是把“接口能返回中奖结果”直接等同于完整抽奖系统。
- **30 秒回答：** 我实现的是一个仅限 development/test、默认关闭的临时选择 API。它把 Nginx、Gin、application use case、MySQL 一致快照和密码学随机加权选择器串成真实纵向链路；成功只表示本次请求读到合法 Strategy 并在内存中选出配置内 Award，没有创建 Draw、保存结果或发奖。
- **深入回答：** 正式抽奖至少要回答四类事实：谁有资格、哪一次 Draw、使用哪个配置版本、最终结果与发放状态是什么。本节只有 StrategyID 和一次同步计算，没有用户/租户/活动 identity，也没有可查询的结果 identity。请求或响应丢失后，客户端无法确认先前结果；再次 POST 会重新随机选择。因此简历可以写“实现受限 Lottery Selection 纵向切片与严格边界”，不能写“实现 exactly-once、高并发可幂等发奖系统”。
- **项目证据：** [application use case](../../../internal/lottery/application/ephemeral_selection.go)、[HTTP adapter](../../../internal/lottery/adapter/httpapi/selection.go)、[第 21 节 QA](../../qa/lessons/lesson-21.md)、[ADR-0018](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)。
- **追问陷阱：** 要区分两种“幂等”：RFC 9110 看重复请求对服务端产生的预期效果，不要求响应相同；本节没有业务状态写入，因此不能仅凭 Award 不同断言它在 RFC 窄义上 unsafe/non-idempotent。但它没有“相同请求身份复用同一 Award”的业务重放保证，这正是正式 Draw 所缺的能力。
- **技术取舍：** 先交付诚实命名、硬 gate 的只读纵向切片，可以提前验证边界组合；代价是它不能直接升级为公开生产抽奖，正式 Draw 仍需独立建模。
- **来源：** `项目事实` 上述代码与 ADR；`面经题型启发` [牛客面经中对“项目介绍、项目难点、重复调用如何幂等”的追问](https://www.nowcoder.com/discuss/603841169302245376)。

## 2. 为什么用 POST，而不是 GET 或 PUT？

- **面试官意图：** 考察 HTTP 方法语义，而不是只会按“有无请求体”选方法。
- **30 秒回答：** 我把它建模为调用方明确触发的一次 selection action，所以用 POST，避免 GET 链接、预取、爬虫或通用缓存误触，也与未来会持久化的 Draw 演进方向一致。它不在客户端指定 URI 上创建或替换资源，所以不用 PUT。
- **深入回答：** 当前实现不写业务状态，因此不能因为消耗熵或两次响应不同，就断言它在 RFC 9110 的“预期服务端状态效果”意义上 unsafe/non-idempotent；HTTP 幂等也不要求响应相同。但产品不希望 selection 被当作普通表示读取：GET 容易被预取、爬虫、复制链接或缓存基础设施无意触发，而未来正式 Draw 会产生耐久业务效果。PUT 的语义是创建或替换目标资源，通常需要客户端知道稳定资源 URI，本节没有 DrawID。POST 表达显式 action；本接口禁止 `Idempotency-Key`，是因为没有持久记录可以提供“同 key 同 Award”的更强业务重放承诺。
- **项目证据：** [`SelectionPath` 与 POST 注册](../../../internal/lottery/adapter/httpapi/selection.go)、[方法契约测试](../../../internal/lottery/adapter/httpapi/selection_test.go)。
- **追问陷阱：** 不要回答“POST 因为参数多/GET 因为参数少”；方法由语义决定。也不要说“POST 一定不幂等”或“响应不同所以非幂等”：POST 没有被标准赋予 idempotent 属性，但具体操作仍要按预期服务端状态效果分析；同结果重放是另一项应用协议。
- **技术取舍：** 若未来正式 Draw 支持 `Idempotency-Key`，仍可使用 POST，但服务端必须将 key、调用主体、请求摘要和已持久结果绑定；若客户端预先分配 DrawID，也可评估 PUT，但冲突与权限模型会更复杂。
- **来源：** `官方技术事实` [RFC 9110：safe/idempotent 方法](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2)、[POST](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.3.3)与[PUT](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.3.4)；`面经题型启发` [牛客 HTTP 方法与状态码题型](https://www.nowcoder.com/discuss/353159351718322176)。

## 3. 为什么成功返回 200，而不是 201、202、204？`no_reward` 为什么也是 200？

- **面试官意图：** 判断候选人能否把 transport 状态与业务 outcome 分开。
- **30 秒回答：** 选择在请求内同步完成并返回表示，所以是 200。没有创建可寻址资源，不能用 201；没有排队等待后续完成，不能用 202；响应需要返回 Award，不能用 204。`no_reward` 是算法合法选中的业务结果，不是系统错误，所以同样 200。
- **深入回答：** 201 通常意味着已创建资源，并应通过目标 URI 或 `Location` 表达资源位置；本节没有 DrawID，也明确断言没有 `Location`。202 只表示已接受但尚未完成，需要后续状态查询协议，本节不存在。204 不允许响应内容，会丢失 `durability`、StrategyID 与 Award。`reward`/`no_reward` 是 `award.outcome` 的封闭枚举；随机源失败、数据库失败、deadline 才走 5xx，绝不能降级伪装成 `no_reward`。
- **项目证据：** [DTO 映射与 200](../../../internal/lottery/adapter/httpapi/selection.go)、[`no_reward` 成功测试](../../../internal/lottery/adapter/httpapi/selection_test.go)、[Compose success fixtures](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** “未中奖”不是 404；404 表示目标资源/Strategy 不存在。也不能用 204 后再靠 header 偷渡业务结果。
- **技术取舍：** 统一 200 能让客户端把 transport success 与 outcome 显式分层；若未来发奖是异步工作流，可以让 Draw 创建返回 201，再让发放状态独立演进，而不是把两者压进一个状态码。
- **来源：** `官方技术事实` [RFC 9110：200](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.3.1)、[201](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.3.2)、[202](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.3.3)、[204](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.3.5)；`项目事实` 上述测试。

## 4. 路径为什么是 `/api/v1/lottery/strategies/:id/ephemeral-selections`？

- **面试官意图：** 考察资源建模、版本策略和命名是否诚实。
- **30 秒回答：** `/api/v1` 表示业务 HTTP contract 的 URI 版本；`strategies/:id` 表示选择依赖哪个配置聚合；`ephemeral-selections` 明确返回的是不可持久查询的临时选择，而不是 Draw。复数名词也为后续真实资源模型保留空间。
- **深入回答：** 路径没有写 `/draw`，因为当前没有 Draw identity 与最终事实；也没有写 `:select` RPC 风格动作，因为“selection”本身可作为从属概念建模。`v1` 只表示契约版本，不代表生产成熟度或公网开放等级。尾斜杠不重定向：对 POST 做 307/308 可能触发客户端重放，所以精确路径以 404 拒绝。
- **项目证据：** [路径常量与注释](../../../internal/lottery/adapter/httpapi/selection.go)、[router 禁止尾斜杠重定向](../../../internal/infrastructure/httpapi/router.go)、[路径验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** 不能因为有 `/v1` 就承诺永不变；兼容性仍取决于字段、状态码、错误码和语义。真正缺少整个 ID segment 的路径不会进入 handler，会是 404；但经真实 Nginx 保留 `$request_uri` 转发的双斜杠空参数可进入 Gin handler，返回 400 `invalid_strategy_id`。二者不能混写。
- **技术取舍：** 路径略长，但用名称承担了重要风险沟通。正式 Draw 可另开 `/draws`，避免悄悄改变现有 ephemeral endpoint 的耐久性和重试语义。
- **来源：** `项目事实` [第 21 节 API 记录](../../api/lessons/lesson-21.md)；`官方技术事实` [RFC 9110 的资源与 URI 语义](https://www.rfc-editor.org/rfc/rfc9110.html#section-4)；`面经题型启发` [Gin 路由与项目深挖面经](https://www.nowcoder.com/discuss/595762627133882368)。

## 5. 为什么 StrategyID/AwardID 用规范十进制字符串，而不是 JSON number？

- **面试官意图：** 考察跨语言精度、输入规范化和身份语义。
- **30 秒回答：** 领域 ID 是完整 `uint64`，JavaScript `number` 只能安全表示到 `2^53-1`。Path 只接受 1 到 MaxUint64 的无符号规范十进制，响应继续用十进制 string，从而让最大 ID 无损往返，并禁止 `01`、`+1`、空白或指数形式产生多种表示。
- **深入回答：** adapter 先限制最长 20 字节，再 `strconv.ParseUint(raw, 10, 64)` 做范围检查，并以 `FormatUint(parsed, 10) == raw` 做 round-trip 规范化。这样缓存键、日志、签名、幂等摘要和数据库 identity 不会因多个等价值产生歧义。DTO 不把 ID 转为 JSON number，也不依赖前端 BigInt 的序列化差异。最大 StrategyID/AwardID 的真实 Compose fixture 验证类型和值都是 string。
- **项目证据：** [`parseStrategyID` 与 `mapSelection`](../../../internal/lottery/adapter/httpapi/selection.go)、[MaxUint64 HTTP 测试](../../../internal/lottery/adapter/httpapi/selection_test.go)、[Compose MaxUint64 验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** `ParseUint` 本身会接受某些可解析形式；必须结合 base 10 和格式 round-trip。数据库用 `BIGINT UNSIGNED` 并不自动解决 JSON/JS 边界精度。
- **技术取舍：** string 牺牲了少量静态“数字感”和客户端转换便利，换取完整 identity 空间与跨语言一致性。若业务真正只需要小 ID，也应通过显式迁移缩窄契约，而不是在 adapter 静默截断。
- **来源：** `官方技术事实` [`strconv.ParseUint`](https://pkg.go.dev/strconv#ParseUint)、[MDN `Number.MAX_SAFE_INTEGER`](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Number/MAX_SAFE_INTEGER)、[RFC 8259 的数字互操作提示](https://www.rfc-editor.org/rfc/rfc8259.html#section-6)；`项目事实` 上述边界测试。

## 6. `X-GrowthOS-Demo-Mode` 是认证、CSRF 防护还是 feature flag？

- **面试官意图：** 判断候选人是否会把一个自定义 header 夸大成完整安全机制。
- **30 秒回答：** 它是调用方对“本次结果不持久”的显式确认，并在本地浏览器场景中提高跨站简单请求门槛；它不是身份认证、租户授权或生产 feature flag。服务端 feature flag 是另一层：默认不注册路由，staging/production 打开会启动失败。
- **深入回答：** header 必须恰好出现一次且值精确为 `ephemeral-selection`，缺失、错误或重复都在进入 use case 前 400。自定义 header 不属于 CORS safelist，普通跨站 HTML form 不能直接设置它，但恶意脚本、非浏览器客户端或已获 CORS 权限的 origin 仍可发送，所以它至多增加本地 demo 的跨站调用摩擦，不能命名成完整 CSRF guard。真正授权必须把用户、租户、活动与 Strategy 的访问关系建模。
- **项目证据：** [header 校验](../../../internal/lottery/adapter/httpapi/selection.go)、[feature flag 与生产硬拒绝配置](../../../internal/platform/appconfig/config.go)、[重复 header 验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** “浏览器默认发不了”不等于“攻击者发不了”；curl、服务端请求和扩展都能设置。feature flag 控制发布面，也不是逐请求权限控制。
- **技术取舍：** 当前采用“默认关闭 + 环境硬拒绝 + 调用方确认”三层误用防线，适合学习接口；正式开放应新增认证、对象级授权、CSRF/CORS 策略和限流，而不是继续堆魔法 header。
- **来源：** `官方技术事实` [Fetch 规范的 CORS-safelisted request-header](https://fetch.spec.whatwg.org/#cors-safelisted-request-header)、[OWASP REST Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html)；`面经题型启发` [Gin middleware 与鉴权追问题型](https://www.nowcoder.com/discuss/595762627133882368)。

## 7. “接口没有 body”具体怎样判断？为什么 `Content-Length: 0` 仍会通过？

- **面试官意图：** 考察 HTTP message framing 与框架 binding 的真实边界。
- **30 秒回答：** handler 不读取 body，而是按 Go 已解析的 framing 拒绝 `ContentLength != 0`、任何 `TransferEncoding` 或可观察到的 `Request.Trailer`。显式 `Content-Length: 0` 与完全没有 body 在 Go server request 中都表现为长度 0，无法可靠区分，所以两者都允许。
- **深入回答：** 若为了“确认空”去读未知长度或 chunked body，慢客户端时间会落在 selection deadline 之外，并占用 handler。当前 contract 因此只接受零长度、无 TE、无可观察 trailer 的 framing；已知非零 `{}`、未知长度 `-1`、空 chunked 都拒绝。`curl --data ''` 可能只产生 `Content-Length: 0`，因此命令行上写了 `--data` 不等于服务器观察到 body。此 endpoint 无 schema，不需要 `ShouldBindJSON`；把空 body 送 binding 只会制造无意义 EOF 分支。
- **项目证据：** [`emptyRequestBody`](../../../internal/lottery/adapter/httpapi/selection.go)、[known nonzero/chunked/trailer 单测](../../../internal/lottery/adapter/httpapi/selection_test.go)、[真实 body/framing 验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** 不要声称“测试拒绝了 `Content-Length: 0`”或“能识别 curl 是否调用 `--data`”；wire 语义没有这项信息。也不要把 16 KiB gateway limit 解释成允许 16 KiB 请求体。
- **技术取舍：** framing-only 判断节省读取和绑定成本，适合严格无 body route；未来有 JSON schema 的 endpoint 应采用限定大小、严格 decoder、unknown-field 策略和 content-type 校验，而不是复制本规则。
- **来源：** `官方技术事实` [Go `http.Request` 的 Body/ContentLength/Trailer 语义](https://pkg.go.dev/net/http#Request)、[RFC 9112 message body length](https://www.rfc-editor.org/rfc/rfc9112.html#section-6.3)；`面经题型启发` [Gin `ShouldBind` 与 HTTP timeout 题型](https://www.nowcoder.com/discuss/703571718387847168)。

## 8. Transfer-Encoding 和 Trailer 为什么要在 Nginx 与 Go 两层防守？所有异常都能统一 JSON 400 吗？

- **面试官意图：** 考察代理正规化、HTTP 解析时序与“统一错误格式”的边界意识。
- **30 秒回答：** Nginx 可能先解码 chunked，导致空 chunked 到 Go 时像普通空请求；也可能让 `Trailer` declaration 在代理后消失，所以边缘用 `$http_transfer_encoding` 和 `$http_trailer` 拒绝仍可观察的非空声明。Go 仍拒绝自身看到的 `TransferEncoding` 和 `Request.Trailer`。但不能承诺所有非法 framing 都是 JSON 400：unsupported/invalid Transfer-Encoding 可能在 Nginx parser 进入 API location 前直接 501/HTML。
- **深入回答：** 验收证明的是受 Nginx 接受并进入 `/api` location 的 `Transfer-Encoding: chunked`（包括空 chunked）被内部 460 映射成 JSON 400；非空 `Trailer: X-Lottery-Ticket` 声明也由 `$http_trailer` 映射拒绝。`$http_trailer` 只观察非空 header 值，不等于能识别所有畸形 trailer；Go 的 `Request.Trailer` 检查仍是独立防线。像 `Transfer-Encoding: identity`、`gzip` 或无效组合可能被 Nginx HTTP parser 早期以 501 或其他原生响应拒绝，无法经过 location 的 error template。
- **项目证据：** [Nginx maps 与 API location](../../../deploy/docker/nginx.conf)、[Go framing gate](../../../internal/lottery/adapter/httpapi/selection.go)、[chunked 与 Trailer Compose 验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** “任意 Transfer-Encoding 都返回 JSON 400”是过度承诺；“Go 检查 Trailer 就足够”也不对，因为代理可能消去原始声明。parser-level 响应仍需依靠外层网关/客户端按状态处理，不能按 JSON code 分支。
- **技术取舍：** 双层防线会复制少量策略，但保留了每层仍可观察的信息。若未来 API 支持 streaming/chunked，需要按 route 重构，而不能继续对整个 `/api` 一刀切拒绝。
- **来源：** `官方技术事实` [RFC 9112 Transfer-Encoding 与 message framing](https://www.rfc-editor.org/rfc/rfc9112.html#section-6.1)、[Go Request Trailer](https://pkg.go.dev/net/http#Request)、[Nginx request processing](https://nginx.org/en/docs/http/request_processing.html)；`项目事实` 上述真实代理验收。

## 9. 为什么 query、seed、ticket 和 `Idempotency-Key` 都被拒绝？

- **面试官意图：** 判断候选人是否理解“未声明输入”会扩大攻击面和虚假协议承诺。
- **30 秒回答：** 本接口唯一业务输入是 path StrategyID；query 会让调用者尝试控制 seed、ticket、候选或隐藏参数，所以任何 query（包括裸 `?`）都拒绝。`Idempotency-Key` 也拒绝，因为服务端没有 durable result 可以复用，接受它会让客户端误以为重试得到同一结果。
- **深入回答：** handler 同时检查 `RawQuery != ""` 和 `ForceQuery`，避免裸问号绕过。随机材料由服务端 CryptoSource 产生，DTO 也不暴露 ticket。幂等协议不仅是“缓存一个响应”：至少需要调用主体、key、请求规范摘要、结果/处理中状态、保留期、并发唯一约束和响应未知后的查询。当前任何重试都是新 selection，故返回 `idempotency_not_supported` 比静默忽略 header 更诚实。
- **项目证据：** [`requestHasQuery` 与幂等 header 检查](../../../internal/lottery/adapter/httpapi/selection.go)、[query/idempotency tests](../../../internal/lottery/adapter/httpapi/selection_test.go)、[Compose contract 验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** 当前代码和单测已覆盖 `ForceQuery` 裸 `?`，真实 Compose 验收覆盖 `?seed=1`；要准确区分证据层。Request ID 也不能代替 Idempotency-Key，它只做相关性。
- **技术取舍：** 严格拒绝牺牲向后“宽容”，换来更小的契约面与可审计随机边界。未来新增参数必须进入 schema、威胁模型和版本策略，不应先容忍后解释。
- **来源：** `官方技术事实` [OWASP Input Validation Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)、[RFC 9110 的 URI query 组成](https://www.rfc-editor.org/rfc/rfc9110.html#section-4.2.1)；`面经题型启发` [API 测试、幂等与状态码题型](https://www.nowcoder.com/discuss/427423513633460224)。

## 10. 400、404、409、422 在这个接口里怎么区分？

- **面试官意图：** 考察错误分类能否反映“哪一层事实失败”。
- **30 秒回答：** 已匹配 route 内的非法 ID、header、query、body/framing 和虚假幂等声明是 400；合法 StrategyID 查不到聚合是业务 404；未注册路径或尾斜杠是 route 404。当前没有资源状态冲突或语义正确的请求内容校验，所以不用 409/422。
- **深入回答：** `invalid_strategy_id` 只发生在 handler 已取得 path 参数后；缺失 path segment 根本不匹配 route，是全局 `route_not_found`。409 适合当前状态与请求冲突，例如未来相同 idempotency key 对应不同请求摘要；422 可用于 media type 和语法都正确但内容违反业务规则的复杂命令。当前 body 被整体禁止，path 的非规范表示属于 request syntax/contract 问题，所以统一 400。多个 handler 错误同时存在时，公开顺序为 ID → idempotency → demo header → query → framing；Nginx 拒绝 Host/TE/oversize 时会更早结束，不受该顺序约束。
- **项目证据：** [handler 校验顺序](../../../internal/lottery/adapter/httpapi/selection.go)、[全局 NoRoute/NoMethod](../../../internal/infrastructure/httpapi/router.go)、[错误矩阵](../../api/lessons/lesson-21.md)。
- **追问陷阱：** 不要把“数据库中没有 Strategy”说成 400，也不要把“表里有 root 但快照非法”说成 404；后者是内部数据不变量破坏，当前映射 500。
- **技术取舍：** 少量稳定 code 比为每个输入形态创建状态码更利于客户端；但状态与 code 必须分层，客户端按 `error.code` 分支而不是匹配 message。
- **来源：** `官方技术事实` [RFC 9110：400](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.1)、[404](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.5)、[409](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.10)、[422](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.21)；`面经题型启发` [HTTP 状态码面经整理](https://www.nowcoder.com/discuss/603841169302245376)。

## 11. 为什么不支持的方法返回 405 和 `Allow: POST`？HEAD 有什么特殊之处？

- **面试官意图：** 考察路由存在性、方法协商和 HEAD 的 wire 语义。
- **30 秒回答：** 精确路径存在但方法不支持，所以是 405，不是 404；`Allow: POST` 告知资源支持的方法。Gin 单测覆盖 GET/PUT/PATCH/DELETE/HEAD/OPTIONS 的 405 envelope，但真实 `net/http` wire 对 HEAD 会保留 405 与 headers、抑制响应 body，因此不能承诺 HEAD 客户端收到 JSON body。
- **深入回答：** router 开启 `HandleMethodNotAllowed` 并用统一 NoMethod fault；middleware 写入 Request ID 和 no-store。`httptest.ResponseRecorder` 能观察 handler 尝试写出的 JSON，适合校验 adapter 分类，但 HTTP server 在真实 HEAD 响应中不发送 message content。这形成一个重要证据边界：HEAD 的真实契约是状态码、`Allow`、Request ID、cache/security headers，不是可解析 error envelope。尾斜杠则是未注册路径 404，且不做 POST redirect。
- **项目证据：** [router method handling](../../../internal/infrastructure/httpapi/router.go)、[六种方法与 `Allow` 单测](../../../internal/lottery/adapter/httpapi/selection_test.go)、[API 记录的 HEAD 边界](../../api/lessons/lesson-21.md)。
- **追问陷阱：** 不要说“所有错误一定有 JSON body”；HEAD、非法 Host 和 Nginx parser-level 错误都是明确例外。也不要为了隐藏接口存在性把所有方法都返回 404，除非威胁模型明确要求且文档一致。
- **技术取舍：** 405 + Allow 更符合可诊断性；对公网敏感对象，认证层可在授权前统一 404 防枚举，但这是另一项安全策略，不应和当前本地 demo 混淆。
- **来源：** `官方技术事实` [RFC 9110：405](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.6)、[`Allow`](https://www.rfc-editor.org/rfc/rfc9110.html#section-10.2.1)与[HEAD](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.3.2)；`项目事实` 上述测试。

## 12. Nginx 为什么校验 Host？非法 Host 的 421 是什么格式？

- **面试官意图：** 考察 authority/Host 路由、DNS rebinding 风险和网关错误边界。
- **30 秒回答：** Compose edge 只服务 loopback，本地只允许 `localhost`、`127.0.0.1` 和 `[::1]` 的可选端口。其他 Host 在 server level 直接 421，避免任意 Host 被代理到 API。这个 421 有 `Cache-Control: no-store`、Request ID 和安全 headers，但当前是 Nginx 原生非 JSON body，没有稳定业务 `error.code`。
- **深入回答：** Host/`:authority` 决定请求要访问哪个 origin；若本地服务接受任意 Host，浏览器可能经攻击者控制的 DNS 名称访问 loopback 服务。Nginx map 在代理前做 allowlist，因而 Go handler、错误 envelope 和应用日志都不会运行。421 表示服务器不愿为该目标 origin 生成响应；本配置没有将它映射到 named JSON location，所以不能向客户端承诺 `misdirected_request` code。验收只断言 421、单一 no-store 和非空单一 Request ID。
- **项目证据：** [Host allowlist 与 server-level 421](../../../deploy/docker/nginx.conf)、[非法 Host 验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** 不要说“421 一定是 JSON”，也不要说“有 Request ID 就进入了 Go”。它由 Nginx 生成，只有边缘相关 ID。端口正则只限制 1～5 位字符，不等于验证端口数值小于等于 65535；真正连接目的仍是已建立的 loopback listener。
- **技术取舍：** 本地 allowlist 比 `server_name _` 更小暴露面；正式多域名部署应由明确的 TLS/SNI、authority、trusted proxy 与 ingress 配置接管，不能照搬 loopback 列表。
- **来源：** `官方技术事实` [RFC 9110 Host/:authority](https://www.rfc-editor.org/rfc/rfc9110.html#section-7.2)与[421](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.20)、[OWASP DNS rebinding 防护说明](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html#dns-pinning)；`项目事实` 上述配置与验收。

## 13. 为什么 success DTO 只返回 durability、两个 ID、Award name/outcome，不返回 weight、ticket 或 Strategy name？

- **面试官意图：** 考察 domain model 与 public DTO 的隔离，以及最小数据暴露原则。
- **30 秒回答：** API 只暴露客户端完成当前展示所需的稳定事实。`durability: ephemeral` 防止误解，string ID 保真，Award name/outcome 支持展示；weight、total、ticket、随机源、Strategy name 和内部错误既不是当前客户端必要字段，也可能泄露运营配置或让调用方误推发布概率，所以不返回。
- **深入回答：** HTTP adapter 显式映射 DTO，没有直接 JSON 序列化 domain。这样领域新增字段不会自动扩张 API，也能避免把 `Weight` 当成绝对概率：真正概率取决于同版本候选集和总权重。ticket 单独没有 Strategy 版本/候选快照/算法版本就不能构成可重放审计证据，暴露反而制造“可验证公平”的错觉。响应也没有 `draw_id`、`location` 或发放状态，因为这些事实尚不存在。
- **项目证据：** [`selectionResponse` 与 `mapSelection`](../../../internal/lottery/adapter/httpapi/selection.go)、[禁止字段测试](../../../internal/lottery/adapter/httpapi/selection_test.go)、[领域对象](../../../internal/lottery/domain/strategy.go)。
- **追问陷阱：** “隐藏 weight”不是安全控制的替代品；有足够样本仍可估计分布。若法规或产品要求概率披露，必须发布带版本和舍入规则的明确读模型，不能直接泄露内部字段应付。
- **技术取舍：** 手写 DTO 有维护成本，但换来兼容性和信息最小化。未来前端需要更多数据时，应新增明确字段/endpoint，而不是把整个 Strategy 聚合序列化出去。
- **来源：** `官方技术事实` [OWASP REST Security：响应最小化与敏感信息](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html#sensitive-information-in-http-requests)、[RFC 8259 JSON](https://www.rfc-editor.org/rfc/rfc8259.html)；`面经题型启发` [provider/schema adapter 抽象的真实项目追问](https://www.nowcoder.com/discuss/603841169302245376)。

## 14. 自定义 error envelope 和 RFC 9457 Problem Details 怎么取舍？为什么不返回内部 cause？

- **面试官意图：** 考察错误协议稳定性、可观测性与信息泄露。
- **30 秒回答：** 项目沿用统一 `{error:{code,message,request_id}}`：code 供程序分支，message 是安全公开文本，request_id 用于关联；原始 driver、SQL、随机源和包内错误不出 API。RFC 9457 更标准、生态更好，但在已有稳定 envelope 的学习项目里迁移收益不足，未来可版本化评估。
- **深入回答：** `fault.Error` 把 kind、稳定 code、public message 与 cause 分离；未知错误一律收敛为 500 `internal_error`。selection failure 日志只写经过 allowlist 的 `error_class`、canonical StrategyID 和 Request ID，不直接格式化 cause、query、Authorization 或 URL query，避免秘密和高基数字段扩散。Nginx 的 400/413/502/504 手写同形 JSON，但这造成模板漂移风险；而 421、HEAD、parser-level 错误明确不保证 envelope。
- **项目证据：** [统一错误 writer](../../../internal/infrastructure/httpapi/errors.go)、[selection 错误映射/日志](../../../internal/lottery/adapter/httpapi/selection.go)、[Nginx named error locations](../../../deploy/docker/nginx.conf)。
- **追问陷阱：** 不要说 request_id 让错误“安全”或能替代 trace；它只是关联键。也不要让客户端按 message 文本分支，message 可因文案/本地化改变。
- **技术取舍：** 自定义 envelope 简单且与现有模块一致；RFC 9457 提供 `type/title/status/detail/instance` 的通用生态。迁移时应避免同时保留两套互相冲突的 code，并处理网关与 HEAD 的例外。
- **来源：** `官方技术事实` [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html)、[OWASP Error Handling Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html)；`面经题型启发` [API 测试与错误状态追问](https://www.nowcoder.com/discuss/427423513633460224)。

## 15. 500、503、502、504 怎么区分？客户端能不能自动重试？

- **面试官意图：** 考察故障归因、恢复语义和“重试不等于同结果重放”的风险。
- **30 秒回答：** 503 是应用判断当前选择暂不可用，如 cancel/deadline、可重试型数据库故障或随机源失败；500 是存储快照非法、普通 repository failure 或组件契约/不变量破坏；502/504 是 Nginx 没拿到可信 upstream response。任何一种都没有形成可返回的 selection，但当前重试会产生一次新选择，客户端不能当作“恢复同一结果”。
- **深入回答：** 404 Strategy not found 与 5xx 分开。MySQL 1205/1213、`driver.ErrBadConn`、`net.Error` 被 repository 分类为 retryable，HTTP 映射 503；本节不返回 `Retry-After`，因为服务端没有可靠的恢复时刻或退避建议。即使未来返回，`Retry-After` 也只建议何时再次请求，不承诺复用同一 Award。Nginx 停止 API 的真实验收允许 502 或 504，因为 Docker endpoint/DNS/已有连接状态会影响观察结果；两者都带 JSON/no-store/Request ID（前提是请求进入 API location）。500 被有意收敛，避免公开 stored schema 或 selector contract。
- **项目证据：** [`publicSelectionFault`](../../../internal/lottery/adapter/httpapi/selection.go)、[repository 错误分类](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[gateway failure 验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** “503 就可以放心重试”是错的；是否重试取决于操作语义、预算和幂等协议。当前 response 丢失后，服务端也没有先前结果可查。
- **技术取舍：** 对外用低基数分类保护实现细节，对内保留受控诊断；代价是运维必须依靠关联日志/指标定位。正式 Draw 应先建立结果 identity 和查询，再设计有限重试与 backoff。
- **来源：** `官方技术事实` [RFC 9110：500/502/503/504](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.6)、[`Retry-After`](https://www.rfc-editor.org/rfc/rfc9110.html#section-10.2.3)；`面经题型启发` [节点不可达、重复调用与错误分类面经](https://www.nowcoder.com/discuss/603841169302245376)。

## 16. Request ID、trace ID、Idempotency-Key 和 DrawID 有什么区别？

- **面试官意图：** 检查候选人是否混淆相关性、重放去重与业务身份。
- **30 秒回答：** Request ID 关联一次 HTTP 请求及 Nginx/Go 日志；trace ID 关联分布式调用链；Idempotency-Key 把同一主体的等价重试绑定到同一处理结果；DrawID 是领域中的一次抽奖身份。当前只有 Request ID，其他三者都没有。
- **深入回答：** 边缘只复用恰好一个、1～64 字节、字符集 `[A-Za-z0-9_.:-]` 的 `X-Request-ID`，否则生成替代值，并把同一个值传给 Go；Go middleware 又把它放入 context。它不要求全局业务唯一，客户端甚至可以重复使用，因此绝不能作为数据库唯一键或结果查询键。正式链路通常会同时存在：trace 用于 observability，idempotency key 保护命令，DrawID 标识最终业务事实，三者可互相记录但不能合并。
- **项目证据：** [Request ID middleware](../../../internal/infrastructure/httpapi/request_id.go)、[Nginx validation/map](../../../deploy/docker/nginx.conf)、[safe/unsafe ID Compose 验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** HTTP unit 测试确实验证 Request ID 进入 repository context；application unit 的 context 测试只验证同一个通用 context 被传下去，不能把它说成 application unit 专门验证了 Request ID。
- **技术取舍：** 复用安全客户端 ID 便于跨边缘关联，替换非法/重复值防日志注入；未来接入 OpenTelemetry 时可保留 Request ID 作为外部相关键，同时遵守标准 trace context。
- **来源：** `官方技术事实` [W3C Trace Context](https://www.w3.org/TR/trace-context/)、[OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)；`面经题型启发` [context 常用场景面经](https://www.nowcoder.com/discuss/603841169302245376)。

## 17. `Cache-Control: no-store` 能保证临时结果不落盘吗？

- **面试官意图：** 考察缓存指令与数据生命周期边界。
- **30 秒回答：** 不能。`no-store` 要求 cache 不存储请求/响应，但无法阻止客户端代码、截图、浏览器扩展、代理日志或调用方数据库主动保存。`ephemeral` 描述的是本服务没有把 selection 写成业务结果，不是数据从世界上消失。
- **深入回答：** 成功、Go JSON error 和 API location 的 gateway error 都设置 no-store；Nginx 也隐藏 upstream 的同名 header，避免重复。它减少浏览器/CDN/shared cache 重放随机响应或错误 envelope 的风险，但不是加密、访问控制或数据删除协议。验收检查恰好一个 no-store header；非法 Host 421 也有 no-store，parser-level 早期错误则不宜泛化保证。
- **项目证据：** [success/error headers](../../../internal/lottery/adapter/httpapi/selection.go)、[shared error writer](../../../internal/infrastructure/httpapi/errors.go)、[Nginx header policy](../../../deploy/docker/nginx.conf)、[Compose header 验收](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** 不要把 `no-cache` 与 `no-store` 混为一谈；前者允许存储但要求复用前重新验证。也不要说 no-store 能保证服务 access log 不含路径，日志策略是另一层。
- **技术取舍：** no-store 是低成本的缓存安全默认值；未来若有可查询 Draw，可对业务结果 endpoint 设计私有缓存/ETag，但命令响应与含身份信息的表示仍需单独威胁建模。
- **来源：** `官方技术事实` [RFC 9111 `no-store`](https://www.rfc-editor.org/rfc/rfc9111.html#section-5.2.2.5)与[`no-cache`](https://www.rfc-editor.org/rfc/rfc9111.html#section-5.2.2.4)；`项目事实` 上述 header 验收。

## 18. 为什么分成 HTTP adapter、application service、domain selector、repository/random adapter？

- **面试官意图：** 判断“分层架构”是否对应真实依赖方向，而不是目录数量。
- **30 秒回答：** HTTP adapter 负责协议解析与 DTO；application service 编排“读 Strategy → 选择 Award → 复核结果”；domain 保证 Strategy/Award 与加权算法不变量；MySQL 和 CryptoSource 是出站适配器。接口由使用方拥有，只暴露 `FindByID` 和 `Select` 的最小能力。
- **深入回答：** application 的 `StrategyReader`/`AwardSelector` 不泄露 SQL、Gin 或随机实现，service 返回 domain 值而不是 HTTP DTO。它还复核 repository 返回的 StrategyID、selector 返回 Award 的完整字段是否属于同一快照，依赖违约时 fail closed。HTTP 层将 domain/application error 映射为公开 fault，composition root 统一注入共享 `sqlx.DB` 与 CryptoSource。分层的价值是每层可独立证明契约，不是“未来一定拆微服务”。
- **项目证据：** [application ports](../../../internal/lottery/application/repository.go)、[selection service](../../../internal/lottery/application/ephemeral_selection.go)、[domain selector](../../../internal/lottery/domain/weighted_selector.go)、[composition root](../../../cmd/growth-api/main.go)。
- **追问陷阱：** 不要说 domain 完全“纯函数”：selector 通过端口依赖随机源；它只是把副作用边界缩小。也不要把 application interface 放到 provider 包再声称依赖倒置。
- **技术取舍：** 多一层映射和 small interfaces 增加文件数，但降低协议/存储对业务规则的污染。对只有一个简单 CRUD 的小程序可能过度；本项目的 uint64、随机、快照和错误边界已足以支撑此拆分。
- **来源：** `官方技术事实` [Go Code Review Comments：interfaces 通常属于 consumer](https://go.dev/wiki/CodeReviewComments#interfaces)；`面经题型启发` [Gin 相比标准库、项目分层追问](https://www.nowcoder.com/discuss/603841169302245376)与[provider adapter 抽象追问](https://www.nowcoder.com/discuss/603841169302245376)。

## 19. 为什么启动时仍构造并 Validate service，而不是 feature flag 关闭就跳过？

- **面试官意图：** 考察 fail-fast、配置漂移和 typed-nil 陷阱。
- **30 秒回答：** feature flag 只决定是否注册公开 route，不应该掩盖错误组合。进程始终构造 Repository、Selector、Service 并 Validate；nil、typed-nil 或手工零值会启动失败，避免 readiness 绿色而首次打开 flag 或首个请求才暴露故障。
- **深入回答：** Go interface 可能包含动态类型为指针、动态值为 nil 的 typed-nil，此时接口本身不等于 nil。构造器用受限 reflection 检查 nil-capable kinds，`Validate` 还保护手工创建的零值 service；HTTP `RegisterRoutes` 再做一层 composition 检查和 timeout 上界检查。生产/staging 若尝试打开 ephemeral flag，配置加载直接报错，不依赖运行时请求路径。
- **项目证据：** [typed-nil/Validate 实现](../../../internal/lottery/application/ephemeral_selection.go)、[composition tests](../../../internal/lottery/application/ephemeral_selection_test.go)、[route registration validation](../../../internal/lottery/adapter/httpapi/selection.go)、[环境硬 gate](../../../internal/platform/appconfig/config.go)。
- **追问陷阱：** reflection 检查不是通用 DI 框架；它只解决当前小接口 typed-nil。健康检查成功也不证明某个 Strategy 存在，`/ready` 只验证 MySQL readiness。
- **技术取舍：** 关闭功能仍验证依赖会让最小进程配置稍严格，但能提前发现部署漂移。若未来模块很多且按需加载，可把“模块未安装”和“模块已安装但 route 关闭”建成不同状态。
- **来源：** `官方技术事实` [Go spec：interface values](https://go.dev/ref/spec#Interface_types)、[Effective Go：interfaces and methods](https://go.dev/doc/effective_go#interfaces_and_types)；`面经题型启发` [Go interface 与单例/依赖问题面经](https://www.nowcoder.com/discuss/603841169302245376)。

## 20. 为什么读取 Strategy 要用一个只读 Repeatable Read 事务？

- **面试官意图：** 考察聚合一致性与事务隔离，不只是“SELECT 不用事务”。
- **30 秒回答：** Strategy root 和 Awards 在两条 SQL 中读取；如果不在同一快照，配置并发变更可能让 root 与 children 来自不同版本。Repository 用一个 read-only Repeatable Read transaction，按 AwardID 排序读取后再恢复领域聚合，保证本次选择基于一个自洽快照。
- **深入回答：** 第一条查询读 root，第二条查询读最多 N+1 个 children；在 InnoDB consistent read 下，同一 Repeatable Read transaction 的一致性读取共享 snapshot。`ReadOnly` 表达意图并允许数据库优化，但不是权限边界；真正写保护来自 runtime 用户的 SELECT-only grants。事务 commit 后才恢复领域对象，避免在持有连接时做额外业务计算。固定 `ORDER BY award_id` 与 domain 的 canonical sort 共同消除数据库返回顺序不确定性。
- **项目证据：** [`FindByID` 与 `readSnapshotOptions`](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[transaction ordering test](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)、[Repository integration tests](../../../internal/lottery/adapter/mysqlrepo/repository_integration_test.go)。
- **追问陷阱：** read-only transaction 不等于账号不能写；有权限时数据库行为仍取决于实现和语句。Repeatable Read 也不冻结其他事务，只保证本事务观察模型；正式 Draw 若要锁库存是另一套事务设计。
- **技术取舍：** 两条简单查询加快照比大 JOIN 更直接地恢复聚合并检测 root 缺失；代价是多一次 round trip。若 profile 证明必要，可评估 JOIN/JSON aggregate，但必须保持 N+1 检测、排序和完整性语义。
- **来源：** `官方技术事实` [MySQL InnoDB consistent nonlocking reads](https://dev.mysql.com/doc/refman/8.4/en/innodb-consistent-read.html)、[`database/sql` transactions](https://go.dev/doc/database/execute-transactions)；`面经题型启发` [MySQL 与数据一致性面经](https://www.nowcoder.com/discuss/603841169302245376)。

## 21. 为什么 Award 上限是 1000，SQL 却 `LIMIT 1001`？

- **面试官意图：** 考察容量边界是否能检测截断，而不是只会“加 LIMIT 防慢查询”。
- **30 秒回答：** 1000 是领域和同步选择路径的 safety bound；读取 1001 是 N+1 sentinel：0～1000 行可完整恢复，读到第 1001 行就能明确判定 stored aggregate 超限。若只 LIMIT 1000，超限配置会被静默截成另一个看似合法的 Strategy。
- **深入回答：** domain 构造/恢复阶段拒绝超过 `MaxAwardsPerStrategy`，加权选择和结果复核最坏都是 O(n)。上限约束单请求 CPU/内存和 cooperative timeout 中不可抢占的同步区段；它不是生产容量结论或运营建议。SQL 的 LIMIT 只限制返回行数，数据库为满足 WHERE/ORDER 仍可能扫描，正确索引和执行计划仍要观察。超限/非法存储映射 500 `stored_strategy_invalid`，不能当 404 或截断成功。
- **项目证据：** [`MaxAwardsPerStrategy`](../../../internal/lottery/domain/strategy.go)、[`LIMIT Max+1`](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[超限领域/Repository tests](../../../internal/lottery/adapter/mysqlrepo/repository_test.go)。
- **追问陷阱：** 不要说 LIMIT 1001 能证明数据库只做 1001 次工作；也不要把 1000 当成压测得出的最佳值。若未来改常量，必须同步查询、测试、内存/延迟证据和产品约束。
- **技术取舍：** 简单线性算法 + 明确上限目前比 prefix cache/alias table 更易验证。只有 profile 和真实候选规模显示热路径瓶颈，才值得引入预计算、版本化缓存和失效协议。
- **来源：** `项目事实` [第 21 节设计思考](../../design-thinking/lessons/lesson-21.md)；`官方技术事实` [MySQL `LIMIT` optimization](https://dev.mysql.com/doc/refman/8.4/en/limit-optimization.html)；`面经题型启发` [接口新增但旧表缺索引的场景追问](https://www.nowcoder.com/discuss/603841169302245376)。

## 22. 3 秒 selection timeout 是硬上限吗？各层 timeout 为什么要有顺序？

- **面试官意图：** 考察 `context` 的协作式取消和 timeout budget，而不是机械地“所有函数传 ctx”。
- **30 秒回答：** 不是硬实时上限。HTTP adapter 派生 3 秒 context，数据库连接等待/查询可观察取消；use case 在读前后、selector 前后检查 `ctx.Err()`。但同步 selector 和 `crypto/rand.Int` 不接受 context，运行中不能被抢占，只能返回后观察。配置要求 selection timeout 加 1 秒预算不超过 MySQL read timeout 和 HTTP write timeout，让应用通常先产生可控 503。
- **深入回答：** budget 关系是 `selection + 1s <= mysql read` 且 `selection + 1s <= http write`；Nginx `proxy_read_timeout 11s` 是 upstream 读取不活动预算，不等同完整请求墙钟 deadline。若更内层 driver timeout 先发生，错误可能被分类为 repository failure，而不是统一 application deadline。service 在依赖返回后优先检查 context，使已观察的取消赢过同时返回的 dependency error，并阻止进入后续随机选择。CPU/本地同步函数仍需要通过 Award 上限限制不可取消工作。
- **项目证据：** [handler `context.WithTimeout`](../../../internal/lottery/adapter/httpapi/selection.go)、[service cancellation checkpoints](../../../internal/lottery/application/ephemeral_selection.go)、[预算配置验证](../../../internal/platform/appconfig/config.go)、[deadline HTTP test](../../../internal/lottery/adapter/httpapi/selection_test.go)。
- **追问陷阱：** Nginx `proxy_read_timeout` 是两次 read 间隔，不是端到端绝对 11 秒；context cancel 也不会强杀 goroutine。不要声称 3 秒内必然完成任意第三方 driver/未来远程 RNG。
- **技术取舍：** 当前保持 domain selector 无 context，避免纯同步算法被 transport concern 污染；若未来接 HSM/远程随机源，应让出站 port 明确 context、独立 timeout/circuit breaker，而不是在外面泄漏 goroutine 做伪超时。
- **来源：** `官方技术事实` [Go `context`](https://pkg.go.dev/context)、[`database/sql` cancellation](https://go.dev/doc/database/cancel-operations)、[Nginx `proxy_read_timeout`](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_read_timeout)；`面经题型启发` [context 常用场景](https://www.nowcoder.com/discuss/603841169302245376)与[高并发保护追问](https://www.nowcoder.com/discuss/418140886884773888)。

## 23. `database/sql` 会不会自动重试？项目说“Repository 不重试”是否绝对准确？

- **面试官意图：** 考察标准库连接池的隐藏行为，以及能否区分“框架级连接重试”和“业务操作重放”。
- **30 秒回答：** 项目的 Repository、use case 和 selector 没有显式 retry loop，不会主动重做完整读取或选择；但不能绝对说底层零重试。Go `database/sql.DB.BeginTx` 在返回 `Tx` 之前通过 `DB.retry` 处理 `driver.ErrBadConn`，可能换连接并重新尝试 begin。Tx 一旦建立，项目的两条查询和后续 selector 没有由项目层显式重执行。
- **深入回答：** 当前调用链是 `sqlx.DB.BeginTxx → database/sql.DB.BeginTx`。标准库源码显示 BeginTx 把 `db.begin` 包在 `db.retry` 中：先按 cached-or-new 策略有限重试 `ErrBadConn`，最后尝试新连接。因此“Repository 源码没有 for-retry”不等于底层从未更换连接。这个重试发生在 transaction 对象交给 repository 之前；它不是把已经完成的 root/award SELECT 或随机 selection 再执行一次。返回 Tx 后，操作绑定该连接，repository 遇到 1205/1213、ErrBadConn/net.Error 只分类并返回，不自动重跑整个 transaction。
- **项目证据：** [`FindByID` 只有一次 `BeginTxx` 与一组查询](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[use case 只调用一次 reader/selector](../../../internal/lottery/application/ephemeral_selection.go)、[transaction expectations](../../../internal/lottery/adapter/mysqlrepo/repository_transaction_test.go)。
- **追问陷阱：** 不要回答“`database/sql` 永远不会 retry transaction”，因为 BeginTx 前置阶段确有 ErrBadConn retry；也不要反过来说“它会自动重试整个 transaction”。两者都混淆了边界。驱动只有在确认连接坏且操作未提交给数据库等安全条件下才应返回 `driver.ErrBadConn`。
- **技术取舍：** 接受标准库的连接健康恢复，但业务层不做隐式整段重放，使 selection 次数和延迟预算可解释。若未来增加 retry，应显式限定阶段、次数、backoff、deadline 和 Draw 幂等语义。
- **来源：** `官方技术事实` [Go `database/sql` 的 `BeginTx` 与 `DB.retry` 源码](https://go.dev/src/database/sql/sql.go)、[`driver.ErrBadConn` 契约](https://pkg.go.dev/database/sql/driver#ErrBadConn)；`面经题型启发` [数据库、节点故障与项目深挖面经](https://www.nowcoder.com/discuss/603841169302245376)。

## 24. 为什么遇到 deadlock、lock wait timeout 或网络错误不在 use case 内自动重试？

- **面试官意图：** 考察 retry budget、结果重放语义与“错误可重试分类”之间的区别。
- **30 秒回答：** `retryable` 只说明故障类别可能暂时恢复，不等于当前操作可以透明重放。本接口每次调用都会重新选择；虽然 repository read 本身无业务写入，但 use case 若整段重试边界不清，未来很容易把随机选择也重复。当前做法是返回 503，并明确下一次请求是新的 selection。
- **深入回答：** Repository 将 MySQL 1205/1213、`driver.ErrBadConn` 和 `net.Error` 分类为 `ErrRepositoryRetryable`，HTTP 公开为 503。真正自动 retry 至少要决定：只重试 transaction begin/read，还是重试 selector；最大次数、指数退避与 jitter；是否仍在 3 秒 deadline；当第一次读取成功但 response 丢失时如何恢复。当前没有 DrawID 或结果查询，任何“看起来透明”的重试都会让调用方无法证明选择发生几次。标准库在 BeginTx 前的安全 ErrBadConn recovery 与业务层整段 retry 必须分开描述。
- **项目证据：** [error classification](../../../internal/lottery/adapter/mysqlrepo/repository.go)、[service 单次编排](../../../internal/lottery/application/ephemeral_selection.go)、[503 mapping](../../../internal/lottery/adapter/httpapi/selection.go)。
- **追问陷阱：** 不要把死锁 victim retry 的通用建议机械套进随机业务；对纯数据库事务的 retry 也要确保事务函数可重入。这里即使重试只读 snapshot，下一阶段的随机行为仍要在边界上被保护。
- **技术取舍：** fail-fast 牺牲短暂故障下的表面成功率，换来选择次数可解释。正式 Draw 可先持久化唯一 intent/result，再让状态机安全恢复，而不是从 HTTP handler 盲重跑。
- **来源：** `官方技术事实` [MySQL deadlock handling](https://dev.mysql.com/doc/refman/8.4/en/innodb-deadlocks-handling.html)、[AWS Builders' Library：timeouts/retries/backoff with jitter](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/)；`面经题型启发` [“单用户重复调用如何保证幂等”](https://www.nowcoder.com/discuss/603841169302245376)。

## 25. service 没有 mutex，为什么还能并发安全？为什么不用 Redis 分布式锁？

- **面试官意图：** 考察并发安全是否基于共享可变状态，而不是遇到“抽奖”就加锁。
- **30 秒回答：** service 本身没有可变字段，Strategy 是防御性复制后的不可变值，`sqlx.DB`/`database/sql.DB` 是共享连接池，WeightedSelector 不写共享状态，默认 `crypto/rand.Reader` 可并发使用；在这些依赖前提下无需全局 mutex。当前也没有库存、次数或持久结果临界区，Redis 锁没有要保护的业务事实。
- **深入回答：** 并发安全是组合属性：替换成非线程安全 fake、带可变状态的 PRNG 或自定义 reader 后，selector 的承诺也会变化。全局 mutex 只会串行所有 Strategy，不能提供幂等、exactly-once 或响应丢失恢复。Redis 锁还有租约过期、owner token、故障转移和 fencing 问题；即使锁完美，也不能代替数据库中的唯一 Draw/result 约束。当前 pool 最大连接数会让超量 handler 排队，context 可取消等待，但没有 admission control 或 rate limit。
- **项目证据：** [service 并发契约](../../../internal/lottery/application/ephemeral_selection.go)、[selector/source 并发契约](../../../internal/lottery/domain/weighted_selector.go)、[application 64 goroutine test](../../../internal/lottery/application/ephemeral_selection_test.go)、[HTTP shared CryptoSource test](../../../internal/lottery/adapter/httpapi/selection_test.go)。
- **追问陷阱：** “无状态”不能脱离依赖说；接口指向的实现可能有状态。Redis `SET NX` 也不自动等于可靠分布式锁，更不等于业务最终一致性。
- **技术取舍：** 当前选择无锁共享组件以保持并行度和简单性。正式库存扣减更适合数据库条件更新/唯一约束或库存服务的原子协议；分布式锁最多做协调层，最终事实仍需可验证存储约束。
- **来源：** `官方技术事实` [`database/sql.DB` 可安全并发使用](https://pkg.go.dev/database/sql#DB)、[`crypto/rand.Reader` 可并发使用](https://pkg.go.dev/crypto/rand#Reader)、[Redis distributed locks](https://redis.io/docs/latest/develop/use/patterns/distributed-locks/)；`面经题型启发` [Go 并发安全与 Redis 锁追问](https://www.nowcoder.com/discuss/603841169302245376)。

## 26. 第 21 节的“64/100 并发测试”分别是什么？Compose 是 64 并发吗？

- **面试官意图：** 检查候选人是否准确报告测试负载，避免把总请求数说成并发度。
- **30 秒回答：** application unit 创建 64 个 goroutine 调用 service；HTTP unit 创建 100 个 goroutine 通过共享 router/WeightedSelector/CryptoSource；真实 Compose acceptance 是总计 64 个请求，`xargs -P 16`，最大并行 worker 为 16。Compose 不是 64 并发。
- **深入回答：** application test 使用无状态 concurrent stubs，证明 service 编排在安全依赖下没有共享写冲突；HTTP test 使用真实 CryptoSource 和内存 reader，证明 100 个 goroutine 发起的 handler 调用都返回两个配置 Award 之一；Compose 经 Nginx→Go→MySQL 发出 64 次请求，但同时最多 16 个 worker，并逐个校验 200、JSON、单一 no-store、单一 Request ID 与 DTO 候选集合。三种测试的依赖、调度和结论不同，不能混成一个“100 并发真实压测”。
- **项目证据：** [application `workers = 64`](../../../internal/lottery/application/ephemeral_selection_test.go)、[HTTP `workers = 100`](../../../internal/lottery/adapter/httpapi/selection_test.go)、[Compose `concurrent_requests=64` / `concurrent_workers=16`](../../../scripts/compose-lottery-api-acceptance.sh)。
- **追问陷阱：** “64 requests at concurrency 16”不等于 QPS 64，也不说明所有请求严格同一时刻开始；unit test 虽创建 64/100 个 goroutine，但没有 barrier 证明它们在同一纳秒进入临界区，goroutine 数也不等于 OS thread 数。race test 无报告不等于逻辑上没有所有并发 bug。
- **技术取舍：** 当前数字用于功能和竞态信心，故保持小而可重复。容量测试应定义到达率、持续时间、数据分布、连接池、资源配额、P50/P95/P99、错误率与暖机，而不是继续放大 goroutine 常量。
- **来源：** `项目事实` 上述三个测试；`官方技术事实` [Go race detector](https://go.dev/doc/articles/race_detector)、[`xargs -P` 并行语义](https://pubs.opengroup.org/onlinepubs/9799919799/utilities/xargs.html)；`面经题型启发` [100 个 IO goroutine 对计算任务影响的追问](https://www.nowcoder.com/discuss/603841169302245376)。

## 27. 这些测试能证明“高并发、高性能、公平”吗？

- **面试官意图：** 考察证据分层、性能方法论和简历数字诚信。
- **30 秒回答：** 不能。它们证明特定环境下的功能 contract、并发组合和 race 检查通过；没有稳定到达率、持续负载、生产资源、延迟分位、错误预算或多副本模型，所以不能宣称高并发容量。返回配置内 Award 也不构成统计公平证明。
- **深入回答：** unit stub 不含数据库/网络，HTTP unit 不含 Nginx/MySQL，Compose 只有 64 总请求且并行上限 16。选择分布的正确性来自第 20 节均匀有界随机契约、数学映射和小区间穷举，而不是这几十次输出的频数。公平还涉及配置审批、版本、权限、随机材料健康、结果不可篡改和审计。性能优化要先建立场景：候选数、热点 Strategy、pool size、容器 CPU/memory、P99/SLO 与 failure injection。
- **项目证据：** [隔离 acceptance](../../../scripts/compose-lottery-api-acceptance.sh)、[第 21 节 QA 的证据边界](../../qa/lessons/lesson-21.md)、[第 20 节选择器测试](../../../internal/lottery/domain/weighted_selector_test.go)。
- **追问陷阱：** 不要用“跑了 `-race`”证明没有逻辑竞态，也不要用 64 次结果接近期望比例证明随机公平。微 benchmark 也不能相加得到 API P99。
- **技术取舍：** 本节优先建立可重复的 contract/隔离证据；后续性能章节再用明确基线优化。简历应写“用 64 请求/最大并行 16 的隔离 Compose 验收验证链路”，而非“支持 64 并发”。
- **来源：** `官方技术事实` [Go benchmark 文档](https://pkg.go.dev/testing#hdr-Benchmarks)、[NIST SP 800-22 的统计测试边界](https://csrc.nist.gov/pubs/sp/800/22/r1/upd1/final)；`面经题型启发` [Redis QPS/高并发保护等追问](https://www.nowcoder.com/discuss/603841169302245376)与[高并发防护面经](https://www.nowcoder.com/discuss/418140886884773888)。

## 28. Nginx 在这个 API 中解决了什么，`proxy_request_buffering off` 又意味着什么？

- **面试官意图：** 判断候选人能否区分 edge policy、应用语义与 parser 局限。
- **30 秒回答：** Nginx 提供 loopback Host allowlist、安全 Request ID、16 KiB ingress 上限、3 秒 client body timeout、TE/Trailer declaration 拒绝、upstream timeout 和 413/502/504 JSON 模板。`proxy_request_buffering off` 让请求体边读边转发，避免先把不合规大/慢 body 全部缓存；它不表示接口支持 streaming body。
- **深入回答：** route 自身仍禁止任何非零/未知 body。16 KiB 只是资源保险，16385-byte 已知长度在 edge 413；慢 body 受 `client_body_timeout` 的相邻 read 间隔约束。`proxy_connect_timeout 2s`、`proxy_send_timeout 4s`、`proxy_read_timeout 11s` 是不同阶段的 inactivity budgets。Nginx隐藏 upstream Request ID/no-store 后重写单一 header，减少重复。日志只记录 `$uri` 而非 `$request_uri`，避免 query 落日志；但 StrategyID 仍在 path 中，日志并非匿名。
- **项目证据：** [Nginx完整配置](../../../deploy/docker/nginx.conf)、[oversize/gateway/headers 验收](../../../scripts/compose-lottery-api-acceptance.sh)、[API 网关契约](../../api/lessons/lesson-21.md)。
- **追问陷阱：** buffering off 不等于无限制透传；`client_max_body_size` 仍生效。各类 timeout 多为读写间隔而非总耗时。unsupported/invalid TE 也可能在 location 前被 parser 用 501/HTML 拒绝。
- **技术取舍：** edge 与 Go 都保留必要防线，提高直连测试和代理部署的韧性；代价是错误模板/策略重复。未来多 body 类型 API 应拆 location/route policy，不能让本节 no-body 规则阻断上传或 streaming。
- **来源：** `官方技术事实` [Nginx `proxy_request_buffering`](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_request_buffering)、[`client_max_body_size`](https://nginx.org/en/docs/http/ngx_http_core_module.html#client_max_body_size)、[`client_body_timeout`](https://nginx.org/en/docs/http/ngx_http_core_module.html#client_body_timeout)；`面经题型启发` [Nginx、负载均衡与 HTTP 报文边界面经](https://www.nowcoder.com/discuss/603841169302245376)。

## 29. 为什么 runtime MySQL 用户只有两张表的 SELECT？`ReadOnly: true` 不够吗？

- **面试官意图：** 考察最小权限是否落实到数据库身份，而不是只靠代码约定。
- **30 秒回答：** 本节 API 只读 Strategy，所以长期 `growthos_app` 只被授予 `lottery_strategy` 和 `lottery_strategy_award` 的 SELECT。事务 `ReadOnly: true` 是意图/数据库提示，不是可靠授权边界；migration 和 grant reconciliation 使用独立身份与 secret，运行时凭据泄漏时不能写业务表或 migration 元数据。
- **深入回答：** 初始化历史上可能给过宽权限，启动时独立 grant job 用 root Unix socket 撤销并按精确 allowlist 重授，随后比较 `SHOW GRANTS`。应用只挂 app secret，migrator 只挂 migration secret，grant job 无网络并单独持 root secret。隔离 acceptance 由 writer fixture 身份插入测试数据，再核对 runtime identity 的精确 `SHOW GRANTS`、mandatory role 为空和最终业务 fingerprint 不变；INSERT、UPDATE、DELETE 与 `schema_migrations` 的 runtime 负向探针由长期 `compose-smoke` 验证。只读权限仍不能防止昂贵 SELECT、数据外泄或侧信道，所以认证、query 固定化、timeout 与资源限制仍必要。
- **项目证据：** [grant reconciliation](../../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)、[Compose identities/secrets](../../../deploy/compose/compose.yaml)、[隔离 acceptance 的 grants/fingerprint](../../../scripts/compose-lottery-api-acceptance.sh)、[长期 smoke 的写权限负向探针](../../../scripts/compose-smoke.sh)。
- **追问陷阱：** 不要把测试 fixture writer 的 SELECT/INSERT 权限说成 runtime app 权限；它只存在于一次性隔离验收。也不要声称 SELECT-only 可以阻止所有 DoS 或敏感数据读取。
- **技术取舍：** 多身份和 secret 增加部署复杂度，但显著缩小 blast radius 并让职责可审计。若未来正式 Draw 需要写，应新增最小存储过程/表级权限或专用 writer，而不是把 app 恢复成 schema-wide CRUD。
- **来源：** `官方技术事实` [MySQL GRANT](https://dev.mysql.com/doc/refman/8.4/en/grant.html)、[OWASP Database Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Database_Security_Cheat_Sheet.html)；`面经题型启发` [MySQL/数据库一致性与项目安全追问](https://www.nowcoder.com/discuss/603841169302245376)。

## 30. 这个接口如果放到公网，最大的安全缺口是什么？

- **面试官意图：** 考察 threat modeling，尤其是对象级授权和资源消耗。
- **30 秒回答：** 最大缺口不是某个 header，而是没有身份、租户/活动资格、对象级授权和次数/速率约束。任何能访问 endpoint 的调用者都可枚举 StrategyID 并无限触发数据库读取和随机选择；demo header、Host allowlist、SELECT-only 都不能解决 BOLA 或 abuse。
- **深入回答：** 公网化至少需要：认证主体；主体对活动/Strategy 的对象级授权；资格与次数的权威事实；rate limit/admission control；CORS/CSRF 策略；防枚举的错误策略；敏感字段和日志审查；Draw/result 幂等；库存/发放与审计。当前 Nginx 只接受 loopback Host，feature flag 又禁止 production，正是因为这些边界尚未建立。连接池会限制数据库并行度，却不是限流器；排队 goroutine 仍可耗尽进程资源。
- **项目证据：** [feature/environment gate](../../../internal/platform/appconfig/config.go)、[threat model 与缺口](../../design-thinking/lessons/lesson-21.md)、[Host boundary](../../../deploy/docker/nginx.conf)。
- **追问陷阱：** `X-GrowthOS-Demo-Mode` 不是 API key；CORS 也只约束浏览器，不阻止服务端调用。返回 404 防枚举不能替代真正授权。Redis 分布式锁不是限流或配额模型。
- **技术取舍：** 当前选择完全不公网化，比半套认证更诚实。未来可先以活动/用户为入口、由服务端解析可用 Strategy，而不是允许客户端自由指定任意 StrategyID。
- **来源：** `官方技术事实` [OWASP API1:2023 BOLA](https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/)、[OWASP API4:2023 Unrestricted Resource Consumption](https://owasp.org/API-Security/editions/2023/en/0xa4-unrestricted-resource-consumption/)；`面经题型启发` [单容器限流和库存超卖场景题](https://www.nowcoder.com/discuss/603841169302245376)。

## 31. 当前可观测性有什么，为什么还不算审计系统？

- **面试官意图：** 考察 logs、metrics、traces 与不可篡改业务审计的区别。
- **30 秒回答：** 当前有安全 Request ID、Nginx/Go access log、低基数 selection `error_class`、health/readiness 和 Docker health；它们能关联请求和故障，但没有 trace、业务指标、DrawID、Strategy 版本、随机证据或不可篡改结果账本，所以不构成抽奖审计。
- **深入回答：** Go access log 使用 method、route pattern、status、duration；Nginx 记录 method、`$uri`、status/upstream status/timing，避免 query 和 Referer。selection failure 只记录 Request ID、canonical StrategyID、审核过的 error class，不渲染 cause。即便日志完整，它仍可能轮转、被有权限的人修改，且成功 selection 没有 durable identity，无法回答“某用户哪一次抽奖最终是什么”。未来建议按 result class 建低基数计数/延迟 histogram，接 trace context，并把审计事实写入受访问控制、版本化、可校验的存储。
- **项目证据：** [HTTP access/recovery middleware](../../../internal/infrastructure/httpapi/middleware.go)、[selection failure logging](../../../internal/lottery/adapter/httpapi/selection.go)、[Nginx sanitized log format](../../../deploy/docker/nginx.conf)。
- **追问陷阱：** Request ID 不是业务审计 ID，access log 也不证明结果已发放。不要把 AwardID/StrategyID 直接做 metrics label，可能造成高基数；它们更适合受控日志/trace 属性。
- **技术取舍：** 当前先建立安全相关性而不引入完整 observability 栈；代价是容量和单次成功无法审计。正式 Draw 的审计模型应从业务不变量推导，不是简单“多打日志”。
- **来源：** `官方技术事实` [OpenTelemetry traces](https://opentelemetry.io/docs/concepts/signals/traces/)、[Prometheus instrumentation：labels](https://prometheus.io/docs/practices/instrumentation/#use-labels)、[OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)；`面经题型启发` [项目性能、PPROF 与故障处理追问](https://www.nowcoder.com/discuss/603841169302245376)。

## 32. 怎样把 ephemeral selection 演进为正式 Draw，而不破坏本节已验证的部分？

- **面试官意图：** 综合考察领域建模、幂等、事务、库存、消息与迁移顺序。
- **30 秒回答：** 保留已验证的 Strategy 聚合、快照读取、加权选择器和 DTO/错误基础设施；新增 Draw 作为一次请求的持久 identity，把 caller/activity/Strategy version/idempotency key/request hash/status/result 绑定。先用数据库唯一约束建立单一最终结果，再在同一事务中记录 outbox，异步发放通过幂等 consumer 推进，客户端按 DrawID 查询未知结果。
- **深入回答：** 一个稳健顺序是：一，定义 eligibility、attempt、result、fulfillment 不变量；二，发布不可变 Strategy version/快照摘要；三，认证与对象级授权；四，`POST /draws` 接收/生成受约束 idempotency key，在唯一约束下先建立 Draw intent；五，在事务边界内选择并持久 result，使重复 key 返回同一事实；六，库存用条件更新/预占与补偿状态机，不能只靠 Redis 锁；七，result 与 outbox 原子提交，consumer 按 event/fulfillment identity 去重；八，提供 GET Draw 查询处理响应丢失；九，接审计、指标、限流和故障注入。CryptoSource 随机性、业务公平与可验证公平仍要分别说明。
- **项目证据：** [本节明确的停止条件](../../course/part-03/lesson-21-lottery-api.md)、[ADR-0018 alternatives/future gate](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)、[first-principles design](../../design-thinking/lessons/lesson-21.md)。
- **追问陷阱：** “数据库事务 + MQ”不自动等于 exactly-once；通常实现的是本地单一事实、at-least-once delivery 和幂等 consumer。响应在 commit 后丢失是 outcome unknown，必须通过相同 key/DrawID 查询，而不是重新抽。也不能先发奖再补写结果。
- **技术取舍：** 单体数据库唯一约束 + transactional outbox 比一开始引入分布式事务更易证明；当库存/发放成为独立服务时，再基于明确一致性边界演进 saga。保留 selector 作为纯选择内核，避免把库存、HTTP 或 MQ 逻辑塞回 domain algorithm。
- **来源：** `技术资料` [AWS Builders' Library：Making retries safe with idempotent APIs](https://aws.amazon.com/builders-library/making-retries-safe-with-idempotent-APIs/)、[microservices.io Transactional Outbox](https://microservices.io/patterns/data/transactional-outbox.html)、[OWASP REST Security](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html)；`面经题型启发` [重复调用幂等、库存超卖与技术选型场景题](https://www.nowcoder.com/discuss/603841169302245376)。

## 一分钟项目口述模板

> 第 21 节我没有直接把“随机返回 Award”包装成正式抽奖，而是先定义一个仅限 development/test、默认关闭的 ephemeral selection contract。请求经 Nginx 做 loopback Host、framing、大小、timeout 与 Request ID 边界，再由 Gin 严格校验规范 uint64 path、demo header、query/body 和虚假幂等声明。application use case 通过 consumer-owned ports 在 MySQL Repeatable Read 只读快照中恢复最多 1000 个 Award 的 Strategy，再调用密码学随机加权选择器，并复核返回 Award 确属该快照；公开 DTO 用十进制 string 保住完整 uint64，错误按 400/404/500/503 与 gateway 502/504 分层。证据上，application unit 是 64 goroutine、HTTP unit 是 100 goroutine，隔离 Compose 是 64 个总请求、最大并行 16，并验证 SELECT-only 权限与业务表 fingerprint 不变。我同时明确它没有 Draw、同 key 同结果的幂等记录、库存和发放，因此下一步必须先建立持久 Draw identity、唯一约束、结果查询与 outbox，而不是盲目重试或加分布式锁。

## 面试前自检清单

1. 能否在 30 秒内说清 `ephemeral selection != DrawResult`？
2. 能否解释 POST/200/no_reward，而不使用“参数多所以 POST”一类话术？
3. 能否说清 `Content-Length: 0` 与无 body 在 Go 入口不可区分？
4. 能否准确说明 supported chunked 的 JSON 400 证据，以及 unsupported/invalid TE 可能 parser-level 501/HTML？
5. 能否说明 `$http_trailer` 只拒绝非空 declaration，Go 仍检查可观察 `Request.Trailer`？
6. 能否说明非法 Host 是非 JSON 421、没有稳定业务 code？
7. 能否说明真实 HEAD wire 是 405 + headers、没有 JSON body？
8. 能否准确复述 application 64 goroutine、HTTP 100 goroutine、Compose 64 总请求/最大并行 16？
9. 能否解释 `database/sql` 可能在 BeginTx 返回前因 ErrBadConn 换连接重试，但项目层没有显式重做 transaction/selection？
10. 能否指出正式 Draw 需要持久 identity、唯一约束、结果查询、库存/发放状态与审计，而不把 Redis 锁当最终正确性？
