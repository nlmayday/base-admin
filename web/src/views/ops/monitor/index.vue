<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from "vue";
import { formatDateTime } from "@/utils/format";
import {
  getAppStats,
  getServerStats,
  type AppStats,
  type DiskStats,
  type ServerStats
} from "@/api/ops/monitor";
import { formatBytes, formatUptime, percentColor } from "../shared/format";
import GaugeCard from "./components/GaugeCard.vue";
import HistoryChart from "./components/HistoryChart.vue";
import type { HistoryPoint } from "./types";
import AppPanel from "./components/AppPanel.vue";
import ServerIcon from "~icons/ri/server-line";
import TimeIcon from "~icons/ri/time-line";

defineOptions({ name: "OpsMonitor" });

const REFRESH_MS = 5000;
/** 保留最近 10 分钟的历史点 */
const HISTORY_LIMIT = 120;

const server = shallowRef<ServerStats | null>(null);
const app = shallowRef<AppStats | null>(null);
const history = shallowRef<HistoryPoint[]>([]);
const loading = ref(false);
const autoRefresh = ref(true);
const failed = ref(false);
const lastUpdated = ref<string | null>(null);

let timer: ReturnType<typeof setTimeout> | undefined;

function pushHistory(s: ServerStats): void {
  const point: HistoryPoint = {
    time: new Date(s.collectedAt).getTime(),
    cpu: s.cpu.percent,
    memory: s.memory.percent
  };
  const prev = history.value;
  if (prev.length && prev[prev.length - 1].time === point.time) return;
  history.value = [...prev, point].slice(-HISTORY_LIMIT);
}

async function refresh(): Promise<void> {
  clearTimeout(timer);
  loading.value = true;
  try {
    const [s, a] = await Promise.all([getServerStats(), getAppStats()]);
    server.value = s;
    app.value = a;
    pushHistory(s);
    lastUpdated.value = s.collectedAt;
    failed.value = false;
  } catch {
    // 错误提示已由拦截器处理；暂停自动刷新以免重复提示
    failed.value = true;
    autoRefresh.value = false;
  } finally {
    loading.value = false;
    schedule();
  }
}

function schedule(): void {
  clearTimeout(timer);
  if (autoRefresh.value) timer = setTimeout(refresh, REFRESH_MS);
}

function onToggleAuto(enabled: boolean): void {
  autoRefresh.value = enabled;
  if (enabled) refresh();
  else clearTimeout(timer);
}

/** 主磁盘：根目录，否则取使用率最高的 */
const primaryDisk = computed<DiskStats | null>(() => {
  const disks = server.value?.disks ?? [];
  return (
    disks.find(d => d.mount === "/") ??
    [...disks].sort((a, b) => b.percent - a.percent)[0] ??
    null
  );
});

const hostItems = computed(() => {
  const s = server.value;
  if (!s) return [];
  const h = s.host;
  return [
    { label: "主机名", value: h.hostname },
    {
      label: "操作系统",
      value: [h.platform || h.os, h.platformVersion].filter(Boolean).join(" ")
    },
    {
      label: "内核",
      value: `${h.kernelVersion || "-"} (${h.kernelArch || "-"})`
    },
    { label: "CPU", value: s.cpu.modelName || "-" },
    {
      label: "核心数",
      value: `${s.cpu.physicalCores || "-"} 物理 / ${s.cpu.logicalCores} 逻辑`
    },
    { label: "进程数", value: String(h.procs) },
    { label: "启动时间", value: formatDateTime(h.bootTime) }
  ];
});

const loadText = computed(() => {
  const l = server.value?.load;
  return l
    ? `负载 ${l.load1.toFixed(2)} / ${l.load5.toFixed(2)} / ${l.load15.toFixed(2)}`
    : "负载不可用";
});

onMounted(refresh);
onBeforeUnmount(() => clearTimeout(timer));
</script>

<template>
  <div class="main">
    <el-card shadow="never" class="mb-3" body-class="py-3!">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-3">
          <span class="text-base font-medium">服务器监控</span>
          <el-tag v-if="failed" type="danger" effect="plain" size="small">
            刷新失败，已暂停自动刷新
          </el-tag>
          <span v-else class="text-xs text-(--el-text-color-secondary)">
            更新于 {{ formatDateTime(lastUpdated) }}
          </span>
        </div>
        <div class="flex items-center gap-3">
          <el-switch
            :model-value="autoRefresh"
            inline-prompt
            active-text="自动刷新"
            inactive-text="已暂停"
            class="auto-refresh-switch"
            @change="(v: boolean) => onToggleAuto(v)"
          />
          <span class="text-xs text-(--el-text-color-secondary)">每 5 秒</span>
          <el-button :loading="loading" @click="refresh">刷新</el-button>
        </div>
      </div>
    </el-card>

    <el-row :gutter="12">
      <el-col :xs="24" :lg="8" class="mb-3">
        <el-card shadow="never" class="h-full">
          <template #header>
            <div class="flex items-center gap-2">
              <ServerIcon class="text-(--el-color-primary)" />
              <span class="font-medium">主机信息</span>
            </div>
          </template>
          <div v-if="server">
            <div class="uptime mb-3">
              <TimeIcon class="text-lg text-(--el-color-success)" />
              <div>
                <div class="text-xs text-(--el-text-color-secondary)">
                  系统已运行
                </div>
                <div class="text-lg font-semibold">
                  {{ formatUptime(server.host.uptimeSeconds) }}
                </div>
              </div>
              <div v-if="app" class="ml-auto text-right">
                <div class="text-xs text-(--el-text-color-secondary)">
                  服务已运行
                </div>
                <div class="text-lg font-semibold">
                  {{ formatUptime(app.process.uptimeSeconds) }}
                </div>
              </div>
            </div>
            <dl class="kv-list">
              <template v-for="item in hostItems" :key="item.label">
                <dt>{{ item.label }}</dt>
                <dd :title="item.value">{{ item.value }}</dd>
              </template>
              <template v-if="app">
                <dt>实例</dt>
                <dd :title="app.process.instanceId">
                  {{ app.process.instanceId || "-" }}
                  <span class="text-(--el-text-color-secondary)">
                    · PID {{ app.process.pid }} ·
                    {{ app.process.version || "-" }}
                  </span>
                </dd>
              </template>
            </dl>
          </div>
          <el-skeleton v-else :rows="6" animated />
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="16" class="mb-3">
        <el-row :gutter="12" class="h-full">
          <el-col :xs="24" :sm="8" class="mb-3">
            <GaugeCard
              title="CPU"
              :percent="server?.cpu.percent ?? 0"
              :loading="!server"
            >
              {{ server ? loadText : "-" }}
            </GaugeCard>
          </el-col>
          <el-col :xs="24" :sm="8" class="mb-3">
            <GaugeCard
              title="内存"
              :percent="server?.memory.percent ?? 0"
              :loading="!server"
              :detail="
                server
                  ? `${formatBytes(server.memory.used)} / ${formatBytes(server.memory.total)}`
                  : ''
              "
            />
          </el-col>
          <el-col :xs="24" :sm="8" class="mb-3">
            <GaugeCard
              title="磁盘"
              :percent="primaryDisk?.percent ?? 0"
              :loading="!server"
              :detail="
                primaryDisk
                  ? `${primaryDisk.mount} · ${formatBytes(primaryDisk.used)} / ${formatBytes(primaryDisk.total)}`
                  : '无可用磁盘'
              "
            />
          </el-col>
          <el-col :span="24">
            <el-card shadow="never">
              <template #header>
                <div class="flex items-center justify-between">
                  <span class="font-medium">CPU / 内存趋势</span>
                  <span class="text-xs text-(--el-text-color-secondary)">
                    仅在本页打开期间采集，最多 10 分钟
                  </span>
                </div>
              </template>
              <HistoryChart :points="history" />
            </el-card>
          </el-col>
        </el-row>
      </el-col>
    </el-row>

    <el-row :gutter="12">
      <el-col :xs="24" :lg="10" class="mb-3">
        <el-card shadow="never" class="h-full">
          <template #header>
            <div class="flex items-center justify-between">
              <span class="font-medium">CPU 核心</span>
              <span
                v-if="server"
                class="text-xs text-(--el-text-color-secondary)"
              >
                交换分区 {{ formatBytes(server.swap.used) }} /
                {{ formatBytes(server.swap.total) }}
              </span>
            </div>
          </template>
          <div v-if="server?.cpu.perCore.length" class="core-grid">
            <div v-for="(p, i) in server.cpu.perCore" :key="i" class="core">
              <span class="core-label">#{{ i }}</span>
              <el-progress
                :percentage="Math.min(100, Math.round(p))"
                :color="percentColor(p)"
                :stroke-width="8"
                class="flex-1"
              />
            </div>
          </div>
          <el-empty v-else :image-size="60" description="暂无数据" />
        </el-card>
      </el-col>
      <el-col :xs="24" :lg="14" class="mb-3">
        <el-card shadow="never" class="h-full">
          <template #header>
            <span class="font-medium">磁盘</span>
          </template>
          <el-table :data="server?.disks ?? []" size="small" max-height="300">
            <template #empty>
              <el-empty :image-size="60" description="暂无磁盘数据" />
            </template>
            <el-table-column
              prop="mount"
              label="挂载点"
              min-width="120"
              show-overflow-tooltip
            />
            <el-table-column
              prop="fsType"
              label="文件系统"
              width="90"
              show-overflow-tooltip
            />
            <el-table-column label="已用 / 总量" min-width="140">
              <template #default="{ row }">
                {{ formatBytes(row.used) }} / {{ formatBytes(row.total) }}
              </template>
            </el-table-column>
            <el-table-column label="使用率" min-width="150">
              <template #default="{ row }">
                <el-progress
                  :percentage="Math.min(100, Math.round(row.percent))"
                  :color="percentColor(row.percent)"
                  :stroke-width="8"
                />
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <div class="mb-2 font-medium">应用</div>
    <AppPanel :stats="app" :loading="!app && loading" />
  </div>
</template>

<style scoped>
.auto-refresh-switch {
  --el-switch-off-color: var(--el-color-info);
}

.uptime {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 10px 12px;
  background: var(--el-fill-color-light);
  border-radius: 8px;
}

.kv-list {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 8px 16px;
  margin: 0;
  font-size: 13px;
}

.kv-list dt {
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}

.kv-list dd {
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.core-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 8px 16px;
}

.core {
  display: flex;
  gap: 8px;
  align-items: center;
}

.core-label {
  width: 28px;
  font-family: var(--el-font-family-mono, ui-monospace, monospace);
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
