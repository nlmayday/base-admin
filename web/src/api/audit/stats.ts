import { http } from "@/utils/http";

export type AuditDayStats = {
  loginSuccess: number;
  loginFailure: number;
  operations: number;
  operationFailures: number;
};

export type AuditTrendPoint = AuditDayStats & {
  /** 服务器本地日期 YYYY-MM-DD */
  date: string;
};

export type AuditStats = {
  today: AuditDayStats;
  /** 近 7 天（含今天），按日期升序 */
  trend: AuditTrendPoint[];
};

/** 任一审计查看权限即可访问 */
export const getAuditStats = () => http.get<AuditStats>("/audit/stats");
