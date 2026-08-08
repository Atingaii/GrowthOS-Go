export interface UserProfile {
  id: string;
  name: string;
  email: string;
  avatar: string;
  role: 'user' | 'admin' | 'growth_operator' | 'mcp_admin';
  points: number;
  level: string;
  unreadNotifications: number;
}

export interface Campaign {
  id: string;
  title: string;
  subtitle: string;
  status: 'active' | 'draft' | 'paused' | 'ended';
  category: string;
  participants: number;
  rewardType: 'points' | 'coupon' | 'gift' | 'rebate';
  rewardAmount: string;
  startDate: string;
  endDate: string;
  bannerBg: string;
  conversionRate: number;
  budgetSpent: number;
  totalBudget: number;
}

export interface GrowthFeedItem {
  id: string;
  authorName: string;
  authorAvatar: string;
  authorRole: string;
  publishedAt: string;
  title: string;
  content: string;
  tag: string;
  likes: number;
  shares: number;
  comments: number;
  isLiked?: boolean;
  campaignLink?: string;
}

export interface LotteryPrize {
  id: string;
  name: string;
  type: 'points' | 'coupon' | 'physical' | 'empty';
  icon: string;
  amount: string;
  probability: number;
  color: string;
}

export interface Coupon {
  id: string;
  code: string;
  title: string;
  discount: string;
  minSpend: string;
  expiryDate: string;
  status: 'available' | 'used' | 'expired';
  category: string;
}

export interface PointTransaction {
  id: string;
  type: 'earn' | 'spend';
  title: string;
  amount: number;
  date: string;
  status: 'completed' | 'pending';
}

export interface AnalyticsMetric {
  title: string;
  value: string | number;
  change: number;
  trend: 'up' | 'down';
  period: string;
}

export interface McpServer {
  id: string;
  name: string;
  version: string;
  status: 'online' | 'degraded' | 'offline';
  endpoint: string;
  toolsCount: number;
  avgLatencyMs: number;
  activeRequests: number;
}

export interface McpTool {
  id: string;
  serverId: string;
  serverName: string;
  name: string;
  description: string;
  riskLevel: 'low' | 'medium' | 'high' | 'critical';
  invocations24h: number;
  successRate: number;
}

export interface AgentTask {
  id: string;
  title: string;
  status: 'running' | 'waiting_approval' | 'completed' | 'failed';
  agentName: string;
  prompt: string;
  startedAt: string;
  mcpToolsUsed: string[];
  riskLevel: 'low' | 'medium' | 'high';
}

export interface AgentApproval {
  id: string;
  taskId: string;
  taskTitle: string;
  toolName: string;
  parameters: Record<string, any>;
  requestedBy: string;
  requestedAt: string;
  riskLevel: 'medium' | 'high' | 'critical';
  status: 'pending' | 'approved' | 'rejected';
}
