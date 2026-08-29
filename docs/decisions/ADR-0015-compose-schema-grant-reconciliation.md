# ADR-0015：Compose 中的业务表精确授权收敛

- **状态：** 已接受
- **日期：** 2026-08-29
- **负责人：** GrowthOS 维护者

## 背景

第 16 节建立 Compose 开发拓扑时，数据库还没有业务表。MySQL 首次初始化脚本创建 `growthos_app` 和 `growthos_migrator`；当时应用账号使用 schema 级 DML 作为后续占位。

第 18 节首次创建 `lottery_strategy` 和 `lottery_strategy_award` 后，安全审查发现该占位授权已不符合当前事实：

- `growthos_app` 可以读写 `schema_migrations`，有能力破坏迁移版本事实；
- API 尚无 Repository 和任何业务写用例，却拥有整个 schema 的 INSERT/UPDATE/DELETE；
- 只修改 `/docker-entrypoint-initdb.d` 脚本不会作用于已存在的 `mysql_data`，因为官方镜像只在首次初始化数据目录时运行它；
- 使用 root 网络连接执行授权会扩大高权限会话暴露面；
- 只追加 GRANT 不会撤销复用 volume 中遗留的旧权限；
- 只检查“所需 SELECT 存在”不能发现额外权限。

需要一个能同时支持新 volume 与复用 volume、在 Migration 之后执行、失败时阻断 API、并把授权收敛到完整 allowlist 的本地开发机制。

## 安全目标与非目标

### 目标

1. 应用账号在第 18 节只读两张真实业务表；
2. 应用账号不能访问 `schema_migrations`；
3. 已有 volume 上的旧通配 DML 能被撤销；
4. 高权限授权动作只在短生命周期 one-shot 中发生；
5. root 会话不通过 Compose 网络暴露；
6. 授权结果按“恰好相等”验证，存在任何额外 direct grant 时失败，并拒绝全局 mandatory role 隐式扩权；
7. Migration 未成功或授权偏差时 API 不启动；
8. 不删除用户已有数据库 volume 来“解决”权限漂移。

### 非目标

- 不把本地 Compose one-shot 当作生产 Secret manager、数据库变更平台或 PAM；
- 不在本节授予 Repository 尚未需要的写权限；
- 不提供任意 SQL、任意账号或通用 RBAC 管理器；
- 不声称 `network_mode: none` 消除了 Unix socket IPC；
- 不让 API、Migrator 或 Web 获得 root Secret。

## 评估过的方案

### 权限初始化/更新方式

| 方案 | 优点 | 问题 | 结论 |
| --- | --- | --- | --- |
| 只修改 init 脚本 | 新 volume 简单 | 已有 volume 不会重跑，旧授权保留 | 不足；init 只负责创建身份 |
| 删除 volume 后重建 | 能重跑 init | 破坏用户数据，掩盖真实升级路径 | 禁止作为常规方案 |
| Migrator 顺便 GRANT | 步骤少 | Migrator 当前不是 grant 管理身份；混合 schema 与账号权限生命周期 | 不采用 |
| API 启动时自检/授权 | 运行时知道需求 | API 必须持有高权限，违反最小权限 | 不采用 |
| 独立 one-shot 在迁移后精确收敛 | 对新旧 volume 都生效；职责和失败点清楚 | 多一个服务与 root Secret 挂载 | 采用 |

### 连接方式

| 方案 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- |
| TCP 加入 data 网络 | 配置直观 | root 客户端拥有网络面；DNS/端口面更大 | 不采用 |
| 宿主机执行 mysql CLI | 不增容器 | 依赖宿主工具、路径和凭据暴露；不可移植 | 不采用 |
| 共享 MySQL Unix socket + `network_mode: none` | 无网络栈；通信路径显式且局部 | socket 仍是高权限 IPC，需只读挂载和 Secret | 采用 |

### 授权更新策略

| 方案 | 优点 | 风险 | 结论 |
| --- | --- | --- | --- |
| 只执行目标 GRANT | 幂等简单 | 额外旧权限不会消失 | 不采用 |
| 针对已知旧 grant 逐条 REVOKE | 变更小 | 无法覆盖未知表/列/routine/角色漂移 | 不采用 |
| `REVOKE ALL PRIVILEGES, GRANT OPTION` 后按 allowlist GRANT | 收敛任意直接遗留 grant；审查简单 | 脚本错误会暂时移除应用权限，需 API gate | 采用 |

### 结果验证

| 方案 | 问题 | 结论 |
| --- | --- | --- |
| 测试两次 SELECT 成功 | 不能发现写权限/版本表权限仍存在 | 不足 |
| 搜索 SHOW GRANTS 包含目标行 | 不能发现额外 grant | 不足 |
| 排序后的完整 SHOW GRANTS 与精确文本 allowlist 相等 | 对 MySQL 输出格式敏感，但能 fail closed 检出多/少权限 | 采用并由固定 MySQL 8.4.11 基线验证 |

### one-shot 容器的数据目录

MySQL 官方镜像声明 `/var/lib/mysql` volume。即使该服务只作为客户端，Docker 也可能为它创建匿名可写 volume。评估：

| 方案 | 问题 | 结论 |
| --- | --- | --- |
| 忽略镜像 VOLUME | 产生无所有者匿名 volume，生命周期和清理不清楚 | 不采用 |
| 把真正 `mysql_data` 挂给授权容器 | root 客户端得到不必要的数据文件可见性；双容器挂同数据目录风险 | 禁止 |
| 显式空目录只读覆盖 `/var/lib/mysql` | 客户端不需要数据文件；避免匿名 volume；真实数据不可见 | 采用 |

## 决策

1. 保留首次初始化脚本只创建 `growthos_app`、`growthos_migrator`，并只给 Migrator 目标 schema 的审核 DDL/DML；初始化脚本不再给应用账号 schema wildcard DML。
2. 新增 Compose one-shot 服务 `mysql-grants`，固定使用与服务端相同的 MySQL 8.4.11 客户端镜像和 mysql UID/GID 999。
3. `mysql-grants` 依赖 MySQL healthy 且 `migrate` successful completion；API 同时依赖 `mysql-grants` successful completion 和 MySQL healthy。
4. MySQL 服务与授权服务共享 named volume `mysql_socket`。授权服务只读挂载 socket，设置 `network_mode: none`，不加入任何 Compose 网络。
5. 只有 `mysql-grants` 挂载 root Secret；API 只挂 app Secret，Migrator 只挂 migration Secret。
6. 授权脚本严格验证 root Secret 是 64 位小写十六进制，并使用 `MYSQL_PWD` + socket 调用本地 mysql client；不打印 Secret。
7. 脚本先对 `'growthos_app'@'%'` 执行 `REVOKE IF EXISTS ALL PRIVILEGES, GRANT OPTION`，再只授予：
   - `SELECT ON growthos.lottery_strategy`；
   - `SELECT ON growthos.lottery_strategy_award`。
8. 脚本读取并排序完整 `SHOW GRANTS`，与包含 `USAGE` 和上述两条表级 SELECT 的 allowlist 做精确相等比较；随后读取 `@@GLOBAL.mandatory_roles` 并要求为空。任何 direct grant 缺失/额外、强制角色隐式扩权或查询异常都非零退出。
9. 第 18 节不授予 INSERT/UPDATE/DELETE。第 19 节必须先实现并列举真实 Repository SQL，再修改 allowlist 与集成测试；权限变化必须可审查。
10. one-shot 容器使用 read-only root filesystem、drop ALL capabilities、no-new-privileges、只读脚本/Secret/socket，只有受限 `/tmp` tmpfs；`restart: "no"` 保留退出状态。
11. 一个受版本控制的空目录只读挂载到 `/var/lib/mysql`，覆盖镜像 VOLUME；不得挂载真实 `mysql_data`。
12. Compose smoke 必须检查 `migrate`/`mysql-grants` exit 0、完整 app direct grant 集、`@@GLOBAL.mandatory_roles` 为空、版本表访问拒绝、两表可读和无额外宿主端口。
13. 普通 `make compose-migrate` 在运行 migration 后必须再运行 grants reconciliation；单独恢复授权可显式使用 `make compose-grants`。

## 生命周期与信任边界

```text
长期数据：mysql_data
    │ 只挂 MySQL server
    ▼
mysql(root server process)
    │ 共享 mysql_socket
    ├───────────────┐
    │               │
Migrator(TCP)   mysql-grants(socket, root Secret, no network)
    │               │
    └─ clean v2 ────┴─ exact grants
                         │
                         ▼
                      API(app Secret, table SELECT only)
```

信任按生命周期递减：

- MySQL server 持有长期数据并接受受控身份；
- Migrator 有目标 schema DDL，但没有 root Secret；
- `mysql-grants` 短暂拥有 root 数据库能力，但看不到数据目录、没有网络，只执行固定脚本；
- API 是长期在线进程，只能读当前两张业务表；
- Web/Redis 不在该授权路径。

“root Secret + socket”仍是高权限边界。无网络与容器加固只能缩小攻击面，不能替代脚本审查、Secret 保护和短生命周期。

## 失败与恢复语义

| 失败点 | 可观察状态 | 恢复原则 |
| --- | --- | --- |
| MySQL 未 healthy | migrate/grants/API 不启动 | 先修复数据库，不绕过 gate |
| Migration dirty/失败 | grants 不运行，API 不启动 | 按 Migration runbook 核实副作用 |
| root Secret 无效 | grants 非零退出 | 修复 Secret 与既有 volume 身份一致性 |
| REVOKE 后某条 GRANT 失败 | 应用可能暂时无权限；grants 非零，API 不启动 | 修复目标表/脚本后重新运行完整 reconciliation |
| SHOW GRANTS 多/少权限 | grants 非零，API 不启动 | 查明漂移来源，不手动忽略差异 |
| `@@GLOBAL.mandatory_roles` 非空 | 即使 direct grants 相等也非零，API 不启动 | 核查强制角色的有效权限；要启用角色必须重新建模而非删除断言 |
| 复用 volume 含旧 wildcard | REVOKE 清除后按 allowlist 重建 | 无需删除 volume |
| app 权限未来扩展 | 当前脚本会收回未入 allowlist 权限 | 必须先更新 ADR/脚本/测试，不能手工长期添加 |

Reconciliation 不是数据库事务：REVOKE 和后续 GRANT 可能产生中间权限状态。安全性来自 API 尚未启动、脚本每次从“全部撤销”重新构建、失败可重跑和最终精确验证，而不是声称授权语句原子完成。

## 影响

### 正面影响

- 复用本地 volume 也能真正撤销历史通配权限；
- API 无法篡改 Migration 版本，也没有尚未使用的业务写能力；
- 高权限动作从长期 API/Migrator 中剥离；
- 授权状态与 schema 版本在同一启动 gate 中可观察；
- 精确 allowlist 能发现 direct grant 增长和缺失，mandatory role 断言还能阻断环境继承的隐式扩权；
- 用户无需删除业务数据 volume；
- 显式覆盖镜像 VOLUME 避免 one-shot 留下匿名数据卷。

### 成本与风险

- Compose 增加一个服务、一个 socket named volume 和启动步骤；
- root Secret 在 one-shot 执行期间仍可被该容器读取；
- 完整 SHOW GRANTS 文本比较和 mandatory role 语义依赖固定 MySQL 版本，需要升级时复核；
- REVOKE 与 GRANT 不是单一事务，失败会让应用权限暂时为空或不完整；
- socket 共享是显式 IPC，需要维护挂载、UID 和路径兼容性；
- 每次 Repository 权限变化都必须同步脚本、集成测试、smoke 和文档。

## 生产边界

生产环境不应无条件复制这个 Compose 方案。生产可使用部署平台 job、数据库角色、受控 DBA pipeline、短期凭据或云数据库权限管理，但必须保留相同原则：

- schema 迁移与在线身份分离；
- 权限按真实用例最小化；
- 可撤销旧权限而非只追加；
- 完整期望状态验证；
- 高权限凭据短生命周期；
- 失败阻断不兼容应用版本；
- 保留审计且不输出 Secret。

## 重新决策触发器

- 第 19 节 Repository 需要业务写权限；
- MySQL 升级改变 SHOW GRANTS 输出或账号语义；
- 应用拆成多个进程，需要按服务拆分身份；
- 引入只读副本、代理、云 IAM 或动态数据库凭据；
- 生产部署要求零中断权限变更或多实例兼容窗口；
- 需要 role 而不是用户直接 grant；
- socket 共享在目标运行环境不可用。

## 验收证据

- [授权收敛脚本](../../deploy/compose/mysql/grants/reconcile-growthos-app-grants.sh)
- [Compose 拓扑](../../deploy/compose/compose.yaml)
- [首次初始化身份脚本](../../deploy/compose/mysql/init/10-create-growthos-users.sh)
- [Compose smoke](../../scripts/compose-smoke.sh)
- [MySQL 权限集成测试](../../migrations/lottery_schema_integration_test.go)
- [第 18 节 QA](../qa/lessons/lesson-18.md)

## 参考

- [MySQL 8.4 GRANT Statement](https://dev.mysql.com/doc/refman/8.4/en/grant.html)
- [MySQL 8.4 REVOKE Statement](https://dev.mysql.com/doc/refman/8.4/en/revoke.html)
- [MySQL 8.4 SHOW GRANTS](https://dev.mysql.com/doc/refman/8.4/en/show-grants.html)
- [MySQL Docker Official Image](https://hub.docker.com/_/mysql)
- [ADR-0010：MySQL 连接、账号隔离与前向 Migration](ADR-0010-mysql-migration-boundaries.md)
- [ADR-0012：Compose 开发拓扑](ADR-0012-compose-development-topology.md)
