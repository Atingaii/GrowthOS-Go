# 第 23 节 QA：Lottery 规则需求、所有权与零代码漂移验收

- **章节：** [第 23 节：需求升级——抽奖策略开始需要规则](../../course/part-03/lesson-23-lottery-strategy-rule-requirements.md)
- **需求基线：** [Lottery 规则需求 v1](../../product/lottery-rule-requirements-v1.md)
- **架构决策：** [ADR-0019](../../decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md)
- **API 记录：** [第 23 节 API](../../api/lessons/lesson-23.md)
- **基准提交：** `1f95779277b1ea882d607a59e0fd2c475f58bd7a`（第 22 节已验收 tip）
- **验收日期：** 2026-08-30，Asia/Shanghai
- **当前记录状态：** 已验收；内容提交 `479947b` 在干净工作树上通过精确白名单、运行时负向 diff 与完整回归，封板提交只登记证据和课程检查点

> 本 QA 验收的是一节“需求升级与边界冻结”交付。通过条件不是出现更多类、表或接口，而是 32 条规则需求连续、每类判断只有一个决定所有者且原始事实提供方明确、失败语义不混淆、后续章节有明确承接，并且相对第 22 节 tip 没有任何运行时代码、API、Migration、Redis、前端或权限实现漂移。概念正反例属于需求评审，不冒充运行时自动化测试。

## 1. 验收范围

### 1.1 应当新增或更新

- 一份 Lottery 规则需求基线 v1；
- 一份规则所有权与求值边界 ADR；
- 第 23 节课程、API、QA、设计手记和面试问答；
- 必要的课程状态、章节索引和当前进度说明；
- 对 M1 与未来章节的诚实边界修订。

### 1.2 明确不得变化

- `cmd/**`、`internal/**`、`pkg/**` 生产/测试代码；
- `web/**` 前端代码、依赖和构建配置；
- `migrations/**` 及 latest version；
- `configs/**`、`deploy/**`、`scripts/**`、`.github/**`；
- `go.mod`、`go.sum`、`Makefile`；
- HTTP route、DTO、status/code、feature gate 与 Nginx 规则；
- Redis client、key、TTL、ACL、网络、readiness、锁或 Lua；
- 认证、会话、RBAC、对象级授权与前端权限裁剪；
- Strategy/Award/WeightedSelector/Repository 的运行时模型；
- 正式 Draw、Result、库存、积分和 Benefit 事实。

## 2. 验收命题与证据矩阵

| 验收命题 | 正向证据 | 反向证据 | 结果填写规则 |
| --- | --- | --- | --- |
| 复合需求已被拆解 | `LRR-001`～`LRR-032` 连续目录与规则分类 | 不允许只有“以后加规则引擎”一句话 | ID/内容检查通过后填写 |
| 决定所有权唯一且事实来源可解释 | Marketing、Participation、Lottery、Benefit、Governance 决定矩阵；原始事实提供方单列 | 不允许万能 `Rule` 抹平上下文语言或把 provider 当 owner | 人工矩阵评审后填写 |
| selector 边界没有膨胀 | `LRR-012` 与 ADR | `WeightedSelector` 不得出现用户/活动/库存/权限依赖 | Git negative diff 通过后填写 |
| 失败语义可区分 | Reject、Authorization、Unavailable、Technical failure、Unknown、`no_reward` 分离 | 不允许依赖失败降级未中奖 | 正反例评审后填写 |
| 版本概念可演进 | RuleSet/config schema/Strategy/app version 分离 | 不用 `updated_at` 或 Git SHA 冒充全部版本 | 文档交叉检查后填写 |
| Redis 顺序诚实 | 第 24 节只考虑可重建 Strategy 读投影 | 不缓存资格、规则 verdict、ephemeral result | 章节映射评审后填写 |
| 权限顺序诚实 | `LRR-031` 与第 31～35 节映射 | 不在第 23 节增加角色 if/隐藏菜单 | Git/API negative diff 通过后填写 |
| 当前能力没有被夸大 | Course/API/QA 都声明零运行时变化 | 不宣称规则已执行 | 文案扫描与人工评审后填写 |
| 全仓原行为不回归 | `make verify` | 不能用文档合理性替代既有测试 | 最终实测后填写 |

## 3. 文档产物存在性

最终分支必须存在：

```text
docs/product/lottery-rule-requirements-v1.md
docs/decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md
docs/course/part-03/lesson-23-lottery-strategy-rule-requirements.md
docs/api/lessons/lesson-23.md
docs/qa/lessons/lesson-23.md
docs/design-thinking/lessons/lesson-23.md
docs/interview/lessons/lesson-23.md
```

可执行检查：

```bash
for lesson23_doc in \
  docs/product/lottery-rule-requirements-v1.md \
  docs/decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md \
  docs/course/part-03/lesson-23-lottery-strategy-rule-requirements.md \
  docs/api/lessons/lesson-23.md \
  docs/qa/lessons/lesson-23.md \
  docs/design-thinking/lessons/lesson-23.md \
  docs/interview/lessons/lesson-23.md
do
  test -s "$lesson23_doc" || exit 1
done
```

这只证明文件存在且非空；内容正确性由后续检查承担。

## 4. 32 条需求连续性验收

需求基线使用 `LRR-001`～`LRR-032`。先生成期望集合，再与文档中实际出现的全部 ID 比较；不去重，因此重复编号也会让 diff 失败：

```bash
diff -u \
  <(awk 'BEGIN { for (i = 1; i <= 32; i++) printf "LRR-%03d\n", i }') \
  <(rg -o 'LRR-[0-9]{3}' docs/product/lottery-rule-requirements-v1.md \
    | LC_ALL=C sort)
```

通过条件：`diff` 无输出并 exit 0。它能发现缺号、重复和越界 ID；不能证明每条需求内容合理。

## 5. 需求质量逐条评审

每条 `LRR-*` 必须至少回答“目的、事实、输出、失败、证据”中的适用项。按六组检查：

### 5.1 场景与产品语义：`LRR-001`～`LRR-007`

- 前置门控没有完成时不得调用 selector；
- 新用户定义来自权威事实，不信任前端标记；
- 会员路由输出明确目标并有显式缺省策略；
- `no_reward` 只来自显式 `no_reward` Award；
- Award 不可分配时不能默认重归一化、补抽或转移权重；
- reward 候选不等于已扣库存/已发放；
- 没有正式业务身份时不能承诺安全重试或结果查询。

### 5.2 确定性与顺序：`LRR-008`～`LRR-012`

- 除终端随机票据外，同版本、同事实快照、同评估时刻得到同一决定；
- 顺序和原因优先级是版本化业务契约；
- 确定拒绝默认短路，不继续消费随机或制造副作用；
- 内部诊断与用户可见单一原因分层；
- `WeightedSelector` 保持无用户、Activity、库存和权限依赖。

### 5.3 事实质量：`LRR-013`～`LRR-017`

- 外部事实登记来源、采集时间、新鲜度和 unknown 处理；
- 缓存是可重建投影，不是权威事实；
- 时间窗使用受控服务端时刻和显式业务时区；
- 一次性额度/库存不能用陈旧布尔读取保证；
- 跨上下文副本不能反向修改权威事实。

### 5.4 版本、解释和审计：`LRR-018`～`LRR-022`

- RuleSet、schema、Strategy 与应用发布版本概念分离；
- RuleCode/ReasonCode 与展示文案分离；
- trace 按顺序保留最小解释，不收集随机材料、完整 PII 或风险特征；
- 正式 Draw 出现后能关联当时配置/事实；
- 用户解释、运营诊断和安全审计使用不同披露级别。

### 5.5 失败、安全和恢复：`LRR-023`～`LRR-028`

- Reject、未认证、无权限、Unavailable、Technical failure、Unknown 分离；
- 关键依赖默认失败关闭；
- 服务端收集可信事实，客户端只提交意图/必要标识；
- timeout/取消不触发静默重新随机；
- 规则图启用前校验根、边、环、深度、不可达、缺省和确定性；
- 公开错误不泄漏 SQL、错误链、凭据、完整输入、阈值或随机材料。

### 5.6 演进和治理：`LRR-029`～`LRR-032`

- 新规则先登记目的、事实所有者、决策所有者、失败与顺序；
- Authorization、Eligibility 和 Inventory 不共用万能规则模型；
- 第 31～35 节提供公共访问控制，业务代码不硬编码 `admin`；
- 责任链、树、DSL 或第三方引擎只能由真实复杂度触发。

## 6. 规则所有权验收

| 阶段 | 决定所有者 | 原始事实提供方 | 可接受输出 | 禁止越界 |
| --- | --- | --- | --- | --- |
| Activity 门控 | Marketing | Activity 发布快照与受控服务端时钟 | continue / business reject | Strategy 不拥有发布与时间窗 |
| Participation 资格 | Participation | 外部用户/会员映射、Participation 账户/流水、受控风险 verdict | continue / business reject / technical failure | React 不提交最终资格 verdict |
| Lottery 路由/决策 | Lottery | Lottery 发布配置与受控事实快照 | route / reject / technical failure | 不读取随机桶决定会员等级 |
| 候选可分配 | Benefit | Benefit 内部库存子能力、权益模板与预占事实 | available / unavailable / unknown | 不把配置行存在当作库存承诺 |
| 终端选择 | Lottery | Strategy/Award 配置与 bounded random source | reward / no_reward candidate | 不关心 User/Activity/权限/库存 |
| 正式 Draw/Result | Lottery | 已确认参与身份、规则/Strategy 快照与选择事实 | committed / unknown | ephemeral response 不冒充最终结果 |
| 发放与补偿 | Benefit | Draw/Reward 引用、发放流水与外部权益回执 | processing / delivered / compensating / failed | reward 候选不等于发放成功 |
| 访问控制 | Governance | 身份/IAM 适配、访问策略、资源/动作/范围 | authenticated / authorized / denied | 不与 eligibility 原因混用 |

人工评审问题：

1. 是否有一条规则同时被两个上下文声称为可修改真相；
2. 是否有消费方把缓存副本或客户端字段当权威事实；
3. 是否把技术 library 的复用接口误写成统一业务语言；
4. 是否允许访问控制读取或改变业务资格结果；
5. 是否让业务拒绝暴露角色策略、风控阈值或受保护对象存在性。

任一问题回答“是”即不通过。

## 7. 正向场景评审

这些场景用于证明需求语言足以区分路径，不表示已有代码会执行它们。

### 7.1 合法 `no_reward`

```text
Given Activity 已发布且在有效窗口
And 用户资格、次数与风险判断全部通过
And 会员路由得到一个合法 Strategy
And 候选可分配策略允许继续
When WeightedSelector 选中显式 no_reward Award
Then 结果类别是正常 no_reward
And 不是 eligibility reject
And 不是 authorization denial
And 不是 technical failure
```

### 7.2 明确业务拒绝

```text
Given 当前主体已通过未来授权检查
And Activity 已过期
When 执行 Activity gate
Then 返回稳定业务拒绝原因
And 不读取不必要的用户/库存事实
And 不调用 WeightedSelector
And 不把结果写成 no_reward
```

### 7.3 明确路由

```text
Given 会员事实来自可信来源且满足新鲜度
And 当前规则集版本固定
When 执行会员分层路由
Then 输出明确 Strategy target 或显式缺省分支
And 不依赖 map/SQL 偶然顺序
```

## 8. 负向场景与失败语义

### 8.1 Authorization 不等于 Eligibility

```text
主体没有读取/参与该 Activity 的权限
```

期望：进入授权拒绝；不得继续运行新人规则，也不得用“不是新用户”隐藏真实授权问题。公开输出还要遵循最小披露，不能确认受保护 Activity 是否存在。

### 8.2 Reject 不等于 `no_reward`

```text
用户已用完参与次数
```

期望：稳定业务拒绝；不得构造一个 `no_reward` Award、消费随机票据或计入未中奖概率。

### 8.3 Technical failure 不等于 Reject

```text
新人事实源 timeout / 返回无法验证的版本
```

期望：技术失败或 unknown，默认失败关闭；不得把缺失事实解释成“不是新人”，也不得默认放行。

### 8.4 Unavailable 不等于 `no_reward`

```text
加权选择前发现 reward Award 无法分配
```

期望：要求版本化产品策略决定排除、拒绝或其他处理；在策略未冻结前不得静默重归一化、补抽或把权重转给 `no_reward`。

### 8.5 Unknown 不等于可以重新随机

```text
未来正式 Draw 已产生副作用，但响应在客户端可见前丢失
```

期望：通过唯一业务身份查询/恢复；不得把 timeout 当作“没有执行”并重新随机。当前 ephemeral API 没有这种身份，因此继续明确不可安全恢复。

### 8.6 前端声明不等于可信事实

```json
{
  "is_new_user": true,
  "member_tier": "gold",
  "role": "admin",
  "award_available": true
}
```

期望：服务端不能把这些最终 verdict 当权威输入；当前 ephemeral route 仍不接受 body。

## 9. 未来章节映射验收

| 章节 | 只允许承接的增量 | 第 23 节不能提前完成 |
| --- | --- | --- |
| 24 | 可重建 Strategy 读投影缓存 | 用户资格/verdict/ephemeral result 缓存 |
| 25 | 第一条真实用户资格规则 | 通用责任链/规则树 |
| 26 | 线性短路责任链 | 声称已适合任意决策图 |
| 27 | 用真实失败暴露线性链局限 | 为展示模式而制造假复杂度 |
| 28 | 规则树 schema 与图校验 | “JSON 存得进库就能执行” |
| 29 | 决策执行与解释 | 把技术失败降级为业务拒绝 |
| 30 | Activity/Strategy 正式边界 | Strategy 吞并 Activity lifecycle |
| 31～35 | 统一访问控制模型、会话、服务端强制、前端权限/能力投影、越权 E2E | 页面级角色 if 代替授权 |
| 39～45 | 账户、订单、库存与完整闭环 | 当前 reward 候选冒充兑付 |
| 46～52 | 结果快照、可靠交付与补偿 | 当前 ephemeral response 冒充可恢复结果 |

检查重点：每一项都是未来计划而非“已完成”表述；第 24 节不会因紧邻规则需求而缓存高风险用户决策；公共权限不会被临时塞入 Lottery 规则接口。

## 10. 精确变更白名单

只有下列 tracked 文件允许相对第 22 节 tip 发生变化；允许列表包含可能需要的进度/索引文件，实际集合可以是其子集：

```text
README.md
docs/README.md
docs/api/README.md
docs/api/lessons/README.md
docs/api/lessons/lesson-23.md
docs/architecture/repository-map.md
docs/course/README.md
docs/course/branch-checkpoints.md
docs/course/route-revisions.md
docs/course/status.csv
docs/course/part-03/lesson-23-lottery-strategy-rule-requirements.md
docs/decisions/README.md
docs/decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md
docs/design-thinking/README.md
docs/design-thinking/lessons/lesson-23.md
docs/frontend/frontend-architecture.md
docs/interview/README.md
docs/interview/lessons/lesson-23.md
docs/product/bounded-context-map-v1.md
docs/product/non-functional-requirements-v1.md
docs/product/product-brief.md
docs/product/system-design-v0.md
docs/product/lottery-rule-requirements-v1.md
docs/qa/README.md
docs/qa/lessons/lesson-23.md
```

提交完成且工作树干净后执行：

```bash
lesson23_base=1f95779277b1ea882d607a59e0fd2c475f58bd7a
lesson23_unexpected_paths="$(
  comm -23 \
    <(git diff --name-only "$lesson23_base"..HEAD | LC_ALL=C sort -u) \
    <(printf '%s\n' \
      README.md \
      docs/README.md \
      docs/api/README.md \
      docs/api/lessons/README.md \
      docs/api/lessons/lesson-23.md \
      docs/architecture/repository-map.md \
      docs/course/README.md \
      docs/course/branch-checkpoints.md \
      docs/course/route-revisions.md \
      docs/course/status.csv \
      docs/course/part-03/lesson-23-lottery-strategy-rule-requirements.md \
      docs/decisions/README.md \
      docs/decisions/ADR-0019-lottery-rule-ownership-and-evaluation-boundaries.md \
      docs/design-thinking/README.md \
      docs/design-thinking/lessons/lesson-23.md \
      docs/frontend/frontend-architecture.md \
      docs/interview/README.md \
      docs/interview/lessons/lesson-23.md \
      docs/product/bounded-context-map-v1.md \
      docs/product/non-functional-requirements-v1.md \
      docs/product/product-brief.md \
      docs/product/system-design-v0.md \
      docs/product/lottery-rule-requirements-v1.md \
      docs/qa/README.md \
      docs/qa/lessons/lesson-23.md \
      | LC_ALL=C sort -u)
)"
test -z "$lesson23_unexpected_paths"
test -z "$(git status --porcelain)"
```

通过条件：没有 unexpected path，且工作树干净。若封板时确实需要增加另一个索引，必须先说明理由并更新本 QA 的精确白名单，不能为了让命令变绿改成任意 `docs/**`。

## 11. 运行时代码负向 diff

白名单防止意外文件进入章节，下面的独立命令进一步证明所有非文档 runtime 路径没有 tracked diff：

```bash
lesson23_base=1f95779277b1ea882d607a59e0fd2c475f58bd7a

git diff --exit-code "$lesson23_base" -- \
  .github Makefile cmd configs deploy go.mod go.sum internal migrations pkg scripts web
```

通过条件：无输出且 exit 0。它覆盖：

- Go production/test code；
- React/TypeScript/CSS/test/package lock；
- Migration 与 embedded files；
- Compose/Docker/Nginx/Secret 边界；
- config、CI、scripts 和依赖图。

该命令只检查 tracked 内容，所以必须与上一节的“提交后 + clean worktree”条件组合；否则 untracked runtime 文件可能被遗漏。

## 12. API、Migration、Redis 与权限负向验收

### 12.1 HTTP/API

```bash
git diff --exit-code "$lesson23_base" -- cmd internal web deploy configs
```

必须证明没有 route、DTO、error mapping、adapter、decoder、页面或 Nginx 变化。现有 API surface 见 [第 23 节 API 记录](../../api/lessons/lesson-23.md)。

### 12.2 Migration

```bash
git diff --exit-code "$lesson23_base" -- migrations
find migrations/sql -maxdepth 1 -type f -name '*.sql' -print | LC_ALL=C sort
```

期望 SQL 仍只有：

```text
migrations/sql/000001_create_lottery_strategy.up.sql
migrations/sql/000002_create_lottery_strategy_award.up.sql
```

不允许 `000003`、rule/activity/participation/audit 表或对既有历史 Migration 的改写。

### 12.3 Redis 与依赖

```bash
git diff --exit-code "$lesson23_base" -- go.mod go.sum configs deploy internal cmd
```

必须没有 Redis client dependency、配置、网络、key、TTL、readiness、锁或 Lua 变化。第 24 节的计划不能倒灌为第 23 节事实。

### 12.4 权限

```bash
git diff --exit-code "$lesson23_base" -- internal cmd web configs deploy
```

必须没有会话、Subject/Role/Permission runtime model、RBAC middleware、前端 role if、路由 guard 或越权测试。文档提到第 31～35 节不等于权限已经实现。

## 13. 文案真实性扫描

自动扫描只能发现一部分高风险用语，仍需人工结合上下文判断：

```bash
rg -n \
  '已实现规则引擎|已上线资格|已接入规则缓存|已完成 RBAC|已持久化 Draw|已保证 exactly-once' \
  docs/course/part-03/lesson-23-lottery-strategy-rule-requirements.md \
  docs/api/lessons/lesson-23.md \
  docs/qa/lessons/lesson-23.md \
  docs/design-thinking/lessons/lesson-23.md \
  docs/interview/lessons/lesson-23.md
```

期望：没有把这些短语用于正向能力声明；如果出现在“不能说”或反例中，人工确认语境后记录，不机械要求零匹配。

人工检查所有完成式动词：

- “冻结/分析/映射/决定”可以描述本节文档交付；
- “执行/校验/路由/缓存/鉴权/发放”只能描述既有 selector 或明确的未来/概念场景；
- “验证”必须说明是文档评审、静态 diff 还是运行时测试；
- 规划章节不能使用“服务已经”“接口返回”“数据库保存”等当前事实语气。

## 14. 文档与全仓门禁

最终封板依次执行：

```bash
make doc-check
git diff --check
make verify
```

2026-08-30 最终实测记录：

| 命令 | 预期 | 最终实测 | 证据边界 |
| --- | --- | --- | --- |
| `make doc-check` | exit 0 | exit 0，`documentation checks passed` | 章节注册、完成证据和 Markdown 链接 |
| `git diff --check` | exit 0 | exit 0，无输出 | whitespace/conflict marker 基础检查 |
| `make verify` | exit 0 | exit 0；Go vet/test、Web 19 个文件 152 条测试、typecheck 与 production build 全通过 | 既有 Go/Web/文档门禁无回归 |
| 精确白名单 | 无 unexpected path | `479947b` 上无 unexpected path，工作树干净 | 章节只修改批准文档/索引 |
| runtime negative diff | exit 0 | 相对 `1f95779` exit 0，无输出 | 相对 Lesson 22 无 tracked runtime 变化 |
| LRR ID diff | exit 0 | exit 0，32 个编号完整且无重复 | `LRR-001`～`LRR-032` 完整且无重复 |

如果任一命令未执行、因环境跳过或失败，必须如实记录，不能因为本节没有代码就默认通过。

## 15. 为什么本节不启动 Docker 或浏览器

本节没有改动容器、数据库、Redis、HTTP、React 或浏览器交互。重复启动 Compose、播种 Strategy 或截图无法证明规则所有权合理，只会产生与验收命题无关的副作用。

因此：

- 复用第 22 节已经封板的运行时测试作为回归基线；
- 用 `make verify` 检查既有自动化没有回归；
- 用 Git negative diff 证明运行时没有变化；
- 用需求正反例和所有权矩阵验收新增内容；
- 不声称做过第 23 节规则 E2E、性能、故障注入或浏览器验收。

真实规则运行出现于后续实现章节后，才增加对应单元、集成、契约、数据库和浏览器证据。

## 16. 清理与保留

本节预期只创建 Markdown/CSV 索引变化，不生成构建产物、浏览器截图、Docker project、数据库 volume 或下载资产。

验收脚本使用 shell process substitution 比较 ID/path，不创建一次性文件。应长期保留：

- 第 23 节分支及其小步 Git 提交；
- 需求基线、ADR、课程、API、QA、设计手记和面试问答；
- 最终命令结果与基准 commit；
- 第 22 节既有可复用测试和依赖缓存。

不得删除或修改用户已有 Docker Desktop 中的 MySQL、Redis、RabbitMQ、PostgreSQL 容器/volume，也不得为了“清理”删除复用依赖缓存。

## 17. 本证据能证明什么

当全部门禁实际通过后，可以证明：

- `LRR-001`～`LRR-032` 需求编号完整；
- 规则阶段、决定所有权、原始事实来源、输出和失败语义已经文档化；
- 已有 `WeightedSelector` 的纯终端职责没有被本节代码改写；
- Authorization、Eligibility、Unavailable、Reject、Technical failure、Unknown 与 `no_reward` 在需求层明确分离；
- 后续 24～52 节有可追踪的演进输入；
- 相对第 22 节 tip 没有 tracked runtime/API/Migration/Redis/frontend/permission change；
- 既有全仓测试和文档门禁没有回归。

## 18. 本证据不能证明什么

即使所有门禁通过，也不能证明：

- 任何规则会在运行时执行；
- 新人、会员、次数或风控判断正确；
- 责任链/规则树/DSL/DMN 已实现；
- Activity、Participation、正式 Draw 或 Result 已存在；
- RuleSetVersion/StrategyVersion 已发布或持久化；
- Redis cache、锁、限流或规则缓存已接入；
- 登录、RBAC、对象级授权或前端裁剪已实现；
- 库存预占、订单、积分扣减、Benefit 发放或补偿已完成；
- 规则吞吐、P99、公平性、合规性或 exactly-once 已验证；
- 文档中的概念正反例等于自动化 E2E。

## 19. 剩余风险

1. 需求 v1 是当前业务假设的基线，真实运营规则可能改变优先级、披露和事实新鲜度要求；
2. 尚无 Activity/User/Participation 实体，部分事实来源仍是未来契约而非项目实测；
3. 候选不可用后的排除、拒绝、重归一化或补偿策略刻意未决定；
4. RuleSetVersion 与 Strategy version 尚无持久化模型；
5. 线性责任链是否足够，必须由第 26～27 节真实规则与失败暴露，而不是本节推测；
6. 访问控制虽已有明确排期，当前所有工作台仍没有真实认证授权；
7. 第 24 节若在没有版本/失效证据时过度缓存，仍可能把派生数据误当事实源；
8. 文档一致不保证未来实现不会偏离，因此每一后续章节都要回链到 `LRR-*` 和 ADR。

## 20. 验收结论

第 23 节已在内容提交 `479947b` 上完成精确白名单、干净工作树、运行时负向 diff、需求编号、文档和全仓回归验收；Migration 集合仍只有 `000001` 与 `000002`。封板提交只增加本页实测记录与课程检查点，并在提交后再次执行同一组静态门禁。

实际满足：

1. 32 条需求连续且通过内容评审；
2. 所有权与失败语义正反例无冲突；
3. 精确白名单没有意外路径；
4. runtime/API/Migration/Redis/frontend/permission negative diff 全部 exit 0；
5. `make doc-check`、`git diff --check`、`make verify` 实际 exit 0；
6. 分支提交、远程 tip 与章节检查点一致。

因此可以准确结论为：

> 第 23 节完成 Lottery 规则需求与求值边界冻结，并以精确变更白名单和相对 Lesson 22 tip 的负向 diff 证明没有提前引入规则引擎、API、Migration、Redis、前端或权限运行时实现。
