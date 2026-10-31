<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { listInbox, readAllInbox, type InboxNotice } from "@/api/notice/inbox";
import { useUnreadNotices } from "@/composables/useUnreadNotices";
import { formatDateTime } from "@/utils/format";
import BellIcon from "~icons/ri/notification-3-line";

defineOptions({ name: "LayNotice" });

/** 未读数轮询间隔 */
const POLL_MS = 60_000;
const LATEST = 5;

const router = useRouter();
const { unread, refreshUnread } = useUnreadNotices();
const latest = ref<InboxNotice[]>([]);
const loading = ref(false);
const visible = ref(false);
let timer: ReturnType<typeof setInterval> | undefined;

async function loadLatest(): Promise<void> {
  loading.value = true;
  try {
    const page = await listInbox({ page: 1, pageSize: LATEST, unread: true });
    latest.value = page?.list ?? [];
  } catch {
    latest.value = [];
  } finally {
    loading.value = false;
  }
}

function onShow(): void {
  refreshUnread();
  loadLatest();
}

function open(id?: string): void {
  visible.value = false;
  router.push({ path: "/message/index", query: id ? { id } : {} });
}

async function readAll(): Promise<void> {
  try {
    await readAllInbox();
    unread.value = 0;
    latest.value = [];
  } catch {
    // 错误提示已由 http 拦截器处理
  }
}

function onFocus(): void {
  if (document.visibilityState === "visible") refreshUnread();
}

onMounted(() => {
  refreshUnread();
  timer = setInterval(refreshUnread, POLL_MS);
  document.addEventListener("visibilitychange", onFocus);
});
onBeforeUnmount(() => {
  clearInterval(timer);
  document.removeEventListener("visibilitychange", onFocus);
});
</script>

<template>
  <el-popover
    v-model:visible="visible"
    placement="bottom-end"
    :width="320"
    trigger="click"
    popper-class="lay-notice-popper"
    @show="onShow"
  >
    <template #reference>
      <span
        class="notice-trigger navbar-bg-hover select-none"
        data-testid="nav-notice"
        title="消息"
      >
        <el-badge
          :value="unread"
          :max="99"
          :hidden="unread === 0"
          :offset="[-2, 4]"
        >
          <IconifyIconOffline :icon="BellIcon" />
        </el-badge>
      </span>
    </template>
    <div class="flex items-center justify-between pb-2 border-b">
      <span class="font-medium">未读消息</span>
      <el-button v-if="unread > 0" link type="primary" @click="readAll">
        全部已读
      </el-button>
    </div>
    <div v-loading="loading" class="min-h-[80px]">
      <div
        v-for="n in latest"
        :key="n.id"
        class="notice-item"
        data-testid="nav-notice-item"
        @click="open(n.id)"
      >
        <div class="truncate">{{ n.title }}</div>
        <div class="text-xs text-[var(--el-text-color-secondary)]">
          {{ formatDateTime(n.publishedAt) }}
        </div>
      </div>
      <el-empty
        v-if="!loading && latest.length === 0"
        description="没有未读消息"
        :image-size="48"
      />
    </div>
    <div class="pt-2 text-center border-t">
      <el-button link type="primary" @click="open()">查看全部消息</el-button>
    </div>
  </el-popover>
</template>

<style lang="scss" scoped>
.notice-trigger {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 48px;
  cursor: pointer;
}

.notice-item {
  padding: 8px 4px;
  cursor: pointer;
  border-bottom: 1px solid var(--el-border-color-lighter);

  &:hover {
    background: var(--el-fill-color-light);
  }
}
</style>
