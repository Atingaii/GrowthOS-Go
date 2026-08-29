// @vitest-environment jsdom

import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ApiResponse } from "../../../api/httpClient";
import type { HealthResponse, ReadinessResponse } from "../../../api/systemApi";
import { fetchHealth, fetchReadiness } from "../../../api/systemApi";
import { useSystemStatus } from "./useSystemStatus";

vi.mock("../../../api/systemApi", async (importOriginal) => {
  const original = await importOriginal<typeof import("../../../api/systemApi")>();
  return {
    ...original,
    fetchHealth: vi.fn(),
    fetchReadiness: vi.fn(),
  };
});

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

function healthResponse(version: string): ApiResponse<HealthResponse> {
  return {
    data: { status: "ok", version, timestamp: "2026-08-29T08:00:00Z" },
    status: 200,
    requestId: `health-${version}`,
    elapsedMs: 8,
  };
}

function readinessResponse(version: string): ApiResponse<ReadinessResponse> {
  return {
    data: { status: "ready", version, timestamp: "2026-08-29T08:00:00Z" },
    status: 200,
    requestId: `ready-${version}`,
    elapsedMs: 11,
  };
}

const mockedFetchHealth = vi.mocked(fetchHealth);
const mockedFetchReadiness = vi.mocked(fetchReadiness);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useSystemStatus", () => {
  it("publishes each probe independently as soon as it completes", async () => {
    const health = deferred<ApiResponse<HealthResponse>>();
    const readiness = deferred<ApiResponse<ReadinessResponse>>();
    mockedFetchHealth.mockReturnValue(health.promise);
    mockedFetchReadiness.mockReturnValue(readiness.promise);

    const { result } = renderHook(() => useSystemStatus());

    await act(async () => health.resolve(healthResponse("health-first")));
    await waitFor(() => expect(result.current.state.health.phase).toBe("success"));
    expect(result.current.state.readiness.phase).toBe("loading");
    expect(result.current.state.completedAt).toBeUndefined();

    await act(async () => readiness.resolve(readinessResponse("ready-second")));
    await waitFor(() => expect(result.current.state.readiness.phase).toBe("success"));
    await waitFor(() => expect(result.current.state.completedAt).toBeDefined());
  });

  it("refresh aborts the old generation and stale completions cannot overwrite the new result", async () => {
    const oldHealth = deferred<ApiResponse<HealthResponse>>();
    const oldReadiness = deferred<ApiResponse<ReadinessResponse>>();
    const newHealth = deferred<ApiResponse<HealthResponse>>();
    const newReadiness = deferred<ApiResponse<ReadinessResponse>>();
    mockedFetchHealth.mockReturnValueOnce(oldHealth.promise).mockReturnValueOnce(newHealth.promise);
    mockedFetchReadiness
      .mockReturnValueOnce(oldReadiness.promise)
      .mockReturnValueOnce(newReadiness.promise);

    const { result } = renderHook(() => useSystemStatus());
    const oldHealthSignal = mockedFetchHealth.mock.calls[0]?.[0]?.signal;
    const oldReadinessSignal = mockedFetchReadiness.mock.calls[0]?.[0]?.signal;

    act(() => result.current.refresh());

    expect(oldHealthSignal?.aborted).toBe(true);
    expect(oldReadinessSignal?.aborted).toBe(true);

    await act(async () => {
      newHealth.resolve(healthResponse("new"));
      newReadiness.resolve(readinessResponse("new"));
    });
    await waitFor(() => expect(result.current.state.completedAt).toBeDefined());

    await act(async () => {
      oldHealth.resolve(healthResponse("stale"));
      oldReadiness.resolve(readinessResponse("stale"));
    });

    expect(result.current.state.health).toMatchObject({
      phase: "success",
      response: { data: { version: "new" } },
    });
    expect(result.current.state.readiness).toMatchObject({
      phase: "success",
      response: { data: { version: "new" } },
    });
  });

  it("aborts both active probe requests when the consumer unmounts", async () => {
    const health = deferred<ApiResponse<HealthResponse>>();
    const readiness = deferred<ApiResponse<ReadinessResponse>>();
    mockedFetchHealth.mockReturnValue(health.promise);
    mockedFetchReadiness.mockReturnValue(readiness.promise);

    const { unmount } = renderHook(() => useSystemStatus());
    const healthSignal = mockedFetchHealth.mock.calls[0]?.[0]?.signal;
    const readinessSignal = mockedFetchReadiness.mock.calls[0]?.[0]?.signal;

    unmount();

    expect(healthSignal?.aborted).toBe(true);
    expect(readinessSignal?.aborted).toBe(true);

    health.resolve(healthResponse("after-unmount"));
    readiness.resolve(readinessResponse("after-unmount"));
  });

  it("survives the StrictMode setup-cleanup-setup cycle without stale active requests", () => {
    const foreverPending = new Promise<never>(() => undefined);
    mockedFetchHealth.mockReturnValue(foreverPending);
    mockedFetchReadiness.mockReturnValue(foreverPending);

    const { unmount } = renderHook(() => useSystemStatus(), {
      reactStrictMode: true,
    });

    expect(mockedFetchHealth).toHaveBeenCalledTimes(2);
    expect(mockedFetchReadiness).toHaveBeenCalledTimes(2);
    expect(mockedFetchHealth.mock.calls[0]?.[0]?.signal?.aborted).toBe(true);
    expect(mockedFetchReadiness.mock.calls[0]?.[0]?.signal?.aborted).toBe(true);
    expect(mockedFetchHealth.mock.calls[1]?.[0]?.signal?.aborted).toBe(false);
    expect(mockedFetchReadiness.mock.calls[1]?.[0]?.signal?.aborted).toBe(false);

    unmount();

    expect(mockedFetchHealth.mock.calls[1]?.[0]?.signal?.aborted).toBe(true);
    expect(mockedFetchReadiness.mock.calls[1]?.[0]?.signal?.aborted).toBe(true);
  });
});
