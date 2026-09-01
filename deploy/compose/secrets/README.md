# Compose 本地秘密目录

运行 `make compose-secrets` 会在本目录创建以下仅供本机使用的 64 位十六进制随机秘密：

- `mysql_root_password`
- `mysql_app_password`
- `mysql_migration_password`
- `mysql_identity_password`
- `redis_password`

这些文件被本目录的 `.gitignore` 排除，不能提交、截图、复制到 QA 文档或通过聊天发送。生成器会把本目录设为 `0700`、秘密文件设为 `0444`：文件级读权限是为了兼容 Compose 将本地秘密实现为容器内 root 所有的只读 bind mount；宿主机上的目录遍历权限仍只属于当前用户，并且每个服务只挂载自己声明的秘密。

生成器按完整集合创建：五个文件全部存在时只验证格式；全部不存在时一次生成。第 32 节升级只额外接受“旧版四个文件均完整、仅缺少 `mysql_identity_password`”这一精确状态：它先验证四个旧秘密，再只为尚不存在的新 Identity 账号原子生成第五个秘密，绝不改写旧账号密码。除此之外的任意部分集合都会失败；已有 MySQL volume 却丢失整套秘密时也会失败。它不会覆盖已有秘密，也不会悄悄生成一套与数据库账号不匹配的新密码。

改变文件内容不会自动修改已初始化 MySQL volume 中的账号密码。密码轮换必须显式更新数据库账号和秘密文件；数据重置也必须由操作者明确决定，不能通过自动删除 volume 假装完成。
