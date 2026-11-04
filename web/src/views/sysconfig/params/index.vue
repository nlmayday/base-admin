<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { usePagedTable } from "@/composables/usePagedTable";
import { confirmAction } from "@/composables/useConfirmAction";
import { hasPerm } from "@/utils/permission";
import { formatDateTime } from "@/utils/format";
import {
  CONFIG_VALUE_TYPES,
  deleteConfig,
  getConfigGroups,
  listConfigs,
  type Config,
  type ConfigValueType
} from "@/api/system/config";
import ConfigFormDialog from "./components/ConfigFormDialog.vue";
import ConfigHistoryDrawer from "./components/ConfigHistoryDrawer.vue";

defineOptions({ name: "SystemConfig" });

const router = useRouter();

const TYPE_LABEL = Object.fromEntries(
  CONFIG_VALUE_TYPES.map(t => [t.value, t.label])
) as Record<ConfigValueType, string>;

const TYPE_TAG: Record<
  ConfigValueType,
  "primary" | "success" | "warning" | "info"
> = {
  string: "info",
  number: "primary",
  boolean: "success",
  json: "warning"
};

const filter = reactive({ keyword: "", group: "", includeBuiltin: false });
const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<Config>(query => listConfigs({ ...query, ...filter }));

const groups = ref<string[]>([]);
const dialogVisible = ref(false);
const historyVisible = ref(false);
const editing = ref<Config | null>(null);
const viewing = ref<Config | null>(null);

const canUpdate = computed(() => hasPerm("system:config:update"));
const canDelete = computed(() => hasPerm("system:config:delete"));
const canOpenSettings = computed(() => hasPerm("system:settings:view"));

/** 内置参数只能在「基础设置」中修改；只读参数不能修改或删除 */
function lockReason(row: Config): string {
  if (row.builtin) return "内置参数，请在「基础设置」中修改";
  if (!row.editable) return "只读参数";
  return "";
}

async function loadGroups(): Promise<void> {
  try {
    groups.value = (await getConfigGroups(filter.includeBuiltin)) ?? [];
  } catch {
    groups.value = [];
  }
}

function reloadAll(): void {
  reload();
  loadGroups();
}

function resetFilter(): void {
  filter.keyword = "";
  filter.group = "";
  filter.includeBuiltin = false;
  loadGroups();
  search();
}

/** 切换是否显示内置参数：分组随之变化，已选分组可能不再存在 */
function onBuiltinToggle(): void {
  filter.group = "";
  loadGroups();
  search();
}

function openCreate(): void {
  editing.value = null;
  dialogVisible.value = true;
}

function openEdit(row: Config): void {
  editing.value = row;
  dialogVisible.value = true;
}

function openHistory(row: Config): void {
  viewing.value = row;
  historyVisible.value = true;
}

function remove(row: Config): void {
  confirmAction({
    confirm: `确定删除参数「${row.name}」（${row.key}）吗？变更记录会一并删除。`,
    success: "已删除",
    action: () => deleteConfig(row.id),
    reload: reloadAll
  });
}

function displayValue(row: Config): string {
  if (row.valueType === "json") {
    return row.value.replace(/\s+/g, " ");
  }
  return row.value === "" ? "（空）" : row.value;
}

function goSettings(): void {
  router.push("/sysconfig/settings");
}

onMounted(() => {
  reload();
  loadGroups();
});
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3" body-class="pb-0!">
      <el-form :inline="true" :model="filter" @submit.prevent="search">
        <el-form-item label="关键字">
          <el-input
            v-model="filter.keyword"
            placeholder="参数键/名称"
            clearable
            class="w-[220px]!"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="分组">
          <el-select
            v-model="filter.group"
            placeholder="全部"
            clearable
            filterable
            class="w-[160px]!"
            @change="search"
          >
            <el-option v-for="g in groups" :key="g" :label="g" :value="g" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-tooltip
            content="内置参数（站点名称、Logo、上传规则、日志保留天数等）只能在「基础设置」中修改，这里仅供查看"
            placement="top"
          >
            <el-checkbox
              v-model="filter.includeBuiltin"
              data-testid="config-include-builtin"
              @change="onBuiltinToggle"
            >
              显示内置参数
            </el-checkbox>
          </el-tooltip>
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
      <div class="mb-3 flex items-center justify-between">
        <span class="font-medium">参数列表</span>
        <div>
          <el-button v-if="canOpenSettings" @click="goSettings">
            基础设置
          </el-button>
          <el-button
            v-perm="'system:config:create'"
            type="primary"
            data-testid="config-create"
            @click="openCreate"
          >
            新增参数
          </el-button>
        </div>
      </div>
      <el-table v-loading="loading" :data="rows" row-key="id" border>
        <template #empty>
          <el-empty
            :description="
              filter.includeBuiltin
                ? '暂无参数'
                : '暂无自定义参数。站点名称、Logo、上传规则等内置参数请在「基础设置」中修改'
            "
            :image-size="80"
          />
        </template>
        <el-table-column label="参数名称" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.name }}</span>
            <el-tag
              v-if="row.builtin"
              size="small"
              type="warning"
              class="ml-1"
              disable-transitions
            >
              内置
            </el-tag>
            <el-tag
              v-else-if="!row.editable"
              size="small"
              type="info"
              class="ml-1"
              disable-transitions
            >
              只读
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="参数键" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="font-mono text-[13px]">{{ row.key }}</span>
          </template>
        </el-table-column>
        <el-table-column label="参数值" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <el-tag
              v-if="row.valueType === 'boolean'"
              :type="row.value === 'true' ? 'success' : 'info'"
              disable-transitions
            >
              {{ row.value === "true" ? "是" : "否" }}
            </el-tag>
            <span
              v-else
              :class="{
                'font-mono text-[13px]': row.valueType !== 'string',
                'text-[var(--el-text-color-placeholder)]': row.value === ''
              }"
            >
              {{ displayValue(row as Config) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="90" align="center">
          <template #default="{ row }">
            <el-tag
              size="small"
              :type="TYPE_TAG[row.valueType as ConfigValueType]"
              disable-transitions
            >
              {{ TYPE_LABEL[row.valueType as ConfigValueType] }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          prop="groupName"
          label="分组"
          width="120"
          show-overflow-tooltip
        />
        <el-table-column
          prop="description"
          label="说明"
          min-width="160"
          show-overflow-tooltip
        />
        <el-table-column label="更新时间" width="215">
          <template #default="{ row }">
            {{ formatDateTime(row.updatedAt) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="170" fixed="right">
          <template #default="{ row }">
            <el-tooltip
              v-if="canUpdate"
              :disabled="!lockReason(row as Config)"
              :content="lockReason(row as Config)"
              placement="top"
            >
              <span class="mr-3">
                <el-button
                  link
                  type="primary"
                  :disabled="!!lockReason(row as Config)"
                  @click="openEdit(row as Config)"
                >
                  编辑
                </el-button>
              </span>
            </el-tooltip>
            <el-button link type="primary" @click="openHistory(row as Config)">
              记录
            </el-button>
            <el-button
              v-if="canDelete && !lockReason(row as Config)"
              link
              type="danger"
              @click="remove(row as Config)"
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

    <ConfigFormDialog
      v-model="dialogVisible"
      :config="editing"
      :groups="groups"
      @saved="reloadAll"
      @reload="reloadAll"
    />
    <ConfigHistoryDrawer v-model="historyVisible" :config="viewing" />
  </div>
</template>
