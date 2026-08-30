// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClientError, type ApiResponse } from "../../../api/httpClient";
import type { EphemeralSelectionResponse } from "../../../api/lotteryApi";
import { LotteryPage } from "./LotteryPage";
import {
  useEphemeralLotterySelection,
  type EphemeralSelectionState,
} from "./useEphemeralLotterySelection";

vi.mock("./useEphemeralLotterySelection", async (importOriginal) => {
  const original = await importOriginal<typeof import("./useEphemeralLotterySelection")>();
  return { ...original, useEphemeralLotterySelection: vi.fn() };
});

const select = vi.fn();
const clear = vi.fn();
const mockedUseSelection = vi.mocked(useEphemeralLotterySelection);

function renderState(state: EphemeralSelectionState) {
  mockedUseSelection.mockReturnValue({ state, select, clear });
  return render(<LotteryPage />);
}

function selectionResponse(
  outcome: "reward" | "no_reward",
): ApiResponse<EphemeralSelectionResponse> {
  return {
    data: {
      durability: "ephemeral",
      strategyId: "18446744073709551615",
      award: {
        id: "18446744073709551615",
        name: outcome === "reward" ? "学习奖励候选" : "未中奖候选",
        outcome,
      },
    },
    status: 200,
    requestId: "req-lottery-page",
    elapsedMs: 27,
  };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("LotteryPage", () => {
  it("starts with an honest, inert development-only form", () => {
    renderState({ phase: "idle" });

    expect(screen.getByRole("heading", { name: "Lottery 临时选择演示" })).toBeTruthy();
    expect(screen.getByText("Development / Test only")).toBeTruthy();
    expect(screen.getByText("非持久化选择")).toBeTruthy();
    expect(screen.getByText(/不会创建 Draw、扣除积分、预占库存或发放奖励/)).toBeTruthy();
    expect(screen.getByText(/页面只证明浏览器结果不再由 Mock 决定/)).toBeTruthy();

    const button = screen.getByRole("button", { name: "发起一次临时选择" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    expect(select).not.toHaveBeenCalled();

    for (const falseClaim of ["100 Points", "恭喜中奖！", "Redis 分布式锁", "5,000 Points"]) {
      expect(screen.queryByText(falseClaim)).toBeNull();
    }
  });

  it("rejects a non-canonical ID in the form and submits a valid string unchanged", () => {
    renderState({ phase: "idle" });
    const input = screen.getByRole("textbox", { name: "Strategy ID" });

    fireEvent.change(input, { target: { value: "01" } });
    expect(screen.getByText(/无前导零、无符号/)).toBeTruthy();
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(input.getAttribute("aria-errormessage")).toBe("lottery-strategy-error");
    expect(
      (screen.getByRole("button", { name: "发起一次临时选择" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(select).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: "18446744073709551615" } });
    expect(input.getAttribute("aria-invalid")).toBe("false");
    expect(input.hasAttribute("aria-errormessage")).toBe(false);
    const button = screen.getByRole("button", { name: "发起一次临时选择" });
    expect((button as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(button);

    expect(select).toHaveBeenCalledOnce();
    expect(select).toHaveBeenCalledWith("18446744073709551615");
    expect(clear).toHaveBeenCalledTimes(2);
  });

  it("renders request progress without pretending the animation performs selection", () => {
    renderState({ phase: "selecting", strategyId: "21003" });

    expect(screen.getByRole("status").textContent).toContain("正在等待服务端结果");
    expect(screen.getByText(/动画只表示请求进行中，不参与随机选择/)).toBeTruthy();
    expect(
      (screen.getByRole("textbox", { name: "Strategy ID" }) as HTMLInputElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "服务端正在选择…" }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it("renders reward as a candidate rather than a durable win or fulfillment", () => {
    renderState({
      phase: "success",
      strategyId: "18446744073709551615",
      response: selectionResponse("reward"),
    });

    const status = screen.getByRole("status");
    expect(status.textContent).toContain("选中了奖励候选");
    expect(status.textContent).toContain("学习奖励候选");
    expect(status.textContent).toContain("这不是中奖记录");
    expect(status.textContent).toContain("18446744073709551615");
    expect(status.textContent).toContain("ephemeral");
    expect(status.textContent).toContain("27 ms");
    expect(status.textContent).toContain("req-lottery-page");
    expect(status.textContent).not.toContain("恭喜中奖");
    expect(status.textContent).not.toContain("获得了");
  });

  it("renders no_reward as a successful business outcome", () => {
    renderState({
      phase: "success",
      strategyId: "18446744073709551615",
      response: selectionResponse("no_reward"),
    });

    const status = screen.getByRole("status");
    expect(status.textContent).toContain("本次选中未中奖候选");
    expect(status.textContent).toContain("no_reward 是合法业务结果");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it.each([
    [
      "missing strategy",
      new ApiClientError("raw backend message", {
        kind: "http",
        status: 404,
        code: "lottery_strategy_not_found",
        requestId: "req-missing",
      }),
      "没有找到这个 Strategy",
    ],
    [
      "disabled route",
      new ApiClientError("raw backend message", {
        kind: "http",
        status: 404,
        code: "route_not_found",
        requestId: "req-route",
      }),
      "临时选择接口当前未启用",
    ],
    [
      "temporarily unavailable",
      new ApiClientError("raw unavailable detail", {
        kind: "http",
        status: 503,
        code: "lottery_selection_unavailable",
        requestId: "req-unavailable",
      }),
      "服务暂时无法给出可信结果",
    ],
    [
      "unknown network outcome",
      new ApiClientError("raw network detail", { kind: "network" }),
      "无法确认这次请求的结果",
    ],
    [
      "gateway timeout envelope",
      new ApiClientError("raw gateway detail", {
        kind: "http",
        status: 504,
        code: "gateway_timeout",
        requestId: "req-gateway",
      }),
      "无法确认这次请求的结果",
    ],
    [
      "invalid response",
      new ApiClientError("raw contract detail", { kind: "contract", status: 200 }),
      "无法验证服务响应契约",
    ],
  ])("renders safe guidance for %s without exposing raw errors", (_name, error, title) => {
    renderState({ phase: "error", strategyId: "21003", error });

    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain(title);
    expect(alert.textContent).not.toContain("raw");
    if (error.requestId) {
      expect(alert.textContent).toContain(error.requestId);
    }
    expect(screen.getByRole("button", { name: "发起一次新的临时选择" })).toBeTruthy();
  });
});
