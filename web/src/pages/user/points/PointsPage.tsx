import { ArrowDownLeft, ArrowUpRight, Coins, ReceiptText } from "lucide-react";
import { PageHeader, SectionHeader, Surface } from "../../../components/common/ProductPage";
import { MOCK_SNAPSHOT_LABEL, mockPointTransactions } from "../../../mocks/growthOsMockData";
import { useAppStore } from "../../../stores/appStore";

const earnedSampleTotal = mockPointTransactions.reduce(
  (total, transaction) => (transaction.type === "earn" ? total + transaction.amount : total),
  0,
);

const spentSampleTotal = mockPointTransactions.reduce(
  (total, transaction) =>
    transaction.type === "spend" ? total + Math.abs(transaction.amount) : total,
  0,
);

const transactionStatusLabel = {
  completed: "已完成",
  pending: "待处理",
} as const;

export function PointsPage() {
  const user = useAppStore((state) => state.user);

  return (
    <div className="space-y-8">
      <PageHeader
        titleId="points-page-title"
        eyebrow="Credits / Local snapshot"
        title="积分资产中心"
        description={`以下余额与账单来自截至 ${MOCK_SNAPSHOT_LABEL} 的本地 Mock 快照，仅用于界面和数据结构演示，不代表实时账户、清算结果或可兑付资产。`}
        actions={
          <span className="inline-flex h-7 items-center rounded-md border border-violet-200 bg-violet-50 px-2.5 text-xs font-medium text-violet-600 dark:border-violet-500/25 dark:bg-violet-500/10 dark:text-violet-300">
            演示数据
          </span>
        }
      />

      <section aria-labelledby="points-summary-title" className="space-y-2">
        <SectionHeader
          titleId="points-summary-title"
          title="演示账户摘要"
          description="数值均由前端 Mock 数据计算，不会读取或修改后端积分账户。"
        />
        <Surface className="overflow-hidden">
          <dl
            aria-label="演示积分摘要"
            className="grid divide-y divide-zinc-200 sm:grid-cols-2 sm:divide-x sm:divide-y-0 lg:grid-cols-4 dark:divide-zinc-800"
          >
            <div className="min-w-0 p-4">
              <dt className="flex items-center gap-2 text-xs font-medium text-zinc-500 dark:text-zinc-400">
                <Coins className="h-4 w-4 text-violet-500" aria-hidden="true" />
                演示可用积分
              </dt>
              <dd className="mt-2">
                <span className="block text-2xl font-semibold leading-8 tracking-[-0.02em] text-zinc-950 tabular-nums dark:text-zinc-50">
                  {user.points.toLocaleString()} PTS
                </span>
                <span className="mt-1 block text-[11px] text-zinc-400 dark:text-zinc-500">
                  来自 mockUser
                </span>
              </dd>
            </div>

            <div className="min-w-0 p-4">
              <dt className="flex items-center gap-2 text-xs font-medium text-zinc-500 dark:text-zinc-400">
                <ArrowUpRight className="h-4 w-4 text-emerald-500" aria-hidden="true" />
                收入样本合计
              </dt>
              <dd className="mt-2">
                <span className="block text-2xl font-semibold leading-8 tracking-[-0.02em] text-emerald-600 tabular-nums dark:text-emerald-400">
                  +{earnedSampleTotal.toLocaleString()} PTS
                </span>
                <span className="mt-1 block text-[11px] text-zinc-400 dark:text-zinc-500">
                  仅汇总下方样本账单
                </span>
              </dd>
            </div>

            <div className="min-w-0 p-4">
              <dt className="flex items-center gap-2 text-xs font-medium text-zinc-500 dark:text-zinc-400">
                <ArrowDownLeft className="h-4 w-4 text-rose-500" aria-hidden="true" />
                支出样本合计
              </dt>
              <dd className="mt-2">
                <span className="block text-2xl font-semibold leading-8 tracking-[-0.02em] text-rose-600 tabular-nums dark:text-rose-400">
                  -{spentSampleTotal.toLocaleString()} PTS
                </span>
                <span className="mt-1 block text-[11px] text-zinc-400 dark:text-zinc-500">
                  不代表真实消费记录
                </span>
              </dd>
            </div>

            <div className="min-w-0 p-4">
              <dt className="flex items-center gap-2 text-xs font-medium text-zinc-500 dark:text-zinc-400">
                <ReceiptText className="h-4 w-4 text-zinc-400" aria-hidden="true" />
                账单样本数
              </dt>
              <dd className="mt-2">
                <span className="block text-2xl font-semibold leading-8 tracking-[-0.02em] text-zinc-950 tabular-nums dark:text-zinc-50">
                  {mockPointTransactions.length} 条
                </span>
                <span className="mt-1 block text-[11px] text-zinc-400 dark:text-zinc-500">
                  静态本地快照
                </span>
              </dd>
            </div>
          </dl>
        </Surface>
      </section>

      <section aria-labelledby="points-ledger-title" className="space-y-2">
        <SectionHeader
          titleId="points-ledger-title"
          title="账单快照"
          description="状态、时间与变动值均直接展示 Mock 字段，不提供真实对账或结算能力。"
        />
        <Surface className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="min-w-full border-collapse text-left text-xs">
              <caption className="sr-only">本地 Mock 积分账单</caption>
              <thead className="border-b border-zinc-200 bg-zinc-50 text-[11px] uppercase tracking-[0.08em] text-zinc-400 dark:border-zinc-800 dark:bg-zinc-900/70">
                <tr>
                  <th scope="col" className="whitespace-nowrap px-4 py-2.5 font-medium">
                    时间
                  </th>
                  <th scope="col" className="min-w-64 px-4 py-2.5 font-medium">
                    摘要
                  </th>
                  <th scope="col" className="whitespace-nowrap px-4 py-2.5 font-medium">
                    账单 ID
                  </th>
                  <th scope="col" className="whitespace-nowrap px-4 py-2.5 font-medium">
                    状态
                  </th>
                  <th scope="col" className="whitespace-nowrap px-4 py-2.5 text-right font-medium">
                    变动
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-100 dark:divide-zinc-900">
                {mockPointTransactions.map((transaction) => {
                  const isEarn = transaction.type === "earn";
                  return (
                    <tr key={transaction.id} className="text-zinc-600 dark:text-zinc-300">
                      <td className="whitespace-nowrap px-4 py-3 text-zinc-400">
                        <time dateTime={transaction.date.replace(" ", "T")}>
                          {transaction.date}
                        </time>
                      </td>
                      <td className="px-4 py-3 font-medium text-zinc-900 dark:text-zinc-100">
                        {transaction.title}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 font-mono text-[11px] text-zinc-400">
                        {transaction.id}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3">
                        <span className="inline-flex rounded-md border border-zinc-200 bg-zinc-50 px-2 py-1 text-[11px] text-zinc-600 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-300">
                          {transactionStatusLabel[transaction.status]}
                        </span>
                      </td>
                      <td
                        className={`whitespace-nowrap px-4 py-3 text-right font-mono font-semibold tabular-nums ${
                          isEarn
                            ? "text-emerald-600 dark:text-emerald-400"
                            : "text-rose-600 dark:text-rose-400"
                        }`}
                      >
                        <span className="sr-only">{isEarn ? "收入" : "支出"}</span>
                        {isEarn ? "+" : ""}
                        {transaction.amount.toLocaleString()} PTS
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <footer className="flex flex-wrap items-center justify-between gap-2 border-t border-zinc-200 px-4 py-2.5 text-[11px] text-zinc-400 dark:border-zinc-800 dark:text-zinc-500">
            <span>数据源：growthOsMockData.ts</span>
            <span>非实时账务 · 不可用于对账</span>
          </footer>
        </Surface>
      </section>
    </div>
  );
}
