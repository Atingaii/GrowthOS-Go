# 第 13 节 API 记录：MySQL readiness

- **章节：** [接入 MySQL 与 Migration](../../course/part-02/lesson-13-mysql-migrations.md)
- **日期：** 2026-08-29
- **状态：** 已完成并验收
- **QA：** [第 13 节 QA 验收记录](../../qa/lessons/lesson-13.md)

## 本节 API 变化

| 动作 | 接口 / 行为 | 调用方 | 当前状态 |
| --- | --- | --- | --- |
| 保持 | `GET /health` 继续表示无外部依赖的进程 liveness | 平台、本地开发；第 15 节起前端 | 已验收 |
| 新增 | `GET /ready` 通过有界 MySQL Ping 表示是否可接收数据库流量 | 平台、本地开发；第 15 节起前端 | 已验收 |
| 新增 | readiness 失败映射为 503 `dependency_unavailable` | 所有 HTTP 调用方 | 已验收 |

本节没有新增业务资源 API。`growth-migrate up/status` 是本地/发布命令，不是 HTTP API，也不应从浏览器调用。

## `GET /health`：liveness 保持不变

```http
GET /health HTTP/1.1
Host: 127.0.0.1:8080
X-Request-ID: health-check-13
```

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: health-check-13
```

```json
{
  "status": "ok",
  "version": "dev",
  "timestamp": "2026-08-29T12:34:56.123456789Z"
}
```

成功 body 仍严格使用 `status`、`version`、`timestamp` 三个字段，不增加数据库状态。数据库在进程启动后发生故障时，liveness 仍可成功，从而避免平台把依赖故障误判为进程崩溃并反复重启。

API 启动阶段仍会先打开并 Ping 数据库；首次 Ping 失败时进程不监听。因此“启动成功后的 `/health` 不探测数据库”和“数据库不可连接时拒绝启动”并不矛盾。

## `GET /ready`：readiness

### 数据库可用

```http
GET /ready HTTP/1.1
Host: 127.0.0.1:8080
X-Request-ID: ready-check-13
```

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: ready-check-13
```

```json
{
  "status": "ready",
  "version": "dev",
  "timestamp": "2026-08-29T12:34:56.123456789Z"
}
```

| 字段 | 类型 | 语义 |
| --- | --- | --- |
| `status` | string | 成功时固定为 `ready` |
| `version` | string | 当前构建标签；空值归一化为 `unknown` |
| `timestamp` | RFC3339Nano string | handler 生成响应时的 UTC 时间 |

### 数据库不可用或 checker 无效

```http
HTTP/1.1 503 Service Unavailable
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: ready-check-13
```

```json
{
  "error": {
    "code": "dependency_unavailable",
    "message": "service unavailable",
    "request_id": "ready-check-13"
  }
}
```

readiness 不返回数据库地址、账号、DSN、SQL、驱动 error number 或 `err.Error()`。服务端访问日志仍只使用第 12 节安全请求维度；客户端根据稳定 `error.code` 分支，不能解析 message。

`POST /ready` 不执行 Ping，沿用统一 405 契约：返回 `method_not_allowed` error envelope、当前请求 ID 和 `Allow: GET`。

## timeout 与取消

readiness 使用请求 context 派生有界 Ping：默认 `3s`，最大 `30s`。客户端取消请求时，取消会向数据库 Ping 传播。配置必须满足：

```text
GROWTHOS_MYSQL_PING_TIMEOUT + 1s response budget
  <= GROWTHOS_HTTP_WRITE_TIMEOUT
```

固定 1 秒为 503 JSON、header 与调度保留空间。非法组合在进程监听前被配置加载器拒绝，不等请求时才暴露。

## 与流量平台的使用约定

| 目的 | 使用端点 | 不应使用 |
| --- | --- | --- |
| 判断进程是否存活、是否需要重启 | `/health` | `/ready` 失败不应直接等同进程崩溃 |
| 判断实例能否接收依赖数据库的请求 | `/ready` | `/health` 不能替代依赖检查 |
| 业务正确性、Migration 版本或数据一致性 | 后续专门指标/检查 | 两个端点都不提供这些证明 |

当前仓库尚未提交 Compose 或 Kubernetes probe 配置；第 16 节再根据本契约接入开发环境。

## 前端调用边界

React 页面仍全部使用 Mock 数据。第 15 节的系统状态页和统一 HTTP client 可以首次消费 `/health`、`/ready`、`X-Request-ID` 与 error envelope；本节不声称浏览器联调已经完成。

## 验证状态

路由、成功/失败响应、timeout、取消、typed-nil checker、request ID、日志级别和隐私均已通过自动化测试。真实 MySQL 8.4.11 中，`/ready` 正常返回 200；锁定并 kill API 数据库连接后，`/ready` 返回安全 503，而同一进程的 `/health` 仍返回 200。真实响应、非 skip 集成命令和清理证据见[第 13 节 QA](../../qa/lessons/lesson-13.md)。

## 遗留问题

- 第 15 节让 React 首次实际调用健康/readiness；
- 第 16 节在 Compose 中配置依赖顺序与健康检查；
- 第 18～21 节出现业务表和 API 后，readiness 仍只检查连接，不执行高成本业务查询；
- 后续增加 Redis、MQ 等依赖时，需要逐一决定它们是否影响接流量，不能把所有依赖机械塞入一个探针。
