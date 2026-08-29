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
| [第 9 节验收](lessons/lesson-09.md) | Git/Go 单仓库、目录职责、统一质量门禁与能力边界 | 通过 |
| [第 10 节验收](lessons/lesson-10.md) | 模块化单体取舍、依赖边界与拆分触发条件评审 | 通过 |
| [第 11 节验收](lessons/lesson-11.md) | Gin 健康契约、进程生命周期、优雅关闭与错误传播 | 通过 |
| [第 12 节验收](lessons/lesson-12.md) | 类型化配置、结构化日志、请求关联、fault 与统一 HTTP 错误 | 通过 |
| [第 13 节验收](lessons/lesson-13.md) | MySQL 身份隔离、连接池、readiness、前向 Migration 与真实容器联调 | 通过 |
| [第 14 节验收](lessons/lesson-14.md) | React 前端整体框架与 UI 基线 | 通过 |
| [第 15 节验收](lessons/lesson-15.md) | 同源探针联调、运行时契约、失败分类、组件状态与真实浏览器故障场景 | 通过 |
| [第 16 节验收](lessons/lesson-16.md) | Compose 拓扑、秘密边界、最小权限、故障恢复、浏览器场景与 M0 负载基线 | 通过 |
| [第 17 节验收](lessons/lesson-17.md) | Lottery Strategy/Award 构造不变量、权重边界、显式结果语义、切片所有权与纯领域依赖 | 通过 |
| [第 18 节验收](lessons/lesson-18.md) | Lottery 两表结构、约束、latest 2 Migration、dirty 停止、身份权限与 Compose 授权启动链 | 通过 |
| [项目 README 规范化验收](readme-refresh-2026-08-09.md) | 项目入口、品牌视觉、内容真实性与秘密管理评审 | 通过 |
| [项目 README 个人镜像验收](docsync-project-readme-2026-08-09.md) | 根 README 同步、路径隔离与个人笔记保护 | 通过 |
| [GitHub Actions 质量门禁修复](ci-verify-fix-2026-08-09.md) | 干净 runner 依赖安装、质量门禁与环境差异验证 | 通过 |
| [工程基线加固](engineering-baseline-hardening-2026-08-29.md) | module 身份、测试隔离、镜像安全、领域占位与分支 CI | 通过 |

## 记录格式

每份记录至少包含：

- 验收对象和日期；
- 可追溯的需求/章节；
- 实际执行的命令或人工评审清单；
- 结果与失败数；
- 未覆盖项和剩余风险。

后续按风险增加单元测试、集成测试、契约测试、Migration 测试、压测和故障演练，不能用单一测试类型代替全部证据。
