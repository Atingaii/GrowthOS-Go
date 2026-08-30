import {
  Activity,
  BarChart3,
  CheckSquare,
  Coins,
  FlaskConical,
  Gift,
  LayoutDashboard,
  Percent,
  Rss,
  ShieldAlert,
  Target,
  Ticket,
  Trophy,
  Users,
} from "lucide-react";
import {
  WorkspaceShell,
  type WorkspaceNavigationSection,
} from "../components/layout/WorkspaceShell";

const adminNavigation: WorkspaceNavigationSection[] = [
  {
    label: "核心增长引擎",
    items: [
      {
        label: "运营仪表盘",
        path: "/admin/dashboard",
        icon: LayoutDashboard,
        matchPaths: ["/admin"],
      },
      { label: "活动策略", path: "/admin/campaigns", icon: Gift },
      { label: "抽奖矩阵", path: "/admin/strategies", icon: Target },
      { label: "奖品库", path: "/admin/awards", icon: Trophy },
    ],
  },
  {
    label: "权益与账户",
    items: [
      { label: "用户账户", path: "/admin/accounts", icon: Users },
      { label: "积分策略", path: "/admin/points", icon: Coins },
      { label: "优惠券发放", path: "/admin/coupons", icon: Ticket },
      { label: "返利中心", path: "/admin/rebates", icon: Percent },
    ],
  },
  {
    label: "数据与实验",
    items: [
      { label: "Growth Feed", path: "/admin/feed", icon: Rss },
      { label: "行为采集", path: "/admin/behavior", icon: Activity },
      { label: "A/B 实验", path: "/admin/experiments", icon: FlaskConical },
      { label: "漏斗分析", path: "/admin/analytics", icon: BarChart3 },
    ],
  },
  {
    label: "任务与审计",
    items: [
      { label: "定时任务", path: "/admin/tasks", icon: CheckSquare },
      { label: "审计日志", path: "/admin/audit", icon: ShieldAlert },
    ],
  },
];

export function AdminLayout() {
  return (
    <WorkspaceShell
      navigation={adminNavigation}
      productLabel="运营后台"
      primaryAction={{ label: "打开活动策略", path: "/admin/campaigns" }}
      footerLabel="GrowthOS Operator v1.0.0"
    />
  );
}
