import React from 'react';
import { Coins, ArrowUpRight, ArrowDownLeft, ShieldCheck } from 'lucide-react';
import { MetricCard } from '../../../components/common/UIComponents';
import { mockPointTransactions } from '../../../mocks/growthOsMockData';
import { useAppStore } from '../../../stores/appStore';

export const PointsPage: React.FC = () => {
  const { user } = useAppStore();

  return (
    <div className="space-y-6 max-w-5xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-extrabold text-slate-900 dark:text-white flex items-center gap-2">
            <Coins className="w-6 h-6 text-amber-500" /> 积分资产中心
          </h1>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            实时对接 GrowthOS-Go 积分账户微服务，记录每一笔积分变动明细。
          </p>
        </div>
        <button className="px-4 py-2 rounded-xl bg-blue-600 text-white font-bold text-xs hover:bg-blue-500 shadow-md">
          + 赚取更多积分
        </button>
      </div>

      {/* Metrics */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <MetricCard title="当前总积分" value={`${user.points} PTS`} icon={Coins} color="amber" badgeText="可用余额" />
        <MetricCard title="本月累计获得" value="+1,750 PTS" change="+24%" icon={ArrowUpRight} color="emerald" />
        <MetricCard title="本月兑换消耗" value="-2,000 PTS" icon={ArrowDownLeft} color="purple" />
      </div>

      {/* Point Transactions List */}
      <div className="bg-white dark:bg-slate-900 rounded-2xl border border-slate-200 dark:border-slate-800 p-6 space-y-4">
        <h3 className="font-bold text-base text-slate-900 dark:text-white flex items-center gap-2">
          <ShieldCheck className="w-4 h-4 text-blue-500" /> 积分变动账单
        </h3>

        <div className="divide-y divide-slate-100 dark:divide-slate-800">
          {mockPointTransactions.map((tx) => (
            <div key={tx.id} className="py-3.5 flex items-center justify-between text-xs">
              <div className="flex items-center gap-3">
                <div
                  className={`w-8 h-8 rounded-xl flex items-center justify-center font-bold ${
                    tx.type === 'earn'
                      ? 'bg-emerald-100 dark:bg-emerald-950/60 text-emerald-600 dark:text-emerald-400'
                      : 'bg-rose-100 dark:bg-rose-950/60 text-rose-600 dark:text-rose-400'
                  }`}
                >
                  {tx.type === 'earn' ? '+' : '-'}
                </div>
                <div>
                  <div className="font-bold text-slate-900 dark:text-white">{tx.title}</div>
                  <div className="text-[10px] text-slate-400 font-mono">{tx.date} • ID: {tx.id}</div>
                </div>
              </div>

              <div className={`font-mono font-extrabold text-sm ${tx.type === 'earn' ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'}`}>
                {tx.type === 'earn' ? `+${tx.amount}` : tx.amount} PTS
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
