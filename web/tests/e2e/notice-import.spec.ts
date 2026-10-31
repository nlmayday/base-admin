import { expect, test } from "@playwright/test";
import { ApiClient } from "./support/api";
import { uniqueSuffix } from "./support/env";
import { openAsAdmin } from "./support/ui";

type Notice = { id: string; title: string };
type UserPage = { list: { id: string; username: string }[] };

test("发布通知公告后，顶栏铃铛和「我的消息」可以查看", async ({ page }) => {
  const title = `E2E公告${uniqueSuffix()}`;
  const api = await ApiClient.admin();
  try {
    await openAsAdmin(page, "/notice/notice");
    await page.getByRole("button", { name: "新增通知公告" }).click();
    const dialog = page.getByRole("dialog", { name: "新增通知公告" });
    await dialog.getByLabel("标题").fill(title);
    await dialog.getByLabel("内容").fill("第一行\n第二行");
    await dialog.locator('input[type="file"]').setInputFiles({
      name: "制度.pdf",
      mimeType: "application/pdf",
      buffer: Buffer.from("%PDF-1.4\n%%EOF\n")
    });
    await expect(dialog.getByRole("link", { name: "制度.pdf" })).toBeVisible();
    await dialog.getByRole("button", { name: "保存" }).click();

    const row = page.getByRole("row", { name: new RegExp(title) });
    await expect(row.getByText("草稿")).toBeVisible();
    await row.getByRole("button", { name: "发布" }).click();
    await page.getByRole("button", { name: "确定" }).click();
    await expect(row.getByText("已发布")).toBeVisible();

    await page.reload();
    await page.getByTestId("nav-notice").first().click();
    const item = page.getByTestId("nav-notice-item").filter({ hasText: title });
    await item.click();
    await expect(page).toHaveURL(/#\/message\/index/);
    const detail = page.getByRole("dialog", { name: title });
    await expect(detail.getByText("第一行")).toBeVisible();
    await expect(
      detail.getByRole("link", { name: "制度.pdf" })
    ).toHaveAttribute(
      "href",
      /\/api\/v1\/files\/a\/\d{4}\/\d{2}\/\d{2}\/[0-9a-f]{32}\.pdf$/
    );
    await detail.getByRole("button", { name: /Close|关闭/ }).click();

    // 已读后不再出现在未读列表
    await page.getByText("未读", { exact: true }).click();
    await expect(
      page.getByRole("row", { name: new RegExp(title) })
    ).toHaveCount(0);
  } finally {
    const found = await api.call<{ list: Notice[] }>(
      "GET",
      `/notices?title=${encodeURIComponent(title)}`
    );
    for (const n of found.list) {
      await api.call("DELETE", `/notices/${n.id}`).catch(() => undefined);
    }
    await api.dispose();
  }
});

test("导入用户：有错误时逐行提示，修正后整批导入", async ({ page }) => {
  const suffix = uniqueSuffix()
    .toLowerCase()
    .replace(/[^a-z0-9]/g, "");
  const names = [`imp_a_${suffix}`, `imp_b_${suffix}`];
  const header = "用户名,昵称,手机号,邮箱,部门,角色,岗位\n";
  const api = await ApiClient.admin();
  try {
    await openAsAdmin(page, "/system/user");
    await page.getByTestId("user-import").click();
    const dialog = page.getByRole("dialog", { name: "导入用户" });
    const fileInput = dialog.locator('input[type="file"]');
    await dialog.getByPlaceholder(/所有导入用户相同/).fill("Import-Pass-2026");

    await fileInput.setInputFiles({
      name: "bad.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(`${header}${names[0]},,,bad-mail,,,\nx,,,,,,\n`)
    });
    await dialog.getByRole("button", { name: "导入", exact: true }).click();
    await expect(dialog.getByText(/导入数据有误/)).toBeVisible();
    await expect(
      dialog.getByRole("cell", { name: "邮箱格式不正确" })
    ).toBeVisible();

    await fileInput.setInputFiles({
      name: "good.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(`${header}${names[0]},甲,,,,,\n${names[1]},乙,,,,,\n`)
    });
    await dialog.getByRole("button", { name: "导入", exact: true }).click();
    await expect(page.getByText("已导入 2 个用户")).toBeVisible();
    await expect(dialog).toBeHidden();
  } finally {
    for (const name of names) {
      const found = await api.call<UserPage>("GET", `/users?keyword=${name}`);
      for (const u of found.list.filter(x => x.username === name)) {
        await api.call("DELETE", `/users/${u.id}`).catch(() => undefined);
      }
    }
    await api.dispose();
  }
});

test("新增菜单时可以从图标库选择图标", async ({ page }) => {
  await openAsAdmin(page, "/system/menu");
  await page.getByTestId("menu-create").click();
  const dialog = page.getByRole("dialog").filter({ hasText: "图标" });
  await dialog.getByTestId("icon-select").click();
  await page.getByText("业务", { exact: true }).click();
  await page.getByTitle("ri:shopping-cart-line").click();
  await expect(dialog.getByTestId("icon-select")).toHaveValue(
    "ri:shopping-cart-line"
  );
  await dialog.getByRole("button", { name: "取消" }).click();
});
