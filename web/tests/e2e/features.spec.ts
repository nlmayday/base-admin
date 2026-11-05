import { expect, test, type Page } from "@playwright/test";
import {
  ApiClient,
  createTempUser,
  removeTempUser,
  type AsyncRoute,
  type TempUser
} from "./support/api";
import { uniqueSuffix } from "./support/env";
import { openAsAdmin, routeUrl } from "./support/ui";

/** 菜单树中所有站内页面的路径（不含目录与外链） */
function pagePaths(routes: AsyncRoute[]): string[] {
  return routes.flatMap(r =>
    r.children?.length
      ? pagePaths(r.children)
      : r.path.startsWith("http")
        ? []
        : [r.path]
  );
}

/** 收集页面运行时错误与 Vue 的「组件未注册」警告 */
function watchProblems(page: Page): string[] {
  const problems: string[] = [];
  page.on("pageerror", err => problems.push(`pageerror: ${err.message}`));
  page.on("console", msg => {
    const text = msg.text();
    if (msg.type() === "error" || /Failed to resolve component/.test(text)) {
      // 接口业务错误由页面提示处理，不算页面缺陷
      if (!/status code 4\d\d/.test(text)) problems.push(`console: ${text}`);
    }
  });
  return problems;
}

test("每个菜单页面都能正常渲染", async ({ page }) => {
  const api = await ApiClient.admin();
  const paths = pagePaths(await api.menus());
  await api.dispose();
  expect(paths.length).toBeGreaterThan(15);

  const problems = watchProblems(page);
  await openAsAdmin(page, "/welcome");
  for (const path of paths) {
    await page.goto(routeUrl(path));
    await expect(page.locator(".main").first(), path).toBeVisible();
    await expect(page.getByText("404", { exact: true })).toHaveCount(0);
  }
  expect(problems).toEqual([]);
});

test("工作台展示统计卡片与快捷入口", async ({ page }) => {
  await openAsAdmin(page, "/welcome");
  // 统计卡片是可点击的链接，标题与侧边栏菜单同名，按角色定位
  await expect(page.getByRole("link", { name: /^用户总数/ })).toBeVisible();
  await expect(page.getByRole("link", { name: /^在线会话/ })).toBeVisible();
  await expect(page.getByText("快捷入口")).toBeVisible();
  await expect(page.getByText("系统状态")).toBeVisible();
  // 快捷入口按菜单顺序取当前用户可见的页面，业务菜单多时不一定有「用户管理」：
  // 点第一个入口，应打开同名页面
  const first = page.getByTestId("quick-link").first();
  await expect(first).toBeVisible();
  const title = (
    await first.getByTestId("quick-link-title").innerText()
  ).trim();
  await first.click();
  await expect(page).not.toHaveURL(/#\/welcome/);
  await expect(
    page.locator(".el-breadcrumb__item:last-child .el-breadcrumb__inner")
  ).toHaveText(title);
});

test("在界面上维护字典项，业务接口按字典返回标签样式", async ({ page }) => {
  const suffix = uniqueSuffix();
  const code = `e2e_${suffix}`.toLowerCase().replace(/[^a-z0-9_]/g, "_");
  const typeName = `E2E字典${suffix}`;
  const api = await ApiClient.admin();
  const type = await api.call<{ id: string }>("POST", "/dict-types", {
    code,
    name: typeName
  });
  try {
    await openAsAdmin(page, "/sysconfig/dict");
    // 左侧类型列表中的条目（右侧标题也会显示同名）
    await page.getByText(typeName, { exact: true }).first().click();
    await page.getByTestId("dict-item-create").click();
    const dialog = page.getByRole("dialog", { name: "新增字典项" });
    await dialog.getByPlaceholder("显示文字").fill("已完成");
    await dialog.getByPlaceholder("存储的值").fill("done");
    await dialog.getByText("成功", { exact: true }).click();
    await dialog.getByRole("button", { name: "保存" }).click();

    const row = page.getByRole("row", { name: /已完成/ });
    await expect(row.locator(".el-tag--success").first()).toHaveText("已完成");

    const options = await api.call<{ value: string; tagType: string }[]>(
      "GET",
      `/dicts/${code}/items`
    );
    expect(options).toEqual([
      expect.objectContaining({ value: "done", tagType: "success" })
    ]);
  } finally {
    await api.call("DELETE", `/dict-types/${type.id}`).catch(() => undefined);
    await api.dispose();
  }
});

test("连续登录失败后要求图形验证码", async ({ page }) => {
  const admin = await ApiClient.admin();
  let user: TempUser | undefined;
  try {
    user = await createTempUser(admin, uniqueSuffix(), []);
    await page.context().clearCookies();
    await page.goto(routeUrl("/login"));
    await page.getByPlaceholder("账号").fill(user.username);
    await page.getByPlaceholder("密码").fill("Wrong-Password-1");
    const submit = page.getByRole("button", { name: "登录" });
    // 默认策略：失败 3 次后需要验证码
    for (let i = 0; i < 3; i++) {
      await submit.click();
      await expect(page.getByText("账号或密码错误").first()).toBeVisible();
    }
    await expect(page.getByPlaceholder("验证码")).toBeVisible();
    await expect(page.locator(".captcha-img")).toBeVisible();
  } finally {
    if (user) {
      await admin
        .call("POST", "/security/unlock", { username: user.username })
        .catch(() => undefined);
    }
    await removeTempUser(admin, user);
    await admin.dispose();
  }
});
