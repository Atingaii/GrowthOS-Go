import { useEffect, useRef, useState, type FormEvent } from "react";
import { ArrowRight, CheckCircle2, Eye, EyeOff, LoaderCircle, TriangleAlert } from "lucide-react";
import { Navigate, useOutletContext } from "react-router";

import type { ApiClientError } from "../../api/httpClient";
import type { UseSessionBoundaryResult } from "../../layouts/useSessionBoundary";

interface ErrorPresentation {
  title: string;
  description: string;
}

function describeLoginError(error: ApiClientError): ErrorPresentation {
  if (error.kind === "http") {
    if (error.status === 401 && error.code === "authentication_failed") {
      return {
        title: "登录未通过",
        description: "账号或密码不正确，请检查后重新输入。",
      };
    }
    if (error.status === 429 && error.code === "authentication_throttled") {
      return {
        title: "尝试次数过多",
        description: "为保护账号安全，登录已暂时受限，请稍后再试。",
      };
    }
    if (error.status === 403 && error.code === "request_origin_rejected") {
      return {
        title: "安全校验未通过",
        description: "当前页面来源未通过校验，请刷新页面后重试。",
      };
    }
  }

  if (
    error.kind === "network" ||
    error.kind === "timeout" ||
    error.kind === "gateway" ||
    error.code === "authentication_unavailable"
  ) {
    return {
      title: "登录服务暂不可用",
      description: "本次登录没有完成，请稍后重新提交。",
    };
  }

  return {
    title: "登录请求未能完成",
    description: "服务返回了无法确认的结果，请刷新页面后重试。",
  };
}

function AnonymousNotice({
  reason,
}: {
  reason: Extract<UseSessionBoundaryResult["sessionState"], { phase: "anonymous" }>["reason"];
}) {
  const noticeRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (reason !== undefined) {
      noticeRef.current?.focus();
    }
  }, [reason]);

  if (reason === undefined) {
    return null;
  }

  if (reason === "revocation-indeterminate") {
    return (
      <div
        ref={noticeRef}
        tabIndex={-1}
        role="alert"
        className="mb-6 rounded-lg border border-amber-200 bg-amber-50 p-4 outline-none focus-visible:ring-2 focus-visible:ring-amber-500 focus-visible:ring-offset-2"
      >
        <div className="flex gap-3">
          <TriangleAlert className="mt-0.5 h-5 w-5 shrink-0 text-amber-700" aria-hidden="true" />
          <div>
            <p className="text-sm font-semibold text-amber-950">服务端撤销状态未能确认</p>
            <p className="mt-1 text-xs leading-5 text-amber-800">
              当前浏览器的会话凭证已清除，但无法证明服务端 token
              已完成撤销。如有疑虑，请联系管理员。
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      ref={noticeRef}
      tabIndex={-1}
      role="status"
      aria-live="polite"
      className="mb-6 rounded-lg border border-emerald-200 bg-emerald-50 p-4 outline-none focus-visible:ring-2 focus-visible:ring-emerald-600 focus-visible:ring-offset-2"
    >
      <div className="flex gap-3">
        <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-emerald-700" aria-hidden="true" />
        <div>
          <p className="text-sm font-semibold text-emerald-950">
            {reason === "signed-out" ? "已退出当前会话" : "会话已经结束"}
          </p>
          <p className="mt-1 text-xs leading-5 text-emerald-800">
            {reason === "signed-out"
              ? "当前设备上的浏览器会话已退出。"
              : "当前会话已失效，请重新登录后继续。"}
          </p>
        </div>
      </div>
    </div>
  );
}

export function LoginPage() {
  const boundary = useOutletContext<UseSessionBoundaryResult>();
  const [passwordVisible, setPasswordVisible] = useState(false);
  const errorRef = useRef<HTMLDivElement>(null);
  const submitting = boundary.loginState.phase === "submitting";

  useEffect(() => {
    if (boundary.loginState.phase === "error") {
      errorRef.current?.focus();
    }
  }, [boundary.loginState]);

  if (boundary.sessionState.phase === "authenticated") {
    return <Navigate to="/session" replace />;
  }
  if (boundary.sessionState.phase !== "anonymous") {
    return null;
  }

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    const loginNameInput = form.elements.namedItem("loginName");
    const passwordInput = form.elements.namedItem("password");
    if (
      !(loginNameInput instanceof HTMLInputElement) ||
      !(passwordInput instanceof HTMLInputElement)
    ) {
      return;
    }

    const attempt = boundary.signIn({
      loginName: loginNameInput.value,
      password: passwordInput.value,
    });
    // The password is never copied into React state and leaves the DOM as soon as
    // the one-shot request has been started.
    passwordInput.value = "";
    setPasswordVisible(false);
    void attempt;
  };

  const errorPresentation =
    boundary.loginState.phase === "error"
      ? describeLoginError(boundary.loginState.error)
      : undefined;

  return (
    <section aria-labelledby="login-title">
      <AnonymousNotice reason={boundary.sessionState.reason} />

      <div className="mb-7">
        <p className="mb-2 font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-[#625df5]">
          Workforce access
        </p>
        <h2 id="login-title" className="text-2xl font-semibold tracking-tight text-zinc-950">
          登录 GrowthOS
        </h2>
        <p className="mt-2 text-sm leading-6 text-zinc-500">使用组织分配的账号继续。</p>
      </div>

      {errorPresentation && boundary.loginState.phase === "error" ? (
        <div
          ref={errorRef}
          tabIndex={-1}
          role="alert"
          className="mb-5 rounded-lg border border-rose-200 bg-rose-50 p-4 outline-none focus-visible:ring-2 focus-visible:ring-rose-500 focus-visible:ring-offset-2"
        >
          <div className="flex gap-3">
            <TriangleAlert className="mt-0.5 h-5 w-5 shrink-0 text-rose-700" aria-hidden="true" />
            <div>
              <p className="text-sm font-semibold text-rose-950">{errorPresentation.title}</p>
              <p className="mt-1 text-xs leading-5 text-rose-800">
                {errorPresentation.description}
              </p>
              {boundary.loginState.error.requestId ? (
                <p className="mt-2 break-all text-[11px] text-rose-800">
                  支持编号：
                  <span className="font-mono">{boundary.loginState.error.requestId}</span>
                </p>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}

      <form
        onSubmit={handleSubmit}
        aria-labelledby="login-title"
        aria-busy={submitting ? "true" : undefined}
      >
        <div>
          <label htmlFor="login-name" className="text-sm font-medium text-zinc-800">
            登录账号
          </label>
          <input
            id="login-name"
            name="loginName"
            type="text"
            required
            minLength={3}
            maxLength={64}
            pattern="[a-z][a-z0-9._-]{2,63}"
            title="请输入 3–64 位小写字母开头的账号，可包含数字、点、下划线或连字符"
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            disabled={submitting}
            className="mt-2 h-12 w-full rounded-lg border border-zinc-300 bg-white px-3.5 text-base text-zinc-950 outline-none transition-shadow placeholder:text-zinc-400 hover:border-zinc-400 focus:border-[#625df5] focus:ring-2 focus:ring-[#625df5]/20 disabled:cursor-not-allowed disabled:bg-zinc-100 sm:text-sm"
            placeholder="例如 operator-1"
          />
        </div>

        <div className="mt-5">
          <label htmlFor="login-password" className="text-sm font-medium text-zinc-800">
            密码
          </label>
          <div className="relative mt-2">
            <input
              id="login-password"
              name="password"
              type={passwordVisible ? "text" : "password"}
              required
              autoComplete="current-password"
              disabled={submitting}
              className="h-12 w-full rounded-lg border border-zinc-300 bg-white px-3.5 pr-12 text-base text-zinc-950 outline-none transition-shadow placeholder:text-zinc-400 hover:border-zinc-400 focus:border-[#625df5] focus:ring-2 focus:ring-[#625df5]/20 disabled:cursor-not-allowed disabled:bg-zinc-100 sm:text-sm"
              placeholder="输入当前密码"
            />
            <button
              type="button"
              aria-label={passwordVisible ? "隐藏密码" : "显示密码"}
              aria-pressed={passwordVisible}
              disabled={submitting}
              onClick={() => setPasswordVisible((visible) => !visible)}
              className="absolute inset-y-0 right-0 inline-flex w-12 items-center justify-center rounded-r-lg text-zinc-500 outline-none hover:text-zinc-900 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[#625df5] disabled:cursor-not-allowed disabled:text-zinc-300"
            >
              {passwordVisible ? (
                <EyeOff className="h-[18px] w-[18px]" aria-hidden="true" />
              ) : (
                <Eye className="h-[18px] w-[18px]" aria-hidden="true" />
              )}
            </button>
          </div>
        </div>

        <button
          type="submit"
          disabled={submitting}
          className="mt-7 inline-flex h-12 w-full items-center justify-center gap-2 rounded-lg bg-[#625df5] px-4 text-sm font-semibold text-white outline-none transition-colors hover:bg-[#544fe5] focus-visible:ring-2 focus-visible:ring-[#625df5] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:bg-[#9f9cf8]"
        >
          {submitting ? (
            <>
              <LoaderCircle className="h-4 w-4 animate-spin" aria-hidden="true" />
              正在登录
            </>
          ) : (
            <>
              安全登录
              <ArrowRight className="h-4 w-4" aria-hidden="true" />
            </>
          )}
        </button>
        <p className="sr-only" role="status" aria-live="polite" aria-atomic="true">
          {submitting ? "正在验证账号，请稍候。" : ""}
        </p>
      </form>

      <p className="mt-6 border-t border-zinc-100 pt-5 text-xs leading-5 text-zinc-500">
        仅限已获授权的组织成员访问。登录行为可能被记录用于安全审计。
      </p>
    </section>
  );
}
