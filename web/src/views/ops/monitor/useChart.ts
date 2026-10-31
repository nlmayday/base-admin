import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";
import { useResizeObserver } from "@vueuse/core";
import { useDark } from "@pureadmin/utils";
import * as echarts from "echarts/core";
import { GaugeChart, LineChart } from "echarts/charts";
import {
  GridComponent,
  LegendComponent,
  TooltipComponent
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { EChartsCoreOption } from "echarts/core";

echarts.use([
  GaugeChart,
  LineChart,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  CanvasRenderer
]);

/**
 * 在 el 上维护一个 ECharts 实例：随容器尺寸自适应，切换暗色主题时重建，
 * 组件卸载时销毁。build 根据当前主题返回完整配置。
 */
export function useChart(
  el: Ref<HTMLElement | undefined>,
  build: (dark: boolean) => EChartsCoreOption
): { render: () => void } {
  const { isDark } = useDark();
  let chart: echarts.ECharts | null = null;

  function render(): void {
    if (!el.value) return;
    if (!chart) {
      chart = echarts.init(el.value, isDark.value ? "dark" : undefined);
    }
    chart.setOption(
      { backgroundColor: "transparent", ...build(isDark.value) },
      { notMerge: false, lazyUpdate: true }
    );
  }

  function dispose(): void {
    chart?.dispose();
    chart = null;
  }

  useResizeObserver(el, () => chart?.resize());
  watch(isDark, () => {
    dispose();
    render();
  });
  onMounted(render);
  onBeforeUnmount(dispose);

  return { render };
}
