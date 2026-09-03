# GrowthOS-Go 前端工程架构

## 定位

`web/` 是统一的 React 单页应用。第 14～22 节建立框架、系统探针、ephemeral Lottery 与共享 `WorkspaceShell`；第 31 节只定义权限语言，没有改 UI。第 32 节已新增并完成 development 验收的独立公共 `AuthLayout`、严格 `sessionApi`、`useSessionBoundary`，以及真实 `/login`、`/session`，把 anonymous/checking/authenticated/unavailable 分开呈现。该边界只负责认证体验，不包裹业务工作台，也不把 Role/Scope/Permission 放进浏览器状态。

## 工作台

| 工作台 | 路由 | 当前内容 |
| --- | --- | --- |
| 用户端 | `/`、`/feed`、`/campaigns`、`/lottery`、`/points`、`/coupons`、`/profile` | `/lottery` 真实消费 ephemeral API；其余页面是带快照时间和能力边界的 Mock/本地交互 |
| 运营端 | `/admin/*` | 活动、策略、奖品、账户、实验、分析、任务和审计入口；数据为明确演示快照，未接入写链路 |
| MCP Gateway | `/mcp/*` | 服务、Tools、权限和审计控制台入口；当前为本地节点/风险样例，不是实时 Gateway telemetry |
| AI Operator | `/agent/*` | 工作区、任务、审批和历史入口；仅可创建浏览器内演示任务，不调用 Agent、MCP Tool 或后端写接口 |
| 系统状态 | `/system/status` | 真实读取 `GET /health` 与 `GET /ready`，区分正常、依赖降级和 API 状态未知 |
| 身份会话 | `/login`、`/session` | 真实调用 Session create/current/revoke；显示可信 human Principal 和到期边界，不显示权限或保护其他 route |

## 目录边界

```text
web/src/
├── api/           # 同源 HTTP client、运行时契约解码与按能力组织的 API 函数
├── layouts/       # 四类工作台的导航配置与 WorkspaceShell 组合
├── pages/         # 按工作台和领域组织的路由页面
│   ├── auth/      # 登录与当前会话页面；不承担 RBAC
│   └── system/status/ # 系统探针页面、状态 hook 与对应测试
├── components/    # 公共、业务和图表组件；layout/WorkspaceShell 统一工作台壳层
├── routes/        # 路由注册
├── mocks/         # 除系统探针与 Lottery selection 外的当前演示快照，后续按领域替换
├── stores/        # 用户、主题、布局和环境状态
└── types/         # 业务类型与 API 契约
```

`src/api/httpClient.ts` 只接受同源绝对路径，并统一处理 JSON、无 body POST、超时、调用方取消、请求关联和浏览器侧耗时；错误显式区分 `http`、`gateway`、`network`、`timeout`、`cancelled` 与 `contract`。`systemApi` 与 `lotteryApi` 继续各自严格解码；新增 `sessionApi` 把同一 `/api/v1/session` 固定为 201 create、200 current 与 zero-body 204 revoke，只接受最小 human Principal/expiry/CSRF DTO。Cookie 由浏览器管理，DELETE 只显式回传内存中的 `X-CSRF-Token`。

`useSessionBoundary` 用 `AbortController` 和单调 generation 管理 current/login/logout，注销开始即使旧 GET 失效，避免迟到响应恢复旧 Principal。password 不镜像到 React state；CSRF 与 Session snapshot 只驻留当前组件树，不进入 Zustand、localStorage 或 sessionStorage。`AuthLayout` 只在 `/login` 与 `/session` 之间做 replace navigation 和焦点交接，不是全站 route guard。

## 运行方式

```bash
cd web
pnpm install --frozen-lockfile
pnpm dev
pnpm test
pnpm run typecheck
pnpm run build
pnpm preview
```

前端要求 Node.js `>=22.22.2`，锁定 pnpm `10.13.1`。开发服务器绑定 `127.0.0.1:5173`，预览服务器绑定 `127.0.0.1:4173`，两者都启用严格端口检查，并只精确代理 `/health`、`/ready` 和 `/api` 路径边界。

代理目标使用 Vite 进程配置 `GROWTHOS_WEB_API_PROXY_TARGET`，默认值为 `http://127.0.0.1:8080`。该变量没有 `VITE_` 前缀，不会被打进浏览器包；配置只接受不含凭据、路径、查询或片段的 HTTP(S) origin。浏览器代码始终请求同源路径，不直接持有后端 origin。

## 当前事实与后续演进

当前真实 React 联调有三组：`/system/status` 读取探针，`/lottery` 消费 ephemeral selection，`/login`/`/session` 消费真实 Session create/current/revoke。会话页面区分 401 anonymous 与 503 unavailable：Identity 依赖失败不会被伪装成退出，也不会回显旧 Principal；恢复后只通过显式 current re-check 重建状态。Lottery 仍不维护候选、概率、库存、Redis key、TTL 或随机决定，其返回值也仍不是 Draw、中奖记录或已发放权益。

第 24 节缓存对浏览器完全透明：public path、method、headers、request/response DTO、错误码和 `Cache-Control: no-store` 都没有变化，也没有增加“cache hit”响应头或页面徽标。浏览器不能选择跳过/刷新服务端缓存，不能自行保存 Strategy 投影，Redis 故障也不应触发前端重试。是否命中、回源、修复 poison value 或降级只属于服务端低基数观测与运维证据；把这些细节泄露到 UI 会把技术拓扑变成产品契约，并诱导调用方依赖不可保证的缓存状态。

活动、Feed、积分、优惠券、个人资料、Admin、MCP 和 Agent 页面继续使用统一 `MOCK_SNAPSHOT_LABEL` 标注的演示快照或浏览器内状态。筛选、路由、复制、主题、搜索和本地任务等交互可以真实工作，但不代表存在相应后端查询/写入、实时账务或生产控制面。第 32 节现在有登录认证，却有意不按该状态锁住工作台：没有 Policy/assignment repository、服务端 Resource facts/enforcement，就不能让一个“已登录”布尔值冒充授权。

因此 `/admin/*`、`/mcp/*`、`/agent/*`、用户业务导航、直接 URL 和页面操作目前都不构成权限隔离。第 33 节先在服务端强制，且拒绝仍不能依赖前端；第 34 节才让导航、路由、操作和搜索结果消费同一服务端 capability projection；第 35 节再覆盖跨角色、跨对象、跨租户与直接 API 的负向 E2E。前端裁剪只改善体验，服务端拒绝才构成授权边界。

第 21～24 节后端/前端/缓存边界继续由各自课程、API 与 ADR 约束。第 32 节会话前端见[产品基线](../product/identity-session-authentication-v1.md)、[API](../api/lessons/lesson-32.md)、[课程](../course/part-04/lesson-32-real-session-authentication.md)、[QA](../qa/lessons/lesson-32.md)、[运维手册](../runbooks/identity-session-operations.md)和仓库根[设计 QA](../../design-qa.md)。这些入口都必须保留“认证不等于授权”的停止线。

第 15 节已做系统探针浏览器关联验收，第 22 节覆盖 Lottery 与共享壳层。第 32 节视觉 QA 分别核查 1719 × 862 登录桌面态、390 × 844 登录移动态和 1280 × 720 authenticated state；真实浏览器旅程另行覆盖 login/current/logout、reload、Identity store unavailable/recovery 与 focus handoff；late-response isolation 由 Hook/单元测试证明。三组证据不能互相替代，也不构成全设备/辅助技术认证、性能、持续可用性、RBAC、越权抗性或生产就绪声明。

每节新增或调整前端 API 时，必须同步写入 [章节 API 记录](../api/lessons/README.md)。
