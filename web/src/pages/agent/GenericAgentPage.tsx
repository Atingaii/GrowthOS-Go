import React from 'react';
import { ListTodo, CheckCircle2, History } from 'lucide-react';

export const GenericAgentPage: React.FC<{ title: string; subtitle: string; icon: any }> = ({
  title,
  subtitle,
  icon: Icon,
}) => {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-extrabold text-white flex items-center gap-2">
          <Icon className="w-6 h-6 text-emerald-400" /> {title}
        </h1>
        <p className="text-xs text-slate-400 mt-1">{subtitle}</p>
      </div>

      <div className="bg-slate-950 rounded-2xl p-8 border border-slate-800 space-y-4">
        <div className="flex items-center justify-between border-b border-slate-800 pb-4 text-xs">
          <span className="text-emerald-400 font-mono">AI OPERATOR AGENT ENGINE</span>
          <span className="text-slate-400">HUMAN-IN-THE-LOOP ACTIVE</span>
        </div>

        <div className="py-12 text-center space-y-3">
          <Icon className="w-8 h-8 text-emerald-400 mx-auto" />
          <h3 className="text-base font-bold text-white">{title} AI 模块</h3>
          <p className="text-xs text-slate-400 max-w-md mx-auto">
            与 Agent 任务调度与高风险操作人工确认链路连接。
          </p>
        </div>
      </div>
    </div>
  );
};
