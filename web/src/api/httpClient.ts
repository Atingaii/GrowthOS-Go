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

export interface RequestOptions {
  signal?: AbortSignal;
  timeoutMs?: number;
  fetcher?: FetchLike;
  now?: () => number;
}

export interface JsonRequestOptions<T> extends RequestOptions {
  decode: RuntimeDecoder<T>;
  expectedStatus?: number;
}

export interface JsonPostWithoutBodyOptions<T> extends JsonRequestOptions<T> {
  headers?: Readonly<Record<string, string>>;
}

export interface JsonPostOptions<T> extends JsonRequestOptions<T> {
  headers?: Readonly<Record<string, string>>;
}

export interface NoContentRequestOptions extends RequestOptions {
  headers?: Readonly<Record<string, string>>;
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

function validatedExpectedStatus(status: number | undefined): number | undefined {
  if (status === undefined) {
    return undefined;
  }
  if (!Number.isSafeInteger(status) || status < 100 || status > 599) {
    throw new ApiClientError("API 预期响应状态配置无效", { kind: "contract" });
  }
  return status;
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

interface RequestSpec<T> {
  method: "GET" | "POST" | "DELETE";
  headers: Readonly<Record<string, string>>;
  body?: string;
  decodeSuccess: (response: Response, requestId?: string) => Promise<T>;
}

async function executeRequest<T>(
  path: string,
  options: RequestOptions,
  spec: RequestSpec<T>,
): Promise<ApiResponse<T>> {
  const safePath = requestPath(path);
  const timeoutMs = requestTimeout(options.timeoutMs);
  const fetcher = options.fetcher ?? fetch;
  const now = options.now ?? monotonicNow;
  const startedAt = now();
  const controller = new AbortController();
  let timedOut = false;

  if (options.signal?.aborted) {
    throw new ApiClientError("请求已取消", { kind: "cancelled" });
  }

  const onCallerAbort = () => controller.abort();
  options.signal?.addEventListener("abort", onCallerAbort, { once: true });

  const timeoutID = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);

  try {
    const requestInit: RequestInit = {
      method: spec.method,
      headers: spec.headers,
      cache: "no-store",
      credentials: "same-origin",
      mode: "same-origin",
      redirect: "error",
      signal: controller.signal,
    };
    if (spec.body !== undefined) {
      requestInit.body = spec.body;
    }

    const response = await fetcher(safePath, requestInit);
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

    if (!response.ok) {
      const body = await readJSON(response, requestId);
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

    const data = await spec.decodeSuccess(response, requestId);
    if (controller.signal.aborted) {
      throw new Error("request aborted");
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

function jsonSuccessDecoder<T>(
  options: JsonRequestOptions<T>,
): (response: Response, requestId?: string) => Promise<T> {
  const expectedStatus = validatedExpectedStatus(options.expectedStatus);
  return async (response, requestId) => {
    if (expectedStatus !== undefined && response.status !== expectedStatus) {
      throw contractError(response.status, requestId);
    }
    const body = await readJSON(response, requestId);
    let data: T | null;
    try {
      data = options.decode(body);
    } catch {
      throw contractError(response.status, requestId);
    }
    if (data === null) {
      throw contractError(response.status, requestId);
    }
    return data;
  };
}

function encodeJSONObject(body: Readonly<Record<string, unknown>>): string {
  if (!isRecord(body)) {
    throw new ApiClientError("API JSON 请求体必须是对象", { kind: "contract" });
  }

  try {
    const encoded = JSON.stringify(body);
    if (encoded === undefined || !isRecord(JSON.parse(encoded))) {
      throw new Error("serialized value is not an object");
    }
    return encoded;
  } catch {
    throw new ApiClientError("API JSON 请求体无法序列化", { kind: "contract" });
  }
}

function withoutPayloadFraming(headers: Readonly<Record<string, string>>): boolean {
  return !Object.keys(headers).some((name) => {
    const normalized = name.toLowerCase();
    return (
      normalized === "content-type" ||
      normalized === "content-length" ||
      normalized === "transfer-encoding"
    );
  });
}

async function decodeNoContent(response: Response, requestId?: string): Promise<void> {
  if (response.status !== 204) {
    throw contractError(response.status, requestId);
  }
  if (response.headers.has("Content-Type") || response.headers.has("Transfer-Encoding")) {
    throw contractError(response.status, requestId);
  }
  const contentLength = response.headers.get("Content-Length");
  if (contentLength !== null && contentLength !== "0") {
    throw contractError(response.status, requestId);
  }

  let bytes: ArrayBuffer;
  try {
    bytes = await response.arrayBuffer();
  } catch {
    throw contractError(response.status, requestId);
  }
  if (bytes.byteLength !== 0) {
    throw contractError(response.status, requestId);
  }
}

export async function requestJSON<T>(
  path: string,
  options: JsonRequestOptions<T>,
): Promise<ApiResponse<T>> {
  return executeRequest(path, options, {
    method: "GET",
    headers: { Accept: "application/json" },
    decodeSuccess: jsonSuccessDecoder(options),
  });
}

/**
 * Sends a same-origin JSON POST whose request has no payload framing.
 *
 * Deliberately no `body` option exists here. The transport adds only `Accept`;
 * callers must opt into every other header and it never infers Content-Type.
 */
export async function postJSONWithoutBody<T>(
  path: string,
  options: JsonPostWithoutBodyOptions<T>,
): Promise<ApiResponse<T>> {
  const { headers = {}, ...requestOptions } = options;

  return executeRequest(path, requestOptions, {
    method: "POST",
    headers: { Accept: "application/json", ...headers },
    decodeSuccess: jsonSuccessDecoder(options),
  });
}

/** Sends one same-origin JSON object POST without retries. */
export async function postJSON<T>(
  path: string,
  body: Readonly<Record<string, unknown>>,
  options: JsonPostOptions<T>,
): Promise<ApiResponse<T>> {
  const { headers = {}, ...requestOptions } = options;
  if (!withoutPayloadFraming(headers)) {
    throw new ApiClientError("JSON POST 不允许覆盖 payload framing headers", {
      kind: "contract",
    });
  }
  const encodedBody = encodeJSONObject(body);

  return executeRequest(path, requestOptions, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      ...headers,
    },
    body: encodedBody,
    decodeSuccess: jsonSuccessDecoder(options),
  });
}

/**
 * Sends one bodyless DELETE and accepts only an exact, unframed 204 response.
 * API failures still use the shared public JSON error-envelope path.
 */
export async function deleteNoContent(
  path: string,
  options: NoContentRequestOptions = {},
): Promise<ApiResponse<void>> {
  const { headers = {}, ...requestOptions } = options;
  if (!withoutPayloadFraming(headers)) {
    throw new ApiClientError("无正文 DELETE 不允许 payload framing headers", {
      kind: "contract",
    });
  }

  return executeRequest(path, requestOptions, {
    method: "DELETE",
    headers: { Accept: "application/json", ...headers },
    decodeSuccess: decodeNoContent,
  });
}

export function asApiClientError(error: unknown): ApiClientError {
  if (error instanceof ApiClientError) {
    return error;
  }
  return new ApiClientError("无法连接服务", { kind: "network" });
}
