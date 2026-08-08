import React from 'react';
import { Link } from 'react-router';
import { ArrowLeft, ShieldAlert } from 'lucide-react';

export const StatusPage: React.FC = () => {
  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">系统运行状态</h1>
        <span className="text-xs font-mono text-emerald-600 dark:text-emerald-400 flex items-center gap-1.5">
          <span className="w-1.5 h-1.5 bg-emerald-500 rounded-full" /> 全部正常
        </span>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {[
          { name: '接口网关', latency: '12ms' },
          { name: '营销活动引擎', latency: '18ms' },
          { name: '抽奖与策略', latency: '15ms' },
          { name: '积分账本', latency: '22ms' },
          { name: 'AI 工具调度', latency: '28ms' },
          { name: '智能增长助手', latency: '45ms' },
        ].map((svc, i) => (
          <div key={i} className="p-4 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 flex items-center justify-between text-xs">
            <div className="font-medium text-slate-900 dark:text-white">{svc.name}</div>
            <div className="flex items-center gap-3">
              <span className="text-stone-400 font-mono">{svc.latency}</span>
              <span className="text-emerald-500 font-mono">● 正常</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

export const Error403Page: React.FC = () => (
  <div className="text-center py-16 space-y-4">
    <ShieldAlert className="w-12 h-12 text-rose-500 mx-auto" />
    <h1 className="text-3xl font-extrabold text-slate-900 dark:text-white">403 无访问权限</h1>
    <p className="text-xs text-slate-500 max-w-sm mx-auto">您暂无权限访问此页面，请联系管理员。</p>
    <Link to="/home" className="inline-block px-4 py-2 rounded-xl bg-blue-600 text-white font-bold text-xs">返回首页</Link>
  </div>
);

export const Error404Page: React.FC = () => (
  <div className="text-center py-16 space-y-4">
    <div className="text-5xl font-mono font-extrabold text-blue-500">404</div>
    <h1 className="text-2xl font-extrabold text-slate-900 dark:text-white">页面未找到</h1>
    <p className="text-xs text-slate-500 max-w-sm mx-auto">页面不存在，请从导航栏进入。</p>
    <Link to="/home" className="inline-block px-4 py-2 rounded-xl bg-blue-600 text-white font-bold text-xs">返回首页</Link>
  </div>
);

export const Error500Page: React.FC = () => (
  <div className="text-center py-16 space-y-4">
    <div className="text-5xl font-mono font-extrabold text-rose-500">500</div>
    <h1 className="text-2xl font-extrabold text-slate-900 dark:text-white">服务暂时不可用</h1>
    <p className="text-xs text-slate-500 max-w-sm mx-auto">服务器暂时无响应，请稍后再试。</p>
    <Link to="/home" className="inline-block px-4 py-2 rounded-xl bg-blue-600 text-white font-bold text-xs">返回首页</Link>
  </div>
);

export const LoginPage: React.FC = () => (
  <div className="space-y-4 text-center">
    <h2 className="text-xl font-bold text-white">登录 GrowthOS 账号</h2>
    <p className="text-xs text-slate-400">使用您的账号安全登录</p>
    <Link to="/home" className="block w-full py-3 rounded-xl bg-blue-600 hover:bg-blue-500 text-white font-bold text-xs shadow-lg">
      一键以 Alex Rivera 身份登录
    </Link>
  </div>
);
