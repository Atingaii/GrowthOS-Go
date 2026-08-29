# ADR-0009：配置、日志、请求关联与错误适配边界

- **状态：** 已接受
- **日期：** 2026-08-29
- **负责人：** GrowthOS 维护者

## 背景

第 11 节建立了最小 Gin 进程。第 12 节需要把硬编码运行参数、标准 `log` 输出和 Gin 默认错误行为升级为后续业务 API 可以共同使用的运行时边界。

如果现在不固定边界，容易出现：业务包直接读环境变量、每个 handler 自己拼日志与 JSON、客户端把 message 当稳定判断条件、内部 cause 泄露给调用方，以及 `request_id` 被误命名成尚未存在的分布式 Trace。

当前仍是单 Go 进程，没有配置中心、日志 SDK、OpenTelemetry Collector 或真实业务 fault。决策需要足够支撑第 13 节后的实现，又不能借未来规划提前引入 Viper、Zap、Nacos 或 OTel。

## 评估过的方案

### 配置

| 方案 | 优点 | 当前问题 | 结论 |
| --- | --- | --- | --- |
| 各包直接读取环境变量 | 局部代码少 | 默认值、校验和测试分散，业务代码绑定部署输入 | 不采用 |
| Viper 统一文件、环境和远端输入 | 功能完整 | 当前没有多格式文件、热更新或远端来源需求，隐式优先级和依赖面过大 | 当前不采用 |
| 显式 `GROWTHOS_` 环境变量 + 类型化 Config | 输入、默认值、校验和错误都可测试，容器注入清楚 | 新增配置需维护显式映射 | 采用 |

### 日志

| 方案 | 优点 | 当前问题 | 结论 |
| --- | --- | --- | --- |
| 继续使用标准 `log` 拼字符串 | 无迁移成本 | 字段不可稳定查询，关联信息依赖文案 | 不采用 |
| 引入 Zap | 性能和生态成熟 | 当前吞吐没有证明标准库不足，引入额外 API 与适配成本 | 当前不采用 |
| 标准库 `log/slog` + 注入 logger | 结构化、级别和 handler 足够，依赖最小 | 后续需自行约束公共字段 | 采用 |

### 请求关联与错误

| 方案 | 问题 | 结论 |
| --- | --- | --- |
| 自定义 `X-Trace-ID`，值由请求中间件生成 | 在没有 OTel span 时伪造 Trace 概念，未来还要迁移标准传播 | 不采用 |
| `request_id` 与未来 OTel `trace_id` 分离 | 当前语义真实，未来可同时关联日志与 span | 采用 |
| 业务错误直接写 Gin status/JSON | 领域与 HTTP 耦合，无法复用到 gRPC、任务或 MCP | 不采用 |
| fault 表达语义，HTTP adapter 映射状态和公开 envelope | 业务 code 稳定，传输策略可替换，内部 cause 可保密 | 采用 |

## 决策

1. 应用自有环境变量统一使用 `GROWTHOS_` 前缀，由 `internal/platform/appconfig` 集中读取并产出类型化 Config。未设置才使用默认值；设置为空或非法时启动失败。
2. 示例配置文件只列无秘密值且不会被程序自动加载。秘密由 shell、容器或部署 Secret 显式注入，不进入 Git。
3. 使用标准库 `log/slog`，由入口按已校验的级别和格式构造 logger，再显式注入 HTTP 中间件和其他组件。业务包不依赖全局默认 logger 形成隐式状态。
4. 每个 HTTP 请求使用 `request_id` 做应用级关联，并通过约定 header、上下文、日志和错误 envelope 传播。客户端输入必须经过格式与长度校验。
5. 当前不生成 `trace_id`，也不使用自定义 `X-Trace-ID`。未来 OpenTelemetry 使用标准 propagator 和 SDK 产生真正的 trace/span 标识；日志可以同时记录 `request_id` 与 `trace_id`，二者不互相冒充。
6. fault 层只表达稳定 code、安全公开消息和可选内部 cause，不导入 Gin 或 `net/http`。`fault.Error()` 只渲染稳定 code，cause 仅通过标准错误链显式访问，不会被默认错误字符串隐式带入日志；HTTP 适配器负责 status、404/405/500 和统一 JSON envelope。
7. 未知错误和 panic 对外只返回稳定 internal code 与通用消息。原始 `err.Error()`、stack、配置值、路径和其他内部 cause 不能写入响应或通用访问日志；只有经过隐私评估的调用方才能选择把必要诊断写入受控服务端日志。
8. `net/http` 的 `ErrorLog` 不得回退到进程全局标准 logger。没有注入产品 logger 时默认丢弃原始诊断；注入后通过 `slog` 桥接，但只输出 `http_server_error` 与 `component=net/http`，不复制可能含 panic、stack 或秘密的原始消息。

## 未来演进

### Nacos

第 76 节若引入 Nacos，它应作为新的**配置来源适配器**，读取远端值后仍经过同一类型化结构、校验和秘密边界。届时需要明确本地默认、进程环境、远端配置的优先级、版本和失败回退，不能让业务包直接调用 Nacos SDK，也不能绕过启动校验。

### OpenTelemetry

第 94 节引入 OTel 时，在 HTTP/RPC/MQ 边界增加标准 context propagation 和 span，不替换当前 fault 或配置模型。访问日志从 context 读取真实 `trace_id`/`span_id` 作为新增字段，同时保留 `request_id` 供客户端支持和单次 HTTP 关联。

### 日志实现

若实测证明 `slog` handler 在吞吐、编码或采集生态上不能满足目标，可以替换 handler 或增加适配器；调用方继续依赖 `*slog.Logger` 与稳定字段语义，不应把 Zap 专有 API 扩散到业务层。

## 影响

- 新配置需要同时修改类型、解析校验、示例、配置参考和测试，显式维护成本换取可追踪性。
- 请求、错误响应和日志可以通过同一个 `request_id` 关联，但这不是分布式追踪能力。
- HTTP code/status 映射集中，未来业务 fault 不需要了解 Gin。
- 500 响应更稳定且不泄露内部 cause；诊断依赖受控结构化日志和 request ID。
- 当前不增加 Viper、Zap、Nacos 或 OpenTelemetry 依赖，降低第 12 节的变更与供应链范围。
- 本决策不引入 MySQL、Redis、消息队列或任何业务能力。
