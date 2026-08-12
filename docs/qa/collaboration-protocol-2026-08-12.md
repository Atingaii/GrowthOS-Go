# 双代理协作协议验收记录

- **日期：** 2026-08-12
- **对象：** 根级 `AGENTS.md`/`CLAUDE.md`、`docs/standards/collaboration-protocol.md`、[ADR-0004](../decisions/ADR-0004-collaboration-protocol.md)、`CONTRIBUTING.md`、`docs/README.md`
- **类型：** 协作规则与文档治理变更评审
- **结果：** 规则内容验收通过；原工作区整体验证存在已知基础设施失败（见"自动化验证"与"失败与剩余风险"）

## 变更目标

建立能被 Codex 与 Claude 分别自动读取、且内容一致的根级协作规则：Codex 负责规划与验收、Claude 是唯一实现与测试负责人，并规定"任务包 → 实现 → 测试 → 交付报告 → 验收 → 修复"完整闭环。本轮只制定规则，不触碰业务实现。

## 验收清单

| 检查项 | 结果 | 证据 |
| --- | --- | --- |
| 根级存在 Codex 可读入口（AGENTS.md） | 通过 | `AGENTS.md` 指向协作协议 |
| 根级存在 Claude 可读入口（CLAUDE.md） | 通过 | `CLAUDE.md` 指向协作协议 |
| 单一权威正文，入口不重复维护规则副本 | 通过 | 正文唯一在 `docs/standards/collaboration-protocol.md`，两个入口仅指针 |
| 固定职责明确：Codex 规划/验收、Claude 唯一实现与测试 | 通过 | 协作协议"固定职责"表 |
| 完整闭环：任务包字段、实现、测试、交付报告、验收、失败重委派 | 通过 | 协作协议"完整闭环"及任务包字段表 |
| 交付/QA 记录强制字段齐全 | 通过 | 协作协议"交付报告"字段表（8 项） |
| 禁止无命令与结果的"已测试"断言 | 通过 | 协作协议"测试"与"证据要求" |
| 明确本轮只制定规则、不触碰业务实现 | 通过 | 仅新增/修改规则、索引、ADR、QA 文档，无业务代码改动 |
| 按文档治理记录决策（ADR）并登记索引 | 通过 | `ADR-0004-collaboration-protocol.md` 已加入 `docs/decisions/README.md` 索引 |
| 更新索引/CONTRIBUTING 使规则易发现 | 通过 | `CONTRIBUTING.md`、`docs/README.md` 增加协作协议导航 |
| 编写 QA 记录留档 | 通过 | 本文件 |

## 自动化验证

执行环境：本地工作区 `/home/zy/tencentgo/newStart/growthOS`；本次为纯文档/规则变更。

| 命令 | 执行环境 | 结果 |
| --- | --- | --- |
| `make verify`（= `fmt-check` + `test` + `doc-check` + `web-verify`） | 原工作区（含 `.ccb/`） | **失败**，见下方"失败与剩余风险" |
| `go test ./...` | 原工作区 | 通过（`cmd/doccheck`、`cmd/docsync` 均 ok） |
| `make web-verify`（typecheck + build） | 原工作区 | 通过（仅既有 chunk 体积提示，与本次无关） |
| `make fmt-check` | 排除 `.ccb`/`node_modules`/`.git` 的干净副本 | 通过 |
| `go test ./...` | 干净副本 | 通过（`cmd/doccheck`、`cmd/docsync` 均 ok） |
| `go run ./cmd/doccheck` | 干净副本 | 通过（`documentation checks passed`） |

## 结果

**规则内容验收通过**：本次新增/修改的规则、索引、ADR、QA 文档经上述文档检查全部通过。

**原工作区整体验证存在已知基础设施失败**：`make verify` 在原工作区失败，失败项全部来自 `.ccb/` 目录下第三方 Markdown 的断链误报，与本次变更内容无关，未判定为本次变更失败。

## 失败与剩余风险

- **失败现象**：`make verify` 在原工作区失败。`doc-check`（`cmd/doccheck`）的链接检查遍历仓库时未将 `.ccb/` 列入跳过目录，扫描到 CCB 运行时目录（`agents/*/provider-state`、`shared-cache`、plugins 等约 3674 个第三方 Markdown），对其中大量依赖插件上下文/外链的链接报"missing target"。
- **失败原因**：`.ccb/` 是 CCB 多代理协作的运行时目录，未纳入 `cmd/doccheck` 的跳过列表，也未被 `.gitignore` 排除（`git status` 中 `.ccb/` 为未跟踪）。
- **验证替代方式**：为证明本次变更本身无文档/链接问题，将工作区复制到排除 `.ccb`/`node_modules`/`.git` 的干净副本，对本次实际改动内容执行 `fmt-check`、`go test ./...`、`doc-check`，全部通过；`web-verify` 不扫描 Markdown，在原工作区直接通过。
- **未修复原因**：修复方向是调整 `cmd/doccheck` 跳过 `.ccb`（或 gitignore 排除 `.ccb`），属基础设施/工具改动，超出本轮"仅修正规则交付的 QA/文档证据、不做业务实现"的范围，故未在本轮修改。
- **剩余风险**：在含 `.ccb/` 的原工作区直接运行 `make verify` 会持续产生 `.ccb` 断链误报，直至 doccheck 或 gitignore 修复。此风险已如实记录，不属于本次规则变更引入。

**后续处置（2026-08-12，见 [Obsidian 最终同步门禁与 .ccb 排除](collaboration-protocol-obsidian-gate-2026-08-12.md)）**：上述风险已通过新增 `/.ccb/` 到 `.gitignore`、让 `cmd/doccheck` 跳过 `.ccb` 并补回归测试而解决。原工作区（含 `.ccb/`）现可正常运行 `make verify`。历史失败记录保留，不抹掉。

## 未覆盖项

- 本协议是过程约束，不强制校验"交付报告字段是否齐全"——字段合规由 Codex 验收把关，属于人工评审环节。
- `AGENTS.md` 与 `CLAUDE.md` 为指针入口；未来协议修订须改正文一处，入口保持不变，二者内容一致性依赖此约定。
