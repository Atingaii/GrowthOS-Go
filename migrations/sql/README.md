# SQL Migration 清单

本目录只保存不可变、前向执行的 MySQL Migration。一个文件尽量只包含一条 DDL，使 Migration 版本边界与 MySQL 单条 atomic DDL 边界一致。

| 版本 | 文件 | 目的 |
| --- | --- | --- |
| 000001 | `000001_create_lottery_strategy.up.sql` | 创建 Lottery Strategy 聚合根表 |
| 000002 | `000002_create_lottery_strategy_award.up.sql` | 创建策略内 Award 表、复合主键与外键 |
| 000003 | `000003_create_lottery_strategy_routing_graph.up.sql` | 创建以 `graph_id + revision` 标识的路由图 header；保存 schema version 与逻辑 root 引用 |
| 000004 | `000004_create_lottery_strategy_routing_node.up.sql` | 创建 decision / strategy target 互斥节点，并以 `RESTRICT` 外键约束 Strategy terminal |
| 000005 | `000005_create_lottery_strategy_routing_edge.up.sql` | 创建同图内的 source/target 复合外键、source-scoped branch 唯一性与 default 映射 |

已经进入共享分支的文件禁止改写。任何结构修正都必须新增更高版本；不要使用 `IF NOT EXISTS` 隐藏环境漂移。

第 28 节必须按 `000003 -> 000004 -> 000005` 执行。graph header 的 `root_node_id` 刻意没有反向外键：MySQL 不支持可延迟外键，如果 header 和 node 互相立即引用，就不存在不关闭约束的合法首次插入顺序。node 仍以复合外键引用 graph，edge 的 `from_node_id` / `to_node_id` 都以 `(graph_id, revision, node_id)` 引用同一版本节点。root 存在性、root 类型、环、深度、可达性、decision 出边完备性和 target 类型由完整恢复后的领域校验负责。

三张路由表是 create-only schema：只有 `created_at`，不提供 `updated_at`。这个 Migration 切片不会给在线应用账号增加 SELECT/INSERT/UPDATE/DELETE 权限；即使同一课程后续补上未装配仓储，也不等于已经形成在线发布运行时。Migration 账号只负责结构演进与隔离验收。
