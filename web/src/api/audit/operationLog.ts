import { http } from "@/utils/http";
import { compactParams, type Page, type PageQuery } from "@/api/types";

export type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export type OperationLog = {
  id: string;
  userId: string | null;
  username: string;
  nickname: string;
  module: string;
  method: HttpMethod;
  /** 路由模板，如 /api/v1/users/:id */
  route: string;
  /** 实际请求路径 */
  path: string;
  /** 操作名称（接口文档中的 Summary） */
  summary: string;
  /** HTTP 状态码 */
  status: number;
  /** 业务错误码，0 表示成功 */
  code: number;
  success: boolean;
  durationMs: number;
  ip: string;
  userAgent: string;
  requestId: string;
  errorMessage: string;
  createdAt: string;
};

export type OperationLogDetail = OperationLog & {
  /** 脱敏后的 JSON 请求体（≤4 KB），未记录时为空或说明文字 */
  requestBody: string;
};

export type OperationLogFilter = {
  /** 用户名或昵称 */
  user?: string;
  module?: string;
  method?: HttpMethod | "";
  success?: "true" | "false" | "";
  /** 路径或操作名称 */
  path?: string;
  startTime?: string;
  endTime?: string;
};

const enc = encodeURIComponent;

export const listOperationLogs = (query: PageQuery & OperationLogFilter) =>
  http.get<Page<OperationLog>>("/audit/operation-logs", compactParams(query));

export const getOperationLog = (id: string) =>
  http.get<OperationLogDetail>(`/audit/operation-logs/${enc(id)}`);

export const exportOperationLogs = (filter: OperationLogFilter) =>
  http.download(
    "/audit/operation-logs/export",
    compactParams(filter),
    "operation-logs.csv"
  );

/** 模块的中文名（取各模块目录菜单的标题） */
export type ModuleName = { name: string; title: string };

export const listOperationLogModules = () =>
  http.get<ModuleName[]>("/audit/operation-logs/modules");
