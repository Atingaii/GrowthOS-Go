# 第 18 节 API 记录：业务 schema 已存在，但 Lottery API 仍不存在

- **章节：** [第一次正式业务建表](../../course/part-03/lesson-18-lottery-schema.md)
- **日期：** 2026-08-29
- **状态：** 两张 Lottery 业务表与精确只读权限已实现；没有新增、修改或删除 HTTP API
- **QA：** [第 18 节 QA](../../qa/lessons/lesson-18.md)
- **ADR：** [ADR-0014](../../decisions/ADR-0014-lottery-persistence-schema.md)、[ADR-0015](../../decisions/ADR-0015-compose-schema-grant-reconciliation.md)

## 1. 本节 API 结论

第 18 节只改变 MySQL schema、Migration 版本和 Compose 启动/授权门，不开放任何 Lottery HTTP route。

| 类型 | 路径 / 契约 | 第 18 节状态 |
| --- | --- | --- |
| 系统 liveness | `GET /health` | 保持既有契约，不变 |
| MySQL readiness | `GET /ready` | 继续执行有界 Ping；不检查 Lottery 行数或 Repository |
| 统一 404/405/500/503 envelope | 既有错误契约 | 不变 |
| Nginx `/api` 转发命名空间 | 基础设施能力 | 仍没有 Lottery handler |
| Strategy 查询 API | 未定义 | 第 19 节 Repository 之后再决定 |
| Draw API | 未定义 | 第 20 节算法、第 21 节接口之后再决定 |
| Strategy 管理 API | 未定义 | 没有创建/编辑/删除用例 |
| React `/lottery` 数据源 | Mock | 没有读取本节数据库表 |

因此，未来可能使用的路径当前仍应作为未知路由处理。例如：

```http
GET /api/lottery/strategies/1 HTTP/1.1
Host: 127.0.0.1
Accept: application/json
```

或：

```http
POST /api/lottery/draw HTTP/1.1
Host: 127.0.0.1
Content-Type: application/json
```

当前都不应返回 Strategy/Award/抽奖结果，只能命中既有统一 404。示例路径不是已经承诺的版本化接口；第 21 节必须根据真实用例重新决定 path、method、认证、幂等、DTO 和错误语义。

## 2. 数据库结构不是 HTTP 契约

本节新增的列：

```text
lottery_strategy(strategy_id, name, created_at, updated_at)
lottery_strategy_award(strategy_id, award_id, name, weight, outcome, created_at, updated_at)
```

不能直接复制成 JSON：

```json
{
  "strategy_id": 18446744073709551615,
  "name": "示例",
  "created_at": "...",
  "updated_at": "...",
  "awards": []
}
```

原因包括：

- JavaScript `number` 不能精确表示整个 MySQL `BIGINT UNSIGNED` / Go `uint64` 范围；
- 表行时间戳只是存储元数据，不一定属于外部产品契约；
- Award 是多行映射，尚无 Repository 保证一致读取和领域重建；
- 空 Awards 在表层可以存在，但领域 Strategy 非法；
- SQL `name_basic` 只校验首尾 U+0020 子集，不能代替完整领域名称规则；
- `outcome=reward` 只表示候选类别，不表示已抽中或已发放；
- AwardID 只有与 StrategyID 组合才完整标识一个候选；
- 没有决定客户端是否需要权重、计算后概率、管理元数据或展示顺序。

正确的未来边界仍然是：

```text
MySQL rows
    │ 第 19 节：扫描、完整加载、领域构造器 fail closed
    ▼
Strategy aggregate
    │ 第 20/21 节：用例与 transport mapping
    ▼
HTTP DTO
```

## 3. 本节没有 Repository，所以 API 不能读取两张表

Compose 将 `growthos_app` 收敛为两张业务表的 SELECT only，只表示数据库权限已经为下一节准备了最小读取面。当前 `growth-api`：

- 没有 `lottery.Repository` 接口；
- 没有 `SELECT ... JOIN` 或两次查询；
- 没有 row struct；
- 没有从 SQL 值调用 `NewAward` / `NewStrategy`；
- 没有 not found 与 corrupt data 错误；
- 没有事务一致性策略；
- 没有 Strategy 服务或 handler。

因此“API 账号能 SELECT”与“API 已能返回 Strategy”是两个不同命题。前者由权限测试证明，后者在本节明确为假。

## 4. `/ready` 没有升级为 schema 完整性探针

`GET /ready` 仍然只证明 API 当前能通过现有连接池 Ping MySQL。它不证明：

- Migration 当前一定为 version 2；
- `lottery_strategy` 两表一定存在；
- 应用账号拥有所有未来业务 SQL 权限；
- Strategy 至少一个 Award；
- 权重和名称数据符合领域不变量；
- Repository 查询能在 timeout 内完成；
- Redis、算法、Activity、库存或 Benefit 可用。

Compose 启动 gate 会在 API 容器启动前等待 Migration 与权限收敛成功，这是发布编排证据；`/ready` 是运行时依赖可达性证据。二者不能互相替代。

如果未来业务 readiness 需要检查 schema compatibility，应设计低成本、稳定且不会把数据质量偶发问题变成进程抖动的机制，而不是每次探针全表扫描或构造所有 Strategy。

## 5. 当前没有请求 DTO

第 18 节没有定义：

- 创建 Strategy 的 body；
- Award 数组、顺序与数量上限；
- ID 由客户端提供还是服务端生成；
- Weight 用 JSON string、number、百分数还是固定基数；
- Outcome 外部 enum 的版本兼容；
- `created_at` / `updated_at` 是否客户端可见；
- 更新是全量替换、patch 还是版本化发布；
- 删除、归档、草稿和审核；
- 鉴权角色与操作审计；
- 幂等 key；
- Activity/Participation/Benefit 关联。

直接拿表列生成 CRUD，会让传输层把尚不存在的业务用例变成长期承诺。

## 6. 当前没有响应 DTO

本节也没有合法的 HTTP 响应能表达：

```json
{
  "strategy_id": "1",
  "award_id": "2",
  "outcome": "reward",
  "delivered": true
}
```

表中保存的 `reward` 是配置候选类别，不是一次 Draw 的选择事实；更不是 Benefit 发放状态。当前没有 draw ID、request ID、用户、策略版本、Award 快照、幂等结果或交付记录。

`no_reward` 同样不是 HTTP error。未来算法选中 `no_reward` 时仍应属于一次成功完成的业务结果；依赖失败、脏配置和结果未知必须使用不同错误/查询语义。第 18 节只保存候选配置，没有实际选择发生。

## 7. 当前没有 HTTP 错误映射

数据库已能产生一组明确错误，例如：

- CHECK violation 3819；
- 外键插入 violation 1452；
- 父行受引用删除 violation 1451；
- 复合主键重复 1062；
- 名称过长 1406；
- API 账号权限拒绝 1142。

这些 MySQL error number 只用于 adapter/integration test 识别数据库行为，不能原样暴露为 API 契约。未来 Repository/handler 必须回答：

- 哪些属于输入非法，哪些属于内部数据损坏；
- 重复键是否是幂等成功、冲突还是实现 bug；
- 外键错误是否可以被客户端修正；
- 依赖不可用与领域不变量失败怎样区分；
- 如何返回稳定公开 code，同时不泄露表名、约束名、SQL 和驱动 cause。

本节没有 handler，因此没有提前绑定 400/404/409/422/500。

## 8. 权限变化不构成 API 能力变化

第 18 节 Compose 下应用身份的精确授权为：

```text
SELECT growthos.lottery_strategy
SELECT growthos.lottery_strategy_award
```

它明确没有：

- INSERT/UPDATE/DELETE；
- CREATE/ALTER/DROP/INDEX/REFERENCES；
- `schema_migrations` SELECT 或 UPDATE；
- root 或 Migrator 权限；
- 隐式 mandatory role 扩权（本地门禁要求为空）。

第 19 节若实现写 Repository，必须先确定 SQL 和事务，再增加最小表级 DML，并更新 grant allowlist 与负向测试。HTTP API 是否允许写还要再经过用例、授权和 DTO 决策，不能因为数据库账号获权就自动开放。

## 9. 前端与缓存边界

### 9.1 React

`/lottery` 仍是 Mock 演示。它没有：

- 从 Go API 获取 Strategy；
- 展示本节数据库中的 Award；
- 以 `uint64` 安全格式传 ID/Weight；
- 发起真实 Draw；
- 显示服务端确认的 reward/no_reward；
- 保存或查询最终结果。

### 9.2 Redis

Redis 仍是 Compose 内部 cache 网络上的隔离占位。API 没有 Redis client，数据库表没有缓存镜像，`updated_at` 也没有被定义为 cache version。当前不存在 key、TTL、miss、回源、失效、穿透、击穿或一致性协议。

## 10. 对第 21 节 API 设计的输入

本节能给未来 API 的只是持久化事实：

1. StrategyID/AwardID/Weight 的存储上限为完整 `uint64`；
2. Award 外部身份至少需要 StrategyID + AwardID；
3. Outcome 原始值严格区分大小写且只允许两个值；
4. 名称最多 128 个 MySQL 字符，但完整领域合法性由构造器决定；
5. `created_at/updated_at` 是行元数据，不是默认公开字段或版本；
6. 一个 Strategy 的 Awards 需要聚合加载，不能把单行当完整对象；
7. 数据库不能保证至少一个 Award 或总权重不溢出；
8. `reward` 不代表发放，`no_reward` 不代表错误。

第 21 节还需依赖第 19～20 节补齐：

- Repository not found/corrupt/dependency error；
- 一致读取和 timeout；
- 加权选择算法与随机源；
- Draw 用例输入、输出和错误；
- 是否已有幂等和最终结果持久化；
- 用户/Activity/Participation 当前是否存在；
- JSON 中 uint64 的编码；
- HTTP 取消后服务端行为；
- 认证、授权、限流、审计与隐私；
- Mock 前端迁移策略。

## 11. API 负向验收

本节应验证“没有业务接口漂移”，而不是伪造成功响应：

```bash
go test ./internal/infrastructure/httpapi
make compose-smoke
```

Compose smoke 继续请求一个不存在的 `/api/**` 路径，并要求返回既有 `route_not_found` JSON、稳定 message、body/header 一致 request ID。它还能证明 schema/grants gate 完成后已有 `/health`、`/ready` 与 SPA 未回归。

这组检查不能证明 Lottery API 可用，因为 Lottery API 在本节有意不存在。

## 12. 下一节

第 19 节建立 Repository 端口和 MySQL adapter，重点是“能否从两张表可信地恢复同一个 Strategy 聚合”。它仍不必开放 HTTP API。第 20 节实现最小加权算法后，第 21 节才有足够事实设计第一个 Lottery API。
