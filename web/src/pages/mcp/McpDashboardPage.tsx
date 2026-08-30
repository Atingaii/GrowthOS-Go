import { Activity, Server, Wrench } from "lucide-react";
import { CompactMetric, PageHeader, Surface } from "../../components/common/ProductPage";
import { StatusBadge } from "../../components/common/UIComponents";
import { MOCK_SNAPSHOT_LABEL, mockMcpServers, mockMcpTools } from "../../mocks/growthOsMockData";

export function McpDashboardPage() {
  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="MCP Gateway · JSON-RPC"
        title="AI 工具调度控制台"
        description={`查看截至 ${MOCK_SNAPSHOT_LABEL} 的本地演示节点、工具风险等级与调用质量快照；本页面尚未连接实时网关观测流。`}
        badge="演示快照"
      />

      <section
        aria-label="MCP 指标摘要"
        className="grid gap-px overflow-hidden rounded-xl border border-zinc-200 bg-zinc-200 sm:grid-cols-3 dark:border-zinc-800 dark:bg-zinc-800"
      >
        <CompactMetric
          className="bg-white dark:bg-zinc-950"
          label="MCP Server"
          value="3"
          helper="演示节点"
          icon={Server}
        />
        <CompactMetric
          className="bg-white dark:bg-zinc-950"
          label="已登记工具"
          value="28"
          helper="本地工具目录"
          icon={Wrench}
        />
        <CompactMetric
          className="bg-white dark:bg-zinc-950"
          label="24h 调用量"
          value="124,890"
          helper="演示均值 32ms"
          icon={Activity}
        />
      </section>

      <section className="grid gap-2 rounded-xl bg-zinc-100 p-2 lg:grid-cols-2 dark:bg-zinc-900">
        <Surface className="border-0 p-4">
          <h2 className="mb-2 text-sm font-semibold text-zinc-900 dark:text-zinc-100">
            MCP Server 节点
          </h2>
          <div className="divide-y divide-zinc-100 text-xs dark:divide-zinc-900">
            {mockMcpServers.map((server) => (
              <div key={server.id} className="flex items-center justify-between gap-4 py-3">
                <div className="min-w-0">
                  <div className="truncate font-medium text-zinc-900 dark:text-zinc-100">
                    {server.name}
                  </div>
                  <div className="mt-0.5 truncate font-mono text-[10px] text-zinc-400">
                    {server.endpoint} · {server.version}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <span className="font-mono text-zinc-400">{server.avgLatencyMs}ms</span>
                  <StatusBadge status={server.status} />
                </div>
              </div>
            ))}
          </div>
        </Surface>

        <Surface className="border-0 p-4">
          <h2 className="mb-2 text-sm font-semibold text-zinc-900 dark:text-zinc-100">
            Tool 风险与成功率
          </h2>
          <div className="divide-y divide-zinc-100 text-xs dark:divide-zinc-900">
            {mockMcpTools.map((tool) => (
              <div key={tool.id} className="flex items-center justify-between gap-4 py-3">
                <div className="min-w-0">
                  <div className="truncate font-mono font-medium text-zinc-900 dark:text-zinc-100">
                    {tool.name}
                  </div>
                  <div className="mt-0.5 line-clamp-1 text-[10px] text-zinc-400">
                    {tool.description}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <span
                    className={`rounded-full px-2 py-0.5 text-[10px] font-medium uppercase ${
                      tool.riskLevel === "critical"
                        ? "bg-rose-50 text-rose-600 dark:bg-rose-500/10 dark:text-rose-300"
                        : "bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-300"
                    }`}
                  >
                    {tool.riskLevel}
                  </span>
                  <span className="font-mono font-medium text-emerald-600">
                    {tool.successRate}%
                  </span>
                </div>
              </div>
            ))}
          </div>
        </Surface>
      </section>
    </div>
  );
}
