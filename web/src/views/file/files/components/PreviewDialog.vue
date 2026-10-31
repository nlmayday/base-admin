<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { FileItem } from "@/api/file";
import { formatDateTime } from "@/utils/format";
import {
  KIND_LABELS,
  KIND_TAG_TYPES,
  extVisual,
  formatSize,
  signedUrlFor
} from "../utils";
import DownloadIcon from "~icons/ri/download-2-line";
import LinkIcon from "~icons/ri/link";

defineOptions({ name: "FilePreviewDialog" });

const props = defineProps<{ file: FileItem | null }>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{
  (e: "download", file: FileItem): void;
  (e: "copy-link", file: FileItem): void;
}>();

const url = ref("");
const loading = ref(false);
const failed = ref(false);

const isImage = computed(
  () => props.file?.mimeType.startsWith("image/") ?? false
);
const isPdf = computed(() => props.file?.mimeType === "application/pdf");
const visual = computed(() =>
  props.file ? extVisual(props.file.ext, props.file.kind) : null
);

watch(
  [visible, () => props.file?.id],
  async ([open]) => {
    url.value = "";
    failed.value = false;
    const file = props.file;
    if (!open || !file || !(isImage.value || isPdf.value)) return;
    loading.value = true;
    try {
      url.value = (await signedUrlFor(file.id)).url;
    } catch {
      failed.value = true;
    } finally {
      loading.value = false;
    }
  },
  { immediate: true }
);
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="file?.originalName ?? '预览'"
    width="min(960px, 92vw)"
    top="5vh"
    destroy-on-close
    append-to-body
    class="file-preview-dialog"
  >
    <div v-if="file" class="preview-layout">
      <div v-loading="loading" class="preview-stage">
        <img
          v-if="isImage && url && !failed"
          :src="url"
          :alt="file.originalName"
          class="preview-image"
          @error="failed = true"
        />
        <iframe
          v-else-if="isPdf && url && !failed"
          :src="url"
          :title="file.originalName"
          class="preview-pdf"
        />
        <div v-else-if="!loading" class="preview-placeholder">
          <component
            :is="visual?.icon"
            class="h-20 w-20"
            :style="{ color: visual?.color }"
          />
          <p class="mt-3 text-sm text-[var(--el-text-color-secondary)]">
            {{
              failed ? "预览加载失败" : "该类型文件不支持在线预览，请下载后查看"
            }}
          </p>
          <el-button
            class="mt-4"
            type="primary"
            @click="emit('download', file)"
          >
            <DownloadIcon class="mr-1" />下载文件
          </el-button>
        </div>
      </div>
      <el-descriptions
        :column="1"
        size="small"
        border
        label-width="76px"
        class="preview-meta"
      >
        <el-descriptions-item label="类型">
          <el-tag
            :type="KIND_TAG_TYPES[file.kind]"
            size="small"
            disable-transitions
          >
            {{ KIND_LABELS[file.kind] }}
          </el-tag>
          <span class="ml-2 text-xs text-[var(--el-text-color-secondary)]">
            {{ file.mimeType }}
          </span>
        </el-descriptions-item>
        <el-descriptions-item label="大小">
          {{ formatSize(file.size) }}
        </el-descriptions-item>
        <el-descriptions-item label="分类">
          {{ file.categoryName || "未分类" }}
        </el-descriptions-item>
        <el-descriptions-item label="上传者">
          {{ file.uploaderName || "-" }}
        </el-descriptions-item>
        <el-descriptions-item label="上传时间">
          {{ formatDateTime(file.createdAt) }}
        </el-descriptions-item>
        <el-descriptions-item label="SHA-256">
          <span class="break-all font-mono text-xs">{{ file.sha256 }}</span>
        </el-descriptions-item>
      </el-descriptions>
    </div>
    <template #footer>
      <el-button v-if="file" @click="emit('copy-link', file)">
        <LinkIcon class="mr-1" />复制链接
      </el-button>
      <el-button v-if="file" type="primary" @click="emit('download', file)">
        <DownloadIcon class="mr-1" />下载
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.preview-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 260px;
  gap: 16px;

  @media (width <= 768px) {
    grid-template-columns: minmax(0, 1fr);
  }
}

.preview-stage {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 420px;
  overflow: hidden;
  background: repeating-conic-gradient(
      var(--el-fill-color-light) 0% 25%,
      var(--el-bg-color) 0% 50%
    )
    50% / 20px 20px;
  border-radius: 8px;
}

.preview-image {
  max-width: 100%;
  max-height: 70vh;
  object-fit: contain;
}

.preview-pdf {
  width: 100%;
  height: 70vh;
  background: var(--el-bg-color);
  border: 0;
}

.preview-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 32px;
}

.preview-meta {
  align-self: start;
}

.preview-meta :deep(.el-descriptions__label) {
  white-space: nowrap;
}
</style>
