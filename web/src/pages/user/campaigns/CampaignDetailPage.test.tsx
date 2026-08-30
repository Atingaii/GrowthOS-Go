// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router";
import { CampaignDetailPage } from "./CampaignDetailPage";

function renderCampaignDetail(campaignId: string) {
  const router = createMemoryRouter([{ path: "/campaigns/:id", element: <CampaignDetailPage /> }], {
    initialEntries: [`/campaigns/${campaignId}`],
  });

  return render(<RouterProvider router={router} />);
}

afterEach(cleanup);

describe("CampaignDetailPage", () => {
  it("presents a known mock campaign without promising real participation or fulfillment", () => {
    const { container } = renderCampaignDetail("cmp_001");

    expect(container.querySelectorAll("h1")).toHaveLength(1);
    expect(
      screen.getByRole("heading", { level: 1, name: "Spring Growth Surge 2026" }),
    ).toBeTruthy();
    expect(screen.getByText("演示活动")).toBeTruthy();
    expect(screen.getByText(/不代表真实报名、资格或奖励状态/)).toBeTruthy();
    expect(screen.getByText(/当前没有积分账户、库存或发奖服务/)).toBeTruthy();
    expect(container.textContent).not.toContain("即刻发放");

    const copyButton = screen.getByRole("button", {
      name: "复制专属邀请链接（演示未接入）",
    }) as HTMLButtonElement;
    expect(copyButton.disabled).toBe(true);
    expect(screen.getByText("演示未接入")).toBeTruthy();

    expect(
      screen
        .getByRole("progressbar", {
          name: "Spring Growth Surge 2026 活动预算使用进度",
        })
        .getAttribute("aria-valuenow"),
    ).toBe("49.6");
    expect(screen.getByRole("link", { name: "返回活动列表" }).getAttribute("href")).toBe(
      "/campaigns",
    );
    expect(container.querySelector('[class*="bg-gradient"]')).toBeNull();
    expect(container.querySelector('[class*="rounded-3xl"]')).toBeNull();
  });

  it("shows an honest in-page not-found state for an unknown ID", () => {
    renderCampaignDetail("cmp_missing");

    expect(screen.getByRole("heading", { level: 1, name: "活动不存在" })).toBeTruthy();
    expect(screen.getByText(/cmp_missing/)).toBeTruthy();
    expect(screen.getByText(/不会回退展示其他活动/)).toBeTruthy();
    expect(screen.queryByText("Spring Growth Surge 2026")).toBeNull();
    expect(screen.queryByRole("button", { name: /复制专属邀请链接/ })).toBeNull();
    expect(screen.getByRole("link", { name: "返回活动列表" }).getAttribute("href")).toBe(
      "/campaigns",
    );
  });
});
