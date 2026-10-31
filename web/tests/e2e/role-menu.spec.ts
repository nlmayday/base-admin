import { expect, test } from "@playwright/test";
import { ApiClient } from "./support/api";
import { uniqueSuffix } from "./support/env";
import { openAsAdmin } from "./support/ui";

type Created = { id: string };

test.describe("角色授权抽屉与菜单管理", () => {
  let admin: ApiClient;
  let role: Created | undefined;
  const suffix = uniqueSuffix();

  test.beforeAll(async () => {
    admin = await ApiClient.admin();
    role = await admin.call<Created>("POST", "/roles", {
      code: `e2e_drawer_${suffix}`.slice(0, 32),
      name: `E2E 抽屉 ${suffix}`
    });
  });

  test.afterAll(async () => {
    if (role) await admin.call("DELETE", `/roles/${role.id}`);
    await admin.dispose();
  });

  test("权限按声明顺序展示，支持菜单级全选与半选", async ({ page }) => {
    await openAsAdmin(page, "/system/role");
    const row = page.getByRole("row", {
      name: new RegExp(`E2E 抽屉 ${suffix}`)
    });
    await row.getByRole("button", { name: "分配权限" }).click();

    const userItems = page.locator('[data-perm-code^="iam:user:"]');
    await expect(userItems.first()).toHaveAttribute(
      "data-perm-code",
      "iam:user:view"
    );

    const userGroup = page.locator('[data-perm-group="iam.user"]');
    await userGroup.click();
    await expect(
      page.locator('[data-perm-code^="iam:user:"] input:checked')
    ).toHaveCount(await userItems.count());
    const dir = page.locator('[data-perm-group="iam.system"]');
    await expect(dir.locator(".el-checkbox__input")).toHaveClass(
      /is-indeterminate/
    );

    // 取消「查看用户」会级联取消依赖它的全部用户权限
    await page.locator('[data-perm-code="iam:user:view"]').click();
    await expect(
      page.locator('[data-perm-code^="iam:user:"] input:checked')
    ).toHaveCount(0);
  });

  test("新增外链后侧边栏立即出现，删除后立即消失", async ({ page }) => {
    const title = `E2E 外链 ${suffix}`;
    await openAsAdmin(page, "/system/menu");
    await page.getByTestId("menu-create").click();
    await page.getByRole("radio", { name: "外链" }).check({ force: true });
    await expect(page.getByText("请输入标题")).toBeHidden();
    await page.getByLabel("标题").fill(title);
    await page.getByLabel("外链地址").fill("https://example.com");
    await page.getByRole("button", { name: "保存" }).click();

    const sidebar = page.locator(".sidebar-container");
    await expect(sidebar.getByText(title)).toBeVisible();

    const menuRow = page.getByRole("row", { name: new RegExp(title) });
    await menuRow.getByRole("button", { name: "删除" }).click();
    await page.getByRole("button", { name: "确定" }).click();
    await expect(sidebar.getByText(title)).toBeHidden();
  });
});
