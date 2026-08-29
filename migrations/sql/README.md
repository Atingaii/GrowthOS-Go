# SQL Migration 清单

本目录只保存不可变、前向执行的 MySQL Migration。一个文件尽量只包含一条 DDL，使 Migration 版本边界与 MySQL 单条 atomic DDL 边界一致。

| 版本 | 文件 | 目的 |
| --- | --- | --- |
| 000001 | `000001_create_lottery_strategy.up.sql` | 创建 Lottery Strategy 聚合根表 |
| 000002 | `000002_create_lottery_strategy_award.up.sql` | 创建策略内 Award 表、复合主键与外键 |

已经进入共享分支的文件禁止改写。任何结构修正都必须新增更高版本；不要使用 `IF NOT EXISTS` 隐藏环境漂移。
