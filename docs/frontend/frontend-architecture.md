# GrowthOS-Go 前端工程架构

## 定位

`web/` 是统一的 React 单页应用，复刻既有 GrowthOS UI 设计，先把整体框架、路由、布局和组件体系搭好，再由后续业务章节逐步接入 Go API。

## 工作台

| 工作台 | 路由 | 当前内容 |
| --- | --- | --- |
| 用户端 | `/`、`/feed`、`/campaigns`、`/lottery`、`/points`、`/coupons` | 使用 Mock 数据呈现增长触达与用户权益 |
| 运营端 | `/admin/*` | 活动、策略、奖品、账户、实验、分析、任务和审计入口 |
| MCP Gateway | `/mcp/*` | 服务、Tools、权限和审计控制台入口 |
| AI Operator | `/agent/*` | 工作区、任务、审批和历史入口 |

## 目录边界

```text
web/src/
├── layouts/       # 四类工作台布局
├── pages/         # 按工作台和领域组织的路由页面
├── components/    # 公共、业务和图表组件
├── routes/        # 路由注册
├── mocks/         # 当前演示数据，后续替换为 API 适配
├── stores/        # 用户、主题、布局和环境状态
└── types/         # 业务类型与 API 契约
```

## 运行方式

```bash
cd web
pnpm install --frozen-lockfile
pnpm dev
pnpm run typecheck
pnpm run build
```

默认开发端口为 `5173`，`/api` 请求代理到 `http://localhost:8080`，可通过 `VITE_API_PROXY` 覆盖。

## 当前事实与后续演进

当前页面主要使用 `src/mocks/growthOsMockData.ts`，不代表对应 Go 领域接口已经完成。第 15 节开始建立真实 API 联调边界；抽奖、活动、积分、Feed、MCP 和 Agent 的真实行为仍按课程章节逐步实现。

每节新增或调整前端 API 时，必须同步写入 [章节 API 记录](../api/lessons/README.md)。
