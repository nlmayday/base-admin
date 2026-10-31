<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useIntersectionObserver } from "@vueuse/core";
import type { FileItem } from "@/api/file";
import { extVisual, signedUrlFor } from "../utils";

defineOptions({ name: "FileThumb" });

const props = withDefaults(
  defineProps<{
    file: FileItem;
    /** 图标尺寸（px） */
    iconSize?: number;
  }>(),
  { iconSize: 44 }
);

const root = ref<HTMLElement>();
const src = ref("");
const failed = ref(false);
const visible = ref(false);

const isImage = computed(() => props.file.mimeType.startsWith("image/"));
const visual = computed(() => extVisual(props.file.ext, props.file.kind));

// 进入视口后才申请签名链接，避免一次性为整页图片发起请求
const { stop } = useIntersectionObserver(
  root,
  ([entry]) => {
    if (entry?.isIntersecting) {
      visible.value = true;
      stop();
    }
  },
  { rootMargin: "120px" }
);

watch(
  [visible, () => props.file.id],
  async ([isVisible]) => {
    if (!isVisible || !isImage.value) return;
    failed.value = false;
    try {
      src.value = (await signedUrlFor(props.file.id)).url;
    } catch {
      failed.value = true;
    }
  },
  { immediate: true }
);
</script>

<template>
  <div ref="root" class="file-thumb">
    <img
      v-if="isImage && src && !failed"
      :src="src"
      :alt="file.originalName"
      loading="lazy"
      decoding="async"
      draggable="false"
      @error="failed = true"
    />
    <div v-else class="file-thumb__icon" :style="{ color: visual.color }">
      <component
        :is="visual.icon"
        :style="{ width: `${iconSize}px`, height: `${iconSize}px` }"
      />
      <span v-if="iconSize >= 32" class="file-thumb__ext">
        {{ file.ext.toUpperCase() || "FILE" }}
      </span>
    </div>
  </div>
</template>

<style scoped lang="scss">
.file-thumb {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
}

.file-thumb__icon {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: center;
  justify-content: center;
}

.file-thumb__ext {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  opacity: 0.85;
}
</style>
