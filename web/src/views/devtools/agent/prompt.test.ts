import { describe, expect, it } from "vitest";
import type { AgentKey } from "@/api/devtools/agent";
import { agentBaseUrl, agentPrompt, keyState } from "./prompt";

const key = (over: Partial<AgentKey> = {}): AgentKey => ({
  id: "1",
  name: "k",
  prefix: "bak_abcdefgh",
  userId: "1",
  userName: "admin",
  expiresAt: null,
  lastUsedAt: null,
  revokedAt: null,
  createdAt: "2026-10-01T00:00:00Z",
  ...over
});

describe("keyState", () => {
  const now = new Date("2026-10-30T00:00:00Z");
  it("没有过期时间且未撤销为可用", () => {
    expect(keyState(key(), now)).toBe("active");
  });
  it("过期时间已到为已过期", () => {
    expect(keyState(key({ expiresAt: "2026-10-29T00:00:00Z" }), now)).toBe(
      "expired"
    );
    expect(keyState(key({ expiresAt: "2026-11-29T00:00:00Z" }), now)).toBe(
      "active"
    );
  });
  it("撤销优先于过期", () => {
    expect(
      keyState(
        key({
          expiresAt: "2026-10-29T00:00:00Z",
          revokedAt: "2026-10-20T00:00:00Z"
        }),
        now
      )
    ).toBe("revoked");
  });
});

describe("agentPrompt", () => {
  it("拼出完整地址", () => {
    expect(
      agentBaseUrl("http://localhost:8848/", "/api/v1/devtools/agent")
    ).toBe("http://localhost:8848/api/v1/devtools/agent");
  });
  it("包含地址、密钥和读取说明的命令", () => {
    const text = agentPrompt("http://h/api/v1/devtools/agent", "bak_secret");
    expect(text).toContain("基础地址：http://h/api/v1/devtools/agent");
    expect(text).toContain("Authorization: Bearer bak_secret");
    expect(text).toContain(
      'curl -s -H "Authorization: Bearer bak_secret" http://h/api/v1/devtools/agent/guide'
    );
  });
  it("没有密钥时用占位符", () => {
    expect(agentPrompt("http://h/x")).toContain("Bearer <密钥>");
  });
});
