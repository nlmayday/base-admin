<script setup lang="ts">
import { useClipboard } from "@vueuse/core";
import { formatDateTime } from "@/utils/format";
import { message } from "@/utils/message";
import type { JobRun } from "@/api/ops/run";
import RunStatusTag from "../../shared/RunStatusTag.vue";
import { formatDuration } from "../../shared/format";
import { TRIGGER_META } from "../../shared/status";

defineOptions({ name: "RunDetail" });

const props = defineProps<{ run: JobRun }>();

const { copy, isSupported } = useClipboard({ legacy: true });

async function copyError(): Promise<void> {
  await copy(props.run.error);
  message("已复制错误信息", { type: "success" });
}
</script>

<template>
  <div class="run-detail">
    <el-descriptions :column="3" size="small" border>
      <el-descriptions-item label="记录 ID">{{ run.id }}</el-descriptions-item>
      <el-descriptions-item label="状态">
        <RunStatusTag :status="run.status" />
      </el-descriptions-item>
      <el-descriptions-item label="触发方式">
        {{ TRIGGER_META[run.trigger]?.label ?? run.trigger }}
        <span v-if="run.triggeredBy" class="text-(--el-text-color-secondary)">
          （用户 {{ run.triggeredBy }}）
        </span>
      </el-descriptions-item>
      <el-descriptions-item label="计划时间">
        {{ formatDateTime(run.scheduledAt) }}
      </el-descriptions-item>
      <el-descriptions-item label="开始时间">
        {{ formatDateTime(run.startedAt) }}
      </el-descriptions-item>
      <el-descriptions-item label="结束时间">
        {{ formatDateTime(run.finishedAt) }}
      </el-descriptions-item>
      <el-descriptions-item label="耗时">
        {{ formatDuration(run.durationMs) }}
      </el-descriptions-item>
      <el-descriptions-item label="尝试次数">
        {{ run.attempt }}
      </el-descriptions-item>
      <el-descriptions-item label="执行实例">
        <span class="font-mono text-xs">{{ run.instance || "-" }}</span>
      </el-descriptions-item>
    </el-descriptions>
    <div v-if="run.error" class="mt-3">
      <div class="mb-1 flex items-center justify-between">
        <span class="text-sm font-medium">错误信息</span>
        <el-button v-if="isSupported" link type="primary" @click="copyError">
          复制
        </el-button>
      </div>
      <pre class="run-error">{{ run.error }}</pre>
    </div>
  </div>
</template>

<style scoped>
.run-detail {
  padding: 8px 16px 12px 48px;
}

.run-error {
  max-height: 240px;
  padding: 10px 12px;
  margin: 0;
  overflow: auto;
  font-family: var(--el-font-family-mono, ui-monospace, monospace);
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-color-danger);
  word-break: break-all;
  white-space: pre-wrap;
  background: var(--el-color-danger-light-9);
  border-radius: 6px;
}
</style>
