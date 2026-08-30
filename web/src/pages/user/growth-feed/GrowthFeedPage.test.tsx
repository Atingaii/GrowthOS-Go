// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router";
import { MOCK_SNAPSHOT_LABEL, mockFeedItems } from "../../../mocks/growthOsMockData";
import { GrowthFeedPage } from "./GrowthFeedPage";

function renderFeed() {
  return render(
    <MemoryRouter>
      <GrowthFeedPage />
    </MemoryRouter>,
  );
}

afterEach(cleanup);

describe("GrowthFeedPage", () => {
  it("renders the mock feed as a dense, explicitly read-only article list", () => {
    renderFeed();

    expect(screen.getByRole("heading", { level: 1, name: "Growth Feed 社区态势" })).toBeTruthy();
    expect(screen.getByText("演示数据")).toBeTruthy();
    expect(screen.getByText(/本地模拟案例/)).toBeTruthy();
    expect(screen.getByText(new RegExp(MOCK_SNAPSHOT_LABEL))).toBeTruthy();
    expect(screen.getByText("只读演示")).toBeTruthy();
    expect(screen.getAllByRole("article")).toHaveLength(mockFeedItems.length);

    for (const feed of mockFeedItems) {
      expect(screen.getByRole("heading", { level: 3, name: feed.title })).toBeTruthy();
    }
  });

  it("uses client-side links for the only implemented feed action", () => {
    renderFeed();

    const links = screen.getAllByRole("link", { name: /查看活动详情/ });
    const campaignLinks = mockFeedItems.flatMap((feed) =>
      feed.campaignLink ? [feed.campaignLink] : [],
    );

    expect(links).toHaveLength(campaignLinks.length);
    expect(links.map((link) => link.getAttribute("href"))).toEqual(campaignLinks);
  });

  it("keeps publishing and interaction controls visibly inert", () => {
    renderFeed();

    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(1 + mockFeedItems.length * 3);
    expect(buttons.every((button) => (button as HTMLButtonElement).disabled)).toBe(true);
    expect(screen.getByRole("button", { name: "发布 Feed（未接入）" })).toBeTruthy();
    expect(screen.getAllByText("演示计数 · 互动未接入")).toHaveLength(mockFeedItems.length);
  });
});
