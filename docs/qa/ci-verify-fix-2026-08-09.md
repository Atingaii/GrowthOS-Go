# GitHub Actions 质量门禁修复验收

- **日期：** 2026-08-09
- **对象：** `.github/workflows/quality.yml`
- **失败运行：** `quality / verify`，Run `31295484606`，Job `93199781289`
- **结果：** 本地干净环境验证通过，等待修复提交的远端 Actions 验证

## 故障现象

推送提交 `c283ea1` 后，GitHub Actions 的 `quality / verify` 在约 17 秒后失败，`make verify` 返回退出码 2。

公开 Actions API 证据显示：

- checkout 成功；
- Go 环境安装成功；
- `make verify` 步骤失败；
- setup-go 提示仓库没有 `go.sum`，无法恢复 Go 依赖缓存；
- `actions/checkout@v4` 和 `actions/setup-go@v5` 产生 Node.js 20 废弃警告。

## 根因

`make verify` 同时执行 Go、文档和 React 校验，但原工作流只安装 Go：

```text
checkout
setup-go
make verify
```

GitHub 的干净 runner 没有项目需要的 pnpm，也没有执行 `web/pnpm-lock.yaml` 对应的依赖安装。开发机已经存在 pnpm 和 `web/node_modules`，所以相同命令在本地通过，掩盖了 CI 环境缺口。

`go.sum` 警告不是本次退出码的直接原因，但当前 Go 模块没有外部依赖，不需要启用依赖缓存。

## 修复

质量工作流现在显式完成：

1. 使用 Node.js 24 运行时版本的 `actions/checkout@v6` 和 `actions/setup-go@v6`；
2. Go 尚无外部依赖时关闭 setup-go 缓存；
3. 安装固定版本 pnpm `10.13.1`；
4. 安装 Node.js 22，并按 `web/pnpm-lock.yaml` 缓存 pnpm 依赖；
5. 执行 `make web-install`，使用 `--frozen-lockfile` 安装前端依赖；
6. 最后执行统一的 `make verify`。

## 干净环境验证

验证时创建不包含 `.git`、`web/node_modules` 和 `web/dist` 的全新临时工作副本，然后执行：

```text
pnpm --dir web install --frozen-lockfile
make verify
```

实际结果：

- 前端从锁文件安装 87 个包；
- `go test ./...` 通过；
- 文档漂移检查通过；
- TypeScript 类型检查通过；
- Vite 生产构建通过；
- 保留既有的 JavaScript chunk 超过 500 kB 警告，不影响退出码。

## 回归要求

- CI 不得依赖开发机已有的全局工具或 `node_modules`；
- `web/package.json` 或锁文件变更后，冻结安装必须仍然通过；
- Go 引入首个外部依赖并生成 `go.sum` 后，可重新评估启用 setup-go 缓存；
- 修复只有在 GitHub 上新一次 `quality / verify` 成功后才能最终关闭。
