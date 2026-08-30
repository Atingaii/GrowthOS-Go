# GrowthOS-Go 前端工程架构

## 定位

`web/` 是统一的 React 单页应用。第 14 节完成框架、路由和基础组件，第 15 节完成系统探针首个真实前后端切片；第 17～21 节建立 Lottery 领域、持久化、选择与 development/test ephemeral API，第 22 节再通过 `lotteryApi`、运行时 decoder 和请求状态 Hook 接入真实 React `/lottery` 页面。同时，四类工作台以同一个 `WorkspaceShell` 统一侧栏、顶栏、搜索、主题、内容几何和移动抽屉，视觉语言采用高密度、扁平、细边框的数据工作台，而不是把业务页面包装成营销落地页。

## 工作台

| 工作台 | 路由 | 当前内容 |
| --- | --- | --- |
| 用户端 | `/`、`/feed`、`/campaigns`、`/lottery`、`/points`、`/coupons`、`/profile` | `/lottery` 真实消费 ephemeral API；其余页面是带快照时间和能力边界的 Mock/本地交互 |
| 运营端 | `/admin/*` | 活动、策略、奖品、账户、实验、分析、任务和审计入口；数据为明确演示快照，未接入写链路 |
| MCP Gateway | `/mcp/*` | 服务、Tools、权限和审计控制台入口；当前为本地节点/风险样例，不是实时 Gateway telemetry |
| AI Operator | `/agent/*` | 工作区、任务、审批和历史入口；仅可创建浏览器内演示任务，不调用 Agent、MCP Tool 或后端写接口 |
| 系统状态 | `/system/status` | 真实读取 `GET /health` 与 `GET /ready`，区分正常、依赖降级和 API 状态未知 |

## 目录边界

```text
web/src/
├── api/           # 同源 HTTP client、运行时契约解码与按能力组织的 API 函数
├── layouts/       # 四类工作台的导航配置与 WorkspaceShell 组合
├── pages/         # 按工作台和领域组织的路由页面
│   └── system/status/ # 系统探针页面、状态 hook 与对应测试
├── components/    # 公共、业务和图表组件；layout/WorkspaceShell 统一工作台壳层
├── routes/        # 路由注册
├── mocks/         # 除系统探针与 Lottery selection 外的当前演示快照，后续按领域替换
├── stores/        # 用户、主题、布局和环境状态
└── types/         # 业务类型与 API 契约
```

`src/api/httpClient.ts` 只接受同源绝对路径，并统一处理 JSON、无 body POST、超时、调用方取消、请求关联和浏览器侧耗时；错误显式区分 `http`、`gateway`、`network`、`timeout`、`cancelled` 与 `contract`。`src/api/systemApi.ts` 对探针响应做运行时解码；`src/api/lotteryApi.ts` 校验 canonical uint64 string、固定 demo header 和 ephemeral success DTO。`src/pages/system/status/useSystemStatus.ts` 管理并行探针；`src/pages/user/lottery/useEphemeralLotterySelection.ts` 以同步 pending guard、`AbortController` 和 generation guard 管理一次 selection，且不自动重试。

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

当前真实 React 联调有两条：`/system/status` 读取 `GET /health` 与由 MySQL 支撑的 `/ready`；`/lottery` 调用 `POST /api/v1/lottery/strategies/:strategy_id/ephemeral-selections`，并把服务端 `reward`、`no_reward` 与各类失败保持为不同状态。Lottery 页面不维护前端候选、概率、库存或随机决定，不在失败后透明重试；返回值仍只是页面刷新后消失的临时候选，不是 Draw、中奖记录或已发放权益。

活动、Feed、积分、优惠券、个人资料、Admin、MCP 和 Agent 页面继续使用统一 `MOCK_SNAPSHOT_LABEL` 标注的演示快照或浏览器内状态。筛选、路由、复制、主题、搜索和本地任务等交互可以真实工作，但不代表存在相应后端查询/写入、实时账务或生产控制面。四类工作台当前也只是信息架构：没有登录认证、RBAC、租户隔离、对象级授权或服务端拒绝路径。路线不会在第 23 节因页面诉求临时插入一套前端角色开关：第 23～30 节先形成 Lottery 规则、缓存、资格决策与 Activity 等真实受保护资源，第 31～35 节再依次建立公共访问控制模型、真实会话、服务端强制、前端权限感知和越权端到端验收，第 36 节首个真实运营后台必须复用这套能力。前端裁剪只改善体验，服务端拒绝才构成授权边界。

第 21 节后端能力仍由[课程](../course/part-03/lesson-21-lottery-api.md)、[API](../api/lessons/lesson-21.md)和 [ADR-0018](../decisions/ADR-0018-ephemeral-lottery-selection-api.md)约束；第 22 节前端实现与边界见[课程](../course/part-03/lesson-22-react-lottery-page.md)、[API](../api/lessons/lesson-22.md)、[QA](../qa/lessons/lesson-22.md)、[设计手记](../design-thinking/lessons/lesson-22.md)和[面试问答](../interview/lessons/lesson-22.md)。

第 15 节已做正常、MySQL 不可用和 API 离线三种系统探针场景的真实浏览器关联验收。第 22 节又覆盖 Lottery 请求状态、共享壳层交互，以及 1719 × 862 桌面和 390 × 844 移动视口；这些证据只证明当前环境中的联调、交互和展示契约，不构成性能、持续可用性、权限隔离或生产就绪声明。

每节新增或调整前端 API 时，必须同步写入 [章节 API 记录](../api/lessons/README.md)。
