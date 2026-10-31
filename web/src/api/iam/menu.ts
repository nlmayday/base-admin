import { http } from "@/utils/http";

export type MenuType = "dir" | "menu" | "link";

export type MenuNode = {
  id: string;
  code: string;
  parentId: string | null;
  type: MenuType;
  title: string;
  path: string;
  component: string;
  icon: string;
  sort: number;
  hidden: boolean;
  keepAlive: boolean;
  permissionCode: string;
  externalUrl: string;
  source: "module" | "custom";
  version: number;
  /** 挂在该菜单下的按钮权限（只读，由模块声明） */
  permissions: MenuPermission[];
  children: MenuNode[];
};

export type MenuPermission = { code: string; name: string };

/** 只能新建自定义的目录或外链 */
export type MenuCreatePayload = {
  parentId: string | null;
  type: "dir" | "link";
  title: string;
  path: string;
  icon: string;
  sort: number;
  hidden: boolean;
  externalUrl: string;
};

/** 模块菜单只能改 title/icon/sort/hidden */
export type ModuleMenuUpdatePayload = {
  title: string;
  icon: string;
  sort: number;
  hidden: boolean;
  version: number;
};

export type CustomMenuUpdatePayload = ModuleMenuUpdatePayload & {
  parentId: string | null;
  path: string;
  externalUrl: string;
};

const enc = encodeURIComponent;

export const getMenuTree = () => http.get<MenuNode[]>("/menus/tree");

export const createMenu = (data: MenuCreatePayload) =>
  http.post<MenuNode>("/menus", data);

export const updateMenu = (
  id: string,
  data: ModuleMenuUpdatePayload | CustomMenuUpdatePayload
) => http.put<MenuNode>(`/menus/${enc(id)}`, data);

export const deleteMenu = (id: string) =>
  http.delete<null>(`/menus/${enc(id)}`);
