# Obsidian 个人文档镜像

项目根 `README.md` 和文档目录 `docs/` 自动镜像到个人 Windows 目录 `E:\TencentGo\growthOS`（在 WSL 中为 `/mnt/e/TencentGo/growthOS`）。这是给个人阅读和批注使用的副本，不是第二个代码事实源。

## 同步范围

- 唯一源：仓库根 `README.md` 和仓库内的 `docs/`。
- 个人镜像目录：`E:\TencentGo\growthOS` 根目录。
- 根 `README.md` 在个人目录中保存为 `项目README.md`，避免覆盖 `docs/README.md` 对应的文档首页。
- 同步器会在 `项目README.md` 中移除文档链接的 `docs/` 前缀，使头图和文档导航适配 Vault 的目录结构；仓库原文件不会被改写。
- 目标目录中的 `.obsidian/` 保留为 Obsidian 配置，不参与同步。
- `.growthos-sync/manifest.json` 只保存在个人目录，用于记录上次镜像状态，不进入 Git。
- Vault 中新增的个人笔记、批注和修改不会写回仓库，也不会进入 Git。

## 使用方式

在仓库根目录执行：

```bash
make docs-sync VAULT=/mnt/e/TencentGo/growthOS
```

持续自动镜像：

```bash
make docs-sync-watch VAULT=/mnt/e/TencentGo/growthOS
```

Windows 环境也可以直接运行：

```powershell
go run ./cmd/docsync --vault 'E:\TencentGo\growthOS' --watch
```

首次运行会把项目根 `README.md` 和 `docs/` 写入 Obsidian 根目录。之后同步器观察这两个来源的变化：

- 项目侧新增或修改：镜像到 Obsidian。
- 项目侧删除：直接删除 Obsidian 中此前由同步工具写入的对应文件。
- Obsidian 侧修改：保留在个人目录；当项目文件再次变化时，以项目版本刷新该文件。
- Obsidian 侧新增个人笔记：保留，不会复制到项目。
- 不存在双向合并，也不会因为 Vault 修改阻塞项目同步。

同步完成后必须执行：

```bash
make verify
```

同步工具不会自动提交 Git，也不会将 Vault 目录加入项目 Git。项目文档变更仍需人工检查、运行质量门禁后再提交。

## 作为协作验收门禁

在 **CCB 模式**的双代理协作中，Obsidian 最终同步是交付前的最后文档镜像步骤，规则见 [双代理协作协议](collaboration-protocol.md)。该门禁仅在 CCB 激活边界内适用（当前会话由 CCB 管理且 Codex/Claude 作为同一 CCB 项目协作代理）；单独运行 Codex/Claude、非 CCB 会话或没有 CCB 项目上下文时不强制此门禁。要点：

- 每个任务无论是否修改 `README.md`/`docs`/QA，都必须在提交交付报告前执行 `make docs-sync VAULT=/mnt/e/TencentGo/growthOS`，不允许跳过；
- 同步命令、结果与一致性核验写入交付/QA 记录；Codex 缺此证据不得验收通过；
- 最终同步之后不再修改仓库 `README.md`/`docs/`，否则需重新同步。
