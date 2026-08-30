import { useState, type FormEvent } from "react";
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Database,
  Gift,
  Hash,
  LoaderCircle,
  Search,
  Server,
  ShieldCheck,
  Sparkles,
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
  ["1", "React", "只提交规范 Strategy ID"],
  ["2", "同源 API", "无 body、无 query、无自动重试"],
  ["3", "GrowthOS-Go", "读取 MySQL 快照并在服务端选择"],
] as const;

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
      className="rounded-2xl border border-rose-300 bg-rose-50 p-5 text-rose-950 dark:border-rose-900 dark:bg-rose-950/50 dark:text-rose-100"
    >
      <div className="flex items-start gap-3">
        <AlertTriangle
          className="mt-0.5 h-6 w-6 shrink-0 text-rose-600 dark:text-rose-400"
          aria-hidden="true"
        />
        <div>
          <h3 className="font-black">{presentation.title}</h3>
          <p className="mt-1 text-xs leading-5 opacity-80">{presentation.detail}</p>
          {error.requestId ? (
            <p className="mt-3 break-all font-mono text-xs font-bold">
              Request ID: {error.requestId}
            </p>
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
  const selecting = state.phase === "selecting";

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selecting && strategyIDValid) {
      select(strategyID);
    }
  };

  return (
    <main className="mx-auto max-w-5xl space-y-6" aria-labelledby="lottery-page-title">
      <header className="overflow-hidden rounded-3xl border border-amber-200 bg-gradient-to-br from-amber-50 via-white to-blue-50 p-6 shadow-sm dark:border-amber-900/60 dark:from-amber-950/30 dark:via-slate-950 dark:to-blue-950/30 sm:p-8">
        <div className="flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-100 px-3 py-1 text-xs font-bold text-amber-800 dark:bg-amber-950 dark:text-amber-300">
            <ShieldCheck className="h-4 w-4" aria-hidden="true" />
            Development / Test only
          </span>
          <span className="inline-flex items-center gap-1.5 rounded-full bg-blue-100 px-3 py-1 text-xs font-bold text-blue-800 dark:bg-blue-950 dark:text-blue-300">
            <Database className="h-4 w-4" aria-hidden="true" />
            非持久化选择
          </span>
        </div>
        <div className="mt-5 max-w-3xl space-y-3">
          <h1
            id="lottery-page-title"
            className="text-3xl font-black tracking-tight text-slate-950 dark:text-white sm:text-4xl"
          >
            Lottery 临时选择演示
          </h1>
          <p className="text-sm leading-6 text-slate-600 dark:text-slate-300 sm:text-base">
            页面会向 GrowthOS-Go 发起一次真实的服务端加权选择。它不会创建
            Draw、扣除积分、预占库存或发放奖励；刷新页面后也无法恢复本次结果。
          </p>
        </div>
      </header>

      <section className="grid gap-6 lg:grid-cols-[minmax(0,1.2fr)_minmax(280px,0.8fr)]">
        <div
          className="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900 sm:p-8"
          aria-busy={selecting}
        >
          <div className="flex items-start gap-3">
            <div className="rounded-2xl bg-blue-100 p-2.5 text-blue-700 dark:bg-blue-950 dark:text-blue-300">
              <Search className="h-5 w-5" aria-hidden="true" />
            </div>
            <div>
              <h2 className="font-extrabold text-slate-900 dark:text-white">
                选择一个已配置的 Strategy
              </h2>
              <p className="mt-1 text-sm leading-6 text-slate-500 dark:text-slate-400">
                当前后端没有 Strategy 列表接口，因此页面只接受你明确输入的规范十进制 ID。
              </p>
            </div>
          </div>

          <form className="mt-6 space-y-4" onSubmit={handleSubmit}>
            <div>
              <label
                htmlFor="lottery-strategy-id"
                className="text-sm font-bold text-slate-800 dark:text-slate-200"
              >
                Strategy ID
              </label>
              <div className="relative mt-2">
                <Hash
                  className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400"
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
                  aria-invalid={strategyID !== "" && !strategyIDValid}
                  aria-describedby="lottery-strategy-help lottery-strategy-error"
                  onChange={(event) => {
                    setStrategyID(event.target.value);
                    clear();
                  }}
                  placeholder="例如：21003"
                  className="w-full rounded-2xl border border-slate-300 bg-white py-3 pl-10 pr-4 font-mono text-sm text-slate-900 outline-none transition focus:border-blue-500 focus:ring-4 focus:ring-blue-500/10 disabled:cursor-not-allowed disabled:bg-slate-100 dark:border-slate-700 dark:bg-slate-950 dark:text-white dark:disabled:bg-slate-800"
                />
              </div>
              <p
                id="lottery-strategy-help"
                className="mt-2 text-xs leading-5 text-slate-500 dark:text-slate-400"
              >
                允许范围为 1～18446744073709551615。ID 始终按字符串传输，不经过 JavaScript Number。
              </p>
              <p
                id="lottery-strategy-error"
                className="mt-1 min-h-5 text-xs font-semibold text-rose-600 dark:text-rose-400"
              >
                {strategyID !== "" && !strategyIDValid
                  ? "请输入无前导零、无符号且不超过 uint64 上限的十进制 ID。"
                  : ""}
              </p>
            </div>

            <button
              type="submit"
              disabled={!strategyIDValid || selecting}
              className="inline-flex w-full items-center justify-center gap-2 rounded-2xl bg-gradient-to-r from-amber-500 to-orange-500 px-5 py-3.5 text-sm font-extrabold text-white shadow-lg shadow-amber-500/20 transition hover:from-amber-400 hover:to-orange-400 focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-amber-500/30 disabled:cursor-not-allowed disabled:from-slate-400 disabled:to-slate-400 disabled:shadow-none sm:w-auto"
            >
              {selecting ? (
                <>
                  <span className="animate-spin" aria-hidden="true">
                    <LoaderCircle className="h-5 w-5" />
                  </span>
                  服务端正在选择…
                </>
              ) : (
                <>
                  <Sparkles className="h-5 w-5" aria-hidden="true" />
                  {state.phase === "idle" ? "发起一次临时选择" : "发起一次新的临时选择"}
                </>
              )}
            </button>
          </form>

          <div className="mt-6" aria-live="polite">
            {state.phase === "selecting" && (
              <div
                role="status"
                className="flex items-start gap-3 rounded-2xl border border-blue-200 bg-blue-50 p-4 text-blue-900 dark:border-blue-900 dark:bg-blue-950/50 dark:text-blue-200"
              >
                <Server className="mt-0.5 h-5 w-5 shrink-0" aria-hidden="true" />
                <div>
                  <p className="text-sm font-extrabold">正在等待服务端结果</p>
                  <p className="mt-1 text-xs leading-5 opacity-80">
                    动画只表示请求进行中，不参与随机选择，也不会在浏览器里再次抽取。
                  </p>
                </div>
              </div>
            )}

            {state.phase === "success" && (
              <div
                role="status"
                className={`rounded-2xl border p-5 ${state.response.data.award.outcome === "reward" ? "border-emerald-300 bg-emerald-50 text-emerald-950 dark:border-emerald-900 dark:bg-emerald-950/50 dark:text-emerald-100" : "border-slate-300 bg-slate-50 text-slate-900 dark:border-slate-700 dark:bg-slate-800/70 dark:text-slate-100"}`}
              >
                <div className="flex items-start gap-3">
                  {state.response.data.award.outcome === "reward" ? (
                    <Gift
                      className="mt-0.5 h-6 w-6 shrink-0 text-emerald-600 dark:text-emerald-400"
                      aria-hidden="true"
                    />
                  ) : (
                    <CheckCircle2
                      className="mt-0.5 h-6 w-6 shrink-0 text-slate-500 dark:text-slate-300"
                      aria-hidden="true"
                    />
                  )}
                  <div>
                    <p className="text-xs font-black uppercase tracking-wide opacity-70">
                      服务端返回的临时结果
                    </p>
                    <h3 className="mt-1 text-lg font-black">
                      {state.response.data.award.outcome === "reward"
                        ? "选中了奖励候选"
                        : "本次选中未中奖候选"}
                    </h3>
                    <p className="mt-1 text-sm font-bold">{state.response.data.award.name}</p>
                    <p className="mt-2 text-xs leading-5 opacity-80">
                      {state.response.data.award.outcome === "reward"
                        ? "这不是中奖记录，也不表示库存已预占或奖励已发放。"
                        : "no_reward 是合法业务结果，不是系统错误或降级结果。"}
                    </p>
                  </div>
                </div>

                <dl className="mt-4 grid gap-3 border-t border-current/10 pt-4 text-xs sm:grid-cols-2">
                  <div>
                    <dt className="opacity-60">Strategy ID</dt>
                    <dd className="mt-1 break-all font-mono font-bold">
                      {state.response.data.strategyId}
                    </dd>
                  </div>
                  <div>
                    <dt className="opacity-60">Award ID</dt>
                    <dd className="mt-1 break-all font-mono font-bold">
                      {state.response.data.award.id}
                    </dd>
                  </div>
                  <div>
                    <dt className="opacity-60">Durability</dt>
                    <dd className="mt-1 font-mono font-bold">{state.response.data.durability}</dd>
                  </div>
                  <div>
                    <dt className="opacity-60">浏览器观测耗时</dt>
                    <dd className="mt-1 font-mono font-bold">{state.response.elapsedMs} ms</dd>
                  </div>
                  {state.response.requestId ? (
                    <div className="sm:col-span-2">
                      <dt className="opacity-60">Request ID（仅用于故障关联）</dt>
                      <dd className="mt-1 break-all font-mono font-bold">
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

        <aside className="space-y-4">
          <div className="rounded-3xl border border-slate-200 bg-slate-950 p-6 text-white shadow-sm dark:border-slate-800">
            <h2 className="text-sm font-extrabold">真实调用链</h2>
            <ol className="mt-4 space-y-3 text-xs text-slate-300">
              {selectionPipeline.map(([step, title, detail]) => (
                <li key={step} className="flex gap-3 rounded-2xl bg-white/5 p-3">
                  <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-blue-500 text-[11px] font-black text-white">
                    {step}
                  </span>
                  <span>
                    <strong className="block text-white">{title}</strong>
                    <span className="mt-0.5 block leading-5">{detail}</span>
                  </span>
                </li>
              ))}
            </ol>
          </div>

          <div className="rounded-3xl border border-blue-200 bg-blue-50 p-5 text-blue-950 dark:border-blue-900 dark:bg-blue-950/40 dark:text-blue-100">
            <div className="flex items-start gap-3">
              <AlertCircle
                className="mt-0.5 h-5 w-5 shrink-0 text-blue-600 dark:text-blue-400"
                aria-hidden="true"
              />
              <div>
                <h2 className="text-sm font-extrabold">为什么失败后不自动重试？</h2>
                <p className="mt-1 text-xs leading-5 opacity-80">
                  当前没有 Draw ID
                  或持久结果。响应丢失后，页面不知道服务端是否已经完成；透明重试会再产生一个可能不同的新选择。
                </p>
              </div>
            </div>
          </div>

          <div className="rounded-3xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
            <div className="flex items-center gap-2 text-slate-800 dark:text-slate-200">
              <Timer className="h-4 w-4 text-amber-500" aria-hidden="true" />
              <h2 className="text-sm font-extrabold">本节明确没有实现</h2>
            </div>
            <p className="mt-3 text-xs leading-5 text-slate-500 dark:text-slate-400">
              用户资格、抽奖次数、积分账户、库存、发奖、幂等结果查询、限流和 Redis
              业务能力。这个页面只证明浏览器结果不再由 Mock 决定。
            </p>
          </div>
        </aside>
      </section>
    </main>
  );
}
