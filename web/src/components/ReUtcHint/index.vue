<script setup lang="ts">
import { computed } from "vue";
import { localZoneLabel, utcHint } from "@/utils/timezone";

/**
 * 日期时间选择器下方的 UTC 对照：选择器按浏览器时区取值，这里显示对应的
 * UTC 时间。浏览器本身就在 UTC 时不显示。
 *
 *   <el-date-picker v-model="range" type="datetimerange" ... />
 *   <ReUtcHint :value="range" />
 */
const props = defineProps<{
  value: string | Date | null | undefined | Array<string | Date>;
}>();

const text = computed(() => {
  const v = props.value;
  if (!v || localZoneLabel() === "GMT+0") return "";
  if (Array.isArray(v)) {
    const [from, to] = v.map(utcHint);
    return from && to ? `${from} 至 ${to}` : "";
  }
  return utcHint(v);
});
</script>

<template>
  <div v-if="text" class="re-utc-hint" data-testid="utc-hint">
    对应 {{ text }}
  </div>
</template>

<style scoped>
.re-utc-hint {
  width: 100%;
  font-size: 12px;
  line-height: 18px;
  color: var(--el-text-color-secondary);
}
</style>
