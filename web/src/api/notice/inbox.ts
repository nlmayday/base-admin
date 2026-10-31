import { http } from "@/utils/http";
import { compactParams, type Page, type PageQuery } from "@/api/types";
import type { AttachmentRef } from "@/api/file";

/** 我的消息：已发布的通知公告及当前用户的已读状态 */
export type InboxNotice = {
  id: string;
  title: string;
  /** 字典 notice_type */
  type: number;
  /** 仅详情返回 */
  content?: string;
  /** 仅详情返回 */
  attachments?: AttachmentRef[];
  publishedAt: string;
  read: boolean;
  readAt: string | null;
  publisher: string;
};

export type InboxFilter = { unread?: boolean; title?: string };

const enc = encodeURIComponent;

export const listInbox = (query: PageQuery & InboxFilter) =>
  http.get<Page<InboxNotice>>("/me/notices", compactParams(query));

export const getUnreadCount = () =>
  http.get<{ count: number }>("/me/notices/unread-count", undefined, {
    skipErrorHandler: true
  });

/** 读取详情，同时标记为已读 */
export const readInbox = (id: string) =>
  http.get<InboxNotice>(`/me/notices/${enc(id)}`);

export const readAllInbox = () =>
  http.post<{ marked: number }>("/me/notices/read-all");
