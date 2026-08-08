# ADR-0005：国内常用技术基线

- **状态：** 已接受
- **日期：** 2026-08-08
- **负责人：** GrowthOS 维护者
- **替代：** ADR-0004

## 背景

GrowthOS-Go 的目标包含国内 Go 后端岗位常见的工程实践。Hertz 与 Kitex 在 CloudWeGo 生态内具备价值，但跨公司的使用面和招聘识别度相对有限。项目应优先选择在国内团队中更常见、资料更丰富、便于与多语言服务协作的框架和中间件。

## 决策

第一版采用以下国内常用技术组合，并继续坚持“由业务问题推动接入”的演进原则：

| 能力 | 技术基线 | 首次接入章节 |
| --- | --- | --- |
| HTTP 服务 | Gin | 第 11 节 |
| 服务间 RPC | gRPC + Protobuf | 第 75 节 |
| 数据访问 | `sqlx` + 手写 SQL + 前向 Migration | 第 13 节 |
| 缓存 | go-redis + Redis | 第 16 节环境；第 24 节业务缓存 |
| 消息 | RocketMQ | 第 42 节 |
| 注册中心与动态配置 | Nacos | 第 76 节 |
| 限流、熔断与降级 | Sentinel-Go | 第 78 节 |
| 分析存储 | ClickHouse | 第 70 节 |
| 运营搜索 | Elasticsearch / OpenSearch | 第 80 节 |
| 可观测性 | OpenTelemetry + Prometheus + Grafana + Tempo + Loki | 第 94 节 |
| 前端 | React + TypeScript + Vite + Ant Design + Ant Design Pro | 第 14 节 |

Gin、gRPC、RocketMQ、Nacos、Redis、ClickHouse、Elasticsearch、Prometheus 和 Grafana 都是在国内互联网与企业服务项目中常见的组合。gRPC 虽是国际标准，但在国内多语言微服务协作中使用广泛，因此作为 RPC 契约基线。

## 影响

- 第 11 节使用 Gin 建立 HTTP 服务，第 75 节使用 gRPC + Protobuf 建立内部服务通信。
- 课程会在引入时解释技术选型和替代方案，但不再将 Hertz 或 Kitex 作为实现主线。
- RocketMQ、Nacos、Sentinel-Go、ClickHouse 与 Elasticsearch/OpenSearch 不提前启动，仍必须有实际业务、性能或运维需求作为引入证据。
- 后续若需要变更技术基线，必须新增 ADR 并同步更新课程路线、产品简述、仓库地图与 QA 证据。
