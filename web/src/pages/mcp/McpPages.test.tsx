// @vitest-environment jsdom

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Server } from "lucide-react";
import { GenericMcpPage } from "./GenericMcpPage";
import { McpDashboardPage } from "./McpDashboardPage";

afterEach(cleanup);

describe("MCP operator pages", () => {
  it("presents gateway health as an explicit local snapshot", () => {
    const { container } = render(<McpDashboardPage />);

    expect(container.querySelectorAll("h1")).toHaveLength(1);
    expect(screen.getByRole("heading", { level: 1, name: "AI 工具调度控制台" })).toBeTruthy();
    expect(screen.getByText("演示快照")).toBeTruthy();
    expect(screen.getByText(/尚未连接实时网关观测流/)).toBeTruthy();

    const metrics = screen.getByRole("region", { name: "MCP 指标摘要" });
    expect(within(metrics).getByText("演示节点")).toBeTruthy();
    expect(within(metrics).getByText("本地工具目录")).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2, name: "MCP Server 节点" })).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2, name: "Tool 风险与成功率" })).toBeTruthy();
    expect(container.querySelector('[class*="bg-gradient"]')).toBeNull();
    expect(container.querySelector('[class*="rounded-2xl"]')).toBeNull();
    expect(container.querySelector('[class*="rounded-3xl"]')).toBeNull();
    expect(container.querySelector('[class*="shadow-xl"]')).toBeNull();
  });

  it("states that generic gateway modules have no live transport or write path", () => {
    render(
      <GenericMcpPage title="MCP 服务节点" subtitle="管理关联的微服务 MCP Server" icon={Server} />,
    );

    expect(screen.getByRole("heading", { level: 1, name: "MCP 服务节点" })).toBeTruthy();
    expect(screen.getByText("建设中")).toBeTruthy();
    expect(screen.getByText(/尚未接入真实 JSON-RPC \/ SSE 观测与写操作/)).toBeTruthy();
    const boundary = screen.getByRole("region", { name: "MCP 服务节点 能力边界" });
    expect(within(boundary).getByText(/不生成虚构的网关状态或调用结果/)).toBeTruthy();
    expect(within(boundary).getByText("JSON-RPC / SSE")).toBeTruthy();
    expect(within(boundary).getAllByText("未接入")).toHaveLength(2);
  });
});
