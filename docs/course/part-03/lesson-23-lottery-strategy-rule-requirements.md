# 第 23 节：需求升级——抽奖策略开始需要规则

> 本节不是“先写一个 `Rule` 接口”的编码练习，而是一次需求升级与架构边界冻结。第 22 节已经能把一个合法 Strategy 快照交给无偏加权选择器，并在 React 页面展示临时候选；真实营销抽奖还必须回答活动是否有效、用户是否有资格、应路由到哪个奖池、候选是否可用，以及拒绝与系统失败怎样解释。本节先区分每个判断的决定所有者与原始事实提供方，冻结失败语义和后续兼容点，**不新增任何运行时代码、数据库结构、HTTP 契约、Redis 调用、前端判断或权限实现**。

- **需求基线：** [Lottery 规则需求 v1](../../product/lottery-rule-requirements-v1.md)
- **长期决策：** [ADR-0019：Lottery 规则所有权与求值边界](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- **API 记录：** [第 23 节 API](../../api/lessons/lesson-23.md)
- **验收证据：** [第 23 节 QA](../../qa/lessons/lesson-23.md)
- **第一性原理手记：** [第 23 节设计手记](../../design-thinking/lessons/lesson-23.md)
- **面试问答：** [第 23 节面试问答](../../interview/lessons/lesson-23.md)

## 1. 为什么需求分析本身是一节正式交付

“抽奖前加几个条件”听起来像一个很小的改动，但它至少跨越五类事实：

1. Activity 的发布状态和有效时间；
2. 用户是否满足新人、会员等级、次数或风险条件；
3. 应选择哪个 Strategy 或奖池；
4. Award 在当前时刻是否仍可参与选择；
5. 最终加权选择、库存兑现和权益发放。

如果立即在 handler 里堆 `if`，规则会依赖 HTTP DTO；如果把所有字段塞进 Strategy，Lottery 会复制并篡改 Marketing、Participation、Benefit、Governance 及外部会员/风险系统的权威事实或决定；如果先造通用规则引擎，又会在规则数量、配置方式、版本发布和解释需求都未知时冻结错误抽象。

因此，本节“实现”的对象是可验证的需求契约：每个判断由谁拥有、输入事实从哪里来、输出属于哪类语义、失败时是否允许继续、未来在哪一节落地。它减少的是下一阶段的返工与歧义，不用空接口或空表伪造进度。

## 2. 本节目标

完成本节后，仓库应能准确回答：

1. 当前 `WeightedSelector` 已经解决什么，为什么它不是规则引擎；
2. 一个“新人抽奖”活动究竟包含哪些相互独立的判断；
3. Marketing、Participation、Lottery、Benefit 与 Governance 分别拥有什么决定，外部事实提供方又承担什么；
4. `continue`、`route`、业务拒绝、`no_reward`、候选不可用、授权拒绝和技术失败为什么不能合并；
5. 为什么规则身份、原因码、规则集版本、配置版本与 schema 版本必须分开；
6. 为什么解释轨迹需要稳定顺序和最小数据，却不能记录完整用户画像；
7. 为什么当前不选 handler 条件分支、巨型 Strategy、通用 `Rule` 接口、JSON DSL 或 DMN 引擎；
8. 第 24 节 Redis 可以缓存什么，绝不能缓存什么；
9. 第 25～30 节怎样在不倒灌实现的前提下承接资格、决策链/树和 Activity；
10. 第 31～35 节的公共访问控制为什么应与业务资格分离，并在第一个真实运营写入口前完成。

## 3. 开始前的真实能力

第 17～22 节已经形成下面这条真实路径：

```text
canonical StrategyID
  -> MySQL Repository 读取一个合法 Strategy 聚合快照
  -> WeightedSelector 从固定 Award 权重中选择一个候选
  -> development/test ephemeral HTTP response
  -> React /lottery 展示临时候选
```

其中已经成立的事实是：

- Strategy 由一个或多个 Award 构成；
- Award outcome 显式区分 `reward` 与 `no_reward`；
- 权重是正整数相对权重，不是假定总和为 100 的百分比；
- Repository 用一致快照恢复合法聚合；
- `WeightedSelector` 对给定合法 Strategy 执行无偏固定权重选择；
- production adapter 使用有界密码学随机源；
- HTTP 路由默认关闭，只在 development/test 明确启用；
- React 页面不再用浏览器随机数决定 Award，也不自动重试不确定结果。

这条路径当前只能回答：

> 给定一个已经合法、已经选定且候选固定的 Strategy 快照，本次临时选择落在哪个 Award 权重桶？

它不能回答：

- 当前是否存在一个已发布且生效的 Activity；
- 调用者是谁、是否被允许执行这个动作；
- 某用户是否是新人、是否还有次数、是否命中风险拒绝；
- 会员等级应路由到哪个 Strategy；
- 奖品是否可售、可发、未冻结或仍有库存；
- 这次判断使用了哪个规则集/配置版本；
- 为什么拒绝、路由或选择了某个候选；
- 是否形成了可恢复的正式 Draw/Result；
- 是否预占库存、扣积分或发放 Benefit。

## 4. 用一个复合场景暴露真实缺口

本节以“新用户抽奖”作为需求分析样例。它是后续设计输入，不是已经上线的接口：

> 只有活动处于已发布且有效时间内、新用户资格成立、参与次数未耗尽且没有命中风险拒绝时，才允许继续；不同会员等级可以路由到不同 Strategy；不可用 Award 不得被当作可兑现奖励；完成所有前置判断后，合法的 `no_reward` 仍可能被正常选中。

这句话至少可以拆成以下问题：

| 判断 | 需要的权威事实 | 期望结果 | 当前能否回答 |
| --- | --- | --- | --- |
| 活动是否已发布 | Activity lifecycle | 继续或业务拒绝 | 否 |
| 当前时间是否在窗口内 | Activity window + 受控时钟 | 继续或业务拒绝 | 否 |
| 是否是新用户 | 用户/参与事实 | 继续或业务拒绝 | 否 |
| 次数是否可用 | Participation quota | 继续或业务拒绝 | 否 |
| 是否命中风控 | Risk decision | 继续、拒绝或技术失败 | 否 |
| 会员等级走哪个池 | 会员事实 + 路由配置 | 路由到 Strategy | 否 |
| Award 是否可参与 | 配置/库存/权益可用性 | 保留、排除或拒绝 | 否 |
| 在合法候选中选哪一个 | Strategy + bounded random source | `reward` / `no_reward` 候选 | 是 |
| 是否成功兑现 | 库存预占 + Benefit/订单事实 | 成功、补偿或失败 | 否 |

拆解后的关键发现是：所谓“规则”并不是一种统一数据，也不是一个上下文可以独占的所有判断。它是多个决定所有者消费受控事实快照完成的一次协作；提供原始事实不等于拥有最终决定。

## 5. 先区分七类判断与阶段

未来链路可以用下面的概念顺序讨论，但本节不创建任何对应接口：

```text
访问控制
  -> Activity gate
  -> Participation eligibility
  -> Lottery routing / decision
  -> candidate allocatability
  -> WeightedSelector
  -> Inventory reservation / Benefit delivery
```

### 5.1 访问控制

访问控制回答“这个主体能否对这个资源执行这个动作”。它依赖可信身份、角色/权限、资源、动作和数据范围，未来由第 31～35 节建立公共能力。

它不回答用户是否是新人，也不回答活动是否已过期。前端隐藏按钮只能改善体验，不能构成授权证据。

### 5.2 Activity gate

Activity 拥有发布、暂停、结束和有效时间等生命周期事实。Activity 未发布或已过期属于业务拒绝，不应该伪装成 Strategy 不存在或随机选择失败。

Activity 在第 30 节才成为真实业务对象。本节只冻结所有权，不能提前创建一个猜测字段不完整的 Activity 表。

### 5.3 Participation eligibility

Participation 负责一次用户参与是否满足资格、次数和既有参与事实。第 25 节先建立第一条真实用户资格规则，第 26 节再由真实组合需求演进出最小线性短路链；本节不创建 User、quota 或 risk 字段。

### 5.4 Lottery routing 与 decision

Lottery 可以根据已经获得的可信事实决定走哪个 Strategy。路由是中间决策，必须有明确 target 或缺省分支；它不应与终端随机选择共用一个含糊的 `bool`。

### 5.5 候选可分配性

候选是否可分配需要配置状态、库存/额度和权益承诺等权威事实。它既不是用户资格，也不是 selector 的随机结果。不可分配后是排除、拒绝还是进入明确 fallback，必须由版本化产品策略决定；在该策略出现前不能默认重归一化或补抽。

### 5.6 terminal `WeightedSelector`

当前 `WeightedSelector` 只接收一个合法且候选固定的 Strategy，输出 `reward` 或 `no_reward` Award 候选。它是第 20 节已经验证的终端算法，不读取用户、Activity、库存或权限。

### 5.7 库存、发放与补偿

候选被选中不等于库存已经预占，更不等于权益已经发放。库存、订单、Benefit 与补偿在后续阶段出现。在它们存在前，规则系统不能通过一个 `available=true` 的临时字段冒充兑现保证。

## 6. 规则所有权矩阵

| 规则族 | 决定所有者 | 原始事实提供方 | 未来落地点 | 本节禁止的捷径 |
| --- | --- | --- | --- | --- |
| 发布状态/有效时间 | Marketing | Marketing 的 Activity 发布快照与受控服务端时钟 | 第 30 节及其后 | 把时间字段塞入 Strategy |
| 新人/会员/次数 | Participation | 外部用户/会员映射与 Participation 账户/流水 | 第 25～27、39～45 节分步形成 | 在 React 里判断用户标签 |
| 本场景风险准入 | Participation | 受控风险事实端口提供的最小 verdict 与版本 | 资格规则出现后按证据演进 | 把依赖 timeout 当作不合格 |
| Strategy 路由 | Lottery | Lottery 发布的决策配置与受控事实快照 | 第 27～29 节 | 用 Award weight 表达会员分流 |
| 固定权重选择 | Lottery | Strategy/Award 配置与 bounded random source | 第 20 节已完成 | 把资格逻辑塞进 selector |
| 候选可分配性 | Benefit | Benefit 内部库存子能力、权益模板与预占事实 | 第 43～45 节形成主链，第 46～52 节补可靠性 | 把“配置存在”当作“库存存在” |
| 正式 Draw/Result | Lottery | 已确认的参与身份、规则/Strategy 快照与选择事实 | 第 45、51 节逐步形成 | 把 ephemeral response 当最终结果 |
| 发放与补偿 | Benefit | Reward/Draw 引用、发放流水与外部权益回执 | 第 46～52 节 | 把 reward 候选当作已到账 |
| 操作者能否发布/编辑 | Governance | 身份/IAM 适配、访问策略、资源/动作/范围 | 第 31～35 节 | 每个页面复制 `activeRole` |

所有权不是“只有该上下文能读取”，而是只有该上下文能定义并改变权威事实。其他上下文通过明确的 application port、快照或已发布事件消费，不复制一份可独立修改的真相。

## 7. 失败语义必须先于实现稳定

### 7.1 业务拒绝

事实读取成功、判断执行成功，但业务条件不成立，例如活动已过期或新人资格不成立。它需要稳定 RuleCode/ReasonCode 和面向用户的安全文案，不能被记录成基础设施异常。

### 7.2 `no_reward`

所有前置判断通过后，`WeightedSelector` 正常选中一个 outcome 为 `no_reward` 的 Award。它是合法终端结果，不是业务拒绝、异常兜底或空响应。

### 7.3 候选不可用

某个 Award 因库存、权益状态或配置治理原因不能参与兑现。这是候选集合/兑现能力问题，不等同于“用户不合格”。到底是排除个别候选、拒绝整次决策还是进入补偿，要等事实模型与正式 Draw 边界出现后决定；本节只要求它不得被静默转成 `no_reward`。

### 7.4 授权拒绝

主体没有对资源执行动作的权限。它发生在业务判断之前或独立的强制点，未来由统一访问控制模型解释。授权拒绝不能暴露受保护对象细节，也不能被计入“新人规则未通过”。

### 7.5 技术失败

事实源 timeout、版本不兼容、配置损坏、随机源失败或依赖不可用，导致系统无法得出可信业务结论。技术失败必须失败关闭并可关联观测；不能为了“用户体验”降级为不合格或 `no_reward`。

### 7.6 结果未知

未来正式 Draw 形成副作用后，客户端 timeout 或响应丢失可能让调用方无法确认结果。Unknown 不是失败已回滚，也不是可以重新随机；只有稳定业务身份、结果持久化和查询/恢复路径出现后，才能承诺安全恢复。当前 ephemeral selection 没有这些能力。

### 7.7 中间决策

`continue` 和 `route` 都不是最终中奖结果。未来评估输出至少要能区分：

```text
continue | reject(reason) | route(target) | technical_failure(cause)
```

本节不冻结 Go 类型或 JSON 字段，只冻结这些语义类别不能被一个 `bool` 或 `error == nil` 抹平。

## 8. 为未来兼容性保留哪些信息

### 8.1 稳定身份与展示文案分离

RuleCode/ReasonCode 用于程序分支、指标聚合与审计关联；中文展示文案可以调整、国际化或按渠道变化。不能让前端通过匹配“您不是新用户”决定逻辑。

### 8.2 三类版本不能复用一个数字

| 版本 | 回答的问题 |
| --- | --- |
| RuleSetVersion | 本次用了哪套已发布规则及顺序 |
| Strategy/配置版本 | 本次用了哪份可选择配置 |
| Schema/解释器版本 | 当前程序怎样解析配置格式 |

当前 `lottery_strategy.updated_at` 是行元数据，不是聚合版本。第 23 节不能为了文档看起来完整就给它虚构版本语义。

### 8.3 输入事实必须可追溯但不过度收集

未来跨上下文事实至少要明确来源、采集时间和可用版本，以判断新鲜度；解释轨迹应记录规则身份、版本、结果、稳定原因码和必要耗时。完整用户画像、手机号、凭据、原始风控特征等不应进入通用 trace。

### 8.4 顺序与短路是可观察行为

未来规则链应保持确定顺序、无隐式共享可变状态，并明确哪些业务拒绝可以短路。调整顺序可能改变结果、依赖调用量和暴露信息，因此必须随 RuleSetVersion 发布，而不能只改数组顺序。

### 8.5 规则树需要结构校验

如果第 28～29 节引入规则树，加载阶段至少要考虑根节点、边引用、环、深度、不可达节点、结果完备性和确定性。现在建立通用树结构只会在实际节点语义未知时制造错误约束。

## 9. 为什么本节不实现通用规则引擎

本节比较了五类可行路线：

| 方案 | 当前收益 | 当前主要风险 | 结论 |
| --- | --- | --- | --- |
| handler 内顺序 `if` | 最快看到条件分支 | 绑定 HTTP、难解释、所有权混乱 | 不采用 |
| 扩大 Strategy 聚合 | 看似对象更少 | Activity/用户/库存事实被 Lottery 吞并 | 不采用 |
| 先建统一 `Rule` 接口 | 便于画结构图 | 输入/输出过宽，容易退化为 `map[string]any` | 暂不采用 |
| JSON DSL/表达式/DMN | 可配置、可视化潜力 | 发布、校验、安全、版本和运维成本无需求证据 | 暂不采用 |
| 先冻结需求与所有权 | 没有运行时“演示感” | 需要下一节继续落地 | 采用 |

“暂不采用”不是永远拒绝。出现跨多个 Activity 的高频规则发布、非开发人员配置、解释回放、灰度版本或大量重复节点时，才有证据重新比较代码规则、决策表、DSL 或标准化决策模型。

## 10. `WeightedSelector` 的边界保持不变

`WeightedSelector` 继续只做一件事：从一个合法、固定候选集合的 Strategy 中执行无偏加权选择。它不应新增：

- UserID、member tier、ActivityID 或 tenant 参数；
- 当前时间和 Activity 状态；
- quota、risk、库存或 Benefit 查询；
- HTTP、JSON、SQL、Redis 或认证依赖；
- 规则解释、审计落库或结果持久化；
- 自动重试与分布式锁。

这样做不是为了“算法纯洁”而抽象，而是为了让已验证的数学命题继续成立。规则变化决定谁能到达 selector、选择哪个 Strategy 或哪些候选合法；selector 只负责最终权重桶映射。

## 11. 第 24 节 Redis 的输入边界

第 24 节可以把 MySQL 中可重建的 Strategy 读取投影作为第一个真实缓存候选，前提是明确：

- MySQL 仍是权威源；
- cache miss、损坏、timeout 与回源语义可区分；
- key 至少包含格式版本和 StrategyID；
- 当前没有真实聚合版本，不能伪造精确失效承诺；
- 缓存失败不能改变 Repository/selector 的业务错误语义。

第 24 节不应缓存：

- 用户资格判断结果；
- 风控结论；
- 一次规则链的中间输出；
- ephemeral selection 结果；
- 未形成正式事实的 Draw/Result；
- 权限决策。

这些对象要么涉及主体、时间和数据新鲜度，要么尚无可恢复身份与失效协议。把它们缓存起来不会让系统更完整，只会把未定义语义扩散到 Redis。

## 12. 与公共权限能力的顺序关系

第 22 节已经有用户、Admin、MCP 和 Agent 四类工作台，但它们仍是信息架构与 Mock/本地状态，不是真实权限系统。第 23 节也不应因为页面已经出现，就立即添加一组前端角色判断。

课程先在第 25～30 节形成真实资格、决策和 Activity 资源，再在第 31～35 节依次建立：

1. 公共主体—角色—权限—资源—动作—数据范围模型与威胁边界；
2. 真实会话认证；
3. 服务端 RBAC 强制；
4. 前端按权限裁剪导航、路由和操作；
5. 越权与浏览器端到端验收。

第 36 节第一个真实运营后台只能消费这套公共能力。业务资格回答“这个用户是否满足活动条件”，访问控制回答“当前主体是否能执行受保护动作”；两者可以连续发生，但不能共用角色开关或拒绝码。

## 13. 本节明确非目标

本节不做以下任何事情：

- 不新增或修改 Go production package、接口、struct 或测试替身；
- 不创建 `Rule`、`RuleEngine`、`Specification`、责任链或规则树接口；
- 不给 Strategy 增加 Activity、User、quota、risk、inventory 或版本字段；
- 不使用 `map[string]any` 传递规则事实；
- 不设计 JSON DSL、表达式语言、脚本、插件或 DMN runtime；
- 不新增 `000003` Migration、规则表或任何索引；
- 不新增、修改或重新解释 HTTP route、DTO、status/code；
- 不在 React 中实现资格判断、角色判断或 Award 过滤；
- 不接入 Redis client、key、TTL、锁、Lua 或业务 readiness；
- 不实现认证、RBAC、对象级授权或前端权限裁剪；
- 不虚构 Strategy aggregate version、RuleSetVersion 或正式 Draw identity；
- 不实现库存、积分扣减、权益发放、幂等或审计落库。

## 14. 本节交付物

| 交付物 | 作用 |
| --- | --- |
| [规则需求 v1](../../product/lottery-rule-requirements-v1.md) | 冻结复合场景、规则族、失败语义、兼容字段和章节映射 |
| [ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md) | 固化跨上下文所有权与暂不建立通用引擎的长期决定 |
| 本课程正文 | 给出学习顺序、当前能力、边界和下一节输入 |
| [API 记录](../../api/lessons/lesson-23.md) | 明确运行时 HTTP 契约零变化，阻止规划倒灌 |
| [QA](../../qa/lessons/lesson-23.md) | 以正反例、语义矩阵和 Git 负向 diff 验收文档切片 |
| [设计手记](../../design-thinking/lessons/lesson-23.md) | 从决定所有权、事实来源、失败和演进成本重放推导过程 |
| [面试问答](../../interview/lessons/lesson-23.md) | 把规则边界、选型与真实能力转化为可追问表达 |

这些文档共同构成交付，任何一份都不能单独代替其他证据。

## 15. 可执行验收

第 22 节最终 tip 固定为：

```text
1f95779277b1ea882d607a59e0fd2c475f58bd7a
```

### 15.1 文档与全仓门禁

```bash
make doc-check
make verify
git diff --check
```

### 15.2 运行时代码零漂移

在第 23 节所有内容提交后执行：

```bash
lesson23_base=1f95779277b1ea882d607a59e0fd2c475f58bd7a
git diff --exit-code "$lesson23_base" -- . \
  ':(exclude)README.md' \
  ':(exclude)docs/**'
```

该命令必须没有输出并 exit 0。它证明相对第 22 节 tip，除根 README 和 `docs/**` 外没有 tracked diff；它不能证明文档中的未来方案已经实现。

### 15.3 Migration 与现有 API 边界

```bash
find migrations/sql -maxdepth 1 -type f -name '*.sql' -print | LC_ALL=C sort
git diff --exit-code "$lesson23_base" -- migrations cmd internal web configs deploy go.mod go.sum Makefile
```

Migration SQL 仍只能是现有 `000001` / `000002` 两个 `.up.sql` 文件，其他 migration support files 也不得变化；第二条命令必须 exit 0。现有 ephemeral route、DTO、feature gate、selector 和 React 消费者保持第 22 节语义。

### 15.4 需求完整性评审

逐条核对 [规则需求 v1](../../product/lottery-rule-requirements-v1.md)：

- 每个判断只有一个决定所有者，并把原始事实提供方单列；
- 每个判断都归入继续、路由、业务拒绝、授权拒绝、候选不可用、`no_reward` 或技术失败之一；
- 每个待实现能力都有未来章节，不把规划写成当前事实；
- `no_reward` 与拒绝/异常有正反例；
- RuleCode、ReasonCode、规则集/配置/schema 版本没有混用；
- trace 只保留必要解释，不要求记录 PII；
- Redis 只接收可重建 Strategy 读投影的候选设计；
- 权限与业务资格保持独立。

完整命令、白名单和正反例见 [第 23 节 QA](../../qa/lessons/lesson-23.md)。

## 16. 本节完成后的真实能力

可以准确表述：

> 基于已完成的 Strategy Repository、无偏加权选择和 ephemeral 全栈切片，拆解新用户抽奖复合需求，建立 Marketing、Participation、Lottery、Benefit 与 Governance 的决定所有权矩阵，并把外部事实提供方单列；同时区分业务拒绝、授权拒绝、`no_reward`、候选不可用和技术失败，以需求基线与 ADR 冻结后续规则链/树、版本和解释边界。

不能表述：

- 已实现规则引擎、责任链、规则树或动态 DSL；
- 已上线新人/会员/次数/风控资格；
- 已实现 Activity、库存、发奖、正式 Draw 或审计回放；
- 已接入 Redis 规则缓存或高并发抽奖；
- 已实现登录、RBAC 或按角色页面权限；
- 规则需求评审等于运行时自动执行或性能已经验证。

## 17. 对后续章节形成的约束

### 第 24 节

只为可重建的 Strategy 读取投影引入 Redis 缓存，先定义权威源、格式版本、key、miss/损坏/timeout、回源和故障放大；不能缓存用户资格或临时选择结果。

### 第 25 节

选择一个最小、真实、由 Participation 单独拥有决定且事实来源明确的用户资格规则形成第一条可执行判断，并补充解释与失败证据；不能一次实现全部复合场景或通用引擎。

### 第 26～27 节

第 26 节在真实第二条前置规则出现后演进最小线性、确定顺序且可短路的责任链；第 27 节再用分支/路由需求暴露线性链局限，不能为展示设计模式虚构复杂度。

### 第 28～29 节

在链路局限已经可复现后，再定义规则树 schema、图校验与决策执行。每次抽象都必须由重复、分支和失败证据驱动。

### 第 30 节

建立 Strategy 与 Activity 的正式边界，决定已发布 Activity 如何引用不可变或可追溯的 Strategy 配置，而不是让 Strategy 反向拥有活动生命周期。

### 第 31～35 节

建立跨用户端、运营端、MCP 和 Agent 的公共访问控制能力。服务端拒绝是安全边界；前端裁剪只消费同一权限投影，不独立定义权限真相。

## 18. 复盘

这一节最重要的工程动作不是创建更多类型，而是承认“现在还不知道足够多”。已有 selector 是一个已经被数学和测试验证的终端算法，把用户、活动、库存或权限塞进去会破坏它的可解释边界；与此同时，只写一句“以后加规则引擎”又无法指导真实演进。

通过复合场景、所有权矩阵、失败语义、版本边界和负向 Git 证据，本节把一个模糊需求变成了可实施队列：下一节可以安全处理读取性能，之后再用真实资格规则迫使抽象逐步出现。这样的停顿不是项目暂停，而是避免大重构的正式架构检查点。
