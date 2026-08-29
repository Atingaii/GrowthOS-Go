# 第 16 节 API 记录：Compose 部署消费契约

- **章节：** [Docker Compose 开发环境](../../course/part-02/lesson-16-docker-compose-development.md)
- **日期：** 2026-08-29
- **状态：** 已完成并验收
- **QA：** [第 16 节 QA 验收记录](../../qa/lessons/lesson-16.md)
- **架构决策：** [ADR-0012](../../decisions/ADR-0012-compose-development-topology.md)

## 1. 本节 API 变化

本节**没有新增、修改或删除任何 Go HTTP 业务路由**，也没有创建新的成功/错误 JSON schema。它把第 11～15 节已有的路由放入 Nginx + Compose 运行边界，并定义部署组件如何消费这些契约。

| 动作 | 接口 / 行为 | 消费方 | 当前状态 |
| --- | --- | --- | --- |
| 保持 | `GET /health`：Go 进程 liveness | Compose API health、浏览器系统状态页、M0 | 已接入 |
| 保持 | `GET /ready`：MySQL readiness | 浏览器系统状态页、readiness 负载检查 | 已接入 |
| 保持 | `/api`、`/api/...`：未来业务命名空间 | Nginx 同源代理 | 路径已代理，当前没有业务资源 |
| 新增部署端点 | `GET /container-health`：Nginx 自身 204 | Compose Web health | Nginx 本地端点，不是 Go API |
| 新增配置消费 | API/Migration 密码 `_FILE` 变量 | Compose Secret | 进程启动契约，不是 HTTP API |

“Nginx 已代理 `/api`”不能写成“业务 API 已实现”。未知 `/api/...` 仍由 Go 的统一 404 handler 返回 `route_not_found`，这是路由边界正确，不是对应资源已经存在。

## 2. 外部入口与内部入口

### 2.1 宿主机/浏览器入口

默认开发 origin：

```text
http://127.0.0.1:8088
```

端口可以通过 Make 变量覆盖，但 host 固定为 loopback。Compose 不发布 API、MySQL、Redis 或 Migration 端口。

### 2.2 Compose 内部入口

```text
web -> http://api:8080
api/migrate -> mysql:3306
```

`api:8080` 只在 `edge` 网络可由 Web 访问；`mysql:3306` 只在 internal `data` 网络可由 API 和 Migration 访问。浏览器不得知道或硬编码这些 service name。

### 2.3 同源映射

| 浏览器请求 | Nginx 行为 | Go 收到的 path | rewrite |
| --- | --- | --- | --- |
| `GET /health` | 代理到 `api:8080` | `/health` | 无 |
| `GET /ready` | 代理到 `api:8080` | `/ready` | 无 |
| 任意 method `/api` | 代理到 `api:8080` | `/api` | 无 |
| 任意 method `/api/...` | 代理到 `api:8080` | `/api/...` | 无 |
| `/health/...`、`/ready/...` | 不匹配探针代理规则 | SPA/static 路由 | 无 |

探针匹配为精确 `/health` 或 `/ready`，API 命名空间匹配 `/api` 或 `/api/` 边界。`/apiwhatever` 不会被误代理。

## 3. Nginx 同源代理约定

Nginx 对代理请求：

- 使用 HTTP/1.1 访问 upstream；
- 保留原始 request URI，包括合法 query；日志只记录规范化 path；
- 向 Go 传递原始 host 以及 `X-Real-IP`、`X-Forwarded-For`、`X-Forwarded-Proto`；
- 不为业务实现认证、授权、rate limit 或 CORS；
- 连接 upstream 的 timeout 为 2 秒，send/read timeout 为 4 秒；
- 继续传递 Go 的 `Content-Type` 与 `Cache-Control`；对 `X-Request-ID` 隐藏 upstream 同名头后只回写一次最终关联 ID；
- 为响应补充 `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY` 与 `Referrer-Policy`。

这些 forwarded headers 当前只用于保留部署上下文，不能自动视为可信客户端身份。真正面向公网时必须由可信代理边界覆盖/清洗，应用不能无条件信任任意调用方自行提交的 `X-Forwarded-For`。

## 4. Docker DNS 与 gateway failure

Nginx 使用 Docker 内置 resolver：

```nginx
resolver 127.0.0.11 valid=5s ipv6=off;
set $growthos_api_origin http://api:8080;
```

把 upstream 放入变量后，Nginx 可以在 API 容器 recreate、IP 改变时重新解析稳定 service name。它也允许 Nginx 在 API 暂时不存在时继续启动并提供静态页面。

API 不可连接或 upstream 超时时，Nginx 产生 502/504 等 gateway 响应。该响应不是 GrowthOS Go error envelope，前端继续按第 15 节约定分类为 `gateway`，不能伪造：

```json
{
  "error": {
    "code": "dependency_unavailable"
  }
}
```

`dependency_unavailable` 只属于 Go 明确生成的安全 503。代理层没有取得可信 Go body 时，只能陈述 gateway failure。不过 gateway 响应仍带一个由 Nginx 生成的 `X-Request-ID`，并与 Nginx access log 一致，所以 API-down 502 也可关联排障；该 ID 不伪装成 Go error body ID。

## 5. `GET /health`：API 容器 liveness

### 5.1 Compose 内部 healthcheck

API 容器从自身网络命名空间请求：

```http
GET /health HTTP/1.1
Host: 127.0.0.1:8080
```

成功响应契约保持：

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: <current-request-id>
```

```json
{
  "status": "ok",
  "version": "lesson-16",
  "timestamp": "2026-08-29T12:34:56.123456789Z"
}
```

Compose 使用 `/health`，因此 MySQL 在 API 已启动后中断不会把 API 容器判为 unhealthy。它证明 HTTP 进程还能处理请求，不证明依赖就绪。

### 5.2 浏览器入口

浏览器请求同一 path：

```http
GET /health HTTP/1.1
Host: 127.0.0.1:8088
Accept: application/json
```

Nginx 不重写 path。成功 JSON 和 Go `X-Request-ID` 语义不变；Nginx access log 使用 upstream request ID 关联 Go 日志。

## 6. `GET /ready`：MySQL readiness

Compose 为 API 设置：

```text
GROWTHOS_MYSQL_PING_TIMEOUT=2s
GROWTHOS_HTTP_WRITE_TIMEOUT=10s
```

正常响应仍为：

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
X-Request-ID: <current-request-id>
```

```json
{
  "status": "ready",
  "version": "lesson-16",
  "timestamp": "2026-08-29T12:34:56.123456789Z"
}
```

MySQL 不可用时：

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

Nginx read timeout 4 秒大于 API 的 2 秒 MySQL Ping timeout，Go 有机会先生成稳定 503；浏览器第 15 节的默认 5 秒预算又大于代理预算。这个顺序降低了客户端先超时、代理先 504而丢失服务端稳定错误的概率，但不是硬实时保证。

readiness 当前只 Ping MySQL。它不检查 Redis，因为 Redis 尚未进入业务；也不检查 Migration version、业务表、复制延迟、慢查询或数据一致性。

## 7. Nginx `GET /container-health`

Compose Web healthcheck 使用：

```http
GET /container-health HTTP/1.1
Host: 127.0.0.1:8080
```

响应：

```http
HTTP/1.1 204 No Content
Cache-Control: no-store
X-Request-ID: <nginx-request-id>
```

该端点由 Nginx 原地返回，不访问 API，不生成 GrowthOS JSON，也不进入普通 access log。它只证明 Nginx worker 正在监听。虽然默认 loopback 入口上也能访问这个 path，它不是业务或 Go API，不应被外部客户端用作整个平台健康判断。

## 8. MySQL Compose health 不是 HTTP API

MySQL healthcheck 读取 `mysql_app_password` Secret，并以 `growthos_app` 连接 `growthos` schema 执行：

```sql
SELECT 1;
```

输出必须严格为 `1`。这比只检查 TCP 端口或 daemon 存活多验证了：

- MySQL 初始化已能接受连接；
- API Secret 与数据库账号一致；
- `growthos_app` 账号存在；
- 目标 schema 可用；
- 最小 `SELECT` 权限有效。

它仍不替代 `/ready`：前者由 Compose 判断启动依赖，后者通过 API 当前连接池回答实例是否可接数据库流量。

## 9. Migration 的部署消费契约

`migrate` 不是 HTTP 服务。Compose 启动流程要求：

```text
mysql service_healthy
        -> growth-migrate up
        -> process exits 0
        -> api may start
```

当前内嵌 Migration 集为空，`up` 返回 `no_migrations` 并成功退出是预期行为。API 自身不执行 DDL，也不会在启动时隐式迁移。

如果 Migration 非零退出，`service_completed_successfully` 条件不成立，API 不应启动。修复后要先使用独立状态命令检查，不能把 API restart 当作 Migration 恢复手段。

## 10. `_FILE` 配置契约

### 10.1 API 密码

API 进程要求以下二者恰好一个：

```text
GROWTHOS_MYSQL_PASSWORD
GROWTHOS_MYSQL_PASSWORD_FILE
```

Compose 使用：

```text
GROWTHOS_MYSQL_PASSWORD_FILE=/run/secrets/mysql_app_password
```

### 10.2 Migration 密码

Migration 进程要求以下二者恰好一个：

```text
GROWTHOS_MYSQL_MIGRATION_PASSWORD
GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE
```

Compose 使用：

```text
GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE=/run/secrets/mysql_migration_password
```

文件读取规则：

- 文件路径变量不能为空白；
- 文件必须可打开、可完整读取和关闭；
- 读取总量有界，最终密码最多 1024 bytes；
- 只移除结尾 CR/LF，其他字符包括首尾空格保持原样；
- 空密码失败；
- 直接变量与 `_FILE` 同时存在失败；
- 错误只报告稳定变量名和规则，不公开密码或真实路径；
- `Load` 不读取 Migrator 密码，`LoadMigration` 不读取 API 密码，保持进程隔离。

这属于进程配置兼容性契约。未来 Secret manager sidecar 或 Kubernetes Secret 只要能提供受控文件，就可以复用 `_FILE` 消费边界；它不意味着当前 Compose file secret 已达到生产机密管理要求。

## 11. request ID 与代理日志关联

Go 继续为每次 `/health`、`/ready` 和 `/api/...` 请求建立 `X-Request-ID`，错误 body 的 `error.request_id` 与 response header 必须一致。

Nginx access log 和最终响应头使用同一个选择：

```text
upstream X-Request-ID exists -> log that ID
otherwise                    -> log Nginx $request_id
```

对代理响应，Nginx 先 `proxy_hide_header X-Request-ID`，再用 `add_header ... always` 统一写回最终值。这避免 Go 头与 Nginx 头重复：Go 正常/错误响应使用 upstream ID，静态响应与 API-down 502 使用 Nginx ID。日志记录 method 和 `$uri`，不记录包含 query string 的 `$request_uri`，也不记录 Referer；还记录最终 status、upstream status、bytes、总耗时与 upstream 耗时。request ID 用于关联，不是 authentication、authorization、不可伪造 trace 或业务幂等键。

Nginx 原生 error log 的请求级消息可能追加原始 request target 与 Referer，且格式不可像 access log 一样完全定制。当前将 error log 阈值设为 `crit`，把可操作的 upstream 失败信息保留在安全 access log。验收向 query 和 Referer 写入唯一 marker 后检索全部 Compose 日志，marker 均未出现。

MySQL driver 内部 logger 被设置为 `NopLogger`，避免原始 driver cause 直接绕过 JSON 日志写 stderr。HTTP/API 仍记录稳定 stage 和 request ID；数据库深度诊断必须走受控运维路径，不在通用 API 错误或容器日志中公开 DSN、账号、SQL 或内网地址。

## 12. 静态资源缓存与 API no-store

| 响应 | 缓存策略 | 原因 |
| --- | --- | --- |
| `/assets/<hashed-file>` | `public, max-age=31536000, immutable` | 文件名带构建 hash，可长期缓存 |
| `/index.html` | `no-store` | shell 需要及时引用当前构建资产 |
| `/container-health` | `no-store` | 容器当前事实 |
| `/health`、`/ready` 成功 | Go `no-store` | 当前实例事实 |
| Go error envelope | Go `no-store` | 错误与 request ID 属于当前请求 |
| gateway error | Nginx 默认错误，不是 Go 契约 | 前端分类 gateway，不将其当业务 JSON |

SPA 路由 fallback 不能吞掉 `/api`、`/health` 或 `/ready`；代理 location 的优先级和边界必须由冒烟验证。

## 13. 故障状态矩阵

| 场景 | Web health | 经 Web `/health` | 经 Web `/ready` | 解释 |
| --- | --- | --- | --- | --- |
| 全部正常 | healthy | 200 JSON | 200 JSON | API 存活且当前 MySQL ready |
| MySQL 运行中停止 | healthy | 200 JSON | 503 JSON `dependency_unavailable` | API 进程存活，数据库未就绪 |
| API 停止 | healthy | gateway 502/504 | gateway 502/504 | 静态入口可用，但未取得可信 Go 响应 |
| Redis 停止 | healthy | 200 JSON | 200 JSON | Redis 尚未进入业务依赖 |
| Web 停止 | 不可检查 | host 连接失败 | host 连接失败 | 唯一外部入口不可用，不代表内部进程自动停止 |
| Migration 启动失败 | Web 可独立启动 | API 尚未启动，gateway | API 尚未启动，gateway | DDL gate 阻止流量进程启动 |

数据库恢复后，`/ready` 应通过现有 API 进程恢复；API recreate 后，Web 应通过动态 DNS 恢复。Compose 本节没有自动 restart 依赖链，恢复行为必须来自正确的进程/连接池设计，而不是容器重启巧合。

## 14. M0 对 API 契约的检查

`make compose-smoke` 验证：

- MySQL/API/Redis/Web 正在运行且 healthy；
- Migration 已退出 0；
- `/health`、`/ready` 返回 200 JSON；
- `/` 返回 HTML；
- 未知 `/api/...` 返回 Go 404 JSON；
- 404 的 header/body request ID 一致；
- 只有 Web 发布配置的 loopback 端口。

`make compose-m0` 在 smoke 后固定运行：

```text
/health 100 RPS × 5m，2s timeout，P99 <= 100ms
/ready   20 RPS × 30s，2s timeout
```

任何 transport error、unexpected status、drop、未完成请求或 health P99 超限都会使命令失败。最终结果为：

| 目标 | scheduled / completed / success | errors / unexpected / dropped | 结果 |
| --- | --- | --- | --- |
| `/health` | `30000 / 30000 / 30000` | `0 / 0 / 0` | P50 `1.084208ms`、P95 `2.744875ms`、P99 `4.1495ms`、max `18.116291ms`、实际 `100.0027 RPS`；P99 100ms 门槛通过 |
| `/ready` | `600 / 600 / 600` | `0 / 0 / 0` | P50 `4.08525ms`、P95 `5.935083ms`、P99 `6.841375ms`、max `8.570541ms`、实际 `20.0276841 RPS` |

32 workers readiness 复测与最终 smoke 后，Web/API/MySQL/Redis 瞬时内存约为 `5.535/6.664/438/23.41 MiB`，Docker 配额为 `1.924 GiB`。这些值会随 allocator/cache 波动，只是当前单机开发入口的取样，不是生产容量、峰值或资源 limit；完整证据见[第 16 节 QA](../../qa/lessons/lesson-16.md)。

## 15. 安全边界

- 只有 `127.0.0.1` Web 端口发布，不默认接受局域网访问；
- Compose `expose` 不是宿主机发布；
- 不新增 CORS，浏览器只使用同源 path；
- Nginx 不记录 query/Referer，不回显 upstream 原始故障；
- Secret 不进入 HTTP、image、Compose environment 或日志；
- API 与 Migration 使用不同文件 Secret 和数据库账号；
- Redis 密码只挂入 Redis，Redis 不与 API 共享网络；
- read-only/cap-drop/non-root 降低运行面，但不替代认证、TLS、主机加固或镜像供应链安全；
- `/health`、`/ready` 和 `/container-health` 当前是本地开发可见端点，生产是否公开必须重新决策。

## 16. 与生产 API 网关的差距

当前 Nginx 只证明本地同源契约。生产仍需另行设计：

- TLS、可信代理链、host allowlist 与 forwarded header 清洗；
- 用户认证、权限、Cookie/Token、CSRF 和 rate limit；
- `/health`、`/ready` 的内部/外部暴露范围；
- 多实例 service discovery、load balancing、retry 和 outlier handling；
- 统一结构化 gateway error 与前端兼容策略；
- 请求/响应大小、上传、streaming、SSE/WebSocket timeout；
- 日志集中化、trace propagation、metrics 与敏感字段治理；
- 缓存、压缩、CSP/HSTS 等正式安全 header；
- 发布版本、回滚、canary 与 schema 兼容窗口。

## 17. 遗留问题

- 当前 `/api` 仍没有业务资源，首次业务路由由后续章节定义；
- Redis 尚未接入，不能在简历或面试中声称已实现分布式缓存；
- 当前 Migration 集为空，第 18 节才创建首个业务 schema；
- Nginx Docker DNS 配置只适用于当前 Docker bridge 环境，生产 discovery 需独立方案；
- 本节没有 OpenAPI、契约生成、认证或多实例测试；
- M0 是开发基线，不是业务容量或生产 SLO。

安全启动、停止和故障排查见[本地 Compose 运维手册](../../runbooks/local-compose.md)。

## 18. 官方参考

- [Docker Compose 启动顺序](https://docs.docker.com/compose/how-tos/startup-order/)
- [Docker Compose Secret](https://docs.docker.com/compose/how-tos/use-secrets/)
- [Compose service healthcheck 与安全属性](https://docs.docker.com/reference/compose-file/services/)
- [Docker bridge 网络与端口发布](https://docs.docker.com/engine/network/drivers/bridge/)
- [NGINX `proxy_pass`](https://nginx.org/en/docs/http/ngx_http_proxy_module.html#proxy_pass)
- [NGINX DNS resolver](https://nginx.org/en/docs/http/ngx_http_core_module.html#resolver)
