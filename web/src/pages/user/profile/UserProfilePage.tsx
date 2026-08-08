import React from 'react';
import { Shield, CheckCircle2 } from 'lucide-react';
import { useAppStore } from '../../../stores/appStore';

const LEVEL_LABEL: Record<string, string> = {
  'Platinum Growth Tier': '铂金成长会员',
  'Gold Growth Tier': '黄金成长会员',
  'Silver Growth Tier': '白银成长会员',
  'Bronze Growth Tier': '青铜成长会员',
};

export const UserProfilePage: React.FC = () => {
  const { user } = useAppStore();
  const levelLabel = LEVEL_LABEL[user.level] ?? '成长会员';

  return (
    <div className="max-w-3xl mx-auto space-y-4">
      {/* Profile header */}
      <div className="bg-white dark:bg-[#141414] border border-stone-200 dark:border-neutral-800 p-6 flex items-center gap-5">
        <img src={user.avatar} alt={user.name} className="w-16 h-16 rounded-full object-cover" />
        <div className="space-y-1">
          <div className="flex items-center gap-2.5">
            <h1 className="text-xl font-bold text-stone-900 dark:text-stone-50">{user.name}</h1>
            <span className="text-[10px] font-medium px-2 py-0.5 bg-blue-50 dark:bg-blue-950/50 text-blue-600 dark:text-blue-400 border border-blue-200 dark:border-blue-900">
              {levelLabel}
            </span>
          </div>
          <div className="text-xs text-stone-400">{user.email}</div>
        </div>
      </div>

      {/* Account security */}
      <div className="bg-white dark:bg-[#141414] border border-stone-200 dark:border-neutral-800 p-6 space-y-4">
        <h3 className="font-semibold text-sm text-stone-800 dark:text-stone-200 flex items-center gap-2">
          <Shield className="w-4 h-4 text-blue-500" /> 账号安全
        </h3>

        <div className="divide-y divide-stone-100 dark:divide-neutral-800 text-xs">
          <div className="py-3 flex justify-between items-center">
            <span className="text-stone-500 dark:text-stone-400">会员等级</span>
            <span className="font-medium text-stone-800 dark:text-stone-200">{levelLabel}</span>
          </div>
          <div className="py-3 flex justify-between items-center">
            <span className="text-stone-500 dark:text-stone-400">账号认证</span>
            <span className="text-emerald-600 dark:text-emerald-400 flex items-center gap-1 font-medium">
              <CheckCircle2 className="w-3.5 h-3.5" /> 已认证
            </span>
          </div>
          <div className="py-3 flex justify-between items-center">
            <span className="text-stone-500 dark:text-stone-400">登录邮箱</span>
            <span className="text-stone-600 dark:text-stone-300">{user.email}</span>
          </div>
        </div>
      </div>
    </div>
  );
};
