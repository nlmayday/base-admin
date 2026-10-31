import { expect, test } from "@playwright/test";
import { ApiClient, type AsyncRoute } from "./support/api";
import { openAsAdmin } from "./support/ui";

const shown = (routes: AsyncRoute[] = []): AsyncRoute[] =>
  routes.filter(r => r.meta?.showLink !== false);

test("侧边栏菜单来自后端 /me/menus", async ({ page }) => {
  const api = await ApiClient.admin();
  const menus = shown(await api.menus());
  await api.dispose();
  expect(menus.length).toBeGreaterThan(0);

  await openAsAdmin(page, "/welcome");
  const sidebar = page.locator(".sidebar-container");
  const visible = (title: string) =>
    expect(sidebar.getByText(title, { exact: true }).first()).toBeVisible();

  // 侧边栏为手风琴模式：逐个展开目录并检查其子菜单
  for (const top of menus) {
    const children = shown(top.children);
    // 目录始终显示（即使只有一个子页面，后端设置了 showParent）
    await visible(top.meta.title);
    if (children.length === 0) continue;
    await sidebar
      .locator(".el-sub-menu__title", { hasText: top.meta.title })
      .first()
      .click();
    for (const child of children) await visible(child.meta.title);
  }
});
