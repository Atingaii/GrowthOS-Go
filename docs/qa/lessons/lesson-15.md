# 第 15 节 QA 验收证据

- **日期：** 2026-08-29
- **产物：** [前后端第一次联调](../../course/part-02/lesson-15-first-fullstack-integration.md)
- **分支：** `codex/lesson-15-first-fullstack-integration`
- **实现提交：** `7e499cc`
- **联调后测试加固：** `2283a70`
- **结果：** 通过

> 本文件记录 2026-08-29 实际执行的自动化与浏览器证据。瞬时 RTT、request ID 和临时凭据不进入长期事实；截图仅用于本轮人工核查，完成后已经清理。

## 1. 验收目标

本节要证明的是浏览器通过同源开发代理，真实调用现有 Go `/health` 与 `/ready`，并在正常、依赖故障、超时、取消和契约漂移时给出不夸大的状态。

不在本节证明：

- 任何抽奖、活动、积分、优惠券、Feed、MCP 或 Agent 业务 API 已实现；
- 所有 Mock 页面已经后端化；
- MySQL schema、Migration 版本或业务数据正确；
- Redis、MQ、PostgreSQL 或多实例集群健康；
- Vite proxy 是生产网关；
- readiness 成功等于业务 SLA 或数据库性能达标。

## 2. 验收环境

| 项目 | 实际值 | 状态 |
| --- | --- | --- |
| 分支 | `codex/lesson-15-first-fullstack-integration` | 已知 |
| Git 实现提交 | `7e499cc`；测试加固 `2283a70` | 已核对 |
| macOS / 架构 | macOS 26.5.1、Darwin 25.5.0、arm64 | 已记录 |
| Docker Engine | 29.7.2 | 已记录 |
| MySQL 容器 / 版本 | 既有 `mysql` 容器，MySQL 8.4.11 | 已实测 |
| Go | go1.26.6 darwin/arm64 | 已实测 |
| Node.js | v24.19.0；项目最低 `>=22.22.2` | 已实测 |
| pnpm | 10.13.1，由 `packageManager` 与 CI 共同固定 | 已实测 |
| React / Vite / Vitest / oxfmt | 19.2.8 / 8.0.3 / 4.1.11 / 0.65.0 | 已实测 |
| 浏览器验收 | agent-browser 0.35.1、系统 Google Chrome、axe-core 4.12.1 | 已实测 |
| Go API origin | `http://127.0.0.1:8080` | 已实测 |
| Vite origin | `http://127.0.0.1:5173` | 已实测 |

真实 MySQL 密码和临时账号必须通过任务环境注入，不写入命令输出、本文、Git、截图、浏览器地址或服务日志。

## 3. 静态实现核查

| 检查项 | 预期证据 | 当前结果 |
| --- | --- | --- |
| dev 与 preview 同时代理精确 `/health`、`/ready` 与 `/api` namespace | 共用正则代理构造函数且不 rewrite | 通过 |
| 代理目标只允许无凭据、无路径的 HTTP(S) origin | URL/protocol/credentials/path/query/hash 校验 | 通过（静态与启动负向核查） |
| 代理变量不进入浏览器 bundle | `GROWTHOS_WEB_API_PROXY_TARGET` 无 `VITE_` 前缀 | 通过 |
| 开发服务器默认只绑定 loopback | dev/preview `host=127.0.0.1` | 通过 |
| 端口冲突不静默漂移 | dev/preview `strictPort=true` 且端口范围校验 | 通过 |
| 请求不能绕出 origin | 拒绝绝对/协议相对/反斜杠路径，`mode=same-origin`、`redirect=error` | 通过 |
| 统一 fetch 选项 | GET、Accept JSON、no-store、same-origin credentials、signal | 通过 |
| HTTP error 只接受统一 envelope | `code/message/request_id` 运行时校验 | 通过 |
| 非 JSON 502/503/504 独立为 gateway | 不伪造后端 code；真实 Vite 502 验证 | 通过 |
| header/body request ID 冲突 fail closed | client contract 测试 | 通过 |
| 成功响应执行运行时解码 | status、非空 version、RFC 3339 形状且可解析 timestamp | 通过 |
| 两探针有独立 loading/success/error 状态 | hook 测试 | 通过 |
| 新刷新取消旧一轮且旧结果不可覆盖 | AbortController + generation 测试 | 通过 |
| StrictMode / 卸载清理 | 双 setup/cleanup 与 unmount 测试 | 通过 |
| 页面不展示假服务、假延迟或“全部正常” | 组件断言与浏览器人工核查 | 通过 |
| readiness 503 展示“API 存活、MySQL 未就绪” | 组件与真实依赖故障 | 通过 |
| 所有 Go error envelope 禁止缓存 | `errors.go` 统一出口测试 | 通过 |

## 4. 自动化验证

实际执行的等价门禁（最终另执行 `make verify`）：

```bash
cd web
CI=true pnpm install --frozen-lockfile
pnpm run test
pnpm run typecheck
pnpm run build

go test -count=1 ./internal/infrastructure/httpapi
go test -count=1 ./...
go vet ./...
make verify
```

| 命令 | 验收标准 | 实际结果 |
| --- | --- | --- |
| `CI=true pnpm install --frozen-lockfile` | pnpm 10.13.1 从 lockfile 重建 167 个包 | 通过 |
| `pnpm run test` | 4 个文件、34 项测试 | 通过，0 失败 |
| `pnpm run typecheck` | TypeScript 无错误 | 通过 |
| `pnpm run build` | Vite 8.0.3 构建成功 | 通过；主 JS 708.34 kB / gzip 210.22 kB warning 保留 |
| HTTP API package test | `no-store` 与既有 health/readiness 契约通过 | 通过 |
| `go test -count=1 ./...` / `go vet ./...` | 无失败、无静态分析错误 | 通过 |
| `make verify` | Go、文档、前端 test/typecheck/build 总门禁 | 通过（最终文档树复跑） |

自动化结果填写规则：记录命令、退出码和异常 warning；普通单元测试不能替代真实代理联调，Vite build 成功也不能证明浏览器请求到达 Go API。

## 5. HTTP client 单元验收

### 5.1 成功路径

| 场景 | 预期 | 实际结果 |
| --- | --- | --- |
| 200 JSON + `X-Request-ID` | 返回 decoder 结果、status、request ID 和非负 elapsed | 通过 |
| 额外响应字段 | 允许兼容性扩展，只提取当前必需字段 | 通过 |
| fetch options | GET、Accept JSON、no-store、credentials/mode same-origin、redirect error、AbortSignal | 通过 |

### 5.2 错误路径

| 场景 | 预期分类 | 实际结果 |
| --- | --- | --- |
| fetch reject | `network`，不泄漏原始 Error | 通过 |
| 非 JSON 502/503/504 | `gateway`，保留 status，不伪造后端 code | 通过 |
| timeout 到期 | `timeout`，底层 signal 已取消 | 通过 |
| 调用方预先/运行中 abort | `cancelled` | 通过 |
| 503 + 合法 error envelope | `http`，保留 status/code/request ID | 通过 |
| 非 2xx + 非法 envelope | `contract` | 通过 |
| 200 但 Content-Type 非 JSON | `contract` | 通过 |
| 200 但 JSON 无法解析 | `contract` | 通过 |
| decoder 返回 null 或抛异常 | `contract` | 通过 |
| header/body request ID 不同 | `contract`，不能选择其一继续 | 通过 |
| 相对、绝对、protocol-relative、反斜杠路径 | 请求前 `contract` | 通过 |
| timeout 小于 100 ms、大于 30 s 或非安全整数 | 请求前 `contract` | 通过 |

测试应使用 fake fetch 与可控时钟/定时器，不访问真实互联网，不依赖执行速度断言一个精确毫秒值。

## 6. React 状态与竞态验收

| 场景 | 预期 | 实际结果 |
| --- | --- | --- |
| 首次挂载 | 同时启动 health 和 readiness，不串行等待 | 通过 |
| health 先成功 | health 卡片先展示，readiness 保持 loading | 通过 |
| readiness 503、health 成功 | 汇总为 API 存活/MySQL 未就绪 | 通过 |
| health 失败、readiness 成功 | 整体保持未知，只保留 readiness 单项事实 | 通过 |
| 两者 settle | 记录本轮完成时间 | 通过 |
| 连续点击刷新 | 旧 controller abort，新一轮回到 loading | 通过 |
| 旧 Promise 忽略 signal 后才完成 | generation 检查阻止旧值覆盖新值 | 通过 |
| 组件卸载 | active request 被取消，无卸载后状态更新 | 通过 |
| React Strict Mode setup/cleanup/setup | 第一轮 signal 已取消，第二轮仍活跃，卸载后全部取消 | 通过 |

## 7. 真实前后端联调

### 7.1 正常链路

前置：使用临时受限 API 账号让 Go API 连接真实 MySQL，并分别启动 API 与 Vite dev server。

实际检查：

1. 打开 `http://127.0.0.1:5173/system/status`；
2. 浏览器 Network 确认请求 URL 是 Vite origin 下的 `/health`、`/ready`，而不是硬编码 `:8080`；
3. 确认两个请求最终得到真实 Go JSON，路径未 rewrite；
4. 页面只显示 Go API 与 MySQL readiness；
5. 两个成功卡片显示真实版本、服务端时间、浏览器 RTT 和 request ID；
6. 响应 header 包含 `Cache-Control: no-store` 与 `X-Request-ID`；
7. 点击“重新检查”产生新一轮 request ID 和完成时间；
8. 服务访问日志能用页面 request ID 定位对应请求，且不含凭据。

| 检查项 | 实际证据 | 结果 |
| --- | --- | --- |
| 页面通过 Vite 同源路径到达 Go | Network 记录为 `GET http://127.0.0.1:5173/health` 与 `/ready`，均 200 | 通过 |
| `/health` 与 `/ready` 都为真实响应 | 页面显示 `lesson-15`、服务端时间、两个不同 request ID 与本轮真实 RTT | 通过 |
| 页面状态与响应一致 | 可见汇总为“已接入检查正常”，只显示两张真实卡片 | 通过 |
| request ID 可关联前后端 | 页面 ID 可在 Go `http_request` 结构化日志中找到同一次请求 | 通过 |
| 手动刷新产生新一轮检查 | 刷新前后两组 request ID 不同 | 通过 |
| CORS 非依赖项 | Vite 同源响应无 `Access-Control-Allow-Origin` 依赖，页面仍可读取 ID | 通过 |

### 7.2 依赖故障链路

使用可恢复且目标明确的方式让 API 的 MySQL readiness 失败，同时保持 Go 进程可响应；不得破坏用户原有数据库或容器数据。

预期：

- `/health` 仍为 200；
- `/ready` 为 503，body 是 `dependency_unavailable/service unavailable/request_id`；
- 503 header 包含相同的 `X-Request-ID` 与 `Cache-Control: no-store`；
- 页面汇总为“API 存活，MySQL 未就绪”，不能写“所有服务宕机”；
- 页面不显示驱动错误、数据库地址、用户名、密码、DSN 或 SQL；
- 恢复 MySQL 后手动刷新，两张卡片回到成功。

| 检查项 | 实际证据 | 结果 |
| --- | --- | --- |
| liveness/readiness 故障语义分离 | 临时账号失效且只终止该账号连接后，health 200 / ready 503 | 通过 |
| 503 安全 envelope 与 no-store | `dependency_unavailable`、`service unavailable`、header/body ID 一致、`no-store` | 通过 |
| 前端降级文案准确 | “API 存活，MySQL 未就绪”；未出现 driver、账号、DSN 或 SQL | 通过 |
| 影响范围 | 未停止 MySQL 容器，只操作 `growthos_l15_qa` 临时 schema/user | 通过 |

### 7.3 API 或代理不可达

停止本节 API 后，Vite 对两个代理请求返回 `502 text/plain`。第一次实跑暴露出一个测试替身未覆盖的问题：客户端把它归为 contract，用户只看到“响应契约无法识别”。修复后新增 `gateway` 类型，第二次实跑页面稳定显示“无法确认 API 状态”与“代理无法连接 API（HTTP 502）”，不把它解释为 MySQL 故障；对应回归测试进入 `2283a70`。

结果：**通过**。这同时说明真实代理联调不能被 mock fetch 或 Vite build 替代。

## 8. 配置与安全负向验收

以下测试只能使用无 Secret 的临时配置：

| 代理目标 | 预期 | 实际结果 |
| --- | --- | --- |
| `ftp://127.0.0.1:8080` | Vite 启动前拒绝 | 通过，返回 HTTP(S) origin 约束 |
| `http://user:pass@127.0.0.1:8080` | 拒绝 credentials | 通过，启动前失败 |
| `http://127.0.0.1:8080/base` | 拒绝 path | 通过，启动前失败 |
| `PORT=0` | 拒绝随机端口 | 通过，返回 1～65535 整数约束 |
| query/fragment/非法 URL | 校验分支拒绝 | 代码审查通过；未逐个启动实跑 |

还需检查：

- 浏览器 bundle 与 source map 不含 MySQL 密码、DSN、代理凭据或测试哨兵；
- 仓库只提交 `.env.example`，不提交 `.env.local`；
- Go 与 Vite 日志不打印 Secret；
- API 不新增宽泛 `Access-Control-Allow-Origin`；
- 404、405、500、503 等统一错误均包含 `Cache-Control: no-store`；
- 页面不渲染原始 response body 或底层 exception。

结果：**通过**。`web/.env.example` 只包含公开 loopback target；未创建或提交 `.env.local`。真实 503 body 只有稳定公开 message 与 request ID，浏览器页和 Go 日志均未出现密码、DSN、SQL 或 driver cause。

## 9. 浏览器与可访问性检查

| 检查项 | 验收标准 | 实际结果 |
| --- | --- | --- |
| 初次 loading | 文本与旋转图标同时表达，不只有颜色 | 组件测试通过 |
| 状态变化播报 | 汇总和每卡 live region；隐藏标题形成“探针名：状态” | 代码与 a11y tree 通过 |
| 键盘刷新 | 原生 button，loading 时禁用 | 组件/浏览器检查通过 |
| 浅色主题对比度 | 初检 1 类、14 个节点不达 WCAG AA；调深状态与说明色后复测 | 正常/降级/离线均 0 violation |
| 宽屏 | 两卡片并列，request ID 正常换行 | 1280×774 截图人工通过 |
| 窄屏 / 深色主题 | 响应式类与 dark token 存在 | 本轮未做独立截图，保留风险 |
| 慢响应 | 先完成的探针先显示，另一张保持 loading | hook 测试通过 |
| 内容真实性 | 不出现未实现服务、假延迟、“全部正常”或业务上线话术 | 组件与真实页面通过 |
| 控制台与错误层 | 只有 Vite 连接/HMR、React DevTools 提示；无 page error / overlay | 通过 |
| 返回首页 | 状态页链接导航到 `/home` 且页面有内容 | 通过 |

浏览器验收应记录实际工具、视口和观察；只看组件源码不能替代本节真实视觉与 Network 检查。

## 10. 当前未覆盖项与剩余风险

- 本节只验证单个本地 API 实例，不能覆盖负载均衡、多实例路由、滚动发布或跨机房；
- 浏览器 elapsed 是客户端观察的往返时间，混合代理、调度和渲染前开销，不是服务端处理时长；
- 5 秒前端 timeout 只终止浏览器等待；服务端是否停止工作取决于请求 context 和下游取消传播；
- 手动刷新不是监控系统，页面关闭后不提供检测或告警；
- readiness 只 Ping MySQL，不能覆盖连接池排队、慢 SQL、锁、磁盘、复制、Migration 或数据正确性；
- Vite proxy 不代表生产反向代理已经配置或安全加固；
- 本轮未单独截图验证窄屏与深色模式；响应式类和 dark token 已存在，但仍需第 16 节容器化后做多视口回归；
- runtime decoder 校验 RFC 3339 形状和 `Date.parse`，但没有独立实现完整公历日合法性算法；真实 Go `time.Time` 输出已覆盖；
- 业务页面仍使用 Mock；真实业务接口需要在对应章节分别验收。

## 11. 清理结果

验收结束后只清理由本节创建且已明确解析的临时对象：

- 停止本节启动的 Go/Vite 临时进程；
- 本轮未创建 `.env.local`，保留可复用且无秘密的 `.env.example`；
- 删除 Vite build 生成的 `web/dist`；
- 删除 `growthos_l15_qa` 临时 MySQL schema 与同名 `%` host 用户，查询计数均为 0；
- 删除 `/tmp/growthos-l15-browser.1XMnQZ` 中两张验收截图；
- 关闭 agent-browser `growthos-l15` 会话并删除本轮 `pnpm dlx` 的精确缓存目录；
- 停止本节 Go/Vite 进程，确认 8080 与 5173 均无监听；
- 保留用户原有 Docker 容器、Volume、数据库、账号、pnpm store、Go module cache、`node_modules` 与源码。

清理后 MySQL 8.4 容器仍为运行态，用户既有数据和其他中间件未被修改。

## 12. 验收结论

第 15 节通过。证据能证明：浏览器经 Vite 同源代理真实消费当前 Go 实例的 liveness/readiness，正常、MySQL 降级和 API 离线三态有不同且不夸大的 UI 语义，取消/竞态/契约和关联 ID 受到自动化保护，统一 Go 错误不会被缓存。

证据不能证明：任何业务 API、业务表、多实例 SLA、生产网关、CORS、自动监控或数据库性能已经完成。主分支与 `origin/main` 在验收时仍均为 `3ec52a2`；学习分支已独立推送。
