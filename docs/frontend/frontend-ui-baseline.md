# 前端 UI 设计基线

## 设计来源

业务工作台继续复用 `实现指定设计风格.zip` 中已经完成的 GrowthOS 设计。第 22 节当时的工作台视觉验收已归档为[历史设计 QA](../design-thinking/evidence/lesson-22-design-qa.md)，不得用当前根文件覆盖其时间切片。第 32 节公共认证界面另以 [`credit.linux.do`](https://credit.linux.do/) 的公开首屏作为视觉语言参考：安静白底、受控紫色强调、宽松双栏、一个高优先级焦点卡片和清晰的大标题层级。实现不复制对方品牌、文案、账户数据或装饰资产，而是把同一构图原则用于真实登录/当前会话任务；同视口对照与交互结论记录在仓库根[当前设计 QA](../../design-qa.md)。

## 视觉原则

- sans 栈以本机可用的 Inter 为首选，并回退到系统 UI 与中日韩字体；monospace 栈以 SFMono 为首选并回退到系统等宽字体。仓库没有远程字体 import，因此不能承诺客户端一定加载某个商业字体；
- 浅色界面以 white/zinc 中性色为基底，`#625df5` 紫色作为受控主色；暗色 token 使用更亮的同色系强调，状态色保持独立语义；
- 通过留白、细边框、适度圆角和轻阴影建立层次；
- 用户端保持轻量触达感，运营端强调信息密度和状态清晰度；
- MCP 和 Agent 工作台保留深色科技感，但不牺牲可读性；
- 状态同时使用文字、图标和颜色表达；
- 页面必须支持桌面端与窄屏布局。

认证页补充以下约束：

- 桌面使用 narrative + task card 双栏，移动端保持 narrative 在表单之前的 DOM、视觉与键盘顺序；
- 登录、checking、authenticated、unavailable 和 signed-out 必须具有不同文案与语义，技术故障不能伪装成未登录；
- 只显示 Session Principal 与到期边界，不提前显示 Role、Scope、Permission 或可访问工作台；
- 登录是单一主操作；当前会话中的“重新核查”和“退出”必须体现不同风险与承诺强度；
- focus、status/alert、44px 触控高度和 reduced-motion 与颜色同时参与状态表达。

## 复用边界

公共图形、状态徽标、指标卡、布局和业务卡片位于 `web/src/components` 与 `web/src/layouts`。工作台使用 `WorkspaceShell`，公共认证使用 `AuthLayout`，两者共享品牌语言但不互相伪装权限边界。Mock 数据集中位于 `web/src/mocks`，真实 Session 页面不得读取这些快照形成身份，也不得把 session/CSRF 写入持久浏览器 store。
