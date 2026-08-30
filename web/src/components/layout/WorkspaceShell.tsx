import { useEffect, useMemo, useRef, useState, type ComponentType, type SVGProps } from "react";
import { Link, Outlet, useLocation } from "react-router";
import {
  Bell,
  Bot,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Command,
  Expand,
  Home,
  Menu,
  Minimize2,
  Moon,
  Plus,
  Search,
  Settings,
  Shield,
  Sun,
  Terminal,
  X,
} from "lucide-react";
import { GrowthOSLogo } from "../common/GrowthOSGraphics";
import { useAppStore } from "../../stores/appStore";

type NavigationIcon = ComponentType<SVGProps<SVGSVGElement>>;

export interface WorkspaceNavigationItem {
  label: string;
  path: string;
  icon: NavigationIcon;
  badge?: string;
  exact?: boolean;
  matchPaths?: string[];
}

export interface WorkspaceNavigationSection {
  label?: string;
  items: WorkspaceNavigationItem[];
}

interface WorkspaceShellProps {
  navigation: WorkspaceNavigationSection[];
  productLabel: string;
  primaryAction?: {
    label: string;
    path: string;
  };
  footerLabel?: string;
}

const workspaceSwitches: WorkspaceNavigationItem[] = [
  { label: "用户工作台", path: "/home", icon: Home },
  { label: "运营后台", path: "/admin/dashboard", icon: Shield },
  { label: "MCP 网关", path: "/mcp/dashboard", icon: Terminal },
  { label: "AI Agent", path: "/agent/workspace", icon: Bot },
];

function isNavigationItemActive(pathname: string, item: WorkspaceNavigationItem) {
  if (item.matchPaths?.some((path) => pathname === path || pathname.startsWith(`${path}/`))) {
    return true;
  }

  if (item.exact) {
    return pathname === item.path;
  }

  return pathname === item.path || pathname.startsWith(`${item.path}/`);
}

interface SidebarNavigationProps {
  collapsed: boolean;
  navigation: WorkspaceNavigationSection[];
  pathname: string;
  productLabel: string;
  footerLabel?: string;
  onNavigate?: () => void;
}

function SidebarNavigation({
  collapsed,
  navigation,
  pathname,
  productLabel,
  footerLabel,
  onNavigate,
}: SidebarNavigationProps) {
  const { user } = useAppStore();

  return (
    <div className="flex h-full min-h-0 flex-col">
      <Link
        to="/profile"
        onClick={onNavigate}
        className={`mx-2 mt-2 flex h-14 items-center rounded-lg px-2 transition-colors hover:bg-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:hover:bg-zinc-900 ${
          collapsed ? "justify-center" : "gap-2.5"
        }`}
        aria-label={`打开 ${user.name} 的个人中心`}
      >
        <img
          src={user.avatar}
          alt=""
          className="h-7 w-7 shrink-0 rounded-md object-cover ring-1 ring-zinc-200 dark:ring-zinc-800"
        />
        {!collapsed ? (
          <>
            <span className="min-w-0 flex-1 text-left">
              <span className="block truncate text-[13px] font-medium text-zinc-900 dark:text-zinc-100">
                {user.name}
              </span>
              <span className="mt-0.5 block truncate text-[11px] text-zinc-500 dark:text-zinc-500">
                演示成员 · {productLabel}
              </span>
            </span>
            <ChevronDown className="h-4 w-4 shrink-0 text-zinc-400" aria-hidden="true" />
          </>
        ) : null}
      </Link>

      <nav aria-label={`${productLabel} 导航`} className="min-h-0 flex-1 overflow-y-auto px-2 py-4">
        <div className="space-y-5">
          {navigation.map((section, sectionIndex) => (
            <section key={section.label ?? sectionIndex} aria-label={section.label}>
              {section.label && !collapsed ? (
                <h2 className="mb-1 px-2 text-[11px] font-normal text-zinc-400 dark:text-zinc-600">
                  {section.label}
                </h2>
              ) : null}
              <div className="space-y-0.5">
                {section.items.map((item) => {
                  const Icon = item.icon;
                  const active = isNavigationItemActive(pathname, item);

                  return (
                    <Link
                      key={item.path}
                      to={item.path}
                      onClick={onNavigate}
                      title={collapsed ? item.label : undefined}
                      aria-label={collapsed ? item.label : undefined}
                      aria-current={active ? "page" : undefined}
                      className={`group flex h-9 items-center rounded-md text-[13px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 ${
                        collapsed ? "justify-center px-0" : "gap-2.5 px-2"
                      } ${
                        active
                          ? "bg-violet-50 text-violet-600 dark:bg-violet-500/10 dark:text-violet-300"
                          : "text-zinc-600 hover:bg-zinc-100 hover:text-zinc-950 dark:text-zinc-400 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
                      }`}
                    >
                      <Icon
                        className={`h-[17px] w-[17px] shrink-0 ${
                          active
                            ? "text-violet-500"
                            : "text-zinc-500 group-hover:text-zinc-800 dark:group-hover:text-zinc-200"
                        }`}
                        aria-hidden="true"
                      />
                      {!collapsed ? (
                        <>
                          <span className="min-w-0 flex-1 truncate">{item.label}</span>
                          {item.badge ? (
                            <span className="inline-flex min-w-5 items-center justify-center rounded-full bg-violet-100 px-1.5 py-0.5 text-[10px] font-semibold text-violet-600 dark:bg-violet-500/15 dark:text-violet-300">
                              {item.badge}
                            </span>
                          ) : null}
                        </>
                      ) : null}
                    </Link>
                  );
                })}
              </div>
            </section>
          ))}
        </div>
      </nav>

      {!collapsed ? (
        <div className="mx-3 border-t border-zinc-200 py-4 text-[11px] leading-5 text-zinc-400 dark:border-zinc-800 dark:text-zinc-600">
          <div>{footerLabel ?? "GrowthOS Web v1.0.0"}</div>
          <div>Lesson 22 · Development Preview</div>
        </div>
      ) : (
        <div className="mx-3 border-t border-zinc-200 py-4 text-center text-[10px] font-semibold text-zinc-400 dark:border-zinc-800">
          GOS
        </div>
      )}
    </div>
  );
}

interface SearchPaletteProps {
  items: WorkspaceNavigationItem[];
  onClose: () => void;
}

function SearchPalette({ items, onClose }: SearchPaletteProps) {
  const [query, setQuery] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const filteredItems = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    if (!normalizedQuery) {
      return items;
    }

    return items.filter((item) =>
      `${item.label} ${item.path}`.toLocaleLowerCase().includes(normalizedQuery),
    );
  }, [items, query]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  return (
    <div
      className="fixed inset-0 z-[80] flex items-start justify-center bg-zinc-950/25 px-4 pt-[12vh] backdrop-blur-[2px]"
      role="presentation"
      onMouseDown={(event) => {
        if (event.currentTarget === event.target) {
          onClose();
        }
      }}
    >
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="workspace-search-title"
        className="w-full max-w-xl overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-2xl shadow-zinc-950/10 dark:border-zinc-800 dark:bg-zinc-950"
      >
        <h2 id="workspace-search-title" className="sr-only">
          搜索 GrowthOS 页面
        </h2>
        <div className="flex items-center gap-3 border-b border-zinc-200 px-4 dark:border-zinc-800">
          <Search className="h-4 w-4 shrink-0 text-zinc-400" aria-hidden="true" />
          <input
            ref={inputRef}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索页面与功能…"
            aria-label="搜索页面与功能"
            className="h-12 min-w-0 flex-1 bg-transparent text-sm text-zinc-900 outline-none placeholder:text-zinc-400 dark:text-zinc-100"
          />
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-8 w-8 items-center justify-center rounded-md text-zinc-400 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
            aria-label="关闭搜索"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
        <div className="max-h-[360px] overflow-y-auto p-2">
          {filteredItems.length ? (
            <div className="space-y-0.5">
              {filteredItems.map((item) => {
                const Icon = item.icon;
                return (
                  <Link
                    key={`${item.path}-${item.label}`}
                    to={item.path}
                    onClick={onClose}
                    className="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-zinc-700 transition-colors hover:bg-zinc-100 hover:text-zinc-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:text-zinc-300 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
                  >
                    <span className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-zinc-200 bg-zinc-50 text-zinc-500 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-400">
                      <Icon className="h-4 w-4" aria-hidden="true" />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block font-medium">{item.label}</span>
                      <span className="block truncate text-[11px] text-zinc-400">{item.path}</span>
                    </span>
                    <Command
                      className="h-3.5 w-3.5 text-zinc-300 dark:text-zinc-700"
                      aria-hidden="true"
                    />
                  </Link>
                );
              })}
            </div>
          ) : (
            <div className="px-4 py-10 text-center text-sm text-zinc-400">没有匹配的页面</div>
          )}
        </div>
      </section>
    </div>
  );
}

export function WorkspaceShell({
  navigation,
  productLabel,
  primaryAction = { label: "进入活动中心", path: "/campaigns" },
  footerLabel,
}: WorkspaceShellProps) {
  const location = useLocation();
  const { sidebarCollapsed, toggleSidebar, theme, toggleTheme } = useAppStore();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [fullWidth, setFullWidth] = useState(false);
  const mobileMenuButtonRef = useRef<HTMLButtonElement>(null);
  const mobileCloseButtonRef = useRef<HTMLButtonElement>(null);
  const mobileWasOpen = useRef(false);
  const searchableItems = useMemo(() => {
    const uniqueItems = new Map<string, WorkspaceNavigationItem>();
    for (const item of [...navigation.flatMap((section) => section.items), ...workspaceSwitches]) {
      uniqueItems.set(`${item.path}-${item.label}`, item);
    }
    return [...uniqueItems.values()];
  }, [navigation]);

  useEffect(() => {
    setMobileOpen(false);
    setSearchOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === "k") {
        event.preventDefault();
        setSearchOpen((open) => !open);
      }
      if (event.key === "Escape") {
        setSearchOpen(false);
        setMobileOpen(false);
      }
    };
    window.addEventListener("keydown", handleShortcut);
    return () => window.removeEventListener("keydown", handleShortcut);
  }, []);

  useEffect(() => {
    if (mobileOpen) {
      mobileWasOpen.current = true;
      const previousOverflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
      mobileCloseButtonRef.current?.focus();
      return () => {
        document.body.style.overflow = previousOverflow;
      };
    }

    if (mobileWasOpen.current) {
      mobileWasOpen.current = false;
      mobileMenuButtonRef.current?.focus();
    }
  }, [mobileOpen]);

  return (
    <div className="min-h-screen bg-white text-zinc-950 dark:bg-zinc-950 dark:text-zinc-50">
      <a
        href="#main-content"
        className="sr-only fixed left-4 top-3 z-[100] rounded-md bg-zinc-950 px-3 py-2 text-sm font-medium text-white focus:not-sr-only focus:outline-none focus:ring-2 focus:ring-violet-400 dark:bg-white dark:text-zinc-950"
      >
        跳到主要内容
      </a>
      <aside
        id="desktop-workspace-navigation"
        className={`fixed inset-y-0 left-0 z-50 hidden border-r border-zinc-200 bg-zinc-50/70 transition-[width] duration-200 motion-reduce:transition-none dark:border-zinc-800 dark:bg-zinc-950 md:block ${
          sidebarCollapsed ? "w-16" : "w-[231px]"
        }`}
      >
        <SidebarNavigation
          collapsed={sidebarCollapsed}
          navigation={navigation}
          pathname={location.pathname}
          productLabel={productLabel}
          footerLabel={footerLabel}
        />
        <button
          type="button"
          onClick={toggleSidebar}
          aria-label={sidebarCollapsed ? "展开侧边栏" : "收起侧边栏"}
          aria-expanded={!sidebarCollapsed}
          aria-controls="desktop-workspace-navigation"
          className="absolute -right-3 top-1/2 inline-flex h-8 w-6 -translate-y-1/2 items-center justify-center rounded-md border border-zinc-200 bg-white text-zinc-400 shadow-sm hover:text-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:border-zinc-800 dark:bg-zinc-950 dark:hover:text-zinc-100"
        >
          {sidebarCollapsed ? (
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          ) : (
            <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          )}
        </button>
      </aside>

      {mobileOpen ? (
        <div
          className="fixed inset-0 z-[70] bg-zinc-950/25 backdrop-blur-[2px] md:hidden"
          role="presentation"
          onMouseDown={(event) => {
            if (event.currentTarget === event.target) {
              setMobileOpen(false);
            }
          }}
        >
          <aside
            id="mobile-workspace-navigation"
            role="dialog"
            aria-modal="true"
            aria-label={`${productLabel} 移动导航`}
            className="h-full w-[min(84vw,292px)] border-r border-zinc-200 bg-white shadow-2xl dark:border-zinc-800 dark:bg-zinc-950"
          >
            <div className="absolute left-[min(84vw,292px)] top-3 ml-3">
              <button
                ref={mobileCloseButtonRef}
                type="button"
                onClick={() => setMobileOpen(false)}
                aria-label="关闭导航"
                className="inline-flex h-10 w-10 items-center justify-center rounded-full bg-white text-zinc-600 shadow-lg dark:bg-zinc-900 dark:text-zinc-200"
              >
                <X className="h-5 w-5" aria-hidden="true" />
              </button>
            </div>
            <SidebarNavigation
              collapsed={false}
              navigation={navigation}
              pathname={location.pathname}
              productLabel={productLabel}
              footerLabel={footerLabel}
              onNavigate={() => setMobileOpen(false)}
            />
          </aside>
        </div>
      ) : null}

      <div
        className={`min-h-screen transition-[padding] duration-200 motion-reduce:transition-none ${
          sidebarCollapsed ? "md:pl-16" : "md:pl-[231px]"
        }`}
      >
        <header className="sticky top-0 z-40 h-[72px] border-b border-zinc-100 bg-white/95 backdrop-blur-md dark:border-zinc-900 dark:bg-zinc-950/95">
          <div
            className={`mx-auto flex h-full items-center gap-3 px-4 transition-[max-width,padding] sm:px-6 md:px-8 lg:px-12 ${
              fullWidth ? "max-w-none" : "max-w-[1320px]"
            }`}
          >
            <button
              ref={mobileMenuButtonRef}
              type="button"
              onClick={() => setMobileOpen(true)}
              aria-label="打开导航"
              aria-expanded={mobileOpen}
              aria-controls="mobile-workspace-navigation"
              className="inline-flex h-9 w-9 items-center justify-center rounded-md text-zinc-500 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:hover:bg-zinc-900 dark:hover:text-zinc-100 md:hidden"
            >
              <Menu className="h-[18px] w-[18px]" aria-hidden="true" />
            </button>

            <Link to="/home" className="shrink-0 md:hidden" aria-label="GrowthOS 首页">
              <GrowthOSLogo />
            </Link>

            <button
              type="button"
              onClick={() => setSearchOpen(true)}
              className="hidden h-8 w-64 items-center gap-2.5 rounded-lg border border-zinc-200 bg-zinc-50 px-3 text-left text-sm text-zinc-500 transition-colors hover:border-zinc-300 hover:bg-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:border-zinc-800 dark:bg-zinc-900/70 dark:text-zinc-400 dark:hover:border-zinc-700 dark:hover:bg-zinc-900 md:flex"
              aria-label="搜索页面与功能"
            >
              <Search className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span className="flex-1">搜索</span>
              <kbd className="rounded border border-zinc-200 bg-white px-1.5 py-0.5 font-sans text-[10px] text-zinc-400 shadow-sm dark:border-zinc-700 dark:bg-zinc-950">
                ⌘K
              </kbd>
            </button>

            <div className="ml-auto flex items-center gap-0.5">
              <button
                type="button"
                onClick={() => setSearchOpen(true)}
                aria-label="搜索页面与功能"
                className="inline-flex h-9 w-9 items-center justify-center rounded-md text-zinc-500 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:hover:bg-zinc-900 dark:hover:text-zinc-100 md:hidden"
              >
                <Search className="h-[18px] w-[18px]" aria-hidden="true" />
              </button>
              <button
                type="button"
                aria-label="查看通知（有未读）"
                className="relative inline-flex h-10 w-10 items-center justify-center rounded-md text-zinc-500 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
              >
                <Bell className="h-[18px] w-[18px]" aria-hidden="true" />
                <span
                  className="absolute right-2 top-1.5 h-1.5 w-1.5 rounded-full bg-rose-500"
                  aria-hidden="true"
                />
              </button>
              <Link
                to="/profile"
                aria-label="打开设置"
                className="inline-flex h-10 w-10 items-center justify-center rounded-md text-zinc-500 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
              >
                <Settings className="h-[18px] w-[18px]" aria-hidden="true" />
              </Link>
              <Link
                to={primaryAction.path}
                aria-label={primaryAction.label}
                className="mx-1 inline-flex h-8 w-8 items-center justify-center rounded-full bg-violet-600 text-white shadow-sm transition-colors hover:bg-violet-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/40 focus-visible:ring-offset-2 dark:ring-offset-zinc-950"
              >
                <Plus className="h-4 w-4" aria-hidden="true" />
              </Link>
              <button
                type="button"
                onClick={() => setFullWidth((current) => !current)}
                aria-label={fullWidth ? "恢复内容宽度" : "切换全宽内容"}
                aria-pressed={fullWidth}
                className="hidden h-10 w-10 items-center justify-center rounded-md text-zinc-500 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:hover:bg-zinc-900 dark:hover:text-zinc-100 sm:inline-flex"
              >
                {fullWidth ? (
                  <Minimize2 className="h-[18px] w-[18px]" aria-hidden="true" />
                ) : (
                  <Expand className="h-[18px] w-[18px]" aria-hidden="true" />
                )}
              </button>
              <button
                type="button"
                onClick={toggleTheme}
                aria-label={theme === "light" ? "切换至暗色主题" : "切换至亮色主题"}
                aria-pressed={theme === "dark"}
                className="inline-flex h-10 w-10 items-center justify-center rounded-md text-zinc-500 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/35 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
              >
                {theme === "light" ? (
                  <Moon className="h-[18px] w-[18px]" aria-hidden="true" />
                ) : (
                  <Sun className="h-[18px] w-[18px]" aria-hidden="true" />
                )}
              </button>
            </div>
          </div>
        </header>

        <main
          id="main-content"
          tabIndex={-1}
          className={`mx-auto w-full min-w-0 px-4 py-6 transition-[max-width,padding] sm:px-6 md:px-8 lg:px-12 ${
            fullWidth ? "max-w-none" : "max-w-[1320px]"
          }`}
        >
          <Outlet />
        </main>
      </div>

      {searchOpen ? (
        <SearchPalette items={searchableItems} onClose={() => setSearchOpen(false)} />
      ) : null}
    </div>
  );
}
