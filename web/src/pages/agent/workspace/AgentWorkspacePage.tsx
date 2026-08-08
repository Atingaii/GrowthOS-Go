import React, { useState } from 'react';
import { Bot, Send, Sparkles, AlertTriangle, Shield, CheckCircle2, RefreshCw } from 'lucide-react';
import { mockAgentTasks, mockAgentApprovals } from '../../../mocks/growthOsMockData';
import { StatusBadge } from '../../../components/common/UIComponents';

export const AgentWorkspacePage: React.FC = () => {
  const [prompt, setPrompt] = useState('');
  const [loading, setLoading] = useState(false);
  const [tasks, setTasks] = useState(mockAgentTasks);

  const handleRunAgent = (e: React.FormEvent) => {
    e.preventDefault();
    if (!prompt.trim()) return;
    setLoading(true);

    setTimeout(() => {
      const newTask = {
        id: `tsk_${Date.now().toString().slice(-3)}`,
        title: prompt.slice(0, 30) + '...',
        status: 'running' as const,
        agentName: 'GrowthAgent-Executor',
        prompt: prompt,
        startedAt: new Date().toISOString().replace('T', ' ').slice(0, 19),
        mcpToolsUsed: ['calculate_funnel_conversion'],
        riskLevel: 'medium' as const,
      };
      setTasks([newTask, ...tasks]);
      setPrompt('');
      setLoading(false);
    }, 1200);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-extrabold text-white flex items-center gap-2">
            <Bot className="w-6 h-6 text-emerald-400" /> AI Operator 工作台
          </h1>
          <p className="text-xs text-slate-400 mt-1">
            自然语言触发自动化营销策略，Agent 通过 MCP Gateway 调度 GrowthOS API。
          </p>
        </div>
      </div>

      {/* Prompt Input Box */}
      <div className="bg-slate-950 rounded-3xl p-6 border border-slate-800 shadow-2xl space-y-4">
        <form onSubmit={handleRunAgent} className="space-y-3">
          <label className="text-xs font-bold text-slate-300 flex items-center gap-2">
            <Sparkles className="w-4 h-4 text-emerald-400" /> 下达 AI 增长营销指令
          </label>
          <div className="relative">
            <textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="如：分析最近 7 天 /campaigns/cmp_001 活动的转化流失率，自动生成 20% 优惠券给流失用户..."
              rows={3}
              className="w-full p-4 rounded-2xl bg-slate-900 border border-slate-800 text-xs text-slate-100 placeholder-slate-500 focus:ring-2 focus:ring-emerald-500 focus:outline-none"
            />
            <button
              type="submit"
              disabled={loading || !prompt.trim()}
              className="absolute bottom-3 right-3 px-4 py-2 rounded-xl bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white text-xs font-bold flex items-center gap-1.5 shadow-lg transition-all"
            >
              {loading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4" />}
              {loading ? 'AI 规划中...' : '生成并执行 Task'}
            </button>
          </div>
        </form>
      </div>

      {/* Live Agent Tasks Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Active Tasks */}
        <div className="lg:col-span-2 space-y-4">
          <h3 className="text-sm font-bold text-slate-200 flex items-center gap-2">
            <Bot className="w-4 h-4 text-emerald-400" /> 正在执行的 AI Tasks ({tasks.length})
          </h3>

          <div className="space-y-3">
            {tasks.map((task) => (
              <div key={task.id} className="p-5 rounded-2xl bg-slate-950 border border-slate-800 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-bold text-slate-400">{task.id}</span>
                    <span className="text-xs font-bold text-white">{task.title}</span>
                  </div>
                  <StatusBadge status={task.status} />
                </div>

                <p className="text-xs text-slate-400 bg-slate-900/80 p-3 rounded-xl font-mono">
                  {task.prompt}
                </p>

                <div className="flex items-center justify-between text-[11px] text-slate-500">
                  <div className="flex items-center gap-2">
                    <span>MCP Tools:</span>
                    {task.mcpToolsUsed.map((t) => (
                      <span key={t} className="px-2 py-0.5 rounded bg-indigo-950 text-indigo-300 font-mono">
                        {t}
                      </span>
                    ))}
                  </div>
                  <span>{task.startedAt}</span>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Pending High Risk Approvals */}
        <div className="space-y-4">
          <h3 className="text-sm font-bold text-rose-400 flex items-center gap-2">
            <AlertTriangle className="w-4 h-4 text-rose-500" /> 待人工审批 (Human-in-the-Loop)
          </h3>

          <div className="space-y-3">
            {mockAgentApprovals.map((app) => (
              <div key={app.id} className="p-4 rounded-2xl bg-slate-950 border border-rose-900/40 space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-[10px] font-mono font-bold px-2 py-0.5 rounded bg-rose-950 text-rose-400 border border-rose-800">
                    {app.riskLevel.toUpperCase()} RISK
                  </span>
                  <span className="text-xs text-slate-400 font-mono">{app.requestedBy}</span>
                </div>

                <div className="text-xs font-bold text-white">{app.taskTitle}</div>

                <div className="text-[11px] font-mono text-slate-400 bg-slate-900 p-2.5 rounded-xl space-y-1">
                  <div>Tool: <span className="text-cyan-400">{app.toolName}</span></div>
                  <div>Params: {JSON.stringify(app.parameters)}</div>
                </div>

                <div className="flex items-center gap-2 pt-2">
                  <button className="flex-1 py-1.5 rounded-xl bg-emerald-600 text-white font-bold text-xs flex items-center justify-center gap-1 hover:bg-emerald-500">
                    <CheckCircle2 className="w-3.5 h-3.5" /> 批准执行
                  </button>
                  <button className="flex-1 py-1.5 rounded-xl bg-rose-950 text-rose-300 font-bold text-xs border border-rose-800 hover:bg-rose-900">
                    驳回
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
