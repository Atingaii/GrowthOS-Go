# 贡献指南

GrowthOS-Go 将文档视为实现的一部分，而不是延期交付物。

提交或合并变更前：

1. 阅读[文档治理规范](docs/standards/documentation-governance.md)。
2. 更新对应的产品、架构、契约、运维或课程文档。
3. 对重大决策新增或替代一份 [ADR](docs/decisions/README.md)。
4. 在 [docs/qa](docs/qa/README.md) 下记录验证证据。
5. 执行 `make verify`。

在所有 `README.md`/`docs/`/QA 修改完成后，按 [Obsidian 同步说明](docs/standards/obsidian-sync.md) 执行最终同步：

```bash
make docs-sync VAULT=/mnt/e/TencentGo/growthOS
```

提交应保持足够小，使代码、文档、测试和 Migration 描述同一个完整变更。
