<script setup lang="ts">
import { computed } from "vue";
import type { RunStatus } from "@/api/ops/job";
import { RUN_STATUS_META } from "./status";

defineOptions({ name: "RunStatusTag" });

const props = defineProps<{ status: RunStatus | "" }>();
const meta = computed(
  () => RUN_STATUS_META[props.status] ?? RUN_STATUS_META[""]
);
</script>

<template>
  <el-tag :type="meta.type" effect="light" disable-transitions>
    <span class="inline-flex items-center gap-1">
      <span
        v-if="status === 'running'"
        class="inline-block size-1.5 rounded-full bg-current animate-pulse"
      />
      {{ meta.label }}
    </span>
  </el-tag>
</template>
