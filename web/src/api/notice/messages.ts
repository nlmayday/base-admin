import { http } from "@/utils/http";
import { compactParams, type Page, type PageQuery } from "@/api/types";

/** 个人消息：业务模块发给当前用户的提醒 */
export type UserMessage = {
  id: string;
  /** 来源，如 crm.reminder */
  category: string;
  title: string;
  content: string;
  /** 点击后打开的前端路由，空表示没有 */
  link: string;
  read: boolean;
  readAt: string | null;
  createdAt: string;
};

export type MessageFilter = { unread?: boolean; title?: string };

const enc = encodeURIComponent;

export const listMessages = (query: PageQuery & MessageFilter) =>
  http.get<Page<UserMessage>>("/me/messages", compactParams(query));

/** 读取详情，同时标记为已读 */
export const readMessage = (id: string) =>
  http.get<UserMessage>(`/me/messages/${enc(id)}`);

export const readAllMessages = () =>
  http.post<{ marked: number }>("/me/messages/read-all");

/** 消息的跳转链接只接受站内路由，防止跳到其他网站 */
export function isSafeLink(link: string): boolean {
  return link.startsWith("/") && !link.startsWith("//") && !link.includes("\\");
}
