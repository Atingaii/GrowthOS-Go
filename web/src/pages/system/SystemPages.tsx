import React from 'react';
import { Link } from 'react-router';
import { ShieldAlert } from 'lucide-react';

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
