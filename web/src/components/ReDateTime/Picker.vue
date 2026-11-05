<script setup lang="ts">
import { computed } from "vue";
import {
  fromZoneWall,
  otherZoneText,
  toZoneWall,
  zoneLabel
} from "@/utils/timezone";

defineOptions({ name: "ReDateTimePicker" });

/**
 * 日期时间输入：按顶栏选择的时区（本地或 UTC）录入，旁边标时区，下方
 * 显示另一个时区的对应时间。v-model 为 UTC 的 RFC 3339 字符串，空为 ""。
 *   <ReDateTimePicker v-model="form.startAt" />
 */
const model = defineModel<string | null | undefined>({ default: "" });
withDefaults(defineProps<{ placeholder?: string; disabled?: boolean }>(), {
  placeholder: "选择时间",
  disabled: false
});

const wall = computed<string>({
  get: () => toZoneWall(model.value),
  set: v => {
    model.value = fromZoneWall(v);
  }
});
const other = computed(() => otherZoneText(model.value));
</script>

<template>
  <div class="w-full">
    <div class="flex items-center gap-2">
      <el-date-picker
        v-model="wall"
        type="datetime"
        value-format="YYYY-MM-DD HH:mm:ss"
        :placeholder="placeholder"
        :disabled="disabled"
        class="flex-1"
      />
      <span class="zone">{{ zoneLabel(model || undefined) }}</span>
    </div>
    <div v-if="other" class="zone" data-testid="other-zone">
      对应 {{ other }}
    </div>
  </div>
</template>

<style scoped>
.zone {
  font-size: 12px;
  line-height: 18px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
</style>
