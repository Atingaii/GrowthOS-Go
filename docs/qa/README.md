# QA 与验证证据

QA 文档保存“如何知道这次交付成立”的证据，而不是复制实现说明。

## 目录

| 记录 | 类型 | 结果 |
| --- | --- | --- |
| [第 1 节验收](lessons/lesson-01.md) | 文档与范围评审 | 通过 |
| [第 2 节验收](lessons/lesson-02.md) | 用户增长旅程与异常体验评审 | 通过 |
| [第 3 节验收](lessons/lesson-03.md) | 运营工作流、审批与生命周期评审 | 通过 |
| [第 4 节验收](lessons/lesson-04.md) | AI Operator 权限、审批、执行与失败语义评审 | 通过 |
| [第 5 节验收](lessons/lesson-05.md) | 事件风暴、命令/事件分类与异常路径评审 | 通过 |
| [第 6 节验收](lessons/lesson-06.md) | 限界上下文、统一语言、事实所有权与协作边界评审 | 通过 |
| [第 7 节验收](lessons/lesson-07.md) | 非功能目标、业务不变量、一致性、恢复与降级评审 | 通过 |
| [第 8 节验收](lessons/lesson-08.md) | 产品架构、系统上下文、用例、领域关系与状态边界评审 | 通过 |
| [第 14 节验收](lessons/lesson-14.md) | React 前端整体框架与 UI 基线 | 通过 |
| [项目 README 规范化验收](readme-refresh-2026-08-09.md) | 项目入口、品牌视觉、内容真实性与秘密管理评审 | 通过 |
| [项目 README 个人镜像验收](docsync-project-readme-2026-08-09.md) | 根 README 同步、路径隔离与个人笔记保护 | 通过 |
| [GitHub Actions 质量门禁修复](ci-verify-fix-2026-08-09.md) | 干净 runner 依赖安装、质量门禁与环境差异验证 | 通过 |
| [双代理协作协议](collaboration-protocol-2026-08-12.md) | 规则内容验收通过；原工作区 `make verify` 存在已知 `.ccb` 断链误报的基础设施失败 | 规则内容通过；原工作区整体验证有已知失败 |
| [Obsidian 同步门禁与 .ccb 排除](collaboration-protocol-obsidian-gate-2026-08-12.md) | 工具/治理/文档变更评审 + 实际 Obsidian 同步 | 通过 |
| [双代理协作激活边界](collaboration-protocol-activation-boundary-2026-08-12.md) | 协作规则激活边界评审 + 实际 Obsidian 同步 | 通过 |

## 记录格式

每份记录至少包含：

- 验收对象和日期；
- 可追溯的需求/章节；
- 实际执行的命令或人工评审清单；
- 结果与失败数；
- 未覆盖项和剩余风险。

后续按风险增加单元测试、集成测试、契约测试、Migration 测试、压测和故障演练，不能用单一测试类型代替全部证据。
