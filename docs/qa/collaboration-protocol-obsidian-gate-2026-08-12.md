# Obsidian 最终同步门禁与 .ccb Git/doccheck 排除验收记录

- **日期：** 2026-08-12
- **对象：** `.gitignore`、`cmd/doccheck` 及其测试、`docs/standards/collaboration-protocol.md`、`docs/standards/obsidian-sync.md`、`docs/standards/definition-of-done.md`、`CONTRIBUTING.md`、`docs/README.md`、[ADR-0007](../decisions/ADR-0007-obsidian-sync-gate-and-ccb-exclusion.md)、[ADR-0004 协作协议](../decisions/ADR-0004-collaboration-protocol.md)、本任务 QA 记录
- **类型：** 工具/治理/文档变更评审 + 实际 Obsidian 同步
- **结果：** 通过

## 任务与范围

- **范围**：`.gitignore`、`cmd/doccheck` 及其测试、协作协议正文、Obsidian 同步/贡献/完成定义/索引文档、ADR、QA 记录、`/mnt/e/TencentGo/growthOS` 个人 Vault。
- **非目标**：不推进课程小节或业务功能；不改 docsync 单向镜像语义；不提交 commit；不删除/纳入 `.ccb`。

## 实现摘要

1. `.gitignore` 增加 `/.ccb/`，排除仓库根 CCB 运行时目录（不删除、不纳入 Git）。
2. `cmd/doccheck` 链接检查将 `.ccb` 列入跳过目录，并新增回归测试验证 `.ccb` 下断链不被报告、非 `.ccb` 断链仍被报告。
3. 协作协议新增"Obsidian 最终同步门禁"：每任务在 README/docs/QA 改完后执行 `make docs-sync VAULT=/mnt/e/TencentGo/growthOS`；最终同步为交付前最后镜像步骤；同步命令/结果/一致性证据进交付报告；Codex 缺证据不得验收。
4. 同步门禁同步写入 obsidian-sync/贡献/完成定义/docs 导航，新增 ADR-0007 记录演进并登记索引，回溯修正先前协作协议 QA 的风险状态为已解决。
5. 执行首次实际同步；编写本 QA 记录后执行最终同步并核验 Vault 一致性。

## 变更文件

- `.gitignore`
- `cmd/doccheck/main.go`（跳过 `.ccb`）
- `cmd/doccheck/main_test.go`（回归测试）
- `docs/standards/collaboration-protocol.md`（新增同步门禁章节）
- `docs/standards/obsidian-sync.md`（新增门禁说明）
- `docs/standards/definition-of-done.md`（新增同步证据条目）
- `CONTRIBUTING.md`（新增同步命令）
- `docs/README.md`（新增同步导航）
- `docs/decisions/ADR-0007-obsidian-sync-gate-and-ccb-exclusion.md`（新增）
- `docs/decisions/README.md`（登记 ADR-0007）
- `docs/qa/collaboration-protocol-2026-08-12.md`（回溯修正风险状态）
- `docs/qa/collaboration-protocol-obsidian-gate-2026-08-12.md`（本文件，新增）
- `docs/qa/README.md`（登记本记录）

## 实际执行命令与结果

| 命令 | 环境 | 结果 |
| --- | --- | --- |
| `go test ./cmd/doccheck -run TestCheckMarkdownLinksSkipsCcb -v` | 原工作区 | 通过（`PASS`，验证 `.ccb` 断链被跳过、普通断链被报告） |
| `go test ./...` | 原工作区 | 通过（`cmd/doccheck`、`cmd/docsync` 均 ok） |
| `git check-ignore -v .ccb .ccb/ccb.config` | 原工作区 | `.gitignore:15:/.ccb/ .ccb` 与 `.ccb/ccb.config` 均匹配 |
| `git ls-files .ccb '.ccb/**'` | 原工作区 | 无输出（无任何 `.ccb` 路径被跟踪） |
| `git status --short` | 原工作区 | `.ccb/` 不再出现 |
| `make docs-sync VAULT=/mnt/e/TencentGo/growthOS` | 原工作区 | 通过（`已同步项目 README.md + docs/ -> /mnt/e/TencentGo/growthOS`） |
| 最终同步（docs 定稿后再次执行，同上命令） | 原工作区 | 通过 |
| `make verify` | 原工作区（含 `.ccb`） | 通过（`fmt-check`、`test`、`doc-check`、`web-verify`） |
| Vault 一致性核验 | Vault | 见下方核验清单 |

## 失败与修复

- **历史失败（已闭环）**：此前原工作区 `make verify` 因 `.ccb/` 断链误报失败；本次通过 `.gitignore` 排除 + doccheck 跳过 + 回归测试根治，原工作区 `make verify` 现通过。历史失败记录保留在 [协作协议 QA](collaboration-protocol-2026-08-12.md)，未抹掉。
- **Codex 首轮验收缺口（本任务内发现并修复）**：
  1. `docs/README.md` 存在两条指向同一文件的"Obsidian 同步"导航。已合并为唯一一条，同时表达单向镜像语义与最终同步门禁。
  2. `cmd/doccheck/main_test.go` 新增测试含两条解释性行内注释。已删除，测试命名与断言不变，语义未改，回归测试复验通过。
  3. 协作协议为"未改同步范围文件"保留"执行同步或按可审计规则处理"的豁免，与"每次更改完都同步"冲突。已改为无条件：每个任务无论是否修改同步范围文件，都必须在交付前执行固定同步命令并记录证据，不允许跳过；同步说明、完成定义、贡献指南、ADR-0007 等配套表述已逐一检查并强化一致，无残留豁免措辞。

以上缺口均已修复并复验（见"实际执行命令与结果"与"验收标准映射"）。

## Vault 一致性核验

| 检查项 | 命令 | 结果 |
| --- | --- | --- |
| 新增协议在 Vault 存在且内容一致 | `cmp docs/standards/collaboration-protocol.md /mnt/e/TencentGo/growthOS/standards/collaboration-protocol.md` | 通过 |
| 新增 ADR-0007 在 Vault 存在 | `test -f /mnt/e/TencentGo/growthOS/decisions/ADR-0007-obsidian-sync-gate-and-ccb-exclusion.md` | 通过 |
| 本 QA 记录在 Vault 存在 | `test -f /mnt/e/TencentGo/growthOS/qa/collaboration-protocol-obsidian-gate-2026-08-12.md` | 通过 |
| QA 索引更新在 Vault 存在 | `grep -q collaboration-protocol-obsidian-gate docs/qa/README.md` 与 Vault 对应文件 | 通过 |
| 根 README 镜像为项目README.md | `test -f /mnt/e/TencentGo/growthOS/项目README.md` | 通过 |

## 未解决风险/阻塞

- **无阻塞。** 同步范围文件（根 `README.md`、`docs/`）在最终同步后未再修改；本 QA 记录在最终同步前已定稿。
- Vault 个人笔记不回写仓库，语义不变；`.ccb/` 不进入 Git、不删除。

## 验收标准映射

| Codex 验收标准 | 结果 | 证据 |
| --- | --- | --- |
| 1. `git check-ignore` 证明根 `.ccb/` 被忽略；`git ls-files .ccb` 无输出 | ✅ | 命令表第 3、4 行；`/.ccb/` 匹配 |
| 2. doccheck 跳过 `.ccb` 且有回归测试 | ✅ | `main.go` 跳过列表 + `TestCheckMarkdownLinksSkipsCcb` |
| 3. 原工作区 `make verify` 成功 | ✅ | 命令表"make verify 原工作区"行通过 |
| 4. 协作协议清楚规定最终同步时点/命令/Claude 证据/Codex 门禁，索引一致 | ✅ | 协议"Obsidian 最终同步门禁"章节 + obsidian-sync/贡献/完成定义/docs 导航 |
| 5. 本轮 docs 已同步到 Vault，新增文件存在且内容一致，根 README 按项目README.md 语义 | ✅ | `make docs-sync` 两次 + Vault 一致性核验表 |
| 6. 工作区除规则/工具/测试外无业务代码变更；`.ccb/` 不再出现在 `git status` | ✅ | `git status --short`（仅规则/工具/测试/docs）；`.ccb/` 消失 |
| 7. 新增 QA 记录完整并登记 `docs/qa/README.md` | ✅ | 本文件 + `docs/qa/README.md` 登记行 |

## 最终同步后未再修改 docs 的说明

最终同步于本 QA 记录定稿后执行；此后未修改任何同步范围文件（根 `README.md`、`docs/`）。
