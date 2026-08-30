import type { LucideIcon } from "lucide-react";
import { ArrowRight, MessageSquare, Plus, Share2, Sparkles, ThumbsUp } from "lucide-react";
import { Link } from "react-router";
import { PageHeader, SectionHeader, Surface } from "../../../components/common/ProductPage";
import { MOCK_SNAPSHOT_LABEL, mockFeedItems } from "../../../mocks/growthOsMockData";

interface DemoActionProps {
  count: number;
  icon: LucideIcon;
  label: string;
  noteId: string;
}

function DemoAction({ count, icon: Icon, label, noteId }: DemoActionProps) {
  return (
    <button
      type="button"
      disabled
      aria-describedby={noteId}
      aria-label={`${label} ${count}（演示，未接入）`}
      className="inline-flex h-8 cursor-not-allowed items-center gap-1.5 rounded-md px-2 text-xs font-medium text-zinc-400 disabled:opacity-100 dark:text-zinc-600"
    >
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
      <span>{label}</span>
      <span className="tabular-nums text-zinc-500 dark:text-zinc-500">{count}</span>
    </button>
  );
}

export function GrowthFeedPage() {
  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="Growth Feed"
        title="Growth Feed 社区态势"
        badge="演示数据"
        description={`截至 ${MOCK_SNAPSHOT_LABEL} 的本地模拟案例；发布、点赞、评论与分享尚未接入服务端，不会写入真实数据。`}
        actions={
          <button
            type="button"
            disabled
            aria-label="发布 Feed（未接入）"
            className="inline-flex h-9 cursor-not-allowed items-center gap-2 rounded-lg border border-zinc-200 bg-zinc-50 px-3 text-xs font-medium text-zinc-400 disabled:opacity-100 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-600"
          >
            <Plus className="h-3.5 w-3.5" aria-hidden="true" />
            发布未接入
          </button>
        }
      />

      <section aria-labelledby="growth-feed-list-title" className="space-y-3">
        <SectionHeader
          title="最新动态"
          titleId="growth-feed-list-title"
          description={`${mockFeedItems.length} 条本地演示内容，按模拟发布时间排列。`}
          action={
            <span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-zinc-500 dark:text-zinc-400">
              <span className="h-1.5 w-1.5 rounded-full bg-amber-500" aria-hidden="true" />
              只读演示
            </span>
          }
        />

        <div className="space-y-3">
          {mockFeedItems.map((feed) => {
            const titleId = `${feed.id}-title`;
            const interactionNoteId = `${feed.id}-interaction-note`;

            return (
              <Surface key={feed.id} as="article" className="overflow-hidden">
                <header className="flex flex-col gap-3 border-b border-zinc-200 px-4 py-3 sm:flex-row sm:items-center sm:justify-between dark:border-zinc-800">
                  <div className="flex min-w-0 items-center gap-3">
                    <img
                      src={feed.authorAvatar}
                      alt={`${feed.authorName} 的头像`}
                      className="h-9 w-9 shrink-0 rounded-full border border-zinc-200 object-cover dark:border-zinc-800"
                    />
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold text-zinc-950 dark:text-zinc-50">
                        {feed.authorName}
                      </div>
                      <div className="flex flex-wrap items-center gap-x-1.5 text-[11px] leading-5 text-zinc-500 dark:text-zinc-400">
                        <span>{feed.authorRole}</span>
                        <span aria-hidden="true">·</span>
                        <time>{feed.publishedAt}</time>
                      </div>
                    </div>
                  </div>
                  <span className="inline-flex w-fit items-center rounded-md border border-violet-200 bg-violet-50 px-2 py-1 text-[11px] font-medium text-violet-600 dark:border-violet-500/25 dark:bg-violet-500/10 dark:text-violet-300">
                    #{feed.tag}
                  </span>
                </header>

                <div className="px-4 py-4">
                  <h3
                    id={titleId}
                    className="text-base font-semibold leading-6 tracking-[-0.01em] text-zinc-950 dark:text-zinc-50"
                  >
                    {feed.title}
                  </h3>
                  <p className="mt-2 text-sm leading-6 text-zinc-600 dark:text-zinc-300">
                    {feed.content}
                  </p>
                </div>

                {feed.campaignLink ? (
                  <div className="flex flex-col gap-2 border-t border-zinc-200 bg-zinc-50/70 px-4 py-2.5 text-xs sm:flex-row sm:items-center sm:justify-between dark:border-zinc-800 dark:bg-zinc-900/60">
                    <span className="inline-flex items-center gap-2 font-medium text-zinc-600 dark:text-zinc-300">
                      <Sparkles className="h-3.5 w-3.5 text-amber-500" aria-hidden="true" />
                      关联增长营销活动
                    </span>
                    <Link
                      to={feed.campaignLink}
                      className="inline-flex w-fit items-center gap-1 font-medium text-violet-600 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:ring-offset-2 dark:text-violet-300 dark:ring-offset-zinc-950"
                    >
                      查看活动详情
                      <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
                    </Link>
                  </div>
                ) : null}

                <footer className="flex flex-col gap-1 border-t border-zinc-200 px-2 py-2 sm:flex-row sm:items-center sm:justify-between dark:border-zinc-800">
                  <div className="flex flex-wrap items-center">
                    <DemoAction
                      count={feed.likes}
                      icon={ThumbsUp}
                      label="点赞"
                      noteId={interactionNoteId}
                    />
                    <DemoAction
                      count={feed.comments}
                      icon={MessageSquare}
                      label="评论"
                      noteId={interactionNoteId}
                    />
                    <DemoAction
                      count={feed.shares}
                      icon={Share2}
                      label="分享"
                      noteId={interactionNoteId}
                    />
                  </div>
                  <p
                    id={interactionNoteId}
                    className="px-2 text-[11px] leading-5 text-zinc-400 dark:text-zinc-600"
                  >
                    演示计数 · 互动未接入
                  </p>
                </footer>
              </Surface>
            );
          })}
        </div>
      </section>
    </div>
  );
}
