import React from 'react';
import {
  FunnelChart,
  Funnel,
  Cell,
  Tooltip,
  ResponsiveContainer,
  LabelList,
} from 'recharts';

// GrowthOS Modern Geometric Visual Graphics & Charts

export const GrowthOSLogo: React.FC<{ className?: string; iconOnly?: boolean }> = ({ className = 'h-8', iconOnly = false }) => {
  return (
    <div className={`inline-flex items-center gap-2.5 select-none ${className}`}>
      <div className="w-7 h-7 bg-blue-600 flex items-center justify-center shrink-0">
        <svg viewBox="0 0 24 24" fill="none" className="w-4 h-4 text-white">
          <path d="M4 18L9.5 12.5L13.5 16.5L20 9" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
          <path d="M15 9H20V14" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </div>
      {!iconOnly && (
        <div className="flex flex-col">
          <span className="font-bold text-sm tracking-tight text-stone-900 dark:text-stone-50 leading-none">
            GrowthOS
          </span>
          <span className="text-[10px] font-mono tracking-widest text-stone-400 uppercase leading-tight">
            Growth Platform
          </span>
        </div>
      )}
    </div>
  );
};

const funnelData = [
  { name: '访客触达', value: 100000, label: '100,000', rate: '' },
  { name: '活动互动', value: 34500,  label: '34,500',  rate: '34.5%' },
  { name: '付费转化', value: 12400,  label: '12,400',  rate: '35.9%' },
];

const FUNNEL_COLORS = ['#2563eb', '#7c3aed', '#059669'];

const FunnelTooltipContent = ({ active, payload }: { active?: boolean; payload?: { payload: typeof funnelData[0] }[] }) => {
  if (!active || !payload?.length) return null;
  const d = payload[0].payload;
  return (
    <div className="bg-white dark:bg-neutral-900 border border-stone-200 dark:border-neutral-700 px-3 py-2 text-xs shadow-sm">
      <p className="font-semibold text-stone-800 dark:text-stone-100">{d.name}</p>
      <p className="text-stone-500 dark:text-stone-400 font-mono">{d.label} 人{d.rate ? `  ·  转化 ${d.rate}` : ''}</p>
    </div>
  );
};

export const GrowthFunnelIllustration: React.FC<{ className?: string }> = () => {
  return (
    <div className="w-full">
      {/* Stage stats row */}
      <div className="grid grid-cols-3 gap-px bg-stone-100 dark:bg-neutral-800 mb-4">
        {funnelData.map((d, i) => (
          <div key={d.name} className="bg-white dark:bg-[#141414] px-4 py-3">
            <p className="text-[10px] text-stone-400 dark:text-stone-500 mb-0.5">{d.name}</p>
            <p className="text-lg font-bold tabular-nums" style={{ color: FUNNEL_COLORS[i] }}>{d.label}</p>
            {d.rate && <p className="text-[10px] font-mono text-stone-400 dark:text-stone-500">转化 {d.rate}</p>}
          </div>
        ))}
      </div>
      {/* Funnel chart */}
      <ResponsiveContainer width="100%" height={180}>
        <FunnelChart>
          <Tooltip content={<FunnelTooltipContent />} />
          <Funnel dataKey="value" data={funnelData} isAnimationActive lastShapeType="rectangle">
            {funnelData.map((_, i) => (
              <Cell key={i} fill={FUNNEL_COLORS[i]} fillOpacity={0.85} />
            ))}
            <LabelList dataKey="name" position="center" fill="#fff" fontSize={12} fontWeight={600} />
          </Funnel>
        </FunnelChart>
      </ResponsiveContainer>
    </div>
  );
};

const ArrowRight: React.FC<{ color: string }> = ({ color }) => (
  <div className="flex items-center justify-center px-1 shrink-0">
    <svg width="28" height="12" viewBox="0 0 28 12" fill="none">
      <path d="M0 6 H22" stroke={color} strokeWidth="1.5" strokeDasharray="3 2" />
      <path d="M18 2 L24 6 L18 10" stroke={color} strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  </div>
);

interface McpNode {
  label: string;
  sub: string;
  tags?: string[];
  accentColor: string;
  dotColor: string;
}

const nodes: McpNode[] = [
  {
    label: 'AI 智能体',
    sub: '增长运营助手',
    accentColor: '#2563eb',
    dotColor: '#3b82f6',
  },
  {
    label: 'MCP 协议网关',
    sub: '统一调度路由',
    tags: ['工具策略', '风险审查'],
    accentColor: '#7c3aed',
    dotColor: '#a78bfa',
  },
  {
    label: '增长核心服务',
    sub: '活动 · 积分 · 用户',
    accentColor: '#059669',
    dotColor: '#34d399',
  },
];

const arrowColors = ['#2563eb', '#7c3aed'];

export const McpArchitectureDiagram: React.FC<{ className?: string }> = () => {
  return (
    <div className="flex items-stretch justify-between gap-0 w-full">
      {nodes.map((node, i) => (
        <React.Fragment key={node.label}>
          <div
            className="flex-1 flex flex-col justify-between p-4 border"
            style={{ borderColor: node.accentColor + '40', background: node.accentColor + '08' }}
          >
            <div>
              <div
                className="text-xs font-semibold leading-snug mb-1"
                style={{ color: node.accentColor }}
              >
                {node.label}
              </div>
              <div className="text-[11px] text-stone-400 dark:text-stone-500">{node.sub}</div>
            </div>
            {node.tags && (
              <div className="flex flex-col gap-1 mt-3">
                {node.tags.map((t) => (
                  <span
                    key={t}
                    className="text-[10px] font-mono px-2 py-0.5 inline-block"
                    style={{ background: node.accentColor + '20', color: node.dotColor }}
                  >
                    {t}
                  </span>
                ))}
              </div>
            )}
            <div
              className="w-2 h-2 rounded-full mt-3"
              style={{ background: node.dotColor }}
            />
          </div>
          {i < nodes.length - 1 && <ArrowRight color={arrowColors[i]} />}
        </React.Fragment>
      ))}
    </div>
  );
};
