import { expect, type Page } from "@playwright/test";
import type { Credentials } from "./env";

/** 路由使用 hash 模式 */
export const routeUrl = (path: string): string => `/#${path}`;

/** 以指定账号重新登录；会先清掉当前上下文中的会话（默认是管理员会话） */
export async function loginViaUi(
  page: Page,
  creds: Credentials
): Promise<void> {
  await page.context().clearCookies();
  await page.goto(routeUrl("/login"));
  await page.getByPlaceholder("账号").fill(creds.username);
  await page.getByPlaceholder("密码").fill(creds.password);
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page).not.toHaveURL(/#\/login/);
}

export async function logoutViaUi(page: Page): Promise<void> {
  await page.getByTestId("nav-user").first().click();
  await page.getByTestId("nav-logout").first().click();
  await expect(page).toHaveURL(/#\/login/);
}

/** 以全局初始化保存的管理员会话打开页面 */
export async function openAsAdmin(page: Page, path: string): Promise<void> {
  await page.goto(routeUrl(path));
  // 等动态路由加载完成（侧边栏出现）后再继续，避免后续跳转被首屏导航覆盖
  await expect(page.locator(".sidebar-container")).toBeVisible();
  await expect(page).not.toHaveURL(/#\/login/);
}
