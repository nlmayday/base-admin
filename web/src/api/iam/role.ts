import { http } from "@/utils/http";
import {
  compactParams,
  type CodeOption,
  type Page,
  type PageQuery,
  type Status
} from "@/api/types";

export type DataScope =
  "all" | "dept_and_children" | "dept" | "self" | "custom";

export const DATA_SCOPE_OPTIONS: ReadonlyArray<{
  label: string;
  value: DataScope;
}> = [
  { label: "全部数据", value: "all" },
  { label: "本部门及以下", value: "dept_and_children" },
  { label: "仅本部门", value: "dept" },
  { label: "仅本人", value: "self" },
  { label: "自定义部门", value: "custom" }
];

export type Role = {
  id: string;
  code: string;
  name: string;
  dataScope: DataScope;
  deptIds: string[];
  status: Status;
  builtin: boolean;
  sort: number;
  remark: string;
  createdAt: string;
  version: number;
};

export type RoleCreatePayload = {
  code: string;
  name: string;
  dataScope: DataScope;
  deptIds: string[];
  status: Status;
  sort: number;
  remark: string;
};

export type RoleUpdatePayload = Omit<RoleCreatePayload, "code"> & {
  version: number;
};

export type PermissionDef = { code: string; name: string; requires: string[] };

export type PermNode = {
  menuCode: string;
  title: string;
  type: "dir" | "menu" | "link";
  permissions: PermissionDef[];
  children: PermNode[];
};

const enc = encodeURIComponent;

export const listRoles = (query: PageQuery & { keyword?: string }) =>
  http.get<Page<Role>>("/roles", compactParams(query));

export const getRoleOptions = () => http.get<CodeOption[]>("/roles/options");

export const createRole = (data: RoleCreatePayload) =>
  http.post<Role>("/roles", data);

export const updateRole = (id: string, data: RoleUpdatePayload) =>
  http.put<Role>(`/roles/${enc(id)}`, data);

export const deleteRole = (id: string) =>
  http.delete<null>(`/roles/${enc(id)}`);

export const getRolePermissions = (id: string) =>
  http.get<{ codes: string[] }>(`/roles/${enc(id)}/permissions`);

export const setRolePermissions = (id: string, codes: string[]) =>
  http.put<{ codes: string[] }>(`/roles/${enc(id)}/permissions`, { codes });

export const getPermissionTree = () =>
  http.get<PermNode[]>("/permissions/tree");
