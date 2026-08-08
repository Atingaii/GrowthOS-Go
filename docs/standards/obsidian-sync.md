# Obsidian 文档双向同步

项目文档目录 `docs/` 与 Windows 目录 `E:\TencentGo\growthOS`（在 WSL 中为 `/mnt/e/TencentGo/growthOS`）采用双向同步。

## 同步范围

- 项目源目录：仓库内的 `docs/`。
- Obsidian 目标目录：`E:\TencentGo\growthOS` 根目录。
- 目标目录中的 `.obsidian/` 保留为 Obsidian 配置，不参与同步。
- `.growthos-sync/manifest.json` 保存共同基线，不参与文档内容同步。

## 使用方式

在仓库根目录执行：

```bash
make docs-sync VAULT=/mnt/e/TencentGo/growthOS
```

Windows 环境也可以直接运行：

```powershell
go run ./cmd/docsync --vault 'E:\TencentGo\growthOS'
```

首次运行会把项目 `docs/` 写入 Obsidian 根目录。之后每次运行都会比较共同基线：

- 只有项目侧变化：同步到 Obsidian。
- 只有 Obsidian 侧变化：同步回项目 `docs/`。
- 两侧同时修改同一文件且内容不同：停止并报告冲突，不覆盖任何一侧。
- 删除也参与比较；单侧删除会同步到另一侧。

同步完成后必须执行：

```bash
make verify
```

同步工具不会自动提交 Git。文档变更仍需人工检查、运行质量门禁后再提交。
