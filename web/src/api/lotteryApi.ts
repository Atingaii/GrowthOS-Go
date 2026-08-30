import {
  ApiClientError,
  postJSONWithoutBody,
  type ApiResponse,
  type FetchLike,
} from "./httpClient";

const maximumUint64 = "18446744073709551615";
const canonicalUint64Pattern = /^[1-9][0-9]{0,19}$/;
const demoModeHeader = "X-GrowthOS-Demo-Mode";
const ephemeralSelectionDemoMode = "ephemeral-selection";
const maximumAwardNameCodePoints = 128;
const edgeWhitespacePattern = /^\p{White_Space}|\p{White_Space}$/u;
const controlCharacterPattern = /\p{Cc}/u;

export interface EphemeralSelectionAward {
  id: string;
  name: string;
  outcome: "reward" | "no_reward";
}

export interface EphemeralSelectionResponse {
  durability: "ephemeral";
  strategyId: string;
  award: EphemeralSelectionAward;
}

export interface EphemeralSelectionRequestOptions {
  signal?: AbortSignal;
  timeoutMs?: number;
  fetcher?: FetchLike;
  now?: () => number;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function isCanonicalUint64ID(value: string): boolean {
  return (
    canonicalUint64Pattern.test(value) &&
    (value.length < maximumUint64.length || value <= maximumUint64)
  );
}

function validAwardName(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    value === "" ||
    edgeWhitespacePattern.test(value) ||
    controlCharacterPattern.test(value)
  ) {
    return false;
  }

  let codePoints = 0;
  for (const character of value) {
    const codePoint = character.codePointAt(0);
    if (codePoint === undefined || (codePoint >= 0xd800 && codePoint <= 0xdfff)) {
      return false;
    }
    codePoints += 1;
    if (codePoints > maximumAwardNameCodePoints) {
      return false;
    }
  }
  return true;
}

function decodeEphemeralSelection(
  value: unknown,
  expectedStrategyId: string,
): EphemeralSelectionResponse | null {
  if (!isRecord(value) || !isRecord(value.selection)) {
    return null;
  }

  const selection = value.selection;
  if (
    selection.durability !== "ephemeral" ||
    selection.strategy_id !== expectedStrategyId ||
    !isRecord(selection.award)
  ) {
    return null;
  }

  const award = selection.award;
  if (
    typeof award.id !== "string" ||
    !isCanonicalUint64ID(award.id) ||
    !validAwardName(award.name) ||
    (award.outcome !== "reward" && award.outcome !== "no_reward")
  ) {
    return null;
  }

  return {
    durability: "ephemeral",
    strategyId: expectedStrategyId,
    award: {
      id: award.id,
      name: award.name,
      outcome: award.outcome,
    },
  };
}

export async function requestEphemeralSelection(
  strategyId: string,
  options: EphemeralSelectionRequestOptions = {},
): Promise<ApiResponse<EphemeralSelectionResponse>> {
  if (!isCanonicalUint64ID(strategyId)) {
    throw new ApiClientError("抽奖策略 ID 必须是规范的 uint64 十进制字符串", {
      kind: "contract",
    });
  }

  return postJSONWithoutBody(`/api/v1/lottery/strategies/${strategyId}/ephemeral-selections`, {
    ...options,
    headers: { [demoModeHeader]: ephemeralSelectionDemoMode },
    decode: (value) => decodeEphemeralSelection(value, strategyId),
  });
}
