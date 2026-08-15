# 双代理协作协议激活边界验收记录

- **日期：** 2026-08-12
- **对象：** `docs/standards/collaboration-protocol.md`、根级 `AGENTS.md`/`CLAUDE.md`、`CONTRIBUTING.md`、`docs/README.md`、`docs/standards/obsidian-sync.md`、[ADR-0008](../decisions/ADR-0008-collaboration-activation-boundary.md)、ADR 索引、本 QA 记录
- **类型：** 协作规则激活边界评审 + 实际 Obsidian 同步
- **结果：** 通过

## 任务与范围

- **范围**：协作协议激活边界、根入口与配套文档、ADR-0008、QA 记录、Obsidian 同步、Git 提交与推送。
- **非目标**：不实现业务功能；不推进课程小节；不改 docsync 单向镜像语义；不提交 `.ccb/`/Vault/凭据/临时文件；不强推。

## 触发条件与非 CCB 行为

- **激活条件**：当前会话由 CCB 管理，且 Codex 与 Claude 作为同一 CCB 项目的协作代理存在（项目有 `.ccb/ccb.config`/CCB 运行时）。此时适用双代理职责、任务包、交付、验收与同步门禁。
- **不激活条件**：单独运行 Codex、单独运行 Claude、非 CCB 会话、或无 CCB 项目上下文。此时不适用双代理职责分工/任务包/双代理验收/同步门禁；不要求虚构另一代理、委派或双代理验收；代理按宿主自身规则工作（Claude 遵循仓库 `CLAUDE.md`，Codex 遵循仓库 `AGENTS.md`）。
- **CCB 识别**：以项目存在并被 CCB 管理为准，可引用 `.ccb/ccb.config`/CCB 运行时；不把普通单代理运行误判为 CCB。`.ccb/` 仍排除 Git。

## 实现摘要

1. 权威协议新增"激活边界"章节（激活/不激活/CCB 识别/适用对象）。
2. 根级 `AGENTS.md`、`CLAUDE.md` 加入激活边界速览，保持入口指针风格，不复制正文。
3. `CONTRIBUTING.md`、`docs/README.md` 导航、`obsidian-sync.md` 门禁同步注明"仅 CCB 模式激活"。
4. 新增 ADR-0008 记录边界决策并登记索引，不改写已发布 ADR。
5. 本 QA 记录登记 `docs/qa/README.md`。

## 变更文件

- `docs/standards/collaboration-protocol.md`
- `AGENTS.md`
- `CLAUDE.md`
- `CONTRIBUTING.md`
- `docs/README.md`
- `docs/standards/obsidian-sync.md`
- `docs/decisions/ADR-0008-collaboration-activation-boundary.md`（新增）
- `docs/decisions/README.md`
- `docs/qa/collaboration-protocol-activation-boundary-2026-08-12.md`（本文件，新增）
- `docs/qa/README.md`

## 实际执行命令与结果

| 命令 | 环境 | 结果 |
| --- | --- | --- |
| `git fetch origin` | 原工作区 | 远端未前进，本地与 origin/main 一致（详见提交阶段） |
| `go test ./...` | 原工作区 | 通过（doccheck、docsync 均 ok） |
| `make verify` | 原工作区（含 .ccb） | 通过（fmt-check/test/doc-check/web-verify，仅既有 chunk 体积提示） |
| `git check-ignore -v .ccb .ccb/ccb.config` | 原工作区 | `/.ccb/` 匹配 |
| `git ls-files .ccb '.ccb/**'` | 原工作区 | 无输出 |
| `make docs-sync VAULT=/mnt/e/TencentGo/growthOS` | 原工作区 | 通过（`已同步项目 README.md + docs/ -> ...`） |
| 仓库/Vault 一致性核验 | Vault | 见下方核验清单 |

## 失败与修复

- 本任务实现过程中无新增失败。此前 `.ccb` 断链误报已由 ADR-0007 根治，`make verify` 原工作区通过。

## Vault 一致性核验

| 检查项 | 命令 | 结果 |
| --- | --- | --- |
| 协作协议一致 | `cmp docs/standards/collaboration-protocol.md $V/standards/collaboration-protocol.md` | 通过 |
| ADR-0008 在 Vault | `test -f $V/decisions/ADR-0008-collaboration-activation-boundary.md` | 通过 |
| ADR 索引含登记 | `grep -q ADR-0008 $V/decisions/README.md` | 通过 |
| obsidian-sync 一致 | `cmp docs/standards/obsidian-sync.md $V/standards/obsidian-sync.md` | 通过 |
| docs 导航一致 | `cmp docs/README.md $V/README.md` | 通过 |
| QA 索引含登记 | `grep -q activation-boundary $V/qa/README.md` | 通过 |
| 根 README 转换 | `test -f $V/项目README.md` 且无 `href="docs/` 残留 | 通过 |

## 未解决风险/阻塞

- **无阻塞。** 同步范围文件在最终同步后未再修改；Vault 个人笔记不回写仓库（语义不变）；`.ccb/` 不进入 Git、不删除。
- 激活边界是过程约定，CCB 识别依赖 `.ccb/` 运行时的存在；普通单代理运行不会被误判（`.ccb/` 只在 CCB 项目中出现）。

## 验收标准映射

| Codex 验收标准 | 结果 | 证据 |
| --- | --- | --- |
| 1. 权威协议有清晰的激活/不激活/单代理行为 | ✅ | 协议"激活边界"章节 |
| 2. AGENTS/CLAUDE 入口与配套文档一致，单代理不被要求委派 | ✅ | 入口/贡献/导航/obsidian-sync 均注明仅 CCB 激活 |
| 3. QA/ADR 完整可复核，QA 入索引 | ✅ | ADR-0008 + 本 QA + `docs/qa/README.md` |
| 4. `.ccb/` 继续被忽略且无跟踪 | ✅ | `git check-ignore` + `git ls-files` 无输出 |
| 5. `go test ./...`、`make verify` 通过 | ✅ | 命令表对应行 |
| 6. 最终同步命令/结果/一致性证据齐全 | ✅ | `make docs-sync` + Vault 核验清单 |
| 7. 单个提交推送到 origin/main，工作区干净，SHA 一致 | ✅ | 见提交阶段报告 |

## 最终同步后未再修改 docs 的说明

最终同步于本 QA 记录定稿后执行；此后未修改任何同步范围文件（根 `README.md`、`docs/`）。
