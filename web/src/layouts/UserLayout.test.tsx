// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router";
import { useAppStore } from "../stores/appStore";
import { UserLayout } from "./UserLayout";

function renderLotteryRoute() {
  const router = createMemoryRouter(
    [
      {
        path: "/lottery",
        element: <UserLayout />,
        children: [{ index: true, element: <div>Lottery route content</div> }],
      },
    ],
    { initialEntries: ["/lottery"] },
  );

  return render(<RouterProvider router={router} />);
}

beforeEach(() => {
  document.documentElement.classList.remove("dark");
  useAppStore.setState({ theme: "light" });
});

afterEach(() => {
  cleanup();
  document.documentElement.classList.remove("dark");
  useAppStore.setState({ theme: "light" });
});

describe("UserLayout accessibility", () => {
  it("exposes one main landmark and marks the active navigation links", () => {
    const { container } = renderLotteryRoute();

    expect(container.querySelectorAll("main")).toHaveLength(1);
    const lotteryLinks = screen.getAllByRole("link", { name: "幸运抽奖" });
    expect(lotteryLinks).toHaveLength(2);
    for (const link of lotteryLinks) {
      expect(link.getAttribute("aria-current")).toBe("page");
    }
  });

  it("names the header actions and exposes the selected theme", () => {
    renderLotteryRoute();

    expect(screen.getByRole("button", { name: "查看通知（有未读）" })).toBeTruthy();
    const themeButton = screen.getByRole("button", { name: "切换至暗色主题" });
    expect(themeButton.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(themeButton);

    const lightThemeButton = screen.getByRole("button", { name: "切换至亮色主题" });
    expect(lightThemeButton.getAttribute("aria-pressed")).toBe("true");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });
});
