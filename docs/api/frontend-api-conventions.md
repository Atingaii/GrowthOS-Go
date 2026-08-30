# 前端 API 约定

## 调用链路

页面和业务组件不得直接拼接 `fetch` 或 Axios 请求。统一通过以下边界调用：

```text
页面/组件
  ↓
领域 API 模块（当前前端仅有 system；后端 lottery route 已存在但尚无前端模块）
  ↓
统一 HTTP Client
  ↓
浏览器同源路径
  ↓
Vite 开发/预览代理，或 Compose Nginx 入口
  ↓
Go Gin API
```

第 15 节已实现的真实前端链路只覆盖系统状态页：`systemApi` 分别调用同源的 `GET /health` 与 `GET /ready`，Vite 在宿主开发模式下代理到 Go API。第 16 节增加 Compose Nginx 入口，动态解析 API 容器、隐藏上游同名请求 ID 后统一回写最终 `X-Request-ID`。第 21 节后端已经实现 `POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections`，但前端尚无 `lotteryApi`、DTO decoder 或页面状态管理，`/lottery` 仍是 `Math.random()` Mock。页面组件不得因为 route 已存在就直接拼接 `fetch` 绕过本约定。

该后端 route 默认关闭且仅 development/test 可启用，返回的是不持久化、不可幂等重放的临时选择。前端接入前必须按[第 21 节 API 契约](lessons/lesson-21.md)实现 decimal-string ID decoder，并区分 reward、no_reward、HTTP error、gateway error 与取消；不能把它包装成正式 Draw 成功。其设计和证据见[课程](../course/part-03/lesson-21-lottery-api.md)、[QA](../qa/lessons/lesson-21.md)、[设计手记](../design-thinking/lessons/lesson-21.md)、[面试问答](../interview/lessons/lesson-21.md)和 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)。

统一 Client 只接受以单个 `/` 开头、且不含反斜杠的同源绝对路径，请求使用 `same-origin` credentials/mode、`redirect: "error"` 与 `cache: "no-store"`。响应必须是可解析 JSON，并通过对应 API 模块的运行时 decoder；成功响应保留浏览器侧耗时和可用的 `X-Request-ID`，错误输出只使用公开 envelope 或前端稳定文案。

## 失败分类

前端必须把失败保持为六类，页面可以据此给出不同恢复提示，日志与 QA 也不能把它们合并成笼统的“接口报错”：

| `kind` | 判定边界 | 典型含义 |
| --- | --- | --- |
| `timeout` | Client 自己的 100ms～30s 有界计时触发并取消请求 | 上游未在本次等待预算内完成 |
| `cancelled` | 调用方 signal 取消，或组件刷新/卸载使旧请求失效 | 结果不应再覆盖当前页面状态，不等于服务故障 |
| `network` | Fetch 在没有 HTTP 响应的情况下失败 | 浏览器网络、DNS、TLS 或直接连接失败 |
| `gateway` | 开发/预览代理返回非 JSON 的 502、503 或 504 | 代理无法获得符合 API 契约的上游响应 |
| `http` | 后端返回非 2xx，并提供合法统一 JSON error envelope | 已到达应用；例如 Go `/ready` 返回的合法 JSON 503 仍是 `http`，不是 `gateway` |
| `contract` | 路径/timeout 输入非法，响应不是 JSON、JSON 无法解析、body 不符合运行时 schema，或 header/body 请求 ID 冲突 | 调用方配置错误、代理返回异常格式或前后端契约漂移 |

`gateway` 的判断刻意同时依赖状态码和非 JSON 媒体类型，避免把后端主动表达的 JSON 503 readiness 失败误判成代理故障。`timeout` 与 `cancelled` 也必须保持独立：前者意味着预算耗尽，后者通常是用户刷新、路由切换或组件生命周期产生的正常控制流。

## 记录规则

每一节涉及前端 API 时，在 `docs/api/lessons/lesson-XX.md` 增加或更新记录，并同步：

1. 课程正文中的本节交付和遗留问题；
2. `docs/course/status.csv`（章节完成时）；
3. 对应 QA 证据；
4. 必要的 ADR、Go 路由、请求/响应类型和前端 API 模块。

## API 状态

| 状态 | 含义 |
| --- | --- |
| `规划中` | 已确定需求，但尚未实现或验证 |
| `Mock` | 前端可通过本地演示数据运行，未调用 Go 服务 |
| `待验收` | 后端实现和契约记录已经存在，但全仓 QA 或人工核查尚未形成最终证据 |
| `已联调` | 前后端真实请求已经打通，但尚未完成完整验收 |
| `已验收` | 有 QA 命令或手工步骤证明契约和页面行为成立 |
| `已废弃` | 不再使用，必须记录替代接口或原因 |

## 单条接口记录字段

每条接口至少写明接口名称、HTTP 方法与路径、调用页面或组件、请求参数、成功响应、错误响应与错误码、幂等/鉴权/超时要求、当前状态和验证证据。

请求和响应示例必须来自当前代码或后端契约，不得凭记忆编写。未实现的接口只能标记为“规划中”，不能写成已经可调用；状态为“待验收”时也不能提前宣称章节已经通过。
