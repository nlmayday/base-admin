import { http } from "@/utils/http";
import { compactParams, type Page, type PageQuery } from "@/api/types";

/** 执行状态；空字符串表示从未执行 */
export type RunStatus =
  "running" | "success" | "failed" | "timeout" | "skipped";

export type Job = {
  id: string;
  name: string;
  handlerKey: string;
  handlerName: string;
  /** 处理器是否仍在当前版本中注册 */
  handlerExists: boolean;
  cronExpr: string;
  params: Record<string, unknown>;
  enabled: boolean;
  /** 0 表示使用处理器默认超时 */
  timeoutSeconds: number;
  maxRetries: number;
  remark: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  lastRunAt: string | null;
  lastStatus: RunStatus | "";
  /** 停用时为 null */
  nextRunAt: string | null;
};

export type JobHandler = {
  key: string;
  name: string;
  description: string;
  defaultTimeoutSeconds: number;
};

export type JobFilter = {
  keyword?: string;
  handlerKey?: string;
  enabled?: "true" | "false" | "";
};

export type JobPayload = {
  name: string;
  handlerKey: string;
  cronExpr: string;
  params: Record<string, unknown>;
  enabled: boolean;
  timeoutSeconds: number;
  maxRetries: number;
  remark: string;
  /** 编辑时必填 */
  version?: number;
};

export type CronPreview = { expr: string; nextTimes: string[] };

export type RunNowResult = { runId: string; status: RunStatus };

const enc = encodeURIComponent;

export const listJobs = (query: PageQuery & JobFilter) =>
  http.get<Page<Job>>("/jobs", compactParams(query));

export const getJob = (id: string) => http.get<Job>(`/jobs/${enc(id)}`);

export const listJobHandlers = () => http.get<JobHandler[]>("/jobs/handlers");

/** 预览 Cron 表达式；校验错误由调用方展示，不弹全局提示 */
export const previewCron = (expr: string) =>
  http.post<CronPreview>(
    "/jobs/cron/preview",
    { expr },
    { skipErrorHandler: true }
  );

export const createJob = (data: JobPayload) => http.post<Job>("/jobs", data);

export const updateJob = (id: string, data: JobPayload) =>
  http.put<Job>(`/jobs/${enc(id)}`, data);

export const setJobEnabled = (id: string, enabled: boolean, version: number) =>
  http.put<Job>(`/jobs/${enc(id)}/status`, { enabled, version });

export const deleteJob = (id: string) => http.delete<null>(`/jobs/${enc(id)}`);

/** 异步执行，立即返回执行记录 ID；任务正在运行时返回 skipped */
export const runJobNow = (id: string) =>
  http.post<RunNowResult>(`/jobs/${enc(id)}/run`);
