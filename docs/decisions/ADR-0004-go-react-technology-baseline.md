# ADR-0004: Go And React Technology Baseline

- **Status:** Accepted
- **Date:** 2026-08-08
- **Owners:** GrowthOS maintainers

## Context

本项目需要把数据库、后端、前端和基础设施按业务问题逐步引入，同时避免每一节重新争论基础工具。技术栈应形成稳定基线，但具体组件仍不能在问题出现前提前启动。

## Decision

第一版 Go 技术基线为：

- Go；
- Hertz HTTP；
- Kitex RPC（第 75 节接入）；
- `sqlx` + 手写 SQL + 前向 Migration；
- `go-redis`；
- RocketMQ；
- Nacos；
- OpenTelemetry。

第一版前端技术基线为：

- React；
- TypeScript；
- Vite；
- Ant Design；
- Ant Design Pro。

MySQL 在第 13 节接入，React 在第 14 节接入，Redis 在第 16 节作为开发环境依赖出现但第 24 节才用于业务缓存，RocketMQ、ClickHouse、OpenSearch 和 Kubernetes 均由后续明确需求推动。

## Consequences

- 技术选择稳定，课程可以专注于需求、数据和系统演进。
- 组件不会因为“已选型”就提前进入每个环境；接入时必须有对应章节问题和 QA 证据。
- 若基准、生态或维护性证据要求替换基线，必须新增 ADR，不直接静默改写路线。
- Java 不属于本 ADR 的实现范围；Go 版完成后另行规划 Java 第二轮。
