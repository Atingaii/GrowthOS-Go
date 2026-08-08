import React from 'react';
import { Link, Outlet, useLocation } from 'react-router';
import {
  Terminal,
  Server,
  Wrench,
  ShieldCheck,
  FileCode,
  Home,
  Shield,
  Bot,
} from 'lucide-react';
import { GrowthOSLogo } from '../components/common/GrowthOSGraphics';

export const McpLayout: React.FC = () => {
  const location = useLocation();

  const mcpNav = [
    { label: '网关概览', path: '/mcp/dashboard', icon: Terminal },
    { label: 'MCP 服务节点', path: '/mcp/servers', icon: Server },
    { label: 'Tool 工具库', path: '/mcp/tools', icon: Wrench },
    { label: '安全策略与权限', path: '/mcp/permissions', icon: ShieldCheck },
    { label: '调用审计日志', path: '/mcp/audit', icon: FileCode },
  ];

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col font-mono">
      {/* High-Tech Dark Header */}
      <header className="h-16 bg-slate-900 border-b border-indigo-900/50 px-6 flex items-center justify-between sticky top-0 z-40">
        <div className="flex items-center gap-6">
          <Link to="/mcp/dashboard">
            <GrowthOSLogo />
          </Link>
          <span className="text-xs bg-indigo-950 text-indigo-300 border border-indigo-700/50 px-2.5 py-1 rounded-md font-bold">
            MCP GATEWAY v2.1 (Go)
          </span>
        </div>

        {/* Top Nav */}
        <nav className="hidden md:flex items-center gap-2">
          {mcpNav.map((item) => {
            const Icon = item.icon;
            const isActive = location.pathname === item.path;
            return (
              <Link
                key={item.path}
                to={item.path}
                className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                  isActive
                    ? 'bg-indigo-600 text-white shadow-lg shadow-indigo-500/30'
                    : 'text-slate-400 hover:bg-slate-800 hover:text-white'
                }`}
              >
                <Icon className="w-4 h-4" />
                {item.label}
              </Link>
            );
          })}
        </nav>

        {/* Quick Switch */}
        <div className="flex items-center gap-3">
          <Link to="/agent/workspace" className="text-xs text-emerald-400 hover:underline flex items-center gap-1 font-sans font-medium">
            <Bot className="w-3.5 h-3.5" /> AI Operator
          </Link>
          <Link to="/admin/dashboard" className="text-xs text-blue-400 hover:underline flex items-center gap-1 font-sans font-medium">
            <Shield className="w-3.5 h-3.5" /> 运营后台
          </Link>
          <Link to="/home" className="text-xs text-slate-400 hover:underline flex items-center gap-1 font-sans font-medium">
            <Home className="w-3.5 h-3.5" /> 用户端
          </Link>
        </div>
      </header>

      {/* Main MCP Console Content */}
      <main className="flex-1 max-w-7xl w-full mx-auto p-6">
        <Outlet />
      </main>
    </div>
  );
};
