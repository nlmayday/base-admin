<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { usePagedTable } from "@/composables/usePagedTable";
import { formatDateTime } from "@/utils/format";
import {
  exportOperationLogs,
  listOperationLogs,
  type HttpMethod,
  type OperationLog,
  type OperationLogFilter
} from "@/api/audit/operationLog";
import AuditStatsCard from "../components/AuditStatsCard.vue";
import OperationLogDrawer from "./components/OperationLogDrawer.vue";
import {
  DATE_RANGE_DEFAULT_TIME,
  DATE_RANGE_FORMAT,
  DATE_RANGE_SHORTCUTS,
  METHOD_OPTIONS,
  SUCCESS_OPTIONS,
  durationClass,
  methodTag,
  loadModuleNames,
  moduleLabel,
  moduleNames,
  rangeParams,
  statusTag
} from "../utils";

defineOptions({ name: "AuditOperation" });

type Filter = {
  user: string;
  module: string;
  method: HttpMethod | "";
  success: "true" | "false" | "";
  path: string;
  range: [string, string] | null;
};

const emptyFilter = (): Filter => ({
  user: "",
  module: "",
  method: "",
  success: "",
  path: "",
  range: null
});

const filter = reactive<Filter>(emptyFilter());
const exporting = ref(false);
const drawerVisible = ref(false);
const selectedId = ref<string | null>(null);
const statsCard = ref<InstanceType<typeof AuditStatsCard> | null>(null);

function queryFilter(): OperationLogFilter {
  return {
    user: filter.user.trim(),
    module: filter.module,
    method: filter.method,
    success: filter.success,
    path: filter.path.trim(),
    ...rangeParams(filter.range)
  };
}

const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<OperationLog>(query =>
  listOperationLogs({ ...query, ...queryFilter() })
);

function resetFilter(): void {
  Object.assign(filter, emptyFilter());
  search();
}

function refresh(): void {
  reload();
  statsCard.value?.reload();
}

function openDetail(row: OperationLog): void {
  selectedId.value = row.id;
  drawerVisible.value = true;
}

function operatorName(row: OperationLog): string {
  if (!row.userId) return "匿名";
  return row.username || `#${row.userId}`;
}

async function onExport(): Promise<void> {
  exporting.value = true;
  try {
    await exportOperationLogs(queryFilter());
  } catch {
    // 错误提示已由 http 拦截器处理
  } finally {
    exporting.value = false;
  }
}

onMounted(() => {
  loadModuleNames();
  reload();
});
</script>

<template>
  <div class="main">
    <AuditStatsCard ref="statsCard" kind="operation" />

    <el-card shadow="never" class="mb-3" body-class="pb-0!">
      <el-form :inline="true" :model="filter" @submit.prevent="search">
        <el-form-item label="操作人">
          <el-input
            v-model="filter.user"
            placeholder="用户名/昵称"
            clearable
            class="w-[150px]!"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="模块">
          <el-select
            v-model="filter.module"
            placeholder="全部"
            clearable
            filterable
            allow-create
            class="w-[130px]!"
          >
            <el-option
              v-for="o in moduleNames"
              :key="o.value"
              :label="o.label"
              :value="o.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="方法">
          <el-select
            v-model="filter.method"
            placeholder="全部"
            clearable
            class="w-[110px]!"
          >
            <el-option
              v-for="m in METHOD_OPTIONS"
              :key="m"
              :label="m"
              :value="m"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="结果">
          <el-select
            v-model="filter.success"
            placeholder="全部"
            clearable
            class="w-[100px]!"
          >
            <el-option
              v-for="o in SUCCESS_OPTIONS"
              :key="o.value"
              :label="o.label"
              :value="o.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="关键字">
          <el-input
            v-model="filter.path"
            placeholder="路径/操作名称"
            clearable
            class="w-[170px]!"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="时间">
          <el-date-picker
            v-model="filter.range"
            type="datetimerange"
            range-separator="至"
            start-placeholder="开始时间"
            end-placeholder="结束时间"
            :shortcuts="DATE_RANGE_SHORTCUTS"
            :default-time="DATE_RANGE_DEFAULT_TIME"
            :value-format="DATE_RANGE_FORMAT"
            class="w-[340px]!"
          />
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
        <span class="font-medium">操作日志</span>
        <div>
          <el-button :loading="loading" @click="refresh">刷新</el-button>
          <el-button
            v-perm="'audit:operation-log:export'"
            :loading="exporting"
            data-testid="operation-log-export"
            @click="onExport"
          >
            导出 CSV
          </el-button>
        </div>
      </div>
      <el-table v-loading="loading" :data="rows" row-key="id" border>
        <template #empty>
          <el-empty description="暂无操作记录" :image-size="80" />
        </template>
        <el-table-column label="时间" width="170">
          <template #default="{ row }">
            {{ formatDateTime(row.createdAt) }}
          </template>
        </el-table-column>
        <el-table-column label="操作人" min-width="110" show-overflow-tooltip>
          <template #default="{ row }">
            <span
              :class="{
                'text-[var(--el-text-color-placeholder)]': !row.userId
              }"
            >
              {{ operatorName(row as OperationLog) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="模块" width="100" show-overflow-tooltip>
          <template #default="{ row }">{{ moduleLabel(row.module) }}</template>
        </el-table-column>
        <el-table-column label="操作" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">{{ row.summary || "-" }}</template>
        </el-table-column>
        <el-table-column label="方法" width="90" align="center">
          <template #default="{ row }">
            <el-tag
              :type="methodTag(row.method)"
              size="small"
              disable-transitions
            >
              {{ row.method }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          prop="path"
          label="请求路径"
          min-width="200"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="font-mono text-xs">{{ row.path }}</span>
          </template>
        </el-table-column>
        <el-table-column label="结果" width="110" align="center">
          <template #default="{ row }">
            <el-tooltip
              :disabled="!row.errorMessage"
              :content="row.errorMessage"
              placement="top"
            >
              <span class="inline-flex items-center gap-1">
                <el-tag
                  :type="row.success ? 'success' : 'danger'"
                  size="small"
                  disable-transitions
                >
                  {{ row.success ? "成功" : "失败" }}
                </el-tag>
                <el-tag
                  v-if="!row.success"
                  :type="statusTag(row.status)"
                  size="small"
                  effect="plain"
                  disable-transitions
                >
                  {{ row.status }}
                </el-tag>
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="耗时" width="90" align="right">
          <template #default="{ row }">
            <span :class="durationClass(row.durationMs)" class="tabular-nums">
              {{ row.durationMs }} ms
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="ip" label="IP" width="130" show-overflow-tooltip>
          <template #default="{ row }">{{ row.ip || "-" }}</template>
        </el-table-column>
        <el-table-column label="操作" width="80" fixed="right" align="center">
          <template #default="{ row }">
            <el-button
              link
              type="primary"
              @click="openDetail(row as OperationLog)"
            >
              详情
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

    <OperationLogDrawer v-model="drawerVisible" :log-id="selectedId" />
  </div>
</template>
