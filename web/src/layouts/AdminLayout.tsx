import React from 'react';
import { Link, Outlet, useLocation } from 'react-router';
import {
  LayoutDashboard,
  Gift,
  Target,
  Trophy,
  Users,
  Coins,
  Ticket,
  Percent,
  Rss,
  Activity,
  FlaskConical,
  BarChart3,
  CheckSquare,
  ShieldAlert,
  ChevronLeft,
  ChevronRight,
  Search,
  Sun,
  Moon,
  Home,
  Bot,
  Terminal,
} from 'lucide-react';
import { GrowthOSLogo } from '../components/common/GrowthOSGraphics';
import { useAppStore } from '../stores/appStore';

export const AdminLayout: React.FC = () => {
  const location = useLocation();
  const { sidebarCollapsed, toggleSidebar, theme, toggleTheme } = useAppStore();

  const menuSections = [
    {
      title: '核心增长引擎',
      items: [
        { label: '运营仪表盘', path: '/admin/dashboard', icon: LayoutDashboard },
        { label: '活动策略', path: '/admin/campaigns', icon: Gift },
        { label: '抽奖矩阵', path: '/admin/strategies', icon: Target },
        { label: '奖品库', path: '/admin/awards', icon: Trophy },
      ],
    },
    {
      title: '权益与账户',
      items: [
        { label: '用户账户', path: '/admin/accounts', icon: Users },
        { label: '积分策略', path: '/admin/points', icon: Coins },
        { label: '优惠券发放', path: '/admin/coupons', icon: Ticket },
        { label: '返利中心', path: '/admin/rebates', icon: Percent },
      ],
    },
    {
      title: '数据与实验',
      items: [
        { label: 'Growth Feed', path: '/admin/feed', icon: Rss },
        { label: '行为采集', path: '/admin/behavior', icon: Activity },
        { label: 'A/B 实验', path: '/admin/experiments', icon: FlaskConical },
        { label: '深度漏斗分析', path: '/admin/analytics', icon: BarChart3 },
      ],
    },
    {
      title: '任务与审计',
      items: [
        { label: '定时任务', path: '/admin/tasks', icon: CheckSquare },
        { label: '审计日志', path: '/admin/audit', icon: ShieldAlert },
      ],
    },
  ];

  return (
    <div className="min-h-screen bg-stone-100 dark:bg-[#0a0a0a] text-stone-900 dark:text-stone-100 flex font-sans">
      {/* Sidebar */}
      <aside
        className={`bg-white dark:bg-[#141414] border-r border-stone-200 dark:border-neutral-800 transition-all duration-200 flex flex-col z-30 sticky top-0 h-screen ${
          sidebarCollapsed ? 'w-[60px]' : 'w-56'
        }`}
      >
        {/* Sidebar Header */}
        <div className="h-14 px-4 flex items-center justify-between border-b border-stone-200 dark:border-neutral-800">
          <Link to="/admin/dashboard" className="flex items-center gap-2 overflow-hidden">
            <GrowthOSLogo iconOnly={sidebarCollapsed} />
          </Link>
          <button
            onClick={toggleSidebar}
            className="p-1 text-stone-400 hover:text-stone-600 dark:hover:text-stone-300 transition-colors"
          >
            {sidebarCollapsed ? <ChevronRight className="w-4 h-4" /> : <ChevronLeft className="w-4 h-4" />}
          </button>
        </div>

        {/* Sidebar Navigation */}
        <div className="flex-1 overflow-y-auto py-3 space-y-4">
          {menuSections.map((section, idx) => (
            <div key={idx}>
              {!sidebarCollapsed && (
                <div className="px-4 text-[10px] font-mono uppercase tracking-widest text-stone-400 dark:text-stone-600 mb-1">
                  {section.title}
                </div>
              )}
              <div>
                {section.items.map((item) => {
                  const Icon = item.icon;
                  const isActive = location.pathname === item.path;
                  return (
                    <Link
                      key={item.path}
                      to={item.path}
                      title={sidebarCollapsed ? item.label : undefined}
                      className={`flex items-center gap-3 px-4 py-2 text-xs font-medium transition-colors border-l-2 ${
                        isActive
                          ? 'border-blue-600 bg-blue-50 dark:bg-blue-950/30 text-blue-600 dark:text-blue-400'
                          : 'border-transparent text-stone-500 dark:text-stone-400 hover:text-stone-900 dark:hover:text-stone-100 hover:bg-stone-50 dark:hover:bg-neutral-800/60'
                      }`}
                    >
                      <Icon className="w-3.5 h-3.5 shrink-0" />
                      {!sidebarCollapsed && <span>{item.label}</span>}
                    </Link>
                  );
                })}
              </div>
            </div>
          ))}
        </div>

        {/* Sidebar Footer */}
        <div className="p-3 border-t border-stone-200 dark:border-neutral-800">
          {!sidebarCollapsed ? (
            <div className="flex items-center justify-between text-[10px] font-mono text-stone-400">
              <Link to="/home" className="hover:text-stone-700 dark:hover:text-stone-200 flex items-center gap-1">
                <Home className="w-3 h-3" /> User
              </Link>
              <Link to="/mcp/dashboard" className="hover:text-stone-700 dark:hover:text-stone-200 flex items-center gap-1">
                <Terminal className="w-3 h-3" /> MCP
              </Link>
              <Link to="/agent/workspace" className="hover:text-stone-700 dark:hover:text-stone-200 flex items-center gap-1">
                <Bot className="w-3 h-3" /> Agent
              </Link>
            </div>
          ) : (
            <Link to="/home" className="flex justify-center text-stone-400 hover:text-stone-600 dark:hover:text-stone-300">
              <Home className="w-4 h-4" />
            </Link>
          )}
        </div>
      </aside>

      {/* Main Workspace Area */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Top Header */}
        <header className="h-14 bg-white dark:bg-[#141414] border-b border-stone-200 dark:border-neutral-800 px-6 flex items-center justify-between sticky top-0 z-20">
          <div className="flex items-center gap-4 flex-1 max-w-sm">
            <div className="relative w-full">
              <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-stone-400" />
              <input
                type="text"
                placeholder="搜索活动、用户 ID、工具..."
                className="w-full pl-8 pr-4 py-1.5 border border-stone-200 dark:border-neutral-700 bg-stone-50 dark:bg-neutral-800 text-xs focus:outline-none focus:ring-1 focus:ring-blue-600 text-stone-800 dark:text-stone-200 placeholder-stone-400"
              />
            </div>
          </div>

          <div className="flex items-center gap-3">
            <span className="hidden sm:inline-flex items-center gap-1.5 text-xs font-mono text-emerald-600 dark:text-emerald-400">
              <span className="w-1.5 h-1.5 bg-emerald-500 rounded-full animate-pulse" /> 服务运行中
            </span>
            <button
              onClick={toggleTheme}
              className="p-2 text-stone-400 hover:text-stone-700 dark:hover:text-stone-200 hover:bg-stone-100 dark:hover:bg-neutral-800 rounded transition-colors"
            >
              {theme === 'light' ? <Moon className="w-4 h-4" /> : <Sun className="w-4 h-4" />}
            </button>
            <span className="text-xs font-mono text-stone-400 dark:text-stone-500 border-l border-stone-200 dark:border-neutral-700 pl-3">Admin</span>
          </div>
        </header>

        {/* Content View */}
        <main className="flex-1 p-6 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
};
