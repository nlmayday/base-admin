import { http } from "@/utils/http";
import { compactParams, type Page, type PageQuery } from "@/api/types";
import type { RunStatus } from "./job";

export type RunTrigger = "cron" | "manual";

export type JobRun = {
  id: string;
  /** 任务已删除时为 null */
  jobId: string | null;
  jobName: string;
  handlerKey: string;
  trigger: RunTrigger;
  status: RunStatus;
  /** 已进行的尝试次数（含重试） */
  attempt: number;
  /** Cron 计划触发时间；手动执行为 null */
  scheduledAt: string | null;
  startedAt: string;
  finishedAt: string | null;
  durationMs: number | null;
  error: string;
  instance: string;
  triggeredBy: string | null;
};

export type RunFilter = {
  jobId?: string;
  status?: RunStatus | "";
  trigger?: RunTrigger | "";
  /** RFC 3339，含 */
  startFrom?: string;
  /** RFC 3339，不含 */
  startTo?: string;
};

const enc = encodeURIComponent;

export const listRuns = (query: PageQuery & RunFilter) =>
  http.get<Page<JobRun>>("/job-runs", compactParams(query));

export const getRun = (id: string) => http.get<JobRun>(`/job-runs/${enc(id)}`);
