import { useState, type FormEvent } from "react";
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  Cpu,
  Database,
  Hash,
  LoaderCircle,
  Search,
  Server,
  ShieldCheck,
  Sparkles,
  Target,
  Terminal,
  Timer,
} from "lucide-react";
import type { ApiClientError } from "../../../api/httpClient";
import { isCanonicalUint64ID } from "../../../api/lotteryApi";
import { useEphemeralLotterySelection } from "./useEphemeralLotterySelection";

interface ErrorPresentation {
  title: string;
  detail: string;
}

const selectionPipeline = [
  {
    step: "01",
    title: "React 校验与传输",
    detail: "前端仅校验并提交规范的 64 位无符号十进制 Strategy ID，ID 始终保持字符串格式传输。",
  },
  {
    step: "02",
    title: "同源 REST API",
    detail:
      "POST /api/v1/lottery/strategies/:id/ephemeral-selections，带 Demo Header，无 Body、无 Query、无自动重试。",
  },
  {
    step: "03",
    title: "GrowthOS-Go 加权选择",
    detail: "后端实时读取 MySQL 快照在服务端执行算法，直接返回只读无状态结果，不落库且不持久化。",
  },
] as const;

function SelectionPipelineSteps() {
  return (
    <ol className="mt-4 space-y-3">
      {selectionPipeline.map((item) => (
        <li
          key={item.step}
          className="rounded-2xl border border-stone-800/60 bg-stone-900/50 p-3 text-xs"
        >
          <div className="flex items-start gap-2.5">
            <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded border border-blue-500/40 bg-blue-500/10 font-mono text-[10px] font-bold text-blue-400">
              {item.step}
            </span>
            <div className="min-w-0 space-y-0.5">
              <strong className="block text-xs font-bold text-stone-100">{item.title}</strong>
              <span className="block text-[11px] leading-relaxed text-stone-400">
                {item.detail}
              </span>
            </div>
          </div>
        </li>
      ))}
    </ol>
  );
}

function ScopeLimitations() {
  return (
    <p className="mt-3 text-[11px] leading-relaxed text-stone-500 dark:text-stone-400">
      用户资格、抽奖次数、积分账户、库存、发奖、幂等结果查询、限流和 Redis
      业务能力。这个页面只证明浏览器结果不再由 Mock 决定。
    </p>
  );
}

function describeSelectionError(error: ApiClientError): ErrorPresentation {
  if (error.kind === "http") {
    if (error.status === 404 && error.code === "lottery_strategy_not_found") {
      return {
        title: "没有找到这个 Strategy",
        detail: "请确认该十进制 ID 已在当前开发数据库中配置。页面不会自动创建或猜测 Strategy。",
      };
    }
    if (error.status === 404 && error.code === "route_not_found") {
      return {
        title: "临时选择接口当前未启用",
        detail:
          "后端可能保持默认关闭，或当前前后端版本不一致。请检查 development/test feature gate。",
      };
    }
    if (error.status === 503) {
      return {
        title: "服务暂时无法给出可信结果",
        detail:
          "这次请求没有可查询的持久结果；再次操作会发起一次全新的临时选择，而不是恢复本次结果。",
      };
    }
    if (error.status === 502 || error.status === 504) {
      return {
        title: "无法确认这次请求的结果",
        detail:
          "网关没有拿到可验证的上游响应。当前接口不保存结果；再次操作一定是一次新的临时选择。",
      };
    }
    if (error.status === 400) {
      return {
        title: "服务端拒绝了临时选择请求",
        detail: "浏览器请求与当前服务契约不一致。请根据 Request ID 排查版本或边缘配置。",
      };
    }
    return {
      title: "服务没有返回可采用的选择结果",
      detail: "页面不会把服务端错误降级成“未中奖”，也不会在后台自动再选一次。",
    };
  }

  if (error.kind === "gateway" || error.kind === "network" || error.kind === "timeout") {
    return {
      title: "无法确认这次请求的结果",
      detail:
        "请求可能没有到达服务，也可能已经完成但响应丢失。当前接口不保存结果；再次操作一定是一次新的临时选择。",
    };
  }

  if (error.kind === "contract") {
    return {
      title: "无法验证服务响应契约",
      detail:
        "页面拒绝展示结构、ID、durability 或 outcome 不符合契约的数据，避免把未知内容当成真实结果。",
    };
  }

  return {
    title: "临时选择已取消",
    detail: "页面已停止等待这个请求，没有生成可恢复的本地结果。",
  };
}

function SelectionError({ error }: { error: ApiClientError }) {
  const presentation = describeSelectionError(error);

  return (
    <div
      role="alert"
      className="relative overflow-hidden rounded-2xl border border-rose-200/80 bg-rose-50/90 p-5 text-rose-950 shadow-sm transition-all dark:border-rose-900/70 dark:bg-rose-950/40 dark:text-rose-100"
    >
      <div className="flex items-start gap-3.5">
        <div className="mt-0.5 rounded-xl bg-rose-100 p-2 text-rose-600 dark:bg-rose-900/50 dark:text-rose-300">
          <AlertTriangle className="h-5 w-5 shrink-0" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1 space-y-1.5">
          <h3 className="text-base font-bold tracking-tight text-rose-950 dark:text-rose-100">
            {presentation.title}
          </h3>
          <p className="text-xs leading-relaxed text-rose-800/90 dark:text-rose-200/80">
            {presentation.detail}
          </p>
          {error.requestId ? (
            <div className="mt-3 inline-flex max-w-full items-center gap-1.5 rounded-lg border border-rose-200/60 bg-rose-100/60 px-2.5 py-1 font-mono text-[11px] font-semibold text-rose-900 dark:border-rose-900/60 dark:bg-rose-900/30 dark:text-rose-200">
              <span className="shrink-0 opacity-60">Request ID:</span>
              <span className="break-all font-bold">{error.requestId}</span>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

export function LotteryPage() {
  const [strategyID, setStrategyID] = useState("");
  const { state, select, clear } = useEphemeralLotterySelection();
  const strategyIDValid = isCanonicalUint64ID(strategyID);
  const isInvalid = strategyID !== "" && !strategyIDValid;
  const selecting = state.phase === "selecting";

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selecting && strategyIDValid) {
      select(strategyID);
    }
  };

  return (
    <div
      className="mx-auto max-w-6xl space-y-8 px-4 py-6 sm:px-6 lg:px-8"
      aria-labelledby="lottery-page-title"
    >
      {/* Header Stage */}
      <header className="relative overflow-hidden rounded-3xl border border-stone-200/80 bg-gradient-to-b from-stone-50 via-white to-amber-50/30 p-6 shadow-sm dark:border-stone-800/80 dark:from-stone-900/90 dark:via-stone-950 dark:to-stone-900/50 sm:p-8">
        <div className="bg-grid-pattern pointer-events-none absolute inset-0 opacity-40 dark:opacity-20" />

        <div className="relative z-10 space-y-4">
          <div className="flex flex-wrap items-center gap-2.5">
            <span className="inline-flex items-center gap-1.5 rounded-full border border-amber-300/60 bg-amber-100/80 px-3 py-1 text-xs font-semibold text-amber-900 dark:border-amber-700/50 dark:bg-amber-950/60 dark:text-amber-300">
              <ShieldCheck className="h-3.5 w-3.5" aria-hidden="true" />
              Development / Test only
            </span>
            <span className="inline-flex items-center gap-1.5 rounded-full border border-stone-300/60 bg-stone-100/80 px-3 py-1 text-xs font-semibold text-stone-800 dark:border-stone-700/50 dark:bg-stone-800/60 dark:text-stone-300">
              <Database className="h-3.5 w-3.5" aria-hidden="true" />
              非持久化选择
            </span>
          </div>

          <div className="max-w-3xl space-y-2">
            <h1
              id="lottery-page-title"
              className="text-2xl font-extrabold tracking-tight text-stone-900 dark:text-stone-50 sm:text-3xl lg:text-4xl"
            >
              Lottery 临时选择演示
            </h1>
            <p className="text-xs leading-relaxed text-stone-600 dark:text-stone-400 sm:text-sm">
              页面会向 GrowthOS-Go 发起一次真实的服务端加权选择。它不会创建
              Draw、扣除积分、预占库存或发放奖励；刷新页面后也无法恢复本次结果。
            </p>
          </div>
        </div>
      </header>

      {/* Experiment Console / Data Theater Grid */}
      <section className="grid gap-8 lg:grid-cols-12 lg:items-start">
        {/* Left Stage: Selection Workbench (7 cols) */}
        <div className="space-y-6 lg:col-span-7">
          {/* Unified Workbench Theater Card */}
          <div
            className="overflow-hidden rounded-3xl border border-stone-200/80 bg-white shadow-sm dark:border-stone-800/80 dark:bg-stone-900/90"
            aria-busy={selecting}
          >
            {/* Console Bar */}
            <div className="flex items-center justify-between border-b border-stone-200/80 bg-stone-50/80 px-6 py-3.5 dark:border-stone-800/80 dark:bg-stone-950/60">
              <div className="flex items-center gap-2">
                <Terminal className="h-4 w-4 text-blue-600 dark:text-blue-400" aria-hidden="true" />
                <span className="font-mono text-[11px] font-bold uppercase tracking-wider text-stone-600 dark:text-stone-400">
                  Selection Workbench
                </span>
              </div>
              <div className="flex items-center gap-2">
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px] text-stone-400 dark:text-stone-500">
                  <span
                    className={`h-2 w-2 rounded-full ${selecting ? "bg-amber-500 animate-pulse motion-reduce:animate-none" : "bg-emerald-500"}`}
                    aria-hidden="true"
                  />
                  {selecting ? "Evaluating" : "Ready"}
                </span>
              </div>
            </div>

            {/* Input & Form Control Area */}
            <div className="p-6 sm:p-7">
              <div className="flex items-start gap-3.5">
                <div className="rounded-2xl border border-blue-200/60 bg-blue-50 p-2.5 text-blue-700 dark:border-blue-900/50 dark:bg-blue-950/60 dark:text-blue-300">
                  <Search className="h-5 w-5" aria-hidden="true" />
                </div>
                <div className="space-y-1">
                  <h2 className="text-base font-bold text-stone-900 dark:text-stone-100">
                    选择一个已配置的 Strategy
                  </h2>
                  <p className="text-xs leading-relaxed text-stone-500 dark:text-stone-400">
                    当前后端没有 Strategy 列表接口，因此页面只接受你明确输入的规范十进制 ID。
                  </p>
                </div>
              </div>

              <form className="mt-6 space-y-5" onSubmit={handleSubmit}>
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <label
                      htmlFor="lottery-strategy-id"
                      className="text-xs font-bold uppercase tracking-wider text-stone-700 dark:text-stone-300"
                    >
                      Strategy ID
                    </label>
                    <span className="font-mono text-[11px] text-stone-400 dark:text-stone-500">
                      uint64 decimal
                    </span>
                  </div>

                  <div className="relative">
                    <Hash
                      className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-stone-400 transition-colors dark:text-stone-500"
                      aria-hidden="true"
                    />
                    <input
                      id="lottery-strategy-id"
                      name="strategy_id"
                      type="text"
                      inputMode="numeric"
                      autoComplete="off"
                      spellCheck={false}
                      value={strategyID}
                      disabled={selecting}
                      aria-invalid={isInvalid}
                      aria-describedby="lottery-strategy-help"
                      aria-errormessage={isInvalid ? "lottery-strategy-error" : undefined}
                      onChange={(event) => {
                        setStrategyID(event.target.value);
                        clear();
                      }}
                      placeholder="例如：21003"
                      className="w-full rounded-2xl border border-stone-300 bg-stone-50/50 py-3 pl-10 pr-4 font-mono text-sm text-stone-900 transition-all placeholder:text-stone-400 focus:border-blue-600 focus:bg-white focus:outline-none focus:ring-4 focus:ring-blue-600/10 disabled:cursor-not-allowed disabled:bg-stone-100 disabled:opacity-60 dark:border-stone-700 dark:bg-stone-950/60 dark:text-stone-100 dark:placeholder:text-stone-600 dark:focus:border-blue-500 dark:focus:bg-stone-950 dark:focus:ring-blue-500/20 dark:disabled:bg-stone-900"
                    />
                  </div>

                  <p
                    id="lottery-strategy-help"
                    className="text-[11px] leading-relaxed text-stone-500 dark:text-stone-400"
                  >
                    允许范围为 1～18446744073709551615。ID 始终按字符串传输，不经过 JavaScript
                    Number。
                  </p>

                  <p
                    id="lottery-strategy-error"
                    className="min-h-5 text-xs font-semibold text-rose-600 dark:text-rose-400"
                  >
                    {isInvalid ? "请输入无前导零、无符号且不超过 uint64 上限的十进制 ID。" : ""}
                  </p>
                </div>

                <div className="pt-1">
                  <button
                    type="submit"
                    disabled={!strategyIDValid || selecting}
                    className="relative inline-flex w-full items-center justify-center gap-2 rounded-2xl bg-stone-900 px-6 py-3.5 text-xs font-bold tracking-wide text-white shadow-sm transition-all hover:bg-stone-800 active:scale-[0.99] focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-stone-900/20 disabled:cursor-not-allowed disabled:bg-stone-300 disabled:opacity-70 disabled:shadow-none dark:bg-stone-100 dark:text-stone-900 dark:hover:bg-white dark:focus-visible:ring-stone-100/30 dark:disabled:bg-stone-800 dark:disabled:text-stone-500 sm:w-auto"
                  >
                    {selecting ? (
                      <>
                        <LoaderCircle
                          className="h-4 w-4 animate-spin motion-reduce:animate-none"
                          aria-hidden="true"
                        />
                        <span>服务端正在选择…</span>
                      </>
                    ) : (
                      <>
                        <Sparkles
                          className="h-4 w-4 text-amber-400 dark:text-amber-600"
                          aria-hidden="true"
                        />
                        <span>
                          {state.phase === "idle" ? "发起一次临时选择" : "发起一次新的临时选择"}
                        </span>
                      </>
                    )}
                  </button>
                </div>
              </form>
            </div>

            {/* Live Result Theater Area */}
            <div className="min-h-44 border-t border-stone-200/80 bg-stone-50/40 p-6 dark:border-stone-800/80 dark:bg-stone-950/40 sm:p-7">
              {state.phase === "idle" && (
                <div className="rounded-2xl border border-dashed border-stone-200 bg-white/50 p-6 text-center dark:border-stone-800 dark:bg-stone-900/30">
                  <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-xl border border-stone-200/80 bg-white shadow-xs dark:border-stone-800 dark:bg-stone-900">
                    <Terminal
                      className="h-4 w-4 text-stone-400 dark:text-stone-500"
                      aria-hidden="true"
                    />
                  </div>
                  <h3 className="mt-3 text-xs font-bold uppercase tracking-wider text-stone-500 dark:text-stone-400">
                    准备就绪 · 暂无选择结果
                  </h3>
                  <p className="mt-1 text-xs text-stone-400 dark:text-stone-500">
                    输入 Strategy ID 并点击上方按钮发起一次服务端临时选择
                  </p>
                </div>
              )}

              {state.phase === "selecting" && (
                <div
                  role="status"
                  aria-live="polite"
                  aria-atomic="true"
                  className="relative overflow-hidden rounded-2xl border border-blue-200/80 bg-blue-50/60 p-5 text-blue-950 shadow-sm transition-all dark:border-blue-900/60 dark:bg-blue-950/40 dark:text-blue-100"
                >
                  <div className="flex items-start gap-3.5">
                    <div className="relative flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-blue-200 bg-white text-blue-600 shadow-xs dark:border-blue-800 dark:bg-blue-900/80 dark:text-blue-300">
                      <Server
                        className="h-5 w-5 animate-pulse motion-reduce:animate-none"
                        aria-hidden="true"
                      />
                    </div>
                    <div className="min-w-0 space-y-1">
                      <div className="flex items-center gap-2">
                        <p className="text-sm font-bold tracking-tight">正在等待服务端结果</p>
                        <span
                          className="inline-flex h-2 w-2 rounded-full bg-blue-500 animate-ping motion-reduce:animate-none"
                          aria-hidden="true"
                        />
                      </div>
                      <p className="text-xs leading-relaxed text-blue-800/80 dark:text-blue-200/80">
                        动画只表示请求进行中，不参与随机选择，也不会在浏览器里再次抽取。
                      </p>
                    </div>
                  </div>
                </div>
              )}

              {state.phase === "success" && (
                <div
                  role="status"
                  aria-live="polite"
                  aria-atomic="true"
                  className={`relative overflow-hidden rounded-2xl border p-5 shadow-sm transition-all ${
                    state.response.data.award.outcome === "reward"
                      ? "border-emerald-200/80 bg-gradient-to-b from-emerald-50/80 to-white text-emerald-950 dark:border-emerald-900/70 dark:from-emerald-950/40 dark:to-stone-950 dark:text-emerald-100"
                      : "border-stone-200/80 bg-gradient-to-b from-stone-50/80 to-white text-stone-900 dark:border-stone-800/80 dark:from-stone-900/50 dark:to-stone-950 dark:text-stone-100"
                  }`}
                >
                  <div className="flex items-start gap-3.5">
                    <div
                      className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border shadow-xs ${
                        state.response.data.award.outcome === "reward"
                          ? "border-emerald-300/80 bg-emerald-100/80 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-900/60 dark:text-emerald-300"
                          : "border-stone-300/80 bg-stone-100/80 text-stone-600 dark:border-stone-700 dark:bg-stone-800/80 dark:text-stone-300"
                      }`}
                    >
                      {state.response.data.award.outcome === "reward" ? (
                        <Target className="h-5 w-5" aria-hidden="true" />
                      ) : (
                        <CheckCircle2 className="h-5 w-5" aria-hidden="true" />
                      )}
                    </div>

                    <div className="min-w-0 flex-1 space-y-1">
                      <span className="text-[11px] font-extrabold uppercase tracking-wider opacity-60">
                        服务端返回的临时结果
                      </span>
                      <h3 className="text-base font-black tracking-tight sm:text-lg">
                        {state.response.data.award.outcome === "reward"
                          ? "选中了奖励候选"
                          : "本次选中未中奖候选"}
                      </h3>
                      <p className="break-all font-mono text-xs font-bold opacity-90 sm:text-sm">
                        {state.response.data.award.name}
                      </p>
                      <p className="pt-1 text-xs leading-relaxed opacity-75">
                        {state.response.data.award.outcome === "reward"
                          ? "这不是中奖记录，也不表示库存已预占或奖励已发放。"
                          : "no_reward 是合法业务结果，不是系统错误或降级结果。"}
                      </p>
                    </div>
                  </div>

                  <dl className="mt-5 grid gap-2.5 border-t border-stone-200/60 pt-4 dark:border-stone-800/80 sm:grid-cols-2">
                    <div className="rounded-xl border border-stone-200/50 bg-white/60 p-2.5 dark:border-stone-800/50 dark:bg-stone-900/60">
                      <dt className="text-[11px] font-medium opacity-60">Strategy ID</dt>
                      <dd className="mt-0.5 break-all font-mono text-xs font-bold">
                        {state.response.data.strategyId}
                      </dd>
                    </div>

                    <div className="rounded-xl border border-stone-200/50 bg-white/60 p-2.5 dark:border-stone-800/50 dark:bg-stone-900/60">
                      <dt className="text-[11px] font-medium opacity-60">Award ID</dt>
                      <dd className="mt-0.5 break-all font-mono text-xs font-bold">
                        {state.response.data.award.id}
                      </dd>
                    </div>

                    <div className="rounded-xl border border-stone-200/50 bg-white/60 p-2.5 dark:border-stone-800/50 dark:bg-stone-900/60">
                      <dt className="text-[11px] font-medium opacity-60">Durability</dt>
                      <dd className="mt-0.5 break-all font-mono text-xs font-bold">
                        {state.response.data.durability}
                      </dd>
                    </div>

                    <div className="rounded-xl border border-stone-200/50 bg-white/60 p-2.5 dark:border-stone-800/50 dark:bg-stone-900/60">
                      <dt className="text-[11px] font-medium opacity-60">浏览器观测耗时</dt>
                      <dd className="mt-0.5 font-mono text-xs font-bold">
                        {state.response.elapsedMs} ms
                      </dd>
                    </div>

                    {state.response.requestId ? (
                      <div className="rounded-xl border border-stone-200/50 bg-white/60 p-2.5 dark:border-stone-800/50 dark:bg-stone-900/60 sm:col-span-2">
                        <dt className="text-[11px] font-medium opacity-60">
                          Request ID（仅用于故障关联）
                        </dt>
                        <dd className="mt-0.5 break-all font-mono text-xs font-bold">
                          {state.response.requestId}
                        </dd>
                      </div>
                    ) : null}
                  </dl>
                </div>
              )}

              {state.phase === "error" ? <SelectionError error={state.error} /> : null}
            </div>
          </div>
        </div>

        {/* Right Column: Compact Contract Sidebar & Progressive Mobile Disclosure (5 cols) */}
        <aside className="space-y-4 lg:col-span-5">
          {/* Pipeline Details Card - Native <details> on mobile, always visible on desktop */}
          <details className="group rounded-3xl border border-stone-800/80 bg-stone-950 p-5 text-stone-100 shadow-sm">
            <summary className="flex cursor-pointer list-none items-center justify-between font-mono text-xs font-bold uppercase tracking-wider text-stone-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400/60 lg:pointer-events-none [&::-webkit-details-marker]:hidden">
              <span className="flex items-center gap-2">
                <Cpu className="h-4 w-4 text-blue-400" aria-hidden="true" />
                真实调用链
              </span>
              <ChevronDown
                className="h-4 w-4 transition-transform group-open:rotate-180 motion-reduce:transition-none lg:hidden"
                aria-hidden="true"
              />
            </summary>
            <div className="hidden group-open:block lg:block">
              <SelectionPipelineSteps />
            </div>
          </details>

          {/* Retry Strategy Card */}
          <div className="rounded-3xl border border-amber-200/80 bg-amber-50/50 p-5 text-amber-950 shadow-sm dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-100">
            <div className="flex items-start gap-3">
              <div className="rounded-lg bg-amber-100 p-1.5 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300">
                <AlertCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
              </div>
              <div className="min-w-0 space-y-1">
                <h2 className="text-xs font-bold tracking-tight">为什么失败后不自动重试？</h2>
                <p className="text-[11px] leading-relaxed text-amber-900/80 dark:text-amber-200/80">
                  当前没有 Draw ID
                  或持久结果。响应丢失后，页面不知道服务端是否已经完成；透明重试会再产生一个可能不同的新选择。
                </p>
              </div>
            </div>
          </div>

          {/* Scope Limitations Card */}
          <details className="group rounded-3xl border border-stone-200/80 bg-white p-5 shadow-sm dark:border-stone-800/80 dark:bg-stone-900/90">
            <summary className="flex cursor-pointer list-none items-center justify-between font-mono text-xs font-bold uppercase tracking-wider text-stone-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/50 dark:text-stone-300 lg:pointer-events-none [&::-webkit-details-marker]:hidden">
              <span className="flex items-center gap-2">
                <Timer className="h-4 w-4 text-amber-500" aria-hidden="true" />
                本节明确没有实现
              </span>
              <ChevronDown
                className="h-4 w-4 transition-transform group-open:rotate-180 motion-reduce:transition-none lg:hidden"
                aria-hidden="true"
              />
            </summary>
            <div className="hidden group-open:block lg:block">
              <ScopeLimitations />
            </div>
          </details>
        </aside>
      </section>
    </div>
  );
}
