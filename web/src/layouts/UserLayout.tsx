import {
  Activity,
  Bot,
  Coins,
  Flame,
  Gift,
  Home,
  Shield,
  Terminal,
  Ticket,
  Trophy,
  User,
} from "lucide-react";
import {
  WorkspaceShell,
  type WorkspaceNavigationSection,
} from "../components/layout/WorkspaceShell";

const userNavigation: WorkspaceNavigationSection[] = [
  {
    items: [
      { label: "首页", path: "/home", matchPaths: ["/"], icon: Home },
      { label: "Growth Feed", path: "/feed", icon: Flame },
      { label: "营销活动", path: "/campaigns", icon: Gift },
      { label: "幸运抽奖", path: "/lottery", icon: Trophy },
      { label: "积分中心", path: "/points", matchPaths: ["/rewards"], icon: Coins },
      { label: "优惠券", path: "/coupons", icon: Ticket },
    ],
  },
  {
    label: "账户",
    items: [{ label: "个人中心", path: "/profile", icon: User }],
  },
  {
    label: "工作空间",
    items: [
      { label: "运营后台", path: "/admin/dashboard", icon: Shield },
      { label: "MCP 网关", path: "/mcp/dashboard", icon: Terminal },
      { label: "AI Agent", path: "/agent/workspace", icon: Bot },
      { label: "系统状态", path: "/system/status", icon: Activity },
    ],
  },
];

export function UserLayout() {
  return (
    <WorkspaceShell
      navigation={userNavigation}
      productLabel="Growth Platform"
      primaryAction={{ label: "进入活动中心", path: "/campaigns" }}
    />
  );
}
