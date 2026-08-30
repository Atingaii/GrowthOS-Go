# Participation 前置资格链基线 v1

- **状态：** 第 26 节已批准实现基线
- **日期：** 2026-08-30
- **适用范围：** Participation domain/application 内核
- **不适用范围：** 身份认证、访问授权、Activity、Lottery 路由、次数扣减、库存、公开 API 与前端

## 1. 为什么现在需要第二条规则

第 25 节只回答“权威注册事实是否满足新用户 cutoff”。一个 evaluator 不足以证明共同接口、顺序和短路语义，提前创建 `RuleEngine` 只会让技术形状先于业务事实。

第 23 节已经登记另一条真实 Participation 前置条件：风险事实提供方拥有最小风险 verdict，Participation 拥有“该 verdict 是否允许进入本场景”的准入决定。因此第 26 节新增风险准入规则，并只抽取两条具体规则已经共同证明的线性组合语义。

不选择“剩余次数大于零”，因为次数是会被并发消费的状态；读取一个 `remaining > 0` 布尔快照会制造检查后再使用的竞态，真正账户、流水、事务或预占安排在第 39～45 节。不选择 Activity 门控，因为 Activity 生命周期和版本直到第 30 节才建立。不选择权限，因为认证与公共访问控制按第 31～35 节独立演进。

## 2. 本节业务命题

对同一 `ParticipantRef` 和同一规则集 revision，一次前置资格评估必须按以下顺序执行：

```text
一次受控 evaluated-at
  -> 新用户资格
       ineligible / error / cancel -> 停止，不访问风险事实
       eligible                    -> 风险准入
                                          blocked -> ineligible
                                          passed  -> eligible
                                          unknown / error / cancel -> 无最终决定
```

只有两条规则都形成确定 `eligible`，组合结果才是最终 `eligible`。任何单节点的 `eligible` 只表示“继续”，不能提前冒充最终资格。

## 3. 事实、决定与所有权

| 概念 | 权威所有者 | Participation 可以做什么 | Participation 不能做什么 |
| --- | --- | --- | --- |
| 注册时间快照 | 外部用户目录 | 验证来源、revision、主体、future/freshness，并应用新用户 cutoff | 修改用户生命周期或相信客户端 `is_new_user` |
| 风险评估快照 | 受控风险事实提供方 | 消费最小 `passed/blocked` verdict，并映射为本场景准入 | 复制设备指纹、模型特征、阈值或宣布外部风险事实 |
| 新用户资格决定 | Participation | 形成固定 rule/reason 和 policy revision | 把事实未知写成业务拒绝 |
| 风险准入决定 | Participation | `passed -> eligible`，`blocked -> ineligible` | 把超时、缺失、过期或损坏默认放行 |
| 最终前置资格 | Participation | 仅在全部必经 gate 都确定通过后形成 | 表示已经授权、扣次数、抽奖、扣库存或发奖 |

`ParticipantRef` 仍只是外部主体的内部查找引用，不是 Principal、登录证明、租户、角色或授权证据。

## 4. 风险事实最小契约

风险快照只包含：

- 非零 `ParticipantRef`；
- 枚举 verdict：`passed` 或 `blocked`；
- 风险提供方产生该 verdict 的 `assessed_at`；
- 受控 `source` 与 source `revision`；
- 不包含用户资料、设备指纹、模型特征、规则阈值、分数或用户文案。

freshness 从权威 `assessed_at` 计算。adapter 不得在每次读取时用本地 `time.Now()` 给旧 verdict 重新盖章。恰好等于最大年龄仍有效，超过一纳秒即过期。

零值、未知 verdict、不同主体、`assessed_at` 晚于逻辑 evaluated-at、过期、not found、provider unavailable 和未分类读取失败都不能形成业务决定。

## 5. 一次逻辑评估时刻

组合器在任何事实读取前从受控服务端 `Clock` 读取一次 canonical UTC evaluated-at。两条规则都使用这一时刻：

- 不信任浏览器或请求传入时间；
- 不让每个节点各读一次“现在”，避免边界附近相同请求得到不可解释的混合结果；
- 不并行预取风险事实，保留前序拒绝后的隐私、成本与故障隔离收益；
- source 返回的注册 `observed_at` 或风险 `assessed_at` 晚于该时刻时，本次评估失败关闭；as-of 后产生的新快照留给下一次评估。

这是逻辑决策基准，不是请求完成时间。当前端口尚无生产 adapter；未来 adapter 必须证明能返回在 evaluated-at 时已经存在的不可变快照，或在无法满足时返回明确技术失败，不能偷偷选择未来事实。

第 25 节独立 `NewUserEligibilityService.Evaluate` 的既有契约保持不变：它仍在成功读取注册事实后读取一次 clock。第 26 节组合路径使用包内受控的 evaluation-instant token，不能让 HTTP 或任意调用者提交旧时间绕过 freshness。

## 6. 顺序为何是版本化业务契约

固定顺序为：

1. `participation.new_user.registered_on_or_after`；
2. `participation.risk.screening_admission`。

注册事实通常低敏、读取成本较低；风险源可能更昂贵、更敏感，也可能具有独立故障域。先筛掉确定的非新用户能够：

- 不为已确定拒绝的主体继续访问风险 authority；
- 固定普通业务拒绝的首要 reason；
- 减少风险依赖流量和暴露面；
- 让测试能够证明 tail reader 零调用，而不只是断言最后一个 bool。

顺序与规则集 revision 绑定。未来调序不是无害重构，必须新增 revision、回归证据与披露评审。

## 7. 结果与终止语义

| 节点结果 | 组合动作 | 最终业务决定 | error |
| --- | --- | --- | --- |
| `eligible` | 还有节点则继续；全部完成才最终通过 | 全部完成后 `eligible` | `nil` |
| `ineligible` | 立即短路 | `ineligible`，沿用终止节点稳定 reason | `nil` |
| 事实/配置/clock/依赖技术失败 | 立即短路 | 零值，无可信最终决定 | 单一稳定 typed error |
| caller cancel/deadline | 立即短路 | 零值 | 原始 context error 可由 `errors.Is` 识别 |

“fail closed”只表示下游不能继续，不表示把技术失败伪装成 `ineligible`。不使用 `bool` 同时承载继续、拒绝、未知、错误和取消。

## 8. 最小执行轨迹

确定结果携带有序、只读、可复制的 executed-step trace。每一步只记录：

- rule code；
- `eligible/ineligible`；
- stable reason code；
- policy revision；
- fact source 与 fact revision；
- 与整条链相同的 evaluated-at。

trace 只包含实际执行的节点，不伪造未执行节点为“成功”或“跳过”。普通指标只允许低基数 rule/outcome/reason/error class；`ParticipantRef`、fact revision、原始 error、风险特征和用户文案不得进入 metric label。技术失败返回零最终决定；失败节点和错误类别由安全 typed error/后续受控 observer 诊断，不把半成品业务决定交给调用方猜测。

## 9. 组合器不是什么

本节实现是 Participation 专用的 ordered eligibility gate chain，也是责任链模式的一种受限变体。经典责任链常让多个候选处理器“直到某一个处理请求”；这里每个节点都是必经 gate，只有 continue 才进入下一项。

它不是：

- 任意 `Rule` 平台、规则树或图；
- `map[string]any` / JSON DSL / 脚本；
- 数据库可配置执行器；
- priority 排序或运行时插件；
- XACML、OPA、DMN 或第三方规则引擎；
- Authorization、Inventory、Activity 与 Eligibility 共用的万能接口；
- middleware、Saga、事务或补偿工作流。

链内节点必须只读。若节点开始创建订单、扣次数、选择 Award、扣库存或发消息，中途失败就需要事务、幂等、补偿和结果恢复；这些副作用不能借“责任链”名称混入本节。

## 10. 验收矩阵

### 10.1 领域规则

- `passed` 形成风险规则 `eligible`；
- `blocked` 形成确定 `ineligible`；
- unknown/zero/future/stale/mismatched snapshot 不形成决定；
- policy、rule、fact 和 ruleset revision 概念分离；
- 风险决定不携带 ParticipantRef、阈值或原始特征。

### 10.2 组合与短路

- 新用户拒绝时 risk reader 零调用；
- 新用户错误/取消时 risk reader 零调用；
- 新用户通过、风险通过时得到两步最终 eligible；
- 新用户通过、风险阻断时得到两步最终 ineligible；
- 风险事实缺失、过期、future、损坏或依赖失败时最终决定为零；
- pre-cancel、clock 后取消、每个 reader 返回后取消都让 caller context 优先；
- chain clock 每次最多且恰好调用一次，两个 trace step 的 evaluated-at 完全相同；
- 两条规则的先后顺序和 trace 长度确定；
- concurrent Evaluate 没有共享请求态竞态，`go test -race` 通过。

### 10.3 架构停止线

- Participation 生产代码不 import Lottery、Gin、SQL、Redis、React 或其他项目上下文；
- 不新增 Migration、route、配置、Compose 服务、Redis key 或 UI；
- 不声明通用 `RuleEngine`、`RuleTree`、`EvaluationContext`、DSL、priority、泛型规则节点；
- Lottery selector、现有 ephemeral API 与 React 页面相对第 25 节零变化。

## 11. 剩余风险与后续触发条件

1. 当前没有真实注册/风险 adapter，端口与测试不能冒充外部系统已经联通；
2. 两次顺序读取不是原子跨系统快照，单一 evaluated-at 只给出可重放的逻辑 as-of，不消除跨 authority 的 TOCTOU；
3. fact revision 仍由提供方定义，未来 adapter 必须限制其内容和基数；
4. 当前 trace 是进程内决定证据，不是 OpenTelemetry span，也没有持久化审计；
5. 规则集尚未绑定 Activity 或正式 Participation/Draw 身份；
6. 第 27 节只有在会员分层引入多出口路由、缺省分支、共享子路径或分支合流后，才有证据说明线性链不够；不得先在 handler 内写隐式 `goto` 把图伪装成链。

## 12. 参考基线

- [Lottery 业务规则需求基线 v1](lottery-rule-requirements-v1.md)
- [新用户资格规则基线 v1](new-user-eligibility-v1.md)
- [Go Code Review Comments：Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)
- [Go context package](https://pkg.go.dev/context)
- [OASIS XACML 3.0 Standard](https://docs.oasis-open.org/xacml/3.0/xacml-3.0-core-spec-en.html)
- [OPA Policy Language](https://www.openpolicyagent.org/docs/policy-language)

