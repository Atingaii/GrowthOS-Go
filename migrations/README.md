# 数据库迁移目录

这里存放 GrowthOS 的前向 MySQL Migration。第 13 节只建立执行机制；第 18 节开始加入真实业务结构；第 28 节把已出现的 Lottery 路由词汇落为三张 create-only 图结构表。

规则：

- 文件直接放在 `sql/` 根层，命名必须为六位递增版本号、下划线描述和 `.up.sql`，例如 `000001_create_lottery_strategy.up.sql`；嵌套目录和其他 `.sql` 后缀会使启动失败，避免拼写错误被静默跳过。
- 只新增前向迁移，不在应用里暴露 `down`、`drop` 或 `force`。
- 已经进入共享分支的迁移是不可变历史；修正结构时新增下一个版本，禁止改写旧文件。
- 每个文件应保持有限、可审查，优先只含一条 DDL，并考虑 MySQL DDL 的隐式提交语义。MySQL atomic DDL 只覆盖单条受支持语句，不会让整个多语句文件变成事务。
- API 账号与 Migration 账号必须分开；迁移命令使用专用凭据和单连接池。
- 第 28 节的路由图按 `graph_id + revision` 复合身份保存 header、node 与 edge。MySQL 外键不可延迟，为保持“先 header、后 node”的合法插入顺序，`root_node_id` 只保存逻辑引用，不建立 header 到 node 的反向外键；写入完成与恢复时必须由领域校验确认 root 存在且为 decision。
- 路由图三表只有 `created_at`，没有 `updated_at`。当前在线 `growthos_app` 不获得这三表权限；这个 Migration 切片只形成结构与 migrator-side schema 证据，后续仓储实现也必须保持未装配状态，不能提前声明在线发布能力。

迁移二进制会同时嵌入本说明和 `sql/` 子目录，因此发布产物不依赖宿主机上的相对路径。
