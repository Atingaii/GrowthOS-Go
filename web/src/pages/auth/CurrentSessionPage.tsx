import { useEffect, useRef } from "react";
import {
  LoaderCircle,
  LogOut,
  RefreshCw,
  ShieldCheck,
  TriangleAlert,
  UserRound,
} from "lucide-react";
import { Navigate, useOutletContext } from "react-router";

import type { ApiClientError } from "../../api/httpClient";
import type { UseSessionBoundaryResult } from "../../layouts/useSessionBoundary";

interface LogoutErrorPresentation {
  title: string;
  description: string;
}

function describeLogoutError(error: ApiClientError): LogoutErrorPresentation {
  if (error.kind === "http" && error.status === 403 && error.code === "request_origin_rejected") {
    return {
      title: "退出请求未通过安全校验",
      description: "当前会话仍保留。请刷新并重新核查会话后再试。",
    };
  }

  if (
    error.kind === "network" ||
    error.kind === "timeout" ||
    error.kind === "gateway" ||
    error.code === "authentication_unavailable"
  ) {
    return {
      title: "暂时无法确认退出结果",
      description: "当前会话仍保留，我们没有自动重试退出操作。请稍后重试或重新核查会话。",
    };
  }

  return {
    title: "退出响应无法确认",
    description: "当前会话仍保留，请重新核查后再决定是否重试。",
  };
}

function formatTimestamp(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
    timeZoneName: "short",
  }).format(new Date(value));
}

export function CurrentSessionPage() {
  const boundary = useOutletContext<UseSessionBoundaryResult>();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);
  const loggingOut = boundary.logoutState.phase === "logging-out";

  useEffect(() => {
    headingRef.current?.focus();
  }, []);

  useEffect(() => {
    if (boundary.logoutState.phase === "error") {
      errorRef.current?.focus();
    }
  }, [boundary.logoutState]);

  if (boundary.sessionState.phase === "anonymous") {
    return <Navigate to="/login" replace />;
  }
  if (boundary.sessionState.phase !== "authenticated") {
    return null;
  }

  const { session } = boundary.sessionState;
  const errorPresentation =
    boundary.logoutState.phase === "error"
      ? describeLogoutError(boundary.logoutState.error)
      : undefined;

  return (
    <section aria-labelledby="current-session-title">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="mb-2 font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-[#625df5]">
            Current session
          </p>
          <h2
            ref={headingRef}
            id="current-session-title"
            tabIndex={-1}
            className="text-2xl font-semibold tracking-tight text-zinc-950 outline-none focus-visible:ring-2 focus-visible:ring-[#625df5] focus-visible:ring-offset-4"
          >
            当前会话
          </h2>
          <p className="mt-2 text-sm leading-6 text-zinc-500">此浏览器已通过身份服务认证。</p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-700">
          <ShieldCheck className="h-3.5 w-3.5" aria-hidden="true" />
          已认证
        </div>
      </div>

      <div className="my-7 flex items-center gap-3 rounded-lg border border-zinc-200 bg-zinc-50 p-4">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-zinc-200 bg-white text-zinc-600">
          <UserRound className="h-5 w-5" aria-hidden="true" />
        </div>
        <div className="min-w-0">
          <p className="text-xs text-zinc-500">登录主体</p>
          <p className="mt-0.5 break-all font-mono text-sm font-semibold text-zinc-900">
            {session.principal.id}
          </p>
        </div>
      </div>

      <dl className="divide-y divide-zinc-100 border-y border-zinc-100">
        <div className="grid gap-1 py-4 sm:grid-cols-[8rem_1fr] sm:items-center">
          <dt className="text-xs font-medium text-zinc-500">身份类型</dt>
          <dd className="text-sm font-medium text-zinc-900">人员账号</dd>
        </div>
        <div className="grid gap-1 py-4 sm:grid-cols-[8rem_1fr] sm:items-center">
          <dt className="text-xs font-medium text-zinc-500">空闲到期时间</dt>
          <dd className="text-sm tabular-nums text-zinc-900">
            <time dateTime={session.idleExpiresAt}>{formatTimestamp(session.idleExpiresAt)}</time>
          </dd>
        </div>
        <div className="grid gap-1 py-4 sm:grid-cols-[8rem_1fr] sm:items-center">
          <dt className="text-xs font-medium text-zinc-500">绝对到期时间</dt>
          <dd className="text-sm tabular-nums text-zinc-900">
            <time dateTime={session.absoluteExpiresAt}>
              {formatTimestamp(session.absoluteExpiresAt)}
            </time>
          </dd>
        </div>
      </dl>

      {errorPresentation && boundary.logoutState.phase === "error" ? (
        <div
          ref={errorRef}
          tabIndex={-1}
          role="alert"
          className="mt-5 rounded-lg border border-amber-200 bg-amber-50 p-4 outline-none focus-visible:ring-2 focus-visible:ring-amber-500 focus-visible:ring-offset-2"
        >
          <div className="flex gap-3">
            <TriangleAlert className="mt-0.5 h-5 w-5 shrink-0 text-amber-700" aria-hidden="true" />
            <div>
              <p className="text-sm font-semibold text-amber-950">{errorPresentation.title}</p>
              <p className="mt-1 text-xs leading-5 text-amber-800">
                {errorPresentation.description}
              </p>
              {boundary.logoutState.error.requestId ? (
                <p className="mt-2 break-all text-[11px] text-amber-800">
                  支持编号：
                  <span className="font-mono">{boundary.logoutState.error.requestId}</span>
                </p>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}

      <div className="mt-7 flex flex-col gap-3 sm:flex-row sm:justify-between">
        <button
          type="button"
          onClick={boundary.retryCurrentSession}
          disabled={loggingOut}
          className="inline-flex min-h-11 items-center justify-center gap-2 rounded-lg border border-zinc-300 bg-white px-4 text-sm font-semibold text-zinc-800 outline-none transition-colors hover:bg-zinc-50 focus-visible:ring-2 focus-visible:ring-[#625df5] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:text-zinc-400"
        >
          <RefreshCw className="h-4 w-4" aria-hidden="true" />
          重新核查
        </button>
        <button
          type="button"
          onClick={() => void boundary.signOut()}
          disabled={loggingOut}
          className="inline-flex min-h-11 items-center justify-center gap-2 rounded-lg bg-zinc-900 px-4 text-sm font-semibold text-white outline-none transition-colors hover:bg-zinc-800 focus-visible:ring-2 focus-visible:ring-[#625df5] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:bg-zinc-400"
        >
          {loggingOut ? (
            <>
              <LoaderCircle className="h-4 w-4 animate-spin" aria-hidden="true" />
              正在退出
            </>
          ) : (
            <>
              <LogOut className="h-4 w-4" aria-hidden="true" />
              退出当前会话
            </>
          )}
        </button>
      </div>
      <p className="sr-only" role="status" aria-live="polite" aria-atomic="true">
        {loggingOut ? "正在退出当前会话，请稍候。" : ""}
      </p>
    </section>
  );
}
