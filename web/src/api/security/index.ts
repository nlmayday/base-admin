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
  /** 允许登录的 IP 或网段（CIDR），空表示不限制 */
  ipAllowlist: string[];
  /** 不受 IP 白名单和登录时段限制的角色编码 */
  loginExemptRoles: string[];
  /** 允许登录时段 HH:mm，都为空表示不限制；结束早于开始表示跨过午夜 */
  loginTimeStart: string;
  loginTimeEnd: string;
  loginTimeZone: string;
  version: number;
};

/** 会话超时由服务端配置决定，这里只读展示 */
export type SecurityPolicyView = SecurityPolicy & {
  sessionIdleMinutes: number;
  sessionAbsoluteMinutes: number;
  /** 服务端看到的当前请求 IP */
  clientIp: string;
};

export type PolicyRole = { code: string; name: string };

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

export const listPolicyRoles = () => http.get<PolicyRole[]>("/security/roles");

export const unlockAccount = (username: string) =>
  http.post<null>("/security/unlock", { username });
