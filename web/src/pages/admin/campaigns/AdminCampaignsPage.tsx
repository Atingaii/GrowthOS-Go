import { useMemo, useState } from "react";
import { Plus, Search } from "lucide-react";
import { PageHeader, Surface } from "../../../components/common/ProductPage";
import { StatusBadge } from "../../../components/common/UIComponents";
import { MOCK_SNAPSHOT_LABEL, mockCampaigns } from "../../../mocks/growthOsMockData";

export function AdminCampaignsPage() {
  const [query, setQuery] = useState("");
  const campaigns = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    if (!normalizedQuery) {
      return mockCampaigns;
    }
    return mockCampaigns.filter((campaign) =>
      `${campaign.id} ${campaign.title} ${campaign.category}`
        .toLocaleLowerCase()
        .includes(normalizedQuery),
    );
  }, [query]);

  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="Campaign Operations"
        title="营销活动与裂变策略"
        description={`检索截至 ${MOCK_SNAPSHOT_LABEL} 的本地演示活动及其预算、参与和转化快照。创建、编辑与发布链路尚未接入。`}
        badge="演示数据"
        actions={
          <button
            type="button"
            disabled
            title="创建活动后端尚未接入"
            className="inline-flex h-10 cursor-not-allowed items-center gap-2 rounded-lg bg-zinc-100 px-4 text-sm font-medium text-zinc-400 dark:bg-zinc-900 dark:text-zinc-600"
          >
            <Plus className="h-4 w-4" aria-hidden="true" /> 创建新活动 · 待接入
          </button>
        }
      />

      <Surface className="overflow-hidden">
        <div className="border-b border-zinc-200 p-4 dark:border-zinc-800">
          <label className="relative block max-w-md">
            <span className="sr-only">搜索活动</span>
            <Search
              className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-400"
              aria-hidden="true"
            />
            <input
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="按活动名称、ID 或类型搜索…"
              className="h-9 w-full rounded-lg border border-zinc-200 bg-zinc-50 pl-9 pr-3 text-sm text-zinc-900 outline-none focus:border-violet-400 focus:ring-2 focus:ring-violet-500/15 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-100"
            />
          </label>
        </div>

        <div
          role="region"
          aria-label="演示营销活动表格，可横向滚动"
          tabIndex={0}
          className="overflow-x-auto focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-violet-500/35"
        >
          <table className="w-full min-w-[860px] text-left text-xs">
            <caption className="sr-only">演示营销活动列表</caption>
            <thead className="bg-zinc-50 text-zinc-500 dark:bg-zinc-900/70 dark:text-zinc-400">
              <tr>
                <th scope="col" className="px-4 py-3 font-medium">
                  活动 ID / 名称
                </th>
                <th scope="col" className="px-4 py-3 font-medium">
                  类别
                </th>
                <th scope="col" className="px-4 py-3 font-medium">
                  状态
                </th>
                <th scope="col" className="px-4 py-3 font-medium">
                  参与人数
                </th>
                <th scope="col" className="px-4 py-3 font-medium">
                  预算消耗
                </th>
                <th scope="col" className="px-4 py-3 font-medium">
                  转化率
                </th>
                <th scope="col" className="px-4 py-3 text-right font-medium">
                  操作
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100 dark:divide-zinc-900">
              {campaigns.map((campaign) => (
                <tr key={campaign.id} className="hover:bg-zinc-50/80 dark:hover:bg-zinc-900/40">
                  <td className="px-4 py-3">
                    <div className="font-medium text-zinc-900 dark:text-zinc-100">
                      {campaign.title}
                    </div>
                    <div className="mt-0.5 font-mono text-[10px] text-zinc-400">{campaign.id}</div>
                  </td>
                  <td className="px-4 py-3 text-zinc-600 dark:text-zinc-300">
                    {campaign.category}
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={campaign.status} />
                  </td>
                  <td className="px-4 py-3 font-medium tabular-nums text-zinc-900 dark:text-zinc-100">
                    {campaign.participants.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 font-mono text-zinc-600 dark:text-zinc-300">
                    ${campaign.budgetSpent.toLocaleString()} / $
                    {campaign.totalBudget.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 font-medium tabular-nums text-emerald-600">
                    {campaign.conversionRate}%
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      type="button"
                      disabled
                      title="编辑链路尚未接入"
                      className="cursor-not-allowed font-medium text-zinc-400"
                    >
                      编辑 · 待接入
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {campaigns.length === 0 ? (
            <div role="status" className="px-4 py-12 text-center text-sm text-zinc-400">
              没有匹配的演示活动
            </div>
          ) : null}
        </div>
      </Surface>
    </div>
  );
}
