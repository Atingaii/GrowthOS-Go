# 第 12 节 QA 验收证据

- **日期：** 2026-08-29
- **产物：** [配置、日志与错误码体系](../../course/part-02/lesson-12-config-logging-errors.md)
- **分支：** `codex/lesson-12-config-logging-errors`
- **实现提交：** `60a6116`（`feat: add lesson twelve runtime boundaries`，已推送至同名 `origin` 分支）
- **结果：** 通过

## 验收环境

| 项目 | 实际值 |
| --- | --- |
| 操作系统 | macOS 26.5.1，arm64 |
| Go | go1.26.6 darwin/arm64 |
| Gin | v1.12.0 |
| Node.js | v24.19.0 |
| pnpm | 11.22.0 |

Go 与 Gin 的版本选择依据见 [ADR-0008](../../decisions/ADR-0008-supported-go-toolchain-baseline.md)，运行时边界依据见 [ADR-0009](../../decisions/ADR-0009-runtime-boundaries.md)。

## 验收结论

| 检查项 | 实际证据 | 结果 |
| --- | --- | --- |
| 九个 `GROWTHOS_` 配置的默认、覆盖和严格校验正确 | `appconfig.Default()` / `Load` 表驱动测试 | 通过 |
| present-but-empty、非法枚举/地址/duration 和超上限值会聚合失败 | 配置负向与多错误测试 | 通过 |
| 配置错误不回显原始值，示例文件无秘密且不自动加载 | 哨兵测试、真实非法配置启动与文件评审 | 通过 |
| HTTP Server 使用类型化地址和五个 timeout | 构造器默认值与覆盖值测试 | 通过 |
| `slog` JSON/text、四个级别与基础字段正确 | 内存日志编码和级别测试 | 通过 |
| `net/http` 诊断不落入全局 logger 或泄露原始内容 | 默认 discard 与脱敏 `slog` 桥接测试 | 通过 |
| fault kind、code/message 校验、`Error()` 仅返回 code 且 cause 只可显式解包 | fault 单元测试 | 通过 |
| `request_id` 的输入校验、生成、context/header/log/envelope 关联正确 | middleware、router 和真实请求 | 通过 |
| 404、405、500 使用稳定 envelope 且不泄露内部 cause | HTTP 契约与 recovery 测试 | 通过 |
| 访问日志按 2xx/3xx、4xx、5xx 使用 Info/Warn/Error | 中间件级别测试 | 通过 |
| `GET /health` 成功 JSON 保持第 11 节三字段契约 | 回归测试与真实进程请求 | 通过 |
| 日志不记录 query、body、授权 header、panic 值、stack 或虚构 `trace_id` | middleware、ErrorLog 测试与真实 JSON 日志评审 | 通过 |
| 课程、配置、API、QA、ADR、分支和仓库地图一致 | `make doc-check` | 通过 |
| 仓库统一质量门禁 | `make verify` | 通过，保留一项前端包体积 warning |

## 自动化验证

实际执行的核心命令：

```bash
test -z "$(gofmt -l $(find . -type f -name '*.go' -not -path './vendor/*'))"
go mod verify
go test -count=1 \
  ./cmd/growth-api \
  ./internal/platform/appconfig \
  ./internal/platform/logging \
  ./internal/platform/fault \
  ./internal/infrastructure/httpapi \
  ./internal/infrastructure/httpserver
go test -race -count=20 \
  ./cmd/growth-api \
  ./internal/platform/appconfig \
  ./internal/platform/logging \
  ./internal/platform/fault \
  ./internal/infrastructure/httpapi \
  ./internal/infrastructure/httpserver
go test -count=1 ./...
go vet ./...
git diff --check
make verify
```

结果：

- 六个目标包的普通测试全部通过；相同六包各执行 20 次 Race 测试，全部通过且没有 data race；
- 全仓 `go test -count=1 ./...`、`go vet ./...`、`go mod verify`、Go 格式检查与 `git diff --check` 全部通过；
- `make verify` 包含的 Go、文档、React/TypeScript 类型检查和 Vite 生产构建全部通过；
- Vite 如实提示主 JavaScript chunk 为 695.11 kB，超过默认 500 kB 提示线。这是非阻塞 warning，后续按真实页面边界拆分，不把它隐去或写成当前失败；
- 本轮自动化失败数为 0。

## 配置失败真实冒烟

将带构建标签 `lesson-12-qa` 的真实二进制配置为非法日志级别后直接启动，进程在打开监听端口前退出：

- 退出码为 1；
- JSON 错误日志包含 `GROWTHOS_LOG_LEVEL` 和允许值约束；
- 输出不包含传入的非法原值；
- 没有用默认 `info` 静默继续运行。

这证明配置拒绝使用最小 bootstrap logger 安全报告，并且配置失败不经过尚未启动的 HTTP error envelope。

## HTTP、日志与信号真实冒烟

验收二进制使用以下非默认地址和结构化日志边界运行：

```text
version=lesson-12-qa
GROWTHOS_ENVIRONMENT=test
GROWTHOS_HTTP_ADDRESS=127.0.0.1:18080
GROWTHOS_LOG_LEVEL=debug
GROWTHOS_LOG_FORMAT=json
```

实际结果：

| 请求 / 动作 | 实际结果 |
| --- | --- |
| 携带合法 `X-Request-ID` 的 `GET /health` | 200；响应仍严格包含 `status`、`version`、`timestamp` 三个 JSON 字段；响应 header 保留同一请求 ID |
| 携带非法 `X-Request-ID` 的 `GET /health` | 200；非法值被新的安全 ID 替换，响应与日志使用替换后的同一值 |
| `GET /missing` | 404；`route_not_found` / `resource not found` / 当前 request ID |
| `POST /health` | 405；`method_not_allowed` / `method not allowed` / 当前 request ID，并返回 `Allow: GET` |
| 进程接收 `SIGTERM` | 完成优雅关闭，退出码为 0 |

真实 JSON 日志可以按 `service`、`environment`、`version`、`request_id`、`method`、`route`、`status` 和 `duration_ms` 查询。访问事件没有 raw query、请求 body、授权 header、panic/stack、内部 cause 或虚构 `trace_id`；未匹配路由使用 `route=unmatched`。

## `net/http` ErrorLog 隐私证据

HTTP Server 不允许标准库在 `ErrorLog=nil` 时回退到进程全局 logger：

- 未注入产品 logger 时，构造器安装显式 discard logger；
- 产品入口注入 `slog` 后，桥接器只输出 `msg=http_server_error` 和 `component=net/http`；
- 测试向底层诊断写入带秘密哨兵和 stack 哨兵的字符串，结构化输出不包含二者；
- Server 返回的可处理错误仍沿正常 Go 错误链交给进程入口，不依赖原始 ErrorLog 文本诊断。

## 第 12 节验收时未覆盖项与剩余风险

- 健康接口只证明进程 liveness，不检查数据库、缓存、消息或业务状态；第 13 节后续新增了独立 MySQL `/ready`，但没有改变本接口语义；
- 本节没有业务 API，fault 到具体业务 code 的第一批真实映射要在后续业务章节验证；
- 没有 OpenTelemetry span、`trace_id`、日志采集平台或跨进程传播；
- 没有动态配置、Nacos、Secret manager 或运行时配置刷新；
- 当前 recovery 与 `net/http` 桥接为避免隐私泄露，不记录 panic 值、stack 或原始底层诊断；受控诊断存储、脱敏和访问策略尚未建立；
- React 尚未消费 error envelope 和 request ID，第 15 节以前没有浏览器联调证据；
- Vite 的 695.11 kB 主 chunk warning 仍需后续代码拆分与体积预算；
- 本轮没有性能、容量、长稳或故障注入证明。

数据库连接、账号隔离、前向 Migration 与 readiness 的后续证据见[第 13 节 QA](lesson-13.md)。本段保留第 12 节验收时的历史范围，不把后续能力倒写成本节交付。

## 清理记录

真实冒烟使用显式任务临时目录保存二进制、响应和日志；验收结束后该目录已经删除并确认未在仓库留下临时文件。Go module/build cache 与 `web/node_modules` 属于可复用依赖，本轮没有删除。
