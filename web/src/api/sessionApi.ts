import {
  ApiClientError,
  deleteNoContent,
  postJSON,
  requestJSON,
  type ApiResponse,
  type RequestOptions,
} from "./httpClient";

export const sessionPath = "/api/v1/session";

const defaultSessionTimeoutMs = 5_000;
const minimumSessionTimeoutMs = 100;
const maximumLoginNameBytes = 64;
const maximumPasswordBytes = 512;
const maximumPasswordCodePoints = 128;
const maximumCSRFTokenBytes = 512;
const canonicalPrincipalIDPattern = /^[a-z0-9](?:[a-z0-9._:-]{0,126}[a-z0-9])?$/;
const canonicalLoginNamePattern = /^[a-z][a-z0-9._-]{2,63}$/;
const visibleASCIITextPattern = /^[\x21-\x7e]+$/;
const rfc3339Pattern =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(?:Z|[+-](\d{2}):(\d{2}))$/;

export interface SessionPrincipal {
  kind: "human";
  id: string;
}

export interface SessionSnapshot {
  authenticated: true;
  principal: SessionPrincipal;
  idleExpiresAt: string;
  absoluteExpiresAt: string;
  csrfToken: string;
}

export interface CreateSessionInput {
  loginName: string;
  password: string;
}

export type SessionRequestOptions = RequestOptions;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasExactKeys(value: Record<string, unknown>, expected: readonly string[]): boolean {
  const keys = Object.keys(value);
  return keys.length === expected.length && expected.every((key) => keys.includes(key));
}

function sessionOptions(options: SessionRequestOptions): RequestOptions {
  const timeoutMs = options.timeoutMs ?? defaultSessionTimeoutMs;
  if (
    !Number.isSafeInteger(timeoutMs) ||
    timeoutMs < minimumSessionTimeoutMs ||
    timeoutMs > defaultSessionTimeoutMs
  ) {
    throw new ApiClientError("会话请求 timeout 配置无效", { kind: "contract" });
  }
  return { ...options, timeoutMs };
}

function validPassword(value: unknown): value is string {
  if (typeof value !== "string" || value === "") {
    return false;
  }

  let codePoints = 0;
  for (const character of value) {
    const codePoint = character.codePointAt(0);
    if (codePoint === undefined || (codePoint >= 0xd800 && codePoint <= 0xdfff)) {
      return false;
    }
    codePoints += 1;
    if (codePoints > maximumPasswordCodePoints) {
      return false;
    }
  }
  return new TextEncoder().encode(value).byteLength <= maximumPasswordBytes;
}

function validCreateSessionInput(value: unknown): value is CreateSessionInput {
  if (!isRecord(value)) {
    return false;
  }
  try {
    return (
      hasExactKeys(value, ["loginName", "password"]) &&
      typeof value.loginName === "string" &&
      value.loginName.length <= maximumLoginNameBytes &&
      canonicalLoginNamePattern.test(value.loginName) &&
      validPassword(value.password)
    );
  } catch {
    return false;
  }
}

function validPrincipalID(value: unknown): value is string {
  return typeof value === "string" && canonicalPrincipalIDPattern.test(value);
}

function daysInMonth(year: number, month: number): number {
  switch (month) {
    case 2:
      return year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28;
    case 4:
    case 6:
    case 9:
    case 11:
      return 30;
    default:
      return 31;
  }
}

function validRFC3339(value: unknown): value is string {
  if (typeof value !== "string") {
    return false;
  }
  const match = rfc3339Pattern.exec(value);
  if (match === null) {
    return false;
  }

  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6]);
  const offsetHour = match[7] === undefined ? 0 : Number(match[7]);
  const offsetMinute = match[8] === undefined ? 0 : Number(match[8]);
  return (
    month >= 1 &&
    month <= 12 &&
    day >= 1 &&
    day <= daysInMonth(year, month) &&
    hour <= 23 &&
    minute <= 59 &&
    second <= 59 &&
    offsetHour <= 23 &&
    offsetMinute <= 59 &&
    Number.isFinite(Date.parse(value))
  );
}

function validCSRFToken(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length <= maximumCSRFTokenBytes &&
    visibleASCIITextPattern.test(value)
  );
}

function decodeSessionSnapshot(value: unknown): SessionSnapshot | null {
  if (!isRecord(value) || !hasExactKeys(value, ["data"]) || !isRecord(value.data)) {
    return null;
  }
  const data = value.data;
  if (
    !hasExactKeys(data, [
      "authenticated",
      "principal",
      "idle_expires_at",
      "absolute_expires_at",
      "csrf_token",
    ]) ||
    data.authenticated !== true ||
    !isRecord(data.principal) ||
    !hasExactKeys(data.principal, ["kind", "id"]) ||
    data.principal.kind !== "human" ||
    !validPrincipalID(data.principal.id) ||
    !validRFC3339(data.idle_expires_at) ||
    !validRFC3339(data.absolute_expires_at) ||
    !validCSRFToken(data.csrf_token)
  ) {
    return null;
  }

  return {
    authenticated: true,
    principal: { kind: "human", id: data.principal.id },
    idleExpiresAt: data.idle_expires_at,
    absoluteExpiresAt: data.absolute_expires_at,
    csrfToken: data.csrf_token,
  };
}

export async function createSession(
  input: CreateSessionInput,
  options: SessionRequestOptions = {},
): Promise<ApiResponse<SessionSnapshot>> {
  if (!validCreateSessionInput(input)) {
    throw new ApiClientError("登录请求不符合会话契约", { kind: "contract" });
  }

  return postJSON(
    sessionPath,
    { login_name: input.loginName, password: input.password },
    {
      ...sessionOptions(options),
      expectedStatus: 201,
      decode: decodeSessionSnapshot,
    },
  );
}

export async function readCurrentSession(
  options: SessionRequestOptions = {},
): Promise<ApiResponse<SessionSnapshot>> {
  return requestJSON(sessionPath, {
    ...sessionOptions(options),
    expectedStatus: 200,
    decode: decodeSessionSnapshot,
  });
}

export async function revokeCurrentSession(
  csrfToken: string,
  options: SessionRequestOptions = {},
): Promise<ApiResponse<void>> {
  if (!validCSRFToken(csrfToken)) {
    throw new ApiClientError("CSRF token 不符合会话契约", { kind: "contract" });
  }

  return deleteNoContent(sessionPath, {
    ...sessionOptions(options),
    headers: { "X-CSRF-Token": csrfToken },
  });
}
