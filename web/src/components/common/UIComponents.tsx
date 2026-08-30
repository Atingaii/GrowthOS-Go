import React from "react";
import { LucideIcon } from "lucide-react";

interface MetricCardProps {
  title: string;
  value: string | number;
  change?: string;
  isPositive?: boolean;
  subtitle?: string;
  icon?: LucideIcon;
  badgeText?: string;
  color?: "blue" | "purple" | "emerald" | "amber" | "cyan";
}

export const MetricCard: React.FC<MetricCardProps> = ({
  title,
  value,
  change,
  isPositive = true,
  subtitle,
  icon: Icon,
  badgeText,
  color = "blue",
}) => {
  const iconColor = {
    blue: "text-blue-600 dark:text-blue-400",
    purple: "text-violet-600 dark:text-violet-400",
    emerald: "text-emerald-600 dark:text-emerald-400",
    amber: "text-amber-600 dark:text-amber-400",
    cyan: "text-cyan-600 dark:text-cyan-400",
  };

  return (
    <div className="p-5 bg-white dark:bg-[#141414] border border-stone-200 dark:border-neutral-800 hover:border-stone-300 dark:hover:border-neutral-700 transition-colors duration-150">
      <div className="flex items-start justify-between mb-4">
        <span className="text-xs font-medium text-stone-400 dark:text-stone-500 leading-tight">
          {title}
        </span>
        {Icon && <Icon className={`w-4 h-4 ${iconColor[color]} shrink-0`} />}
      </div>

      <div className="flex items-baseline gap-2">
        <div className="text-2xl font-bold tracking-tight text-stone-900 dark:text-stone-50 tabular-nums">
          {value}
        </div>
        {change && (
          <span
            className={`text-xs font-medium ${isPositive ? "text-emerald-600 dark:text-emerald-400" : "text-rose-600 dark:text-rose-400"}`}
          >
            {isPositive ? "+" : ""}
            {change}
          </span>
        )}
      </div>

      {(subtitle || badgeText) && (
        <div className="mt-3 flex items-center justify-between text-xs text-stone-400 dark:text-stone-500">
          {subtitle && <span>{subtitle}</span>}
          {badgeText && (
            <span className="font-mono text-[10px] bg-stone-100 dark:bg-neutral-800 text-stone-500 dark:text-stone-400 px-1.5 py-0.5">
              {badgeText}
            </span>
          )}
        </div>
      )}
    </div>
  );
};

export const StatusBadge: React.FC<{ status: string; label?: string }> = ({ status, label }) => {
  const map: Record<string, { bg: string; text: string; dot: string; defaultLabel: string }> = {
    active: {
      bg: "bg-emerald-50 dark:bg-emerald-950/50",
      text: "text-emerald-700 dark:text-emerald-400",
      dot: "bg-emerald-500",
      defaultLabel: "Active",
    },
    online: {
      bg: "bg-emerald-50 dark:bg-emerald-950/50",
      text: "text-emerald-700 dark:text-emerald-400",
      dot: "bg-emerald-500",
      defaultLabel: "Online",
    },
    running: {
      bg: "bg-blue-50 dark:bg-blue-950/50",
      text: "text-blue-700 dark:text-blue-400",
      dot: "bg-blue-500",
      defaultLabel: "Running",
    },
    pending: {
      bg: "bg-amber-50 dark:bg-amber-950/50",
      text: "text-amber-700 dark:text-amber-400",
      dot: "bg-amber-500",
      defaultLabel: "Pending",
    },
    waiting_approval: {
      bg: "bg-purple-50 dark:bg-purple-950/50",
      text: "text-purple-700 dark:text-purple-400",
      dot: "bg-purple-500",
      defaultLabel: "Needs Approval",
    },
    local_only: {
      bg: "bg-violet-50 dark:bg-violet-950/50",
      text: "text-violet-700 dark:text-violet-300",
      dot: "bg-violet-500",
      defaultLabel: "Local only",
    },
    completed: {
      bg: "bg-emerald-50 dark:bg-emerald-950/50",
      text: "text-emerald-700 dark:text-emerald-400",
      dot: "bg-emerald-500",
      defaultLabel: "Completed",
    },
    offline: {
      bg: "bg-zinc-100 dark:bg-zinc-800",
      text: "text-zinc-600 dark:text-zinc-400",
      dot: "bg-zinc-400",
      defaultLabel: "Offline",
    },
    draft: {
      bg: "bg-slate-100 dark:bg-slate-800",
      text: "text-slate-600 dark:text-slate-400",
      dot: "bg-slate-400",
      defaultLabel: "Draft",
    },
    degraded: {
      bg: "bg-orange-50 dark:bg-orange-950/50",
      text: "text-orange-700 dark:text-orange-400",
      dot: "bg-orange-500",
      defaultLabel: "Degraded",
    },
    failed: {
      bg: "bg-rose-50 dark:bg-rose-950/50",
      text: "text-rose-700 dark:text-rose-400",
      dot: "bg-rose-500",
      defaultLabel: "Failed",
    },
  };

  const current = map[status] || {
    bg: "bg-slate-100 dark:bg-slate-800",
    text: "text-slate-600 dark:text-slate-400",
    dot: "bg-slate-400",
    defaultLabel: status,
  };

  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold ${current.bg} ${current.text}`}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${current.dot}`} />
      {label || current.defaultLabel}
    </span>
  );
};
