<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { useMutationObserver, useResizeObserver } from "@vueuse/core";
import type { ECharts, EChartsCoreOption } from "echarts/core";
import echarts from "@/plugins/echarts";
import { getAuditStats, type AuditStats } from "@/api/audit/stats";

defineOptions({ name: "AuditStatsCard" });

const props = defineProps<{ kind: "login" | "operation" }>();

type Tile = { label: string; value: number; tone: string; hint?: string };

const loading = ref(false);
const stats = ref<AuditStats | null>(null);
const chartEl = ref<HTMLDivElement | null>(null);
let chart: ECharts | null = null;

const tiles = computed<Tile[]>(() => {
  const t = stats.value?.today;
  const trend = stats.value?.trend ?? [];
  if (props.kind === "login") {
    const success = t?.loginSuccess ?? 0;
    const failure = t?.loginFailure ?? 0;
    const total = success + failure;
    const weekFailures = trend.reduce((n, p) => n + p.loginFailure, 0);
    return [
      { label: "今日登录成功", value: success, tone: "success" },
      { label: "今日登录失败", value: failure, tone: "danger" },
      {
        label: "今日失败率",
        value: total === 0 ? 0 : Math.round((failure / total) * 100),
        tone: "warning",
        hint: "%"
      },
      { label: "近7天失败次数", value: weekFailures, tone: "info" }
    ];
  }
  const ops = t?.operations ?? 0;
  const failures = t?.operationFailures ?? 0;
  const weekOps = trend.reduce((n, p) => n + p.operations, 0);
  return [
    { label: "今日操作", value: ops, tone: "primary" },
    { label: "今日失败操作", value: failures, tone: "danger" },
    {
      label: "今日成功率",
      value: ops === 0 ? 100 : Math.round(((ops - failures) / ops) * 100),
      tone: "success",
      hint: "%"
    },
    { label: "近7天操作", value: weekOps, tone: "info" }
  ];
});

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
}

function chartOption(): EChartsCoreOption {
  const trend = stats.value?.trend ?? [];
  const dates = trend.map(p => p.date.slice(5));
  const textColor = cssVar("--el-text-color-secondary");
  const lineColor = cssVar("--el-border-color-lighter");
  const series =
    props.kind === "login"
      ? [
          {
            name: "成功",
            data: trend.map(p => p.loginSuccess),
            color: cssVar("--el-color-success")
          },
          {
            name: "失败",
            data: trend.map(p => p.loginFailure),
            color: cssVar("--el-color-danger")
          }
        ]
      : [
          {
            name: "操作",
            data: trend.map(p => p.operations),
            color: cssVar("--el-color-primary")
          },
          {
            name: "失败",
            data: trend.map(p => p.operationFailures),
            color: cssVar("--el-color-danger")
          }
        ];
  return {
    backgroundColor: "transparent",
    tooltip: { trigger: "axis" },
    legend: { right: 0, top: 0, textStyle: { color: textColor } },
    grid: { left: 8, right: 8, top: 32, bottom: 0, containLabel: true },
    xAxis: {
      type: "category",
      boundaryGap: false,
      data: dates,
      axisLabel: { color: textColor },
      axisLine: { lineStyle: { color: lineColor } }
    },
    yAxis: {
      type: "value",
      minInterval: 1,
      axisLabel: { color: textColor },
      splitLine: { lineStyle: { color: lineColor } }
    },
    series: series.map(s => ({
      name: s.name,
      type: "line",
      smooth: true,
      symbolSize: 6,
      data: s.data,
      itemStyle: { color: s.color },
      areaStyle: { opacity: 0.08 }
    }))
  };
}

function render(): void {
  if (!chartEl.value) return;
  chart ??= echarts.init(chartEl.value);
  chart.setOption(chartOption(), true);
}

async function load(): Promise<void> {
  loading.value = true;
  try {
    stats.value = await getAuditStats();
  } catch {
    stats.value = null; // 错误提示已由 http 拦截器处理
  } finally {
    loading.value = false;
  }
  await nextTick();
  render();
}

useResizeObserver(chartEl, () => chart?.resize());
// 主题切换（html 上的 dark 类）后用新的 CSS 变量重绘
useMutationObserver(document.documentElement, () => nextTick(render), {
  attributes: true,
  attributeFilter: ["class"]
});
onMounted(load);
onBeforeUnmount(() => {
  chart?.dispose();
  chart = null;
});

defineExpose({ reload: load });
</script>

<template>
  <el-card v-loading="loading" shadow="never" class="mb-3">
    <div class="flex flex-wrap gap-4">
      <div class="grid min-w-[280px] flex-1 grid-cols-2 gap-3">
        <div
          v-for="tile in tiles"
          :key="tile.label"
          class="audit-tile"
          :class="`audit-tile--${tile.tone}`"
        >
          <div class="text-xs text-[var(--el-text-color-secondary)]">
            {{ tile.label }}
          </div>
          <div class="mt-1 text-2xl font-semibold tabular-nums">
            {{ tile.value
            }}<span v-if="tile.hint" class="ml-0.5 text-sm font-normal">{{
              tile.hint
            }}</span>
          </div>
        </div>
      </div>
      <div class="min-w-[320px] flex-[1.6]">
        <div class="mb-1 text-sm font-medium">近7天趋势</div>
        <div ref="chartEl" class="h-[150px] w-full" />
      </div>
    </div>
  </el-card>
</template>

<style scoped>
.audit-tile {
  padding: 12px 14px;
  background: var(--el-fill-color-light);
  border-left: 3px solid var(--tile-color);
  border-radius: 6px;
}

.audit-tile--success {
  --tile-color: var(--el-color-success);
}

.audit-tile--danger {
  --tile-color: var(--el-color-danger);
}

.audit-tile--warning {
  --tile-color: var(--el-color-warning);
}

.audit-tile--primary {
  --tile-color: var(--el-color-primary);
}

.audit-tile--info {
  --tile-color: var(--el-color-info);
}
</style>
