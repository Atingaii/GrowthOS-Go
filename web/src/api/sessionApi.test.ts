import { afterEach, describe, expect, it, vi } from "vitest";
import type { FetchLike } from "./httpClient";
import { createSession, readCurrentSession, revokeCurrentSession, sessionPath } from "./sessionApi";

const passwordSentinel = "correct horse battery staple";
const csrfSentinel =
  "v1.active.ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq.abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG";

function sessionEnvelope(): Record<string, unknown> {
  return {
    data: {
      authenticated: true,
      principal: { kind: "human", id: "operator-1" },
      idle_expires_at: "2026-09-02T12:15:00.123456789+08:00",
      absolute_expires_at: "2026-09-02T20:00:00Z",
      csrf_token: csrfSentinel,
    },
  };
}

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json; charset=utf-8");
  return new Response(JSON.stringify(body), { ...init, headers });
}

function publicErrorResponse(status: number, code: string, requestId: string): Response {
  return jsonResponse(
    {
      error: {
        code,
        message: `public ${code}`,
        request_id: requestId,
      },
    },
    { status, headers: { "X-Request-ID": requestId } },
  );
}

function abortAwarePendingFetch(): FetchLike {
  return (_input, init) =>
    new Promise((_resolve, reject) => {
      const signal = init?.signal;
      if (signal?.aborted) {
        reject(new DOMException("aborted", "AbortError"));
        return;
      }
      signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), {
        once: true,
      });
    });
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("createSession", () => {
  it("posts the exact login contract through a bounded same-origin credential request", async () => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(
      jsonResponse(sessionEnvelope(), {
        status: 201,
        headers: { "X-Request-ID": "req-login" },
      }),
    );
    const now = vi.fn().mockReturnValueOnce(10).mockReturnValueOnce(16);

    await expect(
      createSession({ loginName: "operator-1", password: passwordSentinel }, { fetcher, now }),
    ).resolves.toEqual({
      data: {
        authenticated: true,
        principal: { kind: "human", id: "operator-1" },
        idleExpiresAt: "2026-09-02T12:15:00.123456789+08:00",
        absoluteExpiresAt: "2026-09-02T20:00:00Z",
        csrfToken: csrfSentinel,
      },
      status: 201,
      requestId: "req-login",
      elapsedMs: 6,
    });

    expect(fetcher).toHaveBeenCalledOnce();
    const [path, init] = fetcher.mock.calls[0];
    const headers = new Headers(init?.headers);
    expect(path).toBe(sessionPath);
    expect(path).toBe("/api/v1/session");
    expect(String(path)).not.toContain(passwordSentinel);
    expect(init).toMatchObject({
      method: "POST",
      cache: "no-store",
      credentials: "same-origin",
      mode: "same-origin",
      redirect: "error",
      signal: expect.any(AbortSignal),
      body: `{"login_name":"operator-1","password":"${passwordSentinel}"}`,
    });
    expect(headers.get("Accept")).toBe("application/json");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(Array.from(headers.entries()).join("\n")).not.toContain(passwordSentinel);
  });

  it.each([
    ["null", null],
    ["array", []],
    ["missing password", { loginName: "operator-1" }],
    ["extra authority field", { loginName: "operator-1", password: "secret", role: "admin" }],
    ["invalid login", { loginName: "Operator-1", password: "secret" }],
    ["empty password", { loginName: "operator-1", password: "" }],
    ["unpaired surrogate", { loginName: "operator-1", password: "\ud800" }],
    ["too many password code points", { loginName: "operator-1", password: "界".repeat(129) }],
    ["too many password bytes", { loginName: "operator-1", password: "🙂".repeat(129) }],
  ])("rejects %s before fetch", async (_name, input) => {
    const fetcher = vi.fn<FetchLike>();

    await expect(createSession(input as never, { fetcher })).rejects.toMatchObject({
      kind: "contract",
    });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("requires the exact 201 success status", async () => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(jsonResponse(sessionEnvelope(), { status: 200 }));

    await expect(
      createSession({ loginName: "operator-1", password: passwordSentinel }, { fetcher }),
    ).rejects.toMatchObject({ kind: "contract", status: 200 });
  });
});

describe("readCurrentSession", () => {
  it("reads the current cookie-backed session without a body or payload headers", async () => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(jsonResponse(sessionEnvelope(), { status: 200 }));

    await expect(readCurrentSession({ fetcher })).resolves.toMatchObject({
      data: {
        authenticated: true,
        principal: { kind: "human", id: "operator-1" },
        csrfToken: csrfSentinel,
      },
      status: 200,
    });

    const [path, init] = fetcher.mock.calls[0];
    const headers = new Headers(init?.headers);
    expect(path).toBe("/api/v1/session");
    expect(init).toMatchObject({
      method: "GET",
      cache: "no-store",
      credentials: "same-origin",
      mode: "same-origin",
      redirect: "error",
      signal: expect.any(AbortSignal),
    });
    expect(init).not.toHaveProperty("body");
    expect(headers.get("Accept")).toBe("application/json");
    expect(headers.has("Content-Type")).toBe(false);
  });

  it.each([
    ["null envelope", null],
    ["array envelope", []],
    ["missing outer data", {}],
    ["extra outer field", { ...sessionEnvelope(), meta: {} }],
    [
      "extra data field",
      (() => {
        const envelope = sessionEnvelope();
        return { data: { ...(envelope.data as object), role: "admin" } };
      })(),
    ],
    [
      "missing csrf token",
      (() => {
        const data = { ...(sessionEnvelope().data as Record<string, unknown>) };
        delete data.csrf_token;
        return { data };
      })(),
    ],
    [
      "false authenticated",
      { data: { ...(sessionEnvelope().data as object), authenticated: false } },
    ],
    [
      "non-human principal",
      {
        data: { ...(sessionEnvelope().data as object), principal: { kind: "service", id: "api" } },
      },
    ],
    [
      "extra principal field",
      {
        data: {
          ...(sessionEnvelope().data as object),
          principal: { kind: "human", id: "operator-1", role: "admin" },
        },
      },
    ],
    [
      "noncanonical principal",
      {
        data: {
          ...(sessionEnvelope().data as object),
          principal: { kind: "human", id: "Operator 1" },
        },
      },
    ],
    [
      "invalid calendar timestamp",
      { data: { ...(sessionEnvelope().data as object), idle_expires_at: "2026-02-30T00:00:00Z" } },
    ],
    [
      "parseable non-RFC3339 timestamp",
      { data: { ...(sessionEnvelope().data as object), absolute_expires_at: "2026-09-02" } },
    ],
    [
      "leap second outside server grammar",
      { data: { ...(sessionEnvelope().data as object), idle_expires_at: "2026-09-02T00:00:60Z" } },
    ],
    ["empty csrf", { data: { ...(sessionEnvelope().data as object), csrf_token: "" } }],
    ["unsafe csrf", { data: { ...(sessionEnvelope().data as object), csrf_token: "x\ny" } }],
  ])("rejects the strict response variant: %s", async (_name, body) => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(jsonResponse(body, { status: 200 }));

    await expect(readCurrentSession({ fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 200,
    });
  });

  it("requires the exact 200 success status", async () => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(jsonResponse(sessionEnvelope(), { status: 201 }));

    await expect(readCurrentSession({ fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 201,
    });
  });
});

describe("public failures", () => {
  it.each([
    [401, "unauthenticated"],
    [429, "authentication_throttled"],
    [503, "authentication_unavailable"],
  ])("maps the public %i %s envelope", async (status, code) => {
    const requestId = `req-${status}`;
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(publicErrorResponse(status, code, requestId));

    await expect(readCurrentSession({ fetcher, now: () => 20 })).rejects.toMatchObject({
      kind: "http",
      status,
      code,
      requestId,
      message: `public ${code}`,
      elapsedMs: 0,
    });
    expect(fetcher).toHaveBeenCalledOnce();
  });

  it("uses the body request ID when the optional header is absent", async () => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(
      jsonResponse(
        {
          error: {
            code: "unauthenticated",
            message: "authentication required",
            request_id: "req-body-only",
          },
        },
        { status: 401 },
      ),
    );

    await expect(readCurrentSession({ fetcher })).rejects.toMatchObject({
      kind: "http",
      status: 401,
      requestId: "req-body-only",
    });
  });

  it.each([
    [
      "mismatched request IDs",
      jsonResponse(
        {
          error: { code: "unauthenticated", message: "required", request_id: "req-body" },
        },
        { status: 401, headers: { "X-Request-ID": "req-header" } },
      ),
    ],
    [
      "missing request ID",
      jsonResponse({ error: { code: "unauthenticated", message: "required" } }, { status: 401 }),
    ],
    [
      "non-JSON API error",
      new Response("unauthenticated", { status: 401, headers: { "Content-Type": "text/plain" } }),
    ],
  ])("rejects %s as a contract failure", async (_name, response) => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(response);

    await expect(readCurrentSession({ fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 401,
    });
  });

  it.each([502, 503, 504])("classifies an HTML %i as a gateway failure", async (status) => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(
      new Response("<html>upstream unavailable</html>", {
        status,
        headers: { "Content-Type": "text/html", "X-Request-ID": `req-${status}` },
      }),
    );

    await expect(readCurrentSession({ fetcher, now: () => 12 })).rejects.toMatchObject({
      kind: "gateway",
      status,
      requestId: `req-${status}`,
      message: "代理无法连接上游 API",
      elapsedMs: 0,
    });
  });
});

describe("revokeCurrentSession", () => {
  it("sends the CSRF token only in the DELETE header and accepts exact empty 204", async () => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(
        new Response(null, { status: 204, headers: { "X-Request-ID": "req-out" } }),
      );

    await expect(revokeCurrentSession(csrfSentinel, { fetcher })).resolves.toMatchObject({
      data: undefined,
      status: 204,
      requestId: "req-out",
    });

    const [path, init] = fetcher.mock.calls[0];
    const headers = new Headers(init?.headers);
    expect(path).toBe("/api/v1/session");
    expect(String(path)).not.toContain(csrfSentinel);
    expect(init).toMatchObject({
      method: "DELETE",
      cache: "no-store",
      credentials: "same-origin",
      mode: "same-origin",
      redirect: "error",
      signal: expect.any(AbortSignal),
    });
    expect(init).not.toHaveProperty("body");
    expect(headers.get("X-CSRF-Token")).toBe(csrfSentinel);
    expect(headers.has("Content-Type")).toBe(false);
  });

  it.each(["", "   ", "line\nbreak", "x".repeat(513)])(
    "rejects the invalid CSRF token before fetch",
    async (csrfToken) => {
      const fetcher = vi.fn<FetchLike>();

      await expect(revokeCurrentSession(csrfToken, { fetcher })).rejects.toMatchObject({
        kind: "contract",
      });
      expect(fetcher).not.toHaveBeenCalled();
    },
  );

  it("maps unauthenticated logout and rejects a JSON 204 marker", async () => {
    const invalidNoContent = new Response(null, {
      status: 204,
      headers: { "Content-Type": "application/json" },
    });
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValueOnce(publicErrorResponse(401, "unauthenticated", "req-logout"))
      .mockResolvedValueOnce(invalidNoContent);

    await expect(revokeCurrentSession(csrfSentinel, { fetcher })).rejects.toMatchObject({
      kind: "http",
      status: 401,
      code: "unauthenticated",
    });
    await expect(revokeCurrentSession(csrfSentinel, { fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 204,
    });
  });
});

describe("session request lifecycle", () => {
  it("enforces the five-second default timeout and aborts the transport", async () => {
    vi.useFakeTimers();
    const fetcher = vi.fn<FetchLike>().mockImplementation(abortAwarePendingFetch());
    const promise = readCurrentSession({ fetcher });
    const assertion = expect(promise).rejects.toMatchObject({
      kind: "timeout",
      message: "请求超时",
    });

    await vi.advanceTimersByTimeAsync(4_999);
    expect(fetcher).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1);
    await assertion;
    expect(fetcher).toHaveBeenCalledOnce();
  });

  it("supports a shorter bounded timeout", async () => {
    vi.useFakeTimers();
    const promise = createSession(
      { loginName: "operator-1", password: passwordSentinel },
      { fetcher: abortAwarePendingFetch(), timeoutMs: 100 },
    );
    const assertion = expect(promise).rejects.toMatchObject({ kind: "timeout" });

    await vi.advanceTimersByTimeAsync(100);
    await assertion;
  });

  it.each([99, 5_001, 1.5])("rejects timeout %s before fetch", async (timeoutMs) => {
    const fetcher = vi.fn<FetchLike>();

    await expect(readCurrentSession({ fetcher, timeoutMs })).rejects.toMatchObject({
      kind: "contract",
    });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("propagates caller cancellation and does not fetch when already cancelled", async () => {
    const active = new AbortController();
    const fetcher = vi.fn<FetchLike>().mockImplementation(abortAwarePendingFetch());
    const promise = readCurrentSession({ fetcher, signal: active.signal });
    active.abort();

    await expect(promise).rejects.toMatchObject({ kind: "cancelled", message: "请求已取消" });
    expect(fetcher).toHaveBeenCalledOnce();

    const alreadyAborted = new AbortController();
    alreadyAborted.abort();
    const unusedFetcher = vi.fn<FetchLike>();
    await expect(
      readCurrentSession({ fetcher: unusedFetcher, signal: alreadyAborted.signal }),
    ).rejects.toMatchObject({ kind: "cancelled" });
    expect(unusedFetcher).not.toHaveBeenCalled();
  });

  it("classifies a rejected fetch as network failure without retrying", async () => {
    const fetcher = vi.fn<FetchLike>().mockRejectedValue(new TypeError("private network detail"));

    await expect(readCurrentSession({ fetcher })).rejects.toMatchObject({
      kind: "network",
      message: "无法连接服务",
    });
    expect(fetcher).toHaveBeenCalledOnce();
  });
});
