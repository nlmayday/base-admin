<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { ElMessageBox } from "element-plus";
import { usePagedTable } from "@/composables/usePagedTable";
import { confirmAction } from "@/composables/useConfirmAction";
import { hasPerm } from "@/utils/permission";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";
import { formatDateTime } from "@/utils/format";
import {
  deleteJob,
  listJobHandlers,
  listJobs,
  runJobNow,
  setJobEnabled,
  type Job,
  type JobFilter,
  type JobHandler
} from "@/api/ops/job";
import RunStatusTag from "../shared/RunStatusTag.vue";
import { describeCron } from "../shared/cron";
import JobFormDialog from "./components/JobFormDialog.vue";

defineOptions({ name: "OpsJobs" });

/** 手动执行后稍等再刷新，以便看到最新状态 */
const RELOAD_AFTER_RUN_MS = 1500;

const router = useRouter();
const canUpdate = computed(() => hasPerm("ops:job:update"));
const canViewRuns = computed(() => hasPerm("ops:run:view"));
const showRowActions = computed(() =>
  ["ops:job:execute", "ops:job:update", "ops:job:delete", "ops:run:view"].some(
    code => hasPerm(code)
  )
);

const filter = reactive<Required<JobFilter>>({
  keyword: "",
  handlerKey: "",
  enabled: ""
});
const {
  loading,
  rows,
  pagination,
  reload,
  search,
  onSizeChange,
  onCurrentChange
} = usePagedTable<Job>(query => listJobs({ ...query, ...filter }));

const handlers = ref<JobHandler[]>([]);
async function loadHandlers(): Promise<void> {
  try {
    handlers.value = await listJobHandlers();
  } catch {
    handlers.value = [];
  }
}

const dialogVisible = ref(false);
const editing = ref<Job | null>(null);
const switching = ref<Record<string, boolean>>({});

function openCreate(): void {
  editing.value = null;
  dialogVisible.value = true;
}

function openEdit(row: Job): void {
  editing.value = row;
  dialogVisible.value = true;
}

function viewRuns(row: Job): void {
  router.push({ path: "/ops/runs", query: { jobId: row.id } });
}

async function toggleEnabled(row: Job, enabled: boolean): Promise<void> {
  switching.value = { ...switching.value, [row.id]: true };
  try {
    await setJobEnabled(row.id, enabled, row.version);
    message(enabled ? "任务已启用" : "任务已停用", { type: "success" });
    reload();
  } catch (err) {
    if (needsReload(err)) reload();
  } finally {
    switching.value = { ...switching.value, [row.id]: false };
  }
}

async function runNow(row: Job): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确定立即执行任务「${row.name}」吗？任务将在后台异步执行。`,
      "立即执行",
      { type: "info", confirmButtonText: "执行", cancelButtonText: "取消" }
    );
  } catch {
    return; // 用户取消
  }
  try {
    const res = await runJobNow(row.id);
    if (res.status === "skipped") {
      message("任务正在运行，本次执行已跳过", { type: "warning" });
    } else {
      message("已触发执行，可在执行记录中查看结果", { type: "success" });
    }
    reload();
    setTimeout(reload, RELOAD_AFTER_RUN_MS);
  } catch (err) {
    if (needsReload(err)) reload();
  }
}

function remove(row: Job): void {
  confirmAction({
    confirm: `确定删除任务「${row.name}」吗？已有的执行记录会保留。`,
    success: "已删除",
    action: () => deleteJob(row.id),
    reload
  });
}

function resetFilter(): void {
  Object.assign(filter, { keyword: "", handlerKey: "", enabled: "" });
  search();
}

onMounted(() => {
  loadHandlers();
  reload();
});
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3" body-class="pb-0!">
      <el-form :inline="true" :model="filter" @submit.prevent="search">
        <el-form-item label="关键字">
          <el-input
            v-model="filter.keyword"
            placeholder="任务名称/备注"
            clearable
            class="w-[200px]!"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="处理器">
          <el-select
            v-model="filter.handlerKey"
            placeholder="全部"
            clearable
            filterable
            class="w-[200px]!"
          >
            <el-option
              v-for="h in handlers"
              :key="h.key"
              :label="h.name"
              :value="h.key"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select
            v-model="filter.enabled"
            placeholder="全部"
            clearable
            class="w-[120px]!"
          >
            <el-option label="启用" value="true" />
            <el-option label="停用" value="false" />
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
      <div class="mb-3 flex items-center justify-between">
        <span class="card-title">定时任务</span>
        <div class="flex items-center gap-2">
          <el-button :loading="loading" @click="reload">刷新</el-button>
          <el-button
            v-perm="'ops:job:create'"
            type="primary"
            data-testid="job-create"
            @click="openCreate"
          >
            新增任务
          </el-button>
        </div>
      </div>
      <el-table v-loading="loading" :data="rows" row-key="id" border>
        <template #empty>
          <el-empty description="暂无定时任务" :image-size="80" />
        </template>
        <el-table-column label="任务名称" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="font-medium">{{ row.name }}</div>
            <div
              v-if="row.remark"
              class="truncate text-xs text-(--el-text-color-secondary)"
            >
              {{ row.remark }}
            </div>
          </template>
        </el-table-column>
        <el-table-column label="处理器" min-width="170">
          <template #default="{ row }">
            <template v-if="row.handlerExists">
              <div>{{ row.handlerName }}</div>
              <div class="font-mono text-xs text-(--el-text-color-secondary)">
                {{ row.handlerKey }}
              </div>
            </template>
            <el-tooltip
              v-else
              content="该处理器未在当前版本中注册，任务无法执行"
              placement="top"
            >
              <el-tag type="danger" effect="plain">
                未注册：{{ row.handlerKey }}
              </el-tag>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="执行周期" min-width="170">
          <template #default="{ row }">
            <code class="cron-code">{{ row.cronExpr }}</code>
            <div class="mt-1 text-xs text-(--el-text-color-secondary)">
              {{ describeCron(row.cronExpr) || "自定义表达式" }}
            </div>
          </template>
        </el-table-column>
        <el-table-column label="下次执行" width="215">
          <template #default="{ row }">
            <span v-if="row.nextRunAt">{{
              formatDateTime(row.nextRunAt)
            }}</span>
            <span v-else class="text-(--el-text-color-placeholder)">-</span>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="76" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              :disabled="!canUpdate"
              :loading="switching[row.id]"
              @change="(v: boolean) => toggleEnabled(row as Job, v)"
            />
          </template>
        </el-table-column>
        <el-table-column label="上次执行" width="175">
          <template #default="{ row }">
            <RunStatusTag :status="row.lastStatus" />
            <div
              v-if="row.lastRunAt"
              class="mt-1 text-xs text-(--el-text-color-secondary)"
            >
              {{ formatDateTime(row.lastRunAt) }}
            </div>
          </template>
        </el-table-column>
        <el-table-column
          v-if="showRowActions"
          label="操作"
          width="200"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              v-perm="'ops:job:execute'"
              link
              type="primary"
              :disabled="!row.handlerExists"
              @click="runNow(row as Job)"
            >
              执行
            </el-button>
            <el-button
              v-perm="'ops:job:update'"
              link
              type="primary"
              @click="openEdit(row as Job)"
            >
              编辑
            </el-button>
            <el-button
              v-if="canViewRuns"
              link
              type="primary"
              @click="viewRuns(row as Job)"
            >
              记录
            </el-button>
            <el-button
              v-perm="'ops:job:delete'"
              link
              type="danger"
              @click="remove(row as Job)"
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

    <JobFormDialog
      v-model="dialogVisible"
      :job="editing"
      :handlers="handlers"
      @saved="reload"
      @reload="reload"
    />
  </div>
</template>

<style scoped>
.cron-code {
  padding: 1px 6px;
  font-family: var(--el-font-family-mono, ui-monospace, monospace);
  font-size: 12px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
}
</style>
