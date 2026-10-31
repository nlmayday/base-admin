<script setup lang="ts">
import { ref, watch } from "vue";
import { useChart } from "../useChart";
import { percentColor } from "../../shared/format";

defineOptions({ name: "GaugeCard" });

const props = defineProps<{
  title: string;
  /** 0–100 */
  percent: number;
  /** 仪表盘下方的说明，如 "3.2 GB / 16 GB" */
  detail?: string;
  loading?: boolean;
}>();

const el = ref<HTMLElement>();

const { render } = useChart(el, dark => {
  const value = Math.min(100, Math.max(0, props.percent ?? 0));
  const color = percentColor(value);
  return {
    series: [
      {
        type: "gauge",
        startAngle: 210,
        endAngle: -30,
        min: 0,
        max: 100,
        radius: "92%",
        center: ["50%", "58%"],
        progress: { show: true, width: 12, itemStyle: { color } },
        axisLine: {
          lineStyle: { width: 12, color: [[1, dark ? "#2b2f36" : "#eef0f4"]] }
        },
        pointer: { show: false },
        axisTick: { show: false },
        splitLine: { show: false },
        axisLabel: { show: false },
        anchor: { show: false },
        title: { show: false },
        detail: {
          valueAnimation: true,
          offsetCenter: [0, "4%"],
          fontSize: 24,
          fontWeight: 600,
          color: dark ? "#e5eaf3" : "#303133",
          formatter: (v: number) => `${v.toFixed(1)}%`
        },
        data: [{ value }]
      }
    ]
  };
});

watch(() => props.percent, render);
</script>

<template>
  <el-card shadow="never" class="h-full" body-class="py-3!">
    <div class="flex items-center justify-between">
      <span class="font-medium">{{ title }}</span>
      <slot name="extra" />
    </div>
    <div v-loading="loading" class="relative">
      <div ref="el" class="h-[170px] w-full" />
    </div>
    <div class="text-center text-xs text-(--el-text-color-secondary)">
      <slot>{{ detail || "-" }}</slot>
    </div>
  </el-card>
</template>
