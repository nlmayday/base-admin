import { http, type PureHttpRequestConfig } from "@/utils/http";
import type { Status } from "@/api/types";

export type Dept = {
  id: string;
  parentId: string | null;
  name: string;
  leaderUserId: string | null;
  sort: number;
  status: Status;
  version: number;
  children: Dept[];
};

export type DeptPayload = {
  parentId: string | null;
  name: string;
  leaderUserId: string | null;
  sort: number;
  status: Status;
};

const enc = encodeURIComponent;

export const getDeptTree = (config?: PureHttpRequestConfig) =>
  http.get<Dept[]>("/departments/tree", undefined, config);

export const createDept = (data: DeptPayload) =>
  http.post<Dept>("/departments", data);

export const updateDept = (
  id: string,
  data: DeptPayload & { version: number }
) => http.put<Dept>(`/departments/${enc(id)}`, data);

export const deleteDept = (id: string) =>
  http.delete<null>(`/departments/${enc(id)}`);
