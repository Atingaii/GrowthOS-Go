import type { LucideIcon } from "lucide-react";
import { PageHeader, Surface } from "../../components/common/ProductPage";

interface GenericAgentPageProps {
  title: string;
  subtitle: string;
  icon: LucideIcon;
}

export function GenericAgentPage({ title, subtitle, icon: Icon }: GenericAgentPageProps) {
  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="AI Operator Engine"
        title={title}
        description={`${subtitle}。当前仅展示模块信息架构，尚未连接 Agent 调度或人工审批后端。`}
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
              只展示 Human-in-the-loop 信息架构，不伪造执行中任务或审批结果。
            </p>
          </div>
        </div>
        <dl className="grid gap-px border-t border-zinc-200 bg-zinc-200 sm:grid-cols-3 dark:border-zinc-800 dark:bg-zinc-800">
          <div className="bg-white p-4 dark:bg-zinc-950">
            <dt className="text-[11px] text-zinc-400">Agent 调度</dt>
            <dd className="mt-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">未接入</dd>
          </div>
          <div className="bg-white p-4 dark:bg-zinc-950">
            <dt className="text-[11px] text-zinc-400">审批写入</dt>
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
