# GrowthOS-Go 前端工程架构

## 定位

`web/` 是统一的 React 单页应用，复刻既有 GrowthOS UI 设计。第 14 节完成整体框架、路由、布局和组件体系，第 15 节完成首个真实前后端切片：系统状态页通过 Vite 同源代理读取 Go API 的 liveness 与 readiness。第 17 节虽已加入 Lottery Strategy/Award 纯 Go 领域对象，第 18 节也已加入两张 Lottery 业务表，但仍没有 Repository、业务 API 或前端适配，因此其余业务能力继续按后续章节逐步接入。

## 工作台

| 工作台 | 路由 | 当前内容 |
| --- | --- | --- |
| 用户端 | `/`、`/feed`、`/campaigns`、`/lottery`、`/points`、`/coupons` | 使用 Mock 数据呈现增长触达与用户权益；`/lottery` 未调用 Go 领域模型或第 18 节两张表 |
| 运营端 | `/admin/*` | 活动、策略、奖品、账户、实验、分析、任务和审计入口 |
| MCP Gateway | `/mcp/*` | 服务、Tools、权限和审计控制台入口 |
| AI Operator | `/agent/*` | 工作区、任务、审批和历史入口 |
| 系统状态 | `/system/status` | 真实读取 `GET /health` 与 `GET /ready`，区分正常、依赖降级和 API 状态未知 |

## 目录边界

```text
web/src/
├── api/           # 同源 HTTP client、运行时契约解码与按能力组织的 API 函数
├── layouts/       # 四类工作台布局
├── pages/         # 按工作台和领域组织的路由页面
│   └── system/status/ # 系统探针页面、状态 hook 与对应测试
├── components/    # 公共、业务和图表组件
├── routes/        # 路由注册
├── mocks/         # 除系统探针外的当前业务演示数据，后续按领域替换为 API 适配
├── stores/        # 用户、主题、布局和环境状态
└── types/         # 业务类型与 API 契约
```

`src/api/httpClient.ts` 只接受同源绝对路径，并统一处理 JSON、超时、调用方取消、请求关联和浏览器侧耗时；错误显式区分 `http`、`gateway`、`network`、`timeout`、`cancelled` 与 `contract`。`src/api/systemApi.ts` 对探针响应做运行时解码，避免把 TypeScript 静态类型误当成网络边界验证。`src/pages/system/status/useSystemStatus.ts` 并行获取两类探针，并在卸载、刷新与竞态时取消或丢弃过期结果。

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

当前只有 `/system/status` 对 Go API `/health` 和由 MySQL 支撑的 `/ready` 的读取是真实联调。抽奖、活动、积分、Feed、MCP、Agent 等业务页面仍主要使用 `src/mocks/growthOsMockData.ts`。仓库已有第 17 节 `internal/lottery/domain` 的 Strategy/Award 配置对象，以及第 18 节 `lottery_strategy` / `lottery_strategy_award` 两张表；但尚无 Repository、业务写路径、概率算法和业务 API。`/lottery` 的客户端随机演示与文案不会读取这两张表，不构成领域对象运行调用链，也不代表一次抽奖结果、Redis 锁或发奖已经实现。

第 22 节接入第一个真实 React 抽奖页时，必须显式替换或隔离对应 Mock，通过同源 API 获取服务端决定的结果，并保留 `reward`、`no_reward` 与系统错误的不同语义。在此之前，不把前端 `LotteryPrize` 展示类型反向用作 Go 领域或数据库契约，也不把表的行级 `updated_at` 当成前端聚合版本、ETag 或缓存失效标记。

第 15 节已做正常、MySQL 不可用和 API 离线三种状态的真实浏览器关联验收，并核对刷新、请求关联与可访问性。该验收只证明当前环境中的联调和展示契约，不构成性能、持续可用性或生产就绪声明。

每节新增或调整前端 API 时，必须同步写入 [章节 API 记录](../api/lessons/README.md)。
