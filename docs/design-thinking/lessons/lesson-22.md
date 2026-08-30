# 第 22 节设计手记：从临时选择纵向切片到可信的 GrowthOS 工作台

> 本文不是一份“做了哪些页面”的装修记录，而是一份可以复用的决策档案：需求为什么成立、哪些是当前事实、约束如何改变方案、被放弃的选项为什么暂时不合适、失败时怎样保持业务语义，以及什么证据足以支持结论。
>
> 本节的核心不是“把抽奖做得像真的”，而是让一个真实调用后端、但仍然 **ephemeral（非持久化）** 的选择能力，在 React 页面和共享工作台中被准确表达。

---

## 0. 如何阅读：仍然只问九个问题

第 21 节把工程判断压缩成九问，本节继续沿用同一套框架：

1. **Why：** 为什么现在要做？
2. **Fact：** 当前系统能够证明什么？
3. **Constraint：** 哪些边界不能绕过？
4. **Options：** 有哪些可选方案？
5. **Trade-off：** 为什么选现在这一个？
6. **Failure：** 失败、竞态和误用会怎样发生？
7. **Evidence：** 用什么证据确认没有偏离？
8. **Limit：** 还不能宣称什么？
9. **Trigger：** 出现什么信号时必须重评？

这九问不是固定的文档标题模板，而是一条推理链：

```text
目标
  ↓
事实与约束
  ↓
候选方案
  ↓
权衡后的最小实现
  ↓
失败模型
  ↓
验证证据
  ↓
剩余限制与重评条件
```

如果只写“用了 React、Tailwind、Zustand、Recharts”，读者无法知道这些技术是否解决了正确的问题；如果只写“界面很高级”，读者也无法判断界面有没有把临时结果误报成中奖。本文关注的是可追溯的因果链。

### 0.1 本节真实交付的两条主线

本节最终把两个原本容易分离的工作合在了同一条交付线上：

- **业务纵向切片：** 从浏览器表单、HTTP transport、API adapter、运行时解码、自定义 Hook 到结果渲染，完成一次真实的服务端 ephemeral selection。
- **产品工作台重构：** 把用户端、运营端、MCP 与 Agent 页面收敛到共享的左侧导航、顶部工具栏和高密度内容骨架，并迁移用户首页、活动、积分、优惠券、个人中心和抽奖页。

二者必须一起考虑。一个技术正确但被营销式 UI 包装成“正式中奖”的页面是不可信的；一个漂亮但继续用浏览器随机数决定结果的页面也不是真实纵向切片。

### 0.2 本节最重要的一句边界

当前 endpoint 的精确定义是：

```http
POST /api/v1/lottery/strategies/{strategyId}/ephemeral-selections
Accept: application/json
X-GrowthOS-Demo-Mode: ephemeral-selection
```

它同时满足：

- `strategyId` 是规范的、非零 `uint64` 十进制 **字符串**；
- 没有 request body；
- 没有 query 或 fragment；
- 没有 `Idempotency-Key`；
- 没有客户端自动重试；
- `durability` 必须是 `ephemeral`；
- `reward` 只表示服务端选中了一个奖励候选；
- `no_reward` 是正常业务成功，不是错误、降级或超时；
- 响应不创建 Draw，不扣积分，不预占库存，不发放奖励，也不提供可恢复结果。

后文所有产品文案、交互状态和验证标准都受这组事实约束。

---

## 1. Why：为什么现在要做这件事？

### 1.1 用户真正缺的不是“一个转盘”，而是浏览器到后端的第一条可信链路

在本节之前，前端已有大量视觉页面与 Mock 数据，后端也已经有临时加权选择能力，但两者之间没有一条可以在浏览器中被用户主动触发、被测试精确约束、被面试时清楚解释的链路。

因此需求不能简单表述为“新增抽奖页”。更准确的目标是：

> 让用户输入一个已经存在的 Strategy ID，由服务端完成一次非持久化候选选择；前端只呈现服务端能够证明的结果，并明确告知它不等于正式中奖交易。

这里的价值有三层：

1. **工程价值：** 验证 React、transport、Go API 与数据库策略快照能够串起来。
2. **产品价值：** 让开发者能够看懂输入、调用链、结果、错误和缺失能力。
3. **学习价值：** 用一个窄而完整的纵向切片学习状态、契约、竞态、无障碍和业务语义，而不是先被正式抽奖的全部复杂度淹没。

### 1.2 为什么不是先做正式抽奖？

正式抽奖至少要回答以下问题：

- 谁在抽：用户、租户、会话和权限从哪里来？
- 为什么能抽：活动资格、次数、时间窗和风控规则由谁判断？
- 付出了什么：积分或权益怎样冻结、扣减和补偿？
- 选中了什么：策略版本、候选快照和随机性如何审计？
- 奖品是否可用：库存怎样预占、确认、释放？
- 结果如何恢复：Draw ID、状态机、幂等键和查询接口如何设计？
- 怎样发放：优惠券、积分、实物或第三方权益怎样履约？
- 故障怎么办：超时、重复请求、消息积压、部分失败怎样补偿？

如果在这些合同还不存在时先画转盘、概率和库存，前端会把未来方案伪装成当前事实。纵向切片的第一性原则是：先打通最小真实链路，再围绕已经出现的失败模式扩展协议。

### 1.3 为什么同时重构共享工作台？

抽奖页不是孤立演示，它位于用户完成“浏览活动—查看积分—获取优惠券—发起选择—核查账户”的连续工作流中。旧式 hero、渐变和大量营销卡片适合解释“产品是什么”，不适合回答“我现在在哪里、下一步做什么、结果来自哪里”。

因此工作台重构的目标不是单纯变好看，而是降低三种认知成本：

- **导航成本：** 功能增多后，顶部导航需要折叠或隐藏；左侧分组能持续呈现当前位置和邻近任务。
- **比较成本：** 运营和账务型页面需要同时看到摘要、趋势、明细和状态；大卡片会把关键数据推到折叠线以下。
- **可信成本：** 真实 API、Mock snapshot、本地演示通知和待接入按钮必须在同一界面中被区分。

最终选择的是“flat high-density workspace”：大面积中性色基底、1px 分隔、有限圆角、紧凑排版和稳定的侧栏/顶栏骨架。这里的 flat 不是拒绝所有阴影；模态搜索、移动抽屉和通知浮层仍用层级和阴影表达覆盖关系。它拒绝的是用无业务含义的多层卡片、超大 hero 和装饰性渐变抢占信息层级。

### 1.4 为什么参考 `credit.linux.do`，但不能照抄？

参考站的价值在于它已经解决了一类相似问题：账户型数据、趋势、明细和多入口如何在桌面工作台中共存。我们提取的是可解释的布局原则：

- 左侧固定工作区导航；
- 顶部搜索与全局动作；
- 内容区有稳定最大宽度，也允许全宽；
- 余额、趋势、明细使用紧凑的并列关系；
- 中性色、细边框、低装饰噪声；
- 重要状态通过文字、数字、位置和颜色共同表达。

没有复制的内容包括：

- 品牌名、Logo、文案、真实账户信息；
- 参考站特有的积分规则、接口能力和业务承诺；
- 具体图标组合和逐页内容顺序；壳层的 sidebar/topbar/content 几何只在同视口测量、并确认对 GrowthOS 任务有意义后才固化，没有把参考页的每个视觉像素都当成产品契约；
- 任何登录态数据或用户身份。

这一区分很重要：设计参考应转化为“问题—原则—token—组件”，而不是“截图—逐像素复刻”。

### 1.5 本节的成功标准

成功不是“按钮能点”，而是同时满足：

- 浏览器结果确实来自服务端，不是 `Math.random()` 或奖品常量；
- transport 精确满足无 body、无 query、无幂等键、无自动重试；
- 超时和网络异常不会被谎报成 `no_reward`；
- `reward` 不会被谎报成已中奖或已发奖；
- 页面可以被键盘和读屏理解；
- 桌面与移动端保持相同业务信息，而不是移动端删除关键边界；
- 共享工作台能稳定标记活动路由、支持搜索、抽屉、主题和跳转；
- Mock 页面明确标注 snapshot，不与真实 lottery API 混淆；
- 代码、测试、课程文档、面试 QA 和设计手记对同一合同使用同一组术语。

---

## 2. Fact：当前系统能够证明什么？

### 2.1 先建立“事实账本”

本节把事实分为四级：

| 级别 | 含义 | 本节示例 |
| --- | --- | --- |
| 后端事实 | 服务端与合同能够直接证明 | 返回 ephemeral selection，结果为 `reward` 或 `no_reward` |
| 前端事实 | 浏览器本次会话能够直接证明 | 当前输入、请求阶段、解码后的响应、Request ID、浏览器观测耗时 |
| 演示事实 | 仓库内固定 snapshot 用于展示 | 用户积分、优惠券、活动、账户资料、通知样例 |
| 未来方案 | 尚未实现，只能作为演进方向 | Draw、资格、积分扣减、库存、发奖、幂等重放、限流 |

UI 的每句话都应该能归入其中一个级别。不能把演示事实写成实时事实，也不能把未来方案写成已上线能力。

### 2.2 Lottery 真实纵向切片的事实

#### 浏览器输入

- 用户必须明确输入 Strategy ID；初始值为空；
- 输入只接受 `1` 到 `18446744073709551615`；
- 不允许正负号、小数、空白、前导零或超过 `uint64` 上限；
- ID 始终保留为 string，不经过 JavaScript `Number`；
- 修改输入会清除旧结果并取消当前等待，避免新输入与旧结果错配。

#### HTTP transport

- 只允许同源绝对路径；
- 使用 `POST`，`cache: no-store`、`credentials: same-origin`、`mode: same-origin`；
- empty-body helper 不设置 body，也不为“空 JSON”猜测 `Content-Type`；
- 调用方 signal 和内部 timeout 统一交给 AbortController；
- 错误分为 http、gateway、network、timeout、cancelled、contract；
- 没有 retry loop。

#### API adapter

- endpoint、Demo header 与返回投影集中在 `lotteryApi.ts`；
- 运行时检查 response wrapper、`durability`、请求与响应 Strategy ID 一致性；
- Award ID 仍是规范 `uint64` 字符串；
- Award name 非空、无边缘空白、无控制字符，最大 128 个 Unicode code point；
- outcome 只允许 closed union：`reward | no_reward`；
- 允许服务端添加未知字段，但投影后不让 UI 随意依赖它们。

#### React 状态

状态不是若干可能互相冲突的布尔值，而是判别联合：

```text
idle
  └─ explicit select ─→ selecting
                          ├─ valid reward/no_reward ─→ success
                          ├─ classified failure ────→ error
                          └─ clear/cancel ──────────→ idle

success/error
  ├─ input change/clear ─→ idle
  └─ explicit select ────→ selecting（一次新的临时选择）
```

`selecting`、`success`、`error` 各自携带 Strategy ID；`success` 携带经过解码的响应；`error` 携带安全分类后的 `ApiClientError`。

#### 页面语义

- 页面明确标注 Development/Test only 与非持久化；
- `reward` 显示“选中了奖励候选”；
- `no_reward` 显示“本次选中未中奖候选”，属于成功；
- 页面说明没有 Draw、扣分、库存、发奖和恢复；
- Request ID 只称作故障关联 ID；
- elapsedMs 只称作浏览器观测耗时，不称服务端算法耗时；
- 请求动画只表示等待，不参与随机选择；
- 再次点击是新选择，不是恢复或重试原结果。

### 2.3 工作台重构的事实

共享 `WorkspaceShell` 当前提供：

- 桌面左侧分组导航与可折叠状态；
- 移动端模态抽屉；
- 顶部搜索入口和 `Cmd/Ctrl + K` 快捷键；
- 页面级搜索结果跳转；
- 当前路由活动状态和 `aria-current="page"`；
- `/` → `/home`、`/rewards` → `/points` 等显式 alias；
- 路由变化后关闭临时浮层并把当前页面滚动归零；
- 本地主题切换；
- 内容宽度切换；
- 本地演示通知；
- skip link 和唯一主内容区。

它没有提供：

- 服务端全文搜索；
- 搜索索引、拼音、模糊排名或权限过滤；
- 真实通知订阅、未读数同步和后台确认；
- 主题持久化或跨设备同步；
- 路由历史滚动恢复；
- 生产级身份切换。

### 2.4 Mock 数据页面的事实

用户首页、积分、优惠券和个人中心复用统一 `growthOsMockData.ts`，并显示 snapshot 标签。这样做的事实边界是：

- 它们是确定性演示数据，不是实时账务；
- 页面可从同一列表派生可用数量、收入、支出、余额等摘要；
- 优惠券“可用/已使用”筛选是浏览器内的真实 UI 行为；
- clipboard 调用是真实浏览器能力，并有成功/失败反馈；
- 通知“已读”只改变浏览器内 state，刷新恢复；
- 个人资料只展示现有字段，不虚构双因素认证、登录记录或安全设备。

这里的“真实”应拆开说：交互可以是真实的，数据来源仍然是 Mock；clipboard 可以真正执行，优惠券本身仍然没有后端兑换能力。

### 2.5 代码落点

- [httpClient.ts](../../../web/src/api/httpClient.ts)：同源 transport、empty-body POST、取消、超时、错误分类。
- [lotteryApi.ts](../../../web/src/api/lotteryApi.ts)：endpoint、header、`uint64` 字符串与运行时 decoder。
- [useEphemeralLotterySelection.ts](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts)：请求所有权、判别联合、取消与 generation。
- [LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx)：表单、状态、结果、边界和响应式披露。
- [WorkspaceShell.tsx](../../../web/src/components/layout/WorkspaceShell.tsx)：导航、搜索、抽屉、通知、主题、滚动和焦点。
- [UserLayout.tsx](../../../web/src/layouts/UserLayout.tsx)：用户信息架构、alias 和工作空间入口。
- [ProductPage.tsx](../../../web/src/components/common/ProductPage.tsx)：页面标题、Section、Surface 和演示标记。
- [growthOsMockData.ts](../../../web/src/mocks/growthOsMockData.ts)：统一演示 snapshot。

---

## 3. Constraint：哪些边界不能绕过？

### 3.1 业务约束：选择不是交易

服务端只做“根据当前 Strategy 快照选一个 Award”。它没有给前端以下证明：

- 用户是否有资格；
- 当前用户还有几次机会；
- 是否应扣积分；
- 奖励候选是否还有库存；
- 结果是否已经持久化；
- 奖励是否已经发放；
- 相同业务请求是否能得到相同结果。

因此 UI 不能补完这些空白。尤其不能因为 outcome 叫 `reward` 就把它翻译成“恭喜中奖”。`reward` 是候选类型，不是交易状态。

### 3.2 HTTP 约束：未知结果不是失败结果

对一个非幂等、无持久结果的 POST：

- timeout 可能发生在请求到达服务端之前；
- 也可能发生在服务端已完成选择、响应尚未到达浏览器之后；
- 502/504 或连接中断同样不能证明服务端没有执行；
- AbortController 只停止客户端继续等待，不能回滚已经发生的服务端工作。

所以页面必须说“无法确认这次请求的结果”，而不是“本次未中奖”或“选择失败”；transport 不能自动再发一次；用户再次点击必须被描述为新操作。

### 3.3 数据约束：`uint64` 超出 JavaScript 安全整数

JavaScript `Number` 只能精确表示到 `2^53 - 1`。后端 `uint64` 最大值远大于此范围。如果把 `18446744073709551615` 转成 Number：

- 可能产生舍入；
- 路径参数可能被改变；
- 响应 ID 与请求 ID 比较会失真；
- 日志和故障关联可能指向错误实体。

因此 ID 的领域类型是“规范十进制字符串”，而不是“看起来像数字的 number”。BigInt 也没有必要进入 transport：它不能直接 JSON 序列化，而且 ID 不参与算术。

### 3.4 接口约束：TypeScript 类型不验证网络数据

TypeScript 在编译时消失。服务端、代理、旧版本、HTML 错误页或错误的测试替身都可能返回不符合接口声明的数据。UI 如果直接 `as EphemeralSelectionResponse`，就会把未知数据当事实。

运行时 decoder 必须至少验证 UI 会据此作出业务断言的字段：

- wrapper 是否存在；
- durability 是否仍是 ephemeral；
- response Strategy ID 是否与 request 一致；
- Award ID/name/outcome 是否在合同内。

未知字段可以容忍，未知 outcome 不能静默当成 `no_reward`。

### 3.5 信息架构约束：功能会继续增长

当前已有用户、运营、MCP、Agent 和系统状态等多个工作空间。顶部横向导航在条目增加时会出现：

- 文案缩短到不可理解；
- 低频入口被塞进“更多”；
- 当前工作区与跨工作区入口混在一起；
- 移动端完全换一套结构。

因此导航骨架必须允许分组、折叠和跨工作区切换；路由 active 不能用字符串包含判断，否则 `/campaigns-old` 也可能误亮。

### 3.6 响应式约束：不是把桌面缩小

桌面 lottery 页面可以并排放置主工作台与合同 rail；390px 手机不能保留同样列宽。移动端需要：

- 单列主流程；
- 主要按钮可在窄屏完整操作；
- 合同与缺失能力保留，但使用原生 `<details>` 渐进披露；
- 不依赖 hover；
- 文本和 ID 可换行；
- 抽屉打开时背景不滚动，焦点不逃出模态层。

响应式的原则是“任务与业务信息等价，布局密度不同”，而不是移动端删掉边界说明。

### 3.7 无障碍约束：视觉状态必须有程序化语义

至少要满足：

- 输入有可见 `<label>`；
- 错误通过 `aria-invalid` 与 `aria-errormessage` 关联；
- pending/success 用礼貌的 `role=status`，不会抢断读屏；
- error 用 `role=alert`；
- `no_reward` 不误用 alert；
- 活动导航使用 `aria-current=page`；
- 展开控件有 `aria-expanded` 与 `aria-controls`；
- 模态搜索和移动抽屉有可访问名称、焦点进入、Tab 循环、Escape 关闭和焦点恢复；
- 装饰图标与重复图表不进入无障碍树；
- reduced-motion 偏好关闭非必要动画；
- 颜色不是唯一的信息载体。

### 3.8 视觉约束：高密度不等于拥挤

高密度工作台仍需保留：

- 明确的标题层级；
- 可扫描的数字与 tabular numerals；
- 足够的可点击面积和焦点环；
- 1px border 作为结构，而不是所有内容都挤在无边界白底；
- `radius <= 12px` 的一致圆角语汇；
- overlay 才使用更强阴影；
- 暗色主题中仍有边界和对比度。

如果为了“flat”把所有边界删掉，会损害结构；如果为了“艺术感”加入大面积渐变、玻璃拟态和不断运动的图形，会损害可读性、性能和业务可信度。

### 3.9 证据约束：单测不能代替所有验证

测试环境可以证明：

- DOM 语义与文案；
- route active；
- 事件后状态变化；
- transport options；
- deferred Promise 的竞态结果。

它不能单独证明：

- 浏览器实际布局没有溢出；
- 自定义 focus trap 在真实浏览器的可见元素过滤下正确循环；
- 代理确实把 `/api` 转到正确后端；
- Network 中没有意外 body/header；
- 真实数据库 Strategy 可返回结果；
- 暗色主题和系统 reduced-motion 的视觉质量。

因此验证必须分层，不能用“测试全绿”概括所有证据。

---

## 4. Options：有哪些可选方案？

### 4.1 抽奖交互的候选方案

#### 方案 A：前端 `Math.random()` + 转盘动画

优点：

- 最快得到“像抽奖”的视觉；
- 不依赖后端；
- 演示时容易控制结果。

缺点：

- 结果不来自服务端策略；
- 浏览器与数据库配置成为两个事实源；
- 很容易把动画停点误当选择算法；
- 无法证明 Go/MySQL 链路；
- 对求职项目而言，经不起“结果究竟在哪里决定”的追问。

结论：不采用。可以作为完全隔离、明确标为视觉原型的 Story，但不能接到真实选择按钮上。

#### 方案 B：前端写死候选、概率和库存，再调用后端结果

优点：

- 界面更丰富；
- 用户在点击前能看到奖品。

缺点：

- 当前没有 Strategy 查询接口和版本号；
- 候选与服务端快照可能分叉；
- 概率展示可能涉及合规、精度与动态配置；
- “库存剩余”会成为未经证明的实时断言。

结论：不采用。等服务端提供只读展示模型、版本和权限边界后重评。

#### 方案 C：只输入 Strategy ID，展示服务端临时结果

优点：

- 只依赖已经存在的合同；
- 输入、请求和结果都可精确验证；
- 可以诚实讲清楚 ephemeral 边界；
- 给后续正式 Draw 留出协议空间。

缺点：

- 面向普通消费者不够友好；
- 用户必须知道 Strategy ID；
- 缺少候选预览和历史记录。

结论：本节采用。当前页面本质上是 development/test selection workbench，不是假装完成的消费者抽奖产品。

### 4.2 请求状态的候选方案

#### 多布尔值

`isLoading`、`isSuccess`、`hasError` 看似简单，但有八种组合；其中多种没有业务意义，还容易遗留旧 response/error。

#### 判别联合 + `useState`

只允许 idle/selecting/success/error；每个阶段携带自己的数据。当前转换不多，代码短，TypeScript 可穷举。

#### reducer 或 XState

更适合确认、冻结、抽取、发放、补偿、恢复等复杂状态图；当前使用会引入比业务本身更多的仪式和依赖。

结论：采用判别联合 + 本地 Hook。状态增长到需要事件日志、并行 actor 或跨页面恢复时再升级。

### 4.3 请求竞态的候选方案

#### 只禁用按钮

可提供视觉反馈，但 React state 更新前同一事件批次、程序调用或多个入口仍可能穿透；也无法处理旧 Promise 晚到。

#### 只用 AbortController

能停止支持 signal 的 fetch 和释放资源，但不能回滚服务端，也不能保证所有异步替身即时拒绝。

#### 只用 generation token

能拒绝陈旧回调写状态，但旧网络仍继续占资源。

#### AbortController + generation + in-flight ref

- controller：资源取消；
- generation：最终状态正确性；
- ref guard：同步防重复；
- disabled：用户可见反馈。

结论：采用组合方案。它仍然只是浏览器内的重复提交保护，不声称服务端幂等。

### 4.4 HTTP helper 的候选方案

#### 复用通用 `postJSON(path, {})`

会发送 `{}` 或推导 `Content-Type: application/json`，改变“无 body”的合同。

#### 每个调用点手写 fetch

短期灵活，但同源检查、超时、错误 envelope、Request ID 和无重试规则会分散。

#### 专用 `postJSONWithoutBody`

把“empty-body POST”提升为 transport 能力，调用者无法意外塞 body；API adapter 只补领域 header 和 decoder。

结论：采用专用 helper。名称本身成为合同提示。

### 4.5 页面外壳的候选方案

#### 顶部导航 + hero/card landing

适合少量入口和营销介绍，能强化品牌情绪；但功能扩张后导航拥挤，超大卡片降低数据密度，活动位置不稳定。

#### 每个模块各自做 layout

局部自由度高，但搜索、主题、抽屉、焦点、active 规则和响应式会重复，行为容易漂移。

#### 共享侧栏 + 顶部工具栏 + flat content surface

稳定工作区层级，能分组多个模块；共享全局交互；内容页只负责自身信息结构。

结论：采用共享 WorkspaceShell。营销落地页仍可独立存在，但不应主导已登录工作台。

### 4.6 搜索的候选方案

#### 真实后端搜索

需要索引、权限、排序、debounce、取消和空结果合同，本节没有相应 API。

#### 静态页面命令面板

从当前 navigation 和 workspace switches 构造唯一页面列表，按 label/path 在本地筛选。它解决“快速去某页”，不声称搜索业务数据。

结论：采用静态页面搜索，并在面试中准确称为 page palette，而不是全站搜索。

### 4.7 Modal 实现的候选方案

#### 原生 `<dialog>`

浏览器可以帮助管理焦点、inert 背景和关闭行为，未来值得评估；但现有项目已使用条件渲染和自定义 overlay，迁移需要统一样式、兼容性与测试策略。

#### `role="dialog"` + 自定义焦点管理

能与当前 React 结构直接整合，但必须自己实现 focus trap、初始焦点、Escape、背景点击、body scroll lock 和焦点恢复。

结论：本节采用自定义 dialog，并把真实浏览器键盘验收列为必需证据和未来可能重构点。

### 4.8 Mock 数据的候选方案

#### 每个页面各写一套常量

实现快，但积分余额、账单、优惠券数量和个人资料很快互相矛盾。

#### 统一 Mock snapshot + 页面派生

所有页面引用同一快照；摘要由交易/优惠券数组派生；UI 明确显示 snapshot 时间。

#### 现在就接完整真实 API

理想，但超出本节已有后端合同，也会迫使前端虚构分页、错误和权限行为。

结论：除 lottery 真实接口外，其余未接入模块继续使用统一、明确标记的 Mock snapshot。等对应合同存在后逐页替换。

### 4.9 图表的候选方案

#### 动画图表作为主叙事

视觉吸引力强，但重复动画会分散注意、增加运动不适，并可能把装饰图形误当实时计算。

#### 静态图表 + 文本摘要/数据表

图形用于快速看趋势，文字和表格承担可访问的事实表达；关闭 Recharts animation，装饰性重复图隐藏。

结论：采用静态图表，不依赖 hover 才能获得唯一信息；需要完整数据时提供账单/阶段统计。

---

## 5. Trade-off：为什么选择当前实现？

### 5.1 总决策：最大化“可解释的真实性”，不是最大化功能数量

本节用一个简单优先级评估方案：

```text
业务语义准确
  > 合同可验证
  > 错误可恢复/可解释
  > 无障碍与响应式
  > 信息密度
  > 视觉情绪
  > 技术栈数量
```

这不意味着视觉不重要，而是视觉不能覆盖事实。一次 ephemeral selection 即使被包装成华丽转盘，也不会因此获得 Draw、库存或发奖能力。

### 5.2 为什么 Lottery 是一个 workbench，而不是消费者转盘？

当前用户需要提供技术 ID，结果需要展示 durability、Request ID、Award ID、调用链和缺失能力。这些信息天然属于开发/测试工作台。

workbench 的结构是：

1. PageHeader 定义页面身份与边界；
2. 主区接受 Strategy ID；
3. 同一 Surface 内呈现 idle/selecting/success/error；
4. 桌面右 rail 解释调用链、重试策略和缺失能力；
5. 移动端用 `<details>` 保留相同信息。

如果套用消费者转盘，技术 ID、合同和错误就会被迫藏进小字；用户也容易把动画停点当成浏览器随机结果。workbench 让“谁做了选择”一眼可见。

### 5.3 为什么不自动填默认 Strategy ID？

默认值会把环境中的某个实体变成隐式依赖，并可能让 Enter 或未来 Effect 触发非预期选择。当前后端又没有 list endpoint，前端无法证明哪个 Strategy 对当前用户合理。

让初始输入为空牺牲了一次点击便利，但换来：

- 用户意图明确；
- 不把开发数据库 ID 写进产品事实；
- 不在 mount/Strict Mode 中触发 POST；
- 测试可以精确证明零自动请求。

### 5.4 为什么 `no_reward` 必须保留在 success 分支？

技术失败表示没有得到可信合同结果；`no_reward` 表示服务端成功完成选择并选中了未中奖候选。如果合并：

- 错误率会被业务结果污染；
- UI 会错误建议“重试”；
- 用户可能误以为系统故障；
- 监控无法区分策略分布和可用性问题。

因此 success 内部再按 outcome 渲染不同措辞；只有 transport/HTTP/contract 失败进入 error。

### 5.5 为什么错误文案不直接展示原始 message？

原始错误可能包含：

- 网关内部信息；
- 上游 HTML；
- 数据库或路径细节；
- 对用户没有行动价值的技术文本。

页面按安全分类映射“发生了什么、能否确认结果、下一步是什么”，只显示用于关联的 Request ID。原始细节应进入受控日志，而不是直接信任并渲染。

### 5.6 为什么 additive response fields 被忽略？

前端只投影当前合同字段，让服务端可以添加非破坏性元数据；同时阻止 UI 偶然依赖尚未稳定的字段。closed union 仍严格：一个新 outcome 会改变业务分支，不能被当作 additive field 忽略。

这个策略的权衡是：服务端新增有价值字段时，前端不会自动显示，需要显式升级 adapter 与测试。这个成本换来更清晰的契约演进。

### 5.7 为什么共享 Shell 采用“路由元数据驱动”，而不是页面猜状态？

`UserLayout` 把 label、path、icon、alias 放在 navigation 配置中，Shell 统一生成：

- 桌面导航；
- 移动抽屉导航；
- page palette 搜索项；
- active 状态。

这样同一页面只登记一次，减少三套入口不一致。`matchPaths` 只为明确 alias 使用；nested route 通过 segment boundary 匹配，不用 `includes`。

代价是当前 metadata 还没有完全类型化到 route manifest，也没有基于权限过滤。路由规模继续增长时需要重评单一 manifest。

### 5.8 为什么路由切换后滚到顶部，而不是保留位置？

当前工作台的主要导航语义是“进入新的任务页”。如果从很长的活动列表跳到详情或积分页仍停在相同 scrollTop，用户可能看不到新页面标题。

所以 Shell 在 pathname 变化时：

- 关闭 drawer/search/notification；
- 把 documentElement 和 body 的 scrollTop 设为零。

这是一条简单、一致的规则，但不等于完整的浏览器历史滚动恢复。返回长列表时用户也会回到顶部；当列表浏览和 back-navigation 成为主要场景时，应使用 React Router 的滚动恢复或按 location key 保存位置。

### 5.9 为什么通知不是 modal？

通知面板是附着在铃铛按钮上的非模态 region：用户可以继续与页面其他区域交互，也不需要强制完成一个决定。因此它：

- 不设置 `aria-modal`；
- 不锁背景滚动；
- 不做 focus trap；
- 支持 Escape 和外部点击关闭；
- 用 `aria-expanded`/`aria-controls` 连接触发器与 panel。

更重要的是，它明确写出“仅为本地界面样例，不读取服务端”，未读状态只存于当前 React state。视觉上有红点，不代表已经拥有真实通知系统。

### 5.10 为什么搜索与抽屉必须恢复焦点？

模态层关闭后，如果焦点落回 body 或一个已经卸载的元素，键盘用户必须从页面开头重新寻找位置。Shell 记录打开搜索的触发器；移动抽屉记录 menu 按钮；关闭后把焦点还给调用者。

这个细节把“能关掉”升级为“可以连续操作”。代价是必须正确处理：

- 路由跳转导致触发器是否仍存在；
- `requestAnimationFrame` 后再 focus；
- Escape 同时存在多个 overlay 时的优先级；
- jsdom 不具备真实布局，Tab trap 需要浏览器验证。

### 5.11 为什么页面组件复用 `ProductPage`，但不做万能卡片系统？

共享层只承载稳定语义：

- PageHeader；
- SectionHeader；
- DemoBadge；
- Surface；
- CompactMetric；
- ProgressBar。

它没有把每一种表格、筛选、图表和结果区都抽成带几十个 props 的“万能 DashboardCard”。抽象的门槛是：至少多个页面有相同职责和状态，而不只是外观相似。

### 5.12 为什么统一 Mock snapshot，还要保留页面派生？

统一 snapshot 解决来源重复；派生数据解决摘要矛盾。例如：

- 优惠券可用数量从 `mockCoupons.filter` 得到；
- 积分收入/支出从 transaction type 与 amount 得到；
- 首页活动摘要来自同一 `mockCampaigns`；
- profile 的积分来自同一 mock user。

不应该把“3 张可用优惠券”“近 7 日收入 610”再写成另一份常量。React 官方状态原则同样适用于 Mock：能从现有数据计算的值不要另存一份会漂移的 state。

### 5.13 为什么静态图表关闭动画？

这些图表描述的是固定演示 snapshot，不是流式实时更新。入场动画不会增加信息，反而可能：

- 让静态数据看起来在实时计算；
- 给 reduced-motion 用户造成不适；
- 让截图和测试不稳定；
- 消耗低端设备资源。

因此 Recharts 使用 `isAnimationActive={false}`；全局 CSS 也响应 `prefers-reduced-motion`；装饰性漏斗在已有文本阶段统计后用 `aria-hidden` 隐藏，避免读屏重复或读出无意义 SVG 节点。

### 5.14 设计 token 怎样从参考中落地？

本节没有声称实现了 DTCG JSON token pipeline，而是先建立轻量、可复用的代码语汇：

- color：zinc neutral + violet primary + semantic green/amber/rose；
- surface：white/zinc-950 基底，zinc 1px border；
- radius：全局约 `0.625rem`，Surface `rounded-xl`，局部控件 `rounded-md/lg`；
- width：sidebar 231px / collapsed 64px，content max 1320px；
- spacing：4/6/8 等少量间距档；
- typography：small labels、tabular numeric、mono IDs；
- elevation：内容面尽量 flat，overlay 才获得明显 shadow；
- motion：短过渡，reduced-motion 关闭非必要效果。

正式设计系统增长到多端、多主题、多团队时，再把这些决策迁移到命名 token、语义 alias 和自动生成流程。

---

## 6. Failure：失败、竞态和误用会怎样发生？

### 6.1 故障模型总览

```text
用户意图错误
  ├─ 非法 Strategy ID
  └─ 把新选择理解成恢复旧结果

客户端生命周期错误
  ├─ 双击/重复调用
  ├─ 输入变化后旧响应覆盖
  ├─ 组件卸载后写状态
  └─ Strict Mode 中 Effect 自动 POST

传输与合同错误
  ├─ 意外 body / query / Content-Type / Idempotency-Key
  ├─ timeout / network / gateway 未知结果
  ├─ 非 JSON / 错误 envelope
  └─ 合同字段错误或未知 outcome

产品语义错误
  ├─ reward 写成中奖/已发奖
  ├─ no_reward 写成系统错误
  ├─ Mock 写成实时账务
  └─ 本地通知写成服务端通知

交互与无障碍错误
  ├─ active route 误亮
  ├─ drawer/search 焦点逃逸
  ├─ 关闭后焦点丢失
  ├─ 移动端删掉关键边界
  └─ 图表只靠颜色/动画/hover
```

### 6.2 非法 Strategy ID

失败路径：用户输入 `0`、`01`、`+1`、空白、小数、科学计数法或超上限数字。

保护：

- UI 即时校验并关联错误文本；
- submit button 禁用；
- API adapter 再做一次本地合同校验；
- invalid input 在 transport 前失败。

为什么双层：页面校验是体验，adapter 校验是可复用边界。未来其他调用者不能绕过规则。

### 6.3 快速重复提交

失败路径：双击、键盘连续 Enter、同事件批次内两次调用、测试或未来组件直接调用 Hook。

保护：

- UI 在 selecting 时 disabled；
- Hook 在同步创建请求前检查 `activeController.current`；
- 只允许一个 in-flight request。

限制：刷新、多个标签页、代理重放和多个客户端不受保护，因此绝不能称为幂等。

### 6.4 清空/改 ID 后旧响应晚到

失败路径：Strategy A 请求未完成，用户改成 B；A 的 Promise 随后 resolve/reject，覆盖当前 UI。

保护顺序：

1. `clear()` 先递增 generation；
2. abort active controller；
3. 清空 ref；
4. state 回 idle；
5. A callback 检查 generation 或 aborted 后退出。

先递增 generation 很关键：即使测试替身忽略 abort，陈旧回调仍失去写权限。

### 6.5 卸载后的状态更新

失败路径：用户在 selecting 时跳转，Promise 完成后尝试写已卸载组件。

保护：effect cleanup 递增 generation、abort、清 ref；effect 本身不发请求，也不在 cleanup 中 setState。

### 6.6 Strict Mode 双执行

失败路径：把 POST 放入 `useEffect`，开发环境 setup-cleanup-setup 产生额外选择。

保护：选择只由 form submit 事件触发；effect 只负责清理。测试在 StrictMode 下断言挂载不请求。

### 6.7 timeout 后自动重试

失败路径：transport 把所有请求统一做指数退避；第一次其实已完成，第二次重新随机选择，UI 只展示第二个结果。

保护：

- HTTP client 没有 retry loop；
- adapter 不设置幂等键；
- Hook 不做 retry；
- 页面明确未知结果和“再次操作是新选择”。

未来正式 Draw 若支持重放，必须由服务端持久化 idempotency key、调用主体、request fingerprint 和结果，不能只在浏览器加 header。

### 6.8 错把网关错误降级成 `no_reward`

失败路径：为了“总有结果”，catch 后生成本地未中奖。

后果：

- 系统故障被掩盖；
- 策略分布数据被污染；
- 用户被错误告知业务结果；
- 运维无法根据 Request ID 排查。

保护：只有 decoder 验证过的 `outcome: no_reward` 才能进入正常成功。任何 transport/contract 错误走 error presentation。

### 6.9 错把 `reward` 当成发奖

失败路径：按钮成功后显示“恭喜获得奖品”，甚至在 mock 账户中增加积分或优惠券。

后果：建立不存在的交易承诺；刷新后结果消失；库存或履约不一致。

保护：

- 固定使用“奖励候选”；
- 展示 durability；
- 不修改全局用户积分或 coupon snapshot；
- 测试禁止虚假中奖文案。

### 6.10 奖品名称导致展示或语义问题

失败路径：服务端返回空名、首尾空白、控制字符、超长 Unicode 或孤立 surrogate。

保护：decoder 按 Unicode code point 计数、拒绝控制字符和边缘空白；UI 不使用 `dangerouslySetInnerHTML`，长 ID 允许 break。

限制：这不是内容审核；它只保证基本合同与显示安全。

### 6.11 active route 误匹配

失败路径：使用 `pathname.startsWith('/campaign')`，导致 `/campaign-archive` 或相似路径误亮；alias 页面没有 active 项。

保护：

- exact path 或 `path + '/'` segment boundary；
- alias 必须在 `matchPaths` 显式登记；
- 每个 link 只有 active 时设置 `aria-current=page`；
- route table tests 覆盖根路径、nested path 和 `/rewards`。

### 6.12 Search 与 drawer 焦点错误

失败路径：

- 打开后焦点仍在背景；
- Tab 移出 modal；
- Escape 关闭但焦点掉到 body；
- drawer 打开时背景仍滚动；
- route change 后 overlay 留在新页面。

保护：

- 条件渲染 dialog；
- 入口时聚焦 input/close；
- 自定义 Tab/Shift-Tab 循环；
- body scroll lock；
- Escape/backdrop/result/nav link 关闭；
- 关闭恢复 trigger；
- pathname effect 清理 overlay。

剩余风险：`getClientRects()` 的可见元素判断在 jsdom 中不等于真实浏览器，必须进行实际键盘验收。

### 6.13 多个 overlay 的 Escape 优先级

当前全局 Escape handler 会关闭 search、drawer 和 notification。正常 UI 不应同时打开多个模态层，但未来功能增长时可能出现嵌套。

重评方向：

- 引入统一 overlay stack；
- 只让最顶层处理 Escape；
- 使用 portal 或原生 dialog；
- 集中管理 scroll lock 与 focus restoration。

### 6.14 Mock snapshot 自相矛盾

失败路径：首页写余额 12,450，积分页写 10,000，profile 又写另一等级；优惠券 tab 的数量与列表不一致。

保护：统一 mock source，摘要从列表派生，页面显式显示 snapshot。测试应同时断言标签与派生结果，而不仅是单个文案。

### 6.15 clipboard 反馈误报成功

失败路径：`navigator.clipboard.writeText` reject，但 UI 立即显示“已复制”；组件卸载后 timer setState。

保护：await Promise 后再设置 success；catch 显示失败；使用 ref 保存 timer；重复复制前 clear；卸载 cleanup 清 timer。

### 6.16 图表无障碍失败

失败路径：

- 用户只能通过三种线条颜色区分余额/收入/支出；
- Tooltip 是唯一数值来源，键盘/触屏无法获得；
- 装饰 funnel 被读屏逐节点朗读；
- 入场动画违背 reduced-motion。

保护：

- 图例同时有文字；
- 页面另有摘要和账单/阶段统计；
- 重复装饰 funnel `aria-hidden`；
- Recharts 关闭 animation；
- 全局 CSS 尊重 prefers-reduced-motion。

剩余限制：趋势图目前没有完整逐日可访问数据表；如果图表成为关键决策工具，需要补表格或详细文本替代。

---

## 7. Evidence：用什么证据确认没有偏离？

### 7.1 证据分层，而不是一个“完成”标签

本节采用以下证据金字塔：

```text
真实浏览器 + 同源后端 + 数据库环境
            ↑
        构建与路由装配
            ↑
     页面/布局 RTL 行为测试
            ↑
 Hook 竞态测试 + API/transport 精确测试
            ↑
    类型检查、格式和文档检查
```

上层不是替代下层：浏览器截图不能证明无自动重试；单元测试也不能证明 390px 没有横向溢出。

### 7.2 Transport 证据

[httpClient.test.ts](../../../web/src/api/httpClient.test.ts) 应证明：

- empty-body POST 没有 body；
- 没有因 JSON helper 自动加 Content-Type；
- method、mode、credentials、cache、redirect 符合预期；
- unsafe/foreign path 在 fetch 前失败；
- timeout 与 caller cancel 分类不同；
- gateway/network/contract envelope 被分类；
- 一次失败只调用一次 fetch。

[lotteryApi.test.ts](../../../web/src/api/lotteryApi.test.ts) 应证明：

- 路径没有 query 或 fragment；
- header 只有合同要求项；
- 没有 Idempotency-Key；
- Strategy/Award ID 的最大 `uint64` 仍是原字符串；
- reward/no_reward 都可解码；
- invalid response 被拒绝；
- additive field 不破坏投影；
- invalid local input 不触发 fetch；
- network failure 不自动重试。

### 7.3 Hook 生命周期证据

[useEphemeralLotterySelection.test.tsx](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.test.tsx) 应用 deferred Promise 构造真实竞态，而不是只测立即 resolved 的 happy path：

- mount 不请求；
- StrictMode mount 仍不请求；
- reward 和 no_reward 都进入 success；
- classified failure 进入 error；
- error 后不会自动产生第二次请求；
- 只有用户显式 select 才会有第二次选择；
- pending 期间重复调用仍只发一次；
- unmount abort；
- clear 后旧 Promise 晚到不能覆盖 idle；
- 新成功后旧失败晚到不能覆盖新状态。

### 7.4 页面语义证据

[LotteryPage.test.tsx](../../../web/src/pages/user/lottery/LotteryPage.test.tsx) 不只测“有按钮”，还应测：

- 页面边界与未实现能力可见；
- 不出现“恭喜中奖”“奖励已到账”等虚假承诺；
- label、help、error 关联；
- canonical ID 才能提交；
- pending 是 polite status；
- reward 只写候选；
- no_reward 是正常 status；
- raw error 不直接泄露；
- correlation ID 在合适时可见。

### 7.5 共享 Layout 证据

[UserLayout.test.tsx](../../../web/src/layouts/UserLayout.test.tsx) 当前覆盖：

- 唯一 main、skip link 和命名导航；
- `/`、`/home`、nested campaign、`/rewards`、`/profile` 的 active 状态；
- collapsed 后仍保留可访问链接名；
- mobile drawer 打开/关闭/Escape；
- body scroll lock；
- 打开时聚焦 close，关闭后回 menu trigger；
- `Cmd/Ctrl + K` 打开 search、过滤并导航；
- Escape 后焦点回 search trigger；
- 通知明确是本地演示，标记已读只改变样例；
- 主题双向切换；
- 设置与主要动作链接目标。

但这些测试不能替代真实浏览器的 Tab 循环、focus-visible、背景不可交互和 viewport 验证。

### 7.6 Mock 页面证据

页面测试应同时验证“功能”和“诚实边界”：

- Points：snapshot 标签、语义摘要、账单表格、收入/支出派生；
- Coupons：available/used 真实筛选、空态、clipboard success/failure、timer cleanup；
- Profile：`dl/dt/dd` 语义和已有字段，不出现未实现安全功能；
- Home：演示标签、摘要与图表辅助文本；
- Campaign/Feed：演示指标与待接入动作明确标注。

### 7.7 视觉证据

视觉核查至少包括：

- authenticated reference 与 GrowthOS final 的结构对照；
- desktop lottery：侧栏、顶栏、主 workbench、右 rail、折叠线位置；
- mobile lottery 390px：单列、按钮、ID 换行、details、无横向滚动；
- light/dark；
- sidebar expanded/collapsed；
- search、drawer、notification overlay；
- success reward/no_reward/error/pending 各状态；
- prefers-reduced-motion。

如果仓库保留 `output/design-qa`，它只能作为当次视觉快照。代码变更后必须重新拍摄；旧图不能证明当前 HEAD。

### 7.8 真后端证据

要证明“浏览器结果不再由 Mock 决定”，至少要在可用环境完成：

1. 启动前端、Go API、MySQL 与所需依赖；
2. 确认 development/test feature gate；
3. 使用数据库中已配置 Strategy ID；
4. 在浏览器 Network 中检查 method、path、headers、无 body、无 query；
5. 检查响应 `selection.durability`、Strategy/Award IDs 和 outcome；
6. 检查页面只展示解码后的结果；
7. 检查 Request ID 能与服务端日志关联；
8. 人工制造无效 ID、not found、timeout/网关错误，确认不会变成 no_reward；
9. 刷新后确认结果不会恢复。

如果本轮只完成单测和截图，就只能说“代码和模拟 transport 证明了合同”，不能说“Docker 全链路已验收”。

### 7.9 文档一致性证据

文档检查不只是链接存在，还要核对术语：

- 课程章节说 ephemeral，面试 QA 也必须说 ephemeral；
- 代码叫 `reward`，文档不能翻译成“已发奖”；
- Request ID 不能被叫 Draw ID；
- 用户再次点击不能叫幂等重试；
- Mock 页面不能写实时账务；
- 未来 Redis/库存/幂等能力只能出现在 Limit/Trigger，不得放进“本节完成”。

### 7.10 可复用的核查矩阵

| 主张 | 最低证据 | 还需要什么才可升级主张 |
| --- | --- | --- |
| 请求无 body | transport/API 单测检查 RequestInit | 浏览器 Network 抽查 |
| 没有自动重试 | fetch 调用次数单测 | 故障注入观察浏览器 Network |
| 避免陈旧响应 | deferred Hook 测试 | 真实慢网手测 |
| reward 只是候选 | 页面文案与禁止词测试 | 产品/API 合同评审 |
| no_reward 正常 | decoder + 页面 status 测试 | 真实 Strategy 返回样例 |
| 移动端可用 | DOM/响应式结构测试 | 390px 浏览器与键盘/触摸核查 |
| 搜索可访问 | RTL 焦点/关闭测试 | 真实 Tab/Shift-Tab/读屏 |
| 图表可理解 | 文本摘要、图例、无动画 | 关键图表补完整数据表 |
| Mock 边界诚实 | snapshot 标签测试 | 对应真实 API 接入后替换 |

---

## 8. Limit：当前还不能宣称什么？

### 8.1 Lottery 业务闭环仍未实现

明确未实现：

- 用户/租户身份；
- 活动资格；
- 抽奖次数；
- 积分冻结与扣减；
- Strategy 列表和展示配置；
- 候选概率披露；
- Draw 聚合与 Draw ID；
- 结果持久化和查询；
- 库存预占/确认/释放；
- 奖励发放；
- 幂等重放；
- 限流、风控、反作弊；
- Redis 业务状态；
- 消息队列履约与补偿；
- 审计账本和运营对账。

因此简历不能写“实现完整高并发抽奖系统”“通过 Redis 防超卖”“保证 exactly-once 发奖”，除非后续章节真的交付并验证。

### 8.2 Ephemeral endpoint 的可观测性有限

Request ID 有助于关联请求，但不是业务结果 ID；elapsedMs 是浏览器观测值，包含网络和前端开销。当前没有：

- 可查询的 selection record；
- 结果审计轨迹；
- 相同业务操作的稳定键；
- 用户可恢复页面。

### 8.3 搜索只是页面导航

它没有：

- 搜索活动、用户、交易或日志；
- 服务端结果；
- 权限过滤；
- fuzzy ranking；
- 最近搜索；
- 搜索分析。

因此 UI 和简历都应叫“工作台页面搜索/命令面板”，不能称“全局业务搜索引擎”。

### 8.4 通知只是本地演示

它没有：

- 服务端消息源；
- WebSocket/SSE/polling；
- 持久未读状态；
- 跨设备同步；
- 通知详情和批量操作。

“标记样例已读”只改变内存 state，刷新恢复。

### 8.5 主题和布局偏好不持久

主题由 Zustand state 与 html class 控制；侧栏折叠、全宽等同样是当前会话体验。没有 localStorage、用户设置 API、系统偏好初始化或 SSR hydration 处理。

### 8.6 Scroll reset 不是 scroll restoration

pathname 变化归零解决了新页面从中间开始的问题，但返回上一页也不会恢复旧位置；锚点、嵌套滚动容器和 history key 也未统一处理。

### 8.7 自定义 focus trap 仍需更强验证

当前实现覆盖基本首尾循环，但尚未证明：

- 元素动态 disabled/hidden 时列表总正确；
- shadow DOM/iframe 场景；
- 多 overlay stack；
- VoiceOver/NVDA 完整体验；
- 背景对所有辅助技术真正 inert。

原生 `<dialog>` 或成熟的无障碍 primitive 可能在规模增长后更稳健。

### 8.8 图表不是完整可访问数据产品

用户首页趋势图有文字图例、摘要和账单，但没有逐日表格一一对应全部点；Tooltip 仍偏鼠标交互。漏斗图被当成有文本统计的装饰性重复表达，而不是独立可交互图表。

### 8.9 Mock snapshot 不是后端 schema 保证

TypeScript mock types 与未来 API DTO 不一定相同；接入真实接口时仍需：

- transport/error contract；
- runtime decoder；
- loading/empty/error/stale 状态；
- pagination/filter semantics；
- 权限与隐私；
- 时区与金额/积分精度。

### 8.10 设计 token 仍是轻量实现

当前用 CSS variables、Tailwind utilities 与共享组件实现一致性，但没有：

- 独立 token package；
- DTCG JSON 源；
- design-to-code 自动生成；
- 多品牌主题；
- visual regression baseline；
- token deprecation policy。

不能把“用了相近颜色和圆角”包装成成熟设计系统。

### 8.11 Reference comparison 不等于品牌授权或用户研究

参考站能说明一种成熟信息布局，但不能代替 GrowthOS 自己的：

- 用户任务研究；
- 内容可理解性测试；
- 数据密度偏好；
- 无障碍测试；
- 品牌规范和法律审查。

本节的选择仍是假设驱动，需要后续证据验证。

---

## 9. Trigger：出现什么信号时必须重评？

### 9.1 从 ephemeral selection 升级为正式 Draw

触发条件：产品要求用户可查历史、刷新恢复、扣积分、锁库存或发奖。

需要重评：

- 新建 Draw command 与 query API；
- 服务端生成 Draw ID；
- idempotency key 与调用主体、请求摘要绑定；
- 状态机，例如 `pending → selected → reserved → delivered/failed/compensating`；
- 积分账本与库存预占的事务边界；
- outbox/event 与异步履约；
- 查询和恢复 UX；
- 重放与重复消息处理；
- 审计、限流和风控。

此时前端状态也应从四阶段本地 Hook 升级为围绕 Draw resource 的可恢复状态。

### 9.2 后端提供 Strategy 展示接口

触发条件：出现带权限、版本、候选展示和活动关联的 read model。

需要重评：

- ID 输入是否改为搜索/选择器；
- 展示模型如何与 selection 使用同一 Strategy version；
- 缓存与失效；
- 候选/概率是否允许公开；
- loading/empty/stale/error；
- 用户是否仍可手工输入 ID。

只有这时才有条件设计可信转盘或候选预览。

### 9.3 Endpoint 获得端到端幂等协议

触发条件：服务端持久化业务键并能返回同一操作的同一结果。

需要重评：

- 自动重试允许哪些错误和次数；
- key 由谁生成、多久有效；
- key reuse 与不同 payload 冲突怎样返回；
- UI 中“重试”与“新建”怎样区分；
- retry budget、backoff 和可观测性。

在协议到来前继续保持零自动重试。

### 9.4 状态转换开始增多

触发条件：加入确认、资格检查、积分冻结、等待履约、取消、补偿、恢复或并行请求；团队开始出现非法转换 bug。

需要重评：

- reducer；
- 显式状态机/XState；
- transition table；
- server state library；
- route-level resource ownership；
- 状态持久化与恢复。

不要因为“项目大”升级，要因为转换复杂度和所有权真的变化。

### 9.5 导航规模或权限模型增长

触发条件：导航项显著增加、不同角色看到不同页面、同一路由跨多个 Shell 重复登记。

需要重评：

- 单一 typed route manifest；
- route handle/meta；
- 权限过滤与不可见/不可达一致性；
- breadcrumb；
- search 与 navigation 共用索引；
- 懒加载与预取。

### 9.6 搜索从“找页面”升级为“找业务实体”

触发条件：用户需要搜索活动、交易、用户、Strategy 或日志。

需要重评：

- 后端 query contract；
- debounce、取消和竞态；
- 权限与敏感字段；
- 排序、分组和高亮；
- 最近历史；
- empty/error/stale；
- 键盘 combobox/listbox pattern；
- 性能与日志隐私。

### 9.7 多个 overlay 或复杂表单出现

触发条件：搜索中再打开详情、通知中有交互、drawer 上叠 modal，或焦点 bug 开始反复出现。

需要重评：

- 原生 `<dialog>`；
- Radix/React Aria 等成熟 primitive；
- 统一 overlay manager；
- inert background；
- scroll lock reference count；
- topmost Escape policy；
- e2e keyboard/读屏回归。

### 9.8 Mock 页面接入真实 API

触发条件：积分、优惠券、账户或通知出现稳定后端合同。

替换步骤：

1. 先定义事实来源和错误语义；
2. 写 adapter/decoder；
3. 加 loading/empty/error/stale；
4. 让摘要仍由同一 response 派生；
5. 删除对应 Mock，而不是同时保留两份隐式 fallback；
6. 更新所有 snapshot 标签和禁止词测试；
7. 做真实浏览器 Network 验收。

### 9.9 图表成为关键决策工具

触发条件：运营人员必须从图表读取精确值、比较时段或执行决策。

需要重评：

- 完整数据表或可访问详情；
- 键盘可达数据点；
- 不只靠颜色；
- 单位、时间范围和数据更新时间；
- 空值、异常值和时区；
- reduced-motion；
- 大数据量性能与降采样。

### 9.10 设计体系跨端或跨团队

触发条件：多个 Web 应用、移动端或团队开始共享视觉规则，utility class 漂移导致 review 成本上升。

需要重评：

- 语义 token；
- DTCG-compatible source；
- component variants；
- Storybook/文档站；
- visual regression；
- token version/deprecation；
- 品牌层与产品层分离。

### 9.11 用户研究推翻当前密度假设

触发条件：目标用户不是高频运营者、移动端占比远高于桌面、用户无法理解 technical workbench，或关键任务完成时间上升。

需要重评：

- 是否拆分“开发演示台”和“消费者抽奖页”；
- 默认信息密度；
- 渐进披露位置；
- 导航命名；
- onboarding 与帮助内容；
- 是否仍以侧栏为主。

设计参考不是不可改变的答案，真实任务证据优先。

---

## 10. 本节可复用的设计检查表

虽然推理仍是九问，落地时可以用下面的短表做 code review。

### 10.1 新增真实 API 页面前

- [ ] 写清 endpoint 能证明和不能证明的业务事实。
- [ ] 区分 command、query、临时计算和持久交易。
- [ ] 检查 ID 精度、时间、金额和枚举边界。
- [ ] 确定未知结果是否允许自动重试。
- [ ] 决定运行时 decoder 的 closed/open 字段。
- [ ] 列出 idle/loading/success/empty/error/cancel/stale。
- [ ] 定义谁拥有请求，谁能取消，旧响应怎样失效。
- [ ] 先写结果文案，再写视觉效果。

### 10.2 引入设计参考时

- [ ] 先说参考解决的任务是否相似。
- [ ] 提取 layout/color/type/spacing/elevation/motion token。
- [ ] 保留自己的品牌、内容和业务合同。
- [ ] 不复制登录数据、文案或未经授权资产。
- [ ] 对比不仅看像不像，还看信息顺序和任务耗时。
- [ ] 给出移动端和无障碍对应方案。

### 10.3 Mock 与真实能力共存时

- [ ] 每个页面标记数据来源和 snapshot 时间。
- [ ] 同一实体只有一个 Mock source。
- [ ] 摘要从原始 snapshot 派生。
- [ ] 真实交互不被描述成真实业务后台。
- [ ] 不用随机数模拟服务端已有能力。
- [ ] 不在错误时静默 fallback 到 Mock。
- [ ] 接入真实 API 后删除对应 Mock 路径。

### 10.4 Layout 与 overlay review

- [ ] active route 使用 segment boundary 和明确 alias。
- [ ] link 有可访问名称和 `aria-current`。
- [ ] drawer/search 有 dialog name 与 modal semantics。
- [ ] 打开后焦点进入，Tab 不逃逸。
- [ ] Escape/backdrop/导航后能关闭。
- [ ] 关闭后焦点回到合理调用者。
- [ ] body scroll lock 正确恢复。
- [ ] route change 清理悬空 overlay。
- [ ] notification 等非模态浮层不误标 modal。

### 10.5 验收前

- [ ] 单测精确检查 request shape 与调用次数。
- [ ] deferred Promise 覆盖竞态。
- [ ] 页面测试覆盖禁止词和边界文案。
- [ ] 390px、桌面、暗色、reduced-motion 目测。
- [ ] 键盘完成全部主要任务。
- [ ] 真实 Network 核对 method/path/header/body。
- [ ] 不把截图、Mock 或单测升级为生产证据。
- [ ] 课程、QA、简历使用同一业务术语。

---

## 11. 最终结论

第 22 节最有价值的成果不是一个视觉更强的抽奖页，而是建立了三种一致性：

1. **合同一致性：** 浏览器严格按 empty-body、无 query、无幂等键、无自动重试的合同调用 ephemeral endpoint。
2. **语义一致性：** `reward` 只是候选，`no_reward` 是正常结果，未知网络结果不被伪造成业务结论，Mock 页面不冒充实时系统。
3. **产品一致性：** 共享工作台用稳定侧栏、顶部工具、紧凑数据层级、响应式披露和焦点管理，把不同模块放进同一个可学习、可核查的框架。

第一性原则不是拒绝丰富功能，而是先问：哪一句话可以由当前系统证明？如果证明不了，就把它写成限制或触发器，而不是用视觉、技术名词或简历措辞掩盖。

当 Draw、资格、积分、库存、发奖和幂等协议真正出现时，这套设计并不会被推翻；它已经预留了升级的判断条件。届时应升级的是事实与状态机，而不是回头为今天的虚假承诺补解释。

---

## 12. 资料与证据入口（访问日期：2026-08-30）

### 12.1 项目内部事实

- [第 21 节设计手记](./lesson-21.md)：九问结构、ephemeral API 的前置决策。
- [第 22 节课程文档](../../course/part-03/lesson-22-react-lottery-page.md)：实现与验收路径。
- [第 22 节面试 QA](../../interview/lessons/lesson-22.md)：可口述问题、追问、误区与代码落点。
- [HTTP client](../../../web/src/api/httpClient.ts)。
- [Lottery API adapter](../../../web/src/api/lotteryApi.ts)。
- [Lottery request Hook](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts)。
- [Lottery page](../../../web/src/pages/user/lottery/LotteryPage.tsx)。
- [Workspace Shell](../../../web/src/components/layout/WorkspaceShell.tsx)。
- [User navigation](../../../web/src/layouts/UserLayout.tsx)。
- [Shared product page primitives](../../../web/src/components/common/ProductPage.tsx)。
- [Unified mock snapshot](../../../web/src/mocks/growthOsMockData.ts)。

### 12.2 React 与路由官方资料

- [React：Choosing the State Structure](https://react.dev/learn/choosing-the-state-structure)：避免矛盾、冗余和重复 state，从已有数据派生结果。
- [React：Reacting to Input with State](https://react.dev/learn/reacting-to-input-with-state)：先枚举视觉状态与触发事件。
- [React：Reusing Logic with Custom Hooks](https://react.dev/learn/reusing-logic-with-custom-hooks)：共享有状态逻辑而非共享状态实例。
- [React：Synchronizing with Effects](https://react.dev/learn/synchronizing-with-effects)：setup/cleanup、取消或忽略乱序请求。
- [React：You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect)：用户事件与 Effect 的职责边界。
- [React Router：NavLink](https://reactrouter.com/api/components/NavLink)：active/pending 状态与 `aria-current`。
- [React Router：useLocation](https://reactrouter.com/api/hooks/useLocation)：在位置变化后执行界面副作用。
- [React Router：ScrollRestoration](https://reactrouter.com/api/components/ScrollRestoration)：模拟浏览器滚动位置恢复；用于理解当前手工 scroll reset 的边界。

### 12.3 Web、HTTP 与 JavaScript 官方资料

- [MDN：AbortController](https://developer.mozilla.org/en-US/docs/Web/API/AbortController)。
- [MDN：AbortSignal](https://developer.mozilla.org/en-US/docs/Web/API/AbortSignal)。
- [MDN：Number.MAX_SAFE_INTEGER](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Number/MAX_SAFE_INTEGER)。
- [MDN：Idempotent](https://developer.mozilla.org/en-US/docs/Glossary/Idempotent)。
- [RFC 9110 §9.2.2：Idempotent Methods](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2)。
- [MDN：aria-current](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Attributes/aria-current)。
- [MDN：aria-expanded](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Attributes/aria-expanded)。
- [MDN：prefers-reduced-motion](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/%40media/prefers-reduced-motion)。

### 12.4 W3C/WAI 与设计系统资料

- [WAI-ARIA APG：Modal Dialog Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)：初始焦点、Tab 循环、Escape 和焦点恢复。APG 是实践指导，不应误称为 W3C Recommendation。
- [WAI-ARIA APG：Developing a Keyboard Interface](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/)：可见焦点和键盘约定。
- [WCAG 2.2：Understanding Non-text Content](https://www.w3.org/WAI/WCAG22/Understanding/non-text-content.html)：图表等非文本内容需要等价文本。
- [WCAG 2.2：Understanding Animation from Interactions](https://www.w3.org/WAI/WCAG22/Understanding/animation-from-interactions.html)：非必要交互动画与关闭机制。
- [WCAG 2.2：Understanding Reflow](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html)：窄视口下的重排要求。
- [Design Tokens Format Module 2025.10](https://www.designtokens.org/tr/2025.10/format/)：用平台无关方式表达设计决策、共享词汇和单一来源。该文档是 Design Tokens Community Group 报告，不是 W3C Standard；本项目也未声称实现其文件格式。
