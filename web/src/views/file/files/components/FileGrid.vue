<script setup lang="ts">
import type { FileItem } from "@/api/file";
import { formatDateTime } from "@/utils/format";
import type { FileAction } from "../types";
import { formatSize } from "../utils";
import FileThumb from "./FileThumb.vue";
import FileActions from "./FileActions.vue";
import EyeIcon from "~icons/ri/eye-line";
import LinkIcon from "~icons/ri/link";
import DownloadIcon from "~icons/ri/download-2-line";

defineOptions({ name: "FileGrid" });

defineProps<{ files: FileItem[] }>();
const emit = defineEmits<{
  (e: "action", action: FileAction, file: FileItem): void;
}>();
</script>

<template>
  <div class="file-grid">
    <div
      v-for="f in files"
      :key="f.id"
      class="file-card"
      tabindex="0"
      :data-testid="`file-card-${f.id}`"
      @dblclick="emit('action', 'preview', f)"
      @keydown.enter="emit('action', 'preview', f)"
    >
      <div class="file-card__media" @click="emit('action', 'preview', f)">
        <FileThumb :file="f" />
        <div class="file-card__overlay">
          <el-tooltip content="预览" placement="top">
            <button type="button" @click.stop="emit('action', 'preview', f)">
              <EyeIcon />
            </button>
          </el-tooltip>
          <el-tooltip content="复制链接" placement="top">
            <button type="button" @click.stop="emit('action', 'copy-link', f)">
              <LinkIcon />
            </button>
          </el-tooltip>
          <el-tooltip content="下载" placement="top">
            <button type="button" @click.stop="emit('action', 'download', f)">
              <DownloadIcon />
            </button>
          </el-tooltip>
        </div>
      </div>
      <div class="file-card__body">
        <div class="flex items-start justify-between gap-1">
          <el-tooltip
            :content="f.originalName"
            placement="top"
            :show-after="500"
          >
            <span class="file-card__name">{{ f.originalName }}</span>
          </el-tooltip>
          <FileActions :file="f" @action="a => emit('action', a, f)" />
        </div>
        <div class="file-card__meta">
          <span>{{ formatSize(f.size) }}</span>
          <span>{{ formatDateTime(f.createdAt).slice(0, 10) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.file-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
  gap: 14px;
}

.file-card {
  overflow: hidden;
  outline: none;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  transition:
    box-shadow 0.2s,
    border-color 0.2s,
    transform 0.2s;

  &:hover,
  &:focus-visible {
    border-color: var(--el-color-primary-light-5);
    box-shadow: var(--el-box-shadow-light);
    transform: translateY(-1px);
  }

  &:hover .file-card__overlay,
  &:focus-within .file-card__overlay {
    opacity: 1;
  }
}

.file-card__media {
  position: relative;
  aspect-ratio: 4 / 3;
  cursor: zoom-in;
  background: var(--el-fill-color-lighter);
}

.file-card__overlay {
  position: absolute;
  inset: auto 0 0;
  display: flex;
  gap: 6px;
  justify-content: center;
  padding: 18px 8px 8px;
  background: linear-gradient(to top, rgb(0 0 0 / 55%), transparent);
  opacity: 0;
  transition: opacity 0.2s;

  button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    font-size: 16px;
    color: #fff;
    cursor: pointer;
    background: rgb(255 255 255 / 18%);
    border: 0;
    border-radius: 50%;
    transition: background-color 0.15s;

    &:hover {
      background: rgb(255 255 255 / 35%);
    }
  }
}

.file-card__body {
  padding: 8px 10px 10px;
}

.file-card__name {
  display: -webkit-box;
  min-height: 2.6em;
  overflow: hidden;
  -webkit-line-clamp: 2;
  font-size: 13px;
  line-height: 1.3;
  color: var(--el-text-color-primary);
  word-break: break-all;
  -webkit-box-orient: vertical;
}

.file-card__meta {
  display: flex;
  justify-content: space-between;
  margin-top: 6px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
