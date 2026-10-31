<script setup lang="ts">
import { ref, watch } from "vue";
import { useChart } from "../useChart";
import type { HistoryPoint } from "../types";

defineOptions({ name: "HistoryChart" });

const props = defineProps<{ points: HistoryPoint[] }>();

const el = ref<HTMLElement>();

const timeLabel = (ms: number) => {
  const d = new Date(ms);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};

const { render } = useChart(el, dark => {
  const axisColor = dark ? "#4c4d4f" : "#dcdfe6";
  const labelColor = dark ? "#a3a6ad" : "#909399";
  const series = (name: string, key: "cpu" | "memory", color: string) => ({
    name,
    type: "line",
    smooth: true,
    showSymbol: false,
    lineStyle: { width: 2, color },
    itemStyle: { color },
    areaStyle: { color, opacity: 0.08 },
    data: props.points.map(p => [p.time, p[key]])
  });
  return {
    grid: { left: 44, right: 16, top: 36, bottom: 28 },
    legend: { top: 0, right: 0, textStyle: { color: labelColor } },
    tooltip: {
      trigger: "axis",
      valueFormatter: (v: number) => `${Number(v).toFixed(1)}%`
    },
    xAxis: {
      type: "time",
      axisLine: { lineStyle: { color: axisColor } },
      axisLabel: { color: labelColor, formatter: timeLabel, hideOverlap: true },
      splitLine: { show: false }
    },
    yAxis: {
      type: "value",
      min: 0,
      max: 100,
      axisLabel: { color: labelColor, formatter: "{value}%" },
      splitLine: { lineStyle: { color: axisColor, type: "dashed" } }
    },
    series: [
      series("CPU", "cpu", "#409eff"),
      series("内存", "memory", "#67c23a")
    ]
  };
});

watch(() => props.points, render, { deep: false });
</script>

<template>
  <div class="relative">
    <div ref="el" class="h-[260px] w-full" />
    <div
      v-if="points.length < 2"
      class="absolute inset-0 flex items-center justify-center text-sm text-(--el-text-color-placeholder)"
    >
      正在采集数据，页面打开期间每次刷新记录一个点
    </div>
  </div>
</template>
