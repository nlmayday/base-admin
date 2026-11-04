<script setup lang="ts">
import type { FileItem } from "@/api/file";
import { formatDateTime } from "@/utils/format";
import type { FileAction } from "../types";
import { KIND_LABELS, KIND_TAG_TYPES, formatSize } from "../utils";
import FileThumb from "./FileThumb.vue";
import FileActions from "./FileActions.vue";

defineOptions({ name: "FileTable" });

defineProps<{ files: FileItem[] }>();
const emit = defineEmits<{
  (e: "action", action: FileAction, file: FileItem): void;
}>();
</script>

<template>
  <el-table :data="files" row-key="id" border>
    <el-table-column label="文件" min-width="260">
      <template #default="{ row }">
        <div class="flex items-center gap-3">
          <div
            class="h-10 w-10 shrink-0 cursor-zoom-in overflow-hidden rounded bg-[var(--el-fill-color-lighter)]"
            @click="emit('action', 'preview', row as FileItem)"
          >
            <FileThumb :file="row as FileItem" :icon-size="24" />
          </div>
          <el-button
            link
            class="min-w-0 justify-start! font-normal!"
            @click="emit('action', 'preview', row as FileItem)"
          >
            <span class="truncate" :title="row.originalName">
              {{ row.originalName }}
            </span>
          </el-button>
        </div>
      </template>
    </el-table-column>
    <el-table-column label="类型" width="90" align="center">
      <template #default="{ row }">
        <el-tag
          :type="KIND_TAG_TYPES[row.kind as FileItem['kind']]"
          size="small"
          disable-transitions
        >
          {{ KIND_LABELS[row.kind as FileItem["kind"]] }}
        </el-tag>
      </template>
    </el-table-column>
    <el-table-column label="大小" width="100" align="right">
      <template #default="{ row }">{{ formatSize(row.size) }}</template>
    </el-table-column>
    <el-table-column label="分类" min-width="110" show-overflow-tooltip>
      <template #default="{ row }">
        <span v-if="row.categoryName">{{ row.categoryName }}</span>
        <span v-else class="text-[var(--el-text-color-placeholder)]"
          >未分类</span
        >
      </template>
    </el-table-column>
    <el-table-column
      prop="uploaderName"
      label="上传者"
      min-width="100"
      show-overflow-tooltip
    />
    <el-table-column label="上传时间" width="215">
      <template #default="{ row }">{{
        formatDateTime(row.createdAt)
      }}</template>
    </el-table-column>
    <el-table-column label="操作" width="190" fixed="right">
      <template #default="{ row }">
        <div class="flex items-center">
          <el-button
            link
            type="primary"
            @click="emit('action', 'preview', row as FileItem)"
          >
            预览
          </el-button>
          <el-button
            link
            type="primary"
            @click="emit('action', 'copy-link', row as FileItem)"
          >
            复制链接
          </el-button>
          <el-button
            link
            type="primary"
            @click="emit('action', 'download', row as FileItem)"
          >
            下载
          </el-button>
          <FileActions
            class="ml-2"
            :file="row as FileItem"
            @action="a => emit('action', a, row as FileItem)"
          />
        </div>
      </template>
    </el-table-column>
  </el-table>
</template>
