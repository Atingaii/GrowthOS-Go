// @vitest-environment jsdom

import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiClientError, type ApiResponse } from "../api/httpClient";
import {
  createSession,
  readCurrentSession,
  revokeCurrentSession,
  type SessionSnapshot,
} from "../api/sessionApi";
import { useSessionBoundary } from "./useSessionBoundary";

vi.mock("../api/sessionApi", async (importOriginal) => {
  const original = await importOriginal<typeof import("../api/sessionApi")>();
  return {
    ...original,
    createSession: vi.fn(),
    readCurrentSession: vi.fn(),
    revokeCurrentSession: vi.fn(),
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

const csrfSentinel = "csrf-unit-test-sentinel";

function snapshot(id = "operator-1", csrfToken = csrfSentinel): SessionSnapshot {
  return {
    authenticated: true,
    principal: { kind: "human", id },
    idleExpiresAt: "2026-09-02T10:15:00Z",
    absoluteExpiresAt: "2026-09-02T18:00:00Z",
    csrfToken,
  };
}

function response<T>(data: T, status = 200): ApiResponse<T> {
  return { data, status, requestId: "req-session", elapsedMs: 8 };
}

function unauthenticatedError(): ApiClientError {
  return new ApiClientError("raw unauthenticated detail", {
    kind: "http",
    status: 401,
    code: "unauthenticated",
    requestId: "req-anonymous",
  });
}

const mockedCreateSession = vi.mocked(createSession);
const mockedReadCurrentSession = vi.mocked(readCurrentSession);
const mockedRevokeCurrentSession = vi.mocked(revokeCurrentSession);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useSessionBoundary", () => {
  it("distinguishes an authenticated snapshot from an anonymous response", async () => {
    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    const authenticated = renderHook(() => useSessionBoundary());

    expect(authenticated.result.current.sessionState.phase).toBe("checking");
    await waitFor(() =>
      expect(authenticated.result.current.sessionState).toMatchObject({
        phase: "authenticated",
        session: { principal: { id: "operator-1" } },
      }),
    );
    authenticated.unmount();

    mockedReadCurrentSession.mockRejectedValue(unauthenticatedError());
    const anonymous = renderHook(() => useSessionBoundary());
    await waitFor(() =>
      expect(anonymous.result.current.sessionState).toEqual({ phase: "anonymous" }),
    );
  });

  it("does not misclassify service failure as an anonymous browser", async () => {
    const unavailable = new ApiClientError("raw dependency detail", {
      kind: "http",
      status: 503,
      code: "authentication_unavailable",
      requestId: "req-unavailable",
    });
    mockedReadCurrentSession.mockRejectedValue(unavailable);

    const { result } = renderHook(() => useSessionBoundary());

    await waitFor(() => expect(result.current.sessionState.phase).toBe("unavailable"));
    expect(result.current.sessionState).toEqual({ phase: "unavailable", error: unavailable });
  });

  it("aborts an old current-session generation and ignores its stale completion", async () => {
    const oldRequest = deferred<ApiResponse<SessionSnapshot>>();
    const newRequest = deferred<ApiResponse<SessionSnapshot>>();
    mockedReadCurrentSession
      .mockReturnValueOnce(oldRequest.promise)
      .mockReturnValueOnce(newRequest.promise);

    const { result } = renderHook(() => useSessionBoundary());
    const oldSignal = mockedReadCurrentSession.mock.calls[0]?.[0]?.signal;

    act(() => result.current.retryCurrentSession());
    expect(oldSignal?.aborted).toBe(true);

    await act(async () => newRequest.resolve(response(snapshot("new-operator"))));
    await waitFor(() =>
      expect(result.current.sessionState).toMatchObject({
        phase: "authenticated",
        session: { principal: { id: "new-operator" } },
      }),
    );

    await act(async () => oldRequest.resolve(response(snapshot("stale-operator"))));
    expect(result.current.sessionState).toMatchObject({
      phase: "authenticated",
      session: { principal: { id: "new-operator" } },
    });
  });

  it("allows only one login attempt and publishes the returned session", async () => {
    mockedReadCurrentSession.mockRejectedValue(unauthenticatedError());
    const loginRequest = deferred<ApiResponse<SessionSnapshot>>();
    mockedCreateSession.mockReturnValue(loginRequest.promise);
    const { result } = renderHook(() => useSessionBoundary());
    await waitFor(() => expect(result.current.sessionState.phase).toBe("anonymous"));

    let firstAttempt: Promise<boolean> | undefined;
    let duplicateAttempt: Promise<boolean> | undefined;
    act(() => {
      firstAttempt = result.current.signIn({ loginName: "operator-1", password: "secret-1" });
      duplicateAttempt = result.current.signIn({ loginName: "operator-1", password: "secret-2" });
      result.current.retryCurrentSession();
    });

    expect(mockedCreateSession).toHaveBeenCalledTimes(1);
    expect(mockedReadCurrentSession).toHaveBeenCalledTimes(1);
    expect(result.current.loginState.phase).toBe("submitting");
    await expect(duplicateAttempt).resolves.toBe(false);

    await act(async () => loginRequest.resolve(response(snapshot(), 201)));
    await expect(firstAttempt).resolves.toBe(true);
    expect(result.current.sessionState.phase).toBe("authenticated");
    expect(JSON.stringify(result.current)).not.toContain("secret-1");
    expect(JSON.stringify(result.current)).not.toContain("secret-2");
  });

  it("clears component session state after confirmed or already-ended logout", async () => {
    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    mockedRevokeCurrentSession.mockResolvedValue(response(undefined, 204));
    const confirmed = renderHook(() => useSessionBoundary());
    await waitFor(() => expect(confirmed.result.current.sessionState.phase).toBe("authenticated"));

    await act(async () => {
      await confirmed.result.current.signOut();
    });
    expect(confirmed.result.current.sessionState).toEqual({
      phase: "anonymous",
      reason: "signed-out",
    });
    expect(JSON.stringify(confirmed.result.current)).not.toContain(csrfSentinel);
    confirmed.unmount();

    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    mockedRevokeCurrentSession.mockRejectedValue(unauthenticatedError());
    const ended = renderHook(() => useSessionBoundary());
    await waitFor(() => expect(ended.result.current.sessionState.phase).toBe("authenticated"));
    await act(async () => {
      await ended.result.current.signOut();
    });
    expect(ended.result.current.sessionState).toEqual({
      phase: "anonymous",
      reason: "session-ended",
    });
  });

  it("clears local credentials but records an honest warning when revocation is indeterminate", async () => {
    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    mockedRevokeCurrentSession.mockRejectedValue(
      new ApiClientError("raw commit outcome detail", {
        kind: "http",
        status: 503,
        code: "session_revocation_indeterminate",
      }),
    );
    const { result } = renderHook(() => useSessionBoundary());
    await waitFor(() => expect(result.current.sessionState.phase).toBe("authenticated"));

    let confirmed: boolean | undefined;
    await act(async () => {
      confirmed = await result.current.signOut();
    });

    expect(confirmed).toBe(false);
    expect(result.current.sessionState).toEqual({
      phase: "anonymous",
      reason: "revocation-indeterminate",
    });
    expect(JSON.stringify(result.current)).not.toContain(csrfSentinel);
    expect(mockedRevokeCurrentSession).toHaveBeenCalledTimes(1);
  });

  it("survives StrictMode effect cleanup without accepting the first stale check", async () => {
    const firstRequest = deferred<ApiResponse<SessionSnapshot>>();
    const secondRequest = deferred<ApiResponse<SessionSnapshot>>();
    mockedReadCurrentSession
      .mockReturnValueOnce(firstRequest.promise)
      .mockReturnValueOnce(secondRequest.promise);

    const { result, unmount } = renderHook(() => useSessionBoundary(), {
      reactStrictMode: true,
    });

    expect(mockedReadCurrentSession).toHaveBeenCalledTimes(2);
    expect(mockedReadCurrentSession.mock.calls[0]?.[0]?.signal?.aborted).toBe(true);
    expect(mockedReadCurrentSession.mock.calls[1]?.[0]?.signal?.aborted).toBe(false);

    await act(async () => secondRequest.resolve(response(snapshot("live-operator"))));
    await waitFor(() =>
      expect(result.current.sessionState).toMatchObject({
        phase: "authenticated",
        session: { principal: { id: "live-operator" } },
      }),
    );
    await act(async () => firstRequest.resolve(response(snapshot("stale-operator"))));
    expect(result.current.sessionState).toMatchObject({
      phase: "authenticated",
      session: { principal: { id: "live-operator" } },
    });

    unmount();
  });

  it("retains the session and does not retry after an ordinary logout failure", async () => {
    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    const unavailable = new ApiClientError("raw mysql detail", {
      kind: "http",
      status: 503,
      code: "authentication_unavailable",
      requestId: "req-logout",
    });
    mockedRevokeCurrentSession.mockRejectedValue(unavailable);
    const { result } = renderHook(() => useSessionBoundary());
    await waitFor(() => expect(result.current.sessionState.phase).toBe("authenticated"));

    await act(async () => {
      await result.current.signOut();
    });

    expect(result.current.sessionState).toMatchObject({
      phase: "authenticated",
      session: { csrfToken: csrfSentinel },
    });
    expect(result.current.logoutState).toEqual({ phase: "error", error: unavailable });
    expect(mockedRevokeCurrentSession).toHaveBeenCalledTimes(1);
  });

  it("lets logout own the transition and refuses a later check until it completes", async () => {
    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    const logoutRequest = deferred<ApiResponse<void>>();
    mockedRevokeCurrentSession.mockReturnValue(logoutRequest.promise);
    const { result } = renderHook(() => useSessionBoundary());
    await waitFor(() => expect(result.current.sessionState.phase).toBe("authenticated"));

    let logoutResult: Promise<boolean> | undefined;
    act(() => {
      logoutResult = result.current.signOut();
      result.current.retryCurrentSession();
    });

    expect(result.current.logoutState.phase).toBe("logging-out");
    expect(mockedReadCurrentSession).toHaveBeenCalledTimes(1);
    expect(mockedRevokeCurrentSession).toHaveBeenCalledTimes(1);

    await act(async () => logoutRequest.resolve(response(undefined, 204)));
    await expect(logoutResult).resolves.toBe(true);
    expect(result.current.sessionState).toEqual({
      phase: "anonymous",
      reason: "signed-out",
    });
    expect(JSON.stringify(result.current)).not.toContain(csrfSentinel);
  });

  it("lets an earlier check own the transition and refuses logout in the opposite order", async () => {
    const retryRequest = deferred<ApiResponse<SessionSnapshot>>();
    mockedReadCurrentSession
      .mockResolvedValueOnce(response(snapshot()))
      .mockReturnValueOnce(retryRequest.promise);
    const { result } = renderHook(() => useSessionBoundary());
    await waitFor(() => expect(result.current.sessionState.phase).toBe("authenticated"));

    act(() => result.current.retryCurrentSession());
    const retrySignal = mockedReadCurrentSession.mock.calls[1]?.[0]?.signal;

    let logoutConfirmed: boolean | undefined;
    await act(async () => {
      logoutConfirmed = await result.current.signOut();
    });

    expect(logoutConfirmed).toBe(false);
    expect(mockedRevokeCurrentSession).not.toHaveBeenCalled();
    expect(retrySignal?.aborted).toBe(false);

    await act(async () => retryRequest.resolve(response(snapshot("fresh-operator"))));
    expect(result.current.sessionState).toMatchObject({
      phase: "authenticated",
      session: { principal: { id: "fresh-operator" } },
    });
  });

  it("aborts live checks and actions when its AuthLayout owner unmounts", async () => {
    const currentRequest = deferred<ApiResponse<SessionSnapshot>>();
    mockedReadCurrentSession.mockReturnValue(currentRequest.promise);
    const checking = renderHook(() => useSessionBoundary());
    const checkSignal = mockedReadCurrentSession.mock.calls[0]?.[0]?.signal;
    checking.unmount();
    expect(checkSignal?.aborted).toBe(true);

    mockedReadCurrentSession.mockRejectedValue(unauthenticatedError());
    const loginRequest = deferred<ApiResponse<SessionSnapshot>>();
    mockedCreateSession.mockReturnValue(loginRequest.promise);
    const loggingIn = renderHook(() => useSessionBoundary());
    await waitFor(() => expect(loggingIn.result.current.sessionState.phase).toBe("anonymous"));
    act(() => {
      void loggingIn.result.current.signIn({ loginName: "operator-1", password: "secret" });
    });
    const actionSignal = mockedCreateSession.mock.calls[0]?.[1]?.signal;
    loggingIn.unmount();
    expect(actionSignal?.aborted).toBe(true);
  });
});
