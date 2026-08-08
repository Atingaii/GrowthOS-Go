# Lesson 01 QA Evidence

- **Date:** 2026-08-08
- **Artifact:** [为什么要做 AI 原生大营销增长平台](../../course/part-01/lesson-01-why-ai-native-growth-platform.md)
- **Result:** Pass

## Acceptance Review

| 检查项 | 结果 | 证据 |
| --- | --- | --- |
| 覆盖电商、出行、金融、内容、游戏五类场景 | Pass | 正文“行业场景” |
| 解释共同业务矛盾而非堆砌功能 | Pass | 正文“共同矛盾” |
| 区分 Business、Growth、AI Native 三层 | Pass | 正文“产品能力分层” |
| 明确 AI 可做与不可绕过的边界 | Pass | 正文“为什么是 AI Native” |
| 明确当前非目标和延迟决策 | Pass | 正文“范围边界” |
| 为第 2 节产生输入问题 | Pass | 正文“遗留问题” |
| 未提前创建业务表或宣称微服务已实现 | Pass | 仓库地图与当前文件树评审 |

## Automated Evidence

从仓库根目录执行：

```text
make verify
```

该命令校验 Go 格式、Go 测试、课程台账、ADR 索引和 Markdown 本地链接。最终提交前重新执行，并以 CI 结果作为远端持续证据。

## Remaining Risk

- 第 1 节是产品问题定义，不验证市场规模或商业收入预测。
- 具体用户旅程、运营工作流和定量 SLO 尚未定义，分别留给第 2、3、7 节。
- 外部参考可能更新，本文只将其作为课程结构背景，不把外部项目内容当作本项目已实现事实。
