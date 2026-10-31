import { expect, test } from "@playwright/test";
import {
  ApiClient,
  createTempUser,
  removeTempUser,
  type TempUser
} from "./support/api";
import { uniqueSuffix } from "./support/env";
import { loginViaUi, logoutViaUi, routeUrl } from "./support/ui";

test.describe("认证", () => {
  // 这些用例从未登录状态开始，登录/退出只使用临时用户，不影响共享的管理员会话
  test.use({ storageState: { cookies: [], origins: [] } });

  test("未登录访问页面会跳转到登录页并带上 redirect", async ({ page }) => {
    await page.goto(routeUrl("/system/user"));
    await expect(page).toHaveURL(/#\/login\?redirect=/);
  });

  test("错误的密码在登录表单上提示", async ({ page }) => {
    await page.goto(routeUrl("/login"));
    // 使用不存在的账号，避免累计管理员账号的失败次数（会触发验证码）
    await page.getByPlaceholder("账号").fill(`e2e_nobody_${uniqueSuffix()}`);
    await page.getByPlaceholder("密码").fill("definitely-wrong-password");
    await page.getByRole("button", { name: "登录" }).click();
    await expect(page.locator(".el-form-item__error")).toBeVisible();
    await expect(page).toHaveURL(/#\/login/);
  });

  test.describe("临时用户", () => {
    let admin: ApiClient;
    let user: TempUser | undefined;

    test.beforeAll(async () => {
      admin = await ApiClient.admin();
      user = await createTempUser(admin, uniqueSuffix(), []);
    });

    test.afterAll(async () => {
      await removeTempUser(admin, user);
      await admin.dispose();
    });

    test("登录后进入系统，刷新页面仍保持登录", async ({ page }) => {
      await loginViaUi(page, user!);
      await expect(page.getByTestId("nav-user").first()).toBeVisible();
      await page.reload();
      await expect(page).not.toHaveURL(/#\/login/);
      await expect(page.getByTestId("nav-user").first()).toBeVisible();
    });

    test("退出登录后回到登录页，且会话失效", async ({ page }) => {
      await loginViaUi(page, user!);
      await logoutViaUi(page);
      await page.goto(routeUrl("/welcome"));
      await expect(page).toHaveURL(/#\/login\?redirect=/);
    });
  });

  test.describe("会话被撤销", () => {
    let admin: ApiClient;
    let user: TempUser | undefined;

    test.beforeAll(async () => {
      admin = await ApiClient.admin();
      user = await createTempUser(admin, uniqueSuffix(), ["iam:user:view"]);
    });

    test.afterAll(async () => {
      await removeTempUser(admin, user);
      await admin.dispose();
    });

    test("管理员重置密码后，下一次请求跳转到登录页", async ({ page }) => {
      await loginViaUi(page, user!);
      await page.goto(routeUrl("/system/user"));
      // 等待用户页自身的元素，避免命中 keep-alive 中其他页面的按钮
      await expect(
        page.getByPlaceholder("用户名/昵称/手机/邮箱")
      ).toBeVisible();

      // 重置密码会结束该用户的全部会话
      await admin.call("PUT", `/users/${user!.id}/password`, {
        password: `${user!.password}-reset`
      });

      await page.getByRole("button", { name: "搜索" }).click();
      await expect(page).toHaveURL(/#\/login\?redirect=/);
    });
  });
});
