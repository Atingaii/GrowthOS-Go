// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router";
import { ApiClientError, type ApiResponse } from "../../../api/httpClient";
import type { HealthResponse, ReadinessResponse } from "../../../api/systemApi";
import { SystemStatusPage } from "./SystemStatusPage";
import { useSystemStatus, type SystemStatusState } from "./useSystemStatus";

vi.mock("./useSystemStatus", async (importOriginal) => {
  const original = await importOriginal<typeof import("./useSystemStatus")>();
  return { ...original, useSystemStatus: vi.fn() };
});

const refresh = vi.fn();
const mockedUseSystemStatus = vi.mocked(useSystemStatus);

function healthResponse(): ApiResponse<HealthResponse> {
  return {
    data: {
      status: "ok",
      version: "lesson-15",
      timestamp: "2026-08-29T08:00:00Z",
    },
    status: 200,
    requestId: "req-health",
    elapsedMs: 9,
  };
}

function readinessResponse(): ApiResponse<ReadinessResponse> {
  return {
    data: {
      status: "ready",
      version: "lesson-15",
      timestamp: "2026-08-29T08:00:00Z",
    },
    status: 200,
    requestId: "req-ready",
    elapsedMs: 13,
  };
}

function renderState(state: SystemStatusState) {
  mockedUseSystemStatus.mockReturnValue({ state, refresh });
  return render(
    <MemoryRouter>
      <SystemStatusPage />
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SystemStatusPage", () => {
  it("renders an honest loading state for only the two implemented probes", () => {
    renderState({
      health: { phase: "loading" },
      readiness: { phase: "loading" },
    });

    expect(screen.getByText("正在检查")).toBeTruthy();
    expect(screen.getByText("Go API 进程")).toBeTruthy();
    expect(screen.getByText("MySQL readiness")).toBeTruthy();
    expect((screen.getByRole("button", { name: "检查中" }) as HTMLButtonElement).disabled).toBe(
      true,
    );

    for (const fictionalService of [
      "接口网关",
      "营销活动引擎",
      "抽奖与策略",
      "积分账本",
      "AI 工具调度",
      "智能增长助手",
      "全部正常",
    ]) {
      expect(screen.queryByText(fictionalService)).toBeNull();
    }
  });

  it("renders healthy only when both real probes succeed", () => {
    renderState({
      health: { phase: "success", response: healthResponse() },
      readiness: { phase: "success", response: readinessResponse() },
      completedAt: "2026-08-29T08:00:01Z",
    });

    expect(screen.getByText("已接入检查正常")).toBeTruthy();
    expect(screen.getByText("req-health")).toBeTruthy();
    expect(screen.getByText("req-ready")).toBeTruthy();
    expect(screen.getAllByText("lesson-15")).toHaveLength(2);
  });

  it("renders a degraded state when the process lives but MySQL is not ready", () => {
    const readinessError = new ApiClientError("internal detail is not rendered", {
      kind: "http",
      status: 503,
      code: "dependency_unavailable",
      requestId: "req-not-ready",
      elapsedMs: 21,
    });
    renderState({
      health: { phase: "success", response: healthResponse() },
      readiness: { phase: "error", error: readinessError },
    });

    expect(screen.getByText("API 存活，MySQL 未就绪")).toBeTruthy();
    expect(screen.getByText("MySQL 连接未就绪")).toBeTruthy();
    expect(screen.getByText("req-not-ready")).toBeTruthy();
    expect(screen.queryByText("internal detail is not rendered")).toBeNull();
  });

  it("renders status as unknown/offline when health cannot reach the API", () => {
    renderState({
      health: {
        phase: "error",
        error: new ApiClientError("network detail", { kind: "network" }),
      },
      readiness: {
        phase: "error",
        error: new ApiClientError("network detail", { kind: "network" }),
      },
    });

    expect(screen.getByText("无法确认 API 状态")).toBeTruthy();
    expect(screen.getAllByText("无法连接 API")).toHaveLength(2);
  });

  it("keeps the overall state unknown when readiness succeeds but health fails", () => {
    renderState({
      health: {
        phase: "error",
        error: new ApiClientError("network detail", { kind: "network" }),
      },
      readiness: { phase: "success", response: readinessResponse() },
    });

    expect(screen.getByText("无法确认 API 状态")).toBeTruthy();
    expect(screen.queryByText("已接入检查正常")).toBeNull();
    expect(screen.getByText("req-ready")).toBeTruthy();
  });

  it("allows an explicit refresh when no probe is currently loading", () => {
    renderState({
      health: { phase: "success", response: healthResponse() },
      readiness: { phase: "success", response: readinessResponse() },
    });

    fireEvent.click(screen.getByRole("button", { name: "重新检查" }));

    expect(refresh).toHaveBeenCalledOnce();
  });
});
