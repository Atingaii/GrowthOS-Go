import { requestJSON, type ApiResponse, type FetchLike } from "./httpClient";

export const healthPath = "/health";
export const readinessPath = "/ready";

export interface HealthResponse {
  status: "ok";
  version: string;
  timestamp: string;
}

export interface ReadinessResponse {
  status: "ready";
  version: string;
  timestamp: string;
}

export interface ProbeRequestOptions {
  signal?: AbortSignal;
  timeoutMs?: number;
  fetcher?: FetchLike;
  now?: () => number;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function validTimestamp(value: unknown): value is string {
  const rfc3339 = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;
  return (
    typeof value === "string" &&
    value.trim() !== "" &&
    rfc3339.test(value) &&
    !Number.isNaN(Date.parse(value))
  );
}

interface ProbePayload<TStatus extends string> {
  status: TStatus;
  version: string;
  timestamp: string;
}

function decodeProbe<TStatus extends string>(
  value: unknown,
  expectedStatus: TStatus,
): ProbePayload<TStatus> | null {
  if (
    !isRecord(value) ||
    value.status !== expectedStatus ||
    typeof value.version !== "string" ||
    value.version.trim() === "" ||
    !validTimestamp(value.timestamp)
  ) {
    return null;
  }

  return {
    status: expectedStatus,
    version: value.version,
    timestamp: value.timestamp,
  };
}

export function fetchHealth(
  options: ProbeRequestOptions = {},
): Promise<ApiResponse<HealthResponse>> {
  return requestJSON(healthPath, {
    ...options,
    decode: (value) => decodeProbe(value, "ok"),
  });
}

export function fetchReadiness(
  options: ProbeRequestOptions = {},
): Promise<ApiResponse<ReadinessResponse>> {
  return requestJSON(readinessPath, {
    ...options,
    decode: (value) => decodeProbe(value, "ready"),
  });
}
