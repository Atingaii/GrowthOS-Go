import React from 'react';
import { Link } from 'react-router';
import {
  Sparkles,
  Gift,
  Trophy,
  Coins,
  Ticket,
  ArrowRight,
  TrendingUp,
  Flame,
  Zap,
} from 'lucide-react';
import { MetricCard } from '../../../components/common/UIComponents';
import { GrowthFunnelIllustration } from '../../../components/common/GrowthOSGraphics';
import { mockCampaigns, mockFeedItems } from '../../../mocks/growthOsMockData';
import { useAppStore } from '../../../stores/appStore';

export const UserHomePage: React.FC = () => {
  const { user } = useAppStore();

  return (
    <div className="space-y-0">
      {/* Hero — typographic, flat, structured */}
      <section className="grid grid-cols-1 lg:grid-cols-[1fr_auto] gap-0 bg-white dark:bg-[#141414] border border-stone-200 dark:border-neutral-800 mb-6">
        <div className="p-8 sm:p-10 border-r-0 lg:border-r border-stone-200 dark:border-neutral-800">
          <p className="text-xs font-mono text-stone-400 dark:text-stone-500 uppercase tracking-widest mb-6">
            GrowthOS · AI Native Growth Platform
          </p>
          <h1 className="text-3xl sm:text-4xl font-bold tracking-tight text-stone-900 dark:text-stone-50 mb-3 leading-tight">
            早上好，{user.name}。<br />
            <span className="text-stone-400 dark:text-stone-500 font-normal">今天是推进增长的好时机。</span>
          </h1>
          <p className="text-sm text-stone-500 dark:text-stone-400 mb-8 max-w-lg leading-relaxed">
            4 个进行中的裂变活动，其中 1 个等待您确认参与。参与 AI 任务最高可解锁 5,000 积分奖励。
          </p>
          <div className="flex flex-wrap items-center gap-3">
            <Link
              to="/campaigns"
              className="inline-flex items-center gap-2 px-5 py-2.5 bg-blue-600 hover:bg-blue-700 text-white font-semibold text-sm transition-colors"
            >
              <Gift className="w-4 h-4" /> 探索营销活动
            </Link>
            <Link
              to="/lottery"
              className="inline-flex items-center gap-2 px-5 py-2.5 border border-stone-300 dark:border-neutral-700 text-stone-700 dark:text-stone-300 hover:bg-stone-50 dark:hover:bg-neutral-800 font-medium text-sm transition-colors"
            >
              <Trophy className="w-4 h-4" /> 每日抽奖
            </Link>
          </div>
        </div>

        {/* Points display panel */}
        <div className="p-8 sm:p-10 flex flex-col justify-between min-w-[220px] bg-stone-50 dark:bg-[#0f0f0f]">
          <div>
            <p className="text-xs font-mono text-stone-400 dark:text-stone-500 uppercase tracking-widest mb-3">当前积分</p>
            <div className="text-5xl font-bold tabular-nums text-stone-900 dark:text-stone-50 tracking-tight leading-none mb-1">
              {user.points.toLocaleString()}
            </div>
            <p className="text-xs text-stone-400 font-mono">PTS · {user.level}</p>
          </div>
          <div className="mt-8 pt-6 border-t border-stone-200 dark:border-neutral-800 space-y-2">
            <div className="flex items-center justify-between text-xs">
              <span className="text-stone-400 dark:text-stone-500">本周增幅</span>
              <span className="text-emerald-600 dark:text-emerald-400 font-mono font-semibold">+12%</span>
            </div>
            <div className="flex items-center justify-between text-xs">
              <span className="text-stone-400 dark:text-stone-500">距下一等级</span>
              <span className="text-stone-600 dark:text-stone-400 font-mono">840 pts</span>
            </div>
            <div className="h-1.5 bg-stone-200 dark:bg-neutral-700 mt-3">
              <div className="h-full bg-blue-600 dark:bg-blue-500 w-[68%]" />
            </div>
          </div>
        </div>
      </section>

      {/* Metrics Strip — borderless, flush */}
      <section className="grid grid-cols-2 lg:grid-cols-4 border-b border-stone-200 dark:border-neutral-800 mb-6 divide-x divide-stone-200 dark:divide-neutral-800">
        <MetricCard title="当前账户积分" value={user.points.toLocaleString()} change="+12% 本周" icon={Coins} color="amber" badgeText={user.level} />
        <MetricCard title="可用优惠券" value="2 张" subtitle="1 张即将过期" icon={Ticket} color="blue" />
        <MetricCard title="参与营销活动" value="4 个" change="+2" icon={Gift} color="purple" />
        <MetricCard title="Growth Feed 获赞" value="854" change="+34%" icon={TrendingUp} color="emerald" />
      </section>

      {/* Main Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-6">
        {/* Left: Campaigns + Funnel */}
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-stone-800 dark:text-stone-200">裂变活动</h2>
            <Link to="/campaigns" className="text-xs text-blue-600 dark:text-blue-400 hover:underline flex items-center gap-1">
              查看全部 <ArrowRight className="w-3 h-3" />
            </Link>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-px bg-stone-200 dark:bg-neutral-800 border border-stone-200 dark:border-neutral-800">
            {mockCampaigns.slice(0, 4).map((campaign) => (
              <div
                key={campaign.id}
                className="bg-white dark:bg-[#141414] p-5 flex flex-col justify-between hover:bg-stone-50 dark:hover:bg-[#1a1a1a] transition-colors"
              >
                <div>
                  <div className="flex items-center justify-between mb-3">
                    <span className="text-[10px] font-mono uppercase tracking-wider text-stone-400 dark:text-stone-500">
                      {campaign.category}
                    </span>
                    <span className="text-xs font-mono font-semibold text-emerald-600 dark:text-emerald-400">
                      {campaign.rewardAmount}
                    </span>
                  </div>
                  <h3 className="font-semibold text-sm text-stone-900 dark:text-stone-100 mb-1.5 leading-snug">
                    {campaign.title}
                  </h3>
                  <p className="text-xs text-stone-400 dark:text-stone-500 line-clamp-2 leading-relaxed">
                    {campaign.subtitle}
                  </p>
                </div>

                <div className="pt-4 flex items-center justify-between text-xs">
                  <span className="text-stone-400 font-mono">{campaign.participants.toLocaleString()} 人</span>
                  <Link
                    to={`/campaigns/${campaign.id}`}
                    className="font-semibold text-blue-600 dark:text-blue-400 hover:underline"
                  >
                    立即参与 →
                  </Link>
                </div>
              </div>
            ))}
          </div>

          {/* Funnel */}
          <div>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-stone-800 dark:text-stone-200">转化漏斗监控</h3>
              <span className="text-xs font-mono text-emerald-600 dark:text-emerald-400">综合转化率 12.4%</span>
            </div>
            <GrowthFunnelIllustration />
          </div>
        </div>

        {/* Right: Growth Feed */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-stone-800 dark:text-stone-200">Growth Feed</h2>
            <Link to="/feed" className="text-xs text-blue-600 dark:text-blue-400 hover:underline">
              更多
            </Link>
          </div>

          <div className="space-y-px bg-stone-200 dark:bg-neutral-800 border border-stone-200 dark:border-neutral-800">
            {mockFeedItems.map((item) => (
              <div
                key={item.id}
                className="bg-white dark:bg-[#141414] p-4 space-y-2.5 hover:bg-stone-50 dark:hover:bg-[#1a1a1a] transition-colors"
              >
                <div className="flex items-center gap-2.5">
                  <img src={item.authorAvatar} alt={item.authorName} className="w-7 h-7 rounded-full object-cover" />
                  <div>
                    <div className="text-xs font-semibold text-stone-800 dark:text-stone-200 leading-none">{item.authorName}</div>
                    <div className="text-[10px] text-stone-400 mt-0.5">{item.publishedAt}</div>
                  </div>
                </div>

                <h4 className="text-xs font-semibold text-stone-800 dark:text-stone-200 leading-snug">
                  {item.title}
                </h4>
                <p className="text-xs text-stone-400 dark:text-stone-500 line-clamp-2 leading-relaxed">
                  {item.content}
                </p>

                <div className="flex items-center gap-4 pt-1 text-[11px] text-stone-400">
                  <span>{item.likes} 赞</span>
                  <span>{item.comments} 评论</span>
                  <span className="font-mono text-blue-500 ml-auto">#{item.tag}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
