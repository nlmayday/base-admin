import { http } from "@/utils/http";
import type { Page, PageQuery } from "@/api/types";

export type OnlineSession = {
  /** 会话标识（服务端存储的哈希，不是会话令牌） */
  id: string;
  userId: string;
  username: string;
  nickname: string;
  ip: string;
  userAgent: string;
  createdAt: string;
  lastSeenAt: string;
  absoluteExpiresAt: string;
  /** 是否为当前浏览器的会话 */
  current: boolean;
};

export type CaptchaMode = "off" | "after_failures" | "always";

export type SecurityPolicy = {
  passwordMinLength: number;
  passwordRequireUpper: boolean;
  passwordRequireLower: boolean;
  passwordRequireDigit: boolean;
  passwordRequireSymbol: boolean;
  captchaMode: CaptchaMode;
  captchaAfterFailures: number;
  lockoutThreshold: number;
  lockoutMinutes: number;
  version: number;
};

/** 会话超时由服务端配置决定，这里只读展示 */
export type SecurityPolicyView = SecurityPolicy & {
  sessionIdleMinutes: number;
  sessionAbsoluteMinutes: number;
};

const enc = encodeURIComponent;

export const listSessions = (query: PageQuery & { keyword?: string }) =>
  http.get<Page<OnlineSession>>("/sessions", query);

export const revokeSession = (id: string) =>
  http.delete<null>(`/sessions/${enc(id)}`);

export const revokeUserSessions = (userId: string) =>
  http.delete<null>(`/sessions/users/${enc(userId)}`);

export const getPolicy = () => http.get<SecurityPolicyView>("/security/policy");

export const updatePolicy = (data: SecurityPolicy) =>
  http.put<SecurityPolicyView>("/security/policy", data);

export const unlockAccount = (username: string) =>
  http.post<null>("/security/unlock", { username });
