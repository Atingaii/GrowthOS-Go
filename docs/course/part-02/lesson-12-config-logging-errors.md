# 第 12 节：配置、日志与错误码体系

**状态：** 已完成并验收

**日期：** 2026-08-29

**阶段：** Go + React 从零搭建

**本节统一进程配置、结构化日志、请求关联和 HTTP 错误语义；不连接数据库、中间件或真实业务 API，也不把 `request_id` 误写成已经接入 OpenTelemetry。**

## 1. 本节工程诉求

第 11 节已经有可启动、可关闭的 Gin 进程和 `GET /health`，但它仍把地址与超时留在代码默认值中，入口只用标准 `log.Printf` 输出最终错误，Gin 的 404、405 和 panic 响应也没有统一的机器可读结构。

这在只有一个健康接口时看似够用，一旦第 13 节开始接入数据库、后续加入业务用例，就会出现四类漂移：

1. 不同环境通过修改代码改变地址、超时或日志级别；
2. 日志是不可稳定查询的自然语言，无法按一次请求关联；
3. HTTP 状态、内部错误和值得给调用方的稳定 `code` 混在一起；
4. panic、未知路径和业务失败返回不同形状，前端只能猜测。

第 12 节先建立技术横切面的共同语言，让下一节数据库接入和后续业务 API 都进入同一边界。

## 2. 上一版状态与目标链路

上一版请求链路是：

```text
请求 -> Gin router -> handler -> JSON 成功响应
          ├─ 未知路径：Gin 默认 404
          ├─ 错误方法：Gin 默认 405
          └─ panic：Gin Recovery 默认行为
```

本节目标链路是：

```text
GROWTHOS_* -> appconfig -> 类型化 Config
                              ├─> slog logger
                              └─> httpserver

HTTP request
  -> request_id middleware
  -> access log / recovery
  -> router / handler
  -> HTTP fault mapper
  -> 稳定 error envelope
```

配置、日志和 HTTP 错误都属于进程平台边界，不是 Marketing、Lottery 或 Benefit 领域模型。

## 3. 本节范围与非目标

### 3.1 交付范围

- 只从统一的 `GROWTHOS_` 环境变量边界加载类型化配置，并提供安全默认值和无秘密示例；
- 在监听前校验全部已知配置，聚合错误且不回显原值；
- 使用标准库 `log/slog` 输出结构化启动、停止、请求和异常日志；
- 为每次 HTTP 请求建立 `request_id`，在上下文、响应头、错误响应和日志之间关联；
- 建立统一错误 envelope 和稳定 `code`；
- 把与传输无关的 fault 和 HTTP 映射分开；
- 让 404、405、500 都遵守同一错误结构，同时保持健康接口成功 body 不变；
- 用单元、HTTP 契约和进程级测试保护边界。

### 3.2 明确不做

- 不连接 MySQL、不运行 Migration；
- 不连接 Redis、RabbitMQ、RocketMQ 或配置中心；
- 不实现动态配置刷新、日志采集平台或远程日志传输；
- 不实现用户认证、权限、限流或业务错误全量清单；
- 不生成 OpenTelemetry span，不接受或传播 `traceparent`，也不把 `request_id` 命名为 `trace_id`；
- 不修改 `GET /health` 的成功 JSON 字段；
- 不让 React 提前切换真实 API，首次联调仍在第 15 节。

## 4. 方案分析

### 4.1 配置方案

| 方案 | 优点 | 当前问题 | 结论 |
| --- | --- | --- | --- |
| 各包自行 `os.Getenv` | 编写最快 | 默认值、解析、测试和秘密处理分散，无法一次报告错误 | 不采用 |
| 立即引入配置中心 | 可动态发布 | 当前只有单进程本地开发，没有远程配置需求和治理能力 | 不采用 |
| 集中 `appconfig` + `GROWTHOS_` 环境变量 | 容器友好、可测试、依赖少，后续可替换输入适配器 | 需要维护明确配置表 | 采用 |

配置加载和配置使用必须分开。只有 `appconfig` 读取进程环境；HTTP Server、日志和未来数据库适配器接收已经校验的类型化值。

### 4.2 日志方案

本节采用 Go 标准库 `log/slog`。相比继续拼接字符串，它提供稳定字段、级别和 JSON/text handler；相比立即引入第三方日志框架，它减少早期依赖，也能通过注入 `*slog.Logger` 保持调用方与具体输出目标解耦。

结构化不等于“把一切都写进日志”。日志字段必须有查询价值且符合隐私边界，详细规则见第 7 节。

### 4.3 错误方案

| 方案 | 问题 |
| --- | --- |
| 直接把 `err.Error()` 返回给客户端 | 泄露内部实现、路径或未来秘密，文案变化也会破坏调用方 |
| 只返回 HTTP 状态 | 前端无法稳定区分同一状态下的不同业务失败 |
| 业务包直接创建 Gin JSON | 领域与传输耦合，换 RPC 或任务入口时无法复用语义 |
| fault + HTTP mapper + error envelope | 业务语义稳定，传输层独立决定状态与公开信息；采用 |

## 5. `GROWTHOS_` 配置边界

当前配置覆盖运行环境、HTTP 地址与五个生命周期 timeout、日志级别和日志格式。完整键、默认值、上限与无秘密示例以[配置参考](../../configuration.md)为准。

关键规则：

1. 未设置时才使用默认值；设置为空属于错误；
2. duration 必须可解析、严格大于零且不超过各自安全上限；
3. 枚举严格小写；监听地址必须包含 1～65535 的合法端口；
4. 启动时尽量聚合多个配置错误，减少一次只修一个的反馈循环；
5. 错误信息只写变量名和约束，不写原值；
6. `configs/growth-api.env.example` 只能含公开示例，不能出现凭据。

入口先创建只含 `service` 与 `version` 的最小 bootstrap logger，以便安全报告配置拒绝；配置加载完成后才创建带 `environment`、级别和格式的正式 logger、router 与监听器。非法配置必须让进程非零退出，而不是以部分默认值继续运行。

## 6. `slog` 结构化日志

日志需要覆盖两类事件：

| 类型 | 典型事件 | 建议稳定字段 |
| --- | --- | --- |
| 进程日志 | 服务启动、正常停止、配置/启动/关闭失败 | `service`、`environment`、`version`、`http_address`、`shutdown_timeout_ms`、必要时 `error` |
| 请求日志 | 请求完成、panic 恢复 | `request_id`、`method`、`route`、`status`、`duration_ms` |

字段名属于日志查询契约；一旦用于 Dashboard 或告警，改名需要迁移计划。日志消息说明“发生了什么”，字段提供可聚合维度，不能把所有信息重新拼回一条字符串。

级别边界：

- `debug`：只在开发诊断需要时开启，不输出请求体或秘密；
- `info`：正常启动、正常停止以及 2xx/3xx 请求摘要；
- `warn`：4xx 调用方错误和未匹配路径；
- `error`：5xx 请求、启动失败、panic、Server 异常或系统不可继续处理的故障。

日志格式由配置选择，但 JSON 与 text 必须表达同一组语义字段。测试不应依赖字段输出顺序。

`net/http` 的底层诊断另设隐私边界：如果调用方没有注入产品 logger，`httpserver` 的 `ErrorLog` 使用显式 discard logger，不回退到进程全局标准 logger；产品进程注入 `slog` 后，桥接器只记录 `msg=http_server_error` 和 `component=net/http`，不复制可能包含 panic、stack、请求数据或秘密的原始诊断字符串。可安全向上传播的 Serve 错误仍由入口按正常错误链处理。

## 7. `request_id` 与隐私边界

每个进入 Gin 的请求必须有一个 `request_id`：`X-Request-ID` 只有恰好出现一次、长度为 1～64 个 ASCII 字节且只含字母、数字、`-_.:` 时才会保留；缺失或不合格时生成新值。最终值写入 Gin context、标准 `context.Context` 和响应 header。后续日志与错误响应只能读取这个已确认值，不能各自重新生成。

### 7.1 `request_id` 不是 `trace_id`

| 标识 | 当前/未来职责 | 生成与传播 |
| --- | --- | --- |
| `request_id` | 当前单个 HTTP 请求的应用级关联标识 | 第 12 节中间件建立，通过响应头、日志和错误 envelope 暴露 |
| `trace_id` | 未来 OpenTelemetry 分布式 Trace 中多个 span 的共同标识 | 由 OTel SDK 和标准传播器创建/解析；当前尚未实现 |

未来接入 OpenTelemetry 后，日志可以同时包含 `request_id` 与 `trace_id`，也可以把 `request_id` 作为 span attribute，但不能复制字符串后声称二者等价。当前日志中不得输出虚构的 `trace_id` 字段。

### 7.2 不进入日志的内容

- `Authorization`、Cookie、Token、密码、私钥和完整连接串；
- 请求与响应 body；
- 未脱敏手机号、邮箱、证件号、设备标识或完整用户资料；
- 未经控制的完整 query string；
- panic 值、panic stack、内部 cause 以及任意配置原值；
- 仅为了“方便排查”而复制的模型 Prompt 或业务载荷。

客户端提供的请求 ID 也是不可信输入，必须限制格式和长度，避免日志注入或无限基数。具体接受规则以实现和测试为准。

## 8. fault 与 HTTP 适配器解耦

fault 表达与传输无关的失败语义，至少包含稳定 `code`、安全公开消息和可选内部 cause。它不能导入 Gin、`net/http` 或写响应。`fault.Error()` 只渲染稳定 code；cause 只通过 `Unwrap`、`errors.Is` / `errors.As` 显式访问，不会被默认错误字符串、HTTP mapper 或 recovery 隐式记录。

HTTP 适配器负责：

1. 判断错误是否为已知 fault；
2. 将 fault 类别映射成 HTTP status；
3. 选择可以公开的稳定 `code` 和 message；
4. 把当前 `request_id` 写入统一 envelope；
5. 将内部 cause 保留在服务端错误链中；只有经过隐私评估的调用方才能选择记录，HTTP 适配器不会把它返回客户端或自动写入访问日志；
6. 对未知错误统一降级为 500。

```text
application/domain error
          │
          ▼
     platform fault      （不认识 HTTP）
          │
          ▼
  httpapi error mapper   （状态与公开 envelope）
```

同一个 fault 以后可以映射到 HTTP、gRPC、任务失败或 MCP Tool error，而不要求业务层改写语义。

## 9. HTTP 错误契约

第 12 节的错误响应统一为 JSON envelope：

```json
{
  "error": {
    "code": "route_not_found",
    "message": "resource not found",
    "request_id": "01..."
  }
}
```

稳定的是 `code` 和字段结构，不是给人阅读的 message。调用方不得根据 message 文案分支。fault code 必须是小写字母开头、后续仅含小写字母/数字/下划线且最多 64 字节；公开 message 为 1～256 个 Unicode 字符，不能含首尾空白或控制字符。

### 9.1 404、405 与 500

| 场景 | HTTP 状态 | 契约要求 |
| --- | ---: | --- |
| 路由不存在 | 404 | `route_not_found` / `resource not found` / 当前 request ID |
| 方法不允许 | 405 | `method_not_allowed` / `method not allowed` / 当前 request ID |
| panic 或未知内部错误 | 500 | `internal_error` / `internal server error`；响应不含 panic、stack 或 cause |

500 与响应中的 `request_id` 关联。当前 recovery 为保护隐私，只记录安全事件和 request ID，不复制可能包含请求数据或秘密的 panic 值与 stack。即使 Recovery 已经处理 panic，尚未提交的响应也不能被误写成成功；若 panic 前已经提交部分响应，则只能停止处理，不能再追加第二个 envelope。

### 9.2 健康接口兼容性

`GET /health` 的成功 body 保持第 11 节契约，不套 `data` 或其他成功 envelope：

```json
{
  "status": "ok",
  "version": "dev",
  "timestamp": "2026-08-29T12:34:56.123456789Z"
}
```

第 12 节已为响应增加请求 ID header，并记录访问日志，但没有改名、删除或嵌套这三个 JSON 字段。`POST /health` 进入新的 405 错误 envelope；未知路径进入新的 404 envelope。

## 10. 工程边界

第 12 节把职责落实在以下边界：

```text
internal/platform/appconfig   # 环境变量、默认值、类型化配置与校验
internal/platform/logging     # logging.New：slog handler、级别、格式与基础字段
internal/platform/fault       # kind、稳定 code、公开 message 与内部 cause
internal/infrastructure/httpapi
  # X-Request-ID、访问日志、recovery、error mapper、404/405
internal/infrastructure/httpserver
  # 使用已校验地址与 timeout，桥接脱敏 ErrorLog，不读取环境变量
cmd/growth-api
  # 加载配置、装配 logger/router/server、决定进程退出
configs/growth-api.env.example
```

`cmd` 可以决定启动失败如何退出，但不解析每个变量或拼装每种 HTTP 错误。平台包提供机制，不得承载营销业务规则。

## 11. 测试策略

| 风险 | 实际自动化证据 |
| --- | --- |
| 默认值与环境覆盖漂移 | 表驱动测试全部 `GROWTHOS_` 键 |
| 空值、非法枚举、非法地址或 duration 被接受 | 单项与聚合配置错误测试；断言不回显原值 |
| 日志格式/级别不符 | 内存 buffer 解码 JSON/text，按字段而非顺序断言 |
| 请求 ID 在层间丢失 | 入站、自生成、响应头、日志与错误 envelope 关联测试 |
| 把 request ID 当 trace ID | 结构和日志测试确认当前不虚构 `trace_id` |
| 404/405 仍返回 Gin 默认 body | 路由契约测试 |
| panic 泄露或错误返回 200 | recovery 测试断言 500、安全 envelope 和关联日志 |
| fault 与 HTTP 耦合 | 包依赖评审与 mapper 单元测试 |
| 健康成功响应破坏兼容 | 保留第 11 节响应字段的回归测试 |
| 全仓回归 | Race 测试、`go vet`、`make verify` |

自动化测试使用内存日志目标、`httptest` 和受控环境读取函数，不依赖开发者真实环境，也不修改全局 logger 后并行运行。六个目标包的普通测试与 Race 测试、全仓测试、静态检查和统一门禁均已通过，精确命令与环境见第 12 节 QA 证据。

## 12. 数据库与外部基础设施变化

**无。**

配置体系具备承载未来数据库参数的能力，不代表数据库已经接入。本节没有 DSN、连接池、Migration、Redis 或 MQ 配置，也不探测本机 Docker Desktop 的容器。

> 后续状态：第 13 节已经在这条类型化边界上加入 MySQL、独立 Migration 配置、连接池和 readiness；这不改变“第 12 节本身没有数据库变化”的历史事实。见[第 13 节正文](lesson-13-mysql-migrations.md)。

## 13. API 变化

- `GET /health` 成功 JSON 保持不变；
- 所有响应增加 `X-Request-ID` 请求关联 header；合法入站值保留，缺失或非法值由服务端替换；
- 404、405、500 从 Gin 默认行为升级为统一 JSON error envelope；
- 没有新增业务资源 API。

精确字段、code 与兼容边界见[第 12 节 API 记录](../../api/lessons/lesson-12.md)。

## 14. QA 验收

最终验收已覆盖配置默认/覆盖/失败、日志结构和隐私、request ID 传播、三类 HTTP 错误、健康成功兼容、进程启动失败、信号关闭与全仓门禁，见[第 12 节 QA 验收证据](../../qa/lessons/lesson-12.md)。

## 15. 学习分支

本节位于 `codex/lesson-12-config-logging-errors`，直接基于第 11 节已验收 tip；实现提交 `60a6116` 已推送至同名远端分支，完整章节以最终分支 tip 为准。具体状态见[课程分支检查点](../branch-checkpoints.md)。

## 16. 本节复盘

配置、日志和错误不是业务功能，却决定后续每个业务功能是否可部署、可定位、可兼容。本节的核心不是增加更多字符串，而是建立三种稳定边界：配置先验证再注入，日志用字段关联但不泄露隐私，失败先表达业务 fault 再由 HTTP 适配器映射。

这一基线仍然克制：只有单进程 `request_id`，没有分布式 Trace；只有无秘密环境示例，没有秘密管理系统；只有通用 HTTP 错误，没有凭空设计全部业务 code。

## 17. 下一节遗留问题

进程已经能够可靠加载配置、记录请求并返回稳定错误，但还没有持久化事实。第 13 节需要回答：

> 如何在不泄露数据库凭据的前提下接入 MySQL、配置并验证连接池、选择前向 Migration 机制，并让启动失败、迁移失败和健康/readiness 语义保持可诊断？

该问题已由[第 13 节：接入 MySQL 与 Migration](lesson-13-mysql-migrations.md)完成：API/Migrator 身份分离，`/health` 与 `/ready` 分离，前向命令只暴露 `up/status`，首个业务 Migration 仍保留到第 18 节。
