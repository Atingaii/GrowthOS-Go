# 第 11 节 QA 验收证据

- **日期：** 2026-08-29
- **产物：** [使用 Gin 初始化 Go Web 服务](../../course/part-02/lesson-11-gin-http-service.md)
- **分支：** `codex/lesson-11-gin-http-service`
- **实现提交：** `ade1fad`（`feat: add lesson eleven Gin HTTP service`）
- **结果：** 通过

## 验收环境

| 项目 | 实际值 |
| --- | --- |
| 操作系统 | macOS 26.5.1，arm64 |
| Go | go1.26.6 darwin/arm64 |
| Gin | v1.12.0 |
| Node.js | v24.19.0 |
| pnpm | 11.22.0 |

Go 1.26.6 和 Gin v1.12.0 的选择依据见 [ADR-0008](../../decisions/ADR-0008-supported-go-toolchain-baseline.md)。

## 验收结论

| 检查项 | 实际证据 | 结果 |
| --- | --- | --- |
| `GET /health` 返回 200、JSON 与禁止缓存头 | handler/router 自动化测试及真实进程 curl | 通过 |
| 响应严格包含 `status`、`version`、UTC RFC3339Nano `timestamp` | 注入版本和固定时钟断言；冒烟版本为 `lesson-11-qa` | 通过 |
| 规范路径不重复 | `/health` 为 200，`/api/v1/health` 为 404 | 通过 |
| 不支持的方法明确拒绝 | `POST /health` 为 405 | 通过 |
| 不信任未配置的转发代理 | 固定直连地址和伪造 `X-Forwarded-For` 的回归测试 | 通过 |
| 服务端安全超时存在 | 构造器测试断言请求头/读/写/空闲超时为 5/15/30/60 秒 | 通过 |
| context 取消触发最长 5 秒的优雅关闭 | 生命周期测试及真实二进制 `SIGTERM` 冒烟 | 通过 |
| 正常关闭不误报，监听/Serve/Shutdown 异常向上传播 | 正常、监听失败、Serve 失败和 Shutdown 超时测试 | 通过 |
| `cmd` 只做装配，router 与 Server 生命周期分离 | 文件职责与依赖评审 | 通过 |
| 未接入数据库、缓存、消息或业务 API | 依赖、配置和变更清单评审 | 通过 |
| 文档和仓库地图一致 | `make doc-check` | 通过 |
| 仓库统一质量门禁 | `make verify` | 通过，保留一项前端包体积 warning |

## 自动化验证

实际执行：

```bash
gofmt -w cmd/growth-api/main.go \
  internal/infrastructure/httpapi/*.go \
  internal/infrastructure/httpserver/*.go
go test -count=100 ./internal/infrastructure/httpapi ./internal/infrastructure/httpserver
go test -race -count=20 ./internal/infrastructure/httpapi ./internal/infrastructure/httpserver
go test -count=1 ./...
go vet ./...
go mod verify
git diff --check
make verify
```

结果：

- HTTP API 与 Server 两个包各连续运行 100 次普通测试、20 次 Race 测试，全部通过；
- 全仓共解析 5 个 Go 包，所有有测试的包通过，`cmd/growth-api` 当前没有独立测试文件；
- `go vet ./...`、`go mod verify`、格式检查和文档检查通过；
- 前端 TypeScript 检查和 Vite 生产构建通过；
- Vite 如实提示主 JavaScript chunk 为 695.11 kB、超过默认 500 kB 提示线。这是非阻塞 warning，后续按真实页面拆分处理，不把它写成当前失败；
- 本轮自动化失败数为 0。

## 真实进程冒烟与信号关闭

为避免 `go run` wrapper 的信号退出码干扰判断，本次把产品二进制构建到 `mktemp -d` 创建的任务临时目录，直接启动二进制：

```bash
TASK_TEMP=$(mktemp -d -t growthos-lesson11)
trap 'find "$TASK_TEMP" -depth -delete' EXIT
go build -ldflags '-X main.version=lesson-11-qa' \
  -o "$TASK_TEMP/growth-api" ./cmd/growth-api
"$TASK_TEMP/growth-api" &
API_PID=$!
curl -i http://127.0.0.1:8080/health
kill -TERM "$API_PID"
wait "$API_PID"
```

实际健康响应：

```http
HTTP/1.1 200 OK
Cache-Control: no-store
Content-Type: application/json; charset=utf-8
```

```json
{"status":"ok","version":"lesson-11-qa","timestamp":"2026-08-29T04:23:28.200025Z"}
```

同一进程的补充检查：

- `POST /health` 返回 405；
- `GET /api/v1/health` 返回 404；
- 向产品二进制发送 `SIGTERM` 后退出码为 0；
- 任务临时目录已在检查结束后删除，不在仓库中留下二进制或响应文件。

一次本地请求只证明当前契约和关闭路径可运行，不构成性能、可用性或容量证明。

## 当前未覆盖项与剩余风险

- 健康接口只检查进程 liveness，不检查数据库、缓存、消息或业务状态；这些能力尚未接入；
- 没有进行性能、容量、长稳、异常网络或操作系统强制终止测试；
- `version` 只是可注入构建标签，正式发布命名和制品溯源流程尚未建立；
- 配置、结构化日志、请求关联字段和统一错误响应留到第 12 节；
- React 仍使用 Mock 数据，第 15 节以前没有真实浏览器联调证据；
- 前端生产包体积已产生 warning，后续增加真实页面时需要代码拆分和体积预算；
- 当前 OAuth 凭据无法推送部分早期历史课程分支，具体范围见[课程分支检查点](../../course/branch-checkpoints.md)；第 11 节分支不受此限制并已推送。

## 清理记录

本节真实冒烟产生的临时二进制、响应头、响应体和日志均位于同一个显式任务临时目录；验收结束后已删除并确认路径不存在。保留 Go module/build cache 和 `web/node_modules`，因为它们是可复用依赖，不属于一次性产物。最终质量门禁生成且由 `web/.gitignore` 明确标记的 `web/dist` 已通过精确 dry-run 核对后删除，并确认路径不存在。
