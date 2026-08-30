# SQL Migration 清单

本目录只保存不可变、前向执行的 MySQL Migration。一个文件尽量只包含一条 DDL，使 Migration 版本边界与 MySQL 单条 atomic DDL 边界一致。

| 版本 | 文件 | 目的 |
| --- | --- | --- |
| 000001 | `000001_create_lottery_strategy.up.sql` | 创建 Lottery Strategy 聚合根表 |
| 000002 | `000002_create_lottery_strategy_award.up.sql` | 创建策略内 Award 表、复合主键与外键 |
| 000003 | `000003_create_lottery_strategy_routing_graph.up.sql` | 创建以 `graph_id + revision` 标识的路由图 header；保存 schema version 与逻辑 root 引用 |
| 000004 | `000004_create_lottery_strategy_routing_node.up.sql` | 创建 decision / strategy target 互斥节点，并以 `RESTRICT` 外键约束 Strategy terminal |
| 000005 | `000005_create_lottery_strategy_routing_edge.up.sql` | 创建同图内的 source/target 复合外键、source-scoped branch 唯一性与 default 映射 |
| 000006 | `000006_create_lottery_strategy_snapshot.up.sql` | 创建以 `strategy_id + revision` 标识的 create-only Strategy snapshot header |
| 000007 | `000007_create_lottery_strategy_snapshot_award.up.sql` | 保存 exact Strategy revision 下的完整 Award 快照 |
| 000008 | `000008_create_marketing_activity.up.sql` | 创建 Marketing Activity root、生命周期、state CAS 与 nullable active publication |
| 000009 | `000009_create_marketing_activity_publication.up.sql` | 创建不可变 release/rollback publication、exact graph ref、时间窗与审批 evidence ref |
| 000010 | `000010_create_marketing_activity_publication_strategy.up.sql` | 保存 publication 的 StrategyID 到 exact Strategy revision 闭集映射 |
| 000011 | `000011_add_marketing_activity_active_publication_fk.up.sql` | 在合法 draft-first 写入顺序形成后增加 Activity active publication 反向复合外键 |

已经进入共享分支的文件禁止改写。任何结构修正都必须新增更高版本；不要使用 `IF NOT EXISTS` 隐藏环境漂移。

第 28 节必须按 `000003 -> 000004 -> 000005` 执行。graph header 的 `root_node_id` 刻意没有反向外键：MySQL 不支持可延迟外键，如果 header 和 node 互相立即引用，就不存在不关闭约束的合法首次插入顺序。node 仍以复合外键引用 graph，edge 的 `from_node_id` / `to_node_id` 都以 `(graph_id, revision, node_id)` 引用同一版本节点。root 存在性、root 类型、环、深度、可达性、decision 出边完备性和 target 类型由完整恢复后的领域校验负责。

三张路由表是 create-only schema：只有 `created_at`，不提供 `updated_at`。这个 Migration 切片不会给在线应用账号增加 SELECT/INSERT/UPDATE/DELETE 权限；即使同一课程后续补上未装配仓储，也不等于已经形成在线发布运行时。Migration 账号只负责结构演进与隔离验收。

第 30 节按 `000006 -> 000007 -> 000008 -> 000009 -> 000010 -> 000011` 前向执行。Strategy snapshot 两表只追加 exact revision；Activity publication/binding 也只追加，只有 Activity root 通过 expected state version 做精确 CAS。普通替换和 rollback 都生成更高 publication version，rollback 记录旧 source 但不更新或删除历史；retire 只更新 root 的终态和 evidence reference，并保留最后 active publication。

`000011` 不会重现 graph root 的首次插入死锁：Activity root 可以先以 draft、`active_version = NULL` 合法写入，再插 publication/bindings，最后更新 active。Marketing 表刻意不对 Lottery graph/snapshot 建跨上下文外键，避免把同库物理布局误当领域契约；发布 verifier 必须 exact 读取 graph、比较全部唯一 terminal StrategyID 集，并逐一 exact 读取 Strategy snapshot。长期 `growthos_app` 在本节仍不获得这五张新表的权限。
