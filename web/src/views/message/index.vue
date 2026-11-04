<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import {
  listInbox,
  readAllInbox,
  readInbox,
  type InboxNotice
} from "@/api/notice/inbox";
import { usePagedTable } from "@/composables/usePagedTable";
import { useDict } from "@/composables/useDict";
import { useUnreadNotices } from "@/composables/useUnreadNotices";
import { formatDateTime } from "@/utils/format";
import { message } from "@/utils/message";
import PersonalMessages from "./components/PersonalMessages.vue";

defineOptions({ name: "MyMessages" });

const route = useRoute();
const { refreshUnread, unreadNotices, unreadMessages } = useUnreadNotices();
/** 公告 / 个人消息；从铃铛点个人消息进来时带 ?tab=messages */
const tab = ref<"notices" | "messages">(
  route.query.tab === "messages" ? "messages" : "notices"
);
const { labelOf: typeLabel, tagTypeOf: typeTagType } = useDict("notice_type");

const onlyUnread = ref(false);
const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<InboxNotice>(q =>
  listInbox({ ...q, unread: onlyUnread.value })
);

const detail = ref<InboxNotice | null>(null);
const detailVisible = ref(false);

async function open(id: string): Promise<void> {
  try {
    detail.value = await readInbox(id);
    detailVisible.value = true;
    rows.value = rows.value.map(r => (r.id === id ? { ...r, read: true } : r));
    refreshUnread();
  } catch {
    // 已撤回等错误由 http 拦截器提示
  }
}

async function readAll(): Promise<void> {
  try {
    const { marked } = await readAllInbox();
    message(marked > 0 ? `已将 ${marked} 条标记为已读` : "没有未读消息", {
      type: "success"
    });
    refreshUnread();
    reload();
  } catch {
    // 错误提示已由 http 拦截器处理
  }
}

/** 从顶栏铃铛点进来时带 ?id=，直接打开该公告 */
async function openFromQuery(): Promise<void> {
  const id = route.query.id;
  if (route.query.tab === "messages") return;
  if (typeof id === "string" && id) await open(id);
}

watch(() => route.query.id, openFromQuery);
watch(
  () => route.query.tab,
  t => {
    if (t === "messages") tab.value = "messages";
  }
);
// 先标记已读再加载列表，列表里的未读标记才是最新的
onMounted(async () => {
  refreshUnread();
  await openFromQuery();
  reload();
});
</script>

<template>
  <div class="main">
    <el-card shadow="never">
      <el-tabs v-model="tab">
        <el-tab-pane name="notices">
          <template #label>
            <el-badge
              :value="unreadNotices"
              :hidden="unreadNotices === 0"
              :max="99"
              class="tab-badge"
            >
              公告
            </el-badge>
          </template>
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
            @row-click="(row: InboxNotice) => open(row.id)"
          >
            <el-table-column label="标题" min-width="260">
              <template #default="{ row }">
                <span class="inline-flex items-center gap-2">
                  <el-badge is-dot :hidden="row.read" />
                  <span :class="row.read ? '' : 'font-medium'">
                    {{ row.title }}
                  </span>
                </span>
              </template>
            </el-table-column>
            <el-table-column label="类型" width="100">
              <template #default="{ row }">
                <el-tag :type="typeTagType(row.type)" disable-transitions>
                  {{ typeLabel(row.type) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="publisher" label="发布人" width="140" />
            <el-table-column label="发布时间" width="215">
              <template #default="{ row }">
                {{ formatDateTime(row.publishedAt) }}
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
        </el-tab-pane>
        <el-tab-pane name="messages" lazy>
          <template #label>
            <el-badge
              :value="unreadMessages"
              :hidden="unreadMessages === 0"
              :max="99"
              class="tab-badge"
            >
              个人消息
            </el-badge>
          </template>
          <PersonalMessages />
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-dialog
      v-model="detailVisible"
      :title="detail?.title"
      width="640px"
      append-to-body
    >
      <div
        v-if="detail"
        class="mb-3 text-xs text-[var(--el-text-color-secondary)]"
      >
        {{ typeLabel(detail.type) }} · {{ detail.publisher || "-" }} ·
        {{ formatDateTime(detail.publishedAt) }}
      </div>
      <div class="notice-content">{{ detail?.content }}</div>
      <div v-if="detail?.attachments?.length" class="mt-4 pt-3 border-t">
        <div class="mb-1 text-sm font-medium">附件</div>
        <div v-for="f in detail.attachments" :key="f.url" class="py-1">
          <el-link type="primary" :href="f.url" target="_blank">
            {{ f.name }}
          </el-link>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.tab-badge :deep(.el-badge__content) {
  top: 8px;
}

.notice-content {
  max-height: 60vh;
  overflow-y: auto;
  line-height: 1.8;
  word-break: break-word;
  white-space: pre-wrap;
}
</style>
