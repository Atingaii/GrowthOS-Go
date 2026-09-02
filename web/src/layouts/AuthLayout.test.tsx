// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router";

import { ApiClientError, type ApiResponse } from "../api/httpClient";
import {
  createSession,
  readCurrentSession,
  revokeCurrentSession,
  type SessionSnapshot,
} from "../api/sessionApi";
import { CurrentSessionPage } from "../pages/auth/CurrentSessionPage";
import { LoginPage } from "../pages/auth/LoginPage";
import { AuthLayout } from "./AuthLayout";

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

const csrfSentinel = "csrf-layout-test-sentinel";
const passwordSentinel = "password-layout-test-sentinel";

function snapshot(): SessionSnapshot {
  return {
    authenticated: true,
    principal: { kind: "human", id: "operator-1" },
    idleExpiresAt: "2026-09-02T10:15:00Z",
    absoluteExpiresAt: "2026-09-02T18:00:00Z",
    csrfToken: csrfSentinel,
  };
}

function response<T>(data: T, status = 200): ApiResponse<T> {
  return { data, status, requestId: "req-session", elapsedMs: 9 };
}

function apiError(status: number, code: string, requestId = "req-error"): ApiClientError {
  return new ApiClientError("raw backend detail must stay hidden", {
    kind: "http",
    status,
    code,
    requestId,
  });
}

function renderAuthRoute(pathname: "/login" | "/session") {
  const router = createMemoryRouter(
    [
      {
        path: "/",
        element: <AuthLayout />,
        children: [
          { path: "login", element: <LoginPage /> },
          { path: "session", element: <CurrentSessionPage /> },
        ],
      },
    ],
    { initialEntries: [pathname] },
  );

  return { router, ...render(<RouterProvider router={router} />) };
}

const mockedCreateSession = vi.mocked(createSession);
const mockedReadCurrentSession = vi.mocked(readCurrentSession);
const mockedRevokeCurrentSession = vi.mocked(revokeCurrentSession);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  document.documentElement.classList.remove("dark");
  document.body.style.overflow = "";
});

describe("AuthLayout session flow", () => {
  it("renders a labelled checking state before the current-session request settles", () => {
    mockedReadCurrentSession.mockReturnValue(new Promise<never>(() => undefined));

    const { container } = renderAuthRoute("/login");

    expect(container.querySelectorAll("main")).toHaveLength(1);
    expect(screen.getByRole("link", { name: "跳到主要内容" }).getAttribute("href")).toBe(
      "#auth-main-content",
    );
    expect(screen.getByRole("link", { name: "查看系统状态" })).toBeTruthy();
    expect(screen.getByRole("heading", { level: 1, name: /安全进入你的/ })).toBeTruthy();
    expect(screen.getByRole("status").textContent).toContain("正在向身份服务确认");
    expect(screen.getByRole("main").getAttribute("aria-busy")).toBe("true");
  });

  it("shows service failure instead of a login form and lets the user explicitly retry", async () => {
    mockedReadCurrentSession
      .mockRejectedValueOnce(apiError(503, "authentication_unavailable", "req-check"))
      .mockRejectedValueOnce(apiError(401, "unauthenticated", "req-anonymous"));
    renderAuthRoute("/login");

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("不会把技术故障当作未登录");
    expect(alert.textContent).toContain("req-check");
    expect(alert.textContent).not.toContain("raw backend detail");
    expect(document.activeElement).toBe(alert);
    expect(screen.queryByRole("form", { name: "登录 GrowthOS" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "重新核查" }));
    expect(await screen.findByRole("form", { name: "登录 GrowthOS" })).toBeTruthy();
    expect(mockedReadCurrentSession).toHaveBeenCalledTimes(2);
  });

  it("submits a real form, clears and hides the password immediately, then routes to the session", async () => {
    mockedReadCurrentSession.mockRejectedValue(apiError(401, "unauthenticated"));
    const loginRequest = deferred<ApiResponse<SessionSnapshot>>();
    mockedCreateSession.mockReturnValue(loginRequest.promise);
    const { router, container } = renderAuthRoute("/login");
    const form = await screen.findByRole("form", { name: "登录 GrowthOS" });
    const loginName = screen.getByRole("textbox", { name: "登录账号" }) as HTMLInputElement;
    const password = screen.getByLabelText("密码") as HTMLInputElement;

    expect(loginName.autocomplete).toBe("username");
    expect(password.autocomplete).toBe("current-password");
    fireEvent.change(loginName, { target: { value: "operator-1" } });
    fireEvent.change(password, { target: { value: passwordSentinel } });
    fireEvent.click(screen.getByRole("button", { name: "显示密码" }));
    expect(password.type).toBe("text");

    fireEvent.submit(form);

    expect(mockedCreateSession).toHaveBeenCalledTimes(1);
    expect(mockedCreateSession).toHaveBeenCalledWith(
      { loginName: "operator-1", password: passwordSentinel },
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(password.value).toBe("");
    expect(password.type).toBe("password");
    expect(form.getAttribute("aria-busy")).toBe("true");
    expect(container.textContent).not.toContain(passwordSentinel);

    loginRequest.resolve(response(snapshot(), 201));
    await waitFor(() => expect(router.state.location.pathname).toBe("/session"));
    const sessionHeading = await screen.findByRole("heading", { name: "当前会话" });
    expect(document.activeElement).toBe(sessionHeading);
    expect(container.textContent).toContain("operator-1");
    expect(container.textContent).not.toContain(csrfSentinel);
    expect(container.querySelectorAll("time")).toHaveLength(2);
  });

  it("maps login errors without exposing backend detail and returns focus to the summary", async () => {
    mockedReadCurrentSession.mockRejectedValue(apiError(401, "unauthenticated"));
    mockedCreateSession.mockRejectedValue(
      apiError(401, "authentication_failed", "req-login-failed"),
    );
    const { container } = renderAuthRoute("/login");
    const form = await screen.findByRole("form", { name: "登录 GrowthOS" });
    const loginName = screen.getByRole("textbox", { name: "登录账号" });
    const password = screen.getByLabelText("密码") as HTMLInputElement;
    fireEvent.change(loginName, { target: { value: "operator-1" } });
    fireEvent.change(password, { target: { value: passwordSentinel } });

    fireEvent.submit(form);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("账号或密码不正确");
    expect(alert.textContent).toContain("req-login-failed");
    expect(alert.textContent).not.toContain("raw backend detail");
    expect(document.activeElement).toBe(alert);
    expect(password.value).toBe("");
    expect(container.textContent).not.toContain(passwordSentinel);
  });

  it("renders only the public session projection and retains it after ordinary logout failure", async () => {
    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    mockedRevokeCurrentSession.mockRejectedValue(
      apiError(503, "authentication_unavailable", "req-logout-failed"),
    );
    const { router, container } = renderAuthRoute("/session");

    expect(await screen.findByText("operator-1")).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "当前会话" }));
    expect(container.textContent).not.toContain(csrfSentinel);
    expect(screen.getAllByRole("button").map((button) => button.textContent)).toEqual([
      "重新核查",
      "退出当前会话",
    ]);
    expect(screen.getByRole("button", { name: "重新核查" }).parentElement?.className).not.toContain(
      "flex-col-reverse",
    );
    fireEvent.click(screen.getByRole("button", { name: "退出当前会话" }));

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("当前会话仍保留");
    expect(alert.textContent).toContain("没有自动重试");
    expect(alert.textContent).not.toContain("raw backend detail");
    expect(document.activeElement).toBe(alert);
    expect(router.state.location.pathname).toBe("/session");
    expect(screen.getByText("operator-1")).toBeTruthy();
    expect(mockedRevokeCurrentSession).toHaveBeenCalledTimes(1);
    expect(mockedRevokeCurrentSession).toHaveBeenCalledWith(
      csrfSentinel,
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(container.textContent).not.toContain(csrfSentinel);
  });

  it("does not steal focus back from a session action during an ordinary rerender", async () => {
    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    const logoutRequest = deferred<ApiResponse<void>>();
    mockedRevokeCurrentSession.mockReturnValue(logoutRequest.promise);
    renderAuthRoute("/session");

    const heading = await screen.findByRole("heading", { name: "当前会话" });
    expect(document.activeElement).toBe(heading);
    const logoutButton = screen.getByRole("button", { name: "退出当前会话" });
    logoutButton.focus();

    fireEvent.click(logoutButton);

    expect(screen.getByRole("button", { name: "正在退出" })).toBe(logoutButton);
    expect(document.activeElement).toBe(logoutButton);

    await act(async () => {
      logoutRequest.reject(apiError(503, "authentication_unavailable", "req-focus"));
    });
    expect(document.activeElement).toBe(await screen.findByRole("alert"));
  });

  it("clears local session state and gives an honest warning for indeterminate revocation", async () => {
    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    mockedRevokeCurrentSession.mockRejectedValue(
      apiError(503, "session_revocation_indeterminate", "req-indeterminate"),
    );
    const { router, container } = renderAuthRoute("/session");
    await screen.findByText("operator-1");

    fireEvent.click(screen.getByRole("button", { name: "退出当前会话" }));

    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    const warning = await screen.findByRole("alert");
    expect(warning.textContent).toContain("服务端撤销状态未能确认");
    expect(warning.textContent).toContain("无法证明服务端 token 已完成撤销");
    expect(warning.textContent).not.toContain("安全退出");
    expect(document.activeElement).toBe(warning);
    expect(container.textContent).not.toContain(csrfSentinel);
    expect(screen.queryByText("operator-1")).toBeNull();
  });

  it("uses replace navigation between login and current-session routes", async () => {
    mockedReadCurrentSession.mockResolvedValue(response(snapshot()));
    const authenticated = renderAuthRoute("/login");
    await waitFor(() => expect(authenticated.router.state.location.pathname).toBe("/session"));
    expect(authenticated.router.state.historyAction).toBe("REPLACE");
    authenticated.unmount();

    mockedReadCurrentSession.mockRejectedValue(apiError(401, "unauthenticated"));
    const anonymous = renderAuthRoute("/session");
    await waitFor(() => expect(anonymous.router.state.location.pathname).toBe("/login"));
    expect(anonymous.router.state.historyAction).toBe("REPLACE");
  });
});
