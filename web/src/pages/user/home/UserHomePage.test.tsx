// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router";
import { UserHomePage } from "./UserHomePage";

function renderHome() {
  const router = createMemoryRouter([{ path: "/home", element: <UserHomePage /> }], {
    initialEntries: ["/home"],
  });
  return render(<RouterProvider router={router} />);
}

afterEach(cleanup);

describe("UserHomePage", () => {
  it("presents a dense dashboard with an explicit demo-data boundary", () => {
    const { container } = renderHome();

    expect(container.querySelectorAll("h1")).toHaveLength(1);
    expect(screen.getByRole("heading", { level: 1, name: "今天" })).toBeTruthy();
    expect(screen.getByText("演示数据")).toBeTruthy();
    expect(screen.getByText(/不代表真实账户账务/)).toBeTruthy();
    expect(screen.getByRole("heading", { name: "积分趋势" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "近期概览" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "活动快照" })).toBeTruthy();
  });

  it("keeps the key dashboard journeys on client-side links", () => {
    renderHome();

    expect(screen.getByRole("link", { name: /查看活动/ }).getAttribute("href")).toBe("/campaigns");
    expect(screen.getByRole("link", { name: "查看全部积分账单" }).getAttribute("href")).toBe(
      "/points",
    );
    expect(screen.queryByRole("button", { name: /刷新/ })).toBeNull();
    expect(screen.getByText("+670 PTS")).toBeTruthy();
    expect(screen.getByText("-200 PTS")).toBeTruthy();
    expect(screen.getByRole("link", { name: /Spring Growth Surge 2026/ })).toBeTruthy();
  });
});
