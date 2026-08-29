# 第 11 节 API 记录：Gin 健康接口

- **章节：** [使用 Gin 初始化 Go Web 服务](../../course/part-02/lesson-11-gin-http-service.md)
- **日期：** 2026-08-29
- **状态：** 已验收
- **QA：** [第 11 节 QA 验收证据](../../qa/lessons/lesson-11.md)

## 本节 API 变化

| 动作 | 接口 | 调用方 | 当前状态 |
| --- | --- | --- | --- |
| 新增 | `GET /health` | 本地开发、自动化测试；第 15 节起供前端联调状态使用 | 已验收 |

本节没有活动、抽奖、权益或其他业务 API，也没有修改前端 Mock 数据。

## `GET /health`

### 用途

确认请求能够到达当前 GrowthOS Go 进程并由 Gin handler 正常响应。当前语义是进程 liveness，不检查尚未接入的数据库、Redis、MQ 或业务模块。

### 请求

```http
GET /health HTTP/1.1
Host: 127.0.0.1:8080
Accept: application/json
```

- **Path 参数：** 无
- **Query 参数：** 无；未知参数不参与响应语义
- **请求体：** 无
- **鉴权：** 无
- **幂等性：** 只读且幂等

### 成功响应

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
```

```json
{
  "status": "ok",
  "version": "dev",
  "timestamp": "2026-08-29T12:34:56.123456789Z"
}
```

| 字段 | 类型 | 必有 | 语义 |
| --- | --- | --- | --- |
| `status` | string | 是 | 当前固定为 `ok`，只表示进程 handler 能响应 |
| `version` | string | 是 | 构建标签；未注入发布标签时为 `dev` |
| `timestamp` | string | 是 | 响应生成时的 UTC RFC3339Nano 时间 |

`version=dev` 不代表正式版本。构建入口支持通过 linker flag 注入标签，具体标签由发布流程决定。

### 错误响应与错误码

本接口当前没有业务输入和外部依赖，handler 没有定义业务错误码。无法监听或服务进程失败时，错误由进程退出状态表达，而不是伪造成健康响应。

第 12 节才建立统一 HTTP 错误结构。Gin 对未知路径产生的默认 404 body 不是本节承诺的稳定错误契约。

`POST /health` 等不支持的方法当前返回 405；其响应体同样要等第 12 节统一错误契约后才稳定。

### 路径版本策略

规范路径只有 `/health`。健康检查描述进程/部署单元，不随营销业务 API 版本变化，所以本节不注册 `/api/v1/health`。后续版本化业务资源使用 `/api/v1/...`，不会为本接口创建重复别名。

### 超时与重试

- handler 不访问外部依赖；调用方应设置自己的短请求超时；
- 只读请求可以在连接失败时有限重试，但重试策略属于调用方/探针配置；
- 服务接到终止信号后最多使用 5 秒关闭窗口处理在途请求；该窗口不是单请求 SLO；
- 本节未进行性能或可用性实测，不把一次本地请求结果写成 SLO 证据。

### 前端调用边界

当前 React 页面继续读取 Mock 数据，没有调用本接口。第 15 节首次联调时应新增前端请求封装、加载/失败状态和联调证据；该后续变化必须在第 15 节 API 记录中登记。

## 验证方式

自动化契约测试应覆盖：

- `GET /health` 返回 200 和 JSON Content-Type；
- 响应带有 `Cache-Control: no-store`，避免缓存把旧时间和旧构建标签伪装成当前进程状态；
- 响应包含 `status`、注入的 `version` 和可确定的 UTC RFC3339Nano `timestamp`；
- `/api/v1/health` 未注册并返回 404；
- 不支持的健康接口方法返回 405；
- context 取消触发优雅关闭，监听/Serve/Shutdown 错误不被吞掉。

实际执行命令和最终结果见[第 11 节 QA 验收证据](../../qa/lessons/lesson-11.md)。

## 遗留问题

- 第 12 节集中管理监听地址、关闭预算等配置，并建立日志与错误码体系；
- 第 15 节让前端通过真实请求使用该接口；
- 接入必需依赖后重新设计 readiness，不能直接扩大当前 `status=ok` 的语义；
- 部署探针与 Compose 配置到对应部署章节再登记。
