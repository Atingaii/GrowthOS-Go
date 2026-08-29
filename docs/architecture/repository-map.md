# 仓库地图

**状态：** 当前
**更新日期：** 2026-08-23

本文件描述当前仓库，而不是第 96 节的目标仓库。目录随需求演进，改动目录职责时必须同步更新本文件。

第 9 节已验收仓库工程基线。验收只确认目录职责、Go module 和质量门禁成立；占位目录仍不代表对应产品能力已经实现。

| 路径 | 当前职责 | 引入产品代码的章节 |
| --- | --- | --- |
| `cmd/doccheck` | 文档完整性与课程证据检查器 | 项目基线 |
| `cmd/docsync` | 项目 docs/ 与 Obsidian Vault 双向同步工具 | 项目基线 |
| `cmd/growth-api` | 预留 Go HTTP 服务入口 | 第 11 节 |
| `internal/*` | 预留私有领域与基础设施边界 | 随对应领域章节引入 |
| `pkg` | 可被外部导入的少量稳定 Go 包 | 仅在确有跨模块公共契约时 |
| `configs` | 无秘密、可版本化的配置示例 | 第 11 节起 |
| `migrations` | 前向 SQL Migration | 第 13 节起 |
| `scripts` | 可重复的本地自动化 | 按需 |
| `deploy` | Compose、Kubernetes 与发布资产 | 第 16 节起渐进加入 |
| `web` | 复刻 GrowthOS UI 的统一 React 用户端、运营端、MCP 与 AI Operator 框架 | 第 14 节已搭建，后续章节接入真实业务 |
| `docs` | 产品、架构、决策、QA 和课程事实 | 全程 |

## 当前依赖规则

1. `cmd/*` 只做装配和进程生命周期管理。
2. `internal/<domain>` 拥有自己的领域模型、应用用例和端口。
3. 领域模块不得直接依赖另一个模块的数据库实现。
4. `internal/infrastructure` 实现数据库、缓存、消息等技术适配器，不承载业务规则。
5. `pkg` 不是杂物目录；不稳定或仅仓库内部使用的代码留在 `internal`。
6. 当前 `.gitkeep` 只表示计划边界，不代表能力已经实现。

## 有意延迟的决定

- Gin 是 Go HTTP 基线；gRPC + Protobuf 是后续服务间 RPC 基线，分别在第 11、75 节接入并以测试验证。
- MySQL、`sqlx`、手写 SQL、连接池和事务封装在第 13 节接入。
- React、TypeScript、Vite、Tailwind CSS、Lucide、Recharts 和 Zustand 在第 14 节接入；页面视觉以设计包为基线。
- 服务拆分、RPC 和注册中心延迟至第 73 节以后。
- 最终目录图和 ER 图延迟至第 96 节复盘。
