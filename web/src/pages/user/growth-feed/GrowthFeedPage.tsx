import React from 'react';
import { Flame, MessageSquare, ThumbsUp, Share2, Sparkles, Plus } from 'lucide-react';
import { mockFeedItems } from '../../../mocks/growthOsMockData';

export const GrowthFeedPage: React.FC = () => {
  return (
    <div className="max-w-4xl mx-auto space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-extrabold text-slate-900 dark:text-white flex items-center gap-2">
            <Flame className="w-6 h-6 text-orange-500" /> Growth Feed 社区态势
          </h1>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            展示 AI Agent 案例、增长玩法、A/B 实验洞察与最新增长话题。
          </p>
        </div>
        <button className="px-4 py-2 rounded-xl bg-blue-600 text-white font-bold text-xs flex items-center gap-1.5 shadow-md">
          <Plus className="w-4 h-4" /> 发布 Feed
        </button>
      </div>

      {/* Feed List */}
      <div className="space-y-4">
        {mockFeedItems.map((feed) => (
          <div key={feed.id} className="bg-white dark:bg-slate-900 rounded-2xl p-6 border border-slate-200 dark:border-slate-800 shadow-sm space-y-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <img src={feed.authorAvatar} alt={feed.authorName} className="w-10 h-10 rounded-full object-cover" />
                <div>
                  <div className="font-bold text-sm text-slate-900 dark:text-white">{feed.authorName}</div>
                  <div className="text-xs text-slate-400">{feed.authorRole} • {feed.publishedAt}</div>
                </div>
              </div>
              <span className="text-xs font-mono font-bold px-2.5 py-1 rounded-full bg-blue-50 dark:bg-blue-950 text-blue-600 dark:text-blue-400">
                #{feed.tag}
              </span>
            </div>

            <div>
              <h3 className="text-base font-bold text-slate-900 dark:text-white mb-2">{feed.title}</h3>
              <p className="text-xs text-slate-600 dark:text-slate-300 leading-relaxed">{feed.content}</p>
            </div>

            {feed.campaignLink && (
              <div className="p-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200/80 dark:border-slate-700/80 flex items-center justify-between text-xs">
                <span className="text-slate-600 dark:text-slate-300 font-medium flex items-center gap-1.5">
                  <Sparkles className="w-4 h-4 text-amber-500" /> 关联增长营销活动
                </span>
                <a href={feed.campaignLink} className="font-bold text-blue-600 dark:text-blue-400 hover:underline">
                  查看活动详情 →
                </a>
              </div>
            )}

            <div className="flex items-center gap-6 pt-3 border-t border-slate-100 dark:border-slate-800 text-xs text-slate-500">
              <button className="flex items-center gap-1.5 hover:text-blue-500 font-medium">
                <ThumbsUp className="w-4 h-4" /> {feed.likes} 点赞
              </button>
              <button className="flex items-center gap-1.5 hover:text-blue-500 font-medium">
                <MessageSquare className="w-4 h-4" /> {feed.comments} 讨论
              </button>
              <button className="flex items-center gap-1.5 hover:text-blue-500 font-medium">
                <Share2 className="w-4 h-4" /> {feed.shares} 分享
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
