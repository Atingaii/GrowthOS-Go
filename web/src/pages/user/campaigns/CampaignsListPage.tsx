import { useState } from "react";
import { ArrowRight, CalendarDays, Filter, Gift, Users } from "lucide-react";
import { Link } from "react-router";
import {
  PageHeader,
  ProgressBar,
  SectionHeader,
  Surface,
} from "../../../components/common/ProductPage";
import { MOCK_SNAPSHOT_LABEL, mockCampaigns } from "../../../mocks/growthOsMockData";

const allCategories = "all";
const campaignCategories = Array.from(new Set(mockCampaigns.map((campaign) => campaign.category)));

const statusPresentation = {
  active: {
    label: "进行中",
    className:
      "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-300",
  },
  draft: {
    label: "草稿",
    className:
      "border-zinc-200 bg-zinc-50 text-zinc-500 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400",
  },
  paused: {
    label: "已暂停",
    className:
      "border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-300",
  },
  ended: {
    label: "已结束",
    className:
      "border-zinc-200 bg-zinc-100 text-zinc-500 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-400",
  },
} as const;

function budgetPercentage(spent: number, total: number) {
  if (total <= 0) {
    return 0;
  }
  return Math.min(100, Math.max(0, (spent / total) * 100));
}

function formatBudget(value: number) {
  return `$${value.toLocaleString("en-US")}`;
}

export function CampaignsListPage() {
  const [selectedCategory, setSelectedCategory] = useState(allCategories);
  const filteredCampaigns =
    selectedCategory === allCategories
      ? mockCampaigns
      : mockCampaigns.filter((campaign) => campaign.category === selectedCategory);

  return (
    <div className="space-y-8">
      <PageHeader
        titleId="campaign-list-title"
        title="营销活动"
        description={`浏览截至 ${MOCK_SNAPSHOT_LABEL} 的本地演示活动状态、奖励配置与预算；本页不会自动报名、扣减预算或发放奖励。`}
        badge="演示数据"
      />

      <section className="space-y-2" aria-labelledby="campaign-directory-title">
        <SectionHeader
          titleId="campaign-directory-title"
          title="活动目录"
          description="按分类筛选当前演示数据集。"
          action={
            <span className="text-xs text-zinc-400" aria-live="polite">
              {filteredCampaigns.length} 个活动
            </span>
          }
        />

        <Surface className="flex flex-wrap items-end gap-3 p-3">
          <label className="min-w-56 flex-1 sm:max-w-72">
            <span className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-zinc-600 dark:text-zinc-300">
              <Filter className="h-3.5 w-3.5 text-violet-500" aria-hidden="true" />
              活动分类
            </span>
            <select
              aria-label="按活动分类筛选"
              value={selectedCategory}
              onChange={(event) => setSelectedCategory(event.target.value)}
              className="h-10 w-full rounded-lg border border-zinc-200 bg-white px-3 text-sm text-zinc-900 outline-none transition-colors focus:border-violet-400 focus:ring-2 focus:ring-violet-500/20 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
            >
              <option value={allCategories}>全部分类</option>
              {campaignCategories.map((category) => (
                <option key={category} value={category}>
                  {category}
                </option>
              ))}
            </select>
          </label>

          {selectedCategory !== allCategories ? (
            <button
              type="button"
              onClick={() => setSelectedCategory(allCategories)}
              className="inline-flex h-10 items-center justify-center rounded-lg border border-zinc-200 px-3 text-xs font-medium text-zinc-600 transition-colors hover:bg-zinc-50 hover:text-zinc-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/30 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-900 dark:hover:text-zinc-50"
            >
              清除筛选
            </button>
          ) : null}
        </Surface>

        {filteredCampaigns.length ? (
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {filteredCampaigns.map((campaign) => {
              const budgetUsed = budgetPercentage(campaign.budgetSpent, campaign.totalBudget);
              const status = statusPresentation[campaign.status];

              return (
                <article
                  key={campaign.id}
                  className="flex min-h-[292px] flex-col rounded-xl border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-950"
                >
                  <div className="flex items-start justify-between gap-3">
                    <span className="text-[11px] font-medium uppercase tracking-[0.12em] text-violet-600 dark:text-violet-300">
                      {campaign.category}
                    </span>
                    <span
                      className={`inline-flex shrink-0 items-center rounded-lg border px-2 py-0.5 text-[10px] font-medium ${status.className}`}
                    >
                      {status.label}
                    </span>
                  </div>

                  <h3 className="mt-3 text-base font-semibold leading-6 text-zinc-950 dark:text-zinc-50">
                    <Link
                      to={`/campaigns/${campaign.id}`}
                      className="transition-colors hover:text-violet-600 focus-visible:rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/30 dark:hover:text-violet-300"
                    >
                      {campaign.title}
                    </Link>
                  </h3>
                  <p className="mt-1 line-clamp-2 text-xs leading-5 text-zinc-500 dark:text-zinc-400">
                    {campaign.subtitle}
                  </p>

                  <dl className="mt-4 grid grid-cols-2 gap-2">
                    <div className="rounded-lg bg-zinc-50 p-3 dark:bg-zinc-900">
                      <dt className="flex items-center gap-1.5 text-[11px] text-zinc-400">
                        <Gift className="h-3.5 w-3.5" aria-hidden="true" /> 演示奖励
                      </dt>
                      <dd className="mt-1 truncate text-sm font-semibold text-zinc-900 dark:text-zinc-100">
                        {campaign.rewardAmount}
                      </dd>
                    </div>
                    <div className="rounded-lg bg-zinc-50 p-3 dark:bg-zinc-900">
                      <dt className="flex items-center gap-1.5 text-[11px] text-zinc-400">
                        <Users className="h-3.5 w-3.5" aria-hidden="true" /> 演示参与数
                      </dt>
                      <dd className="mt-1 text-sm font-semibold tabular-nums text-zinc-900 dark:text-zinc-100">
                        {campaign.participants.toLocaleString("en-US")}
                      </dd>
                    </div>
                  </dl>

                  <div className="mt-4">
                    <div className="flex items-center justify-between gap-3 text-[11px]">
                      <span className="font-medium text-zinc-500 dark:text-zinc-400">预算使用</span>
                      <span className="tabular-nums text-zinc-400">
                        {formatBudget(campaign.budgetSpent)} / {formatBudget(campaign.totalBudget)}
                      </span>
                    </div>
                    <ProgressBar
                      label={`${campaign.title} 活动预算使用进度`}
                      value={budgetUsed}
                      className="mt-2"
                    />
                  </div>

                  <div className="mt-auto flex items-center justify-between gap-3 border-t border-zinc-100 pt-4 text-[11px] dark:border-zinc-900">
                    <span className="flex min-w-0 items-center gap-1.5 text-zinc-400">
                      <CalendarDays className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                      <span className="truncate">
                        {campaign.startDate} — {campaign.endDate}
                      </span>
                    </span>
                    <Link
                      to={`/campaigns/${campaign.id}`}
                      aria-label={`查看 ${campaign.title} 活动详情`}
                      className="inline-flex shrink-0 items-center gap-1 font-medium text-violet-600 hover:text-violet-500 focus-visible:rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/30 dark:text-violet-300"
                    >
                      查看详情 <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
                    </Link>
                  </div>
                </article>
              );
            })}
          </div>
        ) : (
          <Surface className="p-8 text-center">
            <p className="text-sm font-medium text-zinc-800 dark:text-zinc-200">没有匹配的活动</p>
            <p className="mt-1 text-xs text-zinc-400">更换分类或清除筛选后再查看。</p>
          </Surface>
        )}
      </section>
    </div>
  );
}
