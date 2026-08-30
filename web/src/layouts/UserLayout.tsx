import React from "react";
import { Link, Outlet, useLocation } from "react-router";
import {
  Home,
  Flame,
  Gift,
  Coins,
  Ticket,
  Trophy,
  User,
  Sun,
  Moon,
  Shield,
  Bot,
  Terminal,
  Bell,
  Activity,
} from "lucide-react";
import { GrowthOSLogo } from "../components/common/GrowthOSGraphics";
import { useAppStore } from "../stores/appStore";

export const UserLayout: React.FC = () => {
  const location = useLocation();
  const { user, theme, toggleTheme } = useAppStore();

  const navItems = [
    { label: "首页", path: "/home", icon: Home },
    { label: "Growth Feed", path: "/feed", icon: Flame },
    { label: "营销活动", path: "/campaigns", icon: Gift },
    { label: "幸运抽奖", path: "/lottery", icon: Trophy },
    { label: "积分中心", path: "/points", icon: Coins },
    { label: "优惠券", path: "/coupons", icon: Ticket },
    { label: "个人中心", path: "/profile", icon: User },
  ];

  return (
    <div className="min-h-screen bg-stone-100 dark:bg-[#0a0a0a] text-stone-900 dark:text-stone-100 flex flex-col font-sans">
      {/* Top Bar Header */}
      <header className="sticky top-0 z-40 bg-white dark:bg-[#141414] border-b border-stone-200 dark:border-neutral-800">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-14 flex items-center justify-between">
          <div className="flex items-center gap-8">
            <Link to="/home">
              <GrowthOSLogo />
            </Link>
            {/* Desktop Navigation */}
            <nav className="hidden md:flex items-center">
              {navItems.map((item) => {
                const Icon = item.icon;
                const isActive = location.pathname.startsWith(item.path);
                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    aria-current={isActive ? "page" : undefined}
                    className={`flex items-center gap-1.5 px-3 h-14 text-sm font-medium border-b-2 transition-colors ${
                      isActive
                        ? "border-blue-600 dark:border-blue-400 text-blue-600 dark:text-blue-400"
                        : "border-transparent text-stone-500 dark:text-stone-400 hover:text-stone-900 dark:hover:text-stone-100"
                    }`}
                  >
                    <Icon className="w-3.5 h-3.5" aria-hidden="true" />
                    {item.label}
                  </Link>
                );
              })}
            </nav>
          </div>

          {/* Right Header Actions */}
          <div className="flex items-center gap-1">
            {/* Quick Layout Switchers */}
            <div className="hidden lg:flex items-center gap-0.5 mr-2">
              <Link
                to="/admin/dashboard"
                className="px-2 py-1 text-xs text-stone-400 dark:text-stone-500 hover:text-stone-700 dark:hover:text-stone-300 flex items-center gap-1 rounded font-mono"
              >
                <Shield className="w-3 h-3" /> Admin
              </Link>
              <Link
                to="/mcp/dashboard"
                className="px-2 py-1 text-xs text-stone-400 dark:text-stone-500 hover:text-stone-700 dark:hover:text-stone-300 flex items-center gap-1 rounded font-mono"
              >
                <Terminal className="w-3 h-3" /> MCP
              </Link>
              <Link
                to="/agent/workspace"
                className="px-2 py-1 text-xs text-stone-400 dark:text-stone-500 hover:text-stone-700 dark:hover:text-stone-300 flex items-center gap-1 rounded font-mono"
              >
                <Bot className="w-3 h-3" /> Agent
              </Link>
              <Link
                to="/system/status"
                className="px-2 py-1 text-xs text-stone-400 dark:text-stone-500 hover:text-stone-700 dark:hover:text-stone-300 flex items-center gap-1 rounded font-mono"
              >
                <Activity className="w-3 h-3" /> Status
              </Link>
            </div>

            {/* Notifications */}
            <button
              type="button"
              aria-label="查看通知（有未读）"
              className="relative inline-flex min-h-11 min-w-11 items-center justify-center rounded text-stone-400 hover:bg-stone-100 hover:text-stone-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:hover:bg-neutral-800 dark:hover:text-stone-200"
            >
              <Bell className="w-4.5 h-4.5" aria-hidden="true" />
              <span
                className="absolute top-1.5 right-1.5 w-1.5 h-1.5 bg-rose-500 rounded-full"
                aria-hidden="true"
              />
            </button>

            {/* Theme Toggle */}
            <button
              type="button"
              onClick={toggleTheme}
              aria-label={theme === "light" ? "切换至暗色主题" : "切换至亮色主题"}
              aria-pressed={theme === "dark"}
              className="inline-flex min-h-11 min-w-11 items-center justify-center rounded text-stone-400 transition-colors hover:bg-stone-100 hover:text-stone-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40 dark:hover:bg-neutral-800 dark:hover:text-stone-200"
            >
              {theme === "light" ? (
                <Moon className="w-4 h-4" aria-hidden="true" />
              ) : (
                <Sun className="w-4 h-4" aria-hidden="true" />
              )}
            </button>

            {/* User Profile */}
            <Link
              to="/profile"
              className="flex items-center gap-2 ml-1 pl-3 border-l border-stone-200 dark:border-neutral-700"
            >
              <img
                src={user.avatar}
                alt={user.name}
                className="w-7 h-7 rounded-full object-cover"
              />
              <div className="hidden xl:block text-left">
                <div className="text-xs font-semibold leading-none text-stone-800 dark:text-stone-200">
                  {user.name}
                </div>
                <div className="text-[10px] text-blue-600 dark:text-blue-400 font-mono mt-0.5">
                  {user.points.toLocaleString()} pts
                </div>
              </div>
            </Link>
          </div>
        </div>
      </header>

      {/* Main Content View */}
      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 pb-20 md:pb-8">
        <Outlet />
      </main>

      {/* Mobile Tab Bar */}
      <nav className="md:hidden fixed bottom-0 left-0 right-0 z-40 bg-white dark:bg-[#141414] border-t border-stone-200 dark:border-neutral-800 flex justify-around py-2 px-1">
        {navItems.slice(0, 5).map((item) => {
          const Icon = item.icon;
          const isActive = location.pathname.startsWith(item.path);
          return (
            <Link
              key={item.path}
              to={item.path}
              aria-current={isActive ? "page" : undefined}
              className={`flex flex-col items-center gap-0.5 py-1 px-3 ${
                isActive ? "text-blue-600 dark:text-blue-400" : "text-stone-400 dark:text-stone-500"
              }`}
            >
              <Icon className="w-5 h-5" aria-hidden="true" />
              <span className="text-[10px] font-medium">{item.label}</span>
            </Link>
          );
        })}
      </nav>
    </div>
  );
};
