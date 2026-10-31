import { expect, test } from "@playwright/test";
import {
  ApiClient,
  createTempUser,
  removeTempUser,
  type TempUser
} from "./support/api";
import { uniqueSuffix } from "./support/env";
import { loginViaUi, openAsAdmin, routeUrl } from "./support/ui";

test.describe("按钮与页面权限", () => {
  let admin: ApiClient;
  let viewer: TempUser | undefined;

  test.beforeAll(async () => {
    admin = await ApiClient.admin();
    // 只有查看用户的权限，没有 create / update / delete / export
    viewer = await createTempUser(admin, uniqueSuffix(), ["iam:user:view"]);
  });

  test.afterAll(async () => {
    await removeTempUser(admin, viewer);
    await admin.dispose();
  });

  test("没有 iam:user:create 时隐藏「新增用户」按钮", async ({ page }) => {
    await loginViaUi(page, viewer!);
    await page.goto(routeUrl("/system/user"));
    await expect(page.getByPlaceholder("用户名/昵称/手机/邮箱")).toBeVisible();
    await expect(page.getByTestId("user-create")).toBeHidden();
    await expect(page.getByTestId("user-export")).toBeHidden();
  });

  test("管理员可以看到「新增用户」按钮", async ({ page }) => {
    await openAsAdmin(page, "/system/user");
    await expect(page.getByTestId("user-create")).toBeVisible();
  });

  test("未授权的页面没有注册路由，直接访问进入 404", async ({ page }) => {
    await loginViaUi(page, viewer!);
    await page.goto(routeUrl("/system/role"));
    await expect(page.getByText("404", { exact: true }).first()).toBeVisible();
  });
});
