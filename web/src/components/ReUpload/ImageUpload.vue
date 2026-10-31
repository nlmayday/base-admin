<script setup lang="ts">
import type { UploadRawFile } from "element-plus";
import { useAttachmentUpload } from "./useAttachmentUpload";
import AddIcon from "~icons/ri/add-line";

defineOptions({ name: "ReImageUpload" });

/**
 * 单张图片上传：v-model 为图片地址（存入业务表的字符串字段）。
 * `<ReImageUpload v-model="form.coverUrl" />`
 */
const url = defineModel<string>({ default: "" });
const props = withDefaults(defineProps<{ size?: number; tip?: string }>(), {
  size: 96,
  tip: ""
});

const { uploading, percent, upload } = useAttachmentUpload(true);

async function onSelect(file: UploadRawFile): Promise<false> {
  const done = await upload(file);
  if (done) url.value = done.url;
  return false; // 已自行上传，阻止 el-upload 默认行为
}
</script>

<template>
  <div class="re-image-upload">
    <el-upload
      :show-file-list="false"
      accept="image/*"
      :before-upload="onSelect"
      :disabled="uploading"
    >
      <div
        class="box"
        :style="{ width: `${props.size}px`, height: `${props.size}px` }"
      >
        <el-image v-if="url" :src="url" fit="cover" class="w-full h-full" />
        <IconifyIconOffline v-else :icon="AddIcon" class="text-2xl" />
        <div v-if="uploading" class="mask">{{ percent }}%</div>
      </div>
    </el-upload>
    <div class="flex flex-col justify-end gap-1 ml-3">
      <el-button v-if="url" link type="danger" @click="url = ''">
        移除
      </el-button>
      <span
        v-if="props.tip"
        class="text-xs text-[var(--el-text-color-secondary)]"
      >
        {{ props.tip }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.re-image-upload {
  display: flex;
  align-items: stretch;
}

.box {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  color: var(--el-text-color-secondary);
  cursor: pointer;
  border: 1px dashed var(--el-border-color);
  border-radius: 6px;
}

.box:hover {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary);
}

.mask {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  background: rgb(0 0 0 / 45%);
}
</style>
