import { describe, expect, it, vi } from "vitest";
import {
  isCanonicalUint64ID,
  requestEphemeralSelection,
  type EphemeralSelectionResponse,
} from "./lotteryApi";
import type { FetchLike } from "./httpClient";

const maximumUint64 = "18446744073709551615";

function selectionPayload(
  overrides: Record<string, unknown> = {},
  awardOverrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    selection: {
      durability: "ephemeral",
      strategy_id: maximumUint64,
      award: {
        id: "9007199254740993",
        name: "成长值礼包",
        outcome: "reward",
        ...awardOverrides,
      },
      ...overrides,
    },
  };
}

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json; charset=utf-8");
  return new Response(JSON.stringify(body), { ...init, headers });
}

describe("isCanonicalUint64ID", () => {
  it.each(["1", "9", "10", "9007199254740993", maximumUint64])(
    "accepts the canonical uint64 string %s without numeric coercion",
    (value) => {
      expect(isCanonicalUint64ID(value)).toBe(true);
    },
  );

  it.each([
    "",
    "0",
    "00",
    "01",
    "+1",
    "-1",
    "1.0",
    "1e3",
    " 1",
    "1 ",
    "１２",
    "18446744073709551616",
    "99999999999999999999",
    "100000000000000000000",
  ])("rejects the non-canonical or out-of-range ID %j", (value) => {
    expect(isCanonicalUint64ID(value)).toBe(false);
  });
});

describe("requestEphemeralSelection", () => {
  it("sends the exact bodyless POST and preserves uint64 IDs as strings", async () => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(
      jsonResponse(
        {
          ...selectionPayload(),
          future_top_level: true,
        },
        { status: 200, headers: { "X-Request-ID": "req-selection" } },
      ),
    );
    const now = vi.fn().mockReturnValueOnce(10).mockReturnValueOnce(25);

    const result = await requestEphemeralSelection(maximumUint64, { fetcher, now });

    expect(result).toEqual({
      data: {
        durability: "ephemeral",
        strategyId: maximumUint64,
        award: {
          id: "9007199254740993",
          name: "成长值礼包",
          outcome: "reward",
        },
      } satisfies EphemeralSelectionResponse,
      status: 200,
      requestId: "req-selection",
      elapsedMs: 15,
    });
    expect(fetcher).toHaveBeenCalledOnce();

    const [path, init] = fetcher.mock.calls[0];
    const headers = new Headers(init?.headers);
    expect(path).toBe(`/api/v1/lottery/strategies/${maximumUint64}/ephemeral-selections`);
    expect(String(path)).not.toContain("?");
    expect(String(path)).not.toContain("#");
    expect(init?.method).toBe("POST");
    expect(init).not.toHaveProperty("body");
    expect(headers.get("Accept")).toBe("application/json");
    expect(headers.get("X-GrowthOS-Demo-Mode")).toBe("ephemeral-selection");
    expect(headers.has("Content-Type")).toBe(false);
    expect(headers.has("Idempotency-Key")).toBe(false);
    expect([...headers.keys()]).toHaveLength(2);
  });

  it.each(["reward", "no_reward"] as const)("accepts the closed %s outcome", async (outcome) => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(jsonResponse(selectionPayload({}, { outcome }), { status: 200 }));

    await expect(requestEphemeralSelection(maximumUint64, { fetcher })).resolves.toMatchObject({
      data: { award: { outcome } },
    });
  });

  it("accepts the 128-code-point award name boundary without counting UTF-16 code units", async () => {
    const name = "🎁".repeat(128);
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(jsonResponse(selectionPayload({}, { name }), { status: 200 }));

    await expect(requestEphemeralSelection(maximumUint64, { fetcher })).resolves.toMatchObject({
      data: { award: { name } },
    });
  });

  it("allows additive fields while returning only the public frontend DTO", async () => {
    const fetcher = vi
      .fn<FetchLike>()
      .mockResolvedValue(
        jsonResponse(
          selectionPayload(
            { future_selection_field: { ignored: true } },
            { future_award_field: 42 },
          ),
          { status: 200 },
        ),
      );

    await expect(requestEphemeralSelection(maximumUint64, { fetcher })).resolves.toEqual({
      data: {
        durability: "ephemeral",
        strategyId: maximumUint64,
        award: {
          id: "9007199254740993",
          name: "成长值礼包",
          outcome: "reward",
        },
      },
      status: 200,
      requestId: undefined,
      elapsedMs: expect.any(Number),
    });
  });

  it.each([
    ["missing selection", {}],
    ["array selection", { selection: [] }],
    ["wrong durability", selectionPayload({ durability: "ephemeral-selection" })],
    ["missing durability", selectionPayload({ durability: undefined })],
    ["numeric strategy ID", selectionPayload({ strategy_id: 1 })],
    ["mismatched strategy ID", selectionPayload({ strategy_id: "1" })],
    ["missing award", selectionPayload({ award: undefined })],
    ["array award", selectionPayload({ award: [] })],
    ["numeric award ID", selectionPayload({}, { id: 1 })],
    ["zero award ID", selectionPayload({}, { id: "0" })],
    ["leading-zero award ID", selectionPayload({}, { id: "01" })],
    ["overflowing award ID", selectionPayload({}, { id: "18446744073709551616" })],
    ["numeric award name", selectionPayload({}, { name: 7 })],
    ["empty award name", selectionPayload({}, { name: "" })],
    ["edge-whitespace award name", selectionPayload({}, { name: " Reward" })],
    ["control-character award name", selectionPayload({}, { name: "Re\u0000ward" })],
    ["oversized award name", selectionPayload({}, { name: "奖".repeat(129) })],
    ["invalid UTF-16 award name", selectionPayload({}, { name: "\ud800" })],
    ["unknown outcome", selectionPayload({}, { outcome: "pending" })],
    ["missing outcome", selectionPayload({}, { outcome: undefined })],
  ])("rejects a success payload with %s", async (_name, payload) => {
    const fetcher = vi.fn<FetchLike>().mockResolvedValue(jsonResponse(payload, { status: 200 }));

    await expect(requestEphemeralSelection(maximumUint64, { fetcher })).rejects.toMatchObject({
      kind: "contract",
      status: 200,
    });
    expect(fetcher).toHaveBeenCalledOnce();
  });

  it.each(["", "0", "01", "-1", "9007199254740993.0", "18446744073709551616"])(
    "rejects invalid local strategy ID %j before transport",
    async (strategyId) => {
      const fetcher = vi.fn<FetchLike>();

      await expect(requestEphemeralSelection(strategyId, { fetcher })).rejects.toMatchObject({
        kind: "contract",
        status: undefined,
      });
      expect(fetcher).not.toHaveBeenCalled();
    },
  );

  it("does not retry an uncertain POST failure", async () => {
    const fetcher = vi.fn<FetchLike>().mockRejectedValue(new TypeError("connection reset"));

    await expect(requestEphemeralSelection("1", { fetcher })).rejects.toMatchObject({
      kind: "network",
    });
    expect(fetcher).toHaveBeenCalledOnce();
  });
});
