import React from 'react';
import { Gift, Plus, Search, Filter } from 'lucide-react';
import { mockCampaigns } from '../../../mocks/growthOsMockData';
import { StatusBadge } from '../../../components/common/UIComponents';

export const AdminCampaignsPage: React.FC = () => {
  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-extrabold text-slate-900 dark:text-white flex items-center gap-2">
            <Gift className="w-6 h-6 text-blue-600" /> 营销活动与裂变策略管理
          </h1>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            配置与发布裂变活动、预算额度、奖励策略，支持规则引擎一键上线。
          </p>
        </div>

        <button className="px-4 py-2 rounded-xl bg-blue-600 text-white font-bold text-xs flex items-center gap-1.5 shadow-md">
          <Plus className="w-4 h-4" /> 创建新活动
        </button>
      </div>

      <div className="bg-white dark:bg-slate-900 rounded-3xl border border-slate-200 dark:border-slate-800 p-6 space-y-4">
        <div className="flex items-center gap-3">
          <div className="relative flex-1">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
            <input
              type="text"
              placeholder="按活动名称、ID 或类型搜索..."
              className="w-full pl-9 pr-4 py-2 rounded-xl bg-slate-100 dark:bg-slate-800 border-none text-xs focus:ring-2 focus:ring-blue-500 text-slate-900 dark:text-slate-100 placeholder-slate-400"
            />
          </div>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-400 font-mono">
                <th className="pb-3 font-semibold">活动 ID / 名称</th>
                <th className="pb-3 font-semibold">类别</th>
                <th className="pb-3 font-semibold">状态</th>
                <th className="pb-3 font-semibold">参与人数</th>
                <th className="pb-3 font-semibold">预算消耗</th>
                <th className="pb-3 font-semibold">转化率</th>
                <th className="pb-3 font-semibold text-right">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {mockCampaigns.map((cmp) => (
                <tr key={cmp.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/50">
                  <td className="py-3.5">
                    <div className="font-bold text-slate-900 dark:text-white">{cmp.title}</div>
                    <div className="text-[10px] text-slate-400 font-mono">{cmp.id}</div>
                  </td>
                  <td className="py-3.5 font-semibold text-slate-600 dark:text-slate-300">{cmp.category}</td>
                  <td className="py-3.5"><StatusBadge status={cmp.status} /></td>
                  <td className="py-3.5 font-mono font-bold text-slate-900 dark:text-white">{cmp.participants.toLocaleString()}</td>
                  <td className="py-3.5 font-mono text-slate-600 dark:text-slate-300">${cmp.budgetSpent} / ${cmp.totalBudget}</td>
                  <td className="py-3.5 font-mono font-bold text-emerald-500">{cmp.conversionRate}%</td>
                  <td className="py-3.5 text-right">
                    <button className="text-blue-600 dark:text-blue-400 font-bold hover:underline">编辑</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
