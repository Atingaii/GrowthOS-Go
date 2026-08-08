import React from 'react';
import { Terminal, Server, Wrench, ShieldCheck, Activity, Cpu } from 'lucide-react';
import { MetricCard, StatusBadge } from '../../components/common/UIComponents';
import { mockMcpServers, mockMcpTools } from '../../mocks/growthOsMockData';

export const McpDashboardPage: React.FC = () => {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white flex items-center gap-2">
            <Terminal className="w-5 h-5 text-indigo-400" /> AI 工具调度控制台
          </h1>
          <p className="text-xs text-slate-400 mt-1">
            监控已接入的智能工具、调度节点状态与调用质量。
          </p>
        </div>
        <span className="text-xs font-mono text-emerald-400 flex items-center gap-1.5">
          <span className="w-1.5 h-1.5 bg-emerald-500 rounded-full animate-pulse" /> 调度节点在线
        </span>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <MetricCard title="在线 MCP Server" value="3 个" icon={Server} color="purple" />
        <MetricCard title="已注册工具" value="28 个" icon={Wrench} color="cyan" badgeText="策略生效中" />
        <MetricCard title="24h 工具调用总量" value="124,890" icon={Activity} color="emerald" badgeText="均延迟 32ms" />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Servers */}
        <div className="bg-slate-900 rounded-2xl p-6 border border-slate-800 space-y-4">
          <h3 className="text-sm font-bold text-slate-200 flex items-center gap-2">
            <Server className="w-4 h-4 text-indigo-400" /> 运行中的 MCP Server 节点
          </h3>
          <div className="divide-y divide-slate-800 text-xs">
            {mockMcpServers.map((srv) => (
              <div key={srv.id} className="py-3 flex items-center justify-between">
                <div>
                  <div className="font-bold text-white">{srv.name}</div>
                  <div className="text-[10px] text-slate-500 font-mono">{srv.endpoint} • {srv.version}</div>
                </div>
                <div className="flex items-center gap-3">
                  <span className="font-mono text-slate-400">{srv.avgLatencyMs}ms</span>
                  <StatusBadge status={srv.status} />
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Tools */}
        <div className="bg-slate-900 rounded-2xl p-6 border border-slate-800 space-y-4">
          <h3 className="text-sm font-bold text-slate-200 flex items-center gap-2">
            <Wrench className="w-4 h-4 text-cyan-400" /> 高频 MCP Tool 风险与成功率
          </h3>
          <div className="divide-y divide-slate-800 text-xs">
            {mockMcpTools.map((tool) => (
              <div key={tool.id} className="py-3 flex items-center justify-between">
                <div>
                  <div className="font-bold text-white font-mono">{tool.name}</div>
                  <div className="text-[10px] text-slate-400 line-clamp-1">{tool.description}</div>
                </div>
                <div className="flex items-center gap-3">
                  <span className={`text-[10px] font-bold uppercase px-2 py-0.5 rounded ${
                    tool.riskLevel === 'critical' ? 'bg-rose-950 text-rose-400 border border-rose-800' : 'bg-amber-950 text-amber-400'
                  }`}>
                    {tool.riskLevel}
                  </span>
                  <span className="font-mono text-emerald-400 font-bold">{tool.successRate}%</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
