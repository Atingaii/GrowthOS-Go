// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CouponsPage } from "./CouponsPage";

const originalClipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, "clipboard");

function installClipboard(writeText: ReturnType<typeof vi.fn> | undefined) {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: writeText ? { writeText } : undefined,
  });
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  if (originalClipboardDescriptor) {
    Object.defineProperty(navigator, "clipboard", originalClipboardDescriptor);
  } else {
    Reflect.deleteProperty(navigator, "clipboard");
  }
});

describe("CouponsPage", () => {
  it("filters the mock coupons between available and used states", () => {
    render(<CouponsPage />);

    expect(screen.getByRole("heading", { level: 1, name: "优惠券与权益中心" })).toBeTruthy();
    expect(screen.getByText(/本地 Mock 优惠券/)).toBeTruthy();

    const availableFilter = screen.getByRole("button", { name: "可用 2" });
    const usedFilter = screen.getByRole("button", { name: "已使用 1" });
    expect(availableFilter.getAttribute("aria-pressed")).toBe("true");
    expect(usedFilter.getAttribute("aria-pressed")).toBe("false");
    expect(screen.getAllByRole("article")).toHaveLength(2);
    expect(
      screen.getByRole("heading", { level: 3, name: "$50 Off Pro Plan Upgrade" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { level: 3, name: "20% Rebate on MCP Gateway" }),
    ).toBeNull();

    fireEvent.click(usedFilter);

    expect(availableFilter.getAttribute("aria-pressed")).toBe("false");
    expect(usedFilter.getAttribute("aria-pressed")).toBe("true");
    expect(screen.getAllByRole("article")).toHaveLength(1);
    expect(
      screen.getByRole("heading", { level: 3, name: "20% Rebate on MCP Gateway" }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: /复制优惠码/ })).toBeNull();
  });

  it("announces a successful clipboard write and clears the feedback", async () => {
    vi.useFakeTimers();
    const writeText = vi.fn().mockResolvedValue(undefined);
    installClipboard(writeText);
    render(<CouponsPage />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "复制优惠码 GROWTH2026" }));
      await Promise.resolve();
    });

    expect(writeText).toHaveBeenCalledOnce();
    expect(writeText).toHaveBeenCalledWith("GROWTH2026");
    expect(screen.getByRole("status").textContent).toContain("优惠码 GROWTH2026 已复制");
    expect(screen.getByRole("button", { name: "已复制优惠码 GROWTH2026" })).toBeTruthy();

    act(() => vi.advanceTimersByTime(2500));

    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.getByRole("button", { name: "复制优惠码 GROWTH2026" })).toBeTruthy();
  });

  it("reports clipboard failure without claiming that the code was copied", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("permission denied"));
    installClipboard(writeText);
    render(<CouponsPage />);

    fireEvent.click(screen.getByRole("button", { name: "复制优惠码 AGENTVIP" }));

    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect(screen.getByRole("alert").textContent).toContain("无法复制优惠码 AGENTVIP");
    expect(screen.getByRole("button", { name: "复制优惠码 AGENTVIP" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /已复制优惠码 AGENTVIP/ })).toBeNull();
  });

  it("clears a pending feedback timer when the page unmounts", async () => {
    vi.useFakeTimers();
    const writeText = vi.fn().mockResolvedValue(undefined);
    installClipboard(writeText);
    const { unmount } = render(<CouponsPage />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "复制优惠码 GROWTH2026" }));
      await Promise.resolve();
    });

    expect(vi.getTimerCount()).toBe(1);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
