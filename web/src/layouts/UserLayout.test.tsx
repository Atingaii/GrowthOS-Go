// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router";
import { useAppStore } from "../stores/appStore";
import { UserLayout } from "./UserLayout";

function renderUserRoute(pathname = "/lottery") {
  const router = createMemoryRouter(
    [
      {
        path: "/",
        element: <UserLayout />,
        children: [
          { index: true, element: <div>Route content</div> },
          { path: "*", element: <div>Route content</div> },
        ],
      },
    ],
    { initialEntries: [pathname] },
  );

  return { router, ...render(<RouterProvider router={router} />) };
}

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

describe("UserLayout workspace shell", () => {
  it("exposes one main landmark, a skip link, and the active navigation item", () => {
    const { container } = renderUserRoute();

    expect(container.querySelectorAll("main")).toHaveLength(1);
    expect(screen.getByRole("main").id).toBe("main-content");
    expect(screen.getByRole("link", { name: "跳到主要内容" }).getAttribute("href")).toBe(
      "#main-content",
    );
    expect(screen.getByRole("navigation", { name: "Growth Platform 导航" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "幸运抽奖" }).getAttribute("aria-current")).toBe(
      "page",
    );
  });

  it.each([
    ["/", "首页"],
    ["/home", "首页"],
    ["/campaigns/cmp_001", "营销活动"],
    ["/rewards", "积分中心"],
    ["/profile", "个人中心"],
  ])("marks %s as %s in the sidebar", (pathname, activeLabel) => {
    renderUserRoute(pathname);

    expect(screen.getByRole("link", { name: activeLabel }).getAttribute("aria-current")).toBe(
      "page",
    );
  });

  it("collapses the desktop sidebar without losing link names", () => {
    renderUserRoute();

    const collapseButton = screen.getByRole("button", { name: "收起侧边栏" });
    expect(collapseButton.getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(collapseButton);

    expect(useAppStore.getState().sidebarCollapsed).toBe(true);
    expect(screen.getByRole("button", { name: "展开侧边栏" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.getByRole("link", { name: "幸运抽奖" }).getAttribute("aria-current")).toBe(
      "page",
    );
  });

  it("opens and closes a complete mobile navigation dialog", () => {
    renderUserRoute();

    const openButton = screen.getByRole("button", { name: "打开导航" });
    expect(openButton.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("dialog", { name: "Growth Platform 移动导航" })).toBeNull();

    fireEvent.click(openButton);
    const drawer = screen.getByRole("dialog", { name: "Growth Platform 移动导航" });
    expect(openButton.getAttribute("aria-expanded")).toBe("true");
    expect(within(drawer).getByRole("link", { name: "优惠券" })).toBeTruthy();
    expect(within(drawer).getByRole("link", { name: "个人中心" })).toBeTruthy();
    expect(document.body.style.overflow).toBe("hidden");
    expect(document.activeElement).toBe(within(drawer).getByRole("button", { name: "关闭导航" }));

    fireEvent.keyDown(window, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "Growth Platform 移动导航" })).toBeNull();
    expect(document.body.style.overflow).toBe("");
    expect(document.activeElement).toBe(openButton);
  });

  it("filters the navigation palette and uses client-side routing", () => {
    const { router } = renderUserRoute("/home");

    fireEvent.keyDown(window, { key: "k", metaKey: true });
    const palette = screen.getByRole("dialog", { name: "搜索 GrowthOS 页面" });
    const search = within(palette).getByRole("textbox", { name: "搜索页面与功能" });
    fireEvent.change(search, { target: { value: "抽奖" } });

    const result = within(palette).getByRole("link", { name: /幸运抽奖/ });
    expect(result.getAttribute("href")).toBe("/lottery");
    fireEvent.click(result);

    expect(router.state.location.pathname).toBe("/lottery");
    expect(screen.queryByRole("dialog", { name: "搜索 GrowthOS 页面" })).toBeNull();
  });

  it("names the topbar actions and synchronizes both theme directions", () => {
    renderUserRoute();

    expect(screen.getByRole("button", { name: "查看通知（有未读）" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "打开设置" }).getAttribute("href")).toBe("/profile");
    expect(screen.getByRole("link", { name: "进入活动中心" }).getAttribute("href")).toBe(
      "/campaigns",
    );

    const darkButton = screen.getByRole("button", { name: "切换至暗色主题" });
    expect(darkButton.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(darkButton);
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(useAppStore.getState().theme).toBe("dark");

    const lightButton = screen.getByRole("button", { name: "切换至亮色主题" });
    expect(lightButton.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(lightButton);
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(useAppStore.getState().theme).toBe("light");
  });
});
