import { createBrowserRouter } from 'react-router';

import { UserLayout } from '../layouts/UserLayout';
import { AdminLayout } from '../layouts/AdminLayout';
import { McpLayout } from '../layouts/McpLayout';
import { AgentLayout } from '../layouts/AgentLayout';
import { AuthLayout } from '../layouts/AuthLayout';

import { UserHomePage } from '../pages/user/home/UserHomePage';
import { GrowthFeedPage } from '../pages/user/growth-feed/GrowthFeedPage';
import { CampaignsListPage } from '../pages/user/campaigns/CampaignsListPage';
import { CampaignDetailPage } from '../pages/user/campaigns/CampaignDetailPage';
import { LotteryPage } from '../pages/user/lottery/LotteryPage';
import { PointsPage } from '../pages/user/points/PointsPage';
import { CouponsPage } from '../pages/user/coupons/CouponsPage';
import { UserProfilePage } from '../pages/user/profile/UserProfilePage';

import { AdminDashboardPage } from '../pages/admin/dashboard/AdminDashboardPage';
import { AdminCampaignsPage } from '../pages/admin/campaigns/AdminCampaignsPage';
import { GenericAdminModulePage } from '../pages/admin/GenericAdminModulePage';

import { McpDashboardPage } from '../pages/mcp/McpDashboardPage';
import { GenericMcpPage } from '../pages/mcp/GenericMcpPage';

import { AgentWorkspacePage } from '../pages/agent/workspace/AgentWorkspacePage';
import { GenericAgentPage } from '../pages/agent/GenericAgentPage';

import {
  StatusPage,
  Error403Page,
  Error404Page,
  Error500Page,
  LoginPage,
} from '../pages/system/SystemPages';

import {
  Target,
  Trophy,
  Users,
  Coins,
  Ticket,
  Percent,
  Rss,
  Activity,
  FlaskConical,
  BarChart3,
  CheckSquare,
  ShieldAlert,
  Server,
  Wrench,
  ShieldCheck,
  FileCode,
  ListTodo,
  CheckCircle2,
  History,
} from 'lucide-react';

export const router = createBrowserRouter([
  {
    path: '/',
    Component: UserLayout,
    children: [
      { index: true, Component: UserHomePage },
      { path: 'home', Component: UserHomePage },
      { path: 'feed', Component: GrowthFeedPage },
      { path: 'campaigns', Component: CampaignsListPage },
      { path: 'campaigns/:id', Component: CampaignDetailPage },
      { path: 'lottery', Component: LotteryPage },
      { path: 'points', Component: PointsPage },
      { path: 'coupons', Component: CouponsPage },
      { path: 'rewards', Component: PointsPage },
      { path: 'profile', Component: UserProfilePage },
    ],
  },
  {
    path: '/admin',
    Component: AdminLayout,
    children: [
      { index: true, Component: AdminDashboardPage },
      { path: 'dashboard', Component: AdminDashboardPage },
      { path: 'campaigns', Component: AdminCampaignsPage },
      { path: 'strategies', Component: () => <GenericAdminModulePage title="抽奖策略" subtitle="配置抽奖概率与策略控制" icon={Target} /> },
      { path: 'awards', Component: () => <GenericAdminModulePage title="奖品库管理" subtitle="维护实物与虚拟权益奖品" icon={Trophy} /> },
      { path: 'accounts', Component: () => <GenericAdminModulePage title="用户账户" subtitle="查看全网用户积分与账号" icon={Users} /> },
      { path: 'points', Component: () => <GenericAdminModulePage title="积分策略" subtitle="积分发发放与规则引擎" icon={Coins} /> },
      { path: 'coupons', Component: () => <GenericAdminModulePage title="优惠券中心" subtitle="优惠券配置与发放记录" icon={Ticket} /> },
      { path: 'rebates', Component: () => <GenericAdminModulePage title="返利中心" subtitle="裂变佣金与比例配置" icon={Percent} /> },
      { path: 'feed', Component: () => <GenericAdminModulePage title="Feed 管理" subtitle="审核与推荐社区 Feed" icon={Rss} /> },
      { path: 'behavior', Component: () => <GenericAdminModulePage title="用户行为采集" subtitle="实时行为埋点与 SDK 日志" icon={Activity} /> },
      { path: 'experiments', Component: () => <GenericAdminModulePage title="A/B 实验中心" subtitle="分流算法与显著性检验" icon={FlaskConical} /> },
      { path: 'analytics', Component: () => <GenericAdminModulePage title="数据分析" subtitle="深度漏斗分析与 Cohort 留存" icon={BarChart3} /> },
      { path: 'tasks', Component: () => <GenericAdminModulePage title="定时任务" subtitle="GrowthOS CronJob 规则" icon={CheckSquare} /> },
      { path: 'audit', Component: () => <GenericAdminModulePage title="审计日志" subtitle="高风险敏感操作审计" icon={ShieldAlert} /> },
    ],
  },
  {
    path: '/mcp',
    Component: McpLayout,
    children: [
      { index: true, Component: McpDashboardPage },
      { path: 'dashboard', Component: McpDashboardPage },
      { path: 'servers', Component: () => <GenericMcpPage title="MCP 服务节点" subtitle="管理关联的微服务 MCP Server" icon={Server} /> },
      { path: 'tools', Component: () => <GenericMcpPage title="Tools 工具库" subtitle="管理已暴露给 Agent 的工具" icon={Wrench} /> },
      { path: 'permissions', Component: () => <GenericMcpPage title="安全与权限" subtitle="MCP Tool 风险等级策略" icon={ShieldCheck} /> },
      { path: 'audit', Component: () => <GenericMcpPage title="调用审计" subtitle="查看 Tool 详细输入输出 Trace" icon={FileCode} /> },
    ],
  },
  {
    path: '/agent',
    Component: AgentLayout,
    children: [
      { index: true, Component: AgentWorkspacePage },
      { path: 'workspace', Component: AgentWorkspacePage },
      { path: 'tasks', Component: () => <GenericAgentPage title="任务队列" subtitle="查看 Agent 正在运行与排队中的 Task" icon={ListTodo} /> },
      { path: 'approvals', Component: () => <GenericAgentPage title="人工审批中心" subtitle="高风险 MCP 工具调用人工确认" icon={CheckCircle2} /> },
      { path: 'history', Component: () => <GenericAgentPage title="执行历史" subtitle="归档与审计历史 AI 执行记录" icon={History} /> },
    ],
  },
  {
    path: '/',
    Component: AuthLayout,
    children: [
      { path: 'login', Component: LoginPage },
    ],
  },
  { path: '/system/status', Component: StatusPage },
  { path: '/403', Component: Error403Page },
  { path: '/500', Component: Error500Page },
  { path: '*', Component: Error404Page },
]);
