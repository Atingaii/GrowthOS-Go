import React from 'react';
import { Gift, Sparkles, Filter, Plus } from 'lucide-react';
import { mockCampaigns } from '../../../mocks/growthOsMockData';

export const CampaignsListPage: React.FC = () => {
  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-extrabold text-slate-900 dark:text-white flex items-center gap-2">
            <Gift className="w-6 h-6 text-purple-500" /> 增长营销活动大厅
          </h1>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            浏览与参与裂变拉新、抽奖策略、AI 试用与返利活动。
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button className="px-3 py-1.5 rounded-xl border border-slate-200 dark:border-slate-800 text-xs font-semibold flex items-center gap-1">
            <Filter className="w-3.5 h-3.5" /> 筛选分类
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
        {mockCampaigns.map((cmp) => (
          <div
            key={cmp.id}
            className="bg-white dark:bg-slate-900 rounded-3xl border border-slate-200 dark:border-slate-800 shadow-sm overflow-hidden flex flex-col justify-between hover:border-blue-500/50 transition-all"
          >
            <div className={`p-6 bg-gradient-to-br ${cmp.bannerBg} text-white`}>
              <div className="flex items-center justify-between mb-2">
                <span className="text-[10px] font-mono font-bold px-2 py-0.5 rounded bg-black/30 backdrop-blur-md">
                  {cmp.category}
                </span>
                <span className="text-xs font-extrabold text-amber-300">{cmp.rewardAmount}</span>
              </div>
              <h3 className="text-lg font-extrabold mb-1">{cmp.title}</h3>
              <p className="text-xs text-slate-200 line-clamp-2">{cmp.subtitle}</p>
            </div>

            <div className="p-5 space-y-4">
              <div className="space-y-1">
                <div className="flex justify-between text-xs font-medium text-slate-500">
                  <span>活动预算进度</span>
                  <span className="font-mono">${cmp.budgetSpent} / ${cmp.totalBudget}</span>
                </div>
                <div className="w-full h-2 rounded-full bg-slate-100 dark:bg-slate-800 overflow-hidden">
                  <div
                    className="h-full bg-blue-500 rounded-full"
                    style={{ width: `${(cmp.budgetSpent / cmp.totalBudget) * 100}%` }}
                  />
                </div>
              </div>

              <div className="flex items-center justify-between text-xs border-t border-slate-100 dark:border-slate-800 pt-3">
                <span className="text-slate-400 font-mono">转换率: {cmp.conversionRate}%</span>
                <a
                  href={`/campaigns/${cmp.id}`}
                  className="px-4 py-1.5 rounded-xl bg-blue-600 text-white font-bold hover:bg-blue-500 transition-colors"
                >
                  参与活动
                </a>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
