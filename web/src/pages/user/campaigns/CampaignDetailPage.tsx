import React from 'react';
import { useParams, Link } from 'react-router';
import { Gift, ArrowLeft, Users, Calendar, Coins, Share2, CheckCircle2 } from 'lucide-react';
import { mockCampaigns } from '../../../mocks/growthOsMockData';

export const CampaignDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const campaign = mockCampaigns.find((c) => c.id === id) || mockCampaigns[0];

  return (
    <div className="max-w-4xl mx-auto space-y-6">
      <Link to="/campaigns" className="inline-flex items-center gap-1.5 text-xs font-bold text-slate-500 hover:text-blue-500">
        <ArrowLeft className="w-4 h-4" /> 返回活动列表
      </Link>

      <div className={`p-8 rounded-3xl bg-gradient-to-r ${campaign.bannerBg} text-white shadow-xl`}>
        <div className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-white/20 backdrop-blur-md text-xs font-bold mb-4">
          <Gift className="w-3.5 h-3.5 text-amber-300" /> {campaign.category}
        </div>
        <h1 className="text-3xl font-extrabold mb-2">{campaign.title}</h1>
        <p className="text-slate-200 text-sm max-w-2xl">{campaign.subtitle}</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div className="md:col-span-2 space-y-6">
          <div className="bg-white dark:bg-slate-900 rounded-2xl p-6 border border-slate-200 dark:border-slate-800 space-y-4">
            <h3 className="font-bold text-base text-slate-900 dark:text-white">活动任务与奖励规则</h3>
            <ul className="space-y-3 text-xs text-slate-600 dark:text-slate-300">
              <li className="flex items-start gap-2">
                <CheckCircle2 className="w-4 h-4 text-emerald-500 shrink-0 mt-0.5" />
                <span>任务 1：点击下方专属邀请链接或生成口令分享至社交平台。</span>
              </li>
              <li className="flex items-start gap-2">
                <CheckCircle2 className="w-4 h-4 text-emerald-500 shrink-0 mt-0.5" />
                <span>任务 2：被邀请人成功注册并完成 GrowthOS 账号绑定。</span>
              </li>
              <li className="flex items-start gap-2">
                <CheckCircle2 className="w-4 h-4 text-emerald-500 shrink-0 mt-0.5" />
                <span>奖励解锁：即刻发放 <span className="font-bold text-blue-500">{campaign.rewardAmount}</span> 到您的积分资产账户。</span>
              </li>
            </ul>

            <button className="w-full py-3 rounded-xl bg-blue-600 hover:bg-blue-500 text-white font-bold text-sm flex items-center justify-center gap-2 shadow-md">
              <Share2 className="w-4 h-4" /> 复制我的专属邀请链接
            </button>
          </div>
        </div>

        <div className="space-y-4">
          <div className="bg-white dark:bg-slate-900 rounded-2xl p-5 border border-slate-200 dark:border-slate-800 space-y-3 text-xs">
            <div className="font-bold text-slate-900 dark:text-white pb-2 border-b border-slate-100 dark:border-slate-800">
              活动数据看板
            </div>
            <div className="flex justify-between text-slate-500">
              <span className="flex items-center gap-1"><Users className="w-3.5 h-3.5" /> 参与人数</span>
              <span className="font-mono font-bold text-slate-900 dark:text-white">{campaign.participants}</span>
            </div>
            <div className="flex justify-between text-slate-500">
              <span className="flex items-center gap-1"><Calendar className="w-3.5 h-3.5" /> 活动周期</span>
              <span className="font-mono">{campaign.startDate} ~ {campaign.endDate}</span>
            </div>
            <div className="flex justify-between text-slate-500">
              <span className="flex items-center gap-1"><Coins className="w-3.5 h-3.5" /> 奖励池</span>
              <span className="font-mono font-bold text-emerald-500">{campaign.rewardAmount}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
