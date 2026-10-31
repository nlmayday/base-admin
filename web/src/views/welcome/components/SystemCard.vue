<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { formatBytes, formatUptime } from "@/utils/format";
import {
  getAppStats,
  getServerStats,
  type AppStats,
  type ServerStats
} from "@/api/ops/monitor";

defineOptions({ name: "SystemCard" });

const loading = ref(false);
const server = ref<ServerStats | null>(null);
const app = ref<AppStats | null>(null);

type Gauge = { label: string; percent: number; detail: string };

const gauges = computed<Gauge[]>(() => {
  const s = server.value;
  if (!s) return [];
  // 卷可能共享同一物理磁盘，求和会重复计算；取根分区，没有时取最大的卷
  const disk = s.disks.find(d => d.mount === "/") ??
    [...s.disks].sort((a, b) => b.total - a.total)[0] ?? { used: 0, total: 0 };
  return [
    {
      label: "CPU",
      percent: s.cpu.percent,
      detail: `${s.cpu.logicalCores} 核`
    },
    {
      label: "内存",
      percent: s.memory.percent,
      detail: `${formatBytes(s.memory.used)} / ${formatBytes(s.memory.total)}`
    },
    {
      label: "磁盘",
      percent: disk.total ? (disk.used / disk.total) * 100 : 0,
      detail: `${formatBytes(disk.used)} / ${formatBytes(disk.total)}`
    }
  ];
});

function colorOf(percent: number): string {
  if (percent >= 90) return "var(--el-color-danger)";
  if (percent >= 70) return "var(--el-color-warning)";
  return "var(--el-color-success)";
}

async function load(): Promise<void> {
  loading.value = true;
  try {
    const [s, a] = await Promise.allSettled([getServerStats(), getAppStats()]);
    server.value = s.status === "fulfilled" ? s.value : null;
    app.value = a.status === "fulfilled" ? a.value : null;
  } finally {
    loading.value = false;
  }
}

onMounted(load);
defineExpose({ reload: load });
</script>

<template>
  <el-card v-loading="loading" shadow="never" class="h-full">
    <template #header>
      <div class="flex items-center justify-between">
        <span class="font-medium">系统状态</span>
        <router-link
          to="/ops/monitor"
          class="text-sm text-[var(--el-color-primary)]"
        >
          详情
        </router-link>
      </div>
    </template>
    <div v-if="gauges.length" class="grid grid-cols-3 gap-2 text-center">
      <div v-for="g in gauges" :key="g.label">
        <el-progress
          type="dashboard"
          :percentage="Math.round(g.percent)"
          :color="colorOf(g.percent)"
          :width="96"
          :stroke-width="8"
        />
        <div class="font-medium">{{ g.label }}</div>
        <div class="truncate text-xs text-[var(--el-text-color-secondary)]">
          {{ g.detail }}
        </div>
      </div>
    </div>
    <el-empty
      v-else-if="!loading"
      description="暂无监控数据"
      :image-size="60"
    />
    <el-descriptions v-if="app" :column="2" size="small" class="mt-4">
      <el-descriptions-item label="版本">
        {{ app.process.version || "-" }}
      </el-descriptions-item>
      <el-descriptions-item label="已运行">
        {{ formatUptime(app.process.uptimeSeconds) }}
      </el-descriptions-item>
      <el-descriptions-item label="数据库连接">
        {{ app.database.inUse }} / {{ app.database.openConnections }}
      </el-descriptions-item>
      <el-descriptions-item label="Redis">
        <el-tag
          size="small"
          :type="app.redis.available ? 'success' : 'danger'"
          effect="plain"
        >
          {{ app.redis.available ? "正常" : "不可用" }}
        </el-tag>
      </el-descriptions-item>
    </el-descriptions>
  </el-card>
</template>
