# 第 21 节 QA：临时 Lottery API、边缘契约与隔离纵向验收

- **对应章节：** [开放第一个 Lottery API](../../course/part-03/lesson-21-lottery-api.md)
- **API 记录：** [第 21 节 API 边界](../../api/lessons/lesson-21.md)
- **设计推导：** [第 21 节第一性原理设计手记](../../design-thinking/lessons/lesson-21.md)
- **面试复盘：** [第 21 节面试问答](../../interview/lessons/lesson-21.md)
- **长期决策：** [ADR-0018](../../decisions/ADR-0018-ephemeral-lottery-selection-api.md)
- **分支：** `codex/lesson-21-lottery-api`
- **起点：** `ea71640`（第 20 节最终检查点）
- **核心实现：** `65e9627`
- **边界加固：** `be41d92`
- **Transfer-Encoding 边缘修复：** `9100221`
- **隔离 Compose 验收：** `e32ecd4`
- **长期 smoke 稳健性：** `93f5694`
- **Trailer 声明边缘修复：** `3d4a44a`
- **验收临时目录身份加固：** `ef3f266`
- **请求边界测试补齐：** `7c43456`
- **初始验收日期：** 2026-08-29
- **最终复核日期：** 2026-08-30

> 本记录验收的是仅限 development/test、默认关闭、没有 Lottery 业务状态写路径的 ephemeral selection API；E2E 进一步证明两张 Lottery 业务表全列 fingerprint 前后相同。访问日志、运行指标等技术副作用仍会存在。它不能证明一次正式 Draw 已形成、用户有资格、结果可幂等查询、奖励已预占或发放、Redis 已接入业务、真实 React Lottery 页已完成，也不能证明生产容量、公平审计或公网安全。

## 1. 验收结论摘要

第 21 节通过以下边界：

1. 真实产品进程已装配 MySQL Repository、CryptoSource、WeightedSelector 与 application service；
2. 只有 development/test feature flag 明确开启时注册临时 selection route；
3. staging/production 无法通过配置打开该 route；
4. HTTP path、精确 demo/Idempotency-Key 规则、query 和可观察 body framing 失败关闭；其他未使用 header 并非全量 allowlist；
5. MaxUint64 Strategy/Award ID 在 path、Go、MySQL 和 JSON 中无损；
6. `reward` 与 `no_reward` 都是 200；
7. not-found、retryable/unavailable、stored-invalid 和内部不变量映射不同；
8. context 在 MySQL 前后和 selector 边界被观察，并如实记录同步 selector 不可抢占；
9. Strategy Award 数量上限为 1000，DB 读取用 1001 行探测超限；
10. `growthos_app` 当前只有两张 Lottery 表 SELECT；
11. Nginx 对进入 API location 的非空 Transfer-Encoding/Trailer 声明、size、timeout、Request ID 与命名网关错误做约束；Host 421 和 parser 早期拒绝不在统一 JSON 承诺内；
12. 一次性 Compose acceptance 真实通过 Nginx→Go→MySQL→CryptoSource；
13. 总计 64 个请求在最大并行 16 下只返回配置内 Award，调用前后业务表 fingerprint 不变；
14. API 停止时返回相关联 JSON 502/504，恢复后重新健康；
15. acceptance 按 Docker label/ID、临时目录 device/inode identity 和子文件类型验证删除目标，长期 `growthos` 资源身份不变；
16. 长期 Compose 已滚动到 lesson-21 镜像，保留 MySQL 数据卷并通过 smoke。

## 2. 验收环境

| 维度 | 实际值 |
| --- | --- |
| 宿主 | macOS 26.5.1，Apple Silicon arm64 |
| Go | `go1.26.6 darwin/arm64` |
| Docker Engine / Client | 29.7.2 / 29.7.2 |
| Docker Compose | v5.4.0 |
| Compose MySQL | 8.4.11 |
| Compose Nginx | 1.28.0-alpine3.21 镜像内运行；宿主未安装 nginx CLI |
| Node / 项目固定 pnpm | Node 24.19.0；镜像/项目固定 pnpm 10.13.1 |
| 宿主 pnpm | 11.22.0；不作为镜像可复现构建依据 |
| 时间/时区 | 2026-08-29～2026-08-30，Asia/Shanghai（初始验收至最终复核） |
| 长期 project | `growthos` |
| 长期 Web 入口 | `127.0.0.1:8088` |

宿主另有用户自己的 `mysql` 容器发布 3306。GrowthOS Compose 没有修改或删除它；项目 MySQL/API/Redis 仍不发布宿主端口，只有 Web 发布 8088。

## 3. 实现证据矩阵

| 风险命题 | 证据 | 结果 | 不能外推 |
| --- | --- | --- | --- |
| HTTP handler 偷做 SQL/算法 | application-owned ports、具体 adapter 只在 composition root 装配 | 通过 | 未来模块永远不会耦合 |
| 零/typed-nil service 延迟到首请求才失败 | constructor、`Validate`、main/router startup tests | 通过 | 所有第三方依赖都有健康语义 |
| ID 在 JS 边界丢精度 | MaxUint64 path + decimal-string DTO unit/E2E | 通过 | 前端已正确消费 string |
| `no_reward` 被误当系统错误 | unit + isolated MySQL fixture + real HTTP 200 | 通过 | 奖励资格/库存正确 |
| 返回伪造 Award | service 对 StrategyID 和 Award 全字段复核 | 通过 | 同进程供应链恶意代码可被完全隔离 |
| 慢/取消请求继续选 | context 前后检查与可取消 reader test | 通过 | selector 可被 context 抢占 |
| 超大聚合放大 CPU/内存 | domain max 1000 + SQL LIMIT 1001 + restore rejection | 通过 | 1000 是生产最优容量 |
| runtime API 身份误写数据 | exact SELECT-only grants + negative SQL probes + fingerprint | 通过 | migrator 凭据泄漏无风险 |
| body 契约绕过 | Go framing tests + Nginx 空 chunked/非空 Trailer/16KiB E2E | 通过 | parser 早期错误都统一 JSON，或未来 body route 无需重评 |
| Request ID 在 gateway timeout 断链 | edge 生成/验证/注入 + gateway envelope E2E | 通过 | 它等价于 trace/Draw ID |
| 并发访问产生竞态/越界结果 | race + 64 use-case goroutine + 100 handler goroutine unit + 64 requests at concurrency 16 E2E | 通过 | 达到生产高并发/QPS/P99 |
| endpoint 写入业务表 | request 前后全列 SHA-256 fingerprint | 通过 | 外部系统不存在副作用 |
| acceptance 污染长期环境 | 随机 project + Docker label/ID + temp directory identity/type cleanup + before/after identity | 通过 | 对抗同用户 TOCTOU 或任意未来脚本都安全 |
| API 停止返回 HTML/无关联 | real Nginx 502/504 JSON/no-store/request-ID | 通过 | HA/自动故障转移已实现 |

## 4. application 层验收

[`ephemeral_selection_test.go`](../../../internal/lottery/application/ephemeral_selection_test.go) 验证：

- Repository 收到同一个带 sentinel value 的 context；HTTP 层另有 request ID 传播测试；
- Selector 收到 Repository 恢复的 Strategy；
- 成功结果带同一 Strategy 与配置内 Award；
- nil 和 typed-nil reader/selector 被构造器拒绝；
- nil/zero-value service 的 `Validate` 与 `Select` 都失败关闭；
- nil context 和零 StrategyID 在依赖调用前拒绝；
- pre-canceled context 不触达 Repository；
- Repository 返回后发生取消时不调用 selector；
- 依赖返回 error 同时 context 已取消时，已观察的 cancel 优先；
- reader error 不继续 selector；
- selector error 不返回部分结果；
- 错 StrategyID、未知 AwardID 和同 ID 不同字段都拒绝；
- 64 个并发 use-case 调用在安全依赖下完成。

这里的“取消优先”只发生在同步依赖返回以后检查 `ctx.Err()` 的观察点。测试没有声称 Go 可以抢占一个永久阻塞的 selector。

## 5. HTTP adapter 验收

[`selection_test.go`](../../../internal/lottery/adapter/httpapi/selection_test.go) 覆盖：

### 5.1 成功 DTO

- `math.MaxUint64` StrategyID 和 AwardID 返回十进制 string；
- 响应只包含 `durability/strategy_id/award{id,name,outcome}`；
- 不包含 weight、total、Strategy name、DrawID 或 Location；
- `Cache-Control: no-store`；
- `reward` 200；
- `no_reward` 200。

### 5.2 严格请求边界

- 零、前导零、符号、URL 编码空白、小数、十六进制、溢出和非数字 ID；
- Idempotency-Key 出现，包括空值；
- demo header 缺失、错误和重复；
- 带参数的 query 与只有 `?` 的 force-query；
- 非零已知长度 `{}`、未知长度、chunked，以及 Go 能观察到的 Trailer；`Content-Length: 0` 与无 body 等价；
- 非 POST 方法 405 与 `Allow: POST`；
- 尾斜杠 404，不触发 307/308 POST 重放。

### 5.3 错误与日志

- not found → 404；
- cancel/deadline/repository retryable/random failure → 503；
- stored invalid/repository failure/composition/result/selector invariant → 500；
- client 4xx 只由 access log 记录；
- 5xx 模块日志包含规范 StrategyID 和稳定 error class；
- 原始 cause `never-expose` 不进入响应或日志；
- JSON error response 的 header/body request ID 相等；成功 selection body 不含 request ID。

### 5.4 deadline 与并发

- 20ms handler timeout 取消 Repository；
- selector 调用次数保持 0；
- JSON 503 在 gateway budget 前生成；
- 100 个并发 HTTP handler 共享 CryptoSource/WeightedSelector，只返回两个配置内 Award。

## 6. Repository 与容量加固

本节没有删除第 19 节 `Create` 能力，但在线 use case 只使用 `FindByID`。

新增边界：

```go
const MaxAwardsPerStrategy = 1000
```

并在三处防守：

1. `NewStrategy` / `RestoreStrategy` 拒绝 1001 个 Award；
2. MySQL query 使用 `LIMIT 1001`；
3. 若返回数大于 1000，恢复为 stored-invalid，不把截断聚合交给 selector。

这避免“数据库里有 1000000 行，应用全部读入后才发现异常”。`LIMIT 1001` 只限制返回/恢复工作；索引与查询计划仍决定数据库扫描成本，不能被写成绝对资源上限。

Repository 的共享错误分类器把以下故障归为 retryable；其中 1205/1213 是锁等待超时/死锁，不应笼统叫“只读连接故障”，分类同时可被 Create/FindByID 使用：

- MySQL 1205 / 1213；
- `driver.ErrBadConn`；
- `net.Error`。

Repository/use-case/selector 没有显式重试，所以分类主要决定上层 503 与可观测语义，不会再次执行选择。底层 `database/sql` 可能在事务建立前因 `driver.ErrBadConn` 淘汰连接并重新尝试 `BeginTx`；这不等于重放已开始的查询/selection。未来若引入业务 retry，需细化幂等、次数和 backoff。

## 7. 配置验收

新增：

```text
GROWTHOS_LOTTERY_EPHEMERAL_SELECTION_ENABLED=false
GROWTHOS_LOTTERY_SELECTION_TIMEOUT=3s
```

配置测试验证：

- boolean 仅接受合法文本；
- route 默认关闭；
- development/test 可启用；
- staging/production 启用失败；
- selection timeout 为正且不超过 30s；
- `selection + 1s <= HTTP write timeout`；
- `selection + 1s <= MySQL read timeout`；
- 聚合错误只列变量名和安全说明，不泄露 supplied secret/value。

Compose 当前值为：selection 3s、MySQL read 5s、HTTP write 10s；通过交叉校验。

## 8. Nginx 真实边界验收

单元测试无法观察 proxy 对原始 HTTP framing 的正规化，所以本节必须经过真实 Nginx。

已验证：

- 合法 Host 通过；`attacker.example` 返回带单一 no-store/request-ID 的 server-level 421；该 421 没有 JSON error envelope；
- 安全客户端 Request ID `acceptance.client:42` 被保留；
- 含空格的不安全 ID 被替换且不超过 64 字节；
- 普通 JSON body 被 Go 返回 JSON 400；
- 空 chunked/Transfer-Encoding 在 Nginx 返回 JSON 400；
- 非空 `Trailer: X-Lottery-Ticket` 声明在 Nginx 返回 JSON 400；空值与缺失在当前 map 中不可区分，实际 Go Trailer 仍由 adapter 拒绝；
- 16385 字节 Content-Length 在 Nginx 返回 JSON 413；
- 已进入 API location 的受测 400/413/502/504 只有一个 JSON Content-Type、一个 Cache-Control、一个 X-Request-ID；unsupported/invalid Transfer-Encoding 可在 location 前原生 501/HTML，HEAD wire 也不会返回 JSON body；
- API 停止后 Nginx 返回 JSON 502 或 504，并在恢复 API 后重新健康。

### 8.1 验证中发现的协议缺陷

首版 Go adapter 会拒绝 `Request.TransferEncoding`，但真实 Nginx 会在转发前解码空 chunked body。Go 最终看到的是普通零长度请求并返回 200。这说明“origin 单元测试通过”不足以证明 edge contract。

修复 `9100221` 在仍能看见客户端 Transfer-Encoding 的 Nginx 层拒绝它。修复后真实 acceptance 得到预期 `400 request_body_not_allowed`。

后续真实入口还暴露出非空 Trailer 声明在代理后对 Go 不可见；`3d4a44a` 在 Nginx 仍能观察该值时拒绝，并由 2026-08-30 完整运行的一次性 Compose acceptance 覆盖。该 map 不把空 Trailer 值与缺失 header 区分开，所以文档只承诺“非空声明”，不夸大为原始 header-presence 检测。

这不是把 Lottery 业务逻辑搬进 Nginx；它是在最早仍持有原始 framing 事实的组件执行协议校验。

## 9. 隔离 Compose acceptance 设计

命令：

```bash
make compose-lottery-api-acceptance
```

每次执行使用 24 个随机十六进制字符（96 bit）形成 project 后缀，并独立创建：

- Compose project；
- API/Migrator/Redis/Web image tag；
- Docker 分配的 `127.0.0.1` 端口；
- mysql_data/mysql_socket volume；
- Secret directory；
- response directory；
- Buildx docker-container builder/cache。

### 9.1 所有权验证

脚本删除任何资源前检查：

- container 的 Compose project/service label；
- volume 的 project 与 acceptance label；
- network 名称与 project label；
- image tag 对应的构建后 image ID 记录；
- builder 名、driver、node container 与 state volume；
- Secret/response 目录不是 symlink，且 cleanup 时的 device/inode 与创建记录一致；
- 每个预期 temporary child 不是 symlink，存在时必须是 regular file。

若所有权漂移，脚本拒绝扩大删除范围并返回失败，相关目标留给人工检查。检查仍不是针对同用户恶意并发替换的无竞态文件系统沙箱；脚本不使用 `docker system prune`、`volume prune` 或模糊 project 名。

### 9.2 fixture

隔离 migrator 身份原子写入：

| Strategy | 用途 |
| --- | --- |
| `18446744073709551615` | MaxUint64 ID/Award/weight 边界 |
| `21002` | 唯一 `no_reward` |
| `21003` | 1:3 reward/no_reward 多候选 |

`growthos_app` 随后只能 SELECT，并精确验证 fixture shape `3:4:4`。

### 9.3 真实业务断言

通过：

- services healthy/completed；
- migration `2:0`；
- exact SELECT-only grants；
- host port 唯一属于本次 Web container；
- health/ready build version lesson-21；
- Host 421；
- MaxUint64 reward；
- no_reward；
- safe/unsafe request ID；
- missing/invalid/demo/query/method/path/body/chunked/non-empty Trailer/idempotency；
- JSON 413；
- 64 个 multi-award 请求、最大并行 16；
- JSON 502/504 与 API recovery；
- before/after business fingerprint 相等；
- post-traffic migration/grants/port 不漂移。

最终输出：

```text
ok - 64 multi-award requests at concurrency 16 returned only configured outcomes
ok - the gateway returned correlated JSON 504 and the disposable API recovered healthy
ok - lesson-21 isolated Compose acceptance passed for growthosl21521999b441d6c992a7785a24
ok - removed only label/ID-verified Docker resources and identity/type-verified temporary files
ok - the long-lived growthos project resource identity remained unchanged
```

随机 project 名仅用于证明本次执行；它已经被删除，不能再作为运行目标。

## 10. 验收过程中真实暴露并修复的问题

### 10.1 Docker Hub 暂态 manifest 500

首次拉取 Redis 基础镜像遇到 registry 500。它属于外部供应链可用性，不是业务测试失败；重试后继续。脚本没有把网络故障伪装成测试通过。

### 10.2 TTY 下只读临时 Secret 清理提示

BSD `rm` 对只读文件在 TTY 下可能询问。脚本已经在删除前精确证明目标为本次 regular file，随后使用 `rm -f` 使 cleanup 确定，不扩大目录范围。

### 10.3 `docker ps --filter publish` 所有者判断含糊

publish filter 无法精确表达 HostIp 和完整 HostPort。脚本改为遍历运行容器的 `NetworkSettings.Ports`，只接受 `HostIp == 127.0.0.1` 且 HostPort 精确匹配，并要求唯一 container ID。

### 10.4 尾斜杠错误文案预期漂移

全局 router 的 404 message 是 `resource not found`，不是局部假设的 `route not found` 文本。acceptance 对齐已发布全局 envelope，同时仍按稳定 code 断言。

### 10.5 空 chunked 被 proxy 正规化

见第 8.1 节。真实入口暴露单元测试看不到的协议差异，最终在 Nginx 边缘修复。

### 10.6 API 停止可能得到 504 而非 502

Docker/Nginx 可能仍解析到停止容器 endpoint，连接表现为 blackhole/inactivity；也可能直接连接失败。脚本接受 502 或 504，但对实际 status 分别要求精确 code/message/request-ID/no-store，而不是把任意 5xx 当通过。

### 10.7 长期 Web 镜像版本漂移

初次 `make compose-smoke` 发现 API 已是 lesson-21，但 Web/Nginx 仍是 lesson-16。执行 `make compose-up` 只重建/滚动服务，保留 `growthos_mysql_data` 和 `growthos_mysql_socket`；随后所有服务为 lesson-21 并通过 smoke。

### 10.8 固定 MaxUint64 not-found 假设不稳健

长期 smoke 原先假设 MaxUint64 Strategy 永远不存在；开发者完全可能合法插入它。`93f5694` 改为通过 SELECT-only app 身份推导一个真正缺失的 canonical uint64，再调用 404，不修改用户数据。

### 10.9 非空 Trailer 声明在代理后消失

Go adapter 能拒绝自己观察到的 `Request.Trailer`，但真实 Nginx 转发普通 `Trailer: X-Lottery-Ticket` 后，Go 不再拥有原始声明事实。`3d4a44a` 在 API location 仍能读取非空 `$http_trailer` 时返回命名 JSON 400；最终隔离验收证明了这一真实边界。空 Trailer 值仍与缺失不可区分，未被夸大为“任意原始 header presence”。

### 10.10 临时目录本身缺少身份复核

首版 cleanup 会检查每个子文件不是 symlink 且是 regular file，但目录只检查 `-d`，文档因而不能声称目录也已验证所有权。`ef3f266` 在创建 Secret/response 目录后记录 device/inode，清理前先拒绝 symlink、非目录或 identity 漂移，再逐个检查精确子文件。它缩小了误删范围，但仍不包装成对同用户恶意 TOCTOU 的绝对防护。

## 11. 实际执行的代码质量门禁

代码 checkpoint 已执行并通过：

```bash
make fmt-check vet test doc-check
go test -race -count=1 ./...
go test -shuffle=on -count=10 ./internal/lottery/application ./internal/lottery/adapter/httpapi ./internal/lottery/adapter/mysqlrepo ./internal/lottery/domain ./internal/lottery/adapter/randomsource
make compose-config
sh -n scripts/compose-lottery-api-acceptance.sh scripts/compose-smoke.sh
shellcheck scripts/compose-lottery-api-acceptance.sh scripts/compose-smoke.sh
git diff --check
```

静态 acceptance overlay config 也使用显式 dummy project/image/secret directory 执行 `docker compose ... config --quiet` 并通过。

这些命令通过不等于正文、索引和 QA 自动正确；文档提交后仍需重新运行统一门禁。

## 12. 长期 Compose 回归

长期栈更新后实际执行：

```bash
make compose-up
make compose-smoke
```

smoke 通过：

- MySQL/API/Redis/Web running + healthy；
- migrate/mysql-grants exited 0；
- API/Migrator/Web image 为 lesson-21；
- schema clean latest 2；
- name constraints 与 migration 一致；
- `growthos_app` 两表 SELECT-only，无 INSERT/UPDATE/DELETE/migration table；
- `/health`、`/ready`、SPA、unknown route；
- 动态缺失 StrategyID 的 ephemeral route 真实到达 MySQL并返回相关联 no-store 404；
- 只有 Web 发布 `127.0.0.1:8088`。

保留的长期资源：

- `growthos_mysql_data`，创建于 `2026-08-29T07:43:46Z`；
- `growthos_mysql_socket`，创建于 `2026-08-29T09:21:15Z`；
- 用户已有业务数据与 Secret；
- healthy development services，便于继续第 22 节。

## 13. 清理与保留

本节一次性验收已删除：

- 最终复核随机 project `growthosl21521999b441d6c992a7785a24` 的 containers；
- 三个 project networks；
- 两个 project volumes；
- 四个本次 acceptance image tag 与对应无其他引用镜像；
- 本次 Buildx builder/node/state cache；
- 本次临时 Secret 和 response 文件/目录。

清理后额外确认：

- project label 下无 container/volume/network；
- `growthos/acceptance-*` 无残留 tag；
- builder 名不存在；
- 临时目录名无残留；
- 长期 `growthos` project 容器、卷和网络身份与运行前快照相同。

未删除：

- 长期 MySQL 数据卷；
- 长期 Secret；
- 可复用项目依赖；
- Docker 基础镜像/正常 build cache；
- 用户独立安装的 MySQL、Redis、RabbitMQ、PostgreSQL 等资源。

## 14. 独立审查结果

核心代码、HTTP contract 和最终代码经过独立只读审查。初审发现的主要问题包括：

- endpoint 暴露内部 Strategy/权重且缺 Activity/资格边界；
- 没有真实 success Compose 验收；
- selection timeout 被夸大；
- deadline 与依赖 error 竞态；
- zero-value service 可启动；
- driver timeout budget 顺序；
- Award 数量无上限；
- runtime 仍持 INSERT；
- Nginx request-ID、body、Host 与 gateway JSON 边界。

这些问题在 `be41d92`、`9100221`、`e32ecd4`、`3d4a44a`、`ef3f266` 与 `7c43456` 中处理：route 被限定为 dev/test ephemeral、DTO 最小化、service startup validation、context 优先、budget 交叉校验、1000 上限、SELECT-only、Nginx framing/Trailer 加固、临时目录身份复核、请求分支测试与一次性真实 acceptance。

最终独立审查没有 P0/P1/P2。保留的 P3 维护建议：

- >1000 Award 可增加更强的独立真实 DB 读取证据；
- feature flag 关闭后 route 不注册可补直接测试；
- duplicate safe Request ID 可增加更多 edge E2E；
- lesson-19 写集成测试的命名可更明确区分隔离身份。

这些不会改变本节公开边界，也不能被隐去为“零风险”。

## 15. 当前剩余风险

1. 无用户认证和对象级授权，不能公开部署；
2. StrategyID 可枚举，404 会暴露存在性；
3. 无 rate limit，重复采样仍可消耗 DB/熵并统计估计权重；
4. demo header 不是安全凭据；
5. feature flag 不是访问控制；
6. selector 不接受 context，deadline 只在返回后观察；
7. success response 没有 Strategy version，无法历史解释配置变化；
8. Award name 尚无发布、本地化或内容审核投影；
9. Request ID 不是 DrawID、trace 或 audit；
10. Nginx 和 Go 各有 error template，未来需防漂移；
11. 全 `/api` Transfer-Encoding/16KiB 策略会影响未来 body/stream route；
12. 503 不代表可安全重放同一结果；
13. Redis 尚未接入，不能声称锁、限流或缓存；
14. 前端 Lottery 页面仍是 Mock/`Math.random()`，不能作为后端证据；
15. 64/100 goroutine 单测与 64 次请求、最大并行 16 的 E2E 不是业务压测；
16. CSPRNG 与无偏算法不等于公平治理或法规审计。

## 16. 能准确表述与不能表述

可以表述：

> 实现开发/测试专用、无 Lottery 业务状态写路径的临时 Selection API，将 MySQL 一致快照、无偏加权选择与密码学随机源通过 application ports 装配到 Gin；建立完整 uint64 string DTO、稳定错误/超时/Request-ID、Nginx framing 与 SELECT-only 权限边界，并用一次性 Compose 环境验证真实纵向链、64 次请求（最大并行 16）、网关故障恢复及两张业务表全列 fingerprint 不变。

不能表述：

- 已上线生产抽奖；
- 已实现 Draw exactly-once/幂等；
- 已完成用户资格、次数、库存和发奖；
- Redis 锁/缓存/限流已使用；
- 通过高并发压测；
- 已证明概率合规、公平或不可篡改；
- 前端已真实联调；
- 502/504 自动重试一定安全。

## 17. 验收结论

第 21 节通过。GrowthOS-Go 第一次拥有可经真实 Nginx 调用的 Lottery 业务 API，并且该 API 的名称、配置、请求、DTO、数据库权限和 QA 都明确承认它只是一次 ephemeral selection。

这份成功的价值不在“多了一个 POST”，而在于证明前四节的分层能力可以组成一条真实纵向链，同时主动拒绝了尚未建立的最终结果、幂等、授权、限流和发奖承诺。第 22 节可以把 React 页的结果来源切到该 API，但不能让动画、重复点击或刷新伪造成正式 Draw 语义。
