# GrowthOS-Go 配置参考

**状态：** 第 12 节已完成并验收

**更新日期：** 2026-08-29

**来源章节：** [第 12 节：配置、日志与错误体系](course/part-02/lesson-12-config-logging-errors.md)

本页记录当前 `growth-api` 进程真正读取的配置边界。配置项必须集中加载、校验后再传给各基础设施组件；业务包和 HTTP handler 不得随处调用 `os.Getenv`。

## 1. 加载规则

当前只使用两层配置：

```text
代码内安全默认值
        ↓ 被显式覆盖
进程环境变量 GROWTHOS_*
```

- 所有项目自有环境变量统一使用 `GROWTHOS_` 前缀，避免与宿主机通用变量冲突；
- 没有设置变量时使用表中的默认值；变量**已经存在但值为空**时视为配置错误，不回退默认值；
- 枚举值严格使用小写，避免不同环境对大小写作出不同解释；
- duration 使用 Go duration 语法，例如 `500ms`、`5s`、`2m`；裸数字不是 duration；
- 启动时一次性加载并校验，不提供运行时热更新；
- `configs/growth-api.env.example` 是无秘密示例，不是自动加载的 `.env` 文件。运行环境应显式注入变量。

## 2. 当前配置项

| 环境变量 | 默认值 | 允许值 / 校验 | 用途 |
| --- | --- | --- | --- |
| `GROWTHOS_ENVIRONMENT` | `development` | `development` / `test` / `staging` / `production` | 标记运行环境，供日志和后续环境策略使用 |
| `GROWTHOS_HTTP_ADDRESS` | `:8080` | 合法 `host:port`，端口为 1～65535 | HTTP 监听地址 |
| `GROWTHOS_HTTP_SHUTDOWN_TIMEOUT` | `5s` | `> 0` 且 `<= 2m` | 优雅关闭最长等待时间 |
| `GROWTHOS_HTTP_READ_HEADER_TIMEOUT` | `5s` | `> 0` 且 `<= 30s` | 读取请求头的最长时间 |
| `GROWTHOS_HTTP_READ_TIMEOUT` | `15s` | `> 0` 且 `<= 5m` | 读取完整请求的最长时间 |
| `GROWTHOS_HTTP_WRITE_TIMEOUT` | `30s` | `> 0` 且 `<= 10m` | 写响应的最长时间 |
| `GROWTHOS_HTTP_IDLE_TIMEOUT` | `60s` | `> 0` 且 `<= 10m` | keep-alive 空闲连接最长时间 |
| `GROWTHOS_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` | `slog` 最低输出级别 |
| `GROWTHOS_LOG_FORMAT` | `json` | `json` / `text` | 选择结构化日志编码 |

以上枚举严格区分大小写。`appconfig.Load` 只读取调用方传入的 `LookupFunc`；`cmd/growth-api` 显式传入与 `os.LookupEnv` 同签名的查询函数。`appconfig.Default()` 提供同一组默认值，测试不需要修改开发者的真实进程环境。

## 3. 无秘密示例

版本库中的示例只展示可以公开的运行参数：

```dotenv
GROWTHOS_ENVIRONMENT=development
GROWTHOS_HTTP_ADDRESS=:8080
GROWTHOS_HTTP_SHUTDOWN_TIMEOUT=5s
GROWTHOS_HTTP_READ_HEADER_TIMEOUT=5s
GROWTHOS_HTTP_READ_TIMEOUT=15s
GROWTHOS_HTTP_WRITE_TIMEOUT=30s
GROWTHOS_HTTP_IDLE_TIMEOUT=60s
GROWTHOS_LOG_LEVEL=info
GROWTHOS_LOG_FORMAT=json
```

可以在 shell 中显式注入单个覆盖值：

```bash
GROWTHOS_HTTP_ADDRESS=127.0.0.1:18080 \
GROWTHOS_LOG_LEVEL=debug \
go run ./cmd/growth-api
```

示例文件不得包含密码、Token、Cookie、私钥、DSN 或真实内网地址。第 13 节出现数据库连接后，秘密应通过本地未跟踪环境或部署平台的 Secret 机制注入；不能因为变量带有 `GROWTHOS_` 前缀就把值提交到 Git。

## 4. 失败语义

配置错误必须在打开监听端口前使进程启动失败。加载器聚合能够一次发现的错误，并只报告环境变量名称和违反的约束，不回显原始值，避免未来的秘密值进入终端或日志。

以下情况都不能静默回退：

- 已设置但为空；
- 地址缺少端口、端口不是数字或超出 1～65535；
- duration 无法解析、等于零、为负数或超过对应上限；
- 环境、日志级别或日志格式不是受支持的小写枚举。

启动失败属于进程配置问题，不经过 HTTP 错误 envelope，因为此时服务尚未开始监听。

## 5. 配置所有权

```text
internal/platform/appconfig
  ├─ 读取 GROWTHOS_* 与应用默认值
  ├─ 解析、聚合并返回配置错误
  └─ 输出类型化 Config
             │
             ├─> logging：级别、格式、environment
             └─> httpserver：地址与五个 timeout
```

`httpserver` 只接收已校验的类型化值，不读取环境变量；`httpapi` 不负责应用配置。以后数据库、缓存或消息配置也应先进入同一应用配置边界，再注入相应适配器，不能复制一套解析规则。

## 6. 变更规则

新增或修改配置时必须同步：

1. 类型化配置结构、默认值和校验测试；
2. `configs/growth-api.env.example`；
3. 本配置参考与对应课程正文；
4. 需要时更新部署资产和 QA 冒烟命令；
5. 若改变安全、兼容或长期运维约束，新增或替代 ADR。

当前尚未实现配置热更新、远程配置中心或秘密管理服务。这些能力只有出现实际部署问题后才引入。
