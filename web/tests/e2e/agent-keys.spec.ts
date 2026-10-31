import { expect, request, test } from "@playwright/test";
import { ApiClient } from "./support/api";
import { baseURL, uniqueSuffix } from "./support/env";
import { openAsAdmin } from "./support/ui";

type AgentKeyList = {
  keys: { id: string; name: string; revokedAt: string | null }[];
};

test("创建 AI 接口密钥：只显示一次、可调用、撤销后失效", async ({ page }) => {
  const name = `E2E密钥${uniqueSuffix()}`;
  const api = await ApiClient.admin();
  const agent = await request.newContext({ baseURL });
  try {
    await openAsAdmin(page, "/devtools/agent");
    await page.getByRole("button", { name: "创建密钥" }).click();
    const form = page.getByRole("dialog", { name: "创建密钥" });
    await form.getByLabel("用途").fill(name);
    await form.getByText("7 天").click();
    await form.getByRole("button", { name: "创建" }).click();

    const done = page.getByRole("dialog", { name: "密钥已创建" });
    const key = await done.locator("input").first().inputValue();
    expect(key).toMatch(/^bak_[\w-]{43}$/);
    await expect(done.locator("textarea")).toHaveValue(
      new RegExp(`Authorization: Bearer ${key}`)
    );
    await done.getByRole("button", { name: "完成" }).click();

    const auth = { Authorization: `Bearer ${key}` };
    // 开发环境 200，其他环境 403（AI 接口关闭）；都说明密钥本身有效
    const before = await agent.get("/api/v1/devtools/agent/tables", {
      headers: auth
    });
    expect([200, 403]).toContain(before.status());

    const row = page.getByRole("row", { name: new RegExp(name) });
    await expect(row.getByText("可用")).toBeVisible();
    await row.getByRole("button", { name: "撤销" }).click();
    await page.getByRole("button", { name: "确定" }).click();
    await expect(row.getByText("已撤销")).toBeVisible();

    const after = await agent.get("/api/v1/devtools/agent/tables", {
      headers: auth
    });
    expect(after.status()).toBe(401);
  } finally {
    const list = await api.call<AgentKeyList>("GET", "/devtools/agent-keys");
    for (const k of list.keys.filter(k => k.name === name && !k.revokedAt)) {
      await api
        .call("DELETE", `/devtools/agent-keys/${k.id}`)
        .catch(() => undefined);
    }
    await agent.dispose();
    await api.dispose();
  }
});
