import { http } from "@/utils/http";

/** AI 接口密钥；完整密钥只在创建时返回一次 */
export type AgentKey = {
  id: string;
  name: string;
  /** 密钥开头几位，用于区分 */
  prefix: string;
  userId: string;
  /** 创建人，密钥以其身份调用 */
  userName: string;
  expiresAt: string | null;
  lastUsedAt: string | null;
  revokedAt: string | null;
  createdAt: string;
};

export type AgentKeyList = {
  /** 当前环境可用（仅开发环境） */
  enabled: boolean;
  /** AI 接口的基础路径，如 /api/v1/devtools/agent */
  endpoint: string;
  keys: AgentKey[];
};

export type CreatedAgentKey = { key: string; item: AgentKey };

export const listAgentKeys = () =>
  http.request<AgentKeyList>("get", "/devtools/agent-keys");

export const createAgentKey = (data: { name: string; expiresInDays: number }) =>
  http.request<CreatedAgentKey>("post", "/devtools/agent-keys", { data });

export const revokeAgentKey = (id: string) =>
  http.request<null>("delete", `/devtools/agent-keys/${id}`);
