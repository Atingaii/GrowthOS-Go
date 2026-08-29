# GrowthOS-Go API 文档

这里集中记录每一节课程新增或调整的 API 契约，方便从产品页面、Go 后端实现和前端调用之间建立直观映射。

## 目录

| 文档 | 作用 |
| --- | --- |
| [前端 API 约定](frontend-api-conventions.md) | 前端请求层、Mock/真实接口切换和记录格式 |
| [章节 API 记录](lessons/README.md) | 按课程章节查看 API 变更 |
| [章节 API 模板](lesson-template.md) | 新增 API 章节记录时复制的模板 |

## 当前 API 状态

| 章节 | 前端 API 变化 | 状态 | 记录 |
| --- | --- | --- | --- |
| 第 1 节 | 产品定位，无真实前端调用 | 已完成 | [第 1 节 API 记录](lessons/lesson-01.md) |
| 第 2 节 | 用户增长旅程，无真实前端调用 | 已完成 | [第 2 节 API 记录](lessons/lesson-02.md) |
| 第 3 节 | 运营人员工作流，无真实前端调用 | 已完成 | [第 3 节 API 记录](lessons/lesson-03.md) |
| 第 4 节 | AI Operator 工作流，无真实前端调用 | 已完成 | [第 4 节 API 记录](lessons/lesson-04.md) |
| 第 5 节 | 事件风暴分析，无真实前端调用 | 已完成 | [第 5 节 API 记录](lessons/lesson-05.md) |
| 第 6 节 | 限界上下文分析，无真实前端调用 | 已完成 | [第 6 节 API 记录](lessons/lesson-06.md) |
| 第 7 节 | 非功能需求分析，无真实前端调用 | 已完成 | [第 7 节 API 记录](lessons/lesson-07.md) |
| 第 8 节 | V0 系统设计，无真实前端调用 | 已完成 | [第 8 节 API 记录](lessons/lesson-08.md) |
| 第 9 节 | 仓库工程基线，无真实前端调用 | 已完成 | [第 9 节 API 记录](lessons/lesson-09.md) |
| 第 10 节 | 模块化单体决策，无真实前端调用 | 已完成 | [第 10 节 API 记录](lessons/lesson-10.md) |
| 第 11 节 | 新增进程健康接口 `GET /health`，前端尚未调用 | 已完成 | [第 11 节 API 记录](lessons/lesson-11.md) |
| 第 12 节 | 统一请求关联与 404/405/500 错误 envelope，健康成功 body 不变 | 已完成 | [第 12 节 API 记录](lessons/lesson-12.md) |
| 第 13 节 | 新增 MySQL readiness `GET /ready`，保持 `/health` liveness | 已完成 | [第 13 节 API 记录](lessons/lesson-13.md) |
| 第 14 节 | 建立 Mock 数据边界，未新增真实后端调用 | 已完成 | [第 14 节 API 记录](lessons/lesson-14.md) |
| 第 15 节 | 系统状态页真实调用 `GET /health` 与 `GET /ready`，建立同源 Client、运行时解码和失败分类 | 已验收 | [第 15 节 API 记录](lessons/lesson-15.md) |
| 第 16 节 | 路径与 JSON body 保持不变；Compose Nginx 成为唯一同源入口，并统一回写可关联的 `X-Request-ID` | 已验收 | [第 16 节 API 记录](lessons/lesson-16.md) |
| 第 17 节 | 新增持久化无关的 Lottery Strategy/Award 领域对象；无 HTTP 路由、DTO 或真实前端调用 | 已验收 | [第 17 节 API 记录](lessons/lesson-17.md) |
| 第 18 节 | 新增 Lottery 持久化表和最小只读数据库权限；无 HTTP 路由、DTO、业务 SQL 或真实前端调用 | 已验收 | [第 18 节 API 记录](lessons/lesson-18.md) |

## 阅读方式

每个章节记录至少说明：

- 新增、修改或删除了哪些接口；
- 哪个页面或组件调用接口；
- 请求参数、响应结构和错误码；
- 当前是 Mock、已联调还是已验收；
- 如何验证以及遗留问题。

如果某节没有前端 API 变化，也保留一条“无新增真实 API”的记录，避免读者误以为遗漏。
