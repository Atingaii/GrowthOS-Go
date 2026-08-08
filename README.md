# GrowthOS-Go

GrowthOS-Go 是一个以演进式工程课程方式建设的 AI 原生营销增长平台。第一阶段只完成 Go 版本；数据库结构、领域边界和部署拓扑只在新需求证明复杂度确有必要时演进。

当前仓库包含项目骨架、已完成的第 1 节产品分析，以及复刻既有设计的第 14 节完整前端框架。仓库有意不包含最终数据库结构、微服务或完整基础设施栈。

## 当前状态

- 课程进度：96 节中的第 1、14 节已完成
- 产品代码：Go API 尚未开始，第 11 节建立第一个 HTTP 服务；`web/` 已具备完整可运行前端框架
- 数据库：尚未接入，第 13 节建立连接和 Migration 机制
- 架构：规划为模块化单体，后续根据证据拆分服务

请先阅读[文档首页](docs/README.md)、[课程路线](docs/course/README.md)和[第 1 节](docs/course/part-01/lesson-01-why-ai-native-growth-platform.md)。

## 仓库结构

```text
growth-os-go/
├── cmd/             # 可执行程序；课程按需加入产品入口
├── internal/        # 私有领域与基础设施模块
├── pkg/             # 少量稳定的公共 Go 包
├── configs/         # 可版本化且不含秘密的配置示例
├── migrations/      # 第 13 节开始使用的前向 SQL Migration
├── scripts/         # 本地自动化脚本
├── deploy/          # 随基础设施逐步加入的部署资源
├── docs/            # 产品、架构、ADR、QA 和课程证据
└── web/             # 统一 React 用户端、运营端、MCP 与 AI Operator 框架
```

`web/` 当前复刻既有 GrowthOS UI，页面使用集中 Mock 数据；第 15 节与 Go 后端完成首次联调，后续章节逐步替换真实业务能力。

前端架构与启动方式见[前端工程架构](docs/frontend/frontend-architecture.md)。

## 质量门禁

需要 Go 1.24 或更高版本。

```bash
make verify
```

`make verify` 会检查代码格式、运行测试、校验 96 节课程台账、检查 ADR 注册情况，并发现失效的本地 Markdown 链接。

## 协作约定

1. 每次行为或架构变化都要更新对应权威文档。
2. 重大决策应在实现前或同一变更中补充 ADR。
3. 课程正文和 QA 证据均已登记且通过 `make doc-check` 后，章节才能标记为 `已完成`。
4. 引入 Migration 后，数据库变更只能通过前向 Migration 表达。
5. 秘密和环境专属配置不得提交到仓库。
