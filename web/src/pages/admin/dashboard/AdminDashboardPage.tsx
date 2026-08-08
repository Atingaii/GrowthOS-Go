import React from 'react';
import { Gift, Coins, Users, Activity, BarChart3, Bot, Sparkles, TrendingUp } from 'lucide-react';
import { MetricCard } from '../../../components/common/UIComponents';
import { GrowthFunnelIllustration, McpArchitectureDiagram } from '../../../components/common/GrowthOSGraphics';

export const AdminDashboardPage: React.FC = () => {
  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 pb-4 border-b border-stone-200 dark:border-neutral-800">
        <div>
          <p className="text-xs font-mono text-stone-400 dark:text-stone-500 uppercase tracking-widest mb-1">
            Growth Cockpit · 2026 Q1
          </p>
          <h1 className="text-2xl font-bold text-stone-900 dark:text-stone-50">
            增长运营大盘
          </h1>
          <p className="text-xs text-stone-400 dark:text-stone-500 mt-1">
            Go 微服务集群 · 裂变活动转化率 · 积分通胀率 · MCP AI Agent 自动化指标
          </p>
        </div>
        <span className="text-xs font-mono text-emerald-600 dark:text-emerald-400 shrink-0">
          ● 实时数据流 Live
        </span>
      </div>

      {/* Metrics Strip */}
      <div className="grid grid-cols-2 lg:grid-cols-4 border border-stone-200 dark:border-neutral-800 divide-x divide-stone-200 dark:divide-neutral-800">
        <MetricCard title="平台总注册用户" value="128,450" change="+14.2%" icon={Users} color="blue" />
        <MetricCard title="营销活动" value="38" change="+4" icon={Gift} color="purple" badgeText="12 个运行中" />
        <MetricCard title="积分池流通总量" value="14.28M" change="+8.1%" icon={Coins} color="amber" />
        <MetricCard title="AI 工具 24h 调用" value="45,920" change="+32%" icon={Bot} color="emerald" />
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-[1fr_1fr] gap-px bg-stone-200 dark:bg-neutral-800 border border-stone-200 dark:border-neutral-800">
        <div className="bg-white dark:bg-[#141414] p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-stone-800 dark:text-stone-200">全链路增长转化漏斗</h3>
            <span className="text-xs font-mono text-emerald-600 dark:text-emerald-400">转化率 12.4%</span>
          </div>
          <GrowthFunnelIllustration />
        </div>

        <div className="bg-white dark:bg-[#141414] p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-stone-800 dark:text-stone-200">MCP Gateway 架构状态</h3>
            <span className="text-xs font-mono text-stone-400 dark:text-stone-500">实时状态</span>
          </div>
          <McpArchitectureDiagram />
        </div>
      </div>
    </div>
  );
};
