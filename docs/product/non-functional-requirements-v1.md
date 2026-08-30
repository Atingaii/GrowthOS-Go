# 非功能需求基线 v1

**状态：** v1 目标基线；M0 工程探针与 M1 Strategy 缓存本地基线已实测，业务 SLO 尚未实测

**更新日期：** 2026-08-30

**来源章节：** [第 7 节：确定非功能需求](../course/part-01/lesson-07-non-functional-requirements.md)；第 23 节以[规则需求与所有权边界](../course/part-03/lesson-23-lottery-strategy-rule-requirements.md)补充未来决策正确性约束；第 24 节以[首次 Redis Strategy 缓存](../course/part-03/lesson-24-redis-strategy-cache.md)登记 M1 本地证据

## 1. 使用规则

本文件是 GrowthOS 的非功能需求台账。它记录目标、测量口径和未来证据，不是性能宣传页。

1. `目标值` 表示需要在对应里程碑验证的候选 SLO；
2. `实测值` 只能引用带环境和脚本版本的 QA/压测报告；
3. 没有实现或没有测量时写“未测量”，禁止填入估算结果；
4. 需求变化时保留修改原因，不能为了让测试通过而静默降低目标；
5. 外部支付、交易、券、消息和 LLM 的指标与 GrowthOS 自身指标分开统计。

## 2. 当前状态

| 项目 | 当前事实 |
| --- | --- |
| Go 运行时基线 | 第 11～16 节已验收 Gin 进程、`GET /health`、MySQL `GET /ready`、类型化配置、文件秘密、结构化日志、请求关联与统一错误；Compose 只发布同源 Web 入口；第 17～21 节新增并装配 Lottery 领域、两表、Repository、选择器、生产随机源与受限 ephemeral API；第 24 节在权威 Reader 外装配可选 Redis cache-aside，Redis 不参与启动或 readiness |
| 业务数据库 | MySQL 8.4 连接池、账号隔离和前向 Migration 机制已通过功能联调；`000001` / `000002` 创建 `lottery_strategy` / `lottery_strategy_award`，latest 2；Create/FindByID 用手写 SQL、写事务和只读 RR 快照；当前运行应用只有两表 `SELECT`，不能 INSERT、UPDATE、DELETE 或访问 `schema_migrations`，历史 writer 集成测试使用隔离身份 |
| Lottery 领域 | Strategy/Award、Repository、WeightedSelector 与 CryptoSource 已覆盖不变量、快照、错误和边界；第 21 节通过 `EphemeralSelectionService` 与 HTTP adapter 形成真实链路；第 23 节只冻结规则事实所有权；第 24 节缓存版本化 Strategy 投影，限制 2 MiB/1000 Award/TTL≤5m，同 key 合并 cold fill，坏值精确删除并回源。仍没有认证、资格、Draw/Result、幂等、库存、发奖或业务 SLO 实测 |
| Strategy 缓存 | MySQL 始终是权威来源；Redis 只保存可重建读取投影，不保存 not-found、一次选择或最终结果。缓存 miss/错误/写失败 fail-open，staging/production 启用时强制身份验证 TLS；Compose ACL 仅允许版本化 key 前缀内 `PING/GETRANGE/SET/DEL`，48 MiB `allkeys-lru`、无持久化 |
| 规则决策质量 | 未来前置规则除终端随机票据外应在同一规则版本、事实快照和评估时刻下保持确定；业务拒绝、授权拒绝、资源不可用、技术失败/未知与 `no_reward` 必须分类；权威事实、版本、原因码和最小披露 trace 由对应实现章节逐步验证，当前只形成需求基线 |
| React | 第 14 节框架与第 15 节系统状态页已完成；系统探针在宿主开发模式经 Vite、Compose 模式经 Nginx 同源代理真实读取 Go API。第 22 节再用严格 Lottery adapter、运行时解码与请求状态 Hook 让 `/lottery` 真实消费 ephemeral API；其余用户、运营、MCP 与 Agent 工作台仍是显式 Mock 快照或浏览器本地交互；要求 Node.js `>=22.22.2`、pnpm `10.13.1` |
| 前端质量门 | Vitest、TypeScript typecheck 与 Vite build 已纳入验证；第 15 节真实浏览器核对系统探针正常、数据库不可用和 API 离线状态；第 22 节继续核对桌面/移动布局、键盘与焦点交互、请求 pending/成功/失败/取消分支、权限前置缺口以及按路由拆分图表产物 |
| 性能实测 | M0 `/health` 100 RPS×5min：30,000/30,000 成功、P99 4.1495ms；`/ready` 20 RPS×30s：600/600 成功、P99 6.841375ms。M1 在同一本地 Docker Desktop 上对 ephemeral selection 各跑 50 RPS×10s：warm-cache、cache-disabled、Redis-down 均 500/500 成功且零 error/unexpected/dropped；三组 P99 分别 5.202ms、9.747167ms、8.222959ms。均为单机短窗口开发基线，不是业务 SLO或生产容量 |
| 可用性实测 | 两个短窗口内错误、异常状态和丢弃均为 0；没有长稳、跨主机、灾备或生产可用性证据 |
| 恢复演练 | 已演练 MySQL、API、Redis 单点停止与恢复；第 24 节进一步证明 Redis down 时 cold read 回源、MySQL down 时 warm hit 可用而 cold miss 失败、两者恢复后无需重启 API 可重新填充。未验证宿主机故障、Redis/MySQL HA 或数据灾难恢复 |
| 安全与故障演练 | 已验证 MySQL 运行身份仅两表 `SELECT`；Redis 默认用户关闭且业务身份只有四命令/单前缀，channel 与管理/扫描命令被拒绝；还验证唯一回环端口、internal cache 网络、非 root/只读/capability、受限 Host/framing/size、日志低基数/限流和 502/504 请求关联；不等于认证、对象授权或生产渗透测试 |

第 13～23 节证据依次证明 MySQL/Migration、浏览器探针、Compose M0、Lottery 聚合/Schema/Repository/selector、受限 HTTP 链、React 消费者和规则停止线。第 24 节隔离 acceptance 又证明缓存契约、ACL、poison 修复、Redis/MySQL warm/cold 故障恢复和三组定速 M1 基线，且调用前后两张业务表 fingerprint 不变；访问日志、连接统计和缓存写入仍是技术副作用。M1 的每组 500 请求、50 RPS、10 秒、最多 16 workers 只是当前本机开发证据；没有正式 Draw 持久化、资格/库存争用、长稳、多主机或生产数据分布，因此不能把下面任何抽奖业务候选 SLO 标为已达到，也不能仅凭三组延迟差异断言通用缓存收益。

### M0 工程探针实测

| 路径 | 负载窗口 | 成功 / 计划 | 错误 / 异常状态 / 丢弃 | P50 | P95 | P99 | 最大值 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `/health`（经 Nginx） | 100 RPS × 5 min | 30,000 / 30,000 | 0 / 0 / 0 | 1.084208 ms | 2.744875 ms | 4.1495 ms | 18.116291 ms | 通过本节 P99 ≤ 100 ms 工程门槛 |
| `/ready`（经 Nginx + MySQL Ping） | 20 RPS × 30 s | 600 / 600 | 0 / 0 / 0 | 4.08525 ms | 5.935083 ms | 6.841375 ms | 8.570541 ms | 记录基线；未设置独立 P99 门槛 |

环境、原始 JSON、资源水位、故障注入与未覆盖项见[第 16 节 QA](../qa/lessons/lesson-16.md)。

### M1 Strategy 缓存本地实测

| 场景 | 负载窗口 | 成功 / 计划 | 错误 / 异常状态 / 丢弃 | 实际 RPS | P50 / P95 / P99 / 最大值 | 权威读取与缓存证据 |
| --- | --- | ---: | ---: | ---: | --- | --- |
| warm-cache | 50 RPS × 10s | 500 / 500 | 0 / 0 / 0 | 50.086182 | 1.730167 / 4.129458 / 5.202 / 7.387375 ms | MySQL execute `0`；cache hit `500` |
| cache disabled / direct-MySQL | 50 RPS × 10s | 500 / 500 | 0 / 0 / 0 | 50.076986 | 2.390709 / 5.382042 / 9.747167 / 25.003417 ms | MySQL execute `1000`；source load `500`；cache event `0` |
| Redis down | 50 RPS × 10s | 500 / 500 | 0 / 0 / 0 | 50.050506 | 2.602833 / 5.777708 / 8.222959 / 10.596541 ms | MySQL execute `1000`；source load/fill leader `500/500`；joined `0`；限流 read-error log `1` |

MySQL 计数来自 Performance Schema 的 prepared statement `statement/com/Execute`，因为当前 Go driver 会把带参数的两条 Strategy SELECT 作为 prepared execute；脚本同时核对运行身份仍为精确 SELECT-only，查询 fingerprint 未变。三组在同一个 2026-08-30 本地开发环境顺序执行，不能作为跨版本显著性检验、云环境容量结论或 SLO；完整汇总口径与证据边界见[第 24 节 QA](../qa/lessons/lesson-24.md)。

## 3. 服务等级候选目标

| 能力 | 峰值容量目标 | P99 目标 | 可用性目标 | 当前实测 |
| --- | ---: | ---: | ---: | --- |
| Feed 读取 | 50,000 RPS / 5min | ≤ 200ms | 99.90% | 未测量 |
| Behavior 事件接收 | 100,000 RPS / 5min | ≤ 100ms | 99.90% | 未测量 |
| 活动参与 | 10,000 RPS / 5min | ≤ 300ms | 99.95% | 未测量 |
| 抽奖决策 | 10,000 RPS / 5min | ≤ 150ms | 99.95% | 未测量 |
| 权益发放受理 | 5,000 RPS / 5min | ≤ 300ms | 99.95% | 未测量 |
| 运营查询/写入 | 300 RPS / 5min | 查询 ≤ 500ms；写入 ≤ 800ms | 99.95% | 未测量 |
| 分析查询 | 500 RPS / 5min | ≤ 2s | 99.90% | 未测量 |
| MCP Tool 网关 | 500 RPS / 5min | 网关开销 ≤ 100ms | 99.50% | 未测量 |

统计窗口、有效请求定义、外部耗时排除项和阶段性门槛见课程正文。后续修改本表时必须同步对应 QA 证据。

## 4. 关键业务不变量

| 编号 | 不变量 | 未来证据 |
| --- | --- | --- |
| INV-01 | 同一业务请求最多形成一份有效参与订单 | 并发重复请求测试、唯一约束验证 |
| INV-02 | 一次参与最多消耗一次额度 | 余额/流水对账和重试测试 |
| INV-03 | 一次抽奖只能有一个最终结果 | 超时、崩溃恢复和结果查询测试 |
| INV-04 | 权益变化全部关联唯一业务流水 | 账实对账、重复消息测试 |
| INV-05 | 库存领取总量不超过可分配库存 | 高并发库存测试和最终对账 |
| INV-06 | 活动和审批并发修改不会静默覆盖 | 版本冲突测试 |
| INV-07 | 高风险操作不能绕过权限与审批 | 越权、审批失效和审计测试 |
| INV-08 | 模型文本不能直接确认业务成功 | Tool 结果校验和失败注入测试 |
| INV-09 | 每次高风险写操作必须形成完整且不可由普通用户覆盖的审计记录 | 审计完整性对账、故障注入测试 |

## 5. 最终一致性目标

| 链路 | 目标 | 观测方式 | 当前实测 |
| --- | ---: | --- | --- |
| Redis 库存 → MySQL | ≤ 5s | 库存版本延迟、账实差异 | 未测量 |
| 异步权益发放 | 95% ≤ 3s，99.9% ≤ 5min | 创建到到账的分位数 | 未测量 |
| Behavior → 实时画像 | ≤ 10s | 事件时间与画像水位 | 未测量 |
| Behavior → 漏斗分析 | ≤ 5min | 数据水位与查询更新时间 | 未测量 |
| 事实 → OpenSearch 投影 | ≤ 60s | 版本和更新时间差 | 未测量 |
| 外部转化事件 → 分析链 | 接收后 ≤ 60s | 来源时间、接收时间和入链时间 | 未测量 |

## 6. 恢复目标

| 数据/能力 | RPO | RTO | 当前证据 |
| --- | ---: | ---: | --- |
| 单实例故障下已确认核心事实 | 0 | ≤ 5min | 无 |
| 核心数据库灾难恢复 | ≤ 5min | ≤ 60min | 无 |
| 活动与配置数据 | ≤ 5min | ≤ 60min | 无 |
| 已可靠接收的行为数据 | ≤ 15min | ≤ 4h | 无 |
| 可重建的搜索、画像和分析投影 | 可重建 | ≤ 8h | 无 |

## 7. 降级优先级

```text
优先保护：参与事实、抽奖结果、权益流水、库存、审批和审计
可以降级：个性化排序、实时画像、分析查询、搜索和 AI
不能降级：权限校验、幂等校验、金额/额度正确性和事实验证
```

GrowthOS 的人工运营能力不能依赖 LLM 才能工作；核心交易不能依赖分析链实时可用；外部权益不可用时必须保持处理中或失败状态，不能显示假成功。

## 8. 可观测性验收清单

- [ ] HTTP、RPC、MQ、任务、MCP 和 Agent 链路可关联 `trace_id`；
- [ ] 核心写操作可按业务请求号查询最终状态；
- [ ] Dashboard 能看到 QPS、错误率、P95/P99 和资源水位；
- [ ] 库存差异、消息积压、补偿和外部依赖有业务告警；
- [ ] 错误、结果未知和高风险操作保留完整 Trace；
- [ ] 日志、事件与 Prompt 不包含未脱敏秘密和完整个人数据；
- [ ] 告警关联值班手册、影响范围和恢复验证。

## 9. AI 与审批目标

| 控制项 | 候选目标 | 当前实测 |
| --- | --- | --- |
| 审批有效期 | 默认 ≤ 24h；版本、参数、预算或风险变化立即失效 | 未测量 |
| MCP Tool 网关开销 | P99 ≤ 100ms，不含 Tool、LLM 和人工等待 | 未测量 |
| AI 任务检查点恢复 | 执行器恢复后 ≤ 15min 恢复可继续任务 | 未测量 |
| 核心写 Tool 结果未知核对 | ≤ 5min 自动查询或升级人工 | 未测量 |
| 高风险写操作审计完整性 | 100% | 未测量 |

单个 Tool 的业务超时、重试和执行预算必须在接口出现时单独登记，不能用网关开销代替端到端耗时。

## 10. 阶段证据台账

| 里程碑 | 计划证据 | 状态 |
| --- | --- | --- |
| M0 · 第 16 节 | 健康接口负载、连接池与 Compose 稳定性 | 已验证；仅为本机短时工程基线，详见第 16 节 QA |
| 第 17 节领域前置 | Strategy/Award 构造、边界、溢出、所有权与确定性单元测试 | 已验证纯内存配置对象；没有 Draw/Result，INV-03 和业务 SLO 尚未验证 |
| 第 18 节持久化前置 | 两表 DDL、约束、latest 2/dirty、应用只读权限与 Compose 授权启动门 | 已验证 Schema/权限，不含 Repository、业务写入、抽奖执行或业务 SLO |
| 第 19 节仓储前置 | 父子原子 Create、RR/read-only FindByID、恢复校验、并发/取消、1205、执行计划与精确权限 | 已在隔离 MySQL 8.4.11 验证；尚无算法、HTTP 业务负载、Draw/Result 或业务 SLO |
| 第 20 节算法前置 | 精确整数桶、完整 `uint64`、crypto rejection、错误边界、并发路径和本机微基准 | 已验证纯内存 selector 与随机 adapter；不含 HTTP、SQL、Draw/Result、幂等重试或业务 SLO |
| 第 21 节临时 API 前置 | feature gate、最小 DTO、两表 SELECT-only、Nginx→Go→MySQL→CryptoSource、网关故障、数据指纹与有界并发 | 已在隔离 Compose 验证 64 个请求、最大并行 16；没有定速 RPS/延迟分位、正式 Draw、认证、幂等、React 联调或业务 SLO |
| 第 22 节 React 消费者前置 | 同源 bodyless POST、运行时 DTO 解码、pending 抑制、取消与旧响应隔离、桌面/移动工作台、真实浏览器与前端质量门 | 已验证 ephemeral API 的真实可见消费者；没有正式 Draw、认证、RBAC、自动重试、持久化结果或业务 SLO，其余工作台仍是显式 Mock/本地状态 |
| 第 23 节规则边界前置 | 32 条需求、事实/决定所有权、失败分类、版本与解释约束、精确文档白名单及相对第 22 节的 runtime negative diff | 已验证需求与停止线；没有 Rule/责任链/规则树、资格、权限、Draw 或运行时性能证据 |
| M1 · 第 24 节 | Lottery Strategy 读取投影 cache-aside、最小 ACL、poison/依赖故障恢复与三组 50 RPS×10s source-load 基线 | 已在单次本地隔离 Compose 验证；三组均 500/500 成功，warm-cache MySQL execute=0，另两组=1000；不是正式 Draw、业务 SLO、长稳或生产容量 |
| M2 · 第 45 节 | 活动参与、库存、锁和幂等报告 | 待验证 |
| M3 · 第 61 节 | 积分、优惠券、返利与权益闭环报告 | 待验证 |
| M4 · 第 77 节 | Feed、事件接收与分析水位报告 | 待验证 |
| M5 · 第 85 节 | 分布式峰值和分片依据 | 待验证 |
| M6 · 第 93 节 | MCP 权限、审计和网关开销 | 待验证 |
| M7 · 第 101 节 | 12 小时稳定性、灾备和故障演练报告 | 待验证 |

第 24 节缓存证据可追溯到[课程](../course/part-03/lesson-24-redis-strategy-cache.md)、[API](../api/lessons/lesson-24.md)、[QA](../qa/lessons/lesson-24.md)、[设计手记](../design-thinking/lessons/lesson-24.md)、[面试问答](../interview/lessons/lesson-24.md)、[运维手册](../runbooks/redis-strategy-cache.md)与 [ADR-0020](../decisions/ADR-0020-lottery-strategy-cache-aside.md)。

## 11. 变更触发条件

出现以下情况时必须重新评审本基线：

- 真实业务规模、请求比例或数据保留要求发生变化；
- 外部支付、券或 LLM 依赖改变端到端体验；
- 压测证明当前目标不合理或成本不可接受；
- 发生数据丢失、重复发放、越权或长时间恢复事故；
- 合规要求改变审计、隐私、加密或数据驻留边界；
- 上下文被拆为独立服务，统计口径随之变化。

目标变更需要写清业务原因、影响和新验证计划；具体技术方案形成长期约束时新增 ADR。
