# 第 12 节 API 记录：请求关联与统一错误契约

- **章节：** [配置、日志与错误码体系](../../course/part-02/lesson-12-config-logging-errors.md)
- **日期：** 2026-08-29
- **状态：** 已完成
- **QA：** [第 12 节 QA 验收证据](../../qa/lessons/lesson-12.md)

## 本节 API 变化

| 动作 | 接口 / 行为 | 调用方 | 当前状态 |
| --- | --- | --- | --- |
| 修改 | 所有 HTTP 响应增加 `X-Request-ID` | 本地开发、未来前端和运维调用方 | 已验收 |
| 兼容增强 | `GET /health` 增加关联 header 和访问日志；成功 body 不变 | 本地开发；第 15 节起前端联调 | 已验收 |
| 修改 | 未匹配路由返回统一 404 envelope | 所有调用方 | 已验收 |
| 修改 | 不支持的方法返回统一 405 envelope | 所有调用方 | 已验收 |
| 修改 | 未知错误或可安全恢复的未写响应 panic 返回统一 500 envelope | 所有调用方 | 已验收 |

本节没有新增业务资源 API，React 页面仍使用 Mock 数据。

## 请求关联契约

### Header

```http
X-Request-ID: client-request-123
```

调用方可以不提供 `X-Request-ID`。服务端最终总会选择一个请求 ID，并在响应中返回：

```http
X-Request-ID: client-request-123
```

入站值只有同时满足以下条件才会保留：

- header 恰好出现一次；
- 长度为 1～64 个 ASCII 字节；
- 每个字符只能是英文字母、数字、`-`、`_`、`.` 或 `:`。

缺失、重复、过长或包含其他字符的值不会形成 4xx；服务端丢弃它并生成新的安全值。生成值是 opaque identifier，调用方不能依赖其长度、编码或前缀。

同一请求选定的值会进入 Gin context、标准 `context.Context`、响应 header、访问日志和错误 envelope。客户端应保存**响应**中的最终值，用于问题反馈和日志查询。

### 与 Trace 的边界

`X-Request-ID` 只是当前单个 HTTP 请求的应用级关联标识。本节不支持 `X-Trace-ID`，不生成 `trace_id`，也未接入 OpenTelemetry 或 `traceparent`。未来真实 `trace_id` 可以与 `request_id` 同时出现，但二者不是别名。

## `GET /health` 兼容性

请求路径、状态、缓存头和 JSON body 保持第 11 节契约，只增加 `X-Request-ID` 响应 header：

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: 0123456789abcdef0123456789abcdef
```

```json
{
  "status": "ok",
  "version": "dev",
  "timestamp": "2026-08-29T12:34:56.123456789Z"
}
```

成功响应不套统一 `data` envelope，也不新增 `request_id` JSON 字段。健康语义仍是无外部依赖的进程 liveness。

## 统一错误 envelope

```json
{
  "error": {
    "code": "route_not_found",
    "message": "resource not found",
    "request_id": "0123456789abcdef0123456789abcdef"
  }
}
```

| 字段 | 类型 | 必有 | 语义 |
| --- | --- | --- | --- |
| `error.code` | string | 是 | 小写稳定机器码；调用方只根据它分支 |
| `error.message` | string | 是 | 可以安全公开的人类可读英文消息，不作为程序判断条件 |
| `error.request_id` | string | 是 | 与响应 `X-Request-ID` 和服务端日志相同的请求关联值 |

envelope 不包含内部 `cause`、panic 值、stack、源码路径、配置原值或秘密。

## 当前通用错误

### 404：路由不存在

```http
HTTP/1.1 404 Not Found
Content-Type: application/json; charset=utf-8
X-Request-ID: request-404
```

```json
{
  "error": {
    "code": "route_not_found",
    "message": "resource not found",
    "request_id": "request-404"
  }
}
```

### 405：方法不允许

例如 `POST /health`：

```http
HTTP/1.1 405 Method Not Allowed
Content-Type: application/json; charset=utf-8
Allow: GET
X-Request-ID: request-405
```

```json
{
  "error": {
    "code": "method_not_allowed",
    "message": "method not allowed",
    "request_id": "request-405"
  }
}
```

### 500：内部错误

未知错误和在响应尚未写出前被 recovery 捕获的 panic 统一公开：

```http
HTTP/1.1 500 Internal Server Error
Content-Type: application/json; charset=utf-8
X-Request-ID: request-500
```

```json
{
  "error": {
    "code": "internal_error",
    "message": "internal server error",
    "request_id": "request-500"
  }
}
```

如果 handler 在 panic 前已经提交部分响应，HTTP 层无法安全改写为新的 JSON envelope；中间件会停止后续处理并记录受控错误事件。业务 handler 必须尽量在写响应前完成可能失败的工作。

## fault 到 HTTP 的映射

平台 fault 的 kind 与具体业务 code 分离。HTTP adapter 当前按 kind 选择状态：

| fault kind | HTTP 状态 |
| --- | ---: |
| `invalid` | 400 |
| `unauthenticated` | 401 |
| `forbidden` | 403 |
| `not_found` | 404 |
| `conflict` | 409 |
| `rate_limited` | 429 |
| `internal` 或未知错误 | 500 |

具体业务 fault 可以为同一 kind 使用不同稳定 code；code 必须匹配小写字母开头、仅包含小写字母/数字/下划线且不超过 64 字节。公开 message 必须是去除首尾空白、无控制字符且不超过 256 个 Unicode 字符的安全文本。

`fault.Error()` 只返回稳定 code，内部 cause 仅通过 Go 标准错误链显式访问；HTTP adapter、访问日志和 recovery 都不会因为格式化该错误而隐式泄露 cause。

## 日志关联与隐私

访问日志使用路由模板字段 `route`，不记录原始 query string 或请求/响应 body。未匹配路径统一记为 `unmatched`。请求日志只记录 `request_id`、`method`、`route`、`status` 和 `duration_ms` 这些请求维度：2xx/3xx 使用 Info，4xx 使用 Warn，5xx 使用 Error。

Recovery 不把 panic 值或 stack 直接写入当前日志，因为二者可能携带请求数据、凭据或内部实现。500 响应同样不会公开这些信息。后续若增加受控诊断存储，必须另行定义脱敏、访问和保留策略。

`net/http` 的底层 `ErrorLog` 也不会回退到全局 logger：未注入产品 logger 时默认丢弃；产品进程注入 `slog` 后只记录 `http_server_error` 与 `component=net/http`，不复制原始诊断内容。

## 前端调用边界

当前前端尚未读取 `X-Request-ID` 或 error envelope。第 15 节联调时，统一 HTTP client 应：

1. 保存响应 header 中的最终 request ID；
2. 根据 `error.code` 选择稳定行为；
3. 把安全 message 和 request ID 展示给用户或支持人员；
4. 不依赖 message 文案或服务端日志字段顺序。

这部分属于第 15 节交付，不是本节已经联调的能力。

## 验证方式

已通过的契约测试与真实进程冒烟覆盖：

- 合法入站 ID 保留，缺失/重复/非法 ID 被替换；
- context、响应 header、日志和错误 JSON 使用同一个最终值；
- 健康成功 body 与第 11 节完全兼容；
- 404、405、未知错误和 panic 的 status、code、message 与 envelope；
- 500 不包含内部 cause、panic 值或 stack；
- 当前日志没有虚构 `trace_id`。

命令、环境、真实响应和剩余风险见[第 12 节 QA 验收证据](../../qa/lessons/lesson-12.md)。

## 遗留问题

- 第 13 节为数据库接入补充配置和启动失败证据；
- 第 15 节让前端统一 HTTP client 消费 error code 与 request ID；
- 身份认证出现后补充 401/403 的真实契约测试；
- 第 94 节接入 OpenTelemetry 后增加真实 `trace_id`，不替换 request ID；
- 错误 code 注册表随实际业务 fault 渐进增加，不能预制完整清单。
