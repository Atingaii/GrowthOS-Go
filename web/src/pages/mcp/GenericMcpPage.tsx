import React from 'react';
import { Server, Wrench, ShieldCheck, FileCode } from 'lucide-react';

export const GenericMcpPage: React.FC<{ title: string; subtitle: string; icon: any }> = ({
  title,
  subtitle,
  icon: Icon,
}) => {
  return (
    <div className="space-y-6 font-mono">
      <div>
        <h1 className="text-2xl font-extrabold text-white flex items-center gap-2">
          <Icon className="w-6 h-6 text-indigo-400" /> {title}
        </h1>
        <p className="text-xs text-slate-400 mt-1">{subtitle}</p>
      </div>

      <div className="bg-slate-900 rounded-2xl p-8 border border-slate-800 space-y-4">
        <div className="flex items-center justify-between border-b border-slate-800 pb-4 text-xs">
          <span className="text-indigo-400">MCP GATEWAY SUB-SYSTEM</span>
          <span className="text-emerald-400">JSON-RPC / SSE ENDPOINT ACTIVE</span>
        </div>

        <div className="py-12 text-center space-y-3">
          <Icon className="w-8 h-8 text-indigo-400 mx-auto" />
          <h3 className="text-base font-bold text-white">{title} 网关模块入口</h3>
          <p className="text-xs text-slate-400 max-w-md mx-auto font-sans">
            本页面已具备完整 MCP 客户端通信接口，支持与 Go 后端 MCP Server 协议直接对接。
          </p>
        </div>
      </div>
    </div>
  );
};
