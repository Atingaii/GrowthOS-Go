# 项目 README 个人镜像验收记录

- **日期：** 2026-08-09
- **对象：** `cmd/docsync`
- **类型：** 个人文档同步行为测试
- **结果：** 通过

## 变更目标

在保持仓库 `docs/` 为文档事实源、个人 Vault 不回写仓库的前提下，将仓库根 `README.md` 一并同步到 `E:\TencentGo\growthOS`，便于在 Obsidian 中查看项目入口文档。

## 路径映射

| 仓库来源 | 个人目录目标 |
| --- | --- |
| `README.md` | `项目README.md` |
| `docs/README.md` | `README.md` |
| `docs/**` | 保持相对路径 |

根 README 使用独立文件名，避免与 `docs/README.md` 的文档首页互相覆盖。

## 验收清单

| 检查项 | 结果 | 证据 |
| --- | --- | --- |
| 根 README 进入同步清单 | 通过 | `TestCollectSkipsMetadata` |
| 根 README 更新后刷新 `项目README.md` | 通过 | `TestMirrorDoesNotImportVaultEdits` |
| README 中的 `docs/` 链接适配 Vault 根目录 | 通过 | `TestMirrorDoesNotImportVaultEdits` 与实际链接检查 |
| `docs/` 仍按原相对路径同步 | 通过 | `TestCollectSkipsMetadata` |
| Vault 个人笔记不导入仓库 | 通过 | `TestMirrorDoesNotImportVaultEdits` |
| `.obsidian/` 和同步状态不进入镜像清单 | 通过 | `TestCollectSkipsMetadata` |
| 实际个人目录存在 `项目README.md` | 通过 | `make docs-sync` 后文件检查 |

## 自动化验证

```text
go test ./cmd/docsync
make verify
make docs-sync VAULT=/mnt/e/TencentGo/growthOS
```

## 边界

- 同步仍然是仓库到个人目录的单向镜像，不做双向合并；
- Vault 中的个人笔记、批注和 `.obsidian/` 配置不会进入 Git；
- 同步命令不会自动提交仓库；
- 同步器只对个人副本中的 Markdown/HTML 文档链接移除 `docs/` 前缀，使图片和导航指向同步后的 `assets/`、`course/` 等目录；仓库 README 保持不变。
