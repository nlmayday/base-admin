<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { usePagedTable } from "@/composables/usePagedTable";
import { confirmAction } from "@/composables/useConfirmAction";
import {
  deleteNotice,
  listNotices,
  publishNotice,
  revokeNotice,
  type Notice,
  type NoticeFilter
} from "@/api/notice/notice";
import { formatDateTime } from "@/utils/format";
import { hasPerm } from "@/utils/permission";
import { useDict } from "@/composables/useDict";
import NoticeFormDialog from "./components/NoticeFormDialog.vue";

defineOptions({ name: "NoticeNotice" });

/** 没有任何行操作权限时隐藏「操作」列 */
const showRowActions = computed(() =>
  [
    "notice:notice:update",
    "notice:notice:delete",
    "notice:notice:publish"
  ].some(code => hasPerm(code))
);
const {
  items: typeOptions,
  labelOf: typeLabel,
  tagTypeOf: typeTagType
} = useDict("notice_type");

const emptyFilter = (): NoticeFilter => ({ title: "", type: "", state: "" });
const filter = reactive<NoticeFilter>(emptyFilter());
const query = (): NoticeFilter => ({ ...filter });
const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<Notice>(q => listNotices({ ...q, ...query() }));

const dialogVisible = ref(false);
const editing = ref<Notice | null>(null);

function openCreate(): void {
  editing.value = null;
  dialogVisible.value = true;
}

function openEdit(row: Notice): void {
  editing.value = row;
  dialogVisible.value = true;
}

function remove(row: Notice): void {
  confirmAction({
    confirm: `确定删除通知公告「${row.title}」吗？`,
    success: "已删除",
    action: () => deleteNotice(row.id),
    reload
  });
}

function publish(row: Notice): void {
  confirmAction({
    confirm: `发布后所有用户都会收到「${row.title}」，确定发布吗？`,
    success: "已发布",
    type: "info",
    action: () => publishNotice(row.id, row.version),
    reload
  });
}

function revoke(row: Notice): void {
  confirmAction({
    confirm: `撤回后用户将看不到「${row.title}」，确定撤回吗？`,
    success: "已撤回",
    action: () => revokeNotice(row.id, row.version),
    reload
  });
}

function resetFilter(): void {
  Object.assign(filter, emptyFilter());
  search();
}

onMounted(reload);
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3" body-class="pb-0!">
      <el-form :inline="true" :model="filter" @submit.prevent="search">
        <el-form-item label="标题">
          <el-input
            v-model="filter.title"
            placeholder="标题"
            clearable
            class="w-[180px]!"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="类型">
          <el-select
            v-model="filter.type"
            placeholder="全部"
            clearable
            class="w-[140px]!"
          >
            <el-option
              v-for="o in typeOptions"
              :key="o.value"
              :label="o.label"
              :value="o.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select
            v-model="filter.state"
            placeholder="全部"
            clearable
            class="w-[120px]!"
          >
            <el-option label="草稿" value="draft" />
            <el-option label="已发布" value="published" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="search">
            搜索
          </el-button>
          <el-button @click="resetFilter">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>
    <el-card shadow="never">
      <div class="mb-3 flex justify-between">
        <span class="font-medium">通知公告列表</span>
        <div>
          <el-button
            v-perm="'notice:notice:create'"
            type="primary"
            @click="openCreate"
          >
            新增通知公告
          </el-button>
        </div>
      </div>
      <el-table v-loading="loading" :data="rows" row-key="id" border>
        <el-table-column
          prop="title"
          label="标题"
          min-width="180"
          show-overflow-tooltip
        />
        <el-table-column label="类型" min-width="90">
          <template #default="{ row }">
            <el-tag
              v-if="typeLabel(row.type) !== '-'"
              :type="typeTagType(row.type)"
              disable-transitions
            >
              {{ typeLabel(row.type) }}
            </el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag
              :type="row.publishedAt ? 'success' : 'info'"
              disable-transitions
            >
              {{ row.publishedAt ? "已发布" : "草稿" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="发布时间" width="170">
          <template #default="{ row }">
            {{ formatDateTime(row.publishedAt) }}
          </template>
        </el-table-column>
        <el-table-column
          v-if="showRowActions"
          label="操作"
          width="190"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              v-if="!row.publishedAt"
              v-perm="'notice:notice:publish'"
              link
              type="success"
              @click="publish(row as Notice)"
            >
              发布
            </el-button>
            <el-button
              v-else
              v-perm="'notice:notice:publish'"
              link
              type="warning"
              @click="revoke(row as Notice)"
            >
              撤回
            </el-button>
            <el-button
              v-perm="'notice:notice:update'"
              link
              type="primary"
              @click="openEdit(row as Notice)"
            >
              编辑
            </el-button>
            <el-button
              v-perm="'notice:notice:delete'"
              link
              type="danger"
              @click="remove(row as Notice)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        class="mt-3 justify-end"
        :current-page="pagination.currentPage"
        :page-size="pagination.pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        background
        @size-change="onSizeChange"
        @current-change="onCurrentChange"
      />
    </el-card>

    <NoticeFormDialog
      v-model="dialogVisible"
      :row="editing"
      @saved="reload"
      @reload="reload"
    />
  </div>
</template>
