import { useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import { PageHeader, SectionHeader, Surface } from "../../../components/common/ProductPage";
import { MOCK_SNAPSHOT_LABEL, mockCoupons } from "../../../mocks/growthOsMockData";

type CouponFilter = "available" | "used";

interface CopyFeedback {
  code: string;
  kind: "success" | "error";
  message: string;
}

const couponsByFilter = {
  available: mockCoupons.filter((coupon) => coupon.status === "available"),
  used: mockCoupons.filter((coupon) => coupon.status === "used"),
} as const;

const filterOptions: ReadonlyArray<{ label: string; value: CouponFilter }> = [
  { label: "可用", value: "available" },
  { label: "已使用", value: "used" },
];

export function CouponsPage() {
  const [activeFilter, setActiveFilter] = useState<CouponFilter>("available");
  const [copyFeedback, setCopyFeedback] = useState<CopyFeedback | null>(null);
  const feedbackTimerRef = useRef<number | null>(null);
  const copyAttemptRef = useRef(0);
  const visibleCoupons = couponsByFilter[activeFilter];

  useEffect(
    () => () => {
      copyAttemptRef.current += 1;
      if (feedbackTimerRef.current !== null) {
        window.clearTimeout(feedbackTimerRef.current);
      }
    },
    [],
  );

  function clearPendingFeedback() {
    copyAttemptRef.current += 1;
    if (feedbackTimerRef.current !== null) {
      window.clearTimeout(feedbackTimerRef.current);
      feedbackTimerRef.current = null;
    }
    setCopyFeedback(null);
  }

  function selectFilter(filter: CouponFilter) {
    if (filter === activeFilter) {
      return;
    }
    clearPendingFeedback();
    setActiveFilter(filter);
  }

  async function copyCouponCode(code: string) {
    clearPendingFeedback();
    const attempt = copyAttemptRef.current;
    let nextFeedback: CopyFeedback;

    try {
      if (!navigator.clipboard || typeof navigator.clipboard.writeText !== "function") {
        throw new Error("Clipboard API is unavailable");
      }
      await navigator.clipboard.writeText(code);
      nextFeedback = {
        code,
        kind: "success",
        message: `优惠码 ${code} 已复制。`,
      };
    } catch {
      nextFeedback = {
        code,
        kind: "error",
        message: `无法复制优惠码 ${code}，请手动选择并复制。`,
      };
    }

    if (attempt !== copyAttemptRef.current) {
      return;
    }

    setCopyFeedback(nextFeedback);
    feedbackTimerRef.current = window.setTimeout(() => {
      if (attempt === copyAttemptRef.current) {
        setCopyFeedback(null);
      }
      feedbackTimerRef.current = null;
    }, 2500);
  }

  return (
    <div className="space-y-8">
      <PageHeader
        titleId="coupons-page-title"
        eyebrow="Benefits / Local snapshot"
        title="优惠券与权益中心"
        description={`这里展示截至 ${MOCK_SNAPSHOT_LABEL} 的本地 Mock 优惠券并提供浏览器内复制演示；筛选和复制不会核销、发放或修改任何服务端权益。`}
        actions={
          <span className="inline-flex h-7 items-center rounded-md border border-violet-200 bg-violet-50 px-2.5 text-xs font-medium text-violet-600 dark:border-violet-500/25 dark:bg-violet-500/10 dark:text-violet-300">
            演示数据
          </span>
        }
      />

      <section aria-labelledby="coupon-summary-title" className="space-y-2">
        <SectionHeader
          titleId="coupon-summary-title"
          title="权益摘要"
          description="数量由当前静态样本计算，不代表真实账户库存。"
        />
        <Surface className="overflow-hidden">
          <dl className="grid divide-y divide-zinc-200 sm:grid-cols-3 sm:divide-x sm:divide-y-0 dark:divide-zinc-800">
            <div className="p-4">
              <dt className="text-xs font-medium text-zinc-500 dark:text-zinc-400">可用样本</dt>
              <dd className="mt-2 text-2xl font-semibold tabular-nums text-zinc-950 dark:text-zinc-50">
                {couponsByFilter.available.length} 张
              </dd>
            </div>
            <div className="p-4">
              <dt className="text-xs font-medium text-zinc-500 dark:text-zinc-400">已使用样本</dt>
              <dd className="mt-2 text-2xl font-semibold tabular-nums text-zinc-950 dark:text-zinc-50">
                {couponsByFilter.used.length} 张
              </dd>
            </div>
            <div className="p-4">
              <dt className="text-xs font-medium text-zinc-500 dark:text-zinc-400">服务端写入</dt>
              <dd className="mt-2">
                <span className="block text-sm font-semibold text-zinc-950 dark:text-zinc-50">
                  未接入
                </span>
                <span className="mt-1 block text-[11px] text-zinc-400 dark:text-zinc-500">
                  复制仅调用 Clipboard API
                </span>
              </dd>
            </div>
          </dl>
        </Surface>
      </section>

      <section aria-labelledby="coupon-list-title" className="space-y-2">
        <SectionHeader
          titleId="coupon-list-title"
          title="优惠券清单"
          description={`当前显示${activeFilter === "available" ? "可用" : "已使用"}样本。`}
          action={
            <div
              role="group"
              aria-label="优惠券状态筛选"
              className="inline-flex rounded-lg border border-zinc-200 bg-white p-1 dark:border-zinc-800 dark:bg-zinc-950"
            >
              {filterOptions.map((option) => {
                const selected = activeFilter === option.value;
                return (
                  <button
                    key={option.value}
                    type="button"
                    aria-pressed={selected}
                    onClick={() => selectFilter(option.value)}
                    className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/50 ${
                      selected
                        ? "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-950"
                        : "text-zinc-500 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
                    }`}
                  >
                    {option.label} {couponsByFilter[option.value].length}
                  </button>
                );
              })}
            </div>
          }
        />

        <div className="min-h-6">
          {copyFeedback ? (
            <p
              role={copyFeedback.kind === "error" ? "alert" : "status"}
              aria-live={copyFeedback.kind === "error" ? "assertive" : "polite"}
              aria-atomic="true"
              className={`rounded-lg border px-3 py-2 text-xs ${
                copyFeedback.kind === "error"
                  ? "border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-500/25 dark:bg-rose-500/10 dark:text-rose-300"
                  : "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300"
              }`}
            >
              {copyFeedback.message}
            </p>
          ) : null}
        </div>

        {visibleCoupons.length > 0 ? (
          <div className="grid gap-3 lg:grid-cols-2">
            {visibleCoupons.map((coupon) => {
              const copied = copyFeedback?.kind === "success" && copyFeedback.code === coupon.code;
              return (
                <Surface key={coupon.id} as="article" className="flex min-h-48 flex-col p-4">
                  <div className="flex items-start justify-between gap-4">
                    <div className="min-w-0">
                      <p className="text-[10px] font-medium uppercase tracking-[0.12em] text-zinc-400">
                        {coupon.category}
                      </p>
                      <h3 className="mt-1 text-base font-semibold leading-6 text-zinc-950 dark:text-zinc-50">
                        {coupon.title}
                      </h3>
                    </div>
                    <span
                      className={`shrink-0 rounded-md border px-2 py-1 text-[11px] font-medium ${
                        coupon.status === "available"
                          ? "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300"
                          : "border-zinc-200 bg-zinc-50 text-zinc-500 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-400"
                      }`}
                    >
                      {coupon.status === "available" ? "可使用" : "已使用"}
                    </span>
                  </div>

                  <div className="mt-4 flex items-end justify-between gap-4 border-b border-zinc-100 pb-4 dark:border-zinc-900">
                    <div>
                      <p className="text-[11px] text-zinc-400">优惠额度</p>
                      <p className="mt-1 font-mono text-2xl font-semibold tracking-[-0.02em] text-violet-600 dark:text-violet-300">
                        {coupon.discount}
                      </p>
                    </div>
                    <div className="text-right">
                      <p className="text-[11px] text-zinc-400">优惠码</p>
                      <code className="mt-1 block text-sm font-semibold text-zinc-900 dark:text-zinc-100">
                        {coupon.code}
                      </code>
                    </div>
                  </div>

                  <dl className="mt-3 grid grid-cols-2 gap-4 text-xs">
                    <div>
                      <dt className="text-zinc-400">最低消费</dt>
                      <dd className="mt-1 font-medium text-zinc-700 dark:text-zinc-300">
                        {coupon.minSpend}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-zinc-400">样本有效期</dt>
                      <dd className="mt-1 font-medium text-zinc-700 dark:text-zinc-300">
                        <time dateTime={coupon.expiryDate}>{coupon.expiryDate}</time>
                      </dd>
                    </div>
                  </dl>

                  {coupon.status === "available" ? (
                    <button
                      type="button"
                      onClick={() => void copyCouponCode(coupon.code)}
                      aria-label={
                        copied ? `已复制优惠码 ${coupon.code}` : `复制优惠码 ${coupon.code}`
                      }
                      className="mt-4 inline-flex min-h-10 items-center justify-center gap-2 rounded-lg border border-zinc-300 bg-white px-3 text-xs font-medium text-zinc-700 transition-colors hover:border-violet-400 hover:text-violet-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/50 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-300 dark:hover:border-violet-500 dark:hover:text-violet-300"
                    >
                      {copied ? (
                        <Check className="h-4 w-4" aria-hidden="true" />
                      ) : (
                        <Copy className="h-4 w-4" aria-hidden="true" />
                      )}
                      {copied ? "已复制" : "复制优惠码"}
                    </button>
                  ) : null}
                </Surface>
              );
            })}
          </div>
        ) : (
          <Surface className="p-6 text-center text-sm text-zinc-500 dark:text-zinc-400">
            当前筛选下没有演示优惠券。
          </Surface>
        )}
      </section>
    </div>
  );
}
