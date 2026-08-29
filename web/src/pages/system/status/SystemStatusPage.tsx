import type { ReactNode } from "react";
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  CircleHelp,
  Clock3,
  Database,
  RefreshCw,
  Server,
} from "lucide-react";
import { Link } from "react-router";
import type { ApiClientError, ApiResponse } from "../../../api/httpClient";
import type { HealthResponse, ReadinessResponse } from "../../../api/systemApi";
import { useSystemStatus, type ProbeLoadState, type SystemStatusState } from "./useSystemStatus";

type Tone = "healthy" | "warning" | "unknown" | "loading";

interface Summary {
  tone: Tone;
  title: string;
  detail: string;
}

interface ToneStyle {
  panel: string;
  icon: string;
  badge: string;
}

const toneStyles: Record<Tone, ToneStyle> = {
  healthy: {
    panel: "border-emerald-200 bg-emerald-50/70 dark:border-emerald-900 dark:bg-emerald-950/20",
    icon: "text-emerald-800 dark:text-emerald-300",
    badge: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300",
  },
  warning: {
    panel: "border-amber-200 bg-amber-50/70 dark:border-amber-900 dark:bg-amber-950/20",
    icon: "text-amber-900 dark:text-amber-300",
    badge: "bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-300",
  },
  unknown: {
    panel: "border-rose-200 bg-rose-50/70 dark:border-rose-900 dark:bg-rose-950/20",
    icon: "text-rose-800 dark:text-rose-300",
    badge: "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-300",
  },
  loading: {
    panel: "border-blue-200 bg-blue-50/70 dark:border-blue-900 dark:bg-blue-950/20",
    icon: "text-blue-800 dark:text-blue-300",
    badge: "bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300",
  },
};

function readinessUnavailable(error: ApiClientError): boolean {
  return error.kind === "http" && error.status === 503 && error.code === "dependency_unavailable";
}

function summarize(state: SystemStatusState): Summary {
  const { health, readiness } = state;

  if (health.phase === "loading" || readiness.phase === "loading") {
    return {
      tone: "loading",
      title: "正在检查",
      detail: "两个探针独立返回，先完成的结果会先显示。",
    };
  }

  if (health.phase === "success" && readiness.phase === "success") {
    return {
      tone: "healthy",
      title: "已接入检查正常",
      detail: "当前 API 实例能够响应，并且 MySQL 连接已就绪。",
    };
  }

  if (
    health.phase === "success" &&
    readiness.phase === "error" &&
    readinessUnavailable(readiness.error)
  ) {
    return {
      tone: "warning",
      title: "API 存活，MySQL 未就绪",
      detail: "进程仍能响应，但当前实例不应继续接收业务流量。",
    };
  }

  if (health.phase === "error" && readiness.phase === "success") {
    return {
      tone: "unknown",
      title: "无法确认 API 状态",
      detail: "就绪探针成功但存活探针失败；保留单项结果用于诊断，不将整体判为正常。",
    };
  }

  if (health.phase === "success") {
    return {
      tone: "warning",
      title: "API 存活，就绪状态未知",
      detail: "当前实例可响应，但无法确认 MySQL readiness。",
    };
  }

  return {
    tone: "unknown",
    title: "无法确认 API 状态",
    detail: "浏览器未取得可信的存活探针结果，请检查 API 进程与本地代理。",
  };
}

function stateTone<T>(state: ProbeLoadState<T>): Tone {
  if (state.phase === "loading") return "loading";
  if (state.phase === "success") return "healthy";
  return "unknown";
}

function errorText(error: ApiClientError, probe: "health" | "readiness"): string {
  if (probe === "readiness" && readinessUnavailable(error)) return "MySQL 连接未就绪";

  switch (error.kind) {
    case "timeout":
      return "请求超时，状态未知";
    case "network":
      return "无法连接 API";
    case "gateway":
      return error.status === undefined
        ? "代理无法连接 API"
        : `代理无法连接 API（HTTP ${error.status}）`;
    case "contract":
      return "响应契约无法识别";
    case "http":
      return error.status === undefined ? "服务返回错误" : `服务返回 HTTP ${error.status}`;
    case "cancelled":
      return "本次检查已取消";
  }
}

function timestampText(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

interface ProbeCardProps<T extends HealthResponse | ReadinessResponse> {
  endpoint: "/health" | "/ready";
  title: string;
  description: string;
  icon: ReactNode;
  state: ProbeLoadState<T>;
  probe: "health" | "readiness";
}

function ProbeDetails<T extends HealthResponse | ReadinessResponse>({
  response,
}: {
  response: ApiResponse<T>;
}) {
  return (
    <dl className="mt-5 grid gap-3 border-t border-stone-200 pt-4 text-xs dark:border-neutral-800 sm:grid-cols-2">
      <div>
        <dt className="text-stone-600 dark:text-stone-400">浏览器往返</dt>
        <dd className="mt-1 font-mono font-semibold text-stone-700 dark:text-stone-200">
          {response.elapsedMs} ms
        </dd>
      </div>
      <div>
        <dt className="text-stone-600 dark:text-stone-400">服务版本</dt>
        <dd className="mt-1 break-all font-mono font-semibold text-stone-700 dark:text-stone-200">
          {response.data.version}
        </dd>
      </div>
      <div className="sm:col-span-2">
        <dt className="text-stone-600 dark:text-stone-400">服务端时间</dt>
        <dd className="mt-1 font-mono text-stone-700 dark:text-stone-200">
          {timestampText(response.data.timestamp)}
        </dd>
      </div>
      <div className="sm:col-span-2">
        <dt className="text-stone-600 dark:text-stone-400">Request ID</dt>
        <dd className="mt-1 break-all font-mono text-stone-700 dark:text-stone-200">
          {response.requestId ?? "响应未提供"}
        </dd>
      </div>
    </dl>
  );
}

function ProbeCard<T extends HealthResponse | ReadinessResponse>({
  endpoint,
  title,
  description,
  icon,
  state,
  probe,
}: ProbeCardProps<T>) {
  const tone =
    state.phase === "error" && probe === "readiness" && readinessUnavailable(state.error)
      ? "warning"
      : stateTone(state);
  const styles = toneStyles[tone];
  const statusText =
    state.phase === "loading"
      ? "检查中"
      : state.phase === "success"
        ? "正常"
        : errorText(state.error, probe);

  return (
    <article className="rounded-2xl border border-stone-200 bg-white p-5 shadow-sm dark:border-neutral-800 dark:bg-[#141414]">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <span className={`mt-0.5 rounded-xl p-2 ${styles.badge}`}>{icon}</span>
          <div>
            <h2 className="font-bold text-stone-900 dark:text-white">{title}</h2>
            <p className="mt-1 text-xs leading-5 text-stone-600 dark:text-stone-400">
              {description}
            </p>
          </div>
        </div>
        <code className="rounded bg-stone-100 px-2 py-1 text-[11px] text-stone-600 dark:bg-neutral-800 dark:text-stone-300">
          {endpoint}
        </code>
      </div>

      <div
        role="status"
        aria-live="polite"
        aria-atomic="true"
        className={`mt-5 flex items-center gap-2 rounded-xl border px-3 py-2.5 text-sm font-semibold ${styles.panel} ${styles.icon}`}
      >
        {state.phase === "loading" ? (
          <RefreshCw className="h-4 w-4 animate-spin" aria-hidden="true" />
        ) : state.phase === "success" ? (
          <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
        ) : tone === "warning" ? (
          <AlertTriangle className="h-4 w-4" aria-hidden="true" />
        ) : (
          <CircleHelp className="h-4 w-4" aria-hidden="true" />
        )}
        <span className="sr-only">{title}：</span>
        {statusText}
        {state.phase === "error" && state.error.elapsedMs !== undefined ? (
          <span className="ml-auto font-mono text-xs font-normal">{state.error.elapsedMs} ms</span>
        ) : null}
      </div>

      {state.phase === "success" ? <ProbeDetails response={state.response} /> : null}
      {state.phase === "error" && state.error.requestId ? (
        <dl className="mt-4 border-t border-stone-200 pt-4 text-xs dark:border-neutral-800">
          <dt className="text-stone-600 dark:text-stone-400">Request ID</dt>
          <dd className="mt-1 break-all font-mono text-stone-700 dark:text-stone-200">
            {state.error.requestId}
          </dd>
        </dl>
      ) : null}
    </article>
  );
}

export function SystemStatusPage() {
  const { state, refresh } = useSystemStatus();
  const summary = summarize(state);
  const styles = toneStyles[summary.tone];
  const checking = state.health.phase === "loading" || state.readiness.phase === "loading";

  return (
    <main className="min-h-screen bg-stone-100 px-4 py-10 text-stone-900 dark:bg-[#0a0a0a] dark:text-stone-100 sm:px-6">
      <div className="mx-auto max-w-4xl space-y-6">
        <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <Link
              to="/home"
              className="text-xs font-medium text-blue-700 hover:underline dark:text-blue-300"
            >
              ← 返回 GrowthOS
            </Link>
            <div className="mt-3 flex items-center gap-2">
              <Activity className="h-6 w-6 text-blue-700 dark:text-blue-300" aria-hidden="true" />
              <h1 className="text-2xl font-extrabold tracking-tight">系统运行状态</h1>
            </div>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-stone-600 dark:text-stone-400">
              这里展示浏览器刚刚取得的 API 存活与 MySQL 就绪结果，不代表业务功能、集群 SLA
              或数据库迁移版本均已验证。
            </p>
          </div>
          <button
            type="button"
            onClick={refresh}
            disabled={checking}
            className="inline-flex items-center justify-center gap-2 rounded-xl bg-stone-900 px-4 py-2.5 text-xs font-bold text-white transition hover:bg-stone-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-white dark:text-stone-900 dark:hover:bg-stone-200"
          >
            <RefreshCw className={`h-4 w-4 ${checking ? "animate-spin" : ""}`} aria-hidden="true" />
            {checking ? "检查中" : "重新检查"}
          </button>
        </header>

        <section
          aria-live="polite"
          aria-atomic="true"
          className={`rounded-2xl border p-5 ${styles.panel}`}
        >
          <div className={`flex items-center gap-2 text-sm font-bold ${styles.icon}`}>
            {summary.tone === "healthy" ? (
              <CheckCircle2 className="h-5 w-5" aria-hidden="true" />
            ) : summary.tone === "loading" ? (
              <RefreshCw className="h-5 w-5 animate-spin" aria-hidden="true" />
            ) : summary.tone === "warning" ? (
              <AlertTriangle className="h-5 w-5" aria-hidden="true" />
            ) : (
              <CircleHelp className="h-5 w-5" aria-hidden="true" />
            )}
            {summary.title}
          </div>
          <p className="mt-1 pl-7 text-xs leading-5 text-stone-600 dark:text-stone-300">
            {summary.detail}
          </p>
          {state.completedAt ? (
            <p className="mt-3 flex items-center gap-1.5 pl-7 text-[11px] text-stone-600 dark:text-stone-400">
              <Clock3 className="h-3.5 w-3.5" aria-hidden="true" />
              本轮完成于 {timestampText(state.completedAt)}
            </p>
          ) : null}
        </section>

        <section className="grid gap-4 md:grid-cols-2" aria-label="当前实例探针">
          <ProbeCard
            endpoint="/health"
            title="Go API 进程"
            description="只验证当前实例能够处理 HTTP 请求，不检查外部依赖。"
            icon={<Server className="h-5 w-5" aria-hidden="true" />}
            state={state.health}
            probe="health"
          />
          <ProbeCard
            endpoint="/ready"
            title="MySQL readiness"
            description="当前只验证 API 到 MySQL 的连接；失败时实例仍可能存活。"
            icon={<Database className="h-5 w-5" aria-hidden="true" />}
            state={state.readiness}
            probe="readiness"
          />
        </section>

        <aside className="rounded-xl border border-stone-200 bg-white p-4 text-xs leading-5 text-stone-600 dark:border-neutral-800 dark:bg-[#141414] dark:text-stone-400">
          <strong className="text-stone-700 dark:text-stone-200">诊断边界：</strong>
          页面通过同源路径请求两个后端探针。每个 Request ID 只关联一次 HTTP
          请求；两个探针版本不同也不一定是故障，因为滚动发布时可能命中不同实例。
        </aside>
      </div>
    </main>
  );
}
