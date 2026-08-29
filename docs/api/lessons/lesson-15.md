# 第 15 节 API 记录：系统探针首次前端联调

- **章节：** [前后端第一次联调](../../course/part-02/lesson-15-first-fullstack-integration.md)
- **日期：** 2026-08-29
- **状态：** 已实现并完成真实浏览器联调
- **QA：** [第 15 节 QA 验收证据](../../qa/lessons/lesson-15.md)

## 1. 本节 API 变化

本节没有新增 HTTP 路由，也没有新增业务资源 API。它让 React 第一次真实消费第 11～13 节已有的系统端点，并加固所有 Go 统一错误响应的缓存语义。

| 动作 | 接口 / 行为 | 调用方 | 当前状态 |
| --- | --- | --- | --- |
| 首次前端消费 | `GET /health` | 系统状态页 | 已验收 |
| 首次前端消费 | `GET /ready` | 系统状态页 | 已验收 |
| 加固 | 所有统一 error envelope 增加 `Cache-Control: no-store` | 所有 HTTP 调用方 | 已验收 |
| 预留代理前缀 | `/api` 原样代理到 Go upstream | 后续业务 API | 当前没有业务路由，不代表已实现 |

`/health` 与 `/ready` 保持不带 API 版本，因为它们描述进程与实例的运行状态，而不是版本化业务资源。不得为了前端路径统一额外创建 `/api/v1/health` 别名。

## 2. 浏览器调用路径

浏览器只允许使用同源路径：

```ts
fetch("/health", ...)
fetch("/ready", ...)
```

本地 development 和 preview 环境由 Vite 把两个精确探针路径与 `/api` 命名空间原样转发到 Go upstream。匹配使用 `^/health$`、`^/ready$`、`^/api(?:/|$)`，不会把 `/health-anything` 或 `/apiwhatever` 意外转给 Go：

| 浏览器路径 | 默认 upstream 路径 | rewrite |
| --- | --- | --- |
| `/health` | `http://127.0.0.1:8080/health` | 无 |
| `/ready` | `http://127.0.0.1:8080/ready` | 无 |
| `/api/...` | `http://127.0.0.1:8080/api/...` | 无 |

代理目标由 Vite Node 进程读取：

```dotenv
GROWTHOS_WEB_API_PROXY_TARGET=http://127.0.0.1:8080
```

该变量不使用 `VITE_` 前缀，不应出现在浏览器环境或 bundle 中。值必须是无用户名、密码、path、query、fragment 的 HTTP(S) origin。生产环境应由正式同源反向代理/网关实现相同路径契约，不能依赖 Vite dev proxy。

## 3. 通用请求约定

本节统一 JSON client 当前发送：

```http
GET /health HTTP/1.1
Accept: application/json
```

浏览器 fetch 选项：

| 项目 | 值 | 目的 |
| --- | --- | --- |
| `method` | `GET` | 本节只消费只读探针 |
| `cache` | `no-store` | 每次读取当前实例事实，不使用浏览器缓存 |
| `credentials` | `same-origin` | 只允许同源凭证语义，不开启跨源凭证 |
| `mode` | `same-origin` | 浏览器请求模式禁止越过当前 origin |
| `redirect` | `error` | 不跟随 30x 跳转到未授权 origin |
| `signal` | 合并到内部 `AbortController` | 支持调用方取消与本地 timeout |
| timeout | 默认 5000 ms；允许 100～30000 ms 安全整数 | 给浏览器等待设置明确上限 |

client 只接受以单个 `/` 开始的路径。绝对 URL、`//host/path`、包含反斜杠或缺少前导 `/` 的相对路径在发出网络请求前返回 contract error。反斜杠不是代码风格问题：WHATWG URL 解析可能把它归一化成 host 分隔符，因此必须在通用请求边界拒绝。

## 4. `GET /health`

### 4.1 成功响应

```http
GET /health HTTP/1.1
Host: 127.0.0.1:5173
Accept: application/json
```

经本地同源代理后，Go 的响应契约保持：

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: <current-request-id>
```

```json
{
  "status": "ok",
  "version": "dev",
  "timestamp": "2026-08-29T12:34:56.123456789Z"
}
```

前端运行时要求：

| 字段 | 类型 / 约束 | 语义 |
| --- | --- | --- |
| `status` | 固定 `"ok"` | 当前 Go 实例能处理该 HTTP 请求 |
| `version` | 非空 string | 当前构建标签；后端空值会归一化为 `unknown` |
| `timestamp` | RFC 3339 形状且可由 JS 解析的非空 string | handler 生成响应时的 UTC 时间 |

允许额外字段，但不能缺少或弱化以上字段。成功不访问 MySQL，不证明任何外部依赖就绪。

### 4.2 失败解释

`/health` 的 network、gateway、timeout、非预期 HTTP 或 contract 失败都使浏览器无法确认当前 API 状态。前端不能把它自动解释为 MySQL 故障，也不能根据 `/ready` 的结果覆盖 liveness 事实。

## 5. `GET /ready`

### 5.1 MySQL 连接就绪

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: <current-request-id>
```

```json
{
  "status": "ready",
  "version": "dev",
  "timestamp": "2026-08-29T12:34:56.123456789Z"
}
```

前端运行时要求：

| 字段 | 类型 / 约束 | 语义 |
| --- | --- | --- |
| `status` | 固定 `"ready"` | 当前有界 MySQL Ping 成功 |
| `version` | 非空 string | 当前构建标签 |
| `timestamp` | RFC 3339 形状且可解析的非空 string | handler 生成响应时的 UTC 时间 |

该结果只证明当前实例到 MySQL 的连接检查在服务端预算内成功，不证明业务查询、数据一致性、Migration 版本、慢 SQL、复制延迟或整个集群正常。

### 5.2 MySQL 未就绪

```http
HTTP/1.1 503 Service Unavailable
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: <current-request-id>
```

```json
{
  "error": {
    "code": "dependency_unavailable",
    "message": "service unavailable",
    "request_id": "<current-request-id>"
  }
}
```

前端只有在以下条件都成立时，才把它解释为“API 存活、MySQL 未就绪”：

1. `/health` 当前一轮成功；
2. `/ready` 收到 HTTP 503；
3. error envelope 运行时结构合法；
4. `error.code === "dependency_unavailable"`；
5. response header 和 error body 同时提供 request ID 时二者一致。

其他 readiness 失败只表示就绪状态未知，不应被统一压成数据库故障。

## 6. 统一错误 envelope

所有非成功 Go API 响应继续使用：

```json
{
  "error": {
    "code": "stable_machine_code",
    "message": "safe public message",
    "request_id": "correlation-id"
  }
}
```

第 15 节把 `Cache-Control: no-store` 放到统一错误出口。这意味着 readiness 503、404、405 和其他通过该出口返回的 fault 都明确禁止缓存。原因是 `request_id` 属于单次请求，缓存重放旧 body 会破坏关联排障。

前端 HTTP error 解码要求 `code`、`message`、`request_id` 都为非空 string。程序分支依据 `kind/status/code`，不解析 message；页面可以显示项目定义的安全话术，不应直接回显任意响应 body 或底层异常。

## 7. `X-Request-ID` 约定

- 成功响应：前端从 `X-Request-ID` header 读取关联 ID；header 缺失不让成功响应本身失败，但 UI 明确显示“响应未提供”；
- 错误响应：body 必须有 `error.request_id`；header 可用于额外校验；
- header 与 body 同时存在且不同：视为 contract error，不能任选一个继续；
- `/health` 和 `/ready` 是两次 HTTP 请求，应各有自己的 request ID；
- 手动刷新产生新一轮请求，不复用旧 request ID；
- request ID 可以用于关联服务日志，但不是认证令牌、trace 完整证明或跨请求业务 ID。

若未来改为浏览器跨源直连，除了严格 CORS allowlist，还必须明确通过 `Access-Control-Expose-Headers` 暴露 `X-Request-ID`。本节采用同源方案，因此不新增该 CORS 配置。

## 8. timeout、取消与竞态

前端默认 5 秒 timeout 与后端 readiness Ping timeout 是两个不同预算：

- 后端 timeout 约束数据库探测并尽量返回稳定 503；
- 前端 timeout 约束浏览器愿意等待多久；
- 前端 abort 会取消 fetch，并可通过 HTTP request context 向服务端传播，但不能假设所有下游工作都已经终止；
- timeout 返回 `kind=timeout`，页面不把它显示为服务端明确的 503；
- 页面刷新/卸载返回 `kind=cancelled`，旧一轮结果被 generation 阻止，不应覆盖新一轮状态。

两个端点并行请求、分别结算。不能使用一个共同 response type，也不能在任一 rejection 后丢弃另一个已取得的事实。

## 9. 前端错误对象

统一 client 输出的稳定维度：

| 字段 | 必需性 | 说明 |
| --- | --- | --- |
| `kind` | 必需 | `http/gateway/network/timeout/cancelled/contract` |
| `status` | 可选 | 取得可信 HTTP response 时记录 |
| `code` | 可选 | 合法服务端 error envelope 的稳定 code |
| `requestId` | 可选 | 成功 header 或错误 envelope/header 中的关联 ID |
| `elapsedMs` | 可选 | 已取得对应观测时的浏览器往返；不是服务端处理时长 |

前端不暴露原始 response body、fetch exception、Go cause、数据库 driver error、DSN、账号或 SQL。

`gateway` 是真实联调后增加的客户端分类：当同源代理返回非 JSON 的 502/503/504 时，浏览器已经取得网关层 HTTP 响应，但没有取得可信的 GrowthOS error envelope。它与后端合法 JSON 503 严格区分：前者显示“代理无法连接 API”，后者在 health 成功时显示“MySQL 未就绪”。客户端不会为 gateway 伪造后端 `error.code`。

## 10. CORS 与生产边界

本节没有 CORS 需求，因此 Go 不增加 `Access-Control-Allow-Origin: *` 或反射任意 Origin。Vite proxy 只解决本地 development/preview 的同源体验。

生产部署至少需要：

```text
https://growthos.example/        -> frontend static assets
https://growthos.example/health  -> Go /health
https://growthos.example/ready   -> Go /ready（是否外部暴露需按运维边界复核）
https://growthos.example/api/... -> Go business API
```

实际生产网关、认证、TLS、probe 暴露范围和缓存规则尚未在本节实现。若未来确有跨源客户端，需要新增 ADR，定义精确 origin allowlist、credentials、methods、headers、preflight、`X-Request-ID` expose 和 CSRF 威胁模型。

## 11. 验证状态

本契约已由 4 个 Vitest 文件展开的 34 项前端单元/组件测试、Go HTTP 契约测试、冻结 lockfile 安装、类型检查、生产构建以及真实 MySQL + Go + Vite 浏览器联调验证。浏览器正常态为 `/health=200`、`/ready=200`；临时账号失效并终止该账号连接后为 200/503；停止 API 后 Vite 返回非 JSON 502，页面进入 gateway/unknown，而不是误报 MySQL 故障。完整环境、命令、可访问性结果和清理证据见[第 15 节 QA](../../qa/lessons/lesson-15.md)。

## 12. 遗留问题

- 第 16 节将当前端、API 与 MySQL 放入 Compose 时，需要重新验证容器监听和 proxy target；
- 业务 API 出现后，统一 client 是否扩展 POST/body/auth/retry/idempotency 必须按真实用例设计，不能把当前 GET 探针 client 当作最终 SDK；
- 自动轮询、退避、可见性暂停、后台监控与告警当前均未实现；
- 真正的 production reverse proxy、CORS、认证和 probe 外部暴露策略尚未实现；
- Mock 业务页面仍不属于真实 API 验收范围。
