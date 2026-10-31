import type { AgentKey } from "@/api/devtools/agent";

/** 密钥状态：可用 / 已过期 / 已撤销 */
export type KeyState = "active" | "expired" | "revoked";

export function keyState(key: AgentKey, now = new Date()): KeyState {
  if (key.revokedAt) return "revoked";
  if (key.expiresAt && new Date(key.expiresAt) <= now) return "expired";
  return "active";
}

/** 接口的完整地址：页面所在站点 + 基础路径 */
export function agentBaseUrl(origin: string, endpoint: string): string {
  return origin.replace(/\/+$/, "") + endpoint;
}

/** 复制给 AI 的接入说明；key 为空时用占位符 */
export function agentPrompt(baseUrl: string, key = ""): string {
  const k = key || "<密钥>";
  return [
    "本项目提供了 AI 接口，用来按框架约定建业务表、生成增删改查模块（后端接口、前端页面、菜单、权限）。",
    "需要新建表或基础 CRUD 功能时，必须调用这个接口生成，不要手写建表 SQL 和 CRUD 代码；生成后再按需求修改业务逻辑。",
    "",
    `- 基础地址：${baseUrl}`,
    `- 鉴权：每个请求带请求头 Authorization: Bearer ${k}`,
    "",
    "开始前先阅读使用说明（Markdown），并严格按说明的流程和规则调用：",
    `curl -s -H "Authorization: Bearer ${k}" ${baseUrl}/guide`
  ].join("\n");
}
