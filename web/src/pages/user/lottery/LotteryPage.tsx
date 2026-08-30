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
  Server,
  ShieldCheck,
  Sparkles,
  Target,
  Terminal,
  Timer,
} from "lucide-react";
import type { ApiClientError } from "../../../api/httpClient";
import { isCanonicalUint64ID } from "../../../api/lotteryApi";
import { PageHeader } from "../../../components/common/ProductPage";
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
    <ol className="mt-3 space-y-2">
      {selectionPipeline.map((item) => (
        <li key={item.step} className="rounded-lg bg-zinc-50 p-3 text-xs dark:bg-zinc-900">
          <div className="flex items-start gap-2.5">
            <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-violet-100 font-mono text-[10px] font-semibold text-violet-600 dark:bg-violet-500/15 dark:text-violet-300">
              {item.step}
            </span>
            <div className="min-w-0 space-y-0.5">
              <strong className="block text-xs font-medium text-zinc-900 dark:text-zinc-100">
                {item.title}
              </strong>
              <span className="block text-[11px] leading-relaxed text-zinc-500 dark:text-zinc-400">
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
    <p className="mt-2 text-[11px] leading-relaxed text-zinc-500 dark:text-zinc-400">
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
      className="rounded-lg border border-rose-200 bg-rose-50 p-4 text-rose-950 dark:border-rose-900/70 dark:bg-rose-950/30 dark:text-rose-100"
    >
      <div className="flex items-start gap-3">
        <div className="mt-0.5 rounded-lg bg-rose-100 p-2 text-rose-600 dark:bg-rose-900/50 dark:text-rose-300">
          <AlertTriangle className="h-4 w-4 shrink-0" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1 space-y-1">
          <h3 className="text-sm font-semibold text-rose-950 dark:text-rose-100">
            {presentation.title}
          </h3>
          <p className="text-xs leading-relaxed text-rose-800/90 dark:text-rose-200/80">
            {presentation.detail}
          </p>
          {error.requestId ? (
            <div className="mt-2 inline-flex max-w-full items-center gap-1.5 rounded-md border border-rose-200/60 bg-rose-100/60 px-2.5 py-1 font-mono text-[11px] font-medium text-rose-900 dark:border-rose-900/60 dark:bg-rose-900/30 dark:text-rose-200">
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
    <div className="space-y-6" aria-labelledby="lottery-page-title">
      <PageHeader
        titleId="lottery-page-title"
        eyebrow="Lottery · Server Selection"
        title="Lottery 临时选择演示"
        description="页面会向 GrowthOS-Go 发起一次真实的服务端加权选择。它不会创建 Draw、扣除积分、预占库存或发放奖励；刷新页面后也无法恢复本次结果。"
        actions={
          <div className="flex flex-wrap gap-2">
            <span className="inline-flex h-6 items-center gap-1.5 rounded-full border border-amber-200 bg-amber-50 px-2 text-[11px] font-medium text-amber-700 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-300">
              <ShieldCheck className="h-3.5 w-3.5" aria-hidden="true" />
              Development / Test only
            </span>
            <span className="inline-flex h-6 items-center gap-1.5 rounded-full border border-violet-200 bg-violet-50 px-2 text-[11px] font-medium text-violet-600 dark:border-violet-500/25 dark:bg-violet-500/10 dark:text-violet-300">
              <Database className="h-3.5 w-3.5" aria-hidden="true" />
              非持久化选择
            </span>
          </div>
        }
      />

      {/* Experiment Console / Data Theater Grid */}
      <section className="grid gap-6 xl:grid-cols-12 xl:items-start">
        {/* Left Stage: Selection Workbench (7 cols) */}
        <div className="space-y-4 xl:col-span-7">
          {/* Unified Workbench Theater Card */}
          <div
            className="overflow-hidden rounded-xl border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-950"
            aria-busy={selecting}
          >
            {/* Console Bar */}
            <div className="flex min-h-11 items-center justify-between border-b border-zinc-200 bg-zinc-50 px-4 py-2.5 dark:border-zinc-800 dark:bg-zinc-900/60">
              <div className="flex items-center gap-2">
                <Terminal className="h-4 w-4 text-violet-500" aria-hidden="true" />
                <span className="font-mono text-[11px] font-medium uppercase tracking-wider text-zinc-600 dark:text-zinc-400">
                  Selection Workbench
                </span>
              </div>
              <div className="flex items-center gap-2">
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px] text-zinc-400 dark:text-zinc-500">
                  <span
                    className={`h-2 w-2 rounded-full ${selecting ? "bg-amber-500 animate-pulse motion-reduce:animate-none" : "bg-emerald-500"}`}
                    aria-hidden="true"
                  />
                  {selecting ? "Evaluating" : "Ready"}
                </span>
              </div>
            </div>

            {/* Input & Form Control Area */}
            <div className="p-4 sm:p-5">
              <div className="flex items-start gap-3">
                <div className="rounded-lg bg-violet-50 p-2 text-violet-600 dark:bg-violet-500/10 dark:text-violet-300">
                  <Target className="h-4 w-4" aria-hidden="true" />
                </div>
                <div className="space-y-1">
                  <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
                    选择一个已配置的 Strategy
                  </h2>
                  <p className="text-xs leading-relaxed text-zinc-500 dark:text-zinc-400">
                    当前后端没有 Strategy 列表接口，因此页面只接受你明确输入的规范十进制 ID。
                  </p>
                </div>
              </div>

              <form className="mt-5 space-y-4" onSubmit={handleSubmit}>
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <label
                      htmlFor="lottery-strategy-id"
                      className="text-xs font-semibold text-zinc-700 dark:text-zinc-300"
                    >
                      Strategy ID
                    </label>
                    <span className="font-mono text-[11px] text-zinc-400 dark:text-zinc-500">
                      uint64 decimal
                    </span>
                  </div>

                  <div className="relative">
                    <Hash
                      className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-400 dark:text-zinc-500"
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
                      className="h-10 w-full rounded-lg border border-zinc-200 bg-white pl-9 pr-3 font-mono text-sm text-zinc-900 outline-none transition-colors placeholder:text-zinc-400 focus:border-violet-400 focus:ring-2 focus:ring-violet-500/15 disabled:cursor-not-allowed disabled:bg-zinc-100 disabled:text-zinc-400 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-100 dark:placeholder:text-zinc-600 dark:focus:border-violet-500 dark:disabled:bg-zinc-900"
                    />
                  </div>

                  <p
                    id="lottery-strategy-help"
                    className="text-[11px] leading-relaxed text-zinc-500 dark:text-zinc-400"
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
                    className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-lg bg-violet-600 px-4 text-sm font-medium text-white transition-colors hover:bg-violet-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/30 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-400 dark:ring-offset-zinc-950 dark:disabled:bg-zinc-800 dark:disabled:text-zinc-500 sm:w-auto"
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
                        <Sparkles className="h-4 w-4" aria-hidden="true" />
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
            <div className="min-h-40 border-t border-zinc-200 bg-zinc-50/60 p-4 dark:border-zinc-800 dark:bg-zinc-900/30 sm:p-5">
              {state.phase === "idle" && (
                <div className="rounded-lg border border-dashed border-zinc-200 bg-white p-5 text-center dark:border-zinc-800 dark:bg-zinc-950">
                  <div className="mx-auto flex h-9 w-9 items-center justify-center rounded-lg bg-zinc-100 dark:bg-zinc-900">
                    <Terminal
                      className="h-4 w-4 text-zinc-400 dark:text-zinc-500"
                      aria-hidden="true"
                    />
                  </div>
                  <h3 className="mt-3 text-xs font-medium text-zinc-600 dark:text-zinc-300">
                    准备就绪 · 暂无选择结果
                  </h3>
                  <p className="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
                    输入 Strategy ID 并点击上方按钮发起一次服务端临时选择
                  </p>
                </div>
              )}

              {state.phase === "selecting" && (
                <div
                  role="status"
                  aria-live="polite"
                  aria-atomic="true"
                  className="rounded-lg border border-blue-200 bg-blue-50 p-4 text-blue-950 dark:border-blue-900/60 dark:bg-blue-950/30 dark:text-blue-100"
                >
                  <div className="flex items-start gap-3">
                    <div className="relative flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-white text-blue-600 dark:bg-blue-900/80 dark:text-blue-300">
                      <Server
                        className="h-4 w-4 animate-pulse motion-reduce:animate-none"
                        aria-hidden="true"
                      />
                    </div>
                    <div className="min-w-0 space-y-1">
                      <div className="flex items-center gap-2">
                        <p className="text-sm font-semibold">正在等待服务端结果</p>
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
                  className={`rounded-lg border p-4 ${
                    state.response.data.award.outcome === "reward"
                      ? "border-emerald-200 bg-emerald-50/70 text-emerald-950 dark:border-emerald-900/70 dark:bg-emerald-950/25 dark:text-emerald-100"
                      : "border-zinc-200 bg-white text-zinc-900 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-100"
                  }`}
                >
                  <div className="flex items-start gap-3">
                    <div
                      className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${
                        state.response.data.award.outcome === "reward"
                          ? "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/60 dark:text-emerald-300"
                          : "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300"
                      }`}
                    >
                      {state.response.data.award.outcome === "reward" ? (
                        <Target className="h-5 w-5" aria-hidden="true" />
                      ) : (
                        <CheckCircle2 className="h-5 w-5" aria-hidden="true" />
                      )}
                    </div>

                    <div className="min-w-0 flex-1 space-y-1">
                      <span className="text-[11px] font-medium uppercase tracking-wider opacity-60">
                        服务端返回的临时结果
                      </span>
                      <h3 className="text-base font-semibold tracking-tight">
                        {state.response.data.award.outcome === "reward"
                          ? "选中了奖励候选"
                          : "本次选中未中奖候选"}
                      </h3>
                      <p className="break-all font-mono text-xs font-semibold opacity-90 sm:text-sm">
                        {state.response.data.award.name}
                      </p>
                      <p className="pt-1 text-xs leading-relaxed opacity-75">
                        {state.response.data.award.outcome === "reward"
                          ? "这不是中奖记录，也不表示库存已预占或奖励已发放。"
                          : "no_reward 是合法业务结果，不是系统错误或降级结果。"}
                      </p>
                    </div>
                  </div>

                  <dl className="mt-4 grid gap-2 border-t border-zinc-200/70 pt-4 dark:border-zinc-800 sm:grid-cols-2">
                    <div className="rounded-lg bg-white/70 p-2.5 dark:bg-zinc-900/70">
                      <dt className="text-[11px] font-medium opacity-60">Strategy ID</dt>
                      <dd className="mt-0.5 break-all font-mono text-xs font-semibold">
                        {state.response.data.strategyId}
                      </dd>
                    </div>

                    <div className="rounded-lg bg-white/70 p-2.5 dark:bg-zinc-900/70">
                      <dt className="text-[11px] font-medium opacity-60">Award ID</dt>
                      <dd className="mt-0.5 break-all font-mono text-xs font-semibold">
                        {state.response.data.award.id}
                      </dd>
                    </div>

                    <div className="rounded-lg bg-white/70 p-2.5 dark:bg-zinc-900/70">
                      <dt className="text-[11px] font-medium opacity-60">Durability</dt>
                      <dd className="mt-0.5 break-all font-mono text-xs font-semibold">
                        {state.response.data.durability}
                      </dd>
                    </div>

                    <div className="rounded-lg bg-white/70 p-2.5 dark:bg-zinc-900/70">
                      <dt className="text-[11px] font-medium opacity-60">浏览器观测耗时</dt>
                      <dd className="mt-0.5 font-mono text-xs font-semibold">
                        {state.response.elapsedMs} ms
                      </dd>
                    </div>

                    {state.response.requestId ? (
                      <div className="rounded-lg bg-white/70 p-2.5 dark:bg-zinc-900/70 sm:col-span-2">
                        <dt className="text-[11px] font-medium opacity-60">
                          Request ID（仅用于故障关联）
                        </dt>
                        <dd className="mt-0.5 break-all font-mono text-xs font-semibold">
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
        <aside className="space-y-3 xl:col-span-5">
          {/* Desktop contract rail */}
          <div
            role="region"
            aria-label="桌面真实调用链"
            className="hidden rounded-xl border border-zinc-200 bg-white p-4 text-zinc-900 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-100 xl:block"
          >
            <div className="flex items-center gap-2 text-xs font-semibold text-zinc-700 dark:text-zinc-300">
              <Cpu className="h-4 w-4 text-violet-500" aria-hidden="true" />
              真实调用链
            </div>
            <SelectionPipelineSteps />
          </div>

          {/* Mobile contract disclosure */}
          <details
            aria-label="移动真实调用链"
            className="group rounded-xl border border-zinc-200 bg-white p-4 text-zinc-900 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-100 xl:hidden"
          >
            <summary className="flex cursor-pointer list-none items-center justify-between text-xs font-semibold text-zinc-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/30 dark:text-zinc-300 [&::-webkit-details-marker]:hidden">
              <span className="flex items-center gap-2">
                <Cpu className="h-4 w-4 text-violet-500" aria-hidden="true" />
                真实调用链
              </span>
              <ChevronDown
                className="h-4 w-4 transition-transform group-open:rotate-180 motion-reduce:transition-none"
                aria-hidden="true"
              />
            </summary>
            <div className="hidden group-open:block">
              <SelectionPipelineSteps />
            </div>
          </details>

          {/* Retry Strategy Card */}
          <div className="rounded-xl border border-amber-200 bg-amber-50/60 p-4 text-amber-950 dark:border-amber-900/50 dark:bg-amber-950/25 dark:text-amber-100">
            <div className="flex items-start gap-3">
              <div className="rounded-lg bg-amber-100 p-1.5 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300">
                <AlertCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
              </div>
              <div className="min-w-0 space-y-1">
                <h2 className="text-xs font-semibold">为什么失败后不自动重试？</h2>
                <p className="text-[11px] leading-relaxed text-amber-900/80 dark:text-amber-200/80">
                  当前没有 Draw ID
                  或持久结果。响应丢失后，页面不知道服务端是否已经完成；透明重试会再产生一个可能不同的新选择。
                </p>
              </div>
            </div>
          </div>

          {/* Desktop scope statement */}
          <div
            role="region"
            aria-label="桌面功能边界说明"
            className="hidden rounded-xl border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-950 xl:block"
          >
            <div className="flex items-center gap-2 text-xs font-semibold text-zinc-700 dark:text-zinc-300">
              <Timer className="h-4 w-4 text-amber-500" aria-hidden="true" />
              本节明确没有实现
            </div>
            <ScopeLimitations />
          </div>

          {/* Mobile scope disclosure */}
          <details
            aria-label="移动功能边界说明"
            className="group rounded-xl border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-950 xl:hidden"
          >
            <summary className="flex cursor-pointer list-none items-center justify-between text-xs font-semibold text-zinc-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/30 dark:text-zinc-300 [&::-webkit-details-marker]:hidden">
              <span className="flex items-center gap-2">
                <Timer className="h-4 w-4 text-amber-500" aria-hidden="true" />
                本节明确没有实现
              </span>
              <ChevronDown
                className="h-4 w-4 transition-transform group-open:rotate-180 motion-reduce:transition-none"
                aria-hidden="true"
              />
            </summary>
            <div className="hidden group-open:block">
              <ScopeLimitations />
            </div>
          </details>
        </aside>
      </section>
    </div>
  );
}
