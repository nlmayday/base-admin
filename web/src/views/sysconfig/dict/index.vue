<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { confirmAction } from "@/composables/useConfirmAction";
import { invalidateDict } from "@/composables/useDict";
import { hasPerm } from "@/utils/permission";
import { formatDateTime } from "@/utils/format";
import { STATUS_OPTIONS, type Status } from "@/api/types";
import {
  deleteDictItem,
  listDictItems,
  type DictItem,
  type DictType
} from "@/api/system/dict";
import ReStatusTag from "@/components/ReStatusTag/index.vue";
import DictTypePanel from "./components/DictTypePanel.vue";
import DictItemFormDialog from "./components/DictItemFormDialog.vue";

defineOptions({ name: "SystemDict" });

const showRowActions = computed(() =>
  ["system:dict:update", "system:dict:delete"].some(code => hasPerm(code))
);

const panelRef = ref<InstanceType<typeof DictTypePanel>>();
const currentType = ref<DictType | null>(null);
const filter = reactive<{ keyword: string; status: Status | null }>({
  keyword: "",
  status: null
});
const items = ref<DictItem[]>([]);
const loading = ref(false);
const dialogVisible = ref(false);
const editing = ref<DictItem | null>(null);

async function loadItems(): Promise<void> {
  const type = currentType.value;
  if (!type) {
    items.value = [];
    return;
  }
  loading.value = true;
  try {
    const list = await listDictItems(type.id, { ...filter });
    if (currentType.value?.id === type.id) items.value = list ?? [];
  } catch {
    items.value = [];
  } finally {
    loading.value = false;
  }
}

function onTypeSelect(type: DictType | null): void {
  const changed = type?.id !== currentType.value?.id;
  currentType.value = type;
  if (changed) {
    filter.keyword = "";
    filter.status = null;
  }
  loadItems();
}

function resetFilter(): void {
  filter.keyword = "";
  filter.status = null;
  loadItems();
}

/** 字典项变化后刷新右侧列表、左侧计数和前端字典缓存 */
function afterItemChange(): void {
  if (currentType.value) invalidateDict(currentType.value.code);
  panelRef.value?.reload();
}

function openCreate(): void {
  editing.value = null;
  dialogVisible.value = true;
}

function openEdit(row: DictItem): void {
  editing.value = row;
  dialogVisible.value = true;
}

function remove(row: DictItem): void {
  confirmAction({
    confirm: `确定删除字典项「${row.label}」吗？`,
    success: "已删除",
    action: () => deleteDictItem(row.id),
    reload: afterItemChange
  });
}
</script>

<template>
  <div class="main split-page flex gap-3">
    <div class="w-[300px] shrink-0">
      <DictTypePanel ref="panelRef" @select="onTypeSelect" />
    </div>
    <div class="min-w-0 flex-1">
      <el-card v-if="!currentType" shadow="never" class="h-full">
        <el-empty description="请在左侧选择或新增字典类型" />
      </el-card>
      <template v-else>
        <el-card shadow="never" class="mb-3" body-class="pb-0!">
          <el-form :inline="true" :model="filter" @submit.prevent="loadItems">
            <el-form-item label="关键字">
              <el-input
                v-model="filter.keyword"
                placeholder="标签/值"
                clearable
                class="w-[200px]!"
                @keyup.enter="loadItems"
              />
            </el-form-item>
            <el-form-item label="状态">
              <el-select
                v-model="filter.status"
                placeholder="全部"
                clearable
                class="w-[120px]!"
              >
                <el-option
                  v-for="o in STATUS_OPTIONS"
                  :key="o.value"
                  :label="o.label"
                  :value="o.value"
                />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="loading" @click="loadItems">
                搜索
              </el-button>
              <el-button @click="resetFilter">重置</el-button>
            </el-form-item>
          </el-form>
        </el-card>

        <el-card shadow="never">
          <div class="mb-3 flex items-center justify-between gap-3">
            <div class="flex min-w-0 items-center gap-2">
              <span class="card-title truncate">{{ currentType.name }}</span>
              <el-tag type="info" disable-transitions>
                {{ currentType.code }}
              </el-tag>
              <ReStatusTag
                v-if="currentType.status !== 1"
                :status="currentType.status"
              />
            </div>
            <el-button
              v-perm="'system:dict:create'"
              type="primary"
              data-testid="dict-item-create"
              @click="openCreate"
            >
              新增字典项
            </el-button>
          </div>
          <el-alert
            v-if="currentType.status !== 1"
            type="warning"
            :closable="false"
            show-icon
            class="mb-3"
            title="该字典已停用，下拉框和标签中不会返回任何选项"
          />
          <el-table v-loading="loading" :data="items" row-key="id" border>
            <template #empty>
              <el-empty description="暂无字典项" :image-size="80" />
            </template>
            <el-table-column label="字典标签" min-width="140">
              <template #default="{ row }">
                <el-tag :type="row.tagType || undefined" disable-transitions>
                  {{ row.label }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column
              prop="value"
              label="字典值"
              min-width="120"
              show-overflow-tooltip
            />
            <el-table-column
              prop="sort"
              label="排序"
              width="80"
              align="center"
            />
            <el-table-column label="状态" width="90" align="center">
              <template #default="{ row }">
                <ReStatusTag :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column
              prop="remark"
              label="备注"
              min-width="160"
              show-overflow-tooltip
            />
            <el-table-column label="创建时间" width="215">
              <template #default="{ row }">
                {{ formatDateTime(row.createdAt) }}
              </template>
            </el-table-column>
            <el-table-column
              v-if="showRowActions"
              label="操作"
              width="130"
              fixed="right"
            >
              <template #default="{ row }">
                <el-button
                  v-perm="'system:dict:update'"
                  link
                  type="primary"
                  @click="openEdit(row as DictItem)"
                >
                  编辑
                </el-button>
                <el-button
                  v-perm="'system:dict:delete'"
                  link
                  type="danger"
                  @click="remove(row as DictItem)"
                >
                  删除
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </template>
    </div>

    <DictItemFormDialog
      v-if="currentType"
      v-model="dialogVisible"
      :type-id="currentType.id"
      :item="editing"
      @saved="afterItemChange"
      @reload="afterItemChange"
    />
  </div>
</template>
