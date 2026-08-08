# 第 14 节：React TypeScript 前端工程初始化

## 本节诉求

后续用户端、运营后台、MCP Gateway 和 AI Operator 都需要统一的前端基础。本节引入一套已经完成的 GrowthOS UI 设计源码，先搭建完整前端工程框架，避免后续每个业务章节重复创建路由、布局、主题和公共组件。

## 本节交付

- 使用 React 19、TypeScript、Vite 和 Tailwind CSS；
- 复刻既有 GrowthOS UI 的字体、色彩、间距、卡片、图表和深浅色主题；
- 建立用户端、运营端、MCP Gateway、AI Operator 和系统页路由；
- 建立五类布局、公共 UI 组件、业务卡片、SVG 图形和 Mock 数据边界；
- 建立 `web/` 独立安装、开发、类型检查和生产构建流程；
- 清理 Figma Make 专属配置，保证项目可脱离设计工具运行；
- 更新 ADR、QA、仓库地图和前端架构文档。

## 当前边界

页面和交互框架已经搭好，但 Go 业务 API、MySQL、Redis 和真实权限尚未实现。当前数据来自 `web/src/mocks/growthOsMockData.ts`，第 15 节开始进行首次真实 API 联调。

## 验证

```bash
cd web
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run build
```

QA 证据见[第 14 节 QA 验收](../../qa/lessons/lesson-14.md)。

本节没有新增真实后端调用，API 边界见[第 14 节 API 记录](../../api/lessons/lesson-14.md)。

## 下一节遗留问题

当前页面已经具备真实 API 的接入边界，但后端还没有健康接口和统一响应协议。下一节将让 Go 服务、API 代理和前端状态页面完成第一次联调。
