<script setup lang="ts">
import { computed } from "vue";
import { useClipboard } from "@vueuse/core";
import { message } from "@/utils/message";
import { highlight, type Language } from "./highlight";

defineOptions({ name: "DevtoolsCodeBlock" });

const props = withDefaults(
  defineProps<{
    code: string;
    language: Language;
    title?: string;
    maxHeight?: string;
    lineNumbers?: boolean;
  }>(),
  { title: "", maxHeight: "480px", lineNumbers: true }
);

const lines = computed(() => highlight(props.code, props.language));
const gutterWidth = computed(
  () => `${String(lines.value.length).length + 1}ch`
);
const { copy, isSupported } = useClipboard({ legacy: true });

async function onCopy(): Promise<void> {
  try {
    await copy(props.code);
    message("已复制", { type: "success" });
  } catch {
    message("复制失败，请手动选择文本复制", { type: "error" });
  }
}
</script>

<template>
  <div class="code-block">
    <div class="code-block__bar">
      <span class="code-block__title">{{ title || language }}</span>
      <el-button v-if="isSupported" link type="primary" @click="onCopy">
        复制
      </el-button>
    </div>
    <el-scrollbar :max-height="maxHeight">
      <div class="code-block__body">
        <div v-for="(line, i) in lines" :key="i" class="code-block__line">
          <span
            v-if="lineNumbers"
            class="code-block__ln"
            :style="{ width: gutterWidth }"
            >{{ i + 1 }}</span
          ><span v-for="(t, j) in line" :key="j" :class="`tok-${t.kind}`">{{
            t.text
          }}</span>
        </div>
      </div>
    </el-scrollbar>
  </div>
</template>

<style scoped>
.code-block {
  overflow: hidden;
  background: var(--el-fill-color-lighter);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
}

.code-block__bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 12px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.code-block__title {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.code-block__body {
  padding: 8px 0;
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
  font-size: 12.5px;
  line-height: 1.6;
}

.code-block__line {
  min-height: 1.6em;
  padding-right: 12px;
  white-space: pre;
}

.code-block__ln {
  display: inline-block;
  padding: 0 12px 0 8px;
  margin-right: 8px;
  color: var(--el-text-color-placeholder);
  text-align: right;
  user-select: none;
  border-right: 1px solid var(--el-border-color-lighter);
}

.tok-comment {
  font-style: italic;
  color: var(--el-text-color-placeholder);
}

.tok-string {
  color: var(--el-color-success);
}

.tok-number {
  color: var(--el-color-warning);
}

.tok-keyword {
  color: var(--el-color-primary);
}

.tok-tag {
  color: var(--el-color-danger);
}

.tok-key {
  color: var(--el-color-primary-light-3);
}
</style>
