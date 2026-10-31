import { describe, expect, it, vi } from "vitest";
import type { AsyncRoute } from "@/api/auth";
import {
  addAsyncRoutes,
  filterNoPermissionTree,
  filterTree,
  findRouteByPath,
  getParentPaths
} from "./utils";

// The conversion only needs the page index (import.meta.glob); keep the
// router instance, stores and API out of the test.
vi.mock("./index", () => ({ router: {}, resetRouter: vi.fn() }));
vi.mock("@/api/auth", () => ({ getMyMenus: vi.fn() }));
vi.mock("@/store/modules/multiTags", () => ({
  useMultiTagsStoreHook: vi.fn()
}));
vi.mock("@/store/modules/permission", () => ({
  usePermissionStoreHook: vi.fn()
}));

const menus: AsyncRoute[] = [
  {
    path: "/system",
    meta: { title: "系统管理", icon: "ri:settings-3-line", rank: 90 },
    children: [
      {
        path: "/system/user",
        name: "SystemUser",
        component: "system/user/index",
        meta: { title: "用户管理" }
      },
      {
        path: "/system/missing",
        name: "SystemMissing",
        component: "system/missing/index",
        meta: { title: "不存在的页面" }
      }
    ]
  },
  {
    path: "https://docs.example.com/guide",
    name: "Docs",
    meta: { title: "文档" }
  }
];

describe("addAsyncRoutes", () => {
  it("turns /me/menus into router records without changing the input", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const input = structuredClone(menus);
    const routes = addAsyncRoutes(input);
    expect(input).toEqual(menus);

    const dir = routes[0];
    expect(dir.path).toBe("/system");
    expect(dir.redirect).toBe("/system/user");
    expect(dir.name).toBe("SystemUserParent");
    expect(dir.meta).toMatchObject({ title: "系统管理", backstage: true });
    expect(dir.children?.map(c => c.name)).toEqual([
      "SystemUser",
      "SystemMissing"
    ]);
    expect(typeof dir.children?.[0].component).toBe("function");

    // An unknown view falls back to the 404 page and is reported.
    expect(typeof dir.children?.[1].component).toBe("function");
    expect(warn).toHaveBeenCalledTimes(1);
    expect(String(warn.mock.calls[0][0])).toContain("system/missing/index");
  });

  it("maps external links to an in-site placeholder named by the URL", () => {
    const [, link] = addAsyncRoutes(menus);
    expect(link.path).toBe("/external/docs");
    expect(link.name).toBe("https://docs.example.com/guide");
    expect(link.meta).toMatchObject({ title: "文档", backstage: true });
  });

  it("returns nothing for no menus", () => {
    expect(addAsyncRoutes([])).toEqual([]);
  });
});

describe("route tree helpers", () => {
  const routes = [
    {
      path: "/a",
      meta: { showLink: true },
      children: [
        { path: "/a/b", name: "B", meta: {} },
        { path: "/a/hidden", meta: { showLink: false } }
      ]
    },
    { path: "/empty", meta: {}, children: [] }
  ];

  it("finds parent paths and routes by path", () => {
    expect(getParentPaths("/a/b", routes as never)).toEqual(["/a"]);
    expect(getParentPaths("B", routes as never, "name")).toEqual(["/a"]);
    expect(getParentPaths("/nope", routes as never)).toEqual([]);
    expect(findRouteByPath("/a/b", routes as never)).toMatchObject({
      name: "B"
    });
    expect(findRouteByPath("/nope", routes as never)).toBeNull();
  });

  it("hides showLink=false menus and empty directories", () => {
    expect(
      filterTree(routes as never)[0].children.map(
        (c: { path: string }) => c.path
      )
    ).toEqual(["/a/b"]);
    expect(
      filterNoPermissionTree(routes as never).map(
        (r: { path: string }) => r.path
      )
    ).toEqual(["/a"]);
  });
});
