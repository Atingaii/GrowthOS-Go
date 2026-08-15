# 贡献指南

GrowthOS-Go 将文档视为实现的一部分，而不是延期交付物。

## 双代理协作

仓库在 **CCB 模式**下由 Codex 与 Claude 双代理协作开发。完整规则见 [双代理协作协议](docs/standards/collaboration-protocol.md)（根级 `AGENTS.md`/`CLAUDE.md` 为入口）：

- **激活边界**：双代理职责、任务包、验收与同步门禁仅在 CCB 管理且 Codex/Claude 作为同一 CCB 项目协作代理时激活；单独运行 Codex 或单独运行 Claude、非 CCB 会话或无 CCB 项目上下文时不激活，代理按宿主自身规则工作，不被要求虚构另一代理或双代理验收。
- **Codex**：规划与验收负责人——拆分任务、定义范围与验收标准，仅基于 diff、文档和测试证据验收。
- **Claude**：唯一实现与测试负责人——负责所有代码/文档修改与测试，并为每个任务提交交付/QA 记录。

提交或合并变更前：

1. 阅读[文档治理规范](docs/standards/documentation-governance.md)。
2. 更新对应的产品、架构、契约、运维或课程文档。
3. 对重大决策新增或替代一份 [ADR](docs/decisions/README.md)。
4. 在 [docs/qa](docs/qa/README.md) 下记录验证证据。
5. 执行 `make verify`。

Claude 在每个任务的所有 `README.md`/`docs/`/QA 修改完成后、提交交付报告前，须执行 Obsidian 最终同步并将命令/结果/一致性证据写入交付/QA 记录（门禁见 [协作协议](docs/standards/collaboration-protocol.md) 与 [Obsidian 同步说明](docs/standards/obsidian-sync.md)）：

```bash
make docs-sync VAULT=/mnt/e/TencentGo/growthOS
```

提交应保持足够小，使代码、文档、测试和 Migration 描述同一个完整变更。
