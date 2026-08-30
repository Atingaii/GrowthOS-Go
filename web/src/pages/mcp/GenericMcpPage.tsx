import type { LucideIcon } from "lucide-react";
import { PageHeader, Surface } from "../../components/common/ProductPage";

interface GenericMcpPageProps {
  title: string;
  subtitle: string;
  icon: LucideIcon;
}

export function GenericMcpPage({ title, subtitle, icon: Icon }: GenericMcpPageProps) {
  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="MCP Gateway Subsystem"
        title={title}
        description={`${subtitle}。当前仅展示模块边界，尚未接入真实 JSON-RPC / SSE 观测与写操作。`}
        badge="建设中"
      />

      <Surface as="section" className="overflow-hidden" aria-label={`${title} 能力边界`}>
        <div className="flex items-start gap-3 p-4">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-violet-50 text-violet-500 dark:bg-violet-500/10 dark:text-violet-300">
            <Icon className="h-5 w-5" aria-hidden="true" />
          </div>
          <div>
            <h2 className="text-sm font-semibold text-zinc-800 dark:text-zinc-200">
              {title} 能力边界
            </h2>
            <p className="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-400">
              只展示子系统职责，不生成虚构的网关状态或调用结果。
            </p>
          </div>
        </div>
        <dl className="grid gap-px border-t border-zinc-200 bg-zinc-200 sm:grid-cols-3 dark:border-zinc-800 dark:bg-zinc-800">
          <div className="bg-white p-4 dark:bg-zinc-950">
            <dt className="text-[11px] text-zinc-400">JSON-RPC / SSE</dt>
            <dd className="mt-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">未接入</dd>
          </div>
          <div className="bg-white p-4 dark:bg-zinc-950">
            <dt className="text-[11px] text-zinc-400">权限与审计</dt>
            <dd className="mt-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">未接入</dd>
          </div>
          <div className="bg-white p-4 dark:bg-zinc-950">
            <dt className="text-[11px] text-zinc-400">当前输出</dt>
            <dd className="mt-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">
              只读边界说明
            </dd>
          </div>
        </dl>
      </Surface>
    </div>
  );
}
