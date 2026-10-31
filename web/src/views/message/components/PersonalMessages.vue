<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  isSafeLink,
  listMessages,
  readAllMessages,
  readMessage,
  type UserMessage
} from "@/api/notice/messages";
import { usePagedTable } from "@/composables/usePagedTable";
import { useUnreadNotices } from "@/composables/useUnreadNotices";
import { formatDateTime } from "@/utils/format";
import { message } from "@/utils/message";

defineOptions({ name: "PersonalMessages" });

const route = useRoute();
const router = useRouter();
const { refreshUnread } = useUnreadNotices();

const onlyUnread = ref(false);
const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<UserMessage>(q =>
  listMessages({ ...q, unread: onlyUnread.value })
);

const detail = ref<UserMessage | null>(null);
const detailVisible = ref(false);

/** 标记已读；有链接就跳过去，没有就显示内容 */
async function open(id: string, follow = true): Promise<void> {
  try {
    const msg = await readMessage(id);
    rows.value = rows.value.map(r => (r.id === id ? { ...r, read: true } : r));
    refreshUnread();
    if (follow && msg.link && isSafeLink(msg.link)) {
      router.push(msg.link);
      return;
    }
    detail.value = msg;
    detailVisible.value = true;
  } catch {
    // 错误提示已由 http 拦截器处理
  }
}

function goLink(): void {
  const link = detail.value?.link;
  if (!link || !isSafeLink(link)) return;
  detailVisible.value = false;
  router.push(link);
}

async function readAll(): Promise<void> {
  try {
    const { marked } = await readAllMessages();
    message(marked > 0 ? `已将 ${marked} 条标记为已读` : "没有未读消息", {
      type: "success"
    });
    refreshUnread();
    reload();
  } catch {
    // 错误提示已由 http 拦截器处理
  }
}

// 从铃铛点进来时带 ?tab=messages&id=，直接显示该消息（不再跳转）
async function openFromQuery(): Promise<void> {
  const id = route.query.id;
  if (route.query.tab === "messages" && typeof id === "string" && id) {
    await open(id, false);
  }
}

watch(() => route.query.id, openFromQuery);
onMounted(async () => {
  await openFromQuery();
  reload();
});
</script>

<template>
  <div>
    <div class="mb-3 flex items-center justify-between">
      <el-radio-group v-model="onlyUnread" size="small" @change="search">
        <el-radio-button :value="false">全部</el-radio-button>
        <el-radio-button :value="true">未读</el-radio-button>
      </el-radio-group>
      <el-button @click="readAll">全部标记为已读</el-button>
    </div>
    <el-table
      v-loading="loading"
      :data="rows"
      row-key="id"
      class="cursor-pointer"
      data-testid="personal-messages"
      @row-click="(row: UserMessage) => open(row.id)"
    >
      <el-table-column label="标题" min-width="220">
        <template #default="{ row }">
          <span class="inline-flex items-center gap-2">
            <el-badge is-dot :hidden="row.read" />
            <span :class="row.read ? '' : 'font-medium'">{{ row.title }}</span>
          </span>
        </template>
      </el-table-column>
      <el-table-column
        prop="content"
        label="内容"
        min-width="260"
        show-overflow-tooltip
      />
      <el-table-column label="时间" width="170">
        <template #default="{ row }">
          {{ formatDateTime(row.createdAt) }}
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      class="mt-3 justify-end"
      :current-page="pagination.currentPage"
      :page-size="pagination.pageSize"
      :page-sizes="[10, 20, 50, 100]"
      :total="pagination.total"
      layout="total, sizes, prev, pager, next"
      background
      @size-change="onSizeChange"
      @current-change="onCurrentChange"
    />

    <el-dialog
      v-model="detailVisible"
      :title="detail?.title"
      width="560px"
      append-to-body
    >
      <div class="mb-3 text-xs text-[var(--el-text-color-secondary)]">
        {{ formatDateTime(detail?.createdAt) }}
      </div>
      <div class="message-content">{{ detail?.content || "（无内容）" }}</div>
      <template v-if="detail?.link && isSafeLink(detail.link)" #footer>
        <el-button type="primary" @click="goLink">前往查看</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.message-content {
  max-height: 60vh;
  overflow-y: auto;
  line-height: 1.8;
  word-break: break-word;
  white-space: pre-wrap;
}
</style>
