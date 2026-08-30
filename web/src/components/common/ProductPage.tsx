import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "../../lib/cn";

interface PageHeaderProps {
  title: string;
  description?: string;
  eyebrow?: string;
  badge?: string;
  actions?: ReactNode;
  className?: string;
  titleId?: string;
}

export function PageHeader({
  title,
  description,
  eyebrow,
  badge,
  actions,
  className,
  titleId,
}: PageHeaderProps) {
  return (
    <header className={cn("border-b border-zinc-200 pb-3 dark:border-zinc-800", className)}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          {eyebrow ? (
            <p className="mb-1 text-[11px] font-medium uppercase tracking-[0.14em] text-zinc-400 dark:text-zinc-600">
              {eyebrow}
            </p>
          ) : null}
          <div className="flex flex-wrap items-center gap-2">
            <h1
              id={titleId}
              className="text-2xl font-semibold leading-8 tracking-[-0.02em] text-zinc-950 dark:text-zinc-50"
            >
              {title}
            </h1>
            {badge ? <DemoBadge>{badge}</DemoBadge> : null}
          </div>
          {description ? (
            <p className="mt-1 max-w-3xl text-sm leading-6 text-zinc-500 dark:text-zinc-400">
              {description}
            </p>
          ) : null}
        </div>
        {actions ? <div className="shrink-0">{actions}</div> : null}
      </div>
    </header>
  );
}

interface SectionHeaderProps {
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
  titleId?: string;
}

export function SectionHeader({
  title,
  description,
  action,
  className,
  titleId,
}: SectionHeaderProps) {
  return (
    <div
      className={cn(
        "flex min-h-9 flex-col gap-1 border-b border-zinc-200 pb-2 sm:flex-row sm:items-center sm:justify-between dark:border-zinc-800",
        className,
      )}
    >
      <div>
        <h2
          id={titleId}
          className="text-xl font-semibold leading-7 tracking-[-0.015em] text-zinc-950 dark:text-zinc-50"
        >
          {title}
        </h2>
        {description ? (
          <p className="mt-0.5 text-xs leading-5 text-zinc-500 dark:text-zinc-400">{description}</p>
        ) : null}
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}

export function DemoBadge({ children = "演示数据" }: { children?: ReactNode }) {
  return (
    <span className="inline-flex h-6 items-center rounded-full border border-violet-200 bg-violet-50 px-2 text-[11px] font-medium text-violet-600 dark:border-violet-500/25 dark:bg-violet-500/10 dark:text-violet-300">
      {children}
    </span>
  );
}

interface SurfaceProps {
  children: ReactNode;
  className?: string;
  as?: "div" | "section" | "article";
}

export function Surface({ children, className, as: Component = "div" }: SurfaceProps) {
  return (
    <Component
      className={cn(
        "rounded-xl border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-950",
        className,
      )}
    >
      {children}
    </Component>
  );
}

interface CompactMetricProps {
  label: string;
  value: ReactNode;
  helper?: ReactNode;
  icon?: LucideIcon;
  className?: string;
}

export function CompactMetric({ label, value, helper, icon: Icon, className }: CompactMetricProps) {
  return (
    <div className={cn("min-w-0 px-4 py-4", className)}>
      <div className="flex items-start justify-between gap-3">
        <span className="text-xs font-medium text-zinc-500 dark:text-zinc-400">{label}</span>
        {Icon ? <Icon className="h-4 w-4 shrink-0 text-violet-500" aria-hidden="true" /> : null}
      </div>
      <div className="mt-2 text-2xl font-semibold leading-8 tracking-[-0.02em] text-zinc-950 tabular-nums dark:text-zinc-50">
        {value}
      </div>
      {helper ? (
        <div className="mt-1 text-[11px] text-zinc-400 dark:text-zinc-500">{helper}</div>
      ) : null}
    </div>
  );
}

interface ProgressBarProps {
  label: string;
  value: number;
  tone?: "primary" | "success" | "danger";
  className?: string;
}

export function ProgressBar({ label, value, tone = "primary", className }: ProgressBarProps) {
  const clampedValue = Math.min(100, Math.max(0, value));
  const toneClass = {
    primary: "bg-violet-500",
    success: "bg-emerald-500",
    danger: "bg-rose-500",
  }[tone];

  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={clampedValue}
      className={cn("h-2 overflow-hidden rounded-full bg-zinc-100 dark:bg-zinc-800", className)}
    >
      <div className={cn("h-full rounded-full", toneClass)} style={{ width: `${clampedValue}%` }} />
    </div>
  );
}
