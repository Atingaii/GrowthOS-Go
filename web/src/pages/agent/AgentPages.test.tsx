// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ListTodo } from "lucide-react";
import { GenericAgentPage } from "./GenericAgentPage";
import { AgentWorkspacePage } from "./workspace/AgentWorkspacePage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("Agent operator pages", () => {
  it("creates only an in-memory demo task and leaves approvals inert", () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const { container } = render(<AgentWorkspacePage />);

    expect(screen.getByRole("heading", { level: 1, name: "AI Operator 工作台" })).toBeTruthy();
    expect(screen.getByText(/不会调用 Agent、MCP Tool 或 GrowthOS 写接口/)).toBeTruthy();
    expect(screen.getByText(/刷新后丢失，不代表 Agent 已执行/)).toBeTruthy();

    const submit = screen.getByRole("button", { name: "加入本地演示队列" });
    expect((submit as HTMLButtonElement).disabled).toBe(true);
    expect(
      (screen.getByRole("button", { name: "批准 · 未接入" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "驳回 · 未接入" }) as HTMLButtonElement).disabled,
    ).toBe(true);

    const prompt = screen.getByRole("textbox", { name: "创建本地演示任务" });
    expect((prompt as HTMLTextAreaElement).maxLength).toBe(500);
    fireEvent.change(prompt, { target: { value: "检查活动漏斗但不执行任何后端写操作" } });
    expect((submit as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(submit);

    const tasks = screen.getByRole("region", { name: "演示任务" });
    expect(within(tasks).getByText("3 条")).toBeTruthy();
    expect(
      within(tasks).getByRole("heading", {
        level: 3,
        name: "检查活动漏斗但不执行任何后端写操作",
      }),
    ).toBeTruthy();
    expect(within(tasks).getByText("仅本地")).toBeTruthy();
    expect(within(tasks).getByText("尚未发送至后端")).toBeTruthy();
    expect((prompt as HTMLTextAreaElement).value).toBe("");
    expect(screen.getByRole("status").textContent).toContain("没有发送到后端");
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(container.querySelector('[class*="bg-gradient"]')).toBeNull();
    expect(container.querySelector('[class*="rounded-2xl"]')).toBeNull();
    expect(container.querySelector('[class*="rounded-3xl"]')).toBeNull();
    expect(container.querySelector('[class*="shadow-xl"]')).toBeNull();
  });

  it("keeps generic Agent modules scoped to information architecture", () => {
    render(
      <GenericAgentPage
        title="任务队列"
        subtitle="查看 Agent 正在运行与排队中的 Task"
        icon={ListTodo}
      />,
    );

    expect(screen.getByRole("heading", { level: 1, name: "任务队列" })).toBeTruthy();
    expect(screen.getByText("建设中")).toBeTruthy();
    expect(screen.getByText(/尚未连接 Agent 调度或人工审批后端/)).toBeTruthy();
    const boundary = screen.getByRole("region", { name: "任务队列 能力边界" });
    expect(within(boundary).getByText(/不伪造执行中任务或审批结果/)).toBeTruthy();
    expect(within(boundary).getByText("Agent 调度")).toBeTruthy();
    expect(within(boundary).getAllByText("未接入")).toHaveLength(2);
  });
});
