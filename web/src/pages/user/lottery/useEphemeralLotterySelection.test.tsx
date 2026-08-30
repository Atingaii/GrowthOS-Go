// @vitest-environment jsdom

import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClientError, type ApiResponse } from "../../../api/httpClient";
import {
  requestEphemeralSelection,
  type EphemeralSelectionResponse,
} from "../../../api/lotteryApi";
import { useEphemeralLotterySelection } from "./useEphemeralLotterySelection";

vi.mock("../../../api/lotteryApi", () => ({
  requestEphemeralSelection: vi.fn(),
}));

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
}

function deferred<T>(): Deferred<T> {
  let resolve: (value: T) => void = () => undefined;
  let reject: (reason?: unknown) => void = () => undefined;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

function selectionResponse(
  strategyId: string,
  outcome: "reward" | "no_reward",
): ApiResponse<EphemeralSelectionResponse> {
  return {
    data: {
      durability: "ephemeral",
      strategyId,
      award: {
        id: "1",
        name: outcome === "reward" ? "Reward" : "Try again",
        outcome,
      },
    },
    status: 200,
    requestId: `selection-${strategyId}`,
    elapsedMs: 12,
  };
}

const mockedRequestEphemeralSelection = vi.mocked(requestEphemeralSelection);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useEphemeralLotterySelection", () => {
  it.each(["reward", "no_reward"] as const)(
    "publishes a successful %s selection without issuing a request on mount",
    async (outcome) => {
      const request = deferred<ApiResponse<EphemeralSelectionResponse>>();
      mockedRequestEphemeralSelection.mockReturnValue(request.promise);

      const { result } = renderHook(() => useEphemeralLotterySelection());

      expect(result.current.state).toEqual({ phase: "idle" });
      expect(mockedRequestEphemeralSelection).not.toHaveBeenCalled();

      act(() => result.current.select("21003"));

      expect(result.current.state).toEqual({ phase: "selecting", strategyId: "21003" });
      expect(mockedRequestEphemeralSelection).toHaveBeenCalledWith("21003", {
        signal: expect.any(AbortSignal),
      });

      const response = selectionResponse("21003", outcome);
      await act(async () => request.resolve(response));

      await waitFor(() => expect(result.current.state.phase).toBe("success"));
      expect(result.current.state).toEqual({
        phase: "success",
        strategyId: "21003",
        response,
      });
      expect(mockedRequestEphemeralSelection).toHaveBeenCalledTimes(1);
    },
  );

  it("publishes a classified API error and never retries automatically", async () => {
    const request = deferred<ApiResponse<EphemeralSelectionResponse>>();
    mockedRequestEphemeralSelection.mockReturnValue(request.promise);
    const apiError = new ApiClientError("抽奖服务暂不可用", {
      kind: "http",
      status: 503,
      code: "lottery_selection_unavailable",
      requestId: "selection-error",
    });

    const { result } = renderHook(() => useEphemeralLotterySelection());
    act(() => result.current.select("21003"));
    await act(async () => request.reject(apiError));

    await waitFor(() => expect(result.current.state.phase).toBe("error"));
    expect(result.current.state).toEqual({
      phase: "error",
      strategyId: "21003",
      error: apiError,
    });
    expect(mockedRequestEphemeralSelection).toHaveBeenCalledTimes(1);
  });

  it("starts a second request only after the consumer explicitly selects again", async () => {
    const firstRequest = deferred<ApiResponse<EphemeralSelectionResponse>>();
    const secondRequest = deferred<ApiResponse<EphemeralSelectionResponse>>();
    mockedRequestEphemeralSelection
      .mockReturnValueOnce(firstRequest.promise)
      .mockReturnValueOnce(secondRequest.promise);
    const apiError = new ApiClientError("gateway response lost", {
      kind: "http",
      status: 504,
      code: "gateway_timeout",
      requestId: "selection-unknown",
    });

    const { result } = renderHook(() => useEphemeralLotterySelection());
    act(() => result.current.select("21003"));
    await act(async () => firstRequest.reject(apiError));

    await waitFor(() => expect(result.current.state.phase).toBe("error"));
    expect(mockedRequestEphemeralSelection).toHaveBeenCalledTimes(1);

    act(() => result.current.select("21003"));
    expect(result.current.state).toEqual({ phase: "selecting", strategyId: "21003" });
    expect(mockedRequestEphemeralSelection).toHaveBeenCalledTimes(2);

    const response = selectionResponse("21003", "reward");
    await act(async () => secondRequest.resolve(response));

    await waitFor(() => expect(result.current.state.phase).toBe("success"));
    expect(result.current.state).toEqual({
      phase: "success",
      strategyId: "21003",
      response,
    });
  });

  it("suppresses every duplicate submission while one selection is pending", () => {
    const request = deferred<ApiResponse<EphemeralSelectionResponse>>();
    mockedRequestEphemeralSelection.mockReturnValue(request.promise);

    const { result, unmount } = renderHook(() => useEphemeralLotterySelection());

    act(() => {
      result.current.select("21003");
      result.current.select("21003");
      result.current.select("99999");
    });

    expect(mockedRequestEphemeralSelection).toHaveBeenCalledTimes(1);
    expect(mockedRequestEphemeralSelection).toHaveBeenCalledWith("21003", {
      signal: expect.any(AbortSignal),
    });
    expect(result.current.state).toEqual({ phase: "selecting", strategyId: "21003" });

    unmount();
  });

  it("aborts the active request when the consumer unmounts", () => {
    const request = deferred<ApiResponse<EphemeralSelectionResponse>>();
    mockedRequestEphemeralSelection.mockReturnValue(request.promise);

    const { result, unmount } = renderHook(() => useEphemeralLotterySelection());
    act(() => result.current.select("21003"));
    const signal = mockedRequestEphemeralSelection.mock.calls[0]?.[1]?.signal;

    expect(signal?.aborted).toBe(false);
    unmount();
    expect(signal?.aborted).toBe(true);

    request.reject(new ApiClientError("请求已取消", { kind: "cancelled" }));
  });

  it("clear starts a new generation so a cancelled stale promise cannot overwrite a newer result", async () => {
    const oldRequest = deferred<ApiResponse<EphemeralSelectionResponse>>();
    const newRequest = deferred<ApiResponse<EphemeralSelectionResponse>>();
    mockedRequestEphemeralSelection
      .mockReturnValueOnce(oldRequest.promise)
      .mockReturnValueOnce(newRequest.promise);

    const { result } = renderHook(() => useEphemeralLotterySelection());
    act(() => result.current.select("21003"));
    const oldSignal = mockedRequestEphemeralSelection.mock.calls[0]?.[1]?.signal;

    act(() => result.current.clear());
    expect(oldSignal?.aborted).toBe(true);
    expect(result.current.state).toEqual({ phase: "idle" });

    act(() => result.current.select("21004"));
    const newResponse = selectionResponse("21004", "reward");
    await act(async () => newRequest.resolve(newResponse));
    await waitFor(() => expect(result.current.state.phase).toBe("success"));

    await act(async () =>
      oldRequest.reject(new ApiClientError("请求已取消", { kind: "cancelled" })),
    );

    expect(result.current.state).toEqual({
      phase: "success",
      strategyId: "21004",
      response: newResponse,
    });
    expect(mockedRequestEphemeralSelection).toHaveBeenCalledTimes(2);
  });

  it("clear returns settled states to idle without starting another request", async () => {
    const response = selectionResponse("21003", "no_reward");
    mockedRequestEphemeralSelection.mockResolvedValue(response);

    const { result } = renderHook(() => useEphemeralLotterySelection());
    act(() => result.current.select("21003"));
    await waitFor(() => expect(result.current.state.phase).toBe("success"));

    act(() => result.current.clear());

    expect(result.current.state).toEqual({ phase: "idle" });
    expect(mockedRequestEphemeralSelection).toHaveBeenCalledTimes(1);
  });

  it("does not auto-submit during the StrictMode setup-cleanup-setup cycle", () => {
    const { result, unmount } = renderHook(() => useEphemeralLotterySelection(), {
      reactStrictMode: true,
    });

    expect(result.current.state).toEqual({ phase: "idle" });
    expect(mockedRequestEphemeralSelection).not.toHaveBeenCalled();

    unmount();
    expect(mockedRequestEphemeralSelection).not.toHaveBeenCalled();
  });
});
