# 课程分支检查点

**状态：** 当前

**更新日期：** 2026-08-29

本页把课程章节与可检出的 Git 分支对应起来，方便在 `main` 保持原状的同时，逐节查看实现是怎样演进的。分支记录的是学习检查点，不替代[课程状态台账](status.csv)；章节只有具备正文、API 记录、实际 QA 证据并通过质量门禁后，才能在台账中标记“已完成”。从第 13 节开始，稳定分支还必须包含对应的第一性原理设计手记与面试问答；历史章节将按这一新增规范逐节回填。

## 当前检查点

| 检查点 | 分支 | 起点 | 已确认提交 | 2026-08-29 可用位置 | 学习重点 |
| --- | --- | --- | --- | --- | --- |
| 第 9 节：仓库工程基线 | `codex/lesson-09-repository-baseline` | `main` | `e5516b1` | 本地与 `origin` | 单 Go module、目录职责、文档与统一质量门禁 |
| 第 10 节：模块化单体 | `codex/lesson-10-modular-monolith` | 第 9 节 | `2a8f126` | 本地与 `origin` | 单进程边界、模块依赖规则与拆分条件 |
| 工程基线加固 | `codex/engineering-baseline-hardening` | 第 10 节 | `ce7f8a0` | 本地与 `origin` | module 身份、测试隔离、文档镜像安全与领域占位边界 |
| 第 11 节：Gin HTTP 服务 | `codex/lesson-11-gin-http-service` | 工程基线加固 | 实现 `ade1fad`；验收 tip `4734706` | 本地与 `origin` | Gin 进程、健康接口、HTTP Server 生命周期与优雅关闭 |
| 第 12 节：配置、日志与错误 | `codex/lesson-12-config-logging-errors` | 第 11 节已验收 tip | 实现 `60a6116`；验收 tip `ac9ad0e` | 本地与 `origin` | `GROWTHOS_` 配置、`slog`、请求关联、fault 与 HTTP 错误 |
| 第 13 节：MySQL 与 Migration | `codex/lesson-13-mysql-migrations` | 第 12 节验收 tip `ac9ad0e` | 实现 `b3f5aa7`；加固 `b734463`；完整章节以最终分支 tip 为准 | 本地与 `origin` | 身份隔离、`sqlx` pool、readiness、前向 Migration 与真实 MySQL 验收 |

第 11 节可运行实现固定在 `ade1fad`，配套课程和 QA 文档紧随其后提交。完整章节以 `origin/codex/lesson-11-gin-http-service` 的 tip 为准；这种“实现提交 + 验收文档提交”的顺序既能精确引用代码，也方便逐步比较。

第 12 节可运行实现固定在 `60a6116`，完整验收 tip 为 `ac9ad0e`。第 13 节先在 `b3f5aa7` 完成实现，再由交叉审查在 `b734463` 加固 timeout、dirty/version mismatch、取消 terminal、readiness 响应预算和显式集成门禁。学习实现细节时可依次比较这两个提交，学习完整章节时以 `origin/codex/lesson-13-mysql-migrations` 的最终文档 tip 为准。

## 稳定章节分支与累计快照

课程同时保留两类用途不同的分支：

| 类型 | 分支规则 | 是否移动 | 当前事实 |
| --- | --- | --- | --- |
| 单节稳定分支 | `codex/lesson-XX-...` | 一节验收结束后保持稳定 | 第 13 节实现与加固已推送；最终文档提交后冻结同名远端 tip |
| 最新累计快照 | `codex/complete-implementation` | 每节验收后快进 | 第 13 节最终文档提交和门禁通过后快进至该节稳定分支 tip |

学习单节变化时使用对应稳定分支；想直接查看目前全部已验收实现时使用 `codex/complete-implementation`。累计分支只做 fast-forward，不替代每节固定检查点，也不能因为代码已合入工作树就提前代表“已验收”。

## 历史章节分支

| 章节 | 分支 | 检查点提交 | 2026-08-29 可用位置 | 说明 |
| --- | --- | --- | --- | --- |
| 第 1 节 | `codex/lesson-01-growth-platform` | `1653614` | 仅本地 | 严格完整检查点与第 2 节共享，见下方历史说明 |
| 第 2 节 | `codex/lesson-02-user-growth-journey` | `1653614` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 3 节 | `codex/lesson-03-operator-workflow` | `eb538d4` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 4 节 | `codex/lesson-04-ai-operator-workflow` | `788b7f6` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 5 节 | `codex/lesson-05-event-storm` | `16f0f19` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 6 节 | `codex/lesson-06-bounded-contexts` | `9b5507e` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 7 节 | `codex/lesson-07-non-functional-requirements` | `c283ea1` | 仅本地 | 正文、QA、API 与索引完整 |
| 第 8 节 | `codex/lesson-08-system-design-v0` | `6e70006` | 本地与 `origin` | 系统设计 V0 完整检查点 |
| 第 14 节 | `codex/lesson-14-react-frontend-framework` | `d8eacb8` | 仅本地 | 前端实现提交之后补齐章节 API 治理的完整检查点 |

“仅本地”不是遗漏标记：这些分支通过 HTTPS 推送时，当前 OAuth 凭据因历史中包含 workflow 变更而被 GitHub 拒绝。权限问题解决且实际推送成功前，不得把它们写成远端分支。

第 1 节在更早提交已经有正文、QA 和完成状态，但当时尚未建立章节 API 记录体系；第 2 节提交才补齐第 1 节 API。因此严格采用“正文 + QA + API + 索引”口径时，第 1、2 节分支会指向同一提交。第 14 节的前端实现先落在 `5f8a75b`，API 治理随后在 `d8eacb8` 补齐；学习时可以把这两个提交分别理解为实现与文档治理。

## 建议学习顺序

先刷新远端引用，再按下面的顺序切换：

```bash
git fetch origin
git switch codex/lesson-09-repository-baseline
git switch codex/lesson-10-modular-monolith
git switch codex/engineering-baseline-hardening
git switch codex/lesson-11-gin-http-service
git switch codex/lesson-12-config-logging-errors
git switch codex/lesson-13-mysql-migrations
```

如果本地还没有某个**已经确认存在于远端**的课程分支，可从对应远端分支创建跟踪分支：

```bash
git switch --track origin/codex/lesson-11-gin-http-service
git switch --track origin/codex/lesson-12-config-logging-errors
git switch --track origin/codex/lesson-13-mysql-migrations
```

每一节建议先看提交摘要，再比较与上一检查点的差异：

```bash
git log --oneline --decorate --graph --max-count=20
git diff --stat codex/engineering-baseline-hardening..codex/lesson-11-gin-http-service
git diff codex/engineering-baseline-hardening..codex/lesson-11-gin-http-service
git diff --stat codex/lesson-11-gin-http-service..codex/lesson-12-config-logging-errors
git diff codex/lesson-11-gin-http-service..codex/lesson-12-config-logging-errors
git diff --stat codex/lesson-12-config-logging-errors..codex/lesson-13-mysql-migrations
git diff codex/lesson-12-config-logging-errors..codex/lesson-13-mysql-migrations
```

第 11 节的直接起点是工程基线加固，而不是 `main`。这样比较只会显示本节新增的 HTTP 服务、测试和配套文档；若直接与 `main` 比较，还会混入第 9、10 节和工程加固的变化。

第 12 节直接基于第 11 节已验收 tip，完整验收 tip 为 `ac9ad0e`。第 13 节直接基于 `ac9ad0e`；实现 `b3f5aa7` 和交叉审查加固 `b734463` 已推送至 `origin/codex/lesson-13-mysql-migrations`，最终文档提交后以该远端分支 tip 作为完整学习检查点。

## 分支使用约束

1. 每个实现章节使用独立的 `codex/lesson-XX-...` 分支；非课程章节但会改变后续起点的加固工作使用独立命名分支。
2. 下一节从最近一个已验收检查点创建，保留线性、可比较的学习路径。
3. 一个章节可有多个小提交，但最终必须有一个通过完整门禁的分支 tip；QA 记录对应的验证命令、环境和结果，设计手记记录推导与风险，面试问答记录可口述答案和来源边界。
4. 不在 `main` 上提前合并后续实现。学习者可按需 cherry-pick、手工重做或比较差异。
5. 分支已推送不等于章节已完成；以 `status.csv` 和 QA 实测证据为准。

## 后续登记模板

新增章节分支时，在“当前检查点”追加一行，并记录：章节、完整分支名、直接起点、已验收提交或分支 tip、学习重点。若分支被重建或强制移动，必须同步更新本页和对应 QA，避免学习者比较到错误版本。
