import { http } from "@/utils/http";
import { compactParams, type Page, type PageQuery } from "@/api/types";

export type ConfigValueType = "string" | "number" | "boolean" | "json";

export const CONFIG_VALUE_TYPES: ReadonlyArray<{
  label: string;
  value: ConfigValueType;
}> = [
  { label: "字符串", value: "string" },
  { label: "数字", value: "number" },
  { label: "布尔", value: "boolean" },
  { label: "JSON", value: "json" }
];

export type Config = {
  id: string;
  key: string;
  value: string;
  valueType: ConfigValueType;
  groupName: string;
  name: string;
  description: string;
  /** false：只读参数 */
  editable: boolean;
  /** 内置参数，只能在「基础设置」中修改 */
  builtin: boolean;
  createdAt: string;
  updatedAt: string;
  version: number;
};

export type ConfigCreatePayload = {
  key: string;
  value: string;
  valueType: ConfigValueType;
  groupName: string;
  name: string;
  description: string;
};

export type ConfigUpdatePayload = Omit<ConfigCreatePayload, "key"> & {
  version: number;
};

export type ConfigChange = {
  id: string;
  /** 创建时为 null */
  oldValue: string | null;
  newValue: string;
  changedBy: string | null;
  changedByName: string;
  changedAt: string;
};

const enc = encodeURIComponent;

export type ConfigFilter = {
  keyword?: string;
  group?: string;
  /** 默认不列出内置参数（它们只在「基础设置」中修改） */
  includeBuiltin?: boolean;
};

export const listConfigs = (query: PageQuery & ConfigFilter) =>
  http.get<Page<Config>>("/configs", compactParams(query));

export const getConfigGroups = (includeBuiltin = false) =>
  http.get<string[]>("/configs/groups", { includeBuiltin });

export const createConfig = (data: ConfigCreatePayload) =>
  http.post<Config>("/configs", data);

export const updateConfig = (id: string, data: ConfigUpdatePayload) =>
  http.put<Config>(`/configs/${enc(id)}`, data);

export const deleteConfig = (id: string) =>
  http.delete<null>(`/configs/${enc(id)}`);

export const listConfigChanges = (id: string, query: PageQuery) =>
  http.get<Page<ConfigChange>>(`/configs/${enc(id)}/changes`, query);
