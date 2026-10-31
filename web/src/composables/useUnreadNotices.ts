import { ref } from "vue";
import { getUnreadCount } from "@/api/notice/inbox";

/** 未读消息数，顶栏铃铛与「我的消息」页共享 */
const unread = ref(0);

async function refreshUnread(): Promise<void> {
  try {
    unread.value = (await getUnreadCount())?.count ?? 0;
  } catch {
    // 轮询失败不打扰用户，下次再取
  }
}

export function useUnreadNotices() {
  return { unread, refreshUnread };
}
