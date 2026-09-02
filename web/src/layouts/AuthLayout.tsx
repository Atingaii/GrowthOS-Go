import React, { useEffect, useRef } from "react";
import {
  Activity,
  Clock3,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { Link, Outlet } from "react-router";

import type { ApiClientError } from "../api/httpClient";
import { GrowthOSLogo } from "../components/common/GrowthOSGraphics";
import { useSessionBoundary } from "./useSessionBoundary";

const trustPoints = [
  {
    title: "服务端会话凭证",
    description: "浏览器脚本无法读取 HttpOnly Cookie",
    icon: LockKeyhole,
  },
  {
    title: "敏感请求校验",
    description: "退出操作同时校验来源与 CSRF token",
    icon: KeyRound,
  },
  {
    title: "双重到期边界",
    description: "同时执行空闲时限与绝对有效期",
    icon: Clock3,
  },
] as const;

function SessionChecking() {
  return (
    <section aria-labelledby="session-check-title">
      <LoaderCircle className="mb-5 h-6 w-6 animate-spin text-[#625df5]" aria-hidden="true" />
      <h2 id="session-check-title" className="text-2xl font-semibold tracking-tight text-zinc-950">
        正在核查当前会话
      </h2>
      <p className="mt-2 text-sm leading-6 text-zinc-500" role="status" aria-live="polite">
        请稍候，我们正在向身份服务确认此浏览器的登录状态。
      </p>
    </section>
  );
}

function SessionUnavailable({ error, onRetry }: { error: ApiClientError; onRetry: () => void }) {
  const alertRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    alertRef.current?.focus();
  }, []);

  return (
    <section
      ref={alertRef}
      tabIndex={-1}
      className="outline-none focus-visible:ring-2 focus-visible:ring-[#625df5] focus-visible:ring-offset-4"
      aria-labelledby="session-unavailable-title"
      role="alert"
    >
      <div className="mb-5 flex h-10 w-10 items-center justify-center rounded-lg bg-[#f3f1ff] text-[#5b55e7]">
        <ShieldCheck className="h-5 w-5" aria-hidden="true" />
      </div>
      <h2
        id="session-unavailable-title"
        className="text-2xl font-semibold tracking-tight text-zinc-950"
      >
        暂时无法确认登录状态
      </h2>
      <p className="mt-2 text-sm leading-6 text-zinc-500">
        身份服务当前不可用。我们不会把技术故障当作未登录，请稍后重新核查。
      </p>
      {error.requestId ? (
        <p className="mt-4 break-all text-xs text-zinc-500">
          支持编号：<span className="font-mono text-zinc-700">{error.requestId}</span>
        </p>
      ) : null}
      <button
        type="button"
        onClick={onRetry}
        className="mt-6 inline-flex min-h-11 items-center justify-center gap-2 rounded-lg border border-zinc-300 bg-white px-4 text-sm font-semibold text-zinc-800 outline-none transition-colors hover:bg-zinc-50 focus-visible:ring-2 focus-visible:ring-[#625df5] focus-visible:ring-offset-2"
      >
        <RefreshCw className="h-4 w-4" aria-hidden="true" />
        重新核查
      </button>
    </section>
  );
}

export const AuthLayout: React.FC = () => {
  const boundary = useSessionBoundary();

  return (
    <div className="relative min-h-screen overflow-hidden bg-white text-zinc-950">
      <a
        href="#auth-main-content"
        className="sr-only fixed left-4 top-4 z-50 rounded-md bg-[#625df5] px-4 py-2 text-sm font-semibold text-white focus:not-sr-only focus:outline-none focus:ring-2 focus:ring-[#625df5] focus:ring-offset-2"
      >
        跳到主要内容
      </a>

      <div
        className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_78%_42%,rgba(98,93,245,0.10),transparent_27%),radial-gradient(circle_at_16%_78%,rgba(59,130,246,0.06),transparent_30%)]"
        aria-hidden="true"
      />

      <header className="relative z-10 border-b border-zinc-200 bg-white/90">
        <div className="mx-auto flex h-16 w-full max-w-[76rem] items-center justify-between px-5 sm:px-8 lg:px-10">
          <div className="flex items-center gap-2.5" aria-label="GrowthOS Growth Platform">
            <GrowthOSLogo iconOnly className="h-8" />
            <div className="flex flex-col">
              <span className="text-sm font-bold leading-none tracking-tight text-zinc-900">
                GrowthOS
              </span>
              <span className="font-mono text-[10px] uppercase leading-tight tracking-widest text-zinc-400">
                Growth Platform
              </span>
            </div>
          </div>

          <Link
            to="/system/status"
            aria-label="查看系统状态"
            className="inline-flex min-h-11 items-center gap-2 rounded-lg px-3 text-sm font-medium text-zinc-500 outline-none transition-colors hover:bg-zinc-50 hover:text-zinc-950 focus-visible:ring-2 focus-visible:ring-[#625df5] focus-visible:ring-offset-2"
          >
            <Activity className="h-[18px] w-[18px]" aria-hidden="true" />
            <span className="hidden sm:inline">系统状态</span>
          </Link>
        </div>
      </header>

      <main
        id="auth-main-content"
        className="relative z-10 mx-auto grid w-full max-w-[76rem] gap-12 px-5 py-12 sm:px-8 sm:py-16 lg:min-h-[calc(100vh-4rem)] lg:grid-cols-[minmax(0,1fr)_28rem] lg:items-center lg:gap-20 lg:px-10 lg:py-20"
        aria-busy={boundary.sessionState.phase === "checking" ? "true" : undefined}
      >
        <section className="max-w-2xl lg:pb-8" aria-labelledby="auth-introduction-title">
          <p className="mb-5 font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#625df5]">
            Identity Session
          </p>
          <h1
            id="auth-introduction-title"
            className="max-w-xl text-4xl font-semibold leading-[1.12] tracking-[-0.035em] text-zinc-950 sm:text-5xl lg:text-[3.5rem]"
          >
            安全进入你的
            <br className="hidden sm:block" />
            增长工作空间
          </h1>
          <p className="mt-6 max-w-xl text-base leading-7 text-zinc-500 sm:text-lg sm:leading-8">
            GrowthOS
            将身份核验、会话生命周期与敏感操作保护集中到可信边界，让每一次访问都有清晰、可验证的安全状态。
          </p>

          <ul
            className="mt-9 hidden max-w-2xl gap-5 sm:grid sm:grid-cols-3 lg:mt-12"
            aria-label="会话安全特性"
          >
            {trustPoints.map((point) => {
              const Icon = point.icon;
              return (
                <li key={point.title} className="border-t border-zinc-200 pt-4">
                  <Icon className="mb-3 h-5 w-5 text-[#625df5]" aria-hidden="true" />
                  <p className="text-sm font-semibold text-zinc-900">{point.title}</p>
                  <p className="mt-1 text-xs leading-5 text-zinc-500">{point.description}</p>
                </li>
              );
            })}
          </ul>
        </section>

        <div className="w-full rounded-xl border border-zinc-200 bg-white p-6 shadow-[0_22px_60px_rgba(24,24,27,0.08)] sm:p-8">
          {boundary.sessionState.phase === "checking" ? (
            <SessionChecking />
          ) : boundary.sessionState.phase === "unavailable" ? (
            <SessionUnavailable
              error={boundary.sessionState.error}
              onRetry={boundary.retryCurrentSession}
            />
          ) : (
            <Outlet context={boundary} />
          )}
        </div>
      </main>
    </div>
  );
};
