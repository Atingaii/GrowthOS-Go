# 领域事件地图 v1

**状态：** v1 分析基线；第 32 节 Identity 认证事实已验收登记，集成事件发布仍未决定

**更新日期：** 2026-09-03

**来源章节：** [第 5 节：事件风暴第一次领域分析](../course/part-01/lesson-05-first-event-storm.md)；第 30 节以[Activity Publication 绑定基线](activity-publication-binding-v1.md)验收 Activity 生命周期、exact publication、回滚与 resolve 语义；第 31 节冻结 Governance Policy 求值停止线；第 32 节以[真实 Session 认证基线](identity-session-authentication-v1.md)登记 workforce credential 与 Session 生命周期事实。

## 1. 这份地图解决什么问题

第 1～4 节分别描述了用户增长旅程、人工运营工作流和 AI Operator 工作流。本节把这些流程放到同一张事件风暴地图上，区分“要求系统做什么”和“业务事实已经发生了什么”。

这不是最终领域模型、数据库设计或消息主题清单。它只记录当前已知的业务事实和因果关系，为第 6 节第一次划分限界上下文提供输入。

## 2. 事件风暴图例

| 类型 | 含义 | 示例 |
| --- | --- | --- |
| 命令 | 用户、运营或系统要求执行动作 | 创建活动、提交审批、参与活动 |
| 领域事件 | 业务已经确认发生的事实，使用过去时描述 | 活动已发布、权益已到账 |
| 查询 | 只读取信息，不改变业务事实 | 查询人群规模、查询活动指标 |
| 策略 | 事件发生后决定下一步动作的规则 | 审批通过后发布、预算超限后暂停 |
| 外部系统 | 当前平台边界外的参与者 | LLM、支付系统、券中心、消息平台 |
| 失败/补偿 | 业务失败、结果未知或对已发生事实进行恢复 | 权益发放失败、补偿任务已创建 |
| 审计事件 | 需要追责但不等同于业务事实的记录 | 审批已拒绝、人工已接管任务 |

## 3. 主场景事件链

主场景沿用前几节的召回活动：面向连续 14 天未登录、近 90 天有消费的会员，通过 Feed 触达，发放 10 元优惠券，总预算不超过 2 万元，运行 7 天。

```text
运营目标已提出
  ↓
活动草稿已创建
  ↓
人群估算已完成
  ↓
活动配置已校验
  ↓
活动预览已生成
  ↓
活动已提交审批
  ↓
活动审批已通过
  ↓
活动版本已发布
  ↓
活动已进入运行
  ↓
Feed 内容已曝光 → 用户已点击活动
                          ↓
                   用户资格已检查
                          ↓
                   用户已提交参与
                          ↓
                    抽奖/奖励已完成
                          ↓
                    权益发放已开始
                          ↓
                     权益已到账
                          ↓
                  用户已领取/使用权益
                          ↓
                    业务转化已发生
```

其中“事件已发生”必须由确定性的业务模块确认。页面提示、模型文字或数据库写入本身都不能替代业务事实。

## 4. 运营与活动生命周期

### 4.1 命令、查询和事件

| 参与者 | 命令 | 查询 | 产生的领域事件 |
| --- | --- | --- | --- |
| 运营人员 | 创建活动草稿 | 查询活动模板、历史活动 | 活动草稿已创建 |
| 运营人员/AI | 估算目标人群 | 查询人群规模、数据时间 | 人群估算已完成 |
| 运营人员/AI | 校验活动配置 | 查询预算、权益库存、规则冲突 | 活动配置已校验或活动配置校验失败 |
| 运营人员 | 生成预览 | 查询预览受众和展示内容 | 活动预览已生成 |
| 运营人员/AI | 提交审批 | 查询审批人和审批策略 | 活动已提交审批 |
| 审批人 | 通过或驳回审批 | 查询审批快照和差异 | 活动审批已通过或活动审批已驳回 |
| 运营人员 | 发布活动版本 | 查询发布前检查结果 | 活动版本已发布 |
| 调度器 | 启动到达开始时间的活动 | 查询活动状态和时间窗 | 活动已进入运行 |
| 运营人员 | 暂停、恢复或提前结束活动 | 查询运行指标和止损原因 | 活动已暂停、活动已恢复或活动已提前结束 |
| 运营人员/系统 | 生成复盘 | 查询事件、转化和成本数据 | 活动复盘已生成 |

“点击发布按钮”是用户动作或命令；只有发布校验通过并且版本状态切换成功后，才产生“活动版本已发布”。

### 4.2 第 30 节当前实现语义与事件停止线

第 30 节已经通过源码与真实 MySQL 验收以下 Marketing 聚合内的确定状态转换，但尚未装配 Outbox、消息发布器、HTTP/UI 或审计流水，因此这里的“已创建/已发布/已回滚/已退役”是领域语义，不是已经对外可靠投递的集成事件：

| 命令语义 | 聚合内确认条件 | 可形成的领域事实语义 | 当前未声称 |
| --- | --- | --- | --- |
| CreateDraft | Activity identity 合法且 Repository 创建成功 | Activity 草稿已创建 | 第 30 节当时没有操作者会话、授权、HTTP 或审计事件 |
| Publish | 当前状态/version 匹配；exact Lottery graph + terminal Strategy snapshot 闭合集通过；exact candidate 的审批证据通过；CAS 成功 | Activity publication version 已追加并成为 active | 没有真实 Governance provider、调度器或线上运行链 |
| Rollback | 目标历史 publication 尚未结束；复制其 exact 内容并重新校验/审批；CAS 成功 | Activity 已通过一个新 publication version 回滚，且记录 `rollback_of` | 不原地改写旧版本，也不宣称已撤销历史 Draw/权益 |
| Retire | exact current + 审批 + CAS 成功 | Activity 已退役且保留最后 active publication history | 不等于删除，不触发库存/权益补偿 |
| Resolve | 一次 Clock 下状态与 `[start,end)` gate 通过 | 当前调用获得 exact active publication | 这是查询/门控结果，不是“活动已进入运行”事件 |

并发冲突、审批失败、Lottery 依赖失败、exact ref 缺失、terminal manifest 不闭合或时间窗不满足，都不能产生成功事实。`state_version` CAS 只是防止丢失更新的确认条件；它不替代 Governance 审批、访问授权、Outbox 或跨系统幂等。

第 32 节虽新增并验收了真实 Session 认证，但没有把 trusted Principal 接入上述 Marketing command handler；因此也没有追溯性地改变第 30 节证据。业务写操作仍需第 33 节先加载 exact Resource/Policy 并强制授权。

### 4.3 Identity 认证命令、事实与停止线

认证链的目标是把不可信 credential/bearer 解析成最小 trusted human Principal，不是替业务域宣布“已授权”。当前同一路径提供三种语义：

| 请求语义 | 服务端确认条件 | 可记录的安全/领域事实语义 | 不能据此宣称 |
| --- | --- | --- | --- |
| 创建 Session | exact Origin/JSON 通过；双维 throttle reservation 成功；workforce account 与 Argon2id credential 有效；Session 事务确认 | workforce Session 已签发 | 不能表示任一业务 Action 已获授权，也不能记录 raw password/token |
| 查询当前 Session | Cookie token digest 命中；account enabled、authentication epoch、revoke、idle/absolute expiry 全部有效；必要 touch 确认 | 当前请求已恢复 trusted human Principal；这是查询结果，不必产生新领域事件 | 不能把前端缓存、Redis 或客户端 Role 字段当权威 |
| 撤销 Session | exact Origin、Cookie 与 session-bound CSRF 有效；revoke 事务结果确定 | workforce Session 已撤销 | commit outcome unknown 时不能宣布“已退出”，也不能反向撤销已开始的业务副作用 |
| 认证失败或被 throttle | credential 失败、持久 admission policy 拒绝或依赖不可用 | 可形成低披露安全结果/指标 | 不区分并外泄 unknown/disabled/wrong-password；不把技术故障记成坏密码 |

`identity_workforce_account`、`identity_session` 与 `identity_authentication_throttle` 是 Identity 的当前权威状态；源码 Migration latest 为 14。第 32 节没有新增 Outbox、broker topic 或公共认证事件 contract，也没有把 Session 状态写入 Redis。若后续确需向审计、安全响应或外部 IAM 发布事件，必须另行定义最小载荷、去重、顺序、隐私保留和“事务确认 → 可靠发布”两个可观察阶段。

### 4.4 失败和补偿

```text
活动配置校验失败
  ├─ 预算不足
  ├─ 权益库存不足
  ├─ 人群条件不完整
  └─ 规则存在冲突

活动审批已驳回
  └─ 运营修改草稿后重新提交

活动运行异常
  └─ 预授权止损策略触发 → 活动已暂停 → 运营复核

活动已提前结束
  └─ 已发放权益不被“反向抹除”，未执行步骤进入取消或补偿流程
```

## 5. 用户参与与权益链路

| 顺序 | 命令/动作 | 确认后的领域事件 | 失败或补偿事件 |
| --- | --- | --- | --- |
| 1 | 打开 Feed | Feed 内容已曝光 | 曝光采集失败 |
| 2 | 点击活动 | 用户已点击活动 | 点击事件重复 |
| 3 | 请求参与 | 用户资格已检查 | 用户资格检查失败 |
| 4 | 提交参与 | 用户已提交参与 | 参与请求已重复、活动已关闭 |
| 5 | 执行抽奖或奖励选择 | 抽奖已完成、奖品已选定 | 抽奖执行失败、结果未知 |
| 6 | 发放权益 | 权益发放已开始 | 权益发放失败 |
| 7 | 完成到账 | 权益已到账 | 权益补偿已创建 |
| 8 | 用户使用 | 优惠券已使用或积分已消耗 | 核销失败、使用已撤销 |
| 9 | 业务结果确认 | 业务转化已发生 | 转化回传失败、归因结果待确认 |
| 10 | 分享活动 | 用户已分享活动 | 分享采集失败 |

重复请求不是成功事件的第二次发生。它应通过业务请求号、幂等键或去重规则得到“重复参与已识别”这一处理结果，不能重复扣减次数或发放权益。

## 6. AI Operator 事件链

AI 的模型输出是计划输入，不是业务事实。只有受控 Tool 执行并完成业务验证后，才能产生对应的营销领域事件。

```text
AI 任务已创建
  ↓
AI 意图已识别
  ↓
AI 澄清问题已提出（可选）
  ↓
AI 计划已生成
  ↓
AI Tool 已发现
  ↓
AI Tool 调用已申请审批（写操作）
  ├─ AI Tool 调用已批准 → AI Tool 调用已开始
  │                          ├─ AI Tool 调用已成功 → 业务事实验证 → 对应业务事件
  │                          ├─ AI Tool 调用已失败
  │                          └─ AI Tool 调用结果未知 → 查询/人工接管
  └─ AI Tool 调用已拒绝
```

其他任务状态事件：

- AI 计划已修改：计划版本发生变化，之前绑定的审批可能失效；
- AI 任务部分成功：多步骤任务中已有业务事实成立，后续步骤失败；
- AI 任务已完成：所有必需步骤均已验证；
- 人工已接管 AI 任务：后续处理人和权限边界发生变化。

“模型回答活动创建成功”不是事件；“活动草稿已创建”必须来自活动模块的结构化结果和后续查询验证。

## 7. 行为与分析事件

行为事件服务于触达、归因和分析，不直接替代核心交易事实。

```text
Feed 内容已曝光
  ↓
用户已点击活动
  ↓
用户已参与活动
  ↓
用户已领取权益
  ↓
用户已使用权益
  ↓
业务转化已发生
```

行为采集可以异步进入消息系统，允许重复、延迟或乱序；核心参与、账户和权益事件仍由各自领域模块确认。分析侧可以记录渠道、设备、实验、Feed 项目和扩展属性，但不能用分析宽表反向修改交易事实。

## 8. 事件的最小信息建议

当前只提出事件契约需要回答的问题，不锁定 JSON、消息中间件或数据库结构。每个可对外传播的领域事件至少应能定位：

- `event_id`：事件唯一标识；
- `event_type` 和 `event_version`：事件类型与版本；
- `occurred_at`：业务发生时间；
- `trace_id`：跨 HTTP、RPC、消息和任务的关联线索；
- `actor`：用户、运营、系统或 AI 任务；人工主体只能来自服务端已恢复的 trusted Principal，不能接受客户端自报 Role/Scope；
- `tenant_id`/业务范围：事件属于哪个租户或业务空间；
- `aggregate_id`：活动、参与、账户、权益或 AI 任务等业务聚合标识；
- `idempotency_key`/`business_request_id`：需要幂等时的业务请求标识；
- `data`：本事件必须公开的业务快照，不复制无关敏感信息；
- `occurred_by`/`source`：产生事件的业务模块或外部系统。

事件载荷只保留消费方完成业务所需的信息。完整个人信息、raw password、raw Session token、CSRF、token/throttle digest、模型原始提示词和内部数据库细节不应默认塞进事件。

## 9. 当前暂不决定

- 事件是否同步发布、进入消息队列或写入分析库；
- 事件主题、分区键、保留时间和重试策略；
- 每个事件对应哪张表、哪个聚合或哪个限界上下文；
- 是否采用事件溯源；
- 事件版本兼容、Schema Registry 和跨团队发布流程；
- Session 签发/撤销、account disable/epoch bump 与 throttle 是否需要形成可外发集成事件；
- 最终领域边界和服务拆分。

第 30 节也没有改变以上停止线：Activity transition 已在模块化单体源码、一次性 MySQL 与长期 Compose v11 验收，但仍无 durable domain-event/outbox 表、broker topic、delivery retry 或 consumer contract。COMMIT 应答丢失时，application-owned receipt 只能经 exact observation 对账为 committed/not_committed/indeterminate；它不等于集成事件已发布。第 32 节源码已前进到 latest 14 并新增 Session API，但同样没有把认证状态变成集成事件。后续必须把“聚合/Session 事务已确认”和“集成事件已可靠发布”作为两个可观察阶段。

第 32 节已验收实现的证据入口见[产品基线](identity-session-authentication-v1.md)、[课程](../course/part-04/lesson-32-real-session-authentication.md)、[API](../api/lessons/lesson-32.md)、[QA](../qa/lessons/lesson-32.md)、[设计手记](../design-thinking/lessons/lesson-32.md)、[面试问答](../interview/lessons/lesson-32.md)、[运维手册](../runbooks/identity-session-operations.md)与 [ADR-0028](../decisions/ADR-0028-identity-session-authentication.md)。该完成声明只覆盖 development 认证链；raw framing、真实 wire COMMIT 丢应答、production TLS/可信代理与浏览器扩展矩阵仍未完成。第 33 节服务端 RBAC、第 34 节前端 capability 裁剪和第 35 节越权 E2E 也仍未实现。

这些问题需要第 6 节的限界上下文分析、第 7 节的非功能目标以及后续真实业务实现共同验证。

## 10. 第 6 节输入

> **后续状态：** 第 6 节已形成[限界上下文地图 v1](bounded-context-map-v1.md)，事件所有权以该文档 v1 为当前分析基线。

第 6 节需要基于本地图回答：

1. 活动编排、抽奖/规则、账户/权益、Feed/行为和 AI/MCP 是否应成为独立限界上下文；
2. 哪些事件是上下文内部事实，哪些是跨上下文集成事件；
3. 哪些命令必须通过同步调用完成，哪些事件可以异步传播；
4. 哪些名称需要统一语言，避免“活动、任务、计划、策略”在不同模块各自含义不同；
5. 哪些事件值得进入后续数据库或 API 契约，哪些只保留为过程日志。
