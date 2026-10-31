<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue";
import { useMutationObserver, useResizeObserver } from "@vueuse/core";
import type { EChartsCoreOption, EChartsType } from "echarts/core";
import echarts from "@/plugins/echarts";

defineOptions({ name: "FileEChart" });

const props = defineProps<{
  /** 根据当前主题生成配置，切换暗色模式时会重新调用 */
  option: (theme: ChartTheme) => EChartsCoreOption;
  height?: string;
}>();

export type ChartTheme = {
  text: string;
  subtext: string;
  border: string;
  background: string;
};

const el = ref<HTMLDivElement>();
const chart = shallowRef<EChartsType>();

function readTheme(): ChartTheme {
  const css = getComputedStyle(document.documentElement);
  const v = (name: string, fallback: string) =>
    css.getPropertyValue(name).trim() || fallback;
  return {
    text: v("--el-text-color-regular", "#606266"),
    subtext: v("--el-text-color-secondary", "#909399"),
    border: v("--el-border-color-lighter", "#ebeef5"),
    background: v("--el-bg-color", "#ffffff")
  };
}

function render(): void {
  chart.value?.setOption(props.option(readTheme()), true);
}

onMounted(() => {
  if (!el.value) return;
  chart.value = echarts.init(el.value);
  render();
});

watch(() => props.option, render);
useResizeObserver(el, () => chart.value?.resize());
// 主题切换时 <html> 的 class 会变化
useMutationObserver(
  document.documentElement,
  () => requestAnimationFrame(render),
  { attributes: true, attributeFilter: ["class"] }
);

onBeforeUnmount(() => chart.value?.dispose());
</script>

<template>
  <div ref="el" :style="{ height: height ?? '280px' }" class="w-full" />
</template>
