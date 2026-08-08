# ADR-0003: Documentation As A Deliverable

- **Status:** Accepted
- **Date:** 2026-08-08
- **Owners:** GrowthOS maintainers

## Context

快速生成代码时，README、架构说明、接口契约和测试证据容易落后于实现。仅依赖人工提醒无法稳定发现缺页、断链和“章节已完成但无验收证据”等漂移。

## Decision

文档与代码在同一仓库、同一变更中维护。`docs/course/status.csv` 是课程完成状态的机器可读事实源。`cmd/doccheck` 在本地 `make verify` 和 CI 中检查：

- 96 节编号与分部连续性；
- 完成章节同时存在正文和 QA 证据；
- ADR 文件均进入索引；
- 本地 Markdown 链接有效；
- 核心治理文档存在。

## Consequences

- 纯代码变更也必须判断文档影响。
- “完成”需要测试结果和文档证据，不能只看代码是否编译。
- 语义漂移无法完全自动发现，仍通过 Definition of Done 和评审清单控制。
- 检查器本身使用 Go 标准库，不为治理工具引入额外运行时依赖。
