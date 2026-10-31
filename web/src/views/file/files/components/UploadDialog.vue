<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import Axios from "axios";
import { ErrorCode, isApiError } from "@/utils/http/errors";
import { handleApiError } from "@/utils/http/handler";
import {
  uploadFile,
  type FileCategory,
  type FileItem,
  type UploadPolicy
} from "@/api/file";
import { extVisual, formatSize, splitName } from "../utils";
import UploadCloudIcon from "~icons/ri/upload-cloud-2-line";
import CheckIcon from "~icons/ri/checkbox-circle-fill";
import ErrorIcon from "~icons/ri/error-warning-fill";
import CloseIcon from "~icons/ri/close-line";
import RefreshIcon from "~icons/ri/refresh-line";

defineOptions({ name: "FileUploadDialog" });

const props = defineProps<{
  categories: FileCategory[];
  /** 打开时默认选中的分类（当前浏览的分类） */
  defaultCategoryId: string | null;
  policy: UploadPolicy | null;
}>();
const visible = defineModel<boolean>({ required: true });
const emit = defineEmits<{ (e: "uploaded", file: FileItem): void }>();

/** 同时上传的文件数 */
const CONCURRENCY = 3;
/** 会话级错误：出现后停止后续上传并交给全局处理（跳转登录等） */
const SESSION_ERRORS = new Set<number>([
  ErrorCode.UNAUTHENTICATED,
  ErrorCode.CSRF_REJECTED,
  ErrorCode.PASSWORD_CHANGE_REQUIRED
]);

type Status = "pending" | "uploading" | "success" | "error";

type QueueItem = {
  uid: number;
  file: File;
  status: Status;
  percent: number;
  error: string;
  controller: AbortController | null;
};

let nextUid = 1;
const queue = reactive<QueueItem[]>([]);
const categoryId = ref<string | null>(null);
const dragging = ref(false);
const fileInput = ref<HTMLInputElement>();

watch(visible, open => {
  if (open && !queue.some(i => i.status === "uploading")) {
    categoryId.value = props.defaultCategoryId;
  }
});

const accept = computed(() =>
  props.policy ? props.policy.extensions.map(e => `.${e}`).join(",") : ""
);

const counts = computed(() => ({
  total: queue.length,
  success: queue.filter(i => i.status === "success").length,
  error: queue.filter(i => i.status === "error").length,
  active: queue.filter(i => i.status === "uploading" || i.status === "pending")
    .length
}));

/** 客户端预检：与服务端规则一致，仅为提前反馈，真正的校验在后端 */
function precheck(file: File): string {
  const policy = props.policy;
  if (file.size === 0) return "不能上传空文件";
  if (!policy) return "";
  const ext = splitName(file.name).ext.toLowerCase();
  if (!ext) return "文件缺少扩展名";
  if (!policy.extensions.includes(ext)) return `不允许上传 .${ext} 类型的文件`;
  if (file.size > policy.maxBytes) {
    return `文件大小超过限制（最大 ${formatSize(policy.maxBytes)}）`;
  }
  return "";
}

function addFiles(files: File[]): void {
  for (const file of files) {
    const error = precheck(file);
    queue.push({
      uid: nextUid++,
      file,
      status: error ? "error" : "pending",
      percent: 0,
      error,
      controller: null
    });
  }
  pump();
}

function errorMessage(err: unknown): string {
  if (Axios.isCancel(err)) return "已取消";
  if (isApiError(err)) {
    if (err.fields.length > 0) return err.fields[0].message;
    return err.message || "上传失败";
  }
  return "上传失败";
}

async function run(item: QueueItem): Promise<void> {
  item.status = "uploading";
  item.percent = 0;
  item.error = "";
  item.controller = new AbortController();
  try {
    const uploaded = await uploadFile(item.file, {
      categoryId: categoryId.value,
      signal: item.controller.signal,
      onProgress: p => {
        // 字节全部发出后服务端仍需写入存储，保留 99% 直到响应返回
        item.percent = Math.min(p, 99);
      }
    });
    item.status = "success";
    item.percent = 100;
    emit("uploaded", uploaded);
  } catch (err) {
    item.status = "error";
    item.error = errorMessage(err);
    if (isApiError(err) && SESSION_ERRORS.has(err.code)) {
      queue
        .filter(i => i.status === "pending")
        .forEach(i => {
          i.status = "error";
          i.error = "已停止";
        });
      handleApiError(err, {});
    }
  } finally {
    item.controller = null;
    pump();
  }
}

function pump(): void {
  let running = queue.filter(i => i.status === "uploading").length;
  for (const item of queue) {
    if (running >= CONCURRENCY) return;
    if (item.status === "pending") {
      running++;
      run(item);
    }
  }
}

function retry(item: QueueItem): void {
  const error = precheck(item.file);
  if (error) {
    item.error = error;
    return;
  }
  item.status = "pending";
  item.error = "";
  pump();
}

function remove(item: QueueItem): void {
  item.controller?.abort();
  const i = queue.indexOf(item);
  if (i >= 0) queue.splice(i, 1);
}

function clearFinished(): void {
  for (let i = queue.length - 1; i >= 0; i--) {
    if (queue[i].status === "success" || queue[i].status === "error") {
      queue.splice(i, 1);
    }
  }
}

function pick(): void {
  fileInput.value?.click();
}

function onInputChange(e: Event): void {
  const input = e.target as HTMLInputElement;
  addFiles(Array.from(input.files ?? []));
  input.value = "";
}

function onDrop(e: DragEvent): void {
  dragging.value = false;
  addFiles(Array.from(e.dataTransfer?.files ?? []));
}

function onDragLeave(e: DragEvent): void {
  const target = e.currentTarget as HTMLElement;
  if (!target.contains(e.relatedTarget as Node | null)) dragging.value = false;
}

defineExpose({ addFiles });
</script>

<template>
  <el-dialog
    v-model="visible"
    title="上传文件"
    width="640px"
    :close-on-click-modal="false"
    append-to-body
  >
    <div class="mb-3 flex items-center gap-3">
      <span class="shrink-0 text-sm text-[var(--el-text-color-regular)]">
        上传到
      </span>
      <el-select
        v-model="categoryId"
        placeholder="未分类"
        clearable
        filterable
        class="w-[240px]!"
        :disabled="counts.active > 0"
      >
        <el-option
          v-for="c in categories"
          :key="c.id"
          :label="c.name"
          :value="c.id"
        />
      </el-select>
    </div>

    <div
      class="drop-zone"
      :class="{ 'is-dragging': dragging }"
      role="button"
      tabindex="0"
      data-testid="upload-drop-zone"
      @click="pick"
      @keydown.enter.prevent="pick"
      @dragenter.prevent="dragging = true"
      @dragover.prevent="dragging = true"
      @dragleave="onDragLeave"
      @drop.prevent="onDrop"
    >
      <UploadCloudIcon class="drop-zone__icon" />
      <div class="text-[15px]">
        将文件拖到此处，或
        <em class="text-[var(--el-color-primary)]">点击选择</em>
      </div>
      <div
        v-if="policy"
        class="mt-2 text-xs leading-5 text-[var(--el-text-color-secondary)]"
      >
        单个文件不超过 {{ formatSize(policy.maxBytes) }}，支持
        {{ policy.extensions.join("、") }}
      </div>
      <input
        ref="fileInput"
        type="file"
        multiple
        class="hidden"
        :accept="accept"
        @change="onInputChange"
      />
    </div>

    <div v-if="queue.length" class="mt-4">
      <div class="mb-2 flex items-center justify-between text-sm">
        <span class="text-[var(--el-text-color-regular)]">
          共 {{ counts.total }} 个，成功
          <b class="text-[var(--el-color-success)]">{{ counts.success }}</b>
          <template v-if="counts.error">
            ，失败
            <b class="text-[var(--el-color-danger)]">{{ counts.error }}</b>
          </template>
        </span>
        <el-button
          link
          type="primary"
          :disabled="counts.success + counts.error === 0"
          @click="clearFinished"
        >
          清除已完成
        </el-button>
      </div>
      <el-scrollbar max-height="300px">
        <ul class="upload-list">
          <li v-for="item in queue" :key="item.uid" class="upload-item">
            <component
              :is="extVisual(splitName(item.file.name).ext).icon"
              class="upload-item__icon"
              :style="{ color: extVisual(splitName(item.file.name).ext).color }"
            />
            <div class="min-w-0 flex-1">
              <div class="flex items-center justify-between gap-2">
                <span class="truncate text-sm" :title="item.file.name">
                  {{ item.file.name }}
                </span>
                <span
                  class="shrink-0 text-xs text-[var(--el-text-color-secondary)]"
                >
                  {{ formatSize(item.file.size) }}
                </span>
              </div>
              <el-progress
                v-if="item.status === 'uploading' || item.status === 'pending'"
                :percentage="item.percent"
                :stroke-width="4"
                :show-text="false"
                class="mt-1"
              />
              <div
                v-else-if="item.status === 'error'"
                class="mt-1 text-xs text-[var(--el-color-danger)]"
              >
                {{ item.error }}
              </div>
              <div v-else class="mt-1 text-xs text-[var(--el-color-success)]">
                上传成功
              </div>
            </div>
            <div class="upload-item__actions">
              <CheckIcon
                v-if="item.status === 'success'"
                class="text-[var(--el-color-success)]"
              />
              <template v-else-if="item.status === 'error'">
                <ErrorIcon class="text-[var(--el-color-danger)]" />
                <el-button link title="重试" @click="retry(item)">
                  <RefreshIcon />
                </el-button>
              </template>
              <span
                v-else-if="item.status === 'uploading'"
                class="w-9 text-right text-xs text-[var(--el-text-color-secondary)]"
              >
                {{ item.percent }}%
              </span>
              <el-button
                link
                :title="item.status === 'uploading' ? '取消' : '移除'"
                @click="remove(item)"
              >
                <CloseIcon />
              </el-button>
            </div>
          </li>
        </ul>
      </el-scrollbar>
    </div>

    <template #footer>
      <span
        v-if="counts.active > 0"
        class="mr-3 text-xs text-[var(--el-text-color-secondary)]"
      >
        关闭窗口不会中断上传
      </span>
      <el-button @click="visible = false">关闭</el-button>
      <el-button type="primary" @click="pick">继续添加</el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.drop-zone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 28px 16px;
  color: var(--el-text-color-regular);
  text-align: center;
  cursor: pointer;
  background: var(--el-fill-color-lighter);
  border: 1.5px dashed var(--el-border-color);
  border-radius: 8px;
  transition:
    border-color 0.15s,
    background-color 0.15s;

  &:hover,
  &:focus-visible,
  &.is-dragging {
    outline: none;
    background: var(--el-color-primary-light-9);
    border-color: var(--el-color-primary);
  }

  em {
    font-style: normal;
  }
}

.drop-zone__icon {
  width: 48px;
  height: 48px;
  margin-bottom: 8px;
  color: var(--el-color-primary);
}

.upload-list {
  padding: 0;
  margin: 0;
  list-style: none;
}

.upload-item {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 8px 4px;
  border-bottom: 1px solid var(--el-border-color-lighter);

  &:last-child {
    border-bottom: none;
  }
}

.upload-item__icon {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
}

.upload-item__actions {
  display: flex;
  flex-shrink: 0;
  gap: 4px;
  align-items: center;

  :deep(.el-button + .el-button) {
    margin-left: 0;
  }
}
</style>
