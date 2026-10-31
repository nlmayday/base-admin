import { http } from "@/utils/http";
import {
  compactParams,
  type CodeOption,
  type Page,
  type PageQuery,
  type Status
} from "@/api/types";

export type Position = {
  id: string;
  code: string;
  name: string;
  sort: number;
  status: Status;
  remark: string;
  version: number;
};

export type PositionCreatePayload = {
  code: string;
  name: string;
  sort: number;
  status: Status;
  remark: string;
};

export type PositionUpdatePayload = Omit<PositionCreatePayload, "code"> & {
  version: number;
};

const enc = encodeURIComponent;

export const listPositions = (query: PageQuery & { keyword?: string }) =>
  http.get<Page<Position>>("/positions", compactParams(query));

export const getPositionOptions = () =>
  http.get<CodeOption[]>("/positions/options");

export const createPosition = (data: PositionCreatePayload) =>
  http.post<Position>("/positions", data);

export const updatePosition = (id: string, data: PositionUpdatePayload) =>
  http.put<Position>(`/positions/${enc(id)}`, data);

export const deletePosition = (id: string) =>
  http.delete<null>(`/positions/${enc(id)}`);
