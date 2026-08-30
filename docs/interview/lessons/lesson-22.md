# 第 22 节面试题：React Lottery 临时选择页

本文只描述第 22 节能够由当前代码、测试和浏览器验收证明的前端纵向切片：用户在 React 页面中明确输入一个规范 `uint64` Strategy ID，页面通过同源、无请求体、无自动重试的 POST 调用第 21 节 ephemeral selection API，并把服务端返回的 `reward` 或 `no_reward` 渲染为一次非持久化选择结果。

它仍然不是正式抽奖页：没有用户或租户身份、活动资格、抽奖次数、积分扣减、库存预占、Draw ID、结果持久化、奖励发放、幂等重放、审计账本、限流或 Redis 业务能力。页面刷新后不会恢复结果；Request ID 只用于故障关联；`reward` 只表示“选中了奖励候选”，不表示已经中奖或发奖。

## 来源与使用原则

- `项目事实` 只来自本仓库代码、测试和第 22 节 QA；回答时应说“当前实现证明了什么”，不要把未来方案说成已经完成。
- `面经题型触发` 来自牛客用户发布的个人复盘或题目整理，只说明公开面试中出现过这类问题。帖子不是公司官方题库，帖内答案也不作为技术规范。
- `技术结论` 优先依据 React、MDN、W3C/WAI、IETF RFC 与 GOV.UK Design System 等官方资料。
- 所有外部链接最后复核日期为 **2026-08-30**。引用牛客时只概述题型，不大段复刻原文。

## 1. 第 22 节到底实现了什么？“真实抽奖页”这个说法哪里不准确？

- **可直接口述：** 我实现的是一个真实调用后端的 React 临时选择页，不是正式抽奖交易页。真实之处在于结果不再由浏览器 Mock 或 `Math.random()` 决定，而是经过同源 HTTP、GrowthOS-Go、MySQL Strategy 快照和服务端选择器返回；临时之处在于没有创建 Draw、保存结果、扣积分、锁库存或发奖。为了不让 UI 超出后端能证明的事实，我把 `reward` 写成“选中了奖励候选”，把 `no_reward` 当成正常成功结果，并在页面上明确 Development/Test only 与非持久化边界。
- **面试官可能追问：** 如果产品希望按钮和文案更有“中奖氛围”，你会不会直接写“恭喜中奖”？回答应是：视觉表现可以更有情绪，但业务断言不能升级。只有服务端持久化 Draw、完成资格/库存/发放并给出可查询状态后，页面才能声称“已中奖”或“已发放”。
- **常见误区：** 把“浏览器拿到了一个 Award”说成“抽奖闭环”；把 Development/Test 标签当认证；把 `durability: ephemeral` 当可追溯凭证；为了简历好看虚构积分、Redis 锁、库存或高并发指标。
- **本项目代码落点：** [LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 的标题、边界说明、结果文案和“本节明确没有实现”；[LotteryPage.test.tsx](../../../web/src/pages/user/lottery/LotteryPage.test.tsx) 断言页面不出现“恭喜中奖”“获得了”等虚假承诺；[lotteryApi.ts](../../../web/src/api/lotteryApi.ts) 将返回值限制为 `durability: "ephemeral"`。
- **依据：** 牛客[小公司面试](https://www.nowcoder.com/discuss/609410185911652352)体现了面试官会拒绝脱离业务定义的通用背诵；GOV.UK [Confirmation pages](https://design-system.service.gov.uk/patterns/confirmation-pages/)只在事务确实完成时使用完成确认。

## 2. 为什么页面要求用户输入 Strategy ID，而不展示一个看起来完整的奖品转盘或候选列表？

- **可直接口述：** 当前后端只提供按 Strategy ID 发起临时选择的接口，没有 Strategy 列表、候选权重、活动展示配置或用户资格接口。前端如果自行造奖品格子、概率和默认 Strategy，会建立第二个事实来源：用户看到的候选可能与服务端实际快照不一致。因此本节选择最小而诚实的交互——用户明确输入 ID，前端只展示后端确实返回的结果。等后端提供带版本和权限边界的查询接口后，再做可视化转盘。
- **面试官可能追问：** 能不能先把候选列表写在前端常量里改善体验？可以用于纯视觉原型，但不能与真实选择按钮组合后冒充同一份配置；否则配置变更、灰度、权限和缓存失效都会让 UI 与服务端分叉。更合理的是由后端返回只读展示模型及版本号，并把选择结果关联到同一配置版本。
- **常见误区：** 认为“只是展示，写死无所谓”；把演示数据当生产数据；默认填入一个 Strategy ID 导致页面挂载后或误按回车触发不属于用户的选择；为了炫技让浏览器再随机播放一个本地结果。
- **本项目代码落点：** [LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 初始 ID 为空，按钮禁用，并明确“当前后端没有 Strategy 列表接口”；[useEphemeralLotterySelection.test.tsx](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.test.tsx) 验证挂载和 React Strict Mode setup-cleanup-setup 都不会自动发请求。
- **依据：** React [Reacting to Input with State](https://react.dev/learn/reacting-to-input-with-state)强调先枚举真实视觉状态和触发来源；GOV.UK [Check answers](https://design-system.service.gov.uk/patterns/check-answers/)强调在真正提交前清晰表达事务尚未完成。

## 3. 为什么把请求生命周期放进自定义 Hook，而不是全部写在页面组件里？

- **可直接口述：** 页面组件负责业务文案、表单和可访问渲染，Hook 负责选择流程的状态、请求所有权、取消、过期结果抑制和重复提交保护。这样 `LotteryPage` 只消费 `state/select/clear` 这三个意图明确的接口，不需要知道 AbortController 和 generation 的细节；Hook 也能脱离视觉层单独测试生命周期边界。这个拆分共享的是有状态逻辑，不是把状态变成全局单例。
- **面试官可能追问：** 为什么不抽象成通用 `useRequest`？当前流程有领域特有规则：同一时刻只允许一次临时选择、清空即失效旧 generation、未知结果不能重试、取消不显示成业务失败。过早做成万能 Hook 会暴露大量开关并削弱语义；只有多个用例出现稳定共同点后再下沉通用层。
- **常见误区：** 认为所有函数都应抽 Hook；自定义 Hook 命名很通用但内部耦合 Lottery；把请求直接放在 render；认为两次调用同一个 Hook 会共享同一份状态。
- **本项目代码落点：** [useEphemeralLotterySelection.ts](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts) 封装判别联合状态、AbortController、generation 与 clear；[useEphemeralLotterySelection.test.tsx](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.test.tsx) 独立验证成功、失败、重复提交、卸载和陈旧 Promise。
- **依据：** 牛客[字节跳动国际化商业产品与技术前端实习生一面凉经](https://www.nowcoder.com/discuss/646401290917924864)记录了“为什么选状态库、如何封装自定义 Hook、业务场景是什么”的连续追问；React [Reusing Logic with Custom Hooks](https://react.dev/learn/reusing-logic-with-custom-hooks)说明自定义 Hook 共享有状态逻辑而非状态本身。

## 4. 为什么没有用 Redux、Zustand 或 XState？

- **可直接口述：** 当前状态只服务一个页面、刷新即应丢失，也没有跨路由消费者；用本地 Hook 可以保持所有权和生命周期清晰。Redux/Zustand 适合多个远距离组件共享或跨页面协调，当前引入会增加依赖、清理和持久化误解。XState 能显式约束复杂状态图，但本节只有四个阶段和很少的转换，TypeScript 判别联合已经能排除多数不可能状态。未来如果加入确认、积分冻结、发奖、补偿和可恢复 Draw，再评估 reducer 或状态机库。
- **面试官可能追问：** 什么信号出现时会升级？当状态跨多个页面、需要恢复/回放、多个并行 actor、超时与补偿、权限导致不同转换，或团队经常因为非法转换出错时，状态机的可视化和转换约束才开始抵消其成本。
- **常见误区：** “项目大就必须 Redux”；为了简历技术栈数量引入全局库；把服务端状态长期塞入客户端 Store；反过来说本地状态永远优于状态机。
- **本项目代码落点：** [useEphemeralLotterySelection.ts](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts) 只返回页面局部状态；[LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 没有 localStorage 或全局 Store 依赖。
- **依据：** 同一份[字节前端面经](https://www.nowcoder.com/discuss/646401290917924864)包含 Zustand 选型与替代方案追问；React [Extracting State Logic into a Reducer](https://react.dev/learn/extracting-state-logic-into-a-reducer)明确列出 reducer 在代码量、可读性、调试和测试上的收益与代价。

## 5. 为什么状态建模为 `idle | selecting | success | error`，而不是 `isLoading/isSuccess/hasError` 三个布尔值？

- **可直接口述：** 三个布尔值理论上能组合出八种状态，其中“loading 与 success 同时为 true”之类组合没有业务意义。判别联合只允许四种合法阶段，并让每个阶段携带自己需要的数据：selecting 有 Strategy ID，success 有响应，error 有分类错误。页面按 `phase` 穷举渲染，减少状态不同步和陈旧数据残留。
- **面试官可能追问：** 为什么这里没用 reducer？状态转换不多，`select/clear/Promise callbacks` 已集中在一个 Hook 中，`useState` 更短。若未来 action 数量和转换规则增长，可把 transition 提取为纯 reducer 并单测非法转换。
- **常见误区：** 在 success 之外再保留一个独立旧 error；根据是否有 response 推导 loading；把 `no_reward` 建模为 error；在页面多个事件处理器里分别修改若干布尔值。
- **本项目代码落点：** [useEphemeralLotterySelection.ts](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts) 的 `EphemeralSelectionState` 判别联合；[LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 依据 `phase` 渲染互斥状态。
- **依据：** React [Choosing the State Structure](https://react.dev/learn/choosing-the-state-structure)建议避免互相矛盾的“不可能状态”；React [Reacting to Input with State](https://react.dev/learn/reacting-to-input-with-state)给出 empty、submitting、success、error 的状态图方法。

## 6. 为什么既使用 AbortController，又使用 generation/request token？

- **可直接口述：** AbortController 解决资源和生命周期问题，generation 解决最终状态正确性。clear 或卸载时 abort 可以让支持 signal 的 fetch 尽快停止；但 abort 不能撤销服务端已经执行的工作，也不能保证所有 Promise 适配器立即拒绝。generation 为每次有效请求分配逻辑世代，回调只有在世代仍匹配且 signal 未 abort 时才能写状态，因此旧请求即使晚到也覆盖不了新结果。
- **面试官可能追问：** 只检查 `signal.aborted` 不够吗？如果旧 controller 没被正确传播、测试替身忽略 signal，或未来复用逻辑时出现多个控制器，generation 仍提供独立的 last-valid-operation 约束。反过来只用 generation 会让无用请求继续占用网络和服务资源，所以两层各有职责。
- **常见误区：** 说 abort 能回滚服务端选择；catch 到 AbortError 后仍显示红色错误；只在组件卸载时取消，却不在 clear/输入变更时使旧回调失效；用一个全局布尔 `isMounted` 混淆多个请求。
- **本项目代码落点：** [useEphemeralLotterySelection.ts](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts) 中 `generation`、`activeController`、回调前置检查、`clear()` 与 effect cleanup；对应测试构造旧 Promise 晚拒绝并证明不会覆盖新成功结果。
- **依据：** 牛客[深信服前端一二三 HR 面经](https://www.nowcoder.com/discuss/402568671036456960)明确出现请求竞态、防抖、abort 具体实现；React [`useEffect`](https://react.dev/reference/react/useEffect)用 cleanup 的 `ignore` 防止乱序响应写回；MDN [AbortController](https://developer.mozilla.org/en-US/docs/Web/API/AbortController)定义了 abort 对 fetch、响应体和流的取消能力。

## 7. `Promise.race` 超时与 AbortController 取消有什么区别？本项目如何实现超时？

- **可直接口述：** `Promise.race([fetch, timeout])` 只让外层 Promise 先以超时结束，底层 fetch 默认还会继续，稍后仍可能消耗连接、读取响应甚至触发回调。AbortController 是把 signal 传给 fetch，并在计时到期时调用 `abort()`，真正通知支持该信号的异步操作停止。本项目 HTTP client 用内部 controller 汇合调用方取消和五秒超时，并用 `timedOut` 区分 timeout 与主动 cancelled，finally 中清 timer 和监听器。
- **面试官可能追问：** abort 后是否能断言服务端没执行？不能。客户端只能确认不再等待，无法从连接中断推断服务端事务是否开始或完成。这也是未知 POST 结果不能自动重试的原因。
- **常见误区：** 把 `Promise.race` 称为网络取消；所有 AbortError 都映射成 timeout；忘记清理 timer 和外部 signal listener；超时后告诉用户“服务端执行失败”。
- **本项目代码落点：** [httpClient.ts](../../../web/src/api/httpClient.ts) 的 `executeJSONRequest` 创建内部 controller、转发 caller signal、超时 abort、分类 `timeout/cancelled` 并在 finally 清理；[httpClient.test.ts](../../../web/src/api/httpClient.test.ts) 验证取消与 timeout 配置边界。
- **依据：** 牛客[快手前端面经](https://www.nowcoder.com/discuss/541988931953287168)二面直接考 `Promise.race` 与 AbortController 的超时中断封装；MDN [AbortSignal](https://developer.mozilla.org/en-US/docs/Web/API/AbortSignal)与 [AbortController](https://developer.mozilla.org/en-US/docs/Web/API/AbortController)给出标准取消机制。

## 8. 清空、修改 Strategy ID、组件卸载时应该发生什么？为什么页面挂载时不自动请求？

- **可直接口述：** 清空或修改输入意味着旧结果已经与当前用户意图不匹配，所以 `clear()` 先推进 generation，再 abort 活动请求并回到 idle。卸载时做同样的资源清理，但不再 setState。页面挂载不自动请求，因为 selection 是一次用户明确触发的动作，不是为了与某个外部读模型保持同步；Strict Mode 在开发环境会额外执行 setup-cleanup，如果把选择写进 Effect，可能产生难以解释的额外 POST。
- **面试官可能追问：** 为什么输入一变就清除已经成功的结果？否则界面会同时显示新输入和旧 Strategy 的结果，形成跨实体错配。若产品希望保留历史，应把历史建模为带 Strategy ID 和 Draw ID 的独立列表，而不是让一个“当前结果”卡片含混存在。
- **常见误区：** 在 effect 中看到 ID 有值就自动选择；卸载 cleanup 里 setState；修改 ID 后保留旧奖品但不标旧 ID；认为 React Strict Mode 重复 setup 是生产行为或 React bug。
- **本项目代码落点：** [LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 的 input `onChange` 同时更新 ID 和调用 `clear()`；[useEphemeralLotterySelection.ts](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts) 的 effect 只负责卸载清理；[useEphemeralLotterySelection.test.tsx](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.test.tsx) 覆盖 unmount、clear 和 Strict Mode 零自动请求。
- **依据：** React [Synchronizing with Effects](https://react.dev/learn/synchronizing-with-effects)要求 Effect cleanup 镜像 setup，并指出 fetch 应取消或忽略；React [You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect)区分用户交互事件与同步外部系统的 Effect。

## 9. 快速双击只发一个请求，是否就等于接口幂等？为什么用了两层防重复？

- **可直接口述：** 不等于。页面禁用按钮是可见反馈，Hook 的 `activeController.current !== null` 是同步逻辑闸门，防止同一事件批次、程序调用或 UI 状态尚未重渲染时穿透；它们只降低单个浏览器中的重复提交。接口幂等讨论的是相同请求被网络、代理、客户端或多实例重复执行时的服务端预期效果，需要服务端业务键、持久结果、唯一约束或状态机等机制保证，不能靠按钮置灰替代。
- **面试官可能追问：** 为什么不用 debounce？selection 是离散点击，不是连续输入采样；debounce 会延迟第一次明确操作，而且跨标签页、刷新、网络重放仍无效。in-flight guard 更直接地表达“当前只能有一次未决选择”。
- **常见误区：** “按钮 disabled 所以接口幂等”；把 debounce、节流和幂等混为一谈；只依赖 React state 的 `selecting`，忽略同批次内 state 尚未提交；无服务端协议却自行加 Idempotency-Key。
- **本项目代码落点：** [LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 在 selecting 时禁用输入和按钮；[useEphemeralLotterySelection.ts](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.ts) 在创建请求前检查 `activeController`；测试连续调用三次 `select()` 仍断言 transport 只有一次。
- **依据：** 牛客[美团数据系统研发日常实习一面整理](https://www.nowcoder.com/discuss/722397808745033728)与[多益网络 Java 一面](https://www.nowcoder.com/discuss/353159215873204224)都出现接口幂等追问；MDN [Idempotent](https://developer.mozilla.org/en-US/docs/Glossary/Idempotent)给出 HTTP 幂等的准确含义。

## 10. 为什么这个 POST 失败后不自动重试？用户再次点击又算什么？

- **可直接口述：** 当前 endpoint 没有 Draw ID、幂等键和持久结果。连接超时或响应丢失时，前端无法知道服务端是否已完成以及选中了什么；透明重试会执行一次新的随机选择，并可能用第二个结果掩盖第一个未知结果。因此 HTTP client、API adapter 和 Hook 都只发一次。用户看到说明后再次点击，是显式发起一次新的 ephemeral selection，不是恢复或重试同一业务操作。
- **面试官可能追问：** POST 是否永远不能重试？不是。RFC 允许客户端在明确知道操作语义幂等、或能确定原请求未被应用时重试。正式 Draw 可以使用服务端持久化的 idempotency key + 调用主体 + 请求摘要绑定相同结果，随后安全重放；但这必须是端到端协议，不能只在前端生成随机 header。
- **常见误区：** “POST 按标准绝对不能重试”；把网络错误解释为服务端未执行；指数退避适用于所有请求；遇到 502/504 自动再抽一次；把用户第二次点击仍称作“重试原抽奖”。
- **本项目代码落点：** [httpClient.ts](../../../web/src/api/httpClient.ts) 没有 retry loop；[lotteryApi.test.ts](../../../web/src/api/lotteryApi.test.ts) 和 [httpClient.test.ts](../../../web/src/api/httpClient.test.ts) 都断言失败只调用一次 fetch；[LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 明示再次操作是全新的临时选择。
- **依据：** [RFC 9110 §9.2.2](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2)规定客户端不应自动重试非幂等请求，除非知道其语义幂等或能确认原请求未应用；MDN [POST](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Methods/POST)说明相同 POST 可能产生额外效果。

## 11. 为什么 `no_reward` 是 success，不是 error 或 404？

- **可直接口述：** transport success 和业务 outcome 是两个维度。服务端成功读取 Strategy、完成选择并返回一个合法 `no_reward` Award，说明请求处理成功，只是业务结果是未中奖候选，所以 Hook 进入 success、页面用 `role="status"` 呈现。404 表示 Strategy 或 route 不存在，5xx 表示服务无法给出可信结果；把这些错误降级为 `no_reward` 会掩盖故障并污染概率与运营数据。
- **面试官可能追问：** 为什么不把 reward/no_reward 设计成布尔值？封闭字符串枚举更可读，也为未来在显式版本演进中增加其他 outcome 留出契约空间；但客户端仍必须拒绝当前未知 outcome，不能猜测其含义。
- **常见误区：** “没中奖就是失败”；用 catch 返回默认未中奖；把未知响应字段当 no_reward；404 文案写成“本次没抽中”。
- **本项目代码落点：** [lotteryApi.ts](../../../web/src/api/lotteryApi.ts) 只接受 `"reward" | "no_reward"`；[LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 在 success 分支分别渲染两种 outcome；[LotteryPage.test.tsx](../../../web/src/pages/user/lottery/LotteryPage.test.tsx) 断言 no_reward 没有 alert。
- **依据：** React [Reacting to Input with State](https://react.dev/learn/reacting-to-input-with-state)把网络成功/失败作为不同计算机输入驱动状态；W3C [Status Messages](https://www.w3.org/WAI/WCAG21/Understanding/status-messages)说明结果状态应被辅助技术感知，而非都升级为警报。

## 12. 为什么 `reward` 只能写“选中了奖励候选”，不能写“恭喜中奖”或“已发奖”？

- **可直接口述：** 前端文案必须对应可以验证的领域事实。当前响应只包含 ephemeral selection、Strategy ID 和 Award，没有用户、Draw、库存、权益订单或发放状态。因此 `reward` 只证明算法选择了奖励候选；“中奖”通常意味着形成可追溯权利，“已发奖”更意味着履约完成，当前都没有证据。诚实文案不是保守措辞，而是防止用户、客服、测试和监控基于错误状态做后续决策。
- **面试官可能追问：** 正式链路需要哪些状态？至少应区分 Draw created、selected、eligibility confirmed、inventory reserved、fulfillment pending/succeeded/failed，以及可审计的最终结果；是否合并取决于一致性边界，但不能只靠一个前端 success 卡片替代。
- **常见误区：** 认为 UI 文案不属于架构；后端返回 award 就等于用户获得 award；动画结束代表交易完成；用绿色卡片自动推导“成功履约”。
- **本项目代码落点：** [LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 明确写“这不是中奖记录，也不表示库存已预占或奖励已发放”；页面测试检查不存在“恭喜中奖”和“获得了”。
- **依据：** GOV.UK [Confirmation pages](https://design-system.service.gov.uk/patterns/confirmation-pages/)把完成确认限定在已完成事务并要求解释下一步；[Error message](https://design-system.service.gov.uk/components/error-message/)要求文案具体说明发生了什么，而非含糊或误导。

## 13. Request ID、Strategy ID、Award ID 和未来 Draw ID 有什么区别？

- **可直接口述：** Strategy ID 标识配置聚合，Award ID 标识配置里的候选；Request ID 标识一次 HTTP/链路观测，帮助把浏览器错误与网关、服务日志关联，但不证明业务结果持久存在。未来 Draw ID 才应标识一次可查询、可审计、可幂等重放的抽奖业务事实。四者生命周期、生成方和唯一性范围不同，不能因为都长得像字符串就互换。
- **面试官可能追问：** 为什么页面展示 Request ID？它让用户或测试人员在错误时提供关联信息，降低排障成本；但标签明确写“仅用于故障关联”，避免被截图当领取凭证。若响应 header 和 JSON error envelope 的 request ID 冲突，客户端应当拒绝契约，而不是任选一个。
- **常见误区：** 把 Request ID 当幂等键；用它恢复前一次结果；把 Strategy ID 当 Draw ID；日志里有请求记录就声称有业务审计账本。
- **本项目代码落点：** [httpClient.ts](../../../web/src/api/httpClient.ts) 读取 `X-Request-ID` 并校验错误 envelope 一致性；[LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 将其标为“仅用于故障关联”；当前没有 Draw 类型或 Draw 路由。
- **依据：** 项目 API 契约是这里的直接依据；牛客[小公司面试](https://www.nowcoder.com/discuss/609410185911652352)体现了面试官会从通用术语继续追问真实业务 identity 和有效操作定义。

## 14. 前端如何保证请求精确符合“无 body、无 query、特定 header”的契约？

- **可直接口述：** 我没有复用一个默认把对象 `JSON.stringify` 的 POST helper，而是增加 `postJSONWithoutBody`：它的类型接口根本不提供 `body` 参数，只添加 `Accept`，不会推断 Content-Type。Lottery adapter 构造精确 path、只添加 `X-GrowthOS-Demo-Mode: ephemeral-selection`，不加 query、fragment 或 Idempotency-Key；transport 还限制同源绝对路径、`credentials: same-origin`、`cache: no-store` 和 `redirect: error`。这样约束在 API 形状、运行时和测试三层可见。
- **面试官可能追问：** 为什么没有 body 就不发 `Content-Type: application/json`？Content-Type 描述实际传输的表示；没有 payload 就没有 JSON 表示。无意义 header 可能触发额外预检、误导网关或让契约测试失真。
- **常见误区：** `body: JSON.stringify({})` 与无 body 等价；空字符串 body 一定不会产生 framing；所有 POST 都必须 application/json；前端可以添加 seed/query 方便调试；自定义 header 就是认证。
- **本项目代码落点：** [httpClient.ts](../../../web/src/api/httpClient.ts) 的 `postJSONWithoutBody` 注释和无 body 类型；[lotteryApi.ts](../../../web/src/api/lotteryApi.ts) 的精确路径/header；两份 API 测试断言没有 body、Content-Type、Content-Length、Transfer-Encoding、query 和 Idempotency-Key。
- **依据：** MDN [POST](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Methods/POST)说明 Content-Type 用于表示请求体媒体类型；第 21 节服务端契约与本节 transport 测试共同构成项目事实。

## 15. TypeScript 已经有接口类型，为什么还要对 JSON 做运行时解码？

- **可直接口述：** TypeScript 类型在编译后不存在，网络 JSON 仍是 `unknown`。我先验证 wrapper、`durability === "ephemeral"`、响应 Strategy ID 与请求一致、Award 是对象、ID 是规范 uint64 string、name 的边界和 outcome 封闭枚举，再映射成窄的 camelCase DTO。结构不符就抛 contract error，不把未知数据渲染成真实抽奖结果。允许额外字段是为了 additive compatibility，但只向 UI 暴露当前认识的字段。
- **面试官可能追问：** 为什么不对任何额外字段都失败？服务端增加可选字段通常应保持向后兼容；严格拒绝未知字段会让无关扩展击穿旧前端。这里采用“已知安全关键字段严格、额外字段忽略”的策略。若字段影响签名或安全决策，则应另行收紧。
- **常见误区：** `response.json() as Selection` 就完成校验；只检查 HTTP 200；服务端可信所以无需防御；响应 Strategy ID 不必与请求对照；未知 outcome 当 success。
- **本项目代码落点：** [lotteryApi.ts](../../../web/src/api/lotteryApi.ts) 的 `decodeEphemeralSelection` 和 `validAwardName`；[lotteryApi.test.ts](../../../web/src/api/lotteryApi.test.ts) 覆盖缺字段、错误 durability、ID 类型/范围、名称控制字符、孤立代理项、未知 outcome 与 additive fields。
- **依据：** React 与 TypeScript 不替代运行时边界验证；技术结论以实际 JavaScript 执行模型和本项目负面测试为证。MDN [Fetch API](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API)说明 fetch 返回 Response，应用仍需自行读取和解释响应体。

## 16. 为什么 Strategy ID 和 Award ID 始终使用 string，而不是 number 或 BigInt？

- **可直接口述：** 后端 ID 是完整 `uint64`，上限 `18446744073709551615`，远大于 JavaScript `Number.MAX_SAFE_INTEGER` 的 `2^53-1`。转成 number 可能静默丢精度，导致请求错误 Strategy、结果 ID 改写或日志关联失败。string 能在 URL、JSON、React input 和 DOM 中无损透传，也符合“身份不是拿来做算术”的语义。BigInt 可以精确计算，但 JSON 原生序列化、表单与第三方生态需要额外约定；当前没有 ID 运算需求，收益不足以抵消成本。
- **面试官可能追问：** 规范字符串为什么拒绝 `01`、`+1`、空格和指数形式？同一 identity 只保留一种表示，能避免缓存键、日志、签名和比较歧义。前端用正则限制 1–20 位，再用同长度字典序与最大值比较，不经过 number。
- **常见误区：** `parseInt` 后再转回字符串；认为 JS number 能精确表示所有 64 位整数；只在 UI 使用 string、API adapter 又转 number；用 `input type="number"` 接收超大 ID 并依赖浏览器数值解析。
- **本项目代码落点：** [lotteryApi.ts](../../../web/src/api/lotteryApi.ts) 的 `isCanonicalUint64ID`；[LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 使用 `type="text"` + `inputMode="numeric"`；测试用 MaxUint64 和 `9007199254740993` 证明无 numeric coercion。
- **依据：** MDN [`Number.MAX_SAFE_INTEGER`](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Number/MAX_SAFE_INTEGER)说明超过 `2^53-1` 后整数级比较和表示不再可靠。

## 17. 这个页面如何做无障碍？为什么 success 用 `status`、error 用 `alert`？

- **可直接口述：** 页面优先使用原生语义：`main` 关联主标题，表单使用显式 label，帮助和错误文本通过 `aria-describedby` 关联，非法输入用 `aria-invalid`，请求区域用 `aria-busy`，提交用真正的 button 和 disabled。选择中与成功结果是用户预期的非紧急状态，因此使用 `role="status"`/polite live region；错误需要及时提醒，使用 `role="alert"`。图标只是装饰，统一 `aria-hidden`，不能让屏幕阅读器朗读无意义图标名。
- **面试官可能追问：** 为什么不在成功后强制移动焦点？状态 live region 已能播报，强行抢焦点会打断键盘用户当前上下文。只有新内容需要立即操作、或流程发生明确上下文切换时，才考虑经过验证的 focus management。
- **常见误区：** 有 `aria-label` 就算无障碍完成；placeholder 替代 label；所有动态消息都用 assertive alert；只靠红绿颜色区分；给 div 加 click 冒充 button；disabled 后完全不给用户解释原因。
- **本项目代码落点：** [LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 的 label、describedby、invalid、busy、status、alert、disabled 和 aria-hidden；[LotteryPage.test.tsx](../../../web/src/pages/user/lottery/LotteryPage.test.tsx) 通过可访问角色和名称查询页面，而非只查 class。
- **依据：** 牛客[叠纸前端一面面经](https://www.nowcoder.com/discuss/663163502961438720)出现 Web Accessibility 与 AJAX 错误反馈题型；W3C/WAI [Labeling Controls](https://www.w3.org/WAI/tutorials/forms/labels/)、[Button Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/button/)与 [Status Messages](https://www.w3.org/WAI/WCAG21/Understanding/status-messages)分别支撑表单、按钮和动态状态做法。

## 18. 响应式页面应该怎样设计和验收？为什么不能只说“用了 Tailwind 断点”？

- **可直接口述：** 响应式是内容在不同视口、缩放和输入环境下仍可用，不是背几个设备宽度。本页采用流式容器和 Grid/Flex：窄屏单列、按钮占满可触区域，较宽时才形成主操作区与说明侧栏；长 ID 和 Request ID 允许断行。断点由内容开始拥挤的位置决定。验收至少覆盖 320 CSS px 等效宽度、400% zoom、常见桌面与移动视口，确认无信息或功能丢失、无需双向滚动、焦点样式仍可见。
- **面试官可能追问：** mobile-first 和 desktop-first 怎么选？两者都可，关键是基线内容是否天然流动以及覆盖规则是否简单。本页基线为单列，`sm`/`lg` 只增强间距和列结构，接近 mobile-first，能降低窄屏回退复杂度。
- **常见误区：** 响应式等于 rem；只在 iPhone 模拟器看一次；用固定高度裁掉错误文案；桌面双栏直接缩小到手机；通过横向滚动掩盖布局溢出；只测像素宽度不测缩放和长文本。
- **本项目代码落点：** [LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 的 `max-w`、响应式 padding、`lg:grid-cols`、`sm:w-auto`、`break-all` 等布局约束；浏览器 QA 应同时检查窄视口与桌面视口。
- **依据：** 牛客[字节本地服务前端面经](https://www.nowcoder.com/discuss/745084418779287552)和[网易传媒一面](https://www.nowcoder.com/discuss/370979597536579584/)均出现“响应式怎么做”；MDN [Responsive web design](https://developer.mozilla.org/en-US/docs/Learn_web_development/Core/CSS_layout/Responsive_Design)与 WCAG [Reflow](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html)给出多设备与 320 CSS px 重排依据。

## 19. 如何在“大厂感、艺术感”和可访问性、业务可信度之间取舍？

- **可直接口述：** 我把视觉层当信息层级的放大器，而不是业务事实生成器。可以使用统一设计 token、留白、层次化表面、克制渐变、清晰排版和状态动效增强品质，但颜色和动画不能改变语义：reward 仍是候选、no_reward 仍是正常结果、loading 动画不参与随机。暗色模式要重新检查对比度、边界和原生控件；用户设置 reduced motion 时应去掉非必要旋转、缩放和平移，同时保留文字状态。
- **面试官可能追问：** 为什么不照搬某家大厂页面？可以学习层级、栅格、token、动效节奏和内容设计原则，但直接复制品牌视觉既不符合本项目语义，也会引入版权、可维护性和一致性问题。设计应从当前用户任务与状态图出发。
- **常见误区：** 动画越多越高级；暗色就是颜色反相；用金色、礼花和“恭喜”制造虚假完成感；只在 hover 下提供反馈；关闭动画后连 loading 文本也消失；把参考大厂理解为像素级照抄。
- **本项目代码落点：** [LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 用文字与结构表达开发/临时边界，状态图标均为装饰，颜色不是唯一信息载体；全局样式和最终浏览器验收需要验证 dark scheme 与 reduced motion。
- **依据：** MDN [`prefers-reduced-motion`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/%40media/prefers-reduced-motion)、[`prefers-color-scheme`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/%40media/prefers-color-scheme)和 [`color-scheme`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/color-scheme)分别说明动效偏好、主题偏好和浏览器原生 UI 配合方式。

## 20. 错误为什么要分类？网络错误或 502/504 时为什么写“无法确认结果”而不是“选择失败”？

- **可直接口述：** 错误分类决定用户能采取什么动作，也避免泄露原始内部信息。HTTP error 带 status/code/request ID，可区分 Strategy 不存在、路由未启用、服务暂不可用；gateway、network、timeout 表示没有可信响应，其中服务端可能未收到，也可能已完成但响应丢失，所以只能说“无法确认”。contract error 表示响应结构或语义不可信，页面拒绝展示。cancelled 通常是生命周期控制，不应冒充业务失败。所有文案都说明下一步，并且不把错误降级成未中奖。
- **面试官可能追问：** 为什么不直接显示后端 message？公开错误 envelope 仍可能包含不适合最终用户的技术文案，且不同版本不稳定。UI 应按稳定 status/code 映射安全、可行动的内容，Request ID 用于支持排障；未知错误采用保守兜底。
- **常见误区：** catch 全部显示“网络错误”；502/504 就断言服务端未执行；原样输出异常对象或堆栈；contract mismatch 仍按成功字段尽力渲染；所有错误自动重试；取消也弹红色告警。
- **本项目代码落点：** [httpClient.ts](../../../web/src/api/httpClient.ts) 定义 `http/gateway/network/timeout/cancelled/contract`；[LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 的 `describeSelectionError` 做安全映射；页面测试验证 raw backend/network detail 不会直出。
- **依据：** 牛客[叠纸前端一面面经](https://www.nowcoder.com/discuss/663163502961438720)包含 AJAX 失败与用户反馈题型；GOV.UK [Error message](https://design-system.service.gov.uk/components/error-message/)要求具体说明发生了什么、如何修复，并避免无帮助的技术术语。

## 21. 如何证明这个页面真的调用后端，而不是“测试都绿了但浏览器仍是假数据”？

- **可直接口述：** 我用分层证据而不是单一单测：组件测试验证文案和可访问状态；Hook 测试用 deferred Promise 验证竞态、取消、重复提交和无自动重试；API adapter 测试验证精确 path/header/no-body、MaxUint64 string 与负面响应；浏览器 E2E 查看 Network，确认点击只产生一次 POST、无 query/body/Idempotency-Key，并分别跑 reward、no_reward、404、未知网络结果、刷新不恢复、窄屏、暗色、键盘与控制台检查；隔离数据库前后 fingerprint 相同，证明本节没有业务写路径。
- **面试官可能追问：** 为什么组件测试不直接证明真实服务？组件测试隔离 UI 逻辑，不能证明 Nginx、路由、数据库和真实浏览器。E2E 能证明组合，但不适合穷举所有边界；各层证据回答不同问题，不能用一个绿色数字替代。
- **常见误区：** mock fetch 测试通过就声称端到端；只看页面出现奖品、不检查 Network；只测 reward happy path；用长期数据库随意写 fixture；看到 Request ID 就断言 Draw 已持久化；把动画帧当随机真实性证据。
- **本项目代码落点：** [LotteryPage.test.tsx](../../../web/src/pages/user/lottery/LotteryPage.test.tsx)、[useEphemeralLotterySelection.test.tsx](../../../web/src/pages/user/lottery/useEphemeralLotterySelection.test.tsx)、[lotteryApi.test.ts](../../../web/src/api/lotteryApi.test.ts)、[httpClient.test.ts](../../../web/src/api/httpClient.test.ts)及第 22 节 QA 共同构成证据链。
- **依据：** 牛客面经普遍从“项目做了什么”追问到“如何证明正确性、如何处理偶现问题”；例如[深信服前端面经](https://www.nowcoder.com/discuss/402568671036456960)继续追问竞态与偶现问题，[小公司面试](https://www.nowcoder.com/discuss/609410185911652352)继续追问方案在真实业务中的有效性。

## 22. 如果下一步升级为正式抽奖，前后端架构要新增什么？

- **可直接口述：** 我不会在现有 ephemeral endpoint 上偷偷改变语义，而会新增正式 Draw 模型。后端需要认证与对象级授权、活动/用户资格、次数或积分账本、配置版本快照、Draw ID、幂等键绑定调用主体与请求摘要、库存/权益发放状态、事务或可靠消息、补偿与审计、限流防刷和结果查询；前端围绕 Draw 状态渲染 pending/selected/fulfilling/fulfilled/failed，而不是在网络未知时重新抽。现有 Hook 可以保留请求取消和陈旧结果抑制思路，但状态机、持久恢复和错误处理都要按正式协议重构。
- **面试官可能追问：** 为什么不直接给当前接口加 localStorage 和 Idempotency-Key？localStorage 只能保存浏览器自述，不能成为服务端最终事实；前端单方面发送 key 也不能让服务端重放同一结果。两者都可能制造比“明确没有”更危险的虚假可靠性。
- **常见误区：** Redis 锁解决所有 exactly-once；有唯一键就不需要业务状态机；MQ 可保证绝不重复；前端保存结果就完成审计；把 selection、Draw 和 fulfillment 压成一个 success 布尔值。
- **本项目代码落点：** 当前 [lotteryApi.ts](../../../web/src/api/lotteryApi.ts) 明确只调用 `ephemeral-selections`，不发送 Idempotency-Key；[LotteryPage.tsx](../../../web/src/pages/user/lottery/LotteryPage.tsx) 明示缺少 Draw、积分、库存、发奖和幂等查询。这些缺口就是未来设计输入，不是已经实现的能力。
- **依据：** 牛客[美团数据系统研发实习一面整理](https://www.nowcoder.com/discuss/722397808745033728)与[多益网络 Java 一面](https://www.nowcoder.com/discuss/353159215873204224)说明接口幂等是后端项目高频追问；[RFC 9110 §9.2.2](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2)提供自动重试的协议边界。

## 23. 为什么把已登录产品改成左侧栏、高密度工作台，而不是继续用顶部导航和营销卡片？

- **可直接口述：** 这不是“侧栏一定更高级”，而是任务形态变了。GrowthOS 已有用户、运营、MCP、Agent 和系统模块，登录后是反复查看活动、积分、优惠券、抽奖与账户的高频任务。顶部导航和 hero 更适合入口少、以介绍和转化为主的页面；工作台更需要稳定当前位置、邻近任务和同屏比较。因此我选分组侧栏、紧凑顶栏、1px 分隔和低装饰内容面，只让搜索、抽屉、通知等覆盖层使用明显 elevation。这个选择优化的是导航和扫描成本，不是为了“像某大厂”。
- **面试官可能追问：** 什么时候会保留顶部导航或 hero？入口少、目标是品牌介绍或首次转化时更合适；若研究发现用户只做单一线性任务，也不应强加后台式侧栏。营销落地页与已登录工作台可以使用不同外壳。
- **常见误区：** 把侧栏当普适答案；为了高密度压缩点击区和层级；用大量小卡片假装数据丰富；所有 Surface 都加重阴影；只看参考截图，不看本项目任务。
- **本项目代码落点：** [WorkspaceShell.tsx](../../../web/src/components/layout/WorkspaceShell.tsx) 统一侧栏、顶栏和内容容器；[UserLayout.tsx](../../../web/src/layouts/UserLayout.tsx) 按主任务、账户和工作空间组织入口；[ProductPage.tsx](../../../web/src/components/common/ProductPage.tsx) 提供紧凑页面语汇。
- **依据：** 这是项目任务驱动的设计判断，W3C 并不规定工作台必须使用侧栏。WAI [Page Structure Tutorials](https://www.w3.org/WAI/tutorials/page-structure/)支持用标题、区域和可预测结构帮助导航；React Router [NavLink](https://reactrouter.com/api/components/NavLink)提供活动导航语义，但不替代信息架构判断。

## 24. 参考 `credit.linux.do` 时，怎样提取 layout tokens 而不是照抄品牌？

- **可直接口述：** 我先拆出与品牌无关的决策：侧栏/折叠宽度、顶栏高度、内容最大宽度、中性色基底、1px border、有限圆角、tabular 数字、内容面低 elevation、overlay 高 elevation，以及摘要—趋势—明细关系；只固化在同视口测量后仍对 GrowthOS 任务有意义的壳层几何，再换成 GrowthOS 自己的主色、信息架构、文案和组件。复制的是组织信息的原则，不是 Logo、账户数据、逐页内容和每个视觉像素。当前只是 CSS variables、Tailwind 与共享组件形成的轻量 token 语汇，没有声称实现 DTCG token 文件流水线。
- **面试官可能追问：** 怎么证明不是换 Logo 复刻？展示 reference/final 对照，并逐项说明因任务不同而改变的部分，例如多工作区入口、ephemeral 合同 rail、Mock snapshot 标签和演示通知边界；相似外壳不能继承参考站的业务声明。
- **常见误区：** 取色量像素后逐页复刻；复制品牌文案或登录数据；看到相似圆角就宣称成熟 design system；把 DTCG Community Group 报告误称 W3C 标准；用“参考大厂”代替用户任务理由。
- **本项目代码落点：** [index.css](../../../web/src/index.css) 定义颜色、字体、radius 与 reduced-motion；[WorkspaceShell.tsx](../../../web/src/components/layout/WorkspaceShell.tsx) 落地壳层几何与 elevation；[ProductPage.tsx](../../../web/src/components/common/ProductPage.tsx) 落地页面 surface 和排版层级。
- **依据：** [Design Tokens Format Module 2025.10](https://www.designtokens.org/tr/2025.10/format/)把 token 描述为跨工具共享设计决策和共同词汇的方法；其页面明确说明这是 Design Tokens Community Group 报告，不是 W3C Standard。本项目只借鉴方法，没有声称遵循该 JSON 格式。

## 25. 嵌套路由、历史 alias 和滚动应该如何处理？为什么不能只写 `pathname.startsWith(path)`？

- **可直接口述：** active 要按路径段边界判断：路径相等或以 `item.path + '/'` 开始，避免 `/campaigns-old` 误亮；历史入口通过显式 `matchPaths` 登记，例如 `/` 映射首页、`/rewards` 映射积分中心，而不是让组件猜。活动链接设置 `aria-current="page"`。pathname 变化时 Shell 关闭抽屉、搜索和通知并让新任务页滚到顶部；我会明确说这是 scroll reset，不是 history scroll restoration，返回长列表仍不会恢复旧位置。
- **面试官可能追问：** 为什么没直接用 `NavLink` 和 `ScrollRestoration`？当前同一配置生成桌面、移动和搜索入口并含 alias，集中 active 函数便于复用；路由继续增长时可迁移到 typed route manifest/NavLink。当前归零符合“进入新任务”的规则，back-navigation 保留位置成为高频需求时再使用恢复机制。
- **常见误区：** 用 `includes` 或无路径段边界的 `startsWith`；多个 link 同时 `aria-current`；alias 不做 active 测试；把 reset 说成 restoration；换路由后旧 overlay 仍覆盖新页。
- **本项目代码落点：** [WorkspaceShell.tsx](../../../web/src/components/layout/WorkspaceShell.tsx) 的 active 函数和 pathname effect；[UserLayout.tsx](../../../web/src/layouts/UserLayout.tsx) 的 `matchPaths`；[UserLayout.test.tsx](../../../web/src/layouts/UserLayout.test.tsx) 覆盖根路径、nested campaign、`/rewards` 与 profile。
- **依据：** React Router [NavLink](https://reactrouter.com/api/components/NavLink)、[useLocation](https://reactrouter.com/api/hooks/useLocation)与 [ScrollRestoration](https://reactrouter.com/api/components/ScrollRestoration)分别支持 active、位置副作用和滚动恢复；MDN [`aria-current`](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Attributes/aria-current)要求只标记集合中的当前项。

## 26. 顶栏搜索为什么是 modal page palette？焦点生命周期怎么设计？

- **可直接口述：** 当前没有业务搜索 API，所以它只检索 navigation/workspace switch 的页面 label/path，我会称 page palette 而不是全站搜索。按钮或 `Cmd/Ctrl + K` 打开后保存触发器，把焦点放进输入框；Tab/Shift+Tab 留在 dialog；Escape、背景点击或选择结果关闭；卸载后下一帧把焦点还给打开它的元素。这样键盘用户能连续完成打开、过滤、跳转或关闭。
- **面试官可能追问：** 为什么下一帧恢复焦点？先让 dialog 从 DOM 卸载，再 focus 触发器，避免与 React 提交竞争。还要处理路由变化后触发器不存在的 fallback。RTL 可验证初始/恢复焦点，但 jsdom 的 `getClientRects()` 不等于真实布局，所以 Tab 循环仍需浏览器键盘验收。
- **常见误区：** 加 `role=dialog` 就认为自动完成焦点管理；焦点留在背景；只处理 Escape；关闭后焦点掉到 body；快捷键 listener 不清理；把本地 filter 说成后端全文搜索。
- **本项目代码落点：** [WorkspaceShell.tsx](../../../web/src/components/layout/WorkspaceShell.tsx) 的 `SearchPalette`、trigger ref、`keepFocusInside`、快捷键 effect 与 `closeSearch`；[UserLayout.test.tsx](../../../web/src/layouts/UserLayout.test.tsx) 覆盖快捷键、过滤、导航和焦点恢复。
- **依据：** WAI-ARIA APG [Modal Dialog Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)给出焦点进入、Tab 循环、Escape 和焦点返回的指导；[Developing a Keyboard Interface](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/)说明可见焦点和可预测键盘行为。APG 指导不能替代真实辅助技术测试。

## 27. 移动抽屉与桌面侧栏为什么不能只是同一 DOM 用 CSS 平移？

- **可直接口述：** 桌面侧栏是持续可见的导航 landmark，移动抽屉是遮挡页面的模态导航，交互语义不同。移动端只在打开时渲染 `role=dialog`、`aria-modal=true` 的 aside；menu 按钮用 `aria-expanded/controls` 关联；打开后锁 body scroll 并把焦点放到关闭按钮，Tab 留在抽屉；Escape、backdrop 或导航项关闭，最后把焦点还给 menu trigger。两端共享 navigation data 和 active 规则，但 CSS 位移本身不会提供 modal 行为。
- **面试官可能追问：** 为什么初始焦点在关闭按钮而不是第一条链接？关闭按钮稳定且允许立即撤销；若将来有重要说明，也可按内容选择静态标题。初始焦点应由任务和阅读顺序决定，不是固定教条。
- **常见误区：** 移出屏幕后仍可 Tab；只遮背景不锁滚动；关闭后不恢复焦点；desktop collapse 与 mobile modal 混成一个含混 state；只测点击，不测 Escape、Shift+Tab 和 route change。
- **本项目代码落点：** [WorkspaceShell.tsx](../../../web/src/components/layout/WorkspaceShell.tsx) 的条件渲染、body overflow cleanup、close/menu refs、dialog keydown 与 backdrop；[UserLayout.test.tsx](../../../web/src/layouts/UserLayout.test.tsx) 覆盖展开、链接、Escape、scroll lock 和焦点恢复。
- **依据：** WAI-ARIA APG [Modal Dialog Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)给出 modal focus lifecycle；MDN [`aria-expanded`](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Attributes/aria-expanded)说明触发器与受控元素的关系；WCAG 2.2 [Reflow](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html)支持窄视口重组导航。

## 28. 为什么通知红点和“标记已读”必须明确写成演示状态？为什么面板不是 modal？

- **可直接口述：** 当前没有通知 API、SSE/WebSocket、持久未读数或跨设备同步，所以红点和“已读”只是本地 React state。面板明确写“仅为本地界面样例，不读取服务端”，按钮叫“标记样例已读”，刷新恢复，避免把视觉完整度误认成后端能力。它是依附铃铛的非模态 region：不要求用户先作决定，因此不锁滚动、不 trap focus；触发器用 `aria-expanded/controls` 表达开关，并支持 Escape 和外部点击关闭。
- **面试官可能追问：** 接真实通知先补什么？先定义消息 ID、游标/分页、已读写入幂等语义和权限，再选择 polling、SSE 或 WebSocket，并处理乐观已读失败、跨标签同步、重连和未读对账。不能只把 `setNotificationRead(true)` 换成 fetch 就声称可靠通知系统。
- **常见误区：** 只在 README 标演示，页面仍误导；红点颜色是唯一未读信息；非模态 panel 却设 `aria-modal`；关闭等同已读；UI 先报成功而写入失败无恢复；虚构实时推送。
- **本项目代码落点：** [WorkspaceShell.tsx](../../../web/src/components/layout/WorkspaceShell.tsx) 的 notification state、演示文案、region、outside-pointer cleanup 与铃铛名称；[UserLayout.test.tsx](../../../web/src/layouts/UserLayout.test.tsx) 断言演示边界和已读样例。
- **依据：** React [Choosing the State Structure](https://react.dev/learn/choosing-the-state-structure)支持最小且一致的本地 state；MDN [`aria-expanded`](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Attributes/aria-expanded)支持触发器表达 panel 状态。是否 modal 应由交互阻塞性决定。

## 29. 为什么演示页面共享一个 Mock snapshot，并从原始列表派生摘要？

- **可直接口述：** Mock 最大的风险是同一实体出现多个矛盾版本。若首页、积分、优惠券和个人中心分别写余额、数量和用户，修改一处就会漂移。因此用户、活动、交易、优惠券集中在 `growthOsMockData.ts`，页面显示同一 snapshot 标签；可用券数、收入/支出和筛选结果从数组派生，不另存 count/state。真正需要 state 的只是 tab、复制反馈等交互。这样保持演示确定性，也明确这些不是实时账务。
- **面试官可能追问：** 数据大时还每次 render 过滤吗？先测性能；小 snapshot 直接派生最清晰。计算昂贵且引用稳定时可 `useMemo`，但 memo 是性能优化，不是第二事实源。真实 API 也应从同一 response/cache entry 派生，或由后端返回带一致性版本的 summary 和 rows。
- **常见误区：** 每页复制一份 Mock；把 derived count 放 state 再用 Effect 同步；随机数字导致测试不稳定；真实 API 失败静默 fallback Mock；统一 snapshot 却不标来源；把前端 tab filter 说成服务端查询。
- **本项目代码落点：** [growthOsMockData.ts](../../../web/src/mocks/growthOsMockData.ts) 是统一 snapshot；[UserHomePage.tsx](../../../web/src/pages/user/home/UserHomePage.tsx)、[PointsPage.tsx](../../../web/src/pages/user/points/PointsPage.tsx)、[CouponsPage.tsx](../../../web/src/pages/user/coupons/CouponsPage.tsx)与 [UserProfilePage.tsx](../../../web/src/pages/user/profile/UserProfilePage.tsx)复用并派生视图。
- **依据：** React [Choosing the State Structure](https://react.dev/learn/choosing-the-state-structure)建议避免矛盾、冗余和重复 state，并在 render 中从现有数据计算可推导信息。

## 30. 多工作区菜单是否已经实现 RBAC？前端隐藏菜单和后端授权有什么区别？

- **可直接口述：** 没有。当前 Shell 展示用户、运营、MCP、Agent 等入口，只证明信息架构和路由可达性，不证明角色权限。前端隐藏菜单只能改善体验，不能成为安全边界：用户仍可能直接输入 URL 或调用 API。正式 RBAC 需要服务端认证、资源/动作级授权和默认拒绝；前端再基于同一权限模型过滤导航、处理 403 和避免展示无权数据。当前 `mockUser.role` 与 `activeRole` 只是演示状态，不能写成已完成 RBAC。
- **面试官可能追问：** 何时在前端过滤菜单？服务端提供可信 session/claims 或 capabilities 后再派生可见入口；但每个 API 仍独立授权。还要区分“不可见”“只读”“可执行”，处理权限变化、缓存失效和直接深链访问，避免只按角色名写散落的 if。
- **常见误区：** 菜单不显示就等于接口安全；把 JWT 中 role 当唯一授权事实而服务端不校验；403 当 404 随意处理；前端和后端各维护一套漂移矩阵；因为有四个 workspace switch 就在简历写 RBAC。
- **本项目代码落点：** [WorkspaceShell.tsx](../../../web/src/components/layout/WorkspaceShell.tsx) 当前只消费静态 navigation；[UserLayout.tsx](../../../web/src/layouts/UserLayout.tsx) 配置所有工作区入口；[appStore.ts](../../../web/src/stores/appStore.ts) 的 role/mock user 是本地演示。代码没有权限 guard 或服务端 capability contract，这正是结论依据。
- **依据：** OWASP [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)要求默认拒绝、每个请求校验权限，并指出客户端检查不能决定服务器端访问；本题不是牛客题库复刻，而是本项目多工作区设计必然引出的安全追问。

## 31. 静态图表如何做无障碍？为什么关闭动画、隐藏漏斗 SVG 仍不等于完成？

- **可直接口述：** 图表不能成为唯一事实来源。用户首页趋势图关闭 Recharts 入场动画，并用文字图例、积分摘要和账单补信息；运营漏斗已有每阶段名称、数值和转化率文本，所以重复图形 `aria-hidden`，避免读屏重复或读出无意义 SVG。全局 CSS 尊重 `prefers-reduced-motion`。但当前趋势图没有逐日数据表，Tooltip 也偏指针交互；如果必须读取精确点位做决策，还要补等价表格、键盘交互、单位和时间范围，不能声称已经完成完整可访问分析图表。
- **面试官可能追问：** 为什么不只给 SVG 一个 `aria-label`？“近七日趋势图”只说明图是什么，不能替代数据关系；复杂图需要短说明，必要时提供详细文本或表格。反过来，图形若重复已有文本，隐藏比重复朗读更清晰。
- **常见误区：** 只靠颜色区分；Tooltip 是唯一精确值；一句 `aria-label` 就宣称 WCAG 完成；所有 SVG 都隐藏导致唯一信息消失；关闭动画后也删掉 loading/status；不测试 reduced-motion。
- **本项目代码落点：** [UserHomePage.tsx](../../../web/src/pages/user/home/UserHomePage.tsx) 的静态 AreaChart、文字图例与摘要；[GrowthOSGraphics.tsx](../../../web/src/components/common/GrowthOSGraphics.tsx) 在文本阶段统计之后隐藏重复 funnel 并关闭 animation；[index.css](../../../web/src/index.css) 的 reduced-motion media query。
- **依据：** WCAG 2.2 [Understanding Non-text Content](https://www.w3.org/WAI/WCAG22/Understanding/non-text-content.html)说明图表需要等价文本；[Understanding Animation from Interactions](https://www.w3.org/WAI/WCAG22/Understanding/animation-from-interactions.html)与 MDN [`prefers-reduced-motion`](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/%40media/prefers-reduced-motion)支持减少非必要运动，但不会自动解决数据可达性。

## 面试现场的 60 秒项目陈述

> 第 22 节我把原来的前端 Mock 抽奖页改造成了真实服务端临时选择页，但刻意没有把它包装成正式抽奖。页面只接受规范 uint64 字符串 ID，通过无 body、无 query、无自动重试的同源 POST 调用 GrowthOS-Go，并对返回 JSON 做运行时契约校验。请求状态用 `idle/selecting/success/error` 判别联合建模，自定义 Hook 同时使用 AbortController 和 generation token，分别处理资源取消与陈旧响应竞态；pending 期间 UI 与 Hook 双层防重复，但我明确说明这不等于服务端幂等。网络未知时不自动重试，`no_reward` 是正常成功，`reward` 也只显示“选中了奖励候选”，因为当前没有 Draw、积分、库存和发奖事实。页面同时覆盖 label、live region、错误播报、响应式、暗色与 reduced-motion 边界，并用单测、Hook deferred 测试、精确 transport 测试和真实浏览器 Network 验收分层证明。

## 来源清单（访问日期：2026-08-30）

### A. 面经题型触发：牛客用户内容，不作为技术规范

- [深信服前端一二三 HR 面经，已经凉了](https://www.nowcoder.com/discuss/402568671036456960)：请求竞态、防抖、abort 具体实现、偶现问题。
- [快手 前端](https://www.nowcoder.com/discuss/541988931953287168)：`Promise.race`、AbortController、Hooks 与 HTTP。
- [字节跳动－国际化商业产品与技术－前端开发实习生一面凉经](https://www.nowcoder.com/discuss/646401290917924864)：状态库选型、自定义 Hook 与业务场景。
- [腾讯会议 前端一面（3.21）](https://www.nowcoder.com/discuss/600463712922669056)：自定义 Hook、表单输入与反馈组件。
- [字节－本地服务－前端 面经](https://www.nowcoder.com/discuss/745084418779287552)：响应式、模块化与项目难点。
- [网易传媒一面（70min）](https://www.nowcoder.com/discuss/370979597536579584/)：响应式、Flex 与 Effect。
- [叠纸前端一面面经](https://www.nowcoder.com/discuss/663163502961438720)：Web Accessibility、AJAX 失败反馈。该页面更接近题目整理，引用时仅作补充触发证据。
- [字节跳动前端 3+1 面经](https://www.nowcoder.com/discuss/353156819747020800)：搜索组件的防抖与请求竞态。
- [2025-2-19 Java 面试题（美团、快手）](https://www.nowcoder.com/discuss/722397808745033728)：其中包含美团数据系统研发实习一面的接口幂等追问。
- [多益网络软件开发 Java 岗一面](https://www.nowcoder.com/discuss/353159215873204224)：接口幂等与项目深挖。
- [小公司面试](https://www.nowcoder.com/discuss/609410185911652352)：面试官从泛化幂等回答追问业务定义、限流与实际效果。

### B. React 官方资料

- [React：Reacting to Input with State](https://react.dev/learn/reacting-to-input-with-state)：枚举视觉状态、事件与网络响应驱动转换。
- [React：Choosing the State Structure](https://react.dev/learn/choosing-the-state-structure)：避免矛盾、冗余和不可能状态。
- [React：Extracting State Logic into a Reducer](https://react.dev/learn/extracting-state-logic-into-a-reducer)：`useState` 与 reducer 的取舍、可测试性。
- [React：Reusing Logic with Custom Hooks](https://react.dev/learn/reusing-logic-with-custom-hooks)：共享有状态逻辑，不共享状态本身。
- [React：useEffect](https://react.dev/reference/react/useEffect)：cleanup、依赖与忽略乱序响应。
- [React：Synchronizing with Effects](https://react.dev/learn/synchronizing-with-effects)：setup/cleanup 对称，请求应 abort 或 ignore。
- [React：You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect)：交互事件与 Effect 边界、fetch 竞态示例。
- [React Router：NavLink](https://reactrouter.com/api/components/NavLink)：活动链接与 `aria-current`。
- [React Router：useLocation](https://reactrouter.com/api/hooks/useLocation)：位置变化后的界面副作用。
- [React Router：ScrollRestoration](https://reactrouter.com/api/components/ScrollRestoration)：浏览器式滚动恢复，用于界定当前手工 scroll reset 的限制。

### C. Web、HTTP 与 JavaScript 官方资料

- [MDN：AbortController](https://developer.mozilla.org/en-US/docs/Web/API/AbortController)。
- [MDN：AbortSignal](https://developer.mozilla.org/en-US/docs/Web/API/AbortSignal)。
- [MDN：Fetch API](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API)。
- [RFC 9110 §9.2.2：Idempotent Methods](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.2.2)。
- [MDN：POST request method](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Methods/POST)。
- [MDN：Idempotent](https://developer.mozilla.org/en-US/docs/Glossary/Idempotent)。
- [MDN：Number.MAX_SAFE_INTEGER](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Number/MAX_SAFE_INTEGER)。
- [MDN：Responsive web design](https://developer.mozilla.org/en-US/docs/Learn_web_development/Core/CSS_layout/Responsive_Design)。
- [MDN：prefers-reduced-motion](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/%40media/prefers-reduced-motion)。
- [MDN：prefers-color-scheme](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/%40media/prefers-color-scheme)。
- [MDN：color-scheme](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/color-scheme)。
- [MDN：aria-current](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Attributes/aria-current)。
- [MDN：aria-expanded](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Attributes/aria-expanded)。

### D. W3C/WAI、内容设计与 Design Token 资料

- [WAI：Labeling Controls](https://www.w3.org/WAI/tutorials/forms/labels/)。
- [WAI：Page Structure Tutorials](https://www.w3.org/WAI/tutorials/page-structure/)。
- [WAI-ARIA APG：Button Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/button/)。
- [WAI-ARIA APG：Modal Dialog Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)。
- [WAI-ARIA APG：Developing a Keyboard Interface](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/)。
- [WCAG：Understanding Status Messages](https://www.w3.org/WAI/WCAG21/Understanding/status-messages)。
- [WCAG 2.2：Understanding Reflow](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html)。
- [WCAG 2.2：Understanding Non-text Content](https://www.w3.org/WAI/WCAG22/Understanding/non-text-content.html)。
- [WCAG 2.2：Understanding Animation from Interactions](https://www.w3.org/WAI/WCAG22/Understanding/animation-from-interactions.html)。
- [Design Tokens Format Module 2025.10](https://www.designtokens.org/tr/2025.10/format/)：Design Tokens Community Group 报告，不是 W3C Standard；本文只引用其共享设计决策的方法。
- [OWASP：Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)：默认拒绝、逐请求授权和客户端检查的边界。
- [GOV.UK Design System：Confirmation pages](https://design-system.service.gov.uk/patterns/confirmation-pages/)。
- [GOV.UK Design System：Check answers](https://design-system.service.gov.uk/patterns/check-answers/)。
- [GOV.UK Design System：Error message](https://design-system.service.gov.uk/components/error-message/)。
