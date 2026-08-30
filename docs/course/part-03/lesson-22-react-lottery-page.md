# 第 22 节：实现第一个真实 React 抽奖页

> 本节把第 21 节 development/test 专用的 ephemeral selection 接入 React `/lottery` 页面，并移除浏览器 `Math.random()` 决定结果的旧 Mock。页面展示的是服务端返回的**临时候选选择**，不是正式 Draw、中奖记录、库存预占或权益发放。与此同时，本节以 `credit.linux.do` 的工作台信息架构为视觉参照，建立四类工作台共用的 `WorkspaceShell`，并把仍使用 Mock 的页面明确标成快照或建设中状态。

## 1. 本节目标

完成本节后，仓库应能回答以下问题：

1. React 页面怎样在不破坏第 21 节请求边界的前提下发起一次真实 selection；
2. 为什么无请求体的 `POST` 需要独立 transport，而不是给通用 client 随意增加 `body`；
3. StrategyID/AwardID 怎样以十进制 string 穿过表单、URL、JSON decoder 和页面，避免 JavaScript `number` 精度丢失；
4. 为什么 TypeScript interface 不能替代对网络 JSON 的运行时验证；
5. `reward`、`no_reward`、HTTP 拒绝、网关失败、网络失败、timeout、取消和契约漂移怎样映射为不同页面状态；
6. 为什么页面与 Hook 都不能透明重试当前接口；
7. 快速重复点击、输入变化、组件卸载和旧 Promise 晚到时，怎样避免重复或过期结果覆盖当前 UI；
8. 为什么请求进行中的动画只能表达“等待”，不能参与随机选择；
9. 怎样为表单错误、请求进度、结果和失败建立可访问名称与 live region；
10. 怎样在桌面和移动端分别组织真实调用链、功能边界与操作区域；
11. 为什么用户、Admin、MCP、Agent 四套工作台共用壳层，却不能把它们的 Mock 卡片写成真实后端能力；
12. 怎样把参考站点的侧栏、工具栏、内容密度和扁平视觉转化为可维护的产品壳层，而不是复制页面像素或业务文案。

## 2. 开始前的事实

第 21 节已经有：

- development/test 专用、默认关闭的 ephemeral selection route；
- `POST /api/v1/lottery/strategies/:id/ephemeral-selections`；
- 必需的 `X-GrowthOS-Demo-Mode: ephemeral-selection`；
- 无 body、无 query、拒绝 `Idempotency-Key` 的服务端契约；
- MySQL Strategy 一致快照、服务端加权选择与 `CryptoSource`；
- `reward` / `no_reward` 两种合法 outcome；
- 完整 uint64 的 decimal-string 响应 DTO；
- Request ID、`no-store`、稳定 error envelope 与 Nginx 同源入口。

但第 21 节结束时前端仍然缺少：

- bodyless JSON `POST` transport；
- Lottery API adapter 与运行时 decoder；
- 一次选择的 React 状态 Hook；
- 对 uncertain outcome、取消和竞态的页面恢复语义；
- 使用真实服务端结果的 `/lottery` 页面。

旧页面以浏览器 `Math.random()` 和本地奖品数组模拟抽奖，还出现积分、库存、Redis 锁或发奖式文案。它适合第 14 节展示前端路由，不适合在第 21 节 API 已存在后继续充当业务事实。

还必须承认另一个事实：除了第 15 节系统探针和本节 Lottery selection，活动、Feed、积分、优惠券、MCP 和 Agent 暂无对应真实业务 API。UI 可以重构、筛选本地数组、写入 Clipboard 或创建浏览器内状态，但不能因为交互可点击就宣称服务端业务已完成。

## 3. 先冻结浏览器请求契约

页面只允许形成下面这一类业务请求：

```http
POST /api/v1/lottery/strategies/21003/ephemeral-selections HTTP/1.1
Accept: application/json
X-GrowthOS-Demo-Mode: ephemeral-selection
```

前端应用代码明确不提供：

- request body；
- query string 或 fragment；
- `Content-Type`；
- `Idempotency-Key`；
- seed、ticket、AwardID、用户积分或库存参数；
- 自动重试。

浏览器和代理仍会添加 User-Agent、Host 等正常协议 header；“只发送两个 header”指本节前端 transport 主动构造的应用 header，而不是宣称 wire 上只有两个字段。

### 3.1 为什么 Strategy ID 必须由用户明确输入

当前后端没有 Strategy 列表、公开 Activity identity 或默认演示 Strategy 查询。页面如果硬编码 `1`、猜测 MaxUint64 或把某个 fixture 当永久存在，就会把测试数据误写成产品契约。

因此表单只接收调用者明确知道的 StrategyID，并解释当前开发数据库必须已经有该配置。找不到时展示关联 404，不自动创建 Strategy，也不回退到 Mock 结果。

### 3.2 为什么不发送 Content-Type

请求没有 payload，就没有需要由 `Content-Type` 描述的表示。无条件发送 `application/json` 容易让维护者误以为 `{}` 是合法请求体，也可能在跨源环境触发不必要的预检。`postJSONWithoutBody` 的类型签名根本没有 `body` 参数，因此调用者不能靠约定之外的对象偷偷加入 payload。

### 3.3 为什么前端也不支持 Idempotency-Key

服务端没有 Draw、结果记录或 key 唯一约束。前端即使生成一个 UUID，也无法让响应丢失后的第二次请求恢复第一次 Award。静默重试只会创建一次新的临时选择观察，并可能返回不同候选。

因此页面把失败后的按钮写成“发起一次新的临时选择”，而不是“重试原抽奖”。Request ID 只用于关联一次 HTTP 处理，不能充当业务幂等键。

## 4. 前端纵向切片

```text
用户输入 canonical StrategyID
  │
  ▼
LotteryPage
  │ form / accessible states / truthful copy
  ▼
useEphemeralLotterySelection
  │ idle | selecting | success | error
  │ duplicate suppression / abort / generation guard
  ▼
lotteryApi.requestEphemeralSelection
  │ path + Demo Header + runtime decoder
  ▼
httpClient.postJSONWithoutBody
  │ same-origin / no-store / timeout / public error envelope
  ▼
Vite proxy 或 Compose Nginx
  ▼
GrowthOS-Go → MySQL snapshot → WeightedSelector → CryptoSource
  │
  ▼
unknown JSON → decoder → typed minimal DTO → Hook → 页面
```

分层责任是：

| 层 | 负责 | 不负责 |
| --- | --- | --- |
| `LotteryPage` | 输入、状态展示、恢复提示、可访问语义 | 拼 URL、解析 JSON、决定 Award |
| `useEphemeralLotterySelection` | 一次请求生命周期、重复抑制、取消、旧结果隔离 | HTTP 字段校验、随机算法 |
| `lotteryApi` | Lottery 路径、header、ID 与成功 DTO decoder | 通用 fetch 错误、UI 文案 |
| `httpClient` | 同源传输、JSON、timeout、取消、公开错误与相关元数据 | Lottery outcome 的业务解释 |
| Go 服务 | 读取 Strategy 并选择 Award | 正式 Draw、库存、积分扣减与发奖 |

页面组件不直接 `fetch`，Hook 不直接理解 snake_case JSON，通用 client 也不知道 `reward` 是什么。

## 5. 通用 HTTP client 怎样扩展 POST

[`httpClient.ts`](../../../web/src/api/httpClient.ts) 把 GET/POST 的共同机制收敛到内部 `executeJSONRequest`，再提供两个窄入口：

```ts
requestJSON(path, options)
postJSONWithoutBody(path, options)
```

`postJSONWithoutBody` 有意保持以下性质：

- 方法固定 `POST`；
- 只有 `Accept: application/json` 和调用者显式 header；
- `RequestInit` 不含 `body`；
- 不推断 `Content-Type`、`Content-Length` 或 `Transfer-Encoding`；
- 失败只调用一次 fetch，不内置 retry。

两个入口继续共用第 15 节的传输防线：

- 只接受以单个 `/` 开头且不含反斜杠的同源绝对路径；
- `credentials: "same-origin"`；
- `mode: "same-origin"`；
- `redirect: "error"`；
- `cache: "no-store"`；
- 默认浏览器等待预算 5 秒；自定义值必须是 100～30,000 ms 的安全整数；
- 外部 `AbortSignal` 与内部 timeout 统一进入一个 `AbortController`；
- 成功与 error 都要求 JSON，非 JSON 502/503/504 单独归为 gateway；
- error body 必须符合公开 envelope，header/body Request ID 同时存在时必须相等；
- 返回浏览器观测耗时和可用 Request ID，不伪造服务端耗时。

### 5.1 timeout 与取消不是同一件事

内部计时器触发时归为 `timeout`；页面或 Hook 主动取消时归为 `cancelled`。两者都会停止当前浏览器等待，但都不能证明服务端一定没有完成同步选择。

这也是不自动重试的另一个理由：客户端取消的是对响应的等待，不是可验证地回滚一个后端业务事实。

## 6. Lottery adapter 的运行时契约

[`lotteryApi.ts`](../../../web/src/api/lotteryApi.ts) 对输入和响应执行运行时检查，网络数据首先是 `unknown`，通过后才映射为前端 DTO。

### 6.1 canonical uint64 string

合法 ID 的规则是：

```text
^[1-9][0-9]{0,19}$
且数值不大于 18446744073709551615
```

实现不把值转换成 JavaScript `number`。19 位以内由长度判定，20 位时与 MaxUint64 的等长十进制字符串按字典序比较。规范十进制不存在前导零，因此等长字典序与数值序一致。

下列值在 fetch 前失败：

```text
""  "0"  "01"  "+1"  "-1"  "1.0"  "1e3"
" 1"  "1 "  "１２"  "18446744073709551616"
```

### 6.2 成功响应必须满足什么

服务端 JSON：

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

decoder 要求：

- 根和 `selection` / `award` 都是非数组 object；
- `durability` 精确等于 `ephemeral`；
- `strategy_id` 必须精确等于本次请求的 StrategyID；
- AwardID 是 canonical uint64 decimal string；
- Award name 非空、首尾没有 Unicode White_Space、不含控制字符或孤立 surrogate，最多 128 个 Unicode code point；
- outcome 只能是 `reward` 或 `no_reward`。

服务端可以增加未知字段，decoder 会忽略它们并只返回页面需要的最小 DTO。这允许向后兼容的 additive change，但不会放松既有必需字段。

### 6.3 为什么核对响应 StrategyID

同源代理路由错误、服务端 bug 或旧响应串线都可能返回另一个 Strategy 的结果。页面不能只看 Award 合法就接受它。请求 ID 与响应 StrategyID 必须分别承担 HTTP 关联和业务输入回显校验，二者不能互相替代。

## 7. Hook 的并发与生命周期

[`useEphemeralLotterySelection.ts`](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts) 使用封闭状态联合：

```text
idle
selecting(strategyId)
success(strategyId, response)
error(strategyId, ApiClientError)
```

它建立四条关键防线。

### 7.1 挂载不自动请求

Hook 初始化和 React StrictMode 的 setup-cleanup-setup 都保持 `idle`。只有明确调用 `select(strategyId)` 才形成一次 selection，避免开发环境双 effect 或页面预渲染偷偷触发随机 action。

### 7.2 pending 期间抑制所有重复提交

`activeController` 非空时，新的 `select` 直接返回；即使第二次传入另一个 StrategyID，也不会并行发送。页面同时禁用输入和提交按钮，Hook 再提供与 UI 无关的第二层保护。

这不是跨标签页、跨设备或服务端并发控制，只是当前组件实例内的重复抑制。

### 7.3 AbortController 管理资源所有权

每个请求由 Hook 创建 controller。输入变化调用 `clear()`、组件卸载或未来路由离开时，Hook abort 当前等待并释放引用。调用方没有 controller 的所有权，不会在多个组件之间共享一个可变 signal。

### 7.4 generation 阻止旧 Promise 覆盖新状态

仅依赖 abort 不够：测试 double 或某些已经 settle 的 Promise 仍可能晚到。Hook 每次选择或 clear 都递增 generation，回调只有在 generation 仍匹配且 signal 未取消时才允许更新 state。

因此旧请求晚到不会覆盖新的 reward、no_reward 或 error。

## 8. 页面如何保持业务语义诚实

### 8.1 idle 与表单校验

初始按钮禁用，没有默认 Strategy，也不在页面加载时请求。输入使用文本控件和 `inputMode="numeric"`，既保留完整 decimal string，又给移动设备数字键盘提示。

非规范 ID 使用：

- `aria-invalid`；
- `aria-errormessage`；
- 固定帮助文本说明 1～MaxUint64；
- 清楚的“无前导零、无符号”错误。

输入改变会 clear 旧结果，避免页面在 StrategyID 已变时继续显示上一 Strategy 的候选。

### 8.2 selecting

进行中区域使用 `role="status"`、`aria-live="polite"` 与 `aria-atomic="true"`。输入和按钮禁用，Hook 同时抑制重复请求。

旋转或脉冲图标只说明浏览器正在等待。`prefers-reduced-motion` 下动画关闭，页面文字明确声明动画不参与服务端选择。

### 8.3 reward

页面标题是“选中了奖励候选”，并继续显示：

> 这不是中奖记录，也不表示库存已预占或奖励已发放。

它展示 StrategyID、AwardID、`ephemeral`、浏览器观测耗时和可用 Request ID，但不展示权重、概率、ticket、库存或虚构 DrawID。

### 8.4 no_reward

`no_reward` 使用 success status，而不是 error alert。页面解释它是算法正常完成后的合法候选，不是系统失败、降级结果或空响应。

### 8.5 error 与 uncertain outcome

| 前端事实 | 页面表达 | 恢复语义 |
| --- | --- | --- |
| 404 `lottery_strategy_not_found` | 没有找到这个 Strategy | 检查开发库配置，不自动创建 |
| 404 `route_not_found` | 临时选择接口未启用 | 检查 feature gate/版本 |
| 400 | 浏览器与服务契约不一致 | 用 Request ID 排查，不回退 Mock |
| 503 | 服务暂时无法给出可信结果 | 未形成可查询结果；下一次是新选择 |
| 502/504、gateway/network/timeout | 无法确认本次结果 | 不声称未中奖，也不透明重试 |
| contract | 无法验证响应契约 | 拒绝展示未知 JSON |
| cancelled | 页面停止等待 | 不生成本地恢复结果 |

页面只使用审核后的稳定文案，不把底层 `Error.message`、SQL、代理详情或原始响应渲染给用户。Request ID 仅在存在时展示，并明确标成故障关联信息。

## 9. 响应式信息架构

桌面 `xl` 断点使用 7/5 列：左侧是选择工作台，右侧是调用链、无自动重试原因和未实现能力。较窄视口改成单列，调用链与边界使用原生 `<details>/<summary>` 渐进披露。

桌面 region 与移动 details 是两套响应式呈现，不会让同一个可访问节点同时承担两种布局。测试分别按“桌面真实调用链/功能边界”和“移动真实调用链/功能边界”命名，避免测试因为同名隐藏副本产生歧义。

## 10. 为什么同时建立共享 WorkspaceShell

用户提供的视觉目标不是换一组颜色，而是 [`credit.linux.do`](https://credit.linux.do/) 一类高密度数据工作台：固定侧栏、顶部命令区、清楚的内容轨道、扁平卡片和真实可操作的导航。

本节提取 [`WorkspaceShell.tsx`](../../../web/src/components/layout/WorkspaceShell.tsx)，由 User、Admin、MCP 和 Agent 四套布局提供导航配置。当前几何基线为：

- 桌面侧栏展开宽度 231px，折叠后 64px；
- 顶部工具栏高度 72px；
- 内容 wrapper 最大 1320px；
- 大屏左右 padding 48px，形成 1224px 净内容轨道；
- 1px zinc 边框、白/浅灰平面和 violet 主色；
- 普通业务 Surface 圆角不超过 12px，不使用大面积渐变和默认厚重阴影。

参考的是信息密度与交互层级，不复制 Linux DO 的业务数据、账号、品牌资产或后端能力。

### 10.1 壳层的真实交互

- React Router 客户端导航与嵌套路由 active 匹配；
- `/`、`/rewards` 以及 Admin/MCP/Agent index route 的 alias active 状态；
- 桌面侧栏折叠，并在折叠后保留 link accessible name；
- 移动抽屉、body scroll lock、Escape 关闭、焦点进入/返回和 Tab 围栏；
- `⌘K` / `Ctrl+K` 搜索页面，结果使用客户端路由；
- 搜索打开后聚焦输入，关闭后返回触发控件；
- 明暗主题切换；
- 内容全宽/固定轨道切换；
- 本地演示通知的打开与“标记样例已读”。

通知明确写着“不读取服务端”，已读状态刷新后恢复；它不是后台消息中心。顶部 `+` 是有明确 accessible name 的导航 link，不代表创建或提交业务写操作。

### 10.2 壳层的可访问边界

- “跳到主要内容” skip link；
- 唯一具名 `<main id="main-content">`；
- 每套 workspace 有独立导航名称；
- active link 使用 `aria-current="page"`；
- 折叠、主题和全宽按钮暴露 expanded/pressed 状态；
- dialog 使用 `aria-modal`，交互关闭后恢复焦点；
- 装饰性图标 `aria-hidden`；
- 全局 `prefers-reduced-motion` 降低非必要动效。

这些自动化和浏览器检查能证明结构与主要键盘路径，不等于完成了所有浏览器、缩放、屏幕阅读器组合的 WCAG 合规认证。

### 10.3 图表页面为什么按路由拆包

用户首页与 Admin 大盘会引入图表库，但 Lottery、Feed、MCP、Agent 等路由不应仅因为共用壳层就同步下载这些页面模块。[`appRouter.tsx`](../../../web/src/routes/appRouter.tsx) 使用 React Router `lazy` 延迟加载 `UserHomePage` 和 `AdminDashboardPage`，让共享布局保持常驻、图表页面在命中对应路由时再加载。

这项优化只改变前端 bundle 的加载边界，不把快照图表变成实时数据，也不改变 Lottery API 请求时机。生产 build 的 chunk 输出可以证明模块已拆分；实际首屏收益仍需浏览器网络与性能采样，不能只凭代码形式宣称。

## 11. 其他页面为什么也重构

共享壳层如果只包住 Lottery，而其他 route 继续使用“大 Hero + 巨型渐变卡片”，导航切换会产生明显的产品断裂。因此本节把已有页面收敛到同一信息架构，但没有扩大它们的后端事实来源。

| 页面 | 本节真实交互 | 数据/写入边界 |
| --- | --- | --- |
| 用户首页 | Router link、图表 hover | 统一 `MOCK_SNAPSHOT_LABEL` 的静态积分/活动快照；非实时账务 |
| Growth Feed | 关联活动 Router link | Feed/计数为 Mock；发布、点赞、评论、分享禁用 |
| 活动列表/详情 | 本地分类筛选、详情路由 | Mock；不报名、不扣预算、不发奖励 |
| 积分中心 | 语义化摘要与账单表 | Mock；不是实时积分账户 |
| 优惠券 | 本地状态筛选、真实 Clipboard 写入及成功/失败反馈 | 不核销、不发券、不写服务端 |
| 个人中心 | 语义化展示现有 mockUser | 不代表认证、会话或安全设置已实现 |
| Admin 活动 | 本地搜索 | 创建、编辑、发布禁用 |
| Admin 大盘/模块 | 密集摘要与模块边界 | Mock/建设中，不是实时运营流 |
| MCP 控制台 | 只读快照 | 不连接 JSON-RPC/SSE 实时观测或权限写入 |
| Agent 工作台 | 创建当前页面内存中的本地演示任务 | 不调用 Agent/MCP/写接口；审批按钮禁用 |
| 系统状态 | `GET /health`、`GET /ready` | 第 15 节已经真实联调，不因本节改写语义 |
| Lottery | 一次真实 ephemeral selection | 本节唯一新增的真实业务 API 消费 |

静态样本统一显示快照时间，使页面日期按快照解释，而不是按当前系统日期误判为实时状态。

## 12. 测试怎样分层证伪

| 测试层 | 主要要证伪的风险 |
| --- | --- |
| `httpClient.test.ts` | POST 意外带 body/framing header；非 JSON 被采用；timeout/取消混淆；失败自动重试 |
| `lotteryApi.test.ts` | uint64 被数值化；路径/query/header 漂移；坏响应穿过 decoder；未知 outcome 被接受 |
| `useEphemeralLotterySelection.test.tsx` | mount 自动请求；重复 pending；卸载未取消；旧 Promise 覆盖新结果；失败透明重试 |
| `LotteryPage.test.tsx` | reward 写成中奖/发奖；no_reward 写成错误；无效 ID 被提交；原始错误泄漏 |
| `UserLayout.test.tsx` | active alias、搜索路由、焦点恢复、移动抽屉、主题/通知语义回归 |
| 用户页测试 | 本地筛选/Clipboard 等真实交互失效，或 Mock 边界文案消失 |
| Operator 页面/布局测试 | 三套壳层语义漂移、写操作假装可用、页面重新引入大渐变/大圆角 |
| TypeScript/build | 类型和生产 bundle 不能生成 |
| 真实浏览器/Compose | jsdom 单测绿色但代理、布局、焦点或真实 API 链路不成立 |

单元测试不能证明随机公平、MySQL 数据正确、浏览器兼容矩阵、生产容量或正式业务闭环；这些仍由后端证据、真实联调和未来章节承担。

## 13. 建议提交学习顺序

分支：`codex/lesson-22-react-lottery-page`

建议依次比较：

1. `9e3ed50..cbb87d6`：把 GET-only client 重构为可复用 JSON executor，并增加 bodyless POST；
2. `cbb87d6..41a7833`：增加 Lottery adapter、canonical uint64 与 success decoder；
3. `41a7833..428ae0d`：增加 Hook、页面状态和真实服务端 selection；
4. `428ae0d..9cc2d07`：发布 lesson-22 Compose 镜像/版本快照；
5. `9cc2d07..4c094ce`：重构 Lottery 工作台并分离桌面/移动披露；
6. `4c094ce..8027854`：提取 credit-style `WorkspaceShell`；
7. `8027854..3dfb99c`：重建首页和用户增长页面，同时冻结 Mock/真实交互边界；
8. `3dfb99c..9529a28`：把 Lottery 纳入统一轨道并加固导航、焦点和响应式交互；
9. `9529a28..8732002`：统一 Admin/MCP/Agent 三套 workspace 与对应测试；
10. `8732002..fc9c942`：把已有文字摘要覆盖的装饰漏斗从 accessibility tree 隐藏；
11. `fc9c942..06a4a38`：按路由延迟加载带图表的用户首页与 Admin 大盘；
12. 后续文档/QA checkpoint：冻结真实浏览器、Compose、全仓门禁和清理证据。

这些提交是学习时间切片，不应 squash 成一个“最终页面”后再倒推设计理由。

## 14. 运行与核查

### 14.1 前端单元与构建

```bash
cd web
pnpm test
pnpm typecheck
pnpm build
```

### 14.2 本地开发

后端必须处于 development/test，已打开 ephemeral selection feature gate，并且数据库中存在合法 Strategy：

```bash
make compose-up
make compose-smoke
```

然后访问：

```text
http://127.0.0.1:8088/lottery
```

宿主 Vite 开发模式也可以使用既有同源 `/api` proxy；浏览器代码不直接保存后端 origin。

### 14.3 API 纵向验收

```bash
make compose-lottery-api-acceptance
```

该命令复验的是第 21 节后端契约在 lesson-22 镜像快照中没有漂移。React 浏览器交互、响应式布局和可访问路径仍需要单独的页面验收，不能用 curl 代替。

实际执行结果、视口、失败记录、业务表 fingerprint 与临时资源清理写在本节 QA；课程正文不把计划命令冒充已经运行的证据。

## 15. 本节不能写进简历的夸大表述

不能表述：

- “完成在线抽奖业务闭环”；
- “抽奖结果已落库、可查询和 exactly-once”；
- “积分已扣减，奖品库存已锁定或权益已发放”；
- “Redis 分布式锁保证幂等/防超卖”；
- “前端自动重试保证结果不丢”；
- “实现用户鉴权、资格、次数、限流、防刷或风控”；
- “所有工作台已经接入实时后端”；
- “通过 UI 动画实现公平随机”；
- “参考站点的代码或品牌已成为项目依赖”；
- “自动化测试等于 WCAG、生产容量或随机公平认证”。

可以准确表述：

> 为 development/test ephemeral Lottery API 实现 React 纵向切片：扩展无请求体同源 POST transport，使用运行时 decoder 保持完整 uint64 string 与封闭 outcome，通过 AbortController、generation guard 和 pending 抑制处理取消/竞态/不确定结果，并以可访问状态区分奖励候选、no_reward 与系统失败；同时抽取四工作台共享的高密度响应式壳层，并显式隔离其他页面的 Mock 与浏览器本地交互。

## 16. 下一节停止条件

第 23 节会出现“抽奖策略开始需要规则”的业务变化，但不能在 React 表单里临时加入几个 `if`，也不能让客户端复制权威规则。继续之前必须回答：

- Strategy 规则属于哪个领域对象和版本；
- 规则输入由谁提供、怎样验证和审计；
- Activity/public identity 与内部 StrategyID 怎样解耦；
- published/time/audience/qualification 怎样失败关闭；
- 正式 Draw、Participation 与结果查询何时建立；
- 客户端如何只展示服务端决定的资格与结果，而不复制权威规则。

共享工作台同时暴露了另一条平台级问题：当前 `activeRole`、Mock 用户、工作区入口和页面隐藏都不是真实权限系统。它不会被临时塞进第 23 节；课程会先把 Lottery 规则、资格、决策引擎以及 Strategy/Activity 边界演进清楚，再在第一个真实运营写入口出现前，以独立章节建立统一主体—角色—权限—资源—动作—数据范围模型、真实会话、服务端强制授权、前端权限投影和越权验收。这样权限成为公共能力，而不是某个页面的补丁。

在上述业务与平台边界分别落地前，本页继续保持 development/test、显式 StrategyID、无自动重试和非持久化候选语义。

## 17. 关联资料

- [第 22 节 API 记录](../../api/lessons/lesson-22.md)
- [第 22 节 QA](../../qa/lessons/lesson-22.md)
- [第 22 节第一性原理设计手记](../../design-thinking/lessons/lesson-22.md)
- [第 22 节面试问答](../../interview/lessons/lesson-22.md)
- [第 21 节后端 API 契约](../../api/lessons/lesson-21.md)
- [ADR-0018：临时 Lottery Selection API](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)
- [React：Synchronizing with Effects](https://react.dev/learn/synchronizing-with-effects)
- [MDN：AbortController](https://developer.mozilla.org/en-US/docs/Web/API/AbortController)
- [MDN：ARIA live regions](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/ARIA_Live_Regions)
- [MDN：Number.MAX_SAFE_INTEGER](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Number/MAX_SAFE_INTEGER)
