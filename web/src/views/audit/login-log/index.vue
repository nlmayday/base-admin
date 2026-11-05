<script setup lang="ts">
import ReUtcHint from "@/components/ReUtcHint/index.vue";
import { onMounted, reactive, ref } from "vue";
import { usePagedTable } from "@/composables/usePagedTable";
import { formatDateTime } from "@/utils/format";
import {
  exportLoginLogs,
  listLoginLogs,
  type LoginLog,
  type LoginLogFilter,
  type LoginLogKind
} from "@/api/audit/loginLog";
import AuditStatsCard from "../components/AuditStatsCard.vue";
import {
  DATE_RANGE_DEFAULT_TIME,
  DATE_RANGE_FORMAT,
  DATE_RANGE_SHORTCUTS,
  SUCCESS_OPTIONS,
  rangeParams,
  reasonLabel,
  shortUserAgent
} from "../utils";

defineOptions({ name: "AuditLogin" });

const KIND_OPTIONS: ReadonlyArray<{ label: string; value: LoginLogKind }> = [
  { label: "登录", value: "login" },
  { label: "退出", value: "logout" }
];

type Filter = {
  username: string;
  ip: string;
  success: "true" | "false" | "";
  kind: LoginLogKind | "";
  range: [string, string] | null;
};

const emptyFilter = (): Filter => ({
  username: "",
  ip: "",
  success: "",
  kind: "",
  range: null
});

const filter = reactive<Filter>(emptyFilter());
const exporting = ref(false);
const statsCard = ref<InstanceType<typeof AuditStatsCard> | null>(null);

function queryFilter(): LoginLogFilter {
  return {
    username: filter.username.trim(),
    ip: filter.ip.trim(),
    success: filter.success,
    kind: filter.kind,
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
} = usePagedTable<LoginLog>(query =>
  listLoginLogs({ ...query, ...queryFilter() })
);

function resetFilter(): void {
  Object.assign(filter, emptyFilter());
  search();
}

function refresh(): void {
  reload();
  statsCard.value?.reload();
}

async function onExport(): Promise<void> {
  exporting.value = true;
  try {
    await exportLoginLogs(queryFilter());
  } catch {
    // 错误提示已由 http 拦截器处理
  } finally {
    exporting.value = false;
  }
}

onMounted(reload);
</script>

<template>
  <div class="main">
    <AuditStatsCard ref="statsCard" kind="login" />

    <el-card shadow="never" class="mb-3" body-class="pb-0!">
      <el-form :inline="true" :model="filter" @submit.prevent="search">
        <el-form-item label="用户名">
          <el-input
            v-model="filter.username"
            placeholder="用户名"
            clearable
            class="w-[160px]!"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="IP">
          <el-input
            v-model="filter.ip"
            placeholder="IP 前缀"
            clearable
            class="w-[150px]!"
            @keyup.enter="search"
          />
        </el-form-item>
        <el-form-item label="类型">
          <el-select
            v-model="filter.kind"
            placeholder="全部"
            clearable
            class="w-[100px]!"
          >
            <el-option
              v-for="o in KIND_OPTIONS"
              :key="o.value"
              :label="o.label"
              :value="o.value"
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
        <span class="card-title">登录日志</span>
        <div>
          <el-button :loading="loading" @click="refresh">刷新</el-button>
          <el-button
            v-perm="'audit:login-log:export'"
            :loading="exporting"
            data-testid="login-log-export"
            @click="onExport"
          >
            导出 CSV
          </el-button>
        </div>
      </div>
      <el-table v-loading="loading" :data="rows" row-key="id" border>
        <template #empty>
          <el-empty description="暂无登录记录" :image-size="80" />
        </template>
        <el-table-column label="时间" width="215">
          <template #default="{ row }">
            {{ formatDateTime(row.createdAt) }}
          </template>
        </el-table-column>
        <el-table-column label="类型" width="80" align="center">
          <template #default="{ row }">
            <el-tag
              :type="row.kind === 'login' ? 'primary' : 'info'"
              effect="plain"
              size="small"
              disable-transitions
            >
              {{ row.kind === "login" ? "登录" : "退出" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          prop="username"
          label="用户名"
          min-width="120"
          show-overflow-tooltip
        >
          <template #default="{ row }">{{ row.username || "-" }}</template>
        </el-table-column>
        <el-table-column label="结果" width="80" align="center">
          <template #default="{ row }">
            <el-tag
              :type="row.success ? 'success' : 'danger'"
              size="small"
              disable-transitions
            >
              {{ row.success ? "成功" : "失败" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="原因" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">
            <span
              v-if="row.reason"
              class="text-[var(--el-color-danger)]"
              :title="row.reason"
            >
              {{ reasonLabel(row.reason) }}
            </span>
            <span v-else class="text-[var(--el-text-color-placeholder)]">
              -
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="ip" label="IP" width="140" show-overflow-tooltip>
          <template #default="{ row }">{{ row.ip || "-" }}</template>
        </el-table-column>
        <el-table-column label="浏览器 / 系统" min-width="180">
          <template #default="{ row }">
            <el-tooltip
              :content="row.userAgent || '-'"
              placement="top"
              :show-after="300"
            >
              <span class="block truncate">{{
                shortUserAgent(row.userAgent)
              }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column
          prop="requestId"
          label="请求 ID"
          min-width="150"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            <span class="font-mono text-xs">{{ row.requestId || "-" }}</span>
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
