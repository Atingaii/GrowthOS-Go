# 工程基线加固验收

- **日期：** 2026-08-29
- **范围：** Lesson 11 产品代码开始前的仓库身份、测试隔离、文档镜像安全与领域占位边界
- **结果：** 通过

## 修复事实

| 问题 | 修复 | 回归证据 |
| --- | --- | --- |
| Go module 与 GitHub 远端身份不一致 | module 统一为 `github.com/Atingaii/GrowthOS-Go` | `go list -m` |
| `doccheck` 测试可能删除未来根 `testdata` | 完整测试仓库改在 `t.TempDir()` 构造 | `go test ./cmd/doccheck` |
| 被修改的 Vault manifest 可请求越界删除 | 拒绝绝对、未规范化、越界、私有元数据和符号链接路径 | 新增 3 个 `docsync` 安全回归测试 |
| 仓库地图错误描述双向同步 | 统一为项目到个人 Vault 的单向镜像 | 文档漂移评审 |
| 文档首页残留冲突测试文本 | 删除 `source edit` / `source conflict` | `make doc-check` |
| 通用 `Account` 占位与领域事实冲突 | 替换为 `Participation` 与 `Governance` 占位；积分账户归 `Benefit` | 限界上下文地图评审 |
| 完成定义硬编码 WSL Vault 路径 | 改为显式绝对路径；不可访问时要求 QA 如实记录 | 文档评审 |

## 执行命令

```bash
gofmt -w cmd/doccheck/main_test.go cmd/docsync/main.go cmd/docsync/main_test.go
go list -m
go test ./cmd/doccheck ./cmd/docsync
go vet ./...
go test -race ./...
make verify
git diff --check
```

执行结果：本地全部通过。

## 剩余风险

- GitHub `main` 尚未配置 required check 和分支保护；实现分支 push 也不会触发现有 CI。当前 HTTPS 凭据缺少修改 workflow 的权限，因此继续以逐课本地完整门禁为准，后续可由仓库管理员开放 Draft PR 或补充 `workflow` 权限；
- Vault manifest 仍是本地用户可编辑文件，但不再能通过路径键离开 Vault 或覆盖 Vault 私有元数据；
- 当前 macOS 环境没有提供个人 Vault 的绝对路径，因此本次未执行外部镜像同步。
