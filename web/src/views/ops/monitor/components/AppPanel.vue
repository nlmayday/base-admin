<script setup lang="ts">
import { computed } from "vue";
import { formatDateTime } from "@/utils/format";
import type { AppStats } from "@/api/ops/monitor";
import { formatBytes, formatUptime } from "../../shared/format";
import CodeIcon from "~icons/ri/code-box-line";
import DatabaseIcon from "~icons/ri/database-2-line";
import StackIcon from "~icons/ri/stack-line";

defineOptions({ name: "AppPanel" });

const props = defineProps<{ stats: AppStats | null; loading?: boolean }>();

type Item = { label: string; value: string | number };

const runtimeItems = computed<Item[]>(() => {
  const r = props.stats?.runtime;
  if (!r) return [];
  return [
    { label: "Go 版本", value: r.goVersion },
    { label: "平台", value: `${r.goos}/${r.goarch}` },
    { label: "Goroutines", value: r.goroutines },
    { label: "GOMAXPROCS", value: `${r.gomaxprocs} / ${r.numCpu} 核` },
    {
      label: "堆内存",
      value: `${formatBytes(r.heapAlloc)} / ${formatBytes(r.heapSys)}`
    },
    { label: "栈内存", value: formatBytes(r.stackInuse) },
    { label: "向系统申请", value: formatBytes(r.sys) },
    { label: "下次 GC 阈值", value: formatBytes(r.nextGc) },
    { label: "GC 次数", value: r.numGc },
    { label: "最近一次暂停", value: `${r.lastPauseMs.toFixed(3)} ms` },
    { label: "累计暂停", value: `${r.pauseTotalMs.toFixed(1)} ms` },
    { label: "最近 GC", value: formatDateTime(r.lastGcAt) }
  ];
});

const dbItems = computed<Item[]>(() => {
  const d = props.stats?.database;
  if (!d) return [];
  return [
    { label: "最大连接数", value: d.maxOpenConnections || "不限" },
    { label: "当前连接", value: d.openConnections },
    { label: "使用中", value: d.inUse },
    { label: "空闲", value: d.idle },
    { label: "等待次数", value: d.waitCount },
    { label: "累计等待", value: `${d.waitDurationMs} ms` },
    { label: "因空闲数关闭", value: d.maxIdleClosed },
    { label: "因空闲超时关闭", value: d.maxIdleTimeClosed },
    { label: "因生命周期关闭", value: d.maxLifetimeClosed }
  ];
});

const redisItems = computed<Item[]>(() => {
  const r = props.stats?.redis;
  if (!r?.available) return [];
  return [
    { label: "版本", value: r.version },
    { label: "模式", value: r.mode || "-" },
    { label: "已用内存", value: r.usedMemoryHuman },
    { label: "内存峰值", value: r.usedMemoryPeakHuman },
    {
      label: "内存上限",
      value: r.maxMemoryHuman === "0B" ? "不限" : r.maxMemoryHuman || "-"
    },
    { label: "客户端连接", value: r.connectedClients },
    { label: "运行时长", value: formatUptime(r.uptimeSeconds) }
  ];
});

const poolUsage = computed(() => {
  const d = props.stats?.database;
  if (!d || !d.maxOpenConnections) return 0;
  return Math.round((d.inUse / d.maxOpenConnections) * 100);
});
</script>

<template>
  <el-row v-loading="loading" :gutter="12">
    <el-col :xs="24" :md="12" :xl="8" class="mb-3">
      <el-card shadow="never" class="h-full">
        <template #header>
          <div class="flex items-center gap-2">
            <CodeIcon class="text-(--el-color-primary)" />
            <span class="font-medium">Go 运行时</span>
          </div>
        </template>
        <dl v-if="runtimeItems.length" class="kv-grid">
          <template v-for="item in runtimeItems" :key="item.label">
            <dt>{{ item.label }}</dt>
            <dd>{{ item.value }}</dd>
          </template>
        </dl>
        <el-empty v-else :image-size="60" description="暂无数据" />
      </el-card>
    </el-col>
    <el-col :xs="24" :md="12" :xl="8" class="mb-3">
      <el-card shadow="never" class="h-full">
        <template #header>
          <div class="flex items-center gap-2">
            <DatabaseIcon class="text-(--el-color-primary)" />
            <span class="font-medium">数据库连接池</span>
          </div>
        </template>
        <template v-if="dbItems.length">
          <div class="mb-3">
            <div
              class="mb-1 flex justify-between text-xs text-(--el-text-color-secondary)"
            >
              <span>连接使用率</span>
              <span>{{ poolUsage }}%</span>
            </div>
            <el-progress
              :percentage="poolUsage"
              :show-text="false"
              :stroke-width="8"
            />
          </div>
          <dl class="kv-grid">
            <template v-for="item in dbItems" :key="item.label">
              <dt>{{ item.label }}</dt>
              <dd>{{ item.value }}</dd>
            </template>
          </dl>
        </template>
        <el-empty v-else :image-size="60" description="暂无数据" />
      </el-card>
    </el-col>
    <el-col :xs="24" :md="24" :xl="8" class="mb-3">
      <el-card shadow="never" class="h-full">
        <template #header>
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <StackIcon class="text-(--el-color-danger)" />
              <span class="font-medium">Redis</span>
            </div>
            <el-tag
              v-if="stats"
              :type="stats.redis.available ? 'success' : 'danger'"
              size="small"
              effect="plain"
            >
              {{ stats.redis.available ? "正常" : "不可用" }}
            </el-tag>
          </div>
        </template>
        <dl v-if="redisItems.length" class="kv-grid">
          <template v-for="item in redisItems" :key="item.label">
            <dt>{{ item.label }}</dt>
            <dd>{{ item.value }}</dd>
          </template>
        </dl>
        <el-empty v-else :image-size="60" description="无法获取 Redis 信息" />
      </el-card>
    </el-col>
  </el-row>
</template>

<style scoped>
.kv-grid {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 8px 16px;
  margin: 0;
  font-size: 13px;
}

.kv-grid dt {
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}

.kv-grid dd {
  margin: 0;
  font-variant-numeric: tabular-nums;
  color: var(--el-text-color-primary);
  text-align: right;
  word-break: break-all;
}
</style>
