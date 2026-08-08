import React from 'react';
import { Link, Outlet, useLocation } from 'react-router';
import {
  Bot,
  ListTodo,
  CheckCircle2,
  History,
  Terminal,
  Shield,
  Home,
  Sparkles,
} from 'lucide-react';
import { GrowthOSLogo } from '../components/common/GrowthOSGraphics';

export const AgentLayout: React.FC = () => {
  const location = useLocation();

  const agentNav = [
    { label: 'AI 工作台', path: '/agent/workspace', icon: Bot },
    { label: '任务对列', path: '/agent/tasks', icon: ListTodo },
    { label: '人工审批中心', path: '/agent/approvals', icon: CheckCircle2, badge: '1' },
    { label: '执行历史', path: '/agent/history', icon: History },
  ];

  return (
    <div className="min-h-screen bg-slate-900 text-slate-100 flex flex-col font-sans">
      {/* Top Bar Header */}
      <header className="h-16 bg-slate-950 border-b border-slate-800 px-6 flex items-center justify-between sticky top-0 z-40">
        <div className="flex items-center gap-6">
          <Link to="/agent/workspace">
            <GrowthOSLogo />
          </Link>
          <div className="flex items-center gap-2 px-3 py-1 rounded-full bg-emerald-950/80 border border-emerald-500/30 text-emerald-400 text-xs font-semibold">
            <Sparkles className="w-3.5 h-3.5 animate-spin" />
            AI Marketing Agent Active
          </div>
        </div>

        {/* Navigation Tabs */}
        <nav className="flex items-center gap-2">
          {agentNav.map((item) => {
            const Icon = item.icon;
            const isActive = location.pathname === item.path;
            return (
              <Link
                key={item.path}
                to={item.path}
                className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-semibold transition-all relative ${
                  isActive
                    ? 'bg-emerald-600 text-white shadow-md shadow-emerald-600/30'
                    : 'text-slate-400 hover:bg-slate-800 hover:text-white'
                }`}
              >
                <Icon className="w-4 h-4" />
                {item.label}
                {item.badge && (
                  <span className="w-4 h-4 rounded-full bg-rose-500 text-white text-[10px] flex items-center justify-center font-bold">
                    {item.badge}
                  </span>
                )}
              </Link>
            );
          })}
        </nav>

        {/* Global Nav */}
        <div className="flex items-center gap-4 text-xs font-medium">
          <Link to="/mcp/dashboard" className="text-purple-400 hover:underline flex items-center gap-1">
            <Terminal className="w-3.5 h-3.5" /> MCP Gateway
          </Link>
          <Link to="/admin/dashboard" className="text-blue-400 hover:underline flex items-center gap-1">
            <Shield className="w-3.5 h-3.5" /> 运营
          </Link>
          <Link to="/home" className="text-slate-400 hover:underline flex items-center gap-1">
            <Home className="w-3.5 h-3.5" /> 用户
          </Link>
        </div>
      </header>

      {/* Main Agent Area */}
      <main className="flex-1 p-6 flex flex-col max-w-7xl w-full mx-auto">
        <Outlet />
      </main>
    </div>
  );
};
