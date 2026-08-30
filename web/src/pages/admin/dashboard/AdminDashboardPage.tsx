import { Bot, Coins, Gift, Users } from "lucide-react";
import { CompactMetric, PageHeader, Surface } from "../../../components/common/ProductPage";
import {
  GrowthFunnelIllustration,
  McpArchitectureDiagram,
} from "../../../components/common/GrowthOSGraphics";
import { MOCK_SNAPSHOT_LABEL } from "../../../mocks/growthOsMockData";

export function AdminDashboardPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Growth Cockpit · 2026 Q1"
        title="增长运营大盘"
        description={`截至 ${MOCK_SNAPSHOT_LABEL} 的 Go 微服务、活动、积分与 MCP Agent 前端演示快照；不代表实时生产流。`}
        badge="演示快照"
      />

      <section
        aria-label="运营指标摘要"
        className="grid gap-px overflow-hidden rounded-xl border border-zinc-200 bg-zinc-200 sm:grid-cols-2 lg:grid-cols-4 dark:border-zinc-800 dark:bg-zinc-800"
      >
        <CompactMetric
          className="bg-white dark:bg-zinc-950"
          label="平台注册用户"
          value="128,450"
          helper="演示变化 +14.2%"
          icon={Users}
        />
        <CompactMetric
          className="bg-white dark:bg-zinc-950"
          label="营销活动"
          value="38"
          helper="其中 12 个标记为运行中"
          icon={Gift}
        />
        <CompactMetric
          className="bg-white dark:bg-zinc-950"
          label="积分池流通量"
          value="14.28M"
          helper="演示变化 +8.1%"
          icon={Coins}
        />
        <CompactMetric
          className="bg-white dark:bg-zinc-950"
          label="AI 工具调用"
          value="45,920"
          helper="24 小时演示统计"
          icon={Bot}
        />
      </section>

      <section className="grid gap-2 rounded-xl bg-zinc-100 p-2 lg:grid-cols-2 dark:bg-zinc-900">
        <Surface className="border-0 p-5">
          <div className="mb-4 flex items-center justify-between gap-4">
            <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
              全链路增长转化漏斗
            </h2>
            <span className="text-xs font-medium text-emerald-600">演示转化率 12.4%</span>
          </div>
          <GrowthFunnelIllustration />
        </Surface>

        <Surface className="border-0 p-5">
          <div className="mb-4 flex items-center justify-between gap-4">
            <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
              MCP Gateway 架构快照
            </h2>
            <span className="text-xs text-zinc-400">静态示意</span>
          </div>
          <McpArchitectureDiagram />
        </Surface>
      </section>
    </div>
  );
}
