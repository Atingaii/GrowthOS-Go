import { Link } from "react-router";
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis } from "recharts";
import { ArrowRight, Eye } from "lucide-react";
import { PageHeader, SectionHeader, Surface } from "../../../components/common/ProductPage";
import {
  MOCK_SNAPSHOT_LABEL,
  mockCampaigns,
  mockPointTransactions,
} from "../../../mocks/growthOsMockData";
import { useAppStore } from "../../../stores/appStore";

const trendData = [
  { date: "3/08", balance: 11980, income: 0, expense: 0 },
  { date: "3/09", balance: 12040, income: 60, expense: 0 },
  { date: "3/10", balance: 12190, income: 150, expense: 0 },
  { date: "3/11", balance: 12240, income: 50, expense: 0 },
  { date: "3/12", balance: 12300, income: 60, expense: 0 },
  { date: "3/13", balance: 12150, income: 50, expense: 200 },
  { date: "3/14", balance: 12450, income: 300, expense: 0 },
] as const;

const incomeRows = [
  { date: "3/14", value: 300 },
  { date: "3/13", value: 50 },
  { date: "3/12", value: 60 },
  { date: "3/11", value: 50 },
  { date: "3/10", value: 150 },
  { date: "3/09", value: 60 },
] as const;

const expenseRows = [
  { date: "3/14", value: 0 },
  { date: "3/13", value: 200 },
  { date: "3/12", value: 0 },
  { date: "3/11", value: 0 },
  { date: "3/10", value: 0 },
  { date: "3/09", value: 0 },
] as const;

const periodIncome = incomeRows.reduce((total, row) => total + row.value, 0);
const periodExpense = expenseRows.reduce((total, row) => total + row.value, 0);
const nextLevelThreshold = 13_290;

interface TrendTooltipProps {
  active?: boolean;
  label?: string;
  payload?: Array<{ dataKey?: string | number; value?: number }>;
}

function TrendTooltip({ active, payload, label }: TrendTooltipProps) {
  if (!active || !payload?.length) {
    return null;
  }

  const values = Object.fromEntries(
    payload.map((item) => [String(item.dataKey ?? "unknown"), item.value ?? 0]),
  );
  return (
    <div className="rounded-lg border border-zinc-200 bg-white px-3 py-2 text-xs shadow-lg dark:border-zinc-800 dark:bg-zinc-950">
      <div className="font-medium text-zinc-900 dark:text-zinc-100">{label}</div>
      <dl className="mt-1.5 space-y-1 text-zinc-500 dark:text-zinc-400">
        <div className="flex items-center justify-between gap-6">
          <dt>积分余额</dt>
          <dd className="font-medium tabular-nums text-zinc-900 dark:text-zinc-100">
            {Number(values.balance ?? 0).toLocaleString()}
          </dd>
        </div>
        <div className="flex items-center justify-between gap-6">
          <dt>当日收入</dt>
          <dd className="font-medium tabular-nums text-emerald-600">+{values.income ?? 0}</dd>
        </div>
        <div className="flex items-center justify-between gap-6">
          <dt>当日支出</dt>
          <dd className="font-medium tabular-nums text-rose-600">-{values.expense ?? 0}</dd>
        </div>
      </dl>
    </div>
  );
}

function TrendPanel({ balance }: { balance: number }) {
  const pointsToNextLevel = Math.max(0, nextLevelThreshold - balance);

  return (
    <div className="grid gap-8 lg:grid-cols-[minmax(0,2fr)_minmax(240px,0.82fr)] lg:gap-12">
      <section aria-labelledby="home-trend-title" className="min-w-0">
        <h2
          id="home-trend-title"
          className="mb-2 text-sm font-medium text-zinc-500 dark:text-zinc-400"
        >
          积分趋势
        </h2>
        <div className="h-[240px] w-full" aria-label="近七日演示积分趋势图">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={trendData} margin={{ top: 12, right: 8, bottom: 8, left: 8 }}>
              <defs>
                <linearGradient id="homeBalanceFill" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#3c84f6" stopOpacity={0.18} />
                  <stop offset="95%" stopColor="#3c84f6" stopOpacity={0} />
                </linearGradient>
              </defs>
              <XAxis dataKey="date" hide />
              <Tooltip content={<TrendTooltip />} cursor={false} />
              <Area
                type="monotone"
                dataKey="balance"
                stroke="#3c84f6"
                strokeWidth={2}
                fill="url(#homeBalanceFill)"
                dot={false}
                activeDot={{ r: 4, strokeWidth: 2, stroke: "#ffffff" }}
                isAnimationActive={false}
              />
              <Area
                type="monotone"
                dataKey="income"
                stroke="#44d275"
                strokeWidth={1.5}
                strokeDasharray="4 4"
                fill="none"
                dot={false}
                isAnimationActive={false}
              />
              <Area
                type="monotone"
                dataKey="expense"
                stroke="#fb5c60"
                strokeWidth={1.5}
                strokeDasharray="4 4"
                fill="none"
                dot={false}
                isAnimationActive={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-4 text-[11px] text-zinc-400">
          <span className="inline-flex items-center gap-1.5">
            <span className="h-0.5 w-4 rounded-full bg-[#3c84f6]" aria-hidden="true" /> 余额
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-0.5 w-4 rounded-full bg-[#44d275]" aria-hidden="true" /> 收入
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-0.5 w-4 rounded-full bg-[#fb5c60]" aria-hidden="true" /> 支出
          </span>
        </div>
      </section>

      <section
        aria-label="积分摘要"
        className="divide-y divide-zinc-200 self-start dark:divide-zinc-800"
      >
        <div className="pb-4">
          <div className="text-sm font-medium text-zinc-500 dark:text-zinc-400">可用演示积分</div>
          <div className="pt-2 text-2xl font-semibold leading-8 tracking-[-0.02em] tabular-nums text-zinc-950 dark:text-zinc-50">
            {balance.toLocaleString("en-US")}
          </div>
        </div>
        <div className="py-4">
          <div className="text-sm font-medium text-zinc-500 dark:text-zinc-400">近 7 日新增</div>
          <div className="pt-2 text-2xl font-semibold leading-8 tracking-[-0.02em] tabular-nums text-emerald-600">
            +{periodIncome.toLocaleString("en-US")}
          </div>
        </div>
        <div className="pt-4">
          <div className="text-sm font-medium text-zinc-500 dark:text-zinc-400">距下一等级</div>
          <div className="pt-2 text-2xl font-semibold leading-8 tracking-[-0.02em] tabular-nums text-zinc-950 dark:text-zinc-50">
            {pointsToNextLevel.toLocaleString("en-US")}
          </div>
        </div>
      </section>
    </div>
  );
}

interface OverviewCardProps {
  title: string;
  value?: string;
  children: React.ReactNode;
  action?: React.ReactNode;
}

function OverviewCard({ title, value, children, action }: OverviewCardProps) {
  return (
    <Surface className="flex min-h-[300px] flex-col border-0">
      <div className="flex min-h-12 items-center justify-between px-4 py-3">
        <div className="flex items-center gap-4">
          <h3 className="text-sm font-medium text-zinc-900 dark:text-zinc-100">{title}</h3>
          {value ? (
            <span className="text-sm font-semibold tabular-nums text-zinc-950 dark:text-zinc-50">
              {value}
            </span>
          ) : null}
        </div>
        {action}
      </div>
      <div className="min-h-0 flex-1 px-4 pb-4">{children}</div>
      <div className="flex h-9 items-center justify-between border-t border-zinc-100 px-4 text-[11px] text-zinc-400 dark:border-zinc-900 dark:text-zinc-600">
        <span>快照 · {MOCK_SNAPSHOT_LABEL}</span>
        <span>非实时账务</span>
      </div>
    </Surface>
  );
}

function StatBars({
  rows,
  tone,
}: {
  rows: readonly { date: string; value: number }[];
  tone: "success" | "danger";
}) {
  const maxValue = Math.max(1, ...rows.map((row) => row.value));
  const barColor = tone === "success" ? "bg-emerald-500" : "bg-rose-500";
  const valueColor = tone === "success" ? "text-emerald-600" : "text-rose-600";

  return (
    <div className="space-y-2.5">
      {rows.map((row) => (
        <div key={row.date} className="space-y-1">
          <div className="flex items-center justify-between text-[11px]">
            <span className="text-zinc-400">{row.date}</span>
            <span className={`font-medium tabular-nums ${valueColor}`}>
              {tone === "success" ? "+" : "-"}
              {row.value.toFixed(2)}
            </span>
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-zinc-100 dark:bg-zinc-800">
            <div
              className={`h-full rounded-full ${barColor}`}
              style={{
                width: row.value === 0 ? "0%" : `${Math.max(10, (row.value / maxValue) * 100)}%`,
              }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

function RecentOverview() {
  return (
    <div className="grid gap-2 rounded-xl bg-zinc-100 p-2 lg:grid-cols-3 dark:bg-zinc-900">
      <OverviewCard
        title="积分账单"
        value={`${mockPointTransactions.length}`}
        action={
          <Link
            to="/points"
            aria-label="查看全部积分账单"
            className="inline-flex h-8 w-8 items-center justify-center rounded-md text-zinc-400 hover:bg-zinc-100 hover:text-zinc-900 dark:hover:bg-zinc-900 dark:hover:text-zinc-100"
          >
            <Eye className="h-3.5 w-3.5" aria-hidden="true" />
          </Link>
        }
      >
        <div className="space-y-1">
          {mockPointTransactions.map((transaction, index) => (
            <div
              key={transaction.id}
              className="flex min-h-10 items-center gap-3 rounded-lg bg-zinc-50 px-3 py-2 dark:bg-zinc-900"
            >
              <div className="min-w-0 flex-1">
                <div className="truncate text-xs font-medium text-zinc-800 dark:text-zinc-200">
                  {transaction.title}
                </div>
                <div className="mt-0.5 text-[10px] text-zinc-400">{transaction.date}</div>
              </div>
              <span
                className={`shrink-0 text-xs font-semibold tabular-nums ${
                  transaction.type === "earn" ? "text-emerald-600" : "text-rose-600"
                }`}
              >
                {transaction.type === "earn" ? "+" : ""}
                {transaction.amount}
              </span>
              {index === 0 ? (
                <span className="rounded-full bg-violet-100 px-2 py-0.5 text-[10px] font-medium text-violet-600 dark:bg-violet-500/15 dark:text-violet-300">
                  最新
                </span>
              ) : null}
            </div>
          ))}
        </div>
      </OverviewCard>

      <OverviewCard title="7 天积分收入" value={`+${periodIncome.toLocaleString("en-US")} PTS`}>
        <StatBars rows={incomeRows} tone="success" />
      </OverviewCard>

      <OverviewCard title="7 天积分支出" value={`-${periodExpense.toLocaleString("en-US")} PTS`}>
        <StatBars rows={expenseRows} tone="danger" />
      </OverviewCard>
    </div>
  );
}

export function UserHomePage() {
  const { user } = useAppStore();

  return (
    <div className="space-y-10">
      <section className="space-y-6" aria-labelledby="home-title">
        <PageHeader
          titleId="home-title"
          title="今天"
          description={`欢迎回来，${user.name}。数据快照截至 ${MOCK_SNAPSHOT_LABEL}，仅用于学习界面与增长业务建模，不代表真实账户账务。`}
          badge="演示数据"
        />
        <TrendPanel balance={user.points} />
      </section>

      <section className="space-y-2" aria-labelledby="recent-overview-title">
        <SectionHeader
          titleId="recent-overview-title"
          title="近期概览"
          action={
            <Link
              to="/campaigns"
              className="inline-flex items-center gap-1.5 text-xs font-medium text-violet-600 hover:text-violet-500"
            >
              查看活动 <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
            </Link>
          }
        />
        <RecentOverview />
      </section>

      <section className="space-y-2" aria-labelledby="campaign-snapshot-title">
        <SectionHeader
          titleId="campaign-snapshot-title"
          title="活动快照"
          description="当前页面仅显示本地演示活动，不会在后台自动执行奖励。"
        />
        <div className="grid gap-px overflow-hidden rounded-xl border border-zinc-200 bg-zinc-200 sm:grid-cols-2 lg:grid-cols-4 dark:border-zinc-800 dark:bg-zinc-800">
          {mockCampaigns.map((campaign) => (
            <Link
              key={campaign.id}
              to={`/campaigns/${campaign.id}`}
              className="group bg-white p-4 transition-colors hover:bg-zinc-50 dark:bg-zinc-950 dark:hover:bg-zinc-900"
            >
              <div className="flex items-start justify-between gap-3">
                <span className="text-[10px] font-medium uppercase tracking-[0.12em] text-zinc-400">
                  {campaign.category}
                </span>
                <span className="text-xs font-semibold text-emerald-600">
                  {campaign.rewardAmount}
                </span>
              </div>
              <h3 className="mt-3 line-clamp-2 text-sm font-medium leading-5 text-zinc-900 group-hover:text-violet-600 dark:text-zinc-100">
                {campaign.title}
              </h3>
              <div className="mt-4 flex items-center justify-between text-[11px] text-zinc-400">
                <span>{campaign.participants.toLocaleString()} 人参与</span>
                <span>{campaign.conversionRate}% 转化</span>
              </div>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
