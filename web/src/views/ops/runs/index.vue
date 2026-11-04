<script setup lang="ts">
import ReUtcHint from "@/components/ReUtcHint/index.vue";
import {
  computed,
  onBeforeUnmount,
  onMounted,
  reactive,
  ref,
  watch
} from "vue";
import { useRoute } from "vue-router";
import { usePagedTable } from "@/composables/usePagedTable";
import { formatDateTime } from "@/utils/format";
import { hasPerm } from "@/utils/permission";
import { listJobs, type Job, type RunStatus } from "@/api/ops/job";
import { listRuns, type JobRun, type RunTrigger } from "@/api/ops/run";
import RunStatusTag from "../shared/RunStatusTag.vue";
import { formatDuration } from "../shared/format";
import {
  RUN_STATUS_OPTIONS,
  TRIGGER_META,
  TRIGGER_OPTIONS
} from "../shared/status";
import RunDetail from "./components/RunDetail.vue";

defineOptions({ name: "OpsRuns" });

/** 有运行中的记录时自动刷新的间隔 */
const RUNNING_POLL_MS = 3000;
const JOB_OPTIONS_PAGE_SIZE = 100;

const route = useRoute();

type RunFilterForm = {
  jobId: string;
  status: RunStatus | "";
  trigger: RunTrigger | "";
  range: [Date, Date] | null;
};

const filter = reactive<RunFilterForm>({
  jobId: typeof route.query.jobId === "string" ? route.query.jobId : "",
  status: "",
  trigger: "",
  range: null
});

const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<JobRun>(query =>
  listRuns({
    ...query,
    jobId: filter.jobId,
    status: filter.status,
    trigger: filter.trigger,
    startFrom: filter.range?.[0]?.toISOString(),
    startTo: filter.range?.[1]?.toISOString()
  })
);

const jobs = ref<Job[]>([]);
/** 任务下拉需要 ops:job:view；没有时只保留来自 URL 的筛选 */
const canListJobs = hasPerm("ops:job:view");
async function loadJobs(): Promise<void> {
  if (!canListJobs) return;
  try {
    const page = await listJobs({ page: 1, pageSize: JOB_OPTIONS_PAGE_SIZE });
    jobs.value = page.list;
  } catch {
    jobs.value = [];
  }
}

const dateShortcuts = [
  { text: "最近 1 小时", value: () => lastMs(3600_000) },
  { text: "最近 24 小时", value: () => lastMs(86400_000) },
  { text: "最近 7 天", value: () => lastMs(7 * 86400_000) }
];
function lastMs(ms: number): [Date, Date] {
  const end = new Date();
  return [new Date(end.getTime() - ms), end];
}

function resetFilter(): void {
  Object.assign(filter, { jobId: "", status: "", trigger: "", range: null });
  search();
}

// 有运行中的记录时定期刷新，直到全部结束
const hasRunning = computed(() => rows.value.some(r => r.status === "running"));
let pollTimer: ReturnType<typeof setTimeout> | undefined;
watch([hasRunning, loading], ([running, busy]) => {
  clearTimeout(pollTimer);
  if (running && !busy) pollTimer = setTimeout(reload, RUNNING_POLL_MS);
});
onBeforeUnmount(() => clearTimeout(pollTimer));

watch(
  () => route.query.jobId,
  id => {
    if (typeof id === "string" && id !== filter.jobId) {
      filter.jobId = id;
      search();
    }
  }
);

onMounted(() => {
  loadJobs();
  reload();
});
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3" body-class="pb-0!">
      <el-form :inline="true" :model="filter" @submit.prevent="search">
        <el-form-item v-if="canListJobs" label="任务">
          <el-select
            v-model="filter.jobId"
            placeholder="全部任务"
            clearable
            filterable
            class="w-[200px]!"
          >
            <el-option
              v-for="j in jobs"
              :key="j.id"
              :label="j.name"
              :value="j.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select
            v-model="filter.status"
            placeholder="全部"
            clearable
            class="w-[120px]!"
          >
            <el-option
              v-for="o in RUN_STATUS_OPTIONS"
              :key="o.value"
              :label="o.label"
              :value="o.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="触发方式">
          <el-select
            v-model="filter.trigger"
            placeholder="全部"
            clearable
            class="w-[110px]!"
          >
            <el-option
              v-for="o in TRIGGER_OPTIONS"
              :key="o.value"
              :label="o.label"
              :value="o.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="开始时间">
          <el-date-picker
            v-model="filter.range"
            type="datetimerange"
            range-separator="至"
            start-placeholder="开始"
            end-placeholder="结束"
            :shortcuts="dateShortcuts"
            class="w-[340px]!"
          />
          <ReUtcHint :value="filter.range" />
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
        <span class="font-medium">
          执行记录
          <span
            v-if="hasRunning"
            class="ml-2 text-xs font-normal text-(--el-text-color-secondary)"
          >
            有任务正在运行，列表会自动刷新
          </span>
        </span>
        <el-button :loading="loading" @click="reload">刷新</el-button>
      </div>
      <el-table v-loading="loading" :data="rows" row-key="id" border>
        <template #empty>
          <el-empty description="暂无执行记录" :image-size="80" />
        </template>
        <el-table-column type="expand" width="40">
          <template #default="{ row }">
            <RunDetail :run="row as JobRun" />
          </template>
        </el-table-column>
        <el-table-column label="任务" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.jobName }}</span>
            <el-tag
              v-if="!row.jobId"
              size="small"
              type="info"
              effect="plain"
              class="ml-1"
            >
              已删除
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          prop="handlerKey"
          label="处理器"
          min-width="150"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="font-mono text-xs">{{ row.handlerKey }}</span>
          </template>
        </el-table-column>
        <el-table-column label="触发" width="72" align="center">
          <template #default="{ row }">
            <el-tag
              :type="TRIGGER_META[row.trigger as RunTrigger].type"
              size="small"
              effect="plain"
            >
              {{ TRIGGER_META[row.trigger as RunTrigger].label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="96" align="center">
          <template #default="{ row }">
            <RunStatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="尝试" width="64" align="center">
          <template #default="{ row }">
            {{ row.attempt }}
          </template>
        </el-table-column>
        <el-table-column label="开始时间" width="215">
          <template #default="{ row }">
            {{ formatDateTime(row.startedAt) }}
          </template>
        </el-table-column>
        <el-table-column label="耗时" width="100" align="right">
          <template #default="{ row }">
            {{
              row.status === "running" ? "-" : formatDuration(row.durationMs)
            }}
          </template>
        </el-table-column>
        <el-table-column label="错误信息" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.error" class="text-(--el-color-danger)">
              {{ row.error }}
            </span>
            <span v-else class="text-(--el-text-color-placeholder)">-</span>
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
  </div>
</template>
