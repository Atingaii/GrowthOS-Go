import type { LucideIcon } from "lucide-react";
import { PageHeader, Surface } from "../../components/common/ProductPage";

interface GenericAdminModulePageProps {
  title: string;
  subtitle: string;
  icon: LucideIcon;
}

export function GenericAdminModulePage({
  title,
  subtitle,
  icon: Icon,
}: GenericAdminModulePageProps) {
  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="Operator Module"
        title={title}
        description={`${subtitle}。当前页面用于展示信息架构，真实数据与写操作尚未接入。`}
        badge="建设中"
        actions={
          <button
            type="button"
            disabled
            className="inline-flex h-10 cursor-not-allowed items-center rounded-lg bg-zinc-100 px-4 text-sm font-medium text-zinc-400 dark:bg-zinc-900 dark:text-zinc-600"
          >
            配置 {title} · 待接入
          </button>
        }
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
              页面保留真实信息架构，但不会伪造数据加载、保存成功或后台任务。
            </p>
          </div>
        </div>
        <dl className="grid gap-px border-t border-zinc-200 bg-zinc-200 sm:grid-cols-3 dark:border-zinc-800 dark:bg-zinc-800">
          <div className="bg-white p-4 dark:bg-zinc-950">
            <dt className="text-[11px] text-zinc-400">数据源</dt>
            <dd className="mt-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">未接入</dd>
          </div>
          <div className="bg-white p-4 dark:bg-zinc-950">
            <dt className="text-[11px] text-zinc-400">当前交互</dt>
            <dd className="mt-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">
              只读模块边界
            </dd>
          </div>
          <div className="bg-white p-4 dark:bg-zinc-950">
            <dt className="text-[11px] text-zinc-400">实现前置</dt>
            <dd className="mt-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">
              API、权限与审计契约
            </dd>
          </div>
        </dl>
      </Surface>
    </div>
  );
}
