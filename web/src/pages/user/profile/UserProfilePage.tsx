import { Coins, Mail, UserRound } from "lucide-react";
import { PageHeader, SectionHeader, Surface } from "../../../components/common/ProductPage";
import { MOCK_SNAPSHOT_LABEL } from "../../../mocks/growthOsMockData";
import { useAppStore } from "../../../stores/appStore";

const levelLabel: Record<string, string> = {
  "Platinum Growth Tier": "铂金成长会员",
  "Gold Growth Tier": "黄金成长会员",
  "Silver Growth Tier": "白银成长会员",
  "Bronze Growth Tier": "青铜成长会员",
};

const roleLabel = {
  user: "普通用户",
  admin: "管理员",
  growth_operator: "增长运营",
  mcp_admin: "MCP 管理员",
} as const;

export function UserProfilePage() {
  const user = useAppStore((state) => state.user);
  const localizedLevel = levelLabel[user.level] ?? "成长会员";

  return (
    <div className="space-y-8">
      <PageHeader
        titleId="profile-page-title"
        eyebrow="Identity / Local snapshot"
        title="个人中心"
        description={`本页只读展示截至 ${MOCK_SNAPSHOT_LABEL} 的本地 mockUser 资料，不会验证身份、修改账号或写入任何后端系统。`}
        actions={
          <span className="inline-flex h-7 items-center rounded-md border border-violet-200 bg-violet-50 px-2.5 text-xs font-medium text-violet-600 dark:border-violet-500/25 dark:bg-violet-500/10 dark:text-violet-300">
            演示资料
          </span>
        }
      />

      <section aria-labelledby="profile-identity-title" className="space-y-2">
        <SectionHeader
          titleId="profile-identity-title"
          title="身份资料"
          description="字段直接映射当前前端 Mock 模型，没有推断额外认证或安全状态。"
        />
        <Surface className="overflow-hidden">
          <div className="flex flex-col gap-4 border-b border-zinc-200 p-4 sm:flex-row sm:items-center dark:border-zinc-800">
            <img
              src={user.avatar}
              alt={`${user.name} 的头像`}
              className="h-16 w-16 rounded-xl border border-zinc-200 object-cover dark:border-zinc-800"
            />
            <div className="min-w-0">
              <h3 className="truncate text-xl font-semibold tracking-[-0.015em] text-zinc-950 dark:text-zinc-50">
                {user.name}
              </h3>
              <p className="mt-1 flex items-center gap-1.5 text-sm text-zinc-500 dark:text-zinc-400">
                <Mail className="h-4 w-4 shrink-0" aria-hidden="true" />
                <span className="truncate">{user.email}</span>
              </p>
            </div>
          </div>

          <dl className="grid sm:grid-cols-2">
            <div className="border-b border-zinc-100 px-4 py-3 sm:border-r dark:border-zinc-900">
              <dt className="text-[11px] font-medium uppercase tracking-[0.08em] text-zinc-400">
                用户 ID
              </dt>
              <dd className="mt-1 font-mono text-sm font-medium text-zinc-900 dark:text-zinc-100">
                {user.id}
              </dd>
            </div>
            <div className="border-b border-zinc-100 px-4 py-3 dark:border-zinc-900">
              <dt className="text-[11px] font-medium uppercase tracking-[0.08em] text-zinc-400">
                登录邮箱
              </dt>
              <dd className="mt-1 break-all text-sm font-medium text-zinc-900 dark:text-zinc-100">
                {user.email}
              </dd>
            </div>
            <div className="border-b border-zinc-100 px-4 py-3 sm:border-r dark:border-zinc-900">
              <dt className="text-[11px] font-medium uppercase tracking-[0.08em] text-zinc-400">
                演示角色
              </dt>
              <dd className="mt-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">
                {roleLabel[user.role]}
              </dd>
            </div>
            <div className="border-b border-zinc-100 px-4 py-3 dark:border-zinc-900">
              <dt className="text-[11px] font-medium uppercase tracking-[0.08em] text-zinc-400">
                会员等级
              </dt>
              <dd className="mt-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">
                {localizedLevel}
              </dd>
            </div>
            <div className="px-4 py-3 sm:border-r sm:border-zinc-100 dark:sm:border-zinc-900">
              <dt className="flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-[0.08em] text-zinc-400">
                <Coins className="h-3.5 w-3.5" aria-hidden="true" />
                演示积分
              </dt>
              <dd className="mt-1 text-sm font-medium tabular-nums text-zinc-900 dark:text-zinc-100">
                {user.points.toLocaleString()} PTS
              </dd>
            </div>
            <div className="border-t border-zinc-100 px-4 py-3 sm:border-t-0 dark:border-zinc-900">
              <dt className="text-[11px] font-medium uppercase tracking-[0.08em] text-zinc-400">
                未读通知样本
              </dt>
              <dd className="mt-1 text-sm font-medium tabular-nums text-zinc-900 dark:text-zinc-100">
                {user.unreadNotifications} 条
              </dd>
            </div>
          </dl>
        </Surface>
      </section>

      <section aria-labelledby="profile-boundary-title" className="space-y-2">
        <SectionHeader titleId="profile-boundary-title" title="数据边界" />
        <Surface className="p-4">
          <div className="flex items-start gap-3">
            <UserRound className="mt-0.5 h-4 w-4 shrink-0 text-violet-500" aria-hidden="true" />
            <div className="space-y-2 text-xs leading-5 text-zinc-500 dark:text-zinc-400">
              <p>资料来源固定为前端 mockUser；刷新页面不会形成可审计的账号变更记录。</p>
              <p>
                当前模型没有邮箱验证、密码、会话或多因素认证字段，因此本页不输出任何无法由数据证明的认证结论。
              </p>
            </div>
          </div>
        </Surface>
      </section>
    </div>
  );
}
