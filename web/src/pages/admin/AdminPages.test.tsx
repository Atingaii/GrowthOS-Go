// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Target } from "lucide-react";
import { GenericAdminModulePage } from "./GenericAdminModulePage";
import { AdminCampaignsPage } from "./campaigns/AdminCampaignsPage";
import { AdminDashboardPage } from "./dashboard/AdminDashboardPage";

afterEach(cleanup);

describe("Admin operator pages", () => {
  it("labels dashboard values as a non-realtime mock snapshot", () => {
    const { container } = render(<AdminDashboardPage />);

    expect(container.querySelectorAll("h1")).toHaveLength(1);
    expect(screen.getByRole("heading", { level: 1, name: "增长运营大盘" })).toBeTruthy();
    expect(screen.getByText("演示快照")).toBeTruthy();
    expect(screen.getByText(/截至 2026-03-14 12:00 CST/)).toBeTruthy();
    expect(screen.getByText(/不代表实时生产流/)).toBeTruthy();

    const metrics = screen.getByRole("region", { name: "运营指标摘要" });
    expect(within(metrics).getByText("平台注册用户")).toBeTruthy();
    expect(within(metrics).getByText("128,450")).toBeTruthy();
    expect(screen.getByText("静态示意")).toBeTruthy();
    expect(container.querySelector('[class*="bg-gradient"]')).toBeNull();
    expect(container.querySelector('[class*="rounded-2xl"]')).toBeNull();
    expect(container.querySelector('[class*="rounded-3xl"]')).toBeNull();
    expect(container.querySelector('[class*="shadow-xl"]')).toBeNull();
  });

  it("filters local campaign rows and keeps unavailable writes disabled", () => {
    render(<AdminCampaignsPage />);

    expect(screen.getByRole("heading", { level: 1, name: "营销活动与裂变策略" })).toBeTruthy();
    expect(screen.getByText(/创建、编辑与发布链路尚未接入/)).toBeTruthy();
    expect((screen.getByRole("button", { name: /创建新活动/ }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(screen.getAllByRole("button", { name: /编辑 · 待接入/ })).toHaveLength(4);

    fireEvent.change(screen.getByRole("searchbox", { name: "搜索活动" }), {
      target: { value: "cmp_003" },
    });

    const table = screen.getByRole("table", { name: "演示营销活动列表" });
    const tableRegion = screen.getByRole("region", { name: "演示营销活动表格，可横向滚动" });
    expect(tableRegion.getAttribute("tabindex")).toBe("0");
    expect(
      within(table)
        .getAllByRole("columnheader")
        .every((header) => header.getAttribute("scope") === "col"),
    ).toBe(true);
    expect(within(table).getByText("Wheel of Fortune Lucky Draw")).toBeTruthy();
    expect(within(table).queryByText("Spring Growth Surge 2026")).toBeNull();

    fireEvent.change(screen.getByRole("searchbox", { name: "搜索活动" }), {
      target: { value: "不存在的活动" },
    });
    expect(screen.getByRole("status").textContent).toContain("没有匹配的演示活动");
  });

  it("keeps generic modules visibly inert until their real contract exists", () => {
    render(
      <GenericAdminModulePage title="抽奖策略" subtitle="配置抽奖概率与策略控制" icon={Target} />,
    );

    expect(screen.getByRole("heading", { level: 1, name: "抽奖策略" })).toBeTruthy();
    expect(screen.getByText("建设中")).toBeTruthy();
    expect(screen.getByText(/真实数据与写操作尚未接入/)).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: /配置 抽奖策略/ }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });
});
