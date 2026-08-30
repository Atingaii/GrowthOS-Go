// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router";
import { CampaignsListPage } from "./CampaignsListPage";

function renderCampaignsList() {
  const router = createMemoryRouter(
    [
      { path: "/campaigns", element: <CampaignsListPage /> },
      { path: "/campaigns/:id", element: <div>客户端详情路由</div> },
    ],
    { initialEntries: ["/campaigns"] },
  );

  return render(<RouterProvider router={router} />);
}

afterEach(cleanup);

describe("CampaignsListPage", () => {
  it("renders a dense, honest campaign directory with named budget progress", () => {
    const { container } = renderCampaignsList();

    expect(container.querySelectorAll("h1")).toHaveLength(1);
    expect(screen.getByRole("heading", { level: 1, name: "营销活动" })).toBeTruthy();
    expect(screen.getByText("演示数据")).toBeTruthy();
    expect(screen.getByText(/不会自动报名、扣减预算或发放奖励/)).toBeTruthy();
    expect(screen.getAllByRole("progressbar")).toHaveLength(4);
    expect(
      screen
        .getByRole("progressbar", {
          name: "Spring Growth Surge 2026 活动预算使用进度",
        })
        .getAttribute("aria-valuenow"),
    ).toBe("49.6");
    expect(container.querySelector('[class*="bg-gradient"]')).toBeNull();
    expect(container.querySelector('[class*="rounded-3xl"]')).toBeNull();
  });

  it("filters the rendered cards by category and can clear the filter", () => {
    renderCampaignsList();

    fireEvent.change(screen.getByRole("combobox", { name: "按活动分类筛选" }), {
      target: { value: "Gamification" },
    });

    expect(screen.getByText("1 个活动")).toBeTruthy();
    expect(
      screen.getByRole("heading", { level: 3, name: "Wheel of Fortune Lucky Draw" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { level: 3, name: "Spring Growth Surge 2026" }),
    ).toBeNull();
    expect(screen.getAllByRole("progressbar")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "清除筛选" }));
    expect(screen.getByText("4 个活动")).toBeTruthy();
    expect(
      screen.getByRole("heading", { level: 3, name: "Spring Growth Surge 2026" }),
    ).toBeTruthy();
  });

  it("uses the client router for internal campaign detail navigation", () => {
    renderCampaignsList();

    const detailLink = screen.getByRole("link", {
      name: "查看 Spring Growth Surge 2026 活动详情",
    });
    expect(detailLink.getAttribute("href")).toBe("/campaigns/cmp_001");

    fireEvent.click(detailLink);
    expect(screen.getByText("客户端详情路由")).toBeTruthy();
  });
});
