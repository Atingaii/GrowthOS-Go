# 第 14 节 API 记录：前端框架与 Mock 边界

- **章节：** [React TypeScript 前端工程初始化](../../course/part-02/lesson-14-react-frontend-framework.md)
- **日期：** 2026-08-08
- **状态：** Mock
- **QA：** [第 14 节 QA 验收](../../qa/lessons/lesson-14.md)

## 本节 API 变化

本节没有新增真实 Go API 调用。页面通过 `web/src/mocks/growthOsMockData.ts` 使用演示数据，目的是先完成完整 UI 框架和路由，不把尚未实现的后端能力误写为已上线接口。

| 类型 | 边界 | 调用方 | 状态 |
| --- | --- | --- | --- |
| Mock 数据 | `web/src/mocks/growthOsMockData.ts` | 用户端、运营端、MCP、Agent 页面 | Mock |
| 真实 HTTP API | 暂无 | 暂无 | 规划中 |

## 后续领域 API 边界

| 前端模块 | 后续职责 | 计划接入章节 |
| --- | --- | --- |
| `campaignApi` | 活动列表、详情和参与 | 第 30～40 节 |
| `lotteryApi` | 抽奖策略和抽奖结果 | 第 21～24 节 |
| `pointsApi` | 积分余额和流水 | 第 50～52 节 |
| `couponApi` | 优惠券领取、列表和核销 | 第 53～54 节 |
| `feedApi` | Growth Feed 召回和游标分页 | 第 57～60 节 |
| `analyticsApi` | 漏斗、实验和指标查询 | 第 69～72 节 |
| `mcpApi` | Tool 列表、调用和审计 | 第 81～88 节 |
| `agentApi` | Agent 任务、审批和历史 | 第 89～93 节 |

以上是后续真实接口应接入的领域边界，不代表本节已经实现。

## 遗留问题

- 第 15 节需要确定 Go 健康接口路径、响应封装、错误码和前端真实请求适配；
- 真实 API 接入后，应把请求参数和响应示例写入对应章节 API 记录；
- Mock 与真实 API 的切换方式需要在首次联调时补充环境变量和 QA 证据。
