import { useRef, useState, type FormEvent } from "react";
import { AlertTriangle, Bot, CheckCircle2, Send, Sparkles } from "lucide-react";
import { PageHeader, Surface } from "../../../components/common/ProductPage";
import { StatusBadge } from "../../../components/common/UIComponents";
import {
  MOCK_SNAPSHOT_LABEL,
  mockAgentApprovals,
  mockAgentTasks,
} from "../../../mocks/growthOsMockData";
import type { AgentTask } from "../../../types/growthos";

type WorkspaceTask = AgentTask | (Omit<AgentTask, "status"> & { status: "local_only" });

export function AgentWorkspacePage() {
  const [prompt, setPrompt] = useState("");
  const [tasks, setTasks] = useState<WorkspaceTask[]>(mockAgentTasks);
  const [announcement, setAnnouncement] = useState("");
  const demoSequenceRef = useRef(0);

  const handleCreateDemoTask = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedPrompt = prompt.trim();
    if (!normalizedPrompt) {
      return;
    }

    demoSequenceRef.current += 1;
    const demoTask = {
      id: `local_${Date.now().toString(36)}_${demoSequenceRef.current}`,
      title: normalizedPrompt.length > 34 ? `${normalizedPrompt.slice(0, 34)}…` : normalizedPrompt,
      status: "local_only" as const,
      agentName: "Local-Demo-Agent",
      prompt: normalizedPrompt,
      startedAt: "尚未发送至后端",
      mcpToolsUsed: [] as string[],
      riskLevel: "medium" as const,
    };
    setTasks((currentTasks) => [demoTask, ...currentTasks]);
    setAnnouncement(`已将“${demoTask.title}”加入本地演示队列；没有发送到后端。`);
    setPrompt("");
  };

  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="AI Operator · Human in the Loop"
        title="AI Operator 工作台"
        description={`浏览截至 ${MOCK_SNAPSHOT_LABEL} 的任务样例，并可在浏览器内编排本地任务。当前不会调用 Agent、MCP Tool 或 GrowthOS 写接口，也不会执行审批。`}
        badge="本地演示"
      />

      <Surface className="p-4 sm:p-5">
        <form onSubmit={handleCreateDemoTask} className="space-y-3">
          <label
            htmlFor="agent-demo-prompt"
            className="flex items-center gap-2 text-sm font-medium text-zinc-800 dark:text-zinc-200"
          >
            <Sparkles className="h-4 w-4 text-violet-500" aria-hidden="true" /> 创建本地演示任务
          </label>
          <textarea
            id="agent-demo-prompt"
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            placeholder="例如：分析活动转化流失并生成一份建议。此处只创建浏览器内演示任务。"
            rows={3}
            maxLength={500}
            className="w-full resize-y rounded-lg border border-zinc-200 bg-white p-3 text-sm text-zinc-900 outline-none placeholder:text-zinc-400 focus:border-violet-400 focus:ring-2 focus:ring-violet-500/15 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-100"
          />
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-[11px] text-zinc-400">
              最多 500 字；提交只更新当前页面内存，刷新后丢失，不代表 Agent 已执行。
            </p>
            <button
              type="submit"
              disabled={!prompt.trim()}
              className="inline-flex h-10 items-center justify-center gap-2 rounded-lg bg-violet-600 px-4 text-sm font-medium text-white hover:bg-violet-500 disabled:cursor-not-allowed disabled:bg-zinc-200 disabled:text-zinc-400 dark:disabled:bg-zinc-800"
            >
              <Send className="h-4 w-4" aria-hidden="true" /> 加入本地演示队列
            </button>
          </div>
        </form>
        <p className="sr-only" role="status" aria-live="polite">
          {announcement}
        </p>
      </Surface>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,2fr)_minmax(300px,1fr)]">
        <section aria-labelledby="agent-demo-tasks-title" className="space-y-2">
          <div className="flex items-center justify-between border-b border-zinc-200 pb-2 dark:border-zinc-800">
            <h2
              id="agent-demo-tasks-title"
              className="flex items-center gap-2 text-sm font-semibold text-zinc-900 dark:text-zinc-100"
            >
              <Bot className="h-4 w-4 text-violet-500" aria-hidden="true" /> 演示任务
            </h2>
            <span className="text-xs tabular-nums text-zinc-400">{tasks.length} 条</span>
          </div>

          <div className="space-y-2">
            {tasks.map((task) => (
              <Surface as="article" key={task.id} className="p-4">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0">
                    <div className="font-mono text-[10px] text-zinc-400">{task.id}</div>
                    <h3 className="mt-0.5 text-sm font-medium text-zinc-900 dark:text-zinc-100">
                      {task.title}
                    </h3>
                  </div>
                  <StatusBadge
                    status={task.status}
                    label={
                      task.status === "local_only"
                        ? "仅本地"
                        : task.status === "running"
                          ? "演示 · 运行中"
                          : task.status === "waiting_approval"
                            ? "演示 · 待审批"
                            : `演示 · ${task.status}`
                    }
                  />
                </div>
                <p className="mt-3 rounded-lg bg-zinc-50 p-3 text-xs leading-5 text-zinc-600 dark:bg-zinc-900 dark:text-zinc-300">
                  {task.prompt}
                </p>
                <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-[11px] text-zinc-400">
                  <span>{task.agentName}</span>
                  <span>{task.startedAt}</span>
                </div>
              </Surface>
            ))}
          </div>
        </section>

        <section aria-labelledby="agent-approvals-title" className="space-y-2">
          <div className="flex items-center justify-between border-b border-zinc-200 pb-2 dark:border-zinc-800">
            <h2
              id="agent-approvals-title"
              className="flex items-center gap-2 text-sm font-semibold text-rose-600"
            >
              <AlertTriangle className="h-4 w-4" aria-hidden="true" /> 待人工审批示例
            </h2>
            <span className="text-xs text-zinc-400">未接入</span>
          </div>

          {mockAgentApprovals.map((approval) => (
            <Surface
              as="article"
              key={approval.id}
              className="border-rose-200 p-4 dark:border-rose-900/60"
            >
              <div className="flex items-center justify-between gap-3">
                <span className="rounded-full bg-rose-50 px-2 py-0.5 text-[10px] font-medium uppercase text-rose-600 dark:bg-rose-500/10 dark:text-rose-300">
                  {approval.riskLevel} risk
                </span>
                <span className="font-mono text-[10px] text-zinc-400">{approval.requestedBy}</span>
              </div>
              <h3 className="mt-3 text-sm font-medium leading-5 text-zinc-900 dark:text-zinc-100">
                {approval.taskTitle}
              </h3>
              <dl className="mt-3 space-y-1 rounded-lg bg-zinc-50 p-3 font-mono text-[10px] text-zinc-500 dark:bg-zinc-900 dark:text-zinc-400">
                <div className="flex gap-2">
                  <dt>Tool</dt>
                  <dd className="break-all text-violet-600 dark:text-violet-300">
                    {approval.toolName}
                  </dd>
                </div>
                <div className="flex gap-2">
                  <dt>Params</dt>
                  <dd className="break-all">{JSON.stringify(approval.parameters)}</dd>
                </div>
              </dl>
              <div className="mt-3 grid grid-cols-2 gap-2">
                <button
                  type="button"
                  disabled
                  className="inline-flex h-9 cursor-not-allowed items-center justify-center gap-1.5 rounded-lg bg-zinc-100 text-xs font-medium text-zinc-400 dark:bg-zinc-900 dark:text-zinc-600"
                >
                  <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" /> 批准 · 未接入
                </button>
                <button
                  type="button"
                  disabled
                  className="h-9 cursor-not-allowed rounded-lg bg-zinc-100 text-xs font-medium text-zinc-400 dark:bg-zinc-900 dark:text-zinc-600"
                >
                  驳回 · 未接入
                </button>
              </div>
            </Surface>
          ))}
        </section>
      </div>
    </div>
  );
}
