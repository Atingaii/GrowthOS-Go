# 第 22 节 API 记录：React 消费临时 Lottery Selection

- **章节：** [`docs/course/part-03/lesson-22-react-lottery-page.md`](../../course/part-03/lesson-22-react-lottery-page.md)
- **日期：** 2026-08-30
- **状态：** 已联调；最终验收以本节 QA 为准
- **QA：** [`docs/qa/lessons/lesson-22.md`](../../qa/lessons/lesson-22.md)

## 1. 本节 API 变化

第 22 节没有增加或修改 Go 业务路由，而是为第 21 节 route 增加第一个真实 React 消费者：

| 动作 | 接口 | 调用页面/组件 | 状态 |
| --- | --- | --- | --- |
| 前端接入既有接口 | `POST /api/v1/lottery/strategies/:id/ephemeral-selections` | `/lottery`、`requestEphemeralSelection`、`useEphemeralLotterySelection` | development/test 已联调 |

保持不变的系统探针：

| Method | Path | 前端消费者 | 语义 |
| --- | --- | --- | --- |
| `GET` | `/health` | `/system/status` | 进程 liveness |
| `GET` | `/ready` | `/system/status` | 当前实例 MySQL readiness |

本节没有新增 Strategy 列表/详情/创建接口，也没有新增 Draw、结果查询、积分、库存、发奖、资格、认证或幂等接口。

## 2. 前端调用模块

```text
LotteryPage
  ↓ explicit form submit
useEphemeralLotterySelection
  ↓ one request lifecycle
lotteryApi.requestEphemeralSelection
  ↓ exact Lottery request + runtime decoder
httpClient.postJSONWithoutBody
  ↓ same-origin fetch
Vite proxy / Compose Nginx
  ↓
GrowthOS-Go ephemeral selection route
```

| 文件 | API 责任 |
| --- | --- |
| [`httpClient.ts`](../../../web/src/api/httpClient.ts) | 同源 JSON transport、bodyless POST、timeout/取消、公开 error envelope、Request ID 与浏览器耗时 |
| [`lotteryApi.ts`](../../../web/src/api/lotteryApi.ts) | Lottery 路径/header、canonical uint64、success decoder 与最小前端 DTO |
| [`useEphemeralLotterySelection.ts`](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts) | pending 抑制、AbortController、generation stale guard、显式状态机 |
| [`LotteryPage.tsx`](../../../web/src/pages/user/lottery/LotteryPage.tsx) | 输入、提交、可访问状态和诚实业务文案 |

页面不能绕过 adapter 直接 `fetch`，也不能从 Mock 数组或浏览器随机数决定 outcome。

## 3. 请求 contract

### 3.1 Method 与 path

```text
POST /api/v1/lottery/strategies/:id/ephemeral-selections
```

`:id` 是调用者明确输入的 StrategyID。当前没有列表 API，因此前端不会在加载时猜测、枚举或自动创建 Strategy。

### 3.2 必需 header

前端显式构造：

```http
Accept: application/json
X-GrowthOS-Demo-Mode: ephemeral-selection
```

`X-GrowthOS-Demo-Mode` 只确认调用者理解这是 development/test 非持久化选择，不是登录凭证、身份签名、授权或 CSRF 完整防线。

### 3.3 明确不存在的请求部分

前端 adapter 不允许：

- body；
- query；
- fragment；
- `Content-Type`；
- `Idempotency-Key`；
- seed、ticket、AwardID、用户 ID、积分、库存或奖励参数。

`postJSONWithoutBody` 的 options 没有 `body` 字段，生成的 `RequestInit` 不包含 body，也不主动设置 `Content-Length` 或 `Transfer-Encoding`。浏览器/代理可能按协议产生零长度 framing；这不改变“没有请求 payload”的业务契约。

### 3.4 Fetch 选项

当前请求固定使用：

```ts
{
  method: "POST",
  cache: "no-store",
  credentials: "same-origin",
  mode: "same-origin",
  redirect: "error",
  signal
}
```

这表示浏览器只请求当前 origin 的绝对路径，并拒绝跟随重定向到其他位置。`credentials: "same-origin"` 是传输策略，不表示项目已经实现登录 cookie、session、用户身份或对象级授权。

## 4. StrategyID 输入 contract

前端接受：

```text
1 <= id <= 18446744073709551615
```

且必须是 canonical decimal string：

```text
^[1-9][0-9]{0,19}$
```

合法示例：

```text
1
42
9007199254740993
18446744073709551615
```

fetch 前拒绝：

```text
""  "0"  "00"  "01"  "+1"  "-1"  "1.0"  "1e3"
" 1"  "1 "  "１２"  "18446744073709551616"
```

ID 从表单到路径、decoder 与页面始终是 string，不经过 JavaScript `number`。当前实现也不需要 `BigInt`：规范十进制由长度和与 MaxUint64 的等长字典序比较完成范围判断。

## 5. 成功响应 contract

服务端 wire DTO 保持第 21 节契约：

```json
{
  "selection": {
    "durability": "ephemeral",
    "strategy_id": "21003",
    "award": {
      "id": "1",
      "name": "Reward",
      "outcome": "reward"
    }
  }
}
```

前端 decoder 通过后映射为：

```ts
interface EphemeralSelectionResponse {
  durability: "ephemeral";
  strategyId: string;
  award: {
    id: string;
    name: string;
    outcome: "reward" | "no_reward";
  };
}
```

HTTP metadata 与 data 分离：

```ts
interface ApiResponse<T> {
  data: T;
  status: number;
  requestId?: string;
  elapsedMs: number;
}
```

`elapsedMs` 是浏览器观察到的本次往返时间，不是数据库、selector 或服务端处理耗时。

### 5.1 reward

`award.outcome === "reward"` 的准确含义是：

> 服务端算法选中了配置中的奖励候选。

它不表示：

- 创建了 Draw；
- 用户中奖事实已持久化；
- 积分已扣减；
- 库存已预占或扣减；
- Benefit 已发放；
- 请求可以按同一身份恢复。

因此页面只能写“选中了奖励候选”，不能写“恭喜中奖”“获得奖励”或“已发放”。

### 5.2 no_reward

`award.outcome === "no_reward"` 同样是 HTTP 200 和 decoder success。它表示算法正常选择到配置中的未中奖候选，不是：

- 404；
- 空响应；
- 系统降级；
- 网络错误；
- 为掩盖异常而回退的默认值。

## 6. success runtime decoder

网络 body 首先是 `unknown`。decoder 要求：

| 字段/结构 | 规则 |
| --- | --- |
| root / `selection` / `award` | 非 null、非数组 object |
| `selection.durability` | 精确 literal `ephemeral` |
| `selection.strategy_id` | string，且精确等于本次请求 ID |
| `selection.award.id` | canonical uint64 decimal string |
| `selection.award.name` | 非空；首尾无 Unicode whitespace；无控制字符/孤立 surrogate；最多 128 Unicode code point |
| `selection.award.outcome` | 仅 `reward` 或 `no_reward` |

未知 additive field 可以存在，但不会进入前端公开 DTO。必需字段缺失、类型错误、ID 不匹配、未知 outcome 或非法名字都产生 `contract` error，页面不会尝试修复或降级成 no_reward。

## 7. error envelope 与失败分类

服务端合法 error body 仍为：

```json
{
  "error": {
    "code": "lottery_strategy_not_found",
    "message": "lottery strategy not found",
    "request_id": "req-..."
  }
}
```

通用 client 要求 `code`、`message`、`request_id` 都是非空 string。响应 header 与 body 同时包含 Request ID 时必须一致，否则归为 contract drift。

前端保留六类失败：

| `kind` | 触发条件 | 页面含义 |
| --- | --- | --- |
| `http` | 非 2xx 且 JSON error envelope 合法 | 服务明确拒绝/不可用；可按 status/code 分支 |
| `gateway` | 502/503/504 且不是 JSON API envelope | 代理没有给出可信上游响应 |
| `network` | fetch 没有 HTTP response | 无法确认请求是否到达或完成 |
| `timeout` | 前端等待预算触发 abort | 浏览器停止等待，不证明服务端回滚 |
| `cancelled` | 调用方 signal、clear 或 unmount | 生命周期控制；旧结果不得进入页面 |
| `contract` | 本地输入/路径/timeout 非法，或响应媒体类型/JSON/schema/request ID 不可信 | 前后端或环境契约漂移 |

### 7.1 页面使用的稳定 status/code 分支

| HTTP/code | 页面表达 |
| --- | --- |
| 404 `lottery_strategy_not_found` | Strategy 不存在；检查开发数据 |
| 404 `route_not_found` | route 可能保持默认关闭或前后端版本不一致 |
| 400 | 当前浏览器请求与服务契约不一致 |
| 503 `lottery_selection_unavailable` | 服务没有给出可信 selection；不得当 no_reward |
| 502/504 | 无法确认当前请求结果 |
| 其他合法 HTTP error | 没有可采用的 selection |

页面不直接渲染 server `message` 或底层 error object；它只展示审核后的文案，以及存在时用于故障关联的 Request ID。

## 8. Timeout、取消和重复调用

### 8.1 前端 timeout

通用 client 默认 5 秒，允许显式配置 100～30,000 ms 的安全整数。计时器创建内部 controller；调用方 signal 与内部 controller 组合，finally 清理 timer 和 listener。

第 21 节后端 selection deadline 当前为 3 秒。两个预算属于不同层：前端预算包括同源代理和响应传输，不能把 5 秒写成服务端处理 SLO。

### 8.2 pending 抑制

Hook 在存在 active controller 时忽略新的 `select`，页面同时禁用输入与提交按钮。该机制只覆盖当前 React 组件实例，不是服务端幂等、分布式锁、用户级次数或跨标签并发限制。

### 8.3 clear/unmount

`clear()`：

1. 递增 generation；
2. abort 当前 controller；
3. 释放 active controller；
4. 回到 idle。

组件卸载执行相同的资源回收，但不需要再更新 React state。

### 8.4 stale response guard

Promise 回调只有在 generation 与启动时相同、且 signal 未取消时才可更新 state。这样即使旧 Promise 在取消后晚 settle，也不能覆盖后一次请求的 success/error。

### 8.5 无自动重试

transport、adapter 和 Hook 都不自动 retry。失败后只有用户再次明确提交，才会发送第二个 POST；第二次调用是一项新的 ephemeral selection，可能返回不同 Award。

当前没有 `Idempotency-Key`、DrawID、结果查询或本地恢复缓存。任何“恢复同一结果”说法都不成立。

## 9. UI 状态映射

| Hook phase | 页面行为 | ARIA/交互 |
| --- | --- | --- |
| `idle` | 空结果区，等待合法 ID | 非法 ID 使用 `aria-invalid` / `aria-errormessage`；按钮禁用 |
| `selecting` | 等待服务端，动画不参与选择 | `role=status`、polite live；输入和按钮禁用；`aria-busy` |
| `success/reward` | “选中了奖励候选”及最小 metadata | `role=status`；明确不是中奖/发奖 |
| `success/no_reward` | “本次选中未中奖候选” | 正常 status，不使用 alert |
| `error` | 审核后的错误与可选 Request ID | `role=alert`；再次提交写成“新的临时选择” |

刷新页面后 state 回到 idle；不使用 `localStorage`、session storage 或前端结果缓存恢复 selection。

## 10. 真实与 Mock 边界

本节真实后端消费者只有：

- `/system/status` → `GET /health`、`GET /ready`；
- `/lottery` → `POST /api/v1/lottery/strategies/:id/ephemeral-selections`。

共享 `WorkspaceShell` 的侧栏、搜索、主题、全宽、移动抽屉和演示通知是客户端交互，不是新的服务端 API。通知明确标成只更新浏览器状态。

其他用户/Admin/MCP/Agent 页面继续使用 `growthOsMockData.ts` 或建设中状态：

- 活动筛选、Admin 搜索是本地数组派生；
- 优惠码复制只调用浏览器 Clipboard API；
- Agent 新任务只存在当前组件内存；
- Feed 写操作和 Agent 审批禁用；
- MCP 节点/Tool、大盘、积分、优惠券和账户都不代表实时服务端事实。

这些页面不得被 API 记录解读为已经发布对应接口。

用户首页和 Admin 大盘通过 React Router `lazy` 延迟加载，只是图表页面的前端 bundle 边界；它不会改变上述真实/Mock 数据边界，也不会让进入其他路由时自动发起 Lottery selection。

## 11. 请求示例

浏览器 adapter 等价于：

```ts
await postJSONWithoutBody(
  "/api/v1/lottery/strategies/21003/ephemeral-selections",
  {
    headers: { "X-GrowthOS-Demo-Mode": "ephemeral-selection" },
    decode: (value) => decodeEphemeralSelection(value, "21003"),
    signal,
  },
);
```

它不等价于：

```ts
// 错误：这些请求部分都不在当前 contract 中。
fetch("/api/v1/lottery/strategies/21003/ephemeral-selections?seed=1", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "Idempotency-Key": "retry-1",
  },
  body: JSON.stringify({ award_id: "2" }),
});
```

## 12. 验证入口

### 12.1 transport 与 adapter

```bash
cd web
pnpm exec vitest run src/api/httpClient.test.ts src/api/lotteryApi.test.ts
```

覆盖：bodyless POST、两个显式应用 header、无 query/Idempotency、完整 uint64、reward/no_reward、additive field、坏 schema、name 边界、timeout/取消与单次失败不重试。

### 12.2 Hook 与页面

```bash
cd web
pnpm exec vitest run \
  src/pages/user/lottery/useEphemeralLotterySelection.test.tsx \
  src/pages/user/lottery/LotteryPage.test.tsx
```

覆盖：无 mount 自动请求、pending 抑制、明确第二次提交、unmount abort、generation guard、合法/非法 ID、真实候选/no_reward/error 文案和 false-claim 负向断言。

### 12.3 全前端与真实入口

```bash
make web-verify
make compose-smoke
make compose-lottery-api-acceptance
```

- `web-verify` 证明单元/组件测试、TypeScript 与 production build；
- `compose-smoke` 证明 lesson-22 镜像、同源入口、探针和缺失 Strategy 404 边界；
- `compose-lottery-api-acceptance` 复验第 21 节真实 Nginx→Go→MySQL→CryptoSource contract 和业务表不变；
- 浏览器响应式/键盘/视觉验收仍应记录在第 22 节 QA，不能由上述命令替代。

## 13. 安全、认证与幂等边界

当前已做：

- 同源绝对路径限制；
- same-origin mode/credentials；
- redirect error；
- `no-store`；
- demo acknowledgement；
- canonical ID 与运行时响应校验；
- timeout/取消；
- 最小 DTO 和 Request ID 关联；
- 无自动重试。

当前未做：

- 登录、用户身份、session 或 token；
- 租户和 Strategy 对象级授权；
- TLS/public ingress；
- CORS 产品策略；
- rate limit、防刷、资格和次数；
- 正式 Draw、结果持久化与查询；
- Idempotency-Key 记录/唯一约束；
- 积分、库存和 Benefit 一致性；
- 审计与不可抵赖。

`X-GrowthOS-Demo-Mode` 和 `credentials: "same-origin"` 都不能被描述成已经完成认证。

## 14. 未来兼容性

正式前端抽奖不能把当前 Hook 名称改成 `useDraw` 就直接发布。后续至少需要重新建模：

1. 用户可见 Activity/Campaign identity；
2. published/version/time/audience/qualification；
3. Participation、次数和用户身份；
4. 客户端请求身份、唯一约束、DrawID、结果表和查询；
5. Strategy/Award/算法版本快照；
6. inventory reservation 与 Benefit delivery 状态；
7. unknown outcome 的恢复、补偿和审计；
8. rate limit、abuse prevention 与容量预算；
9. 前端区分 selected、persisted、reserved、delivered、failed/compensating；
10. 旧 ephemeral route 的禁用、保留或迁移策略。

正式 route 可以与 ephemeral route 并存，但不能让后者的候选 DTO冒充最终 Draw DTO。

## 15. 依据与关联资料

- [第 22 节课程](../../course/part-03/lesson-22-react-lottery-page.md)
- [第 22 节 QA](../../qa/lessons/lesson-22.md)
- [第 22 节设计手记](../../design-thinking/lessons/lesson-22.md)
- [第 22 节面试问答](../../interview/lessons/lesson-22.md)
- [第 21 节后端 API 记录](lesson-21.md)
- [ADR-0018](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)
- [Fetch API](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API)
- [AbortController](https://developer.mozilla.org/en-US/docs/Web/API/AbortController)
- [JavaScript Number.MAX_SAFE_INTEGER](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Number/MAX_SAFE_INTEGER)
