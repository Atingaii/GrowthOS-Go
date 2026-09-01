# Compose 本地秘密目录

运行 `make compose-secrets` 会在本目录创建以下仅供本机使用的随机秘密。

五份数据库/缓存凭据使用 64 位小写十六进制文本（32 bytes 随机输入）：

- `mysql_root_password`
- `mysql_app_password`
- `mysql_migration_password`
- `mysql_identity_password`
- `redis_password`

两份 Identity 协议密钥各自使用恰好 32 个原始随机 bytes，不是 64 位十六进制文本，也不能互相复用：

- `identity_throttle_hmac_key`
- `identity_csrf_active_key`

这些文件被本目录的 `.gitignore` 排除，不能提交、截图、复制到 QA 文档、通过聊天发送，也不要用 `cat` 打印二进制密钥。仓库只保存本说明、忽略规则以及 [`configs/growth-api.env.example`](../../../configs/growth-api.env.example) 中不含秘密的文件路径示例。生成器会把本目录设为 `0700`、秘密文件设为 `0444`：文件级读权限是为了兼容 Compose 将本地秘密实现为容器内 root 所有的只读 bind mount；宿主机上的目录遍历权限仍只属于当前用户，并且只有 API 挂载两份协议密钥。

生成器按七文件完整集合创建与验证，全部不存在时一次生成。为兼容真实的线性升级，它只额外接受两种精确 partial 状态：

1. 第 31 节留下的四份旧凭据完整时，验证旧值后只新增 `mysql_identity_password` 和两份协议密钥；
2. 第 32 节早期留下的五份数据库/缓存凭据完整时，验证旧值后只新增两份协议密钥。

除此之外的任意部分集合都会失败；已有 MySQL volume 却丢失整套秘密时也会失败。生成器不会覆盖已有秘密，不会用文本命令替换读取二进制密钥，也不会悄悄生成一套与数据库账号不匹配的新密码。若安装过程中断并留下新的 partial 状态，应先核对文件来源再恢复缺失文件，不能反复删除和重生。

生成器通过本目录下的 `.generate.lock` 原子目录拒绝并发写入。正常退出和可处理的中断会自动移除它；`SIGKILL`、宿主断电或文件系统故障可能留下 stale lock。遇到锁错误时先确认没有生成器进程，再确认该路径是本 Secret 目录内一个空的、非符号链接目录；只有这些条件全部成立，才可精确 `rmdir deploy/compose/secrets/.generate.lock`，随后按上面的 4/5/7 文件矩阵核对。不要递归删除 Secret 目录，也不要为了越过锁而复制或重生已有值。

改变文件内容不会自动修改已初始化 MySQL volume 中的账号密码。密码轮换必须显式更新数据库账号和秘密文件。两份 Identity key 也不能无意自动轮换：

- throttle HMAC key 改变后，旧 login/source digest 行不可达，登录失败预算会瞬时重置，旧行只会等待 24 小时有界清理；它不会撤销 session；
- active CSRF key 改变时还必须在同一轮启动中通过 `GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID` 改变默认 `local-v1` key id。未配置 previous key 的本地模式会使既发 CSRF token 失效，但 session Cookie 本身仍可 resolve，客户端可通过 current-session 响应取得新 token。

容器重启后才会重新读取文件，Compose file secret 不是热更新的 Secret Manager。数据重置也必须由操作者明确决定，不能通过自动删除 volume 假装完成。
