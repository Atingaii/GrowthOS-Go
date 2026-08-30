import {
  ArrowLeft,
  CalendarDays,
  CheckCircle2,
  Gift,
  SearchX,
  Share2,
  TrendingUp,
  Users,
} from "lucide-react";
import { Link, useParams } from "react-router";
import {
  CompactMetric,
  DemoBadge,
  PageHeader,
  ProgressBar,
  SectionHeader,
  Surface,
} from "../../../components/common/ProductPage";
import { MOCK_SNAPSHOT_LABEL, mockCampaigns } from "../../../mocks/growthOsMockData";

const statusLabels = {
  active: "进行中",
  draft: "草稿",
  paused: "已暂停",
  ended: "已结束",
} as const;

function budgetPercentage(spent: number, total: number) {
  if (total <= 0) {
    return 0;
  }
  return Math.min(100, Math.max(0, (spent / total) * 100));
}

function MissingCampaign({ campaignId }: { campaignId: string | undefined }) {
  const displayedId = campaignId || "（空）";

  return (
    <div className="space-y-8">
      <PageHeader
        titleId="missing-campaign-title"
        eyebrow="CAMPAIGN"
        title="活动不存在"
        description={`本地演示数据中没有 ID 为“${displayedId}”的活动。页面不会回退展示其他活动。`}
        badge="未找到"
      />

      <section aria-labelledby="missing-campaign-title">
        <Surface className="p-6">
          <div className="flex items-start gap-3">
            <span className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-zinc-100 text-zinc-500 dark:bg-zinc-900 dark:text-zinc-400">
              <SearchX className="h-5 w-5" aria-hidden="true" />
            </span>
            <div>
              <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
                无法显示活动详情
              </h2>
              <p className="mt-1 text-xs leading-5 text-zinc-500 dark:text-zinc-400">
                请从活动目录选择一条存在的演示记录，或检查链接中的活动 ID。
              </p>
              <Link
                to="/campaigns"
                className="mt-4 inline-flex h-10 items-center gap-1.5 rounded-lg border border-zinc-200 px-3 text-xs font-medium text-zinc-700 transition-colors hover:bg-zinc-50 hover:text-zinc-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/30 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-900 dark:hover:text-zinc-50"
              >
                <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" /> 返回活动列表
              </Link>
            </div>
          </div>
        </Surface>
      </section>
    </div>
  );
}

export function CampaignDetailPage() {
  const { id } = useParams<{ id: string }>();
  const campaign = mockCampaigns.find((candidate) => candidate.id === id);

  if (!campaign) {
    return <MissingCampaign campaignId={id} />;
  }

  const budgetUsed = budgetPercentage(campaign.budgetSpent, campaign.totalBudget);

  return (
    <div className="space-y-8">
      <PageHeader
        titleId="campaign-detail-title"
        eyebrow={`${campaign.category} · ${statusLabels[campaign.status]}`}
        title={campaign.title}
        description={`${campaign.subtitle} 状态截至 ${MOCK_SNAPSHOT_LABEL}；内容来自本地演示配置，不代表真实报名、资格或奖励状态。`}
        badge="演示活动"
        actions={
          <Link
            to="/campaigns"
            className="inline-flex h-10 items-center gap-1.5 rounded-lg border border-zinc-200 px-3 text-xs font-medium text-zinc-700 transition-colors hover:bg-zinc-50 hover:text-zinc-950 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/30 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-900 dark:hover:text-zinc-50"
          >
            <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" /> 返回活动列表
          </Link>
        }
      />

      <section className="space-y-2" aria-labelledby="campaign-overview-title">
        <SectionHeader
          titleId="campaign-overview-title"
          title="活动概览"
          description="以下指标均为本地演示快照。"
        />
        <Surface className="grid gap-px overflow-hidden bg-zinc-200 sm:grid-cols-2 lg:grid-cols-4 dark:bg-zinc-800">
          <CompactMetric
            label="演示参与数"
            value={campaign.participants.toLocaleString("en-US")}
            helper="非实时用户统计"
            icon={Users}
            className="bg-white dark:bg-zinc-950"
          />
          <CompactMetric
            label="演示转化率"
            value={`${campaign.conversionRate}%`}
            helper="未连接分析服务"
            icon={TrendingUp}
            className="bg-white dark:bg-zinc-950"
          />
          <CompactMetric
            label="奖励配置"
            value={campaign.rewardAmount}
            helper="不代表已获得或到账"
            icon={Gift}
            className="bg-white dark:bg-zinc-950"
          />
          <CompactMetric
            label="活动状态"
            value={statusLabels[campaign.status]}
            helper="来自本地 mock 配置"
            icon={CheckCircle2}
            className="bg-white dark:bg-zinc-950"
          />
        </Surface>
      </section>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1.55fr)_minmax(280px,0.8fr)]">
        <section className="space-y-2" aria-labelledby="campaign-rules-title">
          <SectionHeader
            titleId="campaign-rules-title"
            title="演示规则与能力边界"
            description="规则文本用于说明界面，不会驱动真实业务流程。"
          />
          <Surface className="p-4 sm:p-5">
            <ol className="space-y-2">
              <li className="flex gap-3 rounded-lg bg-zinc-50 p-3 dark:bg-zinc-900">
                <span className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-lg bg-violet-100 text-[11px] font-semibold text-violet-600 dark:bg-violet-500/15 dark:text-violet-300">
                  01
                </span>
                <div>
                  <h3 className="text-sm font-medium text-zinc-900 dark:text-zinc-100">
                    查看演示配置
                  </h3>
                  <p className="mt-0.5 text-xs leading-5 text-zinc-500 dark:text-zinc-400">
                    页面只读取标题、日期、预算、参与数和奖励文案，没有活动报名接口。
                  </p>
                </div>
              </li>
              <li className="flex gap-3 rounded-lg bg-zinc-50 p-3 dark:bg-zinc-900">
                <span className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-lg bg-violet-100 text-[11px] font-semibold text-violet-600 dark:bg-violet-500/15 dark:text-violet-300">
                  02
                </span>
                <div>
                  <h3 className="text-sm font-medium text-zinc-900 dark:text-zinc-100">
                    不记录任务进度
                  </h3>
                  <p className="mt-0.5 text-xs leading-5 text-zinc-500 dark:text-zinc-400">
                    用户资格、邀请归因、完成状态和风控校验均未接入，本页不会伪造完成记录。
                  </p>
                </div>
              </li>
              <li className="flex gap-3 rounded-lg bg-zinc-50 p-3 dark:bg-zinc-900">
                <span className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-lg bg-violet-100 text-[11px] font-semibold text-violet-600 dark:bg-violet-500/15 dark:text-violet-300">
                  03
                </span>
                <div>
                  <h3 className="text-sm font-medium text-zinc-900 dark:text-zinc-100">
                    不承诺奖励发放
                  </h3>
                  <p className="mt-0.5 text-xs leading-5 text-zinc-500 dark:text-zinc-400">
                    “{campaign.rewardAmount}”仅是演示配置值；当前没有积分账户、库存或发奖服务。
                  </p>
                </div>
              </li>
            </ol>

            <div className="mt-4 border-t border-zinc-200 pt-4 dark:border-zinc-800">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-medium text-zinc-900 dark:text-zinc-100">
                  专属邀请链接
                </h3>
                <DemoBadge>演示未接入</DemoBadge>
              </div>
              <p
                id="campaign-copy-unavailable"
                className="mt-1 text-xs text-zinc-500 dark:text-zinc-400"
              >
                当前没有可复制的真实链接，也不会为演示用户生成邀请凭证。
              </p>
              <button
                type="button"
                disabled
                aria-describedby="campaign-copy-unavailable"
                className="mt-3 inline-flex h-10 cursor-not-allowed items-center gap-2 rounded-lg border border-zinc-200 bg-zinc-100 px-3 text-xs font-medium text-zinc-400 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-600"
              >
                <Share2 className="h-3.5 w-3.5" aria-hidden="true" />
                复制专属邀请链接（演示未接入）
              </button>
            </div>
          </Surface>
        </section>

        <aside className="space-y-2" aria-labelledby="campaign-configuration-title">
          <SectionHeader
            titleId="campaign-configuration-title"
            title="配置快照"
            description="用于学习页面的数据字段。"
          />
          <Surface className="p-4">
            <dl className="divide-y divide-zinc-100 text-xs dark:divide-zinc-900">
              <div className="flex items-start justify-between gap-4 pb-3">
                <dt className="flex items-center gap-1.5 text-zinc-500 dark:text-zinc-400">
                  <CalendarDays className="h-3.5 w-3.5" aria-hidden="true" /> 开始日期
                </dt>
                <dd className="font-medium tabular-nums text-zinc-900 dark:text-zinc-100">
                  {campaign.startDate}
                </dd>
              </div>
              <div className="flex items-start justify-between gap-4 py-3">
                <dt className="flex items-center gap-1.5 text-zinc-500 dark:text-zinc-400">
                  <CalendarDays className="h-3.5 w-3.5" aria-hidden="true" /> 结束日期
                </dt>
                <dd className="font-medium tabular-nums text-zinc-900 dark:text-zinc-100">
                  {campaign.endDate}
                </dd>
              </div>
              <div className="flex items-start justify-between gap-4 py-3">
                <dt className="text-zinc-500 dark:text-zinc-400">活动 ID</dt>
                <dd className="break-all font-mono font-medium text-zinc-900 dark:text-zinc-100">
                  {campaign.id}
                </dd>
              </div>
              <div className="flex items-start justify-between gap-4 pt-3">
                <dt className="text-zinc-500 dark:text-zinc-400">奖励类型</dt>
                <dd className="font-medium text-zinc-900 dark:text-zinc-100">
                  {campaign.rewardType}
                </dd>
              </div>
            </dl>

            <div className="mt-4 border-t border-zinc-200 pt-4 dark:border-zinc-800">
              <div className="flex items-center justify-between gap-3 text-xs">
                <span className="font-medium text-zinc-600 dark:text-zinc-300">预算使用</span>
                <span className="tabular-nums text-zinc-400">{budgetUsed.toFixed(1)}%</span>
              </div>
              <ProgressBar
                label={`${campaign.title} 活动预算使用进度`}
                value={budgetUsed}
                className="mt-2"
              />
              <div className="mt-2 flex items-center justify-between text-[11px] tabular-nums text-zinc-400">
                <span>${campaign.budgetSpent.toLocaleString("en-US")}</span>
                <span>${campaign.totalBudget.toLocaleString("en-US")}</span>
              </div>
            </div>
          </Surface>
        </aside>
      </div>
    </div>
  );
}
