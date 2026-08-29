export type ApiFailureKind = "http" | "gateway" | "network" | "timeout" | "cancelled" | "contract";

export interface ApiErrorDetails {
  kind: ApiFailureKind;
  status?: number;
  code?: string;
  requestId?: string;
  elapsedMs?: number;
}

export class ApiClientError extends Error {
  readonly kind: ApiFailureKind;
  readonly status?: number;
  readonly code?: string;
  readonly requestId?: string;
  readonly elapsedMs?: number;

  constructor(message: string, details: ApiErrorDetails) {
    super(message);
    this.name = "ApiClientError";
    this.kind = details.kind;
    this.status = details.status;
    this.code = details.code;
    this.requestId = details.requestId;
    this.elapsedMs = details.elapsedMs;
  }
}

export interface ApiResponse<T> {
  data: T;
  status: number;
  requestId?: string;
  elapsedMs: number;
}

export type RuntimeDecoder<T> = (value: unknown) => T | null;
export type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export interface JsonRequestOptions<T> {
  decode: RuntimeDecoder<T>;
  signal?: AbortSignal;
  timeoutMs?: number;
  fetcher?: FetchLike;
  now?: () => number;
}

const defaultTimeoutMs = 5_000;
const requestIDHeader = "X-Request-ID";

interface PublicErrorEnvelope {
  error: {
    code: string;
    message: string;
    request_id: string;
  };
}

function monotonicNow(): number {
  return typeof performance === "undefined" ? Date.now() : performance.now();
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function nonemptyString(value: unknown): value is string {
  return typeof value === "string" && value.trim() !== "";
}

function decodePublicError(value: unknown): PublicErrorEnvelope | null {
  if (!isRecord(value) || !isRecord(value.error)) {
    return null;
  }

  const body = value.error;
  if (
    !nonemptyString(body.code) ||
    !nonemptyString(body.message) ||
    !nonemptyString(body.request_id)
  ) {
    return null;
  }

  return {
    error: {
      code: body.code,
      message: body.message,
      request_id: body.request_id,
    },
  };
}

function requestPath(path: string): string {
  if (!path.startsWith("/") || path.startsWith("//") || path.includes("\\")) {
    throw new ApiClientError("API 请求必须使用同源绝对路径", {
      kind: "contract",
    });
  }
  return path;
}

function requestTimeout(timeoutMs: number | undefined): number {
  const timeout = timeoutMs ?? defaultTimeoutMs;
  if (!Number.isSafeInteger(timeout) || timeout < 100 || timeout > 30_000) {
    throw new ApiClientError("API 请求 timeout 配置无效", { kind: "contract" });
  }
  return timeout;
}

function contractError(status: number, requestId?: string): ApiClientError {
  return new ApiClientError("服务返回了无法识别的 JSON 契约", {
    kind: "contract",
    status,
    requestId,
  });
}

function gatewayError(status: number, elapsedMs: number, requestId?: string): ApiClientError {
  return new ApiClientError("代理无法连接上游 API", {
    kind: "gateway",
    status,
    requestId,
    elapsedMs,
  });
}

function isGatewayStatus(status: number): boolean {
  return status === 502 || status === 503 || status === 504;
}

async function readJSON(response: Response, requestId?: string): Promise<unknown> {
  const contentType = response.headers.get("Content-Type")?.toLowerCase() ?? "";
  if (!contentType.includes("application/json")) {
    throw contractError(response.status, requestId);
  }

  try {
    return await response.json();
  } catch {
    throw contractError(response.status, requestId);
  }
}

export async function requestJSON<T>(
  path: string,
  options: JsonRequestOptions<T>,
): Promise<ApiResponse<T>> {
  const safePath = requestPath(path);
  const timeoutMs = requestTimeout(options.timeoutMs);
  const fetcher = options.fetcher ?? fetch;
  const now = options.now ?? monotonicNow;
  const startedAt = now();
  const controller = new AbortController();
  let timedOut = false;

  const onCallerAbort = () => controller.abort();
  if (options.signal?.aborted) {
    controller.abort();
  } else {
    options.signal?.addEventListener("abort", onCallerAbort, { once: true });
  }

  const timeoutID = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);

  try {
    const response = await fetcher(safePath, {
      method: "GET",
      headers: { Accept: "application/json" },
      cache: "no-store",
      credentials: "same-origin",
      mode: "same-origin",
      redirect: "error",
      signal: controller.signal,
    });
    if (controller.signal.aborted) {
      throw new Error("request aborted");
    }
    const requestId = response.headers.get(requestIDHeader) || undefined;
    const contentType = response.headers.get("Content-Type")?.toLowerCase() ?? "";
    if (
      !response.ok &&
      isGatewayStatus(response.status) &&
      !contentType.includes("application/json")
    ) {
      throw gatewayError(response.status, Math.max(0, Math.round(now() - startedAt)), requestId);
    }
    const body = await readJSON(response, requestId);

    if (!response.ok) {
      const envelope = decodePublicError(body);
      if (envelope === null) {
        throw contractError(response.status, requestId);
      }
      if (requestId !== undefined && requestId !== envelope.error.request_id) {
        throw contractError(response.status, requestId);
      }
      throw new ApiClientError(envelope.error.message, {
        kind: "http",
        status: response.status,
        code: envelope.error.code,
        requestId: requestId ?? envelope.error.request_id,
        elapsedMs: Math.max(0, Math.round(now() - startedAt)),
      });
    }

    let data: T | null;
    try {
      data = options.decode(body);
    } catch {
      throw contractError(response.status, requestId);
    }
    if (data === null) {
      throw contractError(response.status, requestId);
    }

    return {
      data,
      status: response.status,
      requestId,
      elapsedMs: Math.max(0, Math.round(now() - startedAt)),
    };
  } catch (error) {
    if (timedOut) {
      throw new ApiClientError("请求超时", { kind: "timeout" });
    }
    if (options.signal?.aborted || controller.signal.aborted) {
      throw new ApiClientError("请求已取消", { kind: "cancelled" });
    }
    if (error instanceof ApiClientError) {
      throw error;
    }
    throw new ApiClientError("无法连接服务", { kind: "network" });
  } finally {
    clearTimeout(timeoutID);
    options.signal?.removeEventListener("abort", onCallerAbort);
  }
}

export function asApiClientError(error: unknown): ApiClientError {
  if (error instanceof ApiClientError) {
    return error;
  }
  return new ApiClientError("无法连接服务", { kind: "network" });
}
