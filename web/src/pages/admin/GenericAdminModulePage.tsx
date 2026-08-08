import React from 'react';
import { Target, Trophy, Users, Coins, Ticket, Percent, Rss, Activity, FlaskConical, BarChart3, CheckSquare, ShieldAlert } from 'lucide-react';

export const GenericAdminModulePage: React.FC<{ title: string; subtitle: string; icon: any }> = ({
  title,
  subtitle,
  icon: Icon,
}) => {
  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-extrabold text-slate-900 dark:text-white flex items-center gap-2">
            <Icon className="w-6 h-6 text-blue-600" /> {title}
          </h1>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">{subtitle}</p>
        </div>
        <button className="px-4 py-2 rounded-xl bg-blue-600 text-white font-bold text-xs shadow-md hover:bg-blue-500">
          + 配置 {title}
        </button>
      </div>

      <div className="bg-white dark:bg-[#141414] border border-stone-200 dark:border-neutral-800 p-8">
        <div className="py-12 text-center space-y-3">
          <div className="w-10 h-10 bg-stone-100 dark:bg-neutral-800 text-stone-400 flex items-center justify-center mx-auto">
            <Icon className="w-5 h-5" />
          </div>
          <h3 className="text-sm font-semibold text-stone-700 dark:text-stone-300">{title}</h3>
          <p className="text-xs text-stone-400 max-w-md mx-auto">
            此模块正在建设中，数据与交互功能即将上线。
          </p>
        </div>
      </div>
    </div>
  );
};
