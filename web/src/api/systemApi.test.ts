import { describe, expect, it, vi } from "vitest";
import { fetchHealth, fetchReadiness, healthPath, readinessPath } from "./systemApi";
import type { FetchLike } from "./httpClient";

function probeResponse(status: "ok" | "ready"): Response {
  return new Response(
    JSON.stringify({
      status,
      version: "lesson-15",
      timestamp: "2026-08-29T08:00:00Z",
    }),
    {
      status: 200,
      headers: {
        "Content-Type": "application/json",
        "X-Request-ID": `req-${status}`,
      },
    },
  );
}

describe("systemApi", () => {
  it("uses the exact root /health and /ready paths without an invented /api prefix", async () => {
    const fetcher = vi.fn<FetchLike>().mockImplementation((input) => {
      if (input === healthPath) return Promise.resolve(probeResponse("ok"));
      if (input === readinessPath) return Promise.resolve(probeResponse("ready"));
      return Promise.reject(new Error(`unexpected path: ${String(input)}`));
    });

    const [health, readiness] = await Promise.all([
      fetchHealth({ fetcher }),
      fetchReadiness({ fetcher }),
    ]);

    expect(healthPath).toBe("/health");
    expect(readinessPath).toBe("/ready");
    expect(fetcher.mock.calls.map(([path]) => path)).toEqual(["/health", "/ready"]);
    expect(health.data.status).toBe("ok");
    expect(readiness.data.status).toBe("ready");
  });

  it("runtime-validates the probe status, version, and timestamp", async () => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(
      new Response(
        JSON.stringify({
          status: "ok",
          version: "",
          timestamp: "not-a-timestamp",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(fetchHealth({ fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 200,
    });
  });

  it("accepts additive fields and an RFC 3339 timestamp with nanoseconds", async () => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(
      new Response(
        JSON.stringify({
          status: "ok",
          version: "lesson-15",
          timestamp: "2026-08-29T08:00:00.123456789Z",
          future_field: "ignored",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(fetchHealth({ fetcher })).resolves.toMatchObject({
      data: {
        status: "ok",
        version: "lesson-15",
        timestamp: "2026-08-29T08:00:00.123456789Z",
      },
    });
  });

  it("rejects a parseable date that is not an RFC 3339 timestamp", async () => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(
        new Response(
          JSON.stringify({ status: "ok", version: "lesson-15", timestamp: "2026-08-29" }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );

    await expect(fetchHealth({ fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 200,
    });
  });
});
