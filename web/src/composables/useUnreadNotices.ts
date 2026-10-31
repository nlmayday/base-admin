import { ref } from "vue";
import { getUnreadCount } from "@/api/notice/inbox";

/** 未读数（公告 + 个人消息），顶栏铃铛与「我的消息」页共享 */
const unread = ref(0);
const unreadNotices = ref(0);
const unreadMessages = ref(0);

async function refreshUnread(): Promise<void> {
  try {
    const c = await getUnreadCount();
    unread.value = c?.count ?? 0;
    unreadNotices.value = c?.notices ?? 0;
    unreadMessages.value = c?.messages ?? 0;
  } catch {
    // 轮询失败不打扰用户，下次再取
  }
}

export function useUnreadNotices() {
  return { unread, unreadNotices, unreadMessages, refreshUnread };
}
