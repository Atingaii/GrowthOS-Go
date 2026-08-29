import { afterEach, describe, expect, it, vi } from "vitest";
import { requestJSON, type FetchLike } from "./httpClient";

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json; charset=utf-8");
  return new Response(JSON.stringify(body), { ...init, headers });
}

function decodeName(value: unknown): { name: string } | null {
  if (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    typeof (value as Record<string, unknown>).name === "string"
  ) {
    return { name: (value as Record<string, string>).name };
  }
  return null;
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
});

describe("requestJSON", () => {
  it("returns decoded data, correlation metadata, and safe same-origin request options", async () => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(
      jsonResponse(
        { name: "GrowthOS" },
        {
          status: 200,
          headers: { "X-Request-ID": "req-success" },
        },
      ),
    );
    const now = vi.fn().mockReturnValueOnce(10).mockReturnValueOnce(24);

    const result = await requestJSON("/health", {
      decode: decodeName,
      fetcher,
      now,
    });

    expect(result).toEqual({
      data: { name: "GrowthOS" },
      status: 200,
      requestId: "req-success",
      elapsedMs: 14,
    });
    expect(fetcher).toHaveBeenCalledOnce();
    expect(fetcher).toHaveBeenCalledWith(
      "/health",
      expect.objectContaining({
        method: "GET",
        cache: "no-store",
        credentials: "same-origin",
        mode: "same-origin",
        redirect: "error",
        headers: { Accept: "application/json" },
        signal: expect.any(AbortSignal),
      }),
    );
  });

  it("maps a valid HTTP 503 error envelope without leaking transport details", async () => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(
      jsonResponse(
        {
          error: {
            code: "dependency_unavailable",
            message: "MySQL dependency is unavailable",
            request_id: "req-503",
          },
        },
        { status: 503, headers: { "X-Request-ID": "req-503" } },
      ),
    );

    await expect(
      requestJSON("/ready", { decode: decodeName, fetcher, now: () => 20 }),
    ).rejects.toMatchObject({
      name: "ApiClientError",
      kind: "http",
      status: 503,
      code: "dependency_unavailable",
      requestId: "req-503",
      elapsedMs: 0,
      message: "MySQL dependency is unavailable",
    });
  });

  it("rejects a successful response that violates the runtime contract", async () => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(jsonResponse({ unexpected: true }, { status: 200 }));

    await expect(requestJSON("/health", { decode: decodeName, fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 200,
    });
  });

  it.each([
    [
      "non-JSON content type",
      () => new Response("GrowthOS", { status: 200, headers: { "Content-Type": "text/plain" } }),
    ],
    [
      "malformed JSON",
      () => new Response("{", { status: 200, headers: { "Content-Type": "application/json" } }),
    ],
  ])("classifies a successful %s response as a contract failure", async (_name, response) => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(response());

    await expect(requestJSON("/health", { decode: decodeName, fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 200,
    });
  });

  it("classifies a decoder exception as a contract failure", async () => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(jsonResponse({ name: "GrowthOS" }, { status: 200 }));

    await expect(
      requestJSON("/health", {
        decode: () => {
          throw new Error("decoder implementation detail");
        },
        fetcher,
      }),
    ).rejects.toMatchObject({ kind: "contract", status: 200 });
  });

  it("rejects conflicting header and body request IDs as a contract failure", async () => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(
      jsonResponse(
        {
          error: {
            code: "dependency_unavailable",
            message: "not ready",
            request_id: "req-body",
          },
        },
        { status: 503, headers: { "X-Request-ID": "req-header" } },
      ),
    );

    await expect(requestJSON("/ready", { decode: decodeName, fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 503,
      requestId: "req-header",
    });
  });

  it("classifies a rejected fetch as a network failure", async () => {
    const fetcher = vi.fn<FetchLike>().mockRejectedValue(new TypeError("connection refused"));

    await expect(requestJSON("/health", { decode: decodeName, fetcher })).rejects.toMatchObject({
      kind: "network",
      message: "无法连接服务",
    });
  });

  it("classifies a non-JSON gateway response without confusing it with an API contract", async () => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(
        new Response("", { status: 502, headers: { "Content-Type": "text/plain" } }),
      );

    await expect(
      requestJSON("/health", { decode: decodeName, fetcher, now: () => 20 }),
    ).rejects.toMatchObject({
      kind: "gateway",
      status: 502,
      message: "代理无法连接上游 API",
      elapsedMs: 0,
    });
  });

  it("requires a JSON gateway response to satisfy the API error envelope", async () => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(jsonResponse({}, { status: 502 }));

    await expect(requestJSON("/health", { decode: decodeName, fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 502,
    });
  });

  it("aborts and classifies a request when the deadline expires", async () => {
    vi.useFakeTimers();
    const promise = requestJSON("/health", {
      decode: decodeName,
      fetcher: abortAwarePendingFetch(),
      timeoutMs: 100,
    });
    const assertion = expect(promise).rejects.toMatchObject({
      kind: "timeout",
      message: "请求超时",
    });

    await vi.advanceTimersByTimeAsync(100);
    await assertion;
  });

  it("propagates an external AbortSignal as cancellation", async () => {
    const controller = new AbortController();
    const promise = requestJSON("/ready", {
      decode: decodeName,
      fetcher: abortAwarePendingFetch(),
      signal: controller.signal,
    });

    controller.abort();

    await expect(promise).rejects.toMatchObject({
      kind: "cancelled",
      message: "请求已取消",
    });
  });

  it("rejects unsafe paths and an already-aborted caller without sending a request", async () => {
    const fetcher = vi.fn<FetchLike>();

    await expect(
      requestJSON("/\\evil.example/path", { decode: decodeName, fetcher }),
    ).rejects.toMatchObject({ kind: "contract" });
    expect(fetcher).not.toHaveBeenCalled();

    const controller = new AbortController();
    controller.abort();
    await expect(
      requestJSON("/health", {
        decode: decodeName,
        fetcher: abortAwarePendingFetch(),
        signal: controller.signal,
      }),
    ).rejects.toMatchObject({ kind: "cancelled" });
  });

  it.each(["https://evil.example/health", "//evil.example/health", "health"])(
    "rejects the unsafe request path %s before fetch",
    async (path) => {
      const fetcher = vi.fn<FetchLike>();

      await expect(requestJSON(path, { decode: decodeName, fetcher })).rejects.toMatchObject({
        kind: "contract",
      });
      expect(fetcher).not.toHaveBeenCalled();
    },
  );

  it.each([99, 30_001, 1.5])("rejects the invalid timeout %s before fetch", async (timeoutMs) => {
    const fetcher = vi.fn<FetchLike>();

    await expect(
      requestJSON("/health", { decode: decodeName, fetcher, timeoutMs }),
    ).rejects.toMatchObject({ kind: "contract" });
    expect(fetcher).not.toHaveBeenCalled();
  });
});
