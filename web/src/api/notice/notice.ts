import { http } from "@/utils/http";
import { compactParams, type Page, type PageQuery } from "@/api/types";
import type { AttachmentRef } from "@/api/file";

/** 通知公告（sys_notices）；publishedAt 为空表示草稿 */
export type Notice = {
  /** ID */
  id: string;
  /** 标题 */
  title: string;
  /** 类型 */
  type: number;
  /** 内容 */
  content: string;
  /** 发布时间 */
  publishedAt: string | null;
  /** 附件 */
  attachments: AttachmentRef[];
  /** 版本 */
  version: number;
  /** 创建时间 */
  createdAt: string;
  /** 更新时间 */
  updatedAt: string;
  /** 创建人 */
  createdBy: string | null;
  /** 更新人 */
  updatedBy: string | null;
};
export type NoticeState = "" | "draft" | "published";

export type NoticeFilter = {
  title?: string;
  type?: string;
  state?: NoticeState;
};

export type NoticePayload = {
  title: string;
  type: number;
  content: string;
  attachments: AttachmentRef[];
};

export type NoticeUpdatePayload = NoticePayload & { version: number };

const enc = encodeURIComponent;

export const listNotices = (query: PageQuery & NoticeFilter) =>
  http.get<Page<Notice>>("/notices", compactParams(query));

export const getNotice = (id: string) =>
  http.get<Notice>(`/notices/${enc(id)}`);

export const createNotice = (data: NoticePayload) =>
  http.post<Notice>("/notices", data);

export const updateNotice = (id: string, data: NoticeUpdatePayload) =>
  http.put<Notice>(`/notices/${enc(id)}`, data);

export const deleteNotice = (id: string) =>
  http.delete<null>(`/notices/${enc(id)}`);

/** 发布后所有用户可见，并重新计为未读 */
export const publishNotice = (id: string, version: number) =>
  http.post<Notice>(`/notices/${enc(id)}/publish`, { version });

/** 撤回后回到草稿，用户不再看到 */
export const revokeNotice = (id: string, version: number) =>
  http.post<Notice>(`/notices/${enc(id)}/revoke`, { version });
