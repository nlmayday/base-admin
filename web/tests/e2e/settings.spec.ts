import { expect, test } from "@playwright/test";
import { openAsAdmin } from "./support/ui";

async function choosePageSize(
  page: import("@playwright/test").Page,
  size: number
) {
  await openAsAdmin(page, "/sysconfig/settings");
  const item = page.locator(".el-form-item", { hasText: "列表默认每页条数" });
  await item.locator(".el-select").click();
  await page
    .locator(".el-select-dropdown__item:visible", { hasText: `${size} 条/页` })
    .click();
  await page.getByTestId("settings-save").click();
  await expect(page.getByText("设置已保存")).toBeVisible();
}

test("基础设置：列表默认每页条数对之后打开的列表生效", async ({ page }) => {
  await choosePageSize(page, 50);
  try {
    // 新打开（刷新）的列表页使用新的默认值
    await openAsAdmin(page, "/audit/login-log");
    await expect(page.locator(".el-pagination .el-select")).toContainText(
      "50条/页"
    );
  } finally {
    await choosePageSize(page, 20);
  }
  await openAsAdmin(page, "/audit/login-log");
  await expect(page.locator(".el-pagination .el-select")).toContainText(
    "20条/页"
  );
});
