import { FileCode, Server, ShieldCheck, Terminal, Wrench } from "lucide-react";
import {
  WorkspaceShell,
  type WorkspaceNavigationSection,
} from "../components/layout/WorkspaceShell";

const mcpNavigation: WorkspaceNavigationSection[] = [
  {
    label: "MCP Gateway",
    items: [
      { label: "网关概览", path: "/mcp/dashboard", icon: Terminal, matchPaths: ["/mcp"] },
      { label: "MCP 服务节点", path: "/mcp/servers", icon: Server },
      { label: "Tool 工具库", path: "/mcp/tools", icon: Wrench },
      { label: "安全与权限", path: "/mcp/permissions", icon: ShieldCheck },
      { label: "调用审计", path: "/mcp/audit", icon: FileCode },
    ],
  },
];

export function McpLayout() {
  return (
    <WorkspaceShell
      navigation={mcpNavigation}
      productLabel="MCP Gateway"
      primaryAction={{ label: "打开 Tool 工具库", path: "/mcp/tools" }}
      footerLabel="GrowthOS MCP Gateway v2.1"
    />
  );
}
