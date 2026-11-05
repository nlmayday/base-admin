<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { EChartsCoreOption } from "echarts/core";
import { getStorageStats, type FileKind, type StorageStats } from "@/api/file";
import {
  KIND_COLORS,
  KIND_LABELS,
  extVisual,
  formatSize
} from "../files/utils";
import EChart, { type ChartTheme } from "./components/EChart.vue";
import FileIcon from "~icons/ri/file-list-3-line";
import DatabaseIcon from "~icons/ri/database-2-line";
import FolderIcon from "~icons/ri/folder-3-line";
import UploadIcon from "~icons/ri/upload-cloud-2-line";
import DriveIcon from "~icons/ri/hard-drive-2-line";
import CloudIcon from "~icons/ri/cloud-line";
import ShieldIcon from "~icons/ri/shield-check-line";
import RefreshIcon from "~icons/ri/refresh-line";

defineOptions({ name: "FileStorage" });

const stats = ref<StorageStats | null>(null);
const loading = ref(false);
const pieMetric = ref<"count" | "size">("size");

async function load(): Promise<void> {
  loading.value = true;
  try {
    stats.value = await getStorageStats();
  } catch {
    // 错误提示已由 http 拦截器统一处理
  } finally {
    loading.value = false;
  }
}

const isS3 = computed(() => stats.value?.driver === "s3");

const tiles = computed(() => {
  const s = stats.value;
  return [
    {
      label: "文件总数",
      value: s ? s.fileCount.toLocaleString() : "-",
      icon: FileIcon,
      color: "#3b82f6"
    },
    {
      label: "占用空间",
      value: s ? formatSize(s.totalSize) : "-",
      icon: DatabaseIcon,
      color: "#10b981"
    },
    {
      label: "文件分类",
      value: s ? s.categoryCount.toLocaleString() : "-",
      icon: FolderIcon,
      color: "#f59e0b"
    },
    {
      label: "单文件上限",
      value: s ? formatSize(s.policy?.maxBytes ?? s.maxUploadBytes) : "-",
      icon: UploadIcon,
      color: "#8b5cf6"
    }
  ];
});

function formatTtl(seconds: number): string {
  if (seconds % 86400 === 0) return `${seconds / 86400} 天`;
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`;
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`;
  return `${seconds} 秒`;
}

const hasFiles = computed(() => (stats.value?.fileCount ?? 0) > 0);

const pieOption = computed(() => {
  const s = stats.value;
  const metric = pieMetric.value;
  return (t: ChartTheme): EChartsCoreOption => ({
    tooltip: {
      trigger: "item",
      formatter: (p: {
        name: string;
        data: { count: number; size: number };
        percent: number;
      }) =>
        `${p.name}<br/>${p.data.count} 个 · ${formatSize(p.data.size)}（${p.percent}%）`
    },
    legend: { bottom: 0, icon: "circle", textStyle: { color: t.text } },
    series: [
      {
        type: "pie",
        radius: ["48%", "72%"],
        center: ["50%", "44%"],
        itemStyle: {
          borderColor: t.background,
          borderWidth: 2,
          borderRadius: 4
        },
        label: {
          show: true,
          position: "center",
          formatter: () =>
            metric === "size"
              ? `{v|${formatSize(s?.totalSize ?? 0)}}\n{l|总占用}`
              : `{v|${s?.fileCount ?? 0}}\n{l|文件数}`,
          rich: {
            v: { fontSize: 20, fontWeight: 600, color: t.text, lineHeight: 28 },
            l: { fontSize: 12, color: t.subtext }
          }
        },
        emphasis: { scale: true, scaleSize: 4, label: { show: true } },
        labelLine: { show: false },
        data: (s?.byKind ?? []).map(k => ({
          name: KIND_LABELS[k.key as FileKind] ?? k.key,
          value: metric === "size" ? k.size : k.count,
          count: k.count,
          size: k.size,
          itemStyle: { color: KIND_COLORS[k.key as FileKind] ?? "#94a3b8" }
        }))
      }
    ]
  });
});

const trendOption = computed(() => {
  const daily = stats.value?.daily ?? [];
  return (t: ChartTheme): EChartsCoreOption => ({
    grid: { left: 8, right: 16, top: 24, bottom: 8, containLabel: true },
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "shadow" },
      formatter: (items: Array<{ dataIndex: number }>) => {
        const d = daily[items[0]?.dataIndex ?? 0];
        return d
          ? `${d.key}<br/>上传 ${d.count} 个 · ${formatSize(d.size)}`
          : "";
      }
    },
    xAxis: {
      type: "category",
      data: daily.map(d => d.key.slice(5)),
      axisLine: { lineStyle: { color: t.border } },
      axisTick: { show: false },
      axisLabel: { color: t.subtext }
    },
    yAxis: {
      type: "value",
      minInterval: 1,
      splitLine: { lineStyle: { color: t.border, type: "dashed" } },
      axisLabel: { color: t.subtext }
    },
    series: [
      {
        type: "bar",
        barMaxWidth: 22,
        itemStyle: { color: "#3b82f6", borderRadius: [4, 4, 0, 0] },
        data: daily.map(d => d.count)
      }
    ]
  });
});

const topExtensions = computed(() => {
  const list = stats.value?.topExtensions ?? [];
  const max = Math.max(1, ...list.map(e => e.count));
  return list.map(e => ({ ...e, percent: Math.round((e.count / max) * 100) }));
});

onMounted(load);
</script>

<template>
  <div v-loading="loading" class="main">
    <div class="mb-3 grid grid-cols-2 gap-3 xl:grid-cols-4">
      <el-card
        v-for="tile in tiles"
        :key="tile.label"
        shadow="never"
        body-class="p-4!"
      >
        <div class="flex items-center gap-4">
          <div
            class="stat-icon"
            :style="{ color: tile.color, backgroundColor: `${tile.color}1a` }"
          >
            <component :is="tile.icon" />
          </div>
          <div class="min-w-0">
            <div class="text-sm text-[var(--el-text-color-secondary)]">
              {{ tile.label }}
            </div>
            <div class="mt-1 truncate text-2xl font-semibold">
              {{ tile.value }}
            </div>
          </div>
        </div>
      </el-card>
    </div>

    <div class="mb-3 grid grid-cols-1 gap-3 lg:grid-cols-5">
      <el-card shadow="never" class="lg:col-span-2">
        <template #header>
          <div class="flex items-center justify-between">
            <span class="card-title">存储驱动</span>
            <el-button link type="primary" :loading="loading" @click="load">
              <RefreshIcon class="mr-1" />刷新
            </el-button>
          </div>
        </template>
        <template v-if="stats">
          <div class="driver-banner">
            <component
              :is="isS3 ? CloudIcon : DriveIcon"
              class="h-9 w-9 shrink-0"
            />
            <div class="min-w-0">
              <div class="font-medium">
                {{ isS3 ? "S3 兼容对象存储" : "本地磁盘" }}
              </div>
              <div class="mt-0.5 text-xs text-[var(--el-text-color-secondary)]">
                {{
                  isS3
                    ? "适用于 AWS S3、MinIO、阿里云 OSS、腾讯云 COS 等"
                    : "文件保存在服务器本地目录，适合单机部署"
                }}
              </div>
            </div>
            <el-tag class="ml-auto" effect="dark" round disable-transitions>
              {{ stats.driver }}
            </el-tag>
          </div>
          <el-descriptions :column="1" border size="small" class="mt-3">
            <template v-if="isS3">
              <el-descriptions-item label="Endpoint">
                <span class="break-all">{{ stats.location.endpoint }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="Bucket">
                {{ stats.location.bucket }}
              </el-descriptions-item>
              <el-descriptions-item label="Region">
                {{ stats.location.region || "自动" }}
              </el-descriptions-item>
              <el-descriptions-item label="传输加密">
                <el-tag
                  :type="stats.location.useSSL ? 'success' : 'warning'"
                  size="small"
                  disable-transitions
                >
                  {{ stats.location.useSSL ? "HTTPS" : "HTTP（未加密）" }}
                </el-tag>
              </el-descriptions-item>
              <el-descriptions-item label="寻址方式">
                {{ stats.location.usePathStyle ? "Path-style" : "自动" }}
              </el-descriptions-item>
            </template>
            <el-descriptions-item v-else label="存储目录">
              <span class="break-all font-mono text-xs">{{
                stats.location.dir
              }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="签名链接有效期">
              {{ formatTtl(stats.signedUrlTtlSeconds) }}
            </el-descriptions-item>
            <el-descriptions-item label="上传硬上限">
              {{ formatSize(stats.maxUploadBytes) }}
            </el-descriptions-item>
          </el-descriptions>
          <div class="secret-note">
            <ShieldIcon class="h-4 w-4 shrink-0" />
            访问密钥与签名密钥只从环境变量读取，不会在页面中展示。
          </div>
        </template>
        <el-empty
          v-else-if="!loading"
          description="暂无数据"
          :image-size="80"
        />
      </el-card>

      <el-card shadow="never" class="lg:col-span-3">
        <template #header>
          <div class="flex items-center justify-between">
            <span class="card-title">类型分布</span>
            <el-radio-group v-model="pieMetric" size="small">
              <el-radio-button value="size">按空间</el-radio-button>
              <el-radio-button value="count">按数量</el-radio-button>
            </el-radio-group>
          </div>
        </template>
        <EChart v-if="hasFiles" :option="pieOption" height="300px" />
        <el-empty v-else description="还没有上传任何文件" :image-size="100" />
      </el-card>
    </div>

    <div class="mb-3 grid grid-cols-1 gap-3 lg:grid-cols-5">
      <el-card shadow="never" class="lg:col-span-3">
        <template #header>
          <span class="card-title">近 14 天上传</span>
        </template>
        <EChart v-if="stats" :option="trendOption" height="260px" />
      </el-card>
      <el-card shadow="never" class="lg:col-span-2">
        <template #header>
          <span class="card-title">常见扩展名</span>
        </template>
        <ul v-if="topExtensions.length" class="ext-list">
          <li v-for="e in topExtensions" :key="e.key">
            <component
              :is="extVisual(e.key).icon"
              class="h-5 w-5 shrink-0"
              :style="{ color: extVisual(e.key).color }"
            />
            <span class="w-12 shrink-0 font-mono text-xs uppercase">{{
              e.key
            }}</span>
            <el-progress
              class="flex-1"
              :percentage="e.percent"
              :stroke-width="8"
              :show-text="false"
              :color="extVisual(e.key).color"
            />
            <span
              class="w-28 shrink-0 text-right text-xs text-[var(--el-text-color-secondary)]"
            >
              {{ e.count }} 个 · {{ formatSize(e.size) }}
            </span>
          </li>
        </ul>
        <el-empty v-else description="暂无数据" :image-size="80" />
      </el-card>
    </div>

    <el-card shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="card-title">允许上传的类型</span>
          <span class="text-xs text-[var(--el-text-color-secondary)]">
            在「系统配置 ›
            基础设置」中调整；HTML、SVG、脚本与可执行文件始终禁止上传
          </span>
        </div>
      </template>
      <div v-if="stats?.policy" class="flex flex-wrap gap-2">
        <el-tag
          v-for="ext in stats.policy.extensions"
          :key="ext"
          type="info"
          effect="plain"
          disable-transitions
        >
          .{{ ext }}
        </el-tag>
      </div>
      <el-alert
        v-else-if="stats"
        type="warning"
        :closable="false"
        show-icon
        title="上传配置暂不可用，请稍后刷新"
      />
    </el-card>
  </div>
</template>

<style scoped lang="scss">
.stat-icon {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  font-size: 24px;
  border-radius: 12px;
}

.driver-banner {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 12px 14px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  border-radius: 8px;

  .font-medium {
    color: var(--el-text-color-primary);
  }
}

.secret-note {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-top: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.ext-list {
  padding: 0;
  margin: 0;
  list-style: none;

  li {
    display: flex;
    gap: 10px;
    align-items: center;
    padding: 7px 0;
  }
}
</style>
