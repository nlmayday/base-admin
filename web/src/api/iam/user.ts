import { http } from "@/utils/http";
import {
  compactParams,
  type Page,
  type PageQuery,
  type Status
} from "@/api/types";

export type User = {
  id: string;
  username: string;
  nickname: string;
  email: string;
  phone: string;
  avatarUrl: string;
  deptId: string | null;
  deptName: string;
  status: Status;
  roleIds: string[];
  positionIds: string[];
  lastLoginAt: string | null;
  createdAt: string;
  updatedAt: string;
  version: number;
};

export type UserFilter = {
  keyword?: string;
  deptId?: string | null;
  status?: Status | null;
};

export type UserCreatePayload = {
  username: string;
  password: string;
  nickname: string;
  email: string;
  phone: string;
  deptId: string | null;
  status: Status;
  roleIds: string[];
  positionIds: string[];
};

export type UserUpdatePayload = {
  nickname: string;
  email: string;
  phone: string;
  deptId: string | null;
  roleIds: string[];
  positionIds: string[];
  version: number;
};

export type UserOption = { id: string; username: string; nickname: string };

const enc = encodeURIComponent;

export const listUsers = (query: PageQuery & UserFilter) =>
  http.get<Page<User>>("/users", compactParams(query));

export const getUser = (id: string) => http.get<User>(`/users/${enc(id)}`);

export const createUser = (data: UserCreatePayload) =>
  http.post<User>("/users", data);

export const updateUser = (id: string, data: UserUpdatePayload) =>
  http.put<User>(`/users/${enc(id)}`, data);

export const updateUserStatus = (id: string, status: Status, version: number) =>
  http.put<User>(`/users/${enc(id)}/status`, { status, version });

export const resetUserPassword = (id: string, password: string) =>
  http.put<null>(`/users/${enc(id)}/password`, { password });

export const deleteUser = (id: string) =>
  http.delete<null>(`/users/${enc(id)}`);

export const getUserOptions = (keyword?: string) =>
  http.get<UserOption[]>("/users/options", compactParams({ keyword }));

export const exportUsers = (filter: UserFilter) =>
  http.download("/users/export", compactParams(filter), "users.csv");

export const downloadImportTemplate = () =>
  http.download("/users/import-template", {}, "users-import-template.csv");

/** 导入失败时 ApiError.data.rows 给出逐行错误 */
export type ImportRowError = { row: number; message: string };

/** 导入用户：CSV（UTF-8 或 GBK）+ 统一初始密码；任一行有误则整批不导入 */
export function importUsers(file: File, password: string) {
  const form = new FormData();
  form.append("password", password);
  form.append("file", file, file.name);
  return http.request<{ created: number }>(
    "post",
    "/users/import",
    { data: form },
    { timeout: 0, skipErrorHandler: true }
  );
}
