<script setup lang="ts">
import { computed } from "vue";
import { formatDateTime } from "@/utils/format";
import { otherZoneText } from "@/utils/timezone";

defineOptions({ name: "ReDateTime" });

/**
 * 列表中的时间：按顶栏时区显示，悬停显示另一个时区（本地 / UTC）的时间。
 *   <ReDateTime :value="row.createdAt" />
 */
const props = defineProps<{ value: string | null | undefined }>();

const other = computed(() => otherZoneText(props.value));
</script>

<template>
  <el-tooltip :disabled="!other" :content="other" placement="top">
    <span>{{ formatDateTime(props.value) }}</span>
  </el-tooltip>
</template>
