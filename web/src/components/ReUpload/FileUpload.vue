<script setup lang="ts">
import { computed } from "vue";
import type { UploadRawFile } from "element-plus";
import type { AttachmentRef } from "@/api/file";
import { useAttachmentUpload } from "./useAttachmentUpload";

defineOptions({ name: "ReFileUpload" });

/**
 * 多附件上传：v-model 为附件数组，存入业务表的 jsonb 字段。
 * `<ReFileUpload v-model="form.attachments" :limit="5" />`
 */
const files = defineModel<AttachmentRef[]>({ default: () => [] });
const props = withDefaults(defineProps<{ limit?: number }>(), { limit: 10 });

const { uploading, percent, policy, upload, sizeText } =
  useAttachmentUpload(false);
const full = computed(() => files.value.length >= props.limit);
const accept = computed(() =>
  policy.value ? policy.value.extensions.map(e => `.${e}`).join(",") : ""
);

async function onSelect(file: UploadRawFile): Promise<false> {
  const done = await upload(file);
  if (done) {
    files.value = [
      ...files.value,
      { name: done.name, url: done.url, size: done.size }
    ];
  }
  return false;
}

function remove(index: number): void {
  files.value = files.value.filter((_, i) => i !== index);
}
</script>

<template>
  <div class="w-full">
    <el-upload
      :show-file-list="false"
      :accept="accept"
      :before-upload="onSelect"
      :disabled="uploading || full"
    >
      <el-button :loading="uploading" :disabled="full">
        {{ uploading ? `上传中 ${percent}%` : "上传附件" }}
      </el-button>
      <span class="ml-2 text-xs text-[var(--el-text-color-secondary)]">
        最多 {{ props.limit }} 个<template v-if="policy">
          ，单个不超过 {{ sizeText(policy.maxBytes) }}</template
        >
      </span>
    </el-upload>
    <ul v-if="files.length" class="mt-2">
      <li
        v-for="(f, i) in files"
        :key="f.url"
        class="flex items-center justify-between py-1 text-sm"
      >
        <el-link type="primary" :href="f.url" target="_blank" class="truncate">
          {{ f.name }}
        </el-link>
        <span class="flex items-center gap-3 shrink-0 ml-3">
          <span class="text-xs text-[var(--el-text-color-secondary)]">
            {{ sizeText(f.size) }}
          </span>
          <el-button link type="danger" @click="remove(i)">删除</el-button>
        </span>
      </li>
    </ul>
  </div>
</template>
