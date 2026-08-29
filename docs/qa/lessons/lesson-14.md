# 第 14 节 QA 验收证据

- **日期：** 2026-08-08
- **产物：** [React 前端工程初始化](../../course/part-02/lesson-14-react-frontend-framework.md)
- **结果：** 通过

## 验收范围

| 检查项 | 结果 | 证据 |
| --- | --- | --- |
| 用户端、运营端、MCP、Agent 和系统页路由已注册 | 通过 | `web/src/routes/appRouter.tsx` |
| 四类布局和设计包视觉基线已导入 | 通过 | `web/src/layouts`、`web/src/index.css` |
| 页面使用集中 Mock 数据而非散落虚假数据 | 通过 | `web/src/mocks/growthOsMockData.ts` |
| Figma Make 专属配置不再参与构建 | 通过 | `web/vite.config.ts` 不含设计工具插件 |
| TypeScript 类型检查通过 | 通过 | `cd web && pnpm run typecheck` |
| 生产构建通过 | 通过 | `cd web && pnpm run build` |
| 未把业务 API、业务表、真实权限或前端联调宣称为已完成 | 通过 | 第 14 节正文与前端架构文档 |
| 前端 API 变化已按章节登记 | 通过 | `docs/api/lessons/lesson-14.md` |

## 执行命令

```bash
cd web
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run build
```

## 未覆盖项

- 第 14 节原始交付时没有执行真实后端联调；当前累计后端已在第 11～13 节提供 `/health`、`/ready` 和统一错误，但 React 消费这些契约仍属于第 15 节；
- 浏览器端逐路由视觉回归和移动端截图验收留到接入开发服务器后执行；
- Mock 页面中的业务操作不作为真实营销能力验收依据。
