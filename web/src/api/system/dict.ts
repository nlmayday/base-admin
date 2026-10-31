import { http } from "@/utils/http";
import {
  compactParams,
  type Page,
  type PageQuery,
  type Status
} from "@/api/types";

/** el-tag 的 type；"" 表示默认样式 */
export type DictTagType =
  "" | "primary" | "success" | "info" | "warning" | "danger";

export const DICT_TAG_TYPES: ReadonlyArray<{
  label: string;
  value: DictTagType;
}> = [
  { label: "默认", value: "" },
  { label: "主要", value: "primary" },
  { label: "成功", value: "success" },
  { label: "信息", value: "info" },
  { label: "警告", value: "warning" },
  { label: "危险", value: "danger" }
];

export type DictType = {
  id: string;
  code: string;
  name: string;
  status: Status;
  remark: string;
  itemCount: number;
  createdAt: string;
  updatedAt: string;
  version: number;
};

export type DictTypeCreatePayload = {
  code: string;
  name: string;
  status: Status;
  remark: string;
};

export type DictTypeUpdatePayload = Omit<DictTypeCreatePayload, "code"> & {
  version: number;
};

export type DictItem = {
  id: string;
  typeId: string;
  label: string;
  value: string;
  sort: number;
  status: Status;
  tagType: DictTagType;
  remark: string;
  createdAt: string;
  version: number;
};

export type DictItemPayload = {
  label: string;
  value: string;
  sort: number;
  status: Status;
  tagType: DictTagType;
  remark: string;
};

/** GET /dicts/:code/items 返回的启用项 */
export type DictOption = { label: string; value: string; tagType: DictTagType };

const enc = encodeURIComponent;

export const listDictTypes = (
  query: PageQuery & { keyword?: string; status?: Status | null }
) => http.get<Page<DictType>>("/dict-types", compactParams(query));

export const createDictType = (data: DictTypeCreatePayload) =>
  http.post<DictType>("/dict-types", data);

export const updateDictType = (id: string, data: DictTypeUpdatePayload) =>
  http.put<DictType>(`/dict-types/${enc(id)}`, data);

export const deleteDictType = (id: string) =>
  http.delete<null>(`/dict-types/${enc(id)}`);

export const listDictItems = (
  typeId: string,
  query: { keyword?: string; status?: Status | null } = {}
) =>
  http.get<DictItem[]>(
    `/dict-types/${enc(typeId)}/items`,
    compactParams(query)
  );

export const createDictItem = (typeId: string, data: DictItemPayload) =>
  http.post<DictItem>(`/dict-types/${enc(typeId)}/items`, data);

export const updateDictItem = (
  id: string,
  data: DictItemPayload & { version: number }
) => http.put<DictItem>(`/dict-items/${enc(id)}`, data);

export const deleteDictItem = (id: string) =>
  http.delete<null>(`/dict-items/${enc(id)}`);

export const getDictOptions = (code: string) =>
  http.get<DictOption[]>(`/dicts/${enc(code)}/items`);
