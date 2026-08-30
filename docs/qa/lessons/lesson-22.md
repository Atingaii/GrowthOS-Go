# 第 22 节 QA：React Lottery 真实联调、Credits 工作台与视觉验收

- **对应章节：** [实现 React Lottery 真实页面](../../course/part-03/lesson-22-react-lottery-page.md)
- **API 记录：** [第 22 节前端调用边界](../../api/lessons/lesson-22.md)
- **设计推导：** [第 22 节第一性原理设计手记](../../design-thinking/lessons/lesson-22.md)
- **面试复盘：** [第 22 节面试问答](../../interview/lessons/lesson-22.md)
- **后端前置决策：** [ADR-0018](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)
- **设计验收：** [仓库根目录 design-qa.md](../../../design-qa.md)
- **分支：** `codex/lesson-22-react-lottery-page`
- **起点：** `9e3ed50`（第 21 节最终文档检查点）
- **前端传输与 Lottery 纵向切片：** `cbb87d6`、`41a7833`、`428ae0d`
- **Compose 发布快照：** `9cc2d07`
- **工作台视觉与响应式迭代：** `3b22628`、`cbd64da`、`4c094ce`、`8027854`、`379b1ee`、`3dfb99c`、`3859878`、`9529a28`、`8732002`
- **可访问性收口：** `fc9c942`
- **路由级性能拆分：** `06a4a38`
- **验收日期：** 2026-08-30

> 本节验收的是一个真实调用第 21 节 ephemeral selection API 的 React 页面，以及一轮以 `credit.linux.do` 为视觉语言参考的前端工作台重构。Lottery 的结果不再由浏览器 Mock 决定；但它仍不是正式 Draw：没有认证/授权、资格、次数、扣积分、库存预占、结果持久化、幂等重放、发奖、审计账本、限流或 Redis 业务能力。除 Lottery 响应外，页面中的账户、活动、Feed、积分、优惠券和运营数据都是明确标注时间的本地演示快照，不得写成实时生产数据。

## 1. 验收结论摘要

第 22 节通过以下边界：

1. `/lottery` 通过同源 POST 调用真实 GrowthOS-Go endpoint，不含请求体、query、`Idempotency-Key` 或自动重试；
2. Strategy/Award 的 `uint64` ID 从输入、URL、运行时解码到 React 渲染始终保持十进制 string，包含 MaxUint64 边界；
3. Hook 将 `idle / selecting / success / error` 建模为判别联合，并用同步 in-flight guard、`AbortController` 与 generation 防重复提交、卸载写回和陈旧响应覆盖；
4. `reward` 只表述为“选中了奖励候选”，`no_reward` 是正常成功；页面不声称中奖、发奖或形成可恢复 Draw；
5. 用户、Admin、MCP、Agent 四类工作区复用同一个 `WorkspaceShell`，不再各自复制内容宽度、外边距和顶栏规则；
6. Desktop 使用 231 px 侧栏、72 px 顶栏、1320 px 容器和大屏 48 px 内边距，形成 1224 px 净内容宽度；
7. 首页、Growth Feed、活动、积分、优惠券和个人中心已统一为高密度、扁平、细边框的数据工作台语言；
8. Admin、MCP、Agent 页面复用相同壳层；未接入的写动作保持禁用或明确说明只在浏览器内演示；
9. 搜索、主题、全宽、通知样例、桌面侧栏与移动抽屉都具有真实可观察行为，不再使用无处理函数的装饰按钮；
10. 真实浏览器已覆盖 1719 × 862 桌面视口与 390 × 844 移动视口，并对 Linux DO 参考和实现做同视口拼接人工比对；
11. 最终检查点执行 `make verify` exit 0：Go vet/全量测试、文档检查、19 个前端 test file/152 个 test、TypeScript typecheck 与生产 build 全部通过；
12. 性能复核把 Home、Admin dashboard 与 Recharts 共享层做 route lazy split，生产 build 已无单 chunk 大于 500 kB 的 warning；最终设计审查未保留 P0/P1/P2。

## 2. 验收对象与环境

| 维度 | 实际对象 |
| --- | --- |
| 前端 | React 19、TypeScript、React Router、Vite、Tailwind CSS |
| 交互/图表 | Lucide React、Recharts、浏览器 Clipboard API |
| 测试 | Vitest + Testing Library + jsdom；另有真实浏览器人工验收 |
| 后端入口 | `POST /api/v1/lottery/strategies/:id/ephemeral-selections` |
| 桌面视口 | 1719 × 862 CSS px，亮色主题、展开侧栏 |
| 移动视口 | 390 × 844 CSS px，移动顶栏与抽屉 |
| 本地演示数据时间 | `2026-03-14 12:00 CST`，由 `MOCK_SNAPSHOT_LABEL` 统一声明 |
| 视觉真值 | 用户提供的已登录参考截图优先；`https://credit.linux.do/` 与官方开源仓库用于交互/实现语境复核 |
| 验收时间 | 2026-08-30，Asia/Shanghai |

公共站点在未登录状态不能还原用户截图中的已登录首页，因此视觉排序是：

1. 用户提供的已登录参考截图，决定该状态下的布局、密度和层级；
2. [`credit.linux.do`](https://credit.linux.do/) 的公开运行状态，补充当前响应与基础交互；
3. [`linux-do/credit`](https://github.com/linux-do/credit) 官方仓库，补充公开实现语境；
4. GrowthOS 自身业务事实与品牌，不复制 Linux DO 的身份、账户数据或内容。

## 3. 证据矩阵

| 风险命题 | 代码/测试/浏览器证据 | 结果 | 不能外推 |
| --- | --- | --- | --- |
| 浏览器仍在本地随机“开奖” | [`lotteryApi.ts`](../../../web/src/api/lotteryApi.ts)、[`useEphemeralLotterySelection.ts`](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts)、真实浏览器网络请求 | 通过 | 已形成正式 Draw |
| 超过 JS safe integer 的 ID 损坏 | canonical uint64 string 校验、运行时 decoder、MaxUint64 测试 | 通过 | 任意第三方 JSON 都可信 |
| POST 失败后偷偷自动再选 | HTTP client/API/Hook 测试断言单次调用；代码无 retry loop | 通过 | 用户再次点击等于恢复原请求 |
| 双击或旧 Promise 覆盖结果 | 同步 controller guard、generation、abort、陈旧 resolve/reject 测试 | 通过 | 服务端 exactly-once |
| UI 把候选写成已中奖 | 页面文案与反向断言，不出现“恭喜中奖/已经发放” | 通过 | Reward 候选一定可兑付 |
| 多工作区各自漂移布局 | User/Admin/MCP/Agent layouts 全部组合 `WorkspaceShell` | 通过 | 所有未来页面自动一致 |
| 内容宽度重复叠加 | 1320 px 和响应式 padding 只由壳层拥有；页面从净内容区继续排版 | 通过 | 任意嵌套组件都不会再加 max-width |
| 移动端保留桌面侧栏造成溢出 | 390 × 844 真实浏览器，desktop aside 隐藏、drawer 覆盖打开 | 通过 | 所有设备/浏览器组合已覆盖 |
| 顶栏图标只是装饰 | 搜索、通知、全宽、主题、profile、主动作均有实际处理或路由 | 通过 | 通知来自服务端 |
| Mock 数值冒充实时账务 | `MOCK_SNAPSHOT_LABEL = 2026-03-14 12:00 CST`，页面说明与测试 | 通过 | 页面可用于财务对账 |
| 未接入写能力却给出可点击承诺 | Feed/Admin/审批等写动作禁用或标注本地状态 | 通过 | 后端写链路已实现 |
| 视觉“像”只凭主观印象 | 同视口完整画面拼接 + 关键区域人工核查 + 两个实现视口 | 通过 | 对参考站点逐像素复制 |
| 类型或 React 生命周期回归 | 前端测试、`tsc --noEmit`、生产 build | 通过 | 没有运行时或浏览器兼容风险 |

## 4. Lottery 真实纵向切片

### 4.1 传输层

[`httpClient.ts`](../../../web/src/api/httpClient.ts) 增加无请求体 JSON POST 能力，Lottery adapter 固定发送：

```http
POST /api/v1/lottery/strategies/:id/ephemeral-selections
X-GrowthOS-Demo-Mode: ephemeral-selection
```

前端不会增加 query、body 或 `Idempotency-Key`，也不会把网络、timeout、502、503、504 自动重试。原因不是“POST 永远不能重试”，而是当前接口没有持久 Draw 或幂等结果：响应丢失时无法判断服务端是否已经完成一次选择，透明重试会产生另一次随机选择。

HTTP client 还承担：

- 同源相对 URL；
- timeout 与调用方 signal 合并；
- Request ID 提取和安全错误分类；
- 非 JSON、错误 envelope 和超大/不合法响应的失败关闭；
- timer 与 signal listener 清理。

### 4.2 运行时契约

[`lotteryApi.ts`](../../../web/src/api/lotteryApi.ts) 不相信 TypeScript interface 能验证网络输入。decoder 要求：

- `selection.durability === "ephemeral"`；
- 返回 Strategy ID 与请求 ID 精确一致；
- Strategy/Award ID 都是规范、非零、无前导零且不超过 MaxUint64 的十进制 string；
- Award name 非空、无首尾 Unicode whitespace、无控制字符，且不超过 128 个 Unicode code point；
- outcome 只能是 `reward | no_reward`。

任何不满足的响应都归为 contract error，不会被 React 作为一次可信选择展示。

### 4.3 Hook 生命周期

[`useEphemeralLotterySelection.ts`](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts) 分离了视觉组件与请求所有权：

- 判别联合排除互相矛盾的多个 loading/success/error boolean；
- `activeController.current` 是同步闸门，覆盖 React state 尚未重渲染时的快速重复调用；
- `AbortController` 释放支持取消的客户端工作；
- generation 只允许当前逻辑世代写回，保护忽略 signal 的测试替身和晚到 Promise；
- clear、输入变化和 unmount 使旧结果失效；
- Strict Mode setup-cleanup-setup 不会触发自动 POST。

这里的 abort 不代表服务端事务回滚，前端也没有把取消或未知网络结果表述成“本次没有中奖”。

### 4.4 页面状态与诚实文案

[`LotteryPage.tsx`](../../../web/src/pages/user/lottery/LotteryPage.tsx) 覆盖：

- 空输入与非 canonical uint64 的本地提示；
- idle、selecting、reward、no_reward、not-found、route disabled、gateway/network/timeout、contract 与 cancelled 状态；
- selecting 时按钮、输入与重复请求保护；
- Request ID 只作为故障关联信息；
- 再次操作明确是一轮新的临时选择；
- 刷新后结果消失；
- 没有前端候选列表、伪概率、积分余额、库存或轮盘随机动画。

页面使用“选中了奖励候选”，而不是“恭喜中奖”；Development/Test only 标签是部署边界提示，不是访问控制。

## 5. 共享 Credits 工作台架构

### 5.1 单一几何所有者

[`WorkspaceShell.tsx`](../../../web/src/components/layout/WorkspaceShell.tsx) 是四个工作区的唯一外层几何所有者：

- 展开侧栏：231 px；折叠侧栏：64 px；
- 顶栏：72 px；
- 顶栏和 main 使用相同 `max-width: 1320px`；
- 大屏横向 padding：48 px，因此净内容宽度为 1224 px；
- 中小视口依次降为 32 / 24 / 16 px；
- main 保持 `min-width: 0`，表格或代码型内容在自己的容器中滚动，不推动整个页面。

[`UserLayout.tsx`](../../../web/src/layouts/UserLayout.tsx)、[`AdminLayout.tsx`](../../../web/src/layouts/AdminLayout.tsx)、[`McpLayout.tsx`](../../../web/src/layouts/McpLayout.tsx) 和 [`AgentLayout.tsx`](../../../web/src/layouts/AgentLayout.tsx) 只提供导航、产品名和主动作，不再重复 max-width/padding。

### 5.2 壳层交互

真实浏览器与组件测试共同核查：

- 桌面侧栏折叠/展开，`aria-expanded` 与控制目标同步；
- 移动抽屉打开、遮罩关闭、Escape、焦点圈定、body 滚动锁定与关闭后的焦点返回；
- `Cmd/Ctrl + K` 打开搜索，搜索列表对同一路径去重，选择路由后关闭；
- 主题按钮实际切换 light/dark 并暴露 pressed 状态；
- 全宽按钮实际在 1320 px 与无上限布局间切换；
- 通知面板明确是本地样例，“标记已读”只改浏览器状态，刷新后可恢复；
- 设置与主动作是实际路由，不使用空链接；
- skip link、main landmark、可见 focus ring、dialog label 与 `motion-reduce` 均存在。

## 6. 用户页面与数据诚实性

除 Lottery 服务端响应外，用户和 operator 页面展示的是共享 Mock、页面内固定演示序列或由它们推导的数据；统一的快照标签由 [`growthOsMockData.ts`](../../../web/src/mocks/growthOsMockData.ts) 导出，值为 `2026-03-14 12:00 CST`。它不表示浏览器在该时间点读取过真实账务或运营系统。

### 6.1 首页

[`UserHomePage.tsx`](../../../web/src/pages/user/home/UserHomePage.tsx) 采用参考站点的高密度工作台结构：积分趋势与三项摘要在第一视觉层，近期活动、7 天收入/支出和活动快照在同一节奏下展开。数据使用等宽/表格数字、细分隔和扁平容器；没有用夸张阴影或大面积营销渐变填充数据界面。

### 6.2 Growth Feed

[`GrowthFeedPage.tsx`](../../../web/src/pages/user/growth-feed/GrowthFeedPage.tsx) 明确说明 Feed 是截至该时间点的本地模拟案例。发布、点赞、评论和分享动作保持 disabled，并写明未连接服务端，避免按钮点击后静默无反应或制造成功假象。

### 6.3 活动

活动列表和详情提供真实的本地搜索、状态过滤与路由；活动状态、预算、奖励和参与数都带快照边界。由于没有报名/资格/发奖 API，CTA 不声称已经加入、领取或完成任务。

### 6.4 积分、优惠券与个人中心

- 积分页使用语义 table、列标题和可水平滚动的局部容器；余额/流水明确不是实时账户或可兑付资产；
- 优惠券提供真实筛选和 Clipboard API 复制，成功、失败与定时器清理由测试覆盖；复制不等于核销或发放；
- 个人中心使用语义 `dl` 展示只读字段，明确不验证身份、不修改账号、不写后端。

## 7. Operator 页面

Admin、MCP 与 Agent 共用壳层和设计 token，但保留各自信息架构：

- Admin dashboard/campaigns 是带快照声明的运营视图；创建、编辑、发布等未接入能力禁用；
- MCP dashboard/servers/tools/permissions/audit 是本地节点与风险样例，不冒充实时 gateway telemetry；
- Agent workspace 可以在浏览器内添加内存任务，但不调用 Agent、MCP Tool 或 GrowthOS 写接口；刷新后不保留；审批等后端能力保持禁用；
- 尚未实现的通用 operator 模块以能力边界和不可用动作呈现，不用“成功” toast 掩盖空实现。

这种处理让页面在视觉上完整可学习，同时不把静态演示数据写成已经交付的业务系统。

## 8. 真实浏览器与视觉验收

### 8.1 桌面 1719 × 862

在真实浏览器的 1719 × 862 CSS 视口核查了：

- `/home` 首屏的 231 / 72 / 1320 / 1224 几何关系；
- `/feed`、`/campaigns`、活动详情、`/points`、`/coupons`、`/profile` 的密度、路由和局部滚动；
- `/lottery` 的输入、loading、成功/无奖励、错误及真实 API 请求；
- `/admin/dashboard`、`/admin/campaigns`、`/mcp/dashboard`、`/agent/workspace` 与通用 operator 页面；
- 搜索、侧栏、主题、全宽、通知和路由动作。

用户参考截图的原始位图为 3438 × 1724 px，对应 1719 × 862 CSS px 的 2× 密度。验收时把参考和实现规范化到相同 CSS 视口后，生成一次性完整画面拼接输入，并人工检查侧栏、顶栏、首屏趋势区、摘要区和近期概览。该临时比较图只用于本轮检视，没有作为仓库资产保留或链接。

### 8.2 移动 390 × 844

在 390 × 844 CSS 视口核查了：

- 桌面侧栏不占据布局宽度；
- 移动顶栏、Logo、搜索、通知和主题按钮不互相挤压；
- 抽屉宽度不超过视口，遮罩与关闭按钮可触达；
- 页面标题、筛选、图表、卡片、表格和 Lottery disclosure 不产生页面级横向溢出；
- 局部宽表保持在局部滚动容器内；
- 点击目标、focus ring 和正文最小可读性保持可用。

参考输入没有同状态的移动截图，因此移动结论是响应式行为验收，不声称对参考站点移动版逐像素复刻。

### 8.3 视觉语言结论

通过项：

- 白色/浅灰画布、1 px 中性边框、紧凑 6/8/12 px 圆角、弱化阴影；
- 系统字体栈、清晰中文层级、等宽标识和 tabular numbers；
- violet 作为 GrowthOS 主色，蓝色趋势、绿色正向、红色支出/风险；
- Lucide 图标保持统一 stroke、尺寸与辅助文本，不复制参考站 Logo；
- 数据面采用扁平分区和行式密度，不把所有内容包成大阴影卡片；
- copy 优先说明事实、时间与能力边界，避免“已发放/实时/在线”等无证据词。

## 9. 组件与质量门禁

文档编写时实际执行：

```bash
cd web
pnpm run test
pnpm run typecheck
pnpm run build
```

结果：

| 门禁 | 当前结果 | 备注 |
| --- | --- | --- |
| 全仓门禁 | `make verify` exit 0 | 包含 Go vet/全量测试、`doccheck` 与完整前端门禁 |
| Vitest | 19 个 test file、152 个 test 全部通过 | 最终检查点实测数量 |
| TypeScript | `tsc --noEmit` exit 0 | 包含 API decoder、Hook 与页面类型 |
| Production build | Vite build exit 0 | route lazy split 后无单 chunk > 500 kB warning |
| 真实浏览器 | 1719 × 862 与 390 × 844 通过 | 覆盖桌面/移动、交互和主要路由 |
| 视觉比较 | 同视口完整画面与关键区域人工检视通过 | 临时图片不入库 |

最终检查点于 2026-08-30 在第 22 节工作树执行：

```bash
make verify
git diff --check
```

两条命令均 exit 0；`doccheck` 同时验证 101 节注册表、显式分部范围、已完成章节 API/QA/设计手记/面试问答与本地 Markdown 链接。最终前端产物仍为 19 个 test file、152 个 test，build 无单 chunk 大于 500 kB warning。

## 10. 本轮暴露并修复的问题

1. **旧 UI 与目标信息架构不一致。** 顶部超宽横向导航、过大空白和优惠券式大卡片无法表达参考站点的紧凑账户工作台；重构为共享侧栏、顶栏和数据区。
2. **页面级 max-width/padding 重复。** 多个布局各自控制容器会在复用时形成双重内缩；收敛到 `WorkspaceShell` 单一所有者。
3. **移动端把桌面壳层缩小。** 改为真正的移动抽屉和顶栏，并补焦点、滚动锁和关闭恢复。
4. **搜索存在重复路由与焦点风险。** 搜索源按 path 去重，并补 dialog focus containment、Escape 和关闭后的焦点返回。
5. **通知和动作容易被误认成后端能力。** 通知改为显式本地样例；未接入写操作保持禁用；真实路由/主题/复制等动作才可点击。
6. **Mock 数据时间不明确。** 统一引入 `MOCK_SNAPSHOT_LABEL`，用户页和 operator 页都声明 `2026-03-14 12:00 CST`。
7. **Lottery 视觉可能夸大业务完成度。** 删除本地开奖/伪候选，保留真实 endpoint、临时性和“候选而非中奖”的文案。
8. **移动 Lottery disclosure 重复或挤压。** 响应式信息区重新分工，避免同一说明在窄屏重复出现并造成过长首屏。
9. **图标与装饰不统一。** 统一使用 Lucide；装饰性图标隐藏于辅助技术，动作图标由按钮 label 提供语义。
10. **初始生产包出现单 chunk 大于 500 kB warning。** `06a4a38` 将 Home、Admin dashboard 和 Recharts 共享层按路由惰性加载；最终 build 中入口 433.34 kB、共享 ProductPage/Recharts 353.32 kB、Home 52.07 kB、Admin 2.08 kB，warning 已消失。该结果只说明当前构建分块，不等于 Core Web Vitals 或真实网络性能已经达标。

## 11. 清理与证据保留

本轮视觉 QA 曾生成一次性浏览器截图、同视口拼接图和聚焦比较输入。它们只用于人工检视，没有进入 Git，也没有成为文档链接；最终复核后已删除仓库内 `.playwright-cli/`、`output/`、`web/dist/`，以及本轮专用的 `/tmp/growthos-l22-browser-secrets.nhMVtK` 与 `/tmp/linuxdo-credit-ui.s3RaVy`。

隔离 Compose project `growthosl22browserfa17592b0bc29accfaa16441` 的 6 个容器、3 个网络、2 个 volume 与 4 个本轮 acceptance image tag 已按精确 project/tag 删除；长期 `growthos` Compose 栈、用户其他 Docker project、依赖缓存和已有业务数据没有被改动。仓库跟踪的 `deploy/compose/compose.lesson21-acceptance.yaml` 是可复用验收资产，复核后保留。

需要长期保留的证据是：

- 源代码与组件测试；
- 本 QA 和 `design-qa.md` 的比较条件、视口、状态与结论；
- 用户提供的会话截图作为当次需求输入记录，而不是仓库运行依赖；
- 公开 URL 与官方仓库链接。

不会删除用户已有 Docker 容器、数据库、Volume、依赖缓存或业务文件。

## 12. 剩余风险与非阻断项

1. Route lazy split 已消除当前 >500 kB build warning，但静态 chunk 大小不能证明真实网络、解析、交互延迟或 Core Web Vitals 达标；仍需性能预算和真实浏览器测量；
2. 本节没有真实登录身份、服务端通知或账户 API，大多数界面仍是本地快照；
3. 外部头像 URL 只是 Mock asset，离线或第三方不可用时应有占位策略；
4. 仅人工覆盖两个代表性视口，不能代替浏览器矩阵、自动视觉回归、屏幕阅读器与真实设备测试；
5. 参考站点未登录公共状态与用户提供的登录状态不同，后续若参考站点改版，不能把变化自动当成 GrowthOS 回归；
6. 本节没有正式 Draw、资格、次数、积分、库存、发奖、审计、幂等、限流与 Redis 业务链；
7. 取消客户端等待不等于取消服务端已开始的选择；502/503/504 也不等于服务端一定没有执行；
8. Request ID 不是 Draw ID、幂等键、trace 或审计凭证；
9. Development/Test header 与 feature flag 都不是认证或授权。

## 13. 能准确表述与不能表述

可以表述：

> 将 React Lottery 页接入 Development/Test 专用 ephemeral selection API，使用 canonical uint64 string、运行时响应解码、AbortController、generation 与同步 in-flight guard 管理真实请求；同时抽象 231 px 侧栏、72 px 顶栏、1320 px 内容容器的共享 WorkspaceShell，以带时间标记的本地快照重构用户/Admin/MCP/Agent 工作台，并通过前端测试、TypeScript、生产构建及 1719 × 862 / 390 × 844 真实浏览器验收。

不能表述：

- 已上线正式抽奖或完整 GrowthOS 产品；
- 已实现用户认证、资格、扣积分、库存锁定、发奖或对账；
- 使用 Redis 锁、RabbitMQ 事件或 PostgreSQL 业务链；
- 自动重试确保 exactly-once；
- 页面数据来自实时生产账户；
- 视觉已逐像素复制 Linux DO，或得到对方官方设计认可；
- 两个浏览器视口等于完成所有兼容性与无障碍认证；
- 测试通过等于没有运行时、网络或业务风险。

## 14. 验收结论

第 22 节通过。前端第一次把服务端 Lottery 选择真实、无损且诚实地呈现给用户，同时把此前分散的页面收敛为可复用的 Credits 风格工作台。

这次通过的核心不是“页面更像参考图”，而是视觉、交互和业务事实被同一套边界约束：真实能力可以操作，Mock 能力标注时间，未实现能力禁用，未知 POST 结果不自动重试，临时选择不包装成中奖。后续可以在这套壳层上继续实现正式 Draw 与更多真实查询，但必须先补齐对应后端不变量，不能只升级前端文案。
