import { Bot, CheckCircle2, History, ListTodo } from "lucide-react";
import {
  WorkspaceShell,
  type WorkspaceNavigationSection,
} from "../components/layout/WorkspaceShell";

const agentNavigation: WorkspaceNavigationSection[] = [
  {
    label: "AI Operator",
    items: [
      { label: "AI 工作台", path: "/agent/workspace", icon: Bot, matchPaths: ["/agent"] },
      { label: "任务队列", path: "/agent/tasks", icon: ListTodo },
      { label: "人工审批中心", path: "/agent/approvals", icon: CheckCircle2 },
      { label: "执行历史", path: "/agent/history", icon: History },
    ],
  },
];

export function AgentLayout() {
  return (
    <WorkspaceShell
      navigation={agentNavigation}
      productLabel="AI Operator"
      primaryAction={{ label: "查看任务队列", path: "/agent/tasks" }}
      footerLabel="GrowthOS Agent Workspace v1.0.0"
    />
  );
}
