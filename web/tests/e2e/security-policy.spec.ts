import { expect, test } from "@playwright/test";
import { openAsAdmin } from "./support/ui";

test("登录限制：保存会把自己锁在外面的白名单时被拒绝", async ({ page }) => {
  await openAsAdmin(page, "/security/policy");
  const allowlist = page.getByTestId("policy-ip-allowlist");
  await expect(page.getByText(/你当前的 IP：\S+/)).toBeVisible();
  await expect(allowlist).toHaveValue("");

  // 一个不包含本机地址的网段；没有豁免角色时保存会锁住当前管理员
  await allowlist.fill("192.0.2.0/24");
  await page.getByRole("button", { name: "保存策略" }).click();
  await expect(page.getByText("保存后你将无法登录")).toBeVisible();

  await page.reload();
  await expect(allowlist).toHaveValue("", { timeout: 10_000 });
});
