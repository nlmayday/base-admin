import { http } from "@/utils/http";
import { compactParams, type Page, type PageQuery } from "@/api/types";

export type LoginLogKind = "login" | "logout";

/** 登录失败原因（后端 auth.Reason*） */
export type LoginFailReason =
  "bad_credentials" | "rate_limited" | "captcha" | "locked" | "error";

export type LoginLog = {
  id: string;
  kind: LoginLogKind;
  username: string;
  userId: string | null;
  success: boolean;
  reason: LoginFailReason | "";
  ip: string;
  userAgent: string;
  requestId: string;
  createdAt: string;
};

export type LoginLogFilter = {
  username?: string;
  /** "true" / "false"，空表示全部 */
  success?: "true" | "false" | "";
  ip?: string;
  kind?: LoginLogKind | "";
  /** RFC 3339 */
  startTime?: string;
  endTime?: string;
};

export const listLoginLogs = (query: PageQuery & LoginLogFilter) =>
  http.get<Page<LoginLog>>("/audit/login-logs", compactParams(query));

export const exportLoginLogs = (filter: LoginLogFilter) =>
  http.download(
    "/audit/login-logs/export",
    compactParams(filter),
    "login-logs.csv"
  );
