import {
  UserProfile,
  Campaign,
  GrowthFeedItem,
  LotteryPrize,
  Coupon,
  PointTransaction,
  McpServer,
  McpTool,
  AgentTask,
  AgentApproval
} from '../types/growthos';

export const mockUser: UserProfile = {
  id: 'usr_88921',
  name: 'Alex Rivera',
  email: 'alex.rivera@growthos.io',
  avatar: 'https://images.unsplash.com/photo-1534528741775-53994a69daeb?w=150&h=150&fit=crop&auto=format',
  role: 'admin',
  points: 12450,
  level: 'Platinum Growth Tier',
  unreadNotifications: 3,
};

export const mockCampaigns: Campaign[] = [
  {
    id: 'cmp_001',
    title: 'Spring Growth Surge 2026',
    subtitle: 'Invite 3 developers to get 5,000 growth points + VIP Pro tier pass.',
    status: 'active',
    category: 'Viral Referral',
    participants: 14280,
    rewardType: 'points',
    rewardAmount: '+5,000 PTS',
    startDate: '2026-03-01',
    endDate: '2026-04-15',
    bannerBg: 'from-blue-600 to-indigo-900',
    conversionRate: 18.4,
    budgetSpent: 12400,
    totalBudget: 25000,
  },
  {
    id: 'cmp_002',
    title: 'AI Agent Workflows Booster',
    subtitle: 'Deploy your first MCP Tool automation workflow to claim 20% Rebate.',
    status: 'active',
    category: 'AI Onboarding',
    participants: 8930,
    rewardType: 'rebate',
    rewardAmount: '20% Off',
    startDate: '2026-03-10',
    endDate: '2026-03-31',
    bannerBg: 'from-purple-600 to-violet-950',
    conversionRate: 24.1,
    budgetSpent: 8900,
    totalBudget: 15000,
  },
  {
    id: 'cmp_003',
    title: 'Wheel of Fortune Lucky Draw',
    subtitle: 'Spin daily for a chance to win 10,000 Points or Cloud Credits.',
    status: 'active',
    category: 'Gamification',
    participants: 32100,
    rewardType: 'gift',
    rewardAmount: 'Up to $500',
    startDate: '2026-02-15',
    endDate: '2026-04-30',
    bannerBg: 'from-emerald-600 to-teal-900',
    conversionRate: 31.2,
    budgetSpent: 45000,
    totalBudget: 50000,
  },
  {
    id: 'cmp_004',
    title: 'Q1 Enterprise API Slash',
    subtitle: 'Special coupon campaign for growth teams upgrading to MCP Enterprise Gateway.',
    status: 'draft',
    category: 'B2B Sales',
    participants: 0,
    rewardType: 'coupon',
    rewardAmount: '$200 OFF',
    startDate: '2026-04-01',
    endDate: '2026-05-01',
    bannerBg: 'from-amber-600 to-orange-900',
    conversionRate: 0,
    budgetSpent: 0,
    totalBudget: 30000,
  }
];

export const mockFeedItems: GrowthFeedItem[] = [
  {
    id: 'feed_101',
    authorName: 'Sarah Chen',
    authorAvatar: 'https://images.unsplash.com/photo-1517841905240-472988babdf9?w=120&h=120&fit=crop&auto=format',
    authorRole: 'Head of Growth @ SaaSify',
    publishedAt: '12 mins ago',
    title: 'How we increased activation rate by 38% with GrowthOS AI Marketing Agent',
    content: 'By replacing manually triggered onboarding emails with dynamic MCP Agent tools that trigger personalized in-app reward popups when users drop off at step 3, our conversion skyrocketed.',
    tag: 'Case Study',
    likes: 342,
    shares: 89,
    comments: 24,
    isLiked: true,
    campaignLink: '/campaigns/cmp_002'
  },
  {
    id: 'feed_102',
    authorName: 'GrowthOS AI Bot',
    authorAvatar: 'https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?w=120&h=120&fit=crop&auto=format',
    authorRole: 'System Optimization Insights',
    publishedAt: '1 hour ago',
    title: 'A/B Experiment Alert: Variant B (Gamified Referral) outperforming Control by +42%',
    content: 'Experiment "Ref-AB-99" reached statistical significance (p < 0.01). Automated recommendation: Shift 80% of campaign traffic budget to Variant B immediately.',
    tag: 'A/B Experiment',
    likes: 512,
    shares: 140,
    comments: 45,
    campaignLink: '/campaigns/cmp_001'
  }
];

export const mockLotteryPrizes: LotteryPrize[] = [
  { id: '1', name: '5,000 Points', type: 'points', icon: 'Coins', amount: '5000', probability: 0.05, color: '#f59e0b' },
  { id: '2', name: '$50 Coupon', type: 'coupon', icon: 'Ticket', amount: '$50', probability: 0.1, color: '#3b82f6' },
  { id: '3', name: '100 Points', type: 'points', icon: 'Sparkles', amount: '100', probability: 0.4, color: '#10b981' },
  { id: '4', name: 'MacBook M3 Pro', type: 'physical', icon: 'Gift', amount: '1 Unit', probability: 0.001, color: '#ec4899' },
  { id: '5', name: '$10 Rebate', type: 'coupon', icon: 'Tag', amount: '$10', probability: 0.2, color: '#8b5cf6' },
  { id: '6', name: 'Try Again', type: 'empty', icon: 'Smile', amount: '0', probability: 0.249, color: '#64748b' },
];

export const mockCoupons: Coupon[] = [
  { id: 'cpn_1', code: 'GROWTH2026', title: '$50 Off Pro Plan Upgrade', discount: '$50.00', minSpend: '$100.00', expiryDate: '2026-04-30', status: 'available', category: 'Subscription' },
  { id: 'cpn_2', code: 'AGENTVIP', title: 'Free 1,000 AI Agent Credits', discount: '100% OFF', minSpend: '$0.00', expiryDate: '2026-05-15', status: 'available', category: 'AI Credits' },
  { id: 'cpn_3', code: 'SPRING20', title: '20% Rebate on MCP Gateway', discount: '20% OFF', minSpend: '$200.00', expiryDate: '2026-03-20', status: 'used', category: 'Gateway' },
];

export const mockPointTransactions: PointTransaction[] = [
  { id: 'tx_1', type: 'earn', title: 'Completed Spring Campaign Referral Task', amount: 500, date: '2026-03-14 10:22', status: 'completed' },
  { id: 'tx_2', type: 'earn', title: 'Daily Check-in Streak Reward', amount: 50, date: '2026-03-14 09:00', status: 'completed' },
  { id: 'tx_3', type: 'spend', title: 'Redeemed $20 Coupon Voucher', amount: -2000, date: '2026-03-12 16:45', status: 'completed' },
  { id: 'tx_4', type: 'earn', title: 'AI Operator Workflow Execution Bonus', amount: 1200, date: '2026-03-10 11:30', status: 'completed' },
];

export const mockMcpServers: McpServer[] = [
  { id: 'srv_1', name: 'growthos-analytics-mcp', version: 'v1.4.2', status: 'online', endpoint: 'https://mcp.growthos.io/analytics', toolsCount: 14, avgLatencyMs: 28, activeRequests: 142 },
  { id: 'srv_2', name: 'growthos-marketing-agent-mcp', version: 'v2.1.0', status: 'online', endpoint: 'https://mcp.growthos.io/agent', toolsCount: 8, avgLatencyMs: 45, activeRequests: 89 },
  { id: 'srv_3', name: 'growthos-user-behavior-mcp', version: 'v0.9.8', status: 'degraded', endpoint: 'https://mcp.growthos.io/behavior', toolsCount: 6, avgLatencyMs: 210, activeRequests: 12 },
];

export const mockMcpTools: McpTool[] = [
  { id: 'tool_1', serverId: 'srv_1', serverName: 'growthos-analytics-mcp', name: 'calculate_funnel_conversion', description: 'Calculates multi-stage conversion rates & friction drop-offs across user cohorts.', riskLevel: 'low', invocations24h: 12450, successRate: 99.8 },
  { id: 'tool_2', serverId: 'srv_2', serverName: 'growthos-marketing-agent-mcp', name: 'trigger_points_reward', description: 'Dispatches instant point grants or coupon vouchers directly to user accounts.', riskLevel: 'high', invocations24h: 890, successRate: 98.5 },
  { id: 'tool_3', serverId: 'srv_2', serverName: 'growthos-marketing-agent-mcp', name: 'update_campaign_budget', description: 'Dynamically reallocates marketing campaign spending limits.', riskLevel: 'critical', invocations24h: 142, successRate: 100.0 },
];

export const mockAgentTasks: AgentTask[] = [
  {
    id: 'tsk_901',
    title: 'Optimize Spring Referral Funnel Conversion Drop-off',
    status: 'running',
    agentName: 'GrowthAgent-Alpha',
    prompt: 'Analyze conversion drop-off on /campaigns/cmp_001, run A/B test simulation, and adjust user reward threshold.',
    startedAt: '2026-03-14 14:10:00',
    mcpToolsUsed: ['calculate_funnel_conversion', 'fetch_user_cohort'],
    riskLevel: 'medium'
  },
  {
    id: 'tsk_902',
    title: 'Bulk Issue $50 VIP Upgrade Coupons to Inactive Platinum Cohort',
    status: 'waiting_approval',
    agentName: 'GrowthAgent-Promo',
    prompt: 'Query churn-risk Platinum users (>30 days inactive) and grant 1,000 $50 discount vouchers.',
    startedAt: '2026-03-14 14:22:15',
    mcpToolsUsed: ['query_user_behavior', 'trigger_points_reward'],
    riskLevel: 'high'
  }
];

export const mockAgentApprovals: AgentApproval[] = [
  {
    id: 'app_301',
    taskId: 'tsk_902',
    taskTitle: 'Bulk Issue $50 VIP Upgrade Coupons to Inactive Platinum Cohort',
    toolName: 'trigger_points_reward',
    parameters: { cohort_size: 1000, voucher_value: '$50', estimated_cost: '$50,000' },
    requestedBy: 'GrowthAgent-Promo',
    requestedAt: '2026-03-14 14:22:18',
    riskLevel: 'high',
    status: 'pending'
  }
];
