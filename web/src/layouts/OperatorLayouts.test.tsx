// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router";
import { useAppStore } from "../stores/appStore";
import { AdminLayout } from "./AdminLayout";
import { AgentLayout } from "./AgentLayout";
import { McpLayout } from "./McpLayout";

interface LayoutCase {
  basePath: string;
  initialPath: string;
  layout: React.ReactNode;
  navigationName: string;
  activeLink: string;
  primaryAction: string;
  primaryActionPath: string;
}

function renderLayout({ basePath, initialPath, layout }: LayoutCase) {
  const router = createMemoryRouter(
    [
      {
        path: basePath,
        element: layout,
        children: [
          { index: true, element: <div>Operator route content</div> },
          { path: "*", element: <div>Operator route content</div> },
        ],
      },
      { path: "/profile", element: <div>Profile route</div> },
    ],
    { initialEntries: [initialPath] },
  );

  return render(<RouterProvider router={router} />);
}

const layoutCases: LayoutCase[] = [
  {
    basePath: "/admin",
    initialPath: "/admin/dashboard",
    layout: <AdminLayout />,
    navigationName: "运营后台 导航",
    activeLink: "运营仪表盘",
    primaryAction: "打开活动策略",
    primaryActionPath: "/admin/campaigns",
  },
  {
    basePath: "/mcp",
    initialPath: "/mcp/dashboard",
    layout: <McpLayout />,
    navigationName: "MCP Gateway 导航",
    activeLink: "网关概览",
    primaryAction: "打开 Tool 工具库",
    primaryActionPath: "/mcp/tools",
  },
  {
    basePath: "/agent",
    initialPath: "/agent/workspace",
    layout: <AgentLayout />,
    navigationName: "AI Operator 导航",
    activeLink: "AI 工作台",
    primaryAction: "查看任务队列",
    primaryActionPath: "/agent/tasks",
  },
];

beforeEach(() => {
  document.documentElement.classList.remove("dark");
  document.body.style.overflow = "";
  useAppStore.setState({ theme: "light", sidebarCollapsed: false });
});

afterEach(() => {
  cleanup();
  document.documentElement.classList.remove("dark");
  document.body.style.overflow = "";
  useAppStore.setState({ theme: "light", sidebarCollapsed: false });
});

describe.each(layoutCases)("$navigationName", (layoutCase) => {
  it("keeps the shared workspace landmarks and canonical route state accessible", () => {
    const { container } = renderLayout(layoutCase);

    expect(container.querySelectorAll("main")).toHaveLength(1);
    expect(screen.getByRole("main").id).toBe("main-content");
    expect(screen.getByRole("link", { name: "跳到主要内容" }).getAttribute("href")).toBe(
      "#main-content",
    );
    expect(screen.getByRole("navigation", { name: layoutCase.navigationName })).toBeTruthy();
    expect(
      screen.getByRole("link", { name: layoutCase.activeLink }).getAttribute("aria-current"),
    ).toBe("page");
  });

  it("routes the named primary action without pretending to complete a write", () => {
    renderLayout(layoutCase);

    const action = screen.getByRole("link", { name: layoutCase.primaryAction });
    expect(action.getAttribute("href")).toBe(layoutCase.primaryActionPath);
  });

  it("marks the dashboard item active on the layout index route", () => {
    renderLayout({ ...layoutCase, initialPath: layoutCase.basePath });

    expect(
      screen.getByRole("link", { name: layoutCase.activeLink }).getAttribute("aria-current"),
    ).toBe("page");
  });
});
