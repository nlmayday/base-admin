<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { usePagedTable } from "@/composables/usePagedTable";
import { confirmAction } from "@/composables/useConfirmAction";
import { invalidateDict } from "@/composables/useDict";
import { hasPerm } from "@/utils/permission";
import {
  deleteDictType,
  listDictTypes,
  type DictType
} from "@/api/system/dict";
import DictTypeFormDialog from "./DictTypeFormDialog.vue";

defineOptions({ name: "DictTypePanel" });

const emit = defineEmits<{ (e: "select", value: DictType | null): void }>();

const filter = reactive({ keyword: "" });
const {
  loading,
  rows,
  pagination,
  reload: reloadRows,
  search,
  onCurrentChange
} = usePagedTable<DictType>(query => listDictTypes({ ...query, ...filter }));

const selectedId = ref<string | null>(null);
const dialogVisible = ref(false);
const editing = ref<DictType | null>(null);

const showRowActions = computed(() =>
  ["system:dict:update", "system:dict:delete"].some(code => hasPerm(code))
);

/** 重新加载列表并同步当前选中项（版本号、名称可能已变化） */
async function reload(): Promise<void> {
  await reloadRows();
  const current = rows.value.find(r => r.id === selectedId.value) ?? null;
  if (!current && rows.value.length > 0 && selectedId.value === null) {
    select(rows.value[0]);
    return;
  }
  if (!current) selectedId.value = null;
  emit("select", current);
}

function select(row: DictType): void {
  selectedId.value = row.id;
  emit("select", row);
}

function rowClassName({ row }: { row: DictType }): string {
  return row.id === selectedId.value ? "dict-type-row--active" : "";
}

function openCreate(): void {
  editing.value = null;
  dialogVisible.value = true;
}

function openEdit(row: DictType): void {
  editing.value = row;
  dialogVisible.value = true;
}

function onSaved(saved: DictType): void {
  invalidateDict(saved.code);
  selectedId.value = saved.id;
  reload();
}

function remove(row: DictType): void {
  confirmAction({
    confirm: `确定删除字典「${row.name}」吗？其下 ${row.itemCount} 个字典项会一并删除。`,
    success: "已删除",
    action: () => deleteDictType(row.id),
    reload: () => {
      invalidateDict(row.code);
      if (row.id === selectedId.value) selectedId.value = null;
      return reload();
    }
  });
}

function onSearch(): void {
  search().then(() => {
    if (!rows.value.some(r => r.id === selectedId.value)) {
      selectedId.value = null;
      emit("select", null);
    }
  });
}

onMounted(reload);

defineExpose({ reload });
</script>

<template>
  <el-card shadow="never" class="h-full" body-class="p-3!">
    <template #header>
      <div class="flex items-center justify-between">
        <span class="font-medium">字典类型</span>
        <el-button
          v-perm="'system:dict:create'"
          type="primary"
          size="small"
          data-testid="dict-type-create"
          @click="openCreate"
        >
          新增
        </el-button>
      </div>
    </template>
    <el-input
      v-model="filter.keyword"
      placeholder="搜索编码/名称"
      clearable
      class="mb-2"
      @keyup.enter="onSearch"
      @clear="onSearch"
    />
    <el-table
      v-loading="loading"
      :data="rows"
      row-key="id"
      :row-class-name="rowClassName"
      :show-header="false"
      class="dict-type-table"
      @row-click="select"
    >
      <template #empty>
        <el-empty description="暂无字典" :image-size="60" />
      </template>
      <el-table-column min-width="150">
        <template #default="{ row }">
          <div class="min-w-0 cursor-pointer">
            <div class="flex items-center gap-1">
              <span class="truncate font-medium">{{ row.name }}</span>
              <el-tag
                v-if="row.status !== 1"
                size="small"
                type="info"
                disable-transitions
              >
                停用
              </el-tag>
            </div>
            <div class="truncate text-xs text-[var(--el-text-color-secondary)]">
              {{ row.code }} · {{ row.itemCount }} 项
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column v-if="showRowActions" width="96" align="right">
        <template #default="{ row }">
          <el-button
            v-perm="'system:dict:update'"
            link
            type="primary"
            @click.stop="openEdit(row as DictType)"
          >
            编辑
          </el-button>
          <el-button
            v-perm="'system:dict:delete'"
            link
            type="danger"
            @click.stop="remove(row as DictType)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      v-if="pagination.total > pagination.pageSize"
      class="mt-2 justify-center"
      small
      :current-page="pagination.currentPage"
      :page-size="pagination.pageSize"
      :total="pagination.total"
      layout="prev, pager, next"
      :pager-count="5"
      @current-change="onCurrentChange"
    />

    <DictTypeFormDialog
      v-model="dialogVisible"
      :dict-type="editing"
      @saved="onSaved"
      @reload="reload"
    />
  </el-card>
</template>

<style scoped>
.dict-type-table :deep(.dict-type-row--active > td.el-table__cell) {
  background-color: var(--el-color-primary-light-9);
}

.dict-type-table :deep(.el-table__row) {
  cursor: pointer;
}
</style>
